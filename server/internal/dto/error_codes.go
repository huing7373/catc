package dto

import (
	"log"
	"net/http"
)

var registry = map[string]*AppError{}

func register(code string, message string, httpStatus int, category ErrCategory) *AppError {
	if category == "" {
		log.Fatalf("error code %q missing category", code)
	}
	if _, exists := registry[code]; exists {
		log.Fatalf("duplicate error code %q", code)
	}
	ae := NewAppError(code, message, httpStatus, category)
	registry[code] = ae
	return ae
}

var (
	ErrAuthInvalidIdentityToken  = register("AUTH_INVALID_IDENTITY_TOKEN", "invalid identity token", http.StatusUnauthorized, CategoryFatal)
	ErrAuthTokenExpired          = register("AUTH_TOKEN_EXPIRED", "token expired", http.StatusUnauthorized, CategoryFatal)
	ErrAuthRefreshTokenRevoked   = register("AUTH_REFRESH_TOKEN_REVOKED", "refresh token revoked", http.StatusUnauthorized, CategoryFatal)
	ErrFriendAlreadyExists       = register("FRIEND_ALREADY_EXISTS", "friend already exists", http.StatusConflict, CategoryClientError)
	ErrFriendLimitReached        = register("FRIEND_LIMIT_REACHED", "friend limit reached", http.StatusUnprocessableEntity, CategoryClientError)
	ErrFriendInviteExpired       = register("FRIEND_INVITE_EXPIRED", "friend invite expired", http.StatusGone, CategoryClientError)
	ErrFriendInviteUsed          = register("FRIEND_INVITE_USED", "friend invite already used", http.StatusConflict, CategoryClientError)
	ErrFriendBlocked             = register("FRIEND_BLOCKED", "user is blocked", http.StatusForbidden, CategoryClientError)
	ErrFriendNotFound            = register("FRIEND_NOT_FOUND", "friend not found", http.StatusNotFound, CategoryClientError)
	ErrBlindboxAlreadyRedeemed   = register("BLINDBOX_ALREADY_REDEEMED", "blindbox already redeemed", http.StatusConflict, CategoryClientError)
	ErrBlindboxInsufficientSteps = register("BLINDBOX_INSUFFICIENT_STEPS", "insufficient steps", http.StatusUnprocessableEntity, CategoryClientError)
	ErrBlindboxNotFound          = register("BLINDBOX_NOT_FOUND", "blindbox not found", http.StatusNotFound, CategoryClientError)
	ErrSkinNotOwned              = register("SKIN_NOT_OWNED", "skin not owned", http.StatusForbidden, CategoryClientError)
	ErrRateLimitExceeded         = register("RATE_LIMIT_EXCEEDED", "rate limit exceeded", http.StatusTooManyRequests, CategoryRetryAfter)
	ErrDeviceBlacklisted         = register("DEVICE_BLACKLISTED", "device blacklisted", http.StatusForbidden, CategoryFatal)
	ErrInternalError             = register("INTERNAL_ERROR", "internal server error", http.StatusInternalServerError, CategoryRetryable)
	ErrValidationError           = register("VALIDATION_ERROR", "validation error", http.StatusBadRequest, CategoryClientError)
	ErrUnknownMessageType        = register("UNKNOWN_MESSAGE_TYPE", "unknown message type", http.StatusBadRequest, CategoryClientError)
	ErrRoomFull                  = register("ROOM_FULL", "room is full", http.StatusConflict, CategoryClientError)
)

func RegisteredCodes() map[string]*AppError {
	cp := make(map[string]*AppError, len(registry))
	for k, v := range registry {
		cp[k] = v
	}
	return cp
}
