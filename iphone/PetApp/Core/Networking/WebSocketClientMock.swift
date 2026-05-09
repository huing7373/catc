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
    /// fix-review round 2 P1：`messages` 由 `let` 改为计算属性 backed by `var`，
    /// 让 `prepareForReconnect()` 能 swap 出新的 stream（disconnect 后旧 stream 已 finish，
    /// 复用同 client 的新 consumer 必须拿到全新 stream，否则永远收不到消息）.
    private var currentStream: AsyncStream<WSMessage>
    private var currentContinuation: AsyncStream<WSMessage>.Continuation

    public var messages: AsyncStream<WSMessage> {
        currentStream
    }

    /// 测试用：记录是否调过 disconnect.
    public private(set) var didDisconnect: Bool = false

    /// 测试用：记录 `prepareForReconnect()` 调用次数（让 viewmodel A→B / leave-rejoin 路径可断言）.
    public private(set) var prepareForReconnectCallCount: Int = 0

    public init() {
        let (stream, cont) = AsyncStream<WSMessage>.makeStream()
        self.currentStream = stream
        self.currentContinuation = cont
    }

    /// 测试用：手动 yield 消息驱动 RealRoomViewModel 解析路径.
    public func emit(_ message: WSMessage) {
        currentContinuation.yield(message)
    }

    public func disconnect() {
        didDisconnect = true
        currentContinuation.finish()
    }

    /// fix-review round 2 P1：实装 `prepareForReconnect()` —— 创建新 stream + continuation 替换
    /// 旧的（旧 stream 已 finish；caller 接下来读 `messages` 会拿到新 stream）.
    /// Story 12.2 production `WebSocketClientImpl.prepareForReconnect()` 行为类似（内部重置 task 状态）.
    public func prepareForReconnect() {
        prepareForReconnectCallCount += 1
        let (stream, cont) = AsyncStream<WSMessage>.makeStream()
        self.currentStream = stream
        self.currentContinuation = cont
    }
}
