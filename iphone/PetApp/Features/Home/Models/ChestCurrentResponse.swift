// ChestCurrentResponse.swift
// Story 21.2 AC1: GET /api/v1/chest/current 响应 wire DTO；V1 §7.1 钦定 5 字段.
//
// 与 ChestDTO（HomeResponse.swift §6.7）字段名 / 类型 100% 对齐（V1 §7.1 行 1208 钦定跨接口字段对齐）.
// 不复用 ChestDTO 的原因：HomeResponse 内 ChestDTO 是嵌套字段；本 endpoint 直接顶层返回 5 字段；
// V1 §13 全局信封封装层在 APIClient 已处理；本 DTO 只描述 `data` 段 5 字段.
//
// 设计决策（story AC1 关键决策 1）：独立 `ChestCurrentResponse` 而非复用 `ChestDTO`.
//   - V1 §7.1 是顶层接口，与 §5.1 GET /home 内嵌的 chest 字段在序列化路径上独立；
//   - 未来若 V1 §7.1 加字段（如 server 端时钟戳）独立 DTO 不会污染 HomeResponse；
//   - 与 `PetStateSyncResponse` / `StepsSyncResponse` 等独立响应 DTO 同模式.
//
// 设计决策（story AC1 关键决策 2）：字段 `Int` 而非 `UInt`.
//   - V1 §7.1 关键约束行 912 钦定 "client 解析层**应**按 `Int` 处理（不是 `UInt` —— Swift 端 `UInt`
//     在解析时若收到负数会 crash）"；server 保证 `remainingSeconds ≥ 0` 但 client 防御性按 Int.

import Foundation

public struct ChestCurrentResponse: Decodable, Equatable, Sendable {
    public let id: String                 // BIGINT 字符串化（V1 §2.5）
    public let status: Int                // 1 = counting, 2 = unlockable（V1 §7.1）
    public let unlockAt: Date             // ISO 8601 RFC3339（APIClient JSONDecoder 已配 .iso8601）
    public let openCostSteps: Int         // 节点 7 阶段固定 1000
    public let remainingSeconds: Int      // server 端 max(0, ceil((unlock_at - now) / 1s)); 总是 ≥ 0

    public init(id: String, status: Int, unlockAt: Date, openCostSteps: Int, remainingSeconds: Int) {
        self.id = id
        self.status = status
        self.unlockAt = unlockAt
        self.openCostSteps = openCostSteps
        self.remainingSeconds = remainingSeconds
    }
}
