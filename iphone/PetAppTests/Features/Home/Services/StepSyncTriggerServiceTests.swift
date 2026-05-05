// StepSyncTriggerServiceTests.swift
// Story 8.5 AC10.2: StepSyncTriggerService 单元测试.
// 注：本测试基于 AC9 落地的 option A（service 持 homeViewModel 而非 motionProvider）.

import XCTest
@testable import PetApp

@MainActor
final class StepSyncTriggerServiceTests: XCTestCase {

    // happy: start() 触发首次同步 + 使用 viewModel.petState 拼请求（epics.md AC 行 1579）.
    func testStart_triggersInitialSync_usingViewModelPetState() async {
        let useCaseMock = MockSyncStepsUseCase()
        let viewModel = HomeViewModel()
        viewModel.petState = .walk
        let service = StepSyncTriggerService(
            syncStepsUseCase: useCaseMock,
            homeViewModel: viewModel
        )

        service.start()
        // 等 Task spawn 完成（Task 排队 → schedule → 执行）.
        await waitForInvocations(useCaseMock, atLeast: 1)

        XCTAssertEqual(useCaseMock.invocations.count, 1, "start() 应触发一次同步")
        XCTAssertEqual(useCaseMock.invocations.first, .walk, "应使用 viewModel.petState")
    }

    // happy: triggerForeground() 触发同步（epics.md AC 行 1580）.
    func testTriggerForeground_triggersSync() async {
        let useCaseMock = MockSyncStepsUseCase()
        let viewModel = HomeViewModel()
        let service = StepSyncTriggerService(syncStepsUseCase: useCaseMock, homeViewModel: viewModel)

        service.triggerForeground()
        await waitForInvocations(useCaseMock, atLeast: 1)

        XCTAssertEqual(useCaseMock.invocations.count, 1)
    }

    // happy: triggerManual() 触发同步并等待完成（epics.md AC 行 1582）.
    func testTriggerManual_awaitsSyncCompletion() async {
        let useCaseMock = MockSyncStepsUseCase()
        let viewModel = HomeViewModel()
        let service = StepSyncTriggerService(syncStepsUseCase: useCaseMock, homeViewModel: viewModel)

        await service.triggerManual()

        XCTAssertEqual(useCaseMock.invocations.count, 1, "triggerManual 应等待同步完成")
    }

    // edge: fire-and-forget 路径在 in-flight 时被忽略（epics.md AC 行 1577 + 1583）.
    // review round 3 [P2] fix 后：in-flight gate 仍对 fire-and-forget 路径生效
    // （launch / foreground / timer），但 triggerManual 改为"等 in-flight 完再自己跑一次".
    // 本测试只验证 fire-and-forget 路径的重叠忽略（用 triggerForeground 跑两次，期望仅一次落地）.
    func testFireAndForgetTrigger_whileInFlight_isIgnored() async {
        let useCaseMock = MockSyncStepsUseCase()
        useCaseMock.executeDelayMs = 80  // 让第一次 sync 有 80ms 窗口
        let viewModel = HomeViewModel()
        let service = StepSyncTriggerService(syncStepsUseCase: useCaseMock, homeViewModel: viewModel)

        // 首次 triggerForeground 启动 fire-and-forget sync（80ms 延迟）.
        service.triggerForeground()
        // 给第一次进入 in-flight 状态.
        try? await Task.sleep(nanoseconds: 10_000_000)  // 10ms
        // in-flight 期间再调 triggerForeground 应被 gate 忽略.
        service.triggerForeground()
        service.triggerForeground()
        // 等 80ms+ 让首次 sync 跑完.
        try? await Task.sleep(nanoseconds: 150_000_000)  // 150ms
        await Task.yield()
        await Task.yield()

        XCTAssertEqual(useCaseMock.invocations.count, 1,
            "fire-and-forget 路径在 in-flight 时新触发应被忽略；总同步次数应为 1")
    }

    // review round 3 [P2] fix: triggerManual 在 in-flight 时不能被短路 return.
    // 必须等 in-flight 完，再自己跑一次 fresh sync —— caller (Story 21.x ChestOpen)
    // 依赖 await 返回时拿到刚跑完的 fresh `currentStepAccount`.
    //
    // 序列：
    //   1. triggerForeground() 启动 fire-and-forget sync（80ms 延迟，模拟"正在跑"）
    //   2. 立刻 await triggerManual() —— 旧实装直接被 gate 短路 return；新实装应：
    //      a. 先 await in-flight 完成（拿到首次 sync 落地）
    //      b. 再自己启动一次新 sync 并 await 完成（拿到第二次 sync 落地）
    //   3. 总 invocations.count == 2，证明 manual 拿到的是 fresh state.
    func testTriggerManual_whileInFlight_awaitsThenRunsOwnSync() async {
        let useCaseMock = MockSyncStepsUseCase()
        useCaseMock.executeDelayMs = 80  // 第一次 sync 80ms 窗口
        let viewModel = HomeViewModel()
        let service = StepSyncTriggerService(syncStepsUseCase: useCaseMock, homeViewModel: viewModel)

        // 启动 in-flight sync.
        service.triggerForeground()
        // 给 in-flight 进入状态.
        try? await Task.sleep(nanoseconds: 10_000_000)  // 10ms
        XCTAssertEqual(useCaseMock.invocations.count, 1, "前置：in-flight sync 已启动")

        // 关键：triggerManual 应等 in-flight 完，再自己跑一次.
        await service.triggerManual()

        // manual await 返回时：应该首次（in-flight）已完成 + 自己跑的也完成.
        XCTAssertEqual(useCaseMock.invocations.count, 2,
            "triggerManual 应等 in-flight 完成后再自己跑一次 fresh sync；总 sync 应为 2")
    }

    // edge: useCase 抛错 → 不阻塞 UI 不传染下次触发（epics.md AC 行 1584）.
    func testSyncFails_doesNotBlockNextTrigger() async {
        let useCaseMock = MockSyncStepsUseCase()
        useCaseMock.stubError = APIError.network(underlying: NSError(domain: "test", code: -1))
        let viewModel = HomeViewModel()
        let service = StepSyncTriggerService(syncStepsUseCase: useCaseMock, homeViewModel: viewModel)

        await service.triggerManual()  // 第一次失败（不抛出，silent 吞掉）
        XCTAssertEqual(useCaseMock.invocations.count, 1)

        useCaseMock.stubError = nil  // 第二次成功
        await service.triggerManual()
        XCTAssertEqual(useCaseMock.invocations.count, 2,
            "第一次失败不应阻塞第二次触发")
    }

    // happy: stop() 后 timer 不再触发（验 cancel 路径）.
    func testStopCancelsTimer() async {
        let useCaseMock = MockSyncStepsUseCase()
        let viewModel = HomeViewModel()
        let service = StepSyncTriggerService(syncStepsUseCase: useCaseMock, homeViewModel: viewModel)

        service.start()
        await waitForInvocations(useCaseMock, atLeast: 1)
        let invocationsAfterStart = useCaseMock.invocations.count

        service.stop()
        // sleep 略多于 timer 触发时机的局部窗口（不能等 5min；仅验证 cancel 路径不抛错）.
        try? await Task.sleep(nanoseconds: 50_000_000)  // 50ms

        XCTAssertEqual(useCaseMock.invocations.count, invocationsAfterStart,
            "stop() 后无新增 invocations（cancel 生效）")
    }

    // codex review round 2 [P2] fix: start → stop → start 序列（rebind 路径）；
    // 模拟 launch state 离开 .ready（onLeaveReady → service.stop()）后又重新进入 .ready
    // （onReadyTask → service.start()）的真实 lifecycle.
    //
    // 验证：
    //   1. 第一次 start() → timer 启动 + launch sync（hasStartedTimer = true）
    //   2. stop() → timer cancel + hasStartedTimer 复位（让下次 start() 走 firstStart 路径）
    //   3. 第二次 start() → 视为新 firstStart，再启 timer + 再发一次 launch sync
    //   4. 总 sync 次数 = 2（第一个 launch + 第二个 rebind launch），证明 stop 后 rebind 路径活了
    func testStart_thenStop_thenStart_rebindPathWorks() async {
        let useCaseMock = MockSyncStepsUseCase()
        let viewModel = HomeViewModel()
        let service = StepSyncTriggerService(syncStepsUseCase: useCaseMock, homeViewModel: viewModel)

        // 第一次：启动 + 一次 launch sync
        service.start()
        await waitForInvocations(useCaseMock, atLeast: 1)
        XCTAssertEqual(useCaseMock.invocations.count, 1)

        // stop（模拟 launch state 离开 .ready）
        service.stop()

        // 第二次 start（模拟重新进入 .ready）：因 hasStartedTimer 已复位，应走 firstStart 路径
        service.start()
        await waitForInvocations(useCaseMock, atLeast: 2)
        XCTAssertEqual(useCaseMock.invocations.count, 2,
            "stop 后再 start 应被视为新 firstStart 并触发一次 launch sync")
    }

    // codex review round 2 [P2] fix: stop() 调用幂等（多次 stop 安全）.
    // RootView 同时在 onLeaveReady 与 .scenePhase .background 路径调 stop()；
    // 状态切换序列可能触发两次 stop，必须不抛错 / 不破坏后续 start 行为.
    func testStop_idempotent_safeToCallTwice() async {
        let useCaseMock = MockSyncStepsUseCase()
        let viewModel = HomeViewModel()
        let service = StepSyncTriggerService(syncStepsUseCase: useCaseMock, homeViewModel: viewModel)

        service.start()
        await waitForInvocations(useCaseMock, atLeast: 1)

        service.stop()
        service.stop()  // 第二次 stop 应不抛错（cancel nil timer 安全 + hasStartedTimer 已 false）

        // 后续仍可正常 start
        service.start()
        await waitForInvocations(useCaseMock, atLeast: 2)
        XCTAssertEqual(useCaseMock.invocations.count, 2,
            "两次 stop 不应破坏后续 start —— 总 sync = 2")
    }

    // codex review round 1 [P3] fix: start() 幂等 —— 多次调用不重启 timer / 不并发 spawn 多个
    // 立即 sync；每次 .active scenePhase RootView 只需调 start() 一次（不再额外调 triggerForeground()）.
    //
    // 验证序列：
    //   1. 第一次 start() → timer 启动 + 一次 sync（reason=.launch）
    //   2. 等第一次 sync 完成
    //   3. 第二次 start() → **不**重启 timer + 一次 sync（reason=.foreground 等同 triggerForeground）
    //   4. 总 sync 次数 = 2，**不**因为 RootView 没再调 triggerForeground 而漏触发回前台 sync
    func testStart_idempotent_noDuplicateOnSecondCall() async {
        let useCaseMock = MockSyncStepsUseCase()
        let viewModel = HomeViewModel()
        let service = StepSyncTriggerService(syncStepsUseCase: useCaseMock, homeViewModel: viewModel)

        // 第一次：启动 + 一次 launch sync
        service.start()
        await waitForInvocations(useCaseMock, atLeast: 1)
        XCTAssertEqual(useCaseMock.invocations.count, 1, "首次 start() 应触发一次 sync")

        // 第二次（模拟 .active scenePhase 再次进入）：应等同立即 sync，不重启 timer
        service.start()
        await waitForInvocations(useCaseMock, atLeast: 2)
        XCTAssertEqual(useCaseMock.invocations.count, 2,
            "第二次 start() 应触发一次回前台 sync（共 2 次），证明幂等且不漏触发")
    }

    // happy: petState=.rest 默认（viewModel 未设 petState）→ wire motionState=1（AC4.2 兜底）.
    func testStart_defaultPetState_isRest() async {
        let useCaseMock = MockSyncStepsUseCase()
        let viewModel = HomeViewModel()
        // viewModel.petState 默认 .rest（HomeViewModel 行 115）；不显式设置.
        let service = StepSyncTriggerService(syncStepsUseCase: useCaseMock, homeViewModel: viewModel)

        await service.triggerManual()

        XCTAssertEqual(useCaseMock.invocations.count, 1)
        XCTAssertEqual(useCaseMock.invocations.first, .rest,
            "viewModel 未启动 motion 订阅时 petState 默认 .rest，service 应传 .rest")
    }

    /// Helper: 轮询等到 mock invocations.count >= atLeast，最多等 1 秒.
    /// 比单纯 `Task.yield()` 可靠：service.start() / triggerForeground() 用
    /// `Task { @MainActor [weak self] in await self?.performSync(...) }` spawn 后
    /// `Task.yield()` 在 main actor 上不一定 schedule 该 child Task；
    /// 用循环 sleep + check 才能稳定等到 fire-and-forget Task 跑完.
    fileprivate func waitForInvocations(
        _ mock: MockSyncStepsUseCase,
        atLeast: Int,
        timeoutNanos: UInt64 = 1_000_000_000
    ) async {
        let deadline = DispatchTime.now().uptimeNanoseconds + timeoutNanos
        while DispatchTime.now().uptimeNanoseconds < deadline {
            if mock.invocations.count >= atLeast {
                return
            }
            try? await Task.sleep(nanoseconds: 5_000_000)  // 5ms
        }
    }
}

// MARK: - Test Doubles

final class MockSyncStepsUseCase: SyncStepsUseCaseProtocol, @unchecked Sendable {
    var invocations: [MotionState] = []
    var stubError: Error?
    var executeDelayMs: UInt64 = 0

    func execute(motionState: MotionState) async throws {
        invocations.append(motionState)
        if executeDelayMs > 0 {
            try? await Task.sleep(nanoseconds: executeDelayMs * 1_000_000)
        }
        if let stubError {
            throw stubError
        }
    }
}
