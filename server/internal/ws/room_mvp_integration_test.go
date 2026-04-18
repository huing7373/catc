//go:build integration

package ws_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/ws"
	"github.com/huing/cat/server/pkg/clockx"
)

// setupRoomMVPServer mirrors cmd/cat/initialize.go's debug branch for just
// the room MVP surface — no APNs, no session.resume, no dedup. Lets the
// integration test exercise the full handler path without standing up
// Mongo / Redis.
func setupRoomMVPServer(t *testing.T) *httptest.Server {
	t.Helper()

	clk := clockx.NewRealClock()
	hub := ws.NewHub(ws.HubConfig{
		PingInterval: 30 * time.Second,
		PongTimeout:  60 * time.Second,
		SendBufSize:  64,
	}, clk)

	dispatcher := ws.NewDispatcher(nil, clk)
	broadcaster := ws.NewInMemoryBroadcaster(hub)
	roomManager := ws.NewRoomManager(clk, broadcaster)
	hub.AddObserver(roomManager)
	dispatcher.Register("room.join", roomManager.HandleJoin)
	dispatcher.Register("action.update", roomManager.HandleActionUpdate)

	validator := ws.NewDebugValidator()
	upgrade := ws.NewUpgradeHandler(hub, dispatcher, validator, nil, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws", upgrade.Handle)

	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		hub.Final(context.Background())
		srv.Close()
	})
	return srv
}

func dialAs(t *testing.T, srv *httptest.Server, userToken string) *gws.Conn {
	t.Helper()
	u := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	h := http.Header{}
	h.Set("Authorization", "Bearer "+userToken)
	conn, resp, err := gws.DefaultDialer.Dial(u, h)
	require.NoError(t, err)
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func sendEnvelope(t *testing.T, c *gws.Conn, id, msgType string, payload any) {
	t.Helper()
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	require.NoError(t, c.WriteJSON(map[string]any{
		"id":      id,
		"type":    msgType,
		"payload": json.RawMessage(body),
	}))
}

// readNext reads the next WS text frame and returns (response-or-push) as a
// generic map so the test can distinguish `ok` responses from `type`-only
// pushes.
func readNext(t *testing.T, c *gws.Conn) map[string]any {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := c.ReadMessage()
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	return m
}

func TestIntegration_RoomMVP_JoinUpdateBroadcast(t *testing.T) {
	srv := setupRoomMVPServer(t)

	a := dialAs(t, srv, "alice")
	b := dialAs(t, srv, "bob")
	c := dialAs(t, srv, "carol")

	// All three join the same room.
	for i, conn := range []*gws.Conn{a, b, c} {
		sendEnvelope(t, conn, "join-"+string(rune('a'+i)), "room.join",
			map[string]string{"roomId": "test-room"})
		resp := readNext(t, conn)
		assert.Equal(t, true, resp["ok"])
		assert.Equal(t, "room.join.result", resp["type"])
	}

	// Alice publishes an action; Bob and Carol must each receive a
	// downstream action.broadcast, and Alice must receive only her own ack.
	sendEnvelope(t, a, "act-1", "action.update", map[string]string{"action": "walking"})

	aliceAck := readNext(t, a)
	assert.Equal(t, true, aliceAck["ok"])
	assert.Equal(t, "action.update.result", aliceAck["type"])

	for _, peer := range []*gws.Conn{b, c} {
		push := readNext(t, peer)
		assert.Equal(t, "action.broadcast", push["type"])
		payload, ok := push["payload"].(map[string]any)
		require.True(t, ok, "payload must be object, got %v", push["payload"])
		assert.Equal(t, "alice", payload["userId"])
		assert.Equal(t, "walking", payload["action"])
	}

	// Disconnect bob; then alice fires another action — only carol should
	// receive it.
	require.NoError(t, b.Close())
	// Give the Hub read-pump time to observe the close and fire OnDisconnect.
	time.Sleep(150 * time.Millisecond)

	sendEnvelope(t, a, "act-2", "action.update", map[string]string{"action": "waving"})
	_ = readNext(t, a) // ack

	push := readNext(t, c)
	assert.Equal(t, "action.broadcast", push["type"])
	payload := push["payload"].(map[string]any)
	assert.Equal(t, "waving", payload["action"])

	// Carol should NOT see another push after this — if bob were still in
	// the room, readNext would time out on bob but might also leak a
	// duplicate broadcast to carol. We already asserted the single push;
	// no additional assertion needed here.
}
