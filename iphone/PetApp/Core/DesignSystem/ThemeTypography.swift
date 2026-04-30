// ThemeTypography.swift
// Story 37.5 AC6: 字号 / 字重 token. 按 ui_design/README §Typography 6 档语义命名.

import SwiftUI

/// TypographyToken: 单个字号 / 字重 容器 + 转 SwiftUI Font.
///
/// **字体家族**:
///   ui_design 钦定 SF Pro Rounded + PingFang SC; SwiftUI 通过
///   `.font(.system(size: t.size, weight: t.weight, design: .rounded))` 取
///   SF Pro Rounded; PingFang SC 由 iOS fallback 链自动接管中文字符.
public struct TypographyToken: Equatable {
    public let size: CGFloat
    public let weight: Font.Weight

    public init(size: CGFloat, weight: Font.Weight) {
        self.size = size
        self.weight = weight
    }

    /// 转 SwiftUI Font (rounded design).
    public var font: Font {
        .system(size: size, weight: weight, design: .rounded)
    }
}

/// ThemeTypography: 字号 / 字重 token. 按 ui_design/README §Typography 6 档语义命名.
///
/// **6 档命名映射**:
///   - largeTitle:    22 / 800        → 大标题
///   - mediumTitle:   17-18 / 800     → 中标题（取 17 作为 default）
///   - cardTitle:     14-15 / 800     → 卡片标题（取 14）
///   - body:          13 / 600-700    → 正文（取 13 / 700）
///   - caption:       11-12 / 700     → 辅助文字（取 11 / 700）
///   - microLabel:    9-10 / 800      → 微小标签（取 10）
///
/// **字重映射**:
///   ui_design 的 800 ≈ SwiftUI .heavy (Font.Weight.heavy = ~800)
///   ui_design 的 700 ≈ SwiftUI .bold  (Font.Weight.bold ≈ 700)
///   ui_design 的 600 ≈ SwiftUI .semibold (Font.Weight.semibold ≈ 600)
public struct ThemeTypography: Equatable {
    public let largeTitle: TypographyToken
    public let mediumTitle: TypographyToken
    public let cardTitle: TypographyToken
    public let body: TypographyToken
    public let caption: TypographyToken
    public let microLabel: TypographyToken

    public init(
        largeTitle: TypographyToken = TypographyToken(size: 22, weight: .heavy),
        mediumTitle: TypographyToken = TypographyToken(size: 17, weight: .heavy),
        cardTitle: TypographyToken = TypographyToken(size: 14, weight: .heavy),
        body: TypographyToken = TypographyToken(size: 13, weight: .bold),
        caption: TypographyToken = TypographyToken(size: 11, weight: .bold),
        microLabel: TypographyToken = TypographyToken(size: 10, weight: .heavy)
    ) {
        self.largeTitle = largeTitle
        self.mediumTitle = mediumTitle
        self.cardTitle = cardTitle
        self.body = body
        self.caption = caption
        self.microLabel = microLabel
    }
}

extension ThemeTypography {
    /// standard: 全部主题共用 typography scale.
    public static let standard = ThemeTypography()
}
