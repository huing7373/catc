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

func setupTestServer(t *testing.T) (*httptest.Server, *ws.Hub) {
	t.Helper()

	hub := ws.NewHub(ws.HubConfig{
		PingInterval: 30 * time.Second,
		PongTimeout:  60 * time.Second,
		SendBufSize:  64,
	}, clockx.NewRealClock())

	dispatcher := ws.NewDispatcher(nil, clockx.NewRealClock())
	dispatcher.Register("debug.echo", func(_ context.Context, _ *ws.Client, env ws.Envelope) (json.RawMessage, error) {
		return env.Payload, nil
	})

	validator := ws.NewDebugValidator()
	upgradeHandler := ws.NewUpgradeHandler(hub, dispatcher, validator)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws", upgradeHandler.Handle)

	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		hub.Final(context.Background())
		srv.Close()
	})

	return srv, hub
}

func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
}

func TestIntegration_WS_EchoRoundTrip(t *testing.T) {
	srv, _ := setupTestServer(t)

	header := http.Header{}
	header.Set("Authorization", "Bearer test-user-1")
	conn, resp, err := gws.DefaultDialer.Dial(wsURL(srv), header)
	require.NoError(t, err)
	defer conn.Close()
	assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	env := map[string]any{
		"id":      "req-1",
		"type":    "debug.echo",
		"payload": map[string]any{"msg": "hello"},
	}
	require.NoError(t, conn.WriteJSON(env))

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)

	var resp2 ws.Response
	require.NoError(t, json.Unmarshal(msg, &resp2))
	assert.True(t, resp2.OK)
	assert.Equal(t, "req-1", resp2.ID)
	assert.Equal(t, "debug.echo.result", resp2.Type)
	assert.JSONEq(t, `{"msg":"hello"}`, string(resp2.Payload))
}

func TestIntegration_WS_NoToken_Rejected(t *testing.T) {
	srv, _ := setupTestServer(t)

	conn, resp, err := gws.DefaultDialer.Dial(wsURL(srv), nil)
	if conn != nil {
		conn.Close()
	}
	if err == nil {
		t.Fatal("expected connection to be rejected without token")
	}
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestIntegration_WS_ShutdownCloseFrame(t *testing.T) {
	srv, hub := setupTestServer(t)

	header := http.Header{}
	header.Set("Authorization", "Bearer test-user-2")
	conn, _, err := gws.DefaultDialer.Dial(wsURL(srv), header)
	require.NoError(t, err)
	defer conn.Close()

	hub.Final(context.Background())
	srv.Close()

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, _, err = conn.ReadMessage()
	require.Error(t, err)

	var closeErr *gws.CloseError
	if assert.ErrorAs(t, err, &closeErr) {
		assert.Equal(t, gws.CloseGoingAway, closeErr.Code)
	}
}
