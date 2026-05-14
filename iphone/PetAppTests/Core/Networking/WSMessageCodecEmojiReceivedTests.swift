// WSMessageCodecEmojiReceivedTests.swift
// Story 18.4 AC9 9a: WSMessageCodec.decode 对 emoji.received 路由 + EmojiReceivedEnvelope 解码 + 空字段守护.
//
// 测试目标（与 story file AC9 钦定）：
//   - happy: decode 完整 JSON 信封 → .emojiReceived(EmojiReceivedPayload(userId, emojiCode))
//   - edge: payload.userId == "" → .unknown(rawType: "emoji.received") (codec 空字段守护防污染 vm)
//   - edge: payload.emojiCode == "" → .unknown(rawType: "emoji.received")
//   - edge: payload 缺 emojiCode 字段 → .unknown(rawType: "emoji.received") (Decodable 抛错)
//   - edge: payload 缺 userId 字段 → .unknown(rawType: "emoji.received")
//   - happy: requestId == "" + ts 任意值 → 仍能解 .emojiReceived (codec 不消费 ts)
//
// 与既有 WSMessageCodecEmojiSendTests (18.3 落地) / WSMessageCodecTests (Epic 10 落地) 同精神.

import XCTest
@testable import PetApp

final class WSMessageCodecEmojiReceivedTests: XCTestCase {

    // MARK: - happy: 完整 wire schema decode

    /// happy: V1 §12.3 行 2446-2464 wire schema 严格对齐.
    /// JSON 信封 type=emoji.received + requestId="" + payload.userId + payload.emojiCode + ts.
    func test_decode_emojiReceived_happyPath() {
        let json = """
        {"type":"emoji.received","requestId":"","payload":{"userId":"1002","emojiCode":"wave"},"ts":1776920345000}
        """
        let msg = WSMessageCodec.decode(json)
        guard case .emojiReceived(let payload) = msg else {
            XCTFail("Expected .emojiReceived, got: \(msg)")
            return
        }
        XCTAssertEqual(payload.userId, "1002")
        XCTAssertEqual(payload.emojiCode, "wave")
    }

    /// happy: V1 §12.3 行 2476 钦定 ts 字段 codec 不消费; requestId 任意值不影响解码.
    func test_decode_emojiReceived_emptyRequestIdAndMissingTs() {
        // 故意省略 ts 字段 + requestId 空; codec 既有信封解码容忍 (ts 字段不 required).
        let json = """
        {"type":"emoji.received","requestId":"","payload":{"userId":"u_other","emojiCode":"love"}}
        """
        let msg = WSMessageCodec.decode(json)
        guard case .emojiReceived(let payload) = msg else {
            XCTFail("Expected .emojiReceived even when ts missing, got: \(msg)")
            return
        }
        XCTAssertEqual(payload.userId, "u_other")
        XCTAssertEqual(payload.emojiCode, "love")
    }

    // MARK: - edge: 空字符串 fallback → .unknown

    /// edge: V1 §12.3 行 2469 钦定 payload.userId 必填且非空; 空字符串 → codec fallback .unknown.
    func test_decode_emojiReceived_emptyUserIdRejected() {
        let json = """
        {"type":"emoji.received","requestId":"","payload":{"userId":"","emojiCode":"wave"},"ts":1}
        """
        let msg = WSMessageCodec.decode(json)
        guard case .unknown(let rawType) = msg else {
            XCTFail("Expected .unknown(rawType: emoji.received) for empty userId, got: \(msg)")
            return
        }
        XCTAssertEqual(rawType, "emoji.received")
    }

    /// edge: V1 §12.3 行 2469 钦定 payload.emojiCode 必填且非空; 空字符串 → codec fallback .unknown.
    func test_decode_emojiReceived_emptyEmojiCodeRejected() {
        let json = """
        {"type":"emoji.received","requestId":"","payload":{"userId":"u1","emojiCode":""},"ts":1}
        """
        let msg = WSMessageCodec.decode(json)
        guard case .unknown(let rawType) = msg else {
            XCTFail("Expected .unknown(rawType: emoji.received) for empty emojiCode, got: \(msg)")
            return
        }
        XCTAssertEqual(rawType, "emoji.received")
    }

    // MARK: - edge: 缺字段 → Decodable 抛错 → fallback .unknown

    /// edge: payload 缺 emojiCode 字段 → Decodable 抛 KeyNotFound → 外层 catch → .unknown.
    func test_decode_emojiReceived_missingEmojiCodeRejected() {
        let json = """
        {"type":"emoji.received","requestId":"","payload":{"userId":"u1"},"ts":1}
        """
        let msg = WSMessageCodec.decode(json)
        guard case .unknown(let rawType) = msg else {
            XCTFail("Expected .unknown(rawType: emoji.received) for missing emojiCode, got: \(msg)")
            return
        }
        XCTAssertEqual(rawType, "emoji.received")
    }

    /// edge: payload 缺 userId 字段 → Decodable 抛 KeyNotFound → 外层 catch → .unknown.
    func test_decode_emojiReceived_missingUserIdRejected() {
        let json = """
        {"type":"emoji.received","requestId":"","payload":{"emojiCode":"wave"},"ts":1}
        """
        let msg = WSMessageCodec.decode(json)
        guard case .unknown(let rawType) = msg else {
            XCTFail("Expected .unknown(rawType: emoji.received) for missing userId, got: \(msg)")
            return
        }
        XCTAssertEqual(rawType, "emoji.received")
    }

    /// edge: payload 整体缺失 → Decodable 抛 KeyNotFound → 外层 catch → .unknown.
    func test_decode_emojiReceived_missingPayloadRejected() {
        let json = """
        {"type":"emoji.received","requestId":"","ts":1}
        """
        let msg = WSMessageCodec.decode(json)
        guard case .unknown(let rawType) = msg else {
            XCTFail("Expected .unknown(rawType: emoji.received) for missing payload, got: \(msg)")
            return
        }
        XCTAssertEqual(rawType, "emoji.received")
    }
}
