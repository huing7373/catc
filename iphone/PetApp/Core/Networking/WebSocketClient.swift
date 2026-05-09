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
}

/// 服务端 → 客户端消息（按 §12.3 type 字段路由后的强类型 enum）.
///
/// **Story 12.1 仅覆盖 Epic 10 阶段 server-active 三种 case** + `unknown` fallback；
/// `member.joined` / `member.left` 由 **Story 12.4** 扩展（已落地）；
/// `pet.state.changed` 由 Epic 14 / Story 14.x 扩展；
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

    /// 解码失败 fallback —— 不破坏 AsyncStream（与 epic Story 12.2 AC "edge: 服务端推未知 type
    /// → 解码失败 + log warning + 不破坏 stream" 一致）.
    case unknown(rawType: String)
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
