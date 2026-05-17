// RealWardrobeViewModelTests.swift
// Story 24.1 AC「单元测试覆盖」: RealWardrobeViewModel 真实 inventory 派生单测.
//
// 测试基础设施约束（ADR-0002 §3.1 钦定 —— 不可违反）：
//   - XCTest only：**禁止** import SnapshotTesting / ViewInspector.
//   - @MainActor + 真实 AppState() 注入（AppState 是 @MainActor final ObservableObject，init() 无参可直接构造；
//     hydrate 用 `appState.currentInventory = [...]` 直设 —— 本 story 只关心 inventory sink）.
//   - 走 ViewModel 行为断言 + 纯函数边界值断言；不走 SwiftUI body 内省.
//
// case 数：7（≥4 提案 §5.4 line 417-421 钦定 + 守护 case A reset + 守护 case B bind 幂等
//   + 私有 mapping 经 public inventory sink 路径间接断言）.

import XCTest
@testable import PetApp

@MainActor
final class RealWardrobeViewModelTests: XCTestCase {

    // MARK: - helper：构造 HomeEquip fixture（slot/rarity 显式，便于断言 mapping）

    private func makeEquip(
        slot: Int,
        userCosmeticItemId: String,
        cosmeticItemId: String = "ci",
        name: String = "道具",
        rarity: Int = 1
    ) -> HomeEquip {
        HomeEquip(
            slot: slot,
            userCosmeticItemId: userCosmeticItemId,
            cosmeticItemId: cosmeticItemId,
            name: name,
            rarity: rarity,
            assetUrl: ""
        )
    }

    // MARK: - case#1 happy（提案 line 418）：5 个 hat 类 HomeEquip → 5 个 CosmeticItem 且 currentCategoryItems(.hat) 长度 = 5

    func testHydrateFiveHatEquipsProducesFiveHatCosmeticItems() {
        let appState = AppState()
        let vm = RealWardrobeViewModel(appState: appState)
        XCTAssertTrue(vm.inventory.isEmpty, "init/空 appState → inventory == []（空仓库占位）")

        appState.currentInventory = (1...5).map {
            makeEquip(slot: 1, userCosmeticItemId: "uci_hat_\($0)", name: "帽子\($0)", rarity: 2)
        }

        XCTAssertEqual(vm.inventory.count, 5, "5 个 hat HomeEquip → 5 个 CosmeticItem")
        XCTAssertTrue(vm.inventory.allSatisfy { $0.category == .hat }, "slot 1 → .hat")
        XCTAssertTrue(vm.inventory.allSatisfy { $0.owned }, "inventory 全为已拥有实例 owned == true")
        XCTAssertTrue(vm.inventory.allSatisfy { $0.rarity == .R }, "rarity int 2 → .R")
        XCTAssertEqual(Set(vm.inventory.map { $0.id }).count, 5, "实例级 id 各独立（userCosmeticItemId，不去重聚合）")

        vm.selectCategory(.hat)
        XCTAssertEqual(vm.currentCategoryItems.count, 5, "selectedCategory=.hat → currentCategoryItems 长度 = 5")
    }

    // MARK: - case#2 happy（提案 line 419）：切 selectedCategory → currentCategoryItems 渲染对应分类（验 mapping category 字段）

    func testSwitchSelectedCategoryFiltersMappedItemsCorrectly() {
        let appState = AppState()
        let vm = RealWardrobeViewModel(appState: appState)

        // slot 1 → .hat (3 件) / slot 4 → .scarf (2 件) / slot 6 → .outfit (1 件)
        appState.currentInventory = [
            makeEquip(slot: 1, userCosmeticItemId: "uci_h1"),
            makeEquip(slot: 1, userCosmeticItemId: "uci_h2"),
            makeEquip(slot: 1, userCosmeticItemId: "uci_h3"),
            makeEquip(slot: 4, userCosmeticItemId: "uci_sc1"),
            makeEquip(slot: 4, userCosmeticItemId: "uci_sc2"),
            makeEquip(slot: 6, userCosmeticItemId: "uci_o1"),
        ]

        XCTAssertEqual(vm.inventory.count, 6, "6 个实例全部映射（不丢实例）")

        vm.selectCategory(.scarf)
        XCTAssertEqual(vm.currentCategoryItems.count, 2, "slot 4 → .scarf；切 .scarf 后 2 件")
        XCTAssertTrue(vm.currentCategoryItems.allSatisfy { $0.category == .scarf })

        vm.selectCategory(.outfit)
        XCTAssertEqual(vm.currentCategoryItems.count, 1, "slot 6 → .outfit；切 .outfit 后 1 件")

        vm.selectCategory(.hat)
        XCTAssertEqual(vm.currentCategoryItems.count, 3, "slot 1 → .hat；切 .hat 后 3 件")
    }

    // MARK: - case#3 happy（提案 line 420）：currentInventory 为空 → inventory == [] （非 WardrobeScaffoldDefaults.inventory）

    func testEmptyCurrentInventoryProducesEmptyInventoryNotMockDefaults() {
        let appState = AppState()
        let vm = RealWardrobeViewModel(appState: appState)

        // 启动后初值即空
        XCTAssertEqual(vm.inventory, [], "空 appState → inventory == [] (空仓库 placeholder，非 mock 18 件)")
        XCTAssertNotEqual(vm.inventory, WardrobeScaffoldDefaults.inventory, "**不**退回 WardrobeScaffoldDefaults.inventory")

        // 先 hydrate 非空，再清空 → 仍回 []（不残留 / 不退 mock）
        appState.currentInventory = [makeEquip(slot: 1, userCosmeticItemId: "uci_1")]
        XCTAssertEqual(vm.inventory.count, 1)
        appState.currentInventory = []
        XCTAssertEqual(vm.inventory, [], "清空后 inventory 回 [] (空态 placeholder)")
        XCTAssertNotEqual(vm.inventory, WardrobeScaffoldDefaults.inventory)
    }

    // MARK: - case#4 edge（提案 line 421）：badgeText(forCount:) 纯函数边界值（不经 view）

    func testBadgeTextClampBoundaryValues() {
        XCTAssertEqual(CosmeticCategory.badgeText(forCount: 0), "0", "count = 0 → \"0\"")
        XCTAssertEqual(CosmeticCategory.badgeText(forCount: 99), "99", "count = 99 → \"99\"（边界含等于不 clamp）")
        XCTAssertEqual(CosmeticCategory.badgeText(forCount: 100), "99+", "count = 100 → \"99+\"")
        XCTAssertEqual(CosmeticCategory.badgeText(forCount: 9999), "99+", "count > 99 → \"99+\"")
        XCTAssertEqual(CosmeticCategory.badgeText(forCount: 1), "1", "count = 1 → \"1\"")
    }

    // MARK: - case#5 守护 A（reset 不回归）：hydrate 非空 → reset → sink 派发 [] → inventory == []

    /// 防 lesson `2026-04-30-published-derived-state-needs-publisher-subscription` 回归 ——
    /// sink 路径必须保持（不可退化为「init 一次性读 appState.currentInventory」），否则 reset 后 inventory stale.
    func testResetClearsInventoryViaSinkNotStale() {
        let appState = AppState()
        let vm = RealWardrobeViewModel(appState: appState)

        appState.currentInventory = [
            makeEquip(slot: 1, userCosmeticItemId: "uci_1"),
            makeEquip(slot: 4, userCosmeticItemId: "uci_2"),
        ]
        XCTAssertEqual(vm.inventory.count, 2, "hydrate 后 inventory 非空")

        appState.reset()  // reset() 把 currentInventory = []
        XCTAssertEqual(vm.inventory, [], "reset 后 sink 派发 [] → inventory == []（不残留旧 inventory）")
        XCTAssertNotEqual(vm.inventory, WardrobeScaffoldDefaults.inventory, "reset 后**不**退回 mock defaults")
    }

    // MARK: - case#6 守护 B（bind 幂等）：无参构造 → bind(appState:) → 后续 currentInventory 变化能派生

    /// 验证 Story 37.9 bind idempotent guard 不破坏 inventory sink ——
    /// 无参 init → inventory == []（AC5 seed）；bind 后 appState.currentInventory 变化经 sink 派生.
    func testBindAppStateThenInventoryDerivation() {
        let appState = AppState()
        appState.currentInventory = [makeEquip(slot: 1, userCosmeticItemId: "uci_pre", name: "预置帽")]

        let vm = RealWardrobeViewModel()  // 无参构造（RootView @StateObject 模式）
        XCTAssertEqual(vm.inventory, [], "bind 前 inventory == [] (AC5 seed)")

        vm.bind(appState: appState)  // 同步 bind → 立即派发当前 currentInventory
        XCTAssertEqual(vm.inventory.count, 1, "bind 后立即派生 appState 既有 inventory（无 RunLoop tick 等待）")
        XCTAssertEqual(vm.inventory.first?.name, "预置帽")

        // bind 后后续变化继续派生
        appState.currentInventory = (1...3).map { makeEquip(slot: 1, userCosmeticItemId: "uci_\($0)") }
        XCTAssertEqual(vm.inventory.count, 3, "bind 后 currentInventory 变化继续经 sink 派生")

        // 再 bind 一次（幂等 guard）→ 不重订阅 / 不破坏 sink
        vm.bind(appState: appState)
        appState.currentInventory = [makeEquip(slot: 1, userCosmeticItemId: "uci_after")]
        XCTAssertEqual(vm.inventory.count, 1, "重复 bind 后 sink 仍单次派生（幂等 guard 不破坏）")
    }

    // MARK: - case#7 守护：未知 slot / 未知 rarity fallback 不丢实例不 crash（V1 §8.2「已拥有不得静默丢失」client 侧延续）

    func testUnknownSlotAndRarityFallbackDoesNotDropInstances() {
        let appState = AppState()
        let vm = RealWardrobeViewModel(appState: appState)

        appState.currentInventory = [
            makeEquip(slot: 2, userCosmeticItemId: "uci_gloves"),   // slot 2 (gloves) → .bow 兜底
            makeEquip(slot: 3, userCosmeticItemId: "uci_glasses"),  // slot 3 (glasses) → .bow 兜底
            makeEquip(slot: 99, userCosmeticItemId: "uci_other"),   // slot 99 (other) → .bow 兜底
            makeEquip(slot: 7, userCosmeticItemId: "uci_tail"),     // slot 7 (tail) → .bow 兜底
            makeEquip(slot: 5, userCosmeticItemId: "uci_back"),     // slot 5 (back) → .bg
            makeEquip(slot: 1, userCosmeticItemId: "uci_unk_rar", rarity: 999),  // 未知 rarity → .N
        ]

        XCTAssertEqual(vm.inventory.count, 6, "未知 slot/rarity 不丢实例（全部映射）")

        let bowItems = vm.inventory.filter { $0.category == .bow }
        XCTAssertEqual(bowItems.count, 4, "slot 2/3/7/99 全归 .bow 兜底桶（不丢实例）")
        XCTAssertEqual(vm.inventory.filter { $0.category == .bg }.count, 1, "slot 5 → .bg")

        let unknownRarityItem = vm.inventory.first { $0.id == "uci_unk_rar" }
        XCTAssertEqual(unknownRarityItem?.rarity, .N, "未知 rarity int → .N（视觉降级不 crash）")
    }
}
