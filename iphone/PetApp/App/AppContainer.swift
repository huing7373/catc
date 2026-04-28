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
    /// Story 5.4：`baseAPIClient` 之上包一层 `AuthRetryingAPIClient` 装饰器 ——
    /// 业务层（makeAuthRepository / 未来 makeHomeRepository 等）拿到的 apiClient 自动具备
    /// 静默重登能力（401 → 自动调 /auth/guest-login 拿新 token → 重试一次）.
    /// 注：`baseRepository` 持 unwrapped `baseAPIClient`，**专给** SilentReloginUseCase 用 ——
    /// 严格隔离循环依赖（即使 /auth/guest-login requiresAuth=false 不会被 wrap 拦，unwrapped
    /// 也是更清晰的语义边界，防御未来 endpoint 改动时的诡异 bug）.
    public convenience init() {
        let baseURL = AppContainer.resolveDefaultBaseURL(from: Bundle.main)
        let keychainStore = KeychainServicesStore()

        // Story 5.4: 先建 baseAPIClient + baseRepository（**只**给 SilentReloginUseCase 用）
        let baseAPIClient = APIClient(baseURL: baseURL, keychainStore: keychainStore)
        let baseRepository = DefaultAuthRepository(apiClient: baseAPIClient)
        let reloginUseCase = DefaultSilentReloginUseCase(
            keychainStore: keychainStore,
            repository: baseRepository
        )
        let coordinator = SilentReloginCoordinator(useCase: reloginUseCase)

        // 业务层用包装后的 wrappedAPIClient ——  401 自动恢复
        let wrappedAPIClient = AuthRetryingAPIClient(inner: baseAPIClient, coordinator: coordinator)

        self.init(
            apiClient: wrappedAPIClient,
            keychainStore: keychainStore
        )
    }

    /// 注入式 init：测试中传 mock APIClient；未来 release build 切到 production baseURL 时也走此入口。
    /// Story 2.8 → Story 5.1 evolution：
    /// - Story 2.8 默认值 `InMemoryKeychainStore()`（占位）
    /// - Story 5.1 默认值切换为 `KeychainServicesStore()`（基于 Apple Security.framework，真实持久化）
    /// 协议 `KeychainStoreProtocol` 四方法签名不变，Story 2.8 既有调用方零改动；
    /// `InMemoryKeychainStore` 作为测试便利 + 模板示范保留（仍由 InMemoryKeychainStoreTests 维护）。
    public init(
        apiClient: APIClientProtocol,
        keychainStore: KeychainStoreProtocol = KeychainServicesStore()
    ) {
        self.apiClient = apiClient
        self.errorPresenter = ErrorPresenter()
        self.keychainStore = keychainStore
        // Story 5.2 新增：sessionStore 在 init 一次性构造，与 errorPresenter 同模式（stable singleton）。
        // 不预留 init(...sessionStore:) 注入式签名 —— 测试场景直接 new SessionStore() 即可（@MainActor 类，
        // 跨 actor 注入需 @Sendable 约束麻烦；container.sessionStore 已暴露足以验证）。
        self.sessionStore = SessionStore()
    }

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
    /// Repository 是 value type struct；apiClient 单例由 container 持有（已被 Story 5.4 装饰器包装 ——
    /// 业务请求 401 自动触发静默重登 + 重试一次）.
    public func makeHomeRepository() -> HomeRepositoryProtocol {
        DefaultHomeRepository(apiClient: apiClient)
    }

    /// Story 5.5 新增：构造 LoadHomeUseCase.
    /// UseCase 是 value type struct；每次调用返回新实例；repository 也是新实例（廉价）.
    public func makeLoadHomeUseCase() -> LoadHomeUseCaseProtocol {
        DefaultLoadHomeUseCase(repository: makeHomeRepository())
    }

    /// Story 5.4 新增：构造 SilentReloginUseCase（默认走 container 持有的 keychain + 新建 repository）.
    /// 让需要直接调 SilentRelogin 的场景（集成测试 / future 业务）走 container 入口.
    /// 注：此处的 repository 是 wrap 过的（来自 makeAuthRepository）—— 因 /auth/guest-login
    /// requiresAuth=false，AuthRetryingAPIClient 不会拦截；与 convenience init 内 baseRepository
    /// 不同（后者直接用 baseAPIClient，**有意为之** —— 见 init 注释）.
    public func makeSilentReloginUseCase() -> SilentReloginUseCaseProtocol {
        DefaultSilentReloginUseCase(
            keychainStore: keychainStore,
            repository: makeAuthRepository()
        )
    }

    #if DEBUG
    /// Story 2.8 新增（仅 Debug build）：构造 ResetIdentityViewModel。
    /// Release build 该方法不存在；调用方（RootView）也必须 #if DEBUG 包裹调用 — fail-closed。
    ///
    /// Story 5.2 round 2 [P2] fix：注入 `sessionStore` —— 让 reset 成功后 in-memory session
    /// 同步清空，HomeView SessionAwareUserInfoBar 立刻退回 fallback nickname，
    /// 不再有"reset 后旧昵称/头像残留到杀进程"的 UI 不一致。
    public func makeResetIdentityViewModel() -> ResetIdentityViewModel {
        ResetIdentityViewModel(
            useCase: makeResetKeychainUseCase(),
            sessionStore: sessionStore
        )
    }
    #endif
}
