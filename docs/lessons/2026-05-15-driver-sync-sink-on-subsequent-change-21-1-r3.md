---
date: 2026-05-15
source_review: codex review on Story 21-1 r2 → r3
story: 21-1-首页宝箱组件-swiftui
commit: 1da2869
lesson_count: 1
---

# Review Lessons — 2026-05-15 — Combine sink 异步 hop 破坏 happens-before：subsequent hydration 闪一帧

## 背景

Story 21-1 第 3 轮 codex review，针对 `ChestTimerDriver.swift:49-52`（r2 commit 9a603d6 引入的 `@Published` sink）的同步性缺陷。

这是同一缺陷类（hydrate flicker）**第 3 次迭代**，over-correction chain 第 3 跳：

- **r0 dev-story**：`isUnlockable = (remaining <= 0 || status == .unlockable)` —— 未初始化 0 闪 unlockable
- **r1 fix**：status-only —— 本地 tick 到 0 不切视觉态
- **r2 fix**：加 sync init + status-aware 双轴 —— **start() 那次初始化**同步了，但后续 `appState.currentChest` 变化时 sink 仍走 `.receive(on: DispatchQueue.main)` async hop
- **r3 本轮**：根治 sink 异步 hop

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `.receive(on:)` 引入异步 hop → subsequent chest hydration 闪 unlockable 一帧 | P2 | architecture | fix | `iphone/PetApp/Features/Home/ViewModels/ChestTimerDriver.swift:49-52` |

## Lesson 1: `@Published` sink 上的 `.receive(on:)` 破坏 happens-before，让 SwiftUI 看到 stale-companion view-state

- **Severity**: P2 (medium)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/ViewModels/ChestTimerDriver.swift:49-52`

### 症状（Symptom）

driver.start() 完成后，`appState.currentChest` 再次被改写（如 `HomeViewModel.refresh` / Story 21.2 LoadChestUseCase 60s 拉取装入新 `.counting` 宝箱）时，SwiftUI 观察 AppState 触发的 rerender 与 driver sink closure 的执行**没有 happens-before 关系**：

- SwiftUI 可能先看到 `currentChest = .counting(unlockAt=now+300s)`
- 但 `chestRemainingSeconds` 还是上一帧的 stale `0`
- `ChestCardView.isUnlockable(.counting, 0)` 真值表返 `true` → 闪一帧金色 unlockable 卡片
- 下个 main-runloop tick sink 跑完写入 `300` → 卡片切回 counting 灰色

视觉症状：**"金色 → 灰色"一帧抖动**，且只在 subsequent hydration 时复现（初次 start 由 r2 的同步初始化路径兜底）。

### 根因（Root cause）

**`@Published` publisher 在 setter 所在线程同步发出，本来就**不需要** `.receive(on:)`**。

r2 写代码时机械添加 `.receive(on: DispatchQueue.main)` 是 Combine 模板代码的"防御性 main-thread 切换"惯性 —— 但当：

1. publisher 来源是 `@MainActor` class 的 `@Published` 字段
2. subscriber（driver）也是 `@MainActor`
3. 所有写入路径都在 main actor（hydrate / reset / refresh 都 `@MainActor`）

`.receive(on:)` **没有意义**，反而引入了一个 dispatch hop —— sink closure 不再在 setter 调用栈内同步执行，而是排到下一个 main-runloop turn。

关键漏洞：**SwiftUI 观察 `@Published var currentChest` 是 willChange publisher 同步触发的 view invalidation 路径**，而 driver 的 sink 是普通 didChange publisher 走 dispatch 异步路径。两者**异步性不一致** → 出现 "view 看到新值 + 配套 view-state 还没更新" 的中间帧。

更深的反模式：**driver 的职责是把 domain state 派生成 view-state**。一旦 derive 过程异步化，"domain 与 view-state 一致" 这个不变量就只在 eventually consistent 下成立，不在 frame-level 成立 —— 但 SwiftUI 是 frame-level 渲染，需要 frame-level 一致性。

### 修复（Fix）

**方案 A（采用）**：删 `.receive(on: DispatchQueue.main)`。

```swift
// before (r2)
subscription = appState?.$currentChest
    .dropFirst()
    .receive(on: DispatchQueue.main)   // ← async hop
    .sink { [weak self] newChest in
        self?.handleChestChange(newChest)
    }

// after (r3)
subscription = appState?.$currentChest
    .dropFirst()
    // **禁止**在此加 `.receive(on:)` 或任何异步 operator —— 见 start() 文档 "同步 sink 契约".
    .sink { [weak self] newChest in
        self?.handleChestChange(newChest)
    }
```

理由：`ChestTimerDriver` 整类 `@MainActor` + `AppState` 整类 `@MainActor` + 所有 `currentChest` 写入路径都在 main actor → sink 闭包**默认就在 main actor 同步执行**，不需要 dispatch hop。

**配套测试**（双轴防 over-correction 反弹）：

- 已有 `testChestTimerDriverInitializesSynchronouslyOnStart`（case#7，r2 加入）—— 验证**首次 start 时**同步初始化
- 新加 `testChestTimerDriverPropagatesSubsequentChestChangeSynchronously`（case#8，本轮加入）—— 验证 **start 之后 currentChest 再次变化**时 sink 同步：

```swift
let appState = AppState()
let vm = HomeViewModel(...)
let driver = ChestTimerDriver(appState: appState, viewModel: vm)
driver.start()
// 模拟 subsequent hydration（关键场景）
appState.currentChest = HomeChest(.counting, unlockAt: +180s)
// 关键断言：setter 返回 == sink closure 已同步跑完。无 await / sleep.
XCTAssertGreaterThanOrEqual(vm.chestRemainingSeconds, 179)
XCTAssertLessThanOrEqual(vm.chestRemainingSeconds, 180)
```

任何未来 PR 重新加 `.receive(on:)` / `.async` / 其他异步 operator 都会让测试立刻挂。

**未选用的方案**：

- **方案 B**（`didSet` 替 Combine）：要侵入 `AppState.currentChest` 接口，不如 A 局部
- **方案 C**（在 ChestCardView 里自己派生 remainingSeconds）：破坏 driver 单一职责，且 driver 还是要负责 tick task

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **从 `@Published` 字段 sink 派生 view-state，且 publisher 来源与 sink 订阅者均为 `@MainActor`** 时，**禁止**加 `.receive(on: DispatchQueue.main)` —— 它没有意义还引入 dispatch hop，破坏 SwiftUI 渲染与 view-state 写入的 happens-before 关系。

> **展开**：
>
> - **判定条件**：publisher 来源类标 `@MainActor` + subscriber 类标 `@MainActor` + 所有 setter 路径都在 main actor → `.receive(on:)` 多余
> - **后果**：SwiftUI 的 willChange 走同步路径触发 view invalidation；driver 的 sink 走 dispatch 异步路径 → view 看到新 `domainState` 但配套 `viewState` 还是上一帧 stale → frame-level 不一致 → 视觉抖动一帧
> - **判定 view-state 派生路径是否安全**：start() 之后任意 setter 调用 → 在同一 stack 内 `viewModel.<derived>` 是否已经写新值？没写新值 = 不安全。
> - **双轴测试守门**：（a）首次 start 同步初始化（CurrentValueSubject 风格的"立即拿当前值"）+（b）start 之后 setter 同步触发 sink。两个测试都用**无 await / 无 sleep** 的同步断言，任何异步化反弹立刻挂。
> - **反例 1**：写 `.receive(on: DispatchQueue.main)` 在 `@MainActor → @MainActor` 链上"以防万一" —— 没有"万一"，只有"必坏"。
> - **反例 2**：测试用 `try? await Task.sleep(nanoseconds: 50_000_000)` 验证 sink 触发 —— 这种测试**无法**捕捉异步 hop 引入的 frame-level race，只能验证 eventually consistent，过不了 review；必须用同步断言（setter 返回即断言）。
> - **反例 3**：用 `didSet` 反向通知 driver 来"绕过 Combine 异步" —— 侵入 model 接口，且没修根因（根因不是 Combine，是 `.receive(on:)` 多余）。
> - **泛化领域**：同一规则适用于任何"domain → view-state 派生 driver"模式，包括但不限于：`MotionStateMapper`（Story 8.4）/ `WSStateMapper` / 未来 epic 21.2+ 的 `ChestRefreshScheduler` 等。所有 `@MainActor → @MainActor` 的 Combine 链路都遵守"无 `.receive(on:)`"约定。

---

## Meta: 本次 review 的宏观教训（over-correction chain 第 3 跳的根本经验）

Story 21-1 走到 r3 才修完一个看似简单的 hydrate flicker，连续 3 轮迭代每一轮都引入新缺陷：

- r0: 业务真值表错（用默认值 0 派生）
- r1: 业务真值表过度修正（去掉 0 → 0 也是合法的本地 tick 状态）
- r2: 加同步初始化（修了 start() 那次），但 sink 链上的 `.receive(on:)` 留作 async hop（subsequent 变化仍有竞态）
- r3: 删 `.receive(on:)`，根治异步 hop

**反思**：r1 → r2 → r3 都是"修了局部 case，没修架构层面的 timing 不变量"。**view-state 与 domain state 必须 frame-level 一致**是这类 driver 的硬约束 —— 写 driver 时第一时间应该问的不是"Combine 模板代码长什么样"，而是"setter 返回时，所有 derived view-state 是否已经同步写完？SwiftUI 下个 rerender 是否会看到一致快照？"

具体的"防 over-correction"做法（已在 Story 20-9 r6/r7 沉淀）：**测试反弹守门** —— 每修一轮加一个"被破坏就立刻挂"的同步断言测试。本 lesson 的 case#7 + case#8 就是这种双轴守门。
