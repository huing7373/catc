// LeaveRoomUseCaseTests.swift
// Story 12.7 AC3: LeaveRoomUseCase 单元测试（≥4 case：happy / nil 早 return / 6004 视同成功 / 1009 透传）.

import XCTest
@testable import PetApp

@MainActor
final class LeaveRoomUseCaseTests: XCTestCase {

    // MARK: - case#1 happy: appState.currentRoomId="3001" + repo 返回 left=true → 不抛 + appState 置 nil

    func testExecuteHappyPathClearsAppState() async throws {
        let mock = MockRoomRepository()
        mock.leaveRoomStub = .success(LeaveRoomResponse(roomId: "3001", left: true))
        let appState = AppState()
        appState.setCurrentRoomId("3001")
        let useCase = DefaultLeaveRoomUseCase(roomRepository: mock, appState: appState)

        try await useCase.execute()

        XCTAssertNil(appState.currentRoomId,
                     "leaveRoom HTTP 200 后必须立即 setCurrentRoomId(nil)（HTTP 200 = authoritative leave 信号）")
        XCTAssertEqual(mock.callCount(of: "leaveRoom(roomId:)"), 1)
        XCTAssertEqual(mock.lastArgumentsSnapshot().first as? String, "3001")
    }

    // MARK: - case#2 happy edge: appState.currentRoomId == nil → 立即 return（idempotent，不调 repo）

    func testExecuteEarlyReturnsWhenCurrentRoomIdNil() async throws {
        let mock = MockRoomRepository()
        // 故意让 stub 是 .failure —— 若 useCase 错误调了 repo，会暴露问题
        mock.leaveRoomStub = .failure(MockError.notStubbed)
        let appState = AppState()
        appState.setCurrentRoomId(nil)
        let useCase = DefaultLeaveRoomUseCase(roomRepository: mock, appState: appState)

        try await useCase.execute()

        XCTAssertNil(appState.currentRoomId)
        XCTAssertEqual(mock.callCount(of: "leaveRoom(roomId:)"), 0,
                       "currentRoomId == nil 时应早 return，不应调 repo（leave 是 idempotent 操作）")
    }

    // MARK: - case#3 edge: 6004 视同成功 → 不抛 + appState 置 nil（leave-idempotent）

    func testExecuteTreats6004AsSuccessAndClearsAppState() async throws {
        let mock = MockRoomRepository()
        mock.leaveRoomStub = .failure(APIError.business(code: 6004, message: "用户不在房间", requestId: "req_x"))
        let appState = AppState()
        appState.setCurrentRoomId("3001")
        let useCase = DefaultLeaveRoomUseCase(roomRepository: mock, appState: appState)

        // 关键：6004 不应抛错
        try await useCase.execute()

        XCTAssertNil(appState.currentRoomId,
                     "6004 视同成功路径必须 setCurrentRoomId(nil)（leave-idempotent）")
    }

    // MARK: - case#4 edge: 1009 透传 + appState 保留原值（让用户重试）

    func testExecuteRethrowsBusiness1009AndPreservesAppState() async {
        let mock = MockRoomRepository()
        mock.leaveRoomStub = .failure(APIError.business(code: 1009, message: "服务繁忙", requestId: "req_y"))
        let appState = AppState()
        appState.setCurrentRoomId("3001")
        let useCase = DefaultLeaveRoomUseCase(roomRepository: mock, appState: appState)

        do {
            try await useCase.execute()
            XCTFail("1009 应该被透传")
        } catch let APIError.business(code, _, _) {
            XCTAssertEqual(code, 1009)
        } catch {
            XCTFail("意外错误：\(error)")
        }
        XCTAssertEqual(appState.currentRoomId, "3001",
                       "1009 透传时应保留 appState.currentRoomId 让用户在 RoomView 内重试")
    }

    // MARK: - case#5 edge: network 透传 + appState 保留原值

    func testExecuteRethrowsNetworkErrorAndPreservesAppState() async {
        let mock = MockRoomRepository()
        mock.leaveRoomStub = .failure(APIError.network(underlying: URLError(.notConnectedToInternet)))
        let appState = AppState()
        appState.setCurrentRoomId("3001")
        let useCase = DefaultLeaveRoomUseCase(roomRepository: mock, appState: appState)

        do {
            try await useCase.execute()
            XCTFail("network 应该被透传")
        } catch APIError.network {
            // pass
        } catch {
            XCTFail("意外错误：\(error)")
        }
        XCTAssertEqual(appState.currentRoomId, "3001",
                       "network 透传时应保留 appState.currentRoomId 让用户重试")
    }

    // MARK: - Story 12.7 r2 [P2] fix: stale leave response 不能 wipe newer room selection

    /// 场景：start at room "3001" → execute leaveRoom("3001") 异步进行中 → 用户已切到 "5005"
    /// → leave HTTP 200 返回. 旧实装无条件 setCurrentRoomId(nil) 会 wipe "5005";
    /// 修复后 guard target==current → "5005" 必须保留.
    func testExecuteHttp200DoesNotWipeNewerRoomSelection() async throws {
        let mock = MockRoomRepository()
        mock.leaveRoomStub = .success(LeaveRoomResponse(roomId: "3001", left: true))
        let appState = AppState()
        appState.setCurrentRoomId("3001")
        let useCase = DefaultLeaveRoomUseCase(roomRepository: mock, appState: appState)

        // 在 leaveRoom await 期间模拟用户切到新房间.
        mock.leaveRoomBeforeReturn = { @Sendable in
            await MainActor.run { appState.setCurrentRoomId("5005") }
        }

        try await useCase.execute()

        XCTAssertEqual(appState.currentRoomId, "5005",
                       "stale leave HTTP-200 response 不得 wipe 用户已切的新房间 \"5005\"（防 race）")
    }

    /// 场景：start at room "3001" → execute leaveRoom("3001") 异步进行中 → 用户已切到 "5005"
    /// → leave HTTP 抛 business 6004（含 V1 §10.5 race "current_room_id != path roomId" 路径）.
    /// 旧实装无条件 setCurrentRoomId(nil) 会 wipe "5005"; 修复后 guard target==current → "5005" 必须保留.
    func testExecuteBusiness6004DoesNotWipeNewerRoomSelection() async throws {
        let mock = MockRoomRepository()
        mock.leaveRoomStub = .failure(APIError.business(code: 6004, message: "用户不在房间", requestId: "req_z"))
        let appState = AppState()
        appState.setCurrentRoomId("3001")
        let useCase = DefaultLeaveRoomUseCase(roomRepository: mock, appState: appState)

        mock.leaveRoomBeforeReturn = { @Sendable in
            await MainActor.run { appState.setCurrentRoomId("5005") }
        }

        // 6004 不应抛错（leave-idempotent）.
        try await useCase.execute()

        XCTAssertEqual(appState.currentRoomId, "5005",
                       "stale leave 6004 response 不得 wipe 用户已切的新房间 \"5005\"（防 V1 §10.5 race 路径）")
    }
}
