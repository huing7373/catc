// Package dto holds request/response structures shared across handler
// and middleware layers, plus the AppError + RespondXxx helpers.
package dto

// AppError is the canonical transport error produced by the service
// layer. Its Code is a stable machine-readable token exposed to
// clients; Message is a short human-readable summary. Wrapped carries
// the original cause for logs but is NEVER emitted to the client.
type AppError struct {
	HTTPStatus int    // HTTP status to emit
	Code       string // stable token, e.g. "USER_NOT_FOUND"
	Message    string // short human summary
	Wrapped    error  // internal cause, logged only
}

// Error implements the error interface. The format is "<code>: <message>".
func (e *AppError) Error() string {
	if e == nil {
		return ""
	}
	return e.Code + ": " + e.Message
}

// Unwrap exposes the wrapped cause for errors.Is / errors.As traversal.
func (e *AppError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Wrapped
}

// WithCause returns a shallow copy of e with Wrapped set to cause. The
// HTTPStatus / Code / Message remain the stable contract; only the
// internal cause changes. Callers use it at the failure site:
//
//	return nil, ErrUserNotFound.WithCause(err)
func (e *AppError) WithCause(cause error) *AppError {
	if e == nil {
		return nil
	}
	clone := *e
	clone.Wrapped = cause
	return &clone
}

// ErrValidation is the canonical 400 emitted when request binding /
// validator fails. Handlers wrap the underlying validator error via
// NewValidationError so the cause is logged but not leaked to clients.
var ErrValidation = &AppError{HTTPStatus: 400, Code: "VALIDATION_ERROR", Message: "request validation failed"}

// NewValidationError wraps cause inside ErrValidation for handler use.
func NewValidationError(cause error) *AppError { return ErrValidation.WithCause(cause) }
