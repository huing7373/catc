---
date: 2026-04-26
source_review: codex review round 2 on Story 2.5 (file: /tmp/epic-loop-review-2-5-r2.md)
story: 2-5-ping-调用-主界面显示-server-version-信息
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — SwiftUI `.task` 在 view 重新出现时会重启，"一次性"语义需 ViewModel 自己 short-circuit

## 背景

Story 2.5 落地"App 启动时 ping server 取 commit + 显示 footer"的链路。RootView 的 `.task` modifier 调
`homeViewModel.bind(pingUseCase:)` + `await homeViewModel.start()`。round 1 时 `start()` 的防重入只用了
`pingTask != nil` 短路（防同一 `.task` 内并发触发），并加了 `bind()` 单次生效防重复注入。

round 2 codex 发现：当 RootView 被 `.fullScreenCover` 覆盖（即用户点 Room / Inventory / Compose CTA），
sheet 关闭后 SwiftUI 会**重启** `.task`。此时上一轮 `pingTask` 已完成置 nil → 新一轮 `start()` 跑通"短路"
检查 → 重新发起 `/ping` + `/version`。结果：每次 sheet 关闭都额外触发一次 ping，与"一次性启动探针"语义违背。

注意区分两个并发模型：
- **同一 .task 边界内的并发**：用 `pingTask != nil` 即可短路。
- **跨 .task 边界（view 重启）**：`pingTask` 不能用，因为每次重启时都已经被前一轮 reset 成 nil。需要一个
  **状态变量**（不依赖 task lifecycle）记录"已完成过一次"。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | SwiftUI `.task` 在 view 重新出现时会重启，"一次性 ping" 语义需 ViewModel 加 hasFetched flag | medium (P2) | concurrency / SwiftUI lifecycle | fix | `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`、`iphone/PetAppTests/Features/Home/HomeViewModelPingTests.swift` |

## Lesson 1: SwiftUI `.task` 在 view 重新出现时会重启，"一次性 side effect" 必须由调用层加显式状态防重入

- **Severity**: medium (P2)
- **Category**: concurrency / SwiftUI lifecycle
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`（`start()` 方法）

### 症状（Symptom）

```swift
// before（round 1 fix 后的状态）
public func start() async {
    let useCase = pingUseCase ?? boundPingUseCase
    guard let useCase = useCase else { return }
    guard pingTask == nil else { return }   // 仅同一 .task 边界内并发短路

    let task = Task { [weak self] in
        let result = await useCase.execute()
        await self?.applyPingResult(result)
    }
    pingTask = task
    await task.value
    pingTask = nil   // <-- 任务结束 reset 后，下一次 view 重启 .task 还会再跑一次
}
```

复现路径：
1. App 启动 → RootView `.task` 触发 `start()` → 跑一次 ping。✓
2. 点 Inventory CTA → `.fullScreenCover` 弹 sheet。RootView 被覆盖。
3. 点 sheet 的 Close → 回到 RootView。SwiftUI 重启 `.task`。
4. `start()` 重跑：`pingTask` 已是 nil（步骤 1 末尾 reset），短路检查不生效 → 又发一次 ping。✗
5. 用户每次开关任一 sheet，都触发一次额外 ping。错。

### 根因（Root cause）

SwiftUI `.task(priority:_:)` 文档明确：task 的生命周期与 view 的 onAppear/onDisappear 绑定，view 重新出现
（reappear）会**重启** task body —— 这是 by design 的，不是 bug，目的是让 view 短期消失再回来时能重新
订阅数据流（如 `for await` AsyncSequence）。

但**业务语义**有两类：
- **持续订阅型**（live data stream / WebSocket / async iterator）：reappear 重启**正确**，原 task 已被
  cancel，重启接续订阅。
- **一次性 side effect**（启动探针 / boot-time ping / one-shot 配置加载）：reappear 重启**错误**，业务
  上只想跑一次。

`pingTask != nil` 这种"基于 task 引用的并发短路"只能 cover 第一类需求里"同一次 task body 内被并发调"
的场景。第二类需求（一次性）必须要在 ViewModel 层加**独立于 task lifecycle 的状态变量**记录"已经跑过
一次"。

把 ping 提到 `App.init` 或 `@main` 入口也能 work，但 Story 2.5 ADR-0002 已锁定"DI 经 AppContainer →
RootView .task 注入"模式，提到 `App.init` 会破坏分层（AppContainer 还没初始化）。最干净的办法是 **`start()`
里加 `hasFetched` flag**。

### 修复（Fix）

```swift
// after
private var hasFetched: Bool = false

public func start() async {
    let useCase = pingUseCase ?? boundPingUseCase
    guard let useCase = useCase else { return }
    guard !hasFetched else { return }      // 跨 task 边界短路：已完成一次直接 return
    guard pingTask == nil else { return }  // 同一 task 边界内并发短路

    let task = Task { [weak self] in
        let result = await useCase.execute()
        await self?.applyPingResult(result)
    }
    pingTask = task
    await task.value
    pingTask = nil
    hasFetched = true   // 失败也置 true：避免不可达 server 时反复重试
}
```

两条短路防御层级清晰：
- `pingTask != nil`：cover "并发同时调 start() 两次"（async let 同时跑）。
- `hasFetched`：cover "前一次跑完 → view 重启 .task → 重新调 start()"。

**为什么失败也置 hasFetched=true**：避免不可达 server（如真机上默认 localhost）时每次开关 sheet 都触发
一次失败请求 + 错误日志爆刷。错误恢复 UI（"点击重试"）由 Story 2.6 负责，本 ViewModel 只保证语义层的
"一次性"。

测试覆盖（新增 2 个 case）：
- `testStartShortCircuitsAfterFirstCompletion`：成功后 executeCallCount 维持 1。
- `testStartShortCircuitsAfterFailure`：失败后 executeCallCount 也维持 1。

### 预防规则（Rule for future Claude）⚡

> **一句话**：在 SwiftUI `.task` 里调"一次性 side effect"（boot-time ping / 配置加载 / 引导请求）时，
> **不能**只用 `Task` 引用做并发短路；必须在 ViewModel 加独立的 `hasFetched: Bool`（或语义等价的状态
> 变量）跨 task lifecycle 记录"已执行过一次"，否则 view 重启 `.task`（sheet 关闭、tab 切换、navigation
> pop）时会重复触发。

> **展开**：
>
> - **触发条件**：你写了 `.task { await viewModel.someStartLikeMethod() }`，且 `someStartLikeMethod`
>   的语义是"一次性"（启动 ping、加载 config、读 token、首次同步）。停下来问："view 在生命周期内会被
>   覆盖然后重新出现吗（fullScreenCover / sheet / NavigationStack push 后 pop）？" 答案是"会"，立刻
>   加 `hasFetched` 短路。
> - **判别"一次性"vs"持续订阅"**：能用 `await viewModel.start()` 单次完成 → "一次性"，需要 hasFetched；
>   用 `for await item in stream` 持续订阅 → "持续订阅"，重启 .task 是正确语义，不要加 hasFetched。
> - **`Task` 引用 != "已完成一次"**：`Task` 引用只能告诉你"现在有没有任务在跑"，不能告诉你"过去有没有
>   跑过"。前者是 transient state，后者是 historical state。后者必须用独立的 Bool / Counter / enum
>   存储。
> - **失败也算"已 fetched"**：避免错误循环（不可达 server 时每次 view 重启都重发请求）。错误恢复策略
>   归属"重试 UI" 这个独立 concern；ViewModel 的"一次性"语义不应被错误路径污染。如果业务确实要"失败
>   后允许手动重试"，加显式 `retry()` 方法（重置 hasFetched + 重新调 start()），不要让 `.task` 自动
>   重试。
> - **测试要 cover 跨 task 边界**：写两个 case：(1) "成功后再调 start() 仍只 execute 1 次"，
>   (2) "失败后再调 start() 仍只 execute 1 次"。仅靠并发 `async let` 双调测试不能验证跨边界短路。
> - **反例 1**：只用 `pingTask != nil` 防重入 —— 跨 view 重启边界不生效，本 review 的 round 1 fix
>   就踩了这个坑。
> - **反例 2**：把 ping 移到 `App.init` 或 `@main` 入口绕过 view 生命周期 —— 破坏 DI 分层（容器未就绪），
>   且把 SwiftUI 生命周期管理拆得更碎。在 ViewModel 加状态比拆架构便宜。
> - **反例 3**：用 `.onAppear` 替代 `.task` —— `.onAppear` 也会在 view 重新出现时再次触发，问题完全
>   一样；而且 `.onAppear` 不能 await async 函数，需要包 `Task { ... }`，反而更绕。
> - **反例 4**：用全局 `static var hasFetched` 跨 viewModel 实例共享 —— global mutable state 是
>   测试灾难（test isolation 全坏），且不同 user / 不同 launch session 该 reset 时不 reset。

### 顺带改动

- `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`：`start()` 增加 `hasFetched` 短路；
  方法 doc / 文件 header 注释同步更新说明语义。
- `iphone/PetAppTests/Features/Home/HomeViewModelPingTests.swift`：新增 2 个 case
  （`testStartShortCircuitsAfterFirstCompletion` + `testStartShortCircuitsAfterFailure`）覆盖跨 task
  边界短路。
- `docs/lessons/2026-04-26-baseurl-from-info-plist.md`："未完成事项 / 后续 TODO" 段追加 round 2
  finding 2 的 defer 登记（不在本 story 修，留给 Epic 3 demo 验收）。
