// AuthBoundaryAPIClientTests.swift
// Story 0008-impl-1 AC4: AuthBoundaryAPIClient decorator 单元测试.
//
// 替代退役的 AuthRetryingAPIClientTests（silent relogin 三件套已删除）.
//
// 关键验证（与 ADR-0008 v2 §6.2 / Story 0008-impl-1 AC1-AC4 对齐）：
//   1. requiresAuth=true 抛 .unauthorized → sink.trigger() 调用 1 次 + throw .unauthorized
//      caller 看到的是普通失败（**不**做 in-app retry，与退役的 AuthRetryingAPIClient 区别）
//   2. requiresAuth=false 抛 .unauthorized → sink **不**触发 + throw 透传
//      （`/auth/guest-login` 自身 401 → 不能用自己救自己；与退役的 AuthRetrying 同语义边界）
//   3. 5 并发 401 + requiresAuth=true → sink trigger 5 次（每个请求都触发；
//      并发去重在 AppLaunchStateMachine.triggerColdStart() 内部 isRetrying flag 完成，
//      **不**在 AuthBoundary 层做去重 —— 与 ADR §6 设计一致）
//   4. 非 unauthorized 错误（.missingCredentials / .localStoreFailure / .network / .business）→
//      sink **不**触发 + throw 透传
//
// Mock 策略：
//   - inner: StatefulMockAPIClient（按 path 维护 Stub 序列，与退役的 AuthRetryingAPIClientTests 同模式）
//   - sink: 真实 UnauthorizedHandlerSink + 自定义 counting handler（验证调用次数）

import XCTest
@testable import PetApp

#if DEBUG

final class AuthBoundaryAPIClientTests: XCTestCase {

    private struct EmptyData: Decodable, Equatable, Sendable {}
    private struct PingData: Decodable, Equatable, Sendable { let value: String }

    private let authedEndpoint = Endpoint(path: "/api/v1/home", method: .get, body: nil, requiresAuth: true)
    private let unauthedEndpoint = Endpoint(path: "/api/v1/auth/guest-login", method: .post, body: nil, requiresAuth: false)

    /// 测试用 counting handler —— 闭包不易计数，借 actor 维护 invocation count.
    private actor CountingHandler {
        private(set) var invocations: Int = 0
        func record() { invocations += 1 }
    }

    /// 装配 sink + counting handler，返回 (sink, counter).
    private func makeCountingSink() -> (UnauthorizedHandlerSink, CountingHandler) {
        let sink = UnauthorizedHandlerSink()
        let counter = CountingHandler()
        sink.setHandler { @Sendable in
            await counter.record()
        }
        return (sink, counter)
    }

    // MARK: - case#1 (happy)：requiresAuth=true 抛 .unauthorized → sink trigger 1 次 + throw

    func testUnauthorizedOnAuthedEndpointTriggersSinkAndThrows() async {
        let inner = StatefulMockAPIClient()
        inner.responseSequence["/api/v1/home"] = [.failure(.unauthorized)]
        let (sink, counter) = makeCountingSink()
        let wrapped = AuthBoundaryAPIClient(inner: inner, sink: sink)

        do {
            let _: EmptyData = try await wrapped.request(authedEndpoint)
            XCTFail("应抛 .unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际 \(error)")
        }

        XCTAssertEqual(inner.callCount(forPath: "/api/v1/home"), 1, "inner 仅 1 次（**不**in-app retry，与退役 AuthRetryingAPIClient 区别）")
        let invocations = await counter.invocations
        XCTAssertEqual(invocations, 1, "sink trigger 应调 1 次（401 + requiresAuth=true）")
    }

    // MARK: - case#2 (edge)：requiresAuth=false 抛 .unauthorized → sink 不 trigger + 直接抛

    func testUnauthorizedOnUnauthedEndpointDoesNotTriggerSink() async {
        let inner = StatefulMockAPIClient()
        inner.responseSequence["/api/v1/auth/guest-login"] = [.failure(.unauthorized)]
        let (sink, counter) = makeCountingSink()
        let wrapped = AuthBoundaryAPIClient(inner: inner, sink: sink)

        do {
            let _: EmptyData = try await wrapped.request(unauthedEndpoint)
            XCTFail("应抛 .unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际 \(error)")
        }

        XCTAssertEqual(inner.callCount(forPath: "/api/v1/auth/guest-login"), 1)
        let invocations = await counter.invocations
        XCTAssertEqual(invocations, 0, "requiresAuth=false 时**绝不**触发 sink（不能用自己救自己）")
    }

    // MARK: - case#3 (并发)：5 并发 401 + requiresAuth=true → sink trigger 5 次

    func testConcurrentUnauthorizedRequestsTriggerSinkPerRequest() async throws {
        let inner = StatefulMockAPIClient()
        // 5 个 401 stub
        inner.responseSequence["/api/v1/home"] = Array(repeating: .failure(.unauthorized), count: 5)
        let (sink, counter) = makeCountingSink()
        let wrapped = AuthBoundaryAPIClient(inner: inner, sink: sink)

        // 5 并发，全部预期 throw .unauthorized
        await withTaskGroup(of: Void.self) { group in
            for _ in 0..<5 {
                group.addTask {
                    do {
                        let _: EmptyData = try await wrapped.request(self.authedEndpoint)
                        XCTFail("应抛 .unauthorized")
                    } catch let error as APIError {
                        XCTAssertEqual(error, .unauthorized)
                    } catch {
                        XCTFail("应抛 APIError.unauthorized，实际 \(error)")
                    }
                }
            }
            await group.waitForAll()
        }

        XCTAssertEqual(inner.callCount(forPath: "/api/v1/home"), 5, "5 并发都 throw 401，inner 调 5 次")
        let invocations = await counter.invocations
        XCTAssertEqual(invocations, 5,
                       "sink trigger 5 次（每个 401 独立触发；并发去重在 stateMachine.triggerColdStart 内部 isRetrying flag）")
    }

    // MARK: - case#4 (edge)：.missingCredentials / .localStoreFailure / .network / .business → sink 不触发

    func testMissingCredentialsDoesNotTriggerSink() async {
        let inner = StatefulMockAPIClient()
        inner.responseSequence["/api/v1/home"] = [.failure(.missingCredentials)]
        let (sink, counter) = makeCountingSink()
        let wrapped = AuthBoundaryAPIClient(inner: inner, sink: sink)

        do {
            let _: EmptyData = try await wrapped.request(authedEndpoint)
            XCTFail("应抛 .missingCredentials")
        } catch let error as APIError {
            XCTAssertEqual(error, .missingCredentials)
        } catch {
            XCTFail("应抛 APIError.missingCredentials，实际 \(error)")
        }

        let invocations = await counter.invocations
        XCTAssertEqual(invocations, 0, ".missingCredentials 是本地态-terminal，**绝不**触发 cold-start sink")
    }

    func testLocalStoreFailureDoesNotTriggerSink() async {
        let inner = StatefulMockAPIClient()
        let underlying = NSError(domain: "Keychain", code: -25291, userInfo: nil)
        inner.responseSequence["/api/v1/home"] = [.failure(.localStoreFailure(underlying: underlying))]
        let (sink, counter) = makeCountingSink()
        let wrapped = AuthBoundaryAPIClient(inner: inner, sink: sink)

        do {
            let _: EmptyData = try await wrapped.request(authedEndpoint)
            XCTFail("应抛 .localStoreFailure")
        } catch let error as APIError {
            if case .localStoreFailure = error {
                // ok
            } else {
                XCTFail("应抛 .localStoreFailure，实际 \(error)")
            }
        } catch {
            XCTFail("应抛 APIError.localStoreFailure，实际 \(error)")
        }

        let invocations = await counter.invocations
        XCTAssertEqual(invocations, 0, ".localStoreFailure 是本地态-transient，**绝不**触发 cold-start sink")
    }

    func testNetworkErrorDoesNotTriggerSink() async {
        let inner = StatefulMockAPIClient()
        inner.responseSequence["/api/v1/home"] = [.failure(.network(underlying: URLError(.notConnectedToInternet)))]
        let (sink, counter) = makeCountingSink()
        let wrapped = AuthBoundaryAPIClient(inner: inner, sink: sink)

        do {
            let _: EmptyData = try await wrapped.request(authedEndpoint)
            XCTFail("应抛 .network")
        } catch let error as APIError {
            if case .network = error {
                // ok
            } else {
                XCTFail("应抛 .network，实际 \(error)")
            }
        } catch {
            XCTFail("应抛 APIError，实际 \(error)")
        }

        let invocations = await counter.invocations
        XCTAssertEqual(invocations, 0, ".network 错误**绝不**触发 cold-start sink")
    }

    func testBusinessErrorDoesNotTriggerSink() async {
        let inner = StatefulMockAPIClient()
        inner.responseSequence["/api/v1/home"] = [.failure(.business(code: 1009, message: "服务繁忙", requestId: "req-1"))]
        let (sink, counter) = makeCountingSink()
        let wrapped = AuthBoundaryAPIClient(inner: inner, sink: sink)

        do {
            let _: EmptyData = try await wrapped.request(authedEndpoint)
            XCTFail("应抛 .business")
        } catch let error as APIError {
            if case .business(let code, _, _) = error {
                XCTAssertEqual(code, 1009)
            } else {
                XCTFail("应抛 .business，实际 \(error)")
            }
        } catch {
            XCTFail("应抛 APIError.business，实际 \(error)")
        }

        let invocations = await counter.invocations
        XCTAssertEqual(invocations, 0, ".business 错误**绝不**触发 cold-start sink")
    }

    // MARK: - case#5 (sanity)：success 路径不触发 sink

    func testSuccessDoesNotTriggerSink() async throws {
        let inner = StatefulMockAPIClient()
        inner.responseSequence["/api/v1/home"] = [.success(PingData(value: "ok"))]
        let (sink, counter) = makeCountingSink()
        let wrapped = AuthBoundaryAPIClient(inner: inner, sink: sink)

        let result: PingData = try await wrapped.request(authedEndpoint)
        XCTAssertEqual(result, PingData(value: "ok"))

        let invocations = await counter.invocations
        XCTAssertEqual(invocations, 0, "success 路径不触发 sink")
    }

    // MARK: - UnauthorizedHandlerSink 自身契约

    func testSinkTriggerWithoutHandlerIsNoOp() async {
        let sink = UnauthorizedHandlerSink()
        // 不调 setHandler —— handler 仍是 nil
        await sink.trigger()  // 不应 crash
        // 通过即 pass（无 assert）
    }

    func testSinkSetHandlerOverwritesPrevious() async {
        let sink = UnauthorizedHandlerSink()
        let counter1 = CountingHandler()
        let counter2 = CountingHandler()

        sink.setHandler { @Sendable in await counter1.record() }
        sink.setHandler { @Sendable in await counter2.record() }
        await sink.trigger()

        let c1 = await counter1.invocations
        let c2 = await counter2.invocations
        XCTAssertEqual(c1, 0, "第一个 handler 被第二个覆盖，不应被调")
        XCTAssertEqual(c2, 1, "最后注入的 handler 被调")
    }
}

#endif
