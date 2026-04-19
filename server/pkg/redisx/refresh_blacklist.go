package redisx

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/huing/cat/server/pkg/clockx"
)

// RefreshBlacklist stores revoked refresh-token JTIs with a TTL equal
// to each token's remaining validity. Story 1.2 rolling-rotation +
// stolen-token reuse detection writes to it on every legitimate refresh
// (old jti → blacklist) and on every detected replay (current jti →
// blacklist). Story 1.6 account deletion and future per-device
// sign-out consume the same store via AuthService.RevokeAllUserTokens /
// RevokeRefreshToken.
//
// # Key space (architecture §D16, PRD §Redis Key Convention)
//
//	refresh_blacklist:{jti}  →  "1"  (string with TTL)
//
// Namespace strictly isolated from:
//
//   - event:*                  (Story 0.10 WS upstream dedup)
//   - event_result:*           (Story 0.10 WS dispatch cache)
//   - resume_cache:*           (Story 0.12 session.resume snapshot)
//   - lock:cron:*              (Story 0.8 distributed cron lock)
//   - ratelimit:ws:*           (Story 0.11 WS connect rate limit)
//   - blacklist:device:*       (Story 0.11 per-device blacklist)
//   - presence:*               (Epic 4 presence — future)
//   - state:*                  (Epic 2 cat state cache — future)
//   - apns:*                   (Story 0.13 APNs platform)
//
// JTI is a UUID v4 (see ids.NewRefreshJTI) → contains no ':' or '/', so
// single-segment concatenation `refresh_blacklist:<jti>` is injective
// without any length-prefix encoding (review-antipatterns §8.2).
//
// # Fail-closed semantics
//
// Revoke and IsRevoked both surface Redis errors unchanged; the service
// caller MUST treat any error from this store as fail-closed per
// architecture guide §21.3 — a refresh call that cannot verify the
// blacklist state must reject, never "assume clean and succeed".
type RefreshBlacklist struct {
	cmd   redis.Cmdable
	clock clockx.Clock
}

// NewRefreshBlacklist constructs a RefreshBlacklist. Fail-fast on nil
// deps (review-antipatterns §4.1): construction happens once at startup
// in cmd/cat/initialize.go; a nil dependency would turn into a silent
// request-time nil-pointer panic, which is strictly worse than a
// startup log.Fatal.
func NewRefreshBlacklist(cmd redis.Cmdable, clk clockx.Clock) *RefreshBlacklist {
	if cmd == nil {
		panic("redisx.NewRefreshBlacklist: cmd must not be nil")
	}
	if clk == nil {
		panic("redisx.NewRefreshBlacklist: clock must not be nil")
	}
	return &RefreshBlacklist{cmd: cmd, clock: clk}
}

// refreshBlacklistKey builds the Redis key. JTI is a UUID v4 — colon-free
// and path-separator-free — so single-segment concat is injective.
func refreshBlacklistKey(jti string) string {
	return "refresh_blacklist:" + jti
}

// Revoke writes the jti with ttl = exp - clock.Now(). If ttl <= 0 (the
// token is already past its natural expiry) the method is a no-op
// returning nil — there is no point storing a blacklist entry for a
// token the verifier would have refused anyway (saves Redis memory and
// avoids a spurious SET that would linger only briefly).
func (b *RefreshBlacklist) Revoke(ctx context.Context, jti string, exp time.Time) error {
	ttl := exp.Sub(b.clock.Now())
	if ttl <= 0 {
		return nil
	}
	return b.cmd.Set(ctx, refreshBlacklistKey(jti), "1", ttl).Err()
}

// IsRevoked returns (true, nil) when the key exists, (false, nil) when
// absent, and (false, err) when the Redis call fails. Callers MUST
// treat err as fail-closed — architecture guide §21.3: inability to
// verify revocation state is not a license to grant access.
func (b *RefreshBlacklist) IsRevoked(ctx context.Context, jti string) (bool, error) {
	n, err := b.cmd.Exists(ctx, refreshBlacklistKey(jti)).Result()
	if err != nil {
		return false, err
	}
	return n >= 1, nil
}
