// ProfileMenuItem.swift
// Story 37.11 AC3: 菜单列表 4 项 enum.
//
// rawValue 与 ui_design profile.jsx:171-175 4 行 mock map 对齐（icon / label / extra）：
//   - achievements: 成就徽章 (icon: trophy, extra: "15/40")
//   - messages: 消息通知 (icon: bell, extra: "3 条未读")
//   - favorites: 喜欢的道具 (icon: heart, extra: "")
//   - settings: 设置 (icon: settings, extra: "")
//
// 关键决策：`extraText` 走 enum computed property 而非 ProfileSummary 字段 ——
// 这些是 mock 占位文案（"15/40" / "3 条未读"），ui_design 钦定值；后续 epic 真接成就 / 消息接口时
// 改派生源（如 vm.unreadMessagesCount 派生自 server）.本期写死 enum 内是为了让单元测试能直接
// 断言渲染文字，**不**预 over-design.

import Foundation

public enum ProfileMenuItem: String, CaseIterable, Identifiable, Sendable {
    case achievements
    case messages
    case favorites
    case settings

    public var id: String { rawValue }

    /// 菜单显示名（ui_design profile.jsx:172-175 钦定）.
    public var label: String {
        switch self {
        case .achievements: return "成就徽章"
        case .messages:     return "消息通知"
        case .favorites:    return "喜欢的道具"
        case .settings:     return "设置"
        }
    }

    /// 菜单 SF Symbol icon 键（走 Icons.symbol(for:) 入口）.
    /// achievements → trophy / messages → bell / favorites → heart / settings → settings.
    public var iconKey: String {
        switch self {
        case .achievements: return "trophy"
        case .messages:     return "bell"
        case .favorites:    return "heart"
        case .settings:     return "settings"
        }
    }

    /// 菜单右侧 extra 文字（profile.jsx:172-175 钦定）；空字符串表示无 extra 显示.
    public var extraText: String {
        switch self {
        case .achievements: return "15/40"
        case .messages:     return "3 条未读"
        case .favorites:    return ""
        case .settings:     return ""
        }
    }
}
