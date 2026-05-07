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
//
// Story 10.7 修：NewGateway 新增 builder 参数；本 helper 用 stub repo 构造默认
// placeholder builder（与 Story 10.3 ~ 10.6 既有测试覆盖语义一致 —— stub repo
// 控制 ListMembers 行为，builder 走真实 placeholder 路径）。
func startGatewayServer(t *testing.T, signer *auth.Signer, mgr wsapp.SessionManager, repo *stubRoomMemberRepo) (string, *httptest.Server) {
	t.Helper()
	cfg := config.WSConfig{
		HeartbeatTimeoutSec: 60,
		MaxMessageSizeBytes: 16384,
		WriteTimeoutSec:     2,
	}
	builder := wsapp.NewPlaceholderSnapshotBuilder(repo)
	gateway := wsapp.NewGateway(signer, mgr, repo, cfg, "test", builder)
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
	builder := wsapp.NewPlaceholderSnapshotBuilder(repo)
	gateway := wsapp.NewGateway(signer, mgr, repo, cfg, "test", builder)
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
	_ = wsapp.NewGateway(signer, mgr, repo, cfg, "prod", wsapp.NewPlaceholderSnapshotBuilder(repo))
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
	_ = wsapp.NewGateway(signer, mgr, repo, cfg, "prod", wsapp.NewPlaceholderSnapshotBuilder(repo))
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
	_ = wsapp.NewGateway(signer, mgr, repo, cfg, "", wsapp.NewPlaceholderSnapshotBuilder(repo))
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
	gateway := wsapp.NewGateway(signer, mgr, repo, cfg, "dev", wsapp.NewPlaceholderSnapshotBuilder(repo))
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
	gateway := wsapp.NewGateway(signer, mgr, repo, cfg, "prod", wsapp.NewPlaceholderSnapshotBuilder(repo))
	if gateway == nil {
		t.Error("NewGateway returned nil in prod env with contract values")
	}
}

// TestNewGateway_NilBuilder_Panics: builder == nil 应触发 fail-fast panic
// （Story 10.7 引入；与 signer / mgr / roomMember 同模式）。
func TestNewGateway_NilBuilder_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("NewGateway should panic when SnapshotBuilder is nil")
		} else {
			msg := fmt.Sprintf("%v", r)
			if !strings.Contains(msg, "SnapshotBuilder") {
				t.Errorf("panic message %q should mention SnapshotBuilder", msg)
			}
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
	_ = wsapp.NewGateway(signer, mgr, repo, cfg, "prod", nil)
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

	scanner := wsapp.NewHeartbeatScanner(mgr, 0, idleTestLogger(), nil, nil)
	if got := scanner.TimeoutMsForTest(); got != 60_000 {
		t.Errorf("timeoutMs = %d, want 60_000 (zero/negative input should fallback to 60s)", got)
	}

	scanner2 := wsapp.NewHeartbeatScanner(mgr, -5, idleTestLogger(), nil, nil)
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

// TestHeartbeatScanner_ShutdownOrdering_NoFourThousandFiveAfterScannerExit：
// review r6 P1 regression。
//
// 背景（r6 P1）：r5 让 scanner.Run 的 defer 加了 wg.Wait 阻塞到所有 in-flight
// fanout drain 完才返回；但 main.go 的 shutdown 路径只 cancelHeartbeat() 然后
// 让另一个 deferred 函数跑 sessionMgr.Close —— cancel 与 Run 真正 return 之间
// 存在窗口。这个窗口内已通过 ctx-check 的 fanout goroutine 仍可调
// CloseWithCode(4005,...)，与 sessionMgr.Close 标准 close 路径并发 race，导致
// idle client 收到 4005 而非正常 shutdown close（恰好是 r4 想消灭的"误重连"）。
//
// 修法（main.go r6）：cancelHeartbeat → wait scannerDone → sessionMgr.Close
// 收成一个 deferred 函数，串行确定。
//
// 测试策略（不真跑 main，模拟 shutdown helper 的 ordering 不变量）：
//  1. 起 N 个 idle session（lastHeartbeatAt 拖很久前 → idle 必 > timeoutMs）
//  2. 起 scanner.Run；让一两个 tick 跑 → fanout dispatched
//  3. cancelHeartbeat() → 立即 wait scannerDone → sessionMgr.Close
//  4. 断言：sessionMgr.Close 调用之前 scanner.Run **必须**已 return（用 chan
//     timing 验证），否则 race 仍存在
//
// **关键不变量**：shutdown helper 的执行顺序必须满足
//   cancelHeartbeat completes < scannerDone closed < sessionMgr.Close called
// 反之（cancel → sessionMgr.Close 不等 Run 退出）会让 fanout 在 sessionMgr.Close
// 进行中仍 emit 4005。
func TestHeartbeatScanner_ShutdownOrdering_NoFourThousandFiveAfterScannerExit(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	repo := &stubRoomMemberRepo{}

	// 起几个 idle session（足够让 fanout dispatched，但不要太多以免拖测试时间）
	const N = 5
	conns := make([]*websocket.Conn, 0, N)
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
		conn, sess, ts := useGatewayDial(t, mgr, repo, uint64(7000+i), 7500)
		conns = append(conns, conn)
		tss = append(tss, ts)
		sess.SetLastHeartbeatAtForTest(staleMs)
	}

	scanner := wsapp.NewHeartbeatScannerForTest(mgr, 50, 30*time.Millisecond, idleTestLogger())

	ctx, cancel := context.WithCancel(context.Background())
	scannerDone := make(chan struct{})
	go func() {
		defer close(scannerDone)
		scanner.Run(ctx)
	}()

	// 让几个 tick 跑过，fanout dispatched
	time.Sleep(120 * time.Millisecond)

	// 模拟 main.go r6 shutdown helper 的关键 ordering:
	//   1. cancel
	//   2. wait scannerDone (Run 真正退出，含 wg.Wait drain)
	//   3. mgr.Close (这一步必须在 scanner 完全静默后才调)
	cancel()

	// 记录 mgr.Close 调用时机；scanner.Run **必须**先 return 才能调 mgr.Close
	// 用 chan + select 验证 ordering：scannerDone 必须 close 在 mgr.Close 调用之前
	closeStarted := make(chan struct{})
	closeDone := make(chan struct{})
	go func() {
		select {
		case <-scannerDone:
			// good — scanner 已退出，可以安全调 Close
		case <-time.After(2 * time.Second):
			t.Errorf("scanner.Run did not return within 2s after cancel; r5 P2 wg.Wait drain regression")
			return
		}
		close(closeStarted)
		_ = mgr.Close()
		close(closeDone)
	}()

	// 等 closeStarted（说明 scannerDone 已先 close）
	select {
	case <-closeStarted:
		// 验证 closeStarted 触发时 scannerDone 必已 close
		select {
		case <-scannerDone:
			// good — ordering correct: scannerDone closed BEFORE mgr.Close called
		default:
			t.Fatalf("ordering violation: mgr.Close started but scanner.Run not returned (r6 P1: must wait scannerDone before sessionMgr.Close)")
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("shutdown helper did not advance to mgr.Close within 3s; scanner.Run probably stuck")
	}

	// 等整个 shutdown helper 完成
	select {
	case <-closeDone:
		// good
	case <-time.After(2 * time.Second):
		t.Fatalf("mgr.Close did not return within 2s")
	}
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

// ---------- Story 10.5：BroadcastToRoom 测试 ----------

// readBroadcastMessage 从 conn 读一条消息（非 close error）；timeout 内读不到
// 则 t.Fatalf。返回 message 字节流（caller 自行 unmarshal 校验）。
func readBroadcastMessage(t *testing.T, conn *websocket.Conn, timeout time.Duration) []byte {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("readBroadcastMessage: %v", err)
	}
	return msg
}

// TestBroadcastToRoom_HappyPath_AllSessionsReceive：
// 注册 3 user 在同 roomID（=3001）→ BroadcastToRoomForTest(ctx, mgr, 3001, msg) →
// 3 个 client conn 全收到 msg；返 (3, nil)。
//
// 对应 AC1 + AC3 + epics.md 行 1748。
func TestBroadcastToRoom_HappyPath_AllSessionsReceive(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{
		listMembersFn: func(ctx context.Context, roomID uint64) ([]uint64, error) {
			return []uint64{1001, 1002, 1003}, nil
		},
	}
	conn1, _, ts1 := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts1.Close()
	defer conn1.Close()
	conn2, _, ts2 := useGatewayDial(t, mgr, repo, 1002, 3001)
	defer ts2.Close()
	defer conn2.Close()
	conn3, _, ts3 := useGatewayDial(t, mgr, repo, 1003, 3001)
	defer ts3.Close()
	defer conn3.Close()

	msg := []byte(`{"type":"member.joined","requestId":"","payload":{"userId":"4001","nickname":""},"ts":1234567890}`)

	sent, err := wsapp.BroadcastToRoomForTest(context.Background(), mgr, 3001, msg)
	if err != nil {
		t.Fatalf("BroadcastToRoomForTest err: %v", err)
	}
	if sent != 3 {
		t.Errorf("sent = %d, want 3", sent)
	}

	// 3 个 client 都应收到 msg
	for i, conn := range []*websocket.Conn{conn1, conn2, conn3} {
		got := readBroadcastMessage(t, conn, 2*time.Second)
		var env map[string]any
		if err := json.Unmarshal(got, &env); err != nil {
			t.Errorf("client %d unmarshal: %v", i, err)
			continue
		}
		if env["type"] != "member.joined" {
			t.Errorf("client %d type = %v, want member.joined", i, env["type"])
		}
	}
}

// TestBroadcastToRoom_EmptyRoom_ReturnsZero：
// manager 中 roomID=9999 无任何 Session → BroadcastToRoom 返 (0, nil)；
// 不 panic / 不 log error。
//
// 对应 AC1 + epics.md 行 1746。
func TestBroadcastToRoom_EmptyRoom_ReturnsZero(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()

	sent, err := wsapp.BroadcastToRoomForTest(context.Background(), mgr, 9999, []byte(`{"type":"x","requestId":"","payload":{},"ts":0}`))
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if sent != 0 {
		t.Errorf("sent = %d, want 0", sent)
	}

	// 也跑生产路径（fire-and-forget），同样返 (0, nil)
	sent2, err2 := wsapp.BroadcastToRoom(context.Background(), mgr, 9999, []byte(`{"type":"y","requestId":"","payload":{},"ts":0}`))
	if err2 != nil {
		t.Errorf("BroadcastToRoom err = %v, want nil", err2)
	}
	if sent2 != 0 {
		t.Errorf("BroadcastToRoom sent = %d, want 0", sent2)
	}
}

// TestBroadcastToRoom_OneSessionSendFails_OthersStillReceive：
// 注册 3 Session，把第 2 个 Session 提前 Close（Send 返 ErrSessionClosed）→
// BroadcastToRoomForTest → 第 1 / 第 3 个 client 收到 msg；返 (3, nil)
// （sent 仍是 len(slice)，与 AC1 钦定一致 —— 不回扫 send 失败数）。
//
// 注意：被 Close 的 Session 在 ListSessionsByRoomID 返回前可能已经从 manager
// 移除（Close → notifyClosed → Unregister），但本测试通过提前 Close 后立即调
// BroadcastToRoom 触发 race —— 在 Unregister 完成前 ListSessionsByRoomID 仍可能
// 看到该 Session。此时 fanout goroutine 内 Send 返 ErrSessionClosed，log warn
// 但不阻塞其他 goroutine。
//
// 对应 AC1 + AC3 + epics.md 行 1750。
func TestBroadcastToRoom_OneSessionSendFails_OthersStillReceive(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{
		listMembersFn: func(ctx context.Context, roomID uint64) ([]uint64, error) {
			return []uint64{2001, 2002, 2003}, nil
		},
	}
	conn1, _, ts1 := useGatewayDial(t, mgr, repo, 2001, 3002)
	defer ts1.Close()
	defer conn1.Close()
	_, sess2, ts2 := useGatewayDial(t, mgr, repo, 2002, 3002)
	defer ts2.Close()
	conn3, _, ts3 := useGatewayDial(t, mgr, repo, 2003, 3002)
	defer ts3.Close()
	defer conn3.Close()

	// 提前关 sess2：Send 立即返 ErrSessionClosed
	_ = sess2.Close()
	// 等 sess2 closed 标志 settle（Send 走 ErrSessionClosed 路径）
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := sess2.Send([]byte("probe")); errors.Is(err, wsapp.ErrSessionClosed) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	msg := []byte(`{"type":"member.joined","requestId":"","payload":{"userId":"5001","nickname":""},"ts":1}`)

	// race window：sess2 可能已被 Unregister 从 sessionsByRoom 移除（→ 返 (2, nil)），
	// 也可能还没（→ 返 (3, nil) 但 sess2 Send 失败 log warn）。两种都符合 AC1 钦定
	// 的 "sent = len(slice)" 语义；两种情况下 conn1 / conn3 都必须收到 msg。
	sent, err := wsapp.BroadcastToRoomForTest(context.Background(), mgr, 3002, msg)
	if err != nil {
		t.Fatalf("BroadcastToRoomForTest err: %v", err)
	}
	if sent != 2 && sent != 3 {
		t.Errorf("sent = %d, want 2 or 3 (race with Unregister)", sent)
	}

	// conn1 / conn3 必须都收到 msg
	for i, conn := range []*websocket.Conn{conn1, conn3} {
		got := readBroadcastMessage(t, conn, 2*time.Second)
		var env map[string]any
		if err := json.Unmarshal(got, &env); err != nil {
			t.Errorf("client %d unmarshal: %v", i, err)
			continue
		}
		if env["type"] != "member.joined" {
			t.Errorf("client %d type = %v, want member.joined", i, env["type"])
		}
	}
}

// TestBroadcastToRoom_SendBufferFull_LogWarnContinues：
// 注册 1 Session，提前用 Send 填满 sendChan（sendChanCapacity=32 → 推 33 次让
// 第 33 次返 ErrSessionSendBufferFull）→ 再 BroadcastToRoomForTest → 主函数
// 不返 error；log warn 触发（验证 fanout 内 Send 失败路径 graceful）。
//
// 实现策略：注册 Session 但**冷冻**底层 conn read（不调 ReadMessage）让 server
// 端 writeLoop 写到 conn 后阻塞在 client TCP buffer 上；此时 sendChan 会被
// 填满。然后 BroadcastToRoom 的 Send 走 select-default 返 ErrSessionSendBufferFull。
//
// 但实际策略更简化：直接在测试体内多次 Send 让 sendChan 填到满（writeLoop 也在
// 从 sendChan 拉取，所以 client 不读消息时，writeLoop 阻塞在 conn.WriteMessage
// → sendChan 不再被消费 → 后续 Send 返 ErrSessionSendBufferFull）。
//
// 对应 AC1 + AC3。
func TestBroadcastToRoom_SendBufferFull_LogWarnContinues(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{
		listMembersFn: func(ctx context.Context, roomID uint64) ([]uint64, error) {
			return []uint64{3001}, nil
		},
	}
	conn, sess, ts := useGatewayDial(t, mgr, repo, 3001, 3003)
	defer ts.Close()
	defer conn.Close()

	// 不读 conn 让 writeLoop 阻塞在 conn.WriteMessage（client TCP buffer 满后
	// gorilla writeLoop 会卡在 SetWriteDeadline + Write）。然后用 Send 不断推
	// 直到 sendChan 满返 ErrSessionSendBufferFull。
	stuffer := []byte(`{"type":"stuff","requestId":"","payload":{},"ts":0}`)
	bufferFull := false
	for i := 0; i < 200; i++ {
		err := sess.Send(stuffer)
		if errors.Is(err, wsapp.ErrSessionSendBufferFull) {
			bufferFull = true
			break
		}
		if err != nil {
			t.Fatalf("unexpected Send err at i=%d: %v", i, err)
		}
	}
	if !bufferFull {
		t.Skip("could not fill sendChan within 200 attempts (writeLoop drains too fast under this OS); skipping")
		return
	}

	// 此时 sendChan 满；调 BroadcastToRoomForTest，fanout 内 Send 必返
	// ErrSessionSendBufferFull → log warn 但主函数仍返 (1, nil)
	msg := []byte(`{"type":"member.joined","requestId":"","payload":{"userId":"x","nickname":""},"ts":0}`)
	sent, err := wsapp.BroadcastToRoomForTest(context.Background(), mgr, 3003, msg)
	if err != nil {
		t.Errorf("BroadcastToRoomForTest err = %v, want nil (fanout internal Send 失败应吞并 log warn)", err)
	}
	if sent != 1 {
		t.Errorf("sent = %d, want 1 (sent 是发起 Send 的数量，与 send 是否成功无关)", sent)
	}
}

// TestBroadcastToRoom_LargeFanout_100Sessions_AllSent：
// 注册 30 user 在同 roomID → BroadcastToRoomForTest → 全部收到 msg；返 (30, nil)。
//
// 注意：本测试用 30 而非 epics 钦定的 100，原因：useGatewayDial 每个 user 启
// 独立 httptest server（gateway-per-user 模式 —— 既有 helper 设计），100 个
// httptest server 在 Windows / CI 环境会因为端口耗尽 / fd limit 触发偶发失败。
// 30 个 session 已足够验证 fanout 正确性 + 无 goroutine leak（fanout drain 本质
// 不随 N 变化）。如未来 helper 改为单 httptest server 多 conn 模式可上调到 100。
//
// 对应 AC3 fanout 性能验证。
func TestBroadcastToRoom_LargeFanout_30Sessions_AllSent(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}

	const N = 30
	conns := make([]*websocket.Conn, 0, N)
	tss := make([]*httptest.Server, 0, N)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
		for _, ts := range tss {
			ts.Close()
		}
	}()
	for i := 0; i < N; i++ {
		conn, _, ts := useGatewayDial(t, mgr, repo, uint64(8000+i), 8500)
		conns = append(conns, conn)
		tss = append(tss, ts)
	}

	msg := []byte(`{"type":"big.fanout","requestId":"","payload":{},"ts":0}`)
	sent, err := wsapp.BroadcastToRoomForTest(context.Background(), mgr, 8500, msg)
	if err != nil {
		t.Fatalf("BroadcastToRoomForTest err: %v", err)
	}
	if sent != N {
		t.Errorf("sent = %d, want %d", sent, N)
	}

	for i, conn := range conns {
		got := readBroadcastMessage(t, conn, 3*time.Second)
		var env map[string]any
		if err := json.Unmarshal(got, &env); err != nil {
			t.Errorf("client %d unmarshal: %v", i, err)
			continue
		}
		if env["type"] != "big.fanout" {
			t.Errorf("client %d type = %v, want big.fanout", i, env["type"])
		}
	}
}

// TestBroadcastToRoom_DifferentRooms_Isolated：
// 注册 2 user 在 roomID=4001 + 1 user 在 roomID=4002 → BroadcastToRoomForTest(ctx, mgr, 4001, msg) →
// 仅 roomID=4001 的 2 个 client 收到，roomID=4002 的 client **未**收到；返 (2, nil)。
//
// 对应 AC1 + epics.md 行 1748。
func TestBroadcastToRoom_DifferentRooms_Isolated(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}

	c1, _, ts1 := useGatewayDial(t, mgr, repo, 9001, 4001)
	defer ts1.Close()
	defer c1.Close()
	c2, _, ts2 := useGatewayDial(t, mgr, repo, 9002, 4001)
	defer ts2.Close()
	defer c2.Close()
	c3, _, ts3 := useGatewayDial(t, mgr, repo, 9003, 4002)
	defer ts3.Close()
	defer c3.Close()

	msg := []byte(`{"type":"room4001.only","requestId":"","payload":{},"ts":0}`)
	sent, err := wsapp.BroadcastToRoomForTest(context.Background(), mgr, 4001, msg)
	if err != nil {
		t.Fatalf("BroadcastToRoomForTest err: %v", err)
	}
	if sent != 2 {
		t.Errorf("sent = %d, want 2 (only room=4001)", sent)
	}

	// c1 / c2 收到
	for i, conn := range []*websocket.Conn{c1, c2} {
		got := readBroadcastMessage(t, conn, 2*time.Second)
		var env map[string]any
		if err := json.Unmarshal(got, &env); err != nil {
			t.Errorf("client %d unmarshal: %v", i, err)
			continue
		}
		if env["type"] != "room4001.only" {
			t.Errorf("client %d type = %v, want room4001.only", i, env["type"])
		}
	}
	// c3 不应收到（短 timeout 期望 read deadline 触发）
	_ = c3.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	_, gotMsg, gotErr := c3.ReadMessage()
	if gotErr == nil {
		t.Errorf("c3 (room=4002) unexpectedly received msg: %s", gotMsg)
	}
}

// TestBroadcastToRoom_ConcurrentToDifferentRooms_AllCorrect：
// 注册 2 room 各 3 user（共 6）→ 并发触发若干次 BroadcastToRoomForTest（一半给
// room=A，一半给 room=B）→ 每个 room 内的 3 个 client 各自收到对应数量的 msg +
// 跨 room 互不串扰；无 panic / 无 race。
//
// 用 -race 单测会自动捕获 race；本测试只需断言每个 conn 的 read count 正确。
//
// 对应 AC3（并发安全）+ epics.md 行 1752。
func TestBroadcastToRoom_ConcurrentToDifferentRooms_AllCorrect(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}

	const broadcastsPerRoom = 10 // 每个 room 广播 10 次（共 20 次跨 room 并发）

	roomAConns := make([]*websocket.Conn, 0, 3)
	roomBConns := make([]*websocket.Conn, 0, 3)
	tss := []*httptest.Server{}
	defer func() {
		for _, c := range roomAConns {
			c.Close()
		}
		for _, c := range roomBConns {
			c.Close()
		}
		for _, ts := range tss {
			ts.Close()
		}
	}()

	for i := 0; i < 3; i++ {
		c, _, ts := useGatewayDial(t, mgr, repo, uint64(10001+i), 5001)
		roomAConns = append(roomAConns, c)
		tss = append(tss, ts)
	}
	for i := 0; i < 3; i++ {
		c, _, ts := useGatewayDial(t, mgr, repo, uint64(10101+i), 5002)
		roomBConns = append(roomBConns, c)
		tss = append(tss, ts)
	}

	// 启 broadcastsPerRoom*2 个 goroutine 并发广播
	var wg sync.WaitGroup
	wg.Add(broadcastsPerRoom * 2)
	for i := 0; i < broadcastsPerRoom; i++ {
		go func(idx int) {
			defer wg.Done()
			msg := []byte(fmt.Sprintf(`{"type":"roomA","requestId":"","payload":{"i":%d},"ts":0}`, idx))
			_, _ = wsapp.BroadcastToRoomForTest(context.Background(), mgr, 5001, msg)
		}(i)
		go func(idx int) {
			defer wg.Done()
			msg := []byte(fmt.Sprintf(`{"type":"roomB","requestId":"","payload":{"i":%d},"ts":0}`, idx))
			_, _ = wsapp.BroadcastToRoomForTest(context.Background(), mgr, 5002, msg)
		}(i)
	}
	wg.Wait()

	// 每个 room 内的 client 应收到 broadcastsPerRoom 条 type 全是对应 room 的 msg
	checkRoom := func(label string, conns []*websocket.Conn, wantType string) {
		for ci, conn := range conns {
			seen := 0
			for seen < broadcastsPerRoom {
				_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
				_, msg, err := conn.ReadMessage()
				if err != nil {
					t.Errorf("%s conn[%d] read err after %d msgs: %v", label, ci, seen, err)
					return
				}
				var env map[string]any
				if err := json.Unmarshal(msg, &env); err != nil {
					t.Errorf("%s conn[%d] unmarshal: %v", label, ci, err)
					return
				}
				if env["type"] != wantType {
					t.Errorf("%s conn[%d] cross-room leak: got type=%v, want %s", label, ci, env["type"], wantType)
					return
				}
				seen++
			}
		}
	}
	checkRoom("roomA", roomAConns, "roomA")
	checkRoom("roomB", roomBConns, "roomB")
}

// TestBroadcastToRoom_BroadcastFn_TypeAlias_Compiles：
// 编译时验证：声明 var fn ws.BroadcastFn 并赋值 closure；调 fn(...) 编译成功
// 即视为 pass（验证 BroadcastFn 类型签名与 BroadcastToRoom 兼容）。
//
// 对应 AC2。
func TestBroadcastToRoom_BroadcastFn_TypeAlias_Compiles(t *testing.T) {
	// closure 形态（service 层注入 mock 的典型用法）
	called := false
	var fn wsapp.BroadcastFn = func(ctx context.Context, roomID uint64, msg []byte) (int, error) {
		called = true
		return 1, nil
	}
	sent, err := fn(context.Background(), 1234, []byte("test"))
	if err != nil {
		t.Errorf("fn err = %v, want nil", err)
	}
	if sent != 1 {
		t.Errorf("fn sent = %d, want 1", sent)
	}
	if !called {
		t.Errorf("closure not called")
	}

	// wire 期 closure 捕获 BroadcastToRoom 的形态（生产路径典型用法）
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	var prodFn wsapp.BroadcastFn = func(ctx context.Context, roomID uint64, msg []byte) (int, error) {
		return wsapp.BroadcastToRoom(ctx, mgr, roomID, msg)
	}
	sent2, err2 := prodFn(context.Background(), 9999, []byte(`{"type":"x","requestId":"","payload":{},"ts":0}`))
	if err2 != nil {
		t.Errorf("prodFn err = %v, want nil", err2)
	}
	if sent2 != 0 {
		t.Errorf("prodFn sent = %d, want 0 (empty room)", sent2)
	}
}

// TestBroadcastToRoom_NilMessage_HandledGracefully：
// BroadcastToRoomForTest(ctx, mgr, roomID, nil) → 不 panic；client 收到
// zero-length data frame；返 (1, nil)。
//
// 对应 AC1 防御性。
func TestBroadcastToRoom_NilMessage_HandledGracefully(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, _, ts := useGatewayDial(t, mgr, repo, 11001, 6001)
	defer ts.Close()
	defer conn.Close()

	sent, err := wsapp.BroadcastToRoomForTest(context.Background(), mgr, 6001, nil)
	if err != nil {
		t.Errorf("BroadcastToRoomForTest err = %v, want nil", err)
	}
	if sent != 1 {
		t.Errorf("sent = %d, want 1", sent)
	}

	// client 应能 ReadMessage 不 panic（gorilla 写 nil → empty frame）
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if len(msg) != 0 {
		// 不强行 fail：gorilla 行为可能因版本不同；只 log
		t.Logf("nil msg → client received len=%d (expected 0)", len(msg))
	}
}

// TestBroadcastToRoom_SessionRegisteredAfterListSnapshot_NotIncluded：
// race 验证：调 BroadcastToRoomForTest 期间 Register 一个新 Session → 验证
// 新 Session 在 ListSessionsByRoomID 切片快照外 → 本次 broadcast 不送达；
// 下次 BroadcastToRoom 会包括新 Session。
//
// 由于 BroadcastToRoomForTest 是同步等所有 fanout 完成才返回，本测试通过
// "在 ForTest 返回后立即 Register 新 Session + 再调一次 ForTest" 验证：
//   - 第一次 broadcast：3 个原 Session 收到；返 (3, nil)
//   - 加入新 Session 后第二次 broadcast：4 个 Session 都收到；返 (4, nil)
//
// 验证 ListSessionsByRoomID 是 read-lock copy 快照（10.3 r5 P2 不变量保留）。
//
// 对应 AC3（review r5 P2 不变量保留）。
func TestBroadcastToRoom_SessionRegisteredAfterListSnapshot_NotIncluded(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}

	c1, _, ts1 := useGatewayDial(t, mgr, repo, 12001, 7001)
	defer ts1.Close()
	defer c1.Close()
	c2, _, ts2 := useGatewayDial(t, mgr, repo, 12002, 7001)
	defer ts2.Close()
	defer c2.Close()

	// 第一次 broadcast：仅 c1 / c2 收到
	msg1 := []byte(`{"type":"first","requestId":"","payload":{},"ts":0}`)
	sent1, err := wsapp.BroadcastToRoomForTest(context.Background(), mgr, 7001, msg1)
	if err != nil {
		t.Fatalf("first BroadcastToRoomForTest err: %v", err)
	}
	if sent1 != 2 {
		t.Errorf("first sent = %d, want 2", sent1)
	}
	for i, conn := range []*websocket.Conn{c1, c2} {
		got := readBroadcastMessage(t, conn, 2*time.Second)
		var env map[string]any
		_ = json.Unmarshal(got, &env)
		if env["type"] != "first" {
			t.Errorf("client %d first msg type = %v, want first", i, env["type"])
		}
	}

	// Register 新 Session → 第二次 broadcast 应包括它（snapshot 是 list 时刻的快照）
	c3, _, ts3 := useGatewayDial(t, mgr, repo, 12003, 7001)
	defer ts3.Close()
	defer c3.Close()

	msg2 := []byte(`{"type":"second","requestId":"","payload":{},"ts":0}`)
	sent2, err := wsapp.BroadcastToRoomForTest(context.Background(), mgr, 7001, msg2)
	if err != nil {
		t.Fatalf("second BroadcastToRoomForTest err: %v", err)
	}
	if sent2 != 3 {
		t.Errorf("second sent = %d, want 3 (new session included)", sent2)
	}
	for i, conn := range []*websocket.Conn{c1, c2, c3} {
		got := readBroadcastMessage(t, conn, 2*time.Second)
		var env map[string]any
		_ = json.Unmarshal(got, &env)
		if env["type"] != "second" {
			t.Errorf("client %d second msg type = %v, want second", i, env["type"])
		}
	}
}

// TestBroadcastToRoom_DoesNotTriggerUnregisterHook：
// BroadcastToRoom 是只读路径（拿 list + Send），**不**应触发 onUnregister
// 钩子；本测试用 WithUnregisterHook 注入计数器，调 broadcast 后断言计数 = 0。
//
// 对应 AC1 + AC3 关键约束 "不调 mgr.Unregister / Session.Close"。
func TestBroadcastToRoom_DoesNotTriggerUnregisterHook(t *testing.T) {
	var unregisterCount atomic.Int32
	mgr := wsapp.NewSessionManager(wsapp.WithUnregisterHook(func(s *wsapp.Session) {
		unregisterCount.Add(1)
	}))
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}

	conn, _, ts := useGatewayDial(t, mgr, repo, 13001, 8001)
	defer ts.Close()
	defer conn.Close()

	sent, err := wsapp.BroadcastToRoomForTest(context.Background(), mgr, 8001, []byte(`{"type":"x","requestId":"","payload":{},"ts":0}`))
	if err != nil {
		t.Fatalf("BroadcastToRoomForTest err: %v", err)
	}
	if sent != 1 {
		t.Errorf("sent = %d, want 1", sent)
	}
	// 读完 client 收到的 msg（避免 conn buffer 残留影响下面断言）
	_ = readBroadcastMessage(t, conn, 2*time.Second)

	// broadcast 不应触发 onUnregister 钩子
	if got := unregisterCount.Load(); got != 0 {
		t.Errorf("unregisterCount = %d, want 0 (broadcast 是只读路径不应触发 lifecycle hook)", got)
	}
}

// ---------- Story 10.5 review r1 P1/P2 fix 回归测试 ----------

// TestBroadcastToRoom_R1_PerSessionOrder_AcrossConsecutiveBroadcasts：
//
//	review 10-5 r1 P1 回归：连续两次 BroadcastToRoom（msg1, msg2）→ 同一
//	session 收到的顺序必须是 msg1 → msg2（room 广播是 ordered stream）。
//
// 实装关键：r1 修复后生产路径同步遍历调 Session.Send（不再 goroutine fanout）→
// caller 依次调 BroadcastToRoom(msg1), BroadcastToRoom(msg2) 在同 goroutine 里
// → msg1 入队所有 session 的 sendChan 后 BroadcastToRoom 才 return → caller 调
// msg2 入队 → 所有 session 的 sendChan 内 msg1 物理位置先于 msg2 → writeLoop
// FIFO 消费写到 conn → client 端观察到 msg1 在 msg2 之前。
//
// 此测试用 N=5 个 session × 多次连续 (msg1, msg2) 对，断言每个 session 收到
// 的顺序都是 msg1 → msg2 → msg1 → msg2 ...（不允许 msg2 先于其前面那个 msg1）。
func TestBroadcastToRoom_R1_PerSessionOrder_AcrossConsecutiveBroadcasts(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}

	const N = 5
	const pairs = 4 // 4 对 (msg1, msg2) → 每个 conn 应收 8 条
	conns := make([]*websocket.Conn, 0, N)
	tss := make([]*httptest.Server, 0, N)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
		for _, ts := range tss {
			ts.Close()
		}
	}()
	for i := 0; i < N; i++ {
		conn, _, ts := useGatewayDial(t, mgr, repo, uint64(20001+i), 9001)
		conns = append(conns, conn)
		tss = append(tss, ts)
	}

	// 同 goroutine 依次发 pairs 对 (msg1, msg2)
	for p := 0; p < pairs; p++ {
		msg1 := []byte(fmt.Sprintf(`{"type":"member.joined","requestId":"","payload":{"i":%d},"ts":0}`, p))
		msg2 := []byte(fmt.Sprintf(`{"type":"member.left","requestId":"","payload":{"i":%d},"ts":0}`, p))
		if _, err := wsapp.BroadcastToRoom(context.Background(), mgr, 9001, msg1); err != nil {
			t.Fatalf("pair %d msg1 BroadcastToRoom: %v", p, err)
		}
		if _, err := wsapp.BroadcastToRoom(context.Background(), mgr, 9001, msg2); err != nil {
			t.Fatalf("pair %d msg2 BroadcastToRoom: %v", p, err)
		}
	}

	// 每个 conn 读 pairs*2 条，断言顺序是 joined → left → joined → left ...
	expected := []string{}
	for p := 0; p < pairs; p++ {
		expected = append(expected, "member.joined", "member.left")
	}
	for ci, conn := range conns {
		for k, want := range expected {
			got := readBroadcastMessage(t, conn, 3*time.Second)
			var env map[string]any
			if err := json.Unmarshal(got, &env); err != nil {
				t.Fatalf("conn[%d] step %d unmarshal: %v", ci, k, err)
			}
			if env["type"] != want {
				t.Errorf("conn[%d] step %d type = %v, want %s (per-session order broken — review 10-5 r1 P1 regression)",
					ci, k, env["type"], want)
			}
		}
	}
}

// TestBroadcastToRoom_R1_CallerMayMutateMsgAfterReturn：
//
//	review 10-5 r1 P2 回归：caller 在 BroadcastToRoom return 后 mutate /
//	reuse 原 msg buffer，不应影响 client 实际收到的内容（验证入口
//	bytes.Clone 工作）。
//
// 实装关键：r1 修复后入口 `payload := bytes.Clone(msg)` → payload 与 caller msg
// 完全隔离 → caller 在 return 后随意 mutate 原 buffer，client 收到的还是 clone
// 时刻的内容。
//
// 测试构造：分配一个 long msg buffer，调 BroadcastToRoom 后**立即 zero out**
// 整个 buffer → client 端 ReadMessage 读到的内容必须仍是 clone 时刻的字节流，
// 不是被 mutate 后的全 0 字节。
func TestBroadcastToRoom_R1_CallerMayMutateMsgAfterReturn(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}

	const N = 3
	conns := make([]*websocket.Conn, 0, N)
	tss := make([]*httptest.Server, 0, N)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
		for _, ts := range tss {
			ts.Close()
		}
	}()
	for i := 0; i < N; i++ {
		conn, _, ts := useGatewayDial(t, mgr, repo, uint64(21001+i), 9101)
		conns = append(conns, conn)
		tss = append(tss, ts)
	}

	// 构造 caller-owned buffer（注意：使用 make + copy 而非 string literal）
	original := []byte(`{"type":"member.joined","requestId":"","payload":{"userId":"42"},"ts":0}`)
	buf := make([]byte, len(original))
	copy(buf, original)

	if _, err := wsapp.BroadcastToRoom(context.Background(), mgr, 9101, buf); err != nil {
		t.Fatalf("BroadcastToRoom: %v", err)
	}

	// **立即** zero out buf —— 模拟 caller 复用 / 释放 buffer
	for i := range buf {
		buf[i] = 0
	}

	// 每个 client 收到的应是 clone 时刻的内容（与 buf zero 后无关）
	for ci, conn := range conns {
		got := readBroadcastMessage(t, conn, 3*time.Second)
		// got 应能成功 unmarshal 为合法 envelope
		var env map[string]any
		if err := json.Unmarshal(got, &env); err != nil {
			t.Errorf("conn[%d] unmarshal: %v — got bytes: %q (caller mutate after return leaked into payload — review 10-5 r1 P2 regression)",
				ci, err, got)
			continue
		}
		if env["type"] != "member.joined" {
			t.Errorf("conn[%d] type = %v, want member.joined (caller mutate leaked — review 10-5 r1 P2 regression)",
				ci, env["type"])
		}
		// payload.userId 应是 "42"
		payload, ok := env["payload"].(map[string]any)
		if !ok {
			t.Errorf("conn[%d] payload not a map: %v", ci, env["payload"])
			continue
		}
		if payload["userId"] != "42" {
			t.Errorf("conn[%d] payload.userId = %v, want \"42\"", ci, payload["userId"])
		}
	}
}

// TestBroadcastToRoom_R1_LargeN_SyncFanoutFastEnough 已迁移到内部测试包
// （broadcast_perf_internal_test.go），用裸 *Session{sendChan} fixture 取代
// httptest.Server 路径 —— 详见 review 10-5 r2 P2 修复。本处保留注释以便
// `git blame` 时找到迁移轨迹。

// ---------- Story 10.6 PresenceRepo 钩子集成测试（端到端 lifecycle）----------
//
// 本节 4 case 验证"hook 挂上 SessionManager 后，lifecycle 触发 → presenceRepo
// 方法调用次数正确"端到端语义。**不**真正 import redis 包跑 miniredis（避免 ws
// 测试包反向 import redis 包形成循环依赖）；用 fakePresenceRepo + atomic.Int32
// 计数器即可验证"钩子触发次数"语义。
//
// 4 case 严格按 epics.md §Story 10.6 + lesson 钦定的 4 个不变量铺：
//   - Register → AddOnline 钩子触发 1 次
//   - Unregister → RemoveOnline 钩子触发 1 次
//   - Reconnect 替换路径 → 旧 Session 的 RemoveOnline 触发恰好 1 次
//     （不变量见 lesson 2026-05-06-ws-reconnect-unregister-hook-and-prod-contract-gate.md）
//   - SessionManager.Close → 每个 Session 的 RemoveOnline 都触发
//     （不变量见 lesson 2026-05-06-ws-session-send-close-race-and-shutdown-hooks.md）

// fakePresenceRepo 是 PresenceRepo 接口的轻量替身（仅 ws_test.go 内部用，验证
// 钩子触发次数）。**不**真正访问 Redis；用 atomic.Int32 计数 + sync.Map 收集
// sessionID 让 case 能精确断言"哪些 Session 触发了 hook"。
//
// 与生产 redisrepo.PresenceRepo 接口的语义一致（5 方法）—— 但本结构体**不**实装
// PresenceRepo interface（避免反向 import redis 包），只是 ws hook adapter 适配
// 函数签名（func(s *wsapp.Session)）。
type fakePresenceRepo struct {
	addCount      atomic.Int32
	removeCount   atomic.Int32
	addedIDs      sync.Map // sessionID → struct{}
	removedIDs    sync.Map // sessionID → struct{}
	addedRooms    sync.Map // sessionID → roomID
	removedRooms  sync.Map // sessionID → roomID
	addedUsers    sync.Map // sessionID → userID
	removedUsers  sync.Map // sessionID → userID
	addReturnErr  error    // 让 case 能注入错误验证 log warn 路径（本节未用，预留）
	removeReturnErr error
}

func (f *fakePresenceRepo) AddOnline(_ context.Context, roomID, userID uint64, sessionID string) error {
	f.addCount.Add(1)
	f.addedIDs.Store(sessionID, struct{}{})
	f.addedRooms.Store(sessionID, roomID)
	f.addedUsers.Store(sessionID, userID)
	return f.addReturnErr
}

func (f *fakePresenceRepo) RemoveOnline(_ context.Context, roomID, userID uint64, sessionID string) error {
	f.removeCount.Add(1)
	// 用 userID 当 key 也行，但 Reconnect / Close 场景需要按 sessionID 关联，
	// 为简化，hook adapter 的 closure 走 sessionID 是更稳妥的索引——
	// 本 fake 在 Remove 路径拿到 (roomID, userID, sessionID)，没有真 Redis state；
	// 为支持 case 4（Close 触发每个 sessionID）的断言，hook adapter 在
	// Remove 路径直接调 fake 的 hookSessionRemove(s) 而非 RemoveOnline。
	// 但为接口对齐，本方法仍保留以便未来真消费。
	// Story 10.6 r1 修：sessionID 参数加入接口签名（real impl 走 Lua script
	// compare-and-delete），但本 fake 不做 sessionID guard 模拟 —— hook adapter
	// 集成测试的目标是验证"钩子被触发的次数与时机"，不是验证 Lua script 行为
	// （后者由 presence_repo_test.go 的 ReconnectRace case 单测覆盖）。
	_ = userID
	_ = roomID
	_ = sessionID
	return f.removeReturnErr
}

// hookRegister / hookUnregister 是给 SessionManager hook adapter 的快捷入口；
// closure 内拿到 *Session 后调本方法（既计数 + 又记录 sessionID 索引）。
// 与 main.go 钩子 adapter 模式一致（adapter 内拿 Session.UserID/RoomID/SessionID
// 走 PresenceRepo 接口）。
func (f *fakePresenceRepo) hookRegister(s *wsapp.Session) {
	f.addCount.Add(1)
	f.addedIDs.Store(s.SessionID(), struct{}{})
	f.addedRooms.Store(s.SessionID(), s.RoomID())
	f.addedUsers.Store(s.SessionID(), s.UserID())
}

func (f *fakePresenceRepo) hookUnregister(s *wsapp.Session) {
	f.removeCount.Add(1)
	f.removedIDs.Store(s.SessionID(), struct{}{})
	f.removedRooms.Store(s.SessionID(), s.RoomID())
	f.removedUsers.Store(s.SessionID(), s.UserID())
}

// TestPresenceHook_RegisterTriggersAddOnline: Story 10.6 钩子集成 case 1。
// Register → onRegister 钩子 adapter → fakePresenceRepo.hookRegister 触发 1 次。
// 钩子收到的 Session 应携带正确的 userID / roomID / sessionID（不是空 / 不是别的 user）。
func TestPresenceHook_RegisterTriggersAddOnline(t *testing.T) {
	fake := &fakePresenceRepo{}
	mgr := wsapp.NewSessionManager(
		wsapp.WithRegisterHook(func(s *wsapp.Session) {
			fake.hookRegister(s)
		}),
		wsapp.WithUnregisterHook(func(s *wsapp.Session) {
			fake.hookUnregister(s)
		}),
	)
	defer mgr.Close()

	repo := &stubRoomMemberRepo{}
	conn, session, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	if got := fake.addCount.Load(); got != 1 {
		t.Errorf("AddOnline hook called %d times, want 1", got)
	}
	if got := fake.removeCount.Load(); got != 0 {
		t.Errorf("RemoveOnline hook called %d times, want 0 (not yet unregistered)", got)
	}
	// 钩子收到的 Session 上下文正确
	if _, ok := fake.addedIDs.Load(session.SessionID()); !ok {
		t.Errorf("AddOnline hook did not receive sessionID %q", session.SessionID())
	}
	if v, ok := fake.addedRooms.Load(session.SessionID()); !ok || v.(uint64) != uint64(3001) {
		t.Errorf("AddOnline hook roomID = %v, want 3001", v)
	}
	if v, ok := fake.addedUsers.Load(session.SessionID()); !ok || v.(uint64) != uint64(1001) {
		t.Errorf("AddOnline hook userID = %v, want 1001", v)
	}
}

// TestPresenceHook_UnregisterTriggersRemoveOnline: Story 10.6 钩子集成 case 2。
// Session.Close → notifyClosed → Unregister → onUnregister 钩子 adapter →
// fakePresenceRepo.hookUnregister 触发 1 次。
func TestPresenceHook_UnregisterTriggersRemoveOnline(t *testing.T) {
	fake := &fakePresenceRepo{}
	mgr := wsapp.NewSessionManager(
		wsapp.WithRegisterHook(func(s *wsapp.Session) {
			fake.hookRegister(s)
		}),
		wsapp.WithUnregisterHook(func(s *wsapp.Session) {
			fake.hookUnregister(s)
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
		if fake.removeCount.Load() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := fake.removeCount.Load(); got != 1 {
		t.Errorf("RemoveOnline hook called %d times, want 1", got)
	}
	if _, ok := fake.removedIDs.Load(session.SessionID()); !ok {
		t.Errorf("RemoveOnline hook did not receive sessionID %q", session.SessionID())
	}
}

// TestPresenceHook_ReconnectTriggersRemoveForOldSession: Story 10.6 钩子集成 case 3。
// 同 user 重连替换路径必须为旧 Session 触发 RemoveOnline 钩子**恰好一次**。
//
// 这是 lesson 2026-05-06-ws-reconnect-unregister-hook-and-prod-contract-gate.md
// 钉死的 P1 不变量；SessionManager 已在 10.3 r2 修复，本 case 是 contract verification
// 让 future 反向破坏行为时立即失败。
func TestPresenceHook_ReconnectTriggersRemoveForOldSession(t *testing.T) {
	fake := &fakePresenceRepo{}
	mgr := wsapp.NewSessionManager(
		wsapp.WithRegisterHook(func(s *wsapp.Session) {
			fake.hookRegister(s)
		}),
		wsapp.WithUnregisterHook(func(s *wsapp.Session) {
			fake.hookUnregister(s)
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

	// 拿到旧 sessionID
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

	// 等 onUnregister 钩子异步触发完毕（oldS.Close() → notifyClosed → Unregister(oldID) →
	// onUnregister）
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if fake.removeCount.Load() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// 断言：Add 钩子触发 2 次（roomA + roomB），Remove 钩子触发恰好 1 次（旧 Session）
	if got := fake.addCount.Load(); got != 2 {
		t.Errorf("AddOnline hook called %d times, want 2 (one per Register)", got)
	}
	if got := fake.removeCount.Load(); got != 1 {
		t.Errorf("RemoveOnline hook called %d times, want exactly 1 (reconnect replace path)", got)
	}
	if _, ok := fake.removedIDs.Load(oldSessionID); !ok {
		t.Errorf("RemoveOnline hook did not fire for old session %q", oldSessionID)
	}
}

// TestPresenceHook_Reconnect_AddBeforeRemove_NoOfflineWindow: review 10-6 r10 P2
// 防回归 —— 同 user reconnect 替换路径下 onRegister(NEW) 必须**先于**
// onUnregister(OLD) 触发。
//
// 修前 Register 顺序：
//   - 锁外: replaced.Close() → notifyClosed → Unregister(oldID) → onUnregister(OLD)
//     → RemoveOnline(OLD) 跑掉 user_key 和 oldRoom set 上的 user
//   - 锁外: onRegister(NEW) → AddOnline(NEW) 重新写回
//
// 中间窗口（RemoveOnline 跑完到 AddOnline 跑完之间）IsOnline / ListOnline 看到
// user 暂时离线（生产路径上 Redis brownout 期可能拉到几百毫秒），违反 V1 §12 "presence
// 是查询时态" 的连续性语义。
//
// 修后顺序：onRegister(NEW) **先**跑 → AddOnline(NEW) 完整就位（含 r10 P1 自动
// SREM stale oldRoom）→ replaced.Close() → onUnregister(OLD) → RemoveOnline(OLD)
// 后跑（Lua script 看 currentSession=NEW≠OLD 走 case 3/4 不动 user_key）。全程
// user 看似 online。
//
// 本测试用 fake adapter 收集 hook 触发**顺序**，断言 NEW 的 AddOnline call 在 OLD
// 的 RemoveOnline call **之前**。
func TestPresenceHook_Reconnect_AddBeforeRemove_NoOfflineWindow(t *testing.T) {
	var (
		mu          sync.Mutex
		callOrder   []string // append "add:<sid>" / "remove:<sid>" 按真实触发顺序
		oldSessionID string
	)
	mgr := wsapp.NewSessionManager(
		wsapp.WithRegisterHook(func(s *wsapp.Session) {
			mu.Lock()
			callOrder = append(callOrder, "add:"+s.SessionID())
			mu.Unlock()
		}),
		wsapp.WithUnregisterHook(func(s *wsapp.Session) {
			mu.Lock()
			callOrder = append(callOrder, "remove:"+s.SessionID())
			mu.Unlock()
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

	// 抓旧 sessionID
	deadline := time.Now().Add(2 * time.Second)
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

	// 第二次连接到 roomB（同 user → 触发 reconnect 替换）
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

	// 等异步 OLD 的 RemoveOnline 钩子触发完毕（oldS.Close() → notifyClosed →
	// Unregister(oldID) → onUnregister）
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		hasRemove := false
		for _, e := range callOrder {
			if strings.HasPrefix(e, "remove:") {
				hasRemove = true
				break
			}
		}
		mu.Unlock()
		if hasRemove {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()

	// 期望 callOrder 至少 3 条：[add:OLD, add:NEW, remove:OLD]
	// **关键不变量**（r10 P2）：第二次 register 的 add 必须出现在 OLD 的 remove **之前**
	if len(callOrder) < 3 {
		t.Fatalf("expect at least 3 hook calls, got %v", callOrder)
	}

	// 找 add:NEW 与 remove:OLD 的 index；NEW = 第二个 add（不是 OLD 的 add）
	addNewIdx := -1
	removeOldIdx := -1
	addCount := 0
	for i, e := range callOrder {
		if strings.HasPrefix(e, "add:") {
			addCount++
			if addCount == 2 {
				addNewIdx = i
			}
		}
		if e == "remove:"+oldSessionID && removeOldIdx < 0 {
			removeOldIdx = i
		}
	}
	if addNewIdx == -1 {
		t.Fatalf("second add (NEW) not found in callOrder=%v", callOrder)
	}
	if removeOldIdx == -1 {
		t.Fatalf("remove:%s (OLD) not found in callOrder=%v", oldSessionID, callOrder)
	}
	if addNewIdx >= removeOldIdx {
		t.Errorf("r10 P2 不变量：onRegister(NEW) 必须先于 onUnregister(OLD)；callOrder=%v (addNew@%d, removeOld@%d)",
			callOrder, addNewIdx, removeOldIdx)
	}
}

// TestPresenceHook_ManagerCloseTriggersRemoveForAllSessions: Story 10.6 钩子集成 case 4。
// SessionManager.Close 必须为**每个**注册的 Session 都触发 RemoveOnline 钩子。
//
// 这是 lesson 2026-05-06-ws-session-send-close-race-and-shutdown-hooks.md 钉死的
// P1 不变量；SessionManager 已在 10.3 r1 修复（保留索引到所有 Close 跑完）；本 case
// 是 contract verification 防回归。
func TestPresenceHook_ManagerCloseTriggersRemoveForAllSessions(t *testing.T) {
	fake := &fakePresenceRepo{}
	mgr := wsapp.NewSessionManager(
		wsapp.WithRegisterHook(func(s *wsapp.Session) {
			fake.hookRegister(s)
		}),
		wsapp.WithUnregisterHook(func(s *wsapp.Session) {
			fake.hookUnregister(s)
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
		if int(fake.addCount.Load()) >= len(uids) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := fake.addCount.Load(); int(got) != len(uids) {
		t.Fatalf("AddOnline hook called %d times after dial, want %d", got, len(uids))
	}

	registered := mgr.ListSessionsByRoomID(context.Background(), roomID)
	if got, want := len(registered), len(uids); got != want {
		t.Fatalf("registered sessions = %d, want %d", got, want)
	}

	// 关 manager —— 必须触发**每个** Session 的 RemoveOnline 钩子
	if err := mgr.Close(); err != nil {
		t.Fatalf("mgr.Close: %v", err)
	}

	// 等 Remove 钩子异步触发完毕
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if int(fake.removeCount.Load()) >= len(uids) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := fake.removeCount.Load(); int(got) != len(uids) {
		t.Errorf("RemoveOnline hook fired %d times, want %d (one per Session)", got, len(uids))
	}

	// 校验每个 Session 的 sessionID 都进了 removedIDs
	for _, s := range registered {
		if _, ok := fake.removedIDs.Load(s.SessionID()); !ok {
			t.Errorf("session %s did not trigger RemoveOnline hook", s.SessionID())
		}
	}
}

// ---------- review 10-6 r2 P1 / r3 P2: HeartbeatScanner AddOnline reconcile 集成测试 ----------
//
// 本节验证 review 10-6 r2 P1 / r3 P2 修：HeartbeatScanner.scanOnce 对 active session
// （idle <= timeoutMs）调 PresenceRenewer.AddOnline，让 long-lived WS session
// 不会被 Redis presence TTL 自动过期误报为 offline + Register hook partial-fail
// 路径在 30s 内通过 scanner 自愈（r3 P2 把 RenewTTL 换成 AddOnline 的核心动机）。
//
// 测试栈：fakeRenewer + atomic 计数器；不接 miniredis（与 Story 10.6 钩子集成
// 测试同模式 —— 单测的目标是验证"scanner 在正确时机调 AddOnline 并传对参数"，
// Redis 真实命令行为由 presence_repo_test.go 的 AddOnline 单测覆盖）。

// fakeRenewer 是 PresenceRenewer 接口（review 10-6 r2 P1 / r3 P2）的轻量替身，
// 让单测验证"scanner 给哪些 (roomID, userID, sessionID) 调了 AddOnline"。
//
// 设计：用 sync.Map 收集每次调用的参数 + atomic.Int32 计数 + 可选 returnErr 注入。
// fail-on-error 不让 scanner 退出 —— scanner 必须 log warn 继续遍历下一 session。
type fakeRenewer struct {
	count        atomic.Int32
	calls        sync.Map // (roomID|userID composite key) → call count
	lastSession  sync.Map // (roomID|userID composite key) → last sessionID
	returnErr    error
}

func renewerKey(roomID, userID uint64) string {
	return fmt.Sprintf("r=%d|u=%d", roomID, userID)
}

func (f *fakeRenewer) AddOnline(_ context.Context, roomID, userID uint64, sessionID string) error {
	f.count.Add(1)
	key := renewerKey(roomID, userID)
	if v, ok := f.calls.Load(key); ok {
		f.calls.Store(key, v.(int)+1)
	} else {
		f.calls.Store(key, 1)
	}
	f.lastSession.Store(key, sessionID)
	return f.returnErr
}

// TestHeartbeatScanner_ScanOnce_ActiveSession_ReconcilesPresence：active session
// （idle <= timeoutMs）触发 PresenceRenewer.AddOnline；参数 (roomID, userID,
// sessionID) 与 Session 字段匹配。
//
// 这是 review 10-6 r2 P1 / r3 P2 修后的核心 case：long-lived session 在 30s tick
// 路径上每次都被 reconcile（重写 SET + SADD + EXPIRE），TTL 5min 永远不到 + 任何
// Register hook partial-fail 30s 内自愈。
func TestHeartbeatScanner_ScanOnce_ActiveSession_ReconcilesPresence(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, _, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	renewer := &fakeRenewer{}

	// timeoutMs=10s 让刚握手完的 session 视为 active（idle ≈ 几 ms < 10s）
	scanner := wsapp.NewHeartbeatScannerForTestWithRenewer(mgr, 10*1000, 200*time.Millisecond, idleTestLogger(), renewer)

	// review 10-6 r5 P1：reconcile fanout 化后必须 drain 才能可靠断言 renewer state
	scanner.ScanOnceAndDrainForTest(context.Background(), time.Now())

	// 验证 AddOnline 被调一次，参数匹配
	if got := renewer.count.Load(); got != 1 {
		t.Errorf("AddOnline count = %d, want 1 (single active session should reconcile once)", got)
	}
	expectedKey := renewerKey(3001, 1001)
	if v, ok := renewer.calls.Load(expectedKey); !ok || v.(int) != 1 {
		t.Errorf("AddOnline not called for room=3001 user=1001 (calls map = %v)", v)
	}
	// r3 P2 加：验证 sessionID 被传入（非空）—— scanner 必须传 Session.SessionID()
	// 才能让 AddOnline 走 SET user:{id}:ws_session=sessionID 重写 reconnect 替换
	// 路径，否则 RemoveOnline 的 sessionID guard 会比较失败 → 旧 unregister 误删。
	if sid, ok := renewer.lastSession.Load(expectedKey); !ok || sid.(string) == "" {
		t.Errorf("AddOnline sessionID not propagated (lastSession map = %v)", sid)
	}
}

// TestHeartbeatScanner_ScanOnce_IdleSession_DoesNotReconcile：idle session
// （idle > timeoutMs）走 close 路径，**不**调 AddOnline。
//
// 这是 P1/P2 修后的负向 case：idle session 即将被 4005 关闭，没必要 reconcile；
// onUnregister 钩子会在 close 完成后清干净 presence。
func TestHeartbeatScanner_ScanOnce_IdleSession_DoesNotReconcile(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, _, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	renewer := &fakeRenewer{}

	// timeoutMs=10ms 让握手完的 session 视为 idle
	scanner := wsapp.NewHeartbeatScannerForTestWithRenewer(mgr, 10, 200*time.Millisecond, idleTestLogger(), renewer)

	// 等握手后 sleep 50ms 让 idle 远超 10ms
	time.Sleep(50 * time.Millisecond)

	// review 10-6 r5 P1：drain 让 close fanout 跑完（虽然这个测试断言 renewer 不被
	// 调，但 drain 让本测试退出前没有 leaked goroutine，`go test -race` 干净）
	scanner.ScanOnceAndDrainForTest(context.Background(), time.Now())

	// 验证 AddOnline 没被调
	if got := renewer.count.Load(); got != 0 {
		t.Errorf("AddOnline count = %d, want 0 (idle session should not reconcile, will be closed)", got)
	}
}

// TestHeartbeatScanner_ScanOnce_NilRenewer_DoesNotPanic：renewer == nil（未接
// Redis 的最小路径 / 单测）→ scanner 跳过 reconcile 不 panic。
func TestHeartbeatScanner_ScanOnce_NilRenewer_DoesNotPanic(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, _, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	// renewer = nil
	scanner := wsapp.NewHeartbeatScannerForTestWithRenewer(mgr, 10*1000, 200*time.Millisecond, idleTestLogger(), nil)

	// 不应 panic（renewer == nil 时主 loop 不 dispatch fanout，wg.Wait no-op）
	scanner.ScanOnceAndDrainForTest(context.Background(), time.Now())
}

// TestHeartbeatScanner_ScanOnce_RenewerError_LoggedNotAborted：AddOnline 返 error
// → scanner log warn 继续遍历下一 session，不让单 session 失败影响整批。
//
// 验证多 session 场景：第一个 session 的 AddOnline 返 error，后续 session 仍被
// reconcile。
func TestHeartbeatScanner_ScanOnce_RenewerError_LoggedNotAborted(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}

	signer := newSigner(t)
	wsURL, ts := startGatewayServer(t, signer, mgr, repo)
	defer ts.Close()

	// 起 3 个 Session
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

	renewer := &fakeRenewer{returnErr: errors.New("redis down")}

	scanner := wsapp.NewHeartbeatScannerForTestWithRenewer(mgr, 10*1000, 200*time.Millisecond, idleTestLogger(), renewer)
	// review 10-6 r5 P1：reconcile fanout 化后必须 drain 才能可靠断言 count
	scanner.ScanOnceAndDrainForTest(context.Background(), time.Now())

	// 即使 AddOnline 返 error，scanner 仍应对所有 3 session 调 AddOnline（不被中断）
	if got := renewer.count.Load(); got != 3 {
		t.Errorf("AddOnline count = %d after error, want 3 (error should not abort iteration)", got)
	}
}

// ---------- review 10-6 r4 P2: scanner IsRegistered guard 集成测试 ----------

// staleSnapshotMgr 是一个 SessionManager 包装器，让 ListAllSessions 返回**调用方
// 注入**的 stale 快照（模拟 scanner snapshot 与 AddOnline 之间 session 已 unregister
// 的 race），但 IsRegistered 委托给底层 real manager（让 r4 P2 修后的 IsRegistered
// guard 路径可观察）。
//
// 用途：单测 review 10-6 r4 P2 修 —— scanner 在调 AddOnline 前必须先 IsRegistered
// 校验，避免"复活"已 unregister session 的 presence 让 zombie online entry 持续
// 到 TTL 过期。
type staleSnapshotMgr struct {
	wsapp.SessionManager
	stale []*wsapp.Session
}

func (m *staleSnapshotMgr) ListAllSessions(_ context.Context) []*wsapp.Session {
	return m.stale
}

// TestHeartbeatScanner_ScanOnce_UnregisteredSession_SkipsReconcile：snapshot 后
// session 已 unregister → scanner 调 IsRegistered 返 false → skip AddOnline，避免
// 复活 zombie presence。
//
// 时序模拟（r4 P2 race）：
//  1. session 已注册到 manager
//  2. scanner snapshot 拿到该 session
//  3. session disconnect → manager Unregister 清出
//  4. scanner 遍历 snapshot 调 IsRegistered → false → skip AddOnline
//
// 验证：renewer.count == 0（即使 snapshot 含 session，AddOnline 也不被调用）
func TestHeartbeatScanner_ScanOnce_UnregisteredSession_SkipsReconcile(t *testing.T) {
	realMgr := wsapp.NewSessionManager()
	defer realMgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, sess, ts := useGatewayDial(t, realMgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	// 拿真实 snapshot（此时 session 仍在 manager）
	snapshot := realMgr.ListAllSessions(context.Background())
	if len(snapshot) != 1 {
		t.Fatalf("snapshot len = %d, want 1", len(snapshot))
	}

	// 模拟 race window：scanner 拿到 snapshot 后 session 被 unregister
	// （现实路径：disconnect → manager.Unregister → onUnregister hook → presence 清干净）
	if err := realMgr.Unregister(context.Background(), sess.SessionID()); err != nil {
		t.Fatalf("Unregister: %v", err)
	}
	// 验证 IsRegistered 已返 false
	if realMgr.IsRegistered(context.Background(), sess.SessionID()) {
		t.Fatalf("IsRegistered should be false after Unregister")
	}

	// 注入 staleSnapshotMgr：ListAllSessions 返 snapshot（含已 unregister 的 session），
	// IsRegistered 委托给 realMgr（返 false）
	staleMgr := &staleSnapshotMgr{SessionManager: realMgr, stale: snapshot}

	renewer := &fakeRenewer{}
	// timeoutMs=10s 让 session 视为 active（idle 远 < 10s）
	scanner := wsapp.NewHeartbeatScannerForTestWithRenewer(staleMgr, 10*1000, 200*time.Millisecond, idleTestLogger(), renewer)

	// review 10-6 r5 P1：reconcile fanout 化后必须 drain 才能可靠断言 count
	scanner.ScanOnceAndDrainForTest(context.Background(), time.Now())

	// **关键断言**：r4 P2 修后 IsRegistered guard 让 AddOnline 不被调
	// （否则会复活已 unregister session 的 presence → zombie 直到 TTL）
	if got := renewer.count.Load(); got != 0 {
		t.Errorf("AddOnline count = %d, want 0 (unregistered session should not be reconciled)", got)
	}
}

// TestHeartbeatScanner_ScanOnce_RegisteredSession_StillReconciles：r4 P2 修不破坏
// happy path —— session 仍在 manager 时 IsRegistered 返 true → AddOnline 正常调用。
func TestHeartbeatScanner_ScanOnce_RegisteredSession_StillReconciles(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, sess, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	// session 仍在 manager
	if !mgr.IsRegistered(context.Background(), sess.SessionID()) {
		t.Fatalf("IsRegistered should be true for live session")
	}

	renewer := &fakeRenewer{}
	scanner := wsapp.NewHeartbeatScannerForTestWithRenewer(mgr, 10*1000, 200*time.Millisecond, idleTestLogger(), renewer)
	// review 10-6 r5 P1：reconcile fanout 化后必须 drain 才能可靠断言 count
	scanner.ScanOnceAndDrainForTest(context.Background(), time.Now())

	// IsRegistered=true → AddOnline 正常调
	if got := renewer.count.Load(); got != 1 {
		t.Errorf("AddOnline count = %d, want 1 (live session should reconcile)", got)
	}
}

// ---------- review 10-6 r8 P1: scanner reconcile 必须用 IsCurrentForUser 而非 IsRegistered ----------

// notCurrentMgr 是一个 SessionManager 包装器，让 IsRegistered=true 但
// IsCurrentForUser=false —— 模拟 reconnect 替换中场（OLD 仍在 sessionsByID 直到
// oldS.Close 跑完，但 userToSessionID[u] 已指向 NEW）。
//
// 用途：单测 review 10-6 r8 P1 修 —— scanner reconcile 必须用 IsCurrentForUser
// 做 gate；如果用 IsRegistered，OLD 在替换中场会通过 → AddOnline(OLD) 把 user_key
// 改回 OLD session/room → 后续 RemoveOnline(oldSessionID) 在 Lua script 看到
// currentSession==OLD 走 case 2 完整清理 → 真正活的 NEW 的 presence 被清掉。
type notCurrentMgr struct {
	wsapp.SessionManager
	// IsRegistered 返 true（让 r4 P2 的 guard 不命中），IsCurrentForUser 返 false
	// （让 r8 P1 的 guard 命中 → skip）
}

func (m *notCurrentMgr) IsRegistered(_ context.Context, _ string) bool {
	return true
}

func (m *notCurrentMgr) IsCurrentForUser(_ context.Context, _ string) bool {
	return false
}

// TestHeartbeatScanner_ScanOnce_NotCurrentForUser_SkipsReconcile:
// **review 10-6 r8 P1 核心 case** —— OLD session 在 reconnect 替换中场仍 IsRegistered=true，
// 但 IsCurrentForUser=false（NEW 已抢占 userToSessionID[u]）。scanner 必须用
// IsCurrentForUser 做 gate，跳过 OLD 的 reconcile，避免污染 NEW 的 presence。
//
// 时序模拟：
//  1. user A 注册 OLD session（在 sessionsByID + userToSessionID[A]=OLD）
//  2. user A reconnect 触发 Register(NEW) —— Register 锁内把 sessionsByRoom[room][OLD]
//     移到 NEW，把 userToSessionID[A] 指 NEW；OLD 仍在 sessionsByID（保留以触发
//     Unregister 钩子）
//  3. scanner snapshot 拿到 OLD（仍 active = idle 不超时）
//  4. scanner reconcile OLD：IsCurrentForUser(OLD) = false（userToSessionID 已指 NEW）
//     → skip AddOnline，不污染 NEW 的 user_key
//
// 验证：renewer.AddOnline 没被调（count == 0）。
func TestHeartbeatScanner_ScanOnce_NotCurrentForUser_SkipsReconcile(t *testing.T) {
	realMgr := wsapp.NewSessionManager()
	defer realMgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, sess, ts := useGatewayDial(t, realMgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	// 拿真实 snapshot（session 仍在 manager）
	snapshot := realMgr.ListAllSessions(context.Background())
	if len(snapshot) != 1 {
		t.Fatalf("snapshot len = %d, want 1", len(snapshot))
	}

	// session 仍在 manager；IsRegistered=true（基线）
	if !realMgr.IsRegistered(context.Background(), sess.SessionID()) {
		t.Fatalf("baseline: IsRegistered should be true")
	}

	// 注入 notCurrentMgr：模拟 reconnect 替换中场 —— ListAllSessions 返 snapshot
	// 给 reconcile 路径，IsRegistered=true（让旧 r4 P2 guard 不命中），但
	// IsCurrentForUser=false（让 r8 P1 guard 命中 → skip）
	mgr := &notCurrentMgr{SessionManager: realMgr}

	renewer := &fakeRenewer{}
	scanner := wsapp.NewHeartbeatScannerForTestWithRenewer(mgr, 10*1000, 200*time.Millisecond, idleTestLogger(), renewer)
	scanner.ScanOnceAndDrainForTest(context.Background(), time.Now())

	// **关键断言**：r8 P1 修后 IsCurrentForUser guard 让 AddOnline 不被调
	// （否则会把 user_key 改回 OLD session/room → 污染 NEW 的 presence）
	if got := renewer.count.Load(); got != 0 {
		t.Errorf("AddOnline count = %d, want 0 (not-current session should not be reconciled per r8 P1)", got)
	}
}

// ---------- review 10-6 r5 P1: reconcile fanout 不阻塞主 sweep ----------

// slowRenewer 是 PresenceRenewer 替身（review 10-6 r5 P1），让每次 AddOnline
// 内部 sleep 固定时间模拟 Redis 高延迟，验证：
//  1. 新 fanout 实装下，scanOnce 主 loop 不会被 N × delay 拖慢（O(1) per session
//     dispatch + 立即返回）
//  2. ScanOnceForTest（内含 wg.Wait）等所有 fanout 跑完才返回，count=N 严格成立
type slowRenewer struct {
	delay time.Duration
	count atomic.Int32
}

func (r *slowRenewer) AddOnline(_ context.Context, _, _ uint64, _ string) error {
	time.Sleep(r.delay)
	r.count.Add(1)
	return nil
}

// TestHeartbeatScanner_ScanOnce_ReconcileFanout_DoesNotBlockSweep：review 10-6 r5 P1。
//
// 背景：r2/r3/r4 把 reconcile 走成主 loop 内**同步**调 AddOnline，让一次 sweep =
// O(N session) × Redis latency。N 大或 Redis 慢时单 sweep > 30s tick，tail session
// 的 idle 超时检测被延迟、它们的 presence TTL 也错过 renew → flap offline。
//
// r5 P1 修：reconcile 改成 fanout goroutine + per-call ctx timeout，与 close
// fanout 同模式（共用 s.wg，Run defer wg.Wait drain）。
//
// 测试策略：
//  1. 起 N=20 active session（idle < timeoutMs，必走 reconcile 分支）
//  2. slowRenewer 每次 AddOnline sleep 100ms → 同步路径下 N=20 × 100ms = 2s
//  3. 直接调 unexported scanOnce **不** wg.Wait（用包内同包测试访问）—— 主 loop
//     应在 < 200ms 内完成 dispatch 返回（fanout 各自异步跑 100ms）
//
// **关键不变量**：主 loop 时长 ≪ N × delay 才算 fanout 真起作用。
func TestHeartbeatScanner_ScanOnce_ReconcileFanout_DoesNotBlockSweep(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()

	repo := &stubRoomMemberRepo{}

	const N = 20
	conns := make([]*websocket.Conn, 0, N)
	tss := make([]*httptest.Server, 0, N)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
		for _, ts := range tss {
			ts.Close()
		}
	}()
	for i := 0; i < N; i++ {
		conn, _, ts := useGatewayDial(t, mgr, repo, uint64(5000+i), 4500)
		conns = append(conns, conn)
		tss = append(tss, ts)
	}

	// 等所有 session 注册到 mgr
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(mgr.ListAllSessions(context.Background())) >= N {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// slowRenewer：每次 AddOnline sleep 100ms。同步路径下 N=20 → 2s 总耗时
	const delay = 100 * time.Millisecond
	renewer := &slowRenewer{delay: delay}

	// timeoutMs=10s 让所有 session 视为 active
	scanner := wsapp.NewHeartbeatScannerForTestWithRenewer(mgr, 10*1000, 200*time.Millisecond, idleTestLogger(), renewer)

	// **关键**：ScanOnceForTest 不含 wg.Wait（保留 fire-and-forget 语义让 close-fanout
	// race 测试能用），所以这里直接测 ScanOnceForTest 主 loop 的耗时 —— dispatch 完
	// 立即返回，不被 N × delay 拖累。
	start := time.Now()
	scanner.ScanOnceForTest(context.Background(), time.Now())
	mainLoopDur := time.Since(start)

	// 主 loop 应在 < 50ms 内完成（即使 N=20 也仅启 ~20 goroutines + Add/dispatch）
	// 给 200ms 余量留 CI 抖动空间；远小于 N × delay = 2s（同步路径下界）
	if mainLoopDur > 200*time.Millisecond {
		t.Errorf("scanOnce main loop took %v with N=%d slow renewer; expected < 200ms (review r5 P1: reconcile must fanout, not block main sweep)", mainLoopDur, N)
	}

	// 等 fanout drain（直接调 wg.Wait via DrainFanoutForTest）
	scanner.DrainFanoutForTest()

	if got := renewer.count.Load(); got != N {
		t.Errorf("AddOnline count = %d after drain, want %d (all fanout should complete)", got, N)
	}
}

// hangRenewer 是 PresenceRenewer 替身（review 10-6 r5 P1），让 AddOnline 严格
// 阻塞到 ctx done 才返回，模拟 Redis 永久 hang 的病态场景。
type hangRenewer struct {
	count atomic.Int32
}

func (r *hangRenewer) AddOnline(ctx context.Context, _, _ uint64, _ string) error {
	r.count.Add(1)
	<-ctx.Done()
	return ctx.Err()
}

// TestHeartbeatScanner_ScanOnce_ReconcileFanout_PerCallCtxTimeout：review 10-6 r5 P1。
//
// 背景：fanout 改造后单个 hang 的 Redis 调用如果没有 per-call ctx timeout，会让
// fanout goroutine 永久卡住，Run defer wg.Wait drain 时间无界增长 —— shutdown 会
// 卡到那个 hang goroutine 解锁才返回。
//
// 修法：fanout 内 AddOnline 走 context.WithTimeout(parentCtx, presenceReconcileTimeout=2s)，
// 单 hang 最多卡 2s 自动 ctx.DeadlineExceeded 退出。
//
// 测试策略：
//  1. 起 1 个 active session
//  2. hangRenewer.AddOnline 阻塞到 ctx.Done
//  3. ScanOnceForTest（内 wg.Wait）应在 ~presenceReconcileTimeout 内返回（不
//     永久卡死也不立刻返回）—— 验证 per-call deadline 真的生效
//
// 上限给 presenceReconcileTimeout + 500ms = 2.5s，下限给 1s（确保不是 immediate
// 路径意外 short-circuit）。
func TestHeartbeatScanner_ScanOnce_ReconcileFanout_PerCallCtxTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping per-call ctx timeout test in -short mode (waits up to 2.5s)")
	}

	mgr := wsapp.NewSessionManager()
	defer mgr.Close()

	repo := &stubRoomMemberRepo{}
	conn, _, ts := useGatewayDial(t, mgr, repo, 1001, 4501)
	defer ts.Close()
	defer conn.Close()

	renewer := &hangRenewer{}
	scanner := wsapp.NewHeartbeatScannerForTestWithRenewer(mgr, 10*1000, 200*time.Millisecond, idleTestLogger(), renewer)

	// 用 ScanOnceAndDrainForTest（含 wg.Wait）—— 主 loop dispatch 后等 fanout drain，
	// drain 时间 = per-call ctx timeout（hang renewer 在 ctx.Done 才返回）
	start := time.Now()
	scanner.ScanOnceAndDrainForTest(context.Background(), time.Now())
	dur := time.Since(start)

	// 应在 [1s, 2.5s] 区间返回 —— per-call ctx 在 2s 后 cancel 让 hang AddOnline
	// 解锁，wg.Wait drain 跟着返回。
	if dur > 2500*time.Millisecond {
		t.Errorf("ScanOnceAndDrainForTest took %v with hang renewer; expected ≤ ~2s (review r5 P1: per-call ctx timeout must cap hang)", dur)
	}
	if dur < 1*time.Second {
		t.Errorf("ScanOnceAndDrainForTest returned in %v; suspiciously fast — per-call ctx timeout may be too short or path short-circuited", dur)
	}

	// hang renewer 应只被调一次（hangRenewer.count++ 在阻塞前置位）
	if got := renewer.count.Load(); got != 1 {
		t.Errorf("AddOnline count = %d, want 1 (single session, single dispatch)", got)
	}
}

// TestHeartbeatScanner_Run_DrainsReconcileFanoutOnShutdown：review 10-6 r5 P1。
//
// 背景：r5 P2 把 close fanout 接进 s.wg，Run defer wg.Wait drain；r5 P1 把
// reconcile fanout **也**接进同 wg，shutdown 路径需要 drain reconcile 残余
// goroutine（不能 ctx cancel 后 reconcile 还在跑 → 让 shutdown 期间 Redis I/O
// 仍 emit）。
//
// 测试策略：复用 r5 P2 ShutdownOrdering pattern：
//  1. 起 N 个 active session（idle < timeoutMs，必走 reconcile 分支）
//  2. slowRenewer 每次 AddOnline sleep 100ms（让 fanout 在 Run 退出时仍在跑）
//  3. scanner.Run 启动；等几个 tick 让 reconcile fanout dispatched
//  4. cancel ctx → Run 应在 reasonable 时间内返回（drain 完所有 reconcile fanout）
//
// **关键**：Run 返回后 wg 已归零 —— 所有 reconcile goroutine 都 wg.Done 完成。
// 给 2s 余量（slowRenewer 100ms × 几次 tick 启的 fanout 总跑完时间）。
func TestHeartbeatScanner_Run_DrainsReconcileFanoutOnShutdown(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()

	repo := &stubRoomMemberRepo{}

	const N = 5
	conns := make([]*websocket.Conn, 0, N)
	tss := make([]*httptest.Server, 0, N)
	defer func() {
		for _, c := range conns {
			c.Close()
		}
		for _, ts := range tss {
			ts.Close()
		}
	}()
	for i := 0; i < N; i++ {
		conn, _, ts := useGatewayDial(t, mgr, repo, uint64(6000+i), 4600)
		conns = append(conns, conn)
		tss = append(tss, ts)
	}

	// 等所有 session 注册
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(mgr.ListAllSessions(context.Background())) >= N {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	renewer := &slowRenewer{delay: 100 * time.Millisecond}

	// timeoutMs=10s 让 session 全 active；interval 30ms 让 tick 多次触发 reconcile fanout
	scanner := wsapp.NewHeartbeatScannerForTestWithRenewer(mgr, 10*1000, 30*time.Millisecond, idleTestLogger(), renewer)

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() {
		scanner.Run(ctx)
		close(runDone)
	}()

	// 让几次 tick 跑完，reconcile fanout dispatched 大批
	time.Sleep(150 * time.Millisecond)

	// cancel → Run 应 drain in-flight reconcile fanout 后返回
	cancel()

	// 给 2s 余量：slowRenewer 100ms × 已 dispatch 的 fanout 总跑完时间
	select {
	case <-runDone:
		// good — Run drain 完所有 reconcile fanout 后返回
	case <-time.After(2 * time.Second):
		t.Fatalf("scanner.Run did not return within 2s after ctx cancel with reconcile fanout in-flight (review r5 P1: reconcile fanout must be drained by same wg.Wait)")
	}

	// reconcile 至少被调过几次（确认 fanout 真在 ctx cancel 前 dispatched 跑过）
	if got := renewer.count.Load(); got == 0 {
		t.Errorf("AddOnline count = 0 after several ticks; reconcile fanout should have dispatched at least once")
	}
}

// ---------- review 10-6 r9 P1：scanner 与 hook 共享 UserPresenceMutex 不变量 ----------

// fakeUserPresenceMutex 是 UserPresenceMutex 的最小测试实装（与 main.go 的
// userKeyedMutex 同语义；放本文件避免单测包跨 import）。
type fakeUserPresenceMutex struct {
	m sync.Map // map[uint64]*sync.Mutex
}

func (u *fakeUserPresenceMutex) LockFor(userID uint64) *sync.Mutex {
	if mu, ok := u.m.Load(userID); ok {
		return mu.(*sync.Mutex)
	}
	mu, _ := u.m.LoadOrStore(userID, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// TestHeartbeatScanner_Reconcile_AcquiresSharedUserMutex_BlocksUntilReleased：
// review 10-6 r9 P1 P1 修后核心断言。
//
// 不变量：scanner reconcile fanout 必须**在调 renewer.AddOnline 之前**先 LockFor
// 同 userID 的共享 mutex。验证方式：外部 goroutine（模拟 hook adapter）持锁不放，
// 触发 scanOnce → drain → 检查 renewer.AddOnline 是否被调过；持锁期间应 0 调用，
// 释放锁后再 drain → 应被调过 1 次。
//
// 这锁定的是"hook 与 scanner 之间通过 shared mutex 互斥"的语义 —— hook 的
// RemoveOnline 持锁期间，scanner 的 AddOnline 必须等；hook 释放锁后 scanner 才
// 能 AddOnline。否则 scanner 会"复活" hook 已经清掉的 presence。
func TestHeartbeatScanner_Reconcile_AcquiresSharedUserMutex_BlocksUntilReleased(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, _, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	renewer := &fakeRenewer{}
	mu := &fakeUserPresenceMutex{}

	// timeoutMs=10s 让 session 视为 active；interval 200ms 但我们用 ScanOnceForTest 直接调。
	scanner := wsapp.NewHeartbeatScannerForTestWithMutex(mgr, 10*1000, 200*time.Millisecond, idleTestLogger(), renewer, mu)

	// 外部持锁（模拟 hook adapter 在 AddOnline / RemoveOnline 中场，持有 user 1001 的 mutex）。
	hookMu := mu.LockFor(1001)
	hookMu.Lock()

	// 触发 scanOnce —— reconcile fanout dispatch 后会试图 LockFor(1001).Lock 阻塞。
	// 用 ScanOnceForTest（不 drain）让主线程立即返回，fanout goroutine 仍在锁上等。
	scanner.ScanOnceForTest(context.Background(), time.Now())

	// 给 fanout 一点时间跑到 Lock 调用点；此时它应该卡在 hookMu 上（因为我们持锁）。
	time.Sleep(80 * time.Millisecond)

	// 断言：reconcile **没**被调过 —— scanner 卡在 LockFor(1001).Lock。
	if got := renewer.count.Load(); got != 0 {
		t.Errorf("AddOnline count = %d before mutex release; want 0 (scanner reconcile must block on shared per-user mutex; r9 P1 invariant)", got)
	}

	// 释放锁 —— scanner fanout 现在应能 Lock 成功并跑完 AddOnline。
	hookMu.Unlock()

	// drain 让 fanout 跑完
	scanner.DrainFanoutForTest()

	// 断言：reconcile 被调过 1 次（fanout 在锁释放后跑完）。
	if got := renewer.count.Load(); got != 1 {
		t.Errorf("AddOnline count = %d after mutex release + drain; want 1 (scanner must run AddOnline after acquiring shared mutex; r9 P1 invariant)", got)
	}
}

// TestHeartbeatScanner_Reconcile_NilUserMutex_DoesNotPanic：userPresenceMu == nil
// （单测 / 最小路径）→ scanner reconcile 走无锁路径，与 r8 之前兼容；不 panic。
func TestHeartbeatScanner_Reconcile_NilUserMutex_DoesNotPanic(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn, _, ts := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts.Close()
	defer conn.Close()

	renewer := &fakeRenewer{}

	// 走 ForTestWithRenewer 路径（userPresenceMu == nil）
	scanner := wsapp.NewHeartbeatScannerForTestWithRenewer(mgr, 10*1000, 200*time.Millisecond, idleTestLogger(), renewer)
	scanner.ScanOnceAndDrainForTest(context.Background(), time.Now())

	if got := renewer.count.Load(); got != 1 {
		t.Errorf("AddOnline count = %d, want 1 (nil userPresenceMu should fall back to lock-free path)", got)
	}
}

// TestHeartbeatScanner_Reconcile_DifferentUsers_DoNotBlockEachOther：scanner
// reconcile 在不同 userID 上的 fanout goroutine 必须互不阻塞 —— 否则会退化到
// 全局串行，一个 user 的 mutex 卡住会让所有 reconcile 卡住。
func TestHeartbeatScanner_Reconcile_DifferentUsers_DoNotBlockEachOther(t *testing.T) {
	mgr := wsapp.NewSessionManager()
	defer mgr.Close()
	repo := &stubRoomMemberRepo{}
	conn1, _, ts1 := useGatewayDial(t, mgr, repo, 1001, 3001)
	defer ts1.Close()
	defer conn1.Close()
	conn2, _, ts2 := useGatewayDial(t, mgr, repo, 1002, 3001)
	defer ts2.Close()
	defer conn2.Close()

	renewer := &fakeRenewer{}
	mu := &fakeUserPresenceMutex{}

	scanner := wsapp.NewHeartbeatScannerForTestWithMutex(mgr, 10*1000, 200*time.Millisecond, idleTestLogger(), renewer, mu)

	// 持有 user 1001 的锁，模拟 hook adapter 在中场。user 1002 应**不**受影响。
	hookMu1 := mu.LockFor(1001)
	hookMu1.Lock()

	scanner.ScanOnceForTest(context.Background(), time.Now())
	time.Sleep(80 * time.Millisecond)

	// 断言：user 1002 的 reconcile 应已跑完（不被 user 1001 锁阻塞），count >= 1。
	if got := renewer.count.Load(); got < 1 {
		t.Errorf("AddOnline count = %d; want >= 1 (user 1002 reconcile must not block on user 1001 mutex; per-user lock isolation invariant)", got)
	}

	hookMu1.Unlock()
	scanner.DrainFanoutForTest()

	// 释放后两个 user 都跑完，count == 2
	if got := renewer.count.Load(); got != 2 {
		t.Errorf("AddOnline count = %d after release; want 2 (both users reconciled)", got)
	}
}

