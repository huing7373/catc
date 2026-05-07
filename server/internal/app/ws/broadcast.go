package ws

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
)

// roomBroadcastMu 是包级 per-room 串行化锁（review 10-5 r2 P2 fix）。
//
// **为什么需要**：r1 改成同步 fanout 后，**单 goroutine** 连续调
// BroadcastToRoom 顺序保证（msg1 → msg2 在所有 session 的 sendChan 内
// 物理位置一致）。但**跨 goroutine** 并发 BroadcastToRoom(roomX, msgA) +
// BroadcastToRoom(roomX, msgB) 仍可乱序：A 的 for-range 和 B 的 for-range
// 在 Go scheduler 间隔执行 → session1 先收 msgA 再 msgB，session2 先收
// msgB 再 msgA → **同 room 内不同 client 看到不同事件序**，违反"room 内
// 全局有序事件流"的 V1 协议契约。
//
// **修法**：每 room 一把 mutex，BroadcastToRoom 进入时拿 per-room mutex，
// for-range 完释放。同 room 并发 broadcast 串行化（msgA 全部入队完再换
// msgB），跨 room broadcast 不互相阻塞（不同 mutex）。
//
// **实装选型**（参见 review 文档 a/b/c 三方案）：
//   - **(a) 包级 sync.Map[roomID]*sync.Mutex**（本实装）：简单清晰，跨实例
//     mutex 状态自然按 room shard，开销极小（每 room < 100B，房间数远低于 1M
//     可忽略）。缺点：mutex 不会被 GC 回收（room archive 后 mutex 仍在 map
//     内），但常驻内存占用极低不影响 prod
//   - (b) 把 perRoomBroadcastMu 字段挂在 SessionManager 上，跟 sessionsByRoom
//     一起 lifecycle 管理，archive 时回收。略复杂，生命周期更干净，但本 story
//     阶段无 archive 路径，复杂度收益不匹配
//   - (c) 直接用 SessionManager.mu 全局锁：所有 broadcast 串行 → 跨 room
//     吞吐大降。**不**采纳
//
// **不变量**：mutex 内**只**做 fanout（sessions 切片获取 + Send 入队循环），
// 不做任何长操作 / 阻塞 IO（Session.Send 是非阻塞 select-default 入队 O(1)），
// 所以"持锁全程"对吞吐影响仅 O(N) × O(1) = µs 级。
var roomBroadcastMu sync.Map // map[uint64]*sync.Mutex

// BroadcastFn 是 BroadcastToRoom 的接口形态（让 service 层注入 mock 用）。
//
// 签名与包级函数 BroadcastToRoom 完全一致；service 层（Story 11.8 / 14.4 /
// 17.5）的构造可接受 BroadcastFn 形参，单测注入 mock 让 service 层不依赖
// 真实 SessionManager / WS conn。
//
// 典型 wire（main.go bootstrap 阶段）：
//
//	memberService := room.NewMemberService(
//	    txMgr,
//	    func(ctx context.Context, roomID uint64, msg []byte) (int, error) {
//	        return ws.BroadcastToRoom(ctx, sessionMgr, roomID, msg)
//	    },
//	)
//
// 单测路径：
//
//	mockFn := func(ctx context.Context, roomID uint64, msg []byte) (int, error) {
//	    capturedCalls = append(capturedCalls, broadcastCall{roomID, msg})
//	    return 1, nil
//	}
//	memberService := room.NewMemberService(txMgr, mockFn)
//
// 设计为 type alias 而非 interface：
//   - 零分配（函数值传递不需要 interface vtable lookup）
//   - 易构造 mock（直接 closure 即可，不需要定义结构体实现 interface）
//   - service 层 wire 期通过 closure 捕获 SessionManager 引用，把 ws 包内部
//     实现细节 hidden 在 closure 内 → service 层 NewXxxService(... fn BroadcastFn)
//     不必反向 import ws.SessionManager
type BroadcastFn func(ctx context.Context, roomID uint64, msg []byte) (sent int, err error)

// BroadcastToRoom 把 msg 推送给 roomID 内所有 active Session（包级 primitive）。
//
// 流程：
//  1. mgr.ListSessionsByRoomID(ctx, roomID) → 拿当前 room 内所有 active Session
//     切片（read-lock copy + 锁外按 sessionID 字典序排序，10.3 r5 P2 实装）
//  2. 入口 `payload := bytes.Clone(msg)` —— **defensive copy**：让 caller 在
//     BroadcastToRoom return 后随意 mutate / reuse 原 msg buffer，不影响已入队的
//     payload（review 10-5 r1 P2 fix）
//  3. **同步**对每个 Session 调 s.Send(payload) —— 不再启 goroutine fanout，
//     直接 for-range 调用：Session.Send 是非阻塞 select-default 入队（O(1)），
//     同步调用不影响吞吐
//  4. Send 失败（ErrSessionClosed / ErrSessionSendBufferFull）→ log warn 但
//     **不**阻塞后续 Session（保持 fanout 一致性）
//
// **为什么改成同步**（review 10-5 r1 P1 fix）：
//   - 原 fire-and-forget goroutine 不保证 per-session 顺序：caller 连续调
//     BroadcastToRoom(msg1), BroadcastToRoom(msg2)，goroutine 调度可能让某个
//     session 先收 msg2 后收 msg1（room 广播应是 ordered stream）
//   - 同步调用让"BroadcastToRoom return 时 sendChan 都入队完"成为不变量 →
//     caller 依次调 BroadcastToRoom(msg1) → BroadcastToRoom(msg2) 在同 goroutine
//     里就保证 msg1 在所有 session sendChan 入队完之后 msg2 才开始
//   - Session.Send 内部是非阻塞 select-default 入队（O(1)），同步遍历无性能
//     回归（即使 1000 session 也 <1ms 量级）
//
// **跨 goroutine 序一致性**（review 10-5 r2 P2 fix）：
//   - r1 同步 fanout 仅保证**单 caller goroutine** 内顺序；**多 goroutine**
//     并发 broadcast 同 room 仍可乱序：A 的 for-range 与 B 的 for-range 在
//     scheduler 间隔执行 → session1 先收 msgA, session2 先收 msgB
//   - r2 用包级 sync.Map[roomID]*sync.Mutex 串行化同 room 并发 broadcast：
//     同 room 任意 goroutine 进入 fanout 必先抢 mutex → 一次 fanout 全部入队
//     完再放下次进入 → 所有 session 看到全局一致的 msgA/msgB 排列序
//   - 跨 room broadcast 走不同 mutex，不互相阻塞，无吞吐回归
//
// 参数：
//   - ctx: ctx-aware 上游传递；BroadcastToRoom 内仅传给 mgr.ListSessionsByRoomID
//     调用 + log 字段
//   - mgr: SessionManager 单例（main.go bootstrap 阶段已构造）
//   - roomID: 目标 room 的 ID
//   - msg: **已序列化的字节流**（serverEnvelope 已 json.Marshal 完成）；
//     primitive 层**不**做 marshal，让 caller 按 V1 §12.3 字段表序列化好后传入。
//     **入口会 bytes.Clone**：caller 在 return 后释放 / mutate 原 buffer 完全
//     安全，不会与已入队的 payload 共享底层数组
//
// 返回：
//   - sent: 发起 Send 的 Session 数量（== len(slice)，不回扫确认）
//   - err: 永远 nil（设计成 future-proof error return；当前实装永不返 error，
//     但接口签名保留 error，让未来 Pub/Sub 跨实例 / Redis 节点不可达 / etc.
//     场景下能扩展）
//
// **关键约束**：
//   - 切片获取走 mgr.ListSessionsByRoomID（read-lock copy）—— **禁止**直接
//     访问 sessionManager 内部 sessionsByRoom map（manager 不导出该字段）
//   - 0 个 active Session（room 不存在 / 无人在线）→ 返 (0, nil) 而非 error
//     （合法场景，与 epics.md AC 行 1746 钦定一致）
//   - Send 失败仅 log warn + slog.String("sessionId", sid) + slog.Uint64("userId", uid)
//     + slog.Any("error", err)；**不** return error 给主函数（fanout 一致性）
//   - **入口 bytes.Clone(msg)**：caller msg buffer 所有权完全隔离（review 10-5
//     r1 P2 fix）。clone 一次成本 O(len(msg))，对 sub-KB 级 msg 可忽略
//   - **不**调 Session.Close / CloseWithCode：BroadcastToRoom 不主动 close 任何
//     Session（Send 失败 ≠ Session 已死；可能是临时 sendChan 满，下次 broadcast
//     会重试）
//   - **不**调 mgr.Unregister：与 close 同理，发起广播的代码路径不应触发
//     lifecycle 变更
//
// **数据源**（Story 10.5 §"实装关键决策" §1）：MVP 单实例阶段走 SessionManager
// （等价于 Redis presence 数据源）；多实例阶段（节点 13+）才走 Pub/Sub 跨实例
// 路径，本函数当前实装**不**消费 Redis。
func BroadcastToRoom(ctx context.Context, mgr SessionManager, roomID uint64, msg []byte) (sent int, err error) {
	return broadcastToRoomFanout(ctx, mgr, roomID, msg)
}

// broadcastToRoomFanout 是 BroadcastToRoom 的核心实装（unexported helper）。
//
// **同步**遍历 sessions 调 Session.Send；不启 goroutine。
//
// 顺序保证：
//   - within 单次 BroadcastToRoom：按 ListSessionsByRoomID 返回顺序（已在锁外
//     按 sessionID 字典序排序，见 10.3 r5 P2）依次入队 → 每个 session 看到
//     的 within-call 顺序确定
//   - across 多次 BroadcastToRoom（同一 caller goroutine，review 10-5 r1 P1 fix）：
//     caller 依次调用 BroadcastToRoom(msg1), BroadcastToRoom(msg2) → 由于本函数
//     同步 return，msg1 已在所有 session sendChan 入队完后 msg2 才开始入队 →
//     所有 session 的 sendChan 内 msg1 物理位置在 msg2 之前 → writeLoop 单一
//     消费方按 chan FIFO 顺序写到 conn → client 端观察到 msg1 先于 msg2
//   - **across goroutines on same room**（review 10-5 r2 P2 fix）：包级
//     roomBroadcastMu 串行化同 room 并发 broadcast。两个 goroutine 同时调
//     BroadcastToRoom(roomX, msgA) + BroadcastToRoom(roomX, msgB) → 一个进入
//     mutex 段先 fanout 完所有 session，另一个再进入。所有 session 看到全局
//     一致的事件序（要么 msgA → msgB，要么 msgB → msgA，但**所有** session
//     看到**相同** order，不会出现 session1 看 A→B 同时 session2 看 B→A）
//
// msg 所有权（review 10-5 r1 P2 fix）：
//   - 入口 bytes.Clone(msg) → payload 底层数组与 caller msg 完全隔离
//   - caller return 后释放 / reuse / mutate 原 msg buffer 完全安全
//   - payload 的所有权转给 Session.sendChan（chan []byte 持有 slice header）→
//     writeLoop 消费完写到 conn 后由 GC 回收
//
// **关键不变量**（与 lessons 对齐）：
//   - 同步 Send 失败 log warn 不阻塞后续 session（lesson `ws-session-send-close-race-and-shutdown-hooks.md`）
//   - 不主动 close / Unregister Session（fanout 是只读路径；lifecycle 变更交给
//     readLoop / writeLoop / scanner 的对应路径）
//   - **不**在循环内监听 ctx.Done（同步遍历 + Session.Send 是非阻塞 select-default
//     入队 → 总耗时 = O(N) × O(1) Send 入队，不会 hang，无需 ctx-cancel hook）
//   - 持 per-room mutex 全程是必要的（fanout 内每个 Send 都是 O(1) 入队，全程
//     仅 µs 级），跨 room broadcast 走不同 mutex 不互相阻塞
func broadcastToRoomFanout(ctx context.Context, mgr SessionManager, roomID uint64, msg []byte) (int, error) {
	// review 10-5 r2 P2 fix：per-room mutex 串行化同 room 并发 broadcast。
	// LoadOrStore 双重 lookup 保证多 goroutine 同时首次进入 roomX 时只创建
	// 一个 mutex（sync.Map 内部 atomic）。mutex 一旦创建不删除（room archive
	// 后 mutex 仍在 map 中，但每 room < 100B，可忽略）。
	muVal, _ := roomBroadcastMu.LoadOrStore(roomID, &sync.Mutex{})
	mu := muVal.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	// **关键**：sessions 列表必须**在持 mutex 后**取，而不是 mutex 前 ——
	// 否则两个 goroutine 都已拿到自己的 sessions 切片再排队进 mutex，意味着
	// 中间 mgr 的 sessionsByRoom 变化看不见，但更重要的是：跨 broadcast 的
	// **顺序**仍取决于哪个 goroutine **先入队 Send**，与本 mutex 设计目标
	// （同 room 同 goroutine 内 fanout atomic）无关。但取 sessions 在 mutex
	// 内可以保证"读 manager 索引 + fanout"是原子段，如果某 session 在 A
	// goroutine fanout 中被 unregister，B goroutine 取 sessions 时已观察到
	// 一致快照。
	sessions := mgr.ListSessionsByRoomID(ctx, roomID)
	if len(sessions) == 0 {
		return 0, nil
	}

	logger := slog.Default().With(slog.String("component", "ws-broadcast"))
	logger.Info("ws broadcast to room",
		slog.Uint64("roomId", roomID),
		slog.Int("targetSessions", len(sessions)),
		slog.Int("msgBytes", len(msg)),
	)

	// review 10-5 r1 P2 fix：defensive copy msg → payload 与 caller buffer 隔离。
	// caller return 后释放 / mutate 原 msg 完全安全；payload 由 Session.sendChan
	// 持有引用，writeLoop 消费完写到 conn 后由 GC 回收。
	//
	// bytes.Clone(nil) 返 nil（Go 1.20+ stdlib 行为）→ 与既有 nil-msg 测试
	// （TestBroadcastToRoom_NilMessage_HandledGracefully）兼容。
	payload := bytes.Clone(msg)

	// review 10-5 r1 P1 fix：同步遍历调 Session.Send（不再启 goroutine fanout）。
	// Session.Send 是非阻塞 select-default 入队（O(1)），同步调用让 caller 在
	// BroadcastToRoom return 时知道所有 session sendChan 都已入队完 → 跨调用
	// 顺序保证（msg1 入队完 → BroadcastToRoom return → caller 调 msg2 入队 →
	// 所有 session 的 sendChan 内 msg1 物理位置先于 msg2）。
	for _, s := range sessions {
		if sendErr := s.Send(payload); sendErr != nil {
			logger.Warn("ws broadcast Send failed",
				slog.String("sessionId", s.SessionID()),
				slog.Uint64("userId", s.UserID()),
				slog.Uint64("roomId", roomID),
				slog.Any("error", sendErr),
			)
		}
	}

	return len(sessions), nil
}
