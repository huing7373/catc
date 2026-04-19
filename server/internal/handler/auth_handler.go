package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/service"
	"github.com/huing/cat/server/pkg/ids"
)

// AuthHandlerService aggregates the service-layer methods AuthHandler
// depends on. Renamed from AuthSignInService (Story 1.1) to reflect
// the expanding surface — Story 1.2 adds RefreshToken; Story 1.6 will
// add RequestDeletion.
type AuthHandlerService interface {
	SignInWithApple(ctx context.Context, req service.SignInWithAppleRequest) (*service.SignInWithAppleResult, error)
	RefreshToken(ctx context.Context, req service.RefreshTokenRequest) (*service.RefreshTokenResult, error)
}

// AuthHandler is the Gin handler for /auth/* unauthenticated bootstrap
// endpoints. Story 1.1 shipped POST /auth/apple; Story 1.2 adds
// POST /auth/refresh.
type AuthHandler struct {
	svc AuthHandlerService
}

func NewAuthHandler(svc AuthHandlerService) *AuthHandler {
	if svc == nil {
		panic("handler.NewAuthHandler: svc must not be nil")
	}
	return &AuthHandler{svc: svc}
}

// SignInWithApple binds the request body, runs the SignInWithApple
// service flow, and writes the success / error response. The endpoint
// is bootstrap-only — it is mounted OUTSIDE the future /v1/* JWT
// middleware group (architecture §13 / §Architectural Boundaries).
func (h *AuthHandler) SignInWithApple(c *gin.Context) {
	var req dto.SignInWithAppleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.RespondAppError(c, dto.ErrValidationError.WithCause(err))
		return
	}
	result, err := h.svc.SignInWithApple(c.Request.Context(), service.SignInWithAppleRequest{
		IdentityToken:     req.IdentityToken,
		AuthorizationCode: req.AuthorizationCode,
		DeviceID:          req.DeviceID,
		Platform:          ids.Platform(req.Platform),
		Nonce:             req.Nonce,
	})
	if err != nil {
		dto.RespondAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.SignInWithAppleResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		User:         dto.UserPublicFromDomain(result.User),
	})
}

// Refresh handles POST /auth/refresh — bootstrap unauthenticated
// endpoint (outside the future /v1/* JWT group). The refresh token in
// the body IS the caller's credential; the handler does not check any
// other auth. The service layer owns verification + rotation +
// blacklist (Story 1.2 AuthService.RefreshToken).
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req dto.RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.RespondAppError(c, dto.ErrValidationError.WithCause(err))
		return
	}
	result, err := h.svc.RefreshToken(c.Request.Context(), service.RefreshTokenRequest{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		dto.RespondAppError(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.RefreshTokenResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
	})
}
