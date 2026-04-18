package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/handler"
	"github.com/huing/cat/server/pkg/clockx"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func newPlatformRouter(h *handler.PlatformHandler) *gin.Engine {
	r := gin.New()
	r.GET("/v1/platform/ws-registry", h.WSRegistry)
	return r
}

func doRegistryRequest(t *testing.T, h *handler.PlatformHandler) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "/v1/platform/ws-registry", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	newPlatformRouter(h).ServeHTTP(w, req)
	return w
}

func TestNewPlatformHandler_NilClockPanics(t *testing.T) {
	t.Parallel()
	assert.PanicsWithValue(t, "handler.NewPlatformHandler: clock is required", func() {
		handler.NewPlatformHandler(nil, "debug")
	})
}

func TestPlatformHandler_WSRegistry_DebugMode(t *testing.T) {
	t.Parallel()

	fakeNow := time.Date(2026, 4, 18, 12, 34, 56, 0, time.UTC)
	h := handler.NewPlatformHandler(clockx.NewFakeClock(fakeNow), "debug")

	w := doRegistryRequest(t, h)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

	var resp handler.WSRegistryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "v1", resp.APIVersion)
	assert.Equal(t, "2026-04-18T12:34:56Z", resp.ServerTime)

	// Debug mode surfaces every WSMessages entry (all DebugOnly today).
	require.Len(t, resp.Messages, len(dto.WSMessages))

	byType := make(map[string]handler.WSRegistryMessage, len(resp.Messages))
	for _, m := range resp.Messages {
		byType[m.Type] = m
	}
	assert.Contains(t, byType, "session.resume")
	assert.Contains(t, byType, "debug.echo")
	assert.Contains(t, byType, "debug.echo.dedup")

	assert.False(t, byType["session.resume"].RequiresDedup)
	assert.False(t, byType["debug.echo"].RequiresDedup)
	assert.True(t, byType["debug.echo.dedup"].RequiresDedup)

	// DebugOnly must never leak onto the wire — assert via raw JSON body.
	assert.NotContains(t, w.Body.String(), "debugOnly")
	assert.NotContains(t, w.Body.String(), "DebugOnly")
}

func TestPlatformHandler_WSRegistry_ReleaseMode(t *testing.T) {
	t.Parallel()

	fakeNow := time.Date(2026, 4, 18, 12, 34, 56, 0, time.UTC)
	h := handler.NewPlatformHandler(clockx.NewFakeClock(fakeNow), "release")

	w := doRegistryRequest(t, h)

	require.Equal(t, http.StatusOK, w.Code)

	var resp handler.WSRegistryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "v1", resp.APIVersion)
	assert.Equal(t, "2026-04-18T12:34:56Z", resp.ServerTime)

	// Release mode filters out every DebugOnly entry. At Story 0.14 all three
	// entries are DebugOnly; this MUST produce an empty-but-not-null messages
	// slice (iOS ISO8601 decoders cope with [] but bomb on null for arrays).
	for _, meta := range dto.WSMessages {
		if !meta.DebugOnly {
			assert.Contains(t,
				typesOf(resp.Messages), meta.Type,
				"release mode must surface non-DebugOnly entry %q", meta.Type)
		} else {
			assert.NotContains(t,
				typesOf(resp.Messages), meta.Type,
				"release mode must hide DebugOnly entry %q", meta.Type)
		}
	}

	// Empty JSON array, never null.
	assert.True(t,
		strings.Contains(w.Body.String(), `"messages":[]`),
		"release mode with zero visible entries must serialize messages as [] not null; body=%s",
		w.Body.String(),
	)
}

func TestPlatformHandler_WSRegistry_ServerTimeUsesClock(t *testing.T) {
	t.Parallel()

	fakeNow := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	h := handler.NewPlatformHandler(clockx.NewFakeClock(fakeNow), "debug")

	w := doRegistryRequest(t, h)

	var resp handler.WSRegistryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	parsed, err := time.Parse(time.RFC3339, resp.ServerTime)
	require.NoError(t, err, "serverTime must be RFC3339")
	assert.True(t, parsed.Equal(fakeNow), "serverTime %q must match FakeClock %q", resp.ServerTime, fakeNow)
}

func typesOf(msgs []handler.WSRegistryMessage) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.Type)
	}
	return out
}
