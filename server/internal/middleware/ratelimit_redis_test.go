package middleware

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/huing7373/catc/server/internal/repository"
)

func newRedisFixture(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	s, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(s.Close)
	c := redis.NewClient(&redis.Options{Addr: s.Addr()})
	t.Cleanup(func() { _ = c.Close() })
	return c, s
}

func TestRedisLimiter_FirstRequestAllowed(t *testing.T) {
	c, _ := newRedisFixture(t)
	lim := NewRedisLimiter(c, "auth-login", 60, repository.RateLimitKey)

	ok, err := lim.Allow(context.Background(), "1.2.3.4", 10, 10)
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if !ok {
		t.Errorf("first request must be allowed")
	}
}

func TestRedisLimiter_WithinLimit_AllAllowed(t *testing.T) {
	c, _ := newRedisFixture(t)
	lim := NewRedisLimiter(c, "auth-login", 60, repository.RateLimitKey)

	for i := 0; i < 10; i++ {
		ok, err := lim.Allow(context.Background(), "ip", 10, 10)
		if err != nil {
			t.Fatalf("Allow #%d: %v", i, err)
		}
		if !ok {
			t.Fatalf("request %d denied while under limit", i)
		}
	}
}

func TestRedisLimiter_OverLimit_Denied(t *testing.T) {
	c, _ := newRedisFixture(t)
	lim := NewRedisLimiter(c, "auth-login", 60, repository.RateLimitKey)

	for i := 0; i < 10; i++ {
		if ok, _ := lim.Allow(context.Background(), "ip", 10, 10); !ok {
			t.Fatalf("setup: request %d should pass", i)
		}
	}
	ok, err := lim.Allow(context.Background(), "ip", 10, 10)
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if ok {
		t.Errorf("11th request must be denied (rate=10)")
	}
}

func TestRedisLimiter_WindowSlides(t *testing.T) {
	c, s := newRedisFixture(t)
	lim := NewRedisLimiter(c, "auth-login", 60, repository.RateLimitKey)

	// Saturate the bucket.
	for i := 0; i < 10; i++ {
		if ok, _ := lim.Allow(context.Background(), "ip", 10, 10); !ok {
			t.Fatalf("setup denied at %d", i)
		}
	}
	if ok, _ := lim.Allow(context.Background(), "ip", 10, 10); ok {
		t.Fatalf("over-limit request should be denied before sliding")
	}

	// Advance miniredis clock past the window. Note: miniredis FastForward
	// only affects EXPIRE-tracked TTLs, not score-based ZREMRANGEBYSCORE
	// reasoning. We rely on real wall-clock for ZSET scores, so to slide
	// the window we sleep just long enough — but to keep the test fast we
	// instead build a shorter window.
	_ = s
	// Re-create with 1-second window for the slide assertion.
	limShort := NewRedisLimiter(c, "auth-login-short", 1, repository.RateLimitKey)
	for i := 0; i < 10; i++ {
		if ok, _ := limShort.Allow(context.Background(), "k", 10, 10); !ok {
			t.Fatalf("short-window setup denied at %d", i)
		}
	}
	if ok, _ := limShort.Allow(context.Background(), "k", 10, 10); ok {
		t.Fatalf("short-window: 11th must be denied")
	}
	time.Sleep(1100 * time.Millisecond)
	if ok, _ := limShort.Allow(context.Background(), "k", 10, 10); !ok {
		t.Errorf("after window slide, request must be allowed again")
	}
}

func TestRedisLimiter_FailOpen_OnRedisDown(t *testing.T) {
	c, s := newRedisFixture(t)
	lim := NewRedisLimiter(c, "auth-login", 60, repository.RateLimitKey)

	// Kill Redis to force a connection error.
	s.Close()

	ok, err := lim.Allow(context.Background(), "ip", 10, 10)
	if err != nil {
		t.Fatalf("Allow must NOT surface an error on fail-open: %v", err)
	}
	if !ok {
		t.Errorf("fail-open must return true when Redis is down")
	}
}

// Smoke: ensure the key built matches the centralised builder.
func TestRedisLimiter_KeyShape(t *testing.T) {
	got := repository.RateLimitKey("auth-login", "1.2.3.4")
	if got != "ratelimit:auth-login:1.2.3.4" {
		t.Errorf("RateLimitKey shape changed: %q", got)
	}
}
