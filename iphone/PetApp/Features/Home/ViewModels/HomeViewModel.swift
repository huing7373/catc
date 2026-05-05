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

    // MARK: - Story 8.4 新增字段（PetSpriteView 三态视觉契约 + MotionProvider 订阅链路）

    /// Story 8.4 AC1: 猫 sprite 当前运动状态.
    /// - 订阅来源：通过 `bind(motionProvider:)` wire 后由 MotionProvider.startUpdates 闭包内调
    ///   `MotionStateMapper.map(activity)` 派生.
    /// - 默认值 `.rest`：HomeView 首次渲染时默认显示 idle 动画（AC2 PetSpriteView 三态分支）；
    ///   未授权 / 未 startUpdates 时也保持 .rest（与 8.2 MotionProvider 协议契约对齐：
    ///   "未授权时 startUpdates 不抛错，handler 不被调用即可"）.
    /// - **不**写 AppState：motionState 不是 ADR-0010 §3.2 白名单 7 字段；
    ///   仅作为 ViewModel 瞬时投影（8.4 addendum 钦定）.
    /// - 子类（RealHomeViewModel / MockHomeViewModel）**不** override：基类 @Published 字段已可用;
    ///   override 会破坏 Combine publisher 链路（详 docs/lessons/2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md）.
    @Published public var petState: MotionState = .rest

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

    /// Story 8.4 AC1: 通过 bind(motionProvider:) 注入的 MotionProvider 引用.
    ///
    /// **strong** 持有（与 Story 37.4 weak vs strong lesson 同精神）：
    /// - container.motionProvider 是 AppContainer 单例（与 ViewModel 同生命周期，不形成循环）.
    /// - bind 后 ViewModel 反向不持 motionProvider —— motionProvider 通过 @Sendable closure
    ///   弱捕获 self（[weak self]）；ViewModel deinit 时主动 stopUpdates 让 closure 取消 subscription.
    /// 详见 docs/lessons/2026-04-30-strong-vs-weak-for-constructor-injected-state.md.
    private var motionProvider: MotionProvider?

    /// Story 8.4 review round 1 P1 引入：是否已成功调过 motionProvider.startUpdates.
    /// guard 让"已经 startUpdates 之后的 rebind"短路（防 8.6 授权后多次 rebind 重复订阅 / 双倍事件）.
    /// 与 motionProvider 字段分离的理由：bind 在未授权时仍会存 motionProvider 引用（仅不 startUpdates），
    /// 单一字段无法区分"未存"与"已存但未订阅"两态. 详见 docs/lessons/2026-05-04-motion-bind-must-gate-on-authorization-status.md.
    private var hasStartedMotionUpdates: Bool = false

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

    /// Story 8.4 AC1: 绑定 MotionProvider（与 bind(pingUseCase:) / bind(loadHomeUseCase:) /
    ///                                   bind(appState:) 同模式；但 review round 1 P1 后**允许 rebind**
    ///                                   场景——授权 flow 在 8.5 / 8.6 走完后调用方需再调一次 bind
    ///                                   让未授权 first-launch path 升级到 authorized startUpdates）.
    ///
    /// 行为（review round 1 P1 + round 4 P2 修订；详见
    /// docs/lessons/2026-05-04-motion-bind-must-gate-on-authorization-status.md +
    /// docs/lessons/2026-05-04-auth-gated-subscription-must-handle-downgrade.md）：
    ///   1. **存引用 once**：仅当 self.motionProvider 为 nil 时写入；防多个不同 MotionProvider
    ///      实例混入引用造成 deinit stopUpdates 漏（上层按约定每个 ViewModel 只配一个 provider）.
    ///   2. **每次都查 authorizationStatus**（**不**用 hasStartedMotionUpdates 做单向短路；round 4 P2 修复）.
    ///      根据"当前订阅状态 × 当前授权状态"四象限决定 action：
    ///      | currentlySubscribed | newAuthStatus     | action                                            |
    ///      |---------------------|-------------------|---------------------------------------------------|
    ///      | false               | .authorized       | startUpdates + hasStartedMotionUpdates = true     |
    ///      | false               | not authorized    | noop（仅持引用，等 8.5 / 8.6 授权后再 rebind）    |
    ///      | true                | .authorized       | noop（idempotent；防重复订阅 / 双倍事件）         |
    ///      | true                | not authorized    | stopUpdates + hasStartedMotionUpdates = false +   |
    ///      |                     |                   | petState = .rest（**downgrade 路径**：让 UI 立即  |
    ///      |                     |                   | 回到 rest，不卡 stale .walk / .run）              |
    ///   3. handler 内部：mapper.map 同步派生 → Task { @MainActor in self?.petState = mapped }
    ///      ── @Sendable closure 捕获 [weak self] 防循环；写 @Published 必须在 main actor.
    ///   4. 不调 requestPermission（红线：权限申请按 AR17 / 节点 3 设计交给 8.5 / 8.6 统一处理）.
    ///
    /// 红线：
    ///   - **不**调 motionProvider.requestPermission（权限申请由 8.5 / 8.6 / 8.1 统一管，
    ///     避免本 story 触发 NSMotionUsageDescription 权限弹窗破坏 first-launch UX + UITest 阻塞）.
    ///   - **更不**在 .notDetermined 下直接 startUpdates —— `CMMotionActivityManager.startActivityUpdates`
    ///     在未授权下会触发系统权限弹窗，与"权限按需"红线冲突. round 1 P1 codex 抓到的就是这条.
    ///   - **不**用 hasStartedMotionUpdates 短路 rebind（round 4 P2 修复）：用户中途去 Settings
    ///     撤销权限后，RootView 在 ScenePhase active 触发 rebind；如果 bind 直接 hasStartedMotionUpdates
    ///     短路 return，老订阅永不拆 + petState 卡在 stale .walk / .run，UI 永远错位直到重启 app.
    ///     现在每次 bind 都重新查 authorizationStatus，downgrade 路径下显式 stopUpdates + reset 到 rest.
    ///   - **不**在 closure 内做长耗时操作（按 8.2 lesson `motion-handler-invoke-must-be-in-lock.md`
    ///     钦定：handler 必须轻量同步 mutate @Published / @State；YAGNI 不引入 throttle / debounce —
    ///     由 mapper / ViewModel 配合做 hysteresis 是未来扩展点，不在本 story 范围）.
    public func bind(motionProvider: MotionProvider) {
        // 第一次 bind：存引用. 后续重 bind（同 instance）走 idempotent 升级 / downgrade 路径——
        // 不覆盖既有引用（避免 deinit 时 stopUpdates 拆错对象）.
        if self.motionProvider == nil {
            self.motionProvider = motionProvider
        }

        // round 4 P2 fix：**每次都查** authorizationStatus，按"当前订阅状态 × 当前授权状态"四象限决策——
        // 不再用 hasStartedMotionUpdates 单向短路 rebind，否则 mid-session permission revoke
        // 后老订阅不拆 + petState 卡 stale .walk / .run.
        let status = motionProvider.authorizationStatus()
        let isAuthorized = (status == .authorized)

        switch (hasStartedMotionUpdates, isAuthorized) {
        case (false, true):
            // 未订阅 + 已授权 → 升级到 startUpdates（生产路径 / first-launch 授权后 rebind）.
            hasStartedMotionUpdates = true
            motionProvider.startUpdates { [weak self] activity in
                // closure 在 OperationQueue.main 触发（8.2 MotionProviderImpl 钦定）；
                // mapper.map 是 pure function 同步调用；写 @Published 必须 main actor.
                let mapped = MotionStateMapper.map(activity)
                Task { @MainActor in
                    self?.petState = mapped
                }
            }
        case (false, false):
            // 未订阅 + 未授权 → 仅持引用 return（first-launch 未授权 path；等 8.5 / 8.6 授权后再 bind 一次）.
            return
        case (true, true):
            // 已订阅 + 已授权 → idempotent noop（防重复订阅 / 双倍事件；包含 RootView 在 ScenePhase
            // active 的 rebind 但权限未变化的常态路径）.
            return
        case (true, false):
            // **downgrade 路径**（round 4 P2 修复的核心）：用户去 Settings 撤销权限后，RootView 触发
            // rebind 时落到这里. 必须主动：
            //   ① stopUpdates 拆掉老订阅（虽然系统在权限拒后已不 deliver 事件，但 generation token
            //      推进 + handler 释放是干净路径，避免 in-flight callback 还能 hit 老 closure）；
            //   ② hasStartedMotionUpdates = false（让后续 re-grant 后 rebind 能升级回 authorized 路径）；
            //   ③ petState = .rest 显式回 baseline——MotionProviderImpl generation token 已能滤掉 stale
            //      callback（详见 docs/lessons/2026-05-04-motion-stop-restart-stale-callback-race.md），
            //      但 UI 端 stale 值是已经 set 到 @Published 的，必须主动 reset 才看得到立即更新.
            motionProvider.stopUpdates()
            hasStartedMotionUpdates = false
            petState = .rest
            return
        }
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

    // MARK: - Story 8.4 AC1: deinit 取消 MotionProvider 订阅（防泄漏）

    /// Story 8.4 AC1: ViewModel deinit 时取消 MotionProvider 订阅（防泄漏，对应 epics.md AC 行 1547）.
    ///
    /// SwiftUI @StateObject ViewModel 的生命周期：
    ///   - RootView @StateObject 持 HomeViewModel 整个 App 生命；正常情况不会 deinit.
    ///   - 但**测试场景**（unit test 创建 ViewModel 实例 → 测试结束 ARC 释放）必须 stopUpdates，
    ///     否则 mock motionProvider 仍持 closure → @testable import 不洁 → flaky test.
    ///   - 也防 future scenario（ViewModel 是 short-lived；如未来 child ViewModel 模式）泄漏.
    ///
    /// `deinit` 不能 await / 不能调 @MainActor —— motionProvider.stopUpdates() 协议契约同步签名（无 actor 标记），
    /// 直接调 OK；与 base class 既有 deinit 行为不冲突（HomeViewModel base 之前没有 deinit，子类也没有）.
    deinit {
        motionProvider?.stopUpdates()
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

    /// Story 37.12: JoinRoomModal "确定加入" 按钮 trigger.
    /// MockHomeViewModel: 写 showJoinModal = false（关 modal）+ 记录 invocation 含 roomId.
    /// RealHomeViewModel（本 story 占位）: **本地 mutate** —— 写 showJoinModal = false + 调
    ///   `appState?.setCurrentRoomId(roomId)`（让 sink 派生 RoomScaffoldView 渲染）.
    ///   按 Story 37.9 round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`
    ///   钦定路径：Real 子类 override 必须实装本 story 范围内能让 UI 视觉工作的最小 placeholder 行为；不能只 log.
    /// Story 12.7（节点 4 后）落地真实 JoinRoomUseCase 后改为：
    ///   1) 调 JoinRoomUseCase.execute(roomId:) 拉起 server 加入房间事务
    ///   2) 成功后 server 推送 WS room.snapshot → setCurrentRoomId 由 server 端权威态写入
    ///   3) 失败时弹 ErrorPresenter retry banner（与 LoadHome 失败路径同精神）
    ///   本 story 不实装 server 调用，仅本地 mutate appState 占位.
    public func onJoinRoomConfirm(roomId: String) {
        fatalError("HomeViewModel.onJoinRoomConfirm must be overridden by subclass")
    }
}
