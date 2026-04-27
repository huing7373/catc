package handler_test

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/handler"
	"github.com/huing/cat/server/internal/app/http/middleware"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// stubAuthService 是 service.AuthService 的测试 stub；
// 通过 guestLoginFn 字段让每个 case 自定义返回。与 4.5 中间件单测 stub 同模式。
type stubAuthService struct {
	guestLoginFn func(ctx context.Context, in service.GuestLoginInput) (*service.GuestLoginOutput, error)
}

func (s *stubAuthService) GuestLogin(ctx context.Context, in service.GuestLoginInput) (*service.GuestLoginOutput, error) {
	return s.guestLoginFn(ctx, in)
}

// newAuthHandlerRouter 构造一个挂上 ErrorMappingMiddleware + AuthHandler 的 router。
// 关键：必须挂 ErrorMappingMiddleware，否则 c.Error(...) 后 body 为空，断不到 envelope.code。
func newAuthHandlerRouter(svc service.AuthService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorMappingMiddleware())
	authHandler := handler.NewAuthHandler(svc)
	r.POST("/api/v1/auth/guest-login", authHandler.GuestLogin)
	return r
}

func decodeEnvelope(t *testing.T, body []byte) response.Envelope {
	t.Helper()
	var env response.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("invalid JSON envelope: %v; body=%s", err, string(body))
	}
	return env
}

// TestGuestLoginHandler_HappyPath_ReturnsCorrectSchema (AC5.1):
// 合法 request → 200 + envelope.code=0 + 严格 V1 §4.1 schema
func TestGuestLoginHandler_HappyPath_ReturnsCorrectSchema(t *testing.T) {
	svc := &stubAuthService{
		guestLoginFn: func(ctx context.Context, in service.GuestLoginInput) (*service.GuestLoginOutput, error) {
			// 断言 service 收到正确字段
			if in.GuestUID != "abc" {
				t.Errorf("svc.GuestUID = %q, want abc", in.GuestUID)
			}
			if in.Platform != "ios" {
				t.Errorf("svc.Platform = %q, want ios", in.Platform)
			}
			return &service.GuestLoginOutput{
				Token:          "test-token",
				UserID:         1001,
				Nickname:       "用户1001",
				AvatarURL:      "",
				HasBoundWechat: false,
				PetID:          2001,
				PetType:        1,
				PetName:        "默认小猫",
			}, nil
		},
	}
	r := newAuthHandlerRouter(svc)

	body := `{"guestUid":"abc","device":{"platform":"ios","appVersion":"1.0.0","deviceModel":"iPhone15,2"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/guest-login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body.Bytes())
	if env.Code != 0 {
		t.Errorf("envelope.code = %d, want 0", env.Code)
	}
	if env.Message != "ok" {
		t.Errorf("envelope.message = %q, want ok", env.Message)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("envelope.data not object: %T", env.Data)
	}
	if data["token"] != "test-token" {
		t.Errorf("data.token = %v, want test-token", data["token"])
	}
	user, ok := data["user"].(map[string]any)
	if !ok {
		t.Fatalf("data.user not object: %T", data["user"])
	}
	// V1 §2.5 钦定 BIGINT id 是 string
	if user["id"] != "1001" {
		t.Errorf("user.id = %v, want \"1001\" (string)", user["id"])
	}
	if user["nickname"] != "用户1001" {
		t.Errorf("user.nickname = %v, want 用户1001", user["nickname"])
	}
	if user["avatarUrl"] != "" {
		t.Errorf("user.avatarUrl = %v, want empty string", user["avatarUrl"])
	}
	if user["hasBoundWechat"] != false {
		t.Errorf("user.hasBoundWechat = %v, want false (boolean)", user["hasBoundWechat"])
	}
	pet, ok := data["pet"].(map[string]any)
	if !ok {
		t.Fatalf("data.pet not object: %T", data["pet"])
	}
	if pet["id"] != "2001" {
		t.Errorf("pet.id = %v, want \"2001\" (string)", pet["id"])
	}
	// JSON number → float64
	if pt, _ := pet["petType"].(float64); pt != 1 {
		t.Errorf("pet.petType = %v, want 1 (number)", pet["petType"])
	}
	if pet["name"] != "默认小猫" {
		t.Errorf("pet.name = %v, want 默认小猫", pet["name"])
	}
}

// TestGuestLoginHandler_MissingGuestUID_Returns1002 (AC5.2):
// 缺 guestUid → ShouldBindJSON 失败 → 1002
func TestGuestLoginHandler_MissingGuestUID_Returns1002(t *testing.T) {
	svc := &stubAuthService{
		guestLoginFn: func(ctx context.Context, in service.GuestLoginInput) (*service.GuestLoginOutput, error) {
			t.Errorf("service should NOT be called when binding fails")
			return nil, nil
		},
	}
	r := newAuthHandlerRouter(svc)

	// 缺 guestUid 字段
	body := `{"device":{"platform":"ios","appVersion":"1.0.0","deviceModel":"iPhone15,2"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/guest-login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 业务码 1002 → HTTP 200（V1 §2.4 钦定业务码与 HTTP status 正交）
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (ErrInvalidParam)", env.Code, apperror.ErrInvalidParam)
	}
}

// TestGuestLoginHandler_GuestUIDTooLong_Returns1002 (AC5.3):
// guestUid 长度 129 字符 → 1002（utf8.RuneCountInString 边界）
func TestGuestLoginHandler_GuestUIDTooLong_Returns1002(t *testing.T) {
	svc := &stubAuthService{
		guestLoginFn: func(ctx context.Context, in service.GuestLoginInput) (*service.GuestLoginOutput, error) {
			t.Errorf("service should NOT be called when guestUid > 128 chars")
			return nil, nil
		},
	}
	r := newAuthHandlerRouter(svc)

	tooLong := strings.Repeat("a", 129) // 129 ASCII char = 129 runes
	body := `{"guestUid":"` + tooLong + `","device":{"platform":"ios","appVersion":"1.0.0","deviceModel":"iPhone15,2"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/guest-login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (ErrInvalidParam)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "guestUid") {
		t.Errorf("envelope.message = %q, want contains 'guestUid'", env.Message)
	}
}

// TestGuestLoginHandler_InvalidPlatform_Returns1002 (AC5.4):
// platform = "web"（不在枚举）→ 1002
func TestGuestLoginHandler_InvalidPlatform_Returns1002(t *testing.T) {
	svc := &stubAuthService{
		guestLoginFn: func(ctx context.Context, in service.GuestLoginInput) (*service.GuestLoginOutput, error) {
			t.Errorf("service should NOT be called when platform invalid")
			return nil, nil
		},
	}
	r := newAuthHandlerRouter(svc)

	body := `{"guestUid":"abc","device":{"platform":"web","appVersion":"1.0.0","deviceModel":"iPhone15,2"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/guest-login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (ErrInvalidParam)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "platform") {
		t.Errorf("envelope.message = %q, want contains 'platform'", env.Message)
	}
}

// TestGuestLoginHandler_ServiceError_Returns1009 (AC5.5):
// service 返 ErrServiceBusy *AppError → handler 透传 → ErrorMappingMiddleware 写 1009
func TestGuestLoginHandler_ServiceError_Returns1009(t *testing.T) {
	wantCause := stderrors.New("simulated DB outage")
	svc := &stubAuthService{
		guestLoginFn: func(ctx context.Context, in service.GuestLoginInput) (*service.GuestLoginOutput, error) {
			return nil, apperror.Wrap(wantCause, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		},
	}
	r := newAuthHandlerRouter(svc)

	body := `{"guestUid":"abc","device":{"platform":"ios","appVersion":"1.0.0","deviceModel":"iPhone15,2"}}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/guest-login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 1009 走 HTTP 500（ErrorMappingMiddleware 钦定）
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for 1009; body=%s", w.Code, w.Body.String())
	}
	env := decodeEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d (ErrServiceBusy)", env.Code, apperror.ErrServiceBusy)
	}
}
