// RealProfileViewModel.swift
// Story 37.11 AC2: ProfileViewModel 生产实装子类（构造注入 AppState；override 5 个 abstract method 占位 mutate）.
//
// 范围（本 story 占位；后续 epic 填充真实 UseCase 调用）：
//   - 构造注入 AppState（按 ADR-0010 §3.1 ViewModel 注入规则）+ parameterless init() 走 bind(appState:)
//   - override 5 个 abstract method：本地 mutate showBindModal / wechatBound / lastToastMessage（占位）
//     按 Story 37.9 round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`：
//     Real 子类 override 必须实装"本 story 范围内能让 UI 视觉工作的最小 placeholder 行为"，禁止只 log.
//   - sink 订阅 appState.$currentUser + appState.$currentPet → 派生 profile 字段
//
// **不**调用任何 UseCase / Repository / APIClient（Epic 37 红线：UI Scaffold 数据完全 mock）.
// **不**调用 WechatService / WXApi（本 story 占位 wechatBound = true 即可；后续 epic 真 OAuth 落地）.
// **不**订阅真实收藏 / 成就接口（后续 epic 落地；本 story RealProfileViewModel.recentCollections 走 ScaffoldDefaults seed）.
//
// Story 37.7 / 37.8 / 37.9 / 37.10 沉淀 lesson 预防性应用（**不重蹈覆辙**）：
//   - lesson 1 `2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md`：
//     RootView `@StateObject profileViewModel` 用 `RealProfileViewModel()` 而非基类 `ProfileViewModel()`.
//   - lesson 4 `2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md`：
//     两条 init 路径都走 `ProfileScaffoldDefaults` seed —— 让 launch / hydrate 前 / reset 后任何
//     Real path 都立刻有 mock profile 占位（不让 ProfileScaffoldView 渲染空头像 / 空姓名 / "Lv.--"）.
//   - lesson 2 `2026-04-30-published-derived-state-needs-publisher-subscription.md`：
//     派生 state 用 sink 路径而非一次性 hydrate —— profile 订阅 appState.$currentUser + $currentPet；
//     reset 路径（appState.reset() 把 currentUser/Pet 置 nil）也能即时反映到字段（不残留旧值）.
//   - lesson 3 `2026-04-30-room-host-name-must-not-derive-from-local-current-pet.md`（**反向应用**）：
//     Profile 域 profile.name / profile.petName 派生自 appState.currentUser / currentPet **是合法**的 ——
//     "我的资料"语义就是本地用户自己的资料（与 Friends 域 currentRoomId 派生同理；与 Story 37.8 hostCatName 反例不冲突 —— 那是"看别人房间"语境）.
//   - lesson 5 `2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md`：
//     RootView `.onAppear` 内同步 bind appState（不放 .task）.
//   - lesson 6 `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`（**关键**）：
//     5 个 override **必须本地 mutate state**（与 Mock 同语义）：
//       · onWeChatCardTap → showBindModal = true
//       · onWeChatBindConfirmTap → wechatBound = true + showBindModal = false + lastToastMessage
//       · onWeChatModalDismissTap → showBindModal = false
//       · onMenuTap(item:) → lastToastMessage = "{item.label}（敬请期待）"
//       · onCollectionViewAllTap → lastToastMessage = "查看全部收藏（敬请期待）"

import Foundation
import Combine
import os.log

@MainActor
public final class RealProfileViewModel: ProfileViewModel {
    /// 构造注入 AppState（与 RealHomeViewModel / RealRoomViewModel / RealWardrobeViewModel / RealFriendsViewModel 同模式）.
    private var appState: AppState?

    /// 派生 state sink 句柄（防多次 bind 重订阅 + 持有 cancellable 让 sink 存活）.
    private var profileSubscriptions: Set<AnyCancellable> = []

    /// round 2 codex review [P2] 修复辅助状态：记录 sink 上一次观察到的 user id.
    /// 判 transient 清理边界 = "user 身份是否变化"（含 A→nil / nil→A / A→B），不再仅看 nil.
    /// 防 cold-start 路径（401 → SessionStore.clear() + bootstrap 直接换 user，**不**调 appState.reset()）：
    /// `currentUser` 直接从 A 跳到 B（中间无 nil），旧"if user == nil"sink 不触发 → transient 泄漏到 B 会话.
    /// nil 哨兵值：parameterless init 后首次 sink emit 时（init 默认 currentUser 即 nil）无身份变化 → no-op.
    private var lastObservedUserId: String?

    /// parameterless init —— RootView `@StateObject` 老模式可用; AppState 通过 bind 异步注入.
    /// 按 Story 37.8 / 37.9 / 37.10 round 1 P2 lesson 预防性应用：seed 5 字段全部走 ProfileScaffoldDefaults,
    /// 让 launch / hydrate 前 / reset 后任何走 Real path 都立刻有 mock 占位.
    /// 注：必写 `override` —— 基类 ProfileViewModel 有显式 `public init() {}`（与 RoomViewModel / WardrobeViewModel / FriendsViewModel 同模式）.
    public override init() {
        super.init()
        self.appState = nil
        self.profile = ProfileScaffoldDefaults.profile
        self.wechatBound = ProfileScaffoldDefaults.wechatBound
        self.recentCollections = ProfileScaffoldDefaults.recentCollections
        self.showBindModal = false
        self.lastToastMessage = nil
    }

    public init(appState: AppState) {
        super.init()
        self.appState = appState
        // round 1 P2 fix：先 seed scaffold defaults（让 sink 还没派发前 ProfileScaffoldView 有数据可渲染）.
        self.profile = ProfileScaffoldDefaults.profile
        self.wechatBound = ProfileScaffoldDefaults.wechatBound
        self.recentCollections = ProfileScaffoldDefaults.recentCollections
        self.showBindModal = false
        self.lastToastMessage = nil
        // 构造路径已注入 AppState；立即订阅派生.
        subscribeProfile(to: appState)
    }

    /// AppState 异步注入入口（与 RealHomeViewModel / RealRoomViewModel.bind / RealWardrobeViewModel.bind / RealFriendsViewModel.bind 同模式）.
    public func bind(appState: AppState) {
        let alreadySubscribed = !profileSubscriptions.isEmpty
        self.appState = appState
        guard !alreadySubscribed else { return }
        subscribeProfile(to: appState)
    }

    /// 订阅 appState.$currentUser + appState.$currentPet → 合并派生 profile 字段（保留 mock 字段如
    /// collectionsCount / achievementsCount 等本地 cache，仅覆盖 name / id / petName / petLevel 等真实字段）.
    /// **关键**：profile 派生源是合法的 —— Profile 域语义就是"我的资料"（本地用户自己的资料），
    /// appState.currentUser / currentPet 是真理源（与 Story 37.10 currentRoomId 派生同合法理由；
    /// 与 Story 37.8 RoomViewModel.hostCatName 反例不冲突 —— 那是"看别人房间"语境）.
    /// 详见 Dev Notes "profile 派生源 vs Story 37.8 hostCatName 反例".
    ///
    /// round 1 codex review [P2] 修复：除 profile 派生外，**额外订阅 currentUser 单独 sink**
    /// 监听 reset 路径（user == nil）→ 清 transient state（wechatBound / showBindModal / lastToastMessage）回 defaults.
    /// 防 ResetIdentityViewModel.tap() → appState.reset() 后旧用户的 transient UI 状态污染下一会话.
    /// 见 lesson `2026-04-30-real-viewmodel-must-clear-transient-state-on-reset.md`.
    ///
    /// round 2 codex review [P2] 修复：判 transient 清理边界改为"任何身份变化"（A→nil / nil→A / A→B）.
    /// 触发场景：401 cold-start → RootView 注入 handler 调 SessionStore.clear() + AppLaunchStateMachine.triggerColdStart()
    ///   → bootstrap 重跑 → applyHomeData(用户 B) 直接覆盖 currentUser；**不调** appState.reset()
    ///   → user 直接 A → B（无 nil 中间态）→ 旧"if user == nil"sink 不触发 → transient 泄漏.
    /// 见 lesson `2026-05-01-real-viewmodel-transient-must-clear-on-any-identity-change.md`.
    private func subscribeProfile(to appState: AppState) {
        // 用 CombineLatest 让 currentUser + currentPet 任一变化都触发 profile 重新合并.
        Publishers.CombineLatest(appState.$currentUser, appState.$currentPet)
            .sink { [weak self] user, pet in
                guard let self else { return }
                self.profile = ProfileSummary(
                    id: user?.id ?? ProfileScaffoldDefaults.profile.id,
                    name: user?.nickname ?? ProfileScaffoldDefaults.profile.name,
                    title: ProfileScaffoldDefaults.profile.title,            // 本期无 server 字段 → 走 defaults
                    joinedAt: ProfileScaffoldDefaults.profile.joinedAt,      // 本期无 server 字段 → 走 defaults
                    petName: pet?.name ?? ProfileScaffoldDefaults.profile.petName,
                    petLevel: ProfileScaffoldDefaults.profile.petLevel,      // 本期 HomePet 无 level 字段 → 走 defaults
                    collectionsCount: ProfileScaffoldDefaults.profile.collectionsCount,
                    friendsCount: ProfileScaffoldDefaults.profile.friendsCount,
                    achievementsCount: ProfileScaffoldDefaults.profile.achievementsCount,
                    coinsCount: ProfileScaffoldDefaults.profile.coinsCount
                )
            }
            .store(in: &profileSubscriptions)

        // round 1 codex review [P2] 修复（round 2 升级）：监听 currentUser 任何身份变化 → 清 transient state.
        // 不放在 CombineLatest 同 sink 里：profile 派生本身已 fallback 到 defaults；transient 字段语义独立,
        // 单独订阅让"reset/换会话清 transient"语义更显式（避免日后改 profile sink 顺带搞砸 transient 边界）.
        //
        // round 2 升级：判 transient 清理改为 newUserId != lastObservedUserId（含 A→nil / nil→A / A→B）.
        // 边界守卫：parameterless init 后首次 sink emit 时 lastObservedUserId == nil 且 newUserId == nil（init 默认）
        //   → 不触发清理（已是 defaults，no-op；同时也要 update lastObservedUserId 让后续 nil→A hydrate 正常进 fix path）.
        appState.$currentUser
            .sink { [weak self] user in
                guard let self else { return }
                let newUserId = user?.id
                if newUserId != self.lastObservedUserId {
                    self.wechatBound = false
                    self.showBindModal = false
                    self.lastToastMessage = nil
                    self.lastObservedUserId = newUserId
                }
            }
            .store(in: &profileSubscriptions)
    }

    // MARK: - override abstract methods（本 story 占位 mutate；后续 epic 实装真实 UseCase）

    /// round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md` 预防性应用：
    /// override 必须**本地 mutate** state 让 UI 立刻反馈，不能只 log.
    ///
    /// 行为与 MockProfileViewModel.onWeChatCardTap 同语义：写 showBindModal = true.
    /// 后续 epic 落地时改为：可选检查 @AppStorage("lastWechatPromptAt") 决定弹 modal vs toast 提示.
    public override func onWeChatCardTap() {
        os_log(.debug, "RealProfileViewModel.onWeChatCardTap (后续 epic will check @AppStorage timestamp)")
        showBindModal = true
    }

    /// round 1 P1 lesson 预防性应用：override 必须本地 mutate state.
    /// 行为与 MockProfileViewModel.onWeChatBindConfirmTap 同语义.
    /// 后续 epic 落地时改为：
    ///   1) 调 BindWechatUseCase / WXApi.sendAuthReq 拉起授权
    ///   2) 后端换 OpenID + 写入用户表
    ///   3) 成功后 server 推送 → setWeChatBound(true) + showBindModal = false + Toast "微信绑定成功，数据已受保护"
    public override func onWeChatBindConfirmTap() {
        os_log(.debug, "RealProfileViewModel.onWeChatBindConfirmTap (后续 epic will wire WXApi.sendAuthReq)")
        wechatBound = true
        showBindModal = false
        lastToastMessage = "微信绑定（敬请期待）"
    }

    /// round 1 P1 lesson 预防性应用：override 必须本地 mutate state.
    /// 行为与 MockProfileViewModel.onWeChatModalDismissTap 同语义：关闭 Modal.
    /// 后续 epic 加入 @AppStorage("lastWechatPromptAt") 时间戳记录"已 dismiss"语义（24 小时再次弹一次）.
    public override func onWeChatModalDismissTap() {
        os_log(.debug, "RealProfileViewModel.onWeChatModalDismissTap (后续 epic will record dismiss timestamp)")
        showBindModal = false
    }

    /// round 1 P1 lesson 预防性应用：override 必须本地 mutate state.
    /// 行为与 MockProfileViewModel.onMenuTap 同语义：写 toast 占位.
    /// 后续 epic 改 NavigationLink push 到具体子页面.
    public override func onMenuTap(item: ProfileMenuItem) {
        os_log(.debug, "RealProfileViewModel.onMenuTap %{public}@ (后续 epic will push child views)", item.rawValue)
        lastToastMessage = "\(item.label)（敬请期待）"
    }

    /// round 1 P1 lesson 预防性应用：override 必须本地 mutate state.
    /// 行为与 MockProfileViewModel.onCollectionViewAllTap 同语义：写 toast 占位.
    /// 后续 epic 改 NavigationLink push 到 AllCollectionsView.
    public override func onCollectionViewAllTap() {
        os_log(.debug, "RealProfileViewModel.onCollectionViewAllTap (后续 epic will push AllCollectionsView)")
        lastToastMessage = "查看全部收藏（敬请期待）"
    }

    /// Story 37.11 round 3 codex review [P2] 修复 + lesson 6 预防性应用：
    /// Real override 必须本地 mutate state（不能仅 log）—— 与 Mock 同语义.
    /// 后续 epic 改 NavigationLink push 到 MessagesView 消息中心.
    public override func onBellTap() {
        os_log(.debug, "RealProfileViewModel.onBellTap (后续 epic will push MessagesView)")
        lastToastMessage = "消息中心（敬请期待）"
    }

    /// Story 37.11 round 3 codex review [P2] 修复 + lesson 6 预防性应用.
    /// 后续 epic 改 NavigationLink push 到 SettingsView.
    public override func onSettingsTap() {
        os_log(.debug, "RealProfileViewModel.onSettingsTap (后续 epic will push SettingsView)")
        lastToastMessage = "设置（敬请期待）"
    }
}
