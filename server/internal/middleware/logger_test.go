package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/huing/cat/server/pkg/logx"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupLoggerTest(t *testing.T) (*bytes.Buffer, *gin.Engine) {
	t.Helper()
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	_, r := gin.CreateTestContext(httptest.NewRecorder())

	r.Use(func(c *gin.Context) {
		ctx := logger.WithContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.Use(Logger())
	return &buf, r
}

func TestLogger_LogsRequestFields(t *testing.T) {
	t.Parallel()
	buf, r := setupLoggerTest(t)

	r.GET("/test-path", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test-path", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var m map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))
	assert.Equal(t, "GET", m["method"])
	assert.Equal(t, "/test-path", m["path"])
	assert.Equal(t, float64(200), m["status"])
	assert.Contains(t, m, "durationMs")
	assert.Equal(t, "request", m["message"])
}

func TestLogger_IncludesUserIDFromContext(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	_, r := gin.CreateTestContext(httptest.NewRecorder())

	r.Use(func(c *gin.Context) {
		ctx := logger.WithContext(c.Request.Context())
		ctx = logx.WithUserID(ctx, "user-42")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.Use(Logger())

	r.GET("/with-user", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/with-user", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var m map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))
	assert.Equal(t, "user-42", m["userId"])
}

func TestLogger_OmitsUserIDWhenAbsent(t *testing.T) {
	t.Parallel()
	buf, r := setupLoggerTest(t)

	r.GET("/no-user", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/no-user", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var m map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))
	assert.NotContains(t, m, "userId")
}

func TestLogger_LogsOnPanic(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	_, r := gin.CreateTestContext(httptest.NewRecorder())

	r.Use(func(c *gin.Context) {
		ctx := logger.WithContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.Use(Logger())
	r.Use(Recover())
	r.GET("/panic", func(c *gin.Context) {
		panic("boom")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	r.ServeHTTP(w, req)

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	require.GreaterOrEqual(t, len(lines), 1)

	hasAccessLog := false
	for _, line := range lines {
		var m map[string]any
		if err := json.Unmarshal(line, &m); err == nil {
			if m["message"] == "request" {
				hasAccessLog = true
				assert.Equal(t, float64(500), m["status"])
			}
		}
	}
	assert.True(t, hasAccessLog, "expected access log line with message='request'")
}

func TestLogger_FallsBackToGlobalLogger(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	origLogger := zerolog.New(&buf)
	ctx := origLogger.WithContext(context.Background())

	l := logx.Ctx(ctx)
	l.Info().Msg("test")

	var m map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))
	assert.Equal(t, "test", m["message"])
}
