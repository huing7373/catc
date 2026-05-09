---
date: 2026-05-10
source_review: codex review (epic-loop round 3) — /tmp/epic-loop-review-12-6-r3.md
story: 12-6-心跳维护
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-10 — WS heartbeat ping send 抛错必须强制走与 pong timeout 相同的 reconnect 路径，不能 silent return

## 背景

Story 12.6（WS 心跳维护）round 3 codex review。round 1 修了 transient close 前的 heartbeat state reset；round 2 修了 pong requestId 配对校验；本轮 codex 指出 heartbeat 第三条 race：`URLSessionWebSocketTask.send(.ping)` 抛错时，heartbeat task 仅 cleanup latch + return，**没**触发 reconnect。在 "locally broken socket"（send 失败但 receive() 仍 blocked，没观察到 close）这种场景下，client 会变成"无 heartbeat + 无 auto-reconnect" 的卡死状态，连接看起来仍 connected 但实际已废。这是 12.6 心跳特性的功能性 regression（因为 12.6 的整个目的就是检测 broken socket 然后触发 reconnect）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | heartbeat ping send 抛错没 force reconnect, 仅 cleanup latch return → broken socket 静默卡死 | P1 (high) | error-handling | fix | `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:1278-1289` |

## Lesson 1: heartbeat detector 的"send error"分支必须等同于"timeout 分支"，不能依赖另一条路径自然 catch

- **Severity**: high (P1)
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:1278-1289`（heartbeat task 内 send(.ping) 的 catch 块）

### 症状（Symptom）

heartbeat task 走"创建 pong latch → send(.ping) → awaitPongOrTimeout"三步。`send(.ping)` 抛错时（catch 路径）：

- 旧实装：在 catch 里持锁清 `pendingPongContinuation` / `pendingPongRequestId` → return；
- 注释里写的假设："send 失败 → underlying task 已死 / notConnected → receive-loop 会接管 catch 走 reconnect"；
- 但这条假设**不成立** —— `URLSessionWebSocketTask.send` 的失败与 `receive()` 的失败是**异步独立**的两条 channel：send pipeline 拒绝写入不一定立即让 receive() 抛错（OS-level socket pipeline 可能 send 半边坏 / receive 半边仍 blocked 等待 frame）；
- 更糟："locally broken socket"（send fail 但 receive 不 fail）下，nothing schedules reconnect → heartbeat task 退出 → 没有下一轮 ping → client 永久卡死，但仍认为自己 connected。

### 根因（Root cause）

heartbeat detector 设计中存在两条独立的 failure detection 路径：(A) ping send error；(B) pong receive timeout。开发者实装时下意识假设了 "(A) 发生时 (B) 路径会自然 catch 到，所以 (A) 不需要主动触发 reconnect"，把 (A) 路径 silent return —— 但这是把 send pipeline 与 receive pipeline 当成同步耦合的 channel 来推理。

实际 OS-level socket / `URLSessionWebSocketTask`：**send 端与 receive 端是独立 pipeline**，可以一边坏一边好（NIC 半双工故障、send buffer 锁死但 keepalive 包还能流入、跨设备路由不对称……）。任何"我假设另一条 path 会接管"都必须有反向证明（"另一条路径在所有 send-fail 子场景下也必失败"），否则就是漏掉一个 silent failure mode。

heartbeat 协议的契约本应是："只要 ping 没成功 round-trip 就触发 reconnect" —— ping 没 round-trip 包含两类 ① send 没成功；② send 成功但 pong 没回。这两类必须**对称对称对称**地走 reconnect path，没有"另一条会接管"的偷懒。

### 修复（Fix）

在 send catch 里，cleanup latch 之后**强制**调 `cancelUnderlyingTaskWithGoingAwayIfCurrent(mySession:)` —— 与 pong timeout 路径完全相同。`cancel(.goingAway)` (rawValue=1001) 走 receive-loop transient 分类 → schedule reconnect。

before（silent return + 错误假设）：
```swift
do {
    try await strongSelf.send(.ping(requestId: "ping_\(seq)"))
} catch {
    // send 失败：underlying task 已死 / notConnected —— receive-loop 会接管 catch 走 reconnect.
    // 本 heartbeat task 退出即可；不主动 cancel underlying（避免双触发，让 receive-loop 自然 catch）
    strongSelf.lock.lock()
    strongSelf.pendingPongContinuation?.finish()
    strongSelf.pendingPongContinuation = nil
    strongSelf.pendingPongRequestId = nil
    strongSelf.lock.unlock()
    return
}
```

after（force reconnect, 与 pong timeout 同路径）：
```swift
do {
    try await strongSelf.send(.ping(requestId: "ping_\(seq)"))
} catch {
    os_log(.info, log: WebSocketClientImpl.logger,
           "heartbeat ping send failed → cancel underlying task with .goingAway (1001) → receive-loop catch transient → schedule reconnect")
    // 先做 latch cleanup（防 leak）
    strongSelf.lock.lock()
    strongSelf.pendingPongContinuation?.finish()
    strongSelf.pendingPongContinuation = nil
    strongSelf.pendingPongRequestId = nil
    strongSelf.lock.unlock()
    // 强制走 reconnect 路径（与 pong timeout 同一路径）
    strongSelf.cancelUnderlyingTaskWithGoingAwayIfCurrent(mySession: mySession)
    return
}
```

回归测试 `test_heartbeat_pingSendThrows_cancelsUnderlyingWithGoingAwayTriggersReconnect`：
- 第一个 fake task 注入 snapshot 解 connect latch + `sendThrowsError = URLError(.notConnectedToInternet)`；
- `blockReceiveForever = true` 模拟 "receive 仍 blocked 没观察到 close"；
- `stubbedCloseCode = .goingAway` 让 `cancel(.goingAway)` 后 receive-loop 抛错时拿到 1001 走 transient；
- 第二个 task 注入 snapshot 让 reconnect attempt 跑通；
- 断言 connection states = [.connected, .reconnecting(1), .connected]，`firstTask.lastCancelCloseCode == .goingAway`，`makeTaskCallCount == 2`；
- 修复前：send 抛错后 silent return → reconnect 永不触发 → state stream 永远不会 emit `.reconnecting(1)` → `collectConnectionStates(count: 3)` timeout fail。

顺带改动：FakeWebSocketTaskHandle 加 `sendThrowsError: Error?` 字段，let `send(_:)` 在持锁 append 后按需 throw（先 append 再 throw 让测试也能验证 ping 帧已 wired through 但被网络层拒绝的语义）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在实装 "client 端 health-check / heartbeat 检测器"时，**所有可能的 failure detection branch（send-error / response-timeout / parse-error / liveness-watchdog 等）必须各自独立触发 reconnect / failover**，不允许任何分支假设"另一条 branch 会接管"，因为底层 send-pipeline 与 receive-pipeline 是**异步独立**的。
>
> **展开**：
> - heartbeat / health-check 的契约是"只要这一轮 round-trip 没成功就触发 reconnect"；任何让 round-trip 失败的子路径都必须 force reconnect，不能 silent return / silent retry；
> - 写"send 失败 catch 块"时，**禁止**写"receive-loop 会自然 catch"这种偷懒注释 —— socket send 端与 receive 端在 OS 层是独立 pipeline，send pipeline 拒绝写入不保证 receive pipeline 立即也抛错（半双工故障、send buffer 锁死但 keepalive 包还能流入等）；
> - 多分支 detector 的对称性是**结构性强约束**：写 catch 块时拿 timeout 路径作为标尺，问"这个 catch 必须做和 timeout 完全一样的事吗"，答案不是"是"就是 silent failure mode；
> - cancel underlying socket 用 transient close code（如 `.goingAway` / 1001）让 receive-loop 走既有 reconnect 状态机，避免在 catch 里写一份并行的 reconnect 调度逻辑（DRY + 减少状态机分裂）；
> - cleanup（清 latch / 清 pending state）和 reconnect trigger 是**两件独立事**：先 cleanup 再 trigger，不能"省一步只 cleanup 不 trigger"；
> - **反例 1（本次踩坑）**：`catch { cleanup latch; return /* receive-loop will catch */ }` —— "另一路径会接管" 是未经证明的假设；只要存在一个反例场景（locally broken socket），detector 就漏检；
> - **反例 2**：在 send catch 里**不**做 cleanup 直接 return —— latch 泄漏 + 后续 reconnect 启动新 heartbeat 时 stale latch 干扰新 in-flight ping；
> - **反例 3**：在 send catch 里调 public `disconnect()` 而非 transient cancel —— `disconnect()` 走 1000 normal close = terminal，不重连；heartbeat detector 必须用 transient close code 触发 reconnect，不是终止 client。
>
> **额外提示**：detector 的"对称性 audit" —— 把 detector 的所有 catch / timeout / error branch 排成一张表，每行回答两个问题 ① 我做了 cleanup 吗？② 我触发了 reconnect 吗？任何"否-否"或"是-否"的行都是潜在 silent failure mode，必须重新审视。理想形态：所有行都"是-是"。

---

## Meta: 本次 review 的宏观教训

Story 12.6 三轮 review 修了 heartbeat 的三个独立 race / failure mode：r1（reconnect 前 state 没 reset）、r2（pong requestId 没配对校验）、r3（ping send error 没 force reconnect）。三者都属于"detector 漏掉一个 failure mode"——r1 是状态残留、r2 是协议层 correlation 缺失、r3 是 detection branch 缺失。

heartbeat 实装的常见思维漏洞：把 heartbeat 当成"timer + ping + 比较 last pong" 的简单状态机，忽视它实际是一个**对 socket health 的 detector**，而 detector 的正确性依赖**所有 failure mode 都被映射到一个明确的"trigger reconnect"动作**。每多一个 silent / 偷懒分支，detector 就漏掉一类故障。后续实装类似检测器（liveness probe / circuit breaker / RPC retry executor）时，应当先把所有可能的 failure mode 列表（send-fail / response-timeout / response-malformed / response-mismatch / cancellation / shutdown），逐个分配 detection branch + reconnect-trigger 动作，**不允许**任何 branch silent return。
