// PetStateSyncTriggerService.swift
// Story 15.4 AC3: 当前 pet 状态同步触发器（reactive subscription + 5s 节流 + roomId preflight + fire-and-forget）.
//
// 与 sibling StepSyncTriggerService 同精神（生命周期 / 注入路径 / 红线一致），但**简化**：
// - StepSyncTriggerService 是 4 触发器（launch / foreground / timer / manual）+ in-flight gate + Task chain；
// - PetStateSyncTriggerService 是单触发器（subscribe `homeViewModel.$petState`）+ 单二元组节流 + spawn-and-forget.
//
// 触发链路（epics.md §15.4 行 2436-2447）：
//   `homeViewModel.$petState` emit → sink 收到 newState
//     → preflight A: 节流（同 state + 5s 内 → return；不消耗节流窗口）
//     → preflight B: roomId guard（appState.currentRoomId == nil → return；不消耗节流窗口）
//     → commit-to-send: 同步写 lastSentState/lastSentAt 节流锚点
//     → spawn fire-and-forget Task → await UseCase.execute(state:) → 失败 silently 吞.
//
// 实装边界（attempt 2 严守红线 —— attempt 1 跑 13 轮 codex review 未收敛根因）:
//   ❌ per-state `[MotionState: Date]` 节流字典 → 用单一 `(lastSentState, lastSentAt)` 二元组
//   ❌ coalesce-to-latest pending state queue → fire-and-forget；失败丢，下次 mutate 重试
//   ❌ 订阅 `appState.$currentRoomId` 做 room edge 主动 sync → 那是 Story 15.5 的责任
//   ❌ stop() 内回滚 cancelled in-flight Task 的节流锚点 → stop / start cycle 自然处理
//   ❌ 区分 publisher subscribe-replay vs 真实 transition → `.dropFirst()` 一刀切
//   ❌ service 内部 in-flight gate / serialize / Task chain → spawn-and-forget 即可（5s throttle 天然防同 state 并发）
//
// 生命周期（与 StepSyncTriggerService 同模式 —— 相关 lessons：
//   - `2026-05-04-scenephase-idempotent-start-no-duplicate-trigger.md`：start() 用 `subscription == nil` guard 实现幂等
//   - `2026-05-04-launch-state-leave-ready-must-stop-feature-services.md`：onLeaveReady 必须 stop()
//   - `2026-04-26-stateobject-debug-instance-aliasing.md`：用 @State 持有引用，**不**用 @StateObject）：
//   - 由 RootView @State 持有；与 RootView 同生命周期
//   - start() 由 RootView .onReadyTask 内调 + scenePhase .active 时调（幂等：subscription != nil 短路）
//   - stop() 由 RootView onLeaveReady 内调 + scenePhase .background 时调
//   - deinit 时 cancel subscription 防泄漏
//
// fire-and-forget 边界（lesson `2026-05-12-fire-and-forget-boundary-must-include-prerequisite-io-14-4-r1.md`）：
//   sink closure 内的 throttle / roomId preflight 是同步 IO（无 await），在 commit-to-send 之前完成；
//   Task 内**只**跑 UseCase + catch；不能把"决定是否触发的前置 IO"放进 Task await 之后.

import Foundation
import Combine  // lesson `2026-04-25-swift-explicit-import-combine.md`：Combine 必须显式 import 才能用 .dropFirst() / AnyCancellable.

/// Story 15.5 AC3: WS 重连成功后由 RealRoomViewModel 通知 PetStateSyncTriggerService 触发对齐的 delegate 协议.
///
/// 设计：vm 不 import service 类型（避免反向耦合 —— Room feature 不应依赖 Home feature 类型）；
/// 通过本"语义中立"协议接缝传递通知；service 适配 conformance；RootView 在 wire 时把 service
/// 注入为 vm.reconnectAlignmentDelegate（弱引用，避免循环）.
///
/// 调用契约：
///   - vm 在 `.connectionStateChanged(.connected)` 分支（含 first connect / reconnect）调用一次本方法
///   - service 在 `didReconnectAlignmentRequested()` 内调 `triggerManualResync()` 触发 self push
public protocol PetStateReconnectAlignmentDelegate: AnyObject {
    @MainActor
    func didReconnectAlignmentRequested()
}

@MainActor
public final class PetStateSyncTriggerService {

    // MARK: - Dependencies

    private let syncPetStateUseCase: SyncPetStateUseCaseProtocol
    /// weak 引用避免循环：HomeViewModel / AppState 由 RootView 持有（@StateObject / @ObservedObject），
    /// service 由 RootView @State 持有；都活在 RootView 生命周期内 → 反向不持 service.
    /// weak 让 HomeViewModel / AppState 释放时 service 自动 nil 化是良好习惯（与 sibling 同精神）.
    private weak var homeViewModel: HomeViewModel?
    private weak var appState: AppState?

    // MARK: - Throttle State（单一二元组锚点 —— **不**用字典；attempt 1 r9 红线）

    /// 上一次发出 state-sync 的 state（commit-to-send 时同步写）.
    private var lastSentState: MotionState?
    /// 上一次发出 state-sync 的时刻（commit-to-send 时同步写）.
    private var lastSentAt: Date?
    /// 5 秒节流窗口（epics.md §15.4 行 2440 钦定 "同一 state 在 5 秒内不重复上报"）.
    private static let throttleWindow: TimeInterval = 5.0

    /// Internal test seam（仅 internal access；protocol 不暴露）：让 time-related test 注入 fake date 而非真 sleep.
    /// 默认 `{ Date() }`；测试可用 `service.nowProvider = { fixedDate }` 覆盖.
    /// **禁** Task.sleep(5_000_000_000) 真等 5 秒（attempt 1 测试反模式）.
    internal var nowProvider: () -> Date = { Date() }

    // MARK: - Subscription

    /// Combine subscription 引用；start() 建，stop() / deinit 清.
    private var subscription: AnyCancellable?

    // MARK: - Init

    public init(
        syncPetStateUseCase: SyncPetStateUseCaseProtocol,
        homeViewModel: HomeViewModel,
        appState: AppState
    ) {
        self.syncPetStateUseCase = syncPetStateUseCase
        self.homeViewModel = homeViewModel
        self.appState = appState
    }

    // MARK: - Public API

    /// 启动 subscription（幂等：subscription != nil 时短路 —— 与 sibling StepSyncTriggerService.start
    /// 同精神：first start / resume 都走同一路径 / 多次调用安全）.
    ///
    /// `.dropFirst()`：抹掉 @Published 订阅瞬间的 currentValue replay（避免 first-start 时把当前 .rest
    /// 误当作 transition 触发一次无谓的 state-sync）；resume 同理 —— 用户切到 background 又回来，
    /// 期间 petState 没真实 mutate，重新 subscribe 不应触发. 一刀切 dropFirst 比"区分订阅瞬间 vs 真实
    /// transition" 简单且鲁棒（attempt 1 r3 / r4 / r13 反复修该边界，本 story 用 dropFirst 一次解决）.
    public func start() {
        guard subscription == nil, let homeViewModel else { return }
        subscription = homeViewModel.$petState
            .dropFirst()
            .sink { [weak self] newState in
                // sink 在 main actor 上执行（@Published 来源也在 @MainActor HomeViewModel），
                // self 是 @MainActor → 闭包内同步调 handlePetStateChange 不需 await.
                self?.handlePetStateChange(newState)
            }
    }

    /// 停止 subscription（cancel 当前；**不**动 lastSentState/lastSentAt 节流锚点 —— 让 resume 后
    /// 同 state 5s 内仍受节流：用户切到 background 又回来，state 没变就不必重发；attempt 1 r13 红线.
    /// 也**不**回滚已 spawn 的 in-flight Task —— 让它自然完成 / 失败 silently 吞；不引 task cancel chain）.
    public func stop() {
        subscription?.cancel()
        subscription = nil
    }

    /// Story 15.5 AC2: 重连成功后由 RealRoomViewModel 通过 PetStateReconnectAlignmentDelegate 触发的
    /// "manual resync" 入口 —— **绕过 5s 同 state 节流**，让 reconnect 边界的状态对齐不被节流锚点错挡.
    ///
    /// 语义：
    ///   1. reset 节流锚点：`lastSentState = nil; lastSentAt = nil` —— 让本次 sync 必发（不论上次 sync 时间）
    ///   2. 调 handlePetStateChange(homeViewModel.petState) —— 走完整 throttle (now no-op) / roomId preflight /
    ///      commit-to-send / fire-and-forget 流水线 —— 与 reactive subscription 路径同流水线，确保 roomId guard
    ///      与 throttle "重新建立锚点" 等行为一致.
    ///
    /// **idempotent / safe**: 当 user 不在房间（appState.currentRoomId == nil）时，roomId preflight 自然短路 ——
    /// 调用本方法是无害的；但 caller 应仅在合适时机调（ws connected + 在房间），避免无谓 syscall.
    ///
    /// **fire-and-forget**: 与 reactive 路径同语义；本方法**同步**返回，UseCase Task 在背景 spawn；失败 silently 吞.
    public func triggerManualResync() {
        guard let homeViewModel else { return }  // service 已 deinit / wire 残缺 → no-op
        // Step 1: reset 节流锚点 —— 让 handlePetStateChange 内 throttle preflight 必通过.
        // **不**重置 nowProvider —— 那是 test seam，与节流逻辑解耦.
        // **不**调 stop() —— subscription 不动，避免 reactive 路径被打断.
        lastSentState = nil
        lastSentAt = nil
        // Step 2: 走完整流水线（throttle 已 reset → 必通过；roomId guard / commit / fire-and-forget 走原路径）.
        handlePetStateChange(homeViewModel.petState)
    }

    // MARK: - Private

    /// sink 回调入口：preflight throttle + roomId guard → commit-to-send → fire-and-forget spawn UseCase Task.
    ///
    /// 4 步顺序（每步语义 + 红线）：
    ///   1. throttle preflight：节流命中（同 state + 5s 内）→ return（**不**消耗节流窗口）
    ///   2. roomId preflight：appState.currentRoomId == nil → return（**不**消耗节流窗口；
    ///      attempt 1 r1 / r2 P2 fix 保留：让用户回到房间后第一次合法 sync 不被错挡）
    ///   3. commit-to-send：同步写 lastSentState/lastSentAt 锚点（attempt 1 r2 P1 fix 保留：防 await
    ///      期间同 state 再 emit 看到 nil 锚点重复 spawn）
    ///   4. fire-and-forget Task：spawn 单个 Task → await UseCase.execute → 失败 silently 吞
    ///      （与 sibling StepSyncTriggerService.runSync catch 同模式；节点 5 阶段不接 logger framework）
    ///
    /// fire-and-forget 边界（lesson `2026-05-12-fire-and-forget-boundary-must-include-prerequisite-io-14-4-r1.md`）:
    /// preflight + commit-to-send 都是同步 IO，必须在 spawn Task 之前完成；Task 内只跑 UseCase + catch.
    private func handlePetStateChange(_ newState: MotionState) {
        // Step 1: throttle preflight —— 同 state + 5s 内命中 → 跳过.
        let now = nowProvider()
        if let lastState = lastSentState, let lastAt = lastSentAt,
           lastState == newState, now.timeIntervalSince(lastAt) < Self.throttleWindow {
            return
        }

        // Step 2: roomId preflight —— 不在房间 → 跳过（不消耗节流窗口）.
        // 同步读 @MainActor isolated currentRoomId（service 也 @MainActor → 直接读不需 await）.
        guard let appState, appState.currentRoomId != nil else {
            return
        }

        // Step 3: commit-to-send —— 同步写节流锚点（防 Task await 期间同 state 再 emit 看到 nil 锚点重复 spawn）.
        lastSentState = newState
        lastSentAt = now

        // Step 4: fire-and-forget spawn UseCase Task —— 失败 silently 吞（与 sibling 同模式）.
        Task { @MainActor [weak self, syncPetStateUseCase] in
            guard self != nil else { return }
            do {
                _ = try await syncPetStateUseCase.execute(state: newState)
                // _ outcome 字段（.success(echoedState:) / .skippedNotInRoom）不消费 —— HTTP ack 仅作信号源；
                // skippedNotInRoom 在 UseCase 内部已防御性短路（caller 已 preflight，两层独立无副作用）.
            } catch {
                // 失败不阻塞 UI（epics.md §15.4 行 2441）；下次状态变化再试.
                // 节点 5 阶段不接 logger framework；失败被 silently 吞掉是 by design.
                _ = error
            }
        }
    }

    // MARK: - Cleanup

    deinit {
        // AnyCancellable.cancel() 是 thread-safe 的 nonisolated 方法 —— deinit 是 nonisolated 但调
        // subscription?.cancel() 安全；不触碰 @MainActor isolated 字段（subscription 字段读取本身
        // Swift 6 严格模式可能告警，但 deinit 是 final class 的 nonisolated 默认 —— 与 sibling
        // StepSyncTriggerService.deinit 同模式 / 同接受度）.
        subscription?.cancel()
    }
}

// MARK: - Story 15.5 AC3: PetStateReconnectAlignmentDelegate conformance

/// 与 class 主体解耦：class 主体只有"reactive subscription 触发器"语义；
/// extension 负责"reconnect-edge 触发器"语义.
extension PetStateSyncTriggerService: PetStateReconnectAlignmentDelegate {
    public func didReconnectAlignmentRequested() {
        triggerManualResync()
    }
}
