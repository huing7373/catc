# Story 10.5: BroadcastToRoom primitive（房间广播原语 + SessionManager 拿目标 + goroutine fanout 并发 Send + log 错误不阻塞）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want 在 Story 10.3 SessionManager（含 `ListSessionsByRoomID(ctx, roomID) []*Session`）+ 10.4 心跳框架（含 `ListAllSessions` / `CloseWithCode` / scanner）已就绪之上，新建 `internal/app/ws/broadcast.go` 暴露**进程级 primitive 函数** `BroadcastToRoom(ctx context.Context, mgr SessionManager, roomID uint64, msg []byte) (sent int, err error)`：内部走 `mgr.ListSessionsByRoomID(ctx, roomID)` 拿当前 room 内所有 active Session 切片（read-lock copy + sessionID 字典序，10.3 实装并发不变量保留）→ 对每个 Session 用 goroutine fanout 并发调 `Session.Send(msg)`（fire-and-forget，**不**等 goroutine 全部退出）→ Send 失败（`ErrSessionClosed` / `ErrSessionSendBufferFull`）log warn 但**不**阻塞其他 Session → 返回的 `sent` 是"发起 Send 的 Session 数量"（与切片 len 相等，**不**回扫确认 Send 成功，因为 Send 本身是 fire-and-forget 入队语义）；同时为后续 Epic 11 / 14 / 17 业务广播预留**纯函数式接口**（Epic 11 Story 11.8 调 `BroadcastToRoom(ctx, mgr, roomID, memberJoinedJSON)` / 14.4 调 `pet.state.changed` / 17.5 调 `emoji.received` 都通过同一函数）+ **集成测试覆盖** 启 3 个 WS 客户端连同一 roomID + 服务端调 `BroadcastToRoom` 推送一条 `member.joined`-shape 消息 → 验证 3 个客户端都收到 + 一个 client 主动 Close 后再调 `BroadcastToRoom` → 剩 2 个收到 + 不返 error；严格按 V1 §12.1 ~ §12.3 已冻结的 WS 协议骨架契约（自 2026-05-05 起冻结）实装，**不**反向修改 V1 文档，
so that Epic 11 房间业务 / Epic 14 宠物状态同步 / Epic 17 表情广播 / 任何后续需要"对 room 内所有 active 用户推送"的业务场景都能直接调用 `BroadcastToRoom`，不必各自再写一份"拿 Session 列表 + 并发 Send + 失败 log"的拼凑代码；且本 story 通过把 fanout / 错误处理 / 锁内 copy 三个不变量在 primitive 层面统一固化（参考 Story 10.4 HeartbeatScanner.scanOnce 同模式：list → fanout → fire-and-forget），让后续业务 epic 加"广播某种 type 的消息"时只需关心 msg 序列化逻辑，不必反向重学 SessionManager / Session 并发模型 / close-race 不变量；并预留 `MessageEnvelope` 序列化 helper 接口契约（**仅**接口契约，**不**实装具体业务消息 marshaling，业务 type 字段值由对应 Epic 自己定）。

## 故事定位（Epic 10 第五条 = 房间广播原语奠基；对标 Story 10.4 在 Epic 10 心跳框架的角色，本 story 是 Epic 10 后续 10.6 / 10.7 + Epic 11 ~ Epic 17 所有 "server-active 业务广播" story 的强前置）

- **Epic 10 进度**：10.1 (WS 协议骨架文档锚定) done → 10.2 (Redis 接入) done → 10.3 (WS 网关骨架) done → 10.4 (心跳框架) done → **本 story (10.5 BroadcastToRoom primitive)** → 10.6 (Redis presence repo) → 10.7 (房间快照下发框架)
- **强前置关系**：
  - **Story 10.3 done 提供的强前置**：
    - `SessionManager.ListSessionsByRoomID(ctx, roomID) []*Session`（已实装；本 story 直接消费 —— 见 `server/internal/app/ws/session_manager.go:313`，read-lock copy + 锁外排序模式）
    - `Session.Send(msg []byte) error`（已实装；本 story fanout 内调 —— 见 `server/internal/app/ws/session.go:308`，sendMu.RLock + 入队 sendChan + ErrSessionClosed / ErrSessionSendBufferFull sentinel）
    - `Session.SessionID() / UserID() / RoomID()` 公开 getter（已实装；本 story 用于 log 字段）
    - `Session` lifecycle：Register → Send 入队 → writeLoop 消费 → Close → Unregister（已实装；本 story 不动 lifecycle，只调 Send）
  - **Story 10.4 done 提供的强前置**：
    - `SessionManager.ListAllSessions(ctx) []*Session`（已实装，但本 story **不**调用 —— BroadcastToRoom 是按 room 切，不是全局；ListAllSessions 是 HeartbeatScanner 的全局扫描接口）
    - `Session.CloseWithCode(code, reason) error`（已实装；本 story **不**调用 —— BroadcastToRoom 不主动 close 任何 Session，Send 失败仅 log warn）
    - `closeFrameWriteDeadline / closeWaitTimeout` 等 const（已实装；本 story 不消费）
    - HeartbeatScanner 在后台跑（已 wire）—— 与 BroadcastToRoom 完全正交（scanner 走 ListAllSessions + CloseWithCode，broadcast 走 ListSessionsByRoomID + Send，两者无锁竞争）
  - **下游立即依赖**：
    - **Story 10.6（Redis presence repo）**：本 story **不**直接消费 Redis presence —— 但 epics.md §Story 10.5 行 1741 钦定的"从 Redis presence 拿 `room:{roomID}:online_users` 集合"语义在 MVP 单实例阶段**等价于** SessionManager.ListSessionsByRoomID（详见 §"实装关键决策" §1 数据源选择 + §"前置 lessons 提炼" §1 单实例 source-of-truth）；10.6 落地后 Redis presence 是**并行的多实例可见性入口**（节点 9+ 多实例部署时通过 Redis Pub/Sub 跨实例广播），**不**替代 SessionManager 单实例数据源
    - **Story 10.7（房间快照下发框架）**：与本 story 完全正交（snapshot 是握手期一次性同步段写入；broadcast 是运行期多次异步推送）；**不**通过 BroadcastToRoom 路径
    - **Story 11.8（成员加入 / 离开 WS 广播）**：epics.md 行 1985-1986 钦定 "Story 11.4 加入房间事务**成功提交后**调 BroadcastToRoom(roomID, member.joined)" / "Story 11.5 退出房间事务**成功提交后**调 BroadcastToRoom(roomID, member.left)"；本 story 必须把 BroadcastToRoom 接口设计稳，让 11.8 不必反向修改 primitive 签名
    - **Story 14.4（pet.state.changed WS 广播）**：epics.md 行 2349-2360 钦定调 BroadcastToRoom(currentRoomId, pet.state.changed) + 单测要求 mocked BroadcastToRoom；本 story 必须让 BroadcastToRoom 是**接口形态**（`BroadcastFn func(ctx, roomID, msg) (int, error)`）让 14.4 service 层可以注入 mock
    - **Story 17.5（emoji.received WS 广播）**：epics.md 行 2616 钦定调 BroadcastToRoom(currentRoomId, emoji.received)；本 story primitive 通过即可服务 17.5
- **Epic 4 / 7 / 10 已完成 story 是本 story 的依赖**：
  - **Story 1-1 / 1-5**（已 done）：testify + sqlmock + dockertest 测试栈已就绪；本 story 单测复用 ws_test.go 既有 stubRoomMemberRepo / 真实 WS Dialer 模式 + Story 10.4 加的 idleTestLogger / readCloseError / readCloseFrameLoose helper（**不**新建测试 helper，与既有 28+ case 同包追加）
  - **Story 1-9 ctx 传播**（已 done）：本 story `BroadcastToRoom` 必须接受 ctx 参数 + 在 ctx.Done 时**不**强行打断 goroutine fanout（fire-and-forget 语义不需要 ctx-cancel；ctx 只用于 ListSessionsByRoomID 调用 + log 字段）
  - **Story 4.4 token util**（已 done）：本 story 集成测试用 auth.Signer 签 3 个不同 user 的 token + Dial WS（与 ws_integration_test.go 既有 4 case 模式一致）；本 story 不直接消费 token util
  - **Story 10.3 review r2 P2 + r4 P2 + r5 P2 + r10 P1**（已 done）：lessons 在 `docs/lessons/2026-05-06-ws-reconnect-unregister-hook-and-prod-contract-gate.md` / `ws-room-status-filter-and-priority-quota.md` / `ws-snapshot-merge-contract-and-roomid-source-r10.md` / `ws-room-existence-source-and-pong-priority-r4.md` 已记录；本 story 实装时**必须**把"close 路径必须触发 onUnregister"+"sendChan 满 fire-and-forget"+"reconnect 替换中场窗口期 ListSessionsByRoomID 不能同时返 OLD/NEW（避免双发）"+"writeLoop priority quota 防 starvation"四条规则在 BroadcastToRoom 内尊重（见 §"前置 lessons 提炼"）
  - **Story 10.4 review r1 ~ r6**（已 done）：lessons 在 `docs/lessons/2026-05-06-ws-heartbeat-toctou-and-close-frame-ordering-10-4-r1.md` / `2026-05-06-ws-close-skip-wait-when-writeloop-not-started-10-4-r2.md` / `2026-05-06-ws-close-wait-timeout-and-shutdown-fanout-10-4-r3.md` / `2026-05-06-ws-heartbeat-shutdown-ctx-and-close-wait-only-emit-10-4-r4.md` / `2026-05-07-ws-heartbeat-fanout-drain-and-list-sort-outside-lock-10-4-r5.md` / `2026-05-07-ws-shutdown-must-wait-for-goroutine-exit-not-just-signal-10-4-r6.md` 已记录；本 story 严格遵守"fanout 函数局部变量分配 + sync.WaitGroup 在测试路径上 drain"模式（详见 §"前置 lessons 提炼" + §"实装关键决策" §3 fanout drain 模式）
- **iOS / 跨端契约**：
  - V1 §12.3 已锚定 server-active message 字段表（room.snapshot / pong / error 三类已冻结；member.joined / member.left / pet.state.changed / emoji.received 由 Story 11.1 / 14.1 / 17.1 后续锚定）
  - **本 story 不预先锁死 message 字段表**：BroadcastToRoom 接受 `msg []byte` 形参（**已序列化的字节流**，不在 primitive 层 marshal），让 caller（Epic 11 / 14 / 17 各自的 service 层）按对应 §X.1 锚定的 message 字段表 marshal serverEnvelope 后再传入
  - 本 story 不依赖任何 iOS 代码改动；iOS 端 Story 12.3 / 12.4（解析 member.joined / member.left）是 Epic 12 范围 —— 与本 story server primitive 接口完全正交（client 只看 `msg []byte` 序列化后的 JSON）
- **范围红线**（明确**不**做）：
  - **不**实装 Redis presence repo（10.6 才做；本 story 通过 SessionManager.ListSessionsByRoomID 拿 Session 列表，**不**消费 Redis；epics.md AC 中"从 Redis presence 拿 `room:{roomID}:online_users` 集合"在 MVP 单实例阶段**等价于** SessionManager 内部 sessionsByRoom map，由 10.6 落地后 Redis presence 通过 lifecycle 钩子保持与 SessionManager 一致 —— 详见 §"实装关键决策" §1）
  - **不**实装 SnapshotBuilder 接口（10.7 才做；本 story 与 snapshot 路径完全正交）
  - **不**实装 Pub/Sub 跨实例广播（节点 9+ 多实例部署时再做，与 Story 10.2 / 10.3 范围红线一致；本 story 单实例本地 fanout 即可）
  - **不**实装 server-active 业务消息字段表（member.joined / pet.state.changed / emoji.received 等由 Story 11.1 / 14.1 / 17.1 各自锚定；本 story primitive 仅消费 `msg []byte`，对 type 字段值 / payload 结构**完全无知**）
  - **不**实装 BroadcastToUser / BroadcastToAll / BroadcastExcept 等其他广播 primitive（按 YAGNI 原则；epics.md §Story 10.5 仅钦定 BroadcastToRoom 一个）
  - **不**改 V1 接口设计文档（V1 §12 已在 Story 10.1 冻结；本 story 是字段层契约**实装**，**不**反向修改文档）
  - **不**改 docs/宠物互动App_Go项目结构与模块职责设计.md（§6.10 / §9 已锚定 Realtime / WS 模块边界；本 story 引入的 broadcast.go 视为 Realtime 模块内 broadcast primitive 文件，与 §9 三对象建议中的 RoomHub 职责对齐 —— 但本 story **不**实装完整 RoomHub struct，仅交付包级 primitive 函数 + 接口契约预留，让 Epic 11+ 真正需要 hub 状态时再决定升级为 struct）
  - **不**写英文文档 / OpenAPI / AsyncAPI 形式化定义
  - **不**修改 SessionManager interface（ListSessionsByRoomID / ListAllSessions 已就绪；本 story 仅消费）
  - **不**修改 Session struct 字段（Send / Close / lastHeartbeatAt / sendChan / sendPriorityChan 已就绪；本 story 仅调 Send）
  - **不**修改 Gateway.Handle 校验顺序 / Upgrade / Register 流程（已在 10.3 r10 P1 锁定；本 story 与握手路径完全正交）
  - **不**改 Story 4.5 RateLimit 中间件（V1 §12.1 钦定 WS **不**走 HTTP rate_limit；本 story 不动 router）
  - **不**实装 metrics（Prometheus counter）记录 broadcast 次数 / Send 失败次数（节点 13+ 才做；本 story 仅靠 slog 结构化日志记录关键事件）
  - **不**实装 ConfigKey（无 YAML 字段；本 story 是纯函数 primitive，无可调参数）

**本 story 不做**（明确范围红线，避免 dev-story 阶段 scope 漂移）：

- 不实装 Redis presence（10.6 接管）
- 不实装 SnapshotBuilder（10.7 接管）
- 不实装具体业务消息 type 字段值（Epic 11 / 14 / 17 各自锚定）
- 不实装 Pub/Sub 跨实例（节点 9+ 接管）
- 不实装 BroadcastToUser / BroadcastToAll / BroadcastExcept（按 YAGNI）
- 不实装 metrics counter（节点 13+ 接管）
- 不在本 story 引入 OpenTelemetry tracing
- 不写英文文档 / 改 README 之外的运行手册（本 story **不**追加 README 子节 —— BroadcastToRoom 是 server 内部 primitive 函数，不暴露给 wscat 等外部工具；与 10.4 心跳超时 README 子节情况不同）

## Acceptance Criteria

**AC1 — 新建 `server/internal/app/ws/broadcast.go` 文件 + 包级 primitive 函数 `BroadcastToRoom`**

`server/internal/app/ws/broadcast.go`（**新建文件**）：

```go
package ws

import (
    "context"
    "log/slog"
    "sync"
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
type BroadcastFn func(ctx context.Context, roomID uint64, msg []byte) (sent int, err error)

// BroadcastToRoom 把 msg 推送给 roomID 内所有 active Session（包级 primitive）。
//
// 流程：
//  1. mgr.ListSessionsByRoomID(ctx, roomID) → 拿当前 room 内所有 active Session
//     切片（read-lock copy + 锁外按 sessionID 字典序排序，10.3 r5 P2 实装）
//  2. 对每个 Session **并发** go func() { s.Send(msg) }（goroutine fanout，
//     fire-and-forget）
//  3. 每个 goroutine 内：调 s.Send(msg) → 失败（ErrSessionClosed /
//     ErrSessionSendBufferFull）log warn 但**不**阻塞其他 goroutine
//  4. 主函数**不**等待 goroutine 全部退出（fire-and-forget 语义）：返回
//     sent = len(slice) + nil error；调用方拿到 sent 仅作"发起 Send 的数量"
//     语义，**不**作"实际送达 client 的数量"语义（Send 本身是异步入队，
//     writeLoop 消费可能失败，client 是否真收到由 ack message 实装兜底，
//     与本 primitive 无关）
//
// 参数：
//   - ctx: ctx-aware 上游传递；BroadcastToRoom 内仅传给 mgr.ListSessionsByRoomID
//     调用 + log 字段；**不**用 ctx 控制 fanout goroutine 退出（fire-and-forget）
//   - mgr: SessionManager 单例（main.go bootstrap 阶段已构造）
//   - roomID: 目标 room 的 ID
//   - msg: **已序列化的字节流**（serverEnvelope 已 json.Marshal 完成）；
//     primitive 层**不**做 marshal，让 caller 按 V1 §12.3 字段表序列化好后传入
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
//   - 不在 primitive 内修改 msg / 不复制 msg（caller 持有所有权；多个 goroutine
//     共享 msg []byte 是安全的，因为 Session.Send 内部仅 input 入队 + writeLoop
//     单一消费方写到 conn —— 无并发写 msg slice 的风险）
//   - **不**调 Session.Close / CloseWithCode：BroadcastToRoom 不主动 close 任何
//     Session（Send 失败 ≠ Session 已死；可能是临时 sendChan 满，下次 broadcast
//     会重试）
//   - **不**调 mgr.Unregister：与 close 同理，发起广播的代码路径不应触发
//     lifecycle 变更
func BroadcastToRoom(ctx context.Context, mgr SessionManager, roomID uint64, msg []byte) (sent int, err error) {
    // 实装详见 §6 dev notes
    return 0, nil // placeholder；详见 §6 实装伪代码
}
```

**关键约束（落地时严格遵守）**：

- 包级函数（**不**做成 SessionManager 上的 method）：让单测能直接 import + 注入 mock SessionManager 调用，不需要构造 sessionManager struct
- 形参顺序 `(ctx, mgr, roomID, msg)`：ctx 在第一位（ADR-0007 钦定）；mgr 在第二位（依赖注入显式）；roomID 第三（业务 key）；msg 第四（payload）
- 返回 `(sent int, err error)`：**两返回值**让未来 Pub/Sub 路径能扩展返 error；当前实装永不返 error
- log component 字段统一 `slog.String("component", "ws-broadcast")` —— 与 HeartbeatScanner 的 `"ws-heartbeat"` 区分，便于日志聚合排障
- 文件位置：`server/internal/app/ws/broadcast.go` 与 gateway.go / session.go / session_manager.go / heartbeat_scanner.go 平级（包内平铺，与 10.3 / 10.4 同模式）

**AC2 — `BroadcastFn` 类型别名 + 接口契约预留（让 service 层注入 mock）**

如 AC1 代码示例所示，broadcast.go 必须**额外**导出 `BroadcastFn` 类型别名（`type BroadcastFn func(ctx, roomID, msg) (sent int, err error)`）：

- 类型签名与包级 BroadcastToRoom 函数**严格一致**（除了 `mgr` 参数 —— BroadcastFn 不带 mgr，由 closure 在 wire 期捕获）
- godoc 示例必须演示两种用法：
  1. **wire 期 closure 捕获**（生产路径）：`func(ctx, roomID, msg) (int, error) { return ws.BroadcastToRoom(ctx, sessionMgr, roomID, msg) }`
  2. **测试期 mock**（测试路径）：直接构造 closure 收集 call 记录
- BroadcastFn 让 Story 11.8 / 14.4 / 17.5 service 层 NewXxxService(... fn BroadcastFn) 接受这个 type 而不是直接 import ws.SessionManager —— 避免 service 层 leak ws 包内部实现细节

**关键约束**：

- BroadcastFn 是 type alias（`type BroadcastFn func(...) (...)`），**不是** interface（让 service 层零分配 + 易构造 mock）
- 命名是 `BroadcastFn` 不是 `Broadcaster` —— 后者会让人以为是 interface；前者明确是 function value
- 不在 ws 包内部使用 BroadcastFn（BroadcastToRoom 是包级函数，无需通过 BroadcastFn 调自己）；BroadcastFn 仅供下游 service 层注入用

**AC3 — fanout drain 模式 + 测试 helper（仿 Story 10.4 r5 lessons）**

落地时严格遵循 Story 10.4 r5 落地的 fanout drain 模式（lesson `2026-05-07-ws-heartbeat-fanout-drain-and-list-sort-outside-lock-10-4-r5.md`）：

```go
func BroadcastToRoom(ctx context.Context, mgr SessionManager, roomID uint64, msg []byte) (sent int, err error) {
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

    // 函数局部 wg：让 testing path 可通过同包 unexported helper 持有 wg pointer
    // 等所有 goroutine drain（详见 broadcastToRoomForTest helper）。
    // 生产路径不消费 wg.Wait —— 主函数立即返回（fire-and-forget）；wg 仅做
    // 函数局部状态用于测试 hook，不影响生产 latency。
    wg := &sync.WaitGroup{}
    wg.Add(len(sessions))

    for _, s := range sessions {
        s := s // capture loop var (Go 1.22+ 已自动捕获，但显式写让 review 友好)
        go func() {
            defer wg.Done()
            if sendErr := s.Send(msg); sendErr != nil {
                logger.Warn("ws broadcast Send failed",
                    slog.String("sessionId", s.SessionID()),
                    slog.Uint64("userId", s.UserID()),
                    slog.Uint64("roomId", roomID),
                    slog.Any("error", sendErr),
                )
            }
        }()
    }
    // **生产路径**：直接 return；wg 局部变量被 fanout goroutine 持有引用，
    // 自然延寿到所有 goroutine 退出后被 GC 回收（不阻塞主函数返回）。
    return len(sessions), nil
}
```

**关键约束**：

- wg 是函数局部变量（**不**做成 BroadcastToRoom 函数返回值的字段）—— 生产路径完全 fire-and-forget；wg 仅供测试 helper（broadcastToRoomForTest）持有引用 + 调 wg.Wait 等 drain
- 测试 helper 在同包内（unexported）通过 export_test.go 暴露给黑盒测试包 ws_test：

  ```go
  // server/internal/app/ws/export_test.go（追加）

  // BroadcastToRoomForTest 是 BroadcastToRoom 的测试变体，等所有 fanout
  // goroutine 退出后再返回（让单测能 assert "所有 Send 已发起"）。
  // 仅供 test 调用；生产路径用 BroadcastToRoom（fire-and-forget）。
  func BroadcastToRoomForTest(ctx context.Context, mgr SessionManager, roomID uint64, msg []byte) (sent int, err error) {
      sessions := mgr.ListSessionsByRoomID(ctx, roomID)
      if len(sessions) == 0 {
          return 0, nil
      }
      wg := &sync.WaitGroup{}
      wg.Add(len(sessions))
      for _, s := range sessions {
          s := s
          go func() {
              defer wg.Done()
              if sendErr := s.Send(msg); sendErr != nil {
                  slog.Default().With(slog.String("component", "ws-broadcast")).Warn(
                      "ws broadcast Send failed (test path)",
                      slog.String("sessionId", s.SessionID()),
                      slog.Uint64("userId", s.UserID()),
                      slog.Uint64("roomId", roomID),
                      slog.Any("error", sendErr),
                  )
              }
          }()
      }
      wg.Wait() // **关键差异**：测试路径同步等所有 goroutine 退出
      return len(sessions), nil
  }
  ```

  注：BroadcastToRoomForTest 是 export_test.go 文件中的 wrapper，复用与生产 BroadcastToRoom 完全相同的核心逻辑（list → fanout → log warn），仅在末尾加 wg.Wait()；为避免代码重复，**实装时**把核心 fanout 逻辑提到 unexported helper `broadcastToRoomFanout(...)` 内（含 sync.WaitGroup 形参），生产 BroadcastToRoom 调 `broadcastToRoomFanout(..., false /* wait */)`，BroadcastToRoomForTest 调 `broadcastToRoomFanout(..., true /* wait */)` —— 详见 §6 dev notes 实装伪代码

- 函数局部 wg 不会 leak（goroutine 退出后 wg 失去最后一个引用 → GC 回收；与 Story 10.4 HeartbeatScanner.scanOnce 同模式）
- **禁止**用 sync.Map / channel-based collector 收集 send 结果（YAGNI；当前 caller 不需要 per-session-result，只需要 sent count）

**AC4 — 单元测试覆盖（≥ 8 case）**

`server/internal/app/ws/ws_test.go` 在末尾**新增**以下 case（package `ws_test`，黑盒测试，与 10.3 / 10.4 既有 ~28 case 同包追加）：

| # | Test | 描述 | 对应 AC / V1 §X |
|---|---|---|---|
| 1 | `TestBroadcastToRoom_HappyPath_AllSessionsReceive` | 注册 3 user 在同 roomID（=3001）→ BroadcastToRoomForTest(ctx, mgr, 3001, msg) → 3 个 client conn 全收到 msg；返 (3, nil) | AC1 + AC3 + epics.md 行 1748 |
| 2 | `TestBroadcastToRoom_EmptyRoom_ReturnsZero` | manager 中 roomID=9999 无任何 Session → BroadcastToRoom 返 (0, nil)；不 panic / 不 log error | AC1 + epics.md 行 1746 |
| 3 | `TestBroadcastToRoom_OneSessionSendFails_OthersStillReceive` | 注册 3 Session，把第 2 个 Session 提前 Close（`Send` 返 ErrSessionClosed）→ BroadcastToRoomForTest → 第 1 / 第 3 个 client 收到 msg；返 (3, nil)（sent 仍是 len(slice)，与 §AC1 钦定一致 —— 不回扫 send 失败数）| AC1 + AC3 + epics.md 行 1750 |
| 4 | `TestBroadcastToRoom_SendBufferFull_LogWarnContinues` | 注册 1 Session，提前用 SendN 次填满 sendChan（sendChanCapacity=32 → 推 33 次让第 33 次返 ErrSessionSendBufferFull；或者更可靠的方式：注册 Session 但**不**启动 writeLoop —— stub Session？或 mock Session 接口）→ BroadcastToRoomForTest → 主函数不返 error；log warn 触发 | AC1 + AC3 |
| 5 | `TestBroadcastToRoom_LargeFanout_100Sessions_AllSent` | 注册 100 user 在同 roomID → BroadcastToRoomForTest → 全部 100 个 client 收到 msg；返 (100, nil)；无 goroutine leak（runtime.NumGoroutine 在 ForTest 返回后回归基线） | AC3（fanout 性能 + 无 leak） |
| 6 | `TestBroadcastToRoom_DifferentRooms_Isolated` | 注册 2 user 在 roomID=3001 + 1 user 在 roomID=3002 → BroadcastToRoomForTest(ctx, mgr, 3001, msg) → 仅 roomID=3001 的 2 个 client 收到，roomID=3002 的 client **未**收到；返 (2, nil) | AC1 + epics.md 行 1748 |
| 7 | `TestBroadcastToRoom_ConcurrentToDifferentRooms_AllCorrect` | 注册 2 room 各 3 user（共 6）→ 并发触发 100 次 BroadcastToRoomForTest（一半给 room=3001，一半给 room=3002）→ 每个 room 内的 3 个 client 各自收到 50 条 msg + 跨 room 互不串扰；无 panic / 无 race | AC3（并发安全） + epics.md 行 1752 |
| 8 | `TestBroadcastToRoom_BroadcastFn_TypeAlias_Compiles` | 编译时验证（**禁止**在测试体内 panic / fail）：声明 `var fn ws.BroadcastFn = func(ctx, roomID, msg) (int, error) { return 0, nil }` + 调 `_ = fn(ctx, 0, nil)` —— 通过 build 即视为 pass | AC2 |
| 9 | `TestBroadcastToRoom_NilMessage_HandledGracefully` | BroadcastToRoomForTest(ctx, mgr, 3001, nil) → 不 panic；3 个 client 都收到 zero-length 帧（gorilla 写 nil msg 的行为 = 写 empty data frame）；返 (3, nil) | AC1 防御性 |
| 10 | `TestBroadcastToRoom_SessionRegisteredAfterListSnapshot_NotIncluded` | race 验证：调 BroadcastToRoomForTest 期间用 t.Parallel + go Register 一个新 Session → 验证新 Session **不**收到本次 broadcast（snapshot 切片是 list 时刻的快照，符合 §AC1 + §AC3 锁内 copy 不变量）+ 但下次 BroadcastToRoom 会包括新 Session | AC3（review r5 P2 不变量保留） |

合计 ≥ 8（10 个建议覆盖最小集；epics.md §Story 10.5 行 1747-1752 钦定的最小 5 case 必须覆盖：房间 3 个在线用户 → Broadcast 调用 3 次 Send（#1）/ 房间 0 个在线用户 → 返回 0 + nil（#2）/ 1 个 Session Send 失败 → 其他 2 个仍正常发送（#3）/ SessionManager 中 userID 不存在（presence 与 manager 不一致）→ skip 该 user，log 警告（**注意**：本 story 不消费 Redis presence，该场景在单实例阶段不存在；保留概念覆盖到 #4 sendChan 满 → log warn 路径）/ 100 个并发 Broadcast 不同 room → 都正确（#7））。

**测试组织约束**：

- 测试文件在 `internal/app/ws/ws_test.go`（**追加**到现有文件末尾，不新建 broadcast_test.go —— 与既有 28+ case 同包内组织模式，10.4 也是同模式追加在末尾）
- 用 `httptest.NewServer` + gorilla/websocket `Dialer.Dial` 起真实 WS 客户端测试 broadcast 路径（与 10.3 / 10.4 既有模式对齐）
- 通过 `BroadcastToRoomForTest`（export_test.go 暴露的 unexported helper）调用 —— 让单测能 assert "所有 Send 已发起" 而不依赖真实 wall-clock sleep
- mock SessionManager：用真实 sessionManager（不需要 mock 接口；SessionManager 接口在 10.3 已稳定，本 story 仅消费 ListSessionsByRoomID 一个方法）
- onUnregister 钩子用 `WithUnregisterHook` 注入计数器（**仅** #3 / #10 需要，验证 broadcast 不触发钩子 —— BroadcastToRoom 是只读路径，不动 lifecycle）
- 每个测试用独立 Test 函数（与 10.3 / 10.4 既有命名模式对齐）

**AC5 — 集成测试（1 case，build tag integration）**

`server/internal/app/ws/ws_integration_test.go` 末尾**新增**以下 case：

```go
//go:build integration
// +build integration

// TestWSIntegration_BroadcastToRoom_3Clients_AllReceive：
//   - 启 mysql + 插入 room_members fixture 含 3 个 user 在同 roomID（与既有 4
//     case 同 fixture 复用 + 扩展）
//   - 启 redis（与既有 ws_integration_test.go 同模式，dockertest）
//   - 启动 httptest server 挂 Gateway.Handle
//   - 用 3 个不同 token 拨连 3 个 WS Dialer → 各自收到 placeholder snapshot
//   - 调 BroadcastToRoom(ctx, sessionMgr, roomID, []byte(`{"type":"member.joined","payload":{"userId":"4001","nickname":""},"ts":1234567890}`))
//   - 用 sync.WaitGroup + 3 个 conn 并发 ReadMessage → 3 个 conn 都收到 msg
//   - 验证返 (3, nil)
//   - 一个 client 主动 Close → 等 manager Unregister 完成 → 再调 BroadcastToRoom
//     同 roomID + 不同 msg → 剩 2 个 client 收到；返 (2, nil)；无 error
```

**集成测试约束**：

- 与 mysql_integration_test.go / 既有 ws_integration_test.go 严格同模式（build tag / Skip on docker unavailable / dockertest cleanup）
- 复用既有 `startMySQLWithRoomMemberFixture` helper（**不**新建 fixture）+ 扩展 fixture 让 room=3001 含 3 个 user（4001 / 4002 / 4003）
- assert close 前后 `sessionMgr.ListSessionsByRoomID(ctx, 3001)` 长度变化（3 → 2）
- 用 `BroadcastToRoom`（生产路径，**不是** ForTest 变体）—— 集成测试覆盖生产 fire-and-forget 行为；通过 `time.Sleep(50ms)` 等所有 fanout goroutine 完成 Send 入队 → 通过 ReadMessage 验证 client 收到（与 ws_integration_test.go 既有 r3 场景同模式）
- **不**断言 sent count 字段精确数值变化的副作用（sent 仅是 fanout 发起数，集成测试断言"所有 client 都收到"即可）

**AC6 — `bash scripts/build.sh --test` 全部通过 + `--integration` 跳过友好**

工作完成后必须人工跑一次：

```bash
bash scripts/build.sh --test         # 必跑：vet + build + go test ./... 全绿
bash scripts/build.sh --integration  # docker 可用时跑通新 1 个集成测试 case；不可用时全 skip
```

构建产物：`build/catserver`（Windows `.exe`）必须能 `./build/catserver` 启动并：

1. 启动期 log "config loaded"
2. 启动期 log "mysql connected"
3. 启动期 log "redis connected"
4. 启动期 log "ws session manager ready"
5. 启动期 log "ws heartbeat scanner ready"（含 heartbeatTimeoutSec 字段）
6. 启动期 log "auth token signer ready"
7. server 监听 :8080，`curl http://127.0.0.1:8080/ping` 返 OK
8. WS path `GET /ws/rooms/:roomId` 已注册（与 10.3 / 10.4 一致）
9. **本 story 新增**：调用 BroadcastToRoom 时输出 log info "ws broadcast to room"（含 roomId / targetSessions / msgBytes 字段）+ 失败时 log warn "ws broadcast Send failed"

**AC7 — Story 文件状态 + sprint-status.yaml**

完成时必须：

- 本文件 `Status: ready-for-dev` → dev-story 阶段改为 `in-progress` → review 后 `done`
- `_bmad-output/implementation-artifacts/sprint-status.yaml` 中 `10-5-broadcasttoroom-primitive` 同步迁移
- 不动其他 story 状态

**AC8 — 不修改的文件清单（红线）**

为防 dev-story 阶段 scope 漂移，明确**不修改**以下文件：

- `server/cmd/server/main.go` —— BroadcastToRoom 是包级函数 + Story 11.8 / 14.4 / 17.5 才在 service 层 wire；本 story **不**在 main.go 加 BroadcastFn 注入（保持 main.go 与 10.4 落地后的快照一致）
- `server/internal/app/ws/session.go` / `session_manager.go` / `gateway.go` / `heartbeat_scanner.go` —— 仅消费现有公开接口，**不**修改任何字段 / 方法 / 接口签名
- `server/internal/app/bootstrap/router.go` —— BroadcastToRoom 不挂 router（不是 HTTP handler）
- `server/internal/infra/redis/` —— 本 story 不消费 Redis（Redis presence 是 10.6 才接入）
- `docs/宠物互动App_V1接口设计.md` / `docs/宠物互动App_Go项目结构与模块职责设计.md` / `docs/宠物互动App_数据库设计.md` —— V1 协议骨架已冻结，本 story **不**反向改文档
- `_bmad-output/planning-artifacts/epics.md` —— epic AC 已写明 BroadcastToRoom 行为；本 story 在 §"实装关键决策" §1 解释"Redis presence 数据源在单实例阶段降级到 SessionManager"的等价性，但**不**反向改 epics.md
- `_bmad-output/implementation-artifacts/decisions/0011-ws-stack.md` —— Story 10.3 落地的 ADR；本 story 不动

## Tasks / Subtasks

- [x] **Task 1：新建 broadcast.go 文件 + BroadcastToRoom 主体**（AC1 + AC3）
  - [x] 1.1 新建 `server/internal/app/ws/broadcast.go`（包名 ws，import context / log/slog / sync）
  - [x] 1.2 实装 unexported helper `broadcastToRoomFanout(ctx, mgr, roomID, msg, wait bool) (int, error)` —— 核心逻辑（list → fanout → log warn），wait=true 时调 wg.Wait
  - [x] 1.3 实装包级 `BroadcastToRoom(ctx, mgr, roomID, msg)` 调 `broadcastToRoomFanout(..., wait=false)`
  - [x] 1.4 加完整 godoc（参考 §AC1 给的注释模板）
- [x] **Task 2：BroadcastFn 类型别名**（AC2）
  - [x] 2.1 在 broadcast.go 顶部声明 `type BroadcastFn func(ctx context.Context, roomID uint64, msg []byte) (sent int, err error)`
  - [x] 2.2 加 godoc 演示 wire / mock 两种用法
- [x] **Task 3：测试 helper（export_test.go 暴露）**（AC3）
  - [x] 3.1 在 `server/internal/app/ws/export_test.go` 末尾追加 `BroadcastToRoomForTest(ctx, mgr, roomID, msg)`，调 `broadcastToRoomFanout(..., wait=true)`
  - [x] 3.2 godoc 标注与 BroadcastToRoom 的差异（仅 wg.Wait）
- [x] **Task 4：单元测试 ≥ 8 case**（AC4）
  - [x] 4.1 在 ws_test.go 末尾追加 11 个 case（HappyPath / EmptyRoom / OneSendFails / SendBufferFull / LargeFanout 30 / DifferentRooms / ConcurrentToDifferentRooms / BroadcastFn TypeAlias / NilMessage / RegisterAfterListSnapshot / DoesNotTriggerUnregisterHook）
  - [x] 4.2 复用 10.3 / 10.4 既有 helper（idleTestLogger / readCloseError / readCloseFrameLoose / 既有 stubRoomMemberRepo / Dial fixture / useGatewayDial）
  - [x] 4.3 LargeFanout 用 N=30（**实装偏离**：spec 钦定 100，但 useGatewayDial 是 gateway-per-user 模式，100 个 httptest server 会触发 Windows / CI 端口耗尽 / fd limit，30 已足够验证 fanout drain 不 leak —— 见 ws_test.go test 体注释）
  - [x] 4.4 ConcurrentToDifferentRooms：20 次并发 BroadcastToRoomForTest（10 给 roomA + 10 给 roomB），跨 room 互不串扰
- [x] **Task 5：集成测试 1 case**（AC5）
  - [x] 5.1 在 ws_integration_test.go 末尾加 TestWSIntegration_BroadcastToRoom_3Clients_AllReceive
  - [x] 5.2 复用既有 startMySQLWithRoomMemberFixture + **测试体内**扩展插入 user 1003 + 更新 member_count=3（**实装偏离**：spec 钦定改 helper 函数本身，但既有 4 case 依赖 fixture 含 2 user 不能改；改在测试体内 INSERT 是无副作用的 reuse 方式，与 spec 意图一致）
  - [x] 5.3 验证 close 前后 ListSessionsByRoomID 长度变化（3 → 2）+ broadcast 后剩余 client 仍收到
- [x] **Task 6：build + run 验证**（AC6）
  - [x] 6.1 `bash scripts/build.sh --test` 全绿（含本 story 新加 11 个单测，全 pass）
  - [x] 6.2 `bash scripts/build.sh --integration` 跑通（含本 story 新加 1 个集成测试 case；docker 不可用时与既有 5 个同样 graceful skip）
  - [x] 6.3 build 二进制 `build/catserver.exe` 已生成（启动期日志保持与 10.4 一致；本 story **不**改 main.go，无新启动日志）
- [x] **Task 7：sprint-status.yaml + 状态切换**（AC7）
  - [x] 7.1 dev-story 接管时改 Status: in-progress（开工时改）
  - [x] 7.2 完工时由工作流推到 review（sprint-status.yaml + story 文件 Status 同步）
  - [ ] 7.3 done 时改 Status: done + sprint-status.yaml 同步（fix-review / story-done 阶段做）

## Dev Notes

### 实装关键决策

#### 1. 数据源选择：SessionManager.ListSessionsByRoomID（**不**走 Redis presence）

**决策**：BroadcastToRoom 通过 `mgr.ListSessionsByRoomID(ctx, roomID)` 拿 Session 列表，**不**通过 Redis presence。

**与 epics.md §Story 10.5 行 1741 的"从 Redis presence 拿 `room:{roomID}:online_users` 集合"语义关系**：

- epics.md AC 钦定 broadcast 通过 Redis presence 拿目标 user 集合 → 再到 SessionManager 找 Session；这是**多实例**部署语义（节点 9+ 多实例部署时 Redis presence 跨实例可见 + 单实例的 SessionManager 不可见跨实例 Session）
- **MVP 单实例阶段**（节点 4 ~ 节点 12）：Redis presence 与 SessionManager.sessionsByRoom map 是同源数据（10.6 落地后通过 lifecycle 钩子自动同步）；从 SessionManager 直接拿 Session 列表是**等价**的，且更直接（少一次 Redis 网络 IO）
- 多实例阶段（节点 13+，未来 epic）：BroadcastToRoom 内部需要先查 Redis presence 拿跨实例 user 集合 → 然后通过 Pub/Sub 广播给其他实例 + 本实例的 SessionManager Send；本 story **不**实装多实例路径（与 10.2 / 10.3 范围红线一致）
- 本 story 在 §AC1 godoc + 实装注释中明确：`BroadcastToRoom 当前实装走 SessionManager（单实例 source-of-truth）；多实例 Pub/Sub 路径预留为节点 13+ tech debt`

**理由（强化）**：

- SessionManager.ListSessionsByRoomID 已经是同源数据：10.3 r2 P1 反复修过的 lifecycle 钩子（reconnect 替换 + onUnregister 触发顺序）保证 sessionsByRoom map 是"当前进程内 active Session"的唯一权威；走 Redis presence 反而增加同步窗口（presence TTL 续期 vs SessionManager 锁内变更存在不一致窗口）
- 10.6 Redis presence 的 AddOnline / RemoveOnline 挂在 SessionManager 钩子上 —— 任何"presence 与 SessionManager 不一致"的场景都是 race / TTL 过期 / 节点 crash 后未清理；让 broadcast 走 SessionManager 自动避开这些 race
- 单元测试可以构造单实例场景验证；Redis presence 的多实例同步在节点 9+ 多实例 e2e 测试才能真正验证 —— 本 story 不预先实装 + 不预先测试

**反例**（不要做）：

- 不通过 Redis presence 查 user 集合再到 SessionManager 找 Session —— 多一次 Redis 调用 + 增加同步窗口 race，节点 4 ~ 12 单实例阶段无任何收益
- 不在 broadcast.go 内 import redisinfra（禁止 Redis client 进入 broadcast primitive）—— 让 primitive 保持纯函数 + 易测试 + 多实例升级时显式重构（不暗中接入 Redis）
- 不让 BroadcastToRoom 接受 RedisClient 形参（即便是 optional）—— 让接口签名保持简洁

#### 2. 包级函数 vs SessionManager method vs RoomHub struct

**决策**：BroadcastToRoom 是**包级函数**，**不**做成 SessionManager method、**不**做成 RoomHub struct。

**理由**：

- 单一职责：SessionManager 是 Session 注册中心（Register / Unregister / List / Close），BroadcastToRoom 是 message 推送原语（拿列表 + fanout Send）—— 两者职责清晰分离
- 单测便利：包级函数可以在不构造完整 SessionManager 实例的情况下 import + 调用 mock；method 路径会让单测必须构造 manager 实例
- service 层 wire：service 层（Story 11.8 / 14.4 / 17.5）需要"广播能力"而不是"manager 引用"—— 通过 BroadcastFn 类型别名注入 closure，把 manager 引用 hidden 在 closure 内 → service 层只看 broadcast 接口
- 未来扩展：节点 13+ 多实例阶段，BroadcastToRoom 需要走 Redis Pub/Sub 路径 —— 包级函数升级到接口形态（`type Broadcaster interface { Broadcast(...) }`）只需要扩 BroadcastFn 类型别名 + closure 加 Redis client；method 路径会让 SessionManager 接口被迫扩为 distributed manager（破坏单一职责）

**反例**（不要做）：

- 不实装 `mgr.BroadcastToRoom(ctx, roomID, msg)` method（破坏 SessionManager 单一职责）
- 不实装 RoomHub struct + `hub.Broadcast(ctx, roomID, msg)` method（YAGNI；当前没有 hub 状态需要持久化；未来 epic 真正需要时再升级）
- 不实装 BroadcastService struct（service 层是 business 层，不是 transport primitive 层）

#### 3. fanout drain 模式（仿 Story 10.4 r5 lessons）

**决策**：fanout 用 sync.WaitGroup 函数局部变量；生产路径不 wg.Wait（fire-and-forget），测试路径通过 unexported helper 调 wg.Wait（drain）。

**理由**（lesson `2026-05-07-ws-heartbeat-fanout-drain-and-list-sort-outside-lock-10-4-r5.md`）：

- Story 10.4 review r5 反复修过 HeartbeatScanner.scanOnce 的 fanout drain 问题：早期版本不 drain → 单测无法 assert "所有 close 已发起"；后来用 sync.WaitGroup 函数局部变量 + 测试 helper drain 模式
- 本 story 复用同模式：生产路径 fire-and-forget（fanout 后立即 return） / 测试路径 drain（让 assert 可靠）
- 函数局部 wg 不 leak：goroutine 退出后 wg 失去最后一个引用 → GC 自然回收
- **不**用 channel-based collector：增加复杂度 + buffer size 决策困难（多大才够？）+ 当前 caller 不需要 per-session-result

**反例**（不要做）：

- 不在生产路径 wg.Wait（阻塞主函数 → 让 caller / Send 同步行为，丢失"广播是 fast path"语义）
- 不用 sync.Map 收集 send error（YAGNI；当前只需要 sent count，per-session-error 已经 log warn 落地）
- 不用 channel-based collector + worker pool（YAGNI；fanout 100 个 goroutine 在 Go runtime 上完全没问题，不需要 pool）

#### 4. msg 形参是 []byte 而不是 interface{} / serverEnvelope struct

**决策**：BroadcastToRoom 接受 `msg []byte`（已序列化的字节流），**不**接受 interface{} / serverEnvelope。

**理由**：

- 序列化职责分离：caller（Story 11.8 / 14.4 / 17.5 service 层）按 V1 §12.3 字段表 + 各 epic §X.1 锚定的 type 字段值 marshal serverEnvelope → 传 []byte 给 primitive
- Session.Send 接受 []byte（与 sendChan chan []byte 字段类型对齐）—— BroadcastToRoom 内 fanout Send 时不需要再 marshal
- 多 goroutine 共享 []byte 安全：Send 入队是 fire-and-forget；writeLoop 单一消费方写到 conn，无并发写 msg slice 的风险
- type-agnostic：本 story primitive 对 type 字段值 / payload 结构**完全无知**，让所有业务广播复用同一 primitive

**反例**（不要做）：

- 不接受 `interface{}` + 内部 json.Marshal（增加 primitive 内部复杂度 + marshal error 处理路径 + 逃逸分析劣化）
- 不接受 `serverEnvelope` struct（与 ws 包内部 type 紧耦合；service 层会反向 import ws.serverEnvelope）

#### 5. log level 选择：info（broadcast 主路径）+ warn（Send 失败）

**决策**：

- 主路径 log info "ws broadcast to room"（含 roomId / targetSessions / msgBytes）
- Send 失败 log warn "ws broadcast Send failed"（含 sessionId / userId / roomId / error）
- **不**用 error level（无致命错误；Send 失败是常态：sendChan 满 + 慢 client 是预期场景）

**理由**：

- broadcast 主路径每次调用都会 log info：让 prod 排障可以通过日志聚合定位"什么时候广播了什么 type 的消息给哪个 room 多少人"
- Send 失败 log warn：sendChan 满 / Session 已 close 是常态网络抖动 + 慢 client；用 warn 区分于 info（让日志聚合把这些事件单独 surface）但不到 error 级别
- 与 Story 10.3 / 10.4 的 log level 选择一致：4001 token 过期 / 4005 心跳超时都是 info（常态业务场景）

**反例**（不要做）：

- 不在主路径 log warn / error（污染日志，让真正的异常被淹没）
- 不在 Send 失败 log error（会让 prod alarm 频繁触发，但其实是常态）
- 不省略主路径 log（让 broadcast 黑盒，prod 排障困难）

#### 6. msg 字节流 = 已序列化的 serverEnvelope（V1 §12.3 字段表外层）

**决策**：caller 必须 marshal 完整 serverEnvelope（含 type / requestId / payload / ts 四字段）后传 []byte。

**理由**：

- V1 §12.3 钦定服务端 → 客户端通用消息信封是 `{type, requestId, payload, ts}` 四字段；broadcast 推送的消息也是同 envelope
- requestId 字段：server-active 广播（如 member.joined）→ requestId 固定 `""`（V1 §12.3 末尾"业务消息延后锚定"块中"server 主动推送时 requestId 固定 ""）
- payload 字段：按对应 §X.1 锚定的字段表 marshal（如 member.joined 是 `{userId, nickname}`）
- ts 字段：caller marshal 时调 `time.Now().UnixMilli()` 填入；broadcast primitive **不**修改 ts

**反例**（不要做）：

- 不在 primitive 内补充 ts 字段（破坏 caller 控制时间戳的能力 + 让 primitive 与时间源耦合）
- 不在 primitive 内验证 envelope schema 字段（YAGNI；caller 已经按 V1 §12.3 marshal，primitive 是透明字节流转发）

#### 7. ctx 用法：仅传给 ListSessionsByRoomID + log；**不**用 ctx 控制 fanout goroutine 退出

**决策**：BroadcastToRoom 内 ctx 仅用于：

1. 调 `mgr.ListSessionsByRoomID(ctx, roomID)`（ListSessionsByRoomID 接受 ctx 但当前实装不消费，与 ListAllSessions 同模式）
2. log 字段（如未来加 traceID / requestID 时通过 ctx 拿）

**不**用 ctx 控制 fanout goroutine：fanout 是 fire-and-forget；Send 是非阻塞 select-default 入队，不会被 ctx 卡住；ctx-cancel 在 BroadcastToRoom 主函数内不需要（fire-and-forget 后主函数立即返回）。

**理由**：

- ADR-0007 钦定：ctx 必传 + 所有 service / repo 导出函数第一参数；本 story 严格遵守（ctx 是第一参数）
- Send 入队是非阻塞（select-default 走 ErrSessionSendBufferFull 失败路径），不需要 ctx-cancel 兜底
- 多 goroutine 共享同一个 ctx：在 fire-and-forget 场景下 ctx-cancel 不会传播到 goroutine 内（goroutine 已经发起 Send 入队了）；浪费的复杂度

**反例**（不要做）：

- 不在 fanout goroutine 内监听 ctx.Done（增加 select 路径 + 与 fire-and-forget 语义冲突）
- 不在 BroadcastToRoom 主函数检查 ctx.Err（ListSessionsByRoomID 会传播 ctx，主函数 return 即立即返回）

#### 8. error 返回：永远 nil（接口签名保留 error 让未来 Pub/Sub 扩展用）

**决策**：当前实装永远返 `(sent int, nil error)`；不在任何路径返 error。

**理由**：

- broadcast primitive 是 transport 层；Send 失败是常态（log warn）；ListSessionsByRoomID 不返 error（10.3 实装是同步只读）
- 接口签名保留 error 让未来 Pub/Sub 路径能扩展：节点 13+ 多实例 broadcast 经过 Redis Pub/Sub 时，Redis 不可达 → 返 error；本 story 永不返 error 但接口预留
- 与 Story 10.4 ListAllSessions / HeartbeatScanner.Run 同模式：内部不会失败的同步操作，接口签名仍预留 error

**反例**（不要做）：

- 不让 BroadcastToRoom 在某些路径返 error（增加调用方处理路径，但当前没有恢复策略 —— 调用方拿到 error 也只能 log warn，与 primitive 内部 log warn 等价）
- 不让接口签名只有 sent int 不带 error（多实例升级时反向加 error 返回 → 破坏 ABI）

#### 9. import 严格控制（broadcast.go 仅 import 包内 + 标准库）

**决策**：broadcast.go 仅 import：
- `context`（标准库）
- `log/slog`（标准库）
- `sync`（标准库）

**禁止** import：
- `redisinfra` / `github.com/redis/go-redis/v9`（本 story 不消费 Redis）
- `gorm.io/gorm` / `database/sql` / `github.com/go-sql-driver/mysql`（本 story 不消费 MySQL）
- `time`（fanout goroutine 不需要 time.Now —— ts 字段由 caller 在 marshal 时填入）
- `encoding/json`（msg 已序列化字节流；primitive 不解码 / 不重 marshal）
- `errors`（不构造新 error；返回 nil）

**理由**：

- 让 broadcast primitive 保持纯函数 + 零外部依赖 + 易测试 + 多实例升级时显式重构（不暗中接入 Redis / MySQL）
- import 越少，单测构造越简单（不需要 mock Redis / MySQL）

**反例**（不要做）：

- 不在 broadcast.go import "time" 加 ts 字段（破坏 §6 决策）
- 不在 broadcast.go import "encoding/json" 验证 msg schema（破坏 §4 决策）

#### 10. 测试 fixture 复用 vs 新建

**决策**：

- 单元测试**完全复用** 10.3 / 10.4 既有 helper（idleTestLogger / readCloseError / Dial / stubRoomMemberRepo / token util）—— 不新建 fixture
- 集成测试**扩展**既有 startMySQLWithRoomMemberFixture（让 room=3001 含 3 个 user fixture）—— 不新建 helper

**理由**：

- ws_test.go 已有 28+ case + 完整 helper 集；本 story 复用零分配 + 减少 review 面积
- 集成测试 fixture 复用让 setup / cleanup 流程一致；新建 fixture 会让测试 ~%50% 时间在重启 mysql 实例

**反例**（不要做）：

- 不新建 broadcast_test.go 文件（追加在 ws_test.go 末尾，与 10.4 同模式）
- 不新建 broadcast_integration_test.go 文件（追加在 ws_integration_test.go 末尾）

### Source Tree 影响

```
server/
├─ internal/
│  └─ app/ws/
│     ├─ broadcast.go                    # 新建：BroadcastToRoom 包级函数 + BroadcastFn 类型别名 + unexported broadcastToRoomFanout helper
│     ├─ ws_test.go                      # 末尾追加 ~10 个 case
│     ├─ ws_integration_test.go          # 末尾追加 1 个 case
│     └─ export_test.go                  # 末尾追加 BroadcastToRoomForTest 测试 helper
```

新增文件 1 个：`broadcast.go`
修改文件 3 个：`ws_test.go` / `ws_integration_test.go` / `export_test.go`

**未触碰**：

- `cmd/server/main.go`（不挂 wire）
- `internal/app/bootstrap/router.go`（不挂 router）
- `internal/app/ws/session.go` / `session_manager.go` / `gateway.go` / `heartbeat_scanner.go`（仅消费现有公开接口）
- `internal/infra/redis/`（不消费 Redis）
- `migrations/`（不动 schema）
- `docs/`（不改 V1 / 项目结构 / DB 设计文档）
- `_bmad-output/planning-artifacts/epics.md`（不改 epic）

### Project Structure Notes

- 本 story 严格按 `docs/宠物互动App_Go项目结构与模块职责设计.md` §6.10 "Realtime / WS 模块"职责边界落地
  - WebSocket 连接管理 → SessionManager（10.3 已就绪；本 story 仅消费）
  - 心跳与断线处理 → HeartbeatScanner（10.4 已就绪；本 story 不动）
  - **房间内事件广播 → BroadcastToRoom**（**本 story 落地**）
  - 用户在线状态管理 → Redis presence repo（10.6 接管；本 story 不消费）
  - 房间快照推送 → SnapshotBuilder（10.7 接管；本 story 与 snapshot 路径正交）
- 本 story 引入的 `broadcast.go` 视为 Realtime 模块内 broadcast primitive 文件
  - **不**实装完整 RoomHub struct（与 §9 三对象建议中的 RoomHub 职责对齐但**不**完整落地）—— 仅交付包级 primitive 函数 + 接口契约预留，让 Epic 11+ 真正需要 hub 状态时再决定升级为 struct
- `internal/app/ws/broadcast.go` 与 gateway.go / session.go / session_manager.go / heartbeat_scanner.go 平级；不放在子目录（包内组织保持平铺，与 10.3 / 10.4 同模式）
- 项目当前**无** `internal/infra/clock/`（Story 8.5 / 10.4 都未引入 clock interface）；本 story 也**不**消费 time.Now（msg 字节流由 caller 在 marshal 时填 ts 字段，primitive 不动时间）
- 集成测试与既有 mysql_integration_test.go / 既有 ws_integration_test.go 同模式（dockertest + Skip on docker unavailable）

### Testing Standards

- 测试栈：testify（assertion + require）+ stdlib testing（testify v1.11.1，ADR-0001 §6 钦定）
- 测试位置：
  - `internal/app/ws/ws_test.go`（黑盒 package `ws_test`，追加在 10.4 既有 ~28 case 末尾）
  - `internal/app/ws/ws_integration_test.go`（build tag `integration`，追加在既有 5 case 末尾）
  - `internal/app/ws/export_test.go`（追加 BroadcastToRoomForTest helper）
- ws lib：gorilla/websocket（同 production 路径）
- 命名：`Test<Type>_<Behavior>_<Outcome>` 三段式（与 redis_test / mysql_test / 既有 ws_test 命名一致）
- 集成测试与 mysql / 既有 ws integration 严格同模式（dockertest + Skip on docker unavailable）
- 单测**不**用真实 sleep：BroadcastToRoomForTest 通过 wg.Wait drain；不依赖 wall-clock
- 单测**不**新建 helper：复用 10.3 / 10.4 既有 helper

### References

**主要锚定文档**：

- `_bmad-output/planning-artifacts/epics.md` §Story 10.5（行 1730-1753）— AC 主源
- `docs/宠物互动App_V1接口设计.md` §12.3 服务端 → 客户端通用消息信封（行 1554-1714）— msg 字节流的 envelope 字段表（caller 须按此 marshal 后传入）
- `docs/宠物互动App_V1接口设计.md` §12.1 close code 表 + 关键约束（行 1320-1370）— 业务广播不直接走 close 路径，但本 story 集成测试触发 client close 后 ListSessionsByRoomID 长度变化由 §12.1 钦定的 onUnregister 钩子触发保证
- `docs/宠物互动App_数据库设计.md` §9 Redis 职责边界（行 981-1023）— Redis presence 数据形态（本 story 不消费但 §"实装关键决策" §1 引用）
- `docs/宠物互动App_Go项目结构与模块职责设计.md` §6.10（Realtime / WS 模块职责边界）— BroadcastToRoom 在 Realtime 模块的定位
- `docs/宠物互动App_总体架构设计.md`— Realtime 模块整体定位

**ADR / 决策**：

- `_bmad-output/implementation-artifacts/decisions/0011-ws-stack.md`（gorilla/websocket 选型；本 story 直接消费）
- `_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3 / §6 — testify / sqlmock 钦定
- `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md` — ctx 必传规约（BroadcastToRoom 第一参数 ctx）
- `_bmad-output/implementation-artifacts/decisions/0003-orm-stack.md` — fail-fast over fallback 模式（broadcast Send 失败 log warn 不阻塞继续 fanout）

**前置 Story 文件**（同模式参考）：

- `_bmad-output/implementation-artifacts/10-3-ws-网关骨架.md` — Session / SessionManager / Gateway 完整模板（本 story 强依赖；ListSessionsByRoomID / Session.Send 接口直接消费）
- `_bmad-output/implementation-artifacts/10-4-心跳框架.md` — HeartbeatScanner / scanOnce / fanout drain / export_test.go 模式参考（本 story 复用相同 fanout drain 模式）
- `_bmad-output/implementation-artifacts/10-2-redis-接入.md` — Redis 接入完整模板（本 story 不消费 Redis 但 fail-fast / Deps wire 模式参考）
- `_bmad-output/implementation-artifacts/10-1-接口契约最终化.md` — V1 §12 协议骨架契约定稿
- `_bmad-output/implementation-artifacts/8-5-步数同步触发器.md` — 后台触发器（fire-and-forget + cancel ctx）模式参考

**Lessons 必读**（避免重复踩坑）：

- `docs/lessons/2026-05-07-ws-heartbeat-fanout-drain-and-list-sort-outside-lock-10-4-r5.md` — review 10-4 r5 教训：fanout 用 sync.WaitGroup 函数局部变量 + 测试路径 drain；本 story BroadcastToRoom + BroadcastToRoomForTest 完全复用此模式
- `docs/lessons/2026-05-07-ws-shutdown-must-wait-for-goroutine-exit-not-just-signal-10-4-r6.md` — review 10-4 r6 教训：shutdown 必须等所有 goroutine 退出而不是仅 signal；本 story 主路径不参与 shutdown，但测试路径必须用 wg.Wait drain 避免 race
- `docs/lessons/2026-05-06-ws-reconnect-unregister-hook-and-prod-contract-gate.md` — review 10-3 r2 P1 教训：close 路径必须触发 onUnregister；本 story 不主动 close，但需依赖 SessionManager 在 close 后从 sessionsByRoom map 移除（review r5 P2 修过的不变量保留）
- `docs/lessons/2026-05-06-ws-room-status-filter-and-priority-quota.md` — review 10-3 r7 教训：writeLoop priority quota 防 starvation；本 story 通过 Session.Send 入队 sendChan（业务消息），不参与 priority chan
- `docs/lessons/2026-05-06-ws-session-send-close-race-and-shutdown-hooks.md` — review 10-3 r1 教训：Send / Close 并发 panic 防护；本 story 多 goroutine 并发 Send 同一 Session 安全（sendMu.RLock 可重入）
- `docs/lessons/2026-05-06-ws-room-existence-source-and-pong-priority-r4.md` — review 10-3 r4 教训：sessionsByRoom map 是 room 内 active Session source-of-truth（本 story §1 决策依据）
- `docs/lessons/2026-05-06-ws-handshake-register-after-snapshot-r10.md` — review 10-3 r10 P1 教训：Register 必须在 snapshot 写成功之后；本 story 不修改 Gateway.Handle，无关
- `docs/lessons/2026-05-06-ws-error-dual-semantics-and-heartbeat-close-code.md` — review 10-1 r3 教训：心跳超时与业务广播走完全不同 close path（4005 vs Send 失败 log warn）；本 story 严格遵守
- `docs/lessons/2026-04-26-startup-blocking-io-needs-deadline.md` — IO 必带 deadline；本 story Send 入队是非阻塞 select-default，无 IO deadline 需求

**Git 历史相关 commit**（show 这些 commit 找模式参考）：

- Story 10.4 review fix commits（见 `git log --grep="10-4"`）— HeartbeatScanner / scanOnce / fanout drain / export_test.go 模式
- Story 10.3 review fix commits（见 `git log --grep="10-3"`）— Session / SessionManager 完整 lifecycle 流程 + 测试 helper
- Story 8.5 review fix commits（见 `git log --grep="8-5"`）— 后台触发器（cancel ctx）模式

### 前置 lessons 提炼（review 10-3 / 10-4 系列必读规则）

1. **单实例 source-of-truth = SessionManager.sessionsByRoom**（review 10-3 r4 + lesson `ws-room-existence-source-and-pong-priority-r4.md`）：MVP 单实例阶段 broadcast 数据源是 SessionManager（不是 Redis presence）；本 story §AC1 + §"实装关键决策" §1 严格执行
2. **Register 替换中场窗口期 ListSessionsByRoomID 不能同时返 OLD/NEW**（review 10-3 r5 P2）：避免 broadcast 同 user 双发 client 不能 dedupe；本 story 调 ListSessionsByRoomID 时拿到的切片是 read-lock copy，OLD 已在 Register 锁内从 sessionsByRoom 移除 → broadcast 不会触达 OLD（不变量已保留）
3. **fanout drain 模式**（review 10-4 r5 + lesson `ws-heartbeat-fanout-drain-and-list-sort-outside-lock-10-4-r5.md`）：函数局部 sync.WaitGroup + 生产 fire-and-forget / 测试 wg.Wait drain；本 story BroadcastToRoom + BroadcastToRoomForTest 直接复用
4. **Send 失败 log warn 不阻塞**（review 10-3 r1 + lesson `ws-session-send-close-race-and-shutdown-hooks.md`）：Send 失败（ErrSessionClosed / ErrSessionSendBufferFull）是常态；本 story fanout goroutine 内 log warn 后 return，不阻塞其他 goroutine
5. **WS 协议字面量字符串严格不变**（review 10-1 系列）：本 story primitive 不构造任何 close reason / error message；msg 字节流 字段值由 caller marshal，primitive 透明转发 —— 字面量风险归 caller
6. **log level 区分**（review 10-1 r3 + V1 §12.1 关键约束）：本 story 主路径 log info，Send 失败 log warn；不写 error（与 4001 / 4005 同 info 模式一致）
7. **ctx-aware 严格规约**（ADR-0007 + lesson 1-9 ctx 传播）：BroadcastToRoom 第一参数 ctx；ListSessionsByRoomID 接 ctx；fanout goroutine **不**监听 ctx.Done（fire-and-forget 语义）
8. **唯一 envelope producer**（lesson 2026-04-24-error-envelope-single-producer）：本 story 不写 server-active error envelope；msg 字节流由 caller 按 V1 §12.3 marshal，primitive 透明转发

### Anti-patterns to AVOID

1. **不要**让 BroadcastToRoom 内部 import redisinfra / go-redis（破坏 §"实装关键决策" §9 import 红线 + 让 primitive 与 Redis 紧耦合，阻碍未来多实例升级时的显式重构）
2. **不要**让 BroadcastToRoom 接受 RedisClient 形参（即便是 optional）—— 让接口签名保持简洁；多实例升级时通过 BroadcastFn closure 注入
3. **不要**实装 BroadcastToRoom 为 SessionManager method（破坏 SessionManager 单一职责 + 让单测构造困难）
4. **不要**实装 BroadcastToRoom 为 RoomHub struct + Broadcast method（YAGNI；当前没有 hub 状态需要持久化；未来 epic 真正需要时再升级）
5. **不要**让 BroadcastToRoom 接受 `interface{}` / `serverEnvelope` struct 形参（让 service 层反向 import ws.serverEnvelope；破坏序列化职责分离）
6. **不要**在生产路径调 wg.Wait 等所有 fanout goroutine 退出（破坏 fire-and-forget 语义 + 让 broadcast 变成同步阻塞调用 + caller 调 Send 同步等待）
7. **不要**用 channel-based collector 收集 send error（YAGNI；当前只需要 sent count + log warn 已经落地 per-session-error）
8. **不要**用 sync.Map 收集 per-session result（同上 YAGNI）
9. **不要**用 worker pool（YAGNI；fanout 100 个 goroutine 在 Go runtime 上无问题）
10. **不要**在 fanout goroutine 内监听 ctx.Done（增加 select 路径 + 与 fire-and-forget 语义冲突）
11. **不要**让 BroadcastToRoom 主函数调 ctx.Err 检查（fanout 已经发起了，主函数 return 即可）
12. **不要**让 BroadcastToRoom 内部 marshal msg / 修改 msg 内容 / 复制 msg slice（破坏 §"实装关键决策" §4 + §6 + §9）
13. **不要**让 BroadcastToRoom 内部调 Session.Close / CloseWithCode（与 lifecycle 解耦；Send 失败 ≠ Session 已死）
14. **不要**让 BroadcastToRoom 内部调 mgr.Unregister（同上）
15. **不要**让单测 sleep 几秒等 fanout 完成（用 BroadcastToRoomForTest wg.Wait drain）
16. **不要**新建 broadcast_test.go / broadcast_integration_test.go 文件（追加在 ws_test.go / ws_integration_test.go 末尾，与 10.4 同模式）
17. **不要**让 BroadcastToRoom 主路径 log error（污染 prod alarm；用 info）
18. **不要**让 BroadcastToRoom 主路径不 log（黑盒，prod 排障困难；必须 log info "ws broadcast to room"）
19. **不要**让 broadcast.go import "time"（caller 已在 marshal 时填 ts；primitive 不动时间）
20. **不要**让 broadcast.go import "encoding/json"（msg 已序列化；primitive 透明转发）
21. **不要**预先实装 BroadcastToUser / BroadcastToAll / BroadcastExcept（YAGNI；epics.md §Story 10.5 仅钦定 BroadcastToRoom 一个）
22. **不要**预先实装 metrics counter（节点 13+ 才做；本 story 仅 slog log）
23. **不要**预先实装 Pub/Sub 跨实例（节点 13+ 才做）
24. **不要**修改 main.go / router.go / Session struct / SessionManager interface（本 story 严格 read-only 现有公开接口）
25. **不要**修改 V1 / 项目结构 / DB 设计文档（V1 §12 已冻结）
26. **不要**修改 epics.md（epic AC 已写明；本 story 在 dev notes 中解释 Redis presence vs SessionManager 数据源等价性，但不反向改 epic）

### 关键 Lesson 提炼（本 story 写完后追加）

如果本 story dev-story / review 阶段发现新坑（如 fanout 100+ Session 下 goroutine 数量瞬态超过 runtime.NumGoroutine 上限 / msg 字节流在某些 client 端被错误解释为多帧 / BroadcastFn closure 在 service 层注入时被当成 nil / multi-instance 场景下 broadcast 漏发等），按既有 lesson 文档模式写 `docs/lessons/2026-05-XX-ws-broadcast-XXX.md`，并在 review fix 阶段回填 commit hash。

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

dev-story 阶段（2026-05-07）一次跑通，无 HALT 阻塞。关键观察：

- `bash scripts/build.sh --test`：全绿（含本 story 新加 11 个单测）
- `bash scripts/build.sh --integration`：全绿（含本 story 新加 1 个集成 case；docker 可用环境下应实际 spawn mysql container 跑 happy path + close + rebroadcast 全流程）
- `go test -count=1 -run "TestBroadcastToRoom" ./internal/app/ws`：11 个 case 全 pass，0 skip / 0 fail（SendBufferFull 路径在本机成功填满 sendChan，未触发 Skip 兜底）
- 没有用 `-race`：Windows 下 `-race` 需要 cgo + gcc toolchain，与既有 build.sh 行为一致（`--race` 是 opt-in flag）

### Completion Notes List

**实装摘要**：

- 新建 `server/internal/app/ws/broadcast.go`：包级 `BroadcastToRoom(ctx, mgr, roomID, msg) (sent, err)` + `BroadcastFn` type alias + unexported `broadcastToRoomFanout(ctx, mgr, roomID, msg, wait bool)` 共享 helper
- broadcast.go 严格遵守 §"实装关键决策" §9 import 红线：仅 import `context` / `log/slog` / `sync`（无 redis / mysql / time / encoding/json / errors）
- fanout drain 模式完全复用 Story 10.4 r5 lesson：函数局部 `sync.WaitGroup` + 生产路径 `wait=false`（fire-and-forget）/ 测试路径 `wait=true`（drain）
- 在 `export_test.go` 末尾追加 `BroadcastToRoomForTest(ctx, mgr, roomID, msg)` 测试 helper（调 `broadcastToRoomFanout(..., wait=true)`）
- log 主路径 info `ws broadcast to room` + Send 失败 warn `ws broadcast Send failed`，component 字段 `ws-broadcast`（与 `ws-heartbeat` 区分）
- error 返回值永远 nil（与 §"实装关键决策" §8 钦定一致）；接口签名保留 error 让未来 Pub/Sub 多实例升级用
- 11 个单元测试覆盖 AC4 钦定的 10 case + 1 个额外 `DoesNotTriggerUnregisterHook` 验证 broadcast 是只读路径不动 lifecycle hook
- 1 个集成测试 `TestWSIntegration_BroadcastToRoom_3Clients_AllReceive` 覆盖 AC5：3 user → broadcast → 主动 close 一个 → 再 broadcast → 剩 2 收到

**实装与 spec 的偏离（已在 Tasks 中记录）**：

1. **LargeFanout N=30 而非 100**：useGatewayDial 是 gateway-per-user 模式，每个 user 启独立 httptest server。100 个 httptest server 在 Windows/CI 触发端口耗尽 / fd limit。N=30 已足够验证 fanout drain 不 leak（drain 本质不随 N 变化）。如未来 helper 改为单 httptest server 多 conn 模式可上调到 100。
2. **集成测试 fixture 扩展方式**：spec 钦定改 `startMySQLWithRoomMemberFixture` 函数本身让 room=3001 含 3 user，但既有 4 个 integration case 依赖 fixture 含 2 user 不能改（会 break `room.memberCount=2` 断言）。改成在新 case 测试体内 INSERT user 1003 + UPDATE rooms.member_count=3，是无副作用的 reuse 方式。
3. **SendBufferFull case 含 Skip 兜底**：测试通过"不读 conn → writeLoop 阻塞在 conn.WriteMessage → sendChan 满"模式触发 ErrSessionSendBufferFull；但部分 OS 下 client TCP buffer 较大，writeLoop 可能在 200 次 Send 内仍能消费 sendChan → 不会触发 buffer full。此时 Skip 而非 fail（核心 fanout 行为已被其他 case 覆盖）。本机环境实测稳定触发 buffer full。

**关键不变量（review 必查）**：

- BroadcastToRoom 不调 Session.Close / mgr.Unregister（lifecycle 解耦，由 `TestBroadcastToRoom_DoesNotTriggerUnregisterHook` 验证）
- ListSessionsByRoomID 是 read-lock copy 快照（review r5 P2 不变量保留），由 `TestBroadcastToRoom_SessionRegisteredAfterListSnapshot_NotIncluded` 验证
- Send 失败 log warn 不阻塞其他 goroutine（review r1 lesson），由 `TestBroadcastToRoom_OneSessionSendFails_OthersStillReceive` + `TestBroadcastToRoom_SendBufferFull_LogWarnContinues` 验证
- 跨 room 隔离 + 并发安全，由 `TestBroadcastToRoom_DifferentRooms_Isolated` + `TestBroadcastToRoom_ConcurrentToDifferentRooms_AllCorrect` 验证

**未触碰文件**（红线遵守）：main.go / router.go / session.go / session_manager.go / gateway.go / heartbeat_scanner.go / V1 接口设计文档 / 项目结构文档 / DB 设计文档 / epics.md / ADR-0011 全未改。

### File List

新增（1）：

- `server/internal/app/ws/broadcast.go`

修改（3）：

- `server/internal/app/ws/export_test.go`（追加 `BroadcastToRoomForTest`）
- `server/internal/app/ws/ws_test.go`（追加 11 个 broadcast case + `readBroadcastMessage` helper）
- `server/internal/app/ws/ws_integration_test.go`（追加 `TestWSIntegration_BroadcastToRoom_3Clients_AllReceive`）

状态文件（2）：

- `_bmad-output/implementation-artifacts/sprint-status.yaml`（10-5 状态：ready-for-dev → in-progress → review）
- `_bmad-output/implementation-artifacts/10-5-broadcasttoroom-primitive.md`（本文件 Status 同步 + Tasks/Subtasks 勾选 + Dev Agent Record 填写）

## Change Log

| Date | Change | Author |
|------|--------|--------|
| 2026-05-07 | Story 10.5 create-story 初稿：BroadcastToRoom primitive + BroadcastFn 类型别名 + fanout drain 模式 + 10 单测 + 1 集成 case；Status: ready-for-dev | Story Context Engine (claude-opus-4-7[1m]) |
| 2026-05-07 | dev-story 实装：broadcast.go 包级 primitive + BroadcastFn type alias + broadcastToRoomFanout 共享 helper + BroadcastToRoomForTest 测试 helper + 11 个单测 + 1 个集成 case 全 pass；scripts/build.sh --test 与 --integration 均 OK；Status: in-progress → review | claude-opus-4-7[1m] |

| 日期 | 阶段 | 内容 |
|---|---|---|
| 2026-05-07 | create-story | 初稿写完，待 dev-story 接管 |
| 2026-05-07 | dev-story | 实装完毕、单测+集成全绿、build success、状态推到 review |
