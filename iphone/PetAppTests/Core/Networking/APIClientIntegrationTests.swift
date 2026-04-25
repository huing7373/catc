// APIClientIntegrationTests.swift
// Story 2.4 AC8：APIClient 集成测试。
//
// 路径与单测互补：单测注入 mock URLSessionProtocol 不真走 URLSession；
// 本集成测试用真 URLSession + StubURLProtocol 拦截，确保 URLSession middleware /
// JSONDecoder / URLProtocol 三件套联调正确。
//
// 注意 register / unregister / reset 严格成对——StubURLProtocol 的 stub 是 static 全局状态。

import XCTest
@testable import PetApp

@MainActor
final class APIClientIntegrationTests: XCTestCase {

    private let baseURL = URL(string: "http://test-server.local/api/v1")!

    private struct VersionResponse: Decodable, Equatable {
        let version: String
    }

    override func setUp() {
        super.setUp()
        StubURLProtocol.reset()
        URLProtocol.registerClass(StubURLProtocol.self)
    }

    override func tearDown() {
        URLProtocol.unregisterClass(StubURLProtocol.self)
        StubURLProtocol.reset()
        super.tearDown()
    }

    private func makeClient() -> APIClient {
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [StubURLProtocol.self]
        let session = URLSession(configuration: config)
        return APIClient(baseURL: baseURL, session: session)
    }

    /// 真 URLSession（注入 StubURLProtocol）+ APIClient → 解出 typed data
    func testFullStackHappyPath() async throws {
        // GIVEN: stub 返回 envelope code=0 + data={"version":"v1.0.0"}
        StubURLProtocol.stubData = """
        {"code":0,"message":"ok","data":{"version":"v1.0.0"},"requestId":"req_abc"}
        """.data(using: .utf8)!
        StubURLProtocol.stubStatusCode = 200

        let client = makeClient()
        let endpoint = Endpoint(path: "/version", method: .get, requiresAuth: false)

        // WHEN
        let result: VersionResponse = try await client.request(endpoint)

        // THEN
        XCTAssertEqual(result, VersionResponse(version: "v1.0.0"))
    }

    /// 真 URLSession + APIClient + envelope code=1004 → throw APIError.business
    func testFullStackBusinessError() async throws {
        // GIVEN: stub 返回 envelope code=1004 业务错误
        StubURLProtocol.stubData = """
        {"code":1004,"message":"操作太频繁","requestId":"req_busy"}
        """.data(using: .utf8)!
        StubURLProtocol.stubStatusCode = 200

        let client = makeClient()
        let endpoint = Endpoint(path: "/chest/open", method: .post,
                                body: nil, requiresAuth: false)

        // WHEN / THEN
        do {
            let _: VersionResponse = try await client.request(endpoint)
            XCTFail("expected throw APIError.business but got success")
        } catch let error as APIError {
            XCTAssertEqual(
                error,
                APIError.business(code: 1004, message: "操作太频繁", requestId: "req_busy")
            )
        } catch {
            XCTFail("unexpected error type: \(error)")
        }
    }
}
