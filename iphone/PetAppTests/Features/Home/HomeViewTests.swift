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

    func testAllSixHomeAccessibilityIdentifiersAreNonEmpty() {
        let identifiers = [
            AccessibilityID.Home.userInfo,
            AccessibilityID.Home.petArea,
            AccessibilityID.Home.stepBalance,
            AccessibilityID.Home.chestArea,
            AccessibilityID.Home.btnRoom,
            AccessibilityID.Home.btnInventory,
            AccessibilityID.Home.btnCompose,
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
            AccessibilityID.Home.btnRoom,
            AccessibilityID.Home.btnInventory,
            AccessibilityID.Home.btnCompose,
            AccessibilityID.Home.versionLabel,
        ]
        let unique = Set(identifiers)
        XCTAssertEqual(unique.count, identifiers.count, "a11y identifier 必须两两不相等")
    }

    func testAccessibilityIdentifierNamingFollowsFeatureUnderscoreElement() {
        // AC6：所有 a11y identifier 使用 <feature>_<element> 命名（小驼峰）
        let identifiers = [
            AccessibilityID.Home.userInfo,
            AccessibilityID.Home.petArea,
            AccessibilityID.Home.stepBalance,
            AccessibilityID.Home.chestArea,
            AccessibilityID.Home.btnRoom,
            AccessibilityID.Home.btnInventory,
            AccessibilityID.Home.btnCompose,
            AccessibilityID.Home.versionLabel,
        ]
        for id in identifiers {
            XCTAssertTrue(id.hasPrefix("home_"), "Home feature a11y id 必须以 home_ 开头：\(id)")
        }
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
            rootView: HomeView(state: viewModel) { EmptyView() }.environmentObject(appState)
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
            rootView: HomeView(state: viewModel) { EmptyView() }.environmentObject(appState)
        )
        controller.view.bounds = CGRect(x: 0, y: 0, width: 430, height: 932)
        controller.view.layoutIfNeeded()
        XCTAssertGreaterThan(controller.view.bounds.width, 0)
    }
}
