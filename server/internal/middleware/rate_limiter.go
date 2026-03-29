package middleware

import "github.com/gin-gonic/gin"

// RateLimiter is a placeholder middleware for rate limiting.
// Will be implemented after Story 2.2 authentication is complete.
func RateLimiter() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}
