---
date: 2026-05-01
source_review: codex review round 3 — Story 37.11 ProfileScaffoldView (file: /tmp/epic-loop-review-37-11-r3.md)
story: 37-11-profileview-scaffold
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-01 — Scaffold View 必须经 ViewModel method seam（按钮闭包 / sheet swipe-dismiss 都不能绕过）

## 背景

Story 37.11 是 ProfileScaffoldView 的 UI 骨架（Epic 37 红线：UI Scaffold 数据完全 mock，零 UseCase / API）。round 3 codex review 抓到 2 条结构性 [P2]：

- **F1**：headerCard 的 bell / settings 圆形按钮闭包**直接写** `state.lastToastMessage = "..."` 绕过 ViewModel —— 与同 view 内其他 action（onWeChatCardTap / onMenuTap 等）走 ViewModel method seam 的模式不一致。
- **F2**：`.sheet(isPresented: $state.showBindModal) { ... }` 的 binding 在 user swipe-dismiss 时 SwiftUI 直接把 binding 设 false，**不调** `ViewModel.onWeChatModalDismissTap()` —— 即 swipe path 在 ViewModel invocations 数组里完全消失，后续 epic（如 `lastWechatPromptAt` 持久化）会 silently skip。

两条都是"View 层旁路 ViewModel method seam"的同一类反模式，统一沉淀为本 lesson。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | bell / settings 按钮闭包绕过 ViewModel seam | P2 | architecture | fix | `iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift` + `ProfileViewModel.swift` + `Mock/RealProfileViewModel.swift` |
| 2 | `.sheet(isPresented:)` swipe-dismiss 绕过 onWeChatModalDismissTap | P2 | architecture | fix | `iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift` |

## Lesson 1: 按钮闭包必须调 ViewModel method，不能直写 @Published

- **Severity**: P2
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift:68-72`

### 症状（Symptom）

ProfileScaffoldView 的两个 headerIconButton 闭包形如：

```swift
headerIconButton(iconKey: "bell") {
    state.lastToastMessage = "消息中心（敬请期待）"
}
headerIconButton(iconKey: "settings") {
    state.lastToastMessage = "设置（敬请期待）"
}
```

直接 mutate `@Published`，没经过 `ProfileViewModel.onBellTap()` / `onSettingsTap()` 之类的 method seam。同文件内其他 action（`onWeChatCardTap` / `onMenuTap` / `onCollectionViewAllTap`）都走 ViewModel method。

### 根因（Root cause）

写 scaffold 时一时偷懒：「反正现在就是写个 toast 字符串，等后续 epic 接 NavigationLink 再补 method」。但 Epic 37 红线明示"zero-edit scaffold"——**未来 epic 接真实导航 / 业务逻辑时只改 ViewModel 子类，不能再回头改 ProfileScaffoldView**。"toast 占位行为要不要走 method seam"在 spec 里其实已隐性钦定（其他 4 个 toast occurrence 都走 method），但容易被"纯本地 mutation 看起来没必要建 method"的直觉冲掉。

### 修复（Fix）

1. `ProfileViewModel` 加 2 个 abstract method（与现有 5 个同模式）：
   ```swift
   public func onBellTap() {
       fatalError("ProfileViewModel.onBellTap must be overridden by subclass")
   }
   public func onSettingsTap() {
       fatalError("ProfileViewModel.onSettingsTap must be overridden by subclass")
   }
   ```
2. `MockProfileViewModel.Invocation` 加 `.bellTap` / `.settingsTap` case；override 实装写 `lastToastMessage` + append invocation。
3. `RealProfileViewModel` 各 override 实装本地 mutate（写 `lastToastMessage` 占位 + os_log；预防 lesson 6 反模式 `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`）。
4. `ProfileScaffoldView` 闭包改成 `state.onBellTap()` / `state.onSettingsTap()`。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写 Scaffold View 的按钮 / 手势 / 链接闭包时，**禁止**直接 mutate `@Published` 字段；**必须**调用 ViewModel 上的具名 method，即使该 method 当前实装就是一行 toast 写入。
>
> **展开**：
> - View 层闭包里**只允许**：调 ViewModel method、读 `@Published` 渲染、`@State` 本地 UI 副作用（如 sheet animation）。**不**允许 `state.someField = ...`（除非 SwiftUI binding 双向绑定的内部 control，如 TextField 的 text binding）。
> - method 命名遵循 `on<Subject>Tap` / `on<Subject>Dismiss` 模式，与同 ViewModel 现有 method 平行；写 fatalError abstract 占位，Mock + Real 子类各 override。
> - Mock 子类 override **必须** append invocation case；Real 子类 override **必须**本地 mutate state（lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`，不能只 log）。
> - **反例**：`Button(action: { state.lastToastMessage = "X" })` —— 不论 X 是占位字符串还是真实文案，都是反模式；后续 epic 接 NavigationLink / UseCase 时必须改 View，违反 zero-edit scaffold 契约。
> - **判据**：写完 closure 后扫一眼，闭包内除了调 ViewModel method 外是否还有其他对 ViewModel `@Published` 的写入？有就改成 method。

## Lesson 2: `.sheet(isPresented:)` swipe-dismiss 不会调 dismiss handler，必须用 `onDismiss:` 闭包路由

- **Severity**: P2
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift:51-53`

### 症状（Symptom）

```swift
.sheet(isPresented: $state.showBindModal) {
    bindWechatSheet
}
```

User 在 modal 里点"稍后再说" → `Button(action: { state.onWeChatModalDismissTap() })` → ViewModel hook 被调，invocations 收录。
User swipe-down dismiss → SwiftUI 直接把 `$state.showBindModal` 设 false → `state.onWeChatModalDismissTap()` **从不被调**，invocations 不记录。

后续 epic 在 `onWeChatModalDismissTap` 里加 `lastWechatPromptAt` 持久化逻辑时，swipe path 会 silently skip 持久化 —— bug 难复现且排查难。

### 根因（Root cause）

`.sheet(isPresented:)` 的 binding 在 SwiftUI 模型里是**直接的 source-of-truth**，swipe-dismiss 是 SwiftUI 框架层行为，框架直接 set binding 而不调任何用户代码。"`showBindModal = false` 即 dismiss"这种心智模型把 ViewModel hook 当成"`showBindModal` setter 的语义副本"，但其实**两条路径相互独立**：按钮路径调 hook 然后 hook 设 false；SwiftUI 路径直接设 false 跳过 hook。

类似坑还存在于 `.alert(isPresented:)` / `.fullScreenCover(isPresented:)` / `NavigationLink(isActive:)` 等所有 isPresented binding 类型——swipe / 系统 back 手势 / 编程 dismiss 都直接打 binding，不调 user code。

### 修复（Fix）

改用带 `onDismiss:` 闭包形式：

```swift
.sheet(
    isPresented: $state.showBindModal,
    onDismiss: { state.onWeChatModalDismissTap() }
) {
    bindWechatSheet
}
```

`onDismiss:` 闭包在**所有** dismiss 路径（按钮 + swipe + 编程 set false）都触发。要求 ViewModel hook **idempotent**：SwiftUI 已经先把 binding 设 false 了，hook 内再 `showBindModal = false` 是 no-op；hook 实装应把"用户主动 dismiss 行为副作用"（如未来的 `lastWechatPromptAt` timestamp 写入）内嵌，不依赖 `showBindModal` 当前值判分支。

不选 option B `.interactiveDismissDisabled(true)`：UX 较差，强迫用户只能点按钮关。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 用 SwiftUI 的 `.sheet(isPresented:)` / `.alert(isPresented:)` / `.fullScreenCover(isPresented:)` / `NavigationLink(isActive:)` 时，**只要 ViewModel 端有"dismiss 时执行某操作"的语义需求**（hook method / 持久化 / log invocation），**必须**走 `onDismiss:` 闭包形式（或 `.sheet(item:)` 模式）；**禁止**只在 modal 内的"取消按钮"挂 hook 而依赖 swipe 路径走 binding setter 副作用。
>
> **展开**：
> - 写 `.sheet(isPresented: $x.flag) { ... }` 时立刻问自己："如果 user swipe-dismiss，需要调什么吗？" 有就加 `onDismiss:` 闭包；没有（纯展示 modal 无副作用）才允许省略。
> - `onDismiss:` 闭包调的 ViewModel hook 必须 **idempotent**——因为 SwiftUI 在调 onDismiss 前已先把 binding set false 了，hook 内对该 binding 的 mutation 是 no-op；副作用（持久化、log、状态清理）应直接执行，**不要** `if showBindModal { ... }` 这类基于 binding 当前值的分支。
> - **`.sheet(item:)` 模式**：`Optional` 数据驱动 + `onDismiss:` 兼具，适合 modal 内容随数据变化的场景；本 case 是固定 modal 内容选 `.sheet(isPresented:onDismiss:)` 即可。
> - **测试守护**：写 ViewModel test case 时，模拟 swipe-dismiss 路径——先手动 `state.flag = false`（SwiftUI 副作用），再调 hook（onDismiss 闭包钦定），断言 invocations 记录 + 状态正确。
> - **反例 1**：`.sheet(isPresented: $state.flag) { content }` + 仅在 modal 内"取消按钮"挂 hook —— swipe path 静默跳过 hook，后续 epic 接持久化时 silently skip。
> - **反例 2**：onDismiss 闭包内写 `if state.flag { state.flag = false; recordDismiss() }` —— SwiftUI 已先设 false，分支永不进入，副作用从不触发。

---

## Meta: 本次 review 的宏观教训

两条 finding 都是 "View 层旁路 ViewModel method seam"。共同根因是：写 scaffold 时把 ViewModel seam 当成"业务逻辑入口"理解（"现在没有业务逻辑，就直接 mutate 不就行了"），而 seam 真实语义是 **"未来扩展点"**——Epic 37 zero-edit scaffold 契约钦定后续 epic 只能改 ViewModel 子类不能改 View。任何 view 层闭包对 `@Published` 的直接写入都是把"未来扩展点"埋在了 view 层。

**统一规则**：**Scaffold View 的所有 user input → state mutation 路径必须经 ViewModel method**——按钮闭包、手势闭包、`.sheet/.alert/.fullScreenCover/.navigationLink` 等 isPresented binding 的 dismiss 路径（用 `onDismiss:`）。view 层只允许"读 `@Published` 渲染 + 调 ViewModel method"，不允许"写 `@Published` 字段"（SwiftUI 双向 binding 的内部 control 除外）。
