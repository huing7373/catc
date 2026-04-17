package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/huing/cat/server/pkg/logx"
)

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		defer func() {
			logx.Ctx(c.Request.Context()).Info().
				Str("method", c.Request.Method).
				Str("path", c.Request.URL.Path).
				Int("status", c.Writer.Status()).
				Int64("durationMs", time.Since(start).Milliseconds()).
				Msg("request")
		}()

		c.Next()
	}
}
