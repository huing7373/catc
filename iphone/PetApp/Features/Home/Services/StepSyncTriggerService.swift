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
//   - currentSyncTask Task 引用追踪：当前 sync in-flight 时新触发被忽略（不排队）
//   - 失败不阻塞 UI（背景同步；下次定时器到达再试）
//
// triggerManual 等待语义（review round 3 [P2] fix → round 4 [P2] race tighten）:
//   - 与 launch / foreground / timer 不同：caller（Story 21.x ChestOpenUseCase 节点 7）
//     需要 await 拿到 fresh `currentStepAccount` 才能继续开箱
//   - 实装：triggerManual 用 while-loop 链式等待 in-flight + @MainActor 同步段原子 assign 自身 task
//     —— 然后 await 自己的 task 完成；保证返回时 sync 一定刚跑完
//   - 不能简单复用 fire-and-forget 路径——那会被 in-flight gate 短路 return,
//     caller 拿到 stale state（review round 3 命中的 bug）.
//   - round 4 race 修法：单次 `await currentSyncTask?.value` resume 后**不能假设**
//     currentSyncTask == nil —— main actor 让出期间，automatic trigger（launch/foreground/timer）
//     可能又 spawn 了新 task 并存进 currentSyncTask. 所以必须 while-loop re-check：
//     每次 await 完后看 currentSyncTask 是不是又被填了，是就继续等. 直到看到 nil 才进入下一步.
//     而"nil check + 创建并 assign 自己 task"是 @MainActor 的同步段（无 await） → 原子.
//   - 详见 docs/lessons/2026-05-04-manual-trigger-must-await-in-flight.md
//     + docs/lessons/2026-05-04-await-then-recheck-single-flight-gate.md
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
    /// 当前 sync 进行中时新触发被忽略（不排队）；非 nil 表示 in-flight.
    /// review round 3 [P2] fix：从 Bool flag 升级为 Task 引用，让 triggerManual
    /// 能 await 到 in-flight sync 完成（而不是被 gate 短路 return 给 stale state）.
    private var currentSyncTask: Task<Void, Never>?

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
        spawnSyncIfIdle(reason: wasFirstStart ? .launch : .foreground)
    }

    /// 回前台触发（epics.md AC 行 1569）.
    /// 由 RootView .onChange(of: scenePhase) `.active` 边沿调.
    public func triggerForeground() {
        spawnSyncIfIdle(reason: .foreground)
    }

    /// 手动触发（epics.md AC 行 1571；节点 7 ChestOpenUseCase 调用入口）.
    /// 等待同步完成 —— caller（Story 21.x ChestOpenUseCase）需要在同步完成后再继续开箱.
    ///
    /// review round 3 [P2] fix → round 4 [P2] race tighten：
    /// 不能复用 spawnSyncIfIdle / performSync 的 fire-and-forget 路径——那会被
    /// in-flight gate 短路 return，caller 拿到 stale `currentStepAccount`，破坏
    /// "同步完成后再继续开箱"契约.
    ///
    /// 改为 while-loop 链式等待 + @MainActor 同步段原子 assign：
    ///   1. 若 currentSyncTask 非 nil → await 它的 value；resume 后**重新 re-check** —— main actor
    ///      让出期间 automatic trigger 可能又 spawn 了新 task 并存进 currentSyncTask，必须继续等.
    ///   2. 直到 currentSyncTask == nil（@MainActor 同步段：从 nil check 到 assign 之间不会让出）
    ///      → 立即创建自己的 task 并 assign currentSyncTask（原子）
    ///   3. await 自己 task.value，保证返回时 sync 一定刚跑完
    ///
    /// 关键不变量：
    ///   - while-loop 退出条件是 `currentSyncTask == nil`，不是 "前一个 inflight 完成"
    ///     （前一个完成不代表没有别的 automatic trigger 又起了新 task）
    ///   - "re-check + create + assign" 三步是 @MainActor 同步段 → 不会被 race 干扰
    ///   - automatic 路径 spawnSyncIfIdle 的 `if currentSyncTask != nil { return }` 短路保证：
    ///     一旦 manual assign 成功，automatic 不会覆盖；一旦 manual 看到 nil，automatic 也看到 nil
    ///     时是 race，但此时 manual 已经在同一个 @MainActor 同步段里完成了 assign，automatic 在
    ///     manual yield 之前不会执行 —— 所以 manual 总是先抢到.
    ///
    /// 详见 docs/lessons/2026-05-04-manual-trigger-must-await-in-flight.md
    /// + docs/lessons/2026-05-04-await-then-recheck-single-flight-gate.md.
    public func triggerManual() async {
        // 链式等待：每次 await 完后 re-check currentSyncTask；只要还非 nil，继续等.
        // round 4 修法关键：单次 `await inflight?.value` resume 后**不假设** nil ——
        // main actor 让出期间 automatic 路径可能又填了一个新 task.
        while let inflight = currentSyncTask {
            await inflight.value
            // resume 时 currentSyncTask 可能：
            //   a. 仍指向 inflight（runSync defer 在该 task @MainActor 里清，已清 nil；
            //      此 case 里 currentSyncTask == nil → loop 退出）
            //   b. 已被 automatic trigger 覆盖为新的 in-flight task（loop 继续等新 task）
            //   c. nil（loop 退出）
        }

        // 此时 currentSyncTask == nil，且我们处在 @MainActor 同步段（while-loop 退出 → 下面创建并
        // assign 之间没有 await） → 不会被任何 race 打断.
        // automatic 路径 spawnSyncIfIdle 也是 @MainActor，但只有在我们 yield 之后才能跑；它会看到
        // currentSyncTask 已被 manual 占位 → 短路 return.
        let task: Task<Void, Never> = Task { @MainActor [weak self] in
            guard let self else { return }
            await self.runSync(reason: .manual)
        }
        currentSyncTask = task
        await task.value
        // 注意：currentSyncTask 在 runSync 的 defer 中已被清 nil；这里不再覆写避免与 fire-and-forget 路径竞态.
    }

    /// 停止触发器：cancel 定时器循环.
    /// 由 RootView .onChange(of: scenePhase) `.background` 边沿调（节省电量；下次 .active 重新 start）.
    /// **不**清 currentSyncTask：让正在 in-flight 的 sync 自然完成；scenePhase .active 时再触发新 sync 时
    /// currentSyncTask 自然 nil（前一个已完成）.
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

    /// fire-and-forget 路径：launch / foreground / timer 用.
    /// 若有 in-flight sync 直接忽略（epics.md AC 行 1577 + 1583 钦定"重叠忽略"）.
    /// 把 spawn 的 Task 存到 currentSyncTask，让 triggerManual 能 await 到.
    private func spawnSyncIfIdle(reason: SyncReason) {
        guard currentSyncTask == nil else {
            // in-flight 时新触发被忽略（不排队）.
            return
        }
        // 注：closure 内显式 unwrap 让 Task<Void, Never> 类型推断成功.
        let task: Task<Void, Never> = Task { @MainActor [weak self] in
            guard let self else { return }
            await self.runSync(reason: reason)
        }
        currentSyncTask = task
    }

    /// 同步 + 错误吞咽 + currentSyncTask 自清；fire-and-forget 与 manual 共用.
    /// 不再做 in-flight gate（caller 自己保证 idle 才进入）；本方法是"已 commit 跑一次同步"的语义.
    private func runSync(reason: SyncReason) async {
        defer { currentSyncTask = nil }

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
        // Task 显式 @MainActor，让循环体直接调本 service 的 @MainActor 方法不需要 actor hop await
        // （否则调 sync 方法 spawnSyncIfIdle 编译器会报 "no async operations within await" warning）.
        timerTask = Task { @MainActor [weak self] in
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
                // timer 走 fire-and-forget 路径（与 launch / foreground 同语义）；
                // in-flight 时被 gate 短路 return（重叠忽略）.
                self?.spawnSyncIfIdle(reason: .timer)
            }
        }
    }

    deinit {
        timerTask?.cancel()
        // option A：service 不持 motionProvider；无需 stopUpdates.
        // 即便 deinit nonisolated 也不会触碰 @MainActor isolated 字段（timerTask 是 nonisolated cancel 安全）.
    }
}
