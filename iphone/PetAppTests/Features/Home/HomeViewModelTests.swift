// HomeViewModelTests.swift
// Story 2.2 AC4 case#3：HomeViewModel 默认状态 + 字段约束验证.
//
// Story 37.3 修改（ADR-0009 §3.5 步骤 4 / AC4）：
//   - 删除 onRoomTap / onInventoryTap / onComposeTap 三个 closure 字段及其测试用例
//     （HomeViewModel 不再持有这三个 closure；主入口改 4 Tab IA）.
//   - 保留 testHardcodedDefaultStateMatchesStorySpec（守护 nickname / appVersion / serverInfo
//     默认值不漂移）.
//
// 测试基础设施约束（与 Story 2.7 衔接）：
// - 仅依赖 stdlib（XCTest + @testable import PetApp），不引入 helper 文件.

import XCTest
@testable import PetApp

@MainActor
final class HomeViewModelTests: XCTestCase {

    func testHardcodedDefaultStateMatchesStorySpec() {
        let viewModel = HomeViewModel()
        XCTAssertEqual(viewModel.nickname, "用户1001")
        XCTAssertEqual(viewModel.appVersion, "0.0.0")
        XCTAssertEqual(viewModel.serverInfo, "----")
    }
}
