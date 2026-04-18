//go:build integration

package ws_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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

// resumeTestServer wraps the WS infrastructure needed to exercise the
// session.resume handler end to end: a real miniredis-backed RedisResumeCache
// plus counter providers so tests can assert how many times the providers
// actually ran across N client calls.
type resumeTestServer struct {
	srv         *httptest.Server
	mr          *miniredis.Miniredis
	cache       *redisx.RedisResumeCache
	userCalls   *atomic.Int64
	friendCalls *atomic.Int64
	catCalls    *atomic.Int64
	skinCalls   *atomic.Int64
	boxCalls    *atomic.Int64
	roomCalls   *atomic.Int64
	authHdr     http.Header
}

// counting providers — each increments a shared atomic counter every time
// the handler invokes it, so "did the cache spare a provider hit?" becomes a
// single integer assertion.
type countingUserProvider struct{ n *atomic.Int64 }

func (p countingUserProvider) GetUser(context.Context, string) (json.RawMessage, error) {
	p.n.Add(1)
	return json.RawMessage(`{"id":"u1"}`), nil
}

type countingFriendsProvider struct{ n *atomic.Int64 }

func (p countingFriendsProvider) ListFriends(context.Context, string) (json.RawMessage, error) {
	p.n.Add(1)
	return json.RawMessage(`[]`), nil
}

type countingCatStateProvider struct{ n *atomic.Int64 }

func (p countingCatStateProvider) GetCatState(context.Context, string) (json.RawMessage, error) {
	p.n.Add(1)
	return json.RawMessage(`{"state":"idle"}`), nil
}

type countingSkinsProvider struct{ n *atomic.Int64 }

func (p countingSkinsProvider) ListUnlocked(context.Context, string) (json.RawMessage, error) {
	p.n.Add(1)
	return json.RawMessage(`[]`), nil
}

type countingBlindboxesProvider struct{ n *atomic.Int64 }

func (p countingBlindboxesProvider) ListActive(context.Context, string) (json.RawMessage, error) {
	p.n.Add(1)
	return json.RawMessage(`[]`), nil
}

type countingRoomSnapshotProvider struct{ n *atomic.Int64 }

func (p countingRoomSnapshotProvider) GetRoomSnapshot(context.Context, string) (json.RawMessage, error) {
	p.n.Add(1)
	return json.RawMessage(`null`), nil
}

func setupResumeServer(t *testing.T) *resumeTestServer {
	t.Helper()

	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { cli.Close() })

	clk := clockx.NewRealClock()
	cache := redisx.NewResumeCache(cli, clk, 60*time.Second)

	// DedupStore is needed for NewDispatcher even though session.resume does
	// not route through it — registering a non-dedup handler does not touch
	// the store, but the constructor still requires one. Use the production
	// TTL shape so we're exercising real configuration.
	dedupStore := redisx.NewDedupStore(cli, 5*time.Minute)

	hub := ws.NewHub(ws.HubConfig{
		PingInterval: 30 * time.Second,
		PongTimeout:  60 * time.Second,
		SendBufSize:  64,
	}, clk)

	dispatcher := ws.NewDispatcher(dedupStore, clk)

	var userCalls, friendCalls, catCalls, skinCalls, boxCalls, roomCalls atomic.Int64
	handler := ws.NewSessionResumeHandler(cache, clk, ws.ResumeProviders{
		User:         countingUserProvider{n: &userCalls},
		Friends:      countingFriendsProvider{n: &friendCalls},
		CatState:     countingCatStateProvider{n: &catCalls},
		Skins:        countingSkinsProvider{n: &skinCalls},
		Blindboxes:   countingBlindboxesProvider{n: &boxCalls},
		RoomSnapshot: countingRoomSnapshotProvider{n: &roomCalls},
	})
	dispatcher.Register("session.resume", handler.Handle)

	validator := ws.NewDebugValidator()
	upgradeHandler := ws.NewUpgradeHandler(hub, dispatcher, validator, nil, nil)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws", upgradeHandler.Handle)

	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		hub.Final(context.Background())
		srv.Close()
	})

	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer resume-user")

	return &resumeTestServer{
		srv:         srv,
		mr:          mr,
		cache:       cache,
		userCalls:   &userCalls,
		friendCalls: &friendCalls,
		catCalls:    &catCalls,
		skinCalls:   &skinCalls,
		boxCalls:    &boxCalls,
		roomCalls:   &roomCalls,
		authHdr:     hdr,
	}
}

func resumeWSURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
}

// TestIntegration_SessionResume_Benchmark_10Calls_OneProviderHit covers the
// epics AC12.a expectation: over ten resume envelopes within the 60s window
// only the first miss hits the providers. Since Epic 0 hasn't stood up Mongo
// collections yet, "provider hit" is the upstream signal — the same property
// the AC-text "mongotest or middleware counter" alludes to.
func TestIntegration_SessionResume_Benchmark_10Calls_OneProviderHit(t *testing.T) {
	ts := setupResumeServer(t)

	conn, resp, err := gws.DefaultDialer.Dial(resumeWSURL(ts.srv), ts.authHdr)
	require.NoError(t, err)
	defer conn.Close()
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

	const calls = 10
	for i := 1; i <= calls; i++ {
		env := map[string]any{
			"id":      "resume-" + strconv.Itoa(i),
			"type":    "session.resume",
			"payload": map[string]any{},
		}
		require.NoError(t, conn.WriteJSON(env))

		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)

		var response ws.Response
		require.NoError(t, json.Unmarshal(msg, &response))
		assert.True(t, response.OK, "call %d should succeed, got error: %+v", i, response.Error)
		assert.Equal(t, "session.resume.result", response.Type)
	}

	assert.Equal(t, int64(1), ts.userCalls.Load(), "UserProvider must run exactly once across 10 resumes")
	assert.Equal(t, int64(1), ts.friendCalls.Load())
	assert.Equal(t, int64(1), ts.catCalls.Load())
	assert.Equal(t, int64(1), ts.skinCalls.Load())
	assert.Equal(t, int64(1), ts.boxCalls.Load())
	assert.Equal(t, int64(1), ts.roomCalls.Load())
}

// TestIntegration_SessionResume_InvalidateRefetches proves the
// ResumeCacheInvalidator contract: calling Invalidate evicts the cache so the
// next resume hits every provider again. Exercises the service-layer
// consumption path (Stories 1.5 / 2.2 / 3.2 / 6.4 / 7.3) without needing the
// service packages to exist yet.
func TestIntegration_SessionResume_InvalidateRefetches(t *testing.T) {
	ts := setupResumeServer(t)

	conn, _, err := gws.DefaultDialer.Dial(resumeWSURL(ts.srv), ts.authHdr)
	require.NoError(t, err)
	defer conn.Close()

	sendResume(t, conn, "evt-1")
	assert.Equal(t, int64(1), ts.userCalls.Load())

	require.NoError(t, ts.cache.Invalidate(context.Background(), "resume-user"))

	sendResume(t, conn, "evt-2")
	assert.Equal(t, int64(2), ts.userCalls.Load(), "providers must be re-invoked after Invalidate")
	assert.Equal(t, int64(2), ts.friendCalls.Load())
}

// TestIntegration_SessionResume_NotDeduped guards AC7 in CI: reusing the same
// envelope.id twice must NOT trip the dedup middleware (session.resume is
// registered via Register, not RegisterDedup). If someone later "fixes" the
// registration to RegisterDedup, the second call would either be held in
// processing or replay the cached result with OK=true — either way this test
// catches the regression because: (a) the handler must be invokable a second
// time with the same id even when the cache has been explicitly cleared
// between calls, and (b) both responses must echo the request id back.
func TestIntegration_SessionResume_NotDeduped(t *testing.T) {
	ts := setupResumeServer(t)

	conn, _, err := gws.DefaultDialer.Dial(resumeWSURL(ts.srv), ts.authHdr)
	require.NoError(t, err)
	defer conn.Close()

	// First call: populate cache.
	first := sendResume(t, conn, "same-id")

	// Evict cache so a dedup-based implementation would *clearly* behave
	// differently (without eviction, a cache hit would also return OK=true
	// and mask the distinction). After Invalidate the second call must go
	// through to providers — proving no dedup gate blocked it.
	require.NoError(t, ts.cache.Invalidate(context.Background(), "resume-user"))

	second := sendResume(t, conn, "same-id")

	assert.True(t, first.OK)
	assert.True(t, second.OK)
	assert.Equal(t, "same-id", first.ID)
	assert.Equal(t, "same-id", second.ID)
	assert.Equal(t, int64(2), ts.userCalls.Load(),
		"same envelope.id used twice must not be dedup-gated — providers run each time after invalidate")
}

// sendResume writes one session.resume envelope, reads one response, and
// returns it parsed. Fails the test on any I/O error.
func sendResume(t *testing.T, conn *gws.Conn, id string) ws.Response {
	t.Helper()
	env := map[string]any{
		"id":      id,
		"type":    "session.resume",
		"payload": map[string]any{},
	}
	require.NoError(t, conn.WriteJSON(env))

	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)

	var resp ws.Response
	require.NoError(t, json.Unmarshal(msg, &resp))
	return resp
}

