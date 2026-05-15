// ChestRepository.swift
// Story 21.2 AC1: 封装 GET /api/v1/chest/current 调用；与 HomeRepository / PetRepository 同模式.
// Story 21.3 AC1: 追加 openChest(_:) 方法（POST /api/v1/chest/open）.
//
// `DefaultChestRepository` 是 `struct`：value type，无内部状态，构造廉价；
// 与 DefaultHomeRepository / DefaultPetRepository 同模式.
//
// 错误处理（与 HomeRepository 同精神）：APIError 原样透传；不在 repo 层吞错或转码.
//
// 注入的 APIClient 是 container.apiClient —— 已被 ADR-0008 v2 AuthBoundaryAPIClient 包装；
// 业务请求 401 会自动触发全局 cold-start，repo 完全无感.
//
// 设计决策（story 21.2 AC1 关键决策 3 / story 21.3 AC1 关键决策 4）：repo 返回 wire DTO，**不**做
// DTO → domain `HomeChest` 转换. 理由：与 `HomeRepository.loadHome()` 返回 `HomeResponse` 同模式；
// UseCase 层做 DTO → domain 转换（保 repo 单一职责）.
//
// **不新建 ChestOpenRepository** —— "chest" 是同一资源域，read + open 行为合并在同一 repository
// （与 StepRepository 内 syncSteps 单方法、HomeRepository 内 loadHome 单方法的"资源单一"原则不同；
// chest 是"读 + 写"双向 repository，与未来 Epic 27 装扮穿戴 CosmeticRepository.equip/unequip 双方法同精神）.

import Foundation

public protocol ChestRepositoryProtocol: Sendable {
    /// 调 GET /api/v1/chest/current 拿当前宝箱状态.
    /// - Returns: ChestCurrentResponse（5 字段：id / status / unlockAt / openCostSteps / remainingSeconds）
    /// - Throws: APIError.business(1001 / 1005 / 1009 / 4001) / APIError.network /
    ///           APIError.unauthorized / APIError.missingCredentials / APIError.decoding
    func fetchCurrent() async throws -> ChestCurrentResponse

    /// Story 21.3 AC1: 调 POST /api/v1/chest/open 开启宝箱.
    /// - Parameter request: V1 §7.2 钦定的请求 wire DTO（idempotencyKey: string，1-128 字符）
    /// - Returns: ChestOpenResponse（三段嵌套：reward + stepAccount + nextChest）
    /// - Throws: APIError.business(1001 / 1002 / 1005 / 1009 / 3002 / 4001 / 4002) /
    ///           APIError.network / APIError.unauthorized / APIError.missingCredentials /
    ///           APIError.decoding
    func openChest(_ request: ChestOpenRequest) async throws -> ChestOpenResponse
}

public struct DefaultChestRepository: ChestRepositoryProtocol {
    private let apiClient: APIClientProtocol

    public init(apiClient: APIClientProtocol) {
        self.apiClient = apiClient
    }

    public func fetchCurrent() async throws -> ChestCurrentResponse {
        try await apiClient.request(ChestEndpoints.current())
    }

    public func openChest(_ request: ChestOpenRequest) async throws -> ChestOpenResponse {
        try await apiClient.request(ChestOpenEndpoints.open(request))
    }
}
