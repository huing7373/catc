# Story 12.7: 创建 / 加入 / 退出 use case + 主界面入口完善

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## 故事定位（Epic 12 第 7 条 / Epic 12 收官 story）

这是 Epic 12「iOS - 房间页面 + WebSocket 客户端」的**最后一条 story**，也是节点 4 iOS 端**业务链路对用户完整闭合**的最后一块拼图。前 6 条 story 把"客户端基础设施"全部就绪：

- **12.1** WSState + WebSocketClient protocol + WSMessage enum + WSMessageCodec + RealRoomViewModel.applySnapshot 端到端联通（含 streamRoomId cross-room race 守护 / payload-level room.id 校验 / merge contract / removeDuplicates）.
- **12.2** WebSocketClientImpl 真实 URLSessionWebSocketTask 拨号 + connect await handshake + JWT URL-encode 注入 + WSMessageCodec ping outgoing.
- **12.3** 端到端 wire 联通 + UITEST_ROOM_THREE_MEMBERS env flag.
- **12.4** WSMessage 扩 `memberJoined` / `memberLeft` + RealRoomViewModel.applyMemberJoined / applyMemberLeft + streamRoomId 启动捕获守护 cross-room race（V1 §12.3 钦定 member.joined / member.left payload **不**含 room.id，须靠 stream-lifecycle 守护）.
- **12.5** 自动重连状态机：close code 分类（terminal vs transient） + 指数退避 1/2/4/8/30s + 最大 5 次 + sessionGeneration / streamGeneration 双 counter 隔离 stale task + connectGate generation-scoped + precondition 在 gen 翻新前 + supersede 旧 gate.
- **12.6** 心跳维护：30s ping / 5s pong timeout + requestId 配对 + send error 强制 reconnect + task identity 守护 + atomic closeCode TOCTOU re-check + terminal/transient 等价 cleanup.

**Server 侧 Epic 11 已 done**：所有 room CRUD HTTP API（POST /rooms / POST /rooms/:id/join / POST /rooms/:id/leave）+ GET /home 扩展（room.currentRoomId 真实数据）+ WS room.snapshot / member.joined / member.left 广播都已在 production-ready 状态。

**本 story 的"实装"动作**（一句话概括）：

落地 **3 个 UseCase**（CreateRoomUseCase / JoinRoomUseCase / LeaveRoomUseCase）+ 1 个 RoomEndpoints + 1 个 RoomRepository + RealHomeViewModel / RealRoomViewModel / RealFriendsViewModel 内 5 个 override（onCreateTap / onJoinRoomConfirm / onJoinFriendTap / onLeaveTap）从"占位 log + 直接 mutate appState"升级为"调 UseCase → 成功后让 server 路径通过 setCurrentRoomId 接管 AppState"。**不**改 server / **不**改 WebSocketClientImpl 主干 / **不**改 RealRoomViewModel 主干（subscribeRoomIdConnect 等订阅链路不动；仅 onLeaveTap override 升级）。

**关键边界**：
- **HTTP 是 leave 的唯一权威信号**（V1 §10.5 r10 锁定 + lesson `2026-05-08-ws-disconnect-only-clears-ephemeral-not-membership-11-1-r3.md`）：LeaveRoomUseCase 收到 HTTP 200 即推进；**禁止**等待 close 4007 才 tear down。close 4007 只是 best-effort cleanup，可能丢包 / 比 HTTP 200 晚到 / leaver WS 早断 → 等 4007 会卡死。
- **CreateRoom / JoinRoom 成功后写 AppState 的顺序**：UseCase 内**先**调 `appState.setCurrentRoomId(roomId)` 让 `RealRoomViewModel.subscribeRoomIdConnect` 的 `nil → A` / `A → B` 分支自动触发 `prepareForReconnect()` + `wsState = .connected` + `startConsumingMessages()`（不显式调 `client.connect`）—— **再**让 UI 通过 HomeContainerView 互斥状态机自动切到 RoomView. WS 实际拨号由 `RealRoomViewModel.subscribeRoomIdConnect` 在感知到 `currentRoomId` 变化后**保持触发现有路径**——本 story 在分支内追加调用 `client.connect(roomId:)`（详见 Task 5）.
- **本 story 唯一新增的 connect 触发点**：`RealRoomViewModel.subscribeRoomIdConnect` 内 `nil → A` / `A → B` 分支 + `bind(appState:webSocketClient:)` 首次注入路径，由这两处统一调 `try? await webSocketClient.connect(roomId: roomId)`（包在 `Task { }` 内异步触发；失败由 12.5 reconnect 状态机接管）。**不**让 UseCase 自己调 `connect`（让 RealRoomViewModel 保持 WS lifecycle 唯一控制点；UseCase 仅写 AppState）.

**本 story 落地后立即解锁**：

- Epic 13 节点 4 demo 验收（13.1 / 13.2 / 13.3）—— 完整业务链路（创建房间 / 加入房间 / 离开房间 / 4 人房间满 / 5 人被拒 / 心跳超时 ephemeral 清理）；
- 节点 5+ 后续 epic（Epic 14 server pet state-sync / Epic 15 iOS 房间内多成员渲染）的入口前置条件。

## 核心契约依据（必读）

> 本 story 严格遵循 V1 §10.1 / §10.4 / §10.5 房间 REST 接口 + §12.1 WS 握手契约 + §12.3 client merge contract，所有实装 / 测试不允许偏离。

### 契约 1：V1 §10.1 POST /api/v1/rooms（创建房间）

- **Path**: `/api/v1/rooms` / **Method**: `POST` / **认证**: 需要 Bearer
- **Request body**: 空对象 `{}`
- **Response body** (`code = 0`):
  - `data.room.id`: string（BIGINT 字符串化，AR21）— **本 story 用作后续 setCurrentRoomId 入参**
  - `data.room.creatorUserId`: string
  - `data.room.maxMembers`: number (int) = 4
  - `data.room.memberCount`: number (int) = 1（创建后含创建者自己）
  - `data.room.status`: number (int) = 1 active
- **业务错误码**:
  - `6003` 用户已在房间中（含 race 兜底）→ 本 story 弹 alert "你已经在房间里了"
  - `1009` 服务繁忙（事务回滚 / DB 异常）→ ErrorPresenter `.alert(...)`
- **不会触发**: 6001 / 6002 / 6004 / 6005（房间在事务内才被创建）

Source: `docs/宠物互动App_V1接口设计.md` §10.1 行 1056-1148.

### 契约 2：V1 §10.4 POST /api/v1/rooms/{roomId}/join（加入房间）

- **Path**: `/api/v1/rooms/{roomId}/join` / **Method**: `POST` / **认证**: 需要 Bearer
- **Path 参数**: `roomId: string`（BIGINT 字符串化，1 ≤ length ≤ 20）
- **Request body**: 空对象 `{}`
- **Response body** (`code = 0`):
  - `data.roomId`: string（必填，回带 path roomId）
  - `data.joined`: boolean = true（固定）
- **业务错误码**:
  - `1002` 参数错误（roomId 格式不合法 / 长度 > 20）
  - `6001` 房间不存在 → alert "房间不存在或已被解散"
  - `6002` 房间已满 → alert "房间已满（4/4）"
  - `6003` 用户已在房间中（含目标房间 / 含其他房间）→ alert "你已经在房间里了"
  - `6005` 房间状态异常（status != 1）→ alert "房间已关闭"
  - `1009` 服务繁忙

Source: `docs/宠物互动App_V1接口设计.md` §10.4 行 1409-1503.

### 契约 3：V1 §10.5 POST /api/v1/rooms/{roomId}/leave（退出房间）

- **Path**: `/api/v1/rooms/{roomId}/leave` / **Method**: `POST` / **认证**: 需要 Bearer
- **Path 参数**: `roomId: string`
- **Request body**: 空对象 `{}`
- **Response body** (`code = 0`):
  - `data.roomId`: string
  - `data.left`: boolean = true
- **业务错误码**:
  - `1002` 参数错误
  - `6004` 用户不在房间中（含三种 race 场景：current_room_id == NULL / current_room_id != path roomId / DELETE RowsAffected==0）→ **本 story 视同"已离开"成功路径**：仍然写 `appState.setCurrentRoomId(nil)` + 不弹 alert（用户已经看到 RoomView，再弹 "你不在房间里" 反而困惑）—— 详见 Task 4 leave-idempotent 决策段。
  - `1009` 服务繁忙 → alert + appState 不变（保留 in-room UI 让用户重试）.
- **不会触发**: 6001 / 6002 / 6003 / 6005.

**HTTP 200 是 leave 完成的唯一 authoritative signal**（V1 §10.5 r10 锁定 + lesson `2026-05-08-ws-disconnect-only-clears-ephemeral-not-membership-11-1-r3.md`）：UseCase 收到 200 即立即推进 `appState.setCurrentRoomId(nil)`；**禁止**等待 WS close 4007（best-effort cleanup，可能丢包 / 晚到 / leaver WS 早断）.

Source: `docs/宠物互动App_V1接口设计.md` §10.5 行 1506-1610.

### 契约 4：V1 §12.1 WS 握手（roomId 来源）

- 连接 URL: `{ws_scheme}://{host}/ws/rooms/{roomId}?token={url-encoded}`（`ws` for dev / `wss` for prod，与 baseURL 同源）
- roomId 来源（client 端，按场景分两路）：
  - **首次连接 / 刚完成房间动作（热路径）**：`POST /rooms` 响应的 `data.room.id` 或 `POST /rooms/{roomId}/join` 响应的 `data.roomId`
  - **冷启 / token 刷新 reconnect**：本地持久化最近一次 roomId
- 节点 4 阶段 client 端 roomId 来源由 `appState.currentRoomId` 单 source of truth 承担（本 story UseCase 写入 → RealRoomViewModel.subscribeRoomIdConnect 订阅触发 connect）.

**Server 侧握手校验顺序**（client 不感知，仅了解失败语义）:
1. token 校验 → close 4001（client 触发 silent re-login by Story 5.4）
2. roomId 路径参数 → close 4002（不应发生 —— UseCase 已传 server 自己刚返回的 roomId）
3. token 合法 → close 4001 续
4. room 存在性 → close 4004（**不**自动重连；UX 提示 + 回退）
5. 用户房间归属 → close 4003（**不**自动重连；UX 提示 + 回退）

本 story **不处理** 4001 / 4002 / 4003 / 4004 → 由 12.5 reconnect 状态机已分类完成（terminal close）；UseCase 层仅关心 HTTP 路径.

Source: `docs/宠物互动App_V1接口设计.md` §12.1 行 1648-1757.

### 契约 5：V1 §12.3 client merge contract（room.snapshot enrich）

server 在 WS 握手成功后必发 `room.snapshot`（authoritative for roster），但其权威性是 **enrich/correct** 而**非** wipe-out：
- snapshot 中**非空字段**覆盖 client 已有值
- snapshot 中**空字符串字段** = "我不知道" placeholder → **保留** client 已有值
- snapshot 中**未出现字段** → 保留 client 已有值

本 story **不改** RealRoomViewModel.applySnapshot 主干（12.1 已实装）；UseCase 仅写 AppState → 触发 RealRoomViewModel.subscribeRoomIdConnect → connect WS → 收到 snapshot → applySnapshot 自动接管 roster 渲染.

Source: `docs/宠物互动App_V1接口设计.md` §12.1.3 行 1696-1700 + §12.3 client merge contract.

### 契约 6：iOS 工程结构 §6.x UseCase / Repository 层

- **UseCase** 位置: `iphone/PetApp/Features/<Domain>/UseCases/<Name>UseCase.swift`
  - protocol-first（`<Name>UseCaseProtocol: Sendable` + `Default<Name>UseCase: <Protocol>`）
  - `func execute(...) async throws -> <Output>`
  - 仅依赖 Repository（不直接持 APIClient）
  - 错误**原样透传** APIError（不在 UseCase 内吞错或转码；ViewModel 层接 ErrorPresenter）
- **Repository** 位置: `iphone/PetApp/Features/<Domain>/Repositories/<Name>Repository.swift`
  - protocol-first（`<Name>RepositoryProtocol: Sendable` + `Default<Name>Repository: <Protocol>` 是 `struct`）
  - 持 `apiClient: APIClientProtocol`
  - 透传 APIError
- **Endpoints** 位置: `iphone/PetApp/Features/<Domain>/UseCases/<Name>Endpoints.swift`
  - `enum <Name>Endpoints { static func <action>(...) -> Endpoint }`
  - path **必含** `/api/v1` 前缀

参考既有: `Features/Auth/UseCases/GuestLoginUseCase.swift`, `Features/Home/UseCases/LoadHomeUseCase.swift`, `Features/Home/Repositories/HomeRepository.swift`, `Features/Home/UseCases/HomeEndpoints.swift`.

Source: `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §6.x.

## Story

As an iPhone 用户,
I want 我可以从 Home Tab 创建队伍 / 输入房间号加入 / 在好友卡片上加入好友的房间，从 Room 视图离开房间，跨 Tab 数据自动同步,
So that 节点 4 业务链路对我完整闭合（创建 → 加入 → 看见房间内成员 → 离开 → 回到主界面）.

## Acceptance Criteria

**Given** Story 12.1-12.6 完成 + Story 11.3 / 11.4 / 11.5 server 端房间 CRUD 接口可用 + Story 37.3 / 37.7 / 37.8 / 37.10 / 37.12 UI Scaffold 已就绪 + Story 37.4 AppState 就绪
**When** 完成本 story
**Then** 实装：

### AC1：CreateRoomUseCase（POST /api/v1/rooms）

- 在 `iphone/PetApp/Features/Room/UseCases/CreateRoomUseCase.swift` 实装 `CreateRoomUseCaseProtocol` + `DefaultCreateRoomUseCase`：
  - `func execute() async throws -> String`（返回 `roomId`）
  - 内部调 `roomRepository.createRoom() → CreateRoomResponse` → 取 `response.data.room.id` 写 `appState.setCurrentRoomId(roomId)` → 返回 roomId
  - **顺序**: 先 set roomId（让 RealRoomViewModel sink 准备好 stream），后 return（让 caller 决定下一步 UI 流程）
  - APIError 原样透传（含 `.business(code: 6003, ...)` / `.business(code: 1009, ...)` / `.network` / `.unauthorized`）
- 错误处理（caller 层 = RealHomeViewModel.onCreateTap）：
  - 成功 → no-op（HomeContainerView 互斥状态机自动切 RoomView）
  - `.business(code: 6003, ...)` → `errorPresenter.present` 走 `.alert("提示", "你已经在房间里了")`（不需要附 onRetry —— 用户应主动 leave 再创建；caller 层显式 catch 该 case 后重抛 / 转为 alert）
  - 其它 → 透传给 ErrorPresenter（默认 mapper 走 `.retry` / `.alert`）
- **单元测试覆盖**（≥3 case，纯 XCTest + Mock RoomRepository + Mock AppState 类似 testing helper）:
  - happy: repo 返回 roomId="3001" → execute() 返回 "3001" + appState.currentRoomId == "3001"
  - edge: repo throw `.business(code: 6003, ...)` → execute() rethrow 同 case + appState.currentRoomId 不变
  - edge: repo throw `.network(...)` → execute() rethrow + appState.currentRoomId 不变

### AC2：JoinRoomUseCase（POST /api/v1/rooms/{roomId}/join）

- 在 `iphone/PetApp/Features/Room/UseCases/JoinRoomUseCase.swift` 实装 `JoinRoomUseCaseProtocol` + `DefaultJoinRoomUseCase`：
  - `func execute(roomId: String) async throws`（无返回值；roomId 已知由 caller 传入）
  - 内部调 `roomRepository.joinRoom(roomId:) → JoinRoomResponse`，**校验 `response.data.roomId == request.roomId`**（防 server bug；不一致 → throw `APIError.decoding(...)` 包装 `JoinRoomMismatchError`，**不**写 appState）
  - 校验通过 → `appState.setCurrentRoomId(roomId)`
  - APIError 原样透传
- 错误处理（caller 层 = RealHomeViewModel.onJoinRoomConfirm / RealFriendsViewModel.onJoinFriendTap）：
  - 成功 → no-op（HomeContainerView 互斥状态机自动切 RoomView）
  - `.business(code: 6001, ...)` → alert "房间不存在或已被解散"
  - `.business(code: 6002, ...)` → alert "房间已满（4/4）"
  - `.business(code: 6003, ...)` → alert "你已经在房间里了"
  - `.business(code: 6005, ...)` → alert "房间已关闭"
  - `.business(code: 1002, ...)` → alert "房间号格式不合法"（理论不应发生 —— modal trim 已挡住空字符串；server 兜底）
  - 其它 → 透传 ErrorPresenter
- **单元测试覆盖**（≥4 case）:
  - happy: repo 返回 `data.roomId == "3001"` + caller 传 "3001" → execute() 不抛 + appState.currentRoomId == "3001"
  - edge: repo throw `.business(code: 6002, ...)` → rethrow + appState 不变
  - edge: repo throw `.business(code: 6001, ...)` → rethrow + appState 不变
  - edge: repo 返回 `data.roomId == "9999"` 但 caller 传 "3001"（server bug）→ throw `.decoding(...)` + appState 不变

### AC3：LeaveRoomUseCase（POST /api/v1/rooms/{roomId}/leave）

- 在 `iphone/PetApp/Features/Room/UseCases/LeaveRoomUseCase.swift` 实装 `LeaveRoomUseCaseProtocol` + `DefaultLeaveRoomUseCase`：
  - `func execute() async throws`（roomId 从 `appState.currentRoomId` 取；nil → no-op return，不抛错——leave 是 idempotent 操作）
  - 内部调 `roomRepository.leaveRoom(roomId: currentRoomId) → LeaveRoomResponse`
  - **HTTP 200 = authoritative leave 信号**（V1 §10.5 r10 锁定）：成功后**立即** `appState.setCurrentRoomId(nil)`（**不**等 WS close 4007）
  - **6004 视同成功路径**（leave-idempotent 决策）：用户已不在该房间（race / 本地 stale）→ 仍写 `appState.setCurrentRoomId(nil)` + **不**重抛 6004（重抛会让 caller 弹 alert "你不在房间里"，体验割裂——用户主动点 leave 已表达"我要离开"意图）。**通过 try-catch 拦截 6004 case 后吞掉**；其它 .business code 全部透传。
  - 其它 APIError 透传（保留 in-room UI 让用户重试）
- 错误处理（caller 层 = RealRoomViewModel.onLeaveTap）：
  - 成功 / 6004 → no-op（HomeContainerView 自动切回 HomeView）
  - 其它 → 透传给 ErrorPresenter
- **单元测试覆盖**（≥4 case）:
  - happy: appState.currentRoomId="3001" + repo 返回 left=true → execute() 不抛 + appState.currentRoomId == nil
  - happy: appState.currentRoomId=nil → execute() 立即 return（不调 repo）
  - edge: repo throw `.business(code: 6004, ...)` → execute() **不抛** + appState.currentRoomId == nil（leave-idempotent）
  - edge: repo throw `.business(code: 1009, ...)` → execute() rethrow + appState.currentRoomId 保留原值（让用户在 RoomView 内重试）

### AC4：RoomEndpoints + RoomRepository + DTO

- 在 `iphone/PetApp/Features/Room/UseCases/RoomEndpoints.swift` 实装 `RoomEndpoints` enum：
  - `static func createRoom() -> Endpoint`（POST `/api/v1/rooms`，body=`Data("{}".utf8)`，requiresAuth=true）
  - `static func joinRoom(roomId: String) -> Endpoint`（POST `/api/v1/rooms/{roomId}/join`，body=`Data("{}".utf8)`，requiresAuth=true）
  - `static func leaveRoom(roomId: String) -> Endpoint`（POST `/api/v1/rooms/{roomId}/leave`，body=`Data("{}".utf8)`，requiresAuth=true）
- 在 `iphone/PetApp/Features/Room/Repositories/RoomRepository.swift` 实装：
  - `protocol RoomRepositoryProtocol: Sendable`
    - `func createRoom() async throws -> CreateRoomResponse`
    - `func joinRoom(roomId: String) async throws -> JoinRoomResponse`
    - `func leaveRoom(roomId: String) async throws -> LeaveRoomResponse`
  - `struct DefaultRoomRepository: RoomRepositoryProtocol`（持 `apiClient: APIClientProtocol`）
- DTO 位置 `iphone/PetApp/Features/Room/Models/RoomEndpointDTO.swift`（与 HomeData 同模式：wire DTO Codable + Domain 类型解耦）：
  - `struct CreateRoomResponse: Codable { let room: RoomDTO }` + `struct RoomDTO: Codable { let id: String; let creatorUserId: String; let maxMembers: Int; let memberCount: Int; let status: Int }`
  - `struct JoinRoomResponse: Codable { let roomId: String; let joined: Bool }`
  - `struct LeaveRoomResponse: Codable { let roomId: String; let left: Bool }`
- 单元测试 `iphone/PetAppTests/Features/Room/Repositories/RoomRepositoryTests.swift`（≥3 case）：
  - happy: createRoom → mock APIClient 收到 endpoint(.createRoom) → 返回 mock CreateRoomResponse → repo 透传
  - happy: joinRoom("3001") → mock APIClient 收到 endpoint(.joinRoom("3001")) + body 为空对象 + path 含 "/3001/join"
  - happy: leaveRoom("3001") → mock APIClient 收到 endpoint(.leaveRoom("3001")) + path 含 "/3001/leave"

### AC5：RealHomeViewModel.onCreateTap / onJoinRoomConfirm 升级（接 UseCase）

- 升级 `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift`：
  - 新增字段 `private let createRoomUseCase: CreateRoomUseCaseProtocol?`（`nil` 默认；构造 / bind 注入）
  - 新增字段 `private let joinRoomUseCase: JoinRoomUseCaseProtocol?`
  - 新增字段 `private let errorPresenter: ErrorPresenter?`
  - 新增 `bind(createRoomUseCase:joinRoomUseCase:errorPresenter:)` 入口（与既有 `bind(appState:)` / `bind(loadHomeUseCase:errorPresenter:)` 同模式）
  - `override func onCreateTap()` 升级：
    - 起 `Task { @MainActor in ... }`：调 `try await self.createRoomUseCase?.execute()`
    - 成功 → no-op（appState 已被 UseCase 写入；UI 自动切）
    - catch `APIError.business(code: 6003, ...)` → `self.errorPresenter?.present(...)` 走 `.alert("提示", "你已经在房间里了")`（用 `presentAlert(title:message:)` 入口）
    - catch 其它 error → `self.errorPresenter?.present(error)` 默认 mapper 路径
  - `override func onJoinRoomConfirm(roomId: String)` 升级：
    - **先**写 `self.showJoinModal = false`（关 modal；与现有 round 1 P1 lesson 保持 mutate-state 顺序：先关 sheet 后调 UseCase 防 sheet 还在但底层 view 已切走的视觉错乱）
    - 起 `Task { @MainActor in ... }`：调 `try await self.joinRoomUseCase?.execute(roomId: roomId)`
    - 成功 → no-op
    - catch `APIError.business(code: 6001, ...)` → alert "房间不存在或已被解散"
    - catch `APIError.business(code: 6002, ...)` → alert "房间已满（4/4）"
    - catch `APIError.business(code: 6003, ...)` → alert "你已经在房间里了"
    - catch `APIError.business(code: 6005, ...)` → alert "房间已关闭"
    - catch `APIError.business(code: 1002, ...)` → alert "房间号格式不合法"
    - catch 其它 → `self.errorPresenter?.present(error)`
  - **删除**老占位行为里"直接 mutate `localAppState?.setCurrentRoomId(roomId)`"那行（UseCase 接管；保留旧行为会双写 appState 触发 sink 两次 → 两次 connect race）；保留 `localAppState` 字段（其它 override 仍可能用，避免大改）.
- **单元测试覆盖**（≥5 case，扩展现有 `RealHomeViewModelTests.swift` —— 注入 mock CreateRoomUseCase / JoinRoomUseCase / ErrorPresenter）:
  - happy: onCreateTap → mock CreateRoomUseCase.execute() 调用 1 次 + 不弹 alert
  - happy: onJoinRoomConfirm("3001") → showJoinModal 立即 false + mock JoinRoomUseCase.execute("3001") 调用 1 次
  - edge: CreateRoomUseCase throw 6003 → ErrorPresenter.present(...) 收到 `.alert(title: "提示", message: 含 "已经在房间")` 类型呈现
  - edge: JoinRoomUseCase throw 6002 → ErrorPresenter 收到含 "房间已满"
  - edge: JoinRoomUseCase throw 1009 (network/服务繁忙) → ErrorPresenter 收到（默认 mapper 路径，alert / retry）
  - happy: onJoinRoomConfirm("3001") + UseCase 成功 → showJoinModal=false + appState.currentRoomId 由 UseCase 写入（断言 mock useCase 内收到的 roomId 即可，不直接断言 appState —— UseCase 单测已覆盖）

### AC6：RealRoomViewModel.onLeaveTap 升级（接 UseCase）+ subscribeRoomIdConnect 内 connect 触发

- 升级 `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`：
  - 新增字段 `private let leaveRoomUseCase: LeaveRoomUseCaseProtocol?`
  - 新增字段 `private let errorPresenter: ErrorPresenter?`
  - **扩展** `bind(appState:webSocketClient:)` 签名为 `bind(appState:webSocketClient:leaveRoomUseCase:errorPresenter:)`（默认 nil 让既有 caller 不破）
  - `override func onLeaveTap()` 升级（基类 `onLeaveTap` 是 abstract `fatalError` —— 本子类必须 override）：
    - 起 `Task { @MainActor in ... }`：调 `try await self.leaveRoomUseCase?.execute()`
    - 成功 → no-op（UseCase 已写 `setCurrentRoomId(nil)` → subscribeRoomIdConnect 的 `A → nil` 分支走 disconnect + 清 roster + wsState=.disconnected → HomeContainerView 自动切回 HomeView）
    - catch APIError → `self.errorPresenter?.present(error)` 默认 mapper 路径（保留 in-room UI 让用户重试）
    - **fallback**（leaveRoomUseCase==nil 时）：直接 `appState?.setCurrentRoomId(nil)` 走旧 mock 行为（让 RootView 老 wire 路径不破，UITest 仍可走 onLeaveTap 切回 idle）
  - **subscribeRoomIdConnect 内追加 connect 触发**（关键改动 —— 让 `nil → A` / `A → B` 自动拨号 WS）：
    - `nil → A` 分支：原有 `prepareForReconnect() + wsState=.connected + startConsumingMessages()` 之后追加 `Task { try? await self.webSocketClient?.connect(roomId: roomId) }`（wrap in Task 让 sink 闭包不阻塞；失败由 12.5 reconnect 状态机接管）
    - `A → B` 分支：同 `nil → A`（disconnect 旧 → prepareForReconnect → wsState=.connected → startConsumingMessages → connect new）
    - **其它分支不动**：`nil → nil` no-op / `A → nil` 仅 disconnect（不 connect 新房间）/ `A → A` 由 removeDuplicates 拦截
  - **bind() 内 first-injection / swap 路径**也追加同样 connect 触发（与 sink 路径对称；既有 `if clientChanged && webSocketClient != nil && lastObservedRoomId != nil` 块内 `startConsumingMessages()` 之后追加 `Task { try? await self.webSocketClient?.connect(roomId: lastObservedRoomId!) }`）
- **单元测试覆盖**（≥5 case，扩展现有 `RealRoomViewModelTests.swift`）:
  - happy: onLeaveTap + appState.currentRoomId="3001" → mock LeaveRoomUseCase.execute() 调用 1 次
  - happy: LeaveRoomUseCase 成功 → mock LeaveRoomUseCase 写 appState.currentRoomId = nil → subscribeRoomIdConnect 的 `A → nil` 分支触发 → wsState=.disconnected + members=[] + memberPetStates=[:]
  - edge: LeaveRoomUseCase throw .business(1009) → ErrorPresenter.present 收到 + appState.currentRoomId 保留 "3001"（不 nil；用户仍在 RoomView）
  - happy: subscribeRoomIdConnect `nil → A` → mock WebSocketClient.connect(roomId:) 被调用 1 次（用 invocations 数组验证）
  - happy: subscribeRoomIdConnect `A → B` → mock WebSocketClient.connect(roomId: "B") 被调用 + 先调用 disconnect 旧 client + members 清空

### AC7：RealFriendsViewModel.onJoinFriendTap 升级（接 JoinRoomUseCase）

- 升级 `iphone/PetApp/Features/Friends/ViewModels/RealFriendsViewModel.swift`：
  - 新增字段 `private let joinRoomUseCase: JoinRoomUseCaseProtocol?` + `private let errorPresenter: ErrorPresenter?`
  - 新增 bind 入口或参数化既有 `bind(appState:)` → `bind(appState:joinRoomUseCase:errorPresenter:)`
  - `override func onJoinFriendTap(_ friend: Friend)` 升级：
    - 解析 `friend.currentRoomId: String?`；nil → 弹 toast "好友不在房间中"（理论不应发生：UI 仅在 friend.currentRoomId != nil 时显示"加入"按钮；防御性兜底）
    - 起 `Task { @MainActor in ... }`：调 `try await self.joinRoomUseCase?.execute(roomId: friendRoomId)`
    - 成功 → no-op（HomeContainerView 自动切到 RoomView；好友帮我 join 的房间）
    - catch 错误 → 同 RealHomeViewModel.onJoinRoomConfirm 错误映射逻辑（6001 / 6002 / 6003 / 6005 / 1002 → 对应 alert）
- **单元测试覆盖**（≥3 case，扩展 `RealFriendsViewModelTests.swift`）：
  - happy: onJoinFriendTap(friend with currentRoomId="3001") → mock JoinRoomUseCase.execute("3001") 调用 1 次
  - edge: onJoinFriendTap(friend with currentRoomId=nil) → 不调 UseCase + ErrorPresenter 收到 toast
  - edge: JoinRoomUseCase throw 6002 → ErrorPresenter 收到 alert "房间已满"

### AC8：AppContainer wire（生产路径）

- 升级 `iphone/PetApp/App/AppContainer.swift`：
  - 新增 factory `func makeRoomRepository() -> RoomRepositoryProtocol`（与 makeHomeRepository 同模式）
  - 新增 factory `func makeCreateRoomUseCase(appState:) -> CreateRoomUseCaseProtocol`
  - 新增 factory `func makeJoinRoomUseCase(appState:) -> JoinRoomUseCaseProtocol`
  - 新增 factory `func makeLeaveRoomUseCase(appState:) -> LeaveRoomUseCaseProtocol`
- 升级 `iphone/PetApp/App/RootView.swift`：
  - 既有 `.onAppear` 内 `homeViewModel.bind(appState: appState)` 之后追加：
    - `if let realHomeVM = homeViewModel as? RealHomeViewModel { realHomeVM.bind(createRoomUseCase: container.makeCreateRoomUseCase(appState: appState), joinRoomUseCase: container.makeJoinRoomUseCase(appState: appState), errorPresenter: container.errorPresenter) }`
  - 既有 `realRoomVM.bind(appState: appState, webSocketClient: nil)` 升级为：
    - `realRoomVM.bind(appState: appState, webSocketClient: container.webSocketClient, leaveRoomUseCase: container.makeLeaveRoomUseCase(appState: appState), errorPresenter: container.errorPresenter)`（**关键**：把 webSocketClient 从 nil 改为真实 container.webSocketClient —— 12.7 落地后由本 story 接通）
  - 既有 `realFriendsVM.bind(appState: appState)` 升级为：
    - `realFriendsVM.bind(appState: appState, joinRoomUseCase: container.makeJoinRoomUseCase(appState: appState), errorPresenter: container.errorPresenter)`

### AC9：UI 测试覆盖（XCUITest）

- 扩展 `iphone/PetAppUITests/RoomUITests.swift`（或新建 `RoomLifecycleUITests.swift`）：
  - **本 story UI 测试不调真实 server**——用 mock APIClient 路径（与现有 UITEST 路径对齐：通过 `UITEST_ROOM_THREE_MEMBERS` 类似 env flag 配 stub repo）.
  - 单 case 覆盖（不要求多端真机联调，那是 Epic 13）：
    - happy: launch app → Home Tab idle → 点 `homeTeamIdleCard_create` → 等待 1s → 验证 RoomView 出现（`AccessibilityID.Room.viewPlaceholder` 或 RoomScaffoldView 顶层 a11y identifier 可定位）+ Tab Bar 仍可见 + 当前 Tab 仍是 Home（accessibility selected = "tab_home"）
- **不**要求覆盖错误 alert / 多种 close code path（那是 Epic 13 节点 4 demo 验收范围）.

### AC10：sprint-status.yaml + retrospective 标记

- 本 story dev-story 完成后切 `12-7-创建-加入-退出-use-case-主界面入口完善` 为 `review`，code-review 通过后切 `done`.
- Epic 12 全部 7 条 story done 后，由后续 retrospective 流程切 `epic-12: done` + `epic-12-retrospective` 处理（不在本 story 范围）.

## Tasks / Subtasks

- [x] Task 1: RoomEndpoints + RoomRepository + DTO (AC: #4)
  - [x] 1.1 新建 `iphone/PetApp/Features/Room/UseCases/RoomEndpoints.swift`：`enum RoomEndpoints { static func createRoom() / joinRoom(roomId:) / leaveRoom(roomId:) }`，path 含 `/api/v1` 前缀，body 为 `Data("{}".utf8)`，requiresAuth=true
  - [x] 1.2 新建 `iphone/PetApp/Features/Room/Models/RoomEndpointDTO.swift`：`CreateRoomResponse` / `RoomDTO` / `JoinRoomResponse` / `LeaveRoomResponse`（全 Codable，字段名按 V1 §10.1 / §10.4 / §10.5 严格对齐）
  - [x] 1.3 新建 `iphone/PetApp/Features/Room/Repositories/RoomRepository.swift`：`RoomRepositoryProtocol` + `DefaultRoomRepository`（持 apiClient）
  - [x] 1.4 新建 `iphone/PetAppTests/Features/Room/Repositories/RoomRepositoryTests.swift`（≥3 case：createRoom / joinRoom path 校验 / leaveRoom path 校验）
  - [x] 1.5 跑 `bash iphone/scripts/build.sh --test`：所有既有 + 新增测试全绿

- [x] Task 2: CreateRoomUseCase (AC: #1)
  - [x] 2.1 新建 `iphone/PetApp/Features/Room/UseCases/CreateRoomUseCase.swift`：`CreateRoomUseCaseProtocol` + `DefaultCreateRoomUseCase`（持 `roomRepository: RoomRepositoryProtocol` + `appState: AppState`）
  - [x] 2.2 `func execute() async throws -> String`：调 repo.createRoom() → 取 response.data.room.id → 调 `await MainActor.run { appState.setCurrentRoomId(roomId) }` → return roomId
  - [x] 2.3 新建 `iphone/PetAppTests/Features/Room/UseCases/CreateRoomUseCaseTests.swift`（≥3 case：happy / 6003 透传 / network 透传）
  - [x] 2.4 跑 build + test

- [x] Task 3: JoinRoomUseCase (AC: #2)
  - [x] 3.1 新建 `iphone/PetApp/Features/Room/UseCases/JoinRoomUseCase.swift`：`JoinRoomUseCaseProtocol` + `DefaultJoinRoomUseCase`（持 `roomRepository` + `appState`）
  - [x] 3.2 `func execute(roomId: String) async throws`：调 repo.joinRoom(roomId:) → 校验 `response.data.roomId == roomId` 不一致抛 `.decoding(...)` 包装 `JoinRoomMismatchError` → 调 `setCurrentRoomId(roomId)`
  - [x] 3.3 新建 `iphone/PetAppTests/Features/Room/UseCases/JoinRoomUseCaseTests.swift`（≥4 case：happy / 6002 透传 / 6001 透传 / mismatch 抛 .decoding）
  - [x] 3.4 跑 build + test

- [x] Task 4: LeaveRoomUseCase (AC: #3)
  - [x] 4.1 新建 `iphone/PetApp/Features/Room/UseCases/LeaveRoomUseCase.swift`：`LeaveRoomUseCaseProtocol` + `DefaultLeaveRoomUseCase`（持 `roomRepository` + `appState`）
  - [x] 4.2 `func execute() async throws`：从 `await appState.currentRoomId` 取；nil → return；else 调 repo.leaveRoom(roomId:) → 成功后 `setCurrentRoomId(nil)`
  - [x] 4.3 6004 leave-idempotent 兜底：`do { try await ... } catch APIError.business(let code, _, _) where code == 6004 { await MainActor.run { appState.setCurrentRoomId(nil) } } catch { throw }`
  - [x] 4.4 新建 `iphone/PetAppTests/Features/Room/UseCases/LeaveRoomUseCaseTests.swift`（≥4 case：happy / nil currentRoomId 早 return / 6004 视同成功 / 1009 透传）
  - [x] 4.5 跑 build + test

- [x] Task 5: RealRoomViewModel.onLeaveTap + subscribeRoomIdConnect connect 触发 (AC: #6)
  - [x] 5.1 修改 `RealRoomViewModel.swift`：新增字段 `private let leaveRoomUseCase: LeaveRoomUseCaseProtocol?` + `private let errorPresenter: ErrorPresenter?`
  - [x] 5.2 扩展 `bind(appState:webSocketClient:leaveRoomUseCase:errorPresenter:)` 签名（默认参数让既有 caller 不破）
  - [x] 5.3 新增 `override func onLeaveTap()`：起 Task → 调 `try await self.leaveRoomUseCase?.execute()`；fallback 路径（useCase==nil）保留 `appState?.setCurrentRoomId(nil)` 走 mock 老行为；catch 调 errorPresenter
  - [x] 5.4 修改 `subscribeRoomIdConnect` 的 `nil → A` / `A → B` 分支：在既有 `startConsumingMessages()` 之后追加 `Task { [weak self] in try? await self?.webSocketClient?.connect(roomId: roomId) }`
  - [x] 5.5 修改 `bind` 内 first-injection / swap 路径：`if clientChanged && ... && lastObservedRoomId != nil` 块内 startConsumingMessages 后追加同样 connect Task
  - [x] 5.6 扩展 `RealRoomViewModelTests.swift`（≥5 case：onLeaveTap 调 UseCase / 成功后 wsState=.disconnected / 1009 透传 / nil→A 触发 connect / A→B 触发 disconnect+connect）
  - [x] 5.7 跑 build + test

- [x] Task 6: RealHomeViewModel.onCreateTap + onJoinRoomConfirm 升级 (AC: #5)
  - [x] 6.1 修改 `RealHomeViewModel.swift`：新增字段 `private let createRoomUseCase: CreateRoomUseCaseProtocol?` + `private let joinRoomUseCase: JoinRoomUseCaseProtocol?` + `private let errorPresenter: ErrorPresenter?`
  - [x] 6.2 新增 `bind(createRoomUseCase:joinRoomUseCase:errorPresenter:)` 入口（与既有 bind 同模式；幂等 / 不破基类 bind 的"first-time-only"约定）
  - [x] 6.3 改写 `override func onCreateTap()`：起 Task 调 UseCase + catch business(6003) 弹 alert "你已经在房间里了" + catch 其它 → errorPresenter.present(error)
  - [x] 6.4 改写 `override func onJoinRoomConfirm(roomId:)`：先写 showJoinModal=false，后起 Task 调 UseCase；catch 6001/6002/6003/6005/1002 各自弹对应 alert + catch 其它 → errorPresenter.present(error)
  - [x] 6.5 **删除**老占位行为里 `localAppState?.setCurrentRoomId(roomId)` 那行（避免 UseCase + 直写 双触发 sink）；保留 `localAppState` 字段
  - [x] 6.6 扩展 `RealHomeViewModelTests.swift`（≥5 case，详见 AC5 单测列表）
  - [x] 6.7 跑 build + test

- [x] Task 7: RealFriendsViewModel.onJoinFriendTap 升级 (AC: #7)
  - [x] 7.1 修改 `RealFriendsViewModel.swift`：新增字段 + bind 扩展
  - [x] 7.2 改写 `override func onJoinFriendTap(_ friend:)`：解析 friend.currentRoomId → 起 Task 调 JoinRoomUseCase + catch 同 RealHomeViewModel
  - [x] 7.3 扩展 `RealFriendsViewModelTests.swift`（≥3 case，详见 AC7）
  - [x] 7.4 跑 build + test

- [x] Task 8: AppContainer + RootView wire (AC: #8)
  - [x] 8.1 修改 `AppContainer.swift`：新增 `makeRoomRepository / makeCreateRoomUseCase / makeJoinRoomUseCase / makeLeaveRoomUseCase` 4 个 factory
  - [x] 8.2 修改 `RootView.swift` `.onAppear` 内：升级 realHomeVM / realRoomVM / realFriendsVM 的 bind 调用，注入对应 UseCase + errorPresenter；**关键**：realRoomVM 的 webSocketClient 从 nil 升级为 `container.webSocketClient`（节点 4 真实 client 接通）
  - [x] 8.3 跑 build + test

- [x] Task 9: UI 测试 + 模拟器手动验证 (AC: #9)
  - [x] 9.1 扩展 / 新建 UITest case：launch → Home Tab idle → 点 homeTeamIdleCard_create → 验证 RoomView 出现
  - [x] 9.2 **必跑**：`bash iphone/scripts/build.sh --test`（全测试绿）
  - [x] 9.3 **必跑**：iphone-simulator MCP 手动验证（按 CLAUDE.md "iOS UI 验证" 段 standard verify workflow）：
    - install_app → launch_app(terminate_running: true)
    - ui_view 看 Home idle 视觉
    - ui_tap "创建队伍" / "加入队伍" / 输入 modal 验证视觉切到 RoomView
    - ui_view + ui_describe_all 验证 a11y identifier 都对
    - 单 happy path 录像或截图（不强制录像；本 story 范围内 demo 留给 Story 13.2 节点 4 demo 验收）

- [x] Task 10: sprint-status 切 review (AC: #10)
  - [ ] 10.1 dev-story 完成后切 `12-7-创建-加入-退出-use-case-主界面入口完善` 状态为 `review`
  - [ ] 10.2 触发 code-review；通过后由 story-done 切 `done`

## Dev Notes

### Project Structure（新建文件清单）

| 文件路径 | 用途 |
|---|---|
| `iphone/PetApp/Features/Room/UseCases/RoomEndpoints.swift` | 3 个 endpoint 工厂 |
| `iphone/PetApp/Features/Room/UseCases/CreateRoomUseCase.swift` | CreateRoom UseCase |
| `iphone/PetApp/Features/Room/UseCases/JoinRoomUseCase.swift` | JoinRoom UseCase |
| `iphone/PetApp/Features/Room/UseCases/LeaveRoomUseCase.swift` | LeaveRoom UseCase |
| `iphone/PetApp/Features/Room/Repositories/RoomRepository.swift` | RoomRepository protocol + Default |
| `iphone/PetApp/Features/Room/Models/RoomEndpointDTO.swift` | CreateRoom / JoinRoom / LeaveRoom Response DTO |
| `iphone/PetAppTests/Features/Room/Repositories/RoomRepositoryTests.swift` | Repo 单测 |
| `iphone/PetAppTests/Features/Room/UseCases/CreateRoomUseCaseTests.swift` | CreateRoom UseCase 单测 |
| `iphone/PetAppTests/Features/Room/UseCases/JoinRoomUseCaseTests.swift` | JoinRoom UseCase 单测 |
| `iphone/PetAppTests/Features/Room/UseCases/LeaveRoomUseCaseTests.swift` | LeaveRoom UseCase 单测 |

### Project Structure（修改文件清单）

| 文件路径 | 改动 |
|---|---|
| `iphone/PetApp/App/AppContainer.swift` | 新增 4 个 factory（makeRoomRepository / makeCreate-/Join-/LeaveRoomUseCase） |
| `iphone/PetApp/App/RootView.swift` | `.onAppear` 内升级 realHomeVM / realRoomVM / realFriendsVM 的 bind 调用 |
| `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift` | onCreateTap / onJoinRoomConfirm 升级；新增 bind 入口；删除直写 setCurrentRoomId |
| `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` | onLeaveTap override；subscribeRoomIdConnect / bind 内追加 client.connect 触发；扩展 bind 签名 |
| `iphone/PetApp/Features/Friends/ViewModels/RealFriendsViewModel.swift` | onJoinFriendTap 升级；新增 bind 入口 |
| `iphone/PetAppTests/Features/Home/ViewModels/RealHomeViewModelTests.swift` | 新增 5+ case |
| `iphone/PetAppTests/Features/Room/ViewModels/RealRoomViewModelTests.swift` | 新增 5+ case |
| `iphone/PetAppTests/Features/Friends/ViewModels/RealFriendsViewModelTests.swift` | 新增 3+ case |
| `iphone/PetAppUITests/RoomUITests.swift`（或新建 RoomLifecycleUITests.swift） | 1 个 happy path UI case |

### Project Structure（**禁止修改**清单）

| 文件路径 | 原因 |
|---|---|
| `iphone/PetApp/Core/Networking/WebSocketClient.swift` / `WebSocketClientImpl.swift` / `WebSocketClientMock.swift` / `WSMessage*` / `WSMessageCodec.swift` | Story 12.1-12.6 已稳定；本 story 仅 caller 层调 connect / disconnect |
| `iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift`（基类） / `MockRoomViewModel.swift` / `RoomScaffoldDefaults.swift` | 接缝设计已稳定；本 story 仅在 Real 子类追加 override |
| `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`（基类） / `MockHomeViewModel.swift` | 同上；abstract method 签名已锁 |
| `iphone/PetApp/Features/Friends/ViewModels/FriendsViewModel.swift`（基类） / `MockFriendsViewModel.swift` | 同上 |
| `iphone/PetApp/Features/Home/Views/HomeView.swift` / `JoinRoomModal/*.swift` | UI Scaffold 已稳定（Story 37.7 / 37.12）；本 story **零 View 层改动** |
| `iphone/PetApp/App/AppState.swift` | setCurrentRoomId 入口已就位（Story 37.4）；本 story 仅调用 |
| **server 任何目录** | Epic 11 已 done；本 story **不**改 server |

### showJoinModal mutation 顺序（lesson 复用）

`onJoinRoomConfirm(roomId:)` override 内**必须先写** `showJoinModal = false`（关 sheet）**后调** UseCase（异步）。理由：

1. SwiftUI sheet 的 `isPresented` 双向绑定立即关闭；用户视觉上看到 modal 退场
2. UseCase 异步路径在 sheet 关闭后才把 appState.currentRoomId 写入 → HomeContainerView 才切 RoomView
3. 不可反序：若先调 UseCase（成功后才关 sheet），HomeContainerView 已切到 RoomView 但 modal 还在最上层 → 用户看到 RoomView 上盖 modal 的视觉错乱

继承自既有 `RealHomeViewModel.onJoinRoomConfirm` round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`.

### connect 触发的责任划分（关键边界）

**UseCase 层**只写 `appState.setCurrentRoomId(roomId)`；**不**调 `webSocketClient.connect(...)`。理由：

- WS lifecycle 由 RealRoomViewModel 唯一管理（subscribeRoomIdConnect 订阅 currentRoomId 变化驱动 stream restart / disconnect）
- 让 UseCase 调 connect 会形成两个 WS lifecycle owner（UseCase + RealRoomViewModel.subscribeRoomIdConnect）→ A→B 切换 / leave-rejoin / 重连场景下两边各自决策易冲突
- RealRoomViewModel 已经有 prepareForReconnect / cancelTask / generation-gated 等防御机制（12.5 落地），不应让 UseCase 旁路这些机制

实现路径：UseCase 写 setCurrentRoomId → RealRoomViewModel.subscribeRoomIdConnect 的 `nil → A` 分支被触发 → 该分支内的现有 prepareForReconnect / wsState=.connected / startConsumingMessages 路径**追加** `Task { try? await client.connect(roomId:) }` 一行 → connect 失败由 12.5 reconnect 状态机自动接管.

### leave-idempotent 决策（6004 视同成功）

V1 §10.5 钦定 6004 触发条件含三种 race 场景：
1. `users.current_room_id == NULL`（用户当前不在任何房间）
2. `users.current_room_id != path roomId`（用户在其他房间）
3. 同一用户并发两次 leave 同一房间，赢家事务内已删除 room_members 行（同一用户两端 / 两次点击）

三种场景 client UX 处理一致："我已经不在那个房间里了" → **写 `setCurrentRoomId(nil)` + 不弹 alert**。理由：

- 用户主动点 leave 已表达"我要离开"意图
- 6004 反馈 "你不在房间里" 让用户困惑（"我刚不还在吗？"）
- 即便底层有 race，最终一致性是用户期望的（不在房间）
- alert 影响 UX，retry 也无意义（重 leave 还会 6004）

继承自 V1 §10.5 r10 锁定 + lesson `2026-05-08-ws-disconnect-only-clears-ephemeral-not-membership-11-1-r3.md`：HTTP 200 / 6004 都是"已离开"语义，client 不应做 UX 区分.

### HTTP 200 vs WS close 4007 — leave 完成的权威信号

**HTTP 200 是 leave 完成的唯一 authoritative signal**（V1 §10.5 r10 锁定）：

- HTTP 200 + `data.left: true` 抵达 LeaveRoomUseCase → 立即 `setCurrentRoomId(nil)` → subscribeRoomIdConnect 的 `A → nil` 分支走 disconnect + 清 roster + wsState=.disconnected
- **不**等待 WS close 4007（best-effort cleanup signal；server 端 fire-and-forget；client 可能完全收不到 / 比 HTTP 200 晚到 / leaver WS 早断）
- 等 4007 才推进 RoomView 退出 = 卡死风险（4007 在合法场景下可能不到达）

12.5 reconnect 状态机已对 4007 做 terminal close 分类（不重连）；本 story UseCase 层不 / 不应感知 4007.

### sessionGeneration 守护检查（落地时复盘）

本 story 改 RealRoomViewModel.subscribeRoomIdConnect 的 connect 触发，**不**改 12.5 sessionGeneration 状态机。但要复盘：

- subscribeRoomIdConnect 的 `nil → A` / `A → B` / `A → nil` 分支内的所有动作都跑在 main actor sink 闭包里 → 同步执行 → 没有 stale task 问题
- 追加的 `Task { try? await client.connect(...) }` 是 fire-and-forget；连接失败 / 期间 currentRoomId 又被改 → 12.5 内部 connectGate generation-gated + supersede 旧 gate 自动接管（lesson `2026-05-09-ws-reconnect-precondition-must-precede-gen-bump-and-gate-supersede-12-5-r5.md`）
- 不需要本 story 在 sink 层再加 generation 守护（12.5 已是单一真相源）

继承自 lesson `2026-05-09-ws-reconnect-generation-counter-isolates-stale-tasks-12-5-r2.md` + `2026-05-09-ws-reconnect-stream-vs-session-generations-must-decouple-12-5-r4.md`.

### bind disconnect-old-client 守护（继承 12.1 r5 lesson）

RealRoomViewModel.bind 已有 swap 分支（`oldClient != nil && newClient != oldClient`）走 disconnect 旧 + cancel 旧 task + swap + prepareForReconnect 新。本 story 在该分支末尾追加 connect 触发时**必须 follow 既有 disconnect 旧顺序**：

```
oldClient.disconnect()       // 先 disconnect 旧 client（继承 12.1 r5 / 12.6 r1 / r6）
self.messageConsumerTask?.cancel()
self.webSocketClient = newClient
newClient.prepareForReconnect()
// 既有：startConsumingMessages()
// 本 story 追加：Task { try? await newClient.connect(roomId: lastObservedRoomId) }
```

继承自 lesson `2026-05-09-bind-replace-must-disconnect-old-client-12-1-r5.md` + `2026-05-09-same-instance-rebind-must-true-noop-12-1-r6.md`.

### codec 必须校验 required 字段（继承 12.4 r2 lesson）

新建 `RoomEndpointDTO.swift` 内的 Codable struct **必须**让所有 required 字段为非 Optional 类型 → JSONDecoder 自动 fail 不合法 server response → APIError.decoding 透传。**不要**用 Optional 装非空字段（Story 12.4 r2 lesson 明确 codec 必须 fail-fast，避免 silent coerce 给后续 UI 埋坑）.

继承自 lesson `2026-05-09-ws-codec-must-validate-required-fields-12-4-r2.md`.

### 关键 lessons 守护检查（落地时 grep 校验）

`docs/lessons/` 内本 story 必读 lessons（按优先级）：
- `2026-05-08-ws-disconnect-only-clears-ephemeral-not-membership-11-1-r3.md`：HTTP leave / WS close 责任划分 —— 本 story LeaveRoomUseCase 必须 HTTP 200 即推进，禁止等 4007
- `2026-05-09-bind-replace-must-disconnect-old-client-12-1-r5.md`：bind 替换必须 disconnect 旧 client + cancel 旧 task —— 本 story bind 扩展不能破坏既有顺序
- `2026-05-09-same-instance-rebind-must-true-noop-12-1-r6.md`：同 instance rebind 必须 true no-op —— 本 story 多次 bind 注入 UseCase 不能 cancel 当前 consumer
- `2026-05-09-stale-snapshot-discard-by-room-id-12-1-r3.md`：snapshot payload 必须 room.id 校验 —— 本 story 不改 applySnapshot 主干
- `2026-05-09-cross-room-race-needs-stream-roomid-capture-12-4-r1.md`：member.joined / member.left streamRoomId 守护 —— 本 story 不改 startConsumingMessages 主干
- `2026-05-09-ws-codec-must-validate-required-fields-12-4-r2.md`：codec required 字段 fail-fast —— 本 story 新建 RoomEndpointDTO 必须遵循
- `2026-05-09-ws-reconnect-precondition-must-precede-gen-bump-and-gate-supersede-12-5-r5.md`：connect 入口的 precondition 必须在 gen 翻新前 —— 本 story 不改 12.5 主干
- `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`：override 必须本地 mutate state，不能只 log —— 本 story onCreateTap / onJoinRoomConfirm / onJoinFriendTap / onLeaveTap override 必须有立即 UI 反馈或 errorPresenter 调用
- `2026-04-30-coordinator-must-mirror-loaded-home-room-state.md`：coordinator state mirror —— 本 story 接 setCurrentRoomId 路径已经走 AppState 单 source of truth，不会触发该 lesson 反模式

### Library / Framework 版本

- Swift 5.9+（Xcode 26 / iOS 17+）
- 仅 Foundation + Combine（无第三方 HTTP / WS 库）
- 测试：XCTest 原生（无 Quick / Nimble / Mockingbird，按 ADR-0001 §3.5）
- ADR-0002 §3.1 单元测试必须秒级完成（无真实 sleep；UseCase 测试用 mock repo + Mock AppState；不跑真实 HTTP / WS）

### Testing standards summary

- 全部 UseCase / Repository / ViewModel override 单测必须**秒级完成**（mock everything；不跑真实 server / WS）
- Repository 测试用 `MockAPIClient`（继承 既有 `iphone/PetAppTests/.../*MockAPIClient*`）
- UseCase 测试用 `MockRoomRepository` + `AppState` 真实实例（@MainActor + final class，不可继承 → 用 testing helper 构造已 hydrate / 已 reset 实例，与 SessionStore 同模式）
- ViewModel override 测试用 `MockCreateRoomUseCase` / `MockJoinRoomUseCase` / `MockLeaveRoomUseCase`（在测试 target 内 inline 定义；不入产品代码 target）
- UI 测试只覆盖 1 个 happy path（详细多端联调由 Epic 13 节点 4 demo 验收 e2e）

### 范围红线（不在本 story 范围）

- 不改 server（Epic 11 已 done；任何 server 改动单开 epic）
- 不改 WebSocketClientImpl 主干（12.2-12.6 已稳定；本 story 仅 caller 层调 connect / disconnect）
- 不改 RealRoomViewModel.applySnapshot / handle / subscribeRoomCode 主干（12.1 / 12.3 / 12.4 / 12.5 / 12.6 已稳定）
- 不改 HomeViewModel / RoomViewModel / FriendsViewModel 基类签名（abstract method 已锁；本 story 仅在 Real 子类 override）
- 不改 HomeView / RoomScaffoldView / FriendsScaffoldView / JoinRoomModal 主干（37.7 / 37.8 / 37.10 / 37.12 已稳定；本 story **零 View 层改动**）
- 不改 SessionStore / AppLaunchStateMachine（节点 1 / 2 已稳定）
- 不改 AppState 字段 / 入口（Story 37.4 已锁；本 story 仅调用 setCurrentRoomId / read currentRoomId）
- 不实装 ResetRoomViewModel / Reconnect overlay UI / 错误重试按钮（节点 4 demo 验收范围 / 后续 epic）
- 不实装 e2e / 跨端联调（Epic 13 范围）
- 不实装 leaveRoom retry / circuit breaker / offline queue（产品规则未定义；保持现有 fire-and-forget 即可）
- 不动 ios/（旧产物归档）

### Open Questions（落地时复盘）

1. **subscribeRoomIdConnect 内 connect 调用是否需要在 sink 闭包外起 Task？**
   - 决策：起 Task（fire-and-forget）。理由：sink 闭包是同步的；async connect 不能直接 await；起 Task 解耦 sink 与 connect 失败
   - 失败由 12.5 reconnect 状态机自动接管（连接失败抛 WSError → close code 1006/1011 transient → schedule reconnect）

2. **CreateRoomUseCase / JoinRoomUseCase 写 setCurrentRoomId 是否需要 await MainActor.run？**
   - 决策：是。AppState 是 @MainActor + ObservableObject + @Published；UseCase 跑在 detached actor 调度（async function），需要显式 hop 到 MainActor 才能写 @Published 字段
   - 实装：`await MainActor.run { appState.setCurrentRoomId(roomId) }`

3. **6004 leave-idempotent 兜底是否暴露 dev 信号？**
   - 决策：是。在 catch 6004 时调 `os_log(.info, "LeaveRoom received 6004 (already left); treating as success")` 让 dev 在 production 仍可从 console 看到 race 发生频率
   - 不弹 UI（影响 UX）

4. **JoinRoomUseCase 校验 `data.roomId == request.roomId` 不一致是否真有可能？**
   - 决策：极小概率（server bug / proxy 改写 path），但加防御层成本极低（一行 == 比较）
   - 不一致抛 `.decoding(...)` → ErrorPresenter 默认 mapper 走 `.alert`（让 dev 看到 server bug 信号）

5. **Friend.currentRoomId 字段是否已就位？**
   - 决策：复用 Story 37.10 落地的 `Friend.currentRoomId: String?`；本 story 仅消费不新增字段
   - 若发现 Friend type 没该字段（dev 落地时校验）→ 在 `iphone/PetApp/Features/Friends/Models/Friend.swift` 加字段（仅 model 层 + mock seed 兼容；不算超 scope）

6. **是否需要 dev tools "强制创建房间" / "强制加入房间 X" 按钮？**
   - 决策：不需要。HomeView Scaffold 既有 TeamIdleCard 已暴露入口；ResetIdentityButton 让 dev 重置后再走完整 flow 即可
   - 后续若 demo / 测试需要可单独立 dev-tools mini-epic

### References

- [V1 接口设计 §10.1 创建房间](docs/宠物互动App_V1接口设计.md#10.1)（行 1056-1148）
- [V1 接口设计 §10.4 加入房间](docs/宠物互动App_V1接口设计.md#10.4)（行 1409-1503）
- [V1 接口设计 §10.5 退出房间](docs/宠物互动App_V1接口设计.md#10.5)（行 1506-1610）
- [V1 接口设计 §12.1 WS 连接地址 + close code 表](docs/宠物互动App_V1接口设计.md#12.1)（行 1648-1757）
- [V1 接口设计 §12.3 client merge contract](docs/宠物互动App_V1接口设计.md#12.3)
- [iOS 工程结构与模块职责设计 §6.x UseCase / Repository / Endpoints 分层](docs/宠物互动App_iOS客户端工程结构与模块职责设计.md)
- [总体架构 §产品规则](docs/宠物互动App_总体架构设计.md)
- [时序图与核心业务流程设计 §11 / §12 / §13](docs/宠物互动App_时序图与核心业务流程设计.md)
- [ADR-0009 iPhone 导航架构 (TabView + 互斥状态机)](_bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md)
- [ADR-0010 iPhone AppState 单 source of truth](_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md)
- [ADR-0011 WS Stack](_bmad-output/implementation-artifacts/decisions/0011-ws-stack.md)
- [Sprint Change Proposal v2 §5.2 Story 12.7 改写](_bmad-output/planning-artifacts/sprint-change-proposal-2026-04-29-v2.md)（行 345-376）
- [Story 11.10 GET /home 扩展 - room.currentRoomId 真实数据（Epic 11 收官）](_bmad-output/implementation-artifacts/11-10-get-home-扩展-room-currentroomid-真实数据.md)
- [Story 12.6 心跳维护](_bmad-output/implementation-artifacts/12-6-心跳维护.md)
- [Story 12.5 自动重连](_bmad-output/implementation-artifacts/12-5-自动重连.md)
- [Story 12.1 房间页面 SwiftUI 骨架（基础接缝 + applySnapshot 端到端）](_bmad-output/implementation-artifacts/12-1-房间页面-swiftui-骨架.md)
- [Story 37.4 AppState 实装 + LoadHome 迁移](_bmad-output/implementation-artifacts/37-4-appstate-实装-loadhome-迁移.md)
- [Story 37.7 HomeView Scaffold（HomeViewModel class 层次 + abstract method）](_bmad-output/implementation-artifacts/37-7-homeview-scaffold.md)
- [Story 37.8 RoomView Scaffold](_bmad-output/implementation-artifacts/37-8-roomview-scaffold.md)
- [Story 37.10 FriendsView Scaffold](_bmad-output/implementation-artifacts/37-10-friendsview-scaffold.md)
- [Story 37.12 JoinRoomModal + 跨屏跳转](_bmad-output/implementation-artifacts/37-12-joinroommodal-跨屏跳转.md)
- [Lesson 2026-05-08 WS disconnect 只清 ephemeral 不清 membership](docs/lessons/2026-05-08-ws-disconnect-only-clears-ephemeral-not-membership-11-1-r3.md)
- [Lesson 2026-05-09 bind 替换必须 disconnect 旧 client](docs/lessons/2026-05-09-bind-replace-must-disconnect-old-client-12-1-r5.md)
- [Lesson 2026-05-09 same instance rebind must true no-op](docs/lessons/2026-05-09-same-instance-rebind-must-true-noop-12-1-r6.md)
- [Lesson 2026-05-09 stale snapshot discard by room.id](docs/lessons/2026-05-09-stale-snapshot-discard-by-room-id-12-1-r3.md)
- [Lesson 2026-05-09 cross-room race needs streamRoomId capture](docs/lessons/2026-05-09-cross-room-race-needs-stream-roomid-capture-12-4-r1.md)
- [Lesson 2026-05-09 codec must validate required fields](docs/lessons/2026-05-09-ws-codec-must-validate-required-fields-12-4-r2.md)
- [Lesson 2026-04-30 real viewmodel override placeholder must mutate state](docs/lessons/2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md)

## Dev Agent Record

### Agent Model Used

claude-opus-4-7 (1M context)

### Debug Log References

- 2026-05-10 dev-story 走完红绿循环 + 实装 + 测试 + 模拟器手动验证；519 单元测试全绿；模拟器三按钮（创建队伍 / 加入队伍 / 离开房间）UI 触发链路验证完成（无 server 路径下走 AuthBoundary 401 cold-start → RetryView，符合预期 wire 行为）。

### Completion Notes List

- **Task 1**：RoomEndpoints / RoomEndpointDTO（CreateRoomResponse / CreateRoomRoomDTO / JoinRoomResponse / LeaveRoomResponse）/ RoomRepository（protocol + Default struct）+ RoomRepositoryTests（6 case：3 happy + 3 错误透传）落地。Endpoint body 用 `AnyEncodable(EmptyObjectBody())`（Encodable 序列为 `{}`），与既有 Endpoint struct AnyEncodable 模式对齐。
- **Task 2**：CreateRoomUseCase（happy / 6003 透传 / network 透传 / 1009 透传 4 case 全绿；spec Open Question §2 决议：先 setCurrentRoomId，后 return）。
- **Task 3**：JoinRoomUseCase + JoinRoomMismatchError 防御层（spec Open Question §4 决议：response.roomId != request.roomId → throw .decoding 包装；happy / 6002 / 6001 / mismatch / 6003 5 case 全绿）。
- **Task 4**：LeaveRoomUseCase（HTTP 200 = authoritative leave 信号；6004 视同成功 leave-idempotent + os_log.info dev signal；happy / nil 早 return / 6004 / 1009 / network 5 case 全绿）。
- **Task 5**：RealRoomViewModel `bind(appState:webSocketClient:leaveRoomUseCase:errorPresenter:)` 签名扩展（默认参数让既有 caller 不破）；新增 `onLeaveTap` override（fallback 路径 useCase==nil 保留旧 mock 行为）；`subscribeRoomIdConnect` 的 `nil → A` / `A → B` 分支追加 connect Task；`bind` 内 first-injection / swap 路径 `if clientChanged && lastObservedRoomId != nil` 块也追加 connect Task；空字符串 roomId 守护让本地占位 "" 不打 server。RealRoomViewModelTests 新增 5 case（onLeaveTap 调 UseCase / wsState=.disconnected / 1009 透传 / nil→A 触发 connect / A→B disconnect+connect）。
- **Task 6**：RealHomeViewModel 新增 `bind(createRoomUseCase:joinRoomUseCase:errorPresenter:)` 入口；`onCreateTap` 改写起 Task 调 UseCase + catch 6003 alert "你已经在房间里了"；`onJoinRoomConfirm` 改写"先 showJoinModal=false 后 起 Task"（顺序锁定 lesson）+ catch 6001/6002/6003/6005/1002 case-by-case 文案；删除老占位行为里 `localAppState?.setCurrentRoomId(roomId)` 双写问题。RealHomeViewModelTests 新建 6 case。
- **Task 7**：RealFriendsViewModel `bind(appState:joinRoomUseCase:errorPresenter:)` 扩展；`onJoinFriendTap` 改写：currentRoomId nil 防御性 toast / 起 Task 调 JoinRoomUseCase / 错误映射与 RealHomeViewModel 一致。RealFriendsViewModelTests 新建 3 case。
- **Task 8**：AppContainer 新增 4 个 factory（makeRoomRepository / makeCreateRoomUseCase / makeJoinRoomUseCase / makeLeaveRoomUseCase）；RootView .onAppear 关键改动：realRoomVM.bind 的 webSocketClient 从 nil 升级为 `container.webSocketClient`（节点 4 真实 client 接通），同时注入 LeaveRoomUseCase + ErrorPresenter；realHomeVM 注入 CreateRoom + JoinRoom UseCase + ErrorPresenter；realFriendsVM 注入 JoinRoom UseCase + ErrorPresenter。
- **Task 9**：UITest 新增 `testHomeTabIdleCreateButtonExistsAndTappable`（launch → Home idle → 锚定 create + join 按钮 + tap create 不 crash + UI 稳定）；`bash iphone/scripts/build.sh --test` 519 测试全绿；模拟器手动 verify：UITEST_FORCE_IN_ROOM 不带路径 → Home idle 渲染（greeting "想你啦 ♥" / catStage / 创建队伍 / 加入队伍 按钮）→ tap "创建队伍" → 触发 CreateRoomUseCase → AuthBoundary 401 cold-start → RetryView（wire 链路验证通过）→ 重 launch → Home idle → tap "加入队伍" → JoinRoomModal 弹出（输入框 + 取消 + 确定加入 按钮）→ X 关闭 modal 回 Home idle → 重 launch 带 UITEST_FORCE_IN_ROOM=1 → Room view 渲染 → tap "离开房间" → 触发 LeaveRoomUseCase → AuthBoundary 401 cold-start → RetryView。
- **Task 10**：sprint-status.yaml `12-7-创建-加入-退出-use-case-主界面入口完善` 切 `review`；last_updated → 2026-05-10。

### File List

**新建（10 文件）**：

- `iphone/PetApp/Features/Room/UseCases/RoomEndpoints.swift`
- `iphone/PetApp/Features/Room/UseCases/CreateRoomUseCase.swift`
- `iphone/PetApp/Features/Room/UseCases/JoinRoomUseCase.swift`
- `iphone/PetApp/Features/Room/UseCases/LeaveRoomUseCase.swift`
- `iphone/PetApp/Features/Room/Repositories/RoomRepository.swift`
- `iphone/PetApp/Features/Room/Models/RoomEndpointDTO.swift`
- `iphone/PetAppTests/Features/Room/Repositories/RoomRepositoryTests.swift`
- `iphone/PetAppTests/Features/Room/UseCases/MockRoomRepository.swift`
- `iphone/PetAppTests/Features/Room/UseCases/CreateRoomUseCaseTests.swift`
- `iphone/PetAppTests/Features/Room/UseCases/JoinRoomUseCaseTests.swift`
- `iphone/PetAppTests/Features/Room/UseCases/LeaveRoomUseCaseTests.swift`
- `iphone/PetAppTests/Features/Home/ViewModels/RealHomeViewModelTests.swift`
- `iphone/PetAppTests/Features/Friends/RealFriendsViewModelTests.swift`

**修改**：

- `iphone/PetApp/App/AppContainer.swift`（新增 4 factory）
- `iphone/PetApp/App/RootView.swift`（.onAppear 升级 realHomeVM / realRoomVM / realFriendsVM bind 注入 UseCase）
- `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift`（新增 bind + 字段 + 改写 onCreateTap / onJoinRoomConfirm）
- `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`（扩展 bind + 字段 + 改写 onLeaveTap + 追加 connect 触发）
- `iphone/PetApp/Features/Friends/ViewModels/RealFriendsViewModel.swift`（扩展 bind + 字段 + 改写 onJoinFriendTap）
- `iphone/PetAppTests/Features/Room/RealRoomViewModelTests.swift`（新增 5 case + inline MockLeaveRoomUseCaseRoom）
- `iphone/PetAppUITests/RoomUITests.swift`（新增 testHomeTabIdleCreateButtonExistsAndTappable）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（12-7 状态切 review）
- `_bmad-output/implementation-artifacts/12-7-创建-加入-退出-use-case-主界面入口完善.md`（Status review + Tasks 全 [x] + Dev Agent Record + File List + 变更日志）

## 变更日志

| 日期 | 类型 | 说明 |
|---|---|---|
| 2026-05-10 | 创建 | Story 12.7 创建/加入/退出 UseCase + 主界面入口完善 context 落地：契约 1-6（V1 §10.1 createRoom / §10.4 joinRoom / §10.5 leaveRoom + HTTP 200 authoritative leave / §12.1 WS roomId 来源 / §12.3 client merge contract / iOS UseCase + Repository + Endpoints 分层）+ AC1-10 + Task 1-10 + 6 个开放问题；红线汇总：不改 server / 不改 WebSocketClientImpl 主干 / 不改 RealRoomViewModel.applySnapshot / handle / 主干 / 不改基类 abstract method 签名 / 不改 HomeView / RoomScaffoldView / JoinRoomModal 主干（零 View 层改动）/ 不改 AppState 字段 / 不动 ios（旧）。状态: ready-for-dev。 |
| 2026-05-10 | 实装 | Tasks 1-10 全部完成。3 个 UseCase（CreateRoom / JoinRoom / LeaveRoom）+ RoomRepository + RoomEndpoints + RoomEndpointDTO + JoinRoomMismatchError 落地；RealHomeViewModel.onCreateTap / onJoinRoomConfirm + RealRoomViewModel.onLeaveTap + RealFriendsViewModel.onJoinFriendTap 全部从占位升级为 UseCase 调用路径；RealRoomViewModel.subscribeRoomIdConnect 的 `nil → A` / `A → B` 分支 + bind() first-injection / swap 路径追加 `Task { try? await client.connect(roomId:) }` 让节点 4 真实 WS 拨号在 setCurrentRoomId 后自动触发；AppContainer 新增 4 factory + RootView wire realRoomVM.bind 把 webSocketClient 从 nil 升级为 container.webSocketClient（节点 4 真实 client 接通）。519 unit test + 1 新 UITest case 全绿；模拟器 ios-simulator MCP 三按钮（创建队伍 / 加入队伍 / 离开房间）触发链路 manual verify 通过（无 server 路径下走 AuthBoundary 401 cold-start → RetryView 是预期 wire 行为）。状态: in-progress → review。 |
