// Package middleware hosts Gin middleware. Middleware may only attach
// cross-cutting state (request_id, user_id, allow/deny) to the Gin
// context; business decisions belong in handler/service layers.
package middleware

import (
	"github.com/gin-gonic/gin"

	"github.com/huing7373/catc/server/pkg/ids"
)

// Gin context keys. Exported as constants so handlers reference them
// symbolically rather than by magic string.
const (
	CtxKeyUserID    = "cat.user_id"
	CtxKeyRequestID = "cat.request_id"
)

// UserIDFrom reads the authenticated user id previously attached by
// AuthRequired. Returns empty string if absent.
func UserIDFrom(c *gin.Context) ids.UserID {
	if v, ok := c.Get(CtxKeyUserID); ok {
		if s, ok := v.(string); ok {
			return ids.UserID(s)
		}
	}
	return ""
}

// RequestIDFrom reads the request id attached by RequestLogger.
func RequestIDFrom(c *gin.Context) string {
	if v, ok := c.Get(CtxKeyRequestID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
