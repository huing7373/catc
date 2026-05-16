// RewardRarityTagMapper.swift
// Story 21.4 AC1: RewardRarity (1..4) → Rarity (N/R/SR/SSR) 映射纯函数 helper.
//
// 抽出来的理由（与 HomeRoomDispatcher / HomePetNameResolver / JoinRoomInputNormalizer 同精神）:
//   - ADR-0002 §3.1 禁用 ViewInspector / SnapshotTesting → 视觉派生规则必须抽成纯函数让 XCTest 直接覆盖
//   - 让 RewardPopupView body 直接用 Rarity primitive (不在 view 内 switch case)，保持 view body 干净
//   - 让单测断言 helper(input) == output 即等价于断言 view 渲染了对应徽章
//
// 命名（_Mapper 后缀模式）: 与 MotionStateMapper 同精神; 与 HomeRoomDispatcher 区分（后者是 dispatch / 决策语义）.

import Foundation

public enum RewardRarityTagMapper {
    /// 映射 RewardRarity → RarityTag 用的 Rarity enum.
    ///
    /// 映射表（V1 §6.9 + 数据库设计 §6.9 + RarityTag 配色）:
    ///   - .common    (1) → .N    灰色  #b0b0b0
    ///   - .rare      (2) → .R    蓝色  #7db3e8
    ///   - .epic      (3) → .SR   紫色  #c58ae8
    ///   - .legendary (4) → .SSR  金红渐变 #ffd166 → #ef476f
    ///
    /// 不做兜底 default case：RewardRarity enum 限定 1..4 四档（switch exhaustive），添加新档时编译期强制更新此 mapper.
    public static func map(_ rarity: RewardRarity) -> Rarity {
        switch rarity {
        case .common:    return .N
        case .rare:      return .R
        case .epic:      return .SR
        case .legendary: return .SSR
        }
    }
}
