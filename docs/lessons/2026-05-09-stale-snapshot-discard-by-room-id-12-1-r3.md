---
date: 2026-05-09
source_review: codex /tmp/epic-loop-review-12-1-r3.md
story: 12-1-房间页面-swiftui-骨架
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-09 — `room.snapshot` 必须按 room.id 校验丢弃 stale 消息（12-1 r3）

## 背景

Story 12.1 第 3 轮 codex review 在 `RealRoomViewModel.handle(message:)` 处理 `.roomSnapshot` 路径上发现一个 race：用户 leave / 直接 room A → B 切换时，前一个 stream 上排队的 `room.snapshot` 可能在 `currentRoomId` 已经变更后才被 deliver；旧实装在 `case .roomSnapshot` 分支无条件 `applySnapshot(payload)`，让 stale snapshot 把已切换房间的 roster 写回旧值。`prepareForReconnect()` 走的是新 stream，但旧 stream 上残留的 in-flight 消息以及 mock/test 直接 `emit` 进新 stream 都能复现。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `room.snapshot` 应用前未按 room.id 校验，stale snapshot 污染当前 roster | high (P1) | error-handling | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:257-260` |

## Lesson 1: WebSocket payload 必须按"消息归属的资源 id"校验当前归属再 apply

- **Severity**: high
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:257-269`

### 症状（Symptom）

`handle(message:)` 在 `.roomSnapshot` 分支无条件 `applySnapshot(payload)`。当用户 leave / 直接切到下一房间后，前一个 stream 上排队的 `room.snapshot` 投递到 ViewModel：
- A → B 切换瞬间：分支 3（A→B）已经清空 `members` 并 `prepareForReconnect()`；若仍有针对 room A 的 snapshot 进入 `handle`（无论旧 stream 残留还是新 stream 上的 stale 消息），members 会被回写为 room A 名单，UI 渲染 room B 但 roster 是 A 的。
- A → nil 离开：分支 4 disconnect + 清 roster；若 stale snapshot for A 仍能进入 `handle`，已清空的 roster 被 ghost 复活。

### 根因（Root cause）

WebSocket 异步模型下，`payload` 的"归属"和 ViewModel "当前归属"是两个独立时间线：消息 enqueue 时归属是 A，consume 时归属可能已经是 B / nil。**只要不显式校验 payload 携带的归属 id 与当前归属 id 是否一致**，就一定有 race 窗口让 stale 消息生效。`prepareForReconnect()` 重置 stream 只能挡住"旧 stream 上的"消息，挡不住"在 ViewModel 切换瞬间已被 deliver 进 handle"的消息（即使是新 stream 的也可能因为 mock 测试 / 实际网络抖动 race 进来）。

### 修复（Fix）

`handle(message:)` 在 `.roomSnapshot` 分支 apply 前先校验 `payload.room.id == lastObservedRoomId`，不匹配则丢弃 + log debug。
- 校验源用 `lastObservedRoomId`（sink 内字段写入；切换瞬间已是新值），而非现读 `roomId` computed getter（间接读 `appState.currentRoomId`，可能在外部 mutate 路径上被改）。
- `lastObservedRoomId == nil` 时（已 leave）任何 snapshot 都判为 stale，不 apply。
- `""` 与 `""` 匹配（与 `HomeRoomDispatcher` 把空字符串当 in-room 的语义一致；round 2 lesson 已锁定）。

```swift
case .roomSnapshot(let payload):
    guard let currentRoomId = lastObservedRoomId, payload.room.id == currentRoomId else {
        os_log(.debug, "RealRoomViewModel: discard stale room.snapshot ...")
        return
    }
    applySnapshot(payload)
```

回归测试：
- `testStaleSnapshotForOldRoomDoesNotOverwriteCurrentRoster` —— A→B 切换后投递 room A 的 stale snapshot，断言 members 仍为空（不被 ghost 复活）；再投 room B 的 fresh snapshot 验证守卫不误伤当前房间。
- `testStaleSnapshotAfterLeaveDoesNotRepopulateMembers` —— A→nil 后断言 members 保持空（防回归把 disconnect 分支里 `members = []` 删掉）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写 **WebSocket / async stream 消息处理路径** 时，**必须**在 apply 前 **校验 payload 自带的"归属 id"（room.id / sessionId / pairingId 等）与 ViewModel 当前归属 id 一致**，不一致则丢弃 + log。
>
> **展开**：
> - 异步消息处理的不变量是"payload 归属时刻 == ViewModel 当前归属时刻"，**不能假设 stream 重置 / 取消 task 就够**。`prepareForReconnect()` / `task.cancel()` 都只能挡新消息进入 stream，挡不住已 in-flight 的消息进 `handle`。必须在 `handle` 末梢做"归属校验"作为最后一道闸。
> - 校验源**优先用 sink 内字段**（如 `lastObservedRoomId`），不要现读 `appState.currentRoomId` —— 后者可能在外部 mutate 路径上被并发修改；前者是 sink 切换瞬间字段级写入，invariant 更稳。
> - 跨模块语义对齐要回查：本仓库 `HomeRoomDispatcher` 把 `""` 当 in-room（round 2 已对齐），所以归属校验时 `""` 与 `""` 必须匹配；不要以为"空字符串和 nil 同义"。
> - 离开场景（current = nil）：**任何 snapshot 都视为 stale**，统一丢弃 —— 不要写"如果 nil 就跳过校验"的特例，否则 ghost roster 会复活已清空状态。
> - 测试覆盖：A→B 切换后 emit room A 的 snapshot 必须不影响 members；emit room B 的 fresh snapshot 必须正常 apply（守卫是单向阻断不是双向）。
> - **反例 1**：在 `handle(message:)` 处理 `.roomSnapshot` 直接 `applySnapshot(payload)`，靠 `prepareForReconnect()` / consumer task cancel 挡 stale —— async 时序上挡不住，必有 race。
> - **反例 2**：用 `appState?.currentRoomId == payload.room.id` 校验 —— 若 appState 在并发路径上被改回旧值（不太可能但路径暴露），校验形同虚设。用本地字段 `lastObservedRoomId` 闭锁。
> - **反例 3**：把"空字符串归 nil 化"加到守卫里 —— 与 `HomeRoomDispatcher` 现有语义冲突（round 2 lesson 已禁止）。
