---
date: 2026-04-30
source_review: codex review (Story 37.6 round 2) — /tmp/epic-loop-review-37-6-r2.md
story: 37-6-shared-primitives
commit: d7abcbc
lesson_count: 1
---

# Review Lessons — 2026-04-30 — SwiftUI `.frame(maxWidth: .infinity)` 与 `.padding` 的顺序敏感性（fullWidth 按钮溢出父容器）

## 背景

Story 37.6 把 ui_design `primitives.jsx` 的 `PrimaryButton` 从 React/CSS 端口到 SwiftUI 时，
`fullWidth = true` 分支用了"先 `.frame(maxWidth: .infinity)` 再 `.padding(.horizontal, 22)`"的
modifier 顺序。这在 modal card / list row 等约束父容器里会让按钮宽度比父容器多 44pt（22 × 2），
被裁剪或触发 layout warning。Round 2 codex review 命中。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `.frame(maxWidth: .infinity)` 必须在 `.padding(.horizontal:)` **之后**（用 CSS box-sizing 心智模型对齐 jsx 源） | medium (P2) | ui-fidelity | fix | `iphone/PetApp/Core/DesignSystem/Primitives/PrimaryButton.swift:55-57` |

## Lesson 1: SwiftUI `.frame(maxWidth: .infinity)` 与 `.padding` 顺序对齐 CSS box-sizing

- **Severity**: medium (P2)
- **Category**: ui-fidelity
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/DesignSystem/Primitives/PrimaryButton.swift:55-57`

### 症状（Symptom）

`PrimaryButton(fullWidth: true)` 放进任何宽度受约束的父容器（modal sheet / list row /
受 `.padding` 控制的卡片）后，按钮整体比父容器宽 44pt（22pt × 2），导致：

- 按 ui_design `primary-button.jsx:152, 158`（`padding: '0 22px'` + `width: 100%`），
  CSS 默认 `box-sizing: border-box` 时 padding 是计入 width 内部的；
- 移植到 SwiftUI 时若先 `.frame(maxWidth: .infinity)` 再 `.padding(.horizontal, 22)`，
  SwiftUI 的 modifier 是"层层叠"语义 — 后加的 padding 是把已经达到父宽的 frame **再**外扩
  44pt，导致按钮溢出。

### 根因（Root cause）

**SwiftUI modifier 顺序对几何 layout 是非交换的（non-commutative）**。`.frame` 和 `.padding`
两个 modifier 的相对顺序决定语义：

- `.padding(h, 22).frame(maxWidth: .infinity)` ≡ CSS `padding: 0 22px; box-sizing: border-box; width: 100%`（padding 在内部）
- `.frame(maxWidth: .infinity).padding(h, 22)` ≡ CSS `width: 100%; margin: 0 22px`（padding 在外部，等价于 margin）

把 jsx 端口到 SwiftUI 时，惯性思维容易"按 jsx 行序逐行翻译"（jsx 里 `padding` 写在 `width`
之前还是之后并不影响 box-sizing 语义），但 SwiftUI 不是 CSS，view tree 的 modifier 是从内
到外应用、外层 modifier 看到内层已计算好的尺寸。**心智模型必须切换：先想"内容外面要包多少
inner padding"再想"整体撑到多宽"**，按这个顺序写 modifier。

### 修复（Fix）

`PrimaryButton.swift:55-57` 把 `.padding(.horizontal, theme.spacing.s22)` 提到
`.frame(maxWidth: fullWidth ? .infinity : nil)` **之前**，并在源码加注释固化语义、引用
ui_design jsx 行号防回归：

```swift
.frame(height: 52)
// ⚠️ Modifier order matters: padding 必须在 .frame(maxWidth:) **之前**.
// 对齐 ui_design primitives.jsx:151-158 — CSS 里 `width: 100%` 默认 box-sizing
// 等价于 padding 计入 width 内；SwiftUI 里要还原此语义就是先 padding 再扩展 maxWidth.
.padding(.horizontal, theme.spacing.s22)
.frame(maxWidth: fullWidth ? .infinity : nil)
```

未单独加 unit test：ADR-0002 §3.1 禁用 ViewInspector / SnapshotTesting，几何属性无法用
sourcekit-only 工具验证；改为源码注释 + jsx 锚点的方式守护（人类 review / Claude 后续读到
此段直接看到反例描述）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **把 web/CSS 组件端口到 SwiftUI**、且原 CSS 里同时使用
> `padding` + `width: 100%`（或 `100vw` / `auto` 撑满）时，**SwiftUI 端必须按"先 `.padding`
> 再 `.frame(maxWidth: .infinity)`"的顺序写**，不要按 jsx style 对象的字段写入顺序逐行翻译。
>
> **展开**：
> - SwiftUI modifier 是从内到外层层应用，每一层只看到内层已确定的尺寸；这是和 CSS 最大的
>   心智差异。CSS 的 `box-sizing: border-box`（默认值）让 `width: 100%` "吸收" padding；
>   SwiftUI 没有 box-sizing 概念，必须靠 modifier 顺序还原。
> - 测试很难直接断言（除非引入 ViewInspector / 拍 snapshot，但 ADR-0002 §3.1 已禁用）。
>   **务必在源码留 inline 注释固化决策**，引用对应 jsx / Figma / spec 锚点行号，避免后人
>   "顺手优化"时把 modifier 顺序换回去。
> - **反例 1**：`.frame(height: 52).frame(maxWidth: .infinity).padding(.horizontal, 22)`
>   — fullWidth 时按钮比父容器多 44pt，溢出/裁剪。
> - **反例 2**：把 `.padding(.horizontal, 22)` 写在 `.background(RoundedRectangle...)` 之后
>   — 这时 padding 不再是按钮内 padding 而是按钮外 margin，背景 pill 没 22pt 内边距，
>   文字贴边。
> - **检查清单**：移植 web 组件时 grep 原 CSS 的 `width: 100%` / `flex: 1` / `align-self:
>   stretch` 等"撑满"声明，每命中一处都要在 SwiftUI 端确认 `.frame(maxWidth: .infinity)`
>   是不是放在 `.padding` 之后（即"先 padding 再 frame"）。

---
