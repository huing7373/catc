package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/service"
	"github.com/huing/cat/server/pkg/ids"
)

type fakeSignInService struct {
	out *service.SignInWithAppleResult
	err error
	got service.SignInWithAppleRequest
}

func (f *fakeSignInService) SignInWithApple(_ context.Context, req service.SignInWithAppleRequest) (*service.SignInWithAppleResult, error) {
	f.got = req
	return f.out, f.err
}

func newRouter(svc AuthSignInService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAuthHandler(svc)
	r.POST("/auth/apple", h.SignInWithApple)
	return r
}

func happyBody(t *testing.T) []byte {
	t.Helper()
	body, err := json.Marshal(dto.SignInWithAppleRequest{
		IdentityToken: "id-token",
		DeviceID:      uuid.NewString(),
		Platform:      "watch",
		Nonce:         "01234567",
	})
	require.NoError(t, err)
	return body
}

func TestAuthHandler_SignInWithApple_Success(t *testing.T) {
	t.Parallel()
	displayName := "kuachan"
	tz := "Asia/Shanghai"
	uid := ids.NewUserID()

	svc := &fakeSignInService{
		out: &service.SignInWithAppleResult{
			AccessToken:  "ACC",
			RefreshToken: "REF",
			User: &domain.User{
				ID:          uid,
				DisplayName: &displayName,
				Timezone:    &tz,
			},
			IsNewUser: true,
		},
	}
	r := newRouter(svc)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/auth/apple", bytes.NewReader(happyBody(t)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body dto.SignInWithAppleResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, "ACC", body.AccessToken)
	assert.Equal(t, "REF", body.RefreshToken)
	assert.Equal(t, string(uid), body.User.ID)
	require.NotNil(t, body.User.DisplayName)
	assert.Equal(t, "kuachan", *body.User.DisplayName)
	assert.Equal(t, ids.PlatformWatch, svc.got.Platform, "handler must convert platform to ids.Platform")
}

func TestAuthHandler_SignInWithApple_BadJSON(t *testing.T) {
	t.Parallel()
	r := newRouter(&fakeSignInService{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/auth/apple", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "VALIDATION_ERROR")
}

func TestAuthHandler_SignInWithApple_ValidationError_MissingDeviceID(t *testing.T) {
	t.Parallel()
	body, _ := json.Marshal(dto.SignInWithAppleRequest{
		IdentityToken: "x",
		Platform:      "watch",
		Nonce:         "01234567",
	})
	r := newRouter(&fakeSignInService{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/auth/apple", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "VALIDATION_ERROR")
}

func TestAuthHandler_SignInWithApple_ValidationError_BadPlatform(t *testing.T) {
	t.Parallel()
	body, _ := json.Marshal(dto.SignInWithAppleRequest{
		IdentityToken: "x",
		DeviceID:      uuid.NewString(),
		Platform:      "android",
		Nonce:         "01234567",
	})
	r := newRouter(&fakeSignInService{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/auth/apple", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "VALIDATION_ERROR")
}

func TestAuthHandler_SignInWithApple_ServiceReturnsAuthInvalid(t *testing.T) {
	t.Parallel()
	svc := &fakeSignInService{err: dto.ErrAuthInvalidIdentityToken.WithCause(errors.New("apple alg mismatch"))}
	r := newRouter(svc)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/auth/apple", bytes.NewReader(happyBody(t)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "AUTH_INVALID_IDENTITY_TOKEN")
	assert.NotContains(t, w.Body.String(), "alg mismatch", "internal cause must not leak to client")
}

func TestAuthHandler_SignInWithApple_ServiceReturnsInternalError(t *testing.T) {
	t.Parallel()
	svc := &fakeSignInService{err: dto.ErrInternalError.WithCause(errors.New("mongo fail"))}
	r := newRouter(svc)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodPost, "/auth/apple", bytes.NewReader(happyBody(t)))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "INTERNAL_ERROR")
}

func TestNewAuthHandler_PanicsOnNilSvc(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() { NewAuthHandler(nil) })
}
