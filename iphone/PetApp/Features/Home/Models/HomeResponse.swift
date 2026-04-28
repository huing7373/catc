// HomeResponse.swift
// Story 5.5 AC1: GET /api/v1/home 的 wire DTO；严格对齐 V1 §5.1 + server homeResponseDTO（行 98-138）.
//
// 注：APIClient 已剥 envelope（code/message/data/requestId）；本类仅模型 envelope.data 字段内容.
//
// 字段可空性（V1 §4.1 行 16 钦定 §5.1 schema 已冻结，本 story 直接 wire）：
//   - data.pet: object | null —— 用户无默认 pet 时为 null（理论不应发生但 server Story 4.8 edge 强制覆盖）
//   - data.room.currentRoomId: string | null —— 节点 2 阶段强制 null；节点 4 后 Story 11.10 注入真实
//   - 其它字段全非空（pet 的子字段在 pet ≠ nil 时全非空）
//
// 与 GuestLoginResponse.UserProfile / PetProfile 的关系（Story 5.2 落地）：
//   - 不复用 —— UserProfile.hasBoundWechat 是 GuestLogin 特有；HomePet 必含 currentState（GuestLogin 没有）
//   - 节点 9 后 HomePet.equips 演化路径独立于 Auth 模块
//   - 详见本 story Dev Note #1 的"双 DTO 体系" 设计动机
//
// 节点 9+ 字段（pet.equips[].renderConfig）本 story 不预解析；节点 10 / Story 29.6 时追加 RenderConfig 子结构.

import Foundation

public struct HomeResponse: Decodable, Equatable {
    public let user: UserInfoDTO
    public let pet: HomePetDTO?           // 可空：V1 §5.1 行 335 钦定
    public let stepAccount: StepAccountDTO
    public let chest: ChestDTO
    public let room: RoomDTO

    public init(
        user: UserInfoDTO,
        pet: HomePetDTO?,
        stepAccount: StepAccountDTO,
        chest: ChestDTO,
        room: RoomDTO
    ) {
        self.user = user
        self.pet = pet
        self.stepAccount = stepAccount
        self.chest = chest
        self.room = room
    }
}

public struct UserInfoDTO: Decodable, Equatable, Sendable {
    public let id: String                 // BIGINT 序列化为 string（V1 §2.5）
    public let nickname: String
    public let avatarUrl: String          // 节点 2 阶段固定 ""（**不**为 null —— V1 §5.1 行 334 钦定）

    public init(id: String, nickname: String, avatarUrl: String) {
        self.id = id
        self.nickname = nickname
        self.avatarUrl = avatarUrl
    }
}

public struct HomePetDTO: Decodable, Equatable, Sendable {
    public let id: String
    public let petType: Int               // 节点 2 固定 1（猫）
    public let name: String
    public let currentState: Int          // 1=rest, 2=walk, 3=run（V1 §5.1 + 数据库设计 §6.4）
    public let equips: [EquipDTO]         // 节点 2 阶段强制 []；节点 9 由 Story 26.6 填充

    public init(id: String, petType: Int, name: String, currentState: Int, equips: [EquipDTO]) {
        self.id = id
        self.petType = petType
        self.name = name
        self.currentState = currentState
        self.equips = equips
    }
}

/// 装扮元素 DTO；节点 2 阶段 server 强制返回 `equips: []`，本类型仅做契约预留，实测不会被构造.
/// 节点 9 / Story 26.6 server 端填充真实数据时，client 自动解码（**0** 改动）.
public struct EquipDTO: Decodable, Equatable, Sendable {
    public let slot: Int
    public let userCosmeticItemId: String
    public let cosmeticItemId: String
    public let name: String
    public let rarity: Int
    public let assetUrl: String

    public init(
        slot: Int,
        userCosmeticItemId: String,
        cosmeticItemId: String,
        name: String,
        rarity: Int,
        assetUrl: String
    ) {
        self.slot = slot
        self.userCosmeticItemId = userCosmeticItemId
        self.cosmeticItemId = cosmeticItemId
        self.name = name
        self.rarity = rarity
        self.assetUrl = assetUrl
    }
}

public struct StepAccountDTO: Decodable, Equatable, Sendable {
    public let totalSteps: Int
    public let availableSteps: Int
    public let consumedSteps: Int

    public init(totalSteps: Int, availableSteps: Int, consumedSteps: Int) {
        self.totalSteps = totalSteps
        self.availableSteps = availableSteps
        self.consumedSteps = consumedSteps
    }
}

public struct ChestDTO: Decodable, Equatable, Sendable {
    public let id: String
    public let status: Int                // 1=counting, 2=unlockable（V1 §5.1 行 345）
    public let unlockAt: Date             // ISO 8601 RFC3339
    public let openCostSteps: Int
    public let remainingSeconds: Int      // ≥ 0；server 已算好

    public init(id: String, status: Int, unlockAt: Date, openCostSteps: Int, remainingSeconds: Int) {
        self.id = id
        self.status = status
        self.unlockAt = unlockAt
        self.openCostSteps = openCostSteps
        self.remainingSeconds = remainingSeconds
    }
}

public struct RoomDTO: Decodable, Equatable, Sendable {
    /// 节点 2 阶段强制 nil；节点 4 起 Story 11.10 注入真实房间 ID.
    public let currentRoomId: String?

    public init(currentRoomId: String?) {
        self.currentRoomId = currentRoomId
    }
}
