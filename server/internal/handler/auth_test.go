package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huing7373/catc/server/internal/dto"
	"github.com/huing7373/catc/server/internal/repository"
	"github.com/huing7373/catc/server/internal/service"
)

// mockAuthSvc is a hand-written stub of handler.AuthSvc.
type mockAuthSvc struct {
	loginFn   func(ctx context.Context, in service.LoginInput) (service.TokenPair, error)
	refreshFn func(ctx context.Context, in service.RefreshInput) (service.TokenPair, error)
}

func (m *mockAuthSvc) Login(ctx context.Context, in service.LoginInput) (service.TokenPair, error) {
	return m.loginFn(ctx, in)
}

func (m *mockAuthSvc) Refresh(ctx context.Context, in service.RefreshInput) (service.TokenPair, error) {
	return m.refreshFn(ctx, in)
}

func newRouter(svc AuthSvc) *gin.Engine {
	r := gin.New()
	h := NewAuthHandler(svc)
	r.POST("/v1/auth/login", h.Login)
	r.POST("/v1/auth/refresh", h.Refresh)
	return r
}

func doJSON(t *testing.T, r *gin.Engine, method, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var raw []byte
	if body != nil {
		var err error
		raw, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var decoded map[string]any
	if w.Body.Len() > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &decoded)
	}
	return w, decoded
}

func validLoginBody(extra map[string]any) map[string]any {
	body := map[string]any{
		"apple_jwt": "fake-apple-jwt",
		"nonce":     "16-char-min-nonce",
		"device_id": "dev-1",
	}
	for k, v := range extra {
		body[k] = v
	}
	return body
}

// --- 200 paths ---

func TestLogin_200_Created(t *testing.T) {
	now := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
	svc := &mockAuthSvc{loginFn: func(_ context.Context, in service.LoginInput) (service.TokenPair, error) {
		if in.AppleJWT != "fake-apple-jwt" || in.DeviceID != "dev-1" {
			t.Errorf("login input: %+v", in)
		}
		return service.TokenPair{
			AccessToken:      "AT-x",
			RefreshToken:     "RT-x",
			AccessExpiresAt:  now.Add(time.Hour),
			RefreshExpiresAt: now.Add(30 * 24 * time.Hour),
			UserID:           "u1",
			LoginOutcome:     repository.OutcomeCreated,
		}, nil
	}}
	w, body := doJSON(t, newRouter(svc), http.MethodPost, "/v1/auth/login", validLoginBody(nil))
	if w.Code != 200 {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	if body["user_id"] != "u1" || body["access_token"] != "AT-x" || body["refresh_token"] != "RT-x" {
		t.Errorf("body: %+v", body)
	}
	if body["login_outcome"] != "created" {
		t.Errorf("outcome: %v", body["login_outcome"])
	}
	// RFC3339 UTC timestamps must be strings, not numbers.
	for _, k := range []string{"access_expires_at", "refresh_expires_at"} {
		s, ok := body[k].(string)
		if !ok || s == "" {
			t.Errorf("%s missing/non-string: %v", k, body[k])
		}
	}
}

func TestLogin_200_Existing(t *testing.T) {
	svc := &mockAuthSvc{loginFn: func(context.Context, service.LoginInput) (service.TokenPair, error) {
		return service.TokenPair{UserID: "u2", AccessToken: "a", RefreshToken: "r", LoginOutcome: repository.OutcomeExisting}, nil
	}}
	w, body := doJSON(t, newRouter(svc), http.MethodPost, "/v1/auth/login", validLoginBody(nil))
	if w.Code != 200 {
		t.Fatalf("status: %d", w.Code)
	}
	if body["login_outcome"] != "existing" {
		t.Errorf("outcome: %v", body["login_outcome"])
	}
}

func TestRefresh_200(t *testing.T) {
	svc := &mockAuthSvc{refreshFn: func(_ context.Context, in service.RefreshInput) (service.TokenPair, error) {
		if in.RefreshToken != "rt" {
			t.Errorf("refresh input: %+v", in)
		}
		return service.TokenPair{UserID: "u3", AccessToken: "AT", RefreshToken: "RT"}, nil
	}}
	w, body := doJSON(t, newRouter(svc), http.MethodPost, "/v1/auth/refresh",
		map[string]any{"refresh_token": "rt"})
	if w.Code != 200 {
		t.Fatalf("status: %d body=%s", w.Code, w.Body.String())
	}
	if body["user_id"] != "u3" || body["access_token"] != "AT" {
		t.Errorf("body: %+v", body)
	}
	if _, hasOutcome := body["login_outcome"]; hasOutcome {
		t.Errorf("refresh response should NOT carry login_outcome: %+v", body)
	}
}

// --- 400 ---

func TestLogin_400_MissingAppleJWT(t *testing.T) {
	svc := &mockAuthSvc{loginFn: func(context.Context, service.LoginInput) (service.TokenPair, error) {
		t.Fatalf("svc must NOT be called when validation fails")
		return service.TokenPair{}, nil
	}}
	w, body := doJSON(t, newRouter(svc), http.MethodPost, "/v1/auth/login",
		map[string]any{"nonce": "16-char-min-nonce"})
	if w.Code != 400 {
		t.Fatalf("status: %d", w.Code)
	}
	assertErrorCode(t, body, "VALIDATION_ERROR")
}

func TestLogin_400_NonceTooShort(t *testing.T) {
	svc := &mockAuthSvc{loginFn: func(context.Context, service.LoginInput) (service.TokenPair, error) {
		t.Fatalf("svc must NOT be called")
		return service.TokenPair{}, nil
	}}
	w, body := doJSON(t, newRouter(svc), http.MethodPost, "/v1/auth/login",
		validLoginBody(map[string]any{"nonce": "short"}))
	if w.Code != 400 {
		t.Fatalf("status: %d", w.Code)
	}
	assertErrorCode(t, body, "VALIDATION_ERROR")
}

// --- 401 branches ---

func TestLogin_401_AppleAuthFail(t *testing.T) {
	svc := &mockAuthSvc{loginFn: func(context.Context, service.LoginInput) (service.TokenPair, error) {
		return service.TokenPair{}, service.ErrAppleAuthFail
	}}
	w, body := doJSON(t, newRouter(svc), http.MethodPost, "/v1/auth/login", validLoginBody(nil))
	if w.Code != 401 {
		t.Fatalf("status: %d", w.Code)
	}
	assertErrorCode(t, body, "APPLE_AUTH_FAIL")
}

func TestLogin_401_NonceMismatch(t *testing.T) {
	svc := &mockAuthSvc{loginFn: func(context.Context, service.LoginInput) (service.TokenPair, error) {
		return service.TokenPair{}, service.ErrNonceMismatch
	}}
	w, body := doJSON(t, newRouter(svc), http.MethodPost, "/v1/auth/login", validLoginBody(nil))
	if w.Code != 401 {
		t.Fatalf("status: %d", w.Code)
	}
	assertErrorCode(t, body, "NONCE_MISMATCH")
}

func TestRefresh_401_Expired(t *testing.T) {
	svc := &mockAuthSvc{refreshFn: func(context.Context, service.RefreshInput) (service.TokenPair, error) {
		return service.TokenPair{}, service.ErrTokenExpired
	}}
	w, body := doJSON(t, newRouter(svc), http.MethodPost, "/v1/auth/refresh",
		map[string]any{"refresh_token": "rt"})
	if w.Code != 401 {
		t.Fatalf("status: %d", w.Code)
	}
	assertErrorCode(t, body, "AUTH_EXPIRED")
}

func TestRefresh_401_DeadAccount(t *testing.T) {
	svc := &mockAuthSvc{refreshFn: func(context.Context, service.RefreshInput) (service.TokenPair, error) {
		return service.TokenPair{}, service.ErrUnauthorized
	}}
	w, body := doJSON(t, newRouter(svc), http.MethodPost, "/v1/auth/refresh",
		map[string]any{"refresh_token": "rt"})
	if w.Code != 401 {
		t.Fatalf("status: %d", w.Code)
	}
	assertErrorCode(t, body, "UNAUTHORIZED")
}

// --- helper / smoke ---

func assertErrorCode(t *testing.T, body map[string]any, want string) {
	t.Helper()
	errBody, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error envelope missing: %+v", body)
	}
	if errBody["code"] != want {
		t.Errorf("code: %v, want %q", errBody["code"], want)
	}
	if _, ok := errBody["message"].(string); !ok {
		t.Errorf("message missing or non-string: %+v", errBody)
	}
}

// Compile-time: AuthHandler satisfies the role expected by buildRouter.
var _ AuthSvc = (*mockAuthSvc)(nil)

// dto.LoginResp shape unchanged guard — protects the iPhone/Watch contract.
func TestLoginResp_JSONShape(t *testing.T) {
	now := time.Date(2026, 4, 15, 12, 0, 0, 0, time.UTC)
	resp := dto.LoginResp{
		UserID: "u", AccessToken: "a", RefreshToken: "r",
		AccessExpiresAt: now, RefreshExpiresAt: now,
		LoginOutcome: "created",
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var roundtrip map[string]any
	_ = json.Unmarshal(raw, &roundtrip)
	for _, k := range []string{"user_id", "access_token", "refresh_token", "access_expires_at", "refresh_expires_at", "login_outcome"} {
		if _, ok := roundtrip[k]; !ok {
			t.Errorf("missing key %q in LoginResp JSON", k)
		}
	}
}
