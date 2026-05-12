// RoomScaffoldView.swift
// Story 37.8 AC4: ui_design room.jsx 高保真房间界面 Scaffold（5 区块视觉 + 7 a11y 锚 + #Preview 4 配置）.
//
// 关键设计：
//   - struct 非泛型（与 HomeView<ChestSlot: View> 不同：Room 无 chestSlot 类似接缝点；
//     Story 12.1 / 35.2 真实接缝时再演进）
//   - `@ObservedObject var state: RoomViewModel` 基类直接（与 HomeView 同模式）
//   - 5 区块：topBar / roomCodeCard / sharedStage / membersList / leaveButton
//   - 复制 feedback 走 @State copiedFeedback（ADR-0010 §3.2 表格：纯本地视觉 transient + 无跨 View 触发场景；
//     与 Story 37.7 showJoinModal 走 ViewModel 决策反向）
//   - 7 个 a11y identifier inline 字符串：returnButton / roomIdDisplay / copyButton /
//     roomMember_0..3 / leaveButton（+ sharedStage 额外锚）；Story 37.13 a11y 总表归并
//   - 视觉规则：iphone/ui_design/source/screens/room.jsx + iphone/ui_design/README.md §RoomScreen
//
// 与 HomeView 同 lessons（应用 Story 37.7 5 轮 review 沉淀踩坑）：
//   - rapid copy tap 取消上一个 1.2s timer task（防 race；与 HomeView resetTask 同模式）
//   - 复制 feedback 派生靠 @State，timer 在 onChange 内 cancel + 新建（不在 ViewModel）
//   - MiniCat 弹跳动画用 @State 驱动 scaleEffect（不靠常量 offset；与 FloatingEmojiView lesson 同精神）

import SwiftUI

public struct RoomScaffoldView: View {
    @ObservedObject public var state: RoomViewModel

    /// Story 37.5: 主题 token 取值入口；RootView 注入 `.environment(\.theme, currentTheme.theme)`.
    @Environment(\.theme) private var theme

    /// 复制按钮 1.2s "已复制" 视觉反馈状态（本地 @State，不进 ViewModel —— 详见文件头决策注解）.
    @State private var copiedFeedback: Bool = false

    /// 复制 feedback timer 句柄（防多次点击 race；与 HomeView resetTask 同模式）.
    @State private var copyFeedbackTask: Task<Void, Never>?

    public init(state: RoomViewModel) {
        self.state = state
    }

    public var body: some View {
        ZStack {
            // 背景渐变（room.jsx:14 钦定 linear-gradient(180deg, accent-soft 0%, page-bg 45%)）.
            LinearGradient(
                colors: [theme.colors.accentSoft, theme.colors.pageBg],
                startPoint: .top,
                endPoint: .bottom
            )
            .ignoresSafeArea()

            ScrollView {
                VStack(spacing: theme.spacing.s14) {
                    topBar               // 区块 1: 返回按钮 + 标题
                    wsStateLabel         // Story 12.1 AC5: WS 连接态占位文字（"已连接 / 正在重连… / 已断开"）
                    roomCodeCard         // 区块 2: 房间号 + 复制按钮
                    sharedStage          // 区块 3: 共享舞台（粉橙渐变 + 装饰 + MiniCat 弹跳）
                    membersList          // 区块 4: 4 格成员列表
                    leaveButton          // 区块 5: 底部"离开房间" PrimaryButton
                }
                .padding(.horizontal, theme.spacing.s20)
                .padding(.top, 8)          // safe area top 已自动 respect；只补呼吸空间. 详见 HomeView.swift:90 注释.
                .padding(.bottom, 100)     // 浮动 TabBar 让出空间
            }
        }
    }

    // MARK: - 区块 1: topBar (room.jsx:18-31)

    /// 顶部返回按钮 + 标题 + 右侧 40pt 空白占位（Story 35.2 share button 留白契约）.
    private var topBar: some View {
        HStack(alignment: .center) {
            // 左：返回按钮（圆形 IconButton；调 state.onLeaveTap()）
            Button(action: { state.onLeaveTap() }) {
                Image(systemName: Icons.symbol(for: "back"))
                    .font(.system(size: 20, weight: .semibold))
                    .foregroundColor(theme.colors.ink)
                    .frame(width: 40, height: 40)
                    .background(Circle().fill(theme.colors.surface))
                    .overlay(Circle().stroke(theme.colors.border, lineWidth: 1))
                    .shadow(
                        color: theme.shadow.sm.color,
                        radius: theme.shadow.sm.radius,
                        x: theme.shadow.sm.x,
                        y: theme.shadow.sm.y
                    )
            }
            .accessibilityIdentifier(AccessibilityID.Room.returnButton)

            Spacer()

            // 中：标题 VStack（11pt 700 副标题 + 18pt 800 主标题）
            VStack(spacing: 0) {
                Text("队伍房间")
                    .font(.system(size: 11, weight: .bold))
                    .foregroundColor(theme.colors.inkSoft)
                    .tracking(0.5)
                Text("\(state.hostCatName)的小屋")
                    .font(.system(size: 18, weight: .heavy))
                    .foregroundColor(theme.colors.ink)
            }

            Spacer()

            // 右：40pt 空白占位（Story 35.2 share button 占位；详见 Dev Notes "顶部留白契约"）
            Color.clear.frame(width: 40, height: 40)
        }
        .padding(.top, 4)
    }

    // MARK: - Story 12.1 AC5: wsStateLabel (WS 连接态占位文字)

    /// WebSocket 连接态文字（"已连接 / 正在重连… / 已断开"）.
    /// 派生自 `state.wsState`；webSocketClient = nil 路径下显示"已断开"占位（AC4 关键决策 3）.
    /// accessibility identifier `wsStateLabel`（inline 字面量；Story 12.5 真实重连交互落地后再决定常量化收口至 `AccessibilityID.Room`）.
    private var wsStateLabel: some View {
        Text(wsStateText)
            .font(.system(size: 12, weight: .regular))
            .foregroundColor(theme.colors.inkSoft)
            .frame(maxWidth: .infinity, alignment: .center)
            .accessibilityIdentifier("wsStateLabel")
    }

    /// `state.wsState` 三态 → 占位文字派生.
    private var wsStateText: String {
        switch state.wsState {
        case .connected: return "已连接"
        case .reconnecting: return "正在重连…"
        case .disconnected: return "已断开"
        }
    }

    // MARK: - 区块 2: roomCodeCard (room.jsx:33-56)

    /// 房间号 Card：左 monospace 房间号 + 右 复制按钮（含 1.2s "已复制" feedback @State）.
    private var roomCodeCard: some View {
        Card(cornerRadius: 22, padding: 14) {
            HStack {
                VStack(alignment: .leading, spacing: 2) {
                    Text("房间代码")
                        .font(.system(size: 11, weight: .bold))
                        .foregroundColor(theme.colors.inkSoft)
                    Text(state.roomCodeForCopy)
                        .font(.system(size: 22, weight: .heavy, design: .monospaced))
                        .tracking(3)
                        .foregroundColor(theme.colors.accentDeep)
                        .accessibilityIdentifier(AccessibilityID.Room.roomIdDisplay)
                }
                Spacer()
                copyButton
            }
        }
    }

    /// 复制按钮：UIPasteboard.general.string 直接调（ViewModel 层不依赖 UIKit）+
    /// 启动 1.2s timer 切 copiedFeedback @State（rapid tap 取消上一个 task 防 race）.
    private var copyButton: some View {
        Button(action: {
            UIPasteboard.general.string = state.roomCodeForCopy
            state.onCopyTap()
            // 1.2s feedback timer（与 HomeView resetTask 同模式 cancel 上一个 task 防 race）
            copyFeedbackTask?.cancel()
            copiedFeedback = true
            copyFeedbackTask = Task { @MainActor in
                try? await Task.sleep(nanoseconds: 1_200_000_000)
                if Task.isCancelled { return }
                copiedFeedback = false
            }
        }) {
            HStack(spacing: 6) {
                Image(systemName: copiedFeedback ? Icons.symbol(for: "check") : Icons.symbol(for: "copy"))
                    .font(.system(size: 16, weight: .semibold))
                Text(copiedFeedback ? "已复制" : "复制")
                    .font(.system(size: 12, weight: .heavy))
            }
            .foregroundColor(copiedFeedback ? .white : theme.colors.accentDeep)
            .padding(.vertical, 10)
            .padding(.horizontal, 14)
            .background(
                RoundedRectangle(cornerRadius: 16)
                    .fill(copiedFeedback ? theme.colors.success : theme.colors.accentSoft)
            )
            .animation(.easeInOut(duration: 0.2), value: copiedFeedback)
        }
        .accessibilityIdentifier(AccessibilityID.Room.copyButton)
    }

    // MARK: - 区块 3: sharedStage (room.jsx:58-100)

    /// 粉橙渐变 Card（fixed colors，不走 theme — ui_design room.jsx:60 钦定）+ 4 emoji 装饰 +
    /// "X 只小猫在玩耍" pill + MiniCat HStack 错峰弹跳动画.
    private var sharedStage: some View {
        ZStack(alignment: .topLeading) {
            // 粉橙固定渐变背景
            LinearGradient(
                colors: [
                    Color(red: 1.00, green: 0.95, blue: 0.88),     // #fff2e0
                    Color(red: 1.00, green: 0.88, blue: 0.91),     // #ffe0e9
                ],
                startPoint: .top,
                endPoint: .bottom
            )

            // 底部 50pt 棕色光晕装饰（rgba(218,165,132,0→0.25) 渐变）
            VStack {
                Spacer()
                LinearGradient(
                    colors: [
                        Color(red: 0.85, green: 0.65, blue: 0.52, opacity: 0),
                        Color(red: 0.85, green: 0.65, blue: 0.52, opacity: 0.25),
                    ],
                    startPoint: .top,
                    endPoint: .bottom
                )
                .frame(height: 50)
            }

            // 4 个固定位 emoji 装饰
            sharedStageEmojiDecorations

            // 主内容 VStack
            VStack(alignment: .leading, spacing: 10) {
                // "X 只小猫在玩耍" pill
                Text("\(state.members.count) 只小猫在玩耍")
                    .font(.system(size: 11, weight: .bold))
                    .foregroundColor(theme.colors.inkSoft)
                    .padding(.vertical, 4)
                    .padding(.horizontal, 10)
                    .background(RoundedRectangle(cornerRadius: 10).fill(Color.white.opacity(0.7)))

                // MiniCat HStack
                HStack(alignment: .bottom, spacing: 8) {
                    ForEach(Array(state.members.enumerated()), id: \.element.id) { index, member in
                        miniCat(member: member, index: index)
                    }
                }
                .padding(.vertical, 14)
                .frame(maxWidth: .infinity)
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 16)
        }
        .frame(minHeight: 260)
        .clipShape(RoundedRectangle(cornerRadius: 28))
        .overlay(RoundedRectangle(cornerRadius: 28).stroke(theme.colors.border, lineWidth: 1))
        .shadow(
            color: theme.shadow.md.color,
            radius: theme.shadow.md.radius,
            x: theme.shadow.md.x,
            y: theme.shadow.md.y
        )
        .accessibilityIdentifier(AccessibilityID.Room.sharedStage)
    }

    /// 4 个固定位 emoji 装饰（room.jsx:62-79）.
    /// 用 GeometryReader 替代 UIScreen.main.bounds（避免 deprecated API + 适配 SwiftUI 布局上下文）.
    private var sharedStageEmojiDecorations: some View {
        GeometryReader { geo in
            Text("🧶")
                .font(.system(size: 32))
                .position(x: geo.size.width - 30, y: 220)
            Text("🐟")
                .font(.system(size: 28))
                .position(x: 30, y: 220)
            Text("☁️")
                .font(.system(size: 22))
                .opacity(0.6)
                .position(x: geo.size.width - 25, y: 30)
            Text("☁️")
                .font(.system(size: 18))
                .opacity(0.5)
                .position(x: 40, y: 50)
        }
    }

    /// MiniCat 子视图（错峰 0.2s 弹跳；用 @State 驱动 scaleEffect 简化版）.
    /// 视觉精度由 Story 37.13 visual-review-checklist 把关；不严格匹配 room.jsx keyframes.
    @ViewBuilder
    private func miniCat(member: RoomMember, index: Int) -> some View {
        MiniCatView(member: member, index: index, theme: theme)
    }

    // MARK: - 区块 4: membersList (room.jsx:102-137)

    /// 成员列表（4 格；按 index 0-3 编号 a11y identifier roomMember_0..3）.
    private var membersList: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("成员 (\(state.members.count)/4)")
                .font(.system(size: 13, weight: .heavy))
                .foregroundColor(theme.colors.ink)
                .padding(.horizontal, 4)

            VStack(spacing: 8) {
                ForEach(Array(state.members.enumerated()), id: \.element.id) { index, member in
                    memberRow(member: member, index: index)
                }
                // 空位（4 - members.count 个）
                if state.members.count < 4 {
                    ForEach(state.members.count..<4, id: \.self) { index in
                        emptySlot(index: index)
                    }
                }
            }
        }
    }

    /// 已加入成员行（Avatar + 名字 + 队长 tag + Lv.x · status 副标题 + PetSpriteView）.
    ///
    /// Story 15.1 AC2：成员行右侧渲染 `PetSpriteView(state:)`，用于直观显示该成员的实时猫状态
    /// （rest / walk / run）. 数据源：`state.memberPetStates[member.id]`（Story 15.1 在 applySnapshot /
    /// applyMemberJoined 中真实写入；缺失时 fallback `.rest`）→ 经 `HomePetState.motionState` 桥接
    /// 后传给 PetSpriteView. 替换原本的 paw icon（与该行的功能位置等价；avoid 增加新位置打破 5 区块视觉契约）.
    ///
    /// 尺寸缩小：PetSpriteView 内部 frame 是 180×180pt（HomeView catStage 尺寸），房间成员行空间有限,
    /// 用外层 `.frame(width: 40, height: 40)` 覆盖（SwiftUI frame modifier 在最后应用的优先）.
    ///
    /// a11y identifier：PetSpriteView 自带 `petSprite_rest / walk / run`（Story 8.4 钦定，AccessibilityID.Home
    /// 内常量）；本 story UITest（AC4）注入 3 个 fixed members 各持不同 state，3 个 identifier 各自唯一可定位.
    private func memberRow(member: RoomMember, index: Int) -> some View {
        HStack(spacing: 12) {
            Avatar(name: member.name, size: 40)
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 4) {
                    Text(member.name)
                        .font(.system(size: 14, weight: .heavy))
                        .foregroundColor(theme.colors.ink)
                    if member.isHost {
                        Text("队长")
                            .font(.system(size: 10, weight: .heavy))
                            .foregroundColor(theme.colors.accentDeep)
                            .padding(.vertical, 2)
                            .padding(.horizontal, 6)
                            .background(RoundedRectangle(cornerRadius: 8).fill(theme.colors.accentSoft))
                    }
                }
                Text("小猫 Lv.\(member.level) · \(member.status)")
                    .font(.system(size: 11, weight: .semibold))
                    .foregroundColor(theme.colors.inkSoft)
            }
            Spacer()
            // Story 15.1 AC2: PetSpriteView 渲染成员当前猫状态（替换原 paw icon）.
            // member.id 不在 memberPetStates 中时 fallback `.rest`（防御性；applySnapshot 会兜底，
            // 但 RoomScaffoldDefaults seed 路径下 memberPetStates 仍是空 map，fallback 让初始帧不空）.
            //
            // Story 15.1 review r1 fix：用 `size: 40` 让 PetSpriteView 真正按 40pt 渲染；
            // 之前 `.frame(width: 40, height: 40).clipped()` 只裁不缩 → SF Symbol 180×180
            // 被裁成"截断的猫头"。详见 docs/lessons/2026-05-12-swiftui-frame-clipped-does-not-scale.md.
            PetSpriteView(
                state: (state.memberPetStates[member.id] ?? .rest).motionState,
                size: 40
            )
            .frame(width: 40, height: 40)
        }
        .padding(10)
        .background(RoundedRectangle(cornerRadius: 16).fill(theme.colors.surface))
        .overlay(RoundedRectangle(cornerRadius: 16).stroke(theme.colors.border, lineWidth: 1))
        .shadow(
            color: theme.shadow.sm.color,
            radius: theme.shadow.sm.radius,
            x: theme.shadow.sm.x,
            y: theme.shadow.sm.y
        )
        // Story 15.1 AC4: 必须用 `.accessibilityElement(children: .contain)`，否则父层
        // `.accessibilityIdentifier(roomMember_N)` 会把 PetSpriteView 的 a11y leaf（带
        // petSprite_rest/walk/run identifier）合并掉，UITest 无法定位三态 sprite identifier.
        // 与 HomeView.catStage（line 288-289）同精神（同样 `.accessibilityElement(children: .contain)`
        // + `.accessibilityIdentifier(...)` 组合让 catStage 父 identifier 与 PetSpriteView 子 identifier 共存）.
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(AccessibilityID.Room.member(at: index))
    }

    /// 空位 dashed border 行（"+ 等待好友加入"）.
    private func emptySlot(index: Int) -> some View {
        HStack {
            Text("+ 等待好友加入")
                .font(.system(size: 13, weight: .heavy))
                .foregroundColor(theme.colors.inkSoft.opacity(0.6))
        }
        .frame(maxWidth: .infinity)
        .frame(height: 60)
        .overlay(
            RoundedRectangle(cornerRadius: 16)
                .stroke(
                    theme.colors.border,
                    style: StrokeStyle(lineWidth: 2, dash: [6, 4])
                )
        )
        .accessibilityIdentifier(AccessibilityID.Room.member(at: index))
    }

    // MARK: - 区块 5: leaveButton (room.jsx:139-147)

    /// 底部"离开房间" PrimaryButton（secondary variant + fullWidth；调 state.onLeaveTap()）.
    private var leaveButton: some View {
        PrimaryButton(
            title: "离开房间",
            variant: .secondary,
            fullWidth: true,
            action: { state.onLeaveTap() }
        )
        .accessibilityIdentifier(AccessibilityID.Room.leaveButton)
        .padding(.top, 4)
    }
}

// MARK: - MiniCatView 子视图（错峰弹跳动画用 @State 驱动 scaleEffect）

/// MiniCat 子视图：圆形猫占位 + 名字 + 错峰 0.2s 弹跳动画.
///
/// 抽成独立 View 是为了让 `@State` bouncing 跟随子视图 identity 重建逻辑（与 FloatingEmojiView lesson 同精神）：
///   - 每个 MiniCat 用自己的 @State + .onAppear 启动 .repeatForever 动画
///   - delay = index * 0.2s 实现 4 只猫错峰
///   - 不靠常量 offset 或父级 ID 强制重建（与 round 5 P2 lesson "FloatingEmojiView 必须 @State 驱动" 同精神）
struct MiniCatView: View {
    let member: RoomMember
    let index: Int
    let theme: Theme

    /// 弹跳动画 scale 状态（0.94 ↔ 1.0）；.onAppear 后 .repeatForever 驱动.
    @State private var bouncing: Bool = false

    /// 4 色调色板 hash（不走 theme — ui_design room.jsx 钦定 fixed palette）.
    private static let palette: [Color] = [
        Color(red: 1.00, green: 0.84, blue: 0.87),  // #ffd6df
        Color(red: 0.87, green: 0.91, blue: 0.78),  // #dfe8c8
        Color(red: 0.81, green: 0.89, blue: 0.95),  // #cfe2f2
        Color(red: 0.96, green: 0.83, blue: 0.65),  // #f5d4a6
    ]

    var body: some View {
        VStack(spacing: 4) {
            Circle()
                .fill(Self.palette[index % Self.palette.count])
                .frame(width: 68, height: 68)
                .overlay(
                    Image(systemName: "cat.fill")
                        .font(.system(size: 28))
                        .foregroundColor(theme.colors.inkSoft)
                )
                .scaleEffect(bouncing ? 1.0 : 0.94)
            Text(member.name)
                .font(.system(size: 10, weight: .bold))
                .foregroundColor(theme.colors.ink)
        }
        .onAppear {
            // delay 实现错峰；.repeatForever autoreverses 来回弹跳.
            withAnimation(
                .easeInOut(duration: 1.1)
                    .repeatForever(autoreverses: true)
                    .delay(Double(index) * 0.2)
            ) {
                bouncing = true
            }
        }
    }
}

// MARK: - Preview (AC6: #Preview 4 配置 candy/dark x 4/2/1 成员)

#if DEBUG
#Preview("RoomScaffoldView — 4 members / candy") {
    RoomScaffoldView(state: MockRoomViewModel())
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("RoomScaffoldView — 4 members / dark") {
    RoomScaffoldView(state: MockRoomViewModel())
        .environment(\.theme, ThemeName.dark.theme)
}

#Preview("RoomScaffoldView — 2 members + 2 empty / candy") {
    RoomScaffoldView(state: MockRoomViewModel(members: MockRoomViewModel.twoMembersMock))
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("RoomScaffoldView — host alone / candy") {
    RoomScaffoldView(state: MockRoomViewModel(
        members: [RoomMember(id: "u1", name: "小花", level: 8, status: "在玩耍", isHost: true)]
    ))
    .environment(\.theme, ThemeName.candy.theme)
}
#endif
