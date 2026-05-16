// RealHomeViewModelSyncHintTests.swift
// Story 21.5 AC4（路径 B）: 验证 RealHomeViewModel.onChestOpenTap 的 isSyncingSteps 2s 阈值行为.
//
// 测试策略（spec Dev Notes "测试边界" 路径 B + AC4 钦定 case 3/4）:
//   - 注入 mock OpenChestUseCaseProtocol（既有 MockOpenChestUseCase，定义在
//     RealHomeViewModelChestOpenTapTests.swift，同 target 可复用）.
//   - 用 mock 的 `onExecute` async hook 注入可控耗时（Task.sleep）模拟 execute() 整体时延.
//   - 钦定折中（spec Dev Notes）：isSyncingSteps 锚定 execute() 整体调用 > 2s，不锚 triggerManual
//     精确边界（不改 21.3 冻结的 execute() 内部加回调）.
//
// 覆盖 spec AC 行 3144-3145 + AC3 defer 复位:
//   - case#3 execute 耗时 < 2s（0.2s）→ isSyncingSteps 全程 false（同步快 → 不显提示）
//   - case#4 execute 耗时 > 2s（2.3s）→ 2s 后 isSyncingSteps == true，execute 返回后 == false
//   - case#5 execute 抛错（快路径 0.2s）→ isSyncingSteps 仍复位 false（defer 三路径保证）
//
// timer 单测稳定性（spec AC4）：阈值（2s）硬编码在 RealHomeViewModel 生产代码内，spec 红线禁改
// execute()，故 slow case 必须真等 ~2.3s（< 4s CI 阈值，spec 钦定可接受）；fast case 仅 0.2s.

import XCTest
import Combine
@testable import PetApp

@MainActor
final class RealHomeViewModelSyncHintTests: XCTestCase {

    // MARK: - Helpers

    private func makeSnapshot() -> ChestRewardSnapshot {
        ChestRewardSnapshot(
            cosmeticItemId: "cos_001",
            name: "星星围巾",
            slot: 1,
            rarity: .common,
            assetUrl: "https://placehold.co/64x64?text=R",
            iconUrl: "https://placehold.co/32x32?text=I"
        )
    }

    private func makeBoundVM(
        mockUseCase: MockOpenChestUseCase
    ) -> (RealHomeViewModel, ErrorPresenter) {
        let appState = AppState()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let vm = RealHomeViewModel(appState: appState)
        vm.bind(
            createRoomUseCase: MockCreateRoomUseCase(),
            joinRoomUseCase: MockJoinRoomUseCase(),
            errorPresenter: presenter
        )
        vm.bind(openChestUseCase: mockUseCase)
        return (vm, presenter)
    }

    /// poll 直到条件成立或超时（避免 race + 不引入固定 sleep）.
    private func waitUntil(
        timeout: TimeInterval,
        _ condition: @MainActor () -> Bool
    ) async {
        let deadline = Date().addingTimeInterval(timeout)
        while Date() < deadline {
            if condition() { return }
            try? await Task.sleep(nanoseconds: 20_000_000)  // 20ms
        }
    }

    // MARK: - case#3: execute 耗时 < 2s → isSyncingSteps 全程 false（spec AC 行 3144）

    func testSyncHintNotShownWhenExecuteFast() async {
        let mockUseCase = MockOpenChestUseCase()
        mockUseCase.executeStub = .success(makeSnapshot())
        // execute 整体耗时 0.2s（远 < 2s 阈值）.
        mockUseCase.onExecute = { @Sendable in
            try? await Task.sleep(nanoseconds: 200_000_000)
        }
        let (vm, _) = makeBoundVM(mockUseCase: mockUseCase)

        vm.onChestOpenTap()
        XCTAssertTrue(vm.isOpening, "tap 后 isOpening 同步段应 true")
        XCTAssertFalse(vm.isSyncingSteps, "tap 瞬间 isSyncingSteps 还不应 true（2s 延迟 task 未到）")

        // 等 execute 完成（pendingReward 写入即代表成功返回）.
        await waitUntil(timeout: 2.0) { vm.pendingReward != nil }

        XCTAssertNotNil(vm.pendingReward, "0.2s execute 应在 2s 内成功返回")
        // 给 defer 一个 tick 复位 isOpening/isSyncingSteps.
        await waitUntil(timeout: 1.0) { !vm.isOpening }
        XCTAssertFalse(vm.isSyncingSteps,
                       "execute 0.2s < 2s → 2s 延迟 task 在 execute 完成时被 cancel → isSyncingSteps 全程 false")
        XCTAssertFalse(vm.isOpening, "execute 返回后 isOpening 复位 false")
    }

    // MARK: - case#4: execute 耗时 > 2s → 2s 后 isSyncingSteps==true，返回后==false（spec AC 行 3145）

    func testSyncHintShownWhenExecuteSlow() async {
        let mockUseCase = MockOpenChestUseCase()
        mockUseCase.executeStub = .success(makeSnapshot())
        // execute 整体耗时 2.3s（> 2s 阈值；2s 延迟 task 应有机会 set isSyncingSteps = true）.
        mockUseCase.onExecute = { @Sendable in
            try? await Task.sleep(nanoseconds: 2_300_000_000)
        }
        let (vm, _) = makeBoundVM(mockUseCase: mockUseCase)

        vm.onChestOpenTap()
        XCTAssertFalse(vm.isSyncingSteps, "tap 瞬间 isSyncingSteps 还不应 true（2s 延迟未到）")

        // 等 2s 阈值跨过 → isSyncingSteps 应翻 true（poll，给延迟 task 调度余量）.
        await waitUntil(timeout: 3.0) { vm.isSyncingSteps }
        XCTAssertTrue(vm.isSyncingSteps,
                      "execute 2.3s > 2s → 2s 延迟 task fire 应 set isSyncingSteps = true")
        XCTAssertTrue(vm.isOpening, "sync 阶段 isOpening 仍为 true（整个 execute 期间）")

        // 等 execute 真正返回 → defer 复位 isSyncingSteps + isOpening.
        await waitUntil(timeout: 3.0) { vm.pendingReward != nil }
        await waitUntil(timeout: 1.0) { !vm.isSyncingSteps && !vm.isOpening }
        XCTAssertFalse(vm.isSyncingSteps, "execute 返回后 isSyncingSteps 必复位 false（defer）")
        XCTAssertFalse(vm.isOpening, "execute 返回后 isOpening 必复位 false（defer）")
        XCTAssertNotNil(vm.pendingReward, "成功路径写 pendingReward")
    }

    // MARK: - case#5: execute 抛错（快路径）→ isSyncingSteps 仍复位 false（AC3 defer 三路径）

    func testSyncHintResetsFalseWhenExecuteThrows() async {
        let mockUseCase = MockOpenChestUseCase()
        mockUseCase.executeStub = .failure(APIError.business(code: 3002, message: "步数不足", requestId: "r"))
        mockUseCase.onExecute = { @Sendable in
            try? await Task.sleep(nanoseconds: 150_000_000)  // 0.15s（< 2s）
        }
        let (vm, presenter) = makeBoundVM(mockUseCase: mockUseCase)

        vm.onChestOpenTap()

        // 等错误弹出（execute 抛 → catch → presentAlert）.
        await waitUntil(timeout: 2.0) { presenter.current != nil }
        await waitUntil(timeout: 1.0) { !vm.isOpening }

        XCTAssertFalse(vm.isSyncingSteps,
                       "execute 抛错路径 isSyncingSteps 仍必复位 false（defer 覆盖成功/抛APIError/抛非APIError 三路径）")
        XCTAssertFalse(vm.isOpening, "execute 抛错路径 isOpening 也必复位 false（defer）")
        XCTAssertNil(vm.pendingReward, "抛错路径不写 pendingReward")
    }
}
