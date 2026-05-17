// InventoryResponse.swift
// Story 24.2 AC1: GET /api/v1/cosmetics/inventory envelope.data wire DTO.
//
// 对应 V1 §8.2（行 1362-1377 响应体字段表，schema Epic 23 冻结）：
//   data: { groups: [InventoryGroup] }
//   InventoryGroup: { cosmeticItemId, name, slot, rarity, iconUrl, assetUrl, count, instances }
//   InventoryInstance: { userCosmeticItemId, status }
//
// `groups` non-optional `[InventoryGroup]`：V1 §8.2 关键约束「空背包」钦定 ——
// 服务端空背包返回 `{groups: []}`（**不**是 `null`、**不**是缺 groups 字段）。client 严格按
// `[InventoryGroup]` 非可选解析（空时为 `[]`）。若 server 违约返 null / 缺字段，APIClient 直接
// 抛 APIError.decoding，符合 fail-fast（与 EmojiListResponse `items` 非可选 + lesson
// `2026-04-27-home-data-fail-fast-on-unknown-enum.md` 同精神）。
//
// `instances[].status`：V1 §8.2 仅可能为 `1=in_bag` / `2=equipped`（server 已过滤
// `3=consumed` / `4=invalid`，不出现在 inventory）。DTO 仍按 Int 解析保留契约完整性
// （未来 Story 24.5 / 27.1 区分 equipped 用）；本 story LoadInventoryUseCase 展平时不映射
// status 进 HomeEquip（HomeEquip 无该字段，详见 story Dev Notes「HomeEquip 字段映射决策」）。
//
// **不**在 DTO 层做排序 / 去重 / 校验：V1 §8.2 步骤 5 已契约保证 groups[] 与 instances[]
// 两级确定性全序，client 直通（与 EmojiRepository 注释「server 已保证顺序，repo 只
// wire→domain 直通」同精神）。
//
// 仅 `Decodable, Equatable, Sendable`（与 EmojiListResponse / ChestCurrentResponse 同
// conformance 模式）；import 仅 Foundation。

import Foundation

public struct InventoryResponse: Decodable, Equatable, Sendable {
    public let groups: [InventoryGroup]

    public init(groups: [InventoryGroup]) {
        self.groups = groups
    }
}

public struct InventoryGroup: Decodable, Equatable, Sendable {
    public let cosmeticItemId: String
    public let name: String
    public let slot: Int
    public let rarity: Int
    public let iconUrl: String
    public let assetUrl: String
    public let count: Int
    public let instances: [InventoryInstance]

    public init(
        cosmeticItemId: String,
        name: String,
        slot: Int,
        rarity: Int,
        iconUrl: String,
        assetUrl: String,
        count: Int,
        instances: [InventoryInstance]
    ) {
        self.cosmeticItemId = cosmeticItemId
        self.name = name
        self.slot = slot
        self.rarity = rarity
        self.iconUrl = iconUrl
        self.assetUrl = assetUrl
        self.count = count
        self.instances = instances
    }
}

public struct InventoryInstance: Decodable, Equatable, Sendable {
    public let userCosmeticItemId: String
    /// V1 §8.2：枚举 `{1,2}`（in_bag / equipped；server 已过滤 consumed / invalid）.
    public let status: Int

    public init(userCosmeticItemId: String, status: Int) {
        self.userCosmeticItemId = userCosmeticItemId
        self.status = status
    }
}
