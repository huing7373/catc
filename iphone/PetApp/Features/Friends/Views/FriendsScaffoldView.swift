// FriendsScaffoldView.swift
// Story 37.10 AC4: ui_design friends.jsx 高保真好友页 Scaffold（5 区块视觉 + 8+ a11y 锚 + #Preview 4 配置）.
//
// 关键设计：
//   - struct 非泛型（Friends 无类似 chestSlot 接缝点；未来 Story 37.13 / 后续 epic 加好友详情页
//     NavigationLink slot 时再走泛型 ViewBuilder 路径）
//   - `@ObservedObject var state: FriendsViewModel` 基类直接（与 HomeView / RoomScaffoldView /
//     WardrobeScaffoldView 同模式）
//   - 5 区块：topCard / myRoomCard / tabBar / friendsList / toastOverlay
//   - selectedTab / currentRoomId / lastToastMessage 走 ViewModel @Published（不走 SwiftUI @State）—
//     单元测试需要直接断言派生 displayedFriends + Mock onInvite/onJoin mutate currentRoomId 行为
//     （ADR-0010 §3.2 + story 37.10 Dev Notes "state owner 边界" 钦定）.
//   - 8+ a11y identifier inline 字符串：friendsView / friendsAddButton / friendsMyRoomCard /
//     friendsTab_<rawValue> / friendRow_<id> / friendActionButton_<id> / friendsToast；
//     Story 37.13 a11y 总表归并到 AccessibilityID.Friends.
//   - 视觉规则：iphone/ui_design/source/screens/friends.jsx + iphone/ui_design/README.md §FriendsScreen
//
// Story 37.7 / 37.8 / 37.9 沉淀 lesson 预防性应用：
//   - 所有 Card / row shadow 必须挂在 `RoundedRectangle.fill(...).shadow(...)` 那一层（不挂最外层 chain）—
//     按 Story 37.6 round 5 lesson `2026-04-30-swiftui-state-survives-id-and-shadow-over-children.md` 钦定路径.
//   - FriendRow 三态分支用 switch 而非 if-else（让漏分支编译期报错）.

import SwiftUI
import os.log

public struct FriendsScaffoldView: View {
    @ObservedObject public var state: FriendsViewModel

    /// Story 37.5: 主题 token 取值入口；RootView 注入 `.environment(\.theme, currentTheme.theme)`.
    @Environment(\.theme) private var theme

    public init(state: FriendsViewModel) {
        self.state = state
    }

    public var body: some View {
        VStack(spacing: 0) {
            topCard               // 区块 1: 顶部 Card（"X 位在线 · 共 Y 位" + "好友" + plus 添加按钮）
            myRoomCard            // 区块 2: 我的房间提示条（仅 currentRoomId != nil 渲染）
            tabBar                // 区块 3: 在线/全部 segmented control
            friendsList           // 区块 4 + 5: 好友列表 + FriendRow（含三态按钮）
        }
        .background(theme.colors.pageBg.ignoresSafeArea())
        .accessibilityIdentifier("friendsView")
        .overlay(alignment: .bottom) { toastOverlay }   // 占位 toast
    }

    // MARK: - 区块 1: topCard (friends.jsx:9-23)

    /// 顶部 Card：左 VStack 在线人数 + "好友" 标题 / 右 plus 圆形按钮.
    private var topCard: some View {
        HStack(alignment: .center) {
            // 左：VStack 在线人数 + 标题
            VStack(alignment: .leading, spacing: 0) {
                Text("\(state.onlineCount) 位在线 · 共 \(state.friends.count) 位")
                    .font(.system(size: 12, weight: .bold))
                    .foregroundColor(theme.colors.inkSoft)
                Text("好友")
                    .font(.system(size: 22, weight: .heavy))
                    .foregroundColor(theme.colors.ink)
            }

            Spacer()

            // 右：圆形 plus 按钮（占位 — 后续 epic 接真实"添加好友"流程；本期仅 print log + 写 toast）.
            Button(action: {
                os_log(.debug, "friendsAddButton tap (后续 epic will wire add friend flow)")
                state.lastToastMessage = "添加好友功能敬请期待"
            }) {
                Image(systemName: Icons.symbol(for: "plus"))
                    .font(.system(size: 20))
                    .foregroundColor(theme.colors.ink)
                    .frame(width: 40, height: 40)
                    .background(
                        Circle()
                            .fill(theme.colors.surface)
                            .shadow(
                                color: theme.shadow.sm.color,
                                radius: theme.shadow.sm.radius,
                                x: theme.shadow.sm.x,
                                y: theme.shadow.sm.y
                            )
                    )
                    .overlay(Circle().stroke(theme.colors.border, lineWidth: 1))
            }
            .accessibilityIdentifier("friendsAddButton")
        }
        .padding(.top, 68)
        .padding(.horizontal, 20)
        .padding(.bottom, 8)
    }

    // MARK: - 区块 2: myRoomCard (friends.jsx:25-44；条件渲染 currentRoomId != nil)

    /// 我的房间提示条：含 paw icon + "你的房间 / 代码 {currentRoomId}" + "分享给好友" 占位按钮.
    ///
    /// fix-review round 1 调整（与 spec Dev Notes "myRoomCard 分享按钮决策" 边界灰区路径一致）：
    /// - 加回 epic AC line 4807 钦定的「分享给好友」次要按钮（spec 给 fallback：若 review 反馈要求保留 → 此处 PrimaryButton(variant: .secondary)）.
    /// - a11y identifier 用 spec 钦定的 `friendsMyRoomShareButton`（命名风格与 `friendsMyRoomCard` 同 prefix；review 建议的 `friendsInRoomCard_share` 不采纳，spec 优先）.
    /// - 占位行为：state.onShareMyRoomTap() → 写 lastToastMessage "分享功能敬请期待"（与 spec Dev Notes 钦定文案一致）.
    /// - 后续 epic（节点 12 真实分享落地）改为接真实 share flow.
    @ViewBuilder
    private var myRoomCard: some View {
        if let roomId = state.currentRoomId {
            VStack(spacing: 8) {
                HStack(spacing: 10) {
                    // 圆形 paw icon
                    Image(systemName: Icons.symbol(for: "paw"))
                        .font(.system(size: 18))
                        .foregroundColor(.white)
                        .frame(width: 36, height: 36)
                        .background(Circle().fill(theme.colors.accent))

                    VStack(alignment: .leading, spacing: 0) {
                        Text("你的房间")
                            .font(.system(size: 11, weight: .bold))
                            .foregroundColor(theme.colors.inkSoft)
                        HStack(spacing: 0) {
                            Text("代码 ")
                                .font(.system(size: 14, weight: .heavy))
                                .foregroundColor(theme.colors.ink)
                            Text(roomId)
                                .font(.system(size: 14, weight: .heavy, design: .monospaced))
                                .foregroundColor(theme.colors.accentDeep)
                                .tracking(2)
                        }
                    }
                    Spacer()
                }

                // fix-review round 1 加回的「分享给好友」次要按钮（spec Dev Notes fallback 路径）.
                PrimaryButton(
                    title: "分享给好友",
                    variant: .secondary,
                    icon: Icons.symbol(for: "wechat"),
                    fullWidth: true,
                    action: { state.onShareMyRoomTap() }
                )
                .accessibilityIdentifier("friendsMyRoomShareButton")
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 10)
            .background(
                RoundedRectangle(cornerRadius: 16)
                    .fill(LinearGradient(
                        colors: [theme.colors.accentSoft, .clear],
                        startPoint: .leading,
                        endPoint: .trailing
                    ))
            )
            .overlay(RoundedRectangle(cornerRadius: 16).stroke(theme.colors.border, lineWidth: 1))
            .padding(.horizontal, 20)
            .padding(.top, 4)
            .padding(.bottom, 8)
            .accessibilityIdentifier("friendsMyRoomCard")
        }
    }

    // MARK: - 区块 3: tabBar (friends.jsx:47-57)

    /// 在线/全部 segmented control.
    private var tabBar: some View {
        HStack(spacing: 6) {
            ForEach(FriendsTab.allCases) { tab in
                tabButton(tab)
            }
            Spacer()
        }
        .padding(.horizontal, 20)
        .padding(.vertical, 6)
    }

    private func tabButton(_ tab: FriendsTab) -> some View {
        let isSelected = state.selectedTab == tab
        return Button(action: { state.selectTab(tab) }) {
            Text(tab.label)
                .font(.system(size: 12, weight: .heavy))
                .padding(.vertical, 7)
                .padding(.horizontal, 18)
                .foregroundColor(isSelected ? theme.colors.surface : theme.colors.inkSoft)
                .background(
                    RoundedRectangle(cornerRadius: 14)
                        .fill(isSelected ? theme.colors.ink : theme.colors.surface)
                )
                .overlay(
                    Group {
                        if !isSelected {
                            RoundedRectangle(cornerRadius: 14).stroke(theme.colors.border, lineWidth: 1)
                        }
                    }
                )
        }
        .accessibilityIdentifier("friendsTab_\(tab.rawValue)")
    }

    // MARK: - 区块 4 + 5: friendsList + FriendRow (friends.jsx:60-126)

    /// 好友列表 ScrollView + LazyVStack.
    private var friendsList: some View {
        ScrollView {
            LazyVStack(spacing: 8) {
                if state.displayedFriends.isEmpty {
                    Text("暂无好友在线～")
                        .font(.system(size: 13, weight: .semibold))
                        .foregroundColor(theme.colors.inkSoft.opacity(0.6))
                        .frame(maxWidth: .infinity)
                        .padding(40)
                } else {
                    ForEach(state.displayedFriends) { friend in
                        friendRow(friend)
                    }
                }
            }
            .padding(.horizontal, 20)
            .padding(.top, 8)
            .padding(.bottom, 100)  // 让出浮动 TabBar 空间
        }
    }

    /// 单条 FriendRow：Avatar + 信息 + 三态按钮（switch f.status）.
    private func friendRow(_ f: Friend) -> some View {
        HStack(spacing: 12) {
            Avatar(name: f.name, size: 48, color: f.color, online: f.online)

            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 4) {
                    Text(f.name)
                        .font(.system(size: 14, weight: .heavy))
                        .foregroundColor(theme.colors.ink)
                    if f.status == .inRoom {
                        Text("房间中")
                            .font(.system(size: 9, weight: .heavy))
                            .foregroundColor(theme.colors.accentDeep)
                            .padding(.vertical, 2)
                            .padding(.horizontal, 6)
                            .background(RoundedRectangle(cornerRadius: 6).fill(theme.colors.accentSoft))
                    }
                }
                Text(f.statusText)
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundColor(f.online ? theme.colors.inkSoft : theme.colors.inkSoft.opacity(0.5))
            }

            Spacer()

            // 三态按钮 switch f.status —— 让漏分支编译期报错（exhaustive enum）.
            actionButton(for: f)
        }
        .padding(12)
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
        .overlay(RoundedRectangle(cornerRadius: 18).stroke(theme.colors.border, lineWidth: 1))
        .accessibilityIdentifier("friendRow_\(f.id)")
    }

    @ViewBuilder
    private func actionButton(for f: Friend) -> some View {
        switch f.status {
        case .inRoom:
            Button(action: { state.onJoinFriendTap(friend: f) }) {
                HStack(spacing: 4) {
                    Image(systemName: Icons.symbol(for: "enter"))
                        .font(.system(size: 14))
                    Text("加入")
                        .font(.system(size: 12, weight: .heavy))
                }
                .padding(.vertical, 8)
                .padding(.horizontal, 14)
                .foregroundColor(.white)
                .background(RoundedRectangle(cornerRadius: 14).fill(theme.colors.accent))
            }
            .accessibilityIdentifier("friendActionButton_\(f.id)")
        case .online:
            Button(action: { state.onInviteFriendTap(friend: f) }) {
                Text("邀请")
                    .font(.system(size: 12, weight: .heavy))
                    .padding(.vertical, 8)
                    .padding(.horizontal, 14)
                    .foregroundColor(theme.colors.accentDeep)
                    .overlay(RoundedRectangle(cornerRadius: 14).stroke(theme.colors.accent, lineWidth: 1.5))
            }
            .accessibilityIdentifier("friendActionButton_\(f.id)")
        case .offline:
            Text("离线")
                .font(.system(size: 11, weight: .bold))
                .foregroundColor(theme.colors.inkSoft.opacity(0.5))
                .padding(.horizontal, 8)
        }
    }

    // MARK: - toastOverlay (本 story 新增；ui_design 无明确视觉)

    /// 占位 toast 视觉（仅 lastToastMessage 非 nil 时渲染）.
    /// 不实装"3 秒自动消失" timer —— 用户切 Tab / 触发其他动作隐式覆盖（详见 spec 关键决策 5）.
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
                .accessibilityIdentifier("friendsToast")
        }
    }
}

// MARK: - Previews

#if DEBUG
#Preview("FriendsScaffoldView — full mock / candy") {
    FriendsScaffoldView(state: MockFriendsViewModel())
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("FriendsScaffoldView — full mock / dark") {
    FriendsScaffoldView(state: MockFriendsViewModel())
        .environment(\.theme, ThemeName.dark.theme)
}

#Preview("FriendsScaffoldView — has my room / candy") {
    FriendsScaffoldView(state: MockFriendsViewModel(currentRoomId: "1234567"))
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("FriendsScaffoldView — empty friends / candy") {
    FriendsScaffoldView(state: MockFriendsViewModel(friends: []))
        .environment(\.theme, ThemeName.candy.theme)
}
#endif
