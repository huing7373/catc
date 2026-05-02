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

// 7. SyncDateParsedInLocalLocation_TZSafeRegression:
// 验证 handler 用 time.ParseInLocation(time.Local) 而非 time.Parse；
// 这样 mysql driver 在 loc=Local DSN 下序列化到 DATE 列时不会"飘日"。
//
// 反例（旧 time.Parse）：返回 UTC 0:00；负偏移服务器把 2026-05-01 → DATE 写
// 2026-04-30。详见 docs/lessons/2026-05-02-mysql-date-gorm-time-tz-pitfall.md。
//
// **本 case 不真起 mysql**（那是 step_service_integration_test 的事），只在
// service stub 里捕获 in.SyncDate 验证：
//   - Location() == time.Local（与 DSN loc=Local 锁同步）
//   - Year/Month/Day 与请求 syncDate 字符串一致（不被 parse 时区漂移影响）
//   - 时分秒 == 0（midnight）
func TestStepsHandler_PostSync_SyncDateParsedInLocalLocation_TZSafeRegression(t *testing.T) {
	uid := uint64(1001)
	var captured time.Time
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
	// Location 必须 == time.Local（不是 time.UTC）：与 DSN loc=Local 锁同步。
	if captured.Location() != time.Local {
		t.Errorf("syncDate.Location() = %v, want time.Local（防 mysql DATE 时区漂移）", captured.Location())
	}
	// Year/Month/Day 必须 == 请求字符串（不能被 parse 转换 UTC 漂离）。
	if got := captured.Year(); got != 2026 {
		t.Errorf("syncDate.Year() = %d, want 2026", got)
	}
	if got := captured.Month(); got != time.May {
		t.Errorf("syncDate.Month() = %v, want May", got)
	}
	if got := captured.Day(); got != 1 {
		t.Errorf("syncDate.Day() = %d, want 1", got)
	}
	// 时分秒必须为 0（midnight）。
	if h, m, s := captured.Clock(); h != 0 || m != 0 || s != 0 {
		t.Errorf("syncDate clock = %02d:%02d:%02d, want 00:00:00", h, m, s)
	}
}
