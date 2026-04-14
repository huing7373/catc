package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/huing7373/catc/server/pkg/jwtx"
	"github.com/huing7373/catc/server/pkg/logx"
)

// AuthRequired verifies a Bearer access token and attaches user_id to
// the Gin context. Requests without a valid token are short-circuited
// with 401.
func AuthRequired(mgr *jwtx.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := extractBearer(c.GetHeader("Authorization"))
		if !ok {
			abortUnauthorized(c)
			return
		}
		uid, err := mgr.ParseAccess(token)
		if err != nil {
			abortUnauthorized(c)
			return
		}
		c.Set(CtxKeyUserID, string(uid))

		ctx := logx.WithUserID(c.Request.Context(), string(uid))
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

func extractBearer(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return "", false
	}
	return token, true
}

func abortUnauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"error": gin.H{"code": "UNAUTHORIZED", "message": "unauthorized"},
	})
}
