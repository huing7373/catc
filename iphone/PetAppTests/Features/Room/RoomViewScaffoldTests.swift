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
    ///
    /// round 3 P2 fix（codex review）：hostCatName 不再 sink 派生 currentPet —— 即便 appState 里
    /// currentPet 已 hydrate 为"测试猫"，hostCatName 也应保持 RoomScaffoldDefaults 占位（pre-WS 阶段
    /// "不知道 host 是谁就用占位"，避免用户加入别人房间时显示自己的猫名）；reset 后同样保持占位.
    func testRealRoomViewModelBindAppStateThenResetUpdatesFields() {
        let appState = AppState()
        appState.setCurrentRoomId("room_1234567")
        appState.applyHomeData(makeSampleHomeData(currentRoomId: "room_1234567"))

        let vm = RealRoomViewModel()
        vm.bind(appState: appState)

        // bind 后 roomCode sink 已派生 roomCodeForCopy.
        XCTAssertEqual(vm.roomCodeForCopy, "room_1234567")
        // round 3 P2 fix：hostCatName 不从 currentPet 派生 — 即便 appState.currentPet="测试猫"，
        // hostCatName 仍保持 init 时 seed 的 RoomScaffoldDefaults.hostCatName 占位.
        XCTAssertEqual(vm.hostCatName, RoomScaffoldDefaults.hostCatName,
                       "round 3 P2 fix：hostCatName 不再派生自 appState.currentPet（local 猫 ≠ room host 猫）")

        // reset：appState.reset() 把 currentRoomId 置 nil → roomCode sink 触发 → 回 RoomScaffoldDefaults 占位.
        // round 1 P2 fix：fallback 从空字符串 / "默认小猫" 改为 RoomScaffoldDefaults 占位（让 in-room 不渲染空房间）.
        // hostCatName 在 reset 前后都保持 RoomScaffoldDefaults 占位（无 sink → 不会因 currentPet 置 nil 而变）.
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

    // MARK: - case#9 happy: round 2 P2 fix 守护 — bind(appState:) 是同步路径（不依赖 await/dispatch）
    //
    // 背景：codex round 2 [P2] finding —— RootView 旧实装在 `.task`（异步）内调 bind(appState:),
    // 当 app 启动时 appState.currentRoomId != nil（restored in-room / UITEST_FORCE_IN_ROOM /
    // /home 返回非 nil currentRoomId），HomeContainerView 在 `.task` 跑之前已经决策走 inRoom 分支 →
    // RoomScaffoldView 渲染 → RealRoomViewModel.appState 仍是 nil → leave tap 无效 + room title/code
    // 显示 placeholder. round 2 fix 把 bind(appState:) 搬到 `.onAppear`（第一次 paint 之前同步执行）.
    //
    // 本守护测试验证 bind(appState:) 是**纯同步**入口（与 onAppear 配对的契约前提）：
    //   1. parameterless init() 后 appState 字段为 nil（用 Mirror 反射；私有字段）
    //   2. 调 bind(appState:) 后**同步立即**(无 await / 无 RunLoop tick) appState 字段为传入实例
    //   3. 紧接着调 onLeaveTap() 能立刻通过 self.appState?.setCurrentRoomId(nil) 写入 AppState
    //
    // 若未来重构把 RealRoomViewModel.bind 改为 async / 把 appState 写延后到 sink dispatch，
    // 本测试会立即失败 —— 提示 RootView.onAppear 同步注入契约被破坏（race 又会回归）.
    //
    // 详见 docs/lessons/2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md.
    func testRealRoomViewModelBindAppStateIsSynchronous() {
        let vm = RealRoomViewModel()

        // 1. 构造后 private appState 字段必须 nil（覆盖 onAppear 触发前的初始态）.
        XCTAssertNil(
            Mirror(reflecting: vm).descendant("appState") as? AppState,
            "parameterless init 后 appState 字段应为 nil（onAppear 之前的初始态）"
        )

        // 2. 同步调 bind(appState:) 后 private appState 字段必须**立刻**为传入实例.
        //    用 ObjectIdentifier 做引用相等（AppState 是 class）.
        let appState = AppState()
        appState.setCurrentRoomId("room_round2_guard")
        vm.bind(appState: appState)

        let bound = Mirror(reflecting: vm).descendant("appState") as? AppState
        XCTAssertNotNil(
            bound,
            "bind(appState:) 同步调用之后 appState 字段必须立即非 nil — RootView.onAppear 同步注入契约的前提"
        )
        XCTAssertTrue(
            bound === appState,
            "bind(appState:) 写入的必须是同一个 AppState 实例（不是 copy）"
        )

        // 3. 同步调 onLeaveTap() 能立刻把 currentRoomId 置 nil（证明 self.appState 已绑且可用，
        //    不依赖 await / dispatch）—— 即"第一次 paint 之后用户立即点 leave，能命中 RealRoomViewModel
        //    override 路径"的关键 invariant.
        XCTAssertEqual(appState.currentRoomId, "room_round2_guard", "前置：currentRoomId 已写入")
        vm.onLeaveTap()
        XCTAssertNil(
            appState.currentRoomId,
            "bind 后立即调 onLeaveTap 必须能写 currentRoomId=nil — 守护 RootView.onAppear 同步 bind 契约"
        )
    }

    // MARK: - case#10 happy: round 3 P2 fix 守护 — hostCatName 不从 appState.currentPet 派生

    /// Story 37.8 round 3 P2 fix 契约守护测试（codex review finding）：
    /// **场景**：用户加入别人的房间（restored session 走 /home → currentRoomId 非 nil + currentPet =
    /// 本地用户的猫）. 此时 RealRoomViewModel.hostCatName 必须保持 RoomScaffoldDefaults.hostCatName
    /// 占位，**不**派生自 appState.currentPet（那是本地猫，不是 room host 的猫）.
    ///
    /// 老 bug 模式（避免）：
    ///   subscribeHostCatName sink → appState.$currentPet → "<我的猫>的小屋"（user-visible 错误）.
    ///
    /// 修复策略（option A，与 user review summary 对齐）：
    ///   彻底删除 hostCatName 的 sink；hostCatName 永远 = init 时 seed 的 RoomScaffoldDefaults.hostCatName,
    ///   直到 Story 12.1 接 WS room.snapshot 后由新 subscribe 入口写真实 host 名.
    ///
    /// 本测试是 round 3 P2 fix 的 contract guard：未来任何 PR 把 subscribeHostCatName 加回来都会让
    /// 本测试 fail，提示 reviewer "你正在重新引入用户加入别人房间时显示自己猫名的 bug".
    func testRealRoomViewModelHostCatNameDoesNotDeriveFromCurrentPet() {
        let appState = AppState()
        // 模拟"用户加入别人的房间"：currentRoomId 已 set + currentPet 是本地猫.
        appState.setCurrentRoomId("room_joined_other")
        appState.applyHomeData(makeSampleHomeData(currentRoomId: "room_joined_other"))
        // 前置 sanity：appState.currentPet?.name == "测试猫"（本地猫）.
        XCTAssertEqual(appState.currentPet?.name, "测试猫", "前置：appState.currentPet 已 hydrate 为本地猫")

        // 路径 1: init(appState:) 构造路径（包含 subscribeRoomCode hookup；旧 bug：还会 hookup hostCatName sink）.
        let vm1 = RealRoomViewModel(appState: appState)
        XCTAssertEqual(
            vm1.hostCatName,
            RoomScaffoldDefaults.hostCatName,
            "round 3 P2 fix：init(appState:) 路径不得让 hostCatName 派生为本地猫名 '\(appState.currentPet?.name ?? "?")'"
        )
        // sanity：roomCode sink 仍正常工作（roomCodeForCopy 仍派生自 currentRoomId）.
        XCTAssertEqual(vm1.roomCodeForCopy, "room_joined_other")

        // 路径 2: parameterless init() + bind(appState:) 异步注入路径.
        let vm2 = RealRoomViewModel()
        vm2.bind(appState: appState)
        XCTAssertEqual(
            vm2.hostCatName,
            RoomScaffoldDefaults.hostCatName,
            "round 3 P2 fix：bind(appState:) 路径不得让 hostCatName 派生为本地猫名"
        )
        XCTAssertEqual(vm2.roomCodeForCopy, "room_joined_other")

        // 即便事后再变更 currentPet（譬如换猫名 / 切账号），hostCatName 仍不应跟动.
        let prev = appState.currentPet
        let updatedPet = HomePet(
            id: prev?.id ?? "p_unknown",
            petType: prev?.petType ?? 1,
            name: "另一只本地猫",
            currentState: prev?.currentState ?? .rest,
            equips: prev?.equips ?? []
        )
        appState.currentPet = updatedPet
        XCTAssertEqual(
            vm1.hostCatName,
            RoomScaffoldDefaults.hostCatName,
            "round 3 P2 fix：currentPet 变更后 hostCatName 仍必须保持占位（无 sink 在派生）"
        )
        XCTAssertEqual(
            vm2.hostCatName,
            RoomScaffoldDefaults.hostCatName,
            "round 3 P2 fix：currentPet 变更后 hostCatName 仍必须保持占位（无 sink 在派生）"
        )
    }
}
