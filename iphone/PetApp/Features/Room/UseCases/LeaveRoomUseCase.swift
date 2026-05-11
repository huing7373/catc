// LeaveRoomUseCase.swift
// Story 12.7 AC3: 退出房间 UseCase（POST /api/v1/rooms/{id}/leave → HTTP 200 视同 leave 完成 → setCurrentRoomId(nil)）.
//
// 流程：
//   1. 从 appState.currentRoomId 读 roomId；nil → 早 return（idempotent，不抛错）
//   2. 调 roomRepository.leaveRoom(roomId:)
//   3. **HTTP 200 = authoritative leave 信号**（V1 §10.5 r10 锁定 + lesson 2026-05-08-ws-disconnect-only-clears-ephemeral-not-membership-11-1-r3.md）：
//      成功后立即 await MainActor.run { appState.setCurrentRoomId(nil) }（**不**等 WS close 4007）.
//   4. **6004 视同成功路径**（leave-idempotent）：catch APIError.business(6004) → 仍 setCurrentRoomId(nil) +
//      不重抛（重抛会让 caller 弹 alert "你不在房间里"，体验割裂）；其他 .business 透传.
//   5. 其他 APIError 透传（保留 in-room UI 让用户重试）.
//
// 6004 leave-idempotent 决策（spec Dev Note "leave-idempotent 决策"）：
//   V1 §10.5 钦定 6004 触发条件含三种 race 场景（current_room_id == NULL / != path roomId / DELETE RowsAffected==0），
//   client UX 处理一致："我已经不在那个房间里了" → 写 setCurrentRoomId(nil) + 不弹 alert.
//   alert "你不在房间里" 让用户困惑（"我刚不还在吗？"）；retry 也无意义（重 leave 还会 6004）.
//
// HTTP 200 vs WS close 4007（spec Dev Note "HTTP 200 vs WS close 4007"）：
//   HTTP 200 是 leave 完成的唯一 authoritative signal；
//   WS close 4007 是 best-effort cleanup signal（server 端 fire-and-forget；client 可能完全收不到 / 比 HTTP 200 晚到 / leaver WS 早断）；
//   等 4007 才推进 RoomView 退出 = 卡死风险.

import Foundation
import os.log

public protocol LeaveRoomUseCaseProtocol: Sendable {
    /// 调 POST /api/v1/rooms/{currentRoomId}/leave 退出房间.
    /// - Throws: APIError（除 6004 视同成功；其他 .business / .network / .unauthorized 透传）.
    func execute() async throws
}

public struct DefaultLeaveRoomUseCase: LeaveRoomUseCaseProtocol {
    private let roomRepository: RoomRepositoryProtocol
    private let appState: AppState

    public init(roomRepository: RoomRepositoryProtocol, appState: AppState) {
        self.roomRepository = roomRepository
        self.appState = appState
    }

    public func execute() async throws {
        // 1. 早 return: appState.currentRoomId == nil（idempotent leave）.
        // MainActor hop 读 currentRoomId，与 caller（ViewModel @MainActor）调用时机解耦.
        let currentRoomId: String? = await MainActor.run { appState.currentRoomId }
        guard let roomId = currentRoomId else { return }

        // Story 12.7 r2 [P2] fix（codex review）：记录调用入口时刻的 `targetRoomId`,
        // 用于 await 返回后 guard `appState.currentRoomId == targetRoomId` —— 防止 stale-response
        // 在用户已切到新房间（如 leave A → join B 序列）的场景下 wipe 新房间状态 + disconnect fresh session.
        //
        // 6004 兼用 "已离开" 和 "current_room_id != path roomId"（V1 §10.5 三种 race 场景），
        // 任意一种迟到的 6004 response 都可能 wipe 后续状态. 因此 200 / 6004 两路都需要 guard.
        //
        // mismatch 时静默跳过 + log debug（不抛错，因为外层视角 leave 早已完成）.
        // 详见 docs/lessons/2026-05-11-leave-room-guard-target-vs-current-against-stale-response.md.
        let targetRoomId = roomId

        do {
            // 2. 调 leave HTTP API.
            _ = try await roomRepository.leaveRoom(roomId: roomId)
            // 3. HTTP 200 = authoritative：guard target == current 后再 setCurrentRoomId(nil).
            await MainActor.run {
                let liveRoomId = appState.currentRoomId
                guard liveRoomId == targetRoomId else {
                    os_log(.info,
                           "LeaveRoomUseCase: stale leave HTTP-200 response (target=%{public}@, current=%{public}@); skip setCurrentRoomId(nil) to keep newer room selection",
                           targetRoomId, liveRoomId ?? "nil")
                    return
                }
                appState.setCurrentRoomId(nil)
            }
        } catch let APIError.business(code, _, _) where code == 6004 {
            // 4. 6004 视同成功（leave-idempotent；spec Dev Note）.
            // dev-facing log 信号（spec Open Question §3 决议：暴露 console signal 让 dev 看到 race 频率，不弹 UI）.
            os_log(.info,
                   "LeaveRoomUseCase: received business 6004 (already left); treating as success path (leave-idempotent)")
            // r2 [P2] fix：同样 guard target == current 防 6004 stale response wipe 新房间.
            await MainActor.run {
                let liveRoomId = appState.currentRoomId
                guard liveRoomId == targetRoomId else {
                    os_log(.info,
                           "LeaveRoomUseCase: stale leave 6004 response (target=%{public}@, current=%{public}@); skip setCurrentRoomId(nil) to keep newer room selection",
                           targetRoomId, liveRoomId ?? "nil")
                    return
                }
                appState.setCurrentRoomId(nil)
            }
        } catch {
            // 5. 其他 APIError 透传（保留 in-room UI 让用户重试）.
            throw error
        }
    }
}
