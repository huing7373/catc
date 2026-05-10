---
date: 2026-05-10
source_review: codex review (epic-loop round 5) — /tmp/epic-loop-review-12-6-r5.md
story: 12-6-心跳维护
commit: f5d5c2c
lesson_count: 1
---

# Review Lessons — 2026-05-10 — WS heartbeat send catch 路径必须用 task identity 守护，仅靠 sessionGeneration 不够 —— reconnect 透明续接保留同 generation

## 背景

Story 12.6（WS 心跳维护）round 5 codex review。前四轮已修复：r1 transient reconnect 前 heartbeat state reset；r2 pong requestId 配对；r3 ping send 抛错 force reconnect；r4 unlock window pre-send re-verify + captured task 直接 send。本轮 codex 指出 send 抛错的 catch 路径残留同一类 race：catch 内调的 `cancelUnderlyingTaskWithGoingAwayIfCurrent(mySession:)` 仍仅靠 `sessionGeneration == mySession` 守护，但 reconnect 透明续接（receive-loop catch → cancelHeartbeatStateForReconnectIfCurrent → attemptReconnect → connectInternal）**故意保留同一 sessionGeneration**（外层 stream / vm 视角不感知 session 翻新），所以这条 guard 在 race 时序下会通过，然后读 `self.underlyingTask` 取到刚装上的新 socket → cancel(.goingAway) 错杀新 socket → receive-loop 又分类为 transient → 又 schedule reconnect → 一个 transient disconnect 演化成 self-sustaining reconnect loop。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | heartbeat send catch 仅用 sessionGeneration 守护 → 与 reconnect 透明续接同 gen 撞车 → 错杀新 underlyingTask → self-sustaining reconnect loop | P1 (high) | concurrency | fix | `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:1356-1367` |

## Lesson 1: 当一个共享字段（如 underlyingTask）会在 race window 内被无声 swap（不翻你监控的那个 generation 计数器），守护必须用**对象 identity**（`===`），不能仅用 generation

- **Severity**: high (P1)
- **Category**: concurrency
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:1356-1367`（heartbeat send 抛错的 catch 块）

### 症状（Symptom）

heartbeat 单轮：① lock 内取 `activeTask = self.underlyingTask` snapshot + install latch；② unlock + pre-send 校验（r4 P1 防御）；③ `try await activeTask.send(.string(text))`；④ catch 跑 cleanup 然后调 `cancelUnderlyingTaskWithGoingAwayIfCurrent(mySession:)`。

race 时序：
- T0: ③ 在 socketA 上 suspended（async send 还没返回）
- T1: socketA 底层断开 → receive-loop 自己的 catch 跑：调 `cancelHeartbeatStateForReconnectIfCurrent`（cancel 旧 heartbeat + finish 旧 pongCont，**不翻 sessionGeneration**）→ `scheduleReconnectIfCurrent` → `attemptReconnect` → `connectInternal` → install 新 `underlyingTask = socketB`，**sessionGeneration 仍 == mySession**（reconnect 透明续接的契约 5）
- T2: ③ 那个 send 终于抛错（因为 socketA 底层已断）
- T3: ④ catch 跑 → 调 `cancelUnderlyingTaskWithGoingAwayIfCurrent(mySession:)`：① guard `sessionGeneration == mySession` **通过**；② lock 内取 `task = self.underlyingTask` 拿到 **socketB**；③ unlock + `task?.cancel(with: .goingAway, ...)` → socketB 被 cancel
- T4: socketB cancel 触发它自己 receive-loop 的 catch → 又分类为 transient → 又 schedule reconnect → 又 install socketC → ……（self-sustaining loop）

弱网 / buffered-send 场景下（send 容易在底层 socket 断之前 suspend 一段），这个 race 会反复触发 → 1 个 transient disconnect 演化成永远连不上。

### 根因（Root cause）

**generation 计数器作为 isolation 工具有它的 sweet spot，也有盲区**。在本仓库 12.5 / 12.6 的设计里，`sessionGeneration` 监控的语义是 "caller 显式 disconnect / prepareForReconnect / 主动 connect" 这类**外层可见**的 session 翻新。reconnect 透明续接**故意**不翻 gen，因为外层 stream / vm 不应该看到一次 transient reconnect 当作"新 session"。这是健康的产品语义。

但这就意味着：在 reconnect 透明续接路径上，`underlyingTask` 字段会被无声 swap，但**任何只查 sessionGeneration 的 isolation guard 都看不到这次 swap**。本次 race 的 `cancelUnderlyingTaskWithGoingAwayIfCurrent` 就掉进这个盲区 —— 它名字里写着 "IfCurrent"，但 "current" 的定义只覆盖了"当前 session"，没覆盖"我当初锁定的那个 task"。

更深一层 anti-pattern：**异步 send 抛错的 catch 块里，"我当初 send 的是哪个 task" 已经在 catch 上下文里有 captured 引用（`activeTask`）了**，但代码却又调一个 `cancelUnderlyingTaskWithGoingAwayIfCurrent(mySession:)` 重新 read `self.underlyingTask`。这种"明明手里有，还要去重新拿一次"的实装，恰好是 r4 P1 同一类问题的另一面 —— r4 P1 是 send 路径不该 re-read，r5 P1 是 catch 路径不该 re-read。两者根因相同：**lock 内拿出的 snapshot 必须贯穿到所有后续对外副作用，不能在 unlock 之后又被 self.X re-read 路径替换掉**。

### 修复（Fix）

`iphone/PetApp/Core/Networking/WebSocketClientImpl.swift` heartbeat send 抛错的 catch 块（around line 1356）：

before（仅 sessionGeneration 守护，且 cancel 路径 re-read self.underlyingTask）：
```swift
} catch {
    // latch cleanup
    strongSelf.lock.lock()
    strongSelf.pendingPongContinuation?.finish()
    strongSelf.pendingPongContinuation = nil
    strongSelf.pendingPongRequestId = nil
    strongSelf.lock.unlock()
    // cancelUnderlyingTaskWithGoingAwayIfCurrent 内部仅 sessionGeneration guard + read self.underlyingTask
    strongSelf.cancelUnderlyingTaskWithGoingAwayIfCurrent(mySession: mySession)
    return
}
```

after（task identity 校验 + latch cleanup 也加 sessionGeneration guard）：
```swift
} catch {
    let underlyingStillSame: Bool
    strongSelf.lock.lock()
    underlyingStillSame = strongSelf.sessionGeneration == mySession
        && strongSelf.underlyingTask === activeTask
    // latch cleanup 仅当还在本 session（防 stale catch 清掉新 session 的 latch）
    if strongSelf.sessionGeneration == mySession {
        strongSelf.pendingPongContinuation?.finish()
        strongSelf.pendingPongContinuation = nil
        strongSelf.pendingPongRequestId = nil
    }
    strongSelf.lock.unlock()
    if !underlyingStillSame {
        // 我当初 send 的那个 task 已被 swap 走（reconnect 透明续接 / 新 session）
        // → silent skip cancel；新 task 的 reconnect 路径已经接管。
        return
    }
    strongSelf.cancelUnderlyingTaskWithGoingAwayIfCurrent(mySession: mySession)
    return
}
```

测试侧：
- 新增 fake `FakeWebSocketTaskHandle.beforeSendThrowHook`：让 send 在抛 `sendThrowsError` 之前 await 一个 hook，模拟 "send 已 suspended、reconnect 已 swap 新 underlyingTask、旧 send 才抛错" 的精确 race 时序。
- 新增 internal `WebSocketClientImpl._simulateTransparentReconnectSwapForTest(newTask:)`：模拟 receive-loop 触发的 transparent reconnect 路径 —— 调 `cancelHeartbeatStateForReconnectIfCurrent` + swap `underlyingTask`，**不翻 sessionGeneration**（与 production reconnect 透明续接同语义）。
- 新增 case `test_heartbeat_sendCatchUsesTaskIdentityNotJustGeneration_round5_P1`：① firstTask 配 sendThrowsError + beforeSendThrowHook；② hook 内调 `_simulateTransparentReconnectSwapForTest(newTask: secondTask)`；③ send 抛错 → catch 跑修复后逻辑；④ 断言 `secondTask.cancelCallCount == 0`（未被错杀）。

修复前测试 fail（actual=1，secondTask 被 cancel(.goingAway) 错杀），修复后测试 pass（actual=0）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写"async I/O 抛错的 catch 路径需要清理某个共享对象"时，**必须**用**对象 identity（`===` 引用比较 / 显式 token 比较）**校验当前共享字段是否仍是当初我开始 I/O 时锁定的那个引用，**禁止**仅靠 generation 计数器这种"按业务节拍翻动"的标签 —— 因为透明 swap 路径（如 reconnect 透明续接）会**故意**不翻你监控的那个 generation。
>
> **展开**：
> - 当一个 generation 计数器有"故意不翻"的合法路径（reconnect 透明续接 / hot reload / 业务无感切换 worker），它就**不**适合作为对该 generation 周期内**短生命周期对象引用**（socket / task / db conn）的 isolation guard。需要在它之上再加一层 identity 校验。
> - lock 内 snapshot 出来的对象引用（如 `activeTask = self.underlyingTask`）必须贯穿到所有后续对外副作用 —— 不光发送路径要用 snapshot 直接 send，**catch 内的 cleanup / cancel 路径也要用 snapshot 直接 cancel**，不能调一个内部 re-read `self.X` 的 helper。
> - 写 catch 块前问自己：① "我这个 catch 是因为对哪个具体对象 X 的操作失败？"；② "我要清理 / cancel 的对象，是不是同一个 X？"；③ "X 在 self 上的字段引用，从我 await 之前到 catch 跑这段时间，可能被换走吗？" —— 如果 ③ 答案是"可能"，**捕获 X 的引用、显式用它**（捕获在 closure / local var）；不要走 self.X re-read。
> - 代码里写 `IfCurrent` 后缀的 helper 时，注释里必须明示 "current" 的定义边界 —— 是 "current session" / "current generation" / "current task identity" 中的哪一个？跨边界使用 = race。本仓库 round 5 P1 的 `cancelUnderlyingTaskWithGoingAwayIfCurrent` 就因为 "current" 的语义边界没明示，让 catch 路径错以为 generation 守护够用。
> - **反例**：在 async send 抛错的 catch 里写 `self.cleanupResourceIfCurrent(mySession: mySession)`，且 `cleanupResourceIfCurrent` 内部 `lock; resource = self.someResource; unlock; resource.cancel()` —— 看起来"持锁内读 + isolation guard"双重保护，实际上在 generation 不翻的透明 swap 场景下，guard 通过 + read 拿到的是新 resource → 错误清理。**正例**：catch 里写 `if self.someResource === capturedResource { self.someResource.cancel(); self.someResource = nil } else { /* silent skip：新 resource 已接管 */ }`。
> - 测试单元这种 race 时，必须能**精确控制 send 已 suspended 但还没抛错** 的时刻 —— fake handle 加 `beforeSendThrowHook` 这种 await 钩子比 `Task.sleep` 时序协调更可靠（不依赖 wall-clock 时序，跨 CI 机器稳定）。
> - 测试 transparent reconnect 类的 race 时，**不能**直接调 `prepareForReconnect()`（那会翻 gen，不再是"透明"），必须有一个 internal test-only swap helper 模拟 receive-loop 路径（cancelHeartbeatStateForReconnectIfCurrent + swap underlyingTask + 不翻 gen）。production 永远不调，仅注入路径同 fake-clock 模式。

---

## Meta: 本次 review 的宏观教训

12.6 心跳维护连续 5 轮 review 都击中**异步 send/cancel 路径的 race**，每一轮的根因都收敛到同一个抽象问题：**当 generation counter（用于 session isolation）和 short-lived object identity（用于 task isolation）不重合时，仅查 generation 是不够的**。r4 修了 send 路径的 generation+identity 双层防御，r5 修了 catch 路径的同一类问题。下一轮（如果有 r6）需要审视的是 pong arrival / pong timeout 路径是否还有同类盲区 —— 凡是"我对这个 socket 的某个引用做副作用"的地方，都要问：generation guard 够吗？还是需要加 identity guard？
