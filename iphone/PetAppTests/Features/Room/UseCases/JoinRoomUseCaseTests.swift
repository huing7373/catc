// JoinRoomUseCaseTests.swift
// Story 12.7 AC2: JoinRoomUseCase 单元测试（≥4 case：happy / 6002 透传 / 6001 透传 / mismatch 抛 .decoding）.

import XCTest
@testable import PetApp

@MainActor
final class JoinRoomUseCaseTests: XCTestCase {

    // MARK: - case#1 happy: data.roomId == 传入 roomId → 不抛 + appState 写入

    func testExecuteHappyPathWritesAppState() async throws {
        let mock = MockRoomRepository()
        mock.joinRoomStub = .success(JoinRoomResponse(roomId: "3001", joined: true))
        let appState = AppState()
        let useCase = DefaultJoinRoomUseCase(roomRepository: mock, appState: appState)

        try await useCase.execute(roomId: "3001")

        XCTAssertEqual(appState.currentRoomId, "3001",
                       "joinRoom 成功后必须 setCurrentRoomId 让 RealRoomViewModel.subscribeRoomIdConnect 触发")
        XCTAssertEqual(mock.callCount(of: "joinRoom(roomId:)"), 1)
        XCTAssertEqual(mock.lastArgumentsSnapshot().first as? String, "3001")
    }

    // MARK: - case#2 edge: 6002 房间已满 → 透传 + appState 不变

    func testExecuteRethrowsBusiness6002() async {
        let mock = MockRoomRepository()
        mock.joinRoomStub = .failure(APIError.business(code: 6002, message: "房间已满", requestId: "req_x"))
        let appState = AppState()
        let useCase = DefaultJoinRoomUseCase(roomRepository: mock, appState: appState)

        do {
            try await useCase.execute(roomId: "3001")
            XCTFail("应抛 APIError.business(6002)")
        } catch let APIError.business(code, _, _) {
            XCTAssertEqual(code, 6002)
        } catch {
            XCTFail("意外错误：\(error)")
        }
        XCTAssertNil(appState.currentRoomId, "joinRoom 失败必须保持 appState.currentRoomId 不变")
    }

    // MARK: - case#3 edge: 6001 房间不存在 → 透传 + appState 不变

    func testExecuteRethrowsBusiness6001() async {
        let mock = MockRoomRepository()
        mock.joinRoomStub = .failure(APIError.business(code: 6001, message: "房间不存在", requestId: "req_y"))
        let appState = AppState()
        let useCase = DefaultJoinRoomUseCase(roomRepository: mock, appState: appState)

        do {
            try await useCase.execute(roomId: "3001")
            XCTFail("应抛 APIError.business(6001)")
        } catch let APIError.business(code, _, _) {
            XCTAssertEqual(code, 6001)
        } catch {
            XCTFail("意外错误：\(error)")
        }
        XCTAssertNil(appState.currentRoomId)
    }

    // MARK: - case#4 edge: response.roomId mismatch → throw .decoding(JoinRoomMismatchError) + appState 不变

    func testExecuteThrowsDecodingOnRoomIdMismatch() async {
        let mock = MockRoomRepository()
        mock.joinRoomStub = .success(JoinRoomResponse(roomId: "9999", joined: true))
        let appState = AppState()
        let useCase = DefaultJoinRoomUseCase(roomRepository: mock, appState: appState)

        do {
            try await useCase.execute(roomId: "3001")
            XCTFail("response.roomId mismatch 应抛 APIError.decoding(JoinRoomMismatchError)")
        } catch let APIError.decoding(underlying) {
            guard let mismatch = underlying as? JoinRoomMismatchError else {
                XCTFail("underlying 应是 JoinRoomMismatchError，实得 \(underlying)")
                return
            }
            XCTAssertEqual(mismatch.requested, "3001")
            XCTAssertEqual(mismatch.received, "9999")
        } catch {
            XCTFail("意外错误：\(error)")
        }
        XCTAssertNil(appState.currentRoomId,
                     "mismatch 路径不应写 appState（防止用户被切到错误房间）")
    }

    // MARK: - case#5 edge: 6003 透传

    func testExecuteRethrowsBusiness6003() async {
        let mock = MockRoomRepository()
        mock.joinRoomStub = .failure(APIError.business(code: 6003, message: "已在房间", requestId: "req_z"))
        let appState = AppState()
        let useCase = DefaultJoinRoomUseCase(roomRepository: mock, appState: appState)

        do {
            try await useCase.execute(roomId: "3001")
            XCTFail("应抛 APIError.business(6003)")
        } catch let APIError.business(code, _, _) {
            XCTAssertEqual(code, 6003)
        } catch {
            XCTFail("意外错误：\(error)")
        }
        XCTAssertNil(appState.currentRoomId)
    }

    // MARK: - Story 12.7 r6 [P1] fix: stale join response 不能 wipe newer room selection

    /// 场景：entryRoomId == nil（idle Home join room A）→ joinRoom await 期间用户切 tab + join room "B" →
    /// join A HTTP 200 迟到. 旧实装无条件 setCurrentRoomId("A") 静默把 user 切回 stale room A;
    /// 修复后 guard entry==current → "B" 必须保留.
    ///
    /// **r14 [P1] 变更**：stale path 现在抛 `RoomNavigationStaleError` 而非 silent success.
    func testExecuteThrowsStaleErrorAndDoesNotWipeNewerRoomSelectionWhenStaleResponseArrives() async throws {
        let mock = MockRoomRepository()
        mock.joinRoomStub = .success(JoinRoomResponse(roomId: "A", joined: true))
        let appState = AppState()
        // entryRoomId == nil（idle Home）
        appState.setCurrentRoomId(nil)
        let useCase = DefaultJoinRoomUseCase(roomRepository: mock, appState: appState)

        // 在 joinRoom await 期间模拟用户切到新房间 B.
        mock.joinRoomBeforeReturn = { @Sendable in
            await MainActor.run { appState.setCurrentRoomId("B") }
        }

        do {
            try await useCase.execute(roomId: "A")
            XCTFail("stale path 应抛 RoomNavigationStaleError")
        } catch let staleError as RoomNavigationStaleError {
            XCTAssertEqual(staleError.source, .joinRoom,
                           "stale signal 必须标注 source=joinRoom")
        } catch {
            XCTFail("意外错误：\(error)")
        }

        XCTAssertEqual(appState.currentRoomId, "B",
                       "stale join HTTP-200 response 不得 wipe 用户已切的新房间 \"B\"（防 race）")
    }

    // MARK: - Story 12.7 r10 [P2] fix: ABA race - generation token 不能被 nil→B→nil cycle 骗过

    /// 场景（codex r10 P2）：entry == nil（idle Home join A, gen G0）→ joinRoom A await 期间 user
    /// 短暂进入 B (gen G1) → leave B 回 idle (gen G2, currentRoomId nil) → join A HTTP 200 迟到.
    /// 旧 `liveRoomId == entryRoomId == nil` 判断 → 校验通过 → setCurrentRoomId("A") → 用户被切到 stale A.
    /// 新 generation 判断：entryGen == G0，liveGen == G2 → mismatch → 拒绝 setCurrentRoomId.
    ///
    /// **r14 [P1] 变更**：stale path 现在抛 `RoomNavigationStaleError` 而非 silent success.
    func testExecuteThrowsStaleErrorAcrossABAcycleViaGeneration() async throws {
        let mock = MockRoomRepository()
        mock.joinRoomStub = .success(JoinRoomResponse(roomId: "A", joined: true))
        let appState = AppState()
        // entryRoomId == nil
        XCTAssertNil(appState.currentRoomId)
        let initialGen = appState.roomNavigationGeneration
        let useCase = DefaultJoinRoomUseCase(roomRepository: mock, appState: appState)

        // 在 joinRoom await 期间模拟 ABA cycle: nil → "B" → nil（currentRoomId 回到原值，gen +2）.
        mock.joinRoomBeforeReturn = { @Sendable in
            await MainActor.run {
                appState.setCurrentRoomId("B")
                appState.setCurrentRoomId(nil)
            }
        }

        do {
            try await useCase.execute(roomId: "A")
            XCTFail("ABA cycle 后 stale path 必须抛 RoomNavigationStaleError")
        } catch let staleError as RoomNavigationStaleError {
            XCTAssertEqual(staleError.source, .joinRoom)
        } catch {
            XCTFail("意外错误：\(error)")
        }

        XCTAssertNil(appState.currentRoomId,
                     "ABA cycle 后 stale join response 不得把 currentRoomId 切到 stale A（generation token guard）")
        XCTAssertGreaterThan(appState.roomNavigationGeneration, initialGen,
                             "navigation generation 必须严格单调")
    }

    // MARK: - Story 12.7 r14 [P1] fix: happy path 不抛 RoomNavigationStaleError（防回归）

    func testExecuteHappyPathDoesNotThrowStaleError() async throws {
        let mock = MockRoomRepository()
        mock.joinRoomStub = .success(JoinRoomResponse(roomId: "7777", joined: true))
        let appState = AppState()
        let useCase = DefaultJoinRoomUseCase(roomRepository: mock, appState: appState)

        do {
            try await useCase.execute(roomId: "7777")
        } catch is RoomNavigationStaleError {
            XCTFail("happy path 不应抛 RoomNavigationStaleError（generation 未变）")
        } catch {
            XCTFail("意外错误：\(error)")
        }
        XCTAssertEqual(appState.currentRoomId, "7777")
    }
}
