---
date: 2026-04-26
source_review: codex review round 1 — Story 2.6 基础错误 UI 框架
story: 2-6-基础错误-ui-框架
commit: 634c564
lesson_count: 1
---

# Review Lessons — 2026-04-26 — SwiftUI fullScreenCover 是隔离 window scene，全局 overlay UI 必须在 sheet 子树重复 attach

## 背景

Story 2.6 设计 `errorPresentationHost(presenter:)` ViewModifier：把 `ErrorPresenter` 挂到根视图，根据 `presenter.current` 渲染对应的 toast / alert / retry 组件，**全局可见**。

第一版按 modifier 链顺序在 RootView body 末尾挂：

```swift
HomeView(viewModel: homeViewModel)
    .onAppear { ... }
    .task { ... }
    .fullScreenCover(item: $coordinator.presentedSheet) { sheet in
        sheetContent(for: sheet)        // ← sheet 子树没有 errorPresentationHost
    }
    .errorPresentationHost(presenter: container.errorPresenter)
```

Codex round 1 指出：当 sheet 打开后，任何业务流程触发的错误（ErrorPresenter.current 改变）会被 `.errorPresentationHost` ZStack 渲染在主 view 层 —— 但 sheet 是独立 window scene，**整片盖在主 view 之上**，错误 UI 永远显示在 sheet 之下，用户看不见。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | errorPresentationHost 被 fullScreenCover 盖住 | high (P1) | architecture | fix | `iphone/PetApp/App/RootView.swift:38-41, 60-69` |

## Lesson 1: fullScreenCover 内容子树不继承 modifier 链上的 overlay host

- **Severity**: high
- **Category**: architecture / ui
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift:60-78`

### 症状（Symptom）

ErrorPresenter 是全局错误 UI 的 source of truth。期望：sheet 打开时若发起的请求出错，全局 retry / alert / toast 仍可见、可交互。

实际：`.errorPresentationHost` 挂在主 view 末尾，sheet 由 `.fullScreenCover` 弹出 → SwiftUI 在新 window scene 渲染 sheet content，sheet content **不**继承外层 modifier 链。结果：
- sheet 关闭时：错误 UI 正常（主 view 顶层渲染）
- sheet 打开时：错误 UI 被 sheet 整片盖住，用户看不到

未来 sheet-based flow（如 `RoomView` 内发起业务请求）触发的错误会**在用户视线之外消失**。

### 根因（Root cause）

误认为 SwiftUI 的 `.fullScreenCover` / `.sheet` 是普通的 child view —— 实际上它是**独立的 view hierarchy / window scene**：

- modifier 链上挂在 `.fullScreenCover` **之后** 的 modifier（如 `.errorPresentationHost`）作用于"被 cover 的内容"，**不**穿透到 cover content
- modifier 链上挂在 `.fullScreenCover` **之前** 的 modifier 也不会自动传播到 cover content 子树
- environment 注入（`.environment(...)` / `.environmentObject(...)`）在 SwiftUI 17+ 通常会**继承**到 sheet content（这是 SwiftUI 显式优化），但**自定义 ViewModifier 渲染的覆盖层（ZStack 内含 view 等内联 UI）不会继承**：modifier 渲染的视图位于宿主视图层级，sheet 把宿主整层盖住

简言之：environment 数据可以跨 sheet 传播，但 ZStack 渲染的 overlay UI 视觉上被 sheet 盖住、无解。

### 修复（Fix）

让 sheet 子树**自己也 attach 一份 `.errorPresentationHost(presenter:)`**，共享同一个 `ErrorPresenter` 实例：

```swift
@ViewBuilder
private func sheetContent(for sheet: SheetType) -> some View {
    Group {
        switch sheet {
        case .room: RoomPlaceholderView(onClose: { coordinator.dismiss() })
        case .inventory: InventoryPlaceholderView(onClose: { coordinator.dismiss() })
        case .compose: ComposePlaceholderView(onClose: { coordinator.dismiss() })
        }
    }
    .errorPresentationHost(presenter: container.errorPresenter)
}
```

工作机制：
- `container.errorPresenter` 是 stable singleton（per AppContainer 一次）
- `ErrorPresenter` 是 `ObservableObject`，`current` 是 `@Published` source of truth
- 主 host（在主 view 末尾）+ sheet host（在 sheet 子树）都通过 `@ObservedObject` 监听同一份 state
- `presenter.current` 改变时两个 host 都重渲染 —— 视觉上 sheet 子树的 host 渲染在 sheet 顶层，主 host 渲染在底层（被 sheet 盖住，不可见但不冲突）
- 用户看到的是"sheet 子树 host 渲染的那份"

测试：
- `testErrorPresenterIsStableSingletonWithinContainer`（`AppContainerTests`）—— 断言 container.errorPresenter 多次访问返回同一 instance（两个 host 必须共享 source of truth）
- 视觉层叠由 SwiftUI 保证（不在单测覆盖范围）；UITest 兜底是后续工作

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 SwiftUI 项目中设计**全局 overlay UI**（错误层 / loading 层 / toast 层）时，**必须** 把 overlay host 在**每一个会进入独立 window scene 的入口子树**也 attach 一遍，不能只挂在 root view 一次。
>
> **展开**：
> - "独立 window scene"在 SwiftUI 里常见的入口：`.fullScreenCover(...)`、`.sheet(...)`、`.popover(...)`、`WindowGroup` / `Window`、UIKit `UIPresentationController` 桥接的弹层
> - overlay host **挂一次就够**这个直觉适用于 environment / DI（state 跨 scene 自动传播），**不**适用于"基于 ZStack / overlay modifier 渲染出来的 UI 元素"——后者绑定在宿主 view 层级，sheet 整片盖在宿主之上时，overlay 永远在 sheet 之下
> - 共享 source of truth 的实现关键：**全局唯一一份 ObservableObject**（如 ErrorPresenter）+ 每个 scene 入口都挂自己的 host modifier，host 通过 `@ObservedObject` 订阅同一份 state。两个 host 看到的状态完全一致 → 渲染一致 → 用户感觉是"全局 UI"
> - 验证 source of truth 共享性的最小测试：断言 container.<presenter> 是 stable singleton（同 instance），不是每次访问都构造新对象
> - **反例 1**：把 `.errorPresentationHost` 只挂在 RootView body 末尾、sheet content 不挂 —— sheet 打开时错误 UI 隐形
> - **反例 2**：sheet content 自己 new 一个 ErrorPresenter（"反正 host 需要 presenter，就地构造"）—— state 不共享，主 view 触发的错误 sheet 看不见，sheet 触发的错误主 view 看不见
> - **反例 3**：把 ErrorPresenter 注入 environment（`.environmentObject(presenter)`），以为 sheet 自动继承就够了 —— environment 数据虽可跨 sheet，但没有 host modifier 的 sheet 子树根本不会渲染 overlay UI
> - **替代方案考量**：iOS 13+ 的 UIWindow API（手动起一个 overlay UIWindow，windowLevel 高于 normal）确实能实现"真·全局始终在最上层"，但成本远高于"sheet 子树 attach host"；除非业务真的需要 sheet 弹时仍能在 sheet 之上盖一层（极少），优先选 host attach 路线

## Meta: 本次 review 的宏观教训

SwiftUI 的"组合即配置"哲学让 modifier 链看起来像简单管线，但 sheet / fullScreenCover / window 这些**控制流跳转 modifier** 在底层是 scene 切换，不是 view 嵌套。每次设计跨 scene 的 UI 元素（错误层 / 全局 toast / 浮动操作按钮 / debug overlay 等）都要先问：**它在 sheet 打开时还需要可见吗？** 如果是，就要在每个 scene 入口子树重复 attach。
