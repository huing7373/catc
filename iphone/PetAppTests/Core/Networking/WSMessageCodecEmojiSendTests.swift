// WSMessageCodecEmojiSendTests.swift
// Story 18.3 AC9 9a: WSMessageCodec.encode 对 emoji.send 的覆盖.
//
// 测试目标（与 story file AC9 钦定一致）：
//   - happy: encode(.emojiSend(...)) JSON 严格对齐 V1 §12.2 schema (sortedKeys 保证 key 顺序确定)
//   - edge: emojiCode 含特殊字符 _ - 0-9 → 编码不破 (V1 §11.1 字符集 [a-z0-9_-])
//   - edge: 空 emojiCode → encode 不抛错 (业务校验由 vm 层)
//   - edge: requestId 空字符串 → 编码合法 (V1 §12.2 行 1993 选填)
//
// 与既有 WSMessageCodecTests (节点 4 落地的 ping 测试) 同精神 + 同 import.

import XCTest
@testable import PetApp

final class WSMessageCodecEmojiSendTests: XCTestCase {

    /// happy: V1 §12.2 行 2000-2008 wire schema 严格对齐.
    /// sortedKeys 让 JSON 输出 key 顺序确定 (与 ping case 同模式).
    func test_encode_emojiSend_matchesV12Schema() throws {
        let message = WSOutgoingMessage.emojiSend(requestId: "emoji_1715600000000", emojiCode: "wave")
        let json = try WSMessageCodec.encode(message)
        let expected = "{\"payload\":{\"emojiCode\":\"wave\"},\"requestId\":\"emoji_1715600000000\",\"type\":\"emoji.send\"}"
        XCTAssertEqual(json, expected)
    }

    /// edge: emojiCode 含特殊字符 (V1 §11.1 字符集 [a-z0-9_-]) 不破编码.
    func test_encode_emojiSend_specialCharactersInCode() throws {
        let message = WSOutgoingMessage.emojiSend(requestId: "emoji_x", emojiCode: "my_emoji-1")
        let json = try WSMessageCodec.encode(message)
        XCTAssertTrue(json.contains("\"emojiCode\":\"my_emoji-1\""),
                      "Expected payload.emojiCode to contain 'my_emoji-1', got: \(json)")
    }

    /// edge: codec 是纯序列化层不做业务校验; 空 emojiCode 不抛错; 业务校验由 ViewModel.onEmojiSelected 层做.
    func test_encode_emojiSend_emptyCodeStillEncodes() throws {
        let message = WSOutgoingMessage.emojiSend(requestId: "emoji_x", emojiCode: "")
        let json = try WSMessageCodec.encode(message)
        XCTAssertTrue(json.contains("\"emojiCode\":\"\""),
                      "Expected payload.emojiCode to be empty string in encoded JSON, got: \(json)")
    }

    /// edge: V1 §12.2 行 1993 requestId 选填; 空字符串合法 (与 ping 同精神).
    func test_encode_emojiSend_emptyRequestId() throws {
        let message = WSOutgoingMessage.emojiSend(requestId: "", emojiCode: "wave")
        let json = try WSMessageCodec.encode(message)
        XCTAssertTrue(json.contains("\"requestId\":\"\""),
                      "Expected requestId to be empty string in encoded JSON, got: \(json)")
        // 同时验证 type / payload 不破
        XCTAssertTrue(json.contains("\"type\":\"emoji.send\""))
        XCTAssertTrue(json.contains("\"emojiCode\":\"wave\""))
    }
}
