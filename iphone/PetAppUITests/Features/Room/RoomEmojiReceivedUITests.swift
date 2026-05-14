// RoomEmojiReceivedUITests.swift
// Story 18.4 AC10: 接收 emoji.received 广播 → 看到飞出动效 UITest (XCUITest, MockRoomViewModel 路径 + launch env trigger emit).
//
// 路径选择 (复用 18.2/18.3 的 `--uitest-emoji-panel-room-host`):
//   - launchArguments = ["--uitest-emoji-panel-room-host"] → MockRoomViewModel(currentUserId: "u1", members: u1/u2/u3).
//   - launchEnvironment:
//     - "UITEST_SKIP_GUEST_LOGIN": "1" → AppLaunchStateMachine 立即切 .ready.
//     - "UITEST_FORCE_IN_ROOM": "1" → appState.setCurrentRoomId → HomeContainer 切 inRoom.
//     - "UITEST_MOCK_EMOJI": "1" → AppContainer 注入 UITestMockEmojiRepository (4 项 fixture).
//     - "UITEST_EMIT_EMOJI_RECEIVED": "1" → launch 后 1s mock vm 主动调 applyEmojiReceived emit 2 条
//                                            fixtures (u2:wave + u3:love), 验证接收端动效 + 多 emoji 独立.
//
// 测试 case (≥3):
//   - A. 别人发表情 → 0.5s 内 activeEmoji_<uuid> 节点可见 + label == emojiCode
//   - B. 别人发不同 emoji (u2:wave + u3:love) → 多个独立 activeEmoji 节点同时可见
//   - C. 1.5s 后 activeEmoji 自动 expire 移除 (epics.md 行 2715 钦定动画时长)
//
// **注**: self-broadcast 去重路径 (V1 §12.3 行 2471 (a)) 走 RealRoomViewModel + 单元测试覆盖
// (RoomViewModelEmojiReceivedTests.test_realApplyEmojiReceived_selfBroadcastIsSkipped);
// UITest 用 Mock path 验证视觉 + 多 emoji 独立, 不验证去重 (MockRoomViewModel 无去重逻辑, 与 Real 不同).

import XCTest

final class RoomEmojiReceivedUITests: XCTestCase {

    var app: XCUIApplication!

    override func setUpWithError() throws {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["--uitest-emoji-panel-room-host"]
        app.launchEnvironment = [
            "UITEST_SKIP_GUEST_LOGIN": "1",
            "UITEST_FORCE_IN_ROOM": "1",
            "UITEST_MOCK_EMOJI": "1",
            "UITEST_EMIT_EMOJI_RECEIVED": "1",
        ]
        app.launch()
    }

    /// Story 18.4 AC10 case A: 别人发表情 → 看到 activeEmoji 节点 + label 对齐 emojiCode.
    /// Mock vm 在 launch 后 1s 调 applyEmojiReceived(u2:wave) → 0.5s 内可见.
    func test_receiveOtherUserEmoji_showsActiveEmojiWithCorrectLabel() throws {
        let activeEmojiPredicate = NSPredicate(format: "identifier BEGINSWITH 'activeEmoji_'")
        let firstActiveEmoji = app.staticTexts.matching(activeEmojiPredicate).firstMatch

        // Wait up to 3s for emit (launch 1s + emit timing 200ms buffer).
        XCTAssertTrue(firstActiveEmoji.waitForExistence(timeout: 3),
                      "别人发 emoji.received 后必须看到 activeEmoji_<uuid> 节点 (label = emojiCode)")
        // label 必是 "wave" 或 "love" (两条 fixture 之一; 先到的是 wave).
        XCTAssertTrue(["wave", "love"].contains(firstActiveEmoji.label),
                      "activeEmoji label 必为 wave 或 love (fixture 钦定); 实际: \(firstActiveEmoji.label)")
    }

    /// Story 18.4 AC10 case B: 多人同时发表情 → 多个独立 activeEmoji 节点.
    /// Mock vm 在 launch 后 1s + 1.3s 各 emit 1 条 (u2:wave / u3:love); 等 1.5s 内两条都到 + 还没 expire.
    /// epics.md 行 2717 钦定 "同时多个表情飞出独立动效不干扰".
    func test_receiveMultipleEmojis_showsMultipleIndependentActiveEmojis() throws {
        let activeEmojiPredicate = NSPredicate(format: "identifier BEGINSWITH 'activeEmoji_'")

        // 等 wave 出现 (≈1s post-launch).
        let waveText = app.staticTexts["wave"]
        XCTAssertTrue(waveText.waitForExistence(timeout: 3),
                      "u2:wave fixture 必须出现")

        // 等 love 出现 (≈1.3s post-launch).
        let loveText = app.staticTexts["love"]
        XCTAssertTrue(loveText.waitForExistence(timeout: 3),
                      "u3:love fixture 必须出现")

        // 两个独立 activeEmoji 节点同时可见 (1.5s 动画期间不互相覆盖; UUID 让 ForEach 区分).
        let count = app.staticTexts.matching(activeEmojiPredicate).count
        XCTAssertGreaterThanOrEqual(count, 2,
                                    "多 emoji 独立动画期间至少 2 个 activeEmoji_<uuid> 节点可见; 实际 \(count)")
    }

    /// Story 18.4 AC10 case C: 1.5s 后 activeEmoji 自动 expire (epics.md 行 2715 钦定).
    /// MockRoomViewModel **不**做 1.5s 自动移除 (story file AC5 钦定); 故本 case 仅验证 emit 出现后
    /// 占位渲染机制工作 + activeEmoji 不无限堆叠 (Real path 1.5s expire 由单测覆盖
    /// test_realApplyEmojiReceived_autoExpiresAfter15Seconds).
    /// 简化为: 等 fixture 全部 emit 完后, activeEmoji 数量稳定 == 2 (不爆到 3+).
    func test_receivedEmojisCountIsStable() throws {
        let activeEmojiPredicate = NSPredicate(format: "identifier BEGINSWITH 'activeEmoji_'")

        // 等所有 fixture 都 emit (1.3s + 缓冲).
        let loveText = app.staticTexts["love"]
        XCTAssertTrue(loveText.waitForExistence(timeout: 3))

        // 再等 500ms 让 SwiftUI render 稳定.
        Thread.sleep(forTimeInterval: 0.5)
        let count = app.staticTexts.matching(activeEmojiPredicate).count
        XCTAssertEqual(count, 2,
                       "Mock vm 仅 emit 2 条 fixture; activeEmoji count 必稳定 == 2 (不无限堆叠)")
    }
}
