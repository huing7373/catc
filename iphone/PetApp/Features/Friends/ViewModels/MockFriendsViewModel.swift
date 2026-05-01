// MockFriendsViewModel.swift
// Story 37.10 AC2: FriendsViewModel mock 子类，用于 #Preview / UITest skip-guest-login / Scaffold 单元测试.
//
// 设计：
//   - 硬编码 mock 数据（friends 8 件 / selectedTab / currentRoomId 全量；走 FriendsScaffoldDefaults seed）
//   - override 2 个 abstract method（onInviteFriendTap / onJoinFriendTap）改本地状态 + 记录 invocations 数组
//   - **不**依赖 AppState（Mock 路径走纯 ViewModel-only 数据）
//
// 与 MockHomeViewModel Story 37.7 / MockRoomViewModel Story 37.8 / MockWardrobeViewModel Story 37.9 同模式（invocations 数组而非 closure spy）.

import Foundation
import Combine
import os.log

@MainActor
public final class MockFriendsViewModel: FriendsViewModel {
    /// 单元测试用：记录所有方法调用
    public enum Invocation: Equatable {
        case inviteTap(friendId: String)
        case joinTap(friendId: String)
    }

    @Published public var invocations: [Invocation] = []

    /// 默认构造 — 走 FriendsScaffoldDefaults seed 全量字段.
    public override init() {
        super.init()
        self.friends = FriendsScaffoldDefaults.friends
        self.selectedTab = FriendsScaffoldDefaults.selectedTab
        self.currentRoomId = FriendsScaffoldDefaults.currentRoomId   // nil
        self.lastToastMessage = nil
    }

    /// 测试 / Preview 灵活构造 — 可注入任意 friends / selectedTab / currentRoomId.
    public init(
        friends: [Friend] = FriendsScaffoldDefaults.friends,
        selectedTab: FriendsTab = FriendsScaffoldDefaults.selectedTab,
        currentRoomId: String? = FriendsScaffoldDefaults.currentRoomId
    ) {
        super.init()
        self.friends = friends
        self.selectedTab = selectedTab
        self.currentRoomId = currentRoomId
        self.lastToastMessage = nil
    }

    // MARK: - override abstract methods

    public override func onInviteFriendTap(friend: Friend) {
        os_log(.debug, "MockFriendsViewModel.onInviteFriendTap %{public}@", friend.id)
        invocations.append(.inviteTap(friendId: friend.id))
        // Mock 路径行为（与 epic AC line 4812 钦定一致）：
        //   - 若 currentRoomId nil → 触发"创建队伍 mock"（设占位 currentRoomId）+ toast "已邀请..."
        //   - 若 currentRoomId 非 nil → 仅 toast "已邀请..."（不再创建）
        if currentRoomId == nil {
            currentRoomId = "1234567"   // 占位"创建队伍"mock；与 RoomScaffoldDefaults 占位风格一致
            lastToastMessage = "已创建队伍并邀请 \(friend.name)"
        } else {
            lastToastMessage = "已邀请 \(friend.name) 加入房间 \(currentRoomId ?? "?")"
        }
    }

    public override func onJoinFriendTap(friend: Friend) {
        os_log(.debug, "MockFriendsViewModel.onJoinFriendTap %{public}@", friend.id)
        invocations.append(.joinTap(friendId: friend.id))
        // Mock 路径行为：解析 friend.currentRoomId 作为目标房间号 + 直接 mutate currentRoomId（不弹 modal —— epic AC line 4859 钦定）.
        guard let targetRoomId = friend.currentRoomId, !targetRoomId.isEmpty else {
            lastToastMessage = "好友不在房间中"
            return
        }
        currentRoomId = targetRoomId
        lastToastMessage = "加入 \(friend.name) 的房间 \(targetRoomId)"
    }
}
