// InventoryResponseTests.swift
// Story 24.2 AC1 守护 case: InventoryResponse 严格非可选解析 + fail-fast.
//
// 验证（不经 network，直接对 Decodable 解析断言）：
//   - {groups: []} → 解析为 InventoryResponse(groups: [])（空背包合法）
//   - 完整 group JSON（V1 §8.2 返回示例）→ 各字段正确解码
//   - {groups: null} → 抛 decoding 错误（fail-fast，防 Swift Codable 解析为 nil）
//   - 缺 groups 字段 {} → 抛 decoding 错误（fail-fast）
//   - group 缺必填字段 → 抛 decoding 错误（严格非可选 + fail-fast）
//
// XCTest only（ADR-0002 §3.1）；用 JSONDecoder 直接解析（与 server 漏发 fail-fast 同精神，
// lesson 2026-04-27-home-data-fail-fast-on-unknown-enum.md）.

import XCTest
@testable import PetApp

final class InventoryResponseTests: XCTestCase {

    private let decoder = JSONDecoder()

    private func decode(_ json: String) throws -> InventoryResponse {
        try decoder.decode(InventoryResponse.self, from: Data(json.utf8))
    }

    // MARK: - 空背包 {groups: []} → []（合法，不报错）

    func testDecodesEmptyGroupsAsEmptyArray() throws {
        let response = try decode(#"{"groups": []}"#)
        XCTAssertEqual(response.groups, [], "空背包 {groups: []} → groups == []（V1 §8.2 契约）")
    }

    // MARK: - 完整 group（V1 §8.2 返回示例）→ 各字段正确解码

    func testDecodesFullGroupWithInstances() throws {
        let json = #"""
        {
          "groups": [
            {
              "cosmeticItemId": "12",
              "name": "小黄帽",
              "slot": 1,
              "rarity": 1,
              "iconUrl": "https://icon",
              "assetUrl": "https://asset",
              "count": 3,
              "instances": [
                {"userCosmeticItemId": "90001", "status": 1},
                {"userCosmeticItemId": "90005", "status": 1},
                {"userCosmeticItemId": "90008", "status": 2}
              ]
            }
          ]
        }
        """#
        let response = try decode(json)

        XCTAssertEqual(response.groups.count, 1)
        let group = response.groups[0]
        XCTAssertEqual(group.cosmeticItemId, "12")
        XCTAssertEqual(group.name, "小黄帽")
        XCTAssertEqual(group.slot, 1)
        XCTAssertEqual(group.rarity, 1)
        XCTAssertEqual(group.iconUrl, "https://icon")
        XCTAssertEqual(group.assetUrl, "https://asset")
        XCTAssertEqual(group.count, 3)
        XCTAssertEqual(group.instances.count, 3)
        XCTAssertEqual(group.instances[2].userCosmeticItemId, "90008")
        XCTAssertEqual(group.instances[2].status, 2)
    }

    // MARK: - {groups: null} → fail-fast 抛 decoding（防 Codable 解析为 nil）

    func testDecodingNullGroupsThrows() {
        XCTAssertThrowsError(try decode(#"{"groups": null}"#),
            "groups: null 必须 fail-fast 抛 decoding（严格非可选 [InventoryGroup]）") { error in
            XCTAssertTrue(error is DecodingError, "应是 DecodingError，实得 \(error)")
        }
    }

    // MARK: - 缺 groups 字段 {} → fail-fast 抛 decoding

    func testDecodingMissingGroupsFieldThrows() {
        XCTAssertThrowsError(try decode(#"{}"#),
            "缺 groups 字段必须 fail-fast 抛 decoding（V1 §8.2 契约：不允许省略 groups）") { error in
            XCTAssertTrue(error is DecodingError, "应是 DecodingError，实得 \(error)")
        }
    }

    // MARK: - group 缺必填字段 → fail-fast 抛 decoding

    func testDecodingGroupMissingRequiredFieldThrows() {
        // 缺 instances 字段（V1 §8.2 必填）.
        let json = #"""
        {"groups": [{"cosmeticItemId": "1", "name": "x", "slot": 1, "rarity": 1,
                     "iconUrl": "i", "assetUrl": "a", "count": 1}]}
        """#
        XCTAssertThrowsError(try decode(json),
            "group 缺必填字段必须 fail-fast 抛 decoding（严格非可选解析）") { error in
            XCTAssertTrue(error is DecodingError, "应是 DecodingError，实得 \(error)")
        }
    }
}
