// ChestRepository.swift
// Story 21.2 AC1: 封装 GET /api/v1/chest/current 调用；与 HomeRepository / PetRepository 同模式.
//
// `DefaultChestRepository` 是 `struct`：value type，无内部状态，构造廉价；
// 与 DefaultHomeRepository / DefaultPetRepository 同模式.
//
// 错误处理（与 HomeRepository 同精神）：APIError 原样透传；不在 repo 层吞错或转码.
//
// 注入的 APIClient 是 container.apiClient —— 已被 ADR-0008 v2 AuthBoundaryAPIClient 包装；
// 业务请求 401 会自动触发全局 cold-start，repo 完全无感.
//
// 设计决策（story AC1 关键决策 3）：repo 返回 wire DTO `ChestCurrentResponse`，**不**做
// DTO → domain `HomeChest` 转换. 理由：与 `HomeRepository.loadHome()` 返回 `HomeResponse` 同模式；
// UseCase 层做 DTO → domain 转换（保 repo 单一职责）.

import Foundation

public protocol ChestRepositoryProtocol: Sendable {
    /// 调 GET /api/v1/chest/current 拿当前宝箱状态.
    /// - Returns: ChestCurrentResponse（5 字段：id / status / unlockAt / openCostSteps / remainingSeconds）
    /// - Throws: APIError.business(1001 / 1005 / 1009 / 4001) / APIError.network /
    ///           APIError.unauthorized / APIError.missingCredentials / APIError.decoding
    func fetchCurrent() async throws -> ChestCurrentResponse
}

public struct DefaultChestRepository: ChestRepositoryProtocol {
    private let apiClient: APIClientProtocol

    public init(apiClient: APIClientProtocol) {
        self.apiClient = apiClient
    }

    public func fetchCurrent() async throws -> ChestCurrentResponse {
        try await apiClient.request(ChestEndpoints.current())
    }
}
