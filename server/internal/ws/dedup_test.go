package ws

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/clockx"
)

type fakeDedupStore struct {
	mu         sync.Mutex
	acquired   map[string]bool
	results    map[string]DedupResult
	acquireErr error
	getErr     error
	storeErr   error
}

func newFakeDedupStore() *fakeDedupStore {
	return &fakeDedupStore{
		acquired: make(map[string]bool),
		results:  make(map[string]DedupResult),
	}
}

func (f *fakeDedupStore) Acquire(_ context.Context, id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.acquireErr != nil {
		return false, f.acquireErr
	}
	if f.acquired[id] {
		return false, nil
	}
	f.acquired[id] = true
	return true, nil
}

func (f *fakeDedupStore) StoreResult(_ context.Context, id string, r DedupResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.storeErr != nil {
		return f.storeErr
	}
	f.results[id] = r
	return nil
}

func (f *fakeDedupStore) GetResult(_ context.Context, id string) (DedupResult, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return DedupResult{}, false, f.getErr
	}
	r, ok := f.results[id]
	return r, ok, nil
}

func testEnvelope(id, typ string, payload string) Envelope {
	env := Envelope{ID: id, Type: typ}
	if payload != "" {
		env.Payload = json.RawMessage(payload)
	}
	return env
}

func TestDedupMiddleware_FirstSuccess(t *testing.T) {
	t.Parallel()
	store := newFakeDedupStore()
	called := 0
	fn := func(_ context.Context, _ *Client, env Envelope) (json.RawMessage, error) {
		called++
		return json.RawMessage(`{"ok":1}`), nil
	}
	mw := dedupMiddleware(store, clockx.NewRealClock(), fn)

	payload, err := mw(context.Background(), newTestClient(), testEnvelope("e1", "t", `{"n":1}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"ok":1}`, string(payload))
	assert.Equal(t, 1, called)

	stored := store.results["u1:t:e1"]
	assert.True(t, stored.OK)
	assert.JSONEq(t, `{"ok":1}`, string(stored.Payload))
}

func TestDedupMiddleware_FirstAppError(t *testing.T) {
	t.Parallel()
	store := newFakeDedupStore()
	fn := func(_ context.Context, _ *Client, _ Envelope) (json.RawMessage, error) {
		return nil, dto.ErrFriendBlocked
	}
	mw := dedupMiddleware(store, clockx.NewRealClock(), fn)

	_, err := mw(context.Background(), newTestClient(), testEnvelope("e2", "t", ""))
	require.Error(t, err)
	var ae *dto.AppError
	require.True(t, errors.As(err, &ae))
	assert.Equal(t, "FRIEND_BLOCKED", ae.Code)

	stored := store.results["u1:t:e2"]
	assert.False(t, stored.OK)
	assert.Equal(t, "FRIEND_BLOCKED", stored.ErrorCode)
	assert.Equal(t, "user is blocked", stored.ErrorMessage)
}

func TestDedupMiddleware_FirstPanic(t *testing.T) {
	t.Parallel()
	store := newFakeDedupStore()
	fn := func(_ context.Context, _ *Client, _ Envelope) (json.RawMessage, error) {
		panic("boom")
	}
	mw := dedupMiddleware(store, clockx.NewRealClock(), fn)

	defer func() {
		r := recover()
		require.NotNil(t, r, "middleware must re-raise panic so readPump recovery logs it")

		stored := store.results["u1:t:e3"]
		assert.False(t, stored.OK)
		assert.Equal(t, "INTERNAL_ERROR", stored.ErrorCode)
		assert.Contains(t, stored.ErrorMessage, "boom")
	}()

	_, _ = mw(context.Background(), newTestClient(), testEnvelope("e3", "t", ""))
	t.Fatal("panic did not propagate")
}

func TestDedupMiddleware_ReplayCachedSuccess(t *testing.T) {
	t.Parallel()
	store := newFakeDedupStore()
	store.acquired["u1:t:e4"] = true
	store.results["u1:t:e4"] = DedupResult{OK: true, Payload: json.RawMessage(`{"n":1}`)}

	called := 0
	fn := func(_ context.Context, _ *Client, _ Envelope) (json.RawMessage, error) {
		called++
		return nil, nil
	}
	mw := dedupMiddleware(store, clockx.NewRealClock(), fn)

	payload, err := mw(context.Background(), newTestClient(), testEnvelope("e4", "t", `{"n":2}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"n":1}`, string(payload), "replay must return cached payload, not current")
	assert.Equal(t, 0, called, "handler must not be invoked on replay")
}

func TestDedupMiddleware_ReplayCachedFailure(t *testing.T) {
	t.Parallel()
	store := newFakeDedupStore()
	store.acquired["u1:t:e5"] = true
	store.results["u1:t:e5"] = DedupResult{OK: false, ErrorCode: "FRIEND_BLOCKED", ErrorMessage: "user is blocked"}

	mw := dedupMiddleware(store, clockx.NewRealClock(), func(_ context.Context, _ *Client, _ Envelope) (json.RawMessage, error) {
		t.Fatal("handler must not be called")
		return nil, nil
	})

	_, err := mw(context.Background(), newTestClient(), testEnvelope("e5", "t", ""))
	require.Error(t, err)
	var ae *dto.AppError
	require.True(t, errors.As(err, &ae))
	assert.Equal(t, "FRIEND_BLOCKED", ae.Code)
	assert.Equal(t, "user is blocked", ae.Message)
}

func TestDedupMiddleware_ReplayNotFoundReturnsEventProcessing(t *testing.T) {
	t.Parallel()
	store := newFakeDedupStore()
	store.acquired["u1:t:e6"] = true // already claimed, no result yet

	mw := dedupMiddleware(store, clockx.NewRealClock(), func(_ context.Context, _ *Client, _ Envelope) (json.RawMessage, error) {
		t.Fatal("handler must not be called")
		return nil, nil
	})

	_, err := mw(context.Background(), newTestClient(), testEnvelope("e6", "t", ""))
	require.Error(t, err)
	var ae *dto.AppError
	require.True(t, errors.As(err, &ae))
	assert.Equal(t, "EVENT_PROCESSING", ae.Code)
}

func TestDedupMiddleware_EmptyEnvelopeID(t *testing.T) {
	t.Parallel()
	store := newFakeDedupStore()

	mw := dedupMiddleware(store, clockx.NewRealClock(), func(_ context.Context, _ *Client, _ Envelope) (json.RawMessage, error) {
		t.Fatal("handler must not be called")
		return nil, nil
	})

	_, err := mw(context.Background(), newTestClient(), testEnvelope("", "t", ""))
	require.Error(t, err)
	var ae *dto.AppError
	require.True(t, errors.As(err, &ae))
	assert.Equal(t, "VALIDATION_ERROR", ae.Code)
	assert.Equal(t, "envelope.id required", ae.Message)

	assert.Empty(t, store.acquired, "Acquire must not be touched for empty envelope.id")
}

// TestDedupMiddleware_KeyScopedByUserAndType proves the storage key is
// namespaced by (userId, msgType) so different users — or the same user on
// different authoritative-write RPCs — never collide on short client-generated
// IDs such as "1", "2", "e1". All three calls below share env.ID="1" but must
// each run the handler exactly once.
func TestDedupMiddleware_KeyScopedByUserAndType(t *testing.T) {
	t.Parallel()
	store := newFakeDedupStore()

	called := 0
	mw := dedupMiddleware(store, clockx.NewRealClock(), func(_ context.Context, _ *Client, _ Envelope) (json.RawMessage, error) {
		called++
		return json.RawMessage(`{}`), nil
	})

	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())
	userA := &Client{connID: "c1", userID: "userA", send: make(chan []byte, 16), done: make(chan struct{}), hub: hub}
	userB := &Client{connID: "c2", userID: "userB", send: make(chan []byte, 16), done: make(chan struct{}), hub: hub}
	hub.Register(userA)
	hub.Register(userB)

	// Same raw event.ID="1" used three times. Two different users on the
	// same msgType must not dedupe; the same user on a different msgType must
	// not dedupe either.
	_, err := mw(context.Background(), userA, testEnvelope("1", "blindbox.redeem", `{}`))
	require.NoError(t, err)
	_, err = mw(context.Background(), userB, testEnvelope("1", "blindbox.redeem", `{}`))
	require.NoError(t, err)
	_, err = mw(context.Background(), userA, testEnvelope("1", "touch.send", `{}`))
	require.NoError(t, err)

	assert.Equal(t, 3, called, "handler must run once per (user, msgType, eventId) tuple")
	assert.Len(t, store.results, 3)
	assert.Contains(t, store.results, "userA:blindbox.redeem:1")
	assert.Contains(t, store.results, "userB:blindbox.redeem:1")
	assert.Contains(t, store.results, "userA:touch.send:1")

	// Same user + same type + same ID must dedupe.
	_, err = mw(context.Background(), userA, testEnvelope("1", "blindbox.redeem", `{}`))
	require.NoError(t, err)
	assert.Equal(t, 3, called, "true replay must not re-run handler")
}
