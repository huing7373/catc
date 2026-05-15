// LoadChestUseCase.swift
// Story 21.2 AC2: 宝箱状态加载 UseCase（业务编排：repo → DTO 转 domain → 写 AppState）.
//
// 职责（epics.md AC 行 3048-3052）:
//   1. 调 repository.fetchCurrent() 拿 ChestCurrentResponse
//   2. 转 domain HomeChest（与 HomeData(from:) 内 ChestDTO → HomeChest 同精神；未知 status fail-fast 抛 .decoding）
//   3. 成功 → 调 appState.applyCurrentChest(_:) 写入 AppState.currentChest（与 SyncStepsUseCase 写
//      stepAccount 同模式）
//   4. 失败 → throw 透传给上层 ChestRefreshTriggerService（service 决定不阻塞 UI / 下次再试）
//
// **不**做的事:
//   - 不接 ErrorPresenter（背景拉取失败不弹 toast；与 SyncStepsUseCase 同精神 → 失败不破坏 UI）
//   - 不做 retry / 指数退避（caller ChestRefreshTriggerService 用 60s timer 自然兜底；YAGNI）
//   - 不动 ChestTimerDriver / HomeViewModel.chestRemainingSeconds（driver 通过 Combine sink
//     AppState.$currentChest 自动 react；与 Story 21.1 AC2 钦定 "driver 订阅 AppState 而非
//     ViewModel 自己字段" 一致）
//   - 不读 HomeViewModel 任何字段（UseCase 不感知 view 层；与 SyncStepsUseCase 不读 ViewModel 同模式）
//
// 设计决策（story AC2 关键决策 1）：单字段 mutation 走新加 `applyCurrentChest(_:)`,
//   不走 applyHomeData. 理由：(a) addendum 钦定 "LoadChestUseCase 拿到 server 状态后写入目标改为
//   `appState.currentChest`"；(b) 本 endpoint 只返 5 个 chest 字段，无法构造完整 `HomeData` 走
//   `applyHomeData`；(c) 与 `SyncStepsUseCase` → `applySyncedStepAccount(_:)` 同前缀同模式.
//
// 设计决策（story AC2 关键决策 2）：未知 status fail-fast 抛 .decoding 而非 silently coerce.
//   - 与 Story 5.5 round 6 [P2] fix 钦定一致（详见 `docs/lessons/2026-04-27-home-data-fail-fast-on-unknown-enum.md`）；
//   - V1 §7.1 字段表 `status` 枚举值冻结，出现 99 之类未知值即 schema drift 信号；
//   - ChestRefreshTriggerService 层 catch 后 silently 吞（log）+ 保留 AppState 上次值 → UI 不破坏,
//     dev 仍能从 underlying error 看到 drift.
//
// 设计决策（story AC2 关键决策 3）：4001 不在 UseCase 内特殊处理，原样透传.
//   - (a) UseCase 层不感知错误码业务含义；
//   - (b) ChestRefreshTriggerService 层失败统一 silently 吞（log）；
//   - (c) 与 LoadHomeUseCase "所有错误**原样**透传" 同精神.
//   - Story 21.1 ChestCardView 已对 `currentChest == nil` 渲染 EmptyView()，4001 路径下
//     AppState.currentChest 保持上次值或仍是 nil 都不破坏 UI.
//
// 设计决策（story AC2 关键决策 4）：`await MainActor.run` 包裹 AppState 写入.
//   - 与 `SyncStepsUseCase` 同模式. AppState 是 `@MainActor`，UseCase 自身不限定 actor
//     （Sendable struct），写入时需显式 hop 到 main actor.

import Foundation

public protocol LoadChestUseCaseProtocol: Sendable {
    /// 执行一次宝箱状态拉取并写入 AppState.
    /// - Throws: APIError（全部 case 原样透传）/ HomeDataDecodingError（未知 chest.status 时）
    func execute() async throws
}

public struct DefaultLoadChestUseCase: LoadChestUseCaseProtocol {
    private let repository: ChestRepositoryProtocol
    private let appState: AppState

    public init(repository: ChestRepositoryProtocol, appState: AppState) {
        self.repository = repository
        self.appState = appState
    }

    public func execute() async throws {
        let response = try await repository.fetchCurrent()

        // DTO → domain（未知 chest.status fail-fast 抛 APIError.decoding；与 HomeData(from:) 同模式）.
        guard let status = HomeChestStatus(rawValue: response.status) else {
            throw APIError.decoding(underlying: HomeDataDecodingError.unknownChestStatus(response.status))
        }
        let chest = HomeChest(
            id: response.id,
            status: status,
            unlockAt: response.unlockAt,
            openCostSteps: response.openCostSteps,
            remainingSeconds: response.remainingSeconds
        )

        // 写 AppState（与 addendum 钦定一致；**不**写 ViewModel）.
        // 用 AppState 提供的 mutation 入口（AC3：applyCurrentChest(_:)）.
        await MainActor.run {
            appState.applyCurrentChest(chest)
        }
    }
}
