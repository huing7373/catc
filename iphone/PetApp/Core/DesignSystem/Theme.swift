// Theme.swift
// Story 37.5 AC1: 设计 token 顶层入口 + ThemeName enum + 4 个 static let Theme 实例.
//
// 关键设计：Theme 是 value type (struct) + immutable (let 字段) → @Environment 注入零开销，
// 切换主题靠 RootView `@State currentTheme` 改值 + SwiftUI 重渲染子树（不走 ObservableObject 路径）.

import SwiftUI

/// ThemeName: 主题名空间 enum.
///
/// CaseIterable 让未来主题切换 UI（白名单 Story 37.14 后续 mini-epic）能 ForEach.
/// raw value 是字符串以便 a11y identifier / 持久化（如 UserDefaults 存当前主题）.
public enum ThemeName: String, CaseIterable, Identifiable {
    case candy
    case matcha
    case sky
    case dark

    public var id: String { rawValue }

    /// 返回对应的 Theme 静态实例.
    /// 设计：用 switch 分发到 Theme.candy / .matcha / .sky / .dark，让 caller 写
    /// `currentTheme.theme.colors.accent` 风格的链式访问.
    public var theme: Theme {
        switch self {
        case .candy: return .candy
        case .matcha: return .matcha
        case .sky: return .sky
        case .dark: return .dark
        }
    }
}

/// Theme: 设计 token 顶层容器.
///
/// **类型选择**: `struct` + `let` 字段（不可变）→ value type，@Environment 注入零开销，
/// 切换主题靠重建（RootView `@State currentTheme` 改值 → SwiftUI 重渲染子树）.
///
/// **范围**: 5 类 sub-token 容器（colors / spacing / radius / shadow / typography），
/// 严格对齐 `iphone/ui_design/README.md` §Design Tokens.
///
/// **不含**: 主题元数据（name / description）—— 那归 ThemeName enum；
/// 不含 ObservableObject / @Published—— 那是 Theme 切换 UI 的实现细节，本期不做.
public struct Theme: Equatable {
    public let colors: ThemeColors
    public let spacing: ThemeSpacing
    public let radius: ThemeRadius
    public let shadow: ThemeShadow
    public let typography: ThemeTypography

    public init(
        colors: ThemeColors,
        spacing: ThemeSpacing,
        radius: ThemeRadius,
        shadow: ThemeShadow,
        typography: ThemeTypography
    ) {
        self.colors = colors
        self.spacing = spacing
        self.radius = radius
        self.shadow = shadow
        self.typography = typography
    }
}

// MARK: - 静态实例

extension Theme {
    /// candy（糖果粉，默认浅色）—— 完整实装；token 全部对齐 ui_design/README §Design Tokens.
    public static let candy = Theme(
        colors: .candy,
        spacing: .standard,
        radius: .standard,
        shadow: .candy,
        typography: .standard
    )

    /// matcha（抹茶）—— stub: 仅 colors.accent / accent-soft / accent-deep 来自 ui_design;
    /// 其余字段全部复用 candy 同字段 placeholder（每个字段须有 TODO 注释指向后续 mini-epic）.
    public static let matcha = Theme(
        colors: .matcha,
        spacing: .standard,
        radius: .standard,
        shadow: .candy, // TODO(Story-Future): matcha shadow 设计待定，当前复用 candy
        typography: .standard
    )

    /// sky（天空）—— stub.
    public static let sky = Theme(
        colors: .sky,
        spacing: .standard,
        radius: .standard,
        shadow: .candy, // TODO(Story-Future): sky shadow 设计待定，当前复用 candy
        typography: .standard
    )

    /// dark（深色模式）—— stub: 仅 colors.pageBg / surface / ink 来自 ui_design 显式表述;
    /// 其余字段以 candy 字段做 placeholder（每个 placeholder 须有 TODO 注释）.
    public static let dark = Theme(
        colors: .dark,
        spacing: .standard,
        radius: .standard,
        shadow: .candy, // TODO(Story-Future): dark shadow 设计待定，当前复用 candy
        typography: .standard
    )
}

// MARK: - Preview (AC10: 视觉抽样验收)

#if DEBUG
/// 4 主题视觉抽样卡片：用一个简单 sampler 让 dev 在 Xcode Canvas 目视确认色值无误.
/// **不**作为单元测试断言（视觉差异容忍）.
private struct ThemePreview_Sampler: View {
    let theme: Theme
    let label: String

    var body: some View {
        VStack(spacing: 8) {
            Text(label)
                .font(theme.typography.cardTitle.font)
                .foregroundColor(theme.colors.ink)
            HStack(spacing: 8) {
                colorSwatch(theme.colors.accent, "accent")
                colorSwatch(theme.colors.accentSoft, "accentSoft")
                colorSwatch(theme.colors.accentDeep, "accentDeep")
            }
            Text("Surface")
                .font(theme.typography.body.font)
                .foregroundColor(theme.colors.ink)
                .padding(theme.spacing.s14)
                .background(theme.colors.surface)
                .cornerRadius(theme.radius.cardMd)
        }
        .padding(theme.spacing.s16)
        .background(theme.colors.pageBg)
    }

    private func colorSwatch(_ c: Color, _ name: String) -> some View {
        VStack {
            RoundedRectangle(cornerRadius: theme.radius.tagMd).fill(c).frame(width: 48, height: 48)
            Text(name).font(theme.typography.caption.font)
        }
    }
}

#Preview("Theme Sampler — candy") { ThemePreview_Sampler(theme: .candy, label: "candy") }
#Preview("Theme Sampler — matcha") { ThemePreview_Sampler(theme: .matcha, label: "matcha (stub)") }
#Preview("Theme Sampler — sky") { ThemePreview_Sampler(theme: .sky, label: "sky (stub)") }
#Preview("Theme Sampler — dark") { ThemePreview_Sampler(theme: .dark, label: "dark (stub)") }
#endif
