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

    /// Story 12.7 r2 [P2] fix 测试基础设施：在 `leaveRoom` 进入但 return stub 之前回调,
    /// 让 race 测试能在 await 返回之前 mutate `appState.currentRoomId`（模拟用户已切到新房间）.
    /// `nil` 时无副作用. 仅 Test target 使用.
    var leaveRoomBeforeReturn: (@Sendable () async -> Void)?

    /// Story 12.7 r6 [P1] fix 测试基础设施：与 leaveRoomBeforeReturn 同模式 ——
    /// 在 `createRoom` 进入但 return stub 之前回调，让 race 测试模拟 await 期间用户已切房间.
    var createRoomBeforeReturn: (@Sendable () async -> Void)?

    /// Story 12.7 r6 [P1] fix 测试基础设施：与 leaveRoomBeforeReturn 同模式 ——
    /// 在 `joinRoom` 进入但 return stub 之前回调，让 race 测试模拟 await 期间用户已切房间.
    var joinRoomBeforeReturn: (@Sendable () async -> Void)?

    func createRoom() async throws -> CreateRoomResponse {
        record(method: "createRoom()")
        if let hook = createRoomBeforeReturn {
            await hook()
        }
        return try createRoomStub.get()
    }

    func joinRoom(roomId: String) async throws -> JoinRoomResponse {
        record(method: "joinRoom(roomId:)", arguments: [roomId])
        if let hook = joinRoomBeforeReturn {
            await hook()
        }
        return try joinRoomStub.get()
    }

    func leaveRoom(roomId: String) async throws -> LeaveRoomResponse {
        record(method: "leaveRoom(roomId:)", arguments: [roomId])
        if let hook = leaveRoomBeforeReturn {
            await hook()
        }
        return try leaveRoomStub.get()
    }
}

#endif
