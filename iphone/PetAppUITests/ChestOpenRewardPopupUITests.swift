// ChestOpenRewardPopupUITests.swift
// Story 21.4 AC7: 集成测试（XCUITest）—— 验证开宝箱 → RewardPopupView 弹出 → 确定关闭 → counting 态恢复.
//
// 测试场景（spec AC 行 3120-3121 + 本 story AC7 钦定）:
//   1. launch unlockable 态 → 点开宝箱按钮 → 等 rewardPopup 出现
//   2. 验 4 子 a11y identifier（icon / nameLabel / rarityTag / confirmButton）都存在
//   3. 验 nameLabel 文本含 "获得"（不固定具体装扮名防 mock data 更新破测）
//   4. tap confirmButton → 等 rewardPopup 消失
//   5. 验 chestCard_counting 仍存在（开箱后 nextChest counting 不被弹窗关闭事件影响）
//
// 实装策略（与 ChestOpenUITests / ChestRefreshUITests 同模式）:
//   - 复用 `UITEST_SKIP_GUEST_LOGIN=1` + `UITEST_MOCK_CHEST_OPEN=1`（21.3 已落地 UITestMockChestRepository
//     注入路径；本 story 直接复用，不改 mock）.
//   - UITestMockChestRepository.openChest 返回固定 reward（rarity=1 common / cosmeticItemId="uitest-cos-1"
//     / name="测试装扮" / iconUrl="https://placehold.co/32x32"）—— 弹窗会显示 "获得 测试装扮" 文案 + N 色徽章.
//
// 不验范围（详 spec Dev Notes "测试边界"）:
//   - **不**测 RarityTag 颜色（SwiftUI 渲染颜色不在 XCTest 能力范围内；mapper 单测覆盖 enum 转换）
//   - **不**测 swipe-dismiss 路径（节点 7 阶段确定按钮路径已足够；SwiftUI 框架行为）
//   - **不**测 FadeIn 动画时长（Primitive 已在 Story 37.6 落地 + 单独测试 FadeIn primitive）

import XCTest

final class ChestOpenRewardPopupUITests: XCTestCase {

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    /// Story 21.4 AC7: launch unlockable → tap 开宝箱 → wait rewardPopup → 验 4 子 identifier →
    ///                 tap confirmButton → wait rewardPopup 消失 → 验 chestCard_counting 仍在.
    func testTappingChestOpenShowsRewardPopupAndConfirmDismissesIt() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launchEnvironment["UITEST_MOCK_CHEST_OPEN"] = "1"
        app.launch()

        let timeout: TimeInterval = 5

        // Step 1: 等 unlockable 态 ChestCardView 渲染 + 开宝箱按钮可见.
        let chestOpenButton = app.buttons[AccessibilityID.Home.chestOpenButton]
        XCTAssertTrue(
            chestOpenButton.waitForExistence(timeout: timeout),
            "home_chestOpenButton 未渲染 —— ChestCardView unlockableView 应可定位"
        )
        XCTAssertTrue(chestOpenButton.isEnabled, "点击前按钮应 enabled（isOpening = false）")

        // Step 2: tap 触发开箱 → 等 RewardPopupView 出现.
        chestOpenButton.tap()

        let rewardPopup = app.descendants(matching: .any)[AccessibilityID.RewardPopup.popup]
        XCTAssertTrue(
            rewardPopup.waitForExistence(timeout: timeout),
            "rewardPopup 未出现 —— 开箱成功后 pendingReward 应触发 .sheet(item:) 弹窗"
        )

        // Step 3: 验 4 子 a11y identifier 都存在.
        let popupIcon = app.descendants(matching: .any)[AccessibilityID.RewardPopup.icon]
        XCTAssertTrue(
            popupIcon.waitForExistence(timeout: timeout),
            "rewardPopup_icon 应存在（AsyncImage 96 × 96 区域 a11y 锚）"
        )

        let popupNameLabel = app.descendants(matching: .any)[AccessibilityID.RewardPopup.nameLabel]
        XCTAssertTrue(
            popupNameLabel.waitForExistence(timeout: timeout),
            "rewardPopup_nameLabel 应存在（Text \"获得 {name}\" a11y 锚）"
        )
        // 验 nameLabel 文本含 "获得"（不固定具体 mock name 防 mock data 更新破测）.
        XCTAssertTrue(
            popupNameLabel.label.contains("获得"),
            "rewardPopup_nameLabel 文本应含 \"获得\" 前缀；实际 = '\(popupNameLabel.label)'"
        )

        let popupRarityTag = app.descendants(matching: .any)[AccessibilityID.RewardPopup.rarityTag]
        XCTAssertTrue(
            popupRarityTag.waitForExistence(timeout: timeout),
            "rewardPopup_rarityTag 应存在（RarityTag 视觉锚；mock data rarity=1 → N 灰色徽章）"
        )

        let confirmButton = app.buttons[AccessibilityID.RewardPopup.confirmButton]
        XCTAssertTrue(
            confirmButton.waitForExistence(timeout: timeout),
            "rewardPopup_confirmButton 应存在（\"确定\" PrimaryButton）"
        )
        XCTAssertTrue(confirmButton.isEnabled, "confirmButton 应 enabled（无 disabled 状态）")

        // Step 4: tap 确定 → 等 rewardPopup 消失.
        confirmButton.tap()

        // 用 waitForNonExistence 等 sheet 消失（含 SwiftUI 动画时间）.
        XCTAssertTrue(
            waitForNonExistence(of: rewardPopup, timeout: timeout),
            "rewardPopup 应在点击确定后消失（onClose closure 内 set pendingReward = nil 让 .sheet(item:) 自动 dismiss）"
        )

        // Step 5: 验 chestCard_counting 仍存在（开箱后 nextChest counting 不被弹窗关闭事件影响）.
        let chestCardCounting = app.descendants(matching: .any)["chestCard_counting"]
        XCTAssertTrue(
            chestCardCounting.waitForExistence(timeout: timeout),
            "chestCard_counting 应存在（开箱后 nextChest counting 是 AppState.currentChest 已写入，弹窗关闭不应影响）"
        )
    }

    // MARK: - Helpers

    /// 等待 element 不存在（XCTest 没有内建 waitForNonExistence；自行 poll）.
    /// 与既有 UITest 同模式（ChestOpenUITests 用 .exists 直接断言，但本 case 含 sheet dismiss 动画，需 poll）.
    private func waitForNonExistence(of element: XCUIElement, timeout: TimeInterval) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if !element.exists {
                return true
            }
            Thread.sleep(forTimeInterval: 0.1)
        }
        return !element.exists
    }
}
