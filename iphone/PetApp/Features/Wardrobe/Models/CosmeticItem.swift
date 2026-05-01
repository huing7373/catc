// CosmeticItem.swift
// Story 37.9 AC3: WardrobeScaffoldView 道具数据模型.
//
// 设计：value type + Equatable + Sendable + Identifiable，纯展示数据；mock 值在 WardrobeScaffoldDefaults.
// Story 24.1 接 LoadInventoryUseCase 后由 RealWardrobeViewModel 内 mapping 写入（HomeEquip → CosmeticItem
// 或 server CosmeticInventoryDTO → CosmeticItem，由 Story 24.1 决定）.
//
// 字段名对齐 ui_design wardrobe.jsx items array shape（id / name / rarity / owned），
// 加 `category` 让按分类过滤时不依赖 dictionary key（顶层数组更易 Story 24.1 mapping）+ `iconEmoji` 给 grid cell 渲染.

import Foundation

public struct CosmeticItem: Equatable, Identifiable, Sendable {
    public let id: String              // cosmeticId / itemId（Story 24.1 后对齐 server cosmeticItemId）
    public let name: String            // 道具名（如"贝雷帽"）
    public let category: CosmeticCategory   // 分类（hat / bow / scarf / outfit / bg）
    public let rarity: Rarity          // Story 37.6 落地的 Rarity enum (N / R / SR / SSR)
    public let owned: Bool             // 是否已拥有（false → grid 半透明 + 🔒 锁标）
    public let iconEmoji: String       // grid cell 占位图标（Story 30.x 接真实 sprite 时升级；mock "🎩" / "🎀" 等）

    public init(
        id: String,
        name: String,
        category: CosmeticCategory,
        rarity: Rarity,
        owned: Bool,
        iconEmoji: String
    ) {
        self.id = id
        self.name = name
        self.category = category
        self.rarity = rarity
        self.owned = owned
        self.iconEmoji = iconEmoji
    }
}
