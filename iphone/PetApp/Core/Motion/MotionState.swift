// MotionState.swift
// Story 8.3 AC1: 业务三态运动状态枚举（System Adapter 层 → Domain 层之间的统一类型基础）.
//
// 设计基线（详见 story 8-3-运动状态机映射.md AC1 段）:
// - 三态闭集：.rest / .walk / .run（与 docs §10.2 钦定的"rest / walk / run"映射目标对齐）
// - 不含 .unknown / .cycling / .automotive 等 system framework 概念——这些由 MotionStateMapper.map(_:)
//   按规则归并到 .rest（详见 MotionStateMapper.swift）；ViewModel / UI 层只感知三态闭集
// - String rawValue 直接用作 V1 §6.1 /steps/sync 的 motionState 字段值（小写英文，与契约对齐）
//   - "rest" / "walk" / "run"（不大写、不带前缀，避免 8.5 拼请求体时再做 lowercased() 转换）
// - Codable + Equatable + Sendable + CaseIterable（CaseIterable 给 8.4 PetSpriteView 测试遍历状态用）
// - 不引 import CoreMotion（pure 业务类型；与协议层 MotionProvider.swift 区分——后者必 import CM）

import Foundation

/// 业务三态运动状态枚举.
/// - rest: 静止 / 坐下 / 系统未识别活动 / cycling / automotive 兜底（详见 MotionStateMapper.map(_:)）
/// - walk: 行走（含慢走）
/// - run: 跑步（含快跑）
public enum MotionState: String, Codable, Equatable, Sendable, CaseIterable {
    case rest = "rest"
    case walk = "walk"
    case run = "run"
}

// MARK: - Story 8.5 AC4.1: V1 §6.1 wire 类型派生

public extension MotionState {
    /// V1 §6.1 motionState 字段 wire value（int 1/2/3）.
    /// - 1 = stationary_or_unknown（与 .rest 对应）
    /// - 2 = walking（与 .walk 对应）
    /// - 3 = running（与 .run 对应）
    /// - **必须**与 server INT8 motion_state 字段值对齐（V1 §6.1.3 + 数据库设计 §5.5）.
    /// - 命名 `wireValue` 与 `rawValue`（"rest"/"walk"/"run" String）区分语义.
    var wireValue: Int {
        switch self {
        case .rest: return 1
        case .walk: return 2
        case .run:  return 3
        }
    }
}
