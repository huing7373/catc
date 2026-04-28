---
date: 2026-04-27
source_review: codex review (round 6) on Story 5.5 — /tmp/epic-loop-review-5-5-r6.md
story: 5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-27 — SwiftUI 多 .task 之间无顺序保证：bind 与 start 必须在同一闭包

## 背景

Story 5.5 round 4 [P2] fix 把 `homeViewModel.start()`（拉 `/ping` 填 footer）从启动期 `.task`
挪到 `LaunchedContentView` 的 `.ready` 分支 `.task`，原意是让 ping 不进冷启动 HTTP 预算（≤2）。

但 round 4 只挪了 `start()` 的调用，**没挪 `bind(pingUseCase:)`** —— bind 仍留在 RootView 顶层
独立的 `.task` 内。codex round 6 [P2] 指出这构成一个真实的启动序列 race。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Bind ping use case before scheduling the ready-state ping | P2 | architecture | fix | `iphone/PetApp/App/RootView.swift:142-163`（修复后挪到 onReadyTask 闭包） |

## Lesson 1: SwiftUI 多 .task 修饰符之间不存在顺序保证 → 跨 .task 的"先 bind 再 start"序列必然 race

- **Severity**: P2 (medium)
- **Category**: architecture（更准确：SwiftUI 生命周期 / 并发顺序）
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift:142-163`（顶层 `.task`）和 `iphone/PetApp/App/RootView.swift:102-125`（`onReadyTask` 闭包）

### 症状（Symptom）

视图启动后，HomeView 底部 server-info footer 永远显示 `"----"` placeholder，从来不会被 `/ping`
返回的真实 build / commit 信息覆盖。SwiftUI 进入 `.ready` 状态足够快（mock / 本地 server
快路径）时必现；真机也可复现。

### 根因（Root cause）

两个独立 `.task` 修饰符跑闭包：

```swift
.task { homeViewModel.bind(pingUseCase: container.makePingUseCase()) }   // task A
.task { await launchStateMachine?.bootstrap() }                          // task B → 进 .ready
```

加上 `.ready` 分支自身又有一个 `.task`：

```swift
case .ready:
    homeView()
        .task { await onReadyTask() }   // task C：调 homeViewModel.start()
```

SwiftUI **不保证** task A、task B、task C 之间的执行顺序。当 bootstrap 跑得很快时，
执行顺序可能是 B → C → A：

1. B 完成 → 进入 `.ready`
2. C 触发 → `start()` 被调
3. `start()` 内部检查 `pingUseCase != nil` —— 此时 A 还没跑、useCase 仍是 nil
4. `start()` silent return（guard 短路）
5. A 终于跑了，但**没有任何机制再次触发 start()**
6. footer 永远 `"----"`

`bind()` 自身没有"已 bind 后立刻补跑一次 start"的语义；`start()` 也不监听 useCase 变化。
所以一旦 race 出现，session 内永远不会自愈。

### 修复（Fix）

把 `bind(pingUseCase:)` 挪进 `onReadyTask` 闭包，与 `start()` 同一个闭包内、bind 在 start 之前：

```swift
onReadyTask: {
    // bind + start 同闭包：杜绝两个独立 .task 之间的 race。
    homeViewModel.bind(pingUseCase: container.makePingUseCase())
    await homeViewModel.start()
}
```

顶层 `.task` 不再 bind ping，仅保留 LoadHomeUseCase + ErrorPresenter 的 bind（onRetry wire
启动早期就需要、非 .ready 分支）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 SwiftUI 视图里要让 "X 必须在 Y 之前发生" 时，**禁止**把 X 和 Y
> 拆到两个独立的 `.task` 修饰符里 —— SwiftUI 不保证多 `.task` 闭包之间的执行顺序，
> 只能把它们写到**同一个**闭包内，靠源代码顺序保证。
>
> **展开**：
> - SwiftUI 的 `.task` / `.task(id:)` / `.task(priority:)` 多次堆叠时，调度顺序是实现细节，
>   不是 API 契约 —— 你看到 "通常 A 先" 是巧合，fast path / mock / 测试环境会翻车。
> - 同样的陷阱也存在于 `.onAppear` + `.task` 的混用；`.onAppear` 早于 `.task`，但两个 `.task`
>   之间无顺序保证。
> - "依赖注入 → 立刻调用" 是典型的 ordering-sensitive 序列，**必须**在同一闭包内串起来。
>   抽离到上游 `.task` 做 "纯 wire"、下游 `.task` 做 "纯调用" 看似干净，实则脆弱。
> - **反例 1**：`view.task { vm.bind(useCaseA) }.task { await vm.start() }` —— start 可能比 bind 早跑。
> - **反例 2**：把 bind 放父 view 的 `.task`、start 放子 view 的 `.task` —— 子 view 的 .task
>   也不依赖父 view 的 .task 完成，仍 race。
> - **正例**：单一 `.task { vm.bind(useCaseA); await vm.start() }`，或者把 bind 移到 init / 显式
>   `await` 一个 `Task` 句柄保证完成后再 fire start。
> - **诊断 hint**：当 "footer / 某个 UI 组件偶发不更新、重启后又正常" 时，警觉是不是 ordering race，
>   先查所有 `.task` 修饰符是否分散了同一个语义序列。

## Meta

本轮（round 6）codex review 指出的两条 finding 之一。另一条（HomeData fail-fast on unknown
enum）是 dev-story 阶段就有的 schema drift 隐患，与本条无关，独立 lesson 在
`docs/lessons/2026-04-27-home-data-fail-fast-on-unknown-enum.md`。

两条都是 [P2]，和前 5 轮综合修复 regression 不同 —— 是 Story 5.5 在 dev-story 落地时就埋下的
两个独立局部问题，本轮单独被发现并修。
