---
date: 2026-04-30
source_review: codex review of Story 37.6 round 4 (file: /tmp/epic-loop-review-37-6-r4.md)
story: 37-6-shared-primitives
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-30 — SwiftUI `.id(nil)` 共享 explicit identity 陷阱（ViewModifier 默认参容易踩）

## 背景

Story 37.6 第四轮 review。前三轮已修：FadeIn 方向、Avatar inset shadow、PrimaryButton modifier order、strokeBorder vs stroke、ButtonStyle vs DragGesture。本轮 codex 抓的 1 个 [P2] —— `FadeInModifier.body` 无条件 `.id(id)`，即使默认参 `id == nil` 也调用，导致所有用 `fadeIn()`（默认参）的 sibling views 共享同一个 explicit identity（nil）。SwiftUI 的 diffing 算法基于 explicit identity 来匹配视图、保留 state；多个 sibling 共享 nil id 会让 SwiftUI 把它们当成"同一个"视图，引发 state retention bug（视图状态被错误重用、不稳定 diffing）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | FadeInModifier.body 无条件 `.id(id)` 让 nil-id 路径共享 explicit identity | P2 / medium | architecture | fix | `iphone/PetApp/Core/DesignSystem/Primitives/FadeIn.swift:44` |

## Lesson 1: SwiftUI `.id(nil)` 不是 no-op —— 它是 explicit identity 为 nil

- **Severity**: medium (P2)
- **Category**: architecture（SwiftUI diffing / view identity 语义）
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/DesignSystem/Primitives/FadeIn.swift:44`

### 症状（Symptom）

`FadeInModifier.body(content:)` 实现是：

```swift
public func body(content: Content) -> some View {
    content
        .opacity(visible ? 1 : 0)
        .offset(y: visible ? Self.offsetEndY : Self.offsetStartY)
        .onAppear { ... }
        .id(id)   // ← id 类型是 AnyHashable?，默认参为 nil
}
```

`fadeIn()` 公共 API 签名 `func fadeIn(id: AnyHashable? = nil)` 让默认调用方完全不传 id —— 但 `.id(nil)` 仍会被调用。SwiftUI 把 `.id(...)` 视作**显式身份赋值**（不论参数是否 nil），所以多个用默认参的 sibling 都会被打上"id == nil"这个**相同**的 explicit identity，diffing 会把它们当成可互换的同一节点，引发 state retention（A view 的 @State 被复用到 B view）/ 不稳定动画。

### 根因（Root cause）

误以为 `.id(nil)` 是"不挂 id"。**它不是**。SwiftUI 的 `.id(_:)` modifier 接受任何 `Hashable`（包括 `Optional<AnyHashable>` 包出的 nil），把这个值作为 view 的 explicit identity；nil 也是合法 identity 值。"不挂 id" 的写法是**根本不调用 `.id(...)` modifier**，让 SwiftUI 走 implicit identity（基于 view 类型 + 在父视图中的位置）。

ViewModifier 把"可选 id"做成默认参很常见（API 友好），但 body 实现里必须**条件性挂载** —— `if let id = id { core.id(id) } else { core }`，否则就把"不传 id 就走 implicit identity"的 API 承诺打破了。

### 修复（Fix）

`FadeIn.swift` body 改成 @ViewBuilder + if/else 拆两条路径：

```swift
@ViewBuilder
public func body(content: Content) -> some View {
    let core = content
        .opacity(visible ? 1 : 0)
        .offset(y: visible ? Self.offsetEndY : Self.offsetStartY)
        .onAppear {
            withAnimation(.easeInOut(duration: 0.28)) {
                visible = true
            }
        }
    if let id = id {
        core.id(id)
    } else {
        core
    }
}
```

公共 API `fadeIn(id: AnyHashable? = nil)` 签名不变，外部 caller 不受影响。

测试侧加 case#6b（`PrimitivesTests.testFadeInModifierNilIdConstructsAndBodyCompiles`）做 type-level 守护：验证 nil id / 默认参 / 显式 nil / 非 nil id 四条构造路径都能编译通过；不试图在 runtime 断 SwiftUI 内部 explicit-identity 状态（无 public API）—— 守护停在 type/构造层，符合 ADR-0002 §3.1 禁视觉测试红线。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写 SwiftUI ViewModifier / View extension 且参数是 `Optional<Hashable>` 默认 nil** 时，**禁止**在 body 里**无条件调用** `.id(...)` / 任何接受这个 optional 的 identity-binding modifier；**必须** 用 `if let` 或 `@ViewBuilder` Group 拆条件路径，nil 路径**根本不挂** modifier。
>
> **展开**：
> - SwiftUI `.id(_:)` 接收 `Hashable`，nil 也是合法 explicit identity 值，不是 no-op；
> - 多个 sibling 视图共享同一 explicit identity（包括 nil）→ SwiftUI diffing 会把它们当同一节点，引发 @State 串味 / 动画不稳定 / state retention bug；
> - 同样的陷阱适用于 `.tag(_:)`、`.matchedGeometryEffect(id:in:)` 等任何 identity-binding modifier；
> - 公共 API 把可选 id 暴露成默认参 nil 是合理的（caller 友好），**漏在 body 实现端**——必须条件性挂载；
> - 实现两种合规写法：
>   - `@ViewBuilder body` + `if let id = id { core.id(id) } else { core }`；
>   - View 扩展 helper `applyIfLet(_:transform:)`，链式 `.applyIfLet(id) { $0.id($1) }`；
> - **反例 1**（踩坑）：`content.id(id)` 直接挂、id 为 `AnyHashable?` 默认 nil —— 所有默认 caller sibling 共享 nil identity；
> - **反例 2**（也错）：`content.id(id ?? "default")` —— 这次给了非 nil 默认值，仍然让所有默认 caller 共享 "default" 这个 id，问题没修只是换了 key；
> - **正例**：`if let id = id { content.id(id) } else { content }` —— nil 路径走 implicit identity（SwiftUI 基于位置 + 类型自动分配），非 nil 路径正常挂；
> - 测试守护放在 type/构造层（验证 nil id / 默认参 / 非 nil id 都能编译通过、modifier 可实例化），**不要**试图在测试里断 runtime explicit-identity 行为 —— SwiftUI 无 public API 可断；
> - Story 37.6 round 4 [P2] 是这个规则的具体起源；FadeInModifier `.id(nil)` 被 codex 抓出来是因为 sibling-fadeIn 共用一个 nil identity 会让 Tab 切换 / List 重排时状态错乱。

## Meta: 本次 review 的宏观教训

跨 round 1-4 的累积观察：FadeIn primitive 一个 ViewModifier 在 4 轮 review 里被抓了 2 次（round 1 抓方向反转、round 4 抓 nil-id 共享），说明**ViewModifier 是高密度陷阱区** —— body 实现里每一个 modifier 都是隐性契约（identity / animation / layout / event），稍有疏忽就会和 SwiftUI 默认行为打架。未来写 ViewModifier 时，建议在 body 里**明示标注**每个 modifier 的语义意图（如 "// 不挂 .id 走 implicit identity"），让 review / future Claude 一眼看出是否合规。
