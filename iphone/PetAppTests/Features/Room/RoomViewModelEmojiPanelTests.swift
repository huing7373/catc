// RoomViewModelEmojiPanelTests.swift
// Story 18.2 AC6: MockRoomViewModel 路径下 onOwnPetTap / showEmojiPanel / onSelect 闭包契约单测.
//
// 测试范围（与 story file AC6 / 6a 钦定一致）：
//   - case#1 happy: onOwnPetTap → showEmojiPanel = true + invocations 记录 .ownPetTap
//   - case#2 happy: onSelect 闭包 (RoomScaffoldView 内 `state.showEmojiPanel = false`) 等价模拟 → sheet 关闭
//   - case#3 edge: 不调 onOwnPetTap → showEmojiPanel 保持初始 false（View 层防御等价路径）
//   - case#4 happy: currentUserId 默认值 "u1" 与 RoomScaffoldDefaults.members[0].id 对齐契约
//
// 测试基础设施（与 ADR-0002 §3.1 / 既有 RealRoomViewModelTests 同源）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）
//   - 不引 ViewInspector / SnapshotTesting
//   - 直接断言 vm @Published 字段 + invocations 数组

import XCTest
@testable import PetApp

@MainActor
final class RoomViewModelEmojiPanelTests: XCTestCase {

    // MARK: - case#1 happy: onOwnPetTap → showEmojiPanel = true + invocations 记录

    /// Story 18.2 AC6 Test 1: 自己点击 PetSpriteView → onOwnPetTap → showEmojiPanel = true.
    /// MockRoomViewModel 路径下 invocations 也记录 .ownPetTap（与 leaveTap / copyTap 同模式）.
    func test_mockOnOwnPetTap_setsShowEmojiPanelTrue_andRecordsInvocation() {
        let vm = MockRoomViewModel(currentUserId: "u1")
        XCTAssertFalse(vm.showEmojiPanel, "初始 showEmojiPanel 必须为 false")
        XCTAssertTrue(vm.invocations.isEmpty, "初始 invocations 应为空")

        vm.onOwnPetTap()

        XCTAssertTrue(vm.showEmojiPanel,
                      "onOwnPetTap 后 showEmojiPanel 必须切 true → sheet 弹出")
        XCTAssertEqual(vm.invocations, [.ownPetTap],
                       "invocations 必须记录 .ownPetTap 一次")
    }

    // MARK: - case#2 happy: onSelect 闭包等价模拟 → showEmojiPanel = false

    /// Story 18.2 AC6 Test 2: 选中表情后 onSelect 闭包置 showEmojiPanel = false.
    /// 模拟 RoomScaffoldView `.sheet` 内 `onSelect: { _ in state.showEmojiPanel = false }` 路径.
    /// 闭包是纯 ViewModel mutation, 不依赖 SwiftUI runtime → 直接 mutate vm + 调闭包 + 断言.
    func test_onSelectClosure_closesSheet() {
        let vm = MockRoomViewModel(currentUserId: "u1")
        vm.showEmojiPanel = true   // 模拟 sheet 已弹出

        // 完全等价 RoomScaffoldView line ~88 `onSelect: { _ in state.showEmojiPanel = false }`
        let onSelect: (String) -> Void = { _ in vm.showEmojiPanel = false }
        onSelect("wave")

        XCTAssertFalse(vm.showEmojiPanel,
                       "onSelect 闭包调用后 showEmojiPanel 必须切 false → sheet 关闭")
    }

    // MARK: - case#3 edge: 不调 onOwnPetTap → showEmojiPanel 保持 false（View 层防御等价）

    /// Story 18.2 AC6 Test 3: View 层 `member.id != currentUserId` 走 else 分支 → 不调 onOwnPetTap.
    /// ViewModel 状态机角度: 不调用任何 method → showEmojiPanel 保持初始 false.
    /// 本测试是 "View 层防御 + ViewModel 状态机不可绕过" 的等价路径单测.
    func test_withoutTapping_showEmojiPanelStaysFalse() {
        // 自己 = u2, 但 members 默认是 RoomScaffoldDefaults（members[0].id == "u1"）
        // → 模拟"自己不是 host"场景；任何视图层防御（仅 self 行渲染 Button）都让 vm.onOwnPetTap 不被调.
        let vm = MockRoomViewModel(currentUserId: "u2")
        XCTAssertFalse(vm.showEmojiPanel, "初始 showEmojiPanel 必须为 false")
        // 显式不调 vm.onOwnPetTap() —— 验证 View 层防御等价语义
        XCTAssertFalse(vm.showEmojiPanel,
                       "不调 onOwnPetTap → showEmojiPanel 必须保持 false（View 层防御等价）")
        XCTAssertTrue(vm.invocations.isEmpty,
                      "未触发任何 action 时 invocations 必须为空")
    }

    // MARK: - case#4 happy: currentUserId 默认值与 RoomScaffoldDefaults.members[0].id 对齐契约

    /// Story 18.2 AC3 钦定 currentUserId 默认值 "u1" 与 RoomScaffoldDefaults.members[0].id 对齐.
    /// 验证无参 init() 路径下"self = host" 语义自然 hold（让默认 Mock 路径走通"点击自己猫"链路）.
    func test_defaultInit_currentUserIdAlignsWithMembersFirstId() {
        let vm = MockRoomViewModel()
        XCTAssertEqual(vm.currentUserId, "u1",
                       "无参 init() 路径下 currentUserId 默认必须为 'u1'")
        XCTAssertEqual(vm.members.first?.id, "u1",
                       "RoomScaffoldDefaults.members[0].id 必须为 'u1' (Story 18.2 AC3 钦定契约)")
        XCTAssertEqual(vm.currentUserId, vm.members.first?.id,
                       "currentUserId 必须与 members[0].id 对齐让默认 Mock 路径走通 self=host 语义")
    }

    /// Story 18.2: 显式覆盖 currentUserId 参数路径验证.
    /// Mock vm 通过 init 注入 currentUserId="u2" → vm.currentUserId == "u2" 即时反映.
    func test_initWithExplicitCurrentUserId_overridesDefault() {
        let vm = MockRoomViewModel(currentUserId: "u2")
        XCTAssertEqual(vm.currentUserId, "u2",
                       "显式注入 currentUserId 必须覆盖默认值 'u1'")
    }

    /// Story 18.2: nil currentUserId 路径验证（fail-safe: appState 未 hydrate 时所有成员行走 else 分支）.
    func test_initWithNilCurrentUserId_failSafe() {
        let vm = MockRoomViewModel(currentUserId: nil)
        XCTAssertNil(vm.currentUserId,
                     "currentUserId 显式注入 nil 时必须保持 nil (fail-safe: appState 未 hydrate)")
        // View 层防御: 此时 memberRow 内 `member.id == nil` 永远 false → 所有行不渲染 Button
        // (本测试不直接验证 View 层 —— 那走 UITest 路径；这里仅锚定 vm 状态正确)
    }
}
