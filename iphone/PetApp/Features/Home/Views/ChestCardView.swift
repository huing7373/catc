// ChestCardView.swift
// Story 21.1 AC3: 首页宝箱组件（counting 倒计时 / unlockable 可开启 双态视觉，按 currentChest.status 派生）.
//
// 设计：
//   - 纯 value-type props 输入：currentChest: HomeChest? + remainingSeconds: Int + onOpenTap: () -> Void.
//   - 不持有 ObservableObject（与 Story 8.4 落地的 PetSpriteView 同模式：视图组装层契约锁定，
//     业务态由 caller 派生后传入；本 view 仅按 props 渲染）.
//   - currentChest == nil → 渲染 EmptyView()（防 hydrate 前白屏抖动；与 Story 5.5 loading 三态语义一致）.
//   - currentChest.status == .counting → 灰色锁定图标 + mm:ss 倒计时 + "倒计时" 标签.
//   - currentChest.status == .unlockable → 金色高亮图标 + "可开启" 标签 + 开箱按钮.
//
// **状态派生权威：status 字段是 source of truth**（review r1 P2 修订）:
//   - 原方案 `(status == .unlockable) || (remainingSeconds <= 0)` 把"倒计时未初始化（默认 0）"和
//     "倒计时刚到 0（已超时）"两种语义混在一起 —— hydrate 阶段 `HomeViewModel.chestRemainingSeconds`
//     默认 0，`ChestTimerDriver` sink 还没处理 currentChest 之前，`.counting` 宝箱会被错误地渲染成
//     unlockable 态（金色 + 开箱按钮闪一帧），违反 server 权威态. 现在视觉只看 `status` 一个字段;
//     server WS / 60s 轮询推送 status 切换才触发 unlockable 视觉. 倒计时数值仅供 counting 态显示.
//
// 视觉规则（无现成 ui_design 钦定 chest 区块；本 story 按 home.jsx 同风格自洽落地）：
//   - 容器：`Card(cornerRadius: theme.radius.cardLg, padding: 20)` (22pt 圆角；与 teamIdleCard 同档)；
//     theme.colors.surface 背景 + theme.shadow.md 中阴影 + 1pt theme.colors.border.
//   - counting 态背景：theme.colors.surface（中性）；unlockable 态背景：theme.colors.accentSoft（高亮）.
//   - 图标：SF Symbol "shippingbox.fill"（与 Icons.mapping["box"] 一致），48pt size；counting 态 inkSoft；
//     unlockable 态 coin 色（金色；ThemeColors 已有 coin token）.
//   - 倒计时文字：theme.typography.cardTitle（17pt heavy）；counting 态 ink 色.
//   - "可开启" 标签：theme.typography.cardTitle 17pt heavy，coin 色；下方 PrimaryButton "开宝箱" variant: .primary.

import SwiftUI

public struct ChestCardView: View {
    @Environment(\.theme) private var theme

    private let currentChest: HomeChest?
    private let remainingSeconds: Int
    private let onOpenTap: () -> Void

    public init(
        currentChest: HomeChest?,
        remainingSeconds: Int,
        onOpenTap: @escaping () -> Void
    ) {
        self.currentChest = currentChest
        self.remainingSeconds = remainingSeconds
        self.onOpenTap = onOpenTap
    }

    public var body: some View {
        // currentChest 为 nil（hydrate 前）→ 不渲染（防白屏抖动 + 让 5 区块布局自洽）.
        if let chest = currentChest {
            content(for: chest)
        } else {
            EmptyView()
        }
    }

    @ViewBuilder
    private func content(for chest: HomeChest) -> some View {
        // 视觉派生：纯 status 判定（review r1 P2 修订）.
        // 不再用 `remainingSeconds <= 0` 短路 —— 该值在 hydrate 阶段默认 0，会让刚 hydrate 的 .counting
        // 宝箱被误判 unlockable 一帧. status 由 server 权威态推送（WS / 60s 轮询）切换；倒计时数值仅供
        // counting 态展示 mm:ss，**不**参与视觉态决策.
        let isUnlockable = ChestCardView.isUnlockableForTesting(status: chest.status)
        if isUnlockable {
            unlockableView
        } else {
            countingView(remainingSeconds: remainingSeconds)
        }
    }

    private var unlockableView: some View {
        Card(cornerRadius: theme.radius.cardLg, padding: 20) {
            HStack(spacing: 14) {
                Image(systemName: Icons.symbol(for: "box"))
                    .font(.system(size: 36, weight: .heavy))
                    .foregroundColor(theme.colors.coin)
                VStack(alignment: .leading, spacing: 6) {
                    Text("宝箱已就绪")
                        .font(.system(size: 17, weight: .heavy))
                        .foregroundColor(theme.colors.ink)
                    Text("可开启")
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundColor(theme.colors.inkSoft)
                }
                Spacer()
                PrimaryButton(title: "开宝箱", variant: .primary, action: onOpenTap)
                    .frame(width: 96)
                    .accessibilityIdentifier(AccessibilityID.Home.chestOpenButton)
            }
        }
        .accessibilityIdentifier("chestCard_unlockable")
        .accessibilityElement(children: .contain)
    }

    private func countingView(remainingSeconds: Int) -> some View {
        Card(cornerRadius: theme.radius.cardLg, padding: 20) {
            HStack(spacing: 14) {
                Image(systemName: Icons.symbol(for: "box"))
                    .font(.system(size: 36, weight: .heavy))
                    .foregroundColor(theme.colors.inkSoft)
                VStack(alignment: .leading, spacing: 6) {
                    Text("宝箱倒计时")
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundColor(theme.colors.inkSoft)
                    Text(ChestCardView.formatMMSSForTesting(remainingSeconds))
                        .font(.system(size: 22, weight: .heavy, design: .rounded))
                        .foregroundColor(theme.colors.ink)
                        .accessibilityIdentifier(AccessibilityID.Home.chestRemaining)
                }
                Spacer()
            }
        }
        .accessibilityIdentifier("chestCard_counting")
        .accessibilityElement(children: .contain)
    }

    // MARK: - Testing helpers (internal visibility 让单测可直接断言；与 Story 8.4 PetSpriteView helper 同模式)

    /// 把秒数格式化为 mm:ss 字符串；负值钳到 0；秒数 ≥ 60 时分母进位.
    /// - 暴露 internal static 让 ChestCardViewTests 直接断言 formatter 行为（不通过 SwiftUI body 内省）.
    internal static func formatMMSSForTesting(_ totalSeconds: Int) -> String {
        let safe = max(0, totalSeconds)
        let minutes = safe / 60
        let seconds = safe % 60
        return String(format: "%02d:%02d", minutes, seconds)
    }

    /// 视觉派生条件：纯 status 判定（review r1 P2 修订）.
    /// - 不变量：当且仅当 `status == .unlockable` 时为 unlockable 态.
    /// - 不再纳入 `remainingSeconds <= 0` 短路 —— 该值默认 0 与"超时 0"语义无法区分，会让 hydrate 阶段
    ///   的 .counting 宝箱被误判 unlockable. server WS / 60s 轮询权威推送 status 切换即可.
    /// - 暴露 internal static 让 ChestCardViewTests 直接断言派生函数（不通过 SwiftUI body 内省）.
    internal static func isUnlockableForTesting(status: HomeChestStatus) -> Bool {
        return status == .unlockable
    }
}

#if DEBUG
#Preview("ChestCardView — counting · candy") {
    ChestCardView(
        currentChest: HomeChest(
            id: "c1",
            status: .counting,
            unlockAt: Date().addingTimeInterval(300),
            openCostSteps: 1000,
            remainingSeconds: 300
        ),
        remainingSeconds: 300,
        onOpenTap: {}
    )
    .padding()
    .environment(\.theme, ThemeName.candy.theme)
}

#Preview("ChestCardView — unlockable · candy") {
    ChestCardView(
        currentChest: HomeChest(
            id: "c1",
            status: .unlockable,
            unlockAt: Date(),
            openCostSteps: 1000,
            remainingSeconds: 0
        ),
        remainingSeconds: 0,
        onOpenTap: {}
    )
    .padding()
    .environment(\.theme, ThemeName.candy.theme)
}

#Preview("ChestCardView — counting · dark") {
    ChestCardView(
        currentChest: HomeChest(
            id: "c1",
            status: .counting,
            unlockAt: Date().addingTimeInterval(300),
            openCostSteps: 1000,
            remainingSeconds: 300
        ),
        remainingSeconds: 300,
        onOpenTap: {}
    )
    .padding()
    .environment(\.theme, ThemeName.dark.theme)
}

#Preview("ChestCardView — unlockable · dark") {
    ChestCardView(
        currentChest: HomeChest(
            id: "c1",
            status: .unlockable,
            unlockAt: Date(),
            openCostSteps: 1000,
            remainingSeconds: 0
        ),
        remainingSeconds: 0,
        onOpenTap: {}
    )
    .padding()
    .environment(\.theme, ThemeName.dark.theme)
}
#endif
