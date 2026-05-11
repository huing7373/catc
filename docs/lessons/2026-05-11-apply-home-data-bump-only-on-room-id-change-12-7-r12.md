---
date: 2026-05-11
source_review: codex review r12 on story 12-7-创建-加入-退出-use-case-主界面入口完善
story: 12-7-创建-加入-退出-use-case-主界面入口完善
commit: 50a35d6
lesson_count: 1
---

# Review Lessons — 2026-05-11 — `applyHomeData(_:)` 仅在 `currentRoomId` 实际变更时 bump generation token（12-7 r12）

## 背景

Story 12.7 round 10 引入 monotonic `roomNavigationGeneration` token 替换 `currentRoomId` equality 作为 in-flight room request 的 stale-guard（详见 lesson `2026-05-11-room-navigation-generation-token-not-room-id-equality-12-7-r10.md`）。r10 实装中 `applyHomeData(_:)` 与 `setCurrentRoomId(_:)` / `reset()` 同等地 **无条件** bump generation —— 当时的想法是"hydrate 也算 navigation cycle"。

Round 12 codex review 指出这是过激设计：`applyHomeData` 是**所有** `/home` 数据 hydrate 的统一入口（包括 `HomeViewModel.loadHome()` retry 和 `RootView` bootstrap/cold-start path 的常规刷新），这些场景与 room navigation 完全无关。无条件 bump 会让 user / pet / stepAccount / chest 等字段的常规 update **看起来像** navigation event，使 in-flight 的 create/join/leave 合法 response 被 exact-equality guard 误判 stale 而丢弃。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `applyHomeData(_:)` 无条件 bump `roomNavigationGeneration` 致非 navigation hydrate 误伤 in-flight room request | P2 | architecture | fix | `iphone/PetApp/App/AppState.swift:79-91` |

## Lesson 1: `applyHomeData(_:)` 必须仅在 `currentRoomId` 实际变更时 bump generation token

- **Severity**: P2（medium）
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/App/AppState.swift:79-91`

### 症状（Symptom）

`applyHomeData(_:)` 内无条件 `self.roomNavigationGeneration &+= 1`：

```swift
public func applyHomeData(_ data: HomeData) {
    self.currentUser = data.user
    self.currentPet = data.pet
    self.currentStepAccount = data.stepAccount
    self.currentChest = data.chest
    self.currentRoomId = data.room.currentRoomId
    self.roomNavigationGeneration &+= 1  // ❌ 即使 currentRoomId 不变也 bump
}
```

下游 `CreateRoomUseCase` / `JoinRoomUseCase` / `LeaveRoomUseCase` 及对应 ViewModel error-handler 都依赖 `entryGen == liveGen` exact-equality 作为 stale-guard。当 `HomeViewModel.loadHome()` retry 或 `RootView` bootstrap path 在用户处于 room flow 期间触发 hydrate 时：

```
T0: user 点击「创建队伍」→ entryGen = 7 → 异步 await POST /room/create
T1: 同时 HomeViewModel.loadHome() 因网络抖动 retry 完成
    → applyHomeData → generation 7 → 8（但 currentRoomId 未变）
T2: T0 的 POST /room/create response 返回 → liveGen = 8 ≠ entryGen = 7
    → 合法 response 被丢弃 → UI 卡在 loading + room 创建静默成功但本地无感知
```

### 根因（Root cause）

r10 把"hydrate 也算 navigation cycle"等同于"navigation event"。两者语义边界不同：

- **navigation event**：用户主动触发的 room 状态转换（create / join / leave），或服务端推送的房间隶属变更（如被踢出 → `currentRoomId` 从 X → nil）。**这些必须** invalidate in-flight room request。
- **hydrate refresh**：周期性 / 启动 / 错误 retry 拉取的 `/home` 全量数据。**绝大多数**情况下 `currentRoomId` 维持原值（用户没离开 room 上下文），只是 user/pet/step/chest 等字段被刷新。**这些不应** invalidate in-flight room request。

判定 navigation event 的最准确 signal 不是"调了 applyHomeData"，而是"`currentRoomId` 字段实际变化了"。

### 修复（Fix）

`applyHomeData(_:)` 改为先取旧值 / 写新值 / **比较后**条件 bump：

```swift
public func applyHomeData(_ data: HomeData) {
    self.currentUser = data.user
    self.currentPet = data.pet
    self.currentStepAccount = data.stepAccount
    self.currentChest = data.chest

    let oldCurrentRoomId = self.currentRoomId
    self.currentRoomId = data.room.currentRoomId
    if data.room.currentRoomId != oldCurrentRoomId {
        self.roomNavigationGeneration &+= 1
    }
}
```

`setCurrentRoomId(_:)` 与 `reset()` 路径**保持无条件 bump**：
- `setCurrentRoomId`：显式 room flow 入口（用户主动操作），不变值也必须 bump 以处理 ABA race（leave A → re-join A，`currentRoomId` A → nil → A，generation 必须严格单调）。
- `reset()`：用户登出 / 切身份是显式 navigation event；reset 路径上下文中即便有 in-flight room request，被丢也合理（用户已经离开当前身份 context）。

回归测试新增 5 条覆盖矩阵（`iphone/PetAppTests/App/AppStateTests.swift`）：
1. `testApplyHomeDataDoesNotBumpWhenRoomIdUnchanged` — X → X 不 bump
2. `testApplyHomeDataBumpsWhenRoomIdChangesFromNilToValue` — nil → X bump
3. `testApplyHomeDataBumpsWhenRoomIdChangesFromValueToNil` — X → nil bump
4. `testApplyHomeDataDoesNotBumpWhenRoomIdStaysNil` — nil → nil 不 bump
5. `testApplyHomeDataBumpsWhenRoomIdChangesBetweenValues` — X → Y bump

旧测试 `testRoomNavigationGenerationBumpsOnHydrateAndReset` 拆为 `testRoomNavigationGenerationBumpsOnReset`（仅留 reset 路径），applyHomeData 部分被上述 5 条用例覆盖。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计 monotonic generation token 用作 stale-guard** 时，**必须** **把 bump 入口严格限定在"目标 invariant 实际发生变化"的代码路径**，不可以"涉及该 invariant 的所有 setter / hydrate 入口都 bump"。
>
> **展开**：
> - generation token 的语义是"该 invariant 的 transition 次数"，不是"该 invariant 所在 data structure 的 mutation 次数"。前者准确，后者过激。
> - 判断标准：在 setter / hydrate path 内**比较旧值与新值**；只有真实变化才 bump。
> - 显式 navigation entry（如 `setCurrentRoomId` / `reset`）可以**无条件** bump —— 这些 path 的调用本身就是用户意图的 explicit 表达，且需要处理 ABA race。
> - 全量数据 hydrate path（如 `applyHomeData`）**必须**字段级判断 —— 这些 path 大多数情况是周期 refresh，不带 navigation 语义。
> - **反例**：
>   - r10 落地的 `applyHomeData` 内无条件 `generation &+= 1`，导致 `HomeViewModel.loadHome()` retry 期间用户的 create/join/leave 合法 response 被误判 stale → 用户体验：点了创建房间，loading 不消失，但服务端实际已创建成功。
>   - 推而广之：任何"WS message 处理入口都 bump generation"的设计 → 心跳 / pong / 非业务 frame 都会触发 bump → 业务 frame 的 stale-guard 全失效。
> - **设计自检 checklist**（generation token 引入时强制走一遍）：
>   1. 列出所有可能触达 bump 的代码路径
>   2. 对每条 path 标注"调用方意图"（用户显式 navigation / 系统刷新 / 错误 retry / 第三方推送 / ...）
>   3. 只保留"用户显式 navigation"和"第三方推送显示状态实际变化"两类 path 做无条件 bump
>   4. 其余 path（hydrate refresh / retry / 健康检查）做字段级 diff 后条件 bump
>   5. 写至少 1 条"该 path 不应 bump"的负面用例，确保 guard 没被过激扩张

---

## Meta: 引入 invariant-based stale-guard 时的元教训

generation token vs equality guard 不是一个"换实现"的简单 refactor，而是改变了**所有 bump 入口的语义**。r10 引入 token 时只验证了"两个 setter 都 bump 后 ABA race 解决"，没复核"applyHomeData 也 bump 是否会引入 hydrate-误伤 in-flight"。

未来 Claude 在引入 token-based race-guard 时，除了正面测"该 token 能解决的 race"，还要**反面测**"该 token 在系统正常运行的 idle / retry / hydrate path 上是否会无端 invalidate 合法操作"。这两个方向的测试缺一不可。
