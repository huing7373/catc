package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/huing7373/catc/server/internal/domain"
	"github.com/huing7373/catc/server/internal/repository"
	"github.com/huing7373/catc/server/pkg/applex"
	"github.com/huing7373/catc/server/pkg/ids"
	"github.com/huing7373/catc/server/pkg/jwtx"
)

// LoginInput carries the validated /v1/auth/login payload.
type LoginInput struct {
	AppleJWT string
	Nonce    string // raw nonce — service computes sha256 internally via applex
	DeviceID string
}

// RefreshInput carries the validated /v1/auth/refresh payload.
type RefreshInput struct {
	RefreshToken string
}

// TokenPair is the success envelope for both Login and Refresh. The
// LoginOutcome field is empty for refreshes.
type TokenPair struct {
	AccessToken      string
	RefreshToken     string
	AccessExpiresAt  time.Time
	RefreshExpiresAt time.Time
	UserID           ids.UserID
	LoginOutcome     repository.LoginOutcome
}

// AuthSvc is the contract consumed by the HTTP handler.
type AuthSvc interface {
	Login(ctx context.Context, in LoginInput) (TokenPair, error)
	Refresh(ctx context.Context, in RefreshInput) (TokenPair, error)
}

// appleVerifier is the consumer-defined view of applex.Verifier.
type appleVerifier interface {
	Verify(ctx context.Context, idToken, rawNonce string) (*applex.Identity, error)
}

// userRepoForAuth is the consumer-defined view of UserRepository
// methods this service needs.
type userRepoForAuth interface {
	UpsertOnAppleLogin(ctx context.Context, appleID, deviceID string, nowFn func() time.Time) (*domain.User, repository.LoginOutcome, error)
	FindByID(ctx context.Context, id ids.UserID) (*domain.User, error)
}

// tokenMinter is the consumer-defined view of jwtx.Manager.
type tokenMinter interface {
	SignAccess(uid ids.UserID) (string, error)
	SignRefresh(uid ids.UserID) (string, error)
	ParseRefresh(token string) (ids.UserID, error)
}

// AuthService implements AuthSvc.
type AuthService struct {
	apple      appleVerifier
	users      userRepoForAuth
	tokens     tokenMinter
	accessTTL  time.Duration
	refreshTTL time.Duration
	nowFn      func() time.Time
}

// NewAuthService builds a *AuthService. nowFn defaults to time.Now and
// is overridable for deterministic tests.
func NewAuthService(
	apple appleVerifier,
	users userRepoForAuth,
	tokens tokenMinter,
	accessTTL, refreshTTL time.Duration,
) *AuthService {
	return &AuthService{
		apple:      apple,
		users:      users,
		tokens:     tokens,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		nowFn:      time.Now,
	}
}

// SetNowFn overrides the clock. Tests only.
func (s *AuthService) SetNowFn(fn func() time.Time) { s.nowFn = fn }

// Login verifies an Apple identity token, upserts the matching user,
// and mints a fresh access/refresh pair.
func (s *AuthService) Login(ctx context.Context, in LoginInput) (TokenPair, error) {
	id, err := s.apple.Verify(ctx, in.AppleJWT, in.Nonce)
	if err != nil {
		return TokenPair{}, mapAppleError(err)
	}
	user, outcome, err := s.users.UpsertOnAppleLogin(ctx, id.Sub, in.DeviceID, s.nowFn)
	if err != nil {
		if errors.Is(err, repository.ErrConflict) {
			return TokenPair{}, ErrAppleAuthFail.WithCause(err)
		}
		return TokenPair{}, fmt.Errorf("auth service: upsert: %w", err)
	}
	pair, err := s.mintPair(user.ID, outcome)
	if err != nil {
		return TokenPair{}, fmt.Errorf("auth service: mint: %w", err)
	}
	log.Ctx(ctx).Info().
		Str("user_id", string(user.ID)).
		Str("login_outcome", string(outcome)).
		Msg("login ok")
	return pair, nil
}

// Refresh validates a refresh token, ensures the user is still active,
// and mints a new access/refresh pair.
func (s *AuthService) Refresh(ctx context.Context, in RefreshInput) (TokenPair, error) {
	uid, err := s.tokens.ParseRefresh(in.RefreshToken)
	if err != nil {
		return TokenPair{}, mapTokenError(err)
	}
	user, err := s.users.FindByID(ctx, uid)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return TokenPair{}, ErrUnauthorized.WithCause(err)
		}
		return TokenPair{}, fmt.Errorf("auth service: refresh find: %w", err)
	}
	if user.IsDeleted {
		// Defensive: FindByID's filter already excludes deleted users,
		// but we double-check in case the contract drifts.
		return TokenPair{}, ErrUnauthorized
	}
	pair, err := s.mintPair(user.ID, "")
	if err != nil {
		return TokenPair{}, fmt.Errorf("auth service: refresh mint: %w", err)
	}
	return pair, nil
}

func (s *AuthService) mintPair(uid ids.UserID, outcome repository.LoginOutcome) (TokenPair, error) {
	now := s.nowFn()
	at, err := s.tokens.SignAccess(uid)
	if err != nil {
		return TokenPair{}, fmt.Errorf("sign access: %w", err)
	}
	rt, err := s.tokens.SignRefresh(uid)
	if err != nil {
		return TokenPair{}, fmt.Errorf("sign refresh: %w", err)
	}
	return TokenPair{
		AccessToken:      at,
		RefreshToken:     rt,
		AccessExpiresAt:  now.Add(s.accessTTL),
		RefreshExpiresAt: now.Add(s.refreshTTL),
		UserID:           uid,
		LoginOutcome:     outcome,
	}, nil
}

func mapAppleError(err error) error {
	switch {
	case errors.Is(err, applex.ErrNonceMismatch):
		return ErrNonceMismatch.WithCause(err)
	case errors.Is(err, applex.ErrExpiredToken),
		errors.Is(err, applex.ErrInvalidToken),
		errors.Is(err, applex.ErrAudienceMismatch),
		errors.Is(err, applex.ErrIssuerMismatch),
		errors.Is(err, applex.ErrJWKSFetchFailed):
		return ErrAppleAuthFail.WithCause(err)
	default:
		return fmt.Errorf("auth service: apple verify: %w", err)
	}
}

func mapTokenError(err error) error {
	switch {
	case errors.Is(err, jwtx.ErrExpiredToken):
		return ErrTokenExpired.WithCause(err)
	case errors.Is(err, jwtx.ErrInvalidToken):
		return ErrTokenInvalid.WithCause(err)
	default:
		return fmt.Errorf("auth service: token parse: %w", err)
	}
}
