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
}
