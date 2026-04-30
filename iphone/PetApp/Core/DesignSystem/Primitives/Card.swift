// Card.swift
// Story 37.6: 通用卡片容器，对齐 ui_design primitives.jsx `Card` 函数.
//
// 视觉规则：theme.colors.surface 背景 + theme.shadow.sm 阴影 + theme.colors.border 1pt 描边 +
// theme.radius.cardXl 圆角（默认 24pt；调用方可通过 cornerRadius 参数覆写）.

import SwiftUI

/// Card: 通用卡片容器.
///
/// 调用方式：`Card { Text("hello") }` 或 `Card(cornerRadius: 22, padding: 18) { ... }`.
public struct Card<Content: View>: View {
    @Environment(\.theme) private var theme

    private let cornerRadius: CGFloat?
    private let padding: CGFloat?
    @ViewBuilder private let content: () -> Content

    public init(
        cornerRadius: CGFloat? = nil,
        padding: CGFloat? = nil,
        @ViewBuilder content: @escaping () -> Content
    ) {
        self.cornerRadius = cornerRadius
        self.padding = padding
        self.content = content
    }

    public var body: some View {
        let resolvedCornerRadius = cornerRadius ?? theme.radius.cardXl
        let resolvedPadding = padding ?? theme.spacing.s16
        // ⚠️ 守护意图：fix-review round 5 / [P2] — `.shadow(...)` 必须 attach 在
        // background 的 RoundedRectangle 上，**不能**挂在最外层 view 链.
        // 错误模式：`content().padding().background(...).overlay(...).shadow(...)` —
        // SwiftUI 的 `.shadow` 会渲染**整棵被修饰子树**的 alpha 蒙版投影，导致 content()
        // 内的 Text / Icon 等 child view 都被一起投影 → 文字/图标边缘有模糊阴影，
        // 与 ui_design `primitives.jsx` `Card` 的 CSS `box-shadow` 语义（仅外壳投影）不符.
        // 正确模式：把 `.shadow` 直接 chain 到 `RoundedRectangle.fill(...)` 那一层，
        // 这样投影只渲染 shape 本身的 alpha，不波及 children.
        return content()
            .padding(resolvedPadding)
            .background(
                RoundedRectangle(cornerRadius: resolvedCornerRadius)
                    .fill(theme.colors.surface)
                    .shadow(
                        color: theme.shadow.sm.color,
                        radius: theme.shadow.sm.radius,
                        x: theme.shadow.sm.x,
                        y: theme.shadow.sm.y
                    )
            )
            .overlay(
                RoundedRectangle(cornerRadius: resolvedCornerRadius)
                    .stroke(theme.colors.border, lineWidth: 1)
            )
    }
}

// MARK: - Preview (AC8: 双主题视觉抽样)

#if DEBUG
private struct CardPreview_Sample: View {
    @Environment(\.theme) private var theme

    var body: some View {
        VStack(spacing: 20) {
            Card {
                VStack(alignment: .leading, spacing: 8) {
                    Text("默认卡片")
                        .font(theme.typography.cardTitle.font)
                        .foregroundColor(theme.colors.ink)
                    Text("cornerRadius: 24 / padding: 16 / shadow.sm")
                        .font(theme.typography.body.font)
                        .foregroundColor(theme.colors.inkSoft)
                }
            }
            Card(cornerRadius: 22, padding: 18) {
                Text("自定义圆角 + padding (22 / 18)")
                    .font(theme.typography.body.font)
                    .foregroundColor(theme.colors.ink)
            }
        }
        .padding(20)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(theme.colors.pageBg)
    }
}

#Preview("Card — candy") {
    CardPreview_Sample()
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("Card — dark") {
    CardPreview_Sample()
        .environment(\.theme, ThemeName.dark.theme)
}
#endif
