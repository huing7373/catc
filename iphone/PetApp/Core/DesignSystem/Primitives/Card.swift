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
        return content()
            .padding(resolvedPadding)
            .background(
                RoundedRectangle(cornerRadius: resolvedCornerRadius)
                    .fill(theme.colors.surface)
            )
            .overlay(
                RoundedRectangle(cornerRadius: resolvedCornerRadius)
                    .stroke(theme.colors.border, lineWidth: 1)
            )
            .shadow(
                color: theme.shadow.sm.color,
                radius: theme.shadow.sm.radius,
                x: theme.shadow.sm.x,
                y: theme.shadow.sm.y
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
