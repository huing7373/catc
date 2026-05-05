// SyncStepsUseCaseTests.swift
// Story 8.5 AC10.1: SyncStepsUseCase 单元测试.
// 不引第三方断言 lib（XCTest only；ADR-0002 §3.1）.

import XCTest
@testable import PetApp

@MainActor
final class SyncStepsUseCaseTests: XCTestCase {

    // happy: 同步成功 → AppState.currentStepAccount 写入正确（epics.md AC 行 1579）.
    //
    // codex review round 1 [P2] fix 后：use case 自己从 captured `now` 派生 syncDate /
    // clientTimestamp，**不再**用 dateProvider.todayString() / nowMillis()；测试期望值因此
    // 改成"用同样的 Date / TimeZone.current 派生的字符串"——TZ-independent，CI 任何时区都过.
    func testExecute_happy_syncSuccess_writesAppState() async throws {
        let healthMock = HealthProviderMock()
        let fixedNow = Date(timeIntervalSince1970: 1_776_920_345)
        let dayKey = Calendar.current.startOfDay(for: fixedNow)
        healthMock.readDailyTotalStepsStub[dayKey] = 3580

        let repoMock = MockStepRepository()
        repoMock.stubResponse = .success(StepsSyncResponse(
            acceptedDeltaSteps: 120,
            stepAccount: StepAccountInSyncResponse(totalSteps: 1140, availableSteps: 840, consumedSteps: 300)
        ))

        let appState = AppState()
        let dateProvider = MockDateProvider(
            now: fixedNow,
            todayString: "ignored-after-p2-fix",  // use case 不再调 todayString()
            nowMillis: 0  // use case 不再调 nowMillis()
        )
        let useCase = DefaultSyncStepsUseCase(
            healthProvider: healthMock,
            repository: repoMock,
            appState: appState,
            dateProvider: dateProvider
        )

        try await useCase.execute(motionState: .walk)

        XCTAssertEqual(repoMock.invocations.count, 1, "repo.syncSteps 应被调一次")
        XCTAssertEqual(repoMock.invocations.first?.syncDate, expectedLocalDateString(from: fixedNow),
            "syncDate 应从 captured now 派生（与 use case formatLocalDateString 一致；TZ-independent）")
        XCTAssertEqual(repoMock.invocations.first?.clientTotalSteps, 3580)
        XCTAssertEqual(repoMock.invocations.first?.motionState, 2)  // .walk → 2
        XCTAssertEqual(repoMock.invocations.first?.clientTimestamp, 1_776_920_345_000,
            "clientTimestamp = Int64(now.timeIntervalSince1970 * 1000) 直接派生")
        XCTAssertEqual(appState.currentStepAccount?.totalSteps, 1140)
        XCTAssertEqual(appState.currentStepAccount?.availableSteps, 840)
        XCTAssertEqual(appState.currentStepAccount?.consumedSteps, 300)
    }

    // edge: HealthProvider 抛 permissionDenied → useCase 透传错误 + AppState 不被改动.
    func testExecute_edge_healthPermissionDenied_throwsAndLeavesAppStateUnchanged() async {
        let healthMock = HealthProviderMock()
        healthMock.readDailyTotalStepsError = HealthProviderError.permissionDenied
        let repoMock = MockStepRepository()
        let appState = AppState()
        let useCase = DefaultSyncStepsUseCase(
            healthProvider: healthMock,
            repository: repoMock,
            appState: appState,
            dateProvider: MockDateProvider.fixed
        )

        do {
            try await useCase.execute(motionState: .rest)
            XCTFail("应抛 HealthProviderError.permissionDenied")
        } catch let error as HealthProviderError {
            XCTAssertEqual(error, .permissionDenied)
        } catch {
            XCTFail("应抛 HealthProviderError.permissionDenied，实际抛 \(error)")
        }

        XCTAssertEqual(repoMock.invocations.count, 0, "permission denied 时不应调 repo")
        XCTAssertNil(appState.currentStepAccount)
    }

    // edge: repo 抛 APIError.network → useCase 透传错误 + AppState 不被改动（epics.md AC 行 1583）.
    func testExecute_edge_networkError_throwsAndLeavesAppStateUnchanged() async {
        let healthMock = HealthProviderMock()
        let dayKey = Calendar.current.startOfDay(for: Date())
        healthMock.readDailyTotalStepsStub[dayKey] = 1000
        let repoMock = MockStepRepository()
        repoMock.stubResponse = .failure(APIError.network(underlying: NSError(domain: "test", code: -1)))
        let appState = AppState()
        let useCase = DefaultSyncStepsUseCase(
            healthProvider: healthMock,
            repository: repoMock,
            appState: appState,
            dateProvider: MockDateProvider.fixed
        )

        do {
            try await useCase.execute(motionState: .rest)
            XCTFail("应抛 APIError.network")
        } catch is APIError {
            // 期望
        } catch {
            XCTFail("应抛 APIError，实际抛 \(error)")
        }

        XCTAssertNil(appState.currentStepAccount, "失败时不应改动 AppState")
    }

    // happy: syncDate / clientTimestamp 全部从同一个 captured `now` 派生（codex review round 1 [P2] fix）.
    //
    // 验证：use case 即使 dateProvider 的 todayString() / nowMillis() 返回任意"假"值，也不会取它们；
    // 只用 dateProvider.now() 一次，所有派生字段都基于这个 Date —— 跨午夜 race 不可能发生.
    //
    // 预期：syncDate = formatLocalDateString(now) / clientTimestamp = Int64(now.timeIntervalSince1970*1000),
    // 与 todayString / nowMillis 的 stub 完全无关.
    func testExecute_derivesAllTimeFieldsFromCapturedNow() async throws {
        let healthMock = HealthProviderMock()
        let fixedNow = Date(timeIntervalSince1970: 1_777_006_745)
        let dayKey = Calendar.current.startOfDay(for: fixedNow)
        healthMock.readDailyTotalStepsStub[dayKey] = 100
        let repoMock = MockStepRepository()
        repoMock.stubResponse = .success(StepsSyncResponse(
            acceptedDeltaSteps: 100,
            stepAccount: StepAccountInSyncResponse(totalSteps: 100, availableSteps: 100, consumedSteps: 0)
        ))
        let appState = AppState()
        // 故意把 todayString / nowMillis 设成与 fixedNow 不一致的"陷阱值".
        // 修复后 use case 不应取这些值；如果取了，syncDate 会变成 "BAD-DATE-1900-01-01"（断言会挂）.
        let dateProvider = MockDateProvider(
            now: fixedNow,
            todayString: "BAD-DATE-1900-01-01",
            nowMillis: 999  // 陷阱：不是 fixedNow 派生的毫秒值
        )
        let useCase = DefaultSyncStepsUseCase(
            healthProvider: healthMock,
            repository: repoMock,
            appState: appState,
            dateProvider: dateProvider
        )

        try await useCase.execute(motionState: .rest)

        XCTAssertEqual(repoMock.invocations.first?.syncDate, expectedLocalDateString(from: fixedNow),
            "syncDate 必须从 captured now 派生，而非 dateProvider.todayString()")
        XCTAssertEqual(repoMock.invocations.first?.clientTimestamp, 1_777_006_745_000,
            "clientTimestamp 必须从 captured now 派生（Int64(now.timeIntervalSince1970*1000)），" +
            "而非 dateProvider.nowMillis()")
    }

    // happy: motionState 三态 → wireValue 各自正确（验 AC4.1 extension）.
    func testMotionStateWireValue_allThreeCases() {
        XCTAssertEqual(MotionState.rest.wireValue, 1)
        XCTAssertEqual(MotionState.walk.wireValue, 2)
        XCTAssertEqual(MotionState.run.wireValue, 3)
    }

    /// Mirror of `DefaultSyncStepsUseCase.formatLocalDateString(from:)` —— 用同样的
    /// gregorian / en_US_POSIX / TimeZone.current 派生本机时区当日 "YYYY-MM-DD".
    /// 测试断言用本 helper 计算 expected 值，从而 TZ-independent（CI 在 UTC 也好、本地在 CST 也好都过）.
    fileprivate func expectedLocalDateString(from date: Date) -> String {
        let formatter = DateFormatter()
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.dateFormat = "yyyy-MM-dd"
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = TimeZone.current
        return formatter.string(from: date)
    }
}

// MARK: - Test Doubles

final class MockStepRepository: StepRepositoryProtocol, @unchecked Sendable {
    var stubResponse: Result<StepsSyncResponse, Error>?
    private(set) var invocations: [StepsSyncRequest] = []

    func syncSteps(_ request: StepsSyncRequest) async throws -> StepsSyncResponse {
        invocations.append(request)
        guard let stub = stubResponse else {
            throw APIError.decoding(underlying: NSError(domain: "MockStepRepository", code: -100))
        }
        switch stub {
        case .success(let r): return r
        case .failure(let e): throw e
        }
    }
}

final class MockDateProvider: DateProvider, @unchecked Sendable {
    let nowDate: Date
    let todayStr: String
    let nowMs: Int64

    init(now: Date, todayString: String, nowMillis: Int64) {
        self.nowDate = now
        self.todayStr = todayString
        self.nowMs = nowMillis
    }

    static let fixed = MockDateProvider(
        now: Date(timeIntervalSince1970: 1_776_920_345),
        todayString: "2026-04-23",
        nowMillis: 1_776_920_345_000
    )

    func now() -> Date { nowDate }
    func nowMillis() -> Int64 { nowMs }
    func todayString() -> String { todayStr }
}
