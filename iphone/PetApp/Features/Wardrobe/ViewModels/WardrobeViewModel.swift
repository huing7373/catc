// WardrobeViewModel.swift
// Story 37.9 AC1: WardrobeScaffoldView 基类 ViewModel（class 层次 + 5 字段 + 1 abstract method + 2 concrete method）.
//
// 设计：与 HomeViewModel / RoomViewModel 同精神（class 而非 final + abstract method 用 fatalError 强制 override）.
// 字段范围：5 字段（catName / inventory / equipped / selectedCategory / selectedCosmeticId）.
// 节点 8/9 后 Story 24.1 RealWardrobeViewModel 子类扩 LoadInventoryUseCase / EquipUseCase 接 sink（不在本 story 范围）.
//
// import 备注（继承 Story 2.2 lesson 2026-04-25-swift-explicit-import-combine.md）：
// `ObservableObject` / `@Published` 来自 Combine，不能依赖 SwiftUI transitive import.

import Foundation
import Combine

@MainActor
public class WardrobeViewModel: ObservableObject {
    /// 顶部 Card 标题用的猫名（mock "小花"；RealWardrobeViewModel 从 appState.$currentPet sink 派生）.
    /// **关键约束**：catName 在 Wardrobe 域是合法派生源 = 本地用户**自己的**猫的名字（Wardrobe 是"看自己衣柜"
    /// 单一视角；与 Story 37.8 RoomViewModel.hostCatName **不可** 派生自 currentPet 的 lesson 不冲突 ——
    /// Wardrobe 域的"猫名"语义就是 appState.currentPet.name 钦定）.
    @Published public var catName: String = ""

    /// 当前用户的 inventory（mock 多分类共 18 件；RealWardrobeViewModel 从 appState.$currentInventory sink 派生）.
    /// 类型 [CosmeticItem]（本 story 新建 value type；不复用 HomeEquip —— HomeEquip 只描述"已装备"非"全 inventory"）.
    @Published public var inventory: [CosmeticItem] = []

    /// 已装备道具映射（key=分类 / value=cosmeticId）；mock 默认 hat=h3 / bow=b1 / scarf=s2.
    /// MockWardrobeViewModel.onEquipTap 改本字段；RealWardrobeViewModel.onEquipTap 也改本字段（占位）—— round 1 P1 fix（codex review）让 Real 路径
    /// 不再 no-op；Story 27.1 改调 EquipUseCase 写 appState.currentEquips → 通过 sink 派生 equipped（不再本地直接写）.
    @Published public var equipped: [CosmeticCategory: String] = [:]

    /// 当前选中分类 Tab（默认 .hat；用户点 Tab 切换）.
    /// 这是 view-specific transient state（按 ADR-0010 §3.2 表格"表单输入 / 当前选中" → ViewModel @Published 或 SwiftUI @State；
    /// 本 story 选 ViewModel @Published 让 sink 派生路径统一 + Story 24.1 落地 Real 时 selectedCategory 仍归 ViewModel transient 不进 AppState）.
    @Published public var selectedCategory: CosmeticCategory = .hat

    /// 当前选中道具 cosmeticId（用户点 grid cell 切换；nil = 当前未选中任何道具，预览区走 fallback "未选择"）.
    /// 与 selectedCategory 同精神：view-specific transient @Published（ADR-0010 §3.2）.
    @Published public var selectedCosmeticId: String?

    /// Story 24.3: 当前选中的品质筛选（nil = "全部"，不按品质过滤）.
    /// view-specific transient @Published（ADR-0010 §3.2「current Tab/sheet/loading/筛选选中 →
    /// ViewModel transient，不进 AppState」）—— 与 selectedCategory / selectedCosmeticId 同归属.
    /// 纯客户端：filteredCategoryItems 据此对 currentCategoryItems 再叠加 rarity 过滤，不发任何 API.
    @Published public var selectedRarity: Rarity? = nil

    public init() {}

    // MARK: - abstract method（基类 fatalError 占位，子类必 override）

    /// 装备/卸下按钮回调（预览区"装备"按钮调；判断已装备时切换为"卸下"语义）.
    /// MockWardrobeViewModel: 改本地 equipped 映射 + 记录 invocation + print log.
    /// RealWardrobeViewModel（本 story 占位）: 改本地 equipped 映射（与 Mock 同语义）+ log；
    ///   round 1 P1 fix 后行为：Real path 不再 no-op，让 production 装备按钮立即视觉反馈.
    /// RealWardrobeViewModel（Story 27.1+）: 改为调 EquipUseCase / UnequipUseCase + appState.updateCurrentEquips,
    ///   通过 sink 从 appState.currentEquips 派生 equipped（不再本地直接写）.
    public func onEquipTap(item: CosmeticItem) {
        fatalError("WardrobeViewModel.onEquipTap must be overridden by subclass")
    }

    // MARK: - concrete view-action method（基类直接实装，子类不 override）

    /// 切换分类 Tab（用户点 5 个 Tab 之一调）.
    /// **不是** abstract —— 切换分类是纯 view-state 行为，没有"Mock vs Real"分化需求；放基类避免子类重复实装.
    /// 副作用：清空 selectedCosmeticId（切换分类后老选中 id 不在新分类 inventory 内，让预览区 fallback 到 "未选择"）.
    public func selectCategory(_ category: CosmeticCategory) {
        self.selectedCategory = category
        self.selectedCosmeticId = nil
        // Story 24.3: 切分类回「全部品质」（用户心智：换一个分类重新看；
        // **不**改既有「清空 selectedCosmeticId」副作用，仅追加本行）.
        self.selectedRarity = nil
    }

    /// 切换选中道具（用户点 grid cell 调）.
    /// **不是** abstract —— 选中是纯 view-state 行为（与 selectCategory 同精神）.
    public func selectItem(_ cosmeticId: String) {
        self.selectedCosmeticId = cosmeticId
    }

    /// Story 24.3: 切换品质筛选（用户点品质 chip 调；nil = "全部"）.
    /// **不是** abstract —— 纯 view-state 行为，无 Mock/Real 分化需求（与 selectCategory / selectItem 同精神，
    /// 放基类避免子类重复实装 → Mock/Real 零 edit 自动继承）.
    /// 副作用：清空 selectedCosmeticId（与 selectCategory 同精神 —— 过滤变化后老选中 id 可能不在
    /// filteredCategoryItems 内，让预览区 activeItem fallback，防止选中态指向已被过滤掉的 item）.
    public func selectRarity(_ rarity: Rarity?) {
        self.selectedRarity = rarity
        self.selectedCosmeticId = nil
    }

    // MARK: - derived helper（view 层方便用，子类不 override）

    /// 当前选中分类的 inventory（按 selectedCategory 过滤；分类 Tab badge / isEquipped 等既有派生数据源）.
    /// **保持原样不改**（37.9/24.1 既有契约依赖；Story 24.3 在其上叠加 filteredCategoryItems，不替换）.
    public var currentCategoryItems: [CosmeticItem] {
        inventory.filter { $0.category == selectedCategory }
    }

    /// Story 24.3: 在 currentCategoryItems 之上再叠加 rarity 过滤的 grid 渲染数据源（纯客户端，零 API）.
    /// selectedRarity == nil ⇒ 原样返回 currentCategoryItems（"全部"）；
    /// selectedRarity == r ⇒ currentCategoryItems.filter { $0.rarity == r }.
    /// 是**新增叠加层**不替换 currentCategoryItems —— 仅 grid 的 ForEach 数据源 + activeItem 底层源
    /// 改用本属性；分类 Tab badge count 仍用原 currentCategoryItems 语义不受品质筛选影响.
    public var filteredCategoryItems: [CosmeticItem] {
        guard let rarity = selectedRarity else { return currentCategoryItems }
        return currentCategoryItems.filter { $0.rarity == rarity }
    }

    /// 当前选中的 active item（selectedCosmeticId → CosmeticItem 查找；nil 时 fallback 到当前分类已装备 item 或第一个 item）.
    /// 与 ui_design wardrobe.jsx:25 `activeItem` 派生逻辑等价：selected || items.find(i => i.equip === cat) || items[0]
    /// Story 24.3: 底层 items 源由 currentCategoryItems 换为 filteredCategoryItems —— 让预览区与可见 grid
    /// 一致（选中只可能落在可见项；fallback equipped/first 也在过滤后集合内，预览区不显示被过滤掉的道具）.
    /// fallback 优先级 selected→equipped→first 逻辑不动，仅换 `let items =` 源那一行.
    public var activeItem: CosmeticItem? {
        let items = filteredCategoryItems
        if let selectedId = selectedCosmeticId,
           let selected = items.first(where: { $0.id == selectedId }) {
            return selected
        }
        if let equippedId = equipped[selectedCategory],
           let equippedItem = items.first(where: { $0.id == equippedId }) {
            return equippedItem
        }
        return items.first
    }

    /// 判断 item 是否已装备（grid cell 右上对勾 + 预览区按钮文案"装备 / ✓ 已装备(点击卸下)" 用）.
    public func isEquipped(_ item: CosmeticItem) -> Bool {
        equipped[item.category] == item.id
    }
}
