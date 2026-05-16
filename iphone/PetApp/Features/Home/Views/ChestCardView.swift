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
// **状态派生权威：status-aware 双轴判定**（review r2 P2 修订；推翻 r1 over-correction）:
//   - r0 方案 `(status == .unlockable) || (remainingSeconds <= 0)` hydrate 阶段错把默认 0 当超时.
//   - r1 收敛为纯 status `status == .unlockable` —— 修了 hydrate 闪烁，但**反弹了 epic 钦定的
//     "本地倒计时归零自动切 unlockable 视觉态"行为**：driver tick 把 chestRemainingSeconds 减到 0
//     后,view 会一直渲染 counting 卡片直到 server 下一次 status push (Story 21.2 落地的 60s 轮询).
//     违反 docs/宠物互动App_MVP节点规划与里程碑.md epic 21 AC "倒计时归零时自动切到 unlockable 视觉状态".
//   - r2 正解：**status-aware 双轴**判定 ——
//       isUnlockable = (status == .unlockable) OR (status == .counting AND remainingSeconds <= 0)
//     语义：要么 server 权威态已经是 unlockable，要么 server 仍是 counting 但本地倒计时已 tick 到 0
//     (乐观切；等 21.2 60s 轮询 / WS push 把 status 改为 unlockable 兜底).
//     和 r1 的本质区别：r1 view 派生只看 status，hydrate 帧 chestRemainingSeconds=0 不再误判
//     unlockable —— 这点 r2 也保住，因为 r2 配套 ChestTimerDriver.start() 同步初始化让
//     chestRemainingSeconds 在 start() 返回前就拿到 server 推下来的真实 remainingSeconds（如 300）,
//     不会停在默认 0. 真正的"业务 0"只发生在 driver tick 之后，那时候切 unlockable 是 epic 钦定行为.
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
    private let isOpening: Bool
    private let isSyncingSteps: Bool
    private let onOpenTap: () -> Void

    /// Story 21.3 AC7: init 加 `isOpening: Bool = false` 默认参数（让既有 callsite 不变；Preview / 老
    /// 单测 / Story 21.1 ChestCardViewTests 等不动；生产 callsite HomeContainerView 改造显式传值）.
    /// Story 21.5 AC2: 同模式加 `isSyncingSteps: Bool = false` 默认参数 —— 既有 callsite / Preview /
    /// countingView 路径不传走默认 false（零破坏）；生产 callsite HomeContainerView 显式传
    /// `state.isSyncingSteps`（与 `state.isOpening` 对称）.
    public init(
        currentChest: HomeChest?,
        remainingSeconds: Int,
        isOpening: Bool = false,
        isSyncingSteps: Bool = false,
        onOpenTap: @escaping () -> Void
    ) {
        self.currentChest = currentChest
        self.remainingSeconds = remainingSeconds
        self.isOpening = isOpening
        self.isSyncingSteps = isSyncingSteps
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
        // 视觉派生：status-aware 双轴判定（review r2 P2 修订）.
        // - status == .unlockable → 直接 unlockable（server 权威）.
        // - status == .counting 且本地 driver tick 把 remainingSeconds 减到 0 → 乐观切 unlockable
        //   (epic 钦定"倒计时归零自动切视觉态"；等 Story 21.2 60s 轮询 / WS push 把 server status 兜底).
        // - hydrate 帧 chestRemainingSeconds=0 默认值不会误判 unlockable，因为 ChestTimerDriver.start()
        //   同步初始化让 start() 返回前 chestRemainingSeconds 已被写成 server 推下来的真实初值（如 300）.
        let isUnlockable = ChestCardView.isUnlockableForTesting(
            status: chest.status,
            remainingSeconds: remainingSeconds
        )
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
                    // Story 21.3 AC7 → Story 21.5 AC2：副标题三态优先级
                    // isSyncingSteps → "同步步数中…" > isOpening → "开箱中…" > "可开启".
                    // 抽 computed `subtitleText` 让三态可读 + 防嵌套三元（与既有 isUnlockable helper 同精神）.
                    Text(subtitleText)
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundColor(theme.colors.inkSoft)
                }
                Spacer()
                // Story 21.3 AC7：ZStack 包 PrimaryButton + isOpening 时叠 ProgressView 在按钮右侧;
                // 按钮文字保持 "开宝箱" 但 .disabled(isOpening) + .opacity(0.5) 视觉降透明.
                ZStack(alignment: .trailing) {
                    PrimaryButton(title: "开宝箱", variant: .primary, action: onOpenTap)
                        .frame(width: 96)
                        .disabled(isOpening)
                        .opacity(isOpening ? 0.5 : 1.0)
                        .accessibilityIdentifier(AccessibilityID.Home.chestOpenButton)
                    if isOpening {
                        ProgressView()
                            .scaleEffect(0.7)
                            .padding(.trailing, 8)
                            .allowsHitTesting(false)  // hit-test 仍走 PrimaryButton（已 disabled）；保持 a11y tree 干净
                    }
                }
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

    /// Story 21.5 AC2: unlockableView 副标题三态派生（isSyncingSteps > isOpening > idle）.
    /// instance computed 直接读 self.isSyncingSteps / self.isOpening；底层逻辑委托给
    /// `subtitleTextForTesting` static helper 让 ChestCardViewTests 可不经 SwiftUI body 直接断言.
    private var subtitleText: String {
        ChestCardView.subtitleTextForTesting(isSyncingSteps: isSyncingSteps, isOpening: isOpening)
    }

    // MARK: - Testing helpers (internal visibility 让单测可直接断言；与 Story 8.4 PetSpriteView helper 同模式)

    /// Story 21.5 AC2: 副标题三态派生纯函数（优先级 isSyncingSteps > isOpening > idle）.
    /// - 不变量：
    ///     isSyncingSteps == true                       → "同步步数中…"（sync 在 open 之前，时序优先）
    ///     isSyncingSteps == false && isOpening == true  → "开箱中…"
    ///     两者皆 false                                  → "可开启"
    /// - 暴露 internal static 让 ChestCardViewTests 直接断言（与 isUnlockableForTesting 同模式）.
    internal static func subtitleTextForTesting(isSyncingSteps: Bool, isOpening: Bool) -> String {
        if isSyncingSteps { return "同步步数中…" }
        if isOpening { return "开箱中…" }
        return "可开启"
    }

    /// 把秒数格式化为 mm:ss 字符串；负值钳到 0；秒数 ≥ 60 时分母进位.
    /// - 暴露 internal static 让 ChestCardViewTests 直接断言 formatter 行为（不通过 SwiftUI body 内省）.
    internal static func formatMMSSForTesting(_ totalSeconds: Int) -> String {
        let safe = max(0, totalSeconds)
        let minutes = safe / 60
        let seconds = safe % 60
        return String(format: "%02d:%02d", minutes, seconds)
    }

    /// 视觉派生条件：status-aware 双轴判定（review r2 P2 修订；推翻 r1 over-correction）.
    /// - 不变量：
    ///     status == .unlockable                                   → true (server 权威)
    ///     status == .counting && remainingSeconds <= 0            → true (本地 tick 归零乐观切)
    ///     status == .counting && remainingSeconds > 0             → false (倒计时进行中)
    /// - hydrate 阶段 chestRemainingSeconds=0 默认值**不会**误判 unlockable，**前提**是
    ///   ChestTimerDriver.start() 已同步初始化（review r2 配套修复）：start() 同步用当前
    ///   appState.currentChest 跑一次 handleChestChange → recomputeAndWrite，让
    ///   chestRemainingSeconds 在 driver.start() 返回前就拿到 server 推下来的真实初值（如 300）,
    ///   不会停在 @Published Int = 0 默认值.
    /// - 真正的"业务 0"仅发生在 driver tick 之后（unlockAt ≤ now），此时切 unlockable 是
    ///   docs/宠物互动App_MVP节点规划与里程碑.md epic 21 AC "倒计时归零自动切视觉态" 钦定行为.
    /// - 暴露 internal static 让 ChestCardViewTests 直接断言派生函数（不通过 SwiftUI body 内省）.
    internal static func isUnlockableForTesting(
        status: HomeChestStatus,
        remainingSeconds: Int
    ) -> Bool {
        if status == .unlockable {
            return true
        }
        // status == .counting：仅当本地倒计时已到 0 时乐观切 unlockable.
        return status == .counting && remainingSeconds <= 0
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

#Preview("ChestCardView — syncing steps · candy") {
    // Story 21.5 AC2: isSyncingSteps=true 抽样 —— 副标题应显 "同步步数中…"（优先于 isOpening）.
    ChestCardView(
        currentChest: HomeChest(
            id: "c1",
            status: .unlockable,
            unlockAt: Date(),
            openCostSteps: 1000,
            remainingSeconds: 0
        ),
        remainingSeconds: 0,
        isOpening: true,
        isSyncingSteps: true,
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
