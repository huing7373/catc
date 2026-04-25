// NavigationUITests.swift
// Story 2.3 AC7：从主界面点击三个按钮分别弹 Sheet + 关闭按钮回到主界面。
//
// AccessibilityID.swift 通过 project.yml 直接作为 UITest target 的 source 引入（Story 2.2 已落地），
// 不需要 @testable import PetApp（UI 测试以黑盒方式跑被测 App）。

import XCTest

final class NavigationUITests: XCTestCase {

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    private let timeout: TimeInterval = 5

    // MARK: - testTapEnterRoomShowsRoomPlaceholder

    func testTapEnterRoomShowsRoomPlaceholder() throws {
        let app = XCUIApplication()
        app.launch()

        let btnRoom = app.buttons[AccessibilityID.Home.btnRoom]
        XCTAssertTrue(btnRoom.waitForExistence(timeout: timeout), "进入房间按钮未找到")
        btnRoom.tap()

        let roomTitle = app.descendants(matching: .any)[AccessibilityID.SheetPlaceholder.roomTitle]
        XCTAssertTrue(roomTitle.waitForExistence(timeout: timeout), "Room placeholder 标题未出现")

        XCTAssertTrue(app.staticTexts["Room Placeholder"].exists, "Room Placeholder 文案应可见")

        let btnClose = app.buttons[AccessibilityID.SheetPlaceholder.btnClose]
        XCTAssertTrue(btnClose.waitForExistence(timeout: timeout), "关闭按钮未找到")
        btnClose.tap()

        // 等待主界面 userInfo 重新可见（间接证明 sheet 已 dismiss）
        let userInfo = app.descendants(matching: .any)[AccessibilityID.Home.userInfo]
        XCTAssertTrue(userInfo.waitForExistence(timeout: timeout), "关闭后主界面 userInfo 未恢复")
    }

    // MARK: - testTapInventoryShowsInventoryPlaceholder

    func testTapInventoryShowsInventoryPlaceholder() throws {
        let app = XCUIApplication()
        app.launch()

        let btnInventory = app.buttons[AccessibilityID.Home.btnInventory]
        XCTAssertTrue(btnInventory.waitForExistence(timeout: timeout), "仓库按钮未找到")
        btnInventory.tap()

        let inventoryTitle = app.descendants(matching: .any)[AccessibilityID.SheetPlaceholder.inventoryTitle]
        XCTAssertTrue(inventoryTitle.waitForExistence(timeout: timeout), "Inventory placeholder 标题未出现")

        XCTAssertTrue(app.staticTexts["Inventory Placeholder"].exists, "Inventory Placeholder 文案应可见")

        let btnClose = app.buttons[AccessibilityID.SheetPlaceholder.btnClose]
        XCTAssertTrue(btnClose.waitForExistence(timeout: timeout), "关闭按钮未找到")
        btnClose.tap()

        let userInfo = app.descendants(matching: .any)[AccessibilityID.Home.userInfo]
        XCTAssertTrue(userInfo.waitForExistence(timeout: timeout), "关闭后主界面 userInfo 未恢复")
    }

    // MARK: - testTapComposeShowsComposePlaceholder

    func testTapComposeShowsComposePlaceholder() throws {
        let app = XCUIApplication()
        app.launch()

        let btnCompose = app.buttons[AccessibilityID.Home.btnCompose]
        XCTAssertTrue(btnCompose.waitForExistence(timeout: timeout), "合成按钮未找到")
        btnCompose.tap()

        let composeTitle = app.descendants(matching: .any)[AccessibilityID.SheetPlaceholder.composeTitle]
        XCTAssertTrue(composeTitle.waitForExistence(timeout: timeout), "Compose placeholder 标题未出现")

        XCTAssertTrue(app.staticTexts["Compose Placeholder"].exists, "Compose Placeholder 文案应可见")

        let btnClose = app.buttons[AccessibilityID.SheetPlaceholder.btnClose]
        XCTAssertTrue(btnClose.waitForExistence(timeout: timeout), "关闭按钮未找到")
        btnClose.tap()

        let userInfo = app.descendants(matching: .any)[AccessibilityID.Home.userInfo]
        XCTAssertTrue(userInfo.waitForExistence(timeout: timeout), "关闭后主界面 userInfo 未恢复")
    }
}
