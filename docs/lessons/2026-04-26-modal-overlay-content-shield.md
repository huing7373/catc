---
date: 2026-04-26
source_review: codex round 2 review of Story 2.6 (file: /tmp/epic-loop-review-2-6-r2.md)
story: 2-6-基础错误-ui-框架
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — SwiftUI modal overlay 必须做下层 hit-testing + accessibility 双屏蔽

## 背景

Story 2.6 引入 `ErrorPresentationHostModifier`，用 ZStack + zIndex 把 toast / alert / retry 三态错误 UI 叠在主内容上。round 2 codex 指出 alert 路径只画了 `AlertOverlayView`、没屏蔽下层 content——VoiceOver 用户仍能 focus 后面的按钮、tap 也可能穿透；alert 设计语义里"必须通过『知道了』退场"的 modal blocking 在访问性场景下根本没生效。retry 路径已经通过 `accessibilityHidden(true)` + `opacity(0)` 做到了屏蔽，alert 路径漏了。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | alert overlay 下层 content 未屏蔽访问性 + hit-testing | P2 (medium) | accessibility | fix | `iphone/PetApp/Core/DesignSystem/Components/ErrorPresentationHostModifier.swift:31-45` |

## Lesson 1: SwiftUI modal overlay 必须做下层 hit-testing + accessibility 双屏蔽

- **Severity**: medium (P2)
- **Category**: accessibility / ui
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/DesignSystem/Components/ErrorPresentationHostModifier.swift:31-45`

### 症状（Symptom）

`ErrorPresentationHostModifier` 在 `presenter.current = .alert` 时，ZStack 里只在主 content 之上画一层 `AlertOverlayView`。但 SwiftUI 的 ZStack overlay **不会自动屏蔽下层**：

- VoiceOver 焦点能穿过 overlay focus 到下层按钮
- 触摸事件能否穿透取决于 overlay 自身是否吃事件，但 alert 卡片只占屏幕中间一小块——卡片外的遮罩区如果没有显式 `.contentShape(Rectangle()).onTapGesture { }`，下层照样响应
- "必须通过『知道了』按钮退场"的 modal blocking 语义，在访问性 + 边角触摸场景下完全失守

retry 分支当时已经做了 `.accessibilityHidden(retryActive)` + `.opacity(0)` 双屏蔽，但 alert 分支漏了——典型的"两个 modal case 落地不齐"。

### 根因（Root cause）

把 SwiftUI 的 ZStack 视觉层级误当作交互/访问性层级。SwiftUI 默认假设：

- **同 ZStack 内多个子视图都是可见 + 可交互的**
- overlay 类组件（`.overlay {}` / 后插入 ZStack）默认**不**屏蔽下层 hit-testing
- VoiceOver 默认会朗读 ZStack 里所有未显式 hide 的元素

要把一个 overlay 变成真正的 modal blocking 层，必须**主动**对下层 content 应用：

1. `.allowsHitTesting(false)` — 屏蔽点击 / 触摸 / 长按 / drag gesture
2. `.accessibilityHidden(true)` — 让 VoiceOver 跳过下层 element

只用 zIndex / 视觉遮罩远远不够——这两层是平行属性，必须各自显式声明。

更深层：modal 语义是**应用级合约**，不是渲染层细节。`ErrorPresentation` 三态里 alert / retry 都是 modal（必须用户主动操作才退场），toast 不是；这个分类应该映射到一个 `modalActive: Bool` 计算属性，下层屏蔽逻辑用 `modalActive` 而不是各 case 各自写一遍。

### 修复（Fix）

引入 `modalActive` 计算属性（true 当 current 是 alert 或 retry），把双屏蔽语义集中表达：

```swift
content
    .accessibilityHidden(modalActive)
    .allowsHitTesting(!modalActive)
    .opacity(retryActive ? 0 : 1)   // retry 额外视觉全覆盖；alert 视觉保留下层（半透明遮罩在 AlertOverlayView 内）
```

`retryActive` 单独保留——retry 还需要把下层 opacity 置 0，alert 不需要（alert 设计上保留下层视觉做空间感）。

测试侧：在 `ErrorComponentSnapshotTests` 里加 4 个 case 直接断言 `modalActive` / `retryActive` 计算结果（绕开 SwiftUI host 环境）：
- nil 状态 → 都 false
- toast → 都 false（toast 不是 modal）
- alert → modalActive=true, retryActive=false
- retry → modalActive=true, retryActive=true（用 `APIError.network` 触发，因为 retry 没有公开 enqueue 入口）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **用 SwiftUI ZStack/overlay 实现 modal blocking 类 UI（alert / sheet 替代品 / 全屏 retry / loading mask）** 时，**必须**对下层 content 同时应用 `.allowsHitTesting(false)` + `.accessibilityHidden(true)`，**禁止**只靠 zIndex 或视觉遮罩当 modal。
>
> **展开**：
> - **modal 语义是应用级合约**：能否被 dismiss / 是否阻塞下层操作 是产品规则，不是渲染细节。把这个语义抽成一个 `modalActive: Bool` 计算属性，下层屏蔽 + dim + opacity 都引用它，避免多 case 分别写忘记一处
> - **三个屏蔽维度独立**：
>   - 视觉（opacity / blur / dim）→ 控制下层是否可见
>   - 交互（`allowsHitTesting`）→ 控制下层能否响应 tap/drag
>   - 访问性（`accessibilityHidden`）→ 控制 VoiceOver 能否 focus
>   三者**默认都不联动**，必须各自显式声明。视觉处理（opacity=0.6 dim）**不**自动屏蔽访问性
> - **同一态机的多个 modal case 必须落地一致**：alert / retry 都是 modal → 屏蔽逻辑必须共用同一个判定（如 `modalActive`），不要 case-by-case 各写各的——否则后加 case 必漏
> - **测试可以绕过 host 环境**：SwiftUI ViewModifier 在没有 host 时调 `.body` 会触发 layout assertion，但**计算属性 / 提取的 helper 函数可以直接断言**。把 modal 判定抽成 `var modalActive: Bool { ... }` 顺带带来可测性收益
> - **反例 1**：写 `if case .alert = current { AlertOverlayView() }` 然后忘了屏蔽下层——只在视觉上叠了一层，访问性下完全失守
> - **反例 2**：依赖"遮罩自己 `.contentShape(Rectangle()).onTapGesture { swallow }`"屏蔽下层点击——这只挡 tap，挡不住 VoiceOver focus、也挡不住 drag/long press 等其它 gesture
> - **反例 3**：用 `.disabled(true)` 替代 `.allowsHitTesting(false)`——`.disabled` 会同时把控件变灰（视觉副作用）+ 不一定屏蔽 gesture recognizer；语义不等价

---
