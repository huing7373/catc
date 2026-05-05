// StepsSyncResponse.swift
// Story 8.5 AC1: V1 §6.1 POST /api/v1/steps/sync 响应 wire DTO.
//
// 契约源（V1 §6.1 已冻结）:
// - 外层走 APIClient 既有 envelope 解包（`code/message/data/requestId`，已在 Story 2.4 / 5.5 落地）
// - data 字段嵌套：data.acceptedDeltaSteps + data.stepAccount.{totalSteps, availableSteps, consumedSteps}
//   （注意：与 §6.2 `GET /steps/account` 直接平铺三档值不同；§6.1 是"动作型"接口，附带本次入账后的最新账户态）
//
// 关键 schema 选择（与 Story 5.5 HomeResponse / StepAccountDTO 同模式）:
// - Decodable（不需要 Encodable；client 只解析）+ Equatable + Sendable
// - 嵌套 `StepAccountInSyncResponse` 结构体单独命名（不复用 `HomeResponse.StepAccountDTO`，
//   避免改 5.5 落地的 HomeData 解析路径——StepAccountDTO 是 GET /home 解析专用；
//   本 story 是 POST /steps/sync 专用）

import Foundation

public struct StepsSyncResponse: Decodable, Sendable, Equatable {
    public let acceptedDeltaSteps: Int
    public let stepAccount: StepAccountInSyncResponse

    public init(
        acceptedDeltaSteps: Int,
        stepAccount: StepAccountInSyncResponse
    ) {
        self.acceptedDeltaSteps = acceptedDeltaSteps
        self.stepAccount = stepAccount
    }
}

public struct StepAccountInSyncResponse: Decodable, Sendable, Equatable {
    public let totalSteps: Int
    public let availableSteps: Int
    public let consumedSteps: Int

    public init(totalSteps: Int, availableSteps: Int, consumedSteps: Int) {
        self.totalSteps = totalSteps
        self.availableSteps = availableSteps
        self.consumedSteps = consumedSteps
    }
}
