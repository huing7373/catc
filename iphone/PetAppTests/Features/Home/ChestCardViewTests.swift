// ChestCardViewTests.swift
// Story 21.1 AC7: ChestCardView + ChestTimerDriver + HomeViewModel.chestRemainingSeconds 单元测试.
//
// 约束（与 Story 37.7 / 8.4 衔接）：
//   - 仅 XCTest + @testable import PetApp；不引 ViewInspector / SnapshotTesting.
//   - 不走 SwiftUI body 内省；走 props / ViewModel 字段 / Driver 行为断言.
//   - 视觉契约（counting / unlockable 颜色 / 图标）由 #Preview + Story 37.13 visual-review-checklist + AC10 ios-simulator MCP 验证.

import XCTest
@testable import PetApp

@MainActor
final class ChestCardViewTests: XCTestCase {

    // MARK: - case#1 happy: counting 态 props 构造合法（视觉断言由 Preview/MCP 兜底）

    /// 验证 ChestCardView 用 counting + remainingSeconds=300 构造不 crash + formatter 派生路径行为符合 spec.
    /// 视觉断言（mm:ss 渲染 "05:00"）由 Preview / MCP 兜底；本测试断言 init 路径完整 + formatter 行为正确.
    func testChestCardViewConstructsWithCountingProps() {
        let chest = HomeChest(
            id: "c1",
            status: .counting,
            unlockAt: Date().addingTimeInterval(300),
            openCostSteps: 1000,
            remainingSeconds: 300
        )
        // 构造合法即不 crash（ChestCardView 是 struct，构造层契约锁定与 PetSpriteView 同模式）.
        _ = ChestCardView(currentChest: chest, remainingSeconds: 300, onOpenTap: {})
        // formatter 行为断言（间接验证 mm:ss 派生路径）.
        XCTAssertEqual(ChestCardView.formatMMSSForTesting(300), "05:00")
        XCTAssertEqual(ChestCardView.formatMMSSForTesting(65), "01:05")
        XCTAssertEqual(ChestCardView.formatMMSSForTesting(0), "00:00")
        XCTAssertEqual(ChestCardView.formatMMSSForTesting(-1), "00:00")   // 负值钳到 0
    }

    // MARK: - case#2 happy: HomeViewModel.chestRemainingSeconds 默认 0

    /// 验证 HomeViewModel 基类 chestRemainingSeconds 默认值 0（Story 21.1 AC1 钦定）.
    func testHomeViewModelChestRemainingSecondsDefaultsToZero() {
        let vm = HomeViewModel(
            nickname: "test",
            appVersion: "0.0.0",
            serverInfo: "test"
        )
        XCTAssertEqual(vm.chestRemainingSeconds, 0)
    }

    // MARK: - case#3 happy: ChestTimerDriver appState.currentChest 切换时 viewModel.chestRemainingSeconds 更新

    /// 验证 driver 订阅 appState.$currentChest，currentChest 变化触发立即重算 + 写 viewModel.chestRemainingSeconds.
    /// 不等待真实秒数过去；通过 appState.currentChest = newValue 触发 Combine sink 路径.
    func testChestTimerDriverUpdatesRemainingSecondsOnChestChange() async {
        let appState = AppState()
        let vm = HomeViewModel(nickname: "t", appVersion: "0", serverInfo: "t")
        let driver = ChestTimerDriver(appState: appState, viewModel: vm)
        driver.start()
        // 设 unlockAt 在未来 300 秒 → driver 立即重算 → chestRemainingSeconds 应为 ~300（容差 1 秒）.
        let unlockAt = Date().addingTimeInterval(300)
        appState.currentChest = HomeChest(
            id: "c1",
            status: .counting,
            unlockAt: unlockAt,
            openCostSteps: 1000,
            remainingSeconds: 300
        )
        // 等 Combine sink 在 main run loop 派发完成（runUntilTickleScheduled 模式）.
        try? await Task.sleep(nanoseconds: 50_000_000)   // 50ms 让 sink 跑完
        XCTAssertGreaterThanOrEqual(vm.chestRemainingSeconds, 299)
        XCTAssertLessThanOrEqual(vm.chestRemainingSeconds, 300)
        driver.stop()
    }

    // MARK: - case#4 happy: ChestTimerDriver appState.currentChest = nil 时 chestRemainingSeconds 归零

    /// 验证 currentChest 被清空（如登出 / reset）→ driver 立即写 chestRemainingSeconds = 0 + 不启 Task.
    func testChestTimerDriverWritesZeroWhenChestNiled() async {
        let appState = AppState()
        let vm = HomeViewModel(nickname: "t", appVersion: "0", serverInfo: "t")
        let driver = ChestTimerDriver(appState: appState, viewModel: vm)
        driver.start()
        appState.currentChest = HomeChest(
            id: "c1",
            status: .counting,
            unlockAt: Date().addingTimeInterval(60),
            openCostSteps: 1000,
            remainingSeconds: 60
        )
        try? await Task.sleep(nanoseconds: 50_000_000)
        XCTAssertGreaterThan(vm.chestRemainingSeconds, 0)
        // 清空 → driver 应立即写 0.
        appState.currentChest = nil
        try? await Task.sleep(nanoseconds: 50_000_000)
        XCTAssertEqual(vm.chestRemainingSeconds, 0)
        driver.stop()
    }

    // MARK: - case#5 happy: unlockable 视觉派生（status-aware 双轴判定；review r2 P2 修订）

    /// 真值表（review r2 推翻 r1 over-correction）：
    ///
    ///   | status      | remainingSeconds | isUnlockable | 业务语义                                |
    ///   |-------------|------------------|--------------|----------------------------------------|
    ///   | .unlockable | 0                | true         | server 权威 unlockable（典型）           |
    ///   | .unlockable | 999              | true         | server 权威覆盖一切（防御性 case）        |
    ///   | .counting   | 300              | false        | 倒计时进行中（hydrate 后 driver 写入正值） |
    ///   | .counting   | 1                | false        | 倒计时边界但未到 0                       |
    ///   | .counting   | 0                | true         | 本地 driver tick 归零乐观切（epic AC 钦定）|
    ///   | .counting   | -5               | true         | 负值（防 system clock 跳跃，钳到归零语义） |
    ///
    /// 关键 hydrate 帧 chestRemainingSeconds=0 默认值**不会**误判 unlockable —— 因为
    /// ChestTimerDriver.start() 同步初始化让 chestRemainingSeconds 在 RealHomeViewModel.init
    /// 返回前就拿到 server 推下来的真实初值（如 300），不会停在 0. 见 case#7 验证.
    func testChestCardViewUnlockableDerivation() {
        // status == .unlockable → 总是 true（server 权威）
        XCTAssertTrue(ChestCardView.isUnlockableForTesting(status: .unlockable, remainingSeconds: 0))
        XCTAssertTrue(ChestCardView.isUnlockableForTesting(status: .unlockable, remainingSeconds: 999))

        // status == .counting && remainingSeconds > 0 → false（倒计时进行中）
        XCTAssertFalse(ChestCardView.isUnlockableForTesting(status: .counting, remainingSeconds: 300))
        XCTAssertFalse(ChestCardView.isUnlockableForTesting(status: .counting, remainingSeconds: 1))

        // status == .counting && remainingSeconds <= 0 → true（本地 tick 归零乐观切）
        XCTAssertTrue(ChestCardView.isUnlockableForTesting(status: .counting, remainingSeconds: 0))
        XCTAssertTrue(ChestCardView.isUnlockableForTesting(status: .counting, remainingSeconds: -5))
    }

    // MARK: - case#6 happy: RealHomeViewModel 构造时 ChestTimerDriver 自动启动 + chestRemainingSeconds 从 appState 派生

    /// 验证 RealHomeViewModel 构造时 driver 自动创建并启动；appState.currentChest 已 hydrate 时立即拉到 remainingSeconds.
    func testRealHomeViewModelStartsDriverOnInit() async {
        let appState = AppState()
        appState.currentChest = HomeChest(
            id: "c1",
            status: .counting,
            unlockAt: Date().addingTimeInterval(120),
            openCostSteps: 1000,
            remainingSeconds: 120
        )
        let vm = RealHomeViewModel(appState: appState)
        try? await Task.sleep(nanoseconds: 50_000_000)
        XCTAssertGreaterThanOrEqual(vm.chestRemainingSeconds, 119)
        XCTAssertLessThanOrEqual(vm.chestRemainingSeconds, 120)
    }

    // MARK: - case#7 critical: ChestTimerDriver.start() 同步初始化（review r2 配套）

    /// 关键不变量（review r2 P2 配套修复）：appState.currentChest 已 hydrate 时调 driver.start()
    /// 必须**同步**（在 start() 返回前）把 viewModel.chestRemainingSeconds 写成真实值,
    /// **不**等 Combine sink 的下一 main runloop 派发.
    ///
    /// 不这么做的后果：ChestCardView 在 RealHomeViewModel.init 后第一帧 body 求值时会读到
    /// chestRemainingSeconds=0（@Published 默认值），结合 status-aware 派生
    /// `(.counting && remainingSeconds <= 0) → unlockable` 会闪一帧金色 unlockable 卡片.
    func testChestTimerDriverInitializesSynchronouslyOnStart() {
        let appState = AppState()
        appState.currentChest = HomeChest(
            id: "c1",
            status: .counting,
            unlockAt: Date().addingTimeInterval(240),
            openCostSteps: 1000,
            remainingSeconds: 240
        )
        let vm = HomeViewModel(nickname: "t", appVersion: "0", serverInfo: "t")
        XCTAssertEqual(vm.chestRemainingSeconds, 0)  // 默认值
        let driver = ChestTimerDriver(appState: appState, viewModel: vm)
        driver.start()
        // 关键断言：start() 返回前就已经写入真实值；**无 await / sleep**.
        XCTAssertGreaterThanOrEqual(vm.chestRemainingSeconds, 239)
        XCTAssertLessThanOrEqual(vm.chestRemainingSeconds, 240)
        driver.stop()
    }

    // MARK: - case#8 critical: subsequent currentChest 变化时 sink 必须同步（review r3 配套）

    /// 关键不变量（review r3 P2 配套修复）：driver.start() 之后 `appState.currentChest` 再次被
    /// 改写（例如 HomeViewModel.refresh / Story 21.2 LoadChestUseCase 60s 拉取装入新 `.counting`
    /// 宝箱）时，sink closure 必须**同步**在 `currentChest` setter 调用栈内跑完 → driver 同步
    /// 把 `viewModel.chestRemainingSeconds` 写成新值. **不**走 await / sleep，**不**等下个 runloop.
    ///
    /// 不这么做的后果（review r3 finding）：SwiftUI 观察 `AppState.currentChest` 触发的 rerender
    /// 与 sink closure 的执行没有 happens-before 关系；ChestCardView 可能先看到新
    /// `currentChest = .counting(300)`，但配套的 `chestRemainingSeconds` 还是上一帧的 stale 0,
    /// `isUnlockable(.counting, 0) == true` → 闪一帧金色 unlockable 卡片.
    ///
    /// 反弹守门：任何未来 PR 重新加 `.receive(on:)` / `.async` / 其他异步 operator 到 sink 链
    /// 都会让本测试立刻挂. 配合 case#7 的"初始化同步性"形成双轴防 over-correction 反弹.
    func testChestTimerDriverPropagatesSubsequentChestChangeSynchronously() {
        let appState = AppState()
        let vm = HomeViewModel(nickname: "t", appVersion: "0", serverInfo: "t")
        let driver = ChestTimerDriver(appState: appState, viewModel: vm)
        driver.start()
        // start() 返回时 currentChest 仍 nil → chestRemainingSeconds = 0（同步初始化路径）.
        XCTAssertEqual(vm.chestRemainingSeconds, 0)

        // 模拟 subsequent hydration：start() 之后 appState.currentChest 被改写（如 /home 刷新）.
        appState.currentChest = HomeChest(
            id: "c1",
            status: .counting,
            unlockAt: Date().addingTimeInterval(180),
            openCostSteps: 1000,
            remainingSeconds: 180
        )
        // 关键断言：setter 返回 == sink closure 已同步跑完 == chestRemainingSeconds 已写入正值.
        // **无 await / sleep**. 如果 sink 链上有任何 `.receive(on:)` / async hop，
        // chestRemainingSeconds 会停在 0，本测试挂.
        XCTAssertGreaterThanOrEqual(vm.chestRemainingSeconds, 179)
        XCTAssertLessThanOrEqual(vm.chestRemainingSeconds, 180)

        // 再换一次（模拟 Story 21.2 60s 拉取装入新 chest）→ 仍然同步.
        appState.currentChest = HomeChest(
            id: "c2",
            status: .counting,
            unlockAt: Date().addingTimeInterval(60),
            openCostSteps: 1000,
            remainingSeconds: 60
        )
        XCTAssertGreaterThanOrEqual(vm.chestRemainingSeconds, 59)
        XCTAssertLessThanOrEqual(vm.chestRemainingSeconds, 60)

        driver.stop()
    }
}
