package jwtx

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"

	"github.com/huing/cat/server/internal/config"
)

// CustomClaims holds the application-specific JWT claims.
type CustomClaims struct {
	UserID    string `json:"uid"`
	DeviceID  string `json:"did"`
	Platform  string `json:"plat"`
	TokenType string `json:"ttype"`
	jwt.RegisteredClaims
}

// Manager handles RS256 JWT signing and verification with key rotation support.
type Manager struct {
	activeKey     *rsa.PrivateKey
	activePub     *rsa.PublicKey
	activeKID     string
	oldPub        *rsa.PublicKey
	oldKID        string
	issuer        string
	accessExpiry  time.Duration
	refreshExpiry time.Duration
}

// New creates a Manager from config. Reads RSA PEM key files.
// Calls log.Fatal if the active key cannot be loaded.
func New(cfg config.JWTCfg) *Manager {
	if cfg.ActiveKID == "" {
		log.Fatal().Msg("jwt: active_kid must not be empty")
	}
	if cfg.Issuer == "" {
		log.Fatal().Msg("jwt: issuer must not be empty")
	}

	activeKey := mustLoadPrivateKey(cfg.PrivateKeyPath)

	m := &Manager{
		activeKey:     activeKey,
		activePub:     &activeKey.PublicKey,
		activeKID:     cfg.ActiveKID,
		issuer:        cfg.Issuer,
		accessExpiry:  time.Duration(cfg.AccessExpirySec) * time.Second,
		refreshExpiry: time.Duration(cfg.RefreshExpirySec) * time.Second,
	}

	if cfg.PrivateKeyPathOld != "" {
		if cfg.OldKID == "" {
			log.Fatal().Msg("jwt: old_kid must not be empty when private_key_path_old is set")
		}
		oldKey := mustLoadPrivateKey(cfg.PrivateKeyPathOld)
		m.oldPub = &oldKey.PublicKey
		m.oldKID = cfg.OldKID
	}

	return m
}

// Issue signs a JWT with the active key. The kid header is set for key selection during verification.
func (m *Manager) Issue(claims CustomClaims) (string, error) {
	now := time.Now()

	expiry := m.accessExpiry
	if claims.TokenType == "refresh" {
		expiry = m.refreshExpiry
	}

	claims.Issuer = m.issuer
	claims.IssuedAt = jwt.NewNumericDate(now)
	claims.ExpiresAt = jwt.NewNumericDate(now.Add(expiry))

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = m.activeKID

	return token.SignedString(m.activeKey)
}

// Verify parses and validates a JWT. It selects the public key by matching the kid header.
func (m *Manager) Verify(tokenStr string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &CustomClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodRS256 {
			return nil, errors.New("unexpected signing method: " + t.Method.Alg())
		}

		kid, _ := t.Header["kid"].(string)
		if kid == m.activeKID {
			return m.activePub, nil
		}
		if m.oldPub != nil && kid == m.oldKID {
			return m.oldPub, nil
		}
		return nil, errors.New("unknown kid: " + kid)
	}, jwt.WithIssuer(m.issuer))
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*CustomClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}
	return claims, nil
}

// AccessExpiry returns the configured access token duration.
func (m *Manager) AccessExpiry() time.Duration { return m.accessExpiry }

// RefreshExpiry returns the configured refresh token duration.
func (m *Manager) RefreshExpiry() time.Duration { return m.refreshExpiry }

func mustLoadPrivateKey(path string) *rsa.PrivateKey {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal().Err(err).Str("path", path).Msg("jwt: read private key failed")
	}

	block, _ := pem.Decode(data)
	if block == nil {
		log.Fatal().Str("path", path).Msg("jwt: no PEM block found")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			log.Fatal().Err(err).Str("path", path).Msg("jwt: parse private key failed")
		}
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		log.Fatal().Str("path", path).Msg("jwt: key is not RSA")
	}
	return rsaKey
}
