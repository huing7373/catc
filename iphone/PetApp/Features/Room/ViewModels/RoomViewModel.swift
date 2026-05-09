// RoomViewModel.swift
// Story 37.8 AC1: RoomScaffoldView 基类 ViewModel（class 层次 + 4 字段 + 2 abstract method）.
// Story 12.1 AC1: 扩 2 个 @Published 字段（wsState / memberPetStates）让基类一次容纳所有
//   RealRoomViewModel 真实 WS 实装会写入的状态字段；MockRoomViewModel 仍可任意 set 用于测试 / Preview.
//
// 设计：与 HomeViewModel 同精神（class 而非 final + abstract method 用 fatalError 强制 override）.
// 字段范围（Story 12.1 后）：6 字段
//   - roomCodeForCopy / hostCatName / members / userIsHost（Story 37.8 落地）
//   - wsState / memberPetStates（Story 12.1 落地；RealRoomViewModel WS 路径写入）.
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
}
