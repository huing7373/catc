package ws

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/redisx"
)

// fakeResumeCache is an in-memory cache suitable for handler unit tests. Each
// operation records its invocation so tests can assert fail-open behaviour
// without poking at internal state. Errors on Get / Put can be injected per-
// test by setting the corresponding err field.
type fakeResumeCache struct {
	mu       sync.Mutex
	store    map[string]redisx.ResumeSnapshot
	getCalls int
	putCalls int
	getErr   error
	putErr   error
}

func newFakeResumeCache() *fakeResumeCache {
	return &fakeResumeCache{store: make(map[string]redisx.ResumeSnapshot)}
}

func (c *fakeResumeCache) Get(_ context.Context, userID string) (redisx.ResumeSnapshot, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.getCalls++
	if c.getErr != nil {
		return redisx.ResumeSnapshot{}, false, c.getErr
	}
	s, ok := c.store[userID]
	return s, ok, nil
}

func (c *fakeResumeCache) Put(_ context.Context, userID string, s redisx.ResumeSnapshot) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.putCalls++
	if c.putErr != nil {
		return c.putErr
	}
	c.store[userID] = s
	return nil
}

func (c *fakeResumeCache) invalidate(userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.store, userID)
}

// counterProvider wraps a payload-generating function with a call counter.
type counterProvider struct {
	mu    sync.Mutex
	calls int
	fn    func() (json.RawMessage, error)
}

func newCounterProvider(payload string) *counterProvider {
	raw := json.RawMessage(payload)
	return &counterProvider{fn: func() (json.RawMessage, error) { return raw, nil }}
}

func newErrProvider(err error) *counterProvider {
	return &counterProvider{fn: func() (json.RawMessage, error) { return nil, err }}
}

func (p *counterProvider) invoke() (json.RawMessage, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	return p.fn()
}

func (p *counterProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

// Adapter types so a single counterProvider can stand in for any of the six
// provider interfaces (UserProvider / FriendsProvider / ... each has a
// different method name so we need lightweight wrappers).
type cpUser struct{ *counterProvider }
type cpFriends struct{ *counterProvider }
type cpCatState struct{ *counterProvider }
type cpSkins struct{ *counterProvider }
type cpBlindboxes struct{ *counterProvider }
type cpRoom struct{ *counterProvider }

func (p cpUser) GetUser(context.Context, string) (json.RawMessage, error) {
	return p.invoke()
}
func (p cpFriends) ListFriends(context.Context, string) (json.RawMessage, error) {
	return p.invoke()
}
func (p cpCatState) GetCatState(context.Context, string) (json.RawMessage, error) {
	return p.invoke()
}
func (p cpSkins) ListUnlocked(context.Context, string) (json.RawMessage, error) {
	return p.invoke()
}
func (p cpBlindboxes) ListActive(context.Context, string) (json.RawMessage, error) {
	return p.invoke()
}
func (p cpRoom) GetRoomSnapshot(context.Context, string) (json.RawMessage, error) {
	return p.invoke()
}

type handlerFixture struct {
	cache    *fakeResumeCache
	clock    *clockx.FakeClock
	handler  *SessionResumeHandler
	client   *Client
	userP    *counterProvider
	friendsP *counterProvider
	catP     *counterProvider
	skinsP   *counterProvider
	boxP     *counterProvider
	roomP    *counterProvider
}

func newHandlerFixture(t *testing.T) *handlerFixture {
	t.Helper()
	cache := newFakeResumeCache()
	clock := clockx.NewFakeClock(time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC))
	user := newCounterProvider(`{"id":"u1","displayName":"Rei"}`)
	friends := newCounterProvider(`[]`)
	cat := newCounterProvider(`{"state":"idle"}`)
	skins := newCounterProvider(`[]`)
	boxes := newCounterProvider(`[]`)
	room := newCounterProvider(`null`)
	h := NewSessionResumeHandler(cache, clock, ResumeProviders{
		User:         cpUser{user},
		Friends:      cpFriends{friends},
		CatState:     cpCatState{cat},
		Skins:        cpSkins{skins},
		Blindboxes:   cpBlindboxes{boxes},
		RoomSnapshot: cpRoom{room},
	})
	return &handlerFixture{
		cache:    cache,
		clock:    clock,
		handler:  h,
		client:   newTestClient(),
		userP:    user,
		friendsP: friends,
		catP:     cat,
		skinsP:   skins,
		boxP:     boxes,
		roomP:    room,
	}
}

func (f *handlerFixture) providerCalls() (user, friends, cat, skins, boxes, room int) {
	return f.userP.callCount(), f.friendsP.callCount(), f.catP.callCount(),
		f.skinsP.callCount(), f.boxP.callCount(), f.roomP.callCount()
}

func decodeSnapshot(t *testing.T, raw json.RawMessage) ResumeSnapshot {
	t.Helper()
	var s ResumeSnapshot
	require.NoError(t, json.Unmarshal(raw, &s))
	return s
}

func TestSessionResumeHandler_FirstResumePopulatesCacheAndProviders(t *testing.T) {
	t.Parallel()

	f := newHandlerFixture(t)

	raw, err := f.handler.Handle(context.Background(), f.client, Envelope{ID: "req-1", Type: "session.resume"})
	require.NoError(t, err)

	u, fr, cat, sk, bb, ro := f.providerCalls()
	assert.Equal(t, 1, u)
	assert.Equal(t, 1, fr)
	assert.Equal(t, 1, cat)
	assert.Equal(t, 1, sk)
	assert.Equal(t, 1, bb)
	assert.Equal(t, 1, ro)
	assert.Equal(t, 1, f.cache.putCalls)
	assert.Contains(t, f.cache.store, "u1")

	snap := decodeSnapshot(t, raw)
	assert.JSONEq(t, `{"id":"u1","displayName":"Rei"}`, string(snap.User))
	assert.JSONEq(t, `[]`, string(snap.Friends))
	assert.JSONEq(t, `{"state":"idle"}`, string(snap.CatState))
	assert.Equal(t, f.clock.Now().UTC().Format(time.RFC3339Nano), snap.ServerTime)
}

func TestSessionResumeHandler_CacheHitSkipsProviders(t *testing.T) {
	t.Parallel()

	f := newHandlerFixture(t)

	// Populate cache via first call.
	_, err := f.handler.Handle(context.Background(), f.client, Envelope{ID: "req-1", Type: "session.resume"})
	require.NoError(t, err)
	u0, fr0, c0, s0, b0, r0 := f.providerCalls()

	// Advance clock to verify ServerTime is regenerated, not cached.
	f.clock.Advance(5 * time.Second)

	raw, err := f.handler.Handle(context.Background(), f.client, Envelope{ID: "req-2", Type: "session.resume"})
	require.NoError(t, err)

	u1, fr1, c1, s1, b1, r1 := f.providerCalls()
	assert.Equal(t, u0, u1, "UserProvider should not be called on cache hit")
	assert.Equal(t, fr0, fr1)
	assert.Equal(t, c0, c1)
	assert.Equal(t, s0, s1)
	assert.Equal(t, b0, b1)
	assert.Equal(t, r0, r1)
	assert.Equal(t, 1, f.cache.putCalls, "Put should not be called again on cache hit")

	snap := decodeSnapshot(t, raw)
	assert.Equal(t, f.clock.Now().UTC().Format(time.RFC3339Nano), snap.ServerTime,
		"ServerTime must reflect the current clock, not the cached snapshot's write time")
}

func TestSessionResumeHandler_InvalidateCausesRefetch(t *testing.T) {
	t.Parallel()

	f := newHandlerFixture(t)

	_, err := f.handler.Handle(context.Background(), f.client, Envelope{ID: "req-1", Type: "session.resume"})
	require.NoError(t, err)

	f.cache.invalidate("u1")

	_, err = f.handler.Handle(context.Background(), f.client, Envelope{ID: "req-2", Type: "session.resume"})
	require.NoError(t, err)

	u, fr, cat, sk, bb, ro := f.providerCalls()
	assert.Equal(t, 2, u)
	assert.Equal(t, 2, fr)
	assert.Equal(t, 2, cat)
	assert.Equal(t, 2, sk)
	assert.Equal(t, 2, bb)
	assert.Equal(t, 2, ro)
	assert.Equal(t, 2, f.cache.putCalls)
}

func TestSessionResumeHandler_CacheGetErrorFailsOpen(t *testing.T) {
	t.Parallel()

	f := newHandlerFixture(t)
	f.cache.getErr = errors.New("redis down")

	raw, err := f.handler.Handle(context.Background(), f.client, Envelope{ID: "req-1", Type: "session.resume"})
	require.NoError(t, err, "cache Get error must NOT propagate — fail-open to providers")

	u, _, _, _, _, _ := f.providerCalls()
	assert.Equal(t, 1, u, "providers must still be invoked when cache Get fails")

	snap := decodeSnapshot(t, raw)
	assert.JSONEq(t, `{"id":"u1","displayName":"Rei"}`, string(snap.User))
}

func TestSessionResumeHandler_CachePutErrorFailsOpen(t *testing.T) {
	t.Parallel()

	f := newHandlerFixture(t)
	f.cache.putErr = errors.New("redis write failed")

	raw, err := f.handler.Handle(context.Background(), f.client, Envelope{ID: "req-1", Type: "session.resume"})
	require.NoError(t, err, "cache Put error must NOT propagate — the response is already built")

	snap := decodeSnapshot(t, raw)
	assert.JSONEq(t, `{"id":"u1","displayName":"Rei"}`, string(snap.User))
}

func TestSessionResumeHandler_ProviderErrorFailsClosed(t *testing.T) {
	t.Parallel()

	cache := newFakeResumeCache()
	clock := clockx.NewFakeClock(time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC))
	friendsErr := errors.New("mongo timeout")
	h := NewSessionResumeHandler(cache, clock, ResumeProviders{
		User:         cpUser{newCounterProvider(`null`)},
		Friends:      cpFriends{newErrProvider(friendsErr)},
		CatState:     cpCatState{newCounterProvider(`null`)},
		Skins:        cpSkins{newCounterProvider(`[]`)},
		Blindboxes:   cpBlindboxes{newCounterProvider(`[]`)},
		RoomSnapshot: cpRoom{newCounterProvider(`null`)},
	})
	client := newTestClient()

	_, err := h.Handle(context.Background(), client, Envelope{ID: "req-1", Type: "session.resume"})
	require.Error(t, err, "provider errors must propagate — a partial payload would mislead the client")

	var ae *dto.AppError
	require.ErrorAs(t, err, &ae)
	assert.Equal(t, "INTERNAL_ERROR", ae.Code)
	assert.ErrorIs(t, err, friendsErr, "original cause must be preserved via WithCause")
	assert.Equal(t, 0, cache.putCalls, "cache must not be populated when provider errored")
}

func TestSessionResumeHandler_EmptyProvidersProduceValidJSON(t *testing.T) {
	t.Parallel()

	cache := newFakeResumeCache()
	clock := clockx.NewFakeClock(time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC))
	h := NewSessionResumeHandler(cache, clock, ResumeProviders{
		User:         EmptyUserProvider{},
		Friends:      EmptyFriendsProvider{},
		CatState:     EmptyCatStateProvider{},
		Skins:        EmptySkinsProvider{},
		Blindboxes:   EmptyBlindboxesProvider{},
		RoomSnapshot: EmptyRoomSnapshotProvider{},
	})
	client := newTestClient()

	raw, err := h.Handle(context.Background(), client, Envelope{ID: "req-1", Type: "session.resume"})
	require.NoError(t, err)

	snap := decodeSnapshot(t, raw)
	assert.JSONEq(t, `null`, string(snap.User))
	assert.JSONEq(t, `[]`, string(snap.Friends))
	assert.JSONEq(t, `null`, string(snap.CatState))
	assert.JSONEq(t, `[]`, string(snap.Skins))
	assert.JSONEq(t, `[]`, string(snap.Blindboxes))
	assert.JSONEq(t, `null`, string(snap.RoomSnapshot))
	assert.NotEmpty(t, snap.ServerTime)
}

// TestSessionResumeHandler_CoalescesConcurrentMisses covers the review-round-1
// finding: without singleflight, N simultaneous cache misses for the same user
// each ran every provider, turning the J4 reconnect storm into N × 6 upstream
// reads. This test gates a blocking provider, fires several concurrent
// clients, and asserts every provider ran exactly once.
func TestSessionResumeHandler_CoalescesConcurrentMisses(t *testing.T) {
	t.Parallel()

	gate := make(chan struct{})
	var userCalls atomic.Int64
	user := &blockingUserProvider{gate: gate, calls: &userCalls}

	cache := newFakeResumeCache()
	clock := clockx.NewFakeClock(time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC))
	friends := newCounterProvider(`[]`)
	cat := newCounterProvider(`null`)
	skins := newCounterProvider(`[]`)
	boxes := newCounterProvider(`[]`)
	room := newCounterProvider(`null`)
	h := NewSessionResumeHandler(cache, clock, ResumeProviders{
		User:         user,
		Friends:      cpFriends{friends},
		CatState:     cpCatState{cat},
		Skins:        cpSkins{skins},
		Blindboxes:   cpBlindboxes{boxes},
		RoomSnapshot: cpRoom{room},
	})

	const concurrent = 5
	results := make(chan error, concurrent)
	// Each client gets its own Client (distinct connID) but identical userID
	// — the coalescing key.
	for i := 0; i < concurrent; i++ {
		go func() {
			client := newTestClient()
			_, err := h.Handle(context.Background(), client, Envelope{ID: "e", Type: "session.resume"})
			results <- err
		}()
	}

	// Give goroutines a moment to reach the singleflight barrier. In a
	// correctly coalesced implementation the first goroutine is blocked in
	// GetUser and the rest are queued behind singleflight; in a broken
	// implementation all N are independently blocked on GetUser.
	time.Sleep(20 * time.Millisecond)

	// Release the provider. If singleflight is in place, GetUser returns once
	// and every waiter gets the same snapshot. If it's missing, N−1
	// additional GetUser invocations would be racing on `gate` (already
	// closed), so the final callCount would be N.
	close(gate)

	for i := 0; i < concurrent; i++ {
		select {
		case err := <-results:
			require.NoError(t, err)
		case <-time.After(2 * time.Second):
			t.Fatal("handler did not complete within 2s — likely deadlocked")
		}
	}

	assert.Equal(t, int64(1), userCalls.Load(),
		"UserProvider must run exactly once across %d concurrent misses", concurrent)
	assert.Equal(t, 1, friends.callCount())
	assert.Equal(t, 1, cat.callCount())
	assert.Equal(t, 1, skins.callCount())
	assert.Equal(t, 1, boxes.callCount())
	assert.Equal(t, 1, room.callCount())
}

// blockingUserProvider stalls on GetUser until gate is closed. Lets the test
// hold the first in-flight build open while additional goroutines pile up on
// the singleflight barrier.
type blockingUserProvider struct {
	gate  <-chan struct{}
	calls *atomic.Int64
}

func (p *blockingUserProvider) GetUser(ctx context.Context, _ string) (json.RawMessage, error) {
	p.calls.Add(1)
	select {
	case <-p.gate:
		return json.RawMessage(`{"id":"u1"}`), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestNewSessionResumeHandler_PanicsOnNilDependency(t *testing.T) {
	t.Parallel()

	cache := newFakeResumeCache()
	clock := clockx.NewFakeClock(time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC))
	allProviders := ResumeProviders{
		User:         EmptyUserProvider{},
		Friends:      EmptyFriendsProvider{},
		CatState:     EmptyCatStateProvider{},
		Skins:        EmptySkinsProvider{},
		Blindboxes:   EmptyBlindboxesProvider{},
		RoomSnapshot: EmptyRoomSnapshotProvider{},
	}

	// Baseline sanity: all-non-nil must succeed.
	require.NotPanics(t, func() {
		_ = NewSessionResumeHandler(cache, clock, allProviders)
	})

	cases := []struct {
		name    string
		cache   ResumeCache
		clock   clockx.Clock
		provs   ResumeProviders
		wantSub string
	}{
		{"nil cache", nil, clock, allProviders, "cache must not be nil"},
		{"nil clock", cache, nil, allProviders, "clock must not be nil"},
		{"nil user", cache, clock, func() ResumeProviders { p := allProviders; p.User = nil; return p }(), "providers.User"},
		{"nil friends", cache, clock, func() ResumeProviders { p := allProviders; p.Friends = nil; return p }(), "providers.Friends"},
		{"nil catState", cache, clock, func() ResumeProviders { p := allProviders; p.CatState = nil; return p }(), "providers.CatState"},
		{"nil skins", cache, clock, func() ResumeProviders { p := allProviders; p.Skins = nil; return p }(), "providers.Skins"},
		{"nil blindboxes", cache, clock, func() ResumeProviders { p := allProviders; p.Blindboxes = nil; return p }(), "providers.Blindboxes"},
		{"nil room", cache, clock, func() ResumeProviders { p := allProviders; p.RoomSnapshot = nil; return p }(), "providers.RoomSnapshot"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.PanicsWithValue(t, panicMessage(tc.wantSub), func() {
				_ = NewSessionResumeHandler(tc.cache, tc.clock, tc.provs)
			})
		})
	}
}

// panicMessage reconstructs the exact panic message emitted by
// NewSessionResumeHandler for a given key substring. Keeps the test assertions
// aligned with the production messages without forcing every test case to
// know the full prefix.
func panicMessage(sub string) string {
	switch sub {
	case "cache must not be nil":
		return "ws.NewSessionResumeHandler: cache must not be nil"
	case "clock must not be nil":
		return "ws.NewSessionResumeHandler: clock must not be nil"
	case "providers.User":
		return "ws.NewSessionResumeHandler: providers.User must not be nil"
	case "providers.Friends":
		return "ws.NewSessionResumeHandler: providers.Friends must not be nil"
	case "providers.CatState":
		return "ws.NewSessionResumeHandler: providers.CatState must not be nil"
	case "providers.Skins":
		return "ws.NewSessionResumeHandler: providers.Skins must not be nil"
	case "providers.Blindboxes":
		return "ws.NewSessionResumeHandler: providers.Blindboxes must not be nil"
	case "providers.RoomSnapshot":
		return "ws.NewSessionResumeHandler: providers.RoomSnapshot must not be nil"
	default:
		return sub
	}
}
