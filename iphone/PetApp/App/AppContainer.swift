// AppContainer.swift
// Story 2.5 AC6：App 全局依赖注入容器（首次落地）。
//
// 职责（本 story 范围）：
// - 持有 APIClient 单例（baseURL 由 init 时注入）
// - 暴露按需构造 UseCase 的工厂方法（如 makePingUseCase()）
//
// 生命周期：由 RootView 通过 `@StateObject private var container = AppContainer()` 持有，
// 与 App scene 同生命周期。当前 MVP 不引入 SceneStorage / AppDelegate 钩子；container 重启 = App 重启。
//
// 后续扩展（→ Epic 4 / 5 / 12+）：
// - 按需追加 KeychainStore / SessionRepository / WebSocketClient 等
// - 业务 UseCase（GuestLoginUseCase / LoadHomeUseCase / OpenChestUseCase 等）按
//   Repository → UseCase 分层在此 wire；本 story 只 wire PingUseCase 作为模板示范。
//
// 不引入第三方 DI 框架（Swinject / Resolver）：
// MVP 阶段 wire 量小（< 20 个对象），手写 init / factory method 就够，避免 DSL 学习成本
// （与 ADR-0002 §3.1 "手写 mock 优于 codegen" 的精神同源）。
//
// import 备注（继承 Story 2.2 lesson 2026-04-25-swift-explicit-import-combine.md）：
// `ObservableObject` 来自 Combine，必须显式 `import Combine`。

import Foundation
import Combine

@MainActor
public final class AppContainer: ObservableObject {
    public let apiClient: APIClientProtocol

    /// Story 2.6 新增：全 App 共享的错误 UI 中心。RootView 通过 `.errorPresentationHost(presenter:)` ViewModifier
    /// 把此实例挂到根视图；后续 Epic 4 GuestLogin / Epic 5 自动登录 / Epic 7+ 业务接口拿同一个实例即可。
    /// 默认 `toastDuration = 2.0`；测试可通过未来追加的 init 重载注入自定义时长（本 story 不预留 YAGNI）。
    public let errorPresenter: ErrorPresenter

    /// Story 2.8 新增 / Story 5.1 升级：KeychainStore 实例。
    /// Story 2.8 默认值为占位 `InMemoryKeychainStore`；Story 5.1 已切换为生产 `KeychainServicesStore`
    /// （基于 Apple Security.framework / kSecClassGenericPassword）。
    /// 协议 `KeychainStoreProtocol` 不变，所有 Story 2.8 UseCase / ViewModel / Mock 测试零回归。
    public let keychainStore: KeychainStoreProtocol

    /// Story 5.2 新增：全 App 共享的会话状态。RootView bootstrapStep1 closure 在登录成功后
    /// 调 sessionStore.updateSession(...) 写入；HomeView / 节点 2 之后的所有需要身份信息的视图通过
    /// @ObservedObject / @EnvironmentObject 订阅 sessionStore.session.
    ///
    /// 与 errorPresenter 同模式 —— stable singleton within container，
    /// 由 init 一次性构造，整个 App 生命周期共享同一 instance.
    public let sessionStore: SessionStore

    /// Info.plist 中存放 baseURL 的 key（约定：`PetAppBaseURL`，避免与 Apple 系统 key 冲突）。
    /// 通过 build configuration / xcconfig 覆盖；缺省时回退到 `localhost` fallback。
    public static let baseURLInfoKey = "PetAppBaseURL"

    /// localhost fallback：仅当 Info.plist 既没配置也读不到时启用。
    /// 注意：真机运行时 localhost 解析为设备自身，仅 simulator 上才能命中 Mac 上的 server；
    /// 真机联调请通过 Info.plist `PetAppBaseURL` 覆盖（详见 lesson 2026-04-26-baseurl-from-info-plist）。
    public static let fallbackBaseURLString = "http://localhost:8080"

    /// ADR-0008 v2 §6 / Story 0008-impl-1 新增：sink for late-bound 401 cold-start handler.
    ///
    /// AuthBoundaryAPIClient 在 401 时调 sink.trigger() —— RootView.ensureLaunchStateMachineWired()
    /// 在创建 AppLaunchStateMachine 后调 sink.setHandler 注入真实闭包
    /// （sessionStore.clear() + stateMachine.triggerColdStart()）.
    ///
    /// 解 chicken-and-egg：container init 时 stateMachine 还不存在；sink 让 handler 可 late-bind.
    public let unauthorizedHandlerSink: UnauthorizedHandlerSink

    /// Story 8.1: HealthProvider 实例.
    /// 节点 3 阶段（Story 8.1～8.5）逐步 wire：
    /// - 8.1（本 story）：仅声明 + init 默认实例化为 HealthProviderImpl；当前无 caller，HomeViewModel 不消费.
    /// - 8.5：StepSyncTriggerService 通过 container.healthProvider 注入；SyncStepsUseCase 调 readDailyTotalSteps.
    /// 测试场景通过未来追加的 init 重载注入 HealthProviderMock（YAGNI：本 story 不预留 init 参数；Story 8.5 落地时再加）.
    public let healthProvider: HealthProvider

    /// Story 8.2: MotionProvider 实例.
    /// 节点 3 阶段（Story 8.1～8.5）逐步 wire：
    /// - 8.2（本 story）：仅声明 + init 默认实例化为 MotionProviderImpl；当前无 caller.
    /// - 8.4：HomeViewModel 通过 container.motionProvider 订阅 startUpdates → 调 8.3 mapper → driving petState.
    /// 测试场景通过未来追加的 init 重载注入 MotionProviderMock（YAGNI：本 story 不预留 init 参数；Story 8.4 落地时再加）.
    public let motionProvider: MotionProvider

    /// Story 8.5 AC8: 时间抽象（dateProvider 默认 DefaultDateProvider；
    /// 测试场景通过 init(apiClient:keychainStore:unauthorizedHandlerSink:dateProvider:) 重载注入 mock）.
    public let dateProvider: DateProvider

    /// Story 12.2 AC7 新增：全 App 共享的 WebSocketClient.
    /// Story 12.7 LeaveRoomUseCase / JoinRoomUseCase 落地后通过 container.webSocketClient 注入到
    /// RealRoomViewModel.bind(appState:webSocketClient:)；本 story 仅就位字段，**不**改 RootView wire
    /// （RootView 仍传 nil；Story 12.7 才把真实 client 注入）.
    ///
    /// 默认 init：实例化 WebSocketClientImpl(baseURL:, tokenProvider:)，
    ///   - baseURL 与 APIClient 共享（resolveDefaultBaseURL，host-only）
    ///   - tokenProvider 闭包从 keychainStore 读 KeychainKey.authToken；与 APIClient 同源 token.
    /// 测试 init 重载允许注入 mock client.
    public let webSocketClient: WebSocketClient

    #if DEBUG
    /// Story 8.5 AC11: UITest 路径下注入的 mock StepRepository（替代默认 DefaultStepRepository）.
    /// 仅 DEBUG 编译；通过 `UITEST_MOCK_STEP_SYNC=1` launch arg 启用.
    /// `makeStepRepository()` 检查此字段；非 nil 时返回 mock，nil 时返回 default.
    private let uiTestMockStepRepository: StepRepositoryProtocol?
    #endif

    /// 默认 init：用 `APIClient(baseURL:, keychainStore:)` 构造默认 client。
    /// baseURL 来源优先级：Info.plist[`PetAppBaseURL`] → fallback `http://localhost:8080`。
    /// 不含 `/api/v1` 前缀（host-only baseURL 决策，见 Story 2.5 Dev Note #1）。
    /// 测试 / 未来环境切换通过 `init(apiClient:keychainStore:)` 重载注入自定义 client / keychain.
    ///
    /// Story 5.3：先 `let store = KeychainServicesStore()` 一次，然后两路注入 ——
    /// APIClient 与 AppContainer 共享同一 keychain instance，避免 APIClient 内自己 new
    /// 一个新 instance 引发未来万一调整 namespace 时双源不一致风险。
    /// GuestLoginUseCase 写 token 后，下一次 APIClient 调 requiresAuth=true 接口
    /// 立刻从同一 keychain 读到（不会因 namespace 不一致或双 instance 而漏读）。
    ///
    /// **ADR-0008 v2 / Story 0008-impl-1**（替代 Story 5.4 silent relogin 三件套）：
    /// `baseAPIClient` 之上包一层 `AuthBoundaryAPIClient` 装饰器 —— 业务层拿到的 apiClient 在
    /// server 401 时自动触发**全局 cold-start**（清 SessionStore + state machine 回 .launching → 重跑 bootstrap），
    /// 不再做 in-app silent relogin（已退役）.
    /// 用户感知：从主屏闪一下 LaunchingView < 1 秒 → 回主屏；keychain guestUid 持久化复用同一身份.
    public convenience init() {
        let baseURL = AppContainer.resolveDefaultBaseURL(from: Bundle.main)
        let keychainStore = KeychainServicesStore()

        // ADR-0008 v2: 先建 baseAPIClient + sink，再用 AuthBoundary 包装.
        // sink 此时持空 handler；RootView 在 stateMachine 创建后注入真实 handler.
        let baseAPIClient = APIClient(baseURL: baseURL, keychainStore: keychainStore)
        let sink = UnauthorizedHandlerSink()
        let wrappedAPIClient = AuthBoundaryAPIClient(inner: baseAPIClient, sink: sink)

        // Story 12.2 AC7：实例化默认 WebSocketClientImpl（与 baseAPIClient 共享 baseURL + keychainStore）.
        // tokenProvider 闭包在 connect 时调；闭包内 strong-capture keychainStore（与 baseAPIClient / AuthBoundary 同源）.
        let wsClient = WebSocketClientImpl(
            baseURL: baseURL,
            tokenProvider: { [keychainStore] in
                return try? keychainStore.get(forKey: KeychainKey.authToken.rawValue)
            }
        )

        #if DEBUG
        // Story 8.5 AC11: UITest 路径走 launch env 注入 mock StepRepository + mock HealthProvider.
        // - UITEST_MOCK_STEP_SYNC=1 启用 mock（同时联动 mock HealthProvider，
        //   避免 sim 上 HealthKit 权限拒绝导致 SyncStepsUseCase 提前抛错→AppState 永不写入）；
        // - UITEST_MOCK_HEALTH_STEPS（可选）：HealthProvider mock 返的当日步数（默认 1234）；
        // - UITEST_MOCK_SYNC_RESPONSE_AVAILABLE（可选）：覆盖 mock 响应的 availableSteps（默认 5678）.
        let env = ProcessInfo.processInfo.environment
        let uitestMockStepRepo: StepRepositoryProtocol?
        let uitestMockHealth: HealthProvider?
        if env["UITEST_MOCK_STEP_SYNC"] == "1" {
            let totalSteps = env["UITEST_MOCK_HEALTH_STEPS"].flatMap(Int.init) ?? 1234
            let availableSteps = env["UITEST_MOCK_SYNC_RESPONSE_AVAILABLE"].flatMap(Int.init) ?? 5678
            uitestMockStepRepo = UITestMockStepRepository(
                stubResponse: StepsSyncResponse(
                    acceptedDeltaSteps: totalSteps,
                    stepAccount: StepAccountInSyncResponse(
                        totalSteps: totalSteps,
                        availableSteps: availableSteps,
                        consumedSteps: 0
                    )
                )
            )
            // HealthProviderMock：让 readDailyTotalSteps 返 totalSteps（任意 day key 均返同值；
            // 详见下方 UITestMockHealthProvider）.
            uitestMockHealth = UITestMockHealthProvider(stubSteps: totalSteps)
        } else {
            uitestMockStepRepo = nil
            uitestMockHealth = nil
        }

        self.init(
            apiClient: wrappedAPIClient,
            keychainStore: keychainStore,
            unauthorizedHandlerSink: sink,
            webSocketClient: wsClient,
            healthProvider: uitestMockHealth,
            uiTestMockStepRepository: uitestMockStepRepo
        )
        #else
        self.init(
            apiClient: wrappedAPIClient,
            keychainStore: keychainStore,
            unauthorizedHandlerSink: sink,
            webSocketClient: wsClient
        )
        #endif
    }

    /// 注入式 init：测试中传 mock APIClient；未来 release build 切到 production baseURL 时也走此入口。
    /// Story 2.8 → Story 5.1 evolution：
    /// - Story 2.8 默认值 `InMemoryKeychainStore()`（占位）
    /// - Story 5.1 默认值切换为 `KeychainServicesStore()`（基于 Apple Security.framework，真实持久化）
    /// 协议 `KeychainStoreProtocol` 四方法签名不变，Story 2.8 既有调用方零改动；
    /// `InMemoryKeychainStore` 作为测试便利 + 模板示范保留（仍由 InMemoryKeychainStoreTests 维护）。
    #if DEBUG
    public init(
        apiClient: APIClientProtocol,
        keychainStore: KeychainStoreProtocol = KeychainServicesStore(),
        unauthorizedHandlerSink: UnauthorizedHandlerSink = UnauthorizedHandlerSink(),
        webSocketClient: WebSocketClient? = nil,
        dateProvider: DateProvider = DefaultDateProvider(),
        healthProvider: HealthProvider? = nil,
        uiTestMockStepRepository: StepRepositoryProtocol? = nil
    ) {
        self.apiClient = apiClient
        self.errorPresenter = ErrorPresenter()
        self.keychainStore = keychainStore
        self.sessionStore = SessionStore()
        self.unauthorizedHandlerSink = unauthorizedHandlerSink
        // Story 12.2 AC7 + fix-review round 1 P2：webSocketClient 默认 nil 时 fallback 到
        //   **apiClient.baseURL**（保 REST + WS 同源），而**不是** Bundle.main 默认值
        //   —— 避免 split-brain（如测试 / Preview / alt-env 注入非默认 apiClient 后，
        //   container.webSocketClient 仍打 localhost）.
        self.webSocketClient = webSocketClient ?? WebSocketClientImpl(
            baseURL: apiClient.baseURL,
            tokenProvider: { [keychainStore] in
                return try? keychainStore.get(forKey: KeychainKey.authToken.rawValue)
            }
        )
        self.healthProvider = healthProvider ?? HealthProviderImpl()
        self.motionProvider = MotionProviderImpl()
        self.dateProvider = dateProvider
        self.uiTestMockStepRepository = uiTestMockStepRepository
    }
    #else
    public init(
        apiClient: APIClientProtocol,
        keychainStore: KeychainStoreProtocol = KeychainServicesStore(),
        unauthorizedHandlerSink: UnauthorizedHandlerSink = UnauthorizedHandlerSink(),
        webSocketClient: WebSocketClient? = nil,
        dateProvider: DateProvider = DefaultDateProvider()
    ) {
        self.apiClient = apiClient
        self.errorPresenter = ErrorPresenter()
        self.keychainStore = keychainStore
        self.sessionStore = SessionStore()
        self.unauthorizedHandlerSink = unauthorizedHandlerSink
        // Story 12.2 AC7 + fix-review round 1 P2：见 DEBUG 分支同段注释.
        self.webSocketClient = webSocketClient ?? WebSocketClientImpl(
            baseURL: apiClient.baseURL,
            tokenProvider: { [keychainStore] in
                return try? keychainStore.get(forKey: KeychainKey.authToken.rawValue)
            }
        )
        self.healthProvider = HealthProviderImpl()
        self.motionProvider = MotionProviderImpl()
        self.dateProvider = dateProvider
    }
    #endif

    /// 解析默认 baseURL：从给定 bundle 的 Info.plist 读 `PetAppBaseURL`，否则回退到 fallback。
    /// 提取为 static + 接受 bundle 参数：方便测试通过 mock bundle / fixture plist 验证读取逻辑。
    ///
    /// 解析失败（key 不存在 / 类型错 / URL 格式错 / scheme 非 http(s) / host 缺失）一律静默回退到
    /// fallback；不抛、不打 log（MVP 阶段保持 init 路径无副作用；future 改进可加 #if DEBUG print）。
    ///
    /// **关于 `URL(string:)` 的宽容性**（codex round 4 [P2] finding）：
    /// `URL(string: "localhost:8080")` 返回 non-nil（Apple URL parser 把它解析成
    /// `scheme=localhost, path=8080`）；`URL(string: "http://")` 也 non-nil 但 host 为 nil。
    /// 这类 malformed 输入若直接进 `APIClient`，`URLRequest` 会构造出无效请求，所有 ping/version
    /// 调用都落到 offline 路径——表现是"App 看似 OK 但 server 永远 offline"。
    /// 为兑现注释承诺的"malformed 一律 fallback"，必须显式校验 scheme + host。
    /// 详见 docs/lessons/2026-04-26-url-string-malformed-tolerance.md。
    public static func resolveDefaultBaseURL(from bundle: Bundle) -> URL {
        if let raw = bundle.object(forInfoDictionaryKey: baseURLInfoKey) as? String,
           let url = validatedBaseURL(fromString: raw) {
            return url
        }
        // swiftlint:disable:next force_unwrapping
        return URL(string: fallbackBaseURLString)!
    }

    /// 校验字符串能否构成合法的 baseURL：必须能被 `URL(string:)` 解析，且 scheme 是 http/https，
    /// host 非空，且 **path 必须为空或仅 `/`**（host-only baseURL 契约，禁带 `/api/v1` 等前缀）。
    /// 任一条件不满足返回 nil，由调用方决定 fallback 策略。
    /// 提为独立 static 方法：让测试可直接覆盖各种 malformed 输入而无需构造 mock Bundle。
    /// 详见 docs/lessons/2026-04-26-url-string-malformed-tolerance.md
    /// 与 docs/lessons/2026-04-26-baseurl-host-only-contract.md。
    ///
    /// **path 校验缘由**（codex round 5 [P2] finding）：
    /// 本 story Dev Note #1 钦定 baseURL 为 host-only —— `/ping`、`/version` 在 server 根路径暴露，
    /// `APIClient` 直接把 endpoint.path 拼到 baseURL 上。若 xcconfig 误带 `/api/v1` 前缀（仓库早期约定），
    /// 拼出的 URL 会变成 `/api/v1/ping`、`/api/v1/version`，server 全部返 404，ping 永远落 offline 路径。
    /// 校验在配置入口拒绝带 path 的 baseURL，让 fallback 立刻生效，比让下游 silent 404 易诊断得多。
    ///
    /// `URL.path` 行为：
    ///   - `URL(string: "https://example.com")?.path` → `""`（接受）
    ///   - `URL(string: "https://example.com/")?.path` → `"/"`（接受，trailing slash 由 APIClient.init normalize）
    ///   - `URL(string: "https://example.com/api/v1")?.path` → `"/api/v1"`（拒绝）
    public static func validatedBaseURL(fromString raw: String) -> URL? {
        guard let url = URL(string: raw),
              let scheme = url.scheme?.lowercased(),
              scheme == "http" || scheme == "https",
              let host = url.host,
              !host.isEmpty
        else {
            return nil
        }
        // host-only 契约：path 仅允许空串或单 `/`。任何其他 path 前缀（如 `/api/v1`）都拒。
        if !url.path.isEmpty && url.path != "/" {
            return nil
        }
        return url
    }

    /// 工厂：构造 PingUseCase。每次调用返回新实例（UseCase 是 value type，构造廉价）。
    public func makePingUseCase() -> PingUseCaseProtocol {
        DefaultPingUseCase(client: apiClient)
    }

    /// Story 2.8 新增：构造 ResetKeychainUseCase（dev "重置身份" 按钮）。
    /// UseCase 是 value type，每次调用返回新实例；keychainStore 单例由 container 持有。
    public func makeResetKeychainUseCase() -> ResetKeychainUseCaseProtocol {
        DefaultResetKeychainUseCase(keychainStore: keychainStore)
    }

    /// Story 5.2 新增：构造 AuthRepository（DefaultAuthRepository）。
    /// Repository 是 value type struct，每次调用返回新实例；apiClient 单例由 container 持有。
    public func makeAuthRepository() -> AuthRepositoryProtocol {
        DefaultAuthRepository(apiClient: apiClient)
    }

    /// Story 5.2 新增：构造 GuestLoginUseCase。
    /// UseCase 是 value type struct；keychainStore 单例由 container 持有；
    /// uuidGenerator / deviceProvider 走默认 closure（生产值）。测试场景直接 new DefaultGuestLoginUseCase 注入 mock。
    public func makeGuestLoginUseCase() -> GuestLoginUseCaseProtocol {
        DefaultGuestLoginUseCase(
            keychainStore: keychainStore,
            repository: makeAuthRepository()
        )
    }

    /// Story 5.5 新增：构造 HomeRepository（DefaultHomeRepository）.
    /// Repository 是 value type struct；apiClient 单例由 container 持有（已被 ADR-0008 v2 装饰器包装 ——
    /// 业务请求 401 自动触发**全局 cold-start**：清 SessionStore + state machine 回 .launching → 重跑 bootstrap.
    /// 替代 Story 5.4 silent relogin 三件套；用户感知 < 1 秒 LaunchingView 闪屏，无错误弹窗）.
    public func makeHomeRepository() -> HomeRepositoryProtocol {
        DefaultHomeRepository(apiClient: apiClient)
    }

    /// Story 5.5 新增：构造 LoadHomeUseCase.
    /// UseCase 是 value type struct；每次调用返回新实例；repository 也是新实例（廉价）.
    public func makeLoadHomeUseCase() -> LoadHomeUseCaseProtocol {
        DefaultLoadHomeUseCase(repository: makeHomeRepository())
    }

    // MARK: - Story 12.7 AC8: 房间 UseCase factory（CreateRoom / JoinRoom / LeaveRoom + RoomRepository）

    /// Story 12.7 AC8: 构造 RoomRepository（每次调用返回新实例；apiClient 单例由 container 持有）.
    /// 与 makeHomeRepository / makeAuthRepository 同模式（value type struct，构造廉价）.
    public func makeRoomRepository() -> RoomRepositoryProtocol {
        DefaultRoomRepository(apiClient: apiClient)
    }

    /// Story 12.7 AC8: 构造 CreateRoomUseCase.
    /// 注入 appState 让 UseCase 写 setCurrentRoomId（spec AC1 钦定路径）.
    /// caller=RootView 通过 .onAppear bind() 注入 RealHomeViewModel.
    public func makeCreateRoomUseCase(appState: AppState) -> CreateRoomUseCaseProtocol {
        DefaultCreateRoomUseCase(roomRepository: makeRoomRepository(), appState: appState)
    }

    /// Story 12.7 AC8: 构造 JoinRoomUseCase.
    /// 注入 appState 让 UseCase 写 setCurrentRoomId.
    /// caller=RootView 通过 .onAppear bind() 注入 RealHomeViewModel + RealFriendsViewModel（共享 UseCase 实例廉价；构造廉价）.
    public func makeJoinRoomUseCase(appState: AppState) -> JoinRoomUseCaseProtocol {
        DefaultJoinRoomUseCase(roomRepository: makeRoomRepository(), appState: appState)
    }

    /// Story 12.7 AC8: 构造 LeaveRoomUseCase.
    /// 注入 appState 让 UseCase 读 currentRoomId + 写 setCurrentRoomId(nil).
    /// caller=RootView 通过 .onAppear bind() 注入 RealRoomViewModel.
    public func makeLeaveRoomUseCase(appState: AppState) -> LeaveRoomUseCaseProtocol {
        DefaultLeaveRoomUseCase(roomRepository: makeRoomRepository(), appState: appState)
    }

    // MARK: - Story 8.5 AC8: Step Sync 链路 factory

    /// Story 8.5 AC2: 构造 StepRepository（每次调用返回新实例；apiClient 单例由 container 持有）.
    /// DEBUG build 路径下若 UITest 注入了 mock，则返回 mock 而非 default.
    public func makeStepRepository() -> StepRepositoryProtocol {
        #if DEBUG
        if let mock = uiTestMockStepRepository {
            return mock
        }
        #endif
        return DefaultStepRepository(apiClient: apiClient)
    }

    /// Story 8.5 AC3: 构造 SyncStepsUseCase（每次调用返回新实例；
    /// 依赖 healthProvider / repository / appState / dateProvider）.
    /// caller 必须传 appState（AppState 在 RootView 持有；不进 AppContainer 字段；ADR-0010 §3.1）.
    public func makeSyncStepsUseCase(appState: AppState) -> SyncStepsUseCaseProtocol {
        DefaultSyncStepsUseCase(
            healthProvider: healthProvider,
            repository: makeStepRepository(),
            appState: appState,
            dateProvider: dateProvider
        )
    }

    /// Story 8.5 AC5 / AC9 (option A): 构造 StepSyncTriggerService（**每次调用返回新实例**；
    /// caller 应自行通过 @State 持有 strong 引用，避免 RootView body 重建时重启 timer）.
    /// option A：service 注入 homeViewModel 而非 motionProvider，借用 HomeViewModel.petState
    /// 作为 motionState 来源（避免与 8.4 motionProvider 单订阅契约冲突；详见 story 8.5 AC9 边界澄清段）.
    public func makeStepSyncTriggerService(
        appState: AppState,
        homeViewModel: HomeViewModel
    ) -> StepSyncTriggerService {
        StepSyncTriggerService(
            syncStepsUseCase: makeSyncStepsUseCase(appState: appState),
            homeViewModel: homeViewModel
        )
    }

    #if DEBUG
    /// Story 2.8 新增（仅 Debug build）：构造 ResetIdentityViewModel。
    /// Release build 该方法不存在；调用方（RootView）也必须 #if DEBUG 包裹调用 — fail-closed。
    ///
    /// Story 5.2 round 2 [P2] fix：注入 `sessionStore` —— 让 reset 成功后 in-memory session
    /// 同步清空，HomeView SessionAwareUserInfoBar 立刻退回 fallback nickname，
    /// 不再有"reset 后旧昵称/头像残留到杀进程"的 UI 不一致。
    ///
    /// Story 37.4 AC6：可选透传 `appState` —— 让 reset 成功后 AppState domain state 同步清空
    /// （ADR-0010 §3.7 Reset 流程）.appState 由 RootView 持有，通过参数透传过来；
    /// container 不持 appState 引用（避免 stable singleton 与 @StateObject lifecycle 耦合）.
    /// 默认参数 nil：保留 Release build 不需要 appState 的兼容路径（Release 无 reset 按钮）.
    public func makeResetIdentityViewModel(appState: AppState? = nil) -> ResetIdentityViewModel {
        ResetIdentityViewModel(
            useCase: makeResetKeychainUseCase(),
            sessionStore: sessionStore,
            appState: appState
        )
    }
    #endif
}

#if DEBUG
/// Story 8.5 AC11: UITest 路径下注入的 mock StepRepository，按 launch env 提供 stub 响应.
/// 仅 DEBUG 编译；不污染 Production binary.
///
/// 与 UITEST_MOCK_STEP_SYNC=1 配合使用；详见 AppContainer.convenience init() 中的 launch env 解析路径.
/// `@unchecked Sendable`：与 MockAPIClient / HealthProviderMock 同模式（mutable 字段但测试串行调用）.
private final class UITestMockStepRepository: StepRepositoryProtocol, @unchecked Sendable {
    private let stubResponse: StepsSyncResponse

    init(stubResponse: StepsSyncResponse) {
        self.stubResponse = stubResponse
    }

    func syncSteps(_ request: StepsSyncRequest) async throws -> StepsSyncResponse {
        return stubResponse
    }
}

/// Story 8.5 AC11: UITest 路径下注入的 mock HealthProvider；与 UITEST_MOCK_STEP_SYNC=1 联动启用.
/// 任意日期都返同一 stubSteps；权限申请直接返 true（避免 sim 上 HealthKit 权限拒绝阻塞链路）.
/// 仅 DEBUG 编译；不污染 Production binary.
private final class UITestMockHealthProvider: HealthProvider, @unchecked Sendable {
    private let stubSteps: Int

    init(stubSteps: Int) {
        self.stubSteps = stubSteps
    }

    func requestPermission() async throws -> Bool { true }

    func readDailyTotalSteps(date: Date) async throws -> Int {
        return stubSteps
    }
}
#endif
