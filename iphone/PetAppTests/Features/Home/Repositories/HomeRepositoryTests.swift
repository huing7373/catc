// HomeRepositoryTests.swift
// Story 5.5 AC10: HomeRepository 单测覆盖（≥ 3 case）.
//
// 测试目标：验证 HomeRepository 调 APIClient 时 endpoint 属性正确 + 错误透传严格.
// 用 Story 2.5 落地的 MockAPIClient（按 path stub）.

import XCTest
@testable import PetApp

@MainActor
final class HomeRepositoryTests: XCTestCase {

    // MARK: - case#1 happy: 调用走 /api/v1/home + GET + requiresAuth=true

    func testLoadHomeCallsCorrectEndpoint() async throws {
        let mock = MockAPIClient()
        let response = HomeResponse(
            user: UserInfoDTO(id: "10001", nickname: "test", avatarUrl: ""),
            pet: nil,
            stepAccount: StepAccountDTO(totalSteps: 0, availableSteps: 0, consumedSteps: 0),
            chest: ChestDTO(
                id: "30001",
                status: 1,
                unlockAt: Date(timeIntervalSince1970: 0),
                openCostSteps: 100,
                remainingSeconds: 600
            ),
            room: RoomDTO(currentRoomId: nil)
        )
        mock.stubResponse["/api/v1/home"] = .success(response)
        let repo = DefaultHomeRepository(apiClient: mock)

        _ = try await repo.loadHome()

        XCTAssertEqual(mock.invocations.count, 1)
        let endpoint = mock.invocations[0]
        XCTAssertEqual(endpoint.path, "/api/v1/home")
        XCTAssertEqual(endpoint.method, .get)
        XCTAssertTrue(endpoint.requiresAuth, "GET /home 必须 requiresAuth=true 让装饰器拦 401")
        XCTAssertNil(endpoint.body, "GET 请求 body 应为 nil")
    }

    // MARK: - case#2 edge: APIError.business 透传

    func testLoadHomePassesThroughBusinessError() async {
        let mock = MockAPIClient()
        mock.stubResponse["/api/v1/home"] = .failure(.business(code: 1009, message: "服务繁忙", requestId: "req_1"))
        let repo = DefaultHomeRepository(apiClient: mock)

        do {
            _ = try await repo.loadHome()
            XCTFail("应抛 APIError.business")
        } catch let APIError.business(code, message, requestId) {
            XCTAssertEqual(code, 1009)
            XCTAssertEqual(message, "服务繁忙")
            XCTAssertEqual(requestId, "req_1")
        } catch {
            XCTFail("意外错误：\(error)")
        }
    }

    // MARK: - case#3 edge: APIError.network 透传

    func testLoadHomePassesThroughNetworkError() async {
        let mock = MockAPIClient()
        mock.stubResponse["/api/v1/home"] = .failure(.network(underlying: URLError(.notConnectedToInternet)))
        let repo = DefaultHomeRepository(apiClient: mock)

        do {
            _ = try await repo.loadHome()
            XCTFail("应抛 APIError.network")
        } catch let APIError.network(underlying) {
            XCTAssertEqual((underlying as? URLError)?.code, .notConnectedToInternet)
        } catch {
            XCTFail("意外错误：\(error)")
        }
    }

    // MARK: - case#4 edge: APIError.unauthorized 透传

    func testLoadHomePassesThroughUnauthorized() async {
        let mock = MockAPIClient()
        mock.stubResponse["/api/v1/home"] = .failure(.unauthorized)
        let repo = DefaultHomeRepository(apiClient: mock)

        do {
            _ = try await repo.loadHome()
            XCTFail("应抛 APIError.unauthorized")
        } catch APIError.unauthorized {
            // pass
        } catch {
            XCTFail("意外错误：\(error)")
        }
    }
}
