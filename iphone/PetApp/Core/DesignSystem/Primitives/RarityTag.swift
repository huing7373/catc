// RarityTag.swift
// Story 37.6: 稀有度 4 档色条，对齐 ui_design README §Wardrobe §稀有度配色:
//   N=灰 #b0b0b0 / R=蓝 #7db3e8 / SR=紫 #c58ae8 / SSR=金红渐变 linear-gradient(90deg,#ffd166,#ef476f).
//
// 配色 hex 不抽到 theme（属 Wardrobe 业务色而非 theme color）.

import SwiftUI

/// Rarity: 装扮道具稀有度 4 档.
public enum Rarity: String, CaseIterable, Identifiable {
    case N
    case R
    case SR
    case SSR

    public var id: String { rawValue }
}

/// RarityTag: 稀有度色条（横条；caller 决定 width/height）.
public struct RarityTag: View {
    private let rarity: Rarity
    private let width: CGFloat
    private let height: CGFloat

    public init(rarity: Rarity, width: CGFloat = 40, height: CGFloat = 4) {
        self.rarity = rarity
        self.width = width
        self.height = height
    }

    public var body: some View {
        Capsule()
            .fill(fillStyle)
            .frame(width: width, height: height)
            .accessibilityIdentifier("rarityTag_\(rarity.rawValue)")
    }

    private var fillStyle: AnyShapeStyle {
        switch rarity {
        case .N:
            return AnyShapeStyle(Color(red: 0xB0 / 255.0, green: 0xB0 / 255.0, blue: 0xB0 / 255.0))
        case .R:
            return AnyShapeStyle(Color(red: 0x7D / 255.0, green: 0xB3 / 255.0, blue: 0xE8 / 255.0))
        case .SR:
            return AnyShapeStyle(Color(red: 0xC5 / 255.0, green: 0x8A / 255.0, blue: 0xE8 / 255.0))
        case .SSR:
            return AnyShapeStyle(LinearGradient(
                colors: [
                    Color(red: 1.00, green: 0.82, blue: 0.40),    // #ffd166
                    Color(red: 0.94, green: 0.28, blue: 0.43),    // #ef476f
                ],
                startPoint: .leading,
                endPoint: .trailing
            ))
        }
    }
}

// MARK: - Preview (AC8: 双主题视觉抽样 + 4 档色条)

#if DEBUG
private struct RarityTagPreview_Sample: View {
    @Environment(\.theme) private var theme

    var body: some View {
        VStack(spacing: 18) {
            ForEach(Rarity.allCases) { rarity in
                HStack(spacing: 12) {
                    Text(rarity.rawValue)
                        .font(theme.typography.cardTitle.font)
                        .foregroundColor(theme.colors.ink)
                        .frame(width: 48, alignment: .leading)
                    RarityTag(rarity: rarity, width: 80, height: 6)
                    RarityTag(rarity: rarity, width: 120, height: 4)
                    Spacer()
                }
            }
        }
        .padding(20)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(theme.colors.pageBg)
    }
}

#Preview("RarityTag — candy") {
    RarityTagPreview_Sample()
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("RarityTag — dark") {
    RarityTagPreview_Sample()
        .environment(\.theme, ThemeName.dark.theme)
}
#endif
