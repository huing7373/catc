---
title: 引入 abstract method base class 时必须同步迁移所有 caller，不能留 fatalError 在 production 注入路径
date: 2026-04-30
severity: 1
category: architecture, swift, refactor-discipline
commit: 5f439a4
related_stories: [37-7-homeview-scaffold]
---

## 现象

Story 37-7（HomeView Scaffold）把 `HomeViewModel` 重构成 abstract base class —— 5 个核心交互方法（`feed()` / `pet()` / `play()` / `tapPet()` / `markTeamCardSeen()` 等）改成了
`func xxx() { fatalError("subclass must override") }`，并新增 `MockHomeViewModel`（preview / unit test 用）和 `RealHomeViewModel`（production 用）两个 subclass。

但 `RootView.swift` 的 `@StateObject` 注入点并未同步替换 —— 仍然是裸 `HomeViewModel()`：

```swift
@StateObject private var homeViewModel: HomeViewModel = HomeViewModel()  // ← 留在 base class
```

后果：

- 启动正常（base class 可实例化，`init()` 不抛错）
- HomeView 第一次渲染正常（render 路径不调那 5 个 abstract method）
- 用户**点 actionRow 三按钮（feed / pet / play）或 teamIdleCard 中的"开始组队"按钮的瞬间** → `fatalError("subclass must override")` → app crash 在 production wire 路径

unit test 全部通过，因为 `MockHomeViewModel` override 了所有 abstract method —— mock 路径覆盖不到 RootView 的注入决策本身。

## 根因

把 concrete class 重构成 abstract base class 是一次**类型契约变更**，但 dev 实装时只完成了:
1. 类型定义改写（`HomeViewModel` 加 abstract method）
2. 新建 subclass（`MockHomeViewModel` / `RealHomeViewModel`）
3. unit test 改用 mock subclass

而**漏掉了 production 注入点的 grep 兜底**（`RootView` / `AppContainer` / Preview 等所有 `HomeViewModel()` 字面量调用）。

抽象表达：把一个 concrete type T 改成 abstract base 时，原本所有 `T()` / `T(...)` 调用点都退化成了**编译能过但运行时 fatalError** 的隐患——因为 base class 在 Swift 里不强制 abstract，编译器不会替你扫这些点。

## 教训

**把 concrete class 重构成 abstract base class 时，必须同步改所有 production 注入点用 concrete subclass。**

具体动作：grep 全仓搜 `HomeViewModel()` / `HomeViewModel.init` / `HomeViewModel(` 全部 caller 一遍：
- production wire 路径（RootView / AppContainer / HomeContainerView 等）→ 改用 `RealHomeViewModel`
- Preview 路径 → 改用 `MockHomeViewModel`
- 单测 / UI test setup → 改用 `MockHomeViewModel` 或同模式的 mock subclass

任何继续用 base class 的地方，都是潜在 production crash 点。

## 预防规则（forward-actionable）

1. **SM 写 spec 钦定 ViewModel 改 abstract 时，AC 必须显式列出所有需要改的注入点清单**：不仅 RootView，还要包含 AppContainer / HomeContainerView / HomeView Preview / 单测 setup / UITest setup 等。dev 按 grep 兜底全部改完，不能漏。
2. **abstract method 的 `fatalError("subclass must override")` 应当配套加 unit test 守护**："production 注入的不是 base class"。例如 RootViewWireTests 加：
   ```swift
   func test_homeViewModel_isRealHomeViewModel_notBaseClass() {
       let rootView = RootView(...)
       XCTAssertTrue(rootView.homeViewModel is RealHomeViewModel)
       XCTAssertFalse(type(of: rootView.homeViewModel) == HomeViewModel.self)
   }
   ```
   这条 test 把 type 契约变成机器可校验的红线，类型系统兜底失效时用 runtime assertion 兜底。
3. **SwiftUI `@StateObject private var x = SomeClass()` 属性初始化器内不能交叉引用同级 `@StateObject`**（self 未求值，会编译错或运行时拿到未初始化的对象）。如果 ViewModel 需要其他 ObservableObject 注入，要么：
   - (a) 用 parameterless `init()` + `.task` 内 `bind(other:)` 模式；ViewModel 提供一个空构造，然后在父 view 的 `.task` / `.onAppear` 内拿到 EnvironmentObject 调 `bind(appState:)` 注入依赖。
   - (b) 把 ViewModel 改成 lazy 字段 / 在父 view onAppear 内创建 —— 不要塞进 `@StateObject` 的初始化器同步路径。

   本次 fix 选 (a)：`RealHomeViewModel` 加 parameterless `public init() { super.init(); configureMockDefaults() }` + 把 mock 默认值抽到 `configureMockDefaults()` 让 `init()` 和 `init(appState:)` 都用，`@StateObject` 处只用裸 `RealHomeViewModel()` 构造。

## 元规则

把"类型变更导致全仓 caller 必须迁移"这件事，**写到 SM 的 AC 模板里固化**——避免依赖 dev 个体的 grep 自觉性。AC 一行：

> 当 abstract / 类型契约变更时，AC 必须列出全仓所有注入点清单 + dev 必须 grep 后逐点 verify。
