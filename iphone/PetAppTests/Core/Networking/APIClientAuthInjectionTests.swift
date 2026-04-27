// APIClientAuthInjectionTests.swift
// Story 5.3 AC5: APIClient interceptor 自动注入 Bearer token 单元测试.
//
// 与 Story 2.4 APIClientTests 的区别：
// - APIClientTests 覆盖 envelope decode / status code / business code 决策；本文件
//   专注 token 注入路径（requiresAuth=true/false × token 存在/不存在 × 并发安全）.
//
// 复用：
// - MockURLSession（Story 2.4 落地，含 invocations: [URLRequest]；Story 5.3 已加 NSLock）
// - MockKeychainStore（Story 2.8 落地，#if DEBUG，继承 MockBase）
//
// 测试覆盖（≥ 4 case，本文件给 7 case）：
// case#1 happy：requiresAuth=true + token 存在 → header 写
// case#2 happy：requiresAuth=false → header 无（即使 keychain 有 token 也不读）
// case#3 edge：requiresAuth=true + token nil → unauthorized + 不发请求
// case#4 edge：requiresAuth=true + token 空串 → unauthorized + 不发请求（防御性）
// case#5 edge：requiresAuth=true + keychain.get 抛错 → unauthorized + 不发请求（降级语义）
// case#6 edge：requiresAuth=true + APIClient.init 未注 keychain → unauthorized + 不发请求
// case#7 edge：同一 APIClient 并发 100 请求 → 全部正确注入（验线程安全）

import XCTest
@testable import PetApp

#if DEBUG

@MainActor
final class APIClientAuthInjectionTests: XCTestCase {

    private let baseURL = URL(string: "http://localhost:8080")!

    // 解码目标（mimic Story 2.4 测试中 PingResponseMock 模式）
    private struct EmptyDataMock: Decodable, Equatable {
        // 解码空对象 {} 的占位
    }

    private func makeHTTPResponse(statusCode: Int) -> HTTPURLResponse {
        HTTPURLResponse(
            url: baseURL,
            statusCode: statusCode,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        )!
    }

    private func makeStubResponseBody() -> Data {
        // 成功 envelope，data = {}
        """
        {"code":0,"message":"ok","data":{},"requestId":"req_abc"}
        """.data(using: .utf8)!
    }

    // MARK: - case#1 (happy)：requiresAuth=true + Keychain 有 token → 请求 URL header 含 Authorization: Bearer xxx
    func testInjectsAuthorizationHeaderWhenRequiresAuthAndTokenExists() async throws {
        let session = MockURLSession()
        session.stubbedResponse = (makeStubResponseBody(), makeHTTPResponse(statusCode: 200))

        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("test-jwt-token-abc")

        let client = APIClient(baseURL: baseURL, session: session, keychainStore: keychain)
        let endpoint = Endpoint(path: "/api/v1/home", method: .get, requiresAuth: true)

        let _: EmptyDataMock = try await client.request(endpoint)

        // 1. session.data(for:) 被调过一次
        XCTAssertEqual(session.invocations.count, 1)
        // 2. 该请求的 Authorization header 严格等于 "Bearer test-jwt-token-abc"
        let request = session.invocations.first!
        XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer test-jwt-token-abc")
        // 3. keychain.get(forKey:) 被调过 1 次（精确次数）
        XCTAssertEqual(keychain.callCount(of: "get(forKey:)"), 1)
    }

    // MARK: - case#2 (happy)：requiresAuth=false → header 无 Authorization（即使 keychain 有 token）
    func testDoesNotInjectAuthorizationHeaderWhenRequiresAuthFalse() async throws {
        let session = MockURLSession()
        session.stubbedResponse = (makeStubResponseBody(), makeHTTPResponse(statusCode: 200))

        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("test-jwt-token-abc")  // 即使有 token

        let client = APIClient(baseURL: baseURL, session: session, keychainStore: keychain)
        let endpoint = Endpoint(path: "/api/v1/auth/guest-login", method: .post, requiresAuth: false)

        let _: EmptyDataMock = try await client.request(endpoint)

        // 1. session.data(for:) 被调过一次
        XCTAssertEqual(session.invocations.count, 1)
        // 2. 该请求 Authorization header 不存在（nil）
        let request = session.invocations.first!
        XCTAssertNil(request.value(forHTTPHeaderField: "Authorization"))
        // 3. keychain.get(forKey:) 一次都不调（false 路径完全跳过 keychain access）
        XCTAssertEqual(keychain.callCount(of: "get(forKey:)"), 0)
    }

    // MARK: - case#3 (edge)：requiresAuth=true 但 Keychain 无 token → 抛 APIError.unauthorized + 不发请求
    func testThrowsUnauthorizedAndDoesNotSendRequestWhenTokenMissing() async {
        let session = MockURLSession()
        // 故意不配 stubbedResponse —— 如果误发请求会抛 .badServerResponse 而非 .unauthorized

        let keychain = MockKeychainStore()
        keychain.getStubResult = .success(nil)  // keychain 无 token

        let client = APIClient(baseURL: baseURL, session: session, keychainStore: keychain)
        let endpoint = Endpoint(path: "/api/v1/home", method: .get, requiresAuth: true)

        do {
            let _: EmptyDataMock = try await client.request(endpoint)
            XCTFail("应抛 APIError.unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际抛 \(error)")
        }

        // 关键断言：session.data(for:) 一次都不调（不浪费网络往返、不让 server 看到伪造请求）
        XCTAssertEqual(session.invocations.count, 0)
        // keychain.get(forKey:) 调过一次（确认 token 状态）
        XCTAssertEqual(keychain.callCount(of: "get(forKey:)"), 1)
    }

    // MARK: - case#4 (edge)：requiresAuth=true 但 keychain 返空字符串 → 视同不存在 → 抛 unauthorized
    func testThrowsUnauthorizedWhenTokenIsEmptyString() async {
        let session = MockURLSession()
        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("")  // 空字符串视同不存在

        let client = APIClient(baseURL: baseURL, session: session, keychainStore: keychain)
        let endpoint = Endpoint(path: "/api/v1/home", method: .get, requiresAuth: true)

        do {
            let _: EmptyDataMock = try await client.request(endpoint)
            XCTFail("应抛 APIError.unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际抛 \(error)")
        }

        XCTAssertEqual(session.invocations.count, 0)
    }

    // MARK: - case#5 (edge)：requiresAuth=true 但 keychain.get 抛错 → 降级为"无 token" → 抛 unauthorized
    func testThrowsUnauthorizedWhenKeychainGetFails() async {
        let session = MockURLSession()
        let keychain = MockKeychainStore()
        keychain.getStubResult = .failure(KeychainError.osStatus(-25300, operation: "get"))

        let client = APIClient(baseURL: baseURL, session: session, keychainStore: keychain)
        let endpoint = Endpoint(path: "/api/v1/home", method: .get, requiresAuth: true)

        do {
            let _: EmptyDataMock = try await client.request(endpoint)
            XCTFail("应抛 APIError.unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized, "keychain 错误应降级为 unauthorized，不透传 KeychainError")
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际抛 \(error)")
        }

        XCTAssertEqual(session.invocations.count, 0)
    }

    // MARK: - case#6 (edge)：requiresAuth=true 但 APIClient 构造时未注入 keychain → 抛 unauthorized
    func testThrowsUnauthorizedWhenKeychainStoreNotInjected() async {
        let session = MockURLSession()

        // APIClient 构造时不传 keychainStore（走默认 nil） —— 模拟"配置错误"
        let client = APIClient(baseURL: baseURL, session: session)
        let endpoint = Endpoint(path: "/api/v1/home", method: .get, requiresAuth: true)

        do {
            let _: EmptyDataMock = try await client.request(endpoint)
            XCTFail("应抛 APIError.unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际抛 \(error)")
        }

        XCTAssertEqual(session.invocations.count, 0)
    }

    // MARK: - case#7 (edge)：同一 APIClient 实例并发 100 个请求 → 都正确注入 header（验证线程安全）
    func testConcurrent100RequestsAllInjectAuthorizationHeaderCorrectly() async throws {
        let session = MockURLSession()
        session.stubbedResponse = (makeStubResponseBody(), makeHTTPResponse(statusCode: 200))

        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("concurrent-test-token")

        let client = APIClient(baseURL: baseURL, session: session, keychainStore: keychain)
        let endpoint = Endpoint(path: "/api/v1/home", method: .get, requiresAuth: true)

        // 并发发 100 个请求
        try await withThrowingTaskGroup(of: Void.self) { group in
            for _ in 0..<100 {
                group.addTask {
                    let _: EmptyDataMock = try await client.request(endpoint)
                }
            }
            try await group.waitForAll()
        }

        // 1. session 总共被调 100 次
        XCTAssertEqual(session.invocations.count, 100)
        // 2. 每次请求 header 都正确注入
        for request in session.invocations {
            XCTAssertEqual(
                request.value(forHTTPHeaderField: "Authorization"),
                "Bearer concurrent-test-token",
                "并发请求应全部注入相同 token；MockURLSession + MockKeychainStore 都是 thread-safe"
            )
        }
        // 3. keychain.get 调用次数 == 100（每次请求都从 keychain 重新读，不缓存）
        // 走 MockBase.callCount(of:) → callCountsSnapshot() → lock 内拷贝
        // （lesson 2026-04-26-mockbase-snapshot-only-reads.md）
        XCTAssertEqual(keychain.callCount(of: "get(forKey:)"), 100)
    }
}

#endif
