---
date: 2026-05-04
source_review: codex review round 2 of Story 8-4 (file: /tmp/epic-loop-review-8-4-r2.md)
story: 8-4-主界面猫-sprite-三态动画切换
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-04 — SwiftUI body 内 switch 分支 swap 必须用 `.id() + .transition() + .animation(value:)` 三件套才能 fade

## 背景

Story 8-4 PetSpriteView 用 `switch state { case .rest / .walk / .run }` 三分支渲染不同 SF Symbol，
spec（epics.md AC 行 1539）钦定 "state 切换时有平滑过渡（淡入淡出 200ms）"。round 1 实装写
`ZStack { switch state { ... } }.animation(.easeInOut(duration: 0.2), value: state)`，
codex round 2 抓到：实际运行时 SF Symbol 瞬时切换，**没有任何 fade**。AC2 视觉契约失效。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | SwiftUI body 内 switch 分支 swap 缺 fade | P2 | architecture | fix | `iphone/PetApp/Features/Home/Views/PetSpriteView.swift` |

## Lesson 1: `.animation(value:)` 单独不会让 view body switch 分支切换产生 fade —— 必须配 `.id()` + `.transition()`

- **Severity**: P2
- **Category**: architecture（SwiftUI 渲染 / 动画 idiom）
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/Views/PetSpriteView.swift:30-55`

### 症状（Symptom）

`PetSpriteView` body 写成：

```swift
ZStack {
    switch state {
    case .rest: Image(systemName: "cat.fill")...
    case .walk: Image(systemName: "figure.walk")...
    case .run:  Image(systemName: "figure.run")...
    }
}
.animation(.easeInOut(duration: 0.2), value: state)
```

期望：state 改变时三个 Image 之间走 200ms fade。
实际：SF Symbol 瞬时切换，无任何过渡。AC2 钦定的"平滑过渡（淡入淡出 200ms）"未实现。

### 根因（Root cause）

SwiftUI 三个 modifier 各管一件事，混淆它们的分工是常见误区：

| modifier | 真实语义 |
|---|---|
| `.animation(_:value:)` | 当 `value` 改变时，对**当前 view tree 的 modifier 状态变化**进行 implicit 动画化（如 frame / opacity / color）。**不**让 body 内不同 view 的 swap 自动 fade |
| `.transition(_:)` | 声明 view 在被**插入 / 移除**时使用什么过渡效果（默认是 `.identity` 即瞬时） |
| `.id(_:)` | 给 view 一个 identity；当 id 改变时，SwiftUI 把它当作**老 view 移除 + 新 view 插入**而不是 mutate modifier |

只写 `.animation(value:)` 时，SwiftUI 看到的是"同一个 ZStack，body closure 重新求值，里面 SF Symbol 名字变了"——
SwiftUI 不知道这算 view replacement，会尽量复用同一个 UIImageView 节点，只是把 image 资源换掉，
所以没有 view insert / remove，`.transition()` 没有触发点，`.animation()` 也没有 modifier-level 数值变化可动画。

要让"分支 swap 走 fade transition" 生效，必须三件套齐全：

1. `.id(state)` 让 SwiftUI 把 state 切换识别成 view replacement（强制 destroy 旧 view + 新建新 view）；
2. `.transition(.opacity)` 声明替换时走 opacity 过渡（替代默认瞬时切换）；
3. `.animation(.easeInOut(duration: 0.2), value: state)` 提供 transition 的 timing curve 与 duration。

### 修复（Fix）

把 body 重构成"单一 view + helper func 提取 state-dependent 字段 + 三件套修饰"：

```swift
public var body: some View {
    spriteImage(
        symbol: spriteSymbolName(for: state),
        tintColor: spriteTintColor(for: state)
    )
    .id(state)                                              // ← 关键：让 state 变化触发 view replacement
    .transition(.opacity)                                   // ← 关键：replacement 时走 opacity
    .animation(.easeInOut(duration: 0.2), value: state)     // ← timing
    .accessibilityElement(children: .ignore)
    .accessibilityLabel(Text(accessibilityLabel))
    .accessibilityIdentifier(currentIdentifier)
}

private func spriteSymbolName(for state: MotionState) -> String { ... }
private func spriteTintColor(for state: MotionState) -> Color  { ... }
```

`spriteImage(symbol:tintColor:)` helper 不变（仍是 SF Symbol + frame + tint）；switch 从 body
搬到两个 helper func 内。a11y label / identifier 已经走外层 modifier 不受影响。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 SwiftUI **要让 body 内 switch / if-else 分支切换出现 transition 效果**时，**必须**同时挂 `.id(<切换变量>)` + `.transition(.opacity)` + `.animation(_:value:<切换变量>)` 三件套；**禁止**只写 `.animation(value:)` 就以为有 fade。
>
> **展开**：
> - `.animation(_:value:)` 只动画化"已存在 modifier"的数值变化（frame / opacity / color），**不**动画化 view body 内不同分支的 swap。
> - `.transition()` 只在 view 被实际插入 / 移除时生效，需要 `.id()` 强制 view replacement 才能触发。
> - 三件套缺一就静默失败：缺 `.id()` → SwiftUI 复用同一节点只换内容，无 transition；缺 `.transition()` → 默认 `.identity` 瞬时切换；缺 `.animation()` → 走默认 transition 时长（也可能是 0）。
> - 等价模式：`switch state` 三分支 → 提取 `stateSymbolName(for:)` / `stateColor(for:)` helper func → body 写**单一** view + 上述三件套。这是 SwiftUI 文档推荐的 idiom，比 `ZStack { switch }` 更可读且 fade 实际生效。
> - **反例 1**：`ZStack { switch state { case ... } }.animation(.easeInOut, value: state)`——视觉瞬时，无 fade（本次踩坑）。
> - **反例 2**：用 `withAnimation { state = newValue }` 包裹 mutation——只能动画化 modifier 数值变化，仍然救不了 view swap。
> - **反例 3**：单独 `.transition(.opacity)` 而不挂 `.id()`——没有 view replacement 就没有 transition 触发。
> - **验证方法**：模拟器实跑（`bash iphone/scripts/build.sh && install_app && launch_app`）触发 state 变化，**眼睛**看 fade 是否真发生；不能只跑单测——单测验证 ViewModel state 流转，验证不到 SwiftUI render-tree fade 行为。

### 顺带改动

无 —— 仅 PetSpriteView 内部重构 + 修 fade idiom，未触碰 ViewModel / a11y / 其他 view。
