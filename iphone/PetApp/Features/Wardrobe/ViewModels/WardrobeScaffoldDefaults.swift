// WardrobeScaffoldDefaults.swift
// Story 37.9 AC3: Mock 与 Real WardrobeViewModel 共享 scaffold 占位数据.
//
// 背景（Story 37.8 round 1 P2 lesson 预防性应用）：
//   原始 RealRoomViewModel.init() 仅 set roomCodeForCopy / hostCatName 占位，**不** seed members / userIsHost,
//   导致 in-room state Real path 渲染近乎空房间. 本 story 直接采用 lesson 钦定 option A：抽 shared defaults 而非 hardcode.
//
// 设计决议（与 RoomScaffoldDefaults 同精神）：
//   抽 shared defaults 而非 hardcode 在两个 ViewModel —— 避免 Mock/Real 重复定义 mock 数据，
//   未来 Story 24.1 接 LoadInventoryUseCase 时只需在 RealWardrobeViewModel sink 内覆盖即可，**不**动 Mock.

import Foundation

/// Mock 与 Real WardrobeViewModel 启动占位数据（wardrobe state UI scaffold defaults）.
///
/// **使用规则**（务必读）：
/// - Mock：直接用 WardrobeScaffoldDefaults 字段初始化 5 个 @Published（参见 MockWardrobeViewModel.init()）.
/// - Real：init() / init(appState:) 都用 WardrobeScaffoldDefaults seed 起手；sink 路径
///   （subscribeCatName / subscribeInventory）作为 override —— currentPet 来 → 派生 catName；
///   currentInventory 来 → 派生 inventory（Story 24.1 落地后写真）；都 fallback 到 WardrobeScaffoldDefaults 占位.
/// - Story 24.1 / 24.2 后：RealWardrobeViewModel 接 LoadInventoryUseCase → 写入时覆盖 inventory；
///   覆盖前仍用 WardrobeScaffoldDefaults 不让 WardrobeScaffoldView 渲染空衣柜.
public enum WardrobeScaffoldDefaults {
    /// 顶部 Card 标题用的猫名占位（mock "小花"；RealWardrobeViewModel sink 派生覆盖）.
    public static let catName: String = "小花"

    /// 默认选中分类（mock .hat —— ui_design wardrobe.jsx:4 useState('hat') 钦定）.
    public static let selectedCategory: CosmeticCategory = .hat

    /// 默认已装备映射（mock hat=h3 皇冠 / bow=b1 粉色蝴蝶结 / scarf=s2 骑士披风；与 ui_design wardrobe.jsx items 内 equip 字段对齐）.
    public static let equipped: [CosmeticCategory: String] = [
        .hat: "h3",
        .bow: "b1",
        .scarf: "s2",
    ]

    /// 完整 mock inventory（5 分类共 18 件；与 ui_design wardrobe.jsx:16-22 items 字段值 1:1 对齐）.
    public static let inventory: [CosmeticItem] = [
        // hat（6 件）
        CosmeticItem(id: "h1", name: "贝雷帽", category: .hat, rarity: .R, owned: true, iconEmoji: "🎩"),
        CosmeticItem(id: "h2", name: "草帽", category: .hat, rarity: .N, owned: true, iconEmoji: "🎩"),
        CosmeticItem(id: "h3", name: "皇冠", category: .hat, rarity: .SR, owned: true, iconEmoji: "🎩"),
        CosmeticItem(id: "h4", name: "魔法帽", category: .hat, rarity: .SSR, owned: false, iconEmoji: "🎩"),
        CosmeticItem(id: "h5", name: "蝴蝶帽", category: .hat, rarity: .R, owned: true, iconEmoji: "🎩"),
        CosmeticItem(id: "h6", name: "警官帽", category: .hat, rarity: .R, owned: false, iconEmoji: "🎩"),
        // bow（4 件）
        CosmeticItem(id: "b1", name: "粉色蝴蝶结", category: .bow, rarity: .N, owned: true, iconEmoji: "🎀"),
        CosmeticItem(id: "b2", name: "星星发夹", category: .bow, rarity: .R, owned: true, iconEmoji: "🎀"),
        CosmeticItem(id: "b3", name: "樱花发饰", category: .bow, rarity: .SR, owned: true, iconEmoji: "🎀"),
        CosmeticItem(id: "b4", name: "彩虹丝带", category: .bow, rarity: .SSR, owned: false, iconEmoji: "🎀"),
        // scarf（3 件）
        CosmeticItem(id: "s1", name: "毛线围巾", category: .scarf, rarity: .N, owned: true, iconEmoji: "🧣"),
        CosmeticItem(id: "s2", name: "骑士披风", category: .scarf, rarity: .SR, owned: true, iconEmoji: "🧣"),
        CosmeticItem(id: "s3", name: "太空斗篷", category: .scarf, rarity: .SSR, owned: false, iconEmoji: "🧣"),
        // outfit（2 件）
        CosmeticItem(id: "o1", name: "水手服", category: .outfit, rarity: .R, owned: true, iconEmoji: "👘"),
        CosmeticItem(id: "o2", name: "和服", category: .outfit, rarity: .SR, owned: false, iconEmoji: "👘"),
        // bg（3 件）
        CosmeticItem(id: "g1", name: "粉色房间", category: .bg, rarity: .N, owned: true, iconEmoji: "🏞️"),
        CosmeticItem(id: "g2", name: "樱花树下", category: .bg, rarity: .SR, owned: true, iconEmoji: "🏞️"),
        CosmeticItem(id: "g3", name: "星空", category: .bg, rarity: .SSR, owned: false, iconEmoji: "🏞️"),
    ]
}
