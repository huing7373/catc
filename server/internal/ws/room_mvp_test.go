package ws

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/clockx"
)

// fakeRoomBroadcaster records every BroadcastToUser invocation so tests can
// assert both target user lists and payload round-trips. Only the ToUser
// path is exercised in this story's scope (see AC4 note on BroadcastToRoom).
type fakeRoomBroadcaster struct {
	mu    sync.Mutex
	sends []broadcastCall
}

type broadcastCall struct {
	userID UserID
	msg    []byte
}

func (b *fakeRoomBroadcaster) BroadcastToUser(_ context.Context, userID UserID, msg []byte) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	cp := make([]byte, len(msg))
	copy(cp, msg)
	b.sends = append(b.sends, broadcastCall{userID, cp})
	return nil
}

func (b *fakeRoomBroadcaster) BroadcastToRoom(_ context.Context, _ RoomID, _ []byte) error {
	return nil
}

func (b *fakeRoomBroadcaster) PushOnConnect(_ context.Context, _ ConnID, _ UserID) error {
	return nil
}

func (b *fakeRoomBroadcaster) BroadcastDiff(_ context.Context, _ UserID, _ []byte) error {
	return nil
}

func (b *fakeRoomBroadcaster) snapshot() []broadcastCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]broadcastCall, len(b.sends))
	copy(out, b.sends)
	return out
}

// newRoomTestClient builds a *Client with just enough state for handler tests
// (connID, userID). Bypasses the real Hub — we never call readPump /
// writePump here.
func newRoomTestClient(connID ConnID, userID UserID) *Client {
	return &Client{
		connID: connID,
		userID: userID,
		send:   make(chan []byte, 16),
		done:   make(chan struct{}),
	}
}

// mustJoin performs a room.join call and fails the test on any error. Returns
// the decoded response.
func mustJoin(t *testing.T, rm *RoomManager, client *Client, roomID string) joinResponse {
	t.Helper()
	payload, err := json.Marshal(joinRequest{RoomID: roomID})
	require.NoError(t, err)
	raw, err := rm.HandleJoin(context.Background(), client, Envelope{
		ID:      "env-" + roomID,
		Type:    "room.join",
		Payload: payload,
	})
	require.NoError(t, err)
	var resp joinResponse
	require.NoError(t, json.Unmarshal(raw, &resp))
	return resp
}

// mustActionUpdate performs action.update and returns the decoded ack (or
// surfaces the error for negative cases).
func mustActionUpdate(t *testing.T, rm *RoomManager, client *Client, action string) (json.RawMessage, error) {
	t.Helper()
	payload, err := json.Marshal(actionUpdateRequest{Action: action})
	require.NoError(t, err)
	return rm.HandleActionUpdate(context.Background(), client, Envelope{
		ID:      "env-act-" + action,
		Type:    "action.update",
		Payload: payload,
	})
}

// fixedRoomManager wires a RoomManager with a fake clock at a known instant
// and a fresh capture broadcaster. Returns all three for assertion access.
func fixedRoomManager(t *testing.T, fixed time.Time) (*RoomManager, *fakeRoomBroadcaster, *clockx.FakeClock) {
	t.Helper()
	br := &fakeRoomBroadcaster{}
	clk := clockx.NewFakeClock(fixed)
	rm := NewRoomManager(clk, br)
	return rm, br, clk
}

func TestRoomManager_JoinRoom_IdempotentSameRoom(t *testing.T) {
	t.Parallel()

	rm, _, _ := fixedRoomManager(t, time.Unix(1700000000, 0).UTC())
	alice := newRoomTestClient("c-a", "alice")

	r1 := mustJoin(t, rm, alice, "room-x")
	assert.Equal(t, "room-x", r1.RoomID)
	assert.Equal(t, []memberSnapshot{}, r1.Members, "first join sees no peers")

	r2 := mustJoin(t, rm, alice, "room-x")
	assert.Equal(t, []memberSnapshot{}, r2.Members, "repeat same-room join stays idempotent")

	rm.mu.RLock()
	defer rm.mu.RUnlock()
	assert.Len(t, rm.rooms, 1, "no phantom room was created")
	assert.Len(t, rm.rooms["room-x"].members, 1, "alice counted exactly once")
}

func TestRoomManager_JoinRoom_SwitchRoomsLeavesOld(t *testing.T) {
	t.Parallel()

	rm, _, _ := fixedRoomManager(t, time.Unix(1700000000, 0).UTC())
	alice := newRoomTestClient("c-a", "alice")

	mustJoin(t, rm, alice, "room-a")
	mustJoin(t, rm, alice, "room-b")

	rm.mu.RLock()
	defer rm.mu.RUnlock()
	_, aliveA := rm.rooms["room-a"]
	assert.False(t, aliveA, "empty room-a must be GC'd when alice switched")
	require.Contains(t, rm.rooms, "room-b")
	assert.Len(t, rm.rooms["room-b"].members, 1)
	assert.Equal(t, RoomID("room-b"), rm.userLoc["alice"])
}

func TestRoomManager_JoinRoom_SnapshotExcludesSelf(t *testing.T) {
	t.Parallel()

	rm, _, _ := fixedRoomManager(t, time.Unix(1700000000, 0).UTC())
	alice := newRoomTestClient("c-a", "alice")
	bob := newRoomTestClient("c-b", "bob")
	carol := newRoomTestClient("c-c", "carol")

	mustJoin(t, rm, alice, "r")
	mustJoin(t, rm, bob, "r")

	resp := mustJoin(t, rm, carol, "r")
	require.Len(t, resp.Members, 2, "carol sees alice + bob, not self")
	peerIDs := []string{resp.Members[0].UserID, resp.Members[1].UserID}
	assert.ElementsMatch(t, []string{"alice", "bob"}, peerIDs)
	for _, m := range resp.Members {
		assert.NotEqual(t, "carol", m.UserID, "self must never appear in snapshot")
	}
}

func TestRoomManager_JoinRoom_EmptyRoomSnapshot(t *testing.T) {
	t.Parallel()

	rm, _, _ := fixedRoomManager(t, time.Unix(1700000000, 0).UTC())
	alice := newRoomTestClient("c-a", "alice")

	payload, err := json.Marshal(joinRequest{RoomID: "solo"})
	require.NoError(t, err)
	raw, err := rm.HandleJoin(context.Background(), alice, Envelope{Payload: payload})
	require.NoError(t, err)

	// Wire-shape check: "members" MUST serialize as [] (array), never null.
	assert.Contains(t, string(raw), `"members":[]`,
		"empty snapshot must marshal as [] for iOS JSONDecoder stability; got=%s", raw)
}

func TestRoomManager_ActionUpdate_BroadcastsToOthers(t *testing.T) {
	t.Parallel()

	fixed := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	rm, br, _ := fixedRoomManager(t, fixed)
	alice := newRoomTestClient("c-a", "alice")
	bob := newRoomTestClient("c-b", "bob")
	carol := newRoomTestClient("c-c", "carol")
	mustJoin(t, rm, alice, "r")
	mustJoin(t, rm, bob, "r")
	mustJoin(t, rm, carol, "r")

	raw, err := mustActionUpdate(t, rm, alice, "walking")
	require.NoError(t, err)
	assert.JSONEq(t, "{}", string(raw), "action.update ack must be empty object")

	sends := br.snapshot()
	require.Len(t, sends, 2, "alice's update must reach bob + carol, not alice")

	gotUsers := []UserID{sends[0].userID, sends[1].userID}
	assert.ElementsMatch(t, []UserID{"bob", "carol"}, gotUsers)

	for _, s := range sends {
		var push Push
		require.NoError(t, json.Unmarshal(s.msg, &push))
		assert.Equal(t, "action.broadcast", push.Type)
		var body actionBroadcastPush
		require.NoError(t, json.Unmarshal(push.Payload, &body))
		assert.Equal(t, "alice", body.UserID)
		assert.Equal(t, "walking", body.Action)
		assert.Equal(t, fixed.UnixMilli(), body.TsMs,
			"ts must be clock.Now().UnixMilli(), not host wall-clock")
	}
}

func TestRoomManager_ActionUpdate_NoRoomReturnsValidationError(t *testing.T) {
	t.Parallel()

	rm, _, _ := fixedRoomManager(t, time.Unix(1700000000, 0).UTC())
	stranger := newRoomTestClient("c-s", "stranger")

	_, err := mustActionUpdate(t, rm, stranger, "idle")
	require.Error(t, err)

	var ae *dto.AppError
	require.True(t, errors.As(err, &ae))
	assert.Equal(t, dto.ErrValidationError.Code, ae.Code)
	assert.Equal(t, "user not in any room", ae.Message)
}

func TestRoomManager_ActionUpdate_LengthValidation(t *testing.T) {
	t.Parallel()

	rm, _, _ := fixedRoomManager(t, time.Unix(1700000000, 0).UTC())
	alice := newRoomTestClient("c-a", "alice")
	mustJoin(t, rm, alice, "r")

	// maxFieldBytes+1 characters (all ASCII = 65 bytes).
	tooLong := strings.Repeat("a", maxFieldBytes+1)
	_, err := mustActionUpdate(t, rm, alice, tooLong)
	require.Error(t, err)
	var ae *dto.AppError
	require.True(t, errors.As(err, &ae))
	assert.Equal(t, dto.ErrValidationError.Code, ae.Code)

	// Empty action is also invalid.
	_, err = mustActionUpdate(t, rm, alice, "")
	require.Error(t, err)
	require.True(t, errors.As(err, &ae))
	assert.Equal(t, dto.ErrValidationError.Code, ae.Code)
}

func TestRoomManager_OnDisconnect_RemovesMember(t *testing.T) {
	t.Parallel()

	rm, _, _ := fixedRoomManager(t, time.Unix(1700000000, 0).UTC())
	alice := newRoomTestClient("c-a", "alice")
	mustJoin(t, rm, alice, "only-room")

	rm.OnDisconnect("c-a", "alice")

	rm.mu.RLock()
	defer rm.mu.RUnlock()
	_, roomAlive := rm.rooms["only-room"]
	assert.False(t, roomAlive, "empty room must be deleted")
	_, userTracked := rm.userLoc["alice"]
	assert.False(t, userTracked, "userLoc must be cleaned")
	_, connTracked := rm.connMap["c-a"]
	assert.False(t, connTracked, "connMap must be cleaned")
}

func TestRoomManager_OnDisconnect_IdempotentForUnknownConn(t *testing.T) {
	t.Parallel()

	rm, _, _ := fixedRoomManager(t, time.Unix(1700000000, 0).UTC())

	// Must not panic; must be a no-op.
	require.NotPanics(t, func() { rm.OnDisconnect("ghost-conn", "ghost-user") })
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	assert.Empty(t, rm.rooms)
	assert.Empty(t, rm.userLoc)
	assert.Empty(t, rm.connMap)
}

func TestRoomManager_Rejoin_SameRoom_StaleDisconnectDoesNotEvict(t *testing.T) {
	t.Parallel()

	rm, _, _ := fixedRoomManager(t, time.Unix(1700000000, 0).UTC())

	// Simulate the reconnect race: alice's first socket joins, then a
	// second socket for the same user re-joins the same room before the
	// first one's OnDisconnect lands. The late disconnect on the stale
	// conn must NOT evict alice from the room her new conn now holds.
	oldClient := newRoomTestClient("conn-old", "alice")
	mustJoin(t, rm, oldClient, "room-x")

	newClient := newRoomTestClient("conn-new", "alice")
	mustJoin(t, rm, newClient, "room-x")

	rm.OnDisconnect("conn-old", "alice")

	rm.mu.RLock()
	defer rm.mu.RUnlock()
	require.Contains(t, rm.rooms, RoomID("room-x"),
		"room must survive the stale disconnect")
	member, ok := rm.rooms["room-x"].members["alice"]
	require.True(t, ok, "alice must still be a member")
	assert.Equal(t, ConnID("conn-new"), member.ConnID,
		"room must reference the new active conn, not the disconnected one")
	assert.Equal(t, RoomID("room-x"), rm.userLoc["alice"])

	_, hasOld := rm.connMap["conn-old"]
	assert.False(t, hasOld, "connMap must not retain the stale conn")
	assert.Equal(t, UserID("alice"), rm.connMap["conn-new"])
}

func TestRoomManager_Rejoin_SwitchRoom_StaleDisconnectDoesNotEvict(t *testing.T) {
	t.Parallel()

	rm, _, _ := fixedRoomManager(t, time.Unix(1700000000, 0).UTC())

	// Same reconnect race but the new conn switches rooms at the same
	// time. The late disconnect on the stale conn must target neither
	// room (old room already GC'd; new room belongs to the new conn).
	oldClient := newRoomTestClient("conn-old", "alice")
	mustJoin(t, rm, oldClient, "room-a")

	newClient := newRoomTestClient("conn-new", "alice")
	mustJoin(t, rm, newClient, "room-b")

	rm.OnDisconnect("conn-old", "alice")

	rm.mu.RLock()
	defer rm.mu.RUnlock()
	_, oldAlive := rm.rooms["room-a"]
	assert.False(t, oldAlive, "empty room-a stays GC'd")
	require.Contains(t, rm.rooms, RoomID("room-b"))
	assert.Contains(t, rm.rooms["room-b"].members, UserID("alice"),
		"alice must still be in room-b under her new conn")
	assert.Equal(t, RoomID("room-b"), rm.userLoc["alice"])

	_, hasOld := rm.connMap["conn-old"]
	assert.False(t, hasOld, "connMap must not retain the stale conn")
}

func TestRoomManager_ConcurrentJoinAndUpdate(t *testing.T) {
	t.Parallel()

	rm, br, _ := fixedRoomManager(t, time.Unix(1700000000, 0).UTC())

	const N = 10
	clients := make([]*Client, N)
	for i := 0; i < N; i++ {
		cid := ConnID("c-" + string(rune('a'+i)))
		uid := UserID("u-" + string(rune('a'+i)))
		clients[i] = newRoomTestClient(cid, uid)
	}

	// First, everybody joins the shared room.
	for _, c := range clients {
		mustJoin(t, rm, c, "party")
	}

	// Then fire concurrent joins + updates. join is a re-join (idempotent)
	// and update fans out; race detector will catch any unsynchronized
	// access to rm's maps.
	var wg sync.WaitGroup
	var updateCount atomic.Int64
	for i := 0; i < N; i++ {
		wg.Add(2)
		c := clients[i]
		go func() {
			defer wg.Done()
			mustJoin(t, rm, c, "party")
		}()
		go func() {
			defer wg.Done()
			_, err := mustActionUpdate(t, rm, c, "a")
			assert.NoError(t, err)
			updateCount.Add(1)
		}()
	}
	wg.Wait()

	assert.Equal(t, int64(N), updateCount.Load())
	// Each of N users fan out to N-1 peers ⇒ N*(N-1) broadcasts.
	assert.Equal(t, N*(N-1), len(br.snapshot()))
}
