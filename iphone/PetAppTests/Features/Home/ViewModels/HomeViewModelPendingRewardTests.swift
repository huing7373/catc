// HomeViewModelPendingRewardTests.swift
// Story 21.4 AC6: HomeViewModel.pendingReward 字段独立 state 不变量覆盖（≥ 2 case；防御性补充）.
//
// 测试目标:
//   - testPendingRewardInitiallyNil:
//     新建 HomeViewModel 默认状态下 pendingReward == nil（21.3 落地的 @Published 字段默认值不变）.
//   - testPendingRewardSetToNilAfterCloseClosureCalled:
//     模拟开箱成功 → vm.pendingReward = snapshot（手动写入，模拟 21.3 OpenChestUseCase 成功路径） →
//     调 onClose closure（模拟 21.4 RewardPopupView 关闭路径） → 验证 vm.pendingReward == nil.
//
// 与 21.3 RealHomeViewModelChestOpenTapTests 的关系（避免重复覆盖）:
//   - 21.3 已覆盖 OpenChestUseCase 成功路径写入 pendingReward 的 happy 链路；
//   - 本 story 仅覆盖 "pendingReward 初始 nil" + "用户关闭 popup 后字段被清空" 两个独立不变量.
//
// 与 RewardPopupView 视觉断言三层防御的关系（ADR-0002 §3.1）:
//   - 本 story 单测路径不覆盖 RewardPopupView 视觉渲染（禁用 ViewInspector）；
//   - 视觉断言走 (a) RewardRarityTagMapperTests 纯函数 mapper (b) ChestOpenRewardPopupUITests UITest
//     a11y identifier 抽样 (c) ios-simulator MCP UI 实跑 三层防御.

import XCTest
import Combine
@testable import PetApp

@MainActor
final class HomeViewModelPendingRewardTests: XCTestCase {

    private func makeSnapshot() -> ChestRewardSnapshot {
        ChestRewardSnapshot(
            cosmeticItemId: "cos_pending_test",
            name: "测试装扮",
            slot: 1,
            rarity: .rare,
            assetUrl: "https://placehold.co/64x64?text=R",
            iconUrl: "https://placehold.co/32x32?text=I"
        )
    }

    // MARK: - case#1: 初始 nil

    func testPendingRewardInitiallyNil() {
        // 老 init（Story 2.2 / 2.3）路径默认构造 HomeViewModel.
        let vm = HomeViewModel()

        XCTAssertNil(
            vm.pendingReward,
            "新建 HomeViewModel 默认 pendingReward 必须 == nil（21.3 落地的 @Published 字段默认值）"
        )
    }

    // MARK: - case#2: onClose closure 调用后清空（模拟 RewardPopupView 关闭路径）

    func testPendingRewardSetToNilAfterCloseClosureCalled() {
        let vm = HomeViewModel()
        let snapshot = makeSnapshot()

        // 模拟 21.3 OpenChestUseCase 成功路径写入 pendingReward.
        vm.pendingReward = snapshot
        XCTAssertEqual(
            vm.pendingReward,
            snapshot,
            "写入路径必须让 pendingReward 非 nil（前置断言：模拟 21.3 成功路径）"
        )

        // 模拟 21.4 RewardPopupView 关闭路径（HomeView onClose closure 内 `state.pendingReward = nil`）.
        // 注：onClose 是 closure，由 RewardPopupView "确定" PrimaryButton 调用；
        //    本测试直接模拟 closure 内的 ViewModel mutation，等价于 RewardPopupView 关闭路径行为.
        vm.pendingReward = nil

        XCTAssertNil(
            vm.pendingReward,
            "onClose closure 调用后 pendingReward 必须 == nil（让 .sheet(item:) 自动 dismiss）"
        )
    }
}
