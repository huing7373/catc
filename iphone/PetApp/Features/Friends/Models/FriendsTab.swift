// FriendsTab.swift
// Story 37.10 AC3: 顶部 segmented control 二选一 enum（对齐 ui_design friends.jsx:48 ['online','all']）.
//
// rawValue 严格对齐 ui_design 钦定，让 a11y identifier 拼接 `friendsTab_\(rawValue)` 与 ui_design 直接映射.

import Foundation

public enum FriendsTab: String, CaseIterable, Identifiable, Sendable {
    case online
    case all

    public var id: String { rawValue }

    /// Tab 显示名（ui_design friends.jsx:48 钦定）.
    public var label: String {
        switch self {
        case .online: return "在线"
        case .all:    return "全部"
        }
    }
}
