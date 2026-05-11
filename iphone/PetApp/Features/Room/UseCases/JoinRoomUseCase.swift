// JoinRoomUseCase.swift
// Story 12.7 AC2: 加入房间 UseCase（POST /api/v1/rooms/{roomId}/join → 校验 → 写 appState.currentRoomId）.
//
// 流程：
//   1. 调用入口 capture `entryRoomId = appState.currentRoomId`（idle Home join 时为 nil；
//      也可能其他场景已有值 —— guard 用 == entryRoomId 而非 == nil 保证语义一致）
//   2. 调 roomRepository.joinRoom(roomId:) → JoinRoomResponse
//   3. 校验 response.roomId == request roomId（防 server bug / proxy 改写 path —— 极小概率但成本极低）
//   4. 不一致 → throw APIError.decoding(JoinRoomMismatchError(...))（让 ErrorPresenter 默认 mapper 走 .alert / .retry）
//   5. await MainActor.run { guard appState.currentRoomId == entryRoomId else skip; setCurrentRoomId(roomId) }
//
// 错误处理：APIError 原样透传（含 .business 全 case：6001 / 6002 / 6003 / 6005 / 1002 / 1009）.
// caller 层（RealHomeViewModel.onJoinRoomConfirm / RealFriendsViewModel.onJoinFriendTap）case-by-case 弹对应 alert.
//
// spec Open Question §4 决议：mismatch 检查保留 —— 一行 == 比较成本极低 + 让 dev 看到 server bug 信号.

import Foundation
import os.log

public protocol JoinRoomUseCaseProtocol: Sendable {
    /// 调 POST /api/v1/rooms/{roomId}/join 加入房间 → 校验 response.roomId 一致 → 写 appState.currentRoomId.
    /// - Parameter roomId: 目标房间号（caller 传入 —— 来自 modal 输入 / 好友卡片 currentRoomId）
    /// - Throws: APIError（含 .decoding 包装 JoinRoomMismatchError）
    func execute(roomId: String) async throws
}

public struct DefaultJoinRoomUseCase: JoinRoomUseCaseProtocol {
    private let roomRepository: RoomRepositoryProtocol
    private let appState: AppState

    public init(roomRepository: RoomRepositoryProtocol, appState: AppState) {
        self.roomRepository = roomRepository
        self.appState = appState
    }

    public func execute(roomId: String) async throws {
        // Story 12.7 r6 [P1] fix（codex review）：与 LeaveRoomUseCase r2 [P2] 同精神 ——
        // capture 调用入口的 `entryRoomId`，await 返回后 guard `appState.currentRoomId == entryRoomId`
        // 才 setCurrentRoomId. 防止 race：
        //   1. user 从 idle Home join room A（entryRoomId == nil）
        //   2. await 期间 user 切 tab → join room B（currentRoomId 已是 "B"）
        //   3. join A HTTP 200 迟到
        //   4. 旧实装无条件 setCurrentRoomId("A") → 静默把 user 切回 stale room A
        // mismatch 时静默 skip + dev-facing log（不抛错，因为这是 race 不是逻辑错误）.
        let entryRoomId: String? = await MainActor.run { appState.currentRoomId }

        let response = try await roomRepository.joinRoom(roomId: roomId)
        // mismatch 防御层（spec Open Question §4）：
        //   - server bug / proxy 改写 path / 老客户端不兼容路径 → 抛 .decoding 让 ErrorPresenter 弹 alert.
        //   - 不写 appState（roomId 不匹配的房间号写入只会让 UI 切到错误房间）.
        guard response.roomId == roomId else {
            throw APIError.decoding(underlying: JoinRoomMismatchError(
                requested: roomId,
                received: response.roomId
            ))
        }
        await MainActor.run {
            let liveRoomId = appState.currentRoomId
            guard liveRoomId == entryRoomId else {
                os_log(.info,
                       "JoinRoomUseCase: stale join response (entry=%{public}@, current=%{public}@, target=%{public}@); skip setCurrentRoomId to keep newer room selection",
                       entryRoomId ?? "nil", liveRoomId ?? "nil", roomId)
                return
            }
            appState.setCurrentRoomId(roomId)
        }
    }
}

/// JoinRoomUseCase mismatch 防御层错误（spec Dev Note "Open Question §4"）.
///
/// 触发条件：response.data.roomId != request.roomId（server bug / proxy 改写）.
/// 不向 caller 暴露 underlying server 字段，仅 dev-facing log 信号.
public struct JoinRoomMismatchError: Error, LocalizedError, Equatable, Sendable {
    public let requested: String
    public let received: String

    public init(requested: String, received: String) {
        self.requested = requested
        self.received = received
    }

    public var errorDescription: String? {
        "JoinRoom response.roomId mismatch: requested=\(requested), received=\(received)"
    }
}
