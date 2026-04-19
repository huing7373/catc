package redisx

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/pkg/clockx"
)

// fixedClock returns a deterministic wall clock for TTL math assertions.
func fixedClock(t time.Time) clockx.Clock {
	return &constClock{now: t}
}

type constClock struct{ now time.Time }

func (c *constClock) Now() time.Time { return c.now }

// newClosedRedis produces a redis.Cmdable whose connection is already
// closed; any command returns a network error. Used to drive the
// fail-closed code paths without needing a mock framework.
func newClosedRedis(t *testing.T) redis.Cmdable {
	t.Helper()
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	// Shut the backing miniredis down — subsequent commands produce
	// net.OpError "connection refused".
	mr.Close()
	return cli
}

func TestRefreshBlacklist_RevokeThenIsRevokedTrue(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	bl := NewRefreshBlacklist(cmd, fixedClock(now))
	ctx := context.Background()
	jti := "11111111-2222-4333-8444-555555555555"

	require.NoError(t, bl.Revoke(ctx, jti, now.Add(1*time.Hour)))

	revoked, err := bl.IsRevoked(ctx, jti)
	require.NoError(t, err)
	assert.True(t, revoked)
}

func TestRefreshBlacklist_IsRevokedFalseWhenAbsent(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	bl := NewRefreshBlacklist(cmd, clockx.NewRealClock())

	revoked, err := bl.IsRevoked(context.Background(), "never-written-jti")
	require.NoError(t, err)
	assert.False(t, revoked)
}

func TestRefreshBlacklist_RevokeZeroTTLNoop(t *testing.T) {
	t.Parallel()
	mr, cmd := setupMiniredis(t)
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	bl := NewRefreshBlacklist(cmd, fixedClock(now))
	ctx := context.Background()
	jti := "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"

	// exp strictly in the past ⇒ ttl <= 0 ⇒ no-op, no key written
	require.NoError(t, bl.Revoke(ctx, jti, now.Add(-1*time.Second)))
	assert.False(t, mr.Exists(refreshBlacklistKey(jti)),
		"Revoke with non-positive ttl must not write the key")

	// And just to be thorough: exp == now is also zero → no-op
	require.NoError(t, bl.Revoke(ctx, jti, now))
	assert.False(t, mr.Exists(refreshBlacklistKey(jti)))
}

func TestRefreshBlacklist_RevokeRespectsTTL(t *testing.T) {
	t.Parallel()
	mr, cmd := setupMiniredis(t)
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	bl := NewRefreshBlacklist(cmd, fixedClock(now))
	ctx := context.Background()
	jti := "cccccccc-dddd-4eee-8fff-000000000000"

	require.NoError(t, bl.Revoke(ctx, jti, now.Add(1*time.Hour)))

	ttl := mr.TTL(refreshBlacklistKey(jti))
	assert.Greater(t, ttl, time.Duration(0))
	assert.LessOrEqual(t, ttl, 1*time.Hour)
}

func TestRefreshBlacklist_MultipleJTIsIndependent(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	bl := NewRefreshBlacklist(cmd, fixedClock(now))
	ctx := context.Background()
	jtiA := "11111111-1111-4111-8111-111111111111"
	jtiB := "22222222-2222-4222-8222-222222222222"

	require.NoError(t, bl.Revoke(ctx, jtiA, now.Add(1*time.Hour)))

	revokedA, err := bl.IsRevoked(ctx, jtiA)
	require.NoError(t, err)
	assert.True(t, revokedA)

	revokedB, err := bl.IsRevoked(ctx, jtiB)
	require.NoError(t, err)
	assert.False(t, revokedB, "revoking jtiA must not leak into jtiB")
}

func TestRefreshBlacklist_RevokeRedisErrorPropagates(t *testing.T) {
	t.Parallel()
	cmd := newClosedRedis(t)
	now := time.Now().UTC()
	bl := NewRefreshBlacklist(cmd, fixedClock(now))

	err := bl.Revoke(context.Background(), "jti-any", now.Add(1*time.Hour))
	assert.Error(t, err, "Revoke must propagate Redis errors — callers rely on fail-closed")
}

func TestRefreshBlacklist_IsRevokedRedisErrorPropagates(t *testing.T) {
	t.Parallel()
	cmd := newClosedRedis(t)
	bl := NewRefreshBlacklist(cmd, clockx.NewRealClock())

	revoked, err := bl.IsRevoked(context.Background(), "jti-any")
	assert.Error(t, err, "IsRevoked must propagate Redis errors — callers rely on fail-closed")
	assert.False(t, revoked, "error path must return revoked=false (caller treats err as fail-closed, not the bool)")
}

func TestRefreshBlacklist_ConstructorPanicsOnNilDeps(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() {
		NewRefreshBlacklist(nil, clockx.NewRealClock())
	}, "nil cmd must fail-fast at construction")

	_, cmd := setupMiniredis(t)
	assert.Panics(t, func() {
		NewRefreshBlacklist(cmd, nil)
	}, "nil clock must fail-fast at construction")
}
