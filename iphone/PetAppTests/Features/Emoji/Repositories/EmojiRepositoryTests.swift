// EmojiRepositoryTests.swift
// Story 18.1 AC7: EmojiRepository 单测覆盖 (≥2 case，MockAPIClient stub).
//
// 测试目标：验证 EmojiRepository 调 APIClient 时 endpoint 属性正确 + items 直通 + APIError 透传.
// 与 HomeRepositoryTests / Story 5.5 模式同源.

import XCTest
@testable import PetApp

@MainActor
final class EmojiRepositoryTests: XCTestCase {

    // MARK: - case#1 happy: 4 个 emoji response → repo 返 4 项 + 字段值精确匹配

    func test_listEmojis_happyPath_returns4Items() async throws {
        let mock = MockAPIClient()
        let stubResponse = EmojiListResponse(items: [
            EmojiConfig(code: "wave", name: "挥手", assetUrl: "https://placehold.co/64x64?text=Wave", sortOrder: 1),
            EmojiConfig(code: "love", name: "爱心", assetUrl: "https://placehold.co/64x64?text=Love", sortOrder: 2),
            EmojiConfig(code: "laugh", name: "大笑", assetUrl: "https://placehold.co/64x64?text=Laugh", sortOrder: 3),
            EmojiConfig(code: "cry", name: "哭泣", assetUrl: "https://placehold.co/64x64?text=Cry", sortOrder: 4)
        ])
        mock.stubResponse["/api/v1/emojis"] = .success(stubResponse)
        let repo = DefaultEmojiRepository(apiClient: mock)

        let emojis = try await repo.listEmojis()

        XCTAssertEqual(emojis.count, 4)
        XCTAssertEqual(emojis[0].code, "wave")
        XCTAssertEqual(emojis[0].name, "挥手")
        XCTAssertEqual(emojis[0].assetUrl, "https://placehold.co/64x64?text=Wave")
        XCTAssertEqual(emojis[0].sortOrder, 1)
        XCTAssertEqual(emojis[3].code, "cry")
        XCTAssertEqual(emojis[3].sortOrder, 4)
    }

    // MARK: - case#2 happy: 验证 endpoint path / method / requiresAuth 严格契约

    func test_listEmojis_callsCorrectEndpoint() async throws {
        let mock = MockAPIClient()
        mock.stubResponse["/api/v1/emojis"] = .success(EmojiListResponse(items: []))
        let repo = DefaultEmojiRepository(apiClient: mock)

        _ = try await repo.listEmojis()

        XCTAssertEqual(mock.invocations.count, 1)
        let endpoint = mock.invocations[0]
        XCTAssertEqual(endpoint.path, "/api/v1/emojis", "GET /api/v1/emojis 必须 path 严格匹配 V1 §11.1")
        XCTAssertEqual(endpoint.method, .get)
        XCTAssertTrue(endpoint.requiresAuth, "GET /emojis 必须 requiresAuth=true (V1 §11.1 钦定)")
        XCTAssertNil(endpoint.body, "GET 请求 body 应为 nil")
    }

    // MARK: - case#3 edge: items: [] 返空数组不 panic (V1 §11.1 server 永远返 [] 而非 null)

    func test_listEmojis_emptyItems_returnsEmptyArray() async throws {
        let mock = MockAPIClient()
        mock.stubResponse["/api/v1/emojis"] = .success(EmojiListResponse(items: []))
        let repo = DefaultEmojiRepository(apiClient: mock)

        let emojis = try await repo.listEmojis()

        XCTAssertEqual(emojis.count, 0)
        XCTAssertTrue(emojis.isEmpty)
    }

    // MARK: - case#4 edge: APIError.business(1009) 透传 (server DB 异常 / 服务繁忙)

    func test_listEmojis_passesThroughBusinessError() async {
        let mock = MockAPIClient()
        mock.stubResponse["/api/v1/emojis"] = .failure(.business(code: 1009, message: "DB 异常", requestId: "req_emoji_1"))
        let repo = DefaultEmojiRepository(apiClient: mock)

        do {
            _ = try await repo.listEmojis()
            XCTFail("应抛 APIError.business")
        } catch let APIError.business(code, message, requestId) {
            XCTAssertEqual(code, 1009)
            XCTAssertEqual(message, "DB 异常")
            XCTAssertEqual(requestId, "req_emoji_1")
        } catch {
            XCTFail("意外错误: \(error)")
        }
    }

    // MARK: - case#5 edge: APIError.network 透传

    func test_listEmojis_passesThroughNetworkError() async {
        let mock = MockAPIClient()
        mock.stubResponse["/api/v1/emojis"] = .failure(.network(underlying: URLError(.notConnectedToInternet)))
        let repo = DefaultEmojiRepository(apiClient: mock)

        do {
            _ = try await repo.listEmojis()
            XCTFail("应抛 APIError.network")
        } catch let APIError.network(underlying) {
            XCTAssertEqual((underlying as? URLError)?.code, .notConnectedToInternet)
        } catch {
            XCTFail("意外错误: \(error)")
        }
    }
}
