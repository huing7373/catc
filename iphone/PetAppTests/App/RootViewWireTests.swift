// RootViewWireTests.swift
// Story 2.3 AC6：HomeViewModel ↔ AppCoordinator 闭包注入链路验证（≥3 case）。
//
// 由于 RootView 是 SwiftUI struct，@StateObject 不易在单测中直接构造（需要 view body 渲染才会触发 .onAppear），
// 本 story 选间接验证策略：在测试中模拟 RootView 的 wire 逻辑：
//   viewModel.onRoomTap = { [coordinator] in coordinator.present(.room) }
// 然后调用闭包，验证 coordinator.presentedSheet 被设到对应值。
//
// RootView 的视图渲染由 PetAppUITests/NavigationUITests 兜底（黑盒）。

import XCTest
@testable import PetApp

@MainActor
final class RootViewWireTests: XCTestCase {

    // MARK: - happy: onRoomTap 闭包接到 coordinator.present(.room)

    func testOnRoomTapClosureRoutesToCoordinatorPresentRoom() throws {
        let coordinator = AppCoordinator()
        let viewModel = HomeViewModel()

        viewModel.onRoomTap = { [coordinator] in coordinator.present(.room) }

        viewModel.onRoomTap()

        XCTAssertEqual(coordinator.presentedSheet, .room,
                       "onRoomTap 闭包未把 coordinator 路由到 .room")
    }

    // MARK: - happy: onInventoryTap 闭包接到 coordinator.present(.inventory)

    func testOnInventoryTapClosureRoutesToCoordinatorPresentInventory() throws {
        let coordinator = AppCoordinator()
        let viewModel = HomeViewModel()

        viewModel.onInventoryTap = { [coordinator] in coordinator.present(.inventory) }

        viewModel.onInventoryTap()

        XCTAssertEqual(coordinator.presentedSheet, .inventory,
                       "onInventoryTap 闭包未把 coordinator 路由到 .inventory")
    }

    // MARK: - happy: onComposeTap 闭包接到 coordinator.present(.compose)

    func testOnComposeTapClosureRoutesToCoordinatorPresentCompose() throws {
        let coordinator = AppCoordinator()
        let viewModel = HomeViewModel()

        viewModel.onComposeTap = { [coordinator] in coordinator.present(.compose) }

        viewModel.onComposeTap()

        XCTAssertEqual(coordinator.presentedSheet, .compose,
                       "onComposeTap 闭包未把 coordinator 路由到 .compose")
    }
}
