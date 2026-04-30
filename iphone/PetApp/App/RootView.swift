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
    @StateObject private var homeViewModel = HomeViewModel()

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
            ensureLaunchStateMachineWired()
        }
        .task {
            // Story 5.5：bind LoadHomeUseCase + ErrorPresenter，让 ErrorPresenter onRetry 闭包
            // 能驱动 ViewModel 重试.
            homeViewModel.bind(
                loadHomeUseCase: container.makeLoadHomeUseCase(),
                errorPresenter: container.errorPresenter
            )
            // Story 37.4 AC3：注入 AppState，让 HomeViewModel.applyHomeData(_:) 内部写 AppState
            // （不再写自身 homeData 字段；ADR-0010 §3.5 钦定）.
            homeViewModel.bind(appState: appState)
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
            .accessibilityIdentifier("compose_placeholder")
    }
}
