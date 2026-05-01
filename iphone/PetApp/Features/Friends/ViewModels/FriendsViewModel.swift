// FriendsViewModel.swift
// Story 37.10 AC1: FriendsScaffoldView 基类 ViewModel（class 层次 + 4 字段 + 2 abstract method + 1 concrete method）.
//
// 设计：与 HomeViewModel / RoomViewModel / WardrobeViewModel 同精神（class 而非 final + abstract method 用 fatalError 强制 override）.
// 字段范围：4 字段（friends / selectedTab / currentRoomId / lastToastMessage）.
// Story 37.12 RealFriendsViewModel 子类扩 onJoinFriendTap 调 JoinRoomUseCase / 走 setCurrentRoomId（不在本 story 范围）.
//
// import 备注（继承 Story 2.2 lesson 2026-04-25-swift-explicit-import-combine.md）：
// `ObservableObject` / `@Published` 来自 Combine，不能依赖 SwiftUI transitive import.

import Foundation
import Combine

@MainActor
public class FriendsViewModel: ObservableObject {
    /// 全部好友列表（mock 8 friend 三态混合；Mock 走 ScaffoldDefaults seed；Real 暂用同一 seed，
    /// 后续 epic 真接 server `/friends` 接口时改 sink 派生）.
    /// **关键约束**：friends 数据归本 ViewModel cache（**不进 AppState** —— 详见 epic AC line 4814 + ADR-0010 §3.2 表格
    /// "好友列表数据 → ViewModel 持有"; AppState 的 currentXxx 系列字段都是"本地用户的某条信息"语义，
    /// friends 是"别人的列表"，语义上不该进 AppState）.
    @Published public var friends: [Friend] = []

    /// 当前选中 Tab（在线 / 全部）；用户点 segmented control 切换.
    /// 这是 view-specific transient state（按 ADR-0010 §3.2 表格"当前选中" → ViewModel @Published）；
    /// 单元测试需要断言切换后 displayedFriends 派生改变（case#1）→ 不能放 SwiftUI @State.
    @Published public var selectedTab: FriendsTab = .online

    /// 当前房间号（"我的房间提示条"渲染依据；nil = 不渲染该 Card）.
    /// 派生源：appState.currentRoomId（RealFriendsViewModel 通过 sink 订阅派生；MockFriendsViewModel 用本地直写）.
    /// **关键约束**：currentRoomId 是 Wardrobe 域 catName 同精神的合法派生 —— "我的房间号"语义就是 appState.currentRoomId
    /// （与 Story 37.9 catName 派生自 currentPet 同合法理由：Friends 域的"我的房间"无歧义就是本地用户自己的房间；
    /// 与 Story 37.8 RoomViewModel.hostCatName 反例不冲突 —— 那是"看别人房间"语义）.
    @Published public var currentRoomId: String?

    /// 最近一次 toast 消息（占位 toast 系统；视觉走 FriendsScaffoldView 简单 overlay；详见 AC4 toast 渲染策略）.
    /// 用户可通过 selectTab / 触发其他动作隐式清空（写新值即覆盖；nil 表示无 toast）；
    /// 本 story 不实装"3 秒自动消失"等 timer 行为（保留给后续 epic）.
    @Published public var lastToastMessage: String?

    public init() {}

    // MARK: - abstract method（基类 fatalError 占位，子类必 override）

    /// "邀请"按钮回调（在线但未在房间的好友点按钮）.
    /// MockFriendsViewModel: 改本地 currentRoomId（若 nil → 设占位 "1234567"）+ 写 lastToastMessage + 记录 invocation.
    /// RealFriendsViewModel（本 story 占位）: **本地 mutate** —— currentRoomId nil 时设占位串 + 写 lastToastMessage,
    ///   非 nil 时仅写 lastToastMessage（"已邀请 {friend.name} 到房间 {currentRoomId}"）.
    ///   按 Story 37.9 round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`
    ///   钦定路径：Real 子类 override 必须实装本地 mutate state 让 production app 立即视觉反馈；不能只 log.
    /// Story 12.7+: RealFriendsViewModel 改调 CreateRoomUseCase + WS invitation 流程.
    public func onInviteFriendTap(friend: Friend) {
        fatalError("FriendsViewModel.onInviteFriendTap must be overridden by subclass")
    }

    /// "加入"按钮回调（friend.status == .inRoom 的好友点按钮）.
    /// MockFriendsViewModel: 改本地 currentRoomId = friend.currentRoomId + 写 lastToastMessage + 记录 invocation.
    /// RealFriendsViewModel（本 story 占位）: **本地 mutate** —— 若 friend.currentRoomId 非空,
    ///   通过 appState?.setCurrentRoomId(friend.currentRoomId) 写入 + 写 lastToastMessage（"加入 {friend.name} 的房间"）.
    ///   按 Story 37.9 round 1 P1 lesson 钦定路径.
    /// Story 37.12: RealFriendsViewModel 改调 JoinRoomUseCase + server 真实加入流程
    ///   （epic AC line 4859 钦定 FriendsScreen 直接调 JoinRoomUseCase，**不**弹 modal）.
    public func onJoinFriendTap(friend: Friend) {
        fatalError("FriendsViewModel.onJoinFriendTap must be overridden by subclass")
    }

    // MARK: - concrete view-action method（基类直接实装，子类不 override）

    /// 切换 Tab（用户点"在线"/"全部"调）.
    /// **不是** abstract —— 切换 Tab 是纯 view-state 行为，没有"Mock vs Real"分化需求.
    /// 副作用：仅写 selectedTab，**不**清 lastToastMessage（toast 与 tab 切换正交语义）.
    public func selectTab(_ tab: FriendsTab) {
        self.selectedTab = tab
    }

    /// myRoomCard "分享给好友" 占位按钮回调（fix-review round 1 加回 —— spec Dev Notes "myRoomCard 分享按钮决策" 钦定的 fallback 路径）.
    ///
    /// **不是** abstract —— 占位 toast 行为没有"Mock vs Real"分化（与本 epic 红线"UI Scaffold 数据完全 mock + 不调真实 share flow"一致；
    /// 让基类直接实装 = 主动规避 lesson `2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md` 反模式）.
    /// 行为：仅写 lastToastMessage —— 与 spec Dev Notes 钦定的"点击仅 print + 写 lastToastMessage '分享功能敬请期待'" 字面一致.
    /// 后续 epic（节点 12 真实分享流程落地）改为 abstract / 接 Share Sheet UseCase；本 story 不走 abstract path.
    public func onShareMyRoomTap() {
        self.lastToastMessage = "分享功能敬请期待"
    }

    // MARK: - derived helper（view 层方便用，子类不 override）

    /// 在线好友数（顶部 Card 显示用：「{onlineCount} 位在线 · 共 {friends.count} 位」）.
    public var onlineCount: Int {
        friends.filter { $0.online }.count
    }

    /// 在线好友列表（selectedTab == .online 时 displayedFriends 用）.
    public var onlineFriends: [Friend] {
        friends.filter { $0.online }
    }

    /// 全部好友列表（selectedTab == .all 时 displayedFriends 用，等价于 friends 全集）.
    public var allFriends: [Friend] {
        friends
    }

    /// 当前 Tab 显示的好友列表（list 渲染数据源；ui_design friends.jsx:5 filter 等价）.
    public var displayedFriends: [Friend] {
        switch selectedTab {
        case .online: return onlineFriends
        case .all:    return allFriends
        }
    }
}
