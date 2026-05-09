---
date: 2026-05-10
source_review: codex review (epic-loop round 2) — /tmp/epic-loop-review-12-6-r2.md
story: 12-6-心跳维护
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-10 — WS heartbeat .pong 必须按 requestId 配对当前 in-flight ping，不能无条件 ack 任意 pong

## 背景

Story 12.6（WS 心跳维护）round 2 codex review。round 1 修复了"transient close 触发自动 reconnect 前必须显式 reset heartbeat 状态"的 race；本轮 codex 进一步指出：heartbeat receive 路径**无条件**接受任何 `.pong` 当 in-flight ping 的 ack —— 违反 V1 §12.2 钦定的 `pong.requestId` echo 协议，可能让 server 推送的 stale pong（旧 ping 的迟到 / 重复 pong）错误 ack 当前 in-flight ping，结果"当前 ping 实际未被 ack"被掩盖，推迟一整个 heartbeat interval 才检测到 reconnect 需要。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | heartbeat .pong 不校验 requestId, 任意 pong 都 ack 当前 in-flight ping | P2 (medium) | error-handling | fix | `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:626-627` |

## Lesson 1: 协议钦定 request-response correlation 必须在 client 端真校验，不能"按 case 分发就当 ack"

- **Severity**: medium (P2)
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:626-627`（receive-loop 的 `.pong` 分发处）

### 症状（Symptom）

WS receive-loop 的 `.string`/`.data` 分支在 decode 出 `WSMessage.pong(requestId:)` 时调 `notifyPongReceivedIfCurrent(mySession:)`，notify 内部直接 `pendingPongContinuation?.yield(())`。这意味着：

- 任意 server 推过来的 pong frame，无论 `requestId` 是什么，都会 yield 当前 latch；
- heartbeat task 的 `awaitPongOrTimeout` 收到 yield → return false（视为 pong 已到达）→ continue 下一轮 sleep；
- 真实场景：server 因网络抖动 / 自身排队延迟，可能在 client 已经 timeout 一次 ping_N 重发 ping_N+1 之后，才把 ping_N 的 pong 推过来；这条 stale pong 会错误 ack ping_N+1 的 latch；
- 结果：ping_N+1 实际未被 server 处理（ping_N+1 的 pong 还没回），但 client 误以为已 ack；
- 整整一个 heartbeat interval（30s）后才会发出 ping_N+2，那时如果连接真有问题，已经迟检测了 30s。

### 根因（Root cause）

实装 12.6 心跳路径时，把"WSMessage.pong case 出现"等价于"ack"，把 ping/pong 的请求-响应配对责任**外包**给了协议层 enum decode（"反正只有 .pong 会触发 notify"）。但 enum 只能告诉你**type 是 pong**，无法告诉你**对应哪一次 ping**。

V1 §12.2 钦定 `pong.requestId` echo 对应 ping 的 `requestId` —— 这是协议层为了 request-response correlation 显式设计的字段；client 端必须读它、必须配对、必须 mismatch 时 silent drop。把 case dispatch 误当成 correlation，是 client 网络层的常见反模式。

类似反模式的家族：
- "HTTP response 200 就当成功"（实际还要校验 body 里的 status）
- "WebSocket ack frame 收到就当 in-flight 请求被处理"（实际要看 ack frame 里的 requestId / sequence）
- "MQTT PUBACK 收到就当当前 publish 完成"（实际要按 packet identifier 配对）

### 修复（Fix）

在 client 引入 `pendingPongRequestId: String?` 字段，与 `pendingPongContinuation` 严格同生命周期：

1. heartbeat task 在持锁内创建新 latch + 写入 `pendingPongRequestId = "ping_<seq>"`（与 ping 真实 requestId 一致）—— 同一个 lock 段保证"新 ping 切换"对 receive-loop notify 是原子可见的；
2. receive-loop decode 出 `.pong(requestId:)` 调 `notifyPongReceivedIfCurrent(requestId:mySession:)`（签名加 requestId 参数）；
3. notify 内部在持锁内校验 `incomingRequestId == pendingPongRequestId`，不匹配 → silent drop + log debug，**不**调 `cont.yield(())`；
4. 任何"清掉 latch"路径（heartbeat task 内的本轮结束、`cancelHeartbeatStateForReconnectIfCurrent`、`disconnect`、`prepareForReconnect`、`connect`）都要把 `pendingPongRequestId` 一起清 nil。

before（无条件 ack）：
```swift
private func notifyPongReceivedIfCurrent(mySession: Int) {
    lock.lock(); guard sessionGeneration == mySession else { lock.unlock(); return }
    let cont = pendingPongContinuation
    lock.unlock()
    cont?.yield(())  // 任何 pong 都进 latch
}
```

after（按 requestId 配对）：
```swift
private func notifyPongReceivedIfCurrent(requestId incomingRequestId: String, mySession: Int) {
    lock.lock(); guard sessionGeneration == mySession else { lock.unlock(); return }
    guard let expected = pendingPongRequestId, expected == incomingRequestId else {
        lock.unlock(); /* log debug stale/duplicated pong */ return
    }
    let cont = pendingPongContinuation
    lock.unlock()
    cont?.yield(())
}
```

回归测试 `test_heartbeat_stalePongMismatchedRequestIdDoesNotAckInflightPing_round2_P2`：
- scriptedFrames = `[snapshot, stale_pong("stale_id")]`，frame[1] 注入 120ms delay
- heartbeatInterval=50ms / pongTimeout=300ms → ping_1 在 T0+50 已发出（pendingPongRequestId="ping_1"），T0+120 stale pong 到达 → silent drop → T0+350 pongTimeout fire → cancel(.goingAway, 1001 transient)
- 断言 `lastCancelCloseCode == .goingAway` —— 修复前因 stale pong 错误 ack ping_1，timeout 不会 fire，断言失败

顺带改动：fake handle 加 `frameDelaysSec: [TimeInterval]?`，让回归测试能精准在"ping 已发出之后"才推送 stale pong（精准重现协议层 race 时序）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在实装"client 端 request-response 协议（heartbeat / ack / RPC over WS）"时，**必须**用协议钦定的 `requestId` / `sequenceNumber` / `correlationId` 字段在 client 持锁内做 in-flight 配对校验，不能把"enum case 匹配"等价于"当前请求被 ack"。
>
> **展开**：
> - 任何"client 发请求 → server 回 ack" 类的协议（WS heartbeat / RPC / pubsub ack），必须在 client 端写一个 `pendingRequestId: String?` 字段，与"等待 ack 的 latch"同生命周期；
> - 发请求时，在持锁段同时写 `pendingRequestId = <本次发的 id>` + 创建新 latch；
> - 收到 ack frame 时，先按 `requestId` 配对再 yield latch；不匹配 silent drop + log debug（不要 throw / 不要 finish stream —— stale ack 是协议正常现象）；
> - 任何"清 latch"路径（终态切换 / reconnect 状态机 reset / disconnect）必须把 `pendingRequestId` 一起清 nil；
> - **反例 1（本次踩坑）**：`if case .pong = message { notifyPong() }` —— 用 enum case 当 dispatch key 但**不读** requestId，等于丢掉协议层显式设计的 correlation 字段；
> - **反例 2**：在 viewModel / use-case 层做 correlation —— correlation 必须发生在最贴近 receive 的 client 内部（持锁），下游层永远不该看到"请求与响应错位"；
> - **反例 3**：用 `seqNumber` 但只取最新值（"反正最新的总是对的"）—— 这等价于无 correlation，stale ack 只要在新请求发出后到达就能错配。
>
> **额外提示**：判断"是否需要 correlation"的简单启发式 —— 协议文档里出现"echo"/"correspond to"/"matching" 等字眼描述某个字段时，client 必有显式校验责任；不能让 server 推什么 ack client 都信。

---

## Meta: 本次 review 的宏观教训

Story 12.6 已在 round 1 修了一个 race（旧 heartbeat state 没在 reconnect 前 reset）；round 2 的 P2 揭示更深的协议合规问题：**仅靠 case dispatch + AsyncStream latch 实现的 ack 机制天生丢掉 correlation**。AsyncStream / Continuation 是单向通知通道，协议层若需要 N 个 in-flight 请求各等各的 ack，必须用 `[requestId: latch]` 字典 + 配对消费；本 story 简化为单 in-flight ping（heartbeat 是 strict serial 模型，每轮等上一轮结束才发下一轮），所以单 `pendingPongRequestId + pendingPongContinuation` 已足够，但仍必须在 client 持锁配对校验，不能"反正只有一个 in-flight，pong 来了就当 ack"。
