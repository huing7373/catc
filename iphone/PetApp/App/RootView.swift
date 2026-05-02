// RootView.swift
// Story 2.2 起占位 RootView：渲染 HomeView.
// Story 2.3 起：注入 AppCoordinator，把 HomeView 三个 CTA 闭包连到 coordinator.present(...)，
// 并通过 .fullScreenCover(item:) 弹出对应 Sheet placeholder.
// Story 2.5 起（追加）：
//   - 新增 @StateObject private var container = AppContainer()
//   - 新增 .task：bind PingUseCase 后触发 start() 拉 /ping + /version
// Story 2.9 起（追加）：
//   - 新增 @StateObject private var launchStateMachine = AppLaunchStateMachine()
//   - body 改为 ZStack { switch launchStateMachine.state } 三分支路由 + 每分支 `.transition(.opacity)`：
//     · .launching → LaunchingView
//     · .ready → HomeView 子树（保留 .onAppear / .fullScreenCover / errorPresenter 既有 wire）
//     · .needsAuth(presentation:) → 根据 presentation 三态分发（Story 5.5 round 2 [P1] fix）.
//
// Story 37.3 改造（ADR-0009 §3.5 步骤 1 + 8）：
//   - 删除 RootView.wireHomeViewModelClosures() 方法（HomeView 不再有 onRoomTap / onInventoryTap /
//     onComposeTap closure 字段；主入口 IA 改 4 Tab + HomeContainerView 互斥状态机）.
//   - `.ready` 分支不再渲染 homeView，改渲染 `MainTabView()` + `.environmentObject(coordinator)`
//     + `.environmentObject(homeViewModel)` + `.environment(\.resetIdentityViewModel, ...)`
//     + `.environment(\.sessionStore, container.sessionStore)`（HomeContainerView 通过 environment
//     读取 HomeView 三参数）.
//   - **保留** `.fullScreenCover(item: $coordinator.presentedSheet)` modifier 仅服务 `.compose`
//     次级路由（ADR-0009 §3.4 钦定 SheetType 白名单仅留 .compose；.room/.inventory 已 supersede
//     为 4 Tab + HomeContainer 互斥状态机）. codex round 1 [P2] fix: 不挂会让 present(.compose)
//     变 silent no-op.
//   - 保留：launching / needsAuth 三态机不变；onReadyTask 内 ping bind / start + 外层 .task 内
//     loadHome bind + errorPresentationHost 全部既有 wire.

import SwiftUI

struct RootView: View {
    @StateObject private var coordinator = AppCoordinator()
    @StateObject private var container = AppContainer()
    /// Story 37.7 codex round 1 [P1] fix：`RealHomeViewModel` 而非裸 `HomeViewModel`.
    ///
    /// 原 (Story 2.5+) 注入裸 `HomeViewModel()` 在 Story 37.7 引入 5 个 abstract method (`onCreateTap`
    /// / `onJoinTap` / `onFeedTap` / `onPetTap` / `onPlayTap`，基类 `fatalError("subclass override")`)
    /// 之后变成生产 crash 路径——用户在 .ready 子树点 actionRow 三按钮 / teamIdleCard "创建队伍" /
    /// "加入队伍" 任一按钮就会 crash. RealHomeViewModel override 5 个方法走占位行为（写 showJoinModal
    /// / interactionAnimation），让 UI 链路活起来.
    ///
    /// 注：用 parameterless `RealHomeViewModel()` 而非 `RealHomeViewModel(appState:)` —— SwiftUI
    /// `@StateObject` 属性初始化器内不能交叉引用同级 `@StateObject appState`（self 未求值）；
    /// AppState 通过下方 `.task` 内 `homeViewModel.bind(appState: appState)` 延迟注入（与 pingUseCase /
    /// loadHomeUseCase 既有 bind 模式一致）.
    /// 详见 docs/lessons/2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md.
    @StateObject private var homeViewModel: HomeViewModel = RealHomeViewModel()

    /// Story 37.8 AC5：RoomScaffoldView 注入入口；与 homeViewModel 同模式 @StateObject 持有 + .environmentObject 注入子树.
    /// 静态类型 `RoomViewModel`（基类）让 SwiftUI `@StateObject` 老模式可用 —— AppState 也是同级 @StateObject,
    /// 不能在属性初始化器内交叉引用（编译期不允许 self 提前求值）.AppState 通过 `.onAppear` 同步注入
    /// （Story 37.8 round 2 [P2] fix；详见下方 `.onAppear` 内注释 + lesson
    /// docs/lessons/2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md）.
    /// 实例类型 `RealRoomViewModel`（生产实装；onLeaveTap / onCopyTap override 完整）—— round 1 P2 fix
    /// 已把基类换 Real，让 inRoom path（UITEST_FORCE_IN_ROOM / 未来 Story 12.1 join flow）走 onLeaveTap
    /// 调 appState.setCurrentRoomId(nil) 切回 idle 不再 silent no-op.
    @StateObject private var roomViewModel: RoomViewModel = RealRoomViewModel()

    /// Story 37.9 AC5：WardrobeScaffoldView 注入入口；与 homeViewModel / roomViewModel 同模式
    /// @StateObject 持有 + .environmentObject 注入子树.
    /// 静态类型 `WardrobeViewModel`（基类）让 SwiftUI `@StateObject` 老模式可用；AppState 通过
    /// `.onAppear` 同步注入（与 RealRoomViewModel.bind 同精神，避免 launch-time race）.
    /// 实例类型 `RealWardrobeViewModel`（生产实装）—— Story 37.7 round 1 [P1] lesson 预防性应用,
    /// 不能用裸基类 WardrobeViewModel()：基类 onEquipTap 是 fatalError，用户点装备按钮即 crash.
    @StateObject private var wardrobeViewModel: WardrobeViewModel = RealWardrobeViewModel()

    /// Story 37.10 AC5：FriendsScaffoldView 注入入口；与 homeViewModel / roomViewModel /
    /// wardrobeViewModel 同模式 @StateObject 持有 + .environmentObject 注入子树.
    /// 静态类型 `FriendsViewModel`（基类）让 SwiftUI `@StateObject` 老模式可用；AppState 通过
    /// `.onAppear` 同步注入（与 RealRoomViewModel.bind / RealWardrobeViewModel.bind 同精神，
    /// 避免 launch-time race；Story 37.8 round 2 [P2] lesson 钦定路径）.
    /// 实例类型 `RealFriendsViewModel`（生产实装）—— Story 37.7 round 1 [P1] lesson 预防性应用,
    /// 不能用裸基类 FriendsViewModel()：基类 onInviteFriendTap / onJoinFriendTap 是 fatalError，
    /// 用户点邀请/加入按钮即 crash.
    @StateObject private var friendsViewModel: FriendsViewModel = RealFriendsViewModel()

    /// Story 37.11 AC5：ProfileScaffoldView 注入入口；与 homeViewModel / roomViewModel /
    /// wardrobeViewModel / friendsViewModel 同模式 @StateObject 持有 + .environmentObject 注入子树.
    /// 静态类型 `ProfileViewModel`（基类）让 SwiftUI `@StateObject` 老模式可用；AppState 通过
    /// `.onAppear` 同步注入（与 RealRoomViewModel.bind / RealWardrobeViewModel.bind /
    /// RealFriendsViewModel.bind 同精神，避免 launch-time race；Story 37.8 round 2 [P2] lesson 钦定路径）.
    /// 实例类型 `RealProfileViewModel`（生产实装）—— Story 37.7 round 1 [P1] lesson 预防性应用,
    /// 不能用裸基类 ProfileViewModel()：基类 5 个 abstract method（onWeChatCardTap /
    /// onWeChatBindConfirmTap / onWeChatModalDismissTap / onMenuTap / onCollectionViewAllTap）
    /// 是 fatalError，用户点任一按钮即 crash.
    @StateObject private var profileViewModel: ProfileViewModel = RealProfileViewModel()

    /// Story 37.4 AC3：全局 AppState 单 source of truth；通过 `.environmentObject(appState)` 注入子树.
    /// 与 coordinator / homeViewModel 同级 @StateObject 持有；HomeViewModel.bind(appState:) 在 .task 内调
    /// 形成 weak 反向引用，让 applyHomeData(_:) 写入 AppState（不再写自身 homeData 字段）.
    @StateObject private var appState = AppState()

    /// Story 37.5 AC8: Theme 注入 source-of-truth.
    /// 当前固定 .candy（Story 37.14 白名单：本期不做 UI 切换面板）.
    /// 用 @State 而非 let 是为了未来主题切换 UI（mini-epic）落地时能直接改 @Binding 不破坏
    /// RootView 类型契约.Theme 是 value type → 用 @State (而非 @StateObject) 即可.
    @State private var currentTheme: ThemeName = .candy

    /// Story 2.9 新增 / Story 5.2 升级：启动状态机.
    @State private var launchStateMachine: AppLaunchStateMachine?

    #if DEBUG
    /// Story 2.8: dev "重置身份" 按钮 ViewModel.仅 Debug build 存在；Release build 字段不存在.
    @State private var resetIdentityViewModel: ResetIdentityViewModel?
    #endif

    var body: some View {
        ZStack {
            if let stateMachine = launchStateMachine {
                LaunchedContentView(
                    stateMachine: stateMachine,
                    coordinator: coordinator,
                    homeViewModel: homeViewModel,
                    roomViewModel: roomViewModel,
                    wardrobeViewModel: wardrobeViewModel,
                    friendsViewModel: friendsViewModel,
                    profileViewModel: profileViewModel,
                    appState: appState,
                    currentTheme: currentTheme,
                    sessionStore: container.sessionStore,
                    resetIdentityViewModel: currentResetIdentityViewModel(),
                    onReadyAppear: {
                        #if DEBUG
                        // lazy 注入：第一次 .onAppear 时从已稳定的 container 拿 keychainStore，
                        // 保证 reset 按钮调的 removeAll() 清的是 container.keychainStore 这同一份.
                        // nil 守卫让 RootView 重建（如旋转 / 离开返回）时不会重新构造覆盖既有 instance.
                        // Story 37.4：注入 appState 让 reset 按钮成功后清 AppState（reset 流程 ADR-0010 §3.7）.
                        if resetIdentityViewModel == nil {
                            resetIdentityViewModel = container.makeResetIdentityViewModel(appState: appState)
                        }
                        #endif
                    },
                    onReadyTask: {
                        // Story 5.5 round 4 / round 6 钦定 wire：bind ping + start 在同一 .ready
                        // 分支 .task 内串行（避免 SwiftUI 多 .task 顺序 race；详见 lesson
                        // 2026-04-27-swiftui-multi-task-no-ordering.md）.
                        homeViewModel.bind(pingUseCase: container.makePingUseCase())
                        await homeViewModel.start()
                    }
                )
            } else {
                LaunchingView().transition(.opacity)
            }

            #if DEBUG
            // Story 5.1 AC5: UITest hook —— XCUITest 通过 launchEnvironment 触发 keychain 写/读.
            KeychainUITestHookView(container: container)
            #endif
        }
        .onAppear {
            // Story 37.8 round 2 [P2] fix（codex review）：把 `bind(appState:)` 同步搬到 `.onAppear`，
            // 让 RealHomeViewModel / RealRoomViewModel 在第一次 paint 之前就持有 AppState 引用.
            //
            // 旧（round 1）实装在下方 `.task` 内调 bind(appState:) 是异步路径——SwiftUI 在第一次
            // paint 之前 `.task` 还没触发，HomeContainerView 互斥状态机已经按 appState.currentRoomId
            // 决定走 inRoom 分支 → RoomScaffoldView 渲染 → RealRoomViewModel.appState 仍 nil →
            // leave tap silent no-op + room title/code 显示 placeholder. 触发条件：
            //   · `/home` 返回 `room.currentRoomId != nil`（restored in-room session）→ AppLaunchStateMachine
            //     bootstrapStep1 内 appState.applyHomeData 写非 nil currentRoomId → ready 子树第一帧 paint
            //   · UITEST_FORCE_IN_ROOM env flag → ensureLaunchStateMachineWired 内 setCurrentRoomId 立即写
            //     非 nil → ready 子树第一帧 paint
            //
            // `.onAppear` 在 SwiftUI 第一次 paint 之前同步执行，比 `.task` 早 → 任何渲染之前 VM 已绑定.
            // 复用现有 bind(appState:) 入口（idempotent；alreadySubscribed guard），不动 ViewModel 持有结构.
            // 详见 docs/lessons/2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md.
            homeViewModel.bind(appState: appState)
            if let realRoomVM = roomViewModel as? RealRoomViewModel {
                realRoomVM.bind(appState: appState)
            }
            // Story 37.9 AC5 Task 5.4：与 RealRoomViewModel.bind 同精神，
            // `.onAppear` 同步路径让 RealWardrobeViewModel 在第一次 paint 之前持有 AppState 引用
            // （按 Story 37.8 round 2 [P2] lesson 钦定路径，**不**放 `.task`）.
            if let realWardrobeVM = wardrobeViewModel as? RealWardrobeViewModel {
                realWardrobeVM.bind(appState: appState)
            }
            // Story 37.10 AC5 Task 5.4：与 RealRoomViewModel.bind / RealWardrobeViewModel.bind 同精神，
            // `.onAppear` 同步路径让 RealFriendsViewModel 在第一次 paint 之前持有 AppState 引用
            // （按 Story 37.8 round 2 [P2] lesson 钦定路径，**不**放 `.task`）.
            if let realFriendsVM = friendsViewModel as? RealFriendsViewModel {
                realFriendsVM.bind(appState: appState)
            }
            // Story 37.11 AC5 Task 5.4：与 RealRoomViewModel.bind / RealWardrobeViewModel.bind /
            // RealFriendsViewModel.bind 同精神，`.onAppear` 同步路径让 RealProfileViewModel 在
            // 第一次 paint 之前持有 AppState 引用（按 Story 37.8 round 2 [P2] lesson 钦定路径，
            // **不**放 `.task`）.
            if let realProfileVM = profileViewModel as? RealProfileViewModel {
                realProfileVM.bind(appState: appState)
            }
            ensureLaunchStateMachineWired()
        }
        .task {
            // Story 5.5：bind LoadHomeUseCase + ErrorPresenter，让 ErrorPresenter onRetry 闭包
            // 能驱动 ViewModel 重试. 这两个依赖 container.makeLoadHomeUseCase() / container.errorPresenter
            // 构造（容器初始化），保留 .task 异步路径.
            homeViewModel.bind(
                loadHomeUseCase: container.makeLoadHomeUseCase(),
                errorPresenter: container.errorPresenter
            )
        }
        .task {
            ensureLaunchStateMachineWired()
            await launchStateMachine?.bootstrap()
        }
        .errorPresentationHost(presenter: container.errorPresenter)
    }

    /// 取当前 ResetIdentityViewModel? 给 LaunchedContentView (Debug 注入；Release 永远 nil).
    private func currentResetIdentityViewModel() -> ResetIdentityViewModel? {
        #if DEBUG
        return resetIdentityViewModel
        #else
        return nil
        #endif
    }

    /// Story 5.2 AC8 新增：lazy 注入 `launchStateMachine`，把 GuestLoginUseCase + SessionStore wire 到 bootstrapStep1.
    private func ensureLaunchStateMachineWired() {
        guard launchStateMachine == nil else { return }

        #if DEBUG
        if ProcessInfo.processInfo.environment["UITEST_SKIP_GUEST_LOGIN"] == "1" {
            // UITest 路径：bootstrap 立即成功，复用 Story 2.9 默认 closure 行为（让 HomeView 可渲染）
            launchStateMachine = AppLaunchStateMachine()
            // Story 37.8 AC8: UITest 路径强制切到 inRoom 态（让 RoomScaffoldView 7 锚 a11y identifier 可定位）.
            // 仅 Debug build 生效；Production build 此 env 被忽略.
            // 复用 Story 37.4 setCurrentRoomId 入口；不污染生产 wire；UITest 跑完 reset env 即恢复正常.
            // env flag 名 `UITEST_FORCE_IN_ROOM` 与现有 `UITEST_SKIP_GUEST_LOGIN` 同前缀（Story 2.2 落地的命名风格）.
            if ProcessInfo.processInfo.environment["UITEST_FORCE_IN_ROOM"] == "1" {
                appState.setCurrentRoomId("1234567")
            }
            return
        }
        #endif

        let useCase = container.makeGuestLoginUseCase()
        let loadHomeUseCase = container.makeLoadHomeUseCase()
        let sessionStore = container.sessionStore
        let homeViewModel = self.homeViewModel
        let appState = self.appState
        launchStateMachine = AppLaunchStateMachine(
            bootstrapStep1: { @Sendable in
                let output: GuestLoginOutput
                do {
                    output = try await useCase.execute()
                } catch {
                    throw BootstrapMappedError(
                        presentation: AppErrorMapper.presentation(for: error),
                        underlying: error
                    )
                }
                await MainActor.run {
                    sessionStore.updateSession(SessionState(user: output.user, pet: output.pet))
                }
                let homeData: HomeData
                do {
                    homeData = try await loadHomeUseCase.execute()
                } catch {
                    throw BootstrapMappedError(
                        presentation: AppErrorMapper.presentation(for: error),
                        underlying: error
                    )
                }
                await MainActor.run {
                    // Story 37.4 AC3：直接写 AppState（而非通过 coordinator.currentRoomId 双写）.
                    // 设计决议：保留 homeViewModel.applyHomeData(homeData) 调用 ——
                    // HomeViewModel 内 loadingState / hasLoadedHome 短路 flag 仍归 HomeViewModel
                    // 自己管（ADR-0010 §3.2 钦定 loading 状态归 ViewModel transient，不进 AppState）；
                    // HomeViewModel.applyHomeData 内部既调 self.appState?.applyHomeData(data) 写
                    // AppState，又设 loadingState=.loaded 与 hasLoadedHome=true.
                    // RootView 这里也直接调 appState.applyHomeData(homeData) 是为了：
                    // 让 AppState hydrate 不依赖 HomeViewModel 实例存在（如未来 LaunchingViewModel
                    // 直接调 LoadHomeUseCase 也能写 AppState）；双写**不**是 anti-pattern：内层
                    // HomeViewModel.applyHomeData 写 AppState 会重复，但因为是同一个 AppState
                    // 实例 + idempotent 赋值（同值），测试可断言 currentUser 与 hasLoadedHome 两套语义并存.
                    appState.applyHomeData(homeData)
                    homeViewModel.applyHomeData(homeData)
                }
            }
        )

        // ADR-0008 v2 §6.3 / Story 0008-impl-1: wire 401 cold-start handler.
        if let stateMachine = launchStateMachine {
            container.unauthorizedHandlerSink.setHandler { [weak stateMachine, weak sessionStore] in
                await MainActor.run {
                    sessionStore?.clear()
                }
                await stateMachine?.triggerColdStart()
            }
        }
    }
}

// Story 5.5 round 3 [P1] fix: 移除原 GuestLoginCompletionGate actor.
// 详见 docs/lessons/2026-04-27-bootstrap-retry-must-not-skip-auth.md.

/// Story 5.5 codex round 1 [P2] fix + round 2 [P1] fix: 把 bootstrap step closure 内的失败
/// 包装成携带完整 ErrorPresentation 语义的 LocalizedError, 让状态机决定 retry vs alert vs toast.
struct BootstrapMappedError: LocalizedError {
    let presentation: ErrorPresentation
    let underlying: Error

    var errorDescription: String? {
        switch presentation {
        case let .toast(message):
            return message
        case let .alert(_, message):
            return message
        case let .retry(message):
            return message
        }
    }
}

/// Story 5.2 新增子视图：用 `@ObservedObject` 订阅 `AppLaunchStateMachine.state` 的变化.
///
/// Story 37.3 改造（ADR-0009 §3.5 步骤 1）：
///   - .ready 分支改渲染 MainTabView + 注入 4 个 environment（coordinator / homeViewModel /
///     resetIdentityViewModel / sessionStore），让 HomeContainerView 通过 environment 透传给 HomeView.
///   - homeView 闭包参数删除（不再渲染 HomeView 当根；HomeContainerView 内嵌 HomeView 由 MainTabView
///     的 Home Tab 拿到）.
///   - **保留** `.fullScreenCover(item: $coordinator.presentedSheet)` modifier 服务 `.compose` 次级
///     路由（codex round 1 [P2] fix）. 渲染 ComposePlaceholderView（Story 33.1 落地真实合成 view）.
private struct LaunchedContentView: View {
    @ObservedObject var stateMachine: AppLaunchStateMachine
    @ObservedObject var coordinator: AppCoordinator
    let homeViewModel: HomeViewModel
    /// Story 37.8 AC5：接收 RootView 的 RoomViewModel，注入到 .ready 子树 environmentObject.
    /// HomeContainerRoomViewBridge 通过 @EnvironmentObject var roomViewModel: RoomViewModel 取出.
    let roomViewModel: RoomViewModel
    /// Story 37.9 AC5：接收 RootView 的 WardrobeViewModel，注入到 .ready 子树 environmentObject.
    /// WardrobeView 通过 @EnvironmentObject var wardrobeViewModel: WardrobeViewModel 取出后传给
    /// WardrobeScaffoldView(state:) 子视图.
    let wardrobeViewModel: WardrobeViewModel
    /// Story 37.10 AC5：接收 RootView 的 FriendsViewModel，注入到 .ready 子树 environmentObject.
    /// FriendsView 通过 @EnvironmentObject var friendsViewModel: FriendsViewModel 取出后传给
    /// FriendsScaffoldView(state:) 子视图.
    let friendsViewModel: FriendsViewModel
    /// Story 37.11 AC5：接收 RootView 的 ProfileViewModel，注入到 .ready 子树 environmentObject.
    /// ProfileView 通过 @EnvironmentObject var profileViewModel: ProfileViewModel 取出后传给
    /// ProfileScaffoldView(state:) 子视图.
    let profileViewModel: ProfileViewModel
    /// Story 37.4 AC3：接收 RootView 的 AppState，注入到 .ready 子树 environmentObject.
    let appState: AppState
    /// Story 37.5 AC8: 接收 RootView 的 ThemeName，转 Theme 实例后注入 .ready 子树 environment(\.theme).
    let currentTheme: ThemeName
    let sessionStore: SessionStore?
    let resetIdentityViewModel: ResetIdentityViewModel?
    let onReadyAppear: () -> Void
    let onReadyTask: () async -> Void

    init(
        stateMachine: AppLaunchStateMachine,
        coordinator: AppCoordinator,
        homeViewModel: HomeViewModel,
        roomViewModel: RoomViewModel,
        wardrobeViewModel: WardrobeViewModel,
        friendsViewModel: FriendsViewModel,
        profileViewModel: ProfileViewModel,
        appState: AppState,
        currentTheme: ThemeName,
        sessionStore: SessionStore?,
        resetIdentityViewModel: ResetIdentityViewModel?,
        onReadyAppear: @escaping () -> Void,
        onReadyTask: @escaping () async -> Void = { }
    ) {
        self.stateMachine = stateMachine
        self.coordinator = coordinator
        self.homeViewModel = homeViewModel
        self.roomViewModel = roomViewModel
        self.wardrobeViewModel = wardrobeViewModel
        self.friendsViewModel = friendsViewModel
        self.profileViewModel = profileViewModel
        self.appState = appState
        self.currentTheme = currentTheme
        self.sessionStore = sessionStore
        self.resetIdentityViewModel = resetIdentityViewModel
        self.onReadyAppear = onReadyAppear
        self.onReadyTask = onReadyTask
    }

    var body: some View {
        ZStack {
            switch stateMachine.state {
            case .launching:
                LaunchingView()
                    .transition(.opacity)
            case .ready:
                MainTabView()
                    .environmentObject(coordinator)
                    .environmentObject(homeViewModel)
                    .environmentObject(roomViewModel)
                    .environmentObject(wardrobeViewModel)
                    .environmentObject(friendsViewModel)
                    .environmentObject(profileViewModel)
                    .environmentObject(appState)
                    .environment(\.theme, currentTheme.theme)
                    .environment(\.sessionStore, sessionStore)
                    .environment(\.resetIdentityViewModel, resetIdentityViewModel)
                    .onAppear { onReadyAppear() }
                    .task {
                        await onReadyTask()
                    }
                    // codex round 1 [P2] fix: 重新挂回 .fullScreenCover —— ADR-0009 §3.4 SheetType
                    // 白名单仍含 .compose（Story 33.1 落地真实形式）；删 modifier 会让
                    // coordinator.present(.compose) 变 silent no-op.
                    // 详见 docs/lessons/2026-04-30-coordinator-must-mirror-loaded-home-room-state.md.
                    .fullScreenCover(item: $coordinator.presentedSheet) { sheet in
                        switch sheet {
                        case .compose:
                            ComposeSheetPlaceholder()
                        }
                    }
                    .transition(.opacity)
            case .needsAuth(let presentation):
                needsAuthContent(for: presentation)
                    .transition(.opacity)
            }
        }
        .animation(.easeInOut(duration: 0.2), value: stateMachine.state)
    }

    /// Story 5.5 round 8 [P1] fix（终极方案）: bootstrap 路径的 `.alert` / `.toast` 改用
    /// 全新的 `TerminalErrorView` (静态全屏 fallback page，无任何按钮，user 必须主动杀进程退出).
    @ViewBuilder
    private func needsAuthContent(for presentation: ErrorPresentation) -> some View {
        switch presentation {
        case let .retry(message):
            RetryView(
                message: message,
                onRetry: { Task { await stateMachine.retry() } }
            )
        case let .alert(title, message):
            TerminalErrorView(title: title, message: message)
        case let .toast(message):
            TerminalErrorView(title: "提示", message: message)
        }
    }
}

/// Story 37.3 codex round 1 [P2] fix: `.compose` 路由的临时占位 view.
///
/// ADR-0009 §3.4 SheetType 白名单仍保留 `.compose`（Story 33.1 决定具体形式 / 落地真实合成 view）.
/// 在此之前，coordinator.present(.compose) 必须有 view 挂载，否则 state 改了但 UI 不渲染.
///
/// a11y identifier `compose_placeholder` 让 UITest 能验证"present(.compose) 后 sheet 真的弹出".
struct ComposeSheetPlaceholder: View {
    var body: some View {
        Text("compose placeholder")
            .accessibilityIdentifier(AccessibilityID.Compose.placeholder)
    }
}
