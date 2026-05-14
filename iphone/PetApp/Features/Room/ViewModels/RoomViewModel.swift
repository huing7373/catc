// RoomViewModel.swift
// Story 37.8 AC1: RoomScaffoldView 基类 ViewModel（class 层次 + 4 字段 + 2 abstract method）.
// Story 12.1 AC1: 扩 2 个 @Published 字段（wsState / memberPetStates）让基类一次容纳所有
//   RealRoomViewModel 真实 WS 实装会写入的状态字段；MockRoomViewModel 仍可任意 set 用于测试 / Preview.
// Story 18.2 AC1: 扩 2 个 @Published 字段（showEmojiPanel / currentUserId）+ 1 abstract method
//   （onOwnPetTap）支持"点击自己猫触发表情面板"路径.
// Story 18.3 AC4: 扩 1 个 @Published 字段（activeEmojis）+ 1 abstract method（onEmojiSelected）
//   支持"选中表情 → 本地立即动效 + WS fire-and-forget"路径.
//
// 设计：与 HomeViewModel 同精神（class 而非 final + abstract method 用 fatalError 强制 override）.
// 字段范围（Story 18.3 后）：9 字段
//   - roomCodeForCopy / hostCatName / members / userIsHost（Story 37.8 落地）
//   - wsState / memberPetStates（Story 12.1 落地；RealRoomViewModel WS 路径写入）
//   - showEmojiPanel / currentUserId（Story 18.2 落地；表情面板 sheet 双向绑定 + self/other 判定 SoT）
//   - activeEmojis（Story 18.3 落地；本地动效 + 18.4 接收动效共用 transient 队列）.
// abstract method 范围（Story 18.3 后）：4 abstract method
//   - onLeaveTap / onCopyTap（Story 37.8 落地）
//   - onOwnPetTap（Story 18.2 落地）
//   - onEmojiSelected（Story 18.3 落地；选中表情入口）.
//
// import 备注（继承 Story 2.2 lesson 2026-04-25-swift-explicit-import-combine.md）：
// `ObservableObject` / `@Published` 来自 Combine，不能依赖 SwiftUI transitive import.

import Foundation
import Combine

@MainActor
public class RoomViewModel: ObservableObject {
    /// 房间代码（mock "1234567"；Story 12.1 RealRoomViewModel 从 appState.currentRoomId 派生）.
    @Published public var roomCodeForCopy: String = ""

    /// 房主猫名（mock "小花"；用于顶部"{猫名}的小屋"标题；Story 12.1 后从 hostMember 派生）.
    @Published public var hostCatName: String = ""

    /// 成员数组（mock 0-4 成员；Story 12.1 后从 WS room.snapshot 派生）.
    @Published public var members: [RoomMember] = []

    /// 当前用户是否为房主（mock false；Story 12.1 后从 user.id == host.id 派生）.
    /// 本 story 仅声明字段，不在视觉中区分（队长标签由 RoomMember.isHost 标记，不依赖此字段）.
    @Published public var userIsHost: Bool = false

    /// Story 12.1 AC1: WebSocket 连接状态（connected / reconnecting / disconnected 三态枚举）.
    /// 默认 `.disconnected`：RealRoomViewModel 在 connect 成功后切 connected；
    /// Story 12.5 后 reconnect 中切 reconnecting；本 story 仅 connect/disconnect 两态切换.
    @Published public var wsState: WSState = .disconnected

    /// Story 12.1 AC1: 成员宠物状态映射（节点 5 后真实启用，节点 4 阶段保持空 map）.
    /// key = userId（String）；value = HomePetState；用于房间页 4 格成员渲染时取每个成员的 currentState.
    /// 节点 4 阶段 server `room.snapshot` 下发 `payload.members[].pet.currentState` 固定 `1`（rest）,
    /// 因此本字段在节点 4 阶段下永远是空 map（解析 snapshot 时**不**写入；待 Epic 14 真实驱动）；
    /// 但**字段必须就位**，否则 Story 14.x / 15.x 落地时还要回工 RealRoomViewModel.
    @Published public var memberPetStates: [String: HomePetState] = [:]

    /// Story 18.2: EmojiPanelView sheet 双向绑定状态.
    /// 视图层 `RoomScaffoldView.sheet(isPresented: $state.showEmojiPanel)` 双向绑定.
    /// - true: 自己 PetSpriteView Button 被点 → onOwnPetTap() 设 → sheet 弹出
    /// - false: SwiftUI swipe-down dismiss 自动设 / onSelect 闭包选中表情后显式设
    /// 唯一 owner = ViewModel @Published (避免 SwiftUI @State 双写漂移; ADR-0010 §3.2 钦定).
    /// 与 HomeViewModel.showJoinModal 同模式.
    @Published public var showEmojiPanel: Bool = false

    /// Story 18.2: 当前用户的 userId (self vs other 判定的 single source of truth).
    /// - RealRoomViewModel: 订阅 `appState.$currentUser.map { $0?.id }.removeDuplicates()` 派生
    /// - MockRoomViewModel: 通过 init 参数 currentUserId 注入 (默认 "u1" 与 RoomScaffoldDefaults.members[0].id 对齐)
    /// - View 层 `memberRow` 内 `member.id == state.currentUserId` 区分自己行 / 他人行
    /// nil 语义: appState.currentUser 尚未 hydrate / 已 reset; 此时所有成员行均**不**渲染 Button (防御性 fail-safe).
    @Published public var currentUserId: String? = nil

    /// Story 18.3 AC4: 房间内 transient 表情动效队列 (self + others 共用; ADR-0010 §3.2 transient UI state).
    /// - 18.3 路径: vm.onEmojiSelected(code:) 内入队 (self 触发)
    /// - 18.4 路径: applyEmojiReceived(payload:) 内入队 (others 触发; self echo 跳过去重)
    /// - 视图层 RoomScaffoldView 内 EmojiAnimationLayer ForEach 渲染 (18.3 占位; 18.4 替换为完整动画)
    /// - 18.4 落地 1.5s 后按 createdAt 自动 expire 移除; 本 story (18.3) **不**实装移除路径
    /// 唯一 owner = ViewModel @Published; SwiftUI 通过 @ObservedObject state 间接订阅.
    @Published public var activeEmojis: [RoomActiveEmoji] = []

    public init() {}

    // MARK: - abstract method（基类 fatalError 占位，子类必 override）

    /// 离开房间按钮回调（顶部返回按钮 / 底部"离开房间" PrimaryButton 共用同一回调）.
    /// MockRoomViewModel: 记录 invocation + print log.
    /// RealRoomViewModel（Story 12.1+）: 调 LeaveRoomUseCase + appState.setCurrentRoomId(nil).
    public func onLeaveTap() {
        fatalError("RoomViewModel.onLeaveTap must be overridden by subclass")
    }

    /// 复制房间号按钮回调.
    /// MockRoomViewModel: 记录 invocation + print log（视图内 1.2s "已复制" feedback 由 SwiftUI @State 本地持有）.
    /// RealRoomViewModel: 调 UIPasteboard.general.string = roomCodeForCopy + 同 mock 行为.
    public func onCopyTap() {
        fatalError("RoomViewModel.onCopyTap must be overridden by subclass")
    }

    /// Story 18.2: 自己 PetSpriteView Button 点击回调.
    /// MockRoomViewModel: 记录 invocation + showEmojiPanel = true.
    /// RealRoomViewModel: showEmojiPanel = true (不调任何 server; 18.3 才在 onSelect 闭包内调 SendEmojiUseCase).
    public func onOwnPetTap() {
        fatalError("RoomViewModel.onOwnPetTap must be overridden by subclass")
    }

    /// Story 18.3 AC4: 用户从 EmojiPanelView 选中表情 cell 后的回调入口.
    /// 触发路径: RoomScaffoldView .sheet onSelect 闭包 → `state.onEmojiSelected(code: code)`.
    /// 子类 override 行为:
    ///   - MockRoomViewModel: 入队 activeEmojis + invocations 记录
    ///   - RealRoomViewModel: 入队 activeEmojis (本地立即动效) + V1 §12.2 缓存校验 + 调 SendEmojiUseCase
    ///     fire-and-forget (Task 包裹, 不阻塞主线程; 失败弹 toast 不回滚动效)
    public func onEmojiSelected(code: String) {
        fatalError("RoomViewModel.onEmojiSelected must be overridden by subclass")
    }
}
