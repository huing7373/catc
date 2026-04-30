---
date: 2026-04-30
source_review: codex review (Story 37.6 round 5) — file: /tmp/epic-loop-review-37-6-r5.md
story: 37-6-shared-primitives
commit: <pending>
lesson_count: 4
---

# Review Lessons — 2026-04-30 — ViewModifier @State 跨 .id 重建幸存陷阱 + `.shadow` 投影到 children + fix-review 5 轮 cap 破例决议

## 背景

Story 37.6 进入 fix-review **第 5 轮** review（codex 触发）。前 4 轮分别修了：
- r1：FadeIn 起点 offsetY 方向（正负号反向）
- r2：StrokeBorder vs stroke 内外绘语义 + ButtonStyle vs DragGesture 取消语义
- r3：PrimaryButton modifier 顺序（padding vs frame(maxWidth:)）+ ButtonStyle 化
- r4：FadeIn `.id(nil)` 共享 explicit identity 陷阱（条件挂 .id）

r5 codex 又揪出 3 个 [P2]：FadeIn id 重放路径不工作 + Card / PrimaryButton `.shadow` 投影到 children。
`/epic-loop` skill 内置 5 轮 cap，r5 触达 cap，但每轮都是真 bug 收敛 fix（非 path B 反复打架），user 钦定**破例**再发一轮。

本次产出 3 条技术 lesson + 1 条 process lesson（破例决议元教训），共 4 条。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | FadeIn @State 跨 `.id(...)` 重建幸存 → 重放路径不触发动画 | medium (P2) | architecture | fix | `iphone/PetApp/Core/DesignSystem/Primitives/FadeIn.swift` |
| 2 | `.shadow` 挂在最外层 → 投影渲染到 Card children text/icons | medium (P2) | ui-fidelity | fix | `iphone/PetApp/Core/DesignSystem/Primitives/Card.swift` |
| 3 | `.shadow` 挂在最外层 → 投影渲染到 PrimaryButton label text/icons | medium (P2) | ui-fidelity | fix | `iphone/PetApp/Core/DesignSystem/Primitives/PrimaryButton.swift` |
| 4 | fix-review 5 轮 cap 触达：破例 vs HALT 决策标准 | meta | process | record | (Meta lesson) |

## Lesson 1: FadeIn @State 跨 `.id(...)` 重建幸存 → id 重放路径不触发动画

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/DesignSystem/Primitives/FadeIn.swift:36-56`

### 症状（Symptom）

`FadeInModifier(id:)` 设计意图：caller 传变化的 id 让动画**重放**（如 tab 切换 / 内容刷新）。
实际行为：第一次 onAppear 跑完后 `visible = true` 持久；id 变化导致 `.id(id)` 重建子树 → 第二次 onAppear 跑时 visible 已经是 true → withAnimation 看到无差值 → **没有动画**。tab 切换 / 内容刷新只动一次。

### 根因（Root cause）

SwiftUI 的 `.id(id)` 重建语义只作用于**被 `.id` 直接修饰的 view 子树**（即 content 部分）；
ViewModifier 自身的 `@State` 属于 **modifier 实例**，不在 `.id` 重建范围内。
所以 `.id(id)` 触发 onAppear 重跑没问题，但 `visible` 这个 @State 跨 id 变化幸存 → onAppear 闭包里的 `visible = true` 是 no-op（已经是 true）。

技术误判路径：以为 `.id(id)` 会"整个重建包括 ViewModifier"，实际只重建 content。

### 修复（Fix）

显式监听 id 变化，先把 `visible` 拨回 false（不带动画，瞬时），再用 withAnimation 驱动到 true，让 SwiftUI 看到差值渲染过渡曲线：

```swift
let core = content
    .opacity(visible ? 1 : 0)
    .offset(y: visible ? Self.offsetEndY : Self.offsetStartY)
    .onAppear {
        withAnimation(.easeInOut(duration: 0.28)) { visible = true }
    }
    .onChange(of: id) { _, _ in
        visible = false                                       // 瞬时回 false
        withAnimation(.easeInOut(duration: 0.28)) {
            visible = true                                    // 带动画到 true
        }
    }
```

iOS 17+ `onChange` 用 `(of:initial:_:)` 闭包带 (oldValue, newValue) 双参签名（项目 deploymentTarget = iOS 17.0，见 `iphone/project.yml`）。

附带 1 case 守护测试：`testFadeInModifierIdReplayPathConstructs`（type-level，不渲染，符合 ADR-0002 §3.1 禁视觉测试）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写 SwiftUI **ViewModifier 含 @State 且想用 `.id(...)` 触发"重新初始化"** 时，**必须**意识到 ViewModifier 的 @State 不在 `.id` 重建范围内，**必须**用 `.onChange(of:)` 显式重置 @State，**禁止**仅靠 `.id(...) + onAppear` 重放。
>
> **展开**：
> - `.id(value)` 只重建被它直接修饰的 view 子树（content）；ViewModifier 自身的 storage（@State / @StateObject）跨 id 变化幸存。
> - 任何"caller 传变化 id 期望动画 / 状态重置"的场景，都要在 ViewModifier body 里同时挂 `.onChange(of: id)` 把内部 @State 显式拨回初值。
> - withAnimation 的差值检测：必须让被动画值"先变到 from，再变到 to"，否则 SwiftUI 看不到差值不渲染过渡。
> - **反例**：只挂 `onAppear { withAnimation { visible = true } }`，依赖 `.id(id)` 重建触发 onAppear 重跑——这不工作，因为 visible 已经是 true，withAnimation 闭包是 no-op。
> - **反例**：在 onChange 闭包里把 `visible = false` 包进同一个 `withAnimation` 块——这会让 false → true 在一帧内被合并优化掉，看不到任何动画。必须让 `visible = false` 走非动画路径。

## Lesson 2: `.shadow` 挂在 view 链最外层 → 投影渲染到 children（Card / PrimaryButton 同模式）

- **Severity**: medium (P2)
- **Category**: ui-fidelity
- **分诊**: fix
- **位置**:
  - `iphone/PetApp/Core/DesignSystem/Primitives/Card.swift:32-47`
  - `iphone/PetApp/Core/DesignSystem/Primitives/PrimaryButton.swift:61-74`

### 症状（Symptom）

链式 `content().padding().background(RoundedRectangle).overlay(...).shadow(...)` 结构里，`.shadow(...)` 挂在最外层 view 上。
SwiftUI 的 `.shadow` 渲染**整棵被修饰子树的 alpha 蒙版投影**——所以不只 RoundedRectangle 投影，连 content() 内的 Text / SF Symbol / 图标都被投影 → 文字/图标边缘有模糊阴影 / 图标 stroke 有黑边。

与 ui_design `primitives.jsx` 的 CSS `box-shadow` 语义（仅外壳投影，不影响 children）不符。

### 根因（Root cause）

混淆 SwiftUI `.shadow` 与 CSS `box-shadow` 语义：
- CSS `box-shadow` 作用在 element 的 **box** 上（盒子边框/背景），不影响 children 的合成
- SwiftUI `.shadow` 是 view modifier，作用于**整个被修饰的 view 子树的 alpha**

正确实现：把 `.shadow` 直接 chain 到提供 chrome 的 shape（`RoundedRectangle.fill(...)`）那一层，作为 background 内部的 modifier，这样投影只渲染 shape 自身的 alpha，不波及外层 content。

### 修复（Fix）

**Card.swift**:
```swift
return content()
    .padding(resolvedPadding)
    .background(
        RoundedRectangle(cornerRadius: resolvedCornerRadius)
            .fill(theme.colors.surface)
            .shadow(  // ← 移到这里，挂在 RoundedRectangle 上
                color: theme.shadow.sm.color,
                radius: theme.shadow.sm.radius,
                x: theme.shadow.sm.x,
                y: theme.shadow.sm.y
            )
    )
    .overlay(RoundedRectangle(...).stroke(...))
    // 删掉外层 .shadow
```

**PrimaryButton.swift**:
```swift
.background(
    RoundedRectangle(cornerRadius: theme.radius.pill)
        .fill(backgroundColor)
        .shadow(color: shadowColor, radius: 0, x: 0, y: shadowY)  // ← 移到这里
)
.overlay(Group { if let borderColor { RoundedRectangle(...).stroke(...) } })
// 删掉外层 .shadow
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 SwiftUI 里实现 **CSS `box-shadow` 语义的"外壳投影 / 不影响 children"**时，**必须**把 `.shadow(...)` chain 在 `RoundedRectangle.fill(...)`（或其他 background shape）那一层，**禁止**挂在 view 链最外层。
>
> **展开**：
> - `.shadow` 是 view modifier，渲染整棵被修饰子树的 alpha 投影；**不**只投影最近的 background shape。
> - 想要"只阴影外壳"必须把 shadow 紧贴到 shape 上，让它作为 background 表达式内部的一环。
> - 检查清单：如果一个 primitive 容器（Card / Button / Sheet）内部会承载 Text / Icon，且要应用 shadow → **必须**把 shadow 嵌进 background。
> - **反例**：`.background(Shape.fill).overlay(Shape.stroke).shadow(...)` —— text/icon children 全部带模糊阴影。
> - **反例**：`.background(Shape.fill).shadow(...).overlay(Shape.stroke)` —— shadow 仍在外层（background + shadow 链已经包含 content padding 后的整个布局），children 仍被投影。
> - **正例**：`.background(Shape.fill.shadow(...)).overlay(Shape.stroke)` —— shadow 嵌入 background 表达式内部，只作用于 fill 的 alpha。
> - 配套 lint 提示：grep `\.shadow\(` 在 primitive 文件里出现位置，确保它紧跟 `.fill(...)` 或包在 `RoundedRectangle.fill(...).shadow(...)` 表达式里，**不**作为 chain 链尾。

## Lesson 3: PrimaryButton shadow 同 Card 同根因（共 1 lesson 拆 2 文件落实）

参见 Lesson 2 — Card 和 PrimaryButton 的 shadow 错位是**同一个心智模型 bug**：把 `.shadow` 当 CSS `box-shadow` 用了。两处独立修复但归同一条预防规则。

## Lesson 4 (Meta): fix-review 5 轮 cap 破例决议（forward-actionable process rule）

- **Severity**: meta (process)
- **Category**: process
- **分诊**: record（不修代码，沉淀决策标准）

### 症状（Symptom）

Story 37.6 review 进入 r5。`/epic-loop` skill 内置 cap = 5 轮，**意图**是防止"path B 反复打架"——即 review 揪 bug → fix 时引入新 bug → 下轮 review 又揪 → 死循环。
但本 story 的 r1-r5 不是 path B 反复打架，每轮都收敛地揪出**新的、独立的、未触及代码区**的真 bug：
- r1: 起点 offsetY 方向（FadeIn）
- r2: StrokeBorder 内外绘 + DragGesture 取消语义
- r3: padding/frame 顺序 + ButtonStyle
- r4: `.id(nil)` 共享 identity（FadeIn）
- r5: ViewModifier @State 跨 `.id` 幸存 + `.shadow` 投影 children

每轮 fix 都是最小改动 + 加 type-level 守护测试 + lesson 沉淀，前 4 轮 lesson 都已 commit。
此为"primitives 复杂度 > 5 轮 cap 假设"的合理情况，不是反复打架。
user 钦定 r5 破例放行，但 main agent 不能据此外推未来都破例。

### 根因（Root cause）

`/epic-loop` 的 5 轮 cap 是**反路径 B 启发式**，不是硬约束。但启发式无法自己分辨"收敛 fix"vs"反复打架"——这要看 fix 的语义内容，不是次数。
main agent 默认守 cap，是因为**让 main agent 自动判断"是否破例"会引入风险**（cap 失去任何兜底意义）。
所以正确解：**main agent 在 5 轮触顶时 HALT 并 ask user**，由 user 看完 lesson 序列做"收敛 vs 打架"的人为判断；user 说"破例"才发 r5+。

### 修复（Fix）

不改代码，改 process rule（写进本 lesson 让未来 Claude 读到）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 main agent / Claude 在 `/epic-loop` 跑某 story 触达 5 轮 fix-review cap 时，**必须** HALT 并 ask user 是否破例，**禁止**自己越线发第 6 轮。
>
> **展开**：
> - cap 触达不是失败信号，是 ambiguity 信号——可能是 path B 反复打架（应停），也可能是 primitive 复杂度高、每轮收敛揪新 bug（可破例）。
> - 判断标准（给 user 决策辅助）：
>   - **path B 反复打架特征**：r(n) fix 改了文件 A → r(n+1) review 揪 A 改坏了别处 → r(n+2) 改回来又揪原 bug。文件改动 churn 高、review 揪同区域多次。
>   - **收敛 fix 特征**：r(n) 揪 file A 区 X → r(n+1) 揪 file B（不同模块或同 file 不同区） → 每轮 fix 文件区段不重叠 / lesson 主题独立。每轮 lesson md 都能独立蒸馏。
> - **判断主体是 user，不是 main agent**——main agent 把 r1-r(n) 的 lesson 主题清单和文件 churn 摘要给 user，user 拍板。
> - **反例**：main agent 看到 r5 揪的 finding 都是真 bug，自己决定"那再发一轮"——这违反 cap 的兜底意义。
> - **反例**：main agent 看到 r5 触顶就强制 abort，丢掉收敛中的 fix——这也是误用，让 cap 变成硬阻塞。
> - **正例**：main agent 在 r5 触顶时，把"r1-r5 lesson 主题清单 + file churn 摘要"递给 user，user 说"破例放行/HALT"才决定下一步。
> - 这条规则对 `/epic-loop` 的所有 cap（fix-review cap、retry cap、test fix cap）通用。

---

## Meta: 本次 review 的宏观教训

r5 的 3 条技术 finding 都指向 **SwiftUI ↔ CSS 心智模型不对齐**：
- Lesson 1（@State 不随 `.id` 重建）：误用 React-style key prop 心智模型套到 SwiftUI ViewModifier
- Lesson 2-3（`.shadow` vs CSS `box-shadow`）：误用 CSS `box-shadow` 语义（shadow only on box）

primitives 实装时（特别是从 ui_design React/CSS 翻译到 SwiftUI 时），需要对 SwiftUI 这两类"看似等价、语义实际不同"的 modifier 做**显式契约对齐**：
- view identity / 状态生命周期：`.id` 范围 ≠ React `key` 范围
- 视觉投影：`.shadow` 范围 ≠ CSS `box-shadow` 范围

未来 primitives 实装 + review 都该把这两类作为 spec 检查项，而不是发现一次修一次。
