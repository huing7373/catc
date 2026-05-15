// OpenChestUseCase.swift
// Story 21.3 AC4: 开箱 UseCase（业务编排：generate idempotencyKey → repo.openChest → DTO 转 domain →
// 写 AppState.currentChest + AppState.currentStepAccount → 返回 ChestRewardSnapshot 给 caller）.
//
// 职责（spec AC 行 3074-3088 + 21.3 addendum 钦定）:
//   1. 从 IdempotencyKeyGenerator capture 一次 idempotencyKey（同一次 execute 调用内复用）
//   2. (Story 21.5 入位：optionally await stepSyncTriggerService.triggerManual() —— 本 story 默认 nil 不调)
//   3. 调 repository.openChest(ChestOpenRequest(idempotencyKey:)) 拿 ChestOpenResponse
//   4. 转 domain:
//        - response.nextChest → HomeChest (未知 status fail-fast 抛 .decoding；与 LoadChestUseCase 同模式)
//        - response.stepAccount → HomeStepAccount (无 enum 字段，直接构造)
//        - response.reward → ChestRewardSnapshot (未知 rarity fail-fast 抛 .decoding)
//   5. **同一 await MainActor.run 块内**双写 AppState：
//        - appState.applyCurrentChest(nextChest) （Story 21.2 AC3 落地的入口）
//        - appState.applySyncedStepAccount(stepAccount) （Story 8.5 AC7 落地的入口）
//   6. 返回 snapshot 给 caller（RealHomeViewModel.onChestOpenTap 写到 pendingReward）
//
// **不**做的事:
//   - 不接 ErrorPresenter（caller 决定错误展示策略；与 SyncStepsUseCase / LoadChestUseCase 同精神）
//   - 不做 retry / 指数退避（V1 §7.2 client 重试策略钦定 retry 由 caller / UI 层决定；本 use case 单次）
//   - 不动 HomeViewModel 任何字段（isOpening 由 RealHomeViewModel.onChestOpenTap 内 set，不进 UseCase）
//   - 不读 AppState 现有字段（idempotency 是无状态生成；不需要 current chest id / currentStepAccount 上下文）
//
// idempotencyKey 复用语义（同一次 execute 调用复用同 key）:
//   - execute 入口 capture 一次 key → request 内 capture → throw 后 caller 重试时调 execute 重新生成
//     新 key（因为 caller 起新 Task → 新 UseCase.execute → 新 generate）.
//   - 这与 V1 §7.2 关键约束行 940 "client 应在每次点击开箱按钮时生成新的 key" 对齐：
//     "每次点击" = 每次 onChestOpenTap = 每次 execute = 一次 generate.
//   - "网络抖动重试时复用同一 key" 由 APIClient 内部 retry policy 落地（如果未来引入；本 story 不引入）.

import Foundation

public protocol OpenChestUseCaseProtocol: Sendable {
    /// 执行一次开箱.
    /// - Returns: 奖励快照（给 caller 写 ViewModel.pendingReward）
    /// - Throws: APIError（全部 case 原样透传给 caller）/ ChestOpenDecodingError（未知 chest.status / 未知 rarity）
    func execute() async throws -> ChestRewardSnapshot
}

public struct DefaultOpenChestUseCase: OpenChestUseCaseProtocol {
    private let repository: ChestRepositoryProtocol
    private let appState: AppState
    private let keyGenerator: IdempotencyKeyGenerator
    /// Story 21.5 入位：本 story 默认 nil，不调；21.5 落地时改默认传 stepSyncTriggerService 实例.
    /// **MainActor-isolated**: StepSyncTriggerService 是 @MainActor 单实例；UseCase 自身不限定 actor,
    /// 调 triggerManual() 时显式 `await` hop 到 main actor.
    private let stepSyncTriggerService: StepSyncTriggerService?

    public init(
        repository: ChestRepositoryProtocol,
        appState: AppState,
        keyGenerator: IdempotencyKeyGenerator = DefaultIdempotencyKeyGenerator(),
        stepSyncTriggerService: StepSyncTriggerService? = nil
    ) {
        self.repository = repository
        self.appState = appState
        self.keyGenerator = keyGenerator
        self.stepSyncTriggerService = stepSyncTriggerService
    }

    public func execute() async throws -> ChestRewardSnapshot {
        // Step 0 (Story 21.5 入位): 主动同步步数让 server 用最新余额判定（本 story stepSyncTriggerService = nil，跳过）.
        // 21.5 落地时此处 await stepSyncTriggerService?.triggerManual() —— 失败也继续开箱（不阻塞，让 server
        // 用上一次 sync 后的余额判定；与 Story 21.5 AC "同步失败也继续开箱" 钦定一致）.
        if let stepSync = stepSyncTriggerService {
            await stepSync.triggerManual()
        }

        // Step 1: 生成 idempotencyKey（同一次 execute 复用同 key；caller 重试调 execute 时自然新 key）.
        let idempotencyKey = keyGenerator.generate()
        let request = ChestOpenRequest(idempotencyKey: idempotencyKey)

        // Step 2: 调 server.
        let response = try await repository.openChest(request)

        // Step 3: DTO → domain（未知 enum fail-fast 抛 .decoding；与 LoadChestUseCase / HomeData(from:) 同模式）.
        guard let chestStatus = HomeChestStatus(rawValue: response.nextChest.status) else {
            throw APIError.decoding(underlying: ChestOpenDecodingError.unknownNextChestStatus(response.nextChest.status))
        }
        guard let rarity = RewardRarity(rawValue: response.reward.rarity) else {
            throw APIError.decoding(underlying: ChestOpenDecodingError.unknownRewardRarity(response.reward.rarity))
        }

        let nextChest = HomeChest(
            id: response.nextChest.id,
            status: chestStatus,
            unlockAt: response.nextChest.unlockAt,
            openCostSteps: response.nextChest.openCostSteps,
            remainingSeconds: response.nextChest.remainingSeconds
        )
        let stepAccount = HomeStepAccount(
            totalSteps: response.stepAccount.totalSteps,
            availableSteps: response.stepAccount.availableSteps,
            consumedSteps: response.stepAccount.consumedSteps
        )
        let snapshot = ChestRewardSnapshot(
            cosmeticItemId: response.reward.cosmeticItemId,
            name: response.reward.name,
            slot: response.reward.slot,
            rarity: rarity,
            assetUrl: response.reward.assetUrl,
            iconUrl: response.reward.iconUrl
        )

        // Step 4: 同一 main actor 同步块写双字段 AppState（保证 nextChest + stepAccount 原子可见；
        //         driver 一次 main actor tick 内同时接收两次 @Published 变化）.
        // 与 SyncStepsUseCase 单字段写、LoadChestUseCase 单字段写 同精神 + 增量扩展（本 use case 是
        // V1 §7.2 钦定的"动作返回三段嵌套"，必须双写）.
        await MainActor.run {
            appState.applyCurrentChest(nextChest)
            appState.applySyncedStepAccount(stepAccount)
        }

        // Step 5: 返回 snapshot 给 caller.
        return snapshot
    }
}

/// 描述 ChestOpenResponse 解析时的 schema drift 失败原因（与 HomeDataDecodingError 同精神）.
/// 单独命名让单测可精确断言子类型；UI 文案统一走 .decoding 通用 mapper（"数据异常，请重试"）.
public enum ChestOpenDecodingError: Error, Equatable {
    case unknownNextChestStatus(Int)
    case unknownRewardRarity(Int)
}
