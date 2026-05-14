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
	forceUnlockChestFn func(ctx context.Context, userID uint64, chestID uint64) error
}

func (s *stubDevChestService) ForceUnlockChest(ctx context.Context, userID uint64, chestID uint64) error {
	return s.forceUnlockChestFn(ctx, userID, chestID)
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
// 9 case（前缀 TestDevChestHandler_PostForceUnlockChest_<场景>）
// ============================================================

// 1. HappyPath: body {"userId":1001,"chestId":"5001"} → stub service 返 nil →
//    200 + envelope.code=0 + data.userId=1001 + data.chestId="5001"；stub service 内部验透传。
//    r2 [P2] 改造：chestId 字段必传 + string 类型（BIGINT 字符串化；与 GET /chest/current 对齐）。
func TestDevChestHandler_PostForceUnlockChest_HappyPath_ReturnsAck(t *testing.T) {
	called := false
	svc := &stubDevChestService{
		forceUnlockChestFn: func(ctx context.Context, userID uint64, chestID uint64) error {
			called = true
			if userID != 1001 {
				t.Errorf("svc userID = %d, want 1001 (透传校验)", userID)
			}
			if chestID != 5001 {
				t.Errorf("svc chestID = %d, want 5001 (chestId 字符串 → uint64 解析后透传)", chestID)
			}
			return nil
		},
	}
	r := newDevChestHandlerRouter(svc)

	body := `{"userId":1001,"chestId":"5001"}`
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
	// chestId 必须是 string 类型（BIGINT 字符串化；V1 §2.5 + §7.1 钦定）
	cid, ok := env.Data["chestId"].(string)
	if !ok {
		t.Errorf("data.chestId 类型 = %T, want string (BIGINT 字符串化)", env.Data["chestId"])
	}
	if cid != "5001" {
		t.Errorf("data.chestId = %q, want \"5001\"", cid)
	}
}

// 2. ChestNotFound: stub service 返 *AppError(ErrResourceNotFound) → handler c.Error →
//    middleware envelope code=1003，HTTP 200（业务码与 HTTP status 正交，仅 1009 走 500）。
func TestDevChestHandler_PostForceUnlockChest_ChestNotFound_Forwards1003_HTTP200(t *testing.T) {
	svc := &stubDevChestService{
		forceUnlockChestFn: func(ctx context.Context, userID uint64, chestID uint64) error {
			return apperror.New(apperror.ErrResourceNotFound, apperror.DefaultMessages[apperror.ErrResourceNotFound])
		},
	}
	r := newDevChestHandlerRouter(svc)

	body := `{"userId":99999,"chestId":"99999"}`
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
		forceUnlockChestFn: func(ctx context.Context, userID uint64, chestID uint64) error {
			return apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		},
	}
	r := newDevChestHandlerRouter(svc)

	body := `{"userId":1001,"chestId":"5001"}`
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

// 4. UserIDZero: body {"userId":0,"chestId":"5001"} → handler 显式校验 0 → 1002 + message 含 "userId" + "0"
func TestDevChestHandler_PostForceUnlockChest_UserIDZero_Returns1002_NoServiceCall(t *testing.T) {
	svc := &stubDevChestService{
		forceUnlockChestFn: func(ctx context.Context, userID uint64, chestID uint64) error {
			t.Errorf("service should NOT be called when userId=0 (handler must intercept; defense-in-depth)")
			return nil
		},
	}
	r := newDevChestHandlerRouter(svc)

	body := `{"userId":0,"chestId":"5001"}`
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

// 5. MissingUserID: body {"chestId":"5001"} → ShouldBindJSON 后 UserID 仍 nil → handler 校验失败 →
//    1002 + message="userId 必填"
func TestDevChestHandler_PostForceUnlockChest_MissingUserID_Returns1002(t *testing.T) {
	svc := &stubDevChestService{
		forceUnlockChestFn: func(ctx context.Context, userID uint64, chestID uint64) error {
			t.Errorf("service should NOT be called when userId missing")
			return nil
		},
	}
	r := newDevChestHandlerRouter(svc)

	body := `{"chestId":"5001"}`
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

// 6. InvalidJSON: body {"userId":"abc"}（userId 类型错）→ ShouldBindJSON 失败 → 1002
func TestDevChestHandler_PostForceUnlockChest_InvalidJSON_Returns1002(t *testing.T) {
	svc := &stubDevChestService{
		forceUnlockChestFn: func(ctx context.Context, userID uint64, chestID uint64) error {
			t.Errorf("service should NOT be called when JSON type wrong")
			return nil
		},
	}
	r := newDevChestHandlerRouter(svc)

	body := `{"userId":"abc","chestId":"5001"}`
	req := httptest.NewRequest(http.MethodPost, "/dev/force-unlock-chest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodeDevChestEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
}

// 7. MissingChestID: body {"userId":1001} → handler 校验 chestId nil → 1002 + message 含 "chestId"
//    r2 [P2] 新增 case：chestId 字段必传。
func TestDevChestHandler_PostForceUnlockChest_MissingChestID_Returns1002(t *testing.T) {
	svc := &stubDevChestService{
		forceUnlockChestFn: func(ctx context.Context, userID uint64, chestID uint64) error {
			t.Errorf("service should NOT be called when chestId missing")
			return nil
		},
	}
	r := newDevChestHandlerRouter(svc)

	body := `{"userId":1001}`
	req := httptest.NewRequest(http.MethodPost, "/dev/force-unlock-chest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodeDevChestEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "chestId") {
		t.Errorf("envelope.message = %q, want contains 'chestId'", env.Message)
	}
}

// 8. ChestIDNonNumeric: body {"userId":1001,"chestId":"abc"} → ParseUint 失败 → 1002 + message 含 "chestId"
//    r2 [P2] 新增 case：chestId 字符串非数字。
func TestDevChestHandler_PostForceUnlockChest_ChestIDNonNumeric_Returns1002(t *testing.T) {
	svc := &stubDevChestService{
		forceUnlockChestFn: func(ctx context.Context, userID uint64, chestID uint64) error {
			t.Errorf("service should NOT be called when chestId non-numeric")
			return nil
		},
	}
	r := newDevChestHandlerRouter(svc)

	body := `{"userId":1001,"chestId":"abc"}`
	req := httptest.NewRequest(http.MethodPost, "/dev/force-unlock-chest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodeDevChestEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "chestId") {
		t.Errorf("envelope.message = %q, want contains 'chestId'", env.Message)
	}
}

// 9. ChestIDZero: body {"userId":1001,"chestId":"0"} → handler 显式校验 chestID==0 → 1002 + message 含 "chestId"
//    r2 [P2] 新增 case："0" 能 ParseUint 通过，业务上无效需 handler 显式拒。
func TestDevChestHandler_PostForceUnlockChest_ChestIDZero_Returns1002(t *testing.T) {
	svc := &stubDevChestService{
		forceUnlockChestFn: func(ctx context.Context, userID uint64, chestID uint64) error {
			t.Errorf("service should NOT be called when chestId=0")
			return nil
		},
	}
	r := newDevChestHandlerRouter(svc)

	body := `{"userId":1001,"chestId":"0"}`
	req := httptest.NewRequest(http.MethodPost, "/dev/force-unlock-chest", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodeDevChestEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "chestId") {
		t.Errorf("envelope.message = %q, want contains 'chestId'", env.Message)
	}
}
