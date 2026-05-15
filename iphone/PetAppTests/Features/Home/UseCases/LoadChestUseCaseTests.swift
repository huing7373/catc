// LoadChestUseCaseTests.swift
// Story 21.2 AC7: LoadChestUseCase 单测覆盖（≥ 4 case；本文件给 6 case 全覆盖 happy + error 透传 + fail-fast）.
//
// 测试目标：验证 UseCase = repo.fetchCurrent() → DTO 转 HomeChest → MainActor.run { appState.applyCurrentChest }
// + 错误透传严格.
// 用 MockChestRepository（scripted Result + invocations）.

import XCTest
@testable import PetApp

@MainActor
final class LoadChestUseCaseTests: XCTestCase {

    // MARK: - Helpers

    private static let testUnlockAt = Date(timeIntervalSince1970: 1_800_000_000)

    private func makeResponse(
        id: String = "30001",
        status: Int = 1,
        unlockAt: Date = LoadChestUseCaseTests.testUnlockAt,
        openCostSteps: Int = 1000,
        remainingSeconds: Int = 300
    ) -> ChestCurrentResponse {
        ChestCurrentResponse(
            id: id,
            status: status,
            unlockAt: unlockAt,
            openCostSteps: openCostSteps,
            remainingSeconds: remainingSeconds
        )
    }

    /// 给 AppState 预填一个初始 chest，让测试断言"AppState 上次值保留"语义可表达.
    private func makeAppStateWithInitialChest() -> AppState {
        let appState = AppState()
        appState.currentChest = HomeChest(
            id: "initial-c1",
            status: .counting,
            unlockAt: Date(timeIntervalSince1970: 1_700_000_000),
            openCostSteps: 1000,
            remainingSeconds: 1000
        )
        return appState
    }

    // MARK: - case#1 happy: status=1 counting → 写 AppState

    func testExecuteWritesCountingChestToAppState() async throws {
        let mock = MockChestRepository()
        mock.stubResponse = .success(makeResponse(status: 1, remainingSeconds: 300))
        let appState = AppState()
        let useCase = DefaultLoadChestUseCase(repository: mock, appState: appState)

        try await useCase.execute()

        XCTAssertEqual(mock.invocations, 1)
        XCTAssertEqual(appState.currentChest?.id, "30001")
        XCTAssertEqual(appState.currentChest?.status, .counting)
        XCTAssertEqual(appState.currentChest?.unlockAt, Self.testUnlockAt)
        XCTAssertEqual(appState.currentChest?.openCostSteps, 1000)
        XCTAssertEqual(appState.currentChest?.remainingSeconds, 300)
    }

    // MARK: - case#2 happy: status=2 unlockable → 写 AppState

    func testExecuteWritesUnlockableChestToAppState() async throws {
        let mock = MockChestRepository()
        mock.stubResponse = .success(makeResponse(status: 2, remainingSeconds: 0))
        let appState = AppState()
        let useCase = DefaultLoadChestUseCase(repository: mock, appState: appState)

        try await useCase.execute()

        XCTAssertEqual(appState.currentChest?.status, .unlockable)
        XCTAssertEqual(appState.currentChest?.remainingSeconds, 0)
    }

    // MARK: - case#3 happy: 重复 execute → repo 调两次 + AppState 两次写入（覆盖语义）

    func testExecuteRepeatedlyCallsRepoEachTime() async throws {
        let mock = MockChestRepository()
        mock.stubResponse = .success(makeResponse(id: "30001", remainingSeconds: 300))
        let appState = AppState()
        let useCase = DefaultLoadChestUseCase(repository: mock, appState: appState)

        try await useCase.execute()
        // 第二次换 stub 模拟 server 状态更新
        mock.stubResponse = .success(makeResponse(id: "30001", status: 2, remainingSeconds: 0))
        try await useCase.execute()

        XCTAssertEqual(mock.invocations, 2)
        // 最后一次的值应覆盖上次
        XCTAssertEqual(appState.currentChest?.status, .unlockable)
        XCTAssertEqual(appState.currentChest?.remainingSeconds, 0)
    }

    // MARK: - case#4 edge: APIError.business(4001) → throw + AppState 保留旧值

    func testExecuteThrowsBusinessErrorPreservesAppStateChest() async {
        let mock = MockChestRepository()
        mock.stubResponse = .failure(APIError.business(code: 4001, message: "无任何宝箱", requestId: "req_x"))
        let appState = makeAppStateWithInitialChest()
        let useCase = DefaultLoadChestUseCase(repository: mock, appState: appState)

        do {
            try await useCase.execute()
            XCTFail("应抛 APIError.business")
        } catch let APIError.business(code, message, requestId) {
            XCTAssertEqual(code, 4001)
            XCTAssertEqual(message, "无任何宝箱")
            XCTAssertEqual(requestId, "req_x")
        } catch {
            XCTFail("意外错误类型：\(error)")
        }

        // AppState.currentChest 应保留 setUp 设置的初始值
        XCTAssertEqual(appState.currentChest?.id, "initial-c1",
            "失败时 AppState.currentChest 应保留上次值，不应被吞成 nil")
    }

    // MARK: - case#5 edge: APIError.network → throw + AppState 保留旧值

    func testExecuteThrowsNetworkErrorPreservesAppStateChest() async {
        let mock = MockChestRepository()
        mock.stubResponse = .failure(APIError.network(underlying: URLError(.timedOut)))
        let appState = makeAppStateWithInitialChest()
        let useCase = DefaultLoadChestUseCase(repository: mock, appState: appState)

        do {
            try await useCase.execute()
            XCTFail("应抛 APIError.network")
        } catch let APIError.network(underlying) {
            XCTAssertEqual((underlying as? URLError)?.code, .timedOut)
        } catch {
            XCTFail("意外错误类型：\(error)")
        }

        XCTAssertEqual(appState.currentChest?.id, "initial-c1",
            "失败时 AppState.currentChest 应保留上次值")
    }

    // MARK: - case#6 edge: 未知 status=99 → APIError.decoding(HomeDataDecodingError.unknownChestStatus(99))
    //
    // Story 21.2 AC2 关键决策 2：未知 status fail-fast 抛 .decoding 而非 silently coerce.
    // 与 LoadHomeUseCaseTests.testExecuteUnknownChestStatusThrowsDecoding 同精神.

    func testExecuteUnknownChestStatusThrowsDecoding() async {
        let mock = MockChestRepository()
        mock.stubResponse = .success(makeResponse(status: 99))
        let appState = makeAppStateWithInitialChest()
        let useCase = DefaultLoadChestUseCase(repository: mock, appState: appState)

        do {
            try await useCase.execute()
            XCTFail("未识别 chest.status 应抛 APIError.decoding，不应静默 fallback")
        } catch let APIError.decoding(underlying) {
            guard let homeErr = underlying as? HomeDataDecodingError else {
                XCTFail("underlying 应是 HomeDataDecodingError，实得 \(underlying)")
                return
            }
            XCTAssertEqual(homeErr, .unknownChestStatus(99),
                           "应携带未知 raw 值供 log / 调试")
        } catch {
            XCTFail("意外错误类型：\(error)")
        }

        // 即便走 fail-fast 抛错，AppState.currentChest 也应保留上次值（与 silently coerce 区分;
        // UI 不破坏 + dev 仍能从 error 看到 schema drift）.
        XCTAssertEqual(appState.currentChest?.id, "initial-c1",
            "未知 status fail-fast 时 AppState.currentChest 应保留上次值")
    }
}
