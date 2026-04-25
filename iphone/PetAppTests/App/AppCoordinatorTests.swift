// AppCoordinatorTests.swift
// Story 2.3 AC5：AppCoordinator 状态切换覆盖（≥5 case）。
//
// 测试基础设施约束（与 Story 2.7 衔接）：
// - 仅依赖 stdlib（XCTest + @testable import PetApp），不引入 helper 文件。
// - 全部测试方法签名用 throws（同步状态切换，不涉及 await）。
// - @MainActor 标在 class 上（AppCoordinator 是 @MainActor）。

import XCTest
@testable import PetApp

@MainActor
final class AppCoordinatorTests: XCTestCase {

    // MARK: - happy: present(.room) 后 presentedSheet == .room

    func testPresentRoomSetsPresentedSheetToRoom() throws {
        let coordinator = AppCoordinator()
        XCTAssertNil(coordinator.presentedSheet, "默认应为 nil")

        coordinator.present(.room)

        XCTAssertEqual(coordinator.presentedSheet, .room)
    }

    // MARK: - happy: dismiss() 后 presentedSheet == nil

    func testDismissResetsPresentedSheetToNil() throws {
        let coordinator = AppCoordinator(presentedSheet: .room)
        XCTAssertEqual(coordinator.presentedSheet, .room, "前置：构造为 .room")

        coordinator.dismiss()

        XCTAssertNil(coordinator.presentedSheet)
    }

    // MARK: - edge: 已 present 一个 sheet，再 present 另一个 → 后者覆盖前者

    func testPresentNewSheetOverridesExistingSheet() throws {
        let coordinator = AppCoordinator(presentedSheet: .inventory)
        XCTAssertEqual(coordinator.presentedSheet, .inventory, "前置：构造为 .inventory")

        coordinator.present(.compose)

        XCTAssertEqual(coordinator.presentedSheet, .compose)
    }

    // MARK: - happy: 三种 SheetType 全 case 覆盖（防 future 加新 case 漏测）

    func testEachSheetTypeCanBePresentedAndDismissed() throws {
        let coordinator = AppCoordinator()

        for sheetType in [SheetType.room, .inventory, .compose] {
            coordinator.present(sheetType)
            XCTAssertEqual(coordinator.presentedSheet, sheetType,
                           "present(\(sheetType)) 后 presentedSheet 应等于该值")

            coordinator.dismiss()
            XCTAssertNil(coordinator.presentedSheet,
                         "dismiss() 后 presentedSheet 应回到 nil（after \(sheetType)）")
        }
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
}
