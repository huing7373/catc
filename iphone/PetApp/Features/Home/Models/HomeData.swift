// HomeData.swift
// Story 5.5 AC2: 首屏 domain 数据；从 HomeResponse wire DTO 转换得到.
//
// 设计：HomeData 是 ViewModel / View 直接消费的"业务数据"层；HomeResponse 是 wire DTO 层.
// 隔离意义：节点 4 / 7 / 9 后续扩展时只改 DTO + 转换 mapping，不污染 ViewModel.
//
// 节点 2 阶段 HomeData 与 HomeResponse 字段几乎 1:1（除 chest.remainingDisplay 等 derived 字段），
// 但保留独立类型让未来 derived 字段（如本地 timer 动态计算的 chest 倒计时）有单一去处.

import Foundation

public struct HomeData: Equatable, Sendable {
    public let user: HomeUser
    public let pet: HomePet?
    public let stepAccount: HomeStepAccount
    public let chest: HomeChest
    public let room: HomeRoom

    public init(
        user: HomeUser,
        pet: HomePet?,
        stepAccount: HomeStepAccount,
        chest: HomeChest,
        room: HomeRoom
    ) {
        self.user = user
        self.pet = pet
        self.stepAccount = stepAccount
        self.chest = chest
        self.room = room
    }

    /// 从 wire DTO 构造 domain 数据；当前节点 2 阶段是直白复制；
    /// 未来节点扩展加 derived 字段时，转换逻辑集中在此 init 内.
    public init(from response: HomeResponse) {
        self.user = HomeUser(
            id: response.user.id,
            nickname: response.user.nickname,
            avatarUrl: response.user.avatarUrl
        )
        if let pet = response.pet {
            self.pet = HomePet(
                id: pet.id,
                petType: pet.petType,
                name: pet.name,
                currentState: HomePetState(rawValue: pet.currentState) ?? .rest,
                equips: pet.equips.map { HomeEquip(from: $0) }
            )
        } else {
            self.pet = nil
        }
        self.stepAccount = HomeStepAccount(
            totalSteps: response.stepAccount.totalSteps,
            availableSteps: response.stepAccount.availableSteps,
            consumedSteps: response.stepAccount.consumedSteps
        )
        self.chest = HomeChest(
            id: response.chest.id,
            status: HomeChestStatus(rawValue: response.chest.status) ?? .counting,
            unlockAt: response.chest.unlockAt,
            openCostSteps: response.chest.openCostSteps,
            remainingSeconds: response.chest.remainingSeconds
        )
        self.room = HomeRoom(currentRoomId: response.room.currentRoomId)
    }

    /// 宝箱倒计时显示（mm:ss 格式）；剩余 0 秒返 "00:00".
    public var chestRemainingDisplay: String {
        chest.remainingDisplay
    }
}

public struct HomeUser: Equatable, Sendable {
    public let id: String
    public let nickname: String
    public let avatarUrl: String

    public init(id: String, nickname: String, avatarUrl: String) {
        self.id = id
        self.nickname = nickname
        self.avatarUrl = avatarUrl
    }
}

public struct HomePet: Equatable, Sendable {
    public let id: String
    public let petType: Int
    public let name: String
    public let currentState: HomePetState
    public let equips: [HomeEquip]

    public init(id: String, petType: Int, name: String, currentState: HomePetState, equips: [HomeEquip]) {
        self.id = id
        self.petType = petType
        self.name = name
        self.currentState = currentState
        self.equips = equips
    }
}

public enum HomePetState: Int, Equatable, Sendable {
    case rest = 1
    case walk = 2
    case run = 3
}

public struct HomeEquip: Equatable, Sendable {
    public let slot: Int
    public let userCosmeticItemId: String
    public let cosmeticItemId: String
    public let name: String
    public let rarity: Int
    public let assetUrl: String

    public init(from dto: EquipDTO) {
        self.slot = dto.slot
        self.userCosmeticItemId = dto.userCosmeticItemId
        self.cosmeticItemId = dto.cosmeticItemId
        self.name = dto.name
        self.rarity = dto.rarity
        self.assetUrl = dto.assetUrl
    }

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

public struct HomeStepAccount: Equatable, Sendable {
    public let totalSteps: Int
    public let availableSteps: Int
    public let consumedSteps: Int

    public init(totalSteps: Int, availableSteps: Int, consumedSteps: Int) {
        self.totalSteps = totalSteps
        self.availableSteps = availableSteps
        self.consumedSteps = consumedSteps
    }
}

public struct HomeChest: Equatable, Sendable {
    public let id: String
    public let status: HomeChestStatus
    public let unlockAt: Date
    public let openCostSteps: Int
    public let remainingSeconds: Int

    public init(id: String, status: HomeChestStatus, unlockAt: Date, openCostSteps: Int, remainingSeconds: Int) {
        self.id = id
        self.status = status
        self.unlockAt = unlockAt
        self.openCostSteps = openCostSteps
        self.remainingSeconds = remainingSeconds
    }

    /// mm:ss 格式倒计时显示；负值钳到 0；秒数 ≥ 60 时分母进位.
    public var remainingDisplay: String {
        let safe = max(0, remainingSeconds)
        let minutes = safe / 60
        let seconds = safe % 60
        return String(format: "%02d:%02d", minutes, seconds)
    }
}

public enum HomeChestStatus: Int, Equatable, Sendable {
    case counting = 1
    case unlockable = 2
}

public struct HomeRoom: Equatable, Sendable {
    public let currentRoomId: String?

    public init(currentRoomId: String?) {
        self.currentRoomId = currentRoomId
    }
}
