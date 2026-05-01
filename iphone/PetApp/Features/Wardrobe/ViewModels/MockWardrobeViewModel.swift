// MockWardrobeViewModel.swift
// Story 37.9 AC2: WardrobeViewModel mock 子类，用于 #Preview / UITest skip-guest-login / Scaffold 单元测试.
//
// 设计：
//   - 硬编码 mock 数据（catName / inventory / equipped 全量；走 WardrobeScaffoldDefaults seed）
//   - override 1 个 abstract method（onEquipTap）改本地 equipped 映射 + 记录 invocations 数组
//   - **不**依赖 AppState（Mock 路径走纯 ViewModel-only 数据）
//
// 与 MockHomeViewModel Story 37.7 / MockRoomViewModel Story 37.8 同模式（invocations 数组而非 closure spy）.

import Foundation
import Combine
import os.log

@MainActor
public final class MockWardrobeViewModel: WardrobeViewModel {
    /// 单元测试用：记录所有方法调用
    public enum Invocation: Equatable {
        case equipTap(itemId: String)
    }

    @Published public var invocations: [Invocation] = []

    /// 默认构造 — 走 WardrobeScaffoldDefaults seed 全量字段.
    public override init() {
        super.init()
        self.catName = WardrobeScaffoldDefaults.catName
        self.inventory = WardrobeScaffoldDefaults.inventory
        self.equipped = WardrobeScaffoldDefaults.equipped
        self.selectedCategory = WardrobeScaffoldDefaults.selectedCategory
        self.selectedCosmeticId = nil
    }

    /// 测试 / Preview 灵活构造 — 可注入任意 inventory / equipped / selectedCategory.
    public init(
        catName: String = WardrobeScaffoldDefaults.catName,
        inventory: [CosmeticItem] = WardrobeScaffoldDefaults.inventory,
        equipped: [CosmeticCategory: String] = WardrobeScaffoldDefaults.equipped,
        selectedCategory: CosmeticCategory = WardrobeScaffoldDefaults.selectedCategory
    ) {
        super.init()
        self.catName = catName
        self.inventory = inventory
        self.equipped = equipped
        self.selectedCategory = selectedCategory
        self.selectedCosmeticId = nil
    }

    // MARK: - override abstract methods

    public override func onEquipTap(item: CosmeticItem) {
        os_log(.debug, "MockWardrobeViewModel.onEquipTap %{public}@", item.id)
        invocations.append(.equipTap(itemId: item.id))
        // Mock 路径：本地切换 equipped 映射（owned == false 不改；与 ui_design wardrobe.jsx:27-32 toggleEquip 等价）.
        guard item.owned else { return }
        if equipped[item.category] == item.id {
            equipped[item.category] = nil  // 卸下
        } else {
            equipped[item.category] = item.id  // 装备
        }
    }
}
