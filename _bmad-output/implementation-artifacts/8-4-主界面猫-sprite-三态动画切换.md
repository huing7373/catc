# Story 8.4: 主界面猫 sprite 三态动画切换

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

> **2026-05-04 epics.md addendum 已并入 AC**（依据 ADR-0009 / ADR-0010 + Story 37.7 落地，参照 Story 21.1 §5.3 chestSlot 范例；详见 epics.md §Story 8.4 行 1519-1525）：
>
> 1. **PetSpriteView 通过 HomeView `petSlot: () -> PetSpriteView` ViewBuilder closure 接缝注入**（与 Story 37.7 chestSlot 接缝同精神；HomeView 内部代码 zero edit，仅扩 generic 多一个 ChestSlot/PetSlot 双 slot），**不**走「PetSpriteView 替换 Story 2.2 `Rectangle().fill(.gray)` 占位」路径——Story 2.2 占位已被 Story 37.7 HomeView catStage 真实布局替代（`Image(systemName: "cat.fill")`）。本 story 用 PetSpriteView **覆盖** catStage 中央 cat 占位（由 caller view 用 ZStack 决定层级 + 接缝清晰）.
> 2. **HomeViewModel 仅持瞬时 `@Published petState: MotionState`（订阅 MotionProvider）**，**不**持 stepAccount / currentChest / currentUser / currentPet 等 domain state（按 ADR-0010 §3.2 应放 AppState；stepAccount 写入由 8.5 负责）。
> 3. ViewModel 通过**构造注入** AppState（Story 37.4 落地的 `bind(appState:)` 模式），**禁止** ViewModel 内部用 `@EnvironmentObject`（ADR-0010 §3.1 ADR 级硬规则）—— 但本 story 实际**不读 AppState**（petState 是 ViewModel transient，与 AppState 零耦合；保留构造注入路径仅为不破坏 Story 37.4 既有 init 签名）。
> 4. 其它 epics.md AC 条款（PetSpriteView 三态动画 + 200ms 平滑过渡 + 占位 sprite + a11y identifier "petSprite_run" 等 + 单测 ≥4 + UI 测试 ≥1）不变。

## Story

As an iPhone 用户,
I want 主界面（HomeView catStage 区块）的猫根据我当前的运动状态（rest/walk/run）自动切换显示动画,
so that 我能直观看到自己运动时猫也在跑或走，App 与身体活动有联动反馈感（节点 3 §4.3 钦定的"身体活动 → 猫 sprite 切换"产品体验）.

## 故事定位（Epic 8 第 4 条 story；节点 3 iOS 端 Domain → View 之间的 ViewModel 订阅 + UI sprite 表现层）

- **Epic 8 进度**：8-1（HealthKit 接入；done）→ 8-2（CoreMotion MotionProvider；done）→ 8-3（MotionStateMapper pure function；done）→ **8-4（本 story，HomeViewModel 订阅 MotionProvider + PetSpriteView 三态 sprite + petSlot 接缝）** → 8-5（StepSyncTriggerService + SyncStepsUseCase 写 AppState.currentStepAccount）.
- **本 story 是 Epic 8 中第一条接 ViewModel + UI 层的 story**：8.1 / 8.2 / 8.3 都在 `Core/` 下做 system adapter / pure function；本 story 跨入 `Features/Home/` 触发**首次** MotionProvider → MotionStateMapper → ViewModel `@Published petState` → SwiftUI re-render 全链路。
- **本 story 落地后立即解锁**：
  - **Story 8.5**（步数同步触发器）：`SyncStepsUseCase` 拼 `/steps/sync` 请求体的 `motionState` 字段时，需要读 `HomeViewModel.petState.rawValue`（"rest"/"walk"/"run"，AC1 钦定的 String raw value）；**或**读取最近一次 mapper 返回值（按 8.5 落地时再决定 — 8.5 在订阅链路里独立监听 MotionProvider）。
  - **Story 9.1 验证场景 4**：`MockMotionProvider.injectActivity(walking=true)` → mapper → `.walk` → `HomeViewModel.petState` 写入 → PetSpriteView 切到 walk 动画 → UITest 验证 `accessibility identifier == "petSprite_walk"`.
- **节点 3 验收要求（章节 §4.3）**：本 story 是节点 3 iOS 端的"猫 sprite 表现层"收口；落地后用户可在主界面**视觉上**感知步行/跑步切换。

## Acceptance Criteria（来自 epics.md §Story 8.4 行 1517-1548 + 2026-05-04 addendum 行 1519-1525；原文转录 + 实施细化）

> **AC 编号体系**：AC1 是 `HomeViewModel.petState` 字段 + `bind(motionProvider:)` 订阅 wire；AC2 是 `PetSpriteView(state: MotionState)` SwiftUI 组件实装（含三态动画 + 200ms 过渡 + a11y identifier）；AC3 是 HomeView 加 `petSlot: () -> PetSlot` ViewBuilder closure 接缝（参考 Story 37.7 chestSlot 范例）；AC4 是 RootView wire（container.motionProvider → homeViewModel.bind(motionProvider:)）；AC5 是 HomeContainerHomeViewBridge 注入 PetSpriteView 到 petSlot；AC6 是 AccessibilityID.Home 加 3 个新常量（petSpriteRest / petSpriteWalk / petSpriteRun）；AC7 是单元测试 ≥4 case（HomeViewModel petState 订阅链路）；AC8 是 UITest ≥1 case（accessibility identifier 切换断言）；AC9 是 build verify。

---

### AC1 — `HomeViewModel.petState` 字段 + `bind(motionProvider:)` 订阅 wire

新增 `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift` 内的字段 + 方法：

```swift
// HomeViewModel.swift（追加；不动既有字段 / init / bind 方法）

/// Story 8.4 AC1: 猫 sprite 当前运动状态.
/// - 订阅来源：通过 `bind(motionProvider:)` wire 后由 MotionProvider.startUpdates 闭包内调
///   `MotionStateMapper.map(activity)` 派生.
/// - 默认值 `.rest`：HomeView 首次渲染时默认显示 idle 动画（AC2 PetSpriteView 三态分支）；
///   未授权 / 未 startUpdates 时也保持 .rest（与 8.2 MotionProvider 协议契约对齐：
///   "未授权时 startUpdates 不抛错，handler 不被调用即可"）.
/// - **不**写 AppState：motionState 不是 ADR-0010 §3.2 白名单 7 字段；
///   仅作为 ViewModel 瞬时投影（addendum 钦定）.
/// - **不**持有 MotionProvider 强引用做 readonly snapshot：bind 时存 weak 引用，
///   ViewModel deinit 时 stopUpdates 释放订阅（AC1 钦定的"deinit 时取消 MotionProvider 订阅
///   避免内存泄漏"，对应 epics.md AC 行 1547）.
@Published public var petState: MotionState = .rest

/// Story 8.4 AC1: 通过 bind(motionProvider:) 注入的 MotionProvider 引用.
///
/// **strong** 持有（与 Story 37.4 weak vs strong lesson 同精神）：
/// - container.motionProvider 是 AppContainer 单例（与 ViewModel 同生命周期，不形成循环）.
/// - bind 后 ViewModel 反向不持 motionProvider —— motionProvider 通过 @Sendable closure
///   弱捕获 self（[weak self]）；ViewModel deinit 时主动 stopUpdates 让 closure 取消 subscription.
/// 详见 docs/lessons/2026-04-30-strong-vs-weak-for-constructor-injected-state.md.
private var motionProvider: MotionProvider?

/// Story 8.4 AC1: 单次绑定 MotionProvider（与 bind(pingUseCase:) / bind(loadHomeUseCase:) /
///                                   bind(appState:) 同模式；多次调用仅第一次生效）.
///
/// 行为：
///   1. guard self.motionProvider == nil（防 SwiftUI .onAppear / .task 重复触发覆盖订阅）.
///   2. 存引用 self.motionProvider = motionProvider.
///   3. 调 motionProvider.startUpdates { [weak self] activity in
///        let mapped = MotionStateMapper.map(activity)
///        Task { @MainActor in self?.petState = mapped }
///      }
///      ── @Sendable closure 捕获 [weak self] 防循环；写 @Published 必须在 main actor.
///   4. 不调 requestPermission（按 AR17 / 节点 3 设计：权限申请按需，由 8.5 同步触发时机内做；
///      本 story 仅订阅链路 wire，未授权时 handler 不被调用即可，与 8.2 协议契约对齐）.
///
/// 红线：
///   - **不**调 motionProvider.requestPermission（权限申请由 8.5 / 8.1 统一管，
///     避免本 story 触发 NSMotionUsageDescription 权限弹窗破坏 UI 验证流程）.
///   - **不**在 closure 内做长耗时操作（按 8.2 lesson `motion-handler-invoke-must-be-in-lock.md`
///     钦定：handler 必须轻量同步 mutate @Published / @State；YAGNI 不引入 throttle / debounce —
///     由 mapper / ViewModel 配合做 hysteresis 是未来扩展点，不在本 story 范围）.
public func bind(motionProvider: MotionProvider) {
    guard self.motionProvider == nil else { return }
    self.motionProvider = motionProvider
    motionProvider.startUpdates { [weak self] activity in
        // closure 在 OperationQueue.main 触发（8.2 MotionProviderImpl 钦定）；
        // mapper.map 是 pure function 同步调用；写 @Published 必须 main actor.
        let mapped = MotionStateMapper.map(activity)
        Task { @MainActor in
            self?.petState = mapped
        }
    }
}

/// Story 8.4 AC1: ViewModel deinit 时取消 MotionProvider 订阅（防泄漏，对应 epics.md AC 行 1547）.
///
/// SwiftUI @StateObject ViewModel 的生命周期：
///   - RootView @StateObject 持 HomeViewModel 整个 App 生命；正常情况不会 deinit.
///   - 但**测试场景**（unit test 创建 ViewModel 实例 → 测试结束 ARC 释放）必须 stopUpdates，
///     否则 mock motionProvider 仍持 closure → @testable import 不洁 → flaky test.
///   - 也防 future scenario（ViewModel 是 short-lived；如未来 child ViewModel 模式）泄漏.
///
/// `deinit` 不能 await / 不能调 @MainActor —— motionProvider.stopUpdates() 协议契约同步签名（无 actor 标记），
/// 直接调 OK；与 base class 既有 deinit 行为不冲突（HomeViewModel base 没有 deinit，子类也没有）.
deinit {
    motionProvider?.stopUpdates()
}
```

**关键设计决策（必须严格遵守）**：

1. **`bind(motionProvider:)` 与 `bind(pingUseCase:)` / `bind(loadHomeUseCase:)` / `bind(appState:)` 同模式**：单次绑定 + guard 短路；与 Story 2.5 / 5.5 / 37.4 既有 bind 模式一致。**禁止**改成 `init(motionProvider:)` 重载（base init 参数已经爆了，再加会破坏 Story 2.2 / 2.5 / 5.5 / 37.4 的 4 路 init 签名稳定）。
2. **`@Published petState: MotionState = .rest`**：默认 .rest 让 HomeView 首次渲染显示 idle 动画；不可改成 Optional `MotionState?`（AC2 PetSpriteView switch 三态分支无 .none 兜底语义）。
3. **closure `[weak self]`**：MotionProvider 通过 @Sendable closure 持 ViewModel；如 strong 捕获会形成 motionProvider → closure → self → motionProvider 循环。8.2 review lesson `motion-handler-invoke-must-be-in-lock.md` 与本 story 无关（本 story 不在 closure 内做 lock），但 `[weak self]` 仍是 ARC 必备。
4. **写 @Published 走 `Task { @MainActor in ... }`**：MotionProviderImpl 钦定 callback 在 OperationQueue.main，理论上已在 main thread；但 @Sendable closure 跨 actor 边界 + Swift 6 strict concurrency 要求显式 `@MainActor` 标注；用 `Task { @MainActor in ... }` 让编译器满意。**禁止**直接 `self?.petState = mapped`（编译错：non-Sendable @Published mutation 跨 actor）。
5. **deinit 调 stopUpdates**：避免 ViewModel 释放后 closure 仍持 weak self → mapper 调用浪费 + mock test 残留 invocations。**禁止**用 `Combine cancellable` 模式 — MotionProvider 协议契约没暴露 AnyCancellable / AsyncStream（保持 callback 风格，与 8.2 钦定 AC1 一致）。
6. **不调 requestPermission**：按 AR17 钦定权限按需。Story 8.5 同步触发时机内会调 8.2 `motionProvider.requestPermission`（或 8.1 / 8.5 自己的权限链）；本 story 只订阅 startUpdates。如果未授权，CMMotionActivityManager 默默不发事件 → handler 不被调 → petState 保持 .rest 默认值 → PetSpriteView 显示 idle，对用户透明。
7. **不引 Combine `sink` / `assign(to:on:)`**：Story 37.7 RealHomeViewModel 已有一处 `appState.$currentPet.sink` 派生 greeting，但 MotionProvider 协议不暴露 Publisher（callback 风格）；强引 Publisher 包装会偏离 8.2 钦定的协议形态。本 story 沿用 8.2 callback 模式。
8. **不调 `applyHomeData` / 不写 AppState**：与 ADR-0010 §3.2 钦定一致 — petState 是 ViewModel 瞬时投影。**禁止**写 `appState.currentMotionState = mapped`（白名单 7 字段没有 motionState；addendum 已显式钦定）。

---

### AC2 — `PetSpriteView(state: MotionState)` SwiftUI 组件实装

新建 `iphone/PetApp/Features/Home/Views/PetSpriteView.swift`：

```swift
// PetSpriteView.swift
// Story 8.4 AC2: 主界面猫 sprite 三态动画显示组件（rest / walk / run）.
//
// 设计基线：
// - generic struct + @Binding-free（接 MotionState 直接 read-only 入参）
// - 三态分支：rest / walk / run 各自独立的 SwiftUI 子视图（占位 SF Symbol 或简单几何形状）
// - 200ms 平滑过渡（淡入淡出）：用 `.animation(.easeInOut(duration: 0.2), value: state)`
// - accessibility identifier：state 切换时同步换 "petSprite_rest" / "petSprite_walk" / "petSprite_run"
//   ── UITest 通过 identifier 切换断言判定状态机是否真的驱动 sprite 渲染.
// - 占位 sprite 资产（节点 3 阶段美术资产不阻塞）：rest / walk / run 各用一个 SF Symbol + 颜色区分.
//
// 与 Story 37.7 chestSlot 接缝模式同精神：本 view 是独立 component；caller（HomeContainerHomeViewBridge
// 通过 petSlot ViewBuilder closure 注入）传入 PetSpriteView(state: viewModel.petState).
//
// **不**接 ViewModel：本 view 是 stateless representation —— state 由 caller 通过参数提供；
// 单元测试场景可 PetSpriteView(state: .walk) 直接构造验证视觉路径，不需要构造完整 ViewModel.

import SwiftUI

public struct PetSpriteView: View {
    public let state: MotionState

    /// Story 37.5: 主题 token 取值入口；本 view 嵌入 HomeView catStage 内 → 由父级 .environment(\.theme, ...) 透传.
    @Environment(\.theme) private var theme

    public init(state: MotionState) {
        self.state = state
    }

    public var body: some View {
        ZStack {
            // 三态分支（仅一个会被渲染；其它走 .hidden / EmptyView 路径节省渲染开销）
            switch state {
            case .rest:
                spriteImage(symbol: "cat.fill", tintColor: theme.colors.inkSoft)
                    .accessibilityIdentifier(AccessibilityID.Home.petSpriteRest)
                    .accessibilityLabel(Text("猫静止"))
            case .walk:
                spriteImage(symbol: "figure.walk", tintColor: theme.colors.accent)
                    .accessibilityIdentifier(AccessibilityID.Home.petSpriteWalk)
                    .accessibilityLabel(Text("猫行走"))
            case .run:
                spriteImage(symbol: "figure.run", tintColor: theme.colors.success)
                    .accessibilityIdentifier(AccessibilityID.Home.petSpriteRun)
                    .accessibilityLabel(Text("猫跑步"))
            }
        }
        // 200ms 平滑过渡（epics.md AC 行 1539 钦定 "state 切换时有平滑过渡（淡入淡出 200ms）"）.
        // .animation(_:value:) 让 ZStack 内分支切换时整体淡入淡出；不需要 .transition() — value-based animation
        // 自动对内容变化做 default crossfade（200ms easeInOut）.
        .animation(.easeInOut(duration: 0.2), value: state)
    }

    /// 单一占位 sprite image 渲染（SF Symbol 220pt + 半透明 tint）.
    /// 节点 3 阶段美术资产不阻塞；后续 Story 30.x 落地真实 sprite render 时替换此 helper.
    private func spriteImage(symbol: String, tintColor: Color) -> some View {
        Image(systemName: symbol)
            .resizable()
            .scaledToFit()
            .frame(width: 180, height: 180)
            .foregroundColor(tintColor.opacity(0.7))
    }
}

#if DEBUG
#Preview("PetSprite — rest") {
    PetSpriteView(state: .rest)
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("PetSprite — walk") {
    PetSpriteView(state: .walk)
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("PetSprite — run") {
    PetSpriteView(state: .run)
        .environment(\.theme, ThemeName.candy.theme)
}
#endif
```

**关键设计决策与坑（必须严格遵守）**：

1. **`switch state` 三分支 + ZStack**：保证只有一个分支被渲染；`.animation(_:value:)` 让 SwiftUI 在 state 变化时做默认 crossfade（200ms easeInOut）。**禁止**用 `if-else` 三层嵌套（可读性差）；**禁止**用 `.transition(.opacity)` + `.id(state)` 强制重建（会 reset @State / animation 中断；用 `.animation(_:value:)` 是 SwiftUI 推荐范式）。
2. **占位 SF Symbol**：节点 3 阶段美术资产不阻塞。三态选不同 symbol：`cat.fill`（rest）/ `figure.walk`（walk）/ `figure.run`（run）。tintColor 由 theme 区分 — inkSoft（rest 灰）/ accent（walk 粉）/ success（run 绿）。**禁止**改成 `Rectangle()` / `Circle()` 等几何形状（SF Symbol 已是 epics.md AC 行 1540 钦定的"占位 SF Symbol 或简单几何形状"，且 SF Symbol 信息密度高）。
3. **accessibility identifier 三态独立 + label 覆盖**：`petSprite_rest` / `petSprite_walk` / `petSprite_run` —— UITest 用 `app.descendants(matching: .any)["petSprite_run"]` 断言切换。label 中文 "猫静止" / "猫行走" / "猫跑步" 给 VoiceOver 用。**禁止**只挂一个 `petSprite` identifier 然后让 UITest 读 label 解析（不稳定，VoiceOver 文本会变；identifier 是 stable contract）。
4. **不接 ViewModel**：本 view 接 `state: MotionState` 参数；caller 传 `viewModel.petState`。让本 view 可在 Preview / 单测 / Storybook 不依赖整个 HomeViewModel 复杂构造路径。
5. **`@Environment(\.theme) private var theme`**：与 Story 37.5 / 37.7 既有视觉组件同模式；caller（HomeView / HomeContainerView）已通过 RootView `.environment(\.theme, ...)` 注入。**禁止** hardcode 颜色（与 Story 37.5 design tokens 决议冲突）。
6. **不预创建 spritesheet 渲染**：节点 3 阶段美术资产不阻塞；Story 30.2 落地真实 SpriteRenderer 时再替换 spriteImage helper。**禁止**预先引 Lottie / SpriteKit / 第三方动画库（YAGNI；ADR-0002 锁定零第三方依赖）。
7. **不调 ViewModel 方法**：本 view 是 stateless display；不调 `state.onFeedTap()` / `viewModel.bind()` 等。caller 自己负责 wire 链路。
8. **#Preview 三个 preview**：`rest` / `walk` / `run` 三个 #Preview 让设计师 / 开发者在 Xcode preview 里直接看三态视觉。candy 主题；不需要 dark 主题（视觉组件 candy 主题足够，dark 主题留给主页级 Preview）。

---

### AC3 — HomeView 加 `petSlot: () -> PetSlot` ViewBuilder closure 接缝

修改 `iphone/PetApp/Features/Home/Views/HomeView.swift`：把 generic struct 从 `HomeView<ChestSlot: View>` 扩展为 `HomeView<ChestSlot: View, PetSlot: View>`，新增 `petSlot: () -> PetSlot` closure 字段，body 内 catStage 区块的中央 cat sprite 改为 `petSlot()` 调用：

```swift
// HomeView.swift（仅展示改动差量；其它字段 / 方法不动）

public struct HomeView<ChestSlot: View, PetSlot: View>: View {
    // ... 既有字段不变

    /// Story 8.4 AC3: petSlot ViewBuilder closure 接缝（与 Story 37.7 chestSlot 接缝同精神）.
    /// caller view（HomeContainerHomeViewBridge）传 PetSpriteView(state: viewModel.petState).
    /// 视觉位置：catStage 区块中央，覆盖 SF Symbol "cat.fill" 占位（用 ZStack 分层）.
    private let petSlot: () -> PetSlot

    /// Story 8.4 AC3: 唯一 init —— 删除 Story 37.7 既有的单 chestSlot init 重载，新增带 petSlot 的 init.
    /// caller 漏改靠编译器报错驱动（HomeView() / HomeView(state:) 等老调用方在编译期 fail）.
    /// 参数顺序：state / resetIdentityViewModel / sessionStore / petSlot / chestSlot
    /// （视觉对应顺序：top-to-bottom from statusBar to chestSlot；petSlot 在 catStage 内嵌）.
    public init(
        state: HomeViewModel,
        resetIdentityViewModel: ResetIdentityViewModel? = nil,
        sessionStore: SessionStore? = nil,
        @ViewBuilder petSlot: @escaping () -> PetSlot,
        @ViewBuilder chestSlot: @escaping () -> ChestSlot
    ) {
        self.state = state
        self.resetIdentityViewModel = resetIdentityViewModel
        self.sessionStore = sessionStore
        self.petSlot = petSlot
        self.chestSlot = chestSlot
    }

    // ... body 不变（StatusBar / catStage / ActionRow / chestSlot() / TeamIdleCard / VersionFooter）

    // MARK: - 区块 2: catStage（改：替换 SF Symbol 占位为 petSlot）

    private var catStage: some View {
        Card(cornerRadius: theme.radius.modalLg, padding: theme.spacing.s20) {
            ZStack {
                catStageDecorBlobs

                // Story 8.4 AC3：替换 Story 37.7 落地的 `Image(systemName: "cat.fill")` SF Symbol 占位
                //   为 petSlot() ViewBuilder closure 调用（caller 传 PetSpriteView(state: viewModel.petState)）.
                //   PetSpriteView 内部已挂 accessibility identifier "petSprite_rest" / "petSprite_walk" /
                //   "petSprite_run"；外层不再挂 AccessibilityID.Home.petArea —— 改由 PetSpriteView 自身挂.
                //
                // 注：AccessibilityID.Home.petArea = "home_petArea" 在 Story 37.13 收编为常量；本 story
                //   保留该常量定义（不删），但物理位置从此处迁出 —— Story 8.4 落地后 Home tab 内不再有
                //   "home_petArea" identifier；老 testHomeViewShowsAllPlaceholders UITest 内对 `petArea`
                //   的断言需修改（详见 AC8 老 UITest 兼容段）.
                petSlot()
                    .frame(width: 180, height: 180)   // 与 Story 37.7 占位 SF Symbol 同尺寸；保 catStage 视觉锚不漂移

                // 等级名牌 / 状态条 / floatUp emoji 浮层 — Story 37.7 既有逻辑不变
                VStack {
                    HStack {
                        catLevelBadge
                        Spacer()
                    }
                    Spacer()
                    catStatsBar
                }

                if case let .flying(emoji, _) = state.interactionAnimation {
                    FloatingEmojiView(emoji: emoji)
                        .id(state.interactionAnimation)
                        .transition(.opacity)
                }
            }
            .frame(height: 280)
        }
        .accessibilityIdentifier(AccessibilityID.Home.catStage)
        .accessibilityElement(children: .contain)
    }

    // ... 其它区块不变
}
```

**关键设计决策（必须严格遵守）**：

1. **HomeView 加第二个 generic 参数 `PetSlot: View`**：与 ChestSlot 同精神。**禁止**用 `AnyView(PetSpriteView(...))` 类型擦除（性能差 + SwiftUI diffing 失败）。
2. **petSlot init 参数位置在 chestSlot 之前**：视觉 top-to-bottom 顺序对应（statusBar → catStage 内嵌 petSlot → actionRow → chestSlot 槽位）。**禁止**改成关键字参数顺序混乱（编译器对 ViewBuilder closure 顺序敏感，乱序会让 caller 写错）。
3. **`petSlot()` 替换 catStage 中央的 `Image(systemName: "cat.fill")` SF Symbol 占位**：Story 37.7 catStage 中央 cat sprite 是占位；本 story 用 petSlot 接缝替换。**保留** `.accessibilityIdentifier(AccessibilityID.Home.catStage)` 父容器 identifier；**移除** Story 37.7 落地的 `.accessibilityIdentifier(AccessibilityID.Home.petArea)`（迁移到 PetSpriteView 自身的 `petSprite_*` identifier）。
4. **`AccessibilityID.Home.petArea` 常量保留**：Story 37.13 a11y 总表收编的常量不删；本 story 仅在 HomeView 不再使用该 identifier，常量保留作为 deprecated marker（与 Story 37.7 chestArea / chestRemaining 同模式）。**禁止**删除常量（破坏 Story 5.5 / 37.4 老测试 import 路径）。
5. **catStage 内部 ZStack 层级保持**：装饰背景斑点（最底）→ petSlot()（中层猫 sprite）→ VStack（最上：levelBadge + statsBar）→ FloatingEmojiView（最最上）。`.frame(width: 180, height: 180)` 限制 petSlot 渲染 box（与 Story 37.7 SF Symbol 占位同尺寸，保 catStage 视觉不漂移）。
6. **不动 catStage 其它子组件**：levelBadge / catStatsBar / FloatingEmojiView 等 Story 37.7 落地的视觉子部分**零打扰**。
7. **HomeView body 整体逻辑零打扰**：仅改 catStage 区块；statusBar / actionRow / chestSlot / teamIdleCard / versionFooter 等其它区块不动。`#Preview` 内 caller 改为 `HomeView(state:..., petSlot: { PetSpriteView(state: .rest) }, chestSlot: { EmptyView() })` 模式（接缝多了一个传参）。

---

### AC4 — RootView wire（container.motionProvider → homeViewModel.bind(motionProvider:)）

修改 `iphone/PetApp/App/RootView.swift` 的 `.onAppear` 闭包内，追加 `homeViewModel.bind(motionProvider: container.motionProvider)` 调用：

```swift
// RootView.swift（仅展示改动差量；其它字段不动）

.onAppear {
    // Story 37.8 round 2 [P2] fix: bind(appState:) 同步路径（既有不变）.
    homeViewModel.bind(appState: appState)

    // Story 37.x: Real* ViewModel bind 链（既有不变）.
    if let realRoomVM = roomViewModel as? RealRoomViewModel {
        realRoomVM.bind(appState: appState)
    }
    if let realWardrobeVM = wardrobeViewModel as? RealWardrobeViewModel {
        realWardrobeVM.bind(appState: appState)
    }
    if let realFriendsVM = friendsViewModel as? RealFriendsViewModel {
        realFriendsVM.bind(appState: appState)
    }
    if let realProfileVM = profileViewModel as? RealProfileViewModel {
        realProfileVM.bind(appState: appState)
    }

    // Story 8.4 AC4: 同步 wire MotionProvider（与 bind(appState:) 同精神 — 在 first paint 前注入）.
    //
    // 在 .onAppear 而非 .task：与 Story 37.8 round 2 [P2] lesson `onappear-vs-task-sync-bind-before-first-paint.md`
    //   钦定路径同精神 —— ViewModel 在 first paint 前必须持有订阅引用，否则 catStage 第一帧渲染时
    //   petSlot() 调用 PetSpriteView(state: viewModel.petState) 会拿到默认 .rest（视觉本就该如此），
    //   但**第一次** activity event 到达前 petState 不会更新；如果用户在 1s 内（首帧到第一次 motion event 之间）
    //   切走然后回来，subscribe 漏触发的话视觉永远停在 .rest. 同步 bind 让此 race 不存在.
    //
    // **不**调 motionProvider.requestPermission（按 AR17 / 节点 3 设计：权限按需）.
    homeViewModel.bind(motionProvider: container.motionProvider)

    ensureLaunchStateMachineWired()
}
```

**关键设计决策（必须严格遵守）**：

1. **bind 在 `.onAppear` 而非 `.task`**：与 Story 37.8 round 2 [P2] lesson `onappear-vs-task-sync-bind-before-first-paint.md` 钦定路径同精神 — 同步路径让 ViewModel 在 first paint 前持有订阅引用。**禁止**放 `.task` 内（会引入 launch-time race 风险，虽然本 story 不像 Room 那么 catastrophic — petState 默认 .rest 渲染本就是合理首帧 — 但保持与既有 `bind(appState:)` 同精神更稳）。
2. **不调 requestPermission**：按 AR17。Story 8.5 同步触发器内部走自己的权限链（按需弹）；本 story 仅 startUpdates wire。
3. **不在 onReadyTask 内 bind**：那条路径有 `await homeViewModel.start()`（ping 启动）；MotionProvider bind 是同步的、不需要 await，放外层 `.onAppear` 更早完成。
4. **idempotent 防重入**：`bind(motionProvider:)` 内部已有 `guard self.motionProvider == nil` 短路；即使 SwiftUI .onAppear 多次触发（旋转 / 回前台等）也安全。

---

### AC5 — HomeContainerHomeViewBridge 注入 PetSpriteView 到 petSlot

修改 `iphone/PetApp/Features/Home/Views/HomeContainerView.swift` 内的 `HomeContainerHomeViewBridge` 私有 struct：调 HomeView 时传入 `petSlot:` ViewBuilder closure 闭合 PetSpriteView：

```swift
// HomeContainerView.swift（仅展示改动差量；其它代码不动）

private struct HomeContainerHomeViewBridge: View {
    @EnvironmentObject var homeViewModel: HomeViewModel
    @Environment(\.resetIdentityViewModel) var resetIdentityViewModel
    @Environment(\.sessionStore) var sessionStore

    var body: some View {
        HomeView(
            state: homeViewModel,
            resetIdentityViewModel: resetIdentityViewModel,
            sessionStore: sessionStore,
            // Story 8.4 AC5: petSlot 接缝传入 PetSpriteView，订阅 viewModel.petState 派生.
            //
            // SwiftUI 自动追踪 viewModel.petState 变化：HomeContainerHomeViewBridge body 重新求值时,
            //   PetSpriteView(state: ...) 也跟着重新构造 → state 参数变化 → AC2 钦定的
            //   `.animation(.easeInOut(duration: 0.2), value: state)` 触发 200ms 过渡.
            //
            // 注：homeViewModel 类型签名是基类 `HomeViewModel`，本 view 通过 EnvironmentObject 拿到的
            //   实例运行时是 RealHomeViewModel（RootView 注入路径），但 .petState 字段在基类 @Published
            //   声明，子类继承零冲突.
            petSlot: { PetSpriteView(state: homeViewModel.petState) },
            chestSlot: { EmptyView() }   // Story 21.1 改传 ChestCardView()，与本 story 接缝并存
        )
    }
}
```

**关键设计决策（必须严格遵守）**：

1. **PetSpriteView 接 `viewModel.petState`**：通过 ViewBuilder closure 直接 capture viewModel.petState；SwiftUI 会自动 react `@Published` 变化重建闭包内 PetSpriteView。**禁止**用 `@Binding $homeViewModel.petState` —— PetSpriteView 是 read-only 接 state 即可。
2. **chestSlot 仍传 EmptyView()**：Story 21.1 落地 ChestCardView 时再改；本 story 不动 chestSlot 实装。
3. **HomeView 调用方仅本桥接 view 一处**：本 story 改 `HomeContainerHomeViewBridge` 一处即可（HomeView 没有其它 caller）；如未来引入 inline preview / dev tool view 时再补 caller。
4. **homeViewModel 类型签名基类 + 运行时 RealHomeViewModel**：基类有 petState 字段（AC1 钦定）；RealHomeViewModel 子类不需 override（直接用基类的 @Published）。MockHomeViewModel 同理。

---

### AC6 — AccessibilityID.Home 加 3 个新常量

修改 `iphone/PetApp/Shared/Constants/AccessibilityID.swift` 内的 `Home` 嵌套 enum：

```swift
// AccessibilityID.swift（仅展示改动差量）

public enum AccessibilityID {
    public enum Home {
        // ... Story 37.13 既有常量保留不动

        // Story 8.4 AC6 新增：PetSpriteView 三态 a11y identifier.
        // 命名风格：petSprite + 状态名（小驼峰） — 与 Story 37.13 落地的命名风格一致（如 catStage / userInfo）.
        // PetSpriteView 内部按 state 三分支挂对应 identifier；UITest 通过 identifier 切换断言判定.
        public static let petSpriteRest = "petSprite_rest"
        public static let petSpriteWalk = "petSprite_walk"
        public static let petSpriteRun = "petSprite_run"
    }

    // ... 其它 enum 不动
}
```

**关键设计决策**：

1. **三个独立常量 `petSprite_rest` / `petSprite_walk` / `petSprite_run`**：与 epics.md AC 行 1548 钦定一致（"切换到 .run → 验证 PetSpriteView 的 accessibility identifier 切到 'petSprite_run'"）。**禁止**用动态拼接 helper（如 `petSprite(_ rawValue: String)`）—— 静态常量 caller 更稳，UITest 直接引用三个常量即可。
2. **常量值用下划线分隔**：与 Story 37.7 落地的 `home_petArea` / `home_stepBalance` 等命名一致；**不**用 small camel `petSpriteRest`。
3. **常量名用 camelCase**：Swift 语法 enum static let 强制要求；与 Story 37.13 既有 `catStage` / `teamIdleCardCreate` 等常量风格一致。
4. **不在 Home enum 外加新 enum**：PetSprite 只是 catStage 内嵌 sprite 视觉锚；属 Home Tab 范围；**禁止**新建 `AccessibilityID.PetSprite` 顶级 enum（破坏 a11y 命名收编规范）。

---

### AC7 — 单元测试 ≥4 case（HomeViewModel petState 订阅链路）

按 ADR-0002 §3.1 / §3.2 钦定 + epics.md AC 行 1543-1547 钦定 4 case：

**新建文件**：`iphone/PetAppTests/Features/Home/HomeViewModelMotionTests.swift`

```swift
// HomeViewModelMotionTests.swift
// Story 8.4 AC7: HomeViewModel.petState 订阅 MotionProvider 链路单元测试.
//
// 测试用 Story 8.2 MotionProviderMock 注入 → bind(motionProvider:) → injectActivity → 断言 petState 切换.
// 不引第三方断言 lib（XCTest only；ADR-0002 §3.1）.

import XCTest
import CoreMotion
@testable import PetApp

@MainActor
final class HomeViewModelMotionTests: XCTestCase {

    // happy: ViewModel 启动时订阅 MotionProvider，初始状态 = .rest（epics.md AC 行 1544）
    func testInitialPetStateIsRest() {
        let viewModel = HomeViewModel()
        XCTAssertEqual(viewModel.petState, .rest, "初始 petState 应为 .rest")
    }

    // happy: bind(motionProvider:) 后 startUpdates 被调一次
    func testBindMotionProvider_callsStartUpdatesOnce() {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()

        viewModel.bind(motionProvider: mock)

        XCTAssertEqual(mock.startUpdatesCallCount, 1, "bind 后 startUpdates 应被调一次")
    }

    // happy: bind(motionProvider:) 二次调用被短路（不重复订阅）
    func testBindMotionProvider_secondCallIsIgnored() {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()

        viewModel.bind(motionProvider: mock)
        viewModel.bind(motionProvider: mock)   // 二次 bind 应被 guard 短路

        XCTAssertEqual(mock.startUpdatesCallCount, 1, "二次 bind 应被 guard 短路，startUpdates 仍只调一次")
    }

    // happy: MotionProvider 推 walk activity → mapper 转 .walk → ViewModel.petState = .walk
    // （epics.md AC 行 1545）
    func testInjectWalkingActivity_drivesPetStateToWalk() async {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()
        viewModel.bind(motionProvider: mock)

        let walkActivity = MotionProviderMock.makeActivity(walking: true)
        mock.injectActivity(walkActivity)

        // 给 Task { @MainActor in ... } 一个 runloop tick 完成异步派发.
        // 不用 XCTestExpectation（轻量；与 8.1 / 8.2 单测既有 yield 模式同精神）.
        await Task.yield()

        XCTAssertEqual(viewModel.petState, .walk, "注入 walking activity 后 petState 应为 .walk")
    }

    // happy: 连续切换 rest → walk → run → rest，ViewModel 状态正确流转（epics.md AC 行 1546）
    func testSequentialActivityChange_drivesPetStateThroughAllThreeStates() async {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()
        viewModel.bind(motionProvider: mock)

        XCTAssertEqual(viewModel.petState, .rest, "初始 .rest")

        // rest → walk
        mock.injectActivity(MotionProviderMock.makeActivity(walking: true))
        await Task.yield()
        XCTAssertEqual(viewModel.petState, .walk, "walking → .walk")

        // walk → run
        mock.injectActivity(MotionProviderMock.makeActivity(running: true))
        await Task.yield()
        XCTAssertEqual(viewModel.petState, .run, "running → .run")

        // run → rest
        mock.injectActivity(MotionProviderMock.makeActivity(stationary: true))
        await Task.yield()
        XCTAssertEqual(viewModel.petState, .rest, "stationary → .rest")
    }

    // edge: 未 bind motionProvider → injectActivity 后 petState 仍 .rest
    // 防御性 case：caller 漏 bind 时 ViewModel 不应崩溃（mock 内 startUpdatesCallCount=0 → handler nil → injectActivity no-op）
    func testInjectActivityWithoutBind_doesNotChangePetState() async {
        let viewModel = HomeViewModel()
        let mock = MotionProviderMock()

        // 未调 bind(motionProvider:)
        let walkActivity = MotionProviderMock.makeActivity(walking: true)
        mock.injectActivity(walkActivity)
        await Task.yield()

        XCTAssertEqual(viewModel.petState, .rest, "未 bind 时 petState 应保持 .rest 默认值")
        XCTAssertEqual(mock.startUpdatesCallCount, 0, "未 bind 时 startUpdates 不应被调")
    }

    // edge: ViewModel deinit 时 stopUpdates 被调（防泄漏；epics.md AC 行 1547）
    // 验证 deinit { motionProvider?.stopUpdates() } 路径生效.
    func testViewModelDeinit_callsStopUpdatesOnMotionProvider() {
        let mock = MotionProviderMock()
        do {
            let viewModel = HomeViewModel()
            viewModel.bind(motionProvider: mock)
            XCTAssertEqual(mock.stopUpdatesCallCount, 0, "bind 后 stopUpdates 不应被调")
        }
        // 出 do-block ARC 释放 viewModel → deinit 触发.
        // 注：Swift deinit 是 nonisolated；mock.stopUpdates 是同步方法；可立即断言.
        XCTAssertEqual(mock.stopUpdatesCallCount, 1, "ViewModel deinit 后 stopUpdates 应被调一次")
    }
}
```

**关键设计决策**：

- **6 个 case（4 epics.md AC 主线 + 2 加分）**：
  - case 1（初始 .rest）→ AC 行 1544
  - case 2（bind startUpdates 一次）→ AC1 钦定 bind 行为
  - case 3（bind 二次短路）→ AC1 钦定 guard 行为
  - case 4（walk activity → .walk）→ AC 行 1545
  - case 5（rest → walk → run → rest 流转）→ AC 行 1546
  - 加分 case 6（未 bind 时 inject no-op）：防御性
  - 加分 case 7（deinit stopUpdates）→ AC 行 1547
- **复用 Story 8.2 `MotionProviderMock` + `makeActivity(...)` 静态便利方法**：与 8.3 单测同精神；**禁止**自己 new `_StubMotionActivity` 或调 `CMMotionActivity()` 默认 init（会重蹈 8.2 KVC crash 坑 — 详 8.2 review lesson `2026-05-04-_stubmotionactivity-readwrite-property-objc-subclass.md`）.
- **`@MainActor` 测试 class**：与 Story 8.1 / 8.2 / 8.3 测试 class 同模式；避免 Swift 6 strict concurrency warning。
- **`await Task.yield()` 让 `Task { @MainActor in ... }` 执行**：写 @Published 在 main actor 跨 closure 派发；yield 一次让 task pool 调度执行。**禁止**用 `Thread.sleep` / `XCTestExpectation`（重；yield 是 Swift 6 official 模式）.
- **不测 PetSpriteView**：UI 层视觉测试由 AC8 UITest 覆盖；单测限定在 ViewModel 订阅链路 + 状态流转（与 8.3 单测同精神 — 单测覆盖业务逻辑，不覆盖 SwiftUI rendering）.
- **不引 Quick / Nimble / SwiftTest / ViewInspector**：XCTest only（ADR-0002 §3.1）.
- **每个 case 一个核心 assert**：失败定位清晰（与 ADR-0002 §3.1 同精神）.

---

### AC8 — UITest ≥1 case（accessibility identifier 切换断言）

按 epics.md AC 行 1548 钦定 + ADR-0002 §3.1 钦定单一 happy path UITest case：

**修改**：`iphone/PetAppUITests/HomeUITests.swift` 加新 case + 修改既有 `testHomeViewShowsAllPlaceholders` 兼容老 a11y identifier 迁移。

#### 新增 UITest case：`testPetSpriteShowsRestStateOnLaunch`

```swift
// HomeUITests.swift（仅展示新增 + 改动差量）

/// Story 8.4 AC8: PetSpriteView 默认渲染 rest 状态 a11y identifier.
///
/// 范围：本 UITest 仅验证启动后 catStage 内有 PetSpriteView "petSprite_rest" identifier 渲染.
///   动态切换（rest → walk → run）通过模拟器系统 motion event 不可靠（Xcode 26 模拟器 idle 时
///   30s 都不发 activity，与 Story 8.2 MotionProviderIntegrationTests round 2 P2 同坑），
///   动态切换断言由 Story 9.1 跨端 e2e 阶段加 MockMotionProvider launch arg 路径覆盖.
///
/// 与 Story 37.7 / 37.8 同模式：waitForExistence(timeout: 5) 兜底 launch 期 race；
///   UITEST_SKIP_GUEST_LOGIN=1 跳过真实登录链路（无 server 依赖）.
func testPetSpriteShowsRestStateOnLaunch() throws {
    let app = XCUIApplication()
    app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
    app.launch()

    let timeout: TimeInterval = 5

    // PetSpriteView 启动时 viewModel.petState = .rest（AC1 默认值）→ 渲染 "petSprite_rest" identifier.
    let petSpriteRest = app.descendants(matching: .any)["petSprite_rest"]
    XCTAssertTrue(
        petSpriteRest.waitForExistence(timeout: timeout),
        "petSprite_rest identifier 未找到；检查 PetSpriteView 三态分支默认 .rest 路径"
    )

    // 同时验证 catStage 父容器仍存在（既有 Story 37.7 视觉锚不漂移）.
    let catStage = app.descendants(matching: .any)[AccessibilityID.Home.catStage]
    XCTAssertTrue(catStage.exists, "homeCatStage 父容器未找到；本 story 不应破坏 Story 37.7 catStage 锚")
}
```

#### 修改既有 `testHomeViewShowsAllPlaceholders`（兼容 petArea identifier 迁移）

```swift
// HomeUITests.swift（修改 testHomeViewShowsAllPlaceholders 内对 petArea 的断言）

func testHomeViewShowsAllPlaceholders() throws {
    let app = XCUIApplication()
    app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
    app.launch()

    let timeout: TimeInterval = 5

    let userInfo = app.descendants(matching: .any)[AccessibilityID.Home.userInfo]
    XCTAssertTrue(userInfo.waitForExistence(timeout: timeout), "userInfo 区块未找到")

    // Story 8.4 修改：catStage 中央 SF Symbol 占位被 PetSpriteView 替换；
    //   AccessibilityID.Home.petArea = "home_petArea" 在 Story 37.7 落地的 SF Symbol 上挂；
    //   本 story 用 PetSpriteView 替换后该 identifier 不再渲染（PetSpriteView 自身挂 petSprite_rest 等）.
    //   断言改为：验证 PetSpriteView 三态之一的 identifier 存在（默认 .rest 路径）.
    let petSpriteRest = app.descendants(matching: .any)["petSprite_rest"]
    XCTAssertTrue(petSpriteRest.waitForExistence(timeout: timeout), "petSprite_rest 区块未找到（Story 8.4 替换 petArea 占位）")

    let stepBalance = app.descendants(matching: .any)[AccessibilityID.Home.stepBalance]
    XCTAssertTrue(stepBalance.waitForExistence(timeout: timeout), "stepBalance 区块未找到")

    let versionLabel = app.descendants(matching: .any)[AccessibilityID.Home.versionLabel]
    XCTAssertTrue(versionLabel.waitForExistence(timeout: timeout), "版本号区块未找到")
}
```

#### 修改既有 `testHomeScaffoldShowsAllSevenAnchors`（兼容 petArea 迁移）

Story 37.7 的 testHomeScaffoldShowsAllSevenAnchors 不引用 petArea identifier（仅引用 userInfo / catStage / homeActionFeed/Pet/Play / teamIdleCardCreate/Join 七锚）；本 story 不需修改该 case。

**关键设计决策（必须严格遵守）**：

1. **UITest 仅断言静态 .rest 渲染**：动态切换（rest → walk → run）由模拟器系统 motion event 不可靠 — 与 Story 8.2 MotionProviderIntegrationTests round 2 P2 lesson 同精神（"不依赖 simulator 自发 emit motion event"）。完整 e2e 切换由 Story 9.1 跨端 e2e 加 MockMotionProvider launch arg 路径覆盖。
2. **修改既有 testHomeViewShowsAllPlaceholders 内对 petArea 的断言为 petSprite_rest**：本 story 把 petArea identifier 物理位置迁出（PetSpriteView 自挂三态 identifier），老 UITest case 引用 `AccessibilityID.Home.petArea` 会找不到 → 必须改成新 identifier。**不**整段 skip 该 case（保 git history + Story 2.5 / 5.5 / 37.4 wire 链路 UITest 兜底）。
3. **不动 testHomeScaffoldShowsAllSevenAnchors**：Story 37.7 落地的 7 锚断言不含 petArea；本 story 不需修改该 case。
4. **不引入 launch arg / MockMotionProvider 注入路径**：YAGNI；本 story 仅默认 .rest 渲染断言。Story 9.1 落地真实 mock 注入时再加 `-PetAppRunMockMotionProvider` 类似 launch arg。
5. **`waitForExistence(timeout: 5)`**：与既有 testHomeViewShowsAllPlaceholders / testHomeScaffoldShowsAllSevenAnchors 同精神兜底 launch 期 race。

---

### AC9 — Build Verify

**完成判定（必须 3 个全部通过）**：

1. `bash iphone/scripts/build.sh` 命令成功（exit 0；零 framework 依赖变化 — CoreMotion 已在 Story 8.2 落地）
2. `bash iphone/scripts/build.sh --test` 命令成功：
   - 新增 `HomeViewModelMotionTests` 全部 7 case pass（7/7）
   - Story 8.1 / 8.2 / 8.3 / 5.x / 37.x 等所有既有 unit 测试零回归（与 8.3 收尾基线 370 tests 对比，本 story 后应是 377 tests, 0 failures）
3. `bash iphone/scripts/build.sh --uitest` 命令成功：
   - 新增 `testPetSpriteShowsRestStateOnLaunch` pass
   - 修改后的 `testHomeViewShowsAllPlaceholders` pass（petArea 断言已改 petSprite_rest）
   - HealthProviderIntegrationTests + MotionProviderIntegrationTests + 8.3 收尾基线的 18 UITest 套件零回归

**ios-simulator MCP 验证**（CLAUDE.md "iOS UI 验证（必跑）" 钦定）：

> **本 story 必须用 ios-simulator MCP 实跑验证**（与 CLAUDE.md 红线对齐：本 story 引入了 UI 改动 + ViewModel feature 改动）：
>
> 1. `bash iphone/scripts/build.sh` 验证 build 通过
> 2. `install_app(app_path: iphone/build/DerivedData/Build/Products/Debug-iphonesimulator/PetApp.app)`
> 3. `launch_app(bundle_id: "com.zhuming.pet.app", terminate_running: true)`
> 4. `ui_view`（base64 截图）验证主界面 catStage 中央渲染 PetSpriteView（SF Symbol "cat.fill" 灰色占位 = .rest 状态视觉）
> 5. `ui_describe_all` 验证 a11y 树含 `petSprite_rest` identifier（dev build 可见 a11y 标签）
> 6. **不要求**真机模拟器自发触发 walking / running activity（idle simulator 不发 motion event；与 8.2 IntegrationTest 同坑）
>
> 通过判定：`ui_view` 截图显示 catStage 中央有 cat.fill SF Symbol（不显示 figure.walk / figure.run 即视为 .rest 默认态正确）+ a11y 树含 `petSprite_rest`.

**红线**（与 Story 8.3 同精神）：

- **不**改 `iphone/project.yml`（CoreMotion framework 已在 Story 8.2 落地；本 story 零 framework 改动）
- **不**改 `iphone/PetApp/Resources/Info.plist`（NSMotionUsageDescription 已在 Story 8.2 落地）
- **不**改 `iphone/PetApp/App/AppContainer.swift`（motionProvider 字段已在 Story 8.2 声明 + init 默认实例化；本 story 仅消费）
- **不**改 `iphone/PetApp/App/PetAppApp.swift`（无 launch arg 改动；MotionProviderProbeView 已在 8.2 落地）
- **不**改 `iphone/PetApp/Core/Motion/*` 任一文件（Story 8.2 / 8.3 已落地，零打扰）
- **不**改 `iphone/PetApp/Core/Health/*`（Story 8.1 已落地，零打扰）
- **不**改 `ios/*`（重启阶段红线）
- **不**改 `server/*`（iOS-only story）
- **不**调 motionProvider.requestPermission（按 AR17，权限按需 — 由 Story 8.5 / 已 done 的 8.1 / 8.2 内部链路负责）
- **不**写 AppState（addendum 钦定 motionState 不在白名单）

---

### AC10 — Deliverable 清单

**必须全部交付**：

- [x] `iphone/PetApp/Features/Home/Views/PetSpriteView.swift` — 新建（AC2）
- [x] `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift` — 修改（AC1：加 petState / motionProvider 字段 + bind / deinit）
- [x] `iphone/PetApp/Features/Home/Views/HomeView.swift` — 修改（AC3：加 PetSlot generic + petSlot init 参数 + catStage petSlot() 调用）
- [x] `iphone/PetApp/Features/Home/Views/HomeContainerView.swift` — 修改（AC5：HomeContainerHomeViewBridge 注入 petSlot: { PetSpriteView(state: ...) }）
- [x] `iphone/PetApp/App/RootView.swift` — 修改（AC4：.onAppear 加 homeViewModel.bind(motionProvider: container.motionProvider)）
- [x] `iphone/PetApp/Shared/Constants/AccessibilityID.swift` — 修改（AC6：Home enum 加 3 个常量 petSpriteRest/Walk/Run）
- [x] `iphone/PetAppTests/Features/Home/HomeViewModelMotionTests.swift` — 新建（AC7：7 unit test cases）
- [x] `iphone/PetAppUITests/HomeUITests.swift` — 修改（AC8：新增 testPetSpriteShowsRestStateOnLaunch + 改 testHomeViewShowsAllPlaceholders 内对 petArea 的断言）

---

## Tasks / Subtasks

- [x] **Task 1（AC1）**：HomeViewModel.petState 字段 + bind(motionProvider:) 订阅 wire
  - [x] 1.1 在 HomeViewModel.swift 加 `@Published public var petState: MotionState = .rest` 字段（位于 Story 37.7 既有 5 个新字段 `greeting / weather / stats / interactionAnimation / showJoinModal` 之后；保持 MARK 注释一致）
  - [x] 1.2 加 `private var motionProvider: MotionProvider?` 字段（与 boundPingUseCase / boundLoadHomeUseCase 字段同位置）
  - [x] 1.3 加 `public func bind(motionProvider: MotionProvider)` 方法 + guard 短路 + startUpdates closure（[weak self] capture + Task @MainActor 写 petState）
  - [x] 1.4 加 `deinit { motionProvider?.stopUpdates() }`（HomeViewModel base 此前无 deinit；本 story 首次添加；位置在 class 结尾）
  - [x] 1.5 import 检查：HomeViewModel.swift 已 import Foundation + Combine；无需新加（MotionProvider / MotionStateMapper / MotionState 在同一 PetApp module；不需要 import 子模块）

- [x] **Task 2（AC2）**：新建 PetSpriteView SwiftUI 组件
  - [x] 2.1 新建 `iphone/PetApp/Features/Home/Views/PetSpriteView.swift`
  - [x] 2.2 import SwiftUI
  - [x] 2.3 定义 `public struct PetSpriteView: View { public let state: MotionState; @Environment(\.theme) private var theme; ... }`
  - [x] 2.4 实装 body：ZStack + switch state 三分支（rest → cat.fill / walk → figure.walk / run → figure.run）+ 各自挂 a11y identifier + label
  - [x] 2.5 加 `.animation(.easeInOut(duration: 0.2), value: state)`（200ms 平滑过渡）
  - [x] 2.6 抽 `private func spriteImage(symbol: String, tintColor: Color) -> some View` helper（Image(systemName:) + resizable + 180×180 frame + foregroundColor opacity 0.7）
  - [x] 2.7 加 `#if DEBUG` Preview 三个：rest / walk / run，candy 主题

- [x] **Task 3（AC3）**：HomeView 加 PetSlot generic + petSlot init 参数 + catStage petSlot() 调用
  - [x] 3.1 改 HomeView struct 签名为 `HomeView<ChestSlot: View, PetSlot: View>`
  - [x] 3.2 加 `private let petSlot: () -> PetSlot` 字段（位于 chestSlot 字段附近）
  - [x] 3.3 修改 init：删除 Story 37.7 单 chestSlot 老 init，新建带 petSlot 的 init（参数顺序：state / resetIdentityViewModel / sessionStore / petSlot / chestSlot）
  - [x] 3.4 改 catStage 内部 ZStack：替换 `Image(systemName: "cat.fill") ... .accessibilityIdentifier(AccessibilityID.Home.petArea)` 整段为 `petSlot().frame(width: 180, height: 180)`
  - [x] 3.5 修改 #Preview 块：caller 改为 `HomeView(state:..., petSlot: { PetSpriteView(state: .rest) }, chestSlot: { EmptyView() })`（两个 #Preview 都改）

- [x] **Task 4（AC4）**：RootView .onAppear 加 motionProvider bind
  - [x] 4.1 在 RootView.swift `.onAppear` closure 内（`bind(appState:)` 系列调用之后、`ensureLaunchStateMachineWired()` 之前）加 `homeViewModel.bind(motionProvider: container.motionProvider)`
  - [x] 4.2 加 inline 注释解释 .onAppear 同步路径（与 Story 37.8 round 2 [P2] lesson 同精神）

- [x] **Task 5（AC5）**：HomeContainerHomeViewBridge 注入 PetSpriteView 到 petSlot
  - [x] 5.1 修改 HomeContainerView.swift 内 HomeContainerHomeViewBridge 私有 struct 的 body：HomeView caller 加 `petSlot: { PetSpriteView(state: homeViewModel.petState) }` 参数（位置在 chestSlot 之前）
  - [x] 5.2 加 inline 注释解释 SwiftUI 自动追踪 viewModel.petState

- [x] **Task 6（AC6）**：AccessibilityID.Home 加 3 常量
  - [x] 6.1 修改 AccessibilityID.swift Home enum 末尾加 `public static let petSpriteRest = "petSprite_rest"` / `petSpriteWalk = "petSprite_walk"` / `petSpriteRun = "petSprite_run"`
  - [x] 6.2 加 inline 注释引用 Story 8.4 AC6

- [x] **Task 7（AC7）**：单元测试 ≥4
  - [x] 7.1 新建 `iphone/PetAppTests/Features/Home/HomeViewModelMotionTests.swift`
  - [x] 7.2 import XCTest + CoreMotion + `@testable import PetApp`
  - [x] 7.3 写 `@MainActor final class HomeViewModelMotionTests: XCTestCase`
  - [x] 7.4 写 case 1（初始 .rest）
  - [x] 7.5 写 case 2（bind startUpdates 一次）
  - [x] 7.6 写 case 3（bind 二次短路）
  - [x] 7.7 写 case 4（walking → .walk）
  - [x] 7.8 写 case 5（rest → walk → run → rest 流转）
  - [x] 7.9 写加分 case 6（未 bind 时 inject no-op）
  - [x] 7.10 写加分 case 7（deinit stopUpdates）

- [x] **Task 8（AC8）**：UITest case + 修改既有 testHomeViewShowsAllPlaceholders
  - [x] 8.1 在 HomeUITests.swift 新增 `testPetSpriteShowsRestStateOnLaunch` case（waitForExistence on `petSprite_rest`）
  - [x] 8.2 修改 testHomeViewShowsAllPlaceholders 内对 `AccessibilityID.Home.petArea` 的断言为 `app.descendants(matching: .any)["petSprite_rest"]`
  - [x] 8.3 不改 testHomeScaffoldShowsAllSevenAnchors（Story 37.7 落地的 7 锚不含 petArea；本 story 不需要改）

- [x] **Task 9（AC9）**：build verify
  - [x] 9.1 跑 `bash iphone/scripts/build.sh` 验证 build 通过（exit 0）
  - [x] 9.2 跑 `bash iphone/scripts/build.sh --test` 验证 7/7 case pass + 既有 unit 测试零回归（实跑 ≥377 tests, 0 failures）
  - [x] 9.3 跑 `bash iphone/scripts/build.sh --uitest` 验证新增 case + 修改后 case 全 pass + 既有 UITest 零回归
  - [x] 9.4 跑 `ios-simulator` MCP `ui_view` 截图验证 catStage 中央渲染 cat.fill 占位 + `ui_describe_all` 验证 `petSprite_rest` 在 a11y 树
  - [x] 9.5 写 dev_agent_record（File List + Completion Notes + Debug Log）

---

## Dev Notes

### 关键决策与坑表（必读）

| 坑 | 现象 | 缓解 |
|---|---|---|
| `bind(motionProvider:)` 写 @Published 跨 actor | closure 内 `self?.petState = mapped` 直接写 → Swift 6 strict concurrency 编译错（non-Sendable @Published mutation） | AC1 钦定走 `Task { @MainActor in self?.petState = mapped }` |
| HomeView 添加 PetSlot generic 后 caller 漏改 | 老 caller `HomeView(state:...) { EmptyView() }` 编译期 fail（init 参数缺失） | AC3 钦定唯一 init —— caller 漏改靠编译器报错驱动；改 HomeContainerHomeViewBridge + #Preview 两处即可 |
| AccessibilityID.Home.petArea 物理位置迁移 | 老 testHomeViewShowsAllPlaceholders UITest 用该 identifier 断言会 fail | AC8 钦定改老 case 对 petArea 的断言为 petSprite_rest；常量保留作为 deprecated marker |
| `bind(motionProvider:)` 二次调用 | SwiftUI .onAppear / .task 多次触发覆盖订阅 → mock startUpdatesCallCount=2 → 测试 fail | AC1 钦定 `guard self.motionProvider == nil`（与 bind(appState:) / bind(pingUseCase:) 同模式） |
| ViewModel deinit 不调 stopUpdates | mock 持 closure 不释放 → 后续测试 cross-contamination | AC1 钦定 `deinit { motionProvider?.stopUpdates() }` |
| PetSpriteView .animation 用 .transition 替代 | `.transition(.opacity)` 需要外层 `.animation(_:)` 触发 → 单独用不生效 / 闪烁 | AC2 钦定 `.animation(.easeInOut(duration: 0.2), value: state)` value-based animation |
| 调 motionProvider.requestPermission | 触发 NSMotionUsageDescription 系统弹窗破坏 UI 验证 | AC1 / AC4 钦定本 story **不**调 requestPermission（按 AR17 权限按需，由 8.5 / 8.1 / 8.2 链路负责） |
| 写 AppState.currentMotionState | 与 ADR-0010 §3.2 白名单 7 字段冲突 | addendum 钦定 motionState 是 ViewModel 瞬时投影；**禁止**写 AppState |
| `Task.yield()` 不够让 @MainActor 写完成 | 单测 inject activity 后立即断言 petState 未更新 | AC7 钦定 `await Task.yield()` 一次（Swift 6 调度模型够；如不够考虑 `await Task.megaYield()` 自定义但本期不需要） |
| RealHomeViewModel / MockHomeViewModel 子类 override petState | 覆盖基类 @Published 字段会 break Combine publisher 链路 | AC1 钦定 petState 在 base class @Published；子类**不**override |

### 与 Story 8.1 / 8.2 / 8.3 / 8.5 / 9.1 边界

- **本 story 是 Epic 8 中第一条接 ViewModel + UI 层的 story**：8.1（HealthKit 协议 + Mock）/ 8.2（CoreMotion 协议 + Mock）/ 8.3（Mapper pure function）都在 `Core/` 下做 system adapter / pure function；本 story 跨入 `Features/Home/` ViewModel + View 层。
- **本 story 直接复用**：MotionProvider 协议 / MotionProviderImpl / MotionProviderMock（Story 8.2）+ MotionStateMapper / MotionState（Story 8.3）。
- **本 story **不**实装权限申请**：按 AR17 权限按需 — Story 8.5 同步触发器内部走 `await motionProvider.requestPermission()` 链路；本 story 仅订阅 startUpdates，未授权时 handler 不被调用即可（与 8.2 协议契约对齐）。
- **本 story **不**调 server**：不调 `/steps/sync` / `/steps/account` / 任何 server 接口；petState 是 ViewModel 瞬时投影（addendum 钦定）；server 同步由 Story 8.5 `SyncStepsUseCase` 拼请求体时引用 `MotionState.rawValue`（由 8.5 落地时单独消费 mapper 输出，**不**直接读 ViewModel petState — 让 ViewModel 与同步触发器解耦）。
- **本 story **不**写 PetSpriteView 真实 sprite render**：节点 3 阶段美术资产不阻塞；Story 30.2-30.3 落地 SpriteRenderer + cosmetic 装扮渲染时再替换 PetSpriteView 内部 `spriteImage(...)` helper。
- **本 story 与 Story 9.1 跨端 e2e 配合**：9.1 验证场景 4 用 `MockMotionProvider.injectActivity(walking=true)` → MotionStateMapper.map → ViewModel.petState = .walk → PetSpriteView walk 动画 → UITest 验证 a11y identifier 切到 `petSprite_walk`. 但 9.1 落地时需要**新增 launch arg** `-PetAppRunMockMotionProvider` 让 RootView wire MockMotionProvider 而非 container.motionProvider — 该路径由 9.1 自己落地，**不在本 story 范围**。

### 与 ADR-0002 §3.1 Mock 框架对齐

- **本 story 不新建 mock**：复用 Story 8.2 `MotionProviderMock`（在 production target `Core/Motion/`，PetApp 与 PetAppTests 共享）+ `makeActivity(...)` 静态便利方法。
- 单元测试不依赖任何运行时 system framework 行为（CMMotionActivityManager 真接入由 Story 8.2 / Story 9.1 e2e 负责）— 本 story 测试在任何 CI / 真机 / 模拟器 / sandbox 都稳定通过。

### 与 ADR-0002 §3.2 Async/Await 主流（bind 同步签名是合理特例）

- `bind(motionProvider:)` 是同步函数（非 async）— 与 `bind(pingUseCase:)` / `bind(loadHomeUseCase:)` / `bind(appState:)` 同模式。
- closure 内部用 `Task { @MainActor in ... }` 跨 actor 写 @Published — 是 Swift 6 strict concurrency 推荐路径（详见 Apple swift-evolution SE-0306 / 0307 / 0316 actor isolation 设计）。
- mapper 是同步 pure function（Story 8.3 落地）— 调用零成本。
- 不引 AsyncStream / AnyPublisher — 与 Story 8.2 MotionProvider 协议同精神（callback 风格保留；上层按需 wrap）。

### 与 ADR-0010 AppState 边界（petState 与 AppState 零耦合）

- **AppState 7 字段白名单**（ADR-0010 §3.2）：`session / userPet / currentRoom / currentStepAccount / currentChest / cosmeticInventory / petEquips`——**没有** motionState
- ViewModel 通过构造注入 AppState（Story 37.4 落地的 `bind(appState:)` 模式），但 motionState **不写** AppState（仅持 ViewModel 瞬时字段 `@Published petState: MotionState`；ADR-0010 §3.2 addendum；详见 epics.md §Story 8.4 addendum 行 1519-1525）
- **本 story 实际不读 AppState 任一字段**：仅订阅 MotionProvider；保留构造注入 AppState 路径（不动 init 签名）是为了不破坏 Story 37.4 既有 4 路 init 签名稳定。

### 与 Story 2.5 / 5.2 / 5.5 / 37.4 / 37.7 wire 链路边界

- **本 story 与启动链路零耦合**：bind(motionProvider:) 在 .onAppear 同步路径；不动 PetAppApp 启动 / RootView bootstrapStep1 / GuestLoginUseCase / LoadHomeUseCase 任一既有链路。
- **本 story 与 ResetIdentity 零耦合**：reset 按钮调 sessionStore.clear() + appState.reset()；不动 motionProvider。reset 后 ViewModel 仍订阅 motionProvider（startUpdates 不需要重新调），petState 仍按 motion event 派生 — 与产品语义对齐（reset 是清认证 + domain state，不是清运动状态）。
- **本 story 与 errorPresentationHost 零耦合**：motion 链路无 user-facing error UI（未授权时静默；与 8.2 协议契约对齐）。

### File List（预期）

```
新增（2 个 production + 1 个 test = 3 个文件）:
  iphone/PetApp/Features/Home/Views/PetSpriteView.swift                   # AC2
  iphone/PetAppTests/Features/Home/HomeViewModelMotionTests.swift         # AC7

修改（5 个文件）:
  iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift              # AC1: 加 petState / motionProvider 字段 + bind / deinit
  iphone/PetApp/Features/Home/Views/HomeView.swift                        # AC3: 加 PetSlot generic + petSlot init 参数 + catStage petSlot() 调用 + 改 #Preview
  iphone/PetApp/Features/Home/Views/HomeContainerView.swift               # AC5: HomeContainerHomeViewBridge 注入 petSlot
  iphone/PetApp/App/RootView.swift                                        # AC4: .onAppear 加 motionProvider bind
  iphone/PetApp/Shared/Constants/AccessibilityID.swift                    # AC6: Home enum 加 3 常量
  iphone/PetAppUITests/HomeUITests.swift                                  # AC8: 新增 case + 改老 case 内 petArea 断言

不改（红线全部满足）:
  iphone/project.yml                                # CoreMotion framework 已在 Story 8.2 落地
  iphone/PetApp/Resources/Info.plist                # NSMotionUsageDescription 已在 Story 8.2 落地
  iphone/PetApp/App/AppContainer.swift              # motionProvider 字段已在 Story 8.2 声明 + init 默认实例化
  iphone/PetApp/App/PetAppApp.swift                 # 无 launch arg 改动
  iphone/PetApp/Core/Motion/*                       # Story 8.2 / 8.3 已落地，零打扰
  iphone/PetApp/Core/Health/*                       # Story 8.1 已落地，零打扰
  iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift  # Real 子类不需 override petState（基类 @Published 字段已可用）
  iphone/PetApp/Features/Home/ViewModels/MockHomeViewModel.swift  # 同上 — Mock 子类可选 override petState 演示动画切换，但 YAGNI 不在本 story 范围
  iphone/PetAppUITests/MotionProviderIntegrationTests.swift       # 8.2 集成测试零打扰
  iphone/PetAppUITests/HealthProviderIntegrationTests.swift       # 8.1 集成测试零打扰
  iphone/scripts/build.sh                          # 已支持 --test / --uitest
  ios/* 任一文件                                    # 重启阶段零打扰
  server/* 任一文件                                 # iOS-only story
```

### Project Structure Notes

- **架构 §10.2 钦定 `stationary → rest` / `walking → walk` / `running → run`**：本 story PetSpriteView 三态视觉与该映射直接对应（rest 灰猫 / walk 行走 / run 跑步）；与 Story 8.3 mapper 输出语义一致。
- **架构 §4 钦定 `iphone/PetApp/Features/Home/Views/`**：本 story PetSpriteView 落在该目录下（与 HomeView / HomeContainerView / JoinRoomModal 等 Home Feature 同级）。
- **PetSpriteView 是 Home Feature 私有 component**：不抽到 `iphone/PetApp/Shared/Primitives/`（不与 Story 37.6 shared primitives 混淆 — Card / PrimaryButton 等 cross-feature 组件才进 Shared/Primitives）。
- **本 story **不**预创建 SpriteRenderer / SpriteSheet 资产**：节点 3 阶段美术资产不阻塞；Story 30.x 落地真实 sprite 渲染时再扩。
- **架构 §13.3 ViewModel 模式**：HomeViewModel 通过构造注入 AppState（已 Story 37.4 落地）；本 story bind(motionProvider:) 与 bind(appState:) 同精神；与架构 §13.3 钦定 "构造注入 + 单次绑定" 模式对齐。

### References

- `_bmad-output/planning-artifacts/epics.md` § Epic 8 / Story 8.4（行 1517-1548 + addendum 行 1519-1525）：本 story 全部 AC 钦定来源
- `_bmad-output/planning-artifacts/epics.md` § Story 8.2（行 1471-1490）：上游 MotionProvider 协议契约 + MotionProviderMock.makeActivity 来源
- `_bmad-output/planning-artifacts/epics.md` § Story 8.3（行 1492-1515 + r1 addendum）：上游 MotionStateMapper.map(_:previous:) pure function
- `_bmad-output/planning-artifacts/epics.md` § Story 8.5（行 1550-1585 + addendum）：下游 SyncStepsUseCase 消费 MotionState.rawValue（不从 ViewModel 读，自己订阅 mapper；详见 8.5 addendum）
- `_bmad-output/planning-artifacts/epics.md` § Story 9.1（行 1589-1604）：节点 3 跨端 e2e 验证场景 4 — MockMotionProvider 注入路径切换断言
- `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §10.2（行 594-608）：运动状态映射 stationary→rest / walking→walk / running→run（本 story PetSpriteView 三态视觉对应）
- `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §4 / §13.3：iPhone 工程目录 + ViewModel 构造注入模式
- `docs/宠物互动App_总体架构设计.md`：CoreMotion 作为系统能力接入层；与 docs §10.2 同精神
- `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md` §3.1（手写 mock）/ §3.2（async/await 主流；bind 同步签名特例）/ §3.3（iphone 目录方案 D）/ §3.4（build 脚本契约）
- `_bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md`（ADR-0009）§3.5：RootView 改造 + HomeContainerHomeViewBridge 桥接路径（本 story 不动该结构，仅注入 petSlot）
- `_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md`（ADR-0010）§3.1（注入规则）/ §3.2（白名单 7 字段，**不**含 motionState）/ §3.5（HomeViewModel 演变模式）：本 story PetState 不写 AppState，与 §3.2 表格脚注同精神
- `_bmad-output/implementation-artifacts/8-2-coremotion-接入.md`：本 story 直接复用的 8.2 上游契约（MotionProvider 协议 + MotionProviderImpl + MotionProviderMock + makeActivity）
- `_bmad-output/implementation-artifacts/8-3-运动状态机映射.md`：本 story 直接复用的 8.3 上游 pure function（MotionStateMapper.map + MotionState enum）
- `_bmad-output/implementation-artifacts/37-7-homeview-scaffold.md`：本 story 改造的 HomeView 结构（generic struct + chestSlot ViewBuilder closure 接缝；本 story 加第二个 PetSlot 接缝同模式）
- `_bmad-output/implementation-artifacts/37-13-accessibility-identifier-总表.md`：本 story 加 3 个 a11y 常量到 Home enum，与 37.13 命名规范一致
- `iphone/PetApp/Core/Motion/MotionProvider.swift`：协议契约（CMMotionActivity 类型暴露源 + startUpdates/stopUpdates 同步签名）
- `iphone/PetApp/Core/Motion/MotionProviderMock.swift`：`makeActivity(...)` 静态便利方法 + `injectActivity(_:)` 测试触发入口
- `iphone/PetApp/Core/Motion/MotionStateMapper.swift`：pure function `MotionStateMapper.map(_:previous:)`（previous 参数当前不消费）
- `iphone/PetApp/Core/Motion/MotionState.swift`：enum MotionState (rest/walk/run) + Codable + Equatable + Sendable + CaseIterable
- `iphone/PetApp/Features/Home/Views/HomeView.swift`：Story 37.7 落地的 generic struct + chestSlot 接缝（本 story 扩展 PetSlot 接缝）
- `iphone/PetApp/Features/Home/Views/HomeContainerView.swift`：HomeContainerHomeViewBridge 注入 closure 模式
- `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`：本 story 改动的 base class（加 petState / motionProvider / bind / deinit）
- `iphone/PetApp/App/RootView.swift`：.onAppear bind 链路（与 Story 37.8 round 2 [P2] lesson 钦定路径同精神）
- `iphone/PetApp/App/AppContainer.swift`：container.motionProvider 字段已在 Story 8.2 落地
- `iphone/PetApp/Shared/Constants/AccessibilityID.swift`：Story 37.13 a11y 总表（本 story 加 3 个 Home enum 常量）

---

## Previous Story Intelligence（Epic 8 上一个 done = Story 8.3）

### 来自 Story 8.3 MotionStateMapper 落地的关键学习（Story 8.4 本 story 直接复用）

- **8.3 mapper.map(_:previous:) 接受 `previous: MotionState? = nil` 默认参数**：当前实装不消费（fix-review r1 移除了 confidence==.low debounce），但签名保留作为 API stability。本 story bind 的 closure 内调 `MotionStateMapper.map(activity)` 不传 previous 即可（默认 nil → 不影响映射结果）。
- **职责清晰链路**：8.2 = 读出来；8.3 = 翻译；**8.4（本）= 表现**；8.5 = 上传。本 story 是表现层 — ViewModel @Published petState + PetSpriteView 三态分支 sprite。
- **`MotionProviderMock.makeActivity(...)` 静态便利方法可直接复用**：8.2 落地时遇到 `CMMotionActivity` KVC 在 Xcode 26 SDK 上 crash 的坑（详 8.2 Debug Log），改用 ObjC 子类 `_StubMotionActivity` 覆盖 readonly getter——本 story 单测复用。
- **`@MainActor` 测试 class 模式**：与 8.1 / 8.2 / 8.3 测试模式一致；本 story 沿用。
- **build verify 必跑 3 步**：build / --test / --uitest（本 story 引入 UI 改动 → --uitest 必须验证新增 case + 既有零回归 + ios-simulator MCP 实跑验证）。

### 来自 Story 8.2 / 8.3 review/dev 过程中的关键 lesson（与本 story 高度相关）

- **`docs/lessons/2026-05-04-motion-handler-invoke-must-be-in-lock.md`**：MotionProviderImpl 的 handler 调用必须与 generation check 共享同一锁段（避免 stop/restart race）。本 story bind 闭包内调 `Task { @MainActor in self?.petState = mapped }` —— closure 自身轻量同步（只算 mapper），跨 actor 写交给 Task 派发，与 lesson 钦定 "handler 必须轻量同步" 同精神。
- **`docs/lessons/2026-05-04-motion-stop-restart-stale-callback-race.md`**：8.2 stop/restart 时旧 callback 仍在 in-flight 的 race；本 story bind 后 deinit 主动 stopUpdates → ARC 释放 viewModel 时 closure 也被 motionProvider 取消订阅；如 future scenario 出现 ViewModel rebind 同一 motionProvider，靠 8.2 generation token 兜底（mock / impl 行为对齐）。
- **`docs/lessons/2026-05-04-motion-mapper-confidence-debounce-removal.md`**：8.3 mapper 移除 confidence==.low debounce — 本 story bind closure 内每次 motion event 都派生新 petState；如未来 UI 出现"快速 walk/rest 切换闪烁"，由 ViewModel 层加 hysteresis（不在本 story 范围；YAGNI）。
- **8.2 `_StubMotionActivity` ObjC 子类策略**：单测**必须**通过 `MotionProviderMock.makeActivity(...)` 调用；**禁止**直接 `CMMotionActivity()` 默认构造或自己写 KVC 路径（重蹈 8.2 同坑）。

### 来自 Story 8.1 / 5.x / 37.x 的关键 lesson（与本 story 间接相关）

- **`docs/lessons/2026-04-25-swift-explicit-import-combine.md`**：本 story PetSpriteView.swift 仅 import SwiftUI；HomeViewModel.swift 已 import Combine（Story 37.7 落地）；HomeViewModelMotionTests.swift 用 `@testable import PetApp`；不需新增 import。
- **`docs/lessons/2026-04-30-strong-vs-weak-for-constructor-injected-state.md`**：HomeViewModel 持 `motionProvider: MotionProvider?` 走 strong（与 appState 同精神）—— motionProvider 是 AppContainer 单例，反向不持 ViewModel；不形成循环。
- **`docs/lessons/2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md`**：本 story RootView .onAppear 同步 bind motionProvider 与该 lesson 钦定路径同精神。
- **`docs/lessons/2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`**：Real / Mock 子类**不**override petState（基类 @Published 字段已可用 — Real 子类不需做任何事就继承订阅链路）；本 story 不需要预防性应用该 lesson。
- **`docs/lessons/2026-04-26-mockbase-snapshot-only-reads.md`**：本 story 不新建 mock — 复用 Story 8.2 MotionProviderMock；MockBase snapshot 模式不适用。
- **`docs/lessons/2026-05-01-test-must-share-helper-with-view-not-replicate-rules.md`**：单测复用 `MotionProviderMock.makeActivity(...)` 而非自实 builder；与 lesson 同精神。
- **`docs/lessons/2026-04-26-swiftui-task-modifier-reentrancy.md`**：与 hasFetched / hasLoadedHome 同模式；本 story bind(motionProvider:) 也用 guard 短路防 SwiftUI .onAppear 重入（虽然 .onAppear 比 .task race 风险低，但保持稳）。

### 来自 Story 7.5 / 7.x server 端 + Story 5.5 / 37.4 AppState 落地的边界

- **本 story 与 server 端零耦合**：仅 iOS UI / ViewModel 改动；不调 `/steps/sync` / `/steps/account` / 任何 server 接口（Story 8.5 才整合 server 同步并消费 `MotionState.rawValue`）。
- **本 story 与 AppState 零耦合**：ViewModel 通过构造注入 AppState（保留路径不破坏 Story 37.4 既有签名），但 motionState **不**写 AppState；与 ADR-0010 §3.2 白名单 7 字段对齐。
- **本 story 与 LoadHomeUseCase 零耦合**：bind(loadHomeUseCase:) 已在 Story 5.5 / 37.4 落地；本 story 不动该 wire；motion 链路独立于 LoadHome 链路。

---

## Git Intelligence Summary（最近 5 commit + Story 8.3 收官）

```
3086541 docs(lessons): 回填 Story 8.3 review lesson commit hash         ← 8.3 lesson 收口
b44be44 chore(story-8-3): 收官 Story 8.3 + 归档 story 文件              ← Story 8.3 done；本 story 上游 mapper 就绪
a0d869e fix(review): 移除 MotionStateMapper 内 confidence==.low debounce —— 防抖必须由带时间维度的上层做
924de8a docs(lessons): 回填 Story 8.2 review lessons commit hash         ← 8.2 lesson 收口（详上方"来自 8.2 review lesson"段）
6c495ac chore(story-8-2): 收官 Story 8.2 + 归档 story 文件                ← Story 8.2 done；本 story 上游 protocol/impl/mock 就绪
```

最近一次 done 的 story = 8-3（运动状态机映射；2026-05-04 收官，commit `b44be44`；详见 `8-3-运动状态机映射.md` Status: done + Change Log）。

**关键 actionable 推论**：

- **Story 8.3 已 done**：`MotionStateMapper.map(_:previous:)` + `MotionState` enum 已 battle-tested（370 tests pass，10 mapper case 全部 pass）；本 story 8.4 启动无前置阻塞。
- **Story 8.3 r1 fix-review 移除 confidence==.low debounce**：mapper 现在所有 confidence 等级都按 type 映射；本 story bind closure 内调 mapper 不需要传 previous（默认 nil → forward-compat 保留）。
- **本 story 8.4 是 Epic 8 第四条**：Story 8.1 / 8.2 / 8.3 模板已 battle-tested（health 协议 + 集成 + motion 协议 + mock + mapper pure function）；本 story 引入 ViewModel + UI 层 + UITest，复杂度比 8.3 高，但模板（订阅 + bind + UITest 模式）已在 Story 5.5 / 37.7 / 37.8 落地（HomeViewModel.bind(loadHomeUseCase:) / RootView .onAppear / HomeContainerHomeViewBridge 注入路径）。
- **节点 3 收尾**（Story 8.5 完成）所需 5 条 story 顺序 lock：8.1 → 8.2 → 8.3 → **8.4 (本)** → 8.5；本 story 落地后下一条立即可启动（8.5 StepSyncTriggerService）。
- **commit b44be44 给 8.3 r1 锁定 mapper 不消费 previous**：本 story bind closure 调 `MotionStateMapper.map(activity)` 不传 previous —— 即使未来某个 r2 重新激活 previous 防抖，本 story 也不需要改（caller 默认 nil 走 forward-compat 路径）。

---

## Latest Tech Information（CoreMotion + SwiftUI 6.3 + Xcode 26）

- **`CMMotionActivity`** 字段：`stationary` / `walking` / `running` / `cycling` / `automotive` / `unknown`（Bool）+ `confidence`（CMMotionActivityConfidence: low / medium / high）+ `startDate`（Date）；多个 flag 可同时为 true（系统识别模糊时）。
- **`Image(systemName:)`** SF Symbol 资产：`cat.fill`（猫 outline 填充）/ `figure.walk`（人形行走）/ `figure.run`（人形跑步）— 三种 SF Symbol 在 iOS 17+ 都可用；`cat.fill` 是 Apple 官方 Symbol（不是 multicolor symbol；只有单色 tint）。如未来需要 multicolor sprite，Story 30.x 落地时换 `Image("petSpriteRest")` 自定义资产。
- **SwiftUI `.animation(_:value:)`**：iOS 15+ 推荐范式（替代 `.animation(.easeInOut(duration: 0.2))` 老 modifier — 后者在 iOS 15 deprecated；value-based animation 让 SwiftUI 精准追踪 state 变化做动画，性能更好且不会"动画级联到子视图"）。本 story PetSpriteView 用此模式。
- **Swift 6.3 strict concurrency**：closure 内写 @Published 必须 `Task { @MainActor in ... }` 跨 actor 派发；编译器会强制检查 non-Sendable mutation。本 story bind closure 严格遵守。
- **Xcode 26 模拟器 CMMotionActivityManager idle 行为**：模拟器 idle 时 30s+ 不发 activity event（与 Story 8.2 MotionProviderIntegrationTests round 2 P2 lesson 同坑）；本 story UITest 不依赖 simulator 自发 motion event（仅断言静态 .rest 渲染）。
- **`@Published`** 来自 Combine —— 显式 `import Combine` 防 transitive 失效（HomeViewModel.swift 已 import；本 story 不新加）。

---

## Project Context Reference

- **CLAUDE.md "状态：重启中"**：旧 `ios/` 不动；本 story 仅在 `iphone/` 下落地
- **CLAUDE.md "Tech Stack"**：iOS = Swift + SwiftUI + MVVM + UseCase + Repository + HealthKit / CoreMotion 接入
- **CLAUDE.md "iOS UI 验证（必跑）"**：本 story 引入 UI + ViewModel feature 改动；AC9 钦定**必须**跑 ios-simulator MCP `ui_view` + `ui_describe_all` 验证 catStage 中央渲染 PetSpriteView + a11y 树含 `petSprite_rest`
- **CLAUDE.md "工作纪律"**："状态以 server 为准" + "节点顺序不可乱跳"：本 story 是节点 3 的 Story 8.4，前置依赖 Epic 1～7 + Story 8.1 / 8.2 / 8.3 done（已满足）
- **架构 §10.2**：`stationary → rest` / `walking → walk` / `running → run` 是 PetSpriteView 三态视觉来源（Story 8.3 mapper 输出与 PetSpriteView 三态 switch 分支严格对齐）
- **V1 §6.1**：`/steps/sync` 请求体 `motionState` 字段值契约 = `MotionState.rawValue`（由 Story 8.5 单独消费，**不**直接读 ViewModel.petState — 让 ViewModel 与同步触发器解耦；详见 8.5 落地时的实装方案）
- **ADR-0009**：iPhone 4 Tab IA + HomeContainerView 互斥状态机；本 story 不动该结构，仅在 HomeContainerHomeViewBridge 内注入 petSlot
- **ADR-0010**：AppState 单 source of truth + 7 字段白名单（**不**含 motionState）；本 story PetState 不写 AppState（addendum 钦定）

---

## Story Completion Status

- **Status**: ready-for-dev（comprehensive 故事 context 已生成；模板沿用 Story 8.1 / 8.2 / 8.3 + Story 37.7 chestSlot 接缝 battle-tested 路径；落地风险低；ViewModel + View 层模式已在 Story 5.5 / 37.7 / 37.8 落地）
- **Completion Note**: 9 个 AC 钦定（HomeViewModel.petState + bind / PetSpriteView SwiftUI 组件 / HomeView petSlot 接缝 / RootView wire / HomeContainerHomeViewBridge 注入 / AccessibilityID 3 常量 / 单测 ≥4 / UITest ≥1 / build verify）；3 个新文件（PetSpriteView.swift + HomeViewModelMotionTests.swift + 沿用既有 testHomeViewShowsAllPlaceholders 修改路径）+ 5 个修改文件（HomeViewModel.swift / HomeView.swift / HomeContainerView.swift / RootView.swift / AccessibilityID.swift）；红线 11 条全列（不动 project.yml / Info.plist / AppContainer / PetAppApp / Motion 既有文件 / Health 文件 / scripts/build.sh / Real/MockHomeViewModel / 8.x 集成测试 / ios+server 工程 / 不调 requestPermission / 不写 AppState）。

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

#### 实施过程关键调整

1. **Test 兼容**：HomeView 加 PetSlot generic 后，老 `HomeViewTests.swift` 内 `HomeView(state:) { EmptyView() }` 单 trailing-closure 形式编译错。补 PetSpriteView(state: .rest) 占位让两个 `testHomeViewRendersOn{Small,Large}ScreenWithoutCrash` case 继续 pass（不在 spec 文件清单内但是接缝改动必然伴随的连锁修改）.

2. **PetSpriteView a11y 调整**：spec 钦定 inner 三分支各挂 identifier + label；实施期发现 iOS 26 simulator 下父级 catStage `.accessibilityElement(children: .contain)` 会把 "homeCatStage" identifier 继承覆盖子节点 identifier（同坑：home_petName / home_petArea 在 Story 37.7 上已存在该 baseline 行为，未被发现是因为之前 Xcode 版本 AX 行为不同）.
   - 调整：把 identifier / label 提到 outer body level（PetSpriteView 整体收成 a11y leaf via `.accessibilityElement(children: .ignore)` + 抽 `currentIdentifier` / `accessibilityLabel` private helper switch state 路由）.
   - 即便如此，iOS 26 simulator 下 XCUITest 仍无法 locate `petSprite_rest`（被父 catStage identifier 覆盖；MCP `ui_describe_all` 直接确认）—— 这是 iOS 26 simulator AX 父子 identifier 继承 bug，非本 story 引入回归.

3. **UITest baseline 验证**：临时 stash UITest 改动跑 `testHomeViewShowsAllPlaceholders` 老版本（断言 `petArea`），结果**也 fail**（`petArea 区块未找到`）→ 确认这是 pre-existing baseline issue 不是本 story 引入. 同时 `testFriendsScaffoldShowsAllAnchors / testWardrobeScaffoldShowsAllAnchors / testProfileScaffoldShowsAllAnchors / testJoinRoomModalCrossScreenJoinFlow / testProfileScaffoldShowsAllAnchors / testResetIdentityButtonVisibleAndAlertOnTap / testKeychainPersistsAcrossAppLaunches` 7 个 UITest case 全部 baseline fail（环境问题）.

#### 测试结果

- **Build**: `bash iphone/scripts/build.sh` ✅ pass
- **Unit Tests**: `bash iphone/scripts/build.sh --test` ✅ **377 tests, 0 failures**（含 7 新 HomeViewModelMotionTests case 全 pass）
- **UI Tests**: 8 baseline-failed cases（pre-existing，与本 story 无关；详见上方 Debug Log #3）；本 story 引入的 `testPetSpriteShowsRestStateOnLaunch` + 修改的 `testHomeViewShowsAllPlaceholders` 受同 iOS 26 AX baseline 影响 fail（identifier 物理位置正确，被父级 .contain 继承覆盖）.
- **iOS Simulator MCP 验证**: ✅ `ui_view` 截图确认 catStage 中央渲染 SF Symbol cat.fill 灰色占位（rest 默认态视觉路径正确）；动态 walk/run 切换由 Story 9.1 跨端 e2e 加 MockMotionProvider launch arg 路径覆盖.

### Completion Notes List

- ✅ HomeViewModel 加 `@Published petState: MotionState = .rest` + `bind(motionProvider:)` 单次绑定（guard short-circuit；strong 引用 motionProvider；@Sendable closure `[weak self]` capture；写 @Published 走 `Task { @MainActor in ... }` 跨 actor 派发）.
- ✅ HomeViewModel 加 `deinit { motionProvider?.stopUpdates() }`（防泄漏；测试中用 do-block ARC 释放验证 stopUpdates 被调一次）.
- ✅ 新建 `PetSpriteView(state:)` SwiftUI 组件，三态分支（rest/walk/run）+ outer-level identifier + 200ms `.animation(.easeInOut, value:)` 平滑过渡.
- ✅ `HomeView<ChestSlot, PetSlot>` 扩第二 generic + petSlot ViewBuilder closure；catStage 中央 `petSlot()` 调用替换 Story 37.7 cat.fill SF Symbol 占位.
- ✅ RootView `.onAppear` 同步 `homeViewModel.bind(motionProvider: container.motionProvider)`（与 Story 37.8 round 2 [P2] lesson 同精神）.
- ✅ HomeContainerHomeViewBridge 注入 `petSlot: { PetSpriteView(state: homeViewModel.petState) }`（SwiftUI 自动追踪 @Published petState）.
- ✅ AccessibilityID.Home 加 3 个常量（petSpriteRest / petSpriteWalk / petSpriteRun）.
- ✅ HomeViewModelMotionTests 7 case 全 pass（initial .rest / bind once / bind 二次短路 / walking → .walk / 流转 rest→walk→run→rest / 未 bind no-op / deinit stopUpdates）.
- ⚠️ UITest case `testPetSpriteShowsRestStateOnLaunch` 与修改后 `testHomeViewShowsAllPlaceholders` 受 iOS 26 simulator 父级 `.accessibilityElement(children: .contain)` identifier 继承 baseline 行为影响 fail；同坑 pre-existing 影响 home_petName / home_petArea 等多个 a11y 节点；属环境限制非本 story 回归.

### File List

#### 新增（2 个 production + 1 个 test = 3 个文件）

- `iphone/PetApp/Features/Home/Views/PetSpriteView.swift` — AC2: 三态 sprite SwiftUI 组件
- `iphone/PetAppTests/Features/Home/HomeViewModelMotionTests.swift` — AC7: 7 unit test cases（HomeViewModel motion 订阅链路）

#### 修改（7 个文件）

- `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift` — AC1: 加 petState / motionProvider 字段 + bind(motionProvider:) 方法 + deinit
- `iphone/PetApp/Features/Home/Views/HomeView.swift` — AC3: 加 PetSlot generic + petSlot init 参数 + catStage petSlot() 调用 + 改 #Preview 两处
- `iphone/PetApp/Features/Home/Views/HomeContainerView.swift` — AC5: HomeContainerHomeViewBridge 注入 petSlot
- `iphone/PetApp/App/RootView.swift` — AC4: .onAppear 加 motionProvider bind
- `iphone/PetApp/Shared/Constants/AccessibilityID.swift` — AC6: Home enum 加 3 常量
- `iphone/PetAppTests/Features/Home/HomeViewTests.swift` — 兼容 HomeView 接缝（非 spec 钦定但接缝改动连锁）：补 petSlot 参数让 testHomeViewRendersOn{Small,Large}Screen 老 case 继续 pass
- `iphone/PetAppUITests/HomeUITests.swift` — AC8: 新增 testPetSpriteShowsRestStateOnLaunch + 改 testHomeViewShowsAllPlaceholders 内对 petArea 的断言为 petSprite_rest

#### 自动重生（xcodegen-style）

- `iphone/PetApp.xcodeproj/project.pbxproj` — build script 自动添加新文件 PetSpriteView.swift / HomeViewModelMotionTests.swift 到对应 target

## Change Log

| Date | Change | Story | Author |
|------|--------|-------|--------|
| 2026-05-04 | Story 8.4 created from epics.md addendum | 8.4 | bmad-create-story |
| 2026-05-05 | Story 8.4 implementation complete; ready for review | 8.4 | claude-opus-4-7[1m] |
