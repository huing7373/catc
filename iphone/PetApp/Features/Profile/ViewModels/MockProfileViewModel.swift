// MockProfileViewModel.swift
// Story 37.11 AC2: ProfileViewModel mock 子类，用于 #Preview / UITest skip-guest-login / Scaffold 单元测试.
//
// 设计：
//   - 硬编码 mock 数据（profile / wechatBound / recentCollections / showBindModal 全量；走 ProfileScaffoldDefaults seed）
//   - override 5 个 abstract method 改本地状态 + 记录 invocations 数组
//   - **不**依赖 AppState（Mock 路径走纯 ViewModel-only 数据）
//
// 与 MockHomeViewModel Story 37.7 / MockRoomViewModel Story 37.8 / MockWardrobeViewModel Story 37.9 /
// MockFriendsViewModel Story 37.10 同模式（invocations 数组而非 closure spy）.

import Foundation
import Combine
import os.log

@MainActor
public final class MockProfileViewModel: ProfileViewModel {
    /// 单元测试用：记录所有方法调用
    public enum Invocation: Equatable {
        case wechatCardTap
        case wechatBindConfirmTap
        case wechatModalDismissTap
        case menuTap(item: ProfileMenuItem)
        case collectionViewAllTap
        /// Story 37.11 round 3 codex review [P2] 新增：bell / settings header 按钮.
        case bellTap
        case settingsTap
    }

    @Published public var invocations: [Invocation] = []

    /// 默认构造 — 走 ProfileScaffoldDefaults seed 全量字段.
    public override init() {
        super.init()
        self.profile = ProfileScaffoldDefaults.profile
        self.wechatBound = ProfileScaffoldDefaults.wechatBound  // false
        self.recentCollections = ProfileScaffoldDefaults.recentCollections
        self.showBindModal = false
        self.lastToastMessage = nil
    }

    /// 测试 / Preview 灵活构造 — 可注入任意字段值.
    public init(
        profile: ProfileSummary = ProfileScaffoldDefaults.profile,
        wechatBound: Bool = ProfileScaffoldDefaults.wechatBound,
        recentCollections: [RecentCollection] = ProfileScaffoldDefaults.recentCollections,
        showBindModal: Bool = false
    ) {
        super.init()
        self.profile = profile
        self.wechatBound = wechatBound
        self.recentCollections = recentCollections
        self.showBindModal = showBindModal
        self.lastToastMessage = nil
    }

    // MARK: - override abstract methods

    public override func onWeChatCardTap() {
        os_log(.debug, "MockProfileViewModel.onWeChatCardTap")
        invocations.append(.wechatCardTap)
        showBindModal = true
    }

    public override func onWeChatBindConfirmTap() {
        os_log(.debug, "MockProfileViewModel.onWeChatBindConfirmTap")
        invocations.append(.wechatBindConfirmTap)
        // Mock 路径行为：写 wechatBound = true + 关 modal + toast
        wechatBound = true
        showBindModal = false
        lastToastMessage = "微信绑定成功（mock）"
    }

    public override func onWeChatModalDismissTap() {
        os_log(.debug, "MockProfileViewModel.onWeChatModalDismissTap")
        invocations.append(.wechatModalDismissTap)
        showBindModal = false
    }

    public override func onMenuTap(item: ProfileMenuItem) {
        os_log(.debug, "MockProfileViewModel.onMenuTap %{public}@", item.rawValue)
        invocations.append(.menuTap(item: item))
        lastToastMessage = "\(item.label)（敬请期待）"
    }

    public override func onCollectionViewAllTap() {
        os_log(.debug, "MockProfileViewModel.onCollectionViewAllTap")
        invocations.append(.collectionViewAllTap)
        lastToastMessage = "查看全部收藏（敬请期待）"
    }

    /// Story 37.11 round 3 codex review [P2] 修复：bell 按钮通过 ViewModel seam.
    public override func onBellTap() {
        os_log(.debug, "MockProfileViewModel.onBellTap")
        invocations.append(.bellTap)
        lastToastMessage = "消息中心（敬请期待）"
    }

    /// Story 37.11 round 3 codex review [P2] 修复：settings 按钮通过 ViewModel seam.
    public override func onSettingsTap() {
        os_log(.debug, "MockProfileViewModel.onSettingsTap")
        invocations.append(.settingsTap)
        lastToastMessage = "设置（敬请期待）"
    }
}
