// ProfileViewScaffoldTests.swift
// Story 37.11 AC7: ProfileScaffoldView + ProfileViewModel class 层次单元测试.
//
// 测试基础设施约束（与 Story 2.7 + ADR-0002 §3.1 衔接）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）.
//   - 不引 ViewInspector / SnapshotTesting.
//   - 走 ViewModel 行为 + invocations 数组断言；不走 SwiftUI body 内省.
//
// case 数：12（≥3 epic AC line 4837 + 5 守护 case 预防 lesson 反例 + 1 round 1 [P2] guard：reset 清 transient）.

import XCTest
@testable import PetApp

@MainActor
final class ProfileViewScaffoldTests: XCTestCase {

    // MARK: - case#1 happy: 默认初始化 → wechatBound=false / showBindModal=false / lastToastMessage=nil + scaffold defaults seed

    func testMockInitSeedsScaffoldDefaults() {
        let vm = MockProfileViewModel()
        XCTAssertFalse(vm.wechatBound)
        XCTAssertFalse(vm.showBindModal)
        XCTAssertNil(vm.lastToastMessage)
        XCTAssertEqual(vm.profile.name, ProfileScaffoldDefaults.profile.name)
        XCTAssertEqual(vm.profile.petName, ProfileScaffoldDefaults.profile.petName)
        XCTAssertEqual(vm.profile.collectionsCount, ProfileScaffoldDefaults.profile.collectionsCount)
        XCTAssertEqual(vm.recentCollections.count, 5, "ScaffoldDefaults 钦定 5 件最近收藏")
    }

    // MARK: - case#2 happy: 点未绑定卡 → showBindModal = true + invocation 记录

    func testWeChatCardTapShowsBindModal() {
        let vm = MockProfileViewModel()
        XCTAssertFalse(vm.showBindModal)

        vm.onWeChatCardTap()
        XCTAssertTrue(vm.showBindModal, "点未绑定卡 → showBindModal = true")
        XCTAssertEqual(vm.invocations, [.wechatCardTap])
    }

    // MARK: - case#3 happy: Modal 内"绑定微信"按钮 → wechatBound = true + showBindModal = false + toast

    func testWeChatBindConfirmTapBindsAndDismissesModal() {
        let vm = MockProfileViewModel(showBindModal: true)
        XCTAssertFalse(vm.wechatBound)

        vm.onWeChatBindConfirmTap()
        XCTAssertTrue(vm.wechatBound, "确认绑定 → wechatBound = true")
        XCTAssertFalse(vm.showBindModal, "确认绑定 → showBindModal = false")
        XCTAssertNotNil(vm.lastToastMessage)
        XCTAssertEqual(vm.invocations, [.wechatBindConfirmTap])
    }

    // MARK: - case#4 happy: Modal "稍后再说"按钮 → showBindModal = false + invocation 记录

    func testWeChatModalDismissTapClosesModal() {
        let vm = MockProfileViewModel(showBindModal: true)
        XCTAssertTrue(vm.showBindModal)

        vm.onWeChatModalDismissTap()
        XCTAssertFalse(vm.showBindModal)
        XCTAssertFalse(vm.wechatBound, "稍后再说 → wechatBound 不变")
        XCTAssertEqual(vm.invocations, [.wechatModalDismissTap])
    }

    // MARK: - case#5 happy: 点 4 个菜单项 → invocation 记录 + toast 含 item.label

    func testMenuTapTriggersToastForEachItem() {
        let vm = MockProfileViewModel()
        for item in ProfileMenuItem.allCases {
            vm.onMenuTap(item: item)
            XCTAssertNotNil(vm.lastToastMessage)
            XCTAssertTrue(
                vm.lastToastMessage!.contains(item.label),
                "toast 必须含 item.label: \(item.label)"
            )
        }
        XCTAssertEqual(vm.invocations.count, ProfileMenuItem.allCases.count)
        XCTAssertEqual(vm.invocations, ProfileMenuItem.allCases.map { .menuTap(item: $0) })
    }

    // MARK: - case#6 happy: 点"查看全部"收藏 → toast + invocation 记录

    func testCollectionViewAllTapTriggersToast() {
        let vm = MockProfileViewModel()
        vm.onCollectionViewAllTap()
        XCTAssertNotNil(vm.lastToastMessage)
        XCTAssertTrue(vm.lastToastMessage!.contains("查看全部"))
        XCTAssertEqual(vm.invocations, [.collectionViewAllTap])
    }

    // MARK: - case#7 守护: RealProfileViewModel 构造注入 AppState 不 crash + override 不 fatalError + Real override 必 mutate state

    /// 防 RealProfileViewModel 漏 override 时本测试立刻 fail（fatalError 在测试中 → trap）.
    /// + 守护 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`：
    ///   Real 子类 override 必须本地 mutate state，禁止只 log.
    func testRealProfileViewModelOverridesMutateStateNotJustLog() {
        let appState = AppState()
        let vm = RealProfileViewModel(appState: appState)
        XCTAssertFalse(vm.wechatBound)
        XCTAssertFalse(vm.showBindModal)

        // onWeChatCardTap 必须 mutate showBindModal
        vm.onWeChatCardTap()
        XCTAssertTrue(vm.showBindModal, "Real path 必须 mutate showBindModal（守护 lesson）")

        // onWeChatBindConfirmTap 必须 mutate wechatBound + showBindModal + lastToastMessage
        vm.onWeChatBindConfirmTap()
        XCTAssertTrue(vm.wechatBound, "Real path 必须 mutate wechatBound（守护 lesson）")
        XCTAssertFalse(vm.showBindModal)
        XCTAssertNotNil(vm.lastToastMessage)

        // onWeChatModalDismissTap 必须 mutate showBindModal
        vm.showBindModal = true  // 重置
        vm.onWeChatModalDismissTap()
        XCTAssertFalse(vm.showBindModal, "Real path 必须 mutate showBindModal（守护 lesson）")

        // onMenuTap 必须 mutate lastToastMessage
        vm.lastToastMessage = nil  // 重置
        vm.onMenuTap(item: .achievements)
        XCTAssertNotNil(vm.lastToastMessage, "Real path 必须 mutate lastToastMessage（守护 lesson）")
        XCTAssertTrue(vm.lastToastMessage!.contains("成就"))

        // onCollectionViewAllTap 必须 mutate lastToastMessage
        vm.lastToastMessage = nil  // 重置
        vm.onCollectionViewAllTap()
        XCTAssertNotNil(vm.lastToastMessage, "Real path 必须 mutate lastToastMessage（守护 lesson）")
    }

    // MARK: - case#8 守护: Real init 必 seed scaffold defaults（lesson 4 预防性应用）

    /// 与 Story 37.8 / 37.9 / 37.10 同模式 ——
    /// 防未来 Claude 重构 init 时漏 seed profile 让 RealProfileViewModel 渲染空头像 / "Lv.--" 等占位字符串.
    /// lesson: 2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md
    func testRealProfileViewModelInitSeedsScaffoldDefaults() {
        // parameterless init 路径
        let vm1 = RealProfileViewModel()
        XCTAssertEqual(vm1.profile.name, ProfileScaffoldDefaults.profile.name)
        XCTAssertEqual(vm1.profile.petLevel, ProfileScaffoldDefaults.profile.petLevel)
        XCTAssertEqual(vm1.recentCollections.count, 5)
        XCTAssertFalse(vm1.wechatBound)
        XCTAssertFalse(vm1.showBindModal)

        // init(appState:) 路径
        let vm2 = RealProfileViewModel(appState: AppState())
        XCTAssertEqual(vm2.profile.name, ProfileScaffoldDefaults.profile.name)
        XCTAssertEqual(vm2.recentCollections.count, 5)
    }

    // MARK: - case#9 守护: profile 派生自 appState.currentUser + currentPet（hydrate + reset 路径）

    /// 防未来 Claude 重构时把 profile sink 改一次性 hydrate 让 reset 后残留旧 user.nickname / pet.name.
    /// lesson: 2026-04-30-published-derived-state-needs-publisher-subscription.md
    /// **关键说明**：profile 派生源是合法的（"我的资料"语义就是本地用户的资料，
    ///   appState.currentUser / currentPet 是真理源；与 Story 37.8 hostCatName 反例不冲突 —— 那是"看别人房间"语境）.
    func testRealProfileViewModelProfileDerivesFromAppState() {
        let appState = AppState()
        let vm = RealProfileViewModel(appState: appState)
        // 初始 nil → profile 走 ScaffoldDefaults
        XCTAssertEqual(vm.profile.name, ProfileScaffoldDefaults.profile.name)

        // hydrate 路径：写入 currentUser / currentPet → profile 派生
        let homeData = makeHomeDataFixture(userNickname: "TestUser", petName: "Mochi")
        appState.applyHomeData(homeData)
        XCTAssertEqual(vm.profile.name, "TestUser", "profile.name 派生自 appState.currentUser.nickname")
        XCTAssertEqual(vm.profile.petName, "Mochi", "profile.petName 派生自 appState.currentPet.name")

        // reset 路径：appState.reset() 把 currentUser / currentPet 置 nil → profile 即时 fallback 到 defaults（不残留旧 "TestUser"）
        appState.reset()
        XCTAssertEqual(vm.profile.name, ProfileScaffoldDefaults.profile.name, "reset 后 profile.name 必回 defaults（防 stale）")
        XCTAssertEqual(vm.profile.petName, ProfileScaffoldDefaults.profile.petName)
    }

    // MARK: - case#9b 守护: appState.reset() 清 transient state（wechatBound / showBindModal / lastToastMessage）回 defaults

    /// Story 37.11 round 1 codex review [P2] guard test：
    /// 防未来 Claude 重构 sink 时漏 reset 路径让 transient state 跨会话污染（旧用户绑定状态泄露给新会话）.
    /// 路径：ResetIdentityViewModel.tap() → appState.reset() → currentUser == nil → 应清 transient state.
    /// lesson: 2026-04-30-real-viewmodel-must-clear-transient-state-on-reset.md
    func testRealProfileViewModelResetClearsTransientState() {
        let appState = AppState()
        let homeData = makeHomeDataFixture(userNickname: "TestUser", petName: "Mochi")
        appState.applyHomeData(homeData)  // hydrate：currentUser / currentPet 非 nil

        let vm = RealProfileViewModel(appState: appState)
        // 模拟用户交互：点未绑定卡 → 弹 modal → 确认绑定 → wechatBound = true / showBindModal = false / toast 写入
        vm.onWeChatCardTap()
        vm.onWeChatBindConfirmTap()
        XCTAssertTrue(vm.wechatBound, "前置：交互后 wechatBound = true")
        XCTAssertFalse(vm.showBindModal)
        XCTAssertNotNil(vm.lastToastMessage, "前置：toast 已写入")

        // reset 路径：appState.reset() → currentUser → nil → transient state 应回 defaults
        appState.reset()
        XCTAssertFalse(vm.wechatBound, "reset 后 wechatBound 必回 false（防跨会话污染）")
        XCTAssertFalse(vm.showBindModal, "reset 后 showBindModal 必回 false")
        XCTAssertNil(vm.lastToastMessage, "reset 后 lastToastMessage 必回 nil")
    }

    // MARK: - case#9c 守护: cold-start 路径（A → B 不经 reset）也清 transient state

    /// Story 37.11 round 2 codex review [P2] guard test：
    /// 路径：401 → RootView 注入的 unauthorized handler 调 SessionStore.clear() + AppLaunchStateMachine.triggerColdStart()
    ///   → bootstrap 重跑 → applyHomeData(用户 B) 直接覆盖 currentUser；**不**调 appState.reset()
    ///   → currentUser 直接 A → B（无 nil 中间态）→ 老"if user == nil"sink 不触发 → transient 泄漏给 B.
    /// 修法：judge "newUserId != lastObservedUserId"（任何身份变化都清 transient）.
    /// lesson: 2026-05-01-real-viewmodel-transient-must-clear-on-any-identity-change.md
    func testRealProfileViewModelColdStartClearsTransientStateOnUserSwitch() {
        let appState = AppState()
        // 用户 A hydrate
        let userA = makeHomeDataFixture(
            userId: "user-A",
            userNickname: "UserA",
            petId: "pet-A",
            petName: "CatA"
        )
        appState.applyHomeData(userA)

        let vm = RealProfileViewModel(appState: appState)
        // 模拟 A 的交互：绑微信 + toast
        vm.onWeChatCardTap()
        vm.onWeChatBindConfirmTap()
        XCTAssertTrue(vm.wechatBound, "前置：A 已 wechatBound = true")
        XCTAssertNotNil(vm.lastToastMessage, "前置：A 已写 toast")

        // cold-start 路径：直接 applyHomeData(B)，**不**调 appState.reset() —— 模拟 401 重跑 bootstrap.
        let userB = makeHomeDataFixture(
            userId: "user-B",
            userNickname: "UserB",
            petId: "pet-B",
            petName: "CatB"
        )
        appState.applyHomeData(userB)

        // 关键断言：A → B 不经 nil 中间态，transient state 也必须清 —— 防泄漏到 B 会话.
        XCTAssertFalse(vm.wechatBound, "cold-start A→B：wechatBound 必回 false（防跨会话污染）")
        XCTAssertFalse(vm.showBindModal, "cold-start A→B：showBindModal 必回 false")
        XCTAssertNil(vm.lastToastMessage, "cold-start A→B：lastToastMessage 必回 nil")
        // profile 派生也应跟上
        XCTAssertEqual(vm.profile.name, "UserB", "profile.name 切换到新用户 B")
        XCTAssertEqual(vm.profile.petName, "CatB")
    }

    // MARK: - case#10 守护: bind(appState:) 是同步入口（lesson 5 预防性应用）

    /// 防未来 Claude 把 bind 改成 async 路径让 RootView .onAppear 触发后第一帧 ViewModel 仍未连上 AppState.
    /// 与 Story 37.8 / 37.9 / 37.10 同模式.
    /// lesson: 2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md
    func testRealProfileViewModelBindAppStateIsSynchronous() {
        let appState = AppState()
        let homeData = makeHomeDataFixture(userNickname: "PreloadedUser", petName: "PreloadedPet")
        appState.applyHomeData(homeData)  // 启动期 currentUser / currentPet 已非 nil（restored session）

        let vm = RealProfileViewModel()  // parameterless init 路径
        XCTAssertEqual(vm.profile.name, ProfileScaffoldDefaults.profile.name, "bind 前 profile = defaults")

        vm.bind(appState: appState)  // 同步路径
        XCTAssertEqual(vm.profile.name, "PreloadedUser", "bind 后立即派生（无 RunLoop tick 等待）")
        XCTAssertEqual(vm.profile.petName, "PreloadedPet")
    }

    // MARK: - case#11 守护: bind(appState:) 重复调用 idempotent（不重订阅）

    /// 防未来 Claude 重构 bind 时漏 alreadySubscribed guard 让多次 bind 派生多次 sink callback.
    func testRealProfileViewModelBindIsIdempotent() {
        let appState = AppState()
        let vm = RealProfileViewModel()
        vm.bind(appState: appState)
        vm.bind(appState: appState)  // 第二次 bind 应 no-op
        // 触发派生：写入 user → profile.name 应只更新一次（不会因双 sink 派生异常）
        let homeData = makeHomeDataFixture(userNickname: "Test", petName: "Cat")
        appState.applyHomeData(homeData)
        XCTAssertEqual(vm.profile.name, "Test")
    }

    // MARK: - 测试辅助

    /// 构造 HomeData fixture（注入 user.nickname + pet.name；可选 user/pet id）.
    /// 避免每个 case 重复样板代码.
    /// userId/petId 可选 —— round 2 case#9c 需要不同 user.id 区分两个会话；旧 case 走默认 "u-test"/"p-test".
    private func makeHomeDataFixture(
        userId: String = "u-test",
        userNickname: String,
        petId: String = "p-test",
        petName: String
    ) -> HomeData {
        HomeData(
            user: HomeUser(id: userId, nickname: userNickname, avatarUrl: ""),
            pet: HomePet(
                id: petId,
                petType: 1,
                name: petName,
                currentState: .rest,
                equips: []
            ),
            stepAccount: HomeStepAccount(totalSteps: 0, availableSteps: 0, consumedSteps: 0),
            chest: HomeChest(
                id: "c-test",
                status: .counting,
                unlockAt: Date(),
                openCostSteps: 0,
                remainingSeconds: 0
            ),
            room: HomeRoom(currentRoomId: nil)
        )
    }
}
