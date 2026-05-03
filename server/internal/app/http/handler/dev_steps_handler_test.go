package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/handler"
	"github.com/huing/cat/server/internal/app/http/middleware"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/service"
)

// ============================================================
// stub DevStepService（独立类型；不复用 stubStepService —— 7.3 / 7.4 占用）
// ============================================================

type stubDevStepService struct {
	grantStepsFn func(ctx context.Context, userID uint64, steps int32) error
}

func (s *stubDevStepService) GrantSteps(ctx context.Context, userID uint64, steps int32) error {
	return s.grantStepsFn(ctx, userID, steps)
}

// newDevStepsHandlerRouter 构造 handler test router。
//
// **关键差异 vs newStepsHandlerRouter**：dev 端点不挂 mock auth middleware（dev 不要求 auth）。
// 仅挂 ErrorMappingMiddleware（c.Error 写 envelope 必需）。
func newDevStepsHandlerRouter(svc service.DevStepService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorMappingMiddleware())
	h := handler.NewDevStepsHandler(svc)
	r.POST("/dev/grant-steps", h.PostGrantSteps)
	return r
}

func decodeDevEnvelope(t *testing.T, body []byte) struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
} {
	t.Helper()
	var env struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("invalid JSON envelope: %v; body=%s", err, string(body))
	}
	return env
}

// ============================================================
// 6 个 case
// ============================================================

// 1. HappyPath: body {"userId":1001,"steps":5000} → stub service 返 nil →
//    200 + envelope.code=0 + data.userId=1001 + data.grantedSteps=5000；
//    stub service 内部验 userID/steps 透传。
func TestDevStepsHandler_PostGrantSteps_HappyPath_ReturnsAck(t *testing.T) {
	called := false
	svc := &stubDevStepService{
		grantStepsFn: func(ctx context.Context, userID uint64, steps int32) error {
			called = true
			if userID != 1001 {
				t.Errorf("svc userID = %d, want 1001 (透传校验)", userID)
			}
			if steps != 5000 {
				t.Errorf("svc steps = %d, want 5000 (透传校验)", steps)
			}
			return nil
		},
	}
	r := newDevStepsHandlerRouter(svc)

	body := `{"userId":1001,"steps":5000}`
	req := httptest.NewRequest(http.MethodPost, "/dev/grant-steps", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !called {
		t.Errorf("service should be called on happy path")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeDevEnvelope(t, w.Body.Bytes())
	if env.Code != 0 {
		t.Errorf("envelope.code = %d, want 0", env.Code)
	}
	if env.Message != "ok" {
		t.Errorf("envelope.message = %q, want ok", env.Message)
	}
	if uid, _ := env.Data["userId"].(float64); uid != 1001 {
		t.Errorf("data.userId = %v, want 1001", env.Data["userId"])
	}
	if gs, _ := env.Data["grantedSteps"].(float64); gs != 5000 {
		t.Errorf("data.grantedSteps = %v, want 5000", env.Data["grantedSteps"])
	}
}

// 2. UserNotFound: stub service 返 *AppError(ErrResourceNotFound) → handler c.Error →
//    middleware envelope code=1003，HTTP 200（业务码与 HTTP status 正交，仅 1009 走 500）。
func TestDevStepsHandler_PostGrantSteps_UserNotFound_Forwards1003_HTTP200(t *testing.T) {
	svc := &stubDevStepService{
		grantStepsFn: func(ctx context.Context, userID uint64, steps int32) error {
			return apperror.New(apperror.ErrResourceNotFound, apperror.DefaultMessages[apperror.ErrResourceNotFound])
		},
	}
	r := newDevStepsHandlerRouter(svc)

	body := `{"userId":99999,"steps":5000}`
	req := httptest.NewRequest(http.MethodPost, "/dev/grant-steps", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (业务码 1003 走 200; ADR-0006/V1 §2.4)", w.Code)
	}
	env := decodeDevEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrResourceNotFound {
		t.Errorf("envelope.code = %d, want %d (1003)", env.Code, apperror.ErrResourceNotFound)
	}
}

// 3. NegativeSteps: body {"userId":1001,"steps":-1} → handler 校验失败返 1002 +
//    message="steps 不能为负数"；stub service.grantStepsFn 主动 t.Errorf 验 handler 拦截在 service 之前。
func TestDevStepsHandler_PostGrantSteps_NegativeSteps_Returns1002_NoServiceCall(t *testing.T) {
	svc := &stubDevStepService{
		grantStepsFn: func(ctx context.Context, userID uint64, steps int32) error {
			t.Errorf("service should NOT be called when steps < 0 (handler must intercept; defense-in-depth)")
			return nil
		},
	}
	r := newDevStepsHandlerRouter(svc)

	body := `{"userId":1001,"steps":-1}`
	req := httptest.NewRequest(http.MethodPost, "/dev/grant-steps", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodeDevEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "steps") || !strings.Contains(env.Message, "负数") {
		t.Errorf("envelope.message = %q, want contains 'steps' and '负数'", env.Message)
	}
}

// 4. DBBusy: stub service 返 ErrServiceBusy → middleware envelope code=1009，HTTP 500。
func TestDevStepsHandler_PostGrantSteps_DBBusy_Forwards1009_HTTP500(t *testing.T) {
	svc := &stubDevStepService{
		grantStepsFn: func(ctx context.Context, userID uint64, steps int32) error {
			return apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		},
	}
	r := newDevStepsHandlerRouter(svc)

	body := `{"userId":1001,"steps":5000}`
	req := httptest.NewRequest(http.MethodPost, "/dev/grant-steps", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (1009 走 500; ADR-0006)", w.Code)
	}
	env := decodeDevEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d (1009)", env.Code, apperror.ErrServiceBusy)
	}
}

// 5. MissingUserID: body {"steps":5000}（无 userId）→ handler 显式 nil 校验 → 1002 +
//    message 含 "userId"。
func TestDevStepsHandler_PostGrantSteps_MissingUserID_Returns1002(t *testing.T) {
	svc := &stubDevStepService{
		grantStepsFn: func(ctx context.Context, userID uint64, steps int32) error {
			t.Errorf("service should NOT be called when userId missing")
			return nil
		},
	}
	r := newDevStepsHandlerRouter(svc)

	body := `{"steps":5000}`
	req := httptest.NewRequest(http.MethodPost, "/dev/grant-steps", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodeDevEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "userId") {
		t.Errorf("envelope.message = %q, want contains 'userId'", env.Message)
	}
}

// 6. UserIDZero: body {"userId":0,"steps":5000} → handler 显式 *uid==0 拒 → 1002 +
//    message="userId 必须 > 0"。
func TestDevStepsHandler_PostGrantSteps_UserIDZero_Returns1002(t *testing.T) {
	svc := &stubDevStepService{
		grantStepsFn: func(ctx context.Context, userID uint64, steps int32) error {
			t.Errorf("service should NOT be called when userId=0")
			return nil
		},
	}
	r := newDevStepsHandlerRouter(svc)

	body := `{"userId":0,"steps":5000}`
	req := httptest.NewRequest(http.MethodPost, "/dev/grant-steps", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodeDevEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "userId") || !strings.Contains(env.Message, "0") {
		t.Errorf("envelope.message = %q, want contains 'userId' and '0' (early-fail with precise message)", env.Message)
	}
}

// 7. MissingSteps: body {"userId":1001} → handler 显式 nil 校验 → 1002 + message 含 "steps"。
func TestDevStepsHandler_PostGrantSteps_MissingSteps_Returns1002(t *testing.T) {
	svc := &stubDevStepService{
		grantStepsFn: func(ctx context.Context, userID uint64, steps int32) error {
			t.Errorf("service should NOT be called when steps missing")
			return nil
		},
	}
	r := newDevStepsHandlerRouter(svc)

	body := `{"userId":1001}`
	req := httptest.NewRequest(http.MethodPost, "/dev/grant-steps", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodeDevEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "steps") {
		t.Errorf("envelope.message = %q, want contains 'steps'", env.Message)
	}
}
