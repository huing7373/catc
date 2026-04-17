package dto

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/huing/cat/server/pkg/logx"
)

type ErrCategory string

const (
	CategoryRetryable   ErrCategory = "retryable"
	CategoryClientError ErrCategory = "client_error"
	CategorySilentDrop  ErrCategory = "silent_drop"
	CategoryRetryAfter  ErrCategory = "retry_after"
	CategoryFatal       ErrCategory = "fatal"
)

type AppError struct {
	Code       string
	Message    string
	HTTPStatus int
	Category   ErrCategory
	Cause      error
	RetryAfter int // seconds; if > 0, RespondAppError sets Retry-After header
}

func NewAppError(code string, message string, httpStatus int, category ErrCategory) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
		Category:   category,
	}
}

func (e *AppError) WithCause(err error) *AppError {
	cp := *e
	cp.Cause = err
	return &cp
}

func (e *AppError) WithRetryAfter(seconds int) *AppError {
	cp := *e
	cp.RetryAfter = seconds
	return &cp
}

func (e *AppError) Error() string {
	return e.Code + ": " + e.Message
}

func (e *AppError) Unwrap() error {
	return e.Cause
}

func (e *AppError) Is(target error) bool {
	var t *AppError
	if errors.As(target, &t) {
		return e.Code == t.Code
	}
	return false
}

func RespondAppError(c *gin.Context, err error) {
	ctx := c.Request.Context()
	var ae *AppError
	if errors.As(err, &ae) {
		logx.Ctx(ctx).Error().Err(ae.Cause).Str("code", ae.Code).Msg("app error")
		if ae.RetryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(ae.RetryAfter))
		}
		c.JSON(ae.HTTPStatus, gin.H{"error": gin.H{"code": ae.Code, "message": ae.Message}})
		return
	}
	logx.Ctx(ctx).Error().Err(err).Msg("unhandled error")
	c.JSON(500, gin.H{"error": gin.H{"code": "INTERNAL_ERROR", "message": "internal server error"}})
}
