// PetRepository.swift
// Story 15.4 AC1: 封装 POST /api/v1/pets/current/state-sync 调用；与 StepRepository / HomeRepository
// 同模式（value type struct + apiClient.request 转发；APIError 原样透传）.
//
// 注入的 APIClient 是 container.apiClient —— 已被 ADR-0008 v2 AuthBoundaryAPIClient 包装；
// 业务请求 401 会自动触发全局 cold-start，repo 完全无感.
//
// **不**做的事（红线，AC1 段钦定）：
//   - 不做 retry / 节流 / roomId guard（这些是 UseCase / TriggerService 的责任）
//   - 不做幂等 header（V1 §5.2 line 500：state-sync 不消耗资产）
//   - 不动 AppState（repo 层不感知 domain state）
//
// 与 DefaultStepRepository 一致：`struct` value type，无内部状态，构造廉价.

import Foundation

public protocol PetRepositoryProtocol: Sendable {
    /// 调 POST /api/v1/pets/current/state-sync 同步当前 pet 状态.
    /// - Parameter request: V1 §5.2 钦定的请求 wire DTO（单字段 state: int ∈ {1,2,3}）
    /// - Returns: PetStateSyncResponse（server-acknowledged echo state，仅作 ack 信号 —— **禁止**驱动 UI）
    /// - Throws: APIError.business / APIError.network / APIError.unauthorized / APIError.decoding
    func syncPetState(_ request: PetStateSyncRequest) async throws -> PetStateSyncResponse
}

public struct DefaultPetRepository: PetRepositoryProtocol {
    private let apiClient: APIClientProtocol

    public init(apiClient: APIClientProtocol) {
        self.apiClient = apiClient
    }

    public func syncPetState(_ request: PetStateSyncRequest) async throws -> PetStateSyncResponse {
        try await apiClient.request(PetStateEndpoints.sync(request))
    }
}
