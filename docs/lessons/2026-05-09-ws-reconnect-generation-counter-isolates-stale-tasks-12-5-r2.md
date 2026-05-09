---
date: 2026-05-09
source_review: codex review (round 2) — /tmp/epic-loop-review-12-5-r2.md
story: 12-5-自动重连
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-09 — generation counter 在重连状态机里隔离 stale task 的共享状态写入（Story 12.5 r2）

## 背景

Story 12.5 r1 修了 reconnect 期间 pre-handshake close 的 close-code 分类决策（让 4001 不再被白白 retry 5 次）。
r2 codex review 又找到两条更深的 generation/cancellation race：

1. 旧 receive-loop 的 catch path 在 `prepareForReconnect()` swap 新 stream **之后**才跑 → 旧 task 通过新 `currentContinuation`
   emit `.disconnected` / `scheduleReconnect()`，let room A's late close 在 room B 的 stream 上显示 `.disconnected`/
   `.reconnecting`，触发错误 session 的 reconnect。
2. `disconnect()` / `prepareForReconnect()` cancel 一个已经在 `connectInternal` 内的 reconnect attempt 时，
   cancelled task 仍然落到 catch block —— 旧实装无 `Task.isCancelled` / generation check，stale catch 安装新
   `reconnectTask`；如果 fresh `connect(roomId:)` 在 stale catch 跑之前发生，delayed retry 会 race 新 connection
   连错房间。

两条同根：reconnect 状态机里的 long-lived async task（receive-loop / scheduled retry / reconnect attempt）
持有的"上下文"在 caller 侧已经被新 connect/disconnect/prepareForReconnect 翻新，但 task 自身没有任何机制感知这件事。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 旧 receive-loop catch path 写入新 stream | P1 | architecture | fix | `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:498-514` |
| 2 | cancelled reconnect attempt 的 catch 仍 schedule new retry | P1 | architecture | fix | `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:667-700` |

## Lesson 1: stale receive-loop catch path 必须按 generation 隔离，不能直接写共享 continuation

- **Severity**: P1 / high
- **Category**: architecture (concurrency)
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift` 旧的 startReceiveLoop catch path（行 498-514）

### 症状

`prepareForReconnect()` 的语义是："cancel 旧资源 + 重建 fresh stream"。但 cancel 不是同步的 ——
旧 receive-loop 的 `catch` 块可能在 `currentContinuation` 已经被 swap 成新 stream **之后**才跑。
旧 catch 调 `emitConnectionState(.disconnected)` / `scheduleReconnect()` 时，从锁内取出来的
`currentContinuation` 已经是**新 stream 的 continuation**。结果：room A 的 late close 在 room B 的 stream 上
emit `.disconnected` / `.reconnecting`；甚至触发 attemptReconnect → 用 currentRoomId 重连错误的 session。

绕过了 `RealRoomViewModel` 的 `streamRoomId` 守护，因为事件 in fact 已经在新 stream 上了。

### 根因

把"redirect 共享状态写入"等同于"cancel 旧 task + swap 新 state"。但 Task cancellation 是协作式的：
旧 task 在哪个 await 点感知 cancellation、catch path 几行代码什么时候跑完，完全是 runtime 调度决定的。
任何"先 swap 状态 + 后 cancel 旧 task"的 reset 路径，都必须假设旧 task 还能再读 / 再写一次新状态。
`NSLock` 保护字段的内存可见性，但**不**保护"task 自己以为的 session 还是当前 session"这层时序约束。

### 修复

加 `private var sessionGeneration: Int = 0` 字段。每次 `connect(roomId:)` / `prepareForReconnect()` /
`disconnect()` 在持锁内 `sessionGeneration += 1`。

所有 launched async task（receive-loop / scheduleReconnect 启动的 sleep+retry Task / reconnect attempt）
在 launch 时**捕获**当时的 `sessionGeneration` 作为 local `let mySession: Int` 常量。

任何写 `currentContinuation` / 调 `emitConnectionState` / 调 `scheduleReconnect` / 写 `reconnectTask` 之前
做 generation 校验：`mySession == sessionGeneration` 才允许写；不匹配 silent return（log debug）。

为 emit / schedule 提供 `*IfCurrent(mySession:)` 包装，原 `emitConnectionState(_:)` 直接删除以杜绝
"未来 callsite 不小心绕过 gate"的复发风险。

具体落地：
- `emitConnectionStateIfCurrent(_:mySession:)`：持锁内 generation 校验 + 取 continuation。
- `scheduleReconnectIfCurrent(mySession:)` → `scheduleReconnect(mySession:)`：入口 generation 校验，sleep+retry
  闭包内再做一次 + 写 `reconnectTask` 前再做一次（防 schedule 期间被 disconnect 抢）。
- 终态 finish stream 前也做 generation 校验（持锁 + guard），防极端 race 在 `cont.finish()` 调用前 gen 翻动。

### 预防规则（Rule for future Claude）

> **一句话**：在写"redirect 长寿命 async 任务上下文"的 reset 路径时（`prepareForReconnect` / `disconnect` /
> `reset` 等），未来 Claude **必须**用 generation counter（或等价的 epoch / token）让旧任务在共享状态写入处自我识别为 stale 并 silent drop —— **不能**只依赖 cancel 信号 + 锁保护字段可见性来"赶走"旧任务。
>
> **展开**：
> - 写 `prepareForXxx` 时问自己：旧任务在 cancel 信号传过去之前还能跑几行代码？这几行代码会读 / 写哪些字段？
>   - 读 `currentXxx` 字段：用 generation 校验 silent drop。
>   - 写 emit / yield 到新 stream：用 generation gate 包装 emit 接缝。
>   - install 新 task / new state：在持锁块内做"check gen → write"原子序列，不要"check gen 释放锁 → write"。
> - generation 校验的 callsite 模式：
>   ```swift
>   func handleSomeAsyncEvent(mySession: Int) {
>       lock.lock()
>       guard sessionGeneration == mySession else { lock.unlock(); return }
>       // ... mutations under lock, or atomically capture refs to use after unlock
>       lock.unlock()
>   }
>   ```
> - launch 任务时把 generation 抓进闭包 local：
>   ```swift
>   let mySession: Int = { lock.lock(); defer { lock.unlock() }; return sessionGeneration }()
>   let task = Task { [weak self] in await self?.doWork(mySession: mySession) }
>   ```
>   这是 closure capture 语义，避免任务每次重读"现在的 generation"（重读会破坏 generation 的语义 —— 旧任务也会
>   看到新 gen 然后误以为自己活着）.
> - 提供 `*IfCurrent(mySession:)` 包装，把"普通 emit /普通 schedule"路径删除（或私有化到只在 generation gate
>   后调用），防未来 callsite 不慎绕过 gate。
> - **反例**：
>   - 仅 `Task.isCancelled` 不够 —— cancel 信号的传播延迟和 catch path 的 unwind 时机无法保证；catch 块入口
>     再加 `if Task.isCancelled { return }` 也只能兜一半 race，因为 cancel 的时间点未必早于"caller 已经 install
>     新 session 状态"的时间点。
>   - 仅 `NSLock` 字段保护不够 —— 锁保证字段的可见性 / 互斥写，但不保证"读到这个字段的 task 还属于当前 session"。
>     旧 task 持锁读到的 `currentContinuation` 在内存可见性意义上正确（确实是新 stream 的 continuation），
>     但语义上完全错（旧 session 的 task 不该 emit 到新 session 的 stream）。
>   - 把 generation 校验放到 unlock 之后再做 —— 释放锁到再次操作字段之间会有 ABA window；必须把"check gen + 取
>     待用引用"放进同一个持锁块。

## Lesson 2: cancelled async task 的 catch path 必须明确 silent-drop 决策，否则 stale catch 会 install 新 session 的资源

- **Severity**: P1 / high
- **Category**: architecture (concurrency)
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift` 旧的 attemptReconnect catch path（行 667-700）

### 症状

`disconnect()` / `prepareForReconnect()` cancel 一个已经在 `connectInternal` 内的 reconnect attempt 时，
cancelled task 仍然落到 `catch` 块。旧实装把所有非-`closedByServer` error 当 transient（包括 `CancellationError`），
**无条件**调 `scheduleReconnect()` —— 把"caller 已经放弃"的 attempt 转成"安装新 reconnectTask"。

如果 caller 在 stale catch 跑之前发生 `connect(roomId: <new>)`，delayed retry 用的还是 `currentRoomId`
（已被 fresh connect 写成新 roomId），就会消耗一次 fresh ROOM_B 的 makeTask 名额做 stale 重试 —— 看起来
"莫名其妙连了一下又断"，更糟时 race fresh connection。

### 根因

`do { try await connect... } catch { ... }` 默认假设 catch 里能读到的状态都属于"产生这个 error 的 session"。
但 reconnect 状态机里 task 是被 caller cancel 的 —— `await` throws `CancellationError`，catch 块继续执行
"基于现在的字段值做下一步决策"。caller 已经翻新了 currentRoomId / reconnectAttempt / sessionGeneration，
catch 块读到的字段是新 session 的，但行为上还在按旧 session 的"我刚刚失败了一次，再来一次"逻辑跑。

### 修复

在 `attemptReconnect(attempt:mySession:)` 的入口、success 路径写共享状态前、catch 入口三处做 generation
校验。catch 入口顺带加 `Task.isCancelled` 兜底（极端时序：cancel 信号传到 task 比 `disconnect()` 的
`sessionGeneration += 1` 更早；这种场景 generation 还没翻但任务已被 cancel）。

stale catch silent return 不再调 `scheduleReconnect`、不再 emit `.disconnected`、不再写 `reconnectTask` /
`currentRoomId` / `reconnectAttempt`。

测试覆盖（3 case）：
- `test_reconnect_cancelledAttemptCatchDoesNotScheduleNewRetry`：attempt 1 卡在 receive() 永久 block →
  disconnect cancel → 验证 makeTaskCallCount 不增（stale catch 没 schedule 第三次 makeTask）。
- `test_reconnect_staleReceiveLoopCatchDoesNotPolluteFreshStream`：connect 后 prepareForReconnect → 旧
  receive-loop catch 跑 → 验证新 stream 1 秒内无任何 connection-state 事件 + makeTaskCallCount == 1。
- `test_reconnect_freshConnectAfterCancellationNotRacedByStaleRetry`：disconnect 后 fresh connect ROOM_B
  → 验证 stale catch 不会 schedule 后续 retry 消耗 fresh ROOM_B 之后的 makeTask 名额。

### 预防规则（Rule for future Claude）

> **一句话**：写"async task catch path 里基于实例字段做下一步动作"的代码时，未来 Claude **必须**先做 staleness
> check（generation 校验 + `Task.isCancelled` 兜底），再读字段做决策；**禁止**把"catch 里直接 schedule 后续动作"
> 当 default behavior。
>
> **展开**：
> - "fail-then-retry" 模式有一个隐藏假设："产生 error 的 session 还活着"。这个假设在长寿命 async task 里不成立：
>   caller 可以在 task await 期间 cancel + reset 实例状态，task 一旦 unwind 进 catch 就会用新状态做旧逻辑。
> - catch 入口标准模板：
>   ```swift
>   } catch {
>       if !isCurrentSession(mySession) { return }   // generation gate
>       if Task.isCancelled { return }                // 兜底：cancel 早于 gen 翻动
>       // ... 真正的 retry / fail 逻辑
>   }
>   ```
> - `Task.isCancelled` 单独不够（catch 已经 unwind 进来，cancel 信号可能没传播完整），但作为"generation 还没翻
>   但 cancel 已经下达"这个极端时序的兜底很便宜也很值。
> - **反例**：
>   - `do { try await ... } catch { scheduleNextAttempt() }` —— 任何长寿命 task 都不能这样写，必须先 staleness
>     check。
>   - 把 `if !isCurrentSession(mySession) { return }` 放在 catch 中间（先做了若干 `lock.lock` / log /
>     emit）—— 这些"前置动作"本身就会污染新 session 的状态序列；gate 必须在 catch 第一行。
>   - 仅靠"catch 里读 currentRoomId == nil 就 abort"防御 —— `nil` 检查只在 disconnect 路径有效（disconnect 清
>     nil）；prepareForReconnect 也清 nil 但 caller 通常立即 fresh connect 写回 → catch 读到的是新 roomId，
>     防御失效。

---

## Meta: 本次 review 的宏观教训

两条 finding 同根：**"长寿命 async task 的状态决策必须显式按 session 隔离，不能默认信任实例字段"**。

任何带"reset / reconnect / rebind"动作的 client，每次 reset 都生成一个新 epoch（generation counter 是最简单的
epoch 实现）。所有"会跨 reset 边界存活的 task"（receive-loop / sleep-then-retry / cancellable in-flight
attempt）都要在 launch 时把 epoch 抓进闭包，所有写共享状态的入口都要 epoch 校验。这与 12.4 r1 的
`streamRoomId` 守护是同精神：用一个 monotonic id 把"事件"和"它产生时所属的 session"绑死，事件到达时如果 session
不再有效，silent drop。

下次写类似 client（WS / SSE / long-poll / observer pattern with reset 接缝）：起手就引入 generation counter。
不要等到 review round 2 才补。
