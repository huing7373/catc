---
date: 2026-04-30
source_review: codex review (epic-loop round 2 for Story 37.8 RoomView Scaffold)
story: 37-8-roomview-scaffold
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-30 — RootView 持有的 ViewModel 必须在第一次 paint 之前同步 bind AppState

## 背景

Story 37.8 RoomView Scaffold round 2 codex review。

round 1 [P1] fix 已让 `RootView.@StateObject roomViewModel = RealRoomViewModel()` 走 parameterless init + Real 实例，让 `onLeaveTap`/`onCopyTap` override 落地（不再走基类 fatalError 路径），AppState 通过 `.task` 内 `bind(appState:)` 异步注入。round 2 codex 指出此实装仍有**启动期 race**：当 `appState.currentRoomId != nil`（restored in-room session / `UITEST_FORCE_IN_ROOM` env / `/home` 返回非 nil currentRoomId）时，HomeContainerView 在 `.task` 跑之前已经按 currentRoomId 走 inRoom 分支 → RoomScaffoldView 渲染 → RealRoomViewModel.appState 仍 nil → leave tap 无效 + room title/code 显示 placeholder。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | RealRoomViewModel launch-time race | medium | architecture | fix | `iphone/PetApp/App/RootView.swift` |

## Lesson 1: RootView 持有的 ViewModel 必须在第一次 paint 之前同步 bind AppState

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift:55` + RootView.body `.onAppear` / `.task`

### 症状（Symptom）

启动时 `appState.currentRoomId != nil`（任一触发条件）：
- HomeContainerView 第一帧 paint 已按 `appState.currentRoomId` 决策走 inRoom 分支 → RoomScaffoldView 渲染
- 此时 `roomViewModel: RealRoomViewModel`（parameterless init）的 `appState` 字段仍为 nil（`.task` 内 `bind(appState:)` 还没触发）
- 用户立即点 leave/back → `onLeaveTap()` 内 `self.appState?.setCurrentRoomId(nil)` 是 nil-coalescing no-op → silent 失败
- room title/code 显示 RoomScaffoldDefaults 占位（不是真房间号）

触发条件至少 3 个，不算 corner case：
1. `/home` 返回 `room.currentRoomId != nil`（用户上次活跃在房间内 + 重启 app；server 状态恢复）
2. `UITEST_FORCE_IN_ROOM` env flag → `ensureLaunchStateMachineWired` 立即 `appState.setCurrentRoomId("1234567")` → ready 子树第一帧已是 inRoom
3. 未来 Story 12.x JoinRoomModal 落地后任何 manual debug 路径

### 根因（Root cause）

SwiftUI 的 `.task` 与 `.onAppear` 触发时序差异：
- `.onAppear`：第一次 paint **之前**同步执行（与 view 生命周期同步）
- `.task`：第一次 paint **之后**异步执行（在新 Task 上调度，至少跨一个 RunLoop tick）

round 1 把 `bind(appState:)` 调用放在 `.task` 内是按"和 pingUseCase / loadHomeUseCase 同模式"的惯性写法 —— 但那两个 bind 依赖 `container.makePingUseCase()` / `container.makeLoadHomeUseCase()`（容器异步构造），合理走 `.task`。`bind(appState:)` 不依赖任何异步资源 —— `appState` 已经是同级 `@StateObject`，body 第一次构造时它就在 —— 没有理由放在 `.task` 内。

更深层原因：在 round 1 设计时关注的是"互斥状态机切到 inRoom"路径**用户操作后**的那次切换（点 join → applyHomeData → currentRoomId 写入 → state 切到 inRoom），那条路径下 `.task` 在切换前早已跑过。但**启动期** restored / forced inRoom 路径下，inRoom 是**第一帧**就成立的，没有"切换之前的窗口"留给 `.task` 跑。

### 修复（Fix）

把 `homeViewModel.bind(appState:)` 和 `realRoomVM.bind(appState:)` 从外层 `.task` 移到外层 `.onAppear`（保留 `bind(loadHomeUseCase:errorPresenter:)` 在 `.task` 内 —— 它确实依赖容器构造）：

```swift
// before (round 1): 异步注入路径
.onAppear { ensureLaunchStateMachineWired() }
.task {
    homeViewModel.bind(loadHomeUseCase: ..., errorPresenter: ...)
    homeViewModel.bind(appState: appState)
    if let realRoomVM = roomViewModel as? RealRoomViewModel {
        realRoomVM.bind(appState: appState)
    }
}

// after (round 2 fix): bind(appState:) 同步路径
.onAppear {
    homeViewModel.bind(appState: appState)
    if let realRoomVM = roomViewModel as? RealRoomViewModel {
        realRoomVM.bind(appState: appState)
    }
    ensureLaunchStateMachineWired()
}
.task {
    homeViewModel.bind(loadHomeUseCase: ..., errorPresenter: ...)
}
```

复用现有 idempotent `bind(appState:)` 入口（`alreadySubscribed` guard 保护），不动 ViewModel 持有结构。

守护测试新增 `RoomViewScaffoldTests.testRealRoomViewModelBindAppStateIsSynchronous`：用 Mirror 反射验证 `RealRoomViewModel().bind(appState:)` 是**纯同步**入口 —— 调用之后 private `appState` 字段立即非 nil + 紧接着 `onLeaveTap()` 立即能写 `currentRoomId=nil`。若未来重构把 bind 改 async / 把 sink dispatch 延后，本测试会立即失败 → 提示同步注入契约破坏。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **RootView（或任何"窗口顶层 + State 互斥状态机决策"View）持有的 ViewModel 通过 bind(appState:) 注入 AppState** 时，**必须**把 bind 调用放在 **`.onAppear`** 而非 **`.task`**，除非该 bind 调用真依赖异步资源构造。
>
> **展开**：
> - `.onAppear` 在第一次 paint **之前**同步执行；`.task` 在第一次 paint **之后**异步调度。差异约一个 RunLoop tick + Task 调度延迟，但当**第一帧**就需要 ViewModel 已绑（如启动期 restored state / UITest force flag）时这一 tick 就是 race。
> - 区分两类 bind：
>   - **同步 bind**（pure ref injection）→ `.onAppear`：`bind(appState:)` / `bind(observerStore:)` 等接收已就位 `@StateObject` 的入口
>   - **异步 bind**（依赖容器 / async UseCase 构造）→ `.task`：`bind(loadHomeUseCase: container.makeX(), ...)`、`await someAsyncSetup()`
- "和 X 同模式" 不是放 `.task` 的理由 —— 看 X 的 bind 依赖什么。
- 启动期 inRoom path 的触发条件不算 corner case，至少 3 个：（a）restored in-room session（server `/home` 返回 currentRoomId）、（b）`UITEST_FORCE_IN_ROOM` env、（c）未来 join flow 落地后的 manual debug 路径。其中 (a) 是**普通用户**主路径（用户重启 app 仍在房间内）。
- **反例**（避免）：
   - 把 `bind(appState:)` 放进 `.task` 闭包 + 把"互斥状态机决策"View（HomeContainerView 等）放在 ready 子树 → 启动期第一帧 race
   - 在 ViewModel 内用 sink dispatch 把"接住 appState 引用"延迟到下一个 RunLoop tick（即"bind 同步赋值，sink 异步建立"是好的；但"bind 把赋值本身延后"会重新引入同样的 race）
   - 用 `await Task { @MainActor in ... }` 包裹 bind 调用 —— 即使在 `.onAppear` 内，加了 await/Task 也会变回异步路径
   - 测试用 `await Task.yield()` / `try await Task.sleep` 等技巧"放给 bind 跑完"绕过 race —— 这只验证了"bind 跑完之后状态对"，没验证"第一帧状态对"
- **正例参考**：本 fix 后的 `RootView.swift` 的 `.onAppear` 内 `bind(appState:)` + `.task` 内 `bind(loadHomeUseCase:)` 双轨；`RoomViewScaffoldTests.testRealRoomViewModelBindAppStateIsSynchronous` 的 Mirror 反射守护契约模式。

## Meta: 与 round 1 lesson 的关系

round 1 lesson `2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md` 解决的是"in-room path 渲染时 4 个 mock member 占位都到位"的视觉初值问题；round 2 这条 lesson 解决的是"in-room path 切到 RoomScaffoldView 后 ViewModel 真的连上 AppState"的引用注入时序问题。两者互补，不替代：
- 没 round 1 fix：bind 早 + 视觉初值缺 → 第一帧仍渲染空房间（即使 leave 按钮可工作）
- 没 round 2 fix：视觉初值在 + bind 晚 → 第一帧渲染 4 mock member（看起来 ok）但 leave 按钮 silent + room code 是占位
- 两个 fix 都到位：第一帧渲染 4 mock + room code 来自 appState.currentRoomId（restored 用户看到真房号）+ leave 按钮立即可用

未来 RootView 再加新 ViewModel（如 Story 12.1 真接 WS 后的 RoomViewModel 真版本）时，**两条规则同时生效**：visual seed + sync bind。
