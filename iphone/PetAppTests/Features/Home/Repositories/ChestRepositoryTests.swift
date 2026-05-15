// ChestRepositoryTests.swift
// Story 21.2 AC7: ChestRepository 单测覆盖（≥ 2 case）.
//
// 测试目标：验证 ChestRepository 调 APIClient 时 endpoint 属性正确 + 错误透传严格.
// 用 Story 2.5 落地的 MockAPIClient（按 path stub）.

import XCTest
@testable import PetApp

@MainActor
final class ChestRepositoryTests: XCTestCase {

    // MARK: - case#1 happy: 调用走 /api/v1/chest/current + GET + requiresAuth=true

    func testFetchCurrentCallsCorrectEndpoint() async throws {
        let mock = MockAPIClient()
        let response = ChestCurrentResponse(
            id: "30001",
            status: 1,
            unlockAt: Date(timeIntervalSince1970: 1_800_000_000),
            openCostSteps: 1000,
            remainingSeconds: 300
        )
        mock.stubResponse["/api/v1/chest/current"] = .success(response)
        let repo = DefaultChestRepository(apiClient: mock)

        _ = try await repo.fetchCurrent()

        XCTAssertEqual(mock.invocations.count, 1)
        let endpoint = mock.invocations[0]
        XCTAssertEqual(endpoint.path, "/api/v1/chest/current")
        XCTAssertEqual(endpoint.method, .get)
        XCTAssertTrue(endpoint.requiresAuth, "GET /chest/current 必须 requiresAuth=true 让装饰器拦 401")
        XCTAssertNil(endpoint.body, "GET 请求 body 应为 nil")
    }

    // MARK: - case#2 happy: 5 字段透传（无字段被吞）

    func testFetchCurrentReturnsAllFields() async throws {
        let mock = MockAPIClient()
        let expectedUnlockAt = Date(timeIntervalSince1970: 1_800_000_000)
        let response = ChestCurrentResponse(
            id: "30001",
            status: 2,
            unlockAt: expectedUnlockAt,
            openCostSteps: 1000,
            remainingSeconds: 0
        )
        mock.stubResponse["/api/v1/chest/current"] = .success(response)
        let repo = DefaultChestRepository(apiClient: mock)

        let result = try await repo.fetchCurrent()

        XCTAssertEqual(result.id, "30001")
        XCTAssertEqual(result.status, 2)
        XCTAssertEqual(result.unlockAt, expectedUnlockAt)
        XCTAssertEqual(result.openCostSteps, 1000)
        XCTAssertEqual(result.remainingSeconds, 0)
    }

    // MARK: - case#3 edge: APIError.business 透传

    func testFetchCurrentPassesThroughBusinessError() async {
        let mock = MockAPIClient()
        mock.stubResponse["/api/v1/chest/current"] = .failure(
            .business(code: 4001, message: "用户未找到任何宝箱", requestId: "req_1")
        )
        let repo = DefaultChestRepository(apiClient: mock)

        do {
            _ = try await repo.fetchCurrent()
            XCTFail("应抛 APIError.business")
        } catch let APIError.business(code, message, requestId) {
            XCTAssertEqual(code, 4001)
            XCTAssertEqual(message, "用户未找到任何宝箱")
            XCTAssertEqual(requestId, "req_1")
        } catch {
            XCTFail("意外错误：\(error)")
        }
    }

    // MARK: - case#4 edge: APIError.network 透传

    func testFetchCurrentPassesThroughNetworkError() async {
        let mock = MockAPIClient()
        mock.stubResponse["/api/v1/chest/current"] = .failure(
            .network(underlying: URLError(.notConnectedToInternet))
        )
        let repo = DefaultChestRepository(apiClient: mock)

        do {
            _ = try await repo.fetchCurrent()
            XCTFail("应抛 APIError.network")
        } catch let APIError.network(underlying) {
            XCTAssertEqual((underlying as? URLError)?.code, .notConnectedToInternet)
        } catch {
            XCTFail("意外错误：\(error)")
        }
    }

    // MARK: - case#5 edge: APIError.unauthorized 透传（装饰器处理 401 后才落到 repo）

    func testFetchCurrentPassesThroughUnauthorized() async {
        let mock = MockAPIClient()
        mock.stubResponse["/api/v1/chest/current"] = .failure(.unauthorized)
        let repo = DefaultChestRepository(apiClient: mock)

        do {
            _ = try await repo.fetchCurrent()
            XCTFail("应抛 APIError.unauthorized")
        } catch APIError.unauthorized {
            // pass
        } catch {
            XCTFail("意外错误：\(error)")
        }
    }
}
