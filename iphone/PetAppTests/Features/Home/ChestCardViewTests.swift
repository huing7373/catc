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

    // MARK: - case#5 happy: unlockable 视觉派生（纯 status 判定；review r1 P2 修订）

    /// 关键不变量：当且仅当 `chest.status == .unlockable` 时为 unlockable 态.
    /// 不再纳入 `remainingSeconds <= 0` 短路 —— 该值默认 0 与"超时 0"语义无法区分，会让 hydrate 阶段
    /// 的 .counting 宝箱被误判 unlockable. server WS / 60s 轮询权威推送 status 切换即可.
    func testChestCardViewUnlockableDerivation() {
        XCTAssertTrue(ChestCardView.isUnlockableForTesting(status: .unlockable))
        XCTAssertFalse(ChestCardView.isUnlockableForTesting(status: .counting))
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
}
