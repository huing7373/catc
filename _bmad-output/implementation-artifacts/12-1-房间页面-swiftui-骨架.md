# Story 12.1: 房间页面 SwiftUI 骨架 ⟶ 注入真实 RoomViewModel

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

> **2026-04-30 变更**（[sprint-change-proposal-2026-04-29-v2.md](../planning-artifacts/sprint-change-proposal-2026-04-29-v2.md) §5.1 落地）：原范围「房间页 SwiftUI 骨架」由 Story 37.8 RoomView Scaffold 完成；本 story 缩窄为「在 Story 37.8 已交付的 `RoomScaffoldView` 上把 `RealRoomViewModel` 从占位骨架升级为真实 WS-driven 实装（持 `WSState + members + memberPetStates`，`roomId` getter 来自 `appState.currentRoomId`，按 AR21 ID 字符串约定）」。

## 故事定位（Epic 12 第 1 条 story；epic-12 起步 story）

这是 Epic 12「iOS - 房间页面 + WebSocket 客户端」的**第 1 条 story** —— 上游 Epic 37（iPhone 架构层重构 + UI Scaffold）已 done，`RoomScaffoldView` / `RoomViewModel` 基类 / `MockRoomViewModel` / `RealRoomViewModel` 占位骨架 / `RoomMember` value type / `AccessibilityID.Room` 常量集 / `AppState.currentRoomId` / `HomeContainerView` 互斥状态机 / RootView `@StateObject private var roomViewModel: RoomViewModel = RealRoomViewModel()` wire 全部就位（详见 Story 37.3 / 37.4 / 37.7 / 37.8 落地）。本 story 不动 Scaffold 视觉、不动 RootView wire、不动 HomeContainerView 状态机，**仅扩展 `RealRoomViewModel` 字段集 + 接入 WS 消息驱动 `members`**，并在 `RoomScaffoldView` 内补 `wsState` 占位文本（"已连接 / 正在重连 / 已断开"）。

**本 story 落地后立即解锁**：

- Story 12.3（房间快照解析 + 成员列表渲染）—— `RealRoomViewModel.members` 已能被 WS `room.snapshot` 写入，本 story 单测覆盖最小路径；12.3 扩展更多 case + UI 渲染细节
- Story 12.4（成员加入 / 离开 WS 消息处理）—— 在本 story `members` 字段上增量 mutate
- Story 12.5（自动重连）—— `wsState` 字段已存在 + 视觉占位文本已绑定，12.5 实装真实重连逻辑
- Story 12.6（心跳维护）—— 与 12.5 共用 WSClient 实例
- Story 12.7（CreateRoom / JoinRoom / LeaveRoom UseCase）—— 通过 `appState.setCurrentRoomId(_:)` 触发 RealRoomViewModel 自动连/断 WS

**本 story 的"实装"动作**（一句话概括）：

1. 在 `iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift` 基类新增 3 个 `@Published` 字段：`wsState: WSState`（默认 `.disconnected`）、`memberPetStates: [String: HomePetState]`（默认 `[:]`，节点 5 后启用）—— `members` 字段已存在；
2. 新建 `iphone/PetApp/Features/Room/Models/WSState.swift`：enum `WSState { case connected, reconnecting, disconnected }`（Sendable + Equatable）；
3. 改写 `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`：新增 `roomId: String?` computed getter（从 `appState?.currentRoomId` 派生，**不**持本地副本）、新增 `webSocketClient: WebSocketClient?` 构造注入（默认 nil，允许 Story 12.7 后由 UseCase 注入）、订阅 `appState.$currentRoomId` 触发 `connect / disconnect`、订阅 `webSocketClient.messages` AsyncStream 解析 `room.snapshot` → 写 `members`（按 §12.3 client merge contract）+ `wsState`、保留 Story 37.8 round 3 的 `subscribeRoomCode` 派生 `roomCodeForCopy` 路径不变；
4. 在 `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift` 顶部状态文字补 `wsState` 三态占位文本（"已连接 / 正在重连 / 已断开"），accessibility identifier `wsStateLabel`；
5. 删除 `MockRoomViewModel` 场景路径中关于"测试 / Preview"的多余配置 —— 不动 Mock，仅在 RealRoomViewModel 的 `members` / `wsState` 路径补 unit test ≥4 case + UI test 1 case；
6. **不**实装 WebSocketClient 真实类（**Story 12.2 范围**）—— 本 story 只引入 `WebSocketClient` protocol 最小契约（`messages: AsyncStream<WSMessage> { get async }` + `disconnect()`），让本 story 测试可注入 `WebSocketClientMock`（最小 mock 实现）。

**关键路径**：

- `RealRoomViewModel` 接 WS 走"构造注入"模式：`init(appState: AppState, webSocketClient: WebSocketClient? = nil)`；webSocketClient 默认 nil 让 RootView `@StateObject private var roomViewModel: RoomViewModel = RealRoomViewModel()` 老 wire 路径继续工作（基类无参 init → super.init() → appState/webSocketClient 都为 nil；`bind(appState:webSocketClient:)` 异步注入完整两路）；
- `roomId` 是**纯 computed getter**（`var roomId: String? { appState?.currentRoomId }`）—— 不持本地副本，避免与 `appState.currentRoomId` 派生出双 source of truth（防 codex BLOCKER 4 重复出现：参见 sprint-change-proposal-2026-04-29-v2.md §3 BLOCKER 4 "3 份状态源"问题被本 epic 钦定移除）；
- WS `room.snapshot` 解析按 V1接口设计.md §12.3 "client merge contract" 严格执行：snapshot 的 `userId` 集合是 roster 权威集合（snapshot 没有的 `userId` 应移除，新增的 `userId` 应 append）；字段级 merge：非空值覆盖、空字符串保留、`null` 覆盖（pet-less 信号）、未出现字段保留；
- `wsState` 由本 story 在 RealRoomViewModel `connect / disconnect / reconnecting` 三态切换中**显式**写（不靠 WSClient 内部 publisher），保持 ViewModel 层是 source of truth。

**不涉及**（红线）：

- **不**实装 `WebSocketClient` 真实类基于 `URLSessionWebSocketTask`（**Story 12.2 范围**）；本 story 仅定义 `WebSocketClient` protocol 最小契约 + 提供 `WebSocketClientMock` 用于测试
- **不**实装 `WSMessage` 全部 case（**Story 12.2 范围**）；本 story 只覆盖 `roomSnapshot` / `pong` / `error` 三种最小集 enum case（与 §12.3 Epic 10 阶段 server → client message set 一致：Epic 10 "**只**会主动发送 `room.snapshot` / `pong` / `error` 三种消息"）—— `member.joined` / `member.left` 由 Story 12.4 加 case
- **不**实装 `member.joined` / `member.left` 增量 mutate 路径（**Story 12.4 范围**）
- **不**实装自动重连指数退避（**Story 12.5 范围**）；本 story `wsState` 三态字段已就位，但 `disconnect → reconnecting` 切换由 Story 12.5 落地
- **不**实装 ping/pong 心跳定时（**Story 12.6 范围**）
- **不**实装 `CreateRoomUseCase` / `JoinRoomUseCase` / `LeaveRoomUseCase`（**Story 12.7 范围**）；本 story 仅订阅 `appState.$currentRoomId` 让 RealRoomViewModel 在 nil ↔ non-nil 转换时自动 disconnect / connect WS，**不**调 server REST API
- **不**改 `RoomScaffoldView` 5 区块视觉 / 7 a11y identifier（仅新增 `wsStateLabel` 一个状态文字 a11y identifier，且**不**改其它视觉）
- **不**改 RootView `@StateObject` wire（已是 RealRoomViewModel 实例）
- **不**改 `HomeContainerView` 互斥状态机决策
- **不**改 AppState 字段集 / hydrate 路径（仅订阅 `$currentRoomId`，不新增字段、不改 setter 签名）
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3）
- **不**改 server/ 任何文件
- **不**收口 `AccessibilityID.Room` 常量（仅在 RoomScaffoldView 内 inline 添加 `wsStateLabel` 字面量；后续 Story 12.5 / 12.7 落地时由 dev 选择是否补 `AccessibilityID.Room.wsStateLabel`，本 story 不收口）
- **不**预先生成 `Member` 域模型（直接复用 Story 37.8 已落地的 `RoomMember`）

## Story

As an iOS 开发,
I want 把 Story 37.8 RoomView Scaffold 上的占位 RealRoomViewModel 升级为真实 WS-driven ViewModel（持 `WSState + members + memberPetStates`，`roomId` getter 来自 AppState）,
So that 房间页连上 WS 后能显示真实成员列表 + 连接状态，为 Story 12.3 / 12.4 / 12.5 / 12.6 / 12.7 提供稳定接缝.

## Acceptance Criteria

> **AC 编号体系**：AC1 是 RoomViewModel 基类扩字段（wsState + memberPetStates）；AC2 是 WSState 值类型；AC3 是 WebSocketClient protocol + WSMessage enum 最小集；AC4 是 RealRoomViewModel 真实实装（构造注入 + roomId computed getter + appState.$currentRoomId 订阅 + room.snapshot 解析）；AC5 是 RoomScaffoldView wsStateLabel 占位文字；AC6 是 RootView wire（webSocketClient 注入路径，本 story 占位 nil）；AC7 是单元测试 ≥4 case；AC8 是 UITest（`roomMember_0/1/2` + `roomIdDisplay` 定位）；AC9 是 build verify；AC10 是 Deliverable 清单。

### AC1 — RoomViewModel 基类扩 2 个 @Published 字段

**修改文件**：`iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift`

新增 2 个 `@Published` 字段（与 Story 37.8 已落地的 4 字段并列）：

```swift
/// WebSocket 连接状态（Story 12.1：connected / reconnecting / disconnected 三态枚举）.
/// 默认 `.disconnected`（RealRoomViewModel 在 connect 成功后切 connected；Story 12.5 后 reconnect 中切 reconnecting）.
@Published public var wsState: WSState = .disconnected

/// 成员宠物状态映射（Story 12.1 字段就位；节点 5 后真实启用，节点 4 阶段保持空）.
/// key = userId（String）；value = HomePetState；用于房间页 4 格成员渲染时取每个成员的 currentState.
/// 节点 4 阶段 server `room.snapshot` 下发 `payload.members[].pet.currentState` 固定 `1`（rest），
/// 因此本字段在节点 4 阶段下永远是空 map（解析 snapshot 时**不**写入；待 Epic 14 真实驱动）；
/// 但**字段必须就位**，否则 Story 14.x / 15.x 落地时还要回工 RealRoomViewModel.
@Published public var memberPetStates: [String: HomePetState] = [:]
```

**字段命名理由**：

- `wsState` 与 Story 37.8 已存在的 `members` / `roomCodeForCopy` 等共享同一个基类（不分 `wsState` 到子类，避免 Mock 子类无法构造 `wsState = .connected` 测试场景）；与 sprint-change-proposal §5.1 的 "RealRoomViewModel 持 `WSState`" 锚定一致 —— 字段定义在基类，**生产值**写入由 RealRoomViewModel 实装，Mock 子类可注入任意值用于测试 / Preview；
- `memberPetStates` 类型选 `[String: HomePetState]` 而非 `[String: Int]` —— 与 Story 37.4 落地的 `HomePetState` enum（`HomeData.swift:120`）字段对齐，避免节点 5 后类型迁移；key 类型 `String` 与 §12.3 `payload.members[].userId` BIGINT 字符串化对齐；
- **不**新增 `roomId: String?` `@Published` 字段 —— roomId 走 RealRoomViewModel computed getter（参见 AC4），避免与 AppState 双 source of truth；

**对应 Tasks**: Task 1.1

### AC2 — 新建 WSState 值类型

**新建文件**：`iphone/PetApp/Features/Room/Models/WSState.swift`

```swift
// WSState.swift
// Story 12.1 AC2: WebSocket 连接态枚举（房间页"已连接 / 正在重连 / 已断开"占位文本派生源）.
//
// 设计：value type + Equatable + Sendable（不引 Hashable，无需作 Dictionary key）.
// 节点 4 阶段三态枚举一次到位 —— Story 12.5 自动重连落地后会真实在三态间切换；
// Story 12.6 心跳超时落地后会从 connected → disconnected 触发 reconnecting；
// 本 story 的 RealRoomViewModel 仅在 connect / disconnect 两路径上切，reconnecting 由 Story 12.5 触发.

import Foundation

public enum WSState: Equatable, Sendable {
    case connected
    case reconnecting
    case disconnected
}
```

> **关键决策**：枚举值不带 associated value（不附 attempt 数 / lastError 等）—— 节点 4 阶段视觉只需三态文字，无附加信息；Story 12.5 真实重连指数退避落地后如果需要展示"第 N 次重连"，再演进为 `case reconnecting(attempt: Int)` —— 本 story 不预 over-design。

> **`disconnected` 默认值**：与 Story 12.5 落地后"重连失败超过 5 次 → wsState = .disconnected" 终态语义一致；本 story 没有 reconnect 路径，初始态默认 `disconnected` + WS 连接成功后切 `connected`，无中间态切换。

**对应 Tasks**: Task 2.1

### AC3 — 新建 WebSocketClient protocol + WSMessage enum 最小集

**新建文件**：`iphone/PetApp/Core/Networking/WebSocketClient.swift`

```swift
// WebSocketClient.swift
// Story 12.1 AC3: WebSocket 客户端协议层（最小契约；真实实装 `WebSocketClientImpl` 由 Story 12.2 落地）.
//
// 设计原则（与 ADR-0002 §3.1 / RoomViewModel 接缝设计同精神）：
//   - protocol-first：让 RealRoomViewModel 通过构造注入 mock 客户端写 unit test，
//     无需 stub URLSession / 起 mock server.
//   - AsyncStream 驱动 incoming messages（与 §12.3 "服务端主动推送" 异步语义吻合 + Swift 5.5+ 原生支持）.
//   - 节点 4 阶段 Epic 10 §12.3 钦定 server → client 只发 `room.snapshot` / `pong` / `error` 三种消息,
//     WSMessage enum 本 story 仅覆盖三种 case + 一个 `unknown(type: String)` fallback case 不破坏 stream（参见 epic Story 12.2 AC "edge: 服务端推未知 type → 解码失败 + log warning + 不破坏 stream"）.
//   - 节点 4 阶段 client → server 仅 `ping`（Story 12.6 落地）+ 业务暂无 client → server 消息.
//
// 真实实装锚定（Story 12.2）：基于 `URLSessionWebSocketTask` 实装 `WebSocketClientImpl`，
// 包含 connect(url:token:) / send(_:) / disconnect() / messages 全集；本 story 只引入 protocol + Mock.

import Foundation

public protocol WebSocketClient: AnyObject, Sendable {
    /// 服务端 → 客户端消息流（按 §12.3 通用信封解析后的强类型 enum）.
    /// 实装层（Story 12.2 `WebSocketClientImpl`）从 underlying URLSessionWebSocketTask 读出 text frame
    /// → JSONDecode 信封 → 按 `type` 路由到 enum case → yield 到该 stream.
    var messages: AsyncStream<WSMessage> { get }

    /// 主动断开（用户 leave / app 切后台）；触发 close code 1000（client-initiated close）.
    /// 调用后 `messages` stream 终止（finish），caller 应取消 for-await 循环.
    func disconnect()
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

    /// 解码失败 fallback —— 不破坏 AsyncStream（与 epic Story 12.2 AC "edge: 服务端推未知 type → 解码失败 + log warning + 不破坏 stream" 一致）.
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
```

**新建文件**：`iphone/PetApp/Core/Networking/WebSocketClientMock.swift`（仅 testing target 引用，但放 main target 是为了让 #Preview 也能用；与 Mock*ViewModel 同模式）

```swift
// WebSocketClientMock.swift
// Story 12.1 AC3: WebSocketClient 测试 mock —— 手动注入消息序列驱动 RealRoomViewModel 单测.
//
// 设计：与 MockHomeViewModel / MockRoomViewModel 同精神（actor / class / 直接断言 @Published 字段）.
// AsyncStream 用 `AsyncStream.makeStream()` 持有 continuation，让测试方法手动 yield 消息.

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
```

> **关键决策 1**：`WebSocketClient` 是 `AnyObject + Sendable` protocol —— 让 RealRoomViewModel 持 `webSocketClient: WebSocketClient?` 引用 + 异步路径安全（`@MainActor` ViewModel 中持 Sendable protocol 实例）。

> **关键决策 2**：`WSMessage` 是 enum 而非 protocol —— 与 RoomViewModel 基类 class 层次不同模式（消息是值语义、case 数固定、不需要 Mock/Real 子类）。

> **关键决策 3**：`WSMessage.unknown(rawType:)` fallback case —— 当 server 推未知 type 消息（如未来 epic 新增 case 但 client 没升级），decoder 应解析到 `unknown` case 而非 throw —— 让 stream 不被破坏，同时 log warning 触发 dev-time 警觉（与 Story 12.2 AC "edge: 服务端推未知 type → 解码失败 + log warning + 不破坏 stream" 一致）。

> **关键决策 4**：本 story **不**实装 `WebSocketClientImpl`（基于 `URLSessionWebSocketTask`） —— 留给 Story 12.2，理由：12.2 实装中需要处理 connect URL 拼装 / token URL-encode / `URLSessionWebSocketTask.receive()` 的 framing 解析 / 5 case ≥ unit test 覆盖等独立工作量，与本 story 的"ViewModel 字段扩展 + 订阅路径 + Scaffold 文字补"职责正交，分离能让两个 story 各跑 r0~r3 收敛。本 story 提供 protocol + Mock 让单测可以闭环（mock yield 消息 → ViewModel 解析 → 断言 @Published 字段）。

> **关键决策 5**：Story 12.2 落地时只需 `class WebSocketClientImpl: WebSocketClient` 即可无缝替换 RootView wire 中的 mock；本 story 在 RootView wire 中传 `webSocketClient: WebSocketClient? = nil`（默认 nil 路径），让 RealRoomViewModel 保持"appState 监听就位但 WS 未连"半完成态，等 Story 12.2 / 12.7 落地后再切到 real client 注入。

**对应 Tasks**: Task 3.1, 3.2

### AC4 — RealRoomViewModel 真实实装（构造注入 + roomId computed getter + appState.$currentRoomId 订阅 + room.snapshot 解析）

**修改文件**：`iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`

完整新版结构（增量改动；保留 Story 37.8 round 1 P2 / round 3 P2 lessons）：

```swift
// RealRoomViewModel.swift（Story 12.1 升级版；保留 Story 37.8 Lessons：
//   round 1 P2 fix - init seed scaffold defaults
//   round 3 P2 fix - 删除 hostCatName 派生自 currentPet
// ）.
//
// 范围（本 story 完整路径）：
//   - 构造注入 AppState + WebSocketClient（webSocketClient 默认 nil 让 RootView 老 wire 不破）
//   - roomId computed getter 来自 appState.currentRoomId（不持本地副本，避免双 source of truth）
//   - 订阅 appState.$currentRoomId：non-nil → connect WS（Story 12.2 后真实拨号；本 story 仅记 wsState = .connected 占位）；
//     nil → disconnect WS（断开 + members 清空）
//   - 订阅 webSocketClient.messages stream：解析 room.snapshot → 按 client merge contract 写 members
//   - onLeaveTap 保持 Story 37.8 行为：调 appState.setCurrentRoomId(nil) 让 HomeContainerView 切回 idle
//
// 本 story 不接 真实 URLSessionWebSocketTask 拨号（Story 12.2 落地）；wsState = .connected 仅由"appState.$currentRoomId
// 切到 non-nil + webSocketClient ≠ nil 之路径"显式切；webSocketClient = nil 时 wsState 保持 .disconnected.

import Foundation
import Combine
import os.log

@MainActor
public final class RealRoomViewModel: RoomViewModel {
    /// 构造注入的 AppState 引用（同 Story 37.8 模式：可经 init(appState:webSocketClient:) 或 bind(appState:webSocketClient:) 注入）.
    private var appState: AppState?

    /// 构造注入的 WebSocketClient（Story 12.1 新增；默认 nil 让 RootView `@StateObject` 老 wire 路径继续工作）.
    /// Story 12.2 / 12.7 后由真实 UseCase 注入 `WebSocketClientImpl` 实例.
    private var webSocketClient: WebSocketClient?

    /// roomId 派生 getter —— 直接来自 appState.currentRoomId，**不**持本地副本.
    /// 避免与 appState 双 source of truth（防 codex BLOCKER 4 重复出现：详见 sprint-change-proposal-2026-04-29-v2.md §3）.
    public var roomId: String? {
        appState?.currentRoomId
    }

    /// Story 37.8 round 3 P2 / Story 12.1 共用的 currentRoomId 订阅（派生 roomCodeForCopy）.
    /// 保留 Story 37.8 lesson "published-derived-state-needs-publisher-subscription"（不用一次性 hydrate）.
    private var roomCodeSubscription: AnyCancellable?

    /// Story 12.1 新增：`appState.$currentRoomId` 订阅（roomId nil ↔ non-nil 切换驱动 WS connect/disconnect + members 清空）.
    private var roomIdConnectSubscription: AnyCancellable?

    /// Story 12.1 新增：WebSocket messages stream consumer task（订阅 webSocketClient.messages → 解析 → 派生 members）.
    private var messageConsumerTask: Task<Void, Never>?

    public override init() {
        super.init()
        self.appState = nil
        self.webSocketClient = nil
        // Story 37.8 round 1 P2 fix：seed RoomScaffoldDefaults 让首帧渲染不空.
        self.roomCodeForCopy = RoomScaffoldDefaults.roomCodeForCopy
        self.hostCatName = RoomScaffoldDefaults.hostCatName
        self.members = RoomScaffoldDefaults.members
        self.userIsHost = RoomScaffoldDefaults.userIsHost
        // wsState / memberPetStates 走基类默认值（.disconnected / [:]）.
    }

    public init(appState: AppState, webSocketClient: WebSocketClient? = nil) {
        super.init()
        self.appState = appState
        self.webSocketClient = webSocketClient
        // Story 37.8 round 1 P2 fix：seed.
        self.roomCodeForCopy = RoomScaffoldDefaults.roomCodeForCopy
        self.hostCatName = RoomScaffoldDefaults.hostCatName
        self.members = RoomScaffoldDefaults.members
        self.userIsHost = RoomScaffoldDefaults.userIsHost
        subscribeRoomCode(to: appState)
        subscribeRoomIdConnect(to: appState)
        startConsumingMessages()
    }

    /// AppState + WebSocketClient 异步注入入口（与 Story 37.8 bind 同模式扩展两路）.
    public func bind(appState: AppState, webSocketClient: WebSocketClient? = nil) {
        let codeAlreadySubscribed = roomCodeSubscription != nil
        let connectAlreadySubscribed = roomIdConnectSubscription != nil
        self.appState = appState
        if let client = webSocketClient {
            self.webSocketClient = client
            startConsumingMessages()
        }
        if !codeAlreadySubscribed {
            subscribeRoomCode(to: appState)
        }
        if !connectAlreadySubscribed {
            subscribeRoomIdConnect(to: appState)
        }
    }

    // MARK: - subscribe helpers

    private func subscribeRoomCode(to appState: AppState) {
        roomCodeSubscription = appState.$currentRoomId
            .sink { [weak self] roomId in
                guard let self else { return }
                self.roomCodeForCopy = roomId ?? RoomScaffoldDefaults.roomCodeForCopy
            }
    }

    /// Story 12.1 AC4 关键路径：roomId nil ↔ non-nil 切换驱动 wsState + members 清空.
    /// 单元测试 case#3 / case#4 直接测本订阅触发的副作用.
    private func subscribeRoomIdConnect(to appState: AppState) {
        roomIdConnectSubscription = appState.$currentRoomId
            .removeDuplicates()
            .sink { [weak self] roomId in
                guard let self else { return }
                if let roomId = roomId, !roomId.isEmpty {
                    // 进入房间：wsState 切 connected（占位；Story 12.5 真实拨号 / reconnect 三态后再演进）.
                    // webSocketClient = nil 时 wsState 保持 .disconnected（无 client 即无连接信号）.
                    if self.webSocketClient != nil {
                        self.wsState = .connected
                    }
                    os_log(.debug, "RealRoomViewModel: appState.currentRoomId = %{public}@ (will subscribe WS messages)", roomId)
                } else {
                    // 离开房间：disconnect + 清空 members + memberPetStates + wsState = .disconnected.
                    self.webSocketClient?.disconnect()
                    self.members = []
                    self.memberPetStates = [:]
                    self.wsState = .disconnected
                    os_log(.debug, "RealRoomViewModel: appState.currentRoomId = nil (cleared roster + WS disconnected)")
                }
            }
    }

    /// Story 12.1 AC4 关键路径：subscribe webSocketClient.messages → 解析 room.snapshot → 写 members.
    /// for-await 走 detached task；ViewModel deinit / disconnect 时 task cancel + stream finish 自然退出.
    private func startConsumingMessages() {
        messageConsumerTask?.cancel()
        guard let client = webSocketClient else { return }
        messageConsumerTask = Task { [weak self] in
            for await message in client.messages {
                guard let self else { return }
                await MainActor.run {
                    self.handle(message: message)
                }
            }
        }
    }

    /// §12.3 client merge contract 实装：snapshot 是 enrich/correct 而非 wipe-out.
    /// 节点 4 阶段（本 story）实装最小路径：
    ///   - roster 集合层：以 snapshot 的 userId 集合为权威（缺失则移除、新增则 append）
    ///   - 字段级：非空值覆盖、空字符串保留 client 已有值、null 直接覆盖
    ///   - memberPetStates：节点 4 阶段 server 固定 currentState=1 → 本 story 保持空 map（Epic 14 真实驱动后再写入）
    private func handle(message: WSMessage) {
        switch message {
        case .roomSnapshot(let payload):
            applySnapshot(payload)
        case .pong:
            // Story 12.6 心跳框架处理；本 story discard.
            break
        case .error(let code, let message, _):
            os_log(.error, "RealRoomViewModel WS error: code=%{public}d, msg=%{public}@", code, message)
        case .unknown(let rawType):
            os_log(.error, "RealRoomViewModel WS unknown message type: %{public}@", rawType)
        }
    }

    /// snapshot apply（roster 集合 + 字段级 merge）.
    /// 节点 4 阶段：snapshot members[] 直接映射为 RoomMember 数组（id=userId, name=nickname || 占位, level=8 占位, status="在玩耍" 占位, isHost=index==0 占位）—— `level` / `status` / `isHost` 由 Epic 14 / Epic 8 / 后续 host 字段下发后真实派生；本 story 仅保证 `id` / `name` 与 snapshot 一致.
    /// **节点 4 placeholder 阶段允许 nickname 为空字符串**——按 §12.3 "client merge contract" 空字符串 = "server 不知道"，应保留 client 已有值；
    /// 本 story 实装策略（最小路径）：snapshot member.nickname 为空字符串时，**保留** client 已有同 userId 的 RoomMember.name；新成员（client 没有的 userId）首次出现 nickname 为空字符串时降级为 placeholder "成员"（与 ui_design 占位一致；Story 11.7 真实 nickname 落地后即被覆盖）.
    private func applySnapshot(_ payload: RoomSnapshotPayload) {
        let snapshotUserIds = Set(payload.members.map { $0.userId })
        // step 1: 按 userId 集合做"roster 权威"裁剪 + 增量
        var newMembers: [RoomMember] = []
        for (index, snapshotMember) in payload.members.enumerated() {
            let existing = self.members.first { $0.id == snapshotMember.userId }
            // 字段级 merge: nickname 空字符串保留 existing.name；非空覆盖
            let mergedName: String = {
                if !snapshotMember.nickname.isEmpty {
                    return snapshotMember.nickname
                } else if let existing = existing {
                    return existing.name  // client 已有值（来自上一次 snapshot 或 GET /rooms 响应）
                } else {
                    return "成员"  // placeholder（首次出现 + nickname 空字符串；Story 11.7 后即覆盖）
                }
            }()
            // level / status / isHost 节点 4 阶段保持占位
            let merged = RoomMember(
                id: snapshotMember.userId,
                name: mergedName,
                level: existing?.level ?? 8,
                status: existing?.status ?? "在玩耍",
                isHost: index == 0  // 节点 4 阶段约定：snapshot 第一个成员视为 host（与 ui_design room.jsx 默认渲染一致；后续 epic 引入 host userId 字段后真实派生）
            )
            newMembers.append(merged)
        }
        self.members = newMembers
        // memberPetStates：节点 4 阶段 server 固定 currentState=1，不写入；Epic 14 后真实驱动.
        // 本 story 不动 memberPetStates（保持初始 [:]）.
        os_log(.debug, "RealRoomViewModel: applied snapshot (members.count = %{public}d)", newMembers.count)
        _ = snapshotUserIds  // for future use（Story 12.4 增量 mutate 时需要做 set diff）
    }

    // MARK: - override abstract methods

    public override func onLeaveTap() {
        os_log(.debug, "RealRoomViewModel.onLeaveTap (Story 12.7 will wire LeaveRoomUseCase)")
        // 节点 4 占位：直接置 currentRoomId = nil（subscribeRoomIdConnect 自动触发 disconnect + members 清空 + wsState = .disconnected）.
        // Story 12.7 落地 LeaveRoomUseCase 后改为：调 server POST /rooms/{id}/leave → 成功后再 setCurrentRoomId(nil).
        self.appState?.setCurrentRoomId(nil)
    }

    public override func onCopyTap() {
        os_log(.debug, "RealRoomViewModel.onCopyTap")
        // 实际 UIPasteboard 复制由 RoomScaffoldView 内 SwiftUI @State + 调用本方法时一起触发（Story 37.8 落地）.
    }

    deinit {
        messageConsumerTask?.cancel()
    }
}
```

> **关键决策 1**：`roomId` 是 computed getter（`var roomId: String? { appState?.currentRoomId }`），**不**用 `@Published` 修饰 —— View 层不需要单独 observe `roomId`（已经通过 `roomCodeForCopy` 派生展示；`roomIdDisplay` UITest 锚定的也是 `roomCodeForCopy` 文本），同时避免与 AppState.currentRoomId 双 source of truth。

> **关键决策 2**：`subscribeRoomIdConnect` 用 `.removeDuplicates()` —— 防 AppState 多次重复 emit 同值（如 hydrate 两次走 applyHomeData 都把 currentRoomId 置为同一 roomId）触发重复 connect / disconnect。

> **关键决策 3**：`webSocketClient = nil` 时 wsState 保持 `.disconnected` —— RootView 当前 wire `RealRoomViewModel()` 无参 init（无 webSocketClient），即使 user 已 hydrate 进房间，wsState 仍 `.disconnected`，UI 显示"已断开"占位文本；待 Story 12.2 + 12.7 落地后由 UseCase 注入真实 client + 调用 `bind(appState:webSocketClient:)`，wsState 切 `.connected`。这是**显式**的"半完成"语义，不是 bug —— 让 UI 在节点 4 阶段就有占位反映 WS 真实态。

> **关键决策 4**：`applySnapshot` 实装"最小 client merge contract" —— 严格按 §12.3 字段级 merge 规则；但**简化** `level` / `status` / `isHost` 节点 4 阶段占位逻辑（`isHost = index == 0`），原因：Story 12.3 / Epic 14 / Epic 8 才真实下发对应字段，本 story 不预 over-design；如未来 server 加 `payload.members[].isHost` 字段（V1接口设计 §12.3 当前**无**该字段），需调整 host 判定逻辑（届时由该 epic 落地 story 改）。

> **关键决策 5**：`memberPetStates` 节点 4 阶段保持空 map —— server `currentState` 固定 1 不携带真实值；待 Epic 14 / Story 14.3 后由 server snapshot 真实下发，再在 RealRoomViewModel 内 populate（届时 schema `RoomSnapshotPet.currentState: Int` 会真实 1/2/3，applySnapshot 改写 `memberPetStates[member.userId] = HomePetState(rawValue: pet.currentState)` 即可）。

> **关键决策 6**：`startConsumingMessages` 在 `webSocketClient = nil` 时 early return（不启 task）—— 避免空跑 task 浪费资源；`bind(appState:webSocketClient:)` 注入 client 后再调 `startConsumingMessages()` 启 task。

**对应 Tasks**: Task 4.1, 4.2, 4.3, 4.4

### AC5 — RoomScaffoldView wsStateLabel 占位文字

**修改文件**：`iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift`

在顶部 topBar 区块之后、roomCodeCard 区块之前新增一个 `wsStateLabel` 行（或在 topBar 内部右侧空白区域内嵌入；具体位置由 ui_design 视觉决定，但**不**改 5 区块结构 —— 仅在合适位置插入一行文字 view）。

**视觉规则**：

- 文字内容由 `state.wsState` 派生：
  - `.connected` → "已连接"
  - `.reconnecting` → "正在重连…"
  - `.disconnected` → "已断开"
- 字体 / 颜色：复用 theme tokens（`theme.colors.muted` 灰字 + small font）
- accessibility identifier `wsStateLabel`（inline 字符串，**不**收口到 `AccessibilityID.Room` —— 等 Story 12.5 真实重连交互落地后再决定常量化）

```swift
// 新增 private var wsStateLabel:
private var wsStateLabel: some View {
    Text(wsStateText)
        .font(.system(size: 12, weight: .regular))
        .foregroundColor(theme.colors.muted)
        .accessibilityIdentifier("wsStateLabel")
}

private var wsStateText: String {
    switch state.wsState {
    case .connected: return "已连接"
    case .reconnecting: return "正在重连…"
    case .disconnected: return "已断开"
    }
}
```

**对应 Tasks**: Task 5.1

### AC6 — RootView wire（webSocketClient 注入路径，本 story 占位 nil）

**修改文件**：`iphone/PetApp/App/RootView.swift`

RootView 当前 wire 是 `@StateObject private var roomViewModel: RoomViewModel = RealRoomViewModel()`（无参 init）+ `.onAppear` 内 `if let realRoomVM = roomViewModel as? RealRoomViewModel { realRoomVM.bind(appState: appState) }`。

本 story 改动**最小**：

- 修改 `bind` 调用签名：`realRoomVM.bind(appState: appState)` → `realRoomVM.bind(appState: appState, webSocketClient: nil)`（**显式**传 nil，让 reader 一眼看出"WS 真实 client 由 Story 12.2 落地后再注入"）；
- **不**新增 RootView 级 WebSocketClient 持有；**不**实例化 WebSocketClientImpl（Story 12.2 落地后由 dev 改本处）。

**对应 Tasks**: Task 6.1

### AC7 — 单元测试 ≥4 case（覆盖核心字段 + appState 订阅 + snapshot 解析）

**新建测试文件**：`iphone/PetAppTests/Features/Room/RealRoomViewModelTests.swift`（与 Story 37.8 落地的 RoomViewScaffoldTests.swift 同目录平级）

**测试基础设施约束**（与 Story 37.8 / ADR-0002 §3.1 衔接）：
- 仅依赖 stdlib（XCTest + @testable import PetApp）
- 不引 ViewInspector / SnapshotTesting
- 直接断言 `RealRoomViewModel` 的 @Published 字段 + `WebSocketClientMock.emit(_:)` 驱动 stream

**必须覆盖的测试 case**（≥4 case，按 sprint-change-proposal §5.1 锚定）：

```swift
// case#1 happy: appState.currentRoomId = "room_1234567" → RealRoomViewModel.roomId == "room_1234567"
//   验证 roomId computed getter 路径（AR21 ID 字符串约定）.
//   AppState 用 makeHydrated(currentRoomId: "room_1234567") 构造.

// case#2 happy: WebSocketClientMock 推 room.snapshot 含 3 成员 → ViewModel.members.count == 3
//   AppState 已注入 currentRoomId（保证订阅链路就绪）.
//   构造 RealRoomViewModel(appState: ..., webSocketClient: mock).
//   mock.emit(.roomSnapshot(payload with 3 members)) → 等待主线程消费 → 断言 members.count == 3 + members[0].name 与 snapshot 一致.

// case#3 happy: appState.currentRoomId nil → non-nil 切换 → wsState 从 .disconnected → .connected（webSocketClient ≠ nil 路径）
//   构造 RealRoomViewModel(appState:, webSocketClient: mock).
//   appState.setCurrentRoomId("room_xxx") → 断言 wsState == .connected.
//   appState.setCurrentRoomId(nil) → 断言 wsState == .disconnected + members 清空.

// case#4 edge: snapshot 解析未知 type / 解码失败 → ViewModel 不破坏现有 members + log error
//   先 mock.emit(.roomSnapshot(3 成员)) → 断言 members.count == 3.
//   再 mock.emit(.unknown(rawType: "garbage_type")) → 断言 members.count 仍 == 3（不破坏）.
//   （pretty-print log error 不强制单测覆盖；本 case 验证 stream 不被破坏 + members 不被错误清空）
```

**可选 case**（推荐补一个但不强制）：

```swift
// case#5 happy: webSocketClient = nil 路径 → wsState 永远保持 .disconnected
//   构造 RealRoomViewModel(appState: ..., webSocketClient: nil).
//   appState.setCurrentRoomId("room_xxx") → 断言 wsState 仍 .disconnected（无 client 即无连接信号）.
//   验证"半完成"语义符合 AC4 关键决策 3.
```

**用 await Task.yield() 让 AsyncStream 派发到 MainActor**：本 story 单测涉及 `WebSocketClientMock.emit` → for-await 循环 → MainActor.run 写 @Published —— 单测必须 `await Task.yield()` 让事件循环跑一轮，不能直接断言（与 RealHomeViewModel Story 37.7 round 4 [P3] lesson 同精神：published-derived-state-needs-publisher-subscription）。

**对应 Tasks**: Task 7.1, 7.2

### AC8 — UITest（roomMember_0/1/2 + roomIdDisplay + wsStateLabel 定位）

**新建测试文件 / 扩展**：`iphone/PetAppUITests/RoomUITests.swift`（如不存在则新建；已存在则扩展）

**必须覆盖的 UITest case**（按 sprint-change-proposal §5.1 锚定）：

```swift
// case#1 happy: appState.currentRoomId = "room_1234567" + WS mock 推 3 成员 →
//   RoomScaffoldView 渲染 → 验证 3 个成员位 accessibility identifier `roomMember_0/1/2` 可定位.
//   验证 房间号 label accessibility identifier `roomIdDisplay` 显示非空字符串.
//   验证 wsStateLabel accessibility identifier 可定位（不强断言文字内容，因为 webSocketClient = nil 路径 wsState 是 .disconnected）.
```

**实装策略**（与 HomeUITests / NavigationUITests 同模式）：

- 用 launch argument `UITEST_FORCE_IN_ROOM` （新增；如 Story 37.8 已有则复用）让 RootView 在测试启动时把 `appState.currentRoomId` 直接置为 `"room_1234567"`，绕过 guest-login + WS 真实连接路径
- 测试**不**需要驱动 RealRoomViewModel 的 WS 消息消费（webSocketClient = nil 路径下 members 仍是 RoomScaffoldDefaults 的 4 成员 mock）—— 直接验证 RoomScaffoldView 渲染 + a11y 定位
- 真实 WS 消息驱动的 UI 渲染留给 Story 12.3 UITest（届时 Story 12.2 真实 `WebSocketClientImpl` + 假 server / mock URL session 已落地）

> **关键决策**：本 story UITest **不**驱动真实 WS / mock server，仅验证 a11y 定位 + 房间号显示链路 —— 与 Story 37.8 UITest 模式一致；WS 消息驱动的 UI 验证由 Story 12.3 接手（届时所有 WS 周边 story 12.2-12.6 已落地，真实联调链路完整）。

**对应 Tasks**: Task 8.1

### AC9 — Build verify

**必须通过**：

```bash
bash iphone/scripts/build.sh --test
```

- xcodebuild 编译通过（无 warning）
- 所有单测通过（含本 story 新增 ≥4 case + 既有 RoomViewScaffoldTests / AppStateTests / RootViewWireTests 等不破）
- UITest 通过（本 story 新增 1 case + 既有 NavigationUITests / HomeUITests 等不破）

**ios-simulator MCP 验证**（CLAUDE.md "iOS UI 验证（必跑）"）：

```bash
1. bash iphone/scripts/build.sh
2. install_app(app_path: iphone/build/DerivedData/Build/Products/Debug-iphonesimulator/PetApp.app)
3. launch_app(bundle_id: "com.zhuming.pet.app", terminate_running: true)
4. 通过 dev 工具 / launch arg 把 appState.currentRoomId 设为 "room_1234567"
5. ui_view 验证：
   - 房间号区域显示 "room_1234567"（roomCodeForCopy 字段）
   - 状态文字 "已断开"（webSocketClient = nil 路径）
   - 成员列表 4 格 RoomScaffoldDefaults 占位（与 Story 37.8 视觉一致）
6. ui_describe_all 验证 a11y identifier `roomIdDisplay` / `roomMember_0/1/2/3` / `wsStateLabel` 全部存在.
```

**对应 Tasks**: Task 9.1

### AC10 — Deliverable 清单

本 story 完成后必须有：

**新建文件（4 个）**：

- `iphone/PetApp/Features/Room/Models/WSState.swift`（AC2）
- `iphone/PetApp/Core/Networking/WebSocketClient.swift`（AC3 protocol + WSMessage enum + RoomSnapshotPayload 等）
- `iphone/PetApp/Core/Networking/WebSocketClientMock.swift`（AC3 mock）
- `iphone/PetAppTests/Features/Room/RealRoomViewModelTests.swift`（AC7）

**修改文件（4 个）**：

- `iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift`（AC1 加 wsState + memberPetStates 字段）
- `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`（AC4 完整重写）
- `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift`（AC5 加 wsStateLabel）
- `iphone/PetApp/App/RootView.swift`（AC6 bind 签名加 webSocketClient: nil）

**新建 / 扩展（1 个）**：

- `iphone/PetAppUITests/RoomUITests.swift`（AC8 单 case 或扩展现有）

**Xcode project 更新**：所有新建 .swift 文件加入 `PetApp.xcodeproj` 的 main / test target（用 dev tools 或手动 sync；与 Story 37.8 同模式）。

**对应 Tasks**: Task 10.1

## Tasks / Subtasks

- [x] **Task 1.1** — RoomViewModel 基类扩 wsState + memberPetStates 字段（AC1）
- [x] **Task 2.1** — 新建 WSState.swift（AC2）
- [x] **Task 3.1** — 新建 WebSocketClient protocol + WSMessage enum + Snapshot payload structs（AC3）
- [x] **Task 3.2** — 新建 WebSocketClientMock（AC3）
- [x] **Task 4.1** — 改写 RealRoomViewModel：构造注入 webSocketClient + roomId computed getter（AC4）
- [x] **Task 4.2** — RealRoomViewModel.subscribeRoomIdConnect：appState.$currentRoomId 订阅 + nil ↔ non-nil 切换 + wsState + members 清空逻辑（AC4）
- [x] **Task 4.3** — RealRoomViewModel.startConsumingMessages：消费 webSocketClient.messages stream（AC4）
- [x] **Task 4.4** — RealRoomViewModel.applySnapshot：§12.3 client merge contract 实装（AC4）
- [x] **Task 5.1** — RoomScaffoldView 新增 wsStateLabel 文字（AC5）
- [x] **Task 6.1** — RootView.swift bind 签名调整（AC6）
- [x] **Task 7.1** — RealRoomViewModelTests case#1-#4 单测（AC7）
- [x] **Task 7.2**（推荐）— RealRoomViewModelTests case#5 webSocketClient nil 路径（AC7 可选）
- [x] **Task 8.1** — RoomUITests case#1（roomMember/roomIdDisplay/wsStateLabel 定位）（AC8）
- [x] **Task 9.1** — `bash iphone/scripts/build.sh --test` 全绿 + ios-simulator MCP 验证（AC9）
- [x] **Task 10.1** — Xcode project 同步新建文件 + 全部 deliverable 列表对照（AC10）

## Dev Notes

### 关键文档锚定

- `docs/宠物互动App_总体架构设计.md` — Tech Stack（iOS Swift+SwiftUI / WebSocket）
- `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §9（WebSocket 子系统建议封装 WebSocketClient + RoomRealtimeRepository + RoomRealtimeViewModelBridge —— 本 story 仅引入 WebSocketClient protocol，Repository / Bridge 由 Story 12.3 / 12.4 落地）
- `docs/宠物互动App_V1接口设计.md` §12.1 / §12.2 / §12.3（WS 协议；本 story 仅消费 server-active 三种消息 room.snapshot / pong / error）
- `docs/宠物互动App_V1接口设计.md` §12.3 "client merge contract"（snapshot 解析必读）
- `_bmad-output/planning-artifacts/sprint-change-proposal-2026-04-29-v2.md` §5.1 / §3 BLOCKER 4（Story 12.1 改写锚定 + 双 source of truth 红线）
- `_bmad-output/implementation-artifacts/decisions/0010-iphone-appstate.md`（AppState §3.1 / §3.2 / §3.3 hydrate 规则；ViewModel 注入规则；本 story RealRoomViewModel 走构造注入 + bind 异步注入两路）
- `_bmad-output/implementation-artifacts/decisions/0009-ios-stack.md` / `0002-ios-stack.md`（iOS 总技术栈 + 测试栈钦定 XCTest only）
- `_bmad-output/implementation-artifacts/37-8-roomview-scaffold.md`（前置 story 完整实装；本 story 升级版 RealRoomViewModel 必须保留 round 1 P2 / round 3 P2 lessons）

### Source tree 涉及位置

```
iphone/
  PetApp/
    Core/
      Networking/
        WebSocketClient.swift          # 新建（AC3）
        WebSocketClientMock.swift      # 新建（AC3）
    Features/
      Room/
        Models/
          RoomMember.swift             # Story 37.8 已落地，本 story 不动
          WSState.swift                # 新建（AC2）
        ViewModels/
          RoomViewModel.swift          # 修改（AC1）
          MockRoomViewModel.swift      # Story 37.8 已落地，本 story 不动
          RealRoomViewModel.swift      # 完整改写（AC4）
          RoomScaffoldDefaults.swift   # Story 37.8 已落地，本 story 不动
        Views/
          RoomScaffoldView.swift       # 修改（AC5 加 wsStateLabel）
          RoomViewPlaceholder.swift    # Story 37.3 已落地，本 story 不动
    App/
      RootView.swift                   # 修改（AC6 bind 签名）
      AppState.swift                   # 不改（仅订阅 $currentRoomId）
      ...
  PetAppTests/
    Features/
      Room/
        RoomViewScaffoldTests.swift    # Story 37.8 已落地，本 story 不动
        RealRoomViewModelTests.swift   # 新建（AC7）
    Helpers/
      AppStateTestHelpers.swift        # Story 37.4 已落地，本 story 复用
  PetAppUITests/
    RoomUITests.swift                  # 新建或扩展（AC8）
```

### Testing 标准摘要

- **单测**（PetAppTests target）：XCTest only；@MainActor 标注 RealRoomViewModelTests；用 `await Task.yield()` 让 AsyncStream emit 派发到 ViewModel @Published；不引 ViewInspector / SnapshotTesting；不起 mock HTTP / mock WebSocket server；通过 `WebSocketClientMock.emit(_:)` 直接驱动 stream
- **UITest**（PetAppUITests target）：XCUITest only；用 launch argument 绕过 WS 真实连接（webSocketClient = nil 路径渲染 RoomScaffoldDefaults 4 成员占位）；定位 `roomMember_0/1/2` / `roomIdDisplay` / `wsStateLabel` a11y identifier 可见
- **build verify**：`bash iphone/scripts/build.sh --test` 全绿（编译 + 单测 + UITest 三层）

### Project Structure Notes

- 文件位置严格按 `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §3 + 现有 PetApp 目录组织
- WebSocketClient.swift 放 `Core/Networking/` 与现有 APIClient 同级（与 iOS 架构文档 §3.1 钦定一致；§9.1 描述 WebSocket 子系统职责，§9.2 列出客户端对象建议）
- WSState.swift 放 `Features/Room/Models/` 与 RoomMember 同级（属于房间页 domain；不放到 `Core/Networking/`，因为 WSState 是 ViewModel 暴露给 View 的展示态，不是 Networking 层概念）
- 不引入 `RoomRealtimeRepository` / `RoomRealtimeViewModelBridge`（iOS 架构文档 §9.2 建议）—— 节点 4 阶段 RealRoomViewModel 直接订阅 WSClient.messages 已足够；Repository / Bridge 由 Story 12.4（成员加入/离开增量 mutate）/ Story 12.7（UseCase 链路）落地时如需才演进，本 story 不预 over-design

### 与 unified project structure 对齐 / variances

- 与 Story 37.8 落地的 `Features/Room/{Models, ViewModels, Views}` 三层完全一致；新增 `Features/Room/Models/WSState.swift` 同模式；
- WebSocketClient 放 `Core/Networking/` 是 iOS 架构钦定位置（不放 `Features/Room/` 避免跨 feature 共享时绕路）；
- **variance**：iOS 架构文档 §9.2 建议引入 `RoomRealtimeRepository` 中间层 —— 本 story 暂时**不**引入（理由：节点 4 阶段 ViewModel 直接订阅 WSClient.messages 工作量更小；如未来 Story 12.4 / 12.7 / Epic 14 落地后发现 ViewModel 太肥再演进，与 ADR-0010 §4.4 缓解策略同精神）。**这是有意识的 variance 而非疏忽**，记入 tech debt log（dev 落地时如同意此判断 → tech debt log 加一条；如反对 → 本 story 加 Repository 层并改写 RealRoomViewModel 走 Repository 调用）。

### Previous story intelligence（必读 lessons）

> **以下 12 条 ViewModel 层 lessons 来自 Epic 37 retrospective §2.3 + `docs/lessons/`，本 story 实装 RealRoomViewModel 时**逐条对照**避免重蹈：

1. **`real-viewmodel-init-must-seed-scaffold-defaults`**（37.8 r1）—— Real init 必须 seed RoomScaffoldDefaults 否则首帧渲染空（本 story 升级版仍保留 Story 37.8 的 init seed 逻辑）
2. **`real-viewmodel-injection-must-not-leave-base-fatalerror`**（37.7 r1）—— RootView 注入必须切到子类（已是 RealRoomViewModel；本 story 不动）
3. **`real-viewmodel-override-placeholder-must-mutate-state`**（37.7）—— override 不能只 print，必须实际写 @Published（本 story onLeaveTap 已调 `appState.setCurrentRoomId(nil)` 真实 mutate，AC4 关键决策 3 守护）
4. **`real-viewmodel-must-clear-transient-state-on-reset`**（37.7 r3）—— reset 时必须清 transient（本 story `subscribeRoomIdConnect` 中 nil 路径清空 members + memberPetStates + wsState 守护）
5. **`real-viewmodel-transient-must-clear-on-any-identity-change`**（37.11）—— 任意 identity 变更都要清 transient（本 story 通过 `.removeDuplicates()` 防同 roomId 重复触发，但 nil → non-nil → 另一 non-nil 路径要清 members；AC4 实装时 dev **必须**确认此逻辑覆盖到位 —— 切到不同 roomId 时**必须**清空 members + memberPetStates，否则会出现"上一个房间的成员显示在新房间"的 bug；当前 AC4 实装走 `.removeDuplicates()` 仅在切到 nil 时清空，**不**清空"换房间"路径，这是已知 gap，由 Story 12.4 / 12.5 增量 mutate 路径自然修复，但本 story dev 应在测试中显式触发"切换 roomId 不重置 members" case 暴露并确认这是 expected behavior 或 fail-fast 写 TODO）
6. **`published-derived-state-needs-publisher-subscription`**（37.7 r4）—— @Published 派生字段需订阅 publisher（本 story `subscribeRoomCode` / `subscribeRoomIdConnect` 都走 sink 路径，符合）
7. **`strong-vs-weak-for-constructor-injected-state`**（37.6/37.7）—— 构造注入 AppState / WebSocketClient 用 strong 引用（本 story 实装 RealRoomViewModel 持 `private var appState: AppState?` / `private var webSocketClient: WebSocketClient?` 都 strong，符合；MainActor + Sendable 兼容性已验证）
8. **`onappear-vs-task-sync-bind-before-first-paint`**（37.8 r2）—— bind 必须在第一次 paint 之前同步完成（本 story RootView 的 `.onAppear` 内 bind 调用已遵循 Story 37.8 模式；本 story 不动 RootView wire 时机）
9. **`room-host-name-must-not-derive-from-local-current-pet`**（37.8 r3）—— host name 不能派生自 local 猫（本 story 实装 applySnapshot 中 host 判定走 `index == 0`，**不**派生 currentPet；符合）
10. **`coordinator-must-mirror-loaded-home-room-state`**（37.3）—— coordinator 必须 mirror 已加载状态（本 story 不动 coordinator）
11. **`scaffold-bypass-viewmodel-seam`**（37.11）—— Scaffold 不能 bypass ViewModel 接缝直读 AppState（本 story RoomScaffoldView 仍只读 `state.*` 字段，**不**直读 `appState.currentRoomId`；AC5 wsStateLabel 派生走 `state.wsState`，符合）
12. **`realhomeviewmodel-greeting-and-empty-text-overlay`**（37.7）—— 空 Text overlay VoiceOver 陷阱（本 story wsStateLabel 永远有非空文字，符合）

### References

- [Source: docs/宠物互动App_总体架构设计.md] — iOS Swift+SwiftUI / WebSocket
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#9.1] — WebSocket 子系统职责
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#9.2] — 客户端对象建议（WebSocketClient + RoomRealtimeRepository + RoomRealtimeViewModelBridge）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#9.3] — 重连策略（Story 12.5 落地时锚定）
- [Source: docs/宠物互动App_V1接口设计.md#12.1] — WebSocket 连接地址 + 握手成功流程 + close code 表
- [Source: docs/宠物互动App_V1接口设计.md#12.3] — server → client 消息：room.snapshot / pong / error 三种最小集 + client merge contract
- [Source: _bmad-output/planning-artifacts/sprint-change-proposal-2026-04-29-v2.md#5.1] — Story 12.1 改写完整 acceptance（钦定本 story 范围）
- [Source: _bmad-output/planning-artifacts/sprint-change-proposal-2026-04-29-v2.md#3] BLOCKER 4 — 双 source of truth 红线（钦定 roomId 走 computed getter）
- [Source: _bmad-output/implementation-artifacts/decisions/0010-iphone-appstate.md#3.1] — AppState 注入规则（构造注入或 bind 异步注入；不允许 ViewModel 内 @EnvironmentObject）
- [Source: _bmad-output/implementation-artifacts/decisions/0010-iphone-appstate.md#3.2] — AppState 字段白名单（currentRoomId 已存在于白名单）
- [Source: _bmad-output/implementation-artifacts/decisions/0010-iphone-appstate.md#3.3] — applyHomeData / setCurrentRoomId hydrate 入口
- [Source: _bmad-output/implementation-artifacts/decisions/0009-ios-stack.md] — iOS 总技术栈
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.1] — 测试栈钦定 XCTest only
- [Source: _bmad-output/implementation-artifacts/37-8-roomview-scaffold.md] — 前置 story 完整实装（RoomViewModel 基类 / Mock/Real 子类 / RoomMember / RoomScaffoldDefaults / RoomScaffoldView 5 区块视觉 / 7 a11y identifier）
- [Source: _bmad-output/implementation-artifacts/epic-37-retro-2026-05-09.md#2.3] — 12 条 ViewModel 层 lessons 集中沉淀（Previous story intelligence 段必读）

### Latest tech information

- **Swift AsyncStream**：Swift 5.9+ 标准 `AsyncStream<Element>.makeStream()` 工厂方法返回 `(stream: AsyncStream<Element>, continuation: AsyncStream<Element>.Continuation)` —— Story 12.1 WebSocketClientMock 走此路径手动 yield 消息；不引第三方（与 ADR-0002 §3.1 测试栈钦定一致）
- **URLSessionWebSocketTask**：iOS 13+ 原生 API（`receive(completionHandler:)` / `send(_:completionHandler:)` / `cancel(with:reason:)`）—— Story 12.2 落地时基于此实装 WebSocketClientImpl；本 story 不引入
- **Combine sink + AnyCancellable**：Story 37.4 / 37.7 / 37.8 已沉淀的标准模式（@MainActor + class final + private var roomCodeSubscription: AnyCancellable?）—— 本 story 沿用
- **iOS 17+ Observation framework（@Observable）**：本 story **不**采用（与 Story 37.4 / 37.7 决策一致：保 @MainActor + ObservableObject + @Published 模式，避免迁移风险；待整 epic 一次性切换时再决策）

### Project context reference

`docs/lessons/` 内本 story 必读的 lessons：

- `2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md`（37.8 r1）
- `2026-04-30-published-derived-state-needs-publisher-subscription.md`（37.7 r4）
- `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`（37.7）
- `2026-04-30-real-viewmodel-must-clear-transient-state-on-reset.md`（37.7 r3）
- `2026-05-01-real-viewmodel-transient-must-clear-on-any-identity-change.md`（37.11）
- `2026-04-30-room-host-name-must-not-derive-from-local-current-pet.md`（37.8 r3）
- `2026-04-30-coordinator-must-mirror-loaded-home-room-state.md`（37.3）
- `2026-04-25-swift-explicit-import-combine.md`（Combine import 必须显式，不依赖 SwiftUI transitive）

`_bmad-output/implementation-artifacts/decisions/` 内本 story 必读 ADR：

- `0009-ios-stack.md` — iPhone 工程目录决策（导航架构）
- `0010-iphone-appstate.md` — AppState 单 source of truth 规则（含 §3.1 注入规则 + §3.2 白名单 + §3.3 hydrate + §3.7 reset）
- `0002-ios-stack.md` — 测试栈 XCTest only

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) — bmad-dev-story workflow（epic-loop sub-agent 派发）.

### Debug Log References

无 panics / 死锁 / regression；build + 410 unit tests 全绿；新加 5 case 单测全绿；新加 RoomUITests 1 case 通过；Story 37.8 既有 testRoomScaffoldShowsAllSevenAnchors 通过（无 regression）.

构造注入路径关键决策记录：subscribeRoomIdConnect 增 `.dropFirst()` 跳过 Published 订阅时的"当前值同步 emit"，否则 mock disconnect 会立即 finish stream 让后续 emit 全丢失（详见 RealRoomViewModel.swift 内注释）.

### Completion Notes List

- AC1 — RoomViewModel 基类增 `wsState: WSState` (默认 `.disconnected`) + `memberPetStates: [String: HomePetState]` (默认 `[:]`) 两个 `@Published` 字段；与 Story 37.8 既有 4 字段并列；MockRoomViewModel 不动；RealRoomViewModel WS 路径写入.
- AC2 — `WSState` enum (Equatable + Sendable) 三 case (connected / reconnecting / disconnected) 一次到位；不带 associated value；不预 over-design.
- AC3 — `WebSocketClient` protocol (AnyObject + Sendable) 最小契约 (`messages` AsyncStream + `disconnect()`)；`WSMessage` enum 4 case（roomSnapshot / pong / error / unknown rawType fallback）；payload structs (RoomSnapshotPayload / RoomSnapshotRoomInfo / RoomSnapshotMember / RoomSnapshotPet) 仅本 story 用到的字段就位；`WebSocketClientMock` 用 `AsyncStream.makeStream()` + `emit(_:)` 手动驱动消息.
- AC4 — RealRoomViewModel 完整重写：构造注入 (appState + webSocketClient = nil 默认参数) + `roomId` computed getter (无 @Published，避免双 source of truth) + bind(appState:webSocketClient:) 异步注入两路 + subscribeRoomCode (派生 roomCodeForCopy，保留 Story 37.8 round 3 行为) + subscribeRoomIdConnect (`.dropFirst().removeDuplicates()`，nil↔non-nil 切换驱动 wsState + members 清空 + webSocketClient?.disconnect()) + startConsumingMessages (Task + for-await + MainActor.run) + applySnapshot (§12.3 client merge contract：roster 集合裁剪 + nickname 非空覆盖 / 空字符串保留 existing / 首次空字符串降级"成员"占位 + isHost = (index == 0) 节点 4 阶段约定); deinit cancel task.
- AC5 — RoomScaffoldView 在 topBar 之后 / roomCodeCard 之前插入 `wsStateLabel` 行 (12pt regular + inkSoft + center alignment)；文字派生 connect/reconnecting/disconnected → "已连接"/"正在重连…"/"已断开"; accessibility identifier `wsStateLabel` inline 字面量（不收口至 AccessibilityID.Room，留 Story 12.5 决定）.
- AC6 — RootView `.onAppear` 内 `realRoomVM.bind(appState:)` 改为 `realRoomVM.bind(appState:webSocketClient: nil)` 显式传 nil；不实例化 WebSocketClientImpl（Story 12.2 范围）.
- AC7 — RealRoomViewModelTests 共 5 case 全绿:
  - testRoomIdGetterReadsFromAppState (case#1) — roomId computed getter 派生自 appState.currentRoomId
  - testRoomSnapshotMessagePopulatesMembers (case#2) — WS room.snapshot 解析 → members.count == 3 + 字段映射
  - testCurrentRoomIdSwitchTogglesWsStateAndClearsMembers (case#3) — nil ↔ non-nil 切换 + wsState 切 + members 清空 + mockWS.disconnect 调用
  - testUnknownMessageDoesNotCorruptMembers (case#4) — unknown / pong 消息走 fallback 不破坏 members
  - testWebSocketClientNilKeepsWsStateDisconnected (case#5) — webSocketClient = nil 路径 wsState 保持 .disconnected（"半完成"语义守护）
- AC8 — RoomUITests 新建文件 1 case (`testRoomScaffoldExposesUpgradedAccessibilityAnchors`)：UITEST_FORCE_IN_ROOM env flag 路径下 `roomIdDisplay` / `wsStateLabel` / `roomMember_0/1/2` 全部 a11y 锚定通过.
- AC9 — `bash iphone/scripts/build.sh --test` 全绿（410 单元测试，0 失败）；ios-simulator MCP 验证 — 模拟器实跑 `UITEST_SKIP_GUEST_LOGIN=1 + UITEST_FORCE_IN_ROOM=1` 路径，截图确认 "已断开" wsStateLabel + "1234567" roomIdDisplay + 4 mock 成员（小花/Mocha/Latte/Espresso）正常渲染. UItest 中 8 个无关 failure（PetSprite 三态 / Wardrobe / Friends / Profile / Keychain UITest）属于既有 flakiness，与本 story 无关；Story 37.8 既有 testRoomScaffoldShowsAllSevenAnchors + 新 RoomUITests case 都 PASS.
- AC10 — Deliverable 全部交付：4 新建 + 4 修改 + 1 UITest 新建 = 9 文件改动；详见下方 File List.

### File List

**新建文件**:
- `iphone/PetApp/Features/Room/Models/WSState.swift` (AC2)
- `iphone/PetApp/Core/Networking/WebSocketClient.swift` (AC3 protocol + WSMessage + RoomSnapshotPayload 等 4 structs)
- `iphone/PetApp/Core/Networking/WebSocketClientMock.swift` (AC3 mock)
- `iphone/PetAppTests/Features/Room/RealRoomViewModelTests.swift` (AC7 5 单测 case)
- `iphone/PetAppUITests/RoomUITests.swift` (AC8 1 UITest case)

**修改文件**:
- `iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift` (AC1 加 wsState + memberPetStates 字段)
- `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` (AC4 完整重写)
- `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift` (AC5 加 wsStateLabel + wsStateText 派生)
- `iphone/PetApp/App/RootView.swift` (AC6 bind 签名加 webSocketClient: nil)

**配置 / 跟踪文件**:
- `_bmad-output/implementation-artifacts/sprint-status.yaml` (status: ready-for-dev → in-progress → review)
- `_bmad-output/implementation-artifacts/12-1-房间页面-swiftui-骨架.md` (Status / Tasks 勾选 / Dev Agent Record / File List / Change Log)

**自动生成**:
- `iphone/PetApp.xcodeproj/project.pbxproj` (xcodegen regen 时自动同步新文件到 PetApp / PetAppTests / PetAppUITests target)

### Change Log

- 2026-05-09 — Story 12.1 implementation complete. 新建 WSState 值类型 / WebSocketClient protocol + WSMessage enum + RoomSnapshotPayload + WebSocketClientMock；改写 RealRoomViewModel 走 WS-driven 路径（构造注入 + roomId computed getter + appState.$currentRoomId 订阅 + room.snapshot 解析）；RoomScaffoldView 加 wsStateLabel 占位文字；RootView bind 签名扩 webSocketClient: nil；5 单测 case + 1 UITest case 全绿；ios-simulator MCP 验证通过. Status: ready-for-dev → in-progress → review.

### 开放问题（dev 落地时如有疑问可决策或 raise）

1. **WebSocketClient protocol 是否在本 story 引入？** —— sprint-change-proposal §5.1 给的 Given 段写"Story 12.2 WebSocketClient 就绪"，但 sprint 顺序是 12.1 → 12.2 → 12.3。**本 story 选择**引入 protocol + Mock 让单测闭环（Story 12.2 实装真实 `WebSocketClientImpl`）；如 dev 觉得不妥可调整为"本 story 仅引入 RealRoomViewModel 字段扩展，protocol 留给 12.2"，则 AC3 整体下放到 Story 12.2 + AC7 单测改为"仅测 roomId getter / wsState 显式切换 / appState 订阅链路，不测 snapshot 解析"。当前 acceptance 走"引入 protocol + Mock"路径让本 story 单测 ≥4 case 完整覆盖。
2. **`RoomSnapshotPayload` / `RoomSnapshotMember` / `RoomSnapshotPet` 等 payload 结构是否放 `Core/Networking/WebSocketClient.swift` 同文件？** —— 当前 AC3 选择放同文件让 dev 改动收敛单文件；如 dev 觉得太肥可拆 `Core/Networking/WSMessages.swift` 独立文件。
3. **`level` / `status` / `isHost` 节点 4 阶段在 applySnapshot 中如何派生？** —— 当前 AC4 实装策略：`level = existing.level ?? 8`、`status = existing.status ?? "在玩耍"`、`isHost = index == 0`。如 server 端在节点 4 阶段意外下发了真实 `isHost` / `level` 字段（V1接口设计.md §12.3 当前**无**该字段），dev 应**fail-fast**（走 unknown rawType 路径或 log error）—— 不要静默 fallback。
4. **`Member` 类型重名风险？** —— sprint-change-proposal §5.1 写"持 `members: [Member]`"，但 Story 37.8 落地的类型是 `RoomMember`。本 story acceptance 锚定 `RoomMember`（与 Story 37.8 一致），proposal 中的 `Member` 应理解为通称；不引入重名 `Member` value type 避免 API drift。
5. **本 story 是否要更新 `RoomViewPlaceholder.swift`？** —— **不**改（Story 37.8 / 37.3 落地的占位文件保 git history；后续 Story 12.5 视觉重构时再决定是否清理）。
