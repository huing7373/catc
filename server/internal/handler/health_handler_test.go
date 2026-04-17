package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockChecker struct {
	err error
}

func (m *mockChecker) HealthCheck(_ context.Context) error {
	return m.err
}

type mockHub struct {
	count int
}

func (m *mockHub) GoroutineCount() int {
	return m.count
}

type mockRedisCmd struct {
	redis.Cmdable
	cronTick string
	cronErr  error
}

func (m *mockRedisCmd) Get(_ context.Context, key string) *redis.StringCmd {
	cmd := redis.NewStringCmd(context.Background(), "get", key)
	if m.cronErr != nil {
		cmd.SetErr(m.cronErr)
	} else {
		cmd.SetVal(m.cronTick)
	}
	return cmd
}

func newTestHandler(mongoErr, redisErr error, hubCount int, cronTick string, cronErr error, maxConn int) *HealthHandler {
	return NewHealthHandler(
		&mockChecker{err: mongoErr},
		&mockChecker{err: redisErr},
		&mockHub{count: hubCount},
		&mockRedisCmd{cronTick: cronTick, cronErr: cronErr},
		maxConn,
	)
}

func doHealthz(t *testing.T, h *HealthHandler) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/healthz", h.Healthz)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/healthz", nil)
	r.ServeHTTP(w, req)
	return w
}

func doReadyz(t *testing.T, h *HealthHandler) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/readyz", h.Readyz)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/readyz", nil)
	r.ServeHTTP(w, req)
	return w
}

func TestHealthHandler_Healthz(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		mongoErr   error
		redisErr   error
		hubCount   int
		maxConn    int
		cronTick   string
		cronErr    error
		wantCode   int
		wantStatus string
		wantMongo  string
		wantRedis  string
		wantWsHub  string
	}{
		{
			name:       "all healthy",
			maxConn:    10000,
			cronTick:   "2026-04-17T03:00:00Z",
			wantCode:   200,
			wantStatus: "ok",
			wantMongo:  "ok",
			wantRedis:  "ok",
			wantWsHub:  "ok",
		},
		{
			name:       "mongo failure",
			mongoErr:   errors.New("connection refused"),
			maxConn:    10000,
			wantCode:   503,
			wantStatus: "error",
			wantMongo:  "error: connection refused",
			wantRedis:  "ok",
			wantWsHub:  "ok",
		},
		{
			name:       "redis failure",
			redisErr:   errors.New("dial tcp: connection refused"),
			maxConn:    10000,
			wantCode:   503,
			wantStatus: "error",
			wantMongo:  "ok",
			wantRedis:  "error: dial tcp: connection refused",
			wantWsHub:  "ok",
		},
		{
			name:       "ws hub goroutine exceeds max",
			hubCount:   10001,
			maxConn:    10000,
			wantCode:   503,
			wantStatus: "error",
			wantMongo:  "ok",
			wantRedis:  "ok",
			wantWsHub:  "error: goroutine count 10001 exceeds max 10000",
		},
		{
			name:       "cron tick missing returns empty string",
			maxConn:    10000,
			cronErr:    redis.Nil,
			wantCode:   200,
			wantStatus: "ok",
			wantMongo:  "ok",
			wantRedis:  "ok",
			wantWsHub:  "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHandler(tt.mongoErr, tt.redisErr, tt.hubCount, tt.cronTick, tt.cronErr, tt.maxConn)
			w := doHealthz(t, h)

			assert.Equal(t, tt.wantCode, w.Code)
			body := w.Body.String()
			assert.Contains(t, body, `"status":"`+tt.wantStatus+`"`)
			assert.Contains(t, body, `"mongo":"`+tt.wantMongo+`"`)
			assert.Contains(t, body, `"redis":"`+tt.wantRedis+`"`)
			assert.Contains(t, body, `"wsHub":"`+tt.wantWsHub+`"`)
		})
	}
}

func TestHealthHandler_Readyz_Ready(t *testing.T) {
	t.Parallel()
	h := newTestHandler(nil, nil, 0, "", redis.Nil, 10000)
	h.SetReady()
	w := doReadyz(t, h)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"ready":true`)
}

func TestHealthHandler_Readyz_NotReady(t *testing.T) {
	t.Parallel()
	h := newTestHandler(nil, nil, 0, "", redis.Nil, 10000)
	w := doReadyz(t, h)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Contains(t, w.Body.String(), `"ready":false`)
}

func TestHealthHandler_Healthz_CronTickPresent(t *testing.T) {
	t.Parallel()
	h := newTestHandler(nil, nil, 0, "2026-04-17T03:00:00Z", nil, 10000)
	w := doHealthz(t, h)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"lastCronTick":"2026-04-17T03:00:00Z"`)
}
