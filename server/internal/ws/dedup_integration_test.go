//go:build integration

package ws_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	gws "github.com/gorilla/websocket"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/ws"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/redisx"
)

type dedupTestServer struct {
	srv      *httptest.Server
	mr       *miniredis.Miniredis
	callCtr  *atomic.Int64
	authHdr  http.Header
}

func setupDedupServer(t *testing.T) *dedupTestServer {
	t.Helper()

	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { cli.Close() })

	store := redisx.NewDedupStore(cli, 5*time.Minute)

	hub := ws.NewHub(ws.HubConfig{
		PingInterval: 30 * time.Second,
		PongTimeout:  60 * time.Second,
		SendBufSize:  64,
	}, clockx.NewRealClock())

	dispatcher := ws.NewDispatcher(store, clockx.NewRealClock())

	var ctr atomic.Int64
	dispatcher.RegisterDedup("debug.echo.dedup", func(_ context.Context, _ *ws.Client, env ws.Envelope) (json.RawMessage, error) {
		ctr.Add(1)
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

	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer dedup-user")

	return &dedupTestServer{srv: srv, mr: mr, callCtr: &ctr, authHdr: hdr}
}

func dedupWSURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
}

// TestIntegration_WS_DedupIdempotency covers AC15: 3 identical eventIds produce
// byte-identical responses; the handler runs once; Redis state reflects done.
func TestIntegration_WS_DedupIdempotency(t *testing.T) {
	ts := setupDedupServer(t)

	conn, resp, err := gws.DefaultDialer.Dial(dedupWSURL(ts.srv), ts.authHdr)
	require.NoError(t, err)
	defer conn.Close()
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	responses := make([][]byte, 0, 3)
	for i := 1; i <= 3; i++ {
		env := map[string]any{
			"id":      "evt-1",
			"type":    "debug.echo.dedup",
			"payload": map[string]any{"n": i},
		}
		require.NoError(t, conn.WriteJSON(env))

		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)
		responses = append(responses, msg)
	}

	assert.Equal(t, int64(1), ts.callCtr.Load(), "handler must run exactly once across 3 identical eventIds")

	// All responses must be byte-identical.
	assert.Equal(t, string(responses[0]), string(responses[1]))
	assert.Equal(t, string(responses[0]), string(responses[2]))

	// First response payload must be {"n":1}, not {"n":2} or {"n":3}.
	var first ws.Response
	require.NoError(t, json.Unmarshal(responses[0], &first))
	assert.True(t, first.OK)
	assert.Equal(t, "evt-1", first.ID)
	assert.JSONEq(t, `{"n":1}`, string(first.Payload))

	// Redis keys are scoped by (userId, msgType, eventId) with length-prefix
	// encoding to prevent cross-user / cross-action collisions on reused
	// client-generated ids and to remain injective when any field contains
	// ":". Reproduce the server-side key shape here.
	scoped := fmt.Sprintf(
		"%d:%s:%d:%s:%d:%s",
		len("dedup-user"), "dedup-user",
		len("debug.echo.dedup"), "debug.echo.dedup",
		len("evt-1"), "evt-1",
	)
	marker, err := ts.mr.Get("event:" + scoped)
	require.NoError(t, err)
	assert.Equal(t, "done", marker)

	ok := ts.mr.HGet("event_result:"+scoped, "ok")
	assert.Equal(t, "true", ok)
}
