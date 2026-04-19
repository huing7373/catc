package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/service"
	"github.com/huing/cat/server/pkg/ids"
)

// AuthSignInService is the consumer-side surface AuthHandler depends on.
// Defined here (not in internal/service) so unit tests can inject a fake
// without crossing the service.UserRepository / VerifyApple / Issue web
// the production wiring stitches together.
type AuthSignInService interface {
	SignInWithApple(ctx context.Context, req service.SignInWithAppleRequest) (*service.SignInWithAppleResult, error)
}

// AuthHandler is the Gin handler for /auth/* unauthenticated bootstrap
// endpoints. Story 1.1 ships POST /auth/apple; Story 1.2 will add
// POST /auth/refresh on the same handler.
type AuthHandler struct {
	svc AuthSignInService
}

func NewAuthHandler(svc AuthSignInService) *AuthHandler {
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
