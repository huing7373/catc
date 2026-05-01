// PetStats.swift
// Story 37.7 AC4: HomeView CatStage 三状态条数据模型（饱食/心情/活力）.
//
// 设计：value type + Equatable + Sendable，纯展示数据；mock 值在 .mockHappy / .mockTired / .mockEmpty.
// 节点 8 / 14.x 后接真实状态时再扩展（如 streak / lastUpdated 等字段）；本 story 范围内仅展示 value.

import Foundation

public struct PetStats: Equatable, Sendable {
    public let hunger: Int      // 饱食 0-100
    public let mood: Int        // 心情 0-100
    public let energy: Int      // 活力 0-100

    public init(hunger: Int, mood: Int, energy: Int) {
        self.hunger = max(0, min(100, hunger))
        self.mood = max(0, min(100, mood))
        self.energy = max(0, min(100, energy))
    }
}

extension PetStats {
    /// mock：开心状态（home.jsx 默认 mock：饱食 72 / 心情 88 / 活力 65）
    public static let mockHappy = PetStats(hunger: 72, mood: 88, energy: 65)
    /// mock：低值状态（用于 edge case 测试 stats.hunger = 0）
    public static let mockEmpty = PetStats(hunger: 0, mood: 0, energy: 0)
    /// 默认 zero（基类 @Published var stats: PetStats = .zero 用）
    public static let zero = PetStats(hunger: 0, mood: 0, energy: 0)
}
