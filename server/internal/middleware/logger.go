package middleware

import (
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	// RequestIDKey is the context key for the request ID.
	RequestIDKey = "request_id"
	// UserIDKey is the context key for the authenticated user ID.
	UserIDKey = "user_id"
)

// InitLogger sets up zerolog for JSON output to stdout.
func InitLogger() {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = zerolog.New(os.Stdout).With().Timestamp().Logger()
}

// RequestLogger returns a Gin middleware that logs each request with structured JSON.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Generate request ID
		requestID := uuid.New().String()
		c.Set(RequestIDKey, requestID)
		c.Header("X-Request-ID", requestID)

		// Process request
		c.Next()

		// Log after request completes
		duration := time.Since(start)
		statusCode := c.Writer.Status()

		event := log.Info()
		if statusCode >= 500 {
			event = log.Error()
		} else if statusCode >= 400 {
			event = log.Warn()
		}

		event.
			Str("request_id", requestID).
			Str("endpoint", c.Request.Method+" "+c.FullPath()).
			Int("status_code", statusCode).
			Int64("duration_ms", duration.Milliseconds()).
			Str("client_ip", c.ClientIP())

		// Include user_id if authenticated
		if userID, exists := c.Get(UserIDKey); exists {
			event.Str("user_id", userID.(string))
		}

		event.Msg("request")
	}
}
