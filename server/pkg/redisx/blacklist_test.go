package redisx

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisBlacklist_AddAndIsBlacklisted(t *testing.T) {
	t.Parallel()
	mr, cmd := setupMiniredis(t)
	bl := NewBlacklist(cmd)
	ctx := context.Background()

	ok, err := bl.IsBlacklisted(ctx, "u1")
	require.NoError(t, err)
	assert.False(t, ok, "fresh user must not be blacklisted")

	require.NoError(t, bl.Add(ctx, "u1", 5*time.Minute))

	ok, err = bl.IsBlacklisted(ctx, "u1")
	require.NoError(t, err)
	assert.True(t, ok)

	ttl := mr.TTL("blacklist:device:u1")
	assert.Greater(t, ttl.Seconds(), 0.0)
	assert.LessOrEqual(t, ttl, 5*time.Minute)
}

func TestRedisBlacklist_TTLExpiry(t *testing.T) {
	t.Parallel()
	mr, cmd := setupMiniredis(t)
	bl := NewBlacklist(cmd)
	ctx := context.Background()

	require.NoError(t, bl.Add(ctx, "u1", 1*time.Minute))

	mr.FastForward(61 * time.Second)

	ok, err := bl.IsBlacklisted(ctx, "u1")
	require.NoError(t, err)
	assert.False(t, ok, "blacklist entry must expire after TTL")
}

func TestRedisBlacklist_Remove(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	bl := NewBlacklist(cmd)
	ctx := context.Background()

	require.NoError(t, bl.Add(ctx, "u1", 5*time.Minute))

	require.NoError(t, bl.Remove(ctx, "u1"))

	ok, err := bl.IsBlacklisted(ctx, "u1")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRedisBlacklist_Remove_NonExistentIsNoOp(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	bl := NewBlacklist(cmd)

	require.NoError(t, bl.Remove(context.Background(), "ghost"))
}

func TestRedisBlacklist_Add_InvalidTTL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ttl  time.Duration
	}{
		{name: "zero ttl rejected", ttl: 0},
		{name: "negative ttl rejected", ttl: -time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, cmd := setupMiniredis(t)
			bl := NewBlacklist(cmd)
			err := bl.Add(context.Background(), "u1", tt.ttl)
			require.Error(t, err)
		})
	}
}

func TestRedisBlacklist_TTL(t *testing.T) {
	t.Parallel()

	t.Run("missing user → exists=false", func(t *testing.T) {
		t.Parallel()
		_, cmd := setupMiniredis(t)
		bl := NewBlacklist(cmd)

		ttl, exists, err := bl.TTL(context.Background(), "ghost")
		require.NoError(t, err)
		assert.False(t, exists)
		assert.Equal(t, time.Duration(0), ttl)
	})

	t.Run("blacklisted user returns positive TTL + exists=true", func(t *testing.T) {
		t.Parallel()
		_, cmd := setupMiniredis(t)
		bl := NewBlacklist(cmd)
		ctx := context.Background()

		require.NoError(t, bl.Add(ctx, "u1", 5*time.Minute))

		ttl, exists, err := bl.TTL(ctx, "u1")
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Greater(t, ttl, time.Duration(0))
		assert.LessOrEqual(t, ttl, 5*time.Minute)
	})
}

func TestRedisBlacklist_KeyFormat(t *testing.T) {
	t.Parallel()
	mr, cmd := setupMiniredis(t)
	bl := NewBlacklist(cmd)

	require.NoError(t, bl.Add(context.Background(), "user-abc", 5*time.Minute))

	assert.True(t, mr.Exists("blacklist:device:user-abc"))
}
