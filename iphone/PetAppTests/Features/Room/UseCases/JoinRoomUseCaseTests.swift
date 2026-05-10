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
}
