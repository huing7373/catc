package dto

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

// errorBody is the outer shape of the JSON error payload:
//
//	{"error": {"code": "...", "message": "..."}}
type errorBody struct {
	Error errorPayload `json:"error"`
}

type errorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// RespondSuccess writes status + payload as JSON. Payload is returned
// verbatim (no {"data": ...} wrapper, per API convention).
func RespondSuccess(c *gin.Context, status int, payload any) {
	c.JSON(status, payload)
}

// RespondAppError writes the JSON error payload corresponding to err.
// If err is an *AppError (directly or via errors.As), its HTTPStatus/
// Code/Message are used and the wrapped cause is logged. Anything else
// becomes a generic 500 INTERNAL_ERROR.
func RespondAppError(c *gin.Context, err error) {
	var ae *AppError
	if errors.As(err, &ae) && ae != nil {
		log.Ctx(c.Request.Context()).
			Error().
			Err(ae.Wrapped).
			Str("code", ae.Code).
			Int("status", ae.HTTPStatus).
			Msg("app error")
		c.JSON(ae.HTTPStatus, errorBody{Error: errorPayload{Code: ae.Code, Message: ae.Message}})
		return
	}
	log.Ctx(c.Request.Context()).
		Error().
		Err(err).
		Msg("unhandled error")
	c.JSON(http.StatusInternalServerError, errorBody{Error: errorPayload{Code: "INTERNAL_ERROR", Message: "internal server error"}})
}
