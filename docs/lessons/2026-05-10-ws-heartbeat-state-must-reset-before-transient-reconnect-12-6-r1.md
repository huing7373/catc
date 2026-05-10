---
date: 2026-05-10
source_review: codex review (epic-loop round 1) — file: /tmp/epic-loop-review-12-6-r1.md
story: 12-6-心跳维护
commit: 7af762a
lesson_count: 1
---

# Review Lessons — 2026-05-10 — 心跳子系统必须在 transient close 触发自动 reconnect 前显式 reset（否则旧 pong timer 错杀新 socket）

## 背景

Story 12.6 落地 WebSocket heartbeat（30s ping / 5s pong timeout / 超时 cancel(.goingAway) 让 12.5 reconnect 状态机接管）。round 1 codex review 发现一条 P1：**heartbeat 仅用 sessionGeneration 守护不够**，因为自动 reconnect（contract 5：透明续接）**不**翻 sessionGeneration —— 旧 heartbeat task 在 reconnect 后仍认为自己 "current"，继续等 pong；timer fire 时新 underlyingTask 已 install，旧 task 调 `cancelUnderlyingTaskWithGoingAwayIfCurrent` 时 `mySession == sessionGeneration` 通过 → cancel 新 socket → 一次 recoverable disconnect 演化成连续 reconnect 失败。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 旧 heartbeat 的 pong timeout 在 transient reconnect 后会 cancel 新 underlyingTask（race） | P1 (high) | architecture | fix | `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift` line 727-741 (transient close 路径) + line 1020-1029 (attemptReconnect 入口) + 新增 helper `cancelHeartbeatStateForReconnectIfCurrent` line 1395 |

## Lesson 1: heartbeat 子系统的"代隔离"必须覆盖 reconnect 的非翻代路径

- **Severity**: P1 (high)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:1281-1285`（pong timeout 调用 cancelUnderlyingTaskWithGoingAwayIfCurrent 处）

### 症状（Symptom）

transient close（1001/1006/1011/4005）触发自动 reconnect 时，若有 heartbeat task 处于"已发 ping、等 pong"状态：

1. T0: heartbeat 发 ping (firstTask) → 进 awaitPongOrTimeout（pongTimeout 默认 5s）
2. T1: 服务器/网络 transient close → receive-loop catch → schedule reconnect
3. T2: attemptReconnect → connectInternal → install 新 underlyingTask（secondTask）
4. T3: 旧 heartbeat 的 pong timer fire → cancelUnderlyingTaskWithGoingAwayIfCurrent
5. T4: 该函数内 `sessionGeneration == mySession` guard **通过**（reconnect 不翻 gen）→ 取出 underlyingTask（此刻 = secondTask）→ cancel(.goingAway, secondTask)
6. T5: secondTask 被错杀 → 又一次 transient close → 又一次 reconnect ……

最终一次 recoverable disconnect 演化成连续 5 次 reconnect 失败（5 = backoffSequence 上限），caller 拿到 `.disconnected` 切 terminal，但其实底层网络早已恢复。

### 根因（Root cause）

**`sessionGeneration` 是为"connect / disconnect / prepareForReconnect"这三个 caller-driven 主动入口设计的代标识**：每次入口翻 +1，所有 sub-task 用 captured mySession 校验 stale。但 Story 12.5 的自动 reconnect（contract 5：透明续接）**故意不翻 sessionGeneration** —— 因为 vm 视角看，session 是同一个（同一 stream、同一 currentRoomId、同一 reconnectAttempt 计数器），仅 underlyingTask 换了。

heartbeat task 的生命周期与 underlyingTask **本应一一对应**（每次新 underlyingTask install 应启动新 heartbeat task），但 12.6 落地时 heartbeat 错绑到了 sessionGeneration 上：reconnect 不翻 gen → 旧 heartbeat 不被 stale-drop → 旧 pong timer 继续 fire → 错杀新 underlyingTask。

设计文档（`12-6-心跳维护.md` line 754）当时甚至明文写过 "heartbeat 仅与 sessionGeneration 关联，无 streamGeneration 校验需求" —— 此结论是错的，因为它假设了"sessionGeneration 翻新覆盖所有 heartbeat 应失效的场景"，**但实际上自动 reconnect 既不翻 sessionGeneration，也不翻 streamGeneration**（contract 5），所以两个 generation 字段都不能 invalidate 旧 heartbeat。

更根本的概念漏洞：**generation 字段是为了 invalidate stale 写者**；要 invalidate 一类 task，必须确保翻 gen 的入口**完全覆盖**该类 task 的生命周期边界。heartbeat 的生命周期边界 = "underlyingTask 生命周期"，而不是 sessionGeneration / streamGeneration 任何一个。

### 修复（Fix）

引入"显式 reset 接缝"而非新增第三个 generation 字段，因为 heartbeat 的 lifecycle owner 是 underlyingTask，**直接由 underlyingTask 的换装入口同步 reset 即可**：

1. 新增 helper `cancelHeartbeatStateForReconnectIfCurrent(mySession:)`（generation-gated）：
   - cancel `heartbeatTask` + finish `pendingPongContinuation` + 清空两个字段
   - generation gate 防御 stale receive-loop 的 catch path 在新 session swap 后才跑、把新 session 的 heartbeat 也清掉
2. 在两处 callsite 同步调用：
   - **主路径**：receive-loop catch 的 transient 分支，`scheduleReconnectIfCurrent` 之前 → 旧 socket close 一确定就立即 reset
   - **防御性双保险**：`attemptReconnect` 进入 `connectInternal` 之前 → 覆盖 receive-loop 在 firstFrame 之前就 catch 等极端时序
3. 新连接成功后 receive-loop 在 firstFrame 处自动 `startHeartbeatTask`，捕获最新 sessionGeneration，启动全新的 heartbeat 循环

**不**翻 sessionGeneration —— 因为 reconnect 必须保持 session 透明续接；整个修复仅限 heartbeat 子系统内部 reset。

```diff
+// receive-loop catch transient 分支（新 socket install 前）
+strongSelf.cancelHeartbeatStateForReconnectIfCurrent(mySession: mySession)
 strongSelf.scheduleReconnectIfCurrent(mySession: mySession)

+// attemptReconnect 入口（防御性双保险）
+cancelHeartbeatStateForReconnectIfCurrent(mySession: mySession)
 try await connectInternal(roomId: roomId, isReconnect: true, attemptNumber: attempt)
```

回归测试 `test_heartbeat_oldPongTimeoutDoesNotCancelInflightReconnectSocket_round1_P1`：
- 时序设计 pongTimeout=500ms / heartbeatInterval=50ms / cancel firstTask=T0+150ms / assert=T0+700ms
- 上限 ~T0+790ms 是 secondTask 自己 heartbeat 的合法 timeout 边界，断言时间挤进 T0+550 ~ T0+790 这个 240ms 窗口

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**给一类 launched task 加 generation gate 时**，**必须**先确认"翻 gen 的入口集合完全覆盖该 task 的生命周期边界"——否则 generation 字段提供假象的隔离。

> **展开**：
> - **检查清单**：列出所有 "应让该 task 失效" 的入口（包括外部 caller-driven 与内部状态机自动触发）；逐一确认每个入口都翻 gen。**只要漏一个**，generation gate 就有假象。
> - **找绑定层级**：task 的"自然 lifecycle owner"是哪个（连接、流、会话、handshake、…）。错绑会让 generation 字段在错误粒度上翻，导致"不该失效时失效"或"该失效时不失效"。
> - **优先选 lifecycle owner 直接 reset，而非引入新 generation 字段**：当 task 的 lifecycle 与既有 generation 不一致，引入新字段（如 heartbeatGeneration）虽可解决，但成本是又多一个状态字段 + 多套 capture/check 模板。如果 lifecycle owner 已有显式 install/teardown 接缝（本案：transient close → schedule reconnect），直接在那处显式 cancel + reset 子系统状态更轻量，且语义更清晰。
> - **Story 12 系列的特殊约束**：自动 reconnect（V1 §12 contract 5：透明续接）**不**翻 sessionGeneration / streamGeneration —— 任何"per-underlyingTask"的 task（不只是 heartbeat，还包括未来可能的 ping latency tracker / inactivity watchdog 等）都不能仅靠这两个 generation 字段隔离 stale。
> - **反例**：写 12.6 落地时假设"heartbeat 仅与 sessionGeneration 关联"就够了，并明文写进 story doc line 754。这条结论建立在对"sessionGeneration 翻新覆盖所有 heartbeat 应失效场景"的隐含假设上 —— 而这个假设在 review round 1 暴露为错。**未来写设计文档时**：generation 与子系统绑定的论证必须**列举不翻 gen 的路径**，明示"是否覆盖该子系统的 invalidation 需求"。
> - **反例**：解决"task 错杀新资源"问题时直接引入新 generation 字段（heartbeatGeneration / underlyingTaskGeneration），导致：(a) 又多一个 capture/check 样板代码；(b) 翻 gen 的入口集合更难维护一致；(c) lifecycle 边界还是隐式的。优先选"在 lifecycle owner 的 install/teardown 接缝处显式 reset 子系统"。

> **测试模式**：generation-related race 的回归测试必须**精确控制时序**才能在不修复时稳定失败。
> - 关键时间点：旧资源失效时间 T_stale（如 pong timer fire = ping_time + pongTimeout）、新资源 install 时间 T_install（如 secondTask install）、新资源自身可能产生噪音的时间 T_noise（如 secondTask 自己 heartbeat 超时）
> - 断言时间窗口 = (T_stale, T_noise) 之间，留 ≥10% 余量
> - 必要时调小 pongTimeout/heartbeatInterval 让窗口落在合理 sleep 时长（本案 240ms 窗口 / 700ms 总测试时长）
