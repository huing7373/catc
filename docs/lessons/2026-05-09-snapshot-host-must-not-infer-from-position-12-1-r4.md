---
date: 2026-05-09
source_review: codex review (round 4 of Story 12.1, file: /tmp/epic-loop-review-12-1-r4.md)
story: 12-1-房间页面-swiftui-骨架
commit: 46ca502
lesson_count: 1
---

# Review Lessons — 2026-05-09 — `RoomMember.isHost` 不能用 snapshot 位置启发式推断（12-1 r4）

## 背景

Story 12.1 RealRoomViewModel 第 4 轮 review。前三轮分别修了 `dropFirst` / `prepareForReconnect` / `room.id` stale guard。本轮 codex 找到一条遗留的"占位语义错配"：`applySnapshot` 用 `isHost = index == 0` 推断房主。

review 钦定的反例：协议明文允许房主离开后房间继续存在；同时明文 client **不能**依赖 member 顺序。"剩下的第一个成员"被错误标 `isHost` → UI 显示错误的"队长"徽章。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | snapshot path 用 `index == 0` 推断 host 在房主已离开后产生错误"队长"徽章 | medium | other (correctness) | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:320` |

## Lesson 1: snapshot 不带 host 字段时所有 `isHost` 必须 false（不依赖位置 / 不依赖任何启发式）

- **Severity**: medium
- **Category**: other (correctness / protocol-conformance)
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:320`

### 症状（Symptom）

`RealRoomViewModel.applySnapshot(_:)` 把 server 推送的 `room.snapshot` 映射成 `[RoomMember]` 时，节点 4 阶段为了让 UI 占位有个"队长"徽章，把 `isHost = (index == 0)` —— 即 snapshot 中第一个成员被视为房主。

合法 server state 下房间可在房主离开后继续存在（协议钦定）；此时 snapshot.members 的"第一个成员"只是数组顺位，与 host 身份完全无关。client 错误标"队长"徽章 → 用户视觉上以为某个普通成员是房主。

### 根因（Root cause）

把"placeholder 占位"和"位置启发式"混淆。当 server 协议尚未下发某字段（host 字段是 Story 12.x 后续节点才接入）时，正确的占位语义是"未知 / 不可推断"，而非"用某种代理量近似"。位置（`index == 0`）作为代理量看似无害，但当 server 状态机允许"位置 0 不是 host"时（房主离开后房间继续 = 协议钦定），代理量就和真实语义脱钩，制造肉眼可见的 UI 错误。

进一步：协议本身已经明文给出第二条强约束 —— **client 不能依赖 member 顺序**。这一条直接否决任何"用 index 派生字段"的实装，无论是占位还是临时。

### 修复（Fix）

改 `applySnapshot` 把所有 `RoomMember.isHost` 一律置 `false`（"未知 host"占位语义）。`for` 循环不再需要 `enumerated()` —— 移除 index 即移除诱因。

```swift
// before
for (index, snapshotMember) in payload.members.enumerated() {
    let merged = RoomMember(
        ...,
        isHost: index == 0  // ← 协议禁止
    )
}

// after
for snapshotMember in payload.members {
    let merged = RoomMember(
        ...,
        isHost: false  // 不依赖位置启发式；等后续 epic snapshot 真带 host 字段时再接
    )
}
```

vm 自身的 `userIsHost`（"我是不是房主"）是与 `RoomMember.isHost` 独立的字段；由 init 时从 `RoomScaffoldDefaults.userIsHost` seed，`applySnapshot` **不**触碰 —— 等真实 host 字段下发后由 vm 单独从 `appState.currentUserId == hostUserId` 派生，不混进集合 merge 路径。

测试：
- 修订既有 case#2 断言：原来断言 `members[0].isHost == true`（基于 `index == 0`），改为 `XCTAssertFalse`
- 新增 case#11 `testSnapshotPathDoesNotInferHostFromMemberOrder`：4 成员 snapshot 全员 isHost 必须 false；同时反向断言 vm.userIsHost 保留 init 时 seed 的占位值（applySnapshot 不该触碰）

不动 mock 默认（`RoomScaffoldDefaults.members[0].isHost == true` / `MockRoomViewModel.fourMembersMock`）—— 那些是 mock 占位场景，不走 snapshot path；review 明确说"修改是针对真实 snapshot path"。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**实装 server snapshot/payload → client model 映射**时，若 payload **不带某个字段**（无论是协议尚未下发还是节点占位阶段），**禁止**用任何**位置启发式**（`index == 0` / `members.first?` / `members.last?`）推断该字段，**必须**赋一个明确的"未知 / 占位"默认值（数值 default / `false` / `nil`），等 server 真正下发后再接。
>
> **展开**：
> - "占位 placeholder" 的合法形式是**字段级常量**（如 `level: 8` / `status: "在玩耍"` / `isHost: false`），**不是**从同 payload 其它字段（位置 / 数量 / 顺序）派生出来的代理量
> - 协议如果明文给出"client **不能依赖** X"的约束，把 X 用作派生输入就是协议违规 —— 不仅占位阶段不能用，永远不能用
> - 若 server state 机允许某种"反直觉但合法"的状态（如本案：房主离开后房间继续存在），任何用代理量近似的派生字段在该状态下必然出错；写 placeholder 前先检查 server state 机的全部合法状态空间
> - **vm 级状态 ≠ 集合元素状态**：`userIsHost`（"我是房主"）和 `RoomMember.isHost`（"集合中某成员是房主"）是两个独立字段；snapshot apply 路径**只**应触碰集合元素字段，vm 级状态保留 init 时 seed 的占位 / 由独立路径（如 `appState.currentUserId == hostUserId`）派生
> - **反例**：`isHost: index == 0` / `name: members.count == 1 ? "只有自己" : "多人"` / `level: members.first?.level ?? 0` —— 只要 RHS 涉及"位置 / 顺序 / 数量"等顺位概念派生出语义字段，就是反例
> - **正例**：`isHost: false`（未知占位）或 `isHost: snapshotMember.userId == payload.room.hostUserId`（payload 真带字段后）

## Meta: 三轮 review 的累积教训

Story 12.1 connect ↔ disconnect 路径在 r1 / r2 / r3 / r4 共四轮 review 中分别命中四个独立类型的 bug：

- **r1**：`Published` 订阅时机（`dropFirst` 丢 restored 同步 emit）+ A→B 切换不重置 roster
- **r2**：WebSocket client 复用需要显式 `prepareForReconnect()` 重置 stream
- **r3**：snapshot 必须按 `room.id` 校验丢弃 stale 消息
- **r4**：snapshot 不带 host 字段时不能用位置启发式推断 host

四个 bug 横跨"订阅时机 / 资源生命周期 / 消息 race / 占位语义"四个维度，但有一个**共同的元模式**：**在节点 4 阶段做"占位实装"时，过度复用同一份变量 / 同一组语义边界**。

教训：节点 4 阶段写 placeholder 时务必显式标注**每一个字段**的占位策略 —— 是赋常量、是 fallback 到 client 已有值、还是直接置 `nil` / `false` —— 不要把"看起来合理的最小路径"当成默认。每多一个隐式启发式就多一个未来要单独 review 的隐患点。
