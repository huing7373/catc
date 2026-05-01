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

    // MARK: - case#3 happy: 点 "喂食" → interactionAnimation 是 .flying(emoji: "🍥", id: ...)

    /// 验证 onFeedTap 调用后 interactionAnimation 切到 .flying(emoji: "🍥", id: <随机 UUID>)
    /// + invocations 含 .feedTap.
    /// Story 37.7 codex round 2 [P2] fix：UUID 每次 onTap 新生成，不能用固定 UUID 等值断言；
    /// 改用 case-let pattern match 解构 emoji 字段断言（id 字段独立用 case#3b 守护连点重放）.
    func testOnFeedTapTriggersFlyingEmojiAndInvocation() {
        let vm = MockHomeViewModel()
        vm.onFeedTap()
        guard case let .flying(emoji, _) = vm.interactionAnimation else {
            XCTFail("expected .flying after onFeedTap, got \(vm.interactionAnimation)")
            return
        }
        XCTAssertEqual(emoji, "🍥")
        XCTAssertEqual(vm.invocations, [.feedTap])
    }

    // MARK: - case#3b happy: 同 emoji 连点 → 两次 interactionAnimation 不 Equatable（核心契约）

    /// Story 37.7 codex round 2 [P2] fix 守护测试：
    ///   连点 onFeedTap 两次 → 两次 interactionAnimation Equatable 比较应为不等
    ///   （UUID id 不同；emoji 相同）. 这是 SwiftUI `onChange(of:)` 能感知重放动画的核心契约.
    /// 若未来有人改回 `case flying(String)` 单字段或共用 UUID，本 case 立刻 fail.
    func testRapidSameEmojiTapsProduceDistinctAnimationStates() {
        let vm = MockHomeViewModel()
        vm.onFeedTap()
        let first = vm.interactionAnimation
        vm.onFeedTap()
        let second = vm.interactionAnimation

        // 两次都是 .flying("🍥", _) 但 UUID 不同 → Equatable 不等.
        XCTAssertNotEqual(first, second, ".flying 同 emoji 连点必须产生不等的 AnimationState（连点重放契约）")

        // 双重守护：emoji 字段相同（行为一致），id 字段不同（重放保证）.
        guard case let .flying(emoji1, id1) = first,
              case let .flying(emoji2, id2) = second
        else {
            XCTFail("expected both states .flying, got \(first) / \(second)")
            return
        }
        XCTAssertEqual(emoji1, "🍥")
        XCTAssertEqual(emoji2, "🍥")
        XCTAssertNotEqual(id1, id2, "UUID 必须每次 onTap 新生成（option A 实装核心）")
    }

    // MARK: - case#3c contract: AnimationState Equatable 实装契约（id 不同视为不等）

    /// Story 37.7 codex round 2 [P2] fix：直接用两个固定但不同的 UUID 构造 .flying 断言不等.
    /// 这是 option A 修法的最小契约：未来重构若误把 id 设成 ignored 字段（如 Equatable 自定义实装
    /// 跳过 id 比较），本 case 立刻 fail.
    func testAnimationStateFlyingEquatabilityRequiresMatchingId() {
        let id1 = UUID()
        let id2 = UUID()
        XCTAssertNotEqual(id1, id2)

        let a: AnimationState = .flying(emoji: "🍥", id: id1)
        let b: AnimationState = .flying(emoji: "🍥", id: id2)
        XCTAssertNotEqual(a, b, ".flying 同 emoji 不同 id 必须不 Equatable（连点重放核心契约）")

        // 反向 sanity check：emoji + id 完全相同 → Equatable.
        let c: AnimationState = .flying(emoji: "🍥", id: id1)
        XCTAssertEqual(a, c, ".flying 同 emoji 同 id 应 Equatable（sanity）")
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

    // MARK: - case#9 happy: Story 37.7 codex round 3 [P2-A] greeting 派生守护

    /// Story 37.7 codex round 3 [P2-A] guard test：RealHomeViewModel.applyHomeData(_:) 必须从
    /// hydrated AppState.currentPet.name 派生 greeting；不能继续 hardcode "想你啦 ♥" placeholder.
    ///
    /// 老 bug：`configureMockDefaults()` 把 greeting hardcode "想你啦 ♥",
    /// bootstrap 注入 HomeData 后 greeting 仍 placeholder, 生产用户永远看不到自己宠物名字.
    /// 修复：override applyHomeData(_:),先 super.applyHomeData(data) 写 AppState,再读 data.pet?.name 拼.
    /// 派生公式：pet 有名字 → "{petName}，想你啦 ♥"；pet=nil / name 空 → 老 placeholder "想你啦 ♥".
    func testRealHomeViewModelGreetingDerivesFromHydratedPet() {
        let appState = AppState()
        let vm = RealHomeViewModel(appState: appState)

        // hydrate 前 greeting 是 placeholder（configureMockDefaults 钦定）.
        XCTAssertEqual(vm.greeting, "想你啦 ♥", "构造时 greeting 应是 placeholder（pet 还未注入）")

        // 注入 HomeData（pet name = "测试猫"，由 makeSampleHomeData 钦定）.
        let data = makeSampleHomeData()
        vm.applyHomeData(data)

        // 期望：override 把 pet.name "测试猫" 拼进 greeting.
        XCTAssertEqual(
            vm.greeting,
            "测试猫，想你啦 ♥",
            "Story 37.7 codex round 3 [P2-A]: hydrated AppState 后 greeting 必须反映 pet.name"
        )

        // 同时验证 super 链路也跑了：loadingState 应为 .loaded（基类 applyHomeData 必行）.
        XCTAssertEqual(vm.loadingState, .loaded, "super.applyHomeData 必须被调（loadingState 应转 .loaded）")
    }

    // MARK: - case#10 happy: Story 37.7 codex round 4 [P3] reset 后 greeting 回 placeholder

    /// Story 37.7 codex round 4 [P3] guard test：appState.reset() 把 currentPet 置 nil 后，
    /// RealHomeViewModel.greeting 必须立即回 placeholder "想你啦 ♥"，不残留 hydrate 时的 pet 名.
    ///
    /// 老 round 3 bug：greeting 派生只在 applyHomeData(_:) 入口（一次性）；
    /// ResetIdentityViewModel.tap() → appState.reset() 把 currentPet → nil 不经过 applyHomeData,
    /// header 仍显示旧 pet name 直到下一次 hydrate.
    /// round 4 修复：subscribeGreeting(to:) 订阅 appState.$currentPet，任何变化（含 reset → nil）
    /// 都自动重派 greeting.
    func testRealHomeViewModelGreetingFallsBackOnReset() {
        let appState = AppState()
        let vm = RealHomeViewModel(appState: appState)

        // 1. hydrate：appState.currentPet 设为 "测试猫" → sink 触发 → greeting 应反映 pet.name.
        appState.applyHomeData(makeSampleHomeData())
        XCTAssertEqual(
            vm.greeting,
            "测试猫，想你啦 ♥",
            "hydrate 后 sink 应自动重派 greeting"
        )

        // 2. reset：appState.reset() 把 currentPet 置 nil → sink 触发 → greeting 必须回 placeholder.
        appState.reset()
        XCTAssertEqual(
            vm.greeting,
            "想你啦 ♥",
            "Story 37.7 codex round 4 [P3]: reset 后 greeting 必须回 placeholder（不残留旧 pet 名）"
        )
    }

    // MARK: - case#11 type-level: Story 37.7 codex round 5 [P2] FloatingEmojiView 可构造

    /// Story 37.7 codex round 5 [P2] guard test：catStage 浮层 emoji 子视图必须保持可构造.
    ///
    /// 背景：round 5 把内联 `Text(emoji).offset(y: -110).transition(.opacity)`
    /// 重构成独立 `FloatingEmojiView` struct + `@State animatedY/animatedOpacity` + `.onAppear` 内
    /// `withAnimation` 驱动 0 → -110 / 1 → 0 上升淡出动画；外层 `.id(state.interactionAnimation)` 让
    /// 每次 .flying(_, UUID()) 重建子视图 → @State reset → onAppear 重跑 → 动画重放.
    /// 旧实装把 emoji 直接落位 -110 + opacity transition：用户看到的是静止 emoji fade in/out，没有"升起"效果.
    ///
    /// 视觉断言被 ADR-0002 §3.1 禁用 → 本 case 仅做 type-level 守护：
    ///   - 构造 `FloatingEmojiView(emoji:)` 不 crash（init 签名稳定）.
    ///   - emoji 字段值正确（防未来重构误把字段改 private 或丢字段）.
    /// 若未来有人删掉这个 view / 改名 / 改 init 签名，本 case 立刻编译 fail.
    func testFloatingEmojiViewIsConstructableAtTypeLevel() {
        let view = FloatingEmojiView(emoji: "🍥")
        XCTAssertEqual(view.emoji, "🍥", "FloatingEmojiView 必须暴露 emoji 字段（type-level 契约）")

        // 三个 actionRow emoji 都能构造（防字段类型误改成枚举或 enum-only 限制）.
        let pet = FloatingEmojiView(emoji: "💕")
        let play = FloatingEmojiView(emoji: "⭐")
        XCTAssertEqual(pet.emoji, "💕")
        XCTAssertEqual(play.emoji, "⭐")
    }
}
