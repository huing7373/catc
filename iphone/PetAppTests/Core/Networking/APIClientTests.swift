// APIClientTests.swift
// Story 2.4 AC7：APIClient 单元测试，mock URLSession 切口注入。
//
// 覆盖 8 个 case：
// 1. happy: 200 + envelope code=0 + data 完整 → 返回 typed T
// 2. edge: 200 + envelope code=1002（参数错误）→ throw APIError.business
// 3. edge: HTTP 401 → throw APIError.unauthorized
// 4. edge: 200 + envelope code=1001 → throw APIError.unauthorized（envelope-level 401 别名）
// 5. edge: URLSession throw URLError(.timedOut) → throw APIError.network
// 6. edge: 200 + body 不是合法 envelope JSON → throw APIError.decoding
// 7. edge: 200 + envelope code=0 + data 字段为 null → throw APIError.decoding
// 8. happy: POST + body 编码 → URLRequest.httpBody 正确填充 + Content-Type 写对

import XCTest
@testable import PetApp

@MainActor
final class APIClientTests: XCTestCase {

    // 共用 baseURL（http://localhost:8080/api/v1）—— 测试中不真实发起请求，由 MockURLSession 拦截
    private let baseURL = URL(string: "http://localhost:8080/api/v1")!

    // 解码目标：mimic V1 ping/version data 字段
    private struct PingResponseMock: Decodable, Equatable {
        let version: String
    }

    // POST body 测试用（Codable：测试中用 encode 写 body，再 decode 验证写入正确）
    private struct LoginRequestMock: Codable, Equatable {
        let deviceId: String
    }

    private func makeClient(session: URLSessionProtocol) -> APIClient {
        APIClient(baseURL: baseURL, session: session)
    }

    private func makeHTTPResponse(statusCode: Int) -> HTTPURLResponse {
        HTTPURLResponse(
            url: baseURL,
            statusCode: statusCode,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        )!
    }

    // MARK: - case#1: happy path → typed data
    func testRequestReturnsDecodedDataOnSuccess() async throws {
        let session = MockURLSession()
        let body = """
        {"code":0,"message":"ok","data":{"version":"v1.0.0"},"requestId":"req_abc"}
        """.data(using: .utf8)!
        session.stubbedResponse = (body, makeHTTPResponse(statusCode: 200))

        let client = makeClient(session: session)
        let endpoint = Endpoint(path: "/version", method: .get, requiresAuth: false)

        let result: PingResponseMock = try await client.request(endpoint)

        XCTAssertEqual(result, PingResponseMock(version: "v1.0.0"))
        XCTAssertEqual(session.invocations.count, 1)
        XCTAssertEqual(session.invocations.first?.httpMethod, "GET")
        XCTAssertEqual(session.invocations.first?.url?.absoluteString,
                       "http://localhost:8080/api/v1/version")
    }

    // MARK: - case#2: envelope code != 0 → APIError.business
    func testRequestThrowsBusinessErrorWhenEnvelopeCodeNonZero() async throws {
        let session = MockURLSession()
        let body = """
        {"code":1002,"message":"参数错误","requestId":"req_def"}
        """.data(using: .utf8)!
        session.stubbedResponse = (body, makeHTTPResponse(statusCode: 200))

        let client = makeClient(session: session)
        let endpoint = Endpoint(path: "/version", method: .get, requiresAuth: false)

        do {
            let _: PingResponseMock = try await client.request(endpoint)
            XCTFail("expected throw APIError.business but got success")
        } catch let error as APIError {
            XCTAssertEqual(
                error,
                APIError.business(code: 1002, message: "参数错误", requestId: "req_def")
            )
        } catch {
            XCTFail("unexpected error type: \(error)")
        }
    }

    // MARK: - case#3: HTTP 401 → APIError.unauthorized
    func testRequestThrowsUnauthorizedOnHttp401() async throws {
        let session = MockURLSession()
        // body 内容随便，APIClient 不会解 401 的 body
        session.stubbedResponse = (Data("nginx 401 page".utf8),
                                   makeHTTPResponse(statusCode: 401))

        let client = makeClient(session: session)
        let endpoint = Endpoint(path: "/home", method: .get, requiresAuth: true)

        do {
            let _: PingResponseMock = try await client.request(endpoint)
            XCTFail("expected throw APIError.unauthorized but got success")
        } catch let error as APIError {
            XCTAssertEqual(error, APIError.unauthorized)
        } catch {
            XCTFail("unexpected error type: \(error)")
        }
    }

    // MARK: - case#4: envelope code=1001 → APIError.unauthorized（envelope-level 别名）
    func testRequestThrowsUnauthorizedWhenEnvelopeCodeIs1001() async throws {
        let session = MockURLSession()
        let body = """
        {"code":1001,"message":"未登录","requestId":"req_unauth"}
        """.data(using: .utf8)!
        session.stubbedResponse = (body, makeHTTPResponse(statusCode: 200))

        let client = makeClient(session: session)
        let endpoint = Endpoint(path: "/home", method: .get, requiresAuth: true)

        do {
            let _: PingResponseMock = try await client.request(endpoint)
            XCTFail("expected throw APIError.unauthorized but got success")
        } catch let error as APIError {
            XCTAssertEqual(error, APIError.unauthorized)
        } catch {
            XCTFail("unexpected error type: \(error)")
        }
    }

    // MARK: - case#5: URLSession throw URLError → APIError.network
    func testRequestThrowsNetworkErrorOnURLSessionFailure() async throws {
        let session = MockURLSession()
        session.stubbedError = URLError(.timedOut)

        let client = makeClient(session: session)
        let endpoint = Endpoint(path: "/version", method: .get, requiresAuth: false)

        do {
            let _: PingResponseMock = try await client.request(endpoint)
            XCTFail("expected throw APIError.network but got success")
        } catch let error as APIError {
            XCTAssertEqual(
                error,
                APIError.network(underlying: URLError(.timedOut))
            )
        } catch {
            XCTFail("unexpected error type: \(error)")
        }
    }

    // MARK: - case#6: 200 + invalid JSON → APIError.decoding
    func testRequestThrowsDecodingErrorOnInvalidEnvelopeBody() async throws {
        let session = MockURLSession()
        let body = Data("not a valid json".utf8)
        session.stubbedResponse = (body, makeHTTPResponse(statusCode: 200))

        let client = makeClient(session: session)
        let endpoint = Endpoint(path: "/version", method: .get, requiresAuth: false)

        do {
            let _: PingResponseMock = try await client.request(endpoint)
            XCTFail("expected throw APIError.decoding but got success")
        } catch let error as APIError {
            // 用 switch 校验类型——underlying 是 DecodingError，深度比较过于脆弱
            switch error {
            case .decoding:
                break  // pass
            default:
                XCTFail("expected .decoding, got \(error)")
            }
        } catch {
            XCTFail("unexpected error type: \(error)")
        }
    }

    // MARK: - case#7: 200 + code=0 + data null → APIError.decoding
    func testRequestThrowsDecodingErrorOnSuccessWithNullData() async throws {
        let session = MockURLSession()
        let body = """
        {"code":0,"message":"ok","data":null,"requestId":"req_null"}
        """.data(using: .utf8)!
        session.stubbedResponse = (body, makeHTTPResponse(statusCode: 200))

        let client = makeClient(session: session)
        let endpoint = Endpoint(path: "/version", method: .get, requiresAuth: false)

        do {
            let _: PingResponseMock = try await client.request(endpoint)
            XCTFail("expected throw APIError.decoding but got success")
        } catch let error as APIError {
            switch error {
            case .decoding:
                break  // pass
            default:
                XCTFail("expected .decoding, got \(error)")
            }
        } catch {
            XCTFail("unexpected error type: \(error)")
        }
    }

    // MARK: - case#8: POST + body 编码 → httpBody / Content-Type 正确
    func testPostRequestEncodesBodyAndSetsContentType() async throws {
        let session = MockURLSession()
        let body = """
        {"code":0,"message":"ok","data":{"version":"v1.0.0"},"requestId":"req_post"}
        """.data(using: .utf8)!
        session.stubbedResponse = (body, makeHTTPResponse(statusCode: 200))

        let client = makeClient(session: session)
        let request = LoginRequestMock(deviceId: "iphone-test-001")
        let endpoint = Endpoint(
            path: "/auth/guest-login",
            method: .post,
            body: AnyEncodable(request),
            requiresAuth: false
        )

        let _: PingResponseMock = try await client.request(endpoint)

        // 断言 1：发起的 URLRequest 是 POST
        XCTAssertEqual(session.invocations.count, 1)
        let invocation = try XCTUnwrap(session.invocations.first)
        XCTAssertEqual(invocation.httpMethod, "POST")

        // 断言 2：Content-Type 写对
        XCTAssertEqual(
            invocation.value(forHTTPHeaderField: "Content-Type"),
            "application/json"
        )
        XCTAssertEqual(
            invocation.value(forHTTPHeaderField: "Accept"),
            "application/json"
        )

        // 断言 3：httpBody 是有效 JSON 且能解回 deviceId
        let bodyData = try XCTUnwrap(invocation.httpBody)
        let decoded = try JSONDecoder().decode(LoginRequestMock.self, from: bodyData)
        XCTAssertEqual(decoded, request)
    }
}
