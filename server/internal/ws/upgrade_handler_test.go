package ws

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/pkg/clockx"
)

// fakeBlacklist is a deterministic in-memory Blacklist for unit tests.
// It records call count so tests can assert priority (e.g. blacklist must
// be consulted before rate limiter).
type fakeBlacklist struct {
	blocked bool
	err     error
	calls   atomic.Int32
}

func (f *fakeBlacklist) IsBlacklisted(_ context.Context, _ string) (bool, error) {
	f.calls.Add(1)
	return f.blocked, f.err
}

// fakeLimiter is a deterministic ConnectRateLimiter for unit tests.
type fakeLimiter struct {
	decision ConnectDecision
	err      error
	calls    atomic.Int32
}

func (f *fakeLimiter) AcquireConnectSlot(_ context.Context, _ string) (ConnectDecision, error) {
	f.calls.Add(1)
	return f.decision, f.err
}

func newTestHandler(t *testing.T, bl Blacklist, rl ConnectRateLimiter) *UpgradeHandler {
	t.Helper()
	hub := NewHub(HubConfig{SendBufSize: 64}, clockx.NewRealClock())
	dispatcher := NewDispatcher(nil, clockx.NewRealClock())
	return NewUpgradeHandler(hub, dispatcher, NewDebugValidator(), bl, rl)
}

func doUpgradeRequest(t *testing.T, h *UpgradeHandler, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws", h.Handle)

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

func TestUpgradeHandler_EmptyToken_Returns401(t *testing.T) {
	t.Parallel()
	h := newTestHandler(t, nil, nil)
	rr := doUpgradeRequest(t, h, "")

	assert.Equal(t, http.StatusUnauthorized, rr.Code)
	assert.Contains(t, rr.Body.String(), "AUTH_INVALID_IDENTITY_TOKEN")
}

func TestUpgradeHandler_BlacklistHit_Returns403_AndSkipsLimiter(t *testing.T) {
	t.Parallel()
	bl := &fakeBlacklist{blocked: true}
	rl := &fakeLimiter{decision: ConnectDecision{Allowed: true}}

	h := newTestHandler(t, bl, rl)
	rr := doUpgradeRequest(t, h, "alice")

	assert.Equal(t, http.StatusForbidden, rr.Code)
	assert.Contains(t, rr.Body.String(), "DEVICE_BLACKLISTED")
	assert.Equal(t, int32(1), bl.calls.Load(), "blacklist must be consulted")
	assert.Equal(t, int32(0), rl.calls.Load(), "limiter must be skipped when blacklist hits — fatal trumps retry_after")
}

func TestUpgradeHandler_RateLimitExceeded_Returns429_WithRetryAfter(t *testing.T) {
	t.Parallel()
	bl := &fakeBlacklist{blocked: false}
	rl := &fakeLimiter{decision: ConnectDecision{Allowed: false, Count: 6, RetryAfter: 42 * time.Second}}

	h := newTestHandler(t, bl, rl)
	rr := doUpgradeRequest(t, h, "alice")

	assert.Equal(t, http.StatusTooManyRequests, rr.Code)
	assert.Contains(t, rr.Body.String(), "RATE_LIMIT_EXCEEDED")
	ra := rr.Header().Get("Retry-After")
	require.NotEmpty(t, ra)
	n, err := strconv.Atoi(ra)
	require.NoError(t, err)
	assert.Equal(t, 42, n)
}

func TestUpgradeHandler_RateLimitRetryAfterSubSecond_RoundsUpToOne(t *testing.T) {
	t.Parallel()
	bl := &fakeBlacklist{blocked: false}
	rl := &fakeLimiter{decision: ConnectDecision{Allowed: false, Count: 6, RetryAfter: 10 * time.Millisecond}}

	h := newTestHandler(t, bl, rl)
	rr := doUpgradeRequest(t, h, "alice")

	assert.Equal(t, http.StatusTooManyRequests, rr.Code)
	ra := rr.Header().Get("Retry-After")
	assert.Equal(t, "1", ra, "sub-second retry must round up to 1")
}

func TestUpgradeHandler_BlacklistError_FailClosed(t *testing.T) {
	t.Parallel()
	bl := &fakeBlacklist{err: errors.New("redis down")}
	rl := &fakeLimiter{decision: ConnectDecision{Allowed: true}}

	h := newTestHandler(t, bl, rl)
	rr := doUpgradeRequest(t, h, "alice")

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "INTERNAL_ERROR")
	assert.Equal(t, int32(0), rl.calls.Load(), "limiter must not be reached when blacklist errors")
}

func TestUpgradeHandler_LimiterError_FailClosed(t *testing.T) {
	t.Parallel()
	bl := &fakeBlacklist{blocked: false}
	rl := &fakeLimiter{err: errors.New("redis down")}

	h := newTestHandler(t, bl, rl)
	rr := doUpgradeRequest(t, h, "alice")

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, rr.Body.String(), "INTERNAL_ERROR")
}

func TestUpgradeHandler_NilGuards_DoNotReject(t *testing.T) {
	// With both guards nil the handler must not short-circuit at a guard.
	// The test request is a plain GET (no WS Upgrade headers) so upgrader.Upgrade
	// will reject with 400, but crucially NOT 403 or 429 — which proves the
	// guard path was skipped.
	t.Parallel()
	h := newTestHandler(t, nil, nil)
	rr := doUpgradeRequest(t, h, "alice")

	assert.NotEqual(t, http.StatusForbidden, rr.Code)
	assert.NotEqual(t, http.StatusTooManyRequests, rr.Code)
	assert.NotEqual(t, http.StatusInternalServerError, rr.Code)
}

func TestUpgradeHandler_StubValidator_Rejects(t *testing.T) {
	t.Parallel()
	hub := NewHub(HubConfig{SendBufSize: 64}, clockx.NewRealClock())
	dispatcher := NewDispatcher(nil, clockx.NewRealClock())
	h := NewUpgradeHandler(hub, dispatcher, NewStubValidator(), nil, nil)

	rr := doUpgradeRequest(t, h, "some-token")
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestCeilSeconds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   time.Duration
		want int
	}{
		{in: 0, want: 1},
		{in: time.Millisecond, want: 1},
		{in: time.Second, want: 1},
		{in: time.Second + 1, want: 2},
		{in: 42 * time.Second, want: 42},
		{in: 42*time.Second + 500*time.Millisecond, want: 43},
		{in: -time.Second, want: 1},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, ceilSeconds(tt.in), "ceilSeconds(%v)", tt.in)
	}
}
