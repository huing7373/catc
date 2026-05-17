// LoadInventoryUseCase.swift
// Story 24.2 AC3: 背包加载 UseCase（业务编排：repo → wire DTO → 展平为 [HomeEquip] →
// 写 appState.currentInventory）.
//
// 职责（与 LoadChestUseCase 同骨架）:
//   1. 调 repository.fetchInventory() 拿 InventoryResponse
//   2. DTO → domain 展平：把 response.groups 的每个 group 的每个 instance 展平为一个
//      HomeEquip —— **一个 instance 一个 HomeEquip**（同 group 多 instance 各成独立 HomeEquip）
//   3. 成功 → await MainActor.run { appState.applyInventory(flattened) } 写 AppState
//   4. 失败 → throw 原样透传给调用方（caller 决定 loading / retry UI）
//
// **展平不聚合**（关键约束）：V1 §8.2 wire 是 group 聚合结构（groups[].instances[]），但
// Story 24.1 既有 sink 订阅 appState.$currentInventory: [HomeEquip]（平铺），且 24.1
// mapToCosmeticItems 是「一 HomeEquip → 一 CosmeticItem，不去重不聚合，count 由 view
// filter().count 自然得出」。故本 UseCase 必须把 groups[].instances[] 展平，**不**做
// group-by 聚合（否则破坏 24.1 既有渲染契约）。这样 24.1 sink / mapping 零 edit 即正确。
//
// **不缓存**（epics.md 行 3367「每次打开都重新加载，不缓存」—— 开箱后需立即看到新道具）:
//   走 `struct` 无状态（与 LoadChestUseCase 同），**不**学 LoadEmojisUseCase 的 actor +
//   cache + single-flight（emoji 是静态配置可缓存；inventory 开箱后必须立即变，反缓存）.
//
// **失败不污染 AppState**：失败路径 throw 原样透传，**不**写 appState.currentInventory
//   （保持上次值或空）；与 LoadChestUseCase「失败透传，caller 决定不阻塞 UI」精神一致.
//
// HomeEquip 字段映射（story Dev Notes「HomeEquip 字段映射决策」钦定）：每 (group, instance)
//   → 一 HomeEquip：slot ← group.slot / userCosmeticItemId ← instance.userCosmeticItemId /
//   cosmeticItemId ← group.cosmeticItemId / name ← group.name / rarity ← group.rarity /
//   assetUrl ← group.assetUrl。group.iconUrl + instance.status 丢弃（HomeEquip 无承载字段；
//   24.1 sink/mapping 不消费；HomeEquip 是节点 1 占位复用类型 ADR-0010 §4.4，本 story 不演进）.
//
// 设计决策（与 LoadChestUseCase 关键决策 4 同模式）：`await MainActor.run` 包裹 AppState
//   写入. AppState 是 @MainActor；UseCase 自身不限定 actor（Sendable struct），写入时需显式
//   hop 到 main actor.
//
// import 仅 Foundation.

import Foundation

public protocol LoadInventoryUseCaseProtocol: Sendable {
    /// 执行一次背包拉取并写入 AppState（成功 → 写 currentInventory；失败 → rethrow）.
    /// - Throws: APIError（全部 case 原样透传）
    func execute() async throws
}

public struct DefaultLoadInventoryUseCase: LoadInventoryUseCaseProtocol {
    private let repository: InventoryRepositoryProtocol
    private let appState: AppState

    public init(repository: InventoryRepositoryProtocol, appState: AppState) {
        self.repository = repository
        self.appState = appState
    }

    public func execute() async throws {
        let response = try await repository.fetchInventory()

        // DTO → domain 展平（groups[].instances[] → [HomeEquip]，一 instance 一 HomeEquip）.
        let flattened = Self.flatten(response)

        // 写 AppState（用 AC4 单字段 mutation 入口；**不**写 ViewModel）.
        await MainActor.run {
            appState.applyInventory(flattened)
        }
    }

    /// 展平：把 `groups[].instances[]` 转成平铺 `[HomeEquip]`（**一个 instance 一个 HomeEquip**，
    /// 不去重不聚合）. 抽 static 私有方法便于单测直接断言（ADR-0002 §3.1 XCTest only ——
    /// 不经 network / view 内省；与 24.1 mapToCosmeticItems 抽方法直测同模式）.
    ///
    /// 空背包：`response.groups == []` → flatMap 结果 `[]`（V1 §8.2「空背包不报错」）.
    static func flatten(_ response: InventoryResponse) -> [HomeEquip] {
        response.groups.flatMap { group in
            group.instances.map { instance in
                HomeEquip(
                    slot: group.slot,
                    userCosmeticItemId: instance.userCosmeticItemId,
                    cosmeticItemId: group.cosmeticItemId,
                    name: group.name,
                    rarity: group.rarity,
                    assetUrl: group.assetUrl
                    // group.iconUrl + instance.status 丢弃（HomeEquip 无承载字段；
                    // 24.1 sink/mapping 不消费；详见 story Dev Notes 字段映射决策）.
                )
            }
        }
    }
}
