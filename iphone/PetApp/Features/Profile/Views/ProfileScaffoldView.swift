// ProfileScaffoldView.swift
// Story 37.11 AC4: ui_design profile.jsx 高保真我的页 Scaffold（5 区块视觉 + 10+ a11y 锚 + #Preview 4 配置 + BindWechatModal sheet）.
//
// 关键设计：
//   - struct 非泛型（Profile 无类似 chestSlot 接缝点；未来 NavigationLink slot 时再走泛型 ViewBuilder 路径）
//   - `@ObservedObject var state: ProfileViewModel` 基类直接（与 HomeView / RoomScaffoldView /
//     WardrobeScaffoldView / FriendsScaffoldView 同模式）
//   - 5 区块：headerCard / statsCard / wechatCard（双态）/ recentCollections / menuList
//     + toastOverlay + BindWechatModal sheet
//   - profile / wechatBound / showBindModal / lastToastMessage 走 ViewModel @Published（不走 SwiftUI @State）—
//     单元测试需要直接断言（ADR-0010 §3.2 + story 37.11 Dev Notes "state owner 边界" 钦定）.
//   - 10+ a11y identifier inline 字符串（profileView / profileHeaderCard / profileStatsCard /
//     profileWeChatCard(Bound) / profileWeChatModal / profileWeChatBindButton / profileWeChatCancelButton /
//     profileCollectionViewAll / profileCollectionCell_<id> / profileMenu_<rawValue> / profileToast）；
//     Story 37.13 a11y 总表归并到 AccessibilityID.Profile.
//   - 视觉规则：iphone/ui_design/source/screens/profile.jsx + iphone/ui_design/wechat_binding.md
//
// Story 37.7 / 37.8 / 37.9 / 37.10 沉淀 lesson 预防性应用：
//   - 所有 Card / IconButton / Stat / DataLossRow shadow 必须挂在
//     `RoundedRectangle.fill(...).shadow(...)` 那一层（不挂最外层 chain）—
//     按 Story 37.6 round 5 lesson `2026-04-30-swiftui-state-survives-id-and-shadow-over-children.md` 钦定路径.
//   - 微信卡 / Modal 用硬编码品牌色（不进 theme tokens）—— 详见 spec AC4 关键决策 5.

import SwiftUI
import os.log

public struct ProfileScaffoldView: View {
    @ObservedObject public var state: ProfileViewModel

    /// Story 37.5: 主题 token 取值入口；RootView 注入 `.environment(\.theme, currentTheme.theme)`.
    @Environment(\.theme) private var theme

    /// Story 37.11 round 4 codex review [P2] 修复：跟踪 sheet 关闭原因，分发 onDismiss 闭包行为.
    ///
    /// **背景**：SwiftUI `.sheet(isPresented:onDismiss:)` 在 sheet disappear 时**总是**跑 onDismiss 闭包，
    /// 不区分关闭路径。round 3 修法（onDismiss 无脑调 `state.onWeChatModalDismissTap()`）在 3 个路径里有 2 个错：
    ///   1. "稍后再说" 按钮 → 闭包先调 dismiss method（写 showBindModal=false）→ SwiftUI 跑 onDismiss → **再调一次 dismiss**（错：fired twice）
    ///   2. "绑定微信" confirm 按钮 → 闭包调 confirm method（写 wechatBound=true + showBindModal=false）→ SwiftUI 跑 onDismiss → 错触发 dismiss path
    ///   3. swipe-dismiss → SwiftUI 直接设 binding=false → onDismiss 调 dismiss method（这个对）
    ///
    /// **修法**：用 dismissReason state tag 在 onDismiss 闭包内分发；按钮路径不再直接调 ViewModel method，只设 reason + 关 sheet.
    /// onDismiss 闭包成为唯一调 dismiss / confirm method 的地方，避免双触发.
    /// lesson: 2026-05-02-sheet-onDismiss-fires-on-button-close-too.md
    private enum DismissReason {
        case confirm     // 用户点"绑定微信"
        case declined    // 用户点"稍后再说"
        // nil = swipe-dismiss
    }
    @State private var dismissReason: DismissReason?

    public init(state: ProfileViewModel) {
        self.state = state
    }

    public var body: some View {
        ScrollView {
            VStack(spacing: 0) {
                headerCard            // 区块 1: 顶部渐变头图 + Avatar + 用户名/ID/title/joinedAt
                statsCard             // 区块 2: 4 列统计卡（覆盖头图底部 1/3 negative margin）
                wechatCard            // 区块 3: 微信绑定卡（双态 wechatBound true/false 切换）
                recentCollectionsSection  // 区块 4: 最近收藏横向滑窗（5 件 cell）
                menuList              // 区块 5: 菜单列表 4 项
            }
            .padding(.bottom, 100)    // 让出浮动 TabBar 空间
        }
        .background(theme.colors.pageBg.ignoresSafeArea())
        .accessibilityIdentifier(AccessibilityID.Profile.view)
        .overlay(alignment: .bottom) { toastOverlay }
        // Story 37.11 round 4 codex review [P2] 修复：onDismiss 按 dismissReason 分发.
        //
        // 三路径行为：
        //   - .confirm（"绑定微信"按钮按下）→ onDismiss 调 onWeChatBindConfirmTap()（让确认/绑定动作集中在 onDismiss）.
        //     按钮闭包**只**设 reason + 让 SwiftUI 关 sheet；按钮不再直接调 ViewModel method.
        //   - .declined（"稍后再说"按钮按下）→ onDismiss 调 onWeChatModalDismissTap() 一次.
        //     按钮闭包**只**设 reason + 让 SwiftUI 关 sheet；按钮不再直接调 ViewModel method.
        //   - nil（swipe-dismiss / 编程外部置 false）→ onDismiss 调 onWeChatModalDismissTap() 一次（与 round 3 同精神：所有 dismiss 路径都过 ViewModel seam）.
        //
        // 关键不变量：onDismiss 是唯一调 ViewModel method 的地方，杜绝按钮闭包 + onDismiss 双触发.
        // dismissReason 在 onDismiss 末尾置回 nil，下一次 sheet 弹出 swipe-dismiss 路径仍走默认 nil 分支.
        //
        // round 3 lesson 不变：sheet swipe-dismiss 必须路由经 ViewModel hook，避免后续 epic（lastWechatPromptAt 持久化）silently skip.
        // round 4 lesson: 2026-05-02-sheet-onDismiss-fires-on-button-close-too.md
        .sheet(
            isPresented: $state.showBindModal,
            onDismiss: {
                switch dismissReason {
                case .confirm:
                    state.onWeChatBindConfirmTap()
                case .declined, .none:
                    state.onWeChatModalDismissTap()
                }
                dismissReason = nil  // reset for next sheet showing
            }
        ) {
            bindWechatSheet
        }
    }

    // MARK: - 区块 1: headerCard (profile.jsx:21-56)

    /// 顶部渐变头图卡：背景渐变 + "我的"标题 + bell/settings 圆形按钮 + Avatar + 用户信息.
    private var headerCard: some View {
        VStack(spacing: 0) {
            // 标题行：左 "我的" + 右 bell / settings 圆形按钮
            HStack {
                Text("我的")
                    .font(.system(size: 22, weight: .heavy))
                    .foregroundColor(.white)
                Spacer()
                HStack(spacing: 8) {
                    // Story 37.11 round 3 codex review [P2-A] 修复：
                    // bell / settings 按钮必须走 ViewModel onBellTap / onSettingsTap method seam，
                    // 不再直接写 state.lastToastMessage —— 防后续 epic 接 NavigationLink 真实导航
                    // 时必须改 View 而非 VM（违反 zero-edit scaffold 契约）.
                    // lesson: 2026-05-01-scaffold-view-must-not-bypass-viewmodel-method-seam.md
                    headerIconButton(iconKey: "bell") {
                        state.onBellTap()
                    }
                    .accessibilityIdentifier(AccessibilityID.Profile.bellButton)
                    headerIconButton(iconKey: "settings") {
                        state.onSettingsTap()
                    }
                    .accessibilityIdentifier(AccessibilityID.Profile.settingsButton)
                }
            }
            .padding(.bottom, 14)

            // 用户信息行：Avatar + VStack(name / id+title / joinedAt 药丸)
            HStack(spacing: 14) {
                Avatar(
                    name: state.profile.name,
                    size: 72,
                    color: Color(hex: 0xfff1e8),
                    ring: true
                )

                VStack(alignment: .leading, spacing: 4) {
                    Text(state.profile.name)
                        .font(.system(size: 22, weight: .heavy))
                        .foregroundColor(.white)
                    Text("ID: \(state.profile.id) · \(state.profile.title)")
                        .font(.system(size: 12, weight: .bold))
                        .foregroundColor(.white.opacity(0.85))
                    HStack(spacing: 4) {
                        Image(systemName: Icons.symbol(for: "sparkle"))
                            .font(.system(size: 10))
                        Text("加入于 \(state.profile.joinedAt)")
                            .font(.system(size: 11, weight: .bold))
                    }
                    .foregroundColor(.white)
                    .padding(.vertical, 3)
                    .padding(.horizontal, 8)
                    .background(
                        Capsule().fill(Color.white.opacity(0.18))
                    )
                }
                Spacer()
            }
        }
        .padding(.top, 8)          // safe area top 已自动 respect；只补呼吸空间. 详见 HomeView.swift:90 注释.
        .padding(.horizontal, 20)
        .padding(.bottom, 50)
        .background(
            LinearGradient(
                colors: [theme.colors.accentSoft, theme.colors.accent],
                startPoint: .top,
                endPoint: .bottom
            )
        )
        .accessibilityIdentifier(AccessibilityID.Profile.headerCard)
    }

    /// headerCard 右上角圆形 IconButton（bell / settings）.
    private func headerIconButton(iconKey: String, action: @escaping () -> Void) -> some View {
        Button(action: action) {
            Image(systemName: Icons.symbol(for: iconKey))
                .font(.system(size: 16))
                .foregroundColor(.white)
                .frame(width: 36, height: 36)
                .background(
                    Circle().fill(Color.white.opacity(0.30))
                )
        }
    }

    // MARK: - 区块 2: statsCard (profile.jsx:58-73)

    /// 4 列统计卡：收藏品 / 好友 / 小猫等级 / 成就（margin top -34 覆盖头图底部）.
    private var statsCard: some View {
        HStack(spacing: 0) {
            statColumn(label: "收藏品", value: "\(state.profile.collectionsCount)", iconKey: "diamond")
            statDivider
            statColumn(label: "好友", value: "\(state.profile.friendsCount)", iconKey: "friends")
            statDivider
            statColumn(label: "小猫等级", value: "Lv.\(state.profile.petLevel)", iconKey: "paw")
            statDivider
            statColumn(label: "成就", value: "\(state.profile.achievementsCount)", iconKey: "trophy")
        }
        .padding(16)
        .background(
            RoundedRectangle(cornerRadius: 22)
                .fill(theme.colors.surface)
                .shadow(
                    color: theme.shadow.md.color,
                    radius: theme.shadow.md.radius,
                    x: theme.shadow.md.x,
                    y: theme.shadow.md.y
                )
        )
        .overlay(
            RoundedRectangle(cornerRadius: 22).stroke(theme.colors.border, lineWidth: 1)
        )
        .padding(.horizontal, 20)
        .padding(.top, -34)   // 覆盖头图底部
        .accessibilityIdentifier(AccessibilityID.Profile.statsCard)
    }

    private func statColumn(label: String, value: String, iconKey: String) -> some View {
        VStack(spacing: 4) {
            HStack(spacing: 4) {
                Image(systemName: Icons.symbol(for: iconKey))
                    .font(.system(size: 14))
                    .foregroundColor(theme.colors.accent)
                Text(value)
                    .font(.system(size: 17, weight: .heavy))
                    .foregroundColor(theme.colors.ink)
            }
            Text(label)
                .font(.system(size: 10, weight: .bold))
                .foregroundColor(theme.colors.inkSoft)
        }
        .frame(maxWidth: .infinity)
    }

    /// statsCard 内 4 列之间的自绘 1pt Divider（高度 6pt vertical inset + theme.colors.border 色）.
    /// 不用 SwiftUI `Divider()` —— system Divider 默认 0.5pt + 系统色，与 ui_design 1pt + theme.colors.border 不一致.
    private var statDivider: some View {
        Rectangle()
            .fill(theme.colors.border)
            .frame(width: 1)
            .padding(.vertical, 6)
    }

    // MARK: - 区块 3: wechatCard (profile.jsx:75-134；双态分支)

    /// 微信绑定卡：未绑定走黄色警告卡（整张可点）/ 已绑定走绿色确认卡（纯展示）.
    @ViewBuilder
    private var wechatCard: some View {
        if state.wechatBound {
            wechatCardBound
        } else {
            wechatCardUnbound
        }
    }

    /// 未绑定卡：黄色警告渐变 + warn icon + 主标题 + 副标题 + 立即绑定胶囊按钮.
    private var wechatCardUnbound: some View {
        Button(action: { state.onWeChatCardTap() }) {
            HStack(spacing: 12) {
                // 左：40x40 圆 12 白底 + 黄色 shadow + warn icon
                Image(systemName: Icons.symbol(for: "warn"))
                    .font(.system(size: 22))
                    .foregroundColor(Color(hex: 0xe89400))
                    .frame(width: 40, height: 40)
                    .background(
                        RoundedRectangle(cornerRadius: 12)
                            .fill(Color.white)
                            .shadow(color: Color(red: 1.0, green: 0.71, blue: 0.0).opacity(0.30), radius: 6, x: 0, y: 2)
                    )

                // 中：VStack 主+副标题
                VStack(alignment: .leading, spacing: 2) {
                    Text("绑定微信，保护小猫数据")
                        .font(.system(size: 14, weight: .heavy))
                        .foregroundColor(Color(hex: 0x7a4f00))
                    Text("未绑定时卸载 App 将丢失全部数据")
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(Color(hex: 0xa06b00))
                }

                Spacer()

                // 右：立即绑定绿色胶囊
                HStack(spacing: 4) {
                    Image(systemName: Icons.symbol(for: "wechat"))
                        .font(.system(size: 12))
                    Text("立即绑定")
                        .font(.system(size: 12, weight: .heavy))
                }
                .foregroundColor(.white)
                .padding(.vertical, 8)
                .padding(.horizontal, 12)
                .background(
                    RoundedRectangle(cornerRadius: 14).fill(Color(hex: 0x1aad19))
                )
            }
            .padding(14)
            .background(
                RoundedRectangle(cornerRadius: 18)
                    .fill(LinearGradient(
                        colors: [Color(hex: 0xfff8e1), Color(hex: 0xffe8b5)],
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
                    ))
                    .shadow(
                        color: theme.shadow.sm.color,
                        radius: theme.shadow.sm.radius,
                        x: theme.shadow.sm.x,
                        y: theme.shadow.sm.y
                    )
            )
            .overlay(
                RoundedRectangle(cornerRadius: 18).stroke(Color(hex: 0xffc94c), lineWidth: 1.5)
            )
        }
        .buttonStyle(.plain)
        .padding(.horizontal, 20)
        .padding(.top, 14)
        .accessibilityIdentifier(AccessibilityID.Profile.weChatCard)
    }

    /// 已绑定卡：surface 背景 + wechat 浅绿底 icon + 主标题 + 已保护角标 + 副标题 + 右 shield icon.
    private var wechatCardBound: some View {
        HStack(spacing: 12) {
            // 左：40x40 圆 12 浅绿底 + wechat icon
            Image(systemName: Icons.symbol(for: "wechat"))
                .font(.system(size: 22))
                .foregroundColor(Color(hex: 0x1aad19))
                .frame(width: 40, height: 40)
                .background(
                    RoundedRectangle(cornerRadius: 12).fill(Color(hex: 0xe8f7e0))
                )

            // 中：主+副标题 + "已保护"角标
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text("微信已绑定")
                        .font(.system(size: 14, weight: .heavy))
                        .foregroundColor(theme.colors.ink)
                    Text("已保护")
                        .font(.system(size: 9, weight: .heavy))
                        .foregroundColor(.white)
                        .padding(.vertical, 2)
                        .padding(.horizontal, 6)
                        .background(
                            RoundedRectangle(cornerRadius: 6).fill(Color(hex: 0x1aad19))
                        )
                }
                Text("数据已同步至云端，卸载重装不会丢失")
                    .font(.system(size: 11, weight: .bold))
                    .foregroundColor(theme.colors.inkSoft)
            }

            Spacer()

            // 右：shield icon
            Image(systemName: Icons.symbol(for: "shield"))
                .font(.system(size: 20))
                .foregroundColor(Color(hex: 0x1aad19))
        }
        .padding(14)
        .background(
            RoundedRectangle(cornerRadius: 18)
                .fill(theme.colors.surface)
                .shadow(
                    color: theme.shadow.sm.color,
                    radius: theme.shadow.sm.radius,
                    x: theme.shadow.sm.x,
                    y: theme.shadow.sm.y
                )
        )
        .overlay(
            RoundedRectangle(cornerRadius: 18).stroke(theme.colors.border, lineWidth: 1)
        )
        .padding(.horizontal, 20)
        .padding(.top, 14)
        .accessibilityIdentifier(AccessibilityID.Profile.weChatCardBound)
    }

    // MARK: - 区块 4: recentCollections (profile.jsx:136-162)

    private var recentCollectionsSection: some View {
        VStack(alignment: .leading, spacing: 10) {
            // SectionHeader：左 "最近收藏" + 右 "查看全部" 按钮
            HStack {
                Text("最近收藏")
                    .font(.system(size: 15, weight: .heavy))
                    .foregroundColor(theme.colors.ink)
                Spacer()
                Button(action: { state.onCollectionViewAllTap() }) {
                    HStack(spacing: 2) {
                        Text("查看全部")
                            .font(.system(size: 12, weight: .bold))
                        Image(systemName: Icons.symbol(for: "chevronRight"))
                            .font(.system(size: 10))
                    }
                    .foregroundColor(theme.colors.accentDeep)
                }
                .accessibilityIdentifier(AccessibilityID.Profile.collectionViewAll)
            }

            // 横向滑窗
            if state.recentCollections.isEmpty {
                Text("暂无收藏")
                    .font(.system(size: 12, weight: .semibold))
                    .foregroundColor(theme.colors.inkSoft.opacity(0.6))
                    .padding(.vertical, 20)
                    .frame(maxWidth: .infinity)
            } else {
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 10) {
                        ForEach(state.recentCollections) { rc in
                            collectionCell(rc)
                        }
                    }
                }
            }
        }
        .padding(.horizontal, 20)
        .padding(.top, 18)
    }

    private func collectionCell(_ rc: RecentCollection) -> some View {
        VStack(spacing: 4) {
            Text(rc.emoji)
                .font(.system(size: 32))
                .frame(width: 60, height: 60)
                .background(
                    RoundedRectangle(cornerRadius: 12).fill(theme.colors.surface2)
                )
            Text(rc.name)
                .font(.system(size: 11, weight: .heavy))
                .foregroundColor(theme.colors.ink)
                .lineLimit(1)
        }
        .frame(width: 88)
        .padding(10)
        .background(
            RoundedRectangle(cornerRadius: 16)
                .fill(theme.colors.surface)
                .shadow(
                    color: theme.shadow.sm.color,
                    radius: theme.shadow.sm.radius,
                    x: theme.shadow.sm.x,
                    y: theme.shadow.sm.y
                )
        )
        .overlay(
            RoundedRectangle(cornerRadius: 16).stroke(theme.colors.border, lineWidth: 1)
        )
        .accessibilityIdentifier(AccessibilityID.Profile.collectionCell(rc.id))
    }

    // MARK: - 区块 5: menuList (profile.jsx:164-192)

    private var menuList: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("更多")
                .font(.system(size: 15, weight: .heavy))
                .foregroundColor(theme.colors.ink)

            VStack(spacing: 0) {
                ForEach(Array(ProfileMenuItem.allCases.enumerated()), id: \.element.id) { idx, item in
                    menuRow(item)
                    if idx < ProfileMenuItem.allCases.count - 1 {
                        Rectangle()
                            .fill(theme.colors.border)
                            .frame(height: 1)
                    }
                }
            }
            .background(
                RoundedRectangle(cornerRadius: 20)
                    .fill(theme.colors.surface)
                    .shadow(
                        color: theme.shadow.sm.color,
                        radius: theme.shadow.sm.radius,
                        x: theme.shadow.sm.x,
                        y: theme.shadow.sm.y
                    )
            )
            .overlay(
                RoundedRectangle(cornerRadius: 20).stroke(theme.colors.border, lineWidth: 1)
            )
            .clipShape(RoundedRectangle(cornerRadius: 20))
        }
        .padding(.horizontal, 20)
        .padding(.top, 18)
    }

    private func menuRow(_ item: ProfileMenuItem) -> some View {
        Button(action: { state.onMenuTap(item: item) }) {
            HStack(spacing: 12) {
                // 左：36x36 accentSoft 圆角内 icon
                Image(systemName: Icons.symbol(for: item.iconKey))
                    .font(.system(size: 18))
                    .foregroundColor(theme.colors.accentDeep)
                    .frame(width: 36, height: 36)
                    .background(
                        RoundedRectangle(cornerRadius: 12).fill(theme.colors.accentSoft)
                    )

                // 中：label
                Text(item.label)
                    .font(.system(size: 14, weight: .bold))
                    .foregroundColor(theme.colors.ink)

                Spacer()

                // 右：extraText（如非空）+ chevronRight
                if !item.extraText.isEmpty {
                    Text(item.extraText)
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(theme.colors.inkSoft)
                }
                Image(systemName: Icons.symbol(for: "chevronRight"))
                    .font(.system(size: 14))
                    .foregroundColor(theme.colors.inkMute)
            }
            .padding(.vertical, 14)
            .padding(.horizontal, 16)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier(AccessibilityID.Profile.menu(item.rawValue))
    }

    // MARK: - toastOverlay (与 Story 37.10 同精神)

    /// 占位 toast 视觉（仅 lastToastMessage 非 nil 时渲染；不实装"3 秒自动消失"timer）.
    @ViewBuilder
    private var toastOverlay: some View {
        if let message = state.lastToastMessage {
            Text(message)
                .font(.system(size: 13, weight: .bold))
                .foregroundColor(.white)
                .padding(.vertical, 8)
                .padding(.horizontal, 16)
                .background(
                    RoundedRectangle(cornerRadius: 12)
                        .fill(Color.black.opacity(0.85))
                )
                .padding(.bottom, 120)  // 让出浮动 TabBar
                .accessibilityIdentifier(AccessibilityID.Profile.toast)
        }
    }

    // MARK: - bindWechatSheet (profile.jsx:205-283 + wechat_binding.md §强制提醒浮窗)

    /// Story 37.11 round 4 codex review [P2] 修复：用 closure 把"按钮意图"回传给 ProfileScaffoldView 设 dismissReason.
    /// BindWechatModalView 不直接调 ViewModel method —— 真正调用集中在 .sheet(onDismiss:) 闭包，杜绝双触发.
    @ViewBuilder
    private var bindWechatSheet: some View {
        if #available(iOS 16.4, *) {
            BindWechatModalView(
                state: state,
                onConfirmRequested: handleConfirmRequested,
                onDeclineRequested: handleDeclineRequested
            )
                .presentationDetents([.fraction(0.85)])
                .presentationCornerRadius(28)
        } else {
            BindWechatModalView(
                state: state,
                onConfirmRequested: handleConfirmRequested,
                onDeclineRequested: handleDeclineRequested
            )
        }
    }

    /// "绑定微信"主按钮回调：标记意图 + 关 sheet → onDismiss 跑 onWeChatBindConfirmTap().
    /// （round 4 P2 修：避免按钮闭包直接调 ViewModel method 导致 onDismiss 重复触发）
    private func handleConfirmRequested() {
        dismissReason = .confirm
        state.showBindModal = false
    }

    /// "稍后再说"次按钮回调：标记意图 + 关 sheet → onDismiss 跑 onWeChatModalDismissTap() **一次**.
    /// （round 4 P2 修：避免按钮闭包直接调 dismiss method + onDismiss 又调一次形成双触发）
    private func handleDeclineRequested() {
        dismissReason = .declined
        state.showBindModal = false
    }
}

// MARK: - BindWechatModalView 子视图（同文件内；与 ProfileScaffoldView 共享 state）

/// 微信绑定 Modal：警告插画 + 标题 + 正文（含红色高亮）+ 数据风险清单 4 行 + 按钮组.
///
/// Story 37.11 round 4 codex review [P2] 修复：按钮闭包**只**回调 onConfirmRequested / onDeclineRequested，
/// 不直接调 state.onWeChatBindConfirmTap / state.onWeChatModalDismissTap.
/// 真正的 ViewModel method 调用集中在 ProfileScaffoldView .sheet(onDismiss:) 闭包按 dismissReason 分发，避免双触发.
private struct BindWechatModalView: View {
    @ObservedObject var state: ProfileViewModel
    /// 用户按"绑定微信"主按钮：父视图设 dismissReason=.confirm + 关 sheet → onDismiss 触发 confirm method.
    let onConfirmRequested: () -> Void
    /// 用户按"稍后再说"次按钮：父视图设 dismissReason=.declined + 关 sheet → onDismiss 触发 dismiss method.
    let onDeclineRequested: () -> Void
    @Environment(\.theme) private var theme

    var body: some View {
        VStack(spacing: 0) {
            // 警告插画区
            warnIllustration
                .padding(.top, 24)

            // 标题
            Text("数据可能丢失！")
                .font(.system(size: 19, weight: .heavy))
                .foregroundColor(theme.colors.ink)
                .padding(.top, 14)

            // 正文（红色高亮 inline）
            warningBody
                .padding(.top, 8)
                .padding(.bottom, 16)
                .padding(.horizontal, 24)

            // 数据风险清单
            dataLossList
                .padding(.horizontal, 24)

            // 按钮组
            buttonGroup
                .padding(.top, 18)
                .padding(.horizontal, 24)
                .padding(.bottom, 24)

            Spacer(minLength: 0)
        }
        .frame(maxWidth: .infinity)
        .background(theme.colors.surface)
        .accessibilityIdentifier(AccessibilityID.Profile.weChatModal)
    }

    /// 警告插画：88x88 圆 44 黄色渐变 + warn icon（不实装外圈装饰旋转虚线圆环 —— spec 略）.
    private var warnIllustration: some View {
        Image(systemName: Icons.symbol(for: "warn"))
            .font(.system(size: 46))
            .foregroundColor(Color(hex: 0xe89400))
            .frame(width: 88, height: 88)
            .background(
                Circle()
                    .fill(LinearGradient(
                        colors: [Color(hex: 0xfff3d6), Color(hex: 0xffd97a)],
                        startPoint: .topLeading,
                        endPoint: .bottomTrailing
                    ))
            )
    }

    /// 正文："您还未绑定微信账号，" + 红色 inline "一旦卸载本 App，您的小猫、收藏品、好友关系等所有数据都将被永久删除，无法恢复。"
    private var warningBody: some View {
        let prefix = Text("您还未绑定微信账号，")
            .font(.system(size: 13, weight: .semibold))
            .foregroundColor(theme.colors.inkSoft)
        let highlight = Text("一旦卸载本 App，您的小猫、收藏品、好友关系等所有数据都将被永久删除，无法恢复。")
            .font(.system(size: 13, weight: .heavy))
            .foregroundColor(Color(hex: 0xe15f7c))
        return (prefix + highlight)
            .multilineTextAlignment(.center)
            .lineSpacing(4)
    }

    /// 数据风险清单 4 行（行间虚线分割除最后行外）.
    private var dataLossList: some View {
        VStack(spacing: 0) {
            dataLossRow(emoji: "🐱", text: "小猫 Lv.\(state.profile.petLevel) · \(state.profile.petName)", isLast: false)
            dashedDivider
            dataLossRow(emoji: "💎", text: "\(state.profile.collectionsCount) 件收藏品 · 价值 \(state.profile.coinsCount) 钻石", isLast: false)
            dashedDivider
            dataLossRow(emoji: "🏆", text: "\(state.profile.achievementsCount) 个成就徽章", isLast: false)
            dashedDivider
            dataLossRow(emoji: "👥", text: "\(state.profile.friendsCount) 位好友关系", isLast: true)
        }
        .padding(.vertical, 12)
        .padding(.horizontal, 14)
        .background(
            RoundedRectangle(cornerRadius: 16).fill(Color(hex: 0xfff5f5))
        )
        .overlay(
            RoundedRectangle(cornerRadius: 16).stroke(Color(hex: 0xffe0e0), lineWidth: 1)
        )
    }

    private func dataLossRow(emoji: String, text: String, isLast: Bool) -> some View {
        HStack(spacing: 8) {
            Text(emoji)
                .font(.system(size: 16))
            Text(text)
                .font(.system(size: 12, weight: .bold))
                .foregroundColor(Color(hex: 0x7a3a3a))
                .lineLimit(2)
            Spacer()
            Text("将丢失")
                .font(.system(size: 10, weight: .heavy))
                .foregroundColor(Color(hex: 0xe15f7c))
        }
        .padding(.vertical, 6)
    }

    /// 1pt dashed 虚线分割（DataLossList 行间）.
    private var dashedDivider: some View {
        Rectangle()
            .stroke(style: StrokeStyle(lineWidth: 1, dash: [3, 3]))
            .foregroundColor(Color(hex: 0xffd0d0))
            .frame(height: 1)
    }

    /// 按钮组：主按钮（绑定微信，保护数据）+ 次按钮（稍后再说）.
    ///
    /// Story 37.11 round 4 codex review [P2] 修复：按钮**只**回调 onConfirmRequested / onDeclineRequested，
    /// 由父视图设 dismissReason + 关 sheet → SwiftUI sheet onDismiss 闭包按 reason 分发到 ViewModel method，避免双触发.
    private var buttonGroup: some View {
        VStack(spacing: 10) {
            // 主按钮
            Button(action: onConfirmRequested) {
                HStack(spacing: 8) {
                    Image(systemName: Icons.symbol(for: "wechat"))
                        .font(.system(size: 18))
                    Text("绑定微信，保护数据")
                        .font(.system(size: 15, weight: .heavy))
                }
                .foregroundColor(.white)
                .frame(maxWidth: .infinity)
                .frame(height: 52)
                .background(
                    RoundedRectangle(cornerRadius: 26)
                        .fill(Color(hex: 0x1aad19))
                        .shadow(color: Color(hex: 0x138a12), radius: 0, x: 0, y: 4)
                )
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier(AccessibilityID.Profile.weChatBindButton)

            // 次按钮
            Button(action: onDeclineRequested) {
                Text("稍后再说（数据将不受保护）")
                    .font(.system(size: 12, weight: .bold))
                    .foregroundColor(theme.colors.inkMute)
                    .frame(maxWidth: .infinity)
                    .frame(height: 40)
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier(AccessibilityID.Profile.weChatCancelButton)
        }
    }
}

// MARK: - Previews

#if DEBUG
#Preview("ProfileScaffoldView — full mock / candy") {
    ProfileScaffoldView(state: MockProfileViewModel())
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("ProfileScaffoldView — full mock / dark") {
    ProfileScaffoldView(state: MockProfileViewModel())
        .environment(\.theme, ThemeName.dark.theme)
}

#Preview("ProfileScaffoldView — wechat bound / candy") {
    ProfileScaffoldView(state: MockProfileViewModel(wechatBound: true))
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("ProfileScaffoldView — bind modal open / candy") {
    ProfileScaffoldView(state: MockProfileViewModel(showBindModal: true))
        .environment(\.theme, ThemeName.candy.theme)
}
#endif
