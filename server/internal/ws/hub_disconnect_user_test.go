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

// TestHub_DisconnectUser_RaceWithSelfDisconnect_DoesNotOvercount
// simulates the FindByUser-vs-readPump race the round-1 review
// flagged: a client that disconnects on its own AFTER FindByUser
// snapshots it but BEFORE the loop reaches unregisterClient must NOT
// be counted in the returned `connectionsClosed`. We force the race
// deterministically by registering two clients of the same user, then
// pre-evicting one via Hub.Unregister (the same path readPump's
// defer takes) before invoking DisconnectUser.
func TestHub_DisconnectUser_RaceWithSelfDisconnect_DoesNotOvercount(t *testing.T) {
	t.Parallel()
	srv := newDisconnectTestServer(t)
	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())

	_, c1 := dialAndRegister(t, srv, hub, "conn-stale", "eve")
	defer c1.Close()
	_, c2 := dialAndRegister(t, srv, hub, "conn-live", "eve")
	defer c2.Close()
	require.Equal(t, 2, hub.ConnectionCount())

	// Pre-evict the first client to simulate readPump's defer
	// unregister having already won the race against DisconnectUser.
	hub.Unregister("conn-stale")
	require.Equal(t, 1, hub.ConnectionCount())

	count, err := hub.DisconnectUser("eve")
	require.NoError(t, err)
	assert.Equal(t, 1, count,
		"DisconnectUser MUST count only successfully unregistered clients — "+
			"snapshot-then-self-disconnect race must not inflate the return")
	assert.Equal(t, 0, hub.ConnectionCount(), "live client must still be evicted")
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

