// StepsSyncRequest.swift
// Story 8.5 AC1: V1 §6.1 POST /api/v1/steps/sync 请求 wire DTO.
//
// 契约源（V1 §6.1 已冻结，**禁止**改任一字段）:
// - syncDate: string YYYY-MM-DD（严格 10 字符；client 按本机时区算"今天"；server [today-2d, today+2d] 容忍）
// - clientTotalSteps: int ≥ 0（HealthKit 当日累计；不是增量）
// - motionState: int ∈ {1,2,3}（1=stationary_or_unknown / 2=walking / 3=running；与 V1 §6.1.3 枚举一致）
// - clientTimestamp: int64 ms > 0（仅审计；不参与 server 差值计算）
//
// 字段类型选择:
// - syncDate: String（Codable encode 直接 JSON 字符串；不用 Date 避免跨时区 ISO 化失误）
// - clientTotalSteps: Int（HealthKit 当日步数 64-bit 安全；服务端 INT32 自然范围内）
// - motionState: Int（与 server INT8 字段对齐；MotionState String enum 通过 wireValue extension 桥接）
// - clientTimestamp: Int64（毫秒；与 V1 §6.1.4 一致；服务端 BIGINT UNSIGNED）

import Foundation

public struct StepsSyncRequest: Encodable, Sendable, Equatable {
    public let syncDate: String
    public let clientTotalSteps: Int
    public let motionState: Int
    public let clientTimestamp: Int64

    public init(
        syncDate: String,
        clientTotalSteps: Int,
        motionState: Int,
        clientTimestamp: Int64
    ) {
        self.syncDate = syncDate
        self.clientTotalSteps = clientTotalSteps
        self.motionState = motionState
        self.clientTimestamp = clientTimestamp
    }
}
