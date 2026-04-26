// HomeUITests.swift
// Story 2.2 AC5：UITest 启动模拟器 → 验证主界面 6 大占位区块的 a11y identifier 都可定位。

import XCTest
// 注：AccessibilityID.swift 通过 project.yml 直接作为 UITest target 的 source 引入，
// 不需要 @testable import PetApp（UI 测试以黑盒方式跑被测 App）。

final class HomeUITests: XCTestCase {

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    func testHomeViewShowsAllSixPlaceholders() throws {
        let app = XCUIApplication()
        app.launch()

        let timeout: TimeInterval = 5

        // 用 descendants(matching: .any) 兜底跨 element type 定位（Rectangle / Circle / Text 等
        // SwiftUI 渲染产物在 XCUITest 中可能体现为 otherElement / staticText / button 等不同类型）。

        let userInfo = app.descendants(matching: .any)[AccessibilityID.Home.userInfo]
        XCTAssertTrue(userInfo.waitForExistence(timeout: timeout), "userInfo 区块未找到")

        let petArea = app.descendants(matching: .any)[AccessibilityID.Home.petArea]
        XCTAssertTrue(petArea.waitForExistence(timeout: timeout), "petArea 区块未找到")

        let stepBalance = app.descendants(matching: .any)[AccessibilityID.Home.stepBalance]
        XCTAssertTrue(stepBalance.waitForExistence(timeout: timeout), "stepBalance 区块未找到")

        let chestArea = app.descendants(matching: .any)[AccessibilityID.Home.chestArea]
        XCTAssertTrue(chestArea.waitForExistence(timeout: timeout), "chestArea 区块未找到")

        let btnRoom = app.buttons[AccessibilityID.Home.btnRoom]
        XCTAssertTrue(btnRoom.waitForExistence(timeout: timeout), "进入房间按钮未找到")

        let btnInventory = app.buttons[AccessibilityID.Home.btnInventory]
        XCTAssertTrue(btnInventory.waitForExistence(timeout: timeout), "仓库按钮未找到")

        let btnCompose = app.buttons[AccessibilityID.Home.btnCompose]
        XCTAssertTrue(btnCompose.waitForExistence(timeout: timeout), "合成按钮未找到")

        let versionLabel = app.descendants(matching: .any)[AccessibilityID.Home.versionLabel]
        XCTAssertTrue(versionLabel.waitForExistence(timeout: timeout), "版本号区块未找到")
    }

    /// Story 2.8 AC10：dev "重置身份" 按钮 + 点击 alert 链路。
    /// XCUITest 默认在 Debug configuration 跑（xcodebuild test 默认 Debug），#if DEBUG 分支生效。
    /// SwiftUI .alert(item:) 在 XCUITest 中表现为 app.alerts 集合；通过 alert 内文字定位。
    func testResetIdentityButtonVisibleAndAlertOnTap() throws {
        let app = XCUIApplication()
        app.launch()

        let timeout: TimeInterval = 5

        // 1. 按钮存在且可点击（AccessibilityID.Home.btnResetIdentity）
        let btn = app.buttons[AccessibilityID.Home.btnResetIdentity]
        XCTAssertTrue(btn.waitForExistence(timeout: timeout), "重置身份按钮未找到（应在 Debug build 渲染）")

        // 2. 点击按钮
        btn.tap()

        // 3. alert 出现（通过 alert 内文字定位 — SwiftUI Alert 的 staticText 含 "已重置" 字样）
        let alertTitle = app.staticTexts["已重置"]
        XCTAssertTrue(alertTitle.waitForExistence(timeout: timeout), "重置成功 alert 未弹出")

        // 4. 点 OK 关闭 alert
        let okButton = app.alerts.buttons["OK"]
        XCTAssertTrue(okButton.waitForExistence(timeout: timeout), "alert OK 按钮未找到")
        okButton.tap()

        // 5. alert 消失（回到主界面，按钮仍存在）
        XCTAssertTrue(btn.waitForExistence(timeout: timeout), "回到主界面后按钮应仍存在")
    }
}
