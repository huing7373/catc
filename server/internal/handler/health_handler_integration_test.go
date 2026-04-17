//go:build integration

package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/handler"
	"github.com/huing/cat/server/internal/testutil"
	"github.com/huing/cat/server/internal/ws"
	"github.com/huing/cat/server/pkg/mongox"
	"github.com/huing/cat/server/pkg/redisx"
)

func setupIntegrationHandler(t *testing.T) (*handler.HealthHandler, *gin.Engine, func()) {
	t.Helper()

	mongoCli, mongoCleanup := testutil.SetupMongo(t)
	redisCli, redisCleanup := testutil.SetupRedis(t)

	mongoWrapper := mongox.WrapClient(mongoCli, "testdb")
	redisWrapper := redisx.WrapClient(redisCli)

	h := handler.NewHealthHandler(mongoWrapper, redisWrapper, ws.NewHubStub(), redisCli, 10000)
	h.SetReady()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/healthz", h.Healthz)
	r.GET("/readyz", h.Readyz)

	cleanup := func() {
		redisCleanup()
		mongoCleanup()
	}
	return h, r, cleanup
}

func TestIntegration_Healthz_AllHealthy(t *testing.T) {
	_, r, cleanup := setupIntegrationHandler(t)
	defer cleanup()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/healthz", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"status":"ok"`)
	assert.Contains(t, body, `"mongo":"ok"`)
	assert.Contains(t, body, `"redis":"ok"`)
}

func TestIntegration_Healthz_MongoDown(t *testing.T) {
	mongoCli, mongoCleanup := testutil.SetupMongo(t)
	redisCli, redisCleanup := testutil.SetupRedis(t)
	defer redisCleanup()

	mongoWrapper := mongox.WrapClient(mongoCli, "testdb")
	redisWrapper := redisx.WrapClient(redisCli)

	h := handler.NewHealthHandler(mongoWrapper, redisWrapper, ws.NewHubStub(), redisCli, 10000)
	h.SetReady()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/healthz", h.Healthz)

	mongoCleanup()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/healthz", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, `"status":"error"`)
	assert.Contains(t, body, `"redis":"ok"`)
}

func TestIntegration_Healthz_P95Under50ms(t *testing.T) {
	_, r, cleanup := setupIntegrationHandler(t)
	defer cleanup()

	const iterations = 100
	durations := make([]time.Duration, iterations)

	for i := 0; i < iterations; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequest(http.MethodGet, "/healthz", nil)
		start := time.Now()
		r.ServeHTTP(w, req)
		durations[i] = time.Since(start)
		require.Equal(t, http.StatusOK, w.Code)
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[int(float64(iterations)*0.95)]
	assert.LessOrEqual(t, p95, 50*time.Millisecond, "p95 latency %v exceeds 50ms", p95)
}

func TestIntegration_Readyz(t *testing.T) {
	_, r, cleanup := setupIntegrationHandler(t)
	defer cleanup()

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/readyz", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"ready":true`)
}

func TestIntegration_Healthz_WithCronTick(t *testing.T) {
	mongoCli, mongoCleanup := testutil.SetupMongo(t)
	defer mongoCleanup()
	redisCli, redisCleanup := testutil.SetupRedis(t)
	defer redisCleanup()

	ctx := context.Background()
	redisCli.Set(ctx, "cron:last_tick", "2026-04-17T03:00:00Z", 0)

	mongoWrapper := mongox.WrapClient(mongoCli, "testdb")
	redisWrapper := redisx.WrapClient(redisCli)
	h := handler.NewHealthHandler(mongoWrapper, redisWrapper, ws.NewHubStub(), redisCli, 10000)
	h.SetReady()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/healthz", h.Healthz)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/healthz", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"lastCronTick":"2026-04-17T03:00:00Z"`)
}
