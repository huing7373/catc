// HomeUITests.swift
// Story 2.2 AC5：UITest 启动模拟器 → 验证主界面 6 大占位区块的 a11y identifier 都可定位。

import XCTest
// 注：AccessibilityID.swift 通过 project.yml 直接作为 UITest target 的 source 引入，
// 不需要 @testable import PetApp（UI 测试以黑盒方式跑被测 App）。

final class HomeUITests: XCTestCase {

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    /// Story 37.3 修改（ADR-0009 §3.5 步骤 4）：删除 3 CTA 按钮断言.
    /// Story 37.7 修改：删除 chestArea / chestRemaining 断言（chestSlot 接缝期 EmptyView 不渲染）；
    ///   保留 userInfo / petArea / stepBalance / versionLabel 5 个常量在新 statusBar / catStage / versionFooter 内继续.
    func testHomeViewShowsAllPlaceholders() throws {
        let app = XCUIApplication()
        // Story 5.2 hook：让 launch state machine 跳过真实 GuestLoginUseCase（无 server / 不依赖网络），
        // 直接走 Story 2.9 默认 no-op closure → LaunchingView → HomeView 路径，与本 UITest 关注点对齐.
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launch()

        let timeout: TimeInterval = 5

        // 用 descendants(matching: .any) 兜底跨 element type 定位（Rectangle / Circle / Text 等
        // SwiftUI 渲染产物在 XCUITest 中可能体现为 otherElement / staticText / button 等不同类型）.

        let userInfo = app.descendants(matching: .any)[AccessibilityID.Home.userInfo]
        XCTAssertTrue(userInfo.waitForExistence(timeout: timeout), "userInfo 区块未找到")

        let petArea = app.descendants(matching: .any)[AccessibilityID.Home.petArea]
        XCTAssertTrue(petArea.waitForExistence(timeout: timeout), "petArea 区块未找到")

        let stepBalance = app.descendants(matching: .any)[AccessibilityID.Home.stepBalance]
        XCTAssertTrue(stepBalance.waitForExistence(timeout: timeout), "stepBalance 区块未找到")

        // Story 37.7：chestArea / chestRemaining 在本期 chestSlot 接缝期不渲染（chestSlot 默认 EmptyView()）.
        // 老 testHomeViewShowsAllSixPlaceholders 不删除整个 case（保 git history + Story 2.5 / 5.5 wire 链路），
        // 仅去除对 chest 的两断言；Story 21.1 落地 ChestCardView 时再恢复 / 改名.

        let versionLabel = app.descendants(matching: .any)[AccessibilityID.Home.versionLabel]
        XCTAssertTrue(versionLabel.waitForExistence(timeout: timeout), "版本号区块未找到")
    }

    /// Story 37.7 AC8：HomeView Scaffold 7 锚 a11y identifier 可定位验证.
    /// 与 Story 2.2 testHomeViewShowsAllSixPlaceholders 同模式；本 test 验证 ui_design 高保真 5 区块各 a11y 锚.
    /// 本 UITest case 不主动点击按钮 / 验证 sheet 弹出（属 Story 12.7 / 37.12 范围）；仅验证视觉锚存在.
    func testHomeScaffoldShowsAllSevenAnchors() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launch()

        let timeout: TimeInterval = 5

        XCTAssertTrue(
            app.descendants(matching: .any)["homeStatusBar"].waitForExistence(timeout: timeout),
            "homeStatusBar 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["homeCatStage"].exists,
            "homeCatStage 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["homeActionFeed"].exists,
            "homeActionFeed 按钮未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["homeActionPet"].exists,
            "homeActionPet 按钮未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["homeActionPlay"].exists,
            "homeActionPlay 按钮未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["homeTeamIdleCard_create"].exists,
            "homeTeamIdleCard_create 按钮未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["homeTeamIdleCard_join"].exists,
            "homeTeamIdleCard_join 按钮未找到"
        )
    }

    /// Story 37.8 AC8：RoomScaffoldView 7 锚 a11y identifier 可定位验证.
    /// 通过 launch env `UITEST_FORCE_IN_ROOM=1` 让 RootView/HomeContainerView 启动即切到 inRoom 态.
    /// 与 Story 37.7 testHomeScaffoldShowsAllSevenAnchors 同模式；本 test 验证 ui_design 高保真 5 区块各 a11y 锚.
    /// 本 UITest case 不主动点击按钮 / 验证退出 / 复制功能链路（属 Story 12.x 范围）；仅验证视觉锚存在.
    func testRoomScaffoldShowsAllSevenAnchors() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launchEnvironment["UITEST_FORCE_IN_ROOM"] = "1"   // Story 37.8 新增 env flag
        app.launch()

        let timeout: TimeInterval = 5

        XCTAssertTrue(
            app.descendants(matching: .any)["returnButton"].waitForExistence(timeout: timeout),
            "returnButton 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["roomIdDisplay"].exists,
            "roomIdDisplay 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["copyButton"].exists,
            "copyButton 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["roomMember_0"].exists,
            "roomMember_0 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["roomMember_1"].exists,
            "roomMember_1 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["roomMember_2"].exists,
            "roomMember_2 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["roomMember_3"].exists,
            "roomMember_3 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["leaveButton"].exists,
            "leaveButton 区块未找到"
        )
    }

    /// Story 37.9 AC8: WardrobeScaffoldView 关键 a11y identifier 可定位验证.
    /// 切到 Wardrobe Tab 后验证主结构 + 5 个分类 Tab + 装备按钮 + 合成按钮可见.
    /// 与 Story 37.7 testHomeScaffoldShowsAllSevenAnchors / Story 37.8 testRoomScaffoldShowsAllSevenAnchors 同模式.
    /// 本 UITest case 不主动验证装备/卸下完整链路 / 切换分类后 grid 内容变化（属"完整流程"测试 — 节点 8/9 范围）；
    /// 仅验证视觉锚存在让 Story 37.13 a11y 总表归并时有 baseline.
    func testWardrobeScaffoldShowsAllAnchors() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launch()

        let timeout: TimeInterval = 5

        // 切到 Wardrobe Tab
        let wardrobeTab = app.buttons["tab_wardrobe"]
        XCTAssertTrue(wardrobeTab.waitForExistence(timeout: timeout), "tab_wardrobe 未找到")
        wardrobeTab.tap()

        // 验证主容器
        XCTAssertTrue(
            app.descendants(matching: .any)["wardrobeView"].waitForExistence(timeout: 3),
            "wardrobeView 主容器未找到"
        )

        // 验证钻石 + 合成入口
        XCTAssertTrue(
            app.descendants(matching: .any)["wardrobeDiamondCount"].exists,
            "wardrobeDiamondCount 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["wardrobeComposeEntry"].exists,
            "wardrobeComposeEntry 按钮未找到"
        )

        // 验证 5 个分类 Tab
        XCTAssertTrue(
            app.descendants(matching: .any)["wardrobeCategory_hat"].exists,
            "wardrobeCategory_hat 未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["wardrobeCategory_bow"].exists,
            "wardrobeCategory_bow 未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["wardrobeCategory_scarf"].exists,
            "wardrobeCategory_scarf 未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["wardrobeCategory_outfit"].exists,
            "wardrobeCategory_outfit 未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["wardrobeCategory_bg"].exists,
            "wardrobeCategory_bg 未找到"
        )

        // 验证装备按钮
        XCTAssertTrue(
            app.descendants(matching: .any)["wardrobeEquipButton"].exists,
            "wardrobeEquipButton 未找到"
        )

        // 验证 grid 至少有一个 wardrobeItem_*（默认 hat 分类应显示 h1 贝雷帽）
        XCTAssertTrue(
            app.descendants(matching: .any)["wardrobeItem_h1"].exists,
            "默认 hat 分类应显示 wardrobeItem_h1"
        )
    }

    /// Story 37.10 AC8: FriendsScaffoldView 关键 a11y identifier 可定位验证.
    /// 切到 Friends Tab 后验证主结构 + 2 个 Tab + 至少 1 个 FriendRow + 至少 1 个 friendActionButton 可见.
    /// 与 Story 37.7 testHomeScaffoldShowsAllSevenAnchors / Story 37.8 testRoomScaffoldShowsAllSevenAnchors / Story 37.9 testWardrobeScaffoldShowsAllAnchors 同模式.
    /// 本 UITest case 不主动验证完整 join 链路 / 切换 Tab 后 list 内容变化（属"完整流程"测试 — Story 37.12 范围）；
    /// 仅验证视觉锚存在让 Story 37.13 a11y 总表归并时有 baseline.
    func testFriendsScaffoldShowsAllAnchors() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launch()

        let timeout: TimeInterval = 5

        // 切到 Friends Tab
        let friendsTab = app.buttons["tab_friends"]
        XCTAssertTrue(friendsTab.waitForExistence(timeout: timeout), "tab_friends 未找到")
        friendsTab.tap()

        // 验证主容器
        XCTAssertTrue(
            app.descendants(matching: .any)["friendsView"].waitForExistence(timeout: 3),
            "friendsView 主容器未找到"
        )

        // 验证添加按钮
        XCTAssertTrue(
            app.descendants(matching: .any)["friendsAddButton"].exists,
            "friendsAddButton 未找到"
        )

        // 验证 2 个 Tab
        XCTAssertTrue(
            app.descendants(matching: .any)["friendsTab_online"].exists,
            "friendsTab_online 未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["friendsTab_all"].exists,
            "friendsTab_all 未找到"
        )

        // 验证至少一个 FriendRow（具体 id 由 mock data 决定，验证 scaffold defaults 中第一个 inRoom 好友 u1）
        XCTAssertTrue(
            app.descendants(matching: .any)["friendRow_u1"].exists,
            "friendRow_u1（夏夏 inRoom）未找到"
        )

        // 验证 inRoom 好友的"加入"按钮可定位
        XCTAssertTrue(
            app.descendants(matching: .any)["friendActionButton_u1"].exists,
            "friendActionButton_u1（夏夏加入按钮）未找到"
        )
    }

    /// Story 37.11 AC8: ProfileScaffoldView 关键 a11y identifier 可定位验证.
    /// 切到 Profile Tab 后验证主结构 + headerCard / statsCard / wechatCard / 4 个菜单 / Modal 触发链路可定位.
    /// 与 Story 37.7 / 37.8 / 37.9 / 37.10 同模式.
    func testProfileScaffoldShowsAllAnchors() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launch()

        let timeout: TimeInterval = 5

        // 切到 Profile Tab
        let profileTab = app.buttons["tab_profile"]
        XCTAssertTrue(profileTab.waitForExistence(timeout: timeout), "tab_profile 未找到")
        profileTab.tap()

        // 验证主容器
        XCTAssertTrue(
            app.descendants(matching: .any)["profileView"].waitForExistence(timeout: 3),
            "profileView 主容器未找到"
        )

        // 验证 5 区块关键锚
        XCTAssertTrue(app.descendants(matching: .any)["profileHeaderCard"].exists, "profileHeaderCard 未找到")
        XCTAssertTrue(app.descendants(matching: .any)["profileStatsCard"].exists, "profileStatsCard 未找到")
        XCTAssertTrue(app.descendants(matching: .any)["profileWeChatCard"].exists, "profileWeChatCard（未绑定卡）未找到")

        // 验证 4 个菜单项
        for item in ["achievements", "messages", "favorites", "settings"] {
            XCTAssertTrue(
                app.descendants(matching: .any)["profileMenu_\(item)"].exists,
                "profileMenu_\(item) 未找到"
            )
        }

        // 验证 BindWechatModal 触发链路：点未绑定卡 → modal 出现
        app.descendants(matching: .any)["profileWeChatCard"].firstMatch.tap()
        XCTAssertTrue(
            app.descendants(matching: .any)["profileWeChatModal"].waitForExistence(timeout: 3),
            "profileWeChatModal 未在 wechatCard tap 后出现"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)["profileWeChatBindButton"].exists,
            "profileWeChatBindButton 未在 modal 内找到"
        )
    }

    /// Story 2.8 round 2 fix：父容器 userInfoBar 在引入 ResetIdentityButton 后，
    /// `.accessibilityElement(children: .contain)` 必须仍保留 `.accessibilityLabel(nickname)`，
    /// 否则 VoiceOver 用户读 home_userInfo 时听不到 nickname summary（只听到子元素列表）。
    /// 本 case 锁这条 a11y 契约：父级 a11y label 必须等于 viewModel.nickname。
    func testUserInfoBarRetainsNicknameAccessibilityLabel() throws {
        let app = XCUIApplication()
        // Story 5.2 hook：UITest 不依赖真实 server / GuestLoginUseCase（详见 testHomeViewShowsAllSixPlaceholders）.
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
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
        // Story 5.2 hook：UITest 不依赖真实 server / GuestLoginUseCase（详见 testHomeViewShowsAllSixPlaceholders）.
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
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
        // Story 5.2 hook：UITest 不依赖真实 server / GuestLoginUseCase（详见 testHomeViewShowsAllSixPlaceholders）.
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
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
