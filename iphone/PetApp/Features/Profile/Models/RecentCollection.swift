// RecentCollection.swift
// Story 37.11 AC3: 最近收藏横向滑窗 cell 数据.
//
// 设计：value type + Equatable + Identifiable + Sendable.
// 字段对齐 ui_design profile.jsx:140-159 mock array `[{n:'樱花发饰',r:'SR',i:'🎀'}, ...]`.
//
// 复用 Story 37.6 落地的 `Rarity` enum（N / R / SR / SSR）—— 与 Story 37.9 落地的
// `CosmeticItem.rarity` 字段同精神；不重新定义.

import Foundation

public struct RecentCollection: Equatable, Identifiable, Sendable {
    public let id: String      // 唯一 id（mock 用 "rc-1" / "rc-2" 等；后续 epic 真接 user_cosmetic_items.id）
    public let name: String    // 道具名（如"樱花发饰"）
    public let rarity: Rarity  // 稀有度（复用 Story 37.6 落地的 Rarity enum）
    public let emoji: String   // 占位 emoji（profile.jsx 视觉走 emoji + ui_design 风格；后续 epic 接真 sprite 时改字段）

    public init(id: String, name: String, rarity: Rarity, emoji: String) {
        self.id = id
        self.name = name
        self.rarity = rarity
        self.emoji = emoji
    }
}
