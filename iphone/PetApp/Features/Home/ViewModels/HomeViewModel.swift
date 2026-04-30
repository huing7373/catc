// HomeViewModel.swift
// Story 2.2 占位 ViewModel：
// - 暴露 nickname / appVersion / serverInfo 三个 @Published（hardcode）
//
// Story 2.3 把三个主 CTA closure 替换为 coordinator.present(...) 路由跳转（已落地）.
// Story 37.3 删除（ADR-0009 §3.5 步骤 4）：
//   - 删 onRoomTap / onInventoryTap / onComposeTap 三个 closure 字段 + 三条 init 内的默认参数.
//     主入口 IA 已改 4 Tab + HomeContainerView 互斥状态机 → 这三个 closure 不再有 caller.
//
// Story 2.5 扩展（**追加，不删除老接口**）：
// - 新增 init(pingUseCase:) 重载，按需注入 PingUseCaseProtocol
// - 新增 bind(pingUseCase:) 单次绑定（RootView .task 路径用，规避 @StateObject init 注入限制）
// - 新增 start() async 触发 ping，重复调用通过 pingTask + hasFetched 双层短路
// - 新增 applyPingResult(_:) 三态文案投影（offline / v? / commit）
// - 新增 nonisolated static func readAppVersion() 从 Bundle 读 CFBundleShortVersionString
//
// Story 2.5 review fix round 2（codex P2）：
// - start() 加 `hasFetched` flag，跨"任务完成 → view 重新出现"边界 short-circuit。
//   原因：SwiftUI `.task` 在 view 重新出现时会重启；只用 pingTask 短路无法防 sheet 关闭后重跑。
//   详见 docs/lessons/2026-04-26-swiftui-task-modifier-reentrancy.md。
//
// 设计选择（参照 Story 2.5 Dev Note #2 / #3 / #5）：
// - appVersion 仅存数字部分（如 "1.0.0" 或 "0.0.0"），View 层拼接 "v\(appVersion) · \(serverInfo)"
// - "offline" / "v?" 字符串归 ViewModel（业务投影），"v" 前缀 / "·" 分隔符归 HomeView 模板（UI）
// - 老 init（无 pingUseCase 参数）保留：Preview / Story 2.2/2.3 老测试零改动
//
// Story 5.5 扩展（**追加，不删除老接口**）：
// - 新增 @Published homeData / loadingState 两字段
// - 新增 init(pingUseCase:loadHomeUseCase:errorPresenter:...) 重载，注入 LoadHomeUseCase + ErrorPresenter
// - 新增 bind(loadHomeUseCase:errorPresenter:) 单次绑定（与 bind(pingUseCase:) 同模式）
// - 新增 loadHome() async 入口（启动期 RootView 注入路径调；用户重试时 ErrorPresenter onRetry 闭包调）
// - 新增 applyHomeData(_:) 同步注入路径（让 RootView bootstrapStep1 closure 直接喂数据，
//   避免 ViewModel 自己再调一次 execute 引发双发请求）
// - 新增 applyHomeError(_:) + resetLoadHomeForRetry() 失败/重试支持
// - 新增 HomeLoadingState enum 描述加载状态
//
// 失败也置 hasLoadedHome=true（与 hasFetched 同模式）：避免 server 不可达时反复重试；
// 用户主动重试通过 resetLoadHomeForRetry() 显式重置 + 再调 loadHome()。

import Foundation
import Combine

/// Story 5.5 AC5: HomeViewModel.loadHome 内部加载状态.
/// 让 UI / 测试可断言"未加载 / 加载中 / 已加载 / 失败"四态.
public enum HomeLoadingState: Equatable {
    case idle
    case loading
    case loaded
    case failed(message: String)
}

@MainActor
public final class HomeViewModel: ObservableObject {
    @Published public var nickname: String
    @Published public var appVersion: String
    @Published public var serverInfo: String

    /// Story 5.5 AC5: 首屏数据；启动期 / 重试期间为 nil，加载完成后由 applyHomeData(_:) / loadHome() 写入.
    @Published public var homeData: HomeData?

    /// Story 5.5 AC5: 加载状态（idle/loading/loaded/failed）。Equatable 让测试可断言.
    @Published public var loadingState: HomeLoadingState = .idle

    // Story 37.3 删除（ADR-0009 §3.5 步骤 4）：
    //   onRoomTap / onInventoryTap / onComposeTap 三个 closure 字段已删除.
    //   主入口 IA 改 4 Tab + HomeContainerView 互斥状态机后,这三个 closure 不再有 caller.

    /// 注入的 PingUseCase（init 路径）；新 init 设非 nil，老 init 设 nil。
    private let pingUseCase: PingUseCaseProtocol?

    /// 通过 `bind(pingUseCase:)` 注入的 PingUseCase（运行时注入路径，RootView .task 用）。
    /// 与 `pingUseCase` 互斥使用：start() 优先用 init 路径，未注入时回退到 bind 路径。
    private var boundPingUseCase: PingUseCaseProtocol?

    /// 当前是否有进行中的 ping 任务。再次调用 start() 时短路（防 SwiftUI .task 多次触发引发重复请求）。
    private var pingTask: Task<Void, Never>?

    /// 是否已经成功跑完一次 ping（含失败也置 true）。用于跨"task 完成 → view 重新出现"边界 short-circuit。
    ///
    /// 背景（review fix round 2）：SwiftUI 的 `.task` modifier 在 view 重新出现时（如 `.fullScreenCover`
    /// 关闭后回到 RootView）会重启 task。此时 `pingTask` 已经完成被置 nil，原有的"并发短路"防御不生效，
    /// 每次 sheet 关闭都会重发一次 ping —— 业务上 ping/version 是"App 启动一次性探针"，不应在普通导航时重跑。
    /// 加 `hasFetched` flag 让 start() 在已完成一次（无论成功/失败）后直接 return。
    /// 失败也置 true 的理由：避免 server 不可达时反复重试；错误恢复 UI 是 Story 2.6 的责任。
    /// 详见 docs/lessons/2026-04-26-swiftui-task-modifier-reentrancy.md。
    private var hasFetched: Bool = false

    /// Story 5.5: 注入的 LoadHomeUseCase（init 路径）；老 init / Story 2.5 init 设 nil.
    private let loadHomeUseCase: LoadHomeUseCaseProtocol?

    /// Story 5.5: 通过 bind(loadHomeUseCase:errorPresenter:) 注入的 UseCase（RootView .task 路径）.
    private var boundLoadHomeUseCase: LoadHomeUseCaseProtocol?

    /// Story 5.5: 注入的 ErrorPresenter（init 路径）.
    private let errorPresenter: ErrorPresenter?

    /// Story 5.5: 通过 bind(loadHomeUseCase:errorPresenter:) 注入的 ErrorPresenter（RootView .task 路径）.
    /// weak 引用：避免 onRetry closure → ViewModel → boundErrorPresenter strong → ErrorPresenter
    /// 持 closure strong → ViewModel 形成循环；container.errorPresenter 是单例 stable，weak 不会过早释放.
    private weak var boundErrorPresenter: ErrorPresenter?

    /// 跨边界短路 flag（与 hasFetched 同模式）：
    /// 跨"task 完成 → view 重新出现"边界保证 loadHome 一次性；
    /// 失败也置 true，避免反复重试；用户主动重试走 resetLoadHomeForRetry() 显式清.
    private var hasLoadedHome: Bool = false

    /// 当前 loadHome 任务句柄；同 task 边界内并发触发短路.
    private var loadHomeTask: Task<Void, Never>?

    /// 老 init（Story 2.2 / 2.3 路径）：保留 hardcode 默认值，pingUseCase = nil；不破坏老调用方 / Preview。
    /// Story 37.3 删除 onRoomTap / onInventoryTap / onComposeTap 三个 closure 参数（ADR-0009 §3.5 步骤 4）.
    public init(
        nickname: String = "用户1001",
        appVersion: String = "0.0.0",
        serverInfo: String = "----"
    ) {
        self.nickname = nickname
        self.appVersion = appVersion
        self.serverInfo = serverInfo
        self.pingUseCase = nil
        self.loadHomeUseCase = nil
        self.errorPresenter = nil
    }

    /// Story 2.5 新增 init：注入 PingUseCaseProtocol；appVersion 默认从 Bundle 读取。
    /// 在 AppContainer wire 时调用此 init；测试也用此 init 注入 mock UseCase。
    /// Story 37.3 删除 onRoomTap / onInventoryTap / onComposeTap 三个 closure 参数（ADR-0009 §3.5 步骤 4）.
    public init(
        nickname: String = "用户1001",
        pingUseCase: PingUseCaseProtocol,
        appVersion: String = HomeViewModel.readAppVersion(),
        serverInfo: String = "----"
    ) {
        self.nickname = nickname
        self.appVersion = appVersion
        self.serverInfo = serverInfo
        self.pingUseCase = pingUseCase
        self.loadHomeUseCase = nil
        self.errorPresenter = nil
    }

    /// Story 5.5 新增 init：注入 LoadHomeUseCase + ErrorPresenter（与 Story 2.5 init 并存）。
    /// 测试场景用此 init 直接注入 mock UseCase + 真 ErrorPresenter；生产路径走 bind() 注入.
    /// Story 37.3 删除 onRoomTap / onInventoryTap / onComposeTap 三个 closure 参数（ADR-0009 §3.5 步骤 4）.
    public init(
        nickname: String = "用户1001",
        pingUseCase: PingUseCaseProtocol? = nil,
        loadHomeUseCase: LoadHomeUseCaseProtocol,
        errorPresenter: ErrorPresenter,
        appVersion: String = HomeViewModel.readAppVersion(),
        serverInfo: String = "----"
    ) {
        self.nickname = nickname
        self.appVersion = appVersion
        self.serverInfo = serverInfo
        self.pingUseCase = pingUseCase
        self.loadHomeUseCase = loadHomeUseCase
        self.errorPresenter = errorPresenter
    }

    /// 单次绑定 PingUseCase。多次调用时仅第一次生效（后续静默 noop）。
    /// 与 init(pingUseCase:) 互斥使用：测试场景用 init 注入，production 路径（RootView.task）用 bind 注入。
    ///
    /// 防重入理由：SwiftUI `.task` 在 view 重新出现时可能多次触发；防止第二次调用覆盖第一次的注入实例。
    ///
    /// 副作用：第一次注入时也同步更新 `appVersion` = `readAppVersion()`。原因：RootView 走 `HomeViewModel()`
    /// 老 init（@StateObject 不能在 init 阶段读 Bundle，避免初始化期注入陷阱），appVersion 会停在 "0.0.0"
    /// 默认值；只有 bind() 时才真正能拿到 PingUseCase 与 production 上下文，此时同步刷一遍 bundle version
    /// 才能让 production 显示 v1.0.0 而不是 v0.0.0。详见 docs/lessons/2026-04-26-stateobject-init-vs-bind-injection.md。
    public func bind(pingUseCase: PingUseCaseProtocol) {
        guard self.boundPingUseCase == nil else { return }
        self.boundPingUseCase = pingUseCase
        self.appVersion = HomeViewModel.readAppVersion()
    }

    /// 触发 ping。**三层**短路:
    ///   1. 已成功跑完一次（hasFetched=true）→ 直接 return（防 SwiftUI .task 在 view 重新出现时重跑）。
    ///   2. 进行中的任务（pingTask 非 nil）→ 直接 return（防并发触发同时调两次）。
    ///   3. 未注入 UseCase → no-op。
    ///
    /// **Story 5.5 round 4 [P2] fix**: 移除 round 3 引入的 "hasLoadedHome → 短路 ping" 第 4 层短路.
    /// 原方案: round 3 把 ping 调用从启动 .task 删掉的同时, 在 start() 加 hasLoadedHome 保险,
    /// 让 ping 即使被误调也短路. 但 round 4 codex 发现 round 3 删除得过死, ping 永远没人调,
    /// serverInfo 永远是 "----" placeholder, footer regress. round 4 把 start() 调用挪到 RootView
    /// LaunchedContentView .ready 分支的 onReadyTask, 此时 LoadHome 已成功 → 调 start() 必须真正发 ping
    /// 来填充 serverInfo, 不能再短路. cold-start HTTP 预算 (≤2) 仍保持: ping 在首屏渲染**之后**才发,
    /// 不计入启动链路, 是用户已经看到主界面后才悄悄填的版本号.
    ///
    /// RootView 在 LaunchedContentView .ready 分支的 .task 中调用，App 启动后异步执行一次（成功/失败都只一次）。
    /// 详见 docs/lessons/2026-04-27-bootstrap-all-error-paths-route-via-mapper.md.
    public func start() async {
        // 取注入实例（init 路径优先，回退 bind 路径）。
        let useCase = pingUseCase ?? boundPingUseCase
        guard let useCase = useCase else { return }   // 未注入：no-op
        guard !hasFetched else { return }              // 已完成一轮：短路（跨 task 边界）
        guard pingTask == nil else { return }          // 已有进行中任务：短路（同一 task 边界内并发）

        let task = Task { [weak self] in
            let result = await useCase.execute()
            await self?.applyPingResult(result)
        }
        pingTask = task
        await task.value
        pingTask = nil
        hasFetched = true   // 失败也置 true：避免不可达 server 时反复重试；错误恢复 UI 见 Story 2.6
    }

    /// 把 PingResult 投影成 serverInfo 文案。
    /// 三态：
    ///   - reachable=false → "offline"
    ///   - reachable=true + commit 非空 → commit 短哈希
    ///   - reachable=true + commit nil/空 → "v?"（部分降级）
    private func applyPingResult(_ result: PingResult) {
        if !result.reachable {
            self.serverInfo = "offline"
        } else if let commit = result.serverCommit, !commit.isEmpty {
            self.serverInfo = commit
        } else {
            self.serverInfo = "v?"
        }
    }

    /// 从 main bundle 读 `CFBundleShortVersionString`；缺省 `"0.0.0"`（与现有 hardcode 默认一致）。
    /// nonisolated 让它能在 `MainActor` init 默认参数位置安全调用（默认参数表达式在 actor isolation 之外评估）。
    public nonisolated static func readAppVersion() -> String {
        (Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String) ?? "0.0.0"
    }

    // MARK: - Story 5.5: LoadHome 入口 / 数据投影 / 错误处理

    /// Story 5.5: 单次绑定 LoadHomeUseCase + ErrorPresenter（与 bind(pingUseCase:) 同模式）。
    /// 多次调用时仅第一次生效（防 SwiftUI .task 多次触发覆盖 first 注入）.
    public func bind(loadHomeUseCase: LoadHomeUseCaseProtocol, errorPresenter: ErrorPresenter) {
        guard self.boundLoadHomeUseCase == nil else { return }
        self.boundLoadHomeUseCase = loadHomeUseCase
        self.boundErrorPresenter = errorPresenter
    }

    /// Story 5.5: LoadHome 入口（启动时由 RootView bootstrapStep1 注入路径调；
    /// 用户重试时由 ErrorPresenter onRetry 闭包调）.
    ///
    /// 三层短路（与 start() 同模式）：
    ///   1. hasLoadedHome → 跨 task 边界短路（防 SwiftUI .task 重启重发请求）
    ///   2. loadHomeTask 非 nil → 同 task 边界并发短路
    ///   3. UseCase 未注入 → no-op
    ///
    /// 失败也置 hasLoadedHome=true：避免 server 不可达时反复重试；
    /// 用户主动重试通过 resetLoadHomeForRetry() + 再调 loadHome() 显式重入。
    public func loadHome() async {
        let useCase = loadHomeUseCase ?? boundLoadHomeUseCase
        guard let useCase = useCase else { return }
        guard !hasLoadedHome else { return }
        guard loadHomeTask == nil else { return }

        loadingState = .loading
        let task = Task { [weak self] in
            do {
                let data = try await useCase.execute()
                await self?.applyHomeData(data)
            } catch {
                await self?.applyHomeError(error)
            }
        }
        loadHomeTask = task
        await task.value
        loadHomeTask = nil
    }

    /// Story 5.5: 同步注入已经拿到的 HomeData（让 RootView bootstrapStep1 closure 可直接喂数据，
    /// 避免 ViewModel 自己再调一次 execute() 引发双发请求）.
    /// 同步置 hasLoadedHome=true → 后续 ViewModel.loadHome() 路径走短路.
    public func applyHomeData(_ data: HomeData) {
        self.homeData = data
        self.loadingState = .loaded
        self.hasLoadedHome = true
    }

    /// Story 5.5: 失败路径处理 —— 写 loadingState + 调 ErrorPresenter 弹 RetryView；
    /// onRetry 闭包让用户主动重试（清 hasLoadedHome flag → 再调 loadHome()）.
    /// internal access 让单测可独立验证错误投影（不依赖 mock 触发 loadHome 全流程）.
    func applyHomeError(_ error: Error) {
        self.loadingState = .failed(message: errorMessageFor(error))
        self.hasLoadedHome = true   // 失败也置 true，避免反复重试；用户走 onRetry 显式重置

        // 优先用 bound（生产路径），fallback init 路径（测试场景）
        let presenter = boundErrorPresenter ?? errorPresenter
        presenter?.present(error, onRetry: { [weak self, weak presenter] in
            // onRetry 闭包：清短路 flag → 再调 loadHome 触发 UseCase.execute().
            // [weak self, weak presenter]：closure 本身被 ErrorPresenter 内部 strong 持有，
            // 防 ViewModel ↔ ErrorPresenter 循环引用（presenter 又 strong 持 self）.
            Task { @MainActor in
                _ = presenter   // 显式 capture 防编译器优化掉
                self?.resetLoadHomeForRetry()
                await self?.loadHome()
            }
        })
    }

    /// Story 5.5: 重试入口 —— 清 hasLoadedHome 让 loadHome() 可再跑一次.
    /// 由 ErrorPresenter onRetry 闭包驱动；测试场景也可直接调.
    public func resetLoadHomeForRetry() {
        hasLoadedHome = false
        loadingState = .idle
    }

    /// 错误转文案（短期为 LocalizedError? errorDescription : defaultMessage；
    /// 与 AppLaunchStateMachine.messageFor 同精神，避免 generic 系统串）.
    private func errorMessageFor(_ error: Error) -> String {
        if let localized = error as? LocalizedError, let desc = localized.errorDescription, !desc.isEmpty {
            return desc
        }
        return "首屏加载失败，请重试"
    }
}
