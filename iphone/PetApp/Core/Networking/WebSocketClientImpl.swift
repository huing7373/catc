// WebSocketClientImpl.swift
// Story 12.2 AC5: WebSocketClient protocol 真实实装 —— 基于 URLSessionWebSocketTask.
//
// 设计原则：
//   - URLSessionWebSocketTask 是 iOS 13+ 原生 API（与 ADR-0002 §3.1 钦定 standard library / 不引第三方一致）
//   - 拨号路径用 URLRequest（不是 URL）—— 留 future header bearer 接缝；当前节点 4 阶段 query token
//   - receive 循环走单一长任务：connect 时启动，disconnect / error 时 cancel + finish stream
//   - prepareForReconnect 与 WebSocketClientMock 同语义：cancel 旧 task + 清旧 stream + 新建 fresh stream/continuation
//   - 所有 mutable state 走 NSLock 保护（class final + @unchecked Sendable + private 字段；与 WebSocketClientMock 同模式）
//
// 不实装：
//   - 心跳定时（Story 12.6）—— 本 story 仅暴露 send(.ping(...)) API
//   - 自动重连（Story 12.5）—— 本 story disconnect 后 stream finish；caller 显式 prepareForReconnect + connect 才复活
//   - 后台 / 前台切换（Story 12.5）
//   - close code 解析（Story 12.5 reconnect 状态机才需要 close code 信号）
//
// 测试 hook 模式：因为 URLSessionWebSocketTask 不可子类化（init 是 NS_UNAVAILABLE，send/receive 在
// extension 内非 @objc 不可 override），引入 internal protocol `WebSocketTaskHandle` + 默认 wrapper
// `URLSessionWebSocketTaskHandle`；测试可注入 `WebSocketTaskFactory` 构造 fake handle.

import Foundation
import os.log

// MARK: - Internal abstractions for testability

/// 抽象 underlying WS task —— 让单测可注入 fake.
/// production 实装：`URLSessionWebSocketTaskHandle`（包 URLSessionWebSocketTask）
internal protocol WebSocketTaskHandle: AnyObject, Sendable {
    /// 是否仍可发送（state == .running）.
    var isRunning: Bool { get }

    func resume()
    func cancel(with closeCode: URLSessionWebSocketTask.CloseCode, reason: Data?)
    func send(_ message: URLSessionWebSocketTask.Message) async throws
    func receive() async throws -> URLSessionWebSocketTask.Message
}

/// production 默认 wrapper：把真实 URLSessionWebSocketTask 委托到 handle 协议.
final class URLSessionWebSocketTaskHandle: WebSocketTaskHandle, @unchecked Sendable {
    private let task: URLSessionWebSocketTask

    init(task: URLSessionWebSocketTask) {
        self.task = task
    }

    var isRunning: Bool { task.state == .running }

    func resume() { task.resume() }

    func cancel(with closeCode: URLSessionWebSocketTask.CloseCode, reason: Data?) {
        task.cancel(with: closeCode, reason: reason)
    }

    func send(_ message: URLSessionWebSocketTask.Message) async throws {
        try await task.send(message)
    }

    func receive() async throws -> URLSessionWebSocketTask.Message {
        try await task.receive()
    }
}

/// 抽象 task 工厂 —— production 路径用 URLSession.shared.webSocketTask(with:)；测试注入 fake.
internal protocol WebSocketTaskFactory: Sendable {
    func makeTask(with request: URLRequest) -> WebSocketTaskHandle
}

/// production 默认工厂：基于 URLSession.shared 构造 URLSessionWebSocketTask + wrapper.
final class URLSessionWebSocketTaskFactory: WebSocketTaskFactory, @unchecked Sendable {
    private let session: URLSession

    init(session: URLSession = .shared) {
        self.session = session
    }

    func makeTask(with request: URLRequest) -> WebSocketTaskHandle {
        URLSessionWebSocketTaskHandle(task: session.webSocketTask(with: request))
    }
}

// MARK: - WebSocketClientImpl

public final class WebSocketClientImpl: WebSocketClient, @unchecked Sendable {

    private static let logger = OSLog(subsystem: "com.zhuming.pet.app", category: "WebSocketClientImpl")

    // MARK: - Dependencies

    /// host-only baseURL（与 APIClient 同源；不含 /api/v1 前缀；scheme 转换 http→ws / https→wss 在内部处理）.
    private let baseURL: URL

    /// token 来源闭包 —— 拨号时同步取最新 token（保 token 刷新后立即生效）.
    /// 闭包内通常调 keychainStore.get(forKey: KeychainKey.authToken.rawValue)；nil / 空字符串 → connect 抛 WSError.tokenMissing.
    private let tokenProvider: () -> String?

    /// 内部 task 工厂 —— production 默认 URLSessionWebSocketTaskFactory；测试可注入 fake.
    private let taskFactory: WebSocketTaskFactory

    // MARK: - State (mutable, lock-protected)

    private let lock = NSLock()

    /// 当前 underlying WS task handle（ connect 时赋值；disconnect / receive 异常时 cancel + nil）.
    private var underlyingTask: WebSocketTaskHandle?

    /// receive 长任务 —— connect 后启动 for-await receive 循环；disconnect / prepareForReconnect 时 cancel.
    private var receiveTask: Task<Void, Never>?

    /// 当前消息 stream + continuation（与 WebSocketClientMock 同模式：var backed by computed `messages`）.
    private var currentStream: AsyncStream<WSMessage>
    private var currentContinuation: AsyncStream<WSMessage>.Continuation

    /// fix-review round 1 P1（Story 12.2 review）：handshake 一次性 latch.
    /// connect() 内 await 此 continuation；receive loop 在第一帧 / 第一次 error 时 resume.
    /// resolve 后置 nil 防双 resume；disconnect / prepareForReconnect 也会兜底 resume(throwing:).
    private var connectGate: CheckedContinuation<Void, Error>?

    // MARK: - WebSocketClient protocol

    public var messages: AsyncStream<WSMessage> {
        lock.lock()
        defer { lock.unlock() }
        return currentStream
    }

    /// production 默认 init —— urlSession 默认 .shared；与 APIClient 同模式.
    public convenience init(
        baseURL: URL,
        tokenProvider: @escaping () -> String?,
        urlSession: URLSession = .shared
    ) {
        self.init(
            baseURL: baseURL,
            tokenProvider: tokenProvider,
            taskFactory: URLSessionWebSocketTaskFactory(session: urlSession)
        )
    }

    /// 测试入口 init —— 注入 task factory（让 fake task handle 接管拨号 / receive / send 路径）.
    /// internal 可见：仅 PetApp / PetAppTests 模块内可见，外部不暴露.
    internal init(
        baseURL: URL,
        tokenProvider: @escaping () -> String?,
        taskFactory: WebSocketTaskFactory
    ) {
        self.baseURL = baseURL
        self.tokenProvider = tokenProvider
        self.taskFactory = taskFactory
        let (stream, cont) = AsyncStream<WSMessage>.makeStream()
        self.currentStream = stream
        self.currentContinuation = cont
    }

    public func connect(roomId: String) async throws {
        // 1. token check
        guard let token = tokenProvider(), !token.isEmpty else {
            throw WSError.tokenMissing
        }
        // 2. URL 构造（http → ws / https → wss）
        let wsURL = try makeWSURL(roomId: roomId, token: token)
        // 3. URLRequest
        let request = URLRequest(url: wsURL)
        // 4. webSocketTask + resume + 启动 receive 长任务
        let task = taskFactory.makeTask(with: request)
        let continuation: AsyncStream<WSMessage>.Continuation
        lock.lock()
        // Story 12.1 r6 lesson：同 instance 复用避免泄漏 —— connect 前若有旧 task / receiveTask，先清掉.
        // 正常 caller 路径应是 disconnect → prepareForReconnect → connect；这里防御性兜底.
        if let oldTask = underlyingTask {
            oldTask.cancel(with: .normalClosure, reason: nil)
        }
        receiveTask?.cancel()
        underlyingTask = task
        continuation = currentContinuation
        lock.unlock()

        task.resume()

        // fix-review round 1 P1（Story 12.2 review）：阻塞 await 第一帧 / 第一次 error，
        // 让 caller 真实拿到 handshake 失败信号（DNS/TLS / 4001 token 过期 / 4004 房间满 等）.
        //
        // 设计：
        //   - 起 receive 长任务前先创建 `connectGate` continuation（一次性 latch；nil 表示已 resolve）
        //   - receive loop 收到第一帧 → resume(success) + 把第一帧 yield 到 stream（不丢消息）
        //   - receive loop 第一次 catch error → resume(throwing: WSError.connectionFailed)
        //   - resolve 后 connectGate = nil；后续帧 / 后续 error 走 stream finish 路径（既有逻辑）
        //
        // 时序契约：V1 §12.1 钦定握手成功后 server 必发 `room.snapshot` 作为第一条消息 ——
        //   "第一帧成功" 与 "握手成功" 等价；4001 / 4004 等 close 在 receive 上以 URLError 出现.
        try await withCheckedThrowingContinuation { (cont: CheckedContinuation<Void, Error>) in
            lock.lock()
            connectGate = cont
            lock.unlock()
            // 启动 receive 长任务（持 task / continuation 两个 reference 进闭包）
            startReceiveLoop(task: task, continuation: continuation)
        }
        os_log(.debug, log: WebSocketClientImpl.logger, "connect resolved for roomId=%{public}@", roomId)
    }

    public func send(_ message: WSOutgoingMessage) async throws {
        let task: WebSocketTaskHandle?
        lock.lock()
        task = underlyingTask
        lock.unlock()

        guard let activeTask = task, activeTask.isRunning else {
            throw WSError.notConnected
        }
        let text = try WSMessageCodec.encode(message)
        try await activeTask.send(.string(text))
    }

    public func disconnect() {
        lock.lock()
        let oldTask = underlyingTask
        let oldReceiveTask = receiveTask
        let oldContinuation = currentContinuation
        underlyingTask = nil
        receiveTask = nil
        lock.unlock()

        // client-initiated close（V1 §12.1 close code 1000 normalClosure）
        oldTask?.cancel(with: .normalClosure, reason: nil)
        oldReceiveTask?.cancel()
        oldContinuation.finish()
        // fix-review round 1 P1：若 caller 在 connect() await 期间 disconnect → 兜底 resolve gate 防 hang.
        resolveConnectGate(
            success: false,
            error: WSError.connectionFailed(underlyingDescription: "disconnect before first frame")
        )
        os_log(.debug, log: WebSocketClientImpl.logger, "disconnect issued")
    }

    public func prepareForReconnect() {
        let (stream, cont) = AsyncStream<WSMessage>.makeStream()
        lock.lock()
        let oldTask = underlyingTask
        let oldReceiveTask = receiveTask
        underlyingTask = nil
        receiveTask = nil
        self.currentStream = stream
        self.currentContinuation = cont
        lock.unlock()

        // 与 WebSocketClientMock.prepareForReconnect 同语义：cancel 旧资源 + 新建 fresh stream
        oldTask?.cancel(with: .normalClosure, reason: nil)
        oldReceiveTask?.cancel()
        // fix-review round 1 P1：若 caller 在 connect() await 期间 prepareForReconnect → 兜底 resolve gate 防 hang.
        resolveConnectGate(
            success: false,
            error: WSError.connectionFailed(underlyingDescription: "prepareForReconnect before first frame")
        )
        os_log(.debug, log: WebSocketClientImpl.logger, "prepareForReconnect (fresh stream issued)")
    }

    // MARK: - Internal helpers

    /// V1 §12.1 连接 URL 构造：`{ws_scheme}://{host}/ws/rooms/{roomId}?token={url-encoded}`.
    /// scheme 转换：http → ws / https → wss（与 baseURL 协议一致）.
    /// 提为 internal（非 private）方便单测覆盖（与既有 APIClient 同模式）.
    internal func makeWSURL(roomId: String, token: String) throws -> URL {
        guard var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: false) else {
            throw WSError.invalidURL
        }
        // scheme 转换
        switch components.scheme?.lowercased() {
        case "http": components.scheme = "ws"
        case "https": components.scheme = "wss"
        case "ws", "wss": break  // 已是 ws scheme
        default: throw WSError.invalidURL
        }
        // path：附加 /ws/rooms/{roomId}（baseURL 不含 /api/v1 前缀，host-only）
        let basePath = components.path.hasSuffix("/") ? String(components.path.dropLast()) : components.path
        components.path = "\(basePath)/ws/rooms/\(roomId)"
        // query：手工 percent-encode token —— `URLComponents.queryItems` 默认**不**编码 `+`（query 允许字符），
        // 但 `+` 在 application/x-www-form-urlencoded 语义下会被解为空格 —— server query 解析器（gorilla/url.Values）
        // 通常按 form-encoded 解，因此 token 中的 `+` `/` `=` 必须 percent-encode（V1 §12.1 钦定 URL-encoded token）.
        // 用 .urlQueryAllowed 减去 reserved 子集：`+`、`/`、`=`、`&`、`?`、`#`.
        let queryAllowed = CharacterSet.urlQueryAllowed.subtracting(CharacterSet(charactersIn: "+/=&?#"))
        guard let encodedToken = token.addingPercentEncoding(withAllowedCharacters: queryAllowed) else {
            throw WSError.invalidURL
        }
        components.percentEncodedQueryItems = [URLQueryItem(name: "token", value: encodedToken)]
        guard let url = components.url else { throw WSError.invalidURL }
        return url
    }

    /// 启动 receive 长任务：循环 await task.receive() → decode → yield 到 stream.
    /// 异常 / close → finish stream + nil out underlyingTask.
    ///
    /// fix-review round 1 P1：在第一帧成功 / 第一次 error 时 resolve `connectGate`,
    /// 让 connect() 真实拿到握手结果（成功 → return；失败 → throw WSError.connectionFailed）.
    private func startReceiveLoop(
        task: WebSocketTaskHandle,
        continuation: AsyncStream<WSMessage>.Continuation
    ) {
        let newReceiveTask = Task { [weak self] in
            var firstFrameReceived = false
            // fix-review round 1 P1：loop 通过 cancellation 退出（while 条件假）时也必须 finish 继承 continuation,
            // 否则 prepareForReconnect / disconnect 后旧 stream 永远 hang 在 for-await 上.
            // defer 兜底兼容所有退出路径（cancel / catch / return）.
            defer {
                continuation.finish()
                if !firstFrameReceived {
                    // cancel 在握手期发生 —— 让 connect() 拿到失败信号
                    self?.resolveConnectGate(
                        success: false,
                        error: WSError.connectionFailed(underlyingDescription: "receive task cancelled before first frame")
                    )
                }
            }
            while !Task.isCancelled {
                do {
                    let frame = try await task.receive()
                    if Task.isCancelled { return }
                    // 第一帧到达 = handshake / token / room_members 校验全过 = §12.1 钦定 server 推 room.snapshot.
                    // 即使是 .unknown 或 .data，能从 receive() 返回也意味着握手已完成.
                    if !firstFrameReceived {
                        firstFrameReceived = true
                        self?.resolveConnectGate(success: true, error: nil)
                    }
                    switch frame {
                    case .string(let text):
                        let message = WSMessageCodec.decode(text)
                        continuation.yield(message)
                    case .data(let data):
                        // V1 §12.2 / §12.3：text frame only；binary frame 不应出现 —— 兜底兼容（解为 UTF-8 string 走同路径）
                        if let text = String(data: data, encoding: .utf8) {
                            let message = WSMessageCodec.decode(text)
                            continuation.yield(message)
                        } else {
                            os_log(.error, log: WebSocketClientImpl.logger, "binary frame non-UTF-8")
                        }
                    @unknown default:
                        os_log(.error, log: WebSocketClientImpl.logger, "unknown WS frame type")
                    }
                } catch {
                    // 拨号 / 连接期异常 → finish stream（continuation.finish() 由 defer 保底）
                    os_log(
                        .error,
                        log: WebSocketClientImpl.logger,
                        "receive failed: %{public}@",
                        String(describing: error)
                    )
                    if let strongSelf = self {
                        strongSelf.lock.lock()
                        // 仅当当前 task 还是这个 task 时才清（避免覆盖 prepareForReconnect / disconnect 设置的状态）
                        if strongSelf.underlyingTask === task {
                            strongSelf.underlyingTask = nil
                        }
                        strongSelf.lock.unlock()
                    }
                    return
                }
            }
        }
        lock.lock()
        receiveTask = newReceiveTask
        lock.unlock()
    }

    /// fix-review round 1 P1：resolve connectGate 一次（多次调用安全 —— 二次后是 no-op）.
    /// success=true → resume(()); success=false → resume(throwing: error).
    private func resolveConnectGate(success: Bool, error: Error?) {
        lock.lock()
        let cont = connectGate
        connectGate = nil
        lock.unlock()
        guard let cont = cont else { return }
        if success {
            cont.resume()
        } else {
            cont.resume(throwing: error ?? WSError.connectionFailed(underlyingDescription: "unknown"))
        }
    }

    deinit {
        underlyingTask?.cancel(with: .normalClosure, reason: nil)
        receiveTask?.cancel()
        currentContinuation.finish()
        // fix-review round 1 P1：deinit 期间 caller 还在 await connect() 概率极低（caller 通常持 strong ref），
        // 防御性兜底 resume(throwing:) 让 await 立即返回，不留 dangling continuation 触发 fatalError.
        if let cont = connectGate {
            connectGate = nil
            cont.resume(throwing: WSError.connectionFailed(underlyingDescription: "deinit before first frame"))
        }
    }
}
