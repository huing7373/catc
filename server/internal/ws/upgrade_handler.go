package ws

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/middleware"
	"github.com/huing/cat/server/pkg/logx"
)

// AuthenticatedIdentity is what UpgradeHandler propagates into ws.Client
// for all downstream WS handlers. Fields map 1:1 to jwtx.CustomClaims so
// the production JWTValidator wiring is a thin unwrap; the debug wiring
// (DebugValidator) synthesizes them for local devtools.
//
// All three fields are populated for happy-path requests; reject paths
// return the zero value alongside a non-nil error. Callers MUST treat
// the zero value as a programmer error (the validator already returned
// err) — not as "anonymous user".
type AuthenticatedIdentity struct {
	UserID   UserID
	DeviceID string
	Platform string
}

type TokenValidator interface {
	ValidateToken(token string) (AuthenticatedIdentity, error)
}

type debugValidator struct{}

// ValidateToken (debug) treats the entire bearer string as the userId
// and synthesizes a deviceId / platform so downstream handlers (Story
// 10.1 room.join, Story 2.x state.tick when added) can rely on a non-
// empty (deviceId, platform) pair without each having to special-case
// the debug branch. The synthetic deviceId is derived from the token
// so two debug clients with distinct bearers get distinct deviceIds —
// useful for room-join tests with two browser tabs. Platform defaults
// to "iphone" because the integration MVP (Story 10.1) is iPhone-side.
func (debugValidator) ValidateToken(token string) (AuthenticatedIdentity, error) {
	if token == "" {
		return AuthenticatedIdentity{}, errors.New("empty token")
	}
	return AuthenticatedIdentity{
		UserID:   token,
		DeviceID: "debug-device-" + token,
		Platform: "iphone",
	}, nil
}

type stubValidator struct{}

func (stubValidator) ValidateToken(_ string) (AuthenticatedIdentity, error) {
	return AuthenticatedIdentity{}, dto.ErrAuthInvalidIdentityToken
}

func NewDebugValidator() TokenValidator { return debugValidator{} }
func NewStubValidator() TokenValidator  { return stubValidator{} }

type UpgradeHandler struct {
	hub         *Hub
	dispatcher  *Dispatcher
	validator   TokenValidator
	blacklist   Blacklist
	rateLimiter ConnectRateLimiter
	upgrader    websocket.Upgrader
}

// NewUpgradeHandler builds the WS upgrade gin handler. Passing nil for
// blacklist or rateLimiter disables that particular guard — useful for
// local debug and unit tests that don't need Redis. Production assembly
// (cmd/cat/initialize.go) MUST pass non-nil instances; fail-closed semantics
// below depend on them.
func NewUpgradeHandler(
	hub *Hub,
	dispatcher *Dispatcher,
	validator TokenValidator,
	blacklist Blacklist,
	rateLimiter ConnectRateLimiter,
) *UpgradeHandler {
	return &UpgradeHandler{
		hub:         hub,
		dispatcher:  dispatcher,
		validator:   validator,
		blacklist:   blacklist,
		rateLimiter: rateLimiter,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

func (h *UpgradeHandler) Handle(c *gin.Context) {
	ctx := c.Request.Context()

	token := middleware.ExtractBearerToken(c.GetHeader("Authorization"))
	if token == "" {
		dto.RespondAppError(c, dto.ErrAuthInvalidIdentityToken)
		return
	}

	identity, err := h.validator.ValidateToken(token)
	if err != nil {
		dto.RespondAppError(c, err)
		return
	}
	userID := identity.UserID

	// Guard order: blacklist first (fatal — client must clear token), then
	// rate limit (retry_after — client backs off). Both checks fail closed
	// on Redis error so a Redis outage cannot re-enable the J4 WS-storm
	// scenario by silently disabling the limiter.
	if h.blacklist != nil {
		blocked, err := h.blacklist.IsBlacklisted(ctx, userID)
		if err != nil {
			logx.Ctx(ctx).Error().Err(err).
				Str("action", "ws_connect_guard_error").
				Str("userId", userID).
				Str("stage", "blacklist").
				Msg("blacklist check failed")
			dto.RespondAppError(c, dto.ErrInternalError.WithCause(err))
			return
		}
		if blocked {
			logx.Ctx(ctx).Info().
				Str("action", "ws_connect_reject").
				Str("userId", userID).
				Str("reason", "blacklist").
				Msg("ws_connect_reject")
			dto.RespondAppError(c, dto.ErrDeviceBlacklisted)
			return
		}
	}

	if h.rateLimiter != nil {
		decision, err := h.rateLimiter.AcquireConnectSlot(ctx, userID)
		if err != nil {
			logx.Ctx(ctx).Error().Err(err).
				Str("action", "ws_connect_guard_error").
				Str("userId", userID).
				Str("stage", "ratelimit").
				Msg("rate limit check failed")
			dto.RespondAppError(c, dto.ErrInternalError.WithCause(err))
			return
		}
		if !decision.Allowed {
			retrySec := ceilSeconds(decision.RetryAfter)
			logx.Ctx(ctx).Info().
				Str("action", "ws_connect_reject").
				Str("userId", userID).
				Str("reason", "ratelimit").
				Int64("count", decision.Count).
				Int64("retryAfterSec", int64(retrySec)).
				Msg("ws_connect_reject")
			dto.RespondAppError(c, dto.ErrRateLimitExceeded.WithRetryAfter(retrySec))
			return
		}
	}

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Error().Err(err).Msg("ws upgrade failed")
		return
	}

	client := &Client{
		connID:     uuid.New().String(),
		userID:     identity.UserID,
		deviceID:   identity.DeviceID,
		platform:   identity.Platform,
		conn:       conn,
		send:       make(chan []byte, h.hub.cfg.SendBufSize),
		done:       make(chan struct{}),
		hub:        h.hub,
		dispatcher: h.dispatcher,
	}

	h.hub.Register(client)
	go h.hub.writePump(client)
	go h.hub.readPump(client)
}

// ceilSeconds rounds up a duration to whole seconds, with a minimum of 1.
// RetryAfter of 0 is misleading ("try immediately") — when the limiter says
// "blocked" the client should wait at least a beat.
func ceilSeconds(d time.Duration) int {
	secs := int((d + time.Second - 1) / time.Second)
	if secs < 1 {
		return 1
	}
	return secs
}

