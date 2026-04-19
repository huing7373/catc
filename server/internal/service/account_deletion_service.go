package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/internal/ws"
	"github.com/huing/cat/server/pkg/ids"
	"github.com/huing/cat/server/pkg/logx"
)

// --- Consumer-side interfaces (P2 §6.2) ----------------------------------
//
// Declared in this package so the concrete collaborators live where they
// were born (repo / auth / ws / redisx) while this service owns the
// narrow surface it actually depends on. Tests substitute fakes per
// interface without dragging in mongo / redis / gorilla drivers.

// accountDeletionUserRepo is the repo surface AccountDeletionService
// consumes. The returned (user, firstTime) tuple lets the service emit
// an accurate `wasAlreadyRequested` audit field without a second read:
//
//	firstTime=true  → this call performed the write; user.DeletionRequestedAt == Clock.Now().
//	firstTime=false → idempotent repeat; user.DeletionRequestedAt is the ORIGINAL first-write stamp.
type accountDeletionUserRepo interface {
	MarkDeletionRequested(ctx context.Context, userID ids.UserID) (*domain.User, bool, error)
}

// accountDeletionTokenRevoker matches *AuthService.RevokeAllUserTokens
// (Story 1.2). Revokes every refresh jti bound to the user.
type accountDeletionTokenRevoker interface {
	RevokeAllUserTokens(ctx context.Context, userID ids.UserID) error
}

// accountDeletionSessionDisconnector matches *ws.Hub.DisconnectUser
// (Story 1.3). Closes every live WS connection for the user; ws.UserID
// is a `string` alias so the service-side conversion is lexically
// explicit at the call site.
type accountDeletionSessionDisconnector interface {
	DisconnectUser(userID ws.UserID) (int, error)
}

// accountDeletionCacheInvalidator reuses the Story 0.12
// ws.ResumeCacheInvalidator shape verbatim (§21.2 — do not define a
// new interface when an existing one fits). *redisx.RedisResumeCache
// satisfies it structurally.
type accountDeletionCacheInvalidator interface {
	Invalidate(ctx context.Context, userID string) error
}

// accountDeletionAccessBlacklister adds the user to the Story 0.11
// Redis blacklist so the WS upgrade handler (which already calls
// IsBlacklisted) AND the HTTP /v1/* JWT middleware reject subsequent
// requests for the access-token lifetime. Without this step the
// issued access token would remain valid for ≤15 minutes after
// DELETE — a much wider attack surface than the documented <100ms
// race window (review round 1 P1).
//
// *redisx.RedisBlacklist satisfies this via its Add method. TTL is
// the configured access-token expiry: the blacklist entry
// self-expires exactly when the last surviving access token would
// naturally expire, so the blacklist is eventually consistent and
// auditable (NFR-SEC-10 — permanent entries rejected by Add).
type accountDeletionAccessBlacklister interface {
	Add(ctx context.Context, userID string, ttl time.Duration) error
}

// AccountDeletionResult is the service-layer output. The handler uses
// RequestedAt to format the 202 body; WasAlreadyRequested drives only
// the audit log (NEVER the response shape — clients cannot distinguish
// a first call from a repeat, by design: idempotency is a server
// contract, not a client concern).
type AccountDeletionResult struct {
	RequestedAt         time.Time
	WasAlreadyRequested bool
}

// AccountDeletionService orchestrates the five-step deletion request
// flow (mark → revoke refresh → blacklist access → disconnect →
// invalidate) with the fail-closed / fail-open matrix described in
// Story 1.6 AC #13. The order is load-bearing; see
// RequestAccountDeletion for rationale.
type AccountDeletionService struct {
	userRepo         accountDeletionUserRepo
	tokenRev         accountDeletionTokenRevoker
	accessBlacklist  accountDeletionAccessBlacklister
	sessionDis       accountDeletionSessionDisconnector
	cacheInv         accountDeletionCacheInvalidator
	accessTokenTTL   time.Duration
}

// NewAccountDeletionService panics on any nil collaborator (§P3 startup
// fail-fast). A misconfigured DI graph must crash at boot, not at the
// first DELETE /v1/users/me.
//
// accessTokenTTL must be > 0 — it is the TTL the service writes into
// the Redis access blacklist entry. Production wires cfg.JWT.
// AccessExpirySec; tests inject a small duration for readability.
func NewAccountDeletionService(
	userRepo accountDeletionUserRepo,
	tokenRev accountDeletionTokenRevoker,
	accessBlacklist accountDeletionAccessBlacklister,
	sessionDis accountDeletionSessionDisconnector,
	cacheInv accountDeletionCacheInvalidator,
	accessTokenTTL time.Duration,
) *AccountDeletionService {
	if userRepo == nil {
		panic("service.NewAccountDeletionService: userRepo must not be nil")
	}
	if tokenRev == nil {
		panic("service.NewAccountDeletionService: tokenRev must not be nil")
	}
	if accessBlacklist == nil {
		panic("service.NewAccountDeletionService: accessBlacklist must not be nil")
	}
	if sessionDis == nil {
		panic("service.NewAccountDeletionService: sessionDis must not be nil")
	}
	if cacheInv == nil {
		panic("service.NewAccountDeletionService: cacheInv must not be nil")
	}
	if accessTokenTTL <= 0 {
		panic(fmt.Sprintf("service.NewAccountDeletionService: accessTokenTTL must be > 0; got %v", accessTokenTTL))
	}
	return &AccountDeletionService{
		userRepo:        userRepo,
		tokenRev:        tokenRev,
		accessBlacklist: accessBlacklist,
		sessionDis:      sessionDis,
		cacheInv:        cacheInv,
		accessTokenTTL:  accessTokenTTL,
	}
}

// RequestAccountDeletion executes the Story 1.6 account-deletion flow
// in strict order. The order is load-bearing (§21.8 #2):
//
//  1. Mark deletion (fail-closed). Database is the truth source — we
//     cannot revoke / disconnect without an audit record showing the
//     user asked to be deleted.
//
//  2. Revoke all refresh tokens (fail-open). The user's per-device
//     refresh tokens land in the Redis blacklist so any subsequent
//     /auth/refresh is rejected. Failure here is logged as warn and
//     does not abort: the 30-day sweep (process_deletion_queue) will
//     catch any stragglers on the refresh path.
//
//  3. Blacklist the user at the access-token layer (fail-open).
//     Writes `blacklist:device:{userId}` with TTL = access token
//     expiry. Both the HTTP JWT middleware (Story 1.6 round-1 fix)
//     and the WS upgrade handler (Story 0.11) fail-closed on this
//     entry, so the still-valid access token the deleting client
//     holds cannot be used to call any /v1/* endpoint or reopen
//     /ws for the remainder of its ≤15-minute lifetime. Without
//     this step the deletion flow only blocks refresh — the gap
//     the round-1 reviewer flagged as "much broader than the
//     documented sub-100ms race".
//
//  4. Disconnect WS (fail-open). Close every live connection for the
//     user. Current ws.Hub.DisconnectUser never returns an error; the
//     fail-open branch is future-proofing. Runs AFTER Step 3 so a
//     client that races to reopen WS between our disconnect frame
//     and their read hits the blacklist.
//
//  5. Invalidate resume cache (fail-open). The 60s session.resume
//     cache may still hold stale user data for this userId; best-
//     effort Del clears it now, TTL self-heal covers any miss.
//
// Error / fail-matrix:
//
//	Step 1 err=ErrUserNotFound → dto.ErrUserNotFound (404).
//	Step 1 err=other           → dto.ErrInternalError (500).
//	Step 2/3/4/5 err           → warn log, main flow continues.
//
// The idempotent path (firstTime=false) ALSO runs Step 2/3/4/5. That
// is intentional: a re-request might legitimately land on a machine
// that lost its earlier blacklist write (Redis outage during the
// first attempt), so re-running the side effects keeps the system
// converged (§21.8 #3).
func (s *AccountDeletionService) RequestAccountDeletion(ctx context.Context, userID ids.UserID) (*AccountDeletionResult, error) {
	if userID == "" {
		return nil, dto.ErrValidationError.WithCause(errors.New("account deletion: empty userId"))
	}

	// --- Step 1: Mark deletion (fail-closed) ---
	user, firstTime, err := s.userRepo.MarkDeletionRequested(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			logx.Ctx(ctx).Warn().
				Str("action", "account_deletion_user_not_found").
				Str("userId", string(userID)).
				Msg("account_deletion_user_not_found")
			return nil, dto.ErrUserNotFound.WithCause(err)
		}
		logx.Ctx(ctx).Error().Err(err).
			Str("action", "account_deletion_mark_error").
			Str("userId", string(userID)).
			Msg("account_deletion_mark_error")
		return nil, dto.ErrInternalError.WithCause(err)
	}
	if user == nil || user.DeletionRequestedAt == nil {
		// Repo contract says this cannot happen on the success path.
		// Belt-and-braces: refuse to build a response with a zero
		// timestamp because the client would see an empty string.
		return nil, dto.ErrInternalError.WithCause(errors.New("account deletion: repo returned nil user or nil deletion_requested_at"))
	}
	wasAlready := !firstTime

	// --- Step 2: Revoke refresh tokens (fail-open) ---
	if revokeErr := s.tokenRev.RevokeAllUserTokens(ctx, userID); revokeErr != nil {
		logx.Ctx(ctx).Warn().Err(revokeErr).
			Str("action", "account_deletion_revoke_partial").
			Str("userId", string(userID)).
			Msg("account_deletion_revoke_partial")
	}

	// --- Step 3: Blacklist access-token layer (fail-open) ---
	// Runs BEFORE DisconnectUser so a client racing between
	// disconnect-frame and their own reconnect cannot sneak a new /ws
	// upgrade through the window. The upgrade handler calls
	// IsBlacklisted for every upgrade; the HTTP /v1/* middleware does
	// the same after Verify (Story 1.6 round-1 fix).
	if blErr := s.accessBlacklist.Add(ctx, string(userID), s.accessTokenTTL); blErr != nil {
		logx.Ctx(ctx).Warn().Err(blErr).
			Str("action", "account_deletion_access_blacklist_error").
			Str("userId", string(userID)).
			Msg("account_deletion_access_blacklist_error")
	}

	// --- Step 4: Disconnect WS (fail-open) ---
	if _, disErr := s.sessionDis.DisconnectUser(ws.UserID(string(userID))); disErr != nil {
		logx.Ctx(ctx).Warn().Err(disErr).
			Str("action", "account_deletion_disconnect_error").
			Str("userId", string(userID)).
			Msg("account_deletion_disconnect_error")
	}

	// --- Step 5: Invalidate resume cache (fail-open) ---
	if invErr := s.cacheInv.Invalidate(ctx, string(userID)); invErr != nil {
		logx.Ctx(ctx).Warn().Err(invErr).
			Str("action", "account_deletion_resume_invalidate_error").
			Str("userId", string(userID)).
			Msg("account_deletion_resume_invalidate_error")
	}

	// --- Step 5: Audit log (§NFR-SEC-10 compliance) ---
	// Never includes displayName / timezone / sub / hash (§M13 PII).
	logx.Ctx(ctx).Info().
		Str("action", "account_deletion_request").
		Str("userId", string(userID)).
		Time("requestedAt", *user.DeletionRequestedAt).
		Bool("wasAlreadyRequested", wasAlready).
		Msg("account_deletion_request")

	return &AccountDeletionResult{
		RequestedAt:         *user.DeletionRequestedAt,
		WasAlreadyRequested: wasAlready,
	}, nil
}

// Compile-time check that *AuthService satisfies the service's
// consumer-side revoker interface. A break in either side surfaces
// at build time rather than at first DELETE.
var _ accountDeletionTokenRevoker = (*AuthService)(nil)
