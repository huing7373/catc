package redisx

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/huing/cat/server/pkg/clockx"
)

// ConnectDecision is the result of a connect-slot acquisition. Defined in
// pkg/redisx (not internal/ws) because Go uses nominal typing for interface
// parameters — the internal/ws consumer interface and this package's
// implementation must reference the same concrete type. internal/ws
// re-exports it via a type alias so business code treats it as a ws-layer
// value while the physical location respects the pkg → internal dependency
// direction (same approach as DedupResult in dedup.go).
//
// Count is always populated (first allowed slot yields Count=1) so audit
// logs can capture the real scale of a connect storm. RetryAfter is only
// meaningful when Allowed == false.
type ConnectDecision struct {
	Allowed    bool
	Count      int64
	RetryAfter time.Duration
}

// RedisConnectRateLimiter enforces a TRUE sliding window on WS connect
// attempts per user, backed by a Redis sorted set whose scores are attempt
// timestamps (nanoseconds). An earlier INCR+EXPIRE-NX implementation was
// correctly flagged as a fixed window (boundary bypass: 5 connects just
// before expiry + 5 connects immediately after reset → 10 within ~2s).
//
// Key space (separated from blacklist:device:* / lock:cron:* / event:* per D16):
//
//	ratelimit:ws:{userID}  →  ZSET { member = "<nanos>:<uuid>", score = <nanos> }
//
// Algorithm (single pipeline — ZADD is atomic, the ZREMRANGEBYSCORE is a
// read-before-write from the same replica's perspective):
//
//  1. ZREMRANGEBYSCORE key -inf (cutoff   → drop entries older than window
//  2. ZADD             key score=now member=now:uuid
//  3. ZCARD            key                 → count within window
//  4. ZRANGE           key 0 0 WITHSCORES  → oldest in-window entry
//  5. PEXPIRE          key window          → GC dormant users' keys
//
// newCount > threshold → Allowed=false. RetryAfter is computed from the
// oldest in-window entry (ageoutAt − now) so the client waits the *minimum*
// time until a slot frees up, not a full window.
//
// Count is NOT capped at threshold — keeping the true value lets audit logs
// describe the real scale of a misbehaving client (the J4 scenario: a single
// user reconnecting 100 times/sec). Failed attempts still ZADD, so the ZSET
// reflects the actual attempt volume; since the key has PEXPIRE=window, it
// is bounded by threshold × retry-rate × window in the worst case.
type RedisConnectRateLimiter struct {
	cmd       redis.Cmdable
	clock     clockx.Clock
	threshold int64
	window    time.Duration
}

// NewConnectRateLimiter panics on nil clock or non-positive threshold/window
// — production config must be valid, and a silent fail-open on
// misconfiguration would re-open the J4 failure mode (single user saturating
// the WS hub). Validate at construction rather than on first Acquire so the
// crash surfaces during service startup, not during a midnight incident.
func NewConnectRateLimiter(cmd redis.Cmdable, clock clockx.Clock, threshold int64, window time.Duration) *RedisConnectRateLimiter {
	if clock == nil {
		panic("redisx.NewConnectRateLimiter: clock must not be nil")
	}
	if threshold <= 0 {
		panic("redisx.NewConnectRateLimiter: threshold must be > 0")
	}
	if window <= 0 {
		panic("redisx.NewConnectRateLimiter: window must be > 0")
	}
	return &RedisConnectRateLimiter{cmd: cmd, clock: clock, threshold: threshold, window: window}
}

func connRateKey(userID string) string {
	return "ratelimit:ws:" + userID
}

// AcquireConnectSlot records the current attempt in the sliding-window ZSET
// and decides whether the attempt is permitted. See type docs for algorithm
// rationale and sliding-window vs fixed-window trade-off.
func (r *RedisConnectRateLimiter) AcquireConnectSlot(ctx context.Context, userID string) (ConnectDecision, error) {
	key := connRateKey(userID)
	now := r.clock.Now()
	nowNanos := now.UnixNano()
	cutoff := now.Add(-r.window).UnixNano()

	// Member must be unique per attempt — two attempts at the same nanosecond
	// would otherwise collapse into a single ZSET entry (ZADD is set-valued).
	member := strconv.FormatInt(nowNanos, 10) + ":" + uuid.New().String()

	pipe := r.cmd.Pipeline()
	// Remove entries strictly older than (now - window). Entries at exactly
	// score == cutoff are the just-turned-60s-old boundary — keep them
	// in-window (conservative: reject rather than risk missing a burst).
	pipe.ZRemRangeByScore(ctx, key, "-inf", "("+strconv.FormatInt(cutoff, 10))
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(nowNanos), Member: member})
	zcardCmd := pipe.ZCard(ctx, key)
	zrangeCmd := pipe.ZRangeWithScores(ctx, key, 0, 0)
	pipe.PExpire(ctx, key, r.window)
	if _, err := pipe.Exec(ctx); err != nil {
		return ConnectDecision{}, err
	}

	count := zcardCmd.Val()
	if count <= r.threshold {
		return ConnectDecision{Allowed: true, Count: count}, nil
	}

	// Blocked: retry = time until the oldest in-window entry ages out.
	// Fallback to full window if ZSet is unexpectedly empty (race with
	// another replica's ZRem — rare, conservative).
	retry := r.window
	oldest := zrangeCmd.Val()
	if len(oldest) > 0 {
		oldestNanos := int64(oldest[0].Score)
		ageoutAt := oldestNanos + r.window.Nanoseconds()
		if d := time.Duration(ageoutAt - nowNanos); d > 0 && d <= r.window {
			retry = d
		}
	}

	return ConnectDecision{Allowed: false, Count: count, RetryAfter: retry}, nil
}
