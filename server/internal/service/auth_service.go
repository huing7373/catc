package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/golang-jwt/jwt/v5"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
	"github.com/huing/cat/server/pkg/jwtx"
	"github.com/huing/cat/server/pkg/logx"
)

// UserRepository is the consumer-side interface AuthService depends on.
// Defined here (not in internal/repository) so the implementation lives
// in internal/repository while the dependency arrow points
// service → domain only — same pattern as ws.ResumeCache vs
// pkg/redisx.RedisResumeCache from Story 0.12.
//
// Each Mongo method maps to one of the SignInWithApple flow steps. The
// FindByID method exists for ws.RealUserProvider (the Story 0.12 Empty
// → Real upgrade) — AuthService itself only needs the other four.
//
// Sentinel errors (ErrUserNotFound, ErrUserDuplicateHash) are exported
// from internal/repository; callers use errors.Is to branch on them.
type UserRepository interface {
	// EnsureIndexes is called once at startup to create the
	// apple_user_id_hash unique index. Idempotent — a re-run with the
	// same key shape is a no-op in Mongo.
	EnsureIndexes(ctx context.Context) error

	// FindByAppleHash returns the user matching the SHA-256 hex of an
	// Apple `sub`. Returns (nil, ErrUserNotFound) when no document
	// matches; other errors propagate unchanged.
	FindByAppleHash(ctx context.Context, hash string) (*domain.User, error)

	// FindByID returns the user matching the UUID-encoded UserID.
	// Returns (nil, ErrUserNotFound) when no document matches.
	// Used by ws.RealUserProvider; AuthService does not call it.
	FindByID(ctx context.Context, id ids.UserID) (*domain.User, error)

	// Insert creates a new user document. On duplicate-key (concurrent
	// sign-in for the same Apple account) returns ErrUserDuplicateHash
	// so the service can resolve the race by re-reading.
	Insert(ctx context.Context, u *domain.User) error

	// ClearDeletion sets deletion_requested = false on the row,
	// updated_at = now, and clears deletion_requested_at. Returns
	// ErrUserNotFound if no row matched. Called from Story 1.6
	// resurrection during /auth/apple.
	ClearDeletion(ctx context.Context, id ids.UserID) error
}

// AppleVerifier abstracts the jwtx.Manager.VerifyApple call so unit
// tests can drive the result deterministically without booting a
// httptest Apple JWKS endpoint. The production implementation is
// jwtx.Manager itself.
type AppleVerifier interface {
	VerifyApple(ctx context.Context, idToken string, expectedNonceSHA256 string) (*jwtx.AppleIdentityClaims, error)
}

// JWTIssuer abstracts the jwtx.Manager.Issue call. Same rationale as
// AppleVerifier: lets tests replace token signing with a stub that
// returns "access-<jti>" / "refresh-<jti>" deterministically.
type JWTIssuer interface {
	Issue(claims jwtx.CustomClaims) (string, error)
}

// SignInWithAppleRequest is the service-layer input. The handler converts
// dto.SignInWithAppleRequest → this struct (M8 — service never sees DTO).
type SignInWithAppleRequest struct {
	IdentityToken     string
	AuthorizationCode string
	DeviceID          string
	Platform          ids.Platform
	Nonce             string // raw nonce the client sent to Apple SIWA
}

// SignInWithAppleResult is the service-layer output. The handler projects
// the *domain.User into a wire DTO before responding.
type SignInWithAppleResult struct {
	AccessToken  string
	RefreshToken string
	User         *domain.User
	IsNewUser    bool
}

// AuthService implements the SignInWithApple flow. Construction is
// fail-closed: every dependency is required.
type AuthService struct {
	users    UserRepository
	verifier AppleVerifier
	issuer   JWTIssuer
	clock    clockx.Clock
	mode     string // cfg.Server.Mode — currently informational only
}

// NewAuthService wires the dependencies. All non-string fields are
// required — passing nil panics at construction so a misconfigured DI
// graph cannot reach request time.
func NewAuthService(users UserRepository, verifier AppleVerifier, issuer JWTIssuer, clk clockx.Clock, mode string) *AuthService {
	if users == nil {
		panic("service.NewAuthService: users repository must not be nil")
	}
	if verifier == nil {
		panic("service.NewAuthService: apple verifier must not be nil")
	}
	if issuer == nil {
		panic("service.NewAuthService: jwt issuer must not be nil")
	}
	if clk == nil {
		panic("service.NewAuthService: clock must not be nil")
	}
	return &AuthService{users: users, verifier: verifier, issuer: issuer, clock: clk, mode: mode}
}

// SignInWithApple executes the Story 1.1 SIWA flow:
//
//  1. Verify the Apple identity token (signature / iss / aud / exp /
//     kid / nonce) — fail-closed on any negative.
//  2. Hash the Apple sub with SHA-256 hex (NFR-SEC-6 — never store the
//     raw sub).
//  3. Find the user by hash. New user → Insert; existing user with
//     deletion_requested → ClearDeletion (resurrection per Story 1.6).
//     Concurrent-sign-in races (duplicate-key from a parallel watch /
//     phone first sign-in) are self-healed by re-reading.
//  4. Issue per-device access + refresh JWTs.
//  5. Emit a single audit log line with userId / deviceId / platform /
//     isNewUser — no PII (identityToken / sub / email / nonce).
//
// Every fail-closed branch maps to dto.ErrInternalError or
// dto.ErrAuthInvalidIdentityToken — see the matrix in the story Dev
// Notes. The audit log line is deliberately a single Info call (not
// per-stage) so log volume scales with sign-in count, not error
// branches; per-stage Error logs are emitted separately when something
// fails.
func (s *AuthService) SignInWithApple(ctx context.Context, req SignInWithAppleRequest) (*SignInWithAppleResult, error) {
	if req.IdentityToken == "" {
		return nil, dto.ErrValidationError.WithCause(errors.New("identityToken empty"))
	}
	if req.DeviceID == "" {
		return nil, dto.ErrValidationError.WithCause(errors.New("deviceId empty"))
	}
	if req.Platform != ids.PlatformWatch && req.Platform != ids.PlatformIphone {
		return nil, dto.ErrValidationError.WithCause(fmt.Errorf("invalid platform %q", req.Platform))
	}
	if req.Nonce == "" {
		return nil, dto.ErrValidationError.WithCause(errors.New("nonce empty"))
	}

	// SIWA spec: Apple hashes the raw nonce the client sent and embeds
	// the hex digest in the identity token's `nonce` claim.
	expectedNonce := hexSHA256(req.Nonce)

	claims, err := s.verifier.VerifyApple(ctx, req.IdentityToken, expectedNonce)
	if err != nil {
		logx.Ctx(ctx).Info().Err(err).
			Str("action", "sign_in_with_apple_reject").
			Str("stage", "verify").
			Str("deviceId", req.DeviceID).
			Str("platform", string(req.Platform)).
			Msg("sign_in_with_apple_reject")
		return nil, dto.ErrAuthInvalidIdentityToken.WithCause(err)
	}

	// NFR-SEC-6 — hex(SHA-256(sub)) is the only thing persisted; the
	// raw Apple sub never lands in Mongo / logs.
	appleHash := hexSHA256(claims.Subject)

	user, isNewUser, err := s.lookupOrCreate(ctx, appleHash)
	if err != nil {
		return nil, err
	}

	if !isNewUser && user.DeletionRequested {
		if err := s.users.ClearDeletion(ctx, user.ID); err != nil {
			s.logRepoError(ctx, "repo_clear_deletion", req, err)
			return nil, dto.ErrInternalError.WithCause(err)
		}
		user.DeletionRequested = false
		user.DeletionRequestedAt = nil
		logx.Ctx(ctx).Info().
			Str("action", "user_resurrected_from_deletion").
			Str("userId", string(user.ID)).
			Str("deviceId", req.DeviceID).
			Msg("user_resurrected_from_deletion")
	}

	access, err := s.issueToken(req, user.ID, "access")
	if err != nil {
		s.logIssueError(ctx, "jwt_issue_access", req, err)
		return nil, dto.ErrInternalError.WithCause(err)
	}
	refresh, err := s.issueToken(req, user.ID, "refresh")
	if err != nil {
		s.logIssueError(ctx, "jwt_issue_refresh", req, err)
		return nil, dto.ErrInternalError.WithCause(err)
	}

	// Single audit log line — NFR-SEC-10. Fields are camelCase
	// (architecture §P5). Deliberately omits identityToken / sub /
	// appleHash / email / nonce per NFR-COMP-1 PII minimization.
	logx.Ctx(ctx).Info().
		Str("action", "sign_in_with_apple").
		Str("userId", string(user.ID)).
		Str("deviceId", req.DeviceID).
		Str("platform", string(req.Platform)).
		Bool("isNewUser", isNewUser).
		Msg("sign_in_with_apple")

	return &SignInWithAppleResult{
		AccessToken:  access,
		RefreshToken: refresh,
		User:         user,
		IsNewUser:    isNewUser,
	}, nil
}

// lookupOrCreate resolves the user for hash, creating a new row if no
// existing one matches. Insert duplicate-key races are self-healed by
// re-reading once (the canonical "two clients did SIWA at the same
// instant" pattern, common when a watch + phone first sign-in happens).
func (s *AuthService) lookupOrCreate(ctx context.Context, hash string) (*domain.User, bool, error) {
	existing, err := s.users.FindByAppleHash(ctx, hash)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, repository.ErrUserNotFound) {
		s.logRepoErrorMinimal(ctx, "repo_find", err)
		return nil, false, dto.ErrInternalError.WithCause(err)
	}

	now := s.clock.Now()
	u := &domain.User{
		ID:              ids.NewUserID(),
		AppleUserIDHash: hash,
		Preferences:     domain.DefaultPreferences(),
		Sessions:        map[string]domain.Session{},
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	if err := s.users.Insert(ctx, u); err != nil {
		if errors.Is(err, repository.ErrUserDuplicateHash) {
			// Concurrent SIWA race — another caller for the same Apple
			// account just landed. Resolve by re-reading; treat as
			// existing user from this point.
			logx.Ctx(ctx).Info().
				Str("action", "sign_in_race_resolved").
				Msg("duplicate-key on insert; re-reading by apple hash")
			again, retryErr := s.users.FindByAppleHash(ctx, hash)
			if retryErr != nil {
				// Theoretically unreachable: the duplicate-key error
				// proves a row exists. Defense in depth — surface as
				// internal error so retries land on the next request.
				s.logRepoErrorMinimal(ctx, "repo_find_after_race", retryErr)
				return nil, false, dto.ErrInternalError.WithCause(retryErr)
			}
			return again, false, nil
		}
		s.logRepoErrorMinimal(ctx, "repo_insert", err)
		return nil, false, dto.ErrInternalError.WithCause(err)
	}
	return u, true, nil
}

func (s *AuthService) issueToken(req SignInWithAppleRequest, userID ids.UserID, tokenType string) (string, error) {
	claims := jwtx.CustomClaims{
		UserID:    string(userID),
		DeviceID:  req.DeviceID,
		Platform:  string(req.Platform),
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: string(userID),
		},
	}
	return s.issuer.Issue(claims)
}

func (s *AuthService) logRepoError(ctx context.Context, stage string, req SignInWithAppleRequest, err error) {
	logx.Ctx(ctx).Error().Err(err).
		Str("action", "sign_in_with_apple_error").
		Str("stage", stage).
		Str("deviceId", req.DeviceID).
		Str("platform", string(req.Platform)).
		Msg("sign_in_with_apple_error")
}

func (s *AuthService) logRepoErrorMinimal(ctx context.Context, stage string, err error) {
	logx.Ctx(ctx).Error().Err(err).
		Str("action", "sign_in_with_apple_error").
		Str("stage", stage).
		Msg("sign_in_with_apple_error")
}

func (s *AuthService) logIssueError(ctx context.Context, stage string, req SignInWithAppleRequest, err error) {
	logx.Ctx(ctx).Error().Err(err).
		Str("action", "sign_in_with_apple_error").
		Str("stage", stage).
		Str("deviceId", req.DeviceID).
		Str("platform", string(req.Platform)).
		Msg("sign_in_with_apple_error")
}

func hexSHA256(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
