//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	gws "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/handler"
	"github.com/huing/cat/server/internal/middleware"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/internal/service"
	"github.com/huing/cat/server/internal/testutil"
	"github.com/huing/cat/server/internal/ws"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/jwtx"
	"github.com/huing/cat/server/pkg/mongox"
	"github.com/huing/cat/server/pkg/redisx"
)

// TestJWTMiddleware_EndToEnd locks AC9 — the full Sign-in-with-Apple →
// JWTAuth middleware → guarded /v1/* handler chain runs through real
// Mongo + miniredis + a real Apple JWKS fetcher pointed at FakeApple.
// Three subtests cover the matrix:
//
//  1. HappyPath — SIWA returns access+refresh; access opens /v1/_test/echo
//     and the echo body confirms (userId, deviceId, platform) propagated.
//  2. MissingToken — no Authorization header → 401 AUTH_TOKEN_EXPIRED.
//  3. ExpiredToken — FakeClock advance past access expiry → 401
//     AUTH_INVALID_IDENTITY_TOKEN (release mode collapses exp into
//     INVALID_IDENTITY_TOKEN per AC2 step 3 / AC11 row 2 decision).
//  4. RefreshTokenRejected — refresh token presented as Bearer → 401
//     AUTH_INVALID_IDENTITY_TOKEN (refresh MUST NEVER unlock /v1/*,
//     even though Verify accepts it at the library layer).
//  5. WSUpgradeExtendsClaimsToClient — dial /ws with the access token,
//     send debug.echo, assert the server-side handler observed
//     deviceID/platform on the *ws.Client (AC5 contract end-to-end).
func TestJWTMiddleware_EndToEnd(t *testing.T) {
	h := setupJWTAuthHarness(t)

	t.Run("HappyPath", func(t *testing.T) {
		dev := uuid.NewString()
		siwa := h.signIn(t, "apple:test:happy-jwt-mw", dev, "iphone")

		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/v1/_test/echo", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+siwa.AccessToken)
		h.r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())
		var body map[string]string
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, siwa.User.ID, body["userId"])
		assert.Equal(t, dev, body["deviceId"])
		assert.Equal(t, "iphone", body["platform"])
	})

	t.Run("MissingToken_Returns401", func(t *testing.T) {
		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/v1/_test/echo", nil)
		require.NoError(t, err)
		h.r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "AUTH_TOKEN_EXPIRED",
			"missing Authorization header must surface AUTH_TOKEN_EXPIRED — clients route to refresh")
	})

	t.Run("ExpiredToken_Returns401", func(t *testing.T) {
		dev := uuid.NewString()
		siwa := h.signIn(t, "apple:test:expired-jwt-mw", dev, "watch")

		// Advance the FakeClock past access TTL (900s configured in
		// harness). FakeClock has no Set method (clock.go), so we do
		// not restore — subsequent subtests issue NEW tokens via
		// signIn() which read Now() from the now-advanced clock and
		// remain in their own [issued, issued+15min] valid window;
		// Verify shares the same clock so post-advance tokens still
		// validate. The order matters only for the EXPIRED case, and
		// that is what this subtest itself controls.
		h.fa.Clock.Advance(20 * time.Minute)

		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/v1/_test/echo", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+siwa.AccessToken)
		h.r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "AUTH_INVALID_IDENTITY_TOKEN",
			"expired access tokens collapse to AUTH_INVALID_IDENTITY_TOKEN per AC2 step 3 / AC11")
	})

	t.Run("RefreshTokenRejected", func(t *testing.T) {
		dev := uuid.NewString()
		siwa := h.signIn(t, "apple:test:refresh-as-access", dev, "iphone")

		w := httptest.NewRecorder()
		req, err := http.NewRequest(http.MethodGet, "/v1/_test/echo", nil)
		require.NoError(t, err)
		// Send the refresh token as if it were access.
		req.Header.Set("Authorization", "Bearer "+siwa.RefreshToken)
		h.r.ServeHTTP(w, req)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "AUTH_INVALID_IDENTITY_TOKEN",
			"refresh-as-access MUST be rejected — TTL is 30d ≫ access 15min")
	})

	t.Run("WSUpgrade_ExtendsClaimsToClient", func(t *testing.T) {
		dev := uuid.NewString()
		siwa := h.signIn(t, "apple:test:ws-claims", dev, "watch")

		// Stand up the test httptest server that exposes /ws via the
		// real UpgradeHandler wired with the production JWTValidator.
		srv := httptest.NewServer(h.r)
		defer srv.Close()

		url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
		hdr := http.Header{}
		hdr.Set("Authorization", "Bearer "+siwa.AccessToken)
		conn, resp, err := gws.DefaultDialer.Dial(url, hdr)
		require.NoError(t, err, "ws dial failed; status=%d", func() int {
			if resp != nil {
				return resp.StatusCode
			}
			return -1
		}())
		defer conn.Close()
		assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

		// Send the test-only echo message; the handler reads
		// client.DeviceID() / Platform() back into the response payload.
		env := map[string]any{
			"id":      "req-claims",
			"type":    "_test.identity_echo",
			"payload": map[string]any{},
		}
		require.NoError(t, conn.WriteJSON(env))

		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, msg, err := conn.ReadMessage()
		require.NoError(t, err)

		var out ws.Response
		require.NoError(t, json.Unmarshal(msg, &out))
		require.True(t, out.OK, "_test.identity_echo failed: %s", string(out.Payload))
		var payload map[string]string
		require.NoError(t, json.Unmarshal(out.Payload, &payload))
		assert.Equal(t, siwa.User.ID, payload["userId"],
			"WS Client.UserID must equal SIWA-issued UserID")
		assert.Equal(t, dev, payload["deviceId"],
			"AC5: WS upgrade MUST propagate deviceId from CustomClaims into Client.DeviceID()")
		assert.Equal(t, "watch", payload["platform"],
			"AC5: WS upgrade MUST propagate platform into Client.Platform()")
	})
}

// jwtAuthHarness is the integration-test harness for AC9: real Mongo,
// miniredis, FakeApple JWKS, jwtx.Manager, AuthService, and the full
// gin stack with /v1/* JWTAuth middleware AND a test-only echo
// endpoint (lives in the harness so production wire.go stays clean).
type jwtAuthHarness struct {
	r      *gin.Engine
	jwtMgr *jwtx.Manager
	fa     *testutil.FakeApple
}

func setupJWTAuthHarness(t *testing.T) *jwtAuthHarness {
	t.Helper()

	cli, cleanup := testutil.SetupMongo(t)
	t.Cleanup(cleanup)

	dbName := "test_jwtmw_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	mongoCli := mongox.WrapClient(cli, dbName)
	t.Cleanup(func() { _ = cli.Database(dbName).Drop(context.Background()) })

	fa := testutil.NewFakeApple(t, "com.test.cat")
	clk := fa.Clock

	serverPriv := mustGenRSA(t)
	signingKeyPath := writePrivateKeyPEM(t, serverPriv)

	jwtMgr := jwtx.NewManagerWithApple(jwtx.Options{
		PrivateKeyPath:   signingKeyPath,
		ActiveKID:        "kid-test-jwtmw",
		Issuer:           "test-cat",
		AccessExpirySec:  900,
		RefreshExpirySec: 2592000,
	}, jwtx.AppleVerifyDeps{
		Fetcher:  fa.Fetcher,
		BundleID: fa.BundleID,
		Clock:    clk,
	})

	userRepo := repository.NewMongoUserRepository(mongoCli.DB(), clk)
	require.NoError(t, userRepo.EnsureIndexes(context.Background()))

	refreshBlacklist := redisx.NewRefreshBlacklist(fa.Redis, clk)
	authSvc := service.NewAuthService(
		userRepo,
		jwtMgr, // AppleVerifier
		jwtMgr, // RefreshVerifier
		jwtMgr, // JWTIssuer
		refreshBlacklist,
		clk,
		"release",
	)
	authHandler := handler.NewAuthHandler(authSvc)

	// WS plumbing — Hub + Dispatcher + JWTValidator (release-mode wiring).
	hub := ws.NewHub(ws.HubConfig{
		PingInterval: 30 * time.Second,
		PongTimeout:  60 * time.Second,
		SendBufSize:  64,
	}, clockx.NewRealClock())
	dispatcher := ws.NewDispatcher(nil, clockx.NewRealClock())
	// _test.identity_echo reads back the (userId, deviceId, platform)
	// from the *ws.Client so the WSUpgrade subtest can verify AC5
	// claim propagation. Lives ONLY in the integration test (build tag
	// integration) so it never appears in production wire.go.
	dispatcher.Register("_test.identity_echo", func(_ context.Context, c *ws.Client, _ ws.Envelope) (json.RawMessage, error) {
		return json.Marshal(map[string]string{
			"userId":   c.UserID(),
			"deviceId": c.DeviceID(),
			"platform": c.Platform(),
		})
	})
	upgradeHandler := ws.NewUpgradeHandler(hub, dispatcher, ws.NewJWTValidator(jwtMgr), nil, nil)
	t.Cleanup(func() { _ = hub.Final(context.Background()) })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	// Bootstrap: bypass JWTAuth.
	r.POST("/auth/apple", authHandler.SignInWithApple)
	r.POST("/auth/refresh", authHandler.Refresh)
	r.GET("/ws", upgradeHandler.Handle)

	// /v1/* group with the production JWTAuth middleware. The
	// _test/echo route is harness-only — it returns the (userId,
	// deviceId, platform) injected by the middleware so we can prove
	// the full SIWA → middleware → handler chain works end-to-end.
	v1 := r.Group("/v1")
	v1.Use(middleware.JWTAuth(jwtMgr))
	v1.GET("/_test/echo", func(c *gin.Context) {
		c.JSON(http.StatusOK, map[string]string{
			"userId":   string(middleware.UserIDFrom(c)),
			"deviceId": middleware.DeviceIDFrom(c),
			"platform": string(middleware.PlatformFrom(c)),
		})
	})

	return &jwtAuthHarness{
		r:      r,
		jwtMgr: jwtMgr,
		fa:     fa,
	}
}

func (h *jwtAuthHarness) signIn(t *testing.T, sub, deviceID, platform string) dto.SignInWithAppleResponse {
	t.Helper()
	rawNonce := "nonce-" + uuid.NewString()
	tok := h.fa.SignIdentityToken(t, testutil.SignOptions{
		Sub:   sub,
		Nonce: hexSHA256(rawNonce),
	})
	body, err := json.Marshal(dto.SignInWithAppleRequest{
		IdentityToken: tok,
		DeviceID:      deviceID,
		Platform:      platform,
		Nonce:         rawNonce,
	})
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, "/auth/apple", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	h.r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "SIWA failed: %s", w.Body.String())

	var out dto.SignInWithAppleResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out
}
