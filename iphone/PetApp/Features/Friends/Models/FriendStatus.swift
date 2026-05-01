// FriendStatus.swift
// Story 37.10 AC3: 三态 enum（对齐 ui_design friends.jsx FriendRow 三分支按钮）.
//
// rawValue 与 ui_design friends.jsx:89 / 100 / 110 三分支 status 字段对齐:
//   - inRoom: 在房间中（按钮"加入"实心 accent 色 + Icons.enter）
//   - online: 在线（按钮"邀请"描边 accent 色，无 icon）
//   - offline: 离线（无按钮，灰字"离线"）

import Foundation

public enum FriendStatus: String, CaseIterable, Identifiable, Sendable {
    case inRoom
    case online
    case offline

    public var id: String { rawValue }
}
