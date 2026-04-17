package dto

import (
	"log"
	"net/http"
)

var (
	ErrAuthInvalidIdentityToken = NewAppError("AUTH_INVALID_IDENTITY_TOKEN", "invalid identity token", http.StatusUnauthorized, CategoryFatal)
	ErrAuthTokenExpired         = NewAppError("AUTH_TOKEN_EXPIRED", "token expired", http.StatusUnauthorized, CategoryFatal)
	ErrAuthRefreshTokenRevoked  = NewAppError("AUTH_REFRESH_TOKEN_REVOKED", "refresh token revoked", http.StatusUnauthorized, CategoryFatal)
	ErrFriendAlreadyExists      = NewAppError("FRIEND_ALREADY_EXISTS", "friend already exists", http.StatusConflict, CategoryClientError)
	ErrFriendLimitReached       = NewAppError("FRIEND_LIMIT_REACHED", "friend limit reached", http.StatusUnprocessableEntity, CategoryClientError)
	ErrFriendInviteExpired      = NewAppError("FRIEND_INVITE_EXPIRED", "friend invite expired", http.StatusGone, CategoryClientError)
	ErrFriendInviteUsed         = NewAppError("FRIEND_INVITE_USED", "friend invite already used", http.StatusConflict, CategoryClientError)
	ErrFriendBlocked            = NewAppError("FRIEND_BLOCKED", "user is blocked", http.StatusForbidden, CategoryClientError)
	ErrFriendNotFound           = NewAppError("FRIEND_NOT_FOUND", "friend not found", http.StatusNotFound, CategoryClientError)
	ErrBlindboxAlreadyRedeemed  = NewAppError("BLINDBOX_ALREADY_REDEEMED", "blindbox already redeemed", http.StatusConflict, CategoryClientError)
	ErrBlindboxInsufficientSteps = NewAppError("BLINDBOX_INSUFFICIENT_STEPS", "insufficient steps", http.StatusUnprocessableEntity, CategoryClientError)
	ErrBlindboxNotFound         = NewAppError("BLINDBOX_NOT_FOUND", "blindbox not found", http.StatusNotFound, CategoryClientError)
	ErrSkinNotOwned             = NewAppError("SKIN_NOT_OWNED", "skin not owned", http.StatusForbidden, CategoryClientError)
	ErrRateLimitExceeded        = NewAppError("RATE_LIMIT_EXCEEDED", "rate limit exceeded", http.StatusTooManyRequests, CategoryRetryAfter)
	ErrDeviceBlacklisted        = NewAppError("DEVICE_BLACKLISTED", "device blacklisted", http.StatusForbidden, CategoryFatal)
	ErrInternalError            = NewAppError("INTERNAL_ERROR", "internal server error", http.StatusInternalServerError, CategoryRetryable)
	ErrValidationError          = NewAppError("VALIDATION_ERROR", "validation error", http.StatusBadRequest, CategoryClientError)
	ErrUnknownMessageType       = NewAppError("UNKNOWN_MESSAGE_TYPE", "unknown message type", http.StatusBadRequest, CategoryClientError)
	ErrRoomFull                 = NewAppError("ROOM_FULL", "room is full", http.StatusConflict, CategoryClientError)
)

var allCodes = []*AppError{
	ErrAuthInvalidIdentityToken,
	ErrAuthTokenExpired,
	ErrAuthRefreshTokenRevoked,
	ErrFriendAlreadyExists,
	ErrFriendLimitReached,
	ErrFriendInviteExpired,
	ErrFriendInviteUsed,
	ErrFriendBlocked,
	ErrFriendNotFound,
	ErrBlindboxAlreadyRedeemed,
	ErrBlindboxInsufficientSteps,
	ErrBlindboxNotFound,
	ErrSkinNotOwned,
	ErrRateLimitExceeded,
	ErrDeviceBlacklisted,
	ErrInternalError,
	ErrValidationError,
	ErrUnknownMessageType,
	ErrRoomFull,
}

var registry = map[string]ErrCategory{}

func init() {
	for _, code := range allCodes {
		if code.Category == "" {
			log.Fatalf("error code %q missing category", code.Code)
		}
		if _, exists := registry[code.Code]; exists {
			log.Fatalf("duplicate error code %q", code.Code)
		}
		registry[code.Code] = code.Category
	}
}

func RegisteredCodes() map[string]ErrCategory {
	cp := make(map[string]ErrCategory, len(registry))
	for k, v := range registry {
		cp[k] = v
	}
	return cp
}
