// KeychainPersistenceUITests.swift
// Story 5.1 AC5: 集成测试 —— 在模拟器上验证 KeychainServicesStore 跨进程持久化（NFR7）。
//
// 流程：
// 1. launch App with launchEnvironment["KEYCHAIN_TEST_SEED"] = "test-uid-<rand>"
// 2. App 在 KeychainUITestHookView .onAppear 检测此 env，调 keychainStore.set(...)
// 3. 通过 hidden Text accessibilityIdentifier "uitest_keychain_seed_done" 暴露种入完成信号
// 4. terminate App
// 5. relaunch App with launchEnvironment["KEYCHAIN_TEST_READBACK"] = "1"
// 6. App 检测此 env，调 keychainStore.get(...)，把读到的值塞 hidden Text label
// 7. 测试断言读到的值 == 种入值
//
// 关键：种入与读回 hook 在 PetApp 入口实装时**仅 #if DEBUG** 下生效，避免污染生产代码。
// "卸载重装亦保留" 是 iOS 系统级行为，不属本 story 自动化测试范围 —— 由 Story 6.1 E2E 文档
// 的人工验证场景兜底。

import XCTest

final class KeychainPersistenceUITests: XCTestCase {

    let testGuestUid = "test-uid-\(UUID().uuidString.prefix(8))"

    override func setUp() {
        super.setUp()
        continueAfterFailure = false
    }

    override func tearDown() {
        // 兜底清理：再 launch 一次，触发 reset 按钮把测试残留 keychain 清掉，
        // 避免下一轮测试 / dev 联调遇到脏数据。
        let app = XCUIApplication()
        // Story 5.2 hook：tearDown 想到达 HomeView 才能点 reset 按钮；UITest 不依赖真实 server，
        // 走 Story 2.9 默认 no-op closure → HomeView 直接渲染.
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launch()
        let resetButton = app.buttons[AccessibilityID.Home.btnResetIdentity]
        if resetButton.waitForExistence(timeout: 5) {
            resetButton.tap()
            // 容忍 alert 弹出（Story 2.8 reset 按钮会弹 confirmation alert）
            let alertButton = app.alerts.buttons.firstMatch
            if alertButton.waitForExistence(timeout: 2) {
                alertButton.tap()
            }
        }
        app.terminate()
        super.tearDown()
    }

    func testKeychainPersistsAcrossAppLaunches() {
        // Step 1-3: launch App + 种入 keychain
        let app1 = XCUIApplication()
        app1.launchEnvironment["KEYCHAIN_TEST_SEED"] = testGuestUid
        // Story 5.2 hook：本测试只关注 keychain seed/readback 链路，不关心 launch state machine 是否能完成
        // bootstrap（无 server 时 .needsAuth 也无所谓 —— hook view 在 ZStack 里独立渲染）；显式 skip
        // 让测试时序不被 GuestLoginUseCase 网络超时耽搁，提速 + 减少 flake.
        app1.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app1.launch()

        // 等待"种入完成"信号 element 出现
        let seedDoneLabel = app1.staticTexts["uitest_keychain_seed_done"]
        XCTAssertTrue(
            seedDoneLabel.waitForExistence(timeout: 5),
            "seed-done signal label must appear after KEYCHAIN_TEST_SEED triggered keychain.set()"
        )

        // Step 4: terminate
        app1.terminate()

        // Step 5: relaunch with readback env
        let app2 = XCUIApplication()
        app2.launchEnvironment["KEYCHAIN_TEST_READBACK"] = "1"
        // Story 5.2 hook：同上 step 1，跳过 GuestLoginUseCase 让 readback 测试更稳更快.
        app2.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app2.launch()

        // Step 6-7: 等待 readback element 出现，断言其 label == 种入值
        let readbackLabel = app2.staticTexts["uitest_keychain_readback_value"]
        XCTAssertTrue(
            readbackLabel.waitForExistence(timeout: 5),
            "readback label must appear after KEYCHAIN_TEST_READBACK triggered keychain.get()"
        )
        XCTAssertEqual(
            readbackLabel.label,
            testGuestUid,
            "keychain value must persist across app launches (terminate + relaunch)"
        )

        app2.terminate()
    }
}
