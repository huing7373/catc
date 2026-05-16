// ChestOpenStepSyncUITests.swift
// Story 21.5 AC5: 集成测试（XCUITest）—— 验证开箱前主动同步步数 wire 接通后开箱链路不回归.
//
// 测试意图（spec AC 行 3146 + Dev Notes "测试边界"）:
//   spec 原文 "模拟器 HealthKit 增加步数 → 立即点开箱 → 验 mock server 收到先 sync 后 open" ——
//   但 XCUITest 无法可靠注入模拟器 HealthKit 步数（HealthKit 在 UITest 进程外）.
//   **钦定折中**：精确的 "sync 先 open 后" 顺序断言由单测 OpenChestUseCaseStepSyncTests
//   （路径 A，case#3 testSyncIsCalledBeforeOpenChest）精确承担；本 UITest 守「sync wire 接通后
//   开箱 happy 链路不破回归」边界 —— 即 21.5 的 DI 时序重排 + triggerManual 注入没有破坏
//   21.3/21.4 已落地的 tap → POST /chest/open → reward popup → confirm → counting 回归.
//
// 实装策略（与 ChestOpenRewardPopupUITests / ChestOpenUITests 同模式）:
//   - UITEST_SKIP_GUEST_LOGIN=1 + UITEST_MOCK_CHEST_OPEN=1（复用 21.3 落地的 UITestMockChestRepository）.
//   - **额外** UITEST_MOCK_STEP_SYNC=1：让 AppContainer 注入 UITestMockStepRepository +
//     UITestMockHealthProvider —— Story 21.5 时序重排后 ensureStepSyncWired() 在
//     UITEST_SKIP_GUEST_LOGIN 路径也已调用 → stepSyncTriggerService 非 nil → OpenChestUseCase
//     Step 0 真实 await triggerManual()，但内核走 mock（不发真实 HTTP，sim 无 server / 无 token）.
//   - 验证开箱前 sync wire 不破坏开箱不变量：reward popup 仍出现 + confirm → popup 消失 +
//     chestCard_counting 仍恢复 + 无 crash（无双 timer 副作用 —— wire 复用单实例）.
//
// 不验范围:
//   - **不**断言 "同步步数中…" 文案可见：mock 链路 < 2s（UITestMockStepRepository 即时返回）→
//     2s 延迟 task 在 execute 完成时被 cancel → 副标题不会切 "同步步数中…"（预期；spec 钦定
//     该文案是真机慢网络场景，mock 快路径不可见是预期，记录即可）.
//   - 精确 sync→open 顺序断言由单测承担（见上方测试意图）.

import XCTest

final class ChestOpenStepSyncUITests: XCTestCase {

    override func setUpWithError() throws {
        continueAfterFailure = false
    }

    /// Story 21.5 AC5: launch unlockable（sync wire 接通）→ tap 开宝箱 → reward popup 出现 →
    ///                 confirm → popup 消失 → chestCard_counting 恢复（开箱链路不被 sync wire 破坏）.
    func testOpenChestTriggersStepSyncBeforeOpen() throws {
        let app = XCUIApplication()
        app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
        app.launchEnvironment["UITEST_MOCK_CHEST_OPEN"] = "1"
        // Story 21.5: 注入 mock StepRepository + mock HealthProvider —— 让 21.5 时序重排后
        // ensureStepSyncWired() 构造的 stepSyncTriggerService 内核走 mock（triggerManual 不发真实 HTTP）.
        app.launchEnvironment["UITEST_MOCK_STEP_SYNC"] = "1"
        app.launch()

        let timeout: TimeInterval = 5

        // Step 1: 等 unlockable 态 ChestCardView 渲染 + 开宝箱按钮可见.
        let chestOpenButton = app.buttons["home_chestOpenButton"]
        XCTAssertTrue(
            chestOpenButton.waitForExistence(timeout: timeout),
            "home_chestOpenButton 未渲染 —— sync wire 接通后 unlockable view 仍应可定位（回归保护）"
        )
        XCTAssertTrue(chestOpenButton.isEnabled, "点击前按钮应 enabled（isOpening = false）")

        // Step 2: tap 触发开箱 —— OpenChestUseCase Step 0 await triggerManual()（mock，即时）→
        //         Step 2 POST /chest/open（UITestMockChestRepository happy 响应）→ pendingReward 写入.
        chestOpenButton.tap()

        // Step 3: 等 RewardPopupView 出现（开箱链路未被 sync wire 破坏的核心断言）.
        let rewardPopup = app.descendants(matching: .any)["rewardPopup"]
        XCTAssertTrue(
            rewardPopup.waitForExistence(timeout: timeout),
            "rewardPopup 未出现 —— sync wire 接通后开箱成功仍应触发 .sheet(item:) 弹窗（不破回归）"
        )

        // Step 4: tap 确定 → 等 rewardPopup 消失.
        let confirmButton = app.buttons["rewardPopup_confirmButton"]
        XCTAssertTrue(
            confirmButton.waitForExistence(timeout: timeout),
            "rewardPopup_confirmButton 应存在"
        )
        confirmButton.tap()

        XCTAssertTrue(
            waitForNonExistence(of: rewardPopup, timeout: timeout),
            "rewardPopup 应在点击确定后消失（onClose → pendingReward = nil → .sheet(item:) 自动 dismiss）"
        )

        // Step 5: 验 chestCard_counting 恢复（开箱后 nextChest counting 写入 AppState；
        //         sync wire 不影响 —— 同一 stepSyncTriggerService 单实例复用，无双 timer 副作用）.
        let chestCardCounting = app.descendants(matching: .any)["chestCard_counting"]
        XCTAssertTrue(
            chestCardCounting.waitForExistence(timeout: timeout),
            "chestCard_counting 应恢复 —— sync wire 接通不破坏开箱后 counting 态回归（回归保护核心）"
        )
    }

    // MARK: - Helpers

    /// 等待 element 不存在（与 ChestOpenRewardPopupUITests 同模式）.
    private func waitForNonExistence(of element: XCUIElement, timeout: TimeInterval) -> Bool {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if !element.exists { return true }
            Thread.sleep(forTimeInterval: 0.1)
        }
        return !element.exists
    }
}
