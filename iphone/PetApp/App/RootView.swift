// RootView.swift
// Story 2.2 起占位 RootView：渲染 HomeView。
// Story 2.3 起：注入 AppCoordinator，把 HomeView 三个 CTA 闭包连到 coordinator.present(...)，
// 并通过 .fullScreenCover(item:) 弹出对应 Sheet placeholder。
// Story 2.5 起（追加）：
//   - 新增 @StateObject private var container = AppContainer()
//   - 新增 .task：bind PingUseCase 后触发 start() 拉 /ping + /version
// Story 2.9 起（追加）：
//   - 新增 @StateObject private var launchStateMachine = AppLaunchStateMachine()
//   - body 改为 ZStack { switch launchStateMachine.state } 三分支路由 + 每分支 `.transition(.opacity)`：
//     · .launching → LaunchingView
//     · .ready → HomeView 子树（保留 .onAppear / .fullScreenCover / errorPresenter 既有 wire）
//     · .needsAuth(presentation:) → 根据 presentation 三态分发（Story 5.5 round 2 [P1] fix）:
//       - .retry → RetryView（onRetry → Task { await launchStateMachine.retry() }）
//       - .alert → AlertOverlayView（onDismiss → retry()）
//       - .toast → 兜底为 alert（bootstrap 阶段不该派 toast）
//   - 新增独立 .task：await launchStateMachine.bootstrap()（与既有 ping/version .task 并发跑）
//   - .animation(.easeInOut(duration: 0.2), value: state) 200ms 淡入淡出（epics.md AC 钦定）
//   - codex round 1 [P3] fix：原 `Group { switch ... }` 不会让分支切换过渡 —— `.animation(_:value:)`
//     仅动画属性变化；要让分支淡入淡出必须 ZStack 容器 + 每分支 `.transition(.opacity)`。
//     详见 docs/lessons/2026-04-26-swiftui-switch-transition-explicit.md。
//
// 设计选择：
//   - 四个 @StateObject（coordinator + container + homeViewModel + launchStateMachine），都在 RootView 持有生命周期。
//   - homeViewModel 仍走老 init（hardcode 默认）：避免 SwiftUI @StateObject init 注入陷阱
//     （详见 Story 2.5 Dev Note #3）；运行时通过 .task 调 bind(pingUseCase:) 注入真实 UseCase。
//   - launchStateMachine 走默认 closure init（占位 `{ }`）—— 本 story 不接 Epic 5 真实 UseCase；
//     Epic 5（Story 5.2 / 5.5）落地时改 wire 模式（参考 RootView.task 内 bind 调用 / @State Optional + .onAppear）。
//   - closure wire 放 .onAppear 而非 init：@StateObject 在 init 阶段未真正初始化，
//     init 写会捕获到错误的实例；.onAppear 时 view 已显示，coordinator 已稳定。
//   - capture list `[coordinator]` 显式声明（防强引用 self；闭包都是值类型，重复赋值仅覆盖）。
//   - resetIdentityViewModel（仅 Debug）走 @State Optional + .onAppear 注入路径，
//     与 container 共享同一 keychainStore instance（codex round 1 [P1] 修复）。
//     详见 docs/lessons/2026-04-26-stateobject-debug-instance-aliasing.md。
//   - .fullScreenCover 挪到 .ready case：launching / needsAuth 状态下 sheet 路由不可用
//     （避免 LaunchingView 显示时用户莫名弹 sheet）。
//   - errorPresentationHost 仍挂在最外层 Group：sheet 子树内 sheetContent(for:) 已重复 attach
//     （lesson 2026-04-26-fullscreencover-isolated-environment.md），本 story 不动该既有方案。

import SwiftUI

struct RootView: View {
    @StateObject private var coordinator = AppCoordinator()
    @StateObject private var container = AppContainer()
    @StateObject private var homeViewModel = HomeViewModel()

    /// Story 2.9 新增 / Story 5.2 升级：启动状态机。
    ///
    /// **Story 5.2 关键改动**：原 `@StateObject private var launchStateMachine = AppLaunchStateMachine()`
    /// 改为 `@State Optional` + `.onAppear` lazy 注入。
    /// 因为 bootstrapStep1 closure 需要捕获 `container.makeGuestLoginUseCase()` —— 但 container 本身
    /// 是 `@StateObject`，init 阶段还没真正实体化（lesson 2026-04-26-stateobject-debug-instance-aliasing.md）。
    /// 与 Story 2.8 `resetIdentityViewModel: ResetIdentityViewModel?` 走同一 lazy 注入模式。
    @State private var launchStateMachine: AppLaunchStateMachine?

    #if DEBUG
    /// Story 2.8: dev "重置身份" 按钮 ViewModel。仅 Debug build 存在；Release build 字段不存在。
    ///
    /// **注入路径**：用 `@State Optional` + `.onAppear` lazy 注入，**不**用 `@StateObject` + init 阶段构造。
    /// 因为 `@StateObject` 必须在 init 阶段给值，但 RootView 的 `container` 同样是 `@StateObject`、
    /// init 期间还**没**真正实体化，无法 `_resetIdentityViewModel = StateObject(...container.make...())`。
    /// 早期实装走 standalone `AppContainer()` bootstrap 喂初值——但那会构造一个**新的** `InMemoryKeychainStore`
    /// 实例，与 `container.keychainStore` 是两个不同的字典，重置按钮调的 `removeAll()` 清的是 bootstrap 那个，
    /// container 实际持有的 keychain 仍残留——UI 显示成功但功能失效（codex round 1 [P1] finding）。
    /// 详见 docs/lessons/2026-04-26-stateobject-debug-instance-aliasing.md。
    @State private var resetIdentityViewModel: ResetIdentityViewModel?
    #endif

    var body: some View {
        // 关键：用 ZStack 而非 Group 包裹 switch 分支，并给每个分支显式 `.transition(.opacity)`。
        // 因为 `.animation(_:value:)` **不**会让 switch 不同分支的插入/删除产生过渡（SwiftUI 文档：
        // 该 modifier 只动画现有 view 的属性变化）；要让分支切换淡入淡出，必须满足两个条件：
        //   1. 容器支持 transition（Group 不支持；ZStack / overlay / 等条件容器才行）
        //   2. 每个分支 view 加 `.transition(.opacity)`
        // codex round 1 [P3] finding 指出"200ms 淡入淡出"实际不生效。
        // 详见 docs/lessons/2026-04-26-swiftui-switch-transition-explicit.md。
        //
        // Story 5.2 升级：launchStateMachine 是 Optional —— body 内三态 switch 包在 `if let` 内。
        // launchStateMachine 在 .onAppear 时 lazy 注入（见 ensureLaunchStateMachineWired）。
        ZStack {
            if let stateMachine = launchStateMachine {
                // 关键：用 LaunchedContentView 子视图包裹三态 switch.
                // 子视图 `@ObservedObject` 订阅 launchStateMachine 的 @Published state，
                // 否则在 RootView 直接 `switch stateMachine.state` 不会触发 SwiftUI 重渲染
                // —— 因为 @State Optional 仅监听 Optional 本身的 nil↔非 nil 变化，
                // 不监听 wrapped class 的 objectWillChange（不像 @StateObject）。
                LaunchedContentView(
                    stateMachine: stateMachine,
                    coordinator: coordinator,
                    homeView: { homeView },
                    onReadyAppear: {
                        wireHomeViewModelClosures()
                        #if DEBUG
                        // lazy 注入：第一次 .onAppear 时从已稳定的 container 拿 keychainStore，
                        // 保证 reset 按钮调的 removeAll() 清的是 container.keychainStore 这同一份。
                        // nil 守卫让 RootView 重建（如旋转 / 离开返回）时不会重新构造覆盖既有 instance。
                        if resetIdentityViewModel == nil {
                            resetIdentityViewModel = container.makeResetIdentityViewModel()
                        }
                        #endif
                    },
                    sheetContent: sheetContent(for:)
                )
            } else {
                // launchStateMachine 还没注入 —— 显示 LaunchingView 兜底（理论上不应出现，
                // 因为 .onAppear 立即注入；保留兜底防 .onAppear 调用前的极短窗口期）.
                LaunchingView().transition(.opacity)
            }

            #if DEBUG
            // Story 5.1 AC5: UITest hook —— XCUITest 通过 launchEnvironment
            // 触发 keychain 写/读，把结果通过 hidden a11y text 暴露给 XCUIApplication 探测。
            // 仅 #if DEBUG 编译；release build 该 view 不存在 → 生产代码零污染。
            // 与 KeychainPersistenceUITests 配合实现"跨 App launch 持久化" 验证（NFR7）。
            KeychainUITestHookView(container: container)
            #endif
        }
        .onAppear {
            // Story 5.2 新增：lazy 注入 launchStateMachine。
            // 这一步必须比 .task 早跑：.onAppear 在 SwiftUI 生命周期中先于 .task 触发；
            // 但 .task 内部仍调一次 ensure 兜底 race。
            ensureLaunchStateMachineWired()
        }
        .task {
            // Story 2.5 既有：bind PingUseCase（仅注入；start() 不再在启动 .task 触发）.
            // bind() 是单次生效（second call 会被 ViewModel guard 短路）.
            //
            // **Story 5.5 round 3 [P1] fix**: 移除原 `await homeViewModel.start()` 调用.
            // 原方案: 启动期独立 .task 调 start() → ping 与 LoadHome 并发发起 → 启动链路 3 个 HTTP
            // (`/auth/guest-login` + `/home` + `/ping`), 违反 Story 5.5 spec line 11 钦定的 "≤2 HTTP".
            // ping 是冗余探针 —— `/home` 成功本身已证明 server reachable + token 有效.
            // 新方案: start() 调用挪到 LaunchedContentView .ready case 内 .task 触发, 此时
            // `homeViewModel.hasLoadedHome=true` → start() 第 4 层短路, 永远不发 ping.
            // 详见 docs/lessons/2026-04-27-cold-start-http-budget-ping-redundant.md.
            homeViewModel.bind(pingUseCase: container.makePingUseCase())
            // Story 5.5 新增：bind LoadHomeUseCase + ErrorPresenter，让 ErrorPresenter onRetry 闭包
            // 能驱动 ViewModel 重试（resetLoadHomeForRetry → loadHome）.
            // 启动期 LoadHome 已通过 bootstrapStep1 closure 调过一次（applyHomeData 同步注入数据 +
            // 置 hasLoadedHome=true），此处的 bind 仅为 onRetry 路径建立 wire；ViewModel.loadHome()
            // 在 hasLoadedHome=true 状态下被短路，不会双发请求.
            homeViewModel.bind(
                loadHomeUseCase: container.makeLoadHomeUseCase(),
                errorPresenter: container.errorPresenter
            )
        }
        .task {
            // Story 2.9 新增 / Story 5.2 升级：跑启动状态机 bootstrap。独立 .task 让两个 await 并发跑（不互相阻塞）。
            // bootstrap() 内部通过 hasBootstrapped flag 防 .task 重启时重复跑 step。
            // 兜底：若 .onAppear 还没跑（理论上不应出现），先 ensure 注入再 bootstrap.
            ensureLaunchStateMachineWired()
            await launchStateMachine?.bootstrap()
        }
        .errorPresentationHost(presenter: container.errorPresenter)
    }

    /// Story 5.2 AC8 新增：lazy 注入 `launchStateMachine`，把 GuestLoginUseCase + SessionStore wire 到 bootstrapStep1.
    ///
    /// 为何 lazy：bootstrapStep1 closure 需要捕获 `container.makeGuestLoginUseCase()` 与 `container.sessionStore`，
    /// 但 container 是 `@StateObject`，init 阶段还没真正实体化（lesson 2026-04-26-stateobject-debug-instance-aliasing.md
    /// + 2026-04-26-stateobject-init-vs-bind-injection.md）。`.onAppear` 时 view 已显示，container 已稳定，
    /// 此刻构造 closure 才能捕获到正确的实例。
    ///
    /// nil 守卫：防 .onAppear / .task 多次触发时重复构造覆盖既有 instance
    /// （lesson 2026-04-26-swiftui-task-modifier-reentrancy.md）。
    /// `bootstrap()` 内部还有 `hasBootstrapped` flag 短路，两层防御。
    ///
    /// **UITest hook（仅 #if DEBUG）**：当 launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] == "1" 时，
    /// bootstrapStep1 退化为 no-op —— 保持 Story 2.9 既有 LaunchingView → HomeView 行为，
    /// 让 HomeUITests / NavigationUITests 不依赖真实 server 即可继续工作.
    /// 与 KeychainUITestHookView 同模式（launchEnvironment hook 仅 Debug 编译；release 路径零污染）.
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
        // Story 5.5 round 3 [P1] fix: 移除 round 2 引入的 GuestLoginCompletionGate actor.
        // 原方案: gate 永久记录 "guest-login 曾经成功" → retry() 重跑闭包时**永远**跳过 useCase.execute(),
        // 让 /home 因 .unauthorized / .missingCredentials 失败时复用同一份坏掉的鉴权状态死循环重试,
        // 用户只能看到同一失败结果直到重启 App. round 2 P2 试图省一次 /auth/guest-login 往返,
        // 但代价是把"重试可恢复"语义变成"重试不可恢复"—— 不可接受的 trade-off.
        //
        // 新方案 (fail-safe): retry() 重跑闭包 = guest-login + load-home 都重跑一次.
        // GuestLoginUseCase.execute() 本身幂等（每次产生新 token 写 keychain）, 重复调用无副作用,
        // 反而能保证 retry 时一定有新鲜 token. round 2 P2 的"省一次往返"成本 (~50ms) 可接受,
        // 换来"重试一定能自愈坏掉的鉴权状态"的语义保证.
        // 详见 docs/lessons/2026-04-27-bootstrap-retry-must-not-skip-auth.md.
        launchStateMachine = AppLaunchStateMachine(
            bootstrapStep1: { @Sendable in
                // Step 1a: 游客登录 —— 写 keychain token + sessionStore.user/pet.
                // 每次 bootstrap / retry 都跑一次, 保证坏掉的鉴权状态可被 retry 刷新.
                let output = try await useCase.execute()
                await MainActor.run {
                    sessionStore.updateSession(SessionState(user: output.user, pet: output.pet))
                }
                // Story 5.5 AC6 Step 1b: 串行调 GET /home 拿首屏数据 → applyHomeData 同步注入
                // 任一步抛错 → step1 整体失败 → 状态机走 .needsAuth(presentation:) → 显示对应错误 UI
                //
                // 串行而非并行：LoadHome 需要 token（GuestLogin 写 keychain 完成后才有）；
                // 并行会引发 LoadHome 先发 → 401 → 静默重登 → 复杂边界.
                //
                // 不用 bootstrapStep2 插槽：让 GuestLogin + LoadHome 是单次原子启动事务的失败语义；
                // 用户重试 → 整个 step1 closure 重跑（但 guest-login 由 gate 短路, 实际只重跑 loadHome）.
                // 详见 Story 5.5 Dev Note #2.
                //
                // **错误映射**（Story 5.5 round 1 [P2] + round 2 [P1] fix）: LoadHome 失败时
                // 调 AppErrorMapper.presentation(for:) 派出对应 ErrorPresentation（alert / retry / toast）,
                // 再用 BootstrapMappedError 包装抛出 —— 状态机直接接 presentation 路由, 不再降级为
                // 单一 retry. 防 .unauthorized / .missingCredentials / .decoding 等 mapper 钦定为
                // .alert 的错误被错误降级为 RetryView, 让用户卡在 unrecoverable retry loop.
                // 详见 docs/lessons/2026-04-27-launch-state-machine-must-carry-presentation.md.
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
                    homeViewModel.applyHomeData(homeData)
                }
            }
            // bootstrapStep2 仍走默认 { } no-op；本 story 故意不用 step2 插槽（Dev Note #2）
        )
    }

    /// Debug build 走带 resetIdentityViewModel 的 init;Release build 走旧 init（按钮不存在）。
    /// resetIdentityViewModel 在 .onAppear 之前为 nil；HomeView 已支持 Optional（按钮在 nil 时不渲染），
    /// 短暂的 nil 期对 UX 无影响（dev 工具，非 release 关键路径）。
    ///
    /// Story 5.2 codex round 1 [P1] fix：两条 init 都传 `sessionStore`，让 HomeView 订阅
    /// `SessionStore.@Published session`，bootstrapStep1 写入后 nickname 立刻刷新到真实身份.
    /// 详见 docs/lessons/2026-04-27-sessionstore-home-nickname-source-of-truth.md.
    @ViewBuilder
    private var homeView: some View {
        #if DEBUG
        HomeView(
            viewModel: homeViewModel,
            resetIdentityViewModel: resetIdentityViewModel,
            sessionStore: container.sessionStore
        )
        #else
        HomeView(
            viewModel: homeViewModel,
            resetIdentityViewModel: nil,
            sessionStore: container.sessionStore
        )
        #endif
    }

    /// 把 HomeViewModel 三个 CTA 闭包接到 coordinator.present(...)。
    /// .onAppear 时机重新 wire 一次（防止 RootView 重新构建后失去引用），
    /// 不重复注册不会导致 leak —— 闭包都是值类型，每次赋值覆盖前一个。
    private func wireHomeViewModelClosures() {
        homeViewModel.onRoomTap = { [coordinator] in
            coordinator.present(.room)
        }
        homeViewModel.onInventoryTap = { [coordinator] in
            coordinator.present(.inventory)
        }
        homeViewModel.onComposeTap = { [coordinator] in
            coordinator.present(.compose)
        }
    }

    /// sheet 内容也挂一份 errorPresentationHost：
    /// SwiftUI 的 `.fullScreenCover` 在独立 window scene 渲染，外层 modifier 链不传播到 sheet 子树；
    /// 主 host（在 RootView body 末尾）会被 sheet 整片盖住，导致 sheet 打开时全局错误 UI 隐形。
    /// 让 sheet 子树共用同一个 ErrorPresenter 实例 → presenter.current 是 @Published source of truth →
    /// 两个 host 都监听同一份状态、同步渲染 → sheet 内的 host 渲染在 sheet 顶层，错误 UI 始终可见。
    /// 详见 docs/lessons/2026-04-26-fullscreencover-isolated-environment.md（codex round 1 [P1] finding 修复）。
    @ViewBuilder
    private func sheetContent(for sheet: SheetType) -> some View {
        Group {
            switch sheet {
            case .room:
                RoomPlaceholderView(onClose: { coordinator.dismiss() })
            case .inventory:
                InventoryPlaceholderView(onClose: { coordinator.dismiss() })
            case .compose:
                ComposePlaceholderView(onClose: { coordinator.dismiss() })
            }
        }
        .errorPresentationHost(presenter: container.errorPresenter)
    }
}

// Story 5.5 round 3 [P1] fix: 移除原 GuestLoginCompletionGate actor.
// round 2 [P2] 引入 gate 短路 guest-login 重跑, 但让 .unauthorized / .missingCredentials 失败死循环
// (gate 永久记录 → retry 跳过 useCase.execute() → 复用同一份坏掉的鉴权状态). round 3 [P1] 改回
// "retry 时 guest-login 也重跑" 的 fail-safe 路径; useCase.execute() 幂等, 重复调一次无副作用,
// 多 ~50ms 一次往返的成本远小于"重试不可恢复"风险.
// 详见 docs/lessons/2026-04-27-bootstrap-retry-must-not-skip-auth.md.

/// Story 5.5 codex round 1 [P2] fix + round 2 [P1] fix: 把 bootstrap step closure 内的失败
/// 包装成携带完整 ErrorPresentation 语义的 LocalizedError, 让状态机决定 retry vs alert vs toast.
///
/// **为什么单独一个 wrapper 类型**:
/// - AppLaunchStateMachine.presentationFor 优先识别本类型 → 直接用其 presentation 字段（不重做判断）.
/// - APIError 直接抛给状态机的话, 状态机只能取 errorDescription 做 fallback, 损失 alert/retry 区分.
/// - 在 bootstrap 边界做一层 wrapper：内部 underlying 保留原 APIError 给 log / 上报,
///   外层 presentation 由 AppErrorMapper 单一决定 → 状态机收到的就是"该弹 alert 还是 retry".
///
/// **round 2 [P1] fix**: 字段从 `userFacingMessage: String` 升级为 `presentation: ErrorPresentation`.
/// 原方案只携带 message → 状态机只能塞进 .needsAuth(message:) → 渲染层只看 message,
/// 永远走 RetryView, 把 .unauthorized / .missingCredentials / .decoding 这些 AppErrorMapper 钦定
/// 为 .alert（带"请重启应用" guidance）的错误降级为 retry, 用户卡在 unrecoverable retry loop.
/// 升级后 RootView LaunchedContentView 根据 presentation 三态分发：
/// - .alert → AlertOverlayView（"请重启应用" 不给重试按钮的 blocking alert）
/// - .retry → RetryView（用户点重试 → stateMachine.retry()）
/// - .toast → AlertOverlayView 兜底（toast 是非 modal 的轻量提示, bootstrap 阶段不该用 → 兜底为 alert）
///
/// LocalizedError conformance 保留：让 errorDescription 仍可读（log / 调试），但状态机走 presentation 路径,
/// 不再依赖 errorDescription 做 UI 决策.
///
/// internal access：让 AppLaunchStateMachineTests / RootView 集成测试能直接构造断言.
struct BootstrapMappedError: LocalizedError {
    let presentation: ErrorPresentation
    let underlying: Error

    /// 从 presentation 提取 message 给 LocalizedError conformance（log / 调试用，不影响 UI 决策）.
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
/// 为何抽出来：RootView 的 `launchStateMachine` 字段是 `@State Optional<AppLaunchStateMachine>`
/// （不是 `@StateObject`，因为 closure 必须 lazy 注入捕获 container 字段）.
/// `@State` 仅监听 Optional 自身的 nil↔非 nil 变化，**不**会订阅 wrapped class 的 `objectWillChange`,
/// 所以直接在 RootView body 内 `switch stateMachine.state` 不会随 @Published state 变化重渲染.
/// 解法是把 stateMachine 作为参数传给子视图，子视图标 `@ObservedObject` 让 SwiftUI 重新接 publisher,
/// state 变化时重渲染子视图，三态 switch 才能正常切换.
///
/// 参考精神：与 Story 2.8 `HomeView` 接收 `resetIdentityViewModel: ResetIdentityViewModel?` 然后
/// 内部用 `@ObservedObject` 订阅同模式.
private struct LaunchedContentView: View {
    @ObservedObject var stateMachine: AppLaunchStateMachine
    @ObservedObject var coordinator: AppCoordinator
    let homeView: () -> AnyView
    let onReadyAppear: () -> Void
    let sheetContent: (SheetType) -> AnyView

    init(
        stateMachine: AppLaunchStateMachine,
        coordinator: AppCoordinator,
        @ViewBuilder homeView: @escaping () -> some View,
        onReadyAppear: @escaping () -> Void,
        @ViewBuilder sheetContent: @escaping (SheetType) -> some View
    ) {
        self.stateMachine = stateMachine
        self.coordinator = coordinator
        self.homeView = { AnyView(homeView()) }
        self.onReadyAppear = onReadyAppear
        self.sheetContent = { AnyView(sheetContent($0)) }
    }

    var body: some View {
        // 三态 switch + 每分支显式 `.transition(.opacity)`（lesson 2026-04-26-swiftui-switch-transition-explicit.md）
        // .needsAuth 内部根据 presentation 类型再分三态（Story 5.5 round 2 [P1] fix）.
        ZStack {
            switch stateMachine.state {
            case .launching:
                LaunchingView()
                    .transition(.opacity)
            case .ready:
                homeView()
                    .onAppear { onReadyAppear() }
                    .fullScreenCover(item: $coordinator.presentedSheet) { sheet in
                        sheetContent(sheet)
                    }
                    .transition(.opacity)
            case .needsAuth(let presentation):
                needsAuthContent(for: presentation)
                    .transition(.opacity)
            }
        }
        .animation(.easeInOut(duration: 0.2), value: stateMachine.state)
    }

    /// Story 5.5 round 2 [P1] fix: 根据 presentation 三态分发错误 UI.
    ///
    /// - `.retry`: 用户可点重试触发 stateMachine.retry()（继续走 GuestLogin + LoadHome 重试链路）.
    /// - `.alert`: 用户必须重启 App（mapper 已钦定文案 "请重启应用" / "请重新启动应用"）.
    ///   onDismiss 仍调 retry() —— 让用户在 alert 关闭后**也**有重试入口（不能让 alert 关闭就死锁
    ///   在白屏；retry 内部 isRetrying guard 防重入）.
    /// - `.toast`: bootstrap 阶段不应该派发 toast（toast 是非 modal 的轻量提示；启动失败必须 modal）.
    ///   出现说明 mapper 配置异常 → 兜底渲染为 alert 而非 toast,避免 toast 自动消失后留白屏.
    @ViewBuilder
    private func needsAuthContent(for presentation: ErrorPresentation) -> some View {
        switch presentation {
        case let .retry(message):
            RetryView(
                message: message,
                onRetry: { Task { await stateMachine.retry() } }
            )
        case let .alert(title, message):
            AlertOverlayView(
                title: title,
                message: message,
                onDismiss: { Task { await stateMachine.retry() } }
            )
        case let .toast(message):
            // 兜底：bootstrap 阶段拿到 toast presentation 异常 —— 渲染为 alert 防白屏.
            AlertOverlayView(
                title: "提示",
                message: message,
                onDismiss: { Task { await stateMachine.retry() } }
            )
        }
    }
}
