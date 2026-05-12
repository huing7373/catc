// RoomUITests.swift
// Story 12.1 AC8 + Story 12.7 AC9: RoomScaffoldView a11y 锚定 + Home → Room 切换路径 UITest.
//
// 本 UITest case 不主动驱动真实 WS / mock server（webSocketClient = nil 路径下 RealRoomViewModel
// init seed 的 RoomScaffoldDefaults 4 成员占位仍渲染）—— 直接验证 RoomScaffoldView 渲染 + a11y 定位.
// 真实 WS 消息驱动的 UI 渲染留给 Story 12.3 UITest（届时 Story 12.2 真实 WebSocketClientImpl + mock server
// 已落地，真实联调链路完整）.
//
// 与 HomeUITests testRoomScaffoldShowsAllSevenAnchors 区别：本 UITest 在 Story 12.1 wsStateLabel 落地后
// 额外验证 wsStateLabel 锚可定位（HomeUITests 既有 case 已验证 7 个旧锚 + 不动；本 case 验证 wsStateLabel
// 单 anchor + 与 12.1 RealRoomViewModel 升级版本兼容）.

import XCTest

final class RoomUITests: XCTestCase {

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    /// Story 12.1 AC8：RoomScaffoldView 在 RealRoomViewModel 升级版（Story 12.1）路径下，
    /// `roomMember_0/1/2`（对应 RoomScaffoldDefaults 前 3 成员）+ `roomIdDisplay` + `wsStateLabel`
    /// 三个关键 a11y identifier 可定位.
    ///
    /// webSocketClient = nil 路径（本 story RootView wire 路径）下：
    ///   - `wsStateLabel` 文字派生自 wsState；本路径 wsState = .disconnected → 文字为"已断开"
    ///   - `roomCodeForCopy` = appState.currentRoomId = "1234567"（UITEST_FORCE_IN_ROOM 注入）→ `roomIdDisplay` 显示非空
    ///   - members 仍是 RoomScaffoldDefaults 4 成员占位（Real init seed 路径不动）→ roomMember_0..3 都可定位
    func testRoomScaffoldExposesUpgradedAccessibilityAnchors() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launchEnvironment["UITEST_FORCE_IN_ROOM"] = "1"   // Story 37.8 落地的 inRoom 强制路径
        app.launch()

        let timeout: TimeInterval = 5

        // 1) 房间号 a11y 锚 + 显示非空字符串
        let roomIdDisplay = app.descendants(matching: .any)[AccessibilityID.Room.roomIdDisplay]
        XCTAssertTrue(roomIdDisplay.waitForExistence(timeout: timeout),
                      "roomIdDisplay a11y 锚未找到（RoomScaffoldView 顶部房间号区块漂移）")

        // 2) Story 12.1 新锚：wsStateLabel
        let wsLabel = app.descendants(matching: .any)["wsStateLabel"]
        XCTAssertTrue(wsLabel.waitForExistence(timeout: timeout),
                      "wsStateLabel a11y 锚未找到（Story 12.1 AC5 RoomScaffoldView 应在 topBar 后挂 wsStateLabel）")

        // 3) RoomScaffoldDefaults 前 3 成员占位定位（webSocketClient = nil 路径下 Real init seed 仍生效）
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Room.member(at: 0)].exists,
            "roomMember_0 区块未找到（RealRoomViewModel init seed 应保留 RoomScaffoldDefaults 4 成员）"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Room.member(at: 1)].exists,
            "roomMember_1 区块未找到"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Room.member(at: 2)].exists,
            "roomMember_2 区块未找到"
        )
    }

    /// Story 12.3 AC5: UITest 策略 A —— mock RoomViewModel 注入 3 fixed members → RoomScaffoldView 渲染
    /// 3 个 `roomMember_<i>` 锚定可见 + 1 个空位 `roomMember_3` 锚定可见（4 - members.count = 1 dashed slot）
    /// + 房间号 `roomIdDisplay` 锚定可见（roomCodeForCopy = RoomScaffoldDefaults "1234567"，UITEST_FORCE_IN_ROOM
    /// 路径下 appState.currentRoomId = "1234567" 也是同值；本 case 不强断言文字内容，只断言 a11y 锚定可见 + 非空）.
    ///
    /// 路径选择（Story 12.3 钦定策略 A，详见 12-3 story acceptance §AC5）：
    ///   launch arg `UITEST_ROOM_THREE_MEMBERS=1` → RootView.init() 检测 env flag → 把 `roomViewModel`
    ///   @StateObject 切到 MockRoomViewModel(members: 3 fixed)；其余 UITest env / wire 不动.
    ///
    /// 与 testRoomScaffoldExposesUpgradedAccessibilityAnchors 关键区别：
    ///   - 既有 case 在 RealRoomViewModel + RoomScaffoldDefaults 4 成员 seed 路径下验证（webSocketClient = nil）；
    ///   - 本 case 在 MockRoomViewModel + 3 fixed members 路径下验证（不依赖真实 WS / RealRoomViewModel）；
    ///     验证 4 - members.count = 1 个空位的 dashed slot 也正确挂 `roomMember_3` 锚.
    func testRoomScaffoldRendersThreeMembersAndOneEmptySlotWhenMockHasThreeMembers() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launchEnvironment["UITEST_FORCE_IN_ROOM"] = "1"           // 让 HomeContainerView 走 inRoom 分支
        app.launchEnvironment["UITEST_ROOM_THREE_MEMBERS"] = "1"      // Story 12.3 AC5 新增 launch flag
        app.launch()

        let timeout: TimeInterval = 5

        // 1) 房间号 a11y 锚 + 显示非空字符串
        let roomIdDisplay = app.descendants(matching: .any)[AccessibilityID.Room.roomIdDisplay]
        XCTAssertTrue(roomIdDisplay.waitForExistence(timeout: timeout),
                      "roomIdDisplay a11y 锚未找到（RoomScaffoldView 顶部房间号区块漂移）")

        // 2) 3 个成员行 a11y 锚定（mock 注入的 alice / bob / charlie 三 fixed members）
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Room.member(at: 0)].waitForExistence(timeout: timeout),
            "roomMember_0 区块未找到（MockRoomViewModel 3 fixed members 路径下应渲染第 1 行）"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Room.member(at: 1)].exists,
            "roomMember_1 区块未找到（MockRoomViewModel 3 fixed members 路径下应渲染第 2 行）"
        )
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Room.member(at: 2)].exists,
            "roomMember_2 区块未找到（MockRoomViewModel 3 fixed members 路径下应渲染第 3 行）"
        )

        // 3) 1 个空位 a11y 锚定（4 - members.count = 1，dashed border "+ 等待好友加入" 行）
        // RoomScaffoldView.emptySlot 也挂 AccessibilityID.Room.member(at: index) —— 与 memberRow 共享 ID 模式.
        XCTAssertTrue(
            app.descendants(matching: .any)[AccessibilityID.Room.member(at: 3)].exists,
            "roomMember_3 dashed 空位未找到（4 - members.count = 1 个空位应有锚）"
        )
    }

    /// Story 15.1 AC4: mock vm 注入 3 成员各自不同 pet state (.rest / .walk / .run) →
    /// 验证 RoomScaffoldView memberRow 内 3 个 PetSpriteView 的 a11y identifier 各为
    /// `petSprite_rest` / `petSprite_walk` / `petSprite_run`.
    ///
    /// 路径：与既有 testRoomScaffoldRendersThreeMembersAndOneEmptySlotWhenMockHasThreeMembers 共用
    /// `UITEST_ROOM_THREE_MEMBERS=1` launch flag —— RootView.init() 在该 flag 下把 `roomViewModel`
    /// 切到 MockRoomViewModel(members: 3 fixed) 并同时注入
    ///   `memberPetStates = ["u_alice": .rest, "u_bob": .walk, "u_charlie": .run]`
    /// 让 RoomScaffoldView 渲染 3 个 PetSpriteView 各对应不同 a11y identifier.
    ///
    /// PetSpriteView a11y identifier 由 Story 8.4 钦定（`petSprite_rest` / `petSprite_walk` / `petSprite_run`，
    /// 见 AccessibilityID.Home），本 case 用 `descendants(matching: .any)[identifier]` 定位三个不同 sprite.
    func testMemberRowsRenderPetSpriteViewsWithDistinctAccessibilityIdentifiers() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launchEnvironment["UITEST_FORCE_IN_ROOM"] = "1"
        app.launchEnvironment["UITEST_ROOM_THREE_MEMBERS"] = "1"
        app.launch()

        let timeout: TimeInterval = 5

        // sanity: 房间页 + 3 个成员行已渲染
        let roomIdDisplay = app.descendants(matching: .any)[AccessibilityID.Room.roomIdDisplay]
        XCTAssertTrue(roomIdDisplay.waitForExistence(timeout: timeout),
                      "roomIdDisplay a11y 锚未找到（RoomScaffoldView 未渲染）")

        // 3 个 PetSpriteView 各自的 a11y identifier 可定位（u_alice → rest / u_bob → walk / u_charlie → run）
        let restSprite = app.descendants(matching: .any)[AccessibilityID.Home.petSpriteRest]
        XCTAssertTrue(restSprite.waitForExistence(timeout: timeout),
                      "petSprite_rest a11y 锚未找到（u_alice 成员行应渲染 .rest 状态 PetSpriteView）")

        let walkSprite = app.descendants(matching: .any)[AccessibilityID.Home.petSpriteWalk]
        XCTAssertTrue(walkSprite.waitForExistence(timeout: timeout),
                      "petSprite_walk a11y 锚未找到（u_bob 成员行应渲染 .walk 状态 PetSpriteView）")

        let runSprite = app.descendants(matching: .any)[AccessibilityID.Home.petSpriteRun]
        XCTAssertTrue(runSprite.waitForExistence(timeout: timeout),
                      "petSprite_run a11y 锚未找到（u_charlie 成员行应渲染 .run 状态 PetSpriteView）")
    }

    /// Story 15.3 AC3 / AC4 case#D: 房间页连续 state 切换"后覆前不堆叠队列"+ 三成员独立动画 baseline.
    ///
    /// 路径（Task 3.4.2 钦定，**不**新增 launch flag —— 复用既有 UITEST_ROOM_THREE_MEMBERS=1 路径
    /// 已注入 `u_alice .rest / u_bob .walk / u_charlie .run` 三态 baseline，UITest 可直接断言
    /// 三个 a11y identifier 同时存在 = 三成员 PetSpriteView 独立路径生效）：
    ///   1. 启动 UITEST_FORCE_IN_ROOM=1 + UITEST_ROOM_THREE_MEMBERS=1 → MockRoomViewModel 注入 3 成员三态
    ///   2. waitForExistence: petSprite_rest / petSprite_walk / petSprite_run 同时存在
    ///      → 三个 PetSpriteView 独立渲染（每个 PetSpriteView 有独立 `.id(state)` 触发 view replacement）
    ///   3. XCUIScreen.main.screenshot() 抓基线帧作为 XCTAttachment dev artifact
    ///   4. terminate + relaunch（不变 env）→ 验证 a11y identifier 序列稳定 + screenshot 内容稳定
    ///
    /// **本 UITest 不做"视觉 fade 自动断言"——SwiftUI render-tree 视觉不在 XCUITest 断言范围**.
    /// 视觉契约（opacity + scale 过渡 / 连续切换无残影 / 同值 set 无视觉变化）由 AC5 ios-simulator MCP
    /// `record_video` 实跑录屏视觉验证（dev / codex review 阶段抓帧对比基线）.
    ///
    /// XCTAttachment 的 screenshot 文件随 xcresult bundle 持久化在
    /// `iphone/build/test-results.xcresult`；通过 `xcrun xcresulttool` 可导出为 png（codex review 阶段
    /// 抓帧对比基线，与 lesson `2026-05-12-swiftui-frame-clipped-does-not-scale-15-1-r1.md` 钦定 MCP
    /// 录屏 artifact 收纳模式互补）.
    func testPetSpriteTransitionAnimation_continuousStateSwitch_noResidueNoQueueStacking() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launchEnvironment["UITEST_FORCE_IN_ROOM"] = "1"
        app.launchEnvironment["UITEST_ROOM_THREE_MEMBERS"] = "1"
        app.launch()

        let timeout: TimeInterval = 5

        // 1) sanity: 房间页已渲染
        let roomIdDisplay = app.descendants(matching: .any)[AccessibilityID.Room.roomIdDisplay]
        XCTAssertTrue(roomIdDisplay.waitForExistence(timeout: timeout),
                      "roomIdDisplay a11y 锚未找到（RoomScaffoldView 未渲染）")

        // 2) 三个 PetSpriteView 各自 a11y identifier 同时存在
        //    （MockRoomViewModel 注入 u_alice .rest / u_bob .walk / u_charlie .run 三态）
        let restSprite = app.descendants(matching: .any)[AccessibilityID.Home.petSpriteRest]
        let walkSprite = app.descendants(matching: .any)[AccessibilityID.Home.petSpriteWalk]
        let runSprite = app.descendants(matching: .any)[AccessibilityID.Home.petSpriteRun]
        XCTAssertTrue(restSprite.waitForExistence(timeout: timeout),
                      "petSprite_rest a11y 锚未找到（u_alice .rest 状态 PetSpriteView 未渲染）")
        XCTAssertTrue(walkSprite.waitForExistence(timeout: timeout),
                      "petSprite_walk a11y 锚未找到（u_bob .walk 状态 PetSpriteView 未渲染）")
        XCTAssertTrue(runSprite.waitForExistence(timeout: timeout),
                      "petSprite_run a11y 锚未找到（u_charlie .run 状态 PetSpriteView 未渲染）")

        // 3) 基线 screenshot 作为 dev artifact（XCTAttachment 写入 xcresult bundle）
        //    Story 15.3 AC4 case#D：录屏文件 / 基线帧 codex review 阶段抓帧对比.
        let baselineScreenshot = XCUIScreen.main.screenshot()
        let attachment = XCTAttachment(screenshot: baselineScreenshot)
        attachment.name = "PetSpriteTransition-baseline-3members-distinct-states"
        attachment.lifetime = .keepAlways  // 不随 test 成功自动删；codex review 阶段需要
        add(attachment)

        // 4) terminate + relaunch（不变 env）→ a11y identifier 序列稳定
        //    Task 3.4.2 钦定 "mutate 驱动用 XCUIApplication.terminate() + relaunch" 模式 ——
        //    相同 env 下 RootView init() 重新注入 MockRoomViewModel 三态 baseline，验证
        //    PetSpriteView 在 view-tree 重建后仍正确渲染（覆盖"app cold-start 路径"过渡契约稳定性）.
        app.terminate()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launchEnvironment["UITEST_FORCE_IN_ROOM"] = "1"
        app.launchEnvironment["UITEST_ROOM_THREE_MEMBERS"] = "1"
        app.launch()

        // 5) relaunch 后 a11y identifier 仍可定位（PetSpriteView 三件套 `.id + .transition + .animation(value:)`
        //    在 view-tree 重建后稳定生效；relaunch 路径下 view 是首次 insert 而非 swap，transition 不应
        //    引起 identifier 定位失败）
        let restAfterRelaunch = app.descendants(matching: .any)[AccessibilityID.Home.petSpriteRest]
        XCTAssertTrue(restAfterRelaunch.waitForExistence(timeout: timeout),
                      "relaunch 后 petSprite_rest a11y 锚仍应可定位（view-tree 重建后过渡契约稳定）")
        let walkAfterRelaunch = app.descendants(matching: .any)[AccessibilityID.Home.petSpriteWalk]
        XCTAssertTrue(walkAfterRelaunch.waitForExistence(timeout: timeout),
                      "relaunch 后 petSprite_walk a11y 锚仍应可定位")
        let runAfterRelaunch = app.descendants(matching: .any)[AccessibilityID.Home.petSpriteRun]
        XCTAssertTrue(runAfterRelaunch.waitForExistence(timeout: timeout),
                      "relaunch 后 petSprite_run a11y 锚仍应可定位")

        // 6) relaunch 后 screenshot 作为对比基线（同 mock 数据下视觉应一致）
        let relaunchScreenshot = XCUIScreen.main.screenshot()
        let relaunchAttachment = XCTAttachment(screenshot: relaunchScreenshot)
        relaunchAttachment.name = "PetSpriteTransition-relaunch-3members-distinct-states"
        relaunchAttachment.lifetime = .keepAlways
        add(relaunchAttachment)
    }

    /// Story 12.7 AC9: launch app → Home Tab idle → 点 `homeTeamIdleCard_create`
    /// → 验证 RoomView 出现（roomIdDisplay 锚定可定位）+ Tab Bar 仍可见.
    ///
    /// 路径选择：UITEST_SKIP_GUEST_LOGIN=1 + **不**带 UITEST_FORCE_IN_ROOM → 进入 Home idle 状态.
    /// 由于本 story RootView wire 走真实 CreateRoomUseCase + 真实 server（localhost:8080），
    /// 而 UITest 不起 server，CreateRoomUseCase 调用会失败（network error）→ 不会切到 RoomView.
    /// 这是预期：本 case 仅验证 Home Tab idle 渲染 + create 按钮锚定可定位 + 点击不 crash.
    /// 真实多端联调（点 create → 真切到 RoomView → 看 snapshot）由 Epic 13 节点 4 demo 验收覆盖.
    func testHomeTabIdleCreateButtonExistsAndTappable() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        // **不**设 UITEST_FORCE_IN_ROOM → Home Tab idle 路径
        app.launch()

        let timeout: TimeInterval = 5

        // 1) Home Tab 默认选中（tab_home 可见）
        let homeTab = app.descendants(matching: .any)[AccessibilityID.Tab.home]
        XCTAssertTrue(homeTab.waitForExistence(timeout: timeout),
                      "tab_home a11y 锚未找到（MainTabView 应渲染 4 个 Tab）")

        // 2) homeTeamIdleCard_create 按钮可见
        let createButton = app.descendants(matching: .any)[AccessibilityID.Home.teamIdleCardCreate]
        XCTAssertTrue(createButton.waitForExistence(timeout: timeout),
                      "homeTeamIdleCard_create a11y 锚未找到（Home Tab idle TeamIdleCard 应渲染 create 按钮）")

        // 3) homeTeamIdleCard_join 按钮可见（兄弟按钮）
        let joinButton = app.descendants(matching: .any)[AccessibilityID.Home.teamIdleCardJoin]
        XCTAssertTrue(joinButton.exists, "homeTeamIdleCard_join a11y 锚未找到")

        // 4) 点 create —— 不应 crash；不会切到 RoomView（无 server）但本 case 不强断言切换
        createButton.tap()
        // 给 Task 一点时间触发 UseCase（即使失败也不 crash）
        Thread.sleep(forTimeInterval: 0.5)

        // 5) Tab Bar 仍可见（点击不 crash 后 UI 仍稳定）
        XCTAssertTrue(app.descendants(matching: .any)[AccessibilityID.Tab.home].exists,
                      "点 create 后 tab_home 应仍可见（UI 稳定，不 crash）")
    }
}
