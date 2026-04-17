package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/huing/cat/server/pkg/logx"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestRequestID_GeneratesWhenMissing(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)

	rid := w.Header().Get(headerRequestID)
	assert.NotEmpty(t, rid)
	_, err := uuid.Parse(rid)
	require.NoError(t, err)
}

func TestRequestID_UsesExistingHeader(t *testing.T) {
	t.Parallel()
	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(headerRequestID, "existing-id-123")
	r.ServeHTTP(w, req)

	assert.Equal(t, "existing-id-123", w.Header().Get(headerRequestID))
}

func TestRequestID_InjectsIntoContextLogger(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	origGlobal := log.Logger
	log.Logger = zerolog.New(&buf)
	defer func() { log.Logger = origGlobal }()

	w := httptest.NewRecorder()
	_, r := gin.CreateTestContext(w)
	r.Use(RequestID())
	r.GET("/test", func(c *gin.Context) {
		logx.Ctx(c.Request.Context()).Info().Msg("inside handler")
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set(headerRequestID, "test-rid-999")
	r.ServeHTTP(w, req)

	var m map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))
	assert.Equal(t, "test-rid-999", m["requestId"])
	assert.Equal(t, "inside handler", m["message"])
}
