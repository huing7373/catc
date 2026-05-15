// ChestTimerDriver.swift
// Story 21.1 AC2: 本地宝箱倒计时驱动（订阅 AppState.currentChest → 每秒写 HomeViewModel.chestRemainingSeconds）.
//
// 设计：
//   - 弱引用 HomeViewModel（防循环；与 Story 8.4 MotionStateMapper 同模式）.
//   - 弱引用 AppState（防循环；ADR-0010 §3.1 ViewModel 注入 AppState 是 strong，driver 是 weak）.
//   - 订阅 appState.$currentChest（Combine sink）→ 每次变化 cancel 老 Task + 启动新 Task.
//   - 倒计时 Task 内 @MainActor `while !Task.isCancelled` 循环每秒 sleep 1s + 计算 remainingSeconds 写回.
//   - 倒计时来源：`max(0, Int(currentChest.unlockAt.timeIntervalSince(Date())))`（绝对时间 → 相对秒数）；
//     这样既不依赖 server 给的 remainingSeconds 初始值（避免双源不一致），也保 timer drift 自校准.
//   - 倒计时到 0 时不停 Task（等待 currentChest 再次变化）—— Story 21.2 落地后 LoadChestUseCase 60s 定时拉取
//     会把 currentChest.status 切到 unlockable + 新 unlockAt，driver 自然 react；本 story 阶段倒计时到 0 后
//     视图通过 status / remainingSeconds 派生切到 unlockable 视觉态.
//
// 红线：
//   - 不调用任何 UseCase / Repository（driver 纯 view-state 派生）.
//   - 不依赖 SwiftUI（纯 ObservableObject + Combine + Foundation）.
//   - 不暴露 public 字段（driver 是 ViewModel 内部 helper；唯一对外 API 是 init + start + stop）.

import Foundation
import Combine

@MainActor
public final class ChestTimerDriver {
    private weak var appState: AppState?
    private weak var viewModel: HomeViewModel?
    private var subscription: AnyCancellable?
    private var tickTask: Task<Void, Never>?

    public init(appState: AppState, viewModel: HomeViewModel) {
        self.appState = appState
        self.viewModel = viewModel
    }

    /// 启动 driver：订阅 appState.$currentChest，首次启动立即用当前值跑一次 recompute.
    public func start() {
        guard subscription == nil else { return }  // 防双启
        subscription = appState?.$currentChest
            .receive(on: DispatchQueue.main)
            .sink { [weak self] newChest in
                self?.handleChestChange(newChest)
            }
    }

    /// 停止 driver（dealloc 时调；测试可显式调以验证 Task 取消）.
    public func stop() {
        subscription?.cancel()
        subscription = nil
        tickTask?.cancel()
        tickTask = nil
    }

    private func handleChestChange(_ chest: HomeChest?) {
        // 老 timer 必停（防 ABA：旧 chest unlockAt + 新 chest unlockAt 两 Task 并存写 chestRemainingSeconds）.
        tickTask?.cancel()

        guard let chest else {
            // currentChest 为 nil（未 hydrate 或被清空）→ remainingSeconds = 0 + 不启 Task.
            viewModel?.chestRemainingSeconds = 0
            return
        }

        // 首次/重启：立即算一次 + 启 Task 每秒 tick.
        recomputeAndWrite(unlockAt: chest.unlockAt)
        let chestId = chest.id
        tickTask = Task { @MainActor [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 1_000_000_000)
                guard !Task.isCancelled else { return }
                guard let self else { return }
                // 防 ABA：tickTask 启动后 currentChest 被换掉，sink 已 cancel 老 task；这里再 guard 一次 chest 仍是同一个.
                guard self.appState?.currentChest?.id == chestId else { return }
                self.recomputeAndWrite(unlockAt: chest.unlockAt)
            }
        }
    }

    private func recomputeAndWrite(unlockAt: Date) {
        let remaining = max(0, Int(unlockAt.timeIntervalSince(Date())))
        viewModel?.chestRemainingSeconds = remaining
    }
}
