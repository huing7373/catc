// SyncPetStateUseCase.swift
// Story 15.4 AC2: 当前 pet 状态同步 UseCase（业务编排：roomId guard → 调 repo → 返 outcome）.
//
// 与 sibling SyncStepsUseCase 同模式（struct + 注入 repository / appState）；与 SyncStepsUseCase
// 不同点：**本 UseCase 不写 AppState**（HTTP `data.state` ack 信号不能驱动 UI；详见 V1 §5.2 line 610-613
// + lesson `2026-05-12-state-sync-err-binary-and-placeholder-whitelist-self-http-ack-14-1-r6.md`）.
//
// 职责（epics.md §15.4 行 2436-2447 钦定）:
//   1. 读 AppState.currentRoomId snapshot（@MainActor 隔离 → 跨 actor 调 await MainActor.run）
//   2. 不在房间 → 返 `.skippedNotInRoom`（caller 已 preflight 一次，本层再做防御性短路）
//   3. 在房间 → 拼 PetStateSyncRequest（state.wireValue ∈ {1,2,3}）→ 调 repo → 返 `.success(echoedState:)`
//   4. 失败 → APIError 原样透传（caller=PetStateSyncTriggerService 在 service 层 silently 吞）
//
// **不**做的事（红线，AC2 段钦定）:
//   - 不写 AppState（与 SyncStepsUseCase 不同；HTTP echo 不是 server-authoritative state，**禁止**驱动 UI；
//     UI 由 motionProvider → HomeViewModel.petState 驱动；self-entry 房间页猫由 Story 15.2 self-broadcast 路径处理）
//   - 不接 ErrorPresenter（背景同步失败不弹 toast；epics.md §15.4 行 2441 钦定）
//   - 不做 retry / 指数退避（YAGNI；TriggerService 层依靠"下次状态变化"自然兜底）
//   - 不在 UseCase 内部读 ViewModel（caller 注入 state 值；与 SyncStepsUseCase 一致）

import Foundation

public enum SyncPetStateUseCaseOutcome: Equatable, Sendable {
    /// 同步成功；echoedState 是 server 回显的 wire state（仅用于 ack 信号 / 测试断言 / 未来 log；**禁止**驱动 UI）.
    case success(echoedState: Int)
    /// roomId == nil → 短路返回（caller TriggerService 也会 preflight，两层独立防御；
    /// 与 V1 §5.2 line 547 "用户不在房间是合法 200 OK + code=0 场景" 形成 client 端流量节省 preflight）.
    case skippedNotInRoom
}

public protocol SyncPetStateUseCaseProtocol: Sendable {
    /// 执行一次 pet 状态同步（roomId guard → 调 repo → 返 outcome）.
    /// - Parameter state: 当前 motion state（由 caller 提供 —— TriggerService 从 HomeViewModel.petState 取）
    /// - Returns: SyncPetStateUseCaseOutcome（success 或 skippedNotInRoom）
    /// - Throws: APIError 原样透传（caller 决定是否 silently 吞）
    func execute(state: MotionState) async throws -> SyncPetStateUseCaseOutcome
}

public struct DefaultSyncPetStateUseCase: SyncPetStateUseCaseProtocol {
    private let repository: PetRepositoryProtocol
    private let appState: AppState

    public init(repository: PetRepositoryProtocol, appState: AppState) {
        self.repository = repository
        self.appState = appState
    }

    public func execute(state: MotionState) async throws -> SyncPetStateUseCaseOutcome {
        // Step 1: 读 AppState.currentRoomId snapshot（@MainActor isolated → 跨 actor 调 await MainActor.run）.
        // 本 UseCase 是 nonisolated struct → 必须 hop 到 main actor 同步读单字段（与 SyncStepsUseCase
        // 写 AppState 用 `await MainActor.run { ... }` 同模式）.
        let roomId: String? = await MainActor.run { appState.currentRoomId }
        guard roomId != nil else {
            // 不在房间 → 短路（不调 repo / 不消耗资源）.
            return .skippedNotInRoom
        }

        // Step 2: 拼请求体（单字段 state.wireValue ∈ {1,2,3}；不带 petId / 不带 idempotencyKey header）.
        let request = PetStateSyncRequest(state: state.wireValue)

        // Step 3: 调 repo → 拿 echoed state（仅 ack 信号；不驱动 UI / 不写 AppState）.
        // 网络失败 / 业务 code != 0 → APIError 原样透传给 caller（TriggerService 层 silently 吞）.
        let response = try await repository.syncPetState(request)

        return .success(echoedState: response.state)
    }
}
