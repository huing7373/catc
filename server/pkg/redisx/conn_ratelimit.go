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
// timestamps (Unix milliseconds). An earlier INCR+EXPIRE-NX implementation
// was correctly flagged as a fixed window (boundary bypass: 5 connects just
// before expiry + 5 connects immediately after reset → 10 within ~2s).
//
// Milliseconds (not nanoseconds) for the ZSET score: Redis sorted set
// scores are double-precision floats (IEEE 754), which represent integers
// exactly only up to 2^53. Nanosecond Unix timestamps (~1.7e18 today)
// exceed that, so round-tripping a ns score through Redis loses ~hundreds
// of ns of precision — causing the ageoutAt-vs-now math below to miss the
// boundary by a few nanoseconds. Millisecond Unix timestamps (~1.7e12)
// stay safely under 2^53 through the 2100s, so the ZRange score readback
// equals the value that was ZADDed, and d == 0 at the exact boundary is a
// reliable (not rounding-influenced) signal.
//
// Key space (separated from blacklist:device:* / lock:cron:* / event:* per D16):
//
//	ratelimit:ws:{userID}  →  ZSET { member = "<ms>:<uuid>", score = <ms> }
//
// Algorithm (single pipeline — ZADD is atomic, the ZREMRANGEBYSCORE is a
// read-before-write from the same replica's perspective):
//
//  1. ZREMRANGEBYSCORE key -inf (cutoff   → drop entries older than window
//  2. ZADD             key score=now_ms member=ms:uuid
//  3. ZCARD            key                 → count within window
//  4. ZRANGE           key 0 0 WITHSCORES  → oldest in-window entry
//  5. PEXPIRE          key window          → GC dormant users' keys
//
// newCount > threshold → Allowed=false. RetryAfter is computed from the
// oldest in-window entry (ageoutAt − now) so the client waits the *minimum*
// time until a slot frees up, not a full window. A sub-millisecond/boundary
// result is clamped to 1 ms so the upstream ceilSeconds yields a usable
// (≥ 1s) Retry-After header rather than falling through to the full window.
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
	nowMillis := now.UnixMilli()
	cutoff := now.Add(-r.window).UnixMilli()

	// Member must be unique per attempt — two attempts at the same millisecond
	// would otherwise collapse into a single ZSET entry (ZADD is set-valued).
	member := strconv.FormatInt(nowMillis, 10) + ":" + uuid.New().String()

	pipe := r.cmd.Pipeline()
	// Remove entries strictly older than (now - window). Entries at exactly
	// score == cutoff are the just-turned-60s-old boundary — keep them
	// in-window (conservative: reject rather than risk missing a burst).
	pipe.ZRemRangeByScore(ctx, key, "-inf", "("+strconv.FormatInt(cutoff, 10))
	pipe.ZAdd(ctx, key, redis.Z{Score: float64(nowMillis), Member: member})
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
	// Defaults to full window only when the ZSET is unexpectedly empty
	// (theoretically impossible — we just ZADDed ourselves).
	retry := r.window
	oldest := zrangeCmd.Val()
	if len(oldest) > 0 {
		oldestMillis := int64(oldest[0].Score)
		ageoutMillis := oldestMillis + r.window.Milliseconds()
		d := time.Duration(ageoutMillis-nowMillis) * time.Millisecond
		switch {
		case d <= 0:
			// Boundary: oldest retained entry has score == cutoff, so its
			// ageout instant equals now. The old `d > 0` guard here fell
			// through to retry = full window, handing the client a 60s
			// Retry-After at the precise moment the slot would free —
			// review round 2 fix. A 1 ms hint is rounded up to 1 second
			// by ceilSeconds, giving the client a real "try again soon".
			retry = time.Millisecond
		case d <= r.window:
			retry = d
			// default: d > window is impossible (ZRem removed older); keep
			// retry = r.window safety net.
		}
	}

	return ConnectDecision{Allowed: false, Count: count, RetryAfter: retry}, nil
}
