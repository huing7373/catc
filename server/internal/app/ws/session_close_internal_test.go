package ws

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestSession_Close_FastWhenWriteLoopNotStarted: review r2 P2 回归。
//
// 场景：Gateway.Handle 的 handshake 失败路径（ListMembers / buildSnapshot /
// snapshot WriteMessage / Register 失败）会在启动 readLoop/writeLoop **之前**
// 调 session.Close 释放资源。此时 writeLoopStarted=false → closeInternal 必须
// 跳过 writeLoopDone wait，否则每次失败 handshake 都付 +500ms tax。
//
// 断言：Close 必须在 50ms 内返回（远小于 closeFrameWriteDeadline=500ms）。
func TestSession_Close_FastWhenWriteLoopNotStarted(t *testing.T) {
	conn, cleanup := newPipeWebsocketConn(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := newSession("", 1001, 3001, conn, logger, 16384, 2*time.Second)

	start := time.Now()
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	elapsed := time.Since(start)

	// 50ms 上限：本路径理论是亚毫秒（不进 wait 分支），50ms 给 Windows / CI 时序
	// 抖动留充足余量；任何 >100ms 都说明 wait 分支被错误命中（regression 信号）。
	if elapsed > 50*time.Millisecond {
		t.Errorf("Close took %v with writeLoop not started, want < 50ms (review r2 P2: should skip writeLoopDone wait when writeLoopStarted=false)", elapsed)
	}
}

// TestSession_CloseWithCode_FastWhenWriteLoopNotStarted: review r2 P2 回归
// （CloseWithCode 路径同样需要短路 wait）。
//
// CloseWithCode 在 happy path 下确实需要 wait writeLoop 退出后才 WriteControl
// （review r1 P2 顺序约束）；但 writeLoop 没启动过的场景下 wait 永远等不到 →
// 必须用 writeLoopStarted gate 跳过 wait，直接 WriteControl + 释放资源。
func TestSession_CloseWithCode_FastWhenWriteLoopNotStarted(t *testing.T) {
	conn, cleanup := newPipeWebsocketConn(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := newSession("", 1001, 3001, conn, logger, 16384, 2*time.Second)

	start := time.Now()
	// CloseWithCode 在 writeLoopStarted=false 下也应该快（跳过 wait）；
	// WriteControl 自身是 best-effort，对端虽是 net.Pipe（无 ws 协议握手），
	// gorilla WriteControl 写完即返。
	_ = s.CloseWithCode(1011, "snapshot build failed")
	elapsed := time.Since(start)

	if elapsed > 50*time.Millisecond {
		t.Errorf("CloseWithCode took %v with writeLoop not started, want < 50ms", elapsed)
	}
}

// TestSession_Close_AfterWriteLoopStarted_StillWaits: review r2 P2 不能 regress
// r1 P2 —— writeLoop 已启动的场景下 Close 必须仍然等 writeLoop 退出（防 close
// frame 与 data frame 顺序错乱）。
//
// 断言：writeLoop 启动 → 调 Close → writeLoopDone 在 Close 返回前已 close。
func TestSession_Close_AfterWriteLoopStarted_StillWaits(t *testing.T) {
	conn, cleanup := newPipeWebsocketConn(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := newSession("", 1001, 3001, conn, logger, 16384, 2*time.Second)

	// 启动 writeLoop（writeLoopStarted 入口立即翻 true）
	go s.writeLoop()
	// 等 writeLoop 真正进入循环（writeLoopStarted=true）
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if s.writeLoopStarted.Load() {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !s.writeLoopStarted.Load() {
		t.Fatal("writeLoop did not start within 500ms")
	}

	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Close 返回时 writeLoopDone 必须已 close（wait 分支生效）。
	select {
	case <-s.writeLoopDone:
		// 期望路径
	default:
		t.Error("writeLoopDone not closed after Close returned (review r1 P2 regression: wait branch did not fire)")
	}
}

// newPipeWebsocketConn 用 net.Pipe + httptest.Server 构造一个真实的
// *websocket.Conn（server 侧）+ cleanup，用于不需要真实 wire IO 的 Session
// 单测。Session 关心的字段（conn / sendChan / writeLoopDone / writeLoopStarted）
// 都能通过 newSession 正常初始化，conn.Close 也能正常调用（gorilla 内部 net.Conn
// Close 走 net.Pipe 的 Close → 不 panic）。
//
// 实现：起一个 httptest.Server + Upgrader，dial 后立即返回 server-side conn。
func newPipeWebsocketConn(t *testing.T) (*websocket.Conn, func()) {
	t.Helper()
	upgrader := &websocket.Upgrader{}

	connCh := make(chan *websocket.Conn, 1)
	errCh := make(chan error, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			errCh <- err
			return
		}
		connCh <- c
	})
	ts := httptest.NewServer(mux)

	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/ws"
	u, err := url.Parse(wsURL)
	if err != nil {
		ts.Close()
		t.Fatalf("url.Parse: %v", err)
	}

	netConn, err := net.Dial("tcp", u.Host)
	if err != nil {
		ts.Close()
		t.Fatalf("net.Dial: %v", err)
	}
	clientConn, _, err := websocket.NewClient(netConn, u, http.Header{}, 1024, 1024)
	if err != nil {
		ts.Close()
		t.Fatalf("websocket.NewClient: %v", err)
	}

	var serverConn *websocket.Conn
	select {
	case serverConn = <-connCh:
	case e := <-errCh:
		ts.Close()
		t.Fatalf("server upgrade: %v", e)
	case <-time.After(2 * time.Second):
		ts.Close()
		t.Fatal("server upgrade timeout")
	}

	cleanup := func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
		ts.Close()
	}
	return serverConn, cleanup
}
