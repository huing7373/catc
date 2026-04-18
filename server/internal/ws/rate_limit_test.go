package ws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConnLimiter_AllowWithinRate(t *testing.T) {
	t.Parallel()

	limiter := newConnLimiter(100)
	for i := 0; i < 100; i++ {
		assert.True(t, limiter.Allow(), "request %d should be allowed", i)
	}
}

func TestConnLimiter_ExceedRate(t *testing.T) {
	t.Parallel()

	limiter := newConnLimiter(10)
	for i := 0; i < 10; i++ {
		limiter.Allow()
	}
	assert.False(t, limiter.Allow(), "should reject after burst exhausted")
}
