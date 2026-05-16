// RewardPopupView.swift
// Story 21.4 AC2: 开箱奖励弹窗纯展示组件（不持 ViewModel；与 JoinRoomModal 同精神）.
//
// 视觉锚（与 ChestCardView Card / JoinRoomModal 风格对齐 + spec AC 行 3111-3115）:
//   1. AsyncImage 加载 reward.iconUrl (cosmetic 图标) → 居中 96pt × 96pt
//      - phase.empty / phase.failure → 灰色 SF Symbol "questionmark.app.dashed" 占位 (96pt)
//      - 与 EmojiPanelView AsyncImage 同模式 (lesson EmojiPanelView line 77-90)
//   2. Text("获得 \(reward.name)") + 18pt heavy + theme.colors.ink (居中)
//   3. RarityTag(rarity: RewardRarityTagMapper.map(reward.rarity), width: 80, height: 6)
//      - 居中放在 name 下方; AC1 helper 派生
//   4. "确定" PrimaryButton(variant: .primary, fullWidth: true) → 调 onClose closure
//      - 关闭由 caller (HomeView) onClose 闭包内置 $pendingReward.wrappedValue = nil
//
// 包装容器:
//   - VStack(spacing: 18) 内含上述 4 元素
//   - .padding(24) + .background(theme.colors.surface) + .clipShape(RoundedRectangle(cornerRadius: 28))
//   - 整 view 挂 FadeInModifier (id = reward.id 让同 sheet 多次弹出仍触发动画重放;
//     spec AC 行 3115 "含淡入动画" 钦定; 配合 .sheet 默认 slide-up 动画形成 "滑入 + 淡入" 组合).
//
// **不**持 ViewModel —— 仅接受 reward 值 + onClose closure (与 JoinRoomModal pattern 一致).

import Foundation
import SwiftUI

public struct RewardPopupView: View {
    public let reward: ChestRewardSnapshot
    public let onClose: () -> Void

    /// Story 37.5: 主题 token 取值入口；caller 注入 `.environment(\.theme, currentTheme.theme)`.
    @Environment(\.theme) private var theme

    public init(reward: ChestRewardSnapshot, onClose: @escaping () -> Void) {
        self.reward = reward
        self.onClose = onClose
    }

    public var body: some View {
        VStack(spacing: 18) {
            // 视觉锚 1: AsyncImage 96pt × 96pt
            iconView
            // 视觉锚 2: "获得 {name}"
            nameLabel
            // 视觉锚 3: RarityTag
            rarityBadge
            // 视觉锚 4: "确定" PrimaryButton
            confirmButton
        }
        .padding(24)
        .background(theme.colors.surface)
        .clipShape(RoundedRectangle(cornerRadius: 28))
        .fadeIn(id: AnyHashable(reward.id))
        .accessibilityIdentifier(AccessibilityID.RewardPopup.popup)
        .accessibilityElement(children: .contain)
    }

    // MARK: - 视觉锚 1: iconView (AsyncImage 96 × 96)

    @ViewBuilder
    private var iconView: some View {
        AsyncImage(url: URL(string: reward.iconUrl)) { phase in
            switch phase {
            case .empty:
                ProgressView()
                    .frame(width: 96, height: 96)
            case .success(let image):
                image
                    .resizable()
                    .aspectRatio(contentMode: .fit)
                    .frame(width: 96, height: 96)
            case .failure:
                Image(systemName: "questionmark.app.dashed")
                    .font(.system(size: 56, weight: .light))
                    .foregroundColor(theme.colors.inkSoft)
                    .frame(width: 96, height: 96)
            @unknown default:
                Image(systemName: "questionmark.app.dashed")
                    .font(.system(size: 56, weight: .light))
                    .foregroundColor(theme.colors.inkSoft)
                    .frame(width: 96, height: 96)
            }
        }
        .accessibilityIdentifier(AccessibilityID.RewardPopup.icon)
    }

    // MARK: - 视觉锚 2: nameLabel

    private var nameLabel: some View {
        Text("获得 \(reward.name)")
            .font(.system(size: 18, weight: .heavy))
            .foregroundColor(theme.colors.ink)
            .multilineTextAlignment(.center)
            .accessibilityIdentifier(AccessibilityID.RewardPopup.nameLabel)
    }

    // MARK: - 视觉锚 3: rarityBadge

    private var rarityBadge: some View {
        RarityTag(
            rarity: RewardRarityTagMapper.map(reward.rarity),
            width: 80,
            height: 6
        )
        // 注：RarityTag 内部已挂 `rarityTag_\(rarity.rawValue)` identifier (RarityTag.swift line 35);
        // 此处再挂 RewardPopup.rarityTag 作为外层定位锚（UITest 用 popup 内层级查 RarityTag 时更稳定）.
        .accessibilityIdentifier(AccessibilityID.RewardPopup.rarityTag)
    }

    // MARK: - 视觉锚 4: confirmButton

    private var confirmButton: some View {
        PrimaryButton(
            title: "确定",
            variant: .primary,
            fullWidth: true,
            action: onClose
        )
        .accessibilityIdentifier(AccessibilityID.RewardPopup.confirmButton)
    }
}

// MARK: - Preview (双主题 + 4 品质抽样)

#if DEBUG
private struct RewardPopupPreview_Sample: View {
    @Environment(\.theme) private var theme

    var body: some View {
        VStack(spacing: 20) {
            RewardPopupView(
                reward: ChestRewardSnapshot(
                    cosmeticItemId: "1001",
                    name: "星星围巾",
                    slot: 2,
                    rarity: .rare,
                    assetUrl: "https://placehold.co/96x96?text=Scarf",
                    iconUrl: "https://placehold.co/96x96?text=Scarf"
                ),
                onClose: {}
            )
            RewardPopupView(
                reward: ChestRewardSnapshot(
                    cosmeticItemId: "1002",
                    name: "神秘王冠",
                    slot: 1,
                    rarity: .legendary,
                    assetUrl: "https://placehold.co/96x96?text=Crown",
                    iconUrl: "https://placehold.co/96x96?text=Crown"
                ),
                onClose: {}
            )
        }
        .padding(16)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(theme.colors.pageBg)
    }
}

#Preview("RewardPopup — candy") {
    RewardPopupPreview_Sample()
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("RewardPopup — dark") {
    RewardPopupPreview_Sample()
        .environment(\.theme, ThemeName.dark.theme)
}
#endif
