// GuestLoginResponse.swift
// Story 5.2 AC1: POST /api/v1/auth/guest-login 响应 data；严格对齐 V1 §4.1 行 178-188 schema。
//
// 注：APIClient 已剥 envelope（code/message/data/requestId）；本类仅模型 envelope.data 字段内容。
//
// V1 §4.1 钦定 data 字段：
// - token: string —— JWT，HS256 + auth.token_secret 签名（Story 4.4 落地）；默认过期 7 天
// - user.id: string —— BIGINT 序列化为 string（V1 §2.5）
// - user.nickname: string —— 自动生成 `用户{id}`
// - user.avatarUrl: string —— 首次创建为 ""（**不是** null —— 客户端不需要 Optional<String>）
// - user.hasBoundWechat: boolean
// - pet.id: string
// - pet.petType: number —— 节点 2 固定 1（猫）
// - pet.name: string —— 首次创建为 "默认小猫"
//
// `UserProfile` / `PetProfile` 提为顶层 `public struct`：因为 Story 5.5 LoadHomeUseCase /
// 节点 4 房间链路也会用同样的类型；本 story 一次性建立，后续 stories 直接复用。
// `Sendable` 标注：SessionState（AC4）持有这两个类型，跨 actor 边界传递必须 Sendable。

import Foundation

public struct GuestLoginResponse: Decodable, Equatable {
    public let token: String
    public let user: UserProfile
    public let pet: PetProfile

    public init(token: String, user: UserProfile, pet: PetProfile) {
        self.token = token
        self.user = user
        self.pet = pet
    }
}

public struct UserProfile: Decodable, Equatable, Sendable {
    public let id: String
    public let nickname: String
    public let avatarUrl: String
    public let hasBoundWechat: Bool

    public init(id: String, nickname: String, avatarUrl: String, hasBoundWechat: Bool) {
        self.id = id
        self.nickname = nickname
        self.avatarUrl = avatarUrl
        self.hasBoundWechat = hasBoundWechat
    }
}

public struct PetProfile: Decodable, Equatable, Sendable {
    public let id: String
    public let petType: Int   // 节点 2 固定 1
    public let name: String

    public init(id: String, petType: Int, name: String) {
        self.id = id
        self.petType = petType
        self.name = name
    }
}
