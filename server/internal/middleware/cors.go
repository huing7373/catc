package middleware

import "github.com/gin-gonic/gin"

// CORS is a placeholder middleware for CORS handling.
func CORS() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
	}
}
