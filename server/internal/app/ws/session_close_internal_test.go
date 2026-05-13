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
	s := newSession("", 1001, 3001, conn, logger, 16384, 2*time.Second, nil)

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
	s := newSession("", 1001, 3001, conn, logger, 16384, 2*time.Second, nil)

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

// TestSession_CloseWithCode_AfterWriteLoopStarted_StillWaits: review r1 P2 +
// r4 P1 联合不变量。
//
// CloseWithCode（emitClose=true）路径**必须**等 writeLoop 退出后再 WriteControl
// close frame（V1 §12.1 协议钦定 close frame 必须是 wire 最后一帧）。r4 P1 把
// plain Close 路径的 wait 拆掉了，但 CloseWithCode 路径必须保留 wait。
//
// 断言：writeLoop 启动 → 调 CloseWithCode → writeLoopDone 在 CloseWithCode
// 返回前已 close。
func TestSession_CloseWithCode_AfterWriteLoopStarted_StillWaits(t *testing.T) {
	conn, cleanup := newPipeWebsocketConn(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := newSession("", 1001, 3001, conn, logger, 16384, 2*time.Second, nil)

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

	if err := s.CloseWithCode(4005, "heartbeat timeout"); err != nil {
		t.Fatalf("CloseWithCode: %v", err)
	}

	// CloseWithCode 返回时 writeLoopDone 必须已 close（wait 分支生效）。
	select {
	case <-s.writeLoopDone:
		// 期望路径
	default:
		t.Error("writeLoopDone not closed after CloseWithCode returned (review r1 P2 regression: wait branch did not fire)")
	}
}

// TestSession_Close_AfterWriteLoopStarted_DoesNotWait: review r4 P1 回归。
//
// **背景**（r4 P1）：原版 closeInternal 无论 emitClose=true（CloseWithCode）还是
// false（plain Close）都 wait writeLoopDone 直到 closeWaitTimeout（writeTimeout +
// 200ms ≈ 5.2s）。CloseWithCode 需要 wait 是为了保证 close frame ordering，但
// plain Close 没有 close frame 要写 → 不需要任何 frame ordering 保证 → wait 是
// pure overhead。让 plain Close 等 5s+ 直接拖垮：
//   - SessionManager.Register 替换路径调 replaced.Close()
//   - SessionManager.Close 批量 s.Close()
// 两个 production 关键路径都被 5.2s tax 拖慢一倍。
//
// 修法：closeInternal 仅在 emitClose=true 时 wait writeLoopDone；emitClose=false
// 直接跳过 wait，sendChan 已关 → writeLoop 下次 select 自然命中关闭分支退出。
//
// 测试策略：起 session + writeLoop，调 plain Close()。Close 应在 50ms 内返回 ——
// **不**等 writeLoop 退出（writeLoop 自己后台慢慢退出无所谓，调用方不被卡住）。
//
// 关键不变量（与上面 CloseWithCode 测试合起来）：
//   - emitClose=true → wait writeLoopDone（协议正确性）
//   - emitClose=false → 不 wait（性能正确性）
func TestSession_Close_AfterWriteLoopStarted_DoesNotWait(t *testing.T) {
	conn, cleanup := newPipeWebsocketConn(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	s := newSession("", 1001, 3001, conn, logger, 16384, 2*time.Second, nil)

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

	start := time.Now()
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	elapsed := time.Since(start)

	// 50ms 上限：plain Close 不需要 wait 任何 frame ordering，关 sendChan + cancel
	// ctx + close conn 是纯本地操作，亚毫秒级。50ms 给 Windows / CI 时序抖动留余量。
	// 任何 >100ms 都说明 wait 分支被错误命中（r4 P1 regression）。
	if elapsed > 50*time.Millisecond {
		t.Errorf("Close took %v with writeLoop started, want < 50ms (review r4 P1: plain Close must skip writeLoopDone wait — it has no close frame ordering needs)", elapsed)
	}
}

// TestSession_Close_FastEvenWhenWriteLoopBlockedOnWrite: review r4 P1 强化场景。
//
// 极端场景：writeLoop 当前卡在 conn.WriteMessage（client 死掉但 TCP 还没 RST）。
// 此时 writeLoop 在 writeTimeout(2s) 内不会退出。原版 closeInternal 让 plain
// Close 等到 closeWaitTimeout = writeTimeout + 200ms = 2.2s 才返回。r4 修后
// plain Close 应立即返回（不再等 writeLoop）。
//
// 测试策略：起 session，**手工**入队一个数据让 writeLoop 在 net.Pipe 卡住
// （pipe peer 不读取 → conn.WriteMessage 走 SetWriteDeadline 挂到 writeTimeout）；
// 主测试 thread 调 plain Close → 必须 <= writeTimeout 一个数量级内返回（< 100ms
// 给抖动余量）。
func TestSession_Close_FastEvenWhenWriteLoopBlockedOnWrite(t *testing.T) {
	conn, cleanup := newPipeWebsocketConn(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// writeTimeout=2s 让对照清晰：原版会等 2.2s，r4 后应 < 100ms
	s := newSession("", 1001, 3001, conn, logger, 16384, 2*time.Second, nil)

	// 启动 writeLoop
	go s.writeLoop()
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

	// 入队一个 frame；client 在 newPipeWebsocketConn 里 dial 了但**不**主动读 ——
	// gorilla 的 net.Conn 用底层 net.Dial("tcp")，read 不发生时 OS 内核 socket
	// buffer 容量决定 server 多久卡住。本测试不强求"必卡到 writeTimeout 整段"
	// （那依赖 OS）；只要 writeLoop 此刻**正在** WriteMessage 内（goroutine
	// 不在 select），plain Close 都不应该 wait —— writeLoop 退出独立于本调用。
	//
	// 注：哪怕 writeLoop 立即写完，r4 修复后 plain Close 也跳过 wait（不等
	// writeLoopDone），所以这条路径在 fast/slow client 下都应该 < 100ms。
	_ = s.Send([]byte(`{"type":"x","requestId":"","payload":{},"ts":0}`))

	start := time.Now()
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	elapsed := time.Since(start)

	// 100ms 上限：r4 修后 plain Close 是纯本地操作（关 chan + cancel + close conn），
	// 与 writeLoop 是否仍卡在 WriteMessage 解耦。原版会卡 writeTimeout+200ms=2.2s。
	if elapsed > 100*time.Millisecond {
		t.Errorf("Close took %v with writeLoop in-progress, want < 100ms (review r4 P1: plain Close must NOT wait writeLoopDone)", elapsed)
	}
}

// TestSession_CloseWaitTimeout_EqualsWriteTimeoutPlusBuffer: review r3 P2 不变量
// 单测。
//
// 不变量：closeWaitTimeout = writeTimeout + closeWaitBufferDuration（200ms）。
// 原因（详见 session.go closeFrameWriteDeadline 注释）：closeInternal 等
// writeLoop 退出的上限**必须** ≥ writeTimeout，否则 writeLoop 在 conn.WriteMessage
// 卡住时 wait 提前超时 → WriteControl 写出 close frame 后 writeLoop 才结束写出
// data frame，wire 上 close frame 后跟 data frame 违反 V1 §12.1 协议。
func TestSession_CloseWaitTimeout_EqualsWriteTimeoutPlusBuffer(t *testing.T) {
	conn, cleanup := newPipeWebsocketConn(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cases := []struct {
		name         string
		writeTimeout time.Duration
		want         time.Duration
	}{
		{"5s default", 5 * time.Second, 5*time.Second + 200*time.Millisecond},
		{"2s startGatewayServer", 2 * time.Second, 2*time.Second + 200*time.Millisecond},
		{"100ms small (single-test)", 100 * time.Millisecond, 100*time.Millisecond + 200*time.Millisecond},
		// writeTimeout=0 → fall back 到 closeFrameWriteDeadline + buffer (500ms+200ms = 700ms)
		{"0 fallback", 0, 500*time.Millisecond + 200*time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newSession("", 1001, 3001, conn, logger, 16384, tc.writeTimeout, nil)
			if got := s.closeWaitTimeout; got != tc.want {
				t.Errorf("closeWaitTimeout = %v with writeTimeout=%v, want %v (review r3 P2: must = writeTimeout + 200ms)", got, tc.writeTimeout, tc.want)
			}
		})
	}
}

// TestSession_CloseWaitTimeout_GreaterThanCloseFrameWriteDeadline_ForProductionWriteTimeout：
// 关键不变量（review r3 P2）：production 配置（writeTimeout ≥ 1s）下，closeWaitTimeout
// **必须**严格大于 closeFrameWriteDeadline (500ms)，否则 r3 修复无效（wait 仍可能
// 提前于 writeLoop 真正退出，让 close frame 与 data frame 顺序错乱）。
//
// 注：production 默认 writeTimeout = 5s（gateway.go cfg.WriteTimeoutSec 兜底），
// startGatewayServer 测试配置 = 2s；两者都远大于 500ms，本不变量自然成立。
// 本测试是 documentation + 回归保护：未来如果有人把 closeWaitBufferDuration
// 改成 ≤ 0 或者把公式改成减号，本测试会立即捕获。
func TestSession_CloseWaitTimeout_GreaterThanCloseFrameWriteDeadline_ForProductionWriteTimeout(t *testing.T) {
	conn, cleanup := newPipeWebsocketConn(t)
	defer cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// 用 production 默认 writeTimeout=5s
	s := newSession("", 1001, 3001, conn, logger, 16384, 5*time.Second, nil)
	if s.closeWaitTimeout <= closeFrameWriteDeadline {
		t.Errorf("closeWaitTimeout %v <= closeFrameWriteDeadline %v with production writeTimeout=5s: r3 fix did not actually extend wait beyond original 500ms",
			s.closeWaitTimeout, closeFrameWriteDeadline)
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
