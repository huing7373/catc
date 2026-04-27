// SessionState.swift
// Story 5.2 AC4: 节点 2 阶段简化版会话状态 —— 仅含 user + pet（**不**含 token）.
//
// token 由 Keychain 单点持有（KeychainKey.authToken），SessionStore 不重复持有避免双源.
//
// 节点 2 之后可能扩展（不属本 story scope）：
// - currentRoom（节点 4 房间状态）
// - stepAccount snapshot（节点 3 步数）
// - 这些字段都通过 GET /home 拿，归 Story 5.5 LoadHomeUseCase 落地后由 SessionStore 或单独 HomeStore 持有

import Foundation

public struct SessionState: Equatable, Sendable {
    public let user: UserProfile
    public let pet: PetProfile

    public init(user: UserProfile, pet: PetProfile) {
        self.user = user
        self.pet = pet
    }
}
