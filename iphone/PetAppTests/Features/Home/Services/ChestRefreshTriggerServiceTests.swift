// ChestRefreshTriggerServiceTests.swift
// Story 21.2 AC7: ChestRefreshTriggerService 单测覆盖（≥ 3 case；本文件给 5 case 覆盖
// start 首次 / re-start 不重启 timer / in-flight gate / 错误不阻塞 / stop 取消 timer）.
//
// 设计精神（story AC7 关键决策）：
//   - 用 fake UseCase（MockLoadChestUseCase）+ 验证调用次数；
//   - 用 XCTestExpectation / 轮询 sleep 等 fire-and-forget Task 排队完成；
//   - **不**等真实 60s timer 流逝（违反单测原则），只断言 timer Task 已启动 + 不重复启动；
//   - 与 StepSyncTriggerServiceTests 同模式（参考其 waitForInvocations helper）.

import XCTest
@testable import PetApp

@MainActor
final class ChestRefreshTriggerServiceTests: XCTestCase {

    // MARK: - case#1 happy: start() 触发首次 launch refresh

    func testStart_triggersInitialRefresh() async {
        let useCaseMock = MockLoadChestUseCase()
        let service = ChestRefreshTriggerService(loadChestUseCase: useCaseMock)

        service.start()
        await waitForInvocations(useCaseMock, atLeast: 1)

        XCTAssertEqual(useCaseMock.invocations.count, 1,
            "start() 应触发一次 chest refresh（reason=.launch）")
    }

    // MARK: - case#2 happy: 已 hasStartedTimer 后再 start() 等同 foreground 触发（+1 次，不重启 timer）

    func testStart_idempotent_secondCallTriggersForegroundRefreshOnly() async {
        let useCaseMock = MockLoadChestUseCase()
        let service = ChestRefreshTriggerService(loadChestUseCase: useCaseMock)

        // 第一次：launch refresh.
        service.start()
        await waitForInvocations(useCaseMock, atLeast: 1)
        XCTAssertEqual(useCaseMock.invocations.count, 1, "首次 start() 应触发一次 refresh")

        // 第二次（模拟 .active scenePhase 再次进入）：foreground refresh，**不**重启 timer.
        service.start()
        await waitForInvocations(useCaseMock, atLeast: 2)
        XCTAssertEqual(useCaseMock.invocations.count, 2,
            "第二次 start() 应触发一次 foreground refresh（共 2 次），证明幂等且不漏触发")
    }

    // MARK: - case#3 edge: in-flight gate —— start() 时 UseCase 未完成，再次 start() 被忽略

    func testStart_whileInFlight_isIgnored() async {
        let useCaseMock = MockLoadChestUseCase()
        useCaseMock.executeDelayMs = 80  // 让第一次 refresh 有 80ms 窗口
        let service = ChestRefreshTriggerService(loadChestUseCase: useCaseMock)

        // 首次 start 进入 in-flight.
        service.start()
        try? await Task.sleep(nanoseconds: 10_000_000)  // 10ms 让 in-flight 起来
        XCTAssertEqual(useCaseMock.invocations.count, 1, "前置：in-flight refresh 已启动")

        // in-flight 期间再调 start() 应被 gate 短路 return（不排队）.
        service.start()
        service.start()

        // 等 80ms+ 让首次 refresh 跑完.
        try? await Task.sleep(nanoseconds: 150_000_000)  // 150ms
        await Task.yield()
        await Task.yield()

        XCTAssertEqual(useCaseMock.invocations.count, 1,
            "fire-and-forget 路径在 in-flight 时新触发应被忽略；总 refresh 次数应为 1")
    }

    // MARK: - case#4 edge: UseCase throw → silently 吞 + 下次 start 可重试

    func testRefreshFails_doesNotBlockNextTrigger() async {
        let useCaseMock = MockLoadChestUseCase()
        useCaseMock.stubError = APIError.network(underlying: NSError(domain: "test", code: -1))
        let service = ChestRefreshTriggerService(loadChestUseCase: useCaseMock)

        // 第一次：失败 silently 吞.
        service.start()
        await waitForInvocations(useCaseMock, atLeast: 1)
        XCTAssertEqual(useCaseMock.invocations.count, 1)

        // 第二次：成功（仍然走 hasStartedTimer 后的 foreground 路径）.
        useCaseMock.stubError = nil
        service.start()
        await waitForInvocations(useCaseMock, atLeast: 2)
        XCTAssertEqual(useCaseMock.invocations.count, 2,
            "第一次失败不应阻塞第二次触发；service.runRefresh silently 吞错保证下次可重试")
    }

    // MARK: - case#5 happy: stop() 取消 timer（验 cancel 路径，不验真实 60s 流逝）

    func testStopCancelsTimer() async {
        let useCaseMock = MockLoadChestUseCase()
        let service = ChestRefreshTriggerService(loadChestUseCase: useCaseMock)

        service.start()
        await waitForInvocations(useCaseMock, atLeast: 1)
        let invocationsAfterStart = useCaseMock.invocations.count

        service.stop()
        // sleep 远小于 60s timer；仅验证 cancel 路径不抛错 / 不再触发.
        try? await Task.sleep(nanoseconds: 50_000_000)  // 50ms

        XCTAssertEqual(useCaseMock.invocations.count, invocationsAfterStart,
            "stop() 后无新增 invocations（cancel 生效）")
    }

    // MARK: - case#6 happy: stop 后再 start 走 firstStart 路径（rebind 路径活）
    //
    // 与 StepSyncTriggerServiceTests.testStart_thenStop_thenStart_rebindPathWorks 同精神：
    // 模拟 launch state 离开 .ready（onLeaveReady → service.stop()）后又重新进入 .ready
    // （onReadyTask → service.start()）的真实 lifecycle.

    func testStart_thenStop_thenStart_rebindPathWorks() async {
        let useCaseMock = MockLoadChestUseCase()
        let service = ChestRefreshTriggerService(loadChestUseCase: useCaseMock)

        // 第一次：launch refresh
        service.start()
        await waitForInvocations(useCaseMock, atLeast: 1)
        XCTAssertEqual(useCaseMock.invocations.count, 1)

        // stop（模拟 launch state 离开 .ready）
        service.stop()

        // 第二次 start（模拟重新进入 .ready）：因 hasStartedTimer 已复位，应走 firstStart 路径
        service.start()
        await waitForInvocations(useCaseMock, atLeast: 2)
        XCTAssertEqual(useCaseMock.invocations.count, 2,
            "stop 后再 start 应被视为新 firstStart 并触发一次 launch refresh")
    }

    // MARK: - Helper

    /// Helper: 轮询等到 mock invocations.count >= atLeast，最多等 1 秒.
    /// 与 StepSyncTriggerServiceTests.waitForInvocations 同模式（fire-and-forget Task 排队等待）.
    fileprivate func waitForInvocations(
        _ mock: MockLoadChestUseCase,
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

final class MockLoadChestUseCase: LoadChestUseCaseProtocol, @unchecked Sendable {
    /// 调用记录（每次 execute() 入栈一条 Date 戳，仅作 count 用）.
    private(set) var invocations: [Date] = []
    private let invocationsLock = NSLock()

    var stubError: Error?
    var executeDelayMs: UInt64 = 0

    func execute() async throws {
        invocationsLock.lock()
        invocations.append(Date())
        invocationsLock.unlock()
        if executeDelayMs > 0 {
            try? await Task.sleep(nanoseconds: executeDelayMs * 1_000_000)
        }
        if let stubError {
            throw stubError
        }
    }
}
