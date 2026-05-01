// PrimaryButton.swift
// Story 37.6: 圆药丸主按钮，对齐 ui_design primitives.jsx `PrimaryButton` 函数.
//
// 三 variant: primary / secondary / ghost; 高度 52pt; 圆角走 theme.radius.pill 圆药丸; 硬阴影
// 立体感（按下 translateY(2)）;支持 disabled 态.

import SwiftUI

/// PrimaryButton variant: 三档样式（来自 ui_design primitives.jsx `PrimaryButton` 函数）.
public enum PrimaryButtonVariant: String, CaseIterable {
    case primary
    case secondary
    case ghost
}

/// PrimaryButton: 圆药丸主按钮.
public struct PrimaryButton: View {
    @Environment(\.theme) private var theme

    private let title: String
    private let variant: PrimaryButtonVariant
    private let icon: String?       // SF Symbol 名（来自 Icons.symbol(for:)）；nil 时无 icon
    private let fullWidth: Bool
    private let isEnabled: Bool
    /// Story 37.12 AC1：disabled 入口（与 isEnabled 互斥；任一为 disabled 即按钮 disabled）.
    /// 默认 false 兼容老 caller；JoinRoomModal `confirm button` 走 `isDisabled: trimmedIsEmpty` 入口.
    /// 设计：内部 `effectiveEnabled = isEnabled && !isDisabled` 统一驱动 .disabled / opacity.
    private let isDisabled: Bool
    private let action: () -> Void

    public init(
        title: String,
        variant: PrimaryButtonVariant = .primary,
        icon: String? = nil,
        fullWidth: Bool = false,
        isEnabled: Bool = true,
        isDisabled: Bool = false,
        action: @escaping () -> Void
    ) {
        self.title = title
        self.variant = variant
        self.icon = icon
        self.fullWidth = fullWidth
        self.isEnabled = isEnabled
        self.isDisabled = isDisabled
        self.action = action
    }

    /// 综合 enabled 判定：isEnabled && !isDisabled.
    private var effectiveEnabled: Bool {
        isEnabled && !isDisabled
    }

    public var body: some View {
        Button(action: action) {
            HStack(spacing: theme.spacing.s8) {
                if let icon {
                    Image(systemName: icon)
                }
                Text(title)
            }
            .font(theme.typography.mediumTitle.font)
            .foregroundColor(foregroundColor)
            .frame(height: 52)
            // ⚠️ Modifier order matters: padding 必须在 .frame(maxWidth:) **之前**.
            // 对齐 ui_design primitives.jsx:151-158 — CSS 里 `width: 100%` 默认 box-sizing
            // 等价于 padding 计入 width 内；SwiftUI 里要还原此语义就是先 padding 再扩展 maxWidth.
            // 反过来（先 .frame(maxWidth: .infinity) 再 padding）会让 padded pill 比父容器多
            // 44pt（22 × 2），在 modal card / list row 等约束布局里溢出或被裁剪.
            .padding(.horizontal, theme.spacing.s22)
            .frame(maxWidth: fullWidth ? .infinity : nil)
            // ⚠️ 守护意图：fix-review round 5 / [P2] — `.shadow(...)` attach 在
            // background 的 RoundedRectangle 上，**不能**挂在最外层 label 链.
            // 错误模式：`.background(RoundedRectangle).overlay(...).shadow(...)` —
            // SwiftUI 的 `.shadow` 会对整棵被修饰子树（含 HStack 内的 Text + Image）
            // 渲染 alpha 投影 → CTA label 文字 + SF Symbol 都带模糊阴影，
            // 与 ui_design `primitives.jsx` `PrimaryButton` 的 `boxShadow` 语义
            // （仅 pill 背景投影）不符.
            // 正确模式：把 `.shadow` chain 到 `RoundedRectangle.fill(...)`，
            // 投影只渲染 pill shape，不波及 label.
            .background(
                RoundedRectangle(cornerRadius: theme.radius.pill)
                    .fill(backgroundColor)
                    .shadow(color: shadowColor, radius: 0, x: 0, y: shadowY)
            )
            .overlay(
                Group {
                    if let borderColor {
                        RoundedRectangle(cornerRadius: theme.radius.pill)
                            .stroke(borderColor, lineWidth: 1.5)
                    }
                }
            )
            .opacity(effectiveEnabled ? 1.0 : 0.5)
        }
        // 用 SwiftUI 钦定的 ButtonStyle / configuration.isPressed 路径处理按下视觉，
        // 而非自定义 simultaneousGesture(DragGesture) + @State isPressed.
        // 关键差异：DragGesture.onEnded 在用户按住按钮再**拖出按钮范围**时不触发，
        // 会导致 isPressed 卡在 true → 按钮 stuck 在 offset(y: 2) → 取消的 tap 看起来卡死.
        // configuration.isPressed 由 SwiftUI 框架管理，自动覆盖 cancellation / drag-out
        // 边界 case，是 SwiftUI 设计的标准路径.
        // 守护：fix-review round 3 / [P2-B].
        .buttonStyle(PressedOffsetButtonStyle())
        .disabled(!effectiveEnabled)
    }

    private var backgroundColor: Color {
        switch variant {
        case .primary:   return theme.colors.accent
        case .secondary: return theme.colors.surface
        case .ghost:     return theme.colors.accentSoft
        }
    }

    private var foregroundColor: Color {
        switch variant {
        case .primary:   return Color.white
        case .secondary: return theme.colors.ink
        case .ghost:     return theme.colors.accentDeep
        }
    }

    private var borderColor: Color? {
        switch variant {
        case .secondary: return theme.colors.border
        default:         return nil
        }
    }

    private var shadowColor: Color {
        switch variant {
        case .primary:   return theme.colors.accentDeep
        case .secondary: return Color.black.opacity(0.08)
        case .ghost:     return Color.black.opacity(0.06)
        }
    }

    private var shadowY: CGFloat {
        switch variant {
        case .ghost: return 3
        default:     return 4
        }
    }
}

// MARK: - PressedOffsetButtonStyle

/// 按下时整体 offset(y: 2) 的 ButtonStyle，用于 PrimaryButton 立体硬阴影按下手感.
/// 走 SwiftUI 钦定的 configuration.isPressed 路径，由框架管理 cancellation / drag-out
/// 等手势取消语义，避免自定义 DragGesture 的 onEnded-only 清空导致的"按下拖出 → 卡在
/// pressed 视觉"bug.
public struct PressedOffsetButtonStyle: ButtonStyle {
    public init() {}

    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .offset(y: configuration.isPressed ? 2 : 0)
            .animation(.easeOut(duration: 0.1), value: configuration.isPressed)
    }
}

// MARK: - Preview (AC8: 双主题视觉抽样 + 4 状态)

#if DEBUG
private struct PrimaryButtonPreview_Sample: View {
    @Environment(\.theme) private var theme

    var body: some View {
        VStack(spacing: 16) {
            PrimaryButton(title: "主按钮 primary", variant: .primary) {}
            PrimaryButton(title: "次要 secondary", variant: .secondary) {}
            PrimaryButton(title: "幽灵 ghost", variant: .ghost) {}
            PrimaryButton(title: "禁用 disabled", variant: .primary, isEnabled: false) {}
            PrimaryButton(
                title: "带图标 enter",
                variant: .primary,
                icon: Icons.symbol(for: "enter"),
                fullWidth: true
            ) {}
        }
        .padding(20)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(theme.colors.pageBg)
    }
}

#Preview("PrimaryButton — candy") {
    PrimaryButtonPreview_Sample()
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("PrimaryButton — dark") {
    PrimaryButtonPreview_Sample()
        .environment(\.theme, ThemeName.dark.theme)
}
#endif
