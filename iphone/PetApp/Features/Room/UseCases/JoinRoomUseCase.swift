// JoinRoomUseCase.swift
// Story 12.7 AC2: 加入房间 UseCase（POST /api/v1/rooms/{roomId}/join → 校验 → 写 appState.currentRoomId）.
//
// 流程：
//   1. 调用入口 capture `entryGen = appState.roomNavigationGeneration`（monotonic 计数器，ABA-safe，r10 P2 fix）
//   2. 调 roomRepository.joinRoom(roomId:) → JoinRoomResponse
//   3. 校验 response.roomId == request roomId（防 server bug / proxy 改写 path —— 极小概率但成本极低）
//   4. 不一致 → throw APIError.decoding(JoinRoomMismatchError(...))（让 ErrorPresenter 默认 mapper 走 .alert / .retry）
//   5. await MainActor.run { guard appState.roomNavigationGeneration == entryGen else skip; setCurrentRoomId(roomId) }
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
        // Story 12.7 r10 [P2] fix（codex review）：用 `roomNavigationGeneration` token 而非 currentRoomId equality ——
        // r6/r9 旧实装只 guard `currentRoomId == entryRoomId`，无法区分 ABA cycle：
        //   1. user 从 idle Home join room A（entryRoomId == nil, entryGen == G0）
        //   2. await 期间 user 切 tab → join room B → leave B 回 idle（currentRoomId nil → "B" → nil; gen G0→G1→G2）
        //   3. join A HTTP 200 迟到
        //   4. 旧 guard: `liveRoomId == nil == entryRoomId` → 校验通过 → 静默把 user 切到 stale room A
        //   5. 新 guard: `roomNavigationGeneration == G0` 但实际是 G2 → 拒绝 setCurrentRoomId
        // mismatch 时静默 skip + dev-facing log（不抛错，因为这是 race 不是逻辑错误）.
        // 详见 docs/lessons/2026-05-11-room-navigation-generation-token-not-room-id-equality.md.
        let entryGen: Int = await MainActor.run { appState.roomNavigationGeneration }

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
            let liveGen = appState.roomNavigationGeneration
            guard liveGen == entryGen else {
                os_log(.info,
                       "JoinRoomUseCase: stale join response (entryGen=%{public}d, currentGen=%{public}d, target=%{public}@); skip setCurrentRoomId to keep newer room selection",
                       entryGen, liveGen, roomId)
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
