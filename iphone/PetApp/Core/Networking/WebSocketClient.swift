// WebSocketClient.swift
// Story 12.1 AC3: WebSocket 客户端协议层（最小契约；真实实装 `WebSocketClientImpl` 由 Story 12.2 落地）.
//
// 设计原则（与 ADR-0002 §3.1 / RoomViewModel 接缝设计同精神）：
//   - protocol-first：让 RealRoomViewModel 通过构造注入 mock 客户端写 unit test，
//     无需 stub URLSession / 起 mock server.
//   - AsyncStream 驱动 incoming messages（与 §12.3 "服务端主动推送" 异步语义吻合 + Swift 5.5+ 原生支持）.
//   - 节点 4 阶段 Epic 10 §12.3 钦定 server → client 只发 `room.snapshot` / `pong` / `error` 三种消息,
//     WSMessage enum 本 story 仅覆盖三种 case + 一个 `unknown(rawType: String)` fallback case
//     不破坏 stream（参见 epic Story 12.2 AC "edge: 服务端推未知 type → 解码失败 + log warning + 不破坏 stream"）.
//   - 节点 4 阶段 client → server 仅 `ping`（Story 12.6 落地）+ 业务暂无 client → server 消息.
//
// 真实实装锚定（Story 12.2）：基于 `URLSessionWebSocketTask` 实装 `WebSocketClientImpl`，
// 包含 connect(url:token:) / send(_:) / disconnect() / messages 全集；本 story 只引入 protocol + Mock.

import Foundation

public protocol WebSocketClient: AnyObject, Sendable {
    /// 服务端 → 客户端消息流（按 §12.3 通用信封解析后的强类型 enum）.
    /// 实装层（Story 12.2 `WebSocketClientImpl`）从 underlying URLSessionWebSocketTask 读出 text frame
    /// → JSONDecode 信封 → 按 `type` 路由到 enum case → yield 到该 stream.
    ///
    /// **重要语义**：getter 返回的 stream 在 `disconnect()` 后被 `finish()`；后续若要复用同一 client
    /// 接收新消息（如 room A→B 切换、leave-rejoin），caller **必须**先调 `prepareForReconnect()` 重置
    /// stream，然后再次读 `messages`（拿到新 stream）+ 起新 consumer task.
    var messages: AsyncStream<WSMessage> { get }

    /// fix-review round 2 P2（Story 15.2）：每次 `prepareForReconnect()` 翻新的"stream 身份" counter.
    ///
    /// **背景**：`RealRoomViewModel` 守护 stale message 时仅靠 `streamRoomId == lastObservedRoomId` 不够 ——
    /// 同房间 leave-rejoin（A→A）/ same-room reconnect 路径下，旧 / 新两条 stream 的 `streamRoomId` 都是同
    /// 一个 room，旧 stream 已 dequeue 的 late message 会通过 roomId-only guard 错误覆盖新 snapshot 派生的
    /// state（codex review r1+r2 反复 flag 的 P2 race）.
    ///
    /// **语义契约**：
    ///   - 单调递增 `Int`；初值由实装决定（Mock 从 0 起；Impl 同模式）
    ///   - 仅在 `prepareForReconnect()` 调用处 +1（与 `messages` stream 的 swap 严格对应；同一 stream 实例
    ///     的整个生命周期内 generation 值不变）
    ///   - **不**由 `connect(roomId:)` / `disconnect()` 翻新 ——它们不 swap stream（disconnect 仅 finish 当前
    ///     continuation；connect 在 fresh state 下不 makeStream）
    ///   - read-only：caller 仅读不写
    ///
    /// **使用模式**（caller 端）：consumer task 启动时优先使用 `currentStreamSnapshot` 原子读 ——
    /// 单独读 `streamGeneration` 仅留作向后兼容（测试断言"prepare 前后差 1"等 read-only 场景）.
    /// 关键 race 场景（codex r4 P1）：consumer 启动若先 `let g = client.streamGeneration` 再
    /// `for await message in client.messages`，两步之间 `prepareForReconnect()` 翻新两个字段 →
    /// 新 task 拿到新 stream 但携带旧 generation，handle 把新 stream 所有消息当 stale 丢弃 →
    /// 房间更新卡死直到下次 restart consumer. **正确接缝是 `currentStreamSnapshot`**.
    var streamGeneration: Int { get }

    /// fix-review round 4 P1（Story 15.2）：原子读 stream + generation 的快照接缝.
    ///
    /// **race 背景**：consumer 启动若分两步读（先 `streamGeneration` 再 `messages`），
    /// `prepareForReconnect()` 在两步之间发生 → 新 task 订阅**新** stream 却携带**旧** generation
    /// → handle 把新 stream 上所有消息识别为 stale 全部丢弃 → 房间更新卡死直到下次 restart consumer.
    /// codex review r4 [P1] 定位的实际可达路径（`RealRoomViewModel.startConsumingMessages`
    /// 第 552-554 行）.
    ///
    /// **语义契约**：返回的 `(stream, generation)` 必须在同一临界区内捕获 ——
    ///   - 实装层（`WebSocketClientImpl`）：lock 内同时读 `currentStream` + `streamGenerationStorage`
    ///     → 任何 `prepareForReconnect()` 要么完全先于本次读（拿到新对 + 新 gen），要么完全后于本次读
    ///     （拿到旧对 + 旧 gen 后下次 message 投递时被 generation gate 识别为 stale）.
    ///   - Mock 层（`WebSocketClientMock`）：单线程语义下两个字段写在同一函数体（`prepareForReconnect`），
    ///     单次方法调用一次读取即可同时返回，天然原子.
    ///
    /// **调用方使用模式**：
    /// ```swift
    /// let snapshot = client.currentStreamSnapshot
    /// consumerTask = Task {
    ///     for await message in snapshot.stream {
    ///         handle(message: message, streamGeneration: snapshot.generation)
    ///     }
    /// }
    /// ```
    /// 注意：**不要**在 Task 体内重新读 `client.messages` / `client.streamGeneration` ——
    /// 那会重新引入 race；快照拿到后视为一对不可分割的"per-task identity".
    var currentStreamSnapshot: (stream: AsyncStream<WSMessage>, generation: Int) { get }

    /// Story 12.2 新增：拨号到指定 roomId 的 WS 网关.
    ///
    /// 实装层（WebSocketClientImpl）：
    /// 1. 用 tokenProvider() 取最新 Bearer token（nil → throw WSError.tokenMissing）
    /// 2. 拼接 `{ws_scheme}://{host}/ws/rooms/{roomId}?token={url-encoded}`（V1 §12.1）
    /// 3. URLSession.webSocketTask(with: URLRequest) → resume()
    /// 4. 启动 receive 长任务，把 underlying frame 解码为 WSMessage 并 yield 到 messages stream
    /// 5. 拨号失败（DNS / TLS / connection refused）→ throw WSError.connectionFailed(underlying:)
    ///
    /// 调用约定：caller（Story 12.7 UseCase 或 Story 12.5 reconnect 状态机）必须在 `messages`
    /// 被消费之前调；同一 client 复用时（leave-rejoin / room A→B）必须先 disconnect → prepareForReconnect → connect.
    func connect(roomId: String) async throws

    /// Story 12.2 新增：发送 client → server 消息（节点 4 阶段仅 ping —— Story 12.6 心跳消费）.
    ///
    /// 实装层：JSONEncode WSOutgoingMessage → 写 underlying URLSessionWebSocketTask.send(.string(...)).
    /// 未连接 / 已 disconnect → throw WSError.notConnected.
    func send(_ message: WSOutgoingMessage) async throws

    /// 主动断开（用户 leave / app 切后台）；触发 close code 1000（client-initiated close）.
    /// 调用后 `messages` stream 终止（finish），caller 应取消 for-await 循环.
    func disconnect()

    /// fix-review round 2 P1 修复：room A→B / leave-rejoin 时复用同一 client 实例的
    /// reconnect/restart 接缝.
    ///
    /// **语义**：调用后 `messages` getter 返回**新的** AsyncStream（旧 stream 已被 disconnect finish；
    ///   下一次 caller 起 for-await 拿到的是这个新 stream）.
    ///
    /// **Story 12.2 落地路线**：`WebSocketClientImpl.prepareForReconnect()` 在内部清掉旧 task 状态
    ///   + 准备好新 stream/continuation（与 `connect(roomId:)` 串接：通常 caller 会调
    ///   `prepareForReconnect()` → `connect(roomId: next)` → `for await ... in client.messages`）.
    func prepareForReconnect()
}

extension WebSocketClient {
    /// 默认 no-op：对不需要重启 stream 的实装（例如未来某些一次性 client）保持向后兼容.
    /// `WebSocketClientMock` 与 Story 12.2 的 `WebSocketClientImpl` 都会 override.
    public func prepareForReconnect() {}

    /// fix-review round 4 P1 默认实现：**非原子**的兜底（仅给那些不可能并发 swap stream 的极简实装用）.
    /// 真实生产 / mock 实装**必须 override** 提供原子语义；这里 default 仅为编译期向后兼容.
    /// 实际生产用 `WebSocketClientImpl` 在锁内同时读两个字段；mock 在单线程语义下天然原子.
    public var currentStreamSnapshot: (stream: AsyncStream<WSMessage>, generation: Int) {
        (messages, streamGeneration)
    }
}

/// 服务端 → 客户端消息（按 §12.3 type 字段路由后的强类型 enum）.
///
/// **Story 12.1 仅覆盖 Epic 10 阶段 server-active 三种 case** + `unknown` fallback；
/// `member.joined` / `member.left` 由 **Story 12.4** 扩展（已落地）；
/// `pet.state.changed` 由 Story 15.2 扩展（已落地）；
/// `emoji.received` 由 Epic 17 / Story 17.x 扩展.
public enum WSMessage: Equatable, Sendable {
    /// `room.snapshot` —— 握手成功后必发的第一条 authoritative 消息（§12.1.3 钦定）.
    /// payload schema 见 §12.3 `room.snapshot` 小节字段表 / "client merge contract" 段.
    case roomSnapshot(RoomSnapshotPayload)

    /// `pong` —— 服务端响应 client `ping` 心跳（§12.3 `pong` 小节）.
    /// Story 12.6 心跳框架落地后处理；本 story `RealRoomViewModel` 不消费 pong（仅 log discard）.
    case pong(requestId: String)

    /// `error` —— 服务端运行时业务错误推送（§12.3 `error` 小节）.
    /// 节点 4 阶段服务端**不**主动推 error（除握手失败 close code 4001-4007 / 1011 / 1006，那些是 close frame，不走本 case）；
    /// Epic 11+ 业务流程的运行时状态错误推送会用本 case，本 story 仅占位 enum case 字段层就位.
    case error(code: Int, message: String, requestId: String)

    /// `member.joined` —— 房间内**其他**成员通过 HTTP join 加入房间事件（V1 §12.3 行 1994-2063）.
    /// payload 完整自包含展示字段（userId + nickname + avatarUrl + pet（nullable）），client 收到后
    /// 可直接 append 一条 roster entry，不需要二次拉取 snapshot.
    /// Story 12.4 落地；trigger 唯一来源 = HTTP `POST /api/v1/rooms/{roomId}/join` 加入事务成功提交后.
    case memberJoined(MemberJoinedPayload)

    /// `member.left` —— 房间内**其他**成员通过 HTTP leave 离开房间事件（V1 §12.3 行 2065-2101）.
    /// payload 精简为仅 userId（V1 §12.3 行 2097 钦定 leave 事件 client UX 不需要显示昵称）.
    /// Story 12.4 落地；trigger 唯一来源 = HTTP `POST /api/v1/rooms/{roomId}/leave` 退出事务成功提交后.
    case memberLeft(MemberLeftPayload)

    /// `pet.state.changed` —— 房间内任一成员（含发起者自己）通过 `POST /pets/current/state-sync` 触发状态变更（V1 §12.3 行 2212-2259）.
    /// payload 三字段（userId + petId + currentState）；client 收到后按 §12.3 client merge contract 字段级 merge：
    /// (a) `memberPetStates[userId]` 已存在 → 覆盖该字段为 `HomePetState(rawValue: currentState)` 映射后的状态
    /// (b) `memberPetStates[userId]` 不存在（理论不该发生 —— 表示 roster 与 server 状态严重不一致）→ ignore + log warn
    /// (c) `payload.userId == self` 自己的 self-broadcast 走同一路径（§5.2 self-broadcast 对称兜底前置条件，由 Story 15.4 落地自身侧 UI 驱动）
    /// Story 15.2 落地；trigger 唯一来源 = HTTP `POST /api/v1/pets/current/state-sync` 成功 UPDATE 之后 service 层广播.
    case petStateChanged(PetStateChangedPayload)

    /// Story 12.5 新增：client-internal 连接状态变更通知（**不**是 server-side 协议消息）.
    /// 由 `WebSocketClientImpl` 内部 reconnect 状态机 emit；vm 收到后写 `wsState`（三态映射）.
    /// **不**走 `WSMessageCodec.decode` 路径（codec 仅解码 server-side text frame，本 case 是 client-internal emit）.
    /// 与 vm-唯一-stream 接缝原则一致（vm 与 client 唯一通信通道是 `messages: AsyncStream<WSMessage>`，
    /// 12.1 锁定）；本 case 让 vm 在同一 stream 上一并感知连接状态，无需新增 publisher.
    case connectionStateChanged(WSConnectionState)

    /// 解码失败 fallback —— 不破坏 AsyncStream（与 epic Story 12.2 AC "edge: 服务端推未知 type
    /// → 解码失败 + log warning + 不破坏 stream" 一致）.
    case unknown(rawType: String)
}

/// Story 12.5 引入：WS 连接状态变更事件载荷（与 `WSMessage.connectionStateChanged(...)` 配套）.
///
/// 三态映射 `WSState`（vm 一行 switch 写完）；`reconnecting(attempt:)` 携带 attempt 字段为预留 ——
/// 节点 4 阶段 `RoomScaffoldView.wsStateLabel` 不展示 N（仅"正在重连…"），但保留字段让 Story 12.6 /
/// Epic 13 端到端验证想展示"第 N 次重连"时无字段层迁移成本.
///
/// 设计说明：
///   - **不**给 `disconnected` 加 `code: Int` 携带 close code —— vm 不需要 close code（business
///     decision 由 caller / UseCase 层处理；vm 仅渲染三态文字）.
///   - 由 `WebSocketClientImpl` reconnect 状态机内部 emit，vm 通过 `messages` stream 透明感知.
public enum WSConnectionState: Equatable, Sendable {
    /// connect 成功（首次 / reconnect 成功后） —— vm 写 `wsState = .connected`.
    case connected

    /// 第 N 次 reconnect in-flight（attempt 从 1 起算）—— vm 写 `wsState = .reconnecting`.
    case reconnecting(attempt: Int)

    /// terminal close（含主动 disconnect / 业务级拒绝 close 4001-4007 / 重连超 5 次失败 / 未知 close code 保守 terminal）.
    /// vm 写 `wsState = .disconnected`.
    case disconnected
}

/// `room.snapshot` payload（§12.3 schema）—— 仅本 story 用到的字段就位，
/// 节点 5 后 Epic 14 / Story 14.x 扩展 `equips` / `renderConfig` 等字段.
public struct RoomSnapshotPayload: Equatable, Sendable {
    public let room: RoomSnapshotRoomInfo
    public let members: [RoomSnapshotMember]

    public init(room: RoomSnapshotRoomInfo, members: [RoomSnapshotMember]) {
        self.room = room
        self.members = members
    }
}

public struct RoomSnapshotRoomInfo: Equatable, Sendable {
    public let id: String
    public let maxMembers: Int
    public let memberCount: Int

    public init(id: String, maxMembers: Int, memberCount: Int) {
        self.id = id
        self.maxMembers = maxMembers
        self.memberCount = memberCount
    }
}

public struct RoomSnapshotMember: Equatable, Sendable {
    public let userId: String
    public let nickname: String         // §12.3 `nickname` 字段；空字符串 = "server 不知道"语义（client merge contract）
    public let pet: RoomSnapshotPet?    // §12.3 `pet` 字段；null = pet-less authoritative 信号

    public init(userId: String, nickname: String, pet: RoomSnapshotPet?) {
        self.userId = userId
        self.nickname = nickname
        self.pet = pet
    }
}

public struct RoomSnapshotPet: Equatable, Sendable {
    public let petId: String
    public let currentState: Int       // §12.3 节点 4 阶段固定 1（rest）；Epic 14 后真实 1/2/3

    public init(petId: String, currentState: Int) {
        self.petId = petId
        self.currentState = currentState
    }
}

// MARK: - Story 12.4 member.joined / member.left payload value types

/// `member.joined` payload（V1 §12.3 行 2003-2013 字段表）.
/// 完整 5 字段（userId + nickname + avatarUrl + pet（nullable））；
/// 与 server-side `MemberJoinedPayload` struct（snapshot.go:416-421）字段集合 1:1 对齐.
///
/// **关键约束**：V1 §12.3 行 2008 钦定 `nickname` 必非空字符串（与 `room.snapshot` placeholder 阶段
/// 允许空字符串的语义**不同** —— member.joined 在加入事务成功提交后触发，server 必有真实 nickname）；
/// `avatarUrl` 可空字符串 ""（节点 4 阶段头像未实装），但**不**为 null；
/// `pet = nil` 是 authoritative pet-less 信号（pet-less 账号下发 nil；否则下发 `{petId, currentState}` 完整 object）.
public struct MemberJoinedPayload: Equatable, Sendable {
    public let userId: String
    public let nickname: String
    public let avatarUrl: String
    public let pet: MemberJoinedPet?

    public init(userId: String, nickname: String, avatarUrl: String, pet: MemberJoinedPet?) {
        self.userId = userId
        self.nickname = nickname
        self.avatarUrl = avatarUrl
        self.pet = pet
    }
}

/// `member.joined.payload.pet` 子结构（V1 §12.3 行 2011-2012）.
/// 与 RoomSnapshotPet 字段集合一致；本 story 选独立类型而非 typealias —— 与 server 端 SnapshotPet 复用模式不同；
/// 留出后续 Epic 26 / 29 装备字段独立演进的空间（V1 §12.3 server 端 SnapshotPet 复用同一 struct，
/// client 端独立类型避免跨消息耦合；如未来字段演进路径相同可改为 typealias）.
public struct MemberJoinedPet: Equatable, Sendable {
    public let petId: String
    public let currentState: Int       // 节点 4 阶段固定 1（rest）

    public init(petId: String, currentState: Int) {
        self.petId = petId
        self.currentState = currentState
    }
}

/// `member.left` payload（V1 §12.3 行 2073-2080 字段表）.
/// 仅 1 字段 userId（V1 §12.3 行 2097 钦定 leave 事件 payload 精简 —— client UX 不需要显示昵称，
/// client 从已有 roster 查 nickname；UX 文案降级"有人离开"也可接受）.
/// 与 server-side `MemberLeftPayload` struct（snapshot.go:428-430）字段集合 1:1 对齐.
public struct MemberLeftPayload: Equatable, Sendable {
    public let userId: String

    public init(userId: String) {
        self.userId = userId
    }
}

// MARK: - Story 15.2 pet.state.changed payload value type

/// `pet.state.changed` payload（V1 §12.3 行 2223-2230 字段表）.
/// 三字段 userId + petId + currentState 全部必填（V1 §12.3 行 2250 "禁止 payload 为 {} 或缺任一字段"）；
/// `userId` / `petId` 是 BIGINT 字符串化（§2.5）；`currentState` 是 Int 枚举 1/2/3（节点 5 阶段 1=rest / 2=walk / 3=run）.
///
/// **本 struct 独立于 `RoomSnapshotPet` / `MemberJoinedPet`** —— 三者虽都映射相同业务字段（petId + currentState），
/// 但每条业务 WS 消息保留各自 payload struct 演进空间（与 Story 12.4 `MemberJoinedPet` 独立模式一致；
/// 跨消息不复用，避免后续装备字段 / 房间字段独立演进时打架）.
///
/// **不引用 `HomePetState`**：payload struct 留 wire 层 Int 值；HomePetState 映射在 ViewModel.applyPetStateChanged
/// 层做，与 `RoomSnapshotPet.currentState: Int` 同精神（未知值降级 `.rest` 在 ViewModel 层做，codec 容忍 server 未来扩展）.
public struct PetStateChangedPayload: Equatable, Sendable {
    public let userId: String
    public let petId: String
    public let currentState: Int

    public init(userId: String, petId: String, currentState: Int) {
        self.userId = userId
        self.petId = petId
        self.currentState = currentState
    }
}
