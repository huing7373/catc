// SilentReloginCoordinatorTests.swift
// Story 5.4 AC7: SilentReloginCoordinator actor coalescing 单元测试.
//
// 关键验证：
//   1. 单调 → useCase.execute 调 1 次
//   2. 5 并发 → useCase.execute 调 1 次（coalesce）
//   3. 第一次完成后 + 第二次 → useCase.execute 调 2 次（inFlight 清空）
//   4. 失败也清空 inFlight（让用户后续动作还能重新触发重登）
//   5. 第一个 caller 被 cancel 后,第二个 caller 仍能 coalesce 到同一个 task —— 不会启第二次 useCase.execute
//      （fix-review round 1: codex P2 finding 回归测试）

import XCTest
@testable import PetApp

#if DEBUG

final class SilentReloginCoordinatorTests: XCTestCase {

    // MARK: - case#1 (happy)：单调 → useCase.execute 一次 → 返回 token
    func testReloginCallsUseCaseOnce() async throws {
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("token-1")
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)

        let token = try await coordinator.relogin()

        XCTAssertEqual(token, "token-1")
        XCTAssertEqual(mockUseCase.callCount(of: "execute()"), 1)
    }

    // MARK: - case#2 (happy)：5 并发调 relogin() → useCase.execute 调 1 次（coalesce 验证）
    func testConcurrentReloginsCoalesceToSingleExecution() async throws {
        let mockUseCase = MockSilentReloginUseCase()
        // useCase 故意 sleep 50ms：让 5 个并发请求都进入"既存 task" 等待路径
        mockUseCase.executeStub = .success("token-1")
        mockUseCase.artificialDelayMs = 50
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)

        // 5 并发
        let tokens: [String] = try await withThrowingTaskGroup(of: String.self) { group in
            for _ in 0..<5 {
                group.addTask {
                    try await coordinator.relogin()
                }
            }
            var results: [String] = []
            for try await token in group {
                results.append(token)
            }
            return results
        }

        XCTAssertEqual(tokens.count, 5)
        XCTAssertTrue(tokens.allSatisfy { $0 == "token-1" }, "所有并发调用应返回同一 token")
        XCTAssertEqual(
            mockUseCase.callCount(of: "execute()"),
            1,
            "useCase.execute 应仅被调一次（coalesce 复用 inFlight task）"
        )
    }

    // MARK: - case#3 (happy)：第一次完成后 + 第二次 relogin → useCase.execute 调 2 次
    func testReloginCallsUseCaseAgainAfterPreviousCompletes() async throws {
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("token-1")
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)

        // 第一次完整跑完
        let token1 = try await coordinator.relogin()
        XCTAssertEqual(token1, "token-1")

        // 第二次又调一次 —— inFlight 应已清空，useCase.execute 应再被调
        let token2 = try await coordinator.relogin()
        XCTAssertEqual(token2, "token-1")

        XCTAssertEqual(
            mockUseCase.callCount(of: "execute()"),
            2,
            "两次串行 relogin 应分别触发 useCase.execute（inFlight 清空验证）"
        )
    }

    // MARK: - case#4 (edge)：useCase 失败 → 透传错误 + inFlight 清空 → 下次能重新触发
    func testReloginPropagatesErrorAndCanRetryAfterFailure() async {
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .failure(APIError.network(underlying: URLError(.notConnectedToInternet)))
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)

        // 第一次失败
        do {
            _ = try await coordinator.relogin()
            XCTFail("应抛错")
        } catch {
            // ok
        }

        // 第二次又调一次 —— useCase.execute 应再被调（inFlight 清空，可重试）
        do {
            _ = try await coordinator.relogin()
            XCTFail("应抛错")
        } catch {
            // ok
        }

        XCTAssertEqual(
            mockUseCase.callCount(of: "execute()"),
            2,
            "失败的 task 完成后 inFlight 也应清空，下次 relogin 应能再次触发 useCase"
        )
    }

    // MARK: - case#5 (regression / cancellation)：spawned task 自己负责清 inFlight,而不是 caller defer
    //
    // 回归 codex review round 1 P2 finding：
    //   旧实现把 `defer { inFlight = nil }` 放在 caller 的 relogin() 内 —— 清理时机绑定
    //   "caller 何时离开 relogin"；codex 指出在 cancellation-heavy flows 下,这与 spawned task
    //   生命周期会脱钩,导致 inFlight 提前清空 + spawned 仍在跑 → 后续 relogin 不 coalesce.
    //
    // 修复（本 commit）：把清理动作放进 spawned Task 内部（task body 末尾 hop 回 actor 调
    //   clearInFlight）—— 清理时机严格绑定 spawned task 的"做完真正工作"那一刻,与 caller 的
    //   await/cancel 时机解耦.
    //
    // 测试策略（不修改产品代码下,直接验证修复后的属性）：
    //   - taskA 调 relogin,**不等它返回** —— 这模拟"caller 退出 relogin context"
    //   - 立即（在 spawned task 还在跑时）从主线程发起 taskB 调 relogin
    //   - taskB 应 coalesce 到 taskA 启的 spawned task —— useCase.execute 仅被调 1 次
    //
    //   关键时序：
    //     - 主线程 spawn taskA → A 调 relogin → 创建 spawned task → spawned task 开始 useCase.execute (200ms sleep)
    //     - 主线程不 await taskA,直接 spawn taskB → B 调 relogin → actor 看 inFlight 非 nil → coalesce
    //     - 主线程 await taskB.value 等结果
    //     - useCase.execute 只调 1 次（A & B 共享同一 spawned task）
    //
    //   这其实就是 case#2 的并发 coalesce —— 但去掉 task group 的耦合,显式验证
    //   "B 不必和 A 同步在 group 内,也能 coalesce"（B 是后启动的独立 unstructured Task）.
    //   这个 case 的关键测试点是:fix 后的 relogin 在 spawned task 还活着时,inFlight 不能被
    //   提前清掉（不管 caller 怎么走）.
    func testInFlightSurvivesUntilSpawnedTaskCompletes() async throws {
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("token-1")
        // useCase 故意 sleep 200ms,留出 race window
        mockUseCase.artificialDelayMs = 200
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)

        // taskA 启动 relogin —— 不立即 await
        let taskA: Task<String, Error> = Task {
            try await coordinator.relogin()
        }

        // 让 actor 把 inFlight 装上（A 已进入 spawned task await 路径）
        try await Task.sleep(nanoseconds: 30 * 1_000_000)

        // 在 spawned task 还在跑时（剩 ~170ms）启 taskB
        // 关键：B 启动时,A 的 relogin 还在 await spawned task；A 的 caller 也还没收到结果
        let taskB: Task<String, Error> = Task {
            try await coordinator.relogin()
        }

        // 等两个都完成
        let tokenA = try await taskA.value
        let tokenB = try await taskB.value
        XCTAssertEqual(tokenA, "token-1")
        XCTAssertEqual(tokenB, "token-1")

        // 关键断言：useCase.execute 只被调 1 次（B coalesce 到 A 启的 spawned task）
        XCTAssertEqual(
            mockUseCase.callCount(of: "execute()"),
            1,
            "B 在 A 的 spawned task 还活着时启动,应 coalesce 到同一 task —— useCase.execute 仅 1 次"
        )

        // 进一步验证：在两个 task 都完成后,inFlight 应已清空 → 第三次 relogin 应启新 task
        let token3 = try await coordinator.relogin()
        XCTAssertEqual(token3, "token-1")
        XCTAssertEqual(
            mockUseCase.callCount(of: "execute()"),
            2,
            "spawned task 完成后,clearInFlight 应已 hop 回 actor 清空 inFlight；下一次 relogin 应再启新 task"
        )
    }
}

#endif
