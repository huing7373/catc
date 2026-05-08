package handler_test

import (
	"context"
	"encoding/json"
	stderrors "errors"
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

// stubRoomService 是 service.RoomService 的测试 stub；
// 通过 createRoomFn / joinRoomFn / leaveRoomFn 字段让每个 case 自定义返回。
type stubRoomService struct {
	createRoomFn func(ctx context.Context, in service.CreateRoomInput) (*service.CreateRoomOutput, error)
	joinRoomFn   func(ctx context.Context, in service.JoinRoomInput) (*service.JoinRoomOutput, error)
	leaveRoomFn  func(ctx context.Context, in service.LeaveRoomInput) (*service.LeaveRoomOutput, error)
}

func (s *stubRoomService) CreateRoom(ctx context.Context, in service.CreateRoomInput) (*service.CreateRoomOutput, error) {
	if s.createRoomFn == nil {
		panic("stubRoomService.CreateRoom not configured")
	}
	return s.createRoomFn(ctx, in)
}

func (s *stubRoomService) JoinRoom(ctx context.Context, in service.JoinRoomInput) (*service.JoinRoomOutput, error) {
	if s.joinRoomFn == nil {
		panic("stubRoomService.JoinRoom not configured")
	}
	return s.joinRoomFn(ctx, in)
}

func (s *stubRoomService) LeaveRoom(ctx context.Context, in service.LeaveRoomInput) (*service.LeaveRoomOutput, error) {
	if s.leaveRoomFn == nil {
		panic("stubRoomService.LeaveRoom not configured")
	}
	return s.leaveRoomFn(ctx, in)
}

// newRoomHandlerRouter 构造一个挂上 ErrorMappingMiddleware + RoomHandler 的 router。
//
// **关键**：必须挂 ErrorMappingMiddleware，否则 c.Error(...) 后 body 为空，断不到
// envelope.code（与 auth_handler_test 同模式）。
//
// **不挂真 auth middleware**：单测路径直接用一个内联 middleware 把 userID 注入
// c.Keys[middleware.UserIDKey] —— 让 handler.CreateRoom 能取到 userID 走业务路径。
// 真 auth middleware 由 router_test 等其他测试覆盖；本 case 只测 handler 单元行为。
func newRoomHandlerRouter(svc service.RoomService, userID uint64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorMappingMiddleware())
	if userID != 0 {
		r.Use(func(c *gin.Context) {
			c.Set(middleware.UserIDKey, userID)
			c.Next()
		})
	}
	roomHandler := handler.NewRoomHandler(svc)
	r.POST("/api/v1/rooms", roomHandler.CreateRoom)
	// Story 11.4 加：POST /api/v1/rooms/:roomId/join
	r.POST("/api/v1/rooms/:roomId/join", roomHandler.JoinRoom)
	// Story 11.5 加：POST /api/v1/rooms/:roomId/leave
	r.POST("/api/v1/rooms/:roomId/leave", roomHandler.LeaveRoom)
	return r
}

func decodeRoomEnvelope(t *testing.T, body []byte) response.Envelope {
	t.Helper()
	var env response.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("invalid JSON envelope: %v; body=%s", err, string(body))
	}
	return env
}

// TestRoomHandler_CreateRoom_Success_ReturnsZeroWithRoomData (AC10 case 1):
// stub service 返 CreateRoomOutput → handler 翻译为 V1 §10.1 钦定 wire DTO。
//
// **关键 assert**：
//   - HTTP status = 200（V1 §2.4 钦定业务码与 HTTP status 正交，0 走 200）
//   - envelope.code = 0
//   - data.room.id / data.room.creatorUserId 都是 string（V1 §2.5 BIGINT 字符串化）
//   - data.room.maxMembers / memberCount / status 都是 number（数值字段不字符串化）
func TestRoomHandler_CreateRoom_Success_ReturnsZeroWithRoomData(t *testing.T) {
	svc := &stubRoomService{
		createRoomFn: func(ctx context.Context, in service.CreateRoomInput) (*service.CreateRoomOutput, error) {
			if in.UserID != 1001 {
				t.Errorf("svc.UserID = %d, want 1001", in.UserID)
			}
			return &service.CreateRoomOutput{
				RoomID:        3001,
				CreatorUserID: 1001,
				MaxMembers:    4,
				MemberCount:   1,
				Status:        1,
			}, nil
		},
	}
	r := newRoomHandlerRouter(svc, 1001)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
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
	room, ok := data["room"].(map[string]any)
	if !ok {
		t.Fatalf("data.room not object: %T", data["room"])
	}
	// V1 §2.5 BIGINT id 是 string
	if room["id"] != "3001" {
		t.Errorf("room.id = %v, want \"3001\" (string)", room["id"])
	}
	if room["creatorUserId"] != "1001" {
		t.Errorf("room.creatorUserId = %v, want \"1001\" (string)", room["creatorUserId"])
	}
	// JSON number → float64
	if maxMembers, _ := room["maxMembers"].(float64); maxMembers != 4 {
		t.Errorf("room.maxMembers = %v, want 4 (number)", room["maxMembers"])
	}
	if memberCount, _ := room["memberCount"].(float64); memberCount != 1 {
		t.Errorf("room.memberCount = %v, want 1 (number)", room["memberCount"])
	}
	if status, _ := room["status"].(float64); status != 1 {
		t.Errorf("room.status = %v, want 1 (number)", room["status"])
	}
}

// TestRoomHandler_CreateRoom_UserAlreadyInRoom_Returns6003 (AC10 case 2):
// service 返 *AppError{Code:6003} → handler 透传 → envelope.code=6003。
func TestRoomHandler_CreateRoom_UserAlreadyInRoom_Returns6003(t *testing.T) {
	svc := &stubRoomService{
		createRoomFn: func(ctx context.Context, in service.CreateRoomInput) (*service.CreateRoomOutput, error) {
			return nil, apperror.New(apperror.ErrUserAlreadyInRoom, apperror.DefaultMessages[apperror.ErrUserAlreadyInRoom])
		},
	}
	r := newRoomHandlerRouter(svc, 1001)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 6003 走 HTTP 200（V1 §2.4 钦定业务码与 HTTP status 正交；6xxx 业务码不映射 5xx）
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 6003; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrUserAlreadyInRoom {
		t.Errorf("envelope.code = %d, want %d (ErrUserAlreadyInRoom 6003)", env.Code, apperror.ErrUserAlreadyInRoom)
	}
}

// TestRoomHandler_CreateRoom_ServiceError_Returns1009 (AC10 case 3):
// service 返 *AppError{Code:1009} → envelope.code=1009 + HTTP 500（ErrorMappingMiddleware 钦定）。
func TestRoomHandler_CreateRoom_ServiceError_Returns1009(t *testing.T) {
	wantCause := stderrors.New("simulated DB outage")
	svc := &stubRoomService{
		createRoomFn: func(ctx context.Context, in service.CreateRoomInput) (*service.CreateRoomOutput, error) {
			return nil, apperror.Wrap(wantCause, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		},
	}
	r := newRoomHandlerRouter(svc, 1001)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 1009 走 HTTP 500（ErrorMappingMiddleware 钦定）
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for 1009; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d (ErrServiceBusy 1009)", env.Code, apperror.ErrServiceBusy)
	}
}

// TestRoomHandler_CreateRoom_NoUserIDInContext_Returns1009 (AC10 case 4):
// 模拟 c.Keys 没注入 userID（理论不应发生，因为 auth middleware 已挂在前；本兜底
// 校验 handler 的"unreachable"防御性分支）→ envelope.code=1009。
func TestRoomHandler_CreateRoom_NoUserIDInContext_Returns1009(t *testing.T) {
	svc := &stubRoomService{
		createRoomFn: func(ctx context.Context, in service.CreateRoomInput) (*service.CreateRoomOutput, error) {
			t.Errorf("svc.CreateRoom 不应被调用（c.Keys 缺 userID 已被 handler 兜底拦截）")
			return nil, nil
		},
	}
	// userID == 0 → newRoomHandlerRouter 不挂 userID 注入 middleware
	r := newRoomHandlerRouter(svc, 0)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d (ErrServiceBusy 1009)", env.Code, apperror.ErrServiceBusy)
	}
}

// ============================================================
// Story 11.4 单测 case：JoinRoom handler（≥6 case）
// ============================================================

// TestRoomHandler_JoinRoom_Success_Returns0WithJoined:
// stub service 返 JoinRoomOutput → handler 翻译为 V1 §10.4 钦定 wire DTO。
//
// **关键 assert**：
//   - HTTP status = 200（V1 §2.4 钦定）
//   - envelope.code = 0
//   - data.roomId 是 string（V1 §2.5 BIGINT 字符串化）
//   - data.joined 是 bool 且为 true
func TestRoomHandler_JoinRoom_Success_Returns0WithJoined(t *testing.T) {
	svc := &stubRoomService{
		joinRoomFn: func(ctx context.Context, in service.JoinRoomInput) (*service.JoinRoomOutput, error) {
			if in.UserID != 1002 {
				t.Errorf("svc.UserID = %d, want 1002", in.UserID)
			}
			if in.RoomID != 3001 {
				t.Errorf("svc.RoomID = %d, want 3001", in.RoomID)
			}
			return &service.JoinRoomOutput{RoomID: 3001, Joined: true}, nil
		},
	}
	r := newRoomHandlerRouter(svc, 1002)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/3001/join", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != 0 {
		t.Errorf("envelope.code = %d, want 0", env.Code)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("envelope.data not object: %T", env.Data)
	}
	// V1 §2.5 BIGINT id 是 string
	if data["roomId"] != "3001" {
		t.Errorf("data.roomId = %v, want \"3001\" (string)", data["roomId"])
	}
	// joined 是 bool（JSON true → bool true）
	joined, ok := data["joined"].(bool)
	if !ok {
		t.Errorf("data.joined not bool: %T", data["joined"])
	}
	if !joined {
		t.Errorf("data.joined = false, want true")
	}
}

// TestRoomHandler_JoinRoom_UserAlreadyInRoom_Returns6003:
// service 返 *AppError{Code:6003} → handler 透传 → envelope.code=6003。
func TestRoomHandler_JoinRoom_UserAlreadyInRoom_Returns6003(t *testing.T) {
	svc := &stubRoomService{
		joinRoomFn: func(ctx context.Context, in service.JoinRoomInput) (*service.JoinRoomOutput, error) {
			return nil, apperror.New(apperror.ErrUserAlreadyInRoom, apperror.DefaultMessages[apperror.ErrUserAlreadyInRoom])
		},
	}
	r := newRoomHandlerRouter(svc, 1002)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/3001/join", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 6003; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrUserAlreadyInRoom {
		t.Errorf("envelope.code = %d, want %d (ErrUserAlreadyInRoom 6003)", env.Code, apperror.ErrUserAlreadyInRoom)
	}
}

// TestRoomHandler_JoinRoom_RoomNotFound_Returns6001:
// service 返 *AppError{Code:6001} → envelope.code=6001。
func TestRoomHandler_JoinRoom_RoomNotFound_Returns6001(t *testing.T) {
	svc := &stubRoomService{
		joinRoomFn: func(ctx context.Context, in service.JoinRoomInput) (*service.JoinRoomOutput, error) {
			return nil, apperror.New(apperror.ErrRoomNotFound, apperror.DefaultMessages[apperror.ErrRoomNotFound])
		},
	}
	r := newRoomHandlerRouter(svc, 1002)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/9999/join", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 6001; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrRoomNotFound {
		t.Errorf("envelope.code = %d, want %d (ErrRoomNotFound 6001)", env.Code, apperror.ErrRoomNotFound)
	}
}

// TestRoomHandler_JoinRoom_RoomFull_Returns6002:
// service 返 *AppError{Code:6002} → envelope.code=6002。
func TestRoomHandler_JoinRoom_RoomFull_Returns6002(t *testing.T) {
	svc := &stubRoomService{
		joinRoomFn: func(ctx context.Context, in service.JoinRoomInput) (*service.JoinRoomOutput, error) {
			return nil, apperror.New(apperror.ErrRoomFull, apperror.DefaultMessages[apperror.ErrRoomFull])
		},
	}
	r := newRoomHandlerRouter(svc, 1002)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/3001/join", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 6002; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrRoomFull {
		t.Errorf("envelope.code = %d, want %d (ErrRoomFull 6002)", env.Code, apperror.ErrRoomFull)
	}
}

// TestRoomHandler_JoinRoom_RoomClosed_Returns6005:
// service 返 *AppError{Code:6005} → envelope.code=6005。
func TestRoomHandler_JoinRoom_RoomClosed_Returns6005(t *testing.T) {
	svc := &stubRoomService{
		joinRoomFn: func(ctx context.Context, in service.JoinRoomInput) (*service.JoinRoomOutput, error) {
			return nil, apperror.New(apperror.ErrRoomInvalidState, apperror.DefaultMessages[apperror.ErrRoomInvalidState])
		},
	}
	r := newRoomHandlerRouter(svc, 1002)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/3001/join", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 6005; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrRoomInvalidState {
		t.Errorf("envelope.code = %d, want %d (ErrRoomInvalidState 6005)", env.Code, apperror.ErrRoomInvalidState)
	}
}

// TestRoomHandler_JoinRoom_ServiceBusy_Returns1009:
// service 返 *AppError{Code:1009} → envelope.code=1009 + HTTP 500。
func TestRoomHandler_JoinRoom_ServiceBusy_Returns1009(t *testing.T) {
	wantCause := stderrors.New("simulated DB outage")
	svc := &stubRoomService{
		joinRoomFn: func(ctx context.Context, in service.JoinRoomInput) (*service.JoinRoomOutput, error) {
			return nil, apperror.Wrap(wantCause, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		},
	}
	r := newRoomHandlerRouter(svc, 1002)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/3001/join", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for 1009; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d", env.Code, apperror.ErrServiceBusy)
	}
}

// TestRoomHandler_JoinRoom_InvalidRoomIDPath_Returns1002:
// path 含非数字 roomId（如 "abc"）→ handler ParseUint 失败 → envelope.code=1002。
// service.JoinRoom **未**被调用。
func TestRoomHandler_JoinRoom_InvalidRoomIDPath_Returns1002(t *testing.T) {
	svc := &stubRoomService{
		joinRoomFn: func(ctx context.Context, in service.JoinRoomInput) (*service.JoinRoomOutput, error) {
			t.Errorf("svc.JoinRoom 不应被调用（path 校验已失败）")
			return nil, nil
		},
	}
	r := newRoomHandlerRouter(svc, 1002)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/abc/join", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 1002; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (ErrInvalidParam 1002)", env.Code, apperror.ErrInvalidParam)
	}
}

// TestRoomHandler_JoinRoom_RoomIDTooLong_Returns1002:
// path 长度 > 20 → handler length 校验失败 → envelope.code=1002。
func TestRoomHandler_JoinRoom_RoomIDTooLong_Returns1002(t *testing.T) {
	svc := &stubRoomService{
		joinRoomFn: func(ctx context.Context, in service.JoinRoomInput) (*service.JoinRoomOutput, error) {
			t.Errorf("svc.JoinRoom 不应被调用（length 校验已失败）")
			return nil, nil
		},
	}
	r := newRoomHandlerRouter(svc, 1002)

	// 21 位数字（V1 §10.4 限 1 ≤ length ≤ 20）
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/123456789012345678901/join", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 1002; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (ErrInvalidParam 1002)", env.Code, apperror.ErrInvalidParam)
	}
}

// TestRoomHandler_JoinRoom_RoomIDZero_Returns1002:
// path = "0" → ParseUint 成功但 roomID == 0 → 防御性返 1002（业务上 ID 必为正）。
func TestRoomHandler_JoinRoom_RoomIDZero_Returns1002(t *testing.T) {
	svc := &stubRoomService{
		joinRoomFn: func(ctx context.Context, in service.JoinRoomInput) (*service.JoinRoomOutput, error) {
			t.Errorf("svc.JoinRoom 不应被调用（roomID > 0 校验已失败）")
			return nil, nil
		},
	}
	r := newRoomHandlerRouter(svc, 1002)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/0/join", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 1002; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (ErrInvalidParam 1002, roomID > 0 校验)", env.Code, apperror.ErrInvalidParam)
	}
}

// TestRoomHandler_JoinRoom_NoUserIDInContext_Returns1009:
// 模拟 c.Keys 缺 userID（理论 unreachable）→ envelope.code=1009。
func TestRoomHandler_JoinRoom_NoUserIDInContext_Returns1009(t *testing.T) {
	svc := &stubRoomService{
		joinRoomFn: func(ctx context.Context, in service.JoinRoomInput) (*service.JoinRoomOutput, error) {
			t.Errorf("svc.JoinRoom 不应被调用（c.Keys 缺 userID 已被 handler 兜底拦截）")
			return nil, nil
		},
	}
	r := newRoomHandlerRouter(svc, 0)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/3001/join", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d", env.Code, apperror.ErrServiceBusy)
	}
}

// ============================================================
// Story 11.5 单测 case：LeaveRoom handler（≥6 case）
// ============================================================

// TestRoomHandler_LeaveRoom_Success_Returns0WithLeft:
// stub service 返 LeaveRoomOutput → handler 翻译为 V1 §10.5 钦定 wire DTO。
//
// **关键 assert**：
//   - HTTP status = 200
//   - envelope.code = 0
//   - data.roomId 是 string（V1 §2.5 BIGINT 字符串化）
//   - data.left 是 bool 且为 true
func TestRoomHandler_LeaveRoom_Success_Returns0WithLeft(t *testing.T) {
	svc := &stubRoomService{
		leaveRoomFn: func(ctx context.Context, in service.LeaveRoomInput) (*service.LeaveRoomOutput, error) {
			if in.UserID != 1001 {
				t.Errorf("svc.UserID = %d, want 1001", in.UserID)
			}
			if in.RoomID != 3001 {
				t.Errorf("svc.RoomID = %d, want 3001", in.RoomID)
			}
			return &service.LeaveRoomOutput{RoomID: 3001, Left: true}, nil
		},
	}
	r := newRoomHandlerRouter(svc, 1001)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/3001/leave", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != 0 {
		t.Errorf("envelope.code = %d, want 0", env.Code)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("envelope.data not object: %T", env.Data)
	}
	if data["roomId"] != "3001" {
		t.Errorf("data.roomId = %v, want \"3001\" (string)", data["roomId"])
	}
	left, ok := data["left"].(bool)
	if !ok {
		t.Errorf("data.left not bool: %T", data["left"])
	}
	if !left {
		t.Errorf("data.left = false, want true")
	}
}

// TestRoomHandler_LeaveRoom_UserNotInRoom_Returns6004:
// service 返 *AppError{Code:6004} → handler 透传 → envelope.code=6004。
func TestRoomHandler_LeaveRoom_UserNotInRoom_Returns6004(t *testing.T) {
	svc := &stubRoomService{
		leaveRoomFn: func(ctx context.Context, in service.LeaveRoomInput) (*service.LeaveRoomOutput, error) {
			return nil, apperror.New(apperror.ErrUserNotInRoom, apperror.DefaultMessages[apperror.ErrUserNotInRoom])
		},
	}
	r := newRoomHandlerRouter(svc, 1001)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/3001/leave", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 6004; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrUserNotInRoom {
		t.Errorf("envelope.code = %d, want %d (ErrUserNotInRoom 6004)", env.Code, apperror.ErrUserNotInRoom)
	}
}

// TestRoomHandler_LeaveRoom_ServiceBusy_Returns1009:
// service 返 *AppError{Code:1009} → envelope.code=1009 + HTTP 500。
func TestRoomHandler_LeaveRoom_ServiceBusy_Returns1009(t *testing.T) {
	wantCause := stderrors.New("simulated DB outage")
	svc := &stubRoomService{
		leaveRoomFn: func(ctx context.Context, in service.LeaveRoomInput) (*service.LeaveRoomOutput, error) {
			return nil, apperror.Wrap(wantCause, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		},
	}
	r := newRoomHandlerRouter(svc, 1001)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/3001/leave", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 for 1009; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d", env.Code, apperror.ErrServiceBusy)
	}
}

// TestRoomHandler_LeaveRoom_InvalidRoomIDPath_Returns1002:
// path 含非数字 roomId → handler ParseUint 失败 → envelope.code=1002 + service 未调。
func TestRoomHandler_LeaveRoom_InvalidRoomIDPath_Returns1002(t *testing.T) {
	svc := &stubRoomService{
		leaveRoomFn: func(ctx context.Context, in service.LeaveRoomInput) (*service.LeaveRoomOutput, error) {
			t.Errorf("svc.LeaveRoom 不应被调用（path 校验已失败）")
			return nil, nil
		},
	}
	r := newRoomHandlerRouter(svc, 1001)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/abc/leave", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 1002; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (ErrInvalidParam 1002)", env.Code, apperror.ErrInvalidParam)
	}
}

// TestRoomHandler_LeaveRoom_RoomIDTooLong_Returns1002:
// path 长度 > 20 → handler length 校验失败 → envelope.code=1002。
func TestRoomHandler_LeaveRoom_RoomIDTooLong_Returns1002(t *testing.T) {
	svc := &stubRoomService{
		leaveRoomFn: func(ctx context.Context, in service.LeaveRoomInput) (*service.LeaveRoomOutput, error) {
			t.Errorf("svc.LeaveRoom 不应被调用（length 校验已失败）")
			return nil, nil
		},
	}
	r := newRoomHandlerRouter(svc, 1001)

	// 21 位数字
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/123456789012345678901/leave", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 1002; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (ErrInvalidParam 1002)", env.Code, apperror.ErrInvalidParam)
	}
}

// TestRoomHandler_LeaveRoom_RoomIDZero_Returns1002:
// path = "0" → ParseUint 成功但 roomID == 0 → 防御性返 1002（业务上 ID 必为正）。
func TestRoomHandler_LeaveRoom_RoomIDZero_Returns1002(t *testing.T) {
	svc := &stubRoomService{
		leaveRoomFn: func(ctx context.Context, in service.LeaveRoomInput) (*service.LeaveRoomOutput, error) {
			t.Errorf("svc.LeaveRoom 不应被调用（roomID > 0 校验已失败）")
			return nil, nil
		},
	}
	r := newRoomHandlerRouter(svc, 1001)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/0/leave", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for 1002; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d", env.Code, apperror.ErrInvalidParam)
	}
}

// TestRoomHandler_LeaveRoom_NoUserIDInContext_Returns1009:
// 模拟 c.Keys 缺 userID → envelope.code=1009。
func TestRoomHandler_LeaveRoom_NoUserIDInContext_Returns1009(t *testing.T) {
	svc := &stubRoomService{
		leaveRoomFn: func(ctx context.Context, in service.LeaveRoomInput) (*service.LeaveRoomOutput, error) {
			t.Errorf("svc.LeaveRoom 不应被调用（c.Keys 缺 userID 已被 handler 兜底拦截）")
			return nil, nil
		},
	}
	r := newRoomHandlerRouter(svc, 0)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/3001/leave", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", w.Code, w.Body.String())
	}
	env := decodeRoomEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d", env.Code, apperror.ErrServiceBusy)
	}
}
