// CreateRoomUseCaseTests.swift
// Story 12.7 AC1: CreateRoomUseCase 单元测试（≥3 case：happy / 6003 透传 / network 透传）.

import XCTest
@testable import PetApp

@MainActor
final class CreateRoomUseCaseTests: XCTestCase {

    // MARK: - case#1 happy: repo 返回 roomId="3001" → execute 返回 + appState 写入

    func testExecuteHappyPathWritesAppStateAndReturnsRoomId() async throws {
        let mock = MockRoomRepository()
        mock.createRoomStub = .success(
            CreateRoomResponse(
                room: CreateRoomRoomDTO(
                    id: "3001",
                    creatorUserId: "10001",
                    maxMembers: 4,
                    memberCount: 1,
                    status: 1
                )
            )
        )
        let appState = AppState()
        let useCase = DefaultCreateRoomUseCase(roomRepository: mock, appState: appState)

        let returned = try await useCase.execute()

        XCTAssertEqual(returned, "3001", "execute() 应返回 response.room.id")
        XCTAssertEqual(appState.currentRoomId, "3001",
                       "execute() 成功后必须 setCurrentRoomId 让 RealRoomViewModel.subscribeRoomIdConnect 触发")
        XCTAssertEqual(mock.callCount(of: "createRoom()"), 1)
    }

    // MARK: - case#2 edge: 6003（用户已在房间）→ rethrow + appState 不变

    func testExecuteRethrowsBusiness6003WithoutMutatingAppState() async {
        let mock = MockRoomRepository()
        mock.createRoomStub = .failure(APIError.business(code: 6003, message: "已在房间", requestId: "req_x"))
        let appState = AppState()
        appState.setCurrentRoomId(nil)
        let useCase = DefaultCreateRoomUseCase(roomRepository: mock, appState: appState)

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.business(6003)")
        } catch let APIError.business(code, _, _) {
            XCTAssertEqual(code, 6003)
        } catch {
            XCTFail("意外错误：\(error)")
        }
        XCTAssertNil(appState.currentRoomId,
                     "createRoom 失败必须保持 appState.currentRoomId 不变（不能误写让 UI 切到 RoomView）")
    }

    // MARK: - case#3 edge: network → rethrow + appState 不变

    func testExecuteRethrowsNetworkErrorWithoutMutatingAppState() async {
        let mock = MockRoomRepository()
        mock.createRoomStub = .failure(APIError.network(underlying: URLError(.notConnectedToInternet)))
        let appState = AppState()
        let useCase = DefaultCreateRoomUseCase(roomRepository: mock, appState: appState)

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.network")
        } catch let APIError.network(underlying) {
            XCTAssertEqual((underlying as? URLError)?.code, .notConnectedToInternet)
        } catch {
            XCTFail("意外错误：\(error)")
        }
        XCTAssertNil(appState.currentRoomId)
    }

    // MARK: - case#4 edge: 1009 服务繁忙 → rethrow + appState 不变

    func testExecuteRethrowsBusiness1009WithoutMutatingAppState() async {
        let mock = MockRoomRepository()
        mock.createRoomStub = .failure(APIError.business(code: 1009, message: "服务繁忙", requestId: "req_y"))
        let appState = AppState()
        let useCase = DefaultCreateRoomUseCase(roomRepository: mock, appState: appState)

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.business(1009)")
        } catch let APIError.business(code, _, _) {
            XCTAssertEqual(code, 1009)
        } catch {
            XCTFail("意外错误：\(error)")
        }
        XCTAssertNil(appState.currentRoomId)
    }
}
