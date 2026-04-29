// HomeRepository.swift
// Story 5.5 AC3: HomeRepository 封装 GET /api/v1/home 调用；让 UseCase 不直接接触 APIClient.
//
// 与 AuthRepository（Story 5.2）同模式：协议方法返回原始 wire DTO；APIError 原样透传.
//
// 注入的 APIClient 是 container.apiClient —— 已被 ADR-0008 v2 AuthBoundaryAPIClient 包装；
// 业务请求 401 会自动触发全局 cold-start (清 SessionStore + 重跑 bootstrap)，HomeRepository 完全无感.
//
// `DefaultHomeRepository` 是 `struct`：value type，无内部状态，构造廉价；与 DefaultAuthRepository 同模式.

import Foundation

public protocol HomeRepositoryProtocol: Sendable {
    /// 调 GET /api/v1/home 拿首屏数据.
    /// - Returns: HomeResponse（含 user / pet / stepAccount / chest / room）
    /// - Throws: APIError.business(1001 / 1009) / APIError.network / APIError.unauthorized
    ///           / APIError.missingCredentials / APIError.decoding
    func loadHome() async throws -> HomeResponse
}

public struct DefaultHomeRepository: HomeRepositoryProtocol {
    private let apiClient: APIClientProtocol

    public init(apiClient: APIClientProtocol) {
        self.apiClient = apiClient
    }

    public func loadHome() async throws -> HomeResponse {
        try await apiClient.request(HomeEndpoints.loadHome())
    }
}
