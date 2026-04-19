package service

import (
	"context"
	"fmt"
	"time"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
	"github.com/huing/cat/server/pkg/logx"
)

// ApnsTokenHandlerService is the contract DeviceHandler consumes.
// Declared here (consumer-side P2 §6.2) so the handler package does not
// import the concrete *ApnsTokenService; tests substitute a fake via
// the same interface.
type ApnsTokenHandlerService interface {
	RegisterApnsToken(ctx context.Context, req RegisterApnsTokenRequest) error
}

// RegisterApnsTokenRequest is the service-layer shape. Platform is the
// authenticated value pulled from the JWT claim by DeviceHandler —
// the service does not trust body.platform (see AC5 / AC11 rationale).
type RegisterApnsTokenRequest struct {
	UserID      ids.UserID
	DeviceID    string
	Platform    ids.Platform
	DeviceToken string
}

// apnsTokenRepo is the minimal repo surface RegisterApnsToken needs.
// Declared at service scope (not in internal/repository) so the
// dependency arrow points service → domain only.
type apnsTokenRepo interface {
	Upsert(ctx context.Context, t *domain.ApnsToken) error
}

// userSessionRepo is the minimal surface for toggling
// users.sessions[deviceId].has_apns_token after a successful register.
// Declared here (not embedded in the AuthService UserRepository
// interface) so Story 1.4 does not force a ripple signature change on
// Story 1.2's refresh code path.
type userSessionRepo interface {
	SetSessionHasApnsToken(ctx context.Context, userID ids.UserID, deviceID string, has bool) error
}

// apnsTokenRegisterRateLimiter throttles per-user registration
// attempts. Sliding window by design (review-antipatterns §9.1) — a
// fixed-window implementation would let a misbehaving client burst 2×
// the quota across the window boundary. Real impl is
// redisx.UserSlidingWindowLimiter (AC6).
type apnsTokenRegisterRateLimiter interface {
	Acquire(ctx context.Context, userID ids.UserID) (allowed bool, retryAfter time.Duration, err error)
}

// ApnsTokenService implements the RegisterApnsToken flow for Story 1.4.
// Four steps (fail-closed at each except the session-flag write, which
// is best-effort — see AC11):
//
//  1. Rate-limit the (userID) → stop before any DB write on burst.
//  2. Persist the (userID, platform, token, now) row via the repo
//     (repo seals device_token on write — plaintext never lands in
//     Mongo).
//  3. Mark users.sessions.<deviceId>.has_apns_token=true. Best-effort:
//     any write error is LOGGED and swallowed because the flag is a
//     /v1/me convenience indicator, not a push-routing invariant.
//  4. Audit-log the registration (masked token NEVER emitted — the
//     service only carries userId + deviceId + platform + action).
type ApnsTokenService struct {
	repo        apnsTokenRepo
	sessionRepo userSessionRepo
	limiter     apnsTokenRegisterRateLimiter
	clock       clockx.Clock
}

// NewApnsTokenService fail-fast-validates its collaborators so a
// mis-wired DI graph crashes at startup rather than on first request.
func NewApnsTokenService(
	repo apnsTokenRepo,
	sessionRepo userSessionRepo,
	limiter apnsTokenRegisterRateLimiter,
	clk clockx.Clock,
) *ApnsTokenService {
	if repo == nil {
		panic("service.NewApnsTokenService: repo must not be nil")
	}
	if sessionRepo == nil {
		panic("service.NewApnsTokenService: sessionRepo must not be nil")
	}
	if limiter == nil {
		panic("service.NewApnsTokenService: limiter must not be nil")
	}
	if clk == nil {
		panic("service.NewApnsTokenService: clock must not be nil")
	}
	return &ApnsTokenService{repo: repo, sessionRepo: sessionRepo, limiter: limiter, clock: clk}
}

// RegisterApnsToken persists the (userID, platform) → token binding and
// marks the corresponding session. See type docs for failure-mode
// decisions.
func (s *ApnsTokenService) RegisterApnsToken(ctx context.Context, req RegisterApnsTokenRequest) error {
	allowed, retry, err := s.limiter.Acquire(ctx, req.UserID)
	if err != nil {
		return fmt.Errorf("apns token service: rate limit acquire: %w", err)
	}
	if !allowed {
		return dto.ErrRateLimitExceeded.
			WithRetryAfter(ceilRateLimitSeconds(retry)).
			WithCause(fmt.Errorf("apns_token register rate: retry_after=%s", retry))
	}

	t := &domain.ApnsToken{
		UserID:      req.UserID,
		Platform:    req.Platform,
		DeviceToken: req.DeviceToken,
		UpdatedAt:   s.clock.Now(),
	}
	if err := s.repo.Upsert(ctx, t); err != nil {
		return fmt.Errorf("apns token service: upsert: %w", err)
	}

	// Best-effort — see AC11 rationale. A session-flag write failure is
	// a convenience-field regression, not a push-delivery regression,
	// so main request stays 200.
	if err := s.sessionRepo.SetSessionHasApnsToken(ctx, req.UserID, req.DeviceID, true); err != nil {
		logx.Ctx(ctx).Warn().Err(err).
			Str("userId", string(req.UserID)).
			Str("deviceId", req.DeviceID).
			Str("action", "apns_token_session_flag_write_failed").
			Msg("apns_token_session_flag_write_failed")
	}

	logx.Ctx(ctx).Info().
		Str("userId", string(req.UserID)).
		Str("deviceId", req.DeviceID).
		Str("platform", string(req.Platform)).
		Str("action", "apns_token_register").
		Msg("apns_token_registered")
	return nil
}

// Compile-time check: a concrete ApnsTokenService must satisfy the
// consumer-side handler interface. If a future field change breaks
// that, the build fails here rather than at wire time.
var _ ApnsTokenHandlerService = (*ApnsTokenService)(nil)

// ceilRateLimitSeconds converts the limiter's sub-second retry hint
// into whole seconds for the HTTP Retry-After header. A zero / negative
// input is clamped to 1s — "0" in Retry-After reads as "try immediately"
// which would defeat the purpose when the limiter already decided the
// attempt is blocked. Mirrors ws/upgrade_handler.ceilSeconds; kept
// local here because the WS helper is unexported and this is the
// second of what will become several per-endpoint limiters (Story 1.5
// profile, Story 5.3 touch). Future refactor: hoist into pkg/redisx or
// an internal helper once the third caller lands.
func ceilRateLimitSeconds(d time.Duration) int {
	secs := int((d + time.Second - 1) / time.Second)
	if secs < 1 {
		return 1
	}
	return secs
}
