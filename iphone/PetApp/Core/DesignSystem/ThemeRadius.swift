// ThemeRadius.swift
// Story 37.5 AC4: 圆角 token. 按 ui_design/README §Border Radius 5 类语义命名.

import SwiftUI

/// ThemeRadius: 圆角 token. 按 ui_design/README §Border Radius 的 5 类语义命名.
///
/// **命名映射**:
///   - tag (小标签):   6 / 8                 → tagSm = 6, tagMd = 8
///   - control (中等元素):  12 / 14 / 16     → controlSm = 12, controlMd = 14, controlLg = 16
///   - card (卡片): 18 / 20 / 22 / 24       → cardSm = 18, cardMd = 20, cardLg = 22, cardXl = 24
///   - modal (大卡片 / Modal): 26 / 28        → modalSm = 26, modalLg = 28
///   - pill (按钮 / 圆药丸): 高度的一半     → pill = 999（"足够大让 SwiftUI 取最小半径，等价圆药丸"）
///
/// **不含**: 头像 / 圆点 50% —— 那靠 .clipShape(Circle()) 实现，无圆角值.
public struct ThemeRadius: Equatable {
    public let tagSm: CGFloat
    public let tagMd: CGFloat
    public let controlSm: CGFloat
    public let controlMd: CGFloat
    public let controlLg: CGFloat
    public let cardSm: CGFloat
    public let cardMd: CGFloat
    public let cardLg: CGFloat
    public let cardXl: CGFloat
    public let modalSm: CGFloat
    public let modalLg: CGFloat
    public let pill: CGFloat

    public init(
        tagSm: CGFloat = 6,
        tagMd: CGFloat = 8,
        controlSm: CGFloat = 12,
        controlMd: CGFloat = 14,
        controlLg: CGFloat = 16,
        cardSm: CGFloat = 18,
        cardMd: CGFloat = 20,
        cardLg: CGFloat = 22,
        cardXl: CGFloat = 24,
        modalSm: CGFloat = 26,
        modalLg: CGFloat = 28,
        pill: CGFloat = 999
    ) {
        self.tagSm = tagSm
        self.tagMd = tagMd
        self.controlSm = controlSm
        self.controlMd = controlMd
        self.controlLg = controlLg
        self.cardSm = cardSm
        self.cardMd = cardMd
        self.cardLg = cardLg
        self.cardXl = cardXl
        self.modalSm = modalSm
        self.modalLg = modalLg
        self.pill = pill
    }
}

extension ThemeRadius {
    /// standard: 全部主题共用 radius scale.
    public static let standard = ThemeRadius()
}
