//go:build integration

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
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
	"github.com/huing/cat/server/pkg/jwtx"
	"github.com/huing/cat/server/pkg/mongox"
)

// TestSignInWithApple_EndToEnd wires every Story 1.1 layer end-to-end —
// AppleJWKFetcher (against an in-process httptest JWKS server),
// jwtx.Manager, MongoUserRepository (against a Testcontainers Mongo),
// AuthService, AuthHandler, the Gin router — then drives three
// scenarios that the SIWA flow MUST handle:
//
//  1. New user: response carries access+refresh tokens and a
//     UserPublic with the freshly-allocated UserID; the `users`
//     collection has exactly one document keyed by SHA-256(claims.Sub);
//     audit log entry has isNewUser=true.
//  2. Repeat sign-in: same `sub` returns the same UserID,
//     isNewUser=false, and no second `users` document is created.
//  3. Resurrection: flipping `deletion_requested` true on the row and
//     repeating sign-in MUST clear the flag and emit
//     `user_resurrected_from_deletion` plus a sign_in_with_apple
//     line with isNewUser=false.
//
// Self-contained per architecture §21.7: nothing reaches Apple's real
// JWKS endpoint, no iOS / watchOS app involved, only Go.
func TestSignInWithApple_EndToEnd(t *testing.T) {
	cli, cleanup := testutil.SetupMongo(t)
	defer cleanup()

	dbName := "test_sign_in_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	mongoCli := mongox.WrapClient(cli, dbName)
	t.Cleanup(func() { _ = cli.Database(dbName).Drop(context.Background()) })

	// FakeApple stands up an httptest JWKS server, RSA key, miniredis
	// and an AppleJWKFetcher pointed at the test server.
	fa := testutil.NewFakeApple(t, "com.test.cat")
	clk := fa.Clock // single shared clock so token exp / issuance lines up

	// Self-signed JWT signing key for our server (Story 0.3).
	serverPriv := mustGenRSA(t)
	signingKeyPath := writePrivateKeyPEM(t, serverPriv)

	jwtMgr := jwtx.NewManagerWithApple(jwtx.Options{
		PrivateKeyPath:   signingKeyPath,
		ActiveKID:        "kid-test",
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

	authSvc := service.NewAuthService(userRepo, jwtMgr, jwtMgr, clk, "release")
	authHandler := handler.NewAuthHandler(authSvc)

	// Capture audit log lines via a thread-safe buffer-backed zerolog
	// — the audit assertions need structured fields, and replacing
	// the global logger is the simplest reliable hook (the audit
	// service uses logx.Ctx which falls back to the global log.Logger
	// when no per-context logger is set).
	auditBuf := &syncBuffer{}
	prev := log.Logger
	log.Logger = zerolog.New(auditBuf).With().Timestamp().Logger()
	t.Cleanup(func() { log.Logger = prev })

	// Same middleware stack as initialize.go (RequestID feeds the
	// audit log requestId field via logx.Ctx).
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.RequestID())
	r.POST("/auth/apple", authHandler.SignInWithApple)

	// ---- Scenario 1: new user ----
	rawNonce := "nonce-" + uuid.NewString()
	deviceID := uuid.NewString()
	tok := fa.SignIdentityToken(t, testutil.SignOptions{
		Sub:   "apple:test:001",
		Nonce: hexSHA256(rawNonce),
	})

	resp1 := postSignIn(t, r, dto.SignInWithAppleRequest{
		IdentityToken: tok,
		DeviceID:      deviceID,
		Platform:      "watch",
		Nonce:         rawNonce,
	})
	require.Equal(t, http.StatusOK, resp1.code, "body=%s", resp1.body)

	var body1 dto.SignInWithAppleResponse
	require.NoError(t, json.Unmarshal([]byte(resp1.body), &body1))
	assert.NotEmpty(t, body1.AccessToken)
	assert.NotEmpty(t, body1.RefreshToken)
	assert.NotEmpty(t, body1.User.ID)

	// Verify access token via Manager.Verify roundtrip.
	claims, err := jwtMgr.Verify(body1.AccessToken)
	require.NoError(t, err)
	assert.Equal(t, body1.User.ID, claims.UserID)
	assert.Equal(t, deviceID, claims.DeviceID)
	assert.Equal(t, "watch", claims.Platform)
	assert.Equal(t, "access", claims.TokenType)
	assert.Equal(t, body1.User.ID, claims.Subject,
		"RegisteredClaims.Subject must equal UserID for downstream audit consumers")

	// Mongo: exactly one user, hash matches sha256("apple:test:001").
	expectedHash := hexSHA256("apple:test:001")
	count, err := mongoCli.DB().Collection("users").CountDocuments(context.Background(), bson.M{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)
	one, err := userRepo.FindByAppleHash(context.Background(), expectedHash)
	require.NoError(t, err)
	assert.Equal(t, body1.User.ID, string(one.ID))

	requireAuditLine(t, auditBuf, "sign_in_with_apple", map[string]any{
		"userId":    body1.User.ID,
		"deviceId":  deviceID,
		"platform":  "watch",
		"isNewUser": true,
	})

	// ---- Scenario 2: repeat sign-in ----
	tok2 := fa.SignIdentityToken(t, testutil.SignOptions{
		Sub:   "apple:test:001",
		Nonce: hexSHA256(rawNonce),
	})
	resp2 := postSignIn(t, r, dto.SignInWithAppleRequest{
		IdentityToken: tok2,
		DeviceID:      deviceID,
		Platform:      "watch",
		Nonce:         rawNonce,
	})
	require.Equal(t, http.StatusOK, resp2.code, "body=%s", resp2.body)

	var body2 dto.SignInWithAppleResponse
	require.NoError(t, json.Unmarshal([]byte(resp2.body), &body2))
	assert.Equal(t, body1.User.ID, body2.User.ID, "same Apple sub must return same UserID")

	count2, err := mongoCli.DB().Collection("users").CountDocuments(context.Background(), bson.M{})
	require.NoError(t, err)
	assert.Equal(t, int64(1), count2, "repeat sign-in must NOT create a second row")

	requireAuditLine(t, auditBuf, "sign_in_with_apple", map[string]any{
		"userId":    body1.User.ID,
		"isNewUser": false,
	})

	// ---- Scenario 3: resurrection ----
	now := clk.Now()
	_, err = mongoCli.DB().Collection("users").UpdateOne(context.Background(),
		bson.M{"_id": body1.User.ID},
		bson.M{"$set": bson.M{
			"deletion_requested":    true,
			"deletion_requested_at": now.Add(-24 * time.Hour),
		}},
	)
	require.NoError(t, err)

	tok3 := fa.SignIdentityToken(t, testutil.SignOptions{
		Sub:   "apple:test:001",
		Nonce: hexSHA256(rawNonce),
	})
	resp3 := postSignIn(t, r, dto.SignInWithAppleRequest{
		IdentityToken: tok3,
		DeviceID:      deviceID,
		Platform:      "watch",
		Nonce:         rawNonce,
	})
	require.Equal(t, http.StatusOK, resp3.code, "body=%s", resp3.body)

	again, err := userRepo.FindByAppleHash(context.Background(), expectedHash)
	require.NoError(t, err)
	assert.False(t, again.DeletionRequested, "resurrection must clear deletion_requested")
	assert.Nil(t, again.DeletionRequestedAt, "resurrection must clear deletion_requested_at")

	requireAuditLine(t, auditBuf, "user_resurrected_from_deletion", map[string]any{
		"userId": body1.User.ID,
	})

	// Compile-time guard against accidental redis import dropping.
	var _ redis.Cmdable = fa.Redis
}

// ---- helpers ----

type httpResp struct {
	code int
	body string
}

func postSignIn(t *testing.T, r http.Handler, req dto.SignInWithAppleRequest) httpResp {
	t.Helper()
	body, err := json.Marshal(req)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	httpReq, err := http.NewRequest(http.MethodPost, "/auth/apple", bytes.NewReader(body))
	require.NoError(t, err)
	httpReq.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, httpReq)
	return httpResp{code: w.Code, body: w.Body.String()}
}

func hexSHA256(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func mustGenRSA(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return k
}

func writePrivateKeyPEM(t *testing.T, k *rsa.PrivateKey) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "signing.pem")
	der, err := x509.MarshalPKCS8PrivateKey(k)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0o600))
	return path
}

// syncBuffer is a thread-safe bytes.Buffer for zerolog's writer.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// requireAuditLine searches the captured zerolog buffer for a JSON log
// line whose `action` matches and whose listed fields all equal want.
// Bool / string equality only — that covers everything our audit lines
// emit.
func requireAuditLine(t *testing.T, buf *syncBuffer, action string, want map[string]any) {
	t.Helper()
	for _, line := range strings.Split(buf.String(), "\n") {
		if line == "" {
			continue
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(line), &doc); err != nil {
			continue
		}
		if doc["action"] != action {
			continue
		}
		ok := true
		for k, v := range want {
			got := doc[k]
			if !valueEqual(got, v) {
				ok = false
				break
			}
		}
		if ok {
			return
		}
	}
	t.Fatalf("no audit line found with action=%q matching %v\nfull capture:\n%s", action, want, buf.String())
}

func valueEqual(got, want any) bool {
	switch w := want.(type) {
	case bool:
		g, ok := got.(bool)
		return ok && g == w
	case string:
		g, ok := got.(string)
		return ok && g == w
	}
	return got == want
}

// _ keeps the os import in scope for writePrivateKeyPEM.
var _ = os.WriteFile
