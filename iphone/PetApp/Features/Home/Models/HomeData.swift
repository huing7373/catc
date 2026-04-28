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
    ///
    /// **Story 5.5 round 6 [P2] fix**：本 init 改 `throws`；未知 `pet.currentState` /
    /// `chest.status` 枚举值不再静默 coerce 到 `.rest` / `.counting`，改抛
    /// `APIError.decoding` —— 让 server/client schema drift 立刻 fail-fast 触达 UI.
    /// **round 9 [P2] 调整**: bootstrap 路径下 `.decoding` 现在渲染 RetryView "数据异常，请重试"
    /// (mapper 把 .decoding 视为 transient, 给 user 自助恢复入口); dev 仍可从 underlying
    /// `HomeDataDecodingError` 看到具体哪个 enum 字段拿到未知值.
    /// 详见 AppErrorMapper `.decoding` 映射 + docs/lessons/2026-04-27-transient-vs-terminal-error-classification.md.
    ///
    /// 原方案 `?? .rest` / `?? .counting` 会在新增枚举值（如未来 `HomePetState.sleep`）时把
    /// 真实状态错误渲染成 rest/counting，dev 期间无任何 signal、生产期 silently 错；
    /// V1 §4.1 行 16 钦定 `/home` schema 已 frozen → 出现未知值就是真实异常，必须 fail-fast.
    /// 详见 docs/lessons/2026-04-27-home-data-fail-fast-on-unknown-enum.md.
    public init(from response: HomeResponse) throws {
        self.user = HomeUser(
            id: response.user.id,
            nickname: response.user.nickname,
            avatarUrl: response.user.avatarUrl
        )
        if let pet = response.pet {
            guard let petState = HomePetState(rawValue: pet.currentState) else {
                throw APIError.decoding(underlying: HomeDataDecodingError.unknownPetCurrentState(pet.currentState))
            }
            self.pet = HomePet(
                id: pet.id,
                petType: pet.petType,
                name: pet.name,
                currentState: petState,
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
        guard let chestStatus = HomeChestStatus(rawValue: response.chest.status) else {
            throw APIError.decoding(underlying: HomeDataDecodingError.unknownChestStatus(response.chest.status))
        }
        self.chest = HomeChest(
            id: response.chest.id,
            status: chestStatus,
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

/// Story 5.5 round 6 [P2] fix: 描述 `HomeData(from:)` 解 wire DTO 时的 schema drift 失败原因.
/// 作为 `APIError.decoding(underlying:)` 的 underlying，让 log / 调试能看到具体哪个 enum 字段拿到了未知值.
///
/// 不直接抛 `APIError.decoding`：把 underlying 单独命名 → 测试断言可以匹配具体子类型，
/// 同时让 AppErrorMapper 仍走 `.decoding` 通用文案 ("数据异常，请重试"，round 9 [P2] fix 后
/// bootstrap 路径渲染为 RetryView, 让 user 能自助重试)，无需新错误码.
public enum HomeDataDecodingError: Error, Equatable {
    /// 后端返回了客户端未知的 `pet.currentState` 枚举值（V1 §4.1 frozen schema 之外）.
    case unknownPetCurrentState(Int)

    /// 后端返回了客户端未知的 `chest.status` 枚举值.
    case unknownChestStatus(Int)
}
