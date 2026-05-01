// HomeContainerView.swift
// Story 37.3：Home Tab 互斥状态机容器（ADR-0009 §3.5 步骤 3）.
// Story 37.4 改造（AC4）：currentRoomId 数据源从 AppCoordinator 临时占位字段切换为 AppState.
//
// 职责：
//   - 根据 `appState.currentRoomId` 在 HomeView ↔ RoomViewPlaceholder 互斥切换（淡入淡出 0.3s）.
//   - 不持有真实数据：HomeView 仍由 RootView 注入 `homeViewModel` / `resetIdentityViewModel`
//     / `sessionStore` 三参数（Story 5.5 / 2.5 / 2.3 钦定的 wire 模式不动）.
//
// 关键设计：
//   - HomeContainerView 内嵌 NavigationStack（每个 Tab 独立 NavigationStack —— ADR-0009 §3.5 步骤 6）.
//   - 互斥决策抽 `HomeRoomDispatcher.shouldShowRoom(currentRoomId:)` 纯函数 helper（ADR-0002
//     §3.1 禁用 ViewInspector / SnapshotTesting → 决策逻辑必须抽纯函数让 XCTest 直接覆盖；
//     与 HomePetNameResolver / HomeNicknameResolver 同精神）.
//   - 用 ZStack + .transition(.opacity) + .animation 实现互斥切换淡入淡出
//     （与 RootView 三态机同 lesson：2026-04-26-swiftui-switch-transition-explicit.md）.

import SwiftUI

public struct HomeContainerView: View {
    /// Story 37.4 AC4：currentRoomId 数据源从 AppCoordinator 临时占位字段切换为 AppState.
    @EnvironmentObject var appState: AppState

    public init() {}

    public var body: some View {
        ZStack {
            if HomeRoomDispatcher.shouldShowRoom(currentRoomId: appState.currentRoomId) {
                // Story 37.8：inRoom 态渲染 RoomScaffoldView（替换 Story 37.3 RoomViewPlaceholder 占位）.
                // RoomViewPlaceholder.swift 类型本身保留不删（保 git history；下游 Story 37.13 决定）.
                HomeContainerRoomViewBridge()
                    .transition(.opacity)
            } else {
                // idle 态：显示 HomeView 包在 NavigationStack 内（Story 5.5 既有内容不动；
                // 仅删 3 CTA 按钮 —— 见 HomeView.swift Story 37.3 修改）.
                NavigationStack {
                    HomeContainerHomeViewBridge()
                }
                .transition(.opacity)
            }
        }
        .animation(.easeInOut(duration: 0.3), value: appState.currentRoomId)
    }
}

/// HomeContainerView 内的 RoomScaffoldView 注入桥接子视图（与 HomeContainerHomeViewBridge 同模式）.
///
/// 为何抽出来：保 RoomViewModel 通过 EnvironmentObject 注入；与 HomeViewModel 注入路径同精神.
/// Story 12.1 落地时改用 RealRoomViewModel 替换基类（RootView wire 决定）.
private struct HomeContainerRoomViewBridge: View {
    @EnvironmentObject var roomViewModel: RoomViewModel

    var body: some View {
        RoomScaffoldView(state: roomViewModel)
    }
}

/// HomeContainerView 内的 HomeView 注入桥接子视图.
///
/// 为何抽出来：HomeView 需要 `homeViewModel: HomeViewModel` + `resetIdentityViewModel:
/// ResetIdentityViewModel?` + `sessionStore: SessionStore?` 三参数；这三者由 RootView 通过
/// EnvironmentObject 与 environment values 注入；本子视图集中读取 environment 后透传给 HomeView.
private struct HomeContainerHomeViewBridge: View {
    @EnvironmentObject var homeViewModel: HomeViewModel
    @Environment(\.resetIdentityViewModel) var resetIdentityViewModel
    @Environment(\.sessionStore) var sessionStore

    var body: some View {
        // Story 37.7: HomeView 改 generic struct + chestSlot ViewBuilder closure 接缝.
        // 参数名 viewModel → state；本期 chestSlot 传 EmptyView()（Story 21.1 改传 ChestCardView()）.
        HomeView(
            state: homeViewModel,
            resetIdentityViewModel: resetIdentityViewModel,
            sessionStore: sessionStore
        ) {
            EmptyView()
        }
    }
}

/// HomeContainerView 互斥状态机的决策 helper（与 HomePetNameResolver 同精神：抽纯函数让单测直接覆盖）.
///
/// 单一职责：根据 currentRoomId 是否为 nil 判断显示 RoomView vs HomeView.
/// 当未来扩展（如 currentRoomId 包含额外校验、leave-room transition 等）时，新规则集中在此处修改.
public enum HomeRoomDispatcher {
    /// 决定 HomeContainerView 应显示 RoomView 还是 HomeView.
    /// - Parameter currentRoomId: 来自 AppState.currentRoomId（临时方案下来自 AppCoordinator.currentRoomId）.
    /// - Returns: true → 显示 RoomView（inRoom 态）；false → 显示 HomeView（idle 态）.
    public static func shouldShowRoom(currentRoomId: String?) -> Bool {
        currentRoomId != nil
    }
}

// MARK: - Environment values for HomeView 依赖注入（替代 init 参数透传）

/// `ResetIdentityViewModel?` 注入入口 (RootView 在 .environment 写入；HomeContainerHomeViewBridge 读取).
///
/// 为何走 EnvironmentValues 而非 init 参数：HomeContainerView 是 MainTabView 内嵌子视图,
/// 中间隔了 TabView 容器；通过 environment 让 RootView 一次性写入,无需每层 init 参数透传.
private struct ResetIdentityViewModelKey: EnvironmentKey {
    static let defaultValue: ResetIdentityViewModel? = nil
}

extension EnvironmentValues {
    var resetIdentityViewModel: ResetIdentityViewModel? {
        get { self[ResetIdentityViewModelKey.self] }
        set { self[ResetIdentityViewModelKey.self] = newValue }
    }
}

/// `SessionStore?` 注入入口（同 ResetIdentityViewModel 模式）.
private struct SessionStoreKey: EnvironmentKey {
    static let defaultValue: SessionStore? = nil
}

extension EnvironmentValues {
    var sessionStore: SessionStore? {
        get { self[SessionStoreKey.self] }
        set { self[SessionStoreKey.self] = newValue }
    }
}
