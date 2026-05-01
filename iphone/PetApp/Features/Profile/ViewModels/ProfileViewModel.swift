// ProfileViewModel.swift
// Story 37.11 AC1: ProfileScaffoldView 基类 ViewModel（class 层次 + 5 字段 + 5 abstract method + 0 concrete method）.
//
// 设计：与 HomeViewModel / RoomViewModel / WardrobeViewModel / FriendsViewModel 同精神
//   （class 而非 final + abstract method 用 fatalError 强制 override）.
// 字段范围：5 字段（profile / wechatBound / recentCollections / showBindModal / lastToastMessage）.
// 后续 epic RealProfileViewModel 子类扩 onWeChatBindConfirmTap 调真实微信 OAuth UseCase / 走 setWeChatBound（不在本 story 范围）.
//
// import 备注（继承 Story 2.2 lesson 2026-04-25-swift-explicit-import-combine.md）：
// `ObservableObject` / `@Published` 来自 Combine，不能依赖 SwiftUI transitive import.

import Foundation
import Combine

@MainActor
public class ProfileViewModel: ObservableObject {
    /// 用户聚合资料卡（顶部渐变头图 + 统计卡共用数据源）.
    /// 派生源：appState.currentUser + appState.currentPet（RealProfileViewModel 通过 sink 订阅派生；MockProfileViewModel 用本地直写）.
    /// **关键约束**：profile 是 view-specific aggregated value type（**不进 AppState** —— 详见 ADR-0010 §3.2 表格
    /// "聚合卡片字段 → ViewModel 持有"; AppState 的 currentUser / currentPet 是 raw domain state，
    /// profile 是 ViewModel 视图聚合，两者职责分离；与 Story 37.10 friends 字段语义同精神）.
    @Published public var profile: ProfileSummary = ProfileScaffoldDefaults.profile

    /// 微信绑定状态（决定"绑定微信卡"渲染未绑定/已绑定双态分支）.
    /// 后续 epic 真实持久化（如 @AppStorage / server 字段）落地时改派生源；本期 ViewModel 持有.
    @Published public var wechatBound: Bool = false

    /// 最近收藏横向滑窗数据（mock 5 件最近开箱）.
    /// 后续 epic 真接 LoadRecentCollectionsUseCase 时改 sink 派生；本期 ScaffoldDefaults seed.
    @Published public var recentCollections: [RecentCollection] = []

    /// "绑定微信 Modal" 显隐状态（用户点"绑定微信卡"或"立即绑定"按钮触发显示）.
    /// 这是 view-specific transient state（按 ADR-0010 §3.2 表格"modal 显隐 → ViewModel @Published"）；
    /// 单元测试需要断言（case#2 卡点击后 showBindModal == true）→ 不能放 SwiftUI @State.
    @Published public var showBindModal: Bool = false

    /// 最近一次 toast 消息（占位 toast 系统；视觉走 ProfileScaffoldView 简单 overlay；详见 AC4 toast 渲染策略）.
    /// 用户可通过其他动作隐式清空（写新值即覆盖；nil 表示无 toast）；
    /// 本 story 不实装"3 秒自动消失"等 timer 行为（保留给后续 epic；与 Story 37.10 同精神）.
    @Published public var lastToastMessage: String?

    public init() {}

    // MARK: - abstract method（基类 fatalError 占位，子类必 override）

    /// "绑定微信卡"整张卡可点击触发（未绑定状态下点卡触发 modal 弹出）.
    /// MockProfileViewModel: 写 showBindModal = true + 记录 invocation.
    /// RealProfileViewModel（本 story 占位）: **本地 mutate** —— 写 showBindModal = true + 记录 lastToastMessage（可选）.
    ///   按 Story 37.9 round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`
    ///   钦定路径：Real 子类 override 必须实装本地 mutate state 让 production app 立即视觉反馈；不能只 log.
    /// 后续 epic: RealProfileViewModel 改调"显示 Modal" + 接入 @AppStorage("lastWechatPromptAt") 24 小时再次弹一次逻辑.
    public func onWeChatCardTap() {
        fatalError("ProfileViewModel.onWeChatCardTap must be overridden by subclass")
    }

    /// Modal 内"绑定微信，保护数据"按钮触发.
    /// MockProfileViewModel: 写 wechatBound = true + showBindModal = false + lastToastMessage = "微信绑定成功（mock）" + 记录 invocation.
    /// RealProfileViewModel（本 story 占位）: **本地 mutate** —— 写 wechatBound = true + showBindModal = false + lastToastMessage = "微信绑定（敬请期待）".
    ///   按 Story 37.9 round 1 P1 lesson 钦定路径.
    /// 后续 epic: 改调 BindWechatUseCase / WXApi.sendAuthReq 拉起授权 → 后端换 OpenID → server 写入 → 成功后 setWeChatBound(true).
    public func onWeChatBindConfirmTap() {
        fatalError("ProfileViewModel.onWeChatBindConfirmTap must be overridden by subclass")
    }

    /// Modal 内"稍后再说"按钮 / Modal 遮罩 / 关闭按钮触发.
    /// MockProfileViewModel: 写 showBindModal = false + 记录 invocation.
    /// RealProfileViewModel（本 story 占位）: **本地 mutate** —— 写 showBindModal = false.
    ///   按 Story 37.9 round 1 P1 lesson 钦定路径.
    /// 后续 epic: 加入 @AppStorage("lastWechatPromptAt") 时间戳记录"已 dismiss"语义.
    public func onWeChatModalDismissTap() {
        fatalError("ProfileViewModel.onWeChatModalDismissTap must be overridden by subclass")
    }

    /// 菜单列表 4 项（成就徽章 / 消息通知 / 喜欢的道具 / 设置）任一点击触发.
    /// MockProfileViewModel: 写 lastToastMessage = "{item.label}（敬请期待）" + 记录 invocation(menuTap(item:)).
    /// RealProfileViewModel（本 story 占位）: **本地 mutate** —— 写 lastToastMessage = "{item.label}（敬请期待）".
    ///   按 Story 37.9 round 1 P1 lesson 钦定路径.
    /// 后续 epic: 改 NavigationLink push 到具体子页面（AchievementsView / MessagesView / FavoritesView / SettingsView）.
    public func onMenuTap(item: ProfileMenuItem) {
        fatalError("ProfileViewModel.onMenuTap must be overridden by subclass")
    }

    /// "最近收藏" SectionHeader 右侧"查看全部"按钮触发.
    /// MockProfileViewModel: 写 lastToastMessage = "查看全部收藏（敬请期待）" + 记录 invocation.
    /// RealProfileViewModel（本 story 占位）: **本地 mutate** —— 同 Mock.
    /// 后续 epic: 改 NavigationLink push 到 AllCollectionsView 全部收藏页.
    public func onCollectionViewAllTap() {
        fatalError("ProfileViewModel.onCollectionViewAllTap must be overridden by subclass")
    }

    /// headerCard 右上角 bell 圆形按钮触发（占位"消息中心"入口）.
    /// MockProfileViewModel: 写 lastToastMessage = "消息中心（敬请期待）" + 记录 invocation.
    /// RealProfileViewModel（本 story 占位）: **本地 mutate** —— 同 Mock.
    ///   按 Story 37.9 round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md` 钦定路径.
    /// 后续 epic: 改 NavigationLink push 到 MessagesView 消息中心.
    ///
    /// Story 37.11 round 3 codex review [P2] 修复：
    /// 之前 ProfileScaffoldView headerIconButton 闭包直接写 `state.lastToastMessage = "..."` 绕过 ViewModel seam，
    /// 导致后续 epic 接 NavigationLink 真实导航时必须改 View 而非 VM —— 违反 "zero-edit scaffold" 契约.
    /// lesson: 2026-05-01-scaffold-view-must-not-bypass-viewmodel-method-seam.md
    public func onBellTap() {
        fatalError("ProfileViewModel.onBellTap must be overridden by subclass")
    }

    /// headerCard 右上角 settings 圆形按钮触发（占位"设置"入口）.
    /// MockProfileViewModel: 写 lastToastMessage = "设置（敬请期待）" + 记录 invocation.
    /// RealProfileViewModel（本 story 占位）: **本地 mutate** —— 同 Mock.
    ///   按 Story 37.9 round 1 P1 lesson 钦定路径.
    /// 后续 epic: 改 NavigationLink push 到 SettingsView.
    ///
    /// Story 37.11 round 3 codex review [P2] 修复（同 onBellTap）.
    public func onSettingsTap() {
        fatalError("ProfileViewModel.onSettingsTap must be overridden by subclass")
    }
}
