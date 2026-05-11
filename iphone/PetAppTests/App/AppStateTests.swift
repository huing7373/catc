// AppStateTests.swift
// Story 37.4 AC8：AppState 类单元测试（≥7 case）.
//
// 范围：
//   - case#1 happy: applyHomeData → 5 字段（user/pet/stepAccount/chest/currentRoomId）就绪
//   - case#2 happy: reset() → 全字段 nil/empty
//   - case#3 happy: setCurrentRoomId 双向（写值 / 写 nil）
//   - case#4 happy: updateCurrentEquips 替换 collection
//   - case#5 happy: updateMyPetState 改 currentState
//   - case#6 edge: 初始空态全 nil/empty（hydrate 之前读字段不崩）
//   - case#7 edge: updateMyPetState 在 currentPet=nil 时是 noop（不崩）
//
// 测试基础设施约束（与 Story 2.7 + ADR-0002 §3.1 衔接）：
// - 仅依赖 stdlib（XCTest + @testable import PetApp + AppStateTestHelpers）.
// - 不引 ViewInspector / SnapshotTesting.
// - @MainActor 标在 class 上（AppState 是 @MainActor）.

import XCTest
@testable import PetApp

@MainActor
final class AppStateTests: XCTestCase {

    // MARK: - case#1 happy: applyHomeData → 5 字段就绪

    func testApplyHomeDataPopulatesAllNode2Fields() {
        let appState = AppState()
        let homeData = makeSampleHomeData(currentRoomId: "room_1234567")

        appState.applyHomeData(homeData)

        XCTAssertNotNil(appState.currentUser)
        XCTAssertEqual(appState.currentUser?.nickname, "tester")
        XCTAssertNotNil(appState.currentPet)
        XCTAssertEqual(appState.currentPet?.name, "测试猫")
        XCTAssertNotNil(appState.currentStepAccount)
        XCTAssertEqual(appState.currentStepAccount?.availableSteps, 50)
        XCTAssertNotNil(appState.currentChest)
        XCTAssertEqual(appState.currentRoomId, "room_1234567")
    }

    // MARK: - case#2 happy: reset() → 全字段 nil/empty

    func testResetClearsAllFields() {
        let appState = AppState()
        appState.applyHomeData(makeSampleHomeData(currentRoomId: "room_1234567"))

        appState.reset()

        XCTAssertNil(appState.currentUser)
        XCTAssertNil(appState.currentPet)
        XCTAssertNil(appState.currentStepAccount)
        XCTAssertNil(appState.currentChest)
        XCTAssertNil(appState.currentRoomId)
        XCTAssertTrue(appState.currentInventory.isEmpty)
        XCTAssertTrue(appState.currentEquips.isEmpty)
        XCTAssertTrue(appState.emojiCatalog.isEmpty)
    }

    // MARK: - case#3 happy: setCurrentRoomId("room_1234567") → currentRoomId == "room_1234567"

    func testSetCurrentRoomIdAcceptsArbitraryString() {
        let appState = AppState()
        appState.setCurrentRoomId("room_1234567")
        XCTAssertEqual(appState.currentRoomId, "room_1234567")

        appState.setCurrentRoomId(nil)
        XCTAssertNil(appState.currentRoomId)
    }

    // MARK: - case#4 happy: updateCurrentEquips([...]) → currentEquips 替换

    func testUpdateCurrentEquipsReplacesCollection() {
        let appState = AppState()
        XCTAssertTrue(appState.currentEquips.isEmpty, "默认应为空")

        let equip = HomeEquip(slot: 1, userCosmeticItemId: "uci_1", cosmeticItemId: "ci_1",
                              name: "帽子", rarity: 1, assetUrl: "")
        appState.updateCurrentEquips([equip])
        XCTAssertEqual(appState.currentEquips.count, 1)
        XCTAssertEqual(appState.currentEquips.first?.name, "帽子")

        appState.updateCurrentEquips([])
        XCTAssertTrue(appState.currentEquips.isEmpty, "再次写空数组应清空")
    }

    // MARK: - case#5 happy: updateMyPetState(.walk) → currentPet.currentState 更新

    func testUpdateMyPetStateMutatesCurrentPetState() {
        let appState = AppState()
        appState.applyHomeData(makeSampleHomeData())
        XCTAssertEqual(appState.currentPet?.currentState, .rest)

        appState.updateMyPetState(.walk)
        XCTAssertEqual(appState.currentPet?.currentState, .walk)
    }

    // MARK: - case#6 edge: 初始空态全 nil/empty

    func testInitialStateIsAllNilOrEmpty() {
        let appState = AppState()
        XCTAssertNil(appState.currentUser)
        XCTAssertNil(appState.currentPet)
        XCTAssertNil(appState.currentStepAccount)
        XCTAssertNil(appState.currentChest)
        XCTAssertNil(appState.currentRoomId)
        XCTAssertTrue(appState.currentInventory.isEmpty)
        XCTAssertTrue(appState.currentEquips.isEmpty)
        XCTAssertTrue(appState.emojiCatalog.isEmpty)
    }

    // MARK: - case#7 edge: updateMyPetState 在 currentPet=nil 时是 noop（不崩）

    func testUpdateMyPetStateNoopWhenCurrentPetIsNil() {
        let appState = AppState()
        XCTAssertNil(appState.currentPet, "前置：currentPet 默认 nil")

        appState.updateMyPetState(.walk)  // 不应抛异常
        XCTAssertNil(appState.currentPet, "currentPet 仍为 nil（updateMyPetState 是 guard let pet noop）")
    }

    // MARK: - Story 12.7 r10 [P2] fix: roomNavigationGeneration token 严格单调（ABA-safe）

    /// roomNavigationGeneration 初始 0，每次 setCurrentRoomId 调用 +1（无论新值是否等于旧值）.
    func testRoomNavigationGenerationStrictlyMonotonic() {
        let appState = AppState()
        XCTAssertEqual(appState.roomNavigationGeneration, 0, "初始 0")

        appState.setCurrentRoomId("A")
        let g1 = appState.roomNavigationGeneration
        XCTAssertGreaterThan(g1, 0, "set value → +1")

        appState.setCurrentRoomId(nil)
        let g2 = appState.roomNavigationGeneration
        XCTAssertGreaterThan(g2, g1, "set nil → +1")

        appState.setCurrentRoomId("A")
        let g3 = appState.roomNavigationGeneration
        XCTAssertGreaterThan(g3, g2,
                             "ABA cycle 关键：currentRoomId 经历 A → nil → A 后回到 A，但 generation 必须严格单调")
        XCTAssertEqual(appState.currentRoomId, "A", "currentRoomId 回到 A 是 ABA 场景的核心特征")
    }

    /// applyHomeData 和 reset 也应 bump generation（防 in-flight stale response 跨 lifecycle 应用）.
    func testRoomNavigationGenerationBumpsOnHydrateAndReset() {
        let appState = AppState()
        let g0 = appState.roomNavigationGeneration

        appState.applyHomeData(makeSampleHomeData(currentRoomId: "X"))
        let g1 = appState.roomNavigationGeneration
        XCTAssertGreaterThan(g1, g0, "applyHomeData 也算 navigation cycle → +1")

        appState.reset()
        let g2 = appState.roomNavigationGeneration
        XCTAssertGreaterThan(g2, g1, "reset 也算 navigation cycle → +1")
    }
}
