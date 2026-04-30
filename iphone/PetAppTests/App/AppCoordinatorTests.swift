// AppCoordinatorTests.swift
// Story 2.3 AC5：AppCoordinator 状态切换覆盖.
//
// Story 37.3 修改（ADR-0009 §3.5 步骤 5 + AC6）：
//   - 删除 `.room` / `.inventory` 用例（SheetType 已删这两个 case；主入口改 4 Tab IA）.
//   - 加 currentTab / switchTab 测试（≥2 case）.
//
// 测试基础设施约束（与 Story 2.7 衔接）：
// - 仅依赖 stdlib（XCTest + @testable import PetApp），不引入 helper 文件.
// - 全部测试方法签名用 throws（同步状态切换，不涉及 await）.
// - @MainActor 标在 class 上（AppCoordinator 是 @MainActor）.

import XCTest
@testable import PetApp

@MainActor
final class AppCoordinatorTests: XCTestCase {

    // MARK: - happy: present(.compose) 后 presentedSheet == .compose

    func testPresentComposeSetsPresentedSheetToCompose() throws {
        let coordinator = AppCoordinator()
        XCTAssertNil(coordinator.presentedSheet, "默认应为 nil")

        coordinator.present(.compose)

        XCTAssertEqual(coordinator.presentedSheet, .compose)
    }

    // MARK: - happy: dismiss() 后 presentedSheet == nil

    func testDismissResetsPresentedSheetToNil() throws {
        let coordinator = AppCoordinator(presentedSheet: .compose)
        XCTAssertEqual(coordinator.presentedSheet, .compose, "前置：构造为 .compose")

        coordinator.dismiss()

        XCTAssertNil(coordinator.presentedSheet)
    }

    // MARK: - edge: 连续 dismiss 多次（state 已是 nil）→ 不抛异常 / 仍为 nil

    func testRepeatedDismissIsIdempotent() throws {
        let coordinator = AppCoordinator()
        XCTAssertNil(coordinator.presentedSheet, "前置：默认 nil")

        coordinator.dismiss()
        coordinator.dismiss()
        coordinator.dismiss()

        XCTAssertNil(coordinator.presentedSheet, "连续 dismiss 后仍为 nil")
    }

    // MARK: - happy: init 默认参数 nil（守护未来不要不小心改默认值）

    func testInitDefaultsToNilPresentedSheet() throws {
        let coordinator = AppCoordinator()
        XCTAssertNil(coordinator.presentedSheet)
    }

    // MARK: - Story 37.3：currentTab / switchTab 测试

    /// happy: coordinator.currentTab 默认值 .home.
    func testCurrentTabDefaultsToHome() throws {
        let coordinator = AppCoordinator()
        XCTAssertEqual(coordinator.currentTab, .home, "currentTab 默认应为 .home")
    }

    /// happy: coordinator.switchTab(.wardrobe) → coordinator.currentTab == .wardrobe.
    func testSwitchTabUpdatesCurrentTab() throws {
        let coordinator = AppCoordinator()
        XCTAssertEqual(coordinator.currentTab, .home, "前置：default .home")

        coordinator.switchTab(.wardrobe)
        XCTAssertEqual(coordinator.currentTab, .wardrobe)

        coordinator.switchTab(.profile)
        XCTAssertEqual(coordinator.currentTab, .profile)

        coordinator.switchTab(.friends)
        XCTAssertEqual(coordinator.currentTab, .friends)

        coordinator.switchTab(.home)
        XCTAssertEqual(coordinator.currentTab, .home)
    }

    /// happy: 程式化切 Tab 不影响 presentedSheet（次级 sheet 与 Tab 互不干扰）.
    func testSwitchTabDoesNotAffectPresentedSheet() throws {
        let coordinator = AppCoordinator()
        coordinator.present(.compose)
        XCTAssertEqual(coordinator.presentedSheet, .compose)

        coordinator.switchTab(.wardrobe)

        XCTAssertEqual(coordinator.presentedSheet, .compose, "切 Tab 不应改变 presentedSheet")
        XCTAssertEqual(coordinator.currentTab, .wardrobe)
    }

    // MARK: - Story 37.3：currentRoomId 临时占位字段（Story 37.4 落地后删除）

    /// happy: currentRoomId 默认值 nil.
    func testCurrentRoomIdDefaultsToNil() throws {
        let coordinator = AppCoordinator()
        XCTAssertNil(coordinator.currentRoomId, "currentRoomId 默认应为 nil")
    }

    /// happy: 写入 currentRoomId 后可读出（驱动 HomeContainerView 互斥状态机切换）.
    func testCurrentRoomIdCanBeAssigned() throws {
        let coordinator = AppCoordinator()

        coordinator.currentRoomId = "room_1234567"
        XCTAssertEqual(coordinator.currentRoomId, "room_1234567")

        coordinator.currentRoomId = nil
        XCTAssertNil(coordinator.currentRoomId)
    }
}
