package ws_test

// emoji_handler_test.go：Story 17.5 EmojiHandler.HandleEmojiSend 单测（≥7 case）。
//
// 测试策略（black-box `package ws_test`）：
//   - 复用 snapshot_test.go 既有的 newPipeWSConnPair helper 构造配对的 server /
//     client *websocket.Conn；server-side conn 包装为真实 *wsapp.Session（通过
//     wsapp.NewSessionForTest export helper，内部自动启动 writeLoop 让 SendPriority
//     的 error envelope 真的写到 wire）
//   - stubEmojiSvcForHandler / stubUserRepoForHandler 走 fn 字段注入每个 case 的
//     行为（与 stubEmojiRepo / stubUserRepo 同模式）
//   - captureBroadcastFn 用 atomic.Int32 + slice 记录 broadcastFn 调用次数 + 入参
//   - error case 验证 broadcastFn 调 0 次 + client 收到正确 error envelope + requestId 回带
//   - happy case 验证 broadcastFn 调 1 次 + roomID + envelope 字段值
//   - BroadcastFails case 验证 fire-and-forget（client 收不到 error frame）
//
// 与 14.4 既有 pet_service_test broadcast 单测同模式（mock broadcast / stub repo /
// 真实 envelope 解析）。

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	wsapp "github.com/huing/cat/server/internal/app/ws"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// ---------- stubs ----------

// stubEmojiSvcForHandler 让每个 case 自定义 ValidateCode 返回。
type stubEmojiSvcForHandler struct {
	validateFn func(ctx context.Context, code string) error
}

func (s *stubEmojiSvcForHandler) ValidateCode(ctx context.Context, code string) error {
	return s.validateFn(ctx, code)
}

// stubUserRepoForHandler 让每个 case 自定义 FindByID 返回；其他 UserRepo 方法
// 占位返 nil（emoji_handler 只调 FindByID）。
type stubUserRepoForHandler struct {
	findByIDFn func(ctx context.Context, id uint64) (*mysql.User, error)
}

func (s *stubUserRepoForHandler) Create(_ context.Context, _ *mysql.User) error { return nil }
func (s *stubUserRepoForHandler) UpdateNickname(_ context.Context, _ uint64, _ string) error {
	return nil
}
func (s *stubUserRepoForHandler) FindByID(ctx context.Context, id uint64) (*mysql.User, error) {
	return s.findByIDFn(ctx, id)
}
func (s *stubUserRepoForHandler) UpdateCurrentRoomID(_ context.Context, _ uint64, _ *uint64) error {
	return nil
}

// captureBroadcastFn 用 atomic.Int32 + slice 记录 broadcastFn 调用次数 + 入参。
type captureBroadcastFn struct {
	callCount  atomic.Int32
	mu         sync.Mutex
	calls      []broadcastCall
	returnSent int
	returnErr  error
}

type broadcastCall struct {
	roomID uint64
	msg    []byte
}

func (c *captureBroadcastFn) fn() wsapp.BroadcastFn {
	return func(ctx context.Context, roomID uint64, msg []byte) (int, error) {
		c.callCount.Add(1)
		c.mu.Lock()
		c.calls = append(c.calls, broadcastCall{roomID: roomID, msg: append([]byte(nil), msg...)})
		c.mu.Unlock()
		return c.returnSent, c.returnErr
	}
}

// ---------- 共用 fixture ----------

// emojiHandlerFixture 把 handler test 的所有组件打包，便于每个 case 一行获取。
type emojiHandlerFixture struct {
	handler    wsapp.EmojiHandler
	session    *wsapp.Session
	clientConn *websocket.Conn
	stubSvc    *stubEmojiSvcForHandler
	stubUser   *stubUserRepoForHandler
	capture    *captureBroadcastFn
	cleanup    func()
}

// buildEmojiHandlerFixture 用 newPipeWSConnPair 起一对配对的 *websocket.Conn，把
// server-side conn 包装为 *wsapp.Session（writeLoop 已自动启动），构造 handler +
// stub svc + stub userRepo + capture broadcastFn。
//
// userID / roomID 由 caller 传入（handler 通过 session.UserID() / RoomID() 读）。
func buildEmojiHandlerFixture(t *testing.T, userID, roomID uint64) *emojiHandlerFixture {
	t.Helper()

	stubSvc := &stubEmojiSvcForHandler{}
	stubUser := &stubUserRepoForHandler{}
	capture := &captureBroadcastFn{returnSent: 2, returnErr: nil}

	h := wsapp.NewEmojiHandler(stubSvc, stubUser, capture.fn())

	serverConn, clientConn, connCleanup := newPipeWSConnPair(t)
	// 注：NewSessionForTest 内部启动 writeLoop（让 SendPriority 入队的 msg 真的
	// 写到 conn）；不启动 readLoop（test 是同步调 HandleEmojiSend 直传 envelope）。
	session := wsapp.NewSessionForTest(t, "test-session-1", userID, roomID, serverConn, h)

	cleanup := func() {
		_ = session.Close() // 关 sendChan + 让 writeLoop 退出
		connCleanup()
	}

	return &emojiHandlerFixture{
		handler:    h,
		session:    session,
		clientConn: clientConn,
		stubSvc:    stubSvc,
		stubUser:   stubUser,
		capture:    capture,
		cleanup:    cleanup,
	}
}

// readErrorEnvelope 从 client conn 读一帧并解析为 error envelope；超时时 t.Fatal。
func readErrorEnvelope(t *testing.T, clientConn *websocket.Conn, timeout time.Duration) (requestID string, payload wsapp.ErrorPayload) {
	t.Helper()
	_ = clientConn.SetReadDeadline(time.Now().Add(timeout))
	_, raw, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	var got struct {
		Type      string             `json:"type"`
		RequestID string             `json:"requestId"`
		Payload   wsapp.ErrorPayload `json:"payload"`
		Ts        int64              `json:"ts"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal error envelope: %v (raw=%s)", err, string(raw))
	}
	if got.Type != "error" {
		t.Errorf("envelope.type = %q, want \"error\" (raw=%s)", got.Type, string(raw))
	}
	if got.Ts <= 0 {
		t.Errorf("envelope.ts = %d, want > 0", got.Ts)
	}
	return got.RequestID, got.Payload
}

// ---------- AC8.1 happy ----------

// TestEmojiHandler_HandleEmojiSend_Happy_Broadcasts:
// emojiCode 合法 + user 在正确房间 → broadcastFn 调 1 次 + envelope 字段正确 +
// client **不**收 error envelope。
func TestEmojiHandler_HandleEmojiSend_Happy_Broadcasts(t *testing.T) {
	f := buildEmojiHandlerFixture(t, 1001, 3001)
	defer f.cleanup()

	f.stubSvc.validateFn = func(_ context.Context, code string) error {
		if code != "wave" {
			t.Errorf("ValidateCode called with code=%q, want wave", code)
		}
		return nil
	}
	f.stubUser.findByIDFn = func(_ context.Context, id uint64) (*mysql.User, error) {
		if id != 1001 {
			t.Errorf("FindByID called with id=%d, want 1001", id)
		}
		roomID := uint64(3001)
		return &mysql.User{ID: 1001, CurrentRoomID: &roomID}, nil
	}

	env := wsapp.NewClientEnvelopeForTest("emoji.send", "msg_001", []byte(`{"emojiCode":"wave"}`))
	f.handler.HandleEmojiSend(context.Background(), f.session, env.AsInternal())

	if got := f.capture.callCount.Load(); got != 1 {
		t.Fatalf("broadcast count = %d, want 1", got)
	}
	f.capture.mu.Lock()
	call := f.capture.calls[0]
	f.capture.mu.Unlock()
	if call.roomID != 3001 {
		t.Errorf("broadcast roomId = %d, want 3001 (Session.RoomID())", call.roomID)
	}

	// 验证 envelope 字段（V1 §12.3 钦定）
	var got struct {
		Type      string                     `json:"type"`
		RequestID string                     `json:"requestId"`
		Payload   wsapp.EmojiReceivedPayload `json:"payload"`
		Ts        int64                      `json:"ts"`
	}
	if err := json.Unmarshal(call.msg, &got); err != nil {
		t.Fatalf("unmarshal broadcast msg: %v", err)
	}
	if got.Type != "emoji.received" {
		t.Errorf("type = %q, want emoji.received", got.Type)
	}
	if got.RequestID != "" {
		t.Errorf("requestId = %q, want \"\" (broadcast 类固定空; V1 §12.3 钦定)", got.RequestID)
	}
	if got.Payload.UserID != "1001" {
		t.Errorf("payload.userId = %q, want \"1001\" (BIGINT 字符串化, V1 §2.5)", got.Payload.UserID)
	}
	if got.Payload.EmojiCode != "wave" {
		t.Errorf("payload.emojiCode = %q, want \"wave\"", got.Payload.EmojiCode)
	}
	if got.Ts <= 0 {
		t.Errorf("ts = %d, want > 0", got.Ts)
	}
}

// ---------- AC8.2 payload missing emojiCode ----------

// TestEmojiHandler_HandleEmojiSend_PayloadMissingEmojiCode_Returns1002:
// payload 缺 emojiCode（JSON `{}`）→ Unmarshal 成功但 EmojiCode="" → svc.ValidateCode
// 返 1002 → 回 error envelope code=1002 + requestId 回带 + broadcast 0 次。
func TestEmojiHandler_HandleEmojiSend_PayloadMissingEmojiCode_Returns1002(t *testing.T) {
	f := buildEmojiHandlerFixture(t, 1001, 3001)
	defer f.cleanup()

	f.stubSvc.validateFn = func(_ context.Context, code string) error {
		if code != "" {
			t.Errorf("code = %q, want \"\" (payload 缺 emojiCode → 零值)", code)
		}
		return apperror.New(apperror.ErrInvalidParam,
			apperror.DefaultMessages[apperror.ErrInvalidParam])
	}
	f.stubUser.findByIDFn = func(_ context.Context, _ uint64) (*mysql.User, error) {
		t.Errorf("FindByID should not be called when ValidateCode fails")
		return nil, nil
	}

	env := wsapp.NewClientEnvelopeForTest("emoji.send", "msg_002", []byte(`{}`))
	f.handler.HandleEmojiSend(context.Background(), f.session, env.AsInternal())

	if got := f.capture.callCount.Load(); got != 0 {
		t.Errorf("broadcast count = %d, want 0", got)
	}

	requestID, payload := readErrorEnvelope(t, f.clientConn, 2*time.Second)
	if requestID != "msg_002" {
		t.Errorf("envelope.requestId = %q, want \"msg_002\" (回带 client requestId)", requestID)
	}
	if payload.Code != apperror.ErrInvalidParam {
		t.Errorf("payload.code = %d, want %d (1002 ErrInvalidParam)", payload.Code, apperror.ErrInvalidParam)
	}
}

// ---------- AC8.3 emojiCode not found ----------

// TestEmojiHandler_HandleEmojiSend_EmojiNotFound_Returns7001:
// emojiCode 字符集合法但 DB 不存在 → svc.ValidateCode 返 7001 → 回 error 7001
// + requestId 回带 + broadcast 0 次。
func TestEmojiHandler_HandleEmojiSend_EmojiNotFound_Returns7001(t *testing.T) {
	f := buildEmojiHandlerFixture(t, 1001, 3001)
	defer f.cleanup()

	f.stubSvc.validateFn = func(_ context.Context, _ string) error {
		return apperror.New(apperror.ErrEmojiNotFound,
			apperror.DefaultMessages[apperror.ErrEmojiNotFound])
	}
	f.stubUser.findByIDFn = func(_ context.Context, _ uint64) (*mysql.User, error) {
		t.Errorf("FindByID should not be called when emoji not found")
		return nil, nil
	}

	env := wsapp.NewClientEnvelopeForTest("emoji.send", "msg_003", []byte(`{"emojiCode":"ghost"}`))
	f.handler.HandleEmojiSend(context.Background(), f.session, env.AsInternal())

	if got := f.capture.callCount.Load(); got != 0 {
		t.Errorf("broadcast count = %d, want 0", got)
	}

	requestID, payload := readErrorEnvelope(t, f.clientConn, 2*time.Second)
	if requestID != "msg_003" {
		t.Errorf("envelope.requestId = %q, want \"msg_003\"", requestID)
	}
	if payload.Code != apperror.ErrEmojiNotFound {
		t.Errorf("payload.code = %d, want %d (7001 ErrEmojiNotFound)", payload.Code, apperror.ErrEmojiNotFound)
	}
}

// ---------- AC8.4 user not in any room ----------

// TestEmojiHandler_HandleEmojiSend_UserNotInRoom_Returns6004:
// user.current_room_id == NULL → 回 error 6004 + requestId 回带 + broadcast 0 次。
func TestEmojiHandler_HandleEmojiSend_UserNotInRoom_Returns6004(t *testing.T) {
	f := buildEmojiHandlerFixture(t, 1001, 3001)
	defer f.cleanup()

	f.stubSvc.validateFn = func(_ context.Context, _ string) error { return nil }
	f.stubUser.findByIDFn = func(_ context.Context, _ uint64) (*mysql.User, error) {
		// user 不在任何房间（CurrentRoomID nil pointer）
		return &mysql.User{ID: 1001, CurrentRoomID: nil}, nil
	}

	env := wsapp.NewClientEnvelopeForTest("emoji.send", "msg_004", []byte(`{"emojiCode":"wave"}`))
	f.handler.HandleEmojiSend(context.Background(), f.session, env.AsInternal())

	if got := f.capture.callCount.Load(); got != 0 {
		t.Errorf("broadcast count = %d, want 0", got)
	}

	requestID, payload := readErrorEnvelope(t, f.clientConn, 2*time.Second)
	if requestID != "msg_004" {
		t.Errorf("envelope.requestId = %q, want \"msg_004\"", requestID)
	}
	if payload.Code != apperror.ErrUserNotInRoom {
		t.Errorf("payload.code = %d, want %d (6004 ErrUserNotInRoom)", payload.Code, apperror.ErrUserNotInRoom)
	}
}

// ---------- AC8.5 stale Session cross-room ----------

// TestEmojiHandler_HandleEmojiSend_StaleSession_CrossRoom_Returns6004:
// session.RoomID() == 3001，但 users.current_room_id = 9999（stale Session 跨房间
// 注入风险）→ 回 error 6004 + log warn 含 stale Session 三字段 + broadcast 0 次。
//
// **关键**：本 case 验证 17.1 r1 review 锁定的"反 stale-Session 跨房间双校验" —— 仅
// 判 `current_room_id != NULL` 不够，必须比对 Session.roomID。
func TestEmojiHandler_HandleEmojiSend_StaleSession_CrossRoom_Returns6004(t *testing.T) {
	f := buildEmojiHandlerFixture(t, 1001, 3001)
	defer f.cleanup()

	f.stubSvc.validateFn = func(_ context.Context, _ string) error { return nil }
	f.stubUser.findByIDFn = func(_ context.Context, _ uint64) (*mysql.User, error) {
		// stale: session.RoomID()=3001，DB current_room_id=9999（另一房间）
		otherRoom := uint64(9999)
		return &mysql.User{ID: 1001, CurrentRoomID: &otherRoom}, nil
	}

	env := wsapp.NewClientEnvelopeForTest("emoji.send", "msg_005", []byte(`{"emojiCode":"wave"}`))
	f.handler.HandleEmojiSend(context.Background(), f.session, env.AsInternal())

	if got := f.capture.callCount.Load(); got != 0 {
		t.Errorf("broadcast count = %d, want 0 (stale Session 跨房间注入应被拦截)", got)
	}

	requestID, payload := readErrorEnvelope(t, f.clientConn, 2*time.Second)
	if requestID != "msg_005" {
		t.Errorf("envelope.requestId = %q, want \"msg_005\"", requestID)
	}
	if payload.Code != apperror.ErrUserNotInRoom {
		t.Errorf("payload.code = %d, want %d (6004 stale Session 跨房间)", payload.Code, apperror.ErrUserNotInRoom)
	}
}

// ---------- AC8.6 FindByID DB error ----------

// TestEmojiHandler_HandleEmojiSend_FindByIDError_Returns1009:
// userRepo.FindByID 返 err → 回 error 1009 + requestId 回带 + broadcast 0 次。
func TestEmojiHandler_HandleEmojiSend_FindByIDError_Returns1009(t *testing.T) {
	f := buildEmojiHandlerFixture(t, 1001, 3001)
	defer f.cleanup()

	f.stubSvc.validateFn = func(_ context.Context, _ string) error { return nil }
	f.stubUser.findByIDFn = func(_ context.Context, _ uint64) (*mysql.User, error) {
		return nil, errors.New("driver: connection lost")
	}

	env := wsapp.NewClientEnvelopeForTest("emoji.send", "msg_006", []byte(`{"emojiCode":"wave"}`))
	f.handler.HandleEmojiSend(context.Background(), f.session, env.AsInternal())

	if got := f.capture.callCount.Load(); got != 0 {
		t.Errorf("broadcast count = %d, want 0", got)
	}

	requestID, payload := readErrorEnvelope(t, f.clientConn, 2*time.Second)
	if requestID != "msg_006" {
		t.Errorf("envelope.requestId = %q, want \"msg_006\"", requestID)
	}
	if payload.Code != apperror.ErrServiceBusy {
		t.Errorf("payload.code = %d, want %d (1009 ErrServiceBusy)", payload.Code, apperror.ErrServiceBusy)
	}
}

// ---------- AC8.7 broadcast fails ----------

// TestEmojiHandler_HandleEmojiSend_BroadcastFails_FireAndForget:
// broadcastFn 返 error → 仅 log warn 不回 error envelope 给发起者（fire-and-forget；
// V1 §12.2 钦定）。
func TestEmojiHandler_HandleEmojiSend_BroadcastFails_FireAndForget(t *testing.T) {
	f := buildEmojiHandlerFixture(t, 1001, 3001)
	defer f.cleanup()

	f.stubSvc.validateFn = func(_ context.Context, _ string) error { return nil }
	f.stubUser.findByIDFn = func(_ context.Context, _ uint64) (*mysql.User, error) {
		roomID := uint64(3001)
		return &mysql.User{ID: 1001, CurrentRoomID: &roomID}, nil
	}
	f.capture.returnErr = errors.New("broadcast fanout failed")

	env := wsapp.NewClientEnvelopeForTest("emoji.send", "msg_007", []byte(`{"emojiCode":"wave"}`))
	f.handler.HandleEmojiSend(context.Background(), f.session, env.AsInternal())

	// broadcastFn 调了 1 次（即使 return err）
	if got := f.capture.callCount.Load(); got != 1 {
		t.Errorf("broadcast count = %d, want 1 (broadcastFn called even when it returns err)", got)
	}

	// 验证 client **不收**到任何 frame（fire-and-forget；不回 error envelope）。
	_ = f.clientConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, raw, err := f.clientConn.ReadMessage()
	if err == nil {
		t.Errorf("expected ReadMessage timeout / EOF (fire-and-forget no error frame); got msg: %s", string(raw))
	}
}
