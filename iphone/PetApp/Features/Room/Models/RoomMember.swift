// RoomMember.swift
// Story 37.8 AC3: RoomScaffoldView 成员列表数据模型.
//
// 设计：value type + Equatable + Sendable + Identifiable，纯展示数据；mock 值在 Mock子类静态属性.
// 节点 4 后 Story 12.1 接 WS room.snapshot 时复用该类型（字段对齐：id / nickname / petLevel / status / isHost）；
// 若发现需要扩展（如 lastSeenAt / pet currentState 等）走 ADR-0010 §4.4 缓解策略，本 story 不预 over-design.

import Foundation

public struct RoomMember: Equatable, Identifiable, Sendable {
    public let id: String         // userId（Story 12.1 后对齐 server user.id）
    public let name: String       // 成员昵称
    public let level: Int         // 小猫等级（mock 6-9；节点 8 后接真实 user_pet level）
    public let status: String     // mock "在玩耍" / "在散步" / "在休息"（节点 5 后接真实 pet.currentState 派生）
    public let isHost: Bool       // 是否房主（决定"队长"标签渲染）

    public init(
        id: String,
        name: String,
        level: Int,
        status: String,
        isHost: Bool
    ) {
        self.id = id
        self.name = name
        self.level = level
        self.status = status
        self.isHost = isHost
    }
}
