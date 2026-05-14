//go:build !integration

// Story 20.5 chest_handler 单元测试：5 case 覆盖 GetCurrent HTTP 边界 + envelope schema 断言。
//
// 与 7.4 steps_handler.GetAccount 测试同模式（stubChestService + newChestHandlerRouter +
// decodeChestEnvelope helper）。
//
// **关键差异 vs home_handler chest 块**（V1 §7.1 vs §5.1）：
//   - §5.1 GET /home：data.chest.{id, status, ...} —— **嵌套** chest 子对象
//   - §7.1 GET /chest/current：data.{id, status, ...} —— **扁平** 5 字段
//
// 本测试核心断言点：HappyPath case 必须验证 data 无 "chest" 子对象 + data.id 是 string。

package handler_test

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/handler"
	"github.com/huing/cat/server/internal/app/http/middleware"
	"github.com/huing/cat/server/internal/infra/config"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// stubChestService 是 service.ChestService 的 stub 实装，仅服务于 chest_handler_test.go。
// Story 20.6 加 openChestFn 字段以满足 interface 新加方法 OpenChest。
type stubChestService struct {
	getCurrentFn func(ctx context.Context, userID uint64) (*service.ChestBrief, error)
	openChestFn  func(ctx context.Context, in service.OpenChestInput) (*service.OpenChestOutput, error)
}

func (s *stubChestService) GetCurrent(ctx context.Context, userID uint64) (*service.ChestBrief, error) {
	return s.getCurrentFn(ctx, userID)
}

// OpenChest Story 20.6 加：interface 新方法；20.5 测试不消费 → panic 兜底。
func (s *stubChestService) OpenChest(ctx context.Context, in service.OpenChestInput) (*service.OpenChestOutput, error) {
	if s.openChestFn != nil {
		return s.openChestFn(ctx, in)
	}
	panic("stubChestService.OpenChest not configured")
}

// newChestHandlerRouter 构造测试 router：挂 ErrorMappingMiddleware + 注入 mockUserID + 注册 GET /chest/current。
//
// 模式与 newStepsHandlerRouter / newHomeHandlerRouter 同（steps_handler_test.go 已建）。
// mockUserID = nil 不挂 mock auth middleware → 测 unreachable userID 缺失分支。
func newChestHandlerRouter(svc service.ChestService, mockUserID *uint64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorMappingMiddleware())
	if mockUserID != nil {
		uid := *mockUserID
		r.Use(func(c *gin.Context) {
			c.Set(middleware.UserIDKey, uid)
			c.Next()
		})
	}
	// Story 20.6 改：NewChestHandler 签名 3 参数；20.5 GetCurrent 测试不消费 idempotencyChecker / rateLimitCfg，
	// 传入 nil + zero-value 即可（GetCurrent 路径不访问该字段）。
	h := handler.NewChestHandler(svc, nil, config.RateLimitConfig{})
	r.GET("/api/v1/chest/current", h.GetCurrent)
	return r
}

func decodeChestEnvelope(t *testing.T, body []byte) response.Envelope {
	t.Helper()
	var env response.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("invalid JSON envelope: %v; body=%s", err, string(body))
	}
	return env
}

// 1. HappyPath_FlatSchema:
// 合法 GET → 200 + envelope.code=0 + data 扁平 5 字段（id / status / unlockAt / openCostSteps / remainingSeconds）
// **核心断言**：data 没有 "chest" 子对象（V1 §7.1 vs §5.1 schema 差异核心断言点）
func TestChestHandler_GetCurrent_HappyPath_FlatSchema(t *testing.T) {
	uid := uint64(1001)
	fixedUnlockAt := time.Date(2026, 4, 23, 10, 20, 0, 0, time.UTC)
	svc := &stubChestService{
		getCurrentFn: func(ctx context.Context, userID uint64) (*service.ChestBrief, error) {
			if userID != 1001 {
				t.Errorf("svc.GetCurrent userID = %d, want 1001 (透传校验)", userID)
			}
			return &service.ChestBrief{
				ID:               5001,
				Status:           1,
				UnlockAt:         fixedUnlockAt,
				OpenCostSteps:    1000,
				RemainingSeconds: 253,
			}, nil
		},
	}
	r := newChestHandlerRouter(svc, &uid)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/chest/current", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeChestEnvelope(t, w.Body.Bytes())
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
	// **关键 AC9**：扁平 schema —— data.id 顶级；**没有** data.chest 子对象
	// （V1 §7.1 vs §5.1 schema 差异核心断言点）
	if _, hasChest := data["chest"]; hasChest {
		t.Errorf("data.chest 不应存在（V1 §7.1 钦定扁平 schema；嵌套是 §5.1 GET /home 的事）；data=%+v", data)
	}

	// data.id 必须是 string 类型（BIGINT 字符串化；V1 §2.5 + §7.1 行 850 钦定）
	idStr, idIsString := data["id"].(string)
	if !idIsString {
		t.Errorf("data.id 类型 = %T, want string (BIGINT 字符串化)；防 future regression 把 id 改回 number", data["id"])
	}
	if idStr != "5001" {
		t.Errorf("data.id = %q, want \"5001\"", idStr)
	}

	if status, _ := data["status"].(float64); status != 1 {
		t.Errorf("data.status = %v, want 1", data["status"])
	}
	if unlockAt, _ := data["unlockAt"].(string); unlockAt != "2026-04-23T10:20:00Z" {
		t.Errorf("data.unlockAt = %q, want %q (RFC3339 UTC)", data["unlockAt"], "2026-04-23T10:20:00Z")
	}
	if openCostSteps, _ := data["openCostSteps"].(float64); openCostSteps != 1000 {
		t.Errorf("data.openCostSteps = %v, want 1000", data["openCostSteps"])
	}
	if remainingSeconds, _ := data["remainingSeconds"].(float64); remainingSeconds != 253 {
		t.Errorf("data.remainingSeconds = %v, want 253", data["remainingSeconds"])
	}
}

// 2. BIGINTIDStringified:
// ID = math.MaxUint64（极端 BIGINT 值）→ data.id 必须是 string "18446744073709551615"（精确）
// 防御断言：JS Number 不能精确表示 > 2^53 的 uint64；若 id 被序列化为 number 精度会损失
func TestChestHandler_GetCurrent_BIGINTIDStringified(t *testing.T) {
	uid := uint64(1001)
	svc := &stubChestService{
		getCurrentFn: func(ctx context.Context, userID uint64) (*service.ChestBrief, error) {
			return &service.ChestBrief{
				ID:               math.MaxUint64, // 极端 BIGINT 值
				Status:           1,
				UnlockAt:         time.Date(2026, 4, 23, 10, 20, 0, 0, time.UTC),
				OpenCostSteps:    1000,
				RemainingSeconds: 100,
			}, nil
		},
	}
	r := newChestHandlerRouter(svc, &uid)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/chest/current", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeChestEnvelope(t, w.Body.Bytes())

	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("envelope.data not object: %T", env.Data)
	}
	idStr, idIsString := data["id"].(string)
	if !idIsString {
		t.Fatalf("data.id 类型 = %T, want string；BIGINT 必须字符串化否则 JS Number 精度损失", data["id"])
	}
	want := strconv.FormatUint(math.MaxUint64, 10) // "18446744073709551615"
	if idStr != want {
		t.Errorf("data.id = %q, want %q (MaxUint64 精确字符串化)", idStr, want)
	}
}

// 3. ServiceReturns4001_ForwardsAsCode4001_HTTP200:
// stub service 返 *AppError(ErrChestNotFound, "当前宝箱不存在") → handler c.Error → middleware envelope code=4001
// HTTP **200**（V1 §2.4 钦定业务错也走 HTTP 200，不是 4xx；只有 1009 走 500）
func TestChestHandler_GetCurrent_ServiceReturns4001_ForwardsAsCode4001_HTTP200(t *testing.T) {
	uid := uint64(1001)
	svc := &stubChestService{
		getCurrentFn: func(ctx context.Context, userID uint64) (*service.ChestBrief, error) {
			return nil, apperror.New(apperror.ErrChestNotFound, apperror.DefaultMessages[apperror.ErrChestNotFound])
		},
	}
	r := newChestHandlerRouter(svc, &uid)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/chest/current", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 业务码 4001 走 HTTP 200（V1 §2.4 钦定；只有 1009 走 500）
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (业务码 4001 走 200, V1 §2.4)", w.Code)
	}
	env := decodeChestEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrChestNotFound {
		t.Errorf("envelope.code = %d, want %d (4001)", env.Code, apperror.ErrChestNotFound)
	}
}

// 4. ServiceReturns1009_ForwardsAsCode1009_HTTP500:
// stub service 返 *AppError(ErrServiceBusy, "服务繁忙") → envelope code=1009，HTTP 500
// （error_mapping.go: ErrServiceBusy 是系统级 → HTTP 500；区别于 4001 业务码 → HTTP 200）
func TestChestHandler_GetCurrent_ServiceReturns1009_ForwardsAsCode1009_HTTP500(t *testing.T) {
	uid := uint64(1001)
	svc := &stubChestService{
		getCurrentFn: func(ctx context.Context, userID uint64) (*service.ChestBrief, error) {
			return nil, apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		},
	}
	r := newChestHandlerRouter(svc, &uid)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/chest/current", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (1009 走 500; ADR-0006)", w.Code)
	}
	env := decodeChestEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d (1009)", env.Code, apperror.ErrServiceBusy)
	}
}

// 5. MissingUserIDInContext_Returns1009:
// 单测启动 router 时 mockUserID = nil（不注入 userID 到 c.Keys）→ handler 走 unreachable 兜底 → 1009
func TestChestHandler_GetCurrent_MissingUserIDInContext_Returns1009(t *testing.T) {
	svc := &stubChestService{
		getCurrentFn: func(ctx context.Context, userID uint64) (*service.ChestBrief, error) {
			t.Errorf("service should NOT be called when userID missing")
			return nil, nil
		},
	}
	// mockUserID = nil → 不挂 mock auth middleware
	r := newChestHandlerRouter(svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/chest/current", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (1009 走 500; ADR-0006)", w.Code)
	}
	env := decodeChestEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d (1009)", env.Code, apperror.ErrServiceBusy)
	}
}
