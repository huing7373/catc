package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRequestLogger_SetsRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestLogger())
	router.GET("/test", func(c *gin.Context) {
		requestID, exists := c.Get(RequestIDKey)
		assert.True(t, exists)
		assert.NotEmpty(t, requestID)
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotEmpty(t, w.Header().Get("X-Request-ID"))
}

func TestRequestLogger_ReturnsRequestIDHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(RequestLogger())
	router.GET("/test", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	requestID := w.Header().Get("X-Request-ID")
	assert.NotEmpty(t, requestID)
	// UUID format: 8-4-4-4-12
	assert.Len(t, requestID, 36)
}

func TestInitLogger_DoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		InitLogger()
	})
}
