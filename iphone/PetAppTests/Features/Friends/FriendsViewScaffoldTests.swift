// FriendsViewScaffoldTests.swift
// Story 37.10 AC7: FriendsScaffoldView + FriendsViewModel class 层次单元测试.
//
// 测试基础设施约束（与 Story 2.7 + ADR-0002 §3.1 衔接）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）.
//   - 不引 ViewInspector / SnapshotTesting.
//   - 走 ViewModel 行为 + invocations 数组断言；不走 SwiftUI body 内省.
//
// case 数：11（≥5 epic AC line 4816 + 6 守护 case 预防 Story 37.7 / 37.8 / 37.9 lesson 反例）.

import XCTest
@testable import PetApp

@MainActor
final class FriendsViewScaffoldTests: XCTestCase {

    // MARK: - case#1 happy: 切到全部 Tab → displayedFriends 含全部好友（含离线）

    func testSelectTabSwitchesDisplayedFriends() {
        let vm = MockFriendsViewModel()
        // 默认 .online → displayedFriends 仅 online == true
        XCTAssertEqual(vm.selectedTab, .online)
        XCTAssertTrue(vm.displayedFriends.allSatisfy { $0.online })
        XCTAssertFalse(vm.displayedFriends.contains(where: { $0.status == .offline }))

        vm.selectTab(.all)
        XCTAssertEqual(vm.selectedTab, .all)
        XCTAssertEqual(vm.displayedFriends.count, vm.friends.count)
        XCTAssertTrue(vm.displayedFriends.contains(where: { $0.status == .offline }))
    }

    // MARK: - case#2 happy: inRoom 好友点"加入" → onJoinFriendTap 触发 + currentRoomId 切到 friend.currentRoomId

    func testOnJoinFriendTapMutatesCurrentRoomId() {
        let vm = MockFriendsViewModel()
        let inRoomFriend = vm.friends.first(where: { $0.status == .inRoom })!
        let targetRoomId = inRoomFriend.currentRoomId!
        XCTAssertNil(vm.currentRoomId, "初始无房间")

        vm.onJoinFriendTap(friend: inRoomFriend)
        XCTAssertEqual(vm.currentRoomId, targetRoomId, "加入后 currentRoomId = friend.currentRoomId")
        XCTAssertEqual(vm.invocations, [.joinTap(friendId: inRoomFriend.id)])
        XCTAssertNotNil(vm.lastToastMessage)
        XCTAssertTrue(vm.lastToastMessage!.contains(inRoomFriend.name))
    }

    // MARK: - case#3 happy: online 好友点"邀请" + currentRoomId nil → mutate currentRoomId 占位 + toast

    func testOnInviteFriendTapWhenNoRoomCreatesPlaceholderRoom() {
        let vm = MockFriendsViewModel()
        let onlineFriend = vm.friends.first(where: { $0.status == .online })!
        XCTAssertNil(vm.currentRoomId)

        vm.onInviteFriendTap(friend: onlineFriend)
        XCTAssertNotNil(vm.currentRoomId, "currentRoomId nil 时邀请触发占位 mock 创建")
        XCTAssertEqual(vm.invocations, [.inviteTap(friendId: onlineFriend.id)])
        XCTAssertNotNil(vm.lastToastMessage)
        XCTAssertTrue(vm.lastToastMessage!.contains(onlineFriend.name))
    }

    // MARK: - case#4 happy: online 好友点"邀请" + currentRoomId 非 nil → 仅 toast，不重新创建

    func testOnInviteFriendTapWhenInRoomOnlyToasts() {
        let vm = MockFriendsViewModel(currentRoomId: "9999999")
        let onlineFriend = vm.friends.first(where: { $0.status == .online })!

        vm.onInviteFriendTap(friend: onlineFriend)
        XCTAssertEqual(vm.currentRoomId, "9999999", "currentRoomId 非 nil → 不重新创建")
        XCTAssertNotNil(vm.lastToastMessage)
    }

    // MARK: - case#5 happy: friends 空数组 → displayedFriends 两 Tab 都空

    func testEmptyFriendsProducesEmptyDisplayedFriends() {
        let vm = MockFriendsViewModel(friends: [])
        XCTAssertTrue(vm.displayedFriends.isEmpty)
        vm.selectTab(.all)
        XCTAssertTrue(vm.displayedFriends.isEmpty)
    }

    // MARK: - case#6 happy: onlineCount derived 正确（hint epic AC「{onlineCount} 位在线 · 共 {friends.count} 位」）

    func testOnlineCountDerivedFromFriends() {
        let vm = MockFriendsViewModel()
        XCTAssertEqual(vm.onlineCount, vm.friends.filter { $0.online }.count)
        XCTAssertEqual(vm.onlineCount, 6, "scaffold defaults: inRoom 3 + online 3 = 6 在线")
    }

    // MARK: - case#7 守护: RealFriendsViewModel 构造注入 AppState 不 crash + override 不 fatalError + Real override 必 mutate state

    /// 防 RealFriendsViewModel 漏 override 时本测试立刻 fail（fatalError 在测试中 → trap）.
    /// + 守护 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`：
    ///   Real 子类 override 必须本地 mutate state（与 Mock 同语义），禁止只 log（否则 production no-op）.
    func testRealFriendsViewModelOverridesMutateStateNotJustLog() {
        let appState = AppState()
        let vm = RealFriendsViewModel(appState: appState)
        XCTAssertNil(vm.currentRoomId, "init 时 appState.currentRoomId nil → sink 派生 nil")
        XCTAssertFalse(vm.friends.isEmpty, "init 走 defaults seed friends")

        // onJoinFriendTap：必须通过 appState 入口写 currentRoomId（不能仅 log）.
        let inRoomFriend = vm.friends.first(where: { $0.status == .inRoom })!
        let targetRoomId = inRoomFriend.currentRoomId!
        vm.onJoinFriendTap(friend: inRoomFriend)
        XCTAssertEqual(appState.currentRoomId, targetRoomId, "Real path 必须通过 appState 入口写 currentRoomId（守护 lesson）")
        XCTAssertEqual(vm.currentRoomId, targetRoomId, "sink 派生让本字段同步")
        XCTAssertNotNil(vm.lastToastMessage)

        // 重置后 onInviteFriendTap：currentRoomId nil 时必须 mutate（占位 mock 创建）.
        appState.setCurrentRoomId(nil)
        XCTAssertNil(vm.currentRoomId)
        let onlineFriend = vm.friends.first(where: { $0.status == .online })!
        vm.onInviteFriendTap(friend: onlineFriend)
        XCTAssertNotNil(appState.currentRoomId, "Real path 邀请 + 无房间 时必须创建占位（守护 lesson）")
    }

    // MARK: - case#8 守护: Real init 必 seed scaffold defaults（lesson 预防性应用）

    /// 与 Story 37.8 / 37.9 同模式 ——
    /// 防未来 Claude 重构 init 时漏 seed friends 让 RealFriendsViewModel 渲染空好友列表.
    /// lesson: 2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md
    func testRealFriendsViewModelInitSeedsScaffoldDefaults() {
        // parameterless init 路径
        let vm1 = RealFriendsViewModel()
        XCTAssertFalse(vm1.friends.isEmpty)
        XCTAssertEqual(vm1.selectedTab, FriendsScaffoldDefaults.selectedTab)
        XCTAssertNil(vm1.currentRoomId)

        // init(appState:) 路径
        let vm2 = RealFriendsViewModel(appState: AppState())
        XCTAssertFalse(vm2.friends.isEmpty)
        XCTAssertEqual(vm2.selectedTab, FriendsScaffoldDefaults.selectedTab)
    }

    // MARK: - case#9 守护: currentRoomId 派生自 appState.currentRoomId（hydrate + reset 路径）

    /// 防未来 Claude 重构时把 currentRoomId sink 改一次性 hydrate 让 reset 后残留旧 roomId.
    /// lesson: 2026-04-30-published-derived-state-needs-publisher-subscription.md
    /// **关键说明**：currentRoomId 派生源是合法的（"我的房间"语义就是本地用户的房间，appState.currentRoomId 是真理源；
    /// 与 Story 37.8 hostCatName 反例不冲突 —— 那是"看别人房间"语境）.
    func testRealFriendsViewModelCurrentRoomIdDerivesFromAppState() {
        let appState = AppState()
        let vm = RealFriendsViewModel(appState: appState)
        XCTAssertNil(vm.currentRoomId, "appState.currentRoomId nil → 派生 nil")

        // hydrate 路径：写入 currentRoomId → 同步派生
        appState.setCurrentRoomId("9999999")
        XCTAssertEqual(vm.currentRoomId, "9999999")

        // reset 路径：appState.reset() 把 currentRoomId 置 nil → 即时 fallback 到 nil（不残留旧值）
        appState.reset()
        XCTAssertNil(vm.currentRoomId, "reset 后 currentRoomId 必回 nil（防 stale）")
    }

    // MARK: - case#10 守护: bind(appState:) 是同步入口（lesson 预防性应用）

    /// 防未来 Claude 把 bind 改成 async 路径让 RootView .onAppear 触发后第一帧 ViewModel 仍未连上 AppState.
    /// 与 Story 37.8 / 37.9 同模式.
    /// lesson: 2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md
    func testRealFriendsViewModelBindAppStateIsSynchronous() {
        let appState = AppState()
        appState.setCurrentRoomId("8888888")  // 启动期 currentRoomId 已非 nil（restored / UITEST_FORCE_IN_ROOM 模拟）

        let vm = RealFriendsViewModel()  // parameterless init 路径
        XCTAssertNil(vm.currentRoomId, "bind 前 currentRoomId = defaults nil")

        vm.bind(appState: appState)  // 同步路径
        XCTAssertEqual(vm.currentRoomId, "8888888", "bind 后立即派生（无 RunLoop tick 等待）")
    }

    // MARK: - case#11 守护: offline 好友不调 onInviteFriendTap / onJoinFriendTap（视觉禁用 + 行为兜底）

    /// 视觉上 offline 好友不渲染按钮（FriendsScaffoldView FriendRow 三态分支，offline → 纯文本"离线"）；
    /// ViewModel 层不强制阻止 —— 但即便外部错误调用，行为有 sane fallback（不 crash + lastToastMessage 失败提示）.
    /// 守护"offline 好友 join 时 friend.currentRoomId 是 nil → 走 nil guard 分支不 crash"路径.
    func testOnJoinFriendTapWithOfflineFriendDoesNotCrash() {
        let vm = MockFriendsViewModel()
        let offlineFriend = vm.friends.first(where: { $0.status == .offline })!
        XCTAssertNil(offlineFriend.currentRoomId, "offline 好友 currentRoomId nil")

        vm.onJoinFriendTap(friend: offlineFriend)
        XCTAssertNil(vm.currentRoomId, "offline join 走 nil guard 分支 → currentRoomId 不变")
        XCTAssertNotNil(vm.lastToastMessage)
        XCTAssertTrue(vm.lastToastMessage!.contains("不在房间中"))
    }

    // MARK: - case#12 守护: onShareMyRoomTap 写 lastToastMessage（fix-review round 1 加回分享按钮 + spec Dev Notes 钦定占位行为）

    /// fix-review round 1 [P2] 加回 myRoomCard 的「分享给好友」按钮 —— 防未来 Claude 重构时漏掉占位 toast 行为.
    /// 与 spec Dev Notes "myRoomCard 分享按钮决策" 钦定一致：仅写 lastToastMessage 占位文案，不真分享.
    /// **关键约束**：onShareMyRoomTap 是基类 concrete method（不是 abstract），让 Mock / Real 共享行为
    ///   —— 主动规避 lesson `2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md` 反模式
    ///   （abstract + fatalError 路径在 production 注入路径下漏 override 即 crash）.
    func testOnShareMyRoomTapWritesPlaceholderToast() {
        // Mock 路径
        let mockVM = MockFriendsViewModel(currentRoomId: "1234567")
        XCTAssertNil(mockVM.lastToastMessage)
        mockVM.onShareMyRoomTap()
        XCTAssertEqual(mockVM.lastToastMessage, "分享功能敬请期待")

        // Real 路径（nil 入口 + bind 入口都不能 crash —— 守护 fatalError 反模式不重现）
        let realVM = RealFriendsViewModel()
        realVM.onShareMyRoomTap()
        XCTAssertEqual(realVM.lastToastMessage, "分享功能敬请期待", "Real 路径走基类 concrete 实装，不 fatalError")

        let appState = AppState()
        appState.setCurrentRoomId("9999999")
        let realVM2 = RealFriendsViewModel(appState: appState)
        realVM2.onShareMyRoomTap()
        XCTAssertEqual(realVM2.lastToastMessage, "分享功能敬请期待")
    }
}
