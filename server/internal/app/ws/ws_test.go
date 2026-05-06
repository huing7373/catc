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
	gateway := wsapp.NewGateway(signer, mgr, repo, cfg)
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
	gateway := wsapp.NewGateway(signer, mgr, repo, cfg)
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
