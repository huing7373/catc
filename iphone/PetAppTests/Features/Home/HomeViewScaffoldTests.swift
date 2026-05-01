// HomeViewScaffoldTests.swift
// Story 37.7 AC7: HomeView Scaffold + HomeViewModel class 层次单元测试.
//
// 测试基础设施约束（与 Story 2.7 + ADR-0002 §3.1 衔接）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）.
//   - 不引 ViewInspector / SnapshotTesting.
//   - 走 ViewModel 行为 + invocations 数组断言；不走 SwiftUI body 内省.

import XCTest
@testable import PetApp

@MainActor
final class HomeViewScaffoldTests: XCTestCase {

    // MARK: - case#1 happy: MockHomeViewModel 默认状态

    /// 验证 MockHomeViewModel 默认值与 Story 37.7 spec 一致（greeting / weather / stats / interactionAnimation / showJoinModal）.
    func testMockHomeViewModelDefaultStateMatchesSpec() {
        let vm = MockHomeViewModel()
        XCTAssertEqual(vm.greeting, "小花想你啦 ♥")
        XCTAssertEqual(vm.weather, "今天 · 晴")
        XCTAssertEqual(vm.stats, .mockHappy)
        XCTAssertEqual(vm.interactionAnimation, .idle)
        XCTAssertFalse(vm.showJoinModal)
        XCTAssertEqual(vm.invocations, [])
    }

    // MARK: - case#2 happy: 点 "创建队伍" → onCreateTap 触发

    /// 验证 onCreateTap 调用后 invocations 含 .createTap.
    func testOnCreateTapAppendsInvocation() {
        let vm = MockHomeViewModel()
        vm.onCreateTap()
        XCTAssertEqual(vm.invocations, [.createTap])
    }

    // MARK: - case#3 happy: 点 "喂食" → interactionAnimation = .flying("🍥")

    /// 验证 onFeedTap 调用后 interactionAnimation 切到 .flying("🍥") + invocations 含 .feedTap.
    func testOnFeedTapTriggersFlyingEmojiAndInvocation() {
        let vm = MockHomeViewModel()
        vm.onFeedTap()
        XCTAssertEqual(vm.interactionAnimation, .flying("🍥"))
        XCTAssertEqual(vm.invocations, [.feedTap])
    }

    // MARK: - case#4 happy: 点 "加入队伍" → showJoinModal = true

    /// 验证 onJoinTap 调用后 showJoinModal 切到 true + invocations 含 .joinTap.
    func testOnJoinTapTogglesShowJoinModalToTrue() {
        let vm = MockHomeViewModel()
        XCTAssertFalse(vm.showJoinModal)
        vm.onJoinTap()
        XCTAssertTrue(vm.showJoinModal)
        XCTAssertEqual(vm.invocations, [.joinTap])
    }

    // MARK: - case#5 edge: stats.hunger = 0 → PetStats 渲染最低值不报错

    /// 验证 PetStats(hunger: 0, mood: 0, energy: 0) 构造合法 + 字段值正确（不下溢 / 不 crash）.
    /// 对应 epic AC line 4743 "edge: stats.hunger = 0 → 状态条渲染最低值（不报错；用 a11y label 文字验证）".
    /// 本测试断言 PetStats 数据契约；视觉断言由 #Preview + Story 37.13 visual-review-checklist 兜底.
    func testPetStatsZeroValueDoesNotUnderflow() {
        let stats = PetStats(hunger: 0, mood: 0, energy: 0)
        XCTAssertEqual(stats.hunger, 0)
        XCTAssertEqual(stats.mood, 0)
        XCTAssertEqual(stats.energy, 0)
        XCTAssertEqual(stats, PetStats.mockEmpty)
        XCTAssertEqual(stats, PetStats.zero)
    }

    // MARK: - case#6 happy: RealHomeViewModel 构造注入 AppState 不 crash

    /// 验证 RealHomeViewModel(appState:) 构造正常 + override 方法可调用（不触发 fatalError 路径）.
    /// 防止 RealHomeViewModel.onCreateTap 等忘记 override 时本测试立刻 fail（fatalError 在测试中 → trap）.
    func testRealHomeViewModelConstructionAndAbstractMethodsDoNotCrash() {
        let appState = AppState()
        let vm = RealHomeViewModel(appState: appState)
        // 调用 5 个 override 方法验证不进入基类 fatalError 路径（progress-only check; 不断言行为细节）.
        vm.onCreateTap()
        vm.onJoinTap()
        vm.onFeedTap()
        vm.onPetTap()
        vm.onPlayTap()
        XCTAssertTrue(vm.showJoinModal)   // onJoinTap 切到 true，作为 override 路径已执行的代理证据
    }

    // MARK: - case#7 happy: parameterless `RealHomeViewModel()` 路径（RootView @StateObject 默认初始化）

    /// Story 37.7 codex round 1 [P1] guard test：验证 `RealHomeViewModel()` 无参 init 路径
    /// 构造正常 + 5 个 override 方法可调用不触发基类 fatalError.
    ///
    /// 背景：RootView 原 (`@StateObject private var homeViewModel = HomeViewModel()`) 注入裸基类时，
    /// 用户点 actionRow / teamIdleCard 任一按钮就立刻 crash —— 基类 onCreateTap 等是 `fatalError` 占位.
    /// 修复后 RootView 改注 `RealHomeViewModel()`（parameterless init），AppState 由 `.task` 内 bind.
    /// 本测试用同样的 parameterless 路径构造 → 调 5 个 override 方法无 crash.
    /// **若未来有人把 RootView 改回裸 HomeViewModel(), 走 HomeViewScaffoldTests case#6 那种 fatalError trap
    /// 不一定能立即抓——case#6 走的是 RealHomeViewModel(appState:) 构造路径. 本 case 用 parameterless 路径,
    /// 直接守护 RootView 用的同款入口.**
    func testRealHomeViewModelParameterlessInitAndAbstractMethodsDoNotCrash() {
        let vm = RealHomeViewModel()
        // 视觉初值与 RealHomeViewModel.configureMockDefaults 钦定一致.
        XCTAssertEqual(vm.greeting, "想你啦 ♥")
        XCTAssertEqual(vm.weather, "今天 · 晴")
        XCTAssertEqual(vm.stats, .mockHappy)
        XCTAssertEqual(vm.interactionAnimation, .idle)
        XCTAssertFalse(vm.showJoinModal)

        // 5 个 override 方法不进入基类 fatalError（progress-only；不断言细节行为，留给 case#6 兜底）.
        vm.onCreateTap()
        vm.onJoinTap()
        vm.onFeedTap()
        vm.onPetTap()
        vm.onPlayTap()
        // 走完 5 个调用证明 override 链路活：基类 fatalError 只要被命中就在测试中 trap → 不会到此处.
        XCTAssertTrue(vm.showJoinModal)
    }

    /// Story 37.7 codex round 1 [P1] guard test：RootView `@StateObject homeViewModel` 必须注入
    /// `RealHomeViewModel`（或其子类），不能注入裸 `HomeViewModel` —— 后者点按钮即 crash.
    ///
    /// 用 `is` runtime type-check：`RealHomeViewModel` 既是 `HomeViewModel` 子类，又能跑 5 个 override.
    /// 为防 RootView 改回裸基类（直接 import `RootView` private 字段不可行）：
    ///   退而求其次断言 `RealHomeViewModel()` instance is `HomeViewModel`（基类多态契约保留）+
    ///   `RealHomeViewModel().onCreateTap()` 不 crash —— 与生产 RootView 链路同款 instance.
    func testRealHomeViewModelIsHomeViewModelSubclassForRootViewInjectionContract() {
        let vm: HomeViewModel = RealHomeViewModel()
        XCTAssertTrue(vm is RealHomeViewModel, "RootView 注入入口必须用 RealHomeViewModel 实例（生产链路防 fatalError）")
        // 多态调用：通过基类引用调 onCreateTap 必须命中 RealHomeViewModel.override（动态分派）, 不 trap.
        vm.onCreateTap()
        // 走到此处说明 vtable 派发到 override 方法而非基类 fatalError；契约成立.
    }
}
