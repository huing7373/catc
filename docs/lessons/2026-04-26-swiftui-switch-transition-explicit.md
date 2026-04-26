---
date: 2026-04-26
source_review: codex round 1 review on Story 2.9 LaunchingView 设计
story: 2-9-launchingview-设计
commit: c94209b
lesson_count: 1
---

# Review Lessons — 2026-04-26 — SwiftUI `.animation(_:value:)` 不会让 switch 分支切换淡入淡出，必须 ZStack 容器 + 每分支显式 `.transition`

## 背景

Story 2.9 AC 钦定"LaunchingView ↔ HomeView ↔ RetryView 三态切换 200ms 淡入淡出"。实装写成：

```swift
Group {
    switch launchStateMachine.state {
    case .launching: LaunchingView()
    case .ready: homeView
    case .needsAuth(let m): RetryView(message: m, ...)
    }
}
.animation(.easeInOut(duration: 0.2), value: launchStateMachine.state)
```

codex round 1 [P3] 指出：实际不会有过渡，state 切换是硬切（abrupt cut）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `.animation(_:value:)` 不动画 switch 分支切换 | P3 (low) | ui/animation | fix | `iphone/PetApp/App/RootView.swift:62-88` |

## Lesson 1: `.animation(_:value:)` 只动画**现有 view 的属性变化**，分支切换是**view 插入/删除**，需要 transition + 支持 transition 的容器

- **Severity**: low (P3)
- **Category**: ui / animation
- **分诊**: fix（修法极简，且达成 AC 设计目标）
- **位置**: `iphone/PetApp/App/RootView.swift:62-88`

### 症状（Symptom）

`.animation(.easeInOut, value: state)` 挂在 Group 上，Group 内 switch 分支换成另一种 view 时不会淡入淡出。视觉上是 0ms 硬切，与 epics.md AC 钦定的 200ms 不符。

### 根因（Root cause）

SwiftUI 文档明确：`.animation(_:value:)` 在 `value` 改变时让**当前 view 树的属性差异**做隐式动画（如 frame / opacity / color 等）。switch 不同分支是**结构性变更**（view A 删除 + view B 插入），属性动画不覆盖这条路径。

要让结构性变更产生过渡，必须满足两条：
1. **容器支持 transition**：`Group` 是非渲染的"逻辑容器"，不参与 view tree 的过渡协调；需要 ZStack / overlay / .background / `if/else` 等条件容器，让新旧 view 在切换时刻**同时存在**做交叉淡入淡出
2. **每个分支 view 显式 `.transition(.opacity)`**：告诉 SwiftUI 该 view 在插入 / 删除时用 opacity transition

### 修复（Fix）

把 Group 换 ZStack + 每分支加 `.transition(.opacity)`：

```swift
ZStack {
    switch launchStateMachine.state {
    case .launching:
        LaunchingView()
            .transition(.opacity)
    case .ready:
        homeView
            .onAppear { ... }
            .fullScreenCover(...) { ... }
            .transition(.opacity)
    case .needsAuth(let message):
        RetryView(message: message, onRetry: { ... })
            .transition(.opacity)
    }
}
.animation(.easeInOut(duration: 0.2), value: launchStateMachine.state)
```

ZStack 让新 view 已 fade-in 时旧 view 仍在 fade-out，达成交叉淡入淡出。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **SwiftUI 多状态 view 路由（state-driven 分支切换）** 场景下，要"分支切换有过渡"必须 **三件套同时具备**：(1) 容器是 ZStack（或其他 transition-aware 容器），(2) 每分支 view 加 `.transition(...)`，(3) 外层 `.animation(_:value:)` 绑 state；**禁止** 只用 `.animation(_:value:)` 套 Group 期待自动过渡。
>
> **展开**：
> - `.animation(_:value:)` 是**属性动画** modifier，不处理 view 插入 / 删除
> - `.transition(...)` 是**结构动画** modifier，告知 SwiftUI 视图进出时如何过渡
> - Group 不支持 transition；ZStack / overlay / .background / 条件 if-else 容器才支持
> - **反例**：`Group { switch state { case .a: A(); case .b: B() } }.animation(.easeInOut, value: state)` —— 表面看像声明了动画，实际硬切；UX 设计的"淡入淡出"完全不生效
> - 单 case 切换可不用 ZStack：`if condition { A().transition(.opacity) } else { B().transition(.opacity) }` 同样 work（条件容器支持 transition）
> - 写完 transition 代码后**真机或 simulator 跑一下**确认视觉过渡 —— `.animation` 不会编译报错也不会运行 crash，光看代码无法确定是否生效
