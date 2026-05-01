// Friend.swift
// Story 37.10 AC3: FriendsScaffoldView 好友数据模型.
//
// 设计：value type + Equatable + Sendable + Identifiable，纯展示数据；mock 值在 FriendsScaffoldDefaults.
// 后续 epic 接 server `/friends` 接口后由 RealFriendsViewModel 内 mapping 写入（API DTO → Friend）.
//
// 字段名对齐 ui_design friends.jsx 内 friends array shape（id / name / online / status / statusText / currentRoomId / color）.

import Foundation
import SwiftUI

public struct Friend: Equatable, Identifiable, Sendable {
    public let id: String                   // userId（后续 epic 后对齐 server user.id）
    public let name: String                 // 好友昵称（如"夏夏"）
    public let online: Bool                 // 是否在线（决定 Avatar 小绿点 + invite/offline 按钮分支）
    public let status: FriendStatus         // 三态分类（offline / online / inRoom）
    public let statusText: String           // 状态文字（如"在房间 1234567 玩耍中" / "刚刚活跃" / "2 小时前在线"）
    public let currentRoomId: String?       // status == .inRoom 时为目标房间号；其它 nil
    public let color: Color?                // 显式覆写 Avatar 背景色（nil 走 Avatar hash 调色板）

    public init(
        id: String,
        name: String,
        online: Bool,
        status: FriendStatus,
        statusText: String,
        currentRoomId: String? = nil,
        color: Color? = nil
    ) {
        self.id = id
        self.name = name
        self.online = online
        self.status = status
        self.statusText = statusText
        self.currentRoomId = currentRoomId
        self.color = color
    }
}
