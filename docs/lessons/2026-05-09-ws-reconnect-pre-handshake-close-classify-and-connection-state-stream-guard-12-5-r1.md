---
date: 2026-05-09
source_review: codex review --uncommitted（/tmp/epic-loop-review-12-5-r1.md，round 1）
story: 12-5-自动重连
commit: 7898ade
lesson_count: 2
---

# Review Lessons — 2026-05-09 — reconnect pre-handshake terminal close 必须分类 + connection-state 事件必须 streamRoomId 守护

## 背景

Story 12.5 自动重连状态机 round 1 review。codex 找到 2 条与"按 close code 分类决策"和"WS event ↔ specific stream 绑定"相关的状态机 race / 误分类问题：reconnect catch 分支无条件 retry 让 4001 token 过期类 terminal close 被白白消耗 5 次 backoff（破坏 caller 的 re-auth handling 触发点）；vm 收到 `.connectionStateChanged` 事件无条件 apply wsState（推翻 dev 阶段开放问题 §6 的"不守护"决定）让 A→B 切换 / leave-rejoin 时旧 stream 投递的 stale state 覆盖当前 room 的 status banner。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | reconnect catch 必须按 close code 分类（terminal stop / transient retry） | high | error-handling | fix | `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:599-688`（attemptReconnect + startReceiveLoop pre-handshake catch） |
| 2 | `.connectionStateChanged` 必须用 streamRoomId 守护防 cross-room race | medium | architecture | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:385-413` |

## Lesson 1: reconnect catch 必须按 close code 分类（不能无条件 retry）

- **Severity**: high
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:623-634`（旧 attemptReconnect catch）

### 症状（Symptom）

reconnect attempt 在第一帧到达之前被 server reject（如 4001 token 过期 / 4003 / 4004），`startReceiveLoop` 在 pre-handshake catch 路径直接 return（让 connectGate throw 一个 `connectionFailed` 占位 error）；`attemptReconnect` catch 拿到这个 error 后**无条件**调 `scheduleReconnect()` → 把 4001 这种 V1 §12.1 钦定的 terminal close 当 transient 重试，白白消耗 5 次 backoff attempts，永远不触发 caller 的 re-auth / room-error handling path。post-handshake terminal close 走 `startReceiveLoop` 的 close code classifier 是 OK 的；问题只发生在 **pre-handshake** 路径上 —— `firstFrameReceived = false` 时 receive loop 没分类就 return。

### 根因（Root cause）

WS reconnect 状态机 V1 §12.1 钦定"按 close code 分类决策"是 terminal vs transient 的**唯一**判定来源。`URLSessionWebSocketTask.closeCode` 在 receive() 抛错时被 runtime 设置；但旧实装把 classify 只埋在 startReceiveLoop 的 **post-handshake** catch 路径里，pre-handshake catch（reconnect 期间最常见的失败位）跳过 classify 直接抛 `connectionFailed(underlyingDescription:)` 让 connectGate throw。`attemptReconnect` 拿到这个无 close code 信息的 error 没法分类 → 默认全部 retry。这是"分类逻辑只覆盖一半路径"的典型疏漏 —— 写代码时容易把 classify 当作"close 后正常退出"路径的事，忽略 reconnect 期间 pre-handshake close 也会被 server 推 4001/4003 等 close code。

### 修复（Fix）

两处协同：
1. **`WebSocketClientImpl.startReceiveLoop` pre-handshake catch（line ~457）**：在 `isReconnectAttempt` 路径下读取 `task.closeCode.rawValue`；非 0（runtime 已设置 close code）→ 通过 `connectGate` 抛 `WSError.closedByServer(code: rawCode, reason:)`；0（`.invalid` 占位，DNS/TLS/任务取消）→ 沿用 defer 兜底走 `connectionFailed`。
2. **`WebSocketClientImpl.attemptReconnect` catch（line ~623）**：判断 error 是否为 `WSError.closedByServer(code:)`；是 → 反向构造 `URLSessionWebSocketTask.CloseCode(rawValue:)` 走 `classifyCloseCode` → terminal → emit `.disconnected` + finish stream + 清状态 + **不** schedule retry；transient → 继续 scheduleReconnect。非 closedByServer 的 error（DNS/TLS/connection refused）视为 transient 继续 retry（与既有 backoff 上限语义一致）。

新增测试：
- `test_reconnect_terminalCloseDuringHandshake_stopsRetrying`：第一次 connect 后 transient 4005 触发 reconnect；reconnect attempt 1 pre-handshake 收 4001 → 应只 emit 1 次 `.reconnecting` + 最终 `.disconnected` + makeTask 总数 = 2（不是 6）。
- `test_reconnect_transientCloseDuringHandshake_continuesRetrying`：4 次 4005 pre-handshake fail + 第 5 次成功 → 应 emit 5 次 `.reconnecting` + 最终 `.connected` + makeTask 总数 = 6（确保 transient 路径未被误伤）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **WS / 长连接 / 重连状态机的 catch 路径**写 retry 决策时，**必须**让 **close code 分类逻辑覆盖所有失败位置（pre-handshake + post-handshake + reconnect attempt 失败）**，不能只埋在"正常断开"路径里。
>
> **展开**：
> - pre-handshake 失败（first frame 之前 server 推 close）和 post-handshake 失败（已收到第一帧后 server 推 close）使用**同一份** close code → terminal/transient 分类表。如果协议钦定 `4001/4003/4004 = terminal`，无论在哪个阶段收到都必须 stop retry。
> - 把 close code 信息**穿过**任何中间错误转换层（如把 `WSError.closedByServer(code:)` 而非 `WSError.connectionFailed(underlyingDescription:)` 抛进 connectGate / promise）。中间层吞掉 close code 后，上层 catch 就只能盲目 retry。
> - reconnect attempt 失败后调 `scheduleReconnect()` 之前**必须先判分类**；任何"catch all → schedule next"的写法都是 bug，会把协议钦定的 terminal close 当 transient 重试。
> - **反例**：旧 `attemptReconnect` catch 一律 `scheduleReconnect()`；旧 `startReceiveLoop` pre-handshake catch 一律走 `connectionFailed(underlyingDescription:)` 不传 close code。这两个习惯组合后 4001 token 过期被白白 retry 5 次。

## Lesson 2: WS event with stream-bound semantics 必须用 streamRoomId 守护（包括 connection state）

- **Severity**: medium
- **Category**: architecture
- **分诊**: fix（推翻 dev 阶段决定）
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:385-399`

### 症状（Symptom）

vm 在 `handle(message:streamRoomId:)` 的 `.connectionStateChanged` 分支无条件更新 `wsState`。dev 阶段开放问题 §6 钦定"connection state 不携带 roster 数据，不需 streamRoomId 守护"，但这个判断错了：connection state 事件和 `member.joined` / `member.left` 一样**绑定 specific socket / stream**。A→B 房间切换 / leave-rejoin 期间，旧 consumer task 可能已 dequeue 一个 `.reconnecting` / `.disconnected` 事件，在 `lastObservedRoomId` 已变更后才被 apply → 覆盖当前 room 的 status banner，显示前一个连接的 stale 状态（"为什么我刚进 room B status 就显示重连中？"—— 因为是 room A 的旧 stream 投递的）。

### 根因（Root cause）

"streamRoomId 守护是不是必要"判断的标准被错误地设为"事件 payload 是否含 room id"。真正的判断标准是**"这个事件的语义是不是绑定到 specific stream / socket"**。connection state 是 **per-socket 状态变化**（这个 socket 在重连 / 这个 socket 已断开），跨 stream 投递没有协议上的合理性 —— 即使 "wsState 字段在 vm 上是 global"，把 stream A 的 reconnecting 写到当前是 stream B 的 vm 上也是错的（语义上 vm 想表达的是"我当前所在的房间这条连接的状态"）。Story 12.4 r1 已经确立"streamRoomId 守护防 cross-room race"的精神；本次 dev 阶段决定"connection state 不守护"是把"payload 不含 room id"误当判断依据。

### 修复（Fix）

`RealRoomViewModel.handle(message:streamRoomId:)` 的 `.connectionStateChanged` 分支加 `guard streamRoomId != nil, streamRoomId == lastObservedRoomId else { return + log }`，与 `member.joined` / `member.left` 同精神。

新增 / 翻转测试：
- 翻转 `testConnectionStateChangedNotGuardedByStreamRoomId` → `testConnectionStateChangedGuardedByStreamRoomId`：直接调 `vm.handle(.connectionStateChanged(.disconnected), streamRoomId: "room_A")` 当 lastObservedRoomId="room_B"，断言 wsState 保持 `.connected` 而非被 stale `.disconnected` 覆盖。
- 新增 `testStaleConnectionStateFromOldRoomDoesNotPollute`：A→B 切换后旧 stream 投递 `.reconnecting` 不应改变 wsState。

注意：H1/H2/H3 既有测试用 `mockWS.emitConnectionState`（注入到 stream，consumer task 启动时已捕获正确的 streamRoomId），守护通过 → 不受影响；H4 reconnect → snapshot 端到端测试同理通过。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 vm 处理"通过房间内 WS stream 投递的事件"时，**必须**对**所有事件类型**用 `streamRoomId == lastObservedRoomId` 守护 —— **不**以"payload 是否含 room id" 作判断标准，而是以"事件语义是否绑定 specific stream / socket"作判断标准。
>
> **展开**：
> - 任何走 vm consumer task → handle 的 WS message 都属于"绑定到 startConsumingMessages 启动时捕获的 streamRoomId"。哪怕事件 payload 本身没 room id 字段，事件**来源**（这条 stream）就是 room-bound 的。
> - "事件不携带 roster / payload-level 数据" **不是**跳过守护的理由；判断标准是"这个事件如果在错误的 room 上下文中 apply 会造成视觉 / 状态 bug 吗"。connection state 的"stale 状态银幕显示"就是典型可见 bug。
> - 守护层是**统一的**而非 case-by-case 的：handle 函数应该把 streamRoomId 守护放在能覆盖所有 stream-bound 事件的位置（snapshot 用 payload room.id 校验，其它所有 cases 用 streamRoomId）。每加一个新 message case 都要默认守护；要"放行不守护"必须有**明确的协议级理由**（如某个事件天然不与 room 绑定，目前没这种 case）。
> - **反例**：dev 阶段开放问题 §6 钦定"connection state 不需守护"——这是把"payload 缺数据"当作"无关 room"，是错误的对应关系。蹦带的 ViewModel handle 路径上 wsState 的 stale 覆盖比 roster 错挂更难 debug，因为 wsState 视觉变化看似"小"（一行 banner）但用户感知错误率 100%。

---

## Meta: 本次 review 的宏观教训

两条 finding 共享同一个深层模式：**"按 close code / streamRoomId 做决策"的逻辑必须覆盖所有相关路径，不能只覆盖一半**。
- Lesson 1：classify 只埋在 post-handshake，pre-handshake 漏；
- Lesson 2：streamRoomId 守护只埋在 member.* / snapshot，connection state 漏。

后续写状态机 / event handler 时，"分类 / 守护逻辑" 应作为**横切关注点**统一覆盖，而不是"先写 happy path，再为某些 case 加补丁"。
