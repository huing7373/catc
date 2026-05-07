package ws

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// makeStubSessionForBroadcast 构造一个**裸** *Session，**不**绑定真实 *websocket.Conn /
// httptest.Server，仅初始化 broadcastToRoomFanout 路径会触达的字段：
//   - userID / roomID（让 Register 把它放进 sessionsByRoom 索引；BroadcastToRoom
//     的 logger 字段也会读 userID）
//   - sendChan（Session.Send 入队走 select sendChan，未初始化会 panic）
//   - logger（Send 不读 logger，但 Register 路径 With 叠加 sessionID 会读，必须非 nil）
//   - closed atomic.Bool 零值 false（Send 路径直接 Load 判定）
//
// **不**初始化的字段（broadcast fanout 路径用不到）：
//   - conn：BroadcastToRoom 不写 conn，writeLoop 才写 conn；本测试不启 writeLoop
//   - sendPriorityChan：BroadcastToRoom 调 Send 而非 SendPriority
//   - writeLoopDone / writeLoopStarted / closeWaitTimeout：仅 Close 路径用，本测试不调 Close
//   - ctx / cancelCtx：BroadcastToRoom 的 ctx 来自 caller，与 Session.ctx 无关
//
// **review 10-5 r2 P2 fix**：取代旧的 useGatewayDial 路径 —— 后者每个 Session 起一个
// httptest.NewServer（端口分配、TCP listen 真起 goroutine），N=100 时 Windows / 慢
// CI 容易因端口耗尽 / fd 上限 flaky；旧测试的 100ms 断言实际测的是 server 启动开销，
// 与 BroadcastToRoom 同步 fanout 本身性能无关。改用裸 *Session 后无端口分配，N=100
// 设置开销 < 10µs，断言可以收紧到 µs 级，但保留 100ms 上限以兼容 -race 慢机。
func makeStubSessionForBroadcast(userID, roomID uint64) *Session {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Session{
		userID:   userID,
		roomID:   roomID,
		sendChan: make(chan []byte, sendChanCapacity),
		logger:   logger,
	}
}

// TestBroadcastToRoom_R2_LargeN_SyncFanoutFastEnough_StubSessions：review 10-5 r2 P2
// 性能验证 + 不再依赖 httptest.Server 端口分配。
//
// **场景**：N=100 stub Session 在同一 roomID 下注册到 manager，调一次 BroadcastToRoom
// 应在 µs 级完成（同步 fanout = N × O(1) Send 入队，无任何 IO）。
//
// **断言**：
//   - sent == N（fanout 覆盖所有 session）
//   - elapsed < 100ms（CI / -race / Windows 宽松上限；实际预期 < 1ms）
//   - 每个 session.sendChan 内有且只有 1 条消息（fanout 正确投递）
//
// **为什么 100ms 阈值仍保留**：去掉了 httptest 启动开销后，N=100 fanout 实测应
// < 1ms（µs 级），但 -race / Windows / CI 慢机调度抖动可能让单次跑到几 ms。100ms
// 上限提供保护带，仍能捕获"fanout 不小心退化为 O(N²)"或"goroutine 误启" 之类的
// 真正回归。
func TestBroadcastToRoom_R2_LargeN_SyncFanoutFastEnough_StubSessions(t *testing.T) {
	mgr := NewSessionManager().(*sessionManager)
	const N = 100
	const roomID uint64 = 9201

	sessions := make([]*Session, 0, N)
	for i := 0; i < N; i++ {
		s := makeStubSessionForBroadcast(uint64(22000+i), roomID)
		if _, err := mgr.Register(context.Background(), s); err != nil {
			t.Fatalf("Register iter %d: %v", i, err)
		}
		sessions = append(sessions, s)
	}

	msg := []byte(`{"type":"big.sync.fanout","requestId":"","payload":{},"ts":0}`)
	start := time.Now()
	sent, err := BroadcastToRoom(context.Background(), mgr, roomID, msg)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("BroadcastToRoom: %v", err)
	}
	if sent != N {
		t.Errorf("sent = %d, want %d", sent, N)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("BroadcastToRoom over %d sessions took %v > 100ms — sync fanout may have regression", N, elapsed)
	}
	t.Logf("BroadcastToRoom over %d sessions took %v (sync fanout, stub sessions)", N, elapsed)

	// 每个 sendChan 内应只有 1 条消息（fanout 正确投递）
	for i, s := range sessions {
		got := len(s.sendChan)
		if got != 1 {
			t.Errorf("session[%d] sendChan len = %d, want 1", i, got)
		}
	}
}

// TestBroadcastToRoom_R2_ConcurrentBroadcasts_SameRoomGlobalOrder：review 10-5 r2 P2
// **关键并发回归测试** —— 跨 goroutine 同 room 并发 BroadcastToRoom 必须让所有
// session 看到全局一致的消息序。
//
// **场景**：
//  1. M 个 stub session 加入同一 room
//  2. G 个 goroutine 各自调 K 次 BroadcastToRoom 推不同 tag 的消息（goroutineG-iterK）
//  3. 等所有 broadcast goroutine 退出
//  4. 抽两个 session 比较它们 sendChan 内 drain 出来的消息序列 —— 必须**完全相等**
//     （所有 session 看到相同的全局序，即使该全局序是哪个 goroutine 抢到 mutex
//     的非确定调度）
//
// **不**断言全局序的具体内容（哪个 goroutine 先抢到 mutex 是 scheduler 自由），
// 只断言**所有** session 看到的 ordering **相同**（这是 review 10-5 r2 修复的核心
// 不变量）。
//
// **为什么这个测试在 r1 实装下会失败**：r1 的 broadcastToRoomFanout 没拿 per-room
// mutex，goroutine A 的 for-range 与 goroutine B 的 for-range 在 scheduler 间隔
// 执行 → session1.sendChan = [A1, B1, A2, B2, ...]，session2.sendChan = [B1, A1,
// B2, A2, ...]，两序列 hash 不等。r2 修复后两序列必然相等。
func TestBroadcastToRoom_R2_ConcurrentBroadcasts_SameRoomGlobalOrder(t *testing.T) {
	mgr := NewSessionManager().(*sessionManager)
	const M = 4 // 每个 room 4 个 session（足够检测 ordering 偏差）
	const G = 4 // 4 个并发 broadcaster goroutine
	const K = 5 // 每个 goroutine 推 5 次
	const roomID uint64 = 9301

	// sendChan 容量必须 ≥ G × K（4×5=20）防 ErrSessionSendBufferFull 干扰；
	// 既存 sendChanCapacity=32 ≥ 20，足够。
	if sendChanCapacity < G*K {
		t.Fatalf("sendChanCapacity=%d < G*K=%d — test premise broken", sendChanCapacity, G*K)
	}

	sessions := make([]*Session, 0, M)
	for i := 0; i < M; i++ {
		s := makeStubSessionForBroadcast(uint64(33000+i), roomID)
		if _, err := mgr.Register(context.Background(), s); err != nil {
			t.Fatalf("Register iter %d: %v", i, err)
		}
		sessions = append(sessions, s)
	}

	// 并发推 G × K 条消息。每条消息 payload 唯一（"g{G}-i{K}"）便于 ordering 比对。
	var wg sync.WaitGroup
	startBarrier := make(chan struct{})
	for g := 0; g < G; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			<-startBarrier
			for k := 0; k < K; k++ {
				msg := []byte(fmtTag(gid, k))
				if _, err := BroadcastToRoom(context.Background(), mgr, roomID, msg); err != nil {
					t.Errorf("g%d-k%d: BroadcastToRoom: %v", gid, k, err)
					return
				}
			}
		}(g)
	}
	close(startBarrier) // 同时放走所有 goroutine 最大化 scheduler 交错
	wg.Wait()

	// drain 每个 session.sendChan 拿到它实际收到的消息序列
	allSeqs := make([][]string, M)
	for i, s := range sessions {
		got := drainSessionSendChan(s)
		allSeqs[i] = got
		if len(got) != G*K {
			t.Errorf("session[%d] received %d msgs, want %d", i, len(got), G*K)
		}
	}

	// 关键断言：所有 session 看到**相同**的全局序（r2 修复的不变量）
	for i := 1; i < M; i++ {
		if !stringSliceEqual(allSeqs[0], allSeqs[i]) {
			t.Errorf("session[0] order differs from session[%d]:\n  s0=%v\n  s%d=%v",
				i, allSeqs[0], i, allSeqs[i])
		}
	}
}

// fmtTag / drainSessionSendChan / stringSliceEqual 是本文件内 helper（不导出，
// 只供 ordering 测试用）。

func fmtTag(g, k int) string {
	// 简单 itoa（避免引入 strconv.Itoa 让测试更紧凑；g/k 都 ≤ 999 足够）
	return "g" + itoaSmall(g) + "-i" + itoaSmall(k)
}

func itoaSmall(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [4]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

func drainSessionSendChan(s *Session) []string {
	out := make([]string, 0, len(s.sendChan))
	for {
		select {
		case msg := <-s.sendChan:
			out = append(out, string(msg))
		default:
			return out
		}
	}
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
