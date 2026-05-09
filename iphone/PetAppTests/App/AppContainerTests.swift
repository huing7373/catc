// AppContainerTests.swift
// Story 2.5 review fix round 1：AppContainer.resolveDefaultBaseURL(from:) 单元测试。
//
// 背景：codex review round 1 finding #2 指出默认 baseURL 不应硬编码 localhost；
// 修复策略：让 AppContainer 从 Info.plist 读 PetAppBaseURL，缺省 fallback 到 localhost。
// 详见 docs/lessons/2026-04-26-baseurl-from-info-plist.md。
//
// 测试基础设施约束（与 Story 2.7 衔接）：
// - 仅依赖 stdlib（XCTest + @testable import PetApp），不引入 helper 文件。
// - 用真实 Bundle (Bundle(for:)) + 临时写入的 Info.plist fixture 覆盖正向 / fallback / 异常路径。

import XCTest
@testable import PetApp

@MainActor
final class AppContainerTests: XCTestCase {

    /// case#1 (happy)：Info.plist 有 PetAppBaseURL → 读取并返回该 URL。
    /// 用 main bundle —— 当前 PetApp Info.plist 已默认配置 http://localhost:8080，验证读取链路。
    func testResolveDefaultBaseURLReadsFromMainBundle() {
        let url = AppContainer.resolveDefaultBaseURL(from: Bundle.main)
        // main bundle 在 test host 下不一定与 PetApp Info.plist 一致；只断言能产出 URL（非 nil）即可。
        XCTAssertNotNil(url.scheme, "应解析出合法 scheme")
        XCTAssertNotNil(url.host, "应解析出 host（fallback localhost 或 PetAppBaseURL 配置值）")
    }

    /// case#2 (edge)：bundle 不含 PetAppBaseURL → fallback 到 localhost。
    /// 用本测试 target 的 Bundle —— 该 bundle Info.plist 没配置 PetAppBaseURL，必走 fallback 分支。
    func testResolveDefaultBaseURLFallsBackWhenKeyMissing() {
        let testBundle = Bundle(for: AppContainerTests.self)
        // 防御：如果未来测试 bundle 配置了 PetAppBaseURL，本测试会假阳性，提前拦截。
        XCTAssertNil(testBundle.object(forInfoDictionaryKey: AppContainer.baseURLInfoKey),
                     "测试 bundle 不应配置 PetAppBaseURL；如需配置请改本测试断言策略")

        let url = AppContainer.resolveDefaultBaseURL(from: testBundle)

        XCTAssertEqual(url.absoluteString, AppContainer.fallbackBaseURLString,
                       "缺 key 时应 fallback 到 \(AppContainer.fallbackBaseURLString)")
    }

    /// case#3 (sanity)：默认 init 走通，apiClient 非 nil。
    func testDefaultInitProducesUsableContainer() {
        let container = AppContainer()
        XCTAssertNotNil(container.apiClient, "默认 init 应构造可用的 APIClient")
        XCTAssertNotNil(container.makePingUseCase(), "默认 container 应能产出 PingUseCase")
    }

    /// codex round 1 [P1] 修复防回归：
    /// AppContainer.errorPresenter 必须是同一个 instance（stable singleton 在 container 范围内）。
    /// RootView 同时在 body 末尾和 sheetContent 内部 attach `errorPresentationHost(presenter:)`,
    /// 两处必须共享同一个 ErrorPresenter，否则 sheet 子树会显示空状态、错过外层 publish 的 current。
    /// 详见 docs/lessons/2026-04-26-fullscreencover-isolated-environment.md。
    func testErrorPresenterIsStableSingletonWithinContainer() {
        let container = AppContainer()
        let first = container.errorPresenter
        let second = container.errorPresenter
        XCTAssertTrue(first === second,
                      "container.errorPresenter 必须是同一个 instance；root host 和 sheet host 共享 source of truth")
    }

    // MARK: - round 4 [P2]：validatedBaseURL(fromString:) 拒绝 malformed 输入

    /// case#4a (round 4)：`URL(string:)` 对 malformed 输入过于宽容，需在 resolve 层补校验。
    /// codex round 4 finding：`URL(string: "localhost:8080")` 返回 non-nil
    /// （Apple parser 把它当成 `scheme=localhost, path=8080`），但 `APIClient` 构请求会失败，
    /// 表现是 ping/version 永远 offline。所以 resolve 层必须自己卡 scheme + host。
    /// 详见 docs/lessons/2026-04-26-url-string-malformed-tolerance.md。
    func testValidatedBaseURLRejectsMalformedInputs() {
        // 1. 无 scheme（仅 host:port） — Apple URL parser 不会拒，但语义错。
        XCTAssertNil(AppContainer.validatedBaseURL(fromString: "localhost:8080"),
                     "缺 scheme 的 host:port 字符串应被拒绝")

        // 2. 仅有 scheme 没有 host
        XCTAssertNil(AppContainer.validatedBaseURL(fromString: "http://"),
                     "缺 host 的 URL 字符串应被拒绝")

        // 3. 不支持的 scheme（ftp / ws / file 等都不应作为 HTTP API baseURL）
        XCTAssertNil(AppContainer.validatedBaseURL(fromString: "ftp://example.com"),
                     "ftp scheme 不支持 → 应被拒绝")
        XCTAssertNil(AppContainer.validatedBaseURL(fromString: "ws://example.com"),
                     "ws scheme 不支持（WebSocket 用 wss/ws 走另一通道）→ 应被拒绝")

        // 4. 空字符串
        XCTAssertNil(AppContainer.validatedBaseURL(fromString: ""),
                     "空字符串应被拒绝")

        // 5. 含空格的非法 URL（URL parser 也会拒）
        XCTAssertNil(AppContainer.validatedBaseURL(fromString: "http://example .com"),
                     "含空格的字符串应被拒绝")

        // 6. (round 5 [P2]) 带 path prefix 的 baseURL —— host-only 契约要求拒。
        //    若 xcconfig 误带 `/api/v1` 前缀，APIClient 拼出 `/api/v1/ping`、`/api/v1/version`，
        //    server 全部返 404，ping/version 永远 offline。
        //    详见 docs/lessons/2026-04-26-baseurl-host-only-contract.md。
        XCTAssertNil(AppContainer.validatedBaseURL(fromString: "https://api.example.com/api/v1"),
                     "带 path prefix 的 baseURL 违反 host-only 契约 → 应被拒绝")
        XCTAssertNil(AppContainer.validatedBaseURL(fromString: "http://localhost:8080/api/v1/"),
                     "带 path prefix（含 trailing slash）的 baseURL 也应被拒绝")
        XCTAssertNil(AppContainer.validatedBaseURL(fromString: "https://api.example.com/v2/foo"),
                     "任何非空非 `/` 的 path 都应被拒绝")
    }

    /// case#4b (round 4)：合法 http/https URL 必须被接受，scheme 大小写不敏感。
    func testValidatedBaseURLAcceptsValidHTTPAndHTTPS() {
        // 标准 http（无 path）
        XCTAssertEqual(
            AppContainer.validatedBaseURL(fromString: "http://localhost:8080")?.absoluteString,
            "http://localhost:8080"
        )
        // 标准 https（无 path）
        XCTAssertEqual(
            AppContainer.validatedBaseURL(fromString: "https://api.example.com")?.absoluteString,
            "https://api.example.com"
        )
        // 大写 scheme 也接受（lowercased 后比较）
        XCTAssertNotNil(AppContainer.validatedBaseURL(fromString: "HTTPS://api.example.com"))
        XCTAssertNotNil(AppContainer.validatedBaseURL(fromString: "HTTP://localhost:8080"))
        // (round 5 [P2]) 仅 trailing slash 应被接受 —— `URL.path` 此时为 "/"，host-only 契约容忍。
        XCTAssertNotNil(AppContainer.validatedBaseURL(fromString: "https://api.example.com/"),
                        "仅 trailing slash 不属于 path prefix，应被接受")
        XCTAssertNotNil(AppContainer.validatedBaseURL(fromString: "http://localhost:8080/"),
                        "localhost trailing slash 应被接受")
    }

    // MARK: - round 1 [P1] (Story 2.8 dev-tools)：resetIdentityViewModel 必须共享 container.keychainStore

    /// case#5a (Story 2.8 round 1 [P1])：通过 container.makeResetIdentityViewModel() 构造的 ViewModel
    /// 必须用 container.keychainStore 同一 instance；不能像早期实装那样 standalone 新建一个 InMemoryKeychainStore，
    /// 否则按下"重置身份"调的 removeAll() 清的不是 App 实际写入的那份字典 → UI 显示成功但功能失效。
    ///
    /// 验证策略：往 container.keychainStore 写一个值 → 调 reset ViewModel.tap() 触发 useCase.execute()
    /// → 断言 container.keychainStore 里的值被清空。即"reset 影响 container 自己持有的 store"。
    /// 详见 docs/lessons/2026-04-26-stateobject-debug-instance-aliasing.md。
    ///
    /// 测试隔离强约束（codex round 3 [P2] finding 修复后的版本）：
    /// **不**走默认 `AppContainer()`——那会绑生产 namespace `com.zhuming.pet.app` 的 KeychainServicesStore，
    /// 测试中的 set / removeAll 会污染手动调试遗留的 `guestUid` / `authToken`，并与 PetAppUITests
    /// 跨 launch 持久化测试 cross-talk。改走 `init(apiClient:keychainStore:)` 注入专属 namespace
    /// （带 UUID 后缀的 `KeychainServicesStore`），与 KeychainServicesStoreTests 同模式。
    /// 详见 docs/lessons/2026-04-27-appcontainertests-must-inject-isolated-keychain-namespace.md。
    #if DEBUG
    func testResetIdentityViewModelSharesContainerKeychainStore() async throws {
        // 专属 namespace：UUID 保证跨 test method / 跨 bundle 都不会撞，
        // tap() 触发的 removeAll() 只会清此隔离区，不影响生产 namespace。
        let testService = "com.zhuming.pet.app.tests.\(UUID().uuidString)"
        let isolatedKeychain = KeychainServicesStore(service: testService)
        defer { try? isolatedKeychain.removeAll() }

        let container = AppContainer(
            apiClient: APIClient(baseURL: AppContainer.resolveDefaultBaseURL(from: Bundle.main)),
            keychainStore: isolatedKeychain
        )

        // 1. 写入一个值，模拟 App 后续真实写 keychain（如 sessionToken / userId）。
        try container.keychainStore.set("test-token", forKey: "sessionToken")
        XCTAssertEqual(try container.keychainStore.get(forKey: "sessionToken"), "test-token",
                       "前置：值已写入 container.keychainStore")

        // 2. 通过 container factory 拿 ViewModel，触发 reset。
        let viewModel = container.makeResetIdentityViewModel()
        await viewModel.tap()

        // 3. 必须读不到值——只有 reset 调到 container.keychainStore.removeAll() 才会发生。
        //    若 ViewModel 拿的是另一个 standalone keychainStore（早期实装的 bug），container.keychainStore
        //    里 "test-token" 不会被清，本断言会失败。
        XCTAssertNil(try container.keychainStore.get(forKey: "sessionToken"),
                     "reset 必须清 container.keychainStore；若失败说明 ViewModel 拿到的是别的 keychainStore instance")
    }

    /// case#5b (Story 5.2 round 2 [P2])：container.makeResetIdentityViewModel() 必须把
    /// container.sessionStore 同一 instance 注入 ViewModel —— 这样 reset 按下后 HomeView
    /// SessionAwareUserInfoBar 立刻退回 fallback nickname，不再渲染旧身份。
    ///
    /// 验证策略：往 container.sessionStore 写入 session → 调 reset ViewModel.tap() 触发 useCase
    /// → 断言 container.sessionStore.session 被清空（即"reset 影响 container 自己持有的 sessionStore"）。
    /// 若 ViewModel 拿的是另一个 standalone SessionStore，container.sessionStore.session 不会被清。
    /// 详见 docs/lessons/2026-04-27-reset-identity-must-clear-in-memory-session.md。
    func testResetIdentityViewModelClearsContainerSessionStore() async throws {
        let testService = "com.zhuming.pet.app.tests.\(UUID().uuidString)"
        let isolatedKeychain = KeychainServicesStore(service: testService)
        defer { try? isolatedKeychain.removeAll() }

        let container = AppContainer(
            apiClient: APIClient(baseURL: AppContainer.resolveDefaultBaseURL(from: Bundle.main)),
            keychainStore: isolatedKeychain
        )

        // 1. 把 session 写入 container.sessionStore 模拟登录成功后状态
        let preLoginSession = SessionState(
            user: UserProfile(id: "1001", nickname: "登录后昵称", avatarUrl: "", hasBoundWechat: false),
            pet: PetProfile(id: "2001", petType: 1, name: "默认小猫")
        )
        container.sessionStore.updateSession(preLoginSession)
        XCTAssertNotNil(container.sessionStore.session, "前置：session 已写入 container.sessionStore")

        // 2. 通过 container factory 拿 ViewModel，触发 reset
        let viewModel = container.makeResetIdentityViewModel()
        await viewModel.tap()

        // 3. container.sessionStore.session 必须被清；若失败说明 ViewModel 拿到别的 SessionStore 实例。
        XCTAssertNil(container.sessionStore.session,
                     "reset 必须清 container.sessionStore.session；若失败说明 ViewModel 拿到的是别的 SessionStore instance")
    }
    #endif

    /// case#5 (round 3)：PetApp 的 Info.plist 必须配置 NSAppTransportSecurity → NSAllowsLocalNetworking = true。
    /// 否则 cleartext HTTP（http://localhost:8080）会被 iOS ATS 在 OS 层拒绝，feature 永远 offline。
    /// 详见 docs/lessons/2026-04-26-ios-ats-cleartext-http.md。
    ///
    /// 注意：直接读 Bundle.main.infoDictionary 拿到的是 test host 的 plist；要拿被测 PetApp.app 的 plist，
    /// 需要从 PetApp 内部某 class（如 AppContainer 本身）的 Bundle(for:) 反查。
    func testInfoPlistAllowsLocalNetworking() {
        // PetApp.app 的 Bundle —— 通过 AppContainer 这个类反查（与 main bundle 不同）。
        let petAppBundle = Bundle(for: AppContainer.self)

        guard let ats = petAppBundle.object(forInfoDictionaryKey: "NSAppTransportSecurity") as? [String: Any] else {
            XCTFail("PetApp Info.plist 必须配置 NSAppTransportSecurity（用于允许 cleartext localhost）")
            return
        }

        let allowsLocal = ats["NSAllowsLocalNetworking"] as? Bool
        XCTAssertEqual(allowsLocal, true,
                       "NSAllowsLocalNetworking 必须为 true；缺失会让 ping/version 在真机和模拟器都被 ATS 拒绝")
    }

    // MARK: - fix-review round 1 P2 (Story 12.2)：injected apiClient + 默认 webSocketClient 必须同源 baseURL

    /// case#6 (Story 12.2 r1 [P2])：当 caller 注入非默认 baseURL 的 apiClient 但**不**传 webSocketClient,
    /// 默认 fallback 必须从 apiClient.baseURL 派生 WS URL —— 不能 silent 退回 Bundle.main / localhost.
    /// 否则 REST 打注入 backend、WS 打 default host → split-brain container.
    /// 详见本轮 review 修复（fix-review round 1 P2）.
    func testWebSocketClientFallbackUsesInjectedAPIClientBaseURL() throws {
        // 注入一个非默认 baseURL 的 APIClient（模拟测试 / Preview / alt-env 场景）
        let injectedBaseURL = URL(string: "https://staging.example.com")!
        let injectedAPIClient = APIClient(baseURL: injectedBaseURL)

        let testService = "com.zhuming.pet.app.tests.\(UUID().uuidString)"
        let isolatedKeychain = KeychainServicesStore(service: testService)
        defer { try? isolatedKeychain.removeAll() }

        // 不传 webSocketClient → 走 fallback path
        let container = AppContainer(
            apiClient: injectedAPIClient,
            keychainStore: isolatedKeychain
        )

        // container.webSocketClient 应为 WebSocketClientImpl 且 baseURL 与 injectedBaseURL 同源
        guard let wsImpl = container.webSocketClient as? WebSocketClientImpl else {
            XCTFail("Expected fallback WebSocketClientImpl, got \(type(of: container.webSocketClient))")
            return
        }

        // 通过 makeWSURL（internal）验证派生路径用的是 injectedBaseURL host —— 不该出现 localhost.
        let derivedURL = try wsImpl.makeWSURL(roomId: "RM01", token: "tok")
        XCTAssertEqual(derivedURL.host, injectedBaseURL.host,
                       "fallback WebSocketClient 的 host 必须与 injected apiClient.baseURL.host 同源；" +
                       "若拿到 localhost 说明走了 Bundle.main 默认值（split-brain bug）")
        XCTAssertEqual(derivedURL.scheme, "wss",
                       "https://staging.example.com 应派生 wss")
    }
}
