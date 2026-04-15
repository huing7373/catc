package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/huing7373/catc/server/internal/config"
	"github.com/huing7373/catc/server/internal/handler"
	"github.com/huing7373/catc/server/internal/middleware"
	"github.com/huing7373/catc/server/internal/repository"
	"github.com/huing7373/catc/server/internal/service"
	"github.com/huing7373/catc/server/internal/ws"
	"github.com/huing7373/catc/server/pkg/ids"
	"github.com/huing7373/catc/server/pkg/jwtx"
)

func init() { gin.SetMode(gin.TestMode) }

// TestMustNewJWT_ForwardsPreviousSecrets is the regression guard for
// the wiring bug where mustNewJWT dropped the previous-secret pair on
// the floor. We mint a token with a previous-only manager and parse it
// with the dual-secret manager built via mustNewJWT — the parse must
// succeed if rotation forwarding works, and fail otherwise.
func TestMustNewJWT_ForwardsPreviousSecrets(t *testing.T) {
	cfg := config.JWTCfg{
		AccessSecret:          "current-access",
		RefreshSecret:         "current-refresh",
		AccessSecretPrevious:  "previous-access",
		RefreshSecretPrevious: "previous-refresh",
		Issuer:                "cat-test",
	}
	prev, err := jwtx.New(jwtx.Config{
		AccessSecret:  "previous-access",
		RefreshSecret: "previous-refresh",
		AccessTTL:     time.Hour,
		RefreshTTL:    time.Hour,
	})
	if err != nil {
		t.Fatalf("prev: %v", err)
	}
	mgr := mustNewJWT(cfg, time.Hour, time.Hour)

	// Mint with the OLD secret, parse with the production-wired manager.
	at, err := prev.SignAccess(ids.UserID("u1"))
	if err != nil {
		t.Fatalf("SignAccess: %v", err)
	}
	uid, err := mgr.ParseAccess(at)
	if err != nil {
		t.Fatalf("ParseAccess of previous-signed token failed — wiring dropped previous secret: %v", err)
	}
	if uid != "u1" {
		t.Errorf("uid: %q", uid)
	}

	rt, err := prev.SignRefresh(ids.UserID("u1"))
	if err != nil {
		t.Fatalf("SignRefresh: %v", err)
	}
	if _, err := mgr.ParseRefresh(rt); err != nil {
		t.Fatalf("ParseRefresh of previous-signed token failed: %v", err)
	}
}

// stubAuthSvc is a no-op AuthSvc used only to keep buildRouter happy
// during the rate-limit smoke test. The login path is never reached
// from the 11th request (rate-limit aborts first) and earlier requests
// just return a dummy 200, so we don't need real Apple verification.
type stubAuthSvc struct{}

func (stubAuthSvc) Login(context.Context, service.LoginInput) (service.TokenPair, error) {
	return service.TokenPair{UserID: "u-stub", AccessToken: "AT", RefreshToken: "RT"}, nil
}
func (stubAuthSvc) Refresh(context.Context, service.RefreshInput) (service.TokenPair, error) {
	return service.TokenPair{UserID: "u-stub", AccessToken: "AT", RefreshToken: "RT"}, nil
}

// TestBuildRouter_AuthRouteRateLimited429 is the router-level contract
// test. It wires the actual buildRouter with a real RedisLimiter
// (miniredis) plus a stub AuthSvc, fires 11 POST /v1/auth/login from
// the same client IP, and asserts:
//   - the first 10 succeed (200)
//   - the 11th is 429 with body code "RATE_LIMITED"
//   - it includes the standard error envelope shape
//
// This catches regressions in: route group path, middleware order,
// limiter wiring, and rate value (10/min). Unit-testing handler.Login
// alone (auth_test.go) cannot detect a missing /v1/auth group or a
// dropped middleware.
func TestBuildRouter_AuthRouteRateLimited429(t *testing.T) {
	srv, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(srv.Close)
	rdb := redis.NewClient(&redis.Options{Addr: srv.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })

	authLimiter := middleware.NewRedisLimiter(rdb, "auth-login", 60, repository.RateLimitKey)

	cfg := &config.Config{
		Server: config.ServerCfg{Port: 0, Mode: "release"},
	}
	jwtMgr, err := jwtx.New(jwtx.Config{
		AccessSecret: "x", RefreshSecret: "y", AccessTTL: time.Hour, RefreshTTL: time.Hour,
	})
	if err != nil {
		t.Fatalf("jwt: %v", err)
	}
	h := handlers{
		health: handler.NewHealthHandler(
			func(context.Context) error { return nil },
			func(context.Context) error { return nil },
		),
		auth: handler.NewAuthHandler(stubAuthSvc{}),
		// Real WSHandler with real Hub/Router — we never exercise the
		// /v1/ws route in this test; it's only here so buildRouter can
		// register it without nil-deref.
		ws: handler.NewWSHandler(ws.NewHub(), ws.NewRouter(), jwtMgr, nil),
	}

	router := buildRouter(cfg, h, jwtMgr, authLimiter)

	body := []byte(`{"apple_jwt":"x","nonce":"sixteen-chars-ok-x","device_id":"d"}`)
	hit := func() (int, map[string]any) {
		req := httptest.NewRequest(http.MethodPost, "/v1/auth/login", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		// Stable client IP so the per-IP bucket actually accumulates.
		req.RemoteAddr = "203.0.113.42:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		var decoded map[string]any
		if w.Body.Len() > 0 {
			_ = json.Unmarshal(w.Body.Bytes(), &decoded)
		}
		return w.Code, decoded
	}

	// First 10 must pass.
	for i := 0; i < 10; i++ {
		code, _ := hit()
		if code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, code)
		}
	}
	// 11th must hit the rate-limit middleware.
	code, body11 := hit()
	if code != http.StatusTooManyRequests {
		t.Fatalf("11th request: expected 429, got %d", code)
	}
	envelope, ok := body11["error"].(map[string]any)
	if !ok {
		t.Fatalf("11th body missing error envelope: %+v", body11)
	}
	if envelope["code"] != "RATE_LIMITED" {
		t.Errorf("11th code: %v, want RATE_LIMITED", envelope["code"])
	}
}

// strconv kept available for future per-IP fan-out variants of the
// test (e.g. asserting per-IP isolation). Currently unused but cheap.
var _ = strconv.Itoa
