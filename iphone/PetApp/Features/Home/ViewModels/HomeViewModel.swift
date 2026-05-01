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
//
// Story 37.4 改造（ADR-0010 §3.5 + AC2）：
// - 删除 `@Published homeData: HomeData?` 字段（domain state 单 source of truth 改为 AppState）.
// - 新增 `private weak var appState: AppState?` + `bind(appState:)` 构造注入入口；applyHomeData(_:) 内部
//   把 `self.homeData = data` 改为 `self.appState?.applyHomeData(data)`（写入 AppState 而非自身字段）.
// - `loadingState` / `hasLoadedHome` / `hasFetched` 等 transient flag 仍归 HomeViewModel
//   （ADR-0010 §3.2 表格"Loading / error toast → ViewModel @Published"钦定）.
// - 三个 init 追加 `appState: AppState? = nil` 默认参数（不删除老接口；测试 / Preview 仍走老路径）.
// - applyHomeData(_:) 承担**双重职责**：写 AppState（驱动 UI）+ 设 hasLoadedHome=true（驱动 ViewModel 短路 flag）.
//
// Story 37.7 改造（class 层次重构 + 5 字段 + 5 abstract method）：
// - `final class` → `class`（去 final）让 MockHomeViewModel / RealHomeViewModel 子类可继承.
// - 新增 5 个 @Published 字段：greeting / weather / stats / interactionAnimation / showJoinModal
//   （HomeView 5 区块视觉契约 + Story 12.7 / 21.1 / 37.12 接缝点）.
// - 新增 5 个 abstract method：onCreateTap / onJoinTap / onFeedTap / onPetTap / onPlayTap，
//   基类用 `fatalError("subclass override")` 占位强制子类必 override（漏 override 立刻 crash）.
// - 全部 Story 2.2 / 2.5 / 5.5 / 37.4 老公开 API（init #1/#2/#3 + bind 3 个 + start / loadHome /
//   applyHomeData / resetLoadHomeForRetry / readAppVersion / applyHomeError）签名 / 行为完全不变.

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
public class HomeViewModel: ObservableObject {
    @Published public var nickname: String
    @Published public var appVersion: String
    @Published public var serverInfo: String

    /// Story 5.5 AC5: 加载状态（idle/loading/loaded/failed）。Equatable 让测试可断言.
    /// Story 37.4 AC2 / ADR-0010 §3.2 表格：loading / error toast 仍归 HomeViewModel transient state；
    /// domain state（user/pet/stepAccount/chest）改由 AppState 持有（不再有 self.homeData 字段）.
    @Published public var loadingState: HomeLoadingState = .idle

    // MARK: - Story 37.7 新增字段（HomeView 5 区块视觉契约 + 下游 story 接缝点）

    /// 顶部 statusBar 主标题（home.jsx:34 钦定 22pt 800 weight；mock 默认 "想你啦 ♥"）.
    /// MockHomeViewModel / RealHomeViewModel 子类构造时按需覆写.
    @Published public var greeting: String = "想你啦 ♥"

    /// 顶部 statusBar 副标题（home.jsx:28 钦定 12pt 600 weight；mock 默认 "今天 · 晴"）.
    @Published public var weather: String = "今天 · 晴"

    /// CatStage 三状态条数据（饱食/心情/活力）；mock 默认 .mockHappy.
    /// 节点 8 / 14.x 后 WS pet.state.changed 真实状态切换时再分化.
    @Published public var stats: PetStats = .mockHappy

    /// CatStage floatUp emoji 浮动动画状态（idle 不渲染；flying("🍥"/"💕"/"⭐") 触发 1.4s 浮动消失）.
    /// 由 onFeedTap / onPetTap / onPlayTap 设置；HomeView .onChange 内部 1.4s 后自动重置回 idle.
    @Published public var interactionAnimation: AnimationState = .idle

    /// JoinRoomModal sheet 双向绑定状态（HomeView `.sheet(isPresented: $state.showJoinModal)`）.
    /// onJoinTap 设 true；JoinRoomModalPlaceholder（Story 37.12 真实 modal）dismiss 时 SwiftUI 自动设 false.
    /// 唯一 owner = ViewModel @Published（避免 SwiftUI @State 双写漂移；详见 Dev Notes "showJoinModal 唯一 owner"）.
    @Published public var showJoinModal: Bool = false

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

    /// Story 37.4：通过 `bind(appState:)` 注入或 init 注入的 AppState 引用.
    ///
    /// **strong** 持有（Story 37.4 codex round 1 [P2] fix）：原方案 weak 与 init 参数注入路径不兼容 ——
    /// 若 caller 用 `HomeViewModel(appState: AppState())` 这种 fresh instance 而无外部 strong owner,
    /// weak 会立刻释放,`applyHomeData` / `loadHome` 静默 fail. 改 strong 后:
    /// - 生产路径（RootView @StateObject 同时 strong 持 AppState 与 HomeViewModel）：HomeViewModel 再
    ///   strong 持 AppState **不会形成循环** —— AppState 不反向持 HomeViewModel（已在
    ///   `iphone/PetApp/State/` 下全局 grep 确认）.
    /// - 测试 / Preview / 其他 caller 路径：strong 让 `HomeViewModel(appState: AppState())`
    ///   也能正常工作,符合构造注入语义（ADR-0010 §3.1 "ViewModel 仅允许构造注入 AppState"）.
    /// 详见 docs/lessons/2026-04-30-strong-vs-weak-for-constructor-injected-state.md.
    private var appState: AppState?

    /// 老 init（Story 2.2 / 2.3 路径）：保留 hardcode 默认值，pingUseCase = nil；不破坏老调用方 / Preview。
    /// Story 37.3 删除 onRoomTap / onInventoryTap / onComposeTap 三个 closure 参数（ADR-0009 §3.5 步骤 4）.
    /// Story 37.4 追加 `appState: AppState? = nil` 默认参数（保留老接口；测试 / Preview 不传走 nil 路径）.
    public init(
        nickname: String = "用户1001",
        appVersion: String = "0.0.0",
        serverInfo: String = "----",
        appState: AppState? = nil
    ) {
        self.nickname = nickname
        self.appVersion = appVersion
        self.serverInfo = serverInfo
        self.pingUseCase = nil
        self.loadHomeUseCase = nil
        self.errorPresenter = nil
        self.appState = appState
    }

    /// Story 2.5 新增 init：注入 PingUseCaseProtocol；appVersion 默认从 Bundle 读取。
    /// 在 AppContainer wire 时调用此 init；测试也用此 init 注入 mock UseCase。
    /// Story 37.3 删除 onRoomTap / onInventoryTap / onComposeTap 三个 closure 参数（ADR-0009 §3.5 步骤 4）.
    /// Story 37.4 追加 `appState: AppState? = nil` 默认参数.
    public init(
        nickname: String = "用户1001",
        pingUseCase: PingUseCaseProtocol,
        appVersion: String = HomeViewModel.readAppVersion(),
        serverInfo: String = "----",
        appState: AppState? = nil
    ) {
        self.nickname = nickname
        self.appVersion = appVersion
        self.serverInfo = serverInfo
        self.pingUseCase = pingUseCase
        self.loadHomeUseCase = nil
        self.errorPresenter = nil
        self.appState = appState
    }

    /// Story 5.5 新增 init：注入 LoadHomeUseCase + ErrorPresenter（与 Story 2.5 init 并存）。
    /// 测试场景用此 init 直接注入 mock UseCase + 真 ErrorPresenter；生产路径走 bind() 注入.
    /// Story 37.3 删除 onRoomTap / onInventoryTap / onComposeTap 三个 closure 参数（ADR-0009 §3.5 步骤 4）.
    /// Story 37.4 追加 `appState: AppState? = nil` 默认参数.
    public init(
        nickname: String = "用户1001",
        pingUseCase: PingUseCaseProtocol? = nil,
        loadHomeUseCase: LoadHomeUseCaseProtocol,
        errorPresenter: ErrorPresenter,
        appVersion: String = HomeViewModel.readAppVersion(),
        serverInfo: String = "----",
        appState: AppState? = nil
    ) {
        self.nickname = nickname
        self.appVersion = appVersion
        self.serverInfo = serverInfo
        self.pingUseCase = pingUseCase
        self.loadHomeUseCase = loadHomeUseCase
        self.errorPresenter = errorPresenter
        self.appState = appState
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

    /// Story 37.4: 单次绑定 AppState（构造注入路径），由 RootView .task 内调用.
    /// 多次调用时仅第一次生效（防 SwiftUI .task 多次触发覆盖 first 注入）.
    /// 与 bind(pingUseCase:) lesson 同精神：跨 task 边界注入 AppState 引用，
    /// 让 applyHomeData(_:) 能 propagate 到 AppState（不再写 self.homeData 字段）.
    /// 详见 docs/lessons/2026-04-26-stateobject-init-vs-bind-injection.md.
    public func bind(appState: AppState) {
        guard self.appState == nil else { return }
        self.appState = appState
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
    ///
    /// Story 37.4 改造（ADR-0010 §3.5）：
    /// - domain state（user/pet/stepAccount/chest/currentRoomId）写入 AppState，由 SwiftUI View 通过
    ///   @EnvironmentObject 订阅；不再持自身 homeData 字段.
    /// - 仍保留 `loadingState = .loaded` + `hasLoadedHome = true` —— 这两个 transient flag 归
    ///   HomeViewModel 自己管（ADR-0010 §3.2 表格钦定 loading state 不进 AppState）.
    /// - 双写模式（RootView bootstrap closure 同时调 appState.applyHomeData + homeViewModel.applyHomeData）
    ///   不是反模式：内层这里写的是同一个 AppState 实例 + idempotent 赋值（同值），
    ///   同时让 hasLoadedHome 短路 flag 准确生效.
    public func applyHomeData(_ data: HomeData) {
        self.appState?.applyHomeData(data)
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

    // MARK: - Story 37.7: 5 个 abstract method（基类 fatalError 占位，子类必 override）

    /// 用户点击 TeamIdleCard "创建队伍" 按钮 → MockHomeViewModel 记录 invocation /
    /// RealHomeViewModel Story 12.7 实装 CreateRoomUseCase 调用.
    /// 基类 `fatalError`：漏 override 时立刻 crash（不接受 default empty 实现 silent miss）.
    public func onCreateTap() {
        fatalError("HomeViewModel.onCreateTap must be overridden by subclass")
    }

    /// 用户点击 TeamIdleCard "加入队伍" 按钮 → 子类 override 设 showJoinModal = true 触发 sheet.
    public func onJoinTap() {
        fatalError("HomeViewModel.onJoinTap must be overridden by subclass")
    }

    /// 用户点击 ActionRow "喂食" 按钮 → 子类 override 设 interactionAnimation = .flying(emoji: "🍥", id: UUID()).
    /// Story 37.7 codex round 2 [P2] fix：每次新 UUID 保证连点同 emoji 也触发 onChange 重放动画.
    public func onFeedTap() {
        fatalError("HomeViewModel.onFeedTap must be overridden by subclass")
    }

    /// 用户点击 ActionRow "抚摸" 按钮 → 子类 override 设 interactionAnimation = .flying(emoji: "💕", id: UUID()).
    public func onPetTap() {
        fatalError("HomeViewModel.onPetTap must be overridden by subclass")
    }

    /// 用户点击 ActionRow "玩耍" 按钮 → 子类 override 设 interactionAnimation = .flying(emoji: "⭐", id: UUID()).
    public func onPlayTap() {
        fatalError("HomeViewModel.onPlayTap must be overridden by subclass")
    }
}
