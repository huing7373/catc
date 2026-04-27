// GuestLoginUseCaseTests.swift
// Story 5.2 AC9: DefaultGuestLoginUseCase 单元测试.
//
// 测试基础设施约束（与 Story 2.7 衔接）：
// - 仅 stdlib（XCTest + @testable import PetApp）
// - MockKeychainStore（Story 2.8 落地，继承 MockBase）+ MockAuthRepository（本 story 新建，继承 MockBase）
// - **不**用 KeychainServicesStore（那是真实 keychain，单测层不接触；集成测试 GuestLoginUseCaseIntegrationTests 用真实 store）.

import XCTest
@testable import PetApp

#if DEBUG

@MainActor
final class GuestLoginUseCaseTests: XCTestCase {

    // MARK: - Helpers

    // 注：标 `nonisolated static` 让闭包（@Sendable 上下文）能直接调用而不跨 actor.
    nonisolated private static func makeDevice() -> GuestLoginRequest.Device {
        GuestLoginRequest.Device(platform: "ios", appVersion: "1.0.0", deviceModel: "iPhone15,2")
    }

    private func makeStubResponse(token: String = "test-token", userId: String = "1001") -> GuestLoginResponse {
        GuestLoginResponse(
            token: token,
            user: UserProfile(id: userId, nickname: "用户\(userId)", avatarUrl: "", hasBoundWechat: false),
            pet: PetProfile(id: "2001", petType: 1, name: "默认小猫")
        )
    }

    // MARK: - case#1 (happy)：无 guestUid → 生成 UUID → 写 keychain → 调 API → 写 token → 返回 user/pet

    func testExecuteGeneratesNewGuestUidWhenAbsent() async throws {
        let keychain = MockKeychainStore()
        let repo = MockAuthRepository()
        repo.guestLoginStub = .success(makeStubResponse())

        let useCase = DefaultGuestLoginUseCase(
            keychainStore: keychain,
            repository: repo,
            uuidGenerator: { "fixed-uuid-1" },
            deviceProvider: { Self.makeDevice() }
        )

        let output = try await useCase.execute()

        // 1. keychain.get(guestUid) 调过一次（确认是否已存在）
        // 2. keychain.set(guestUid="fixed-uuid-1") 调过一次
        // 3. repo.guestLogin(guestUid="fixed-uuid-1", device=stub) 调过一次
        // 4. keychain.set(authToken="test-token") 调过一次
        XCTAssertEqual(keychain.callCount(of: "get(forKey:)"), 1)
        XCTAssertEqual(keychain.callCount(of: "set(_:forKey:)"), 2, "应 set guestUid 与 token 各一次")
        XCTAssertEqual(repo.callCount(of: "guestLogin(guestUid:device:)"), 1)
        XCTAssertEqual(repo.lastGuestUid, "fixed-uuid-1")
        XCTAssertEqual(output.user.id, "1001")
        XCTAssertEqual(output.pet.id, "2001")
    }

    // MARK: - case#2 (happy)：已有 guestUid → 直接调 API → 拿同 user_id（mock 返固定）

    func testExecuteReusesExistingGuestUid() async throws {
        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("existing-uid-abc")
        let repo = MockAuthRepository()
        repo.guestLoginStub = .success(makeStubResponse(userId: "1001"))

        let useCase = DefaultGuestLoginUseCase(
            keychainStore: keychain,
            repository: repo,
            uuidGenerator: { XCTFail("不应生成新 UUID"); return "should-not-be-called" },
            deviceProvider: { Self.makeDevice() }
        )

        let output = try await useCase.execute()

        // 已存在时只 set token 一次（不 set guestUid）
        XCTAssertEqual(keychain.callCount(of: "set(_:forKey:)"), 1, "已有 guestUid 时只 set token")
        XCTAssertEqual(repo.lastGuestUid, "existing-uid-abc")
        XCTAssertEqual(output.user.id, "1001")
    }

    // MARK: - case#3 (edge)：APIClient 网络失败 → UseCase 抛 APIError.network

    func testExecuteThrowsNetworkErrorWhenAPIFails() async {
        let keychain = MockKeychainStore()
        let repo = MockAuthRepository()
        repo.guestLoginStub = .failure(.network(underlying: URLError(.notConnectedToInternet)))

        let useCase = DefaultGuestLoginUseCase(
            keychainStore: keychain,
            repository: repo,
            uuidGenerator: { "fixed-uuid-2" },
            deviceProvider: { Self.makeDevice() }
        )

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.network")
        } catch let error as APIError {
            XCTAssertEqual(error, .network(underlying: URLError(.notConnectedToInternet)))
        } catch {
            XCTFail("应抛 APIError，实际抛 \(error)")
        }

        // API 失败时 keychain.guestUid 已写入，但 token 未写
        XCTAssertEqual(keychain.callCount(of: "set(_:forKey:)"), 1, "仅写 guestUid，不写 token")
    }

    // MARK: - case#4 (edge)：APIClient 业务错误（1009 服务繁忙）→ UseCase 抛 APIError.business

    func testExecuteThrowsBusinessErrorWhenAPIBusiness() async {
        let keychain = MockKeychainStore()
        let repo = MockAuthRepository()
        repo.guestLoginStub = .failure(.business(code: 1009, message: "服务繁忙", requestId: "req_x"))

        let useCase = DefaultGuestLoginUseCase(
            keychainStore: keychain,
            repository: repo,
            uuidGenerator: { "fixed-uuid-3" },
            deviceProvider: { Self.makeDevice() }
        )

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.business")
        } catch let error as APIError {
            if case .business(let code, _, _) = error {
                XCTAssertEqual(code, 1009)
            } else {
                XCTFail("应是 .business，实际 \(error)")
            }
        } catch {
            XCTFail("应抛 APIError，实际抛 \(error)")
        }
    }

    // MARK: - case#5 (edge)：Keychain write 失败 → UseCase 抛 KeychainError；不调 API

    func testExecuteThrowsKeychainErrorWhenWriteGuestUidFails() async {
        let keychain = MockKeychainStore()
        keychain.setStubError = KeychainError.osStatus(-25300, operation: "set.add")  // errSecItemNotFound 模拟
        let repo = MockAuthRepository()
        repo.guestLoginStub = .success(makeStubResponse())

        let useCase = DefaultGuestLoginUseCase(
            keychainStore: keychain,
            repository: repo,
            uuidGenerator: { "fixed-uuid-4" },
            deviceProvider: { Self.makeDevice() }
        )

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 KeychainError")
        } catch is KeychainError {
            // ok
        } catch {
            XCTFail("应抛 KeychainError，实际抛 \(error)")
        }

        // keychain.set 抛错 → repo.guestLogin 不应被调
        XCTAssertEqual(repo.callCount(of: "guestLogin(guestUid:device:)"), 0,
                       "keychain set guestUid 失败后，API 不应被调")
    }

    // MARK: - case#6 (edge)：API 成功但 keychain write token 失败 → 抛 KeychainError

    func testExecuteThrowsWhenWriteTokenFails() async {
        let keychain = MockKeychainStore()
        // 用 trick：existing guestUid 让 set guestUid 分支不进入；set 一律抛错（即 set token 时抛）.
        keychain.getStubResult = .success("existing-uid")
        keychain.setStubError = KeychainError.osStatus(-25291, operation: "set.add")
        let repo = MockAuthRepository()
        repo.guestLoginStub = .success(makeStubResponse())

        let useCase = DefaultGuestLoginUseCase(
            keychainStore: keychain,
            repository: repo,
            uuidGenerator: { XCTFail("不应生成 UUID"); return "x" },
            deviceProvider: { Self.makeDevice() }
        )

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 KeychainError")
        } catch is KeychainError {
            // ok
        } catch {
            XCTFail("应抛 KeychainError，实际抛 \(error)")
        }

        // API 调用过；keychain set token 失败抛错
        XCTAssertEqual(repo.callCount(of: "guestLogin(guestUid:device:)"), 1)
    }

    // MARK: - case#7 (edge)：existing guestUid 是空串 → 视为不存在 → 重新生成 UUID

    func testExecuteRegeneratesUidWhenExistingIsEmpty() async throws {
        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("")  // 空串视为不存在
        let repo = MockAuthRepository()
        repo.guestLoginStub = .success(makeStubResponse())

        let useCase = DefaultGuestLoginUseCase(
            keychainStore: keychain,
            repository: repo,
            uuidGenerator: { "regenerated-uuid" },
            deviceProvider: { Self.makeDevice() }
        )

        _ = try await useCase.execute()

        XCTAssertEqual(repo.lastGuestUid, "regenerated-uuid")
        XCTAssertEqual(keychain.callCount(of: "set(_:forKey:)"), 2, "空串 → 重新生成 UUID 写 keychain，再写 token")
    }
}

#endif
