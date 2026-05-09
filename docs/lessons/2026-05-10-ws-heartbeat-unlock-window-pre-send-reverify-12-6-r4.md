---
date: 2026-05-10
source_review: codex review (epic-loop round 4) — /tmp/epic-loop-review-12-6-r4.md
story: 12-6-心跳维护
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-10 — WS heartbeat 在 lock-unlock 后必须 pre-send 重新校验，且发送应使用 captured task 引用而非 re-read self.underlyingTask

## 背景

Story 12.6（WS 心跳维护）round 4 codex review。前三轮已修复：r1 transient reconnect 前 heartbeat state reset；r2 pong requestId 配对；r3 ping send 抛错 force reconnect。本轮 codex 指出第四个 race：heartbeat task 在持锁块内 `install pendingPongContinuation + 取出 underlyingTask snapshot` 之后、`unlock` 之后、`send(.ping)` 之前的极小 race window 内，若 reconnect / caller-driven `connect()` 跑过把 `underlyingTask` 换成新 socket，旧 heartbeat task 会通过 `self.send(...)` 内部 re-read `self.underlyingTask` 拿到**新 socket**，把 stale `ping_<seq>` 发到新连接。新连接的 server 在 mandatory `room.snapshot` 之前回 `pong` → 打破 12.2 的 "first frame == handshake snapshot" invariant → resolve `connect()` 在 room state 初始化前 → caller 拿到 incomplete state。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | heartbeat 在 lock-unlock 后没 pre-send 重新校验 + send 内部 re-read self.underlyingTask 让旧 ping 落到新 socket | P1 (high) | concurrency | fix | `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:1249-1300` |

## Lesson 1: lock 内 snapshot 出来的引用必须**直接使用**，绝不要在解锁后再走一条会 re-read 共享状态的路径

- **Severity**: high (P1)
- **Category**: concurrency
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:1249-1300`（heartbeat task 内 install latch → unlock → send 三步）

### 症状（Symptom）

heartbeat task 单轮执行 5 步：① lock； ② install pendingPongCont + heartbeatSeq++；③ 取出 `activeTaskOpt = self.underlyingTask` snapshot；④ unlock；⑤ 调 `self.send(.ping(...))` 发 ping。

旧实装的 race window 在 ④→⑤ 之间：
- 此时 `Task.isCancelled` 为 false（reset 路径还没 cancel 旧 heartbeatTask 完成）
- `sessionGeneration` 也可能仍 == mySession（reset 路径还没翻 gen）
- 但 reset 路径若**已经在跑**（parallel）：`cancelHeartbeatStateForReconnectIfCurrent` cancel 旧 task + finish pendingPongCont + 新一轮 connectInternal install 新 underlyingTask + `sessionGeneration += 1`
- ⑤ 调 `self.send(...)` —— send 内部又一次 `lock; task = self.underlyingTask; unlock` → 拿到**新装的 socket**
- ping 发到新 socket → server 在 mandatory `room.snapshot` 之前回 pong → resolve `connect()` 在 room state 初始化之前

观察到的 caller-side 症状（如果 race 命中）：connect() 返回成功但 home / room view 看到 incomplete room state（roster 空 / host 未定 / cosmetics 未拉取）。这是非确定性 race，发生概率取决于 ping interval 与 reconnect 时序的重叠窗口。

### 根因（Root cause）

两个相互独立的 anti-pattern 叠加成 race：

**Anti-pattern 1**：在持锁块内 snapshot 出 `activeTask` 引用，但解锁后**没用**这个 snapshot 直接发送，而是绕了一圈调 `self.send(...)`，让 `self.send` 内部再次 lock + re-read `self.underlyingTask`。这两次 read **不是同一个**值（中间有 unlock window），但代码读起来像是同一个值，掩盖了二次 read 的语义陷阱。**lock 内 snapshot 的全部价值就在于"我后续要用这个固定快照"** —— 一旦解锁后又走 re-read 路径，snapshot 等于白做。

**Anti-pattern 2**：lock 内的两次 isolation check（`Task.isCancelled` + `sessionGeneration == mySession`）都在**取 snapshot 之前**做，没在 unlock 之后、send 之前再做一次 final check。这违反了 "check-then-act" 的最常见 race 修复模式 —— **在 act（"act"=对外副作用，本例就是发 ping 到 socket）之前，必须重新核验之前的 check 仍然成立**，否则 unlock window 内 race 跑完会让 check 与 act 之间状态变化。

合在一起看，问题的本质是 **race window 的存在被 multi-step 实装隐藏**。读代码的人第一眼看到 "lock → check → snapshot → unlock → 用 snapshot" 会觉得 race-free；但 "用 snapshot" 这一步实际是 `self.send(...)` 内部又走一次 read 路径，snapshot 没有真正被使用。

### 修复（Fix）

**双层防御**（review 提示中的 "更稳妥" 路径）：

防御层 1（最强）：用 captured `activeTask` 直接 send，绕过 `self.send` 的 re-read。即便 race 跑赢、`self.underlyingTask` 已被换成新 socket，本 send 仍只能落到 captured 的旧 socket。旧 socket 已被 reset 路径 cancel → `activeTask.send` 抛 `URLError(.cancelled)` → 走 catch 分支跑既有 cleanup（与 round 3 P1 同路径）。

防御层 2（pre-send final 校验）：lock 内再次校验 `sessionGeneration == mySession && underlyingTask === activeTask`。不匹配 = unlock window 内有 reconnect / connect() / disconnect 跑过 → silent skip + cleanup latch。即便防御层 1 万一被未来重构破坏（如某次"优化" 把 `activeTask.send` 改回 `self.send(...)`），本层仍能拦下 stale ping。

before（race 暴露）：
```swift
strongSelf.lock.lock()
guard strongSelf.sessionGeneration == mySession else { strongSelf.lock.unlock(); ...; return }
strongSelf.pendingPongContinuation = pongCont
strongSelf.heartbeatSeq += 1
strongSelf.pendingPongRequestId = "ping_\(seq)"
activeTaskOpt = strongSelf.underlyingTask  // snapshot
strongSelf.lock.unlock()

guard let activeTask = activeTaskOpt, activeTask.isRunning else { ...cleanup; return }

// race window 在 unlock 与下面 send 之间：reconnect 可能跑过把 underlyingTask 换掉
do {
    try await strongSelf.send(.ping(requestId: "ping_\(seq)"))  // 内部 re-read self.underlyingTask
} catch { ... }
```

after（双层防御）：
```swift
// (snapshot 路径同上不变)
strongSelf.lock.unlock()

guard let activeTask = activeTaskOpt, activeTask.isRunning else { ...cleanup; return }

// 测试钩子：仅单测注入 race（production nil → no-op）
if let hook = strongSelf.beforeHeartbeatSendHook {
    await hook()
    if Task.isCancelled { return }
}

// 防御层 2：lock 内 final pre-send 校验
let finalCheckOK: Bool
strongSelf.lock.lock()
finalCheckOK = strongSelf.sessionGeneration == mySession
    && strongSelf.underlyingTask === activeTask
strongSelf.lock.unlock()
if Task.isCancelled || !finalCheckOK {
    strongSelf.lock.lock()
    if strongSelf.sessionGeneration == mySession {
        strongSelf.pendingPongContinuation?.finish()
        strongSelf.pendingPongContinuation = nil
        strongSelf.pendingPongRequestId = nil
    }
    strongSelf.lock.unlock()
    return  // silent skip
}

// 防御层 1：用 captured activeTask 直接 send，绕过 self.send 的 re-read
do {
    let text = try WSMessageCodec.encode(.ping(requestId: "ping_\(seq)"))
    try await activeTask.send(.string(text))
} catch { ...force reconnect (round 3 P1 同路径)... }
```

回归测试 `test_heartbeat_unlockWindowRaceCanceledHeartbeatDoesNotPingNewSocket_round4_P1`：
- 用新增的 `internal var beforeHeartbeatSendHook: (@Sendable () async -> Void)?` 注入 race window 内的精确时序点
- hook 内调 `client.prepareForReconnect()`（cancel 旧 heartbeatTask + finish pongCont + 翻 sessionGen）+ `try? await client.connect(roomId: "RM01")` 让 secondTask 进 underlyingTask
- 单次门控 `SendableBox<Bool>` 防 secondTask 上的新 heartbeat 第二次进 hook 形成死循环
- 用 `WeakRef<WebSocketClientImpl>` 避免 hook closure retain client（与既有 SendableBox 同 unchecked-Sendable + NSLock 模式）
- 核心断言：`secondTask.sentMessages` 不应含 `requestId == "ping_1"` 的 ping —— 修复前旧 heartbeat 的 ping_1 会发到 secondTask；修复后双层防御 silent skip。

顺带改动：`FakeWebSocketTaskHandle` 不需变；`internal var beforeHeartbeatSendHook` 是 production code 上加的测试 hook（与现有 `heartbeatInterval` / `pongTimeout` / `heartbeatSeq` 同 internal 模式，production 永远 nil 等同零成本）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写"持锁块内 snapshot 共享状态 → 解锁 → 后续异步动作"模式时，**必须**直接使用 snapshot 出来的本地常量，**禁止**在异步动作里再走一条会 re-read 同一共享状态的路径；并且**必须**在 act-with-side-effect 之前再加一次 lock 内 final 校验。
>
> **展开**：
> - **lock-snapshot 的契约**：lock 内 read shared state 到本地 const 的目的是"冻结一个一致快照"；解锁后所有"用 snapshot" 的代码必须直接用本地 const，不能调一个会内部 lock + re-read 同一字段的 helper（即便 helper 名字看起来无害，如 `self.send(...)`）。
> - **check-then-act race 的标准修法**：在 act 之前再做一次 lock 内 check，比对 act 依赖的所有共享状态都没变（generation counter、object identity ===、状态字段等）。这一步即便看起来"前面已经 check 过"也不能省 —— unlock window 是 race 的主战场。
> - **多层防御不是过度设计**：当一个 race 修复同时具备"绕过 helper 直接用 snapshot" 与 "act 前重新校验" 两种独立修法时，**两个都加**。任何一层被未来重构破坏（如 reviewer "优化掉" 重复校验，或重构把 `activeTask.send` 改回 `self.send(...)`），另一层仍能兜底。注释里要写明**"为什么这两层都不能去掉"**。
> - **测试 hook 的注入点要精准**：本次的 hook 设在 "lock unlock 之后、final pre-send check 之前"，正是 race window 内部。production 永远 nil 是必须前提（避免 prod 性能损失 / 行为漂移）。`internal var` + `@Sendable async () -> Void` 是 Swift 下最自然的形态（与 `heartbeatInterval` / `pongTimeout` 同模式）。
> - **object identity（===）vs equality（==）**：本次 final check 用 `===` 比 task 引用，因为 `WebSocketTaskHandle` 是 protocol，不能假设 `Equatable`；同时本意就是"这是不是当时 snapshot 出来的同一个 task object"，identity 语义而非 value 语义 —— `===` 是正解。
> - **反例 1（本次踩坑）**：lock 内 snapshot `let activeTask = self.underlyingTask` → unlock → 调 `self.send(...)`，self.send 内部 lock + re-read `self.underlyingTask`。snapshot 等于白做；race window 在 unlock-与-send 之间被悄悄打开。
> - **反例 2**：只加防御层 2（pre-send check）但没改 send 路径仍走 `self.send(...)`。check 通过后 unlock 到 send 之间又有一段 race window，理论仍可命中（虽然窗口更小）。完整修复要 send 也用 captured 引用。
> - **反例 3**：直接在 final check 持锁块**内**调 `activeTask.send(...)`。这会在持锁块内做 IO（违反 lock-discipline 黄金法则"持锁块内不做 IO / await"）；正确做法是先 final check + 在持锁块内决策"过 / 不过"，然后**释放锁后**再用 captured `activeTask` send。
> - **反例 4**：写 silent skip 时不做 latch cleanup —— `pendingPongContinuation` / `pendingPongRequestId` 残留会让下一轮 reconnect 后启动的新 heartbeat 看到 stale latch。silent skip 仍要 cleanup（仅当 `sessionGeneration == mySession` 时 cleanup，避免污染新 session 的 latch）。
>
> **额外提示**：写 "lock-then-async" 模式时，画一张时序图：① lock 内做了什么；② unlock 后到下一个对外副作用动作之间发生什么 race 才会破坏不变量；③ 对外副作用动作依赖什么 invariant；④ invariant 在 unlock 之后还成立吗？任何步骤的回答里只要有"假设它没变"，就要在 act 前补一次 lock 内 final check + 用 snapshot 引用直接 act。

---

## Meta: 本次 review 的宏观教训

Story 12.6 四轮 review 找到 4 个独立 race / failure mode：r1 reconnect 前 state 没 reset → r2 pong requestId 没配对 → r3 ping send error 没 force reconnect → r4 unlock-window race + 用 self.send 而非 captured task。四者都是 detector 实装的"小漏点累积成大事故"。

heartbeat / health-check 这类 multi-step async detector 的 race 几乎一定出现在**步骤切换的接缝**：(a) 共享状态的写入与读取的接缝；(b) lock 与 unlock 的接缝；(c) snapshot 与使用 snapshot 的接缝；(d) detector branch 之间的对称性接缝。每个接缝都需要明确写出 invariant + race window + 防御策略；偷懒一处就埋一颗 race bug。

后续 4 轮加起来体现一个共同的方法论：**不要假设"另一路径会接管" / "snapshot 和 re-read 是同一个值" / "前面已经 check 过就不用再 check"** —— 这些假设在 multi-step async + shared-state-mutation 场景下，几乎全是漏检 silent failure mode 的入口。每一次假设都要被显式证明或被显式防御。
