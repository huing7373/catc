// RoomRepositoryTests.swift
// Story 12.7 AC4: RoomRepository 单元测试（≥3 case，验证 endpoint 属性 + 错误透传）.
//
// 用 Story 2.5 落地的 MockAPIClient（按 path stub）.

import XCTest
@testable import PetApp

@MainActor
final class RoomRepositoryTests: XCTestCase {

    // MARK: - createRoom

    /// case#1 happy: createRoom → endpoint(.createRoom) + path / method / requiresAuth 校验.
    func testCreateRoomCallsCorrectEndpoint() async throws {
        let mock = MockAPIClient()
        let response = CreateRoomResponse(
            room: CreateRoomRoomDTO(
                id: "3001",
                creatorUserId: "10001",
                maxMembers: 4,
                memberCount: 1,
                status: 1
            )
        )
        mock.stubResponse["/api/v1/rooms"] = .success(response)
        let repo = DefaultRoomRepository(apiClient: mock)

        let received = try await repo.createRoom()

        XCTAssertEqual(received.room.id, "3001")
        XCTAssertEqual(received.room.creatorUserId, "10001")
        XCTAssertEqual(mock.invocations.count, 1)
        let endpoint = mock.invocations[0]
        XCTAssertEqual(endpoint.path, "/api/v1/rooms")
        XCTAssertEqual(endpoint.method, .post)
        XCTAssertTrue(endpoint.requiresAuth, "POST /rooms 必须 requiresAuth=true")
        XCTAssertNotNil(endpoint.body, "POST /rooms body 应为空对象 {}（非 nil）")
    }

    // MARK: - joinRoom

    /// case#2 happy: joinRoom("3001") → path 含 "/3001/join" + body 非 nil.
    func testJoinRoomCallsCorrectEndpointWithRoomIdInPath() async throws {
        let mock = MockAPIClient()
        let response = JoinRoomResponse(roomId: "3001", joined: true)
        mock.stubResponse["/api/v1/rooms/3001/join"] = .success(response)
        let repo = DefaultRoomRepository(apiClient: mock)

        let received = try await repo.joinRoom(roomId: "3001")

        XCTAssertEqual(received.roomId, "3001")
        XCTAssertTrue(received.joined)
        XCTAssertEqual(mock.invocations.count, 1)
        let endpoint = mock.invocations[0]
        XCTAssertEqual(endpoint.path, "/api/v1/rooms/3001/join")
        XCTAssertEqual(endpoint.method, .post)
        XCTAssertTrue(endpoint.requiresAuth)
        XCTAssertNotNil(endpoint.body, "POST /rooms/{id}/join body 应为空对象 {}")
    }

    // MARK: - leaveRoom

    /// case#3 happy: leaveRoom("3001") → path 含 "/3001/leave".
    func testLeaveRoomCallsCorrectEndpointWithRoomIdInPath() async throws {
        let mock = MockAPIClient()
        let response = LeaveRoomResponse(roomId: "3001", left: true)
        mock.stubResponse["/api/v1/rooms/3001/leave"] = .success(response)
        let repo = DefaultRoomRepository(apiClient: mock)

        let received = try await repo.leaveRoom(roomId: "3001")

        XCTAssertEqual(received.roomId, "3001")
        XCTAssertTrue(received.left)
        XCTAssertEqual(mock.invocations.count, 1)
        let endpoint = mock.invocations[0]
        XCTAssertEqual(endpoint.path, "/api/v1/rooms/3001/leave")
        XCTAssertEqual(endpoint.method, .post)
        XCTAssertTrue(endpoint.requiresAuth)
    }

    // MARK: - error pass-through

    /// case#4 edge: APIError.business 透传（6003 已在房间）.
    func testCreateRoomPassesThroughBusinessError() async {
        let mock = MockAPIClient()
        mock.stubResponse["/api/v1/rooms"] = .failure(.business(code: 6003, message: "已在房间", requestId: "req_1"))
        let repo = DefaultRoomRepository(apiClient: mock)

        do {
            _ = try await repo.createRoom()
            XCTFail("应抛 APIError.business")
        } catch let APIError.business(code, _, _) {
            XCTAssertEqual(code, 6003)
        } catch {
            XCTFail("意外错误：\(error)")
        }
    }

    /// case#5 edge: APIError.network 透传（joinRoom）.
    func testJoinRoomPassesThroughNetworkError() async {
        let mock = MockAPIClient()
        mock.stubResponse["/api/v1/rooms/3001/join"] = .failure(.network(underlying: URLError(.timedOut)))
        let repo = DefaultRoomRepository(apiClient: mock)

        do {
            _ = try await repo.joinRoom(roomId: "3001")
            XCTFail("应抛 APIError.network")
        } catch let APIError.network(underlying) {
            XCTAssertEqual((underlying as? URLError)?.code, .timedOut)
        } catch {
            XCTFail("意外错误：\(error)")
        }
    }

    /// case#6 edge: 6004 透传（leaveRoom 不在房间）.
    func testLeaveRoomPassesThroughBusinessError6004() async {
        let mock = MockAPIClient()
        mock.stubResponse["/api/v1/rooms/3001/leave"] = .failure(.business(code: 6004, message: "不在房间", requestId: "req_2"))
        let repo = DefaultRoomRepository(apiClient: mock)

        do {
            _ = try await repo.leaveRoom(roomId: "3001")
            XCTFail("应抛 APIError.business(6004)")
        } catch let APIError.business(code, _, _) {
            XCTAssertEqual(code, 6004)
        } catch {
            XCTFail("意外错误：\(error)")
        }
    }
}
