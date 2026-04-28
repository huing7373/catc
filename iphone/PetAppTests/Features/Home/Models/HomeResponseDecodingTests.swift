// HomeResponseDecodingTests.swift
// Story 5.5 AC11: HomeResponse wire 解码契约锁定（≥ 4 case）.
//
// 用 JSONDecoder 直接解码 raw JSON，独立于 APIClient，保证 schema 解码契约可靠.
// decoder 配置 .iso8601（与 APIClient 内的 decoder 配置对齐 —— V1 §5.1 钦定 chest.unlockAt 用 RFC3339）.

import XCTest
@testable import PetApp

final class HomeResponseDecodingTests: XCTestCase {

    private func makeDecoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .iso8601
        return decoder
    }

    private func decode(_ json: String) throws -> HomeResponse {
        let data = json.data(using: .utf8)!
        return try makeDecoder().decode(HomeResponse.self, from: data)
    }

    // MARK: - case#1 happy: 完整 JSON（V1 §5.1 节点 2 阶段示例）

    func testDecodeFullResponse() throws {
        let json = """
        {
          "user": {
            "id": "10001",
            "nickname": "用户10001",
            "avatarUrl": ""
          },
          "pet": {
            "id": "20001",
            "petType": 1,
            "name": "默认小猫",
            "currentState": 1,
            "equips": []
          },
          "stepAccount": {
            "totalSteps": 0,
            "availableSteps": 0,
            "consumedSteps": 0
          },
          "chest": {
            "id": "30001",
            "status": 1,
            "unlockAt": "2026-04-27T10:20:00Z",
            "openCostSteps": 100,
            "remainingSeconds": 600
          },
          "room": {
            "currentRoomId": null
          }
        }
        """

        let response = try decode(json)

        XCTAssertEqual(response.user.id, "10001")
        XCTAssertEqual(response.user.nickname, "用户10001")
        XCTAssertEqual(response.user.avatarUrl, "")
        XCTAssertEqual(response.pet?.id, "20001")
        XCTAssertEqual(response.pet?.petType, 1)
        XCTAssertEqual(response.pet?.name, "默认小猫")
        XCTAssertEqual(response.pet?.currentState, 1)
        XCTAssertEqual(response.pet?.equips, [])
        XCTAssertEqual(response.stepAccount.totalSteps, 0)
        XCTAssertEqual(response.stepAccount.availableSteps, 0)
        XCTAssertEqual(response.stepAccount.consumedSteps, 0)
        XCTAssertEqual(response.chest.id, "30001")
        XCTAssertEqual(response.chest.status, 1)
        XCTAssertEqual(response.chest.openCostSteps, 100)
        XCTAssertEqual(response.chest.remainingSeconds, 600)
        // unlockAt 用 ISO8601 解码后是 Date 类型；验证关键时间点
        let formatter = ISO8601DateFormatter()
        XCTAssertEqual(response.chest.unlockAt, formatter.date(from: "2026-04-27T10:20:00Z"))
        XCTAssertNil(response.room.currentRoomId)
    }

    // MARK: - case#2 happy edge: pet=null

    func testDecodeWithNullPet() throws {
        let json = """
        {
          "user": { "id": "10001", "nickname": "n", "avatarUrl": "" },
          "pet": null,
          "stepAccount": { "totalSteps": 0, "availableSteps": 0, "consumedSteps": 0 },
          "chest": {
            "id": "30001", "status": 1,
            "unlockAt": "2026-04-27T10:20:00Z",
            "openCostSteps": 100, "remainingSeconds": 600
          },
          "room": { "currentRoomId": null }
        }
        """

        let response = try decode(json)

        XCTAssertNil(response.pet)
    }

    // MARK: - case#3 happy edge: room.currentRoomId=null

    func testDecodeWithNullCurrentRoomId() throws {
        let json = """
        {
          "user": { "id": "10001", "nickname": "n", "avatarUrl": "" },
          "pet": null,
          "stepAccount": { "totalSteps": 0, "availableSteps": 0, "consumedSteps": 0 },
          "chest": {
            "id": "30001", "status": 1,
            "unlockAt": "2026-04-27T10:20:00Z",
            "openCostSteps": 100, "remainingSeconds": 600
          },
          "room": { "currentRoomId": null }
        }
        """

        let response = try decode(json)

        XCTAssertNil(response.room.currentRoomId)
    }

    // MARK: - case#4 happy edge: pet.equips=[]

    func testDecodeWithEmptyEquips() throws {
        let json = """
        {
          "user": { "id": "10001", "nickname": "n", "avatarUrl": "" },
          "pet": {
            "id": "20001", "petType": 1, "name": "p", "currentState": 1,
            "equips": []
          },
          "stepAccount": { "totalSteps": 0, "availableSteps": 0, "consumedSteps": 0 },
          "chest": {
            "id": "30001", "status": 1,
            "unlockAt": "2026-04-27T10:20:00Z",
            "openCostSteps": 100, "remainingSeconds": 600
          },
          "room": { "currentRoomId": null }
        }
        """

        let response = try decode(json)

        XCTAssertEqual(response.pet?.equips, [])
    }

    // MARK: - case#5 edge: 缺 unlockAt → DecodingError

    func testDecodeMissingUnlockAtThrows() {
        let json = """
        {
          "user": { "id": "10001", "nickname": "n", "avatarUrl": "" },
          "pet": null,
          "stepAccount": { "totalSteps": 0, "availableSteps": 0, "consumedSteps": 0 },
          "chest": {
            "id": "30001", "status": 1,
            "openCostSteps": 100, "remainingSeconds": 600
          },
          "room": { "currentRoomId": null }
        }
        """

        XCTAssertThrowsError(try decode(json)) { error in
            XCTAssertTrue(error is DecodingError, "缺 unlockAt 应抛 DecodingError")
        }
    }

    // MARK: - case#6 edge: unlockAt 是非 ISO8601 → DecodingError

    func testDecodeNonIso8601UnlockAtThrows() {
        let json = """
        {
          "user": { "id": "10001", "nickname": "n", "avatarUrl": "" },
          "pet": null,
          "stepAccount": { "totalSteps": 0, "availableSteps": 0, "consumedSteps": 0 },
          "chest": {
            "id": "30001", "status": 1,
            "unlockAt": "2026/04/27 10:20:00",
            "openCostSteps": 100, "remainingSeconds": 600
          },
          "room": { "currentRoomId": null }
        }
        """

        XCTAssertThrowsError(try decode(json)) { error in
            XCTAssertTrue(error is DecodingError, "非 ISO8601 unlockAt 应抛 DecodingError")
        }
    }

    // MARK: - case#7 happy edge: pet.equips 含一个完整 equip

    func testDecodeWithSingleEquip() throws {
        let json = """
        {
          "user": { "id": "10001", "nickname": "n", "avatarUrl": "" },
          "pet": {
            "id": "20001", "petType": 1, "name": "p", "currentState": 2,
            "equips": [
              {
                "slot": 1,
                "userCosmeticItemId": "uci_1",
                "cosmeticItemId": "ci_1",
                "name": "红帽子",
                "rarity": 3,
                "assetUrl": "https://cdn/x.png"
              }
            ]
          },
          "stepAccount": { "totalSteps": 100, "availableSteps": 50, "consumedSteps": 50 },
          "chest": {
            "id": "30001", "status": 2,
            "unlockAt": "2026-04-27T10:20:00Z",
            "openCostSteps": 100, "remainingSeconds": 0
          },
          "room": { "currentRoomId": "3001" }
        }
        """

        let response = try decode(json)

        XCTAssertEqual(response.pet?.equips.count, 1)
        let equip = response.pet!.equips[0]
        XCTAssertEqual(equip.slot, 1)
        XCTAssertEqual(equip.userCosmeticItemId, "uci_1")
        XCTAssertEqual(equip.cosmeticItemId, "ci_1")
        XCTAssertEqual(equip.name, "红帽子")
        XCTAssertEqual(equip.rarity, 3)
        XCTAssertEqual(equip.assetUrl, "https://cdn/x.png")
        XCTAssertEqual(response.pet?.currentState, 2)
        XCTAssertEqual(response.chest.status, 2)
        XCTAssertEqual(response.chest.remainingSeconds, 0)
        XCTAssertEqual(response.room.currentRoomId, "3001")
    }
}
