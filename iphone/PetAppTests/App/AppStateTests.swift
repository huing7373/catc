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

    /// reset 也应 bump generation（防 in-flight stale response 跨 lifecycle 应用）.
    /// Story 12.7 r12 [P2] fix（codex review r12）：applyHomeData 已改为**仅当 currentRoomId
    /// 实际变更时** bump（区别测试见下方 5 个 applyHomeData* 用例）.
    func testRoomNavigationGenerationBumpsOnReset() {
        let appState = AppState()
        appState.applyHomeData(makeSampleHomeData(currentRoomId: "X"))
        let g1 = appState.roomNavigationGeneration

        appState.reset()
        let g2 = appState.roomNavigationGeneration
        XCTAssertGreaterThan(g2, g1, "reset 也算 navigation cycle → +1")
    }

    // MARK: - Story 12.7 r12 [P2] fix（codex review r12）: applyHomeData 仅在 currentRoomId 实际变更时 bump

    /// applyHomeData 不变 currentRoomId 时 **不** bump generation —— 防 HomeViewModel.loadHome()
    /// retry 或 RootView bootstrap/cold-start path 的 home hydrate 误伤 in-flight room flow request.
    func testApplyHomeDataDoesNotBumpWhenRoomIdUnchanged() {
        let appState = AppState()
        // 先 hydrate 一次进 room "X"，generation 从 0 → 1（nil → "X"）.
        appState.applyHomeData(makeSampleHomeData(currentRoomId: "X"))
        let gAfterFirstHydrate = appState.roomNavigationGeneration

        // 再次 hydrate 同一 currentRoomId（典型场景：HomeViewModel.loadHome() retry）
        // → generation 不应 bump.
        appState.applyHomeData(makeSampleHomeData(currentRoomId: "X"))
        XCTAssertEqual(appState.roomNavigationGeneration, gAfterFirstHydrate,
                       "currentRoomId 不变（X → X）→ generation 不变（防误伤 in-flight room request）")
    }

    /// applyHomeData currentRoomId 从 nil → "X" 时 bump generation（启动 / 首次 hydrate 是真实 navigation event）.
    func testApplyHomeDataBumpsWhenRoomIdChangesFromNilToValue() {
        let appState = AppState()
        XCTAssertNil(appState.currentRoomId, "前置：currentRoomId 默认 nil")
        let g0 = appState.roomNavigationGeneration

        appState.applyHomeData(makeSampleHomeData(currentRoomId: "X"))
        XCTAssertGreaterThan(appState.roomNavigationGeneration, g0,
                             "currentRoomId nil → X（首次进 room）→ generation +1")
    }

    /// applyHomeData currentRoomId 从 "X" → nil 时 bump generation（server 推送显示用户被踢出 room）.
    func testApplyHomeDataBumpsWhenRoomIdChangesFromValueToNil() {
        let appState = AppState()
        appState.applyHomeData(makeSampleHomeData(currentRoomId: "X"))
        let g1 = appState.roomNavigationGeneration

        appState.applyHomeData(makeSampleHomeData(currentRoomId: nil))
        XCTAssertGreaterThan(appState.roomNavigationGeneration, g1,
                             "currentRoomId X → nil（被踢出 / leave hydrate）→ generation +1")
    }

    /// applyHomeData currentRoomId 从 nil → nil 时 **不** bump generation —— 未登录 / 无房间用户
    /// 周期性 hydrate 不应触发 navigation cycle.
    func testApplyHomeDataDoesNotBumpWhenRoomIdStaysNil() {
        let appState = AppState()
        XCTAssertNil(appState.currentRoomId)
        let g0 = appState.roomNavigationGeneration

        appState.applyHomeData(makeSampleHomeData(currentRoomId: nil))
        XCTAssertEqual(appState.roomNavigationGeneration, g0,
                       "currentRoomId nil → nil（未在 room 的 hydrate）→ generation 不变")
    }

    /// applyHomeData currentRoomId 从 "X" → "Y" 时 bump generation（罕见，但 server 推送换 room 算 navigation event）.
    func testApplyHomeDataBumpsWhenRoomIdChangesBetweenValues() {
        let appState = AppState()
        appState.applyHomeData(makeSampleHomeData(currentRoomId: "X"))
        let g1 = appState.roomNavigationGeneration

        appState.applyHomeData(makeSampleHomeData(currentRoomId: "Y"))
        XCTAssertGreaterThan(appState.roomNavigationGeneration, g1,
                             "currentRoomId X → Y（换 room hydrate）→ generation +1")
    }

    // MARK: - Story 21.2 AC3: applyCurrentChest 单字段 mutation

    /// Story 21.2 AC3: applyCurrentChest 仅写 currentChest 字段，不动其它 6 字段.
    /// 与 applySyncedStepAccount 同模式（单字段 mutation 入口；apply* 前缀；详见 ADR-0010 §3.3）.
    func testApplyCurrentChestUpdatesOnlyChestField() {
        let appState = AppState()
        // 先 hydrate 完整数据让 6 个字段就位
        appState.applyHomeData(makeSampleHomeData(currentRoomId: "room_1234567"))
        let originalUser = appState.currentUser
        let originalPet = appState.currentPet
        let originalStepAccount = appState.currentStepAccount
        let originalRoomId = appState.currentRoomId

        // 调 applyCurrentChest 写新 chest
        let newChest = HomeChest(
            id: "new_chest",
            status: .unlockable,
            unlockAt: Date(timeIntervalSince1970: 2_000_000_000),
            openCostSteps: 1000,
            remainingSeconds: 0
        )
        appState.applyCurrentChest(newChest)

        // currentChest 应被新值覆盖
        XCTAssertEqual(appState.currentChest?.id, "new_chest")
        XCTAssertEqual(appState.currentChest?.status, .unlockable)
        XCTAssertEqual(appState.currentChest?.remainingSeconds, 0)

        // 其它 6 字段应保持不变
        XCTAssertEqual(appState.currentUser, originalUser)
        XCTAssertEqual(appState.currentPet, originalPet)
        XCTAssertEqual(appState.currentStepAccount, originalStepAccount)
        XCTAssertEqual(appState.currentRoomId, originalRoomId)
    }

    /// Story 21.2 AC3 关键决策: applyCurrentChest **不** bump roomNavigationGeneration
    /// （chest mutation 与 room navigation 完全独立；与 applySyncedStepAccount 不 bump 同精神）.
    /// 与 Story 12.7 r12 [P2] fix 钦定 generation 仅在 room 字段实际变更时 bump 同精神.
    func testApplyCurrentChestDoesNotBumpRoomNavigationGeneration() {
        let appState = AppState()
        appState.applyHomeData(makeSampleHomeData(currentRoomId: "room_1234567"))
        let gBefore = appState.roomNavigationGeneration

        let newChest = HomeChest(
            id: "new_chest",
            status: .counting,
            unlockAt: Date(timeIntervalSince1970: 2_000_000_000),
            openCostSteps: 1000,
            remainingSeconds: 600
        )
        appState.applyCurrentChest(newChest)

        XCTAssertEqual(appState.roomNavigationGeneration, gBefore,
            "applyCurrentChest 不应 bump roomNavigationGeneration（chest mutation 与 room flow 无关）")
    }

    /// Story 21.2 AC3: applyCurrentChest 在空 AppState 上写入 OK（hydrate 前调也安全）.
    func testApplyCurrentChestOnEmptyAppStateWritesChestOnly() {
        let appState = AppState()
        XCTAssertNil(appState.currentChest)
        XCTAssertNil(appState.currentUser)

        let newChest = HomeChest(
            id: "new_chest",
            status: .counting,
            unlockAt: Date(timeIntervalSince1970: 2_000_000_000),
            openCostSteps: 1000,
            remainingSeconds: 600
        )
        appState.applyCurrentChest(newChest)

        XCTAssertEqual(appState.currentChest?.id, "new_chest")
        XCTAssertNil(appState.currentUser, "其它字段保持 nil")
        XCTAssertNil(appState.currentPet)
        XCTAssertNil(appState.currentStepAccount)
        XCTAssertNil(appState.currentRoomId)
    }
}
