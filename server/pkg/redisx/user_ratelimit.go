package redisx

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
)

// UserSlidingWindowLimiter is a per-user TRUE sliding-window limiter.
// Unlike RedisConnectRateLimiter (which is hard-coded to `ratelimit:ws:*`
// and returns a WS-specific ConnectDecision), this type takes its key
// prefix at construction so different endpoints can share the same
// implementation:
//
//	"ratelimit:apns_token:<userID>"     (Story 1.4 — this story)
//	"ratelimit:profile_update:<userID>" (Story 1.5, future)
//	"ratelimit:touch_send:<fromUserID>:<toUserID>" (Story 5.3, future)
//
// The algorithm is identical to RedisConnectRateLimiter — ZSET with
// Unix-millisecond scores, ZREMRANGEBYSCORE on every attempt, and a
// PEXPIRE to GC dormant keys. See that type's docs for
// millisecond-vs-nanosecond rationale, sliding-vs-fixed trade-off, and
// boundary d==0 clamping.
//
// # Why two nearly-identical types
//
// RedisConnectRateLimiter's signature is AcquireConnectSlot(ctx, userID
// string) (ConnectDecision, error) — the userID is a bare string because
// pkg/ids.UserID did not yet exist when Story 0.11 landed. Changing that
// signature now would ripple across the WS hub tests. Keeping a second
// type that takes ids.UserID lets /v1/* business endpoints stay typed.
// TODO(refactor): once Story 2+ lands, fold both into a shared
// slidingWindowAcquire(pipe, key, nowMs, windowMs, threshold) helper so
// there is one sliding-window implementation with two thin adapters.
type UserSlidingWindowLimiter struct {
	cmd       redis.Cmdable
	clock     clockx.Clock
	keyPrefix string
	threshold int64
	window    time.Duration
}

// NewUserSlidingWindowLimiter fail-fast-validates every input. keyPrefix
// MUST end in ':' so the concatenated `<prefix><userID>` Redis key is
// injective — a prefix like "ratelimit:apns_token" would collide with a
// future key called "ratelimit:apns_tokenv2:<id>" (review-antipatterns
// §8.2). threshold > 0 and window > 0 are mandatory so a mis-zeroed
// config does not silently disable limiting (review-antipatterns §4.1).
func NewUserSlidingWindowLimiter(
	cmd redis.Cmdable,
	clock clockx.Clock,
	keyPrefix string,
	threshold int64,
	window time.Duration,
) *UserSlidingWindowLimiter {
	if cmd == nil {
		panic("redisx.NewUserSlidingWindowLimiter: cmd must not be nil")
	}
	if clock == nil {
		panic("redisx.NewUserSlidingWindowLimiter: clock must not be nil")
	}
	if keyPrefix == "" {
		panic("redisx.NewUserSlidingWindowLimiter: keyPrefix must not be empty")
	}
	if !strings.HasSuffix(keyPrefix, ":") {
		panic("redisx.NewUserSlidingWindowLimiter: keyPrefix must end in ':' (review-antipatterns §8.2 injectivity)")
	}
	if threshold <= 0 {
		panic("redisx.NewUserSlidingWindowLimiter: threshold must be > 0")
	}
	if window <= 0 {
		panic("redisx.NewUserSlidingWindowLimiter: window must be > 0")
	}
	return &UserSlidingWindowLimiter{
		cmd:       cmd,
		clock:     clock,
		keyPrefix: keyPrefix,
		threshold: threshold,
		window:    window,
	}
}

// Acquire records the current attempt in the sliding-window ZSET and
// decides whether the attempt is permitted. Returns (allowed,
// retryAfter, err). retryAfter is meaningful only when allowed == false,
// and is the minimum wait until the oldest in-window entry ages out —
// NOT the full window. A sub-millisecond / boundary result is clamped to
// 1 ms so the upstream ceilSeconds yields a usable Retry-After header
// rather than handing the client a full-window stall at the precise
// moment the slot frees (mirrors RedisConnectRateLimiter round-2 fix).
func (l *UserSlidingWindowLimiter) Acquire(ctx context.Context, userID ids.UserID) (bool, time.Duration, error) {
	key := l.keyPrefix + string(userID)
	now := l.clock.Now()
	nowMillis := now.UnixMilli()
	cutoff := now.Add(-l.window).UnixMilli()

	// Member must be unique per attempt — two attempts at the same
	// millisecond would otherwise collapse into a single ZSET entry.
	member := strconv.FormatInt(nowMillis, 10) + ":" + uuid.New().String()

	pipe := l.cmd.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "-inf", "("+strconv.FormatInt(cutoff, 10))
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(nowMillis), Member: member})
	zcardCmd := pipe.ZCard(ctx, key)
	zrangeCmd := pipe.ZRangeWithScores(ctx, key, 0, 0)
	pipe.PExpire(ctx, key, l.window)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, 0, err
	}

	count := zcardCmd.Val()
	if count <= l.threshold {
		return true, 0, nil
	}

	retry := l.window
	oldest := zrangeCmd.Val()
	if len(oldest) > 0 {
		oldestMillis := int64(oldest[0].Score)
		ageoutMillis := oldestMillis + l.window.Milliseconds()
		d := time.Duration(ageoutMillis-nowMillis) * time.Millisecond
		switch {
		case d <= 0:
			retry = time.Millisecond
		case d <= l.window:
			retry = d
		}
	}
	return false, retry, nil
}
