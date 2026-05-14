// SendEmojiUseCaseTests.swift
// Story 18.3 AC9 9b: DefaultSendEmojiUseCase 单测覆盖 (≥3 case，MockWebSocketClient capture sentMessages).
//
// 测试目标（与 story file AC9 钦定一致）：
//   - happy: execute(emojiCode:) 触发 webSocketClient.send 1 次, sentMessages 第一项是 .emojiSend
//   - edge: webSocketClient.sendError = .notConnected → execute 透传抛 WSError.notConnected
//   - happy: 多次 execute 连续调 → sentMessages 累计 3 项 + 每次 emojiCode 对应入参
//
// 与既有 LoadEmojisUseCaseTests / RealRoomViewModelTests 同精神 (XCTest + @testable import PetApp).

import XCTest
@testable import PetApp

final class SendEmojiUseCaseTests: XCTestCase {

    /// happy: execute 触发 webSocketClient.send 1 次, sentMessages 第一项是 .emojiSend(requestId:..., emojiCode:"wave").
    /// requestId 由 UseCase 内部生成 (用 "emoji_<ts_ms>" 格式; V1 §12.2 行 1993 推荐).
    func test_execute_callsWebSocketClientSendWithEmojiSendMessage() async throws {
        let mockClient = WebSocketClientMock()
        let useCase = DefaultSendEmojiUseCase(webSocketClient: mockClient)

        try await useCase.execute(emojiCode: "wave")

        XCTAssertEqual(mockClient.sentMessages.count, 1, "expected exactly 1 send call")
        guard case .emojiSend(let requestId, let code) = mockClient.sentMessages.first else {
            XCTFail("Expected .emojiSend, got \(String(describing: mockClient.sentMessages.first))")
            return
        }
        XCTAssertEqual(code, "wave")
        XCTAssertTrue(requestId.hasPrefix("emoji_"),
                      "requestId should start with emoji_ prefix; got: \(requestId)")
    }

    /// edge: webSocketClient.sendError = .notConnected → execute 透传抛 WSError.notConnected (不转换).
    /// V1 §12.2 / UseCase 设计原则: transport 层错误原样透传, 由 vm 层 mapError 走 toast.
    func test_execute_throwsWhenWebSocketSendFails() async {
        let mockClient = WebSocketClientMock()
        mockClient.sendError = .notConnected
        let useCase = DefaultSendEmojiUseCase(webSocketClient: mockClient)

        do {
            try await useCase.execute(emojiCode: "wave")
            XCTFail("Expected throw, got success")
        } catch let error as WSError {
            XCTAssertEqual(error, .notConnected,
                           "Expected WSError.notConnected, got \(error)")
        } catch {
            XCTFail("Expected WSError.notConnected, got \(error)")
        }
    }

    /// happy: 多次 execute 连续调 → sentMessages 累计 3 项 + 每次 emojiCode 对应入参.
    /// requestId 在毫秒粒度下可能撞同, 不强测唯一性 (与 story file AC9 钦定: "如不分隔可断言 sentMessages.count == 3 即可").
    func test_execute_multipleCallsSendMultipleMessages() async throws {
        let mockClient = WebSocketClientMock()
        let useCase = DefaultSendEmojiUseCase(webSocketClient: mockClient)

        try await useCase.execute(emojiCode: "wave")
        try await useCase.execute(emojiCode: "love")
        try await useCase.execute(emojiCode: "laugh")

        XCTAssertEqual(mockClient.sentMessages.count, 3)
        let codes = mockClient.sentMessages.compactMap { msg -> String? in
            if case .emojiSend(_, let code) = msg { return code } else { return nil }
        }
        XCTAssertEqual(codes, ["wave", "love", "laugh"])
    }
}
