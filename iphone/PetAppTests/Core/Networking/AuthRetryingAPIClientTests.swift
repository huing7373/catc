// AuthRetryingAPIClientTests.swift
// Story 5.4 AC9: AuthRetryingAPIClient decorator 单元测试.
// Story 5.4 round 2 [P2] fix：新增 case#7 验证 .missingCredentials（本地态）**不**触发 relogin
//   —— 跟 .unauthorized（server 态）行为严格区分.
//
// 关键验证：
//   1. happy: requiresAuth=true 第一次 401 → coordinator.relogin → 重试 success → 用户感知 0
//   2. edge: requiresAuth=true 第一次 401 → relogin 失败 → 抛上层（不重试 inner）
//   3. edge: requiresAuth=true 第一次 401 → relogin success → 重试**仍** 401 → 抛 unauthorized（不二次 relogin）
//   4. happy: 5 个并发 401 请求 → coordinator coalesce → 只 1 次 relogin + 5 次重试都成功
//   5. edge: requiresAuth=false 抛 unauthorized → **不** relogin → 直接抛上去
//   6. edge: 非 unauthorized 错误（.network / .business）→ **不** relogin → 直接抛
//   7. (round 2) edge: requiresAuth=true 抛 .missingCredentials → **不** relogin → 直接抛
//   8. (round 2) edge: requiresAuth=false 抛 .missingCredentials → **不** relogin → 直接抛
//
// Mock 策略：
//   - inner: 用 StatefulMockAPIClient（按 path 维护 Stub 序列；按调用次数 pop）
//     —— 既有 MockAPIClient 是 path → 单 stub 的 map，不能表达"第 1 次 fail + 第 2 次 success"
//   - coordinator: 用真实 SilentReloginCoordinator + MockSilentReloginUseCase（让 relogin 调用次数可验）

import XCTest
@testable import PetApp

#if DEBUG

final class AuthRetryingAPIClientTests: XCTestCase {

    private struct EmptyData: Decodable, Equatable, Sendable {}
    private struct PingData: Decodable, Equatable, Sendable { let value: String }

    private let authedEndpoint = Endpoint(path: "/api/v1/home", method: .get, body: nil, requiresAuth: true)
    private let unauthedEndpoint = Endpoint(path: "/api/v1/auth/guest-login", method: .post, body: nil, requiresAuth: false)

    // MARK: - case#1 (happy)：requiresAuth=true 第一次 401 → relogin → 重试 success
    func testRetriesOnceAfterUnauthorizedAndSuccess() async throws {
        let inner = StatefulMockAPIClient()
        inner.responseSequence["/api/v1/home"] = [
            .failure(.unauthorized),
            .success(PingData(value: "after-relogin"))
        ]
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("new-token-1")
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)
        let wrapped = AuthRetryingAPIClient(inner: inner, coordinator: coordinator)

        let result: PingData = try await wrapped.request(authedEndpoint)

        XCTAssertEqual(result, PingData(value: "after-relogin"))
        XCTAssertEqual(inner.callCount(forPath: "/api/v1/home"), 2, "inner 应被调 2 次（原始 + 重试）")
        XCTAssertEqual(mockUseCase.callCount(of: "execute()"), 1, "relogin 应触发 1 次")
    }

    // MARK: - case#2 (edge)：requiresAuth=true 第一次 401 → relogin 失败 → 抛上层
    func testReloginFailureIsThrownAndInnerNotRetried() async {
        let inner = StatefulMockAPIClient()
        inner.responseSequence["/api/v1/home"] = [
            .failure(.unauthorized),
            .success(EmptyData())  // 即使配了 success，relogin 失败后也不该走到这里
        ]
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .failure(APIError.network(underlying: URLError(.notConnectedToInternet)))
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)
        let wrapped = AuthRetryingAPIClient(inner: inner, coordinator: coordinator)

        do {
            let _: EmptyData = try await wrapped.request(authedEndpoint)
            XCTFail("应抛错")
        } catch let error as APIError {
            if case .network = error {
                // ok
            } else {
                XCTFail("应抛 .network，实际 \(error)")
            }
        } catch {
            XCTFail("应抛 APIError.network，实际 \(error)")
        }

        XCTAssertEqual(inner.callCount(forPath: "/api/v1/home"), 1, "inner 仅调 1 次（原始失败 + relogin 失败 → 不重试）")
        XCTAssertEqual(mockUseCase.callCount(of: "execute()"), 1)
    }

    // MARK: - case#3 (edge)：第一次 401 → relogin success → 重试**仍** 401 → 不二次 relogin
    func testRetryStillUnauthorizedDoesNotTriggerSecondRelogin() async {
        let inner = StatefulMockAPIClient()
        inner.responseSequence["/api/v1/home"] = [
            .failure(.unauthorized),  // 原始
            .failure(.unauthorized),  // 重试也失败
            .success(EmptyData())     // 第 3 次：永远不会被调
        ]
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("new-token-1")
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)
        let wrapped = AuthRetryingAPIClient(inner: inner, coordinator: coordinator)

        do {
            let _: EmptyData = try await wrapped.request(authedEndpoint)
            XCTFail("应抛 unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际 \(error)")
        }

        XCTAssertEqual(inner.callCount(forPath: "/api/v1/home"), 2, "inner 应调 2 次（原始 + 重试 1 次，**不**第 3 次）")
        XCTAssertEqual(mockUseCase.callCount(of: "execute()"), 1, "relogin 仅调 1 次（重试失败后**不**二次重登）")
    }

    // MARK: - case#4 (happy)：5 并发 401 → coordinator coalesce → 1 次 relogin + 5 次重试 success
    func testConcurrentUnauthorizedRequestsCoalesceReloginAndAllRetrySucceed() async throws {
        let inner = StatefulMockAPIClient()
        // 每个并发请求的"序列"是独立的 path → [stub] —— 但 5 个请求都打同一 path
        // 必须让 stub 可以按"全局调用次数" pop（StatefulMockAPIClient 内部用 lock 保护序列）
        // 配 10 个 stub：5 个 .failure(.unauthorized) + 5 个 .success(...)
        inner.responseSequence["/api/v1/home"] = [
            .failure(.unauthorized), .failure(.unauthorized), .failure(.unauthorized),
            .failure(.unauthorized), .failure(.unauthorized),
            .success(EmptyData()), .success(EmptyData()), .success(EmptyData()),
            .success(EmptyData()), .success(EmptyData())
        ]
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("new-token-1")
        mockUseCase.artificialDelayMs = 50  // 让 5 个并发请求都进入"等待既存 task"路径
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)
        let wrapped = AuthRetryingAPIClient(inner: inner, coordinator: coordinator)

        // 5 并发
        try await withThrowingTaskGroup(of: Void.self) { group in
            for _ in 0..<5 {
                group.addTask {
                    let _: EmptyData = try await wrapped.request(self.authedEndpoint)
                }
            }
            try await group.waitForAll()
        }

        XCTAssertEqual(inner.callCount(forPath: "/api/v1/home"), 10, "5 并发 → 5 原始失败 + 5 重试 = 10 次 inner")
        XCTAssertEqual(
            mockUseCase.callCount(of: "execute()"),
            1,
            "5 并发 401 应 coalesce 到 1 次 relogin"
        )
    }

    // MARK: - case#5 (edge)：requiresAuth=false 抛 unauthorized → 不 relogin
    func testRequiresAuthFalseUnauthorizedDoesNotTriggerRelogin() async {
        let inner = StatefulMockAPIClient()
        inner.responseSequence["/api/v1/auth/guest-login"] = [.failure(.unauthorized)]
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("new-token-1")
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)
        let wrapped = AuthRetryingAPIClient(inner: inner, coordinator: coordinator)

        do {
            let _: EmptyData = try await wrapped.request(unauthedEndpoint)
            XCTFail("应抛 unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际 \(error)")
        }

        XCTAssertEqual(inner.callCount(forPath: "/api/v1/auth/guest-login"), 1, "inner 仅 1 次（不重试）")
        XCTAssertEqual(mockUseCase.callCount(of: "execute()"), 0, "requiresAuth=false 时**绝不**触发 relogin")
    }

    // MARK: - case#6 (edge)：非 unauthorized 错误（.network）→ 不 relogin
    func testNonUnauthorizedErrorsDoNotTriggerRelogin() async {
        let inner = StatefulMockAPIClient()
        inner.responseSequence["/api/v1/home"] = [.failure(.network(underlying: URLError(.notConnectedToInternet)))]
        let mockUseCase = MockSilentReloginUseCase()
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)
        let wrapped = AuthRetryingAPIClient(inner: inner, coordinator: coordinator)

        do {
            let _: EmptyData = try await wrapped.request(authedEndpoint)
            XCTFail("应抛 network")
        } catch let error as APIError {
            if case .network = error {
                // ok
            } else {
                XCTFail("应抛 .network，实际 \(error)")
            }
        } catch {
            XCTFail("应抛 APIError，实际 \(error)")
        }

        XCTAssertEqual(inner.callCount(forPath: "/api/v1/home"), 1, "inner 仅 1 次")
        XCTAssertEqual(mockUseCase.callCount(of: "execute()"), 0, ".network 错误**绝不**触发 relogin")
    }

    // MARK: - case#7 (edge)：requiresAuth=true 抛 .missingCredentials → **不** relogin → 直接抛
    // Story 5.4 round 2 [P2] fix —— 关键回归保护：
    //   round 1 实装把 buildURLRequest 阶段的 .unauthorized 也送进 relogin 路径，
    //   会屏蔽 cold-start / 配置错信号；本 case 锁死 .missingCredentials **绝不**触发 relogin.
    func testMissingCredentialsOnAuthedEndpointDoesNotTriggerRelogin() async {
        let inner = StatefulMockAPIClient()
        // 模拟 APIClient.buildURLRequest 抛 .missingCredentials（本地无 token / keychain 配置错）
        inner.responseSequence["/api/v1/home"] = [.failure(.missingCredentials)]
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("new-token-1")  // 配了也不该被调
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)
        let wrapped = AuthRetryingAPIClient(inner: inner, coordinator: coordinator)

        do {
            let _: EmptyData = try await wrapped.request(authedEndpoint)
            XCTFail("应抛 missingCredentials")
        } catch let error as APIError {
            XCTAssertEqual(error, .missingCredentials, "本地态应原样透传，**不**被翻译成 unauthorized")
        } catch {
            XCTFail("应抛 APIError.missingCredentials，实际 \(error)")
        }

        XCTAssertEqual(inner.callCount(forPath: "/api/v1/home"), 1, "inner 仅 1 次（不重试）")
        XCTAssertEqual(
            mockUseCase.callCount(of: "execute()"),
            0,
            ".missingCredentials **绝不**触发 relogin —— 这是跟 .unauthorized 的核心行为差"
        )
    }

    // MARK: - case#8 (edge)：requiresAuth=false 抛 .missingCredentials → **不** relogin（防御性）
    // 即使 endpoint.requiresAuth==false 路径上抛了 .missingCredentials（理论上不会发生 —
    // APIClient false 路径跳过 keychain access；但放断言锁死语义边界）.
    func testMissingCredentialsOnUnauthedEndpointDoesNotTriggerRelogin() async {
        let inner = StatefulMockAPIClient()
        inner.responseSequence["/api/v1/auth/guest-login"] = [.failure(.missingCredentials)]
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("new-token-1")
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)
        let wrapped = AuthRetryingAPIClient(inner: inner, coordinator: coordinator)

        do {
            let _: EmptyData = try await wrapped.request(unauthedEndpoint)
            XCTFail("应抛 missingCredentials")
        } catch let error as APIError {
            XCTAssertEqual(error, .missingCredentials)
        } catch {
            XCTFail("应抛 APIError.missingCredentials，实际 \(error)")
        }

        XCTAssertEqual(inner.callCount(forPath: "/api/v1/auth/guest-login"), 1)
        XCTAssertEqual(mockUseCase.callCount(of: "execute()"), 0)
    }
}

#endif
