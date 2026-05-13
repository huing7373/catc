// PetStateSyncResponse.swift
// Story 15.4 AC1: V1 §5.2 wire DTO 响应（POST /api/v1/pets/current/state-sync ack）.
//
// schema（V1 §5.2 line 580-620）：
//   - 单字段 `state: int`（server-acknowledged echo 信号；与请求体 state 一般相等）
//
// **HTTP `data.state` ack 信号严禁作权威信号源**（V1 §5.2 line 610-613 +
// lesson `2026-05-12-state-sync-err-binary-and-placeholder-whitelist-self-http-ack-14-1-r6.md`
// 钦定）：本字段仅作两用 ——
//   (a) state-sync 调用成功的标志（HTTP 200 + decode 成功即视为 ack 到位）；
//   (b) self-broadcast 兜底信号之一（与 14.4 server fan-out 配合，用于未来诊断 / log）.
// **不**进入 HomeViewModel / RoomViewModel 的 mutate 路径；**不**驱动 UI；
// 本 story SyncPetStateUseCase.execute 仅以 `.success(echoedState:)` 透传给 caller 用于测试断言.

import Foundation

public struct PetStateSyncResponse: Decodable, Sendable, Equatable {
    public let state: Int

    public init(state: Int) {
        self.state = state
    }
}
