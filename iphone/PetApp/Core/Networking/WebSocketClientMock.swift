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

    /// Story 12.2 AC6 新增：记录 connect 调用（让单测断言"connect 被调 + roomId 正确"）.
    public private(set) var connectCallArgs: [String] = []

    /// Story 12.2 AC6 新增：记录 send 调用（让单测断言"ping 被发出 + requestId 一致"）.
    public private(set) var sentMessages: [WSOutgoingMessage] = []

    /// Story 12.2 AC6 新增：connect / send 的可控 stub 错误（默认 nil = 不抛错）.
    /// 测试场景：模拟"token 失效"路径让 connect 抛 WSError.tokenMissing.
    public var connectError: WSError?
    public var sendError: WSError?

    /// fix-review round 4 P2（Story 12.7）测试用：让 `connect(roomId:)` await 一个外部 gate
    /// 才完成，模拟"用户在 connect mid-flight 切换房间"的 race 时序.
    /// 默认 nil = 不 gate（connect 立即返回 / 立即抛 connectError）；
    /// non-nil = 每次 connect 调用追加一个 continuation；测试调 `releaseConnect(at:)` 触发完成.
    /// 与现有 `connectError` 的协作：gate 释放后再判 `connectError` 是否抛错 ——
    ///   gateContinuations resume → 检查 connectError → throw / return.
    private var gateContinuations: [CheckedContinuation<Void, Error>] = []

    /// 开启 connect gate（async）—— 启用后 connect 调用会 await 直到 releaseConnect.
    public var connectShouldGate: Bool = false

    public init() {
        let (stream, cont) = AsyncStream<WSMessage>.makeStream()
        self.currentStream = stream
        self.currentContinuation = cont
    }

    /// fix-review round 4 P2 测试用：释放第 `index` 个 gate（按 connect 调用顺序）.
    /// `throwing` = true → 让 await 端 throw stub error（模拟 disconnect 触发的 stale failure）；
    /// false → 正常完成（成功 connect）.
    public func releaseConnect(at index: Int, throwing: Bool = false) {
        guard index < gateContinuations.count else { return }
        let cont = gateContinuations[index]
        if throwing {
            cont.resume(throwing: WSError.connectionFailed(underlyingDescription: "MockStaleClose"))
        } else {
            cont.resume()
        }
    }

    /// 测试用：手动 yield 消息驱动 RealRoomViewModel 解析路径.
    public func emit(_ message: WSMessage) {
        currentContinuation.yield(message)
    }

    /// Story 12.5 测试用：手动 emit `connectionStateChanged` 到 stream，让单测验证 vm 处理路径.
    /// 不实装真实 reconnect 状态机（mock 无 closeCode / backoff 逻辑）—— 仅暴露 emit 接缝.
    /// reconnect 状态机本身的单测在 `WebSocketClientImplTests.swift` 通过 fake `WebSocketTaskHandle` 覆盖.
    public func emitConnectionState(_ state: WSConnectionState) {
        currentContinuation.yield(.connectionStateChanged(state))
    }

    /// Story 12.2 AC6：mock connect —— 不真实拨号，只记录调用 + 可选抛错.
    /// fix-review round 4 P2：可选 gate 模式 —— `connectShouldGate = true` 时本调用 await
    /// 直到 `releaseConnect(at:)` 触发；让单测能在 connect mid-flight 切换房间.
    public func connect(roomId: String) async throws {
        connectCallArgs.append(roomId)
        if connectShouldGate {
            try await withCheckedThrowingContinuation { cont in
                self.gateContinuations.append(cont)
            }
        }
        if let err = connectError { throw err }
    }

    /// Story 12.2 AC6：mock send —— 不真实发送，只记录调用 + 可选抛错.
    public func send(_ message: WSOutgoingMessage) async throws {
        sentMessages.append(message)
        if let err = sendError { throw err }
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
