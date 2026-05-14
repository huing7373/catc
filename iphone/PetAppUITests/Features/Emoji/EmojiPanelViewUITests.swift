// EmojiPanelViewUITests.swift
// Story 18.1 AC8: EmojiPanelView UI 测试 (XCUITest, mock API mode).
//
// 路径选择 (Story 18.1 钦定):
//   - launchArguments = ["--uitest-emoji-panel-host"] → RootView 检测 arg → 直接渲染 EmojiPanelHostView
//     (DEBUG-only stub host view; 绕过 MainTabView + bootstrap).
//   - launchEnvironment["UITEST_MOCK_EMOJI"] = "1" → AppContainer 检测 → 注入 UITestMockEmojiRepository.
//   - launchEnvironment["UITEST_MOCK_EMOJI_JSON"] (可选) → 注入自定义 fixture JSON; 缺省走 4 项内置 fixture
//     (wave / love / laugh / cry).
//
// 测试 case:
//   1. 可见性：4 个 emojiCell_<code> a11y 节点可定位
//   2. onSelect 回调：点 emojiCell_wave → emojiPanel_uitestSelectedCode label == "wave"

import XCTest

final class EmojiPanelViewUITests: XCTestCase {

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    /// Story 18.1 AC8 case#1: EmojiPanelHostView 渲染 → 4 个 emojiCell 可见.
    /// mock mode 注入 4 项内置 fixture (wave / love / laugh / cry).
    func testEmojiPanelRendersFourCellsInMockMode() throws {
        let app = XCUIApplication()
        app.launchArguments = ["--uitest-emoji-panel-host"]
        app.launchEnvironment["UITEST_MOCK_EMOJI"] = "1"
        app.launch()

        let timeout: TimeInterval = 5

        // 等 EmojiPanelView loaded 态出现 (.task → useCase.execute() → state = .loaded)
        let panel = app.descendants(matching: .any)[AccessibilityID.Emoji.panel]
        XCTAssertTrue(panel.waitForExistence(timeout: timeout),
                      "emojiPanel a11y 锚未找到 (EmojiPanelView 应在 mock fixture 加载后渲染 LazyVGrid)")

        // 验证 4 个内置 fixture cell 都可见
        let waveCell = app.descendants(matching: .any)[AccessibilityID.Emoji.cell("wave")]
        XCTAssertTrue(waveCell.waitForExistence(timeout: timeout), "emojiCell_wave 未找到")

        let loveCell = app.descendants(matching: .any)[AccessibilityID.Emoji.cell("love")]
        XCTAssertTrue(loveCell.exists, "emojiCell_love 未找到")

        let laughCell = app.descendants(matching: .any)[AccessibilityID.Emoji.cell("laugh")]
        XCTAssertTrue(laughCell.exists, "emojiCell_laugh 未找到")

        let cryCell = app.descendants(matching: .any)[AccessibilityID.Emoji.cell("cry")]
        XCTAssertTrue(cryCell.exists, "emojiCell_cry 未找到")
    }

    /// Story 18.1 AC8 case#2: 点 emojiCell_wave → onSelect 回调更新 lastSelectedCode → 隐藏 Text label == "wave".
    func testEmojiPanelOnSelectCallbackPropagatesCode() throws {
        let app = XCUIApplication()
        app.launchArguments = ["--uitest-emoji-panel-host"]
        app.launchEnvironment["UITEST_MOCK_EMOJI"] = "1"
        app.launch()

        let timeout: TimeInterval = 5

        // 用 `.firstMatch` 锁定单 emojiCell 节点 (cell 内 `.accessibilityElement(children: .combine)`
        // 已让 cell 折叠成单一 a11y 节点; .firstMatch 防多匹配 + tap 时的歧义).
        let waveCell = app.descendants(matching: .any).matching(identifier: AccessibilityID.Emoji.cell("wave")).firstMatch
        XCTAssertTrue(waveCell.waitForExistence(timeout: timeout), "emojiCell_wave 未找到")
        waveCell.tap()

        // 隐藏 Text 节点的 label 应被更新为 "wave"
        let selectedCodeText = app.descendants(matching: .any)[AccessibilityID.Emoji.uitestSelectedCode]
        XCTAssertTrue(selectedCodeText.waitForExistence(timeout: timeout),
                      "emojiPanel_uitestSelectedCode a11y 锚未找到 (EmojiPanelHostView 应挂隐藏 Text 字段)")
        // 等 lastSelectedCode 写入 (tap → @State 更新 → Text label 重渲染 → a11y tree 刷新).
        // 用 NSPredicate 等"label 含 wave" 满足，避免读时机问题.
        let labelPredicate = NSPredicate(format: "label == %@ OR value == %@", "wave", "wave")
        let labelExp = expectation(for: labelPredicate, evaluatedWith: selectedCodeText)
        let result = XCTWaiter.wait(for: [labelExp], timeout: timeout)
        XCTAssertEqual(result, .completed,
                       "onSelect 回调应把 emojiCode 'wave' 写入 lastSelectedCode (实际 label: '\(selectedCodeText.label)', value: '\((selectedCodeText.value as? String) ?? "<nil>")')")
    }
}
