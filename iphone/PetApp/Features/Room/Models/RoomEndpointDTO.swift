// RoomEndpointDTO.swift
// Story 12.7 AC4: 房间 REST 接口的 wire DTO（与 V1 §10.1 / §10.4 / §10.5 严格对齐）.
//
// 设计：与 HomeResponse 同模式 —— APIClient 已剥 envelope（code / message / data / requestId），
// 本类型仅模型 envelope.data 字段内容.
//
// 所有 required 字段非 Optional —— JSONDecoder 自动 fail 不合法 server response → APIError.decoding 透传.
// 继承 lesson 2026-05-09-ws-codec-must-validate-required-fields-12-4-r2.md：codec 必须 fail-fast,
// 不要用 Optional 装非空字段（Optional silent coerce 给后续 UI 埋坑）.

import Foundation

// MARK: - POST /rooms response （V1 §10.1）

/// `data` 字段（envelope.data）—— `room` 子结构.
public struct CreateRoomResponse: Decodable, Equatable, Sendable {
    public let room: CreateRoomRoomDTO

    public init(room: CreateRoomRoomDTO) {
        self.room = room
    }
}

/// `data.room` —— V1 §10.1 行 1083-1109 字段表.
public struct CreateRoomRoomDTO: Decodable, Equatable, Sendable {
    /// BIGINT 字符串化（V1 AR21）—— 后续 setCurrentRoomId 入参.
    public let id: String
    public let creatorUserId: String
    public let maxMembers: Int        // 固定 4
    public let memberCount: Int       // 创建后含创建者自己 = 1
    public let status: Int            // 1 = active

    public init(id: String, creatorUserId: String, maxMembers: Int, memberCount: Int, status: Int) {
        self.id = id
        self.creatorUserId = creatorUserId
        self.maxMembers = maxMembers
        self.memberCount = memberCount
        self.status = status
    }
}

// MARK: - POST /rooms/{roomId}/join response （V1 §10.4）

/// `data` 字段 —— V1 §10.4 行 1432-1438 字段表.
public struct JoinRoomResponse: Decodable, Equatable, Sendable {
    /// BIGINT 字符串化（必填，回带 path roomId）.
    public let roomId: String
    /// 固定 true（业务码 0 时；6001/6002/6003/6005 走 envelope.code 路径，不进 data）.
    public let joined: Bool

    public init(roomId: String, joined: Bool) {
        self.roomId = roomId
        self.joined = joined
    }
}

// MARK: - POST /rooms/{roomId}/leave response （V1 §10.5）

/// `data` 字段 —— V1 §10.5 行 1530-1536 字段表.
public struct LeaveRoomResponse: Decodable, Equatable, Sendable {
    public let roomId: String
    /// 固定 true（业务码 0 时；6004 走 envelope.code 路径）.
    public let left: Bool

    public init(roomId: String, left: Bool) {
        self.roomId = roomId
        self.left = left
    }
}
