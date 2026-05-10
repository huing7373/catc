---
date: 2026-05-10
source_review: codex review (round 8) — /tmp/epic-loop-review-12-6-r8.md
story: 12-6-心跳维护
commit: 218639b
lesson_count: 1
---

# Review Lessons — 2026-05-10 — WS heartbeat closeCode TOCTOU re-check 必须 atomic 折进 helper 内部，catch 入口先读 + helper 内 cancel 是 race window（12-6 r8）

## 背景

Story 12.6 第 7 轮 fix-review 已经把 "heartbeat send catch 不能用 cancel(.goingAway) 1001 覆盖
server 真实 close code（4001/4004 等 terminal）" 的逻辑加了进去 —— 实装方式是：catch 入口先读
一次 `observedCloseCode = activeTask.closeCode`，不为 `.invalid` 就 silent skip；为 `.invalid`
才调 `cancelUnderlyingTaskWithGoingAwayIfCurrent(mySession:)` helper 进入 cancel 路径。

第 8 轮 codex review 指出 round 7 修复仍留有 TOCTOU race window：catch 入口 read 与 helper
内 cancel 调用之间是 unlocked，server close frame 仍可能在中间到达把 `task.closeCode` 从
`.invalid` 改为 4001/4004。此时旧路径已 commit 走 cancel 分支 → cancel(.goingAway) 覆盖真实
close code → terminal 被错分 transient → silent retry 而非 emit `.disconnected` 触发 re-auth。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | catch 入口 closeCode 先 read 与 helper 内 cancel 调用之间存在 unlocked window，server close 仍可能在 race 内覆盖 closeCode → 1001 错杀 4001/4004 | P2 | architecture / concurrency | fix | `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:1438-1473, 1503, 1547-1605` |

## Lesson 1: closeCode TOCTOU re-check 必须 atomic 折进 helper 持锁段，不能 catch 入口先 read 后 cancel 跨锁

- **Severity**: P2
- **Category**: architecture / concurrency
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:1438-1473`（heartbeat send catch caller）+ `:1503`（pong timeout caller）+ `:1547-1605`（helper 本身）

### 症状（Symptom）

Round 7 修复在 catch 入口读一次 `let observedCloseCode = activeTask.closeCode`：
- != `.invalid` → silent skip cancel
- == `.invalid` → 调 `cancelUnderlyingTaskWithGoingAwayIfCurrent(mySession:)` 走 cancel 路径

但这条逻辑两步之间是 unlocked 的（lock 已 release，cancel 调用前还要做其它 cleanup）。
race 时序：

```
T_a  catch 跑：read activeTask.closeCode = .invalid  →  决定走 cancel 分支
T_b  server close frame 到达 → URLSessionWebSocketTask runtime 设 task.closeCode = 4001
T_c  helper 跑：直接 cancel(.goingAway)
T_d  runtime 把 task.closeCode 覆写为 .goingAway（1001）
T_e  receive-loop catch → read closeCode = .goingAway → classifier transient → silent retry
```

terminal close（4001 token 过期 / 4004 房间满）应该 emit `.disconnected` 触发 caller re-auth /
显错；silent retry 让 client 反复尝试已 invalid 的 token，损坏 12.5 terminal-vs-transient contract。

### 根因（Root cause）

> "先 check、再操作" 是经典 TOCTOU。即使 round 7 把 check 引进来，**check 与操作之间存在
> unlocked window** = race 仍存在。修复 TOCTOU 的根本不是"读一次"而是"消除中间 unlocked
> window"—— 把 check 移到 mutation 同一持锁段内，让 read + cancel 是 atomic 的。

之前 helper 内的持锁段已经做了 `sessionGeneration` 校验和释放 task ref，但没有加
closeCode re-check —— 我们以为 catch 入口 read 一次就够，但 OS runtime 写 task.closeCode 是
异步且不依赖客户端 lock 的（URLSession 在内部线程写），任意时刻都能发生。**消除 race window
的唯一办法是把 check 紧贴 cancel 调用，并在同一锁内完成 "read closeCode → 决策 → release task ref"
的全部步骤**。

### 修复（Fix）

**1. helper 签名扩展**：`cancelUnderlyingTaskWithGoingAwayIfCurrent(mySession:activeTask:)` 新增
`activeTask` 参数 —— caller 把自己捕获的 task 传进来；helper 在持锁段内完成三件事：

```swift
lock.lock()
guard sessionGeneration == mySession else { ... silent skip }
guard underlyingTask === activeTask else { ... silent skip (transparent reconnect swap) }
let observedCloseCode = activeTask.closeCode      // ← atomic re-check 在持锁段内
guard observedCloseCode == .invalid else {
    lock.unlock()
    return  // server close 已到达 → silent skip 让 receive-loop classify 真实 code
}
let task = underlyingTask
lock.unlock()
task?.cancel(with: .goingAway, ...)               // ← 出锁立刻 cancel；与 re-check 之间没有
                                                   //    OS-driven 第三方写 closeCode 的可观察 window
```

**2. caller 同步**：两个 caller（heartbeat send catch + pong-timeout）都改为传 activeTask 进
helper。catch 路径仍保留入口的 round 7 silent-skip（最常见路径下能省一次锁），但**真正的
原子保障在 helper 内**。

**3. 顺带防线**：helper 内多加 `underlyingTask === activeTask` 校验 —— pong-timeout caller 之前
只校验 generation 不校验 task identity，新加的校验等于把 round 1 P1 的修复在 helper 也加了一道
防御网（reconnect swap 后旧 timer fire 时拒绝错杀新 socket）。

**4. 测试 fixture 配合调整**：
- `FakeWebSocketTaskHandle.cancel(with:)` 模拟 URLSessionWebSocketTask 的 production 行为 ——
  cancel 后把 stubbedCloseCode 对齐成 caller 传入值（仅当当前是 `.invalid`），让 receive-loop
  catch 后能正确 classify。
- 旧测试里 `firstTask.stubbedCloseCode = .goingAway` 这种 pre-set 在 round 8 修复后会让 helper
  提前 silent skip → 改为默认 `.invalid` + 让 fake 在 cancel 时自动对齐。

**5. 新增回归测试** `test_heartbeat_sendCatchAtomicCloseCodeReCheck_round8_P2`：
- fake task `swapCloseCodeOnNthRead = (nthRead: 2, swapTo: 4001)` 模拟两次 closeCode read 之间
  server close 到达：第 1 次 read（catch 入口）= .invalid → 进入 helper；第 2 次 read（helper
  atomic re-check）= 4001 → silent skip
- 断言：`firstTask.lastCancelCloseCode != .goingAway`（修复前会等于 .goingAway → 失败）
- 断言：`closeCodeReadCount >= 2`（验证 helper 真的多读了一次）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写 "先 check 某状态、再做有副作用动作（cancel / write / commit）" 的
> 路径时，**必须**把 check 放进与 mutation 同一持锁段（atomic check-and-act），**禁止**让
> "check 后释放锁、再做 mutation" 的模式留到 production —— 锁外随时有 OS / 异步路径写状态。
>
> **展开**：
> - 状态来源是 OS runtime（如 `URLSessionWebSocketTask.closeCode`、`URLSessionTask.state`、
>   `Process.isRunning`）时尤其危险 —— OS 内部线程写、不取你的锁、任意时刻可发生。
> - "catch 入口 read + 后续 cancel" 这种两步是 TOCTOU ：要么把 read 拷进同一持锁段，要么
>   写一个原子 helper 让所有 caller 用统一入口（推荐第二种 —— call site 不用各自重复）。
> - 把 check 放进 helper 后，所有 caller（不止你修的那个）都自动受益 —— 例如本轮 pong-timeout
>   caller 没在 review 里点名，但同样路径仍有 race（pong timeout 触发时 server 也可能已 close
>   socket），把 check 折进 helper 顺带也修了它。
> - **反例 1**：在 catch 入口写 `if x.closeCode != .invalid { return }; x.cancel(.goingAway)`
>   ——两行之间任意时刻 OS 写 x.closeCode 都会让 cancel 错误覆盖。
> - **反例 2**：把 check 移进 helper 但仍在 lock release **之后**调 cancel —— 仍有 unlocked
>   window；正确做法是在持锁段内确认 closeCode 仍 == .invalid，**release lock 后立即 cancel**
>   （这中间窗口缩到极小且没有任何客户端代码会改 closeCode）。
> - **反例 3**：测试里 pre-set stubbedCloseCode 模拟 cancel 后 runtime 设值 —— 这会被 atomic
>   re-check 看到从而 silent skip，破坏测试本意。正确做法是让 fake 在 cancel(with:) 实际被
>   调用时自动对齐 closeCode，模拟 production runtime 行为。
> - 当 review 提示 "small window" / "race between A and B" 时，先想 **能否消除中间 window**
>   （atomic 化），而不是 "再读一次状态" —— 后者只是把 window 缩短，没消除。
