// HomeViewModelTests.swift
// Story 2.2 AC4 case#3：HomeViewModel 三个主按钮 closure 注册链路验证。
//
// 测试基础设施约束（与 Story 2.7 衔接）：
// - 仅依赖 stdlib（XCTest + @testable import PetApp），不引入 helper 文件。

import XCTest
@testable import PetApp

@MainActor
final class HomeViewModelTests: XCTestCase {

    func testOnRoomTapInvokesInjectedClosure() {
        var roomTapped = false
        let viewModel = HomeViewModel()
        viewModel.onRoomTap = { roomTapped = true }

        viewModel.onRoomTap()

        XCTAssertTrue(roomTapped, "onRoomTap 注入闭包未被调用")
    }

    func testOnInventoryTapInvokesInjectedClosure() {
        var inventoryTapped = false
        let viewModel = HomeViewModel()
        viewModel.onInventoryTap = { inventoryTapped = true }

        viewModel.onInventoryTap()

        XCTAssertTrue(inventoryTapped, "onInventoryTap 注入闭包未被调用")
    }

    func testOnComposeTapInvokesInjectedClosure() {
        var composeTapped = false
        let viewModel = HomeViewModel()
        viewModel.onComposeTap = { composeTapped = true }

        viewModel.onComposeTap()

        XCTAssertTrue(composeTapped, "onComposeTap 注入闭包未被调用")
    }

    func testDefaultClosuresAreNoOp() {
        // 默认空函数：调用不抛异常 / 不 crash 即可
        let viewModel = HomeViewModel()
        viewModel.onRoomTap()
        viewModel.onInventoryTap()
        viewModel.onComposeTap()
    }

    func testHardcodedDefaultStateMatchesStorySpec() {
        let viewModel = HomeViewModel()
        XCTAssertEqual(viewModel.nickname, "用户1001")
        XCTAssertEqual(viewModel.appVersion, "0.0.0")
        XCTAssertEqual(viewModel.serverInfo, "----")
    }
}
