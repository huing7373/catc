package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/huing7373/catc/server/pkg/logx"
)

// RequestLogger generates a request_id, propagates it via both Gin
// context and the request's standard context (so log.Ctx inherits it),
// and emits one structured access-log entry when the request finishes.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		rid := uuid.NewString()
		c.Set(CtxKeyRequestID, rid)

		ctx := logx.ContextWithRequestID(c.Request.Context(), rid)
		c.Request = c.Request.WithContext(ctx)

		c.Writer.Header().Set("X-Request-ID", rid)

		c.Next()

		duration := time.Since(start)
		entry := log.Ctx(c.Request.Context()).Info().
			Str("endpoint", c.Request.Method+" "+c.FullPath()).
			Int("status_code", c.Writer.Status()).
			Int64("duration_ms", duration.Milliseconds())

		uid := UserIDFrom(c)
		if uid != "" {
			entry = entry.Str("user_id", string(uid))
		}
		entry.Msg("http request")
	}
}
