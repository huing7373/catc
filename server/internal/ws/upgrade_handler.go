package ws

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/logx"
)

type TokenValidator interface {
	ValidateToken(token string) (userID string, err error)
}

type debugValidator struct{}

func (debugValidator) ValidateToken(token string) (string, error) {
	if token == "" {
		return "", errors.New("empty token")
	}
	return token, nil
}

type stubValidator struct{}

func (stubValidator) ValidateToken(_ string) (string, error) {
	return "", dto.ErrAuthInvalidIdentityToken
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

	token := extractBearerToken(c.GetHeader("Authorization"))
	if token == "" {
		dto.RespondAppError(c, dto.ErrAuthInvalidIdentityToken)
		return
	}

	userID, err := h.validator.ValidateToken(token)
	if err != nil {
		dto.RespondAppError(c, err)
		return
	}

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
		userID:     userID,
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

func extractBearerToken(header string) string {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return ""
}
