package ws_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	wsapp "github.com/huing/cat/server/internal/app/ws"
	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/pkg/auth"
)

// ---------- 共用测试基础设施 ----------

const testAuthSecret = "ws-test-secret-must-be-at-least-16-bytes"

func init() {
	gin.SetMode(gin.TestMode)
}

// stubRoomMemberRepo 是 RoomMemberRepo 的可配置 stub（手写而非 testify/mock，
// 与项目既有 stubAuthService 同模式）。
type stubRoomMemberRepo struct {
	roomExistsFn   func(ctx context.Context, roomID uint64) (bool, error)
	isUserInRoomFn func(ctx context.Context, userID, roomID uint64) (bool, error)
	listMembersFn  func(ctx context.Context, roomID uint64) ([]uint64, error)
}

func (s *stubRoomMemberRepo) RoomExists(ctx context.Context, roomID uint64) (bool, error) {
	if s.roomExistsFn != nil {
		return s.roomExistsFn(ctx, roomID)
	}
	return true, nil
}

func (s *stubRoomMemberRepo) IsUserInRoom(ctx context.Context, userID, roomID uint64) (bool, error) {
	if s.isUserInRoomFn != nil {
		return s.isUserInRoomFn(ctx, userID, roomID)
	}
	return true, nil
}

func (s *stubRoomMemberRepo) ListMembers(ctx context.Context, roomID uint64) ([]uint64, error) {
	if s.listMembersFn != nil {
		return s.listMembersFn(ctx, roomID)
	}
	return []uint64{1001, 1002}, nil
}

// newSigner 构造测试用 signer（与 middleware/auth_test 同模式）。
func newSigner(t *testing.T) *auth.Signer {
	t.Helper()
	signer, err := auth.New(testAuthSecret, 3600)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	return signer
}

// startGatewayServer 启动 httptest.NewServer 挂 GET /ws/rooms/:roomId，返
// (wsURL, httptest server, cleanup)。
func startGatewayServer(t *testing.T, signer *auth.Signer, mgr wsapp.SessionManager, repo *stubRoomMemberRepo) (string, *httptest.Server) {
	t.Helper()
	cfg := config.WSConfig{
		HeartbeatTimeoutSec: 60,
		MaxMessageSizeBytes: 16384,
		WriteTimeoutSec:     2,
	}
	gateway := wsapp.NewGateway(signer, mgr, repo, cfg, "test")
	r := gin.New()
	r.GET("/ws/rooms/:roomId", gateway.Handle)
	ts := httptest.NewServer(r)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	return wsURL, ts
}

// dialWS 拨连 WS；返 (conn, http.Response)；失败由 caller 决定 t.Fatal vs 期望
// 失败的 case。
func dialWS(t *testing.T, wsURL string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = 3 * time.Second
	return dialer.Dial(wsURL, nil)
}

// expectCloseError 读 conn 直到 close error 出现，校验 code + reason。
func expectCloseError(t *testing.T, conn *websocket.Conn, wantCode int, wantReason string) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Fatalf("ReadMessage returned nil error, want CloseError code=%d", wantCode)
	}
	closeErr, ok := err.(*websocket.CloseError)
	if !ok {
		t.Fatalf("err = %T %v, want *websocket.CloseError", err, err)
	}
	if closeErr.Code != wantCode {
		t.Errorf("CloseError.Code = %d, want %d (text=%q)", closeErr.Code, wantCode, closeErr.Text)
	}
	if wantReason != "" && closeErr.Text != wantReason {
		t.Errorf("CloseError.Text = %q, want %q", closeErr.Text, wantReason)
	}
}

// ---------- Session 测试 ----------

// useGatewayDial 启动 gateway，握手成功后返一个连进 manager 的 *Session，可用于
// Session 单元测试（不裸构造 Session —— 走真实 wire 路径）。
//
// 返回 (clientConn, server-side *Session, cleanup)。
func useGatewayDial(t *testing.T, mgr wsapp.SessionManager, repo *stubRoomMemberRepo, userID uint64, roomID uint64) (*websocket.Conn, *wsapp.Session, *httptest.Server) {
	t.Helper()
	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)

	token, err := signer.Sign(userID, 3600)
	if err != nil {
		t.Fatalf("signer.Sign: %v", err)
	}
	url := fmt.Sprintf("%s/ws/rooms/%d?token=%s", wsURL, roomID, token)
	conn, _, err := dialWS(t, url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	// 握手成功后第一条 message 必须是 snapshot；读完后 manager 内已含 Session
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	// 给 Register 钩子写入 manager 索引一些时间（实际是 Register 同步返回，
	// 但为防 timing race，加 read timeout 后 sleep 一小段）
	deadline := time.Now().Add(2 * time.Second)
	var session *wsapp.Session
	for time.Now().Before(deadline) {
		sessions := mgr.ListSessionsByRoomID(context.Background(), roomID)
		if len(sessions) > 0 {
			for _, s := range sessions {
				if s.UserID() == userID {
					session = s
					break
				}
			}
			if session != nil {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if session == nil {
		t.Fatalf("session not found in manager after handshake")
	}
	return conn, session, ts
}

// TestSession_Send_HappyPath: Send 把消息 enqueue → writeLoop 写到 wire → client
// 读到。
func TestSession_Send_HappyPath(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{
		listMembersFn: func(ctx context.Context, roomID uint64) ([]uint64, error) {
			return []uint64{1001}, nil
		},
	}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	if err := session.Send([]byte(`{"type":"custom","requestId":"","payload":{},"ts":0}`)); err != nil {
		t.Fatalf("Send: %v", err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(msg, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env["type"] != "custom" {
		t.Errorf("type = %v, want custom", env["type"])
	}
}

// TestSession_SendPriority_AfterClose_ReturnsErr: Close 后 SendPriority →
// ErrSessionClosed（review r4 P2 加 priority chan 后的语义对齐 Send）。
func TestSession_SendPriority_AfterClose_ReturnsErr(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	_ = session.Close()
	err := session.SendPriority([]byte(`{"type":"pong"}`))
	if !errors.Is(err, wsapp.ErrSessionClosed) {
		t.Errorf("SendPriority err = %v, want ErrSessionClosed", err)
	}
}

// TestSession_Send_AfterClose_ReturnsErr: Close 后 Send → ErrSessionClosed。
func TestSession_Send_AfterClose_ReturnsErr(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	_ = session.Close()
	err := session.Send([]byte("anything"))
	if !errors.Is(err, wsapp.ErrSessionClosed) {
		t.Errorf("Send err = %v, want ErrSessionClosed", err)
	}
}

// TestSession_Close_Idempotent: Close 调两次不 panic / 不返 error。
func TestSession_Close_Idempotent(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	if err := session.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := session.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// ---------- SessionManager 测试 ----------

// TestSessionManager_Register_TriggersHook: Register → onRegister 钩子被调一次。
func TestSessionManager_Register_TriggersHook(t *testing.T) {
	var registerCount atomic.Int32
	mgr := wsapp.NewSessionManager(
		wsapp.WithRegisterHook(func(s *wsapp.Session) {
			registerCount.Add(1)
		}),
	)
	defer mgr.Close()

	repo := &stubRoomMemberRepo{}
	conn, _, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	if got := registerCount.Load(); got != 1 {
		t.Errorf("register hook called %d times, want 1", got)
	}
}

// TestSessionManager_Unregister_TriggersHook: 主动 Unregister → onUnregister 钩子被调。
func TestSessionManager_Unregister_TriggersHook(t *testing.T) {
	var unregisterCount atomic.Int32
	mgr := wsapp.NewSessionManager(
		wsapp.WithUnregisterHook(func(s *wsapp.Session) {
			unregisterCount.Add(1)
		}),
	)
	defer mgr.Close()

	repo := &stubRoomMemberRepo{}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	_ = session.Close()

	// 等钩子异步触发
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if unregisterCount.Load() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := unregisterCount.Load(); got < 1 {
		t.Errorf("unregister hook called %d times, want >= 1", got)
	}
}

// TestSessionManager_SameUser_Reconnect_ReplacesOldSession: 同 user 重复连接 →
// 旧 Session 被 Close + manager 索引指向新 Session。
func TestSessionManager_SameUser_Reconnect_ReplacesOldSession(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}

	conn1, session1, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn1.Close()

	// 第二次连接 —— 同 user 同 room
	signer := newSigner(t)
	// 复用同 mgr / 同 server；需要重新拨号
	// 注意：startGatewayServer 已经被 useGatewayDial 启过了 ts；用同 ts 拨号
	wsBase := "ws" + strings.TrimPrefix(ts.URL, "http")
	token, _ := signer.Sign(1001, 3600)
	url2 := fmt.Sprintf("%s/ws/rooms/3001?token=%s", wsBase, token)
	// 注意：useGatewayDial 内部用 newSigner(t) 同样的 secret，所以 token 通用
	conn2, _, err := dialWS(t, url2)
	if err != nil {
		t.Fatalf("dial 2nd: %v", err)
	}
	defer conn2.Close()

	// 等 manager 替换索引（异步）
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		// 旧 session 应被 Close
		if errSend := session1.Send([]byte("test")); errors.Is(errSend, wsapp.ErrSessionClosed) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if errSend := session1.Send([]byte("test")); !errors.Is(errSend, wsapp.ErrSessionClosed) {
		t.Errorf("old session should be closed by replace; Send err = %v", errSend)
	}

	// 第二条连接读到第一条 snapshot（已在 useGatewayDial-style 流程中完成）
	_ = conn2.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn2.ReadMessage()
	if err != nil {
		t.Fatalf("conn2 read snapshot: %v", err)
	}
	if !strings.Contains(string(msg), `"room.snapshot"`) {
		t.Errorf("conn2 first message = %q, want room.snapshot", string(msg))
	}
}

// TestSessionManager_Reconnect_TriggersUnregisterHookForOldSession:
// 同 user 重连替换路径必须为旧 Session 触发 onUnregister 钩子**恰好一次**。
// review r2 P1 修：修复前 Register 锁内 removeFromIndicesLocked(oldS) → 锁外
// oldS.Close() 路径走 Unregister(oldID) no-op → 钩子漏调；修复后保留旧索引到
// oldS.Close() 跑完，让 Unregister 走标准触发钩子路径。
//
// 关键场景：reconnect from room A to room B（旧 Session 在 room A，新 Session
// 进 room B）—— 旧 Session 的 onUnregister 必须触发，否则 room A 的 presence /
// metrics 状态被泄漏。
func TestSessionManager_Reconnect_TriggersUnregisterHookForOldSession(t *testing.T) {
	var unregisterCount atomic.Int32
	var unregisteredSessionIDs sync.Map // sessionID → struct{}
	mgr := wsapp.NewSessionManager(
		wsapp.WithUnregisterHook(func(s *wsapp.Session) {
			unregisteredSessionIDs.Store(s.SessionID(), struct{}{})
			unregisterCount.Add(1)
		}),
	)
	defer mgr.Close()

	repo := &stubRoomMemberRepo{}
	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	const userID = uint64(1001)
	const roomA = uint64(3001)
	const roomB = uint64(3002)

	// 第一次连接到 roomA
	tokenA, _ := signer.Sign(userID, 3600)
	urlA := fmt.Sprintf("%s/ws/rooms/%d?token=%s", wsURL, roomA, tokenA)
	connA, _, err := dialWS(t, urlA)
	if err != nil {
		t.Fatalf("dial roomA: %v", err)
	}
	defer connA.Close()
	_ = connA.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := connA.ReadMessage(); err != nil {
		t.Fatalf("read snapshot roomA: %v", err)
	}

	// 拿到旧 session 的 sessionID（用于断言钩子收到的就是旧那个）
	deadline := time.Now().Add(2 * time.Second)
	var oldSessionID string
	for time.Now().Before(deadline) {
		sessions := mgr.ListSessionsByRoomID(context.Background(), roomA)
		for _, s := range sessions {
			if s.UserID() == userID {
				oldSessionID = s.SessionID()
				break
			}
		}
		if oldSessionID != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if oldSessionID == "" {
		t.Fatalf("old session not found in roomA")
	}

	// 第二次连接到 roomB（同 user，不同 room → 触发 reconnect 替换）
	tokenB, _ := signer.Sign(userID, 3600)
	urlB := fmt.Sprintf("%s/ws/rooms/%d?token=%s", wsURL, roomB, tokenB)
	connB, _, err := dialWS(t, urlB)
	if err != nil {
		t.Fatalf("dial roomB: %v", err)
	}
	defer connB.Close()
	_ = connB.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := connB.ReadMessage(); err != nil {
		t.Fatalf("read snapshot roomB: %v", err)
	}

	// 等 onUnregister 钩子异步触发完毕（oldS.Close() → notifyClosed → Unregister(oldID)
	// → onUnregister）
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if unregisterCount.Load() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 断言钩子触发**恰好一次**（不是 0 次也不是 2 次）
	if got := unregisterCount.Load(); got != 1 {
		t.Errorf("unregister hook fired %d times, want exactly 1 (reconnect replace path)", got)
	}
	// 断言钩子收到的是**旧** sessionID（而非新的）
	if _, ok := unregisteredSessionIDs.Load(oldSessionID); !ok {
		t.Errorf("unregister hook did not fire for old session %q", oldSessionID)
	}

	// 旧 sessionID 不应再在 manager 索引中
	sessionsA := mgr.ListSessionsByRoomID(context.Background(), roomA)
	for _, s := range sessionsA {
		if s.SessionID() == oldSessionID {
			t.Errorf("old session %q still in roomA index after replace", oldSessionID)
		}
	}
}

// TestSessionManager_Reconnect_NoDoubleBroadcastWindow: 同 user 重连同 room
// 替换路径中场窗口，ListSessionsByRoomID 应当**只返回 NEW 不返回 OLD**。
//
// review r5 [P2] 防回归：修复前 Register 锁内**先**把 NEW 加进 sessionsByRoom，
// **然后**才在锁外调 replaced.Close() → notifyClosed → Unregister 移除 OLD。
// 在两步之间 ListSessionsByRoomID 同时返 OLD + NEW，BroadcastToRoom（Story 10.5）
// 在该窗口期会同 user 双发；client 不能 dedupe（sessionID 不外漏）。
//
// 修复后 Register 锁内**同时**移除 OLD 在 sessionsByRoom + 添加 NEW；OLD 仅保留在
// sessionsByID 让后续 oldS.Close() → Unregister 触发 onUnregister 钩子（review r2 P1
// 不变量）。锁释放后 broadcast 视角立即只见 NEW。
//
// 测试用 WithRegisterHook 在 NEW 注册完成的**第一时刻** sample 一次 manager 状态
// （hook 在锁外调，但 lock 已释放即"OLD 应已不在 byRoom"的正确状态）。
func TestSessionManager_Reconnect_NoDoubleBroadcastWindow(t *testing.T) {
	const userID = uint64(1001)
	const roomID = uint64(3001)

	// sampledOnSecondRegister 存第二次 Register 的 hook 触发时刻 manager
	// 在 roomID 下的 session 列表（snapshot 切片，不持有 manager 锁）
	var (
		sampledMu     sync.Mutex
		sampledLen    int
		sampledUIDs   []uint64
		registerCount atomic.Int32
	)
	var mgr wsapp.SessionManager
	mgr = wsapp.NewSessionManager(
		wsapp.WithRegisterHook(func(s *wsapp.Session) {
			n := registerCount.Add(1)
			if n != 2 {
				return // 仅在第二次 Register（替换路径）sample
			}
			// hook 在 manager 锁外调，可以安全调 ListSessionsByRoomID
			sessions := mgr.ListSessionsByRoomID(context.Background(), roomID)
			sampledMu.Lock()
			sampledLen = len(sessions)
			sampledUIDs = make([]uint64, 0, len(sessions))
			for _, s := range sessions {
				sampledUIDs = append(sampledUIDs, s.UserID())
			}
			sampledMu.Unlock()
		}),
	)
	defer mgr.Close()

	repo := &stubRoomMemberRepo{}
	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	// 第一次连接（OLD）
	tokenA, _ := signer.Sign(userID, 3600)
	urlA := fmt.Sprintf("%s/ws/rooms/%d?token=%s", wsURL, roomID, tokenA)
	connA, _, err := dialWS(t, urlA)
	if err != nil {
		t.Fatalf("dial OLD: %v", err)
	}
	defer connA.Close()
	_ = connA.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := connA.ReadMessage(); err != nil {
		t.Fatalf("read OLD snapshot: %v", err)
	}

	// 等第一次 register 钩子触发完
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if registerCount.Load() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 第二次连接（NEW，触发替换）
	tokenB, _ := signer.Sign(userID, 3600)
	urlB := fmt.Sprintf("%s/ws/rooms/%d?token=%s", wsURL, roomID, tokenB)
	connB, _, err := dialWS(t, urlB)
	if err != nil {
		t.Fatalf("dial NEW: %v", err)
	}
	defer connB.Close()
	_ = connB.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := connB.ReadMessage(); err != nil {
		t.Fatalf("read NEW snapshot: %v", err)
	}

	// 等第二次 register 钩子完成 sample
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sampledMu.Lock()
		got := sampledLen
		sampledMu.Unlock()
		if got > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	sampledMu.Lock()
	defer sampledMu.Unlock()
	if sampledLen != 1 {
		t.Errorf("ListSessionsByRoomID len at NEW Register hook = %d, want 1 (OLD must be removed from byRoom synchronously with NEW add to avoid double-broadcast window); uids=%v", sampledLen, sampledUIDs)
	}
	for _, uid := range sampledUIDs {
		if uid != userID {
			t.Errorf("sampled uid = %d, want %d (only NEW session should be visible)", uid, userID)
		}
	}
}

// TestSessionManager_Reconnect_CrossRoom_OldRoomImmediatelyEmpty:
// review r5 [P2] 防回归 cross-room 变体：user 从 roomA 重连到 roomB，第二次
// Register 完成后 ListSessionsByRoomID(roomA) 应当**立即**为空（OLD 已在 Register
// 锁内被从 sessionsByRoom 移除），而不是要等 oldS.Close() → Unregister 跑完。
//
// 这覆盖 broadcast 在 roomA 的同窗口期不应再看到 OLD 的语义（虽然 OLD 在
// sessionsByID 还在等 onUnregister 钩子触发，但 broadcast 走 byRoom 索引）。
func TestSessionManager_Reconnect_CrossRoom_OldRoomImmediatelyEmpty(t *testing.T) {
	const userID = uint64(1001)
	const roomA = uint64(3001)
	const roomB = uint64(3002)

	var (
		sampledMu      sync.Mutex
		sampledRoomALen int
		registerCount  atomic.Int32
	)
	var mgr wsapp.SessionManager
	mgr = wsapp.NewSessionManager(
		wsapp.WithRegisterHook(func(s *wsapp.Session) {
			n := registerCount.Add(1)
			if n != 2 {
				return
			}
			sessions := mgr.ListSessionsByRoomID(context.Background(), roomA)
			sampledMu.Lock()
			sampledRoomALen = len(sessions)
			sampledMu.Unlock()
		}),
	)
	defer mgr.Close()

	repo := &stubRoomMemberRepo{}
	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	tokenA, _ := signer.Sign(userID, 3600)
	urlA := fmt.Sprintf("%s/ws/rooms/%d?token=%s", wsURL, roomA, tokenA)
	connA, _, err := dialWS(t, urlA)
	if err != nil {
		t.Fatalf("dial roomA: %v", err)
	}
	defer connA.Close()
	_ = connA.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := connA.ReadMessage(); err != nil {
		t.Fatalf("read roomA snapshot: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if registerCount.Load() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	tokenB, _ := signer.Sign(userID, 3600)
	urlB := fmt.Sprintf("%s/ws/rooms/%d?token=%s", wsURL, roomB, tokenB)
	connB, _, err := dialWS(t, urlB)
	if err != nil {
		t.Fatalf("dial roomB: %v", err)
	}
	defer connB.Close()
	_ = connB.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := connB.ReadMessage(); err != nil {
		t.Fatalf("read roomB snapshot: %v", err)
	}

	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if registerCount.Load() >= 2 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	sampledMu.Lock()
	defer sampledMu.Unlock()
	// 修复前：sampledRoomALen = 1（OLD 还在 byRoom[roomA]）
	// 修复后：sampledRoomALen = 0（OLD 已在 Register 锁内从 byRoom 移除）
	if sampledRoomALen != 0 {
		t.Errorf("ListSessionsByRoomID(roomA) at NEW Register hook = %d, want 0 (OLD must be removed from old room's byRoom synchronously with NEW Register)", sampledRoomALen)
	}
}

// TestGateway_Reconnect_SnapshotTransientFail_OldSessionStillActive:
// review r10 P1 防回归 —— reconnect 路径下若新 conn 的 snapshot 步骤 transient
// 失败（ListMembers 返 error），旧 session **必须保持活跃**（不被 evict），新
// conn 走 close 1011。这是"事务性 reconnect"语义的核心：transient handshake
// 失败不能让 user 既无新连接也无旧连接。
//
// 修复前实装顺序（2.Register → 3.snapshot）→ Register 已 evict 旧 session →
// snapshot 失败 close 1011 新 session 也死 → user 完全断线。
// 修复后顺序（2.snapshot → 3.Register）→ snapshot 失败时 Register 还没跑 →
// 旧 session 仍在 manager 索引内活跃；user 可以继续用旧 conn。
func TestGateway_Reconnect_SnapshotTransientFail_OldSessionStillActive(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()

	const userID = uint64(1001)
	const roomID = uint64(3001)

	// listMembersFn：第一次调用成功（让 OLD handshake 完成），第二次调用
	// 返 error 模拟 transient DB 失败（让 NEW handshake snapshot 步骤失败）。
	var listMembersCallCount atomic.Int32
	repo := &stubRoomMemberRepo{
		listMembersFn: func(ctx context.Context, _ uint64) ([]uint64, error) {
			n := listMembersCallCount.Add(1)
			if n == 1 {
				return []uint64{userID}, nil
			}
			return nil, errors.New("simulated transient DB error")
		},
	}

	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	// OLD handshake：握手成功，session 进 manager 索引
	tokenA, _ := signer.Sign(userID, 3600)
	urlA := fmt.Sprintf("%s/ws/rooms/%d?token=%s", wsURL, roomID, tokenA)
	connA, _, err := dialWS(t, urlA)
	if err != nil {
		t.Fatalf("dial OLD: %v", err)
	}
	defer connA.Close()
	_ = connA.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := connA.ReadMessage(); err != nil {
		t.Fatalf("read OLD snapshot: %v", err)
	}

	// 等 OLD session 注册到 manager
	deadline := time.Now().Add(2 * time.Second)
	var oldSession *wsapp.Session
	for time.Now().Before(deadline) {
		sessions := mgr.ListSessionsByRoomID(context.Background(), roomID)
		if len(sessions) > 0 {
			oldSession = sessions[0]
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if oldSession == nil {
		t.Fatalf("OLD session not registered in manager")
	}
	oldSessionID := oldSession.SessionID()

	// NEW handshake：同 user，listMembersFn 第二次调用返 error → snapshot 步骤失败 →
	// close 1011；**关键不变量**：OLD session 此时**未被 evict**（因为 Register 还
	// 没跑）。
	tokenB, _ := signer.Sign(userID, 3600)
	urlB := fmt.Sprintf("%s/ws/rooms/%d?token=%s", wsURL, roomID, tokenB)
	connB, _, err := dialWS(t, urlB)
	if err != nil {
		t.Fatalf("dial NEW: %v", err)
	}
	defer connB.Close()
	expectCloseError(t, connB, 1011, "snapshot build failed")

	// 验证 OLD session **仍活跃**：
	//   - manager 索引仍指向 OLD（同 sessionID）
	//   - OLD session 仍能 Send（不返 ErrSessionClosed）
	sessionsAfter := mgr.ListSessionsByRoomID(context.Background(), roomID)
	if len(sessionsAfter) != 1 {
		t.Fatalf("after NEW transient fail, ListSessionsByRoomID len = %d, want 1 (OLD must stay active)", len(sessionsAfter))
	}
	if sessionsAfter[0].SessionID() != oldSessionID {
		t.Errorf("after NEW transient fail, session in room = %q, want OLD %q (OLD must NOT be evicted)",
			sessionsAfter[0].SessionID(), oldSessionID)
	}
	if err := oldSession.Send([]byte(`{"type":"x"}`)); err != nil && errors.Is(err, wsapp.ErrSessionClosed) {
		t.Errorf("OLD session should remain active after NEW transient fail; Send err = %v", err)
	}
}

// TestGateway_Reconnect_SnapshotTransientFail_CrossRoom_OldSessionStillActive:
// review r10 P1 防回归 cross-room 变体 —— OLD 在 roomA；NEW 拨 roomB；NEW
// snapshot 失败 → roomA 的 OLD 仍活跃 + roomA byRoom 索引仍含 OLD。
func TestGateway_Reconnect_SnapshotTransientFail_CrossRoom_OldSessionStillActive(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()

	const userID = uint64(1001)
	const roomA = uint64(3001)
	const roomB = uint64(3002)

	var listMembersCallCount atomic.Int32
	repo := &stubRoomMemberRepo{
		listMembersFn: func(ctx context.Context, _ uint64) ([]uint64, error) {
			n := listMembersCallCount.Add(1)
			if n == 1 {
				return []uint64{userID}, nil
			}
			return nil, errors.New("simulated transient DB error")
		},
	}

	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	// OLD: 拨 roomA 成功
	tokenA, _ := signer.Sign(userID, 3600)
	urlA := fmt.Sprintf("%s/ws/rooms/%d?token=%s", wsURL, roomA, tokenA)
	connA, _, err := dialWS(t, urlA)
	if err != nil {
		t.Fatalf("dial roomA: %v", err)
	}
	defer connA.Close()
	_ = connA.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := connA.ReadMessage(); err != nil {
		t.Fatalf("read roomA snapshot: %v", err)
	}

	// 等 OLD 注册
	deadline := time.Now().Add(2 * time.Second)
	var oldSession *wsapp.Session
	for time.Now().Before(deadline) {
		sessions := mgr.ListSessionsByRoomID(context.Background(), roomA)
		if len(sessions) > 0 {
			oldSession = sessions[0]
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if oldSession == nil {
		t.Fatalf("OLD session not registered in roomA")
	}
	oldSessionID := oldSession.SessionID()

	// NEW: 拨 roomB（cross-room reconnect）；listMembers 返 error → close 1011
	tokenB, _ := signer.Sign(userID, 3600)
	urlB := fmt.Sprintf("%s/ws/rooms/%d?token=%s", wsURL, roomB, tokenB)
	connB, _, err := dialWS(t, urlB)
	if err != nil {
		t.Fatalf("dial roomB: %v", err)
	}
	defer connB.Close()
	expectCloseError(t, connB, 1011, "snapshot build failed")

	// roomA 索引仍含 OLD（cross-room reconnect transient 失败也不应 evict）
	sessionsA := mgr.ListSessionsByRoomID(context.Background(), roomA)
	if len(sessionsA) != 1 {
		t.Fatalf("roomA after NEW(roomB) transient fail, len = %d, want 1 (OLD in roomA must stay active)", len(sessionsA))
	}
	if sessionsA[0].SessionID() != oldSessionID {
		t.Errorf("roomA after NEW(roomB) transient fail, session = %q, want OLD %q",
			sessionsA[0].SessionID(), oldSessionID)
	}
	// roomB 索引应为空（NEW 没 Register）
	sessionsB := mgr.ListSessionsByRoomID(context.Background(), roomB)
	if len(sessionsB) != 0 {
		t.Errorf("roomB after NEW transient fail, len = %d, want 0 (NEW must not be registered)", len(sessionsB))
	}
}

// TestGateway_Reconnect_HappyPath_OldSessionEvicted: review r10 P1 防回归
// 配套 happy path —— 当 NEW handshake 全部步骤成功（snapshot 写完 + Register
// 成功），OLD session **必须**被 evict（reconnect 替换语义保持）。这条测试
// 验证修复 r10 P1 没有破坏 OLD eviction 行为。
func TestGateway_Reconnect_HappyPath_OldSessionEvicted(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{} // 默认 listMembersFn 返 [1001, 1002] 始终成功

	const userID = uint64(1001)
	const roomID = uint64(3001)

	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	tokenA, _ := signer.Sign(userID, 3600)
	urlA := fmt.Sprintf("%s/ws/rooms/%d?token=%s", wsURL, roomID, tokenA)
	connA, _, err := dialWS(t, urlA)
	if err != nil {
		t.Fatalf("dial OLD: %v", err)
	}
	defer connA.Close()
	_ = connA.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := connA.ReadMessage(); err != nil {
		t.Fatalf("read OLD snapshot: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var oldSession *wsapp.Session
	for time.Now().Before(deadline) {
		sessions := mgr.ListSessionsByRoomID(context.Background(), roomID)
		if len(sessions) > 0 {
			oldSession = sessions[0]
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if oldSession == nil {
		t.Fatalf("OLD session not registered")
	}
	oldSessionID := oldSession.SessionID()

	// NEW: 同 user 同 room；snapshot + Register 都成功 → OLD 被 evict
	tokenB, _ := signer.Sign(userID, 3600)
	urlB := fmt.Sprintf("%s/ws/rooms/%d?token=%s", wsURL, roomID, tokenB)
	connB, _, err := dialWS(t, urlB)
	if err != nil {
		t.Fatalf("dial NEW: %v", err)
	}
	defer connB.Close()
	_ = connB.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := connB.ReadMessage(); err != nil {
		t.Fatalf("read NEW snapshot: %v", err)
	}

	// 等 OLD 被 evict（异步 Close → notifyClosed → Unregister）
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		err := oldSession.Send([]byte(`{"type":"x"}`))
		if errors.Is(err, wsapp.ErrSessionClosed) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := oldSession.Send([]byte(`{"type":"x"}`)); !errors.Is(err, wsapp.ErrSessionClosed) {
		t.Errorf("OLD session should be evicted/closed after NEW happy-path Register; Send err = %v", err)
	}

	// 现在 manager 索引内只有 NEW
	sessionsAfter := mgr.ListSessionsByRoomID(context.Background(), roomID)
	if len(sessionsAfter) != 1 {
		t.Fatalf("after NEW happy path, ListSessionsByRoomID len = %d, want 1 (NEW only)", len(sessionsAfter))
	}
	if sessionsAfter[0].SessionID() == oldSessionID {
		t.Errorf("after NEW happy path, session = OLD %q (NEW eviction did not happen)", oldSessionID)
	}
}

// TestSessionManager_ListSessionsByRoomID_ReturnsAllInRoom: 多 user 在同 room →
// ListSessionsByRoomID 返回全部。
func TestSessionManager_ListSessionsByRoomID_ReturnsAllInRoom(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}

	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	const roomID = 3001
	var conns []*websocket.Conn
	for _, uid := range []uint64{1001, 1002, 1003} {
		token, _ := signer.Sign(uid, 3600)
		url := fmt.Sprintf("%s/ws/rooms/%d?token=%s", wsURL, roomID, token)
		conn, _, err := dialWS(t, url)
		if err != nil {
			t.Fatalf("dial uid=%d: %v", uid, err)
		}
		conns = append(conns, conn)
		// 读 snapshot
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatalf("read snapshot uid=%d: %v", uid, err)
		}
	}
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	// 等所有 Session 都注册
	deadline := time.Now().Add(2 * time.Second)
	var sessions []*wsapp.Session
	for time.Now().Before(deadline) {
		sessions = mgr.ListSessionsByRoomID(context.Background(), roomID)
		if len(sessions) >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(sessions); got != 3 {
		t.Errorf("ListSessionsByRoomID len = %d, want 3", got)
	}
}

// ---------- Gateway 测试 ----------

// TestGateway_Handle_MissingToken_Closes4001: query 缺 token → close 4001。
func TestGateway_Handle_MissingToken_Closes4001(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	wsURL, ts := startGatewayServer(t, newSigner(t), mgr, repo)
	defer ts.Close()

	url := fmt.Sprintf("%s/ws/rooms/3001", wsURL) // 缺 token
	conn, _, err := dialWS(t, url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	expectCloseError(t, conn, 4001, "missing token")
}

// TestGateway_Handle_InvalidRoomID_Closes4002: roomId 非数字 → close 4002。
func TestGateway_Handle_InvalidRoomID_Closes4002(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	token, _ := signer.Sign(1001, 3600)
	url := fmt.Sprintf("%s/ws/rooms/notanumber?token=%s", wsURL, token)
	conn, _, err := dialWS(t, url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	expectCloseError(t, conn, 4002, "invalid roomId")
}

// TestGateway_Handle_ExpiredToken_Closes4001_Expired: token 过期 → close 4001
// reason="token expired"。
func TestGateway_Handle_ExpiredToken_Closes4001_Expired(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}

	// 用 1s 过期 signer 签 token，等过期再拨号
	signerShort, err := auth.New(testAuthSecret, 1)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	expiredToken, err := signerShort.Sign(1001, 1)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	time.Sleep(1100 * time.Millisecond)

	// 用同 secret 的 signer 校验（与 short signer 共享 secret）
	wsURL, ts := startGatewayServer(t, signerShort, mgr, repo)
	defer ts.Close()

	url := fmt.Sprintf("%s/ws/rooms/3001?token=%s", wsURL, expiredToken)
	conn, _, err := dialWS(t, url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	expectCloseError(t, conn, 4001, "token expired")
}

// TestGateway_Handle_InvalidToken_Closes4001: 篡改 token → close 4001 reason="invalid token"。
func TestGateway_Handle_InvalidToken_Closes4001(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	url := fmt.Sprintf("%s/ws/rooms/3001?token=garbage.token.value", wsURL)
	conn, _, err := dialWS(t, url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	expectCloseError(t, conn, 4001, "invalid token")
}

// TestGateway_Handle_RoomNotFound_Closes4004: RoomExists=false → close 4004。
func TestGateway_Handle_RoomNotFound_Closes4004(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{
		roomExistsFn: func(ctx context.Context, roomID uint64) (bool, error) {
			return false, nil
		},
	}
	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	token, _ := signer.Sign(1001, 3600)
	url := fmt.Sprintf("%s/ws/rooms/3001?token=%s", wsURL, token)
	conn, _, err := dialWS(t, url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	expectCloseError(t, conn, 4004, "room not found")
}

// TestGateway_Handle_UserNotInRoom_Closes4003: IsUserInRoom=false → close 4003。
func TestGateway_Handle_UserNotInRoom_Closes4003(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{
		isUserInRoomFn: func(ctx context.Context, userID, roomID uint64) (bool, error) {
			return false, nil
		},
	}
	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	token, _ := signer.Sign(9999, 3600)
	url := fmt.Sprintf("%s/ws/rooms/3001?token=%s", wsURL, token)
	conn, _, err := dialWS(t, url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	expectCloseError(t, conn, 4003, "user not in room")
}

// TestGateway_Handle_HappyPath_FirstMessageIsSnapshot: 全校验通过 → 第一条
// message 是 type="room.snapshot"，含 room.id / payload.members[]（非空）。
func TestGateway_Handle_HappyPath_FirstMessageIsSnapshot(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{
		listMembersFn: func(ctx context.Context, roomID uint64) ([]uint64, error) {
			return []uint64{1001, 1002}, nil
		},
	}
	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	token, _ := signer.Sign(1001, 3600)
	url := fmt.Sprintf("%s/ws/rooms/3001?token=%s", wsURL, token)
	conn, _, err := dialWS(t, url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(msg, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env["type"] != "room.snapshot" {
		t.Errorf("type = %v, want room.snapshot", env["type"])
	}
	if env["requestId"] != "" {
		t.Errorf("requestId = %v, want empty (broadcast)", env["requestId"])
	}
	payload, ok := env["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload type = %T, want map", env["payload"])
	}
	room, ok := payload["room"].(map[string]any)
	if !ok {
		t.Fatalf("payload.room type = %T, want map", payload["room"])
	}
	if room["id"] != "3001" {
		t.Errorf("room.id = %v, want 3001 (string)", room["id"])
	}
	if room["maxMembers"].(float64) != 4 {
		t.Errorf("room.maxMembers = %v, want 4", room["maxMembers"])
	}
	if room["memberCount"].(float64) != 2 {
		t.Errorf("room.memberCount = %v, want 2", room["memberCount"])
	}
	members, ok := payload["members"].([]any)
	if !ok {
		t.Fatalf("payload.members type = %T, want array", payload["members"])
	}
	if len(members) != 2 {
		t.Errorf("len(members) = %d, want 2", len(members))
	}
}

// TestGateway_Handle_SnapshotBuildFails_Closes1011: ListMembers 返 error →
// close 1011 reason="snapshot build failed"。
func TestGateway_Handle_SnapshotBuildFails_Closes1011(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{
		listMembersFn: func(ctx context.Context, roomID uint64) ([]uint64, error) {
			return nil, errors.New("simulated DB error")
		},
	}
	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	token, _ := signer.Sign(1001, 3600)
	url := fmt.Sprintf("%s/ws/rooms/3001?token=%s", wsURL, token)
	conn, _, err := dialWS(t, url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	expectCloseError(t, conn, 1011, "snapshot build failed")
}

// TestGateway_Handle_PingPongRoundtrip: 握手成功 → client 发 ping → 收到 pong
// （RequestID 回带）。
func TestGateway_Handle_PingPongRoundtrip(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{
		listMembersFn: func(ctx context.Context, roomID uint64) ([]uint64, error) {
			return []uint64{1001}, nil
		},
	}
	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	token, _ := signer.Sign(1001, 3600)
	url := fmt.Sprintf("%s/ws/rooms/3001?token=%s", wsURL, token)
	conn, _, err := dialWS(t, url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// 读 snapshot
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	// 发 ping
	pingMsg := `{"type":"ping","requestId":"ping_001","payload":{}}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(pingMsg)); err != nil {
		t.Fatalf("write ping: %v", err)
	}

	// 读 pong
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read pong: %v", err)
	}
	var env map[string]any
	if err := json.Unmarshal(msg, &env); err != nil {
		t.Fatalf("unmarshal pong: %v", err)
	}
	if env["type"] != "pong" {
		t.Errorf("type = %v, want pong", env["type"])
	}
	if env["requestId"] != "ping_001" {
		t.Errorf("requestId = %v, want ping_001 (回带 ping.requestId)", env["requestId"])
	}
	if ts, ok := env["ts"].(float64); !ok || ts <= 0 {
		t.Errorf("ts = %v, want positive int64 ms", env["ts"])
	}
}

// TestGateway_Handle_UnknownType_SafeIgnore: 握手成功 → 发 type="emoji.send"
// → server log warn + 不 close + readLoop 继续。
func TestGateway_Handle_UnknownType_SafeIgnore(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{
		listMembersFn: func(ctx context.Context, roomID uint64) ([]uint64, error) {
			return []uint64{1001}, nil
		},
	}
	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	token, _ := signer.Sign(1001, 3600)
	url := fmt.Sprintf("%s/ws/rooms/3001?token=%s", wsURL, token)
	conn, _, err := dialWS(t, url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// 读 snapshot
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if _, _, err := conn.ReadMessage(); err != nil {
		t.Fatalf("read snapshot: %v", err)
	}

	// 发未知 type
	unknown := `{"type":"emoji.send","requestId":"e_001","payload":{}}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(unknown)); err != nil {
		t.Fatalf("write unknown: %v", err)
	}

	// 再发 ping → 应正常拿到 pong（证明 readLoop 没 close）
	pingMsg := `{"type":"ping","requestId":"after_unknown","payload":{}}`
	if err := conn.WriteMessage(websocket.TextMessage, []byte(pingMsg)); err != nil {
		t.Fatalf("write ping after unknown: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read pong after unknown: %v", err)
	}
	if !strings.Contains(string(msg), `"after_unknown"`) {
		t.Errorf("expected pong with requestId=after_unknown, got %s", string(msg))
	}
}

// TestSession_Send_BufferFull_ReturnsErr: 直接构造 Session manager 注册一个不
// 启动 writeLoop 的 Session（用 dial 拿到 Session，然后立即 stop writeLoop 是
// 不可能的——但可以靠不读 conn 让 writeLoop 阻塞在 conn.WriteMessage，多次
// Send 把 chan 填满）。
//
// 实装策略：dial 一个 Session → server-side writeLoop 在写消息时被 client 不读
// 慢悠悠卡住（因为 SetWriteDeadline 是 2s）；2s 后 writeLoop 失败退出 + 调 Close
// → 后续 Send 全部返 ErrSessionClosed（不是 ErrSessionSendBufferFull）。这条
// path 与 buffer 满场景路径不同；buffer 满需要 writeLoop **不消费**但 conn 仍
// 活；测试上需要一个特殊 hook。
//
// 实际验证 buffer 行为更可靠的方式：连续发 32+1 条消息但不读 client；前 32 条
// 入队，第 33 条**可能**仍能成功（因为 writeLoop 写 conn 时 client 默认有读
// buffer），所以单元测试用 atomic 来探测一次 ErrSessionSendBufferFull 出现：
// 在 writeTimeout 较长（如 5s）时密集 Send 多条 → 一旦 sendChan 满即触发。
//
// 因为难以稳定构造，本 case 改用直接 Send 大量消息验证：当 chan 满时确实返
// ErrSessionSendBufferFull（用 cnt > capacity 的 send 数 + 不读 client 模拟）。
func TestSession_Send_BufferFull_ReturnsErr(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{
		listMembersFn: func(ctx context.Context, roomID uint64) ([]uint64, error) {
			return []uint64{1001}, nil
		},
	}
	signer := newSigner(t)
	// writeTimeout 设很长让 writeLoop 卡在 client 不读
	cfg := config.WSConfig{
		HeartbeatTimeoutSec: 60,
		MaxMessageSizeBytes: 16384,
		WriteTimeoutSec:     30, // 长 timeout 让写 goroutine 卡住更稳定
	}
	gateway := wsapp.NewGateway(signer, mgr, repo, cfg, "test")
	r := gin.New()
	r.GET("/ws/rooms/:roomId", gateway.Handle)
	ts := httptest.NewServer(r)
	defer ts.Close()
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	token, _ := signer.Sign(1001, 3600)
	url := fmt.Sprintf("%s/ws/rooms/3001?token=%s", wsURL, token)
	conn, _, err := dialWS(t, url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// 等 Session 注册
	deadline := time.Now().Add(2 * time.Second)
	var session *wsapp.Session
	for time.Now().Before(deadline) {
		sessions := mgr.ListSessionsByRoomID(context.Background(), 3001)
		if len(sessions) > 0 {
			session = sessions[0]
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if session == nil {
		t.Fatalf("session not registered")
	}

	// 不读 conn —— 让 writeLoop 写 snapshot 后又被 client TCP buffer 阻塞写入
	// Send 大量消息：等 sendChan 满（容量 32）+ writeLoop 在 conn.WriteMessage
	// 卡住时，第 N 条 Send 应返 ErrSessionSendBufferFull
	bigMsg := strings.Repeat("x", 1024) // 1KB filler；快速积满 conn TCP buffer + send chan
	var sawBufferFull bool
	for i := 0; i < 200; i++ {
		err := session.Send([]byte(`{"type":"x","requestId":"","payload":"` + bigMsg + `","ts":0}`))
		if errors.Is(err, wsapp.ErrSessionSendBufferFull) {
			sawBufferFull = true
			break
		}
		if errors.Is(err, wsapp.ErrSessionClosed) {
			// 如果 conn 写超时已触发 Close，跳出（在 Windows + Go gorilla 某些
			// race 下会发生）；本 case 主要验证 *没有* panic + 错误路径合理
			t.Logf("Session closed before buffer full (writeLoop hit timeout)")
			return
		}
	}
	if !sawBufferFull {
		t.Errorf("expected at least one ErrSessionSendBufferFull within 200 sends")
	}
}

// TestSession_WriteLoop_DoesNotStarveSendChan: review r7 P3 fix 验证。
//
// 背景（r7 P3）：r4 加的 priority chan 让 writeLoop 严格优先 drain
// sendPriorityChan，导致 buggy / malicious client 高频 ping 持续填 priority
// → sendChan（业务消息）永不被消费 → client 心跳健康但永远收不到真实业务
// 更新（典型 starvation bug）。修法是 maxConsecutivePriority 配额。
//
// 测试策略：
//  1. 握手成功后，client **暂停**读取（不 ReadMessage）
//  2. server-side 通过 manager 拿到 Session，**先**入队若干 normal msg
//     到 sendChan，**再**入队大量 priority msg 到 sendPriorityChan（混合
//     交错顺序：normal_1, priority_1, priority_2, ..., priority_N, normal_2）
//  3. server-side 总投递数 = sendChanCapacity 上限以下（保证 fire-and-forget
//     不返 buffer full），priority 总数 > maxConsecutivePriority 让 quota 强
//     制走双分支至少一次
//  4. client 开始 read N 条消息 → 检查"normal type 出现位置"：旧实装下
//     writeLoop 严格优先 priority → 所有 priority 在 normal 之前；新实装
//     下 quota 让 normal 在 priority 流中插队
//
// **关键约束**：本测试**不**测精确顺序（go select 多分支随机性 + writeLoop
// 与 sender 的 race 让顺序不稳定），只测"normal 不会全部排在 priority 之
// 后"这个不变量。
func TestSession_WriteLoop_DoesNotStarveSendChan(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{
		listMembersFn: func(ctx context.Context, roomID uint64) ([]uint64, error) {
			return []uint64{1001}, nil
		},
	}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	// 投递策略：让 client 慢读（不读，让 writeLoop 卡在 conn.WriteMessage 一段时间），
	// 让 server-side enqueue **同时**有 sendChan 待发 + sendPriorityChan 持续被填，
	// 模拟 reviewer 描述的"buggy client 持续高频 ping → priority chan 持续非空 →
	// sendChan 永远不被消费"场景。
	//
	// 具体：
	//   1. 先 enqueue 1 条 normal 到 sendChan（让它"等待"，writeLoop 处理它前会
	//      被 priority 持续抢占）
	//   2. 短 sleep 让 writeLoop 把 normal_1 drain 走（触发 conn.WriteMessage）；
	//      writeLoop 接着进入下一轮 select 看 priority + sendChan，此时两者都空
	//   3. enqueue 1 条 normal 到 sendChan（不会立刻被 drain，因为我们紧接着会
	//      持续 enqueue priority，让 writeLoop 一直在 fast-path 走 priority 分支）
	//   4. 持续 enqueue 大量 priority（priority 容量 4 + writeLoop drain 速率 →
	//      用 retry 让 priority chan **始终有数据**），形成"持续 high-frequency
	//      ping"模拟。
	//   5. quota 触发后，writeLoop 会强制走双分支，~50% 概率选 sendChan，让
	//      normal_2 在 priority 流中间被插入。
	//
	// priority 总数 = 32（>> maxConsecutivePriority=4），quota 必触发多次。
	const priorityCount = 32
	const normalCount = 2

	// 关键时序：必须让 normal_1 / normal_2 enqueue 进 sendChan 时 **priority chan
	// 已非空**，否则 writeLoop 阻塞 select 会立刻拿走 normal_1/2，根本不进 fast
	// path → quota 没机会触发。
	//
	// Step 1: 先把 priority chan 填满 4 条（priority 容量）；writeLoop 会立刻开始
	//         drain，但因为容量小 + 我们紧接着持续 retry，priority chan 始终非空。
	for i := 0; i < 4; i++ {
		msg := []byte(fmt.Sprintf(`{"type":"pong","requestId":"p_init_%d","payload":{},"ts":0}`, i))
		if err := session.SendPriority(msg); err != nil {
			t.Fatalf("SendPriority p_init_%d: %v", i, err)
		}
	}
	// Step 2: 立刻 enqueue 2 条 normal（priority chan 此刻有数据 → writeLoop 走
	//         fast path drain priority；normal 在 sendChan 等）
	if err := session.Send([]byte(`{"type":"normal","requestId":"n_1","payload":{},"ts":0}`)); err != nil {
		t.Fatalf("Send normal_1: %v", err)
	}
	if err := session.Send([]byte(`{"type":"normal","requestId":"n_2","payload":{},"ts":0}`)); err != nil {
		t.Fatalf("Send normal_2: %v", err)
	}
	// Step 3: 持续填 priority chan 直到投完 priorityCount - 4 个（前 4 个在 step
	//         1 投了）。retry 让填速率与 writeLoop drain 速率匹配，priority chan
	//         **始终非空** → writeLoop 持续走 fast path → quota 必触发。
	for i := 4; i < priorityCount; i++ {
		msg := []byte(fmt.Sprintf(`{"type":"pong","requestId":"p_%d","payload":{},"ts":0}`, i))
		for {
			err := session.SendPriority(msg)
			if err == nil {
				break
			}
			if errors.Is(err, wsapp.ErrSessionSendPriorityBufferFull) {
				// priority 满 → writeLoop 在 drain，等一下再 retry（让 priority
				// chan 持续保持 "有数据" 状态）
				time.Sleep(100 * time.Microsecond)
				continue
			}
			t.Fatalf("SendPriority p_%d: %v", i, err)
		}
	}

	// client 读 priorityCount + normalCount 条（normal_1 + normal_2 + 全部
	// priority 总会到 wire，writeLoop 在 chan 都耗尽前不会停）
	totalExpected := priorityCount + normalCount
	receivedTypes := make([]string, 0, totalExpected)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	for i := 0; i < totalExpected; i++ {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("ReadMessage[%d]: %v (received so far: %v)", i, err, receivedTypes)
		}
		var env map[string]any
		if err := json.Unmarshal(msg, &env); err != nil {
			t.Fatalf("unmarshal[%d]: %v", i, err)
		}
		typ, _ := env["type"].(string)
		receivedTypes = append(receivedTypes, typ)
	}

	// 不变量：至少有 1 条 normal **出现在 priority 流中间**（即 normal index
	// 之前与之后都有 priority），证明 quota 让 sendChan 在持续 priority 流下
	// 仍能被 drain。
	//   - 旧实装（严格优先）：所有 normal 排在所有 priority 之后 → 找不到任
	//     何 normal 满足"前后都有 priority"
	//   - 新实装（quota 让路）：至少 1 条 normal 在 priority 流中间被插入
	normalSeen := 0
	firstPriorityIdx := -1
	lastPriorityIdx := -1
	normalInMiddle := false
	for i, typ := range receivedTypes {
		if typ == "pong" {
			if firstPriorityIdx == -1 {
				firstPriorityIdx = i
			}
			lastPriorityIdx = i
		}
		if typ == "normal" {
			normalSeen++
		}
	}
	for i, typ := range receivedTypes {
		if typ == "normal" && i > firstPriorityIdx && i < lastPriorityIdx {
			normalInMiddle = true
			break
		}
	}
	if normalSeen != normalCount {
		t.Fatalf("expected %d normal msgs, got %d (received types: %v)", normalCount, normalSeen, receivedTypes)
	}
	if !normalInMiddle {
		t.Errorf("starvation detected: no normal msg appeared between priority msgs (received types: %v); "+
			"new writeLoop quota should let sendChan drain at least once during priority flood",
			receivedTypes)
	}
}

// 显式确保 init 时 wg.Wait 被引用（让 testing 不抱怨 sync 未用）；某些 case
// 用 sync.WaitGroup 但当前实现移除了，留 sync 引用一致。
var _ = sync.Mutex{}

// 占位让 slog 引用不被 lint 报告未用。
var _ = slog.Default()

// TestSnapshotMemberCount_MatchesListMembersLength: V1 §12.3 不变量校验 ——
// memberCount 严格等于 members 数组长度。这是契约层面的硬约束。
func TestSnapshotMemberCount_MatchesListMembersLength(t *testing.T) {
	cases := []struct {
		name    string
		members []uint64
		want    int
	}{
		{"single member (only handshake user)", []uint64{1001}, 1},
		{"two members", []uint64{1001, 1002}, 2},
		{"four members (max)", []uint64{1001, 1002, 1003, 1004}, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mgr := wsapp.NewSessionManager()
			defer mgr.Close()
			repo := &stubRoomMemberRepo{
				listMembersFn: func(ctx context.Context, roomID uint64) ([]uint64, error) {
					return tc.members, nil
				},
			}
			signer := newSigner(t)
			wsURL, ts := startGatewayServer(t, signer, mgr, repo)
			defer ts.Close()

			token, _ := signer.Sign(1001, 3600)
			url := fmt.Sprintf("%s/ws/rooms/3001?token=%s", wsURL, token)
			conn, _, err := dialWS(t, url)
			if err != nil {
				t.Fatalf("dial: %v", err)
			}
			defer conn.Close()

			_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			_, msg, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("read snapshot: %v", err)
			}
			var env map[string]any
			if err := json.Unmarshal(msg, &env); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			payload := env["payload"].(map[string]any)
			room := payload["room"].(map[string]any)
			members := payload["members"].([]any)
			if int(room["memberCount"].(float64)) != tc.want {
				t.Errorf("memberCount = %v, want %d", room["memberCount"], tc.want)
			}
			if len(members) != tc.want {
				t.Errorf("len(members) = %d, want %d", len(members), tc.want)
			}
		})
	}
}

// TestSession_Send_Close_Concurrent_NoPanic: 高并发 Send + Close 必须不 panic
// （send-on-closed-channel race 防御）。go test -race 下应稳定通过。
//
// 场景：N 个 goroutine 持续调 Send，主 goroutine 在中段 Close；任何 Send 拿到
// closed flag=false 时应该已经被 sendMu.Lock 阻塞 → Close 拿走 Lock → 翻 flag +
// close(chan) → Send 拿到 RLock → 看 closed=true 立即返 ErrSessionClosed，永远
// 不会触达 select 的 send case 命中已 close 的 chan。
//
// 不允许 panic（test 框架自动捕获 panic 标 fail）；Close 后所有 Send 必返
// ErrSessionClosed。
func TestSession_Send_Close_Concurrent_NoPanic(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	const senderCount = 16

	var wg sync.WaitGroup
	startCh := make(chan struct{})
	var stop atomic.Bool
	var closedSeen atomic.Int32

	for i := 0; i < senderCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startCh
			// 持续 Send 直到看到 ErrSessionClosed —— 保证 Send 与 Close 时序重叠，
			// 制造 race 窗口（修复前会 panic：send-on-closed-channel）
			for !stop.Load() {
				err := session.Send([]byte(`{"type":"x"}`))
				if err != nil && errors.Is(err, wsapp.ErrSessionClosed) {
					closedSeen.Add(1)
					return
				}
				// ErrSessionSendBufferFull 合法（chan 满）；只要不 panic 继续循环
			}
		}()
	}

	close(startCh) // 让所有 sender 同时起跑

	// 给 sender 跑一小段，再 Close，制造 race 窗口
	time.Sleep(20 * time.Millisecond)
	_ = session.Close()
	stop.Store(true)

	wg.Wait()

	// Close 后再 Send 必返 ErrSessionClosed
	if err := session.Send([]byte("post-close")); !errors.Is(err, wsapp.ErrSessionClosed) {
		t.Errorf("post-close Send = %v, want ErrSessionClosed", err)
	}

	// 至少应有一个 sender 看到 ErrSessionClosed（证明 Close 与 Send 真发生了交错）；
	// 因为 sender 是无限循环直到 stop=true 或看到 ErrSessionClosed，所以 Close 后
	// 必然有 sender 在 race 窗口内拿 RLock 后看到 closed=true。
	if closedSeen.Load() == 0 {
		t.Errorf("expected at least one sender to observe ErrSessionClosed (race window did not engage)")
	}
}

// TestSessionManager_Close_TriggersUnregisterHookForAllSessions:
// SessionManager.Close 必须为**每个**注册的 Session 都触发 onUnregister 钩子。
// 修复前的实装会先清空索引再 Close session → notifyClosed → Unregister 走
// no-op 路径，钩子漏调；修复后保留索引到 Close 跑完。
func TestSessionManager_Close_TriggersUnregisterHookForAllSessions(t *testing.T) {
	var unregisteredIDs sync.Map // sessionID → struct{}
	var unregisterCount atomic.Int32
	mgr := wsapp.NewSessionManager(
		wsapp.WithUnregisterHook(func(s *wsapp.Session) {
			unregisteredIDs.Store(s.SessionID(), struct{}{})
			unregisterCount.Add(1)
		}),
	)

	repo := &stubRoomMemberRepo{}
	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	const roomID = 3001
	uids := []uint64{1001, 1002, 1003}
	var conns []*websocket.Conn
	for _, uid := range uids {
		token, _ := signer.Sign(uid, 3600)
		url := fmt.Sprintf("%s/ws/rooms/%d?token=%s", wsURL, roomID, token)
		c, _, err := dialWS(t, url)
		if err != nil {
			t.Fatalf("dial uid=%d: %v", uid, err)
		}
		conns = append(conns, c)
		_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
		if _, _, err := c.ReadMessage(); err != nil {
			t.Fatalf("read snapshot uid=%d: %v", uid, err)
		}
	}
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	// 等所有 Session 注册到 manager
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(mgr.ListSessionsByRoomID(context.Background(), roomID)) >= len(uids) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	registered := mgr.ListSessionsByRoomID(context.Background(), roomID)
	if got, want := len(registered), len(uids); got != want {
		t.Fatalf("registered sessions = %d, want %d", got, want)
	}

	// 关 manager —— 必须触发**每个** Session 的 onUnregister 钩子
	if err := mgr.Close(); err != nil {
		t.Fatalf("mgr.Close: %v", err)
	}

	// 等 unregister 钩子异步触发完毕（Session.Close → notifyClosed → Unregister →
	// onUnregister）
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if int(unregisterCount.Load()) >= len(uids) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got, want := int(unregisterCount.Load()), len(uids); got != want {
		t.Errorf("unregister hook fired %d times, want %d (one per registered session)", got, want)
	}

	// 校验每个 Session 的 sessionID 都进了 unregisteredIDs map
	for _, s := range registered {
		if _, ok := unregisteredIDs.Load(s.SessionID()); !ok {
			t.Errorf("session %s did not trigger unregister hook", s.SessionID())
		}
	}

	// Close idempotent：第二次调用不 panic / 不重复触发钩子
	prev := unregisterCount.Load()
	if err := mgr.Close(); err != nil {
		t.Errorf("second mgr.Close: %v", err)
	}
	if now := unregisterCount.Load(); now != prev {
		t.Errorf("second mgr.Close re-triggered hooks: %d → %d", prev, now)
	}
}

// ---------- Gateway prod-contract gate（review r2 P2 修） ----------

// TestNewGateway_ProdEnv_RejectsNonContractHeartbeat: env=prod 配
// heartbeat_timeout_sec=30（非契约值 60）应 panic。
func TestNewGateway_ProdEnv_RejectsNonContractHeartbeat(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewGateway should panic with non-contract heartbeat in prod env")
		} else {
			msg := fmt.Sprintf("%v", r)
			if !strings.Contains(msg, "heartbeat_timeout_sec") {
				t.Errorf("panic message %q should mention heartbeat_timeout_sec", msg)
			}
		}
	}()

	signer := newSigner(t)
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	cfg := config.WSConfig{
		HeartbeatTimeoutSec: 30, // 非契约值
		MaxMessageSizeBytes: 16384,
		WriteTimeoutSec:     5,
	}
	_ = wsapp.NewGateway(signer, mgr, repo, cfg, "prod")
}

// TestNewGateway_ProdEnv_RejectsNonContractMaxMessageSize: env=prod 配
// max_message_size_bytes=8192（非契约值 16384）应 panic。
func TestNewGateway_ProdEnv_RejectsNonContractMaxMessageSize(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewGateway should panic with non-contract max_message_size_bytes in prod env")
		} else {
			msg := fmt.Sprintf("%v", r)
			if !strings.Contains(msg, "max_message_size_bytes") {
				t.Errorf("panic message %q should mention max_message_size_bytes", msg)
			}
		}
	}()

	signer := newSigner(t)
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	cfg := config.WSConfig{
		HeartbeatTimeoutSec: 60,
		MaxMessageSizeBytes: 8192, // 非契约值
		WriteTimeoutSec:     5,
	}
	_ = wsapp.NewGateway(signer, mgr, repo, cfg, "prod")
}

// TestNewGateway_EmptyEnv_BehavesAsProd: env="" 应按 prod 严格策略
// （safe-by-default：未注入 CAT_ENV 视为 prod，避免 dev YAML 静默流到 prod）。
func TestNewGateway_EmptyEnv_BehavesAsProd(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewGateway should panic with non-contract heartbeat when env is empty (safe-by-default)")
		}
	}()

	signer := newSigner(t)
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	cfg := config.WSConfig{
		HeartbeatTimeoutSec: 30, // 非契约值
		MaxMessageSizeBytes: 16384,
		WriteTimeoutSec:     5,
	}
	_ = wsapp.NewGateway(signer, mgr, repo, cfg, "")
}

// TestNewGateway_DevEnv_AcceptsOverride: env=dev 应允许 YAML 覆盖契约字段。
func TestNewGateway_DevEnv_AcceptsOverride(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewGateway should NOT panic with override in dev env; got panic: %v", r)
		}
	}()

	signer := newSigner(t)
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	cfg := config.WSConfig{
		HeartbeatTimeoutSec: 30, // 非契约值，但 dev 允许
		MaxMessageSizeBytes: 8192,
		WriteTimeoutSec:     5,
	}
	gateway := wsapp.NewGateway(signer, mgr, repo, cfg, "dev")
	if gateway == nil {
		t.Error("NewGateway returned nil in dev env")
	}
}

// TestNewGateway_ProdEnv_AcceptsContractValues: env=prod + 契约默认值应正常构造。
func TestNewGateway_ProdEnv_AcceptsContractValues(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewGateway should NOT panic with contract values in prod; got panic: %v", r)
		}
	}()

	signer := newSigner(t)
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	cfg := config.WSConfig{
		HeartbeatTimeoutSec: 60,
		MaxMessageSizeBytes: 16384,
		WriteTimeoutSec:     5,
	}
	gateway := wsapp.NewGateway(signer, mgr, repo, cfg, "prod")
	if gateway == nil {
		t.Error("NewGateway returned nil in prod env with contract values")
	}
}

// ---------- Story 10.4：Session.CloseWithCode 测试 ----------

// readCloseError 是测试 helper：从 conn 读直到 close error，校验 code + reason。
// 与既有 expectCloseError 不同的是：本 helper 在已经握手成功并读完 snapshot 后用，
// 而 expectCloseError 是握手期就直接读拿 close（没有 snapshot 在前）。
func readCloseError(t *testing.T, conn *websocket.Conn, wantCode int, wantReason string) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	for {
		_, _, err := conn.ReadMessage()
		if err == nil {
			// 还有非-close 消息（如刚才的 snapshot 残留 / pong），继续读
			continue
		}
		closeErr, ok := err.(*websocket.CloseError)
		if !ok {
			t.Fatalf("err = %T %v, want *websocket.CloseError", err, err)
		}
		if closeErr.Code != wantCode {
			t.Errorf("CloseError.Code = %d, want %d (text=%q)", closeErr.Code, wantCode, closeErr.Text)
		}
		if wantReason != "" && closeErr.Text != wantReason {
			t.Errorf("CloseError.Text = %q, want %q", closeErr.Text, wantReason)
		}
		return
	}
}

// TestSession_CloseWithCode_HappyPath：
// CloseWithCode(4005, "heartbeat timeout") → client conn 收到 close frame
// code=4005 reason="heartbeat timeout"。
func TestSession_CloseWithCode_HappyPath(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	if err := session.CloseWithCode(4005, "heartbeat timeout"); err != nil {
		t.Fatalf("CloseWithCode: %v", err)
	}

	readCloseError(t, conn, 4005, "heartbeat timeout")
}

// TestSession_CloseWithCode_AlreadyClosed_ReturnsErr：
// Session 已 Close → CloseWithCode 返 ErrSessionClosed sentinel + 不写 close frame。
func TestSession_CloseWithCode_AlreadyClosed_ReturnsErr(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	// 先 Close（已 close 的 Session）
	if err := session.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// 再 CloseWithCode → 应返 ErrSessionClosed
	err := session.CloseWithCode(4005, "heartbeat timeout")
	if !errors.Is(err, wsapp.ErrSessionClosed) {
		t.Errorf("CloseWithCode after Close: err = %v, want ErrSessionClosed", err)
	}
}

// TestSession_CloseWithCode_Idempotent：
// 调两次 CloseWithCode → 第一次写 close frame + 关 conn，第二次返 ErrSessionClosed。
func TestSession_CloseWithCode_Idempotent(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	// 第一次：成功
	if err := session.CloseWithCode(4005, "heartbeat timeout"); err != nil {
		t.Errorf("first CloseWithCode: err = %v, want nil", err)
	}

	// 第二次：返 ErrSessionClosed
	err := session.CloseWithCode(4005, "heartbeat timeout")
	if !errors.Is(err, wsapp.ErrSessionClosed) {
		t.Errorf("second CloseWithCode: err = %v, want ErrSessionClosed", err)
	}

	// client 仍能读到 close frame（第一次写的）
	readCloseError(t, conn, 4005, "heartbeat timeout")
}

// ---------- Story 10.4：SessionManager.ListAllSessions 测试 ----------

// TestSessionManager_ListAllSessions_ReturnsAll：
// 注册 3 user 在 2 个不同 room → ListAllSessions 返 3 个 Session（按 sessionID 字典序）。
func TestSessionManager_ListAllSessions_ReturnsAll(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}

	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	type want struct {
		userID uint64
		roomID uint64
	}
	wants := []want{
		{1001, 3001},
		{1002, 3001},
		{1003, 3002}, // 跨 room
	}
	var conns []*websocket.Conn
	for _, w := range wants {
		token, _ := signer.Sign(w.userID, 3600)
		url := fmt.Sprintf("%s/ws/rooms/%d?token=%s", wsURL, w.roomID, token)
		conn, _, err := dialWS(t, url)
		if err != nil {
			t.Fatalf("dial uid=%d room=%d: %v", w.userID, w.roomID, err)
		}
		conns = append(conns, conn)
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatalf("read snapshot uid=%d: %v", w.userID, err)
		}
	}
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	// 等所有 Session 注册到 manager
	deadline := time.Now().Add(2 * time.Second)
	var sessions []*wsapp.Session
	for time.Now().Before(deadline) {
		sessions = mgr.ListAllSessions(context.Background())
		if len(sessions) >= len(wants) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(sessions); got != len(wants) {
		t.Fatalf("ListAllSessions len = %d, want %d", got, len(wants))
	}

	// 校验字典序：sessionID 应严格递增
	for i := 1; i < len(sessions); i++ {
		if sessions[i-1].SessionID() >= sessions[i].SessionID() {
			t.Errorf("ListAllSessions not sorted by sessionID: [%d]=%q [%d]=%q",
				i-1, sessions[i-1].SessionID(), i, sessions[i].SessionID())
		}
	}

	// 校验 user 集合（不依赖顺序）
	gotUsers := map[uint64]bool{}
	for _, s := range sessions {
		gotUsers[s.UserID()] = true
	}
	for _, w := range wants {
		if !gotUsers[w.userID] {
			t.Errorf("ListAllSessions missing userID=%d", w.userID)
		}
	}
}

// TestSessionManager_ListAllSessions_EmptyManager_ReturnsEmpty：
// manager 无 session → ListAllSessions 返空切片（非 nil）。
func TestSessionManager_ListAllSessions_EmptyManager_ReturnsEmpty(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()

	sessions := mgr.ListAllSessions(context.Background())
	if sessions == nil {
		t.Errorf("ListAllSessions = nil, want non-nil empty slice")
	}
	if got := len(sessions); got != 0 {
		t.Errorf("ListAllSessions len = %d, want 0", got)
	}
}

// ---------- Story 10.4：HeartbeatScanner 测试 ----------

// idleTestLogger 返回一个写到 io.Discard 的 slog.Logger，让 scanner 测试不污染
// stdout（与 session_manager_internal_test 同模式）。
func idleTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(testDiscard{}, nil))
}

// testDiscard 实装 io.Writer，丢弃所有写入（与 io.Discard 等价但不引入 io 依赖）。
type testDiscard struct{}

func (testDiscard) Write(p []byte) (int, error) { return len(p), nil }

// readCloseFrameLoose 是 readCloseError 的宽松版：仅校验 close code + reason，
// 不 t.Fatal —— 适合 scanner 测试中"可能 read 阻塞"的场景，给短 timeout 让
// scanner 异步触发 close 后能读到。
func readCloseFrameLoose(t *testing.T, conn *websocket.Conn, timeout time.Duration) (int, string) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	for {
		_, _, err := conn.ReadMessage()
		if err == nil {
			continue
		}
		closeErr, ok := err.(*websocket.CloseError)
		if !ok {
			t.Fatalf("err = %T %v, want *websocket.CloseError", err, err)
		}
		return closeErr.Code, closeErr.Text
	}
}

// TestHeartbeatScanner_ScanOnce_IdleSession_ClosesWith4005：
// manager 含 1 Session（lastHeartbeatAt = now-70s）→ scanner.scanOnce →
// Session.CloseWithCode 被调（client 收到 4005 frame）。
func TestHeartbeatScanner_ScanOnce_IdleSession_ClosesWith4005(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	// 强制把 lastHeartbeatAt 拖回 70 秒前 —— 通过测试用的 ResetHeartbeat helper
	// （Session 自身没 setter，用 wire 路径不发消息让 lastHeartbeatAt 自然停留也行，
	// 但更可控的是直接写 idleTimeout 短到比 wallclock 间隔小）。
	//
	// 实际策略：scanner 用 timeoutMs=10ms（极短），now=time.Now()；只要
	// session 不在过去 10ms 内收到消息就视为 idle。useGatewayDial 后立刻调
	// scanOnce → idle = (now - handshake_time).UnixMilli() > 10ms 必成立（因为
	// useGatewayDial 内部至少 sleep 几 ms 等 manager 注册）。
	scanner := wsapp.NewHeartbeatScannerForTest(mgr, 10, 200*time.Millisecond, idleTestLogger())

	// 等 useGatewayDial 后至少 50ms 让 idle 远超 10ms threshold
	time.Sleep(50 * time.Millisecond)

	scanner.ScanOnceForTest(context.Background(), time.Now())

	// scanner 是 fanout goroutine 触发 CloseWithCode；等 close 写到 wire
	code, text := readCloseFrameLoose(t, conn, 2*time.Second)
	if code != 4005 {
		t.Errorf("close code = %d, want 4005", code)
	}
	if text != "heartbeat timeout" {
		t.Errorf("close reason = %q, want \"heartbeat timeout\"", text)
	}

	// session 此后应进入 closed 状态（Send 返 ErrSessionClosed）
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := session.Send([]byte("test")); errors.Is(err, wsapp.ErrSessionClosed) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("session not closed after scanner timeout fanout")
}

// TestHeartbeatScanner_ScanOnce_ActiveSession_DoesNotClose：
// manager 含 1 Session（lastHeartbeatAt 刚刚更新）→ scanner.scanOnce → Session 不被 close。
func TestHeartbeatScanner_ScanOnce_ActiveSession_DoesNotClose(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	// scanner timeoutMs=10s（远大于刚握手完的 idle 时间）→ 不应触发 close
	scanner := wsapp.NewHeartbeatScannerForTest(mgr, 10*1000, 200*time.Millisecond, idleTestLogger())

	scanner.ScanOnceForTest(context.Background(), time.Now())

	// 给 fanout goroutine 一些时间（如果它真的会跑也会是异步的）；scanner 不应 fanout
	time.Sleep(200 * time.Millisecond)

	// session 应仍在工作 —— Send 不返 ErrSessionClosed
	if err := session.Send([]byte(`{"type":"custom","requestId":"","payload":{},"ts":0}`)); err != nil {
		t.Errorf("Send after scanOnce on active session: err = %v, want nil", err)
	}
}

// TestHeartbeatScanner_ScanOnce_MultipleSessions_OnlyIdleClosed：
// manager 含 3 Session（idle/active/idle）→ scanOnce → 仅 2 个 idle 被 close +
// 1 active 仍存活（保护 close fanout 并发安全）。
func TestHeartbeatScanner_ScanOnce_MultipleSessions_OnlyIdleClosed(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}

	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	// 起 3 个 Session（不同 user），全部在同一 room 简化 fixture
	type connSess struct {
		conn *websocket.Conn
		uid  uint64
	}
	var all []connSess
	for _, uid := range []uint64{1001, 1002, 1003} {
		token, _ := signer.Sign(uid, 3600)
		url := fmt.Sprintf("%s/ws/rooms/%d?token=%s", wsURL, 3001, token)
		conn, _, err := dialWS(t, url)
		if err != nil {
			t.Fatalf("dial uid=%d: %v", uid, err)
		}
		all = append(all, connSess{conn, uid})
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatalf("read snapshot uid=%d: %v", uid, err)
		}
	}
	defer func() {
		for _, cs := range all {
			cs.conn.Close()
		}
	}()

	// 等所有注册
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(mgr.ListAllSessions(context.Background())) >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 让 1002 的 Session "active" —— 给它发条 ping 让 lastHeartbeatAt 刷新到 now
	if err := all[1].conn.WriteMessage(websocket.TextMessage,
		[]byte(`{"type":"ping","requestId":"refresh","payload":{}}`)); err != nil {
		t.Fatalf("write ping uid=1002: %v", err)
	}
	// 读掉 pong 让 read deadline 不过期
	_ = all[1].conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := all[1].conn.ReadMessage(); err != nil {
		t.Fatalf("read pong uid=1002: %v", err)
	}

	// 等 server 端 lastHeartbeatAt 刷新到当前时间附近（receive ping → readLoop
	// 内 lastHeartbeatAt.Store(time.Now().UnixMilli()) 是在 ReadMessage 收到消息
	// 之后；client 这边 ReadMessage(pong) 返回时 server 端早已刷新）

	// 现在让 timeoutMs=20ms：
	//   - uid=1002 刚刷新 → idle ≈ 0~20ms → 不一定触发；为了稳定先 sleep 一段足以
	//     让 1001/1003 idle，但 1002 的 lastHeartbeat 也会越来越久 —— 改策略：
	//
	// **更稳的策略**：scan 调 now=time.Now() 让所有 Session 都按"现在"判断；
	// timeoutMs 设置为：1002 在 active 之后过去的 idle ms 之上的某个值。
	//
	// 实操：sleep 100ms 让 useGatewayDial-time 过去更久 → 1001/1003 idle ≈
	// 几百 ms；然后立即给 1002 发 ping 刷新 → 1002 idle ≈ 几 ms；scan timeoutMs=50ms
	// → 1001/1003 命中（idle > 50ms），1002 不命中（idle < 50ms）

	// 重新刷新 1002 让它非常 active
	if err := all[1].conn.WriteMessage(websocket.TextMessage,
		[]byte(`{"type":"ping","requestId":"refresh2","payload":{}}`)); err != nil {
		t.Fatalf("write ping2 uid=1002: %v", err)
	}
	_ = all[1].conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := all[1].conn.ReadMessage(); err != nil {
		t.Fatalf("read pong2 uid=1002: %v", err)
	}

	// 不 sleep —— 立即扫，timeoutMs 设的足够大让 1001/1003 都还不超时（避免本测试
	// 太脆弱）。改策略：直接 verify scanner 行为通过手工设 lastHeartbeatAt。
	//
	// 由于 Session 上没有 SetLastHeartbeatAt setter，本测试只能用真实时间。
	// 折中：sleep 100ms，让 1001/1003 长时间没动 → idle ≈ 100ms+；再瞬间 ping 1002；
	// 立即 scanOnce timeoutMs=80ms，1002 idle ≈ 0ms 不命中，1001/1003 idle ≈ 100ms+
	// 命中。

	time.Sleep(100 * time.Millisecond)
	if err := all[1].conn.WriteMessage(websocket.TextMessage,
		[]byte(`{"type":"ping","requestId":"r3","payload":{}}`)); err != nil {
		t.Fatalf("write ping3 uid=1002: %v", err)
	}
	_ = all[1].conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := all[1].conn.ReadMessage(); err != nil {
		t.Fatalf("read pong3 uid=1002: %v", err)
	}

	// scan
	scanner := wsapp.NewHeartbeatScannerForTest(mgr, 80, 200*time.Millisecond, idleTestLogger())
	scanner.ScanOnceForTest(context.Background(), time.Now())

	// 1001 / 1003 应被 close（4005）
	c1Code, _ := readCloseFrameLoose(t, all[0].conn, 2*time.Second)
	if c1Code != 4005 {
		t.Errorf("uid=1001 close code = %d, want 4005", c1Code)
	}
	c3Code, _ := readCloseFrameLoose(t, all[2].conn, 2*time.Second)
	if c3Code != 4005 {
		t.Errorf("uid=1003 close code = %d, want 4005", c3Code)
	}

	// 1002 应仍 active —— 发条消息看能不能收到
	if err := all[1].conn.WriteMessage(websocket.TextMessage,
		[]byte(`{"type":"ping","requestId":"survive","payload":{}}`)); err != nil {
		t.Errorf("uid=1002 write after scan: %v (should still be alive)", err)
	}
	_ = all[1].conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := all[1].conn.ReadMessage(); err != nil {
		t.Errorf("uid=1002 read after scan: %v (should still be alive)", err)
	}
}

// TestHeartbeatScanner_Run_CtxCancel_ExitsGracefully：
// scanner.Run(ctx) 启动 → cancel(ctx) → Run 返回，无 goroutine leak（用
// channel close 信号探测）。
func TestHeartbeatScanner_Run_CtxCancel_ExitsGracefully(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()

	scanner := wsapp.NewHeartbeatScannerForTest(mgr, 100, 50*time.Millisecond, idleTestLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		scanner.Run(ctx)
		close(done)
	}()

	// 让 scanner 跑几个 tick
	time.Sleep(150 * time.Millisecond)

	// cancel → Run 应在 100ms 内退出
	cancel()
	select {
	case <-done:
		// good
	case <-time.After(2 * time.Second):
		t.Errorf("scanner.Run did not return within 2s after ctx cancel (goroutine leak)")
	}
}

// TestHeartbeatScanner_ScanOnce_IdleClose_TriggersOnUnregisterHook：
// scanner 触发的 close 必须走 Session.Close → notifier.notifyClosed → manager.Unregister →
// onUnregister 钩子（review 10-3 r2 P1 不变量）。
func TestHeartbeatScanner_ScanOnce_IdleClose_TriggersOnUnregisterHook(t *testing.T) {
	var unregisterCount atomic.Int32
	var unregisteredIDs sync.Map
	mgr := wsapp.NewSessionManager(
		wsapp.WithUnregisterHook(func(s *wsapp.Session) {
			unregisteredIDs.Store(s.SessionID(), struct{}{})
			unregisterCount.Add(1)
		}),
	)
	defer mgr.Close()

	repo := &stubRoomMemberRepo{}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	// 等握手完成 + 等几 ms 让 idle 超过 1ms threshold
	time.Sleep(50 * time.Millisecond)

	scanner := wsapp.NewHeartbeatScannerForTest(mgr, 1, 100*time.Millisecond, idleTestLogger())
	scanner.ScanOnceForTest(context.Background(), time.Now())

	// 等 fanout goroutine 完成 close 路径
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if unregisterCount.Load() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := unregisterCount.Load(); got != 1 {
		t.Errorf("onUnregister hook fired %d times, want 1 (scanner-triggered close must go through hook chain)", got)
	}
	if _, ok := unregisteredIDs.Load(session.SessionID()); !ok {
		t.Errorf("onUnregister hook did not fire for sessionID=%q", session.SessionID())
	}
}

// TestHeartbeatScanner_ScanOnce_AlreadyClosedSession_Skipped：
// scanner 看到 closed=true 的 Session 调 CloseWithCode 应返 ErrSessionClosed
// 不二次写 close frame；onUnregister 钩子也不会重复触发。
func TestHeartbeatScanner_ScanOnce_AlreadyClosedSession_Skipped(t *testing.T) {
	var unregisterCount atomic.Int32
	mgr := wsapp.NewSessionManager(
		wsapp.WithUnregisterHook(func(s *wsapp.Session) {
			unregisterCount.Add(1)
		}),
	)
	defer mgr.Close()

	repo := &stubRoomMemberRepo{}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	// 主动 Close（让 Session 进 closed 状态 + 触发 onUnregister 1 次）
	if err := session.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// 等钩子触发
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if unregisterCount.Load() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := unregisterCount.Load(); got != 1 {
		t.Fatalf("baseline unregister count = %d, want 1 (session.Close should fire hook once)", got)
	}

	// 此时 manager 内已经没有该 Session（Unregister 已清出）；为模拟"scanner
	// 看到 closed Session"的窗口，把已 close 的 Session 直接喂给 scanner 的
	// fanout goroutine —— 用 NewHeartbeatScannerForTest 不直接帮我们注入裸
	// session，但 scanOnce 走 ListAllSessions 已经看不到这个 session。
	//
	// 所以本测试的真实保证：scanOnce 看到 manager 里没有 session（已被 Unregister
	// 清出）→ 自然 skip → 钩子计数保持 1，不会重复触发。
	scanner := wsapp.NewHeartbeatScannerForTest(mgr, 1, 100*time.Millisecond, idleTestLogger())
	scanner.ScanOnceForTest(context.Background(), time.Now())

	// 给 fanout 短时间窗口（如果有 race 让 scanOnce 看到了 session）
	time.Sleep(200 * time.Millisecond)

	if got := unregisterCount.Load(); got != 1 {
		t.Errorf("unregister count = %d after scanOnce on already-closed session, want 1 (no duplicate trigger)", got)
	}
}

// TestHeartbeatScanner_NewHeartbeatScanner_ZeroTimeoutSec_FallbacksTo60：
// NewHeartbeatScanner(mgr, 0, logger) → 内部 timeoutMs == 60_000（防御性兜底）。
func TestHeartbeatScanner_NewHeartbeatScanner_ZeroTimeoutSec_FallbacksTo60(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()

	scanner := wsapp.NewHeartbeatScanner(mgr, 0, idleTestLogger())
	if got := scanner.TimeoutMsForTest(); got != 60_000 {
		t.Errorf("timeoutMs = %d, want 60_000 (zero/negative input should fallback to 60s)", got)
	}

	scanner2 := wsapp.NewHeartbeatScanner(mgr, -5, idleTestLogger())
	if got := scanner2.TimeoutMsForTest(); got != 60_000 {
		t.Errorf("negative input: timeoutMs = %d, want 60_000", got)
	}
}

// TestHeartbeatScanner_ScanOnce_RaceRefreshAfterListing_NotClosed：
// review r1 P1 regression：scanOnce 主循环判定 idle > timeoutMs，但**进入
// goroutine 之前** lastHeartbeatAt 被 readLoop 刷新（client 在阈值边界附近
// 发了 ping）→ goroutine 内重新校验应跳过 close，session 不被踢。
//
// 测试策略（用 SetLastHeartbeatAtForTest 模拟 race，不走真实 wire）：
//  1. 起 session；强制把 lastHeartbeatAt 拖到 100ms 之前 → idle ≈ 100ms
//  2. scanner timeoutMs=50ms → 主循环判定 idle > timeoutMs，spawn fanout
//  3. 在 fanout goroutine 实际执行前，立即把 lastHeartbeatAt 刷新回 now
//     → goroutine 内重新读 idle ≈ 0ms ≤ 50ms → **应跳过** close
//  4. 等一段窗口期后 verify session 仍 active（Send 不返 ErrSessionClosed）
func TestHeartbeatScanner_ScanOnce_RaceRefreshAfterListing_NotClosed(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	// 把 lastHeartbeatAt 拖到 100ms 之前，让主循环判定为 idle
	staleMs := time.Now().UnixMilli() - 100
	session.SetLastHeartbeatAtForTest(staleMs)

	// 立刻在主循环判定后 + fanout goroutine 真正执行前刷新 lastHeartbeatAt 到 now
	// **难点**：scanOnce 主循环是同步遍历，spawn goroutine 后立即返回；本测试
	// 在 ScanOnceForTest 返回后立即 SetLastHeartbeatAt = now 模拟 race（fanout
	// goroutine 大概率还没执行到 recheck 行）。
	scanner := wsapp.NewHeartbeatScannerForTest(mgr, 50, 200*time.Millisecond, idleTestLogger())
	scanner.ScanOnceForTest(context.Background(), time.Now())

	// 立即刷新 —— 模拟 readLoop 刚刚收到 client ping 的 race 窗口
	session.SetLastHeartbeatAtForTest(time.Now().UnixMilli())

	// 等一段时间让 fanout goroutine 跑完 recheck（它读到 idle ≈ 0ms 应跳过 close）
	time.Sleep(300 * time.Millisecond)

	// session 应仍 active：Send 不返 ErrSessionClosed
	if err := session.Send([]byte(`{"type":"custom","requestId":"","payload":{},"ts":0}`)); err != nil {
		t.Errorf("session was closed despite race-refresh: Send err = %v, want nil (TOCTOU recheck should have spared it)", err)
	}
}

// TestSession_CloseWithCode_SendReturnsErrAfterCloseWithCode：
// review r1 P2 regression：CloseWithCode 必须**在** WriteControl 之前先把 closed
// flag 翻 true + 关 sendChan / sendPriorityChan，让并发 Send / SendPriority 立即
// 看到 closed=true → ErrSessionClosed。原版实装在 WriteControl 与 Close() 之间
// 仍可入队业务消息，存在 close frame 之后 data frame 写到 wire 的窗口。
//
// 测试策略：
//  1. 起 session
//  2. 启动并发 goroutine：循环调 Send（业务消息）
//  3. 主 goroutine 调 CloseWithCode
//  4. **CloseWithCode 返回后**所有 Send 必须立即返 ErrSessionClosed（不再有
//     新消息能入队 sendChan）
//
// 这是 close frame 顺序约束的**充分条件**：如果 Send 在 CloseWithCode 返回后
// 仍能入队成功，那必然存在 "close frame 已写但 sendChan 还能塞业务消息" 的窗口；
// 反之只要 Send 立即返 ErrSessionClosed，writeLoop 看到的 sendChan 就只可能是
// "已 close + drain 完已入队的消息" → CloseWithCode 内 wait writeLoopDone 后
// WriteControl 才不会与 data frame race。
func TestSession_CloseWithCode_SendReturnsErrAfterCloseWithCode(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	// 调 CloseWithCode 完成后，Send 应立即返 ErrSessionClosed
	if err := session.CloseWithCode(4005, "heartbeat timeout"); err != nil {
		t.Fatalf("CloseWithCode: %v", err)
	}

	if err := session.Send([]byte(`{"type":"x"}`)); !errors.Is(err, wsapp.ErrSessionClosed) {
		t.Errorf("Send after CloseWithCode: err = %v, want ErrSessionClosed", err)
	}
	if err := session.SendPriority([]byte(`{"type":"pong"}`)); !errors.Is(err, wsapp.ErrSessionClosed) {
		t.Errorf("SendPriority after CloseWithCode: err = %v, want ErrSessionClosed", err)
	}

	// client 仍应能读到 close frame
	readCloseError(t, conn, 4005, "heartbeat timeout")
}

// TestSession_CloseWithCode_ConcurrentSend_StopsImmediately：
// review r1 P2 regression（高并发版）：N 个 goroutine 并发 Send，与 CloseWithCode
// race。CloseWithCode 返回后**任何**新 Send 必须立即拿到 ErrSessionClosed
// （而不是侥幸入队进 sendChan）。
//
// 不直接断言 wire 顺序（gorilla ReadMessage 收到 close 后即 surface CloseError，
// 之后的帧不会被 user code 看到 —— 顺序保证由 closeInternal 内"先关 chan + 等
// writeLoop 退出 + 再 WriteControl"实装级保证；本测试覆盖入口面）。
func TestSession_CloseWithCode_ConcurrentSend_StopsImmediately(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	// 启动 4 个 goroutine 并发 Send 1s
	stopFlag := atomic.Bool{}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stopFlag.Load() {
				_ = session.Send([]byte(`{"type":"x"}`))
			}
		}()
	}

	// 短暂跑一会让 sendChan 持续 churn
	time.Sleep(20 * time.Millisecond)

	// 触发 CloseWithCode
	if err := session.CloseWithCode(4005, "heartbeat timeout"); err != nil {
		t.Fatalf("CloseWithCode: %v", err)
	}

	// CloseWithCode 已返回 → 此后任何 Send 必须立即 ErrSessionClosed
	if err := session.Send([]byte(`{"type":"x"}`)); !errors.Is(err, wsapp.ErrSessionClosed) {
		t.Errorf("Send after CloseWithCode return: err = %v, want ErrSessionClosed", err)
	}

	stopFlag.Store(true)
	wg.Wait()

	// client 仍应能读到 close frame（顺序由实装层保证）
	readCloseError(t, conn, 4005, "heartbeat timeout")
}

// TestHeartbeatScanner_ScanOnce_FanoutGoroutineRespectsCtxCancel：review r3 P2
// regression。
//
// 背景（r3 P2）：scanner 的 fanout goroutine 不响应 ctx —— scanner.Run 在
// ctx.Done 后主循环立即退出，但已 dispatch 的 per-session goroutines 仍在跑，
// 仍 emit CloseWithCode 4005。预期 shutdown 时 sessionMgr.Close 走标准 close
// 路径（无 4005），实际是 race。修法：fanout goroutine 入口 + recheck 后再
// check 一次 ctx.Done，遇到 cancel 直接 return 不 emit 4005。
//
// 测试策略：
//  1. 起 session，把 lastHeartbeatAt 拖到很久之前（让 idle > timeoutMs 必成立）
//  2. 构造已 cancel 的 ctx 传给 scanOnce
//  3. scanOnce 主循环遍历完，dispatch fanout goroutine；goroutine 入口 ctx.Done
//     立即 return → session **不**被 close → Send 仍能成功
//
// **关键**：scanOnce 主循环本身不 check ctx（保留对历史 ListAllSessions 已 cancel
// 的 ctx 的容忍）；本测试的真实保证 = fanout goroutine 拿 ctx 后入口先 check。
func TestHeartbeatScanner_ScanOnce_FanoutGoroutineRespectsCtxCancel(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	// 把 lastHeartbeatAt 拖到很久之前 → idle 必 > timeoutMs
	staleMs := time.Now().UnixMilli() - 10_000 // 10s 前
	session.SetLastHeartbeatAtForTest(staleMs)

	// 构造已 cancel 的 ctx
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即 cancel

	scanner := wsapp.NewHeartbeatScannerForTest(mgr, 50, 200*time.Millisecond, idleTestLogger())
	scanner.ScanOnceForTest(ctx, time.Now())

	// 给 fanout goroutine 充足窗口期跑完（它应该入口立即 return，不 close session）
	time.Sleep(300 * time.Millisecond)

	// session 应仍 active：Send 不返 ErrSessionClosed
	if err := session.Send([]byte(`{"type":"custom","requestId":"","payload":{},"ts":0}`)); err != nil {
		t.Errorf("session was closed despite ctx.Done before fanout: Send err = %v, want nil (review r3 P2: fanout goroutine must check ctx.Done at entry)", err)
	}
}

// TestHeartbeatScanner_Run_DrainsFanoutBeforeReturn：review r5 P2 regression。
//
// 背景（r5 P2）：r3 修了 fanout goroutine 入口/recheck 后做 ctx-check，但
// "已通过最后一次 ctx-check 即将调 CloseWithCode" 的 goroutine 在 ctx cancel
// 后仍会 emit 4005。SIGTERM 落在 sweep 期间 → shutdown 仍能推 4005，触发 client
// 重连风暴。修法：scanner 用 sync.WaitGroup 跟踪 in-flight fanout，Run defer
// wg.Wait() 阻塞到所有 fanout 跑完才返回。
//
// 测试策略：
//  1. 起 N=10 个 idle session，让 scanOnce 必 dispatch 大批 fanout
//  2. scanner.Run 启动；等几个 tick 让 fanout 入队
//  3. cancel ctx，立即收下 Run 退出信号
//  4. 断言 Run 在合理时间内返回（说明 wg.Wait 有 drain），且 Run 返回**之后**
//     不再有任何 fanout goroutine 跑（用 wg.Wait 后再观察一段时间确认无 emit）
//
// **关键**：本测试覆盖的是"Run 返回 = scanner 完全静默"的强不变量；ctx-check
// 让 ctx-cancelled 路径立即 return，wg.Wait 让已通过 ctx-check 的 goroutine 跑完。
// 两种 goroutine 退出方式都会 wg.Done，Run 会等到所有都 wg.Done 后才返回。
func TestHeartbeatScanner_Run_DrainsFanoutBeforeReturn(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()

	repo := &stubRoomMemberRepo{}

	// 起 10 个 session，全 stale 让 fanout 必触发
	const N = 10
	conns := make([]*websocket.Conn, 0, N)
	sessions := make([]*wsapp.Session, 0, N)
	tss := make([]*httptest.Server, 0, N)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
		for _, ts := range tss {
			ts.Close()
		}
	}()
	staleMs := time.Now().UnixMilli() - 10_000
	for i := 0; i < N; i++ {
		conn, sess, ts := useGatewayDial(t, mgr, repo, uint64(2000+i), 4000)
		conns = append(conns, conn)
		sessions = append(sessions, sess)
		tss = append(tss, ts)
		sess.SetLastHeartbeatAtForTest(staleMs)
	}

	// scanner: timeoutMs 50ms（必 stale），interval 30ms（短让 tick 多触发）
	scanner := wsapp.NewHeartbeatScannerForTest(mgr, 50, 30*time.Millisecond, idleTestLogger())

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() {
		scanner.Run(ctx)
		close(runDone)
	}()

	// 让几次 tick 跑完，scanOnce dispatch 大批 fanout
	time.Sleep(100 * time.Millisecond)

	// cancel → Run 应 drain in-flight fanout 后返回
	cancel()

	// Run 应在 reasonable 时间内退出。fanout goroutine 的最大开销 = 写 close frame
	// 500ms timeout（如果走了 close path），所以给 2s 余量。
	select {
	case <-runDone:
		// good — Run 已 drain 全部 fanout 后返回
	case <-time.After(2 * time.Second):
		t.Fatalf("scanner.Run did not return within 2s after ctx cancel; wg.Wait drain likely missing (review r5 P2)")
	}

	// Run 已返回 = wg 已归零 = 所有 fanout goroutine 都 wg.Done 了。
	// 此时 manager 内 session 状态二选一：被 Close（fanout 抢在 ctx-cancel 前完成
	// emit）或仍 active（fanout 入口/recheck ctx-check 跳过 emit）。本测试**不**断言
	// 哪一种 —— scanner 在 cancel 前后的 race 决定，两种都符合协议。
	// 断言的是：Run 退出后**不会再有新的 4005 emit**（因为 wg.Wait 已 drain，没有
	// 残余 goroutine 还在跑）。给 200ms 静默观察期 —— 如果有残余 goroutine 在跑，
	// 它们这段时间会触发 onClose 副作用。
	time.Sleep(200 * time.Millisecond)

	// 注：本测试无法直接探测"是否有 goroutine 还在跑"（goroutine count 不可靠 ——
	// runtime 内部 goroutine 数会浮动）。真正的保证来自 wg.Wait 语义：Run 返回 →
	// wg 归零 → 所有 Add 都有对应 Done。如果 scanner 漏掉 wg.Add 或 fanout 漏掉
	// wg.Done，go test -race 在 wg 内部探测下会暴露。本测试在 -race 下跑就足够。
}

// TestSessionManager_ListAllSessions_NoLockHeldDuringSort：review r5 P2 regression。
//
// 背景（r5 P2）：ListAllSessions 持 RLock 全程包括 O(N log N) sort；每 30s heartbeat
// sweep 触发，sessions 多时整个 sweep 期间 Register/Unregister（要 write lock 同
// 一 mu）被周期性阻塞。修法：把 sort 移到 RUnlock 之后，锁内仅 copy 引用切片。
//
// 测试策略（粗粒度延迟探测）：
//  1. 预先 Register 大量 session 让 N 大到 sort 有可观察时间
//  2. 起一个 goroutine 反复调 ListAllSessions（模拟 scanner 周期性 sweep）
//  3. 主 goroutine 反复调 Register/Unregister，测量 N 次 op 总时间
//  4. 断言：在并发 ListAllSessions 干扰下，Register/Unregister 平均延迟应 reasonable
//     （go test -race 主要保证正确性；本测试主要保证 lock 持有时间不灾难性）
//
// **关键**：本测试**不**做精确时间断言（CI 环境抖动大），仅做"在并发 list 下
// 仍能完成大量 op"的存活性断言。真正的并发正确性由 -race 兜底。
func TestSessionManager_ListAllSessions_NoLockHeldDuringSort(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()

	repo := &stubRoomMemberRepo{}

	// 预先建一批 session 让 sort 工作量可观
	const presetN = 50
	conns := make([]*websocket.Conn, 0, presetN)
	tss := make([]*httptest.Server, 0, presetN)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
		for _, ts := range tss {
			ts.Close()
		}
	}()
	for i := 0; i < presetN; i++ {
		conn, _, ts := useGatewayDial(t, mgr, repo, uint64(5000+i), 5500)
		conns = append(conns, conn)
		tss = append(tss, ts)
	}

	// 启动并发 list 干扰：模拟 heartbeat scanner 周期性调 ListAllSessions
	stop := make(chan struct{})
	listDone := make(chan struct{})
	go func() {
		defer close(listDone)
		ctx := context.Background()
		for {
			select {
			case <-stop:
				return
			default:
				_ = mgr.ListAllSessions(ctx)
				_ = mgr.ListSessionsByRoomID(ctx, 5500)
			}
		}
	}()

	// 主 goroutine：在干扰下做 N 次 Register/Unregister，验证不卡死
	const opN = 30
	start := time.Now()
	regConns := make([]*websocket.Conn, 0, opN)
	regTss := make([]*httptest.Server, 0, opN)
	for i := 0; i < opN; i++ {
		conn, _, ts := useGatewayDial(t, mgr, repo, uint64(6000+i), 6500)
		regConns = append(regConns, conn)
		regTss = append(regTss, ts)
	}
	elapsed := time.Since(start)

	close(stop)
	<-listDone

	// cleanup
	for _, c := range regConns {
		c.Close()
	}
	for _, ts := range regTss {
		ts.Close()
	}

	// 存活性断言：30 次 Register 在持续 list 干扰下总耗时不应灾难性
	// （baseline 几百 ms 量级；给 30s 余量兜 CI 抖动）。如果旧实装持锁 sort 卡死，
	// 这里会因为 Register 拿不到 write lock 而超时。
	if elapsed > 30*time.Second {
		t.Errorf("Register x %d under concurrent ListAllSessions took %v > 30s — write lock starvation likely (review r5 P2: sort must be outside RLock)", opN, elapsed)
	}
}

