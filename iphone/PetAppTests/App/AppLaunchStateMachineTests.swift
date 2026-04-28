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

    /// case#3 (edge)：step1 抛 LocalizedError → state 是 .needsAuth(presentation: .retry(message:))
    /// LocalizedError fallback 路径：状态机用 errorDescription 包成 .retry —— 给用户重试入口的宽容兜底.
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
        XCTAssertEqual(sm.state, .needsAuth(presentation: .retry(message: "step1 失败")))
    }

    /// case#4 (edge)：step2 抛 LocalizedError → state 是 .needsAuth(presentation: .retry(message:))
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
        XCTAssertEqual(sm.state, .needsAuth(presentation: .retry(message: "step2 失败")))
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

    /// case#8 (edge)：非 LocalizedError 抛出 → presentation 用默认 fallback `.retry(defaultFailureMessage)`
    /// 防 `error.localizedDescription` 对 plain Error 返回的系统串
    /// （`"The operation couldn't be completed (PetApp.SomeError error 1.)"`）漏到 UI
    /// （codex round 1 [P2] finding 修复 + round 2 [P1] 升级为 presentation）。
    func testBootstrapWithPlainErrorUsesDefaultFallback() async {
        struct PlainError: Error {}  // 故意**不**实现 LocalizedError
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { throw PlainError() },
            bootstrapStep2: { /* never called */ }
        )
        await sm.bootstrap()
        XCTAssertEqual(
            sm.state,
            .needsAuth(presentation: AppLaunchStateMachine.defaultFailurePresentation),
            "非 LocalizedError 应回落到默认 presentation（.retry），不应展示 NSError 系统串"
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

    /// case#10 (Story 5.5 round 1 [P2] + round 2 [P1] fix)：bootstrap step closure 抛 BootstrapMappedError
    /// → state.presentation 直接等于 mapper 派出的 ErrorPresentation（**不是** message 字符串）.
    ///
    /// 防回退（regression guard）: round 1 fix 前 closure 把 APIError 直接抛给 messageFor → developer 串.
    /// round 2 fix 前 closure 用 userFacingMessage 包成 BootstrapMappedError → 状态机塞进 .needsAuth(message:)
    /// → RootView 永远渲染 RetryView, 把 mapper 钦定为 .alert 的错误（unauthorized / decoding 等）误降级.
    /// round 2 fix 后状态机直接收 ErrorPresentation, RootView 三态分发 → alert/retry 各自渲染.
    /// 本 case 验证 .network → mapper 派 .retry → 状态机透传.
    func testBootstrapWithMappedNetworkErrorRoutesToRetryPresentation() async {
        let underlying = APIError.network(underlying: URLError(.timedOut))
        let wrapped = BootstrapMappedError(
            presentation: AppErrorMapper.presentation(for: underlying),
            underlying: underlying
        )
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { throw wrapped },
            bootstrapStep2: { /* never called */ }
        )
        await sm.bootstrap()
        XCTAssertEqual(
            sm.state,
            .needsAuth(presentation: .retry(message: "网络异常，请检查后重试")),
            "network 错误必须走 .retry 让用户重试; mapper 派出的 presentation 必须直达状态机"
        )
    }

    /// case#11 (Story 5.5 round 5 [P1] fix): transient business code 1009 (服务繁忙) 走 mapper → .retry.
    /// **regression guard**: round 4 fix 错误把所有 business code 一律映射成 .alert,导致
    /// bootstrap 路径下 1009 进 AlertOverlayView ("知道了" 按钮 no-op) 死锁.
    /// round 5 fix: mapper 把 transient 业务码（1005/1007/1008/1009）改派 .retry,
    /// 让 bootstrap 失败 → RetryView → 用户重试 → 自愈.
    func testBootstrapWithMappedBusinessErrorRoutesToRetryPresentation() async {
        let underlying = APIError.business(code: 1009, message: "server 原文", requestId: "req_x")
        let wrapped = BootstrapMappedError(
            presentation: AppErrorMapper.presentation(for: underlying),
            underlying: underlying
        )
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { throw wrapped },
            bootstrapStep2: { /* never called */ }
        )
        await sm.bootstrap()
        XCTAssertEqual(
            sm.state,
            .needsAuth(presentation: .retry(message: "服务繁忙，请稍后重试")),
            "transient business 1009 必须走 .retry; round 5 [P1] regression guard"
        )
    }

    /// case#11b (Story 5.5 round 5 [P1] fix → round 8 [P1] 文案回归简洁): permanent business code 仍走 .alert.
    /// 用 5002 (道具不属于你) 作 permanent 类代表 —— 用户重试也不会改变结果.
    /// round 8 [P1] fix: alert 文案回归 round 5 风格,不再带 "持续失败时请杀进程重启 App" suffix
    /// —— 该指引已 move 到 TerminalErrorView 底部静态文本 (RootView 把 bootstrap .alert 渲染为
    /// TerminalErrorView 全屏静态 page, mapper 文案专注表达"什么错了").
    func testBootstrapWithMappedPermanentBusinessErrorRoutesToAlertPresentation() async {
        let underlying = APIError.business(code: 5002, message: "server 原文", requestId: "req_x")
        let wrapped = BootstrapMappedError(
            presentation: AppErrorMapper.presentation(for: underlying),
            underlying: underlying
        )
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { throw wrapped },
            bootstrapStep2: { /* never called */ }
        )
        await sm.bootstrap()
        XCTAssertEqual(
            sm.state,
            .needsAuth(presentation: .alert(title: "提示", message: "道具不属于你")),
            "permanent business 仍走 .alert (terminal); round 8 文案回归简洁,force-quit 指引在 TerminalErrorView 静态文本中"
        )
    }

    /// case#12 (Story 5.5 round 2 [P1] fix → round 8 [P1] 文案回归简洁): `.unauthorized` 必须走 `.alert` 而非 `.retry`.
    ///
    /// **核心 regression guard**: round 2 [P1] finding 的精确复现.
    /// round 8 [P1] fix: 文案回归 round 5 风格 ("登录失败，请重新启动应用"), 不带 "请重试" 前缀
    /// —— RootView .alert 分支渲染 TerminalErrorView (无按钮 → 无重试入口), 文案不该 promise UI 不提供的动作.
    func testBootstrapWithUnauthorizedRoutesToAlertPresentation() async {
        let underlying = APIError.unauthorized
        let wrapped = BootstrapMappedError(
            presentation: AppErrorMapper.presentation(for: underlying),
            underlying: underlying
        )
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { throw wrapped },
            bootstrapStep2: { /* never called */ }
        )
        await sm.bootstrap()
        XCTAssertEqual(
            sm.state,
            .needsAuth(presentation: .alert(title: "提示", message: "登录失败，请重新启动应用")),
            ".unauthorized 必须走 .alert; round 8 文案不 promise 重试 (TerminalErrorView 无按钮)"
        )
    }

    /// case#13 (Story 5.5 round 2 [P1] fix → round 8 [P1] 文案回归简洁): `.decoding` 也走 `.alert`（mapper 钦定）.
    func testBootstrapWithDecodingErrorRoutesToAlertPresentation() async {
        struct StubDecodingError: Error {}
        let underlying = APIError.decoding(underlying: StubDecodingError())
        let wrapped = BootstrapMappedError(
            presentation: AppErrorMapper.presentation(for: underlying),
            underlying: underlying
        )
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { throw wrapped },
            bootstrapStep2: { /* never called */ }
        )
        await sm.bootstrap()
        XCTAssertEqual(
            sm.state,
            .needsAuth(presentation: .alert(title: "提示", message: "数据异常，请稍后重试")),
            ".decoding 必须走 .alert; round 8 文案回归简洁 (force-quit 指引在 TerminalErrorView 静态文本)"
        )
    }

    /// case#14 (Story 5.5 round 2 [P1] fix): `.missingCredentials` 走 `.alert("登录信息丢失，请重启 App")`.
    /// round 7 [P1] fix: 这条文案直接钦定 "请重启 App"，不加 "请重试" 前缀
    /// (retry 救不回, keychain 真没 token, 重试只是反复弹同一 alert).
    func testBootstrapWithMissingCredentialsRoutesToAlertPresentation() async {
        let underlying = APIError.missingCredentials
        let wrapped = BootstrapMappedError(
            presentation: AppErrorMapper.presentation(for: underlying),
            underlying: underlying
        )
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { throw wrapped },
            bootstrapStep2: { /* never called */ }
        )
        await sm.bootstrap()
        XCTAssertEqual(
            sm.state,
            .needsAuth(presentation: .alert(title: "提示", message: "登录信息丢失，请重启 App")),
            ".missingCredentials 必须走 .alert; 文案直接钦定 force-quit (retry 救不回)"
        )
    }

    /// case#15 (Story 5.5 round 8 [P1] fix - 终极方案): bootstrap 路径 `.alert` presentation
    /// 不再有 dismiss closure — RootView 渲染 TerminalErrorView (静态全屏 page, 无任何按钮).
    ///
    /// **dismiss 行为迭代史 (本 case 守的就是 round 8 终极契约, 防 regress 回任何前轮模式)**:
    /// - round 0 (dev-story): 默认 .needsAuth → 自动 retry → P2 finding (隐式 retry, 不可控).
    /// - round 3: 区分 alert/retry, 但 alert dismiss 仍 retry → P1 死循环.
    /// - round 4: alert dismiss 改 no-op → P2 卡死.
    /// - round 5: alert dismiss 改 exit(0) → P1: iOS HIG 反模式.
    /// - round 7: alert dismiss → retry() + mapper 文案 "持续失败时请杀进程重启 App"
    ///   → P1 (round 8 review): user 仍可被困 retry → fail → retry 循环.
    /// - **round 8 (current — 终极方案)**: bootstrap 路径 `.alert` 不再用 dismiss-able overlay.
    ///   改用 TerminalErrorView (静态全屏 page, **无任何按钮**, user 必须 force-quit).
    ///   **不再有 dismiss closure 可纠结**.
    ///
    /// **为什么 round 8 是终极方案**:
    /// 5 轮 fix-review 揭示: bootstrap terminal 错误的"dismiss 行为"本身是伪命题. AlertOverlayView
    /// 是 dismiss-able overlay → 必须有按钮 → 必须有 closure → closure 选什么动作都跟 terminal 语义冲突.
    /// 唯一可调和: 不给 dismiss 入口. iOS error boundary 模式 = full-screen static page = user force-quit.
    ///
    /// **本 case 验证**: 状态机的 `.alert` presentation 路径仍正确 (mapper 派 .alert → state 落到
    /// .needsAuth(.alert)). dismiss closure 不再由 RootView 提供 → 本 case 不再 simulate "user 点 OK"
    /// 的行为 (因为 TerminalErrorView 没 OK 按钮, user 唯一行为是杀进程, 不在 in-app test scope).
    /// 状态机本身的 retry() 仍可被外部调用 (e.g. 未来 SceneDelegate willEnterForeground 自动 retry),
    /// 本 case 保留 retry 后回到 .alert 的 invariant 测试 —— 但语义改为"如果未来某条路径 (非 alert OK
    /// 按钮) 调 retry(), 状态机仍能正确处理".
    func testStateMachinePresentsAlertForTerminalErrorAndRetryStaysIdempotent() async {
        let underlying = APIError.unauthorized
        let wrapped = BootstrapMappedError(
            presentation: AppErrorMapper.presentation(for: underlying),
            underlying: underlying
        )
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { throw wrapped },
            bootstrapStep2: { /* never called */ }
        )
        await sm.bootstrap()
        // 首次 bootstrap 失败 → 落到 .alert.
        // **round 8 [P1] regression guard**: state 必须是 .alert (mapper 钦定), 否则
        // RootView 不会去渲染 TerminalErrorView, 又会回到前轮 (RetryView / AlertOverlay) 反模式.
        XCTAssertEqual(
            sm.state,
            .needsAuth(presentation: .alert(title: "提示", message: "登录失败，请重新启动应用")),
            "round 8 contract: terminal-class error 必须是 .alert presentation, RootView 才会渲染 TerminalErrorView (force-quit-only)"
        )

        // retry() 路径仍 invariant: 重跑闭包 → 仍抛同样错 → 同 .alert 再次 set.
        // 注意: round 8 后 RootView 的 .alert 分支是 TerminalErrorView (无 dismiss button), 所以
        // user 不会主动触发 retry. 但状态机 retry() API 仍 exposed —— 本 case 守 API 自身的 idempotency,
        // 防未来某条路径 (e.g. SceneDelegate willEnterForeground 自动 retry / dev menu 触发) 调 retry()
        // 时状态机崩溃.
        await sm.retry()
        XCTAssertEqual(
            sm.state,
            .needsAuth(presentation: .alert(title: "提示", message: "登录失败，请重新启动应用")),
            "retry() API 自身 idempotent: 重跑同失败 closure → 同 .alert presentation 再次 set"
        )
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
