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
    ///
    /// **同步初始化契约**（review r2 P2 修订）：subscribe 之前**先同步**用 `appState.currentChest`
    /// 当前快照跑一次 `handleChestChange`，让 `viewModel.chestRemainingSeconds` 在 start() 返回前
    /// 就拿到正确的初值。否则 Combine sink 的 `.receive(on: .main)` 派发到下一 runloop 才跑，
    /// 中间一帧 ChestCardView 会读到 `@Published Int = 0` 默认值。结合 ChestCardView 的
    /// status-aware 视觉派生（`.counting` 且 `remainingSeconds <= 0` 也算 unlockable，让本地 tick
    /// 到 0 时能切视觉态），如果 driver 不同步初始化，hydrate 时序就会让 `.counting` 宝箱闪一帧
    /// unlockable 金色卡片。详 `docs/lessons/2026-05-15-driver-sync-init-on-sink-21-1-r2.md`.
    public func start() {
        guard subscription == nil else { return }  // 防双启
        // 同步初始化（review r2）：sink 前先用当前 currentChest 跑一次，让 chestRemainingSeconds
        // 在 start() 返回前就拿到正确初值。CurrentValueSubject 风格的"立即拿当前值"语义.
        handleChestChange(appState?.currentChest)
        subscription = appState?.$currentChest
            .dropFirst()  // 上面已同步处理一次，订阅时跳过 Published 首次发送（避免重复 cancel/restart tick task）
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
