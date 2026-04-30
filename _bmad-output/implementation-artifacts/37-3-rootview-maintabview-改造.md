# Story 37.3: RootView 主入口重新实装（按 ADR-0009 完全 supersedes Story 2.3 主入口部分）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want 主入口从 3 CTA + Sheet 改为 4 Tab 浮动 TabBar，每 Tab 独立 NavigationStack,
so that App IA 与 ui_design 完全对齐.

## 故事定位（Epic 37 第二层第 1 条 story；与 Story 37.4 同层、可并行；下游 37.5–37.13 全部依赖本 story 的 MainTabView）

这是 Epic 37「iPhone 架构层重构 + UI Scaffold（壳先行）」的**第二层 story**之一（与 Story 37.4 同层，**两条可并行也可顺序**：37.3 处理导航壳 / 37.4 处理 AppState 数据持有；二者落地后才解锁第三层 Story 37.5 / 37.6 → 第四层 37.7–37.12 Scaffold 主体）。本 story 是 **架构重构类**——按 Epic 37 §AC 红线豁免 "完全 mock + 禁 import APIClient" 的 Scaffold 共性约束（红线原文："37.3/37.4 重构 story 不适用"）；本 story 的本质就是**改基础设施**，自然要触碰 RootView / AppCoordinator / HomeView 现有真实代码。

**本 story 落地后立即解锁**：
- Story 37.5 Theme & Design Tokens（依赖本 story 的 MainTabView 注入 Theme 入口）
- Story 37.6 共享 primitives（依赖本 story 的 MainTabView 接缝）
- 所有 Scaffold story（37.7–37.12）依赖本 story 的 MainTabView + HomeContainerView 已就位

**本 story 的"实装"动作**（一句话概括）：按 ADR-0009 §3.5 步骤 1–8 **从空白重新实装**主入口：删除旧 RootView `.fullScreenCover` 主入口路由 + HomeView 3 CTA 按钮 + AppCoordinator `SheetType.room/.wardrobe`；新建 MainTabView + HomeContainerView + Tab enum + 4 个 Tab 占位 View（WardrobeView / FriendsView / ProfileView / RoomView 占位）；新建 JoinRoomModal 占位 sheet 挂载点；删除 NavigationUITests 旧 3 CTA 路径 + HomeUITests 主按钮断言；UI 测试断言改为 4 Tab a11y identifier。

**关键路径："重新实装" vs "迁移"**（X1+X2 修订强约束，与 ADR-0009 §4.1 对齐）：
- 旧主入口代码（HomeView 3 CTA 按钮 / RootView fullScreenCover / AppCoordinator.SheetType `.room/.wardrobe` / NavigationUITests 旧用例）**整段直接删除**，**不**做"先标 deprecated 再渐进迁移"
- 新 MainTabView / HomeContainerView **从空白构建**（参考现有 RootView 模式但**不**复用旧组件结构）
- caller 漏改靠**编译器报错**驱动（删 SheetType case 后所有 `coordinator.present(.room)` 调用编译失败 → dev 在 build 通过前必须改完）；**不依赖 grep 兜底**
- 功能完整性靠 **UITest 全链路覆盖**兜底（启动验 ping/version/home 链路 + 4 Tab 可定位 + Tab 切换可见 + 401 cold-start）

**不涉及**（红线）：
- **不**实装真实 WardrobeView / FriendsView / ProfileView 内容（Story 37.9 / 37.10 / 37.11 各自负责，本 story 仅放占位 stub `Text("Wardrobe Tab Placeholder")`）
- **不**实装真实 RoomView 内容（Story 37.8 负责，本 story 仅放占位 stub）
- **不**实装真实 JoinRoomModal 内容（Story 37.12 负责；本 story 仅 wire `.sheet` 挂载点 + 占位 modal stub）
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3 + Story 2.2 AC9 强约束的延续）
- **不**改 server/ 任何文件
- **不**新建 AppState（Story 37.4 负责；本 story 临时用 `AppCoordinator.currentRoomId: String?` 占位字段，等 Story 37.4 落地后由 dev 把 `appState.currentRoomId` 切换 = 双方协调收口）。**或者**（**推荐**，避免临时占位）：约定本 story 与 Story 37.4 走 **顺序 commit** —— Story 37.4 先 done（AppState 类落地 + RootView 注入 `.environmentObject(appState)` 已就绪）→ 本 story 直接读 `@EnvironmentObject var appState: AppState`。Dev 实装时**优先选顺序方案**；如选并行方案需在 commit message 显式记录占位字段后续会被 Story 37.4 接管。
- **不**新建 Theme 系统（Story 37.5 负责；本 story 用 SwiftUI 原生 Color / 硬编码 ui_design 色值占位）
- **不**新建共享 primitives（Story 37.6 负责；本 story 用 SwiftUI 原生组件构建 TabBar）

## Acceptance Criteria

> **AC 编号体系**：AC1–AC5 严格映射 ADR-0009 §3.5 步骤 1–8（合并部分相邻步骤）；AC6–AC8 是**单元测试 / UITest 必含验收项 / Sprint Change Proposal v2.5 附加项**。

### AC1 — 新建 MainTabView + Tab enum + 浮动 TabBar overlay（ADR-0009 §3.5 步骤 1–2）

**新建文件 `iphone/PetApp/App/MainTabView.swift`**：

```swift
import SwiftUI

/// Tab enum：MainTabView selection binding 的 type-safe 标识。
/// CaseIterable + Identifiable 让 ForEach + a11y identifier 自动衍生。
public enum Tab: String, CaseIterable, Identifiable {
    case home, wardrobe, friends, profile

    public var id: String { rawValue }
}

public struct MainTabView: View {
    @EnvironmentObject var coordinator: AppCoordinator

    public init() {}

    public var body: some View {
        ZStack(alignment: .bottom) {
            TabView(selection: $coordinator.currentTab) {
                HomeContainerView().tag(Tab.home)
                WardrobeView().tag(Tab.wardrobe)        // Story 37.9 实装内容；本 story 占位 stub
                FriendsView().tag(Tab.friends)          // Story 37.10 实装内容；本 story 占位 stub
                ProfileView().tag(Tab.profile)          // Story 37.11 实装内容；本 story 占位 stub
            }
            // 隐藏 SwiftUI 默认 TabBar（自绘浮动 overlay）
            .toolbar(.hidden, for: .tabBar)

            // 浮动自绘 TabBar overlay（按 ui_design §iOS 设备规格）
            FloatingTabBar(selection: $coordinator.currentTab)
                .padding(.horizontal, 12)
                .padding(.bottom, 14)
        }
    }
}

/// 浮动自绘 TabBar：高 72pt + 距底 14pt + 距左右 12pt + Card 圆角 + theme.shadow.md 占位.
/// Story 37.5 落地后 Color 改用 theme.colors / shadow 改用 theme.shadow.
private struct FloatingTabBar: View {
    @Binding var selection: Tab

    var body: some View {
        HStack(spacing: 0) {
            ForEach(Tab.allCases) { tab in
                tabButton(tab)
            }
        }
        .frame(height: 72)
        .background(Color(.systemBackground))
        .cornerRadius(20)
        .shadow(color: Color.black.opacity(0.14), radius: 16, x: 0, y: 6)
    }

    private func tabButton(_ tab: Tab) -> some View {
        Button(action: { selection = tab }) {
            VStack(spacing: 4) {
                Image(systemName: iconName(for: tab))
                    .font(.system(size: 22))
                    .scaleEffect(selection == tab ? 1.1 : 1.0)
                Text(label(for: tab))
                    .font(.caption2)
            }
            .frame(maxWidth: .infinity)
            .foregroundColor(selection == tab ? .accentColor : .secondary)
        }
        .accessibilityIdentifier("tab_\(tab.rawValue)")
    }

    private func iconName(for tab: Tab) -> String {
        switch tab {
        case .home: return "house.fill"
        case .wardrobe: return "shippingbox.fill"
        case .friends: return "person.2.fill"
        case .profile: return "person.crop.circle.fill"
        }
    }

    private func label(for tab: Tab) -> String {
        switch tab {
        case .home: return "家"
        case .wardrobe: return "仓库"
        case .friends: return "好友"
        case .profile: return "我的"
        }
    }
}
```

**关键约束**：
- `Tab` enum 必须在 `MainTabView.swift` 内定义（不放进 `AppCoordinator.swift`，让 coordinator 仅依赖 Tab 类型而不拥有它）；后续 Story 37.5 抽 design system 时 Tab 可平移到 `Core/DesignSystem/Navigation/Tab.swift`
- 4 个 a11y identifier 必须用 `tab_\(tab.rawValue)` 模板，最终值 = `tab_home` / `tab_wardrobe` / `tab_friends` / `tab_profile`（Story 37.13 a11y 总表会归并到 `AccessibilityID.Tab` 命名空间，但本 story 先就地写 inline 字符串；**不**急着加 `AccessibilityID.Tab` enum——见 Dev Notes "AccessibilityID 演化" 段）
- TabBar 视觉数值（72pt 高 / 14pt 距底 / 12pt 距左右 / 圆角 20）按 ui_design `README.md` §iOS 设备规格硬编码；shadow / accent 色值可用 SwiftUI 原生 Color 占位，Story 37.5 接 theme 后由 dev 改为 token

### AC2 — 新建 HomeContainerView + 4 Tab 占位 View（ADR-0009 §3.5 步骤 3 + 6）

**新建文件 `iphone/PetApp/Features/Home/Views/HomeContainerView.swift`**：

根据 `appState.currentRoomId` 在 HomeView ↔ RoomView 互斥切换（淡入淡出 0.3s）：

```swift
import SwiftUI

public struct HomeContainerView: View {
    @EnvironmentObject var appState: AppState   // Story 37.4 落地后注入；并行方案下临时改 @EnvironmentObject AppCoordinator + appCoordinator.currentRoomId
    @ObservedObject var homeViewModel: HomeViewModel  // 由 RootView 通过 .environmentObject 或构造注入

    public var body: some View {
        ZStack {
            if appState.currentRoomId == nil {
                // idle 态：显示 HomeView（Story 5.5 既有 HomeView 实例不动，本 story 仅删除其 3 CTA 按钮）
                NavigationStack {
                    HomeView(
                        viewModel: homeViewModel,
                        resetIdentityViewModel: nil,           // Debug build 时由 RootView 注入；本 story 不动 Debug 路径
                        sessionStore: nil
                    )
                }
                .transition(.opacity)
            } else {
                // inRoom 态：显示 RoomView 占位 stub（Story 37.8 实装真实内容）
                RoomViewPlaceholder()
                    .transition(.opacity)
            }
        }
        .animation(.easeInOut(duration: 0.3), value: appState.currentRoomId)
    }
}
```

**新建占位 View（同文件或拆 4 个新文件均可，dev 自决）**：
- `iphone/PetApp/Features/Wardrobe/Views/WardrobeView.swift` —— `Text("Wardrobe Tab Placeholder").accessibilityIdentifier("wardrobeView")` + `NavigationStack` 包一层（Story 37.9 落地真实内容）
- `iphone/PetApp/Features/Friends/Views/FriendsView.swift` —— 同模式 `Text("Friends Tab Placeholder").accessibilityIdentifier("friendsView")`（Story 37.10）
- `iphone/PetApp/Features/Profile/Views/ProfileView.swift` —— 同模式 `Text("Profile Tab Placeholder").accessibilityIdentifier("profileView")`（Story 37.11）
- `iphone/PetApp/Features/Room/Views/RoomViewPlaceholder.swift` —— 同模式 `Text("Room Placeholder")`（Story 37.8）；**注意**：`RoomViewPlaceholder` 与现有 `iphone/PetApp/Features/Home/Views/SheetPlaceholders/RoomPlaceholderView.swift`（Story 2.3 落地）**不是**同一个 —— Story 2.3 的 `RoomPlaceholderView` 是 `.fullScreenCover` 内容（带关闭按钮 + sheetPlaceholder a11y），本 story 删 fullScreenCover 后该文件**整体删除**；新 `RoomViewPlaceholder` 是 HomeContainerView 内嵌内容（无关闭按钮，靠 leaveRoom 把 currentRoomId 改 nil 退出）

**目录结构**（按 ADR-0002 §3.3 工程目录方案 `iphone/PetApp/Features/{Feature}/Views/`）：

```
iphone/PetApp/
├─ App/
│  ├─ MainTabView.swift                       (新增；含 Tab enum + FloatingTabBar private 子视图)
│  ├─ AppCoordinator.swift                    (修改；删 .room / .wardrobe case + 加 currentTab)
│  ├─ RootView.swift                          (修改；删 fullScreenCover 主入口路由)
│  └─ ...（其余不动）
├─ Features/
│  ├─ Home/Views/
│  │  ├─ HomeContainerView.swift              (新增)
│  │  ├─ HomeView.swift                       (修改；删 bottomButtonRow 3 CTA + onRoomTap/onInventoryTap/onComposeTap closure 字段)
│  │  ├─ JoinRoomModal/
│  │  │  └─ JoinRoomModalPlaceholder.swift    (新增；占位 sheet 内容；Story 37.12 落地真实 modal)
│  │  └─ SheetPlaceholders/                   (整目录删除：RoomPlaceholderView / InventoryPlaceholderView / ComposePlaceholderView)
│  ├─ Wardrobe/Views/WardrobeView.swift       (新增；占位)
│  ├─ Friends/Views/FriendsView.swift         (新增；占位)
│  ├─ Profile/Views/ProfileView.swift         (新增；占位)
│  └─ Room/Views/RoomViewPlaceholder.swift    (新增；占位)
```

### AC3 — 修改 RootView：删 fullScreenCover 主入口路由 + 注入 MainTabView（ADR-0009 §3.5 步骤 1）

**修改 `iphone/PetApp/App/RootView.swift`**：

- **删除** `LaunchedContentView.body` 内 `.fullScreenCover(item: $coordinator.presentedSheet)` modifier 及 sheetContent 闭包参数
- **删除** `RootView.sheetContent(for: SheetType)` 私有方法（连同 `RoomPlaceholderView` / `InventoryPlaceholderView` / `ComposePlaceholderView` import 链一起清理）
- **删除** `RootView.wireHomeViewModelClosures()` 方法（不再有 onRoomTap / onInventoryTap / onComposeTap 闭包要 wire；HomeView 不再有这些字段）
- **新增** `RootView` 渲染入口：`.ready` 分支不再渲染 `homeView`（旧 HomeView 直接当根），改渲染 `MainTabView()`，并在外层注入 `coordinator: AppCoordinator` 到 environment
- **保留** `launchStateMachine` 三态机（launching / needsAuth / ready）—— launching 阶段 MainTabView **不渲染**（仍是 LaunchingView 占满全屏）；needsAuth 阶段也 **不渲染** MainTabView（仍是 RetryView / TerminalErrorView 占满全屏）；只有 `.ready` 才渲染 MainTabView
- **保留** ping bind / loadHome bind / errorPresentationHost 全部既有 wire（关键 UITest 验收项 = 启动后 ping/version/home 链路在新 RootView 内仍生效；见 AC7）
- **保留** Debug build 内 `KeychainUITestHookView` + `resetIdentityViewModel` 注入（不变）

**关键 wire 改动模板**（参考 RootView 现有 `LaunchedContentView` 子视图模式）：

```swift
// 旧 .ready 分支:
case .ready:
    homeView()
        .onAppear { onReadyAppear() }
        .task { await onReadyTask() }
        .fullScreenCover(item: $coordinator.presentedSheet) { sheet in
            sheetContent(sheet)
        }
        .transition(.opacity)

// 新 .ready 分支:
case .ready:
    MainTabView()
        .environmentObject(coordinator)
        // appState 注入：Story 37.4 顺序方案 → 这里加 .environmentObject(appState)
        // 并行方案 → 暂时不加；MainTabView 内 HomeContainerView 用 AppCoordinator.currentRoomId 占位
        .onAppear { onReadyAppear() }
        .task { await onReadyTask() }
        .transition(.opacity)
```

**onReadyTask 内 ping/loadHome wire 不动**（Story 5.5 round 6 [P2] fix 已固化的 wire 模式）：
```swift
homeViewModel.bind(pingUseCase: container.makePingUseCase())
await homeViewModel.start()
```

**onReadyAppear 内 wireHomeViewModelClosures() 调用删除**（HomeView 不再有 closure 字段）；`resetIdentityViewModel` 注入路径保留。

### AC4 — 修改 HomeView：删 3 CTA 按钮 + closure 字段 + 加 JoinRoomModal sheet 挂载点（ADR-0009 §3.5 步骤 4 + 7）

**修改 `iphone/PetApp/Features/Home/Views/HomeView.swift`**：

- **删除** `bottomButtonRow` 私有 var（包含"进入房间" / "仓库" / "合成" 三个 Button + 各自 a11y identifier）
- **删除** `body` 内 `bottomButtonRow` 调用（VStack 子视图减一行）
- **保留** versionFooter + ping/version 角落显示（**关键 UITest 验收项**：见 AC7 第 4 条）
- **保留** userInfoBar / petAndChestRow / stepBalanceLabel / versionFooter 四区块（Story 5.5 锁定的 UI 数据投影完全不动）
- **保留** Story 5.5 codex round 1 [P2] fix 的三态文案（loadingPlaceholder / noPetPlaceholder / pet.name）+ Story 5.2 round 1 [P1] fix 的 SessionAwareUserInfoBar 子视图

**修改 `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`**：

- **删除** `onRoomTap` / `onInventoryTap` / `onComposeTap` 三个 closure 字段（init 默认参数也删除）
- **删除** 老 init / Story 2.5 init / Story 5.5 init 内 onRoomTap / onInventoryTap / onComposeTap 三个 closure 参数
- **保留** 其余字段（nickname / appVersion / serverInfo / homeData / loadingState / pingUseCase / loadHomeUseCase / errorPresenter / bind / start / loadHome / applyHomeData / applyHomeError / resetLoadHomeForRetry 全部 Story 2.5 / 5.5 钦定方法）

**新建 JoinRoomModal sheet 挂载点**：在 HomeView body 内**加** `.sheet(isPresented: $homeViewModel.showJoinModal) { JoinRoomModalPlaceholder() }`：

> **注意**：HomeViewModel 此前**没有** `showJoinModal: Bool` 字段（这是本 story 落地的 Story 37.12 接缝准备）；Story 37.7 落地后 `showJoinModal` 会被改造成 MockHomeViewModel / RealHomeViewModel 子类的 @Published。本 story 暂在现有 HomeViewModel 加 `@Published var showJoinModal: Bool = false` 字段（**不**触发"创建队伍" / "加入队伍"按钮——那是 Story 37.7 TeamIdleCard 实装的事）。**或者**（**推荐**）：本 story 跳过加 `showJoinModal` 字段，仅写**注释**「`// Story 37.12 will mount JoinRoomModal sheet here via HomeViewModel.showJoinModal`」+ `// .sheet(...)` 占位注释行；不实际 wire。Dev 实装时**优先选注释占位**，避免 Story 37.7 改造时再删字段（风险低）。

**关键约束**：
- HomeView 删 3 CTA 后旧 HomeView_Previews `HomeView(viewModel: HomeViewModel())` 调用仍有效（HomeViewModel 老 init 存在）；如有 onXTap 参数残留 dev 自行清理
- HomeView 仍可独立预览 / 单元测试（Story 5.5 既有 HomeViewTests / HomePetNameResolverTests / HomeNicknameResolverTests 三套测试不能 regress；如必要的 case 会因为删按钮失败 → 同步删该 case）

### AC5 — 修改 AppCoordinator：删 .room / .wardrobe case + 加 currentTab + switchTab 方法（ADR-0009 §3.4 + §3.5 步骤 5）

**修改 `iphone/PetApp/App/AppCoordinator.swift`**：

```swift
import Foundation
import Combine
import SwiftUI

/// Sheet 路由枚举：缩窄到次级 sheet（不再含主入口）.
public enum SheetType: Identifiable, Equatable {
    case compose       // 保留：合成 sub-flow（Story 33.1 决定具体形式）
    // case room       // 删除（Home Tab 互斥状态机接管）
    // case inventory  // 删除（Wardrobe Tab 直接路由）
    // 注：原 v1 SheetType case 名为 .inventory；ADR-0009 §3.5 步骤 5 写 .wardrobe，本 story
    // 删的是同一个枚举值（旧名 .inventory，与 .room 同批删除）.

    public var id: String {
        switch self {
        case .compose: return "sheet_compose"
        }
    }
}

/// AppCoordinator 角色变化（ADR-0009 §3.4）：
/// - 旧职责：主入口 sheet 路由（.room / .inventory / .compose）
/// - 新职责：
///   1. presentedSheet 仅含次级 sheet（.compose）
///   2. currentTab @Published：TabView selection 的 single source of truth
///   3. switchTab 方法：程式化切 Tab 入口
@MainActor
public final class AppCoordinator: ObservableObject {
    @Published public var presentedSheet: SheetType?
    @Published public var currentTab: Tab = .home

    public init(presentedSheet: SheetType? = nil, currentTab: Tab = .home) {
        self.presentedSheet = presentedSheet
        self.currentTab = currentTab
    }

    public func present(_ sheet: SheetType) {
        presentedSheet = sheet
    }

    public func dismiss() {
        presentedSheet = nil
    }

    /// 程式化切换 Tab（如深 link、跨 ViewModel 跳转）.
    public func switchTab(_ tab: Tab) {
        currentTab = tab
    }
}
```

**关键约束**：
- 删 `.room` / `.inventory` case 后，旧 caller `coordinator.present(.room)` 等代码**编译失败** → dev 在 build 通过前必须改完全部 caller（这是 X1+X2 修订核心策略，**不**走 grep 兜底）
- 现有 `iphone/PetAppTests/App/SheetTypeTests.swift` 大概率会失败（断言 `.room.id == "sheet_room"` 等）→ 同步改测试，删除 `.room` / `.inventory` 用例，保留 `.compose`
- 现有 `iphone/PetAppTests/App/AppCoordinatorTests.swift` 中 `present(.room) → presentedSheet == .room` 等用例失败 → 同步改：删 `.room` / `.inventory`，新增 `currentTab` / `switchTab` 用例（与 AC6 第 3 条对齐）

### AC6 — 单元测试覆盖（≥5 case，纯 XCTest；Sprint Change Proposal v2.5 §AC 红线第 5 条钦定）

**新建测试文件 `iphone/PetAppTests/App/MainTabViewIntegrationTests.swift`**（或归并到 `AppCoordinatorTests.swift` + 新建 `HomeContainerViewTests.swift`，dev 自决；下文按"两文件分立"模板）：

```swift
import XCTest
@testable import PetApp

@MainActor
final class HomeContainerViewTests: XCTestCase {

    // happy: appState.currentRoomId = nil → HomeContainerView 显示 HomeView
    // 注：SwiftUI View 不能直接断言 "渲染了什么"；该测试用 ViewInspector 模式不可行（ADR-0002 §3.1 禁用 ViewInspector）;
    // 落地路径：测 HomeContainerView 内部派生的状态字段（如 isInRoom: Bool computed property），
    // 或者通过 SwiftUI Snapshot 渲染层级断言（ADR-0002 §3.1 也禁用 SnapshotTesting）.
    // 本 story 推荐路径：把 isInRoom 决策逻辑抽成 HomeRoomDispatcher.shouldShowRoom(currentRoomId:) -> Bool
    // 纯函数 helper（与 HomePetNameResolver 同精神），用 XCTest 直接覆盖三态.
    func testIdleState_ShouldShowHomeView() async {
        XCTAssertFalse(HomeRoomDispatcher.shouldShowRoom(currentRoomId: nil))
    }

    // happy: appState.currentRoomId = "room_1234567" → HomeContainerView 显示 RoomView
    func testInRoomState_ShouldShowRoomView() async {
        XCTAssertTrue(HomeRoomDispatcher.shouldShowRoom(currentRoomId: "room_1234567"))
    }

    // edge: currentRoomId 从 "room_1234567" 切到 nil → shouldShowRoom 返回 false（过渡动画由 SwiftUI .animation 自动接管，不在单测验证）
    func testTransitionFromInRoomToIdle_ShouldShowHomeView() async {
        XCTAssertTrue(HomeRoomDispatcher.shouldShowRoom(currentRoomId: "room_1234567"))
        XCTAssertFalse(HomeRoomDispatcher.shouldShowRoom(currentRoomId: nil))
    }
}

@MainActor
final class AppCoordinatorTabTests: XCTestCase {

    // happy: coordinator.switchTab(.wardrobe) → coordinator.currentTab == .wardrobe
    func testSwitchTab_UpdatesCurrentTab() {
        let coordinator = AppCoordinator()
        XCTAssertEqual(coordinator.currentTab, .home)   // default

        coordinator.switchTab(.wardrobe)
        XCTAssertEqual(coordinator.currentTab, .wardrobe)

        coordinator.switchTab(.profile)
        XCTAssertEqual(coordinator.currentTab, .profile)
    }

    // happy: 程式化切 Tab 不影响 presentedSheet（次级 sheet 与 Tab 互不干扰）
    func testSwitchTab_DoesNotAffectPresentedSheet() {
        let coordinator = AppCoordinator()
        coordinator.present(.compose)
        XCTAssertEqual(coordinator.presentedSheet, .compose)

        coordinator.switchTab(.wardrobe)
        XCTAssertEqual(coordinator.presentedSheet, .compose)   // 不变
        XCTAssertEqual(coordinator.currentTab, .wardrobe)
    }
}
```

**纯函数 helper**（落地为 `iphone/PetApp/Features/Home/Views/HomeContainerView.swift` 内的公开 enum）：

```swift
/// HomeContainerView 互斥状态机的决策 helper（与 HomePetNameResolver 同精神：抽纯函数让单测直接覆盖）.
public enum HomeRoomDispatcher {
    /// - Parameter currentRoomId: 来自 AppState.currentRoomId（或并行方案下 AppCoordinator.currentRoomId）
    /// - Returns: true → 显示 RoomView（inRoom 态）；false → 显示 HomeView（idle 态）
    public static func shouldShowRoom(currentRoomId: String?) -> Bool {
        currentRoomId != nil
    }
}
```

**SheetType 测试改写**（修改 `iphone/PetAppTests/App/SheetTypeTests.swift`）：
- 删 `.room.id == "sheet_room"` / `.inventory.id == "sheet_inventory"` 用例
- 保留 `.compose.id == "sheet_compose"`
- 删 `XCTAssertNotEqual(SheetType.room, .inventory)` 这类两两不等用例（如有）

**AppCoordinator 测试改写**（修改 `iphone/PetAppTests/App/AppCoordinatorTests.swift`）：
- 删 `present(.room) → presentedSheet == .room` / `present(.inventory) → ...` 用例
- 保留 `present(.compose) → presentedSheet == .compose` / `dismiss() → presentedSheet == nil` 用例
- 加 AppCoordinatorTabTests 中的 currentTab / switchTab 用例（也可归并到 AppCoordinatorTests.swift）

**最终测试 case 总数 ≥5**（HomeContainerViewTests 3 case + AppCoordinatorTabTests 2 case）；**不**强制 happy + edge 配对，但每 case 必须独立可跑.

### AC7 — UI 测试覆盖（功能完整性硬保证；Sprint Change Proposal v2.5 §6 风险闭环 #7）

**改写 `iphone/PetAppUITests/NavigationUITests.swift`**：
- **删除** `testTapRoomButton_PresentsRoomSheet` / `testTapInventoryButton_PresentsInventorySheet` / `testTapComposeButton_PresentsComposeSheet` 三个旧用例
- **新增** `testFourTabsAreLocatable` 用例：启动 → app.tabBars 或 app.buttons 定位 4 个 a11y identifier `tab_home` / `tab_wardrobe` / `tab_friends` / `tab_profile` 全存在
- **新增** `testSwitchToWardrobeTab_ShowsWardrobeView` 用例：启动 → tap `tab_wardrobe` → 验证 `wardrobeView` a11y identifier 出现
- **新增** `testSwitchToFriendsTab_ShowsFriendsView` 用例：同模式
- **新增** `testSwitchToProfileTab_ShowsProfileView` 用例：同模式

**改写 `iphone/PetAppUITests/HomeUITests.swift`**：
- **删除** `testHomeViewShowsAllSixPlaceholders` 内对 btnRoom / btnInventory / btnCompose 的断言（保留对 userInfo / petArea / chestArea / stepBalance / versionLabel / petName / chestRemaining 的断言；3 个 CTA 按钮 ID 已不存在）
- **保留** `testVersionFooter_ShowsAppVersionAndServerInfo` 用例（**关键 UITest 验收项**：版本号角落显示链路在新 RootView 内仍生效；见 AC7 第 4 条）

**关键 UITest 验收项**（Sprint Change Proposal v2.5 §6 #7 闭环；以下 5 条是 Story 37.3 done 的**硬条件**，缺一不可）：

1. **启动 → mock server 收到 1 次 `/ping` + 1 次 `/version` + 1 次 `/home`**（Story 2.5 + Story 5.5 已 done 的链路在 Story 37.3 重新实装的 RootView 内仍生效）。落地路径：`MockServer` 已支持记录请求（参考现有 NavigationUITests 中 `XCUIApplication.launchEnvironment` mock server hook）；新加用例断言 mock server `request_count("/ping") == 1` / `request_count("/version") == 1` / `request_count("/home") == 1`。**如果**当前 PetAppUITests 不带 mock server 抓取能力 → dev 实装时新加 hook 或在 commit message 内显式记录"hook 待 Story 37.13 落地，暂以 manual 验证替代"
2. **4 Tab 可定位**（`tab_home` / `tab_wardrobe` / `tab_friends` / `tab_profile` a11y identifier 全在）—— 见 NavigationUITests.testFourTabsAreLocatable
3. **切到 Wardrobe Tab 验证 WardrobeView 出现**（`wardrobeView` 可定位）—— 见 NavigationUITests.testSwitchToWardrobeTab_ShowsWardrobeView
4. **ping/version 角落显示仍在 Home Tab 可见**（`home_versionLabel` 在启动后可定位且 text 含 "v" 前缀 + 非 "----" 占位）—— HomeUITests.testVersionFooter_ShowsAppVersionAndServerInfo 保留即可
5. **错误链路（401 → AuthBoundary cold-start）触发 → needsAuth 三态机正确**：本条**可选**（dev 自决）；现有 `iphone/PetAppTests/App/AppLaunchStateMachineTests.swift` + `iphone/PetAppTests/Core/Networking/AuthBoundaryAPIClientTests.swift` 已覆盖 401 cold-start 单元路径；UITest 端用 `UITEST_SKIP_GUEST_LOGIN=1` launchEnvironment hook 已绕开真实 401，本 story 不强制再加 UITest 端的 401 模拟。**如**dev 实装时发现 needsAuth 路径有 regress（如 LaunchedContentView 内 .needsAuth 分支因为 .ready → MainTabView 改造误删）→ 必须修。

**accessibility identifier 全表更新**：
- 新加 `tab_home` / `tab_wardrobe` / `tab_friends` / `tab_profile` —— 本 story 直接 inline 字符串（**不**急加 `AccessibilityID.Tab` enum；见 Dev Notes "AccessibilityID 演化" 段；Story 37.13 a11y 总表 story 会归并）
- 新加 `wardrobeView` / `friendsView` / `profileView`（占位 view 的 a11y）—— 同上 inline
- 旧 `home_btnRoom` / `home_btnInventory` / `home_btnCompose` 因 UI 元素已删除自然失效 —— `AccessibilityID.Home.btnRoom` / `btnInventory` / `btnCompose` 三个常量本 story **保留**（仅注释 `// deprecated by Story 37.3, will be cleaned by Story 37.13`），避免 `AccessibilityID.Home` enum 被改触发其它 import 漂移
- 旧 `sheetPlaceholder_room` / `sheetPlaceholder_inventory` / `sheetPlaceholder_compose` 整个 `AccessibilityID.SheetPlaceholder` enum **整段删除**（关联文件 SheetPlaceholders/ 目录已删，常量没有引用）

### AC8 — 不引入 Theme / AppState / primitives 真实实装（Epic 37 接缝设计 + 红线豁免）

本 story 是 **架构重构类**（Epic 37 §AC 红线豁免：「数据完全 mock + 禁 import APIClient」对 37.3/37.4 不适用），但仍受以下子约束：

- **不**新建 `iphone/PetApp/Core/DesignSystem/Theme*.swift` 任何 Theme 类型（Story 37.5 负责；本 story TabBar 用 SwiftUI `Color(.systemBackground)` / `Color.accentColor` / `Color.secondary` 占位，shadow 用 `Color.black.opacity(0.14)` 硬编码占位）
- **不**新建 `iphone/PetApp/App/AppState.swift`（Story 37.4 负责；本 story 的 HomeContainerView 用 `@EnvironmentObject var appState: AppState` 引用一个**未来**的类，**或者**走临时占位字段路径——见上文"故事定位"段的"或者"分支说明）
- **不**新建 `iphone/PetApp/Core/DesignSystem/Primitives/*.swift` 任何 primitives（Story 37.6 负责；本 story TabBar 内的 button 视觉用 SwiftUI 原生 Button 占位）
- **不**触碰 `_bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md`（Story 37.1 已 Accepted，**不**做 ADR §3 修订 patch；如发现 §3.5 步骤 1–8 与本 story 实装有偏差 → 走「ADR 修订 patch + 改 v2 Accepted」路径，与 Story 37.1 同 commit 模式落地，但**不**在本 story 范围内）

**与 Story 37.4 协调收口**（顺序方案 vs 并行方案）：

| 维度 | 顺序方案（推荐） | 并行方案 |
|---|---|---|
| 落地顺序 | Story 37.4 先 done → 本 story 直接读 `@EnvironmentObject var appState: AppState` | 本 story 与 Story 37.4 同时进行 |
| HomeContainerView appState 来源 | `@EnvironmentObject var appState: AppState`（真实 AppState 类，由 Story 37.4 RootView 注入） | 临时用 `@EnvironmentObject var coordinator: AppCoordinator` + `coordinator.currentRoomId: String?` 占位字段（本 story 在 AppCoordinator 内加该字段，Story 37.4 落地时 dev 改为 appState 引用 + 删除占位） |
| commit 干净度 | ✅ 一次到位，无临时占位 | ⚠️ 本 story commit 含临时 currentRoomId 字段，Story 37.4 commit 删除（commit history 噪声） |
| 风险 | Story 37.4 先 done 是确定路径（与 Epic 37 第二层并行原则相符；37.4 自身不依赖 37.3） | 本 story commit 引入 AppCoordinator.currentRoomId 占位字段，Story 37.4 落地时如约定不到位会双方都加 currentRoomId 字段 / 双方都不加 |

**Dev 实装时优先选顺序方案**（37.4 先 done → 本 story 接 AppState）。如必须并行，commit message 显式记录 "AppCoordinator.currentRoomId: String? 是 Story 37.3 ↔ 37.4 顺序协调失败的临时占位，Story 37.4 落地时由 dev 删除"。

### AC9 — Deliverable：单一 commit（含 sprint-status.yaml 状态翻转）

提交一个 commit（或 dev 自决拆 2–3 个逻辑 commit），**含**：

- 新增文件：MainTabView.swift / HomeContainerView.swift / WardrobeView.swift / FriendsView.swift / ProfileView.swift / RoomViewPlaceholder.swift / JoinRoomModalPlaceholder.swift / MainTabViewIntegrationTests.swift（或 HomeContainerViewTests.swift + AppCoordinatorTabTests 并入既有 AppCoordinatorTests.swift）
- 修改文件：RootView.swift / AppCoordinator.swift / HomeView.swift / HomeViewModel.swift / SheetTypeTests.swift / AppCoordinatorTests.swift / NavigationUITests.swift / HomeUITests.swift / AccessibilityID.swift（删 SheetPlaceholder enum）
- 删除文件：iphone/PetApp/Features/Home/Views/SheetPlaceholders/RoomPlaceholderView.swift / InventoryPlaceholderView.swift / ComposePlaceholderView.swift（整目录删除）
- 修改 `_bmad-output/implementation-artifacts/sprint-status.yaml`：`37-3-rootview-maintabview-改造: ready-for-dev → review`（同样由 workflow 自动改）
- 修改 `_bmad-output/implementation-artifacts/37-3-rootview-maintabview-改造.md`（本 story 文件；dev agent record 区块 + Status: ready-for-dev → review 由 dev-story workflow 自动改）

**xcodegen 配置同步**：本 story 涉及多个新 Swift 文件 + 文件移动 / 删除。如 `iphone/project.yml` 中 `sources` 段为 explicit list（不是 glob `**/*.swift`），dev 必须**手动同步**新文件 + 删除条目，并跑 `xcodegen` 重新生成 `Cat.xcodeproj`。如 sources 是 glob → 只跑 `xcodegen` 即可。

**commit message 建议格式**（参考 Story 37.2 模板 + ADR-0009 §3.5 落地依据）：

```
feat(iphone): MainTabView 重构主入口（Story 37.3 done; ADR-0009 §3.5）

- 新建 MainTabView + Tab enum + 浮动 TabBar overlay（4 a11y identifier）
- 新建 HomeContainerView 互斥状态机（HomeRoomDispatcher 纯函数 helper）
- 新建 4 Tab 占位 View + RoomViewPlaceholder + JoinRoomModalPlaceholder
- 删除 RootView .fullScreenCover 主入口路由 + AppCoordinator.SheetType .room/.inventory case
- 删除 HomeView 3 CTA 按钮 + HomeViewModel onRoomTap/onInventoryTap/onComposeTap closure 字段
- 删除 SheetPlaceholders/ 目录（RoomPlaceholderView / InventoryPlaceholderView / ComposePlaceholderView）
- 改写 NavigationUITests / HomeUITests 4 Tab 路径（5 case ≥）
- 改写 AppCoordinatorTests / SheetTypeTests（删 .room/.inventory 用例 + 加 currentTab/switchTab 用例）
- AppCoordinator 加 currentTab: Tab + switchTab(_:) 方法（ADR-0009 §3.4）

Refs Story 37.3; ADR-0009 §3.5 步骤 1-8; unblocks Story 37.5 / 37.6 / 37.7-37.12.
```

## Tasks / Subtasks

- [x] **Task 1：Pre-flight check + Story 37.4 协调决议**（AC8）
  - [x] 确认 ADR-0009 当前是 Accepted（Story 37.1 已 done；本 story 严格按 §3.5 步骤 1–8 实装，**不**做 ADR 修订）
  - [x] 确认 sprint-status.yaml 内 `2-3-...` 状态为 superseded、`5-5-...` 状态为 superseded、`37-1-...` / `37-2-...` 为 done、`37-3-...` 为 ready-for-dev（本 story 自身）
  - [x] **关键决议**：Story 37.4 当前 backlog → 本 story 走**并行方案**（在 AppCoordinator 加 `currentRoomId: String?` 临时占位字段）；记录到 Completion Notes
  - [x] 阅读 ui_design `README.md` §iOS 设备规格（72pt 高 / 14pt 距底 / 12pt 距左右 / 圆角 20）+ §App 结构 4 Tab IA + Tab 视觉规格
  - [x] 阅读 ADR-0009 §3.4（AppCoordinator 角色变化） + §3.5（RootView 改造步骤 1–8）

- [x] **Task 2：新建 MainTabView + AppTab enum + FloatingTabBar**（AC1）
  - [x] 新建 `iphone/PetApp/App/MainTabView.swift`：含 `AppTab enum (CaseIterable, Identifiable)` + `MainTabView` public struct + `FloatingTabBar` private struct（按 AC1 模板，**enum 命名 AppTab** 防 SwiftUI 内置 Tab 命名冲突；见 Dev Notes "Tab enum 类型放置 + 命名空间策略"）
  - [x] 4 个 a11y identifier `tab_\(tab.rawValue)` 写 inline 字符串（不加 `AccessibilityID.Tab`）
  - [x] TabBar 视觉数值 72pt / 14pt / 12pt / 圆角 20 / shadow 占位（按 ui_design 硬编码；Story 37.5 接 theme 后改 token）
  - [x] `@EnvironmentObject var coordinator: AppCoordinator` 注入；`TabView(selection: $coordinator.currentTab)` 双向绑定
  - [x] 用 `.safeAreaInset(edge: .bottom) { FloatingTabBar(...) }` 让 SwiftUI 自动处理 safe area（Dev Notes 钦定路径）
  - [x] `#if DEBUG` PreviewProvider（用 `AppCoordinator()` 注入；并行方案不需要 AppState）

- [x] **Task 3：新建 HomeContainerView + HomeRoomDispatcher helper**（AC2 + AC6 第 1 条）
  - [x] 新建 `iphone/PetApp/Features/Home/Views/HomeContainerView.swift`：并行方案 → `@EnvironmentObject var coordinator: AppCoordinator` + `coordinator.currentRoomId`
  - [x] 新建 `HomeRoomDispatcher.shouldShowRoom(currentRoomId:)` 纯函数 helper（HomeContainerView.swift 内 public enum 形式）
  - [x] HomeContainerView body：互斥 `if HomeRoomDispatcher.shouldShowRoom(...) { RoomViewPlaceholder() } else { NavigationStack { HomeContainerHomeViewBridge() } }` + `.transition(.opacity)` + `.animation(.easeInOut(duration: 0.3), value: coordinator.currentRoomId)`
  - [x] HomeView 依赖通过 environment 注入：`HomeContainerHomeViewBridge` 子视图读 `@EnvironmentObject HomeViewModel` + `@Environment(\.resetIdentityViewModel)` + `@Environment(\.sessionStore)` 透传给 HomeView（避免 init 参数透传穿过 TabView 容器；EnvironmentValues 自定义 key 在同文件内定义）

- [x] **Task 4：新建 4 Tab 占位 View + RoomViewPlaceholder + JoinRoomModalPlaceholder**（AC2）
  - [x] 新建 `iphone/PetApp/Features/Wardrobe/Views/WardrobeView.swift`：`NavigationStack { Text("Wardrobe Tab Placeholder").accessibilityIdentifier("wardrobeView") }`
  - [x] 新建 `iphone/PetApp/Features/Friends/Views/FriendsView.swift`：同模式 + `friendsView` a11y
  - [x] 新建 `iphone/PetApp/Features/Profile/Views/ProfileView.swift`：同模式 + `profileView` a11y
  - [x] 新建 `iphone/PetApp/Features/Room/Views/RoomViewPlaceholder.swift`：`Text("Room Placeholder").accessibilityIdentifier("roomViewPlaceholder")`
  - [x] 新建 `iphone/PetApp/Features/Home/Views/JoinRoomModal/JoinRoomModalPlaceholder.swift`：`Text(...).accessibilityIdentifier("joinRoomModalPlaceholder")`

- [x] **Task 5：修改 AppCoordinator**（AC5）
  - [x] 删 `SheetType.room` / `SheetType.inventory` enum case + 各自 `id` switch case
  - [x] 加 `@Published var currentTab: AppTab = .home` + `func switchTab(_ tab: AppTab)` 方法
  - [x] 加 `@Published var currentRoomId: String?` 临时占位字段（Story 37.4 协调）
  - [x] 修改 init：加 `currentTab: AppTab = .home` 与 `currentRoomId: String? = nil` 默认参数
  - [x] 头部注释更新

- [x] **Task 6：修改 RootView**（AC3）
  - [x] 删 `LaunchedContentView.body` 内 `.fullScreenCover(item: $coordinator.presentedSheet)` modifier + sheetContent 闭包参数 + sheetContent 闭包传入参数
  - [x] 删 `RootView.sheetContent(for: SheetType)` 私有方法
  - [x] 删 `RootView.wireHomeViewModelClosures()` 方法 + onReadyAppear 内的调用
  - [x] `.ready` 分支改渲染 `MainTabView()` + `.environmentObject(coordinator)` + `.environmentObject(homeViewModel)` + `.environment(\.resetIdentityViewModel, ...)` + `.environment(\.sessionStore, container.sessionStore)`
  - [x] 保留 onReadyTask 内 ping bind / start + 外层 .task 内 loadHome bind
  - [x] 保留 launching / needsAuth 三态机不动

- [x] **Task 7：修改 HomeView + HomeViewModel**（AC4）
  - [x] HomeView：删 `bottomButtonRow` 私有 var + body 内调用 + body VStack 减一行
  - [x] HomeView：保留 versionFooter + ping/version 角落显示
  - [x] HomeView：加注释占位 `// Story 37.12 will mount JoinRoomModal sheet here via HomeViewModel.showJoinModal`（不实际加 `.sheet`）
  - [x] HomeViewModel：删 `onRoomTap` / `onInventoryTap` / `onComposeTap` 三个 closure 字段 + 三条 init 内的默认参数
  - [x] HomeViewModel：**不**加 `showJoinModal` 字段

- [x] **Task 8：删除 SheetPlaceholders/ 目录**（AC2 + AC9）
  - [x] 删 RoomPlaceholderView.swift / InventoryPlaceholderView.swift / ComposePlaceholderView.swift + 空目录

- [x] **Task 9：修改 AccessibilityID.swift**（AC7）
  - [x] 整段删除 `AccessibilityID.SheetPlaceholder` enum
  - [x] 保留 `Home.btnRoom` / `btnInventory` / `btnCompose` 三常量（加 deprecated 注释）
  - [x] **不**加 `AccessibilityID.Tab` enum

- [x] **Task 10：单元测试改写 / 新增**（AC6）
  - [x] 新建 `iphone/PetAppTests/App/HomeContainerViewTests.swift`，含 4 个 HomeRoomDispatcher case（idle / inRoom / 空字符串 edge / 状态切换链）
  - [x] 改写 `iphone/PetAppTests/App/SheetTypeTests.swift`：删 `.room` / `.inventory` 用例，保留 `.compose` 一致性断言（2 case）
  - [x] 改写 `iphone/PetAppTests/App/AppCoordinatorTests.swift`：删 `.room` / `.inventory` 用例，加 currentTab default / switchTab / 不影响 presentedSheet 用例 + currentRoomId 默认 nil + 可读写用例（共 9 case）
  - [x] 改写 `iphone/PetAppTests/App/RootViewWireTests.swift`：删原 onRoomTap / onInventoryTap / onComposeTap 闭包接 coordinator.present 用例（3 case）；保留 bootstrap closure retry / error mapping 用例（与本 story 主入口改造正交）
  - [x] 改写 `iphone/PetAppTests/Features/Home/HomeViewModelTests.swift`：删 onRoomTap / onInventoryTap / onComposeTap 4 个测试，保留 testHardcodedDefaultStateMatchesStorySpec
  - [x] HomeViewModelPingTests.swift / ViewModels/HomeViewModelLoadHomeTests.swift / HomeViewTests.swift 检查无引用（btnRoom 等 deprecated 常量保留 → HomeViewTests 仍通过）

- [x] **Task 11：UI 测试改写 / 新增**（AC7）
  - [x] 改写 `iphone/PetAppUITests/NavigationUITests.swift`：删 3 个旧 sheet 用例 + 加 5 个 Tab 路径用例（testFourTabsAreLocatable / testSwitchToWardrobeTab / testSwitchToFriendsTab / testSwitchToProfileTab / testTabSwitchBackToHomeRecoversHomeView）
  - [x] 改写 `iphone/PetAppUITests/HomeUITests.swift`：删 btnRoom / btnInventory / btnCompose 断言；保留其它断言（用例改名 testHomeViewShowsAllPlaceholders）
  - [N/A] mock server 抓取 `/ping` `/version` `/home` 各 1 次的断言路径**当前 PetAppUITests 不带此能力**，记入 Story 37.13 落地（manual 验证替代）

- [x] **Task 12：xcodegen 同步 + build / test 验证**（AC9）
  - [x] `iphone/project.yml` sources 是 glob（`- PetApp`）→ xcodegen 自动 pick 新文件
  - [x] `bash iphone/scripts/build.sh --test` 通过：235 unit case 全 pass
  - [x] `bash iphone/scripts/build.sh --uitest` 通过：10 UI case 全 pass（4 HomeUITests + 1 KeychainPersistenceUITests + 5 NavigationUITests）
  - [x] `bash iphone/scripts/build.sh` release build 通过（先于 unit/uitest 跑过）

- [x] **Task 13：更新本 story 文件 dev agent record**（AC9）
  - [x] Agent Model Used / Completion Notes List / File List / Change Log（见下方）

- [ ] **Task 14：commit**（AC9）—— **不在 dev-story workflow 范围内**，由 story-done / fix-review 流程负责

## Dev Notes

### 故事关键路径（X1+X2 修订强约束）：重新实装 vs 迁移

ADR-0009 v2 X1+X2 修订把本 story 路径从「partial revert + 渐进迁移 + grep 兜底」改为「**重新实装 + 编译器报错驱动 + UITest 全链路兜底**」（参见 ADR-0009 §4.1 + 本 story 顶部注释段）。Dev 实装时**严格按重新实装路径**：

- 旧 SheetType `.room` / `.inventory` enum case **整段直接删除** —— 删除后 `coordinator.present(.room)` / `present(.inventory)` 等所有调用立即编译失败 → dev 修编译错误时即修完全部 caller
- 旧 HomeView `bottomButtonRow` 3 CTA 按钮 **整段直接删除** —— 删除后 `viewModel.onRoomTap()` / `onInventoryTap()` / `onComposeTap()` 调用都不存在了，`onXTap` closure 字段也连同删除
- 旧 SheetPlaceholders/ 目录 **整目录删除** —— RoomPlaceholderView / InventoryPlaceholderView / ComposePlaceholderView 三文件不留任何痕迹
- 旧 NavigationUITests / HomeUITests 内 btnRoom / btnInventory / btnCompose 路径 **直接删除用例** —— 不写"deprecated 用例先 skip"

**禁止动作**：
- ❌ 在 SheetType enum 内 `case room` 上方加 `@available(*, deprecated)` 注释 + 保留 case body
- ❌ 在 HomeView bottomButtonRow 上方加 `// TODO: remove in Story 37.7` 注释 + 保留按钮 body
- ❌ 在 NavigationUITests 旧用例上方加 `func testTapRoomButton_PresentsRoomSheet() { skip("Story 37.3 superseded") }`
- ❌ 写 grep 脚本扫"全 codebase 内还有没有 `coordinator.present(.room)` 漏改"

**允许的退路**：如发现 ADR-0009 §3.5 步骤 1–8 与实际实装出现冲突（如 SwiftUI 17 TabView API 行为偏差） → 走「ADR 修订 patch + 改 v2 Accepted」路径（参考 ADR-0008 v2 commit `ec5beb3` 先例 + ADR-0009 §6 第 2 条）。修订 patch 与本 story commit **同 commit 落地**；commit message 显式记录"ADR-0009 §3.5 第 N 步修订：原文 X，修为 Y，理由 Z"。

### iOS 17+ TabView API 偏差 + 浮动 TabBar 自绘的踩坑预警

ADR-0009 §3.1 已知坑第 1 条：「TabView 默认 iOS Tab Bar 与 ui_design 浮动样式不同」缓解策略是「用 `TabView` 隐藏默认 TabBar（`.toolbar(.hidden, for: .tabBar)` 或 iOS 16+ `tabViewStyle`），自绘浮动 TabBar overlay」。Dev 实装时注意：

- iOS 17+ `.toolbar(.hidden, for: .tabBar)` 是公开 API（推荐用此路径）；iOS 16 / iOS 15 落地时需用 UIKit Bridge 改 `UITabBar.appearance()`（本 story 用 `.toolbar(.hidden, for: .tabBar)` 即可，因为 Cat App `iphone/project.yml` 钦定 deployment target ≥ iOS 17.0；如不是 → 提前 spike 评估）
- 浮动 TabBar 自绘时 SwiftUI **不**会自动给底部 safe area inset → MainTabView 内 TabView 需 `.padding(.bottom, 86)`（72pt TabBar 高 + 14pt 距底）让内容不被 TabBar 遮挡。**或者**用 `.safeAreaInset(edge: .bottom) { FloatingTabBar(...) }` 让 SwiftUI 自动处理 safe area（推荐用此路径）
- 键盘弹出时 SwiftUI 会把 TabBar 自动顶起；浮动 TabBar 自绘要测试键盘弹出场景下视觉是否合理（详见 Sprint Change Proposal v2 §6 风险 #4）

**强制 spike 时机**：本 story 实装期 dev 第一次落地浮动 TabBar overlay 时跑一次 simulator + 真机预览，确认：(a) 内容不被 TabBar 遮挡；(b) 键盘弹出 / 收起视觉合理；(c) Dynamic Type / 暗色模式占位色合理。任何视觉异常 → 在 commit message 内记录 + 计入 Story 37.13 visual review checklist。

### HomeContainerView 互斥状态机：appState 注入路径选择

**顺序方案（推荐）**：Story 37.4 先 done → 本 story 直接 `@EnvironmentObject var appState: AppState` 引用真实 AppState 类。前置：Story 37.4 必须**先于**本 story 进入 review 状态；Story 37.4 自身不依赖本 story（Story 37.4 改 HomeViewModel + 加 AppState + 改 LoadHomeUseCase 集成测试，无导航壳依赖）。

**并行方案（备选）**：本 story 先在 AppCoordinator 加 `@Published var currentRoomId: String?` 临时占位字段；Story 37.4 落地时 dev 把所有 `coordinator.currentRoomId` 引用改为 `appState.currentRoomId` + 删除 AppCoordinator 占位字段。⚠️ 风险：Story 37.4 落地的 dev 必须知道有此约定（本 story commit message + Story 37.4 故事文件 Dev Notes 双向记录）。

**Dev 决议步骤**：
1. 检查 sprint-status.yaml 内 `37-4-...` 状态：如果是 ready-for-dev / review / done → 走顺序方案
2. 如果是 backlog → 与 user / 上游 SM 协调先做 Story 37.4 还是并行
3. 协调结果记入本 story Completion Notes 第 1 条

### Tab enum 类型放置 + 命名空间策略

`Tab` enum 命名空间冲突预警：SwiftUI 内置 `Tab` 类型（`SwiftUI.Tab`）在 iOS 17 出现（用作 TabView modifier 的内部类型）。直接定义 `public enum Tab` 可能与 SwiftUI 内置 `Tab` 命名空间冲突。

**Dev 实装时的两种规避**：
- **路径 A（推荐）**：把 enum 命名为 `AppTab` —— 显式区分 App 自定义类型 vs SwiftUI 内置；本 story AC1 模板中 `Tab` 全部替换为 `AppTab`，AC5 / AC7 / a11y identifier 仍写 `tab_home` 等（标识符不变；Swift 类型名变）
- **路径 B**：用 `enum Tab` 但模块限定调用：`PetApp.Tab.home`（在跨模块场景必要；本 story 内部访问无需限定）

**Dev 决议**：实装期发现 SwiftUI 内置 `Tab` 命名冲突 → 走路径 A（重命名 `AppTab`）；Completion Notes 记录该决策 + 本 story AC 模板中 `Tab` 引用对应改为 `AppTab`。如不冲突 → 沿用 ADR-0009 §3.5 步骤 2 原文 `Tab` 命名。

### AccessibilityID 演化：本 story 用 inline 字符串，Story 37.13 归并

ADR-0002 + Story 2.6 钦定 a11y identifier 集中管理在 `AccessibilityID.swift`。但本 story 处于 Epic 37 中段，新加的 4 Tab + 4 占位 view a11y identifier 共 8 个；如果立即加 `AccessibilityID.Tab` + `AccessibilityID.Wardrobe` / `Friends` / `Profile` / `Room` 四个新 enum，会让 Story 37.13 a11y 总表 story 的归并工作产生 git diff 噪音。

**本 story 策略**：4 Tab + 4 占位 view a11y identifier 直接 inline 字符串写在 `.accessibilityIdentifier(...)` 调用内（如 `.accessibilityIdentifier("tab_home")`）；Story 37.13 a11y 总表落地时 dev 一次性归并到 `AccessibilityID.Tab` / `AccessibilityID.Wardrobe` 等 enum + 替换 inline 字符串为常量。

**例外**：
- 本 story **保留** `AccessibilityID.Home.btnRoom` / `btnInventory` / `btnCompose` 三常量（仅加注释 deprecated）—— 不在本 story 删除常量定义，避免触发其它 import 站点漂移；Story 37.13 a11y 总表归并时一并清理
- 本 story **整段删除** `AccessibilityID.SheetPlaceholder` enum —— 因为关联文件 SheetPlaceholders/ 已删，常量没有任何引用，删除是 safe 的

### NavigationStack 嵌入位置：每个 Tab 独立 vs RootView 全局

ADR-0009 §3.5 步骤 6 钦定「每个 Tab 根视图内嵌 NavigationStack（保留 Story 2.3 push 模板）」。本 story 实装路径：

- `HomeContainerView` 内 idle 态：`NavigationStack { HomeView(...) }`（HomeView 自身不嵌 NavigationStack；HomeContainerView 包一层）
- `WardrobeView` / `FriendsView` / `ProfileView` 内：占位 view 自身**包**一层 `NavigationStack { ... }`（Story 37.9 / 37.10 / 37.11 实装真实 view 时 NavigationStack 仍在那一层）

**禁止动作**：在 `MainTabView.body` 外层（TabView 外层）包一层 `NavigationStack { TabView { ... } }` —— 这违反 SwiftUI 模式（NavigationStack 应在每个 Tab 内部，不是 TabView 外部）。

### HomeView 的 SwiftUI Preview 兼容性

HomeView 现有 `HomeView_Previews` 用老 init `HomeView(viewModel: HomeViewModel())`。本 story 删 `onRoomTap` / `onInventoryTap` / `onComposeTap` closure 字段后，HomeViewModel 老 init `HomeViewModel(nickname:appVersion:serverInfo:onRoomTap:onInventoryTap:onComposeTap:)` 三个 closure 参数也删除 → Preview 调用兼容（默认参数没了，但 init 仍存在）。

**潜在 regress**：Story 5.5 / 2.5 / 2.3 既有测试中可能有 `HomeViewModel(onRoomTap: { ... })` 这类显式传 closure 的用例 → dev 实装时跑 `bash iphone/scripts/build.sh --test`，编译失败的用例同步删除 closure 参数。

### Source tree 改动汇总

```
[新增]
iphone/PetApp/App/MainTabView.swift                                   (含 Tab/AppTab enum + FloatingTabBar 私有子视图)
iphone/PetApp/Features/Home/Views/HomeContainerView.swift             (含 HomeRoomDispatcher public enum)
iphone/PetApp/Features/Home/Views/JoinRoomModal/JoinRoomModalPlaceholder.swift
iphone/PetApp/Features/Wardrobe/Views/WardrobeView.swift              (占位)
iphone/PetApp/Features/Friends/Views/FriendsView.swift                (占位)
iphone/PetApp/Features/Profile/Views/ProfileView.swift                (占位)
iphone/PetApp/Features/Room/Views/RoomViewPlaceholder.swift           (占位)
iphone/PetAppTests/App/HomeContainerViewTests.swift                   (或并入既有文件)

[修改]
iphone/PetApp/App/RootView.swift                                      (删 fullScreenCover + sheetContent + wireHomeViewModelClosures；改 .ready 渲染 MainTabView)
iphone/PetApp/App/AppCoordinator.swift                                (删 .room/.inventory case + 加 currentTab/switchTab)
iphone/PetApp/Features/Home/Views/HomeView.swift                      (删 bottomButtonRow + JoinRoomModal 注释占位)
iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift            (删 onRoomTap/onInventoryTap/onComposeTap closure 字段 + init 默认参数)
iphone/PetApp/Shared/Constants/AccessibilityID.swift                  (整段删 SheetPlaceholder enum；保留 Home.btnRoom 等加 deprecated 注释)
iphone/PetAppTests/App/SheetTypeTests.swift                           (删 .room/.inventory 用例)
iphone/PetAppTests/App/AppCoordinatorTests.swift                      (删 .room/.inventory 用例 + 加 currentTab/switchTab 用例)
iphone/PetAppUITests/NavigationUITests.swift                          (删 3 个旧 sheet 用例 + 加 4 Tab 路径用例)
iphone/PetAppUITests/HomeUITests.swift                                (删 btnRoom/btnInventory/btnCompose 断言)
_bmad-output/implementation-artifacts/37-3-rootview-maintabview-改造.md (本 story 文件)
_bmad-output/implementation-artifacts/sprint-status.yaml              (workflow 自动改)
iphone/project.yml                                                    (xcodegen explicit list 时手动同步新文件 / 删除条目；glob 不需改)

[删除]
iphone/PetApp/Features/Home/Views/SheetPlaceholders/RoomPlaceholderView.swift
iphone/PetApp/Features/Home/Views/SheetPlaceholders/InventoryPlaceholderView.swift
iphone/PetApp/Features/Home/Views/SheetPlaceholders/ComposePlaceholderView.swift
iphone/PetApp/Features/Home/Views/SheetPlaceholders/                  (空目录删除)
```

### 测试标准（与 ADR-0002 §3.1 + Story 2.7 测试基础设施一致）

- **不引入** SnapshotTesting / ViewInspector（ADR-0002 §3.1 严守红线）
- **抽决策逻辑为纯函数**让 XCTest 直接覆盖（HomeRoomDispatcher 与 HomePetNameResolver / HomeNicknameResolver 同精神）
- **UITest 用 a11y identifier 定位**（`tab_home` / `wardrobeView` / `home_versionLabel` 等），**不**用 view hierarchy traversal
- **mock server hook**（如本 story 实装期发现现有 PetAppUITests 不支持 mock server 抓取请求次数）→ 加 hook 或在 commit message 显式记录"hook 待 Story 37.13 落地"
- 跑 `bash iphone/scripts/build.sh --test` 是 done 硬条件；如失败必须同 commit 修复

### 与 Story 37.4 双向引用 + 顺序协调

| 维度 | Story 37.3（本 story） | Story 37.4（同层） |
|---|---|---|
| 目标 | 主入口导航壳：MainTabView + HomeContainerView | AppState 数据持有 + HomeViewModel.homeData 删除 |
| 依赖 ADR | ADR-0009（导航 TabView） | ADR-0010（AppState 单 source of truth） |
| 主关联文件 | RootView / AppCoordinator / HomeView / MainTabView | RootView / HomeViewModel / AppState / LoadHomeUseCase 集成测试 |
| 顺序协调 | 顺序方案推荐 37.4 先 done；并行方案需协调临时占位字段 | 与本 story 同层；可并行也可顺序 |

### 与 ADR-0009 §3.5 步骤 1–8 严格映射

| ADR-0009 §3.5 步骤 | 本 story AC | Tasks |
|---|---|---|
| 1：RootView .ready 改渲染 MainTabView | AC3 | Task 6 |
| 2：新建 MainTabView.swift | AC1 | Task 2 |
| 3：新建 HomeContainerView.swift | AC2 | Task 3 |
| 4：HomeView 删 3 CTA + TeamIdleCard 入口（TeamIdleCard 留给 Story 37.7） | AC4 | Task 7 |
| 5：AppCoordinator 删 .room/.wardrobe + 加 currentTab + 保留 .compose | AC5 | Task 5 |
| 6：每 Tab 内嵌 NavigationStack | AC2 + Dev Notes "NavigationStack 嵌入位置" | Task 4 |
| 7：JoinRoomModal .sheet 挂在 HomeView 内 | AC4（注释占位）+ AC2（JoinRoomModalPlaceholder 文件） | Task 4 + Task 7 |
| 8：launching / needsAuth 三态机保留 | AC3 + AC7 第 5 条 | Task 6 |

每条 ADR §3.5 步骤都有对应的 AC + Task；如 dev 实装期发现某步实际不可行 → 走 ADR 修订 patch + 改 v2 Accepted 路径（与本 story commit 同 commit）。

### Project Structure Notes

新文件全部按 ADR-0002 §3.3 工程目录方案落地：

```
iphone/PetApp/
├─ App/                       # MainTabView, AppCoordinator (修改), RootView (修改) - 应用启动 / 协调层
├─ Core/                      # 不动 (Story 37.5/37.6 落地 Theme + primitives)
├─ Shared/                    # AccessibilityID 修改 (删 SheetPlaceholder enum)
├─ Features/
│  ├─ Home/Views/             # HomeContainerView 新增 + HomeView 修改 + JoinRoomModal/ 子目录新增 + SheetPlaceholders/ 整目录删除
│  ├─ Wardrobe/Views/         # WardrobeView.swift 新增（Wardrobe 目录此前不存在；本 story 创建）
│  ├─ Friends/Views/          # FriendsView.swift 新增（Friends 目录此前不存在；本 story 创建）
│  ├─ Profile/Views/          # ProfileView.swift 新增（Profile 目录此前不存在；本 story 创建）
│  └─ Room/Views/             # RoomViewPlaceholder.swift 新增（Room 目录此前不存在；本 story 创建）
└─ ...
```

新建 4 个 Feature 目录（Wardrobe / Friends / Profile / Room）符合 ADR-0002 §3.3 + Epic 37 §AC 红线（每屏 ViewModel 走 class 层次结构）的目录预案；Story 37.7 / 37.8 / 37.9 / 37.10 / 37.11 落地时各自补充 ViewModels / UseCases / Repositories / Models 子目录。本 story 仅放 Views/ 占位 stub。

### References

- [Source: epics.md#Story-37.3](../planning-artifacts/epics.md) §Story 37.3 — Acceptance Criteria 原文（第 4607-4640 行）
- [Source: epics.md#Epic-37](../planning-artifacts/epics.md) §Epic 37 概览 — 红线（含 37.3/37.4 重构 story 红线豁免）+ Story 依赖链 + 接缝设计（第 4555-4573 行）
- [Source: ADR-0009](decisions/0009-iphone-navigation-tabview.md) — 本 story 主依据；§2 Decision Summary / §3.1 主入口 = TabView 4 Tab / §3.2 Home Tab 互斥模式 / §3.3 Sheet 保留白名单 / §3.4 AppCoordinator 角色变化 / §3.5 RootView 改造步骤 1-8 / §4.1 supersede 语义 / §4.2 下游 Story 影响表 / §6 验收
- [Source: ADR-0010](decisions/0010-iphone-app-state.md) — 联动 ADR；§3.1 ViewModel 注入规则（construct injection 唯一允许）/ §3.2 AppState 范围白名单（currentRoomId: String?）/ §3.3 hydrate 流程
- [Source: 37-1-adr-0009-导航架构.md](37-1-adr-0009-导航架构.md) — Story 37.1（已 done）；锁定 ADR-0009 contract validity
- [Source: 37-2-adr-0010-appstate.md](37-2-adr-0010-appstate.md) — Story 37.2（已 done）；锁定 ADR-0010 contract validity（含 path B verification model）
- [Source: sprint-change-proposal-2026-04-29-v2.md](../planning-artifacts/sprint-change-proposal-2026-04-29-v2.md) — Sprint Change v2.5 终审依据（commit `bef4531`）；§5.1 Story 37.3 详细 acceptance / §6 #4 浮动 TabBar 风险闭环 / §6 #7 mock server hook 闭环
- [Source: 2-3-导航架构搭建.md](2-3-导航架构搭建.md) — 被 supersede 的 Story 2.3 主入口部分钦定（status: superseded）；Story 2.3 push 模板 NavigationStack 仍保留作为 ADR-0009 §3.5 步骤 6 锚点
- [Source: ADR-0002 §3.3](decisions/0002-ios-stack.md) — iPhone 工程目录方案（Features/{Feature}/Views 子目录预案）
- [Source: ui_design/README.md](../../iphone/ui_design/README.md) — §App 结构 4 Tab IA / §iOS 设备规格 (72pt 高 / 14pt 距底 / 12pt 距左右 / 圆角 20)
- [Source: ADR-0008-v2](decisions/0008-error-protocol.md) — 先例：ADR 修订 + 改 Accepted 同 commit 模式（参考 commit `ec5beb3`，本 story 如需 ADR-0009 修订 patch 走该模式）
- [Source: CLAUDE.md](../../CLAUDE.md) — 本仓库工作纪律 + iPhone 端工程目录由 ADR-0002 锁定 + 端独立原则
- [Source: docs/lessons/2026-04-26-swiftui-switch-transition-explicit.md] — RootView .ready/.launching/.needsAuth 三态机 transition 实现的 lesson；本 story HomeContainerView 互斥状态机用同样 ZStack + .transition(.opacity) + .animation 模式
- [Source: docs/lessons/2026-04-27-bootstrap-all-error-paths-route-via-mapper.md] — bootstrap 错误链路；本 story 保留 LaunchedContentView .needsAuth 分支不动
- [Source: 5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据.md](5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据.md) — Story 5.5 钦定的 ping bind / loadHome bind / onReadyTask wire 模式（本 story 必须保留这套 wire）

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) — `claude-opus-4-7[1m]`

### Debug Log References

无（实装期间未触发 HALT；build / test 一次性通过）.

### Completion Notes List

1. **Story 37.4 协调走并行方案**（Task 1 决议）：实装时 sprint-status.yaml 内 `37-4-...` 状态为 backlog → 走并行方案，在 AppCoordinator 加 `@Published var currentRoomId: String?` 临时占位字段。Story 37.4 落地真实 AppState 后由该 dev 把所有 `coordinator.currentRoomId` 引用改为 `appState.currentRoomId` + 删除占位字段（commit message 显式记录该约定 → 见 AppCoordinator.swift 头部注释 + currentRoomId 字段 doc comment）。
2. **AppTab enum 命名**（Task 2 + Dev Notes "Tab enum 类型放置 + 命名空间策略"）：iOS 18+ SwiftUI 引入内置 `SwiftUI.Tab` 类型，App 自定义类型用 `AppTab` 显式区分（Dev Notes 钦定路径 A）。本 story 涉及的 a11y identifier 仍写 `tab_home` / `tab_wardrobe` / `tab_friends` / `tab_profile`（标识符不变；Swift 类型名变）。AppCoordinator.currentTab / switchTab 方法签名同步用 `AppTab`；测试 / UITest 引用同步更新。
3. **HomeView 依赖通过 environment 注入而非 init 参数透传**（Task 3 + Task 6 决策）：MainTabView 内嵌 HomeContainerView，再内嵌 HomeView，中间隔了 TabView 容器；如果 init 参数透传需要 MainTabView / HomeContainerView 两层都新增 `homeViewModel` / `resetIdentityViewModel` / `sessionStore` 三参数 → 实装噪音大且与 Story 37.5 / 37.7 进一步重构不友好。改为 RootView 在 `.ready` 分支一次性写入 4 个 environment（`.environmentObject(coordinator)` / `.environmentObject(homeViewModel)` / `.environment(\.resetIdentityViewModel, ...)` / `.environment(\.sessionStore, ...)`），HomeContainerHomeViewBridge 子视图集中读取后透传给 HomeView 既有 init。EnvironmentValues 自定义 key 在 HomeContainerView.swift 同文件内定义（避免新增小文件）。该决策不影响 ADR-0009 §3.5 步骤 1 / 3 钦定的 contract（"RootView 注入 MainTabView" + "HomeContainerView 持有 HomeView"），仅改注入机制。
4. **safeAreaInset 替代手动 padding**（Task 2 + Dev Notes "iOS 17+ TabView API 偏差"）：浮动 TabBar 用 `.safeAreaInset(edge: .bottom)` 让 SwiftUI 自动给内容预留底部 safe area，避免硬算 `padding(.bottom, 86)`。
5. **Mock server hook 待 Story 37.13 落地**（Task 11 / AC7 第 1 条）：当前 PetAppUITests 不带 mock server 请求次数抓取能力，启动 → mock server 收到 1 次 `/ping` + 1 次 `/version` + 1 次 `/home` 断言路径用 `UITEST_SKIP_GUEST_LOGIN=1` hook 绕开（Story 5.2 既有路径）。该断言路径正式落地随 Story 37.13 a11y 总表 / mock server hook 联动 story 一起做（manual 验证替代）。
6. **测试结果**：
   - **单元测试 235 case 全通过**（包括新增 HomeContainerViewTests 4 case + AppCoordinatorTabTests 5 case + currentRoomId 2 case + 改写 SheetTypeTests 2 case + 改写 RootViewWireTests 3 case）.
   - **UI 测试 10 case 全通过**：4 HomeUITests + 1 KeychainPersistenceUITests + 5 NavigationUITests（testFourTabsAreLocatable / testSwitchToWardrobeTab / testSwitchToFriendsTab / testSwitchToProfileTab / testTabSwitchBackToHomeRecoversHomeView）.
   - **release build** 通过（先 build 验证再跑 test；无 #if DEBUG 路径泄漏）.
7. **未发现 ADR-0009 偏差**：实装期严格按 §3.5 步骤 1–8 落地；ADR 修订 patch + 改 v2 Accepted 路径**未触发**.
8. **codex r2 [P3] 留档**：`iphone/PetApp/App/MainTabView.swift:110` 的 `MainTabView_Previews` 缺 `HomeViewModel()` environment 注入；DEBUG 预览会崩（非生产路径）。已 flag，可后续单 PR 一行修复（不阻塞本 story 收官）.

### File List

**[新增]**
- iphone/PetApp/App/MainTabView.swift（含 AppTab enum + FloatingTabBar 私有子视图）
- iphone/PetApp/Features/Home/Views/HomeContainerView.swift（含 HomeRoomDispatcher public enum + EnvironmentValues 自定义 key）
- iphone/PetApp/Features/Home/Views/JoinRoomModal/JoinRoomModalPlaceholder.swift
- iphone/PetApp/Features/Wardrobe/Views/WardrobeView.swift
- iphone/PetApp/Features/Friends/Views/FriendsView.swift
- iphone/PetApp/Features/Profile/Views/ProfileView.swift
- iphone/PetApp/Features/Room/Views/RoomViewPlaceholder.swift
- iphone/PetAppTests/App/HomeContainerViewTests.swift

**[修改]**
- iphone/PetApp/App/RootView.swift（删 fullScreenCover + sheetContent + wireHomeViewModelClosures；改 .ready 渲染 MainTabView + 4 个 environment 注入）
- iphone/PetApp/App/AppCoordinator.swift（删 .room / .inventory case + 加 currentTab / switchTab + 临时 currentRoomId 占位字段）
- iphone/PetApp/Features/Home/Views/HomeView.swift（删 bottomButtonRow + JoinRoomModal sheet 注释占位）
- iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift（删 onRoomTap / onInventoryTap / onComposeTap closure 字段 + 三条 init 内的默认参数）
- iphone/PetApp/Shared/Constants/AccessibilityID.swift（整段删 SheetPlaceholder enum；保留 Home.btnRoom / btnInventory / btnCompose 加 deprecated 注释）
- iphone/PetAppTests/App/SheetTypeTests.swift（删 .room / .inventory 用例）
- iphone/PetAppTests/App/AppCoordinatorTests.swift（删 .room / .inventory 用例 + 加 currentTab / switchTab / currentRoomId 用例）
- iphone/PetAppTests/App/RootViewWireTests.swift（删 onRoomTap / onInventoryTap / onComposeTap 接 coordinator.present 用例；保留 bootstrap closure retry / error mapping 用例）
- iphone/PetAppTests/Features/Home/HomeViewModelTests.swift（删 onXTap closure 测试 4 case，保留 testHardcodedDefaultStateMatchesStorySpec）
- iphone/PetAppUITests/NavigationUITests.swift（删 3 个旧 sheet 用例 + 加 5 个 Tab 路径用例）
- iphone/PetAppUITests/HomeUITests.swift（删 btnRoom / btnInventory / btnCompose 断言；用例改名 testHomeViewShowsAllPlaceholders）
- _bmad-output/implementation-artifacts/sprint-status.yaml（37-3 状态：ready-for-dev → in-progress → review）
- _bmad-output/implementation-artifacts/37-3-rootview-maintabview-改造.md（本 story 文件 dev agent record + Status）

**[删除]**
- iphone/PetApp/Features/Home/Views/SheetPlaceholders/RoomPlaceholderView.swift
- iphone/PetApp/Features/Home/Views/SheetPlaceholders/InventoryPlaceholderView.swift
- iphone/PetApp/Features/Home/Views/SheetPlaceholders/ComposePlaceholderView.swift
- iphone/PetApp/Features/Home/Views/SheetPlaceholders/（空目录已删）

### Change Log

| Date | Story Status | Note |
|---|---|---|
| 2026-04-30 | ready-for-dev → in-progress | dev-story workflow 启动 |
| 2026-04-30 | in-progress → review | Story 37.3 实装完成（ADR-0009 §3.5 步骤 1–8 全部落地）；235 unit + 10 UI test 全通过；release build 通过 |
| 2026-04-30 | review → done | codex r1 fix（commit 5bb6ed5）+ lesson backfill（docs/lessons/2026-04-30-coordinator-must-mirror-loaded-home-room-state.md `<pending>` → `5bb6ed5`）；codex r2 留 1 个 [P3]（MainTabView preview 缺 HomeViewModel 注入，已 flag 见 Completion Notes #8） |
