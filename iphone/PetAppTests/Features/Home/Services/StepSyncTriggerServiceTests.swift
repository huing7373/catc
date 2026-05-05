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

    // edge: in-flight 时新触发被忽略（epics.md AC 行 1577 + 1583）.
    func testTriggerWhileInFlight_isIgnored() async {
        let useCaseMock = MockSyncStepsUseCase()
        useCaseMock.executeDelayMs = 80  // 让第一次 sync 有 80ms 窗口
        let viewModel = HomeViewModel()
        let service = StepSyncTriggerService(syncStepsUseCase: useCaseMock, homeViewModel: viewModel)

        async let first: Void = service.triggerManual()
        // 在第一次 in-flight 时立即触发第二次（fire-and-forget）.
        // 给第一次 manual 足够时间进入 isSyncing 状态.
        try? await Task.sleep(nanoseconds: 10_000_000)  // 10ms
        service.triggerForeground()
        service.triggerForeground()
        await first
        await Task.yield()
        await Task.yield()

        XCTAssertEqual(useCaseMock.invocations.count, 1,
            "in-flight 时新触发应被忽略；总同步次数应为 1")
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
