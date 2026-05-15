// ChestOpenUITests.swift
// Story 21.3 AC10: 集成测试（XCUITest）—— 验证开宝箱按钮 + POST /chest/open 链路端到端通畅.
//
// 测试场景（spec AC 行 3120-3121 + 本 story 限定 21.4 之前可独立 verify 的范围）:
//   1. launch unlockable 态 → 点开宝箱按钮 → 等 chestCard_counting 出现（mock 响应 nextChest 是 counting）
//
// 实装策略（与 ChestRefreshUITests / Story 8.5 集成测试同模式）:
//   - 复用 `UITEST_SKIP_GUEST_LOGIN=1` + 新增 `UITEST_MOCK_CHEST_OPEN=1` 让 AppContainer 注入
//     UITestMockChestRepository（fetchCurrent → unlockable / openChest → happy + counting nextChest）.
//   - chestRefreshTriggerService.start() 在 onReadyTask 调一次 fetchCurrent → mock unlockable → AppState.currentChest
//     = unlockable → ChestCardView 渲染 chestCard_unlockable + chestOpenButton.
//   - 点 chestOpenButton → openChestUseCase.execute → mock openChest → AppState.currentChest =
//     nextChest counting → ChestCardView 重新渲染 chestCard_counting.
//
// 不验 RewardPopupView 弹出（21.4 范围）—— 本 story 仅写 pendingReward 到 transient ViewModel 字段;
// 集成测试在本 story 阶段只验 "按钮 → POST → AppState 更新 → counting 态回归" 链路.

import XCTest

final class ChestOpenUITests: XCTestCase {

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    /// Story 21.3 AC10 #1: launch unlockable 态 → tap 开宝箱按钮 → counting 态回归.
    /// 验证完整链路：UITestMockChestRepository → ChestRefreshTriggerService.start() → unlockable AppState
    /// → ChestCardView 渲染 chestCard_unlockable → tap chestOpenButton → OpenChestUseCase.execute →
    /// mock openChest 成功 → AppState 更新 nextChest counting → ChestCardView 重新渲染 chestCard_counting.
    func testTappingChestOpenButtonTransitionsCardToCountingState() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launchEnvironment["UITEST_MOCK_CHEST_OPEN"] = "1"
        app.launch()

        let timeout: TimeInterval = 5

        // Step 1: 等 unlockable 态 ChestCardView 渲染（mock fetchCurrent 返 status=2 → AppState.currentChest
        // 被 ChestRefreshTriggerService.start() 写入 → ChestCardView 渲染 chestCard_unlockable）.
        let chestCardUnlockable = app.descendants(matching: .any)["chestCard_unlockable"]
        XCTAssertTrue(
            chestCardUnlockable.waitForExistence(timeout: timeout),
            "chestCard_unlockable 未渲染 —— UITestMockChestRepository.fetchCurrent 应返 status=2 让 unlockable view 出现"
        )

        // Step 2: 等开宝箱按钮可见
        let chestOpenButton = app.buttons["home_chestOpenButton"]
        XCTAssertTrue(
            chestOpenButton.waitForExistence(timeout: timeout),
            "home_chestOpenButton 未渲染 —— ChestCardView unlockableView 内的 PrimaryButton 应可定位"
        )
        XCTAssertTrue(chestOpenButton.isEnabled, "点击前按钮应 enabled（isOpening = false）")

        // Step 3: tap 触发开箱
        chestOpenButton.tap()

        // Step 4: 等 chestCard_counting 出现（mock openChest 成功 → AppState.currentChest = nextChest counting
        // → ChestCardView 重新派生 → counting view 渲染）.
        let chestCardCounting = app.descendants(matching: .any)["chestCard_counting"]
        XCTAssertTrue(
            chestCardCounting.waitForExistence(timeout: timeout),
            "chestCard_counting 未出现 —— 开箱后 nextChest counting 应让 ChestCardView 重新渲染 counting 态"
        )

        // Step 5: chestCard_unlockable 应消失（counting 态不渲染按钮）.
        XCTAssertFalse(
            chestCardUnlockable.exists,
            "开箱后 chestCard_unlockable 应消失（status=counting → 仅渲染 chestCard_counting）"
        )
    }
}
