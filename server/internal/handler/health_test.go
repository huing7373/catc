package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	catredis "github.com/huing7373/catc/server/pkg/redis"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func setupMockDB(t *testing.T) (*gorm.DB, *sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	require.NoError(t, err)

	// GORM pings on Open, so expect it
	mock.ExpectPing()

	dialector := postgres.New(postgres.Config{
		Conn:       sqlDB,
		DriverName: "postgres",
	})
	db, err := gorm.Open(dialector, &gorm.Config{})
	require.NoError(t, err)

	return db, sqlDB, mock
}

func setupMockRedis() *catredis.Client {
	rdb := redis.NewClient(&redis.Options{Addr: "localhost:0"})
	return &catredis.Client{RDB: rdb}
}

func TestHealth_DBUp_Returns200(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, _, mock := setupMockDB(t)
	// Expect the health check ping
	mock.ExpectPing()

	redisClient := setupMockRedis()
	h := NewHealthHandler(db, redisClient)

	router := gin.New()
	router.GET("/health", h.Health)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// AC #2: always returns 200
	assert.Equal(t, http.StatusOK, w.Code)

	var resp HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "up", resp.Postgres)
	assert.Greater(t, resp.Goroutines, 0)
	assert.GreaterOrEqual(t, resp.UptimeSec, int64(0))
}

func TestHealth_DBDown_Returns200(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, _, mock := setupMockDB(t)
	mock.ExpectPing().WillReturnError(sql.ErrConnDone)

	redisClient := setupMockRedis()
	h := NewHealthHandler(db, redisClient)

	router := gin.New()
	router.GET("/health", h.Health)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// AC #2: always returns 200 even when degraded
	assert.Equal(t, http.StatusOK, w.Code)

	var resp HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "degraded", resp.Status)
	assert.Equal(t, "down", resp.Postgres)
}

func TestHealth_RedisDown_Returns200(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, _, mock := setupMockDB(t)
	mock.ExpectPing()

	redisClient := setupMockRedis()
	h := NewHealthHandler(db, redisClient)

	router := gin.New()
	router.GET("/health", h.Health)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "degraded", resp.Status)
	assert.Equal(t, "down", resp.Redis)
}

func TestHealth_ResponseContainsAllFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, _, mock := setupMockDB(t)
	mock.ExpectPing()

	redisClient := setupMockRedis()
	h := NewHealthHandler(db, redisClient)

	router := gin.New()
	router.GET("/health", h.Health)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var m map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &m)
	require.NoError(t, err)

	// Verify all expected JSON fields per AC #2
	assert.Contains(t, m, "status")
	assert.Contains(t, m, "postgres")
	assert.Contains(t, m, "redis")
	assert.Contains(t, m, "goroutines")
	assert.Contains(t, m, "uptime_sec")
}

func TestNewHealthHandler_SetsStartTime(t *testing.T) {
	before := time.Now()
	h := NewHealthHandler(nil, nil)
	after := time.Now()

	assert.False(t, h.startTime.Before(before))
	assert.False(t, h.startTime.After(after))
}

func TestRedisClient_PingFails(t *testing.T) {
	client := setupMockRedis()
	err := client.Ping(context.Background())
	assert.Error(t, err)
}
