---
date: 2026-05-14
source_review: /tmp/epic-loop-review-18-3-r1.md (codex round 1)
story: 18-3-选中表情-本地立即动效-ws-发送-emoji-send
commit: 484e757
lesson_count: 1
---

# Review Lessons — 2026-05-14 — Room-scoped transient UI state 必须在 room 切换/离开时重置（与 members / memberPetStates 同精神）（18-3 r1）

## 背景

Story 18.3 在 `RoomViewModel` 上新增 `@Published var activeEmojis: [RoomActiveEmoji]`，作为 room-scoped transient UI 队列（self 触发本地立即入队 + 18.4 落地 others 触发的 echo 入队）。codex round 1 review 指出：新引入的 room-scoped state 没有在 `subscribeRoomIdConnect` 的三个 reset 分支里跟随 `members` / `memberPetStates` 一起清空，导致 room A 选过的 emoji 在 A→B / A→nil → A' 等 vm 复用路径下残留到下一次 room 渲染。

review 原文：`/tmp/epic-loop-review-18-3-r1.md` 末尾 codex 段（唯一 P2 项）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | activeEmojis 在 room 切换 / 离开时未清空 → stale emoji 渲染到新房间 | P2 / medium | architecture | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` |

## Lesson 1: Room-scoped transient UI state 引入时必须同步加入"room 切换 reset 矩阵"

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` —— `subscribeRoomIdConnect` 的 nil→A / A→B / A→nil 三个 case

### 症状（Symptom）

新加的 `@Published var activeEmojis: [RoomActiveEmoji]` 是 room-scoped transient UI 队列，但 `RealRoomViewModel.subscribeRoomIdConnect` 内已经清空 `members` / `memberPetStates` 的三个 reset 分支（nil→A、A→B、A→nil）**没有同步清** `activeEmojis`。后果：

- room A 内 `vm.onEmojiSelected(code:)` 入队的 emoji → A→B 直接切换路径下残留 → room B 渲染 RoomScaffoldView 时 `EmojiAnimationLayer` 渲染 room A 的 stale emoji
- 同 vm 实例复用场景下（如 `@StateObject` 跨 Container view 留存），用户回到 home 再进同一/别的房间 → stale emoji 渲染
- 18.4 落地的 1.5s 自动 expire 路径**不**能兜底：用户切房间速度可能快于 1.5s，且 expire 路径本来就只是"队列里项的生命周期管理"，不是 room 边界管理

### 根因（Root cause）

引入 room-scoped transient state 时**没有 grep `self.members = []` 找现有 reset 路径并对齐**。设计文档 ADR-0010 §3.2 钦定 transient UI state 不持久化、不进 AppState，但**没有显式列出"必须在 room 切换/离开时清空"** —— 隐含规则被新人/未来 Claude 错过。

`members` / `memberPetStates` 自 Story 12.1 起就在三个 reset 分支里清，是公开规则的一部分；但**新增字段**的人需要自己主动发现并对齐。

具体疏漏点：本 story 实装 `onEmojiSelected` 时只考虑了"入队 + WS send"两件事，没考虑"vm 生命周期跨多个 room session 时的 reset 语义"。

### 修复（Fix）

在 `subscribeRoomIdConnect` 的三个分支里同步清 `activeEmojis`（与 `members` / `memberPetStates` 同位置）：

```swift
// 分支 2: nil → A (进入房间)
if self.webSocketClient != nil {
    self.members = []
    self.memberPetStates = [:]
}
self.activeEmojis = []   // ← 新增（无条件清；transient 队列非 scaffold seeded，UITest nil-client 路径下也安全）

// 分支 3: A → B (直接切换)
if self.webSocketClient != nil {
    self.members = []
    self.memberPetStates = [:]
}
self.activeEmojis = []   // ← 新增（A→B 是最关键的 stale 路径，无 nil reset 中间态）

// 分支 4: A → nil (离开房间)
self.members = []
self.memberPetStates = [:]
self.activeEmojis = []   // ← 新增（leave 后回房间不能残留）
```

**条件化选择**：`members` / `memberPetStates` 在 nil→A 和 A→B 分支条件 `webSocketClient != nil` 是为了保留 UITest nil-client 路径下的 `RoomScaffoldDefaults` seed roster（lesson 2026-05-11-uitest-nil-ws-client-must-preserve-scaffold-roster.md）。`activeEmojis` 不是 seeded data，无条件清是更一致的选择 —— UITest 路径下根本没有 emoji 入队，无条件清是 no-op。

**新增测试**（`RealRoomViewModelTests.swift`）：
- `testLeaveRoomClearsActiveEmojis`：vm 在 room A 入队 2 emoji → setCurrentRoomId(nil) → 断言 activeEmojis = []
- `testDirectRoomToRoomSwitchClearsActiveEmojis`：vm 在 room A 入队 1 emoji → 直接 setCurrentRoomId("room_B") → 断言 activeEmojis = []

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 RoomViewModel / RealRoomViewModel 上**新增 room-scoped @Published 字段**时，**必须** grep `self.members = []` 找到所有现有 reset 路径并对齐清空。
>
> **展开**：
> - room-scoped state = 语义上"只属于当前 room session"的数据，包括 roster (`members` / `memberPetStates`)、transient UI 队列 (`activeEmojis`)、room 元信息 (`roomCodeForCopy`) 等。新增此类字段时，对照下面的 checklist：
>   1. grep `self.members = []` —— 找到 `subscribeRoomIdConnect` 的三个 reset 分支（nil→A, A→B, A→nil）
>   2. 新字段是否需要在这三处同步清？默认答案是**是**（除非该字段语义上跨 room session 保留，如 currentUserId 派生自 appState.currentUser 与 room 无关）
>   3. 条件化（`if webSocketClient != nil`）vs 无条件：scaffold seeded data 走条件化（保护 UITest）；transient 非 seeded 走无条件
>   4. 在 `RealRoomViewModelTests` 加 leaveRoom + roomSwitch 两个回归 test
> - **反例**：新增 `@Published var typingIndicators: [TypingIndicator]`、`@Published var roomNotifications: [Notice]` 等 transient 队列，**只**实装入队/去重路径，**忘了** room 切换 reset → 与本 lesson 同病。
> - **反例 2**：仅在 `onLeaveTap` （主动 leave 路径）清，忽略 A→B / nil→A 等其他 transition → 因为 reset 路径是 **`subscribeRoomIdConnect` 的 sink 闭包**而不是 `onLeaveTap`（onLeaveTap 走的是 LeaveRoomUseCase → server `room.leave` → server push `currentRoomId=nil` → AppState 写 nil → sink 闭包触发 A→nil 分支），所以 sink 闭包是 single source of truth for room transition reset。
> - **正例参照**：本 lesson 修复 + 已有 `members` / `memberPetStates` 处理模式。
