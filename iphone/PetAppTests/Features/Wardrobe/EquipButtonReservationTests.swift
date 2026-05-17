// EquipButtonReservationTests.swift
// Story 24.5: 把 Story 37.9 已交付的 `previewCard.equipButton`「节点 9 Story 27.1 接入契约」
// 钉成机器可校验守护测试。
//
// === 范围 / reconcile 说明（必读 —— 防 review 误判「漏实现 per-row disabled 按钮 / 没退回 disabled」）===
//
// epics.md §Story 24.5 写于 ADR-0009 推翻 Sheet 主入口之前，字面要求：在 InventoryView 的
// instance 列表每行右侧显示一个「预留 / disabled」的"穿戴（节点 9）"按钮，节点 8 阶段
// 「disabled + 灰 + onTap no-op」，节点 9 Story 27.1 变 enabled + 触发 EquipUseCase。
//
// 当前已交付形态（Story 37.9 `WardrobeScaffoldView` + 24.1 `RealWardrobeViewModel` +
// 24.2 `LoadInventoryUseCase` + 24.3 品质筛选，全 done）**没有** InventoryView / instance
// flat 列表 / per-instance 行按钮，而是 `previewCard` 内单个 `equipButton`
// （`PrimaryButton(fullWidth:true)`，作用于"当前预览的 active item"）。该按钮 37.9 起就：
//   - 有稳定 a11y 锚 `AccessibilityID.Wardrobe.equipButton`（= "wardrobeEquipButton"）
//   - 有 enable 规则 `isEnabled = activeItem?.owned ?? false`
//   - 有 label 状态机 `equippedNow ? "✓ 已装备 (点击卸下)" : "装备"`
//   - 有本地 placeholder toggle（Mock + Real `onEquipTap` 均本地 mutate `equipped`，
//     37.9 round1 codex review P1 fix —— 非 no-op）
//
// 故 epics.md §24.5「预留一个 disabled 占位按钮位置」的工程意图（= 节点 9 接入点已就位、
// 不返工布局）在 37.9 落地 `equipButton` 时已**结构性满足**。本 story 不按字面新建
// per-row disabled 按钮、不退回 disabled、不改 `onEquipTap` 为 no-op（会与已 done 的
// 37.9/24.1/24.2/24.3 架构冲突 + 越界 + 违反 severity 1 lesson
// `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`，已复犯一次）。
//
// 本文件 = Story 24.5 唯一产物：**零 production 代码改动**，仅把上述 4 项契约
// （a11y 锚 / enable 规则 / label 状态机 / placeholder 必须可用）钉成机器可校验守护，
// 供 Story 27.1 直接接入（仅换 `onEquipTap` 内部实装）+ review 自解释不误判。
// 详见 24-5 story 文件 Dev Notes「epics.md 偏离登记」+「给 Story 27.1 的接入说明」。
//
// === 测试基础设施约束（ADR-0002 §3.1 + 与 WardrobeViewScaffoldTests.swift 同模式）===
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）；@MainActor 测试。
//   - **禁止** import SnapshotTesting / ViewInspector。
//   - 走 ViewModel 行为 + 派生属性 + `AccessibilityID` 常量断言，**不**走 SwiftUI body 内省。
//   - label 状态机契约通过断言驱动量（`activeItem?.owned` / `isEquipped(_:)`）+ 把
//     `equipButton`（WardrobeScaffoldView.swift line 190-194）的 label 决策逻辑复刻为
//     测试内纯函数（`equipButtonContractTitle`）实现，**不**实例化 view 读 label。
//
// === 与 WardrobeViewScaffoldTests case#11 的关系（有意冗余，不删/不改 case#11）===
//   case#11 是 37.9 round1 P1 fix 防回归守护；本文件 AC4 是带「节点 9 接入前置」语义注释
//   的等价自包含守护，使「24.5 预留契约」对 Story 27.1 dev 与 review 自解释（lesson 元规则：
//   契约机器可校验且语义自解释，跨 story 传染时不蒸发）。二者关注点不同，并存是有意决策。
//
// case 数：8（a11y 锚契约 / owned-enabled / unowned-disabled / 未装备 label /
//   已装备 label+grid对勾 / 节点8 onTap 本地 toggle 非 no-op / Real placeholder 必须可用 /
//   Mock-Real 同语义）。

import XCTest
@testable import PetApp

@MainActor
final class EquipButtonReservationTests: XCTestCase {

    // MARK: - 测试内纯函数：复刻 equipButton label 决策契约（WardrobeScaffoldView.swift line 190-194）

    /// 复刻 `WardrobeScaffoldView.equipButton` 的 label 决策逻辑（line 190-194）为测试内纯函数：
    /// `equippedNow = (active != nil) && state.isEquipped(active)`；
    /// `title = equippedNow ? "✓ 已装备 (点击卸下)" : "装备"`。
    ///
    /// **不**实例化 SwiftUI view 读 label（ADR-0002 §3.1 禁 ViewInspector）；
    /// 用此纯函数 + ViewModel 驱动量断言钉死「节点 9 Story 27.1 必须保持的 label 状态机契约」。
    /// 若 production line 194 字面值改动 → 节点 9 接入语义破坏；本函数与 production 字面值
    /// 同步是有意的契约副本（注释钉死，review 比对此处与 line 194 一致即可）。
    private func equipButtonContractTitle(vm: WardrobeViewModel) -> String {
        let equippedNow: Bool = {
            guard let active = vm.activeItem else { return false }
            return vm.isEquipped(active)
        }()
        return equippedNow ? "✓ 已装备 (点击卸下)" : "装备"
    }

    // MARK: - case#1 守护: a11y 锚 = 节点 9 Story 27.1 接入契约（AC1）

    /// 守护 `AccessibilityID.Wardrobe.equipButton == "wardrobeEquipButton"`。
    ///
    /// **此锚是 Story 27.1 `onEquipTap` 激活点 + Epic 28 E2E / UITest 锚定点，
    /// 是节点 9 接入契约，不可改。** 改动此常量值 = 破坏节点 9 接入契约（27.1 dev
    /// 找不到按钮 / E2E 锚失效），本 case 立刻 fail。
    ///
    /// 复用断言已存在于 `AccessibilityIDTests.swift` line 78；本 case 是带「Story 27.1
    /// 接入锚契约」语义注释的等价同断言（语义文档化，不重复造锚常量；二者并存有意 ——
    /// AccessibilityIDTests 防常量表整体回归，本 case 让「24.5 预留契约」自包含可读）。
    func testEquipButtonAccessibilityIDIsNode9IntegrationAnchorContract() {
        // 节点 9 接入契约锚：Story 27.1 据此值 wire 真实 EquipUseCase；Epic 28 E2E/UITest 据此值定位按钮。
        XCTAssertEqual(
            AccessibilityID.Wardrobe.equipButton,
            "wardrobeEquipButton",
            "节点 9 接入契约：此 a11y 锚是 Story 27.1 onEquipTap 激活点 + Epic 28 E2E/UITest 锚定点，不可改"
        )
    }

    // MARK: - case#2 守护: owned active item → equipButton enabled（AC2）

    /// `equipButton.isEnabled = activeItem?.owned ?? false`（WardrobeScaffoldView.swift line 196）。
    /// owned == true 的 active item → enable 驱动量 true（节点 9 Story 27.1 据此让按钮 enabled）。
    ///
    /// 注：epics.md 字面要求节点 8 阶段按钮 disabled；本 story reconcile 为「37.9 既有
    /// enabled placeholder」（37.9 round1 P1 钉死占位必须可用，禁退 disabled —— 见文件头 +
    /// 24-5 story Dev Notes 偏离登记）。Story 27.1 接 EquipUseCase 后此 enable 规则**不变**
    /// （按钮一直 enabled，换的是点击后干什么）。
    func testOwnedActiveItemDrivesEquipButtonEnabled() {
        let ownedHat = CosmeticItem(id: "h1", name: "贝雷帽", category: .hat, rarity: .R, owned: true, iconEmoji: "🎩")
        let vm = MockWardrobeViewModel(inventory: [ownedHat], equipped: [:], selectedCategory: .hat)
        vm.selectItem(ownedHat.id)

        XCTAssertEqual(vm.activeItem?.id, ownedHat.id, "fixture 前提：active item 是注入的 owned hat")
        XCTAssertEqual(
            vm.activeItem?.owned, true,
            "节点 9 enable 契约：active.owned == true → equipButton.isEnabled true（line 196）"
        )
    }

    // MARK: - case#3 守护: unowned active item → equipButton disabled（AC2）

    /// owned == false 的 active item → enable 驱动量 false（按钮 disabled —— epics.md
    /// 「未拥有不可装备」语义；与节点 9 server 校验一致方向，Story 27.1 接 EquipUseCase
    /// 后此规则不变）。
    func testUnownedActiveItemDrivesEquipButtonDisabled() {
        let unownedHat = CosmeticItem(id: "h6", name: "警官帽", category: .hat, rarity: .R, owned: false, iconEmoji: "🎩")
        let vm = MockWardrobeViewModel(inventory: [unownedHat], equipped: [:], selectedCategory: .hat)
        vm.selectItem(unownedHat.id)

        XCTAssertEqual(vm.activeItem?.id, unownedHat.id, "fixture 前提：active item 是注入的 unowned hat")
        XCTAssertEqual(
            vm.activeItem?.owned, false,
            "节点 9 enable 契约：active.owned == false → equipButton.isEnabled false（节点 9 server 校验方向一致）"
        )
    }

    // MARK: - case#4 守护: 未装备态 label 契约 = "装备"（AC3）

    /// owned 未装备 active item → `isEquipped == false` → 契约 label = "装备"
    /// （WardrobeScaffoldView.swift line 194；节点 9 Story 27.1 据此保留"装备/穿戴"语义入口）。
    func testNotEquippedActiveItemLabelContractIsEquip() {
        let ownedHat = CosmeticItem(id: "h1", name: "贝雷帽", category: .hat, rarity: .R, owned: true, iconEmoji: "🎩")
        // equipped 不含 hat → 该 item 未装备。
        let vm = MockWardrobeViewModel(inventory: [ownedHat], equipped: [:], selectedCategory: .hat)
        vm.selectItem(ownedHat.id)

        XCTAssertNotNil(vm.activeItem, "fixture 前提：active item 非空")
        XCTAssertEqual(vm.isEquipped(ownedHat), false, "fixture 前提：active item 未装备")
        XCTAssertEqual(
            equipButtonContractTitle(vm: vm), "装备",
            "节点 9 label 契约：未装备 → label \"装备\"（与 WardrobeScaffoldView.swift line 194 一致）"
        )
    }

    // MARK: - case#5 守护: 已装备态 label 契约 = "✓ 已装备 (点击卸下)" + gridCell 对勾 badge 驱动量（AC3）

    /// `equipped[category] == item.id` → `isEquipped == true` → 契约 label
    /// = "✓ 已装备 (点击卸下)"（line 194）+ gridCell 右上对勾 badge 驱动量
    /// `isEquippedNow`（WardrobeScaffoldView.swift line 344/359）为 true。
    ///
    /// epics.md 字面「status=equipped 显示独立"已装备"标签替代按钮」的等价：37.9 钦定用
    /// 按钮 label 切换 + grid 对勾**双重表达**已装备态（用户可见、语义等价，无需 epics
    /// 字面"独立标签控件"）—— 见文件头 + 24-5 story Dev Notes 偏离登记。
    func testEquippedActiveItemLabelAndGridBadgeContract() {
        let ownedHat = CosmeticItem(id: "h1", name: "贝雷帽", category: .hat, rarity: .R, owned: true, iconEmoji: "🎩")
        // equipped[.hat] == h1 → 该 item 已装备。
        let vm = MockWardrobeViewModel(inventory: [ownedHat], equipped: [.hat: "h1"], selectedCategory: .hat)
        vm.selectItem(ownedHat.id)

        XCTAssertEqual(vm.isEquipped(ownedHat), true, "fixture 前提：active item 已装备")
        XCTAssertEqual(
            equipButtonContractTitle(vm: vm), "✓ 已装备 (点击卸下)",
            "节点 9 label 契约：已装备 → label \"✓ 已装备 (点击卸下)\"（与 line 194 一致）"
        )
        // gridCell 对勾 badge 驱动量：`let isEquippedNow = state.isEquipped(item)`
        // （WardrobeScaffoldView.swift line 344）→ true 时 line 359-367 渲染右上对勾。
        XCTAssertEqual(
            vm.isEquipped(ownedHat), true,
            "节点 9 grid 对勾契约：isEquipped(item) == true → gridCell 渲染右上对勾 badge（line 344/359）"
        )
    }

    // MARK: - case#6 守护: 节点 8 onTap = 本地 placeholder toggle（非 no-op）（AC4）

    /// epics.md 字面「节点 8 onTap no-op」已被 37.9 round1 P1 fix 取代为「本地 placeholder
    /// toggle 可用」。owned active item → 调 `onEquipTap` → `equipped` **本地** toggle
    /// （本地视觉变更，**未**调 server/EquipUseCase —— server 真实写入留 Story 27.1/27.2）。
    ///
    /// 此 case 钉死「节点 8 onTap 非 no-op」契约（防按字面退回 no-op = 复犯 severity 1 lesson）。
    func testNode8OnEquipTapIsLocalPlaceholderToggleNotNoOp() {
        let ownedHat = CosmeticItem(id: "h1", name: "贝雷帽", category: .hat, rarity: .R, owned: true, iconEmoji: "🎩")
        let vm = MockWardrobeViewModel(inventory: [ownedHat], equipped: [:], selectedCategory: .hat)
        vm.selectItem(ownedHat.id)

        XCTAssertNil(vm.equipped[.hat], "fixture 前提：hat 槽位未装备")
        // 节点 8：onTap 本地 toggle（非 no-op）。
        vm.onEquipTap(item: ownedHat)
        XCTAssertEqual(
            vm.equipped[.hat], "h1",
            "节点 8 placeholder 契约：onEquipTap owned 道具 → equipped 本地 toggle 装备（非 no-op；server 写入留 27.1）"
        )
    }

    // MARK: - case#7 守护: RealWardrobeViewModel.onEquipTap placeholder 必须可用（防 severity 1 lesson 复犯）（AC4）

    /// **节点 9 前置：placeholder override 必须本地 mutate `equipped`，禁退回 disabled/no-op。**
    /// lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`
    /// （**severity 1，已复犯一次**）：Real 子类 abstract method override 占位必须本地 mutate
    /// state 让 production 可用，禁 no-op/log-only/disabled。节点 8 production 走 Real 子类，
    /// 退回 no-op = 直接违反该 lesson 预防规则。
    ///
    /// 与 `WardrobeViewScaffoldTests` case#11 等价的自包含冗余守护（关注点不同：case#11
    /// 防 37.9 回归；本 case 让「24.5 预留契约」对 Story 27.1 与 review 自解释）。
    /// 二者并存有意决策，**不**删/改 case#11。
    func testRealWardrobeViewModelOnEquipTapPlaceholderMustMutateEquipped() {
        let vm = RealWardrobeViewModel(appState: AppState())

        // (1) owned + 未装备 → 装备（equipped[.hat] = h1）。
        let ownedHat = CosmeticItem(id: "h1", name: "贝雷帽", category: .hat, rarity: .R, owned: true, iconEmoji: "🎩")
        XCTAssertNotEqual(vm.equipped[.hat], ownedHat.id, "fixture 前提：h1 未装备")
        vm.onEquipTap(item: ownedHat)
        XCTAssertEqual(
            vm.equipped[.hat], ownedHat.id,
            "节点 9 前置：Real placeholder override 必须本地 mutate（装备）—— 禁 no-op（lesson severity 1，已复犯一次）"
        )

        // (2) owned + 已装备同 id → 卸下（equipped[.hat] = nil）。
        vm.onEquipTap(item: ownedHat)
        XCTAssertNil(
            vm.equipped[.hat],
            "节点 9 前置：Real placeholder 再点同 id → 卸下（toggle 语义；非 no-op）"
        )

        // (3) unowned → equipped 不变（owned guard 保留）。
        let unownedScarf = CosmeticItem(id: "s9", name: "未拥有围巾", category: .scarf, rarity: .R, owned: false, iconEmoji: "🧣")
        let originalScarf = vm.equipped[.scarf]
        vm.onEquipTap(item: unownedScarf)
        XCTAssertEqual(
            vm.equipped[.scarf], originalScarf,
            "节点 9 前置：unowned 道具 onEquipTap → equipped 不变（owned guard 保留）"
        )
    }

    // MARK: - case#8 守护: Mock / Real 子类 onEquipTap 同语义（Preview/单测/production 观感一致）（AC4）

    /// 同 fixture 下 `MockWardrobeViewModel` 与 `RealWardrobeViewModel` 的 `onEquipTap`
    /// 对 `equipped` 产生**相同** toggle 结果（保证 Preview/单测/production 观感一致 ——
    /// 37.9 round1 P1 fix 精神延续：Real 子类占位必须与 Mock 同语义可用）。
    func testMockAndRealOnEquipTapProduceSameToggleResult() {
        let ownedHat = CosmeticItem(id: "h1", name: "贝雷帽", category: .hat, rarity: .R, owned: true, iconEmoji: "🎩")

        // Mock 路径。
        let mockVM = MockWardrobeViewModel(inventory: [ownedHat], equipped: [:], selectedCategory: .hat)
        mockVM.onEquipTap(item: ownedHat)
        let mockAfterEquip = mockVM.equipped[.hat]
        mockVM.onEquipTap(item: ownedHat)
        let mockAfterUnequip = mockVM.equipped[.hat]

        // Real 路径（同初始 equipped 空态）。
        let realVM = RealWardrobeViewModel(appState: AppState())
        realVM.equipped = [:]  // 与 Mock fixture 对齐（Real 默认 seed WardrobeScaffoldDefaults.equipped）
        realVM.onEquipTap(item: ownedHat)
        let realAfterEquip = realVM.equipped[.hat]
        realVM.onEquipTap(item: ownedHat)
        let realAfterUnequip = realVM.equipped[.hat]

        XCTAssertEqual(mockAfterEquip, "h1")
        XCTAssertEqual(realAfterEquip, "h1")
        XCTAssertEqual(
            mockAfterEquip, realAfterEquip,
            "Mock/Real onEquipTap 装备结果必须同语义（Preview/单测/production 观感一致）"
        )
        XCTAssertNil(mockAfterUnequip)
        XCTAssertNil(realAfterUnequip)
        XCTAssertEqual(
            mockAfterUnequip, realAfterUnequip,
            "Mock/Real onEquipTap 卸下结果必须同语义（37.9 round1 P1 fix 精神延续）"
        )
    }
}
