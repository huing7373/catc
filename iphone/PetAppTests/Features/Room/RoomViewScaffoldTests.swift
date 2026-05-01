// RoomViewScaffoldTests.swift
// Story 37.8 AC7: RoomScaffoldView + RoomViewModel class 层次单元测试.
//
// 测试基础设施约束（与 Story 2.7 + ADR-0002 §3.1 衔接）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）.
//   - 不引 ViewInspector / SnapshotTesting.
//   - 走 ViewModel 行为 + invocations 数组断言；不走 SwiftUI body 内省.
//
// 与 HomeViewScaffoldTests Story 37.7 同模式 —— 不走 `UIHostingController` 渲染 SwiftUI body；
// 视觉断言由 #Preview + UITest a11y identifier 兜底.

import XCTest
@testable import PetApp

@MainActor
final class RoomViewScaffoldTests: XCTestCase {

    // MARK: - case#1 happy: MockRoomViewModel 默认 4 成员状态

    /// 验证 MockRoomViewModel 默认值与 Story 37.8 spec 一致（roomCode / hostCatName / 4 成员 / userIsHost）.
    /// 对应 epic AC line 4765 "happy: 注入 mock 4 成员 → View 渲染 4 格无占位".
    /// （视觉断言由 #Preview + UITest 兜底；本测试断言 ViewModel 数据契约）.
    ///
    /// round 1 P2 fix（codex review）：Mock 默认值改为从 RoomScaffoldDefaults 读取（与 Real 同源），
    /// `userIsHost` 默认值由原 `false` 调整为 `true`（让 in-room 占位符合"自身视为房主"语境；
    /// 自定 init(userIsHost: false) 仍可用于 Preview/Test 走非房主视角）.
    func testMockRoomViewModelDefaultStateMatchesSpec() {
        let vm = MockRoomViewModel()
        XCTAssertEqual(vm.roomCodeForCopy, RoomScaffoldDefaults.roomCodeForCopy)
        XCTAssertEqual(vm.hostCatName, RoomScaffoldDefaults.hostCatName)
        XCTAssertEqual(vm.members.count, RoomScaffoldDefaults.members.count)
        XCTAssertEqual(vm.members[0].name, RoomScaffoldDefaults.members[0].name)
        XCTAssertTrue(vm.members[0].isHost)
        XCTAssertEqual(vm.userIsHost, RoomScaffoldDefaults.userIsHost)
        XCTAssertEqual(vm.invocations, [])
    }

    // MARK: - case#2 happy: 2 成员场景注入 → members.count = 2（驱动 View 渲染 2 + 2 占位）

    /// 验证可注入任意 members 数（mock 可配场景）.
    /// 对应 epic AC line 4766 "happy: 注入 mock 2 成员 → View 渲染 2 实 + 2 虚线占位".
    /// 占位 dashed border 的视觉断言由 #Preview + UITest 兜底.
    func testMockRoomViewModelTwoMembersScenario() {
        let vm = MockRoomViewModel(members: MockRoomViewModel.twoMembersMock)
        XCTAssertEqual(vm.members.count, 2)
        XCTAssertEqual(vm.members[0].name, "小花")
        XCTAssertTrue(vm.members[0].isHost)
        XCTAssertEqual(vm.members[1].name, "Mocha")
        XCTAssertFalse(vm.members[1].isHost)
    }

    // MARK: - case#3 happy: 点击复制按钮 → onCopyTap 触发 + invocations 含 .copyTap

    /// 验证 onCopyTap 调用后 invocations 含 .copyTap.
    /// 对应 epic AC line 4767 "happy: 点击复制按钮 → onCopyTap 触发 + UI 显示绿色对勾 1.2s".
    /// （UI 1.2s feedback 由 RoomScaffoldView 内 @State 控制；视觉断言由 UITest case 测点击后 a11y 状态）.
    func testOnCopyTapAppendsInvocation() {
        let vm = MockRoomViewModel()
        vm.onCopyTap()
        XCTAssertEqual(vm.invocations, [.copyTap])
    }

    // MARK: - case#4 happy: 点击离开 → onLeaveTap 触发

    /// 验证 onLeaveTap 调用后 invocations 含 .leaveTap.
    /// 对应 epic AC line 4768 "happy: 点击离开 → onLeaveTap 触发".
    func testOnLeaveTapAppendsInvocation() {
        let vm = MockRoomViewModel()
        vm.onLeaveTap()
        XCTAssertEqual(vm.invocations, [.leaveTap])
    }

    // MARK: - case#5 happy: RealRoomViewModel 构造注入 AppState 不 crash + onLeaveTap 写 currentRoomId nil

    /// 验证 RealRoomViewModel(appState:) 构造正常 + override 方法可调用（不触发 fatalError 路径）.
    /// 防止 RealRoomViewModel.onLeaveTap 等忘记 override 时本测试立刻 fail（fatalError 在测试中 → trap）.
    /// onLeaveTap 调 appState.setCurrentRoomId(nil) → 验证 appState.currentRoomId == nil 作为 override 路径已执行的代理证据.
    func testRealRoomViewModelConstructionAndAbstractMethodsDoNotCrash() {
        let appState = AppState()
        appState.setCurrentRoomId("room_1234567")
        let vm = RealRoomViewModel(appState: appState)
        // Story 37.8: subscribeRoomCode sink 在 init(appState:) 内 hookup —— roomCodeForCopy 应已派生.
        XCTAssertEqual(vm.roomCodeForCopy, "room_1234567")
        // 调用 2 个 override 方法验证不进入基类 fatalError 路径.
        vm.onCopyTap()      // 仅 log，不改 state
        vm.onLeaveTap()     // 调 appState.setCurrentRoomId(nil)
        XCTAssertNil(appState.currentRoomId, "onLeaveTap 应通过 appState 写 nil 触发互斥状态机切回 idle")
    }

    // MARK: - case#6 happy: parameterless `RealRoomViewModel()` 路径（RootView @StateObject 默认初始化）

    /// Story 37.8 lesson "abstract method base class 注入点全部要换 concrete subclass" 守护测试：
    /// 验证 `RealRoomViewModel()` 无参 init 路径构造正常 + 2 个 override 方法可调用不触发基类 fatalError.
    ///
    /// 背景：RootView 走 `@StateObject private var roomViewModel: RoomViewModel = RealRoomViewModel()` 老模式时，
    /// AppState 也是同级 @StateObject，不能在属性初始化器内交叉引用（编译期不允许 self 提前求值）；
    /// AppState 通过 `.task` 内 `roomViewModel.bind(appState: appState)` 延迟注入（与 RealHomeViewModel 同模式）.
    /// 若未来有人把 RootView 改回基类 `RoomViewModel()`, 用户在 inRoom 态点 returnButton / leaveButton 即 fatalError crash.
    ///
    /// round 1 P2 fix（codex review）：seed 4 字段全部走 RoomScaffoldDefaults，断言更新对齐.
    func testRealRoomViewModelParameterlessInitAndAbstractMethodsDoNotCrash() {
        let vm = RealRoomViewModel()
        // round 1 P2 fix：视觉初值统一走 RoomScaffoldDefaults；不再是空 / "默认小猫" / 0 members.
        XCTAssertEqual(vm.roomCodeForCopy, RoomScaffoldDefaults.roomCodeForCopy)
        XCTAssertEqual(vm.hostCatName, RoomScaffoldDefaults.hostCatName)
        XCTAssertEqual(vm.members.count, RoomScaffoldDefaults.members.count)
        XCTAssertEqual(vm.userIsHost, RoomScaffoldDefaults.userIsHost)

        // 2 个 override 方法不进入基类 fatalError（progress-only；不断言细节行为，留给 case#5 兜底）.
        vm.onCopyTap()
        // onLeaveTap 在无 appState 时是 no-op（appState? 链；不 crash）.
        vm.onLeaveTap()
    }

    // MARK: - case#7 happy: Story 37.8 sink 守护 — bind(appState:) 后 reset 应即时反映

    /// Story 37.8 守护测试（应用 RealHomeViewModel codex round 4 [P3] lesson "派生 state 必须订阅 publisher"）：
    /// RealRoomViewModel.bind(appState:) 异步注入路径走 sink；reset 把 currentRoomId 置 nil →
    /// roomCodeForCopy 应即时回 fallback（round 1 P2 fix 后 fallback = RoomScaffoldDefaults.roomCodeForCopy 而非 ""）.
    ///
    /// 老 bug 模式（避免）：派生只在 init / bind 入口一次性 hydrate → reset → currentRoomId nil 后，
    /// roomCodeForCopy 仍残留旧值 "room_xxx". 修复：subscribeRoomCode(to:) 订阅 appState.$currentRoomId,
    /// 任何变化（含 reset → nil）都自动重派.
    func testRealRoomViewModelBindAppStateThenResetUpdatesFields() {
        let appState = AppState()
        appState.setCurrentRoomId("room_1234567")
        appState.applyHomeData(makeSampleHomeData(currentRoomId: "room_1234567"))

        let vm = RealRoomViewModel()
        vm.bind(appState: appState)

        // bind 后 sink 应已派生 roomCodeForCopy / hostCatName.
        XCTAssertEqual(vm.roomCodeForCopy, "room_1234567")
        XCTAssertEqual(vm.hostCatName, "测试猫")  // makeSampleHomeData pet.name = "测试猫"

        // reset：appState.reset() 把 currentRoomId / currentPet 置 nil → sink 触发 → 字段必须回 RoomScaffoldDefaults 占位.
        // round 1 P2 fix：fallback 从空字符串 / "默认小猫" 改为 RoomScaffoldDefaults 占位（让 in-room 不渲染空房间）.
        appState.reset()
        XCTAssertEqual(vm.roomCodeForCopy, RoomScaffoldDefaults.roomCodeForCopy)
        XCTAssertEqual(vm.hostCatName, RoomScaffoldDefaults.hostCatName)
    }

    // MARK: - case#8 happy: round 1 P2 fix 守护 — RealRoomViewModel in-room scaffold defaults

    /// Story 37.8 round 1 P2 fix 守护测试：验证 `RealRoomViewModel()` / `RealRoomViewModel(appState:)`
    /// 两条 init 路径都 seed RoomScaffoldDefaults，让 in-room state 走 Real path 时 RoomScaffoldView
    /// 不会渲染空房间（4 个 mock member 占位 + host cat 占位都到位）.
    ///
    /// 触发场景（任一即让本测试有意义）：
    ///   - UITEST_FORCE_IN_ROOM env flag → AppState.currentRoomId 写非 nil → RootView 切到 inRoom →
    ///     HomeContainerView 渲染 RoomScaffoldView(state: roomViewModel) 而该 roomViewModel = RealRoomViewModel()
    ///   - Story 37.12 后 JoinRoomModal 让用户主动 join → 同样走 RealRoomViewModel
    ///   - 任何手动 debug appState mutation 切到 inRoom 态
    /// 若 RealRoomViewModel.init() 不 seed members → RoomScaffoldView 渲染 0 实 + 4 虚线占位（"形同未交付"）.
    ///
    /// 守护断言：members.count >= 1 + userIsHost == true（精确数依 RoomScaffoldDefaults）.
    func testRealRoomViewModelInitSeedsRoomScaffoldDefaults() {
        // 路径 1: parameterless init.
        let vm1 = RealRoomViewModel()
        XCTAssertGreaterThanOrEqual(vm1.members.count, 1, "parameterless init 必须 seed members")
        XCTAssertTrue(vm1.userIsHost, "parameterless init 必须 seed userIsHost = RoomScaffoldDefaults.userIsHost")
        XCTAssertEqual(vm1.members.count, RoomScaffoldDefaults.members.count)
        XCTAssertEqual(vm1.members[0].name, RoomScaffoldDefaults.members[0].name)
        XCTAssertTrue(vm1.members[0].isHost, "RoomScaffoldDefaults 第一项必须为 host")

        // 路径 2: init(appState:). sink hookup 同时立即 seed defaults.
        let appState = AppState()  // currentRoomId / currentPet 都 nil（fallback 路径）.
        let vm2 = RealRoomViewModel(appState: appState)
        XCTAssertGreaterThanOrEqual(vm2.members.count, 1, "init(appState:) 必须 seed members")
        XCTAssertTrue(vm2.userIsHost, "init(appState:) 必须 seed userIsHost = RoomScaffoldDefaults.userIsHost")
        XCTAssertEqual(vm2.members.count, RoomScaffoldDefaults.members.count)
        // sink 已派发首值（appState 为空 → fallback 到 RoomScaffoldDefaults.roomCodeForCopy / hostCatName）.
        XCTAssertEqual(vm2.roomCodeForCopy, RoomScaffoldDefaults.roomCodeForCopy)
        XCTAssertEqual(vm2.hostCatName, RoomScaffoldDefaults.hostCatName)
    }
}
