// Package jwtx signs and verifies HS256 JWTs for access and refresh
// tokens. Secrets are injected at construction time and never read from
// the environment directly.
package jwtx

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/huing7373/catc/server/pkg/ids"
)

// Config is the minimal shape jwtx needs.
type Config struct {
	AccessSecret  string
	RefreshSecret string
	AccessTTL     time.Duration
	RefreshTTL    time.Duration
	Issuer        string
}

// TokenKind distinguishes access vs refresh tokens in claims.
type TokenKind string

const (
	// KindAccess marks a short-lived API token.
	KindAccess TokenKind = "access"
	// KindRefresh marks a long-lived token usable only on /v1/auth/refresh.
	KindRefresh TokenKind = "refresh"
)

// Errors exposed by jwtx.
var (
	// ErrInvalidToken is returned when signature, format, or kind is wrong.
	ErrInvalidToken = errors.New("jwtx: invalid token")
	// ErrExpiredToken is returned when the token is well-formed but expired.
	ErrExpiredToken = errors.New("jwtx: token expired")
	// ErrEmptySecret is returned when the manager is built without a secret.
	ErrEmptySecret = errors.New("jwtx: empty secret")
)

// Claims is the custom JWT claim set.
type Claims struct {
	UserID ids.UserID `json:"uid"`
	Kind   TokenKind  `json:"kind"`
	jwt.RegisteredClaims
}

// Manager signs and parses tokens.
type Manager struct {
	cfg Config
}

// New builds a Manager. Empty secrets cause a nil return and error.
func New(cfg Config) (*Manager, error) {
	if cfg.AccessSecret == "" || cfg.RefreshSecret == "" {
		return nil, ErrEmptySecret
	}
	// Defaults apply only when TTL is unset (zero). A negative TTL is
	// accepted so tests can mint pre-expired tokens.
	if cfg.AccessTTL == 0 {
		cfg.AccessTTL = time.Hour
	}
	if cfg.RefreshTTL == 0 {
		cfg.RefreshTTL = 30 * 24 * time.Hour
	}
	if cfg.Issuer == "" {
		cfg.Issuer = "cat-backend"
	}
	return &Manager{cfg: cfg}, nil
}

// MustNew is like New but panics (via jwt error) rather than returning.
// Used by initialize where failure = Fatal.
func MustNew(cfg Config) *Manager {
	m, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return m
}

// SignAccess issues a short-lived access token for uid.
func (m *Manager) SignAccess(uid ids.UserID) (string, error) {
	return m.sign(uid, KindAccess, m.cfg.AccessTTL, m.cfg.AccessSecret)
}

// SignRefresh issues a refresh token for uid.
func (m *Manager) SignRefresh(uid ids.UserID) (string, error) {
	return m.sign(uid, KindRefresh, m.cfg.RefreshTTL, m.cfg.RefreshSecret)
}

// ParseAccess verifies an access token and returns the enclosed user id.
func (m *Manager) ParseAccess(token string) (ids.UserID, error) {
	return m.parse(token, KindAccess, m.cfg.AccessSecret)
}

// ParseRefresh verifies a refresh token and returns the enclosed user id.
func (m *Manager) ParseRefresh(token string) (ids.UserID, error) {
	return m.parse(token, KindRefresh, m.cfg.RefreshSecret)
}

func (m *Manager) sign(uid ids.UserID, kind TokenKind, ttl time.Duration, secret string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: uid,
		Kind:   kind,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    m.cfg.Issuer,
			Subject:   string(uid),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := t.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("jwtx: sign: %w", err)
	}
	return signed, nil
}

func (m *Manager) parse(token string, want TokenKind, secret string) (ids.UserID, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method %v", ErrInvalidToken, t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return "", ErrExpiredToken
		}
		return "", fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	// Defensive explicit-expiry check in case jwt.ParseWithClaims
	// swallowed the exp comparison (some v5 builds accept exp == now).
	if claims.ExpiresAt != nil && !time.Now().Before(claims.ExpiresAt.Time) {
		return "", ErrExpiredToken
	}
	if claims.Kind != want {
		return "", fmt.Errorf("%w: wrong kind %q", ErrInvalidToken, claims.Kind)
	}
	if claims.UserID == "" {
		return "", fmt.Errorf("%w: empty subject", ErrInvalidToken)
	}
	return claims.UserID, nil
}
