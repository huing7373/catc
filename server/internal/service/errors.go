// Package service defines application services: business rules,
// cross-repository coordination, transaction boundaries, and external
// side effects. Every error crossing the handler boundary is a
// *dto.AppError.
package service

import "github.com/huing7373/catc/server/internal/dto"

// Common AppError sentinels shared across services. Specific services
// may define additional sentinels next to their source files.
var (
	ErrUserNotFound = &dto.AppError{HTTPStatus: 404, Code: "USER_NOT_FOUND", Message: "user not found"}
	ErrUnauthorized = &dto.AppError{HTTPStatus: 401, Code: "UNAUTHORIZED", Message: "unauthorized"}
	ErrRateLimited  = &dto.AppError{HTTPStatus: 429, Code: "RATE_LIMITED", Message: "too many requests"}
	ErrForbidden    = &dto.AppError{HTTPStatus: 403, Code: "FORBIDDEN", Message: "forbidden"}
	ErrAppleAuthFail = &dto.AppError{HTTPStatus: 401, Code: "APPLE_AUTH_FAIL", Message: "apple sign-in failed"}
	ErrNonceMismatch = &dto.AppError{HTTPStatus: 401, Code: "NONCE_MISMATCH", Message: "nonce mismatch"}
	ErrTokenExpired  = &dto.AppError{HTTPStatus: 401, Code: "AUTH_EXPIRED", Message: "session expired"}
	ErrTokenInvalid  = &dto.AppError{HTTPStatus: 401, Code: "AUTH_INVALID", Message: "session invalid"}

	ErrNicknameInvalid = &dto.AppError{HTTPStatus: 400, Code: "NICKNAME_INVALID", Message: "display name is invalid"}
	ErrNicknameSame    = &dto.AppError{HTTPStatus: 400, Code: "NICKNAME_SAME", Message: "display name unchanged"}
)
