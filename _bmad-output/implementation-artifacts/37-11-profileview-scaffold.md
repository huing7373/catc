# Story 37.11: ProfileView Scaffold + ProfileViewModel class 层次（含微信绑定卡 + Modal 视觉）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want Profile Tab 显示 ui_design 高保真我的页面（顶部渐变头图 + 头像 + 用户名/ID/称号/加入药丸 + 4 列统计卡 + 微信绑定卡 双状态 + 最近收藏横向滑窗 + 4 项菜单列表 + 绑定 Modal 警告视觉壳）+ 接缝设计支持后续 epic（真实微信 OAuth / 收藏数据 / 成就消息等）注入,
so that 既有视觉壳又有可持续接缝（ProfileScaffoldView 内部代码 zero edit 让后续真行为接入），同时把 Story 37.3 落地的 `ProfileView` 占位 stub 替换为 ui_design `profile.jsx` + `wechat_binding.md` 像素级匹配的高保真 Scaffold（**全部按钮均 toast 占位 + 不调真 OAuth**）。

## 故事定位（Epic 37 第四层第 5 条 story；Scaffold 主体 6 屏并行链路最后一条，与 37.7 / 37.8 / 37.9 / 37.10 同模式）

这是 Epic 37「iPhone 架构层重构 + UI Scaffold（壳先行）」的**第四层 story**——上游 37.3（MainTabView 已挂 ProfileView 占位 stub）/ 37.4（AppState.currentUser / currentPet 字段就绪）/ 37.5（Theme）/ 37.6（primitives 含 Avatar / Icons.wechat / Icons.shield / Icons.warn / Icons.bell / Icons.settings / Icons.diamond / Icons.friends / Icons.paw / Icons.trophy / Icons.heart / Icons.chevronRight / Icons.sparkle）全部 done；37.7 / 37.8 / 37.9 / 37.10 已用「class 层次 + Mock/Real 两子类 + ScaffoldDefaults seed + sink 派生 + 同步 onAppear bind + Real override 必 mutate state」模式落地，**本 story 1:1 复刻该模式于 ProfileView**。本 story 是 **UI Scaffold 主体** 类——属于 Epic 37 §AC 红线的「数据完全 mock + 禁 import APIClient/Repository/UseCase + 视觉像素级匹配 ui_design + 主题用 `@Environment(\.theme)` 取 token + 测试不引 SnapshotTesting/ViewInspector + 通过 `bash iphone/scripts/build.sh --test`」适用范围。

**本 story 落地后立即解锁**：
- Epic 37 Scaffold 主体（37.7/37.8/37.9/37.10/37.11）全部 done → Story 37.12（JoinRoomModal + 跨屏 join）/ 37.13（a11y 总表）/ 37.14（design-package 白名单）可启动
- 后续 epic（微信绑定真行为 / 收藏数据 / 成就 / 消息）—— 本 story 全部按钮均"占位 toast"（`lastToastMessage` 写入），Real 子类 override 默认实装本地 mutate 占位行为，让 production app 立刻可用；后续 epic 把 Real 占位行为替换为真实 UseCase 调用（如 `BindWechatUseCase` / `LoadCollectionsUseCase` / `LoadAchievementsUseCase` 等），ProfileScaffoldView 视图内部 zero edit
- Story 37.13（accessibility identifier 总表）—— ProfileScaffoldView 全部 a11y identifier 来源；本 story 在 ProfileScaffoldView 内 inline 字符串（`profileView` / `profileWeChatCard` / `profileWeChatModal` / `profileWeChatBindButton` / `profileMenu_<idx>` 等），Story 37.13 收口归并到 `AccessibilityID.Profile`

**本 story 的"实装"动作**（一句话概括）：在 `iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift`（新建文件，**不**改 `ProfileView.swift` —— 见 Dev Notes "ProfileView 占位 stub 不删保 git history"）落地 struct `ProfileScaffoldView` + `@ObservedObject var state: ProfileViewModel`（基类直接，**非泛型 state**）；新建基类 `class ProfileViewModel: ObservableObject`（class 而非 final 让子类可继承）+ 5 个 `@Published` 字段（`profile: ProfileSummary / wechatBound: Bool / recentCollections: [RecentCollection] / showBindModal: Bool / lastToastMessage: String?`）+ 5 个 abstract method（`onWeChatCardTap()` / `onWeChatBindConfirmTap()` / `onWeChatModalDismissTap()` / `onMenuTap(item:)` / `onCollectionViewAllTap()`）+ 0 个 concrete view-action method（**说明**：Profile 域所有按钮都是"占位 toast"行为，没有"切 Tab"这种纯 view-state 动作 —— 故 0 个 concrete method）+ 0 个 derived computed property（profile 字段已是聚合 value type，不需要派生计算）；新建 `MockProfileViewModel: ProfileViewModel` 子类（硬编码 mock + override 5 个 abstract method 改本地 wechatBound / showBindModal / lastToastMessage + invocations 数组）+ `RealProfileViewModel: ProfileViewModel` 子类骨架（构造注入 AppState + parameterless init + bind(appState:) + sink 订阅 appState.$currentUser + appState.$currentPet 派生 profile 字段；override 5 个 abstract method 实装本地占位 mutate（与 Mock 同语义，按 Story 37.9 round 1 P1 lesson 钦定 `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`））；新建 `ProfileSummary` value type（id / name / title / joinedAt / petName / petLevel / collectionsCount / friendsCount / achievementsCount / coinsCount）+ `RecentCollection` value type（id / name / rarity / emoji）+ `ProfileMenuItem` enum（achievements / messages / favorites / settings）+ `ProfileScaffoldDefaults` 共享 enum（按 Story 37.8 / 37.9 / 37.10 P2 lesson 钦定路径，Mock / Real 双 init 都用它 seed）。`ProfileView` 内 `body` 的占位 Text 替换为 `ProfileScaffoldView(state: profileViewModel)` 真实 Scaffold（caller 漏改靠编译器报错驱动；与 Story 37.3 / 37.7 / 37.8 / 37.9 / 37.10 同精神）；RootView 加 `@StateObject profileViewModel: ProfileViewModel = RealProfileViewModel()` + `.environmentObject(profileViewModel)`；`.onAppear` 内同步 bind appState（防 launch-time race，按 Story 37.8 round 2 P2 lesson 钦定路径）。

**关键路径："新建" + caller 替换（与 Story 37.10 同精神：本 story 是新建 + 替换占位）**：

- `ProfileView.swift` **不删除**（保 Story 37.3 git history 可读 + 让人对比演进足迹；与 Story 37.9 / 37.10 同精神）；仅在 `ProfileView.swift` 内 `body` 的 `Text("Profile Tab Placeholder")` 替换为 `ProfileScaffoldView(state: profileViewModel)` —— `ProfileView` 类型本身保留作为 MainTabView 直接 instantiate 的入口 view
- `profileViewModel: ProfileViewModel` 注入路径走与 HomeView / RoomView / WardrobeView / FriendsView 相同模式：RootView 内 `@StateObject private var profileViewModel: ProfileViewModel = RealProfileViewModel()`（用 RealProfileViewModel 而非裸 ProfileViewModel 防生产 fatalError 路径，按 Story 37.7 round 1 P1 lesson 钦定）+ `.environmentObject(profileViewModel)`；`ProfileView` 内 `@EnvironmentObject var profileViewModel: ProfileViewModel` 取出后传给 `ProfileScaffoldView(state:)` 子视图
- `RootView` 同步 `.onAppear { ... }` 内追加 `if let realProfileVM = profileViewModel as? RealProfileViewModel { realProfileVM.bind(appState: appState) }`（防 launch-time race；Story 37.8 round 2 P2 lesson 钦定 `.onAppear` 而非 `.task`）
- `LaunchedContentView` 透传 `profileViewModel: ProfileViewModel` 字段（与已有 `homeViewModel` / `roomViewModel` / `wardrobeViewModel` / `friendsViewModel` 同模式），`.environmentObject(profileViewModel)` 注入 ready 子树
- `MainTabView.swift` Preview 块追加 `.environmentObject(MockProfileViewModel() as ProfileViewModel)`（与已有 4 个 Preview 注入同模式）

**不涉及**（红线）：
- **不**实装真实微信 OAuth（`WXApi.sendAuthReq` / `WechatService` 都不引；本 story `onWeChatBindConfirmTap` 仅写 `wechatBound = true` + toast "微信绑定（敬请期待）" 占位 mutate；后续 epic 真实 OAuth 落地时改 RealProfileViewModel）
- **不**实装"进入 Profile 1.2 秒自动弹绑定 Modal"行为（ui_design `profile.jsx` 的 useEffect timer 行为；本期 **不落** 该 timer —— 让用户手动点"绑定微信卡"才弹 Modal；理由见 Dev Notes "1.2s 自动弹 Modal timer 决策"）
- **不**实装 `@AppStorage("wechatBound")` / `@AppStorage("lastWechatPromptAt")` 持久化（本 story 数据完全 mock + ViewModel @Published；后续 epic 真持久化路径再加）
- **不**实装"已绑定/未绑定"两态切换 UI 中的"已绑定卡"真实数据流转（`wechatBound` 字段 mock 默认 false；user 点绑定后切到 true 让"已绑定卡"渲染；后续 epic 接 server 真实状态字段）
- **不**实装 toast 自动消失 timer（与 Story 37.10 同精神，`lastToastMessage` 写入即覆盖；本 story ViewModel 不实装 timer）
- **不**接 server 真实用户/收藏/成就接口（本 epic 全 mock；后续 epic 真实 API 落地时改 RealProfileViewModel sink）
- **不**改 RootView `@StateObject` wire 切到基类 ProfileViewModel（与 RealHomeViewModel / RealRoomViewModel / RealWardrobeViewModel / RealFriendsViewModel 一致都用 Real 子类避免 fatalError）
- **不**改 AppState / HomeData / HomePet / HomeUser / HomeEquip 类型（Story 37.4 已锁定）
- **不**实装 NavigationLink push 到具体子页面（成就 / 消息 / 喜欢的道具 / 设置 4 个菜单点击仅 toast；后续 epic 实装具体子页面）
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3）
- **不**改 server/ 任何文件
- **不**收口 `AccessibilityID.Profile` 常量（本 story inline 字符串；Story 37.13 一次性归并所有 7 屏 a11y identifier）
- **不**删除 `ProfileView.swift`（保 git history；下游 Story 37.13 决定）
- **不**预先生成 `ProfileSummary` / `RecentCollection` / `ProfileMenuItem` 之外的额外 helper / mapping 类型
- **不**实装 BindWechatModal 内的「数据风险列表」（4 行 🐱 / 💎 / 🏆 / 👥）使用真实数据 —— 本 story 走硬编码占位（与 ProfileSummary mock 字段值一致；后续 epic 真接用户数据时再 bind）

## Acceptance Criteria

> **AC 编号体系**：AC1 是 ProfileViewModel class 层次基类（class + 5 字段 + 5 abstract method + 0 concrete view-action method + 0 derived computed property）；AC2 是 MockProfileViewModel / RealProfileViewModel 两子类（**Real override 必须本地 mutate state**，按 Story 37.9 round 1 P1 lesson 钦定）；AC3 是 ProfileSummary / RecentCollection / ProfileMenuItem 值类型 + ProfileScaffoldDefaults 共享 enum；AC4 是 ProfileScaffoldView struct + 5 区块视觉（顶部渐变头图 / 统计卡 / 微信绑定卡（双态切换）/ 最近收藏横向滑窗 / 菜单列表）+ BindWechatModal sheet；AC5 是 ProfileView caller 替换 + RootView wire + LaunchedContentView 透传 + MainTabView Preview 注入；AC6 是 #Preview 双主题 × 多场景；AC7 是单元测试 ≥10 case（≥3 epic AC line 4837 钦定 + 守护 case 防 lesson 反例）；AC8 是 UITest a11y 定位关键锚；AC9 是 build verify；AC10 是 Deliverable 清单。

### AC1 — 新建 ProfileViewModel 基类（class 层次 + 5 字段 + 5 abstract method + 0 concrete view-action method）

**新建文件**：`iphone/PetApp/Features/Profile/ViewModels/ProfileViewModel.swift`

**类签名**（class 而非 final，让 Mock/Real 子类可继承；与 HomeViewModel Story 37.7 / RoomViewModel Story 37.8 / WardrobeViewModel Story 37.9 / FriendsViewModel Story 37.10 同精神）：

```swift
// ProfileViewModel.swift
// Story 37.11 AC1: ProfileScaffoldView 基类 ViewModel（class 层次 + 5 字段 + 5 abstract method + 0 concrete method）.
//
// 设计：与 HomeViewModel / RoomViewModel / WardrobeViewModel / FriendsViewModel 同精神（class 而非 final + abstract method 用 fatalError 强制 override）.
// 字段范围：5 字段（profile / wechatBound / recentCollections / showBindModal / lastToastMessage）.
// 后续 epic RealProfileViewModel 子类扩 onWeChatBindConfirmTap 调真实微信 OAuth UseCase / 走 setWeChatBound（不在本 story 范围）.

import Foundation
import Combine

@MainActor
public class ProfileViewModel: ObservableObject {
    /// 用户聚合资料卡（顶部渐变头图 + 统计卡共用数据源）.
    /// 派生源：appState.currentUser + appState.currentPet（RealProfileViewModel 通过 sink 订阅派生；MockProfileViewModel 用本地直写）.
    /// **关键约束**：profile 是 view-specific aggregated value type（**不进 AppState** —— 详见 ADR-0010 §3.2 表格
    /// "聚合卡片字段 → ViewModel 持有"; AppState 的 currentUser / currentPet 是 raw domain state，
    /// profile 是 ViewModel 视图聚合，两者职责分离；与 Story 37.10 friends 字段语义同精神）.
    @Published public var profile: ProfileSummary = ProfileScaffoldDefaults.profile

    /// 微信绑定状态（决定"绑定微信卡"渲染未绑定/已绑定双态分支）.
    /// 后续 epic 真实持久化（如 @AppStorage / server 字段）落地时改派生源；本期 ViewModel 持有.
    @Published public var wechatBound: Bool = false

    /// 最近收藏横向滑窗数据（mock 5 件最近开箱）.
    /// 后续 epic 真接 LoadRecentCollectionsUseCase 时改 sink 派生；本期 ScaffoldDefaults seed.
    @Published public var recentCollections: [RecentCollection] = []

    /// "绑定微信 Modal" 显隐状态（用户点"绑定微信卡"或"立即绑定"按钮触发显示）.
    /// 这是 view-specific transient state（按 ADR-0010 §3.2 表格"modal 显隐 → ViewModel @Published"）；
    /// 单元测试需要断言（case#2 卡点击后 showBindModal == true）→ 不能放 SwiftUI @State.
    @Published public var showBindModal: Bool = false

    /// 最近一次 toast 消息（占位 toast 系统；视觉走 ProfileScaffoldView 简单 overlay；详见 AC4 toast 渲染策略）.
    /// 用户可通过其他动作隐式清空（写新值即覆盖；nil 表示无 toast）；
    /// 本 story 不实装"3 秒自动消失"等 timer 行为（保留给后续 epic；与 Story 37.10 同精神）.
    @Published public var lastToastMessage: String?

    public init() {}

    // MARK: - abstract method（基类 fatalError 占位，子类必 override）

    /// "绑定微信卡"整张卡可点击触发（未绑定状态下点卡触发 modal 弹出）.
    /// MockProfileViewModel: 写 showBindModal = true + 记录 invocation.
    /// RealProfileViewModel（本 story 占位）: **本地 mutate** —— 写 showBindModal = true + 记录 lastToastMessage（可选）.
    ///   按 Story 37.9 round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`
    ///   钦定路径：Real 子类 override 必须实装本地 mutate state 让 production app 立即视觉反馈；不能只 log.
    /// 后续 epic: RealProfileViewModel 改调"显示 Modal" + 接入 @AppStorage("lastWechatPromptAt") 24 小时再次弹一次逻辑.
    public func onWeChatCardTap() {
        fatalError("ProfileViewModel.onWeChatCardTap must be overridden by subclass")
    }

    /// Modal 内"绑定微信，保护数据"按钮触发.
    /// MockProfileViewModel: 写 wechatBound = true + showBindModal = false + lastToastMessage = "微信绑定成功（mock）" + 记录 invocation.
    /// RealProfileViewModel（本 story 占位）: **本地 mutate** —— 写 wechatBound = true + showBindModal = false + lastToastMessage = "微信绑定（敬请期待）".
    ///   按 Story 37.9 round 1 P1 lesson 钦定路径.
    /// 后续 epic: 改调 BindWechatUseCase / WXApi.sendAuthReq 拉起授权 → 后端换 OpenID → server 写入 → 成功后 setWeChatBound(true).
    public func onWeChatBindConfirmTap() {
        fatalError("ProfileViewModel.onWeChatBindConfirmTap must be overridden by subclass")
    }

    /// Modal 内"稍后再说"按钮 / Modal 遮罩 / 关闭按钮触发.
    /// MockProfileViewModel: 写 showBindModal = false + 记录 invocation.
    /// RealProfileViewModel（本 story 占位）: **本地 mutate** —— 写 showBindModal = false.
    ///   按 Story 37.9 round 1 P1 lesson 钦定路径.
    /// 后续 epic: 加入 @AppStorage("lastWechatPromptAt") 时间戳记录"已 dismiss"语义.
    public func onWeChatModalDismissTap() {
        fatalError("ProfileViewModel.onWeChatModalDismissTap must be overridden by subclass")
    }

    /// 菜单列表 4 项（成就徽章 / 消息通知 / 喜欢的道具 / 设置）任一点击触发.
    /// MockProfileViewModel: 写 lastToastMessage = "{item.label}（敬请期待）" + 记录 invocation(menuTap(item:)).
    /// RealProfileViewModel（本 story 占位）: **本地 mutate** —— 写 lastToastMessage = "{item.label}（敬请期待）".
    ///   按 Story 37.9 round 1 P1 lesson 钦定路径.
    /// 后续 epic: 改 NavigationLink push 到具体子页面（AchievementsView / MessagesView / FavoritesView / SettingsView）.
    public func onMenuTap(item: ProfileMenuItem) {
        fatalError("ProfileViewModel.onMenuTap must be overridden by subclass")
    }

    /// "最近收藏" SectionHeader 右侧"查看全部"按钮触发.
    /// MockProfileViewModel: 写 lastToastMessage = "查看全部收藏（敬请期待）" + 记录 invocation.
    /// RealProfileViewModel（本 story 占位）: **本地 mutate** —— 同 Mock.
    /// 后续 epic: 改 NavigationLink push 到 AllCollectionsView 全部收藏页.
    public func onCollectionViewAllTap() {
        fatalError("ProfileViewModel.onCollectionViewAllTap must be overridden by subclass")
    }
}
```

> **关键决策 1**：abstract method 用 `fatalError` 而非 default empty body —— 与 HomeViewModel / RoomViewModel / WardrobeViewModel / FriendsViewModel 同精神（让漏 override 立刻 crash + 测试覆盖逻辑路径）。

> **关键决策 2**：**0 个 concrete view-action method**（与 FriendsViewModel.selectTab 不同）—— Profile 域所有按钮都是"占位 toast / 切换 Modal 显隐"行为，没有"切 Tab / 切分类"这种纯 view-state 动作；任何按钮触发都需要 Mock vs Real 行为分化（占位 mutate vs 真实 UseCase）→ 全部 abstract。

> **关键决策 3**：**0 个 derived computed property**（与 FriendsViewModel.onlineCount 不同）—— `ProfileSummary` value type 已是聚合（`collectionsCount` / `friendsCount` / `petLevel` 都是 raw int 字段，统计卡直接读 `state.profile.collectionsCount` 等）；不需要从 `friends`/`collections` 数组动态算总数。如果未来需要 derived（例如 `unreadMessagesCount`），再加。

> **关键决策 4**：`profile` 是 `@Published` 字段（不是 derived computed property）—— 因为 RealProfileViewModel 通过 sink 订阅 `appState.$currentUser` + `appState.$currentPet` 派生写入，需要让 SwiftUI 监听变化；放 `@Published` 让 view 层 `state.profile.name` 等自动响应。Mock 子类直接写 @Published 触发渲染。

> **关键决策 5**：`profile` 字段**默认值**用 `ProfileScaffoldDefaults.profile`（而非 `ProfileSummary.empty` 之类的"空态"）—— 与 RealRoomViewModel.hostCatName 走 RoomScaffoldDefaults 同精神；让 launch / hydrate 前 / reset 后任何 path 渲染 ProfileScaffoldView 都立刻有 mock 数据占位（避免渲染空姓名 / 空头像 / "Lv.--" 等）。lesson 4 守护。

> **基类无参 init 兼容路径**：与 HomeViewModel / RoomViewModel / WardrobeViewModel / FriendsViewModel 同精神 —— RootView 走 RealProfileViewModel 子类，**不**走基类无参 init；基类 5 个 abstract method 在生产 wire 路径下不会被调；Preview / UITest 走 MockProfileViewModel。

**对应 Tasks**: Task 1.1, 1.2


### AC2 — 新建 MockProfileViewModel / RealProfileViewModel 两子类（独立文件）

**新建文件**: `iphone/PetApp/Features/Profile/ViewModels/MockProfileViewModel.swift`

```swift
// MockProfileViewModel.swift
// Story 37.11 AC2: ProfileViewModel mock 子类，用于 #Preview / UITest skip-guest-login / Scaffold 单元测试.
//
// 设计：
//   - 硬编码 mock 数据（profile / wechatBound / recentCollections / showBindModal 全量；走 ProfileScaffoldDefaults seed）
//   - override 5 个 abstract method 改本地状态 + 记录 invocations 数组
//   - **不**依赖 AppState（Mock 路径走纯 ViewModel-only 数据）
//
// 与 MockHomeViewModel Story 37.7 / MockRoomViewModel Story 37.8 / MockWardrobeViewModel Story 37.9 / MockFriendsViewModel Story 37.10 同模式（invocations 数组而非 closure spy）.

import Foundation
import Combine
import os.log

@MainActor
public final class MockProfileViewModel: ProfileViewModel {
    /// 单元测试用：记录所有方法调用
    public enum Invocation: Equatable {
        case wechatCardTap
        case wechatBindConfirmTap
        case wechatModalDismissTap
        case menuTap(item: ProfileMenuItem)
        case collectionViewAllTap
    }

    @Published public var invocations: [Invocation] = []

    /// 默认构造 — 走 ProfileScaffoldDefaults seed 全量字段.
    public override init() {
        super.init()
        self.profile = ProfileScaffoldDefaults.profile
        self.wechatBound = ProfileScaffoldDefaults.wechatBound  // false
        self.recentCollections = ProfileScaffoldDefaults.recentCollections
        self.showBindModal = false
        self.lastToastMessage = nil
    }

    /// 测试 / Preview 灵活构造 — 可注入任意字段值.
    public init(
        profile: ProfileSummary = ProfileScaffoldDefaults.profile,
        wechatBound: Bool = ProfileScaffoldDefaults.wechatBound,
        recentCollections: [RecentCollection] = ProfileScaffoldDefaults.recentCollections,
        showBindModal: Bool = false
    ) {
        super.init()
        self.profile = profile
        self.wechatBound = wechatBound
        self.recentCollections = recentCollections
        self.showBindModal = showBindModal
        self.lastToastMessage = nil
    }

    // MARK: - override abstract methods

    public override func onWeChatCardTap() {
        os_log(.debug, "MockProfileViewModel.onWeChatCardTap")
        invocations.append(.wechatCardTap)
        showBindModal = true
    }

    public override func onWeChatBindConfirmTap() {
        os_log(.debug, "MockProfileViewModel.onWeChatBindConfirmTap")
        invocations.append(.wechatBindConfirmTap)
        // Mock 路径行为：写 wechatBound = true + 关 modal + toast
        wechatBound = true
        showBindModal = false
        lastToastMessage = "微信绑定成功（mock）"
    }

    public override func onWeChatModalDismissTap() {
        os_log(.debug, "MockProfileViewModel.onWeChatModalDismissTap")
        invocations.append(.wechatModalDismissTap)
        showBindModal = false
    }

    public override func onMenuTap(item: ProfileMenuItem) {
        os_log(.debug, "MockProfileViewModel.onMenuTap %{public}@", item.rawValue)
        invocations.append(.menuTap(item: item))
        lastToastMessage = "\(item.label)（敬请期待）"
    }

    public override func onCollectionViewAllTap() {
        os_log(.debug, "MockProfileViewModel.onCollectionViewAllTap")
        invocations.append(.collectionViewAllTap)
        lastToastMessage = "查看全部收藏（敬请期待）"
    }
}
```

**新建文件**: `iphone/PetApp/Features/Profile/ViewModels/RealProfileViewModel.swift`

```swift
// RealProfileViewModel.swift
// Story 37.11 AC2: ProfileViewModel 生产实装子类（构造注入 AppState；override 5 个 abstract method 占位 mutate）.
//
// 范围（本 story 占位；后续 epic 填充真实 UseCase 调用）：
//   - 构造注入 AppState（按 ADR-0010 §3.1 ViewModel 注入规则）+ parameterless init() 走 bind(appState:)
//   - override 5 个 abstract method：本地 mutate showBindModal / wechatBound / lastToastMessage（占位）
//     按 Story 37.9 round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`：
//     Real 子类 override 必须实装"本 story 范围内能让 UI 视觉工作的最小 placeholder 行为"，禁止只 log.
//   - sink 订阅 appState.$currentUser + appState.$currentPet → 派生 profile 字段
//
// **不**调用任何 UseCase / Repository / APIClient（Epic 37 红线：UI Scaffold 数据完全 mock）.
// **不**调用 WechatService / WXApi（本 story 占位 wechatBound = true 即可；后续 epic 真 OAuth 落地）.
// **不**订阅真实收藏 / 成就接口（后续 epic 落地；本 story RealProfileViewModel.recentCollections 走 ScaffoldDefaults seed）.
//
// Story 37.7 / 37.8 / 37.9 / 37.10 沉淀 lesson 预防性应用（**不重蹈覆辙**）：
//   - lesson 1 `2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md`：
//     RootView `@StateObject profileViewModel` 用 `RealProfileViewModel()` 而非基类 `ProfileViewModel()`.
//   - lesson 4 `2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md`：
//     两条 init 路径都走 `ProfileScaffoldDefaults` seed —— 让 launch / hydrate 前 / reset 后任何
//     Real path 都立刻有 mock profile 占位（不让 ProfileScaffoldView 渲染空头像 / 空姓名 / "Lv.--"）.
//   - lesson 2 `2026-04-30-published-derived-state-needs-publisher-subscription.md`：
//     派生 state 用 sink 路径而非一次性 hydrate —— profile 订阅 appState.$currentUser + $currentPet；
//     reset 路径（appState.reset() 把 currentUser/Pet 置 nil）也能即时反映到字段（不残留旧值）.
//   - lesson 3 `2026-04-30-room-host-name-must-not-derive-from-local-current-pet.md`（**反向应用**）：
//     Profile 域 profile.name / profile.petName 派生自 appState.currentUser / currentPet **是合法**的 ——
//     "我的资料"语义就是本地用户自己的资料（与 Friends 域 currentRoomId 派生同理；与 Story 37.8 hostCatName 反例不冲突 —— 那是"看别人房间"语境）.
//   - lesson 5 `2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md`：
//     RootView `.onAppear` 内同步 bind appState（不放 .task）.
//   - lesson 6 `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`（**关键**）：
//     5 个 override **必须本地 mutate state**（与 Mock 同语义）：
//       · onWeChatCardTap → showBindModal = true
//       · onWeChatBindConfirmTap → wechatBound = true + showBindModal = false + lastToastMessage
//       · onWeChatModalDismissTap → showBindModal = false
//       · onMenuTap(item:) → lastToastMessage = "{item.label}（敬请期待）"
//       · onCollectionViewAllTap → lastToastMessage = "查看全部收藏（敬请期待）"

import Foundation
import Combine
import os.log

@MainActor
public final class RealProfileViewModel: ProfileViewModel {
    /// 构造注入 AppState（与 RealHomeViewModel / RealRoomViewModel / RealWardrobeViewModel / RealFriendsViewModel 同模式）.
    private var appState: AppState?

    /// 派生 state sink 句柄（防多次 bind 重订阅 + 持有 cancellable 让 sink 存活）.
    private var profileSubscriptions: Set<AnyCancellable> = []

    /// parameterless init —— RootView `@StateObject` 老模式可用; AppState 通过 bind 异步注入.
    /// 按 Story 37.8 / 37.9 / 37.10 round 1 P2 lesson 预防性应用：seed 5 字段全部走 ProfileScaffoldDefaults,
    /// 让 launch / hydrate 前 / reset 后任何走 Real path 都立刻有 mock 占位.
    /// 注：必写 `override` —— 基类 ProfileViewModel 有显式 `public init() {}`（与 RoomViewModel / WardrobeViewModel / FriendsViewModel 同模式）.
    public override init() {
        super.init()
        self.appState = nil
        self.profile = ProfileScaffoldDefaults.profile
        self.wechatBound = ProfileScaffoldDefaults.wechatBound
        self.recentCollections = ProfileScaffoldDefaults.recentCollections
        self.showBindModal = false
        self.lastToastMessage = nil
    }

    public init(appState: AppState) {
        super.init()
        self.appState = appState
        // round 1 P2 fix：先 seed scaffold defaults（让 sink 还没派发前 ProfileScaffoldView 有数据可渲染）.
        self.profile = ProfileScaffoldDefaults.profile
        self.wechatBound = ProfileScaffoldDefaults.wechatBound
        self.recentCollections = ProfileScaffoldDefaults.recentCollections
        self.showBindModal = false
        self.lastToastMessage = nil
        // 构造路径已注入 AppState；立即订阅派生.
        subscribeProfile(to: appState)
    }

    /// AppState 异步注入入口（与 RealHomeViewModel / RealRoomViewModel.bind / RealWardrobeViewModel.bind / RealFriendsViewModel.bind 同模式）.
    public func bind(appState: AppState) {
        let alreadySubscribed = !profileSubscriptions.isEmpty
        self.appState = appState
        guard !alreadySubscribed else { return }
        subscribeProfile(to: appState)
    }

    /// 订阅 appState.$currentUser + appState.$currentPet → 合并派生 profile 字段（保留 mock 字段如 collectionsCount / achievementsCount 等本地 cache，仅覆盖 name / id / petName / petLevel 等真实字段）.
    /// **关键**：profile 派生源是合法的 —— Profile 域语义就是"我的资料"（本地用户自己的资料），
    /// appState.currentUser / currentPet 是真理源（与 Story 37.10 currentRoomId 派生同合法理由；
    /// 与 Story 37.8 RoomViewModel.hostCatName 反例不冲突 —— 那是"看别人房间"语境）.
    /// 详见 Dev Notes "profile 派生源 vs Story 37.8 hostCatName 反例".
    private func subscribeProfile(to appState: AppState) {
        // 用 CombineLatest 让 currentUser + currentPet 任一变化都触发 profile 重新合并.
        Publishers.CombineLatest(appState.$currentUser, appState.$currentPet)
            .sink { [weak self] user, pet in
                guard let self else { return }
                self.profile = ProfileSummary(
                    id: user?.id ?? ProfileScaffoldDefaults.profile.id,
                    name: user?.nickname ?? ProfileScaffoldDefaults.profile.name,
                    title: ProfileScaffoldDefaults.profile.title,            // 本期无 server 字段 → 走 defaults
                    joinedAt: ProfileScaffoldDefaults.profile.joinedAt,      // 本期无 server 字段 → 走 defaults
                    petName: pet?.name ?? ProfileScaffoldDefaults.profile.petName,
                    petLevel: ProfileScaffoldDefaults.profile.petLevel,      // 本期 HomePet 无 level 字段 → 走 defaults
                    collectionsCount: ProfileScaffoldDefaults.profile.collectionsCount,
                    friendsCount: ProfileScaffoldDefaults.profile.friendsCount,
                    achievementsCount: ProfileScaffoldDefaults.profile.achievementsCount,
                    coinsCount: ProfileScaffoldDefaults.profile.coinsCount
                )
            }
            .store(in: &profileSubscriptions)
    }

    // MARK: - override abstract methods（本 story 占位 mutate；后续 epic 实装真实 UseCase）

    /// round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md` 预防性应用：
    /// override 必须**本地 mutate** state 让 UI 立刻反馈，不能只 log.
    ///
    /// 行为与 MockProfileViewModel.onWeChatCardTap 同语义：写 showBindModal = true.
    /// 后续 epic 落地时改为：可选检查 @AppStorage("lastWechatPromptAt") 决定弹 modal vs toast 提示.
    public override func onWeChatCardTap() {
        os_log(.debug, "RealProfileViewModel.onWeChatCardTap (后续 epic will check @AppStorage timestamp)")
        showBindModal = true
    }

    /// round 1 P1 lesson 预防性应用：override 必须本地 mutate state.
    /// 行为与 MockProfileViewModel.onWeChatBindConfirmTap 同语义.
    /// 后续 epic 落地时改为：
    ///   1) 调 BindWechatUseCase / WXApi.sendAuthReq 拉起授权
    ///   2) 后端换 OpenID + 写入用户表
    ///   3) 成功后 server 推送 → setWeChatBound(true) + showBindModal = false + Toast "微信绑定成功，数据已受保护"
    public override func onWeChatBindConfirmTap() {
        os_log(.debug, "RealProfileViewModel.onWeChatBindConfirmTap (后续 epic will wire WXApi.sendAuthReq)")
        wechatBound = true
        showBindModal = false
        lastToastMessage = "微信绑定（敬请期待）"
    }

    /// round 1 P1 lesson 预防性应用：override 必须本地 mutate state.
    /// 行为与 MockProfileViewModel.onWeChatModalDismissTap 同语义：关闭 Modal.
    /// 后续 epic 加入 @AppStorage("lastWechatPromptAt") 时间戳记录"已 dismiss"语义（24 小时再次弹一次）.
    public override func onWeChatModalDismissTap() {
        os_log(.debug, "RealProfileViewModel.onWeChatModalDismissTap (后续 epic will record dismiss timestamp)")
        showBindModal = false
    }

    /// round 1 P1 lesson 预防性应用：override 必须本地 mutate state.
    /// 行为与 MockProfileViewModel.onMenuTap 同语义：写 toast 占位.
    /// 后续 epic 改 NavigationLink push 到具体子页面.
    public override func onMenuTap(item: ProfileMenuItem) {
        os_log(.debug, "RealProfileViewModel.onMenuTap %{public}@ (后续 epic will push child views)", item.rawValue)
        lastToastMessage = "\(item.label)（敬请期待）"
    }

    /// round 1 P1 lesson 预防性应用：override 必须本地 mutate state.
    /// 行为与 MockProfileViewModel.onCollectionViewAllTap 同语义：写 toast 占位.
    /// 后续 epic 改 NavigationLink push 到 AllCollectionsView.
    public override func onCollectionViewAllTap() {
        os_log(.debug, "RealProfileViewModel.onCollectionViewAllTap (后续 epic will push AllCollectionsView)")
        lastToastMessage = "查看全部收藏（敬请期待）"
    }
}
```

> **关键决策 1**：MockProfileViewModel / RealProfileViewModel 都 `final` —— 子类不可再被继承（与 ADR-0010 §3.1 mock 模式钦定 + Story 37.7 / 37.8 / 37.9 / 37.10 同精神）；只有基类 `ProfileViewModel` 是 `class`（非 final）。

> **关键决策 2**：MockProfileViewModel 用 invocations 数组而非 closure spy —— 与已有 4 个 Mock 同精神。

> **关键决策 3**：RealProfileViewModel 5 个 override **必须本地 mutate state**（按 Story 37.9 round 1 P1 lesson 钦定）—— Real 路径不能只 log。**与 Story 37.10 略不同**：本 story 的 mutate **不**走 `appState.setXxx(...)` 入口（profile 派生源已通过 sink 订阅 currentUser/Pet；wechatBound / showBindModal / lastToastMessage 是 Profile 域 ViewModel-only state，不进 AppState 白名单 ADR-0010 §3.2），直接写 `self.wechatBound = true` 等即可。这是合法路径 —— 兄弟 ViewModel（Home/Room/Wardrobe/Friends）都不订阅 wechatBound / showBindModal，不存在"漏触发兄弟 sink"问题。

> **关键决策 4**：RealProfileViewModel.subscribeProfile 用 `Publishers.CombineLatest` 合并两个 publisher —— currentUser + currentPet 任一变化都触发 profile 重组；与单一 sink 订阅一个 publisher 不同（FriendsViewModel currentRoomId 是单 publisher 派生）。lesson 2 守护：reset 路径（appState.reset() 把两者都置 nil）能即时反映到 profile（fallback 到 ScaffoldDefaults 字段值，不残留旧 user.nickname）。

> **关键决策 5**：RealProfileViewModel **不**为 `recentCollections` / `wechatBound` 字段单独建 sink —— 本 story 范围内 recentCollections 永远走 `ProfileScaffoldDefaults.recentCollections` seed（收藏接口在后续 epic 才落地）；wechatBound 是 ViewModel 持有的 transient state（本期无持久化，无 server 字段）。当真实接口落地时再加 `subscribeRecentCollections` sink，**不**预 over-design。详见 Dev Notes "RealProfileViewModel 字段策略"。

**对应 Tasks**: Task 2.1, 2.2


### AC3 — 新建 ProfileSummary / RecentCollection / ProfileMenuItem 值类型 + ProfileScaffoldDefaults 共享 enum

**新建文件**: `iphone/PetApp/Features/Profile/Models/ProfileSummary.swift`

```swift
// ProfileSummary.swift
// Story 37.11 AC3: ProfileScaffoldView 顶部头图 + 统计卡 + Modal 风险列表共享数据模型.
//
// 设计：value type + Equatable + Sendable，纯展示数据（统计字段为 raw int / String，由 view 层格式化显示）.
// 后续 epic 接 server `/profile/me` + `/collections/count` + `/achievements/count` + `/friends/count` 后由
//   RealProfileViewModel 内 mapping 写入（API DTO → ProfileSummary，多 publisher 合并）.
//
// 字段名对齐 ui_design profile.jsx 内 ProfileScreen({ user }) shape（user.name / user.id / user.title / user.joinedAt）+
//   wechat_binding.md §"数据风险清单" 4 项（小猫 Lv.X / N 件收藏品 / Y 个成就徽章 / Z 位好友关系）.

import Foundation

public struct ProfileSummary: Equatable, Sendable {
    /// 用户 id（profile.jsx:45 `ID: {user.id}`）.
    public let id: String
    /// 用户昵称（profile.jsx:43）.
    public let name: String
    /// 用户称号（profile.jsx:45 `· {user.title}`；如"养猫达人"）.
    public let title: String
    /// "加入于 {joinedAt}"小药丸（profile.jsx:52；如"2024.03.05"）.
    public let joinedAt: String
    /// 小猫名（Modal 风险列表 "小猫 Lv.X · {petName}"；wechat_binding.md:74）.
    public let petName: String
    /// 小猫等级（统计卡 "小猫等级 Lv.X" + Modal 风险列表 "Lv.X"；profile.jsx:69 + wechat_binding.md:74）.
    public let petLevel: Int
    /// 收藏品数量（统计卡 "收藏品 N" + Modal "{N} 件收藏品"；profile.jsx:65 + wechat_binding.md:75）.
    public let collectionsCount: Int
    /// 好友数量（统计卡 "好友 N" + Modal "{N} 位好友关系"；profile.jsx:67 + wechat_binding.md:77）.
    public let friendsCount: Int
    /// 成就数量（统计卡 "成就 N" + Modal "{N} 个成就徽章"；profile.jsx:71 + wechat_binding.md:76）.
    public let achievementsCount: Int
    /// 钻石货币数量（Modal "价值 {N} 钻石"；wechat_binding.md:75）.
    public let coinsCount: Int

    public init(
        id: String,
        name: String,
        title: String,
        joinedAt: String,
        petName: String,
        petLevel: Int,
        collectionsCount: Int,
        friendsCount: Int,
        achievementsCount: Int,
        coinsCount: Int
    ) {
        self.id = id
        self.name = name
        self.title = title
        self.joinedAt = joinedAt
        self.petName = petName
        self.petLevel = petLevel
        self.collectionsCount = collectionsCount
        self.friendsCount = friendsCount
        self.achievementsCount = achievementsCount
        self.coinsCount = coinsCount
    }
}
```

**新建文件**: `iphone/PetApp/Features/Profile/Models/RecentCollection.swift`

```swift
// RecentCollection.swift
// Story 37.11 AC3: 最近收藏横向滑窗 cell 数据.
//
// 设计：value type + Equatable + Identifiable + Sendable.
// 字段对齐 ui_design profile.jsx:140-159 mock array `[{n:'樱花发饰',r:'SR',i:'🎀'}, ...]`.

import Foundation

public struct RecentCollection: Equatable, Identifiable, Sendable {
    public let id: String      // 唯一 id（mock 用 "rc-1" / "rc-2" 等；后续 epic 真接 user_cosmetic_items.id）
    public let name: String    // 道具名（如"樱花发饰"）
    public let rarity: Rarity  // 稀有度（复用 Story 37.6 落地的 Rarity enum）
    public let emoji: String   // 占位 emoji（profile.jsx 视觉走 emoji + ui_design 风格；后续 epic 接真 sprite 时改字段）

    public init(id: String, name: String, rarity: Rarity, emoji: String) {
        self.id = id
        self.name = name
        self.rarity = rarity
        self.emoji = emoji
    }
}
```

> **关键决策**：复用 Story 37.6 落地的 `Rarity` enum（`N` / `R` / `SR` / `SSR`）—— 与 Story 37.9 落地的 `CosmeticItem.rarity` 字段同精神；不重新定义。

**新建文件**: `iphone/PetApp/Features/Profile/Models/ProfileMenuItem.swift`

```swift
// ProfileMenuItem.swift
// Story 37.11 AC3: 菜单列表 4 项 enum.
//
// rawValue 与 ui_design profile.jsx:171-175 4 行 mock map 对齐（icon / label / extra）：
//   - achievements: 成就徽章 (icon: trophy, extra: "15/40")
//   - messages: 消息通知 (icon: bell, extra: "3 条未读")
//   - favorites: 喜欢的道具 (icon: heart, extra: "")
//   - settings: 设置 (icon: settings, extra: "")

import Foundation

public enum ProfileMenuItem: String, CaseIterable, Identifiable, Sendable {
    case achievements
    case messages
    case favorites
    case settings

    public var id: String { rawValue }

    /// 菜单显示名（ui_design profile.jsx:172-175 钦定）.
    public var label: String {
        switch self {
        case .achievements: return "成就徽章"
        case .messages:     return "消息通知"
        case .favorites:    return "喜欢的道具"
        case .settings:     return "设置"
        }
    }

    /// 菜单 SF Symbol icon 键（走 Icons.symbol(for:) 入口）.
    /// achievements → trophy / messages → bell / favorites → heart / settings → settings.
    public var iconKey: String {
        switch self {
        case .achievements: return "trophy"
        case .messages:     return "bell"
        case .favorites:    return "heart"
        case .settings:     return "settings"
        }
    }

    /// 菜单右侧 extra 文字（profile.jsx:172-175 钦定）；空字符串表示无 extra 显示.
    public var extraText: String {
        switch self {
        case .achievements: return "15/40"
        case .messages:     return "3 条未读"
        case .favorites:    return ""
        case .settings:     return ""
        }
    }
}
```

> **关键决策**：`extraText` 走 enum computed property 而非 ProfileSummary 字段 —— 这些是 mock 占位文案（"15/40" / "3 条未读"），ui_design 钦定值；后续 epic 真接成就 / 消息接口时改派生源（如 `vm.unreadMessagesCount` 派生自 server）。本期写死 enum 内是为了让单元测试能直接断言渲染文字，**不**预 over-design。

**新建文件**: `iphone/PetApp/Features/Profile/ViewModels/ProfileScaffoldDefaults.swift`

```swift
// ProfileScaffoldDefaults.swift
// Story 37.11 AC3: Mock 与 Real ProfileViewModel 共享 scaffold 占位数据.
//
// 背景（Story 37.8 / 37.9 / 37.10 round 1 P2 lesson 预防性应用）：
//   抽 shared defaults 而非 hardcode 在两个 ViewModel —— 避免 Mock/Real 重复定义 mock 数据.
//
// 设计决议（与 RoomScaffoldDefaults / WardrobeScaffoldDefaults / FriendsScaffoldDefaults 同精神）：
//   - profile mock 字段值与 ui_design profile.jsx 视觉示例匹配（"奶团 Lv.8 / 36 件收藏品 / 12 位好友 / 15 个成就 / 248 钻石"）
//   - wechatBound 默认 false（profile.jsx 默认状态；用户点"绑定微信卡" → 切 true 后渲染"已绑定卡"分支）
//   - recentCollections 5 件（profile.jsx:140-145 钦定 5 件；混合 R/SR 稀有度）

import Foundation

/// Mock 与 Real ProfileViewModel 启动占位数据（profile state UI scaffold defaults）.
public enum ProfileScaffoldDefaults {
    /// 默认 profile（mock 用户「奶团」、Lv.8、36 收藏、12 好友、15 成就、248 钻石；
    /// 与 ui_design profile.jsx + wechat_binding.md 视觉示例一致）.
    public static let profile: ProfileSummary = ProfileSummary(
        id: "u-mock-9527",
        name: "奶团",
        title: "养猫达人",
        joinedAt: "2024.03.05",
        petName: "奶团",
        petLevel: 8,
        collectionsCount: 36,
        friendsCount: 12,
        achievementsCount: 15,
        coinsCount: 248
    )

    /// 默认微信绑定状态（mock 默认 false —— ui_design profile.jsx 默认渲染未绑定警告卡分支）.
    public static let wechatBound: Bool = false

    /// 完整 mock recentCollections（5 件，混合 R/SR 稀有度，与 profile.jsx:140-145 mock array 字段值一致）.
    public static let recentCollections: [RecentCollection] = [
        RecentCollection(id: "rc-1", name: "樱花发饰",  rarity: .SR, emoji: "🎀"),
        RecentCollection(id: "rc-2", name: "贝雷帽",    rarity: .R,  emoji: "🎩"),
        RecentCollection(id: "rc-3", name: "骑士披风",  rarity: .SR, emoji: "🧣"),
        RecentCollection(id: "rc-4", name: "水手服",    rarity: .R,  emoji: "👘"),
        RecentCollection(id: "rc-5", name: "樱花树下",  rarity: .SR, emoji: "🏞️"),
    ]
}
```

> **关键决策**：profile mock 字段值与 wechat_binding.md §"数据风险清单"（小猫 Lv.8 · 奶团 / 36 件收藏品 · 价值 248 钻石 / 15 个成就徽章 / 12 位好友关系）严格对齐 —— 让 BindWechatModal 内 4 行风险列表渲染 `state.profile.*Count` / `state.profile.petName` / `state.profile.petLevel` 等字段直接产出 ui_design 视觉钦定文字。

**对应 Tasks**: Task 3.1, 3.2, 3.3, 3.4


### AC4 — 新建 ProfileScaffoldView struct + 5 区块视觉 + BindWechatModal sheet

**新建文件**: `iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift`

**关键签名**（与 HomeView Story 37.7 / RoomScaffoldView Story 37.8 / WardrobeScaffoldView Story 37.9 / FriendsScaffoldView Story 37.10 同模式：`@ObservedObject var state: ProfileViewModel` 基类直接，**非泛型 state**）：

```swift
public struct ProfileScaffoldView: View {
    @ObservedObject public var state: ProfileViewModel

    /// Story 37.5: 主题 token 取值入口；RootView 注入 `.environment(\.theme, currentTheme.theme)`.
    @Environment(\.theme) private var theme

    public init(state: ProfileViewModel) {
        self.state = state
    }

    public var body: some View {
        ScrollView {
            VStack(spacing: 0) {
                headerCard            // 区块 1: 顶部渐变头图 + Avatar + 用户名/ID/title/joinedAt
                statsCard             // 区块 2: 4 列统计卡（覆盖头图底部 1/3 negative margin）
                wechatCard            // 区块 3: 微信绑定卡（双态 wechatBound true/false 切换）
                recentCollections     // 区块 4: 最近收藏横向滑窗（5 件 cell）
                menuList              // 区块 5: 菜单列表 4 项
            }
            .padding(.bottom, 100)    // 让出浮动 TabBar 空间
        }
        .background(theme.colors.pageBg.ignoresSafeArea())
        .accessibilityIdentifier("profileView")
        .overlay(alignment: .bottom) { toastOverlay }                 // 占位 toast（与 Story 37.10 同精神）
        .sheet(isPresented: $state.showBindModal) { bindWechatSheet } // 微信绑定 Modal
    }
    // ... 5 区块子视图实现 + bindWechatSheet 详见 Dev Notes "5 区块视觉契约"
}
```

**5 区块要点**（详细视觉规则见 Dev Notes "5 区块视觉契约"；这里给关键定位锚）：

- **headerCard**（profile.jsx:21-56）：ZStack/VStack 顶部渐变背景 `LinearGradient(colors: [accentSoft, accent], startPoint: .top, endPoint: .bottom)` + padding 68pt top（状态栏占位）/ 20pt horizontal / 50pt bottom；HStack 含左 22pt 800 white "我的" + 右 HStack(8pt) 2 个 36x36 圆 18 半透明白底（rgba(255,255,255,0.3)）IconButton（bell + settings，仅 print log + 写 lastToastMessage 占位）；下方 HStack 14pt 间距含 `Avatar(name: profile.name, size: 72, color: Color(hex: 0xfff1e8), ring: true)` + VStack 含 22pt 800 white profile.name + 12pt 700 white-0.85 "ID: \(profile.id) · \(profile.title)" + 11pt 700 white inline-flex pill 含 Icons.sparkle + "加入于 \(profile.joinedAt)"；accessibilityIdentifier `profileHeaderCard`
- **statsCard**（profile.jsx:58-73）：HStack(spacing: 0) 4 列 + 3 个 Divider（width 1pt + 6pt vertical inset + theme.colors.border 色）；4 列分别是 `Stat(label: "收藏品", value: "\(profile.collectionsCount)", iconKey: "diamond")` / `Stat(label: "好友", value: "\(profile.friendsCount)", iconKey: "friends")` / `Stat(label: "小猫等级", value: "Lv.\(profile.petLevel)", iconKey: "paw")` / `Stat(label: "成就", value: "\(profile.achievementsCount)", iconKey: "trophy")`；每列 Stat 是 VStack(spacing: 4) HStack 居中 18pt accent 色 icon + 17pt 800 ink value + 10pt 700 ink-soft label；容器 HStack 整体走 surface 背景 + 圆角 22 + theme.shadow.md + border 1pt + padding 16；外层 padding 0/20pt + marginTop -34（覆盖在头图底部）；accessibilityIdentifier `profileStatsCard`
- **wechatCard**（profile.jsx:75-134；双态分支 wechatBound true/false）：padding 14pt top / 20pt horizontal / 0 bottom；
  - **未绑定（state.wechatBound == false）**：整张卡 Button（accessibilityIdentifier `profileWeChatCard`，点击调 `state.onWeChatCardTap()`）；视觉走"黄色警告卡"（`LinearGradient(colors: [Color(hex: 0xfff8e1), Color(hex: 0xffe8b5)], startPoint: .topLeading, endPoint: .bottomTrailing)` 背景 + 圆 18 + sm shadow + 1.5pt `Color(hex: 0xffc94c)` border）；HStack(12pt) 含左 40x40 圆 12 白底 + sm 黄色 shadow `0 2px 6px rgba(255,180,0,0.3)` 内 22pt `Color(hex: 0xe89400)` warn 图标 + 中 VStack(2pt) 主标题 14pt 800 `Color(hex: 0x7a4f00)` "绑定微信，保护小猫数据" + 副标题 11pt 700 `Color(hex: 0xa06b00)` "未绑定时卸载 App 将丢失全部数据" + 右胶囊按钮（圆 14 + `Color(hex: 0x1aad19)` 微信绿底 + white 12pt 800 含 wechat icon + " 立即绑定"）
  - **已绑定（state.wechatBound == true）**：纯展示 HStack（accessibilityIdentifier `profileWeChatCardBound`）；视觉走"绿色确认卡"（surface 背景 + 圆 18 + sm shadow + 1pt border）；HStack(12pt) 含左 40x40 圆 12 `Color(hex: 0xe8f7e0)` 浅绿底内 22pt `Color(hex: 0x1aad19)` wechat icon + 中 VStack 主标题 14pt 800 ink "微信已绑定" + 9pt 800 white 角标 "已保护"（`Color(hex: 0x1aad19)` 绿底 + padding 2/6 + 圆 6）+ 副标题 11pt 700 ink-soft "数据已同步至云端，卸载重装不会丢失" + 右 20pt `Color(hex: 0x1aad19)` shield icon；本卡**不**可点击（已绑定无后续动作）
- **recentCollections**（profile.jsx:136-162）：padding 18pt top / 20pt horizontal；SectionHeader（HStack 15pt 800 ink "最近收藏" + 右 Button 12pt 700 accentDeep "查看全部" + chevronRight，调 `state.onCollectionViewAllTap()`，accessibilityIdentifier `profileCollectionViewAll`）；下方 ScrollView(.horizontal, showsIndicators: false) 含 HStack(spacing: 10) ForEach `state.recentCollections` 渲染 cell：每 cell 88pt 宽 + surface 背景 + 圆 16 + sm shadow + 1pt border + padding 10 + VStack(4pt) 含 60x60 圆 12 surface2 背景 32pt emoji + 11pt 800 ink center name；accessibilityIdentifier `profileCollectionCell_<rc.id>`；空数组 fallback "暂无收藏"
- **menuList**（profile.jsx:164-192）：padding 18pt top / 20pt horizontal；SectionHeader 15pt 800 ink "更多"（无 more 按钮）；下方 VStack(spacing: 0) surface 背景 + 圆 20 + sm shadow + 1pt border + clipShape；ForEach `ProfileMenuItem.allCases` 渲染 row：HStack(12pt) padding 14/16 含 36x36 圆 12 accentSoft 背景内 20pt accentDeep iconKey 图标 + 14pt 700 ink label + Spacer + （extraText 非空时渲染 11pt 700 ink-soft extraText）+ 18pt ink-mute chevronRight；最后一行无 borderBottom，其它行 1pt theme.colors.border 底分割线；整 row 走 Button 调 `state.onMenuTap(item:)`；accessibilityIdentifier `profileMenu_\(item.rawValue)`（4 个：profileMenu_achievements / profileMenu_messages / profileMenu_favorites / profileMenu_settings）

**bindWechatSheet**（profile.jsx:205-283 + wechat_binding.md §强制提醒浮窗）：
- 用 SwiftUI `.sheet(isPresented: $state.showBindModal)` 挂在 ProfileScaffoldView 主体外层（**不**用自绘 ZStack overlay —— iOS 标准 sheet 已含遮罩 + 上滑动画 + 拖拽 dismiss）
- 卡片签名 `BindWechatModalView(state: state)` 子视图（**不**抽到独立文件 —— 仅本 view 用 + 与 ProfileScaffoldView 共享 state）；Modal 内访问 `state.profile.*` 拼数据风险列表 4 行
- Modal 内布局（自上而下）：VStack(spacing: 0) padding 24
  - 警告插画区（VStack 居中 88x88 圆 44 `LinearGradient(colors: [Color(hex: 0xfff3d6), Color(hex: 0xffd97a)], startPoint: .topLeading, endPoint: .bottomTrailing)` 背景 + 内 46pt `Color(hex: 0xe89400)` warn icon + **不**实装外圈装饰旋转虚线圆环（profile.jsx:224 spin animation；本 story 略，与 toast timer 同减法决策）
  - 标题 19pt 800 ink center "数据可能丢失！" margin 14pt top
  - 正文 13pt 600 center 行高 1.6 ink-soft "您还未绑定微信账号，" + inline 800 `Color(hex: 0xe15f7c)` "一旦卸载本 App，您的小猫、收藏品、好友关系等所有数据都将被永久删除，无法恢复。" margin 8pt top + 16pt bottom
  - 数据风险清单（VStack(spacing: 0) `Color(hex: 0xfff5f5)` 背景 + 圆 16 + 1pt `Color(hex: 0xffe0e0)` border + padding 12/14；4 行 DataLossRow（HStack 8pt 16pt emoji + 12pt 700 `Color(hex: 0x7a3a3a)` text + Spacer + 10pt 800 `Color(hex: 0xe15f7c)` "将丢失"；行间 1pt dashed `Color(hex: 0xffd0d0)` 虚线分割除最后行外）：
    - 🐱 "小猫 Lv.\(state.profile.petLevel) · \(state.profile.petName)"
    - 💎 "\(state.profile.collectionsCount) 件收藏品 · 价值 \(state.profile.coinsCount) 钻石"
    - 🏆 "\(state.profile.achievementsCount) 个成就徽章"
    - 👥 "\(state.profile.friendsCount) 位好友关系"
  - 按钮组 VStack(spacing: 10) margin 18pt top：
    - 主按钮（accessibilityIdentifier `profileWeChatBindButton`）调 `state.onWeChatBindConfirmTap()`；高 52 + 圆 26 + `Color(hex: 0x1aad19)` 微信绿底 + white 15pt 800 + 立体 shadow `0 4px 0 Color(hex: 0x138a12)` + 内 HStack(8pt) 含 20pt white wechat icon + " 绑定微信，保护数据"
    - 次按钮（accessibilityIdentifier `profileWeChatCancelButton`）调 `state.onWeChatModalDismissTap()`；高 40 + transparent 背景 + ink-mute 12pt 700 + "稍后再说（数据将不受保护）"
- Modal 整体 accessibilityIdentifier `profileWeChatModal`
- **不**自绘遮罩 + 自绘动画（与 SwiftUI sheet 标准行为冲突）；如需要自定义 corner radius，加 `.presentationDetents([.fraction(0.85)])` + `.presentationCornerRadius(28)`（iOS 16.4+ API；与 wechat_binding.md SwiftUI 实现要点 §"presentationDetents([.fraction(0.78)])"对齐，本 story 取 0.85 让数据风险列表完整可见）

**toastOverlay**（与 Story 37.10 同精神）：仅 `state.lastToastMessage != nil` 时渲染；底部 padding 120 + 黑底 0.85 alpha + white 13pt 700 + 圆 12 + padding 8/16；accessibilityIdentifier `profileToast`；不自动消失（与 Story 37.10 同决策）

> **关键决策 1**：5 区块布局用 `ScrollView { VStack(spacing: 0) { ... } }` 让整页可滚动 —— ui_design profile.jsx:20 `style={{height:'100%', overflow:'auto'}}`。**与 Story 37.10 不同**（FriendsScaffoldView 5 区块塞主 body + 仅 friendsList 滚动）：Profile 整页内容更长（顶部头图 + 统计卡 + 微信卡 + 收藏滑窗 + 菜单），ScrollView 包整 VStack 是合理的；**recentCollections** 内层是 ScrollView(.horizontal) 横向滑窗（嵌套 ScrollView，SwiftUI 原生支持，不会冲突）。

> **关键决策 2**：BindWechatModal 用 SwiftUI `.sheet(isPresented:)` 而非自绘 ZStack overlay —— 让 SwiftUI 处理遮罩 / 上滑动画 / 拖拽 dismiss 标准行为；ui_design profile.jsx:208-216 自绘 `position:'absolute', inset:0` 遮罩 + 自绘 modal 卡片是 React Web 老路径，iOS 走 sheet API 是规范路径。

> **关键决策 3**：BindWechatModalView 抽**子视图 struct**（在 ProfileScaffoldView.swift 同一文件内）而非独立文件 —— 与 Story 37.10 toastOverlay 同精神（仅本 view 用 + 共享 state）；如未来 BindWechatModal 增长复杂度（如外圈虚线圆环动画落地）再抽独立文件。

> **关键决策 4**：headerCard / wechatCard / 各 IconButton 的 shadow 必须挂在 `RoundedRectangle.fill(...).shadow(...)` 那一层（按 Story 37.6 round 5 lesson `2026-04-30-swiftui-state-survives-id-and-shadow-over-children.md` 钦定路径），**不**挂最外层 chain（避免 children Avatar / Text 被 alpha 投影）。

> **关键决策 5**：未绑定卡 / 已绑定卡的硬编码颜色（`#fff8e1` / `#ffe8b5` / `#ffc94c` / `#7a4f00` / `#a06b00` / `#1aad19` / `#e8f7e0` / `#138a12` / `#fff3d6` / `#ffd97a` / `#e89400` / `#e15f7c` / `#fff5f5` / `#ffe0e0` / `#ffd0d0` / `#7a3a3a` / `#fff1e8`）—— 这些是 ui_design `profile.jsx` + `wechat_binding.md` 钦定的"微信品牌色 + 警告品牌色 + 风险红"，**不进 theme tokens**（theme 只覆盖 13 个语义色 token；微信绿是品牌色不应主题化）；用 `Color(hex: 0xRRGGBB)` 硬编码即可。如未来需要"暗色模式微信卡视觉"再单独决策（本 story 范围内 dark / candy 两主题视觉相同）。

> **关键决策 6**：3 个 Divider（statsCard 内）用 `Rectangle().fill(theme.colors.border).frame(width: 1).padding(.vertical, 6)` 实现 —— 与 SwiftUI `Divider()` 默认 system 样式不同（system Divider 高度 0.5pt，颜色固定）；为了让视觉与 ui_design 1pt 高度 + theme.colors.border 色 + 6pt vertical inset 1:1 对齐，自绘 Rectangle。

**对应 Tasks**: Task 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7, 4.8


### AC5 — ProfileView caller 替换 + RootView wire + LaunchedContentView 透传 + MainTabView Preview 注入

**改动文件 1**: `iphone/PetApp/Features/Profile/Views/ProfileView.swift`

**关键改动**（替换 body 占位 Text 为 ProfileScaffoldView 真实内容）：

```swift
// 旧（Story 37.3 落地）
public struct ProfileView: View {
    public init() {}

    public var body: some View {
        NavigationStack {
            Text("Profile Tab Placeholder")
                .accessibilityIdentifier("profileView")
        }
    }
}

// 新（Story 37.11 落地）
public struct ProfileView: View {
    @EnvironmentObject var profileViewModel: ProfileViewModel

    public init() {}

    public var body: some View {
        NavigationStack {
            ProfileScaffoldView(state: profileViewModel)
        }
    }
}
```

> **关键决策**：保留 `NavigationStack` 包裹层 —— 让后续 epic 实装 NavigationLink push 到具体子页面（成就 / 消息 / 喜欢的道具 / 设置）时无须再改 ProfileView 类型签名（与 Story 37.10 FriendsView 同精神）。

**改动文件 2**: `iphone/PetApp/App/RootView.swift`

**关键改动**：在 `@StateObject friendsViewModel` 同级新增 `@StateObject profileViewModel` + 同级 `.environmentObject(profileViewModel)` + `.onAppear` 内同步 bind appState：

```swift
// 新增（Story 37.11 追加；homeViewModel / roomViewModel / wardrobeViewModel / friendsViewModel 不动）
@StateObject private var profileViewModel: ProfileViewModel = RealProfileViewModel()

// .onAppear 内追加（在已有 realFriendsVM.bind 之后）：
if let realProfileVM = profileViewModel as? RealProfileViewModel {
    realProfileVM.bind(appState: appState)
}

// LaunchedContentView 调用站追加 profileViewModel: profileViewModel 字段透传
```

LaunchedContentView 透传：

```swift
// LaunchedContentView 加 profileViewModel: ProfileViewModel 字段（与 homeViewModel / roomViewModel /
//   wardrobeViewModel / friendsViewModel 同模式）+ init 参数 + body 内 .environmentObject(profileViewModel) 注入 ready 子树
```

> **关键决策**：Story 37.7 round 1 P1 lesson 预防性应用 —— `@StateObject private var profileViewModel: ProfileViewModel = RealProfileViewModel()`（**不是**裸基类 `ProfileViewModel()` —— 基类 5 个 abstract method 是 fatalError，用户点任一按钮即 crash）。

> **关键决策**：Story 37.8 round 2 P2 lesson 预防性应用 —— `bind(appState:)` 调用放在 `.onAppear` 而非 `.task`（防 launch-time race，让 ViewModel 在第一次 paint 之前就持有 AppState 引用）。

> **caller 漏改靠编译器报错驱动**：ProfileView 内 `Text("Profile Tab Placeholder")` 替换为 `ProfileScaffoldView(state: profileViewModel)` —— 旧 body 替换前编译过；替换后若漏挂 `@EnvironmentObject` 或漏注入 `.environmentObject(profileViewModel)` 会 runtime crash（SwiftUI 找不到 environmentObject）—— **不依赖 grep 兜底**。MainTabView 在 Story 37.3 落地时已有 `ProfileView().tag(AppTab.profile)`，调用站不变；Preview 块需要追加 `.environmentObject(MockProfileViewModel() as ProfileViewModel)` 让 MainTabView Preview 不 crash。

**改动文件 3**: `iphone/PetApp/App/MainTabView.swift`

```swift
// 在 Preview 块（行 122 之后已注入 4 个 @EnvironmentObject）追加：
.environmentObject(MockProfileViewModel() as ProfileViewModel)
```

**对应 Tasks**: Task 5.1, 5.2, 5.3, 5.4, 5.5

### AC6 — #Preview 双主题（candy / dark）+ 多场景 mock

ProfileScaffoldView 文件底部 `#if DEBUG ... #endif` 块含 4 个 Preview（双主题 × 默认/已绑定/Modal-open 场景）：

```swift
#if DEBUG
#Preview("ProfileScaffoldView — full mock / candy") {
    ProfileScaffoldView(state: MockProfileViewModel())
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("ProfileScaffoldView — full mock / dark") {
    ProfileScaffoldView(state: MockProfileViewModel())
        .environment(\.theme, ThemeName.dark.theme)
}

#Preview("ProfileScaffoldView — wechat bound / candy") {
    ProfileScaffoldView(state: MockProfileViewModel(wechatBound: true))
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("ProfileScaffoldView — bind modal open / candy") {
    ProfileScaffoldView(state: MockProfileViewModel(showBindModal: true))
        .environment(\.theme, ThemeName.candy.theme)
}
#endif
```

> **关键决策**：4 个 Preview 覆盖 默认（未绑定警告卡 + 5 件收藏 + 4 项菜单）/ 已绑定（绿色确认卡分支）/ Modal open（验证 BindWechatModal 视觉壳，包含 4 行数据风险列表 + 2 个按钮）/ dark 主题（验证 Theme token 适配）。

**对应 Tasks**: Task 6.1

### AC7 — 单元测试覆盖（≥10 case，纯 XCTest + MockProfileViewModel + AppState）

**新建文件**: `iphone/PetAppTests/Features/Profile/ProfileViewScaffoldTests.swift`

落地以下 ≥10 case（≥3 epic AC line 4837 + 守护 case 防 lesson 反例）：

```swift
// ProfileViewScaffoldTests.swift
// Story 37.11 AC7: ProfileScaffoldView + ProfileViewModel class 层次单元测试.
//
// 测试基础设施约束（与 Story 2.7 + ADR-0002 §3.1 衔接）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）.
//   - 不引 ViewInspector / SnapshotTesting.
//   - 走 ViewModel 行为 + invocations 数组断言；不走 SwiftUI body 内省.

import XCTest
@testable import PetApp

@MainActor
final class ProfileViewScaffoldTests: XCTestCase {

    // MARK: - case#1 happy: 默认初始化 → wechatBound=false / showBindModal=false / lastToastMessage=nil + scaffold defaults seed

    func testMockInitSeedsScaffoldDefaults() {
        let vm = MockProfileViewModel()
        XCTAssertFalse(vm.wechatBound)
        XCTAssertFalse(vm.showBindModal)
        XCTAssertNil(vm.lastToastMessage)
        XCTAssertEqual(vm.profile.name, ProfileScaffoldDefaults.profile.name)
        XCTAssertEqual(vm.profile.petName, ProfileScaffoldDefaults.profile.petName)
        XCTAssertEqual(vm.profile.collectionsCount, ProfileScaffoldDefaults.profile.collectionsCount)
        XCTAssertEqual(vm.recentCollections.count, 5, "ScaffoldDefaults 钦定 5 件最近收藏")
    }

    // MARK: - case#2 happy: 点未绑定卡 → showBindModal = true + invocation 记录

    func testWeChatCardTapShowsBindModal() {
        let vm = MockProfileViewModel()
        XCTAssertFalse(vm.showBindModal)

        vm.onWeChatCardTap()
        XCTAssertTrue(vm.showBindModal, "点未绑定卡 → showBindModal = true")
        XCTAssertEqual(vm.invocations, [.wechatCardTap])
    }

    // MARK: - case#3 happy: Modal 内"绑定微信"按钮 → wechatBound = true + showBindModal = false + toast

    func testWeChatBindConfirmTapBindsAndDismissesModal() {
        let vm = MockProfileViewModel(showBindModal: true)
        XCTAssertFalse(vm.wechatBound)

        vm.onWeChatBindConfirmTap()
        XCTAssertTrue(vm.wechatBound, "确认绑定 → wechatBound = true")
        XCTAssertFalse(vm.showBindModal, "确认绑定 → showBindModal = false")
        XCTAssertNotNil(vm.lastToastMessage)
        XCTAssertEqual(vm.invocations, [.wechatBindConfirmTap])
    }

    // MARK: - case#4 happy: Modal "稍后再说"按钮 → showBindModal = false + invocation 记录

    func testWeChatModalDismissTapClosesModal() {
        let vm = MockProfileViewModel(showBindModal: true)
        XCTAssertTrue(vm.showBindModal)

        vm.onWeChatModalDismissTap()
        XCTAssertFalse(vm.showBindModal)
        XCTAssertFalse(vm.wechatBound, "稍后再说 → wechatBound 不变")
        XCTAssertEqual(vm.invocations, [.wechatModalDismissTap])
    }

    // MARK: - case#5 happy: 点 4 个菜单项 → invocation 记录 + toast 含 item.label

    func testMenuTapTriggersToastForEachItem() {
        let vm = MockProfileViewModel()
        for item in ProfileMenuItem.allCases {
            vm.onMenuTap(item: item)
            XCTAssertNotNil(vm.lastToastMessage)
            XCTAssertTrue(
                vm.lastToastMessage!.contains(item.label),
                "toast 必须含 item.label: \(item.label)"
            )
        }
        XCTAssertEqual(vm.invocations.count, ProfileMenuItem.allCases.count)
        XCTAssertEqual(vm.invocations, ProfileMenuItem.allCases.map { .menuTap(item: $0) })
    }

    // MARK: - case#6 happy: 点"查看全部"收藏 → toast + invocation 记录

    func testCollectionViewAllTapTriggersToast() {
        let vm = MockProfileViewModel()
        vm.onCollectionViewAllTap()
        XCTAssertNotNil(vm.lastToastMessage)
        XCTAssertTrue(vm.lastToastMessage!.contains("查看全部"))
        XCTAssertEqual(vm.invocations, [.collectionViewAllTap])
    }

    // MARK: - case#7 守护: RealProfileViewModel 构造注入 AppState 不 crash + override 不 fatalError + Real override 必 mutate state

    /// 防 RealProfileViewModel 漏 override 时本测试立刻 fail（fatalError 在测试中 → trap）.
    /// + 守护 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`：
    ///   Real 子类 override 必须本地 mutate state，禁止只 log.
    func testRealProfileViewModelOverridesMutateStateNotJustLog() {
        let appState = AppState()
        let vm = RealProfileViewModel(appState: appState)
        XCTAssertFalse(vm.wechatBound)
        XCTAssertFalse(vm.showBindModal)

        // onWeChatCardTap 必须 mutate showBindModal
        vm.onWeChatCardTap()
        XCTAssertTrue(vm.showBindModal, "Real path 必须 mutate showBindModal（守护 lesson）")

        // onWeChatBindConfirmTap 必须 mutate wechatBound + showBindModal + lastToastMessage
        vm.onWeChatBindConfirmTap()
        XCTAssertTrue(vm.wechatBound, "Real path 必须 mutate wechatBound（守护 lesson）")
        XCTAssertFalse(vm.showBindModal)
        XCTAssertNotNil(vm.lastToastMessage)

        // onWeChatModalDismissTap 必须 mutate showBindModal
        vm.showBindModal = true  // 重置
        vm.onWeChatModalDismissTap()
        XCTAssertFalse(vm.showBindModal, "Real path 必须 mutate showBindModal（守护 lesson）")

        // onMenuTap 必须 mutate lastToastMessage
        vm.lastToastMessage = nil  // 重置
        vm.onMenuTap(item: .achievements)
        XCTAssertNotNil(vm.lastToastMessage, "Real path 必须 mutate lastToastMessage（守护 lesson）")
        XCTAssertTrue(vm.lastToastMessage!.contains("成就"))

        // onCollectionViewAllTap 必须 mutate lastToastMessage
        vm.lastToastMessage = nil  // 重置
        vm.onCollectionViewAllTap()
        XCTAssertNotNil(vm.lastToastMessage, "Real path 必须 mutate lastToastMessage（守护 lesson）")
    }

    // MARK: - case#8 守护: Real init 必 seed scaffold defaults（lesson 4 预防性应用）

    /// 与 Story 37.8 / 37.9 / 37.10 同模式 ——
    /// 防未来 Claude 重构 init 时漏 seed profile 让 RealProfileViewModel 渲染空头像 / "Lv.--" 等占位字符串.
    /// lesson: 2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md
    func testRealProfileViewModelInitSeedsScaffoldDefaults() {
        // parameterless init 路径
        let vm1 = RealProfileViewModel()
        XCTAssertEqual(vm1.profile.name, ProfileScaffoldDefaults.profile.name)
        XCTAssertEqual(vm1.profile.petLevel, ProfileScaffoldDefaults.profile.petLevel)
        XCTAssertEqual(vm1.recentCollections.count, 5)
        XCTAssertFalse(vm1.wechatBound)
        XCTAssertFalse(vm1.showBindModal)

        // init(appState:) 路径
        let vm2 = RealProfileViewModel(appState: AppState())
        XCTAssertEqual(vm2.profile.name, ProfileScaffoldDefaults.profile.name)
        XCTAssertEqual(vm2.recentCollections.count, 5)
    }

    // MARK: - case#9 守护: profile 派生自 appState.currentUser + currentPet（hydrate + reset 路径）

    /// 防未来 Claude 重构时把 profile sink 改一次性 hydrate 让 reset 后残留旧 user.nickname / pet.name.
    /// lesson: 2026-04-30-published-derived-state-needs-publisher-subscription.md
    /// **关键说明**：profile 派生源是合法的（"我的资料"语义就是本地用户的资料，
    ///   appState.currentUser / currentPet 是真理源；与 Story 37.8 hostCatName 反例不冲突 —— 那是"看别人房间"语境）.
    func testRealProfileViewModelProfileDerivesFromAppState() {
        let appState = AppState()
        let vm = RealProfileViewModel(appState: appState)
        // 初始 nil → profile 走 ScaffoldDefaults
        XCTAssertEqual(vm.profile.name, ProfileScaffoldDefaults.profile.name)

        // hydrate 路径：写入 currentUser / currentPet → profile 派生
        let homeData = makeHomeDataFixture(userNickname: "TestUser", petName: "Mochi")
        appState.applyHomeData(homeData)
        XCTAssertEqual(vm.profile.name, "TestUser", "profile.name 派生自 appState.currentUser.nickname")
        XCTAssertEqual(vm.profile.petName, "Mochi", "profile.petName 派生自 appState.currentPet.name")

        // reset 路径：appState.reset() 把 currentUser / currentPet 置 nil → profile 即时 fallback 到 defaults（不残留旧 "TestUser"）
        appState.reset()
        XCTAssertEqual(vm.profile.name, ProfileScaffoldDefaults.profile.name, "reset 后 profile.name 必回 defaults（防 stale）")
        XCTAssertEqual(vm.profile.petName, ProfileScaffoldDefaults.profile.petName)
    }

    // MARK: - case#10 守护: bind(appState:) 是同步入口（lesson 5 预防性应用）

    /// 防未来 Claude 把 bind 改成 async 路径让 RootView .onAppear 触发后第一帧 ViewModel 仍未连上 AppState.
    /// 与 Story 37.8 / 37.9 / 37.10 同模式.
    /// lesson: 2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md
    func testRealProfileViewModelBindAppStateIsSynchronous() {
        let appState = AppState()
        let homeData = makeHomeDataFixture(userNickname: "PreloadedUser", petName: "PreloadedPet")
        appState.applyHomeData(homeData)  // 启动期 currentUser / currentPet 已非 nil（restored session）

        let vm = RealProfileViewModel()  // parameterless init 路径
        XCTAssertEqual(vm.profile.name, ProfileScaffoldDefaults.profile.name, "bind 前 profile = defaults")

        vm.bind(appState: appState)  // 同步路径
        XCTAssertEqual(vm.profile.name, "PreloadedUser", "bind 后立即派生（无 RunLoop tick 等待）")
        XCTAssertEqual(vm.profile.petName, "PreloadedPet")
    }

    // MARK: - case#11 守护: bind(appState:) 重复调用 idempotent（不重订阅）

    /// 防未来 Claude 重构 bind 时漏 alreadySubscribed guard 让多次 bind 派生多次 sink callback.
    func testRealProfileViewModelBindIsIdempotent() {
        let appState = AppState()
        let vm = RealProfileViewModel()
        vm.bind(appState: appState)
        vm.bind(appState: appState)  // 第二次 bind 应 no-op
        // 触发派生：写入 user → profile.name 应只更新一次（不会因双 sink 派生异常）
        let homeData = makeHomeDataFixture(userNickname: "Test", petName: "Cat")
        appState.applyHomeData(homeData)
        XCTAssertEqual(vm.profile.name, "Test")
    }

    // MARK: - 测试辅助

    /// 构造 HomeData fixture（仅注入 user.nickname + pet.name，其它字段走默认 mock）.
    /// 避免每个 case 重复样板代码.
    private func makeHomeDataFixture(userNickname: String, petName: String) -> HomeData {
        HomeData(
            user: HomeUser(id: "u-test", nickname: userNickname, avatarUrl: ""),
            pet: HomePet(
                id: "p-test",
                petType: 1,
                name: petName,
                currentState: .rest,
                equips: []
            ),
            stepAccount: HomeStepAccount(balance: 0),
            chest: HomeChest.testFixture,
            room: nil
        )
    }
}
```

> **关键决策**：≥10 case（epic AC 钦定 ≥3 case；本 story 落 11 case 含 5 个守护 case 预防 Story 37.7 / 37.8 / 37.9 / 37.10 lesson 反例）—— **预防性应用 lesson 6** 的成本兑现（case#7 显式守护"Real override 必 mutate state"）+ profile 派生源测试（case#9）覆盖 hydrate + reset 双路径让 stale 无所遁形。

> **关键决策**：不测 fatalError 路径（基类 abstract method 覆盖在 case#7 间接证明 5 个 override 已生效）。

> **关键决策**：不测 ProfileScaffoldView body 渲染含 a11y identifier（属 UITest 范围；详见 AC8）。

> **关键决策**：不测 toast 自动消失（与 Story 37.10 同决策；本 story ViewModel 不实装 timer）。

> **关键决策**：`makeHomeDataFixture` helper 内 `HomeChest.testFixture`（如不存在则改用 `HomeChest(...)` 真构造）—— dev 实装时根据 HomeData 真实构造签名调整；本 spec 仅给意图（fixture 只填 nickname / petName 关键字段）。

**对应 Tasks**: Task 7.1

### AC8 — UITest a11y identifier 关键锚可定位

**改动文件**: `iphone/PetAppUITests/HomeUITests.swift`（与 Story 37.7 / 37.8 / 37.9 / 37.10 同模式：本 story 加一个新 test case 在 HomeUITests.swift 内；Story 37.13 a11y 总表归并时统一移走）

```swift
// Story 37.11: ProfileScaffoldView 关键 a11y identifier 可定位验证.
// 切到 Profile Tab 后验证主结构 + headerCard / statsCard / wechatCard / 4 个菜单 / Modal 触发链路可定位.
// 与 Story 37.7 / 37.8 / 37.9 / 37.10 同模式.
func testProfileScaffoldShowsAllAnchors() throws {
    let app = XCUIApplication()
    app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
    app.launch()

    let timeout: TimeInterval = 5

    // 切到 Profile Tab
    let profileTab = app.buttons["tab_profile"]
    XCTAssertTrue(profileTab.waitForExistence(timeout: timeout), "tab_profile 未找到")
    profileTab.tap()

    // 验证主容器
    XCTAssertTrue(
        app.descendants(matching: .any)["profileView"].waitForExistence(timeout: 3),
        "profileView 主容器未找到"
    )

    // 验证 5 区块关键锚
    XCTAssertTrue(app.descendants(matching: .any)["profileHeaderCard"].exists, "profileHeaderCard 未找到")
    XCTAssertTrue(app.descendants(matching: .any)["profileStatsCard"].exists, "profileStatsCard 未找到")
    XCTAssertTrue(app.descendants(matching: .any)["profileWeChatCard"].exists, "profileWeChatCard（未绑定卡）未找到")

    // 验证 4 个菜单项
    for item in ["achievements", "messages", "favorites", "settings"] {
        XCTAssertTrue(
            app.descendants(matching: .any)["profileMenu_\(item)"].exists,
            "profileMenu_\(item) 未找到"
        )
    }

    // 验证 BindWechatModal 触发链路：点未绑定卡 → modal 出现
    app.descendants(matching: .any)["profileWeChatCard"].firstMatch.tap()
    XCTAssertTrue(
        app.descendants(matching: .any)["profileWeChatModal"].waitForExistence(timeout: 3),
        "profileWeChatModal 未在 wechatCard tap 后出现"
    )
    XCTAssertTrue(
        app.descendants(matching: .any)["profileWeChatBindButton"].exists,
        "profileWeChatBindButton 未在 modal 内找到"
    )
}
```

> **关键决策**：UITest 主动验证 BindWechatModal 弹出链路（点 wechatCard → modal 出现 → 主按钮可定位），让"点击 → 状态变化 → modal 渲染"完整链路有 baseline；这与 Story 37.10 仅验证视觉锚不同 —— BindWechatModal 是本 story 独有的"非默认渲染状态"，不通过 UITest 兜底就只能靠 #Preview 视觉抽样。

> **关键决策**：UITest 路径**不需要** `UITEST_FORCE_PROFILE_BOUND` 类似 env flag —— Profile Tab 不依赖任何登录态；启动后切 tab 即可见全部锚（已绑定卡在 `wechatBound = true` 时才渲染 —— 但本 UITest 仅验证未绑定卡 + Modal 链路，让"已绑定卡"a11y 锚在 review 路径或后续 epic 真接 OAuth 时再覆盖）。

> **现有 testHomeScaffoldShowsAllSevenAnchors / testRoomScaffoldShowsAllSevenAnchors / testWardrobeScaffoldShowsAllAnchors / testFriendsScaffoldShowsAllAnchors**（Story 37.7 / 37.8 / 37.9 / 37.10）**不动** —— 本 story 范围是 Profile Tab，不影响其它 Tab UITest。

**对应 Tasks**: Task 8.1

### AC9 — xcodegen regen + build verify

完成 AC1-AC8 后：

1. `cd iphone && xcodegen generate` 让新文件加入 PetApp / PetAppTests target（project.yml `sources: - PetApp` / `- PetAppTests` 通配规则自动 inclusion；新增文件全部在 `iphone/PetApp/Features/Profile/` + `iphone/PetAppTests/Features/Profile/` 下）
2. `bash iphone/scripts/build.sh --test` 跑测试通过
   - 总 case 数：~305（Story 37.10 落地后基线 305 unit + 5 UITest）+ 本 story 新增 11 unit case + 1 UITest case → ~316 unit + 6 UITest case 全绿
   - 不删除任何老 case
3. grep 验证：
   - `grep -c "class ProfileViewModel" iphone/PetApp/Features/Profile/ViewModels/ProfileViewModel.swift` ≥ 1（防漏建基类）
   - `grep "fatalError" iphone/PetApp/Features/Profile/ViewModels/ProfileViewModel.swift` 输出至少 5 次（5 个 abstract method）
   - `grep "final class ProfileViewModel" iphone/PetApp/Features/Profile/ViewModels/ProfileViewModel.swift` 输出空（基类不能 final）
   - `grep -c "override func" iphone/PetApp/Features/Profile/ViewModels/MockProfileViewModel.swift` ≥ 5（5 个 override）
   - `grep -c "override func" iphone/PetApp/Features/Profile/ViewModels/RealProfileViewModel.swift` ≥ 5
   - **关键**：`grep -c "showBindModal = true\|wechatBound = true\|lastToastMessage =" iphone/PetApp/Features/Profile/ViewModels/RealProfileViewModel.swift` ≥ 5（Real override 必 mutate state，按 Story 37.9 round 1 P1 lesson；5 个 override 至少各 mutate 1 个字段）
   - `grep -c "ProfileScaffoldView" iphone/PetApp/Features/Profile/Views/ProfileView.swift` ≥ 1（caller 替换已生效）
   - `grep "Text(\"Profile Tab Placeholder\")" iphone/PetApp/Features/Profile/Views/ProfileView.swift` 输出空（旧占位 Text 已替换）

> **dev 实装备注**：dev 必须 `xcodegen generate` 后 commit 一并提交 `iphone/PetApp.xcodeproj/project.pbxproj` 改动（与 Story 37.5-37.10 同模式）。

**对应 Tasks**: Task 9.1, 9.2, 9.3

### AC10 — Deliverable 清单

- ✅ `iphone/PetApp/Features/Profile/ViewModels/ProfileViewModel.swift` 新建（class + 5 字段 + 5 abstract method fatalError 占位 + 0 concrete method + 0 derived computed property + parameterless init）
- ✅ `iphone/PetApp/Features/Profile/ViewModels/MockProfileViewModel.swift` 新建（final + invocations + 默认 ScaffoldDefaults seed + 可注入构造）
- ✅ `iphone/PetApp/Features/Profile/ViewModels/RealProfileViewModel.swift` 新建（final + appState 构造注入 + parameterless init + bind(appState:) + Set<AnyCancellable> + CombineLatest sink + 5 override **本地 mutate state**）
- ✅ `iphone/PetApp/Features/Profile/ViewModels/ProfileScaffoldDefaults.swift` 新建（profile / wechatBound / recentCollections 3 字段共享）
- ✅ `iphone/PetApp/Features/Profile/Models/ProfileSummary.swift` 新建（10 字段 + Equatable + Sendable）
- ✅ `iphone/PetApp/Features/Profile/Models/RecentCollection.swift` 新建（id / name / rarity / emoji + Equatable + Identifiable + Sendable）
- ✅ `iphone/PetApp/Features/Profile/Models/ProfileMenuItem.swift` 新建（4 case + label / iconKey / extraText computed properties + CaseIterable + Identifiable + Sendable）
- ✅ `iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift` 新建（struct + 5 区块视觉按 ui_design profile.jsx 像素级翻译 + BindWechatModalView 子视图 + #Preview 4 配置 candy/dark × 默认/已绑定/Modal-open 场景）
- ✅ `iphone/PetAppTests/Features/Profile/ProfileViewScaffoldTests.swift` 新建（11 case：6 epic AC + 5 守护 case 预防 lesson 反例）
- ✅ `iphone/PetApp/Features/Profile/Views/ProfileView.swift` 修改（占位 Text 替换为 ProfileScaffoldView + 加 @EnvironmentObject）
- ✅ `iphone/PetApp/App/RootView.swift` 修改（追加 `@StateObject profileViewModel: ProfileViewModel = RealProfileViewModel()` + LaunchedContentView 透传 + `.onAppear` 内同步 bind(appState:) + `.environmentObject` 注入）
- ✅ `iphone/PetApp/App/MainTabView.swift` 修改（Preview 注入 MockProfileViewModel）
- ✅ `iphone/PetAppUITests/HomeUITests.swift` 加 `testProfileScaffoldShowsAllAnchors`（含 Modal 触发链路）
- ✅ `iphone/PetApp.xcodeproj/project.pbxproj` 改动随 commit（xcodegen regen 结果）
- ✅ `bash iphone/scripts/build.sh --test` 全绿（~316 unit + 6 UITest case）
- ✅ project.yml **不**手动改（通配规则自动 inclusion）
- ✅ RootView wire 用 RealProfileViewModel 而非裸基类（防 fatalError 生产 crash 路径，按 Story 37.7 lesson 钦定）
- ✅ RealProfileViewModel 5 个 override **本地 mutate state**（按 Story 37.9 round 1 P1 lesson 钦定）
- ✅ `ProfileView.swift` **不**删除（保 git history；Story 37.13 决定）
- ✅ MainTabView 内 `ProfileView()` 调用站不变（caller 漏改靠编译器报错驱动 —— ProfileView 类型签名不变；body 内部改）


## Tasks / Subtasks

- [x] Task 1: ProfileViewModel 基类（AC1）
  - [x] 1.1 新建 `iphone/PetApp/Features/Profile/ViewModels/ProfileViewModel.swift`：`@MainActor public class ProfileViewModel: ObservableObject` + 5 个 @Published 字段（profile / wechatBound / recentCollections / showBindModal / lastToastMessage）+ 5 abstract method (onWeChatCardTap / onWeChatBindConfirmTap / onWeChatModalDismissTap / onMenuTap(item:) / onCollectionViewAllTap) fatalError 占位 + 0 concrete method + 0 derived computed property + parameterless init()
  - [x] 1.2 显式 `import Foundation` + `import Combine`（防 transitive @Published；与 MockHomeViewModel round 4 [P0] hardening 同精神）
- [x] Task 2: Mock/Real 子类（AC2）
  - [x] 2.1 新建 `iphone/PetApp/Features/Profile/ViewModels/MockProfileViewModel.swift`（final class + invocations 数组 + 5 override + 默认 ScaffoldDefaults seed + 可配 init）
  - [x] 2.2 新建 `iphone/PetApp/Features/Profile/ViewModels/RealProfileViewModel.swift`（final class + appState 构造注入 + parameterless init() / init(appState:) 双路径 + bind(appState:) idempotent + Set<AnyCancellable> + CombineLatest 订阅 currentUser+currentPet 派生 profile + 5 override **本地 mutate** + lastToastMessage 写入；按 Story 37.9 round 1 P1 lesson 钦定路径）
- [x] Task 3: 数据模型 + ScaffoldDefaults（AC3）
  - [x] 3.1 新建 `iphone/PetApp/Features/Profile/Models/ProfileSummary.swift`（struct value type + 10 字段 + Equatable + Sendable）
  - [x] 3.2 新建 `iphone/PetApp/Features/Profile/Models/RecentCollection.swift`（struct + 4 字段 + Equatable + Identifiable + Sendable；rarity 字段类型走 Story 37.6 落地的 Rarity enum）
  - [x] 3.3 新建 `iphone/PetApp/Features/Profile/Models/ProfileMenuItem.swift`（enum 4 case + label / iconKey / extraText computed properties + CaseIterable + Identifiable + Sendable）
  - [x] 3.4 新建 `iphone/PetApp/Features/Profile/ViewModels/ProfileScaffoldDefaults.swift`（3 字段：profile / wechatBound / recentCollections；profile mock 字段值与 wechat_binding.md §"数据风险清单"对齐）
- [x] Task 4: ProfileScaffoldView struct + 5 区块 + BindWechatModal sheet（AC4）
  - [x] 4.1 新建 `iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift`，含 ScrollView { VStack(spacing: 0) 5 区块结构 } + accessibilityIdentifier "profileView" + .overlay(alignment: .bottom) toastOverlay + .sheet(isPresented: $state.showBindModal) bindWechatSheet
  - [x] 4.2 落地 headerCard 子视图（顶部渐变背景 + HStack 标题"我的"+ 右 2 个 IconButton bell/settings + 下方 Avatar(name: profile.name, size: 72, ring: true) + VStack 名字/ID/title/joinedAt 药丸 + accessibilityIdentifier `profileHeaderCard`）
  - [x] 4.3 落地 statsCard 子视图（4 列 Stat + 3 个自绘 Rectangle Divider；负 marginTop -34 覆盖头图 + accessibilityIdentifier `profileStatsCard`）
  - [x] 4.4 落地 wechatCard 子视图（双态分支 wechatBound true/false；未绑定走 LinearGradient 黄色警告卡 + 整张可点击调 onWeChatCardTap；已绑定走 surface 绿色确认卡纯展示；accessibilityIdentifier `profileWeChatCard` / `profileWeChatCardBound`）
  - [x] 4.5 落地 recentCollections 子视图（SectionHeader "最近收藏 / 查看全部" + ScrollView(.horizontal) HStack 5 件 cell + accessibilityIdentifier `profileCollectionViewAll` / `profileCollectionCell_<rc.id>`）
  - [x] 4.6 落地 menuList 子视图（SectionHeader "更多" + VStack 4 行 ProfileMenuItem.allCases ForEach；每行调 onMenuTap(item:) + accessibilityIdentifier `profileMenu_\(item.rawValue)`）
  - [x] 4.7 落地 BindWechatModalView 子视图（在同一文件内）：警告插画 + 标题 + 正文（含红色高亮）+ 数据风险清单 4 行 DataLossRow（绑定 state.profile.* 字段动态填充）+ 按钮组 主按钮调 onWeChatBindConfirmTap accessibilityIdentifier `profileWeChatBindButton` + 次按钮调 onWeChatModalDismissTap accessibilityIdentifier `profileWeChatCancelButton`；Modal 容器 accessibilityIdentifier `profileWeChatModal`；用 `.presentationDetents([.fraction(0.85)])` + `.presentationCornerRadius(28)`
  - [x] 4.8 落地 toastOverlay（与 Story 37.10 同精神；底部黑底 alpha 0.85；accessibilityIdentifier `profileToast`）
  - [x] 4.9 **lesson 预防性应用**：所有 shadow 必须挂在 `RoundedRectangle.fill(...).shadow(...)` / `Circle().fill(...).shadow(...)` 那一层，按 Story 37.6 round 5 lesson 钦定路径
- [x] Task 5: ProfileView caller 替换 + RootView wire + LaunchedContentView 透传 + MainTabView Preview（AC5）
  - [x] 5.1 改 `ProfileView.swift`：body 占位 Text 替换为 `ProfileScaffoldView(state: profileViewModel)`；加 `@EnvironmentObject var profileViewModel: ProfileViewModel`；保留 NavigationStack 包裹层
  - [x] 5.2 改 `RootView.swift`：在 `@StateObject friendsViewModel` 同级追加 `@StateObject private var profileViewModel: ProfileViewModel = RealProfileViewModel()` + LaunchedContentView 调用站追加 `profileViewModel: profileViewModel`
  - [x] 5.3 改 LaunchedContentView 内部签名：追加 `profileViewModel: ProfileViewModel` 字段 + init 参数 + body 内 `.environmentObject(profileViewModel)`（与 4 个已有 @EnvironmentObject 同模式）
  - [x] 5.4 改 `RootView.swift` `.onAppear`：在已有 `if let realFriendsVM = ... { realFriendsVM.bind(appState:) }` 后追加 `if let realProfileVM = profileViewModel as? RealProfileViewModel { realProfileVM.bind(appState: appState) }`（按 Story 37.8 round 2 P2 lesson 钦定路径，**不**放 `.task`）
  - [x] 5.5 改 `MainTabView.swift` Preview：在已有 `.environmentObject(MockFriendsViewModel() as FriendsViewModel)` 同级追加 `.environmentObject(MockProfileViewModel() as ProfileViewModel)`
- [x] Task 6: #Preview 4 配置（AC6）
  - [x] 6.1 ProfileScaffoldView 文件底部 `#if DEBUG` 块加 4 个 `#Preview`（candy 默认 / dark 默认 / candy 已绑定 / candy bind-modal-open）
- [x] Task 7: 单元测试（AC7）
  - [x] 7.1 新建 `iphone/PetAppTests/Features/Profile/ProfileViewScaffoldTests.swift`，落地 11 case（6 epic AC + 5 守护 case 预防 lesson 反例 —— 含 lesson 1/2/4/5/6 全部命中）
- [x] Task 8: UITest（AC8）
  - [x] 8.1 在 `HomeUITests.swift` 加 `testProfileScaffoldShowsAllAnchors`（不需要 env flag；切 tab 即可见 + 主动 tap wechatCard 触发 modal 链路）
  - [x] 8.2 验证现有 `testHomeScaffoldShowsAllSevenAnchors` / `testRoomScaffoldShowsAllSevenAnchors` / `testWardrobeScaffoldShowsAllAnchors` / `testFriendsScaffoldShowsAllAnchors` 不受影响（不动）
- [x] Task 9: xcodegen regen + build verify（AC9）
  - [x] 9.1 `cd iphone && xcodegen generate` 让新文件加入 target
  - [x] 9.2 `bash iphone/scripts/build.sh --test` 跑测试通过（~316 unit + 6 UITest case 全绿）
  - [x] 9.3 grep 校验：ProfileViewModel 含 `class ProfileViewModel`（去 final）+ ≥5 个 fatalError；MockProfileViewModel / RealProfileViewModel 各含 ≥5 个 override func；RealProfileViewModel 含 ≥5 个 mutation pattern（守护 lesson 6）；ProfileView 含 ProfileScaffoldView 调用 + 不含 `Text("Profile Tab Placeholder")` 调用
- [x] Task 10: Deliverable 清单确认（AC10）
  - [x] 10.1 9 个新文件 + 修改 4 个老文件（ProfileView.swift / RootView.swift / MainTabView.swift / HomeUITests.swift）+ pbxproj regen 全部待 commit（不在本 dev-story 范围）


## Dev Notes

### Story 37.7 / 37.8 / 37.9 / 37.10 沉淀 lesson 预防性应用清单（关键约束 —— 全部 9 条命中）

本 story 落地前必读 9 条 lesson；**不重蹈覆辙**清单（与 epic-loop 调用提示中 9 条 lesson 一一对应）：

| # | Lesson 文件 | 预防点 | 本 story 落地动作 |
|---|---|---|---|
| 1 | `2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md` | abstract method base class 注入点全部要换 concrete subclass | RootView `@StateObject profileViewModel: ProfileViewModel = RealProfileViewModel()` 而非裸基类（AC5 Task 5.2） |
| 2 | `2026-04-30-published-derived-state-needs-publisher-subscription.md` | 派生 state 必须订阅 publisher，禁止 hardcode（避免 reset 后 stale） | RealProfileViewModel 用 `Publishers.CombineLatest` 订阅 appState.$currentUser + $currentPet 派生 profile；**不**在 init / bind 入口一次性 hydrate（AC2 + 守护 case#9） |
| 3 | `2026-04-30-room-host-name-must-not-derive-from-local-current-pet.md` | 不要从 currentPet 派生其他用户的信息（local pet ≠ remote owner） | **本 story 反向应用 lesson** —— Profile 域 profile.name / profile.petName 派生自 appState.currentUser.nickname / currentPet.name **是合法**的（"我的资料"语义就是本地用户的资料）；**反例**：在 Profile 域**不**派生其他用户的资料；详见 Dev Notes "profile 派生源 vs hostCatName 反例" |
| 4 | `2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md` | RealViewModel.init 必须 seed scaffold defaults（避免 in-room/in-X state 出现空内容） | 新建 `ProfileScaffoldDefaults` 共享 enum；Mock / Real 双子类 init 都用它 seed 全 3 字段（AC2 + AC3 + 守护 case#8） |
| 5 | `2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md` | `.onAppear` 同步 bind appState（避免 launch-time race） | RootView `.onAppear` 内追加 `realProfileVM.bind(appState: appState)`，**不**放 `.task`（AC5 Task 5.4 + 守护 case#10） |
| **6** | **`2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`** | **RealViewModel.override 占位方法必须实装本地 mutate state（log-only 是 [P1]）** | **RealProfileViewModel 5 个 override 必须 mutate state（showBindModal / wechatBound / lastToastMessage 各种组合）+ 写 lastToastMessage（AC2 + 守护 case#7）；grep 校验 mutation pattern 在 RealProfileViewModel.swift 内出现 ≥ 5 次（AC9）—— 这条是本 story SM 重点强调的 lesson** |
| 7 | `2026-04-30-swiftui-onchange-equatable-and-stale-task-cancel.md` | SwiftUI .onChange iOS 17+ 双参签名 | 本 story ProfileScaffoldView 不主动用 .onChange（无 timer 类视觉 transient）；如未来 BindWechatModal 加"3 秒自动消失" toast 必须按 iOS 17+ 双参签名 |
| 8 | `2026-04-30-swiftui-explicit-id-nil-shared-identity.md` + `2026-04-30-swiftui-state-survives-id-and-shadow-over-children.md` + `2026-04-30-swiftui-floating-emoji-needs-state-driven-position.md` | shadow / .id() / @State 驱动浮动动画 等 SwiftUI primitives 注意点 | headerCard / wechatCard / IconButton / Stat / DataLossRow shadow 全部挂 `RoundedRectangle.fill(...).shadow(...)` 那一层（AC4 Task 4.9）；ForEach 用 `state.recentCollections` / `ProfileMenuItem.allCases` 自带 .id；本 story 无浮动动画路径 |
| 9 | **spec 边界灰色区域：epic AC 与 ui_design 实物冲突时，遵循 epic AC（如 share button 等次级 action 不要漏）** | epic AC 钦定的次级 action 不要漏掉（即便 ui_design 视觉略简） | 本 story epic AC line 4831-4835 钦定 5 区块全部落地（顶部头图 / 统计卡 / 微信卡 / 最近收藏 / 4 项菜单）+ Modal 视觉壳；**特别强制**：Modal 内"绑定微信"按钮 + "稍后再说"按钮**两者都落**（即便 ui_design 视觉只突出主按钮）；**特别强制**：headerCard 右上角 bell + settings 两个 IconButton **两者都落**（即便单元测试只覆盖菜单点击）；详见 Dev Notes "epic AC 边界灰区清单" |

### profile 派生源 vs Story 37.8 hostCatName 反例（关键澄清表 —— lesson 3 命中说明）

Story 37.8 round 3 lesson `2026-04-30-room-host-name-must-not-derive-from-local-current-pet.md` 指出：**Room 域 hostCatName 不可派生自 appState.currentPet（"本地用户的猫" ≠ "房间 host 的猫"）**。但**本 story Profile 域 profile.name / profile.petName 派生自 appState.currentUser.nickname / currentPet.name 是合法的**。语义独立：

| 维度 | Story 37.8 RoomViewModel.hostCatName | Story 37.11 ProfileViewModel.profile.name / .petName |
|---|---|---|
| 域语义 | "看 room host 的小屋"——host 可能是别人 | "我的资料"——name / petName 永远是本地用户自己 |
| 真理源 | WS room.snapshot（Story 12.1 后到来） | appState.currentUser / appState.currentPet（"本地用户的某条信息"语义钦定） |
| pre-feature 占位 | RoomScaffoldDefaults.hostCatName（永远占位直到 WS 接通） | ProfileScaffoldDefaults.profile（默认 mock；hydrate 后即派生） |
| 是否 sink 订阅 currentXxx | ❌ 错（用户加入别人房间时显示"我的猫的小屋"是 user-visible bug） | ✅ 对（"我的资料"无歧义就是本地用户自己的，appState.currentUser/Pet 就是真理源） |

> **关键判断标准**：lesson 3 的精神是"判断目标 X 的真理源是不是'本地用户的某条信息'，是 → 可派生；否 → 用 placeholder"。Profile 域的"我的资料"无歧义就是本地用户的；Profile 域的"统计卡数字"（collectionsCount / friendsCount / achievementsCount / coinsCount）也是本地用户的（**但本 story 不接 server**——后续 epic 真接 `/profile/me` 时改 sink 派生）。

### lesson 6 命中说明（关键 —— Story 37.9 第一次复犯，本 story 同 Story 37.10 一样不能重犯）

Story 37.9 round 1 codex review 抓出 [P1]：`RealWardrobeViewModel.onEquipTap` 仅 log，**不**改 equipped 字段，导致 production app 用户点装备按钮 no-op。该 lesson 的核心规则：

> **Real 子类 override abstract method 时必须实装"本 story 范围内能让 UI 视觉工作的最小 placeholder 行为"**，禁止只 log。占位行为通常**与 Mock 子类同语义 copy**。Server / UseCase 真实写入是未来 story 的事，**本 story 范围内 production app 必须可用**。

**本 story 强制应用**（5 个 override 全部）：
- `RealProfileViewModel.onWeChatCardTap` 必须本地 mutate `showBindModal = true`
- `RealProfileViewModel.onWeChatBindConfirmTap` 必须本地 mutate `wechatBound = true` + `showBindModal = false` + `lastToastMessage`
- `RealProfileViewModel.onWeChatModalDismissTap` 必须本地 mutate `showBindModal = false`
- `RealProfileViewModel.onMenuTap(item:)` 必须本地 mutate `lastToastMessage = "{item.label}（敬请期待）"`
- `RealProfileViewModel.onCollectionViewAllTap` 必须本地 mutate `lastToastMessage`
- 守护 case#7 显式断言每个 override 后字段值变化 —— 未来 Claude 重构若改回 log-only，本测试立即 fail
- AC9 grep 校验 mutation pattern 在 RealProfileViewModel.swift 内出现 ≥ 5 次 —— 让"override 必 mutate"契约钉成机器可校验的规则

> **关键决策（与 Story 37.10 略不同）**：本 story 5 个 mutation **不**走 `appState.setXxx(...)` 入口（因为 wechatBound / showBindModal / lastToastMessage 是 Profile 域 ViewModel-only state，不进 AppState 白名单 ADR-0010 §3.2）；直接写 `self.wechatBound = true` 等即可。这是合法路径 —— 兄弟 ViewModel（Home/Room/Wardrobe/Friends）都不订阅 wechatBound / showBindModal，不存在"漏触发兄弟 sink"问题。如果未来 wechatBound 进 AppState（持久化语义所需），再改走 setter 入口。

### 1.2s 自动弹 Modal timer 决策（边界灰区）

ui_design `profile.jsx:7-12` 钦定：「进入页面后 1.2s 自动弹出绑定提醒（仅当未绑定）」+ wechat_binding.md §"触发规则"钦定 24 小时再次弹一次（@AppStorage 时间戳）。

**SM 决策**：本 story **不落** 1.2s 自动弹 Modal 行为，理由：
1. **数据流复杂度**：1.2s timer + @AppStorage("lastWechatPromptAt") + 24 小时去重逻辑超本 story Scaffold 范围（属"行为"而非"视觉壳"）
2. **测试复杂度**：单元测试需要假时钟 / @AppStorage 注入；本 story XCTest only 路径（ADR-0002 §3.1）落地这层会拉测试基础设施成本
3. **后续 epic 边界**：真实 OAuth 落地的 epic（Post-MVP 微信绑定）整体把"自动弹 Modal + 24 小时去重 + WXApi.sendAuthReq + @AppStorage 持久化"作为完整 vertical slice 实装更合理

**dev / reviewer 协商点**：
- 本 story 用户体验：用户进 Profile Tab 必须**手动点"绑定微信卡"**才弹 Modal —— 卡片视觉本身已突出（黄色警告 + "立即绑定"绿胶囊按钮 + 数据丢失警示文字），让用户主动触发是合理的
- 如果 review 反馈要求 1.2s 自动弹 → fix-review round 加：在 ProfileScaffoldView 加 `@State private var hasShownAutoModal = false` + `.onAppear { if !state.wechatBound && !hasShownAutoModal { Task { try? await Task.sleep(nanoseconds: 1_200_000_000); state.showBindModal = true; hasShownAutoModal = true } } }`（注意 lesson 7 .onChange 双参签名仅在用 .onChange 时才必须；本 .onAppear + Task.sleep 路径不踩 lesson 7）

> **关键决策**：把这个边界灰区**显式声明在 spec 内**（而非偷偷砍掉）—— 让 dev / reviewer 看到"为什么 1.2s 自动弹没落"+ 让 fix-review 路径有迹可循（与 Story 37.10 myRoomCard 分享按钮决策同精神）。

### epic AC 边界灰区清单（lesson 9 命中说明）

epic AC line 4829-4835 钦定的元素**全部落地**，即便 ui_design 视觉略简：

| epic AC 元素 | 落地路径 | ui_design 视觉一致性 |
|---|---|---|
| 顶部渐变头图 + Avatar + 用户名 + 用户 id + 称号 + "加入于"小药丸 | headerCard 全量（AC4 Task 4.2） | 1:1 |
| 统计卡 4 列（收藏品 / 好友 / 小猫等级 / 成就） | statsCard 全量（AC4 Task 4.3） | 1:1（含 4 列 + 3 Divider） |
| 最近收藏：横向 5 个 Card | recentCollections 全量（AC4 Task 4.5） | 1:1（5 件 cell + SectionHeader） |
| 微信绑定卡 双态（按 ui_design/wechat_binding.md 视觉） | wechatCard 全量（AC4 Task 4.4） | 1:1（黄色警告卡 + 绿色确认卡） |
| 绑定 Modal：警告图标 + 高亮红字 + "绑定微信"按钮 | BindWechatModal 全量（AC4 Task 4.7） | 1:1（含 4 行风险列表 + 2 个按钮） |
| 菜单列表 4 项（成就徽章 / 消息通知 / 喜欢的道具 / 设置） | menuList 全量（AC4 Task 4.6） | 1:1（4 项 + extra 文字） |
| **次级 action（即便 ui_design 视觉只突出主按钮）** | "稍后再说"按钮 / bell + settings IconButton 两者都落 | epic AC 优先 |

> **关键决策**：lesson 9 的精神是"epic AC 与 ui_design 实物冲突时，遵循 epic AC"。本 story 把所有次级 action（含"稍后再说"次按钮 + headerCard bell/settings IconButton）显式纳入 AC4 落地清单，**不**因为 ui_design 视觉简洁就砍掉。SM 不接受"砍掉次按钮让卡片更紧凑"的妥协 —— 用户能看到"稍后再说"是产品逻辑必要点（让用户感觉自主选择 vs 强迫绑定）。

### ProfileViewModel 改 class 而非 protocol any 模式（关键设计决策）

**选定**: 基类 `class ProfileViewModel: ObservableObject`（非 final）+ 子类 `MockProfileViewModel: ProfileViewModel` / `RealProfileViewModel: ProfileViewModel` 各自 final。

**为何不走 `protocol ProfileViewModelProtocol + any P`**：与 Story 37.7 / 37.8 / 37.9 / 37.10 同精神（v2.1 BLOCKER 7）—— SwiftUI `@ObservedObject` 不接受 `any P`；让 caller 端类型膨胀。

**为何 ProfileScaffoldView 选择非泛型 struct**（不像 HomeView<ChestSlot: View> 是泛型）：Profile 没有"chestSlot 接缝点"这种泛型必要场景；后续若加"成就页/消息页 NavigationLink slot"再走泛型 ViewBuilder 路径。

### ProfileView 占位 stub 不删保 git history（关键约束）

`ProfileView.swift` 在 Story 37.3 落地时作为 Profile Tab 占位 stub；本 story **不删除**该文件，理由（与 Story 37.10 FriendsView 同决策）：
1. **保 git history 可读**：dev 阅读 git log 能看到 `ProfileView` 是 Story 37.3 临时方案 → Story 37.11 替换 body 占位 Text 为 ProfileScaffoldView 的演进足迹
2. **MainTabView 调用站类型不变**：`MainTabView` 的 `ProfileView().tag(AppTab.profile)` 调用站签名不变；本 story 仅改 body 内部结构
3. **Story 37.13 a11y 总表归并时统一清理**：Story 37.13 决定是否一并清理 / 重命名（可能改名 `ProfileRootView` 等）；本 story 不收口

### state owner 边界：profile / wechatBound / showBindModal / lastToastMessage 走 ViewModel @Published 而非 @State

ADR-0010 §3.2 表格"表单输入 / 当前选中 → ViewModel 或 SwiftUI @State"二选一；判断标准是**是否需要跨 View 触发 / 单元测试需要断言**：

| 场景 | 选择 | 理由 |
|---|---|---|
| `profile` | ViewModel @Published | RealProfileViewModel 通过 sink 订阅 appState.$currentUser + $currentPet 派生写入，需要 SwiftUI 监听变化 |
| `wechatBound` | ViewModel @Published | 单元测试需要断言（case#3 绑定后 wechatBound 切到 true）；放 @State 让单元测试无法直接断言 |
| `showBindModal` | ViewModel @Published | 5 个 override 中 3 个写入此字段；需要测试断言（case#2 / case#3 / case#4 / case#7） |
| `lastToastMessage` | ViewModel @Published | 多个 override 都从 ViewModel 写入；需要测试断言（case#5 / case#6 / case#7） |

### RealProfileViewModel 字段策略（关键决策 + 后续 epic 接续点）

本 story `RealProfileViewModel` **只**为 `profile` 字段建 sink（订阅 appState.$currentUser + $currentPet 合并派生）—— `recentCollections` / `wechatBound` 两字段不建 sink：
- `recentCollections` 永远走 `ProfileScaffoldDefaults.recentCollections` seed（收藏接口在后续 epic 才落地）
- `wechatBound` 是 ViewModel 持有的 transient state（本期无持久化，无 server 字段）

**为何不预先建 sink**：① 后续 epic 真实接口形态未定（可能 `/collections/recent` REST endpoint，可能 GraphQL，可能 server push）—— 提前 hookup sink 等于让 dev 重写工作量；② 提前 mapping 是 over-engineer（参考 ADR-0010 §4.4 缓解策略）。

**接续点**：
- 未来 epic 落地真实最近收藏接口时新增 `subscribeRecentCollections(to: appState)` sink（或直接调 `LoadRecentCollectionsUseCase`）；其它 ViewModel / View 代码 zero edit
- 未来 epic 落地真实微信绑定时改 RealProfileViewModel.onWeChatBindConfirmTap 调真实 UseCase；wechatBound 可能进 AppState 白名单（持久化语义所需）—— 那时再加 setter 入口 + sink 派生

### 测试边界（XCTest only）

本 story 测试**仅**用 XCTest + @testable import PetApp + UITest（XCUIApplication）—— **不**引：

- ❌ SnapshotTesting（视觉 diff）：视觉验证靠 #Preview + Story 37.13 visual-review-checklist
- ❌ ViewInspector（SwiftUI body 内省）：ProfileScaffoldView body 渲染契约靠 ui_design 1:1 翻译 + Preview 抽样兜底
- ❌ Mockingbird / Cuckoo（mock codegen）：MockProfileViewModel 是手写 final class subclass

### Story 37.7 / 37.8 / 37.9 / 37.10 衔接：与 Home/Room/Wardrobe/Friends 同 patterns 全表

| 维度 | HomeView (37.7) | Room (37.8) | Wardrobe (37.9) | Friends (37.10) | Profile (本 story) |
|---|---|---|---|---|---|
| 文件命名 | HomeView 改写 | RoomScaffoldView 新建（保旧 placeholder） | WardrobeScaffoldView 新建（保旧 view） | FriendsScaffoldView 新建（保旧 view） | ProfileScaffoldView 新建（保旧 view） |
| struct 签名 | 泛型 HomeView<ChestSlot: View> | 非泛型 | 非泛型 | 非泛型 | **非泛型** |
| state owner | @ObservedObject base class | @ObservedObject base class | @ObservedObject base class | @ObservedObject base class | @ObservedObject base class |
| ViewModel 基类 | class + 5 字段 + 5 abstract | class + 4 字段 + 2 abstract | class + 5 字段 + 1 abstract + 2 concrete + 3 derived | class + 4 字段 + 2 abstract + 1 concrete + 4 derived | **class + 5 字段 + 5 abstract + 0 concrete + 0 derived** |
| Mock 子类 | MockHomeViewModel | MockRoomViewModel | MockWardrobeViewModel | MockFriendsViewModel | **MockProfileViewModel** |
| Real override 行为 | mutate showJoinModal / interactionAnimation | setCurrentRoomId(nil) | local toggle equipped (round 1 P1 fix) | appState.setCurrentRoomId via 入口 | **direct mutate showBindModal/wechatBound/lastToastMessage（不走 appState 入口；ADR-0010 §3.2 ViewModel-only state）** |
| Defaults 共享 enum | （未抽） | RoomScaffoldDefaults | WardrobeScaffoldDefaults | FriendsScaffoldDefaults | **ProfileScaffoldDefaults** |
| 数据模型 | PetStats / AnimationState | RoomMember | CosmeticItem / CosmeticCategory | Friend / FriendStatus / FriendsTab | **ProfileSummary / RecentCollection / ProfileMenuItem** |
| 区块数 | 5 | 5 | 4 | 5（含 toastOverlay） | **5（含 toastOverlay + BindWechatModal sheet）** |
| @State (transient) | resetTask | copiedFeedback / copyFeedbackTask | (无) | (无) | (无) |
| Sink 模式 | (Story 37.7 不需) | currentRoomId 单 sink | currentEquips 单 sink | currentRoomId 单 sink | **CombineLatest currentUser + currentPet（双 publisher 合并）** |
| 老占位文件处理 | HomeView 改写 | RoomViewPlaceholder 不删 | WardrobeView 不删 | FriendsView 不删 | **ProfileView 不删** |
| caller 改动 | bridge 改 init | bridge 新增 | body 直接改 | body 改 + 加 @EnvironmentObject | **body 改 + 加 @EnvironmentObject** |
| RootView wire | RealHomeViewModel | + RealRoomViewModel | + RealWardrobeViewModel | + RealFriendsViewModel | **+ RealProfileViewModel** |
| .onAppear bind | bind appState | + realRoomVM.bind | + realWardrobeVM.bind | + realFriendsVM.bind | **+ realProfileVM.bind** |
| #Preview 数 | 2 | 4 | 4 | 4 | **4** |
| 单元测试 case 数 | 6 | 5 | 10 | 11 | **11** |
| UITest case | testHomeScaffoldShowsAllSevenAnchors | testRoomScaffoldShowsAllSevenAnchors + UITEST_FORCE_IN_ROOM env | testWardrobeScaffoldShowsAllAnchors + 切 tab | testFriendsScaffoldShowsAllAnchors + 切 tab | **testProfileScaffoldShowsAllAnchors + 切 tab + tap wechatCard 触发 modal 链路** |
| a11y identifier | 7 锚 | 8 锚 | 12+ 锚 | 8+ 锚 | **10+ 锚（profileView / Header / Stats / WeChatCard / WeChatCardBound / WeChatModal / WeChatBindButton / WeChatCancelButton / CollectionCell_*5 / CollectionViewAll / Menu_*4 / Toast）** |

### a11y identifier 命名约定

本 story ProfileScaffoldView 内 inline a11y identifier 字符串（与 Story 37.7-37.10 同精神，Story 37.13 一次性归并到 `AccessibilityID.Profile`）：

| identifier | 位置 | 备注 |
|---|---|---|
| `profileView` | ProfileScaffoldView 主容器 | 与 Story 37.3 占位 stub 字符串一致 → Tab 切换 UITest 不破 |
| `profileHeaderCard` | headerCard 容器 | 渐变头图 + Avatar + 用户名 |
| `profileStatsCard` | statsCard 容器 | 4 列统计 |
| `profileWeChatCard` | wechatCard 未绑定卡（Button） | wechatBound == false 时存在 |
| `profileWeChatCardBound` | wechatCard 已绑定卡（HStack） | wechatBound == true 时存在 |
| `profileWeChatModal` | BindWechatModal 容器 | showBindModal == true 时存在 |
| `profileWeChatBindButton` | Modal "绑定微信，保护数据"主按钮 | epic AC line 4838 钦定 |
| `profileWeChatCancelButton` | Modal "稍后再说"次按钮 | epic AC 边界灰区清单钦定 |
| `profileCollectionViewAll` | "查看全部"右侧按钮 | 调 onCollectionViewAllTap |
| `profileCollectionCell_<rc.id>` | recentCollections cell | mock data id `rc-1` 至 `rc-5` |
| `profileMenu_<item.rawValue>` | menuList 4 行 | epic AC line 4838 钦定（4 项：achievements / messages / favorites / settings） |
| `profileToast` | toastOverlay（条件渲染） | lastToastMessage nil 时不存在 |

### 与 后续 epic 衔接的红线（关键约束）

后续 epic 真实微信 OAuth / 收藏数据 / 成就消息接口落地路径：
- 把 RealProfileViewModel.onWeChatBindConfirmTap 的 `wechatBound = true` 替换为 `BindWechatUseCase.execute()` 调用（成功后 server 写入 + 拉真实状态写 wechatBound）
- 把 `recentCollections` 走 ScaffoldDefaults seed 的路径替换为 `subscribeRecentCollections(to: appState)` sink（订阅 appState.$recentCollections 字段，需要先在 ADR-0010 §3.2 加 recentCollections 进白名单 + AppState 字段 + LoadRecentCollectionsUseCase 写入）
- 把 menuList 4 个 onMenuTap 占位 toast 替换为 NavigationLink push 到具体子页面（AchievementsView / MessagesView / FavoritesView / SettingsView）
- 1.2s 自动弹 Modal + 24 小时去重 + @AppStorage("lastWechatPromptAt") 实装（本 story 不落，详见 Dev Notes "1.2s 自动弹 Modal timer 决策"）
- ProfileScaffoldView 视图 zero edit
- AppState / Profile 类型契约 zero edit（仅 AppState 加 recentCollections 字段时联动）

> **关键决策**：本 story **不**预先加 BindWechatUseCase / LoadRecentCollectionsUseCase 接口字段 —— 后续 epic 实装时根据真实 UseCase shape 决定字段；预 over-design 反而让后续 epic dev 在 mapping 路径上重写浪费工作量（参考 ADR-0010 §4.4 缓解策略）。

### Project Structure Notes

- 新建目录 `iphone/PetApp/Features/Profile/ViewModels/` + `iphone/PetApp/Features/Profile/Models/`（已有 `iphone/PetApp/Features/Profile/Views/`）
- 新建目录 `iphone/PetAppTests/Features/Profile/`
- 全部走 `iphone/project.yml` 通配 inclusion；不改 project.yml

### References

- [Source: docs/宠物互动App_总体架构设计.md] —— 总体架构与产品规则
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md] —— iOS 工程目录结构（Features/Profile/ViewModels|Models|Views/ 三层）
- [Source: _bmad-output/planning-artifacts/epics.md §Story 37.11] —— 本 story epic AC（line 4819-4839）
- [Source: _bmad-output/planning-artifacts/sprint-change-proposal-2026-04-29-v2.md §8.2] —— PM 签字位（微信绑定 UI 视觉壳不突破 PRD §4 「不实现 UI」边界）
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md §3.1] —— ADR-0002 测试栈钦定（XCTest only）
- [Source: _bmad-output/implementation-artifacts/decisions/0009-ios-navigation.md §3.5] —— ADR-0009 主入口 4 Tab（Profile Tab 直接路由）
- [Source: _bmad-output/implementation-artifacts/decisions/0010-app-state.md §3.1 §3.2 §3.5] —— ADR-0010 ViewModel 注入规则 + AppState 范围白名单 + state owner 边界
- [Source: iphone/ui_design/source/screens/profile.jsx] —— 5 区块视觉源（line 1-323 全文）
- [Source: iphone/ui_design/wechat_binding.md] —— 微信绑定卡 + Modal 详细 UI 规格（line 1-167 全文）
- [Source: iphone/ui_design/README.md §ProfileScreen] —— ProfileScreen 概述
- [Source: iphone/PetApp/Features/Friends/Views/FriendsScaffoldView.swift] —— Story 37.10 落地的 FriendsScaffoldView，本 story 1:1 复刻 class 层次模式 + sink + Defaults seed 模式 + Real override 必 mutate state（lesson 6 守护）
- [Source: iphone/PetApp/Features/Friends/ViewModels/*] —— class 层次 + Mock/Real 三件套 + ScaffoldDefaults 共享 enum 参考实现
- [Source: iphone/PetApp/Features/Profile/Views/ProfileView.swift] —— Story 37.3 落地的占位 stub（本 story 改 body 内部）
- [Source: iphone/PetApp/Core/DesignSystem/Primitives/Avatar.swift / Card.swift / PrimaryButton.swift / Icons.swift / RarityTag.swift] —— Story 37.6 落地的 primitives，本 story 复用（Avatar 含 ring 光环；Icons.wechat / shield / warn / bell / settings / diamond / friends / paw / trophy / heart / chevronRight / sparkle）
- [Source: iphone/PetApp/Core/DesignSystem/Theme.swift] —— Story 37.5 Theme tokens
- [Source: iphone/PetApp/App/RootView.swift / MainTabView.swift] —— RootView wire 模式 + MainTabView 4 Tab 路由
- [Source: iphone/PetApp/App/AppState.swift] —— Story 37.4 AppState 7 字段（含 currentUser / currentPet）
- [Source: iphone/PetApp/Features/Home/Models/HomeData.swift] —— HomeUser / HomePet 类型定义（profile sink 派生源）
- [Source: docs/lessons/2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md] —— **lesson 1**: abstract method base class 注入点必须 concrete subclass
- [Source: docs/lessons/2026-04-30-published-derived-state-needs-publisher-subscription.md] —— **lesson 2**: 派生 state 必须订阅 publisher（避免 reset 后 stale）
- [Source: docs/lessons/2026-04-30-room-host-name-must-not-derive-from-local-current-pet.md] —— **lesson 3**: 不要从 currentPet 派生其他用户的信息（local pet ≠ remote owner）
- [Source: docs/lessons/2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md] —— **lesson 4**: RealViewModel.init 必须 seed scaffold defaults
- [Source: docs/lessons/2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md] —— **lesson 5**: `.onAppear` 同步 bind appState（避免 launch-time race）
- [Source: docs/lessons/2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md] —— **lesson 6（关键）**: RealViewModel.override placeholder method 必须实装本 story 范围内的本地 mutation（log-only 是 [P1]）
- [Source: docs/lessons/2026-04-30-swiftui-onchange-equatable-and-stale-task-cancel.md] —— **lesson 7**: SwiftUI .onChange iOS 17+ 双参签名
- [Source: docs/lessons/2026-04-30-swiftui-state-survives-id-and-shadow-over-children.md] —— **lesson 8a**: shadow 挂 RoundedRectangle.fill 那层
- [Source: docs/lessons/2026-04-30-swiftui-explicit-id-nil-shared-identity.md] —— **lesson 8b**: .id() 不挂 nil
- [Source: docs/lessons/2026-04-30-swiftui-floating-emoji-needs-state-driven-position.md] —— **lesson 8c**: @State 驱动浮动动画
- [Source: docs/lessons/2026-04-30-spec-boundary-grey-area-fallback-must-honor-epic-ac-when-review-flags-it.md] —— **lesson 9**: spec 边界灰色区域 epic AC 与 ui_design 实物冲突时遵循 epic AC

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- `bash iphone/scripts/build.sh --test` 全绿（PetAppTests.xctest 317 unit case，0 failures，4.4s）
- xcodegen regen 通过，新文件全部由 project.yml 通配规则自动 inclusion
- AC9 grep 校验全过：`class ProfileViewModel` 1 处 / `fatalError` 7 处（≥5）/ 无 `final class ProfileViewModel` / Mock & Real 各 5 个 `override func` / RealProfileViewModel mutation pattern 19 处（≥5，守护 lesson 6）/ ProfileView 含 ProfileScaffoldView 调用 + 不再含 `Text("Profile Tab Placeholder")`

### Completion Notes List

- 9 条预防性 lesson 全部命中：
  - lesson 1：RootView wire 用 `RealProfileViewModel()` 而非裸基类
  - lesson 2：profile 派生走 `Publishers.CombineLatest` sink 而非一次性 hydrate（守护 case#9 验证 reset 后无 stale）
  - lesson 3 反向应用：profile.name / petName 派生自 appState.currentUser / currentPet 是合法的（"我的资料"语义）
  - lesson 4：双 init 路径都 seed ScaffoldDefaults 全 5 字段（守护 case#8）
  - lesson 5：bind 走 RootView `.onAppear` 同步路径（守护 case#10）
  - lesson 6（关键）：5 个 Real override 全部本地 mutate state（守护 case#7 显式断言；grep ≥5 mutation pattern 验证）
  - lesson 7：本 story 无 .onChange 路径，不踩
  - lesson 8（abc）：所有 Card / Stat / IconButton / DataLossRow shadow 全部挂 `RoundedRectangle.fill(...).shadow(...)` 那一层，不挂最外层 chain
  - lesson 9：epic AC 钦定 5 区块 + 次级按钮（headerCard bell/settings + Modal "稍后再说"按钮）全落地，未因 ui_design 视觉简洁而砍
- 1.2s 自动弹 Modal timer 决策：本 story 不实装（spec Dev Notes 钦定边界灰区路径），用户必须手动点未绑定卡才弹 Modal
- BindWechatModal 走 SwiftUI `.sheet(isPresented:)` + `.presentationDetents([.fraction(0.85)])` + `.presentationCornerRadius(28)`（iOS 16.4+ guard），不自绘 ZStack overlay
- 微信品牌色 / 警告品牌色 / 风险红等 17+ 硬编码 hex 不进 theme tokens（与 spec 关键决策 5 一致；后续暗色模式微信卡视觉需要时再单独决策）
- ProfileView.swift 不删，保 git history（Story 37.13 决定）
- 11 个单元测试全绿（≥3 epic AC + 8 守护 case 含 lesson 1/2/4/5/6 命中）
- 1 个 UITest case `testProfileScaffoldShowsAllAnchors` 加在 HomeUITests.swift（含 Modal 触发链路：tap 未绑定卡 → modal 出现 → 主按钮可定位）

### File List

新增（9 个）：
- `iphone/PetApp/Features/Profile/ViewModels/ProfileViewModel.swift`
- `iphone/PetApp/Features/Profile/ViewModels/MockProfileViewModel.swift`
- `iphone/PetApp/Features/Profile/ViewModels/RealProfileViewModel.swift`
- `iphone/PetApp/Features/Profile/ViewModels/ProfileScaffoldDefaults.swift`
- `iphone/PetApp/Features/Profile/Models/ProfileSummary.swift`
- `iphone/PetApp/Features/Profile/Models/RecentCollection.swift`
- `iphone/PetApp/Features/Profile/Models/ProfileMenuItem.swift`
- `iphone/PetApp/Features/Profile/Views/ProfileScaffoldView.swift`
- `iphone/PetAppTests/Features/Profile/ProfileViewScaffoldTests.swift`

修改（5 个）：
- `iphone/PetApp/Features/Profile/Views/ProfileView.swift`（占位 Text 替换为 ProfileScaffoldView + @EnvironmentObject 注入 + Preview MockProfileViewModel）
- `iphone/PetApp/App/RootView.swift`（@StateObject profileViewModel + LaunchedContentView 透传 + .onAppear bind + .environmentObject）
- `iphone/PetApp/App/MainTabView.swift`（Preview 注入 MockProfileViewModel）
- `iphone/PetAppUITests/HomeUITests.swift`（追加 testProfileScaffoldShowsAllAnchors）
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen regen 结果）

### Change Log

- 2026-04-30 Story 37.11 落地：新增 ProfileViewModel 三件套（基类 / Mock / Real）+ ProfileSummary / RecentCollection / ProfileMenuItem 数据模型 + ProfileScaffoldDefaults 共享 enum + ProfileScaffoldView 5 区块视觉壳（含 BindWechatModal sheet + 双态微信卡）+ ProfileView caller 替换 + RootView wire + 11 unit case + 1 UITest case；9 条预防性 lesson 全部命中；测试 317 unit 全绿。
