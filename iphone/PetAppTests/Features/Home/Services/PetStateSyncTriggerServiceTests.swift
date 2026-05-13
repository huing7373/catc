// PetStateSyncTriggerServiceTests.swift
// Story 15.4 AC5: PetStateSyncTriggerService 单元测试.
//
// 范围（5 case，严守 spec；attempt 1 测试爆到 2508 行是过度测试反模式，本 file 目标 ≤ 350 行）:
//   case#E happy: in-room + petState mutate → executeCalls 增 1
//   case#F edge: not-in-room → petState mutate → executeCalls.isEmpty
//   case#G edge: 5s 内重复 set 同 state → executeCalls.count == 1（用 nowProvider test seam，**禁** Task.sleep 真等 5s）
//   case#H edge: 5s 后重新 set 同 state → executeCalls.count == 2（同上 test seam）
//   case#I edge: API 失败 → 不 crash + 下次 state 变化照常 spawn（executeCalls.count: 1 → 2）
//
// 测试基础设施约束（与 8.5 / 12.x / 15.1-15.3 一致）:
//   - XCTest only（ADR-0002 §3.1 钦定 / 零外部依赖）
//   - @MainActor 标注测试 class（service 是 @MainActor）
//   - 不引入 ViewInspector / SnapshotTesting / Combine pipeline 测试库
//   - time-related tests 用 service.nowProvider test seam（internal access）注入 fake date
//   - 测试连续 mutate 后的 publish 完成用 await Task.yield() × N 等待（与 Story 15.2 case#D / 15.3 case#E 同模式）
//
// 不测试的 case（spec 范围外，attempt 1 引入但本 story attempt 2 不复刻）:
//   - per-state 字典 cross-state 干扰（service 用单二元组锚点 → 不存在该 case）
//   - publisher subscribe-replay vs 真实 transition 区分（dropFirst 一刀切 → 不需测）
//   - room edge nil → non-nil / non-nil → nil 主动行为（service 不订阅 currentRoomId → 不存在）
//   - stop / start cycle 期间 in-flight Task 行为（service 不 cancel in-flight Task → 不需测）
//   - coalesce-to-latest pending state（service 不维护 pending → 不需测）

import XCTest
@testable import PetApp

@MainActor
final class PetStateSyncTriggerServiceTests: XCTestCase {

    // MARK: - case#E happy: in-room + petState mutate .walk → executeCalls 增 1

    func test_E_happy_inRoom_petStateMutate_triggersUseCaseOnce() async {
        let useCase = MockSyncPetStateUseCase()
        let viewModel = HomeViewModel()
        let appState = AppState()
        appState.setCurrentRoomId("room-X")

        let service = PetStateSyncTriggerService(
            syncPetStateUseCase: useCase,
            homeViewModel: viewModel,
            appState: appState
        )
        service.start()
        // 等订阅瞬间的 currentValue replay（.rest）被 dropFirst 抹掉.
        await yieldRepeatedly()

        viewModel.petState = .walk
        // 等 sink 被调 + Task spawn + UseCase.execute 完成.
        await yieldRepeatedly()

        XCTAssertEqual(useCase.executeCalls.count, 1,
            "in-room 时第一次真实 petState mutate 必须触发 1 次 UseCase.execute")
        XCTAssertEqual(useCase.executeCalls.first, .walk,
            "入参 state 必须是 viewModel.petState 的新值（.walk）")
    }

    // MARK: - case#F edge: not-in-room → petState mutate 也不调 UseCase

    func test_F_edge_notInRoom_petStateMutate_doesNotTriggerUseCase() async {
        let useCase = MockSyncPetStateUseCase()
        let viewModel = HomeViewModel()
        let appState = AppState()
        // 不调 setCurrentRoomId → currentRoomId 默认 nil.

        let service = PetStateSyncTriggerService(
            syncPetStateUseCase: useCase,
            homeViewModel: viewModel,
            appState: appState
        )
        service.start()
        await yieldRepeatedly()

        viewModel.petState = .walk
        await yieldRepeatedly()
        viewModel.petState = .run
        await yieldRepeatedly()

        XCTAssertTrue(useCase.executeCalls.isEmpty,
            "not-in-room 时 petState mutate 不应触发 UseCase（roomId preflight 在 service 内拦截）")
    }

    // MARK: - case#G edge: 5s 内重复 set 同 state → 只调 1 次

    /// 不真睡 5s（attempt 1 测试反模式）；用 nowProvider test seam 让 throttle check 时间可控.
    /// 默认 nowProvider() = Date.now，T1 与 T2 之间 wall-clock < 5s 必然命中节流窗口.
    func test_G_edge_throttle_sameStateWithin5s_skipsSecondCall() async {
        let useCase = MockSyncPetStateUseCase()
        let viewModel = HomeViewModel()
        let appState = AppState()
        appState.setCurrentRoomId("room-X")

        let baseDate = Date(timeIntervalSince1970: 1_700_000_000)
        // nowProvider 返定值（baseDate）—— 让 T1 / T2 都看到同一时间 → diff = 0 < 5s 窗口必命中节流.
        let service = PetStateSyncTriggerService(
            syncPetStateUseCase: useCase,
            homeViewModel: viewModel,
            appState: appState
        )
        service.nowProvider = { baseDate }
        service.start()
        await yieldRepeatedly()

        // 第一次 mutate → 触发（throttle 锚点 lastSentState=.walk, lastSentAt=baseDate）
        viewModel.petState = .walk
        await yieldRepeatedly()
        // 第二次 mutate 同 state，nowProvider 仍返 baseDate → diff=0 命中 5s 窗口 → 跳过.
        viewModel.petState = .walk
        await yieldRepeatedly()

        XCTAssertEqual(useCase.executeCalls.count, 1,
            "5s 内重复 set 同 state 必须仅调一次 UseCase（throttle 锚点命中）")
    }

    // MARK: - case#H edge: 5s 后重新 set 同 state → 又调 1 次

    func test_H_edge_throttle_sameStateAfter5s_triggersSecondCall() async {
        let useCase = MockSyncPetStateUseCase()
        let viewModel = HomeViewModel()
        let appState = AppState()
        appState.setCurrentRoomId("room-X")

        let baseDate = Date(timeIntervalSince1970: 1_700_000_000)
        // nowProvider 是 mutable 闭包，可在 test 中改写返回值模拟时间流逝（与改 lastSentAt 等价）.
        var fakeNow = baseDate
        let service = PetStateSyncTriggerService(
            syncPetStateUseCase: useCase,
            homeViewModel: viewModel,
            appState: appState
        )
        service.nowProvider = { fakeNow }
        service.start()
        await yieldRepeatedly()

        // T0：第一次 mutate → 锚点写 (.walk, baseDate).
        viewModel.petState = .walk
        await yieldRepeatedly()
        XCTAssertEqual(useCase.executeCalls.count, 1, "前置：T0 第一次 mutate 触发 1 次")

        // T0+6s：fakeNow 推到 6 秒后，节流窗口已过 → 同 state mutate 应再次触发.
        fakeNow = baseDate.addingTimeInterval(6.0)
        viewModel.petState = .walk
        await yieldRepeatedly()

        XCTAssertEqual(useCase.executeCalls.count, 2,
            "5s 窗口外重新 set 同 state 必须再次触发 UseCase（节流锚点已过期）")
    }

    // MARK: - case#I edge: API 失败 → 不 crash + 下次 state 变化照常 spawn

    func test_I_edge_apiFailure_doesNotCrash_nextMutateStillTriggers() async {
        let useCase = MockSyncPetStateUseCase()
        // 第一次 execute 直接 throw（fire-and-forget Task 内必须 silently 吞）.
        useCase.scriptedError = APIError.network(underlying: NSError(domain: "test", code: -1))
        let viewModel = HomeViewModel()
        let appState = AppState()
        appState.setCurrentRoomId("room-X")

        let service = PetStateSyncTriggerService(
            syncPetStateUseCase: useCase,
            homeViewModel: viewModel,
            appState: appState
        )
        service.start()
        await yieldRepeatedly()

        // 第一次 mutate → UseCase throw → 吞掉，service 不 crash.
        viewModel.petState = .walk
        await yieldRepeatedly()
        XCTAssertEqual(useCase.executeCalls.count, 1,
            "失败也算 sent；UseCase 被调过 + throw 被 service 内 catch 吞掉")

        // 清错误，第二次 mutate（不同 state，避开 throttle 窗口）→ 应再次触发.
        useCase.scriptedError = nil
        viewModel.petState = .run
        await yieldRepeatedly()
        XCTAssertEqual(useCase.executeCalls.count, 2,
            "上次失败不应阻塞后续触发；下次 state 变化照常 spawn")
    }

    // MARK: - Helpers

    /// 让出当前 task N 次，让 Combine sink + spawned Task + UseCase.execute 排队完成.
    /// 与 Story 15.2 case#D / 15.3 case#E 同模式.
    private func yieldRepeatedly(times: Int = 8) async {
        for _ in 0..<times {
            await Task.yield()
        }
    }
}

// MARK: - Test Doubles

/// MockSyncPetStateUseCase：记录 execute 调用 + scripted error / outcome.
/// `@unchecked Sendable`：与 sibling MockSyncStepsUseCase 同模式（mutable 字段但测试串行调用）.
final class MockSyncPetStateUseCase: SyncPetStateUseCaseProtocol, @unchecked Sendable {
    /// 已 execute 调用列表（按顺序记录入参 state；让测试断言"调了几次 + 入参是什么"）.
    private(set) var executeCalls: [MotionState] = []

    /// scripted error：非 nil 时 execute 抛此 error（service 应 silently 吞）.
    var scriptedError: Error?

    /// scripted outcome：默认 .success(echoedState: 1)；execute 在无 error 时返此值.
    var scriptedOutcome: SyncPetStateUseCaseOutcome = .success(echoedState: 1)

    func execute(state: MotionState) async throws -> SyncPetStateUseCaseOutcome {
        executeCalls.append(state)
        if let error = scriptedError {
            throw error
        }
        return scriptedOutcome
    }
}
