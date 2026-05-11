---
date: 2026-05-11
source_review: codex review --base 3dc6584（Story 12.7 round 11 P2）
story: 12-7-创建-加入-退出-use-case-主界面入口完善
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-11 — Room 进入路径必须先清 scaffold roster 再 connect（nil→A 与 A→B 分支同语义）

## 背景

Story 12.7 第 11 轮 codex review。前 10 轮逐步把 RealRoomViewModel 的 Room navigation 路径打磨：r1 修
`try?` 静默吞错、r2 修 leave-rejoin 后 stream 不重启、r3 禁掉 WS connect failure 走 errorPresenter、
r4 加 stale connect failure 守护、r5/r6 修 bind same-instance rebind / swap 路径、r7-r10 修各种
HTTP/UseCase 路径的 stale-response 守护。本轮触达最后一处遗漏：**RoomScaffoldDefaults seed 假成员**
在 `nil → A` 进入房间时未被清空，与 `A → B` 分支不对齐 —— 若首个 `room.snapshot` 延迟到达或
`connect(roomId:)` 抛错时，假成员会停留在 UI 上（短暂或永久）.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | nil→A 分支必须先 reset roster 再 connect（与 A→B 对齐） | medium (P2) | architecture | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:317-323` |

## Lesson 1: nil→A 分支必须先 reset roster 再 connect（与 A→B 对齐）

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:317-323`（旧路径；新增 `self.members = []` + `self.memberPetStates = [:]`）

### 症状（Symptom）

`RealRoomViewModel.init()` 用 `RoomScaffoldDefaults.members`（4 个假成员：小花 / Mocha / Latte / Espresso）
seed `members` —— 让 RoomScaffoldView 首帧不空（Story 37.8 round 1 P2 落地决议）.
当 `appState.currentRoomId` 由 `nil → A` 切换时，`subscribeRoomIdConnect` 的 sink 走 `nil → A` 分支，
做了三件事：① `prepareForReconnect()` ② `wsState = .connected` 占位 ③ `startConsumingMessages()` +
spawn Task 真实拨号. **唯独漏掉清空 `members` / `memberPetStates`**.

后果：
- 用户进入房间瞬间 → UI 立即切到 RoomView → 但 `members` 仍是 RoomScaffoldDefaults 4 个假成员
- 首个 `room.snapshot` 到达前 → 用户看到 4 个不属于当前房间的假成员（accuracy regression）
- 若 `connect(roomId:)` 抛错（token missing / server down / handshake error）→ snapshot 永远不到达
  → 假成员**永久残留**直到用户主动离开

`A → B` 分支（房间切换）已经做了 `self.members = []` + `self.memberPetStates = [:]`；`A → nil` 分支
（离开房间）也清空了 —— 唯独 `nil → A` 分支没有 reset roster.

### 根因（Root cause）

逻辑漏洞来自三件事的组合：

1. **设计期错位**：Story 37.8 round 1 P2 决策 "seed scaffold defaults 让首帧不空" 是为了 UITest /
   占位场景（彼时没有 WS）；当时 vm 不会从 `nil → A` 切换（RootView 直接显示 inRoom）.
2. **Story 12.1 增量演进**：subscribeRoomIdConnect 落地后引入 `nil ↔ A` ↔ `A ↔ B` ↔ `A ↔ nil` 四态转换.
   A→B / A→nil 分支显式 reset 了 roster（明确是从一个 active room 切走，旧 roster 必须废弃），但
   `nil → A` 被默认假设"初始 roster 是空的"—— 没考虑 init seed 的 scaffold defaults.
3. **测试盲区**：既有单测验证了"进入 room 后 snapshot 来到时 members 正确"，没有测"进入 room 但
   snapshot 还没来 / 永远不来时 members 状态"——空窗期 / 失败路径下的 UI 状态没断言.

本质规则：**`init()` seed 的 placeholder 数据，在状态机进入"真实数据模式"前必须被显式清空**；
不能依赖"反正下一条真实数据来会覆盖"—— 因为"下一条数据"可能永远不来.

### 修复（Fix）

```swift
case (nil, .some(let roomId)):
    // 分支 2：nil → A，进入房间.
    //
    // **fix-review round 11 P2**：先 reset roster（与 A→B 分支对齐）.
    // 旧实装不清空 → snapshot delayed / connect 失败时假成员残留.
    self.members = []
    self.memberPetStates = [:]
    //
    // **fix-review round 2 P1**：若 client 之前被 disconnect 过 ...
    self.webSocketClient?.prepareForReconnect()
    ...
```

回归测试两条（`RealRoomViewModelTests`）：
- `testNilToAClearsScaffoldRosterBeforeConnect`：start with `RoomScaffoldDefaults.members` (4 个) →
  trigger `nil → A` (gate 卡住 connect) → 断言 `vm.members == []` + `vm.memberPetStates == [:]`，
  **不**等 snapshot 到达
- `testNilToAClearsRosterEvenWhenConnectFails`：让 `connect(roomId:)` 抛 `WSError.tokenMissing` →
  snapshot 永远不到 → 断言 `vm.members == []`（验证 review 命中的"假成员永久残留"场景被根治）

**审核结论**：扫了 RealRoomViewModel 是否还有"切到 room 但 roster-untouched"的分支：
- `bind` 的 `clientChanged && lastObservedRoomId != nil` 路径（line 174-249）：VM 已经 in-room 时
  swap client。理论上 sink 已经先 fire 过 `nil → A` 分支（订阅时同步 emit），roster 在那里
  已经被 reset；不需要 bind 路径重复 reset. 不修.
- `A → A`（同值）：被 `removeDuplicates()` 抑制，不会进 sink. 不修.
- `nil → nil`：no-op 分支，roster 保持 init seed（这是 RoomScaffoldView 的设计期占位语义）.
  仅在 "没进过房间" 状态生效，符合 Story 37.8 设计. 不修.

### 预防规则（Rule for future Claude）⚡

> **一句话**：在 ViewModel 用 placeholder / seed 数据（如 `RoomScaffoldDefaults`）初始化 `@Published`
> 字段时，**任何**让状态机进入"真实数据驱动"模式的路径（包括 `nil → real`、`bind` 替换 client、
> 异步注入 dependency 等）**都必须**在进入新模式的**同步路径**里**立即**清掉 placeholder，
> 不能依赖"下一条真实数据来会覆盖"——下一条数据可能 delayed / 失败 / 永远不来.
>
> **展开**：
> - 状态机的所有"进入"分支必须把"清 placeholder"作为前置步骤，与"清旧真实数据"同语义（不要因为
>   placeholder 是 init 留的就特殊对待）.
> - 写状态机分支时做对称性 audit：四态转换里如果其中 3 个分支都 reset 了某个字段，第 4 个不 reset
>   的需要写明理由（"对称性 audit failed without explicit reason" = 隐藏 bug）.
> - **测试**：每个"进入"分支必须有"snapshot 还没到达 / 永远不到达时 UI 状态"的测试，不能只测
>   happy path. 用 mock 的 gate / `connectError` 让 connect 卡住 / 抛错，断言 UI 立即处于
>   "空但真实"状态（不能保留 placeholder）.
> - **反例**：旧实装在 `nil → A` 分支只调 `prepareForReconnect()` + `wsState = .connected` +
>   `startConsumingMessages()` + spawn connect Task —— **没有** `self.members = []` /
>   `self.memberPetStates = [:]`. 同模块的 `A → B` 分支已显式 reset；`A → nil` 分支也已 reset；
>   唯独 `nil → A` 漏掉. 这种"3 个分支对称 reset 但第 4 个分支例外"就是典型 audit 漏网鱼.
> - **反例**：仅靠 happy-path 测试（"snapshot 到达后 members 正确"）覆盖 → 不能发现"snapshot 永远
>   不到达"路径下 placeholder 残留. 必须有失败路径 / delay 路径的单独 case.

## Meta: Round 11 收尾观察

Story 12.7 已经 11 轮 review，每轮揭示一个"前一轮没考虑到的边界"：

| 轮次 | 主题 | 类型 |
|---|---|---|
| r1 | try? 吞掉 sync connect failure | 错误处理 |
| r2 | leave-rejoin 后 stream 没 prepareForReconnect | 资源生命周期 |
| r3 | WS connect failure 走 errorPresenter 阻断 hit-testing | UI 状态分类 |
| r4 | stale connect failure 守护（A→B / A→nil 切换时）| 异步 stale guard |
| r5 | bind same-instance rebind / swap 区分 | 资源生命周期 |
| r6 | consumer task 重启 gated on client swap 而非 rebind | 资源生命周期 |
| r7 | RoomEndpoints percent-encode pre-escaped 输入 | 输入校验 |
| r8 | 业务错误码 fallback 必须 forward 原 error | 错误处理 |
| r9 | 异步 catch 同时守护 roomId + client identity | 异步 stale guard |
| r10 | room navigation generation token 防 ABA race | 异步 stale guard |
| r11 | nil→A 分支必须 reset scaffold roster | 状态机对称性 |

蒸馏出的元规则：**每次新增/调整一个状态转换分支后，做"对称性 audit"** —— 把所有同状态机的 in-/out-
edges 并列对照，确认副作用一致；不一致的边必须有显式技术理由或必须补齐. 这是比"逐个修 bug"更
有效率的 review 方式.
