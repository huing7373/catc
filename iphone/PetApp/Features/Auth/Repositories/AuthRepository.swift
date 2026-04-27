// AuthRepository.swift
// Story 5.2 AC3: AuthRepository 封装 /auth/* 接口调用；让 UseCase 不直接接触 APIClient.
//
// 设计：协议定义业务方法（guestLogin）；默认实装注入 APIClient；测试用 MockAuthRepository（继承 MockBase）.
//
// 失败处理：APIClient 抛的 APIError 一律原样透传，**不**在 repo 层映射成业务错误
// （UseCase / ErrorPresenter 才负责错误分诊与文案）.
//
// `DefaultAuthRepository` 是 `struct`：value type，无内部状态，构造廉价；与 `DefaultPingUseCase` 同模式.

import Foundation

public protocol AuthRepositoryProtocol: Sendable {
    /// 调 POST /api/v1/auth/guest-login。
    /// - Parameters:
    ///   - guestUid: 客户端 Keychain 持久化的游客 UID（已生成 / 已存在）
    ///   - device: 客户端设备信息（DeviceInfoProvider 提供）
    /// - Returns: GuestLoginResponse（含 token + user + pet）
    /// - Throws: APIError.business(1002 / 1005 / 1009) / APIError.network / APIError.decoding
    func guestLogin(guestUid: String, device: GuestLoginRequest.Device) async throws -> GuestLoginResponse
}

public struct DefaultAuthRepository: AuthRepositoryProtocol {
    private let apiClient: APIClientProtocol

    public init(apiClient: APIClientProtocol) {
        self.apiClient = apiClient
    }

    public func guestLogin(guestUid: String, device: GuestLoginRequest.Device) async throws -> GuestLoginResponse {
        let req = GuestLoginRequest(guestUid: guestUid, device: device)
        return try await apiClient.request(AuthEndpoints.guestLogin(request: req))
    }
}
