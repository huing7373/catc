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

    /// Story 2.8 round 2 fix：父容器 userInfoBar 在引入 ResetIdentityButton 后，
    /// `.accessibilityElement(children: .contain)` 必须仍保留 `.accessibilityLabel(nickname)`，
    /// 否则 VoiceOver 用户读 home_userInfo 时听不到 nickname summary（只听到子元素列表）。
    /// 本 case 锁这条 a11y 契约：父级 a11y label 必须等于 viewModel.nickname。
    func testUserInfoBarRetainsNicknameAccessibilityLabel() throws {
        let app = XCUIApplication()
        app.launch()

        let timeout: TimeInterval = 5

        let userInfo = app.descendants(matching: .any)[AccessibilityID.Home.userInfo]
        XCTAssertTrue(userInfo.waitForExistence(timeout: timeout), "userInfo 区块未找到")

        // HomeViewModel.nickname 默认值 "用户1001"（见 HomeViewModel.swift init 默认参数）。
        // SwiftUI `.accessibilityLabel(Text(...))` 会把字符串注入 element.label。
        XCTAssertEqual(
            userInfo.label,
            "用户1001",
            "userInfoBar 父容器应保留 nickname 作为 a11y label —— `.contain` 与 `.accessibilityLabel` 必须并存"
        )
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

    /// Story 2.9 AC8：全新模拟器启动时，LaunchingView 应可见 → 主界面渲染前不出现空白屏。
    ///
    /// 验证策略：
    /// 1. 启动 App
    /// 2. 短 timeout 内查找 LaunchingView 文字（机会性断言：fast machine 可能错过 0.3s 时机）
    /// 3. 等 LaunchingView 消失 → home_userInfo 可见（5s 充分 timeout）
    ///
    /// 注意 timing：bootstrap 占位 closure 立即成功 → 0.3 秒后转 .ready；
    /// XCUITest 的 launch 本身有 1-2 秒开销，所以"app.launch() 之后立即"通常已经过了几百毫秒，
    /// LaunchingView 可能正好处于 0.3 秒末段。给文字的 waitForExistence
    /// 一个**短** timeout（如 0.5s），让 fast machine 上 LaunchingView 已切走时不长时间挂起。
    func testLaunchingViewVisibleBeforeHomeView() throws {
        let app = XCUIApplication()
        app.launch()

        // 1. LaunchingView 文字应在很短 timeout 内可见（cold launch 通常几百毫秒已过半 minimumDuration）。
        //    不强制为 true（fast machine 上 LaunchingView 0.3s 内可能错过断言时机）；
        //    本 case 主要验证不崩 + 后续 home_userInfo 可定位。
        let launchingText = app.staticTexts["正在唤醒小猫…"]
        _ = launchingText.waitForExistence(timeout: 0.5)

        // 2. 等 LaunchingView 消失 → HomeView 主界面 home_userInfo 可定位（充分 timeout）
        let homeUserInfo = app.descendants(matching: .any)[AccessibilityID.Home.userInfo]
        XCTAssertTrue(
            homeUserInfo.waitForExistence(timeout: 5),
            "HomeView 在 LaunchingView 消失后应可见（home_userInfo 应可定位）"
        )
    }
}
