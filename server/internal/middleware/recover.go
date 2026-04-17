package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/huing/cat/server/pkg/logx"
)

func Recover() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logx.Ctx(c.Request.Context()).Error().
					Interface("panic", r).
					Str("stack", string(debug.Stack())).
					Msg("panic recovered")

				if !c.Writer.Written() {
					c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
						"code":    "INTERNAL_ERROR",
						"message": "internal server error",
					})
				} else {
					c.Abort()
				}
			}
		}()
		c.Next()
	}
}
