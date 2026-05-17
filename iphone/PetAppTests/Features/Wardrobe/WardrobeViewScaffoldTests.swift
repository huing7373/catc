// WardrobeViewScaffoldTests.swift
// Story 37.9 AC7: WardrobeScaffoldView + WardrobeViewModel class 层次单元测试.
//
// 测试基础设施约束（与 Story 2.7 + ADR-0002 §3.1 衔接）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）.
//   - 不引 ViewInspector / SnapshotTesting.
//   - 走 ViewModel 行为 + invocations 数组断言；不走 SwiftUI body 内省.
//
// case 数：10（≥4 epic AC line 4789-4792 + 6 守护 case 预防 Story 37.7 / 37.8 lesson 反例）.

import XCTest
@testable import PetApp

@MainActor
final class WardrobeViewScaffoldTests: XCTestCase {

    // MARK: - case#1 happy: 切到饰品 Tab → currentCategoryItems 仅 bow 道具（epic AC line 4789）

    func testSelectCategoryFiltersInventoryByCategory() {
        let vm = MockWardrobeViewModel()
        // 默认 hat
        XCTAssertEqual(vm.selectedCategory, .hat)
        XCTAssertTrue(vm.currentCategoryItems.allSatisfy { $0.category == .hat })

        vm.selectCategory(.bow)
        XCTAssertEqual(vm.selectedCategory, .bow)
        XCTAssertTrue(vm.currentCategoryItems.allSatisfy { $0.category == .bow })
        XCTAssertFalse(vm.currentCategoryItems.isEmpty, "bow 分类至少 1 件 mock")
    }

    // MARK: - case#2 happy: 点选 owned 道具 → onEquipTap 触发 + Mock 改 equipped 映射（epic AC line 4790）

    func testOnEquipTapWithOwnedItemTogglesEquipped() {
        let vm = MockWardrobeViewModel()
        let bowItem = vm.inventory.first { $0.category == .bow && $0.id == "b2" }!
        XCTAssertTrue(bowItem.owned)
        XCTAssertNotEqual(vm.equipped[.bow], bowItem.id, "默认 b1 装备，b2 未装备")

        vm.onEquipTap(item: bowItem)
        XCTAssertEqual(vm.equipped[.bow], bowItem.id, "装备后 equipped[.bow] = b2")
        XCTAssertEqual(vm.invocations, [.equipTap(itemId: bowItem.id)])

        // 再点一次 → 卸下
        vm.onEquipTap(item: bowItem)
        XCTAssertNil(vm.equipped[.bow], "再点同 item 卸下 → equipped[.bow] = nil")
    }

    // MARK: - case#3 happy: 点选 unowned 道具 → onEquipTap 触发但 equipped 不动（epic AC line 4791）

    /// 验证 epic AC "点选 unowned 道具 → 装备按钮 disabled" —— 按钮 disabled 由 view 层 isEnabled 判断；
    /// 但即便 ViewModel.onEquipTap 被绕过 disabled 调用（如 UITest），equipped 映射也不变（Mock 路径内置 owned check）.
    func testOnEquipTapWithUnownedItemDoesNotMutateEquipped() {
        let vm = MockWardrobeViewModel()
        let unownedHat = vm.inventory.first { $0.category == .hat && !$0.owned }!
        let originalEquipped = vm.equipped

        vm.onEquipTap(item: unownedHat)
        XCTAssertEqual(vm.equipped, originalEquipped, "unowned 道具点击 equipped 不变（Mock 路径内置 owned check）")
        XCTAssertEqual(vm.invocations, [.equipTap(itemId: unownedHat.id)])
    }

    // MARK: - case#4 edge: items 空数组 → currentCategoryItems / activeItem 空（epic AC line 4792）

    func testEmptyInventoryProducesEmptyCurrentCategoryItemsAndNilActiveItem() {
        let vm = MockWardrobeViewModel(inventory: [])
        XCTAssertTrue(vm.currentCategoryItems.isEmpty)
        XCTAssertNil(vm.activeItem)
    }

    // MARK: - case#5 happy: selectItem 切换 selectedCosmeticId → activeItem 反映新选中

    func testSelectItemUpdatesActiveItem() {
        let vm = MockWardrobeViewModel()
        vm.selectCategory(.hat)
        let originalActive = vm.activeItem
        let candidate = vm.currentCategoryItems.first { $0.id != originalActive?.id }!

        vm.selectItem(candidate.id)
        XCTAssertEqual(vm.selectedCosmeticId, candidate.id)
        XCTAssertEqual(vm.activeItem?.id, candidate.id)
    }

    // MARK: - case#6 happy: selectCategory 副作用清空 selectedCosmeticId

    /// 验证 selectCategory 切分类后 selectedCosmeticId 被置 nil（避免老分类的 id 在新分类查不到导致 activeItem 为 nil 但 selected 仍残留）.
    func testSelectCategoryClearsSelectedCosmeticId() {
        let vm = MockWardrobeViewModel()
        vm.selectItem("h1")
        XCTAssertEqual(vm.selectedCosmeticId, "h1")

        vm.selectCategory(.bow)
        XCTAssertNil(vm.selectedCosmeticId, "切分类后 selectedCosmeticId 必清空")
    }

    // MARK: - case#7 守护: RealWardrobeViewModel 构造注入 AppState 不 crash + onEquipTap override 不 fatalError

    /// 防 RealWardrobeViewModel 漏 override onEquipTap 时本测试立刻 fail（fatalError 在测试中 → trap）.
    /// 与 Story 37.8 RoomViewScaffoldTests case#5 同模式.
    func testRealWardrobeViewModelConstructionAndAbstractMethodDoesNotCrash() {
        let appState = AppState()
        let vm = RealWardrobeViewModel(appState: appState)
        XCTAssertEqual(vm.catName, WardrobeScaffoldDefaults.catName, "init 走 defaults seed (catName)")
        // Story 24.1 AC5：inventory seed 改 []（真实 inventory 语义；非 37.9 mock 18 件）.
        // appState.currentInventory 启动时为空 → subscribeInventory sink 派发 [] → inventory == [].
        XCTAssertTrue(vm.inventory.isEmpty, "Story 24.1 AC5: init/空 appState → inventory == [] (空仓库占位)")

        // onEquipTap override 验证不进入基类 fatalError 路径（用 mock item，inventory 现为空）.
        // round 1 P1 fix 后行为变更：Real 路径与 Mock 一样本地 toggle equipped（详见 case#11 守护测试）.
        let testItem = CosmeticItem(id: "h1", name: "贝雷帽", category: .hat, rarity: .R, owned: true, iconEmoji: "🎩")
        vm.onEquipTap(item: testItem)
    }

    // MARK: - case#8 守护: Real init 必 seed scaffold defaults（lesson 预防性应用）

    /// 与 Story 37.8 testRealRoomViewModelInitSeedsRoomScaffoldDefaults 同模式 ——
    /// 防未来 Claude 重构 init 时漏 seed equipped / catName / selectedCategory.
    /// lesson: 2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md
    ///
    /// **Story 24.1 AC5 有意偏离**：catName / equipped / selectedCategory 仍 seed
    /// WardrobeScaffoldDefaults（保留 lesson 精神：hydrate 前 UI 不空白崩溃）；
    /// 但 `inventory` seed 改 `[]`（真实 inventory 语义下空仓库 placeholder 才是正确初值，
    /// hydrate 前显示 mock 18 件假装扮会误导用户）—— 本断言改为验证 inventory 空 + 其余 3 字段仍 seed.
    func testRealWardrobeViewModelInitSeedsScaffoldDefaults() {
        // parameterless init 路径
        let vm1 = RealWardrobeViewModel()
        XCTAssertTrue(vm1.inventory.isEmpty, "Story 24.1 AC5: inventory seed [] (真实语义空仓库占位)")
        XCTAssertEqual(vm1.catName, WardrobeScaffoldDefaults.catName)
        XCTAssertEqual(vm1.equipped, WardrobeScaffoldDefaults.equipped)
        XCTAssertEqual(vm1.selectedCategory, WardrobeScaffoldDefaults.selectedCategory)

        // init(appState:) 路径（空 appState → sink 派发 [] → inventory 仍空）
        let vm2 = RealWardrobeViewModel(appState: AppState())
        XCTAssertTrue(vm2.inventory.isEmpty, "Story 24.1 AC5: 空 appState → inventory == []")
        XCTAssertEqual(vm2.equipped, WardrobeScaffoldDefaults.equipped)
    }

    // MARK: - case#9 守护: catName 派生自 appState.currentPet（hydrate + reset 路径）

    /// 与 Story 37.7 testRealHomeViewModelGreetingFallsBackOnReset 同模式 ——
    /// 防未来 Claude 重构时把 catName sink 改一次性 hydrate 让 reset 后残留旧 pet 名.
    /// lesson: 2026-04-30-published-derived-state-needs-publisher-subscription.md
    ///
    /// **关键说明**：本测试与 Story 37.8 testRealRoomViewModelHostCatNameDoesNotDeriveFromCurrentPet 看似相反 ——
    /// Wardrobe 域 catName 派生自 currentPet 是合法（"看自己衣柜"语义），Room 域 hostCatName 派生自 currentPet 是错误（"看别人房间"语境）.
    func testRealWardrobeViewModelCatNameDerivesFromCurrentPet() {
        let appState = AppState()
        let vm = RealWardrobeViewModel(appState: appState)
        XCTAssertEqual(vm.catName, WardrobeScaffoldDefaults.catName, "appState.currentPet nil → fallback 占位")

        // hydrate 路径：写入 currentPet → catName 同步派生
        let testPet = HomePet(id: "p1", petType: 1, name: "测试猫", currentState: .rest, equips: [])
        appState.currentPet = testPet
        XCTAssertEqual(vm.catName, "测试猫")

        // reset 路径：appState.reset() 把 currentPet 置 nil → catName 即时 fallback 占位（不残留旧值）
        appState.reset()
        XCTAssertEqual(vm.catName, WardrobeScaffoldDefaults.catName, "reset 后 catName 必回占位（防 stale）")
    }

    // MARK: - case#10 守护: bind(appState:) 是同步入口（lesson 2026-04-30-onappear-vs-task-sync-bind 预防性应用）

    /// 防未来 Claude 把 bind 改成 async 路径让 RootView .onAppear 触发后第一帧 ViewModel 仍未连上 AppState.
    /// 与 Story 37.8 testRealRoomViewModelBindAppStateIsSynchronous 同模式.
    func testRealWardrobeViewModelBindAppStateIsSynchronous() {
        let appState = AppState()
        let testPet = HomePet(id: "p1", petType: 1, name: "测试猫", currentState: .rest, equips: [])
        appState.currentPet = testPet

        let vm = RealWardrobeViewModel()  // parameterless init 路径
        XCTAssertEqual(vm.catName, WardrobeScaffoldDefaults.catName, "bind 前 catName = defaults")

        vm.bind(appState: appState)  // 同步路径
        XCTAssertEqual(vm.catName, "测试猫", "bind 后立即派生（无 RunLoop tick 等待）")
    }

    // MARK: - case#11 守护: RealWardrobeViewModel.onEquipTap 必须本地 toggle equipped（防 P1 no-op 回归）

    /// round 1 P1 fix（codex review 2026-04-30）守护测试 ——
    /// 防未来 Claude 重构时把 onEquipTap override 改回"仅 log 不 mutate"让 production 按钮 no-op.
    /// lesson: `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`
    /// （Story 37.7 RealHomeViewModel.onJoinTap 同模式 lesson 第二次复犯; 本 case 把契约钉成机器可校验）.
    ///
    /// 本测试覆盖三件事：
    ///   1) owned + 未装备 → 装备（equipped[bow] = b2）
    ///   2) owned + 已装备同 id → 卸下（equipped[bow] = nil）
    ///   3) unowned 道具 → equipped 不变（owned guard 保留）
    /// Story 24.1 AC5：Real init 改 inventory seed []，本测试不再依赖 seeded mock inventory，
    /// 改为直接构造 CosmeticItem 传入 onEquipTap（测试真实意图是「override 必须 mutate equipped」
    /// 而非依赖 inventory 内容，构造法更稳健）.
    func testRealWardrobeViewModelOnEquipTapTogglesEquipped() {
        let appState = AppState()
        let vm = RealWardrobeViewModel(appState: appState)

        // case 1: owned + 未装备 → 装备
        let bowB2 = CosmeticItem(id: "b2", name: "星星发夹", category: .bow, rarity: .R, owned: true, iconEmoji: "🎀")
        XCTAssertTrue(bowB2.owned, "fixture 前提：b2 owned")
        XCTAssertNotEqual(vm.equipped[.bow], bowB2.id, "默认 b1 装备，b2 未装备")
        vm.onEquipTap(item: bowB2)
        XCTAssertEqual(vm.equipped[.bow], bowB2.id, "Real path: 装备后 equipped[.bow] = b2 (不再是 no-op)")

        // case 2: owned + 已装备同 id → 卸下
        vm.onEquipTap(item: bowB2)
        XCTAssertNil(vm.equipped[.bow], "Real path: 再点同 item 卸下 → equipped[.bow] = nil")

        // case 3: unowned → equipped 不变（owned guard）
        let unownedHat = CosmeticItem(id: "h6", name: "警官帽", category: .hat, rarity: .R, owned: false, iconEmoji: "🎩")
        let originalHat = vm.equipped[.hat]
        vm.onEquipTap(item: unownedHat)
        XCTAssertEqual(vm.equipped[.hat], originalHat, "Real path: unowned 道具点击 equipped[.hat] 不变（与 Mock 一致）")
    }
}
