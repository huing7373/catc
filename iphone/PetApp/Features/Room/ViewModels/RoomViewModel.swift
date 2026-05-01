// RoomViewModel.swift
// Story 37.8 AC1: RoomScaffoldView 基类 ViewModel（class 层次 + 4 字段 + 2 abstract method）.
//
// 设计：与 HomeViewModel 同精神（class 而非 final + abstract method 用 fatalError 强制 override）.
// 字段范围：仅 4 字段（roomCodeForCopy / hostCatName / members / userIsHost）.
// 节点 4 后 Story 12.1 RealRoomViewModel 子类扩 wsState / memberPetStates 字段（不在本 story 范围）.
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
