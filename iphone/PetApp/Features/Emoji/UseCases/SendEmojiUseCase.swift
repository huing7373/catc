// SendEmojiUseCase.swift
// Story 18.3 AC3: V1 §12.2 `emoji.send` 业务 UseCase 包装层.
//
// 设计原则:
//   - 单一职责: 把"用户选中 emojiCode → 通过 WebSocketClient 发 emoji.send" 封装为业务 UseCase,
//     调用方(RealRoomViewModel.onEmojiSelected)无需关心 wire schema / requestId 生成 / WS encode
//   - 透传 WSError: send 失败 (.notConnected / .decodingFailed) 原始抛出, 由 vm 层 mapError 走 toast
//   - 不做 emojiCode 合法性二次校验: V1 §12.2 行 2074 缓存校验由 caller (vm) 在调用前完成
//     (用 LoadEmojisUseCase cache); UseCase 是 transport 包装层不重复职责
//   - fire-and-forget 语义: 不等 server 任何响应 (V1 §12.2 行 2024 "无 HTTP 响应、无 server → client ack 消息")
//   - requestId 在 UseCase 内部生成 (caller 无需关心): 用 "emoji_<ms_timestamp>" 格式 (V1 §12.2 行 1993 推荐)
//
// 物理位置: Features/Emoji/UseCases/ 与 LoadEmojisUseCase.swift / EmojisEndpoints.swift 同模块.

import Foundation

public protocol SendEmojiUseCaseProtocol: Sendable {
    /// 发送 emoji.send 消息.
    /// - Parameter emojiCode: V1 §11.1 emojiCode 字符集 [a-z0-9_-], length 1-64; caller 应已校验合法性.
    /// - Throws: `WSError.notConnected` (WS 未连接) / `WSError.decodingFailed` (JSON 序列化失败, 理论不该);
    ///           其他 WSError case 同模式透传不转换.
    /// 调用约定: caller 应用 do-try-catch 包裹; 不抛错 = server 端"已收到 frame" (server 处理 / 广播失败由
    /// fire-and-forget 静默处理, 不通过抛错告知 caller; V1 §12.2 行 2031 钦定).
    func execute(emojiCode: String) async throws
}

public final class DefaultSendEmojiUseCase: SendEmojiUseCaseProtocol {
    private let webSocketClient: WebSocketClient

    public init(webSocketClient: WebSocketClient) {
        self.webSocketClient = webSocketClient
    }

    public func execute(emojiCode: String) async throws {
        // V1 §12.2 行 1993: requestId 推荐格式 "emoji_<ts_ms>"; client 自行生成, server 不强制.
        let requestId = "emoji_\(Int(Date().timeIntervalSince1970 * 1000))"
        let message = WSOutgoingMessage.emojiSend(requestId: requestId, emojiCode: emojiCode)
        try await webSocketClient.send(message)
        // V1 §12.2 行 2024: 不等 server ack; send 返回即视为本 UseCase 成功.
    }
}
