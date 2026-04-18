package redisx

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/pkg/clockx"
)

func newResumeCacheTestFixture(t *testing.T, ttl time.Duration) (*RedisResumeCache, *clockx.FakeClock, func(key string) time.Duration) {
	t.Helper()
	mr, cmd := setupMiniredis(t)
	clock := clockx.NewFakeClock(time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC))
	cache := NewResumeCache(cmd, clock, ttl)
	return cache, clock, mr.TTL
}

func sampleSnapshot() ResumeSnapshot {
	return ResumeSnapshot{
		User:         json.RawMessage(`{"id":"u1","displayName":"Rei"}`),
		Friends:      json.RawMessage(`[{"id":"f1"}]`),
		CatState:     json.RawMessage(`{"state":"idle"}`),
		Skins:        json.RawMessage(`["skin_a","skin_b"]`),
		Blindboxes:   json.RawMessage(`[]`),
		RoomSnapshot: json.RawMessage(`null`),
	}
}

func TestResumeCache_PutGetRoundTrip(t *testing.T) {
	t.Parallel()
	cache, _, _ := newResumeCacheTestFixture(t, 60*time.Second)
	ctx := context.Background()

	require.NoError(t, cache.Put(ctx, "u1", sampleSnapshot()))

	got, found, err := cache.Get(ctx, "u1")
	require.NoError(t, err)
	require.True(t, found)
	assert.JSONEq(t, `{"id":"u1","displayName":"Rei"}`, string(got.User))
	assert.JSONEq(t, `[{"id":"f1"}]`, string(got.Friends))
	assert.JSONEq(t, `{"state":"idle"}`, string(got.CatState))
	assert.JSONEq(t, `["skin_a","skin_b"]`, string(got.Skins))
	assert.JSONEq(t, `[]`, string(got.Blindboxes))
	assert.JSONEq(t, `null`, string(got.RoomSnapshot))
	assert.Empty(t, got.ServerTime, "ServerTime must NOT be persisted — handler stamps it fresh")
}

func TestResumeCache_PutSetsTTL(t *testing.T) {
	t.Parallel()
	cache, _, ttlFn := newResumeCacheTestFixture(t, 60*time.Second)
	ctx := context.Background()

	require.NoError(t, cache.Put(ctx, "u1", sampleSnapshot()))

	ttl := ttlFn("resume_cache:u1")
	assert.Greater(t, ttl.Seconds(), 0.0)
	assert.LessOrEqual(t, ttl, 60*time.Second)
}

func TestResumeCache_ExpiryMakesGetMiss(t *testing.T) {
	t.Parallel()
	cache, _, _ := newResumeCacheTestFixture(t, 60*time.Second)
	ctx := context.Background()

	require.NoError(t, cache.Put(ctx, "u1", sampleSnapshot()))

	// Reach into the shared miniredis via the cmd — FastForward is on the
	// Miniredis struct which setupMiniredis returns as the first value, so
	// this branch re-constructs fixture with explicit access.
	mr, cmd := setupMiniredis(t)
	clock := clockx.NewFakeClock(time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC))
	c := NewResumeCache(cmd, clock, 60*time.Second)
	require.NoError(t, c.Put(ctx, "u1", sampleSnapshot()))

	mr.FastForward(61 * time.Second)

	_, found, err := c.Get(ctx, "u1")
	require.NoError(t, err)
	assert.False(t, found, "after TTL expiry Get must report not found")
}

func TestResumeCache_GetMissReturnsFoundFalse(t *testing.T) {
	t.Parallel()
	cache, _, _ := newResumeCacheTestFixture(t, 60*time.Second)
	ctx := context.Background()

	_, found, err := cache.Get(ctx, "nonexistent")
	require.NoError(t, err)
	assert.False(t, found)
}

func TestResumeCache_Invalidate(t *testing.T) {
	t.Parallel()
	cache, _, _ := newResumeCacheTestFixture(t, 60*time.Second)
	ctx := context.Background()

	require.NoError(t, cache.Put(ctx, "u1", sampleSnapshot()))
	_, found, err := cache.Get(ctx, "u1")
	require.NoError(t, err)
	require.True(t, found)

	require.NoError(t, cache.Invalidate(ctx, "u1"))

	_, found, err = cache.Get(ctx, "u1")
	require.NoError(t, err)
	assert.False(t, found, "Get after Invalidate must miss")
}

func TestResumeCache_InvalidateMissingKeyIsNoError(t *testing.T) {
	t.Parallel()
	cache, _, _ := newResumeCacheTestFixture(t, 60*time.Second)
	ctx := context.Background()

	// No Put first; DEL of a non-existent key must not error.
	require.NoError(t, cache.Invalidate(ctx, "ghost-user"))
}

func TestNewResumeCache_PanicsOnBadArgs(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	clock := clockx.NewFakeClock(time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC))

	assert.PanicsWithValue(t, "redisx.NewResumeCache: clock must not be nil", func() {
		_ = NewResumeCache(cmd, nil, 60*time.Second)
	})
	assert.PanicsWithValue(t, "redisx.NewResumeCache: ttl must be > 0", func() {
		_ = NewResumeCache(cmd, clock, 0)
	})
	assert.PanicsWithValue(t, "redisx.NewResumeCache: ttl must be > 0", func() {
		_ = NewResumeCache(cmd, clock, -1*time.Second)
	})
}

func TestResumeCache_IndependentUsers(t *testing.T) {
	t.Parallel()
	cache, _, _ := newResumeCacheTestFixture(t, 60*time.Second)
	ctx := context.Background()

	a := sampleSnapshot()
	a.User = json.RawMessage(`{"id":"u_alice"}`)
	b := sampleSnapshot()
	b.User = json.RawMessage(`{"id":"u_bob"}`)

	var wg sync.WaitGroup
	wg.Add(2)
	var errA, errB error
	go func() {
		defer wg.Done()
		errA = cache.Put(ctx, "alice", a)
	}()
	go func() {
		defer wg.Done()
		errB = cache.Put(ctx, "bob", b)
	}()
	wg.Wait()
	require.NoError(t, errA)
	require.NoError(t, errB)

	gotA, foundA, err := cache.Get(ctx, "alice")
	require.NoError(t, err)
	require.True(t, foundA)
	assert.JSONEq(t, `{"id":"u_alice"}`, string(gotA.User))

	gotB, foundB, err := cache.Get(ctx, "bob")
	require.NoError(t, err)
	require.True(t, foundB)
	assert.JSONEq(t, `{"id":"u_bob"}`, string(gotB.User))

	// Invalidate alice; bob must remain.
	require.NoError(t, cache.Invalidate(ctx, "alice"))
	_, foundA, err = cache.Get(ctx, "alice")
	require.NoError(t, err)
	assert.False(t, foundA)
	_, foundB, err = cache.Get(ctx, "bob")
	require.NoError(t, err)
	assert.True(t, foundB)
}
