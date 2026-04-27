// GuestLoginUseCaseIntegrationTests.swift
// Story 5.2 AC12: GuestLoginUseCase + 真实 APIClient + 真实 KeychainServicesStore（隔离 namespace）+
// StubURLProtocol（伪造 server 响应）的端到端集成测试.
//
// 与单测的区别：
// - 单测用 MockAuthRepository 跳过 APIClient 层；本测试用真实 APIClient 验证 endpoint 拼接 / JSON 编解码
// - 单测用 MockKeychainStore；本测试用真实 KeychainServicesStore（带 UUID namespace 隔离）验证 keychain
//   读写实际生效（lesson 2026-04-27-keychain-service-namespace-injectable.md 钦定测试 namespace 必须注入）
//
// 复用 Story 2.5 落地的 PingStubURLProtocol 同模式：URLProtocol 子类 + URLSessionConfiguration 注入.
//
// ⚠️ 并发风险与使用约束（继承 StubURLProtocol / PingStubURLProtocol 同精神）：
// - GuestLoginStubURLProtocol 是 process-global static 状态；任一时刻仅一个 testcase 用
// - setUp / tearDown 必须 reset()
// - 仅 session-local 注入（URLSessionConfiguration.protocolClasses），禁止 URLProtocol.registerClass(_:)

import XCTest
@testable import PetApp

#if DEBUG

@MainActor
final class GuestLoginUseCaseIntegrationTests: XCTestCase {

    private var sut: DefaultGuestLoginUseCase!
    private var keychain: KeychainServicesStore!
    private var apiClient: APIClient!

    override func setUp() {
        super.setUp()
        // 1. 隔离 namespace 的 KeychainServicesStore（防污染生产 / 其他测试）
        let testService = "com.zhuming.pet.app.tests.\(UUID().uuidString)"
        keychain = KeychainServicesStore(service: testService)
        try? keychain.removeAll()

        // 2. 注入 GuestLoginStubURLProtocol 到 URLSession（session-local，非 global registry）
        GuestLoginStubURLProtocol.reset()
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [GuestLoginStubURLProtocol.self]
        let session = URLSession(configuration: config)
        apiClient = APIClient(baseURL: URL(string: "http://localhost:8080")!, session: session)

        sut = DefaultGuestLoginUseCase(
            keychainStore: keychain,
            repository: DefaultAuthRepository(apiClient: apiClient),
            uuidGenerator: { "integration-test-uuid" },
            deviceProvider: { GuestLoginRequest.Device(platform: "ios", appVersion: "1.0.0", deviceModel: "iPhone15,2") }
        )
    }

    override func tearDown() {
        try? keychain?.removeAll()
        sut = nil
        keychain = nil
        apiClient = nil
        GuestLoginStubURLProtocol.reset()
        super.tearDown()
    }

    // MARK: - case#1: happy E2E：StubURLProtocol 返 200 + 完整 envelope → guestUid + token 正确写入 keychain

    func testEndToEndHappyPathWritesKeychainAndReturnsOutput() async throws {
        let body = """
        {
          "code": 0,
          "message": "ok",
          "data": {
            "token": "stub-jwt-token",
            "user": {"id": "1001", "nickname": "用户1001", "avatarUrl": "", "hasBoundWechat": false},
            "pet": {"id": "2001", "petType": 1, "name": "默认小猫"}
          },
          "requestId": "req_int_1"
        }
        """
        GuestLoginStubURLProtocol.setStub(statusCode: 200, data: body.data(using: .utf8))

        let output = try await sut.execute()

        // 1. UseCase 输出
        XCTAssertEqual(output.user.id, "1001")
        XCTAssertEqual(output.user.nickname, "用户1001")
        XCTAssertEqual(output.pet.id, "2001")
        XCTAssertEqual(output.pet.name, "默认小猫")

        // 2. keychain 写入（真实 KeychainServicesStore，验证 set/get 实际生效）
        XCTAssertEqual(try keychain.get(forKey: KeychainKey.guestUid.rawValue), "integration-test-uuid")
        XCTAssertEqual(try keychain.get(forKey: KeychainKey.authToken.rawValue), "stub-jwt-token")

        // 3. 请求 URL / method 验证（验 Endpoint 拼接 + AppContainer host-only baseURL 契约）
        let lastRequest = GuestLoginStubURLProtocol.lastRequest()
        XCTAssertEqual(lastRequest?.url?.absoluteString, "http://localhost:8080/api/v1/auth/guest-login")
        XCTAssertEqual(lastRequest?.httpMethod, "POST")
        XCTAssertEqual(lastRequest?.value(forHTTPHeaderField: "Content-Type"), "application/json")
    }

    // MARK: - case#2: edge E2E：server 返 1009 → 抛 APIError.business；keychain 已写 guestUid 但未写 token

    func testEndToEndServerBusinessErrorThrowsButKeepsGuestUid() async throws {
        let body = """
        {"code": 1009, "message": "服务繁忙", "data": null, "requestId": "req_int_2"}
        """
        GuestLoginStubURLProtocol.setStub(statusCode: 200, data: body.data(using: .utf8))

        do {
            _ = try await sut.execute()
            XCTFail("应抛 APIError.business")
        } catch let error as APIError {
            if case .business(let code, _, _) = error {
                XCTAssertEqual(code, 1009)
            } else {
                XCTFail("应是 .business，实际 \(error)")
            }
        } catch {
            XCTFail("应抛 APIError，实际抛 \(error)")
        }

        // 关键断言：guestUid 已写入（不回滚）；token 未写
        XCTAssertEqual(try keychain.get(forKey: KeychainKey.guestUid.rawValue), "integration-test-uuid",
                       "API 业务错误时 guestUid 不回滚（下次重试复用同一身份）")
        XCTAssertNil(try keychain.get(forKey: KeychainKey.authToken.rawValue),
                     "API 失败时 token 未写")
    }
}

// MARK: - GuestLoginStubURLProtocol
//
// 配套 StubURLProtocol（参考 Story 2.5 PingStubURLProtocol / Story 2.4 StubURLProtocol 同模式）.
// process-global static 状态 + NSLock 保护读写 + snapshot 原子读.
// 单一 stub 模式（非按 path 路由）：本测试只调一个 endpoint，不需要 PingStubURLProtocol 的 routes map.

final class GuestLoginStubURLProtocol: URLProtocol {

    private static let lock = NSLock()
    private static var _stubData: Data?
    private static var _stubStatusCode: Int = 200
    private static var _lastRequest: URLRequest?

    /// 测试设置 stub（setUp / 测试中调）.
    static func setStub(statusCode: Int, data: Data?) {
        lock.lock(); defer { lock.unlock() }
        _stubStatusCode = statusCode
        _stubData = data
    }

    /// 测试断言用：拿最近一次拦截到的 request.
    static func lastRequest() -> URLRequest? {
        lock.lock(); defer { lock.unlock() }
        return _lastRequest
    }

    /// 测试 setUp / tearDown 调用清状态.
    static func reset() {
        lock.lock(); defer { lock.unlock() }
        _stubData = nil
        _stubStatusCode = 200
        _lastRequest = nil
    }

    /// 一次性原子读取 stub，避免 startLoading 中途被并发写入.
    private static func snapshot() -> (data: Data?, statusCode: Int) {
        lock.lock(); defer { lock.unlock() }
        return (_stubData, _stubStatusCode)
    }

    private static func recordRequest(_ request: URLRequest) {
        lock.lock(); defer { lock.unlock() }
        _lastRequest = request
    }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        Self.recordRequest(request)
        let snap = Self.snapshot()

        let response = HTTPURLResponse(
            url: request.url!,
            statusCode: snap.statusCode,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        )!

        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        if let data = snap.data {
            client?.urlProtocol(self, didLoad: data)
        }
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}

#endif
