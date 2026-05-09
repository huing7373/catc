package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	stderrors "errors"
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
// stub HomeService（与 4.6 stubAuthService 同模式）
// ============================================================

type stubHomeService struct {
	loadHomeFn func(ctx context.Context, userID uint64) (*service.HomeOutput, error)
}

func (s *stubHomeService) LoadHome(ctx context.Context, userID uint64) (*service.HomeOutput, error) {
	return s.loadHomeFn(ctx, userID)
}

// newHomeHandlerRouter 构造 handler test router。
//
// 必挂中间件：ErrorMappingMiddleware（否则 c.Error 不写 envelope，断不到 envelope.code）。
// 可选挂：mock auth middleware（直接 c.Set UserIDKey 给定 uint64 值），避免引入
// 真实 4.4 signer / 4.5 Auth 联动。
//
// 不挂 mock auth middleware 的场景：测 unreachable userID 缺失分支。
func newHomeHandlerRouter(svc service.HomeService, mockUserID *uint64) *gin.Engine {
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
	h := handler.NewHomeHandler(svc)
	r.GET("/api/v1/home", h.LoadHome)
	return r
}

func decodeHomeEnvelope(t *testing.T, body []byte) response.Envelope {
	t.Helper()
	var env response.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("invalid JSON envelope: %v; body=%s", err, string(body))
	}
	return env
}

// ============================================================
// 测试 case
// ============================================================

// AC5.1 Happy path（节点 2 阶段首登）→ 200 + envelope.code=0 + 完整 V1 §5.1 schema
//
// 验证：
//   - user.id="1001"（string，BIGINT 转字符串）
//   - pet.id="2001" / petType=1 / name=默认小猫 / currentState=1 / equips=[]
//   - stepAccount 全 0
//   - chest.id="5001" / status=1 / unlockAt=ISO8601 UTC / openCostSteps=1000 / remainingSeconds≈600
//   - room.currentRoomId=null
func TestHomeHandler_HappyPath_FirstLogin_ReturnsCompleteSchema(t *testing.T) {
	unlockAt := time.Date(2026, 4, 23, 10, 20, 0, 0, time.UTC)
	uid := uint64(1001)

	svc := &stubHomeService{
		loadHomeFn: func(ctx context.Context, userID uint64) (*service.HomeOutput, error) {
			if userID != 1001 {
				t.Errorf("svc.LoadHome userID = %d, want 1001", userID)
			}
			return &service.HomeOutput{
				User: service.UserBrief{ID: 1001, Nickname: "用户1001", AvatarURL: ""},
				Pet: &service.PetBrief{
					ID: 2001, PetType: 1, Name: "默认小猫", CurrentState: 1,
				},
				StepAccount: service.StepAccountBrief{TotalSteps: 0, AvailableSteps: 0, ConsumedSteps: 0},
				Chest: service.ChestBrief{
					ID: 5001, Status: 1, UnlockAt: unlockAt,
					OpenCostSteps: 1000, RemainingSeconds: 600,
				},
			}, nil
		},
	}
	r := newHomeHandlerRouter(svc, &uid)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/home", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeHomeEnvelope(t, w.Body.Bytes())
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

	// user 段
	user, ok := data["user"].(map[string]any)
	if !ok {
		t.Fatalf("data.user not object: %T", data["user"])
	}
	if user["id"] != "1001" {
		t.Errorf("user.id = %v, want \"1001\" (string)", user["id"])
	}
	if user["nickname"] != "用户1001" {
		t.Errorf("user.nickname = %v, want 用户1001", user["nickname"])
	}
	if user["avatarUrl"] != "" {
		t.Errorf("user.avatarUrl = %v, want empty string", user["avatarUrl"])
	}

	// pet 段
	pet, ok := data["pet"].(map[string]any)
	if !ok {
		t.Fatalf("data.pet not object: %T", data["pet"])
	}
	if pet["id"] != "2001" {
		t.Errorf("pet.id = %v, want \"2001\" (string)", pet["id"])
	}
	if pt, _ := pet["petType"].(float64); pt != 1 {
		t.Errorf("pet.petType = %v, want 1 (number)", pet["petType"])
	}
	if pet["name"] != "默认小猫" {
		t.Errorf("pet.name = %v, want 默认小猫", pet["name"])
	}
	if cs, _ := pet["currentState"].(float64); cs != 1 {
		t.Errorf("pet.currentState = %v, want 1", pet["currentState"])
	}
	// equips：节点 2 阶段强制 []，**不**是 nil
	equips, ok := pet["equips"].([]any)
	if !ok {
		t.Fatalf("pet.equips not array: %T (must be [], not nil)", pet["equips"])
	}
	if len(equips) != 0 {
		t.Errorf("pet.equips len = %d, want 0", len(equips))
	}
	// 字面量验证：必须含 "equips":[] 而非 "equips":null
	if !bytes.Contains(w.Body.Bytes(), []byte(`"equips":[]`)) {
		t.Errorf(`body 未含 "equips":[] 字面量；body=%s`, w.Body.String())
	}

	// stepAccount 段
	step, ok := data["stepAccount"].(map[string]any)
	if !ok {
		t.Fatalf("data.stepAccount not object: %T", data["stepAccount"])
	}
	if ts, _ := step["totalSteps"].(float64); ts != 0 {
		t.Errorf("stepAccount.totalSteps = %v, want 0", step["totalSteps"])
	}
	if as, _ := step["availableSteps"].(float64); as != 0 {
		t.Errorf("stepAccount.availableSteps = %v, want 0", step["availableSteps"])
	}
	if cs, _ := step["consumedSteps"].(float64); cs != 0 {
		t.Errorf("stepAccount.consumedSteps = %v, want 0", step["consumedSteps"])
	}

	// chest 段
	chest, ok := data["chest"].(map[string]any)
	if !ok {
		t.Fatalf("data.chest not object: %T", data["chest"])
	}
	if chest["id"] != "5001" {
		t.Errorf("chest.id = %v, want \"5001\" (string)", chest["id"])
	}
	if cstatus, _ := chest["status"].(float64); cstatus != 1 {
		t.Errorf("chest.status = %v, want 1 (counting)", chest["status"])
	}
	// unlockAt: RFC3339 UTC "2026-04-23T10:20:00Z"
	wantUnlock := "2026-04-23T10:20:00Z"
	if chest["unlockAt"] != wantUnlock {
		t.Errorf("chest.unlockAt = %v, want %q", chest["unlockAt"], wantUnlock)
	}
	if oc, _ := chest["openCostSteps"].(float64); oc != 1000 {
		t.Errorf("chest.openCostSteps = %v, want 1000", chest["openCostSteps"])
	}
	if rs, _ := chest["remainingSeconds"].(float64); rs != 600 {
		t.Errorf("chest.remainingSeconds = %v, want 600", chest["remainingSeconds"])
	}

	// room.currentRoomId 必须是 null（本 case stub 默认 RoomBrief{}.CurrentRoomID=nil
	// 即"用户不在任何房间"；wire 行为与 4.8 节点 2 阶段一致，但 service 层语义已不同：
	// 节点 2 阶段是 service 强制 nil；节点 4 阶段（Story 11.10 落地后）是 service 透传
	// user.CurrentRoomID nil。本 case 输入未设 CurrentRoomID 故走 nil 分支）
	room, ok := data["room"].(map[string]any)
	if !ok {
		t.Fatalf("data.room not object: %T", data["room"])
	}
	if room["currentRoomId"] != nil {
		t.Errorf("room.currentRoomId = %v, want nil (null)", room["currentRoomId"])
	}
	// 字面量验证 "currentRoomId":null
	if !bytes.Contains(w.Body.Bytes(), []byte(`"currentRoomId":null`)) {
		t.Errorf(`body 未含 "currentRoomId":null 字面量；body=%s`, w.Body.String())
	}
}

// ============================================================
// Story 11.10: GET /home wire 层 room.currentRoomId 真实数据
//
// 验证 handler.homeResponseDTO 在节点 4 阶段（11.10 落地后）的 wire 输出：
//   - out.Room.CurrentRoomID != nil → wire 写 strconv.FormatUint(...) 字符串
//   - out.Room.CurrentRoomID == nil → wire 写 JSON null（**不**是 ""）
// ============================================================

// AC11.10.4 wire: 用户在房间 → response.data.room.currentRoomId = "3001"（string，**不**是 number）
//
// 验证 handler 把 *uint64 → strconv.FormatUint 字符串化路径正确。
func TestHomeHandler_UserInRoom_CurrentRoomIDIsString(t *testing.T) {
	roomID := uint64(3001)
	uid := uint64(1)
	svc := &stubHomeService{
		loadHomeFn: func(ctx context.Context, userID uint64) (*service.HomeOutput, error) {
			return &service.HomeOutput{
				User:        service.UserBrief{ID: 1, Nickname: "u"},
				Pet:         &service.PetBrief{ID: 2, PetType: 1, Name: "p", CurrentState: 1},
				StepAccount: service.StepAccountBrief{},
				Chest: service.ChestBrief{
					ID: 5, Status: 1, UnlockAt: time.Now().UTC().Add(10 * time.Minute),
					OpenCostSteps: 1000, RemainingSeconds: 600,
				},
				Room: service.RoomBrief{CurrentRoomID: &roomID},
			}, nil
		},
	}
	r := newHomeHandlerRouter(svc, &uid)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/home", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeHomeEnvelope(t, w.Body.Bytes())
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("envelope.data not object: %T", env.Data)
	}
	room, ok := data["room"].(map[string]any)
	if !ok {
		t.Fatalf("data.room not object: %T", data["room"])
	}
	if room["currentRoomId"] != "3001" {
		t.Errorf("room.currentRoomId = %v (%T), want \"3001\" (string)", room["currentRoomId"], room["currentRoomId"])
	}
	// 字面量验证：必须含 "currentRoomId":"3001" 而非 "currentRoomId":3001（number）
	if !bytes.Contains(w.Body.Bytes(), []byte(`"currentRoomId":"3001"`)) {
		t.Errorf(`body 未含 "currentRoomId":"3001" 字面量；body=%s`, w.Body.String())
	}
}

// AC11.10.5 wire: 用户不在任何房间 → response.data.room.currentRoomId = null（**不**是 ""）
//
// 验证 handler 把 nil *uint64 → JSON null 路径正确（与节点 2 阶段 4.8 同字面量行为，
// 但 service 层语义不同：节点 2 是 service 强制 nil，节点 4 是 service 透传 user.CurrentRoomID nil）。
func TestHomeHandler_UserNotInAnyRoom_CurrentRoomIDIsNull(t *testing.T) {
	uid := uint64(1)
	svc := &stubHomeService{
		loadHomeFn: func(ctx context.Context, userID uint64) (*service.HomeOutput, error) {
			return &service.HomeOutput{
				User:        service.UserBrief{ID: 1, Nickname: "u"},
				Pet:         &service.PetBrief{ID: 2, PetType: 1, Name: "p", CurrentState: 1},
				StepAccount: service.StepAccountBrief{},
				Chest: service.ChestBrief{
					ID: 5, Status: 1, UnlockAt: time.Now().UTC().Add(10 * time.Minute),
					OpenCostSteps: 1000, RemainingSeconds: 600,
				},
				Room: service.RoomBrief{CurrentRoomID: nil},
			}, nil
		},
	}
	r := newHomeHandlerRouter(svc, &uid)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/home", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeHomeEnvelope(t, w.Body.Bytes())
	data := env.Data.(map[string]any)
	room := data["room"].(map[string]any)
	if room["currentRoomId"] != nil {
		t.Errorf("room.currentRoomId = %v, want nil (null)", room["currentRoomId"])
	}
	// 字面量验证：必须含 "currentRoomId":null
	if !bytes.Contains(w.Body.Bytes(), []byte(`"currentRoomId":null`)) {
		t.Errorf(`body 未含 "currentRoomId":null 字面量；body=%s`, w.Body.String())
	}
}

// AC5.2 chest unlockAt 已过 → status=2 (unlockable) / remainingSeconds=0
func TestHomeHandler_ChestUnlocked_StatusIs2_RemainingSecondsIs0(t *testing.T) {
	pastUnlock := time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)
	uid := uint64(1)

	svc := &stubHomeService{
		loadHomeFn: func(ctx context.Context, userID uint64) (*service.HomeOutput, error) {
			return &service.HomeOutput{
				User: service.UserBrief{ID: 1, Nickname: "u"},
				Pet:  &service.PetBrief{ID: 2, PetType: 1, Name: "p", CurrentState: 1},
				StepAccount: service.StepAccountBrief{},
				Chest: service.ChestBrief{
					ID: 3, Status: 2, UnlockAt: pastUnlock,
					OpenCostSteps: 1000, RemainingSeconds: 0,
				},
			}, nil
		},
	}
	r := newHomeHandlerRouter(svc, &uid)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/home", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeHomeEnvelope(t, w.Body.Bytes())
	data := env.Data.(map[string]any)
	chest := data["chest"].(map[string]any)
	if cstatus, _ := chest["status"].(float64); cstatus != 2 {
		t.Errorf("chest.status = %v, want 2 (unlockable)", chest["status"])
	}
	if rs, _ := chest["remainingSeconds"].(float64); rs != 0 {
		t.Errorf("chest.remainingSeconds = %v, want 0", chest["remainingSeconds"])
	}
}

// AC5.3 pet 为 nil（无默认 pet）→ "pet": null
//
// **关键**：用 bytes.Contains 验证 JSON 字面量含 `"pet":null` 而非 `"pet":{}`，
// 防 LLM 误返空对象（V1 §5.1 行 335 钦定 data.pet 可空 → null 而非 {}）。
func TestHomeHandler_NoDefaultPet_PetFieldIsNull(t *testing.T) {
	uid := uint64(1)
	svc := &stubHomeService{
		loadHomeFn: func(ctx context.Context, userID uint64) (*service.HomeOutput, error) {
			return &service.HomeOutput{
				User:        service.UserBrief{ID: 1, Nickname: "u"},
				Pet:         nil, // 关键：nil
				StepAccount: service.StepAccountBrief{},
				Chest: service.ChestBrief{
					ID: 3, Status: 1, UnlockAt: time.Now().UTC().Add(time.Hour),
					OpenCostSteps: 1000, RemainingSeconds: 3600,
				},
			}, nil
		},
	}
	r := newHomeHandlerRouter(svc, &uid)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/home", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	// 字面量断言：必须含 "pet":null（不是 "pet":{}）
	if !bytes.Contains(w.Body.Bytes(), []byte(`"pet":null`)) {
		t.Errorf(`body 不含 "pet":null 字面量；body=%s`, w.Body.String())
	}
	if bytes.Contains(w.Body.Bytes(), []byte(`"pet":{}`)) {
		t.Errorf(`body 含 "pet":{} 字面量（错误！应为 null）；body=%s`, w.Body.String())
	}

	// JSON decoded 验证
	env := decodeHomeEnvelope(t, w.Body.Bytes())
	data := env.Data.(map[string]any)
	if data["pet"] != nil {
		t.Errorf("data.pet = %v, want nil (null)", data["pet"])
	}
}

// AC5.4 service 返 1009 → handler 透传 → ErrorMappingMiddleware 写 envelope
func TestHomeHandler_ServiceError_Returns1009(t *testing.T) {
	uid := uint64(1)
	wantCause := stderrors.New("simulated DB outage")
	svc := &stubHomeService{
		loadHomeFn: func(ctx context.Context, userID uint64) (*service.HomeOutput, error) {
			return nil, apperror.Wrap(wantCause, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		},
	}
	r := newHomeHandlerRouter(svc, &uid)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/home", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 1009 走 HTTP 500（ErrorMappingMiddleware 钦定）
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for 1009; body=%s", w.Code, w.Body.String())
	}
	env := decodeHomeEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d (ErrServiceBusy)", env.Code, apperror.ErrServiceBusy)
	}
	if !strings.Contains(env.Message, "服务繁忙") {
		t.Errorf("envelope.message = %q, want contains 服务繁忙", env.Message)
	}
}

// AC5.5 userID 缺失（不挂 mock auth middleware）→ 1009（unreachable 兜底）
//
// **关键**：handler 假设 Auth 中间件已注入 UserIDKey，但保险起见兜底；本 case
// 不挂 mock auth middleware 直接调，验证 1009 兜底分支。
func TestHomeHandler_NoUserIDInContext_Returns1009(t *testing.T) {
	svc := &stubHomeService{
		loadHomeFn: func(ctx context.Context, userID uint64) (*service.HomeOutput, error) {
			t.Errorf("service should NOT be called when userID missing")
			return nil, nil
		},
	}
	// mockUserID = nil → 不挂 mock auth middleware
	r := newHomeHandlerRouter(svc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/home", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
	env := decodeHomeEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d", env.Code, apperror.ErrServiceBusy)
	}
}
