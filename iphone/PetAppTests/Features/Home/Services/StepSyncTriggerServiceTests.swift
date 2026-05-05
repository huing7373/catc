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

    // codex review round 4 [P2] fix: triggerManual 在等 in-flight 期间，automatic trigger 又起一次 sync
    // 时必须 chain-wait（不能只 await 第一个 in-flight 完就跑自己）.
    //
    // 旧实装（round 3 fix）: `await currentSyncTask?.value` 单次 await + 无条件覆盖 currentSyncTask
    //   → 等 launch 完后，main actor 让出期间 timer-like automatic trigger 起新 sync 并填 currentSyncTask
    //   → manual resume 时无视新 task，把 currentSyncTask 直接覆写成自己的 → 旧 automatic task 失去引用
    //   但仍在跑 → manual 自己也启动 → **双 sync 并发**，违反 "同步不重叠" 契约.
    //
    // 新实装（round 4 fix）: while-loop 链式等待 + @MainActor 同步段原子 assign
    //   → manual await launch 完 → re-check 看到 currentSyncTask 又被 timer 填了 → 继续 await 它
    //   → 它完后 currentSyncTask 清 nil → manual 此时同步段创建并 assign 自己 task → 跑.
    //   总 sync 落地数 = 3（launch + automatic 第二次 + manual），证明 chain-wait 生效.
    //
    // 测试序列：
    //   1. triggerForeground() 启动 in-flight A（80ms 延迟）
    //   2. 启动 manual（不 await，先放 detached Task 拿到 manual completion future）
    //   3. 等 ~30ms 让 manual 进入"await currentSyncTask?.value（A）"挂起态
    //   4. 此时 main actor 让出（A 在 sleep；manual 在 await）→ 主测试 task 跑 triggerForeground() 第二次
    //      —— 因为 A 仍 in-flight，A 还在 currentSyncTask 里 → 第二次 triggerForeground 被 gate 短路.
    //      ⚠️ 这无法触发本场景的 race —— race 的关键是"manual await resume 时新 automatic task 已填".
    //
    // 重新设计测试：通过 mock 控制时序 —— 让 mock execute 在第一次 80ms 完成后，
    //   manual 还没 resume 前，模拟 timer-like trigger 再起一次新 in-flight.
    //   实现：在 manual await 期间用 detached Task 间隔再调一次 triggerForeground()，
    //   该 trigger 会观察到 currentSyncTask == nil（A 刚跑完）→ spawn 新 task B，然后 manual resume
    //   看到 currentSyncTask = B（非 nil）→ 旧实装会覆写，新实装应继续 await B 完成.
    //
    // 时序控制要点：
    //   - useCase delay 80ms（A 持续期）
    //   - manual 开始 await 后，等 A 完成（>80ms）+ B 立即起来这一窗口
    //   - 期望最终 invocations.count == 3（A + B + manual）
    func testTriggerManual_chainWaitsThroughMultipleAutomaticInflights() async {
        let useCaseMock = MockSyncStepsUseCase()
        useCaseMock.executeDelayMs = 60  // 每次 sync 60ms 窗口
        let viewModel = HomeViewModel()
        let service = StepSyncTriggerService(syncStepsUseCase: useCaseMock, homeViewModel: viewModel)

        // 步骤 1: 起 in-flight A.
        service.triggerForeground()
        try? await Task.sleep(nanoseconds: 5_000_000)  // 5ms
        XCTAssertEqual(useCaseMock.invocations.count, 1, "前置：A 已启动")

        // 步骤 2: detached task 跑 manual（不 await 它，让本 task 继续控制时序）.
        let manualDone = Task { @MainActor in
            await service.triggerManual()
        }

        // 步骤 3: 等 A 跑完（60ms+ 缓冲）但 manual resume 之前，模拟 automatic trigger 再起一次.
        // A 起在 t=0, 60ms 完. manual 进入 await currentSyncTask?.value 在 ~5ms 后.
        // 等到 A 完成（~80ms 缓冲让 defer 清 nil）.
        try? await Task.sleep(nanoseconds: 80_000_000)  // 80ms

        // A 应已完成；currentSyncTask 应被 A 自己的 defer 清成 nil.
        // 现在 race-window: manual 可能还没 resume 来抢；本 task 立刻 spawn automatic B.
        // 注意：manual resume 是 main actor 调度问题；如果 manual 已 resume 完成自己创建并 assign 了
        // 自己的 task，那么 triggerForeground 此时被 gate 短路（manual task in-flight）→ 总数 = 2
        // （旧实装的 race-good case）. 但若 manual 还在 await suspension queue 里没 resume，本 task
        // 先抢到 → triggerForeground spawn B（observe nil → 起新 task）→ manual resume 后看到 B.
        //
        // 旧实装：manual resume 看到 currentSyncTask = B → 无视它直接覆写 → B 失去引用但仍在跑 →
        //   manual 起自己 task → 总落地 invocations = 3，但 currentSyncTask 在某瞬间 = manual task
        //   而 B 还在跑 → "同步不重叠"违反.
        //
        // 新实装：manual resume 看到 currentSyncTask = B（非 nil）→ while-loop 继续等 B → B 完成
        //   → currentSyncTask = nil → manual 同步段 assign 自己 task → 跑 → 总落地 = 3.
        //
        // 两个实装在 invocations.count 上都可能达到 3，但新实装保证"任意时刻最多 1 个 in-flight".
        // 用 mock 的"max-concurrent-execute"计数器去断言这一点.
        service.triggerForeground()

        // 等 manual 完成（最多再等 200ms：B 60ms + manual 60ms + 缓冲）.
        await manualDone.value

        // 至少跑了 3 次（A + B + manual）；若 manual 抢得快本 trigger 被 gate 短路则跑 2 次（也合规）.
        // 关键断言：永远不超过 1 个 in-flight.
        XCTAssertGreaterThanOrEqual(useCaseMock.invocations.count, 2,
            "至少 A + manual；race-window 命中时 = 3（A + B + manual）")
        XCTAssertLessThanOrEqual(useCaseMock.invocations.count, 3,
            "最多 A + B + manual = 3")
        XCTAssertEqual(useCaseMock.maxConcurrentInflight, 1,
            "不变量：任意时刻最多 1 个 sync in-flight；旧实装 race 路径会让此值 = 2")
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

    /// codex review round 4 [P2] regression test: 追踪并发 in-flight 峰值.
    /// 每次 execute 进入时 +1，离开时 -1；max 值供"任意时刻最多 1 个"不变量断言.
    private(set) var maxConcurrentInflight: Int = 0
    private var currentInflight: Int = 0
    private let inflightLock = NSLock()

    func execute(motionState: MotionState) async throws {
        invocations.append(motionState)
        inflightLock.lock()
        currentInflight += 1
        if currentInflight > maxConcurrentInflight {
            maxConcurrentInflight = currentInflight
        }
        inflightLock.unlock()
        defer {
            inflightLock.lock()
            currentInflight -= 1
            inflightLock.unlock()
        }
        if executeDelayMs > 0 {
            try? await Task.sleep(nanoseconds: executeDelayMs * 1_000_000)
        }
        if let stubError {
            throw stubError
        }
    }
}
