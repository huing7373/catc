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
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// ============================================================
// stub StepService（与 4.6 stubAuthService / 4.8 stubHomeService 同模式）
// ============================================================

type stubStepService struct {
	syncStepsFn func(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error)
}

func (s *stubStepService) SyncSteps(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error) {
	return s.syncStepsFn(ctx, in)
}

// newStepsHandlerRouter 构造 handler test router。
//
// 必挂 ErrorMappingMiddleware（否则 c.Error 不写 envelope，断不到 envelope.code）。
// 可选挂 mock auth middleware（直接 c.Set UserIDKey 给定 uint64 值）。
//
// mockUserID = nil 不挂 mock auth → 测 unreachable userID 缺失分支。
func newStepsHandlerRouter(svc service.StepService, mockUserID *uint64) *gin.Engine {
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
	h := handler.NewStepsHandler(svc)
	r.POST("/api/v1/steps/sync", h.PostSync)
	return r
}

func decodeStepsEnvelope(t *testing.T, body []byte) response.Envelope {
	t.Helper()
	var env response.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("invalid JSON envelope: %v; body=%s", err, string(body))
	}
	return env
}

// ============================================================
// 6 个 case
// ============================================================

// 1. HappyPath_ReturnsCorrectSchema:
// 合法 request → 200 + envelope.code=0 + data.acceptedDeltaSteps + data.stepAccount.{total/available/consumed}
func TestStepsHandler_PostSync_HappyPath_ReturnsCorrectSchema(t *testing.T) {
	uid := uint64(1001)

	svc := &stubStepService{
		syncStepsFn: func(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error) {
			if in.UserID != 1001 {
				t.Errorf("svc.UserID = %d, want 1001", in.UserID)
			}
			if in.ClientTotalSteps != 100 {
				t.Errorf("svc.ClientTotalSteps = %d, want 100", in.ClientTotalSteps)
			}
			if in.MotionState != 2 {
				t.Errorf("svc.MotionState = %d, want 2", in.MotionState)
			}
			return &service.SyncStepsOutput{
				AcceptedDeltaSteps: 100,
				StepAccount: service.StepAccountBrief{
					TotalSteps: 100, AvailableSteps: 100, ConsumedSteps: 0,
				},
			}, nil
		},
	}
	r := newStepsHandlerRouter(svc, &uid)

	body := `{"syncDate":"2026-05-01","clientTotalSteps":100,"motionState":2,"clientTimestamp":1714560000000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/steps/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeStepsEnvelope(t, w.Body.Bytes())
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
	if ad, _ := data["acceptedDeltaSteps"].(float64); ad != 100 {
		t.Errorf("acceptedDeltaSteps = %v, want 100", data["acceptedDeltaSteps"])
	}
	stepAcc, ok := data["stepAccount"].(map[string]any)
	if !ok {
		t.Fatalf("data.stepAccount not object: %T", data["stepAccount"])
	}
	if ts, _ := stepAcc["totalSteps"].(float64); ts != 100 {
		t.Errorf("stepAccount.totalSteps = %v, want 100", stepAcc["totalSteps"])
	}
	if as, _ := stepAcc["availableSteps"].(float64); as != 100 {
		t.Errorf("stepAccount.availableSteps = %v, want 100", stepAcc["availableSteps"])
	}
	if cs, _ := stepAcc["consumedSteps"].(float64); cs != 0 {
		t.Errorf("stepAccount.consumedSteps = %v, want 0", stepAcc["consumedSteps"])
	}
}

// 2. InvalidSyncDateFormat_Returns1002:
// syncDate = "2026/04/23" → 1002（YYYY/MM/DD 不符 YYYY-MM-DD 格式）。
func TestStepsHandler_PostSync_InvalidSyncDateFormat_Returns1002(t *testing.T) {
	uid := uint64(1001)
	svc := &stubStepService{
		syncStepsFn: func(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error) {
			t.Errorf("service should NOT be called when syncDate format invalid")
			return nil, nil
		},
	}
	r := newStepsHandlerRouter(svc, &uid)

	body := `{"syncDate":"2026/04/23","clientTotalSteps":100,"motionState":2,"clientTimestamp":1714560000000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/steps/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (业务码 1002 走 200; ADR-0006/V1 §2.4)", w.Code)
	}
	env := decodeStepsEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
}

// 3. MotionStateOutOfRange_Returns1002:
// motionState = 5 → 1002。
func TestStepsHandler_PostSync_MotionStateOutOfRange_Returns1002(t *testing.T) {
	uid := uint64(1001)
	svc := &stubStepService{
		syncStepsFn: func(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error) {
			t.Errorf("service should NOT be called when motionState invalid")
			return nil, nil
		},
	}
	r := newStepsHandlerRouter(svc, &uid)

	body := `{"syncDate":"2026-05-01","clientTotalSteps":100,"motionState":5,"clientTimestamp":1714560000000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/steps/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodeStepsEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d", env.Code, apperror.ErrInvalidParam)
	}
}

// 4. ServiceReturns3001_HandlerForwardsAsCode3001_HTTP200:
// service 返 *AppError(ErrStepSyncInvalid) → handler c.Error → middleware envelope code=3001，HTTP 200。
func TestStepsHandler_PostSync_ServiceReturns3001_HandlerForwardsAsCode3001_HTTP200(t *testing.T) {
	uid := uint64(1001)
	svc := &stubStepService{
		syncStepsFn: func(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error) {
			return nil, apperror.New(apperror.ErrStepSyncInvalid, apperror.DefaultMessages[apperror.ErrStepSyncInvalid])
		},
	}
	r := newStepsHandlerRouter(svc, &uid)

	body := `{"syncDate":"2026-05-01","clientTotalSteps":53000,"motionState":2,"clientTimestamp":1714560000000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/steps/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 业务码 3001 走 HTTP 200（V1 §2.4 钦定；只有 1009 走 500）
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (业务码 3001 走 200)", w.Code)
	}
	env := decodeStepsEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrStepSyncInvalid {
		t.Errorf("envelope.code = %d, want %d (3001)", env.Code, apperror.ErrStepSyncInvalid)
	}
}

// 5. ServiceReturnsBusyErr_Forwards1009:
// service 返 ErrServiceBusy → envelope code=1009，HTTP 500。
func TestStepsHandler_PostSync_ServiceReturnsBusyErr_Forwards1009(t *testing.T) {
	uid := uint64(1001)
	svc := &stubStepService{
		syncStepsFn: func(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error) {
			return nil, apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		},
	}
	r := newStepsHandlerRouter(svc, &uid)

	body := `{"syncDate":"2026-05-01","clientTotalSteps":100,"motionState":2,"clientTimestamp":1714560000000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/steps/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (1009 走 500; ADR-0006)", w.Code)
	}
	env := decodeStepsEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d", env.Code, apperror.ErrServiceBusy)
	}
}

// 6. MissingUserIDInContext_Returns1009:
// 单测启动 router 时 **不**注入 userID 到 c.Keys → handler 走 unreachable bug 兜底 → 1009。
func TestStepsHandler_PostSync_MissingUserIDInContext_Returns1009(t *testing.T) {
	svc := &stubStepService{
		syncStepsFn: func(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error) {
			t.Errorf("service should NOT be called when userID missing")
			return nil, nil
		},
	}
	// mockUserID = nil → 不挂 mock auth middleware
	r := newStepsHandlerRouter(svc, nil)

	body := `{"syncDate":"2026-05-01","clientTotalSteps":100,"motionState":2,"clientTimestamp":1714560000000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/steps/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	env := decodeStepsEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d", env.Code, apperror.ErrServiceBusy)
	}
}

// 7. SyncDatePassedAsStringToService_TZIndependent:
// 验证 handler 把 syncDate 作为 **string** 透传给 service（Story 7.3 review r2 [P2]）；
// 全程不走 time.Time，无 time.Local / DSN loc 耦合。
//
// 反例（r1 旧版）：handler 用 time.ParseInLocation(time.Local)，依赖 DSN loc=Local
// 才正确；DSN loc=UTC 时仍漂日。本轮根治：service.SyncStepsInput.SyncDate
// 字段类型 = string，repo `WHERE sync_date = ?` 直传 string，driver 走
// VARCHAR→DATE 隐式转换，**完全无时区语义**。
//
// 详见 docs/lessons/2026-05-02-mysql-date-string-transit.md（接力上一轮
// 2026-05-02-mysql-date-gorm-time-tz-pitfall.md 的递进根治）。
//
// **本 case 不真起 mysql**（那是 step_service_integration_test 的事），只在
// service stub 里捕获 in.SyncDate 验证字符串原样透传。
func TestStepsHandler_PostSync_SyncDatePassedAsStringToService_TZIndependent(t *testing.T) {
	uid := uint64(1001)
	var captured string
	svc := &stubStepService{
		syncStepsFn: func(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error) {
			captured = in.SyncDate
			return &service.SyncStepsOutput{
				AcceptedDeltaSteps: 0,
				StepAccount:        service.StepAccountBrief{},
			}, nil
		},
	}
	r := newStepsHandlerRouter(svc, &uid)

	body := `{"syncDate":"2026-05-01","clientTotalSteps":100,"motionState":2,"clientTimestamp":1714560000000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/steps/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	// 字符串原样透传（不 parse 不 format，与时区无关）
	if captured != "2026-05-01" {
		t.Errorf("captured SyncDate = %q, want %q", captured, "2026-05-01")
	}
}

// 8. ClientTotalStepsMissing_Returns1002:
// Story 7.3 review r2 [P2]：clientTotalSteps 字段在 JSON 中缺失（pointer = nil）→ 1002。
// 反例：r1 用 int64 值类型，缺失绑定为 0 与显式 0 无法区分。
func TestStepsHandler_PostSync_ClientTotalStepsMissing_Returns1002(t *testing.T) {
	uid := uint64(1001)
	svc := &stubStepService{
		syncStepsFn: func(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error) {
			t.Errorf("service should NOT be called when clientTotalSteps missing")
			return nil, nil
		},
	}
	r := newStepsHandlerRouter(svc, &uid)

	// 故意省略 clientTotalSteps 字段
	body := `{"syncDate":"2026-05-01","motionState":2,"clientTimestamp":1714560000000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/steps/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodeStepsEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "clientTotalSteps") {
		t.Errorf("envelope.message = %q, want 包含 clientTotalSteps 定位字段", env.Message)
	}
}

// 9. ClientTotalStepsExplicitZero_AllowedThrough:
// Story 7.3 review r2 [P2]：clientTotalSteps 显式传 0 是合法（首次同步当日 0 步），
// 应进入 service。区别于 case 8 的"缺失"。
func TestStepsHandler_PostSync_ClientTotalStepsExplicitZero_AllowedThrough(t *testing.T) {
	uid := uint64(1001)
	called := false
	svc := &stubStepService{
		syncStepsFn: func(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error) {
			called = true
			if in.ClientTotalSteps != 0 {
				t.Errorf("service.ClientTotalSteps = %d, want 0 (显式 0 透传)", in.ClientTotalSteps)
			}
			return &service.SyncStepsOutput{AcceptedDeltaSteps: 0, StepAccount: service.StepAccountBrief{}}, nil
		},
	}
	r := newStepsHandlerRouter(svc, &uid)

	body := `{"syncDate":"2026-05-01","clientTotalSteps":0,"motionState":2,"clientTimestamp":1714560000000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/steps/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !called {
		t.Fatal("service should be called when clientTotalSteps=0 (显式 0 合法)")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
}

// 10. MotionStateMissing_Returns1002:
func TestStepsHandler_PostSync_MotionStateMissing_Returns1002(t *testing.T) {
	uid := uint64(1001)
	svc := &stubStepService{
		syncStepsFn: func(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error) {
			t.Errorf("service should NOT be called when motionState missing")
			return nil, nil
		},
	}
	r := newStepsHandlerRouter(svc, &uid)

	body := `{"syncDate":"2026-05-01","clientTotalSteps":100,"clientTimestamp":1714560000000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/steps/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodeStepsEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "motionState") {
		t.Errorf("envelope.message = %q, want 包含 motionState 定位字段", env.Message)
	}
}

// 11. ClientTimestampMissing_Returns1002:
func TestStepsHandler_PostSync_ClientTimestampMissing_Returns1002(t *testing.T) {
	uid := uint64(1001)
	svc := &stubStepService{
		syncStepsFn: func(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error) {
			t.Errorf("service should NOT be called when clientTimestamp missing")
			return nil, nil
		},
	}
	r := newStepsHandlerRouter(svc, &uid)

	body := `{"syncDate":"2026-05-01","clientTotalSteps":100,"motionState":2}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/steps/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodeStepsEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "clientTimestamp") {
		t.Errorf("envelope.message = %q, want 包含 clientTimestamp 定位字段", env.Message)
	}
}

// ============================================================
// Story 7.3 review r4 [P2]：syncDate 必须用 MySQL DATE 物理范围 [1000-01-01, 9999-12-31]
// 在 handler 拦截，避免下游 driver 报错把 1002 误转 1009。
// ============================================================

// runSyncDateRangeCase 是下方 4 个 boundary case 的共享 driver。
//
// 期望逻辑：
//   - wantInvalid=true  → service 不应被调用，envelope.code=1002（ErrInvalidParam），HTTP 200
//   - wantInvalid=false → service 应被调用一次，envelope.code=0，HTTP 200
//
// 不真起 mysql；只验 handler 入口对 syncDate 的判定。
func runSyncDateRangeCase(t *testing.T, syncDate string, wantInvalid bool) {
	t.Helper()
	uid := uint64(1001)
	called := false
	svc := &stubStepService{
		syncStepsFn: func(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error) {
			called = true
			return &service.SyncStepsOutput{AcceptedDeltaSteps: 0, StepAccount: service.StepAccountBrief{}}, nil
		},
	}
	r := newStepsHandlerRouter(svc, &uid)

	body := `{"syncDate":"` + syncDate + `","clientTotalSteps":100,"motionState":2,"clientTimestamp":1714560000000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/steps/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodeStepsEnvelope(t, w.Body.Bytes())
	if wantInvalid {
		if called {
			t.Errorf("syncDate=%q: service should NOT be called (out of MySQL DATE range)", syncDate)
		}
		if env.Code != apperror.ErrInvalidParam {
			t.Errorf("syncDate=%q: envelope.code = %d, want %d (1002)", syncDate, env.Code, apperror.ErrInvalidParam)
		}
	} else {
		if !called {
			t.Errorf("syncDate=%q: service should be called (within MySQL DATE range)", syncDate)
		}
		if env.Code != 0 {
			t.Errorf("syncDate=%q: envelope.code = %d, want 0 (within range)", syncDate, env.Code)
		}
	}
}

// 12. SyncDatePre1000_Returns1002:
// "0999-12-31" 是 Go time.Parse 接受的合法日期，但 MySQL DATE 不接受（< 1000-01-01）。
// 必须在 handler 拦掉 → 1002，**不**让它走到 DB 然后被 driver 拒（那会变成 1009）。
func TestStepsHandler_PostSync_SyncDatePre1000_Returns1002(t *testing.T) {
	runSyncDateRangeCase(t, "0999-12-31", true /* wantInvalid */)
}

// 13. SyncDateFiveDigitYear_Returns1002:
// "10000-01-01" 长度 = 11 ≠ 10 → 被现有 len 校验拦掉 → 1002。
// 即便绕过 len 校验（如未来某次重构），time.Parse("2006-01-02") 解析 5 位年份会失败 →
// isValidYYYYMMDD 返 false → 仍 1002。本 case 锁死 5 位年份永不放行。
func TestStepsHandler_PostSync_SyncDateFiveDigitYear_Returns1002(t *testing.T) {
	runSyncDateRangeCase(t, "10000-01-01", true /* wantInvalid */)
}

// 14. SyncDateMinBoundary_Allowed:
// "1000-01-01" 是 MySQL DATE 物理下界，应通过 isValidYYYYMMDD → service 被调。
func TestStepsHandler_PostSync_SyncDateMinBoundary_Allowed(t *testing.T) {
	runSyncDateRangeCase(t, "1000-01-01", false /* wantInvalid */)
}

// 15. SyncDateMaxBoundary_Allowed:
// "9999-12-31" 是 MySQL DATE 物理上界，应通过 isValidYYYYMMDD → service 被调。
// 业务上不会真有这种日期，但 handler 应只校验物理范围，不加业务上界。
func TestStepsHandler_PostSync_SyncDateMaxBoundary_Allowed(t *testing.T) {
	runSyncDateRangeCase(t, "9999-12-31", false /* wantInvalid */)
}
