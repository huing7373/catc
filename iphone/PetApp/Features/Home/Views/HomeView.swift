// HomeView.swift
// Story 37.7 重写：HomeView 改 generic struct + chestSlot ViewBuilder closure 接缝 + ui_design HomeScreen 5 区块视觉.
//
// 历史（保留 git 演进足迹）：
// - Story 2.2：6 占位区块版（用户昵称 / 猫展示 / 步数 / 宝箱 / 3 CTA / 版本号）.
// - Story 5.2：userInfoBar 接 SessionStore 订阅 nickname.
// - Story 5.5：petColumn 加 pet name + chestColumn 加 chest 倒计时.
// - Story 37.3：删除 bottomButtonRow（3 CTA → 4 Tab IA + JoinRoomModal sheet）.
// - Story 37.4：3 处 viewModel.homeData 改读 appState 投影.
// - Story 37.7（本文件 body 整段重写）：
//   · struct 改 `HomeView<ChestSlot: View>` generic + `chestSlot: () -> ChestSlot` ViewBuilder closure.
//   · 参数名 viewModel → state（v2 提案钦定）；删除老 3 个 init 重载，1 个新 init.
//   · body 重写：ZStack 背景渐变 + ScrollView VStack 5 区块（statusBar / catStage / actionRow / chestSlot() / teamIdleCard）.
//   · 7 个新 a11y identifier inline 字符串（Story 37.13 归并到 AccessibilityID.Home.statusBar 等）.
//   · 保留 AccessibilityID.Home.userInfo / petArea / petName / stepBalance / versionLabel 5 个老常量物理位置变 / 命名继续.
//   · chestArea / chestRemaining 在本期 chestSlot 接缝期不渲染（chestSlot 默认 EmptyView；Story 21.1 改传 ChestCardView）.
//   · 仍保留 SessionAwareUserInfoBar / StaticUserInfoBar 子视图（含 ResetIdentityButton 注入路径）；新 5 区块在 statusBar 内 inline ResetIdentityButton.
//   · HomeNicknameResolver / HomePetNameResolver 不动（纯函数 helper）.
//
// 视觉规则来源：iphone/ui_design/source/screens/home.jsx + iphone/ui_design/README.md §HomeScreen.

import SwiftUI

public struct HomeView<ChestSlot: View>: View {
    @ObservedObject public var state: HomeViewModel

    /// Story 37.4：通过 `.environmentObject(appState)` 由 RootView 注入 → 子视图（HomeView）订阅 AppState.
    /// ADR-0010 §3.1 例外条款：纯展示 SwiftUI View 可直接用 @EnvironmentObject AppState
    /// （ViewModel 层禁用 @EnvironmentObject，但 View 层不属于 ViewModel）.
    @EnvironmentObject var appState: AppState

    /// Story 37.5: 主题 token 取值入口；RootView 注入 `.environment(\.theme, currentTheme.theme)`.
    @Environment(\.theme) private var theme

    // Story 2.8: optional dev "重置身份" 按钮的 ViewModel（仅在 Debug build 由 RootView 注入）。
    private let resetIdentityViewModel: ResetIdentityViewModel?

    /// Story 5.2 codex round 1 [P1] fix：optional SessionStore，nickname 显示来源。
    private let sessionStore: SessionStore?

    /// Story 37.7 AC3: chestSlot ViewBuilder closure 接缝（Story 21.1 改传 ChestCardView()，HomeView 内部 zero edit）.
    /// 本期调用方传 EmptyView() 占位（HomeContainerHomeViewBridge / Preview）；视图位置在 ScrollView VStack 第 4 区块.
    private let chestSlot: () -> ChestSlot

    /// Story 37.7 AC3: 唯一 init —— 删除老 3 个重载，caller 漏改靠编译器报错驱动.
    /// 参数名 `state` 而非 `viewModel`（v2 提案钦定）.
    public init(
        state: HomeViewModel,
        resetIdentityViewModel: ResetIdentityViewModel? = nil,
        sessionStore: SessionStore? = nil,
        @ViewBuilder chestSlot: @escaping () -> ChestSlot
    ) {
        self.state = state
        self.resetIdentityViewModel = resetIdentityViewModel
        self.sessionStore = sessionStore
        self.chestSlot = chestSlot
    }

    public var body: some View {
        ZStack {
            // 背景渐变（home.jsx:18 钦定 linear-gradient(180deg, accent-soft 0%, page-bg 38%)）.
            LinearGradient(
                colors: [theme.colors.accentSoft, theme.colors.pageBg],
                startPoint: .top,
                endPoint: .bottom
            )
            .ignoresSafeArea()

            ScrollView {
                VStack(spacing: theme.spacing.s14) {
                    statusBar
                    catStage
                    actionRow
                    chestSlot()           // 接缝：本期传 EmptyView()，Story 21.1 传 ChestCardView
                    teamIdleCard
                    versionFooter
                }
                .padding(.horizontal, theme.spacing.s20)
                .padding(.top, 68)         // ui_design §iOS 设备规格: 状态栏 padding 68pt
                .padding(.bottom, 100)     // 浮动 TabBar 让出空间
            }
        }
        .sheet(isPresented: $state.showJoinModal) {
            // Story 37.12 落地真实 JoinRoomModal；本期挂 placeholder（已存在 stub）
            JoinRoomModalPlaceholder()
        }
        .onChange(of: state.interactionAnimation) { _, newValue in
            // floatUp 动画完成后自动重置回 idle（1.4s ≈ ui_design home.jsx 钦定 floatUp 时长）.
            // 保 mock / production 行为一致：只有 .flying(...) 触发重置 timer，.idle 不做任何事.
            // iOS 17+ 双参签名：(oldValue, newValue) -> Void（避免单参 deprecation warning）.
            guard case .flying = newValue else { return }
            Task { @MainActor [weak state] in
                try? await Task.sleep(nanoseconds: 1_400_000_000)
                state?.interactionAnimation = .idle
            }
        }
    }

    // MARK: - 区块 1: statusBar (home.jsx:21-38)

    /// 顶部状态栏：左侧 weather + greeting，右侧步数 capsule（含老 ResetIdentityButton 注入入口）.
    /// a11y：父容器挂 `homeStatusBar` 新锚 + `AccessibilityID.Home.userInfo` 老锚（双 identifier 共存）.
    private var statusBar: some View {
        HStack(alignment: .center) {
            // 左：weather + greeting (ui_design 钦定 22pt 800 大标题 + 12pt 600 副标题).
            statusBarLeftBlock

            Spacer()

            // 右：步数 capsule（Card 替代不上，因 Capsule shape）.
            stepBalanceCapsule

            // Debug：重置身份按钮（仅 Debug build；从老 userInfoBar 透传过来）.
            #if DEBUG
            if let resetIdentityViewModel = resetIdentityViewModel {
                ResetIdentityButton(viewModel: resetIdentityViewModel)
            }
            #endif
        }
        .padding(.top, 4)
        // 双 a11y identifier 共存策略：
        //   - 老 `home_userInfo`（Story 2.2 / 5.2 / 2.8 测试用）作为父容器主 identifier + 携带 nickname label
        //     （保 Story 2.8 testUserInfoBarRetainsNicknameAccessibilityLabel：父级 a11y label 必须 = nickname）.
        //   - 新 `homeStatusBar`（Story 37.7 AC8）作为兄弟节点透明 overlay,通过 `.background` 注入,
        //     允许新老两条 UITest 同时定位（XCUITest 通过 descendants 跨子元素递归查 identifier）.
        // .accessibilityElement(children: .contain) 让子元素（stepBalance / btnResetIdentity）仍可独立定位.
        .accessibilityElement(children: .contain)
        .accessibilityLabel(Text(currentNicknameForA11y))
        .accessibilityIdentifier(AccessibilityID.Home.userInfo)
        // 新 homeStatusBar 锚通过透明 overlay 子元素提供（与父级 home_userInfo 并存；
        // SwiftUI 父容器 `.contain` 让子元素 a11y identifier 在 XCUITest descendants 内可定位）.
        .overlay(
            Text("")
                .frame(width: 0, height: 0)
                .accessibilityElement(children: .ignore)
                .accessibilityIdentifier("homeStatusBar")
                .accessibilityHidden(false)
        )
    }

    /// statusBar 左侧 VStack: weather (12pt 600) + greeting (22pt 800).
    private var statusBarLeftBlock: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(state.weather)
                .font(.system(size: 12, weight: .semibold))
                .foregroundColor(theme.colors.inkSoft)
            Text(greetingDisplay)
                .font(.system(size: 22, weight: .heavy, design: .rounded))
                .foregroundColor(theme.colors.ink)
        }
    }

    /// greeting 显示：sessionStore 非 nil 时 fallback 到 nickname（保 5.2 lesson）；
    /// 否则取 state.greeting（Mock / Real 子类构造时已设置）.
    /// 两条路径都在 statusBar 内可视一致；测试 / Preview 走静态路径.
    private var greetingDisplay: String {
        if let sessionStore = sessionStore {
            // 生产路径：session 非 nil 时也优先用 state.greeting（greeting 是 ViewModel 视觉态，不是身份名）.
            // 老 5.2 lesson 仍保 a11y label 用 nickname (见 currentNicknameForA11y).
            _ = sessionStore   // 显式 capture 防编译器警告未用
            return state.greeting
        }
        return state.greeting
    }

    /// 老 lesson 5.2：a11y label 必须等于 nickname（保 testUserInfoBarRetainsNicknameAccessibilityLabel）.
    /// session 非 nil 走 SessionStore.session?.user.nickname；否则 fallback state.nickname.
    private var currentNicknameForA11y: String {
        HomeNicknameResolver.resolve(session: sessionStore?.session, fallback: state.nickname)
    }

    /// 步数 capsule：Image footprint + 数字 + "步"，圆角胶囊背景 + border + sm shadow.
    /// 保留 AccessibilityID.Home.stepBalance 老锚（命名继续，物理位置变化）.
    private var stepBalanceCapsule: some View {
        HStack(spacing: 6) {
            Image(systemName: Icons.symbol(for: "footprint"))
                .font(.system(size: 14))
                .foregroundColor(theme.colors.coin)
            Text("\(appState.currentStepAccount?.availableSteps ?? 0)")
                .font(.system(size: 15, weight: .heavy))
                .foregroundColor(theme.colors.ink)
            Text("步")
                .font(.system(size: 11, weight: .semibold))
                .foregroundColor(theme.colors.inkSoft)
        }
        .padding(.vertical, 8)
        .padding(.horizontal, 14)
        .background(Capsule().fill(theme.colors.surface))
        .overlay(Capsule().stroke(theme.colors.border, lineWidth: 1))
        .shadow(
            color: theme.shadow.sm.color,
            radius: theme.shadow.sm.radius,
            x: theme.shadow.sm.x,
            y: theme.shadow.sm.y
        )
        .accessibilityIdentifier(AccessibilityID.Home.stepBalance)
    }

    // MARK: - 区块 2: catStage (home.jsx:40-81)

    /// 猫展示卡：Card 28pt 圆角 + 内含 SF Symbol cat.fill 220pt + 等级名牌左上 + 三状态条底部 + floatUp emoji 浮层.
    /// a11y：父容器 `homeCatStage` + cat sprite `AccessibilityID.Home.petArea` + 名牌 `AccessibilityID.Home.petName`.
    private var catStage: some View {
        Card(cornerRadius: theme.radius.modalLg, padding: theme.spacing.s20) {
            ZStack {
                // 装饰背景斑点（accent-soft 50pt + 30pt 圆，固定位置 + 低 opacity）.
                catStageDecorBlobs

                // 中心 cat sprite (SF Symbol cat.fill 220pt; ink-soft).
                Image(systemName: "cat.fill")
                    .resizable()
                    .scaledToFit()
                    .frame(width: 180, height: 180)
                    .foregroundColor(theme.colors.inkSoft.opacity(0.7))
                    .accessibilityElement(children: .ignore)
                    .accessibilityLabel(Text("猫展示区"))
                    .accessibilityIdentifier(AccessibilityID.Home.petArea)

                // 等级名牌（左上）+ 三状态条（底部）+ floatUp emoji 浮层
                VStack {
                    HStack {
                        catLevelBadge
                        Spacer()
                    }
                    Spacer()
                    catStatsBar
                }

                // floatUp emoji 浮层（interactionAnimation = .flying(emoji) 时渲染）.
                //
                // Story 37.7 codex round 1 [P2] fix：删除常量 `.opacity(0)`（永不可见 bug）.
                // `.transition(.opacity)` 在 emoji `if` 入/出场切换时会自动 fade in/out；
                // `.animation(.easeOut(duration: 1.4), value: state.interactionAnimation)` 让此过渡
                // 跟随 interactionAnimation 切换驱动. HomeView body 末尾 `.onChange` 在 1.4s 后
                // 自动把 interactionAnimation 重置回 .idle，触发离场动画.
                if case let .flying(emoji) = state.interactionAnimation {
                    Text(emoji)
                        .font(.system(size: 44))
                        .offset(y: -110)
                        .transition(.opacity)
                        .animation(.easeOut(duration: 1.4), value: state.interactionAnimation)
                }
            }
            .frame(height: 280)
        }
        .accessibilityIdentifier("homeCatStage")
        .accessibilityElement(children: .contain)
    }

    /// catStage 装饰背景斑点（accent-soft 不同 size 圆）.
    private var catStageDecorBlobs: some View {
        ZStack {
            Circle()
                .fill(theme.colors.accentSoft.opacity(0.5))
                .frame(width: 100, height: 100)
                .offset(x: -80, y: -60)
            Circle()
                .fill(theme.colors.accentSoft.opacity(0.4))
                .frame(width: 60, height: 60)
                .offset(x: 90, y: 70)
        }
    }

    /// 等级名牌：accent 背景 + white 文字 12pt 700.
    /// petLevel 暂时 mock=8（HomeUser 没 level 字段；mock 字面量）；petName 来自 HomePetNameResolver.
    private var catLevelBadge: some View {
        Text("Lv.8 · \(petNameDisplay)")
            .font(.system(size: 12, weight: .heavy))
            .foregroundColor(.white)
            .padding(.horizontal, 10)
            .padding(.vertical, 4)
            .background(
                RoundedRectangle(cornerRadius: theme.radius.tagMd)
                    .fill(theme.colors.accent)
            )
            .accessibilityIdentifier(AccessibilityID.Home.petName)
    }

    /// pet 名称显示（保 Story 5.5 / 37.4 三态语义）.
    private var petNameDisplay: String {
        HomePetNameResolver.resolve(pet: appState.currentPet, hasHydrated: appState.currentUser != nil)
    }

    /// 三状态条（饱食/心情/活力）：横向并列 progress bar 0-100.
    private var catStatsBar: some View {
        HStack(spacing: theme.spacing.s10) {
            statBar(label: "饱食", value: state.stats.hunger, color: theme.colors.warn)
            statBar(label: "心情", value: state.stats.mood, color: theme.colors.accent)
            statBar(label: "活力", value: state.stats.energy, color: theme.colors.success)
        }
    }

    /// 单条状态条（label + bar + 数值文本）.
    private func statBar(label: String, value: Int, color: Color) -> some View {
        VStack(spacing: 2) {
            Text(label)
                .font(.system(size: 10, weight: .heavy))
                .foregroundColor(theme.colors.inkSoft)
            GeometryReader { geo in
                ZStack(alignment: .leading) {
                    Capsule()
                        .fill(theme.colors.inkSoft.opacity(0.15))
                    Capsule()
                        .fill(color)
                        .frame(width: geo.size.width * CGFloat(value) / 100.0)
                }
            }
            .frame(height: 6)
        }
    }

    // MARK: - 区块 3: actionRow (home.jsx:83-88)

    /// 三个动作按钮：喂食 🍥 / 抚摸 💕 / 玩耍 ⭐.
    /// 按钮调 state.onFeedTap() / onPetTap() / onPlayTap()；inline Card 容器（不抽 Primitives；属 Home Feature 私有 composite）.
    private var actionRow: some View {
        HStack(spacing: theme.spacing.s10) {
            actionButton(
                label: "喂食",
                iconKey: "bowl",
                accentColor: theme.colors.warn,
                a11yId: "homeActionFeed",
                action: { state.onFeedTap() }
            )
            actionButton(
                label: "抚摸",
                iconKey: "heart",
                accentColor: theme.colors.accent,
                a11yId: "homeActionPet",
                action: { state.onPetTap() }
            )
            actionButton(
                label: "玩耍",
                iconKey: "ball",
                accentColor: theme.colors.success,
                a11yId: "homeActionPlay",
                action: { state.onPlayTap() }
            )
        }
    }

    /// 单个 ActionButton: Card 容器 + Icon + label，包在 Button 内.
    private func actionButton(
        label: String,
        iconKey: String,
        accentColor: Color,
        a11yId: String,
        action: @escaping () -> Void
    ) -> some View {
        Button(action: action) {
            VStack(spacing: 6) {
                Image(systemName: Icons.symbol(for: iconKey))
                    .font(.system(size: 22, weight: .semibold))
                    .foregroundColor(accentColor)
                Text(label)
                    .font(.system(size: 13, weight: .heavy))
                    .foregroundColor(theme.colors.ink)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, 14)
            .background(
                RoundedRectangle(cornerRadius: theme.radius.cardMd)
                    .fill(theme.colors.surface)
                    .shadow(
                        color: theme.shadow.sm.color,
                        radius: theme.shadow.sm.radius,
                        x: theme.shadow.sm.x,
                        y: theme.shadow.sm.y
                    )
            )
            .overlay(
                RoundedRectangle(cornerRadius: theme.radius.cardMd)
                    .stroke(theme.colors.border, lineWidth: 1)
            )
        }
        .buttonStyle(PressedOffsetButtonStyle())
        .accessibilityIdentifier(a11yId)
    }

    // MARK: - 区块 5: teamIdleCard (home.jsx:147-188)

    /// "和好友一起玩耍" 卡片：accent → accentDeep 渐变背景 + 22pt 圆角 + 标题 + 副标题 + 两 PrimaryButton.
    private var teamIdleCard: some View {
        ZStack {
            // 渐变背景容器
            RoundedRectangle(cornerRadius: theme.radius.cardLg)
                .fill(
                    LinearGradient(
                        colors: [theme.colors.accent, theme.colors.accentDeep],
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
                    )
                )
                .shadow(
                    color: theme.shadow.md.color,
                    radius: theme.shadow.md.radius,
                    x: theme.shadow.md.x,
                    y: theme.shadow.md.y
                )

            // 装饰圆点（白色透明，固定位置）
            teamIdleCardDecorDots

            // 内容（标题/副标题/两按钮）
            VStack(alignment: .leading, spacing: theme.spacing.s12) {
                HStack(spacing: 8) {
                    Image(systemName: Icons.symbol(for: "paw"))
                        .font(.system(size: 18, weight: .heavy))
                        .foregroundColor(.white)
                    Text("和好友一起玩耍")
                        .font(.system(size: 18, weight: .heavy))
                        .foregroundColor(.white)
                }
                Text("创建一个小屋，或用房间代码加入好友的队伍")
                    .font(.system(size: 13, weight: .semibold))
                    .foregroundColor(.white.opacity(0.85))

                HStack(spacing: theme.spacing.s10) {
                    PrimaryButton(
                        title: "创建队伍",
                        variant: .secondary,
                        icon: Icons.symbol(for: "enter"),
                        fullWidth: true,
                        action: { state.onCreateTap() }
                    )
                    .accessibilityIdentifier("homeTeamIdleCard_create")

                    PrimaryButton(
                        title: "加入队伍",
                        variant: .ghost,
                        icon: Icons.symbol(for: "enter"),
                        fullWidth: true,
                        action: { state.onJoinTap() }
                    )
                    .accessibilityIdentifier("homeTeamIdleCard_join")
                }
            }
            .padding(18)
        }
    }

    /// teamIdleCard 装饰圆点（白色透明 0.1 / 0.08）.
    private var teamIdleCardDecorDots: some View {
        ZStack {
            Circle()
                .fill(Color.white.opacity(0.1))
                .frame(width: 80, height: 80)
                .offset(x: 130, y: -50)
            Circle()
                .fill(Color.white.opacity(0.08))
                .frame(width: 50, height: 50)
                .offset(x: -120, y: 40)
        }
    }

    // MARK: - 版本号 footer（保 Story 37.3 AC7 第 4 条 ping/version 角落显示红线）

    private var versionFooter: some View {
        HStack {
            Spacer()
            Text("v\(state.appVersion) · \(state.serverInfo)")
                .font(.caption)
                .foregroundStyle(.secondary)
                .accessibilityIdentifier(AccessibilityID.Home.versionLabel)
        }
        .padding(.top, 4)
    }
}

/// nickname 显示决策的纯函数 helper（Story 5.2 codex round 1 [P1] fix 沿用）.
public enum HomeNicknameResolver {
    /// 决定 statusBar a11y label 应显示哪个 nickname.
    /// - Parameters:
    ///   - session: 当前 SessionStore.session（nil 表示未登录 / 启动早期）.
    ///   - fallback: viewModel 透传的默认 nickname（如 "用户1001"）.
    /// - Returns: session 非 nil 时返回 session.user.nickname；否则返回 fallback.
    public static func resolve(session: SessionState?, fallback: String) -> String {
        session?.user.nickname ?? fallback
    }
}

/// pet 名称显示决策（Story 5.5 codex round 1 [P2] fix；Story 37.4 签名改造，Story 37.7 沿用不动）.
public enum HomePetNameResolver {
    /// loading 期占位文案（hasHydrated == false）.
    public static let loadingPlaceholder = "默认小猫"

    /// server 明确返回 pet=null 时的文案（hasHydrated == true && pet == nil）.
    public static let noPetPlaceholder = "暂无宠物"

    /// 决定 catStage 等级名牌应显示哪个名称.
    public static func resolve(pet: HomePet?, hasHydrated: Bool) -> String {
        guard hasHydrated else { return loadingPlaceholder }
        guard let pet = pet else { return noPetPlaceholder }
        return pet.name
    }
}

#if DEBUG
#Preview("HomeView — candy") {
    HomeView(state: MockHomeViewModel()) { EmptyView() }
        .environmentObject(AppState())
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("HomeView — dark") {
    HomeView(state: MockHomeViewModel()) { EmptyView() }
        .environmentObject(AppState())
        .environment(\.theme, ThemeName.dark.theme)
}
#endif
