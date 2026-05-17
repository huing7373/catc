// WardrobeRarityFilterTests.swift
// Story 24.3 AC「单元测试覆盖」：在既有分类 Tab 上叠加品质（N/R/SR/SSR）筛选的 ViewModel 行为单测.
//
// 测试基础设施约束（ADR-0002 §3.1 + 与 WardrobeViewScaffoldTests.swift 同模式）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）.
//   - 禁止 import ViewInspector / SnapshotTesting.
//   - 走 ViewModel 行为 + 派生属性断言（filteredCategoryItems / activeItem / selectedRarity /
//     selectedCosmeticId），不走 SwiftUI body 内省（与 37.9 走 currentCategoryItems / 24.1 走 inventory 同模式）.
//
// 测试文件归属决策（Task 1.5 / 5.1）：新建独立 WardrobeRarityFilterTests.swift（不并入
//   WardrobeViewScaffoldTests.swift）—— 把「品质筛选」这条新增能力的 case 内聚在一个文件，
//   既不撑大既有 12-case 文件，也与 InventoryResponseTests / RealWardrobeViewModelTests 等
//   按关注点分文件的既有组织风格一致.
//
// case 数：8（≥4 epics.md line 3390-3394 钦定 + 4 守护 case）.

import XCTest
@testable import PetApp

@MainActor
final class WardrobeRarityFilterTests: XCTestCase {

    /// 构造一个仅含 hat 类、品质分布 2 N + 2 R + 1 SR 的 mock inventory（无 SSR —— 用于「某品质 0 件」edge）.
    private func makeHatFixtureVM() -> MockWardrobeViewModel {
        let inventory: [CosmeticItem] = [
            CosmeticItem(id: "n1", name: "草帽A", category: .hat, rarity: .N, owned: true, iconEmoji: "🎩"),
            CosmeticItem(id: "n2", name: "草帽B", category: .hat, rarity: .N, owned: true, iconEmoji: "🎩"),
            CosmeticItem(id: "r1", name: "贝雷帽A", category: .hat, rarity: .R, owned: true, iconEmoji: "🎩"),
            CosmeticItem(id: "r2", name: "贝雷帽B", category: .hat, rarity: .R, owned: false, iconEmoji: "🎩"),
            CosmeticItem(id: "sr1", name: "皇冠", category: .hat, rarity: .SR, owned: true, iconEmoji: "🎩"),
            // bow 类掺 1 件，验证品质过滤是叠加在「当前分类」之上而非全 inventory.
            CosmeticItem(id: "bn1", name: "蝴蝶结", category: .bow, rarity: .N, owned: true, iconEmoji: "🎀"),
        ]
        return MockWardrobeViewModel(inventory: inventory, selectedCategory: .hat)
    }

    // MARK: - case#1 happy（epics.md line 3391 等价：逐一选品质 chip 各得对应子集）

    func testSelectRarityFiltersCurrentCategoryByRarity() {
        let vm = makeHatFixtureVM()
        XCTAssertEqual(vm.selectedCategory, .hat)
        XCTAssertNil(vm.selectedRarity, "默认 selectedRarity == nil（全部）")
        XCTAssertEqual(vm.currentCategoryItems.count, 5, "hat 类共 5 件（2N+2R+1SR）")

        vm.selectRarity(.N)
        XCTAssertEqual(vm.filteredCategoryItems.count, 2)
        XCTAssertTrue(vm.filteredCategoryItems.allSatisfy { $0.rarity == .N })

        vm.selectRarity(.R)
        XCTAssertEqual(vm.filteredCategoryItems.count, 2)
        XCTAssertTrue(vm.filteredCategoryItems.allSatisfy { $0.rarity == .R })

        vm.selectRarity(.SR)
        XCTAssertEqual(vm.filteredCategoryItems.count, 1)
        XCTAssertTrue(vm.filteredCategoryItems.allSatisfy { $0.rarity == .SR })
    }

    // MARK: - case#2 happy（epics.md line 3392「切到槽位按 slot 分组」等价：分类 Tab 承担 slot 维度）

    /// 验证既有分类 Tab 仍承担「槽位」维度 —— selectCategory(.bow) 后 currentCategoryItems /
    /// filteredCategoryItems 仅 bow 类（复用 24.1/37.9 既有契约，证明本 story reconcile 正确，
    /// 不需新建 slot segment）.
    func testCategoryTabStillCarriesSlotDimension() {
        let vm = makeHatFixtureVM()
        vm.selectCategory(.bow)
        XCTAssertEqual(vm.selectedCategory, .bow)
        XCTAssertTrue(vm.currentCategoryItems.allSatisfy { $0.category == .bow })
        XCTAssertTrue(vm.filteredCategoryItems.allSatisfy { $0.category == .bow })
        XCTAssertEqual(vm.filteredCategoryItems.count, 1, "bow 类仅 1 件 fixture")
    }

    // MARK: - case#3 happy（epics.md line 3393「切回全部平铺所有」）

    func testSelectRarityNilRestoresAllCurrentCategoryItems() {
        let vm = makeHatFixtureVM()
        vm.selectRarity(.SR)
        XCTAssertEqual(vm.filteredCategoryItems.count, 1)

        vm.selectRarity(nil)
        XCTAssertNil(vm.selectedRarity)
        XCTAssertEqual(vm.filteredCategoryItems, vm.currentCategoryItems,
            "「全部」态 = 当前分类全件，无品质过滤")
        XCTAssertEqual(vm.filteredCategoryItems.count, 5)
    }

    // MARK: - case#4 edge（epics.md line 3394「某品质 0 件」→ 折叠 = 空 grid）

    func testSelectRarityWithNoMatchingItemProducesEmptyFilteredItems() {
        let vm = makeHatFixtureVM()  // hat 类无 SSR
        vm.selectRarity(.SSR)
        XCTAssertTrue(vm.filteredCategoryItems.isEmpty,
            "hat 类无 SSR → filteredCategoryItems 空（view 层 ForEach 空集自然渲染空 grid，本 story 不做 placeholder）")
    }

    // MARK: - case#5 守护：切分类重置品质 + 清选中（既有副作用不回归）

    func testSelectCategoryResetsRarityAndClearsSelection() {
        let vm = makeHatFixtureVM()
        vm.selectRarity(.SR)
        vm.selectItem("x")
        XCTAssertEqual(vm.selectedRarity, .SR)
        XCTAssertEqual(vm.selectedCosmeticId, "x")

        vm.selectCategory(.bow)
        XCTAssertNil(vm.selectedRarity, "切分类回「全部品质」")
        XCTAssertNil(vm.selectedCosmeticId, "selectCategory 既有「清空 selectedCosmeticId」副作用不回归")
        XCTAssertEqual(vm.selectedCategory, .bow)
    }

    // MARK: - case#6 守护：selectRarity 单字段隔离（transient mutation 不波及其它字段）

    func testSelectRarityIsolatesSingleFieldMutation() {
        let inventory: [CosmeticItem] = [
            CosmeticItem(id: "r1", name: "贝雷帽", category: .hat, rarity: .R, owned: true, iconEmoji: "🎩"),
            CosmeticItem(id: "n1", name: "草帽", category: .hat, rarity: .N, owned: true, iconEmoji: "🎩"),
        ]
        let equipped: [CosmeticCategory: String] = [.hat: "r1"]
        let vm = MockWardrobeViewModel(
            catName: "测试猫",
            inventory: inventory,
            equipped: equipped,
            selectedCategory: .hat
        )
        let originalInventory = vm.inventory
        let originalEquipped = vm.equipped

        vm.selectRarity(.R)

        XCTAssertEqual(vm.inventory, originalInventory, "inventory 不变")
        XCTAssertEqual(vm.equipped, originalEquipped, "equipped 不变")
        XCTAssertEqual(vm.catName, "测试猫", "catName 不变")
        XCTAssertEqual(vm.selectedCategory, .hat, "selectedCategory 不变")
        XCTAssertEqual(vm.selectedRarity, .R, "仅 selectedRarity 变")
        XCTAssertNil(vm.selectedCosmeticId, "selectRarity 副作用：清 selectedCosmeticId")
    }

    // MARK: - case#7 守护：activeItem 与品质过滤一致（预览区不显示被过滤掉的道具）

    func testActiveItemStaysConsistentWithRarityFilter() {
        let inventory: [CosmeticItem] = [
            CosmeticItem(id: "A", name: "贝雷帽", category: .hat, rarity: .R, owned: true, iconEmoji: "🎩"),
            CosmeticItem(id: "B", name: "草帽", category: .hat, rarity: .N, owned: true, iconEmoji: "🎩"),
        ]
        let vm = MockWardrobeViewModel(inventory: inventory, selectedCategory: .hat)
        vm.selectItem("A")
        XCTAssertEqual(vm.activeItem?.id, "A", "选中 A → activeItem 是 A")

        vm.selectRarity(.N)  // A（R 品质）被过滤掉
        XCTAssertNotEqual(vm.activeItem?.id, "A",
            "A 被品质过滤掉后 activeItem 不应仍指向 A（预览区与可见 grid 一致）")
        // fallback 必落在 filteredCategoryItems 内（或 nil）.
        if let active = vm.activeItem {
            XCTAssertTrue(vm.filteredCategoryItems.contains(where: { $0.id == active.id }),
                "activeItem 必在 filteredCategoryItems 内")
            XCTAssertEqual(active.rarity, .N, "过滤后 activeItem 必是 N 品质")
        }
    }

    // MARK: - case#8 守护：Mock 子类零 edit 自动继承（基类新增能力子类无 edit 即生效）

    func testMockSubclassInheritsRarityFilterWithZeroEdit() {
        let vm = MockWardrobeViewModel()  // 默认 18 件 WardrobeScaffoldDefaults seed
        vm.selectCategory(.hat)
        vm.selectRarity(.R)
        XCTAssertFalse(vm.filteredCategoryItems.isEmpty, "hat 类含 R 品质（h1/h5/h6）")
        XCTAssertTrue(vm.filteredCategoryItems.allSatisfy { $0.category == .hat },
            "全 hat 类（基类能力子类零 edit 继承，37.9 mock 数据契约不回归）")
        XCTAssertTrue(vm.filteredCategoryItems.allSatisfy { $0.rarity == .R },
            "全 R 品质")
    }
}
