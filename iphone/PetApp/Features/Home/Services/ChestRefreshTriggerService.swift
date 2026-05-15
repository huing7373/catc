// ChestRefreshTriggerService.swift
// Story 21.2 AC4: 宝箱状态拉取触发器服务（3 触发器 + in-flight gate + 60s 定时器）.
//
// 3 触发时机（epics.md AC 行 3049）:
//   1. App 启动后进入主界面（RootView .onReadyTask 调 service.start() 触发首次）
//   2. App 从后台回到前台（RootView .onChange(of: scenePhase) .active 触发）
//   3. 主界面停留期间每 60 秒定时拉取一次（service.start() 内启动 Task.sleep 循环）
//
// in-flight gate（与 StepSyncTriggerService 同模式）:
//   - currentRefreshTask Task 引用追踪：当前 refresh in-flight 时新触发被忽略（不排队）
//   - 失败不破坏 UI（背景拉取；下次定时器到达或 foreground 再试；与 SyncStepsUseCase 同精神）
//
// 与 StepSyncTriggerService 的差异（简化）:
//   - **不**实装 triggerManual（本 story 无 manual await 场景；Story 21.3 OpenChestUseCase 直接 await
//     POST /chest/open 响应，**不**经本 service 主动 refresh —— server 端响应已含 nextChest）
//   - **不**借用 HomeViewModel（chest refresh 无需 motionState；service 完全独立于 ViewModel）
//   - **不**接 reconnect alignment delegate（chest 不走 WS 推送，仅 REST 60s 拉取兜底）
//
// 生命周期（与 StepSyncTriggerService 100% 镜像）:
//   - 由 RootView 通过 @State 持有；与 RootView 同生命周期
//   - start() 由 RootView .onReadyTask 内调（启动 + 定时器循环）
//   - stop() 由 RootView .onChange(of: scenePhase) .background 边沿调
//   - deinit 时 cancel timer 防泄漏
//
// 性能 / 资源约束:
//   - Timer 周期 60 秒（epics.md AC 行 3049 钦定）—— 不可配置（YAGNI；prod 默认值锚定 epic AC）
//   - Timer 用 `Task.sleep(nanoseconds:)` 循环，不用 Foundation `Timer`
//     （@MainActor 友好 + 可被 cancel；与 Swift 6 strict concurrency 一致；与 StepSyncTriggerService 同选型）

import Foundation

@MainActor
public final class ChestRefreshTriggerService {

    // MARK: - Dependencies

    private let loadChestUseCase: LoadChestUseCaseProtocol

    // MARK: - State

    /// in-flight gate.
    /// 当前 refresh 进行中时新触发被忽略（不排队）；非 nil 表示 in-flight.
    private var currentRefreshTask: Task<Void, Never>?

    /// 定时器循环 task；start() 启动；stop() / deinit 取消.
    private var timerTask: Task<Void, Never>?

    /// 是否已启动定时循环（防 .scenePhase .active 多次触发重复启动 timer；与 StepSyncTriggerService 同模式）.
    private var hasStartedTimer = false

    /// Timer 周期：60 秒（epics.md AC 行 3049）.
    private static let timerIntervalNanos: UInt64 = 60 * 1_000_000_000

    // MARK: - Init

    public init(loadChestUseCase: LoadChestUseCaseProtocol) {
        self.loadChestUseCase = loadChestUseCase
    }

    // MARK: - Public API

    /// 启动触发器：启动 60 秒定时循环 + 触发首次拉取.
    /// 由 RootView .onReadyTask 在主界面就绪后调；幂等（多次调安全）.
    ///
    /// 与 StepSyncTriggerService.start() 同精神（codex review round 1 [P3] fix 锁定路径）：
    ///   - 首次调用：startTimerIfNeeded() 启动 timer + spawn 一次 launch refresh；
    ///   - 已 hasStartedTimer 的后续调用：等同 foreground reactivate refresh,
    ///     **不**重启 timer，避免老 timer 还在跑就启动新的.
    /// 详见 docs/lessons/2026-05-04-scenephase-idempotent-start-no-duplicate-trigger.md.
    public func start() {
        let wasFirstStart = !hasStartedTimer
        startTimerIfNeeded()
        spawnRefreshIfIdle(reason: wasFirstStart ? .launch : .foreground)
    }

    /// 停止触发器：cancel 定时器循环.
    /// 由 RootView .onChange(of: scenePhase) .background 边沿调.
    /// **不**清 currentRefreshTask：让正在 in-flight 的 refresh 自然完成；下次 start() 时
    /// currentRefreshTask 自然 nil（前一个已完成的 defer 会清）.
    public func stop() {
        timerTask?.cancel()
        timerTask = nil
        hasStartedTimer = false
    }

    // MARK: - Private

    private enum RefreshReason: String {
        case launch
        case foreground
        case timer
    }

    /// fire-and-forget 路径：launch / foreground / timer 用.
    /// 若有 in-flight refresh 直接忽略（与 StepSyncTriggerService 同模式）.
    private func spawnRefreshIfIdle(reason: RefreshReason) {
        guard currentRefreshTask == nil else {
            return
        }
        let task: Task<Void, Never> = Task { @MainActor [weak self] in
            guard let self else { return }
            await self.runRefresh(reason: reason)
        }
        currentRefreshTask = task
    }

    /// 拉取 + 错误吞咽 + currentRefreshTask 自清.
    private func runRefresh(reason: RefreshReason) async {
        defer { currentRefreshTask = nil }

        do {
            try await loadChestUseCase.execute()
        } catch {
            // 失败不破坏 UI（epics.md AC 行 3053）；下次触发再试.
            // 节点 7 阶段不做 logger framework；失败被 silently 吞掉是 by design.
            // future: 接 logger 后此处 log warning（与 StepSyncTriggerService 同精神）.
            _ = reason  // 防 unused 编译警告；reason 作为未来 logger 的语义键
            _ = error
        }
    }

    private func startTimerIfNeeded() {
        guard !hasStartedTimer else { return }
        hasStartedTimer = true
        timerTask = Task { @MainActor [weak self] in
            while !Task.isCancelled {
                do {
                    try await Task.sleep(nanoseconds: ChestRefreshTriggerService.timerIntervalNanos)
                } catch {
                    // CancellationError → 退出循环；其它 error 不应发生（sleep 仅抛 CancellationError）.
                    return
                }
                guard !Task.isCancelled else { return }
                // timer 走 fire-and-forget 路径（与 launch / foreground 同语义）；
                // in-flight 时被 gate 短路 return（重叠忽略）.
                self?.spawnRefreshIfIdle(reason: .timer)
            }
        }
    }

    deinit {
        timerTask?.cancel()
    }
}
