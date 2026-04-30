// ThemeShadow.swift
// Story 37.5 AC5: 阴影 token. 3 档语义阴影 + 按钮硬阴影约定（按钮硬阴影留给 PrimaryButton 落地）.

import SwiftUI

/// ShadowToken: 单个阴影 4 参数容器（color + radius + x + y）.
///
/// **类型选择**: helper struct 取代 SwiftUI 多参数 → 让 caller 通过
///   `.shadow(color: t.color, radius: t.radius, x: t.x, y: t.y)` 一次取齐.
public struct ShadowToken: Equatable {
    public let color: Color
    public let radius: CGFloat
    public let x: CGFloat
    public let y: CGFloat

    public init(color: Color, radius: CGFloat, x: CGFloat = 0, y: CGFloat = 0) {
        self.color = color
        self.radius = radius
        self.x = x
        self.y = y
    }
}

/// ThemeShadow: 阴影 token.
///
/// **3 档语义阴影** (ui_design/README §Shadows):
///   - sm: 0 2px 0 rgba(180,100,120,0.08)  → 卡片
///   - md: 0 6px 16px rgba(180,100,120,0.14) → Tab Bar / 主要卡片
///   - lg: 0 14px 38px rgba(180,100,120,0.18) → Modal
///
/// **按钮硬阴影** (ui_design "立体感硬阴影 0 4px 0 var(--accent-deep)"):
///   - 不在本 struct 内—— 按钮硬阴影与 accent-deep color 强绑定，应在 PrimaryButton (Story 37.6)
///     内部用 `theme.colors.accentDeep` + offset 4 直接组合，不抽象为独立 token.
public struct ThemeShadow: Equatable {
    public let sm: ShadowToken
    public let md: ShadowToken
    public let lg: ShadowToken

    public init(sm: ShadowToken, md: ShadowToken, lg: ShadowToken) {
        self.sm = sm
        self.md = md
        self.lg = lg
    }
}

extension ThemeShadow {
    /// candy: shadow 色基 rgba(180,100,120,X).
    public static let candy = ThemeShadow(
        sm: ShadowToken(
            // 0 2px 0 rgba(180,100,120,0.08) → SwiftUI shadow radius=0 → 硬边阴影
            color: Color(red: 180.0 / 255, green: 100.0 / 255, blue: 120.0 / 255).opacity(0.08),
            radius: 0,
            x: 0,
            y: 2
        ),
        md: ShadowToken(
            // 0 6px 16px rgba(180,100,120,0.14)
            color: Color(red: 180.0 / 255, green: 100.0 / 255, blue: 120.0 / 255).opacity(0.14),
            radius: 16,
            x: 0,
            y: 6
        ),
        lg: ShadowToken(
            // 0 14px 38px rgba(180,100,120,0.18)
            color: Color(red: 180.0 / 255, green: 100.0 / 255, blue: 120.0 / 255).opacity(0.18),
            radius: 38,
            x: 0,
            y: 14
        )
    )
}
