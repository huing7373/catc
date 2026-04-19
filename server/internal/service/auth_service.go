package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

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

	// UpsertSession writes sessions.<deviceId> = {current_jti,
	// issued_at} atomically via $set, leaving any other session field
	// (has_apns_token, owned by Story 1.4) untouched. Returns
	// ErrUserNotFound if no user document matched. Called by
	// SignInWithApple (Story 1.1 extension, see the AC7 patch in this
	// story) — SIWA is not subject to the rotation CAS guard because
	// it always represents a fresh interactive login, not a race.
	UpsertSession(ctx context.Context, userID ids.UserID, deviceID string, s domain.Session) error

	// UpsertSessionIfJTIMatches is the rotation-safe variant of
	// UpsertSession used by RefreshToken. The Mongo UpdateOne is gated
	// on sessions.<deviceId>.current_jti == expectedJTI, so two
	// concurrent refreshes racing with the same incoming jti cannot
	// both succeed. Returns repository.ErrSessionStale when the CAS
	// fails (or when the user is missing, which the repo cannot cheaply
	// distinguish from a CAS mismatch in a single query — the service
	// semantic for both cases is "your refresh token is no longer the
	// live one").
	UpsertSessionIfJTIMatches(ctx context.Context, userID ids.UserID, deviceID string, expectedJTI string, s domain.Session) error

	// GetSession returns sessions.<deviceId> or (domain.Session{},
	// false, nil) if the sub-document is absent for an existing user.
	// Returns ErrUserNotFound when the user document itself does not
	// exist. Non-Mongo errors propagate unchanged.
	GetSession(ctx context.Context, userID ids.UserID, deviceID string) (domain.Session, bool, error)

	// ListDeviceIDs returns every key of sessions for userID. Used
	// exclusively by AuthService.RevokeAllUserTokens (Story 1.6
	// account deletion). Returns ErrUserNotFound on missing user;
	// returns []string{} (not nil) when the user exists but has no
	// sessions yet.
	ListDeviceIDs(ctx context.Context, userID ids.UserID) ([]string, error)
}

// AppleVerifier abstracts the jwtx.Manager.VerifyApple call so unit
// tests can drive the result deterministically without booting a
// httptest Apple JWKS endpoint. The production implementation is
// jwtx.Manager itself.
type AppleVerifier interface {
	VerifyApple(ctx context.Context, idToken string, expectedNonceSHA256 string) (*jwtx.AppleIdentityClaims, error)
}

// JWTIssuer abstracts the jwtx.Manager.Issue + RefreshExpiry calls.
// Same rationale as AppleVerifier: lets tests replace token signing
// with a stub that returns deterministic values. RefreshExpiry is the
// conservative TTL used by RevokeRefreshToken (Story 1.2 AC8) when the
// caller does not know the original token's exp — worst case: the
// blacklist entry lingers past the token's natural expiry, which is
// harmless.
type JWTIssuer interface {
	Issue(claims jwtx.CustomClaims) (string, error)
	RefreshExpiry() time.Duration
}

// RefreshVerifier abstracts jwtx.Manager.Verify for RefreshToken.
// Production wires the same *jwtx.Manager that implements AppleVerifier
// + JWTIssuer; tests inject a narrow fake that returns deterministic
// claims + errors.
type RefreshVerifier interface {
	Verify(tokenStr string) (*jwtx.CustomClaims, error)
}

// RefreshBlacklist abstracts pkg/redisx.RefreshBlacklist (Story 1.2
// AC3). The service never imports pkg/redisx directly — production
// wiring in cmd/cat/initialize.go constructs the concrete store and
// injects it as this interface.
//
// Fail-closed is a CALLER obligation: both IsRevoked (read path) and
// Revoke (write path) surface Redis errors unchanged, and RefreshToken
// MUST wrap them as dto.ErrInternalError per architecture §21.3.
type RefreshBlacklist interface {
	IsRevoked(ctx context.Context, jti string) (bool, error)
	Revoke(ctx context.Context, jti string, exp time.Time) error
}

// AccessBlacklistRemover is the optional user-level blacklist write
// surface AuthService consults during the SIWA resurrection path.
// Injected via SetAccessBlacklistRemover after construction so the
// existing NewAuthService signature stays stable (tests that don't
// exercise resurrection leave it nil).
//
// Purpose: when a user returns via SIWA inside the 30-day grace
// window, Story 1.1's ClearDeletion flips deletion_requested=false,
// but WITHOUT this hook the Redis blacklist entry that Story 1.6
// wrote on DELETE would still reject the freshly-issued access
// token. Remove here closes that gap — resurrected users get a
// clean session immediately, not after TTL expiry.
//
// Fail-open with warn: a Remove failure is observable in ops but
// does not block the SIWA response (the user's new tokens still
// work, just with a short window of 401s until TTL expires naturally
// or the user retries).
type AccessBlacklistRemover interface {
	Remove(ctx context.Context, userID string) error
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

// AuthService implements the Story 1.1 SignInWithApple + Story 1.2
// RefreshToken / RevokeRefreshToken / RevokeAllUserTokens flows.
// Construction is fail-closed: every required dependency panics on
// nil. accessBlacklist is the only optional field (set via
// SetAccessBlacklistRemover) because it is a Story 1.6 round-1
// addition; tests that don't exercise resurrection leave it nil.
type AuthService struct {
	users           UserRepository
	appleVerifier   AppleVerifier
	refreshVerifier RefreshVerifier
	issuer          JWTIssuer
	blacklist       RefreshBlacklist
	clock           clockx.Clock
	mode            string // cfg.Server.Mode — currently informational only
	// accessBlacklist is optional — nil = skip blacklist clear on
	// resurrection (test harnesses, debug paths that predate 1.6).
	accessBlacklist AccessBlacklistRemover
}

// NewAuthService wires the dependencies. All non-string fields are
// required — passing nil panics at construction so a misconfigured DI
// graph cannot reach request time.
//
// Production wires the same *jwtx.Manager into appleVerifier,
// refreshVerifier, and issuer; tests inject narrow fakes per argument
// so individual failure paths can be driven without cross-method
// coupling.
func NewAuthService(
	users UserRepository,
	appleVerifier AppleVerifier,
	refreshVerifier RefreshVerifier,
	issuer JWTIssuer,
	blacklist RefreshBlacklist,
	clk clockx.Clock,
	mode string,
) *AuthService {
	if users == nil {
		panic("service.NewAuthService: users repository must not be nil")
	}
	if appleVerifier == nil {
		panic("service.NewAuthService: apple verifier must not be nil")
	}
	if refreshVerifier == nil {
		panic("service.NewAuthService: refresh verifier must not be nil")
	}
	if issuer == nil {
		panic("service.NewAuthService: jwt issuer must not be nil")
	}
	if blacklist == nil {
		panic("service.NewAuthService: refresh blacklist must not be nil")
	}
	if clk == nil {
		panic("service.NewAuthService: clock must not be nil")
	}
	return &AuthService{
		users:           users,
		appleVerifier:   appleVerifier,
		refreshVerifier: refreshVerifier,
		issuer:          issuer,
		blacklist:       blacklist,
		clock:           clk,
		mode:            mode,
	}
}

// SetAccessBlacklistRemover installs the Story 1.6 round-1
// resurrection hook. Pass nil (or never call) to disable the
// behaviour — SIWA still succeeds, but a resurrected user will see
// 401 blacklist responses until the Redis TTL expires naturally.
// Production wires the same *redisx.RedisBlacklist that
// AccountDeletionService uses, so DELETE + resurrection converge on
// one blacklist entry.
func (s *AuthService) SetAccessBlacklistRemover(r AccessBlacklistRemover) {
	s.accessBlacklist = r
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

	claims, err := s.appleVerifier.VerifyApple(ctx, req.IdentityToken, expectedNonce)
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

		// Story 1.6 round-1: clear the user-level access blacklist
		// entry DELETE wrote, so the new access + refresh tokens
		// issued below pass subsequent /v1/* and /ws checks without
		// waiting for TTL. Fail-open with warn: a Remove failure
		// leaves the blacklist in place for ≤15 minutes, which
		// shows up as 401 noise in client logs but does not break
		// the SIWA response (the tokens are valid once TTL passes).
		if s.accessBlacklist != nil {
			if rmErr := s.accessBlacklist.Remove(ctx, string(user.ID)); rmErr != nil {
				logx.Ctx(ctx).Warn().Err(rmErr).
					Str("action", "user_resurrection_blacklist_clear_error").
					Str("userId", string(user.ID)).
					Msg("user_resurrection_blacklist_clear_error")
			}
		}

		logx.Ctx(ctx).Info().
			Str("action", "user_resurrected_from_deletion").
			Str("userId", string(user.ID)).
			Str("deviceId", req.DeviceID).
			Msg("user_resurrected_from_deletion")
	}

	// Story 1.2 AC7: pre-generate the refresh token's jti so we can
	// both sign it into the token and write it to
	// users.sessions[deviceId].current_jti BEFORE returning to the
	// client. Missing this step is what makes every subsequent
	// /auth/refresh call fail reuse-detection with "session not
	// initialized".
	accessJTI := ids.NewRefreshJTI() // access jti is audit-only; using the same helper keeps UUIDs uniform
	refreshJTI := ids.NewRefreshJTI()

	access, err := s.issueTokenWithJTI(req, user.ID, "access", accessJTI)
	if err != nil {
		s.logIssueError(ctx, "jwt_issue_access", req, err)
		return nil, dto.ErrInternalError.WithCause(err)
	}
	refresh, err := s.issueTokenWithJTI(req, user.ID, "refresh", refreshJTI)
	if err != nil {
		s.logIssueError(ctx, "jwt_issue_refresh", req, err)
		return nil, dto.ErrInternalError.WithCause(err)
	}

	// Story 1.2 AC7: persist the refresh jti to sessions[deviceId].
	// Fail-closed — if we cannot record the session, the client must
	// not receive the token (otherwise the very next refresh would hit
	// reuse detection on a legitimate session).
	if err := s.users.UpsertSession(ctx, user.ID, req.DeviceID, domain.Session{
		CurrentJTI: refreshJTI,
		IssuedAt:   s.clock.Now(),
	}); err != nil {
		s.logRepoError(ctx, "repo_upsert_session", req, err)
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

// issueTokenWithJTI builds access/refresh claims for the SIWA flow with
// an explicit caller-supplied jti. Story 1.2 rolling-rotation depends
// on the refresh jti being the SAME value the service writes to
// users.sessions[deviceId].current_jti — re-using a single constructor
// keeps that invariant visible.
func (s *AuthService) issueTokenWithJTI(req SignInWithAppleRequest, userID ids.UserID, tokenType string, jti string) (string, error) {
	claims := jwtx.CustomClaims{
		UserID:    string(userID),
		DeviceID:  req.DeviceID,
		Platform:  string(req.Platform),
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: string(userID),
			ID:      jti,
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

// RefreshTokenRequest is the service-layer input for POST /auth/refresh.
// Deliberately narrow: userId / deviceId / platform come from the
// verified refresh claims, not from the request body — a compromised
// client cannot trick the server by lying about its deviceId (defense
// in depth vs the §8.2 session path injection angle).
type RefreshTokenRequest struct {
	RefreshToken string
}

// RefreshTokenResult is the service-layer output. Deliberately does not
// return the domain.User — this is a token rotation, not a login; the
// client already holds the user profile from the prior sign-in.
type RefreshTokenResult struct {
	AccessToken  string
	RefreshToken string
}

// RefreshToken is the Story 1.2 rolling-rotation + stolen-token-reuse-
// detection flow. Every step is fail-closed per the decision matrix
// (Dev Notes §fail-closed / fail-open):
//
//  1. Verify refresh token (signature / iss / alg / kid / exp) — any
//     failure ⇒ AUTH_INVALID_IDENTITY_TOKEN.
//  2. Refuse if claims.TokenType != "refresh" — an access token must
//     never be accepted at the refresh endpoint.
//  3. Check blacklist — any Redis error ⇒ INTERNAL_ERROR (fail-closed,
//     no "assume clean"). Hit ⇒ AUTH_REFRESH_TOKEN_REVOKED.
//  4. Reuse detection via sessions[deviceId].current_jti:
//     - GetSession err ⇒ INTERNAL_ERROR (fail-closed).
//     - ok=false or current_jti empty ⇒ AUTH_REFRESH_TOKEN_REVOKED
//       with reason=session_not_initialized (an uninitialized session
//       is indistinguishable from a stolen token replayed against a
//       device that never had a session).
//     - current_jti != claims.ID ⇒ stolen-token reuse detected.
//       Revoke the current jti (burns the live token) and return
//       AUTH_REFRESH_TOKEN_REVOKED. If the Revoke itself fails we
//       return INTERNAL_ERROR — a reuse detection that cannot burn
//       the live token leaves the attack window open.
//  5. Issue new access + refresh tokens with fresh jtis.
//  6. Persist sessions[deviceId].current_jti = new refresh jti.
//     Any error ⇒ INTERNAL_ERROR (token was signed but never reached
//     the client; next refresh will retry from the old jti). No
//     atomicity across Mongo+Redis here — the reuse-detection path
//     absorbs the inconsistency per Dev Notes.
//  7. Revoke old jti with ttl = claims.exp - now. Any error ⇒
//     INTERNAL_ERROR (rare; the new token was already written — the
//     client gets a 500 and will retry from the updated session on
//     the next refresh, and reuse detection will burn the un-blacklisted
//     old jti on any reuse attempt).
func (s *AuthService) RefreshToken(ctx context.Context, req RefreshTokenRequest) (*RefreshTokenResult, error) {
	if req.RefreshToken == "" {
		return nil, dto.ErrValidationError.WithCause(errors.New("refreshToken empty"))
	}

	// Step 1: verify signature / iss / exp / alg / kid.
	claims, err := s.refreshVerifier.Verify(req.RefreshToken)
	if err != nil {
		logx.Ctx(ctx).Info().Err(err).
			Str("action", "refresh_token_reject").
			Str("reason", "verify_failed").
			Msg("refresh_token_reject")
		return nil, dto.ErrAuthInvalidIdentityToken.WithCause(err)
	}

	// Step 2: token-type guard. An access token must not pass refresh.
	if claims.TokenType != "refresh" {
		logx.Ctx(ctx).Info().
			Str("action", "refresh_token_reject").
			Str("reason", "token_type_mismatch").
			Str("deviceId", claims.DeviceID).
			Str("tokenType", claims.TokenType).
			Msg("refresh_token_reject")
		return nil, dto.ErrAuthInvalidIdentityToken.WithCause(errors.New("not a refresh token"))
	}

	userID := ids.UserID(claims.UserID)
	deviceID := claims.DeviceID
	oldJTI := claims.ID

	// Step 3: blacklist check — Redis error is fail-closed.
	revoked, err := s.blacklist.IsRevoked(ctx, oldJTI)
	if err != nil {
		logx.Ctx(ctx).Error().Err(err).
			Str("action", "refresh_token_error").
			Str("stage", "blacklist_check").
			Str("userId", string(userID)).
			Str("deviceId", deviceID).
			Msg("refresh_token_error")
		return nil, dto.ErrInternalError.WithCause(err)
	}
	if revoked {
		logx.Ctx(ctx).Info().
			Str("action", "refresh_token_reject").
			Str("reason", "blacklisted").
			Str("userId", string(userID)).
			Str("deviceId", deviceID).
			Msg("refresh_token_reject")
		return nil, dto.ErrAuthRefreshTokenRevoked.WithCause(nil)
	}

	// Step 4: reuse detection via sessions[deviceId].current_jti.
	session, sessionOK, err := s.users.GetSession(ctx, userID, deviceID)
	if err != nil {
		logx.Ctx(ctx).Error().Err(err).
			Str("action", "refresh_token_error").
			Str("stage", "repo_get_session").
			Str("userId", string(userID)).
			Str("deviceId", deviceID).
			Msg("refresh_token_error")
		return nil, dto.ErrInternalError.WithCause(err)
	}
	if !sessionOK || session.CurrentJTI == "" {
		logx.Ctx(ctx).Info().
			Str("action", "refresh_token_reject").
			Str("reason", "session_not_initialized").
			Str("userId", string(userID)).
			Str("deviceId", deviceID).
			Msg("refresh_token_reject")
		return nil, dto.ErrAuthRefreshTokenRevoked.WithCause(errors.New("session not initialized for device"))
	}
	if session.CurrentJTI != oldJTI {
		// Stolen-token reuse detected. Burn the live jti so the attacker
		// cannot continue using it. TTL = configured refresh expiry
		// (conservative over-estimate; we do not have the live token's
		// exp in hand here).
		burnTTL := s.issuer.RefreshExpiry()
		burnExp := s.clock.Now().Add(burnTTL)
		if revokeErr := s.blacklist.Revoke(ctx, session.CurrentJTI, burnExp); revokeErr != nil {
			// Reuse detection without the burn ⇒ attack window stays
			// open. Surface as INTERNAL_ERROR so ops see the
			// inconsistency rather than a silent 401.
			logx.Ctx(ctx).Error().Err(revokeErr).
				Str("action", "refresh_token_error").
				Str("stage", "blacklist_revoke").
				Str("reasonSubStage", "reuse_detection_burn").
				Str("userId", string(userID)).
				Str("deviceId", deviceID).
				Str("oldJti", oldJTI).
				Str("currentJti", session.CurrentJTI).
				Msg("refresh_token_error")
			return nil, dto.ErrInternalError.WithCause(revokeErr)
		}
		logx.Ctx(ctx).Warn().
			Str("action", "refresh_token_reuse_detected").
			Str("userId", string(userID)).
			Str("deviceId", deviceID).
			Str("oldJti", oldJTI).
			Str("currentJti", session.CurrentJTI).
			Msg("refresh_token_reuse_detected")
		return nil, dto.ErrAuthRefreshTokenRevoked.WithCause(errors.New("refresh token reuse detected"))
	}

	// Step 5: issue new access + refresh with fresh jtis.
	newRefreshJTI := ids.NewRefreshJTI()
	newAccessJTI := ids.NewRefreshJTI() // access jti audit-only; not in sessions
	platform := ids.Platform(claims.Platform)
	syntheticReq := SignInWithAppleRequest{DeviceID: deviceID, Platform: platform}

	access, err := s.issueTokenWithJTI(syntheticReq, userID, "access", newAccessJTI)
	if err != nil {
		logx.Ctx(ctx).Error().Err(err).
			Str("action", "refresh_token_error").
			Str("stage", "jwt_issue_access").
			Str("userId", string(userID)).
			Str("deviceId", deviceID).
			Msg("refresh_token_error")
		return nil, dto.ErrInternalError.WithCause(err)
	}
	refresh, err := s.issueTokenWithJTI(syntheticReq, userID, "refresh", newRefreshJTI)
	if err != nil {
		logx.Ctx(ctx).Error().Err(err).
			Str("action", "refresh_token_error").
			Str("stage", "jwt_issue_refresh").
			Str("userId", string(userID)).
			Str("deviceId", deviceID).
			Msg("refresh_token_error")
		return nil, dto.ErrInternalError.WithCause(err)
	}

	// Step 6: revoke the old jti FIRST (see Dev Notes "UpsertSession +
	// Revoke 不是原子" — round-1 review P2). A transient blacklist-write
	// failure at this stage leaves the session unchanged, so the client
	// can retry with the same oldJTI and recover. If we had already
	// persisted the new jti to sessions, a Revoke failure would force
	// the legitimate user to re-login: their retry would hit reuse
	// detection, which would burn the un-delivered newJTI.
	if err := s.blacklist.Revoke(ctx, oldJTI, claims.ExpiresAt.Time); err != nil {
		logx.Ctx(ctx).Error().Err(err).
			Str("action", "refresh_token_error").
			Str("stage", "blacklist_revoke").
			Str("userId", string(userID)).
			Str("deviceId", deviceID).
			Str("oldJti", oldJTI).
			Msg("refresh_token_error")
		return nil, dto.ErrInternalError.WithCause(err)
	}

	// Step 7: persist new session state with a compare-and-swap on the
	// current_jti (round-1 review P1). Two concurrent refreshes that
	// both pass the Step-4 reuse-detection gate cannot both succeed at
	// this CAS — only the request observing `sessions.<device>.current_jti
	// == oldJTI` wins. The loser gets ErrSessionStale and we surface it
	// as AUTH_REFRESH_TOKEN_REVOKED (no burn: whoever won already
	// committed their newJTI, so the single-use invariant is preserved).
	if err := s.users.UpsertSessionIfJTIMatches(ctx, userID, deviceID, oldJTI, domain.Session{
		CurrentJTI: newRefreshJTI,
		IssuedAt:   s.clock.Now(),
	}); err != nil {
		if errors.Is(err, repository.ErrSessionStale) {
			logx.Ctx(ctx).Info().
				Str("action", "refresh_token_reject").
				Str("reason", "rotation_race_lost").
				Str("userId", string(userID)).
				Str("deviceId", deviceID).
				Str("oldJti", oldJTI).
				Msg("refresh_token_reject")
			return nil, dto.ErrAuthRefreshTokenRevoked.WithCause(err)
		}
		logx.Ctx(ctx).Error().Err(err).
			Str("action", "refresh_token_error").
			Str("stage", "repo_upsert_session").
			Str("userId", string(userID)).
			Str("deviceId", deviceID).
			Msg("refresh_token_error")
		return nil, dto.ErrInternalError.WithCause(err)
	}

	logx.Ctx(ctx).Info().
		Str("action", "refresh_token").
		Str("userId", string(userID)).
		Str("deviceId", deviceID).
		Str("oldJti", oldJTI).
		Str("newJti", newRefreshJTI).
		Msg("refresh_token")

	return &RefreshTokenResult{AccessToken: access, RefreshToken: refresh}, nil
}

// RevokeRefreshToken revokes the refresh token currently bound to
// (userID, deviceID) — i.e. sessions[deviceID].current_jti. Idempotent:
// a missing user or missing session returns nil so Story 1.6 does not
// need to pre-check. The revoked jti is blacklisted with the full
// RefreshExpiry TTL because we do not have the original token's exp at
// this point (conservative over-estimate — worst case the blacklist
// entry lingers past the token's natural expiry, harmless).
//
// Does not clear sessions[deviceID].current_jti; leaving it in place
// serves as an audit trail and causes any subsequent reuse attempt to
// trip reuse detection (double coverage).
func (s *AuthService) RevokeRefreshToken(ctx context.Context, userID ids.UserID, deviceID string) error {
	session, ok, err := s.users.GetSession(ctx, userID, deviceID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil // idempotent — nothing to revoke on a deleted user
		}
		return fmt.Errorf("revoke refresh token: get session: %w", err)
	}
	if !ok || session.CurrentJTI == "" {
		return nil // idempotent — never signed in on this device
	}

	burnTTL := s.issuer.RefreshExpiry()
	burnExp := s.clock.Now().Add(burnTTL)
	if err := s.blacklist.Revoke(ctx, session.CurrentJTI, burnExp); err != nil {
		return fmt.Errorf("revoke refresh token: blacklist: %w", err)
	}

	logx.Ctx(ctx).Info().
		Str("action", "revoke_refresh_token").
		Str("userId", string(userID)).
		Str("deviceId", deviceID).
		Str("jti", session.CurrentJTI).
		Msg("revoke_refresh_token")
	return nil
}

// RevokeAllUserTokens blacklists sessions[<device>].current_jti for
// every device of userID. Called from Story 1.6 account deletion.
// Iterates ListDeviceIDs and delegates to RevokeRefreshToken.
//
// Best-effort: a per-device failure does not short-circuit the loop —
// we try every device then return the first error observed. Story 1.6
// should treat any error as "partial revoke, account deletion still
// proceeds, ops alert logged". An ErrUserNotFound from ListDeviceIDs
// is treated as idempotent (nil) — a user that never existed or was
// already fully cleaned up is equivalent to "nothing to revoke".
func (s *AuthService) RevokeAllUserTokens(ctx context.Context, userID ids.UserID) error {
	deviceIDs, err := s.users.ListDeviceIDs(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil
		}
		return fmt.Errorf("revoke all user tokens: list devices: %w", err)
	}

	var firstErr error
	for _, d := range deviceIDs {
		if err := s.RevokeRefreshToken(ctx, userID, d); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	if firstErr == nil {
		logx.Ctx(ctx).Info().
			Str("action", "revoke_all_user_tokens").
			Str("userId", string(userID)).
			Int("deviceCount", len(deviceIDs)).
			Msg("revoke_all_user_tokens")
	}
	return firstErr
}
