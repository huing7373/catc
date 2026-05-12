---
date: 2026-05-13
source_review: codex review r4 on Story 15.2 (/tmp/epic-loop-review-15-2-r4.md)
story: 15-2-pet-state-changed-ws-消息处理
commit: 0184339
lesson_count: 1
---

# Review Lessons — 2026-05-13 — Consumer 启动必须原子捕获 (stream, generation) —— 分两步读会被 prepareForReconnect 撕成"新 stream + 旧 gen"，让新 stream 所有消息被错误识别为 stale 丢弃（15-2 r4）

## 背景

Story 15.2 r1 → r2 → r3 迭代收敛了"per-stream generation gate"（`memberJoined` / `memberLeft` / `petStateChanged` / `connectionStateChanged` / `roomSnapshot` 都用 `streamGeneration == client.streamGeneration` 校验防 same-room rejoin race）。codex r4 接着 flag 了**消费方的启动接缝**本身有 race：`RealRoomViewModel.startConsumingMessages()` 分两步读 `let streamGeneration = client.streamGeneration` + `for await message in client.messages`，两步之间若 `prepareForReconnect()` 发生 → 新 Task 订阅 NEW stream 却携带 OLD generation → handle 把新 stream 上所有消息当 stale 全部丢弃。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | startConsumingMessages 分两步读 stream / generation 留下 race（codex r4 P1） | high | architecture | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:552-554` |

## Lesson 1: Consumer 启动接缝必须原子捕获 (stream, generation)

- **Severity**: high
- **Category**: architecture / concurrency
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:552-554`（修复前）

### 症状（Symptom）

`prepareForReconnect()` 恰好在 `let streamGeneration = client.streamGeneration` 之后、`for await message in client.messages` 之前发生（A→B 快速切换 / same-room rejoin / 心跳超时 reconnect 都可触发）。新建的 consumer Task 订阅**新的** AsyncStream，却继续携带**旧的** generation 值。随后 `handle(..., streamGeneration:)` 把新 stream 上的所有消息都识别为 stale 丢掉；房间更新卡死直到下一次 restart consumer。更糟：被 cancel 的旧 Task 也可能先挂到这个新 stream 上，把帧消费掉后再丢弃。

### 根因（Root cause）

把"per-stream identity"（stream 实例 + generation counter）当成两个独立可分别读取的字段。protocol 层把 `messages` 和 `streamGeneration` 设成两个独立 getter，看起来各自线程安全（每个 getter 内部都持锁），但**两次 getter 之间没有任何互斥** —— 在并发 / 异步重置语义下，"读两次 + 期望读到同一代"在协议设计层就不成立。

正确的协议契约是：把"per-task identity"作为**不可分割的快照**暴露 —— `currentStreamSnapshot: (stream, generation)`，由实装内部在同一临界区内原子读取两个字段返回。

### 修复（Fix）

**Plan A**：protocol 层加原子快照 getter；Impl 层 lock 内同时读两字段；Mock 层在单线程语义下显式实现保持契约对齐；VM 调用方改用单次 snapshot 读。

- `WebSocketClient.swift`（protocol）：新增
  ```swift
  var currentStreamSnapshot: (stream: AsyncStream<WSMessage>, generation: Int) { get }
  ```
  并在 protocol extension 提供非原子默认实现作为编译期兜底（不推荐生产用）。

- `WebSocketClientImpl.swift`：override，**同一个 `lock.lock()`/`lock.unlock()` 临界区**内同时读 `currentStream` 与 `streamGenerationStorage`，让 `prepareForReconnect()`（已持同一锁写两个字段）与本读路径严格互斥。

- `WebSocketClientMock.swift`：override（mock 单线程语义下两字段写在同一方法体，单次方法调用一次读取即可同时返回；显式实现以对齐 protocol 契约 + 测试时可作单一接缝点）。

- `RealRoomViewModel.startConsumingMessages`：把
  ```swift
  let streamGeneration = client.streamGeneration
  messageConsumerTask = Task { ... for await message in client.messages { ... } }
  ```
  改为
  ```swift
  let snapshot = client.currentStreamSnapshot
  let stream = snapshot.stream
  let streamGeneration = snapshot.generation
  messageConsumerTask = Task { ... for await message in stream { ... } }
  ```
  注意 for-await 订阅的是 `snapshot.stream`（**不**再是 `client.messages`），避免 Task 启动后 swap 也会跟着切到新 stream。

- 测试新增（`RealRoomViewModelTests.swift`）：
  1. `testCurrentStreamSnapshotReturnsMatchingPair` —— 验证 protocol 契约："snapshot 拿到 (s0, g0) 之后 prepareForReconnect → 第二次 snapshot 拿到 (s1, g1)，且 g1 == g0 + 1；snapshot0.generation 不会被后续 prepare 污染"。
  2. `testStartConsumingMessagesAfterPrepareForReconnectStillReceivesMessages` —— 端到端正向路径回归守护，确保修复后 prepareForReconnect → 新 gen 的 snapshot 仍能正常被 handle apply。
- 既有 3 个 generation-gate 测试（pet.state.changed / 4 case / .roomSnapshot）保持不变，全部 590 tests green。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **暴露一组"必须配对使用"的字段（stream + generation / lease + token / version + payload 等）** 时，**必须** **把它们打包成原子 snapshot getter，禁止留两个独立 getter 让 caller 分两步读**。
>
> **展开**：
> - 协议层标准是：caller 一次 getter 拿到的 tuple / struct 在 caller 后续使用过程中是**自洽的不可变快照**；实装内部用一个临界区读所有字段。
> - 反过来：**任何**"读字段 A → 中间发生外部 mutate → 读字段 B"的两段式访问，只要 A / B 在语义上有"必须同代"要求，就一定有 race。锁住每个 getter 不解决问题，问题在两次 getter 之间。
> - consumer 启动这类 fire-and-then-forget 异步任务（`Task { for await ... in stream }`）尤其危险：Task 体在另一个 actor / 调度点跑，**Task 启动语句**和 **Task 体首条语句**之间天然有时间窗口，足够任何并发 mutate 介入。
> - 不要被"AsyncStream / Int Getter 各自看起来线程安全"误导 —— 单字段线程安全 ≠ 多字段组合线程安全。
> - **反例**：
>   - `let gen = client.gen; Task { for await m in client.stream { handle(m, gen) } }` —— Task 启动前后两次访问 client，gen 和 stream 不配对。
>   - `let leaseId = mgr.currentLeaseId; let token = mgr.currentToken` —— 后续校验 (leaseId, token) 是否成对生效会因 mgr 中途轮换而错配。
>   - `let version = store.version; let payload = store.payload` —— 中途 store mutate 后 payload 已是 v2，但 version 还是 v1，写回 server 会被乐观锁拒。

### 元规则补充

generation gate 这条线已迭代 r1 → r2 → r3 → r4 共四轮：
  - r1：identified 同房间 rejoin race，初判定为 defer（错）
  - r2：决策反转 fix，引入 streamGeneration 字段
  - r3：纠正 r2 漏盖 `.roomSnapshot` —— 所有 state-mutating message 必须统一过 gate
  - r4（本轮）：纠正 r2/r3 漏盖**消费侧启动接缝** —— gate 字段本身要原子读

  教训：**当一条"防 stale 守护"的设计引入一个新的同步字段（generation）时，至少要审查三类接缝**：
  1. 字段的所有 mutate 点是否都翻新（覆盖所有 swap 路径）—— r2 解决
  2. 字段守护的所有 message 类型是否都加 gate（覆盖所有 state-mutating event）—— r3 解决
  3. 字段在 consumer 端如何被**捕获**（读取时机 / 是否与"被守护对象"配对原子）—— r4 解决

  下次引入类似"identity counter + 关联对象"的设计模式（如 epoch / lease / fence token + payload）时直接照单全收这三类审查点，省一轮 review。
