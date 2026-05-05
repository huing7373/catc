// StepRepository.swift
// Story 8.5 AC2: 封装 POST /api/v1/steps/sync 调用；与 AuthRepository / HomeRepository 同模式.
//
// 与 HomeRepository（Story 5.5）同模式：协议方法返回原始 wire DTO；APIError 原样透传.
//
// 注入的 APIClient 是 container.apiClient —— 已被 ADR-0008 v2 AuthBoundaryAPIClient 包装；
// 业务请求 401 会自动触发全局 cold-start (清 SessionStore + 重跑 bootstrap)，StepRepository 完全无感.
//
// `DefaultStepRepository` 是 `struct`：value type，无内部状态，构造廉价；与 DefaultHomeRepository 同模式.

import Foundation

public protocol StepRepositoryProtocol: Sendable {
    /// 调 POST /api/v1/steps/sync 同步步数.
    /// - Parameter request: V1 §6.1 钦定的请求 wire DTO（syncDate / clientTotalSteps / motionState / clientTimestamp）
    /// - Returns: StepsSyncResponse（含 acceptedDeltaSteps + 最新 stepAccount 三字段）
    /// - Throws: APIError.business(1001 / 1002 / 1005 / 3001 / 1009) / APIError.network / APIError.unauthorized
    ///           / APIError.missingCredentials / APIError.decoding
    func syncSteps(_ request: StepsSyncRequest) async throws -> StepsSyncResponse
}

public struct DefaultStepRepository: StepRepositoryProtocol {
    private let apiClient: APIClientProtocol

    public init(apiClient: APIClientProtocol) {
        self.apiClient = apiClient
    }

    public func syncSteps(_ request: StepsSyncRequest) async throws -> StepsSyncResponse {
        try await apiClient.request(StepsEndpoints.sync(request))
    }
}
