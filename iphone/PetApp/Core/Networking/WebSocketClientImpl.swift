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

    /// fix-review round 3 P1（Story 12.5）：与 `connectGate` 配对的 owner session generation.
    /// install gate 时（`connectInternal` 内）记录当时的 `sessionGeneration`；
    /// 任何 generation-scoped resolve 路径（receive-loop defer / catch path）必须用自己的 `mySession` 与
    /// 此值比对：不匹配 silent drop（防 stale receive-task defer 跑去 resolve 新 session 的 gate
    /// 让 fresh `connect(roomId:)` 拿到 stale failure）.
    /// disconnect / prepareForReconnect / deinit 走"unconditional resolve" 路径（这三类 caller 显式
    /// 想要 fail 当前 in-flight connect()，不需要 generation 校验）.
    private var connectGateOwnerSession: Int?

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

    /// fix-review round 2 P1（Story 12.5）：session generation counter —— 用于把"过期 session 的 stale task"
    /// 与"当前活跃 session 的 task"在共享状态写入处隔离.
    ///
    /// **递增点**：每次 `connect(roomId:)` / `prepareForReconnect()` / `disconnect()` 都 +1.
    /// **捕获点**：所有 launched async task（receive-loop / reconnect attempt / scheduleReconnect 闭包）在 launch
    ///   时把当时的 `sessionGeneration` 抓进 local `mySession` 常量.
    /// **校验点**：任何写 `currentContinuation` / `reconnectTask` / 调 `emitConnectionState` /
    ///   `scheduleReconnect` 之前先校验 `mySession == sessionGeneration`，不匹配 silent return（log debug）.
    ///
    /// **为什么需要**：
    ///   - 旧 receive-loop 的 catch path 可能在 `prepareForReconnect()` 已 swap 新 stream **之后**才跑 ——
    ///     旧 task 通过新 `currentContinuation` emit `.disconnected` / `scheduleReconnect`，导致
    ///     room A 的 late close 在 room B 的 stream 上显示 `.disconnected`/`.reconnecting`，
    ///     甚至触发错误 session 的 reconnect logic.
    ///   - `disconnect()` / `prepareForReconnect()` cancel 一个已经在 `connectInternal` 内的 reconnect attempt
    ///     时，cancelled task 仍然落到 catch block；旧实装无 generation check，stale catch 安装新 reconnectTask
    ///     → 如果 fresh `connect(roomId:)` 在 stale catch 跑之前发生，delayed retry 会 race 新 connection 连错房间.
    ///
    /// 与 12.4 r1 的 `streamRoomId` 守护同精神：用 generation counter 保证只有"当前活跃 session" 的 task
    /// 能 mutate 共享状态.
    private var sessionGeneration: Int = 0

    /// fix-review round 4 P2（Story 12.5）：stream generation counter —— 与 sessionGeneration 解耦.
    /// **递增点**：仅在 `makeStream()` 调用处（init / `prepareForReconnect()`）+1.
    /// **不**在 `connect(roomId:)` 翻新 —— connect 在已 connected client 上调用时复用现存 stream / continuation.
    /// **捕获点**：receive-loop launch 时把当时的 `streamGeneration` 抓进 local `myStreamGen`.
    /// **校验点**：`yieldIfCurrent` / `finishStreamIfCurrent` 第一层 gate 用此字段判断 "stream 是否还是
    /// receive-loop launch 时那个" —— 不一致 → 孤儿 continuation，yield silent drop / finish 仍走（让旧
    /// consumer for-await 退出）.
    /// **为什么不复用 sessionGeneration**：sessionGeneration 在每次 connect/prepareForReconnect/disconnect 都翻；
    /// 但 connect-replace 路径（review 关切）下 stream **没换**，仍是同一个 currentContinuation/currentStream；
    /// 单字段无法区分"stream 已 swap"vs"session 翻新但 stream 复用". 双字段精确刻画两个不同语义.
    ///
    /// **fix-review round 2 P2（Story 15.2）**：该字段同时被 `WebSocketClient.streamGeneration` protocol getter
    /// 暴露给 ViewModel —— RealRoomViewModel 在 consumer task 启动时 snapshot 当时的 generation，handle
    /// message 时校验未变（变了 = stream 已被 swap = 当前 task 是旧 stream 的 stale consumer，丢弃所有事件）.
    /// 同房间 leave-rejoin（A→A）/ same-room reconnect 路径下，旧 / 新两条 stream 的 `streamRoomId` 都是同
    /// 一个 room，仅靠 roomId guard 无法区分；引入 streamGeneration 后能精确区分.
    private var streamGenerationStorage: Int = 0

    // MARK: - Story 12.6 heartbeat 字段

    /// Story 12.6：当前 in-flight heartbeat task（connect 成功 firstFrame 后启动；disconnect /
    /// prepareForReconnect / 终态切 disconnected 时 cancel + 清 nil）.
    /// **关键**：与 reconnectTask / receiveTask 同精神 —— 任何"终止活跃 session"路径必须 cancel
    /// 此 task，否则用户主动断开后 client 仍在循环 send(.ping) → send 抛 notConnected → noise log,
    /// 最坏情况 task 泄漏.
    private var heartbeatTask: Task<Void, Never>?

    /// Story 12.6：心跳间隔（秒）—— V1 §12.2 行 1807 钦定 30s 默认值.
    /// `internal` 实例字段（非 static）让单测可注入短值（如 0.05s）跑 ms 级 fake-clock 路径,
    /// 避免单测真实跑 30s（违反 ADR-0002 §3.1「unit test 必须秒级完成」）.
    internal var heartbeatInterval: TimeInterval = 30.0

    /// Story 12.6：pong 超时（秒）—— epic line 2176 钦定 5s.
    /// 5s 未收到 pong → 视连接失效 → cancel underlying task with .goingAway（1001 transient）→
    /// receive-loop catch → 12.5 状态机自动重连.
    /// `internal` 实例字段同上 —— 单测可注入短值跑 fake-clock.
    internal var pongTimeout: TimeInterval = 5.0

    /// Story 12.6：心跳序号（每发一次 ping 自增；ping requestId = "ping_<seq>"）.
    /// 单调递增 + 进程内复用即可（V1 §12.2 行 1784 钦定 requestId 仅用于 session 内 request-response 配对，
    /// 不进 server 持久化日志）.
    /// `internal` 可见让单测可读断言序列（"第 N 次 ping requestId == 'ping_N'"）.
    internal var heartbeatSeq: Int = 0

    /// Story 12.6：当前 in-flight ping 的 pong-arrived latch.
    /// 每发一次 ping 在 lock 内创建新 AsyncStream<Void>.makeStream() 替换；receive-loop 收到
    /// `.pong(...)` 调 `notifyPongReceivedIfCurrent(mySession:)` → yield + finish；
    /// heartbeat task 用 `withTaskGroup` 并发等"latch yield"或"Task.sleep(pongTimeout)"先到者赢.
    /// finish 后 nil；下一轮 ping 重新创建.
    /// `private`（非 internal）—— 仅 client 内部 receive-loop / heartbeat task 写读，单测不直接访问.
    private var pendingPongContinuation: AsyncStream<Void>.Continuation?

    /// Story 12.6 fix-review round 2 P2：当前 in-flight ping 的 requestId（"ping_<seq>"）.
    /// V1 §12.2 钦定 `pong.requestId` 必须 echo 对应 ping 的 requestId.
    /// receive-loop 收到 `.pong(requestId:)` 必须先校验 == pendingPongRequestId 才 yield latch；
    /// 不匹配 = stale / duplicated pong（旧 ping 的迟到 pong），silent drop + log debug.
    /// 旧实装无条件接受任何 .pong 当 ack → 旧 pong 错误 ack 当前 in-flight ping → miss 当前 ping
    /// 实际未被 ack → 推迟一整个 heartbeat interval 才检测到 reconnect 需要.
    /// 生命周期：与 pendingPongContinuation 严格同步；同时由 lock 保护一起读写.
    private var pendingPongRequestId: String?

    /// Story 12.6 fix-review round 4 P1：单测注入点 —— 模拟"heartbeat task 已在 lock 内 install
    /// pendingPongContinuation + 取出 underlyingTask snapshot 但**还没 send ping**" 这个 race window.
    ///
    /// 触发时序：heartbeat task 在持锁块内已完成 latch install + 取出 activeTask snapshot →
    /// `lock.unlock()` 之后立即 await 此 hook → hook 内可主动模拟 reconnect/caller-driven connect()
    /// 路径（cancelHeartbeatStateForReconnectIfCurrent + install 新 underlyingTask）→ hook 返回后
    /// heartbeat task 进入 final pre-send 校验路径 → 校验失败 silent skip（不 send 到新 socket）.
    ///
    /// production 永远 nil（无 perf 影响 —— async-let nil 等同空 await）；仅单测在 setup 阶段注入.
    /// `@Sendable` + `async` 让 hook 可执行真实异步操作（如 cancel old + install new task）.
    /// 与既有 `heartbeatInterval` / `pongTimeout` / `heartbeatSeq` 同 internal 模式（fake-clock 风格）.
    internal var beforeHeartbeatSendHook: (@Sendable () async -> Void)?

    /// Story 12.6 fix-review round 5 P1：单测专用 swap helper —— 模拟 receive-loop 触发的
    /// **transient reconnect 透明续接**：调 `cancelHeartbeatStateForReconnectIfCurrent` +
    /// install 新 `underlyingTask`，但 **不**翻 `sessionGeneration`（与 production reconnect 路径同语义）.
    ///
    /// production 不调用；仅 round 5 P1 测试用，模拟以下 race：
    ///   T0  heartbeat send（在 socketA 上）suspended
    ///   T1  socketA 底层断 → receive-loop catch → cancelHeartbeatStateForReconnectIfCurrent →
    ///       attemptReconnect → connectInternal → install 新 underlyingTask = socketB（sessionGen 不变）
    ///   T2  T0 那个 send 抛错 → catch 跑修复后逻辑 → task identity 校验失败 → silent skip cancel
    ///
    /// 必须**先于** send 抛错完成，让 catch 跑时 underlyingTask 已 swap.
    internal func _simulateTransparentReconnectSwapForTest(newTask: WebSocketTaskHandle) {
        // 1. 翻 streamGeneration / heartbeat 子系统 reset（与 receive-loop transient catch 同路径）
        let mySession: Int
        lock.lock()
        mySession = sessionGeneration
        lock.unlock()
        cancelHeartbeatStateForReconnectIfCurrent(mySession: mySession)
        // 2. swap underlyingTask（不翻 gen，模拟 reconnect 透明续接）
        lock.lock()
        // 旧 task 由 receive-loop catch 路径自然 cancel；这里只 swap underlyingTask 字段.
        underlyingTask = newTask
        lock.unlock()
    }

    // MARK: - WebSocketClient protocol

    public var messages: AsyncStream<WSMessage> {
        lock.lock()
        defer { lock.unlock() }
        return currentStream
    }

    /// fix-review round 2 P2（Story 15.2）：暴露内部 `streamGenerationStorage` 给 protocol 层 caller
    /// （ViewModel 守护用）.
    ///
    /// **lock 必要性**：`streamGenerationStorage` 字段写发生在 `makeStream()`（init / `prepareForReconnect()`）
    /// 内部，受 `lock` 保护；公共 getter 必须同样持锁读，否则 caller 在 `prepareForReconnect()` 进行中
    /// 读到撕裂值的风险（Int 写在 64-bit 平台是原子的，但语义上要保证 read 看到的 generation 与 currentStream
    /// 配对，与 `messages` getter 同模式）.
    ///
    /// **caller 端典型用法**：consumer task 启动时 `let myStreamGen = client.streamGeneration` snapshot；
    /// handle message 时校验 `myStreamGen == client.streamGeneration` —— 不一致 = stream 已被 swap.
    public var streamGeneration: Int {
        lock.lock()
        defer { lock.unlock() }
        return streamGenerationStorage
    }

    /// fix-review round 4 P1（Story 15.2）：原子读 stream + generation 快照.
    ///
    /// **必要性**：consumer 启动若分两步读（`streamGeneration` 然后 `messages`），
    /// `prepareForReconnect()` 在两次锁之间发生 → 新 task 订阅新 stream 但携带旧 generation
    /// → handle 把新 stream 所有消息当 stale 全丢，房间更新卡死.
    ///
    /// **实装**：同一个 `lock.lock()` / `lock.unlock()` 临界区内同时读 `currentStream` +
    /// `streamGenerationStorage`，让两者作为不可分割的快照对返回. `prepareForReconnect()` 也持同一
    /// `lock` 写两个字段（见行 535-557），与本读路径互斥.
    public var currentStreamSnapshot: (stream: AsyncStream<WSMessage>, generation: Int) {
        lock.lock()
        defer { lock.unlock() }
        return (currentStream, streamGenerationStorage)
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
        // fix-review round 5 P1：preconditions 必须**先于** sessionGeneration 翻新.
        //
        // 旧实装在入口直接 `sessionGeneration += 1`，但若紧随的 `connectInternal` 因 token nil/空
        // 或 `makeWSURL` throw 而早退（room switch 时 auth 暂时不可用是典型场景），现存活的 socket
        // 仍在 receive，但其 receive-loop 持有的 `mySession` 已 < sessionGeneration → 后续 frame 走
        // `yieldIfCurrent` / `emitConnectionStateIfCurrent` 全部 silent drop；此时连接物理上还活，
        // 逻辑上已被 wedged，必须等外部显式 disconnect 才能恢复 —— **用户不可见**的 wedge.
        //
        // 修：preconditions 先 dry-run，throw 路径完全不动 generation / 不动 reconnect 状态.
        //   - token 取（nil/空 → throw + 不翻 gen）
        //   - URL 构造（throw → 不翻 gen）
        //   - 都过了才进入 displace-session 持锁块：cancel 老 reconnectTask + 翻 gen + 重置 attempt
        // 注：`connectInternal` 内仍会重做这两步检查（reconnect 路径直接走那条），代码重复 OK.
        guard let token = tokenProvider(), !token.isEmpty else {
            throw WSError.tokenMissing
        }
        _ = try makeWSURL(roomId: roomId, token: token)  // dry-run；throw 路径不翻 gen

        // preconditions 都过 → 真正 displace 当前 session.
        // Story 12.5：caller 主动 connect → 清掉任何 in-flight reconnect 状态
        // fix-review round 2 P1：递增 sessionGeneration —— 之前 launch 的 receive-loop / reconnect-attempt
        //   再写共享状态时会因 mySession != sessionGeneration 被 silent drop（防 stale event 串到新 session）.
        // Story 12.6：caller 主动 connect 等价于"放弃当前 heartbeat 循环" —— cancel heartbeatTask + finish pong latch
        lock.lock()
        let oldReconnectTask = reconnectTask
        let oldHeartbeatTask = heartbeatTask  // Story 12.6
        let oldPongCont = pendingPongContinuation  // Story 12.6
        reconnectTask = nil
        heartbeatTask = nil           // Story 12.6
        reconnectAttempt = 0
        pendingPongContinuation = nil // Story 12.6
        pendingPongRequestId = nil    // Story 12.6 fix-review round 2 P2：与 pendingPongContinuation 同步清
        sessionGeneration += 1
        lock.unlock()
        oldReconnectTask?.cancel()
        oldHeartbeatTask?.cancel()    // Story 12.6
        oldPongCont?.finish()         // Story 12.6

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
        let mySession: Int  // fix-review round 2 P1：捕获当前 generation 传给 receive-loop
        let myStreamGen: Int  // fix-review round 4 P2：捕获当前 stream generation 传给 receive-loop
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
        mySession = sessionGeneration
        myStreamGen = streamGenerationStorage
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
            // fix-review round 5 P2：install 新 gate 前必须先 resolve 旧 gate ——
            //   场景：caller-driven `connect()` 与已在 `connectInternal` 内 await 的 reconnect-attempt
            //   竞争时，旧实装直接覆盖 `connectGate`，被覆盖的 reconnect 后续走 `resolveConnectGate(...)`
            //   因 session 不匹配 silent drop → 那个 `withCheckedThrowingContinuation` 永远不被 resume,
            //   reconnect 的 connectInternal await 永久 suspend → task 泄漏.
            //
            //   策略：抢锁内一次性 swap 旧 gate 引用出来（清字段），出锁后在 install 新 gate 之前
            //   resume(throwing:) 让旧 caller 拿到明确失败而非 hang. 走 unconditional 路径——本场景
            //   显式想 fail 旧 gate（caller 主动 supersede），不需要 generation 校验.
            let staleGate: CheckedContinuation<Void, Error>?
            lock.lock()
            staleGate = connectGate
            connectGate = nil
            connectGateOwnerSession = nil
            lock.unlock()
            staleGate?.resume(throwing: WSError.connectionFailed(
                underlyingDescription: "superseded by new connect attempt"
            ))

            lock.lock()
            connectGate = cont
            // fix-review round 3 P1：把 gate 与本次 connectInternal 的 mySession 绑定 ——
            //   stale receive-task defer / catch 路径只能 resolve "自己 install 的 gate"；
            //   新 session 的 connect 已 install 新 gate（new owner session）→ stale resolve 入口的
            //   `mySession != connectGateOwnerSession` → silent drop，不污染新 connect.
            connectGateOwnerSession = mySession
            lock.unlock()
            // 启动 receive 长任务（持 task / continuation 两个 reference 进闭包）
            // Story 12.5：isReconnectAttempt 让 receive loop pre-handshake 失败时不主动 finish stream
            // fix-review round 2 P1：mySession 让 receive loop 在 stale session 时 silent drop 共享状态写入
            startReceiveLoop(
                task: task,
                continuation: continuation,
                isReconnectAttempt: isReconnect,
                mySession: mySession,
                myStreamGen: myStreamGen
            )
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
        let oldHeartbeatTask = heartbeatTask  // Story 12.6：cancel in-flight heartbeat 防 disconnect 后仍发 ping
        let oldContinuation = currentContinuation
        let oldPongCont = pendingPongContinuation  // Story 12.6
        underlyingTask = nil
        receiveTask = nil
        reconnectTask = nil           // Story 12.5
        heartbeatTask = nil           // Story 12.6
        currentRoomId = nil           // Story 12.5：清掉 reconnect 复用源
        reconnectAttempt = 0          // Story 12.5：重置计数
        pendingPongContinuation = nil // Story 12.6
        pendingPongRequestId = nil    // Story 12.6 fix-review round 2 P2
        // fix-review round 2 P1：递增 sessionGeneration —— stale receive-loop / cancelled reconnect-attempt
        //   随后落到 catch path 时 mySession != sessionGeneration → 不再 emit / 不再 schedule.
        sessionGeneration += 1
        lock.unlock()

        // client-initiated close（V1 §12.1 close code 1000 normalClosure）
        oldTask?.cancel(with: .normalClosure, reason: nil)
        oldReceiveTask?.cancel()
        oldReconnectTask?.cancel()  // Story 12.5：关键 —— 防 caller disconnect 后 client 仍在 reconnect 循环
        oldHeartbeatTask?.cancel()  // Story 12.6
        oldPongCont?.finish()       // Story 12.6：让 heartbeat task 的 awaitPongOrTimeout 立即退出
        oldContinuation.finish()
        // fix-review round 1 P1：若 caller 在 connect() await 期间 disconnect → 兜底 resolve gate 防 hang.
        // fix-review round 3 P1：disconnect 已经在持锁内 `sessionGeneration += 1`，但仍要显式 fail 当前
        // in-flight connect()（caller 主动放弃）—— 走 unconditional 路径绕过 generation 校验.
        resolveConnectGateUnconditionally(
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
        let oldHeartbeatTask = heartbeatTask  // Story 12.6
        let oldPongCont = pendingPongContinuation  // Story 12.6
        underlyingTask = nil
        receiveTask = nil
        reconnectTask = nil           // Story 12.5
        heartbeatTask = nil           // Story 12.6
        currentRoomId = nil           // Story 12.5：caller 通常马上调 connect(roomId:) 重新建立
        reconnectAttempt = 0          // Story 12.5
        pendingPongContinuation = nil // Story 12.6
        pendingPongRequestId = nil    // Story 12.6 fix-review round 2 P2
        // fix-review round 2 P1：递增 sessionGeneration —— 旧 receive-loop 的 catch path 即使在 swap 新 stream
        //   **之后**才跑，也会因 mySession != sessionGeneration 被 silent drop；不会污染新 stream / 错误 schedule.
        sessionGeneration += 1
        // fix-review round 4 P2：递增 streamGeneration —— stream 已 swap，旧 receive-loop 的 yield/finish
        //   通过 streamGeneration 校验时识别为孤儿（finish 仍允许让旧 consumer for-await 退出，yield silent drop）.
        streamGenerationStorage += 1
        self.currentStream = stream
        self.currentContinuation = cont
        lock.unlock()

        // 与 WebSocketClientMock.prepareForReconnect 同语义：cancel 旧资源 + 新建 fresh stream
        oldTask?.cancel(with: .normalClosure, reason: nil)
        oldReceiveTask?.cancel()
        oldReconnectTask?.cancel()  // Story 12.5
        oldHeartbeatTask?.cancel()  // Story 12.6
        oldPongCont?.finish()       // Story 12.6
        // fix-review round 1 P1：若 caller 在 connect() await 期间 prepareForReconnect → 兜底 resolve gate 防 hang.
        // fix-review round 3 P1：同 disconnect —— prepareForReconnect 已翻 generation，但显式 fail in-flight
        // connect() 是预期语义 → 走 unconditional 路径.
        resolveConnectGateUnconditionally(
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
    ///
    /// `mySession`：fix-review round 2 P1 —— launch 时 caller（`connectInternal`）捕获当时的
    ///   `sessionGeneration` 传入；任何写共享状态前先 `isCurrentSession(mySession)` 校验，stale session
    ///   silent return.
    private func startReceiveLoop(
        task: WebSocketTaskHandle,
        continuation: AsyncStream<WSMessage>.Continuation,
        isReconnectAttempt: Bool,
        mySession: Int,
        myStreamGen: Int
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
            //
            // fix-review round 4 P2 (#1)：defer 内 `continuation.finish()` 必须 generation-gated.
            // 触发条件：`connect(roomId:)` 在 client 已 connected 时被调用 —— 复用现存 `currentContinuation`
            // 但 `sessionGeneration += 1` 已先翻；旧 receiveTask 被 cancel 后落到本 defer，若无 generation
            // 校验，stale `continuation.finish()` 会终结新 session 复用的 stream → 新连接立即失活.
            // 走 `finishStreamIfCurrent(_:mySession:)` 包装：mySession != sessionGeneration → silent skip.
            defer {
                if !leaveStreamOpen {
                    self?.finishStreamIfCurrent(continuation, mySession: mySession, myStreamGen: myStreamGen)
                }
                if !firstFrameReceived {
                    // cancel 在握手期发生 —— 让 connect() 拿到失败信号.
                    // fix-review round 3 P1：generation-gated resolve —— 旧 receive-task 的 defer 在新 connect
                    // 已 install 新 gate（new owner session）之后才跑时，stale defer 不能 resolve 新 gate.
                    // resolveConnectGate 内部检查 `mySession == connectGateOwnerSession` 不匹配 silent drop.
                    self?.resolveConnectGate(
                        success: false,
                        error: WSError.connectionFailed(underlyingDescription: "receive task cancelled before first frame"),
                        mySession: mySession
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
                        // fix-review round 2 P1：emit 前 generation 校验 —— 旧 session 的 receive-loop 在新
                        // session 已 swap 新 stream 之后才跑，应 silent drop（不污染新 stream）.
                        self?.emitConnectionStateIfCurrent(.connected, mySession: mySession)
                        // fix-review round 3 P1：success resolve 也走 generation-gated 路径 ——
                        // 极端时序下 stale receive-task 的 first frame 可能在新 connect 已 install 新 gate
                        // 之后才到，stale `resume(())` 会让 fresh connect 拿到 spurious success（连错 session）.
                        self?.resolveConnectGate(success: true, error: nil, mySession: mySession)
                        // Story 12.6：firstFrame 同步路径启动 heartbeat task（与 emitConnectionStateIfCurrent 同位置）.
                        // 启动逻辑内部已 cancel 既有 heartbeatTask（防御性）+ 走 sessionGeneration 守护.
                        self?.startHeartbeatTask(mySession: mySession)
                    }
                    // fix-review round 4 P2 (#2)：所有 frame yield 也 generation-gated.
                    // 触发条件：旧 receive-loop 已 dequeue 一条 frame（旧房间的 room.snapshot / member.*）
                    // 但 cancel/replace 同时发生 → 不 generation-gate 的话 stale frame 会被 yield 到现在被
                    // 新 session 复用的 stream 上 → 旧房间 traffic 漏到新连接.
                    // 走 `yieldIfCurrent(_:to:mySession:)` 包装：mySession != sessionGeneration → silent drop.
                    switch frame {
                    case .string(let text):
                        let message = WSMessageCodec.decode(text)
                        // Story 12.6：先 internal pong-notify（清 latch），再 yield 到 stream（vm 仍 break）.
                        // 顺序原因：pong notify 是 client-internal，必须比 vm main-actor hop 先到；否则 5s
                        // pongTimeout 计算受 vm 的 main-actor 调度 jitter 影响 → 可能超时误判.
                        // fix-review round 2 P2：携 requestId 让 notify 路径校验 in-flight ping 配对.
                        if case .pong(let requestId) = message {
                            self?.notifyPongReceivedIfCurrent(requestId: requestId, mySession: mySession)
                        }
                        self?.yieldIfCurrent(message, to: continuation, mySession: mySession, myStreamGen: myStreamGen)
                    case .data(let data):
                        // V1 §12.2 / §12.3：text frame only；binary frame 不应出现 —— 兜底兼容（解为 UTF-8 string 走同路径）
                        if let text = String(data: data, encoding: .utf8) {
                            let message = WSMessageCodec.decode(text)
                            // Story 12.6：与 .string 路径同
                            // fix-review round 2 P2：携 requestId 让 notify 路径校验 in-flight ping 配对.
                            if case .pong(let requestId) = message {
                                self?.notifyPongReceivedIfCurrent(requestId: requestId, mySession: mySession)
                            }
                            self?.yieldIfCurrent(message, to: continuation, mySession: mySession, myStreamGen: myStreamGen)
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
                                // fix-review round 3 P1：generation-gated resolve —— stale receive-task 的
                                // pre-handshake close 即使 raw != 0，也不能 resolve 新 session 的 gate.
                                strongSelf.resolveConnectGate(
                                    success: false,
                                    error: WSError.closedByServer(
                                        code: raw,
                                        reason: "pre-handshake close during reconnect (rawCode=\(raw))"
                                    ),
                                    mySession: mySession
                                )
                            }
                            // raw == 0：让 defer 兜底走 connectionFailed（attemptReconnect 视为 transient）
                        }
                        return
                    }

                    // post-handshake 错误：按 close code 分类决策
                    // fix-review round 2 P1：所有写共享状态 / 调 emit / 调 scheduleReconnect 之前先做 generation
                    //   校验 —— stale session（旧 receive-loop 在新 session swap 之后才跑）silent drop.
                    if !strongSelf.isCurrentSession(mySession) {
                        os_log(.debug,
                               log: WebSocketClientImpl.logger,
                               "stale receive-loop catch dropped (mySession=%{public}d != current)",
                               mySession)
                        // 不更动新 session 的 leaveStreamOpen 决策 —— 保持初始值即可；新 session 的 receive-loop
                        // 自己的 defer 会按需 finish 它自己的 continuation（捕获了 fresh 的 continuation 引用）.
                        return
                    }
                    let closeCode = task.closeCode
                    let category = strongSelf.classifyCloseCode(closeCode)
                    switch category {
                    case .terminal(let code):
                        os_log(.info,
                               log: WebSocketClientImpl.logger,
                               "post-handshake terminal close (code=%{public}d) → emit disconnected + finish stream",
                               code)
                        // fix-review round 6 P2：terminal 分支也必须 cancel heartbeat 子系统 ——
                        // 与 transient 分支对齐. 旧实装只把 client 切到 .disconnected，但 heartbeatTask +
                        // pendingPongContinuation 仍 alive：
                        //   - 旧 heartbeat task 仍在 sleep up to one interval（默认 30s）或 fire timeout path
                        //   - 已 finish 的 stream 之后还产生 post-disconnect ping / timeout activity
                        //   - leak 旧 heartbeat loop 直到 task 自己退出
                        // helper 名虽含 "ForReconnect"（最初为 transient reconnect 设计），但内部逻辑就是
                        // "cancel 旧 heartbeat task + finish pendingPong + 清字段"，对 terminal final
                        // cleanup 路径同样适用 —— 没必要为单 callsite 抽新 helper（minimal-fix 纪律）.
                        // **terminal 分支不会 reconnect**，cleanup 是最终态，不需要重启 heartbeat task.
                        strongSelf.cancelHeartbeatStateForReconnectIfCurrent(mySession: mySession)
                        // emit .disconnected 进 stream → vm 写 wsState = .disconnected
                        strongSelf.emitConnectionStateIfCurrent(.disconnected, mySession: mySession)
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
                        // fix-review round 1 P1：旧 socket close 已确定 → reconnect 前**必须**先 cancel
                        // 旧 heartbeatTask + finish pendingPongContinuation；否则旧 pong 5s timer 会在
                        // 新 underlyingTask install 之后才 fire，调 cancelUnderlyingTaskWithGoingAwayIfCurrent
                        // 时 sessionGeneration 没翻（reconnect 透明续接）→ 错杀新 socket handshake.
                        // 详见 cancelHeartbeatStateForReconnectIfCurrent 文档块.
                        strongSelf.cancelHeartbeatStateForReconnectIfCurrent(mySession: mySession)
                        // 关键：schedule reconnect + **不**finish stream（契约 5）
                        strongSelf.scheduleReconnectIfCurrent(mySession: mySession)
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

    // fix-review round 2 P1：原 `emitConnectionState(_:)` 已删除 —— 它**不**做 generation 校验，
    // 留着会成为 race 复发隐患（任何新 callsite 不小心调它都会绕过 generation gate）.
    // 所有 emit 走 `emitConnectionStateIfCurrent(_:mySession:)`.

    /// fix-review round 2 P1：generation-gated emit —— 仅当 mySession == sessionGeneration 时 emit.
    /// stale session（旧 receive-loop / cancelled reconnect-attempt 在新 session 已 swap 之后才跑）
    /// 通过此接缝 silent drop，不污染新 stream 的状态序列.
    private func emitConnectionStateIfCurrent(_ state: WSConnectionState, mySession: Int) {
        lock.lock()
        guard sessionGeneration == mySession else {
            lock.unlock()
            os_log(.debug,
                   log: WebSocketClientImpl.logger,
                   "emitConnectionState dropped: stale mySession=%{public}d (current=%{public}d)",
                   mySession, sessionGeneration)
            return
        }
        let cont = currentContinuation
        lock.unlock()
        cont.yield(.connectionStateChanged(state))
    }

    /// fix-review round 4 P2：generation-gated yield —— stale receive-loop（旧 session 的）已 dequeue
    /// 的 frame 在 cancel 与 connect-replace race 下，**不能**写到现在被新 session 复用的共享 stream 上.
    ///
    /// **gate 逻辑**（两层）：
    ///   - **第 1 层 stream-owner 校验**：`myStreamGen != streamGeneration` → stream 已被 swap（典型路径
    ///     `prepareForReconnect()`）→ 旧 continuation 是孤儿，silent drop（孤儿 stream 没人读，yield 浪费）.
    ///   - **第 2 层 session 校验**：stream 仍是当前的（`myStreamGen == streamGeneration`）但 `mySession !=
    ///     sessionGeneration` → 表示 stream 被复用（典型路径 `connect(roomId:)` 在 already-connected client
    ///     上 chain 调用，不调 prepareForReconnect）；本 task 是 stale，silent drop —— 这正是 review #2 的修复.
    ///   - 都通过 → 正常 yield.
    ///
    /// **为什么需要两个 generation 字段**：
    ///   - sessionGeneration 在每次 `connect` / `prepareForReconnect` / `disconnect` 都翻；其中 `connect`
    ///     不一定换 stream（已 connected client 复用 stream），所以单凭 sessionGeneration 不知道 stream
    ///     是新是旧.
    ///   - streamGeneration 仅在 makeStream() 时翻（init + `prepareForReconnect`），精确刻画"stream
    ///     是否还是 receive-loop launch 时那个".
    ///   - 两个字段解耦：sessionGeneration 区分"哪个 session 的 task"；streamGeneration 区分"哪个 stream
    ///     的 owner". stream 被复用时只翻 sessionGeneration（review race）；stream 被换时翻两者
    ///     （prepareForReconnect 路径）.
    private func yieldIfCurrent(
        _ message: WSMessage,
        to continuation: AsyncStream<WSMessage>.Continuation,
        mySession: Int,
        myStreamGen: Int
    ) {
        lock.lock()
        let curStreamGen = streamGenerationStorage
        let curSession = sessionGeneration
        lock.unlock()
        // 第 1 层：stream 已 swap → 孤儿 continuation，silent drop
        if curStreamGen != myStreamGen {
            os_log(.debug,
                   log: WebSocketClientImpl.logger,
                   "yieldIfCurrent dropped: orphan stream (myStreamGen=%{public}d != current=%{public}d)",
                   myStreamGen, curStreamGen)
            return
        }
        // 第 2 层：stream 复用 + generation 不匹配 → stale session，silent drop
        if curSession != mySession {
            os_log(.debug,
                   log: WebSocketClientImpl.logger,
                   "yieldIfCurrent dropped: stale mySession=%{public}d (current=%{public}d, streamGen=%{public}d)",
                   mySession, curSession, curStreamGen)
            return
        }
        continuation.yield(message)
    }

    /// fix-review round 4 P2：generation-gated stream finish —— stale receive-task 的 defer 块在
    /// cancel 后跑到 `continuation.finish()` 时，若 `currentContinuation` 已被新 session 复用
    /// （`connect(roomId:)` 复用现存 stream 路径，不调 prepareForReconnect），stale finish 会终结
    /// 新 session 的 stream → 新连接的 vm 立即收不到任何消息.
    ///
    /// **gate 逻辑**：
    ///   - **如果 stream 已被 swap（`myStreamGen != streamGeneration`，典型 `prepareForReconnect()`）**：
    ///     **必须** finish 旧 continuation —— 旧 stream 的 consumer 仍 hang 在 for-await 上等终态信号；
    ///     此路径不 race 新 stream（currentContinuation 已被换）→ 直接 finish.
    ///   - **如果 stream 被复用（`myStreamGen == streamGeneration`）**：第 2 层 session gate；
    ///     mySession != sessionGeneration → silent skip（防 stale finish 终结新 session 的 stream，review #1）；
    ///     mySession == sessionGeneration → 正常 finish.
    ///
    /// 这个区分 critical：
    ///   - test_prepareForReconnect_swapsToFreshStream 期望 prepareForReconnect 后旧 stream finish
    ///   - review #1 的 race 是 stream 被复用时 stale finish 终结新 stream → 必须 session gate
    private func finishStreamIfCurrent(
        _ continuation: AsyncStream<WSMessage>.Continuation,
        mySession: Int,
        myStreamGen: Int
    ) {
        lock.lock()
        let curStreamGen = streamGenerationStorage
        let curSession = sessionGeneration
        lock.unlock()
        // stream 已 swap → 孤儿 continuation，必须 finish 让旧 consumer for-await 退出.
        // 这条路径不 race 新 stream（因为 currentContinuation 已被换）.
        if curStreamGen != myStreamGen {
            continuation.finish()
            return
        }
        // stream 被复用 + session 不匹配 → stale finish 会终结新 session stream，silent skip
        if curSession != mySession {
            os_log(.debug,
                   log: WebSocketClientImpl.logger,
                   "finishStreamIfCurrent dropped: stale mySession=%{public}d (current=%{public}d, sameStream)",
                   mySession, curSession)
            return
        }
        continuation.finish()
    }

    /// fix-review round 2 P1：snapshot 校验当前 session generation —— 给 receive-loop catch 入口、
    /// scheduleReconnect 的 sleep+retry 闭包入口、attemptReconnect 入口等地方做 cheap 早期 silent-drop 决策.
    /// 注意：此函数返回 true 只代表"check 时一致"，后续 mutation 仍需在持锁内做最终一致性校验,
    /// 防 check-then-act race（持锁块外两次校验中间 sessionGeneration 被 disconnect 翻动）.
    private func isCurrentSession(_ mySession: Int) -> Bool {
        lock.lock()
        defer { lock.unlock() }
        return sessionGeneration == mySession
    }

    /// schedule 下一次 reconnect attempt（受 maxReconnectAttempts 上限保护）.
    /// 上限超出 → emit `.disconnected` + finish stream.
    /// 否则：emit `.reconnecting(attempt: N)` + 起 task sleep `backoffSequence[N-1]` 秒 → 调 attemptReconnect.
    ///
    /// fix-review round 2 P1：所有 callsite 走 `scheduleReconnectIfCurrent(mySession:)` 包装，
    /// 在持锁内做 generation 校验 + capture nextAttempt 决策；本函数只接受当前 session 的调用，
    /// 内部启动的 sleep+retry 闭包再捕获自己的 `mySession` 让 disconnect/prepareForReconnect 后旧闭包 silent drop.
    private func scheduleReconnect(mySession: Int) {
        // 上限校验（reconnectAttempt 是"上一次成功失败到的次数"；本次将尝试 attempt = +1）
        // fix-review round 2 P1：在持锁内同时校验 mySession + 决策 nextAttempt —— 防 stale callsite 过 gate
        // 之后再被新 session 写入打断
        let nextAttempt: Int
        let backoffSec: TimeInterval
        let exceeded: Bool
        lock.lock()
        if sessionGeneration != mySession {
            lock.unlock()
            os_log(.debug,
                   log: WebSocketClientImpl.logger,
                   "scheduleReconnect dropped: stale session (mySession=%{public}d != current=%{public}d)",
                   mySession, sessionGeneration)
            return
        }
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
            emitConnectionStateIfCurrent(.disconnected, mySession: mySession)
            lock.lock()
            // 终态写入也走 generation 校验（防极端 race：上面 emit 之后正好被 disconnect 翻 gen）
            guard sessionGeneration == mySession else {
                lock.unlock()
                return
            }
            let cont = currentContinuation
            currentRoomId = nil
            reconnectAttempt = 0
            reconnectTask = nil
            lock.unlock()
            cont.finish()
            return
        }

        // emit reconnecting(attempt: N) → vm 写 wsState = .reconnecting
        emitConnectionStateIfCurrent(.reconnecting(attempt: nextAttempt), mySession: mySession)

        let task = Task { [weak self] in
            // 用 Task.sleep（cancellation-aware）—— `disconnect()` / `prepareForReconnect()` cancel reconnectTask 后立即退出.
            try? await Task.sleep(nanoseconds: UInt64(backoffSec * 1_000_000_000))
            if Task.isCancelled { return }
            guard let self else { return }
            // fix-review round 2 P1：sleep+retry 闭包用 mySession 校验 —— disconnect / prepareForReconnect
            // 翻 gen 后旧闭包不再 effective（即使 cancel 信号传播延迟也兜底）.
            if !self.isCurrentSession(mySession) {
                os_log(.debug,
                       log: WebSocketClientImpl.logger,
                       "reconnect sleep+retry closure dropped (stale mySession=%{public}d)",
                       mySession)
                return
            }
            await self.attemptReconnect(attempt: nextAttempt, mySession: mySession)
        }
        lock.lock()
        // 写 reconnectTask 前再做一次 generation 校验，防 schedule 期间被 disconnect 抢
        guard sessionGeneration == mySession else {
            lock.unlock()
            task.cancel()
            return
        }
        reconnectTask = task
        lock.unlock()
    }

    /// fix-review round 2 P1：scheduleReconnect 的 generation-gated 入口 —— 所有 callsite 走此包装.
    private func scheduleReconnectIfCurrent(mySession: Int) {
        scheduleReconnect(mySession: mySession)
    }

    /// 真正执行第 N 次 reconnect attempt：调 connectInternal（复用 currentRoomId）.
    /// 成功 → 重置 reconnectAttempt = 0 + emit `.connected`.
    /// 失败 → 累计 attempt + scheduleReconnect 再来一次（直到上限切 terminal）.
    ///
    /// fix-review round 2 P1：`mySession` 是 scheduleReconnect 启动 sleep+retry Task 时捕获的 session
    /// generation —— 任何写共享状态 / 调 scheduleReconnect / emit / connectInternal 之前先做 generation 校验.
    /// 这样即使 cancelled task 落入 catch（disconnect / prepareForReconnect 已经翻 gen 并 cancel 但
    /// catch 还在 unwind），catch path 不会再 install 新 reconnectTask 或者 race 新 connection.
    private func attemptReconnect(attempt: Int, mySession: Int) async {
        // fix-review round 2 P1：gate at entry —— stale schedule（已被 disconnect / prepareForReconnect cancel）
        // 即使 cancel 信号传播延迟，本函数也 silent return；不再走 connectInternal、不再 install reconnectTask.
        if !isCurrentSession(mySession) {
            os_log(.debug,
                   log: WebSocketClientImpl.logger,
                   "attemptReconnect dropped at entry (stale mySession=%{public}d)",
                   mySession)
            return
        }
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
        // fix-review round 1 P1（防御性双保险）：connectInternal install 新 underlyingTask 之前再做一次
        // heartbeat reset；正常路径下 receive-loop catch 的 transient 分支已经 reset 过，
        // 此处覆盖：(1) receive-loop 在 firstFrame 之前就 catch（heartbeat 还没启动 → no-op）；
        //          (2) 极端时序绕过 #1 的 heartbeat 残留. 整体仍 generation-gated.
        cancelHeartbeatStateForReconnectIfCurrent(mySession: mySession)
        do {
            try await connectInternal(roomId: roomId, isReconnect: true, attemptNumber: attempt)
            // 成功：重置计数；`.connected` 已由 receive loop 在 firstFrameReceived 时 emit（避免与 first frame yield 调度 race）.
            // server 自动重发 room.snapshot 作为第一帧（V1 §12.1.3）→ vm 接 snapshot 对齐 roster.
            // fix-review round 2 P1：写共享状态前 generation 校验 —— 防 connectInternal 期间被 disconnect 抢.
            lock.lock()
            guard sessionGeneration == mySession else {
                lock.unlock()
                os_log(.debug,
                       log: WebSocketClientImpl.logger,
                       "attemptReconnect success path dropped: stale mySession=%{public}d",
                       mySession)
                return
            }
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

            // fix-review round 2 P1：catch 入口先做 generation 校验 —— 解决 review 第二条 race：
            //   cancelled task （disconnect / prepareForReconnect 后）仍然落到 catch；旧实装无 generation
            //   check，stale catch 安装新 reconnectTask → fresh connect 与 stale retry race.
            //   gate 后 stale catch silent return，不 emit / 不 schedule / 不写 reconnectTask.
            if !isCurrentSession(mySession) {
                os_log(.debug,
                       log: WebSocketClientImpl.logger,
                       "attemptReconnect catch dropped: stale mySession=%{public}d",
                       mySession)
                return
            }
            // 也兼顾 Task.isCancelled：caller 主动 cancel reconnectTask 时即使 generation 还没翻
            // （极端时序 cancel 先于 disconnect 内的 sessionGeneration += 1），也应 silent return.
            if Task.isCancelled {
                os_log(.debug,
                       log: WebSocketClientImpl.logger,
                       "attemptReconnect catch dropped: Task.isCancelled (mySession=%{public}d)",
                       mySession)
                return
            }

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
                emitConnectionStateIfCurrent(.disconnected, mySession: mySession)
                // 清状态 + finish stream（终态）—— generation 校验防终态写期间被 disconnect 抢
                lock.lock()
                guard sessionGeneration == mySession else {
                    lock.unlock()
                    return
                }
                let cont = currentContinuation
                currentRoomId = nil
                reconnectAttempt = 0
                reconnectTask = nil
                lock.unlock()
                cont.finish()
            case .transient:
                lock.lock()
                guard sessionGeneration == mySession else {
                    lock.unlock()
                    return
                }
                reconnectAttempt = attempt
                reconnectTask = nil  // 当前 task 已结束；scheduleReconnect 会写新的
                lock.unlock()
                scheduleReconnectIfCurrent(mySession: mySession)
            }
        }
    }

    /// fix-review round 1 P1：resolve connectGate 一次（多次调用安全 —— 二次后是 no-op）.
    /// success=true → resume(()); success=false → resume(throwing: error).
    ///
    /// fix-review round 3 P1：generation-gated resolve —— 仅当 `mySession == connectGateOwnerSession` 时
    /// 才 resolve；不匹配 silent drop（log debug）.
    /// 用途：receive-loop defer / catch path 等 "session-scoped task" 路径调用 —— 防 stale receive-task
    /// 在新 connect 已 install 新 gate 之后才跑 defer 块，把 stale failure 写到新 session 的 gate.
    /// disconnect / prepareForReconnect / deinit 显式想 fail 当前 in-flight connect → 走
    /// `resolveConnectGateUnconditionally(...)` 而非本函数.
    private func resolveConnectGate(success: Bool, error: Error?, mySession: Int) {
        lock.lock()
        guard connectGateOwnerSession == mySession else {
            lock.unlock()
            os_log(.debug,
                   log: WebSocketClientImpl.logger,
                   "resolveConnectGate dropped: stale mySession=%{public}d (gateOwner=%{public}@)",
                   mySession,
                   connectGateOwnerSession.map { String($0) } ?? "nil")
            return
        }
        let cont = connectGate
        connectGate = nil
        connectGateOwnerSession = nil
        lock.unlock()
        guard let cont = cont else { return }
        if success {
            cont.resume()
        } else {
            cont.resume(throwing: error ?? WSError.connectionFailed(underlyingDescription: "unknown"))
        }
    }

    /// fix-review round 3 P1：unconditional resolve —— 不做 generation 校验，专供 disconnect /
    /// prepareForReconnect / deinit 路径使用：这三类 caller 已经在持锁内 `sessionGeneration += 1` 翻新了
    /// generation，但仍然显式想"fail 当前 in-flight connect()"（caller 主动放弃）.
    /// 安全性：此函数只读 + 清 `connectGate` / `connectGateOwnerSession` 两个字段，不写其他共享状态;
    /// 二次调用 no-op（`cont == nil`）.
    private func resolveConnectGateUnconditionally(success: Bool, error: Error?) {
        lock.lock()
        let cont = connectGate
        connectGate = nil
        connectGateOwnerSession = nil
        lock.unlock()
        guard let cont = cont else { return }
        if success {
            cont.resume()
        } else {
            cont.resume(throwing: error ?? WSError.connectionFailed(underlyingDescription: "unknown"))
        }
    }

    // MARK: - Story 12.6 heartbeat helpers

    /// Story 12.6：启动 heartbeat task. 在 firstFrameReceived 同步路径调，与 emitConnectionState(.connected) 同位置.
    /// `mySession` 由 receive-loop 内捕获的 mySession 透传 —— heartbeat task 内任何 mutation / 调
    /// `cancelUnderlyingTaskWithGoingAway` 之前必须 generation-gated.
    ///
    /// **lifecycle**：
    ///   - 启动前 cancel 既有 heartbeatTask（防御性兜底；正常路径下 disconnect/prepareForReconnect 已 cancel）
    ///   - 用 Task { ... } 起 detached-style task（捕获 self weak）
    ///   - task 内 while !Task.isCancelled 循环：
    ///     1. await Task.sleep(heartbeatInterval) —— 30s 心跳间隔
    ///     2. 如果 cancelled / stale session → return
    ///     3. lock 内创建新 pendingPongContinuation + 写入 + 自增 heartbeatSeq + 取出 **activeTask 引用 snapshot**
    ///     4. fix-review round 4 P1：lock unlock 之后、send 之前再做一次 lock 内 final 校验
    ///        （`sessionGeneration == mySession && underlyingTask === activeTask`）—— 防 unlock window 内
    ///        reconnect / caller-driven connect() 跑过把 underlyingTask 换掉，旧 ping 误发到新 socket.
    ///        校验失败 silent skip + cleanup latch.
    ///     5. fix-review round 4 P1：用 captured activeTask 直接 send（不走 self.send 的 re-read），
    ///        从根上杜绝"send 内部又取一次 self.underlyingTask 拿到新 socket"的 race.
    ///        send 抛错走与 pong timeout 同 fallback：cancelUnderlyingTaskWithGoingAwayIfCurrent → reconnect.
    ///     6. 用 withTaskGroup 并发等：(a) AsyncStream 收到 yield 即 pong-arrived；(b) Task.sleep(pongTimeout) 即超时
    ///     7. (a) 先到 → 清 pendingPongContinuation = nil → continue 下一轮
    ///     8. (b) 先到 → cancel underlying task with .goingAway → return（receive-loop catch 接管走 12.5 transient → reconnect）
    private func startHeartbeatTask(mySession: Int) {
        lock.lock()
        let oldTask = heartbeatTask
        heartbeatTask = nil
        lock.unlock()
        oldTask?.cancel()

        let interval = self.heartbeatInterval
        let timeout = self.pongTimeout
        let task = Task { [weak self] in
            while !Task.isCancelled {
                // 1. 心跳间隔 sleep
                do {
                    try await Task.sleep(nanoseconds: UInt64(interval * 1_000_000_000))
                } catch {
                    return  // cancellation
                }
                if Task.isCancelled { return }
                guard let strongSelf = self else { return }
                if !strongSelf.isCurrentSession(mySession) {
                    return  // stale session
                }

                // 2. 创建 pong latch + 取 underlying task + 自增 seq
                let (pongStream, pongCont) = AsyncStream<Void>.makeStream()
                let activeTaskOpt: WebSocketTaskHandle?
                let seq: Int
                strongSelf.lock.lock()
                // 再次 generation 校验（持锁内最终一致）
                guard strongSelf.sessionGeneration == mySession else {
                    strongSelf.lock.unlock()
                    pongCont.finish()
                    return
                }
                // 关 finish 既有 pendingPongContinuation（防泄漏；上一轮 ping 若没回 pong 也已被 timeout 处理过）
                strongSelf.pendingPongContinuation?.finish()
                strongSelf.pendingPongContinuation = pongCont
                strongSelf.heartbeatSeq += 1
                seq = strongSelf.heartbeatSeq
                // fix-review round 2 P2：在持锁内一起写 requestId —— receive-loop notify 路径与本写入用同一锁,
                // 保证"新一轮 ping 发出 + pendingPongRequestId 切换"是 lock 内原子；不会有"旧 requestId 还在 +
                // 新 latch 已 install" 的中间态被 receive-loop 错配.
                strongSelf.pendingPongRequestId = "ping_\(strongSelf.heartbeatSeq)"
                activeTaskOpt = strongSelf.underlyingTask
                strongSelf.lock.unlock()

                guard let activeTask = activeTaskOpt, activeTask.isRunning else {
                    // underlying task 不在了（被 cancel / disconnect）→ 直接 return
                    strongSelf.lock.lock()
                    strongSelf.pendingPongContinuation?.finish()
                    strongSelf.pendingPongContinuation = nil
                    strongSelf.pendingPongRequestId = nil  // round 2 P2
                    strongSelf.lock.unlock()
                    return
                }

                // fix-review round 4 P1：测试注入点 —— production 此处永远 no-op.
                // 单测在此 hook 内主动跑 cancel-old-heartbeat + install-new-socket 模拟 reconnect race；
                // hook 返回后下方 final pre-send 校验必须能识别"我已 stale" → silent skip.
                if let hook = strongSelf.beforeHeartbeatSendHook {
                    await hook()
                    if Task.isCancelled { return }
                }

                // 3. send ping —— fix-review round 4 P1：双层防御.
                //
                // race 触发条件（review round 4 P1）：
                //   T0  heartbeat task 已在 lock 内 install pendingPongContinuation + 取出 activeTask snapshot
                //   T0+ lock.unlock() 之后、send 之前的 unlock window 内：
                //       reconnect / caller-driven connect() 跑 reset 路径（cancelHeartbeatStateForReconnect /
                //       prepareForReconnect / connectInternal swap）→ cancel 旧 heartbeatTask + finish pendingPongCont +
                //       sessionGeneration += 1 + install 新 underlyingTask（socketB）.
                //   旧实装：直接调 `self.send(.ping(...))` —— 内部 re-read `self.underlyingTask` 拿到 socketB →
                //       ping 发到新 socket → server 在 mandatory `room.snapshot` 之前回 pong → 打破
                //       "first frame == handshake snapshot" invariant → resolve connect() 在 room state 初始化前.
                //
                // 防御层 1（最强）：用 captured `activeTask` 直接 send，绕过 self.send 的 re-read.
                //   即便 race 跑赢、underlyingTask 已被换成 socketB，本 send 仍只能落到 socketA.
                //   socketA 已被 reset 路径 cancel → activeTask.send 抛 URLError(.cancelled) → 走 catch
                //   分支跑既有 cleanup（与 round 3 P1 同路径）.
                //
                // 防御层 2（pre-send final 校验）：lock 内再次校验 sessionGeneration + underlyingTask identity.
                //   不匹配 = unlock window 内有 reconnect / connect()/ disconnect 跑过 → silent skip + cleanup latch.
                //   即便防御层 1 万一被未来重构破坏，本层仍能拦下 stale ping.
                let finalCheckOK: Bool
                strongSelf.lock.lock()
                finalCheckOK = strongSelf.sessionGeneration == mySession
                    && strongSelf.underlyingTask === activeTask
                strongSelf.lock.unlock()
                if Task.isCancelled || !finalCheckOK {
                    // unlock window 内 race 跑赢（reconnect / connect()/ disconnect 已 swap 状态）→ silent skip.
                    // 旧 task 的 cancel 已由 reset 路径 finish pendingPongCont（防 leak）；这里再做一次
                    // 防御性 cleanup（仅当本 mySession 仍 == sessionGeneration 时）.
                    strongSelf.lock.lock()
                    if strongSelf.sessionGeneration == mySession {
                        strongSelf.pendingPongContinuation?.finish()
                        strongSelf.pendingPongContinuation = nil
                        strongSelf.pendingPongRequestId = nil
                    }
                    strongSelf.lock.unlock()
                    os_log(.debug,
                           log: WebSocketClientImpl.logger,
                           "heartbeat send pre-flight check failed (stale gen / task swapped) — silent skip")
                    return
                }

                // fix-review round 3 P1：send 抛错 **不能** silent return —— 旧实现假设 receive-loop 会
                // 接管 catch 走 reconnect，但在 "locally broken socket" 场景下 send 失败时 receive() 仍
                // blocked（没观察到 close） → heartbeat 静默停止 + nothing schedules reconnect → client 卡死
                // 看起来仍 connected. 修复：send 失败强制走与 pong timeout **完全相同** 的 fallback ——
                // cancelUnderlyingTaskWithGoingAwayIfCurrent 让 receive-loop classify 为 transient (1001)
                // → schedule reconnect.
                do {
                    // fix-review round 4 P1：用 captured activeTask 直接 send，绕过 self.send 的 re-read.
                    let text = try WSMessageCodec.encode(.ping(requestId: "ping_\(seq)"))
                    try await activeTask.send(.string(text))
                } catch {
                    // fix-review round 5 P1：catch 路径不能仅靠 sessionGeneration 守护 ——
                    // reconnect 透明续接保持同 generation，但会在 catch 跑前 install 新 underlyingTask（socketB）.
                    //
                    // race 时序：
                    //   T0  heartbeat send（在 socketA 上）suspended
                    //   T1  socketA 底层断 → receive-loop catch → cancelHeartbeatStateForReconnectIfCurrent
                    //       （但 reconnect 透明续接，sessionGeneration 不翻）→ attemptReconnect →
                    //       connectInternal → install 新 underlyingTask = socketB（sessionGen 仍 == mySession）
                    //   T2  T0 那个 send 终于抛错（因为 socketA 断了）
                    //   T3  catch 跑 → 旧实装直接调 cancelUnderlyingTaskWithGoingAwayIfCurrent(mySession:)
                    //       → guard sessionGeneration == mySession **通过** → 读 self.underlyingTask（= socketB）
                    //       → cancel(.goingAway) → 错杀 socketB → receive-loop 又 catch transient →
                    //       又 schedule reconnect → self-sustaining loop.
                    //
                    // 修复（task identity 校验）：cancel underlying 之前先确认 self.underlyingTask 仍 === activeTask.
                    //   不一致 = "我 send 的那个 socket 已被新 session/新 reconnect swap 走" → silent skip cancel.
                    //   理由：新 underlyingTask 的 install 路径（cancelHeartbeatStateForReconnectIfCurrent）
                    //   已经把旧 heartbeatTask cancel + finish 旧 pongCont，新 task 不需要也不应该被旧 catch 干扰.
                    //
                    // latch cleanup 仍保留（已经持锁内做 sessionGeneration 守护，安全）.
                    //
                    // fix-review round 7 P1：另一类 race —— **post-handshake server-initiated terminal close**
                    // 与 heartbeat send 之间的 race。
                    //
                    // 时序：
                    //   T0  heartbeat send（在 socketA 上）suspended
                    //   T1  server 发送 close frame（如 4001 token 过期）→ URLSessionWebSocketTask
                    //       runtime 把 task.closeCode 设为 4001（read-only property，server close frame 到达时设置）
                    //   T2  receive-loop 还没消费到 close（异步调度延迟）
                    //   T3  T0 那个 send 终于抛错
                    //   T4  catch 跑 → 旧实装无条件 cancel(.goingAway) → 把 task.closeCode **覆盖**为 1001
                    //   T5  receive-loop 终于跑到 catch → 读 task.closeCode → 拿到 1001（已被 T4 覆盖）→
                    //       classify 为 transient → schedule reconnect 而非 emit .disconnected → 破坏 12.5
                    //       terminal-vs-transient contract（4001 应触发 re-auth，不应静默 retry）.
                    //
                    // 修复：catch 内先观测 activeTask.closeCode：
                    //   - != .invalid → server 已发送 close frame（任意 close code）→ silent skip cancel；
                    //     让 receive-loop 自然处理 server 发的真实 close code
                    //     （正确分类 terminal vs transient，不被 1001 注入污染）.
                    //   - == .invalid → 还没收到 close frame（locally broken socket / 底层 TCP 异常）→
                    //     当前 transient reconnect 路径合理（receive 仍 blocked，需要主动 1001 唤醒它）.
                    //
                    // 注：此校验在 task identity 校验之前 —— task identity 只覆盖 "underlyingTask 已 swap"
                    // 的 race；server-initiated close 的 race 中 underlyingTask 仍 === activeTask（receive-loop
                    // 还没跑到 swap 路径），所以必须独立校验.
                    let observedCloseCode = activeTask.closeCode
                    let underlyingStillSame: Bool
                    strongSelf.lock.lock()
                    underlyingStillSame = strongSelf.sessionGeneration == mySession
                        && strongSelf.underlyingTask === activeTask
                    // latch cleanup（仅当还在本 session）—— 防 stale catch 清掉新 session 的 latch.
                    if strongSelf.sessionGeneration == mySession {
                        strongSelf.pendingPongContinuation?.finish()
                        strongSelf.pendingPongContinuation = nil
                        strongSelf.pendingPongRequestId = nil  // round 2 P2
                    }
                    strongSelf.lock.unlock()

                    if !underlyingStillSame {
                        os_log(.debug,
                               log: WebSocketClientImpl.logger,
                               "heartbeat send catch: stale send (underlyingTask was swapped to new socket) — silent skip cancel; new task already owns reconnect path")
                        return
                    }

                    // round 7 P1：server 已发送 terminal/transient close → silent skip cancel.
                    // 不能用 1001 覆盖真实 server close code（4001/4002/4003/4004 等 terminal 必须传到 receive-loop
                    // classify 走 .disconnected 而非 transient retry）.
                    //
                    // round 8 P2：把"closeCode == .invalid 才 cancel"的 atomic check 折进 helper 内部.
                    // 旧实装：catch 入口先读一次 `activeTask.closeCode`，**unlock 后**再调 cancel —— TOCTOU
                    // race window：T_a 读 closeCode = .invalid → T_b server close frame 到达，runtime 设 4001
                    // → T_c 仍走 cancel(.goingAway) 路径覆盖 4001 → terminal 被错分 transient → silent retry.
                    //
                    // 修复：把 closeCode re-check 移到 helper 持锁段内（与 sessionGeneration / underlying-task
                    // identity 校验一起 atomic），消除中间 unlocked window. 即便 catch 入口 closeCode 为 .invalid,
                    // 进 helper 之后 cancel 之前再读一次 —— 不是 .invalid → silent skip.
                    if observedCloseCode != .invalid {
                        os_log(.info,
                               log: WebSocketClientImpl.logger,
                               "heartbeat send catch: server-initiated close already in flight (closeCode rawValue=%{public}d) — silent skip cancel; receive-loop will classify real close code",
                               observedCloseCode.rawValue)
                        return
                    }

                    os_log(.info,
                           log: WebSocketClientImpl.logger,
                           "heartbeat ping send failed (no server close observed at catch entry) → call helper with atomic closeCode re-check → cancel(.goingAway) only if still .invalid")
                    // round 8 P2：传入 activeTask 让 helper 内做 closeCode atomic re-check.
                    strongSelf.cancelUnderlyingTaskWithGoingAwayIfCurrent(
                        mySession: mySession,
                        activeTask: activeTask
                    )
                    return
                }

                // 4. 并发等 pong-arrived OR pong-timeout
                let timedOut = await Self.awaitPongOrTimeout(
                    pongStream: pongStream,
                    timeout: timeout
                )

                // 5. 清 latch（无论 timedOut 与否；防泄漏）
                strongSelf.lock.lock()
                // 再次 generation 校验
                guard strongSelf.sessionGeneration == mySession else {
                    strongSelf.pendingPongContinuation?.finish()
                    strongSelf.pendingPongContinuation = nil
                    strongSelf.pendingPongRequestId = nil  // round 2 P2
                    strongSelf.lock.unlock()
                    return
                }
                strongSelf.pendingPongContinuation?.finish()
                strongSelf.pendingPongContinuation = nil
                strongSelf.pendingPongRequestId = nil  // round 2 P2：本轮结束，下一轮 ping 会重置.
                strongSelf.lock.unlock()

                if Task.isCancelled { return }
                if timedOut {
                    // 6. pong 超时 → cancel underlying task with .goingAway（1001 = transient）
                    //    → receive-loop catch 走 12.5 状态机 schedule reconnect.
                    //    **不**调 public disconnect()（那是 close 1000 = terminal 不重连）.
                    //
                    // round 8 P2：pong-timeout 触发时 server 也可能已发送 terminal close（4001 等）.
                    // 同样把 closeCode atomic check 折进 helper —— 不是 .invalid → silent skip 让 receive-loop
                    // classify 真实 close code，不让 1001 覆盖.
                    strongSelf.cancelUnderlyingTaskWithGoingAwayIfCurrent(
                        mySession: mySession,
                        activeTask: activeTask
                    )
                    return
                }
                // 7. pong 正常到达 → continue 下一轮 sleep
            }
        }
        lock.lock()
        // 防御性兜底：极端 race 下若已有 disconnect / prepareForReconnect 抢着 cancel 旧 task 同时翻 gen，
        // 此处校验确保 stale heartbeat 注入不污染新 session 的 heartbeatTask 字段.
        if sessionGeneration == mySession {
            heartbeatTask = task
        } else {
            task.cancel()
        }
        lock.unlock()
    }

    /// Story 12.6：内部 helper —— 并发等 pong-arrived OR Task.sleep timeout，先到者赢.
    /// 返回 true = 超时；false = pong arrived（或 stream 被 finish）.
    private static func awaitPongOrTimeout(
        pongStream: AsyncStream<Void>,
        timeout: TimeInterval
    ) async -> Bool {
        return await withTaskGroup(of: Bool.self) { group in
            // task A: 等 pong yield；stream finish 后 for-await 自动退出 = false（pong 已到 / cancellation）
            group.addTask {
                for await _ in pongStream {
                    return false  // pong arrived
                }
                // stream 被 finish 但没 yield —— 视为 cancellation（caller 路径），返回 false 让 caller 决策
                return false
            }
            // task B: timer
            group.addTask {
                try? await Task.sleep(nanoseconds: UInt64(timeout * 1_000_000_000))
                return true  // timeout
            }
            // 第一个返回的赢
            let result = await group.next() ?? false
            group.cancelAll()
            return result
        }
    }

    /// Story 12.6：在 underlying URLSessionWebSocketTask 上 cancel(with: .goingAway) ——
    /// rawValue=1001 在 12.5 classifyCloseCode 是 transient → receive-loop catch → schedule reconnect.
    /// generation-gated：stale heartbeat task 不能 cancel 新 session 的 underlying task.
    ///
    /// fix-review round 8 P2：closeCode atomic re-check —— caller 传入 send / pong-timeout 当时 captured 的
    /// `activeTask`，helper 在持锁段内重读 `activeTask.closeCode`：
    ///   - != .invalid → server 已发送真实 close frame（任意 close code）→ silent skip cancel；让 receive-loop
    ///     拿到真实 close code 走 classifier（避免 1001 覆盖 4001/4004 等 terminal close 导致 silent retry）.
    ///   - == .invalid → 仍是 "locally broken socket / pong timeout" 路径 → 主动 cancel(.goingAway) 唤醒
    ///     receive-loop 走 transient reconnect.
    /// 同时校验 `underlyingTask === activeTask`：reconnect 透明续接保持同 generation 但会 swap underlyingTask;
    /// 不一致 = "我捕获的那个 socket 已被新 session swap 走" → silent skip（与 send-catch task identity 校验同精神）.
    /// 持锁内做这三个校验是 atomic 的关键 —— 消除"check 后、cancel 前"的 unlocked window 上的 TOCTOU race.
    private func cancelUnderlyingTaskWithGoingAwayIfCurrent(mySession: Int, activeTask: WebSocketTaskHandle) {
        lock.lock()
        guard sessionGeneration == mySession else {
            lock.unlock()
            os_log(.debug,
                   log: WebSocketClientImpl.logger,
                   "cancelUnderlying drop: stale mySession=%{public}d (current=%{public}d)",
                   mySession, sessionGeneration)
            return
        }
        guard underlyingTask === activeTask else {
            lock.unlock()
            os_log(.debug,
                   log: WebSocketClientImpl.logger,
                   "cancelUnderlying drop: underlyingTask was swapped (transparent reconnect installed new socket) — silent skip; new task owns its own reconnect path")
            return
        }
        // round 8 P2：atomic closeCode re-check —— 在持锁段内、cancel 调用之前重读，消除 TOCTOU race window.
        // server close frame 在 catch 入口与 cancel 之间到达时，runtime 已把 task.closeCode 设为真实值（如 4001）;
        // 此时 silent skip，让 receive-loop 走真实 close code 的 classifier 路径.
        let observedCloseCode = activeTask.closeCode
        guard observedCloseCode == .invalid else {
            lock.unlock()
            os_log(.info,
                   log: WebSocketClientImpl.logger,
                   "cancelUnderlying atomic re-check: server-initiated close arrived inside lock window (closeCode rawValue=%{public}d) — silent skip cancel; receive-loop will classify real close code",
                   observedCloseCode.rawValue)
            return
        }
        let task = underlyingTask
        lock.unlock()
        os_log(.info,
               log: WebSocketClientImpl.logger,
               "cancelUnderlying with .goingAway (1001) → receive-loop catch transient → schedule reconnect")
        task?.cancel(with: .goingAway, reason: "heartbeat send/pong timeout".data(using: .utf8))
    }

    /// Story 12.6 fix-review round 1 P1：transient close 触发自动 reconnect 前**必须**显式
    /// cancel 旧 heartbeatTask + finish pendingPongContinuation —— 否则 race 窗口打开：
    ///
    ///   T0  heartbeat task 已 sendPing → 进入 awaitPongOrTimeout（pendingPongContinuation 在场）
    ///   T1  underlying 发生 transient close（1001/1006/1011/4005）→ receive-loop catch
    ///   T2  receive-loop catch 调 scheduleReconnectIfCurrent —— **不**翻 sessionGeneration
    ///       （契约 5：reconnect 透明续接，复用同一 session.gen / stream.gen）
    ///   T3  attemptReconnect 跑 connectInternal → makeTask + resume → install 新 underlyingTask
    ///   T4  旧 heartbeat task 的 5s pong timer fire → 调 cancelUnderlyingTaskWithGoingAwayIfCurrent
    ///   T5  其内的 `sessionGeneration == mySession` guard **通过**（gen 没翻）→ 取出 underlyingTask
    ///       （此刻指向新 socket）→ cancel(.goingAway)
    ///   T6  新 socket handshake 被错杀 → 又被分类为 transient → 又一次 schedule reconnect ……
    ///       一个 recoverable disconnect 演化成连续 reconnect 失败.
    ///
    /// **修复策略**（review 提示方向 1）：在"旧 socket close → 新 underlyingTask install" 的接缝上
    /// **同步** cancel 旧 heartbeat task + finish 旧 pendingPongContinuation：
    ///   - heartbeat task 被 cancel → 内层 `Task.sleep` / `awaitPongOrTimeout` 立即抛 cancellation → return.
    ///   - pendingPongContinuation finish → awaitPongOrTimeout 内 `for await _ in pongStream` 自然退出
    ///     （走 false 分支 = "pong 到达"语义；caller 已 cancel，下一行 `Task.isCancelled` 检测 → return，
    ///     不再 fire `cancelUnderlyingTaskWithGoingAwayIfCurrent`）.
    ///   - 即使极端时序漏过两层屏障让 cancelUnderlying 被调到，因为 heartbeatTask / pendingPong 字段已清，
    ///     新 heartbeat 启动前不会被旧 task 污染；新 task 也会持有自己的 mySession（即将到来的 firstFrame
    ///     重启路径会捕获最新 sessionGeneration）.
    ///
    /// **不**翻 sessionGeneration —— 因为 reconnect 必须保持 session 透明续接，外层 stream / vm 视角不能感知 session 翻新.
    /// 整个修复仅限 heartbeat 子系统内部 reset.
    ///
    /// **callsite**：
    ///   1. receive-loop catch path 的 transient close 分支，`scheduleReconnectIfCurrent` 之前
    ///   2. attemptReconnect 进入 `connectInternal` 之前（防御性双保险，覆盖 receive-loop 在
    ///      firstFrame 之前就 catch / 极端时序漏过 #1 的兜底）
    ///   3. **fix-review round 6 P2**：receive-loop catch path 的 terminal close 分支
    ///      （4001/4002/4003/4004/4006/4007/1000/未知 close code）—— terminal final cleanup,
    ///      **不**重启 heartbeat. 与 transient 分支对齐：旧实装漏 cancel 导致 heartbeatTask +
    ///      pendingPongContinuation 在 stream finish 之后仍 alive 一整个 interval. helper 名含
    ///      "ForReconnect" 是历史包袱（最初为 reconnect 路径设计），但内部 reset 逻辑同样适用
    ///      terminal 路径，故复用而非抽新 helper.
    ///
    /// generation-gated：仅当 `sessionGeneration == mySession` 才执行 reset；stale 调用 silent drop
    /// （防 stale receive-loop 的 catch path 在新 session swap 后才跑、把新 session 的 heartbeat 也清掉）.
    private func cancelHeartbeatStateForReconnectIfCurrent(mySession: Int) {
        lock.lock()
        guard sessionGeneration == mySession else {
            lock.unlock()
            os_log(.debug,
                   log: WebSocketClientImpl.logger,
                   "cancelHeartbeatStateForReconnect dropped: stale mySession=%{public}d (current=%{public}d)",
                   mySession, sessionGeneration)
            return
        }
        let oldHeartbeatTask = heartbeatTask
        let oldPongCont = pendingPongContinuation
        heartbeatTask = nil
        pendingPongContinuation = nil
        pendingPongRequestId = nil  // round 2 P2：与 pendingPongContinuation 同生命周期
        lock.unlock()
        os_log(.debug,
               log: WebSocketClientImpl.logger,
               "cancelHeartbeatStateForReconnect: cancelling old heartbeat (taskNonNil=%{public}@, pongContNonNil=%{public}@)",
               oldHeartbeatTask != nil ? "yes" : "no",
               oldPongCont != nil ? "yes" : "no")
        oldHeartbeatTask?.cancel()
        oldPongCont?.finish()
    }

    /// Story 12.6：generation-gated pong-notify —— receive-loop decode 出 .pong 时调.
    /// stream-owner stale（旧 receive-loop 在新 session 已翻 gen 后才跑）silent drop，
    /// 不打扰新 session 的 heartbeat latch.
    ///
    /// fix-review round 2 P2：`incomingRequestId` 必须 == `pendingPongRequestId` 才 yield latch.
    /// V1 §12.2 钦定 `pong.requestId` echo 对应 ping 的 requestId —— receive-loop 必须按
    /// requestId 配对 pong 与 in-flight ping，**不能**无条件接受任何 pong 当 ack.
    /// 不匹配场景：server 发的 delayed / duplicated pong（旧 ping 的 pong 才到达，但此时 client
    /// 已发出新 ping 等新 pong）→ silent drop + log debug；让 in-flight ping 真等到 timeout / 真 pong.
    /// 旧实装无条件 yield → 旧 pong 错误 ack 当前 in-flight ping → miss 当前 ping 实际未被 ack →
    /// 推迟一整个 heartbeat interval 才检测到 reconnect 需要.
    private func notifyPongReceivedIfCurrent(requestId incomingRequestId: String, mySession: Int) {
        lock.lock()
        guard sessionGeneration == mySession else {
            lock.unlock()
            os_log(.debug,
                   log: WebSocketClientImpl.logger,
                   "notifyPongReceived dropped: stale mySession=%{public}d (current=%{public}d)",
                   mySession, sessionGeneration)
            return
        }
        // fix-review round 2 P2：requestId 校验 —— 必须 == 当前 in-flight ping 的 requestId.
        // pendingPongRequestId == nil 场景：本轮无 in-flight ping（heartbeat 还在 sleep / 上一轮已 ack）→
        //   收到 pong 必然是 stale（重复 / 迟到的旧 pong）→ silent drop.
        // pendingPongRequestId != incomingRequestId 场景：旧 ping 的迟到 pong → silent drop.
        guard let expected = pendingPongRequestId, expected == incomingRequestId else {
            let actualExpected = pendingPongRequestId ?? "<nil>"
            lock.unlock()
            os_log(.debug,
                   log: WebSocketClientImpl.logger,
                   "notifyPongReceived dropped: requestId mismatch (incoming=%{public}@, expected=%{public}@) — stale or duplicated pong",
                   incomingRequestId, actualExpected)
            return
        }
        let cont = pendingPongContinuation
        lock.unlock()
        cont?.yield(())
        // 注意：**不**在这里 finish + 清 nil；让 heartbeat task 内的 awaitPongOrTimeout return false 后
        // 在 lock 内 finish + 清 nil（保单一所有权：pendingPongContinuation 的生命周期由 heartbeat task 拥有）.
    }

    deinit {
        underlyingTask?.cancel(with: .normalClosure, reason: nil)
        receiveTask?.cancel()
        reconnectTask?.cancel()       // Story 12.6：防御性（既有 12.5 落地未做）
        heartbeatTask?.cancel()       // Story 12.6
        pendingPongContinuation?.finish()  // Story 12.6
        currentContinuation.finish()
        // fix-review round 1 P1：deinit 期间 caller 还在 await connect() 概率极低（caller 通常持 strong ref），
        // 防御性兜底 resume(throwing:) 让 await 立即返回，不留 dangling continuation 触发 fatalError.
        // fix-review round 3 P1：deinit 路径同步清 connectGateOwnerSession 字段（保字段一致性）.
        if let cont = connectGate {
            connectGate = nil
            connectGateOwnerSession = nil
            cont.resume(throwing: WSError.connectionFailed(underlyingDescription: "deinit before first frame"))
        }
    }
}
