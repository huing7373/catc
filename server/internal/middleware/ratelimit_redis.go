package middleware

import (
	"context"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// memberSeq breaks ties when multiple Allow calls land in the same
// nanosecond bucket — enough on Linux but trivial to defeat on
// coarser-resolution clocks (Windows). The counter is process-local so
// no inter-process coordination is needed; collisions across processes
// are absorbed by the nano timestamp.
var memberSeq atomic.Uint64

// RedisLimiter implements Limiter on top of a Redis sorted-set sliding
// window. Each request appends a (now, unique-token) member to the set,
// trims members older than the window, then counts the survivors. If
// the survivor count exceeds rate, the request is denied.
//
// Failure mode: Redis unreachable / Lua error → fail-OPEN (Allow=true)
// with a warn log. Auth availability outranks strict throttling for the
// MVP. Operators can detect this via the warn-rate metric.
type RedisLimiter struct {
	rdb        *redis.Client
	bucket     string        // e.g. "auth-login" — passed to RateLimitKey
	windowSec  int           // sliding window size in seconds (per-bucket policy)
	keyTTL     time.Duration // Redis key auto-expire so cold subjects self-clean
	keyBuilder func(bucket, subject string) string
}

// NewRedisLimiter wires the limiter to a Redis client. windowSec defines
// the sliding-window size in seconds. The bucket label appears in
// generated keys and structured logs.
func NewRedisLimiter(rdb *redis.Client, bucket string, windowSec int, keyBuilder func(bucket, subject string) string) *RedisLimiter {
	if windowSec <= 0 {
		windowSec = 60
	}
	if keyBuilder == nil {
		// Defensive default: callers should pass repository.RateLimitKey.
		keyBuilder = func(bucket, subject string) string { return "ratelimit:" + bucket + ":" + subject }
	}
	return &RedisLimiter{
		rdb:        rdb,
		bucket:     bucket,
		windowSec:  windowSec,
		keyTTL:     time.Duration(2*windowSec) * time.Second, // 2× window cushion
		keyBuilder: keyBuilder,
	}
}

// rateLimitScript trims old entries, counts survivors, and only inserts
// the new request when survivors < rate. Returning {allowed, count}
// keeps a round-trip for observability if we want it later.
//
// KEYS[1] = key
// ARGV[1] = now (ms)
// ARGV[2] = window start (ms) = now - windowMs
// ARGV[3] = rate (max requests per window)
// ARGV[4] = unique member (use now + random suffix from caller)
// ARGV[5] = key TTL (seconds)
const rateLimitScript = `
redis.call('ZREMRANGEBYSCORE', KEYS[1], 0, ARGV[2])
local count = redis.call('ZCARD', KEYS[1])
if count >= tonumber(ARGV[3]) then
    return {0, count}
end
redis.call('ZADD', KEYS[1], ARGV[1], ARGV[4])
redis.call('EXPIRE', KEYS[1], ARGV[5])
return {1, count + 1}
`

// luaScript memoises script SHA so go-redis' EvalSha avoids resending
// the script body after the first call.
var luaScript = redis.NewScript(rateLimitScript)

// Allow reports whether the (key, rate) pair is under the threshold.
// burst is currently unused (the sliding window already absorbs short
// bursts up to rate); it is kept in the Limiter signature so handler
// code can document intent.
func (l *RedisLimiter) Allow(ctx context.Context, key string, rate int, burst int) (bool, error) {
	if rate <= 0 {
		return true, nil
	}
	now := time.Now().UnixNano()
	windowStart := now - int64(l.windowSec)*int64(time.Second)

	fullKey := l.keyBuilder(l.bucket, key)
	// Member must be unique per Allow call. Nano timestamp + monotonic
	// counter guarantees no collisions even on Windows' 100ns clock.
	member := strconv.FormatInt(now, 10) + "-" + strconv.FormatUint(memberSeq.Add(1), 10)

	res, err := luaScript.Run(ctx, l.rdb, []string{fullKey},
		now, windowStart, rate, member, int(l.keyTTL.Seconds()),
	).Slice()
	if err != nil {
		// Fail-open. Availability of /v1/auth outweighs strict limiting
		// during a Redis brownout.
		log.Ctx(ctx).Warn().
			Err(err).
			Str("bucket", l.bucket).
			Str("key", fullKey).
			Msg("rate limiter fail-open: redis error")
		return true, nil
	}
	if len(res) == 0 {
		log.Ctx(ctx).Warn().
			Str("bucket", l.bucket).
			Str("key", fullKey).
			Msg("rate limiter fail-open: empty script response")
		return true, nil
	}
	allowed, _ := res[0].(int64)
	return allowed == 1, nil
}
