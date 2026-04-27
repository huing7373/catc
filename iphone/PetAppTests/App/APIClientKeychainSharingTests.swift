// APIClientKeychainSharingTests.swift
// Story 5.3 AC6: 验证 AppContainer convenience init() 改动后 APIClient 与 AppContainer
// 共享同一 keychain instance —— GuestLoginUseCase 写 token 后，下一次 APIClient 调
// requiresAuth=true 接口立刻能从同一 keychain 读到（不会因 namespace 不一致或双 instance 漏读）.
//
// 测试策略：
// - 走 `init(apiClient:keychainStore:)` 注入式入口（**不**通过 `convenience init()`）—— 后者会真实
//   new `KeychainServicesStore`，测试无法注入 mock；testing convenience init() 需要做 namespace 隔离的
//   真实 keychain（与 Story 5.2 `GuestLoginUseCaseIntegrationTests` 同精神，本 story 不增加这种集成测试）.
// - 共享一个 MockKeychainStore instance 注入到 APIClient + AppContainer 两边
// - 通过 mock URLSession 拦截请求，断言 header 与 container.keychainStore 写入的 token 一致
//   即"AppContainer 与 APIClient 共享同一 keychain 时，APIClient 读到的 token 与 container 暴露的
//   keychain 写入的 token 一致".

import XCTest
@testable import PetApp

#if DEBUG

@MainActor
final class APIClientKeychainSharingTests: XCTestCase {

    func testApiClientReadsSameKeychainAsContainer() async throws {
        // 共享 keychain instance（用 Story 2.8 MockKeychainStore）
        let sharedKeychain = MockKeychainStore()
        sharedKeychain.getStubResult = .success("shared-token-xyz")

        let mockSession = MockURLSession()
        mockSession.stubbedResponse = (
            "{\"code\":0,\"message\":\"ok\",\"data\":{},\"requestId\":\"r\"}"
                .data(using: .utf8)!,
            HTTPURLResponse(url: URL(string: "http://x")!, statusCode: 200,
                            httpVersion: "HTTP/1.1",
                            headerFields: ["Content-Type": "application/json"])!
        )

        let apiClient = APIClient(
            baseURL: URL(string: "http://localhost:8080")!,
            session: mockSession,
            keychainStore: sharedKeychain  // ← 同一个 keychain 也注入到 APIClient
        )

        let container = AppContainer(apiClient: apiClient, keychainStore: sharedKeychain)

        // 通过 container 暴露的接口读 keychain → 与 APIClient 内部读到的应是同一值
        XCTAssertEqual(
            try container.keychainStore.get(forKey: KeychainKey.authToken.rawValue),
            "shared-token-xyz"
        )

        // 通过 APIClient 发 requiresAuth=true 请求 → 验证 header 带上 token
        struct Empty: Decodable {}
        let endpoint = Endpoint(path: "/api/v1/home", method: .get, requiresAuth: true)
        let _: Empty = try await apiClient.request(endpoint)

        let request = mockSession.invocations.first!
        XCTAssertEqual(
            request.value(forHTTPHeaderField: "Authorization"),
            "Bearer shared-token-xyz"
        )
    }
}

#endif
