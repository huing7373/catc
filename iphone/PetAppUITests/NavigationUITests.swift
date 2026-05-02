// NavigationUITests.swift
// Story 2.3 AC7：从主界面点击三个按钮分别弹 Sheet（已删除）.
//
// Story 37.3 改写（ADR-0009 §3.5 步骤 1 + AC7）：
//   - 删除 testTapEnterRoomShowsRoomPlaceholder / testTapInventoryShowsInventoryPlaceholder /
//     testTapComposeShowsComposePlaceholder 三个旧用例（SheetType .room/.inventory case 已删；
//     主入口 IA 改 4 Tab + HomeContainerView 互斥状态机 + JoinRoomModal sheet）.
//   - 加 testFourTabsAreLocatable 用例：启动 → 4 Tab a11y identifier 全可定位.
//   - 加 testSwitchToWardrobeTab / testSwitchToFriendsTab / testSwitchToProfileTab 三用例：
//     tap Tab → 验证对应 placeholder view 出现.
//
// AccessibilityID.swift 通过 project.yml 直接作为 UITest target 的 source 引入（Story 2.2 已落地）;
// Story 37.13: 4 Tab + scaffold view a11y identifier 全部改用 AccessibilityID 常量引用.

import XCTest

final class NavigationUITests: XCTestCase {

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    private let timeout: TimeInterval = 5

    // MARK: - 4 Tab 可定位（AC7 第 2 条）

    /// 启动后 4 个 Tab a11y identifier (`tab_home` / `tab_wardrobe` / `tab_friends` / `tab_profile`) 全在.
    func testFourTabsAreLocatable() throws {
        let app = XCUIApplication()
        // Story 5.2 hook：UITest 不依赖真实 server / GuestLoginUseCase.
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launch()

        // FloatingTabBar 自绘 button → app.buttons 集合可定位.
        let homeTab = app.buttons[AccessibilityID.Tab.home]
        XCTAssertTrue(homeTab.waitForExistence(timeout: timeout), "tab_home 未找到")

        let wardrobeTab = app.buttons[AccessibilityID.Tab.wardrobe]
        XCTAssertTrue(wardrobeTab.waitForExistence(timeout: timeout), "tab_wardrobe 未找到")

        let friendsTab = app.buttons[AccessibilityID.Tab.friends]
        XCTAssertTrue(friendsTab.waitForExistence(timeout: timeout), "tab_friends 未找到")

        let profileTab = app.buttons[AccessibilityID.Tab.profile]
        XCTAssertTrue(profileTab.waitForExistence(timeout: timeout), "tab_profile 未找到")
    }

    // MARK: - 切到 Wardrobe Tab 验证 WardrobeView 出现（AC7 第 3 条）

    func testSwitchToWardrobeTabShowsWardrobeView() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launch()

        let wardrobeTab = app.buttons[AccessibilityID.Tab.wardrobe]
        XCTAssertTrue(wardrobeTab.waitForExistence(timeout: timeout), "tab_wardrobe 未找到")
        wardrobeTab.tap()

        let wardrobeView = app.descendants(matching: .any)[AccessibilityID.Wardrobe.view]
        XCTAssertTrue(wardrobeView.waitForExistence(timeout: timeout), "切到 Wardrobe Tab 后 wardrobeView 未出现")
    }

    // MARK: - 切到 Friends Tab 验证 FriendsView 出现

    func testSwitchToFriendsTabShowsFriendsView() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launch()

        let friendsTab = app.buttons[AccessibilityID.Tab.friends]
        XCTAssertTrue(friendsTab.waitForExistence(timeout: timeout), "tab_friends 未找到")
        friendsTab.tap()

        let friendsView = app.descendants(matching: .any)[AccessibilityID.Friends.view]
        XCTAssertTrue(friendsView.waitForExistence(timeout: timeout), "切到 Friends Tab 后 friendsView 未出现")
    }

    // MARK: - 切到 Profile Tab 验证 ProfileView 出现

    func testSwitchToProfileTabShowsProfileView() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launch()

        let profileTab = app.buttons[AccessibilityID.Tab.profile]
        XCTAssertTrue(profileTab.waitForExistence(timeout: timeout), "tab_profile 未找到")
        profileTab.tap()

        let profileView = app.descendants(matching: .any)[AccessibilityID.Profile.view]
        XCTAssertTrue(profileView.waitForExistence(timeout: timeout), "切到 Profile Tab 后 profileView 未出现")
    }

    // MARK: - Tab 切换可逆：home → wardrobe → home

    /// 验证从 wardrobe Tab 切回 home Tab 后 home_userInfo 仍可定位（HomeView 重新可见）.
    func testTabSwitchBackToHomeRecoversHomeView() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launch()

        // 1. 初始应在 home Tab —— home_userInfo 可见
        let userInfo = app.descendants(matching: .any)[AccessibilityID.Home.userInfo]
        XCTAssertTrue(userInfo.waitForExistence(timeout: timeout), "初始 home_userInfo 未找到")

        // 2. 切到 wardrobe Tab → wardrobeView 出现
        let wardrobeTab = app.buttons[AccessibilityID.Tab.wardrobe]
        XCTAssertTrue(wardrobeTab.waitForExistence(timeout: timeout), "tab_wardrobe 未找到")
        wardrobeTab.tap()

        let wardrobeView = app.descendants(matching: .any)[AccessibilityID.Wardrobe.view]
        XCTAssertTrue(wardrobeView.waitForExistence(timeout: timeout), "wardrobeView 未出现")

        // 3. 切回 home Tab → home_userInfo 仍可定位
        let homeTab = app.buttons[AccessibilityID.Tab.home]
        XCTAssertTrue(homeTab.waitForExistence(timeout: timeout), "tab_home 未找到")
        homeTab.tap()

        XCTAssertTrue(userInfo.waitForExistence(timeout: timeout), "切回 home Tab 后 home_userInfo 未恢复")
    }
}
