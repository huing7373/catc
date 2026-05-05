// StepSyncTriggerService.swift
// Story 8.5 AC5 / AC9 (option A): 步数同步触发器服务（4 触发器 + in-flight gate + 借用 HomeViewModel.petState）.
//
// 4 触发时机（epics.md AC 行 1567-1571）:
//   1. App 启动后进入主界面（RootView .task / .onReadyTask 调 service.start() 触发首次）
//   2. App 从后台回到前台（RootView .onChange(of: scenePhase) 监听 .active 触发）
//   3. 主界面停留期间每 5 分钟定时同步一次（service.start() 内启动 Task.sleep 循环）
//   4. 手动触发接口（service.triggerManual() 公开入口；Story 21.x ChestOpenUseCase 节点 7 用）
//
// in-flight gate（epics.md AC 行 1577 钦定 "同步不重叠"）:
//   - isSyncing flag 防重叠：当前 sync in-flight 时新触发被忽略（不排队）
//   - 失败不阻塞 UI（背景同步；下次定时器到达再试）
//
// option A 锁定（AC9 边界澄清段）:
//   - service 注入 `HomeViewModel`（**不**接 motionProvider）；
//   - performSync 时读 `homeViewModel.petState` 拼请求 motionState；
//   - 8.4 视觉层负责 motionProvider 唯一订阅；本 service 不再自己 startUpdates，
//     避免与 HomeViewModel.bind(motionProvider:) 冲突（startUpdates 单订阅契约：后调用者覆盖前者）.
//
// 生命周期:
//   - 由 RootView 通过 @State 持有；与 RootView 同生命周期
//   - start() 由 RootView .onReadyTask 内调（启动 + 定时器循环）
//   - stop() 由 RootView .onChange(of: scenePhase) `.background` 边沿调
//   - deinit 时 cancel timer 防泄漏
//
// 性能 / 资源约束:
//   - Timer 周期 5 分钟（300 秒）—— 不可配置（YAGNI；prod 默认值锚定 epics.md AC 钦定）
//   - Timer 用 `Task.sleep(nanoseconds:)` 循环，不用 Foundation `Timer`
//     （@MainActor 友好 + 可被 cancel；与 Swift 6 strict concurrency 一致）

import Foundation

@MainActor
public final class StepSyncTriggerService {

    // MARK: - Dependencies

    private let syncStepsUseCase: SyncStepsUseCaseProtocol
    /// option A：service 借用 HomeViewModel.petState 作为 motionState 来源（不订阅 motionProvider）.
    /// weak 引用避免循环：HomeViewModel 由 RootView @StateObject 持有，service 由 RootView @State 持有，
    /// 都活在 RootView 生命周期内 → 反向不持 service，没有循环 retain；但 weak 让 HomeViewModel 释放时
    /// service 自动 nil 化是良好习惯（与 HomeViewModel.appState weak 同精神）.
    private weak var homeViewModel: HomeViewModel?

    // MARK: - State

    /// in-flight gate（epics.md AC 行 1577）.
    /// 当前 sync 进行中时新触发被忽略（不排队）.
    private var isSyncing = false

    /// 定时器循环 task；start() 启动；stop() / deinit 取消.
    private var timerTask: Task<Void, Never>?

    /// 是否已启动定时循环（防 .scenePhase .active 多次触发重复启动 timer）.
    private var hasStartedTimer = false

    /// Timer 周期：5 分钟（epics.md AC 行 1570）.
    private static let timerIntervalNanos: UInt64 = 5 * 60 * 1_000_000_000

    // MARK: - Init

    public init(
        syncStepsUseCase: SyncStepsUseCaseProtocol,
        homeViewModel: HomeViewModel
    ) {
        self.syncStepsUseCase = syncStepsUseCase
        self.homeViewModel = homeViewModel
    }

    // MARK: - Public API

    /// 启动触发器：启动 5 分钟定时循环 + 触发首次同步（epics.md AC 行 1568）.
    /// 由 RootView .onReadyTask 在主界面就绪后调；幂等（多次调安全）.
    ///
    /// codex review round 1 [P3] fix：让 RootView `.scenePhase .active` 路径只调本方法（不再
    /// 同时调 `triggerForeground()`），避免每次回前台 enqueue 两个独立 Task → 第一个完成后
    /// in-flight gate 已 release → 第二个就会真的发出 duplicate `/steps/sync` 请求.
    /// 现在 start() 自己充当"幂等 reactivate 入口"：
    ///   - 首次调用：startTimerIfNeeded() 启动 timer + spawn 一次 launch sync;
    ///   - 已 hasStartedTimer 的后续调用：等同 triggerForeground()（只 spawn 一次 reactivate sync,
    ///     **不**重启 timer，避免老 timer 还在跑就启动新的）.
    /// 详见 docs/lessons/2026-05-04-scenephase-idempotent-start-no-duplicate-trigger.md.
    public func start() {
        let wasFirstStart = !hasStartedTimer
        startTimerIfNeeded()
        Task { [weak self] in
            await self?.performSync(reason: wasFirstStart ? .launch : .foreground)
        }
    }

    /// 回前台触发（epics.md AC 行 1569）.
    /// 由 RootView .onChange(of: scenePhase) `.active` 边沿调.
    public func triggerForeground() {
        Task { [weak self] in
            await self?.performSync(reason: .foreground)
        }
    }

    /// 手动触发（epics.md AC 行 1571；节点 7 ChestOpenUseCase 调用入口）.
    /// 等待同步完成（与 launch / foreground / timer 不等待不同 —— caller 需要在同步完成后再继续开箱）.
    public func triggerManual() async {
        await performSync(reason: .manual)
    }

    /// 停止触发器：cancel 定时器循环.
    /// 由 RootView .onChange(of: scenePhase) `.background` 边沿调（节省电量；下次 .active 重新 start）.
    /// **不**清 isSyncing：让正在 in-flight 的 sync 自然完成；scenePhase .active 时再触发新 sync 时
    /// isSyncing 自然 false（前一个已完成）.
    public func stop() {
        timerTask?.cancel()
        timerTask = nil
        hasStartedTimer = false
        // option A：service 不持 motionProvider，无需调 stopUpdates.
        // 8.4 ViewModel 的 motion 订阅由 ViewModel 自管，不受本 service stop 影响.
    }

    // MARK: - Private

    private enum SyncReason: String {
        case launch
        case foreground
        case timer
        case manual
    }

    private func performSync(reason: SyncReason) async {
        guard !isSyncing else {
            // 重叠忽略（epics.md AC 行 1577 + 1583）.
            return
        }
        isSyncing = true
        defer { isSyncing = false }

        // option A：从 HomeViewModel 读 petState 作为 motionState（默认 .rest，与 8.4 同精神）.
        let motionState = homeViewModel?.petState ?? .rest

        do {
            try await syncStepsUseCase.execute(motionState: motionState)
        } catch {
            // 失败不阻塞 UI（epics.md AC 行 1576）；下次触发再试.
            // 节点 3 阶段不做 logger framework；失败被 silently 吞掉是 by design.
            // future: 接 logger 后此处 log warning（与 server 7.3 防作弊 log warning 同精神）.
            _ = reason  // 防 unused 编译警告；reason 仅作为未来 logger 的语义键
            _ = error
        }
    }

    private func startTimerIfNeeded() {
        guard !hasStartedTimer else { return }
        hasStartedTimer = true
        timerTask = Task { [weak self] in
            // 定时循环：每 5 分钟一次（epics.md AC 行 1570）.
            // 用 Task.sleep + while !Task.isCancelled，不用 Foundation Timer
            // （Timer 跨 actor / cancel 复杂；Task 模型在 Swift 6 strict concurrency 下更干净）.
            while !Task.isCancelled {
                do {
                    try await Task.sleep(nanoseconds: StepSyncTriggerService.timerIntervalNanos)
                } catch {
                    // CancellationError → 退出循环；其它 error 不应发生（sleep 仅抛 CancellationError）.
                    return
                }
                guard !Task.isCancelled else { return }
                await self?.performSync(reason: .timer)
            }
        }
    }

    deinit {
        timerTask?.cancel()
        // option A：service 不持 motionProvider；无需 stopUpdates.
        // 即便 deinit nonisolated 也不会触碰 @MainActor isolated 字段（timerTask 是 nonisolated cancel 安全）.
    }
}
