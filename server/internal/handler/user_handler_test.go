package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/handler"
	"github.com/huing/cat/server/internal/middleware"
	"github.com/huing/cat/server/internal/service"
	"github.com/huing/cat/server/pkg/ids"
	"github.com/huing/cat/server/pkg/jwtx"
)

// --- fakes ---

type fakeUserSvc struct {
	call   int
	gotUID ids.UserID
	result *service.AccountDeletionResult
	err    error
}

func (f *fakeUserSvc) RequestAccountDeletion(_ context.Context, uid ids.UserID) (*service.AccountDeletionResult, error) {
	f.call++
	f.gotUID = uid
	return f.result, f.err
}

// newUserRouter wires JWTAuth(fake) in front of the user handler so
// tests drive the userId claim via the fake verifier — same pattern
// as device_handler_test.go. Pass v=nil to exercise the
// UserIDFrom-empty-401 defense-in-depth branch directly.
func newUserRouter(svc handler.UserHandlerService, v middleware.JWTVerifier) *gin.Engine {
	r := gin.New()
	h := handler.NewUserHandler(svc)
	g := r.Group("/v1")
	if v != nil {
		g.Use(middleware.JWTAuth(v))
	}
	g.DELETE("/users/me", h.RequestDeletion)
	return r
}

// Reusable: make a DELETE request against /v1/users/me, optionally
// with a body (to exercise DELETE-with-body behavior). Returns the
// response recorder.
func deleteUsersMe(t *testing.T, r *gin.Engine, body []byte, header string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	} else {
		reader = bytes.NewReader(nil)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/v1/users/me", reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	r.ServeHTTP(w, req)
	return w
}

type fakeVerifier struct {
	claims *jwtx.CustomClaims
	err    error
}

func (f *fakeVerifier) Verify(_ string) (*jwtx.CustomClaims, error) { return f.claims, f.err }

// --- tests ---

func TestUserHandler_RequestDeletion_HappyPath_202(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	svc := &fakeUserSvc{result: &service.AccountDeletionResult{RequestedAt: stamp, WasAlreadyRequested: false}}
	v := &fakeVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "watch", TokenType: "access",
	}}
	r := newUserRouter(svc, v)

	w := deleteUsersMe(t, r, nil, "Bearer tok")
	require.Equal(t, http.StatusAccepted, w.Code)

	var body dto.AccountDeletionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, dto.AccountDeletionStatusRequested, body.Status)
	assert.Equal(t, "2026-04-19T12:00:00Z", body.RequestedAt)
	assert.Equal(t, dto.AccountDeletionNoteMVP, body.Note)

	assert.Equal(t, 1, svc.call)
	assert.Equal(t, ids.UserID("u1"), svc.gotUID)
}

func TestUserHandler_RequestDeletion_UserNotFound_Surfaces404(t *testing.T) {
	t.Parallel()
	svc := &fakeUserSvc{err: dto.ErrUserNotFound.WithCause(errors.New("repo ghost"))}
	v := &fakeVerifier{claims: &jwtx.CustomClaims{
		UserID: "ghost", DeviceID: "d1", Platform: "watch", TokenType: "access",
	}}
	r := newUserRouter(svc, v)

	w := deleteUsersMe(t, r, nil, "Bearer tok")
	assert.Equal(t, http.StatusNotFound, w.Code)

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "USER_NOT_FOUND", body.Error.Code)
}

func TestUserHandler_RequestDeletion_InternalError_Surfaces500(t *testing.T) {
	t.Parallel()
	svc := &fakeUserSvc{err: dto.ErrInternalError.WithCause(errors.New("mongo disconnect"))}
	v := &fakeVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "watch", TokenType: "access",
	}}
	r := newUserRouter(svc, v)

	w := deleteUsersMe(t, r, nil, "Bearer tok")
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestUserHandler_RequestDeletion_MissingMiddleware_401(t *testing.T) {
	t.Parallel()
	// Defense-in-depth: with no JWT middleware mounted, the handler
	// MUST return 401 from its own UserIDFrom-empty guard — NOT reach
	// the service.
	svc := &fakeUserSvc{}
	r := newUserRouter(svc, nil)

	w := deleteUsersMe(t, r, nil, "")
	assert.Equal(t, http.StatusUnauthorized, w.Code)

	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "AUTH_INVALID_IDENTITY_TOKEN", body.Error.Code)
	assert.Zero(t, svc.call, "svc must NOT run when middleware did not inject userId")
}

func TestUserHandler_RequestDeletion_RequestedAtIsUTC_ZSuffix(t *testing.T) {
	t.Parallel()
	// §21.8 #4: format MUST be `.UTC().Format(RFC3339)` — the marker
	// is the `Z` suffix. If a future refactor drops the .UTC() call,
	// a server in Asia/Shanghai would emit `+08:00` offsets here.
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	require.NoError(t, err)
	nonUTC := time.Date(2026, 4, 19, 12, 0, 0, 0, shanghai)
	svc := &fakeUserSvc{result: &service.AccountDeletionResult{RequestedAt: nonUTC}}
	v := &fakeVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "watch", TokenType: "access",
	}}
	r := newUserRouter(svc, v)

	w := deleteUsersMe(t, r, nil, "Bearer tok")
	require.Equal(t, http.StatusAccepted, w.Code)

	var body dto.AccountDeletionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

	re := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)
	assert.Regexp(t, re, body.RequestedAt,
		"RequestedAt MUST serialize as UTC RFC3339 ending in Z; got %q — likely `.UTC()` got dropped",
		body.RequestedAt)

	// Double-check: Shanghai 12:00 = UTC 04:00.
	assert.Equal(t, "2026-04-19T04:00:00Z", body.RequestedAt,
		"Shanghai 2026-04-19T12:00 must render as UTC 2026-04-19T04:00Z")
}

func TestUserHandler_RequestDeletion_ContentTypeJSON(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	svc := &fakeUserSvc{result: &service.AccountDeletionResult{RequestedAt: stamp}}
	v := &fakeVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "watch", TokenType: "access",
	}}
	r := newUserRouter(svc, v)

	w := deleteUsersMe(t, r, nil, "Bearer tok")
	require.Equal(t, http.StatusAccepted, w.Code)
	assert.True(t,
		strings.Contains(w.Header().Get("Content-Type"), "application/json"),
		"Content-Type must be JSON; got %q", w.Header().Get("Content-Type"))
}

// TestUserHandler_RequestDeletion_DeleteWithBody_StillSucceeds locks
// §21.8 #7: handler MUST NOT call ShouldBindJSON, so a body sent on
// DELETE is neither parsed nor rejected — the service still runs with
// the userId injected by middleware, and the response shape is
// unaffected by the body.
func TestUserHandler_RequestDeletion_DeleteWithBody_StillSucceeds(t *testing.T) {
	t.Parallel()
	stamp := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	svc := &fakeUserSvc{result: &service.AccountDeletionResult{RequestedAt: stamp}}
	v := &fakeVerifier{claims: &jwtx.CustomClaims{
		UserID: "u1", DeviceID: "d1", Platform: "watch", TokenType: "access",
	}}
	r := newUserRouter(svc, v)

	// Body contains some noise — it must be ignored, not rejected.
	body := []byte(`{"foo":"bar","reason":"attempt-to-smuggle"}`)
	w := deleteUsersMe(t, r, body, "Bearer tok")
	require.Equal(t, http.StatusAccepted, w.Code,
		"DELETE with body must still 202 — handler does not parse body")

	var resp dto.AccountDeletionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, dto.AccountDeletionStatusRequested, resp.Status)

	assert.Equal(t, 1, svc.call, "service MUST run exactly once regardless of body")
	assert.Equal(t, ids.UserID("u1"), svc.gotUID, "userId comes from middleware, not body")
}

func TestNewUserHandler_NilServicePanics(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() { handler.NewUserHandler(nil) })
}
