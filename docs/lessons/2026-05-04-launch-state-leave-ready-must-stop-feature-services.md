---
date: 2026-05-04
source_review: codex review round 2 — Story 8.5 dev-story output（base=story 8-5 r1 fix commit）
story: 8-5-步数同步触发器
commit: f43bc89
lesson_count: 1
---

# Review Lessons — 2026-05-04 — launch state 离开 .ready 时必须 stop 所有 .ready-attached 后台 service（对称生命周期）

## 背景

Story 8.5（步数同步触发器）落地时把 `StepSyncTriggerService` 由 RootView `@State` 持有，
通过 `LaunchedContentView.onReadyTask` 在 `.ready` 状态下调 `service.start()` 启动 5min 定时同步。
但**没有**对称的 stop 路径：当 token expiry 401 → AuthBoundaryAPIClient 调
`unauthorizedHandlerSink` → `AppLaunchStateMachine.triggerColdStart()` 把 state 拉回 `.launching`
甚至 `.needsAuth(presentation)` 时，老 timer 仍每 5min 跑一次 sync，用已被 `sessionStore.clear()`
清掉的 token → 又触发 401 → 又触发 cold-start，形成自激循环；
即便用户停在 LaunchingView / RetryView 上，`scenePhase .active` listener 在下次 active 边沿
仍会无脑调 `service.start()` 把 timer 重启。codex round 2 [P2] 命中。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Fresh install HealthKit requestPermission gap（spec gap） | P1 | architecture | wontfix | `iphone/PetApp/Features/Home/UseCases/SyncStepsUseCase.swift:56-57` |
| 2 | service.start() 没对称 stop()，launch state 离开 .ready 时 timer 仍跑 | P2 | architecture | fix | `iphone/PetApp/App/RootView.swift:160-164` |

Finding 1 是 round 1 已 wontfix 决议在 round 2 的二次复现 —— codex 不知道我们已 wontfix（spec gap
归 epic 切片层，不在本 story 范围）；既有 lesson `2026-05-04-step-sync-fresh-install-requestpermission-gap.md`
+ `2026-05-04-auth-gated-feature-needs-explicit-requester-story.md` 已完整记录决议链路，本次
**不写新 lesson**，**不改任何代码**，commit message 中明示"重现于 round 2，与 round 1 resumed
同 wontfix 决策"。本文件仅落 Finding 2 的 lesson。

## Lesson 1: 由 RootView 顶层在 .ready 启动的 service，**必须**有对称的"离开 .ready 时 stop"路径

- **Severity**: P2
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift:160-164`（onReadyTask）+ 247-278（scenePhase listener）

### 症状（Symptom）

`StepSyncTriggerService` 的 5min 定时同步 timer 在以下场景仍会跑：

1. token 过期 401 → 401 handler 调 `triggerColdStart()` → `AppLaunchStateMachine.state` 切回
   `.launching`；UI 渲染 LaunchingView；service 的 timerTask 仍 alive，5min 后调
   `syncStepsUseCase.execute()` → 401 → 又一次 cold-start → 自激循环；
2. cold-start 进一步失败 → state 切到 `.needsAuth(presentation:)`；用户停在 RetryView/TerminalErrorView
   上，service timer 仍每 5min 跑一次同步（本质是垃圾请求 + 资源浪费）；
3. 用户后台/前台切换：`scenePhase` `.active` listener 即便此时 launch state 不在 `.ready`，
   也会调 `service.start()` 重启 timer（绕过 1/2 的 stop 也能重新启）。

### 根因（Root cause）

**生命周期不对称**。RootView 在 `.ready` 顶层的 `onReadyTask`（即 `LaunchedContentView` `.ready`
分支的 `.task`）调 `service.start()`，但**没有**在 `.ready` 离开时调 `service.stop()` 的对称路径。

SwiftUI 的 `.task` 修饰符在 view 不再渲染时会自动 cancel 它派出的 Task，但**不会**通过任何
out-of-band 方式调用关联 service 的 stop —— `start()` 内启动的 `timerTask` 是用 `Task { ... }`
而非 `.task` body 自己 await 的，**与 view 生命周期解耦**；service 由 RootView `@State`
持有，state machine 切到 `.launching` / `.needsAuth` 不会让 RootView 卸载，service 自然不会 deinit。

`scenePhase` listener 把 `service.start()` 当作"前台时该跑的事"来调，但**它不知道当前是否
真的在 .ready 流程内** —— 它只看 phase 边沿，不看 launch state 维度。

这是一个典型的 "cross-cutting service 必须感知 cross-cutting 状态门" 的盲点：
- ✅ 懂 phase（前台/后台）
- ❌ 漏 launch state（.ready / .launching / .needsAuth）

两个维度独立，service 只 gate 一个就会漏。

### 修复（Fix）

在 RootView + LaunchedContentView 加两条对称路径：

1. **`LaunchedContentView` 内监听 `stateMachine.state` 切换**，当从 `.ready` 离开时调
   `onLeaveReady` 回调（新参数）→ 外层 RootView 调 `stepSyncTriggerService?.stop()`：
   ```swift
   .onChange(of: stateMachine.state) { oldState, newState in
       let wasReady = (if case .ready = oldState { true } else { false })
       let isReady  = (if case .ready = newState { true } else { false })
       if wasReady && !isReady { onLeaveReady() }
   }
   ```
   重新进入 `.ready` 时 `.task` modifier 重新触发 `onReadyTask` → 重新调 `service.start()`
   （既有路径，幂等）.

2. **`scenePhase` `.active` 路径加 `.ready` gate**：
   ```swift
   if launchStateMachine?.state == .ready {
       stepSyncTriggerService?.start()
   }
   ```
   不在 `.ready` 时即便切回前台也不重启 timer；下次真正进入 `.ready` 时由 `onReadyTask` 启动。

3. service 的 `stop()` 已经做了 `timerTask?.cancel(); timerTask = nil; hasStartedTimer = false`，
   天然幂等 —— 多次 stop 安全；stop 后再 start 视为新 firstStart 路径（重启 timer + 触发一次 sync）。
   单测 `testStart_thenStop_thenStart_rebindPathWorks` + `testStop_idempotent_safeToCallTwice` 覆盖。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 RootView 顶层（或任何"app 级 state machine 的子状态分支"）启动后台
> service / timer / subscription 时，**必须**配对设计三条路径：
>   1. 进入子状态 → start
>   2. 离开子状态 → stop（**通过 `.onChange(of: stateMachine.state)` 显式监听，不能依赖 view 卸载隐式 cleanup**）
>   3. cross-cutting trigger（如 scenePhase `.active`）→ **gate 在子状态门上**，不能无脑 start
>
> **展开**：
> - SwiftUI `.task` modifier 自动 cancel 它的 `body` Task，但**不会** cascade 给 service 内部
>   `Task { ... }` 派出的子 Task —— 后者必须由 service 自己的 stop / deinit 显式 cancel；
> - app 级 state machine（如 `AppLaunchStateMachine`）在 401 / network failure 等路径会把 state
>   拉回上游分支（如 `.ready` → `.launching` / `.needsAuth`），但**view 树不一定卸载** —— RootView
>   会一直 alive，因此 service 的 `@State` storage 也一直 alive，不会自动 deinit；
> - cross-cutting trigger（scenePhase / network reachability / app foreground notification）
>   **必须 gate 在最严格的子状态门上** —— 否则会绕过 stop 路径独立 restart；
> - service `stop()` 必须**幂等**（多次 stop 安全 + stop 后 start 走完整 firstStart 路径）；
> - 单测必须覆盖 `start → stop → start` 序列（rebind path）+ `stop()` 多次调用幂等性，
>   否则只测 `start → stop` 而漏 stop 后 start 是否真活了；
>
> **反例**（务必避免）：
> - `RootView .task { service.start() }` + 没有任何 `.onChange(of: stateMachine.state) {
>   if !isReady { service.stop() } }` —— 这是本 lesson 修复前的实装；
> - `scenePhase .active → service.start()` 不带 `if state == .ready` gate —— 即便已对 onLeaveReady
>   stop，scenePhase 仍会 silently restart；
> - 把 service 生命周期"绑定"到 `.task` 的隐式 cancel 上：`.task` 只 cancel 它 body 内 await
>   的 Task，service 内独立派出的 `Task { ... }`（持 `[weak self]` 的 timer loop）**不会**被
>   `.task` cancel，timer 会孤儿化；
> - 信任"401 handler 自己会清 service"：401 handler 只清 sessionStore + 重置 launch state，
>   它**不知道** RootView 持有了哪些 feature service —— service 拥有者必须自己监听 launch state。

---

## Meta: 本次 review 的宏观教训

`/fix-review` round 2 在 round 1 fix 的基础上跑 codex baseline；codex 不知道我们已 wontfix
某些 round 1 finding（如 fresh install HealthKit requestPermission gap），会**重新 surface
同一条 finding**。处置原则：

1. **不重写 lesson**：既有 wontfix lesson 已完整记录决议链路（含技术理由 + spec gap 归属），
   round 2 重现不引入新信息 → 仅在 commit message 中 explicit 写"重现于 round 2，与 round 1
   resumed 同 wontfix 决策"，并保留既有 lesson 不动；
2. **不改代码**：wontfix 决议在 round 1 已落地（含 epic 切片层 spec gap 的 process lesson）；
3. **新 lesson 仅给真正新 fix**：本次只有 Finding 2（service 对称 stop）是新 fix，故本文件
   仅落一条新 lesson。

这条 meta 教训本身值得记一笔：codex round-N 时若发现 finding 与 round-M (M<N) 的 wontfix
决议同一条，**不要**怀疑既有 wontfix 决议，**不要**重写已有 lesson —— 直接 reaffirm + 推进
到下一条 finding 即可。否则 lesson 库会在每个 round 复制粘贴同一份决议。
