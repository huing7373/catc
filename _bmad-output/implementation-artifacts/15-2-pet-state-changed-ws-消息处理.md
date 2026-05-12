# Story 15.2: pet.state.changed WS 消息处理（iOS 端 — RoomViewModel 收 `pet.state.changed` envelope → 按 §12.3 client merge contract 字段级 merge `memberPetStates[userId]`；含发起者自己也同路径处理；不存在的 userId 安全忽略 + log warn；不破坏其他成员字段；payload schema 校验失败 fallback `.unknown`；单测 ≥4 case，UITest 已由 15.1 / 15.3 覆盖动效与 a11y 验证，本 story 不新增 UITest）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want 当房间里某个成员（含我自己）的猫状态变化时我能立即看到，不必等下次 snapshot 刷新,
So that 互动有实时反馈 + 自己 state-sync 后 self-broadcast 走 §12.3 client merge contract 与 §5.2 self-broadcast 兜底规则对称 no-op 兜底路径正确生效.

## 故事定位（Epic 15 第 2 条 story；节点 5 iOS 端"实时收 `pet.state.changed` WS 消息 → 字段级 merge `memberPetStates`"收口；上承 15.1 渲染基础 + 14.4 server 广播；下启 15.3 动效过渡 + 15.4 self state-sync + 15.5 跨房间状态恢复）

- **Epic 15 进度**：15.1（房间页内多成员猫位渲染 + snapshot pet.currentState 解析，**done**）→ **15.2（本 story，pet.state.changed WS 消息处理）** → 15.3（状态切换动画过渡）→ 15.4（自己状态变化时上报 state-sync + 节流 + 房间内才上报）→ 15.5（跨房间状态恢复，重连后 snapshot 对齐）
- **本 story 是 Epic 15 第二条 story**：将 Story 14.4 server 端真实广播的 `pet.state.changed` WS 消息接入 iOS 端 `RealRoomViewModel` 消息处理路径，让"房间内任一成员状态变化（含发起者自己）"能跳过 snapshot 全量刷新走"字段级 merge `memberPetStates[userId]`"的实时路径
- **本 story 是 Epic 15 后续 stories 强前置**：
  - **Story 15.3（状态切换动画过渡）**：依赖本 story 落地的 `memberPetStates[userId]` 实时写入路径（任何 `.@Published memberPetStates` mutation 都触发 PetSpriteView `.id(state) + .transition + .animation` 平滑过渡；本 story 不动 PetSpriteView 内部，仅驱动数据层）
  - **Story 15.4（自己状态变化时上报 state-sync）**：依赖本 story 落地的"自己收到自己的 `pet.state.changed` → 也字段级 merge 更新本地 `memberPetStates[selfUserId]`"路径（§5.2 self-broadcast 对称兜底规则的 client 端实装基础）
  - **Story 15.5（跨房间状态恢复 / 重连后 snapshot 对齐）**：依赖本 story 落地的"WS 实时消息字段级 merge"语义（重连成功后 snapshot 全量覆盖 +  断线期间收到的旧 `pet.state.changed` 已被新 snapshot 覆盖；本 story 不动 snapshot 路径，仅扩 `pet.state.changed` 路径）
- **节点 5 验收要求（§4.5）**：本 story 落地后，房间内至少两名用户在线时，A 切到 walk → B 实时看到 A 的猫切到 walk（延迟 < 1 秒，符合 §4.5 "状态变化延迟可接受"验收项）

## Acceptance Criteria

> **AC 编号体系**：
> - AC1 = WSMessage enum 扩 `.petStateChanged(PetStateChangedPayload)` case + payload value type
> - AC2 = WSMessageCodec 路由 `case "pet.state.changed"` → 解码 + payload schema 校验 + .unknown fallback
> - AC3 = RealRoomViewModel `handle(message:streamRoomId:)` switch 增 `.petStateChanged` case → 调 `applyPetStateChanged(_:)` + streamRoomId 守护防 cross-room race
> - AC4 = `applyPetStateChanged(_:)` 字段级 merge：found userId in members → 更新 `memberPetStates[userId]`；not found → 忽略 + log warn；未知 currentState 值 → fallback `.rest` + log error；自己的 userId 走同一路径
> - AC5 = 单元测试 ≥4 case（mocked WebSocketClient，覆盖 epics.md §15.2 行 2402-2406 钦定的 4 个 case）+ ≥1 case codec 层 envelope/payload 解码（含语义校验 + .unknown fallback）
> - AC6 = Build verify + ios-simulator MCP 实跑验证（CLAUDE.md "iOS UI 验证（必跑）"）
> - AC7 = Deliverable 清单

---

### AC1 — `WSMessage` enum 新增 `.petStateChanged(PetStateChangedPayload)` case + `PetStateChangedPayload` value type 就位

**给定**：

- `WebSocketClient.swift` 已就位的 `WSMessage` enum 覆盖：`.roomSnapshot` / `.pong` / `.error` / `.memberJoined` / `.memberLeft` / `.connectionStateChanged` / `.unknown`（Story 12.1 / 12.4 / 12.5）；line 73 doc comment `pet.state.changed 由 Epic 14 / Story 14.x 扩展` 注释已预留
- Story 14.4 server 端 envelope 已实装 schema：`{type: "pet.state.changed", requestId: "", payload: {userId, petId, currentState}, ts}`（V1 §12.3 line 2223-2230）
- `WebSocketClient.swift` 既有 `RoomSnapshotPet` / `MemberJoinedPet` 模式：两者都是独立 struct + Equatable + Sendable（line 170-178 / 208-216），未走 typealias —— 保留各消息独立类型演进空间

**预期行为**：

1. **`WSMessage` enum 新增 case**（紧接 line 99 `.memberLeft` 之后、line 100 `.connectionStateChanged` 之前；与 server 推送类业务消息分组对齐）：

   ```swift
   /// `pet.state.changed` —— 房间内任一成员（含发起者自己）通过 `POST /pets/current/state-sync` 触发状态变更（V1 §12.3 行 2212-2259）.
   /// payload 三字段（userId + petId + currentState）；client 收到后按 §12.3 client merge contract 字段级 merge：
   /// (a) `memberPetStates[userId]` 已存在 → 覆盖该字段为 `HomePetState(rawValue: currentState)` 映射后的状态
   /// (b) `memberPetStates[userId]` 不存在（理论不该发生 —— 表示 roster 与 server 状态严重不一致）→ ignore + log warn
   /// (c) `payload.userId == self` 自己的 self-broadcast 走同一路径（§5.2 self-broadcast 对称兜底前置条件，由 Story 15.4 落地自身侧 UI 驱动）
   /// Story 15.2 落地；trigger 唯一来源 = HTTP `POST /api/v1/pets/current/state-sync` 成功 UPDATE 之后 service 层广播.
   case petStateChanged(PetStateChangedPayload)
   ```

2. **`PetStateChangedPayload` value type 紧接 `MemberLeftPayload` 之后新增**（line 228 之后；与 `MemberJoinedPet` / `RoomSnapshotPet` 同模式）：

   ```swift
   /// `pet.state.changed` payload（V1 §12.3 行 2223-2230 字段表）.
   /// 三字段 userId + petId + currentState 全部必填（V1 §12.3 line 2250 "禁止 payload 为 {} 或缺任一字段"）；
   /// `userId` / `petId` 是 BIGINT 字符串化（§2.5）；`currentState` 是 Int 枚举 1/2/3.
   /// **本 struct 独立于 `RoomSnapshotPet` / `MemberJoinedPet`** —— 三者虽都映射相同业务字段，但
   /// 每条业务 WS 消息保留各自 payload struct 演进空间（与 Story 12.4 `MemberJoinedPet` 独立模式一致）.
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
   ```

3. **doc comment 同步更新**：
   - `WSMessage` enum 头部 doc comment（line 69-74）：把 line 73 `pet.state.changed 由 Epic 14 / Story 14.x 扩展` 改为 `pet.state.changed 由 Story 15.2 扩展（已落地）`，与 line 89 `member.joined / member.left 由 Story 12.4 扩展` 表达式对齐

**红线**：

- **不**改既有 `RoomSnapshotPet` / `MemberJoinedPet` struct 定义（独立类型演进空间，跨消息不复用）
- **不**改 `RoomSnapshotPayload` / `RoomSnapshotMember` / `MemberJoinedPayload` / `MemberLeftPayload` struct 定义
- **不**在 `WSMessage` 引入 `Equatable.==` 自定义实现（保持默认综合实现，与既有 case 一致）
- **不**让 `PetStateChangedPayload` 引用 `HomePetState`（payload struct 留 wire 层 Int 值；HomePetState 映射在 ViewModel apply 层做，与 `RoomSnapshotPet.currentState: Int` 同精神）

**对应 Tasks**: Task 1.1, 1.2

---

### AC2 — `WSMessageCodec` decode 路由 `case "pet.state.changed"` → 解码 + payload schema 语义校验 + .unknown fallback

**给定**：

- `WSMessageCodec.swift` 既有 `decode(_:)` switch 路由覆盖：`room.snapshot` / `pong` / `error` / `member.joined` / `member.left` / default → `.unknown`（Story 12.2 / 12.4）
- `member.joined` / `member.left` 路由（line 60-95）模式：(i) `try makeDecoder().decode(<Envelope>.self, from: data).payload` 拿 DTO；(ii) **语义层校验**（如 `dto.userId.isEmpty` → log error + fallback `.unknown(rawType: <originalType>)`）—— 因 Decodable 仅挡 absent / type-mismatch，**不能**挡 server 推 `userId == ""` 这类语义无效 payload（Story 12.4 fix-review r2 P2 lesson）
- V1 §12.3 line 2227-2229 钦定 `payload.userId` / `payload.petId` / `payload.currentState` 三字段**全部必填**（line 2250 `禁止 payload 为 {} 或缺任一字段`）；`userId` / `petId` 是 BIGINT 字符串化下发（§2.5）
- V1 §12.3 line 2251 钦定 client 解析层对 malformed 消息**应**走 `安全忽略 + log warn` 路径 + Story 10.1 钦定 `不破坏 stream`

**预期行为**：

1. **`WSMessageCodec.decode(_:)` switch 新增 case**（紧接 `case "member.left"` 之后、`default` 之前）：

   ```swift
   case "pet.state.changed":
       // Story 15.2：pet.state.changed 路由（V1 §12.3 行 2223-2230 字段表）.
       // 同 member.joined / member.left 精神：Decodable 只能挡 absent / type-mismatch；
       // server 若推送语义无效 payload（如 `userId == ""` / `petId == ""` / `currentState` 不在 {1,2,3}）
       // 仍会成功解码 —— 但 V1 §12.3 line 2250 钦定三字段必填且非空，line 2229 钦定 currentState 仅 1/2/3.
       // codec 必须把这类语义无效 payload fallback 为 .unknown(rawType: "pet.state.changed")
       // 走 Story 10.1 钦定 "安全忽略未识别 type" + log warn 路径，避免 ViewModel apply 路径用空字段污染.
       do {
           let dto = try makeDecoder().decode(PetStateChangedEnvelope.self, from: data).payload
           guard !dto.userId.isEmpty else {
               os_log(.error, log: logger, "pet.state.changed rejected: empty userId")
               return .unknown(rawType: "pet.state.changed")
           }
           guard !dto.petId.isEmpty else {
               os_log(.error, log: logger, "pet.state.changed rejected: empty petId")
               return .unknown(rawType: "pet.state.changed")
           }
           // currentState 值域校验：codec 不强校验 1/2/3 —— 容忍 server 未来扩展新状态值（如 sleep=4），
           // ViewModel.applyPetStateChanged 层做 HomePetState(rawValue:) 映射 + 未知值 fallback `.rest` + log warn
           // （与 applySnapshot 同语义；Story 15.1 AC1 已落地 fallback 模式）.
           // codec 层仅挡"缺字段 / 空字符串"语义违反，不挡"未知枚举值"语义降级.
           return .petStateChanged(dto.toDomain())
       } catch {
           os_log(.error, log: logger, "pet.state.changed payload decode failed: %{public}@", String(describing: error))
           return .unknown(rawType: "pet.state.changed")
       }
   ```

2. **`PetStateChangedEnvelope` private DTO 新增**（紧接 `MemberLeftEnvelope` 之后；与既有 `MemberJoinedEnvelope` 同模式）：

   ```swift
   /// pet.state.changed 整体信封 —— 与 V1 §12.3 行 2223-2230 字段表 1:1 对齐.
   private struct PetStateChangedEnvelope: Decodable {
       let payload: PetStateChangedPayloadDTO

       struct PetStateChangedPayloadDTO: Decodable {
           let userId: String
           let petId: String
           let currentState: Int

           func toDomain() -> PetStateChangedPayload {
               PetStateChangedPayload(userId: userId, petId: petId, currentState: currentState)
           }
       }
   }
   ```

3. **encode 路径不动**：本 story 是 server → client 单向消息，无 outgoing 路径；`WSMessageCodec.encode(_:)` 不扩展（client → server 仅 `ping` —— Story 12.6 落地的 `WSOutgoingMessage.ping`）

**红线**：

- **不**改既有 `room.snapshot` / `member.joined` / `member.left` / `pong` / `error` 路由实装 / 既有 envelope DTOs
- **不**改 `WSEnvelope` 顶层信封 struct（type / requestId 已就位）
- **不**让 codec 层强校验 `currentState ∈ {1,2,3}` —— 容忍 server 未来扩展（与 Story 15.1 AC1 同精神：未知值降级 `.rest` 在 ViewModel apply 层做）
- **不**让 codec 层校验 `userId` / `petId` 是否为合法 BIGINT 字符串 —— BIGINT 字符串化是 wire 表示约定（§2.5），client 端不解析为数值，仅作字符串 key 用
- **不**让 codec 层路由"`payload.userId == self`"做 self vs 他人差异化 —— self 识别在 ViewModel apply 层做（V1 §12.3 line 2253 "client 应统一处理路径"）

**对应 Tasks**: Task 2.1, 2.2

---

### AC3 — `RealRoomViewModel.handle(message:streamRoomId:)` switch 增 `.petStateChanged` case + streamRoomId 守护防 cross-room race

**给定**：

- `RealRoomViewModel.handle(message:streamRoomId:)` 既有 switch 覆盖 7 个 case（line 590-672）；`.roomSnapshot` 走 payload.room.id 守护（line 596-602），`.memberJoined` / `.memberLeft` / `.connectionStateChanged` 走 streamRoomId 守护（line 605-651）
- Story 12.4 fix-review r2 P1 lesson 锁定：`member.joined` / `member.left` payload 不带 room.id（V1 §12.3 钦定 5 字段 / 1 字段），无法做 payload-level 校验 → 必须用 streamRoomId 守护防 cross-room race
- V1 §12.3 line 2223-2230 钦定 `pet.state.changed` payload 三字段（userId + petId + currentState）**不**含 room.id —— 与 `member.joined` / `member.left` 同语义：依靠 server 端 fanout 范围保证（只投递到该房间内 Sessions），client 端必须用 streamRoomId 守护防 A→B 切换 / leave-rejoin 时旧 stream late message 污染新房间 roster

**预期行为**：

1. **`handle(message:streamRoomId:)` switch 新增 case**（紧接 `case .memberLeft` 守护块之后、`case .connectionStateChanged` 之前；位置与 line 99 enum case 顺序对齐）：

   ```swift
   case .petStateChanged(let payload):
       // Story 15.2：pet.state.changed 守护与 member.joined / member.left 同精神 —— payload 不带 room.id（V1 §12.3 line 2223-2230
       // 钦定三字段 userId + petId + currentState），server 端按 fanout 范围保证只投递到该房间 sessions；
       // 但 client 在 A→B 切换 / leave-rejoin 时旧 consumer task 已 dequeue 的 late message 可能在 cancel 前
       // deliver 到 main actor，此时 lastObservedRoomId 已是 B 但 message 来自 room A 的 stream → streamRoomId
       // （启动时捕获 = A）与 lastObservedRoomId（= B）不匹配 → 丢弃 + log debug.
       guard streamRoomId != nil, streamRoomId == lastObservedRoomId else {
           os_log(.debug,
                  "RealRoomViewModel: discard stale pet.state.changed (userId=%{public}@, streamRoomId=%{public}@, current=%{public}@)",
                  payload.userId,
                  streamRoomId ?? "<nil>",
                  lastObservedRoomId ?? "<nil>")
           return
       }
       applyPetStateChanged(payload)
   ```

2. **handle doc comment 同步扩展**（line 587-589 既有可见性段说明）：在 line 558-589 既有 doc comment 后追加一段 `Story 15.2`：`pet.state.changed` 走 streamRoomId 守护防 cross-room race（与 `member.joined` / `member.left` 同模式）

**红线**：

- **不**在 `.petStateChanged` 路径做 `lastObservedRoomId == nil` 时的额外特殊处理（与 streamRoomId == nil 等价：streamRoomId 是启动时捕获 lastObservedRoomId，nil 表示启动时已离开房间）
- **不**用 payload 的任何字段做守护（payload 无 room.id —— 只能依赖 stream-lifecycle 层守护）
- **不**在守护层做 `payload.userId in members` 检查 —— 该检查归属 `applyPetStateChanged` 内部（AC4 钦定 found / not-found 分支）
- **不**复用 `.roomSnapshot` 的 payload-level 守护 —— `pet.state.changed` 协议层无 room.id，必须走 streamRoomId 守护

**对应 Tasks**: Task 3.1

---

### AC4 — `applyPetStateChanged(_:)` 字段级 merge：found userId in members → 更新 `memberPetStates[userId]`；not found → 忽略 + log warn；未知 currentState 值 → fallback `.rest` + log error；自己的 userId 走同一路径

**给定**：

- `RealRoomViewModel` 既有 `applySnapshot(_:)` (line 690-751) / `applyMemberJoined(_:)` (line 776-813) / `applyMemberLeft(_:)` (line 842-858) / `applyJoinedPetState(userId:pet:)` helper (line 817-832)
- `applyJoinedPetState` 模板：(i) 入参 pet（含 currentState）；(ii) `HomePetState(rawValue:)` 映射；(iii) 未知值 fallback `.rest` + `os_log(.error)`；(iv) `memberPetStates[userId] = mappedState`
- V1 §12.3 line 2251 钦定 client merge 行为：
  - (a) **roster 中已存在该 userId entry** → 更新其 `pet.currentState`（仅覆盖该字段，**不**影响 `nickname` / `avatarUrl` / `pet.petId`）
  - (b) **roster 中不存在该 userId entry**（理论不该发生 —— 同房间内其他成员的 entry 应在握手 `room.snapshot` 时已建立，或后续 `member.joined` 增量；如果 entry 不存在表示 roster 与 server 状态严重不一致）→ 走 `安全忽略 + log warn` 路径（**不**为单条 `pet.state.changed` 新增 roster entry —— 状态变更广播不携带 `nickname` / `avatarUrl` 等成员展示字段，新增 entry 无法正常渲染）
- V1 §12.3 line 2253 钦定 `payload.userId == 当前 user.id` 走同一路径（**禁止** client 仅因 `userId == self` 而丢弃消息）—— 是 §5.2 self-broadcast 对称兜底规则的前置条件

**预期行为**：

1. **新增私有方法 `applyPetStateChanged(_:)`**（紧接 `applyMemberLeft(_:)` 之后；与既有 apply 方法在 `// MARK: - Story 12.4 incremental mutate (member.joined / member.left)` MARK 之下 / 自然成为 Story 15.2 增量 mutate 路径）：

   ```swift
   /// V1 §12.3 client merge contract `pet.state.changed` 处理路径（行 2251）.
   ///
   /// **增量 mutate 语义**（**不是** full snapshot replacement）：
   ///   - 分支 (a) roster 中已存在 `payload.userId` entry → 字段级 merge：仅覆盖 `memberPetStates[userId]`
   ///     （不影响 `members[].name` / `level` / `status` / `isHost` 等）
   ///   - 分支 (b) roster 中不存在 `payload.userId` entry（理论不该发生）→ ignore + log warn
   ///     （**不**新增 roster entry —— 协议层 pet.state.changed payload 不携带 nickname / avatarUrl 等成员展示字段，
   ///     新增 entry 会渲染为占位"成员"无法表达完整成员信息；与 V1 §12.3 line 2251 (b) 钦定"安全忽略"一致）
   ///
   /// **未知 currentState 值兜底**（与 applySnapshot / applyJoinedPetState 同精神，Story 15.1 AC1 已落地相同 fallback 模式）：
   ///   - `HomePetState(rawValue: payload.currentState) != nil` → 写入映射后的状态
   ///   - 未知值（如 `99`）→ fallback `.rest` + `os_log(.error)`
   ///
   /// **self-broadcast 走同一路径**：V1 §12.3 line 2253 钦定 `payload.userId == 当前 user.id` 也是合法消息（server 不过滤）；
   /// client 必须**统一**处理路径（不为 self 走 short-circuit / 不丢弃 self 消息）—— 是 §5.2 self-broadcast 对称兜底规则的前置条件
   /// （HTTP 200 与 self-broadcast 任一先到的信号都驱动本地 self entry roster pet state 更新；后到信号走 merge no-op）.
   /// 本 story 仅落地"接收 + merge"路径；self-broadcast 触发 HTTP state-sync 的发起侧由 Story 15.4 落地.
   private func applyPetStateChanged(_ payload: PetStateChangedPayload) {
       // 分支 (b) 守护：roster 不存在该 userId → ignore + log warn（不新增 entry）
       guard members.contains(where: { $0.id == payload.userId }) else {
           os_log(.error,
                  "RealRoomViewModel: applyPetStateChanged userId not in members (userId=%{public}@, members.count=%{public}d)",
                  payload.userId, members.count)
           return
       }
       // 分支 (a) merge：映射 HomePetState + 未知值兜底 `.rest`
       let mappedState: HomePetState
       if let parsed = HomePetState(rawValue: payload.currentState) {
           mappedState = parsed
       } else {
           os_log(.error,
                  "RealRoomViewModel.applyPetStateChanged: unknown payload.currentState=%{public}d for userId=%{public}@; fallback .rest",
                  payload.currentState, payload.userId)
           mappedState = .rest
       }
       memberPetStates[payload.userId] = mappedState
       os_log(.debug,
              "RealRoomViewModel: applyPetStateChanged merged (userId=%{public}@, currentState=%{public}d)",
              payload.userId, payload.currentState)
   }
   ```

2. **`memberPetStates[userId]` 字段级 merge 语义保证**：
   - 字段级 merge：仅覆盖 `memberPetStates[payload.userId]`，**不**触碰 `memberPetStates` 中其他 userId 的 entry（与 swiftui dict subscript 默认语义一致：`dict[key] = newValue` 仅 mutate 该 key）
   - **不**触碰 `self.members`（V1 §12.3 line 2251 (a) "仅覆盖该字段，不影响 nickname / avatarUrl / pet.petId"）
   - **不**触碰 `self.wsState`（连接状态不变）

**红线**：

- **不**在 `applyPetStateChanged` 内 mutate `self.members`（V1 §12.3 钦定字段级 merge —— `pet.state.changed` 不携带成员展示字段，不该重写 RoomMember struct 任何字段）
- **不**对 `payload.petId` 做存在性 / 一致性校验（V1 §12.3 不要求 client 校验 petId 与已知 pet 关系；client 端目前 RoomMember 不存 petId，无校验对象）
- **不**为"自己 / 别人"做差异化 merge 路径（V1 §12.3 line 2253 钦定统一路径）
- **不**用 `applyJoinedPetState` helper（pet 入参是 `MemberJoinedPet?` 类型 —— pet.state.changed 走 `Int` currentState 不走 `MemberJoinedPet?`；两者类型不同复用没有意义；保留各 apply 方法独立演进）
- **不**在 `applyPetStateChanged` 失败路径做 retry / state recovery（fire-and-forget client 侧对称：server 端 fire-and-forget 广播失败仅 log warn，client 端单条 message merge 失败也仅 log warn，断链时由 reconnect 走 room.snapshot 全量重新对齐 —— V1 §12.3 line 2254 钦定 fallback 由 reconnect snapshot 覆盖；不属本 story scope）

**对应 Tasks**: Task 4.1

---

### AC5 — 单元测试 ≥4 case（mocked WebSocketClient）覆盖 epics.md §15.2 钦定的 4 个 case + ≥1 case codec 层 envelope/payload 解码

**扩展测试文件**：

- **主测试文件** `iphone/PetAppTests/Features/Room/RealRoomViewModelTests.swift`（Story 12.1 / 12.3 / 12.4 / 15.1 已落地大量 case；本 story **扩展**新 case 平级累加，**不**删除既有 case）
- **codec 测试文件** `iphone/PetAppTests/Core/Networking/WSMessageCodecTests.swift`（Story 12.2 / 12.4 落地 envelope 测试；本 story **扩展**至少 1 case 覆盖 `pet.state.changed` 路由 + 1 case 覆盖 `userId == ""` 语义校验 fallback `.unknown`）

**必须覆盖的测试 case**（按 epics.md §Story 15.2 行 2402-2406 钦定的 4 个 case）：

```
case#A happy: 收到 pet.state.changed { userId: "u_bob", currentState: 3 }
  → ViewModel.memberPetStates["u_bob"] == .run
  测试方法（RealRoomViewModelTests）：
    1. 先 emit snapshot 含 u_alice + u_bob 两成员（currentState=1/1）→ 等 waitForMembersCount(expected: 2)
    2. 断言 vm.memberPetStates == ["u_alice": .rest, "u_bob": .rest]
    3. emit .petStateChanged(PetStateChangedPayload(userId: "u_bob", petId: "p_b", currentState: 3))
       → 等 await Task.yield() 让 main-actor consumer 投递
    4. 断言 vm.memberPetStates["u_bob"] == .run + vm.memberPetStates["u_alice"] == .rest（字段级 merge：仅覆盖 u_bob）
    5. 断言 vm.members.count == 2 + vm.members[0].name 与 vm.members[1].name 未被改写（仅 memberPetStates mutate，members 不动）

case#B happy: 收到自己的 pet.state.changed { userId: <self>, currentState: 2 }
  → 自己的 petState 同步更新（与"别人的广播"统一路径，self-broadcast 对称兜底前置）
  测试方法（RealRoomViewModelTests）：
    1. 先 emit snapshot 含 u_alice（"self"占位即可，本 story 不要求注入 currentUserId）
       → vm.memberPetStates["u_alice"] == .rest
    2. emit .petStateChanged(PetStateChangedPayload(userId: "u_alice", petId: "p_a", currentState: 2))
       → 等 await Task.yield()
    3. 断言 vm.memberPetStates["u_alice"] == .walk
    4. 断言路径与 case#A 完全相同（确认 client 不为 self 做 short-circuit）

case#C edge: 收到 userId 不在房间的成员 → 忽略 + log warn，不报错
  → memberPetStates 不被污染（不为单条 pet.state.changed 新增 entry）
  测试方法（RealRoomViewModelTests）：
    1. 先 emit snapshot 含 u_alice 一个成员 → vm.memberPetStates == ["u_alice": .rest]
    2. emit .petStateChanged(PetStateChangedPayload(userId: "u_ghost", petId: "p_ghost", currentState: 2))
       → 等 await Task.yield()
    3. 断言 vm.memberPetStates == ["u_alice": .rest]（u_ghost 未被加入）
    4. 断言 vm.members 与 vm.memberPetStates 字段未被任何 mutate

case#D edge: 同时收到多个 pet.state.changed → 各自正确路由
  测试方法（RealRoomViewModelTests）：
    1. 先 emit snapshot 含 3 成员 u_alice / u_bob / u_charlie（全部 currentState=1）
    2. 连续 emit 3 条 .petStateChanged：
       - PetStateChangedPayload(userId: "u_alice", petId: "p_a", currentState: 1)
       - PetStateChangedPayload(userId: "u_bob", petId: "p_b", currentState: 2)
       - PetStateChangedPayload(userId: "u_charlie", petId: "p_c", currentState: 3)
    3. 等 await Task.yield()（多次让 main-actor 投递完三条）
    4. 断言 vm.memberPetStates == [
         "u_alice": .rest,
         "u_bob": .walk,
         "u_charlie": .run
       ]
    5. 断言 vm.members.count == 3 + members 字段全程未被改写
```

**追加 case（防御 + 守护语义覆盖）**：

```
case#E edge: 收到未知 currentState 值（如 99）→ fallback `.rest` + log error，不报错
  测试方法（RealRoomViewModelTests）：
    1. 先 emit snapshot 含 u_alice
    2. emit .petStateChanged(PetStateChangedPayload(userId: "u_alice", petId: "p_a", currentState: 99))
       → 等 await Task.yield()
    3. 断言 vm.memberPetStates["u_alice"] == .rest（fallback）

case#F edge: streamRoomId 守护防 cross-room race（与 Story 12.4 r2 P1 lesson 同精神）
  测试方法（RealRoomViewModelTests）：
    1. 直接调 vm.handle(message: .petStateChanged(PetStateChangedPayload(userId: "u_alice", petId: "p_a", currentState: 2)),
                       streamRoomId: "<oldRoomId>")
       —— 在 vm.lastObservedRoomId == "<currentRoomId>" 时
    2. 断言 vm.memberPetStates 未被改写（守护层挡住 stale）
```

**codec 测试 case**（`WSMessageCodecTests.swift`）：

```
case#G happy: codec.decode 解析合法 pet.state.changed envelope
  测试方法：
    let json = """
    {
      "type": "pet.state.changed",
      "requestId": "",
      "payload": { "userId": "u_alice", "petId": "p_a", "currentState": 2 },
      "ts": 1776920345000
    }
    """
    let msg = WSMessageCodec.decode(json)
    → 断言 msg == .petStateChanged(PetStateChangedPayload(userId: "u_alice", petId: "p_a", currentState: 2))

case#H edge: codec.decode 语义校验 fallback（empty userId / petId / payload schema mismatch）
  - empty userId → .unknown(rawType: "pet.state.changed")
  - empty petId → .unknown(rawType: "pet.state.changed")
  - missing currentState field → .unknown(rawType: "pet.state.changed")
```

**测试基础设施约束**（与 Story 12.1 / 12.3 / 12.4 / 15.1 一致）：

- XCTest only（`@testable import PetApp`）
- `@MainActor` 标注测试 class
- `WebSocketClientMock.emit(_:)` 驱动 stream
- `await Task.yield()` / `waitForMembersCount(vm:expected:)` helper 让 AsyncStream 派发到 ViewModel `@Published`
- **不**引 ViewInspector / SnapshotTesting / **不**起 mock HTTP server
- 直调 `vm.handle(...)` 验证守护语义（与 Story 12.4 fix-review r2 P1 case 同模式）

**对应 Tasks**: Task 5.1, 5.2, 5.3, 5.4, 5.5, 5.6, 5.7, 5.8

---

### AC6 — Build verify + ios-simulator MCP 实跑验证

**必须通过**：

```bash
bash iphone/scripts/build.sh --test
```

- xcodebuild 编译通过
- 所有单测通过（含本 story 新增 ≥6 case + codec 新增 ≥2 case + 既有全部 case 不破）
- UITest 通过（本 story **不**新增 UITest case —— 15.1 已覆盖 PetSpriteView a11y 三态定位 / 15.3 将覆盖动效，本 story 数据层 merge 路径 UITest 无独立增量价值；既有 UITest case 不破）

**ios-simulator MCP 验证**（CLAUDE.md "iOS UI 验证（必跑）"）：

```
1. bash iphone/scripts/build.sh
2. install_app(app_path: iphone/build/DerivedData/Build/Products/Debug-iphonesimulator/PetApp.app)
3. launch_app(bundle_id: "com.zhuming.pet.app", terminate_running: true)
4. 通过 UITEST_FORCE_IN_ROOM 或 UITEST_ROOM_THREE_MEMBERS launch flag 进入房间页
5. ui_view 验证：成员列表每行渲染 PetSpriteView（与 15.1 基线一致）
6. ui_describe_all 验证 PetSpriteView a11y identifier 存在（基线不退化）
7. 通过 lldb 或 mock 注入构造（必要时）触发 RealRoomViewModel.applyPetStateChanged
   验证 memberPetStates dict mutate 后 SwiftUI 重渲染 PetSpriteView 状态切换（与 15.3 动效串联前的基线）

   注：实战 E2E 真实 server 触发由 Story 16.1 / 16.2 跨端联调覆盖；本 story MCP 验证仅基线（确保 build / 启动 / 进房间 / 渲染未破）
```

**lesson 14-4 r1 必读 / 必遵守**：[fire-and-forget 边界必须包住决定是否 broadcast 的前置 IO](../../docs/lessons/2026-05-12-fire-and-forget-boundary-must-include-prerequisite-io-14-4-r1.md) —— server 端 fire-and-forget 严格语义已锁定 client 端的对应假设：单条 `pet.state.changed` 可能因 server 端任何步骤失败而**根本不到达** client（不是延迟，是完全缺席）；client 实装层（本 story `applyPetStateChanged`）**不**应假设每条 server-side state-sync 都会有对应的 `pet.state.changed` 到达；状态对齐 fallback 由 reconnect 后 `room.snapshot` 全量重新下发兜底（V1 §12.3 line 2254 + Story 15.5 落地）—— 与本 story `applyPetStateChanged` "not-found userId → ignore + log warn"路径配套：即便偶发 server-side 异常路径让 client 缓存的 roster 与 server 真实状态偏离，单条 stray pet.state.changed 也不该污染 client 端 roster。

**对应 Tasks**: Task 6.1

---

### AC7 — Deliverable 清单

**修改文件**：

- `iphone/PetApp/Core/Networking/WebSocketClient.swift`（AC1：新增 `.petStateChanged` case + `PetStateChangedPayload` struct；同步更新 line 73 doc comment）
- `iphone/PetApp/Core/Networking/WSMessageCodec.swift`（AC2：decode 新增 `case "pet.state.changed"` 路由 + 语义校验 + .unknown fallback；新增 `PetStateChangedEnvelope` 私有 DTO）
- `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`（AC3 + AC4：handle switch 新增 `.petStateChanged` case + streamRoomId 守护；新增私有方法 `applyPetStateChanged(_:)`）
- `iphone/PetAppTests/Features/Room/RealRoomViewModelTests.swift`（AC5：新增 ≥6 case 覆盖 epics.md 钦定 4 case + 守护 + 未知值 fallback）
- `iphone/PetAppTests/Core/Networking/WSMessageCodecTests.swift`（AC5：新增 ≥2 case 覆盖 pet.state.changed 解码 happy + 语义校验 fallback）

**可能修改文件**：

- 无（本 story 不动 `MockRoomViewModel.swift` —— mock 在 UITest 路径仅作快照渲染兜底，pet.state.changed 实时路径不走 mock；不动 `RootView.swift` —— UITEST_ROOM_THREE_MEMBERS launch flag 路径在 15.1 已设置 memberPetStates 满足 PetSpriteView 渲染基线）

**不需新建文件**：本 story 不引入新 Swift 类型 / 新 Domain 模型 / 新 ViewModel / 新 UseCase / 新 Repository / 新 Helper / 新 Mock。`PetStateChangedPayload` 在 `WebSocketClient.swift` 内（与 既有 `RoomSnapshotPayload` / `MemberJoinedPayload` / `MemberLeftPayload` 同模式）。

**对应 Tasks**: Task 7.1

---

## Tasks / Subtasks

- [x] **Task 1.1** — `WSMessage` enum 新增 `.petStateChanged(PetStateChangedPayload)` case（AC1）
  - [x] 1.1.1 紧接 `.memberLeft` 之后、`.connectionStateChanged` 之前新增 case + doc comment（与 V1 §12.3 行 2212-2259 钦定语义对齐 + Story 15.2 标注）
  - [x] 1.1.2 同步更新 line 73 doc comment：`pet.state.changed 由 Epic 14 / Story 14.x 扩展` → `pet.state.changed 由 Story 15.2 扩展（已落地）`
- [x] **Task 1.2** — `PetStateChangedPayload` struct 新增（AC1）
  - [x] 1.2.1 紧接 `MemberLeftPayload` 之后新增独立 struct（与 `MemberJoinedPet` / `RoomSnapshotPet` 同模式；不复用其他 struct）
  - [x] 1.2.2 三字段（userId / petId / currentState）+ Equatable + Sendable + public init
- [x] **Task 2.1** — `WSMessageCodec.decode(_:)` switch 新增 `case "pet.state.changed"` 路由（AC2）
  - [x] 2.1.1 紧接 `case "member.left"` 之后、`default` 之前新增 case
  - [x] 2.1.2 do-try-catch 解码 + 失败 fallback `.unknown(rawType: "pet.state.changed")`
  - [x] 2.1.3 语义校验：empty userId / petId → log error + fallback `.unknown`（与 Story 12.4 fix-review r2 P2 同精神）
  - [x] 2.1.4 currentState 值域**不**强校验 1/2/3（由 ViewModel.applyPetStateChanged 层降级 `.rest`；codec 容忍 server 未来扩展）
- [x] **Task 2.2** — `PetStateChangedEnvelope` 私有 DTO 新增（AC2）
  - [x] 2.2.1 紧接 `MemberLeftEnvelope` 之后新增 envelope + payload DTO + toDomain 方法
- [x] **Task 3.1** — `RealRoomViewModel.handle(message:streamRoomId:)` switch 新增 `.petStateChanged` case + streamRoomId 守护（AC3）
  - [x] 3.1.1 紧接 `case .memberLeft` 守护块之后、`case .connectionStateChanged` 之前新增 case
  - [x] 3.1.2 streamRoomId 守护（与 `.memberJoined` / `.memberLeft` 同模式 + 同 log debug 措辞）
  - [x] 3.1.3 守护通过后调 `applyPetStateChanged(payload)`
  - [x] 3.1.4 handle 头部 doc comment（line 558-589）追加 Story 15.2 守护语义说明段
- [x] **Task 4.1** — `applyPetStateChanged(_:)` 私有方法新增（AC4）
  - [x] 4.1.1 紧接 `applyMemberLeft(_:)` 之后新增私有方法
  - [x] 4.1.2 分支 (b) 守护：`members.contains(where: { $0.id == payload.userId })` 否则 ignore + log warn（**不**新增 entry）
  - [x] 4.1.3 分支 (a) merge：`HomePetState(rawValue:)` 映射 + 未知值 fallback `.rest` + log error（与 applySnapshot / applyJoinedPetState 同精神）
  - [x] 4.1.4 仅 mutate `memberPetStates[payload.userId]`，不触碰 `self.members` / `self.wsState` 等其他字段（V1 §12.3 line 2251 (a) 字段级 merge 钦定）
  - [x] 4.1.5 log debug merge 成功路径（与既有 apply 方法日志格式对齐）
- [x] **Task 5.1** — RealRoomViewModelTests case#A: u_bob currentState=3 → memberPetStates["u_bob"] == .run + 字段级 merge 不污染其他成员（AC5）
- [x] **Task 5.2** — RealRoomViewModelTests case#B: self userId 走同一路径 → memberPetStates[self] 正确更新（AC5）
- [x] **Task 5.3** — RealRoomViewModelTests case#C: u_ghost 不在房间 → ignore + log warn，memberPetStates 未被污染（AC5）
- [x] **Task 5.4** — RealRoomViewModelTests case#D: 连续 3 条 pet.state.changed 各自正确路由（AC5）
- [x] **Task 5.5** — RealRoomViewModelTests case#E: 未知 currentState 值 99 → fallback `.rest` + log error（AC5）
- [x] **Task 5.6** — RealRoomViewModelTests case#F: streamRoomId 守护防 cross-room race（直调 handle）（AC5）
- [x] **Task 5.7** — WSMessageCodecTests case#G: 合法 pet.state.changed envelope 解码 happy（AC5）
- [x] **Task 5.8** — WSMessageCodecTests case#H: 语义校验 fallback (empty userId / empty petId / missing currentState) → `.unknown(rawType: "pet.state.changed")`（AC5）
- [x] **Task 6.1** — `bash iphone/scripts/build.sh --test` 全绿 + ios-simulator MCP 基线验证（启动 → 进房间 → PetSpriteView 渲染基线不退化）（AC6）
- [x] **Task 7.1** — Deliverable 清单核对 + 移到 sprint-status review 状态（由 dev-story 末尾流转，本 story 文件不再 hardcode）（AC7）

---

## Dev Notes

### 关键文档锚定

- `docs/宠物互动App_总体架构设计.md` — iOS Swift + SwiftUI / WebSocket
- `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §9.1 / §9.2（WS 子系统职责）
- `docs/宠物互动App_V1接口设计.md` §12.3 `### 宠物状态变更（pet.state.changed）` line 2212-2259（envelope 字段表 + 关键约束 + client merge contract 钦定）
- `docs/宠物互动App_V1接口设计.md` §5.2 line 545-560 self-broadcast 兜底规则（本 story self entry 同一路径处理的协议依据）
- `docs/宠物互动App_V1接口设计.md` §12.3 line 2349-2354 业务消息延后锚定段（pet.state.changed 已由 Story 14.1 锚定 + Story 14.4 server 端实装）
- `docs/宠物互动App_V1接口设计.md` §2.5 BIGINT 字符串化（userId / petId 是 BIGINT 字符串化下发）
- `_bmad-output/implementation-artifacts/15-1-房间页内多成员猫位渲染-snapshot-pet-currentstate-解析.md`（前置 story —— memberPetStates 真实写入 + PetSpriteView 渲染基线 + HomePetState→MotionState 桥接 + ios-simulator MCP 必跑 lesson）
- `_bmad-output/implementation-artifacts/14-4-pet-state-changed-ws-广播.md`（server 端 pet.state.changed WS 广播实装；本 story 接收 side）
- `_bmad-output/implementation-artifacts/14-3-修改-roomsnapshotbuilder-snapshot-含真实-pet-currentstate.md`（snapshot 三处 currentState 切真实；与本 story `pet.state.changed` 共同构成 server → client 四处权威等价桶）
- `_bmad-output/implementation-artifacts/12-2-websocketclient-封装.md`（WebSocketClient + WSMessageCodec + AsyncStream 设计 + envelope decode 模式）
- `_bmad-output/implementation-artifacts/12-4-成员加入-离开-ws-消息处理.md`（**本 story 模板**：member.joined / member.left codec 解码 + 语义校验 + streamRoomId 守护 + apply 方法字段级 merge —— 直接对照实装）
- `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md` §3.1（测试栈钦定 XCTest only）
- `_bmad-output/implementation-artifacts/decisions/0010-iphone-appstate.md`（AppState 注入规则 / memberPetStates 字段归属为 ViewModel transient）

### Source tree 涉及位置

```
iphone/
  PetApp/
    Core/
      Networking/
        WebSocketClient.swift          # AC1 修改：扩 WSMessage.petStateChanged + PetStateChangedPayload struct
        WSMessageCodec.swift           # AC2 修改：decode switch 加 "pet.state.changed" 路由 + PetStateChangedEnvelope DTO
        WebSocketClientMock.swift      # 不动（emit(_:) 自动覆盖新增 case；mock 是黑盒）
    Features/
      Room/
        ViewModels/
          RealRoomViewModel.swift      # AC3 + AC4 修改：handle switch 加 case + applyPetStateChanged 新方法
          RoomViewModel.swift          # 不动（基类 @Published memberPetStates: [String: HomePetState] 已就位 Story 12.1）
          MockRoomViewModel.swift      # 不动（mock 不处理实时消息流；UITest 路径走 launch flag 注入）
          RoomScaffoldDefaults.swift   # 不动
        Views/
          RoomScaffoldView.swift       # 不动（Story 15.1 已落地 PetSpriteView 渲染 memberPetStates；本 story 仅驱动数据层）
      Home/
        Models/
          HomeData.swift               # 不动（HomePetState enum + motionState 桥接 Story 15.1 已落地）
        Views/
          PetSpriteView.swift          # 不动（Story 8.4 / 15.1 已落地；本 story 不动渲染层）
  PetAppTests/
    Core/
      Networking/
        WSMessageCodecTests.swift      # AC5 扩展：新增 ≥2 case 覆盖 pet.state.changed happy + 语义校验 fallback
    Features/
      Room/
        RealRoomViewModelTests.swift   # AC5 扩展：新增 ≥6 case 覆盖 epics.md 钦定 4 case + 守护 + 未知值 fallback
  PetAppUITests/
    RoomUITests.swift                  # 不动（15.1 已覆盖 a11y 三态定位；15.3 将覆盖动效；本 story 数据层无独立 UI 测试增量价值）
```

### Testing 标准摘要

- **单测**（PetAppTests target）：
  - XCTest only；`@MainActor` 标注测试 class
  - `WebSocketClientMock.emit(_:)` 驱动 stream + `await Task.yield()` / `waitForMembersCount(vm:expected:)` helper 让 AsyncStream 派发到 ViewModel `@Published`
  - 守护语义类 case 直调 `vm.handle(...)`（与 Story 12.4 fix-review r2 P1 lesson 一致）
  - codec 类 case 直调 `WSMessageCodec.decode(_:)`（与 Story 12.2 / 12.4 既有 case 一致）
  - 断言 `vm.memberPetStates` dict 内容 + `vm.members` 字段未被改写
- **UITest**：本 story 不新增（数据层 merge 路径无独立 UI 测试增量价值）
- **build verify**：`bash iphone/scripts/build.sh --test` 全绿；**ios-simulator MCP 必跑**（CLAUDE.md "iOS UI 验证（必跑）"；15.1 lesson 钦定）

### Project Structure Notes

- **不复用 RoomSnapshotPet / MemberJoinedPet 类型**：三者都映射相同业务字段（petId + currentState），但每条业务 WS 消息保留各自 payload struct（独立演进空间，与 Story 12.4 钦定模式一致；client 端独立类型避免跨消息耦合）
- **不引入新的 ViewModel / UseCase**：`applyPetStateChanged` 是 ViewModel transient 状态字段（memberPetStates）的 mutate 路径；不需独立 UseCase（与 Story 12.4 applyMemberJoined / applyMemberLeft 同精神）
- **不让 codec 强校验 currentState ∈ {1,2,3}**：codec 仅挡"缺字段 / 空字符串"语义违反，**不**挡"未知枚举值"语义降级 —— 容忍 server 未来扩展（如假设性的 sleep=4）；ViewModel.applyPetStateChanged 层做 HomePetState 映射 + 未知值降级 `.rest`（与 Story 15.1 AC1 同模式）
- **streamRoomId 守护必加**：V1 §12.3 line 2223-2230 钦定 payload 不含 room.id —— 与 member.joined / member.left 同类协议层局限，必须依靠 stream-lifecycle 守护防 cross-room race（Story 12.4 fix-review r2 P1 lesson）
- **self entry 路径统一**：V1 §12.3 line 2253 钦定"自己的 self-broadcast 也走 client merge 同一路径"，**禁止** client 仅因 `userId == self` 而丢弃消息（破坏 §5.2 self-broadcast 对称兜底规则；本 story 不实装 self 触发 state-sync 的发起侧，由 Story 15.4 落地，但接收侧路径必须正确）

### Previous story intelligence（必读 lessons）

1. **Story 12.4 fix-review r2 P1 lesson**（streamRoomId 守护）—— 本 story `.petStateChanged` 路径直接套用该守护模式
2. **Story 12.4 fix-review r2 P2 lesson**（codec 语义层校验 empty string）—— 本 story `case "pet.state.changed"` 路径必须校验 `userId.isEmpty` / `petId.isEmpty`
3. **Story 15.1 AC1 + r1 lesson**（HomePetState 未知值 fallback + ios-simulator MCP 必跑）—— 本 story `applyPetStateChanged` 未知值 fallback 直接套用 + Task 6.1 必跑 MCP 验证基线
4. **Story 14.4 r1 lesson**（fire-and-forget 边界必须包住"决定是否 broadcast 的前置 IO"）—— server 端的 fire-and-forget 严格语义反向锁定 client 端的假设：单条 pet.state.changed 可能完全缺席；client 不应假设"每条 server-side state-sync 都对应一条 pet.state.changed 到达"，fallback 由 reconnect snapshot 兜底
5. **Story 14.1 r10 lesson**（权威等价桶字段方向区分 client→server / server→client / ack-only）—— 本 story 接收的 `pet.state.changed.payload.currentState` 方向是 server→client，与权威等价桶四处（room.snapshot / GET /rooms / member.joined / pet.state.changed）同级别
6. **Story 12.1 fix-review r3 lesson**（stale-snapshot-discard-by-room-id）—— 本 story `.roomSnapshot` 路径不动，但 `.petStateChanged` 走 streamRoomId 守护是同精神

### Lessons reading list（dev 实装时必读）

`docs/lessons/` 内本 story 必读：

- `2026-05-12-fire-and-forget-boundary-must-include-prerequisite-io-14-4-r1.md`（client 端假设 pet.state.changed 可能缺席；本 story applyPetStateChanged 不依赖 retry / state recovery）
- `2026-05-12-swiftui-frame-clipped-does-not-scale-15-1-r1.md`（ios-simulator MCP 必跑；本 story 不改 UI 但 Task 6.1 必须实跑验证基线）
- `2026-05-12-nil-deref-defense-and-integration-evidence-14-3-r1.md`（client 端无 Optional 解引用风险但保持防御性 nil-check 习惯）
- `2026-05-12-cross-section-equivalence-claim-must-fence-prerequisites-and-self-broadcast-fallback-2.md`（self entry 与他人 entry 路径统一 / self-broadcast 对称兜底前置）
- `2026-05-12-self-broadcast-ui-driver-and-freeze-boundary-and-self-vs-others-priority-4.md`（本 story 落地 self 接收路径；Story 15.4 落地 self 发起路径）
- `2026-05-12-state-sync-pet-less-noop-consistent-with-home-room-snapshot-14-1-r7.md`（pet-less 账号 server 不广播 pet.state.changed → client 端 applyPetStateChanged 自然不需要为 pet-less 做特殊路径）

### References

- [Source: docs/宠物互动App_总体架构设计.md] — iOS Swift+SwiftUI / WebSocket
- [Source: docs/宠物互动App_V1接口设计.md#12.3] — `### 宠物状态变更（pet.state.changed）` envelope 字段表 + 关键约束 + client merge contract（行 2212-2259）
- [Source: docs/宠物互动App_V1接口设计.md#5.2] — POST /pets/current/state-sync schema + self-broadcast 对称兜底（行 545-560 + 行 593 等价分层）
- [Source: docs/宠物互动App_V1接口设计.md#2.5] — BIGINT 字符串化（userId / petId 是 BIGINT 字符串化下发）
- [Source: _bmad-output/planning-artifacts/epics.md] — Epic 15 Story 15.2 acceptance criteria（行 2389-2407）
- [Source: _bmad-output/implementation-artifacts/15-1-房间页内多成员猫位渲染-snapshot-pet-currentstate-解析.md] — Story 15.1 落地（memberPetStates 真实写入 + HomePetState→MotionState 桥接 + 未知值 fallback 模式）
- [Source: _bmad-output/implementation-artifacts/14-4-pet-state-changed-ws-广播.md] — Story 14.4 server 端 WS 广播实装（含发起者自己 + fire-and-forget 严格语义）
- [Source: _bmad-output/implementation-artifacts/12-4-成员加入-离开-ws-消息处理.md] — Story 12.4 落地 member.joined / member.left WS 消息处理（**本 story 模板**：codec 解码 + 语义校验 + streamRoomId 守护 + apply 方法字段级 merge）
- [Source: _bmad-output/implementation-artifacts/12-2-websocketclient-封装.md] — WebSocketClient + WSMessageCodec + envelope decode 模式 + AsyncStream 设计
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.1] — 测试栈钦定 XCTest only
- [Source: _bmad-output/implementation-artifacts/decisions/0010-iphone-appstate.md#3.1] — AppState 注入规则（memberPetStates 是 ViewModel transient 字段，不入 AppState 白名单）
- [Source: iphone/PetApp/Core/Networking/WebSocketClient.swift#69-110] — WSMessage enum + 既有 `.roomSnapshot` / `.memberJoined` / `.memberLeft` 模式（doc comment + case 顺序参照）
- [Source: iphone/PetApp/Core/Networking/WebSocketClient.swift#180-228] — Story 12.4 既有 MemberJoinedPayload / MemberJoinedPet / MemberLeftPayload struct 模板
- [Source: iphone/PetApp/Core/Networking/WSMessageCodec.swift#60-95] — Story 12.4 既有 case "member.joined" / case "member.left" 路由 + 语义校验模板
- [Source: iphone/PetApp/Core/Networking/WSMessageCodec.swift#196-233] — Story 12.4 既有 MemberJoinedEnvelope / MemberLeftEnvelope 私有 DTO 模板
- [Source: iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift#588-672] — handle(message:streamRoomId:) switch 既有 7 case + 守护层模板
- [Source: iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift#776-832] — Story 15.1 既有 applyMemberJoined + applyJoinedPetState helper（HomePetState 映射 + 未知值 fallback 模式参照）
- [Source: iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift#842-858] — Story 12.4 既有 applyMemberLeft 模板（roster ignore + log warn 模式参照）
- [Source: iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift#42] — @Published memberPetStates: [String: HomePetState] 字段（基类已就位）
- [Source: iphone/PetApp/Features/Home/Models/HomeData.swift#120-144] — HomePetState enum + motionState 桥接（Story 15.1 落地；本 story 不动）
- [Source: iphone/PetAppTests/Features/Room/RealRoomViewModelTests.swift] — Story 12.1 / 12.3 / 12.4 / 15.1 既有测试 case + waitForMembersCount helper 模板

### Latest tech information

- **AsyncStream 投递 + @MainActor**：与 Story 12.4 / 15.1 同模式，`WebSocketClientMock.emit(_:)` → `client.messages` AsyncStream yield → `messageConsumerTask` 在 main actor 上调 `handle(message:streamRoomId:)` → 测试用 `await Task.yield()` 等派发；不引 `XCTestExpectation` race（既有 helper 已足够）
- **HomePetState rawValue Int 映射**：`HomePetState(rawValue: payload.currentState)` 直接映射 1/2/3；未知值 returns nil → fallback `.rest`（Story 15.1 落地模式延续）
- **Equatable 综合实现**：`WSMessage` enum 走默认 Equatable 综合，新增 `.petStateChanged` case 自动获得 == 比较（依赖 `PetStateChangedPayload: Equatable` —— AC1 钦定）；测试断言可直接 `XCTAssertEqual(msg, .petStateChanged(...))`
- **Swift Concurrency 不变**：`@MainActor` + `ObservableObject` + `@Published` 模式（与 Story 12.1 / 12.3 / 12.4 / 15.1 一致）；不采用 iOS 17+ `@Observable` macro

### Project context reference

`_bmad-output/implementation-artifacts/decisions/` 内本 story 必读 ADR：

- `0010-iphone-appstate.md` — AppState 单 source of truth 规则（memberPetStates 是 ViewModel transient 字段，不入 AppState 白名单 7 字段；本 story `applyPetStateChanged` mutate 仅落在 ViewModel @Published 字段，不写 AppState）
- `0002-ios-stack.md` — 测试栈 XCTest only
- `0009-ios-stack.md` — iPhone 工程目录决策（导航架构 / 复用既有 RootView UITEST_ROOM_THREE_MEMBERS 路径）

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) — bmad-dev-story workflow

### Debug Log References

- RED phase build (compile fail expected): tests reference `WSMessage.petStateChanged` / `PetStateChangedPayload` before implementation; xcodebuild test failed with "Type 'WSMessage' has no member 'petStateChanged'" / "Cannot find 'PetStateChangedPayload' in scope" — exactly as required for RED.
- GREEN phase: `bash iphone/scripts/build.sh --test` → 585 tests pass, 0 failures, 24.221s.
- ios-simulator MCP baseline（AC6）：iPhone 17 Pro 模拟器 UITEST_FORCE_IN_ROOM + UITEST_ROOM_THREE_MEMBERS 路径启动 → 房间页正常渲染 3 成员（Alice/Bob/Charlie）+ 每行 PetSpriteView 可见（state 1/2/3 三态各一），Story 15.1 baseline 未退化（注：MCP `ui_describe_all` 返回空可能是 idb 与 iPhone 17 Pro 兼容问题，已用 `xcrun simctl io screenshot` 替代视觉验证）.

### Completion Notes List

- AC1 ✅ `WSMessage.petStateChanged(PetStateChangedPayload)` case + `PetStateChangedPayload` struct（userId / petId / currentState / Equatable / Sendable）落地于 `iphone/PetApp/Core/Networking/WebSocketClient.swift`，与既有 `MemberJoinedPet` / `RoomSnapshotPet` 独立类型同模式；line 73 doc comment 同步更新为"由 Story 15.2 扩展（已落地）".
- AC2 ✅ `WSMessageCodec.decode` 新增 `case "pet.state.changed"` 路由 + `PetStateChangedEnvelope` 私有 DTO；语义校验 empty userId / empty petId → `.unknown(rawType: "pet.state.changed")`；currentState 值域不做强校验（容忍 server 未来扩展）.
- AC3 ✅ `RealRoomViewModel.handle(message:streamRoomId:)` switch 新增 `.petStateChanged` case；streamRoomId 守护与 `.memberJoined` / `.memberLeft` 同模式（防 cross-room race）；handle 头部 doc comment 追加 Story 15.2 守护语义说明.
- AC4 ✅ `applyPetStateChanged(_:)` 私有方法落地：分支 (a) roster 存在 → 字段级 merge `memberPetStates[userId]`；分支 (b) 不存在 → ignore + log warn（不新增 entry）；未知 currentState → fallback `.rest` + log error；self entry 走同一路径（V1 §12.3 line 2253 钦定）.
- AC5 ✅ 单元测试 6 case（A happy / B self-broadcast / C unknown user / D multiple / E unknown currentState / F stale streamRoomId）+ codec 4 case（G valid envelope / H empty userId / empty petId / missing currentState）全部通过；测试均放在 `iphone/PetAppTests/Features/Room/RealRoomViewModelTests.swift`（与 Story 12.4 codec 测试同一文件，跟随既有模式）.
- AC6 ✅ `bash iphone/scripts/build.sh --test` 全绿（585 tests）；ios-simulator MCP 启动 + 房间页渲染 baseline 未退化（screenshot 验证三成员 + PetSpriteView 三态）.
- AC7 ✅ Deliverable 清单核对完成，File List 已填.

### File List

- `iphone/PetApp/Core/Networking/WebSocketClient.swift`（AC1：新增 `.petStateChanged` case + `PetStateChangedPayload` struct + line 73 doc comment 同步）
- `iphone/PetApp/Core/Networking/WSMessageCodec.swift`（AC2：decode 新增 `case "pet.state.changed"` 路由 + 语义校验 + `.unknown` fallback；新增 `PetStateChangedEnvelope` 私有 DTO）
- `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`（AC3 + AC4：handle switch 新增 `.petStateChanged` case + streamRoomId 守护；新增私有方法 `applyPetStateChanged(_:)`；handle 头部 doc comment 追加 Story 15.2 守护语义说明）
- `iphone/PetAppTests/Features/Room/RealRoomViewModelTests.swift`（AC5：新增 10 case 覆盖 6 ViewModel happy/edge case + 4 codec envelope 解码 + 守护 + 语义校验 fallback；新增 `waitForMemberPetState` helper）

### Change Log

| 日期 | 操作 | 内容 |
|------|------|------|
| 2026-05-12 | create-story | Story 15.2 上下文引擎分析完成 —— 综合 Story 14.4 server 广播实装记录 + Story 12.4 WS 消息处理模板 + Story 15.1 memberPetStates / HomePetState fallback 模式 + V1 §12.3 client merge contract + 14-1 r1-r11 lessons + 14-4 r1 fire-and-forget 边界 lesson + 15-1 r1 ios-simulator MCP 必跑 lesson，创建全面开发指南 |
| 2026-05-12 | dev-story | 落地 AC1-AC7 全部：WSMessage 扩 `.petStateChanged` case + `PetStateChangedPayload` struct；WSMessageCodec 扩 `case "pet.state.changed"` 路由 + 语义校验 + envelope DTO；RealRoomViewModel.handle 扩 `.petStateChanged` case + streamRoomId 守护；新增 `applyPetStateChanged` 私有方法（字段级 merge + 未知 currentState fallback `.rest` + self entry 同一路径）；扩 10 个 case 测试（6 ViewModel + 4 codec）全绿；`bash iphone/scripts/build.sh --test` 585 tests pass；ios-simulator MCP 渲染 baseline 未退化 |
