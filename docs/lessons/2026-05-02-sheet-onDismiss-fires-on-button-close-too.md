---
date: 2026-05-02
source_review: codex review round 4 for story 37-11-profileview-scaffold
story: 37-11-profileview-scaffold
commit: c3f52ab
lesson_count: 1
---

# Review Lessons — 2026-05-02 — SwiftUI `.sheet(onDismiss:)` 在按钮触发关闭时也会跑，必须用 dismissReason tag 做意图分发

## 背景

Story 37.11（ProfileView Scaffold）round 3 修复曾把 `.sheet(isPresented:)` 改成 `.sheet(isPresented:onDismiss:)`，让 onDismiss 闭包无脑调 `state.onWeChatModalDismissTap()`，目的是让 swipe-dismiss 路径也经 ViewModel seam（避免 silent skip）。

round 4 codex review 指出该修法的反作用：**SwiftUI 在按钮触发 sheet 关闭时也会跑 onDismiss 闭包**，导致两条路径出错：

1. "稍后再说" 按钮：按钮闭包先调 `onWeChatModalDismissTap()`（写 `showBindModal=false`）→ SwiftUI 跑 onDismiss → **再调一次 dismiss**（双触发）
2. "绑定微信" confirm 按钮：按钮闭包调 `onWeChatBindConfirmTap()`（写 `wechatBound=true + showBindModal=false`）→ SwiftUI 跑 onDismiss → **错误触发 dismiss path**（confirm 路径不应 fire dismiss）
3. swipe-dismiss：onDismiss 调 dismiss method 一次（这条对）

后续 epic 给 `onWeChatModalDismissTap()` 接持久化（如 `lastWechatPromptAt` 时间戳）时，路径 1 会"用户拒绝一次记两次"、路径 2 会"用户成功绑定也被记为拒绝"——观测语义彻底崩。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `.sheet(onDismiss:)` 在按钮关闭路径也 fire，必须 dismissReason 分发 | medium | architecture | fix | `iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift` |

## Lesson 1: `.sheet(onDismiss:)` fires on every disappear path —— 用 dismissReason tag 做意图分发，按钮闭包不直接调 ViewModel method

- **Severity**: medium (P2)
- **Category**: architecture / ui
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift:59-61`（round 3 修法所在的 `.sheet(...)` 调用点）

### 症状（Symptom）

- 用户点"稍后再说" → `MockProfileViewModel.invocations` 含 `.wechatModalDismissTap` **两次**（应该一次）
- 用户点"绑定微信"确认 → `MockProfileViewModel.invocations` 含 `.wechatBindConfirmTap` + `.wechatModalDismissTap`（应该只 confirm，不 dismiss）
- swipe-dismiss → 一次 `.wechatModalDismissTap`（对）

### 根因（Root cause）

SwiftUI `.sheet(isPresented:onDismiss:)` 的 `onDismiss` 闭包契约是 **"sheet disappear 时无条件 fire"**，**不区分**触发关闭的源（用户按钮 / swipe / 编程 binding 改 false）。round 3 修法误以为 onDismiss 只在"非按钮"路径 fire（"如果按钮自己处理 dismiss，onDismiss 应该不触发"），与文档实际行为相反。

文档 + 实测：
- SwiftUI 看到 `isPresented` 由 true → false（无论是谁改的）就触发 sheet 消失动画；动画完成后跑 onDismiss
- 按钮闭包写 `state.showBindModal = false` 后，是按钮闭包先把 binding 改 false → SwiftUI 检测到 → 走 sheet 关闭路径 → **跑 onDismiss**
- 因此按钮闭包 + onDismiss 是 **串行双触发**，不是 either/or

### 修复（Fix）

引入 `dismissReason` 私有 `@State`（enum: `.confirm` / `.declined` / nil），让按钮闭包**只标记意图 + 关 sheet**，把真正的 ViewModel method 调用集中到 onDismiss 按 reason 分发。

#### before（round 3）

```swift
// .sheet:
.sheet(
    isPresented: $state.showBindModal,
    onDismiss: { state.onWeChatModalDismissTap() }   // ❌ 在所有路径都跑，按钮路径双触发
) { bindWechatSheet }

// "稍后再说" 按钮：
Button(action: { state.onWeChatModalDismissTap() })  // ❌ 自己调一次，再被 onDismiss 调一次

// "绑定微信" confirm 按钮：
Button(action: { state.onWeChatBindConfirmTap() })   // ❌ 调完 confirm，onDismiss 又错触发 dismiss method
```

#### after（round 4）

```swift
private enum DismissReason {
    case confirm     // 用户点"绑定微信"
    case declined    // 用户点"稍后再说"
    // nil = swipe-dismiss
}
@State private var dismissReason: DismissReason?

.sheet(
    isPresented: $state.showBindModal,
    onDismiss: {
        switch dismissReason {
        case .confirm:
            state.onWeChatBindConfirmTap()
        case .declined, .none:
            state.onWeChatModalDismissTap()
        }
        dismissReason = nil   // reset 给下次 sheet 弹出用
    }
) { bindWechatSheet }

// 按钮闭包通过 closure 回调到父视图：
private func handleConfirmRequested() {
    dismissReason = .confirm
    state.showBindModal = false   // 触发 sheet dismiss → onDismiss 调 confirm method
}
private func handleDeclineRequested() {
    dismissReason = .declined
    state.showBindModal = false
}

// BindWechatModalView 子视图改成接受 onConfirmRequested / onDeclineRequested closure，
// 不再直接调 state.method（保持"调用 ViewModel method 是 onDismiss 的专属职责"不变量）
```

不变量：**onDismiss 闭包是 .wechatBindConfirmTap / .wechatModalDismissTap 的唯一调用点**。这让"按钮 + onDismiss 双触发"在结构上不可能发生。

新增守护测试 2 条（`iphone/PetAppTests/Features/Profile/ProfileViewScaffoldTests.swift` case#6d / case#6e）：

- `testSheetDeclinePathDispatchesDismissOnce`：模拟"稍后再说"路径 → invocations 必恰好 `[.wechatModalDismissTap]`（不双触发）
- `testSheetConfirmPathDoesNotDispatchDismiss`：模拟"绑定微信"路径 → invocations 必为 `[.wechatBindConfirmTap]`，**不**含 `.wechatModalDismissTap`

round 3 守护测试 `testSheetSwipeDismissRoutesThroughViewModelHook`（case#6c）保留不动，覆盖 swipe-dismiss 路径。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 SwiftUI `.sheet(isPresented:onDismiss:)`/`.fullScreenCover(...)` 上挂 `onDismiss` 闭包时，**必须**假设它在**所有** disappear 路径（按钮 / swipe / 编程 binding 改 false）都会 fire；**不能**让按钮闭包直接调和 onDismiss 同语义的 ViewModel method —— 否则按钮路径必然双触发。
>
> **展开**：
> - SwiftUI sheet 的 `onDismiss` 不是"非按钮关闭兜底"，是**所有关闭路径的统一回调**。和 UIKit `UIViewController.viewDidDisappear` 同精神（不区分关闭来源）
> - 标准修法：用 `@State private var dismissReason: SomeEnum?` 标记意图，按钮闭包只设 reason + 改 binding；onDismiss 按 reason switch 到正确的 ViewModel method
> - sheet 内部子视图（如本例的 `BindWechatModalView`）也不能直接调 `state.method` —— 必须用 closure callback 让父视图设 reason，**保持"调 ViewModel method"是父视图 onDismiss 闭包的专属职责**这一不变量
> - dismissReason 必须在 onDismiss 末尾置回 nil —— 下一次 sheet 弹出时 swipe-dismiss 路径仍走默认 nil 分支
> - **反例 1**：`.sheet(onDismiss: { state.onDismissTap() }) { ButtonGroup(action: state.onDismissTap) }` — 按钮路径必然双触发
> - **反例 2**：`.sheet(onDismiss: { state.onDismissTap() }) { ButtonGroup(action: state.onConfirmTap) }` — confirm 路径错触发 dismiss method（"用户成功绑定"被记成"用户拒绝"）
> - **反例 3**：用 `if state.somethingChanged { state.onDismissTap() }` 这种"看 state 推断意图"的写法 —— state 已被按钮闭包改完了，从 state 推不出"按钮按了哪个" vs "swipe 关掉了"的区别。必须显式 dismissReason tag
> - **正例**：本 lesson §"after" 代码块（dismissReason enum + onDismiss 集中分发 + 子视图通过 closure 回调）

### 关联

- round 3 lesson 不被 supersede：`2026-05-01-scaffold-bypass-viewmodel-seam.md` 仍坚持"swipe-dismiss 必须走 ViewModel hook" —— round 4 修法在保留这个不变量的同时多处理了"按钮路径不能双触发"维度
- 同主题更宏观的反例：round 3 / round 4 都暴露 **"swiftui modifier 的 callback 触发条件需要文档级精确理解，不能凭直觉假设"** —— 未来挂任何 `onDisappear` / `onChange` / `onReceive` 时都先确认"这个 callback 到底在哪些路径 fire"
