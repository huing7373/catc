package redisx

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDedupStore_PanicsOnZeroTTL(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	assert.Panics(t, func() { _ = NewDedupStore(cmd, 0) })
	assert.Panics(t, func() { _ = NewDedupStore(cmd, -time.Second) })
}

func TestRedisDedupStore_AcquireFirstThenDuplicate(t *testing.T) {
	t.Parallel()
	mr, cmd := setupMiniredis(t)
	store := NewDedupStore(cmd, 5*time.Minute)

	ok, err := store.Acquire(context.Background(), "e1")
	require.NoError(t, err)
	assert.True(t, ok, "first Acquire must succeed")

	ok, err = store.Acquire(context.Background(), "e1")
	require.NoError(t, err)
	assert.False(t, ok, "second Acquire on same id must return false")

	ttl := mr.TTL("event:e1")
	assert.Greater(t, ttl.Seconds(), 0.0, "TTL should be set")
	assert.LessOrEqual(t, ttl, 5*time.Minute)
}

func TestRedisDedupStore_StoreAndGetResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result DedupResult
	}{
		{
			name:   "success with payload",
			result: DedupResult{OK: true, Payload: json.RawMessage(`{"n":1}`)},
		},
		{
			name:   "success with empty payload",
			result: DedupResult{OK: true},
		},
		{
			name:   "app error result",
			result: DedupResult{OK: false, ErrorCode: "FRIEND_BLOCKED", ErrorMessage: "user is blocked"},
		},
		{
			name:   "internal error result",
			result: DedupResult{OK: false, ErrorCode: "INTERNAL_ERROR", ErrorMessage: "boom"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mr, cmd := setupMiniredis(t)
			store := NewDedupStore(cmd, 5*time.Minute)
			ctx := context.Background()

			ok, err := store.Acquire(ctx, "x")
			require.NoError(t, err)
			require.True(t, ok)

			require.NoError(t, store.StoreResult(ctx, "x", tt.result))

			got, found, err := store.GetResult(ctx, "x")
			require.NoError(t, err)
			require.True(t, found)
			assert.Equal(t, tt.result.OK, got.OK)
			assert.Equal(t, tt.result.ErrorCode, got.ErrorCode)
			assert.Equal(t, tt.result.ErrorMessage, got.ErrorMessage)
			if len(tt.result.Payload) == 0 {
				assert.Empty(t, got.Payload)
			} else {
				assert.JSONEq(t, string(tt.result.Payload), string(got.Payload))
			}

			// event marker must be flipped to "done".
			doneVal, getErr := mr.Get("event:x")
			require.NoError(t, getErr)
			assert.Equal(t, "done", doneVal)

			// both keys must carry the configured TTL.
			assert.Greater(t, mr.TTL("event:x").Seconds(), 0.0)
			assert.Greater(t, mr.TTL("event_result:x").Seconds(), 0.0)
		})
	}
}

func TestRedisDedupStore_GetResult_NotFound(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	store := NewDedupStore(cmd, 5*time.Minute)

	got, found, err := store.GetResult(context.Background(), "nope")
	require.NoError(t, err)
	assert.False(t, found)
	assert.Equal(t, DedupResult{}, got)
}

func TestRedisDedupStore_TTLExpiry(t *testing.T) {
	t.Parallel()
	mr, cmd := setupMiniredis(t)
	store := NewDedupStore(cmd, 5*time.Minute)
	ctx := context.Background()

	ok, err := store.Acquire(ctx, "exp")
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, store.StoreResult(ctx, "exp", DedupResult{OK: true, Payload: json.RawMessage(`{}`)}))

	mr.FastForward(10 * time.Minute)

	_, found, err := store.GetResult(ctx, "exp")
	require.NoError(t, err)
	assert.False(t, found, "result should be gone after TTL")

	ok, err = store.Acquire(ctx, "exp")
	require.NoError(t, err)
	assert.True(t, ok, "Acquire should succeed again after TTL expiry")
}

func TestDedupResult_HashRoundTrip(t *testing.T) {
	t.Parallel()

	cases := []DedupResult{
		{OK: true, Payload: json.RawMessage(`{"a":1}`)},
		{OK: true},
		{OK: false, ErrorCode: "X", ErrorMessage: "y"},
	}
	for _, c := range cases {
		h := c.ToHash()
		got := DedupResultFromHash(h)
		assert.Equal(t, c.OK, got.OK)
		assert.Equal(t, c.ErrorCode, got.ErrorCode)
		assert.Equal(t, c.ErrorMessage, got.ErrorMessage)
		if len(c.Payload) == 0 {
			assert.Empty(t, got.Payload)
		} else {
			assert.JSONEq(t, string(c.Payload), string(got.Payload))
		}
	}
}
