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

// Story 20.8 dev_cosmetic_handler 单元测试。
//
// **stub 设计**：独立 stubDevCosmeticService（与 stubDevStepService / stubDevChestService 平级）；
// 测试 router **不**挂 mock auth middleware（dev 路径无 auth）。仅挂 ErrorMappingMiddleware
// （c.Error 写 envelope 必需；与 7.5 / 20.7 newDevXxxHandlerRouter 同模式）。

// ============================================================
// stub DevCosmeticService（独立类型；与 stubDevStepService / stubDevChestService 平级）
// ============================================================

type stubDevCosmeticService struct {
	grantCosmeticBatchFn func(ctx context.Context, userID uint64, rarity int8, count int32) error
}

func (s *stubDevCosmeticService) GrantCosmeticBatch(ctx context.Context, userID uint64, rarity int8, count int32) error {
	return s.grantCosmeticBatchFn(ctx, userID, rarity, count)
}

// newDevCosmeticHandlerRouter 构造 handler test router。
//
// **关键差异 vs newChestHandlerRouter**：dev 端点不挂 mock auth middleware（dev 不要求 auth）。
// 仅挂 ErrorMappingMiddleware（c.Error 写 envelope 必需；与 7.5 / 20.7 newXxxHandlerRouter 同模式）。
func newDevCosmeticHandlerRouter(svc service.DevCosmeticService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorMappingMiddleware())
	h := handler.NewDevCosmeticHandler(svc)
	r.POST("/dev/grant-cosmetic-batch", h.PostGrantCosmeticBatch)
	return r
}

func decodeDevCosmeticEnvelope(t *testing.T, body []byte) struct {
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

func doPostJSON(r *gin.Engine, path string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ============================================================
// 必须覆盖 5 case + 加分 case
// ============================================================

// 1. HappyPath_ServiceReturnsNotImplemented_Forwards501: body 合法 {"userId":1001,"rarity":1,"count":10}
//    → 透传到 stub service → 节点 7 stub 返 *AppError(ErrNotImplemented=1010) → middleware 翻 HTTP 501 +
//    envelope.code=1010。
//
//    **设计**：节点 7 阶段端点是 stub，**没有真正的 200 happy path**（lesson:
//    docs/lessons/2026-05-15-stub-endpoint-not-implemented-error-code.md）。本 case 验"handler 把合法参数原样
//    透传到 service，并把 service 的 1010 错误转 envelope" —— 这是节点 7 阶段"真实可用"的最深路径。
//
//    **r2 改造**：从 ErrServiceBusy(1009 → HTTP 500 + ERROR log) 改为 ErrNotImplemented(1010 → HTTP 501
//    + WARN log)。HTTP 501 是标准"Not Implemented"语义；e2e 工具可按 501 正确识别。
//
//    节点 8 / Story 23.5 激活时本 case 必须改回"stub service 返 nil → 200 + envelope.code=0 + data 透传"
//    happy path 语义。
func TestDevCosmeticHandler_PostGrantCosmeticBatch_HappyPath_ServiceReturnsNotImplemented_Forwards501(t *testing.T) {
	called := false
	svc := &stubDevCosmeticService{
		grantCosmeticBatchFn: func(ctx context.Context, userID uint64, rarity int8, count int32) error {
			called = true
			if userID != 1001 {
				t.Errorf("svc userID = %d, want 1001 (透传校验)", userID)
			}
			if rarity != 1 {
				t.Errorf("svc rarity = %d, want 1 (透传校验)", rarity)
			}
			if count != 10 {
				t.Errorf("svc count = %d, want 10 (透传校验)", count)
			}
			return apperror.New(apperror.ErrNotImplemented, "dev/grant-cosmetic-batch not yet implemented (node-7 stub; awaits Story 23.5 to activate)")
		},
	}
	r := newDevCosmeticHandlerRouter(svc)

	w := doPostJSON(r, "/dev/grant-cosmetic-batch", `{"userId":1001,"rarity":1,"count":10}`)

	if !called {
		t.Errorf("service should be called when params are valid (handler 必须把合法参数透传到 service)")
	}
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501 (1010 走 501; ErrorMappingMiddleware §HTTP-status 决策)；body=%s", w.Code, w.Body.String())
	}
	env := decodeDevCosmeticEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrNotImplemented {
		t.Errorf("envelope.code = %d, want %d (1010; 节点 7 stub explicit failure)", env.Code, apperror.ErrNotImplemented)
	}
	if !strings.Contains(env.Message, "node-7 stub") && !strings.Contains(env.Message, "not yet implemented") {
		t.Errorf("envelope.message = %q, want contains 'node-7 stub' or 'not yet implemented' (让调用方明确知道 stub 状态)", env.Message)
	}
}

// 2. RarityInvalid_99: body {"userId":1001,"rarity":99,"count":10} → handler 显式校验 99 ∉ [1,4]
//    → 1002 + message="rarity 必须 ∈ [1,4]"；stub service 内 t.Errorf 兜底
func TestDevCosmeticHandler_PostGrantCosmeticBatch_RarityInvalid_99_Returns1002_NoServiceCall(t *testing.T) {
	svc := &stubDevCosmeticService{
		grantCosmeticBatchFn: func(ctx context.Context, userID uint64, rarity int8, count int32) error {
			t.Errorf("service should NOT be called when rarity=99 (handler must intercept)")
			return nil
		},
	}
	r := newDevCosmeticHandlerRouter(svc)

	w := doPostJSON(r, "/dev/grant-cosmetic-batch", `{"userId":1001,"rarity":99,"count":10}`)

	env := decodeDevCosmeticEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "rarity") {
		t.Errorf("envelope.message = %q, want contains 'rarity'", env.Message)
	}
}

// 3. RarityZero: body {"userId":1001,"rarity":0,"count":10} → handler 显式校验 rarity=0 ∉ [1,4] → 1002
//    （验"0 不被当作合法值放行" —— validator/v10 把 0 视为 zero value 误判 required 的陷阱由指针类型规避）
func TestDevCosmeticHandler_PostGrantCosmeticBatch_RarityZero_Returns1002_NoServiceCall(t *testing.T) {
	svc := &stubDevCosmeticService{
		grantCosmeticBatchFn: func(ctx context.Context, userID uint64, rarity int8, count int32) error {
			t.Errorf("service should NOT be called when rarity=0 (handler must intercept)")
			return nil
		},
	}
	r := newDevCosmeticHandlerRouter(svc)

	w := doPostJSON(r, "/dev/grant-cosmetic-batch", `{"userId":1001,"rarity":0,"count":10}`)

	env := decodeDevCosmeticEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "rarity") {
		t.Errorf("envelope.message = %q, want contains 'rarity'", env.Message)
	}
}

// 4. CountZero: body {"userId":1001,"rarity":1,"count":0} → handler 显式校验 count=0 ∉ [1,100]
//    → 1002 + message 含 "count"
func TestDevCosmeticHandler_PostGrantCosmeticBatch_CountZero_Returns1002_NoServiceCall(t *testing.T) {
	svc := &stubDevCosmeticService{
		grantCosmeticBatchFn: func(ctx context.Context, userID uint64, rarity int8, count int32) error {
			t.Errorf("service should NOT be called when count=0 (handler must intercept)")
			return nil
		},
	}
	r := newDevCosmeticHandlerRouter(svc)

	w := doPostJSON(r, "/dev/grant-cosmetic-batch", `{"userId":1001,"rarity":1,"count":0}`)

	env := decodeDevCosmeticEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "count") {
		t.Errorf("envelope.message = %q, want contains 'count'", env.Message)
	}
}

// 5. CountTooLarge_101: body {"userId":1001,"rarity":1,"count":101} → handler 显式校验 count > 100 → 1002
//    （验"上限保护"防 demo 误传 1e6 砸 DB）
func TestDevCosmeticHandler_PostGrantCosmeticBatch_CountTooLarge_101_Returns1002_NoServiceCall(t *testing.T) {
	svc := &stubDevCosmeticService{
		grantCosmeticBatchFn: func(ctx context.Context, userID uint64, rarity int8, count int32) error {
			t.Errorf("service should NOT be called when count=101 (handler must intercept upper bound)")
			return nil
		},
	}
	r := newDevCosmeticHandlerRouter(svc)

	w := doPostJSON(r, "/dev/grant-cosmetic-batch", `{"userId":1001,"rarity":1,"count":101}`)

	env := decodeDevCosmeticEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "count") {
		t.Errorf("envelope.message = %q, want contains 'count'", env.Message)
	}
}

// 6. MissingUserID: body {"rarity":1,"count":10}（无 userId）→ ShouldBindJSON 后 UserID 仍 nil →
//    handler 校验失败 → 1002 + message="userId 必填"
func TestDevCosmeticHandler_PostGrantCosmeticBatch_MissingUserID_Returns1002(t *testing.T) {
	svc := &stubDevCosmeticService{
		grantCosmeticBatchFn: func(ctx context.Context, userID uint64, rarity int8, count int32) error {
			t.Errorf("service should NOT be called when userId missing")
			return nil
		},
	}
	r := newDevCosmeticHandlerRouter(svc)

	w := doPostJSON(r, "/dev/grant-cosmetic-batch", `{"rarity":1,"count":10}`)

	env := decodeDevCosmeticEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "userId") {
		t.Errorf("envelope.message = %q, want contains 'userId'", env.Message)
	}
}

// 7. MissingRarity: body {"userId":1001,"count":10}（无 rarity）→ Rarity nil → 1002 + message="rarity 必填"
func TestDevCosmeticHandler_PostGrantCosmeticBatch_MissingRarity_Returns1002(t *testing.T) {
	svc := &stubDevCosmeticService{
		grantCosmeticBatchFn: func(ctx context.Context, userID uint64, rarity int8, count int32) error {
			t.Errorf("service should NOT be called when rarity missing")
			return nil
		},
	}
	r := newDevCosmeticHandlerRouter(svc)

	w := doPostJSON(r, "/dev/grant-cosmetic-batch", `{"userId":1001,"count":10}`)

	env := decodeDevCosmeticEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "rarity") {
		t.Errorf("envelope.message = %q, want contains 'rarity'", env.Message)
	}
}

// 8. MissingCount: body {"userId":1001,"rarity":1}（无 count）→ Count nil → 1002 + message="count 必填"
func TestDevCosmeticHandler_PostGrantCosmeticBatch_MissingCount_Returns1002(t *testing.T) {
	svc := &stubDevCosmeticService{
		grantCosmeticBatchFn: func(ctx context.Context, userID uint64, rarity int8, count int32) error {
			t.Errorf("service should NOT be called when count missing")
			return nil
		},
	}
	r := newDevCosmeticHandlerRouter(svc)

	w := doPostJSON(r, "/dev/grant-cosmetic-batch", `{"userId":1001,"rarity":1}`)

	env := decodeDevCosmeticEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "count") {
		t.Errorf("envelope.message = %q, want contains 'count'", env.Message)
	}
}

// 9. InvalidJSON: body {"userId":"abc","rarity":1,"count":10}（userId 类型错）→ ShouldBindJSON 失败 → 1002
func TestDevCosmeticHandler_PostGrantCosmeticBatch_InvalidJSON_Returns1002(t *testing.T) {
	svc := &stubDevCosmeticService{
		grantCosmeticBatchFn: func(ctx context.Context, userID uint64, rarity int8, count int32) error {
			t.Errorf("service should NOT be called when JSON type wrong")
			return nil
		},
	}
	r := newDevCosmeticHandlerRouter(svc)

	w := doPostJSON(r, "/dev/grant-cosmetic-batch", `{"userId":"abc","rarity":1,"count":10}`)

	env := decodeDevCosmeticEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
}

// 10. UserIDZero: body {"userId":0,"rarity":1,"count":10} → handler 显式校验 0 → 1002 + message 含 "userId"
//
// 验"0 不被当作合法值放行"（与 rarity=0 / count=0 同语义）。
func TestDevCosmeticHandler_PostGrantCosmeticBatch_UserIDZero_Returns1002(t *testing.T) {
	svc := &stubDevCosmeticService{
		grantCosmeticBatchFn: func(ctx context.Context, userID uint64, rarity int8, count int32) error {
			t.Errorf("service should NOT be called when userId=0 (handler must intercept)")
			return nil
		},
	}
	r := newDevCosmeticHandlerRouter(svc)

	w := doPostJSON(r, "/dev/grant-cosmetic-batch", `{"userId":0,"rarity":1,"count":10}`)

	env := decodeDevCosmeticEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "userId") {
		t.Errorf("envelope.message = %q, want contains 'userId'", env.Message)
	}
}

// 11. ServiceError_Forwards1009_HTTP500: stub service 返 *AppError(ErrServiceBusy) → middleware envelope
//    code=1009 + HTTP **500**（占位测试，验 handler 转发 service error 路径在节点 8 激活后仍能工作）。
//    1009 是唯一走非 200 的业务码（ADR-0006）。
func TestDevCosmeticHandler_PostGrantCosmeticBatch_ServiceError_Forwards1009_HTTP500(t *testing.T) {
	svc := &stubDevCosmeticService{
		grantCosmeticBatchFn: func(ctx context.Context, userID uint64, rarity int8, count int32) error {
			return apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		},
	}
	r := newDevCosmeticHandlerRouter(svc)

	w := doPostJSON(r, "/dev/grant-cosmetic-batch", `{"userId":1001,"rarity":1,"count":10}`)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (1009 走 500; ADR-0006)", w.Code)
	}
	env := decodeDevCosmeticEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d (1009)", env.Code, apperror.ErrServiceBusy)
	}
}
