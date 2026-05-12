---
date: 2026-05-12
source_review: codex review round 1 of Story 15-1 (file: /tmp/epic-loop-review-15-1-r1.md)
story: 15-1-房间页内多成员猫位渲染-snapshot-pet-currentstate-解析
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-12 — SwiftUI 外层 `.frame(...).clipped()` 只裁不缩；内容硬编码尺寸的子 view 必须显式参数化 size

## 背景

Story 15.1 给 `RoomScaffoldView` 成员行添加 `PetSpriteView` 渲染当前猫状态（替换原 paw icon）。
round 1 实装：`PetSpriteView(state: ...).frame(width: 40, height: 40).clipped()`。
codex round 1 抓到：实际运行时每个成员行渲染的是**被裁切的猫头**而非缩放到 40pt 的小 sprite ——
因为 `PetSpriteView` 内部 `spriteImage` helper 硬编码 `.frame(width: 180, height: 180)`，
外层 `.frame(width: 40, height: 40).clipped()` 仅把 180×180 的 SF Symbol 裁掉超出 40pt 的部分，
**完全没有缩放**。AC2 钦定的"在成员行右侧渲染小尺寸 sprite"视觉契约失效。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | PetSpriteView 在 40pt 容器内被裁切而非缩放 | P2 | correctness/UI | fix | `iphone/PetApp/Features/Home/Views/PetSpriteView.swift` + `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift:340-344` |

## Lesson 1: SwiftUI 外层 `.frame(w:h:).clipped()` 不会"缩放"内容硬编码尺寸的子 view —— 子 view 必须暴露 size 参数

- **Severity**: P2
- **Category**: correctness / UI（SwiftUI layout idiom）
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/Views/PetSpriteView.swift:99-105` + `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift:340-345`

### 症状（Symptom）

`PetSpriteView` 内部：

```swift
private func spriteImage(symbol: String, tintColor: Color) -> some View {
    Image(systemName: symbol)
        .resizable()
        .scaledToFit()
        .frame(width: 180, height: 180)   // ← 硬编码 180pt
        .foregroundColor(tintColor.opacity(0.7))
}
```

`RoomScaffoldView` 成员行调用：

```swift
PetSpriteView(state: ...)
    .frame(width: 40, height: 40)   // ← 外层 40pt
    .clipped()                       // ← 想"裁出 40pt"
```

期望：成员行右侧看到 40pt 的小猫 sprite。
实际：看到 40pt 框里**塞着 180×180 SF Symbol 的左上角一小块**（猫的鼻子或一只爪子的局部），视觉上是"被截断的猫头"——成员行 sprite 直观上"显示不全 / 渲染错误"。

### 根因（Root cause）

SwiftUI 的 layout 协议是：**子 view 先报告自己理想大小 → 父 view 给出建议尺寸 → 子 view 按自己规则决定最终大小**。

`.frame(width: 40, height: 40)` 给子 view 提议 40×40，但**子 view 可以拒绝**：当子 view 内部已经用 `.frame(width: 180, height: 180)` 把自己锁定到 180×180 时，外层 frame 只能**给布局占位 40×40，子 view 仍然按 180×180 渲染出去**——内容溢出。

`.clipped()` 只裁掉溢出部分，**绝不缩放**。它修改的是渲染层（compositing），不修改 layout 树。所以 180×180 的 SF Symbol 被裁成 40×40 的一小块，呈现为"截断的图标"。

要让外层 size 真正影响内容：

1. **首选**：让子 view 参数化 size（本次方案）—— `init(size: CGFloat = 默认)`，子 view 用 `size` 决定内部 `.frame()`；调用方传 40 就真渲染 40pt。
2. **备选 A**：用 `.scaleEffect(40.0/180.0)` 把整个子 view 缩 0.222 倍 —— 但 `.scaleEffect` 是 visual transform，**不**影响 layout（即 hit test / a11y frame 仍按 180×180 算，与父容器对齐错位），且要小心 anchor。
3. **备选 B**：把子 view 内部硬编码的 `.frame()` 去掉，改成由父决定大小 —— 但这会让所有 caller 都得自己挂 frame，破坏现有 HomeView 调用的视觉基线。

### 修复（Fix）

PetSpriteView 添加 `size: CGFloat = 180` 参数（默认 180 保持 HomeView caller 视觉零回归）：

```swift
public struct PetSpriteView: View {
    public let state: MotionState
    public let size: CGFloat   // ← 新增

    public init(state: MotionState, size: CGFloat = 180) {
        self.state = state
        self.size = size
    }

    // ...
    private func spriteImage(symbol: String, tintColor: Color) -> some View {
        Image(systemName: symbol)
            .resizable()
            .scaledToFit()
            .frame(width: size, height: size)   // ← 用 size 而非硬编码 180
            .foregroundColor(tintColor.opacity(0.7))
    }
}
```

RoomScaffoldView 成员行：

```swift
PetSpriteView(
    state: (state.memberPetStates[member.id] ?? .rest).motionState,
    size: 40                  // ← 真正告诉 PetSpriteView 渲染 40pt
)
.frame(width: 40, height: 40)  // ← 保留（确保 layout 占位也是 40pt；防御性）
// 移除 .clipped()（不再需要裁切，因为内容真就 40pt）
```

HomeView 几处 `PetSpriteView(state: ...)` 调用因 `size` 有默认值 180 不需要改动。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 SwiftUI **想给一个内容硬编码 `.frame(W,H)` 的子 view 套上不同尺寸**时，**必须**给子 view 增加 `size` 参数（或把内部 frame 删掉让父决定），**禁止**只在外层套 `.frame(...).clipped()` 当成"缩放"用 —— `.clipped()` 只裁不缩，会得到截图式残缺视觉。

> **展开**：
> - `.frame(w:h:)` 是**给子 view 提议**的尺寸，子 view 可以拒绝（特别是内部已经挂了 `.frame()` 把自己钉死的子 view）。被拒绝时子 view 仍按自己尺寸渲染，溢出父容器。
> - `.clipped()` / `.mask(...)` 是渲染层操作，**永远不缩放**内容，只决定哪些像素被显示出来。误用为"resize" 是新手常见陷阱。
> - `.scaleEffect(_:)` 是 visual transform，会缩放视觉但**不**影响 layout / hit test / a11y frame；如果父容器靠 layout 对齐内容（如 HStack 间距），用 scaleEffect 会留下"空白占位"。仅在确实要 visual-only 缩放（如点击反馈缩放动画）时用。
> - **首选 idiom**：子 view 暴露 `size: CGFloat`（或 width/height 分开）参数；默认值保持现有 caller 的视觉基线；新 caller 传需要的 size。调用方同时挂 `.frame(width: size, height: size)` 是**冗余但无害**的防御（layout 与内容双重锁定）。
> - **反例 1**：`MyView().frame(width: 40, height: 40).clipped()`，但 MyView 内部硬编码 `.frame(width: 180, height: 180)` —— 视觉表现为"180×180 内容只露出 40×40 的左上角"（本次踩坑）。
> - **反例 2**：用 `.scaleEffect(40/180)` 替代参数化 —— 视觉缩了，但 a11y frame / hit test / layout 占位仍按 180×180 算，可能让父 HStack 间距错位、UITest a11y identifier 命中点偏移。
> - **验证方法**：**必须**用 ios-simulator MCP 实跑截图肉眼确认（参考 CLAUDE.md "iOS UI 验证（必跑）"一节）。`bash iphone/scripts/build.sh` 通过只验证编译；UI 缩放 / 裁切类视觉 bug 单测和 build 都抓不到，只能眼睛看截图。本次就是 build pass + 单测 575 个全绿，但视觉直接断在 codex 截图分析上。

### 顺带改动

无 —— 仅 PetSpriteView 增加 `size` 参数 + RoomScaffoldView 调用点改两行（加 `size: 40`、删 `.clipped()`）。HomeView 4 处 caller 因 size 默认值 180 全部零改动，无视觉回归。
