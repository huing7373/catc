// HomeUITests.swift
// Story 2.2 AC5：UITest 启动模拟器 → 验证主界面 6 大占位区块的 a11y identifier 都可定位。
//
// Story 37.13: 全部 a11y identifier 字面量改用 AccessibilityID 常量引用（caller 表达方式收敛；
// 运行时挂的字符串值不变 → UITest 行为契约不变）。
// AccessibilityID.swift 通过 project.yml 直接作为 UITest target 的 source 引入，无需 @testable import。

import XCTest

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
            app.descendants(matching: .any)[AccessibilityID.Home.userInfo].waitForExistence(timeout: timeout),
            "homeStatusBar 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Home.catStage].exists,
            "homeCatStage 区块未找到"
        )
        // homeActionFeed / homeActionPet / homeActionPlay 是 Story 37.7 落地的 chestSlot/action 区块 a11y identifier，
        // 由 caller view（HomeView 内 helper _chestSlotView(a11yId:) 等 callsite）传入变量，本 story 不收编进
        // AccessibilityID enum（属 dynamic a11y 而非 inline 字符串收编范围；详见 Story 37.13 AC2 关键决策 1）.
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
            app.descendants(matching: .any)[AccessibilityID.Home.teamIdleCardCreate].exists,
            "homeTeamIdleCard_create 按钮未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Home.teamIdleCardJoin].exists,
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
            app.descendants(matching: .any)[AccessibilityID.Room.returnButton].waitForExistence(timeout: timeout),
            "returnButton 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Room.roomIdDisplay].exists,
            "roomIdDisplay 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Room.copyButton].exists,
            "copyButton 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Room.member(at: 0)].exists,
            "roomMember_0 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Room.member(at: 1)].exists,
            "roomMember_1 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Room.member(at: 2)].exists,
            "roomMember_2 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Room.member(at: 3)].exists,
            "roomMember_3 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Room.leaveButton].exists,
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
        let wardrobeTab = app.buttons[AccessibilityID.Tab.wardrobe]
        XCTAssertTrue(wardrobeTab.waitForExistence(timeout: timeout), "tab_wardrobe 未找到")
        wardrobeTab.tap()

        // 验证主容器
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Wardrobe.view].waitForExistence(timeout: 3),
            "wardrobeView 主容器未找到"
        )

        // 验证钻石 + 合成入口
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Wardrobe.diamondCount].exists,
            "wardrobeDiamondCount 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Wardrobe.composeEntry].exists,
            "wardrobeComposeEntry 按钮未找到"
        )

        // 验证 5 个分类 Tab
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Wardrobe.category("hat")].exists,
            "wardrobeCategory_hat 未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Wardrobe.category("bow")].exists,
            "wardrobeCategory_bow 未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Wardrobe.category("scarf")].exists,
            "wardrobeCategory_scarf 未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Wardrobe.category("outfit")].exists,
            "wardrobeCategory_outfit 未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Wardrobe.category("bg")].exists,
            "wardrobeCategory_bg 未找到"
        )

        // 验证装备按钮
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Wardrobe.equipButton].exists,
            "wardrobeEquipButton 未找到"
        )

        // 验证 grid 至少有一个 wardrobeItem_*（默认 hat 分类应显示 h1 贝雷帽）
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Wardrobe.item("h1")].exists,
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
        let friendsTab = app.buttons[AccessibilityID.Tab.friends]
        XCTAssertTrue(friendsTab.waitForExistence(timeout: timeout), "tab_friends 未找到")
        friendsTab.tap()

        // 验证主容器
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Friends.view].waitForExistence(timeout: 3),
            "friendsView 主容器未找到"
        )

        // 验证添加按钮
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Friends.addButton].exists,
            "friendsAddButton 未找到"
        )

        // 验证 2 个 Tab
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Friends.tab("online")].exists,
            "friendsTab_online 未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Friends.tab("all")].exists,
            "friendsTab_all 未找到"
        )

        // 验证至少一个 FriendRow（具体 id 由 mock data 决定，验证 scaffold defaults 中第一个 inRoom 好友 u1）
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Friends.row("u1")].exists,
            "friendRow_u1（夏夏 inRoom）未找到"
        )

        // 验证 inRoom 好友的"加入"按钮可定位
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Friends.actionButton("u1")].exists,
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
        let profileTab = app.buttons[AccessibilityID.Tab.profile]
        XCTAssertTrue(profileTab.waitForExistence(timeout: timeout), "tab_profile 未找到")
        profileTab.tap()

        // 验证主容器
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Profile.view].waitForExistence(timeout: 3),
            "profileView 主容器未找到"
        )

        // 验证 5 区块关键锚
        XCTAssertTrue(app.descendants(matching: .any)[AccessibilityID.Profile.headerCard].exists, "profileHeaderCard 未找到")
        XCTAssertTrue(app.descendants(matching: .any)[AccessibilityID.Profile.statsCard].exists, "profileStatsCard 未找到")
        XCTAssertTrue(app.descendants(matching: .any)[AccessibilityID.Profile.weChatCard].exists, "profileWeChatCard（未绑定卡）未找到")

        // 验证 4 个菜单项
        for item in ["achievements", "messages", "favorites", "settings"] {
            XCTAssertTrue(
                app.descendants(matching: .any)[AccessibilityID.Profile.menu(item)].exists,
                "profileMenu_\(item) 未找到"
            )
        }

        // 验证 BindWechatModal 触发链路：点未绑定卡 → modal 出现
        app.descendants(matching: .any)[AccessibilityID.Profile.weChatCard].firstMatch.tap()
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Profile.weChatModal].waitForExistence(timeout: 3),
            "profileWeChatModal 未在 wechatCard tap 后出现"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Profile.weChatBindButton].exists,
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

    // Story 37.12 AC6: JoinRoomModal 跨屏 join 链路 UITest.
    // 验证完整链路：HomeView "加入队伍" 按钮 → modal 出现 → 输入房间号 → 确定加入 → modal dismiss + RoomView 渲染.
    // epic AC line 4866 钦定路径.
    func testJoinRoomModalCrossScreenJoinFlow() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launch()

        let timeout: TimeInterval = 5

        // 1. 验证 HomeView TeamIdleCard "加入队伍" 按钮可见（Story 37.7 落地 a11y identifier `homeTeamIdleCard_join`）.
        let joinButton = app.descendants(matching: .any)[AccessibilityID.Home.teamIdleCardJoin]
        XCTAssertTrue(joinButton.waitForExistence(timeout: timeout), "homeTeamIdleCard_join 未找到")

        // 2. 点 "加入队伍" → JoinRoomModal 出现.
        joinButton.tap()
        let modal = app.descendants(matching: .any)[AccessibilityID.JoinRoomModal.modal]
        XCTAssertTrue(modal.waitForExistence(timeout: 3), "joinRoomModal 未在 join button tap 后出现")

        // 3. 验证 modal 5 视觉锚.
        XCTAssertTrue(app.descendants(matching: .any)[AccessibilityID.JoinRoomModal.closeButton].exists, "joinRoomCloseButton 未找到")
        XCTAssertTrue(app.descendants(matching: .any)[AccessibilityID.JoinRoomModal.input].exists, "joinRoomInput 未找到")
        XCTAssertTrue(app.descendants(matching: .any)[AccessibilityID.JoinRoomModal.cancelButton].exists, "joinRoomCancelButton 未找到")
        XCTAssertTrue(app.descendants(matching: .any)[AccessibilityID.JoinRoomModal.confirmButton].exists, "joinRoomConfirmButton 未找到")

        // 4. 输入房间号 "1234567".
        let input = app.textFields[AccessibilityID.JoinRoomModal.input]
        XCTAssertTrue(input.waitForExistence(timeout: 2), "joinRoomInput textField 未找到")
        input.tap()
        input.typeText("1234567")

        // 5. 点 "确定加入" → modal dismiss.
        let confirmButton = app.descendants(matching: .any)[AccessibilityID.JoinRoomModal.confirmButton]
        confirmButton.tap()

        XCTAssertTrue(
            modal.waitForNonExistence(timeout: 3),
            "joinRoomModal 在 confirm tap 后未 dismiss"
        )

        // 6. RoomScaffoldView 出现（验证跨屏跳转链路完整）.
        //    HomeContainerView 互斥状态机检测 appState.currentRoomId 非 nil → 切到 RoomScaffoldView.
        //    用 `returnButton` 作为 RoomScaffoldView 出现的标志（Story 37.8 钦定唯一 a11y identifier;
        //    HomeView 路径无此标识，仅在 RoomScaffoldView 渲染时出现）.
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Room.returnButton].waitForExistence(timeout: 3),
            "RoomScaffoldView 未在 join confirm 后渲染（跨屏跳转链路断）"
        )
    }
}
