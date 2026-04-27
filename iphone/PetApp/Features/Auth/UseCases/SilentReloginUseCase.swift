// SilentReloginUseCase.swift
// Story 5.4 AC1: 静默重登 UseCase —— "复用已有 guestUid 重新拿 token" 语义.
//
// 与 Story 5.2 GuestLoginUseCase 的区别：
//   - GuestLoginUseCase: cold-start 路径，会 generate-or-read guestUid + 写 SessionStore
//   - SilentReloginUseCase: 稳态恢复路径，仅 read 已有 guestUid（无则失败，绝不 generate）+ 不动 SessionStore
//
// 流程：
//   1. keychain.get(KeychainKey.guestUid.rawValue) —— 读现有 UID
//   2. nil / 空字符串 → throw APIError.unauthorized（无身份可恢复 —— 让业务层 fallback 到 cold-start 流）
//   3. 调 repo.guestLogin(guestUid: uid, device: deviceProvider())
//   4. 成功 → keychain.set(response.token, KeychainKey.authToken.rawValue) 写新 token
//   5. 返回新 token（让上层 AuthRetryingAPIClient 不必再读 keychain 一次即可重试原请求）
//
// 错误处理：
//   - keychain.get 失败 → 透传 KeychainError 上层（不吞错；让业务层 / coordinator 看到本 root cause）
//   - keychain.set 失败 → 透传 KeychainError；调用方收到错误后**不**重试原请求（半成功状态：server 已发新 token 但本地未存）
//   - repo.guestLogin 失败 → 透传 APIError；keychain 不动（已有的旧 token 仍在；下一次 401 会再触发重登）
//
// 不在本 story 范围：
//   - 不调 SessionStore.updateSession（详见 story 非范围 §5）
//   - 不调 GuestLoginUseCase（避免 generate 新 guestUid 覆盖既有身份）
//   - 不做 retry / 指数退避（重登失败一次就抛；上层决定是否重试）

import Foundation

public protocol SilentReloginUseCaseProtocol: Sendable {
    /// 复用 Keychain 中现有 guestUid 调 /auth/guest-login 拿新 token + 写 keychain.
    /// - Returns: 新 token（已写 keychain，调用方可直接用以重试原请求）
    /// - Throws:
    ///   - APIError.unauthorized: keychain 中无 guestUid（无身份可恢复）
    ///   - KeychainError: keychain 读 / 写失败
    ///   - APIError.network / .business / .decoding: /auth/guest-login 调用失败
    func execute() async throws -> String
}

public struct DefaultSilentReloginUseCase: SilentReloginUseCaseProtocol {
    private let keychainStore: KeychainStoreProtocol
    private let repository: AuthRepositoryProtocol
    private let deviceProvider: @Sendable () -> GuestLoginRequest.Device

    public init(
        keychainStore: KeychainStoreProtocol,
        repository: AuthRepositoryProtocol,
        deviceProvider: @escaping @Sendable () -> GuestLoginRequest.Device = { DeviceInfoProvider.current() }
    ) {
        self.keychainStore = keychainStore
        self.repository = repository
        self.deviceProvider = deviceProvider
    }

    public func execute() async throws -> String {
        // Step 1: 读已有 guestUid
        let existing = try keychainStore.get(forKey: KeychainKey.guestUid.rawValue)

        // Step 2: 无 guestUid → 不能"假装重登"——必须走 cold-start，故抛 unauthorized
        guard let guestUid = existing, !guestUid.isEmpty else {
            throw APIError.unauthorized
        }

        // Step 3: 调 /auth/guest-login（requiresAuth=false）
        let device = deviceProvider()
        let response = try await repository.guestLogin(guestUid: guestUid, device: device)

        // Step 4: 写新 token（覆盖旧 token）
        try keychainStore.set(response.token, forKey: KeychainKey.authToken.rawValue)

        // Step 5: 返回新 token
        return response.token
    }
}
