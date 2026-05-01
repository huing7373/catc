# Story 37.10: FriendsView Scaffold + FriendsViewModel class 层次 + Mock/Real 两子类

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want Friends Tab 显示 ui_design 高保真好友页（顶部在线人数 Card + 我的房间提示条 + 在线/全部 Tab + FriendRow 列表 + 三态操作按钮）+ 接缝设计支持 Story 37.12（JoinRoomModal + 真实 JoinRoomUseCase 跨屏链路）后续注入,
so that 既有视觉壳又有可持续接缝（FriendsScaffoldView 内部代码 zero edit 让 Story 37.12 / 12.7 链路打开），同时把 Story 37.3 落地的 `FriendsView` 占位 stub 替换为 ui_design `friends.jsx` 像素级匹配的高保真 Scaffold。

## 故事定位（Epic 37 第四层第 4 条 story；Scaffold 主体 6 屏并行链路第四条，与 37.7 / 37.8 / 37.9 同模式）

这是 Epic 37「iPhone 架构层重构 + UI Scaffold（壳先行）」的**第四层 story**——上游 37.3（MainTabView 已挂 FriendsView 占位 stub）/ 37.4（AppState.currentRoomId 字段就绪）/ 37.5（Theme）/ 37.6（primitives 含 Avatar / Icons.plus / Icons.paw / Icons.enter）全部 done；37.7（HomeView）/ 37.8（RoomView）/ 37.9（WardrobeView）已用「class 层次 + Mock/Real 两子类 + ScaffoldDefaults seed + sink 派生 + 同步 onAppear bind + Real override 必 mutate state」模式落地，**本 story 1:1 复刻该模式于 FriendsView**。本 story 是 **UI Scaffold 主体** 类——属于 Epic 37 §AC 红线的「数据完全 mock + 禁 import APIClient/Repository/UseCase + 视觉像素级匹配 ui_design + 主题用 `@Environment(\.theme)` 取 token + 测试不引 SnapshotTesting/ViewInspector + 通过 `bash iphone/scripts/build.sh --test`」适用范围。

**本 story 落地后立即解锁**：
- Story 37.12（JoinRoomModal + 跨屏 join 链路）—— 本 story FriendRow inRoom 状态 "加入"按钮的 `onJoinFriendTap` 回调本期是占位（Mock：appendInvocation；Real：mutate currentRoomId 让 inRoom 路径立即视觉切换以满足 lesson 6），Story 37.12 把 Real 的占位行为替换为「解析 friend.currentRoomId → 直接调 JoinRoomUseCase」（**不**走 modal —— 仅 HomeView TeamIdleCard 走 modal）；FriendsScaffoldView 视图内部 zero edit
- Story 12.7（CreateRoom / JoinRoom UseCase）—— Real 子类 onInviteFriendTap 当 currentRoomId nil 时本期占位"创建队伍 mock"（mutate appState.currentRoomId 为占位串）；Story 12.7 改为调 CreateRoomUseCase + 真 server 写入
- Story 37.13（accessibility identifier 总表）—— FriendsScaffoldView 全部 a11y identifier 来源；本 story 在 FriendsScaffoldView 内 inline 字符串（`friendsView` / `friendsTab_online` / `friendsTab_all` / `friendsAddButton` / `friendsMyRoomCard` / `friendRow_<userId>` / `friendActionButton_<userId>`），Story 37.13 收口归并到 `AccessibilityID.Friends`

**本 story 的"实装"动作**（一句话概括）：在 `iphone/PetApp/Features/Friends/Views/FriendsScaffoldView.swift`（新建文件，**不**改 `FriendsView.swift` —— 见 Dev Notes "FriendsView 占位 stub 不删保 git history"）落地 struct `FriendsScaffoldView` + `@ObservedObject var state: FriendsViewModel`（基类直接，**非泛型 state**）；新建基类 `class FriendsViewModel: ObservableObject`（class 而非 final 让子类可继承）+ 4 个 `@Published` 字段（`friends: [Friend] / selectedTab: FriendsTab / currentRoomId: String? / lastToastMessage: String?`）+ 2 个 abstract method（`onInviteFriendTap(friend:)` / `onJoinFriendTap(friend:)`）+ 1 个 concrete view-action method（`selectTab(_:)`）+ 4 个 derived computed property（`onlineFriends` / `allFriends` / `displayedFriends` / `onlineCount`）；新建 `MockFriendsViewModel: FriendsViewModel` 子类（硬编码 mock 8 friends + override 2 个 abstract method 改本地 currentRoomId / lastToastMessage + invocations 数组）+ `RealFriendsViewModel: FriendsViewModel` 子类骨架（构造注入 AppState + parameterless init + bind(appState:) + sink 订阅 appState.$currentRoomId 派生 currentRoomId；override 2 个 abstract method 实装本地占位 mutate（与 Mock 同语义，按 Story 37.9 round 1 P1 lesson 钦定 `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`））；新建 `Friend` value type（id / name / online / status / statusText / currentRoomId / color）+ `FriendStatus` enum（offline / online / inRoom）+ `FriendsTab` enum（online / all）+ `FriendsScaffoldDefaults` 共享 enum（按 Story 37.8 / 37.9 P2 lesson 钦定路径，Mock / Real 双 init 都用它 seed）。`FriendsView` 内 `body` 的占位 Text 替换为 `FriendsScaffoldView(state: friendsViewModel)` 真实 Scaffold（caller 漏改靠编译器报错驱动；与 Story 37.3 / 37.7 / 37.8 / 37.9 同精神）；RootView 加 `@StateObject friendsViewModel: FriendsViewModel = RealFriendsViewModel()` + `.environmentObject(friendsViewModel)`；`.onAppear` 内同步 bind appState（防 launch-time race，按 Story 37.8 round 2 P2 lesson 钦定路径）。

**关键路径："新建" + caller 替换（与 Story 37.9 同精神：本 story 是新建 + 替换占位）**：

- `FriendsView.swift` **不删除**（保 Story 37.3 git history 可读 + 让人对比演进足迹；与 Story 37.9 WardrobeView 不删同精神）；仅在 `FriendsView.swift` 内 `body` 的 `Text("Friends Tab Placeholder")` 替换为 `FriendsScaffoldView(state: friendsViewModel)` —— `FriendsView` 类型本身保留作为 MainTabView 直接 instantiate 的入口 view（Story 37.13 a11y 总表归并时再决定是否一并清理）
- `friendsViewModel: FriendsViewModel` 注入路径走与 HomeView / RoomView / WardrobeView 相同模式：RootView 内 `@StateObject private var friendsViewModel: FriendsViewModel = RealFriendsViewModel()`（与 `homeViewModel` / `roomViewModel` / `wardrobeViewModel` 同模式；用 RealFriendsViewModel 而非裸 FriendsViewModel 防生产 fatalError 路径，按 Story 37.7 round 1 P1 lesson 钦定）+ `.environmentObject(friendsViewModel)`；`FriendsView` 内 `@EnvironmentObject var friendsViewModel: FriendsViewModel` 取出后传给 `FriendsScaffoldView(state:)` 子视图
- `RootView` 同步 `.onAppear { ... }` 内追加 `if let realFriendsVM = friendsViewModel as? RealFriendsViewModel { realFriendsVM.bind(appState: appState) }`（防 launch-time race；Story 37.8 round 2 P2 lesson 钦定 `.onAppear` 而非 `.task`）
- `LaunchedContentView` 透传 `friendsViewModel: FriendsViewModel` 字段（与已有 `homeViewModel` / `roomViewModel` / `wardrobeViewModel` 同模式），`.environmentObject(friendsViewModel)` 注入 ready 子树

**不涉及**（红线）：
- **不**实装 `JoinRoomUseCase` / `CreateRoomUseCase`（Story 12.7 / 37.12 落地；本 story 占位 mutate currentRoomId 视觉反馈 + invocations 记录）
- **不**接 server 真实好友列表接口（本 epic 全 mock；后续 epic 真实好友 API 落地时改 RealFriendsViewModel sink）
- **不**改 RootView `@StateObject` wire 切到基类 FriendsViewModel（与 RealHomeViewModel / RealRoomViewModel / RealWardrobeViewModel 一致都用 Real 子类避免 fatalError）
- **不**改 AppState / HomeData / HomePet / HomeUser / HomeEquip 类型（Story 37.4 已锁定）
- **不**实装 JoinRoomModal sheet（Story 37.12 落地；本 story FriendRow inRoom "加入"按钮**不**触发 modal —— 直接占位调 JoinRoomUseCase，按 epic AC line 4859「FriendsScreen "加入"按钮直接调 JoinRoomUseCase 不弹 Modal」钦定）
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3）
- **不**改 server/ 任何文件
- **不**收口 `AccessibilityID.Friends` 常量（本 story inline 字符串；Story 37.13 一次性归并所有 7 屏 a11y identifier）
- **不**删除 `FriendsView.swift`（保 git history；下游 Story 37.13 决定）
- **不**预先生成 `Friend` 之外的额外 helper / mapping 类型
- **不**实装"添加好友"按钮真实流程（顶部右侧 plus.circle.fill 按钮仅 print log + invocations 记录；后续 epic 真做）
- **不**实装 toast 真实动效（lastToastMessage @Published 写入即可；FriendsScaffoldView 暂用最简显示策略，详见 AC4）

## Acceptance Criteria

> **AC 编号体系**：AC1 是 FriendsViewModel class 层次基类（class + 4 字段 + 2 abstract method + 1 concrete view-action method + 4 derived computed property）；AC2 是 MockFriendsViewModel / RealFriendsViewModel 两子类（**Real override 必须本地 mutate state**，按 Story 37.9 round 1 P1 lesson 钦定）；AC3 是 Friend / FriendStatus / FriendsTab 值类型 + FriendsScaffoldDefaults 共享 enum；AC4 是 FriendsScaffoldView struct + 5 区块视觉（顶部 Card / 我的房间提示条 / Tab segmented control / FriendRow 列表 / FriendRow 三态按钮）；AC5 是 FriendsView caller 替换 + RootView wire + LaunchedContentView 透传；AC6 是 #Preview 双主题 × 多场景；AC7 是单元测试 ≥7 case（≥5 epic AC line 4816 钦定 + 守护 case 防 lesson 反例）；AC8 是 UITest a11y 定位关键锚；AC9 是 build verify；AC10 是 Deliverable 清单。

### AC1 — 新建 FriendsViewModel 基类（class 层次 + 4 字段 + 2 abstract method + 1 concrete view-action method）

**新建文件**：`iphone/PetApp/Features/Friends/ViewModels/FriendsViewModel.swift`

**类签名**（class 而非 final，让 Mock/Real 子类可继承；与 HomeViewModel Story 37.7 / RoomViewModel Story 37.8 / WardrobeViewModel Story 37.9 同精神）：

```swift
// FriendsViewModel.swift
// Story 37.10 AC1: FriendsScaffoldView 基类 ViewModel（class 层次 + 4 字段 + 2 abstract method + 1 concrete method）.
//
// 设计：与 HomeViewModel / RoomViewModel / WardrobeViewModel 同精神（class 而非 final + abstract method 用 fatalError 强制 override）.
// 字段范围：4 字段（friends / selectedTab / currentRoomId / lastToastMessage）.
// Story 37.12 RealFriendsViewModel 子类扩 onJoinFriendTap 调 JoinRoomUseCase / 走 setCurrentRoomId（不在本 story 范围）.

import Foundation
import Combine

@MainActor
public class FriendsViewModel: ObservableObject {
    /// 全部好友列表（mock 8 friend 三态混合；Mock 走 ScaffoldDefaults seed；Real 暂用同一 seed，
    /// 后续 epic 真接 server `/friends` 接口时改 sink 派生）.
    /// **关键约束**：friends 数据归本 ViewModel cache（**不进 AppState** —— 详见 epic AC line 4814 + ADR-0010 §3.2 表格
    /// "好友列表数据 → ViewModel 持有"; AppState 的 currentXxx 系列字段都是"本地用户的某条信息"语义，
    /// friends 是"别人的列表"，语义上不该进 AppState）.
    @Published public var friends: [Friend] = []

    /// 当前选中 Tab（在线 / 全部）；用户点 segmented control 切换.
    /// 这是 view-specific transient state（按 ADR-0010 §3.2 表格"当前选中" → ViewModel @Published）；
    /// 单元测试需要断言切换后 displayedFriends 派生改变（case#1）→ 不能放 SwiftUI @State.
    @Published public var selectedTab: FriendsTab = .online

    /// 当前房间号（"我的房间提示条"渲染依据；nil = 不渲染该 Card）.
    /// 派生源：appState.currentRoomId（RealFriendsViewModel 通过 sink 订阅派生；MockFriendsViewModel 用本地直写）.
    /// **关键约束**：currentRoomId 是 Wardrobe 域 catName 同精神的合法派生 —— "我的房间号"语义就是 appState.currentRoomId
    /// （与 Story 37.9 catName 派生自 currentPet 同合法理由：Friends 域的"我的房间"无歧义就是本地用户自己的房间；
    /// 与 Story 37.8 RoomViewModel.hostCatName 反例不冲突 —— 那是"看别人房间"语义）.
    @Published public var currentRoomId: String?

    /// 最近一次 toast 消息（占位 toast 系统；视觉走 FriendsScaffoldView 简单 overlay；详见 AC4 toast 渲染策略）.
    /// 用户可通过 selectTab / 触发其他动作隐式清空（写新值即覆盖；nil 表示无 toast）；
    /// 本 story 不实装"3 秒自动消失"等 timer 行为（保留给后续 epic）.
    @Published public var lastToastMessage: String?

    public init() {}

    // MARK: - abstract method（基类 fatalError 占位，子类必 override）

    /// "邀请"按钮回调（在线但未在房间的好友点按钮）.
    /// MockFriendsViewModel: 改本地 currentRoomId（若 nil → 设占位 "1234567"）+ 写 lastToastMessage + 记录 invocation.
    /// RealFriendsViewModel（本 story 占位）: **本地 mutate** —— currentRoomId nil 时设占位串 + 写 lastToastMessage,
    ///   非 nil 时仅写 lastToastMessage（"已邀请 {friend.name} 到房间 {currentRoomId}"）.
    ///   按 Story 37.9 round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`
    ///   钦定路径：Real 子类 override 必须实装本地 mutate state 让 production app 立即视觉反馈；不能只 log.
    /// Story 12.7+: RealFriendsViewModel 改调 CreateRoomUseCase + WS invitation 流程.
    public func onInviteFriendTap(friend: Friend) {
        fatalError("FriendsViewModel.onInviteFriendTap must be overridden by subclass")
    }

    /// "加入"按钮回调（friend.status == .inRoom 的好友点按钮）.
    /// MockFriendsViewModel: 改本地 currentRoomId = friend.currentRoomId + 写 lastToastMessage + 记录 invocation.
    /// RealFriendsViewModel（本 story 占位）: **本地 mutate** —— 若 friend.currentRoomId 非空,
    ///   通过 appState?.setCurrentRoomId(friend.currentRoomId) 写入 + 写 lastToastMessage（"加入 {friend.name} 的房间"）.
    ///   按 Story 37.9 round 1 P1 lesson 钦定路径.
    /// Story 37.12: RealFriendsViewModel 改调 JoinRoomUseCase + server 真实加入流程
    ///   （epic AC line 4859 钦定 FriendsScreen 直接调 JoinRoomUseCase，**不**弹 modal）.
    public func onJoinFriendTap(friend: Friend) {
        fatalError("FriendsViewModel.onJoinFriendTap must be overridden by subclass")
    }

    // MARK: - concrete view-action method（基类直接实装，子类不 override）

    /// 切换 Tab（用户点"在线"/"全部"调）.
    /// **不是** abstract —— 切换 Tab 是纯 view-state 行为，没有"Mock vs Real"分化需求.
    /// 副作用：仅写 selectedTab，**不**清 lastToastMessage（toast 与 tab 切换正交语义）.
    public func selectTab(_ tab: FriendsTab) {
        self.selectedTab = tab
    }

    // MARK: - derived helper（view 层方便用，子类不 override）

    /// 在线好友数（顶部 Card 显示用：「{onlineCount} 位在线 · 共 {friends.count} 位」）.
    public var onlineCount: Int {
        friends.filter { $0.online }.count
    }

    /// 在线好友列表（selectedTab == .online 时 displayedFriends 用）.
    public var onlineFriends: [Friend] {
        friends.filter { $0.online }
    }

    /// 全部好友列表（selectedTab == .all 时 displayedFriends 用，等价于 friends 全集）.
    public var allFriends: [Friend] {
        friends
    }

    /// 当前 Tab 显示的好友列表（list 渲染数据源；ui_design friends.jsx:5 filter 等价）.
    public var displayedFriends: [Friend] {
        switch selectedTab {
        case .online: return onlineFriends
        case .all:    return allFriends
        }
    }
}
```

> **关键决策 1**：abstract method 用 `fatalError` 而非 default empty body —— 与 HomeViewModel / RoomViewModel / WardrobeViewModel 同精神（让漏 override 立刻 crash + 测试覆盖逻辑路径）。

> **关键决策 2**：`selectTab` 是 **concrete** 在基类（不是 abstract）—— 切换 Tab 是纯 view state 行为，没有 Mock vs Real 分化需求；abstract 只用于"未来真实业务路径有 Mock vs Real 行为分化"的方法（onInviteFriendTap / onJoinFriendTap 是分化点：Mock 改本地状态 / Real 调 UseCase）。

> **关键决策 3**：`onlineCount` / `onlineFriends` / `allFriends` / `displayedFriends` 是 **derived computed property** 而非 @Published 字段 —— 它们是 friends + selectedTab 的纯函数派生，每次 SwiftUI body 求值时重新算。@Published 字段会让"派生 state 跟手动 mutate"漂移（与 ADR-0010 §3.5 派生 state 单源真理同精神）。

> **关键决策 4**：`currentRoomId` 是 `@Published` 字段（不是 derived computed property）—— 因为 RealFriendsViewModel 通过 sink 订阅 `appState.$currentRoomId` 派生写入，需要让 SwiftUI 监听变化；放 `@Published` 让 view 层 `state.currentRoomId == nil ? hideMyRoomCard : showMyRoomCard` 自动响应。Mock 子类直接写 @Published 触发渲染。

> **基类无参 init 兼容路径**：与 HomeViewModel / RoomViewModel / WardrobeViewModel 同精神 —— RootView 走 RealFriendsViewModel 子类，**不**走基类无参 init；基类 onInviteFriendTap / onJoinFriendTap 在生产 wire 路径下不会被调；Preview / UITest 走 MockFriendsViewModel。

**对应 Tasks**: Task 1.1, 1.2

### AC2 — 新建 MockFriendsViewModel / RealFriendsViewModel 两子类（独立文件）

**新建文件**: `iphone/PetApp/Features/Friends/ViewModels/MockFriendsViewModel.swift`

```swift
// MockFriendsViewModel.swift
// Story 37.10 AC2: FriendsViewModel mock 子类，用于 #Preview / UITest skip-guest-login / Scaffold 单元测试.
//
// 设计：
//   - 硬编码 mock 数据（friends 8 件 / selectedTab / currentRoomId 全量；走 FriendsScaffoldDefaults seed）
//   - override 2 个 abstract method（onInviteFriendTap / onJoinFriendTap）改本地状态 + 记录 invocations 数组
//   - **不**依赖 AppState（Mock 路径走纯 ViewModel-only 数据）
//
// 与 MockHomeViewModel Story 37.7 / MockRoomViewModel Story 37.8 / MockWardrobeViewModel Story 37.9 同模式（invocations 数组而非 closure spy）.

import Foundation
import Combine
import os.log

@MainActor
public final class MockFriendsViewModel: FriendsViewModel {
    /// 单元测试用：记录所有方法调用
    public enum Invocation: Equatable {
        case inviteTap(friendId: String)
        case joinTap(friendId: String)
    }

    @Published public var invocations: [Invocation] = []

    /// 默认构造 — 走 FriendsScaffoldDefaults seed 全量字段.
    public override init() {
        super.init()
        self.friends = FriendsScaffoldDefaults.friends
        self.selectedTab = FriendsScaffoldDefaults.selectedTab
        self.currentRoomId = FriendsScaffoldDefaults.currentRoomId   // nil
        self.lastToastMessage = nil
    }

    /// 测试 / Preview 灵活构造 — 可注入任意 friends / selectedTab / currentRoomId.
    public init(
        friends: [Friend] = FriendsScaffoldDefaults.friends,
        selectedTab: FriendsTab = FriendsScaffoldDefaults.selectedTab,
        currentRoomId: String? = FriendsScaffoldDefaults.currentRoomId
    ) {
        super.init()
        self.friends = friends
        self.selectedTab = selectedTab
        self.currentRoomId = currentRoomId
        self.lastToastMessage = nil
    }

    // MARK: - override abstract methods

    public override func onInviteFriendTap(friend: Friend) {
        os_log(.debug, "MockFriendsViewModel.onInviteFriendTap %{public}@", friend.id)
        invocations.append(.inviteTap(friendId: friend.id))
        // Mock 路径行为（与 epic AC line 4812 钦定一致）：
        //   - 若 currentRoomId nil → 触发"创建队伍 mock"（设占位 currentRoomId）+ toast "已邀请..."
        //   - 若 currentRoomId 非 nil → 仅 toast "已邀请..."（不再创建）
        if currentRoomId == nil {
            currentRoomId = "1234567"   // 占位"创建队伍"mock；与 RoomScaffoldDefaults 占位风格一致
            lastToastMessage = "已创建队伍并邀请 \(friend.name)"
        } else {
            lastToastMessage = "已邀请 \(friend.name) 加入房间 \(currentRoomId ?? "?")"
        }
    }

    public override func onJoinFriendTap(friend: Friend) {
        os_log(.debug, "MockFriendsViewModel.onJoinFriendTap %{public}@", friend.id)
        invocations.append(.joinTap(friendId: friend.id))
        // Mock 路径行为：解析 friend.currentRoomId 作为目标房间号 + 直接 mutate currentRoomId（不弹 modal —— epic AC line 4859 钦定）.
        guard let targetRoomId = friend.currentRoomId, !targetRoomId.isEmpty else {
            lastToastMessage = "好友不在房间中"
            return
        }
        currentRoomId = targetRoomId
        lastToastMessage = "加入 \(friend.name) 的房间 \(targetRoomId)"
    }
}
```

**新建文件**: `iphone/PetApp/Features/Friends/ViewModels/RealFriendsViewModel.swift`

```swift
// RealFriendsViewModel.swift
// Story 37.10 AC2: FriendsViewModel 生产实装子类（构造注入 AppState；override 2 个 abstract method 占位 mutate）.
//
// 范围（本 story 占位；Story 12.7 / 37.12 等填充真实 UseCase 调用）：
//   - 构造注入 AppState（按 ADR-0010 §3.1 ViewModel 注入规则）+ parameterless init() 走 bind(appState:)
//   - override onInviteFriendTap / onJoinFriendTap：本地 mutate currentRoomId / lastToastMessage（占位）
//     按 Story 37.9 round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`：
//     Real 子类 override 必须实装"本 story 范围内能让 UI 视觉工作的最小 placeholder 行为"，禁止只 log.
//
// **不**调用任何 UseCase / Repository / APIClient（Epic 37 红线：UI Scaffold 数据完全 mock）.
// **不**订阅真实好友列表 server 接口（后续 epic 落地；本 story RealFriendsViewModel friends 走 ScaffoldDefaults seed）.
//
// Story 37.7 / 37.8 / 37.9 沉淀 lesson 预防性应用（**不重蹈覆辙**）：
//   - lesson `2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md`：
//     RootView `@StateObject friendsViewModel` 用 `RealFriendsViewModel()` 而非基类 `FriendsViewModel()` —
//     基类 onInviteFriendTap / onJoinFriendTap 是 fatalError 占位，用户点按钮即 crash.
//   - lesson `2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md`：
//     两条 init 路径都走 `FriendsScaffoldDefaults` seed —— 让 launch 后 / hydrate 前 / reset 后任何
//     Real path 都立刻有 mock friends 占位（不让 FriendsScaffoldView 渲染空好友列表）.
//   - lesson `2026-04-30-published-derived-state-needs-publisher-subscription.md`：
//     派生 state 用 sink 路径而非一次性 hydrate —— currentRoomId 订阅 appState.$currentRoomId；
//     reset 路径（appState.reset() 把 currentRoomId 置 nil）也能即时反映到字段（不残留旧值）.
//   - lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`（**关键** —— Story 37.9 第一次复犯）：
//     onInviteFriendTap / onJoinFriendTap override **必须本地 mutate state**（与 Mock 同语义），
//     不能只 log（否则 production 路径下用户点按钮 no-op）.

import Foundation
import Combine
import os.log

@MainActor
public final class RealFriendsViewModel: FriendsViewModel {
    /// 构造注入 AppState（与 RealHomeViewModel / RealRoomViewModel / RealWardrobeViewModel 同模式）.
    private var appState: AppState?

    /// 派生 state sink 句柄（防多次 bind 重订阅 + 持有 cancellable 让 sink 存活）.
    private var currentRoomIdSubscription: AnyCancellable?

    /// parameterless init —— RootView `@StateObject` 老模式可用; AppState 通过 bind 异步注入.
    /// 按 Story 37.8 / 37.9 round 1 P2 lesson 预防性应用：seed `friends` / `selectedTab` 全部走 FriendsScaffoldDefaults,
    /// 让 launch / hydrate 前 / reset 后任何走 Real path 都立刻有 mock 好友列表占位.
    /// 注：必写 `override` —— 基类 FriendsViewModel 有显式 `public init() {}`（与 RoomViewModel / WardrobeViewModel 同模式）.
    public override init() {
        super.init()
        self.appState = nil
        self.friends = FriendsScaffoldDefaults.friends
        self.selectedTab = FriendsScaffoldDefaults.selectedTab
        self.currentRoomId = FriendsScaffoldDefaults.currentRoomId  // nil; bind 后 sink 派生
        self.lastToastMessage = nil
    }

    public init(appState: AppState) {
        super.init()
        self.appState = appState
        // round 1 P2 fix：先 seed scaffold defaults（让 sink 还没派发前 FriendsScaffoldView 有数据可渲染）.
        self.friends = FriendsScaffoldDefaults.friends
        self.selectedTab = FriendsScaffoldDefaults.selectedTab
        self.currentRoomId = FriendsScaffoldDefaults.currentRoomId
        self.lastToastMessage = nil
        // 构造路径已注入 AppState；立即订阅 currentRoomId 派生.
        subscribeCurrentRoomId(to: appState)
    }

    /// AppState 异步注入入口（与 RealHomeViewModel / RealRoomViewModel.bind / RealWardrobeViewModel.bind 同模式）.
    public func bind(appState: AppState) {
        let alreadySubscribed = currentRoomIdSubscription != nil
        self.appState = appState
        guard !alreadySubscribed else { return }
        subscribeCurrentRoomId(to: appState)
    }

    /// 订阅 appState.$currentRoomId —— hydrate / reset / 单独 mutate 都派生 currentRoomId.
    /// **关键**：currentRoomId 派生源是合法的 —— Friends 域语义就是"我的房间"（本地用户自己的房间）,
    /// appState.currentRoomId 是真理源（与 Story 37.9 catName 派生自 currentPet 同合法理由；
    /// 与 Story 37.8 RoomViewModel.hostCatName 反例不冲突 —— 那是"看别人房间"语境）.
    private func subscribeCurrentRoomId(to appState: AppState) {
        currentRoomIdSubscription = appState.$currentRoomId
            .sink { [weak self] roomId in
                guard let self else { return }
                self.currentRoomId = roomId
            }
    }

    // MARK: - override abstract methods（本 story 占位 mutate；Story 12.7 / 37.12 实装真实 UseCase）

    /// round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md` 预防性应用：
    /// override 必须**本地 mutate** state 让 UI 立刻反馈，不能只 log（否则 production 走 RealFriendsViewModel 时
    /// 邀请按钮 no-op，主交互失效；单测/Preview 走 Mock 路径覆盖不到本 bug）.
    ///
    /// 行为与 MockFriendsViewModel.onInviteFriendTap 同语义（currentRoomId nil → 设占位串 + toast；非 nil → 仅 toast）.
    /// 让 Mock 单测 / Preview 与 Real 生产观感一致.
    /// Story 12.7 落地 CreateRoomUseCase 后改为：
    ///   1) 调 CreateRoomUseCase（若 currentRoomId nil）/ WS invitation
    ///   2) 成功后通过 appState.setCurrentRoomId(...) 写入
    ///   3) 通过 sink 派生 currentRoomId 字段（不再本地直接写）
    public override func onInviteFriendTap(friend: Friend) {
        os_log(.debug, "RealFriendsViewModel.onInviteFriendTap (Story 12.7 will wire CreateRoomUseCase) %{public}@", friend.id)
        if currentRoomId == nil {
            // 占位创建队伍：通过 appState.setCurrentRoomId 走规范入口（让 sink 派生 currentRoomId，与 Story 12.7 落地后路径一致）.
            // **不**直接写 self.currentRoomId —— Real 路径必须走 appState 入口,
            //   让 RealRoomViewModel / RealWardrobeViewModel 等订阅了 currentRoomId 的兄弟 ViewModel 也同步.
            appState?.setCurrentRoomId("1234567")
            lastToastMessage = "已创建队伍并邀请 \(friend.name)"
        } else {
            lastToastMessage = "已邀请 \(friend.name) 加入房间 \(currentRoomId ?? "?")"
        }
    }

    /// round 1 P1 lesson 预防性应用：override 必须本地 mutate state.
    /// 行为与 MockFriendsViewModel.onJoinFriendTap 同语义（mutate currentRoomId 到 friend.currentRoomId）.
    /// Story 37.12 落地 JoinRoomUseCase 后改为：
    ///   1) 调 JoinRoomUseCase(roomId: friend.currentRoomId)（epic AC line 4859 钦定 FriendsScreen 直接调，不弹 modal）
    ///   2) 成功后 server 真实加入 + WS room.snapshot 推送
    ///   3) appState.setCurrentRoomId 由 server 端权威态写入 + sink 派生
    public override func onJoinFriendTap(friend: Friend) {
        os_log(.debug, "RealFriendsViewModel.onJoinFriendTap (Story 37.12 will wire JoinRoomUseCase) %{public}@", friend.id)
        guard let targetRoomId = friend.currentRoomId, !targetRoomId.isEmpty else {
            lastToastMessage = "好友不在房间中"
            return
        }
        // 走规范入口：appState.setCurrentRoomId（与 Story 37.8 onLeaveTap 同精神；让 sink 派生）.
        appState?.setCurrentRoomId(targetRoomId)
        lastToastMessage = "加入 \(friend.name) 的房间 \(targetRoomId)"
    }
}
```

> **关键决策 1**：MockFriendsViewModel / RealFriendsViewModel 都 `final` —— 子类不可再被继承（与 ADR-0010 §3.1 mock 模式钦定 + Story 37.7 / 37.8 / 37.9 同精神）；只有基类 `FriendsViewModel` 是 `class`（非 final）。

> **关键决策 2**：MockFriendsViewModel 用 invocations 数组而非 closure spy —— 与 MockHomeViewModel / MockRoomViewModel / MockWardrobeViewModel 同精神。

> **关键决策 3**：RealFriendsViewModel.onInviteFriendTap / onJoinFriendTap **必须本地 mutate state**（按 Story 37.9 round 1 P1 lesson 钦定）—— Real 路径不能只 log，否则 production app 用户点按钮 no-op；占位行为通过 `appState.setCurrentRoomId(...)` 入口（**不**直接写 self.currentRoomId），让 sink 派生 currentRoomId 字段（与 Story 27.1 落地后真实写入路径一致）+ 同步触发兄弟 ViewModel sink（如 RoomViewModel 也订阅 currentRoomId 时同步切到 inRoom）。这个模式让 Mock 与 Real 视觉等价 + 让 Story 37.12 落地真实 UseCase 时改动局部化。

> **关键决策 4**：RealFriendsViewModel 不为 `friends` 字段单独建 sink —— 本 story 范围内 friends 永远走 FriendsScaffoldDefaults seed（好友列表 server 接口在后续 epic 才落地）；当真实接口落地时再加 `subscribeFriends` sink，**不**预 over-design。详见 Dev Notes "RealFriendsViewModel friends 字段策略"。

**对应 Tasks**: Task 2.1, 2.2

### AC3 — 新建 Friend / FriendStatus / FriendsTab 值类型 + FriendsScaffoldDefaults 共享 enum

**新建文件**: `iphone/PetApp/Features/Friends/Models/Friend.swift`

```swift
// Friend.swift
// Story 37.10 AC3: FriendsScaffoldView 好友数据模型.
//
// 设计：value type + Equatable + Sendable + Identifiable，纯展示数据；mock 值在 FriendsScaffoldDefaults.
// 后续 epic 接 server `/friends` 接口后由 RealFriendsViewModel 内 mapping 写入（API DTO → Friend）.
//
// 字段名对齐 ui_design friends.jsx 内 friends array shape（id / name / online / status / statusText / currentRoomId / color）.

import Foundation
import SwiftUI

public struct Friend: Equatable, Identifiable, Sendable {
    public let id: String                   // userId（后续 epic 后对齐 server user.id）
    public let name: String                 // 好友昵称（如"夏夏"）
    public let online: Bool                 // 是否在线（决定 Avatar 小绿点 + invite/offline 按钮分支）
    public let status: FriendStatus         // 三态分类（offline / online / inRoom）
    public let statusText: String           // 状态文字（如"在房间 1234567 玩耍中" / "刚刚活跃" / "2 小时前在线"）
    public let currentRoomId: String?       // status == .inRoom 时为目标房间号；其它 nil
    public let color: Color?                // 显式覆写 Avatar 背景色（nil 走 Avatar hash 调色板）

    public init(
        id: String,
        name: String,
        online: Bool,
        status: FriendStatus,
        statusText: String,
        currentRoomId: String? = nil,
        color: Color? = nil
    ) {
        self.id = id
        self.name = name
        self.online = online
        self.status = status
        self.statusText = statusText
        self.currentRoomId = currentRoomId
        self.color = color
    }
}
```

> **关键决策**：`color: Color?` 字段 —— Avatar 视觉支持显式覆写 + ui_design friends.jsx 内 mock data 走 hash（无显式 color），本 story `FriendsScaffoldDefaults` 内全部用 `nil` 走 Avatar hash 调色板（与 ui_design 视觉一致）。`color` 字段保留是为了后续 epic 真实数据可能传 color；本期不强用。

**新建文件**: `iphone/PetApp/Features/Friends/Models/FriendStatus.swift`

```swift
// FriendStatus.swift
// Story 37.10 AC3: 三态 enum（对齐 ui_design friends.jsx FriendRow 三分支按钮）.
//
// rawValue 与 ui_design friends.jsx:89 / 100 / 110 三分支 status 字段对齐:
//   - inRoom: 在房间中（按钮"加入"实心 accent 色 + Icons.enter）
//   - online: 在线（按钮"邀请"描边 accent 色，无 icon）
//   - offline: 离线（无按钮，灰字"离线"）

import Foundation

public enum FriendStatus: String, CaseIterable, Identifiable, Sendable {
    case inRoom
    case online
    case offline

    public var id: String { rawValue }
}
```

**新建文件**: `iphone/PetApp/Features/Friends/Models/FriendsTab.swift`

```swift
// FriendsTab.swift
// Story 37.10 AC3: 顶部 segmented control 二选一 enum（对齐 ui_design friends.jsx:48 ['online','all']）.
//
// rawValue 严格对齐 ui_design 钦定，让 a11y identifier 拼接 `friendsTab_\(rawValue)` 与 ui_design 直接映射.

import Foundation

public enum FriendsTab: String, CaseIterable, Identifiable, Sendable {
    case online
    case all

    public var id: String { rawValue }

    /// Tab 显示名（ui_design friends.jsx:48 钦定）.
    public var label: String {
        switch self {
        case .online: return "在线"
        case .all:    return "全部"
        }
    }
}
```

**新建文件**: `iphone/PetApp/Features/Friends/ViewModels/FriendsScaffoldDefaults.swift`

```swift
// FriendsScaffoldDefaults.swift
// Story 37.10 AC3: Mock 与 Real FriendsViewModel 共享 scaffold 占位数据.
//
// 背景（Story 37.8 round 1 P2 lesson 预防性应用）：
//   抽 shared defaults 而非 hardcode 在两个 ViewModel —— 避免 Mock/Real 重复定义 mock 数据.
//
// 设计决议（与 RoomScaffoldDefaults / WardrobeScaffoldDefaults 同精神）：
//   - friends 三态各 2-3 个共 8 件（epic AC line 4814 钦定）
//   - selectedTab 默认 .online（ui_design friends.jsx:4 useState('online') 钦定）
//   - currentRoomId 默认 nil（启动后用户未进房间）

import Foundation

/// Mock 与 Real FriendsViewModel 启动占位数据（friends state UI scaffold defaults）.
public enum FriendsScaffoldDefaults {
    /// 默认选中 Tab（mock .online —— ui_design friends.jsx:4 useState('online') 钦定）.
    public static let selectedTab: FriendsTab = .online

    /// 默认 currentRoomId（启动占位 nil；RealFriendsViewModel sink 派生覆盖）.
    public static let currentRoomId: String? = nil

    /// 完整 mock friends（8 件，三态混合 inRoom 3 / online 3 / offline 2，epic AC line 4814 钦定 ≥2-3 each）.
    /// 字段值与 ui_design friends.jsx FriendRow 视觉示例匹配（name / status / statusText 风格一致）.
    public static let friends: [Friend] = [
        // inRoom（3）
        Friend(id: "u1", name: "夏夏", online: true, status: .inRoom, statusText: "在房间 1234567 玩耍中", currentRoomId: "1234567"),
        Friend(id: "u2", name: "茉茉", online: true, status: .inRoom, statusText: "在房间 8888888 喂猫", currentRoomId: "8888888"),
        Friend(id: "u3", name: "可乐", online: true, status: .inRoom, statusText: "和小伙伴在房间 7654321", currentRoomId: "7654321"),
        // online（3）
        Friend(id: "u4", name: "豆豆", online: true, status: .online, statusText: "刚刚活跃"),
        Friend(id: "u5", name: "馒头", online: true, status: .online, statusText: "在线 · 想散步"),
        Friend(id: "u6", name: "拿铁", online: true, status: .online, statusText: "在线 · 等队友"),
        // offline（2）
        Friend(id: "u7", name: "饭团", online: false, status: .offline, statusText: "2 小时前在线"),
        Friend(id: "u8", name: "椰奶", online: false, status: .offline, statusText: "昨天活跃"),
    ]
}
```

> **关键决策**：8 件 friends 严格对应 inRoom 3 / online 3 / offline 2（epic AC line 4814 「每态各 2-3 个」满足）。statusText 字符串与 ui_design 风格一致（包含 roomId 数字让"房间中"角标视觉合理）。

**对应 Tasks**: Task 3.1, 3.2, 3.3, 3.4

### AC4 — 新建 FriendsScaffoldView struct + 5 区块视觉

**新建文件**: `iphone/PetApp/Features/Friends/Views/FriendsScaffoldView.swift`

**关键签名**（与 HomeView Story 37.7 / RoomScaffoldView Story 37.8 / WardrobeScaffoldView Story 37.9 同模式：`@ObservedObject var state: FriendsViewModel` 基类直接，**非泛型 state**）：

```swift
public struct FriendsScaffoldView: View {
    @ObservedObject public var state: FriendsViewModel

    /// Story 37.5: 主题 token 取值入口；RootView 注入 `.environment(\.theme, currentTheme.theme)`.
    @Environment(\.theme) private var theme

    public init(state: FriendsViewModel) {
        self.state = state
    }

    public var body: some View {
        VStack(spacing: 0) {
            topCard               // 区块 1: 顶部 Card（"X 位在线 · 共 Y 位" + "好友" + plus 添加按钮）
            myRoomCard            // 区块 2: 我的房间提示条（仅 currentRoomId != nil 渲染）
            tabBar                // 区块 3: 在线/全部 segmented control
            friendsList           // 区块 4 + 5: 好友列表 + FriendRow（含三态按钮）
        }
        .background(theme.colors.pageBg.ignoresSafeArea())
        .accessibilityIdentifier("friendsView")
        .overlay(alignment: .bottom) { toastOverlay }   // 占位 toast
    }
    // ... 5 区块子视图实现略（Dev Notes "5 区块视觉契约"详述每块视觉 + a11y + 颜色 / spacing 规则）
}
```

**5 区块要点**（详细视觉规则见 Dev Notes "5 区块视觉契约"；这里给关键定位锚）：

- **topCard**（friends.jsx:9-23）：HStack 左 VStack 12pt 700 「{onlineCount} 位在线 · 共 {friends.count} 位」+ 22pt 800 「好友」；右 IconButton 圆形 40x40（surface 背景 + sm shadow + border 1pt + 圆 20，含 `Icons.symbol(for: "plus")` ink 色，accessibilityIdentifier `friendsAddButton`，点击仅 print log + invocations 记录到 base class lastToastMessage）；padding 68pt top（状态栏占位）/ 20pt horizontal / 8pt bottom
- **myRoomCard**（friends.jsx:25-44）：仅 `state.currentRoomId != nil` 时渲染；HStack 左 36x36 圆形 accent 背景含 `Icons.symbol(for: "paw")` 18pt white + 中 VStack 11pt 700 ink-soft 「你的房间」+ 14pt 800 ink 「代码 」 + 14pt 800 accent-deep monospaced letterSpacing 2pt currentRoomId（**不**实装"分享给好友"按钮 —— epic AC line 4807 钦定该按钮"占位 toast，本 epic 不真分享"，本 story 选择**不渲染该按钮**而非渲染占位按钮，理由见 Dev Notes "myRoomCard 分享按钮决策"）；padding 4pt top / 20pt horizontal / 8pt bottom；背景 linear-gradient(accent-soft → transparent) + border 1pt border 色；圆角 16；accessibilityIdentifier `friendsMyRoomCard`
- **tabBar**（friends.jsx:47-57）：HStack 6pt + 2 个 Button（按 FriendsTab.allCases 渲染；selected = ink 背景 + surface 文字 + 无 border，unselected = surface 背景 + ink-soft 文字 + border 1pt；padding 7pt vertical / 18pt horizontal；圆角 14；font 12pt 800；accessibilityIdentifier `friendsTab_\(tab.rawValue)`；点击调 `state.selectTab(tab)`）；padding 6pt vertical / 20pt horizontal
- **friendsList**（friends.jsx:60-73）：ScrollView 内 LazyVStack 8pt + ForEach `state.displayedFriends` 渲染 FriendRow；padding 8pt top / 20pt horizontal / 100pt bottom（让出浮动 TabBar 空间）；当 displayedFriends 空时渲染 fallback 文案 「暂无好友在线～」（13pt 600 ink-mute 居中 padding 40）
- **FriendRow**（friends.jsx:78-126）：HStack 12pt 含 Avatar(name: f.name, size: 48, online: f.online, color: f.color) + VStack 左对齐含 14pt 800 HStack(name + (status == .inRoom ? 房间中角标 9pt 800 accent-deep accent-soft 圆 6 padding 2/6 : nil)) + 11pt 600 (online ? ink-soft : ink-mute) statusText；右侧三态按钮：
  - **inRoom**（status == .inRoom）：「{Icons.enter} 加入」accent 实心 + white text + 圆 14 + padding 8/14 + font 12pt 800；调 `state.onJoinFriendTap(friend: f)`；accessibilityIdentifier `friendActionButton_\(f.id)`
  - **online**（status == .online）：「邀请」描边 1.5pt accent + transparent 背景 + accent-deep text + 圆 14 + padding 8/14 + font 12pt 800；调 `state.onInviteFriendTap(friend: f)`；accessibilityIdentifier `friendActionButton_\(f.id)`
  - **offline**（status == .offline）：「离线」纯文本 11pt 700 ink-mute + padding 0/8（**不**渲染 Button —— epic AC line 4813 钦定 disabled）；**不**带 accessibilityIdentifier（无可点击元素）
  - FriendRow 容器：surface 背景 + sm shadow + border 1pt + 圆 18 + padding 12；accessibilityIdentifier `friendRow_\(f.id)`
- **toastOverlay**（**本 story 新增；ui_design 无明确视觉**）：仅 `state.lastToastMessage != nil` 时渲染；底部 padding 120（让出浮动 TabBar）+ Card-like 黑底 0.85 alpha + white 文字 13pt 700 + 圆 12 + padding 8/16 + 居中；**不**自动消失（lastToastMessage 写入即覆盖；ViewModel 不实装 timer）；accessibilityIdentifier `friendsToast`

> **关键决策 1**：5 区块布局用 `VStack(spacing: 0)` 全塞主 body —— ui_design friends.jsx:8 `display: flex / flexDirection: column`，仅最底层 friendsList 滚动。

> **关键决策 2**：myRoomCard `分享给好友` 按钮 **不渲染**（与 epic AC line 4807 字面"含房间 id 字符串 + 分享给好友次要按钮（占位 toast）"相比的删减决策；详见 Dev Notes "myRoomCard 分享按钮决策"）—— 减少占位按钮等于减少未来废弃的 a11y identifier 与 invocations 噪声。如果 review 反馈要求保留占位按钮，可在 fix-review round 加回（一行 `PrimaryButton(variant: .secondary)` 即可）。**SM 选择保留这个边界灰区让 dev / reviewer 在实装时根据视觉密度做最终决定**：若 myRoomCard 缺按钮看起来太空 → 渲染占位按钮；若密度刚好 → 保持纯展示。

> **关键决策 3**：FriendRow 三态分支用 `switch f.status` 而非 `if-else` chain —— 让漏分支编译期报错（按 Swift exhaustive enum 语义；与 RoomScaffoldView host/member 分支同精神）。

> **关键决策 4**：FriendRow shadow 必须挂在 `RoundedRectangle.fill(...).shadow(...)` 那一层（按 Story 37.6 round 5 lesson `2026-04-30-swiftui-state-survives-id-and-shadow-over-children.md` 钦定路径），**不**挂最外层 chain（避免 children Avatar / Text 被 alpha 投影）。

> **关键决策 5**：toastOverlay 用 `.overlay(alignment: .bottom)` 而非 `.background` 或独立 ZStack —— 让 toast 视觉浮在 friendsList 滚动内容之上；底部 padding 120 让出浮动 TabBar 空间；不自动消失（为简化）—— 用户切 Tab / 点其他动作隐式覆盖。**toast 视觉精度由 review 把关**；本 story 仅保证"toast 出现 + 可定位 + Mock/Real 行为一致"。

**对应 Tasks**: Task 4.1, 4.2, 4.3, 4.4, 4.5, 4.6, 4.7

### AC5 — FriendsView caller 替换 + RootView wire + LaunchedContentView 透传

**改动文件 1**: `iphone/PetApp/Features/Friends/Views/FriendsView.swift`

**关键改动**（替换 body 占位 Text 为 FriendsScaffoldView 真实内容）：

```swift
// 旧（Story 37.3 落地）
public struct FriendsView: View {
    public init() {}

    public var body: some View {
        NavigationStack {
            Text("Friends Tab Placeholder")
                .accessibilityIdentifier("friendsView")
        }
    }
}

// 新（Story 37.10 落地）
public struct FriendsView: View {
    @EnvironmentObject var friendsViewModel: FriendsViewModel

    public init() {}

    public var body: some View {
        NavigationStack {
            FriendsScaffoldView(state: friendsViewModel)
        }
    }
}
```

> **关键决策**：保留 `NavigationStack` 包裹层 —— 让后续 epic 实装 NavigationLink push 好友详情页时无须再改 FriendsView 类型签名。

**改动文件 2**: `iphone/PetApp/App/RootView.swift`

**关键改动**：在 `@StateObject wardrobeViewModel` 同级新增 `@StateObject friendsViewModel` + 同级 `.environmentObject(friendsViewModel)` + `.onAppear` 内同步 bind appState：

```swift
// 旧（Story 37.9 落地）
@StateObject private var homeViewModel: HomeViewModel = RealHomeViewModel()
@StateObject private var roomViewModel: RoomViewModel = RealRoomViewModel()
@StateObject private var wardrobeViewModel: WardrobeViewModel = RealWardrobeViewModel()
@StateObject private var appState = AppState()
// ...
.onAppear {
    homeViewModel.bind(appState: appState)
    if let realRoomVM = roomViewModel as? RealRoomViewModel {
        realRoomVM.bind(appState: appState)
    }
    if let realWardrobeVM = wardrobeViewModel as? RealWardrobeViewModel {
        realWardrobeVM.bind(appState: appState)
    }
    ensureLaunchStateMachineWired()
}

// 新（Story 37.10 追加；homeViewModel / roomViewModel / wardrobeViewModel 不动）
@StateObject private var homeViewModel: HomeViewModel = RealHomeViewModel()
@StateObject private var roomViewModel: RoomViewModel = RealRoomViewModel()
@StateObject private var wardrobeViewModel: WardrobeViewModel = RealWardrobeViewModel()
@StateObject private var friendsViewModel: FriendsViewModel = RealFriendsViewModel()
@StateObject private var appState = AppState()
// ...
.onAppear {
    homeViewModel.bind(appState: appState)
    if let realRoomVM = roomViewModel as? RealRoomViewModel {
        realRoomVM.bind(appState: appState)
    }
    if let realWardrobeVM = wardrobeViewModel as? RealWardrobeViewModel {
        realWardrobeVM.bind(appState: appState)
    }
    if let realFriendsVM = friendsViewModel as? RealFriendsViewModel {
        realFriendsVM.bind(appState: appState)
    }
    ensureLaunchStateMachineWired()
}
```

LaunchedContentView 透传：

```swift
// LaunchedContentView 加 friendsViewModel: FriendsViewModel 字段（与 homeViewModel / roomViewModel / wardrobeViewModel 同模式）
// + body 内 .environmentObject(friendsViewModel) 注入 ready 子树
```

> **关键决策**：Story 37.7 round 1 P1 lesson 预防性应用 —— `@StateObject private var friendsViewModel: FriendsViewModel = RealFriendsViewModel()`（**不是**裸基类 `FriendsViewModel()` —— 基类 onInviteFriendTap / onJoinFriendTap 是 fatalError，用户点按钮即 crash）。

> **关键决策**：Story 37.8 round 2 P2 lesson 预防性应用 —— `bind(appState:)` 调用放在 `.onAppear` 而非 `.task`（防 launch-time race，让 ViewModel 在第一次 paint 之前就持有 AppState 引用）。

> **caller 漏改靠编译器报错驱动**：FriendsView 内 `Text("Friends Tab Placeholder")` 替换为 `FriendsScaffoldView(state: friendsViewModel)` —— 旧 body 替换前编译过；替换后若漏挂 `@EnvironmentObject` 或漏注入 `.environmentObject(friendsViewModel)` 会 runtime crash（SwiftUI 找不到 environmentObject）—— **不依赖 grep 兜底**。MainTabView 在 Story 37.3 落地时已有 `FriendsView().tag(AppTab.friends)`，调用站不变；Preview 块需要追加 `.environmentObject(MockFriendsViewModel() as FriendsViewModel)` 让 MainTabView Preview 不 crash。

**改动文件 3**: `iphone/PetApp/App/MainTabView.swift`

```swift
// 在 Preview 块（行 121 之后）追加：
.environmentObject(MockFriendsViewModel() as FriendsViewModel)
```

**对应 Tasks**: Task 5.1, 5.2, 5.3, 5.4, 5.5

### AC6 — #Preview 双主题（candy / dark）+ 多场景 mock

FriendsScaffoldView 文件底部 `#if DEBUG ... #endif` 块含 4 个 Preview（双主题 × 默认/有房间/空 friends 场景）：

```swift
#if DEBUG
#Preview("FriendsScaffoldView — full mock / candy") {
    FriendsScaffoldView(state: MockFriendsViewModel())
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("FriendsScaffoldView — full mock / dark") {
    FriendsScaffoldView(state: MockFriendsViewModel())
        .environment(\.theme, ThemeName.dark.theme)
}

#Preview("FriendsScaffoldView — has my room / candy") {
    FriendsScaffoldView(state: MockFriendsViewModel(currentRoomId: "1234567"))
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("FriendsScaffoldView — empty friends / candy") {
    FriendsScaffoldView(state: MockFriendsViewModel(friends: []))
        .environment(\.theme, ThemeName.candy.theme)
}
#endif
```

> **关键决策**：4 个 Preview 覆盖 默认（online tab + 满 friends + 无 myRoom）/ 有 myRoom（验证 myRoomCard 渲染分支）/ 空 friends（验证 fallback 文案）/ dark 主题（验证 Theme token 适配）。

**对应 Tasks**: Task 6.1

### AC7 — 单元测试覆盖（≥7 case，纯 XCTest + MockFriendsViewModel + AppState）

**新建文件**: `iphone/PetAppTests/Features/Friends/FriendsViewScaffoldTests.swift`

落地以下 ≥7 case（≥5 epic AC line 4816 + 守护 case 防 lesson 反例）：

```swift
// FriendsViewScaffoldTests.swift
// Story 37.10 AC7: FriendsScaffoldView + FriendsViewModel class 层次单元测试.
//
// 测试基础设施约束（与 Story 2.7 + ADR-0002 §3.1 衔接）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）.
//   - 不引 ViewInspector / SnapshotTesting.
//   - 走 ViewModel 行为 + invocations 数组断言；不走 SwiftUI body 内省.

import XCTest
@testable import PetApp

@MainActor
final class FriendsViewScaffoldTests: XCTestCase {

    // MARK: - case#1 happy: 切到全部 Tab → displayedFriends 含全部好友（含离线）

    func testSelectTabSwitchesDisplayedFriends() {
        let vm = MockFriendsViewModel()
        // 默认 .online → displayedFriends 仅 online == true
        XCTAssertEqual(vm.selectedTab, .online)
        XCTAssertTrue(vm.displayedFriends.allSatisfy { $0.online })
        XCTAssertFalse(vm.displayedFriends.contains(where: { $0.status == .offline }))

        vm.selectTab(.all)
        XCTAssertEqual(vm.selectedTab, .all)
        XCTAssertEqual(vm.displayedFriends.count, vm.friends.count)
        XCTAssertTrue(vm.displayedFriends.contains(where: { $0.status == .offline }))
    }

    // MARK: - case#2 happy: inRoom 好友点"加入" → onJoinFriendTap 触发 + currentRoomId 切到 friend.currentRoomId

    func testOnJoinFriendTapMutatesCurrentRoomId() {
        let vm = MockFriendsViewModel()
        let inRoomFriend = vm.friends.first(where: { $0.status == .inRoom })!
        let targetRoomId = inRoomFriend.currentRoomId!
        XCTAssertNil(vm.currentRoomId, "初始无房间")

        vm.onJoinFriendTap(friend: inRoomFriend)
        XCTAssertEqual(vm.currentRoomId, targetRoomId, "加入后 currentRoomId = friend.currentRoomId")
        XCTAssertEqual(vm.invocations, [.joinTap(friendId: inRoomFriend.id)])
        XCTAssertNotNil(vm.lastToastMessage)
        XCTAssertTrue(vm.lastToastMessage!.contains(inRoomFriend.name))
    }

    // MARK: - case#3 happy: online 好友点"邀请" + currentRoomId nil → mutate currentRoomId 占位 + toast

    func testOnInviteFriendTapWhenNoRoomCreatesPlaceholderRoom() {
        let vm = MockFriendsViewModel()
        let onlineFriend = vm.friends.first(where: { $0.status == .online })!
        XCTAssertNil(vm.currentRoomId)

        vm.onInviteFriendTap(friend: onlineFriend)
        XCTAssertNotNil(vm.currentRoomId, "currentRoomId nil 时邀请触发占位 mock 创建")
        XCTAssertEqual(vm.invocations, [.inviteTap(friendId: onlineFriend.id)])
        XCTAssertNotNil(vm.lastToastMessage)
        XCTAssertTrue(vm.lastToastMessage!.contains(onlineFriend.name))
    }

    // MARK: - case#4 happy: online 好友点"邀请" + currentRoomId 非 nil → 仅 toast，不重新创建

    func testOnInviteFriendTapWhenInRoomOnlyToasts() {
        let vm = MockFriendsViewModel(currentRoomId: "9999999")
        let onlineFriend = vm.friends.first(where: { $0.status == .online })!

        vm.onInviteFriendTap(friend: onlineFriend)
        XCTAssertEqual(vm.currentRoomId, "9999999", "currentRoomId 非 nil → 不重新创建")
        XCTAssertNotNil(vm.lastToastMessage)
    }

    // MARK: - case#5 happy: friends 空数组 → displayedFriends 两 Tab 都空

    func testEmptyFriendsProducesEmptyDisplayedFriends() {
        let vm = MockFriendsViewModel(friends: [])
        XCTAssertTrue(vm.displayedFriends.isEmpty)
        vm.selectTab(.all)
        XCTAssertTrue(vm.displayedFriends.isEmpty)
    }

    // MARK: - case#6 happy: onlineCount derived 正确（hint epic AC「{onlineCount} 位在线 · 共 {friends.count} 位」）

    func testOnlineCountDerivedFromFriends() {
        let vm = MockFriendsViewModel()
        XCTAssertEqual(vm.onlineCount, vm.friends.filter { $0.online }.count)
        XCTAssertEqual(vm.onlineCount, 6, "scaffold defaults: inRoom 3 + online 3 = 6 在线")
    }

    // MARK: - case#7 守护: RealFriendsViewModel 构造注入 AppState 不 crash + override 不 fatalError + Real override 必 mutate state

    /// 防 RealFriendsViewModel 漏 override 时本测试立刻 fail（fatalError 在测试中 → trap）.
    /// + 守护 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`：
    ///   Real 子类 override 必须本地 mutate state（与 Mock 同语义），禁止只 log（否则 production no-op）.
    func testRealFriendsViewModelOverridesMutateStateNotJustLog() {
        let appState = AppState()
        let vm = RealFriendsViewModel(appState: appState)
        XCTAssertNil(vm.currentRoomId, "init 时 appState.currentRoomId nil → sink 派生 nil")
        XCTAssertFalse(vm.friends.isEmpty, "init 走 defaults seed friends")

        // onJoinFriendTap：必须通过 appState 入口写 currentRoomId（不能仅 log）.
        let inRoomFriend = vm.friends.first(where: { $0.status == .inRoom })!
        let targetRoomId = inRoomFriend.currentRoomId!
        vm.onJoinFriendTap(friend: inRoomFriend)
        XCTAssertEqual(appState.currentRoomId, targetRoomId, "Real path 必须通过 appState 入口写 currentRoomId（守护 lesson）")
        XCTAssertEqual(vm.currentRoomId, targetRoomId, "sink 派生让本字段同步")
        XCTAssertNotNil(vm.lastToastMessage)

        // 重置后 onInviteFriendTap：currentRoomId nil 时必须 mutate（占位 mock 创建）.
        appState.setCurrentRoomId(nil)
        XCTAssertNil(vm.currentRoomId)
        let onlineFriend = vm.friends.first(where: { $0.status == .online })!
        vm.onInviteFriendTap(friend: onlineFriend)
        XCTAssertNotNil(appState.currentRoomId, "Real path 邀请 + 无房间 时必须创建占位（守护 lesson）")
    }

    // MARK: - case#8 守护: Real init 必 seed scaffold defaults（lesson 预防性应用）

    /// 与 Story 37.8 / 37.9 同模式 ——
    /// 防未来 Claude 重构 init 时漏 seed friends 让 RealFriendsViewModel 渲染空好友列表.
    /// lesson: 2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md
    func testRealFriendsViewModelInitSeedsScaffoldDefaults() {
        // parameterless init 路径
        let vm1 = RealFriendsViewModel()
        XCTAssertFalse(vm1.friends.isEmpty)
        XCTAssertEqual(vm1.selectedTab, FriendsScaffoldDefaults.selectedTab)
        XCTAssertNil(vm1.currentRoomId)

        // init(appState:) 路径
        let vm2 = RealFriendsViewModel(appState: AppState())
        XCTAssertFalse(vm2.friends.isEmpty)
        XCTAssertEqual(vm2.selectedTab, FriendsScaffoldDefaults.selectedTab)
    }

    // MARK: - case#9 守护: currentRoomId 派生自 appState.currentRoomId（hydrate + reset 路径）

    /// 防未来 Claude 重构时把 currentRoomId sink 改一次性 hydrate 让 reset 后残留旧 roomId.
    /// lesson: 2026-04-30-published-derived-state-needs-publisher-subscription.md
    /// **关键说明**：currentRoomId 派生源是合法的（"我的房间"语义就是本地用户的房间，appState.currentRoomId 是真理源；
    /// 与 Story 37.8 hostCatName 反例不冲突 —— 那是"看别人房间"语境）.
    func testRealFriendsViewModelCurrentRoomIdDerivesFromAppState() {
        let appState = AppState()
        let vm = RealFriendsViewModel(appState: appState)
        XCTAssertNil(vm.currentRoomId, "appState.currentRoomId nil → 派生 nil")

        // hydrate 路径：写入 currentRoomId → 同步派生
        appState.setCurrentRoomId("9999999")
        XCTAssertEqual(vm.currentRoomId, "9999999")

        // reset 路径：appState.reset() 把 currentRoomId 置 nil → 即时 fallback 到 nil（不残留旧值）
        appState.reset()
        XCTAssertNil(vm.currentRoomId, "reset 后 currentRoomId 必回 nil（防 stale）")
    }

    // MARK: - case#10 守护: bind(appState:) 是同步入口（lesson 预防性应用）

    /// 防未来 Claude 把 bind 改成 async 路径让 RootView .onAppear 触发后第一帧 ViewModel 仍未连上 AppState.
    /// 与 Story 37.8 / 37.9 同模式.
    /// lesson: 2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md
    func testRealFriendsViewModelBindAppStateIsSynchronous() {
        let appState = AppState()
        appState.setCurrentRoomId("8888888")  // 启动期 currentRoomId 已非 nil（restored / UITEST_FORCE_IN_ROOM 模拟）

        let vm = RealFriendsViewModel()  // parameterless init 路径
        XCTAssertNil(vm.currentRoomId, "bind 前 currentRoomId = defaults nil")

        vm.bind(appState: appState)  // 同步路径
        XCTAssertEqual(vm.currentRoomId, "8888888", "bind 后立即派生（无 RunLoop tick 等待）")
    }

    // MARK: - case#11 守护: offline 好友不调 onInviteFriendTap / onJoinFriendTap（视觉禁用 + 行为兜底）

    /// 视觉上 offline 好友不渲染按钮（FriendsScaffoldView FriendRow 三态分支，offline → 纯文本"离线"）；
    /// ViewModel 层不强制阻止 —— 但即便外部错误调用，行为有 sane fallback（不 crash + lastToastMessage 失败提示）.
    /// 守护"offline 好友 join 时 friend.currentRoomId 是 nil → 走 nil guard 分支不 crash"路径.
    func testOnJoinFriendTapWithOfflineFriendDoesNotCrash() {
        let vm = MockFriendsViewModel()
        let offlineFriend = vm.friends.first(where: { $0.status == .offline })!
        XCTAssertNil(offlineFriend.currentRoomId, "offline 好友 currentRoomId nil")

        vm.onJoinFriendTap(friend: offlineFriend)
        XCTAssertNil(vm.currentRoomId, "offline join 走 nil guard 分支 → currentRoomId 不变")
        XCTAssertNotNil(vm.lastToastMessage)
        XCTAssertTrue(vm.lastToastMessage!.contains("不在房间中"))
    }
}
```

> **关键决策**：≥7 case（epic AC 钦定 ≥5 case；本 story 落 11 case 含 5 个守护 case 预防 Story 37.7 / 37.8 / 37.9 lesson 反例）—— 与 Story 37.9 10 case 相比扩到 11 case 是**预防性应用 lesson 6** 的成本兑现（case#7 显式守护"Real override 必 mutate state"的 lesson）。

> **关键决策**：不测 fatalError 路径（基类 abstract method 覆盖在 case#7 间接证明 override 已生效）。

> **关键决策**：不测 FriendsScaffoldView body 渲染含 a11y identifier（属 UITest 范围；详见 AC8）。

> **关键决策**：不测 toast 自动消失（本 story ViewModel 不实装 timer；toast 行为是"写新值即覆盖旧值"，守护测试在 case#2 / case#3 / case#4 已隐含）。

**对应 Tasks**: Task 7.1

### AC8 — UITest a11y identifier 关键锚可定位

**改动文件**: `iphone/PetAppUITests/HomeUITests.swift`（与 Story 37.7 / 37.8 / 37.9 同模式：本 story 加一个新 test case 在 HomeUITests.swift 内；Story 37.13 a11y 总表归并时统一移走）

```swift
// Story 37.10: FriendsScaffoldView 关键 a11y identifier 可定位验证.
// 切到 Friends Tab 后验证主结构 + 2 个 Tab + 至少 1 个 FriendRow + 至少 1 个 friendActionButton 可见.
// 与 Story 37.7 testHomeScaffoldShowsAllSevenAnchors / Story 37.8 testRoomScaffoldShowsAllSevenAnchors / Story 37.9 testWardrobeScaffoldShowsAllAnchors 同模式.
func testFriendsScaffoldShowsAllAnchors() throws {
    let app = XCUIApplication()
    app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
    app.launch()

    let timeout: TimeInterval = 5

    // 切到 Friends Tab
    let friendsTab = app.buttons["tab_friends"]
    XCTAssertTrue(friendsTab.waitForExistence(timeout: timeout), "tab_friends 未找到")
    friendsTab.tap()

    // 验证主容器
    XCTAssertTrue(
        app.descendants(matching: .any)["friendsView"].waitForExistence(timeout: 3),
        "friendsView 主容器未找到"
    )

    // 验证添加按钮
    XCTAssertTrue(
        app.descendants(matching: .any)["friendsAddButton"].exists,
        "friendsAddButton 未找到"
    )

    // 验证 2 个 Tab
    XCTAssertTrue(
        app.descendants(matching: .any)["friendsTab_online"].exists,
        "friendsTab_online 未找到"
    )
    XCTAssertTrue(
        app.descendants(matching: .any)["friendsTab_all"].exists,
        "friendsTab_all 未找到"
    )

    // 验证至少一个 FriendRow（具体 id 由 mock data 决定，验证 scaffold defaults 中第一个 inRoom 好友 u1）
    XCTAssertTrue(
        app.descendants(matching: .any)["friendRow_u1"].exists,
        "friendRow_u1（夏夏 inRoom）未找到"
    )

    // 验证 inRoom 好友的"加入"按钮可定位
    XCTAssertTrue(
        app.descendants(matching: .any)["friendActionButton_u1"].exists,
        "friendActionButton_u1（夏夏加入按钮）未找到"
    )
}
```

> **关键决策**：UITest 不主动验证完整 join 链路 / 切换 Tab 后 list 内容变化（属"完整流程"测试 —— 节点 4 / Story 37.12 范围；本 story 仅验证视觉锚存在，让 Story 37.13 a11y 总表归并时有 baseline）。

> **关键决策**：UITest 路径**不需要** `UITEST_FORCE_IN_ROOM` 类似 env flag —— Friends Tab 不依赖任何 inRoom / inX state，启动后切 tab 即可见全部锚（myRoomCard 在默认路径下不渲染 —— 但本 UITest 不验证 myRoomCard 锚，让 friendsMyRoomCard 锚在 Story 37.12 的"join → currentRoomId 写入"链路 UITest 出现时再覆盖）。

> **现有 testHomeScaffoldShowsAllSevenAnchors / testRoomScaffoldShowsAllSevenAnchors / testWardrobeScaffoldShowsAllAnchors**（Story 37.7 / 37.8 / 37.9）**不动** —— 本 story 范围是 Friends Tab，不影响其它 Tab UITest。

**对应 Tasks**: Task 8.1

### AC9 — xcodegen regen + build verify

完成 AC1-AC8 后：

1. `cd iphone && xcodegen generate` 让新文件加入 PetApp / PetAppTests target（project.yml `sources: - PetApp` / `- PetAppTests` 通配规则自动 inclusion；新增文件全部在 `iphone/PetApp/Features/Friends/` + `iphone/PetAppTests/Features/Friends/` 下）
2. `bash iphone/scripts/build.sh --test` 跑测试通过
   - 总 case 数：~293（Story 37.9 落地后基线 ~293 unit + 4 UITest）+ 本 story 新增 11 unit case + 1 UITest case → ~304 unit + 5 UITest case 全绿
   - 不删除任何老 case
3. grep 验证：
   - `grep -c "class FriendsViewModel" iphone/PetApp/Features/Friends/ViewModels/FriendsViewModel.swift` ≥ 1（防漏建基类）
   - `grep "fatalError" iphone/PetApp/Features/Friends/ViewModels/FriendsViewModel.swift` 输出至少 2 次（onInviteFriendTap + onJoinFriendTap abstract method）
   - `grep "final class FriendsViewModel" iphone/PetApp/Features/Friends/ViewModels/FriendsViewModel.swift` 输出空（基类不能 final）
   - `grep -c "override func" iphone/PetApp/Features/Friends/ViewModels/MockFriendsViewModel.swift` ≥ 2（onInviteFriendTap + onJoinFriendTap override）
   - `grep -c "override func" iphone/PetApp/Features/Friends/ViewModels/RealFriendsViewModel.swift` ≥ 2
   - **关键**：`grep -c "appState?.setCurrentRoomId" iphone/PetApp/Features/Friends/ViewModels/RealFriendsViewModel.swift` ≥ 2（Real override 必 mutate state，按 Story 37.9 round 1 P1 lesson）
   - `grep -c "FriendsScaffoldView" iphone/PetApp/Features/Friends/Views/FriendsView.swift` ≥ 1（caller 替换已生效）
   - `grep "Text(\"Friends Tab Placeholder\")" iphone/PetApp/Features/Friends/Views/FriendsView.swift` 输出空（旧占位 Text 已替换）

> **dev 实装备注**：dev 必须 `xcodegen generate` 后 commit 一并提交 `iphone/PetApp.xcodeproj/project.pbxproj` 改动（与 Story 37.5 / 37.6 / 37.7 / 37.8 / 37.9 同模式）。

**对应 Tasks**: Task 9.1, 9.2, 9.3

### AC10 — Deliverable 清单

- ✅ `iphone/PetApp/Features/Friends/ViewModels/FriendsViewModel.swift` 新建（class + 4 字段 + 2 abstract method fatalError 占位 + 1 concrete view-action method + 4 derived computed property + parameterless init）
- ✅ `iphone/PetApp/Features/Friends/ViewModels/MockFriendsViewModel.swift` 新建（final + invocations + 默认 ScaffoldDefaults seed + 可注入构造）
- ✅ `iphone/PetApp/Features/Friends/ViewModels/RealFriendsViewModel.swift` 新建（final + appState 构造注入 + parameterless init + bind(appState:) + 1 sink + 2 override **本地 mutate state**）
- ✅ `iphone/PetApp/Features/Friends/ViewModels/FriendsScaffoldDefaults.swift` 新建（selectedTab / currentRoomId / friends 3 字段共享）
- ✅ `iphone/PetApp/Features/Friends/Models/Friend.swift` 新建（id/name/online/status/statusText/currentRoomId/color + Equatable + Identifiable + Sendable）
- ✅ `iphone/PetApp/Features/Friends/Models/FriendStatus.swift` 新建（3 case + CaseIterable + Identifiable + Sendable）
- ✅ `iphone/PetApp/Features/Friends/Models/FriendsTab.swift` 新建（2 case + label + CaseIterable + Identifiable + Sendable）
- ✅ `iphone/PetApp/Features/Friends/Views/FriendsScaffoldView.swift` 新建（struct + 5 区块视觉按 ui_design friends.jsx 像素级翻译 + #Preview 4 配置 candy/dark × 默认/has-room/empty 场景）
- ✅ `iphone/PetAppTests/Features/Friends/FriendsViewScaffoldTests.swift` 新建（11 case：6 epic AC + 5 守护 case 预防 lesson 反例）
- ✅ `iphone/PetApp/Features/Friends/Views/FriendsView.swift` 修改（占位 Text 替换为 FriendsScaffoldView + 加 @EnvironmentObject 取出 friendsViewModel）
- ✅ `iphone/PetApp/App/RootView.swift` 修改（追加 `@StateObject friendsViewModel: FriendsViewModel = RealFriendsViewModel()` + `.environmentObject(friendsViewModel)` + `.onAppear` 内同步 bind(appState:) + LaunchedContentView 接收 friendsViewModel 透传）
- ✅ `iphone/PetApp/App/MainTabView.swift` 修改（Preview 注入 MockFriendsViewModel）
- ✅ `iphone/PetAppUITests/HomeUITests.swift` 加 `testFriendsScaffoldShowsAllAnchors`
- ✅ `iphone/PetApp.xcodeproj/project.pbxproj` 改动随 commit（xcodegen regen 结果）
- ✅ `bash iphone/scripts/build.sh --test` 全绿（~304 unit case + 5 UITest case）
- ✅ project.yml **不**手动改（通配规则自动 inclusion）
- ✅ RootView wire 用 RealFriendsViewModel 而非裸基类（防 fatalError 生产 crash 路径，按 Story 37.7 lesson 钦定）
- ✅ RealFriendsViewModel.onInviteFriendTap / onJoinFriendTap **本地 mutate state**（按 Story 37.9 round 1 P1 lesson 钦定）
- ✅ `FriendsView.swift` **不**删除（保 git history；Story 37.13 决定）
- ✅ MainTabView 内 `FriendsView()` 调用站不变（caller 漏改靠编译器报错驱动 —— FriendsView 类型签名不变；body 内部改）

## Tasks / Subtasks

- [x] Task 1: FriendsViewModel 基类（AC1）
  - [x] 1.1 新建 `iphone/PetApp/Features/Friends/ViewModels/FriendsViewModel.swift`：`@MainActor public class FriendsViewModel: ObservableObject` + 4 个 @Published 字段（friends / selectedTab / currentRoomId / lastToastMessage）+ 2 abstract method (onInviteFriendTap / onJoinFriendTap) fatalError 占位 + 1 concrete method (selectTab) + 4 derived computed property（onlineCount / onlineFriends / allFriends / displayedFriends）+ parameterless init()
  - [x] 1.2 显式 `import Foundation` + `import Combine`（防 transitive @Published；与 MockHomeViewModel round 4 [P0] hardening 同精神）
- [x] Task 2: Mock/Real 子类（AC2）
  - [x] 2.1 新建 `iphone/PetApp/Features/Friends/ViewModels/MockFriendsViewModel.swift`（final class + invocations 数组 + 2 override + 默认 ScaffoldDefaults seed + 可配 init）
  - [x] 2.2 新建 `iphone/PetApp/Features/Friends/ViewModels/RealFriendsViewModel.swift`（final class + appState 构造注入 + parameterless init() / init(appState:) 双路径 + bind(appState:) idempotent + 1 sink (subscribeCurrentRoomId) + 2 override **本地 mutate** 通过 appState.setCurrentRoomId 入口 + lastToastMessage 写入；按 Story 37.9 round 1 P1 lesson 钦定路径）
- [x] Task 3: 数据模型 + ScaffoldDefaults（AC3）
  - [x] 3.1 新建 `iphone/PetApp/Features/Friends/Models/Friend.swift`（struct value type + 7 字段 + Equatable + Identifiable + Sendable + Color? 字段需 import SwiftUI）
  - [x] 3.2 新建 `iphone/PetApp/Features/Friends/Models/FriendStatus.swift`（enum 3 case + CaseIterable + Identifiable + Sendable）
  - [x] 3.3 新建 `iphone/PetApp/Features/Friends/Models/FriendsTab.swift`（enum 2 case + label + CaseIterable + Identifiable + Sendable）
  - [x] 3.4 新建 `iphone/PetApp/Features/Friends/ViewModels/FriendsScaffoldDefaults.swift`（3 字段：selectedTab / currentRoomId / friends；friends 8 件三态混合）
- [x] Task 4: FriendsScaffoldView struct + 5 区块（AC4）
  - [x] 4.1 新建 `iphone/PetApp/Features/Friends/Views/FriendsScaffoldView.swift`，含 VStack(spacing: 0) 5 区块结构 + accessibilityIdentifier "friendsView" + .overlay(alignment: .bottom) toastOverlay
  - [x] 4.2 落地 topCard 子视图（左 VStack onlineCount + "好友" 标题 / 右 IconButton 圆形 plus 添加按钮 accessibilityIdentifier `friendsAddButton`；点击仅 print log + 写 lastToastMessage）
  - [x] 4.3 落地 myRoomCard 子视图（条件渲染 `state.currentRoomId != nil`；HStack 圆形 paw icon + VStack "你的房间" + roomId monospaced；accessibilityIdentifier `friendsMyRoomCard`；**不**渲染分享按钮）
  - [x] 4.4 落地 tabBar 子视图（HStack + ForEach FriendsTab.allCases 2 个 Button + 选中态视觉 + accessibilityIdentifier `friendsTab_<rawValue>` + 调 state.selectTab(tab)）
  - [x] 4.5 落地 friendsList 子视图（ScrollView + LazyVStack + ForEach state.displayedFriends 渲染 FriendRow + 空态 fallback 文案）
  - [x] 4.6 落地 FriendRow 子视图（HStack Avatar + VStack name(含房间中角标 if inRoom) + statusText + 三态按钮 switch f.status：inRoom 实心加入 / online 描边邀请 / offline 纯文本"离线"）+ accessibilityIdentifier `friendRow_<f.id>` + `friendActionButton_<f.id>`（仅 inRoom / online 渲染按钮 → 仅这两态有 actionButton id）
  - [x] 4.7 落地 toastOverlay（仅 lastToastMessage 非 nil 时渲染；底部浮动黑底 alpha 0.85 + accessibilityIdentifier `friendsToast`）
  - [x] 4.8 **lesson 预防性应用**：所有 shadow 必须挂在 `RoundedRectangle.fill(...).shadow(...)` 那一层（不挂最外层 chain），按 Story 37.6 round 5 lesson 钦定路径
- [x] Task 5: FriendsView caller 替换 + RootView wire + LaunchedContentView 透传 + MainTabView Preview（AC5）
  - [x] 5.1 改 `FriendsView.swift`：body 占位 Text 替换为 `FriendsScaffoldView(state: friendsViewModel)`；加 `@EnvironmentObject var friendsViewModel: FriendsViewModel`；保留 NavigationStack 包裹层
  - [x] 5.2 改 `RootView.swift`：在 `@StateObject wardrobeViewModel` 同级追加 `@StateObject private var friendsViewModel: FriendsViewModel = RealFriendsViewModel()`（用 Real 子类避免基类 fatalError 生产路径）+ LaunchedContentView 调用站追加 `friendsViewModel: friendsViewModel`
  - [x] 5.3 改 LaunchedContentView 内部签名：追加 `friendsViewModel: FriendsViewModel` 字段 + init 参数 + body 内 `.environmentObject(friendsViewModel)`（与 homeViewModel / roomViewModel / wardrobeViewModel 同模式）
  - [x] 5.4 改 `RootView.swift` `.onAppear`：在已有 `if let realWardrobeVM = wardrobeViewModel as? RealWardrobeViewModel { realWardrobeVM.bind(appState: appState) }` 后追加 `if let realFriendsVM = friendsViewModel as? RealFriendsViewModel { realFriendsVM.bind(appState: appState) }`（按 Story 37.8 round 2 P2 lesson `2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md` 钦定路径，**不**放 `.task`）
  - [x] 5.5 改 `MainTabView.swift` Preview：在已有 `.environmentObject(MockWardrobeViewModel() as WardrobeViewModel)` 同级追加 `.environmentObject(MockFriendsViewModel() as FriendsViewModel)`
- [x] Task 6: #Preview 4 配置（AC6）
  - [x] 6.1 FriendsScaffoldView 文件底部 `#if DEBUG` 块加 4 个 `#Preview`（candy 默认 / dark 默认 / candy has-room / candy empty）
- [x] Task 7: 单元测试（AC7）
  - [x] 7.1 新建 `iphone/PetAppTests/Features/Friends/FriendsViewScaffoldTests.swift`，落地 11 case（6 epic AC + 5 守护 case 预防 lesson 反例 —— 含 lesson 1/2/3/4/5/6 全部命中）
- [x] Task 8: UITest（AC8）
  - [x] 8.1 在 `HomeUITests.swift` 加 `testFriendsScaffoldShowsAllAnchors`（不需要 env flag；切 tab 即可见）
  - [x] 8.2 验证现有 `testHomeScaffoldShowsAllSevenAnchors` / `testRoomScaffoldShowsAllSevenAnchors` / `testWardrobeScaffoldShowsAllAnchors` 不受影响（不动）
- [x] Task 9: xcodegen regen + build verify（AC9）
  - [x] 9.1 `cd iphone && xcodegen generate` 让新文件加入 target
  - [x] 9.2 `bash iphone/scripts/build.sh --test` 跑测试通过（~304 unit + 5 UITest case 全绿）
  - [x] 9.3 grep 校验：FriendsViewModel 含 `class FriendsViewModel`（去 final）+ ≥2 个 fatalError；MockFriendsViewModel / RealFriendsViewModel 各含 ≥2 个 override func；RealFriendsViewModel 含 ≥2 个 `appState?.setCurrentRoomId`（守护 lesson 6）；FriendsView 含 FriendsScaffoldView 调用 + 不含 `Text("Friends Tab Placeholder")` 调用
- [x] Task 10: Deliverable 清单确认（AC10）
  - [x] 10.1 8 个新文件 + 修改 4 个老文件（FriendsView.swift / RootView.swift / MainTabView.swift / HomeUITests.swift）+ pbxproj regen 全部待 commit（不在本 dev-story 范围）

## Dev Notes

### Story 37.7 / 37.8 / 37.9 沉淀 lesson 预防性应用清单（关键约束 —— 全部 8 条命中）

本 story 落地前必读 8 条 lesson；**不重蹈覆辙**清单（与 epic-loop 调用提示中 8 条 lesson 一一对应）：

| # | Lesson 文件 | 预防点 | 本 story 落地动作 |
|---|---|---|---|
| 1 | `2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md` | abstract method base class 注入点全部要换 concrete subclass | RootView `@StateObject friendsViewModel: FriendsViewModel = RealFriendsViewModel()` 而非裸基类（AC5 Task 5.2） |
| 2 | `2026-04-30-published-derived-state-needs-publisher-subscription.md` | 派生 state 必须订阅 publisher，禁止 hardcode（避免 reset 后 stale） | RealFriendsViewModel 用 sink 订阅 appState.$currentRoomId 派生 currentRoomId；**不**在 init / bind 入口一次性 hydrate（AC2 + 守护 case#9） |
| 3 | `2026-04-30-room-host-name-must-not-derive-from-local-current-pet.md` | 不要从 currentPet 派生其他用户的信息（local pet ≠ remote owner） | **本 story 反向应用 lesson** —— Friends 域 currentRoomId **是合法**派生自 appState.currentRoomId（"我的房间"语义就是本地用户的房间）；**反例**：friends 列表中**不**派生自 currentPet（friends 是别人的列表）；详见 Dev Notes "currentRoomId 派生源 vs hostCatName 反例" |
| 4 | `2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md` | RealViewModel.init 必须 seed scaffold defaults（避免 in-room/in-X state 出现空内容） | 新建 `FriendsScaffoldDefaults` 共享 enum；Mock / Real 双子类 init 都用它 seed 全 3 字段（AC2 + AC3 + 守护 case#8） |
| 5 | `2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md` | `.onAppear` 同步 bind appState（避免 launch-time race） | RootView `.onAppear` 内追加 `realFriendsVM.bind(appState: appState)`，**不**放 `.task`（AC5 Task 5.4 + 守护 case#10） |
| **6** | **`2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`** | **RealViewModel.override 占位方法必须实装本地 mutate state（log-only 是 [P1]）** | **RealFriendsViewModel.onInviteFriendTap / onJoinFriendTap 必须通过 `appState?.setCurrentRoomId(...)` 入口本地 mutate state + 写 lastToastMessage（AC2 + 守护 case#7）；grep 校验 `appState?.setCurrentRoomId` ≥ 2（AC9）—— 这条是本 story SM 重点强调的 lesson** |
| 7 | `2026-04-30-swiftui-onchange-equatable-and-stale-task-cancel.md` | SwiftUI .onChange iOS 17+ 双参签名 | 本 story FriendsScaffoldView 不主动用 .onChange（无 timer 类视觉 transient）；若未来加 selectedTab 联动动画时必须按 iOS 17+ 双参签名 |
| 8 | `2026-04-30-swiftui-explicit-id-nil-shared-identity.md` + `2026-04-30-swiftui-state-survives-id-and-shadow-over-children.md` + `2026-04-30-swiftui-floating-emoji-needs-state-driven-position.md` | shadow / .id() / @State 驱动浮动动画 等 SwiftUI primitives 注意点（已沉淀到 Story 37-6 lessons） | FriendRow / Card / IconButton shadow 全部挂 `RoundedRectangle.fill(...).shadow(...)` 那一层（AC4 Task 4.8）；ForEach 用 `state.displayedFriends` 自带 .id；本 story 无浮动动画路径 |

### currentRoomId 派生源 vs Story 37.8 hostCatName 反例（关键澄清表 —— lesson 3 命中说明）

Story 37.8 round 3 lesson `2026-04-30-room-host-name-must-not-derive-from-local-current-pet.md` 指出：**Room 域 hostCatName 不可派生自 appState.currentPet（"本地用户的猫" ≠ "房间 host 的猫"）**。但**本 story Friends 域 currentRoomId 派生自 appState.currentRoomId 是合法的**。两者看似冲突，但语义独立：

| 维度 | Story 37.8 RoomViewModel.hostCatName | Story 37.10 FriendsViewModel.currentRoomId |
|---|---|---|
| 域语义 | "看 room host 的小屋"——host 可能是别人 | "我的房间"——roomId 永远是本地用户当前所在房间（Friends 视角下） |
| 真理源 | WS room.snapshot（Story 12.1 后到来） | appState.currentRoomId（"本地用户当前所在房间"语义钦定） |
| pre-feature 占位 | RoomScaffoldDefaults.hostCatName（永远占位直到 WS 接通） | FriendsScaffoldDefaults.currentRoomId（默认 nil；hydrate / 用户 join 后即派生） |
| 是否 sink 订阅 currentXxx | ❌ 错（用户加入别人房间时显示"我的猫的小屋"是 user-visible bug） | ✅ 对（"我的房间"无歧义就是本地用户当前所在的，appState.currentRoomId 就是真理源） |
| ADR-0010 §3.2 字段语义 | currentPet = "本地用户的猫"（Wardrobe / Friends "我的房间提示" 视角下就是真理源；Room 视角下不是） | 同上 |

> **关键判断标准**：lesson 3 的精神是"判断目标 X 的真理源是不是'本地用户的某条信息'，是 → 可派生；否 → 用 placeholder"。Friends 域的"我的房间号"无歧义就是本地用户的；Friends 域的"好友列表"语义不是本地用户的（**friends 不派生自 appState 任何字段** —— 后续 epic 真接 server `/friends` 接口时改 sink）。

### lesson 6 命中说明（关键 —— Story 37.9 第一次复犯，本 story 不能重犯）

Story 37.9 round 1 codex review 抓出 [P1]：`RealWardrobeViewModel.onEquipTap` 仅 log，**不**改 equipped 字段，导致 production app 用户点装备按钮 no-op（详见 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`）。该 lesson 的核心规则：

> **Real 子类 override abstract method 时必须实装"本 story 范围内能让 UI 视觉工作的最小 placeholder 行为"**，禁止只 log。占位行为通常**与 Mock 子类同语义 copy**。Server / UseCase 真实写入是未来 story 的事，**本 story 范围内 production app 必须可用**。

**本 story 强制应用**：
- `RealFriendsViewModel.onInviteFriendTap` 必须本地 mutate（通过 `appState?.setCurrentRoomId("1234567")` + 写 `lastToastMessage`）—— 不能只 log
- `RealFriendsViewModel.onJoinFriendTap` 必须本地 mutate（通过 `appState?.setCurrentRoomId(friend.currentRoomId)` + 写 `lastToastMessage`）—— 不能只 log
- 守护 case#7 显式断言 `appState.currentRoomId == targetRoomId` after `onJoinFriendTap` —— 未来 Claude 重构若改回 log-only，本测试立即 fail
- AC9 grep 校验 `appState?.setCurrentRoomId` 在 RealFriendsViewModel.swift 内出现 ≥ 2 次 —— 让"override 必 mutate"契约钉成机器可校验的规则

> **关键决策**：Real path 通过 `appState?.setCurrentRoomId(...)` 入口而**不**直接写 `self.currentRoomId` —— 让 sink 派生本字段（与 Story 12.7 / 37.12 落地后真实路径一致：UseCase 调 server → 成功后写 appState → sink 派生）。这样 Story 37.12 / 12.7 落地真实 UseCase 时只需把"本地直接写 appState 占位串"换成"调 UseCase + 成功回调写 appState"，FriendsScaffoldView 视图与 RealFriendsViewModel sink 链路 zero edit。**附带好处**：兄弟 ViewModel（如 RealRoomViewModel 也订阅 currentRoomId 时）会立即同步切到 inRoom 视图。

### myRoomCard 分享按钮决策（边界灰区 + dev/reviewer 协商点）

epic AC line 4807 字面要求：「我的房间提示条 Card：仅当 `appState.currentRoomId != nil` 时显示；含房间 id 字符串 + "分享给好友"次要按钮（占位 toast，本 epic 不真分享）」。

**SM 决策**：本 story **不渲染**分享按钮，理由：
1. **减少占位噪声**：占位按钮等于多一个未来废弃的 a11y identifier + invocations 路径（Story 35.x 节点 12 后真实分享落地时该按钮要重做）
2. **myRoomCard 视觉密度**：epic AC 已有"代码 {roomId}"显示，加分享按钮让卡片宽度紧（参照 friends.jsx 视觉 friends.jsx:25-44 也没分享按钮 —— ui_design 视觉**没有**分享按钮，与 epic AC 文字描述实际略不一致）
3. **与 PRD 边界一致**：本 epic 不实装真实分享流程（Story 37.14 白名单文档钦定该决议）

**dev / reviewer 协商点**：
- 若 dev 实装时觉得 myRoomCard 缺按钮看起来太空 → 可加一个占位 PrimaryButton(variant: .secondary) "分享给好友"（点击仅 print + 写 lastToastMessage "分享功能敬请期待"）；不需要新一轮 SM 改 spec
- 若 review 反馈要求保留占位按钮 → 在 fix-review round 加（一行 PrimaryButton 即可；a11y identifier `friendsMyRoomShareButton`）

> **关键决策**：把这个边界灰区**显式声明在 spec 内**（而非偷偷砍掉）—— 让 dev / reviewer 看到"为什么没渲染分享按钮"+ 让 fix-review 路径有迹可循。

### FriendsViewModel 改 class 而非 protocol any 模式（关键设计决策）

**选定**: 基类 `class FriendsViewModel: ObservableObject`（非 final）+ 子类 `MockFriendsViewModel: FriendsViewModel` / `RealFriendsViewModel: FriendsViewModel` 各自 final。

**为何不走 `protocol FriendsViewModelProtocol + any P`**：与 Story 37.7 / 37.8 / 37.9 同精神（v2.1 BLOCKER 7）—— SwiftUI `@ObservedObject` 不接受 `any P`；让 caller 端类型膨胀。

**为何 FriendsScaffoldView 选择非泛型 struct**（不像 HomeView<ChestSlot: View> 是泛型）：Friends 没有"chestSlot 接缝点"这种泛型必要场景；若未来加好友详情页 NavigationLink slot，再走泛型 ViewBuilder 路径。

### FriendsView 占位 stub 不删保 git history（关键约束）

`FriendsView.swift` 在 Story 37.3 落地时作为 Friends Tab 占位 stub；本 story **不删除**该文件，理由：
1. **保 git history 可读**：dev 阅读 git log 能看到 `FriendsView` 是 Story 37.3 临时方案 → Story 37.10 替换 body 占位 Text 为 FriendsScaffoldView 的演进足迹；删除会让 git blame 失去线索。
2. **MainTabView 调用站类型不变**：`MainTabView` 的 `FriendsView().tag(AppTab.friends)` 调用站签名不变；本 story 仅改 body 内部结构。这与 Story 37.9 WardrobeView 不删同精神。
3. **Story 37.13 a11y 总表归并时统一清理**：Story 37.13 决定是否一并清理 / 重命名（可能改名 `FriendsRootView` 等）；本 story 不收口。

### state owner 边界：selectedTab / currentRoomId / lastToastMessage 走 ViewModel @Published 而非 @State

ADR-0010 §3.2 表格"表单输入 / 当前选中 → ViewModel 或 SwiftUI @State"二选一；判断标准是**是否需要跨 View 触发 / 单元测试需要断言**：

| 场景 | 选择 | 理由 |
|---|---|---|
| `selectedTab` | ViewModel @Published | 单元测试需要断言（case#1 切 Tab 后 displayedFriends 派生改变）；放 @State 让单元测试无法直接断言（必须走 ViewInspector） |
| `currentRoomId` | ViewModel @Published | RealFriendsViewModel 通过 sink 订阅 appState.$currentRoomId 派生写入，需要 SwiftUI 监听变化；放 @State 不能从 ViewModel 写入 |
| `lastToastMessage` | ViewModel @Published | Mock / Real onInviteFriendTap / onJoinFriendTap 都从 ViewModel 写入；需要测试断言（case#2 / case#3 / case#4 / case#7 / case#11） |

### RealFriendsViewModel friends 字段策略（关键决策 + 后续 epic 接续点）

本 story `RealFriendsViewModel` **不**为 `friends` 字段建 sink —— 本 story 范围内 friends 永远走 `FriendsScaffoldDefaults.friends` seed（好友列表 server 接口在后续 epic 才落地）。

**为何不预先建 sink**：① 后续 epic 真实 server 接口形态未定（可能 `/friends` REST endpoint，可能 WS push，可能 GraphQL）—— 提前 hookup sink 等于让 dev 重写工作量；② 提前 mapping 是 over-engineer（参考 ADR-0010 §4.4 缓解策略）。

**接续点**：未来 epic 落地真实好友列表接口时新增 `subscribeFriends(to: appState)` sink（或直接调 LoadFriendsUseCase）；其它 ViewModel / View 代码 zero edit。

### 测试边界（XCTest only）

本 story 测试**仅**用 XCTest + @testable import PetApp + UITest（XCUIApplication）—— **不**引：

- ❌ SnapshotTesting（视觉 diff）：视觉验证靠 #Preview + Story 37.13 visual-review-checklist
- ❌ ViewInspector（SwiftUI body 内省）：FriendsScaffoldView body 渲染契约靠 ui_design 1:1 翻译 + Preview 抽样兜底
- ❌ Mockingbird / Cuckoo（mock codegen）：MockFriendsViewModel 是手写 final class subclass

### Story 37.7 / 37.8 / 37.9 衔接：与 HomeView / RoomScaffoldView / WardrobeScaffoldView 同 patterns 全表

| 维度 | HomeView (Story 37.7) | RoomScaffoldView (Story 37.8) | WardrobeScaffoldView (Story 37.9) | FriendsScaffoldView (本 story) |
|---|---|---|---|---|
| 文件命名 | `HomeView.swift` (改写) | `RoomScaffoldView.swift` (新建)（旧 `RoomViewPlaceholder.swift` 不动） | `WardrobeScaffoldView.swift` (新建)（旧 `WardrobeView.swift` body 改写但保文件） | `FriendsScaffoldView.swift` (新建)（旧 `FriendsView.swift` body 改写但保文件 + 改 NavigationStack 包裹层 + 加 @EnvironmentObject） |
| struct 签名 | `HomeView<ChestSlot: View>` 泛型 | `RoomScaffoldView` 非泛型 | `WardrobeScaffoldView` 非泛型 | `FriendsScaffoldView` 非泛型 |
| state owner | `@ObservedObject var state: HomeViewModel` 基类 | `@ObservedObject var state: RoomViewModel` 基类 | `@ObservedObject var state: WardrobeViewModel` 基类 | `@ObservedObject var state: FriendsViewModel` 基类 |
| ViewModel 基类 | `class HomeViewModel`（去 final）+ 5 字段 + 5 abstract method | `class RoomViewModel`（class）+ 4 字段 + 2 abstract method | `class WardrobeViewModel`（class）+ 5 字段 + 1 abstract method + 2 concrete method + 3 derived | `class FriendsViewModel`（class）+ 4 字段 + 2 abstract method + 1 concrete method + 4 derived |
| Mock 子类 | `MockHomeViewModel`（final）+ invocations | `MockRoomViewModel`（final）+ invocations | `MockWardrobeViewModel`（final）+ invocations | `MockFriendsViewModel`（final）+ invocations |
| Real 子类 override 行为 | onJoinTap 写 showJoinModal（mutate） | onLeaveTap 写 setCurrentRoomId(nil) | onEquipTap 本地 toggle equipped（**round 1 P1 fix 后**） | onInviteFriendTap / onJoinFriendTap 通过 appState.setCurrentRoomId 入口 mutate（**lesson 6 预防性应用**） |
| Defaults 共享 enum | （未抽，Story 37.7 不需要） | `RoomScaffoldDefaults`（4 字段） | `WardrobeScaffoldDefaults`（4 字段） | `FriendsScaffoldDefaults`（3 字段） |
| 数据模型 | `PetStats` / `AnimationState` 新建 | `RoomMember` 新建 | `CosmeticItem` / `CosmeticCategory` 新建 | `Friend` / `FriendStatus` / `FriendsTab` 新建 |
| 区块 | 5 区块 | 5 区块 | 4 区块 | 5 区块（含 toastOverlay） |
| State (transient) | `@State resetTask` | `@State copiedFeedback` + `@State copyFeedbackTask` | (无 SwiftUI @State) | (无 SwiftUI @State) |
| 老占位文件处理 | 无（HomeView 改写） | RoomViewPlaceholder.swift 保留不删 | WardrobeView.swift 保留不删 | FriendsView.swift 保留不删 |
| caller 改动 | HomeContainerHomeViewBridge 改新 init 签名 | HomeContainerView inRoom 分支改 caller + 新增 HomeContainerRoomViewBridge | WardrobeView body 直接改 | FriendsView body 直接改（占位 Text → FriendsScaffoldView；保 NavigationStack） |
| RootView wire | `@StateObject homeViewModel = RealHomeViewModel()` | 追加 `@StateObject roomViewModel = RealRoomViewModel()` | 追加 `@StateObject wardrobeViewModel = RealWardrobeViewModel()` | 追加 `@StateObject friendsViewModel = RealFriendsViewModel()` |
| `.onAppear` bind | bind appState（已有） | 追加 `realRoomVM.bind(appState:)` | 追加 `realWardrobeVM.bind(appState:)` | 追加 `realFriendsVM.bind(appState:)` |
| #Preview 数 | 2（candy / dark） | 4（candy 4/2/1 + dark 4） | 4（candy default / dark default / candy bow / candy empty） | 4（candy default / dark default / candy has-room / candy empty） |
| 单元测试 case 数 | 6（≥4 epic AC） | 5（≥4 epic AC） | 10（≥4 epic AC + 6 守护 case） | 11（≥5 epic AC + 6 守护 case 含 lesson 6） |
| UITest case | `testHomeScaffoldShowsAllSevenAnchors` | `testRoomScaffoldShowsAllSevenAnchors` + 新 env `UITEST_FORCE_IN_ROOM` | `testWardrobeScaffoldShowsAllAnchors` + 切 tab 路径 | `testFriendsScaffoldShowsAllAnchors` + 切 tab 路径（无需 env flag） |
| a11y identifier | inline 7 锚 | inline 8 锚 | inline 12+ 锚 | inline 8+ 锚（friendsView / friendsAddButton / friendsMyRoomCard / friendsTab_*2 / friendRow_*N / friendActionButton_*N / friendsToast） |
| 老 a11y 常量 | 保留 AccessibilityID.Home.* | 不引入 AccessibilityID.Room（Story 37.13 归并） | 不引入 AccessibilityID.Wardrobe（Story 37.13 归并） | 不引入 AccessibilityID.Friends（Story 37.13 归并） |

### 5 区块视觉契约（详细 ui_design 翻译表）

按 `iphone/ui_design/source/screens/friends.jsx` + ui_design CSS 像素级翻译：

#### topCard（friends.jsx:9-23）

```swift
HStack(alignment: .center) {
    // 左：VStack 在线人数 + 标题
    VStack(alignment: .leading, spacing: 0) {
        Text("\(state.onlineCount) 位在线 · 共 \(state.friends.count) 位")
            .font(.system(size: 12, weight: .bold))
            .foregroundColor(theme.colors.inkSoft)
        Text("好友")
            .font(.system(size: 22, weight: .heavy))
            .foregroundColor(theme.colors.ink)
    }

    Spacer()

    // 右：圆形 plus 按钮
    Button(action: {
        os_log(.debug, "friendsAddButton tap (后续 epic will wire add friend flow)")
        state.lastToastMessage = "添加好友功能敬请期待"
    }) {
        Image(systemName: Icons.symbol(for: "plus"))
            .font(.system(size: 20))
            .foregroundColor(theme.colors.ink)
            .frame(width: 40, height: 40)
            .background(
                Circle()
                    .fill(theme.colors.surface)
                    .shadow(color: theme.shadow.sm.color, radius: theme.shadow.sm.radius, x: theme.shadow.sm.x, y: theme.shadow.sm.y)
            )
            .overlay(Circle().stroke(theme.colors.border, lineWidth: 1))
    }
    .accessibilityIdentifier("friendsAddButton")
}
.padding(.top, 68)
.padding(.horizontal, 20)
.padding(.bottom, 8)
```

#### myRoomCard（friends.jsx:25-44；条件渲染 currentRoomId != nil）

```swift
@ViewBuilder
private var myRoomCard: some View {
    if let roomId = state.currentRoomId {
        HStack(spacing: 10) {
            // 圆形 paw icon
            Image(systemName: Icons.symbol(for: "paw"))
                .font(.system(size: 18))
                .foregroundColor(.white)
                .frame(width: 36, height: 36)
                .background(Circle().fill(theme.colors.accent))

            VStack(alignment: .leading, spacing: 0) {
                Text("你的房间")
                    .font(.system(size: 11, weight: .bold))
                    .foregroundColor(theme.colors.inkSoft)
                HStack(spacing: 0) {
                    Text("代码 ")
                        .font(.system(size: 14, weight: .heavy))
                        .foregroundColor(theme.colors.ink)
                    Text(roomId)
                        .font(.system(size: 14, weight: .heavy, design: .monospaced))
                        .foregroundColor(theme.colors.accentDeep)
                        .tracking(2)
                }
            }
            Spacer()
        }
        .padding(.horizontal, 14)
        .padding(.vertical, 10)
        .background(
            RoundedRectangle(cornerRadius: 16)
                .fill(LinearGradient(
                    colors: [theme.colors.accentSoft, .clear],
                    startPoint: .leading, endPoint: .trailing
                ))
        )
        .overlay(RoundedRectangle(cornerRadius: 16).stroke(theme.colors.border, lineWidth: 1))
        .padding(.horizontal, 20)
        .padding(.top, 4)
        .padding(.bottom, 8)
        .accessibilityIdentifier("friendsMyRoomCard")
    }
}
```

#### tabBar（friends.jsx:47-57）

```swift
HStack(spacing: 6) {
    ForEach(FriendsTab.allCases) { tab in
        tabButton(tab)
    }
    Spacer()
}
.padding(.horizontal, 20)
.padding(.vertical, 6)

private func tabButton(_ tab: FriendsTab) -> some View {
    let isSelected = state.selectedTab == tab
    return Button(action: { state.selectTab(tab) }) {
        Text(tab.label)
            .font(.system(size: 12, weight: .heavy))
            .padding(.vertical, 7)
            .padding(.horizontal, 18)
            .foregroundColor(isSelected ? theme.colors.surface : theme.colors.inkSoft)
            .background(
                RoundedRectangle(cornerRadius: 14)
                    .fill(isSelected ? theme.colors.ink : theme.colors.surface)
            )
            .overlay(
                Group {
                    if !isSelected {
                        RoundedRectangle(cornerRadius: 14).stroke(theme.colors.border, lineWidth: 1)
                    }
                }
            )
    }
    .accessibilityIdentifier("friendsTab_\(tab.rawValue)")
}
```

#### friendsList + FriendRow（friends.jsx:60-126）

```swift
ScrollView {
    LazyVStack(spacing: 8) {
        if state.displayedFriends.isEmpty {
            Text("暂无好友在线～")
                .font(.system(size: 13, weight: .semibold))
                .foregroundColor(theme.colors.inkSoft.opacity(0.6))
                .frame(maxWidth: .infinity)
                .padding(40)
        } else {
            ForEach(state.displayedFriends) { friend in
                friendRow(friend)
            }
        }
    }
    .padding(.horizontal, 20)
    .padding(.top, 8)
    .padding(.bottom, 100)
}

private func friendRow(_ f: Friend) -> some View {
    HStack(spacing: 12) {
        Avatar(name: f.name, size: 48, color: f.color, online: f.online)

        VStack(alignment: .leading, spacing: 2) {
            HStack(spacing: 4) {
                Text(f.name)
                    .font(.system(size: 14, weight: .heavy))
                    .foregroundColor(theme.colors.ink)
                if f.status == .inRoom {
                    Text("房间中")
                        .font(.system(size: 9, weight: .heavy))
                        .foregroundColor(theme.colors.accentDeep)
                        .padding(.vertical, 2)
                        .padding(.horizontal, 6)
                        .background(RoundedRectangle(cornerRadius: 6).fill(theme.colors.accentSoft))
                }
            }
            Text(f.statusText)
                .font(.system(size: 11, weight: .semibold))
                .foregroundColor(f.online ? theme.colors.inkSoft : theme.colors.inkSoft.opacity(0.5))
        }

        Spacer()

        // 三态按钮 switch f.status
        switch f.status {
        case .inRoom:
            Button(action: { state.onJoinFriendTap(friend: f) }) {
                HStack(spacing: 4) {
                    Image(systemName: Icons.symbol(for: "enter"))
                        .font(.system(size: 14))
                    Text("加入")
                        .font(.system(size: 12, weight: .heavy))
                }
                .padding(.vertical, 8)
                .padding(.horizontal, 14)
                .foregroundColor(.white)
                .background(RoundedRectangle(cornerRadius: 14).fill(theme.colors.accent))
            }
            .accessibilityIdentifier("friendActionButton_\(f.id)")
        case .online:
            Button(action: { state.onInviteFriendTap(friend: f) }) {
                Text("邀请")
                    .font(.system(size: 12, weight: .heavy))
                    .padding(.vertical, 8)
                    .padding(.horizontal, 14)
                    .foregroundColor(theme.colors.accentDeep)
                    .overlay(RoundedRectangle(cornerRadius: 14).stroke(theme.colors.accent, lineWidth: 1.5))
            }
            .accessibilityIdentifier("friendActionButton_\(f.id)")
        case .offline:
            Text("离线")
                .font(.system(size: 11, weight: .bold))
                .foregroundColor(theme.colors.inkSoft.opacity(0.5))
                .padding(.horizontal, 8)
        }
    }
    .padding(12)
    .background(
        RoundedRectangle(cornerRadius: 18)
            .fill(theme.colors.surface)
            .shadow(color: theme.shadow.sm.color, radius: theme.shadow.sm.radius, x: theme.shadow.sm.x, y: theme.shadow.sm.y)
    )
    .overlay(RoundedRectangle(cornerRadius: 18).stroke(theme.colors.border, lineWidth: 1))
    .accessibilityIdentifier("friendRow_\(f.id)")
}
```

#### toastOverlay（本 story 新增；ui_design 无明确视觉）

```swift
@ViewBuilder
private var toastOverlay: some View {
    if let message = state.lastToastMessage {
        Text(message)
            .font(.system(size: 13, weight: .bold))
            .foregroundColor(.white)
            .padding(.vertical, 8)
            .padding(.horizontal, 16)
            .background(
                RoundedRectangle(cornerRadius: 12)
                    .fill(Color.black.opacity(0.85))
            )
            .padding(.bottom, 120)  // 让出浮动 TabBar
            .accessibilityIdentifier("friendsToast")
    }
}
```

> **关键决策**：toast 不实装"3 秒自动消失" timer —— 让 ViewModel 保持纯数据语义；用户切 Tab / 触发其他动作隐式覆盖。如果 review 反馈要求 timer，可在 fix-review round 加（`@State private var toastTask: Task<Void, Never>?` + `.onChange(of: state.lastToastMessage) { ... }`，注意 iOS 17+ 双参签名 lesson 7）。

### EnvironmentKey 默认值的 fallback（与 Story 37.5 协调）

FriendsScaffoldView 内全部 `@Environment(\.theme) var theme` 取主题；`Environment+Theme.swift` 已落地 `defaultValue: Theme = .candy` fallback。Preview 显式 `.environment(\.theme, ThemeName.candy.theme)` 注入；Production RootView 注入 currentTheme.theme（Story 37.5 落地）。

### xcodegen regen 节奏 + project.yml 兼容性

`iphone/project.yml` 内 `targets.PetApp.sources: - PetApp` + `targets.PetAppTests.sources: - PetAppTests` 通配；新增 8 个文件全部在 `PetApp/Features/Friends/` + `PetAppTests/Features/Friends/` 下 → 自动 inclusion，**不**改 project.yml。dev 必须 `cd iphone && xcodegen generate` 重生成 `PetApp.xcodeproj`，commit pbxproj diff。

### 与 ADR-0002 §3.1 测试栈钦定的对齐

本 story 测试**仅**用 XCTest + @testable import PetApp + UITest（XCUIApplication）—— 见"测试边界"段。

### 测试 case 数量取舍（≥7 / 实装 11 / 守护 lesson 反例）

epic AC line 4816 钦定 ≥5 case；本 story 落地 11 case：

1. 切到全部 Tab → displayedFriends 含全部好友（含离线）（epic AC line 4808）
2. inRoom 好友点"加入" → onJoinFriendTap 触发 + currentRoomId 切到 friend.currentRoomId（epic AC line 4811）
3. online 好友点"邀请" + currentRoomId nil → mutate currentRoomId 占位 + toast（epic AC line 4812）
4. online 好友点"邀请" + currentRoomId 非 nil → 仅 toast，不重新创建（epic AC line 4812）
5. friends 空数组 → displayedFriends 两 Tab 都空（fallback "暂无好友在线～"路径覆盖）
6. onlineCount derived 正确（hint epic AC「{onlineCount} 位在线 · 共 {friends.count} 位」）
7. 守护：RealFriendsViewModel 构造注入 AppState 不 crash + override 不 fatalError + **Real override 必 mutate state**（**lesson 6 守护**）
8. 守护：Real init 必 seed scaffold defaults（**lesson 4 守护**）
9. 守护：currentRoomId 派生自 appState.currentRoomId（hydrate + reset 路径）（**lesson 2 守护**）
10. 守护：bind(appState:) 是同步入口（**lesson 5 守护**）
11. 守护：offline 好友 onJoinFriendTap 走 nil guard 不 crash（边界覆盖）

### a11y identifier 命名约定

本 story FriendsScaffoldView 内 inline a11y identifier 字符串（与 Story 37.7 / 37.8 / 37.9 同精神，Story 37.13 一次性归并到 `AccessibilityID.Friends`）：

| identifier | 位置 | 备注 |
|---|---|---|
| `friendsView` | FriendsScaffoldView 主容器 | 与 Story 37.3 占位 stub 字符串一致 → Tab 切换 UITest 不破 |
| `friendsAddButton` | topCard 右侧 plus 按钮 | 后续 epic 真添加好友流程时改 action |
| `friendsMyRoomCard` | myRoomCard 容器（条件渲染） | currentRoomId nil 时不存在 |
| `friendsTab_online` / `friendsTab_all` | tabBar 2 个 Button | rawValue 拼接 |
| `friendRow_<f.id>` | FriendRow 容器 | mock data id `u1`-`u8`；后续 epic 真 userId |
| `friendActionButton_<f.id>` | FriendRow 三态按钮（仅 inRoom / online） | offline 不渲染 actionButton |
| `friendsToast` | toastOverlay（条件渲染） | lastToastMessage nil 时不存在 |

### 与 Story 37.12 / 12.7 衔接的红线（关键约束）

Story 37.12（JoinRoomModal）+ Story 12.7（Create/Join UseCase）落地路径：
- 把 RealFriendsViewModel.onJoinFriendTap 的 `appState?.setCurrentRoomId(friend.currentRoomId)` 替换为 `JoinRoomUseCase.execute(roomId: friend.currentRoomId)` 调用（成功后 server 端 WS 推送或 LoadHomeUseCase 回调写 appState；本期占位代码保留）
- 把 RealFriendsViewModel.onInviteFriendTap 的 `appState?.setCurrentRoomId("1234567")` 替换为 `CreateRoomUseCase.execute()` 调用（成功后真实 roomId 由 server 返回写 appState；本期占位代码保留）
- FriendsScaffoldView 视图 zero edit
- AppState / Friends 类型契约 zero edit

> **关键决策**：本 story **不**预先加 JoinRoomUseCase / CreateRoomUseCase 接口字段 —— Story 12.7 / 37.12 实装时根据真实 UseCase shape 决定字段；预 over-design 反而让 Story 12.7 / 37.12 dev 在 mapping 路径上重写浪费工作量（参考 ADR-0010 §4.4 缓解策略）。

### Project Structure Notes

- 新建目录 `iphone/PetApp/Features/Friends/ViewModels/` + `iphone/PetApp/Features/Friends/Models/`（已有 `iphone/PetApp/Features/Friends/Views/`）
- 新建目录 `iphone/PetAppTests/Features/Friends/`
- 全部走 `iphone/project.yml` 通配 inclusion；不改 project.yml

### References

- [Source: docs/宠物互动App_总体架构设计.md] —— 总体架构与产品规则（好友 / 房间概念）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md] —— iOS 工程目录结构（Features/Friends/ViewModels|Models|Views/ 三层）
- [Source: docs/宠物互动App_V1接口设计.md §房间] —— Story 37.12 / 12.7 后接的 server 接口契约（本 story 不依赖）
- [Source: _bmad-output/planning-artifacts/epics.md §Story 37.10] —— 本 story epic AC（line 4795-4817）
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md §3.1] —— ADR-0002 测试栈钦定（XCTest only）
- [Source: _bmad-output/implementation-artifacts/decisions/0009-ios-navigation.md §3.5] —— ADR-0009 主入口 4 Tab（Friends Tab 直接路由）
- [Source: _bmad-output/implementation-artifacts/decisions/0010-app-state.md §3.1 §3.2 §3.5] —— ADR-0010 ViewModel 注入规则 + AppState 范围白名单 + state owner 边界
- [Source: _bmad-output/planning-artifacts/sprint-change-proposal-2026-04-29-v2.md §5.4] —— Friends Tab 直接路由 + epic 37 Scaffold 链路
- [Source: iphone/ui_design/source/screens/friends.jsx] —— 5 区块视觉源（line 1-128 全文）
- [Source: iphone/ui_design/README.md §FriendsScreen] —— FriendsScreen 概述（line 275）
- [Source: iphone/PetApp/Features/Wardrobe/Views/WardrobeScaffoldView.swift] —— Story 37.9 落地的 WardrobeScaffoldView，本 story 1:1 复刻 class 层次模式 + sink + Defaults seed 模式 + Real override 必 mutate state（lesson 6 守护）
- [Source: iphone/PetApp/Features/Wardrobe/ViewModels/WardrobeViewModel.swift / MockWardrobeViewModel.swift / RealWardrobeViewModel.swift / WardrobeScaffoldDefaults.swift] —— class 层次 + Mock/Real 三件套 + ScaffoldDefaults 共享 enum 参考实现
- [Source: iphone/PetApp/Features/Wardrobe/Models/CosmeticItem.swift] —— 数据模型 value type 参考实现
- [Source: iphone/PetApp/Features/Friends/Views/FriendsView.swift] —— Story 37.3 落地的占位 stub（本 story 改 body 内部）
- [Source: iphone/PetApp/Core/DesignSystem/Primitives/Avatar.swift / Card.swift / PrimaryButton.swift / Icons.swift] —— Story 37.6 落地的 primitives，本 story 复用（Avatar 含 online 小绿点；Icons.plus / Icons.paw / Icons.enter）
- [Source: iphone/PetApp/Core/DesignSystem/Theme.swift] —— Story 37.5 Theme tokens
- [Source: iphone/PetApp/App/RootView.swift / MainTabView.swift] —— RootView wire 模式 + MainTabView 4 Tab 路由
- [Source: iphone/PetApp/App/AppState.swift] —— Story 37.4 AppState 7 字段（含 currentRoomId）
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

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) — claude-opus-4-7[1m]

### Debug Log References

- `bash iphone/scripts/build.sh --test` 通过：305 unit tests / 0 failures（含本 story 新增 11 case，全部 FriendsViewScaffoldTests）
- xcodegen regen 成功（pbxproj diff 含 8 个新文件 + 1 个 PetAppTests 新文件 + UITest 修改）
- AC9 grep 校验全过：`class FriendsViewModel` = 1（去 final），fatalError = 4（≥2），Mock override = 2，Real override = 2，Real `appState?.setCurrentRoomId` = 2（守护 lesson 6），FriendsView `FriendsScaffoldView` = 2，旧 `Text("Friends Tab Placeholder")` = 0

### Completion Notes List

- Tasks 1-10 全部完成；8 个新文件 + 4 个修改文件 + pbxproj regen 全部就位
- 8 条 lesson 预防性应用全部命中：lesson 1 (RootView 用 RealFriendsViewModel) / lesson 2 (sink 派生 currentRoomId) / lesson 3 (反向应用 — currentRoomId 合法派生) / lesson 4 (Real init seed defaults) / lesson 5 (.onAppear 同步 bind) / lesson 6 (Real override 必 mutate state via appState 入口) / lesson 7 (无 .onChange) / lesson 8 (shadow 挂 RoundedRectangle.fill 那层)
- 5 区块视觉按 ui_design friends.jsx 1:1 翻译；FriendRow 三态用 switch 让漏分支编译期报错
- toastOverlay 不实装"3 秒自动消失" timer（spec 钦定）；用户切 Tab / 触发其他动作隐式覆盖
- myRoomCard **不**渲染分享按钮（spec Dev Notes "myRoomCard 分享按钮决策" 钦定）
- RealFriendsViewModel `friends` 字段未建 sink（本 story 范围内 friends 永远走 ScaffoldDefaults seed；后续 epic 真实 server 接口落地时再加 subscribeFriends sink）

### File List

新建文件：
- iphone/PetApp/Features/Friends/Models/Friend.swift
- iphone/PetApp/Features/Friends/Models/FriendStatus.swift
- iphone/PetApp/Features/Friends/Models/FriendsTab.swift
- iphone/PetApp/Features/Friends/ViewModels/FriendsViewModel.swift
- iphone/PetApp/Features/Friends/ViewModels/MockFriendsViewModel.swift
- iphone/PetApp/Features/Friends/ViewModels/RealFriendsViewModel.swift
- iphone/PetApp/Features/Friends/ViewModels/FriendsScaffoldDefaults.swift
- iphone/PetApp/Features/Friends/Views/FriendsScaffoldView.swift
- iphone/PetAppTests/Features/Friends/FriendsViewScaffoldTests.swift

修改文件：
- iphone/PetApp/Features/Friends/Views/FriendsView.swift（占位 Text → FriendsScaffoldView + @EnvironmentObject + Preview 注入 Mock）
- iphone/PetApp/App/RootView.swift（追加 friendsViewModel @StateObject + LaunchedContentView 透传 + .onAppear bind + .environmentObject 注入）
- iphone/PetApp/App/MainTabView.swift（Preview 追加 .environmentObject(MockFriendsViewModel)）
- iphone/PetAppUITests/HomeUITests.swift（追加 testFriendsScaffoldShowsAllAnchors）
- iphone/PetApp.xcodeproj/project.pbxproj（xcodegen regen）

### Change Log

- 2026-04-30: 落地 Story 37.10 FriendsView Scaffold + FriendsViewModel class 层次 + Mock/Real 两子类。8 条 lesson 预防性应用命中。305 unit tests 通过（含 11 新增 FriendsViewScaffoldTests case）。Status: in-progress → review.
