// ChestRewardSnapshot.swift
// Story 21.3 AC3: 开箱奖励 domain 快照（给 Story 21.4 RewardPopupView 用）.
//
// 来源 / 用途:
//   - OpenChestUseCase.execute 成功路径返回此 snapshot 给 caller（RealHomeViewModel.onChestOpenTap）
//   - RealHomeViewModel 把 snapshot 写到 transient `pendingReward` 字段（HomeViewModel @Published）
//   - Story 21.4 RewardPopupView 通过 .sheet(item: $pendingReward) 订阅触发弹窗（Identifiable 需要）
//
// 字段从 ChestRewardDTO 映射:
//   - cosmeticItemId / name / slot / rarity / assetUrl / iconUrl 透传（rarity 转 RewardRarity enum）
//   - userCosmeticItemId 节点 7 占位 "0" → 不存（V1 §7.2 关键约束行 1148 "client UI 层禁止展示此字段"
//     + "client 不作为业务路径分支判断"；snapshot 是 UI domain model，不存 audit-only 字段）
//
// 与 wire DTO（ChestRewardDTO）区分的理由（与 HomeChest vs ChestCurrentResponse 同精神）:
//   - DTO 是 wire schema（API 解析层）
//   - Snapshot 是 domain model（UseCase 层 + View 层共享）
//   - 转换在 UseCase 内做（与 Story 21.2 LoadChestUseCase 内 ChestCurrentResponse → HomeChest 同模式）

import Foundation

public struct ChestRewardSnapshot: Equatable, Sendable, Identifiable {
    /// 用 cosmeticItemId 作为 Identifiable.id（让 SwiftUI .sheet(item:) 复用同一弹窗实例时 diff 正确）.
    /// 节点 7 阶段每次 reward 的 cosmeticItemId 可能重复（同一装扮被多次抽中），但 SwiftUI .sheet(item:)
    /// 在 item 从 nil → non-nil 时仍触发新 sheet —— Identifiable 实现仅影响"同 non-nil item 变化时是否
    /// reuse sheet"，本场景下不可达（要么 nil，要么从 nil 到 non-nil；节点 8 后才有同时刻多 reward 队列场景）.
    public var id: String { cosmeticItemId }

    public let cosmeticItemId: String
    public let name: String
    public let slot: Int
    public let rarity: RewardRarity
    public let assetUrl: String
    public let iconUrl: String

    public init(
        cosmeticItemId: String,
        name: String,
        slot: Int,
        rarity: RewardRarity,
        assetUrl: String,
        iconUrl: String
    ) {
        self.cosmeticItemId = cosmeticItemId
        self.name = name
        self.slot = slot
        self.rarity = rarity
        self.assetUrl = assetUrl
        self.iconUrl = iconUrl
    }
}

/// V1 §6.9 + 数据库设计 §6.9 钦定 4 档品质枚举.
/// 单独 enum 让 RewardPopupView（Story 21.4 落地）按 rarity 派生徽章颜色（common 灰 / rare 蓝 / epic 紫 / legendary 金）.
/// raw value 与 wire DTO `rarity: Int` 对齐（1..4）.
public enum RewardRarity: Int, Equatable, Sendable {
    case common = 1
    case rare = 2
    case epic = 3
    case legendary = 4
}
