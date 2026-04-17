package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRecover_CatchesPanic(t *testing.T) {
	t.Parallel()
	_, r := gin.CreateTestContext(httptest.NewRecorder())
	r.Use(Recover())
	r.GET("/panic", func(c *gin.Context) {
		panic("something went wrong")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var body map[string]map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "INTERNAL_ERROR", body["error"]["code"])
	assert.Equal(t, "internal server error", body["error"]["message"])
	assert.NotContains(t, w.Body.String(), "something went wrong")
}

func TestRecover_PassesThroughNormally(t *testing.T) {
	t.Parallel()
	_, r := gin.CreateTestContext(httptest.NewRecorder())
	r.Use(Recover())
	r.GET("/ok", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRecover_SkipsBodyWhenAlreadyWritten(t *testing.T) {
	t.Parallel()
	_, r := gin.CreateTestContext(httptest.NewRecorder())
	r.Use(Recover())
	r.GET("/partial", func(c *gin.Context) {
		c.Writer.WriteHeader(http.StatusOK)
		_, _ = c.Writer.WriteString("partial")
		panic("late panic")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/partial", nil)
	r.ServeHTTP(w, req)

	assert.NotContains(t, w.Body.String(), "INTERNAL_ERROR")
}
