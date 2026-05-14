// RoomEmojiSendUITests.swift
// Story 18.3 AC10: 选中表情 → 本地立即动效 UITest (XCUITest, MockRoomViewModel + mock emoji fixture).
//
// 路径选择 (复用 18.2 的 `--uitest-emoji-panel-room-host`):
//   - launchArguments = ["--uitest-emoji-panel-room-host"] → RootView.init 路径下 @StateObject
//     roomViewModel 切到 MockRoomViewModel(currentUserId: "u1", members: u1/u2/u3, isHost: true).
//   - launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1" → AppLaunchStateMachine 立即切 .ready, 跳过 guest login.
//   - launchEnvironment["UITEST_FORCE_IN_ROOM"] = "1" → appState.setCurrentRoomId("1234567") →
//     HomeContainerView 切到 inRoom 态 → 渲染 RoomScaffoldView.
//   - launchEnvironment["UITEST_MOCK_EMOJI"] = "1" → AppContainer 注入 UITestMockEmojiRepository →
//     LoadEmojisUseCase 返 4 项 fixture (wave / love / laugh / cry).
//
// MockRoomViewModel.onEmojiSelected (18.3 落地) 入队 activeEmojis → RoomScaffoldView 的
// EmojiAnimationLayerPlaceholder ForEach 渲染 Text(emojiCode) + accessibilityIdentifier "activeEmoji_<uuid>".
// 不需要真 WS / SendEmojiUseCase 链路 (那是 RealRoomViewModel 路径; 单测已覆盖, UITest 用 Mock 路径稳定性更高).
//
// 测试 case:
//   A. 选中 wave → 屏幕上立即可见 wave 文本 + activeEmoji_<uuid> 节点 (0 延迟语义)
//   B. 连点 3 次 → 屏幕上至少 3 个 activeEmoji 节点叠加 (UUID 区分让 ForEach 各自渲染)
//   C. 选不同 emoji (wave + love) → 屏幕上同时可见 wave + love 两个文本
//
// 弱网降级 (toast) 路径: **不**在 UITest 覆盖 —— toast 自动 2s 消失 + UITest setUp/wait 时间窗不稳;
// 单元测试 `test_realOnEmojiSelected_wsFailureKeepsLocalAnimationAndShowsToast` 已覆盖该路径.

import XCTest

final class RoomEmojiSendUITests: XCTestCase {

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

    /// Story 18.3 AC10 case A: 本地动效 0 延迟可见.
    /// 进房间 → 点 self → 选 wave → 等 0.5s 内 wave 文本 + activeEmoji_<uuid> 节点出现.
    func test_selectEmoji_immediatelyShowsLocalAnimation() throws {
        let timeout: TimeInterval = 5

        // 进入 RoomScaffoldView → 点自己 PetSprite Button → emojiPanel sheet 弹出
        let selfButton = app.buttons[AccessibilityID.Room.ownPetSpriteButton(at: 0)]
        XCTAssertTrue(selfButton.waitForExistence(timeout: timeout),
                      "roomMember_0_petSprite Button 必须可定位 (self 行包 Button)")
        selfButton.tap()

        let waveCell = app.descendants(matching: .any)[AccessibilityID.Emoji.cell("wave")]
        XCTAssertTrue(waveCell.waitForExistence(timeout: timeout),
                      "emojiCell_wave 必须在 panel 内可见 (mock fixture 4 项)")
        waveCell.tap()

        // 验证 0.5s 内屏幕上至少 1 个 activeEmoji 节点 (占位渲染 Text("wave") + a11y id)
        let activeEmojiPredicate = NSPredicate(format: "identifier BEGINSWITH 'activeEmoji_'")
        let activeEmoji = app.staticTexts.matching(activeEmojiPredicate).firstMatch
        XCTAssertTrue(activeEmoji.waitForExistence(timeout: 0.5),
                      "选中 wave 后 0.5s 内必须看到至少 1 个 activeEmoji_<uuid> 节点 (本地动效 0 延迟语义)")
        XCTAssertEqual(activeEmoji.label, "wave",
                       "activeEmoji 文本必须等于 emojiCode 'wave'")
    }

    /// Story 18.3 AC10 case B: 连点 3 次 → 屏幕上至少 3 个 activeEmoji 节点叠加.
    /// epics.md 行 2697 钦定 "同一表情快速连点 3 次 → activeEmojis 添加 3 项";
    /// 占位渲染下表现为 3 个 "wave" Text 节点 (UUID 区分让 ForEach 各自渲染).
    /// Story 18.4 落地后这 3 个 Text 会变成 3 个独立飞出动画.
    func test_rapidThreeTaps_showsThreeActiveEmojis() throws {
        let timeout: TimeInterval = 5

        for _ in 0..<3 {
            let selfButton = app.buttons[AccessibilityID.Room.ownPetSpriteButton(at: 0)]
            XCTAssertTrue(selfButton.waitForExistence(timeout: timeout))
            selfButton.tap()

            let waveCell = app.descendants(matching: .any)[AccessibilityID.Emoji.cell("wave")]
            XCTAssertTrue(waveCell.waitForExistence(timeout: timeout))
            waveCell.tap()

            // 等 sheet 关闭再开下一轮 (sheet 没关下个 selfButton.tap 不会落到 self 按钮上)
            let panel = app.descendants(matching: .any)[AccessibilityID.Emoji.panel]
            XCTAssertTrue(panel.waitForNonExistence(timeout: timeout),
                          "选 emoji cell 后 sheet 必须关闭再开下一轮")
        }

        let activeEmojiPredicate = NSPredicate(format: "identifier BEGINSWITH 'activeEmoji_'")
        let count = app.staticTexts.matching(activeEmojiPredicate).count
        XCTAssertGreaterThanOrEqual(count, 3,
                                    "连点 3 次后必须看到至少 3 个 activeEmoji 节点; 实际 \(count)")
    }

    /// Story 18.3 AC10 case C: 选不同 emoji → 屏幕上同时可见 wave + love 两个文本.
    /// 验证 activeEmojis 队列支持多种 emojiCode 并存.
    func test_selectDifferentEmojis_showsBoth() throws {
        let timeout: TimeInterval = 5

        // 第一次: wave
        let selfButton1 = app.buttons[AccessibilityID.Room.ownPetSpriteButton(at: 0)]
        XCTAssertTrue(selfButton1.waitForExistence(timeout: timeout))
        selfButton1.tap()
        let waveCell = app.descendants(matching: .any)[AccessibilityID.Emoji.cell("wave")]
        XCTAssertTrue(waveCell.waitForExistence(timeout: timeout))
        waveCell.tap()
        let panelAfterWave = app.descendants(matching: .any)[AccessibilityID.Emoji.panel]
        XCTAssertTrue(panelAfterWave.waitForNonExistence(timeout: timeout))

        // 第二次: love (重弹 sheet)
        let selfButton2 = app.buttons[AccessibilityID.Room.ownPetSpriteButton(at: 0)]
        XCTAssertTrue(selfButton2.waitForExistence(timeout: timeout))
        selfButton2.tap()
        let loveCell = app.descendants(matching: .any)[AccessibilityID.Emoji.cell("love")]
        XCTAssertTrue(loveCell.waitForExistence(timeout: timeout))
        loveCell.tap()
        let panelAfterLove = app.descendants(matching: .any)[AccessibilityID.Emoji.panel]
        XCTAssertTrue(panelAfterLove.waitForNonExistence(timeout: timeout))

        // 屏幕上同时可见 wave + love 两个文本
        XCTAssertTrue(app.staticTexts["wave"].waitForExistence(timeout: 0.5),
                      "屏幕上必须可见 wave activeEmoji")
        XCTAssertTrue(app.staticTexts["love"].waitForExistence(timeout: 0.5),
                      "屏幕上必须可见 love activeEmoji")
    }
}
