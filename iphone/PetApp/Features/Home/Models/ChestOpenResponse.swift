// ChestOpenResponse.swift
// Story 21.3 AC1: V1 §7.2 POST /api/v1/chest/open 响应 wire DTO.
//
// 契约源（V1 §7.2 r15 review 已冻结）:
// - 外层走 APIClient 既有 envelope 解包（`code/message/data/requestId`，已 Story 2.4 / 5.5 落地）
// - data 字段三段嵌套：data.reward + data.stepAccount + data.nextChest
//
// 关键 schema 选择（与 Story 8.5 StepsSyncResponse 同模式）:
// - Decodable + Equatable + Sendable
// - 嵌套结构体单独命名（不复用 §7.1 ChestCurrentResponse —— §7.2 是"动作型"返回 next chest;
//   命名 ChestSnapshotInOpenResponse 表达"作为 open 响应的一部分"，与 StepAccountInSyncResponse 同精神）
//
// 节点 7 vs 节点 8 阶段（V1 §7.2.4h + 21.3 spec 红线钦定）:
// - reward.userCosmeticItemId: 节点 7 固定字符串 "0" 占位；节点 8 Story 23.5 起为真实 BIGINT 字符串
//   client 解析层**严格按 String** 处理（不 Optional / 不动态判断 "0" 做业务路径分支；V1 §7.2 关键约束行 1148）
//
// nextChest.status / nextChest.remainingSeconds: server 实时计算字段（与 §7.1 GET /chest/current
// 同源同时刻；详见 V1 §7.2.6 字段表 status / remainingSeconds 行说明）.

import Foundation

public struct ChestOpenResponse: Decodable, Sendable, Equatable {
    public let reward: ChestRewardDTO
    public let stepAccount: StepAccountInOpenResponse
    public let nextChest: ChestSnapshotInOpenResponse

    public init(
        reward: ChestRewardDTO,
        stepAccount: StepAccountInOpenResponse,
        nextChest: ChestSnapshotInOpenResponse
    ) {
        self.reward = reward
        self.stepAccount = stepAccount
        self.nextChest = nextChest
    }
}

public struct ChestRewardDTO: Decodable, Sendable, Equatable {
    public let userCosmeticItemId: String   // 节点 7 阶段固定 "0"；节点 8 起真实主键；client 只存不展示
    public let cosmeticItemId: String       // 装扮配置 id（BIGINT 字符串化）
    public let name: String                 // "星星围巾" 等装扮名（1 ≤ length ≤ 64）
    public let slot: Int                    // 1..7 / 99 枚举（V1 §6.8）
    public let rarity: Int                  // 1..4 枚举（common/rare/epic/legendary；V1 §6.9）
    public let assetUrl: String             // 非空字符串
    public let iconUrl: String              // 非空字符串

    public init(
        userCosmeticItemId: String,
        cosmeticItemId: String,
        name: String,
        slot: Int,
        rarity: Int,
        assetUrl: String,
        iconUrl: String
    ) {
        self.userCosmeticItemId = userCosmeticItemId
        self.cosmeticItemId = cosmeticItemId
        self.name = name
        self.slot = slot
        self.rarity = rarity
        self.assetUrl = assetUrl
        self.iconUrl = iconUrl
    }
}

public struct StepAccountInOpenResponse: Decodable, Sendable, Equatable {
    public let totalSteps: Int
    public let availableSteps: Int
    public let consumedSteps: Int

    public init(totalSteps: Int, availableSteps: Int, consumedSteps: Int) {
        self.totalSteps = totalSteps
        self.availableSteps = availableSteps
        self.consumedSteps = consumedSteps
    }
}

public struct ChestSnapshotInOpenResponse: Decodable, Sendable, Equatable {
    public let id: String                  // BIGINT 字符串化
    public let status: Int                 // 1 = counting / 2 = unlockable
    public let unlockAt: Date              // ISO 8601 RFC3339（APIClient JSONDecoder 已配 .iso8601）
    public let openCostSteps: Int          // 节点 7 阶段固定 1000
    public let remainingSeconds: Int       // server 实时计算（0..600）

    public init(id: String, status: Int, unlockAt: Date, openCostSteps: Int, remainingSeconds: Int) {
        self.id = id
        self.status = status
        self.unlockAt = unlockAt
        self.openCostSteps = openCostSteps
        self.remainingSeconds = remainingSeconds
    }
}
