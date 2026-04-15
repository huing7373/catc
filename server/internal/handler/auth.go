package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/huing7373/catc/server/internal/dto"
	"github.com/huing7373/catc/server/internal/service"
)

// AuthSvc is the service-layer surface this handler depends on. The
// interface lives here (consumer side) so the handler can be unit-tested
// without dragging in the real service implementation.
type AuthSvc interface {
	Login(ctx context.Context, in service.LoginInput) (service.TokenPair, error)
	Refresh(ctx context.Context, in service.RefreshInput) (service.TokenPair, error)
}

// AuthHandler exposes POST /v1/auth/login and /v1/auth/refresh.
type AuthHandler struct {
	svc AuthSvc
}

// NewAuthHandler wires the handler to an AuthSvc.
func NewAuthHandler(svc AuthSvc) *AuthHandler { return &AuthHandler{svc: svc} }

// Login handles POST /v1/auth/login.
func (h *AuthHandler) Login(c *gin.Context) {
	var req dto.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.RespondAppError(c, dto.NewValidationError(err))
		return
	}
	pair, err := h.svc.Login(c.Request.Context(), service.LoginInput{
		AppleJWT: req.AppleJWT,
		Nonce:    req.Nonce,
		DeviceID: req.DeviceID,
	})
	if err != nil {
		dto.RespondAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.LoginResp{
		UserID:           string(pair.UserID),
		AccessToken:      pair.AccessToken,
		RefreshToken:     pair.RefreshToken,
		AccessExpiresAt:  pair.AccessExpiresAt.UTC(),
		RefreshExpiresAt: pair.RefreshExpiresAt.UTC(),
		LoginOutcome:     string(pair.LoginOutcome),
	})
}

// Refresh handles POST /v1/auth/refresh.
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req dto.RefreshReq
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.RespondAppError(c, dto.NewValidationError(err))
		return
	}
	pair, err := h.svc.Refresh(c.Request.Context(), service.RefreshInput{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		dto.RespondAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.RefreshResp{
		UserID:           string(pair.UserID),
		AccessToken:      pair.AccessToken,
		RefreshToken:     pair.RefreshToken,
		AccessExpiresAt:  pair.AccessExpiresAt.UTC(),
		RefreshExpiresAt: pair.RefreshExpiresAt.UTC(),
	})
}
