package ws

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/pkg/clockx"
)

// dialAndRegister opens a real WS loopback connection against the
// per-test httptest server and returns the server-side *Client (which
// has been registered into hub) plus the client-side *websocket.Conn.
//
// Real conns are needed because Hub.DisconnectUser writes a close
// frame via c.conn.WriteControl and unregisterClient calls
// c.conn.Close — bare Client{} fixtures with nil conn would nil-panic.
// Using net.Pipe is not enough because gorilla/websocket's NewConn is
// unexported (v1.5.3 conn.go:285); a real httptest server is the
// shortest path to a *Conn that won't blow up on these calls.
func dialAndRegister(t *testing.T, srv *httptest.Server, hub *Hub, connID ConnID, userID UserID) (*Client, *websocket.Conn) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http")
	dialed, _, err := websocket.DefaultDialer.Dial(url, nil)
	require.NoError(t, err, "ws dial failed for %s/%s", connID, userID)

	// The handshake handler stashes the server-side *websocket.Conn
	// into the channel; pull it out and wrap into a *Client. We do
	// NOT start read/write pumps — DisconnectUser only needs the
	// conn for WriteControl + Close.
	serverConn := <-srv.Config.Handler.(*disconnectTestHandler).conns

	client := &Client{
		connID: connID,
		userID: userID,
		conn:   serverConn,
		send:   make(chan []byte, 16),
		done:   make(chan struct{}),
	}
	hub.Register(client)
	return client, dialed
}

// disconnectTestHandler upgrades incoming HTTP connections to
// WebSocket and pushes the server-side *Conn into conns so the test
// can register it into a Hub manually. Avoids running the production
// UpgradeHandler — these tests are about Hub.DisconnectUser, not WS
// upgrade authentication.
type disconnectTestHandler struct {
	upgrader websocket.Upgrader
	conns    chan *websocket.Conn
}

func (h *disconnectTestHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	h.conns <- c
}

func newDisconnectTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	h := &disconnectTestHandler{
		upgrader: websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }},
		conns:    make(chan *websocket.Conn, 8),
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestHub_DisconnectUser_NoConnections(t *testing.T) {
	t.Parallel()
	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())

	count, err := hub.DisconnectUser("ghost")
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Equal(t, 0, hub.ConnectionCount())
}

func TestHub_DisconnectUser_SingleConnection(t *testing.T) {
	t.Parallel()
	srv := newDisconnectTestServer(t)
	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())

	obs := &captureObserver{}
	hub.AddObserver(obs)

	_, clientConn := dialAndRegister(t, srv, hub, "conn-1", "alice")
	defer clientConn.Close()

	require.Equal(t, 1, hub.ConnectionCount())

	count, err := hub.DisconnectUser("alice")
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, 0, hub.ConnectionCount())

	calls := obs.snapshot()
	require.Len(t, calls, 1, "observer must fire exactly once on disconnect")
	assert.Equal(t, ConnID("conn-1"), calls[0].connID)
	assert.Equal(t, UserID("alice"), calls[0].userID)

	// The client should observe the close frame within a short window.
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err = clientConn.ReadMessage()
	require.Error(t, err, "client must observe the server-initiated close frame")
	if ce, ok := err.(*websocket.CloseError); ok {
		assert.Equal(t, websocket.CloseNormalClosure, ce.Code,
			"DisconnectUser must send code 1000 NormalClosure with reason \"revoked\"")
		assert.Equal(t, "revoked", ce.Text)
	}
}

func TestHub_DisconnectUser_MultipleConnections_SameUser(t *testing.T) {
	t.Parallel()
	srv := newDisconnectTestServer(t)
	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())

	_, watch := dialAndRegister(t, srv, hub, "conn-watch", "bob")
	defer watch.Close()
	_, iphone := dialAndRegister(t, srv, hub, "conn-iphone", "bob")
	defer iphone.Close()

	require.Equal(t, 2, hub.ConnectionCount())

	count, err := hub.DisconnectUser("bob")
	require.NoError(t, err)
	assert.Equal(t, 2, count, "DisconnectUser must close ALL conns of the user, not stop at first")
	assert.Equal(t, 0, hub.ConnectionCount())
}

func TestHub_DisconnectUser_OtherUsersNotAffected(t *testing.T) {
	t.Parallel()
	srv := newDisconnectTestServer(t)
	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())

	_, c1 := dialAndRegister(t, srv, hub, "c1", "alice")
	defer c1.Close()
	_, c2 := dialAndRegister(t, srv, hub, "c2", "bob")
	defer c2.Close()

	require.Equal(t, 2, hub.ConnectionCount())

	count, err := hub.DisconnectUser("alice")
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	assert.Equal(t, 1, hub.ConnectionCount(), "bob's connection MUST remain registered")

	bobsConns := hub.FindByUser("bob")
	require.Len(t, bobsConns, 1)
	assert.Equal(t, ConnID("c2"), bobsConns[0].connID)
}

func TestHub_DisconnectUser_IdempotentCallTwice(t *testing.T) {
	t.Parallel()
	srv := newDisconnectTestServer(t)
	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())

	_, c1 := dialAndRegister(t, srv, hub, "c1", "carol")
	defer c1.Close()
	require.Equal(t, 1, hub.ConnectionCount())

	first, err := hub.DisconnectUser("carol")
	require.NoError(t, err)
	assert.Equal(t, 1, first)

	// Second call must be a no-op — unregisterClient relies on
	// sync.Map.LoadAndDelete which is idempotent.
	second, err := hub.DisconnectUser("carol")
	require.NoError(t, err)
	assert.Equal(t, 0, second, "second DisconnectUser on the same user must report 0 closed")
	assert.Equal(t, 0, hub.ConnectionCount())
}

// TestHub_disconnectAllForUser_StaleSnapshotEntry_NotCounted is the
// round-2 fix for the round-1 race regression test that did not
// actually exercise the snapshot-vs-unregister window.
//
// Why the previous attempt was inadequate: pre-evicting before
// calling DisconnectUser meant FindByUser only returned the live
// client, so the loop ran exactly once and count==1 either way —
// guarded or not. A regression of `count++` to unconditional would
// have stayed invisible.
//
// This test bypasses DisconnectUser's internal FindByUser call by
// driving disconnectAllForUser directly with a snapshot we capture
// BEFORE the eviction. That snapshot includes the stale *Client
// pointer; when the loop reaches it, unregisterClient's
// LoadAndDelete misses (the entry is already gone) and the guarded
// `if h.unregisterClient(c) { count++ }` correctly skips. Removing
// the guard would let count climb to 2.
func TestHub_disconnectAllForUser_StaleSnapshotEntry_NotCounted(t *testing.T) {
	t.Parallel()
	srv := newDisconnectTestServer(t)
	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())

	_, c1 := dialAndRegister(t, srv, hub, "conn-live", "eve")
	defer c1.Close()
	_, c2 := dialAndRegister(t, srv, hub, "conn-stale", "eve")
	defer c2.Close()
	require.Equal(t, 2, hub.ConnectionCount())

	// Capture the snapshot BEFORE the race: at this point both
	// clients are registered and the slice contains real *Client
	// pointers from h.clients.
	snapshot := hub.FindByUser("eve")
	require.Len(t, snapshot, 2, "snapshot must include both clients")

	// Now race readPump's defer: evict c-stale via Hub.Unregister
	// (the same path readPump's defer takes on connection close).
	// The snapshot still holds the *Client pointer.
	hub.Unregister("conn-stale")
	require.Equal(t, 1, hub.ConnectionCount(),
		"only c-live should remain in the hub's sync.Map")

	count := hub.disconnectAllForUser("eve", snapshot)
	assert.Equal(t, 1, count,
		"snapshot entry whose connID was evicted between snapshot and "+
			"unregisterClient MUST NOT be counted; if this fails, the "+
			"`if h.unregisterClient(c) { count++ }` guard regressed to "+
			"unconditional count++ — round-1 review fix lost")
	assert.Equal(t, 0, hub.ConnectionCount(),
		"live client must still be evicted")
}

// TestHub_DisconnectUser_AllRegistered_CountMatches keeps the prior
// "all clients still alive at call time" coverage now that the
// snapshot-race assertion moved to the helper-level test above.
func TestHub_DisconnectUser_AllRegistered_CountMatches(t *testing.T) {
	t.Parallel()
	srv := newDisconnectTestServer(t)
	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())

	_, c1 := dialAndRegister(t, srv, hub, "conn-a", "frank")
	defer c1.Close()
	_, c2 := dialAndRegister(t, srv, hub, "conn-b", "frank")
	defer c2.Close()
	require.Equal(t, 2, hub.ConnectionCount())

	count, err := hub.DisconnectUser("frank")
	require.NoError(t, err)
	assert.Equal(t, 2, count, "both registered clients must be counted")
	assert.Equal(t, 0, hub.ConnectionCount())
}

// TestHub_DisconnectUser_WriteCloseFrameFailureSwallowed simulates the
// "client conn already broken" branch: close the client side first so
// the server-side WriteControl returns a write error. DisconnectUser
// MUST swallow it (only Warn-log) and still clean up the hub entry —
// otherwise one bad connection would starve the eviction loop and
// Story 1.6 account deletion would block on a hung TCP socket.
func TestHub_DisconnectUser_WriteCloseFrameFailureSwallowed(t *testing.T) {
	t.Parallel()
	srv := newDisconnectTestServer(t)
	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())

	_, c1 := dialAndRegister(t, srv, hub, "c1", "dave")
	// Close the client side so server-side WriteControl fails.
	require.NoError(t, c1.Close())
	// Yield so the close propagates to the server side.
	time.Sleep(50 * time.Millisecond)

	count, err := hub.DisconnectUser("dave")
	require.NoError(t, err, "WriteControl error must be swallowed")
	assert.Equal(t, 1, count, "client must still be evicted from hub")
	assert.Equal(t, 0, hub.ConnectionCount())
}

