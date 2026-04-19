//go:build integration

package main

import (
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
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/handler"
	"github.com/huing/cat/server/internal/middleware"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/internal/service"
	"github.com/huing/cat/server/internal/testutil"
	"github.com/huing/cat/server/internal/ws"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
	"github.com/huing/cat/server/pkg/jwtx"
	"github.com/huing/cat/server/pkg/mongox"
	"github.com/huing/cat/server/pkg/redisx"
)

// profileHarness is the integration-test harness for Story 1.5. It
// spins up: real Mongo (Testcontainers), miniredis (via FakeApple),
// SIWA → JWT → WS stack with Dispatcher.RegisterDedup("profile.update",
// ...). Each test dials /ws with a Bearer token, sends a
// profile.update envelope, and asserts both the wire response and
// the downstream Mongo / Redis side-effects.
type profileHarness struct {
	r           *gin.Engine
	jwtMgr      *jwtx.Manager
	fa          *testutil.FakeApple
	users       *mongo.Collection
	resumeCache *redisx.RedisResumeCache
	srv         *httptest.Server
}

func (h *profileHarness) signIn(t *testing.T, sub, deviceID, platform string) dto.SignInWithAppleResponse {
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
	req, err := http.NewRequest(http.MethodPost, "/auth/apple", strings.NewReader(string(body)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	h.r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, "SIWA failed: %s", w.Body.String())

	var out dto.SignInWithAppleResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	return out
}

func (h *profileHarness) dialWS(t *testing.T, accessToken string) *gws.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(h.srv.URL, "http") + "/ws"
	hdr := http.Header{}
	hdr.Set("Authorization", "Bearer "+accessToken)
	conn, resp, err := gws.DefaultDialer.Dial(url, hdr)
	require.NoError(t, err, "ws dial failed; status=%v", resp)
	t.Cleanup(func() { _ = conn.Close() })
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	return conn
}

func (h *profileHarness) sendAndReadProfileUpdate(t *testing.T, conn *gws.Conn, envID string, payload map[string]any) ws.Response {
	t.Helper()
	env := map[string]any{
		"id":      envID,
		"type":    "profile.update",
		"payload": payload,
	}
	require.NoError(t, conn.WriteJSON(env))

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, msg, err := conn.ReadMessage()
	require.NoError(t, err)

	var out ws.Response
	require.NoError(t, json.Unmarshal(msg, &out))
	return out
}

func setupProfileHarness(t *testing.T) *profileHarness {
	t.Helper()

	cli, cleanup := testutil.SetupMongo(t)
	t.Cleanup(cleanup)

	dbName := "test_profile_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	mongoCli := mongox.WrapClient(cli, dbName)
	t.Cleanup(func() { _ = cli.Database(dbName).Drop(context.Background()) })

	fa := testutil.NewFakeApple(t, "com.test.cat")
	clk := fa.Clock

	serverPriv := mustGenRSA(t)
	signingKeyPath := writePrivateKeyPEM(t, serverPriv)

	jwtMgr := jwtx.NewManagerWithApple(jwtx.Options{
		PrivateKeyPath:   signingKeyPath,
		ActiveKID:        "kid-test-profile",
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

	resumeCache := redisx.NewResumeCache(fa.Redis, clk, 60*time.Second)

	// Profile service + handler — the production wiring.
	profileSvc := service.NewProfileService(userRepo, resumeCache, clk)
	profileHandler := ws.NewProfileHandler(&profileServiceHandlerAdapter{svc: profileSvc})

	hub := ws.NewHub(ws.HubConfig{
		PingInterval: 30 * time.Second,
		PongTimeout:  60 * time.Second,
		SendBufSize:  64,
	}, clockx.NewRealClock())
	dedupStore := redisx.NewDedupStore(fa.Redis, 60*time.Second)
	dispatcher := ws.NewDispatcher(dedupStore, clockx.NewRealClock())
	dispatcher.RegisterDedup("profile.update", profileHandler.HandleUpdate)
	upgradeHandler := ws.NewUpgradeHandler(hub, dispatcher, ws.NewJWTValidator(jwtMgr), nil, nil)
	t.Cleanup(func() { _ = hub.Final(context.Background()) })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.POST("/auth/apple", authHandler.SignInWithApple)
	r.POST("/auth/refresh", authHandler.Refresh)
	r.GET("/ws", upgradeHandler.Handle)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return &profileHarness{
		r:           r,
		jwtMgr:      jwtMgr,
		fa:          fa,
		users:       mongoCli.DB().Collection("users"),
		resumeCache: resumeCache,
		srv:         srv,
	}
}

// --- tests ---

func TestProfileUpdate_Integration_HappyPath_AllThreeFields(t *testing.T) {
	h := setupProfileHarness(t)
	dev := uuid.NewString()
	siwa := h.signIn(t, "apple:test:profile-happy", dev, "iphone")

	conn := h.dialWS(t, siwa.AccessToken)
	out := h.sendAndReadProfileUpdate(t, conn, "env-1", map[string]any{
		"displayName": "Alice",
		"timezone":    "Asia/Shanghai",
		"quietHours":  map[string]string{"start": "23:00", "end": "07:00"},
	})
	require.True(t, out.OK, "profile.update failed: %+v", out)

	// Wire shape
	var resp dto.ProfileUpdateResponse
	require.NoError(t, json.Unmarshal(out.Payload, &resp))
	require.NotNil(t, resp.User.DisplayName)
	assert.Equal(t, "Alice", *resp.User.DisplayName)
	require.NotNil(t, resp.User.Timezone)
	assert.Equal(t, "Asia/Shanghai", *resp.User.Timezone)
	assert.Equal(t, "23:00", resp.User.Preferences.QuietHours.Start)
	assert.Equal(t, "07:00", resp.User.Preferences.QuietHours.End)

	// Mongo side-effect
	var raw bson.M
	require.NoError(t, h.users.FindOne(context.Background(), bson.M{"_id": siwa.User.ID}).Decode(&raw))
	assert.Equal(t, "Alice", raw["display_name"])
	assert.Equal(t, "Asia/Shanghai", raw["timezone"])
	prefs, ok := raw["preferences"].(bson.M)
	require.True(t, ok)
	qh, ok := prefs["quiet_hours"].(bson.M)
	require.True(t, ok)
	assert.Equal(t, "23:00", qh["start"])
	assert.Equal(t, "07:00", qh["end"])
}

func TestProfileUpdate_Integration_Partial_DisplayNameOnly(t *testing.T) {
	h := setupProfileHarness(t)
	dev := uuid.NewString()
	siwa := h.signIn(t, "apple:test:profile-dn-only", dev, "watch")

	conn := h.dialWS(t, siwa.AccessToken)
	out := h.sendAndReadProfileUpdate(t, conn, "env-2", map[string]any{
		"displayName": "  Bob  ",
	})
	require.True(t, out.OK, "body=%s", string(out.Payload))

	// Mongo: display_name set AND trimmed; timezone nil; quiet_hours
	// preserved at seed default.
	var raw bson.M
	require.NoError(t, h.users.FindOne(context.Background(), bson.M{"_id": siwa.User.ID}).Decode(&raw))
	assert.Equal(t, "Bob", raw["display_name"], "trim must happen before persistence")
	assert.Nil(t, raw["timezone"])
	prefs := raw["preferences"].(bson.M)
	qh := prefs["quiet_hours"].(bson.M)
	assert.Equal(t, "23:00", qh["start"], "quietHours preserved at seed default")
	assert.Equal(t, "07:00", qh["end"])
}

func TestProfileUpdate_Integration_Partial_TimezoneOnly(t *testing.T) {
	h := setupProfileHarness(t)
	dev := uuid.NewString()
	siwa := h.signIn(t, "apple:test:profile-tz-only", dev, "watch")

	conn := h.dialWS(t, siwa.AccessToken)
	out := h.sendAndReadProfileUpdate(t, conn, "env-3", map[string]any{
		"timezone": "America/New_York",
	})
	require.True(t, out.OK, "body=%s", string(out.Payload))

	var raw bson.M
	require.NoError(t, h.users.FindOne(context.Background(), bson.M{"_id": siwa.User.ID}).Decode(&raw))
	assert.Nil(t, raw["display_name"], "displayName untouched")
	assert.Equal(t, "America/New_York", raw["timezone"])
}

func TestProfileUpdate_Integration_ResumeCacheInvalidated(t *testing.T) {
	h := setupProfileHarness(t)
	dev := uuid.NewString()
	siwa := h.signIn(t, "apple:test:profile-cache", dev, "iphone")

	// Seed the resume cache with a stale snapshot.
	ctx := context.Background()
	err := h.resumeCache.Put(ctx, siwa.User.ID, redisx.ResumeSnapshot{
		User:     json.RawMessage(`{"id":"stale","displayName":"Stale"}`),
		Friends:  json.RawMessage(`[]`),
		CatState: json.RawMessage(`null`),
	})
	require.NoError(t, err)

	// Verify seeded.
	_, found, err := h.resumeCache.Get(ctx, siwa.User.ID)
	require.NoError(t, err)
	require.True(t, found, "cache must contain the stale snapshot before profile.update")

	// Trigger update — must invalidate cache.
	conn := h.dialWS(t, siwa.AccessToken)
	out := h.sendAndReadProfileUpdate(t, conn, "env-4", map[string]any{
		"displayName": "Fresh",
	})
	require.True(t, out.OK, "body=%s", string(out.Payload))

	// Cache must now be empty.
	_, found, err = h.resumeCache.Get(ctx, siwa.User.ID)
	require.NoError(t, err)
	assert.False(t, found, "profile.update MUST invalidate resume cache (Story 1.5 AC4)")
}

func TestProfileUpdate_Integration_ReplayEventIDReturnsCachedResponse(t *testing.T) {
	h := setupProfileHarness(t)
	dev := uuid.NewString()
	siwa := h.signIn(t, "apple:test:profile-replay", dev, "iphone")

	conn := h.dialWS(t, siwa.AccessToken)
	envID := "env-replay-" + uuid.NewString()
	out1 := h.sendAndReadProfileUpdate(t, conn, envID, map[string]any{
		"displayName": "First",
	})
	require.True(t, out1.OK)

	// Second send with the SAME envelope.id — dedup middleware MUST
	// either return the cached result OR surface EVENT_PROCESSING
	// (both are §21.1 / 0.10 compatible outcomes per the dedup
	// contract). Either way it must NOT produce a second UpdateOne
	// with a different displayName stored.
	out2 := h.sendAndReadProfileUpdate(t, conn, envID, map[string]any{
		"displayName": "Second",
	})
	// The replay either gets cached OK (same body) or EVENT_PROCESSING.
	if out2.OK {
		assert.JSONEq(t, string(out1.Payload), string(out2.Payload),
			"dedup replay MUST return identical cached response — NOT re-process with new payload")
	} else {
		require.NotNil(t, out2.Error)
		assert.Equal(t, "EVENT_PROCESSING", out2.Error.Code,
			"dedup replay must either return cached OK or EVENT_PROCESSING")
	}

	// Mongo must still read "First" — the replay must not have
	// persisted "Second".
	var raw bson.M
	require.NoError(t, h.users.FindOne(context.Background(), bson.M{"_id": siwa.User.ID}).Decode(&raw))
	assert.Equal(t, "First", raw["display_name"],
		"dedup MUST prevent the second UpdateOne — 'Second' would indicate double-processing")
}

func TestProfileUpdate_Integration_QuietHoursOnly_RejectedWhenTimezoneUnset(t *testing.T) {
	h := setupProfileHarness(t)
	dev := uuid.NewString()
	// Fresh SIWA → Timezone field is nil on the user doc (SIWA seeds
	// Preferences.QuietHours at "23:00-07:00" but leaves Timezone
	// unset). The review-round-1 preflight MUST reject a
	// quietHours-only update in this state because
	// RealQuietHoursResolver would silently short-circuit to "not
	// quiet" at resolve time otherwise.
	siwa := h.signIn(t, "apple:test:profile-qh-no-tz", dev, "iphone")

	conn := h.dialWS(t, siwa.AccessToken)
	out := h.sendAndReadProfileUpdate(t, conn, "env-qh-only-no-tz", map[string]any{
		"quietHours": map[string]string{"start": "22:00", "end": "06:00"},
	})
	require.False(t, out.OK, "quietHours-only update must be rejected when user.Timezone is unset; body=%s", string(out.Payload))
	require.NotNil(t, out.Error)
	assert.Equal(t, "VALIDATION_ERROR", out.Error.Code)
	assert.Contains(t, out.Error.Message, "timezone",
		"error message must name the missing field so the client can act")

	// Mongo must be unchanged — the rejection runs BEFORE UpdateOne.
	var raw bson.M
	require.NoError(t, h.users.FindOne(context.Background(), bson.M{"_id": siwa.User.ID}).Decode(&raw))
	prefs := raw["preferences"].(bson.M)
	qh := prefs["quiet_hours"].(bson.M)
	assert.Equal(t, "23:00", qh["start"], "rejected update MUST NOT leak to Mongo")
	assert.Equal(t, "07:00", qh["end"])

	// Client recovery path: send timezone first, then quietHours works.
	// This locks the "set tz first, then qh" contract.
	out2 := h.sendAndReadProfileUpdate(t, conn, "env-set-tz-first", map[string]any{
		"timezone": "Asia/Shanghai",
	})
	require.True(t, out2.OK, "timezone-only setup must succeed; body=%s", string(out2.Payload))

	out3 := h.sendAndReadProfileUpdate(t, conn, "env-qh-after-tz", map[string]any{
		"quietHours": map[string]string{"start": "22:00", "end": "06:00"},
	})
	require.True(t, out3.OK, "quietHours-only update must succeed AFTER tz is set; body=%s", string(out3.Payload))
}

func TestProfileUpdate_Integration_InvalidPayload_ReturnsValidationError(t *testing.T) {
	h := setupProfileHarness(t)
	dev := uuid.NewString()
	siwa := h.signIn(t, "apple:test:profile-invalid-tz", dev, "iphone")

	conn := h.dialWS(t, siwa.AccessToken)
	out := h.sendAndReadProfileUpdate(t, conn, "env-bad", map[string]any{
		"timezone": "Pacific/Nope",
	})
	require.False(t, out.OK)
	require.NotNil(t, out.Error)
	assert.Equal(t, "VALIDATION_ERROR", out.Error.Code)
	assert.Contains(t, out.Error.Message, "IANA")

	// Mongo must be unchanged (timezone remains nil from SIWA seed).
	var raw bson.M
	require.NoError(t, h.users.FindOne(context.Background(), bson.M{"_id": siwa.User.ID}).Decode(&raw))
	assert.Nil(t, raw["timezone"], "rejected update MUST NOT leak to Mongo")
}

// unused-guard for imports that some subtests need (they all do but
// keep the import shape explicit).
var _ ids.UserID
