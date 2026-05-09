---
date: 2026-05-09
source_review: codex review (round 4) — Story 12.5 reconnect 状态机
story: 12-5-自动重连
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-09 — WS reconnect: stream 复用 vs session 翻新必须双 generation 解耦（Story 12.5 r4）

## 背景

r2 已经引入 `sessionGeneration`，让 `connect` / `prepareForReconnect` / `disconnect` 三类 caller 在持锁内
`+1` 翻新；所有 launched async task launch 时抓 `mySession`，写共享状态前 `mySession == sessionGeneration`
不匹配 silent drop。r3 把 `connectGate` 也纳入此 generation 框架。看似已经覆盖所有共享状态入口。

r4 codex review 找到 r2 留下的**第二条未被 audit 的共享状态入口**：`continuation.yield(...)` 与 `continuation.finish()`。
这两条直写共享 stream 的路径，r2 当时没收口到 `*IfCurrent` 包装里，导致：

- **race #1**：`connect(roomId:)` 在已 connected client 上被调用时 —— 不调 prepareForReconnect → 复用现存
  `currentContinuation`，但 `sessionGeneration += 1` 在 cancel 旧 receiveTask 之前就已翻；旧 receive-task
  cancel 后落入 defer 块，调 `continuation.finish()` 终结**新 session 复用的同一个 stream** → 新连接的 vm
  立即收不到任何消息。
- **race #2**：旧 receive-loop 在 cancel 信号传播之前已 dequeue 了一条旧房间的 frame；翻 gen 后旧 task 仍 yield
  那条 frame 到现在被新 session 复用的 stream → 旧房间的 `room.snapshot` / `member.*` 漏到新连接 →
  session-isolation 破洞。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | stale receive-task defer 跑 `continuation.finish()` 终结新 session 复用的 stream | P2 | architecture | fix | `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift` |
| 2 | stale receive-task `continuation.yield(frame)` 漏旧房间 traffic 到新 session 的 stream | P2 | architecture | fix | `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift` |

## Lesson 1: generation 模式必须区分"session 翻新"与"stream 翻新"两个独立维度

- **Severity**: P2 / medium
- **Category**: architecture (concurrency)
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift`
  - `WebSocketClientImpl.startReceiveLoop` defer 内 `continuation.finish()` + 三条 yield 路径
  - 新增 helper：`yieldIfCurrent(_:to:mySession:myStreamGen:)` / `finishStreamIfCurrent(_:mySession:myStreamGen:)`
  - 新增字段：`streamGeneration`（与 `sessionGeneration` 解耦）

### 症状（Symptom）

第一次本能反应是「上一轮已经把 `sessionGeneration` 加在 `connect` / `prepareForReconnect` / `disconnect`
三处，所有写共享状态都校验 mySession 就够了」。但实际 review #1 / #2 命中两条全新 race：

1. 旧 receive-task 的 `defer` 块跑 `continuation.finish()` —— 不在 r2 列出的"三条共享状态写入路径"里，
   因为它语义上是"task lifecycle cleanup"，不是"session-scoped state mutation"。
2. 旧 receive-task 的 frame yield 路径同样不在 r2 audit 里。

如果天真用单字段 `sessionGeneration` 给 yield/finish 加 gate，会立刻撞上**新的 race**：

- `connect("RM01")` 内 await gate → first frame 到达 → resolve gate → caller 醒来 chain `connect("ROOM_B")`
  翻 gen → 与此同时旧 receive-task 还在 process first frame、还没跑 yield(RM01 snapshot) →
  yield-gate 校验 `mySession=N+1 != sessionGeneration=N+2` → silent drop → **RM01 的合法 first frame 被错误吞掉**。

这条新 race 不是 review 直接说的，但是单字段 generation 的副作用：first-frame yield 与 caller 翻 gen 的时序不可逆，
yield 必然有概率落在翻 gen 之后。

### 根因（Root cause）

**两个语义混在一个字段里**：

- `sessionGeneration` 在每次 `connect` / `prepareForReconnect` / `disconnect` 翻 —— 它真正想表达的是
  "哪个 caller-driven session 是当前合法的"。
- 但其中 **`connect` 不一定换 stream**（已 connected client 复用现存 `currentStream` / `currentContinuation`）。
  所以 sessionGeneration 翻新**不**等价于 stream 换了。
- yield/finish 的 race 真正的问题是「我手里这个 continuation 是不是孤儿（已被 swap）/ 还是被其它 session 复用」，
  这是**stream-scoped 维度**，不是 session-scoped 维度。

单字段 generation 无法同时表达两个维度：

- 用 sessionGeneration 给 yield 加 gate → first-frame yield 被误杀（上文 race）
- 不加 gate → review #2 的 stale frame 漏到新 stream（不修）

### 修复（Fix）

引入**第二个 generation 字段** `streamGeneration`：

- **递增点**：仅在 `makeStream()` 调用处（init / `prepareForReconnect()`），sessionGeneration 翻新但 stream
  没换的路径（已 connected client 上 chain 调 `connect`）**不**翻 streamGeneration。
- **捕获点**：receive-loop launch 时同步抓 `myStreamGen`（与 `mySession` 平级 closure 参数）。
- **校验点**：yield/finish 包装内做**两层 gate**：
  - 第 1 层 stream-owner：`myStreamGen != streamGeneration` → 我手持的 continuation 是孤儿（stream 已被 swap）。
    yield 路径 silent drop（孤儿 stream 没人读）；finish 路径仍允许（让旧 stream consumer 的 for-await 退出）。
  - 第 2 层 session：`myStreamGen == streamGeneration` 但 `mySession != sessionGeneration` → stream 还是
    receive-loop launch 时那个但 session 已被 chain-connect 翻新，本 task 是 stale。yield/finish 都 silent drop。

```swift
// 新增字段
private var streamGeneration: Int = 0  // 仅 makeStream() 时 +1

// connectInternal 内捕获
myStreamGen = streamGeneration

// helper 双层 gate
private func yieldIfCurrent(
    _ message: WSMessage,
    to continuation: AsyncStream<WSMessage>.Continuation,
    mySession: Int,
    myStreamGen: Int
) {
    // 第 1 层：stream 已 swap → 孤儿 silent drop
    if streamGeneration != myStreamGen { return }
    // 第 2 层：stream 复用但 session 翻新 → stale silent drop
    if sessionGeneration != mySession { return }
    continuation.yield(message)
}

private func finishStreamIfCurrent(
    _ continuation: AsyncStream<WSMessage>.Continuation,
    mySession: Int,
    myStreamGen: Int
) {
    if streamGeneration != myStreamGen {
        // stream 已 swap → 必须 finish 让旧 consumer for-await 退出（不 race 新 stream）
        continuation.finish()
        return
    }
    if sessionGeneration != mySession { return }  // stale session 复用 stream → silent skip
    continuation.finish()
}
```

回归测试两条：

- `test_reconnect_staleReceiveTaskDeferFinishDoesNotTerminateReusedStream`：复现 race #1，验证修复后 stream alive
- `test_reconnect_staleReceiveLoopAfterConnectReplaceLeavesNewStreamCleanForNewSnapshot`：复现 race #2，验证最后一条 snapshot 是新 session 的（不被 stale yield 污染）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **给一类共享 mutable state 引入 generation counter 隔离 stale task 时**，
> **必须**先**列出该资源的所有"翻新点"和"读取/写入点"两个集合**，并问"翻新点是否对应同一个语义维度";
> 如果**写入点的语义维度比翻新点的语义维度更细**，说明需要**多个独立 generation 字段**。

> **展开**：
>
> - 一个 generation 字段只能刻画**一个**语义维度。如果 caller 操作触发的"重置语义"有两套不同粒度（如本 case
>   的"session 翻新"vs"stream 翻新"），单字段 generation 无法精确表达，必然在某条 race 里产生误杀或漏杀。
> - 引入 generation counter 时的 audit 流程：
>   1. 列出所有 caller-triggered 翻新点（递增字段的位置）
>   2. 列出所有 launched async task 内会写共享状态的位置（包括 `defer` / `catch` / cleanup 路径，**不只是
>      "正常路径"**）
>   3. 把 (2) 按"我希望 stale task 继续执行还是 silent drop" 给每条路径分类
>   4. 检查 (3) 的分类是否能用 (1) 翻新点的单字段 generation 精确实现 —— **如果发现某条 happy-path 写入会被
>      误杀（"我希望 stale 但合法的 first-frame yield 通过"），就要拆分 generation**
>
> - **反例 #1**：用单字段 generation 给 stream yield 加 gate；first-frame yield 与 caller 翻 gen 的
>   scheduler race 必然出现 → 合法 first frame 被误杀。
> - **反例 #2**：在 fix-review 中只 audit reviewer 列出的"三条共享状态路径"（如本 case r2 的
>   `currentContinuation` / `scheduleReconnect` / `attemptReconnect`），不主动扫描 receive-loop 的 `defer` /
>   普通 yield 路径 —— 那些不在 reviewer 列表里的入口同样可能跨 generation 写共享状态（review r4 命中的就是
>   这种漏网入口）。
>
> - **正例**：本 case 的 `streamGeneration` —— 仅在 `makeStream()` 翻新；`sessionGeneration` 在三处 caller 翻新；
>   两个字段独立递增、独立捕获、独立校验；yield/finish 用两层 gate 区分孤儿（stream 已 swap）vs stale session
>   （stream 复用但 session 翻新）。

> **次级规则（每次 fix-review 引入 generation 时的 checklist）**：
>
> - 写一行注释明确**所有递增点**（如 r2 注释里的「**递增点**：每次 `connect` / `prepareForReconnect` / `disconnect`」）
> - 写一行注释明确**所有读取/校验点**（如 r2 注释里的「**校验点**：任何写 ... 之前先校验 ...」）
> - 然后**反向 audit**：扫一遍代码里所有写共享状态的位置，每一条都问"这条路径在我列的校验点里吗？"
>   不在的话立刻补上或解释为什么不需要。
> - **`defer` 块和 `catch` 块都算 "task 内的写入点"**，不能因为它们语义是"cleanup"就跳过 audit。

---

## Meta: 本次 review 的宏观教训（可选）

12.5 走到 r4 已经是同一 reconnect 状态机的**第四轮 fix-review**。每一轮都在补 r(n-1) 漏掉的"小入口"：

- r1：pre-handshake terminal close 分类 + connection-state stream 守护
- r2：sessionGeneration 隔离 stale task 三大共享状态
- r3：connectGate 也要 generation 守护（r2 漏掉的 one-shot 同步原语）
- r4：yield / finish 也要 generation 守护（r2 漏掉的 stream-scoped 写入点）

宏观教训：**每次引入新的 generation 模式时，第一轮就要做完整的"写共享状态点"反向 audit，而不是只覆盖 reviewer
当时点出的几条**。reviewer 不点全是必然的（reviewer 看的是 patch diff）；防御性 audit 是 implementer 的职责。

具体到 reconnect/stream 这类长寿命 async-task + 共享 mutable state 的代码：

- 列出所有"长寿命 task launch"位置（receive-loop / scheduleReconnect / attemptReconnect）
- 对每个 task，扫它**整个 closure 体**（包括 `defer` / `catch` / 嵌套闭包）的所有 mutable state 写入
- 每条写入路径都要明示"我是当前 session 才允许执行还是无条件执行"
- 无条件执行的需要解释（如 finish 孤儿 stream 让旧 consumer 退出 —— 这是合理的无条件操作）；
  条件执行的必须有 generation gate
