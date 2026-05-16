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
/// Story 18.2: 通过 `\.emojiPanelViewModelFactory` environment value 拿到 RootView 注入的工厂闭包,
///   传给 RoomScaffoldView init（避免在 bridge / Container 任一层持有 AppContainer 依赖）.
private struct HomeContainerRoomViewBridge: View {
    @EnvironmentObject var roomViewModel: RoomViewModel
    @Environment(\.emojiPanelViewModelFactory) var emojiPanelViewModelFactory
    /// Story 18.4 AC8: LoadEmojisUseCase 从 RootView 注入 (\.loadEmojisUseCase environment value),
    /// 透传给 RoomScaffoldView init → EmojiAnimationLayer → FloatingEmojiCellView .task 查 catalog 拿 assetUrl.
    @Environment(\.loadEmojisUseCase) var loadEmojisUseCase

    var body: some View {
        RoomScaffoldView(
            state: roomViewModel,
            emojiPanelViewModelFactory: emojiPanelViewModelFactory,
            loadEmojisUseCase: loadEmojisUseCase
        )
    }
}

/// HomeContainerView 内的 HomeView 注入桥接子视图.
///
/// 为何抽出来：HomeView 需要 `homeViewModel: HomeViewModel` + `resetIdentityViewModel:
/// ResetIdentityViewModel?` + `sessionStore: SessionStore?` 三参数；这三者由 RootView 通过
/// EnvironmentObject 与 environment values 注入；本子视图集中读取 environment 后透传给 HomeView.
private struct HomeContainerHomeViewBridge: View {
    @EnvironmentObject var homeViewModel: HomeViewModel
    /// Story 21.1 AC4：在 Bridge 子视图追加 AppState 注入，让 chestSlot closure 读 `appState.currentChest`
    /// 派生 ChestCardView props（与 HomeContainerView 外层 @EnvironmentObject 同模式；RootView 注入路径继承不变）.
    @EnvironmentObject var appState: AppState
    @Environment(\.resetIdentityViewModel) var resetIdentityViewModel
    @Environment(\.sessionStore) var sessionStore

    var body: some View {
        // Story 37.7: HomeView 改 generic struct + chestSlot ViewBuilder closure 接缝.
        // 参数名 viewModel → state；本期 chestSlot 传 EmptyView()（Story 21.1 改传 ChestCardView()）.
        // Story 8.4 AC5: petSlot 接缝传入 PetSpriteView，订阅 viewModel.petState 派生.
        //
        // SwiftUI 自动追踪 viewModel.petState 变化：HomeContainerHomeViewBridge body 重新求值时,
        //   PetSpriteView(state: ...) 也跟着重新构造 → state 参数变化 → AC2 钦定的
        //   `.animation(.easeInOut(duration: 0.2), value: state)` 触发 200ms 过渡.
        //
        // 注：homeViewModel 类型签名是基类 `HomeViewModel`，本 view 通过 EnvironmentObject 拿到的
        //   实例运行时是 RealHomeViewModel（RootView 注入路径），但 .petState 字段在基类 @Published
        //   声明，子类继承零冲突.
        //
        // Story 21.1 AC4: chestSlot closure 从 EmptyView() 替换为 ChestCardView(...)；
        //   onOpenTap 占位 `{}`（Story 21.3 落地真实 OpenChestUseCase 调用）.
        HomeView(
            state: homeViewModel,
            resetIdentityViewModel: resetIdentityViewModel,
            sessionStore: sessionStore,
            petSlot: { PetSpriteView(state: homeViewModel.petState) },
            chestSlot: {
                ChestCardView(
                    currentChest: appState.currentChest,
                    remainingSeconds: homeViewModel.chestRemainingSeconds,
                    isOpening: homeViewModel.isOpening,
                    // Story 21.5 AC2: 传 state.isSyncingSteps（与既有 state.isOpening 对称）——
                    // 开箱前步数同步 > 2s 时副标题切 "同步步数中…".
                    isSyncingSteps: homeViewModel.isSyncingSteps,
                    onOpenTap: {
                        // Story 21.3 AC7：替换占位空闭包为真实 onChestOpenTap.
                        // RealHomeViewModel override 调 OpenChestUseCase；MockHomeViewModel 记录 invocation.
                        homeViewModel.onChestOpenTap()
                    }
                )
            }
        )
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

/// Story 18.2 AC5: `EmojiPanelViewModel` 工厂闭包注入入口（RootView 写入；HomeContainerRoomViewBridge 读取）.
///
/// 为何走 EnvironmentValues 而非 RoomScaffoldView caller 直接构造：HomeContainerView 是 MainTabView
/// 内嵌子视图，中间隔 TabView 容器；RoomScaffoldView 实际由 HomeContainerRoomViewBridge 构造，
/// 直接给 RoomScaffoldView 传 AppContainer 会让 bridge / Container 都引入 AppContainer 依赖.
/// 用 environment 模式让 RootView 一次性 write `{ container.makeEmojiPanelViewModel() }`，
/// 路径所有节点保持单向 wire（无需每层 init 参数透传）.
///
/// 默认值: 返回 placeholder EmojiPanelViewModel（永远 loading 态；caller 若忘记 wire 时 sheet 弹出
/// 仅显示 ProgressView，**不**导致 crash）.
/// 详见 docs/lessons/2026-04-27-environment-value-default-must-not-crash.md（与既有 EnvironmentValues
/// 默认值同精神：默认值是 fail-safe，不是"正常使用"路径）.
private struct EmojiPanelViewModelFactoryKey: EnvironmentKey {
    @MainActor
    static var defaultValue: () -> EmojiPanelViewModel {
        return { EmojiPanelViewModel(useCase: NeverReturnsLoadEmojisUseCase()) }
    }
}

extension EnvironmentValues {
    var emojiPanelViewModelFactory: () -> EmojiPanelViewModel {
        get { self[EmojiPanelViewModelFactoryKey.self] }
        set { self[EmojiPanelViewModelFactoryKey.self] = newValue }
    }
}

/// Story 18.2 AC5: environment 默认 fallback 用 UseCase —— execute() 永久挂起,
/// 让 EmojiPanelViewModel.state 永远停 loading（让 caller 忘 wire 时不 crash 而是显示 ProgressView）.
/// 与 EmojiPanelView.swift 内 `NeverReturnsUseCase` 同精神（私有给 #Preview 用）.
private struct NeverReturnsLoadEmojisUseCase: LoadEmojisUseCaseProtocol {
    func execute() async throws -> [EmojiConfig] {
        try await Task.sleep(nanoseconds: UInt64.max)
        return []
    }
}

/// Story 18.4 AC8: `LoadEmojisUseCaseProtocol?` 注入入口（RootView 写入；HomeContainerRoomViewBridge 读取）.
///
/// 为何走 EnvironmentValues 而非 init 参数：与 emojiPanelViewModelFactory 同模式 —— HomeContainerView
/// 是 MainTabView 内嵌子视图，bridge 直接拿 environment 即可，不需层层透传 AppContainer.
///
/// 默认 nil → RoomScaffoldView 收到 nil → EmojiAnimationLayer 把 nil 透传给 FloatingEmojiCellView
/// → .task 内 `if let loader = loadEmojisUseCase` 短路 → assetUrl 保持 nil → 问号 SF Symbol fallback.
/// (V1 §12.3 行 2474 (d) catalog miss 渲染层 fallback；nil 路径与 catalog miss 路径表现一致).
private struct LoadEmojisUseCaseKey: EnvironmentKey {
    static let defaultValue: LoadEmojisUseCaseProtocol? = nil
}

extension EnvironmentValues {
    var loadEmojisUseCase: LoadEmojisUseCaseProtocol? {
        get { self[LoadEmojisUseCaseKey.self] }
        set { self[LoadEmojisUseCaseKey.self] = newValue }
    }
}
