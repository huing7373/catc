package dto

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRespondError_Format(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/err", func(c *gin.Context) {
		RespondError(c, http.StatusBadRequest, "BAD_REQUEST", "invalid input")
	})

	req := httptest.NewRequest(http.MethodGet, "/err", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body ErrorBody
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, "BAD_REQUEST", body.Error.Code)
	assert.Equal(t, "invalid input", body.Error.Message)
}

func TestRespondSuccess_Format(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/ok", func(c *gin.Context) {
		RespondSuccess(c, http.StatusOK, map[string]string{"name": "cat"})
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var body map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, "cat", body["name"])
}

func TestRespondNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/nf", func(c *gin.Context) {
		RespondNotFound(c, "resource not found")
	})

	req := httptest.NewRequest(http.MethodGet, "/nf", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var body ErrorBody
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, "NOT_FOUND", body.Error.Code)
}
