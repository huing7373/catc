---
date: 2026-04-30
source_review: /tmp/epic-loop-review-37-6-r3.md (codex P2×2 — Story 37.6 round 3)
story: 37-6-shared-primitives
commit: abc8ab3
lesson_count: 2
---

# Review Lessons — 2026-04-30 — SwiftUI strokeBorder vs stroke 内外绘语义 & ButtonStyle vs 自定义 DragGesture 取消语义

## 背景

Story 37.6（共享 primitives）round 3 review，codex 报 2 条 P2 视觉/交互 bug：
- Avatar `ring=true` 用了 `strokeBorder` 导致 face area 被侵占 5pt
- PrimaryButton 自定义 `simultaneousGesture(DragGesture)` 在用户按住后**拖出按钮范围**时 `onEnded` 不触发 → `isPressed` 卡 true → 按钮 stuck 在 pressed 视觉

两条都改 — option A（`.stroke()` 替换 `.strokeBorder()`；`ButtonStyle` 替换自定义 DragGesture）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Avatar ring 用 strokeBorder 内绘侵占 face area | medium | style | fix | `iphone/PetApp/Core/DesignSystem/Primitives/Avatar.swift:54-65` |
| 2 | PrimaryButton 自定义 DragGesture 不处理手势取消 | medium | architecture | fix | `iphone/PetApp/Core/DesignSystem/Primitives/PrimaryButton.swift:75-81` |

## Lesson 1: SwiftUI strokeBorder（内绘）vs stroke（中心线）vs box-shadow（外绘）的对齐选择

- **Severity**: medium
- **Category**: style
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/DesignSystem/Primitives/Avatar.swift:54-65`

### 症状（Symptom）

`Avatar(ring: true)` 渲染出来的 colored face area 比 `size` 参数小约 5pt（双圈描边总 5pt 全部画在 circle 内侧），与 ui_design `primitives.jsx` 用的 `box-shadow`（halo 在 avatar 外部）视觉不一致。

### 根因（Root cause）

SwiftUI 三种"画圈"方式的绘制位置语义不同，但极易混淆：

| 方法 | 绘制位置 | 类比 web |
|---|---|---|
| `.strokeBorder(_, lineWidth: w)` | 在 shape 边界 **内侧** 画 w 宽 | 类似 `border` + `box-sizing: border-box`（border 计入 width） |
| `.stroke(_, lineWidth: w)` | 在 shape 边界 **中心线**（一半在内一半在外） | 类似 SVG `stroke` 默认行为 |
| `.shadow(color:, radius: 0)` | 完全在 shape 边界 **外侧** | 类似 `box-shadow`（不计 width） |

ui_design 的 `box-shadow` 是 100% 外绘 — face area 不被侵占。SwiftUI 里和它**视觉最接近**的不是 `strokeBorder`（100% 内绘）而是 `.stroke()`（50/50 内外）。dev 凭 "border" 直觉选了 `strokeBorder`，没意识到 box-shadow 的"外绘"语义只有 `.stroke()` 或真 `.shadow()` 能近似还原。

### 修复（Fix）

`Avatar.swift` ring 分支：`.strokeBorder(...)` → `.stroke(...)`，同时保留 `.padding(-2)` 让外圈 accent 略大于内圈 surface。

```swift
// Before: 内绘 → face area 被侵占 ~5pt
Circle().strokeBorder(theme.colors.surface, lineWidth: 3)
Circle().strokeBorder(theme.colors.accent, lineWidth: 2).padding(-2)

// After: 中心线 → 50% 在外，face area 仅被侵占 ~2.5pt（且更接近 box-shadow 视觉）
Circle().stroke(theme.colors.surface, lineWidth: 3)
Circle().stroke(theme.colors.accent, lineWidth: 2).padding(-2)
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 SwiftUI 里**还原 ui_design / web 的 `box-shadow` halo / ring 效果**时，**优先用 `.stroke()` 或 `.shadow(radius: 0, x: 0, y: 0)`，禁止用 `.strokeBorder()`**。
>
> **展开**：
> - `box-shadow` 是**外绘**语义（不计 frame size）；`.strokeBorder` 是**内绘**语义（计入 frame size）。两者像 reciprocal — 直接互替会让被框 shape 的可见尺寸缩水 `lineWidth × 2`。
> - 如果 ui_design 的 css/jsx 用的是 `border:`（对应 `box-sizing: border-box`），那 `.strokeBorder` 才对；只有此时直觉才能信。
> - **快速判定**：看 ui_design 源码用 `boxShadow:` 还是 `border:`。`boxShadow` → `.stroke` / `.shadow`；`border` → `.strokeBorder`。
> - **反例**：`Circle().strokeBorder(.surface, lineWidth: 3)` + 注释里写"对齐 box-shadow ring" — 注释和实现矛盾，未来 reviewer 看实现忽视注释，bug 隐形传播。

## Lesson 2: SwiftUI 按钮的"按下视觉"必须走 ButtonStyle / configuration.isPressed，不要自己挂 DragGesture

- **Severity**: medium
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/DesignSystem/Primitives/PrimaryButton.swift:75-81`

### 症状（Symptom）

PrimaryButton 用 `.simultaneousGesture(DragGesture(minimumDistance: 0).onChanged { isPressed = true }.onEnded { isPressed = false })` 实现按下视觉。当用户按住按钮 → 把手指**拖出按钮范围**（在 scrollable 屏幕极常见）→ 直到手指松开 `onEnded` 才会触发。期间 `isPressed` 卡在 `true`，按钮 stuck 在 `offset(y: 2)` 的 pressed 视觉，并且这次 tap 已经被系统 cancel 了（因为拖出 button bounds 太远），但视觉上还像还按着，给用户"按钮卡死"的错觉。

### 根因（Root cause）

`DragGesture` 的 `onEnded` 只在**手指真的松开**时触发，不区分"手指松开在按钮内 vs 拖出后松开"。SwiftUI Button 的系统手势会在拖出 button bounds 时**取消 tap action**（这是正确的 — 用户把手指划走表示反悔），但**自定义的 simultaneousGesture 不感知这个取消信号**，因此 `isPressed = false` 永远不会被触发到 → 视觉与系统手势状态脱节。

SwiftUI 钦定的处理路径是 `ButtonStyle.makeBody(configuration:)` + `configuration.isPressed`：这个布尔由框架管理，自动覆盖 cancellation / drag-out / scroll-conflict 等所有边界 case。dev 选自定义 DragGesture 是因为不熟 ButtonStyle API，绕过框架后就要自己 reimplement 整个手势状态机 — 而 SwiftUI 的手势状态机比想象的复杂。

### 修复（Fix）

抽 `PressedOffsetButtonStyle: ButtonStyle`（同文件下方），把 `offset(y:)` + `animation(.easeOut, value:)` 搬到 `makeBody(configuration:)` 里用 `configuration.isPressed`；删掉 PrimaryButton 的 `@State isPressed` + `.simultaneousGesture(DragGesture...)`；body 末尾 `.buttonStyle(.plain)` 改 `.buttonStyle(PressedOffsetButtonStyle())`。

```swift
// Before: 自定义 DragGesture（不感知 cancellation）
@State private var isPressed: Bool = false
// ...
.offset(y: isPressed ? 2 : 0)
.animation(.easeOut(duration: 0.1), value: isPressed)
// ...
.buttonStyle(.plain)
.simultaneousGesture(
    DragGesture(minimumDistance: 0)
        .onChanged { _ in isPressed = isEnabled }
        .onEnded { _ in isPressed = false }
)

// After: SwiftUI 钦定路径，框架管理 isPressed
.buttonStyle(PressedOffsetButtonStyle())

// + 同文件
public struct PressedOffsetButtonStyle: ButtonStyle {
    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .offset(y: configuration.isPressed ? 2 : 0)
            .animation(.easeOut(duration: 0.1), value: configuration.isPressed)
    }
}
```

加测试：`PrimitivesTests.testPressedOffsetButtonStyleConformsToButtonStyle` — type-level 守护（不渲染、不断 frame size，符合 ADR-0002 §3.1 禁视觉测试约束）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 SwiftUI 里**实现 Button 的按下视觉**（offset / scale / opacity / color shift）时，**必须**用 `ButtonStyle` + `configuration.isPressed`，**禁止**用 `simultaneousGesture(DragGesture)` + `@State isPressed` 自己实现。
>
> **展开**：
> - SwiftUI Button 的系统手势会在用户拖出 button bounds 时取消 tap，但**只有 ButtonStyle 拿得到这个取消信号**（通过 `configuration.isPressed → false`）。自定义 DragGesture 看不到 → 状态脱节 → 视觉卡 pressed。
> - 也别用 `.gesture(...)`/`.onLongPressGesture` 实现"按下视觉"。前者抢系统 tap action 互斥，后者有 minimum duration 干扰短按反馈。
> - 如果需要 ButtonStyle 拿不到的额外 state（如长按 hold 计时器、双击 tap 区分），那才考虑 `.onLongPressGesture` 系列；纯按下视觉**不在此列**。
> - **反例**：`@State var isPressed; ...; .simultaneousGesture(DragGesture(minimumDistance: 0).onChanged { isPressed = true }.onEnded { isPressed = false })` — 经典坑，拖出按钮卡 pressed，未来 reviewer / QA 在 scrollable view 里复现时会以为是其他 bug。
> - **反例**：在 ButtonStyle 内部又叠 `simultaneousGesture` 想"加增强" — 等于把上面这个坑搬回来。ButtonStyle 内只用 configuration.isPressed。

---

## Meta: 本次 review 的宏观教训

两条 finding 都是同一个思维模式：**用 web/CSS 直觉直接套 SwiftUI API 的同名概念，没核对底层语义差异**。

- `border` ≠ `strokeBorder`（前者按 box-sizing 决定，后者绝对内绘）
- "按钮按下"在 web 是 `:active` pseudoclass（CSS 全自动），在 SwiftUI 必须走 ButtonStyle（不是任意 gesture 都行）

未来 Claude 把 ui_design 的 jsx/css 翻译成 SwiftUI 时，**优先看是否有 SwiftUI 钦定的 idiom**（ButtonStyle / configuration / view modifier），再考虑手搓 gesture / view state。手搓的成本不只是写多几行 — 是要 reimplement 整个 SwiftUI 帮你 handle 的边界 case 集合。
