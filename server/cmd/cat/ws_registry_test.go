package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/config"
	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/handler"
	"github.com/huing/cat/server/internal/ws"
	"github.com/huing/cat/server/pkg/clockx"
)

// stubHandlers builds a *handlers struct with the minimum wiring needed for
// buildRouter to compile and mount every route. Non-platform handlers are
// constructed with nil dependencies — their methods panic if called, but the
// AC9 test only exercises /v1/platform/ws-registry so they never run.
func stubHandlers(t *testing.T, clock clockx.Clock, mode string) *handlers {
	t.Helper()
	return &handlers{
		health:    handler.NewHealthHandler(nil, nil, nil, nil, 0),
		wsUpgrade: ws.NewUpgradeHandler(nil, nil, nil, nil, nil),
		platform:  handler.NewPlatformHandler(clock, mode),
	}
}

func fireRegistry(t *testing.T, cfg *config.Config, h *handlers) *httptest.ResponseRecorder {
	t.Helper()
	router := buildRouter(cfg, h)
	req, err := http.NewRequest(http.MethodGet, "/v1/platform/ws-registry", nil)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestWSRegistryEndpoint_ReleaseMode(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	fakeNow := time.Date(2026, 4, 18, 12, 34, 56, 0, time.UTC)
	h := stubHandlers(t, clockx.NewFakeClock(fakeNow), "release")

	w := fireRegistry(t, cfg, h)

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json; charset=utf-8", w.Header().Get("Content-Type"))

	var resp handler.WSRegistryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "v1", resp.APIVersion)
	assert.Equal(t, "2026-04-18T12:34:56Z", resp.ServerTime)

	// Release mode at Story 0.14: every WSMessages entry is DebugOnly, so
	// messages slice must be empty-but-not-null.
	for _, meta := range dto.WSMessages {
		if !meta.DebugOnly {
			found := false
			for _, m := range resp.Messages {
				if m.Type == meta.Type {
					found = true
					break
				}
			}
			assert.True(t, found, "release mode must surface non-DebugOnly entry %q", meta.Type)
		}
	}
	assert.True(t,
		strings.Contains(w.Body.String(), `"messages":[]`),
		"release mode messages slice must marshal as [] not null; body=%s", w.Body.String(),
	)
}

func TestWSRegistryEndpoint_DebugMode(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{}
	fakeNow := time.Date(2026, 4, 18, 12, 34, 56, 0, time.UTC)
	h := stubHandlers(t, clockx.NewFakeClock(fakeNow), "debug")

	w := fireRegistry(t, cfg, h)

	require.Equal(t, http.StatusOK, w.Code)

	var resp handler.WSRegistryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "v1", resp.APIVersion)

	require.Len(t, resp.Messages, 3)
	gotTypes := make(map[string]handler.WSRegistryMessage, len(resp.Messages))
	for _, m := range resp.Messages {
		gotTypes[m.Type] = m
	}
	assert.Contains(t, gotTypes, "session.resume")
	assert.Contains(t, gotTypes, "debug.echo")
	assert.Contains(t, gotTypes, "debug.echo.dedup")

	assert.False(t, gotTypes["session.resume"].RequiresDedup, "session.resume must NOT require dedup")
	assert.True(t, gotTypes["debug.echo.dedup"].RequiresDedup, "debug.echo.dedup MUST require dedup")
}

func TestWSRegistryEndpoint_ServerTimeUsesInjectedClock(t *testing.T) {
	t.Parallel()

	// Story 0.7 AC binding: the ws-registry endpoint is the first verified
	// consumer of clockx.Clock. A fake clock with a fixed instant must round-
	// trip to the response without touching time.Now().
	cfg := &config.Config{}
	fakeNow := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	h := stubHandlers(t, clockx.NewFakeClock(fakeNow), "debug")

	w := fireRegistry(t, cfg, h)
	var resp handler.WSRegistryResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	parsed, err := time.Parse(time.RFC3339, resp.ServerTime)
	require.NoError(t, err)
	assert.True(t, parsed.Equal(fakeNow))
}
