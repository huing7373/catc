package dto

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ErrorBody is the standard error response wrapper.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail contains the error code and message.
type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RespondError sends a standardized error JSON response.
func RespondError(c *gin.Context, statusCode int, code, message string) {
	c.JSON(statusCode, ErrorBody{
		Error: ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// RespondSuccess sends a JSON response with the given data directly (no wrapper).
func RespondSuccess(c *gin.Context, statusCode int, data interface{}) {
	c.JSON(statusCode, data)
}

// RespondNotFound sends a 404 error.
func RespondNotFound(c *gin.Context, message string) {
	RespondError(c, http.StatusNotFound, "NOT_FOUND", message)
}

// RespondBadRequest sends a 400 error.
func RespondBadRequest(c *gin.Context, message string) {
	RespondError(c, http.StatusBadRequest, "BAD_REQUEST", message)
}

// RespondUnauthorized sends a 401 error.
func RespondUnauthorized(c *gin.Context, message string) {
	RespondError(c, http.StatusUnauthorized, "UNAUTHORIZED", message)
}

// RespondInternalError sends a 500 error.
func RespondInternalError(c *gin.Context, message string) {
	RespondError(c, http.StatusInternalServerError, "INTERNAL_ERROR", message)
}
