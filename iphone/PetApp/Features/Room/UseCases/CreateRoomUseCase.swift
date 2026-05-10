// CreateRoomUseCase.swift
// Story 12.7 AC1: 创建房间 UseCase（POST /api/v1/rooms → 写 appState.currentRoomId）.
//
// 流程：
//   1. 调 roomRepository.createRoom() → CreateRoomResponse
//   2. 取 response.room.id
//   3. await MainActor.run { appState.setCurrentRoomId(roomId) }（先写 AppState 让 RealRoomViewModel.subscribeRoomIdConnect 准备 stream）
//   4. return roomId（让 caller 决定下一步 UI 流程）
//
// 顺序锁定（spec AC1）：先 set roomId，后 return —— 让 caller catch 路径不需自己再写 AppState；
// 失败路径不写 AppState（保证 6003 等错误下 currentRoomId 保持原值，UI 不切到 RoomView）.
//
// 错误处理：APIError 原样透传（含 .business(6003 / 1009 / ...) / .network / .unauthorized / .decoding）.
// caller 层（RealHomeViewModel.onCreateTap）负责映射到 ErrorPresenter（6003 → alert "你已经在房间里了"；其他走默认 mapper）.
//
// 不在本 story 范围（设计选择）：
//   - 不调 webSocketClient.connect（WS lifecycle 由 RealRoomViewModel.subscribeRoomIdConnect 唯一管；
//     UseCase 只写 AppState；详见 spec Dev Note "connect 触发的责任划分"）
//   - 不做 retry / 指数退避 / 缓存（caller 层 ErrorPresenter 处理用户主动重试）
//   - 不在 UseCase 内 catch APIError 转码（让 ViewModel 接 APIError 自己 case-by-case 映射 alert 文案）

import Foundation

public protocol CreateRoomUseCaseProtocol: Sendable {
    /// 调 POST /api/v1/rooms 创建房间 → 写 appState.currentRoomId → 返回 roomId.
    /// - Returns: 新建房间的 BIGINT roomId（字符串化）
    /// - Throws: APIError（全部 case 原样透传）
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
        let response = try await roomRepository.createRoom()
        let roomId = response.room.id
        // AppState 是 @MainActor + ObservableObject + @Published；
        // UseCase 跑在 detached actor 调度（async function），需要显式 hop 到 MainActor 才能写 @Published.
        // spec Open Question §2 决议：先 set 后 return（让 RealRoomViewModel.subscribeRoomIdConnect 准备 stream）.
        await MainActor.run {
            appState.setCurrentRoomId(roomId)
        }
        return roomId
    }
}
