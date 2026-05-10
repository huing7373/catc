---
date: 2026-05-10
source_review: codex review (round 6) — /tmp/epic-loop-review-12-6-r6.md
story: 12-6-心跳维护
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-10 — terminal post-handshake close 必须与 transient 分支对齐 cancel heartbeat 子系统（12-6 r6）

## 背景

Story 12.6 第 6 轮 codex review。前 5 轮已修复：transient reconnect 前 reset heartbeat 状态（r1）、按 requestId 配对 pong（r2）、ping send 抛错强制 reconnect（r3）、lock-unlock window pre-send 重校验（r4）、send catch 用 task identity 守护（r5）。本轮（r6）codex 找出第 6 条边角：post-handshake **terminal** close 分支只把 client 切到 `.disconnected`，但 `heartbeatTask` + `pendingPongContinuation` 仍 alive —— 与 transient 分支不一致.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | terminal post-handshake close 不清理 heartbeat 子系统，留 leak 直到 task 自己退出 | P2 (medium) | architecture | fix | `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:764-778` |

修了 1 / defer 0 / wontfix 0.

## Lesson 1: post-handshake terminal close 必须 cancel heartbeat 子系统（与 transient 分支对齐）

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:764-778`（receive-loop catch 的 `case .terminal` 分支）

### 症状（Symptom）

post-handshake 阶段 server close 用 terminal close code（4001 / 4002 / 4003 / 4004 / 4006 / 4007 / 1000 / 未知），receive-loop 的 catch 分类到 `case .terminal`：

```swift
case .terminal(let code):
    strongSelf.emitConnectionStateIfCurrent(.disconnected, mySession: mySession)
    strongSelf.lock.lock()
    strongSelf.currentRoomId = nil
    strongSelf.reconnectAttempt = 0
    strongSelf.lock.unlock()
    leaveStreamOpen = false  // → finish stream
    return
```

但 `heartbeatTask` 和 `pendingPongContinuation` 没被 cancel，留下：

- 旧 heartbeat task 仍在 `Task.sleep(heartbeatInterval)` 等下一轮（默认 30s）
- 或正在 `awaitPongOrTimeout` 等 pongTimeout fire（默认 5s）
- pong timer fire 后仍走 `cancelUnderlyingTaskWithGoingAwayIfCurrent` 路径 → generation 校验通过（terminal 不翻 sessionGeneration）→ post-disconnect ping / `.goingAway` cancel 行为
- task 直到自己自然退出才释放（最坏 30s）

stream 已 finish、外层 vm 已写 `wsState = .disconnected`，但 client 内部 heartbeat 子系统仍在 background 折腾 —— 行为漂移 + 资源泄漏 + log 噪音.

对比 transient 分支（r1 修复后）：

```swift
case .transient(let code):
    strongSelf.cancelHeartbeatStateForReconnectIfCurrent(mySession: mySession)  // ← 已 reset
    strongSelf.scheduleReconnectIfCurrent(mySession: mySession)
    leaveStreamOpen = true
    return
```

terminal 路径漏了等价 cleanup，纯路径完整性遗漏.

### 根因（Root cause）

子系统 lifecycle 的"对称性"思维漏洞：

1. **transient / terminal 分支同一 catch handler 但 cleanup 责任分布不对称**。transient 走 reconnect 必须先 reset（避免旧 heartbeat 错杀新 socket），所以 r1 修了；terminal 没 reconnect，看起来"没有错杀对象"，就漏掉了 cleanup。但 cleanup 的目的不止防错杀 —— 还包括"释放不再相关的 background work"。两个分支都应**至少**清理 heartbeat task，区别仅在于"清完是否重启"。

2. **helper 名字 `cancelHeartbeatStateForReconnectIfCurrent` 误导语义**。"ForReconnect" 后缀让调用者误以为 helper 仅适用 reconnect 路径，于是 terminal 分支的实现者没想到去调它。但 helper 内部逻辑（cancel 旧 task + finish pong cont + 清字段）对 terminal 路径完全适用 —— 名字遗留自最初的 r1 callsite，没随复用面扩展更新。

3. **测试盲区**：r1 / r4 / r5 都覆盖 transient + reconnect 路径下 heartbeat 子系统的清理时序；terminal 路径的 happy path 测试（如 `test_reconnect_terminalClose4001_emitsDisconnectedFinishesStream`）只断言 `.disconnected` emit + `makeTaskCallCount == 1`，没断言 heartbeat task 是否仍发 ping。**断言"什么不该发生"**（heartbeat post-disconnect 不该再发 ping）需要主动构造时间窗 + 前后计数对比，比断言"什么发生了"更容易被遗漏.

### 修复（Fix）

最小改动：在 terminal 分支也调一次 `cancelHeartbeatStateForReconnectIfCurrent(mySession:)`，复用 transient 分支已经验证过的 cleanup 逻辑.

```swift
case .terminal(let code):
    // fix-review round 6 P2：terminal 分支也必须 cancel heartbeat 子系统 ——
    // 与 transient 分支对齐. terminal 不会 reconnect，cleanup 是最终态.
    strongSelf.cancelHeartbeatStateForReconnectIfCurrent(mySession: mySession)
    strongSelf.emitConnectionStateIfCurrent(.disconnected, mySession: mySession)
    strongSelf.lock.lock()
    strongSelf.currentRoomId = nil
    strongSelf.reconnectAttempt = 0
    strongSelf.lock.unlock()
    leaveStreamOpen = false
    return
```

helper 不重命名（rename 会触及多个 doc-comment 和 test 注释，违反"最小修复"纪律）；改在 helper docstring 的 `**callsite**` 列表里追加第 3 条，明确"helper 名字虽含 ForReconnect 是历史包袱，对 terminal final cleanup 路径同样适用".

回归测试 `test_heartbeat_terminalCloseStopsHeartbeatTaskNoMorePing_round6_P2`：
- heartbeatInterval = 50ms，发 snapshot 启 heartbeat，让 heartbeat 至少跑一轮 ping 形成前置 baseline (`countBefore`)
- 主动 `cancel(with: .init(rawValue: 4001)!)` 触发 terminal 分支
- 等 500ms（=10 个 heartbeatInterval）→ 断言 `sentMessages.count == countBefore`（修复未生效则会增长 ~10）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 receive-loop / 状态机 catch 中处理 **terminal vs transient 双分支** 时，**必须**确认两个分支对子系统资源（heartbeat / timer / continuation / background task）做**等价的 cleanup**；区别只能在"是否重启"，不能在"是否清理"。
>
> **展开**：
> - **对称性 checklist**：每写 / 修改任何"close handler 双分支"（terminal / transient、success / failure、normal / error）时，列出该 handler 路径上**所有 background work**（task 引用、continuation、scheduled timer、pending latch、observer subscription），逐项问："这条 work 在这个分支后还应该跑吗？" 不该跑就 cancel/finish；该跑但需要重启就 cancel + 重新 launch.
> - **复用 cleanup helper 时不要被名字限制**：helper 名字（`...ForReconnect`、`...OnDisconnect`）反映的是它最初被设计的 callsite，不是 helper 内部逻辑的边界。看 helper 实现而不是名字判断能否复用；必要时扩展 docstring 的 callsite 列表说明新用法 —— **不**为了一个 callsite 改名字（污染面太大）.
> - **背景任务 leak 的可观察性**：unit test 必须断言"背景 task 在 terminal 路径后**不再产生副作用**"（例如对比 send/log/state-write count 在 N 个 interval 前后是否相等），不能只断言 user-visible state（`.disconnected` emit 已发出）。后者掩盖 leaked-loop 类 bug.
> - **反例（避免再犯）**：
>   - terminal 分支只写 `emitConnectionStateIfCurrent(.disconnected, ...)` + 清 currentRoomId / reconnectAttempt → **错**（heartbeatTask 还活着）；正确做法：先调 helper cancel heartbeat，再 emit + 清字段.
>   - 看到 helper 名字 `cancelXxxForReconnectIfCurrent` 就以为"reconnect 路径专用"不调它 → **错**；要看 helper body 的实际逻辑（cancel + finish + clear）是否覆盖当前路径需要的 cleanup 范围，而不是看名字推断.
>   - 写 terminal close 测试时只断言 `lastConnectionState == .disconnected` 与 `makeTaskCallCount == 1` → **不充分**；应再加一条"terminal 后等 N×interval，sentMessages.count 不增长"的断言挡住未来漏 cleanup 类回归.
