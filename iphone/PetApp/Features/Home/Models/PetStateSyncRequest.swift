// PetStateSyncRequest.swift
// Story 15.4 AC1: V1 §5.2 wire DTO 入参（POST /api/v1/pets/current/state-sync 请求体）.
//
// schema（V1 §5.2 line 490-540）：
//   - 单字段 `state: int` ∈ {1, 2, 3} —— 与 MotionState.wireValue 对齐
//     (.rest=1 / .walk=2 / .run=3；详见 MotionState.swift "wireValue" extension)
//   - **不**带 `petId`（V1 §5.2 line 605：server 自查默认 pet）
//   - **不**带 `idempotencyKey` header（V1 §5.2 line 500：state-sync 不消耗资产）
//
// 与 sibling StepsSyncRequest 同模式 —— pure value type / Encodable / Sendable / Equatable.
// Equatable 让测试可直接 XCTAssertEqual(repo.invocations.first, expected) 不必逐字段断言.

import Foundation

public struct PetStateSyncRequest: Encodable, Sendable, Equatable {
    public let state: Int

    public init(state: Int) {
        self.state = state
    }
}
