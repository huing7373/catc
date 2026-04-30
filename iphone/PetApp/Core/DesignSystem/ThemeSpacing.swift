// ThemeSpacing.swift
// Story 37.5 AC3: 间距 token. 9 档值对齐 ui_design/README §Spacing.

import SwiftUI

/// ThemeSpacing: 间距 token. 9 档值对齐 ui_design/README §Spacing.
///
/// 命名: 用 t-shirt size + 数值后缀避免歧义（s / m / l 等命名在 9 档下不够清晰）.
/// 字段值即 SwiftUI CGFloat point 值.
public struct ThemeSpacing: Equatable {
    public let s8: CGFloat    // 8
    public let s10: CGFloat   // 10
    public let s12: CGFloat   // 12
    public let s14: CGFloat   // 14
    public let s16: CGFloat   // 16
    public let s18: CGFloat   // 18
    public let s20: CGFloat   // 20
    public let s22: CGFloat   // 22
    public let s28: CGFloat   // 28

    public init(
        s8: CGFloat = 8,
        s10: CGFloat = 10,
        s12: CGFloat = 12,
        s14: CGFloat = 14,
        s16: CGFloat = 16,
        s18: CGFloat = 18,
        s20: CGFloat = 20,
        s22: CGFloat = 22,
        s28: CGFloat = 28
    ) {
        self.s8 = s8
        self.s10 = s10
        self.s12 = s12
        self.s14 = s14
        self.s16 = s16
        self.s18 = s18
        self.s20 = s20
        self.s22 = s22
        self.s28 = s28
    }
}

extension ThemeSpacing {
    /// standard: 9 档 8/10/12/14/16/18/20/22/28（ui_design 钦定）.
    /// 全部主题共用一份 spacing scale—— 主题切换不改间距（与 ui_design 设计一致）.
    public static let standard = ThemeSpacing()
}
