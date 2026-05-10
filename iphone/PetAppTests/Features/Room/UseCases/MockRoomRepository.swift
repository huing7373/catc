// MockRoomRepository.swift
// Story 12.7 测试基础设施: RoomRepositoryProtocol mock；让 UseCase 测试不必构造 repo + APIClient.
//
// 与 MockHomeRepository / MockAuthRepository 同模式：每个方法独立 stub 字段，由测试 setUp 写入；
// method body 通过 record() + try stub.get() 调用一次；继承 MockBase（snapshot-only reads）.

@testable import PetApp
import Foundation

#if DEBUG

final class MockRoomRepository: MockBase, RoomRepositoryProtocol, @unchecked Sendable {
    var createRoomStub: Result<CreateRoomResponse, Error> = .failure(MockError.notStubbed)
    var joinRoomStub: Result<JoinRoomResponse, Error> = .failure(MockError.notStubbed)
    var leaveRoomStub: Result<LeaveRoomResponse, Error> = .failure(MockError.notStubbed)

    func createRoom() async throws -> CreateRoomResponse {
        record(method: "createRoom()")
        return try createRoomStub.get()
    }

    func joinRoom(roomId: String) async throws -> JoinRoomResponse {
        record(method: "joinRoom(roomId:)", arguments: [roomId])
        return try joinRoomStub.get()
    }

    func leaveRoom(roomId: String) async throws -> LeaveRoomResponse {
        record(method: "leaveRoom(roomId:)", arguments: [roomId])
        return try leaveRoomStub.get()
    }
}

#endif
