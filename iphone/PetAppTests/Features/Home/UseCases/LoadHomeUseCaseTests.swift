// LoadHomeUseCaseTests.swift
// Story 5.5 AC9: LoadHomeUseCase 单测覆盖（≥ 5 case）.
//
// 测试目标：验证 UseCase = repo.loadHome() → HomeData(from:) → 返回 / 透传错误.
// 用 MockHomeRepository（继承 MockBase + loadHomeStub: Result<HomeResponse, Error>）.

import XCTest
@testable import PetApp

@MainActor
final class LoadHomeUseCaseTests: XCTestCase {

    // MARK: - Helpers

    private func makeFullResponse(
        userId: String = "10001",
        nickname: String = "用户10001",
        avatarUrl: String = "",
        pet: HomePetDTO? = nil,
        stepAccount: StepAccountDTO = StepAccountDTO(totalSteps: 0, availableSteps: 0, consumedSteps: 0),
        chest: ChestDTO? = nil,
        room: RoomDTO = RoomDTO(currentRoomId: nil)
    ) -> HomeResponse {
        HomeResponse(
            user: UserInfoDTO(id: userId, nickname: nickname, avatarUrl: avatarUrl),
            pet: pet ?? Self.defaultPet(),
            stepAccount: stepAccount,
            chest: chest ?? Self.defaultChest(),
            room: room
        )
    }

    /// 与 makeFullResponse 同精神，但允许 caller 显式传 pet=nil 来构造 nil-pet 场景.
    private func makeFullResponseAllowingNilPet(
        pet: HomePetDTO?,
        chest: ChestDTO? = nil,
        room: RoomDTO = RoomDTO(currentRoomId: nil)
    ) -> HomeResponse {
        HomeResponse(
            user: UserInfoDTO(id: "10001", nickname: "用户10001", avatarUrl: ""),
            pet: pet,
            stepAccount: StepAccountDTO(totalSteps: 0, availableSteps: 0, consumedSteps: 0),
            chest: chest ?? Self.defaultChest(),
            room: room
        )
    }

    nonisolated static func defaultPet(currentState: Int = 1, equips: [EquipDTO] = []) -> HomePetDTO {
        HomePetDTO(
            id: "20001",
            petType: 1,
            name: "默认小猫",
            currentState: currentState,
            equips: equips
        )
    }

    nonisolated static func defaultChest(status: Int = 1, remainingSeconds: Int = 600) -> ChestDTO {
        ChestDTO(
            id: "30001",
            status: status,
            unlockAt: Date(timeIntervalSince1970: 1_800_000_000),
            openCostSteps: 100,
            remainingSeconds: remainingSeconds
        )
    }

    // MARK: - case#1 happy: 完整响应 → 字段一一对应

    func testExecuteHappyPathFullResponse() async throws {
        let mock = MockHomeRepository()
        mock.loadHomeStub = .success(makeFullResponse())
        let useCase = DefaultLoadHomeUseCase(repository: mock)

        let data = try await useCase.execute()

        XCTAssertEqual(data.user.id, "10001")
        XCTAssertEqual(data.user.nickname, "用户10001")
        XCTAssertEqual(data.pet?.name, "默认小猫")
        XCTAssertEqual(data.pet?.currentState, .rest)
        XCTAssertEqual(data.stepAccount.availableSteps, 0)
        XCTAssertEqual(data.chest.status, .counting)
        XCTAssertEqual(data.chestRemainingDisplay, "10:00")
        XCTAssertNil(data.room.currentRoomId)
        XCTAssertEqual(mock.callCount(of: "loadHome()"), 1)
    }

    // MARK: - case#2 happy edge: pet=nil

    func testExecuteWithNilPet() async throws {
        let mock = MockHomeRepository()
        mock.loadHomeStub = .success(makeFullResponseAllowingNilPet(pet: nil))
        let useCase = DefaultLoadHomeUseCase(repository: mock)

        let data = try await useCase.execute()

        XCTAssertNil(data.pet)
        XCTAssertEqual(data.user.id, "10001")
    }

    // MARK: - case#3 happy edge: pet.equips=[]

    func testExecuteWithEmptyEquips() async throws {
        let mock = MockHomeRepository()
        mock.loadHomeStub = .success(makeFullResponse(pet: LoadHomeUseCaseTests.defaultPet(equips: [])))
        let useCase = DefaultLoadHomeUseCase(repository: mock)

        let data = try await useCase.execute()

        XCTAssertEqual(data.pet?.equips, [])
    }

    // MARK: - case#4 happy edge: chest.status=2 + remainingSeconds=0

    func testExecuteWithUnlockableChest() async throws {
        let mock = MockHomeRepository()
        mock.loadHomeStub = .success(makeFullResponse(
            chest: LoadHomeUseCaseTests.defaultChest(status: 2, remainingSeconds: 0)
        ))
        let useCase = DefaultLoadHomeUseCase(repository: mock)

        let data = try await useCase.execute()

        XCTAssertEqual(data.chest.status, .unlockable)
        XCTAssertEqual(data.chest.remainingDisplay, "00:00")
    }

    // MARK: - case#5 happy edge: room.currentRoomId 非 nil（节点 4 后场景）

    func testExecuteWithNonNilRoom() async throws {
        let mock = MockHomeRepository()
        mock.loadHomeStub = .success(makeFullResponse(room: RoomDTO(currentRoomId: "3001")))
        let useCase = DefaultLoadHomeUseCase(repository: mock)

        let data = try await useCase.execute()

        XCTAssertEqual(data.room.currentRoomId, "3001")
    }

    // MARK: - case#6 edge: APIError.business 透传

    func testExecuteThrowsBusinessError() async {
        let mock = MockHomeRepository()
        mock.loadHomeStub = .failure(APIError.business(code: 1009, message: "服务繁忙", requestId: "req_xxx"))
        let useCase = DefaultLoadHomeUseCase(repository: mock)

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.business")
        } catch let APIError.business(code, message, requestId) {
            XCTAssertEqual(code, 1009)
            XCTAssertEqual(message, "服务繁忙")
            XCTAssertEqual(requestId, "req_xxx")
        } catch {
            XCTFail("意外错误类型：\(error)")
        }
    }

    // MARK: - case#7 edge: APIError.network 透传

    func testExecuteThrowsNetworkError() async {
        let mock = MockHomeRepository()
        mock.loadHomeStub = .failure(APIError.network(underlying: URLError(.timedOut)))
        let useCase = DefaultLoadHomeUseCase(repository: mock)

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.network")
        } catch let APIError.network(underlying) {
            XCTAssertEqual((underlying as? URLError)?.code, .timedOut)
        } catch {
            XCTFail("意外错误类型：\(error)")
        }
    }

    // MARK: - case#8 edge: APIError.unauthorized 透传（不在 UseCase 层做特殊处理）

    func testExecuteThrowsUnauthorized() async {
        let mock = MockHomeRepository()
        mock.loadHomeStub = .failure(APIError.unauthorized)
        let useCase = DefaultLoadHomeUseCase(repository: mock)

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.unauthorized")
        } catch APIError.unauthorized {
            // pass
        } catch {
            XCTFail("意外错误类型：\(error)")
        }
    }

    // MARK: - case#9 edge: 未识别 chest.status 走 fallback

    func testExecuteUnknownChestStatusFallsBackToCounting() async throws {
        let mock = MockHomeRepository()
        mock.loadHomeStub = .success(makeFullResponse(
            chest: LoadHomeUseCaseTests.defaultChest(status: 99)
        ))
        let useCase = DefaultLoadHomeUseCase(repository: mock)

        let data = try await useCase.execute()

        XCTAssertEqual(data.chest.status, .counting,
                       "未识别 chest.status 应 fallback 到 .counting，不抛 decoding error")
    }

    // MARK: - case#10 edge: 未识别 pet.currentState 走 fallback

    func testExecuteUnknownPetStateFallsBackToRest() async throws {
        let mock = MockHomeRepository()
        mock.loadHomeStub = .success(makeFullResponse(
            pet: LoadHomeUseCaseTests.defaultPet(currentState: 99)
        ))
        let useCase = DefaultLoadHomeUseCase(repository: mock)

        let data = try await useCase.execute()

        XCTAssertEqual(data.pet?.currentState, .rest,
                       "未识别 pet.currentState 应 fallback 到 .rest")
    }
}
