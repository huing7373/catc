package redisx

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
)

const apnsTokenPrefix = "ratelimit:apns_token:"

func newUserRLFakeClock() *clockx.FakeClock {
	return clockx.NewFakeClock(time.Unix(1_700_000_000, 0))
}

func TestUserSlidingWindowLimiter_UnderThresholdAllowed(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	fc := newUserRLFakeClock()
	rl := NewUserSlidingWindowLimiter(cmd, fc, apnsTokenPrefix, 5, 60*time.Second)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		fc.Advance(time.Second)
		allowed, retry, err := rl.Acquire(ctx, ids.UserID("u1"))
		require.NoError(t, err)
		assert.True(t, allowed, "attempt %d must be allowed", i)
		assert.Equal(t, time.Duration(0), retry)
	}
}

func TestUserSlidingWindowLimiter_AtThresholdBlockedOn6th(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	fc := newUserRLFakeClock()
	rl := NewUserSlidingWindowLimiter(cmd, fc, apnsTokenPrefix, 5, 60*time.Second)
	ctx := context.Background()

	for i := 1; i <= 5; i++ {
		fc.Advance(time.Second)
		_, _, err := rl.Acquire(ctx, ids.UserID("u1"))
		require.NoError(t, err)
	}

	fc.Advance(time.Second)
	allowed, retry, err := rl.Acquire(ctx, ids.UserID("u1"))
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Greater(t, retry, time.Duration(0))
	assert.LessOrEqual(t, retry, 60*time.Second)
}

// TestUserSlidingWindowLimiter_SlidingBoundary proves the true sliding
// window: with 5 attempts spread across a second, jumping ~61s past the
// first attempt ages that slot out and unblocks the 6th attempt. An
// INCR+EXPIRE fixed-window regression would fail here (slots 2-5 would
// still carry a full window forward).
func TestUserSlidingWindowLimiter_SlidingBoundary(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	fc := newUserRLFakeClock()
	rl := NewUserSlidingWindowLimiter(cmd, fc, apnsTokenPrefix, 5, 60*time.Second)
	ctx := context.Background()

	// 5 attempts at t = 1s, 2s, 3s, 4s, 5s (relative to fc init).
	for i := 1; i <= 5; i++ {
		fc.Advance(time.Second)
		allowed, _, err := rl.Acquire(ctx, ids.UserID("u1"))
		require.NoError(t, err)
		require.True(t, allowed)
	}

	// Jump to t = 62s — first attempt (at 1s) is now 61s old and fully
	// aged out; 4 attempts remain in-window. Next Acquire adds a 5th →
	// still under threshold, allowed.
	fc.Advance(57 * time.Second)
	allowed, _, err := rl.Acquire(ctx, ids.UserID("u1"))
	require.NoError(t, err)
	assert.True(t, allowed, "oldest entry must age out in a true sliding window")
}

// TestUserSlidingWindowLimiter_BoundaryRetryClampedTo1ms reproduces the
// §9.3 d<=0 branch: when the oldest retained entry's score equals the
// cutoff, the ageout delta is zero. The naive guard (`d > 0`) would fall
// through to retry = full window (60s), handing the client a bogus
// Retry-After at the exact moment the slot would free. The fix clamps
// to 1 ms so ceilSeconds yields a usable (≥ 1s) header.
func TestUserSlidingWindowLimiter_BoundaryRetryClampedTo1ms(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	fc := newUserRLFakeClock()
	rl := NewUserSlidingWindowLimiter(cmd, fc, apnsTokenPrefix, 5, 60*time.Second)
	ctx := context.Background()

	// Seed 6 attempts spaced 1 ms apart (the 6th is already blocked;
	// all 6 land in the ZSET for later accounting). Millisecond
	// granularity is required so ZSET float64 scores round-trip exactly.
	for i := 1; i <= 6; i++ {
		fc.Advance(time.Millisecond)
		_, _, err := rl.Acquire(ctx, ids.UserID("u1"))
		require.NoError(t, err)
	}

	// Advance so cutoff lands exactly on the second seeded entry (t0+2ms).
	// ZRemRangeByScore uses "-inf" ... "(cutoff" (exclusive), so the entry
	// at score == cutoff is retained → oldest.score == cutoff → d == 0.
	// Current clock is t0+6ms; jump to t0+60s+2ms.
	fc.Advance(60*time.Second - 4*time.Millisecond)

	allowed, retry, err := rl.Acquire(ctx, ids.UserID("u1"))
	require.NoError(t, err)
	require.False(t, allowed)
	assert.Greater(t, retry, time.Duration(0))
	assert.Less(t, retry, time.Second,
		"boundary (d==0) must NOT hand the client a full-window Retry-After")
}

func TestUserSlidingWindowLimiter_PerUserIsolation(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	fc := newUserRLFakeClock()
	rl := NewUserSlidingWindowLimiter(cmd, fc, apnsTokenPrefix, 5, 60*time.Second)
	ctx := context.Background()

	// u1 fills the window.
	for i := 1; i <= 5; i++ {
		fc.Advance(time.Millisecond)
		_, _, err := rl.Acquire(ctx, ids.UserID("u1"))
		require.NoError(t, err)
	}
	fc.Advance(time.Millisecond)
	blocked, _, err := rl.Acquire(ctx, ids.UserID("u1"))
	require.NoError(t, err)
	require.False(t, blocked, "u1 sixth attempt blocks")

	// u2 is unaffected.
	fc.Advance(time.Millisecond)
	allowed, _, err := rl.Acquire(ctx, ids.UserID("u2"))
	require.NoError(t, err)
	assert.True(t, allowed, "u2 must be isolated from u1")
}

func TestUserSlidingWindowLimiter_KeyPrefixAppliedCorrectly(t *testing.T) {
	t.Parallel()
	mr, cmd := setupMiniredis(t)
	fc := newUserRLFakeClock()
	rl := NewUserSlidingWindowLimiter(cmd, fc, apnsTokenPrefix, 5, 60*time.Second)

	_, _, err := rl.Acquire(context.Background(), ids.UserID("u-xyz"))
	require.NoError(t, err)

	assert.True(t, mr.Exists("ratelimit:apns_token:u-xyz"),
		"keyPrefix must be prepended to userID verbatim")
	assert.False(t, mr.Exists("ratelimit:ws:u-xyz"),
		"must not collide with RedisConnectRateLimiter namespace")
}

func TestUserSlidingWindowLimiter_NilDepsPanic(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	fc := newUserRLFakeClock()

	cases := []struct {
		name      string
		cmdArg    any
		clock     clockx.Clock
		prefix    string
		threshold int64
		window    time.Duration
	}{
		{name: "nil cmd", cmdArg: nil, clock: fc, prefix: apnsTokenPrefix, threshold: 5, window: time.Second},
		{name: "nil clock", cmdArg: "cmd", clock: nil, prefix: apnsTokenPrefix, threshold: 5, window: time.Second},
		{name: "empty prefix", cmdArg: "cmd", clock: fc, prefix: "", threshold: 5, window: time.Second},
		{name: "zero threshold", cmdArg: "cmd", clock: fc, prefix: apnsTokenPrefix, threshold: 0, window: time.Second},
		{name: "zero window", cmdArg: "cmd", clock: fc, prefix: apnsTokenPrefix, threshold: 5, window: 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assert.Panics(t, func() {
				if c.cmdArg == nil {
					NewUserSlidingWindowLimiter(nil, c.clock, c.prefix, c.threshold, c.window)
				} else {
					NewUserSlidingWindowLimiter(cmd, c.clock, c.prefix, c.threshold, c.window)
				}
			})
		})
	}
}

func TestUserSlidingWindowLimiter_KeyPrefixWithoutColonPanics(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	fc := newUserRLFakeClock()
	assert.Panics(t, func() {
		NewUserSlidingWindowLimiter(cmd, fc, "ratelimit:apns_token", 5, time.Second)
	})
}
