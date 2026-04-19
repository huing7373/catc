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

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/handler"
	"github.com/huing/cat/server/internal/middleware"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/internal/service"
	"github.com/huing/cat/server/internal/testutil"
	"github.com/huing/cat/server/pkg/ids"
	"github.com/huing/cat/server/pkg/jwtx"
	"github.com/huing/cat/server/pkg/mongox"
	"github.com/huing/cat/server/pkg/redisx"
)

// TestRefreshToken_EndToEnd exercises /auth/apple + /auth/refresh
// back-to-back through the real router + Mongo + miniredis stack.
// Four scenarios cover the Story 1.2 fail-closed matrix:
//
//  1. HappyPath: rotate refresh1 → receive refresh2, assert blacklist
//     entry for refresh1 + sessions.<device>.current_jti == new jti.
//  2. Revoked: pre-blacklist the jti manually, then /auth/refresh
//     returns 401 AUTH_REFRESH_TOKEN_REVOKED.
//  3. ReuseDetection: refresh refresh1 successfully (→ refresh2), then
//     reuse refresh1 again — the second call returns 401 AND the
//     current-live refresh2 also becomes blacklisted (OWASP / RFC 6819
//     §5.2.2.3 active defense).
//  4. PerDeviceIsolation: two devices sign in; refresh on device A
//     does NOT touch device B's sessions/blacklist state; triggering
//     reuse detection on A does NOT invalidate B.
func TestRefreshToken_EndToEnd(t *testing.T) {
	h := setupRefreshHarness(t)

	t.Run("HappyPath", func(t *testing.T) {
		dev := uuid.NewString()
		siwa := h.signIn(t, "apple:test:happy", dev, "watch")

		// POST /auth/refresh (refresh1)
		refreshResp := h.postRefresh(t, siwa.RefreshToken)
		require.Equal(t, http.StatusOK, refreshResp.code, "body=%s", refreshResp.body)
		var rbody dto.RefreshTokenResponse
		require.NoError(t, json.Unmarshal([]byte(refreshResp.body), &rbody))
		assert.NotEqual(t, siwa.AccessToken, rbody.AccessToken)
		assert.NotEqual(t, siwa.RefreshToken, rbody.RefreshToken)

		// sessions.<deviceId>.current_jti == jti(refresh2)
		claimsOld, err := h.jwtMgr.Verify(siwa.RefreshToken)
		require.NoError(t, err)
		claimsNew, err := h.jwtMgr.Verify(rbody.RefreshToken)
		require.NoError(t, err)

		user, err := h.userRepo.FindByID(context.Background(), userIDFromSIWA(t, siwa))
		require.NoError(t, err)
		require.Contains(t, user.Sessions, dev)
		assert.Equal(t, claimsNew.ID, user.Sessions[dev].CurrentJTI,
			"rolling-rotation: sessions.current_jti must match the new refresh jti")

		// Blacklist carries the old jti.
		exists := h.fa.Miniredis.Exists("refresh_blacklist:" + claimsOld.ID)
		assert.True(t, exists, "old jti must be blacklisted after rotation")

		requireAuditLine(t, h.audit, "refresh_token", map[string]any{
			"userId":   string(user.ID),
			"deviceId": dev,
			"oldJti":   claimsOld.ID,
			"newJti":   claimsNew.ID,
		})
	})

	t.Run("Revoked", func(t *testing.T) {
		dev := uuid.NewString()
		siwa := h.signIn(t, "apple:test:revoked", dev, "watch")

		// Pre-revoke the jti directly through the blacklist store.
		claims, err := h.jwtMgr.Verify(siwa.RefreshToken)
		require.NoError(t, err)
		require.NoError(t, h.refreshBlacklist.Revoke(
			context.Background(), claims.ID, claims.ExpiresAt.Time,
		))

		resp := h.postRefresh(t, siwa.RefreshToken)
		require.Equal(t, http.StatusUnauthorized, resp.code, "body=%s", resp.body)
		assert.Contains(t, resp.body, "AUTH_REFRESH_TOKEN_REVOKED")
	})

	t.Run("ReuseDetection", func(t *testing.T) {
		dev := uuid.NewString()
		siwa := h.signIn(t, "apple:test:reuse", dev, "watch")

		// First refresh: successful rotation.
		first := h.postRefresh(t, siwa.RefreshToken)
		require.Equal(t, http.StatusOK, first.code, "body=%s", first.body)
		var firstBody dto.RefreshTokenResponse
		require.NoError(t, json.Unmarshal([]byte(first.body), &firstBody))

		claimsNew, err := h.jwtMgr.Verify(firstBody.RefreshToken)
		require.NoError(t, err)

		// Second use of refresh1 → reuse detection.
		replay := h.postRefresh(t, siwa.RefreshToken)
		require.Equal(t, http.StatusUnauthorized, replay.code, "body=%s", replay.body)
		assert.Contains(t, replay.body, "AUTH_REFRESH_TOKEN_REVOKED")

		// Live refresh2 must now also be blacklisted (active defense).
		assert.True(t,
			h.fa.Miniredis.Exists("refresh_blacklist:"+claimsNew.ID),
			"reuse detection must burn the live jti (OAuth2 RFC 6819 §5.2.2.3)",
		)

		// Any subsequent refresh on refresh2 → 401 too.
		afterBurn := h.postRefresh(t, firstBody.RefreshToken)
		require.Equal(t, http.StatusUnauthorized, afterBurn.code)
		assert.Contains(t, afterBurn.body, "AUTH_REFRESH_TOKEN_REVOKED")
	})

	t.Run("PerDeviceIsolation", func(t *testing.T) {
		devW := uuid.NewString()
		devP := uuid.NewString()
		sub := "apple:test:per-device"

		siwaW := h.signIn(t, sub, devW, "watch")
		siwaP := h.signIn(t, sub, devP, "iphone")

		// Grab jtis up-front to compare.
		clW, err := h.jwtMgr.Verify(siwaW.RefreshToken)
		require.NoError(t, err)
		clP, err := h.jwtMgr.Verify(siwaP.RefreshToken)
		require.NoError(t, err)

		// Refresh device W.
		respW := h.postRefresh(t, siwaW.RefreshToken)
		require.Equal(t, http.StatusOK, respW.code, "body=%s", respW.body)

		// Device P's original refresh still works.
		respP := h.postRefresh(t, siwaP.RefreshToken)
		require.Equal(t, http.StatusOK, respP.code, "body=%s", respP.body)

		// sessions carries two distinct keys with distinct current jtis.
		user, err := h.userRepo.FindByID(context.Background(), userIDFromSIWA(t, siwaW))
		require.NoError(t, err)
		assert.Contains(t, user.Sessions, devW)
		assert.Contains(t, user.Sessions, devP)
		assert.NotEqual(t,
			user.Sessions[devW].CurrentJTI,
			user.Sessions[devP].CurrentJTI,
			"per-device isolation: watch and iphone jtis must diverge")
		assert.NotEqual(t, clW.ID, user.Sessions[devW].CurrentJTI,
			"device W jti rotated")
		assert.NotEqual(t, clP.ID, user.Sessions[devP].CurrentJTI,
			"device P jti rotated")

		// Trigger reuse detection on device W by replaying siwaW.RefreshToken.
		replay := h.postRefresh(t, siwaW.RefreshToken)
		require.Equal(t, http.StatusUnauthorized, replay.code, "body=%s", replay.body)

		// Device P's session must be untouched.
		userAfter, err := h.userRepo.FindByID(context.Background(), userIDFromSIWA(t, siwaP))
		require.NoError(t, err)
		assert.Equal(t, user.Sessions[devP].CurrentJTI, userAfter.Sessions[devP].CurrentJTI,
			"reuse detection on W must NOT mutate P's session.current_jti")

		// Device P's current jti must NOT be in the blacklist.
		assert.False(t,
			h.fa.Miniredis.Exists("refresh_blacklist:"+userAfter.Sessions[devP].CurrentJTI),
			"reuse detection on W must NOT blacklist P's jti")
	})
}

// ---- harness ----

type refreshHarness struct {
	r                *gin.Engine
	jwtMgr           *jwtx.Manager
	userRepo         *repository.MongoUserRepository
	refreshBlacklist *redisx.RefreshBlacklist
	fa               *testutil.FakeApple
	audit            *syncBuffer
}

func setupRefreshHarness(t *testing.T) *refreshHarness {
	t.Helper()

	cli, cleanup := testutil.SetupMongo(t)
	t.Cleanup(cleanup)

	dbName := "test_refresh_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	mongoCli := mongox.WrapClient(cli, dbName)
	t.Cleanup(func() { _ = cli.Database(dbName).Drop(context.Background()) })

	fa := testutil.NewFakeApple(t, "com.test.cat")
	clk := fa.Clock

	serverPriv := mustGenRSA(t)
	signingKeyPath := writePrivateKeyPEM(t, serverPriv)

	jwtMgr := jwtx.NewManagerWithApple(jwtx.Options{
		PrivateKeyPath:   signingKeyPath,
		ActiveKID:        "kid-test-refresh",
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

	audit := &syncBuffer{}
	prev := log.Logger
	log.Logger = zerolog.New(audit).With().Timestamp().Logger()
	t.Cleanup(func() { log.Logger = prev })

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.POST("/auth/apple", authHandler.SignInWithApple)
	r.POST("/auth/refresh", authHandler.Refresh)

	return &refreshHarness{
		r:                r,
		jwtMgr:           jwtMgr,
		userRepo:         userRepo,
		refreshBlacklist: refreshBlacklist,
		fa:               fa,
		audit:            audit,
	}
}

func (h *refreshHarness) signIn(t *testing.T, sub, deviceID, platform string) dto.SignInWithAppleResponse {
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

func (h *refreshHarness) postRefresh(t *testing.T, refreshToken string) httpResp {
	t.Helper()
	body, err := json.Marshal(dto.RefreshTokenRequest{RefreshToken: refreshToken})
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req, err := http.NewRequest(http.MethodPost, "/auth/refresh", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	h.r.ServeHTTP(w, req)
	return httpResp{code: w.Code, body: w.Body.String()}
}

func userIDFromSIWA(t *testing.T, r dto.SignInWithAppleResponse) ids.UserID {
	t.Helper()
	require.NotEmpty(t, r.User.ID)
	return ids.UserID(r.User.ID)
}

// Keep bson import reachable when future assertions need direct queries.
var _ = bson.M{}
