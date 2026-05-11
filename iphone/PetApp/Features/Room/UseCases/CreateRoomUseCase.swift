// CreateRoomUseCase.swift
// Story 12.7 AC1: 创建房间 UseCase（POST /api/v1/rooms → 写 appState.currentRoomId）.
//
// 流程：
//   1. 调用入口 capture `entryGen = appState.roomNavigationGeneration`（monotonic 计数器，ABA-safe，r10 P2 fix）
//   2. 调 roomRepository.createRoom() → CreateRoomResponse
//   3. 取 response.room.id
//   4. await MainActor.run { guard appState.roomNavigationGeneration == entryGen else skip; setCurrentRoomId(roomId) }
//   5. return roomId（让 caller 决定下一步 UI 流程）
//
// 顺序锁定（spec AC1）：先 set roomId，后 return —— 让 caller catch 路径不需自己再写 AppState；
// 失败路径不写 AppState（保证 6003 等错误下 currentRoomId 保持原值，UI 不切到 RoomView）.
//
// 错误处理：APIError 原样透传（含 .business(6003 / 1009 / ...) / .network / .unauthorized / .decoding）.
// caller 层（RealHomeViewModel.onCreateTap）负责映射到 ErrorPresenter（6003 → alert "你已经在房间里了"；其他走默认 mapper）.
//
// **r14 [P1] fix（codex review）**：stale-success path 改抛 `RoomNavigationStaleError`（之前 silent
// return roomId）—— silent success 让 server (已建好 room) 与 client UI (仍 idle) 短暂 desync,
// 后续 create/join 会拿 6003 直到 /home 重新 hydrate. 抛 error 让 ViewModel 显式知道发生 race
// → silent skip errorPresenter + 触发 home refresh 拿 authoritative state.
// 详见 docs/lessons/2026-05-11-stale-usecase-success-must-refresh-not-silently-return.md.
//
// 不在本 story 范围（设计选择）：
//   - 不调 webSocketClient.connect（WS lifecycle 由 RealRoomViewModel.subscribeRoomIdConnect 唯一管；
//     UseCase 只写 AppState；详见 spec Dev Note "connect 触发的责任划分"）
//   - 不做 retry / 指数退避 / 缓存（caller 层 ErrorPresenter 处理用户主动重试）
//   - 不在 UseCase 内 catch APIError 转码（让 ViewModel 接 APIError 自己 case-by-case 映射 alert 文案）

import Foundation
import os.log

public protocol CreateRoomUseCaseProtocol: Sendable {
    /// 调 POST /api/v1/rooms 创建房间 → 写 appState.currentRoomId → 返回 roomId.
    /// - Returns: 新建房间的 BIGINT roomId（字符串化）
    /// - Throws: APIError（全部 case 原样透传）/ RoomNavigationStaleError（r14 P1 fix：navigation
    ///   race 检测到 entryGen != liveGen 时抛此 error，让 ViewModel silent skip + 触发 home refresh）
    func execute() async throws -> String
}

public struct DefaultCreateRoomUseCase: CreateRoomUseCaseProtocol {
    private let roomRepository: RoomRepositoryProtocol
    private let appState: AppState

    public init(roomRepository: RoomRepositoryProtocol, appState: AppState) {
        self.roomRepository = roomRepository
        self.appState = appState
    }

    public func execute() async throws -> String {
        // Story 12.7 r10 [P2] fix（codex review）：用 `roomNavigationGeneration` token 而非 currentRoomId equality ——
        // r6/r9 旧实装只 guard `currentRoomId == entryRoomId`，无法区分 ABA cycle：
        //   1. user 在 idle Home 点 Create（entryRoomId == nil, entryGen == G0）
        //   2. createRoom() HTTP in-flight 期间 user 切到 friend tab → join room B → leave B 回 idle
        //      （currentRoomId 经历 nil → "B" → nil，但 generation 已 G0 → G1 → G2）
        //   3. createRoom() HTTP 200 迟到带回 newRoomId "A"
        //   4. 旧 guard: `liveRoomId == nil == entryRoomId` → 校验通过 → user 被强制切到 stale 房间 A
        //   5. 新 guard: `roomNavigationGeneration == G0` 但实际是 G2 → 拒绝 setCurrentRoomId
        // generation 严格单调递增（即使 currentRoomId 回到原值），是 ABA-safe 的 navigation-cycle invariant.
        // mismatch 时静默 skip + dev-facing log（不抛错，因 server 端 room 已建好，但 client 已 move on）.
        // 详见 docs/lessons/2026-05-11-room-navigation-generation-token-not-room-id-equality.md.
        let entryGen: Int = await MainActor.run { appState.roomNavigationGeneration }

        let response = try await roomRepository.createRoom()
        let roomId = response.room.id
        // AppState 是 @MainActor + ObservableObject + @Published；
        // UseCase 跑在 detached actor 调度（async function），需要显式 hop 到 MainActor 才能写 @Published.
        // spec Open Question §2 决议：先 set 后 return（让 RealRoomViewModel.subscribeRoomIdConnect 准备 stream）.
        //
        // r14 [P1] fix（codex review）：stale path 抛 RoomNavigationStaleError 而非 silent skip + return.
        // silent return 让 server (已 commit 用户进 room A) 与 client UI (因 stale 仍 idle) desync,
        // 后续 create/join 会拿到 6003（已在房间）直到下次 /home hydrate. 抛 error 让 ViewModel
        // 收到信号 → silent skip errorPresenter + 触发 home refresh 拿 authoritative state.
        let staleSignal: Bool = await MainActor.run {
            let liveGen = appState.roomNavigationGeneration
            guard liveGen == entryGen else {
                os_log(.info,
                       "CreateRoomUseCase: stale create response (entryGen=%{public}d, currentGen=%{public}d, newRoom=%{public}@); skip setCurrentRoomId, will throw RoomNavigationStaleError so caller refreshes home",
                       entryGen, liveGen, roomId)
                return true
            }
            appState.setCurrentRoomId(roomId)
            return false
        }
        if staleSignal {
            throw RoomNavigationStaleError(source: .createRoom)
        }
        return roomId
    }
}
