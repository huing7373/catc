// RoomRepository.swift
// Story 12.7 AC4: 房间 REST 仓储层（封装 createRoom / joinRoom / leaveRoom 三个 endpoint 调用）.
//
// 与 HomeRepository / AuthRepository 同模式：协议方法返回 wire DTO；APIError 原样透传.
//
// 注入的 APIClient 是 container.apiClient —— 已被 ADR-0008 v2 AuthBoundaryAPIClient 包装；
// 业务请求 401 自动触发**全局 cold-start**（清 SessionStore + 重跑 bootstrap）.
//
// `DefaultRoomRepository` 是 `struct`：value type，无内部状态，构造廉价；与 DefaultHomeRepository 同模式.

import Foundation

public protocol RoomRepositoryProtocol: Sendable {
    /// 调 POST /api/v1/rooms 创建房间.
    /// - Returns: CreateRoomResponse（含 room.id / creatorUserId / maxMembers / memberCount / status）
    /// - Throws: APIError.business(6003 / 1009 / ...) / APIError.network / APIError.unauthorized
    func createRoom() async throws -> CreateRoomResponse

    /// 调 POST /api/v1/rooms/{roomId}/join 加入房间.
    /// - Parameter roomId: 目标房间号（BIGINT 字符串化）
    /// - Returns: JoinRoomResponse（含 data.roomId / data.joined）
    /// - Throws: APIError.business(1002 / 6001 / 6002 / 6003 / 6005 / 1009) / APIError.network / APIError.unauthorized
    func joinRoom(roomId: String) async throws -> JoinRoomResponse

    /// 调 POST /api/v1/rooms/{roomId}/leave 退出房间.
    /// - Parameter roomId: 目标房间号（通常是 appState.currentRoomId）
    /// - Returns: LeaveRoomResponse（含 data.roomId / data.left）
    /// - Throws: APIError.business(1002 / 6004 / 1009) / APIError.network / APIError.unauthorized
    func leaveRoom(roomId: String) async throws -> LeaveRoomResponse
}

public struct DefaultRoomRepository: RoomRepositoryProtocol {
    private let apiClient: APIClientProtocol

    public init(apiClient: APIClientProtocol) {
        self.apiClient = apiClient
    }

    public func createRoom() async throws -> CreateRoomResponse {
        try await apiClient.request(RoomEndpoints.createRoom())
    }

    public func joinRoom(roomId: String) async throws -> JoinRoomResponse {
        try await apiClient.request(RoomEndpoints.joinRoom(roomId: roomId))
    }

    public func leaveRoom(roomId: String) async throws -> LeaveRoomResponse {
        try await apiClient.request(RoomEndpoints.leaveRoom(roomId: roomId))
    }
}
