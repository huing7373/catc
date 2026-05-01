// FriendsScaffoldDefaults.swift
// Story 37.10 AC3: Mock 与 Real FriendsViewModel 共享 scaffold 占位数据.
//
// 背景（Story 37.8 round 1 P2 lesson 预防性应用）：
//   抽 shared defaults 而非 hardcode 在两个 ViewModel —— 避免 Mock/Real 重复定义 mock 数据.
//
// 设计决议（与 RoomScaffoldDefaults / WardrobeScaffoldDefaults 同精神）：
//   - friends 三态各 2-3 个共 8 件（epic AC line 4814 钦定）
//   - selectedTab 默认 .online（ui_design friends.jsx:4 useState('online') 钦定）
//   - currentRoomId 默认 nil（启动后用户未进房间）

import Foundation

/// Mock 与 Real FriendsViewModel 启动占位数据（friends state UI scaffold defaults）.
public enum FriendsScaffoldDefaults {
    /// 默认选中 Tab（mock .online —— ui_design friends.jsx:4 useState('online') 钦定）.
    public static let selectedTab: FriendsTab = .online

    /// 默认 currentRoomId（启动占位 nil；RealFriendsViewModel sink 派生覆盖）.
    public static let currentRoomId: String? = nil

    /// 完整 mock friends（8 件，三态混合 inRoom 3 / online 3 / offline 2，epic AC line 4814 钦定 ≥2-3 each）.
    /// 字段值与 ui_design friends.jsx FriendRow 视觉示例匹配（name / status / statusText 风格一致）.
    public static let friends: [Friend] = [
        // inRoom（3）
        Friend(id: "u1", name: "夏夏", online: true, status: .inRoom, statusText: "在房间 1234567 玩耍中", currentRoomId: "1234567"),
        Friend(id: "u2", name: "茉茉", online: true, status: .inRoom, statusText: "在房间 8888888 喂猫", currentRoomId: "8888888"),
        Friend(id: "u3", name: "可乐", online: true, status: .inRoom, statusText: "和小伙伴在房间 7654321", currentRoomId: "7654321"),
        // online（3）
        Friend(id: "u4", name: "豆豆", online: true, status: .online, statusText: "刚刚活跃"),
        Friend(id: "u5", name: "馒头", online: true, status: .online, statusText: "在线 · 想散步"),
        Friend(id: "u6", name: "拿铁", online: true, status: .online, statusText: "在线 · 等队友"),
        // offline（2）
        Friend(id: "u7", name: "饭团", online: false, status: .offline, statusText: "2 小时前在线"),
        Friend(id: "u8", name: "椰奶", online: false, status: .offline, statusText: "昨天活跃"),
    ]
}
