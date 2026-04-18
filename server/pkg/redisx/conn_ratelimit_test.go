package redisx

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/pkg/clockx"
)

func newTestFakeClock() *clockx.FakeClock {
	return clockx.NewFakeClock(time.Unix(1_700_000_000, 0))
}

func TestRedisConnectRateLimiter_AllowsUnderThreshold(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	fc := newTestFakeClock()
	rl := NewConnectRateLimiter(cmd, fc, 5, 60*time.Second)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		fc.Advance(time.Millisecond) // distinct timestamps per attempt
		d, err := rl.AcquireConnectSlot(ctx, "u1")
		require.NoError(t, err)
		assert.True(t, d.Allowed, "attempt %d must be allowed", i)
		assert.Equal(t, int64(i), d.Count)
	}
}

func TestRedisConnectRateLimiter_BlocksSixth(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	fc := newTestFakeClock()
	rl := NewConnectRateLimiter(cmd, fc, 5, 60*time.Second)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		fc.Advance(time.Millisecond)
		_, err := rl.AcquireConnectSlot(ctx, "u1")
		require.NoError(t, err)
	}

	fc.Advance(time.Millisecond)
	d, err := rl.AcquireConnectSlot(ctx, "u1")
	require.NoError(t, err)
	assert.False(t, d.Allowed)
	assert.Equal(t, int64(6), d.Count, "count keeps incrementing even when blocked — needed for audit")
	assert.Greater(t, d.RetryAfter, time.Duration(0))
	assert.LessOrEqual(t, d.RetryAfter, 60*time.Second)
}

func TestRedisConnectRateLimiter_ResetsAfterWindow(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	fc := newTestFakeClock()
	rl := NewConnectRateLimiter(cmd, fc, 5, 60*time.Second)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		fc.Advance(time.Millisecond)
		_, err := rl.AcquireConnectSlot(ctx, "u1")
		require.NoError(t, err)
	}

	fc.Advance(61 * time.Second)

	d, err := rl.AcquireConnectSlot(ctx, "u1")
	require.NoError(t, err)
	assert.True(t, d.Allowed)
	assert.Equal(t, int64(1), d.Count, "count should reset after all prior entries age out")
}

// TestRedisConnectRateLimiter_SlidingWindow_NoBoundaryBypass verifies the
// attack vector that motivated switching from INCR+EXPIRE-NX to a sorted
// set: in a fixed-window scheme, 5 connects just before expiry plus 5 more
// immediately after the window reset could both be admitted (total 10
// within a few seconds, violating 60s ≤ 5). With a sliding window, the
// oldest attempt only ages out one at a time; a 6th attempt at t just under
// 60s after the first must still be blocked because no prior entries have
// left the window yet.
func TestRedisConnectRateLimiter_SlidingWindow_NoBoundaryBypass(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	fc := newTestFakeClock()
	rl := NewConnectRateLimiter(cmd, fc, 5, 60*time.Second)
	ctx := context.Background()

	// Burst of 5 rapid connects (1μs apart — effectively simultaneous).
	for i := 1; i <= 5; i++ {
		fc.Advance(time.Microsecond)
		d, err := rl.AcquireConnectSlot(ctx, "u1")
		require.NoError(t, err)
		require.True(t, d.Allowed, "burst #%d must succeed", i)
	}

	// Advance to just before the oldest entry ages out (59.999s past the
	// last burst attempt, so still inside the window for all 5).
	fc.Advance(59*time.Second + 999*time.Millisecond)

	d, err := rl.AcquireConnectSlot(ctx, "u1")
	require.NoError(t, err)
	assert.False(t, d.Allowed,
		"sliding window must still be full ~60s after burst — fixed-window bypass regression")
	assert.Equal(t, int64(6), d.Count, "blocked attempt still counted for audit")
}

func TestRedisConnectRateLimiter_RetryAfterTracksOldestEntry(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	fc := newTestFakeClock()
	rl := NewConnectRateLimiter(cmd, fc, 5, 60*time.Second)
	ctx := context.Background()

	// Fill the window at t0.
	for i := 1; i <= 5; i++ {
		fc.Advance(time.Millisecond)
		_, err := rl.AcquireConnectSlot(ctx, "u1")
		require.NoError(t, err)
	}

	// Check retry at +50s — oldest entry ages out at ~t+60s, so retry ≈ 10s.
	fc.Advance(50 * time.Second)
	d, err := rl.AcquireConnectSlot(ctx, "u1")
	require.NoError(t, err)
	require.False(t, d.Allowed)
	assert.InDelta(t, (10 * time.Second).Seconds(), d.RetryAfter.Seconds(), 1.5,
		"retry-after must track the oldest in-window entry, not the full window")
}

func TestRedisConnectRateLimiter_IndependentUsers(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	fc := newTestFakeClock()
	rl := NewConnectRateLimiter(cmd, fc, 5, 60*time.Second)

	var wg sync.WaitGroup
	var mu sync.Mutex
	run := func(uid string) {
		defer wg.Done()
		for i := 1; i <= 5; i++ {
			mu.Lock()
			fc.Advance(time.Millisecond)
			mu.Unlock()
			d, err := rl.AcquireConnectSlot(context.Background(), uid)
			assert.NoError(t, err)
			assert.True(t, d.Allowed, "user %s attempt %d must be allowed (isolated)", uid, i)
		}
	}

	wg.Add(2)
	go run("alice")
	go run("bob")
	wg.Wait()

	// Each user should now be at exactly 5; a sixth attempt blocks per-user.
	for _, uid := range []string{"alice", "bob"} {
		fc.Advance(time.Millisecond)
		d, err := rl.AcquireConnectSlot(context.Background(), uid)
		require.NoError(t, err)
		assert.False(t, d.Allowed, "user %s sixth attempt must block", uid)
		assert.Equal(t, int64(6), d.Count)
	}
}

func TestRedisConnectRateLimiter_Constructor_PanicsOnBadParams(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	fc := newTestFakeClock()

	cases := []struct {
		name      string
		clock     clockx.Clock
		threshold int64
		window    time.Duration
	}{
		{name: "nil clock", clock: nil, threshold: 5, window: time.Second},
		{name: "zero threshold", clock: fc, threshold: 0, window: time.Second},
		{name: "negative threshold", clock: fc, threshold: -1, window: time.Second},
		{name: "zero window", clock: fc, threshold: 5, window: 0},
		{name: "negative window", clock: fc, threshold: 5, window: -time.Second},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assert.Panics(t, func() { NewConnectRateLimiter(cmd, c.clock, c.threshold, c.window) })
		})
	}
}

func TestRedisConnectRateLimiter_KeyFormat(t *testing.T) {
	t.Parallel()
	mr, cmd := setupMiniredis(t)
	fc := newTestFakeClock()
	rl := NewConnectRateLimiter(cmd, fc, 5, 60*time.Second)

	_, err := rl.AcquireConnectSlot(context.Background(), "u-xyz")
	require.NoError(t, err)

	assert.True(t, mr.Exists("ratelimit:ws:u-xyz"))
}
