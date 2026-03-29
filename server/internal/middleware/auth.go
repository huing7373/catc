package middleware

import "github.com/gin-gonic/gin"

// AuthRequired is a placeholder middleware for JWT authentication.
// Will be implemented in Story 2.2.
func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}
