# Story 37.7: HomeView Scaffold + HomeViewModel class 层次 + Mock/Real 两子类

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want Home Tab idle 态显示 ui_design 高保真界面（StatusBar / CatStage / ActionRow / ChestSlot 接缝 / TeamIdleCard）+ 接缝设计支持 Story 21.1（宝箱）/ Story 12.7（创建/加入队伍 UseCase）后续注入真实 ViewModel,
so that 既有视觉壳又有可持续接缝（HomeView 内部代码 zero edit 让宝箱 / UseCase 注入路径打开），同时把 Story 2.2 老 6 占位区块版 HomeView 完整重写为 ui_design 像素级匹配的高保真界面。

## 故事定位（Epic 37 第四层第 1 条 story；Scaffold 主体 6 屏并行链路的起点）

这是 Epic 37「iPhone 架构层重构 + UI Scaffold（壳先行）」的**第四层 story**——上游 37.3（MainTabView）/ 37.4（AppState）/ 37.5（Theme）/ 37.6（primitives）全部 done，本层 37.7-37.12（5 屏 Scaffold + JoinRoomModal）6 条仅依赖第三层基础，**可并行**。本 story 是 **UI Scaffold 主体** 类——属于 Epic 37 §AC 红线的「数据完全 mock + 禁 import APIClient/Repository/UseCase + 视觉像素级匹配 ui_design + 主题用 `@Environment(\.theme)` 取 token + 测试不引 SnapshotTesting/ViewInspector + 通过 `bash iphone/scripts/build.sh --test`」适用范围。

**本 story 落地后立即解锁**：
- Story 12.7（创建/加入/退出 UseCase + 主界面入口完善）—— `state.onCreateTap()` / `state.onJoinTap()` 改 RealHomeViewModel 实装调用 CreateRoomUseCase / 触发 JoinRoomModal；TeamIdleCard 不动
- Story 21.1（首页宝箱组件）—— 调用方传入 `ChestCardView` 替换 `EmptyView()`；HomeView 内部 zero edit；HomeViewModel 仅持 `chestRemainingSeconds: Int`（Timer 驱动）
- Story 37.12（JoinRoomModal）—— 通过 `.sheet(isPresented: $state.showJoinModal)` 挂在 HomeView 内；`state` 是 HomeViewModel 基类持 `@Published var showJoinModal: Bool` 的唯一 owner（不在 View 层用 @State 双写）
- Story 37.13（accessibility identifier 总表）—— HomeView 全部 a11y identifier 来源；本 story 在 HomeView 内 inline 字符串（`homeStatusBar` / `homeCatStage` / `homeActionFeed` / `homeActionPet` / `homeActionPlay` / `homeTeamIdleCard_create` / `homeTeamIdleCard_join`），Story 37.13 收口归并到 `AccessibilityID.Home`

**本 story 的"实装"动作**（一句话概括）：把 `iphone/PetApp/Features/Home/Views/HomeView.swift`（Story 2.2 落地的 6 占位区块版）+ `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`（Story 5.5 / 37.4 演进的 final class 单实现）**完整重写**：HomeView 改 `generic struct HomeView<ChestSlot: View>` 含 `let chestSlot: () -> ChestSlot` ViewBuilder closure + `@ObservedObject var state: HomeViewModel`（基类直接，**不**再泛型 state）；HomeViewModel 从 `final class` 改 `class`（去 `final` 让 Mock/Real 子类可继承）+ 加 5 个 `@Published` 字段 + 5 个 abstract method + `MockHomeViewModel: HomeViewModel` 子类（硬编码 mock + override 方法仅 print）+ `RealHomeViewModel: HomeViewModel` 子类骨架（构造注入 AppState；override 方法本期为占位，Story 12.7/21.1 实装真实 UseCase 调用）。HomeView 视觉按 `iphone/ui_design/source/screens/home.jsx` + `iphone/ui_design/README.md` §HomeScreen 像素级翻译（StatusBar / CatStage / ActionRow / 宝箱位接缝 / TeamIdleCard 5 区块）。

**关键路径："重新实装"而非"渐进重构"（与 Story 37.3 / 37.4 同精神）**：

- Story 2.2 落地的老 HomeView body（VStack + petAndChestRow + stepBalanceLabel + versionFooter）**整段重写**——保留 `userInfoBar` / `petArea` / `chestArea` / `stepBalance` / `versionLabel` 等 inline modifier 不切实际（视觉 IA 已变），但保留对应 `AccessibilityID.Home.userInfo` / `petArea` / `stepBalance` / `chestRemaining` / `petName` / `versionLabel` 常量**继续被新 HomeView 的对应区块复用**（命名继续，物理位置变；让 Story 2.5 / 5.5 / 37.4 老测试不漂移）
- HomeViewModel 字段保留：`@Published nickname / appVersion / serverInfo / loadingState`（Story 5.2 / 5.5 / 2.5 wire 路径仍生效）；新增字段（greeting / weather / stats / interactionAnimation / showJoinModal）默认值合理；老 `init` 5 个 / `bind` 3 个 / `applyHomeData(_:)` / `loadHome()` / `start()` / `applyPingResult` / `resetLoadHomeForRetry()` 等公开 API **签名不变**（基类继续暴露）
- 新增 `MockHomeViewModel` / `RealHomeViewModel` 文件**不替换** RootView 内 `HomeViewModel()` 老实例化路径——RootView 仍用 `HomeViewModel()` 无参 init（基类）；本 story **不**改 RootView wire（Story 12.7 / 21.x 时由下游 caller 选择是用基类 vs Mock vs Real）

**不涉及**（红线）：
- **不**改 RootView wire 路径（仍用 `@StateObject homeViewModel = HomeViewModel()` 基类无参实例；Story 12.7 决定何时切到 RealHomeViewModel）
- **不**改 MainTabView FloatingTabBar SF Symbol 占位（Dev Notes Story 37.6 §"MainTabView FloatingTabBar SF Symbol 与 Icons 表的一致性" 已对齐；本 story 不收口）
- **不**实装 JoinRoomModal 真实内容（Story 37.12 落地；本 story 只在 HomeView 内通过 `.sheet(isPresented: $state.showJoinModal)` 挂 `JoinRoomModalPlaceholder()` —— 已有占位 stub）
- **不**实装 ChestCardView（Story 21.1 落地；本 story 调用方传 `EmptyView()` 占位）
- **不**实装 RealHomeViewModel 内的 CreateRoomUseCase / JoinRoomUseCase / FeedUseCase 真实调用（本 story 占位 print；Story 12.7 / 14.x 等接真 UseCase）
- **不**改 HomeData / AppState 类型 / 字段（Story 37.4 已锁定）
- **不**引 SnapshotTesting / ViewInspector（ADR-0002 §3.1 钦定 XCTest only）
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3）
- **不**改 server/ 任何文件
- **不**实装真实状态条进度动画 / 真实 sprite 渲染（CatStage 用 `Image(systemName: "cat.fill")` + 椭圆背景占位；Story 8.x / 30.x 后接 sprite）
- **不**收口 `AccessibilityID.Home` 常量到新 7 个字段（本 story inline 字符串；Story 37.13 一次性归并所有 7 屏 a11y identifier）
- **不**做 emoji floatUp 真实 keyframe 动画（用 SwiftUI `.animation` 简化版即可；视觉精度由 Story 37.13 visual-review-checklist 把关）
- **不**预先生成 PetStats / AnimationState 之外的额外 helper / mapping 类型

## Acceptance Criteria

> **AC 编号体系**：AC1 是 HomeViewModel class 层次重构（去 final + 加字段 + abstract methods）；AC2 是 MockHomeViewModel / RealHomeViewModel 两子类落地；AC3 是 HomeView generic struct 重写（含 chestSlot 接缝 + 5 区块视觉）；AC4 是 PetStats / AnimationState 辅助类型；AC5 是 RootView wire 兼容（zero edit）；AC6 是 #Preview 双主题；AC7 是单元测试 ≥4 case；AC8 是 UITest a11y 定位 7 锚；AC9 是 build verify；AC10 是 Deliverable 清单。

### AC1 — HomeViewModel 改 class 层次基类（去 final + 加 5 字段 + 5 abstract method + 保留全部老 API）

**改动文件**：`iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`

**关键改动 1**（去 final 让子类可继承）：
```swift
// 旧
public final class HomeViewModel: ObservableObject { ... }

// 新
public class HomeViewModel: ObservableObject { ... }
```

**关键改动 2**（新增 5 个 @Published 字段 + 默认值；与 Story 12.7 / 21.x / 37.12 接缝）：
```swift
// 新增字段（与现有 nickname / appVersion / serverInfo / loadingState 同模块）
@Published public var greeting: String = "想你啦 ♥"
@Published public var weather: String = "今天 · 晴"
@Published public var stats: PetStats = .mockHappy   // 见 AC4
@Published public var interactionAnimation: AnimationState = .idle   // 见 AC4
@Published public var showJoinModal: Bool = false
```

**关键改动 3**（新增 5 个 abstract method；基类用 `fatalError("subclass override")` 占位，子类必 override）：
```swift
public func onCreateTap() {
    fatalError("HomeViewModel.onCreateTap must be overridden by subclass")
}
public func onJoinTap() {
    fatalError("HomeViewModel.onJoinTap must be overridden by subclass")
}
public func onFeedTap() {
    fatalError("HomeViewModel.onFeedTap must be overridden by subclass")
}
public func onPetTap() {
    fatalError("HomeViewModel.onPetTap must be overridden by subclass")
}
public func onPlayTap() {
    fatalError("HomeViewModel.onPlayTap must be overridden by subclass")
}
```

> **关键决策**：abstract method 用 `fatalError` 而非 `Default Implementation Empty Body`——因 Story 37.7 epic AC line 4734 明确钦定 abstract method 语义（"基类含字段 + 默认/abstract 方法"），且 fatalError 让 caller 漏 override 时**立即崩溃** + 测试覆盖逻辑路径。**不接受**默认 empty 实现（会让 RealHomeViewModel 漏 override 不 crash 但行为静默错）。

> **基类无参 init 兼容路径**：RootView 内 `@StateObject private var homeViewModel = HomeViewModel()` 老路径调用基类**无参 init**（Story 5.2 / 2.5 路径）—— 调用基类 `onCreateTap` / `onJoinTap` 等会触发 fatalError；但**本 story 范围内 RootView 走的是 LaunchedContentView 三态机内分发的 HomeView 子树，按 §AC5 规则 HomeView 仍接受基类 `HomeViewModel`，按钮 onCreateTap / onJoinTap 等回调路径在生产 RootView wire 路径下不会被触发**（Story 12.7 / 21.x 实装时再决定 RootView 是否切到 RealHomeViewModel）。**Preview / UITest skip-guest-login 路径必须用 MockHomeViewModel**（详见 AC2 + Dev Notes "RootView wire 不动 + 基类 vs Mock 选择策略"）。

**关键改动 4**（保留全部老公开 API，签名 / 行为完全不变）：
- `public init(nickname:appVersion:serverInfo:appState:)` 老 init #1
- `public init(nickname:pingUseCase:appVersion:serverInfo:appState:)` Story 2.5 init #2
- `public init(nickname:pingUseCase:loadHomeUseCase:errorPresenter:appVersion:serverInfo:appState:)` Story 5.5 init #3
- `public func bind(pingUseCase:)` Story 2.5
- `public func bind(loadHomeUseCase:errorPresenter:)` Story 5.5
- `public func bind(appState:)` Story 37.4
- `public func start() async` Story 2.5
- `public func loadHome() async` Story 5.5
- `public func applyHomeData(_:)` Story 5.5 / 37.4
- `public func resetLoadHomeForRetry()` Story 5.5
- `public nonisolated static func readAppVersion()` Story 2.5
- `func applyHomeError(_:)` internal Story 5.5

**对应 Tasks**: Task 1.1, 1.2, 1.3, 1.4

### AC2 — MockHomeViewModel / RealHomeViewModel 两子类（独立文件）

**新建文件**: `iphone/PetApp/Features/Home/ViewModels/MockHomeViewModel.swift`

```swift
// MockHomeViewModel.swift
// Story 37.7 AC2: HomeViewModel mock 子类，用于 #Preview / UITest skip-guest-login / Scaffold 单元测试.
//
// 设计：
//   - 硬编码 mock 数据（greeting / weather / stats / nickname / chestRemaining 等）
//   - override 5 个 abstract method 仅 print log（不调任何 UseCase / AppState mutation）
//   - 暴露 `invocations: [Invocation]` 数组让单元测试断言点击触发
//   - **不**依赖 AppState（Mock 路径走纯 ViewModel-only 数据）

import Foundation
import os.log

@MainActor
public final class MockHomeViewModel: HomeViewModel {
    /// 单元测试用：记录所有方法调用
    public enum Invocation: Equatable {
        case createTap
        case joinTap
        case feedTap
        case petTap
        case playTap
    }

    @Published public var invocations: [Invocation] = []

    public init() {
        super.init(
            nickname: "小花",
            appVersion: "0.0.0",
            serverInfo: "mock"
        )
        // 重置默认值为更"展示用" mock 数据
        self.greeting = "小花想你啦 ♥"
        self.weather = "今天 · 晴"
        self.stats = .mockHappy
        self.interactionAnimation = .idle
        self.showJoinModal = false
    }

    // MARK: - override abstract methods

    public override func onCreateTap() {
        os_log(.debug, "MockHomeViewModel.onCreateTap")
        invocations.append(.createTap)
    }

    public override func onJoinTap() {
        os_log(.debug, "MockHomeViewModel.onJoinTap")
        invocations.append(.joinTap)
        self.showJoinModal = true
    }

    public override func onFeedTap() {
        os_log(.debug, "MockHomeViewModel.onFeedTap")
        invocations.append(.feedTap)
        self.interactionAnimation = .flying("🍥")
    }

    public override func onPetTap() {
        os_log(.debug, "MockHomeViewModel.onPetTap")
        invocations.append(.petTap)
        self.interactionAnimation = .flying("💕")
    }

    public override func onPlayTap() {
        os_log(.debug, "MockHomeViewModel.onPlayTap")
        invocations.append(.playTap)
        self.interactionAnimation = .flying("⭐")
    }
}
```

**新建文件**: `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift`

```swift
// RealHomeViewModel.swift
// Story 37.7 AC2: HomeViewModel 生产实装子类（构造注入 AppState；override 5 个 abstract method 占位 stub）.
//
// 范围（本 story 占位；Story 12.7 / 21.x 等填充真实 UseCase 调用）：
//   - 构造注入 AppState（按 ADR-0010 §3.1 ViewModel 注入规则）
//   - override onCreateTap / onJoinTap / onFeedTap / onPetTap / onPlayTap 为占位行为：
//     · onCreateTap: print log（Story 12.7 实装 CreateRoomUseCase）
//     · onJoinTap: 设 showJoinModal = true（与 Mock 同行为；Story 12.7 / 37.12 落地真实 modal 闭包）
//     · onFeedTap / onPetTap / onPlayTap: 设 interactionAnimation = .flying(emoji)（与 Mock 同行为；
//       未来 Story 14.x WS pet.state.changed 真实状态切换时再分化）
//
// **不**调用任何 UseCase / Repository / APIClient（Epic 37 红线：UI Scaffold 数据完全 mock）.

import Foundation
import os.log

@MainActor
public final class RealHomeViewModel: HomeViewModel {
    private let injectedAppState: AppState

    public init(appState: AppState) {
        self.injectedAppState = appState
        super.init(appState: appState)
        // 视觉初值：从 AppState.currentPet?.name 派生 greeting（hydrate 后）；hydrate 前用空 placeholder
        self.greeting = "想你啦 ♥"
        self.weather = "今天 · 晴"
        self.stats = .mockHappy   // Story 8.x / 14.x 后接真实状态
        self.interactionAnimation = .idle
        self.showJoinModal = false
    }

    // MARK: - override abstract methods（本 story 占位；Story 12.7 / 14.x 实装真实 UseCase 调用）

    public override func onCreateTap() {
        os_log(.debug, "RealHomeViewModel.onCreateTap (Story 12.7 will wire CreateRoomUseCase)")
    }

    public override func onJoinTap() {
        os_log(.debug, "RealHomeViewModel.onJoinTap")
        self.showJoinModal = true
    }

    public override func onFeedTap() {
        os_log(.debug, "RealHomeViewModel.onFeedTap (Story 14.x will wire WS pet.state.changed)")
        self.interactionAnimation = .flying("🍥")
    }

    public override func onPetTap() {
        os_log(.debug, "RealHomeViewModel.onPetTap")
        self.interactionAnimation = .flying("💕")
    }

    public override func onPlayTap() {
        os_log(.debug, "RealHomeViewModel.onPlayTap")
        self.interactionAnimation = .flying("⭐")
    }
}
```

> **关键决策 1**：MockHomeViewModel / RealHomeViewModel 都 `final`——子类不可再被继承（ADR-0010 §3.1 mock 模式钦定 + 保 Swift dispatch 性能）；只有基类 `HomeViewModel` 是 `class`（非 final）。

> **关键决策 2**：MockHomeViewModel 用 invocations 数组而非 closure 注入——便于单元测试用 `XCTAssertEqual(vm.invocations, [.feedTap])` 断言；不需要外层创建 closure spy 支架（与 Story 37.6 PrimitivesTests 风格一致：直接断言 ObservableObject 内 @Published 字段）。

> **关键决策 3**：RealHomeViewModel.onJoinTap 也设 `showJoinModal = true`（与 Mock 同），不在本 story 写 Story 12.7 真实 JoinRoomUseCase 调用——保持本 story Scaffold 红线（数据完全 mock + 禁 import UseCase）。Story 37.12 落地真实 JoinRoomModal 内容时，**不**改 onJoinTap 行为（onJoinTap 仅打开 sheet；真正 join 调用在 modal `onConfirm` 闭包内）。

**对应 Tasks**: Task 2.1, 2.2

### AC3 — HomeView 改 generic struct + chestSlot 接缝 + 5 区块视觉重写

**改动文件**: `iphone/PetApp/Features/Home/Views/HomeView.swift`

**关键改动 1**（struct 签名重构 → generic + ViewBuilder closure 参数）：
```swift
// 旧
public struct HomeView: View {
    @ObservedObject public var viewModel: HomeViewModel
    @EnvironmentObject var appState: AppState
    private let resetIdentityViewModel: ResetIdentityViewModel?
    private let sessionStore: SessionStore?
    public init(viewModel: HomeViewModel) { ... }
    public init(viewModel: HomeViewModel, resetIdentityViewModel: ResetIdentityViewModel?) { ... }
    public init(viewModel: HomeViewModel, resetIdentityViewModel: ResetIdentityViewModel?, sessionStore: SessionStore?) { ... }
    public var body: some View { ... }
}

// 新
public struct HomeView<ChestSlot: View>: View {
    @ObservedObject public var state: HomeViewModel
    @EnvironmentObject var appState: AppState
    private let resetIdentityViewModel: ResetIdentityViewModel?
    private let sessionStore: SessionStore?
    private let chestSlot: () -> ChestSlot

    public init(
        state: HomeViewModel,
        resetIdentityViewModel: ResetIdentityViewModel? = nil,
        sessionStore: SessionStore? = nil,
        @ViewBuilder chestSlot: @escaping () -> ChestSlot
    ) { ... }

    public var body: some View { ... }
}
```

> **关键决策**：参数命名 `state` 而非 `viewModel`（v2 提案 §Story 37.7 钦定）；旧三个 init 重载**全部删除**（caller 漏改靠编译器报错驱动；详见 AC5 RootView wire 兼容）。

> **保留**: `@EnvironmentObject var appState` / `resetIdentityViewModel` / `sessionStore` 三字段（Story 5.2 / 2.8 / 37.4 wire 不动）。`resetIdentityViewModel` / `sessionStore` 改为 init 默认值 nil（让 Preview / 测试简化构造）。

**关键改动 2**（body 重写——从老 6 占位区块 VStack 改为 ui_design HomeScreen 5 区块）：

按 `iphone/ui_design/source/screens/home.jsx` + `iphone/ui_design/README.md` §HomeScreen 视觉规则：

```swift
public var body: some View {
    ZStack {
        // 背景渐变 (ui_design home.jsx:18 钦定: linear-gradient(180deg, accent-soft 0%, page-bg 38%))
        LinearGradient(
            colors: [theme.colors.accentSoft, theme.colors.pageBg],
            startPoint: .top,
            endPoint: .bottom
        )
        .ignoresSafeArea()

        ScrollView {
            VStack(spacing: theme.spacing.s14) {
                statusBar
                catStage
                actionRow
                chestSlot()           // 接缝：本期传 EmptyView()，Story 21.1 传 ChestCardView
                teamIdleCard
            }
            .padding(.horizontal, theme.spacing.s20)
            .padding(.top, 68)         // ui_design §iOS 设备规格: 状态栏 padding 68pt
            .padding(.bottom, 100)     // 浮动 TabBar 让出空间
        }
    }
    .sheet(isPresented: $state.showJoinModal) {
        // Story 37.12 落地真实 JoinRoomModal；本期挂 placeholder（已存在 stub）
        JoinRoomModalPlaceholder()
    }
}
```

**关键改动 3**（5 区块子视图）—— 详细视觉规则见 Dev Notes "5 区块视觉契约"；这里给关键定位锚：

- **statusBar**（ui_design home.jsx:21-38）：HStack `weather + greeting` 左 + 步数计 capsule 右；`accessibilityIdentifier("homeStatusBar")`
- **catStage**（home.jsx:40-81）：Card 圆角 28（`theme.radius.modalLg` —— ui_design 钦定 28pt）+ overflow 隐藏 + 内含 SF Symbol "cat.fill" 220pt + 等级名牌（左上） + 三状态条（饱食/心情/活力，progress bar 0-100）+ floatUp emoji（interactionAnimation 触发）；`accessibilityIdentifier("homeCatStage")`
- **actionRow**（home.jsx:84-88）：HStack 3 个 ActionButton（喂食 🍥 / 抚摸 💕 / 玩耍 ⭐）；按钮调 `state.onFeedTap() / onPetTap() / onPlayTap()`；inline 内 `Card` 容器；分别 `accessibilityIdentifier("homeActionFeed" / "homeActionPet" / "homeActionPlay")`
- **chestSlot()**：直接调用 ViewBuilder closure；本期传入 `EmptyView()` 占位（接缝硬契约）
- **teamIdleCard**（home.jsx:147-188）：渐变背景 `LinearGradient(accent → accentDeep)` + 圆角 22（`theme.radius.cardLg`）+ 标题"和好友一起玩耍" + 副标题 + 两 PrimaryButton（"创建队伍" 调 `state.onCreateTap()` / "加入队伍" 调 `state.onJoinTap()`）；分别 `accessibilityIdentifier("homeTeamIdleCard_create" / "homeTeamIdleCard_join")`

**关键改动 4**（保留 `userInfoBar` / `petName` / `chestRemaining` / `stepBalance` / `versionLabel` 等老 a11y identifier）：

> **路径**：本 story 视觉 IA 已变（顶部不再是"用户昵称 + 头像"独立 row，而是 statusBar 含 weather + greeting + 步数；猫 / 宝箱 / 步数被融入 catStage / chestSlot()）—— **但 `AccessibilityID.Home.userInfo` / `petArea` / `petName` / `stepBalance` / `chestArea` / `chestRemaining` / `versionLabel` 7 个常量**继续在 HomeView body 内使用，物理位置变化，命名继续，让 Story 2.5 / 5.5 / 37.4 老测试 / UITest 不漂移：
> - `AccessibilityID.Home.userInfo` → 仍标在 statusBar 容器（取代旧 userInfoBar 容器）
> - `AccessibilityID.Home.petArea` → 仍标在 catStage 内 cat sprite Image
> - `AccessibilityID.Home.petName` → 仍标在 catStage 内等级名牌（pet.name 显示位）
> - `AccessibilityID.Home.stepBalance` → 仍标在 statusBar 内步数 capsule
> - `AccessibilityID.Home.chestArea` → **deprecated**，本 story 不渲染（chestSlot 是接缝；本期 EmptyView 占位无元素）—— 老测试若依赖该 identifier 渲染存在，需在本 story 评估改成 chestSlot 占位 wrapper 或老测试 case 转 skip
> - `AccessibilityID.Home.chestRemaining` → 同 `chestArea`，本 story 不渲染（chestSlot 接缝；Story 21.1 chestSlot 内含倒计时）—— 老测试同上
> - `AccessibilityID.Home.versionLabel` → 保留 footer ping/version 角落显示（Story 2.5 钦定 + Story 37.3 AC7 第 4 条 "ping/version 角落显示仍在 Home Tab 可见" 红线）—— Dev Notes "versionFooter 显示位置策略" 详述

> **关键决策**：Dev Notes "Story 5.5 / 2.5 / 37.4 老测试兼容"详细列出哪些老测试可能 broken + 修复策略；本 story **接受** chestArea / chestRemaining 老 identifier 在本期 EmptyView() 接缝期消失（这两个常量本身不删，只是 HomeView body 不渲染）—— 旧 testHomeViewShowsAllSixPlaceholders 类的测试需在本 story 同步调整。

**对应 Tasks**: Task 3.1, 3.2, 3.3, 3.4

### AC4 — 新建 PetStats / AnimationState 辅助类型

**新建文件**: `iphone/PetApp/Features/Home/Models/PetStats.swift`

```swift
// PetStats.swift
// Story 37.7 AC4: HomeView CatStage 三状态条数据模型（饱食/心情/活力）.
//
// 设计：value type + Equatable + Sendable，纯展示数据；mock 值在 .mockHappy / .mockTired / .mockEmpty.
// 节点 8 / 14.x 后接真实状态时再扩展（如 streak / lastUpdated 等字段）；本 story 范围内仅展示 value.

import Foundation

public struct PetStats: Equatable, Sendable {
    public let hunger: Int      // 饱食 0-100
    public let mood: Int        // 心情 0-100
    public let energy: Int      // 活力 0-100

    public init(hunger: Int, mood: Int, energy: Int) {
        self.hunger = max(0, min(100, hunger))
        self.mood = max(0, min(100, mood))
        self.energy = max(0, min(100, energy))
    }
}

extension PetStats {
    /// mock：开心状态（home.jsx 默认 mock：饱食 72 / 心情 88 / 活力 65）
    public static let mockHappy = PetStats(hunger: 72, mood: 88, energy: 65)
    /// mock：低值状态（用于 edge case 测试 stats.hunger = 0）
    public static let mockEmpty = PetStats(hunger: 0, mood: 0, energy: 0)
    /// 默认 zero（基类 @Published var stats: PetStats = .zero 用）
    public static let zero = PetStats(hunger: 0, mood: 0, energy: 0)
}
```

**新建文件**: `iphone/PetApp/Features/Home/Models/AnimationState.swift`

```swift
// AnimationState.swift
// Story 37.7 AC4: HomeView CatStage interactionAnimation 状态枚举（floatUp emoji 触发用）.
//
// 设计：enum + Equatable，关联值是 emoji 字符串（"🍥" / "💕" / "⭐" 三种触发；未来扩展加 case）；
// idle 不渲染，flying 触发 1.4s ease 上移消失.

import Foundation

public enum AnimationState: Equatable {
    /// 静止态，不渲染浮动 emoji.
    case idle

    /// 触发 floatUp 动画，关联值是 emoji 字符串（"🍥" / "💕" / "⭐"）.
    /// 1.4s 后 ViewModel 自动重置回 idle（HomeView 内 SwiftUI animation 完成 callback 触发）.
    case flying(String)
}
```

> **关键决策**：PetStats 字段名 `hunger / mood / energy` 直接对齐 ui_design home.jsx:77-79 三条状态条 `饱食 / 心情 / 活力`；不引入"feedingLevel" 等长名（Swift 习惯 + ui_design 已是简短中文）。AnimationState 关联值用 String 而非 enum case `feed/pet/play` —— floatUp 内容是视觉 emoji，让调用方决定（"🍥" 是喂食心智 emoji，未来可能扩成"🐟"/"🍕"等多 case；解耦语义与表现）。

**对应 Tasks**: Task 4.1, 4.2

### AC5 — RootView wire 兼容（zero edit）

**关键约束**: RootView 当前路径（line 268 `.environmentObject(homeViewModel)`）+ HomeContainerView 内的 HomeContainerHomeViewBridge（HomeContainerView.swift:50-62）使用旧 HomeView init `HomeView(viewModel: homeViewModel, resetIdentityViewModel: resetIdentityViewModel, sessionStore: sessionStore)` ——本 story 改 HomeView 签名后，**必须**修复 caller 否则编译失败。

**修复路径**（caller 漏改靠编译器报错驱动，详见 Story 37.3 / 37.4 同精神）：

1. **HomeContainerView.HomeContainerHomeViewBridge**（HomeContainerView.swift:50-62）—— HomeView 调用必改：
   ```swift
   // 旧
   HomeView(
       viewModel: homeViewModel,
       resetIdentityViewModel: resetIdentityViewModel,
       sessionStore: sessionStore
   )

   // 新（Story 37.7 改造）
   HomeView(
       state: homeViewModel,                       // 参数名 viewModel → state
       resetIdentityViewModel: resetIdentityViewModel,
       sessionStore: sessionStore
   ) {
       EmptyView()                                 // chestSlot ViewBuilder closure：本期占位；Story 21.1 改传 ChestCardView()
   }
   ```

2. **HomeView_Previews**（HomeView.swift:344-352）—— Preview 改用 MockHomeViewModel + chestSlot 占位：
   ```swift
   #if DEBUG
   struct HomeView_Previews: PreviewProvider {
       static var previews: some View {
           HomeView(state: MockHomeViewModel()) { EmptyView() }
               .environmentObject(AppState())
               .environment(\.theme, ThemeName.candy.theme)
       }
   }
   #endif
   ```

3. **MainTabView_Previews**（MainTabView.swift:108-119）—— 旧 Preview `.environmentObject(HomeViewModel())` 仍生效（基类 HomeViewModel 仍是 ObservableObject）；不需改。

> **关键决策**：本 story **不**改 RootView line 34 `@StateObject private var homeViewModel = HomeViewModel()` 老路径——基类 HomeViewModel 仍允许无参 init（Story 5.5 / 2.5 / 37.4 wire 全部走基类 / Story 12.7 决定何时切到 RealHomeViewModel）。RootView 内 `.environmentObject(homeViewModel)` 注入基类 HomeViewModel，HomeContainerHomeViewBridge 取出后传给 HomeView `state:` 参数（基类 ObservableObject 兼容）。

> **caller 漏改靠编译器报错驱动**：HomeView 的旧 3 个 init 重载（`init(viewModel:)` / `init(viewModel:resetIdentityViewModel:)` / `init(viewModel:resetIdentityViewModel:sessionStore:)`）**全部删除**；任何残留调用旧 init 的地方编译失败 → dev 修编译错误时修完全部 caller。**不依赖 grep 兜底**。

**对应 Tasks**: Task 5.1, 5.2, 5.3

### AC6 — #Preview 提供 MockHomeViewModel + candy 主题（双 Preview：candy / dark）

HomeView 文件底部 `#if DEBUG ... #endif` 块含 2 个 Preview：

```swift
#if DEBUG
#Preview("HomeView — candy") {
    HomeView(state: MockHomeViewModel()) { EmptyView() }
        .environmentObject(AppState())
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("HomeView — dark") {
    HomeView(state: MockHomeViewModel()) { EmptyView() }
        .environmentObject(AppState())
        .environment(\.theme, ThemeName.dark.theme)
}
#endif
```

> **关键决策**：Preview 用 `#Preview` macro 而非 `PreviewProvider`（与 Story 37.5 / 37.6 同模式）；保留下方 `HomeView_Previews: PreviewProvider` 同时存在也可（Xcode Canvas 双显），但本 story 推荐切到 `#Preview` macro 简化；同时移除老 `PreviewProvider` 实现避免重复。

**对应 Tasks**: Task 6.1

### AC7 — 单元测试覆盖（≥4 case，纯 XCTest + MockHomeViewModel + AppState）

新建文件: `iphone/PetAppTests/Features/Home/HomeViewScaffoldTests.swift`

落地以下 5 case（≥4 case 按 epic AC；额外 +1 给 invocations 顺序稳定性更稳）：

```swift
// HomeViewScaffoldTests.swift
// Story 37.7 AC7: HomeView Scaffold + HomeViewModel class 层次单元测试.
//
// 测试基础设施约束（与 Story 2.7 + ADR-0002 §3.1 衔接）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）.
//   - 不引 ViewInspector / SnapshotTesting.
//   - 走 ViewModel 行为 + invocations 数组断言；不走 SwiftUI body 内省.

import XCTest
@testable import PetApp

@MainActor
final class HomeViewScaffoldTests: XCTestCase {

    // MARK: - case#1 happy: MockHomeViewModel 默认状态

    /// 验证 MockHomeViewModel 默认值与 Story 37.7 spec 一致（greeting / weather / stats / interactionAnimation / showJoinModal）.
    func testMockHomeViewModelDefaultStateMatchesSpec() {
        let vm = MockHomeViewModel()
        XCTAssertEqual(vm.greeting, "小花想你啦 ♥")
        XCTAssertEqual(vm.weather, "今天 · 晴")
        XCTAssertEqual(vm.stats, .mockHappy)
        XCTAssertEqual(vm.interactionAnimation, .idle)
        XCTAssertFalse(vm.showJoinModal)
        XCTAssertEqual(vm.invocations, [])
    }

    // MARK: - case#2 happy: 点 "创建队伍" → onCreateTap 触发

    /// 验证 onCreateTap 调用后 invocations 含 .createTap.
    func testOnCreateTapAppendsInvocation() {
        let vm = MockHomeViewModel()
        vm.onCreateTap()
        XCTAssertEqual(vm.invocations, [.createTap])
    }

    // MARK: - case#3 happy: 点 "喂食" → interactionAnimation = .flying("🍥")

    /// 验证 onFeedTap 调用后 interactionAnimation 切到 .flying("🍥") + invocations 含 .feedTap.
    func testOnFeedTapTriggersFlyingEmojiAndInvocation() {
        let vm = MockHomeViewModel()
        vm.onFeedTap()
        XCTAssertEqual(vm.interactionAnimation, .flying("🍥"))
        XCTAssertEqual(vm.invocations, [.feedTap])
    }

    // MARK: - case#4 happy: 点 "加入队伍" → showJoinModal = true

    /// 验证 onJoinTap 调用后 showJoinModal 切到 true + invocations 含 .joinTap.
    func testOnJoinTapTogglesShowJoinModalToTrue() {
        let vm = MockHomeViewModel()
        XCTAssertFalse(vm.showJoinModal)
        vm.onJoinTap()
        XCTAssertTrue(vm.showJoinModal)
        XCTAssertEqual(vm.invocations, [.joinTap])
    }

    // MARK: - case#5 edge: stats.hunger = 0 → PetStats 渲染最低值不报错

    /// 验证 PetStats(hunger: 0, mood: 0, energy: 0) 构造合法 + 字段值正确（不下溢 / 不 crash）.
    /// 对应 epic AC line 4743 "edge: stats.hunger = 0 → 状态条渲染最低值（不报错；用 a11y label 文字验证）".
    /// 本测试断言 PetStats 数据契约；视觉断言由 #Preview + Story 37.13 visual-review-checklist 兜底.
    func testPetStatsZeroValueDoesNotUnderflow() {
        let stats = PetStats(hunger: 0, mood: 0, energy: 0)
        XCTAssertEqual(stats.hunger, 0)
        XCTAssertEqual(stats.mood, 0)
        XCTAssertEqual(stats.energy, 0)
        XCTAssertEqual(stats, PetStats.mockEmpty)
        XCTAssertEqual(stats, PetStats.zero)
    }

    // MARK: - case#6 happy: RealHomeViewModel 构造注入 AppState 不 crash

    /// 验证 RealHomeViewModel(appState:) 构造正常 + override 方法可调用（不触发 fatalError 路径）.
    /// 防止 RealHomeViewModel.onCreateTap 等忘记 override 时本测试立刻 fail（fatalError 在测试中 → trap）.
    func testRealHomeViewModelConstructionAndAbstractMethodsDoNotCrash() {
        let appState = AppState()
        let vm = RealHomeViewModel(appState: appState)
        // 调用 5 个 override 方法验证不进入基类 fatalError 路径（progress-only check; 不断言行为细节）.
        vm.onCreateTap()
        vm.onJoinTap()
        vm.onFeedTap()
        vm.onPetTap()
        vm.onPlayTap()
        XCTAssertTrue(vm.showJoinModal)   // onJoinTap 切到 true，作为 override 路径已执行的代理证据
    }
}
```

> **关键决策**：本测试**不**走 `UIHostingController` 渲染 SwiftUI body（与 HomeViewTests case#2 风格不同）—— ADR-0002 §3.1 钦定 XCTest only + ViewModel 行为可独立断言；视觉断言由 #Preview + UITest a11y identifier 兜底。

> **不**测 HomeView body 渲染含 a11y identifier（属 UITest 范围；详见 AC8 + Dev Notes "测试边界"）。

> **不**测 fatalError 路径（基类 abstract method 覆盖在 case#6 间接证明 override 已生效；显式 fatalError trap 测试 ADR-0002 §3.1 不强制）。

**对应 Tasks**: Task 7.1

### AC8 — UITest a11y identifier 7 锚可定位

修改文件: `iphone/PetAppUITests/HomeUITests.swift` —— 加 1 个新 test case 验证 7 个新 a11y identifier 可定位（沿用 Story 2.2 / 2.5 既有 HomeUITests 风格）：

```swift
// Story 37.7: HomeView Scaffold 7 锚 a11y identifier 可定位验证.
// 与 Story 2.2 testHomeViewShowsAllSixPlaceholders 同模式；本 test 验证 ui_design 高保真 5 区块各 a11y 锚.
func testHomeScaffoldShowsAllSevenAnchors() throws {
    let app = XCUIApplication()
    app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
    app.launch()

    XCTAssertTrue(app.otherElements["homeStatusBar"].waitForExistence(timeout: 5))
    XCTAssertTrue(app.otherElements["homeCatStage"].exists)
    XCTAssertTrue(app.buttons["homeActionFeed"].exists)
    XCTAssertTrue(app.buttons["homeActionPet"].exists)
    XCTAssertTrue(app.buttons["homeActionPlay"].exists)
    XCTAssertTrue(app.buttons["homeTeamIdleCard_create"].exists)
    XCTAssertTrue(app.buttons["homeTeamIdleCard_join"].exists)
}
```

> **关键决策**：本 UITest case **不**主动点击按钮 / 验证 sheet 弹出（属 Story 12.7 / 37.12 范围）；仅验证视觉锚存在（让 Story 37.13 a11y 总表归并时有 baseline）。

> **现有 testHomeViewShowsAllSixPlaceholders**（Story 2.2 落地）需保留 vs 调整：本 story 范围内 chestArea 不再渲染（chestSlot EmptyView 占位），该 case 内对 `app.otherElements[AccessibilityID.Home.chestArea]` / `chestRemaining` 的断言**会 fail**——本 story 必须同步修改老 case：删除对 chestArea / chestRemaining 的 `XCTAssertTrue(...exists)` 断言（chestSlot 接缝期不渲染），保留其它 5 个 identifier 断言。**不** skip 整个 case（保 Story 2.2 / 2.5 / 5.5 / 37.4 wire 链路 UITest 兜底）。

**对应 Tasks**: Task 8.1, 8.2

### AC9 — xcodegen regen + build verify

完成 AC1-AC8 后：

1. `cd iphone && xcodegen generate` 让 4 个新文件加入 PetApp / PetAppTests target（project.yml 通配规则自动 inclusion；新增 `Features/Home/ViewModels/MockHomeViewModel.swift` + `Features/Home/ViewModels/RealHomeViewModel.swift` + `Features/Home/Models/PetStats.swift` + `Features/Home/Models/AnimationState.swift` + `PetAppTests/Features/Home/HomeViewScaffoldTests.swift`）
2. `bash iphone/scripts/build.sh --test` 跑测试通过
   - 总 case 数：256（Story 37.6 落地后基线）+ 本 story 新增 6 case + 老 HomeUITests case 调整（不增减总数，仅修改 chestArea / chestRemaining 断言）→ ~262 case 全绿
   - 老 testHomeViewShowsAllSixPlaceholders 调整后**不**改名（保 git history 可读）；如确实需要改名 dev 自定（如 `testHomeViewShowsCoreAnchors`）记入 Change Log
3. grep 验证：
   - `grep -c "class HomeViewModel" iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift` ≥ 1（防漏去 final）
   - `grep -c "fatalError" iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift` ≥ 5（5 个 abstract method 的 fatalError 占位）
   - `grep "final class" iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift` 输出空（基类不能 final）
   - `grep -c "override func" iphone/PetApp/Features/Home/ViewModels/MockHomeViewModel.swift` ≥ 5（5 abstract method override）
   - `grep -c "override func" iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift` ≥ 5

> **dev 实装备注**：dev 必须 `xcodegen generate` 后 commit 一并提交 `iphone/PetApp.xcodeproj/project.pbxproj` 改动（与 Story 37.5 / 37.6 同模式）。

**对应 Tasks**: Task 9.1, 9.2, 9.3

### AC10 — Deliverable 清单

- ✅ HomeViewModel 改 `class`（去 final）+ 加 5 `@Published` 字段 + 5 abstract method（fatalError 占位）；保留全部 Story 2.2 / 2.5 / 5.2 / 5.5 / 37.4 老 init / bind / 公开 API 签名
- ✅ MockHomeViewModel: HomeViewModel 子类落地（硬编码 mock + 5 override + invocations 数组）
- ✅ RealHomeViewModel: HomeViewModel 子类落地（构造注入 AppState + 5 override 占位 stub）
- ✅ HomeView 改 `generic struct HomeView<ChestSlot: View>`（参数名 viewModel → state）+ chestSlot ViewBuilder closure 接缝 + 5 区块视觉重写（按 ui_design home.jsx）
- ✅ PetStats 类型（hunger/mood/energy 0-100，含 mockHappy / mockEmpty / zero）+ AnimationState enum（idle / flying(String)）
- ✅ HomeContainerView.HomeContainerHomeViewBridge 修复 caller（HomeView 新签名调用）
- ✅ HomeView 老 PreviewProvider 移除 / 改 #Preview macro 双主题（candy / dark）
- ✅ HomeViewScaffoldTests.swift 含 6 case（Mock 默认状态 / onCreateTap / onFeedTap / onJoinTap / PetStats 零值 / RealHomeViewModel 构造）
- ✅ HomeUITests 加 testHomeScaffoldShowsAllSevenAnchors + 调整老 testHomeViewShowsAllSixPlaceholders
- ✅ `iphone/PetApp.xcodeproj/project.pbxproj` 改动随 commit（xcodegen regen 结果）
- ✅ `bash iphone/scripts/build.sh --test` 全绿
- ✅ project.yml **不**手动改（通配规则自动 inclusion）
- ✅ RootView wire 不改（line 34 `HomeViewModel()` 基类老路径保留）
- ✅ MainTabView wire 不改（FloatingTabBar SF Symbol 占位 Story 37.7 不收口；Story 37.13 时一并清理）
- ✅ 老 AccessibilityID.Home 7 个常量保留（chestArea / chestRemaining 在本期 chestSlot 接缝期不渲染但常量本身不删）
- ✅ JoinRoomModal sheet 挂载点（`.sheet(isPresented: $state.showJoinModal)`）落地 + 内部仍是 JoinRoomModalPlaceholder（Story 37.12 替换）

## Tasks / Subtasks

- [x] Task 1: HomeViewModel 改 class 层次基类（AC1）
  - [x] 1.1 改 `public final class HomeViewModel` → `public class HomeViewModel`（去 final）
  - [x] 1.2 新增 5 个 `@Published` 字段：greeting / weather / stats: PetStats / interactionAnimation: AnimationState / showJoinModal: Bool
  - [x] 1.3 新增 5 个 abstract method：onCreateTap / onJoinTap / onFeedTap / onPetTap / onPlayTap，全部 `fatalError("subclass override")` 占位
  - [x] 1.4 验证全部老公开 API（init #1/#2/#3 + bind 3 个 + start / loadHome / applyHomeData / resetLoadHomeForRetry / readAppVersion / applyHomeError）签名 / 行为不变
- [x] Task 2: 新建 Mock/Real 子类（AC2）
  - [x] 2.1 新建 `iphone/PetApp/Features/Home/ViewModels/MockHomeViewModel.swift`（final class + invocations 数组 + 5 override + 硬编码 mock 数据）
  - [x] 2.2 新建 `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift`（final class + appState 构造注入 + 5 override 占位 stub）
- [x] Task 3: HomeView struct 重写 + 5 区块（AC3）
  - [x] 3.1 改 HomeView struct 签名为 `HomeView<ChestSlot: View>`，参数 `state: HomeViewModel` + `chestSlot: () -> ChestSlot` ViewBuilder closure；删除老 3 个 init 重载，新建 1 个 init
  - [x] 3.2 body 重写为 ZStack 含背景渐变 + ScrollView VStack 5 区块（statusBar / catStage / actionRow / chestSlot() / teamIdleCard）
  - [x] 3.3 落地 5 区块子视图（statusBar / catStage / actionRow / teamIdleCard），按 ui_design home.jsx 视觉规则像素级翻译
  - [x] 3.4 7 个 a11y identifier inline 字符串：homeStatusBar / homeCatStage / homeActionFeed / homeActionPet / homeActionPlay / homeTeamIdleCard_create / homeTeamIdleCard_join；同时保留 AccessibilityID.Home.userInfo / petArea / petName / stepBalance / versionLabel 5 个常量在新 body 内对应位置（不删 Home enum 常量）
- [x] Task 4: PetStats / AnimationState 辅助类型（AC4）
  - [x] 4.1 新建 `iphone/PetApp/Features/Home/Models/PetStats.swift`（hunger/mood/energy + mockHappy / mockEmpty / zero static）
  - [x] 4.2 新建 `iphone/PetApp/Features/Home/Models/AnimationState.swift`（enum idle / flying(String)）
- [x] Task 5: RootView / HomeContainerView wire 修复（AC5）
  - [x] 5.1 改 `HomeContainerView.HomeContainerHomeViewBridge.body`：`HomeView(viewModel:..., resetIdentityViewModel:..., sessionStore:...)` → `HomeView(state:..., resetIdentityViewModel:..., sessionStore:...) { EmptyView() }`
  - [x] 5.2 改 `HomeView_Previews`（旧 PreviewProvider）→ `#Preview` macro 双主题（candy / dark），用 MockHomeViewModel + AppState（详见 AC6）
  - [x] 5.3 验证 RootView line 34 / 268 不改（`@StateObject homeViewModel = HomeViewModel()` + `.environmentObject(homeViewModel)` 路径不动）
- [x] Task 6: #Preview 双主题（AC6）
  - [x] 6.1 HomeView 文件底部 `#if DEBUG` 块加 2 个 `#Preview`（candy / dark）；移除老 PreviewProvider
- [x] Task 7: 单元测试（AC7）
  - [x] 7.1 新建 `iphone/PetAppTests/Features/Home/HomeViewScaffoldTests.swift`，落地 6 case
- [x] Task 8: UITest 调整（AC8）
  - [x] 8.1 修改 `iphone/PetAppUITests/HomeUITests.swift` 内 `testHomeViewShowsAllSixPlaceholders`：删除对 `AccessibilityID.Home.chestArea` / `chestRemaining` 两断言（本期 chestSlot 接缝期不渲染）；保留其它 4 个 identifier 断言
  - [x] 8.2 加 `testHomeScaffoldShowsAllSevenAnchors`：验证 7 个新 a11y identifier 可定位
- [x] Task 9: xcodegen regen + build verify（AC9）
  - [x] 9.1 `cd iphone && xcodegen generate` 让 5 个新文件加入 target
  - [x] 9.2 `bash iphone/scripts/build.sh --test` 跑测试通过（266 unit tests / 0 failures）
  - [x] 9.3 grep 校验抽样：HomeViewModel 含 `class HomeViewModel`（去 final）+ 5 个 fatalError；MockHomeViewModel / RealHomeViewModel 各含 5 个 override func
- [x] Task 10: Deliverable 清单确认（AC10）
  - [x] 10.1 5 个新文件 + 修改 4 个老文件（HomeViewModel.swift / HomeView.swift / HomeContainerView.swift / HomeViewTests.swift）+ 调整 HomeUITests + pbxproj regen 全部待 commit（不在本 dev-story 范围）

## Dev Notes

### HomeViewModel 改 class 而非 protocol any 模式（关键设计决策）

**选定**: 基类 `class HomeViewModel: ObservableObject`（去 final）+ 子类 `MockHomeViewModel: HomeViewModel` / `RealHomeViewModel: HomeViewModel` 各自 final。

**为何不走 `protocol HomeViewModelProtocol + any HomeViewModelProtocol`**：

1. **SwiftUI `@ObservedObject` 不接受 `any P`**：`@ObservedObject var state: any HomeViewModelProtocol` 编译失败——`any P` 不能 conform `ObservableObject`（这是 v2.1 codex BLOCKER 7 命中点）；SwiftUI 数据流要求 `@ObservedObject` 持有具体 `class & ObservableObject` 实例。
2. **HomeView signature 简化**：`HomeView<ChestSlot: View>` 仅一个泛型参（chestSlot 内容类型）；state 走 class 层次而非泛型 `<State: HomeViewModelProtocol>` 让 caller 端 type 不膨胀。
3. **多态调用**：MockHomeViewModel / RealHomeViewModel 都是 HomeViewModel 子类，HomeView body 内调 `state.onCreateTap()` 走 Swift 动态 dispatch（class method 默认 dynamic），子类 override 自动生效。

**否决**：
- **Closure-based ViewModel**（让 HomeView 接受 `let onCreateTap: () -> Void`）：否决——v2.1 BLOCKER 10 命中（state owner 双写：showJoinModal 在 View @State + ViewModel @Published 同时存在）；class 层次让 showJoinModal 唯一 owner = HomeViewModel 基类。
- **Generic class（`HomeView<VM: HomeViewModelProtocol>`）**：否决——caller 端类型膨胀严重；`HomeContainerHomeViewBridge` 内每次构造 HomeView 都要写完整 generic 类型签名。

### HomeView<ChestSlot: View> generic + chestSlot ViewBuilder closure 接缝（关键设计决策）

**选定**: HomeView 是 `generic struct HomeView<ChestSlot: View>`，含 `let chestSlot: () -> ChestSlot` ViewBuilder closure 参数；本期 Scaffold 阶段调用方传入 `EmptyView()` 占位；Story 21.1 改传入 `ChestCardView()`，**HomeView 内部代码 zero edit**。

**为何走 ViewBuilder closure 而非 enum case `chestState: ChestSlotState`**：

1. **接缝硬契约**：调用方决定 chestSlot 渲染什么；Story 21.1 落地时无需改 HomeView body 任何一行。
2. **类型安全 + Preview 灵活**：泛型让 EmptyView / ChestCardView / ChestPreviewMock 等都可填入；Preview 写 `HomeView(state: ..) { EmptyView() }` 简洁。
3. **避免预设 enum 字段**：若 HomeView 自带 `chestState` enum + 内部 switch 渲染 ChestCardView——HomeView 必须 import ChestCardView 类型；本 story 范围红线"禁 import APIClient/Repository/UseCase"延伸"禁 import 业务 composite 组件"。closure 让 HomeView 仅依赖 `ChestSlot: View` 抽象。

**否决**：
- **Color.clear 占位 + 后续 story 替换**：否决——v2.1 WARN 5 命中（不是真正接缝；Story 21.1 仍需改 HomeView body 嵌入 ChestCardView）。
- **EnvironmentObject 注入 ChestCardView**：否决——业务组件不应进 SwiftUI environment（namespace 污染）。

### Story 5.5 / 2.5 / 37.4 老测试兼容（详细列表）

本 story 改 HomeView signature → 部分老测试 / Preview / wire 必须修：

| 文件 | 行 | 老调用 | 修复路径 | 影响测试 |
|---|---|---|---|---|
| `HomeContainerView.swift` | 56-60 | `HomeView(viewModel:..., resetIdentityViewModel:..., sessionStore:...)` | 改 `HomeView(state:..., resetIdentityViewModel:..., sessionStore:...) { EmptyView() }` | RootViewWireTests 启动链路（间接走该 bridge） |
| `HomeView.swift` | 348 | `HomeView(viewModel: HomeViewModel())` Preview | 改 `HomeView(state: MockHomeViewModel()) { EmptyView() }` | 仅 Preview，无测试影响 |
| `HomeViewTests.swift` | 75 | `HomeView(viewModel: viewModel)` testHomeViewRendersOnSmallScreenWithoutCrash | 改 `HomeView(state: viewModel) { EmptyView() }`；保留语义 | testHomeViewRendersOnSmallScreenWithoutCrash + testHomeViewRendersOnLargeScreenWithoutCrash 两 case |
| `HomeUITests.swift` | testHomeViewShowsAllSixPlaceholders | 断言 chestArea / chestRemaining a11y identifier exists | 删除对 chestArea / chestRemaining 的断言；保留其它 5 个 | testHomeViewShowsAllSixPlaceholders（不改名） |

> **关键决策**：本 story 不预先改名 `testHomeViewShowsAllSixPlaceholders` → `testHomeViewShowsCoreAnchors`（保 git history 可读 + 让下一次 grep 该 case 名能找到 Story 2.2 / 2.5 / 5.5 / 37.4 的演进足迹）；只调整断言内容。

> **HomeViewModelLoadHomeTests / HomeViewModelPingTests**：完全不动（这些测 ViewModel 内部行为，签名不变；新增字段 / abstract method 不影响老 init 路径）。

### versionFooter 显示位置策略（保 Story 37.3 AC7 第 4 条红线）

Story 37.3 AC7 第 4 条钦定："ping/version 角落显示仍在 Home Tab 可见"——即 `AccessibilityID.Home.versionLabel` 元素必须在 HomeView 内可见。

**本 story 落地**：在 statusBar 内右下角小字（与老 versionFooter 同模式，但物理位置移到 statusBar 内 trailing 角而非 ScrollView 末尾），保持文案 `"v\(state.appVersion) · \(state.serverInfo)"`，a11y identifier 用 `AccessibilityID.Home.versionLabel`。

**为何不放 ScrollView 末尾**：home.jsx 钦定 ScrollView 末尾是 TeamIdleCard（不留 footer 空间）；statusBar 顶部右下角有"步数 capsule"右下空隙可放 12pt small caption 不破坏视觉。

**Preview 验证**：dev 在 Xcode Canvas 双主题 Preview 内目视确认 versionLabel 在 statusBar 区域可见（candy 主题深色字 / dark 主题浅色字，与 ink-soft 一致）。

### MockHomeViewModel 不调 super.onXxxTap()（避免触发 fatalError）

MockHomeViewModel 5 个 override 方法**不**调 `super.onCreateTap()` 等——基类 abstract method 是 `fatalError()`，super 调用直接 crash。MockHomeViewModel 自己实装"记录 invocation + 设状态"逻辑。

**对应 RealHomeViewModel 同模式**：override 方法内不 super 调用；自己实装占位 stub。

**未来扩展**：若需要"每个 override 方法都跑某个共享前置/后置逻辑"，应在基类加非 abstract 的 helper method（如 `protected func recordInvocation(_:)`）让子类调用，而非把基类 abstract 改成"默认空实现 + super hook"模式。

### showJoinModal 唯一 owner = HomeViewModel 基类（避免 state 双写）

老路径（Story 2.3 设计）：sheet binding 走 `@State private var showJoinModal: Bool = false` 在 HomeView 内；Modal `dismiss` callback 设回 false。

**本 story 路径**（v2.1 BLOCKER 10 命中后修复）：`showJoinModal` 唯一 owner = `HomeViewModel.@Published var showJoinModal: Bool`；HomeView 用 `.sheet(isPresented: $state.showJoinModal)` 双向绑定。

**为何走 ViewModel 持有**：

1. **跨子视图触发**：FriendsView "加入" 按钮也可能让 HomeView 弹 modal（v2 提案 Story 37.12 钦定）—— 这种跨 View 触发只能通过共享 ObservableObject（HomeViewModel）；@State 子视图本地不行。
2. **测试覆盖**：单元测试 `XCTAssertFalse(vm.showJoinModal); vm.onJoinTap(); XCTAssertTrue(vm.showJoinModal)` 直接断言；@State 在 SwiftUI body 内不可单独测试。
3. **state owner 单一**：v2.1 BLOCKER 10 决议——避免 SwiftUI bug "@State 重置但 @Published 不重置" 漂移。

### 5 区块视觉契约（详细 ui_design 翻译表）

按 `iphone/ui_design/source/screens/home.jsx` + `iphone/ui_design/README.md` §HomeScreen 像素级翻译：

#### statusBar（home.jsx:21-38）

```swift
// 视觉规则（home.jsx:22 钦定）
HStack(alignment: .center) {
    VStack(alignment: .leading, spacing: 2) {
        // weather: 12pt 600 weight, ink-soft
        Text(state.weather)
            .font(.system(size: 12, weight: .semibold))
            .foregroundColor(theme.colors.inkSoft)
        // greeting: 22pt 800 weight, ink, app-font
        Text(state.greeting)
            .font(.system(size: 22, weight: .heavy, design: .rounded))
            .foregroundColor(theme.colors.ink)
    }
    Spacer()
    // 步数 capsule: surface 背景 + 8pt vert 14pt horz padding + 圆角 20 + shadow-sm + border 1pt
    HStack(spacing: 6) {
        Image(systemName: Icons.symbol(for: "footprint"))
            .font(.system(size: 14))
            .foregroundColor(theme.colors.coin)
        Text("\(appState.currentStepAccount?.availableSteps ?? 0)")
            .font(.system(size: 15, weight: .heavy))
            .foregroundColor(theme.colors.ink)
        Text("步")
            .font(.system(size: 11, weight: .semibold))
            .foregroundColor(theme.colors.inkSoft)
    }
    .padding(.vertical, 8)
    .padding(.horizontal, 14)
    .background(
        Capsule().fill(theme.colors.surface)
    )
    .overlay(
        Capsule().stroke(theme.colors.border, lineWidth: 1)
    )
    .shadow(color: theme.shadow.sm.color, radius: theme.shadow.sm.radius, x: theme.shadow.sm.x, y: theme.shadow.sm.y)
    .accessibilityIdentifier(AccessibilityID.Home.stepBalance)
}
.padding(.top, 4)
.accessibilityIdentifier("homeStatusBar")
.accessibilityElement(children: .contain)   // 让 stepBalance / userInfo 子元素仍可定位
```

> **关键**：statusBar 容器同时挂 `homeStatusBar` (新) + `AccessibilityID.Home.userInfo` (老 Story 2.2 / 5.2 测试用) —— 双 a11y identifier 通过 `.accessibilityElement(children: .contain)` 共存（老 lesson 2026-04-26 SwiftUI 父容器 a11y identifier 默认传播覆盖子元素 / a11y contain + label 兼容）。

> **versionLabel** 不在 statusBar 内 inline，而是在 HomeView body 末尾另起一个小 `versionFooter` 区块（保留 Story 2.2 ScrollView 末尾右下小字模式；与新 statusBar 视觉互不干扰）。

#### catStage（home.jsx:40-81）

视觉关键值：
- 容器: `Card(cornerRadius: theme.radius.modalLg, padding: 20) { ... }`（28pt 圆角 + 20pt 内边距 + theme.colors.surface 背景 + theme.shadow.md 中阴影）
- ZStack 内含：① 背景斑点（accent-soft 50pt 圆 + 30pt 圆，opacity 0.4-0.5；fixed 位置）；② 等级名牌左上（accent 背景 + white 文字 12pt 700；显示 `"Lv.\(petLevel) · \(petName)"`，petLevel 暂时 mock=8（`HomeUser` 没 level 字段；mock 字面量）+ petName 来自 `appState.currentPet?.name ?? "默认小猫"` 用 `HomePetNameResolver.resolve(pet: appState.currentPet, hasHydrated: appState.currentUser != nil)`）；③ Image(systemName: "cat.fill") 中心 220pt size，theme.colors.inkSoft 色；④ 三状态条 HStack 底部（饱食/心情/活力，progress bar 0-100，颜色 warn/accent/success；mock 值来自 state.stats）；⑤ floatUp emoji 浮层（interactionAnimation = .flying(emoji) 时渲染）
- a11y identifier: `homeCatStage` 挂在 Card 容器 + `AccessibilityID.Home.petArea` 挂在 cat sprite Image + `AccessibilityID.Home.petName` 挂在等级名牌

#### actionRow（home.jsx:83-88）

```swift
HStack(spacing: theme.spacing.s10) {
    actionButton(label: "喂食", emoji: "🍥", iconKey: "bowl", id: "homeActionFeed", action: { state.onFeedTap() })
    actionButton(label: "抚摸", emoji: "💕", iconKey: "heart", id: "homeActionPet", action: { state.onPetTap() })
    actionButton(label: "玩耍", emoji: "⭐", iconKey: "ball", id: "homeActionPlay", action: { state.onPlayTap() })
}
```

actionButton 内部：`Card(cornerRadius: theme.radius.cardMd, padding: 0) { VStack { Image(systemName: Icons.symbol(for: iconKey)).foregroundColor(theme.colors.accentDeep) + Text(label).font(...) } }` 包在 Button 内 + `.accessibilityIdentifier(id)`。

> **不**用 `PrimaryButton` —— PrimaryButton 是圆药丸 + 单行 horizontal layout；ActionButton 是方形垂直 icon+label；两者视觉契约不同。actionButton 是 HomeView 内 inline 子视图（不抽 Primitives；属 Home Feature 私有 composite）。

#### chestSlot()

直接调用 ViewBuilder closure：
```swift
chestSlot()
```
本期传入 `EmptyView()` 占位；Story 21.1 传入 ChestCardView()，HomeView 内部代码 zero edit。

#### teamIdleCard（home.jsx:147-188）

视觉关键值：
- 容器: `LinearGradient(colors: [theme.colors.accent, theme.colors.accentDeep], startPoint: .topLeading, endPoint: .bottomTrailing)` + `RoundedRectangle(cornerRadius: theme.radius.cardLg)` (22pt 圆角) + `theme.shadow.md` 中阴影 + 18pt padding
- 内含: ① 装饰圆点（白色透明 0.1 / 0.08 圆，fixed 位置）；② 标题行 `Image(systemName: Icons.symbol(for: "paw")) + Text("和好友一起玩耍")`；③ 副标题 `Text("创建一个小屋，或用房间代码加入好友的队伍")`；④ HStack 两 PrimaryButton：
  - `PrimaryButton(title: "创建队伍", variant: .secondary, icon: Icons.symbol(for: "enter")) { state.onCreateTap() }` + `.accessibilityIdentifier("homeTeamIdleCard_create")`
  - `PrimaryButton(title: "加入队伍", variant: .ghost, icon: Icons.symbol(for: "enter")) { state.onJoinTap() }` + `.accessibilityIdentifier("homeTeamIdleCard_join")`

> **PrimaryButton variant 选择**：home.jsx 钦定"创建"按钮是白底 accent-deep 文字 + accent-deep emoji（即 secondary variant 视觉）；"加入"按钮是 22% 白透明背景 + white 文字 + white emoji（与 ghost variant 接近但底色规则不同——本 story 接受用 ghost variant，由 Story 37.13 visual-review-checklist 兜底视觉精度；不为此扩展 PrimaryButton variant 至 4 档）。

> **floatUp 动画（emoji 飘心）简化版**：interactionAnimation = .flying(emoji) 时，在 catStage 内 ZStack 上层渲染 `Text(emoji)` + `.offset(y: -110)` `.opacity(0)` 配合 `.animation(.easeOut(duration: 1.4))` —— 不严格匹配 home.jsx keyframe 0% / 25% / 100% 三阶段动画；视觉精度由 Story 37.13 visual-review-checklist 把关。完成动画后 ViewModel 自动重置 interactionAnimation = .idle（在 `.onChange(of: state.interactionAnimation)` 内 + `Task { try? await Task.sleep(nanoseconds: 1_400_000_000); state.interactionAnimation = .idle }`）。

### Theme.typography.mediumTitle vs ad-hoc system font（关键决策）

PrimaryButton 用 `theme.typography.mediumTitle.font`（17pt heavy）—— 与 Story 37.6 PrimaryButton 文件契约一致。HomeView statusBar greeting 用 `.system(size: 22, weight: .heavy, design: .rounded)` —— 22pt 800 weight 与 ui_design "大标题" 对齐；不走 theme.typography.title（避免引入 typography 字段加锁）。

> **关键决策**：本 story 接受 statusBar / catStage / actionRow 内**部分** font 用 inline `.system(size: ..., weight: ...)` 而非 theme.typography token——Story 37.5 typography token 字段不全（仅 mediumTitle / cardTitle / body / caption 4 档），不能覆盖 22pt heavy / 14pt 700 等场景；本 story 不强制扩 theme.typography（属 Story 37.5 / 37.13 范围）。

### MainTabView FloatingTabBar 不收口（关键约束）

Story 37.6 Dev Notes "MainTabView FloatingTabBar SF Symbol 与 Icons 表的一致性" 钦定：MainTabView FloatingTabBar 内 4 个 hardcode SF Symbol（house.fill / shippingbox.fill / person.2.fill / person.crop.circle.fill）与 Icons.mapping["home"/"box"/"friends"/"user"] 完全一致；Story 37.7 实装时**不**主动改 MainTabView。

**理由**：本 story 范围是 HomeView body / HomeViewModel 重构；MainTabView 是 4 Tab 容器层，由 Story 37.13 a11y 总表归并时统一收口（届时把 FloatingTabBar 改用 `Icons.symbol(for: "home")` 等查表 + 加 AccessibilityID.Tab.home 等常量）。

### EnvironmentKey 默认值的 fallback（与 Story 37.5 协调）

HomeView 内全部 `@Environment(\.theme) var theme` 取主题；`Environment+Theme.swift` 已落地 `defaultValue: Theme = .candy` fallback。Preview 显式 `.environment(\.theme, ThemeName.candy.theme)` 注入；Production RootView 注入 currentTheme.theme（Story 37.5 落地）。

### xcodegen regen 节奏 + project.yml 兼容性

`iphone/project.yml` 内 `targets.PetApp.sources: - PetApp` 通配；新增 5 个文件全部在 PetApp/Features/Home/ 下 → 自动 inclusion，**不**改 project.yml。dev 必须 `cd iphone && xcodegen generate` 重生成 `PetApp.xcodeproj`，commit pbxproj diff。

### 与 ADR-0002 §3.1 测试栈钦定的对齐

本 story 测试**仅**用 XCTest + @testable import PetApp + UITest（XCUIApplication）—— **不**引：

- ❌ SnapshotTesting（视觉 diff）：视觉验证靠 #Preview + Story 37.13 visual-review-checklist
- ❌ ViewInspector（SwiftUI body 内省）：HomeView body 渲染契约靠 ui_design 1:1 翻译 + Preview 抽样兜底
- ❌ Mockingbird / Cuckoo（mock codegen）：MockHomeViewModel 是手写 final class subclass

### 测试 case 数量取舍（≥4 / 实装 6 / 不再加）

epic AC line 4739-4743 钦定 ≥4 case；本 story 落地 6 case：
1. MockHomeViewModel 默认状态（greeting/weather/stats/interactionAnimation/showJoinModal/invocations 全部就位）
2. onCreateTap → invocations.append(.createTap)
3. onFeedTap → interactionAnimation = .flying("🍥") + invocation 顺序
4. onJoinTap → showJoinModal = true（与 epic AC line 4742 不同：epic 要 onCreateTap closure 触发，这里 onJoinTap 更代表 sheet 展开链路；本 story 保 onCreateTap + 加 onJoinTap 两 case 覆盖更稳）
5. PetStats(0, 0, 0) 不下溢 + 与 mockEmpty / zero 等价
6. RealHomeViewModel(appState:) 构造 + 5 abstract method override 不 crash（间接证 fatalError 路径未被命中）

**为何不加 HomeView body 渲染测试**：ADR-0002 §3.1 不允许 ViewInspector；body 视觉断言由 #Preview + UITest a11y identifier 兜底。

**为何不加 RealHomeViewModel 单字段断言**：RealHomeViewModel 各 override 方法在本 story 范围是占位 stub（行为简单），不需要逐方法逐字段细测；case#6 的整体 happy path 已覆盖。

### 与 Story 12.7 / 21.1 / 37.12 各下游 story 的协调（接缝清单）

本 story 落地后，下游 story 可立即按以下方式扩展：

- **Story 12.7（创建/加入/退出 UseCase + 主界面入口完善）**：
  - RealHomeViewModel.onCreateTap override 内调 CreateRoomUseCase（新加 `let createRoomUseCase: CreateRoomUseCaseProtocol` 字段；构造注入）
  - RealHomeViewModel.onJoinTap 不动（仅 `showJoinModal = true`；Modal `onConfirm` 闭包内调 JoinRoomUseCase）
  - 不改 HomeView 内部代码
- **Story 21.1（首页宝箱组件）**：
  - 调用方（HomeContainerHomeViewBridge）改传 `ChestCardView()` 替代 `EmptyView()`
  - HomeViewModel 加 `@Published var chestRemainingSeconds: Int = 0` 字段（Timer 驱动）
  - HomeView 内部代码 zero edit
- **Story 37.12（JoinRoomModal）**：
  - HomeView 内 `.sheet(isPresented: $state.showJoinModal) { JoinRoomModalPlaceholder() }` 改为 `.sheet { JoinRoomModal(onConfirm: { roomId in await ... }, onCancel: {}) }`
  - HomeViewModel 加 `func handleJoinSubmit(_ roomId: String) async { ... }` 让 RealHomeViewModel override 调 JoinRoomUseCase
- **Story 37.13（a11y 总表）**：
  - inline 字符串 `homeStatusBar` / `homeCatStage` / `homeActionFeed` / `homeActionPet` / `homeActionPlay` / `homeTeamIdleCard_create` / `homeTeamIdleCard_join` 收口到 `AccessibilityID.Home.statusBar` / `catStage` / `actionFeed` / `actionPet` / `actionPlay` / `teamIdleCardCreate` / `teamIdleCardJoin` 7 个新常量
  - 不改 HomeView body 视觉（仅常量名替换）

### Source tree components to touch

- 新建 production：
  - `iphone/PetApp/Features/Home/ViewModels/MockHomeViewModel.swift`
  - `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift`
  - `iphone/PetApp/Features/Home/Models/PetStats.swift`
  - `iphone/PetApp/Features/Home/Models/AnimationState.swift`
- 新建测试：
  - `iphone/PetAppTests/Features/Home/HomeViewScaffoldTests.swift`
- 修改：
  - `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`（去 final + 加 5 字段 + 5 abstract method）
  - `iphone/PetApp/Features/Home/Views/HomeView.swift`（generic struct 重写 + 5 区块）
  - `iphone/PetApp/Features/Home/Views/HomeContainerView.swift`（HomeContainerHomeViewBridge 修复 caller，line 56-60）
  - `iphone/PetAppTests/Features/Home/HomeViewTests.swift`（testHomeViewRendersOnSmallScreenWithoutCrash + testHomeViewRendersOnLargeScreenWithoutCrash 改 HomeView 调用签名）
  - `iphone/PetAppUITests/HomeUITests.swift`（testHomeViewShowsAllSixPlaceholders 调整 + 加 testHomeScaffoldShowsAllSevenAnchors）
  - `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen regen 结果）
- **不**改：
  - `iphone/PetApp/App/RootView.swift`（line 34 / 268 wire 路径不动）
  - `iphone/PetApp/App/MainTabView.swift`（FloatingTabBar 占位保 Story 37.13 收口）
  - `iphone/PetApp/App/AppState.swift`（Story 37.4 已锁字段 + hydrate 入口）
  - `iphone/PetApp/Features/Home/Models/HomeData.swift`（Story 5.5 / 37.4 已锁）
  - `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（Story 37.13 一次性归并 7 屏 a11y 常量；本 story 用 inline 字符串）
  - `iphone/PetApp/Core/DesignSystem/*`（Story 37.5 / 37.6 已锁；本 story 仅消费）
  - `iphone/project.yml`（通配规则自动 inclusion）

### Testing standards summary

- 测试入口：`bash iphone/scripts/build.sh --test`（ADR-0002 §3.4 钦定）
- 测试框架：XCTest only（ADR-0002 §3.1）；UITest 走 XCUIApplication（沿用 Story 2.2 / 2.5 既有 HomeUITests 风格）
- 单元测试位置：`iphone/PetAppTests/Features/Home/HomeViewScaffoldTests.swift`（与 production 镜像）
- 测试 case 数量：≥4 case（按 epic AC）；本 story 落地 6 case（含 Mock 默认状态 / 3 个 onTap 行为 / PetStats 零值 / RealHomeViewModel 构造）
- 测试运行时：每 case ≤ 50ms（纯 ViewModel 行为 + value type 构造）；UITest 单 case 5-15s（XCUIApplication 启动 + 元素查找）
- **不**测 SwiftUI View body 渲染（HomeView 是 struct，内部子视图 hosting 测试需要 ViewInspector）
- 覆盖目标：HomeViewModel 公开 API（5 abstract method 路径）+ MockHomeViewModel invocations + RealHomeViewModel 构造正常 + PetStats 数据契约

### Project Structure Notes

- Alignment with unified project structure: 完全按 `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §4 + ADR-0002 §3.3 的 `iphone/PetApp/Features/Home/` 目录约定。新增 `ViewModels/MockHomeViewModel.swift` / `ViewModels/RealHomeViewModel.swift` 与已存在 `ViewModels/HomeViewModel.swift` 同级；新增 `Models/PetStats.swift` / `Models/AnimationState.swift` 与已存在 `Models/HomeData.swift` 同级。Test mirror 路径 `iphone/PetAppTests/Features/Home/HomeViewScaffoldTests.swift` 与 production 严格镜像（已有 `PetAppTests/Features/Home/HomeViewModelTests.swift` / `HomeViewModelPingTests.swift` / `HomeViewModelLoadHomeTests.swift` 同模式）。
- Naming convention: `MockHomeViewModel` / `RealHomeViewModel`（PascalCase + 子类前缀）—— 与 Story 37.8-37.11 计划落地的 `MockRoomViewModel` / `RealRoomViewModel` / `MockWardrobeViewModel` 等 6 屏 6 ViewModel 命名风格统一。`PetStats` / `AnimationState` 是 Home Feature 私有 model 类型（不抽到 Shared）。
- Detected conflicts or variances: 无。`iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift` 已存在；本 story 改 final class → class 不破坏老 import 路径；新增子类与基类同目录下不冲突。

### References

- [Source: iphone/ui_design/source/screens/home.jsx] — HomeScreen 5 区块视觉源头（StatusBar / CatStage / ActionRow / TeamIdleCard，本 story 翻译为 SwiftUI generic struct + 子视图）
- [Source: iphone/ui_design/source/components/cat-placeholder.jsx] — CatPlaceholder 视觉占位（本 story 不直接落地 SVG 翻译；用 `Image(systemName: "cat.fill")` 占位，Story 30.x 后接 sprite）
- [Source: iphone/ui_design/README.md#HomeScreen] — HomeScreen 布局规则 + 关键交互
- [Source: iphone/ui_design/README.md#Design Tokens] — 5 类 token（colors/spacing/radius/shadow/typography），HomeView 通过 `@Environment(\.theme)` 取
- [Source: iphone/ui_design/README.md#Interactions] — floatUp 1.4s + Modal 出现 + Toast 钦定值
- [Source: \_bmad-output/planning-artifacts/epics.md#Story 37.7] — 本 story acceptance 原文（≥4 case 测试 / class 层次结构 / chestSlot 接缝 / 7 a11y identifier）
- [Source: \_bmad-output/planning-artifacts/sprint-change-proposal-2026-04-29-v2.md#Story 37.7] — class 层次（v2.2 修复 BLOCKER 7 codex review；不用 protocol any P 模式）+ generic struct + showJoinModal 唯一 owner
- [Source: \_bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md#3.2] — HomeContainerView 互斥状态机契约（idle ↔ inRoom）
- [Source: \_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md#3.1] — ViewModel 注入规则（构造注入 AppState；禁 @EnvironmentObject）
- [Source: \_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md#3.5] — HomeViewModel 演变模式（chestRemainingSeconds 留 Story 21.1；transient state 归 ViewModel）
- [Source: \_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.1] — XCTest only 测试框架钦定
- [Source: \_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.3] — `iphone/PetApp/` 目录约定 + xcodegen 通配规则
- [Source: \_bmad-output/implementation-artifacts/37-3-rootview-maintabview-改造.md] — RootView wire 改造原文（本 story 保不动）
- [Source: \_bmad-output/implementation-artifacts/37-4-appstate-实装-loadhome-迁移.md] — AppState hydrate 路径（本 story 通过 RealHomeViewModel 构造注入消费 AppState）
- [Source: \_bmad-output/implementation-artifacts/37-5-theme-design-tokens.md] — Theme 类型契约 + ThemeName 路由（本 story 通过 @Environment 消费）
- [Source: \_bmad-output/implementation-artifacts/37-6-shared-primitives.md] — Card / PrimaryButton / Icons.symbol(for:) 接缝（本 story TeamIdleCard 用 PrimaryButton；statusBar 步数计 prefix 用 Icons.symbol(for: "footprint")；ActionRow 用 Card 容器）
- [Source: iphone/PetApp/Features/Home/Views/HomeView.swift] — Story 2.2 / 5.2 / 5.5 / 37.4 老 HomeView 实装（本 story 重写 body）
- [Source: iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift] — Story 5.5 / 37.4 老 HomeViewModel（本 story 改 final class → class + 加字段/方法）
- [Source: iphone/PetApp/Features/Home/Views/HomeContainerView.swift#L56-L60] — HomeContainerHomeViewBridge HomeView 调用点（本 story 修复签名）
- [Source: iphone/PetApp/Features/Home/Views/JoinRoomModal/JoinRoomModalPlaceholder.swift] — Story 37.12 落地真实 modal 前的占位 stub（本 story 通过 .sheet 挂载）
- [Source: iphone/PetApp/Shared/Constants/AccessibilityID.swift] — Home enum 7 个常量（本 story 部分继续使用，部分在 chestSlot 接缝期不渲染但保留常量）
- [Source: docs/lessons/2026-04-26-swiftui-task-modifier-reentrancy.md] — SwiftUI .task 重启边界 lesson（本 story HomeViewModel.start / loadHome 短路 flag 路径不动）
- [Source: docs/lessons/2026-04-30-strong-vs-weak-for-constructor-injected-state.md] — RealHomeViewModel 持 AppState strong 引用同 lesson（不形成循环）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- `bash iphone/scripts/build.sh --test` (2026-04-30): 266 unit tests, 0 failures, BUILD SUCCESS. HomeViewScaffoldTests 6 cases 全部 passed (testMockHomeViewModelDefaultStateMatchesSpec / testOnCreateTapAppendsInvocation / testOnFeedTapTriggersFlyingEmojiAndInvocation / testOnJoinTapTogglesShowJoinModalToTrue / testPetStatsZeroValueDoesNotUnderflow / testRealHomeViewModelConstructionAndAbstractMethodsDoNotCrash).
- HomeViewTests 既有 2 case (testHomeViewRendersOnSmallScreenWithoutCrash / testHomeViewRendersOnLargeScreenWithoutCrash) 改用 MockHomeViewModel + 新 HomeView 签名（state: + ViewBuilder closure），全部 passed.
- grep 抽样校验：`class HomeViewModel` 1 处（去 final），`final class HomeViewModel` 0 处，`fatalError` 5 个 abstract method 落地，MockHomeViewModel 5 个 override，RealHomeViewModel 5 个 override。
- 双 a11y identifier 共存策略：statusBar 父容器挂 `AccessibilityID.Home.userInfo`（保 Story 2.8 testUserInfoBarRetainsNicknameAccessibilityLabel：a11y label = nickname）+ overlay 子元素挂 `homeStatusBar`（保 Story 37.7 AC8 7 锚 UITest 定位）.

### Completion Notes List

- HomeViewModel 改 `class`（去 final）+ 加 5 个 @Published 字段（greeting / weather / stats / interactionAnimation / showJoinModal）+ 5 个 abstract method（onCreateTap / onJoinTap / onFeedTap / onPetTap / onPlayTap，全部 fatalError 占位）；保留全部老公开 API 签名。
- MockHomeViewModel: HomeViewModel 子类（final class），含 invocations 数组 + 5 override（硬编码 mock 数据）。
- RealHomeViewModel: HomeViewModel 子类（final class），构造注入 AppState，5 override 占位 stub（os_log + 视觉态 mutation；不调任何 UseCase / Repository）。
- HomeView 改 `generic struct HomeView<ChestSlot: View>`，参数 viewModel → state，新增 chestSlot ViewBuilder closure 接缝；body 整段重写为 ZStack 含背景渐变 + ScrollView VStack 5 区块（statusBar / catStage / actionRow / chestSlot() / teamIdleCard）+ versionFooter；保留 SessionStore / ResetIdentityViewModel 注入。
- 7 个新 inline a11y identifier (homeStatusBar / homeCatStage / homeActionFeed / homeActionPet / homeActionPlay / homeTeamIdleCard_create / homeTeamIdleCard_join) + 4 个老常量保留 (AccessibilityID.Home.userInfo / petArea / petName / stepBalance / versionLabel) 物理位置变化，命名继续；chestArea / chestRemaining 在本期 chestSlot 接缝期不渲染（chestSlot 默认 EmptyView()）。
- PetStats / AnimationState 辅助类型落地（Models/）。
- HomeContainerView.HomeContainerHomeViewBridge.body 修复 caller：`HomeView(viewModel: ...)` → `HomeView(state: ...) { EmptyView() }`。
- HomeViewTests.swift 改用 MockHomeViewModel + 新签名调用（保 2 case 不改名）。
- HomeUITests.swift：testHomeViewShowsAllPlaceholders 去掉 chestArea / chestRemaining 断言；新增 testHomeScaffoldShowsAllSevenAnchors（7 锚定位，不主动点击）。
- HomeView Preview 改 `#Preview` macro 双主题（candy / dark），用 MockHomeViewModel；老 `HomeView_Previews: PreviewProvider` 移除。
- xcodegen 重生成 PetApp.xcodeproj/project.pbxproj（5 个新文件自动 inclusion，project.yml 通配规则不改）。
- 全 build verify 通过：`bash iphone/scripts/build.sh --test` → 266 unit tests / 0 failures / BUILD SUCCESS。

### File List

新增（生产）：
- `iphone/PetApp/Features/Home/ViewModels/MockHomeViewModel.swift`
- `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift`
- `iphone/PetApp/Features/Home/Models/PetStats.swift`
- `iphone/PetApp/Features/Home/Models/AnimationState.swift`

新增（测试）：
- `iphone/PetAppTests/Features/Home/HomeViewScaffoldTests.swift`

修改：
- `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`（去 final + 加 5 字段 + 5 abstract method）
- `iphone/PetApp/Features/Home/Views/HomeView.swift`（generic struct 重写 + 5 区块 body + 双 Preview）
- `iphone/PetApp/Features/Home/Views/HomeContainerView.swift`（HomeContainerHomeViewBridge 修复 caller）
- `iphone/PetAppTests/Features/Home/HomeViewTests.swift`（2 case 改用 MockHomeViewModel + 新签名）
- `iphone/PetAppUITests/HomeUITests.swift`（testHomeViewShowsAllPlaceholders 调整 + testHomeScaffoldShowsAllSevenAnchors 新增）
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen regen 自动结果）

## Change Log

| Date       | Change |
|------------|--------|
| 2026-04-30 | 初稿落地：Story 37.7 HomeView Scaffold + HomeViewModel class 层次（去 final）+ MockHomeViewModel / RealHomeViewModel 子类 + HomeView generic struct 重写 + chestSlot ViewBuilder closure 接缝 + PetStats / AnimationState 辅助类型 + 6 case 单元测试 + UITest 7 锚 + xcodegen regen + RootView wire 不动. Sprint-status: backlog → ready-for-dev. |
| 2026-04-30 | dev-story 完成：HomeViewModel 去 final + 加 5 字段 + 5 abstract method（fatalError 占位）；MockHomeViewModel + RealHomeViewModel 双子类落地；HomeView 改 generic struct + chestSlot ViewBuilder closure 接缝 + 5 区块视觉重写（statusBar / catStage / actionRow / chestSlot() / teamIdleCard + versionFooter）；7 个 a11y 锚 inline 字符串 + 老 5 个常量保留物理位置变化命名继续；PetStats / AnimationState 辅助类型落地；HomeContainerView caller 修复；HomeViewTests 2 case 改 MockHomeViewModel + 新签名；HomeUITests 调整 testHomeViewShowsAllPlaceholders + 新增 testHomeScaffoldShowsAllSevenAnchors；HomeView Preview 改 #Preview macro 双主题；xcodegen regen pbxproj；`bash iphone/scripts/build.sh --test` 266 unit tests 全绿（含 6 case HomeViewScaffoldTests）；BUILD SUCCESS. Sprint-status: ready-for-dev → in-progress → review. |
