// SilentReloginCoordinatorTests.swift
// Story 5.4 AC7: SilentReloginCoordinator actor coalescing 单元测试.
//
// 关键验证：
//   1. 单调 → useCase.execute 调 1 次
//   2. 5 并发 → useCase.execute 调 1 次（inFlight coalesce 路径）
//   3. 第一次完成后 + 第二次 → useCase.execute 调 2 次（inFlight 清空 + caller 也用新 generation）
//   4. 失败也清空 inFlight（让用户后续动作还能重新触发重登）+ generation **不**推进
//   5. 第一个 caller 被 cancel 后,第二个 caller 仍能 coalesce 到同一个 task —— 不会启第二次 useCase.execute
//      （fix-review round 1: codex P2 finding 回归测试）
//   6. **stale-401 dedup**：A 完成后清空 inFlight,B 此时才进 relogin（带 pre-refresh snapshot generation）
//      → 应直接拿到 A 的 lastIssuedToken,不启第二次 useCase
//      （fix-review round 3: codex P2 finding 回归测试）
//   7. callerGeneration == 当前 generation → 仍然走正常重登路径（generation 路径只在"超过"时短路）

import XCTest
@testable import PetApp

#if DEBUG

final class SilentReloginCoordinatorTests: XCTestCase {

    // MARK: - case#1 (happy)：单调 → useCase.execute 一次 → 返回 token
    func testReloginCallsUseCaseOnce() async throws {
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("token-1")
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)
        let gen = await coordinator.currentGeneration()

        let token = try await coordinator.relogin(callerGeneration: gen)

        XCTAssertEqual(token, "token-1")
        XCTAssertEqual(mockUseCase.callCount(of: "execute()"), 1)
    }

    // MARK: - case#2 (happy)：5 并发调 relogin() → useCase.execute 调 1 次（inFlight coalesce 路径）
    func testConcurrentReloginsCoalesceToSingleExecution() async throws {
        let mockUseCase = MockSilentReloginUseCase()
        // useCase 故意 sleep 50ms：让 5 个并发请求都进入"既存 task" 等待路径
        mockUseCase.executeStub = .success("token-1")
        mockUseCase.artificialDelayMs = 50
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)
        let gen = await coordinator.currentGeneration()

        // 5 并发
        let tokens: [String] = try await withThrowingTaskGroup(of: String.self) { group in
            for _ in 0..<5 {
                group.addTask {
                    try await coordinator.relogin(callerGeneration: gen)
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
    // 注意：caller 每次都重新 snapshot generation —— 即"caller 知道上次已经 refresh 过了,这次是新一波 401",
    // 所以不走 stale dedup 路径,走正常重登路径.
    func testReloginCallsUseCaseAgainAfterPreviousCompletes() async throws {
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("token-1")
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)

        // 第一次完整跑完
        let gen1 = await coordinator.currentGeneration()
        let token1 = try await coordinator.relogin(callerGeneration: gen1)
        XCTAssertEqual(token1, "token-1")

        // 第二次又调一次 —— caller 重新 snapshot generation（已 ++ 到 1）→ 走正常路径,启新 useCase
        let gen2 = await coordinator.currentGeneration()
        XCTAssertEqual(gen2, 1, "成功完成一次后,generation 应 ++ 到 1")
        let token2 = try await coordinator.relogin(callerGeneration: gen2)
        XCTAssertEqual(token2, "token-1")

        XCTAssertEqual(
            mockUseCase.callCount(of: "execute()"),
            2,
            "两次串行 relogin 应分别触发 useCase.execute（每次 caller 用最新 snapshot）"
        )
    }

    // MARK: - case#4 (edge)：useCase 失败 → 透传错误 + inFlight 清空 + generation **不**推进
    func testReloginPropagatesErrorAndCanRetryAfterFailure() async {
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .failure(APIError.network(underlying: URLError(.notConnectedToInternet)))
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)

        // 第一次失败
        let gen1 = await coordinator.currentGeneration()
        do {
            _ = try await coordinator.relogin(callerGeneration: gen1)
            XCTFail("应抛错")
        } catch {
            // ok
        }

        // 关键：失败**不**推进 generation —— 后续 caller 不能因为 generation 偏移而跳过自己的重登
        let genAfterFail = await coordinator.currentGeneration()
        XCTAssertEqual(genAfterFail, 0, "失败的 task 完成不应推进 generation")

        // 第二次又调一次 —— useCase.execute 应再被调（inFlight 清空 + generation 仍 0）
        do {
            _ = try await coordinator.relogin(callerGeneration: genAfterFail)
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
    // 修复（round 1 fix）：把清理动作放进 spawned Task 内部（task body 末尾 hop 回 actor 调
    //   finishInFlight）—— 清理时机严格绑定 spawned task 的"做完真正工作"那一刻,与 caller 的
    //   await/cancel 时机解耦.
    func testInFlightSurvivesUntilSpawnedTaskCompletes() async throws {
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("token-1")
        // useCase 故意 sleep 200ms,留出 race window
        mockUseCase.artificialDelayMs = 200
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)
        let gen = await coordinator.currentGeneration()

        // taskA 启动 relogin —— 不立即 await
        let taskA: Task<String, Error> = Task {
            try await coordinator.relogin(callerGeneration: gen)
        }

        // 让 actor 把 inFlight 装上（A 已进入 spawned task await 路径）
        try await Task.sleep(nanoseconds: 30 * 1_000_000)

        // 在 spawned task 还在跑时（剩 ~170ms）启 taskB,**用与 A 相同的 callerGeneration**
        // → B 走 inFlight 路径 coalesce 到 A 的 spawned task
        let taskB: Task<String, Error> = Task {
            try await coordinator.relogin(callerGeneration: gen)
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

        // 进一步验证：在两个 task 都完成后,inFlight 应已清空 → 第三次 relogin（重新 snapshot）应启新 task
        let gen2 = await coordinator.currentGeneration()
        let token3 = try await coordinator.relogin(callerGeneration: gen2)
        XCTAssertEqual(token3, "token-1")
        XCTAssertEqual(
            mockUseCase.callCount(of: "execute()"),
            2,
            "spawned task 完成后,finishInFlight 应已 hop 回 actor 清空 inFlight；下一次 relogin 应再启新 task"
        )
    }

    // MARK: - case#6 (regression / stale-401 dedup)：generation 路径
    //
    // 回归 codex review round 3 P2 finding：
    //   旧实现只通过 inFlight 字段做 coalesce. 时序：
    //     - caller A 携 stale token T1 → server 401 → 进 relogin → 启 spawned task → useCase.execute → T2
    //       → finishInFlight → A 拿 T2 → inFlight 清空
    //     - caller B 同样携 stale T1 早就发了请求,server 401 响应到达 client 较慢,**晚于 A 完成** →
    //       B 进 relogin → 看到 inFlight=nil → 启第二个 spawned task → useCase.execute（多余）
    //   违反 "concurrent 401s trigger one relogin" 契约.
    //
    // 修复（round 3 fix）：caller 在调 relogin 之前 snapshot generation —— B 进入时
    //   coordinator.generation > B 的 snapshot → 直接返 lastIssuedToken,不启第二次 useCase.
    //
    // 测试编排（不依赖真实 keychain / 真实网络）：
    //   1. 两个 caller 都先 snapshot gen=0
    //   2. caller A 调 relogin(0) → spawned task 跑 → 完成后 generation=1 + lastIssuedToken=T1
    //   3. caller B **此时才**调 relogin(0) —— 0 < 1 → 应直接返 T1,不启 useCase
    //   4. 断言 useCase.execute 仅 1 次,B 拿到 T1
    func testStaleConcurrent401DoesNotTriggerSecondReloginViaGenerationDedup() async throws {
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("token-1")
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)

        // 两个 caller 都基于初始 gen=0 准备
        let preGenA = await coordinator.currentGeneration()
        let preGenB = preGenA  // B 的 401 是基于同一旧 token,所以 snapshot 也是 0
        XCTAssertEqual(preGenA, 0)

        // A 完成（同步模拟"A 比 B 快"——其实就是调用顺序）
        let tokenA = try await coordinator.relogin(callerGeneration: preGenA)
        XCTAssertEqual(tokenA, "token-1")
        XCTAssertEqual(mockUseCase.callCount(of: "execute()"), 1, "A 完成后 useCase 应被调 1 次")

        let postGen = await coordinator.currentGeneration()
        XCTAssertEqual(postGen, 1, "成功完成 → generation ++ 到 1")

        // B 此时才进 relogin（preGenB=0 < 当前 generation=1）→ 应走 generation 短路路径
        let tokenB = try await coordinator.relogin(callerGeneration: preGenB)
        XCTAssertEqual(tokenB, "token-1", "B 应直接拿到 A 已经写入的 lastIssuedToken")

        // 关键断言：useCase 仍仅 1 次 —— B 没触发第二次 useCase.execute
        XCTAssertEqual(
            mockUseCase.callCount(of: "execute()"),
            1,
            "B 的 callerGeneration(0) < 当前 generation(1) → 应短路返 lastIssuedToken,**不**启第二次 useCase"
        )
    }

    // MARK: - case#7 (edge)：callerGeneration == 当前 generation → 走正常重登路径
    // 验证 generation 路径的边界条件：仅在 generation **严格大于** snapshot 时短路；相等不短路.
    // 这避免 cold start 后 (gen=0, lastIssuedToken=nil) 的 caller 误中短路路径.
    func testEqualGenerationDoesNotShortCircuit() async throws {
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("token-1")
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)

        // 初始 gen=0
        let gen = await coordinator.currentGeneration()
        XCTAssertEqual(gen, 0)

        // caller 用 callerGeneration=0,与当前 generation 相等 → 不短路 → 应正常启 useCase
        let token = try await coordinator.relogin(callerGeneration: gen)
        XCTAssertEqual(token, "token-1")
        XCTAssertEqual(
            mockUseCase.callCount(of: "execute()"),
            1,
            "callerGeneration == 当前 generation 时不应短路（避免 cold start lastIssuedToken=nil 误命中）"
        )
    }
}

#endif
