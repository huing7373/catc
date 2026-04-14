package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
)

// Limiter is the rate-limiter surface consumed by middleware. A concrete
// Redis-backed token-bucket implementation will land with Story 2-2.
// Until then, NullLimiter allows all requests.
type Limiter interface {
	// Allow reports whether a request identified by key is allowed. The
	// rate and burst arguments let the caller scope buckets per endpoint.
	Allow(ctx context.Context, key string, rate int, burst int) (bool, error)
}

// NullLimiter always permits. It keeps the middleware wiring stable
// before real limits are needed.
type NullLimiter struct{}

// Allow returns (true, nil) unconditionally.
func (NullLimiter) Allow(ctx context.Context, key string, rate, burst int) (bool, error) {
	return true, nil
}

// RateLimit returns a Gin middleware that consults lim. keyFn derives a
// bucket key from the request (typically remote IP before auth, user_id
// after). rate and burst are passed through to lim.Allow.
func RateLimit(lim Limiter, keyFn func(*gin.Context) string, rate, burst int) gin.HandlerFunc {
	if lim == nil {
		lim = NullLimiter{}
	}
	return func(c *gin.Context) {
		key := keyFn(c)
		ok, err := lim.Allow(c.Request.Context(), key, rate, burst)
		if err != nil || !ok {
			c.AbortWithStatusJSON(429, gin.H{
				"error": gin.H{"code": "RATE_LIMITED", "message": "too many requests"},
			})
			return
		}
		c.Next()
	}
}

// IPKey is a default keyFn that returns the client IP.
func IPKey(c *gin.Context) string { return c.ClientIP() }
