package ws

import (
	"bytes"
	"context"
	"log/slog"
)

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
// 顺序保证（review 10-5 r1 P1 fix）：
//   - within 单次 BroadcastToRoom：按 ListSessionsByRoomID 返回顺序（已在锁外
//     按 sessionID 字典序排序，见 10.3 r5 P2）依次入队 → 每个 session 看到
//     的 within-call 顺序确定
//   - across 多次 BroadcastToRoom（同一 caller goroutine）：caller 依次调用
//     BroadcastToRoom(msg1), BroadcastToRoom(msg2) → 由于本函数同步 return，
//     msg1 已在所有 session sendChan 入队完后 msg2 才开始入队 → 所有 session
//     的 sendChan 内 msg1 物理位置在 msg2 之前 → writeLoop 单一消费方按 chan
//     FIFO 顺序写到 conn → client 端观察到 msg1 先于 msg2
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
func broadcastToRoomFanout(ctx context.Context, mgr SessionManager, roomID uint64, msg []byte) (int, error) {
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
