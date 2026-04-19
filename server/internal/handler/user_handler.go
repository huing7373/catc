package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/middleware"
	"github.com/huing/cat/server/internal/service"
	"github.com/huing/cat/server/pkg/ids"
)

// UserHandlerService is the consumer-side contract UserHandler depends
// on. Declared in the handler package (P2 §6.2) so the handler does not
// import *service.AccountDeletionService directly; tests substitute a
// fake via the same interface.
//
// Story 1.6 ships the only method on this interface today
// (RequestAccountDeletion). Future Story 1.x profile-read endpoints,
// if they live under /v1/users/me, extend this interface rather than
// splitting into a new handler.
type UserHandlerService interface {
	RequestAccountDeletion(ctx context.Context, userID ids.UserID) (*service.AccountDeletionResult, error)
}

// UserHandler wraps authenticated endpoints under /v1/users/me. Story
// 1.6 adds DELETE /v1/users/me (request account deletion); other
// profile-level endpoints can join here as Epic 1 and later stories
// extend.
type UserHandler struct {
	svc UserHandlerService
}

// NewUserHandler panics on nil svc — a mis-wired DI graph must crash
// at startup, not at the first DELETE.
func NewUserHandler(svc UserHandlerService) *UserHandler {
	if svc == nil {
		panic("handler.NewUserHandler: svc must not be nil")
	}
	return &UserHandler{svc: svc}
}

// RequestDeletion handles DELETE /v1/users/me.
//
// Authentication: the endpoint lives inside the /v1/* group so
// middleware.JWTAuth runs first in release mode. In debug mode
// jwtAuth is nil; this handler's UserIDFrom-empty-401 check
// (defense-in-depth, matches device_handler.go) keeps unauthenticated
// traffic out even with no middleware.
//
// Body: deliberately NOT parsed. DELETE has no body in the client
// contract (see client guide §16). Gin allows it but we skip
// ShouldBindJSON so a malicious client cannot smuggle unexpected
// fields through a permissive decoder.
//
// Response: always 202 AccountDeletionResponse on success (first
// call OR idempotent repeat). RequestedAt serializes as UTC RFC3339
// — NOT the server's local zone — so iOS / watchOS clients get a
// byte-stable string regardless of which region the server runs in
// (§21.8 #4).
func (h *UserHandler) RequestDeletion(c *gin.Context) {
	userID := middleware.UserIDFrom(c)
	if userID == "" {
		// Defense in depth: release mode mounts JWTAuth before this
		// handler, so UserIDFrom can only be empty when the middleware
		// did not run (misconfigured DI graph, or debug mode without
		// its own bearer verifier). Match device_handler.go by
		// returning 401 AUTH_INVALID_IDENTITY_TOKEN — the client then
		// clears its Keychain and forces a fresh SIWA, which is the
		// safest action when middleware wiring is untrustworthy.
		dto.RespondAppError(c, dto.ErrAuthInvalidIdentityToken.WithCause(
			errors.New("user handler: middleware did not inject userId — check wiring"),
		))
		return
	}

	result, err := h.svc.RequestAccountDeletion(c.Request.Context(), userID)
	if err != nil {
		dto.RespondAppError(c, err)
		return
	}

	c.JSON(http.StatusAccepted, dto.AccountDeletionResponse{
		Status:      dto.AccountDeletionStatusRequested,
		RequestedAt: result.RequestedAt.UTC().Format(time.RFC3339),
		Note:        dto.AccountDeletionNoteMVP,
	})
}
