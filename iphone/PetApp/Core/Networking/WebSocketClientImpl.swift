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

    /// Story 12.5 新增：暴露 underlying URLSessionWebSocketTask 的 closeCode 字段.
    /// receive() 抛错后该字段被 runtime 设置为 server emit 的 close code 或 `.invalid`（client 本地合成 1006）.
    /// reconnect 状态机在 catch error 时按 `closeCode.rawValue` 分类 transient / terminal（V1 §12.1 close code 表）；
    /// **绝对不**用错误描述字符串（NSLocalizedDescription）做分类 —— 脆弱、依赖 OS 国际化文案.
    /// `.invalid` (rawValue=0) ↔ 1006（V1 §12.1 行 1729 钦定 1006 仅由 client 在底层 TCP 异常断开时本地合成）.
    var closeCode: URLSessionWebSocketTask.CloseCode { get }

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

    /// Story 12.5：直通 underlying URLSessionWebSocketTask 的 closeCode（receive() 抛错后 runtime 设置）.
    var closeCode: URLSessionWebSocketTask.CloseCode { task.closeCode }

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

    // MARK: - Story 12.5 reconnect 状态机字段

    /// Story 12.5：当前正在拨号 / 已连接的 roomId —— reconnect 路径用同一 roomId 重连.
    /// `connect(roomId:)` 成功路径写入；`disconnect()` / terminal close / 超过 maxReconnectAttempts 失败 → 清 nil.
    private var currentRoomId: String?

    /// Story 12.5：当前 reconnect attempt 计数（首次 connect = 0；reconnect 第 N 次 = N）.
    /// 成功 reconnect 后清 0；caller 主动 connect 也清 0.
    private var reconnectAttempt: Int = 0

    /// Story 12.5：in-flight reconnect task —— `scheduleReconnect()` 写入；`disconnect()` /
    /// `prepareForReconnect()` / terminal close / 成功 reconnect 后清 nil.
    /// **关键**：`disconnect()` / `prepareForReconnect()` 必须 cancel 此 task，否则 caller 主动断开后
    /// client 内部仍在 reconnect 循环里 attempt connect，违反"用户主动 disconnect 终止所有自动行为"语义.
    private var reconnectTask: Task<Void, Never>?

    /// Story 12.5：reconnect 退避序列（秒）—— 第 N 次重连等 backoffSequence[min(N-1, count-1)] 秒.
    /// 钦定 [1, 2, 4, 8, 30]：max 30s（节点 4 阶段 UX 角度；30s 后仍未恢复多半真宕机，60s 等待对 UX 太长）.
    /// `internal` 实例字段（非 static）让单测可注入短序列（如 [0.001, ...]）跑 fake-clock 路径,
    /// 避免单测真实跑 1s/2s/4s/8s 退避（违反 ADR-0002 §3.1「unit test 必须秒级完成」）.
    internal var backoffSequence: [TimeInterval] = [1, 2, 4, 8, 30]

    /// Story 12.5：最大 reconnect 尝试次数 —— 钦定 5（与 acceptance 行 2154 一致）.
    /// 5 次失败后切 `.disconnected` + finish stream + log error.
    /// `internal` 实例字段让单测可注入更小值（如 3）覆盖"超过上限"路径而不必跑 5 次.
    internal var maxReconnectAttempts: Int = 5

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

    /// caller 主动拨号入口（Story 12.7 UseCase / Story 12.5 reconnect 状态机的"首次"语义）.
    ///
    /// **Story 12.5 改动**：
    ///   - 重置 `reconnectAttempt = 0`（caller 主动 connect → 任何旧的 reconnect 累计 attempt 失效）
    ///   - cancel in-flight reconnect task（防御性：caller 重新 connect 等价于"放弃当前 reconnect 循环"）
    ///   - 成功后写入 `currentRoomId` 让 reconnect 状态机后续可用同一 roomId 重连
    ///   - **`.connectionStateChanged(.connected)` 由 receive loop 在第一帧到达时同步 emit**
    ///     （早于第一帧 yield，避免与第一帧 yield 之间的调度 race；详见 startReceiveLoop 注释）
    public func connect(roomId: String) async throws {
        // Story 12.5：caller 主动 connect → 清掉任何 in-flight reconnect 状态
        lock.lock()
        let oldReconnectTask = reconnectTask
        reconnectTask = nil
        reconnectAttempt = 0
        lock.unlock()
        oldReconnectTask?.cancel()

        try await connectInternal(roomId: roomId, isReconnect: false, attemptNumber: 0)
        // .connected 由 receive loop 在 firstFrameReceived 时已 emit；caller 不需重复 emit.
    }

    /// Story 12.5 内部 connect 主干：被 public `connect(roomId:)` 与 reconnect 状态机的 `attemptReconnect`
    /// 共同复用. `isReconnect` / `attemptNumber` 仅用于 log（不影响主干行为）.
    ///
    /// **关键约束**（与契约 5 一致）：
    ///   - 本路径**不**重建 `currentStream` / `currentContinuation`；reconnect 内部在同一外部 stream 上透明续接
    ///     （vm 持有的 AsyncStream 不被 finish；handshake 成功后 server 自动重发 room.snapshot 顺势 yield）.
    ///   - prepareForReconnect 才是"finish 旧 stream + 新建 stream"的接缝（caller-driven 全 reset 路径）.
    private func connectInternal(roomId: String, isReconnect: Bool, attemptNumber: Int) async throws {
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
        // Story 12.5：记录 currentRoomId 让 reconnect 路径可复用
        currentRoomId = roomId
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
            // Story 12.5：isReconnectAttempt 让 receive loop pre-handshake 失败时不主动 finish stream
            startReceiveLoop(task: task, continuation: continuation, isReconnectAttempt: isReconnect)
        }
        os_log(.debug,
               log: WebSocketClientImpl.logger,
               "connect resolved for roomId=%{public}@ (isReconnect=%{public}@, attempt=%{public}d)",
               roomId, isReconnect ? "true" : "false", attemptNumber)
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
        let oldReconnectTask = reconnectTask  // Story 12.5：cancel in-flight reconnect 防 disconnect 后仍 attempt
        let oldContinuation = currentContinuation
        underlyingTask = nil
        receiveTask = nil
        reconnectTask = nil           // Story 12.5
        currentRoomId = nil           // Story 12.5：清掉 reconnect 复用源
        reconnectAttempt = 0          // Story 12.5：重置计数
        lock.unlock()

        // client-initiated close（V1 §12.1 close code 1000 normalClosure）
        oldTask?.cancel(with: .normalClosure, reason: nil)
        oldReceiveTask?.cancel()
        oldReconnectTask?.cancel()  // Story 12.5：关键 —— 防 caller disconnect 后 client 仍在 reconnect 循环
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
        let oldReconnectTask = reconnectTask  // Story 12.5：caller-driven reset 也终止任何 in-flight reconnect
        underlyingTask = nil
        receiveTask = nil
        reconnectTask = nil           // Story 12.5
        currentRoomId = nil           // Story 12.5：caller 通常马上调 connect(roomId:) 重新建立
        reconnectAttempt = 0          // Story 12.5
        self.currentStream = stream
        self.currentContinuation = cont
        lock.unlock()

        // 与 WebSocketClientMock.prepareForReconnect 同语义：cancel 旧资源 + 新建 fresh stream
        oldTask?.cancel(with: .normalClosure, reason: nil)
        oldReceiveTask?.cancel()
        oldReconnectTask?.cancel()  // Story 12.5
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
    /// 异常 / close → 走 Story 12.5 reconnect 状态机分类决策（terminal vs transient）.
    ///
    /// fix-review round 1 P1（Story 12.2）：在第一帧成功 / 第一次 error 时 resolve `connectGate`,
    /// 让 connect() 真实拿到握手结果（成功 → return；失败 → throw WSError.connectionFailed）.
    ///
    /// **Story 12.5 改动**：
    ///   - catch error 路径上从 `task.closeCode` 提取 close code 分类决策（V1 §12.1 行 1710-1732）
    ///   - terminal 类（4001-4007 + 1000 + unknown 4xxx）→ emit `.disconnected` + finish stream
    ///   - transient 类（1001 / 1006(.invalid) / 1011 / 4005）→ schedule reconnect + **不**finish stream
    ///     （契约 5：reconnect 在同一外部 stream 上透明续接）
    ///   - **不**用错误描述字符串做分类（脆弱，依赖 OS 国际化文案）—— 仅用 `closeCode.rawValue`
    ///
    /// `isReconnectAttempt`：true → 此 receive 任务由 reconnect 状态机的 `attemptReconnect` 启动.
    ///   pre-handshake 失败时 caller (attemptReconnect) 控制 stream finish/不 finish；defer 不主动 finish.
    private func startReceiveLoop(
        task: WebSocketTaskHandle,
        continuation: AsyncStream<WSMessage>.Continuation,
        isReconnectAttempt: Bool
    ) {
        let newReceiveTask = Task { [weak self] in
            var firstFrameReceived = false
            // Story 12.5：缺省下 receive loop 退出（cancel / pre-handshake error / unknown error）会 finish stream;
            // 但若进入 reconnect 路径（transient close + scheduleReconnect 完成），需要保留 stream（契约 5）.
            // 同时 reconnect-attempt 路径下的 pre-handshake 失败也保留 stream，由 attemptReconnect 决定后续.
            var leaveStreamOpen = isReconnectAttempt
            // fix-review round 1 P1：loop 通过 cancellation 退出（while 条件假）时也必须 finish 继承 continuation,
            // 否则 prepareForReconnect / disconnect 后旧 stream 永远 hang 在 for-await 上.
            // defer 兜底兼容所有退出路径（cancel / catch / return）.
            defer {
                if !leaveStreamOpen {
                    continuation.finish()
                }
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
                        // Story 12.5：先 emit `.connected` 进 stream，再 yield 第一帧 —— 保证流上的顺序为
                        // [.connected, room.snapshot]，避免 emit 与 yield 间的 caller-scheduler race.
                        // **不**通过 connect() 路径（caller 唤醒后 emit）—— 那条路径下 caller 唤醒由 scheduler
                        // 决定，可能晚于本 receive task yield 第一帧（snapshot），导致 vm 先收到 snapshot 再收到 .connected.
                        self?.emitConnectionState(.connected)
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
                    // 拨号 / 连接期异常 → Story 12.5 reconnect 状态机决策
                    os_log(
                        .error,
                        log: WebSocketClientImpl.logger,
                        "receive failed: %{public}@",
                        String(describing: error)
                    )
                    guard let strongSelf = self else { return }

                    // 清掉 underlying task（仅当还是当前 task）
                    strongSelf.lock.lock()
                    if strongSelf.underlyingTask === task {
                        strongSelf.underlyingTask = nil
                    }
                    strongSelf.lock.unlock()

                    if !firstFrameReceived {
                        // pre-handshake 失败：让 connectGate throw（defer 兜底）.
                        // 若 isReconnectAttempt = true → leaveStreamOpen 已是 true，stream 不被 finish；
                        //   `attemptReconnect` 在 connectInternal throw 后决定 schedule next reconnect 或 terminal.
                        // 若 isReconnectAttempt = false（首次 connect）→ leaveStreamOpen = false，stream finish；caller's connect() throws.
                        //
                        // **fix-review round 1 P1（Story 12.5）**：reconnect 路径的 pre-handshake 失败必须把
                        // close code 透传给 attemptReconnect，让它按 V1 §12.1 close code 分类决定 retry vs terminal.
                        // 旧实装 defer 兜底走 `connectionFailed(underlyingDescription:)`，attemptReconnect catch
                        // 拿不到 close code → 无条件 retry；4001 token 过期等 terminal close 也会被白白 retry 5 次,
                        // 永远不触发 caller 的 re-auth / room-error 处理路径.
                        // 修：reconnect 路径 + close code 不为 0（receive 抛错时 runtime 已设置）→ 优先抛
                        // `closedByServer(code:reason:)`；attemptReconnect 用此 code 走 classify 分诊.
                        // 0 (`.invalid` rawValue) 仅用于"无 close code"占位（拨号期 DNS/TLS error / 任务取消等）→
                        // 仍走 connectionFailed 路径，attemptReconnect 视为 transient retry.
                        if isReconnectAttempt {
                            let closeCode = task.closeCode
                            let raw = closeCode.rawValue
                            if raw != 0 {
                                strongSelf.resolveConnectGate(
                                    success: false,
                                    error: WSError.closedByServer(
                                        code: raw,
                                        reason: "pre-handshake close during reconnect (rawCode=\(raw))"
                                    )
                                )
                            }
                            // raw == 0：让 defer 兜底走 connectionFailed（attemptReconnect 视为 transient）
                        }
                        return
                    }

                    // post-handshake 错误：按 close code 分类决策
                    let closeCode = task.closeCode
                    let category = strongSelf.classifyCloseCode(closeCode)
                    switch category {
                    case .terminal(let code):
                        os_log(.info,
                               log: WebSocketClientImpl.logger,
                               "post-handshake terminal close (code=%{public}d) → emit disconnected + finish stream",
                               code)
                        // emit .disconnected 进 stream → vm 写 wsState = .disconnected
                        strongSelf.emitConnectionState(.disconnected)
                        // 同步清掉 reconnect 状态（terminal 不再重试）
                        strongSelf.lock.lock()
                        strongSelf.currentRoomId = nil
                        strongSelf.reconnectAttempt = 0
                        strongSelf.lock.unlock()
                        // leaveStreamOpen 保持初始值（false → finish；true 但 terminal → 强制 finish）
                        leaveStreamOpen = false
                        return
                    case .transient(let code):
                        os_log(.info,
                               log: WebSocketClientImpl.logger,
                               "post-handshake transient close (code=%{public}d) → schedule reconnect",
                               code)
                        // 关键：schedule reconnect + **不**finish stream（契约 5）
                        strongSelf.scheduleReconnect()
                        leaveStreamOpen = true
                        return
                    }
                }
            }
        }
        lock.lock()
        receiveTask = newReceiveTask
        lock.unlock()
    }

    // MARK: - Story 12.5 reconnect 状态机内部 helpers

    /// V1 §12.1 close code 分类（行 1710-1732）.
    /// terminal = {1000, 4001, 4002, 4003, 4004, 4006, 4007, 未知 close code（保守 terminal）}
    /// transient = {1001, 1006(.invalid), 1011, 4005}
    /// 设计决策：未知 close code 保守 terminal —— 防野生 server bug 推未知 code 导致死循环重连.
    private enum CloseCodeCategory: Equatable {
        case terminal(code: Int)
        case transient(code: Int)
    }

    private func classifyCloseCode(_ closeCode: URLSessionWebSocketTask.CloseCode) -> CloseCodeCategory {
        let raw = closeCode.rawValue
        switch raw {
        case 1000:
            return .terminal(code: 1000)
        case 1001, 1011:
            return .transient(code: raw)
        case 4001, 4002, 4003, 4004, 4006, 4007:
            return .terminal(code: raw)
        case 4005:
            return .transient(code: 4005)
        case 0:
            // `.invalid` rawValue=0 ↔ 1006（V1 §12.1 行 1729 钦定 1006 仅由 client 在底层 TCP 异常断开时本地合成）
            return .transient(code: 1006)
        default:
            // 未知 close code 保守 terminal（防野生 server bug 死循环；与 dev decisions 钦定一致）
            return .terminal(code: raw)
        }
    }

    /// 内部接口：emit `.connectionStateChanged(...)` 进 currentContinuation.
    /// reconnect 状态机 + public connect 的成功路径都用此接缝.
    /// vm 通过 `messages` stream 透明感知（与契约 4 一致）.
    private func emitConnectionState(_ state: WSConnectionState) {
        lock.lock()
        let cont = currentContinuation
        lock.unlock()
        cont.yield(.connectionStateChanged(state))
    }

    /// schedule 下一次 reconnect attempt（受 maxReconnectAttempts 上限保护）.
    /// 上限超出 → emit `.disconnected` + finish stream.
    /// 否则：emit `.reconnecting(attempt: N)` + 起 task sleep `backoffSequence[N-1]` 秒 → 调 attemptReconnect.
    private func scheduleReconnect() {
        // 上限校验（reconnectAttempt 是"上一次成功失败到的次数"；本次将尝试 attempt = +1）
        let nextAttempt: Int
        let backoffSec: TimeInterval
        let exceeded: Bool
        lock.lock()
        nextAttempt = reconnectAttempt + 1
        if nextAttempt > maxReconnectAttempts {
            exceeded = true
            backoffSec = 0
        } else {
            exceeded = false
            // 取 backoffSequence[min(N-1, count-1)] —— 序列耗尽后用最后一个值
            let idx = min(nextAttempt - 1, backoffSequence.count - 1)
            backoffSec = backoffSequence[idx]
        }
        lock.unlock()

        if exceeded {
            os_log(.error,
                   log: WebSocketClientImpl.logger,
                   "reconnect exceeded maxAttempts (%{public}d) → emit disconnected + finish stream",
                   maxReconnectAttempts)
            // 终态：emit + finish stream + 清状态
            emitConnectionState(.disconnected)
            lock.lock()
            let cont = currentContinuation
            currentRoomId = nil
            reconnectAttempt = 0
            reconnectTask = nil
            lock.unlock()
            cont.finish()
            return
        }

        // emit reconnecting(attempt: N) → vm 写 wsState = .reconnecting
        emitConnectionState(.reconnecting(attempt: nextAttempt))

        let task = Task { [weak self] in
            // 用 Task.sleep（cancellation-aware）—— `disconnect()` / `prepareForReconnect()` cancel reconnectTask 后立即退出.
            try? await Task.sleep(nanoseconds: UInt64(backoffSec * 1_000_000_000))
            if Task.isCancelled { return }
            guard let self else { return }
            await self.attemptReconnect(attempt: nextAttempt)
        }
        lock.lock()
        reconnectTask = task
        lock.unlock()
    }

    /// 真正执行第 N 次 reconnect attempt：调 connectInternal（复用 currentRoomId）.
    /// 成功 → 重置 reconnectAttempt = 0 + emit `.connected`.
    /// 失败 → 累计 attempt + scheduleReconnect 再来一次（直到上限切 terminal）.
    private func attemptReconnect(attempt: Int) async {
        let roomId: String?
        lock.lock()
        roomId = currentRoomId
        lock.unlock()
        guard let roomId = roomId else {
            // 极少数 race：disconnect 已清空 currentRoomId 但 reconnectTask 还没被 cancel 完
            os_log(.debug,
                   log: WebSocketClientImpl.logger,
                   "attemptReconnect aborted: currentRoomId is nil (likely disconnect race)")
            return
        }
        do {
            try await connectInternal(roomId: roomId, isReconnect: true, attemptNumber: attempt)
            // 成功：重置计数；`.connected` 已由 receive loop 在 firstFrameReceived 时 emit（避免与 first frame yield 调度 race）.
            // server 自动重发 room.snapshot 作为第一帧（V1 §12.1.3）→ vm 接 snapshot 对齐 roster.
            lock.lock()
            reconnectAttempt = 0
            reconnectTask = nil
            lock.unlock()
            os_log(.info,
                   log: WebSocketClientImpl.logger,
                   "reconnect attempt %{public}d succeeded",
                   attempt)
        } catch {
            // 失败：先按 V1 §12.1 close code 分类决定 retry vs terminal（fix-review round 2 P1）.
            //
            // **核心**：reconnect 期间的 pre-handshake 失败（first frame 之前 server 直接 close）必须按
            // close code 分诊，不能无条件 retry：
            //   - `closedByServer(code: 4001/4003/4004/...)` 这类 terminal close → 立即 emit `.disconnected` +
            //     finish stream + 清状态，不再 schedule（白白 retry 5 次掩盖 caller 的 re-auth 触发点）
            //   - `closedByServer(code: 4005/1006/1011/1001)` 这类 transient close → 继续 scheduleReconnect
            //   - 非 closedByServer error（DNS / TLS / connection refused / connectionFailed 等）→
            //     视为 transient（拨号期短瞬故障）继续 scheduleReconnect
            //
            // 旧实装 catch 不分类、无条件 schedule → 4001 token 过期会消耗 5 次 backoff attempts 才切 terminal,
            // caller 的 re-auth handling path 被严重延迟（V1 §12.1 close code 表 4001 钦定 "重新登录"，
            // 不应靠 5 次 retry 触发）.
            os_log(.error,
                   log: WebSocketClientImpl.logger,
                   "reconnect attempt %{public}d failed: %{public}@",
                   attempt,
                   String(describing: error))

            // 提取 close code（仅 closedByServer error 携带；其它 error 视 .invalid → transient）
            let category: CloseCodeCategory
            if case let WSError.closedByServer(code, _) = error {
                // 反向构造 URLSessionWebSocketTask.CloseCode 走 classifier（保持 raw → category 的单一映射源）
                let cc = URLSessionWebSocketTask.CloseCode(rawValue: code) ?? .invalid
                category = classifyCloseCode(cc)
            } else {
                // 拨号期非 close 类异常（DNS / TLS / refused）—— 视为 transient，由 scheduleReconnect
                // 继续走 backoff retry 直到上限切 terminal
                category = .transient(code: 0)
            }

            switch category {
            case .terminal(let code):
                os_log(.info,
                       log: WebSocketClientImpl.logger,
                       "reconnect terminal close (code=%{public}d) → stop retrying + emit disconnected",
                       code)
                // emit `.disconnected` 让 vm 写 wsState = .disconnected → caller 触发 re-auth / room-error 处理
                emitConnectionState(.disconnected)
                // 清状态 + finish stream（终态）
                lock.lock()
                let cont = currentContinuation
                currentRoomId = nil
                reconnectAttempt = 0
                reconnectTask = nil
                lock.unlock()
                cont.finish()
            case .transient:
                lock.lock()
                reconnectAttempt = attempt
                reconnectTask = nil  // 当前 task 已结束；scheduleReconnect 会写新的
                lock.unlock()
                scheduleReconnect()
            }
        }
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
