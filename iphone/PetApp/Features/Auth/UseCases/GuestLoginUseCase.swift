// GuestLoginUseCase.swift
// Story 5.2 AC6: 启动自动登录核心 UseCase.
//
// 流程（顺序串行，前后步骤依赖）：
//   1. keychain.get(KeychainKey.guestUid.rawValue) —— 读已存在 UID
//   2. nil → 生成 UUID v4 字符串（uuidGenerator() —— 注入式，便于测试 stub）
//   3. nil 时立即 keychain.set(uid, KeychainKey.guestUid.rawValue) —— 先写本地保证下次启动复用
//   4. 调 repo.guestLogin(guestUid: uid, device: deviceProvider())
//   5. 成功 → keychain.set(token, KeychainKey.authToken.rawValue) —— 写 token
//   6. 返回 GuestLoginOutput(user, pet)（**不**返回 token —— token 只在 keychain，Story 5.3 interceptor 自动注入）
//
// 失败处理：所有错误**原样**透传 throw；不在 UseCase 内吞错或转码。
// - keychain read 失败 → throws KeychainError（极少见）
// - keychain write guestUid 失败 → throws KeychainError；不调 API（继续无意义）
// - API 调用失败 → throws APIError（network / business / unauthorized / decoding）
//   失败时**不**回滚 keychain.guestUid（已写的 guestUid 下次启动会复用，server 那边没 binding 就当首次创建）
// - keychain write token 失败 → throws KeychainError；UseCase 不返回成功 output
//   （避免 "server 已有 user 但 client 没 token" 半成功状态；下次重试同 guestUid 再走一遍）
//
// 不在本 story 范围：
// - 不调 SessionStore.updateSession() —— 那是 RootView bootstrapStep1 closure 的责任
//   （keep UseCase 纯：input → output；side effect 收敛到 closure 注入点）
// - 不做 retry / 静默重登 —— 归 Story 5.4

import Foundation

public protocol GuestLoginUseCaseProtocol: Sendable {
    func execute() async throws -> GuestLoginOutput
}

public struct GuestLoginOutput: Equatable, Sendable {
    public let user: UserProfile
    public let pet: PetProfile

    public init(user: UserProfile, pet: PetProfile) {
        self.user = user
        self.pet = pet
    }
}

public struct DefaultGuestLoginUseCase: GuestLoginUseCaseProtocol {
    private let keychainStore: KeychainStoreProtocol
    private let repository: AuthRepositoryProtocol
    private let uuidGenerator: @Sendable () -> String
    private let deviceProvider: @Sendable () -> GuestLoginRequest.Device

    public init(
        keychainStore: KeychainStoreProtocol,
        repository: AuthRepositoryProtocol,
        uuidGenerator: @escaping @Sendable () -> String = { UUID().uuidString },
        deviceProvider: @escaping @Sendable () -> GuestLoginRequest.Device = { DeviceInfoProvider.current() }
    ) {
        self.keychainStore = keychainStore
        self.repository = repository
        self.uuidGenerator = uuidGenerator
        self.deviceProvider = deviceProvider
    }

    public func execute() async throws -> GuestLoginOutput {
        // Step 1: 读已存在 guestUid
        let existing = try keychainStore.get(forKey: KeychainKey.guestUid.rawValue)

        // Step 2-3: 不存在 / 空串 → 生成 + 写入
        let guestUid: String
        if let existing, !existing.isEmpty {
            guestUid = existing
        } else {
            guestUid = uuidGenerator()
            try keychainStore.set(guestUid, forKey: KeychainKey.guestUid.rawValue)
        }

        // Step 4: 调 API
        let device = deviceProvider()
        let response = try await repository.guestLogin(guestUid: guestUid, device: device)

        // Step 5: 写 token
        try keychainStore.set(response.token, forKey: KeychainKey.authToken.rawValue)

        // Step 6: 返回（不含 token —— 单点持有在 keychain，Story 5.3 interceptor 注入）
        return GuestLoginOutput(user: response.user, pet: response.pet)
    }
}
