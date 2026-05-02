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

    /// Story 37.7 codex round 2 [P2] fix：interactionAnimation `.flying` → `.idle` 重置 timer 句柄.
    /// rapid tap（如 t=0 Feed → t=0.5s Play）时取消上一个 1.4s sleep task —— 否则
    /// 第一个 timer 在 t=1.4s 触发把 .idle 写入，第二个 emoji 提前消失（应当持续到 t=1.9s）.
    @State private var resetTask: Task<Void, Never>?

    /// Story 37.12: JoinRoomModal 输入字段的 owner.
    /// SwiftUI @State（**不**进 ViewModel）—— modal 输入是 view-local transient，不需要跨 view 触发,
    /// 不需要单元测试断言（单元测试走 ViewModel 层断言 onJoinRoomConfirm 收到的 roomId 字符串）.
    /// 详见 spec Dev Notes "joinRoomInput @State vs @Published 决策".
    @State private var joinRoomInput: String = ""

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
        .sheet(
            isPresented: $state.showJoinModal,
            onDismiss: {
                // Story 37.12: dismiss 后清空 transient 输入字段（防 swipe-down dismiss 后再次打开 modal
                // 残留旧值；button-driven dismiss 走 onCancel/onConfirm 闭包内已可由 caller 决定关 sheet,
                // 此 onDismiss 是兜底，覆盖 swipe-down dismiss 路径，按 lesson 8
                // `2026-04-30-sheet-on-dismiss-button-vs-swipe-driven.md` 钦定路径预防性应用）.
                joinRoomInput = ""
            }
        ) {
            JoinRoomModal(
                roomIdInput: $joinRoomInput,
                onConfirm: { roomId in
                    state.onJoinRoomConfirm(roomId: roomId)
                    // 注：onJoinRoomConfirm override 内已设 showJoinModal = false（caller 不再重写）.
                    //   onDismiss 闭包稍后被 SwiftUI 调用，自动清空 joinRoomInput.
                },
                onCancel: {
                    state.showJoinModal = false
                    // 注：onDismiss 闭包稍后被 SwiftUI 调用，自动清空 joinRoomInput.
                }
            )
            .presentationDetents([.medium])
            .presentationCornerRadius(28)
        }
        .onChange(of: state.interactionAnimation) { _, newValue in
            // floatUp 动画完成后自动重置回 idle（1.4s ≈ ui_design home.jsx 钦定 floatUp 时长）.
            // 保 mock / production 行为一致：只有 .flying(...) 触发重置 timer，.idle 不做任何事.
            // iOS 17+ 双参签名：(oldValue, newValue) -> Void（避免单参 deprecation warning）.
            //
            // Story 37.7 codex round 2 [P2] fix（"rapid tap stale timer 提前清动画"）：
            //   每次进入 .flying 前先 cancel 上一个 resetTask —— 否则连续 onTap 时旧 timer
            //   仍会在 1.4s 后触发 .idle，把新 emoji 提前抹掉. 新 task body 开头 `Task.isCancelled`
            //   double-check 防 sleep 期间被取消的 race（cancel 不会中断 sleep，只标记 isCancelled）.
            guard case .flying = newValue else { return }
            resetTask?.cancel()
            resetTask = Task { @MainActor [weak state] in
                try? await Task.sleep(nanoseconds: 1_400_000_000)
                if Task.isCancelled { return }
                state?.interactionAnimation = .idle
            }
        }
    }

    // MARK: - 区块 1: statusBar (home.jsx:21-38)

    /// 顶部状态栏：左侧 weather + greeting，右侧步数 capsule（含老 ResetIdentityButton 注入入口）.
    /// a11y：父容器单 identifier `AccessibilityID.Home.userInfo`（值 = "homeStatusBar"）+ a11y label = nickname.
    /// Story 37.7 codex round 3 [P2-B] fix：删除老的空 Text overlay（VoiceOver 把空 node 当 focusable 卡顿）;
    /// 改 AccessibilityID.Home.userInfo 值 "home_userInfo" → "homeStatusBar" 同时满足新 / 老两条 UITest.
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
        // 父容器单 identifier：值 = "homeStatusBar"（AccessibilityID.Home.userInfo 重定义后）.
        // 老 UITest 用 enum 引用 → 自动兼容; 新 Story 37.7 AC8 UITest 用字面量 "homeStatusBar" → 也命中.
        // .accessibilityElement(children: .contain) 让子元素（stepBalance / btnResetIdentity）仍可独立定位.
        .accessibilityElement(children: .contain)
        .accessibilityLabel(Text(currentNicknameForA11y))
        .accessibilityIdentifier(AccessibilityID.Home.userInfo)
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
                // Story 37.7 codex round 5 [P2] fix（"emoji 静态出现不上升"）：
                //   把 emoji 抽成 `FloatingEmojiView` 子视图，用 `@State` 控制 y offset / opacity，
                //   `.onAppear` 内 withAnimation 驱动 0 → -110 + 1 → 0；外层 `.id(state.interactionAnimation)`
                //   让 SwiftUI 在每次 .flying(_, UUID()) 更新时重建子视图 → @State reset → onAppear 重跑动画.
                //   旧实装只是 emoji 直接落位 -110 + opacity transition，没有动画 position state 变化，所以视觉上
                //   是静止 emoji fade in/out，没有"升起"效果.
                //   注：这里 `.id(...)` 用的 explicit identity 不是 nil（与 Story 37.6 r4 lesson 不冲突）；
                //   AnimationState.flying 每次新 UUID 即新 identity，`.idle` 也是另一个 identity（emoji 视图
                //   仅在 .flying 分支被构造，.idle 时 if 整支不执行 → 自然卸载）.
                if case let .flying(emoji, _) = state.interactionAnimation {
                    FloatingEmojiView(emoji: emoji)
                        .id(state.interactionAnimation)
                        .transition(.opacity)
                }
            }
            .frame(height: 280)
        }
        .accessibilityIdentifier(AccessibilityID.Home.catStage)
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
                    .accessibilityIdentifier(AccessibilityID.Home.teamIdleCardCreate)

                    PrimaryButton(
                        title: "加入队伍",
                        variant: .ghost,
                        icon: Icons.symbol(for: "enter"),
                        fullWidth: true,
                        action: { state.onJoinTap() }
                    )
                    .accessibilityIdentifier(AccessibilityID.Home.teamIdleCardJoin)
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

/// catStage floatUp emoji 浮层子视图（Story 37.7 codex round 5 [P2] fix）.
///
/// 抽成独立 View 是为了让 `@State` y/opacity 跟随 `.id(state.interactionAnimation)` 重建逻辑：
///   - 父级在 `.flying(emoji, UUID)` 不同 UUID 切换时，`.id(...)` 让 SwiftUI 视为不同 identity → 重建
///     `FloatingEmojiView` → 新 @State 实例 → `.onAppear` 触发 → 动画从 y=0 / opacity=1 重新跑到 y=-110 / opacity=0.
///   - 若不抽离，把 @State 放 HomeView 上，rapid tap 会因为 @State 持久不重置导致动画 jump 或不重放.
///
/// 时长 1.4s 与 HomeView.onChange 内 reset timer 1.4s 对齐：动画播完时 ViewModel.interactionAnimation 也被
/// 重置回 `.idle`，emoji 视图自然卸载（if case 整支不再执行）；中途 rapid tap 触发新 UUID → 立即重建 → 重放.
public struct FloatingEmojiView: View {
    public let emoji: String

    /// y 起点 0（cat 中心基线），向上动画到 -110.
    @State private var animatedY: CGFloat = 0
    /// opacity 起点 1，淡出到 0.
    @State private var animatedOpacity: Double = 1.0

    public init(emoji: String) {
        self.emoji = emoji
    }

    public var body: some View {
        Text(emoji)
            .font(.system(size: 44))
            .offset(y: animatedY)
            .opacity(animatedOpacity)
            .onAppear {
                // 1.4s easeOut：y 0 → -110、opacity 1 → 0；与 ui_design home.jsx 钦定 floatUp 时长对齐.
                withAnimation(.easeOut(duration: 1.4)) {
                    animatedY = -110
                    animatedOpacity = 0
                }
            }
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
