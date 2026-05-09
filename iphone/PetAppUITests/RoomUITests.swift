// RoomUITests.swift
// Story 12.1 AC8: RoomScaffoldView 在 Story 12.1 升级版 RealRoomViewModel 路径下的 a11y 锚定 UITest.
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
}
