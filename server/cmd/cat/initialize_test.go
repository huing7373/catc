package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/handler"
	"github.com/huing/cat/server/internal/ws"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/jwtx"
	"github.com/huing/cat/server/pkg/redisx"
)

// stubJWTVerifier is a tiny fake — buildHTTPJWTAuth does not invoke
// it at construction time, so we just need a non-nil concrete value
// that satisfies middleware.JWTVerifier.
type stubJWTVerifier struct{}

func (stubJWTVerifier) Verify(_ string) (*jwtx.CustomClaims, error) {
	return nil, errors.New("unused")
}

// newRouterForTesting builds the real buildRouter with the minimum
// viable handlers struct so AC7 routing assertions exercise the
// production wire.go code path. Health / wsUpgrade are intentionally
// omitted (no test below hits /healthz, /readyz, /ws); platform is
// real because TestRouter_V1Group_DoesNotIntercept_PlatformRegistry
// asserts the actual handler runs.
func newRouterForTesting(jwtAuth gin.HandlerFunc, v1Routes func(*gin.RouterGroup)) *gin.Engine {
	platform := handler.NewPlatformHandler(clockx.NewRealClock(), "release")
	h := &handlers{
		platform: platform,
		jwtAuth:  jwtAuth,
		v1Routes: v1Routes,
	}
	return buildRouter(nil, h)
}

type fakeDedupStore struct{}

func (fakeDedupStore) Acquire(_ context.Context, _ string) (bool, error) { return true, nil }
func (fakeDedupStore) StoreResult(_ context.Context, _ string, _ redisx.DedupResult) error {
	return nil
}
func (fakeDedupStore) GetResult(_ context.Context, _ string) (redisx.DedupResult, bool, error) {
	return redisx.DedupResult{}, false, nil
}

func newStubDispatcher() *ws.Dispatcher {
	return ws.NewDispatcher(fakeDedupStore{}, clockx.NewRealClock())
}

func noopHandler(_ context.Context, _ *ws.Client, env ws.Envelope) (json.RawMessage, error) {
	return env.Payload, nil
}

func TestValidateRegistryConsistency_DebugModeFullyRegistered(t *testing.T) {
	t.Parallel()

	d := newStubDispatcher()
	d.Register("debug.echo", noopHandler)
	d.RegisterDedup("debug.echo.dedup", noopHandler)
	d.Register("session.resume", noopHandler)
	d.Register("room.join", noopHandler)
	d.Register("action.update", noopHandler)
	// action.broadcast is Direction=down; MUST NOT be registered. The
	// drift check exempts downstream-only types (Story 10.1).

	require.NoError(t, validateRegistryConsistency(d, "debug"))
}

func TestValidateRegistryConsistency_DownstreamOnlyExempt(t *testing.T) {
	t.Parallel()

	// action.broadcast is Direction=down: it lives in dto.WSMessages so the
	// /v1/platform/ws-registry endpoint advertises it, but it never flows
	// through Dispatcher (server→client push). The consistency check must
	// therefore exempt it from the "must be registered in debug mode"
	// requirement. Equally, registering a downstream type would also be
	// wrong — but that case is already caught by unknownRegistered in the
	// existing machinery (action.broadcast IS in WSMessages, so it
	// wouldn't trip unknownRegistered; the exemption is one-sided — we
	// don't test registering a downstream type because it would panic at
	// Dispatch time anyway when Direction conventions are respected).
	d := newStubDispatcher()
	d.Register("debug.echo", noopHandler)
	d.RegisterDedup("debug.echo.dedup", noopHandler)
	d.Register("session.resume", noopHandler)
	d.Register("room.join", noopHandler)
	d.Register("action.update", noopHandler)

	// Debug mode must accept this configuration despite action.broadcast
	// being in WSMessages but not registered.
	require.NoError(t, validateRegistryConsistency(d, "debug"))
}

func TestValidateRegistryConsistency_ReleaseMode(t *testing.T) {
	t.Parallel()

	// Story 1.1 flipped session.resume to non-DebugOnly. Release mode
	// must register it (the same way debug mode does) — without the
	// registration the dispatcher would return UNKNOWN_MESSAGE_TYPE
	// while the registry endpoint advertises the type.
	d := newStubDispatcher()
	d.Register("session.resume", noopHandler)
	require.NoError(t, validateRegistryConsistency(d, "release"))
}

func TestValidateRegistryConsistency_ReleaseModeMissingSessionResumeFails(t *testing.T) {
	t.Parallel()

	// Inverse of the above: forgetting to register session.resume in
	// release mode must trip the drift guard. Direct regression test
	// for the Story 1.1 flip.
	d := newStubDispatcher()
	err := validateRegistryConsistency(d, "release")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session.resume")
	assert.Contains(t, err.Error(), "missingInRelease")
}

func TestValidateRegistryConsistency_UnknownRegisteredFails(t *testing.T) {
	t.Parallel()

	d := newStubDispatcher()
	d.Register("ghost.type", noopHandler)

	err := validateRegistryConsistency(d, "debug")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost.type",
		"error must name the drifting type for triage")
	assert.Contains(t, err.Error(), "unknownRegistered",
		"error must classify the drift bucket")
}

func TestValidateRegistryConsistency_DebugModeMissingRegistrationFails(t *testing.T) {
	t.Parallel()

	// Debug mode drift the endpoint can actually expose: dto.WSMessages still
	// lists a type (advertised to clients on /v1/platform/ws-registry) but
	// the dispatcher registration was removed / forgotten, so clients that
	// send it get UNKNOWN_MESSAGE_TYPE. Simulate by registering only a
	// proper subset of the three WSMessages entries.
	d := newStubDispatcher()
	d.Register("debug.echo", noopHandler)
	d.RegisterDedup("debug.echo.dedup", noopHandler)
	// session.resume deliberately missing.

	err := validateRegistryConsistency(d, "debug")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session.resume",
		"error must name the drifting type for triage")
	assert.Contains(t, err.Error(), "missingInDebug",
		"error must classify the drift bucket")
}

// TestBuildHTTPJWTAuth_ReleaseModeMounted locks the AC8 release-side
// branch: any non-debug mode wires middleware.JWTAuth(jwtMgr) so the
// /v1/* group is actually guarded in production. Pairs with
// TestBuildHTTPJWTAuth_DebugModeNotMounted to defeat the
// review-antipatterns §7.1 "gate written backwards" failure mode —
// either test alone would let `if mode == "debug"` (release mounts
// nothing!) pass undetected.
func TestBuildHTTPJWTAuth_ReleaseModeMounted(t *testing.T) {
	t.Parallel()
	got := buildHTTPJWTAuth("release", stubJWTVerifier{})
	require.NotNil(t, got, "release mode MUST mount JWTAuth on /v1/*")
}

func TestBuildHTTPJWTAuth_DebugModeNotMounted(t *testing.T) {
	t.Parallel()
	got := buildHTTPJWTAuth("debug", stubJWTVerifier{})
	assert.Nil(t, got, "debug mode MUST NOT mount JWTAuth (no /v1/* business endpoint yet)")
}

// TestBuildHTTPJWTAuth_UnknownModeTreatedAsRelease keeps the gate
// fail-closed for typos / future modes — anything that isn't
// literally "debug" mounts the middleware.
func TestBuildHTTPJWTAuth_UnknownModeTreatedAsRelease(t *testing.T) {
	t.Parallel()
	got := buildHTTPJWTAuth("staging", stubJWTVerifier{})
	require.NotNil(t, got, "unknown mode must default to release-style fail-closed wiring")
}

// TestRouter_V1Group_DoesNotIntercept_PlatformRegistry locks the AC7
// drift case: even with httpJWTAuth wired on /v1, the explicit
// top-level GET /v1/platform/ws-registry MUST take precedence. A
// regression here would silently 401 a pre-auth client probe.
func TestRouter_V1Group_DoesNotIntercept_PlatformRegistry(t *testing.T) {
	t.Parallel()

	// Use a stub middleware that always 401s — if it ever runs on
	// /v1/platform/ws-registry, the test fails because the body
	// won't be the platform handler's response.
	always401 := func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "MIDDLEWARE_RAN"}})
	}

	r := newRouterForTesting(always401, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/platform/ws-registry", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code,
		"platform/ws-registry must skip JWTAuth — got %d, body=%s", w.Code, w.Body.String())
	assert.NotContains(t, w.Body.String(), "MIDDLEWARE_RAN",
		"platform/ws-registry must NOT pass through JWTAuth")
}

// TestRouter_V1Group_RejectsUnauthenticated locks the positive path:
// any /v1/* endpoint registered via v1Routes IS guarded by
// httpJWTAuth and returns the middleware's reject (401 stub).
func TestRouter_V1Group_RejectsUnauthenticated(t *testing.T) {
	t.Parallel()

	always401 := func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "AUTH_TOKEN_EXPIRED"}})
	}

	echoCalled := false
	r := newRouterForTesting(always401, func(g *gin.RouterGroup) {
		g.GET("/echo", func(c *gin.Context) {
			echoCalled = true
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/echo", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.False(t, echoCalled, "downstream /v1/echo MUST NOT execute when JWTAuth rejects")
	assert.Contains(t, w.Body.String(), "AUTH_TOKEN_EXPIRED")
}

// TestRouter_Bootstrap_NoAuth proves /auth/apple lives outside the
// JWT group. With JWTAuth installed as always-401, hitting
// /auth/apple must NOT return 401 — instead it reaches the auth
// handler (which then validates the body and returns its own error).
func TestRouter_Bootstrap_NoAuth(t *testing.T) {
	t.Parallel()

	always401 := func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "MIDDLEWARE_RAN"}})
	}

	r := newRouterForTesting(always401, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/auth/apple", nil)
	r.ServeHTTP(w, req)

	// /auth/apple is bootstrap; its handler will return its own
	// validation error for an empty body. We just need to assert the
	// JWTAuth stub did NOT intercept.
	assert.NotContains(t, w.Body.String(), "MIDDLEWARE_RAN",
		"/auth/apple must skip JWTAuth — bootstrap endpoint")
}

// TestRouter_V1Group_404OnUnknownRoute proves the v1 group itself
// does not eat unknown paths — without a registered route, /v1/x
// returns 404 (gin tree miss), not 401 (middleware match). This is
// the cheap-to-maintain version of the "test-tag echo route" the AC
// alternative suggested.
func TestRouter_V1Group_404OnUnknownRoute(t *testing.T) {
	t.Parallel()

	always401 := func(c *gin.Context) {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "MIDDLEWARE_RAN"}})
	}

	r := newRouterForTesting(always401, nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/no-such-route", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code,
		"unknown /v1/* route must 404 (gin tree miss), not 401 (middleware match)")
}

func TestValidateRegistryConsistency_DebugOnlyInReleaseFails(t *testing.T) {
	t.Parallel()

	// Registering a DebugOnly entry (debug.echo — session.resume left
	// DebugOnly via Story 1.1 so it no longer demonstrates this case)
	// in release mode is the Story 0.12 regression the guard catches.
	// session.resume MUST also be registered in release mode after the
	// 1.1 flip, otherwise we hit `missingInRelease` first.
	d := newStubDispatcher()
	d.Register("session.resume", noopHandler)
	d.Register("debug.echo", noopHandler)

	err := validateRegistryConsistency(d, "release")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "debug.echo")
	assert.Contains(t, err.Error(), "debugOnlyInRelease")
}
