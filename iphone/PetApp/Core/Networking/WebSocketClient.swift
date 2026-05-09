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

    /// 主动断开（用户 leave / app 切后台）；触发 close code 1000（client-initiated close）.
    /// 调用后 `messages` stream 终止（finish），caller 应取消 for-await 循环.
    func disconnect()

    /// fix-review round 2 P1 修复：room A→B / leave-rejoin 时复用同一 client 实例的
    /// reconnect/restart 接缝.
    ///
    /// **语义**：调用后 `messages` getter 返回**新的** AsyncStream（旧 stream 已被 disconnect finish；
    ///   下一次 caller 起 for-await 拿到的是这个新 stream）.
    ///
    /// **本 story（12.1）阶段**：只有 mock 需要实装真正的 stream 重置；production `WebSocketClientImpl`
    ///   尚未落地，protocol 给默认 **no-op**（见 protocol extension）.
    ///
    /// **Story 12.2 落地路线**：`WebSocketClientImpl.prepareForReconnect()` 在内部清掉旧 task 状态
    ///   + 准备好新 stream/continuation（与 `connect(roomId:)` 串接：通常 caller 会调
    ///   `prepareForReconnect()` → `connect(roomId: next)` → `for await ... in client.messages`）.
    ///   届时本方法可以与 `connect(roomId:)` 合并，或保持独立接缝（让"准备 stream"与"拨号"解耦）.
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
/// `member.joined` / `member.left` 由 Story 12.4 扩展；
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
