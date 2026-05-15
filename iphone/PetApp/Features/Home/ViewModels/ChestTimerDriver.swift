// ChestTimerDriver.swift
// Story 21.1 AC2: 本地宝箱倒计时驱动（订阅 AppState.currentChest → 每秒写 HomeViewModel.chestRemainingSeconds）.
//
// 设计：
//   - 弱引用 HomeViewModel（防循环；与 Story 8.4 MotionStateMapper 同模式）.
//   - 弱引用 AppState（防循环；ADR-0010 §3.1 ViewModel 注入 AppState 是 strong，driver 是 weak）.
//   - 订阅 appState.$currentChest（Combine sink）→ 每次变化 cancel 老 Task + 启动新 Task.
//   - 倒计时 Task 内 @MainActor `while !Task.isCancelled` 循环每秒 sleep 1s + 计算 remainingSeconds 写回.
//   - **倒计时来源（review r4 P2 修订）**: server-anchored —— hydrate 时捕获 `(hydratedAt, anchorRemaining)`,
//     之后 displayed = max(0, anchorRemaining - elapsed since hydratedAt). **不**用 `unlockAt - Date()`,
//     避免 device clock skew（如设备时钟快 5 分钟）让 server-counting 宝箱误显示 unlockable.
//     详 `docs/lessons/2026-05-15-driver-server-anchored-time-21-1-r4.md`.
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

    /// 注入式 clock provider —— prod 用默认 `{ Date() }`，测试注入 mock clock 函数.
    /// 设计意图（review r4 P2）：让 server-anchored countdown 的"自 hydrate 起经过多少秒"可被
    /// fake clock 推进，无需真等系统时间流逝.
    private let clock: () -> Date

    /// **anchor 状态**（review r4 P2）：捕获 hydrate 时刻的 `(hydratedAt, anchorRemaining)`,
    /// 之后所有 tick 都从这两个值派生 displayed —— 而非用 `unlockAt - Date()`.
    /// - `hydratedAt`: 当前 `chest` 进入 driver 时的本地时钟快照（**device 时钟**，不是 server 时钟）.
    /// - `anchorRemaining`: 当前 `chest.remainingSeconds`（server 算好的真值）.
    /// - `anchoredChestId`: 配套的 chest id，chest 切换 → 重新 anchor.
    /// 计算：`displayed = max(0, anchorRemaining - Int(now - hydratedAt))` —— 两端都是同 device 同 clock，
    /// 差值不受 wall-clock skew 影响.
    private var hydratedAt: Date?
    private var anchorRemaining: Int?
    private var anchoredChestId: String?

    public init(
        appState: AppState,
        viewModel: HomeViewModel,
        clock: @escaping () -> Date = { Date() }
    ) {
        self.appState = appState
        self.viewModel = viewModel
        self.clock = clock
    }

    /// 启动 driver：订阅 appState.$currentChest，首次启动立即用当前值跑一次 recompute.
    ///
    /// **同步初始化契约**（review r2 P2 修订）：subscribe 之前**先同步**用 `appState.currentChest`
    /// 当前快照跑一次 `handleChestChange`，让 `viewModel.chestRemainingSeconds` 在 start() 返回前
    /// 就拿到正确的初值。否则 Combine sink 的下次派发才跑，中间一帧 ChestCardView 会读到
    /// `@Published Int = 0` 默认值。
    ///
    /// **同步 sink 契约**（review r3 P2 修订）：**不**走 `.receive(on: DispatchQueue.main)`.
    /// `@Published` 的 publisher 在 setter 所在线程同步发出 —— driver 整类已 `@MainActor` 标注 +
    /// AppState 整类已 `@MainActor` 标注 + `currentChest` 所有写入路径（applyHomeData / reset /
    /// HomeViewModel.refresh / Story 21.2 LoadChestUseCase）都在 main actor，
    /// **不需要**额外 dispatch hop. 删 `.receive(on:)` 让 sink closure 在 `currentChest` setter
    /// 调用栈内同步执行，subsequent hydration（如 /home 刷新装入新 `.counting` 宝箱）时
    /// `chestRemainingSeconds` 的写入 **happens-before** SwiftUI 观察 AppState 触发的 rerender,
    /// ChestCardView 第一次见到新 `currentChest` 时配套的 `chestRemainingSeconds` 已经是正值,
    /// 不会闪一帧 `(.counting, 0) → unlockable` 金色卡片.
    ///
    /// 同步性测试反弹守门：`testChestTimerDriverPropagatesSubsequentChestChangeSynchronously`,
    /// 任何未来 PR 重新加 `.receive(on:)` 或异步 hop 都会立刻挂.
    /// 详 `docs/lessons/2026-05-15-driver-sync-sink-on-subsequent-change-21-1-r3.md`.
    public func start() {
        guard subscription == nil else { return }  // 防双启
        // 同步初始化（review r2）：sink 前先用当前 currentChest 跑一次，让 chestRemainingSeconds
        // 在 start() 返回前就拿到正确初值。CurrentValueSubject 风格的"立即拿当前值"语义.
        handleChestChange(appState?.currentChest)
        subscription = appState?.$currentChest
            .dropFirst()  // 上面已同步处理一次，订阅时跳过 Published 首次发送（避免重复 cancel/restart tick task）
            // **禁止**在此加 `.receive(on:)` 或任何异步 operator —— 见 start() 文档 "同步 sink 契约".
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
            // currentChest 为 nil（未 hydrate 或被清空）→ remainingSeconds = 0 + 清 anchor + 不启 Task.
            hydratedAt = nil
            anchorRemaining = nil
            anchoredChestId = nil
            viewModel?.chestRemainingSeconds = 0
            return
        }

        // **server-anchored hydrate**（review r4 P2）: chest id 变 → 重新捕获 anchor.
        // 同 id 的 chest 重新进 sink（罕见 —— Story 21.2 60s 拉取若返同 id 会触发）→ 仍重新 anchor,
        // 因为 server 重发说明 remainingSeconds 是新算的、应当采信.
        hydratedAt = clock()
        anchorRemaining = chest.remainingSeconds
        anchoredChestId = chest.id

        // 首次/重启：立即算一次 + 启 Task 每秒 tick.
        recomputeAndWrite()
        let chestId = chest.id
        tickTask = Task { @MainActor [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(nanoseconds: 1_000_000_000)
                guard !Task.isCancelled else { return }
                guard let self else { return }
                // 防 ABA：tickTask 启动后 currentChest 被换掉，sink 已 cancel 老 task；这里再 guard 一次 chest 仍是同一个.
                guard self.appState?.currentChest?.id == chestId else { return }
                self.recomputeAndWrite()
            }
        }
    }

    /// 从 anchor 派生 displayed remainingSeconds（review r4 P2 server-anchored 计算）.
    ///
    /// 公式：`displayed = max(0, anchorRemaining - Int(now - hydratedAt))`
    ///
    /// 关键性质：
    /// - **抗 wall-clock skew**：`now` 和 `hydratedAt` 都是同 device 同 clock 的快照，
    ///   两者差值是真实的"自 hydrate 起经过的秒数"，与 server 时钟、device 时钟的绝对偏差无关.
    /// - **抗 background/foreground**：app 进后台 X 秒后回前台调 tick → `now - hydratedAt` 自动跳进 X 秒,
    ///   displayed 追上正确 remaining，无需额外恢复逻辑.
    /// - **资源 nil 兜底**：anchor 未设置（罕见 race）→ 写 0 防止 stale 显示.
    private func recomputeAndWrite() {
        guard let hydratedAt, let anchorRemaining else {
            viewModel?.chestRemainingSeconds = 0
            return
        }
        let elapsed = Int(clock().timeIntervalSince(hydratedAt))
        let remaining = max(0, anchorRemaining - elapsed)
        viewModel?.chestRemainingSeconds = remaining
    }
}
