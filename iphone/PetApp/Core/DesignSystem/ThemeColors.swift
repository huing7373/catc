// ThemeColors.swift
// Story 37.5 AC2: 13 字段语义 color token. 字段名 1:1 对齐 ui_design/README §Design Tokens.
//
// 说明：本文件含 candy 完整实装 + matcha / sky / dark 三主题 stub（仅 ui_design 显式列出的字段
// 落实值，其余 placeholder 复用 candy 同字段 + `// TODO(Story-Future):` 注释）。

import SwiftUI

/// ThemeColors: 13 个语义 color token. 字段名 1:1 对齐 ui_design/README §Design Tokens.
///
/// **命名转换** (CSS 变量 → Swift property):
///   --page-bg     → pageBg
///   --accent      → accent
///   --accent-soft → accentSoft
///   --accent-deep → accentDeep
///   --surface     → surface
///   --surface-2   → surface2
///   --ink         → ink
///   --ink-soft    → inkSoft
///   --ink-mute    → inkMute
///   --success     → success
///   --warn        → warn
///   --coin        → coin
///   --border      → border
///
/// **dark 主题字段语义反转** (ui_design README §Design Tokens 钦定):
///   "深色模式: page-bg #2a1c22, surface #3a2831, ink #fbe5ec（其余配色自动反转）"
///   本期 stub: 仅显式落地 README 列出的 3 字段；其余字段复用 candy placeholder + TODO 注释.
public struct ThemeColors: Equatable {
    public let pageBg: Color
    public let accent: Color
    public let accentSoft: Color
    public let accentDeep: Color
    public let surface: Color
    public let surface2: Color
    public let ink: Color
    public let inkSoft: Color
    public let inkMute: Color
    public let success: Color
    public let warn: Color
    public let coin: Color
    public let border: Color

    public init(
        pageBg: Color,
        accent: Color,
        accentSoft: Color,
        accentDeep: Color,
        surface: Color,
        surface2: Color,
        ink: Color,
        inkSoft: Color,
        inkMute: Color,
        success: Color,
        warn: Color,
        coin: Color,
        border: Color
    ) {
        self.pageBg = pageBg
        self.accent = accent
        self.accentSoft = accentSoft
        self.accentDeep = accentDeep
        self.surface = surface
        self.surface2 = surface2
        self.ink = ink
        self.inkSoft = inkSoft
        self.inkMute = inkMute
        self.success = success
        self.warn = warn
        self.coin = coin
        self.border = border
    }
}

// MARK: - 静态实例

extension ThemeColors {
    /// candy（糖果粉）—— 完整实装；hex 值 1:1 对齐 ui_design/README §Design Tokens.
    public static let candy = ThemeColors(
        pageBg:     Color(hex: 0xF7E9E0),
        accent:     Color(hex: 0xFF8FA3),
        accentSoft: Color(hex: 0xFFD6DF),
        accentDeep: Color(hex: 0xE15F7C),
        surface:    Color(hex: 0xFFF9F5),
        surface2:   Color(hex: 0xFFF1E8),
        ink:        Color(hex: 0x4A2C36),
        inkSoft:    Color(hex: 0x8B6B75),
        inkMute:    Color(hex: 0xB99BA5),
        success:    Color(hex: 0x7BC47F),
        warn:       Color(hex: 0xFFB26B),
        coin:       Color(hex: 0xFFB84D),
        // border: rgba(74,44,54,0.08) → opacity 0.08 of ink base color.
        border:     Color(red: 74.0 / 255, green: 44.0 / 255, blue: 54.0 / 255).opacity(0.08)
    )

    /// matcha（抹茶）—— stub: accent / accentSoft / accentDeep 来自 ui_design;
    /// 其余字段以 candy 同字段 placeholder + TODO 注释.
    public static let matcha = ThemeColors(
        pageBg:     ThemeColors.candy.pageBg,         // TODO(Story-Future): matcha pageBg 设计待定，当前复用 candy
        accent:     Color(hex: 0x94B97C),             // ui_design 钦定
        accentSoft: Color(hex: 0xDFE8C8),             // ui_design 钦定
        accentDeep: Color(hex: 0x63894A),             // ui_design 钦定
        surface:    ThemeColors.candy.surface,        // TODO(Story-Future): matcha surface 设计待定，当前复用 candy
        surface2:   ThemeColors.candy.surface2,       // TODO(Story-Future): matcha surface2 设计待定，当前复用 candy
        ink:        ThemeColors.candy.ink,            // TODO(Story-Future): matcha ink 设计待定，当前复用 candy
        inkSoft:    ThemeColors.candy.inkSoft,        // TODO(Story-Future): matcha inkSoft 设计待定，当前复用 candy
        inkMute:    ThemeColors.candy.inkMute,        // TODO(Story-Future): matcha inkMute 设计待定，当前复用 candy
        success:    ThemeColors.candy.success,        // TODO(Story-Future): matcha success 设计待定，当前复用 candy
        warn:       ThemeColors.candy.warn,           // TODO(Story-Future): matcha warn 设计待定，当前复用 candy
        coin:       ThemeColors.candy.coin,           // TODO(Story-Future): matcha coin 设计待定，当前复用 candy
        border:     ThemeColors.candy.border          // TODO(Story-Future): matcha border 设计待定，当前复用 candy
    )

    /// sky（天空）—— stub.
    public static let sky = ThemeColors(
        pageBg:     ThemeColors.candy.pageBg,         // TODO(Story-Future): sky pageBg 设计待定，当前复用 candy
        accent:     Color(hex: 0x7BB3E0),             // ui_design 钦定
        accentSoft: Color(hex: 0xCFE2F2),             // ui_design 钦定
        accentDeep: Color(hex: 0x4E86B6),             // ui_design 钦定
        surface:    ThemeColors.candy.surface,        // TODO(Story-Future): sky surface 设计待定，当前复用 candy
        surface2:   ThemeColors.candy.surface2,       // TODO(Story-Future): sky surface2 设计待定，当前复用 candy
        ink:        ThemeColors.candy.ink,            // TODO(Story-Future): sky ink 设计待定，当前复用 candy
        inkSoft:    ThemeColors.candy.inkSoft,        // TODO(Story-Future): sky inkSoft 设计待定，当前复用 candy
        inkMute:    ThemeColors.candy.inkMute,        // TODO(Story-Future): sky inkMute 设计待定，当前复用 candy
        success:    ThemeColors.candy.success,        // TODO(Story-Future): sky success 设计待定，当前复用 candy
        warn:       ThemeColors.candy.warn,           // TODO(Story-Future): sky warn 设计待定，当前复用 candy
        coin:       ThemeColors.candy.coin,           // TODO(Story-Future): sky coin 设计待定，当前复用 candy
        border:     ThemeColors.candy.border          // TODO(Story-Future): sky border 设计待定，当前复用 candy
    )

    /// dark（深色模式）—— stub: pageBg / surface / ink 来自 ui_design 显式表述;
    /// 其余字段以 candy 同字段 placeholder + TODO 注释.
    public static let dark = ThemeColors(
        pageBg:     Color(hex: 0x2A1C22),             // ui_design 钦定
        accent:     ThemeColors.candy.accent,         // TODO(Story-Future): dark accent 设计待定，当前复用 candy
        accentSoft: ThemeColors.candy.accentSoft,     // TODO(Story-Future): dark accentSoft 设计待定，当前复用 candy
        accentDeep: ThemeColors.candy.accentDeep,     // TODO(Story-Future): dark accentDeep 设计待定，当前复用 candy
        surface:    Color(hex: 0x3A2831),             // ui_design 钦定
        surface2:   ThemeColors.candy.surface2,       // TODO(Story-Future): dark surface2 设计待定，当前复用 candy
        ink:        Color(hex: 0xFBE5EC),             // ui_design 钦定
        inkSoft:    ThemeColors.candy.inkSoft,        // TODO(Story-Future): dark inkSoft 设计待定，当前复用 candy
        inkMute:    ThemeColors.candy.inkMute,        // TODO(Story-Future): dark inkMute 设计待定，当前复用 candy
        success:    ThemeColors.candy.success,        // TODO(Story-Future): dark success 设计待定，当前复用 candy
        warn:       ThemeColors.candy.warn,           // TODO(Story-Future): dark warn 设计待定，当前复用 candy
        coin:       ThemeColors.candy.coin,           // TODO(Story-Future): dark coin 设计待定，当前复用 candy
        border:     ThemeColors.candy.border          // TODO(Story-Future): dark border 设计待定，当前复用 candy
    )
}

// MARK: - Color hex 辅助 init

extension Color {
    /// 从 24-bit RGB hex literal 构造 Color（如 0xFF8FA3）.
    /// alpha 默认 1.0；如需 alpha 通过 `.opacity(_:)` 修饰.
    /// 仅供 ThemeColors 内部使用；外部代码应取 token 而非自己 hex.
    init(hex: UInt32, alpha: Double = 1.0) {
        let r = Double((hex >> 16) & 0xFF) / 255.0
        let g = Double((hex >> 8) & 0xFF) / 255.0
        let b = Double(hex & 0xFF) / 255.0
        self.init(.sRGB, red: r, green: g, blue: b, opacity: alpha)
    }
}
