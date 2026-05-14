// RoomEmojiPanelTriggerUITests.swift
// Story 18.2 AC7: 点击自己猫触发表情面板 UITest (XCUITest, MockRoomViewModel + mock emoji fixture).
//
// 路径选择 (Story 18.2 钦定):
//   - launchArguments = ["--uitest-emoji-panel-room-host"] → RootView.init 路径下 @StateObject
//     roomViewModel 切到 MockRoomViewModel(currentUserId: "u1", members: u1/u2/u3, isHost: true).
//   - launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1" → AppLaunchStateMachine 立即切 .ready, 跳过 guest login.
//   - launchEnvironment["UITEST_FORCE_IN_ROOM"] = "1" → appState.setCurrentRoomId("1234567") →
//     HomeContainerView 切到 inRoom 态 → 渲染 RoomScaffoldView.
//   - launchEnvironment["UITEST_MOCK_EMOJI"] = "1" → AppContainer 注入 UITestMockEmojiRepository →
//     LoadEmojisUseCase 返 4 项 fixture (wave / love / laugh / cry).
//
// 测试 case:
//   A. self button 存在 + other 不存在 (roomMember_0_petSprite 渲染 Button; roomMember_1_petSprite 不渲染).
//   B. 点 self petSprite Button → emojiPanel sheet 弹出 + emojiCell_wave 可点.
//   C. 点 emoji cell (wave) → sheet 关闭 (emojiPanel 不可见).
//   D. (skipped — 在 Mock vm 路径下"他人位"也只是 PetSpriteView 无 Button, 由 case A 已断言 "no button" 即覆盖防误触语义).

import XCTest

final class RoomEmojiPanelTriggerUITests: XCTestCase {

    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["--uitest-emoji-panel-room-host"]
        app.launchEnvironment = [
            "UITEST_SKIP_GUEST_LOGIN": "1",
            "UITEST_FORCE_IN_ROOM": "1",
            "UITEST_MOCK_EMOJI": "1",
        ]
        app.launch()
    }

    /// Story 18.2 AC7 case A: 自己 PetSpriteView Button 可见; 他人位 Button 不存在.
    /// 物理上 he/her 行只渲染 PetSpriteView (无 Button identifier) — 验证 self vs other 视觉契约.
    func test_selfPetSpriteButton_exists_otherDoesNot() {
        let timeout: TimeInterval = 5

        // 等 RoomScaffoldView 渲染完成 — 先验证 roomMember_0 父行存在
        let selfRow = app.descendants(matching: .any)[AccessibilityID.Room.member(at: 0)]
        XCTAssertTrue(selfRow.waitForExistence(timeout: timeout),
                      "roomMember_0 (self 行) 必须可定位")

        // 自己位 PetSpriteView Button 必须可定位 (因 self → Button 包 PetSpriteView)
        let selfButton = app.buttons[AccessibilityID.Room.ownPetSpriteButton(at: 0)]
        XCTAssertTrue(selfButton.waitForExistence(timeout: timeout),
                      "roomMember_0_petSprite Button 必须可定位 (self 行包 Button)")

        // 他人位 PetSpriteView 不应有同模式 Button identifier
        let otherButton = app.buttons[AccessibilityID.Room.ownPetSpriteButton(at: 1)]
        XCTAssertFalse(otherButton.exists,
                       "roomMember_1_petSprite Button **不**应存在 (他人位仅渲染 PetSpriteView 无 Button)")
    }

    /// Story 18.2 AC7 case B: 点击自己 PetSpriteView Button → EmojiPanel sheet 弹出.
    /// 验证 onOwnPetTap → showEmojiPanel = true → SwiftUI sheet 双向绑定弹出.
    func test_tapSelfPetSprite_opensEmojiPanel() {
        let timeout: TimeInterval = 5

        let selfButton = app.buttons[AccessibilityID.Room.ownPetSpriteButton(at: 0)]
        XCTAssertTrue(selfButton.waitForExistence(timeout: timeout),
                      "roomMember_0_petSprite Button 必须可定位")
        selfButton.tap()

        // sheet 内 EmojiPanelView loaded 态 → emojiPanel a11y 锚出现 + 4 cell 可见
        let panel = app.descendants(matching: .any)[AccessibilityID.Emoji.panel]
        XCTAssertTrue(panel.waitForExistence(timeout: timeout),
                      "点 self petSprite Button 后 emojiPanel 必须弹出 (EmojiPanelView loaded 态)")

        let waveCell = app.descendants(matching: .any)[AccessibilityID.Emoji.cell("wave")]
        XCTAssertTrue(waveCell.waitForExistence(timeout: timeout),
                      "emojiCell_wave 必须在 panel 内可见 (mock fixture 4 项)")
    }

    /// Story 18.2 AC7 case C: 选中 emoji cell → onSelect 闭包置 showEmojiPanel = false → sheet 关闭.
    func test_selectEmojiCell_closesPanel() {
        let timeout: TimeInterval = 5

        let selfButton = app.buttons[AccessibilityID.Room.ownPetSpriteButton(at: 0)]
        XCTAssertTrue(selfButton.waitForExistence(timeout: timeout))
        selfButton.tap()

        let panel = app.descendants(matching: .any)[AccessibilityID.Emoji.panel]
        XCTAssertTrue(panel.waitForExistence(timeout: timeout),
                      "panel 必须先弹出")

        // 点 wave cell 触发 onSelect 闭包 → vm.showEmojiPanel = false → sheet 关闭
        let waveCell = app.descendants(matching: .any).matching(identifier: AccessibilityID.Emoji.cell("wave")).firstMatch
        XCTAssertTrue(waveCell.waitForExistence(timeout: timeout))
        waveCell.tap()

        XCTAssertTrue(panel.waitForNonExistence(timeout: timeout),
                      "选 emoji cell 后 emojiPanel 必须从 view tree 消失 (sheet 已关闭)")
    }
}
