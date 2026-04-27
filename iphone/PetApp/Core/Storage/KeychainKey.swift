// KeychainKey.swift
// Story 5.1 AC1: Keychain 已知 key 常量化。
//
// 设计：raw value 即真实 keychain account 字段；用 `auth.<name>` namespace
// 避免未来若引入业务存储（如 `cache.lastHomeSnapshot`）时与 auth 类 key 撞名。
//
// 已知 key 列表（节点 2 全集）：
// - guestUid: 客户端持久化的游客身份 UID（写入时机：Story 5.2 GuestLoginUseCase 首次启动生成 UUID v4）
// - authToken: server 签发的 JWT token（写入时机：Story 5.2 调 /auth/guest-login 拿到 token 后）
//
// 节点 2 之后可能扩展（不属本 story scope）：
// - refreshToken（如未来引入 refresh token 流，目前 /auth/guest-login 设计上幂等，不需要）

import Foundation

public enum KeychainKey: String, CaseIterable, Sendable {
    case guestUid = "auth.guestUid"
    case authToken = "auth.token"
}
