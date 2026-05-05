// StepSyncIntegrationTests.swift
// Story 8.5 AC11: 步数同步链路 UITest（XCUITest + launch env 路径）.
//
// 范围：本 UITest 验证 App 启动后 service.start() → useCase → repo → mock /steps/sync
//        → AppState.currentStepAccount 写入 → HomeView stepBalance 显示更新.
//
// 不在本 UITest 范围:
//   - 5min 定时器触发（实跑超时；epic-9 跨端 e2e 验证）
//   - scenePhase 切换（XCUITest 模拟 background/active 不可靠；epic-9 验证）
//   - 真实 server 调（e2e 阶段；本测试用 launch env 注入 mock 路径）
//
// AppContainer 在 DEBUG build 内对 `UITEST_MOCK_STEP_SYNC=1` launch env 做特殊处理:
//   - 注入 UITestMockStepRepository（替代 DefaultStepRepository）
//   - 注入 UITestMockHealthProvider（替代 HealthProviderImpl）—— 避免 sim 权限拒绝导致 silent 失败.
//   - 同时自动跳过 guest login（设 UITEST_SKIP_GUEST_LOGIN=1），让 launchStateMachine 立即 ready.

import XCTest

final class StepSyncIntegrationTests: XCTestCase {

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    /// happy: App 启动 → service.start() 触发同步 → mock 返新 stepAccount
    /// → AppState.currentStepAccount 写入 → HomeView stepBalance 显示更新.
    func testAppLaunch_triggersStepSync_updatesHomeViewStepBalance() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launchEnvironment["UITEST_MOCK_STEP_SYNC"] = "1"
        app.launchEnvironment["UITEST_MOCK_HEALTH_STEPS"] = "1234"
        app.launchEnvironment["UITEST_MOCK_SYNC_RESPONSE_AVAILABLE"] = "5678"
        app.launch()

        let timeout: TimeInterval = 10

        // stepBalance 区块（Story 37.7 落地的 a11y identifier）必须出现.
        // iOS 26 simulator 上父 statusBar `.accessibilityElement(children: .contain)` 会让 capsule 内
        // 3 个子 element（footprint Image + "5678" StaticText + "步" StaticText）都继承 `home_stepBalance`
        // identifier —— 直接 `[id]` 访问 .label 会因为多 match 抛错；必须用 .matching + 类型 + label predicate.
        let stepBalance = app.descendants(matching: .any)["home_stepBalance"]
        XCTAssertTrue(stepBalance.waitForExistence(timeout: timeout),
            "stepBalance 区块未找到")

        // 等异步同步完成 → AppState.applySyncedStepAccount 写入 5678 → HomeView 重渲染 →
        // capsule 内 staticText（identifier=home_stepBalance, label="5,678" 或 "5678"）出现.
        // 注：HomeView stepBalanceCapsule 渲染 `Text("\(...availableSteps ?? 0)")` —— Swift Int 默认
        // 字符串化是无千位分隔符（"5678"），但 SwiftUI Text + 系统 a11y 合成 label 可能加千位分隔符（"5,678"）.
        // 用 NSPredicate CONTAINS "5678" 同时匹配两种形态——5,678 和 5678.
        let pollDeadline = Date().addingTimeInterval(timeout)
        let staticTextPredicate = NSPredicate(
            format: "label CONTAINS[c] %@ AND identifier == %@",
            "5,678",  // iOS 自动加千位分隔符的形态
            "home_stepBalance"
        )
        let plainPredicate = NSPredicate(
            format: "label CONTAINS[c] %@ AND identifier == %@",
            "5678",
            "home_stepBalance"
        )
        while Date() < pollDeadline {
            let withComma = app.descendants(matching: .staticText).matching(staticTextPredicate)
            let withoutComma = app.descendants(matching: .staticText).matching(plainPredicate)
            if withComma.count > 0 || withoutComma.count > 0 {
                return  // success
            }
            Thread.sleep(forTimeInterval: 0.3)
        }
        XCTFail("step sync 后主界面 home_stepBalance staticText 应含 '5678' 或 '5,678'；超时未找到")
    }
}
