// RoomNavigationStaleError.swift
// Story 12.7 r14 [P1] fix（codex review）：UseCase stale-success path 抛此 error 而非 silent return.
//
// 背景（r10/r12 累计架构）：
//   - r10 引入 `appState.roomNavigationGeneration` 单调计数器 token，capture entryGen → await HTTP →
//     compare liveGen，mismatch 视为 navigation cycle 已发生（用户已 navigate away）.
//   - r10/r12 的 stale path 行为：CreateRoomUseCase / JoinRoomUseCase / LeaveRoomUseCase 在 stale 时
//     **静默 skip setCurrentRoomId** 但 **仍返回 success（无错误）**.
//
// r14 codex 发现的 desync：
//   - server 端已经 commit 用户进 room（或离开 room）.
//   - client 端因 stale guard 没写 appState.currentRoomId → UI 仍留在旧状态.
//   - 后续 create/join 会拿 6003（already-in-room）/ 6001（not in room）等业务错误，直到下一次 /home
//     hydrate 重新同步 authoritative state.
//
// 修复策略（推荐 r14 选项 B）：
//   1. UseCase 的 stale path 抛此 error 而非 silent return.
//   2. ViewModel catch 此 error → silent skip（不走 errorPresenter，因为用户视角下没出错）
//      + 触发 home refresh（让 server /home 重新告诉 client 当前 authoritative room）.
//   3. 不向 user 弹 alert（race 是后台问题，不是 user 操作错误）.
//
// 详见 docs/lessons/2026-05-11-stale-usecase-success-must-refresh-not-silently-return.md.

import Foundation

/// Room navigation race 检测信号 —— UseCase 在 stale-success path 抛此 error.
///
/// 不持具体 roomId / generation 字段（caller ViewModel 不需要这些信息做决策；它们仅在 log 里有用，
/// 但 UseCase 内已 os_log dev-facing 信号）；保持 marker error 轻量.
///
/// **重要语义**：本 error 不是 user-facing error —— ViewModel catch 时应 silent skip + 触发 home
/// refresh，**不**调 ErrorPresenter 弹 alert / retry（用户视角下 create/join/leave 完成的非常快,
/// 没有任何"操作失败"的感受，反而是 server state 与 client UI 短暂飘移；refresh 即修复）.
public struct RoomNavigationStaleError: Error, Equatable, Sendable {
    /// stale 检测发生在哪个 UseCase（dev-facing log；ViewModel 不分支处理）.
    public enum Source: String, Sendable {
        case createRoom
        case joinRoom
        case leaveRoom
    }

    public let source: Source

    public init(source: Source) {
        self.source = source
    }
}
