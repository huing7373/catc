// WebSocketClientMock.swift
// Story 12.1 AC3: WebSocketClient 测试 mock —— 手动注入消息序列驱动 RealRoomViewModel 单测.
//
// 设计：与 MockHomeViewModel / MockRoomViewModel 同精神（actor / class / 直接断言 @Published 字段）.
// AsyncStream 用 `AsyncStream.makeStream()` 持有 continuation，让测试方法手动 yield 消息.
//
// 放 main target（而非 testing target）是为了让 #Preview 也能用；与 MockRoomViewModel 同模式
// （Story 37.8 落地 MockRoomViewModel 也放 main target，PetApp/Features/Room/ViewModels 目录下）.

import Foundation

public final class WebSocketClientMock: WebSocketClient, @unchecked Sendable {
    public let messages: AsyncStream<WSMessage>
    private let continuation: AsyncStream<WSMessage>.Continuation

    /// 测试用：记录是否调过 disconnect.
    public private(set) var didDisconnect: Bool = false

    public init() {
        let (stream, cont) = AsyncStream<WSMessage>.makeStream()
        self.messages = stream
        self.continuation = cont
    }

    /// 测试用：手动 yield 消息驱动 RealRoomViewModel 解析路径.
    public func emit(_ message: WSMessage) {
        continuation.yield(message)
    }

    public func disconnect() {
        didDisconnect = true
        continuation.finish()
    }
}
