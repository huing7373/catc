// EmojiRepository.swift
// Story 18.1 AC1: Emoji REST 仓储层 (封装 GET /api/v1/emojis 调用).
//
// 与 HomeRepository / RoomRepository 同模式：协议方法返回 domain `[EmojiConfig]`；APIError 原样透传.
//
// 注入的 APIClient 是 container.apiClient —— 已被 ADR-0008 v2 AuthBoundaryAPIClient 包装；
// 业务请求 401 自动触发**全局 cold-start** (清 SessionStore + 重跑 bootstrap).
//
// `DefaultEmojiRepository` 是 `struct`：value type，无内部状态，构造廉价；与 DefaultHomeRepository
// / DefaultRoomRepository 同模式.
//
// **不**在 Repository 层包错 / 转码 / 二次排序 —— server 17.4 钦定 ORDER BY sort_order ASC, id ASC
// 已保证顺序；Repository 只负责 wire → domain 直通.

import Foundation

public protocol EmojiRepositoryProtocol: Sendable {
    /// 调 GET /api/v1/emojis 拿表情列表.
    /// - Returns: `[EmojiConfig]` —— 已按 server `ORDER BY sort_order ASC, id ASC` 排好;
    ///   空列表语义为合法 (server 永远返 `items: []` 而非 `null`).
    /// - Throws: APIError.business(1001 / 1005 / 1009) / APIError.network / APIError.unauthorized
    ///           / APIError.missingCredentials / APIError.decoding
    func listEmojis() async throws -> [EmojiConfig]
}

public struct DefaultEmojiRepository: EmojiRepositoryProtocol {
    private let apiClient: APIClientProtocol

    public init(apiClient: APIClientProtocol) {
        self.apiClient = apiClient
    }

    public func listEmojis() async throws -> [EmojiConfig] {
        let response: EmojiListResponse = try await apiClient.request(EmojisEndpoints.listEmojis())
        return response.items
    }
}
