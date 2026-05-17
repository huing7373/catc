// InventoryRepository.swift
// Story 24.2 AC2: 封装 GET /api/v1/cosmetics/inventory 调用；与 EmojiRepository /
// ChestRepository 同模式.
//
// `DefaultInventoryRepository` 是 `struct`：value type，无内部状态，构造廉价；
// 与 DefaultEmojiRepository / DefaultChestRepository 同模式.
//
// 错误处理（与 EmojiRepository / ChestRepository 同精神）：APIError **原样透传**；
// 不在 repo 层吞错 / 转码 / 二次排序.
//
// 注入的 APIClient 是 container.apiClient —— 已被 ADR-0008 v2 AuthBoundaryAPIClient 包装；
// 业务请求 401 会自动触发全局 cold-start，repo 完全无感.
//
// 设计决策（与 ChestRepository / EmojiRepository 同模式）：repo 返回 wire DTO
// `InventoryResponse`，**不**做 DTO → domain 转换 / 排序 / 去重. 理由：V1 §8.2 步骤 5
// server 端已保证 groups[] / instances[] 两级确定性全序；repo 只负责 wire → DTO 直通,
// DTO → domain 展平由 LoadInventoryUseCase 做（保 repo 单一职责）.

import Foundation

public protocol InventoryRepositoryProtocol: Sendable {
    /// 调 GET /api/v1/cosmetics/inventory 拿背包聚合 + 实例列表.
    /// - Returns: InventoryResponse（`{groups: [InventoryGroup]}`；空背包为 `groups: []`）
    /// - Throws: APIError.business(1001 / 1005 / 1009) / APIError.network /
    ///           APIError.unauthorized / APIError.missingCredentials / APIError.decoding
    func fetchInventory() async throws -> InventoryResponse
}

public struct DefaultInventoryRepository: InventoryRepositoryProtocol {
    private let apiClient: APIClientProtocol

    public init(apiClient: APIClientProtocol) {
        self.apiClient = apiClient
    }

    public func fetchInventory() async throws -> InventoryResponse {
        try await apiClient.request(InventoryEndpoints.inventory())
    }
}
