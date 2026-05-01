// ProfileSummary.swift
// Story 37.11 AC3: ProfileScaffoldView 顶部头图 + 统计卡 + Modal 风险列表共享数据模型.
//
// 设计：value type + Equatable + Sendable，纯展示数据（统计字段为 raw int / String，由 view 层格式化显示）.
// 后续 epic 接 server `/profile/me` + `/collections/count` + `/achievements/count` + `/friends/count` 后由
//   RealProfileViewModel 内 mapping 写入（API DTO → ProfileSummary，多 publisher 合并）.
//
// 字段名对齐 ui_design profile.jsx 内 ProfileScreen({ user }) shape（user.name / user.id / user.title / user.joinedAt）+
//   wechat_binding.md §"数据风险清单" 4 项（小猫 Lv.X / N 件收藏品 / Y 个成就徽章 / Z 位好友关系）.

import Foundation

public struct ProfileSummary: Equatable, Sendable {
    /// 用户 id（profile.jsx:45 `ID: {user.id}`）.
    public let id: String
    /// 用户昵称（profile.jsx:43）.
    public let name: String
    /// 用户称号（profile.jsx:45 `· {user.title}`；如"养猫达人"）.
    public let title: String
    /// "加入于 {joinedAt}"小药丸（profile.jsx:52；如"2024.03.05"）.
    public let joinedAt: String
    /// 小猫名（Modal 风险列表 "小猫 Lv.X · {petName}"；wechat_binding.md:74）.
    public let petName: String
    /// 小猫等级（统计卡 "小猫等级 Lv.X" + Modal 风险列表 "Lv.X"；profile.jsx:69 + wechat_binding.md:74）.
    public let petLevel: Int
    /// 收藏品数量（统计卡 "收藏品 N" + Modal "{N} 件收藏品"；profile.jsx:65 + wechat_binding.md:75）.
    public let collectionsCount: Int
    /// 好友数量（统计卡 "好友 N" + Modal "{N} 位好友关系"；profile.jsx:67 + wechat_binding.md:77）.
    public let friendsCount: Int
    /// 成就数量（统计卡 "成就 N" + Modal "{N} 个成就徽章"；profile.jsx:71 + wechat_binding.md:76）.
    public let achievementsCount: Int
    /// 钻石货币数量（Modal "价值 {N} 钻石"；wechat_binding.md:75）.
    public let coinsCount: Int

    public init(
        id: String,
        name: String,
        title: String,
        joinedAt: String,
        petName: String,
        petLevel: Int,
        collectionsCount: Int,
        friendsCount: Int,
        achievementsCount: Int,
        coinsCount: Int
    ) {
        self.id = id
        self.name = name
        self.title = title
        self.joinedAt = joinedAt
        self.petName = petName
        self.petLevel = petLevel
        self.collectionsCount = collectionsCount
        self.friendsCount = friendsCount
        self.achievementsCount = achievementsCount
        self.coinsCount = coinsCount
    }
}
