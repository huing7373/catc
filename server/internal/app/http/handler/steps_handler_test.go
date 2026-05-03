package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	syncStepsFn  func(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error)
	getAccountFn func(ctx context.Context, userID uint64) (*service.StepAccountBrief, error) // Story 7.4 加
}

func (s *stubStepService) SyncSteps(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error) {
	return s.syncStepsFn(ctx, in)
}

// GetAccount 实装（Story 7.4 加）。
func (s *stubStepService) GetAccount(ctx context.Context, userID uint64) (*service.StepAccountBrief, error) {
	return s.getAccountFn(ctx, userID)
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
	r.GET("/api/v1/steps/account", h.GetAccount) // Story 7.4 加
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

// dynamicValidSyncDate 返回当前 server today（UTC）的 YYYY-MM-DD string，
// 必落在 r7 引入的 ±2 天容忍窗口内。
//
// 所有"需要让 syncDate 通过校验"的 case **必须**用这个函数构造 body，
// 而非硬编码 "2026-05-01"——硬编码在系统真实时间漂离 2026-05-02±2 后会破。
func dynamicValidSyncDate() string {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

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

	body := `{"syncDate":"` + dynamicValidSyncDate() + `","clientTotalSteps":100,"motionState":2,"clientTimestamp":1714560000000}`
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

	body := `{"syncDate":"` + dynamicValidSyncDate() + `","clientTotalSteps":100,"motionState":5,"clientTimestamp":1714560000000}`
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

	body := `{"syncDate":"` + dynamicValidSyncDate() + `","clientTotalSteps":53000,"motionState":2,"clientTimestamp":1714560000000}`
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

	body := `{"syncDate":"` + dynamicValidSyncDate() + `","clientTotalSteps":100,"motionState":2,"clientTimestamp":1714560000000}`
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

	body := `{"syncDate":"` + dynamicValidSyncDate() + `","clientTotalSteps":100,"motionState":2,"clientTimestamp":1714560000000}`
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

	wantSyncDate := dynamicValidSyncDate()
	body := `{"syncDate":"` + wantSyncDate + `","clientTotalSteps":100,"motionState":2,"clientTimestamp":1714560000000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/steps/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	// 字符串原样透传（不 parse 不 format，与时区无关）
	if captured != wantSyncDate {
		t.Errorf("captured SyncDate = %q, want %q", captured, wantSyncDate)
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
	body := `{"syncDate":"` + dynamicValidSyncDate() + `","motionState":2,"clientTimestamp":1714560000000}`
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

	body := `{"syncDate":"` + dynamicValidSyncDate() + `","clientTotalSteps":0,"motionState":2,"clientTimestamp":1714560000000}`
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

	body := `{"syncDate":"` + dynamicValidSyncDate() + `","clientTotalSteps":100,"clientTimestamp":1714560000000}`
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

	body := `{"syncDate":"` + dynamicValidSyncDate() + `","clientTotalSteps":100,"motionState":2}`
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

// 14. SyncDateMinBoundary_PassesFormatButRejectedByToleranceWindow:
// "1000-01-01" 通过 isValidYYYYMMDD（MySQL DATE 物理下界 ok），但被 r7 引入的
// ±2 天容忍窗口拒（远离 server today）→ 仍 1002。本 case 锁死 r4+r7 双校验组合：
//   - r4：物理范围 [1000-01-01, 9999-12-31] 通过
//   - r7：实际再被 [server today - 2, server today + 2] 拦截
//
// 注：本 case 只验"被拒"，不强求拒的是哪一层（实装中 r7 在 r4 之后，所以 r7 拦掉）。
func TestStepsHandler_PostSync_SyncDateMinBoundary_PassesFormatButRejectedByToleranceWindow(t *testing.T) {
	runSyncDateRangeCase(t, "1000-01-01", true /* wantInvalid: r7 容忍窗口拦截 */)
}

// 15. SyncDateMaxBoundary_PassesFormatButRejectedByToleranceWindow:
// "9999-12-31" 同 case 14：物理范围通过、容忍窗口拒。
func TestStepsHandler_PostSync_SyncDateMaxBoundary_PassesFormatButRejectedByToleranceWindow(t *testing.T) {
	runSyncDateRangeCase(t, "9999-12-31", true /* wantInvalid: r7 容忍窗口拦截 */)
}

// ============================================================
// Story 7.3 review r7 [P1]：syncDate 必须 ∈ [server today - 2, server today + 2]
// 防止恶意客户端旋转日期重复入账绕过 daily_cap。
// 所有 case 用 server today 相对偏移构造（避免硬编码日期跑到未来失效）。
// ============================================================

// formatRelativeDate 把 server today 加上 offset 天数，返回 YYYY-MM-DD string。
// offsetDays = 0 → server today（UTC）；正数 = 未来；负数 = 过去。
func formatRelativeDate(offsetDays int) string {
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return today.AddDate(0, 0, offsetDays).Format("2006-01-02")
}

// 16. SyncDateEqualsServerToday_Allowed:
// offset = 0 → 必通过（中心点）。
func TestStepsHandler_PostSync_SyncDateEqualsServerToday_Allowed(t *testing.T) {
	runSyncDateRangeCase(t, formatRelativeDate(0), false /* wantInvalid: 在窗口内 */)
}

// 17. SyncDateMinusTwoDays_Allowed:
// offset = -2 → 窗口下界，必通过。
func TestStepsHandler_PostSync_SyncDateMinusTwoDays_Allowed(t *testing.T) {
	runSyncDateRangeCase(t, formatRelativeDate(-2), false /* wantInvalid: 下界含 */)
}

// 18. SyncDatePlusTwoDays_Allowed:
// offset = +2 → 窗口上界，必通过。
func TestStepsHandler_PostSync_SyncDatePlusTwoDays_Allowed(t *testing.T) {
	runSyncDateRangeCase(t, formatRelativeDate(2), false /* wantInvalid: 上界含 */)
}

// 19. SyncDateMinusThreeDays_Returns1002:
// offset = -3 → 窗口外，必拒。
func TestStepsHandler_PostSync_SyncDateMinusThreeDays_Returns1002(t *testing.T) {
	runSyncDateRangeCase(t, formatRelativeDate(-3), true /* wantInvalid: 越出下界 */)
}

// 20. SyncDatePlusThreeDays_Returns1002:
// offset = +3 → 窗口外，必拒。
func TestStepsHandler_PostSync_SyncDatePlusThreeDays_Returns1002(t *testing.T) {
	runSyncDateRangeCase(t, formatRelativeDate(3), true /* wantInvalid: 越出上界 */)
}

// 21. SyncDateRotationAttack_AllNearbyDatesPass_FarRejected:
// 模拟 review r7 描述的旋转攻击场景（在容忍窗口内允许；窗口外拒）。
//   - server today, today-1, today+1, today-2, today+2 → 全通过（窗口内）
//   - today-7, today+7 → 全拒（远超窗口）
//
// 这道 case 锁死"窗口大小是 ±2 天而非 ±7 天"——若未来误把窗口扩大成 ±7，本 case 会破。
func TestStepsHandler_PostSync_SyncDateRotationAttack_AllNearbyDatesPass_FarRejected(t *testing.T) {
	// 窗口内：今 / 昨 / 前 / 明 / 后 → 全通过
	for _, off := range []int{-2, -1, 0, 1, 2} {
		runSyncDateRangeCase(t, formatRelativeDate(off), false /* wantInvalid */)
	}
	// 窗口外：±7 天 → 全拒
	for _, off := range []int{-7, 7} {
		runSyncDateRangeCase(t, formatRelativeDate(off), true /* wantInvalid */)
	}
}

// 22. SyncDateRejectMessageContainsToleranceHint:
// 拒绝消息必须包含"server today" / "天"等关键字方便 iOS debug。
// 不强求精确字符串，只校"包含中文'天'+ '范围'"，避免 i18n 微调破测。
func TestStepsHandler_PostSync_SyncDateRejectMessageContainsToleranceHint(t *testing.T) {
	uid := uint64(1001)
	called := false
	svc := &stubStepService{
		syncStepsFn: func(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error) {
			called = true
			return &service.SyncStepsOutput{}, nil
		},
	}
	r := newStepsHandlerRouter(svc, &uid)

	body := `{"syncDate":"` + formatRelativeDate(-10) + `","clientTotalSteps":100,"motionState":2,"clientTimestamp":1714560000000}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/steps/sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if called {
		t.Errorf("service should NOT be called for out-of-window syncDate")
	}
	env := decodeStepsEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	// 关键关键字：让 iOS 端读到错误消息能定位到"是 syncDate 范围越界"
	if !strings.Contains(env.Message, "syncDate") {
		t.Errorf("envelope.message = %q, 缺少 syncDate 关键字（iOS debug 不友好）", env.Message)
	}
	if !strings.Contains(env.Message, "范围") && !strings.Contains(env.Message, "天") {
		t.Errorf("envelope.message = %q, 缺少范围/天 提示词（让 client 知道是窗口越界，不是格式错）", env.Message)
	}
}

// ============================================================
// Story 7.4: GetAccount 3 case
//
// 关键约束（详见 Story 7.4 AC5）：
//   - HappyPath 必须断**扁平** schema（data.totalSteps 顶级；data.stepAccount 不存在）
//     这是 V1 §6.2 行 628 钦定差异的核心断言点（区别于 §6.1 PostSync 的嵌套 schema）
//   - 1003 case 验 HTTP 200 + envelope.code=1003（V1 §2.4 钦定业务错走 200）
//   - MissingUserID case 用 mockUserID = nil → handler 走 unreachable 兜底 → 1009
// ============================================================

// 23. HappyPath_ReturnsFlatSchema:
// 合法 GET → 200 + envelope.code=0 + data.{totalSteps, availableSteps, consumedSteps} **扁平**结构
// **关键断言**：data.stepAccount 子对象不存在（区别于 PostSync 嵌套）
func TestStepsHandler_GetAccount_HappyPath_ReturnsFlatSchema(t *testing.T) {
	uid := uint64(1001)
	svc := &stubStepService{
		getAccountFn: func(ctx context.Context, userID uint64) (*service.StepAccountBrief, error) {
			if userID != 1001 {
				t.Errorf("svc.GetAccount userID = %d, want 1001 (透传校验)", userID)
			}
			return &service.StepAccountBrief{
				TotalSteps: 1140, AvailableSteps: 840, ConsumedSteps: 300,
			}, nil
		},
	}
	r := newStepsHandlerRouter(svc, &uid)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/steps/account", nil)
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
	// **关键 AC8 #2 / #7**：扁平 schema —— data.totalSteps 顶级；**没有** data.stepAccount 子对象
	// （V1 §6.2 行 628 钦定差异；区别于 §6.1 PostSync 的 data.stepAccount.totalSteps 嵌套结构）
	if _, hasStepAccount := data["stepAccount"]; hasStepAccount {
		t.Errorf("data.stepAccount 不应存在（V1 §6.2 钦定扁平 schema；嵌套是 §6.1 PostSync 的事）；data=%+v", data)
	}
	if ts, _ := data["totalSteps"].(float64); ts != 1140 {
		t.Errorf("data.totalSteps = %v, want 1140 (顶级访问)", data["totalSteps"])
	}
	if as, _ := data["availableSteps"].(float64); as != 840 {
		t.Errorf("data.availableSteps = %v, want 840 (顶级访问)", data["availableSteps"])
	}
	if cs, _ := data["consumedSteps"].(float64); cs != 300 {
		t.Errorf("data.consumedSteps = %v, want 300 (顶级访问)", data["consumedSteps"])
	}
}

// 24. ServiceReturns1003_ForwardsAsCode1003_HTTP200:
// service 返 *AppError(ErrResourceNotFound, "资源不存在") → handler c.Error → middleware envelope code=1003
// HTTP **200**（V1 §2.4 钦定业务错也走 HTTP 200，不是 4xx）
func TestStepsHandler_GetAccount_ServiceReturns1003_ForwardsAsCode1003_HTTP200(t *testing.T) {
	uid := uint64(1001)
	svc := &stubStepService{
		getAccountFn: func(ctx context.Context, userID uint64) (*service.StepAccountBrief, error) {
			return nil, apperror.New(apperror.ErrResourceNotFound, apperror.DefaultMessages[apperror.ErrResourceNotFound])
		},
	}
	r := newStepsHandlerRouter(svc, &uid)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/steps/account", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 业务码 1003 走 HTTP 200（V1 §2.4 钦定；只有 1009 走 500）
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (业务码 1003 走 200, V1 §2.4)", w.Code)
	}
	env := decodeStepsEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrResourceNotFound {
		t.Errorf("envelope.code = %d, want %d (1003)", env.Code, apperror.ErrResourceNotFound)
	}
}

// 25. MissingUserIDInContext_Returns1009:
// 单测启动 router 时 mockUserID = nil（不注入 userID 到 c.Keys）→ handler 走 unreachable 兜底 → 1009
func TestStepsHandler_GetAccount_MissingUserIDInContext_Returns1009(t *testing.T) {
	svc := &stubStepService{
		getAccountFn: func(ctx context.Context, userID uint64) (*service.StepAccountBrief, error) {
			t.Errorf("service should NOT be called when userID missing")
			return nil, nil
		},
	}
	// mockUserID = nil → 不挂 mock auth middleware
	r := newStepsHandlerRouter(svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/steps/account", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (1009 走 500; ADR-0006)", w.Code)
	}
	env := decodeStepsEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d (1009)", env.Code, apperror.ErrServiceBusy)
	}
}

// 26. ServiceReturnsBusyErr_Forwards1009:
// service 返 ErrServiceBusy → envelope code=1009，HTTP 500（与 PostSync 同模式）
func TestStepsHandler_GetAccount_ServiceReturnsBusyErr_Forwards1009(t *testing.T) {
	uid := uint64(1001)
	svc := &stubStepService{
		getAccountFn: func(ctx context.Context, userID uint64) (*service.StepAccountBrief, error) {
			return nil, apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		},
	}
	r := newStepsHandlerRouter(svc, &uid)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/steps/account", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (1009 走 500)", w.Code)
	}
	env := decodeStepsEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d", env.Code, apperror.ErrServiceBusy)
	}
}
