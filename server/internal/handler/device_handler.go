package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/middleware"
	"github.com/huing/cat/server/internal/service"
	"github.com/huing/cat/server/pkg/ids"
)

// DeviceHandlerService is the subset of service methods DeviceHandler
// consumes. Declared here (P2 §6.2 consumer-side) so the handler package
// does not import *ApnsTokenService directly; tests substitute a fake
// via the same interface.
type DeviceHandlerService interface {
	RegisterApnsToken(ctx context.Context, req service.RegisterApnsTokenRequest) error
}

// DeviceHandler is the Gin handler for authenticated device-management
// endpoints. Story 1.4 ships the first one (POST /v1/devices/apns-token);
// future stories (1.5 preferences, 1.6 deletion) will append sibling
// methods as they land.
type DeviceHandler struct {
	svc DeviceHandlerService
}

func NewDeviceHandler(svc DeviceHandlerService) *DeviceHandler {
	if svc == nil {
		panic("handler.NewDeviceHandler: svc must not be nil")
	}
	return &DeviceHandler{svc: svc}
}

// RegisterApnsToken handles POST /v1/devices/apns-token. Mounted inside
// the /v1/* group wired by Story 1.3 — JWTAuth runs before this handler,
// so middleware.UserIDFrom / DeviceIDFrom / PlatformFrom are populated
// by the time we read them.
//
// Platform resolution (AC5 / §21.8 #6):
//   - JWT is the source of truth.
//   - body.platform is OPTIONAL; when present it MUST match the JWT
//     (defense in depth).
//   - JWT missing platform → 401 AUTH_INVALID_IDENTITY_TOKEN. There is
//     NO fallback to body.platform, because Story 1.1/1.2 tokens ALL
//     carry the claim in production. Accepting body.platform when the
//     JWT is empty opens a phishing vector (attacker with a stolen
//     Watch token registers an iPhone APNs token against the victim).
func (h *DeviceHandler) RegisterApnsToken(c *gin.Context) {
	var req dto.RegisterApnsTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.RespondAppError(c, dto.ErrValidationError.WithCause(err))
		return
	}

	userID := middleware.UserIDFrom(c)
	deviceID := middleware.DeviceIDFrom(c)
	jwtPlatform := middleware.PlatformFrom(c)

	// Defense-in-depth — middleware already rejects empty userId /
	// deviceId, but keep the guard so a misconfigured DI graph that
	// somehow bypasses the middleware still fails closed.
	if userID == "" || deviceID == "" {
		dto.RespondAppError(c, dto.ErrAuthInvalidIdentityToken.WithCause(
			errors.New("device handler: middleware did not inject userId/deviceId — check wiring"),
		))
		return
	}

	var platform ids.Platform
	if jwtPlatform == "" {
		dto.RespondAppError(c, dto.ErrAuthInvalidIdentityToken.WithCause(
			errors.New("device handler: jwt missing platform claim — client must refresh to get a 1.2+ access token"),
		))
		return
	}
	platform = jwtPlatform
	if req.Platform != "" && req.Platform != string(jwtPlatform) {
		dto.RespondAppError(c, dto.ErrValidationError.WithCause(fmt.Errorf(
			"device handler: body.platform=%q does not match jwt.platform=%q",
			req.Platform, jwtPlatform,
		)))
		return
	}

	if err := h.svc.RegisterApnsToken(c.Request.Context(), service.RegisterApnsTokenRequest{
		UserID:      userID,
		DeviceID:    deviceID,
		Platform:    platform,
		DeviceToken: req.DeviceToken,
	}); err != nil {
		dto.RespondAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.RegisterApnsTokenResponse{Ok: true})
}
