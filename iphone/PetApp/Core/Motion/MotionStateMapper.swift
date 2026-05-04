// MotionStateMapper.swift
// Story 8.3 AC2: CoreMotion CMMotionActivity → 业务三态 MotionState 的映射规则集中点.
//
// 设计基线（详见 story 8-3-运动状态机映射.md AC2 段）:
// - pure function（enum + static func；namespace 用法），无状态外部依赖
// - 优先级规则：running > walking > stationary > 其他
//   - epics.md AC 行 1514 钦定"多个 flag 同时 true（如 walking + stationary）→ 优先级 run > walk > rest"
//   - cycling / automotive / unknown / 其他全部归 .rest（docs §10.2 "坐下按静止处理" 同精神扩展）
// - confidence 策略：**所有 confidence 等级（含 .low）都按 activity type 映射**，不再做 confidence 防抖.
//   - 历史背景：epics.md AC 行 1515 原文钦定"confidence < .low → 保持上一次状态（防抖）"；
//     但 CMMotionActivityConfidence 公开 enum 当前只有 .low / .medium / .high 三档（无"<.low"），
//     初版（Story 8.3 r0）把 `.low` 视作"低置信度"防抖入口 → fix-review r1 确认这是 over-spec.
//   - fix-review r1（codex P2）反查得：低置信度 stationary 后 mapper 返 previous 会导致下游
//     HomeViewModel (8.4) 与 /steps/sync (8.5) 卡在 stale .walk / .run；用户停下却看不到 .rest.
//   - 修复决策：移除 confidence==.low debounce 分支；`.low` confidence 也按 type 映射.
//     上游订阅方（8.4）若需要 hysteresis / 防闪烁，由 ViewModel 层自己做（不在 pure mapper 里）.
// - previous 参数依然保留接受签名兼容（API stability + 让未来 caller 可注入历史态做高层防抖判断），
//   但**实装内不再消费** —— 标记 `@_unused`-style 文档（不引入编译器属性，仅文档）.

import Foundation
import CoreMotion  // 输入是 CMMotionActivity；本文件唯一引 CoreMotion 的 pure 业务文件

/// CoreMotion CMMotionActivity → 业务三态 MotionState 的纯函数映射器.
/// pure function（enum + static func；namespace 用法）；无状态、无副作用、无外部依赖；同输入同输出.
///
/// 规则（按优先级从高到低）:
///   1. running == true → .run
///   2. walking == true → .walk
///   3. stationary == true → .rest
///   4. cycling / automotive / unknown / 其他（含全 false 兜底）→ .rest
///
/// confidence 等级（.low / .medium / .high）**不参与裁决** — fix-review r1 移除 `.low` debounce
/// 分支，避免下游（8.4 / 8.5）在低置信度 stationary 时卡在 stale walk/run 状态.
///
/// 优先级与 epics.md §Story 8.3 AC 行 1514 钦定"多个 flag 同时为 true → run > walk > rest"严格对齐.
/// docs/宠物互动App_iOS客户端工程结构与模块职责设计.md §10.2 钦定的 "stationary → rest" /
/// "walking → walk" / "running → run" 是规则 1-3 的来源；规则 4 是 "坐下按静止处理"语义扩展
///（cycling / automotive 等也按"非 walk/run 即 rest"归并）.
public enum MotionStateMapper {
    /// 把 CMMotionActivity 映射到业务三态 MotionState.
    /// - Parameters:
    ///   - activity: 系统识别到的 motion activity 实例（含 stationary/walking/running/cycling/automotive/unknown
    ///               多个 Bool flag + confidence + startDate）.
    ///   - previous: caller 持有的上一次映射结果（可选）；**当前实装不消费此参数**（保留为 API
    ///                stability — 未来如需 ViewModel 层 hysteresis 复用 mapper 签名时可启用）.
    ///                所有 confidence 等级都按 flag type 映射，不做 mapper 层防抖.
    /// - Returns: 业务三态 MotionState.
    public static func map(_ activity: CMMotionActivity, previous: MotionState? = nil) -> MotionState {
        // 历史 r0 在此 if confidence == .low { return previous ?? .rest } —— fix-review r1 移除.
        // previous 参数当前未消费；保留签名是为了 API stability + 给上层将来做 hysteresis 留接口.
        _ = previous

        // 规则 1-3：running > walking > stationary 优先级（多个 flag 同时 true 时按此顺序裁决）.
        if activity.running {
            return .run
        }
        if activity.walking {
            return .walk
        }
        if activity.stationary {
            return .rest
        }

        // 规则 4：cycling / automotive / unknown / 全 false 兜底 → .rest.
        // docs §10.2 "坐下按静止处理"同精神扩展：业务三态闭集，非 walk/run 即 rest.
        return .rest
    }
}
