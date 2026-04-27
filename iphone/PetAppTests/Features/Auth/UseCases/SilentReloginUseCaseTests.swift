// SilentReloginUseCaseTests.swift
// Story 5.4 AC6: SilentReloginUseCase 单元测试.
// 复用 MockKeychainStore（Story 2.8）+ MockAuthRepository（Story 5.2），不新建 mock.

import XCTest
@testable import PetApp

#if DEBUG

@MainActor
final class SilentReloginUseCaseTests: XCTestCase {

    nonisolated private static func makeDevice() -> GuestLoginRequest.Device {
        GuestLoginRequest.Device(platform: "ios", appVersion: "1.0.0", deviceModel: "iPhone15,2")
    }

    private func makeStubResponse(token: String = "new-token-1") -> GuestLoginResponse {
        GuestLoginResponse(
            token: token,
            user: UserProfile(id: "1001", nickname: "用户1001", avatarUrl: "", hasBoundWechat: false),
            pet: PetProfile(id: "2001", petType: 1, name: "默认小猫")
        )
    }

    // MARK: - case#1 (happy)：keychain 有 guestUid → repo 成功 → 写新 token → 返回新 token
    func testExecuteRevolvesExistingGuestUidAndReturnsNewToken() async throws {
        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("existing-uid-abc")
        let repo = MockAuthRepository()
        repo.guestLoginStub = .success(makeStubResponse(token: "new-token-1"))

        let useCase = DefaultSilentReloginUseCase(
            keychainStore: keychain,
            repository: repo,
            deviceProvider: { Self.makeDevice() }
        )

        let token = try await useCase.execute()

        XCTAssertEqual(token, "new-token-1")
        XCTAssertEqual(keychain.callCount(of: "get(forKey:)"), 1, "应读 guestUid 一次")
        XCTAssertEqual(keychain.callCount(of: "set(_:forKey:)"), 1, "应写 token 一次（不写 guestUid）")
        XCTAssertEqual(repo.callCount(of: "guestLogin(guestUid:device:)"), 1)
        XCTAssertEqual(repo.lastGuestUid, "existing-uid-abc", "必须复用既有 guestUid")
    }

    // MARK: - case#2 (edge)：keychain 无 guestUid → 抛 unauthorized + 不调 repo
    func testThrowsUnauthorizedWhenGuestUidMissing() async {
        let keychain = MockKeychainStore()
        keychain.getStubResult = .success(nil)
        let repo = MockAuthRepository()

        let useCase = DefaultSilentReloginUseCase(
            keychainStore: keychain,
            repository: repo,
            deviceProvider: { Self.makeDevice() }
        )

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际抛 \(error)")
        }

        XCTAssertEqual(repo.callCount(of: "guestLogin(guestUid:device:)"), 0, "无 guestUid 时**绝不**调 repo")
        XCTAssertEqual(keychain.callCount(of: "set(_:forKey:)"), 0, "也不该写 token")
    }

    // MARK: - case#3 (edge)：keychain 无 guestUid（空字符串视同）→ 抛 unauthorized
    func testThrowsUnauthorizedWhenGuestUidIsEmptyString() async {
        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("")
        let repo = MockAuthRepository()

        let useCase = DefaultSilentReloginUseCase(
            keychainStore: keychain,
            repository: repo
        )

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际抛 \(error)")
        }

        XCTAssertEqual(repo.callCount(of: "guestLogin(guestUid:device:)"), 0)
    }

    // MARK: - case#4 (edge)：repo.guestLogin 失败 → 透传 APIError + 不写 token
    func testPropagatesRepoErrorAndDoesNotWriteToken() async {
        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("existing-uid-abc")
        let repo = MockAuthRepository()
        repo.guestLoginStub = .failure(.network(underlying: URLError(.notConnectedToInternet)))

        let useCase = DefaultSilentReloginUseCase(
            keychainStore: keychain,
            repository: repo
        )

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.network")
        } catch let error as APIError {
            // 用 case 模式比较：.network 嵌的 underlying 不强校验
            if case .network = error {
                // ok
            } else {
                XCTFail("应抛 .network，实际抛 \(error)")
            }
        } catch {
            XCTFail("应抛 APIError.network，实际抛 \(error)")
        }

        XCTAssertEqual(repo.callCount(of: "guestLogin(guestUid:device:)"), 1)
        XCTAssertEqual(keychain.callCount(of: "set(_:forKey:)"), 0, "repo 失败时绝不写 token")
    }

    // MARK: - case#5 (edge)：keychain.get 抛错 → 透传 KeychainError
    func testPropagatesKeychainGetError() async {
        let keychain = MockKeychainStore()
        keychain.getStubResult = .failure(KeychainError.osStatus(-25300, operation: "get"))
        let repo = MockAuthRepository()

        let useCase = DefaultSilentReloginUseCase(
            keychainStore: keychain,
            repository: repo
        )

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 KeychainError")
        } catch let error as KeychainError {
            // ok
            if case .osStatus(let status, _) = error {
                XCTAssertEqual(status, -25300)
            } else {
                XCTFail("应抛 .osStatus(-25300, ...)，实际抛 \(error)")
            }
        } catch {
            XCTFail("应抛 KeychainError，实际抛 \(error)")
        }

        XCTAssertEqual(repo.callCount(of: "guestLogin(guestUid:device:)"), 0, "keychain 出错时不应继续")
    }

    // MARK: - case#6 (edge)：keychain.set token 失败 → 透传 KeychainError + repo 已调过
    func testPropagatesKeychainSetError() async {
        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("existing-uid-abc")
        keychain.setStubError = KeychainError.osStatus(-25299, operation: "set")
        let repo = MockAuthRepository()
        repo.guestLoginStub = .success(makeStubResponse(token: "new-token-1"))

        let useCase = DefaultSilentReloginUseCase(
            keychainStore: keychain,
            repository: repo
        )

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 KeychainError")
        } catch is KeychainError {
            // ok
        } catch {
            XCTFail("应抛 KeychainError，实际抛 \(error)")
        }

        XCTAssertEqual(repo.callCount(of: "guestLogin(guestUid:device:)"), 1, "set 失败前 repo 已调过")
    }
}

#endif
