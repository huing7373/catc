---
date: 2026-05-09
source_review: codex review on Story 12.1 round 2 (file: /tmp/epic-loop-review-12-1-r2.md)
story: 12-1-房间页面-swiftui-骨架
commit: 8e5f182
lesson_count: 2
---

# Review Lessons — 2026-05-09 — WebSocketClient 复用必须先 prepareForReconnect 重置 stream & "空 roomId" 语义必须跨模块对齐 dispatcher（12-1 r2）

## 背景

Story 12.1（房间页面 SwiftUI 骨架）round 2 codex review。round 1 已修了 dropFirst 丢 restored state +
A→B 房间切换不重置 roster 两个问题；本轮在那基础上 codex 又 surfaced 两个相关但更深层的坑：

1. **P1**：A→B 切换路径 `disconnect()` finish 了 `messages` AsyncStream，紧接着 `startConsumingMessages()`
   在**同一** webSocketClient 上启新 consumer task —— 新 consumer 接到的还是已 finish 的 stream，subsequent
   `room.snapshot` 永远不会到达。同坑也存在于 leave-rejoin（A→nil→A'）路径。Story 12.2/12.7 注入真实
   `WebSocketClientImpl` 复用同 client 实例时一定踩。
2. **P2**：`RealRoomViewModel.subscribeRoomIdConnect` sink 内部把 `""` normalize 成 `nil` 走 disconnect 分支；
   而 `HomeRoomDispatcher.shouldShowRoom("")` 钦定 `""` 为 in-room（`HomeContainerViewTests:41` 锁住）.
   两边不一致 → caller 漏出空字符串时 UI 渲染 `RoomScaffoldView` 而 vm 走 disconnect/clear-members 路径,
   状态机错位.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | A→B / leave-rejoin 复用同 client → finished stream consumer 拿不到消息 | P1 | architecture | fix | `iphone/PetApp/Core/Networking/WebSocketClient.swift`、`WebSocketClientMock.swift`、`Features/Room/ViewModels/RealRoomViewModel.swift` |
| 2 | "" roomId 在 vm 与 dispatcher 不对齐 | P2 | architecture | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` |

## Lesson 1: AsyncStream-based client 复用必须留 reconnect 接缝；disconnect 后再起 consumer 必拿新 stream

- **Severity**: high (P1)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:179-211`、`Core/Networking/WebSocketClient.swift:18-50`、`WebSocketClientMock.swift`

### 症状（Symptom）

`WebSocketClient.disconnect()` 文档钦定 finish 该 client 的 `messages` AsyncStream；在 vm 内部 A→B 切换分支
紧接着调用 `webSocketClient?.disconnect()` 然后 `startConsumingMessages()` —— 后者用 `client.messages` 读
stream 起新 `for-await` task，而该 stream 已被 finish，循环立刻退出，subsequent `room.snapshot` 消息永远
不会被 consume。同坑也在 leave-rejoin 路径（A→nil 走 disconnect 分支后再 nil→A' 走 connect 分支重新起
consumer 时）出现。

### 根因（Root cause）

AsyncStream 是 single-shot：`continuation.finish()` 之后该 stream 不能"复活"，新 iterator 立即退出循环。
"复用同一 client 接收新消息"必须**显式**让 client 创建新 stream / 新 continuation 替换旧的，否则 stream
property 仍指向旧（已 finish）stream，新 consumer 永远等不到消息。

vm 在 round 1 的 fix 只 cancel 了旧的 consumer task + 起了新的 task，但 new task 用的还是**老 stream**，
等于换了消费者却没换"消息源"。

设计漏洞：`WebSocketClient` protocol 只暴露 `disconnect()` + `messages`，没有"重置 stream"接缝。
disconnect 后无路可走 —— 要么换 client 实例（违背 review 担心的"同 client 复用"假设），要么协议必须
增 reconnect/restart 接缝.

### 修复（Fix）

按 review 推荐路径 (a)：留接缝，不破坏协议最小性（与 Story 12.2 落地路径一致）。

1. **Protocol 增 `prepareForReconnect()` 方法 + 默认 no-op 实现**
   ```swift
   public protocol WebSocketClient: AnyObject, Sendable {
       var messages: AsyncStream<WSMessage> { get }
       func disconnect()
       /// 调用后 messages getter 返回新 AsyncStream（旧 stream 已 finish；
       /// 下一次 caller 起 for-await 拿到新 stream）.
       func prepareForReconnect()
   }
   extension WebSocketClient {
       public func prepareForReconnect() {} // 默认 no-op
   }
   ```
2. **`WebSocketClientMock` override 实装真正 swap**：内部 `messages` 改为计算属性 backed by `var
   currentStream` + `var currentContinuation`，`prepareForReconnect()` 创建新 stream/continuation 替换；
   测试 hook `prepareForReconnectCallCount` 让 vm 测试断言。
3. **`RealRoomViewModel` 在每个会"起新 consumer"的转换分支里都先调 `prepareForReconnect()`**：
   - 分支 (nil → A)：在调 `startConsumingMessages()` 前调 `prepareForReconnect()`（覆盖 leave-rejoin
     路径——上一轮可能已被 disconnect）.
   - 分支 (A → B)：在 `disconnect()` 之后、`startConsumingMessages()` 之前调 `prepareForReconnect()`.
4. **测试锁住可观测后果**：
   - `testDirectRoomToRoomSwitchResetsRosterAndStream` 增断言：A→B 切换后向 mockWS emit room B 的 snapshot,
     vm.members 必须能更新到 room B 成员（旧实装 stream 永远 finish → 永远 0 成员）.
   - 新增 `testLeaveRejoinReusesSameClientAndReceivesMessages`：A→nil→A' 复用同 client，A' 的 snapshot
     必须能驱动 vm.members.

Story 12.2 落地 `WebSocketClientImpl` 时 override `prepareForReconnect()` 做真实 task 状态重置 + 准备好
新 stream/continuation；与 `connect(roomId:token:)` 配合：caller 调 `prepareForReconnect()` →
`connect(roomId: next, token: ...)` → `for await ... in client.messages` 拿新 stream。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计基于 AsyncStream 的"可断开-可重连"client protocol** 时，**必须**
> **同时暴露"重置 stream"接缝**（如 `prepareForReconnect()` / `reset()` / 内置在 `connect(...)`），
> **不能只暴露 `disconnect()` + `messages` getter**.
>
> **展开**：
> - AsyncStream `continuation.finish()` 是不可逆的：一旦 finish，新 iterator 立即退出，没有"复活"语义.
> - 任何"复用同 client 接收新消息"的路径（如 room 切换、leave-rejoin、reconnect after timeout）都必须先让
>   client 持有新 stream/continuation，再起新 consumer task.
> - protocol 即使最小，也要把"换 stream 的接缝"写进契约。如果 caller 不能自己换 stream（client 实例由
>   外部注入），就一定要让 protocol 暴露这个能力——否则 caller 就被锁死在"disconnect 后只能丢弃实例"的
>   pattern 上.
> - 测试必须断言**可观测后果**而不仅仅是中间字段：`mockWS.didDisconnect == true` 不够，要 emit 新消息
>   并断言 vm.members 能更新（这才是用户关心的"消息流是活的"语义）.
> - 用 mock 提供**真实的 stream swap 行为**而不是只 stub 一个 boolean 标志位 —— 否则 mock 与 production
>   行为偏移，回归测试看似绿但 production 会踩坑.
> - **反例**：A→B 切换分支只 `disconnect()` + `cancel old task` + `start new task`，没让 client 准备新
>   stream → 新 task 接到已 finish 的 stream → 永远收不到消息 → UI 永远空房间。Story 12.2 注入真实
>   client 后立即暴露此问题。同样反例适用于 leave-rejoin 路径（A→nil→A' 复用同 client）.

## Lesson 2: "空 roomId" 这种 edge-case 语义必须**跨所有相关模块统一对齐**，不能各自 normalize

- **Severity**: medium (P2)
- **Category**: architecture / consistency
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:170-175`、`Features/Home/Views/HomeContainerView.swift:98-100`、`PetAppTests/App/HomeContainerViewTests.swift:41-46`

### 症状（Symptom）

UI 路由层 `HomeRoomDispatcher.shouldShowRoom("")` 返回 `true`（即 `""` 视为 in-room；测试明确锁住），
而同应用内的 `RealRoomViewModel.subscribeRoomIdConnect` sink 把 `""` normalize 成 `nil` 走
disconnect/clear-members 分支。两边对 `""` 的语义判断相反 → 当某 caller 漏出 `""`：
- HomeContainerView 依据 dispatcher 判断 → 渲染 `RoomScaffoldView`（in-room UI）
- RealRoomViewModel 依据自己的 normalize → 走 disconnect 分支（清 members、wsState=.disconnected）

UI 看起来在房间，状态机说不在 → 用户看到空 RoomScaffold 而 vm 已断连，交互完全卡住.

### 根因（Root cause）

vm 作者只看到自己内部的 transition 表（nil/non-nil 二态），加了 `""` → `nil` 的"防御性 normalize"
让 (nil, nil) no-op 分支吃掉空字符串信号；但忽略了**同 currentRoomId 数据被多个模块消费**：dispatcher
有自己的判断规则、HomeContainer 用 dispatcher、Room scaffold 用 vm。三处对"空字符串语义"必须统一,
否则任意路径漏出 `""` 都会让两边不一致.

更深的问题：当 server 契约保证 roomId 非空时，"" 本身是 caller bug 信号。两边对齐成 in-room（dispatcher
路径）比都对齐成 nil 风险更小：
- 对齐 in-room：bug 表现是"渲染 RoomScaffold + 走 connect 分支" —— 仍可观测、UI 可见、可继续修.
- 对齐 nil：bug 表现是"漏过 idle 检查 + 漏 disconnect" —— 状态机静默错位，更难 debug.

### 修复（Fix）

让 RealRoomViewModel 与 HomeRoomDispatcher 对齐：删掉 `""` → `nil` normalize，直接用 newRoomId 的
`nil`/`non-nil` 判断（`""` 走 connect 分支，与 dispatcher 一致）。

```swift
// before（fix-review round 1 实装；与 dispatcher 不一致）
let normalizedNew: String? = (newRoomId?.isEmpty == false) ? newRoomId : nil
self.lastObservedRoomId = normalizedNew
switch (previous, normalizedNew) { ... }

// after（与 dispatcher 对齐）
self.lastObservedRoomId = newRoomId
switch (previous, newRoomId) { ... }
```

新增测试 `testEmptyStringRoomIdTreatedAsInRoomAlignsWithDispatcher` 锁住该决策（`""` → wsState
== .connected + didDisconnect == false）.

**未选反向路径（让 dispatcher 把 "" 当 nil）的理由**：
- HomeContainerViewTests:41 已显式锁住 `shouldShowRoom("") == true`（"防 caller 漏检；server 契约保证
  roomId 非空"），改 dispatcher 会破多个 nav 测试.
- RootViewWireTests 也用 `shouldShowRoom`，影响面更大.
- 改 vm 的 sink 内部转换是单点 minimal change，改 dispatcher 是跨多组件 contract 变更.

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **某个状态字段（如 currentRoomId / currentUserId / sessionToken）会被多个模块
> 消费且各自有 nil-vs-empty 判断逻辑** 时，**必须**先 grep 所有 consumer 的 normalize 规则、统一到**同一**
> 边界条件，**不能**在某一个 consumer 内部加局部 normalize 而其他 consumer 不知情.
>
> **展开**：
> - 多 consumer 共享一个状态字段时，"empty"、"nil"、"whitespace-only" 这些边界值的语义必须**单一来源**：
>   要么字段定义层把 setter 强制 normalize（如 `setCurrentRoomId(_ value: String?)` 内部把 `""` → `nil`），
>   要么所有 consumer 用同一个 helper（如 `RoomIdNormalizer.isInRoom(_)`）.
> - "防御性 normalize" 是反模式当且仅当你只在自己模块内部加：你"防"住了自己的边界，但下游模块的判断逻辑
>   还是按原始值走 → 状态机各路径不自洽.
> - 写新逻辑前必须 grep 该字段所有 consumer：搜 `appState.currentRoomId` / `$currentRoomId`，看每个 site
>   的判断条件，确认你的 normalize 不与他们冲突.
> - 选哪边对齐：风险评估用"bug 信号 silent vs visible"判断 —— 静默错位（visible UI + dead state machine）
>   通常比"渲染了不该渲染的 UI"更难 debug；优先选**让 bug 可见**的方向.
> - **反例**：vm 内部 sink `""` → `nil` normalize 把 (nil, "") 转换吃掉，dispatcher 那侧仍渲染 RoomScaffold
>   → UI 显示房间但 vm 已 disconnect。两边各自正确，组合错位。

---

## Meta: 本次 review 的宏观教训

两条 lesson 共享一个元问题：**"看似最小的 protocol / 最小的局部 normalize 是反模式"**。

- Lesson 1：协议太小（只 disconnect + messages，没 reconnect 接缝）→ 调用方陷入"无路可走"陷阱.
- Lesson 2：局部 normalize（vm 内部 `""` → `nil`）→ 跨模块状态机不自洽.

写这种"边界 + 多模块共享"的代码时，**最小化的不应是 API 表面，而是不变量集合**。protocol 应该暴露**所有
合法转换路径**所需的接缝（即使默认实装是 no-op），状态字段应该有**单一 normalize 真值**（在 setter 层
强制 + 所有 consumer 用同一 helper）。否则"最小"会变成"功能缺口"或"语义裂缝"，回归测试看不到的坑.
