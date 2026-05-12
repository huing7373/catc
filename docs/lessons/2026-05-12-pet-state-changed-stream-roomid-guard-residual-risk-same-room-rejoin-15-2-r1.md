---
date: 2026-05-12
source_review: codex review (round 1) — /tmp/epic-loop-review-15-2-r1.md（末尾 ^codex$ 段，line 末段）
story: 15-2-pet-state-changed-ws-消息处理
commit: 9ea7e13
lesson_count: 1
---

# Review Lessons — 2026-05-12 — `pet.state.changed` streamRoomId 守护在 same-room rejoin / same-room reconnect 路径下的残余风险（defer 至 Story 15.5 + 跨 4 case 统一重设计）

## 背景

Story 15.2 给 `RealRoomViewModel.handle(message:streamRoomId:)` 新增了 `.petStateChanged` 分支，沿用 Story 12.4 r1 落地的 `streamRoomId != nil, streamRoomId == lastObservedRoomId` 守护（与 `.memberJoined` / `.memberLeft` / `.connectionStateChanged` 同模式）。codex 在 round 1 指出该守护**在 same-room rejoin / same-room reconnect 路径下失效**：

- 路径 1（same-room leave-rejoin）：A → leave → rejoin **同 A**。旧 stream 启动时 `streamRoomId = A`，新 stream 启动时 `streamRoomId = A`；`lastObservedRoomId` 在 leave 阶段经过 `nil` 后又回到 `A`。旧 stream late `.petStateChanged` 在 cancel 前被 main-actor 投递时 `streamRoomId == lastObservedRoomId == A` → 守护放行 → 旧值覆盖新 stream 已建立的 roster.
- 路径 2（same-room reconnect）：WS 断 → 自动重连同 roomId A。`prepareForReconnect()` swap stream 但 vm 端 `lastObservedRoomId` 不变（仍 A）；新旧两个 stream 的 `streamRoomId` 都是 A → 守护无差别化能力.

codex 建议：用 per-stream generation / identity（如 stream UUID / 单调递增 stream seq）替代单纯的 roomId 比较.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | streamRoomId 守护在 same-room rejoin / same-room reconnect 路径下失效（pet.state.changed 旧 stream late message 仍能覆盖 roster） | P2 | architecture (concurrency) | **defer** | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:644-652`（同坑也在 line 617-624 memberJoined / 626-636 memberLeft / 666-672 connectionStateChanged） |

## Lesson 1: streamRoomId 守护在 same-room rejoin / same-room reconnect 路径下失效 —— defer 至 Story 15.5 + 跨 4 case 统一重设计

- **Severity**: P2
- **Category**: architecture (concurrency)
- **分诊**: **defer**
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:644-652` （`.petStateChanged`），同模式在 :617-624 / :626-636 / :666-672

### 症状（Symptom）

`streamRoomId == lastObservedRoomId` 守护设计假设：跨 room 切换（A → B）时 streamRoomId 与 lastObservedRoomId 必有一侧已翻新 → 不匹配 → drop。但**两种 same-room 路径**绕过此假设：

1. **same-room leave-rejoin**（A → nil → A'，A' 与 A 是同一个 roomId 字符串）：用户离开房间后再次加入同房间，`subscribeRoomIdConnect` 的 nil → A' 分支会 `prepareForReconnect + startConsumingMessages`，新 task 捕获 streamRoomId = A。旧 stream 的 `streamRoomId` 也是 A。late delivery 通过守护.
2. **same-room reconnect**（WS 断 → 自动重连原 roomId A）：`WebSocketClientImpl` 内部 `prepareForReconnect()` 翻 streamGeneration + 重建 stream，但 vm 端 `messageConsumerTask` 是绑定 `client.messages` AsyncStream 的（不是绑定底层 underlying task），且 vm 端 `lastObservedRoomId` 不会被翻动。新旧 stream 在 vm 视角下都对应 streamRoomId = A.

具体后果：旧 stream 上一条 stale `.petStateChanged { userId: u_alice, currentState: 1 (rest) }` 在 same-room rejoin 后被 main-actor 投递 → 覆盖新 stream `room.snapshot` 已建立的 `{ userId: u_alice, currentState: 2 (walk) }` → UI 显示错误状态，**直到下一次 pet.state.changed 或显式 snapshot 重拉才自愈**.

### 根因（Root cause）

12.4 r1 lesson 钦定 streamRoomId 守护是基于"跨 room 切换"的核心场景（`A → B` 时 streamRoomId 与 lastObservedRoomId 必有一侧不同）。当时**未充分枚举** same-room 边界路径：

- same-room leave-rejoin 因为中间经过 `nil` 状态，旧 task 应该在 `nil` 那一瞬间被 cancel + finish；理论上不会有 late delivery。但 cancel 非同步 + `await MainActor.run` 有 in-flight 窗口 → race 仍存在.
- same-room reconnect 是 WS layer 内部 swap，vm 完全不感知；vm 的 streamRoomId 只跟随 `subscribeRoomIdConnect` 的 sink 翻动（roomId 字段变化），不跟随底层 stream lifecycle.

protocol 层 V1 §12.3 钦定 `pet.state.changed` payload 只有 `userId / petId / currentState`，不含 room.id / stream.id / sequence number → client 无 payload-level 区分 stream 的能力。

**生态盲区**：12.4 r1 钦定的 streamRoomId 守护**仅刻画"流的 room 归属"，不刻画"流的身份"**。room 归属在跨 room 场景下足够，但身份才是 same-room 场景下的真正区分量。WebSocketClientImpl 内部已经有 `streamGeneration` 字段（12.5 r4 落地）能精确刻画"流的身份"，但**未对外暴露**给 vm 层 → vm 无 API 可用.

### 修复（Fix）

**不修，defer 至 Story 15.5 + 跨 4 case 统一重设计**。理由：

1. **跨 case 一致性约束**：同坑在 4 个 case 都存在（`.memberJoined` :617 / `.memberLeft` :626 / `.petStateChanged` :644 / `.connectionStateChanged` :666），都用 12.4 r1 钦定的 streamRoomId 守护。**仅修 pet.state.changed 一处会引入 4-case 不一致**（同样的 race window 三处放行一处拦截，未来 Claude 读代码时会困惑"为什么 pet.state.changed 用 stream-id 而 member.* 不用"）。如要修，必须 4 case 一起改 → 改动面 +4×guard + 4×测试 case + 重新审视 12.4 r1 钦定决策 → 已超 fix-review 单 commit 范围.

2. **基础设施缺失**：实现 codex 建议的"per-stream generation/identity"需要在 `WebSocketClient` protocol 层暴露新 API（如 `var currentStreamId: UUID { get }` 或在 `client.messages` 投递的 `WSMessage` 上附带 `streamId`）。Story 12.x 系列已 ship 的所有 mock / production WebSocketClient 都需要适配 → 涉及 12.2 + 12.4 + 12.5 + 12.6 + 12.7 多个 story 的代码契约扩展 → 不应在 15.2 一处 review 顺手做.

3. **Story 15.5 已规划兜底**：Epic 15 Story 15.5（跨房间状态恢复）AC 第 3 条（epics.md line 2464）明确钦定：**"edge: 断线期间收到旧的 pet.state.changed（晚到的）→ 被新 snapshot 覆盖（snapshot 始终为权威）"**。该 story 的核心机制是"重连后 server 端会下发新 room.snapshot 重新对齐全员状态"——这构成了 same-room reconnect 路径下 stale pet.state.changed 的自愈兜底（new snapshot 到达后 roster 被 server 权威值覆盖，stale 写入被 overwrite）。same-room leave-rejoin 路径同理：rejoin 会触发新 connect → server 下发 room.snapshot → 全员 roster 重置.

4. **残余风险概率评估**：
   - **时序窗口**：旧 stream `await MainActor.run` pending + 新 stream `room.snapshot` 到达之间的窗口。典型 ms 级别（main actor 调度延迟 + WS 帧到达延迟）.
   - **触发条件**：用户在 same-room leave-rejoin 或 WS same-room reconnect 的**毫秒级窗口内**对应房间某成员**恰好**有 pet.state 变化广播.
   - **影响范围**：单条 `member.pet.currentState` 错误显示。UI 显示状态错误**直到下一次状态变化广播或显式 snapshot 重拉**（< 几十秒级别，因为 motion-sync state 在用户活跃时频繁广播）.
   - **致命性**：低。不会导致 crash / 数据库不一致 / 跨用户串扰。仅 UI 视觉短暂不一致.

5. **统一重设计计划**（写给未来 Claude / Story 15.5 dev）：
   - 在 `WebSocketClient` protocol 加 `var currentStreamId: UUID { get }` 或类似 monotonically-increasing token，在 `prepareForReconnect()` / connect 路径翻动.
   - `RealRoomViewModel.startConsumingMessages()` 启动 task 时捕获 `let streamId = client.currentStreamId`（替代 streamRoomId，或与 streamRoomId 并存做双层防御）.
   - 4 case 都改用 `streamId == client.currentStreamId` 守护（capture vs 当前）.
   - 同步加 same-room rejoin / same-room reconnect 单测 case（构造方式参考 12.4 r1 lesson "直接调内部 handle hook" 模式）.

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 client 端处理"按房间 fanout 但 payload 不含 stream-id"的 WS 业务消息时（如 `member.joined` / `member.left` / `pet.state.changed` / `.connectionStateChanged`），**streamRoomId 守护仅在跨 room 场景下充分**；same-room leave-rejoin 与 same-room reconnect 路径仍存在 race window，**必须**通过"上游下发新 snapshot 全量对齐"的兜底机制 resolve（不能仅靠 streamRoomId 守护）；若要彻底闭环，需在 `WebSocketClient` protocol 层引入 per-stream identity（UUID / monotonic seq），4 case 守护统一改用 stream-id 比较.
>
> **展开**：
>
> - **streamRoomId 守护的边界**：12.4 r1 钦定的 streamRoomId 守护**精确刻画"流所属的 room"**，但**不刻画"流的身份"**。在以下两种 room 字段相同但 stream 不同的路径下守护失效：
>   1. same-room leave-rejoin（A → nil → A'，A' === A）
>   2. same-room reconnect（WS 内部 swap 但 vm 不感知）
>
> - **Story 15.5 snapshot 重对齐是兜底**：Story 15.5 AC 明确把"晚到 pet.state.changed 被新 snapshot 覆盖"列为 acceptance criterion。这意味着 same-room race 残余风险**通过"事件最终一致性 + snapshot 权威重对齐"机制 resolve**，不必在 event 层完美防御.
>
> - **stream-id 重设计前不要单点修补**：如果只在 `.petStateChanged` 一处升级到 stream-id 守护、其他 3 case 不动 → 引入 4-case 不一致 → 未来 Claude 读代码不易理解。要么 4 case 一起改，要么 4 case 一起接受残余风险 + 依赖 snapshot 兜底.
>
> - **协议层不允许加 stream-id field**：V1 §12.3 钦定 `pet.state.changed` payload 三字段（userId / petId / currentState），加 stream-id 违反协议精简钦定。stream-id 必须由 `WebSocketClient` protocol 层（client 内部）携带，不能塞进 protocol 层 payload.
>
> - **same-room reconnect 与 WebSocketClientImpl streamGeneration 解耦**：12.5 r4 lesson 已在 `WebSocketClientImpl` 内部加 `streamGeneration: Int`，但**仅用于隔离 stale receive-loop / catch path 在 client 内部对共享状态的写入**，并不投递给 vm 层。vm 层若需要 stream-id 守护，应通过新 `WebSocketClient` protocol API 拿到（如 `var currentStreamId: UUID`），而不是直接复用内部 `streamGeneration`（otherwise 跨 protocol mock 适配工作量爆炸）.
>
> - **反例**：
>   - 看到 codex 说"加 stream-id 守护"就在 `.petStateChanged` 一处偷偷加 UUID 字段比较 → 错。会形成 4-case 不一致（其他 3 case 仍 streamRoomId 守护）+ 局部修补但不闭环（real 测试 case 仍不能覆盖 same-room race）.
>   - 在 vm 内部自己维护 stream UUID（不下穿到 `WebSocketClient`）→ 错。无法在 mock 上构造端到端 race（mock 不知道 vm 自己的 UUID 何时翻动）.
>   - 直接说"snapshot 兜底就够了，不修也行"→ 部分对。snapshot 兜底确实能 resolve 残余风险，但记录决策**必须显式**写"接受残余风险 + 兜底机制 + 未来若要闭环的统一重设计计划"，不能含糊带过.
>   - 把这条 defer 决策埋在 commit message 里不归档 lesson → 错。lesson 是给未来 Claude 读的"为什么没修"参考；commit message 太短承载不了完整 rationale.

---

## Meta: 本次 review 的宏观教训（可选）

12.x 累积的 race 防御层级（在 12.4 r1 Meta 已枚举）：

1. **payload 层**（最精确）：消息自带房间/会话归属 → 用 payload 字段校验.
2. **stream 层**：消息不带归属 → 启动 task 时捕获 lastObservedRoomId 当 streamRoomId.
3. **state 层**（最弱）：仅靠 `lastObservedRoomId != nil`.

15.2 r1 暴露的是 **stream 层守护的盲区**：streamRoomId 是"流的 room 归属"而非"流的身份"，在 same-room 路径下退化。**第四层 stream-identity 层**（UUID / monotonic seq）应该被加入认知图谱，但其落地需要 protocol-wide 改动 → 不能 ad-hoc 修.

**下次设计 client message handler 时，主动 enumerate 四层防御**：

| 层 | 区分量 | 失效场景 | 覆盖手段 |
|---|---|---|---|
| 1. payload | 自带 room.id / session.id | payload 字段缺失 | server 协议层加字段（如有钦定空间） |
| 2. stream-room | 流所属 room | same-room rejoin / reconnect | stream-identity 层兜底 |
| 3. stream-identity | 流的唯一 ID | （无） | UUID / monotonic seq |
| 4. state | 仅 lastObservedRoomId nil 检查 | 已离开房间外几乎全部 race | 不能作为唯一防线 |

跨 case 应统一选定层级，**不要每个 case 选不同层**（一致性 > 局部最优）。
