// AppLaunchStateMachineTests.swift
// Story 2.9 AC6：状态机单元测试。覆盖 epics.md 钦定的 4 个 case + 跨 .task 边界短路（hasBootstrapped 防重入）。
//
// import 备注（继承 lesson 2026-04-25-swift-explicit-import-combine.md）：
// `@MainActor` 测试类引用 `AppLaunchStateMachine`（@MainActor + ObservableObject），
// 所有测试方法标 `@MainActor` 让 await 调用 / @Published 写入都在 main actor 内合法。

import XCTest
@testable import PetApp

@MainActor
final class AppLaunchStateMachineTests: XCTestCase {

    /// case#1 (happy)：初值是 .launching（epics.md AC 第 1 条 "App 启动 → .launching"）。
    func testInitialStateIsLaunching() {
        let sm = AppLaunchStateMachine()
        XCTAssertEqual(sm.state, .launching)
    }

    /// case#2 (happy)：两个 step 都成功 → state 最终是 .ready
    /// （epics.md AC 第 3 条 "GuestLoginUseCase + LoadHomeUseCase 都成功 → .ready"）。
    func testBootstrapWithBothStepsSuccessReachesReady() async {
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { /* immediate success */ },
            bootstrapStep2: { /* immediate success */ }
        )
        await sm.bootstrap()
        XCTAssertEqual(sm.state, .ready)
    }

    /// case#3 (edge)：step1 抛错 → state 是 .needsAuth(message:)
    /// （epics.md AC 第 4 条 "任一失败 → .needsAuth"，含 step1 抛错）。
    func testBootstrapWithStep1FailureReachesNeedsAuth() async {
        struct TestError: Error, LocalizedError {
            var errorDescription: String? { "step1 失败" }
        }
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { throw TestError() },
            bootstrapStep2: { /* never called */ }
        )
        await sm.bootstrap()
        XCTAssertEqual(sm.state, .needsAuth(message: "step1 失败"))
    }

    /// case#4 (edge)：step2 抛错 → state 是 .needsAuth(message:)
    /// （epics.md AC 第 4 条 "LoadHomeUseCase 失败 → .needsAuth → RetryView"）。
    func testBootstrapWithStep2FailureReachesNeedsAuth() async {
        struct TestError: Error, LocalizedError {
            var errorDescription: String? { "step2 失败" }
        }
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { /* success */ },
            bootstrapStep2: { throw TestError() }
        )
        await sm.bootstrap()
        XCTAssertEqual(sm.state, .needsAuth(message: "step2 失败"))
    }

    /// case#5 (edge)：minimumDuration（0.3 秒）保护
    /// 用极快 step（立即成功）调 bootstrap，断言至少 elapsed ≥ minimumDuration。
    /// 防 LaunchingView 在快网络下闪一下就消失（epics.md AC 钦定）。
    func testBootstrapEnforcesMinimumDuration() async {
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { /* immediate */ },
            bootstrapStep2: { /* immediate */ }
        )
        let start = Date()
        await sm.bootstrap()
        let elapsed = Date().timeIntervalSince(start)
        XCTAssertGreaterThanOrEqual(
            elapsed,
            AppLaunchStateMachine.minimumDuration,
            "极快 bootstrap 也应至少经过 minimumDuration（\(AppLaunchStateMachine.minimumDuration)s）才进入 .ready"
        )
        XCTAssertEqual(sm.state, .ready)
    }

    /// case#6 (edge)：hasBootstrapped 防重入（跨 .task 边界）
    /// 调两次 bootstrap()，第二次应 short-circuit 不再跑 step；用 step1 计数器验证。
    func testBootstrapShortCircuitsAfterFirstCompletion() async {
        let counter = CallCounter()
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { await counter.increment() },
            bootstrapStep2: { /* success */ }
        )
        await sm.bootstrap()
        await sm.bootstrap()  // 第二次：应被 hasBootstrapped 短路
        let count = await counter.value
        XCTAssertEqual(count, 1, "bootstrap() 第二次调用应短路；step1 应只跑 1 次")
    }

    /// case#8 (edge)：非 LocalizedError 抛出 → message 用默认 fallback "登录失败，请重试"
    /// 防 `error.localizedDescription` 对 plain Error 返回的系统串
    /// （`"The operation couldn't be completed (PetApp.SomeError error 1.)"`）漏到 RetryView
    /// （codex round 1 [P2] finding 修复）。
    func testBootstrapWithPlainErrorUsesDefaultFallback() async {
        struct PlainError: Error {}  // 故意**不**实现 LocalizedError
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { throw PlainError() },
            bootstrapStep2: { /* never called */ }
        )
        await sm.bootstrap()
        XCTAssertEqual(
            sm.state,
            .needsAuth(message: AppLaunchStateMachine.defaultFailureMessage),
            "非 LocalizedError 应回落到默认文案，不应展示 NSError 系统串"
        )
    }

    /// case#9 (edge)：连点两次 retry → 第二次直接被 isRetrying guard 短路 →
    /// step closure 不会重复跑 + 最终 state 不被 race
    /// （codex round 1 [P2] finding 修复）。
    func testRetryConcurrentInvocationsRunOnce() async {
        let counter = CallCounter()
        let sm = AppLaunchStateMachine(
            bootstrapStep1: {
                await counter.increment()
                // 故意 sleep 让第一个 retry 在飞中，留窗口给第二个 retry 进 guard
                try? await Task.sleep(nanoseconds: 50_000_000)  // 50ms
            },
            bootstrapStep2: { }
        )
        // 先 bootstrap 一次到 .ready 让 hasBootstrapped = true，retry() 才会清 hasBootstrapped 触发 step
        await sm.bootstrap()
        let initialCount = await counter.value
        XCTAssertEqual(initialCount, 1, "首次 bootstrap 应跑 step1 一次")

        // 关键断言：两个 retry concurrent 跑，第二个应被 isRetrying guard 短路
        async let retry1: Void = sm.retry()
        async let retry2: Void = sm.retry()
        _ = await (retry1, retry2)

        let totalCount = await counter.value
        XCTAssertEqual(
            totalCount,
            2,
            "concurrent 两次 retry 应只跑一次新 bootstrap（共 step1 计数 = 1 初始 + 1 retry = 2，**不是** 3）"
        )
        XCTAssertEqual(sm.state, .ready)
    }

    /// case#7 (happy)：retry() 重置 state = .launching → 重跑 step → 成功后 .ready
    /// 用 retry 验证"用户在 .needsAuth 状态点重试按钮"路径。
    func testRetryResetsStateAndReruns() async {
        let counter = CallCounter()
        let shouldFailHolder = ShouldFailHolder()
        let sm = AppLaunchStateMachine(
            bootstrapStep1: {
                await counter.increment()
                if await shouldFailHolder.value {
                    struct E: Error {}
                    throw E()
                }
            },
            bootstrapStep2: { }
        )
        await sm.bootstrap()
        if case .needsAuth = sm.state {
            // expected
        } else {
            XCTFail("first bootstrap should fail and reach .needsAuth")
        }

        await shouldFailHolder.setValue(false)
        await sm.retry()
        XCTAssertEqual(sm.state, .ready)
        let count = await counter.value
        XCTAssertEqual(count, 2, "retry() 应重跑 step1（共 2 次：原失败 1 次 + retry 成功 1 次）")
    }
}

/// 简单 actor 计数器（避免 Sendable 警告 + 测试线程隔离）。
actor CallCounter {
    private(set) var value: Int = 0
    func increment() { value += 1 }
}

/// 简单 actor 持有 mutable Bool（避免 Swift 6 strict concurrency 捕获 var 的警告）。
actor ShouldFailHolder {
    private(set) var value: Bool = true
    func setValue(_ newValue: Bool) { value = newValue }
}
