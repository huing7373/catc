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
// stub PetService（与 4.6 stubAuthService / 4.8 stubHomeService / 7.3 stubStepService 同模式）
// ============================================================

type stubPetService struct {
	syncCurrentStateFn func(ctx context.Context, in service.SyncCurrentStateInput) (*service.SyncCurrentStateOutput, error)
	calls              int
	lastInput          service.SyncCurrentStateInput
}

func (s *stubPetService) SyncCurrentState(ctx context.Context, in service.SyncCurrentStateInput) (*service.SyncCurrentStateOutput, error) {
	s.calls++
	s.lastInput = in
	if s.syncCurrentStateFn == nil {
		return &service.SyncCurrentStateOutput{State: in.State}, nil
	}
	return s.syncCurrentStateFn(ctx, in)
}

// newPetsHandlerRouter 构造 handler test router。
//
// 必挂 ErrorMappingMiddleware（否则 c.Error 不写 envelope，断不到 envelope.code）。
// 可选挂 mock auth middleware（直接 c.Set UserIDKey 给定 uint64 值）。
//
// mockUserID = nil 不挂 mock auth → 测 unreachable userID 缺失分支。
func newPetsHandlerRouter(svc service.PetService, mockUserID *uint64) *gin.Engine {
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
	h := handler.NewPetsHandler(svc)
	r.POST("/api/v1/pets/current/state-sync", h.PostStateSync)
	return r
}

func decodePetsEnvelope(t *testing.T, body []byte) response.Envelope {
	t.Helper()
	var env response.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("invalid JSON envelope: %v; body=%s", err, string(body))
	}
	return env
}

// ============================================================
// 5 个 case（AC6 钦定 ≥4 + 可选 case 5）
// ============================================================

// case 1 — happy state=2
// stub PetService.SyncCurrentState 返 &SyncCurrentStateOutput{State: 2}, nil →
// POST /api/v1/pets/current/state-sync {"state": 2} → 200 OK + envelope
// {code:0, message:"ok", data:{state:2}, requestId:"..."} + stub 调用次数 1 + UserID 透传正确。
func TestPetsHandler_PostStateSync_Happy_State2(t *testing.T) {
	uid := uint64(10)
	svc := &stubPetService{
		syncCurrentStateFn: func(ctx context.Context, in service.SyncCurrentStateInput) (*service.SyncCurrentStateOutput, error) {
			if in.UserID != 10 {
				t.Errorf("svc.UserID = %d, want 10", in.UserID)
			}
			if in.State != 2 {
				t.Errorf("svc.State = %d, want 2", in.State)
			}
			return &service.SyncCurrentStateOutput{State: 2}, nil
		},
	}
	r := newPetsHandlerRouter(svc, &uid)

	body := `{"state":2}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pets/current/state-sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodePetsEnvelope(t, w.Body.Bytes())
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
	// data.state 是 int8 → JSON number → Go float64
	if state, _ := data["state"].(float64); state != 2 {
		t.Errorf("data.state = %v, want 2", data["state"])
	}
	if svc.calls != 1 {
		t.Errorf("svc.calls = %d, want 1", svc.calls)
	}
}

// case 2 — state 字段缺失 → 1002
// POST {} → envelope code=1002 + message "state 必填" + stub 调用次数 0。
func TestPetsHandler_PostStateSync_StateMissing_Returns1002(t *testing.T) {
	uid := uint64(10)
	svc := &stubPetService{
		syncCurrentStateFn: func(ctx context.Context, in service.SyncCurrentStateInput) (*service.SyncCurrentStateOutput, error) {
			t.Errorf("service should NOT be called when state missing")
			return nil, nil
		},
	}
	r := newPetsHandlerRouter(svc, &uid)

	body := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pets/current/state-sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodePetsEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "state") {
		t.Errorf("envelope.message = %q, want 包含 state 字段定位", env.Message)
	}
	if svc.calls != 0 {
		t.Errorf("svc.calls = %d, want 0", svc.calls)
	}
}

// case 3 — state=4 非法 → 1002
// POST {"state": 4} → envelope code=1002 + message "state 必须是 1 / 2 / 3" + stub 调用次数 0。
func TestPetsHandler_PostStateSync_StateOutOfRange_Returns1002(t *testing.T) {
	uid := uint64(10)
	svc := &stubPetService{
		syncCurrentStateFn: func(ctx context.Context, in service.SyncCurrentStateInput) (*service.SyncCurrentStateOutput, error) {
			t.Errorf("service should NOT be called when state out of range")
			return nil, nil
		},
	}
	r := newPetsHandlerRouter(svc, &uid)

	body := `{"state":4}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pets/current/state-sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodePetsEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	if !strings.Contains(env.Message, "1 / 2 / 3") {
		t.Errorf("envelope.message = %q, want 包含 '1 / 2 / 3' 范围提示", env.Message)
	}
	if svc.calls != 0 {
		t.Errorf("svc.calls = %d, want 0", svc.calls)
	}
}

// case 4 — service 返 1009 → 1009 envelope
// stub PetService.SyncCurrentState 返 nil, apperror.New(ErrServiceBusy, "服务繁忙") →
// POST {"state": 2} → 500 / 1009 envelope + stub 调用次数 1。
func TestPetsHandler_PostStateSync_ServiceReturns1009_Forwards1009(t *testing.T) {
	uid := uint64(10)
	svc := &stubPetService{
		syncCurrentStateFn: func(ctx context.Context, in service.SyncCurrentStateInput) (*service.SyncCurrentStateOutput, error) {
			return nil, apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		},
	}
	r := newPetsHandlerRouter(svc, &uid)

	body := `{"state":2}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pets/current/state-sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (1009 走 500; ADR-0006)", w.Code)
	}
	env := decodePetsEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d", env.Code, apperror.ErrServiceBusy)
	}
	if svc.calls != 1 {
		t.Errorf("svc.calls = %d, want 1", svc.calls)
	}
}

// case 5（可选 ≥4 要求外）— state=0 非法 → 1002（边界值 + 指针 zero-value-vs-missing 区分）
// POST {"state": 0} → 1002 envelope + message "state 必须是 1 / 2 / 3"
// 与 case 2 "字段缺失 → 1002 'state 必填'" 区分（handler 区分能力来自指针类型 + 顺序校验）。
func TestPetsHandler_PostStateSync_StateZero_Returns1002WithRangeMessage(t *testing.T) {
	uid := uint64(10)
	svc := &stubPetService{
		syncCurrentStateFn: func(ctx context.Context, in service.SyncCurrentStateInput) (*service.SyncCurrentStateOutput, error) {
			t.Errorf("service should NOT be called when state=0 (out of [1,3] range)")
			return nil, nil
		},
	}
	r := newPetsHandlerRouter(svc, &uid)

	body := `{"state":0}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pets/current/state-sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	env := decodePetsEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (1002)", env.Code, apperror.ErrInvalidParam)
	}
	// 与"字段缺失"区分：缺失返 "state 必填"，0 返 "state 必须是 1 / 2 / 3"
	if !strings.Contains(env.Message, "1 / 2 / 3") {
		t.Errorf("envelope.message = %q, want 包含 '1 / 2 / 3'（显式 0 走范围校验路径，不是 nil 缺失路径）", env.Message)
	}
	if strings.Contains(env.Message, "必填") {
		t.Errorf("envelope.message = %q, want NOT 包含 '必填'（显式 0 不是字段缺失）", env.Message)
	}
}

// case 6（额外）— missing userID in context → 1009 unreachable bug 兜底
// mockUserID = nil → 不挂 mock auth middleware → handler 走 unreachable bug 兜底 → 1009。
func TestPetsHandler_PostStateSync_MissingUserIDInContext_Returns1009(t *testing.T) {
	svc := &stubPetService{
		syncCurrentStateFn: func(ctx context.Context, in service.SyncCurrentStateInput) (*service.SyncCurrentStateOutput, error) {
			t.Errorf("service should NOT be called when userID missing")
			return nil, nil
		},
	}
	r := newPetsHandlerRouter(svc, nil)

	body := `{"state":2}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/pets/current/state-sync", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	env := decodePetsEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d", env.Code, apperror.ErrServiceBusy)
	}
}
