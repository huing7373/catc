package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Envelope struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Data      any    `json:"data"`
	RequestID string `json:"requestId"`
}

func Success(c *gin.Context, data any, message string) {
	if data == nil {
		data = map[string]any{}
	}
	c.JSON(http.StatusOK, Envelope{
		Code:      0,
		Message:   message,
		Data:      data,
		RequestID: requestIDFromCtx(c),
	})
}

func Error(c *gin.Context, httpStatus, code int, message string) {
	c.JSON(httpStatus, Envelope{
		Code:      code,
		Message:   message,
		Data:      map[string]any{},
		RequestID: requestIDFromCtx(c),
	})
}

func requestIDFromCtx(c *gin.Context) string {
	if v := c.Request.Header.Get("X-Request-Id"); v != "" {
		return v
	}
	return "req_xxx"
}
