// LoadInventoryUseCaseTests.swift
// Story 24.2 AC「单元测试覆盖」: LoadInventoryUseCase 单测覆盖.
//
// 测试目标：验证 UseCase = repo.fetchInventory() → wire DTO 展平为 [HomeEquip]
//   → MainActor.run { appState.applyInventory } + 失败原样透传不污染 AppState.
//
// 测试基础设施约束（ADR-0002 §3.1 钦定 —— 不可违反）：
//   - XCTest only：**禁止** import SnapshotTesting / ViewInspector.
//   - @MainActor + 真实 AppState() 注入（AppState 是 @MainActor final ObservableObject，
//     init() 无参可直接构造）.
//   - mock InventoryRepositoryProtocol（scripted Result 队列 + invocations）.
//   - 端到端闭环 case：注入真实 AppState + RealWardrobeViewModel(appState:)（24.1 既有,
//     构造即订阅 sink）→ execute() → 断言 realWardrobeVM.inventory 反映展平后道具
//     （证明「24.2 写 → 24.1 sink → UI 数据」闭环，不经 view 渲染只断言 ViewModel 派生字段）.
//
// case 覆盖（≥4 epics.md 行 3369-3372 钦定 + 守护 case）：
//   1. happy 5 group（每 group count=1）→ currentInventory.count == 5（+ 端到端 24.1 sink 反映）
//   2. happy 空 groups → currentInventory == []（+ realWardrobeVM.inventory == []）
//   3. happy 多实例展平：1 group count=3 → currentInventory.count == 3，userCosmeticItemId 各不同
//   4. edge API 失败：repo 抛 APIError.network → execute rethrow + AppState 不被污染
//   5. edge 失败后重试：第一次抛错 → 第二次返 2 group → currentInventory.count == 2
//   6. 守护：applyInventory 单字段隔离（先 applyCurrentChest → applyInventory → chest 不变）
//   7. flatten 字段映射断言（slot/userCosmeticItemId/cosmeticItemId/name/rarity/assetUrl 填对）

import XCTest
@testable import PetApp

@MainActor
final class LoadInventoryUseCaseTests: XCTestCase {

    // MARK: - Helpers

    private func makeInstance(_ uciId: String, status: Int = 1) -> InventoryInstance {
        InventoryInstance(userCosmeticItemId: uciId, status: status)
    }

    private func makeGroup(
        cosmeticItemId: String,
        name: String = "道具",
        slot: Int = 1,
        rarity: Int = 1,
        iconUrl: String = "https://icon",
        assetUrl: String = "https://asset",
        instances: [InventoryInstance]
    ) -> InventoryGroup {
        InventoryGroup(
            cosmeticItemId: cosmeticItemId,
            name: name,
            slot: slot,
            rarity: rarity,
            iconUrl: iconUrl,
            assetUrl: assetUrl,
            count: instances.count,
            instances: instances
        )
    }

    /// 给 AppState 预填一个初始 inventory，让「失败时 AppState 不被污染」语义可断言.
    private func makeAppStateWithInitialInventory() -> AppState {
        let appState = AppState()
        appState.currentInventory = [
            HomeEquip(slot: 1, userCosmeticItemId: "init-uci", cosmeticItemId: "init-ci",
                      name: "初始道具", rarity: 1, assetUrl: "")
        ]
        return appState
    }

    // MARK: - case#1 happy（epics.md 行 3369）：5 group（每 group count=1）→ 展平 5 + 24.1 sink 反映

    func testExecuteFlattensFiveGroupsToFiveEquipsAndReflectsViaSink() async throws {
        let mock = MockInventoryRepository()
        let groups = (1...5).map {
            makeGroup(cosmeticItemId: "ci\($0)", name: "帽子\($0)", slot: 1, rarity: 2,
                      instances: [makeInstance("uci\($0)")])
        }
        mock.stubResponses = [.success(InventoryResponse(groups: groups))]
        let appState = AppState()
        // 端到端闭环：构造即订阅 appState.$currentInventory（Story 24.1 既有 sink）.
        let realVM = RealWardrobeViewModel(appState: appState)
        let useCase = DefaultLoadInventoryUseCase(repository: mock, appState: appState)

        try await useCase.execute()

        XCTAssertEqual(mock.invocations, 1)
        XCTAssertEqual(appState.currentInventory.count, 5, "5 group × 1 instance → 展平 5 HomeEquip")
        // 端到端断言：24.2 写 appState → 24.1 sink → RealWardrobeViewModel.inventory 反映.
        XCTAssertEqual(realVM.inventory.count, 5,
            "24.2 写 currentInventory → Story 24.1 既有 sink → realVM.inventory 反映 5 个 CosmeticItem")
    }

    // MARK: - case#2 happy（epics.md 行 3370）：空 groups → currentInventory == [] + sink 反映空

    func testExecuteEmptyGroupsProducesEmptyInventory() async throws {
        let mock = MockInventoryRepository()
        mock.stubResponses = [.success(InventoryResponse(groups: []))]
        let appState = AppState()
        let realVM = RealWardrobeViewModel(appState: appState)
        let useCase = DefaultLoadInventoryUseCase(repository: mock, appState: appState)

        try await useCase.execute()

        XCTAssertEqual(appState.currentInventory, [], "空背包 {groups: []} → currentInventory == []（不报错）")
        XCTAssertEqual(realVM.inventory, [], "空仓库 placeholder 路径：realVM.inventory == []")
    }

    // MARK: - case#3 happy（多实例展平）：1 group count=3 → 展平 3，userCosmeticItemId 各不同

    func testExecuteFlattensMultiInstanceGroupWithoutDedup() async throws {
        let mock = MockInventoryRepository()
        let group = makeGroup(
            cosmeticItemId: "ci-shared",
            name: "小黄帽",
            slot: 1,
            rarity: 1,
            instances: [makeInstance("uci-a"), makeInstance("uci-b"), makeInstance("uci-c", status: 2)]
        )
        mock.stubResponses = [.success(InventoryResponse(groups: [group]))]
        let appState = AppState()
        let useCase = DefaultLoadInventoryUseCase(repository: mock, appState: appState)

        try await useCase.execute()

        XCTAssertEqual(appState.currentInventory.count, 3,
            "1 group / 3 instance → 展平 3 HomeEquip（一 instance 一 HomeEquip，不去重不聚合）")
        let uciIds = Set(appState.currentInventory.map { $0.userCosmeticItemId })
        XCTAssertEqual(uciIds, ["uci-a", "uci-b", "uci-c"], "3 个 HomeEquip.userCosmeticItemId 各不相同")
        XCTAssertTrue(appState.currentInventory.allSatisfy { $0.cosmeticItemId == "ci-shared" },
            "同 group 多实例共享 group.cosmeticItemId（配置 id）")
    }

    // MARK: - case#4 edge（epics.md 行 3371，API 失败）：APIError.network → rethrow + AppState 不污染

    func testExecuteThrowsNetworkErrorPreservesAppStateInventory() async {
        let mock = MockInventoryRepository()
        mock.stubResponses = [.failure(APIError.network(underlying: URLError(.timedOut)))]
        let appState = makeAppStateWithInitialInventory()
        let useCase = DefaultLoadInventoryUseCase(repository: mock, appState: appState)

        do {
            try await useCase.execute()
            XCTFail("应抛 APIError.network")
        } catch let APIError.network(underlying) {
            XCTAssertEqual((underlying as? URLError)?.code, .timedOut)
        } catch {
            XCTFail("意外错误类型：\(error)")
        }

        XCTAssertEqual(appState.currentInventory.count, 1,
            "失败时 AppState.currentInventory 应保留上次值，不被污染")
        XCTAssertEqual(appState.currentInventory.first?.userCosmeticItemId, "init-uci")
    }

    // MARK: - case#5 edge（epics.md 行 3372，失败后手动重试）：第一次抛错 → 第二次返 2 group

    func testExecuteRetryAfterFailureLoadsFreshInventory() async throws {
        let mock = MockInventoryRepository()
        let groups = [
            makeGroup(cosmeticItemId: "ci1", instances: [makeInstance("uci1")]),
            makeGroup(cosmeticItemId: "ci2", instances: [makeInstance("uci2")])
        ]
        mock.stubResponses = [
            .failure(APIError.network(underlying: URLError(.notConnectedToInternet))),
            .success(InventoryResponse(groups: groups))
        ]
        let appState = AppState()
        let useCase = DefaultLoadInventoryUseCase(repository: mock, appState: appState)

        // 第一次：失败
        do {
            try await useCase.execute()
            XCTFail("第一次应抛错")
        } catch is APIError {
            // 预期
        }
        XCTAssertEqual(appState.currentInventory, [], "失败后 currentInventory 仍空（未污染）")

        // 第二次：重试成功（不缓存语义：重试 = 再 execute 一次拿新数据）
        try await useCase.execute()

        XCTAssertEqual(mock.invocations, 2, "重试 = 再发一次请求（不缓存）")
        XCTAssertEqual(appState.currentInventory.count, 2, "重试成功后展平 2 HomeEquip")
    }

    // MARK: - case#6 守护：applyInventory 单字段隔离（applyCurrentChest 后 chest 不变）

    func testExecuteAppliesInventoryWithoutTouchingOtherAppStateFields() async throws {
        let mock = MockInventoryRepository()
        mock.stubResponses = [.success(InventoryResponse(groups: [
            makeGroup(cosmeticItemId: "ci1", instances: [makeInstance("uci1")])
        ]))]
        let appState = AppState()
        // 先设非空 chest（验证 applyInventory 不波及其它字段）.
        let chest = HomeChest(id: "chest-x", status: .counting,
                              unlockAt: Date(timeIntervalSince1970: 1_700_000_000),
                              openCostSteps: 1000, remainingSeconds: 500)
        appState.applyCurrentChest(chest)
        let genBefore = appState.roomNavigationGeneration
        let useCase = DefaultLoadInventoryUseCase(repository: mock, appState: appState)

        try await useCase.execute()

        XCTAssertEqual(appState.currentInventory.count, 1, "currentInventory 已更新")
        XCTAssertEqual(appState.currentChest?.id, "chest-x", "applyInventory 不动 currentChest")
        XCTAssertEqual(appState.roomNavigationGeneration, genBefore,
            "applyInventory 不 bump roomNavigationGeneration（与 applyCurrentChest 同决策）")
    }

    // MARK: - case#7 flatten 字段映射断言（钦定字段全部填对）

    func testFlattenMapsAllHomeEquipFieldsCorrectly() {
        let group = makeGroup(
            cosmeticItemId: "ci-99",
            name: "炫彩披风",
            slot: 4,
            rarity: 3,
            iconUrl: "https://icon-99",
            assetUrl: "https://asset-99",
            instances: [makeInstance("uci-777", status: 2)]
        )
        let flattened = DefaultLoadInventoryUseCase.flatten(InventoryResponse(groups: [group]))

        XCTAssertEqual(flattened.count, 1)
        let equip = flattened[0]
        XCTAssertEqual(equip.slot, 4, "slot ← group.slot")
        XCTAssertEqual(equip.userCosmeticItemId, "uci-777", "userCosmeticItemId ← instance.userCosmeticItemId")
        XCTAssertEqual(equip.cosmeticItemId, "ci-99", "cosmeticItemId ← group.cosmeticItemId")
        XCTAssertEqual(equip.name, "炫彩披风", "name ← group.name")
        XCTAssertEqual(equip.rarity, 3, "rarity ← group.rarity")
        XCTAssertEqual(equip.assetUrl, "https://asset-99", "assetUrl ← group.assetUrl")
        // group.iconUrl + instance.status 无 HomeEquip 字段承载（丢弃）—— HomeEquip 仅 6 字段,
        // 编译期即保证不存在 iconUrl / status 字段，无需运行时断言.
    }
}
