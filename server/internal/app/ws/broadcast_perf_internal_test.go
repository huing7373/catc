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

// TestBroadcastToRoom_R3_PerRoomMutex_LoadFastPath_NoExtraAllocs：review 10-5 r3 P2
// 验证 Load fast path —— 同一 room 多次 BroadcastToRoom 不会让 roomBroadcastMu
// 中 mutex 数量增长（即每次都不会 alloc 新 *sync.Mutex 留给 GC）。
//
// **场景**：
//  1. 同 room 调 BroadcastToRoom 100 次
//  2. 数 roomBroadcastMu 内 size（Range counter）—— 必须为 1
//
// **为什么这个测试在 r3 修复前会"误绿"**：旧实装 `LoadOrStore(roomID, &sync.Mutex{})`
// 即使 hit，新 alloc 出来的 *sync.Mutex 会立即被丢弃 GC，不会**留**在 map 内 →
// sync.Map size 仍然是 1。所以单纯数 size 不能直接验证 alloc。**真正断言点**
// 用 testing.AllocsPerRun：r3 修复前每次 alloc 一个 sync.Mutex（>= 1 alloc/op），
// r3 修复后 hot path 零 alloc。
//
// **本测试两段断言**：
//   - 段1：调 100 次同 room，roomBroadcastMu size 应为 1（基线，r2 起即应为 1）
//   - 段2：用 testing.AllocsPerRun 在已暖好（room 已存在 mutex）的 hot path 上
//     验证 BroadcastToRoom 不再每次 alloc *sync.Mutex（r3 修复点）
func TestBroadcastToRoom_R3_PerRoomMutex_LoadFastPath_NoExtraAllocs(t *testing.T) {
	mgr := NewSessionManager().(*sessionManager)
	const roomID uint64 = 9401

	// 单 stub session（让 fanout 走真实路径，但 Send 是 O(1) chan 入队）
	s := makeStubSessionForBroadcast(44000, roomID)
	if _, err := mgr.Register(context.Background(), s); err != nil {
		t.Fatalf("Register: %v", err)
	}

	// 关键：在测试开始前清空 roomBroadcastMu 与本 roomID 相关的 entry，
	// 防止跨 test 残留干扰（其他 test 用别的 roomID，但 sync.Map 共享）。
	// 不能直接 delete 全部（会破坏其他并发 test），只 delete 本 roomID。
	roomBroadcastMu.Delete(roomID)

	msg := []byte(`{"type":"r3.fastpath","payload":{}}`)

	// 段1：调 100 次，roomBroadcastMu 内本 roomID 的 entry 始终唯一。
	const N = 100
	for i := 0; i < N; i++ {
		if _, err := BroadcastToRoom(context.Background(), mgr, roomID, msg); err != nil {
			t.Fatalf("BroadcastToRoom iter %d: %v", i, err)
		}
		// drain sendChan 防止后续 Send 撞 ErrSessionSendBufferFull
		<-s.sendChan
	}

	count := 0
	roomBroadcastMu.Range(func(k, _ any) bool {
		if k.(uint64) == roomID {
			count++
		}
		return true
	})
	if count != 1 {
		t.Errorf("roomBroadcastMu entries for roomID=%d = %d, want 1 (mutex must be reused, not duplicated)", roomID, count)
	}

	// 段2：room 已暖（Load hit path），验证 BroadcastToRoom 不再每次 alloc *sync.Mutex。
	//
	// **预期**：r3 修复后 hot path 上 BroadcastToRoom 的每次调用 alloc 数应低于
	// r3 修复前（修复前每次至少 1 alloc 给丢弃的 sync.Mutex + 1 alloc 给 interface
	// 装箱传给 LoadOrStore；修复后这两个 alloc 消失）。
	//
	// **不**断言绝对 0 alloc —— BroadcastToRoom 内部还有 `slog.Default().With(...)`
	// （logger 构造）、bytes.Clone（payload 复制）、slog 字段装箱、interface 转换
	// 等若干其他必要 alloc。本测试只断言"显著少于 baseline"——具体阈值用 hot-path
	// 实测 + 余量。
	//
	// 实测（go1.23 / Windows / 单 session）：
	//   - r3 修复**前** baseline：~15 allocs/op（包含 sync.Mutex + interface 装箱）
	//   - r3 修复**后**：~13 allocs/op（少 2 个：sync.Mutex 实例 + interface{}-box）
	//
	// 阈值 < 15 严格捕获 r3 回归（修复前的 15 会失败），同时 < 14 防御别的小幅
	// 增长。设 14：精确卡在"修复后 ≤ 13 通过 / 回到修复前 ≥ 15 失败"中间。
	allocs := testing.AllocsPerRun(100, func() {
		// 先 drain，避免 sendChan 满
		select {
		case <-s.sendChan:
		default:
		}
		_, _ = BroadcastToRoom(context.Background(), mgr, roomID, msg)
	})
	t.Logf("BroadcastToRoom hot-path allocs/op: %.2f", allocs)
	if allocs >= 14 {
		t.Errorf("BroadcastToRoom hot path allocs/op = %.2f, want < 14 (r3 fix: Load fast path should remove per-call sync.Mutex+interface alloc; baseline pre-fix ≈ 15)", allocs)
	}
}
