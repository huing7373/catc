// ProfileScaffoldDefaults.swift
// Story 37.11 AC3: Mock 与 Real ProfileViewModel 共享 scaffold 占位数据.
//
// 背景（Story 37.8 / 37.9 / 37.10 round 1 P2 lesson 预防性应用）：
//   抽 shared defaults 而非 hardcode 在两个 ViewModel —— 避免 Mock/Real 重复定义 mock 数据.
//
// 设计决议（与 RoomScaffoldDefaults / WardrobeScaffoldDefaults / FriendsScaffoldDefaults 同精神）：
//   - profile mock 字段值与 ui_design profile.jsx 视觉示例匹配
//     ("奶团 Lv.8 / 36 件收藏品 / 12 位好友 / 15 个成就 / 248 钻石")
//   - wechatBound 默认 false（profile.jsx 默认状态；用户点"绑定微信卡" → 切 true 后渲染"已绑定卡"分支）
//   - recentCollections 5 件（profile.jsx:140-145 钦定 5 件；混合 R/SR 稀有度）

import Foundation

/// Mock 与 Real ProfileViewModel 启动占位数据（profile state UI scaffold defaults）.
public enum ProfileScaffoldDefaults {
    /// 默认 profile（mock 用户「奶团」、Lv.8、36 收藏、12 好友、15 成就、248 钻石；
    /// 与 ui_design profile.jsx + wechat_binding.md 视觉示例一致）.
    public static let profile: ProfileSummary = ProfileSummary(
        id: "u-mock-9527",
        name: "奶团",
        title: "养猫达人",
        joinedAt: "2024.03.05",
        petName: "奶团",
        petLevel: 8,
        collectionsCount: 36,
        friendsCount: 12,
        achievementsCount: 15,
        coinsCount: 248
    )

    /// 默认微信绑定状态（mock 默认 false —— ui_design profile.jsx 默认渲染未绑定警告卡分支）.
    public static let wechatBound: Bool = false

    /// 完整 mock recentCollections（5 件，混合 R/SR 稀有度，与 profile.jsx:140-145 mock array 字段值一致）.
    public static let recentCollections: [RecentCollection] = [
        RecentCollection(id: "rc-1", name: "樱花发饰",  rarity: .SR, emoji: "🎀"),
        RecentCollection(id: "rc-2", name: "贝雷帽",    rarity: .R,  emoji: "🎩"),
        RecentCollection(id: "rc-3", name: "骑士披风",  rarity: .SR, emoji: "🧣"),
        RecentCollection(id: "rc-4", name: "水手服",    rarity: .R,  emoji: "👘"),
        RecentCollection(id: "rc-5", name: "樱花树下",  rarity: .SR, emoji: "🏞️"),
    ]
}
