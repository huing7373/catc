package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/ids"
	"github.com/huing/cat/server/pkg/jwtx"
)

// fakeVerifier returns canned (claims, err) — keeps the middleware
// test free of real RS256 keys, which are pkg/jwtx/manager_test.go's
// turf. Using a fake also lets us drive every reject branch
// deterministically: forging "TokenType=refresh" via real Issue is
// pointless when a fake is enough.
type fakeVerifier struct {
	claims *jwtx.CustomClaims
	err    error
}

func (f *fakeVerifier) Verify(_ string) (*jwtx.CustomClaims, error) {
	return f.claims, f.err
}

func init() {
	gin.SetMode(gin.TestMode)
}

// newGuardedRouter wires JWTAuth + an /guarded handler that records
// whether it ran (downstreamHits) so AbortsDownstream can assert
// non-execution.
func newGuardedRouter(t *testing.T, v JWTVerifier) (*gin.Engine, *atomic.Int32) {
	t.Helper()
	r := gin.New()
	hits := &atomic.Int32{}
	r.Use(JWTAuth(v))
	r.GET("/guarded", func(c *gin.Context) {
		hits.Add(1)
		c.JSON(http.StatusOK, gin.H{
			"userId":   string(UserIDFrom(c)),
			"deviceId": DeviceIDFrom(c),
			"platform": string(PlatformFrom(c)),
		})
	})
	return r, hits
}

func doGet(r *gin.Engine, header string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/guarded", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	r.ServeHTTP(w, req)
	return w
}

func decodeErrCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Error.Code
}

func TestJWTAuth_HappyPath(t *testing.T) {
	t.Parallel()
	v := &fakeVerifier{claims: &jwtx.CustomClaims{
		UserID:    "u1",
		DeviceID:  "d1",
		Platform:  "iphone",
		TokenType: "access",
	}}
	r, hits := newGuardedRouter(t, v)
	w := doGet(r, "Bearer t")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int32(1), hits.Load())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "u1", body["userId"])
	assert.Equal(t, "d1", body["deviceId"])
	assert.Equal(t, "iphone", body["platform"])
}

func TestJWTAuth_MissingHeader(t *testing.T) {
	t.Parallel()
	v := &fakeVerifier{claims: &jwtx.CustomClaims{UserID: "u1", DeviceID: "d1", TokenType: "access"}}
	r, hits := newGuardedRouter(t, v)
	w := doGet(r, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "AUTH_TOKEN_EXPIRED", decodeErrCode(t, w))
	assert.Equal(t, int32(0), hits.Load())
}

func TestJWTAuth_NonBearerHeader(t *testing.T) {
	t.Parallel()
	v := &fakeVerifier{claims: &jwtx.CustomClaims{UserID: "u1", DeviceID: "d1", TokenType: "access"}}
	r, hits := newGuardedRouter(t, v)
	w := doGet(r, "Basic dXNlcjpwYXNz")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "AUTH_TOKEN_EXPIRED", decodeErrCode(t, w))
	assert.Equal(t, int32(0), hits.Load())
}

func TestJWTAuth_BearerEmptyToken(t *testing.T) {
	t.Parallel()
	v := &fakeVerifier{claims: &jwtx.CustomClaims{UserID: "u1", DeviceID: "d1", TokenType: "access"}}
	r, hits := newGuardedRouter(t, v)
	w := doGet(r, "Bearer ")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "AUTH_TOKEN_EXPIRED", decodeErrCode(t, w))
	assert.Equal(t, int32(0), hits.Load())
}

func TestJWTAuth_VerifyError(t *testing.T) {
	t.Parallel()
	v := &fakeVerifier{err: errors.New("bad sig")}
	r, hits := newGuardedRouter(t, v)
	w := doGet(r, "Bearer rotten")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "AUTH_INVALID_IDENTITY_TOKEN", decodeErrCode(t, w))
	assert.Equal(t, int32(0), hits.Load())
}

func TestJWTAuth_RefreshTokenAsAccess(t *testing.T) {
	t.Parallel()
	v := &fakeVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "iphone", TokenType: "refresh",
	}}
	r, hits := newGuardedRouter(t, v)
	w := doGet(r, "Bearer rt")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "AUTH_INVALID_IDENTITY_TOKEN", decodeErrCode(t, w))
	assert.Equal(t, int32(0), hits.Load())
}

func TestJWTAuth_EmptyUID(t *testing.T) {
	t.Parallel()
	v := &fakeVerifier{claims: &jwtx.CustomClaims{
		UserID: "", DeviceID: "d1", TokenType: "access",
	}}
	r, hits := newGuardedRouter(t, v)
	w := doGet(r, "Bearer t")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "AUTH_INVALID_IDENTITY_TOKEN", decodeErrCode(t, w))
	assert.Equal(t, int32(0), hits.Load())
}

func TestJWTAuth_EmptyDeviceID(t *testing.T) {
	t.Parallel()
	v := &fakeVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "", TokenType: "access",
	}}
	r, hits := newGuardedRouter(t, v)
	w := doGet(r, "Bearer t")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "AUTH_INVALID_IDENTITY_TOKEN", decodeErrCode(t, w))
	assert.Equal(t, int32(0), hits.Load())
}

func TestJWTAuth_EmptyPlatformAllowed(t *testing.T) {
	t.Parallel()
	v := &fakeVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "", TokenType: "access",
	}}
	r, hits := newGuardedRouter(t, v)
	w := doGet(r, "Bearer t")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int32(1), hits.Load())
}

func TestJWTAuth_AbortsDownstream(t *testing.T) {
	t.Parallel()
	v := &fakeVerifier{err: errors.New("bad sig")}
	r, hits := newGuardedRouter(t, v)
	w := doGet(r, "Bearer x")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, int32(0), hits.Load(),
		"downstream handler MUST NOT execute when JWTAuth rejects")
}

// TestJWTAuth_InjectsLoggerUserID locks AC2 step 8: the happy-path
// branch must inherit userId into the std-context logger so a later
// access-log / handler-log line carries the field automatically.
// Cannot run with t.Parallel() because it captures the per-request
// logger via a closure — running parallel would race the buffer with
// the other parallel cases that also write to a buffer.
func TestJWTAuth_InjectsLoggerUserID(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf)

	v := &fakeVerifier{claims: &jwtx.CustomClaims{
		UserID: "u-logger", DeviceID: "d1", Platform: "iphone", TokenType: "access",
	}}
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := logger.WithContext(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.Use(JWTAuth(v))
	r.GET("/guarded", func(c *gin.Context) {
		// Log from inside the handler — middleware should have
		// already inherited userId into the ctx logger.
		zerolog.Ctx(c.Request.Context()).Info().Msg("inside-handler")
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/guarded", nil)
	req.Header.Set("Authorization", "Bearer t")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	// Find the inside-handler line and assert it carries userId.
	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	found := false
	for _, line := range lines {
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			continue
		}
		if m["message"] == "inside-handler" {
			assert.Equal(t, "u-logger", m["userId"],
				"handler-emitted log must inherit userId from JWTAuth")
			found = true
		}
	}
	require.True(t, found, "expected inside-handler log line, got: %s", buf.String())
}

func TestNewJWTAuth_PanicsOnNilVerifier(t *testing.T) {
	t.Parallel()
	// Both JWTAuth(nil) and JWTAuthWithBlacklist(nil, ...) share the
	// same guard because the former is a thin wrapper. Verify both
	// entry points panic so a future caller pointed at either one
	// still fails-fast.
	assert.Panics(t, func() { _ = JWTAuth(nil) })
	assert.Panics(t, func() { _ = JWTAuthWithBlacklist(nil, nil) })
	assert.Panics(t, func() { _ = JWTAuthWithBlacklist(nil, &fakeUserBlacklist{}) })
}

// fakeUserBlacklist is the test double for UserBlacklist used by the
// Story 1.6 round-1 blacklist-aware middleware subtests below. It
// records every IsBlacklisted call so assertions can verify the
// middleware queried with the userId claim (not some other field).
type fakeUserBlacklist struct {
	blocked bool
	err     error
	calls   []string
}

func (f *fakeUserBlacklist) IsBlacklisted(_ context.Context, userID string) (bool, error) {
	f.calls = append(f.calls, userID)
	return f.blocked, f.err
}

// newGuardedRouterWithBL wires JWTAuthWithBlacklist + the same
// downstream /guarded handler as newGuardedRouter so assertions
// compare shape byte-for-byte.
func newGuardedRouterWithBL(t *testing.T, v JWTVerifier, bl UserBlacklist) (*gin.Engine, *atomic.Int32) {
	t.Helper()
	r := gin.New()
	hits := &atomic.Int32{}
	r.Use(JWTAuthWithBlacklist(v, bl))
	r.GET("/guarded", func(c *gin.Context) {
		hits.Add(1)
		c.JSON(http.StatusOK, gin.H{"userId": string(UserIDFrom(c))})
	})
	return r, hits
}

// TestJWTAuthWithBlacklist_HappyPath — non-blocked user passes through
// and the downstream handler runs. Also verifies the blacklist was
// actually consulted (sentinel call captured).
func TestJWTAuthWithBlacklist_HappyPath(t *testing.T) {
	t.Parallel()
	v := &fakeVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "watch", TokenType: "access",
	}}
	bl := &fakeUserBlacklist{blocked: false}
	r, hits := newGuardedRouterWithBL(t, v, bl)

	w := doGet(r, "Bearer tok")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int32(1), hits.Load())
	require.Len(t, bl.calls, 1)
	assert.Equal(t, "u1", bl.calls[0], "blacklist MUST be queried with the userId claim")
}

// TestJWTAuthWithBlacklist_UserBlocked_401 — a blocked user (e.g.
// post-account-deletion) gets 401 AUTH_INVALID_IDENTITY_TOKEN and the
// downstream handler never runs.
func TestJWTAuthWithBlacklist_UserBlocked_401(t *testing.T) {
	t.Parallel()
	v := &fakeVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "watch", TokenType: "access",
	}}
	bl := &fakeUserBlacklist{blocked: true}
	r, hits := newGuardedRouterWithBL(t, v, bl)

	w := doGet(r, "Bearer tok")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "AUTH_INVALID_IDENTITY_TOKEN", decodeErrCode(t, w))
	assert.Zero(t, hits.Load(), "downstream handler MUST NOT run when user is blacklisted")
}

// TestJWTAuthWithBlacklist_RedisError_500_FailClosed — a blacklist
// read error is fail-closed (500), NOT "assume not blacklisted". This
// mirrors ws.UpgradeHandler's strategy.
func TestJWTAuthWithBlacklist_RedisError_500_FailClosed(t *testing.T) {
	t.Parallel()
	v := &fakeVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "watch", TokenType: "access",
	}}
	bl := &fakeUserBlacklist{err: errors.New("redis down")}
	r, hits := newGuardedRouterWithBL(t, v, bl)

	w := doGet(r, "Bearer tok")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "INTERNAL_ERROR", decodeErrCode(t, w))
	assert.Zero(t, hits.Load(),
		"fail-closed on blacklist read error: downstream handler MUST NOT run on Redis outage (do NOT assume not-blacklisted)")
}

// TestJWTAuthWithBlacklist_NilBlacklist_SkipsCheck — passing
// blacklist=nil (test harnesses / pre-round-1 boot paths) must skip
// the check without panic. JWTAuth(v) wraps JWTAuthWithBlacklist(v,
// nil), so this also proves the back-compat wrapper works.
func TestJWTAuthWithBlacklist_NilBlacklist_SkipsCheck(t *testing.T) {
	t.Parallel()
	v := &fakeVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "watch", TokenType: "access",
	}}
	r, hits := newGuardedRouterWithBL(t, v, nil)

	w := doGet(r, "Bearer tok")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int32(1), hits.Load(),
		"nil blacklist must behave identically to legacy JWTAuth(v)")
}

func TestUserIDFrom_WithoutMiddleware(t *testing.T) {
	t.Parallel()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/x", nil)
	assert.Equal(t, ids.UserID(""), UserIDFrom(c))
	assert.Equal(t, "", DeviceIDFrom(c))
	assert.Equal(t, ids.Platform(""), PlatformFrom(c))
}

// TestJWTAuth_VerifyErrorIsAppError sanity-checks that the rejected
// 401 carries AppError sentinel identity (errors.Is) so future code
// that wants to branch on dto.ErrAuthInvalidIdentityToken via
// errors.Is keeps working — guards against the WithCause refactor
// silently dropping the sentinel (Story 1.1 round 2 lesson).
func TestJWTAuth_VerifyErrorIsAppError(t *testing.T) {
	t.Parallel()
	cause := errors.New("boom")
	wrapped := dto.ErrAuthInvalidIdentityToken.WithCause(cause)
	require.True(t, errors.Is(wrapped, dto.ErrAuthInvalidIdentityToken),
		"WithCause must preserve the sentinel for downstream errors.Is")
}
