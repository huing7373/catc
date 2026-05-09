# Story 12.2: WebSocketClient 封装（基于 URLSessionWebSocketTask）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## 故事定位（Epic 12 第 2 条 story）

这是 Epic 12「iOS - 房间页面 + WebSocket 客户端」的**第 2 条 story**。Story 12.1 已经把 `WebSocketClient` protocol 接缝 + `WSMessage` enum 最小集 + `WebSocketClientMock` + `RealRoomViewModel.subscribeRoomIdConnect` 路径 + `prepareForReconnect()` reconnect 接缝全部就位（详见 Story 12.1 落地实装 + r1~r6 fix-review lessons 沉淀）。本 story 落地**真实** `WebSocketClientImpl`：基于 `URLSessionWebSocketTask` 实装拨号 / 收发 / 关闭三层，让 RealRoomViewModel 能真实联通节点 4 阶段 server WS 网关（Epic 10 已 done：`/ws/rooms/{roomId}` 路由 + 心跳框架 + 房间快照下发）。

**本 story 落地后立即解锁**：

- Story 12.3（房间快照解析 + 成员列表渲染）—— 真实 server 推 `room.snapshot` 后由 RealRoomViewModel 解析（解析路径 Story 12.1 已落地，本 story 提供真实 wire 触发）
- Story 12.4（成员加入 / 离开 WS 消息处理）—— 在本 story 真实 stream 上扩展 `member.joined` / `member.left` case
- Story 12.5（自动重连指数退避）—— 在本 story `WebSocketClientImpl.disconnect()` / 异常断开 / `prepareForReconnect()` + `connect(roomId:)` 接缝上加 reconnect 状态机
- Story 12.6（心跳 ping/pong）—— 在本 story `WebSocketClientImpl.send(_:)` 路径上加 30s 定时 ping
- Story 12.7（CreateRoom / JoinRoom / LeaveRoom UseCase）—— 通过 AppContainer 注入 `WebSocketClientImpl` 实例 + 在 RootView wire 中替换 `bind(webSocketClient:)` 的 nil 实参为真实 client

**本 story 的"实装"动作**（一句话概括）：

1. **新建 `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift`**：实装 `class WebSocketClientImpl: WebSocketClient`，构造注入 `baseURL: URL` + `tokenProvider: () -> String?`（从 Keychain 读 token；与 APIClient 同模式），暴露 `connect(roomId:) async throws`（拨号 `wss?://{host}/ws/rooms/{roomId}?token={url-encoded}`）+ `send(_ message: WSOutgoingMessage) async throws`（仅 `ping` 一种 case，本 story 范围）+ `disconnect()` + `messages: AsyncStream<WSMessage>`（与 protocol 一致；getter 返回当前 stream，`disconnect()` 后 finish）+ `prepareForReconnect()`（清旧 task / 旧 stream + 重建 fresh stream + continuation）；
2. **新建 `iphone/PetApp/Core/Networking/WSMessageCodec.swift`**：统一 incoming JSON → `WSMessage` enum decode + outgoing `WSOutgoingMessage`（仅 `ping` case + `requestId`）encode；按 V1接口设计 §12.3 通用信封（`type` / `requestId` / `payload` / `ts`）+ §12.2 通用信封（`type` / `requestId` / `payload`）字段层语义；未识别 `type` → `.unknown(rawType: String)` 不破坏 stream + log warn；
3. **新建 `iphone/PetApp/Core/Networking/WSError.swift`**：定义 `WSError` enum（`connectionFailed(underlying: Error?)` / `closedByServer(code: Int, reason: String)` / `tokenMissing` / `invalidURL` / `notConnected` / `decodingFailed(rawType: String?)`）；
4. **扩展 `WebSocketClient` protocol**：新增 `connect(roomId: String) async throws` 方法（必须在 `messages` 被消费之前调）+ `send(_ message: WSOutgoingMessage) async throws`（Story 12.6 心跳消费）；`WebSocketClientMock` 同步扩展（mock 记录 `connectCallArgs: [(roomId: String)]` / `sentMessages: [WSOutgoingMessage]`，不做真实拨号）；
5. **AppContainer wire**：`AppContainer` 持 `let webSocketClient: WebSocketClient`（默认 init 中实例化 `WebSocketClientImpl(baseURL:, tokenProvider:)`，tokenProvider 闭包从 keychainStore 读 `kKeyAuthToken`）；测试 init 重载允许注入 mock；**不**改 RootView wire（继续传 nil；Story 12.7 才把真实 client 注入到 RealRoomViewModel.bind —— **本 story 仅就位 AppContainer 字段，不改 bind 实参**）；
6. **单元测试 ≥5 case**（XCTest only + 手动 mock URLSession 不引第三方）：connect URL 构造正确、send 序列化正确、incoming text frame → AsyncStream yield WSMessage 正确、connect 失败 → 抛 WSError、未知 type → unknown case 不破坏 stream。

**关键路径**：

- 拨号路径走 `URLSession.shared.webSocketTask(with: URLRequest)` —— **必须**以 URLRequest 形式传，不是 URL；理由：URL 形式的 `webSocketTask(with: url)` API 不支持自定义 header（节点 4 阶段 query token 已足够，但保 URLRequest 路径是为了 future 切到 header bearer 时不破 ABI）。token 通过 query `?token=...` 传递（与 V1 §12.1 钦定一致）；URL-encode 用 `URLComponents.queryItems`（自动 URL-encode）而**不是**手工字符串拼接（避免 token 中 `+ / =` 漏 encode）。
- `messages` AsyncStream 走 single producer 模型：`WebSocketClientImpl` 内部持 `var currentStream: AsyncStream<WSMessage>` + `var continuation: AsyncStream<WSMessage>.Continuation`（计算属性 backed by var，与 `WebSocketClientMock` 同精神 —— 让 `prepareForReconnect()` 可 swap 出新 stream）；`connect(roomId:)` 拨号成功后启动**长生命周期** receive task（`Task { while ... await task.receive() ... continuation.yield(parse(text)) ... }`），异常 / close 时 `continuation.finish()`。
- `disconnect()` 走 client-initiated close：`urlSessionWebSocketTask.cancel(with: .normalClosure, reason: nil)` → close code 1000（按 V1 §12.1 close code 表 1000 行）+ continuation.finish() + 清 receiveTask；**不**重置 stream（`prepareForReconnect()` 才重置）。
- server-initiated close（4001 token 失效 / 4003 user not in room / 4004 room not found / 4005 心跳超时 / 4007 leave / 1011 内部错误）：URLSessionWebSocketTask delegate 或 receive 抛错 → 解析 close code → continuation.yield(.unknown(rawType: "close:\(code)")) **不要**做（错误的）—— 而是 continuation.finish() 让 stream 终止 + 通过 `WSError.closedByServer(code:reason:)` 让上一次未完成的 `connect` / `send` 抛错；本 story **不**实装"按 close code 分类自动重连"逻辑（**Story 12.5 范围**）。
- `prepareForReconnect()` 实装：cancel 旧 receiveTask、close 旧 underlying task（best-effort，可能已 closed）、新建 stream + continuation；**不**自动重连（caller 显式调 `connect(roomId:)`）。
- `WSOutgoingMessage` 是新增 enum（仅 client → server）—— 节点 4 阶段范围只有 `case ping(requestId: String)`；与 incoming `WSMessage`（server → client）严格分离避免类型膨胀。

**不涉及**（红线）：

- **不**实装心跳定时（30s 定时 ping —— **Story 12.6 范围**）；本 story 仅提供 `send(.ping(requestId:))` API + ping outgoing 编码，**不**起定时器；
- **不**实装自动重连（指数退避 1s/2s/4s/8s/16s/32s/max60s —— **Story 12.5 范围**）；本 story `disconnect` 后 stream finish，由 caller（Story 12.5 RealRoomViewModel reconnect 状态机）显式调 `prepareForReconnect()` + `connect(roomId:)`；
- **不**实装 App 进后台 / 回前台 disconnect / reconnect（**Story 12.5 范围**）；
- **不**改 RealRoomViewModel（Story 12.1 已落地解析路径；本 story 仅在 protocol 上扩 `connect / send` 方法 —— RealRoomViewModel 既有 bind 路径继续工作，**不需要**改 bind 签名调用 connect，因为本 story 仍**不**在 RootView wire 中注入真实 client，wire 改动留给 Story 12.7）；
- **不**实装 `member.joined` / `member.left` 增量 mutate（**Story 12.4 范围**）；本 story `WSMessage` enum **不**新增 case；
- **不**改 server 端任何文件（server epic 10 已完整支持 ws + 心跳 + snapshot；本 story 是纯 client 实装）；
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3）；
- **不**实装"WS 路径冷启时从本地持久化恢复 roomId"（V1 §12.1 提到的 reconnect 场景从 UserDefaults / Keychain 读最近一次 roomId —— **Story 12.5 范围**）；
- **不**改 AppState 字段集 / hydrate 路径；
- **不**预先实装 `RoomRealtimeRepository` / `RoomRealtimeViewModelBridge`（iOS 架构 §9.2 建议）—— 节点 4 阶段 ViewModel 直接订阅 client.messages 已足够，本 story 沿用 Story 12.1 的实装节奏，**不**预 over-design。

## Story

As an iOS 开发,
I want 一个基于 `URLSessionWebSocketTask` 的真实 `WebSocketClientImpl`，配套 connect / send / disconnect / messages stream / prepareForReconnect 五件套,
So that RealRoomViewModel 在 Story 12.7 注入此 client 后能与 server WS 网关真实联通，Story 12.3 / 12.4 / 12.5 / 12.6 在此 client 上扩展业务消息消费 + 重连 + 心跳.

## Acceptance Criteria

> **AC 编号体系**：AC1 是 `WebSocketClient` protocol 扩展（新增 `connect / send`）；AC2 是 `WSOutgoingMessage` enum；AC3 是 `WSError` enum；AC4 是 `WSMessageCodec` 编解码（incoming + outgoing）；AC5 是 `WebSocketClientImpl` 真实实装（拨号 + receive 循环 + send + disconnect + prepareForReconnect）；AC6 是 `WebSocketClientMock` 扩展（连接 / 发送记录）；AC7 是 AppContainer wire（持 webSocketClient 字段 + 默认实例化）；AC8 是 单元测试 ≥5 case；AC9 是 build verify；AC10 是 Deliverable 清单。

### AC1 — WebSocketClient protocol 扩展（新增 `connect` + `send`）

**修改文件**：`iphone/PetApp/Core/Networking/WebSocketClient.swift`

在 Story 12.1 已落地的最小契约（`messages: AsyncStream<WSMessage> { get }` + `disconnect()` + `prepareForReconnect()`）基础上**新增**两个方法：

```swift
public protocol WebSocketClient: AnyObject, Sendable {
    var messages: AsyncStream<WSMessage> { get }

    /// Story 12.2 新增：拨号到指定 roomId 的 WS 网关.
    ///
    /// 实装层（WebSocketClientImpl）：
    /// 1. 用 tokenProvider() 取最新 Bearer token（nil → throw WSError.tokenMissing）
    /// 2. 拼接 `{ws_scheme}://{host}/ws/rooms/{roomId}?token={url-encoded}`（V1 §12.1）
    /// 3. URLSession.shared.webSocketTask(with: URLRequest) → resume()
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

    func disconnect()

    func prepareForReconnect()
}

extension WebSocketClient {
    public func prepareForReconnect() {}  // Story 12.1 既有 default no-op，保留
}
```

**关键决策 1**：`connect(roomId:)` 设计成 **throws async 而非 throws Void** —— 让上层（Story 12.7 UseCase）能 await 直到拨号成功 + 第一个 frame 可读，避免"调 connect 后立即调 send 但 underlying task 还在 connecting"的 race；实装层在 `URLSessionWebSocketTask.resume()` 后**立即**返回（不等 server snapshot），上层根据 server 主动推 `room.snapshot`（V1 §12.1.3 钦定握手成功后必发）作为"WS 真实就绪"信号。

**关键决策 2**：`send(_:)` 接收 `WSOutgoingMessage` enum 而非泛型 `Encodable`，因为节点 4 阶段 client → server 仅 ping，强类型 enum 避免误发非法 type；Story 17.x 加 `emoji.send` 时扩展 enum case 即可。

**关键决策 3**：**不**新增 `connectionState` publisher 字段（如 connected / connecting / disconnected）—— RealRoomViewModel 已经持 `wsState: WSState`（Story 12.1 落地），由 ViewModel 在 `connect / disconnect / 收到第一条 snapshot / 收到 close` 路径上**显式**写；client protocol 层保持 thin，避免双 source of truth。

**对应 Tasks**: Task 1.1

### AC2 — 新建 WSOutgoingMessage enum

**新建文件**：`iphone/PetApp/Core/Networking/WSOutgoingMessage.swift`

```swift
// WSOutgoingMessage.swift
// Story 12.2 AC2: client → server WS 消息 enum（与 incoming WSMessage 严格分离）.
//
// 设计原则：
//   - 节点 4 阶段 V1接口设计 §12.2 钦定 client → server 仅 `ping` 一种合法消息（emoji.send 由 Story 17.1 锚定加入）
//   - 与 incoming WSMessage 分离：incoming 是 server-controlled（含 unknown fallback），outgoing 是 client-controlled（必须严格符合协议）
//   - case 数固定 → enum 而非 protocol；Codable 由 WSMessageCodec 处理而**不**让 enum 自身 conform Codable
//     （让 codec 集中控制 wire JSON 形态 —— payload 始终为 `{}`，requestId 字段层固定写出）
//
// V1 §12.2 ping 信封：
//   { "type": "ping", "requestId": "<optional>", "payload": {} }
//
// 心跳间隔 / 触发由 Story 12.6 决定，本 story 仅提供消息构造 + 编码.

import Foundation

public enum WSOutgoingMessage: Equatable, Sendable {
    /// V1 §12.2 ping —— Story 12.6 心跳框架消费.
    /// `requestId`：客户端可生成（推荐 `"ping_<seq>"` 或 `"ping_<ts_ms>"`）；省略时用空字符串 `""`（server 回带空 requestId）.
    case ping(requestId: String)
}
```

> **关键决策**：保留 `requestId` 即使是 ping —— 让 Story 12.6 心跳框架能用 RTT 做 jitter 监控（不强求；但接缝就位避免 12.6 落地时回工 12.2 enum）。requestId 字符串约束（≤64 字符 / 客户端定义格式）由 codec 层强制（生成时用 UUID 或 ts_ms 都满足；不在 enum 层做范围 enforce）。

**对应 Tasks**: Task 2.1

### AC3 — 新建 WSError enum

**新建文件**：`iphone/PetApp/Core/Networking/WSError.swift`

```swift
// WSError.swift
// Story 12.2 AC3: WebSocketClient 错误类型 —— 与 APIError 同精神（终态 case 集 + Equatable + Sendable）.
//
// 设计原则：
//   - 错误来源分类：tokenMissing / invalidURL（构造期），connectionFailed（拨号期），closedByServer（连接后被 close），notConnected（未拨号或已断开），decodingFailed（incoming frame 解码失败）
//   - 与 V1 §12.1 close code 表对齐：closedByServer.code 直接存 server emit 的 close code（4001 / 4003 / 4004 / 4005 / 4006 / 4007 / 1011 / 1006），让 Story 12.5 reconnect 状态机按 code 分类决策（详见 V1 §12.1 close code 表）
//   - **不**做"按 close code 自动重连"决策（**Story 12.5 范围**）；本 story 仅暴露 code 让上层决策
//   - underlying error 用 Error existential 而非具体 URLError —— 让 future 切到非 URLSession 实装时不破契约

import Foundation

public enum WSError: Error, Equatable, Sendable {
    /// `WebSocketClientImpl.connect(roomId:)` 时 tokenProvider() 返回 nil / 空字符串.
    /// caller（Story 12.7）应触发"无效 token 静默重新登录"流程（参考 ADR-0008 v2 + Story 5.4）.
    case tokenMissing

    /// connect URL 构造失败（baseURL invalid / roomId 含非法字符 —— 节点 4 阶段不应发生，防御性 case）.
    case invalidURL

    /// 拨号失败（DNS 解析失败 / TLS 握手失败 / connection refused / network unreachable 等）.
    /// underlying 是 URLError 时 Equatable 比较仅比 case，不比 underlying（避免 Equatable 难做）.
    case connectionFailed(underlyingDescription: String)

    /// 连接成功后被 server 主动 close（V1 §12.1 close code 表 4001 / 4003 / 4004 / 4005 / 4006 / 4007 / 1000 / 1001 / 1011）.
    /// caller 按 code 决策：4001 → 重新登录；4003 / 4004 / 4006 / 4007 → 业务级拒绝不重连；4005 / 1006 / 1011 → transient 应重连（指数退避，Story 12.5）；1000 / 1001 → 主动关闭路径.
    case closedByServer(code: Int, reason: String)

    /// `send(_:)` / 复用同 client 时 underlying URLSessionWebSocketTask 已 closed / nil.
    case notConnected

    /// incoming frame 解码失败（非法 JSON / 信封字段缺失）—— 仅在 codec 抛错时出现；正常 case 已被 `.unknown(rawType:)` 兜底.
    case decodingFailed(rawType: String?)
}
```

> **关键决策 1**：`connectionFailed` 携带 `underlyingDescription: String`（不是 `Error` existential）—— Equatable 易实现 + Sendable 易满足 + 单测断言简单（`XCTAssertEqual(err, .connectionFailed(underlyingDescription: ...))` 不需要 `as? URLError` cast）。production caller 通过 string description 写 log 即可（不需要 underlying error 做后续业务决策）。

> **关键决策 2**：`closedByServer.code: Int` 而非 `enum CloseCode { case 4001 / 4003 / ... }` —— 节点 4 阶段 close code 表 10 个值 + 未来可能扩展，且 1006 是 RFC reserved（不在表内但 client runtime 可能合成），用 Int 接住所有可能值更鲁棒；Story 12.5 reconnect 状态机内部可定义 `enum CloseDecision` 把 `Int` 映射到决策。

**对应 Tasks**: Task 3.1

### AC4 — 新建 WSMessageCodec（incoming decode + outgoing encode）

**新建文件**：`iphone/PetApp/Core/Networking/WSMessageCodec.swift`

```swift
// WSMessageCodec.swift
// Story 12.2 AC4: WS 消息编解码层 —— incoming JSON text frame → WSMessage enum；outgoing WSOutgoingMessage → JSON text frame.
//
// 设计原则：
//   - 与 APIClient 内部 JSONDecoder / JSONEncoder 同精神：每次新建 instance（避免 Sendable 歧义；详见 APIClient.swift makeDecoder()）
//   - incoming 路径走"按 type 字段分发"策略：先解一个 envelope 拿 type，再按 type 解 payload；不引入 polymorphic Codable trick（Codable 对 enum w/ associated value 的 decode 写起来繁琐易错）
//   - V1 §12.3 通用信封：{ "type": str, "requestId": str, "payload": {...}, "ts": int64 }
//   - V1 §12.2 通用信封：{ "type": str, "requestId": str, "payload": {...} }（无 ts）
//   - 未识别 type → return .unknown(rawType: ...) + log warn（不抛错；不破坏 stream）
//   - payload 解码失败 → return .unknown(rawType: type) + log warn（同样不破坏 stream；防 server malformed payload 把房间页搞崩）
//
// 节点 4 阶段 incoming 已知 type 集合：room.snapshot / pong / error（Epic 10 钦定）
// 节点 4 阶段 outgoing 已知 type 集合：ping（V1 §12.2）

import Foundation
import os.log

public enum WSMessageCodec {

    // MARK: - Incoming

    /// 解析 server → client 消息 text frame（UTF-8 JSON string）.
    /// - 已知 type 解码成功 → 返回对应 WSMessage case
    /// - 已知 type 解码失败（payload schema mismatch）→ 返回 .unknown(rawType: type) + log warn
    /// - 未知 type → 返回 .unknown(rawType: rawType) + log warn
    /// - 信封自身解码失败（非 JSON / 缺 type 字段）→ 返回 .unknown(rawType: nil) + log error
    public static func decode(_ text: String) -> WSMessage {
        guard let data = text.data(using: .utf8) else {
            os_log(.error, "WSMessageCodec: text frame UTF-8 conversion failed")
            return .unknown(rawType: nil ?? "")
        }
        let envelope: WSEnvelope
        do {
            envelope = try makeDecoder().decode(WSEnvelope.self, from: data)
        } catch {
            os_log(.error, "WSMessageCodec: envelope decode failed: %{public}@", String(describing: error))
            return .unknown(rawType: "")
        }
        switch envelope.type {
        case "room.snapshot":
            do {
                let payload = try makeDecoder().decode(RoomSnapshotEnvelope.self, from: data).payload
                return .roomSnapshot(payload)
            } catch {
                os_log(.error, "WSMessageCodec: room.snapshot payload decode failed: %{public}@", String(describing: error))
                return .unknown(rawType: "room.snapshot")
            }
        case "pong":
            return .pong(requestId: envelope.requestId)
        case "error":
            do {
                let payload = try makeDecoder().decode(ErrorEnvelope.self, from: data).payload
                return .error(code: payload.code, message: payload.message, requestId: envelope.requestId)
            } catch {
                os_log(.error, "WSMessageCodec: error payload decode failed: %{public}@", String(describing: error))
                return .unknown(rawType: "error")
            }
        default:
            os_log(.info, "WSMessageCodec: unknown server type: %{public}@", envelope.type)
            return .unknown(rawType: envelope.type)
        }
    }

    // MARK: - Outgoing

    /// 编码 client → server 消息 → text frame（UTF-8 JSON string）.
    /// 节点 4 阶段仅 ping case；future 扩展加 case 即可.
    public static func encode(_ message: WSOutgoingMessage) throws -> String {
        let json: [String: Any]
        switch message {
        case .ping(let requestId):
            json = [
                "type": "ping",
                "requestId": requestId,
                "payload": [String: Any]()  // V1 §12.2 ping payload 固定空对象
            ]
        }
        let data = try JSONSerialization.data(withJSONObject: json, options: [.sortedKeys])
        guard let text = String(data: data, encoding: .utf8) else {
            throw WSError.decodingFailed(rawType: "ping")
        }
        return text
    }

    // MARK: - Internal envelope DTOs

    /// V1 §12.3 通用信封最小投影 —— 仅取分发用 type / requestId 两字段（payload / ts 由后续 envelope 各自解）.
    private struct WSEnvelope: Decodable {
        let type: String
        let requestId: String
    }

    /// room.snapshot 整体信封 —— 与 V1 §12.3 schema 严格对齐.
    private struct RoomSnapshotEnvelope: Decodable {
        let payload: RoomSnapshotPayloadDTO

        enum CodingKeys: String, CodingKey { case payload }
    }

    /// payload 层 DTO —— 与 Story 12.1 落地的 RoomSnapshotPayload 字段对齐（Equatable / Sendable struct 已就位）.
    /// 这里**不**直接 conform Codable 在 RoomSnapshotPayload 上，避免 Story 12.1 既有类型被 codec 实装耦合
    /// （让 RoomSnapshotPayload 保持纯值类型；DTO 层负责 wire 解码 + 字段映射）.
    private struct RoomSnapshotPayloadDTO: Decodable {
        let room: RoomInfoDTO
        let members: [MemberDTO]

        struct RoomInfoDTO: Decodable {
            let id: String
            let maxMembers: Int
            let memberCount: Int
        }

        struct MemberDTO: Decodable {
            let userId: String
            let nickname: String
            let pet: PetDTO?  // V1 §12.3：null = pet-less authoritative 信号

            struct PetDTO: Decodable {
                let petId: String
                let currentState: Int
            }
        }

        var payload: RoomSnapshotPayload {
            RoomSnapshotPayload(
                room: RoomSnapshotRoomInfo(id: room.id, maxMembers: room.maxMembers, memberCount: room.memberCount),
                members: members.map { dto in
                    RoomSnapshotMember(
                        userId: dto.userId,
                        nickname: dto.nickname,
                        pet: dto.pet.map { p in
                            RoomSnapshotPet(petId: p.petId, currentState: p.currentState)
                        }
                    )
                }
            )
        }
    }

    /// error 整体信封.
    private struct ErrorEnvelope: Decodable {
        let payload: ErrorPayloadDTO

        struct ErrorPayloadDTO: Decodable {
            let code: Int
            let message: String
        }
    }

    private static func makeDecoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        // 节点 4 阶段 WS 信封无 Date 字段；保持默认策略
        return decoder
    }
}
```

> **关键决策 1**：用 `JSONSerialization` 编码 outgoing（而非 Codable）—— ping 只有 3 字段固定形态，JSONSerialization 不需要写 Encodable struct + 不需要担心 key encoding strategy，单一职责更明确。Story 17.x 加 `emoji.send` 时若字段复杂可改用 Codable struct，本 story YAGNI。

> **关键决策 2**：incoming envelope 用 **DTO + 映射** 模式（不让 RoomSnapshotPayload 直接 conform Decodable）—— 让 Story 12.1 已落地的值类型保持"纯展示用"语义，wire 形态变化（如 future 加 `avatarUrl` 字段）只改 DTO 不破 ViewModel 层；与 APIClient 内 envelope DTO 同精神。

> **关键决策 3**：未识别 type / payload 解码失败均返 `.unknown(rawType: ...)` 而**不**抛错 —— 节点 4 阶段 client 容错策略钦定（V1 §12.3 末注 + Story 10.1 既有规则）："对未识别的 type 走安全忽略 + log warn 路径，不因未识别消息 close 连接 / crash app"。

> **关键决策 4**：**不**实装 `member.joined` / `member.left` / `pet.state.changed` / `emoji.received` decode 路径（Story 12.4 / 14.x / 17.x 范围）；本 story `decode` 函数仅 handle 节点 4 阶段三种 case + unknown fallback。当 server 推后续 epic 才支持的 type（如 `member.joined`）时，本 story 实装会 yield `.unknown(rawType: "member.joined")` + log warn，**不**破坏 stream —— Story 12.4 落地时再扩展 case。

> **关键决策 5**：`ts: int64` 字段在 `WSEnvelope` 中**不**解（仅 type / requestId）—— 节点 4 阶段 client 不消费 ts（V1 §12.3 钦定 ts 用于 client 日志关联 + UI 排序，本 story / Story 12.3 不做时序排序，YAGNI）；future 需要时直接给 envelope 加 `let ts: Int64?` 字段。

**对应 Tasks**: Task 4.1, 4.2, 4.3

### AC5 — WebSocketClientImpl 真实实装（拨号 + receive 循环 + send + disconnect + prepareForReconnect）

**新建文件**：`iphone/PetApp/Core/Networking/WebSocketClientImpl.swift`

```swift
// WebSocketClientImpl.swift
// Story 12.2 AC5: WebSocketClient protocol 真实实装 —— 基于 URLSessionWebSocketTask.
//
// 设计原则：
//   - URLSessionWebSocketTask 是 iOS 13+ 原生 API（与 ADR-0002 §3.1 钦定 standard library / 不引第三方一致）
//   - 拨号路径用 URLRequest（不是 URL）—— 留 future header bearer 接缝；当前节点 4 阶段 query token
//   - receive 循环走单一长任务：connect 时启动，disconnect / error 时 cancel + finish stream
//   - prepareForReconnect 与 WebSocketClientMock 同语义：cancel 旧 task + 清旧 stream + 新建 fresh stream/continuation
//   - 所有 actor-protected state（receiveTask / underlying task / continuation）走 actor isolation
//     （class final + @unchecked Sendable + private 字段 + 在 actor / Task 内 mutate；与 WebSocketClientMock 同模式）
//
// 不实装：
//   - 心跳定时（Story 12.6）—— 本 story 仅暴露 send(.ping(...)) API
//   - 自动重连（Story 12.5）—— 本 story disconnect 后 stream finish；caller 显式 prepareForReconnect + connect 才复活
//   - 后台 / 前台切换（Story 12.5）

import Foundation
import os.log

public final class WebSocketClientImpl: WebSocketClient, @unchecked Sendable {

    // MARK: - Dependencies

    /// host-only baseURL（与 APIClient 同源；不含 /api/v1 前缀；scheme 转换 http→ws / https→wss 在内部处理）.
    private let baseURL: URL

    /// token 来源闭包 —— 拨号时同步取最新 token（保 token 刷新后立即生效）.
    /// 闭包内通常调 keychainStore.get(.kKeyAuthToken)；nil / 空字符串 → connect 抛 WSError.tokenMissing.
    private let tokenProvider: () -> String?

    /// URLSession 注入入口 —— 默认 URLSession.shared；测试可注入 mock URLSession + URLProtocol stub.
    private let urlSession: URLSession

    // MARK: - State

    /// 当前 underlying WS task（ connect 时赋值；disconnect / receive 异常时 cancel + nil）.
    private var underlyingTask: URLSessionWebSocketTask?

    /// receive 长任务 —— connect 后启动 for-await receive 循环；disconnect / prepareForReconnect 时 cancel.
    private var receiveTask: Task<Void, Never>?

    /// 当前消息 stream + continuation（与 WebSocketClientMock 同模式：var backed by computed `messages`）.
    private var currentStream: AsyncStream<WSMessage>
    private var currentContinuation: AsyncStream<WSMessage>.Continuation

    // MARK: - WebSocketClient protocol

    public var messages: AsyncStream<WSMessage> {
        currentStream
    }

    public init(
        baseURL: URL,
        tokenProvider: @escaping () -> String?,
        urlSession: URLSession = .shared
    ) {
        self.baseURL = baseURL
        self.tokenProvider = tokenProvider
        self.urlSession = urlSession
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
        // 4. webSocketTask + resume
        let task = urlSession.webSocketTask(with: request)
        self.underlyingTask = task
        task.resume()
        // 5. 启动 receive 长任务
        startReceiveLoop(task: task)
        // 注：拨号成功 vs 真实建连成功**有时序差异** —— resume() 立即返回，但 server 端
        // upgrade + token 校验 + room_members 校验 + push room.snapshot 走 §12.1 校验顺序后才完成；
        // 拨号期 close（如 4001 token 失效）会通过 receive 循环捕获并 yield close error 信号.
        os_log(.debug, "WebSocketClientImpl: connect issued for roomId=%{public}@", roomId)
    }

    public func send(_ message: WSOutgoingMessage) async throws {
        guard let task = underlyingTask, task.state == .running else {
            throw WSError.notConnected
        }
        let text = try WSMessageCodec.encode(message)
        try await task.send(.string(text))
    }

    public func disconnect() {
        // client-initiated close（V1 §12.1 close code 1000 normalClosure）
        underlyingTask?.cancel(with: .normalClosure, reason: nil)
        underlyingTask = nil
        receiveTask?.cancel()
        receiveTask = nil
        currentContinuation.finish()
        os_log(.debug, "WebSocketClientImpl: disconnect issued")
    }

    public func prepareForReconnect() {
        // 与 WebSocketClientMock.prepareForReconnect 同语义：cancel 旧资源 + 新建 fresh stream
        underlyingTask?.cancel(with: .normalClosure, reason: nil)
        underlyingTask = nil
        receiveTask?.cancel()
        receiveTask = nil
        let (stream, cont) = AsyncStream<WSMessage>.makeStream()
        self.currentStream = stream
        self.currentContinuation = cont
        os_log(.debug, "WebSocketClientImpl: prepareForReconnect (fresh stream issued)")
    }

    // MARK: - Internal helpers

    /// V1 §12.1 连接 URL 构造：`{ws_scheme}://{host}/ws/rooms/{roomId}?token={url-encoded}`.
    /// scheme 转换：http → ws / https → wss（与 baseURL 协议一致）.
    private func makeWSURL(roomId: String, token: String) throws -> URL {
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
        // query：URLComponents 会自动 url-encode token 中的 + / = 等字符
        components.queryItems = [URLQueryItem(name: "token", value: token)]
        guard let url = components.url else { throw WSError.invalidURL }
        return url
    }

    /// 启动 receive 长任务：循环 await task.receive() → decode → yield 到 stream.
    /// 异常 / close → finish stream + nil out underlyingTask.
    private func startReceiveLoop(task: URLSessionWebSocketTask) {
        receiveTask = Task { [weak self] in
            guard let self else { return }
            while !Task.isCancelled {
                do {
                    let frame = try await task.receive()
                    switch frame {
                    case .string(let text):
                        let message = WSMessageCodec.decode(text)
                        self.currentContinuation.yield(message)
                    case .data(let data):
                        // V1 §12.2 / §12.3：text frame only；binary frame 不应出现 —— 兜底兼容（解为 UTF-8 string 走同路径）
                        if let text = String(data: data, encoding: .utf8) {
                            let message = WSMessageCodec.decode(text)
                            self.currentContinuation.yield(message)
                        } else {
                            os_log(.error, "WebSocketClientImpl: binary frame non-UTF-8")
                        }
                    @unknown default:
                        os_log(.error, "WebSocketClientImpl: unknown WS frame type")
                    }
                } catch {
                    // 拨号 / 连接期异常 → finish stream（caller 通过 messages stream 终结感知）
                    // 不抛回 connect() —— connect() 已经 return；只让 stream 终结
                    os_log(.error, "WebSocketClientImpl: receive failed: %{public}@", String(describing: error))
                    self.currentContinuation.finish()
                    self.underlyingTask = nil
                    return
                }
            }
        }
    }

    deinit {
        underlyingTask?.cancel(with: .normalClosure, reason: nil)
        receiveTask?.cancel()
        currentContinuation.finish()
    }
}
```

> **关键决策 1**：`tokenProvider: () -> String?` 是闭包而非直接持 `KeychainStoreProtocol` —— 解耦 client 与 keychain 实装；测试时直接传 `{ "test_token" }` 闭包（不需要 mock keychain）；production 时 `AppContainer` 闭包内调 `keychainStore.get(...)`。

> **关键决策 2**：`urlSession: URLSession = .shared` 默认共享 session —— 与 APIClient 同模式（共享 session 利于连接池复用 + iOS 系统级网络优化）；测试可注入 stub URLSession（带 URLProtocol fake）。

> **关键决策 3**：scheme 转换在 client 内做（http → ws / https → wss）—— APIContainer 持的 baseURL 是 host-only http(s) 形态，不要求 caller 提前转换；与 V1 §12.1 钦定 "host 与 HTTP 接口同 host" 吻合。

> **关键决策 4**：receive 循环异常 / close 时**仅** finish stream + 清 underlyingTask，**不**重新 connect、**不**改 wsState —— wsState 由 RealRoomViewModel 管理（ViewModel 是 source of truth），重连状态机在 Story 12.5 落地。

> **关键决策 5**：用 `task.state == .running` 判断 send 可发性 —— 比 `task.closeCode == .invalid`（默认 invalid 表 not-yet-closed）语义清晰；`URLSessionTask.State.running` 含 connecting + connected 两种 active 子态，节点 4 阶段无需细分。

> **关键决策 6**：`@unchecked Sendable` 标注 —— class final + private mutable state + 单线程 caller 假设（同 WebSocketClientMock）。Swift 6 strict concurrency 时再切到 actor，本 story 保持节奏一致。

> **关键决策 7**：**不**实装 close code 解析（如从 `URLSessionWebSocketTask.closeCode` / `closeReason` 读取并通过 stream 投递 `WSError.closedByServer(code:reason:)`）—— 节点 4 阶段 RealRoomViewModel 不消费 close code；Story 12.5 reconnect 状态机才需要 close code 信号。**接缝预留**：当前实装在 receive catch 块仅 finish stream + log；Story 12.5 落地时可在该 catch 块处读 `underlyingTask?.closeCode` / `closeReason` 后向 stream 投递（或暴露 `closeCode publisher` 字段）。本 story **不**预先实装该接缝避免 over-design，但**必须**在 Tasks / Dev Notes 显式记录此 gap 让 Story 12.5 dev 知道 hook 点。

**对应 Tasks**: Task 5.1, 5.2, 5.3, 5.4, 5.5

### AC6 — WebSocketClientMock 扩展（connect / send 记录）

**修改文件**：`iphone/PetApp/Core/Networking/WebSocketClientMock.swift`

在 Story 12.1 已落地的 `WebSocketClientMock`（含 `messages` / `disconnect` / `prepareForReconnect` / `emit` / `didDisconnect` / `prepareForReconnectCallCount`）基础上**新增**：

```swift
public final class WebSocketClientMock: WebSocketClient, @unchecked Sendable {
    // ... 既有字段 ...

    /// Story 12.2 AC6 新增：记录 connect 调用（让单测断言"connect 被调 + roomId 正确"）.
    public private(set) var connectCallArgs: [String] = []

    /// Story 12.2 AC6 新增：记录 send 调用（让单测断言"ping 被发出 + requestId 一致"）.
    public private(set) var sentMessages: [WSOutgoingMessage] = []

    /// Story 12.2 AC6 新增：connect / send 的可控 stub 错误（默认 nil = 不抛错）.
    /// 测试场景：模拟"token 失效"路径让 connect 抛 WSError.tokenMissing.
    public var connectError: WSError?
    public var sendError: WSError?

    public func connect(roomId: String) async throws {
        connectCallArgs.append(roomId)
        if let err = connectError { throw err }
    }

    public func send(_ message: WSOutgoingMessage) async throws {
        sentMessages.append(message)
        if let err = sendError { throw err }
    }

    // ... 既有 disconnect / prepareForReconnect / emit / init 不动 ...
}
```

> **关键决策**：mock 的 `connect` 不真实拨号、不自动 emit 消息 —— 让测试用例通过 `mock.emit(.roomSnapshot(...))` 显式驱动消息流，控制权完全在测试方手里。这与 Story 12.1 的 mock 模式一致。

**对应 Tasks**: Task 6.1

### AC7 — AppContainer wire（持 webSocketClient 字段 + 默认实例化）

**修改文件**：`iphone/PetApp/App/AppContainer.swift`

新增 `let webSocketClient: WebSocketClient` 字段 + 默认 init 中实例化 `WebSocketClientImpl(baseURL:, tokenProvider:)`：

```swift
public final class AppContainer: ObservableObject {
    // ... 既有字段 ...

    /// Story 12.2 AC7 新增：全 App 共享的 WebSocketClient.
    /// Story 12.7 LeaveRoomUseCase / JoinRoomUseCase 落地后通过 container.webSocketClient 注入到
    /// RealRoomViewModel.bind(appState:webSocketClient:)；本 story 仅就位字段，**不**改 RootView wire
    /// （RootView 仍传 nil；Story 12.7 才把真实 client 注入）.
    ///
    /// 默认 init：实例化 WebSocketClientImpl(baseURL:, tokenProvider:)，
    ///   - baseURL 与 APIClient 共享（resolveDefaultBaseURL，host-only）
    ///   - tokenProvider 闭包从 keychainStore 读 kKeyAuthToken；与 APIClient 同源 token.
    /// 测试 init 重载允许注入 mock client.
    public let webSocketClient: WebSocketClient

    public convenience init() {
        let baseURL = AppContainer.resolveDefaultBaseURL(from: Bundle.main)
        let keychainStore = KeychainServicesStore()
        // ... 既有 baseAPIClient / sink / wrappedAPIClient 构造保留不变 ...

        // Story 12.2 AC7：实例化默认 WebSocketClientImpl
        let wsClient = WebSocketClientImpl(
            baseURL: baseURL,
            tokenProvider: { [weak keychainStore] in
                guard let store = keychainStore else { return nil }
                return try? store.get(.kKeyAuthToken)
            }
        )

        // 既有 #if DEBUG / #else 分支保留 —— 在 self.init(...) 调用中追加 webSocketClient 实参
        #if DEBUG
        // ... uitestMockStepRepo / uitestMockHealth 既有 ...
        self.init(
            apiClient: wrappedAPIClient,
            keychainStore: keychainStore,
            unauthorizedHandlerSink: sink,
            webSocketClient: wsClient,  // ← 新增
            healthProvider: uitestMockHealth,
            uiTestMockStepRepository: uitestMockStepRepo
        )
        #else
        self.init(
            apiClient: wrappedAPIClient,
            keychainStore: keychainStore,
            unauthorizedHandlerSink: sink,
            webSocketClient: wsClient  // ← 新增
        )
        #endif
    }

    #if DEBUG
    public init(
        apiClient: APIClientProtocol,
        keychainStore: KeychainStoreProtocol = KeychainServicesStore(),
        unauthorizedHandlerSink: UnauthorizedHandlerSink = UnauthorizedHandlerSink(),
        webSocketClient: WebSocketClient? = nil,  // ← 新增（默认 nil 让既有调用零回归；nil 时 init 内自建）
        dateProvider: DateProvider = DefaultDateProvider(),
        healthProvider: HealthProvider? = nil,
        uiTestMockStepRepository: StepRepositoryProtocol? = nil
    ) {
        self.apiClient = apiClient
        self.errorPresenter = ErrorPresenter()
        self.keychainStore = keychainStore
        self.sessionStore = SessionStore()
        self.unauthorizedHandlerSink = unauthorizedHandlerSink
        // Story 12.2 AC7：webSocketClient 默认 nil 时 fallback 到 baseURL fallback + tokenProvider from keychainStore
        self.webSocketClient = webSocketClient ?? WebSocketClientImpl(
            baseURL: AppContainer.resolveDefaultBaseURL(from: Bundle.main),
            tokenProvider: { [weak keychainStore] in
                guard let store = keychainStore else { return nil }
                return try? store.get(.kKeyAuthToken)
            }
        )
        // ... 既有字段赋值不变 ...
    }
    #else
    // release 路径同模式；webSocketClient 参数 + fallback ?? 默认实例化
    #endif
}
```

> **关键决策 1**：webSocketClient 字段类型是 `WebSocketClient` protocol 而非 `WebSocketClientImpl` 具体类 —— 测试 / future 切实装时不破依赖。

> **关键决策 2**：默认 init 中**不**直接 wire 到 RootView 的 RealRoomViewModel —— 字段就位但 wire 路径留给 Story 12.7（LeaveRoomUseCase / JoinRoomUseCase 落地后 wire）；本 story 不改 RootView wire 的红线由"Story 12.2 仅交付 WebSocketClientImpl + AppContainer 字段，不改业务 ViewModel 注入"原则锁定。

> **关键决策 3**：`tokenProvider` 闭包用 `[weak keychainStore]` —— 防 client 持有 store 强引用导致 deinit 链路异常（store 是 stable singleton 的话弱引用安全；强引用也无 leak 风险，但弱引用语义更明确）。

**对应 Tasks**: Task 7.1, 7.2

### AC8 — 单元测试 ≥5 case（XCTest only + 手动 mock URLSession 不引第三方）

**新建测试文件**：`iphone/PetAppTests/Core/Networking/WebSocketClientImplTests.swift`（与 APIClientTests 同目录平级；目录已存在）

**测试基础设施约束**（与 APIClientTests / RealRoomViewModelTests 同精神 + ADR-0002 §3.1）：

- 仅依赖 stdlib（XCTest + @testable import PetApp）
- 不引 ViewInspector / SnapshotTesting / Mockingbird
- mock URLSession 走 URLProtocol stub 模式（可参考 APIClientTests 既有的 `MockURLProtocol` 实装；或者本 story 新建一个简化版仅 mock `webSocketTask(with:)` 路径）
- 直接断言 `WebSocketClientImpl` 的可观察行为：connect 成功后 messages 流可用、send 调用 underlying task.send、URL 构造正确

**必须覆盖的测试 case**（≥5 case）：

```swift
// case#1 happy: connect URL 构造正确（baseURL "http://localhost:8080" + roomId "1234567" + token "abc+def" 含特殊字符）
//   → URL 应是 "ws://localhost:8080/ws/rooms/1234567?token=abc%2Bdef"（"+" url-encode 为 "%2B"）
//   实现：用 URLSession 替换 mock，断言 URLSessionWebSocketTask 创建时的 URLRequest.url.absoluteString
//   关键：验证 scheme http→ws、path 拼接、token URL-encode 三个独立路径

// case#2 happy: connect 后 incoming text frame → AsyncStream yield WSMessage
//   → mock URLSessionWebSocketTask.receive() 返回 .string("{\"type\":\"room.snapshot\",\"requestId\":\"\",\"payload\":{...},\"ts\":...}")
//   → for-await client.messages 拿到 .roomSnapshot(payload) + payload.members.count == 期望值
//   关键：验证 receive 循环 + WSMessageCodec.decode 集成路径

// case#3 happy: send(.ping(requestId: "ping_001")) → underlying task.send 被调用 + 序列化字符串符合 V1 §12.2 ping 信封
//   断言：URLSessionWebSocketTask.send 收到的 .string 内容含 "type":"ping" / "requestId":"ping_001" / "payload":{}
//   关键：验证 outgoing encoding + V1 §12.2 信封字段层契约

// case#4 edge: connect 失败（tokenProvider() 返回 nil）→ 抛 WSError.tokenMissing
//   构造：WebSocketClientImpl(baseURL:, tokenProvider: { nil }, urlSession:)
//   await client.connect(roomId: "1234567") → 应抛 WSError.tokenMissing
//   关键：验证 token check 早退路径（不发起 underlying task）

// case#5 edge: incoming 未识别 type 字符串 → AsyncStream yield .unknown(rawType: "unknown_type") + 不破坏 stream
//   mock URLSessionWebSocketTask.receive() 第一次返回 .string({"type":"unknown_type",...}) → yield .unknown(rawType: "unknown_type")
//   再次 receive() 返回 .string({"type":"room.snapshot",...}) → 仍 yield .roomSnapshot(...)（stream 没断）
//   关键：验证未知 type 容错（V1 §12.3 末注 + Story 10.1 既有规则）

// case#6（可选 + 推荐）: prepareForReconnect 后 messages 是 fresh stream
//   先 emit 一些消息（旧 stream）→ 调 prepareForReconnect → 旧 stream finish → for-await client.messages 拿到的是新 stream
//   关键：验证 leave-rejoin / room A→B 路径接缝就位（与 WebSocketClientMock 同语义）

// case#7（可选）: disconnect 后 send 抛 WSError.notConnected
//   connect → disconnect → await send(...) → 应抛 WSError.notConnected
//   关键：验证 send 在 task.state != .running 时早退
```

**用 await Task.yield() 让 AsyncStream 派发**：incoming 路径走 `await task.receive() → continuation.yield → for-await client.messages`，单测必须 `await Task.yield()`（或用 `withCheckedContinuation` 等 wait helper）让事件循环跑一轮（与 RealRoomViewModelTests 同 lesson：published-derived-state-needs-publisher-subscription）。

> **关键决策 1**：mock URLSession 实装路径推荐**新建一个简化版 fake**（如 `class FakeURLSessionWebSocketTask: URLSessionWebSocketTask { override func resume() {...}; override func receive() async throws -> URLSessionWebSocketTask.Message {...}; override func send(_:) async throws {...}; override func cancel(with:reason:) {...} }`）+ `class FakeURLSession: URLSession { override func webSocketTask(with: URLRequest) -> URLSessionWebSocketTask { ... } }` —— 比 URLProtocol stub 更直接，因为 URLSession 内置的 URLProtocol 钩子对 webSocketTask 路径支持有限。subclass override 是 `URLSession` / `URLSessionWebSocketTask` 测试 hook 的标准 iOS 模式。

> **关键决策 2**：unit test 覆盖**不**包含"server 端真 WS 联调"路径（那是 Epic 13 跨端集成测试 Story 13.1 范围，不在 Epic 12 单 story 内）；本 story 只测客户端孤立行为。

**对应 Tasks**: Task 8.1, 8.2

### AC9 — Build verify

**必须通过**：

```bash
bash iphone/scripts/build.sh --test
```

- xcodegen regen 成功（新文件自动加入 PetApp / PetAppTests target；与 Story 12.1 / 37.x 同模式）
- xcodebuild 编译通过（无 warning）
- 所有单测通过（含本 story 新增 ≥5 case + 既有 RealRoomViewModelTests / RoomViewScaffoldTests / APIClientTests 等不破）
- UITest 通过（既有 RoomUITests / NavigationUITests / HomeUITests 等不破 —— 本 story **不**新增 UITest）

**ios-simulator MCP 验证**（CLAUDE.md "iOS UI 验证（必跑）"）：

```bash
1. bash iphone/scripts/build.sh
2. install_app(app_path: iphone/build/DerivedData/Build/Products/Debug-iphonesimulator/PetApp.app)
3. launch_app(bundle_id: "com.zhuming.pet.app", terminate_running: true)
4. ui_view 验证：
   - Home tab idle 态 / Home tab inRoom 态切换正常（与 Story 12.1 落地后视觉一致）
   - RoomScaffoldView 仍显示 wsStateLabel "已断开"（因 RootView wire 仍传 nil；本 story 不改 wire）
   - 既有 4 mock 成员渲染正常（webSocketClient = nil 路径渲染 RoomScaffoldDefaults）
5. ui_describe_all 验证 a11y identifier 全集（roomIdDisplay / wsStateLabel / roomMember_0/1/2/3）不变
6. **不**做"WS 真实联调"（无 server 端联调环境的 dev 环境跑不通；联调 case 由 Epic 13 Story 13.1 在跨端测试中验证）
```

**关键约束**：本 story 改动**不应**影响任何现有 UI 视觉 —— webSocketClient 字段加在 AppContainer 但 RootView wire 不改（仍传 nil 给 RealRoomViewModel.bind），所以视觉路径与 Story 12.1 完全一致；任何视觉 regression 即代码 bug。

**对应 Tasks**: Task 9.1

### AC10 — Deliverable 清单

本 story 完成后必须有：

**新建文件（5 个）**：

- `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift`（AC5）
- `iphone/PetApp/Core/Networking/WSMessageCodec.swift`（AC4）
- `iphone/PetApp/Core/Networking/WSError.swift`（AC3）
- `iphone/PetApp/Core/Networking/WSOutgoingMessage.swift`（AC2）
- `iphone/PetAppTests/Core/Networking/WebSocketClientImplTests.swift`（AC8）

**修改文件（3 个）**：

- `iphone/PetApp/Core/Networking/WebSocketClient.swift`（AC1 加 connect + send 方法）
- `iphone/PetApp/Core/Networking/WebSocketClientMock.swift`（AC6 加 connect / send 记录字段 + 实装）
- `iphone/PetApp/App/AppContainer.swift`（AC7 加 webSocketClient 字段 + 默认实例化）

**Xcode project 更新**：xcodegen regen 自动同步新文件到 PetApp / PetAppTests target（`bash iphone/scripts/build.sh` 内置触发）。

**对应 Tasks**: Task 10.1

## Tasks / Subtasks

- [x] **Task 1.1** — WebSocketClient protocol 扩展 connect + send（AC1）
- [x] **Task 2.1** — 新建 WSOutgoingMessage.swift（AC2）
- [x] **Task 3.1** — 新建 WSError.swift（AC3）
- [x] **Task 4.1** — 新建 WSMessageCodec.swift（incoming envelope DTO + 分发 by type）（AC4）
- [x] **Task 4.2** — WSMessageCodec.encode（outgoing ping）（AC4）
- [x] **Task 4.3** — RoomSnapshotPayloadDTO → RoomSnapshotPayload 映射 + ErrorEnvelope 解码（AC4）
- [x] **Task 5.1** — 新建 WebSocketClientImpl.swift 骨架（init + 字段 + protocol conform）（AC5）
- [x] **Task 5.2** — WebSocketClientImpl.connect(roomId:)：token check + URL 构造 + URLSession.webSocketTask + resume + 启动 receiveTask（AC5）
- [x] **Task 5.3** — WebSocketClientImpl.send(_:)：state check + WSMessageCodec.encode + task.send（AC5）
- [x] **Task 5.4** — WebSocketClientImpl.disconnect / prepareForReconnect（AC5）
- [x] **Task 5.5** — receive 循环 startReceiveLoop（for await receive → decode → yield；异常 → finish + log）（AC5）
- [x] **Task 6.1** — WebSocketClientMock 扩展 connectCallArgs / sentMessages / connectError / sendError + connect/send 实装（AC6）
- [x] **Task 7.1** — AppContainer 加 webSocketClient 字段（AC7）
- [x] **Task 7.2** — AppContainer 默认 init / 测试 init 重载实例化 WebSocketClientImpl + tokenProvider 闭包（AC7）
- [x] **Task 8.1** — WebSocketClientImplTests case#1-#5 单测（AC8 必须）
- [x] **Task 8.2**（推荐）— WebSocketClientImplTests case#6 prepareForReconnect / case#7 disconnect-then-send 单测（AC8 可选）
- [x] **Task 9.1** — `bash iphone/scripts/build.sh --test` 全绿 + ios-simulator MCP 视觉无回归（AC9）
- [x] **Task 10.1** — Deliverable 清单核对 + xcodegen regen 自动同步（AC10）

## Dev Notes

### 关键文档锚定

- `docs/宠物互动App_总体架构设计.md` — Tech Stack（iOS Swift+SwiftUI / WebSocket）
- `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §9.1 / §9.2 / §9.3（WebSocket 子系统职责 / 客户端对象建议 / 重连策略；本 story 实装 WebSocketClient + 占位 prepareForReconnect 接缝；Repository / Bridge 留给 Story 12.4 / 12.7）
- `docs/宠物互动App_V1接口设计.md` §12.1（连接地址 + URL 构造 + 握手成功流程 + close code 表 + 服务端校验顺序）
- `docs/宠物互动App_V1接口设计.md` §12.2（client → server 通用信封 + ping 字段表）
- `docs/宠物互动App_V1接口设计.md` §12.3（server → client 通用信封 + room.snapshot / pong / error 字段表 + client merge contract）
- `_bmad-output/implementation-artifacts/12-1-房间页面-swiftui-骨架.md`（前置 story；本 story 在其留下的 protocol + Mock 接缝上扩展真实实装）
- `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md`（iOS 测试栈钦定 XCTest only + 手写 mock）
- `_bmad-output/implementation-artifacts/decisions/0009-ios-stack.md`（iPhone 工程目录决策；详细工程结构）
- `_bmad-output/implementation-artifacts/decisions/0010-iphone-appstate.md`（AppState 注入规则；本 story AppContainer wire webSocketClient 字段 follow §3.1 注入规则）
- `_bmad-output/implementation-artifacts/decisions/0011-ws-stack.md`（server 端 WS 库选型 ADR；client 实装无直接 ADR，本 story 用 Apple 原生 URLSessionWebSocketTask 与 ADR-0002 §3.1 standard library 路径一致）

### Source tree 涉及位置

```
iphone/
  PetApp/
    Core/
      Networking/
        APIClient.swift              # 既有，本 story 不动；参考其 baseURL / keychain 注入模式
        AuthBoundaryAPIClient.swift  # 既有，本 story 不动
        Endpoint.swift               # 既有，本 story 不动
        URLSessionProtocol.swift     # 既有；本 story 不引用（直接用 URLSession）
        WebSocketClient.swift        # 修改（AC1：扩 connect + send）
        WebSocketClientMock.swift    # 修改（AC6：加 connectCallArgs / sentMessages + 实装）
        WebSocketClientImpl.swift    # 新建（AC5）
        WSMessageCodec.swift         # 新建（AC4）
        WSError.swift                # 新建（AC3）
        WSOutgoingMessage.swift      # 新建（AC2）
    App/
      AppContainer.swift             # 修改（AC7：加 webSocketClient 字段）
      RootView.swift                 # **不**改（webSocketClient wire 留 Story 12.7）
    Features/
      Room/
        ViewModels/
          RealRoomViewModel.swift    # **不**改（Story 12.1 已就位 bind + appState 订阅；本 story 不动 ViewModel）
  PetAppTests/
    Core/
      Networking/
        APIClientTests.swift         # 既有，本 story 参考其 URLProtocol mock 模式
        WebSocketClientImplTests.swift # 新建（AC8）
    Features/
      Room/
        RealRoomViewModelTests.swift # 既有，Story 12.1 落地，本 story 不动
```

### Testing 标准摘要

- **单测**（PetAppTests target）：XCTest only；mock URLSession 走 subclass override（FakeURLSession + FakeURLSessionWebSocketTask）；不引 ViewInspector / SnapshotTesting / Mockingbird；用 `await Task.yield()` 让 receive 循环 yield 派发
- **不需要 UITest**（本 story 不改 UI 视觉）—— UI 链路由 Story 12.3 / 12.7 在 wire 真实 client 后再加 UITest
- **build verify**：`bash iphone/scripts/build.sh --test` 全绿（编译 + 单测 + UITest 三层）

### Project Structure Notes

- 文件位置严格按 `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §3 + §9
- WebSocketClientImpl / WSMessageCodec / WSError / WSOutgoingMessage 全放 `Core/Networking/` 与 APIClient 同级（与 iOS 架构 §3.1 钦定一致）
- **不**引入 `RoomRealtimeRepository` / `RoomRealtimeViewModelBridge`（架构 §9.2 建议；Story 12.1 同决策延续）—— 节点 4 阶段 ViewModel 直接订阅 client.messages 已足够；后续 epic 落地时如需再演进
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3 钦定）

### 与 unified project structure 对齐 / variances

- 与 Story 12.1 落地的 `Core/Networking/{WebSocketClient.swift, WebSocketClientMock.swift}` 同目录扩展，新增 4 个文件不破现有模块边界
- AppContainer 字段扩展走 Story 5.x / 8.x 同模式（init 加参数 + 默认 nil fallback + 默认 init 内实例化）
- **variance**：架构 §9.2 建议引入 `RoomRealtimeRepository` 中间层 —— 本 story 暂时**不**引入（与 Story 12.1 同决策）；如未来 Story 12.4 / 12.7 / Epic 14 落地后发现 ViewModel 太肥再演进；记入 tech debt log

### Previous story intelligence（必读 lessons）

> **以下 lessons 来自 Story 12.1 的 6 轮 codex review 沉淀（commits 8e5f182 / 791d942 / 等）+ Epic 37 retrospective §2.3 + `docs/lessons/` —— 本 story 实装 WebSocketClientImpl 时**逐条对照**避免重蹈：

#### 来自 Story 12.1 fix-review（同 epic 直接前序）

1. **`2026-05-09-published-subscription-dropfirst-and-room-switch-roster-reset-12-1-r1.md`** — `Published` 订阅起步**不要**预设 lastObservedRoomId 为当前值（否则 restored in-room session 路径下 nil→A 转换信号被识别为 A→A no-op 丢失）；`subscribeRoomIdConnect` 字段保持默认 nil + 第一条 emission 走 (nil, A) connect 分支。**本 story 不改 RealRoomViewModel，但 WebSocketClientImpl.connect 设计上必须是 idempotent + 可重入**：caller 可能多次调 connect（如 reconnect 状态机），同一 client 复用时调用顺序是 disconnect → prepareForReconnect → connect，**不**支持"connect 已经 connected 的 client"路径（应抛 WSError.notConnected 或类似）。

2. **`2026-05-09-ws-client-reuse-needs-stream-restart-and-empty-room-id-must-align-with-dispatcher-12-1-r2.md`** — AsyncStream 是 single-shot：`continuation.finish()` 后该 stream 不能复活；复用同 client 接收新消息**必须**显式 `prepareForReconnect()` 重建 stream + continuation。本 story `WebSocketClientImpl.prepareForReconnect()` 必须严格 follow 此语义（cancel 旧 task + 新建 fresh stream/continuation）—— 与 WebSocketClientMock 同行为，否则 Story 12.5 reconnect 状态机会踩同样的坑。

3. **`2026-05-09-stale-snapshot-discard-by-room-id-12-1-r3.md`** — `room.snapshot` 解析路径必须校验 `payload.room.id == lastObservedRoomId`，否则 A→B 切换时旧 stream 的 stale snapshot 会污染新房间。**本 story 不改 RealRoomViewModel 既有解析逻辑，但 WebSocketClientImpl 在 receive 循环中**不**做 roomId 校验**（client 层不持房间状态；ViewModel 层是 source of truth）—— WebSocketClientImpl 只负责"按字面 yield 收到的 frame 解码后的 message"，让 RealRoomViewModel 自己用已就位的 lastObservedRoomId 路径丢弃 stale。

4. **`2026-05-09-snapshot-host-must-not-infer-from-position-12-1-r4.md`** — snapshot path 下 isHost 一律置 false（"未知 host"占位语义）。本 story **不**实装 host 字段；与 RealRoomViewModel 已落地策略一致，无回归风险。

5. **`2026-05-09-bind-replace-must-disconnect-old-client-12-1-r5.md`** — `bind()` 替换 webSocketClient instance 必须先 disconnect 旧 client + cancel 旧 task。**本 story 不改 RealRoomViewModel.bind**，但 WebSocketClientImpl 自身**接受被替换**的语义：caller 持有的 client 实例可能在生命周期切换时被丢弃，此时 deinit 必须保证 underlying task / receiveTask / continuation 都被清掉（已在 AC5 deinit 实装）。

6. **`2026-05-09-same-instance-rebind-must-true-noop-12-1-r6.md`** — same-instance rebind 必须真正 no-op（不 cancel 旧 task / 不 restart receive）。**本 story 不改 bind**；WebSocketClientImpl 自身的 connect 调用保 idempotent —— 同一 roomId 多次调 connect 应**不**重新拨号（或者必须先 disconnect + prepareForReconnect 才能再 connect；选第二种更明确，节点 4 阶段语义易理解）。建议实装：connect 时若 underlyingTask != nil 且 state == .running → throw WSError 或 noop；当前 AC5 实装只走"直接 resume + 替换 underlyingTask 引用"路径，会把旧 task 泄漏，**dev 落地时建议加 guard：connect 已连接时直接 return / throw**。

#### 来自 Server 端 Epic 10 lessons（间接相关）

7. **`2026-05-06-ws-error-dual-semantics-and-heartbeat-close-code.md`** — error 消息双重语义：响应类带回 requestId / 主动推送类固定 ""；client 解析层应把 error 与 pong 同等对待。WSMessageCodec 实装时 `error` case 已正确传 envelope.requestId（本 story AC4 已设计）。

8. **`2026-05-06-ws-frozen-examples-and-close-code-collision.md`** — V1 §3 全局错误码 1006 / 1008 / 1009 与 close code 数字段冲突 → close frame 限定 4xxx 段（4001-4007）+ 1xxx 限定 1000 / 1001 / 1011。本 story `WSError.closedByServer.code: Int` 需在 future Story 12.5 reconnect 状态机中按此分类（4xxx 业务级不重连 / 1xxx + 4005 transient 重连）。

#### 来自 Epic 37 retrospective §2.3（ViewModel 层 lessons —— 仍需关注）

9. **`onappear-vs-task-sync-bind-before-first-paint`**（37.8 r2） — bind 必须在 first paint 之前同步完成。本 story **不**改 RootView .onAppear wire，与既有路径一致。

10. **`coordinator-must-mirror-loaded-home-room-state`**（37.3）— 本 story 不动 coordinator。

11. **`scaffold-bypass-viewmodel-seam`**（37.11）— 本 story 不动 RoomScaffoldView。

#### 通用 Swift / Combine lessons

12. **`2026-04-25-swift-explicit-import-combine.md`** — Combine import 必须显式（不依赖 SwiftUI transitive）。本 story 新建文件**不**引 Combine（仅 Foundation + os.log）；AppContainer 既有 import Combine 不动。

### References

- [Source: docs/宠物互动App_总体架构设计.md] — iOS Swift+SwiftUI / WebSocket
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#9.1] — WebSocket 子系统职责
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#9.2] — 客户端对象建议（WebSocketClient + RoomRealtimeRepository + RoomRealtimeViewModelBridge）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#9.3] — 重连策略（Story 12.5 落地时锚定，本 story 不实装）
- [Source: docs/宠物互动App_V1接口设计.md#12.1] — WebSocket 连接地址 + 握手成功流程 + close code 表 + 服务端校验顺序
- [Source: docs/宠物互动App_V1接口设计.md#12.2] — client → server 通用信封 + ping 字段表（送达本 story 重点）
- [Source: docs/宠物互动App_V1接口设计.md#12.3] — server → client 通用信封 + room.snapshot / pong / error 字段表 + client merge contract
- [Source: _bmad-output/implementation-artifacts/12-1-房间页面-swiftui-骨架.md] — 前置 story（含 protocol + Mock 接缝 + r1~r6 fix-review lessons）
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.1] — 测试栈 XCTest only + standard library 优先
- [Source: _bmad-output/implementation-artifacts/decisions/0010-iphone-appstate.md#3.1] — AppState 注入规则（本 story AppContainer.webSocketClient 字段 follow 同模式）
- [Source: _bmad-output/implementation-artifacts/decisions/0011-ws-stack.md] — server 端 ws 库 ADR（client 端用 Apple URLSessionWebSocketTask）
- [Source: docs/lessons/2026-05-09-ws-client-reuse-needs-stream-restart-and-empty-room-id-must-align-with-dispatcher-12-1-r2.md] — AsyncStream single-shot + prepareForReconnect 必须重建 stream（本 story `WebSocketClientImpl.prepareForReconnect` 严格遵循）
- [Source: docs/lessons/2026-05-09-bind-replace-must-disconnect-old-client-12-1-r5.md] — bind 替换 client 必须 tear down 旧资源（本 story client 自身 deinit 路径同精神）

### Latest tech information

- **URLSessionWebSocketTask**：iOS 13+ 原生 API，本 story 核心依赖
  - `webSocketTask(with: URLRequest)` ：URLRequest 形式拨号（支持自定义 header；当前节点 4 阶段不用 header bearer，但保接缝）
  - `resume()` ：异步拨号，立即返回（不阻塞）
  - `receive(completionHandler:)` / `receive() async throws -> URLSessionWebSocketTask.Message`：异步读 frame；message 是 `.string(String)` 或 `.data(Data)`
  - `send(_ message: URLSessionWebSocketTask.Message) async throws`：异步写 frame
  - `cancel(with: URLSessionWebSocketTask.CloseCode, reason: Data?)`：主动 close；CloseCode 含 `.normalClosure` (1000) / `.goingAway` (1001) 等枚举值
  - `state: URLSessionTask.State`：含 `.suspended` / `.running` / `.canceling` / `.completed` 四态
- **AsyncStream.makeStream()**：Swift 5.9+ 标准工厂方法，返回 `(stream: AsyncStream<Element>, continuation: Continuation)`；与 Story 12.1 WebSocketClientMock 同 API，本 story `WebSocketClientImpl` 沿用
- **URLComponents.queryItems**：自动 URL-encode（`+` → `%2B` / `/` → `%2F` / `=` → `%3D` 等），是 V1 §12.1 钦定 token URL-encode 路径的标准实装
- **iOS 17+ Observation framework（@Observable）**：本 story **不**采用（与 Story 37.x / 12.1 决策一致：保 @MainActor + ObservableObject + @Published 模式）

### Project context reference

`docs/lessons/` 内本 story 必读的 lessons（按重要性排序）：

- `2026-05-09-ws-client-reuse-needs-stream-restart-and-empty-room-id-must-align-with-dispatcher-12-1-r2.md` —— prepareForReconnect 必须重建 stream（实装 AC5 时严格遵循）
- `2026-05-09-bind-replace-must-disconnect-old-client-12-1-r5.md` —— 替换持有资源时必须 tear down 旧资源（client deinit 实装时遵循）
- `2026-05-09-same-instance-rebind-must-true-noop-12-1-r6.md` —— same-instance rebind 真正 no-op（client.connect 同一 roomId 时考虑 idempotent）
- `2026-05-06-ws-error-dual-semantics-and-heartbeat-close-code.md` —— error 双重语义；本 story WSMessageCodec 已设计 envelope.requestId 透传
- `2026-04-25-swift-explicit-import-combine.md` —— Combine import 必须显式（本 story 不引 Combine 但保险起见提醒）

`_bmad-output/implementation-artifacts/decisions/` 内本 story 必读 ADR：

- `0002-ios-stack.md` —— 测试栈 XCTest only + standard library 优先
- `0010-iphone-appstate.md` —— AppState 注入规则（AppContainer 字段扩展模式）
- `0011-ws-stack.md` —— server WS ADR（参考其取舍逻辑，client 端不冲突）

### 开放问题（dev 落地时如有疑问可决策或 raise）

1. **同一 client 重复 connect 的语义？** —— AC1 / AC5 当前实装走"直接 resume + 替换 underlyingTask 引用"路径会泄漏旧 task。**建议 dev 落地时加 guard**：`connect()` 开头判断 `underlyingTask?.state == .running` → 直接 return（idempotent）；或抛 WSError（提醒 caller 必须先 disconnect）。选第一种更宽容，但需在 Tasks 显式记录。Story 12.1 r6 lesson 倾向"same-instance rebind 真正 no-op"原则，建议本 story 也走"已 connected 时 return / log"路径。

2. **拨号期 close 的错误传递？** —— `connect(roomId:)` resume() 后立即返回，不等 server 端 upgrade 完成；如果 server 立即 close 4001（token 失效）/ 4003（不在房间）/ 4004（房间不存在），错误是通过 receive 循环抛错（receiveTask catch 块）→ continuation.finish() 让 stream 终止。**问题**：caller 调 connect() 时不会感知此错误（已 return），只能通过 messages stream 终结感知。**当前设计**：messages stream finish 后 caller 判断"为什么 finish" 需要额外信号（如 closeCode publisher）—— 但本 story 范围**不**实装该信号（Story 12.5 范围）；当前 caller（Story 12.1 RealRoomViewModel.startConsumingMessages）的 for-await loop 在 stream finish 后退出，wsState 切回 disconnected（已就位），UX 路径是"WS 进入 disconnected 态"—— 暂可接受。Story 12.5 reconnect 状态机落地时 dev 评估是否要在 client protocol 加 closeCode publisher。

3. **token 在 query 而非 header 的安全考量？** —— V1 §12.1 钦定 query token，与 server 端实装一致；缺点是 token 可能落入服务器访问日志（即使 prod 是 wss TLS 加密）。本 story 严格 follow 协议，不偏离；future 切到 header bearer 时 V1 协议需相应升级（一次性改 client + server）。

4. **WSMessageCodec 是否要做"严格 type 校验"（如 type 字段必须匹配 `^[a-z0-9.]+$` 才允许 dispatch）？** —— V1 §12.2 钦定 type 字段值约束（只允许 `[a-z0-9.]`），但 client 解析层无需做格式校验（已经收到 server emit 的消息，server 自己保证）；当前 AC4 实装只按 type 字符串相等做 dispatch，未识别走 unknown，**建议 dev 落地时不加额外校验**避免 over-engineering。

5. **测试用例 case#1 验证 URL 构造时如何拿到 underlying URLRequest？** —— 推荐在 FakeURLSession.webSocketTask(with: URLRequest) 中记录传入 URLRequest 到 mock 字段（如 `var lastRequest: URLRequest?`），测试断言 `mock.lastRequest?.url?.absoluteString == "ws://localhost:8080/ws/rooms/1234567?token=abc%2Bdef"`。这是标准 iOS subclass override 测试模式。

6. **是否要在本 story 实装 WebSocketClientImpl 的"close code 解析"接缝？** —— AC5 关键决策 7 钦定**不**实装（Story 12.5 范围）；但 receive 循环 catch 块的 log 中可以记录 `underlyingTask?.closeCode.rawValue` / `closeReason`，方便 dev 调试 + 后续 Story 12.5 接管时有日志依据。建议 dev 落地时**加 log 不加 publisher**。

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m] (Opus 4.7 1M context)

### Debug Log References

- 首轮 build 失败：AppContainer 用 `[weak keychainStore]` 闭包捕获 `KeychainStoreProtocol`，但 protocol 未 `: AnyObject` —— 切到 strong-capture（store 由 container 持，生命周期一致，安全）
- 首轮 test 编译失败：原本 fake 直接 subclass `URLSessionWebSocketTask` —— `init()` 被 NS_UNAVAILABLE 标记 + `send`/`receive` 在 extension 内不可 override —— 引入 internal `WebSocketTaskHandle` protocol + `WebSocketTaskFactory` factory，production 路径用 `URLSessionWebSocketTaskHandle` wrapper；测试通过 internal init 注入 fake
- 首轮 test 运行失败：URL 编码 case#1 期望 `abc%2Bdef` 但 `URLComponents.queryItems` 默认不编码 `+`（`+` 在 RFC 3986 query 中是 allowed） —— 改用 `addingPercentEncoding(withAllowedCharacters:)` 减去 `+/=&?#` reserved 子集 + 写 `percentEncodedQueryItems`，与 V1 §12.1 钦定 URL-encoded token 一致（防 `+` 被 server form-decoder 解为空格）

### Completion Notes List

- ✅ AC1: `WebSocketClient` protocol 新增 `connect(roomId:) async throws` + `send(_:) async throws`（保留 `prepareForReconnect()` 默认 no-op extension）
- ✅ AC2: 新建 `WSOutgoingMessage.swift`（仅 `case ping(requestId: String)`，节点 4 阶段 V1 §12.2 范围）
- ✅ AC3: 新建 `WSError.swift`（6 个 case：tokenMissing / invalidURL / connectionFailed / closedByServer / notConnected / decodingFailed；`underlyingDescription: String` 而非 Error existential，让 Equatable 易实现）
- ✅ AC4: 新建 `WSMessageCodec.swift`（incoming JSON envelope decode + outgoing JSONSerialization encode；DTO + 映射模式不耦合 Story 12.1 既有 RoomSnapshotPayload；未识别 type / payload 解码失败统一走 `.unknown(rawType:)` 不破坏 stream）
- ✅ AC5: 新建 `WebSocketClientImpl.swift`（基于 URLSessionWebSocketTask；引入 internal `WebSocketTaskHandle` protocol + `WebSocketTaskFactory` 让单测可注入 fake handle；NSLock 保护 mutable state；deinit 兜底 cancel + finish；scheme http→ws / https→wss 自动转换；URL token percent-encode 严格 RFC 3986 unreserved + 排除 `+/=&?#`）
- ✅ AC6: 扩展 `WebSocketClientMock`（新增 `connectCallArgs` / `sentMessages` 记录字段 + `connectError` / `sendError` 可控 stub；`connect / send` mock 不真实拨号 / 发送）
- ✅ AC7: `AppContainer.swift` 加 `let webSocketClient: WebSocketClient` + 默认 init 实例化 `WebSocketClientImpl(baseURL:, tokenProvider:)`；测试 init 重载允许注入 mock；**RootView wire 不改**（仍传 nil 给 RealRoomViewModel.bind —— Story 12.7 才接管）
- ✅ AC8: `WebSocketClientImplTests.swift` 9 case 全绿（含必须 case#1-#5 + 推荐 case#6 prepareForReconnect / case#7 disconnect-then-send + URL https→wss 路径）
- ✅ AC9: `bash iphone/scripts/build.sh --test` BUILD SUCCESS + 430 tests passed (含本 story 9 新增 + 421 既有 0 回归); ios-simulator MCP 视觉验证 home 页面渲染正常无 crash + 视觉无回归
- ✅ AC10: 新建 5 文件 + 修改 3 文件（清单见 File List）
- ⚠️ 已知 gap（Story 12.5 范围）：receive 循环 catch 块仅 finish stream + log，**不**读取 close code 投递 `WSError.closedByServer(code:reason:)`；Story 12.5 reconnect 状态机落地时需要在该 hook 点扩展 close code 信号传递（log 中已有 closeCode 但移除以避免 production noise；按需重新加）

### File List

**新建文件（5 个）**：
- `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift`（AC5）
- `iphone/PetApp/Core/Networking/WSMessageCodec.swift`（AC4）
- `iphone/PetApp/Core/Networking/WSError.swift`（AC3）
- `iphone/PetApp/Core/Networking/WSOutgoingMessage.swift`（AC2）
- `iphone/PetAppTests/Core/Networking/WebSocketClientImplTests.swift`（AC8）

**修改文件（3 个）**：
- `iphone/PetApp/Core/Networking/WebSocketClient.swift`（AC1：加 `connect` + `send` 方法）
- `iphone/PetApp/Core/Networking/WebSocketClientMock.swift`（AC6：加 connectCallArgs / sentMessages / connectError / sendError + connect/send 实装）
- `iphone/PetApp/App/AppContainer.swift`（AC7：加 webSocketClient 字段 + 默认 init 实例化 + 测试 init 注入入口）

**Story / Sprint 维护**：
- `_bmad-output/implementation-artifacts/12-2-websocketclient-封装.md`（本 story 状态 ready-for-dev → in-progress → review；任务全部 [x]；Dev Agent Record 填写）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（development_status[12-2-websocketclient-封装] ready-for-dev → in-progress → review）

### Change Log

- 2026-05-09 — Story 12.2 完成实装（WebSocketClientImpl + 4 周边类型 + AppContainer wire + 9 单测）；状态 → review
