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
// stub DevChestService（独立类型；不复用 stubChestService —— 20.5 / 20.6 占用）
// ============================================================

type stubDevChestService struct {
	forceUnlockChestFn func(ctx context.Context, userID uint64) error
}

func (s *stubDevChestService) ForceUnlockChest(ctx context.Context, userID uint64) error {
	return s.forceUnlockChestFn(ctx, userID)
}

// newDevChestHandlerRouter 构造 handler test router。
//
// **关键差异 vs newChestHandlerRouter**：dev 端点不挂 mock auth middleware（dev 不要求 auth）。
// 仅挂 ErrorMappingMiddleware（c.Error 写 envelope 必需；与 7.5 newDevStepsHandlerRouter 同模式）。
func newDevChestHandlerRouter(svc service.DevChestService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorMappingMiddleware())
	h := handler.NewDevChestHandler(svc)
	r.POST("/dev/force-unlock-chest", h.PostForceUnlockChest)
	return r
}

func decodeDevChestEnvelope(t *testing.T, body []byte) struct {
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
// 6 case（前缀 TestDevChestHandler_PostForceUnlockChest_<场景>）
// ============================================================

// 1. HappyPath: body {"userId":1001} → stub service 返 nil →
//    200 + envelope.code=0 + data.userId=1001；stub service 内部验 userID 透传。
func TestDevChestHandler_PostForceUnlockChest_HappyPath_ReturnsAck(t *testing.T) {
	called := false
	svc := &stubDevChestService{
		forceUnlockChestFn: func(ctx context.Context, userID uint64) error {
			called = true
			if userID != 1001 {
				t.Errorf("svc userID = %d, want 1001 (透传校验)", userID)
			}
			return nil
		},
	}
	r := newDevChestHandlerRouter(svc)

	body := `{"userId":1001}`
	req := httptest.NewRequest(http.MethodPost, "/dev/force-unlock-chest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !called {
		t.Errorf("service should be called on happy path")
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeDevChestEnvelope(t, w.Body.Bytes())
	if env.Code != 0 {
		t.Errorf("envelope.code = %d, want 0", env.Code)
	}
	if env.Message != "ok" {
		t.Errorf("envelope.message = %q, want ok", env.Message)
	}
	if uid, _ := env.Data["userId"].(float64); uid != 1001 {
		t.Errorf("data.userId = %v, want 1001", env.Data["userId"])
	}
}

// 2. ChestNotFound: stub service 返 *AppError(ErrResourceNotFound) → handler c.Error →
//    middleware envelope code=1003，HTTP 200（业务码与 HTTP status 正交，仅 1009 走 500）。
func TestDevChestHandler_PostForceUnlockChest_ChestNotFound_Forwards1003_HTTP200(t *testing.T) {
	svc := &stubDevChestService{
		forceUnlockChestFn: func(ctx context.Context, userID uint64) error {
			return apperror.New(apperror.ErrResourceNotFound, apperror.DefaultMessages[apperror.ErrResourceNotFound])
		},
	}
	r := newDevChestHandlerRouter(svc)

	body := `{"userId":99999}`
	req := httptest.NewRequest(http.MethodPost, "/dev/force-unlock-chest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (业务码 1003 走 200; ADR-0006/V1 §2.4)", w.Code)
	}
	env := decodeDevChestEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrResourceNotFound {
		t.Errorf("envelope.code = %d, want %d (1003)", env.Code, apperror.ErrResourceNotFound)
	}
}

// 3. DBBusy: stub service 返 ErrServiceBusy → middleware envelope code=1009，**HTTP 500**。
func TestDevChestHandler_PostForceUnlockChest_DBBusy_Forwards1009_HTTP500(t *testing.T) {
	svc := &stubDevChestService{
		forceUnlockChestFn: func(ctx context.Context, userID uint64) error {
			return apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		},
	}
	r := newDevChestHandlerRouter(svc)

	body := `{"userId":1001}`
	req := httptest.NewRequest(http.MethodPost, "/dev/force-unlock-chest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (1009 走 500; ADR-0006)", w.Code)
	}
	env := decodeDevChestEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d (1009)", env.Code, apperror.ErrServiceBusy)
	}
}

// 4. UserIDZero: body {"userId":0} → handler 显式校验 0 → 1002 + message="userId 必须 > 0"；
//    stub service.forceUnlockChestFn 内 t.Errorf("should NOT be called") 验 handler 拦截在 service 之前。
func TestDevChestHandler_PostForceUnlockChest_UserIDZero_Returns1002_NoServiceCall(t *testing.T) {
	svc := &stubDevChestService{
		forceUnlockChestFn: func(ctx context.Context, userID uint64) error {
			t.Errorf("service should NOT be called when userId=0 (handler must intercept; defense-in-depth)")
			return nil
		},
	}
	r := newDevChestHandlerRouter(svc)

	body := `{"userId":0}`
	req := httptest.NewRequest(http.MethodPost, "/dev/force-unlock-chest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodeDevChestEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "userId") || !strings.Contains(env.Message, "0") {
		t.Errorf("envelope.message = %q, want contains 'userId' and '0' (early-fail with precise message)", env.Message)
	}
}

// 5. MissingUserID: body {} → ShouldBindJSON 后 UserID 仍 nil → handler 校验失败 →
//    1002 + message="userId 必填"；stub.forceUnlockChestFn 内 t.Errorf 兜底。
func TestDevChestHandler_PostForceUnlockChest_MissingUserID_Returns1002(t *testing.T) {
	svc := &stubDevChestService{
		forceUnlockChestFn: func(ctx context.Context, userID uint64) error {
			t.Errorf("service should NOT be called when userId missing")
			return nil
		},
	}
	r := newDevChestHandlerRouter(svc)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/dev/force-unlock-chest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodeDevChestEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "userId") {
		t.Errorf("envelope.message = %q, want contains 'userId'", env.Message)
	}
}

// 6. InvalidJSON: body {"userId":"abc"}（类型错）→ ShouldBindJSON 失败 → 1002；
//    stub.forceUnlockChestFn 内 t.Errorf 兜底。
func TestDevChestHandler_PostForceUnlockChest_InvalidJSON_Returns1002(t *testing.T) {
	svc := &stubDevChestService{
		forceUnlockChestFn: func(ctx context.Context, userID uint64) error {
			t.Errorf("service should NOT be called when JSON type wrong")
			return nil
		},
	}
	r := newDevChestHandlerRouter(svc)

	body := `{"userId":"abc"}`
	req := httptest.NewRequest(http.MethodPost, "/dev/force-unlock-chest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodeDevChestEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
}
