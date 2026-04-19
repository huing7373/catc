package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/handler"
	"github.com/huing/cat/server/internal/middleware"
	"github.com/huing/cat/server/internal/service"
	"github.com/huing/cat/server/pkg/jwtx"
)

// --- fakes ---

type fakeDeviceSvc struct {
	err  error
	got  service.RegisterApnsTokenRequest
	call int
}

func (f *fakeDeviceSvc) RegisterApnsToken(_ context.Context, req service.RegisterApnsTokenRequest) error {
	f.call++
	f.got = req
	return f.err
}

type fakeJWTVerifier struct {
	claims *jwtx.CustomClaims
	err    error
}

func (f *fakeJWTVerifier) Verify(_ string) (*jwtx.CustomClaims, error) {
	return f.claims, f.err
}

func init() {
	gin.SetMode(gin.TestMode)
}

// newDeviceRouter wires middleware.JWTAuth(fake) in front of the device
// handler so test code drives the userId / deviceId / platform claim via
// the fake verifier — same strategy as middleware/jwt_auth_test.go.
// This avoids exposing a test-only SetUserIDForTest on the middleware
// package surface.
func newDeviceRouter(svc handler.DeviceHandlerService, v middleware.JWTVerifier) *gin.Engine {
	r := gin.New()
	h := handler.NewDeviceHandler(svc)
	g := r.Group("/v1")
	if v != nil {
		g.Use(middleware.JWTAuth(v))
	}
	g.POST("/devices/apns-token", h.RegisterApnsToken)
	return r
}

func postJSON(t *testing.T, r *gin.Engine, body any, header string) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/devices/apns-token", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	r.ServeHTTP(w, req)
	return w
}

func decodeCode(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	return body.Error.Code
}

const validHex64 = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"

// --- tests ---

func TestDeviceHandler_RegisterApnsToken_HappyPath(t *testing.T) {
	t.Parallel()
	svc := &fakeDeviceSvc{}
	v := &fakeJWTVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "watch", TokenType: "access",
	}}
	r := newDeviceRouter(svc, v)

	w := postJSON(t, r, dto.RegisterApnsTokenRequest{DeviceToken: validHex64}, "Bearer tok")
	require.Equal(t, http.StatusOK, w.Code)

	var body dto.RegisterApnsTokenResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.True(t, body.Ok)

	assert.Equal(t, 1, svc.call)
	assert.Equal(t, "u1", string(svc.got.UserID))
	assert.Equal(t, "d1", svc.got.DeviceID)
	assert.Equal(t, "watch", string(svc.got.Platform))
	assert.Equal(t, validHex64, svc.got.DeviceToken)
}

func TestDeviceHandler_RegisterApnsToken_InvalidBody_400(t *testing.T) {
	t.Parallel()
	svc := &fakeDeviceSvc{}
	v := &fakeJWTVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "watch", TokenType: "access",
	}}
	r := newDeviceRouter(svc, v)
	w := postJSON(t, r, map[string]any{"deviceToken": ""}, "Bearer tok")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "VALIDATION_ERROR", decodeCode(t, w))
	assert.Zero(t, svc.call)
}

func TestDeviceHandler_RegisterApnsToken_InvalidHex_400(t *testing.T) {
	t.Parallel()
	svc := &fakeDeviceSvc{}
	v := &fakeJWTVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "watch", TokenType: "access",
	}}
	r := newDeviceRouter(svc, v)
	w := postJSON(t, r, map[string]any{"deviceToken": "ZZZZZZZZ", "platform": "watch"}, "Bearer tok")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "VALIDATION_ERROR", decodeCode(t, w))
	assert.Zero(t, svc.call)
}

func TestDeviceHandler_RegisterApnsToken_PlatformMismatch_400(t *testing.T) {
	t.Parallel()
	svc := &fakeDeviceSvc{}
	v := &fakeJWTVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "watch", TokenType: "access",
	}}
	r := newDeviceRouter(svc, v)
	w := postJSON(t, r,
		map[string]any{"deviceToken": validHex64, "platform": "iphone"},
		"Bearer tok")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, "VALIDATION_ERROR", decodeCode(t, w))
	assert.Zero(t, svc.call, "svc must not be called on platform mismatch")
}

func TestDeviceHandler_RegisterApnsToken_PlatformOmittedInBody_UsesJWT(t *testing.T) {
	t.Parallel()
	svc := &fakeDeviceSvc{}
	v := &fakeJWTVerifier{claims: &jwtx.CustomClaims{
		UserID: "u2", DeviceID: "d2", Platform: "iphone", TokenType: "access",
	}}
	r := newDeviceRouter(svc, v)
	w := postJSON(t, r, map[string]any{"deviceToken": validHex64}, "Bearer tok")
	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "iphone", string(svc.got.Platform))
}

// TestDeviceHandler_RegisterApnsToken_JWTMissingPlatform_401 locks the
// §21.8 #6 attack surface: when the JWT claim is empty we MUST reject,
// NOT fall back to body.platform — otherwise an attacker with a stolen
// Watch access token could register an iPhone APNs token against the
// victim.
func TestDeviceHandler_RegisterApnsToken_JWTMissingPlatform_401(t *testing.T) {
	t.Parallel()
	svc := &fakeDeviceSvc{}
	v := &fakeJWTVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "", TokenType: "access",
	}}
	r := newDeviceRouter(svc, v)
	w := postJSON(t, r,
		map[string]any{"deviceToken": validHex64, "platform": "watch"},
		"Bearer tok")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "AUTH_INVALID_IDENTITY_TOKEN", decodeCode(t, w))
	assert.Zero(t, svc.call, "svc must not be called when JWT platform is empty")
}

func TestDeviceHandler_RegisterApnsToken_JWTMissingPlatform_BodyAlsoMissing_401(t *testing.T) {
	t.Parallel()
	svc := &fakeDeviceSvc{}
	v := &fakeJWTVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "", TokenType: "access",
	}}
	r := newDeviceRouter(svc, v)
	w := postJSON(t, r, map[string]any{"deviceToken": validHex64}, "Bearer tok")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "AUTH_INVALID_IDENTITY_TOKEN", decodeCode(t, w))
	assert.Zero(t, svc.call)
}

// TestDeviceHandler_RegisterApnsToken_MissingUserID_401 exercises the
// defense-in-depth branch in the handler: if the middleware fails to
// inject userId (no Authorization header → JWTAuth rejects before the
// handler runs, but we keep the handler-side 401 as a belt-and-braces
// check). Here we hit the handler-level guard by leaving the middleware
// off so UserIDFrom returns "".
func TestDeviceHandler_RegisterApnsToken_MissingUserID_401(t *testing.T) {
	t.Parallel()
	svc := &fakeDeviceSvc{}
	r := newDeviceRouter(svc, nil) // no JWT middleware

	w := postJSON(t, r, map[string]any{"deviceToken": validHex64, "platform": "watch"}, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Equal(t, "AUTH_INVALID_IDENTITY_TOKEN", decodeCode(t, w))
	assert.Zero(t, svc.call)
}

func TestDeviceHandler_RegisterApnsToken_ServiceError_Bubbles(t *testing.T) {
	t.Parallel()
	svc := &fakeDeviceSvc{err: dto.ErrRateLimitExceeded}
	v := &fakeJWTVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "watch", TokenType: "access",
	}}
	r := newDeviceRouter(svc, v)
	w := postJSON(t, r, dto.RegisterApnsTokenRequest{DeviceToken: validHex64}, "Bearer tok")
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", decodeCode(t, w))
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
}

func TestDeviceHandler_RegisterApnsToken_ServiceUnwrappedError_500(t *testing.T) {
	t.Parallel()
	svc := &fakeDeviceSvc{err: errors.New("boom")}
	v := &fakeJWTVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "watch", TokenType: "access",
	}}
	r := newDeviceRouter(svc, v)
	w := postJSON(t, r, dto.RegisterApnsTokenRequest{DeviceToken: validHex64}, "Bearer tok")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "INTERNAL_ERROR", decodeCode(t, w))
}

func TestNewDeviceHandler_NilServicePanics(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() { handler.NewDeviceHandler(nil) })
}
