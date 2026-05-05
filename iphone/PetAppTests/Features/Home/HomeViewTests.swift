// HomeViewTests.swift
// Story 2.2 AC4 case#1（happy）+ case#2（edge）：
//   - case#1：6 个 a11y identifier 常量值不为空，且两两不相等（简化方案）
//   - case#2：HomeView 在 iPhone SE / iPhone 15 Pro Max 两种 bounds 下渲染不 crash
//
// 注：a11y identifier 在 view hierarchy 中真实出现的验证由 PetAppUITests/HomeUITests 覆盖。

import XCTest
import SwiftUI
@testable import PetApp

@MainActor
final class HomeViewTests: XCTestCase {

    // MARK: - case#1（happy）：a11y identifier 常量值不为空 + 两两不相等
    //
    // Story 37.13: deprecated `btnRoom` / `btnInventory` / `btnCompose` 3 常量从 AccessibilityID.Home
    // 删除（Story 37.3 主入口已改 4 Tab IA, 3 CTA 按钮 view 已删, 常量保留无意义）；
    // 本 case 同步从 identifiers 数组移除，避免 compile error.

    func testAllHomeAccessibilityIdentifiersAreNonEmpty() {
        let identifiers = [
            AccessibilityID.Home.userInfo,
            AccessibilityID.Home.petArea,
            AccessibilityID.Home.stepBalance,
            AccessibilityID.Home.chestArea,
            AccessibilityID.Home.versionLabel,
        ]
        for id in identifiers {
            XCTAssertFalse(id.isEmpty, "a11y identifier 不应为空：\(id)")
        }
    }

    func testAllHomeAccessibilityIdentifiersAreUnique() {
        let identifiers = [
            AccessibilityID.Home.userInfo,
            AccessibilityID.Home.petArea,
            AccessibilityID.Home.stepBalance,
            AccessibilityID.Home.chestArea,
            AccessibilityID.Home.versionLabel,
        ]
        let unique = Set(identifiers)
        XCTAssertEqual(unique.count, identifiers.count, "a11y identifier 必须两两不相等")
    }

    func testAccessibilityIdentifierNamingFollowsFeatureUnderscoreElement() {
        // AC6：Home feature a11y id 应使用 home 前缀（snake_case `home_xxx` 主流；
        //   Story 37.7 AC8 新锚使用 camelCase `homeXxx`，作为 ui_design 高保真区块锚兼容路径）.
        // Story 37.7 codex round 3 [P2-B] fix：AccessibilityID.Home.userInfo 值从 "home_userInfo" 改
        //   "homeStatusBar"（解 VoiceOver 空 Text overlay 卡顿；同时 AC8 UITest 字面量 "homeStatusBar" 自动命中）.
        let snakeCaseIdentifiers = [
            AccessibilityID.Home.petArea,
            AccessibilityID.Home.stepBalance,
            AccessibilityID.Home.chestArea,
            AccessibilityID.Home.versionLabel,
        ]
        for id in snakeCaseIdentifiers {
            XCTAssertTrue(id.hasPrefix("home_"), "Home feature snake_case a11y id 必须以 home_ 开头：\(id)")
        }

        // userInfo 单独验证（值已改 "homeStatusBar" —— camelCase，仍以 home 前缀维持 feature scope 可识别）.
        XCTAssertTrue(
            AccessibilityID.Home.userInfo.hasPrefix("home"),
            "AccessibilityID.Home.userInfo 必须以 home 前缀维持 feature scope 可识别：\(AccessibilityID.Home.userInfo)"
        )
    }

    // MARK: - case#2（edge）：不同尺寸下渲染不 crash
    //
    // Story 37.4 改造：HomeView 现在依赖 @EnvironmentObject AppState；测试用 .environmentObject(_:)
    // 注入空态 AppState（hasHydrated == false → 渲染 loading placeholder，与 UITest skip-guest-login 路径一致）.

    // Story 37.7: HomeView 改 generic struct + chestSlot ViewBuilder closure；
    // 用 MockHomeViewModel（基类 HomeViewModel 的 abstract method 用 fatalError 占位，不能直接实例化测视图行为）.
    // 这里两 case 仅验证 layout 不 crash —— 视图不调任何 onXxxTap，所以基类 HomeViewModel 也可用,
    // 但为统一起见仍走 MockHomeViewModel（避免未来扩展测试场景时遇到 abstract method 路径 crash）.
    func testHomeViewRendersOnSmallScreenWithoutCrash() {
        // iPhone SE (3rd gen) ≈ 375 x 667
        let viewModel = MockHomeViewModel()
        let appState = AppState()
        let controller = UIHostingController(
            rootView: HomeView(
                state: viewModel,
                petSlot: { PetSpriteView(state: .rest) },
                chestSlot: { EmptyView() }
            ).environmentObject(appState)
        )
        controller.view.bounds = CGRect(x: 0, y: 0, width: 375, height: 667)
        controller.view.layoutIfNeeded()
        XCTAssertGreaterThan(controller.view.bounds.width, 0)
    }

    func testHomeViewRendersOnLargeScreenWithoutCrash() {
        // iPhone 15 Pro Max ≈ 430 x 932
        let viewModel = MockHomeViewModel()
        let appState = AppState()
        let controller = UIHostingController(
            rootView: HomeView(
                state: viewModel,
                petSlot: { PetSpriteView(state: .rest) },
                chestSlot: { EmptyView() }
            ).environmentObject(appState)
        )
        controller.view.bounds = CGRect(x: 0, y: 0, width: 430, height: 932)
        controller.view.layoutIfNeeded()
        XCTAssertGreaterThan(controller.view.bounds.width, 0)
    }
}
