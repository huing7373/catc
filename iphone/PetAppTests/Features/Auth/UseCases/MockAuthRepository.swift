// MockAuthRepository.swift
// Story 5.2 AC9: AuthRepositoryProtocol 测试 mock；继承 MockBase（Story 2.7 落地）.
//
// stub 字段（guestLoginStub）由测试 setUp 阶段写入；method body 读取一次后立即用，
// 符合 MockBase snapshot-only 精神（lesson 2026-04-26-mockbase-snapshot-only-reads.md）.
//
// `lastGuestUid` / `lastDevice` 是便利 snapshot 字段：让测试断言 "上次调用传的什么".
// 它们是 private(set) + lock 保护，与 MockBase.lastArguments 同精神，但更类型安全（无需 cast Any）.

@testable import PetApp
import Foundation

#if DEBUG

final class MockAuthRepository: MockBase, AuthRepositoryProtocol, @unchecked Sendable {
    var guestLoginStub: Result<GuestLoginResponse, APIError> = .failure(.network(underlying: URLError(.unknown)))

    private let argumentsLock = NSLock()
    private var _lastGuestUid: String?
    private var _lastDevice: GuestLoginRequest.Device?

    /// 最近一次 guestLogin 调用的 guestUid 参数（便于断言）.
    var lastGuestUid: String? {
        argumentsLock.lock()
        defer { argumentsLock.unlock() }
        return _lastGuestUid
    }

    /// 最近一次 guestLogin 调用的 device 参数（便于断言）.
    var lastDevice: GuestLoginRequest.Device? {
        argumentsLock.lock()
        defer { argumentsLock.unlock() }
        return _lastDevice
    }

    func guestLogin(guestUid: String, device: GuestLoginRequest.Device) async throws -> GuestLoginResponse {
        record(method: "guestLogin(guestUid:device:)", arguments: [guestUid, device])
        argumentsLock.lock()
        _lastGuestUid = guestUid
        _lastDevice = device
        argumentsLock.unlock()
        return try guestLoginStub.get()
    }
}

#endif
