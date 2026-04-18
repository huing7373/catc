//go:build integration

package ws_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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

// setupGuardedTestServer starts an httptest.Server with a real
// WS UpgradeHandler wired to miniredis-backed Blacklist + ConnectRateLimiter.
// Each call uses an independent miniredis instance so subtests cannot
// collide on key state.
func setupGuardedTestServer(t *testing.T, threshold int64, window time.Duration) (*httptest.Server, *redisx.RedisBlacklist, *miniredis.Miniredis) {
	t.Helper()

	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { cli.Close() })

	hub := ws.NewHub(ws.HubConfig{
		PingInterval: 30 * time.Second,
		PongTimeout:  60 * time.Second,
		SendBufSize:  64,
	}, clockx.NewRealClock())

	dispatcher := ws.NewDispatcher(nil, clockx.NewRealClock())

	bl := redisx.NewBlacklist(cli)
	rl := redisx.NewConnectRateLimiter(cli, clockx.NewRealClock(), threshold, window)

	validator := ws.NewDebugValidator()
	upgradeHandler := ws.NewUpgradeHandler(hub, dispatcher, validator, bl, rl)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws", upgradeHandler.Handle)

	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		hub.Final(context.Background())
		srv.Close()
	})

	return srv, bl, mr
}

func guardWSURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
}

func dialWith(t *testing.T, url, userID string) (*gws.Conn, *http.Response, error) {
	t.Helper()
	header := http.Header{}
	header.Set("Authorization", "Bearer "+userID)
	return gws.DefaultDialer.Dial(url, header)
}

func TestIntegration_WS_ConnectRateLimit_BlocksSixth(t *testing.T) {
	srv, _, _ := setupGuardedTestServer(t, 5, 60*time.Second)
	url := guardWSURL(srv)

	for i := 1; i <= 5; i++ {
		conn, resp, err := dialWith(t, url, "ratelimit-user")
		require.NoError(t, err, "attempt %d must succeed", i)
		assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
		// Close immediately so hub/readPump wind down before next connect.
		require.NoError(t, conn.Close())
	}

	conn, resp, err := dialWith(t, url, "ratelimit-user")
	if conn != nil {
		conn.Close()
	}
	require.Error(t, err, "sixth attempt must be rejected")
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)

	ra := resp.Header.Get("Retry-After")
	require.NotEmpty(t, ra, "Retry-After header must be set on 429")
	n, err := strconv.Atoi(ra)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 1)
	assert.LessOrEqual(t, n, 60)
}

func TestIntegration_WS_BlacklistBlocksUpgrade(t *testing.T) {
	srv, bl, _ := setupGuardedTestServer(t, 5, 60*time.Second)
	url := guardWSURL(srv)
	ctx := context.Background()

	require.NoError(t, bl.Add(ctx, "banned-user", 10*time.Minute))

	conn, resp, err := dialWith(t, url, "banned-user")
	if conn != nil {
		conn.Close()
	}
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)

	require.NoError(t, bl.Remove(ctx, "banned-user"))

	conn2, resp2, err := dialWith(t, url, "banned-user")
	require.NoError(t, err, "removal must re-enable connect")
	defer conn2.Close()
	assert.Equal(t, http.StatusSwitchingProtocols, resp2.StatusCode)
}

func TestIntegration_WS_BlacklistOverridesRateLimit(t *testing.T) {
	srv, bl, _ := setupGuardedTestServer(t, 5, 60*time.Second)
	url := guardWSURL(srv)
	ctx := context.Background()

	// Saturate the rate limit counter first (5 successful connects).
	for i := 1; i <= 5; i++ {
		conn, resp, err := dialWith(t, url, "bad-actor")
		require.NoError(t, err, "setup attempt %d must succeed", i)
		assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
		require.NoError(t, conn.Close())
	}

	// Now the user is both over-threshold AND freshly blacklisted.
	// Blacklist must win: fatal takes priority over retry_after.
	require.NoError(t, bl.Add(ctx, "bad-actor", 5*time.Minute))

	conn, resp, err := dialWith(t, url, "bad-actor")
	if conn != nil {
		conn.Close()
	}
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusForbidden, resp.StatusCode,
		"blacklist must short-circuit before rate limiter — expected 403, got %d", resp.StatusCode)
}
