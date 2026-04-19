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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/handler"
	"github.com/huing/cat/server/internal/middleware"
	"github.com/huing/cat/server/internal/push"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/internal/service"
	"github.com/huing/cat/server/internal/testutil"
	"github.com/huing/cat/server/internal/ws"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/cryptox"
	"github.com/huing/cat/server/pkg/ids"
	"github.com/huing/cat/server/pkg/jwtx"
	"github.com/huing/cat/server/pkg/mongox"
	"github.com/huing/cat/server/pkg/redisx"
)

const apnsTokenTestValidHex = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
const apnsTokenTestValidHex2 = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"

// apnsTokenHarness is the integration-test harness for Story 1.4. It
// extends setupJWTAuthHarness with the APNs token repository + device
// handler + APNs router wired against the real repo — so every test
// below exercises the full SIWA → JWTAuth → DeviceHandler → service →
// repo (sealer) → Mongo chain. Separate from jwtAuthHarness because we
// need to drive Register rate-limit thresholds and directly inspect
// Mongo to verify encryption-at-rest.
type apnsTokenHarness struct {
	r         *gin.Engine
	jwtMgr    *jwtx.Manager
	fa        *testutil.FakeApple
	repo      *repository.MongoApnsTokenRepository
	router    *push.APNsRouter
	rawTokens *mongo.Collection
	sealer    *cryptox.AESGCMSealer
}

// signIn duplicates jwtAuthHarness.signIn so this file stays
// self-contained; we pass the same sub across platforms where tests
// need a single user on multiple device classes.
func (h *apnsTokenHarness) signIn(t *testing.T, sub, deviceID, platform string) dto.SignInWithAppleResponse {
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

// registerToken issues POST /v1/devices/apns-token with the given
// access token. Returns the recorder so tests can assert status + body.
func (h *apnsTokenHarness) registerToken(t *testing.T, accessToken, deviceToken, platform string) *httptest.ResponseRecorder {
	t.Helper()
	body := map[string]any{"deviceToken": deviceToken}
	if platform != "" {
		body["platform"] = platform
	}
	b, err := json.Marshal(body)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, "/v1/devices/apns-token", bytes.NewReader(b))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	h.r.ServeHTTP(w, req)
	return w
}

// setupApnsTokenHarness wires up the full stack used by Story 1.4
// integration tests. ratePerWindow lets callers crank the limiter down
// (default 5) so the rate-limit subtest does not require firing 5+ real
// upserts. encryptionKey is always the 32-byte zero key unless a test
// needs to drive the sealer-mismatch path.
func setupApnsTokenHarness(t *testing.T, ratePerWindow int) *apnsTokenHarness {
	t.Helper()

	cli, cleanup := testutil.SetupMongo(t)
	t.Cleanup(cleanup)

	dbName := "test_apnstok_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	mongoCli := mongox.WrapClient(cli, dbName)
	t.Cleanup(func() { _ = cli.Database(dbName).Drop(context.Background()) })

	fa := testutil.NewFakeApple(t, "com.test.cat")
	clk := fa.Clock

	serverPriv := mustGenRSA(t)
	signingKeyPath := writePrivateKeyPEM(t, serverPriv)
	jwtMgr := jwtx.NewManagerWithApple(jwtx.Options{
		PrivateKeyPath:   signingKeyPath,
		ActiveKID:        "kid-test-apnstok",
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
		userRepo, jwtMgr, jwtMgr, jwtMgr,
		refreshBlacklist, clk, "release",
	)
	authHandler := handler.NewAuthHandler(authSvc)

	sealer, err := cryptox.NewAESGCMSealer(make([]byte, 32))
	require.NoError(t, err)
	apnsRepo := repository.NewMongoApnsTokenRepository(mongoCli.DB(), clk, sealer)
	require.NoError(t, apnsRepo.EnsureIndexes(context.Background()))

	limiter := redisx.NewUserSlidingWindowLimiter(
		fa.Redis, clk,
		"ratelimit:apns_token:",
		int64(ratePerWindow),
		60*time.Second,
	)
	apnsTokenSvc := service.NewApnsTokenService(apnsRepo, userRepo, limiter, clk)
	deviceHandler := handler.NewDeviceHandler(apnsTokenSvc)

	router := push.NewAPNsRouter(apnsRepo, "com.test.cat.watchkitapp", "com.test.cat")

	// WS plumbing — minimal since these tests do not exercise /ws but
	// setupJWTAuthHarness peer precedent has it. Dispatcher/Hub omitted
	// for brevity.

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.POST("/auth/apple", authHandler.SignInWithApple)
	r.POST("/auth/refresh", authHandler.Refresh)
	v1 := r.Group("/v1")
	v1.Use(middleware.JWTAuth(jwtMgr))
	v1.POST("/devices/apns-token", deviceHandler.RegisterApnsToken)

	return &apnsTokenHarness{
		r:         r,
		jwtMgr:    jwtMgr,
		fa:        fa,
		repo:      apnsRepo,
		router:    router,
		rawTokens: mongoCli.DB().Collection("apns_tokens"),
		sealer:    sealer,
	}
}

// ws unused — keep the import alive for future tests that want a WS
// conn alongside the HTTP register call.
var _ = ws.NewHub
var _ = clockx.NewRealClock

// --- tests ---

func TestApnsToken_Integration_Register_HappyPath(t *testing.T) {
	h := setupApnsTokenHarness(t, 5)
	dev := uuid.NewString()
	siwa := h.signIn(t, "apple:test:apns-happy", dev, "watch")

	w := h.registerToken(t, siwa.AccessToken, apnsTokenTestValidHex, "watch")
	require.Equal(t, http.StatusOK, w.Code, "body=%s", w.Body.String())

	var body dto.RegisterApnsTokenResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.True(t, body.Ok)

	// Verify at-rest encryption — device_token bytes in Mongo must NOT
	// equal the plaintext hex, yet our sealer must round-trip.
	var raw bson.M
	err := h.rawTokens.FindOne(context.Background(), bson.M{"user_id": siwa.User.ID}).Decode(&raw)
	require.NoError(t, err)
	bin, ok := raw["device_token"].(bson.Binary)
	require.True(t, ok)
	assert.NotEqual(t, apnsTokenTestValidHex, string(bin.Data),
		"device_token must be sealed at rest, not plaintext")
	pt, err := h.sealer.Open(bin.Data)
	require.NoError(t, err)
	assert.Equal(t, apnsTokenTestValidHex, string(pt))
}

func TestApnsToken_Integration_Register_MissingAuth_401(t *testing.T) {
	h := setupApnsTokenHarness(t, 5)
	w := h.registerToken(t, "", apnsTokenTestValidHex, "watch")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "AUTH_TOKEN_EXPIRED")
}

func TestApnsToken_Integration_Register_RefreshTokenRejected_401(t *testing.T) {
	h := setupApnsTokenHarness(t, 5)
	siwa := h.signIn(t, "apple:test:apns-ref-as-acc", uuid.NewString(), "watch")
	w := h.registerToken(t, siwa.RefreshToken, apnsTokenTestValidHex, "watch")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "AUTH_INVALID_IDENTITY_TOKEN",
		"refresh-as-access MUST reject on /v1/devices/apns-token")
}

func TestApnsToken_Integration_Register_InvalidDeviceToken_400(t *testing.T) {
	h := setupApnsTokenHarness(t, 5)
	siwa := h.signIn(t, "apple:test:apns-bad-hex", uuid.NewString(), "watch")
	w := h.registerToken(t, siwa.AccessToken, "ZZZZZZZZ", "watch")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "VALIDATION_ERROR")
}

func TestApnsToken_Integration_Register_PlatformMismatch_400(t *testing.T) {
	h := setupApnsTokenHarness(t, 5)
	siwa := h.signIn(t, "apple:test:apns-pmismatch", uuid.NewString(), "watch")
	w := h.registerToken(t, siwa.AccessToken, apnsTokenTestValidHex, "iphone")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "VALIDATION_ERROR")
}

func TestApnsToken_Integration_Register_ReRegister_OverwritesSamePlatform(t *testing.T) {
	h := setupApnsTokenHarness(t, 10)
	dev := uuid.NewString()
	siwa := h.signIn(t, "apple:test:apns-rereg", dev, "watch")

	w := h.registerToken(t, siwa.AccessToken, apnsTokenTestValidHex, "watch")
	require.Equal(t, http.StatusOK, w.Code)
	w = h.registerToken(t, siwa.AccessToken, apnsTokenTestValidHex2, "watch")
	require.Equal(t, http.StatusOK, w.Code)

	toks, err := h.repo.ListByUserID(context.Background(), ids.UserID(siwa.User.ID))
	require.NoError(t, err)
	require.Len(t, toks, 1)
	assert.Equal(t, apnsTokenTestValidHex2, toks[0].DeviceToken)
}

func TestApnsToken_Integration_Register_CrossPlatformCoexists(t *testing.T) {
	h := setupApnsTokenHarness(t, 10)

	// Same underlying Apple account, two separate devices/platforms.
	siwaWatch := h.signIn(t, "apple:test:apns-cross", uuid.NewString(), "watch")
	w := h.registerToken(t, siwaWatch.AccessToken, apnsTokenTestValidHex, "watch")
	require.Equal(t, http.StatusOK, w.Code)

	siwaPhone := h.signIn(t, "apple:test:apns-cross", uuid.NewString(), "iphone")
	w = h.registerToken(t, siwaPhone.AccessToken, apnsTokenTestValidHex2, "iphone")
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, siwaWatch.User.ID, siwaPhone.User.ID,
		"second SIWA with same Apple sub must resolve to the same UserID")

	toks, err := h.repo.ListByUserID(context.Background(), ids.UserID(siwaWatch.User.ID))
	require.NoError(t, err)
	assert.Len(t, toks, 2)
}

func TestApnsToken_Integration_Register_RateLimitBlocks(t *testing.T) {
	h := setupApnsTokenHarness(t, 3)
	siwa := h.signIn(t, "apple:test:apns-rate", uuid.NewString(), "watch")

	for i := 0; i < 3; i++ {
		w := h.registerToken(t, siwa.AccessToken, apnsTokenTestValidHex, "watch")
		require.Equal(t, http.StatusOK, w.Code, "attempt %d body=%s", i+1, w.Body.String())
	}
	w := h.registerToken(t, siwa.AccessToken, apnsTokenTestValidHex, "watch")
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Contains(t, w.Body.String(), "RATE_LIMIT_EXCEEDED")
	assert.NotEmpty(t, w.Header().Get("Retry-After"),
		"rate-limited response MUST carry Retry-After header")
}

// TestApnsToken_Integration_Register_PusherChainIntact proves the §21.2
// Empty→Real swap is genuine — calling Router.RouteTokens goes through
// the real apnsTokenRepo (not EmptyTokenProvider) and surfaces the
// plaintext deviceToken registered by the happy-path HTTP call. If
// initialize.go still wired push.EmptyTokenProvider{} in place of the
// repo, this test would get an empty RouteTokens result.
func TestApnsToken_Integration_Register_PusherChainIntact(t *testing.T) {
	h := setupApnsTokenHarness(t, 5)
	siwa := h.signIn(t, "apple:test:apns-pusher", uuid.NewString(), "watch")

	w := h.registerToken(t, siwa.AccessToken, apnsTokenTestValidHex, "watch")
	require.Equal(t, http.StatusOK, w.Code)

	// Router.RouteTokens consults the real TokenProvider (apnsTokenRepo)
	// — if Empty were still wired, infos would be empty and out would be
	// nil/empty.
	routed, err := h.router.RouteTokens(context.Background(), ids.UserID(siwa.User.ID))
	require.NoError(t, err)
	require.Len(t, routed, 1,
		"§21.2 Empty→Real swap: router must see the registered token")
	assert.Equal(t, apnsTokenTestValidHex, routed[0].DeviceToken)
	assert.Equal(t, "com.test.cat.watchkitapp", routed[0].Topic)
	assert.Equal(t, "watch", routed[0].Platform)
}
