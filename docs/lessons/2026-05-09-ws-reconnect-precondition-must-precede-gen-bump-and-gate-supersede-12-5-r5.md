---
date: 2026-05-09
source_review: codex review (round 5) — /tmp/epic-loop-review-12-5-r5.md
story: 12-5-自动重连
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-09 — WS reconnect: precondition 必须先于 gen 翻新 & connectGate 覆盖前必 resolve（12-5 r5）

## 背景

Story 12.5（WebSocket 自动重连）round 5 review。前 4 轮陆续修了：(r1) pre-handshake close 分类、(r2) generation counter 隔离 stale task、(r3) connectGate generation 守护、(r4) stream-vs-session 双 generation 解耦。

本轮 codex 又找到两个 generation 边角问题：
1. P1 — `connect(roomId:)` 入口在 precondition 检查**之前**翻 sessionGeneration，导致 token nil / makeWSURL throw 时
   现存活 session 立即被 stale 化，still-open connection 全部 frame silent dropped → wedge.
2. P2 — `connectInternal` 内 install 新 connectGate 直接覆盖旧 gate，被覆盖的 reconnect-attempt 的
   `withCheckedThrowingContinuation` 永远不被 resume → suspended forever（task 泄漏）.

两条都是真问题，都已修。同时本轮整理出 WebSocketClientImpl generation invariants 全景图（Meta 节）让
未来 review 看清楚哪些路径必须翻 gen、哪些必须 generation-gated、哪些是 unconditional.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `connect(roomId:)` precondition 必须先于 `sessionGeneration += 1` | high (P1) | architecture | fix | `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:242-256` |
| 2 | install 新 `connectGate` 前必须 resolve 旧 gate（unconditional） | medium (P2) | architecture | fix | `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:307-336` |

---

## Lesson 1: connect 入口的 precondition 检查必须先于 sessionGeneration 翻新

- **Severity**: high (P1)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:242-256`

### 症状（Symptom）

`connect(roomId:)` 旧实装在入口直接 `sessionGeneration += 1`。若紧随的 `connectInternal` 因 token nil/空
或 `makeWSURL` throw 而早退（room switch 时 auth 暂时不可用是典型场景），现存活的 socket 仍在物理 receive，
但其 receive-loop 持有的 `mySession` 已 < `sessionGeneration` → 后续 frame 走 `yieldIfCurrent` /
`emitConnectionStateIfCurrent` 全部因 stale-session 检查被 silent drop。**连接物理上还活，逻辑上已 wedged**，
必须等外部显式 disconnect 才能恢复 — 用户视角 "莫名其妙不再收消息"。

触发场景（review 钦定）：用户在 RoomA 期间触发 room switch（试图 connect RoomB），auth 暂时不可用使
tokenProvider 返回 nil → connect("RoomB") throws；但 RoomA 的连接也死了。

### 根因（Root cause）

generation counter 的设计意图是 "只有当前活跃 session 的 task 能 mutate 共享状态"。但旧实装把 "翻新 generation"
当成 "进入 connect 仪式" 的第一步，把 "状态 displace 时刻" 与 "最早可能 throw 的入口" 混淆 —— 没意识到
preconditions 本身可以失败。规则应是：**"先验证可能 throw 的所有 preconditions，preconditions 都 OK 才翻
generation"**；这样 throw 路径上 generation 与共享状态保持一致，不会 invalidate 任何活的 session。

### 修复（Fix）

把 `connect(roomId:)` 改成 "preconditions 先 dry-run，过了才进 displace-session 块"：

```swift
public func connect(roomId: String) async throws {
    // preconditions 先 dry-run（throw 路径不翻 gen）
    guard let token = tokenProvider(), !token.isEmpty else {
        throw WSError.tokenMissing
    }
    _ = try makeWSURL(roomId: roomId, token: token)  // throw → 不翻 gen

    // preconditions OK → 真正 displace
    lock.lock()
    let oldReconnectTask = reconnectTask
    reconnectTask = nil
    reconnectAttempt = 0
    sessionGeneration += 1
    lock.unlock()
    oldReconnectTask?.cancel()

    try await connectInternal(roomId: roomId, isReconnect: false, attemptNumber: 0)
}
```

`connectInternal` 内部仍会重做 token + URL 检查（reconnect path 直接走那条），代码重复但语义清晰。**未在
connectInternal 入口翻 gen** — `attemptReconnect` 已经在 reconnect session 里运行，连续翻 gen 会破坏
reconnect 闭环。

新增单测 `test_connect_tokenNilDoesNotInvalidateLiveSession_round5_P1`：
- 首次 connect RM01 成功 + 收到 snapshot
- 切到 nil token + 调 connect("RM02") 期望 throws
- 然后 firstTask 推一帧（member.left）— 验证 stream 仍能 yield 第二条消息
- 旧实装在第二步翻 gen 后，第二帧被 silent drop → 测试只收到 1 条；修复后收到 2 条

### 预防规则（Rule for future Claude）

> **一句话**：**generation counter 翻新必须放在所有可能 throw 的 preconditions 之后** ——
> 把 "翻 generation" 当成 "displace session 的 commit 时刻"，不是 "进入仪式的第一步"。

> **展开**：
> - 任何提前 throw 路径都必须保持 generation 与共享状态一致：throw 后 `mySession`（旧 receive-loop /
>   reconnect-attempt 持有的）必须仍 == `sessionGeneration`，否则现存活 session 的所有 generation-gated
>   写入会被 silent drop → 连接物理活、逻辑死.
> - **顺序模板**：① 取 token / 验证 input（throw → 不翻 gen） → ② 调 makeURL / parser（throw → 不翻 gen）
>   → ③ 持锁 cancel 旧 reconnect + 翻 gen + 重置 attempt → ④ 调 internal helper.
> - 设计 generation invariant 时**必须**列出 "翻新触点" 清单，并 review 每个触点之前是否还有 throw 路径；
>   有 → 把那段移到触点之前.
> - 如果 internal helper 已经做了 token / URL 检查（reconnect path 复用），caller 路径也**必须**重做这两步
>   作为 dry-run（代码重复 OK，语义清晰）；不能依赖 helper "throw 时还原 gen" 这种隐性约束（容易后续被破坏）.
> - **反例**：直接在 public entry 第一行 `sessionGeneration += 1`，理由 "反正 helper 也会重做检查";
>   反例的失效点：helper throw 时已经 invalidate 了所有 mySession-持有者，shared state 与 gen 错位.

---

## Lesson 2: install 新 CheckedContinuation 字段前，必须先 resolve 旧的（unconditional）

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:307-336`

### 症状（Symptom）

`connectInternal` 在 `withCheckedThrowingContinuation` 闭包内做 `connectGate = cont`，覆盖任何已有
`connectGate` 而不 resume 旧 continuation。当 caller-driven `connect()` 与已在 `connectInternal` 内部
await 的 reconnect-attempt 竞争时：旧 reconnect 的 `connectGate` 被覆盖，旧 gate 后续在 `resolveConnectGate(...)`
入口因 `connectGateOwnerSession != mySession` silent drop → 旧 `withCheckedThrowingContinuation` 永远
不被 resume → reconnect 的 connectInternal await **永久 suspend**（task 泄漏 + client/task graph 残留）。

### 根因（Root cause）

`CheckedContinuation` 字段是 "排他独占" 资源 — 同一字段在生命周期内只能 resume 一次，不能被覆盖。把它当
普通 var 直接 `=` 是把"排他资源"按"普通字段"语义处理。round 3 已经把 connectGate 加上 generation 守护，
让 stale resolver 不污染新 owner；但**没考虑反向**：新 owner 上任时如何处理被取代的旧 owner。
generation 守护只保证 "stale resolver silent drop"，不保证 "stale awaiter 被唤醒"。后者是覆盖语义本身的
责任 — 必须显式 resume 旧 continuation 才能让旧 awaiter 解套。

### 修复（Fix）

`connectInternal` 在 install 新 gate 之前显式 resolve 旧 gate（如有），走 unconditional 路径
（caller-driven 显式想 supersede，不需要 generation 校验）：

```swift
try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, Error>) in
    // 先 resolve 旧 gate（如果存在），让旧 awaiter throw 而非 hang.
    let staleGate: CheckedContinuation<Void, Error>?
    lock.lock()
    staleGate = connectGate
    connectGate = nil
    connectGateOwnerSession = nil
    lock.unlock()
    staleGate?.resume(throwing: WSError.connectionFailed(
        underlyingDescription: "superseded by new connect attempt"
    ))

    // 然后 install 自己的 gate.
    lock.lock()
    connectGate = cont
    connectGateOwnerSession = mySession
    lock.unlock()
    startReceiveLoop(...)
}
```

新增单测 `test_connect_supersededInflightGateMustBeResolved_round5_P2`：
- 首个 connect 在 firstTask 永远阻塞 receive 时 stuck 在 connectGate await
- 第二个 connect 同步 install 新 gate（修复前覆盖；修复后 resolve）
- 验证旧 connect Task 在 2s 内拿到 thrown error（修复前会 timeout / hang）

### 预防规则（Rule for future Claude）

> **一句话**：**任何 `CheckedContinuation?` 字段在被新 continuation 赋值之前，必须先 resolve 旧 continuation**
> （走 unconditional resume 路径），永不直接覆盖.

> **展开**：
> - `CheckedContinuation` 是排他资源 — 一旦被丢弃且没 resume，其 awaiter 永久 suspend（且 release-build 下
>   可能 fatal "leaked"）. 与普通 var 字段的 "覆盖" 语义不同.
> - 守护 / 隔离机制（generation 校验、ownership token 等）**只解决 "stale resolver 不污染新 awaiter"**；
>   反向问题 "新 owner 上任时旧 awaiter 怎么办" 必须由覆盖点显式处理 — resume 旧 continuation 让 awaiter
>   throw 出来，**不能**依赖 stale resolver 兜底（stale resolver 大概率被守护 silent drop，旧 awaiter
>   就 hang 了）.
> - resolve 旧 gate 用 unconditional 路径（caller 显式 supersede 旧 owner，不需要 generation 校验）;
>   resolve 用语义清晰的 error（"superseded by new connect attempt"），让 caller / log 可分辨这种取代型失败
>   与真实握手失败.
> - **反例 1**：`field = newCont` 直接覆盖 — 旧 awaiter hang.
> - **反例 2**：依赖 "下次 stale resolver 触发时会 resume" — 但 stale resolver 走的是带 generation 守护的
>   `resolveConnectGate(mySession:)`，它会因 owner 不匹配 silent drop，不会 resume 旧 awaiter.
> - **反例 3**：覆盖前 resolve 走 generation-gated 路径（`resolveConnectGate(mySession:)`） —
>   但本场景下 ownerSession 已经被新 caller 即将翻新，旧 mySession 没机会注入；走 unconditional 是唯一对的.

---

## Meta: WebSocketClientImpl generation invariants 全景图

经过 5 轮 review，本类的 generation 模式已达 **2 个 generation counter + 4 类访问语义** 的成熟度。
为防 round 6 再被找到角落问题，把 invariants 落到这里：

### Generation 字段

| 字段 | 翻新触点 | 用途 |
|---|---|---|
| `sessionGeneration` | `connect()` precondition pass 后 / `prepareForReconnect()` / `disconnect()` | 区分 "哪个 session 的 task" |
| `streamGeneration` | `init` / `prepareForReconnect()`（即 `makeStream()` 调用处） | 区分 "哪个 stream 的 owner"（stream 复用 vs swap） |
| `connectGateOwnerSession` | `connectInternal` install gate 时 | 与 `connectGate` 配对的 owner session |

### 路径分类

**A. 必须翻 sessionGeneration**（"换 session" 的语义点）：
- `connect(roomId:)`（precondition pass 之后）
- `prepareForReconnect()`
- `disconnect()`

**B. 必须 generation-gated（mySession 校验）写入**（防 stale task 污染新 session）：
- `yieldIfCurrent(_, to:, mySession:, myStreamGen:)` — 双 gate（streamGen + sessionGen）
- `emitConnectionStateIfCurrent(_:mySession:)` — sessionGen
- `finishStreamIfCurrent(_:mySession:myStreamGen:)` — streamGen 主导（swap 路径必 finish），
  stream 复用时 sessionGen 守护
- `scheduleReconnectIfCurrent(mySession:)` — sessionGen
- `attemptReconnect` 入口 / catch / success 路径 — sessionGen 多重校验
- `resolveConnectGate(success:error:mySession:)` — `connectGateOwnerSession == mySession` 校验

**C. unconditional resolve 路径**（caller 显式 supersede / 拆除 in-flight connect）：
- `disconnect()` → `resolveConnectGateUnconditionally(...)`（caller 主动放弃 in-flight connect）
- `prepareForReconnect()` → `resolveConnectGateUnconditionally(...)`（同上）
- `deinit` → 内联 unconditional resume（防 dangling continuation）
- **本轮新增**: `connectInternal` 安装新 gate 前 → `staleGate?.resume(throwing:)`
  （新 owner 上任，supersede 旧 owner）

**D. 非 generation-gated 但仍正确**（依赖其他不变量）：
- `send(_:)` — 用 `underlyingTask.isRunning` 单独守护（不依赖 generation）
- `tokenProvider()` / `makeWSURL` — 纯函数，无共享状态写入

### 6 大不变量

1. **Inv-1（precondition before bump）**: `sessionGeneration += 1` 必须发生在所有可能 throw 的 preconditions 之后
   （round 5 P1）.
2. **Inv-2（gate ownership）**: `connectGate` 与 `connectGateOwnerSession` 必须**同步** install / 同步 clear；
   永不出现 `connectGate != nil && connectGateOwnerSession == nil`（或反之）.
3. **Inv-3（gate supersede resolves before overwrite）**: 任何 `connectGate = newCont` 之前必须先 resolve
   旧 cont（若 != nil），走 unconditional 路径（round 5 P2）.
4. **Inv-4（stale write silent-drop）**: 任何 launched async task 的写共享状态前必须 generation-gate；
   stale silent drop 不抛错（防污染新 session 的状态序列）.
5. **Inv-5（stream finish swap path）**: 当 streamGeneration 翻新时（仅 prepareForReconnect），旧 receive-loop
   的 defer 必须**仍** finish 旧 continuation（让旧 consumer for-await 退出）— streamGen swap 比 sessionGen
   守护优先级高（round 4）.
6. **Inv-6（reconnect 不翻 sessionGeneration）**: `attemptReconnect` 调 `connectInternal` 时**不**翻 gen ——
   这是同一 session 内的"延续"，不是"换 session"；翻 gen 会让自己 stale-out.

### 触发未来 review 关注点的 trigger words

- 添加新的 entry-point 函数（公开 API） → 检查 Inv-1（preconditions 必须先于 gen bump）
- 任何 `*?: CheckedContinuation` 字段被赋值 → 检查 Inv-3（覆盖前必 resolve）
- 任何新增 launched async task → 检查 Inv-4（mySession 校验）+ Inv-2（gate ownership 同步）
- 任何 makeStream / 新 currentContinuation 创建 → 检查 Inv-5（streamGen 翻新 + 兼容旧 consumer 退出）

如果这些不变量后续再被破坏（round 6+），可以引入 actor 把 mutable state 全包起来 + per-action ownership token,
但当前 NSLock 模式 + 双 generation counter 已能覆盖绝大多数 race —— 见以上不变量清单.
