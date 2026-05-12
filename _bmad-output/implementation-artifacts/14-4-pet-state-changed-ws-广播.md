# Story 14.4: pet.state.changed WS 广播（service 层激活 14.2 pre-wire 的 broadcast 挂载点：UPDATE 成功后查 users.current_room_id 非 null → BroadcastToRoom 给该房间内全员（**包含**发起者自己）+ envelope BuildPetStateChangedEnvelope 新建 + fire-and-forget 严格语义 + 单测 ≥4 case + dockertest 集成测试覆盖"A 调 state-sync → A 自己收到 pet.state.changed"）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want **激活 Story 14.2 在 `server/internal/service/pet_service.go` `petServiceImpl.SyncCurrentState` 末尾 pre-wire 的 broadcast 挂载点**：在 UPDATE `pets.current_state` 成功（`err == nil`）之后、return ack 之前，新增 service 层 fire-and-forget 调用 `broadcastPetStateChanged(detachedCtx, userID, pet.ID, in.State)` —— 该方法 (a) 查 `userRepo.FindByID(ctx, userID)` 拿 `*User.CurrentRoomID *uint64`；(b) `CurrentRoomID == nil`（用户不在任何房间）→ 直接 return（**不**广播，**不** log warn，**不**影响 HTTP 200 响应；与 §5.2 服务端逻辑步骤 5 "null → 不广播" + §12.3 line 2218 一致）；(c) `CurrentRoomID != nil` → 构造 `ws.PetStateChangedPayload{UserID: strconv.FormatUint(userID, 10), PetID: strconv.FormatUint(petID, 10), CurrentState: int(state)}`（BIGINT 字符串化遵循 §2.5）→ `ws.BuildPetStateChangedEnvelope(payload)` 拿 `[]byte`（envelope 字段：`type="pet.state.changed"` / `requestId=""` / `payload=<上述>` / `ts=time.Now().UnixMilli()`，与 V1 §12.3 `### 宠物状态变更` 字段表 1:1 对齐）→ `s.broadcastFn(ctx, *user.CurrentRoomID, msgBytes)` 推送给该房间内**所有**当前在线 Session（**包含**发起者自己，与 `member.joined` / `member.left` 用 `broadcastExceptFn` 排除发起者**不同**语义，详见 V1 §12.3 line 2249 "广播范围"段 + line 55 节点 5 冻结声明）；任何步骤失败（FindByID 异常 / marshal 失败 / broadcastFn return error）一律 log warn 不返 error / 不回滚 UPDATE / 不影响 HTTP 200（fire-and-forget 严格语义，V1 §12.3 line 2217 + line 2254 钦定，与 11.8 `broadcastMemberJoined` / `broadcastMemberLeft` 模式一致）。具体实装为：(1) `server/internal/app/ws/snapshot.go`（沿用 11.8 envelope helpers 同包同文件）新增 `PetStateChangedPayload` struct + `BuildPetStateChangedEnvelope(payload PetStateChangedPayload) ([]byte, error)` helper（与既有 `BuildMemberJoinedEnvelope` / `BuildMemberLeftEnvelope` 同模式 + 同 json marshal 防御性 wrap）；(2) `server/internal/service/pet_service.go` `petServiceImpl` 新增私有方法 `broadcastPetStateChanged(ctx context.Context, roomID, userID, petID uint64, state int8)` 内部走 fire-and-forget 严格语义；`SyncCurrentState` 在 UPDATE 成功路径替换既有 `// TODO(Story 14.4)` 注释段为 `go s.broadcastPetStateChanged(detachedCtx, userID, pet.ID, in.State)`（**注意**：是否走 goroutine + detached ctx 由 dev-story 阶段权衡定夺 —— 推荐对齐 11.8 `enqueueRoomEvent` 模式走 detached ctx + 独立 goroutine + 10s timeout 防 client cancel 误中断；具体见 AC2 "broadcastPetStateChanged 实装细节"段）；(3) `server/internal/app/bootstrap/router.go` `petSvc` 构造点把 `nil, nil` 第 3/4 参数（14.2 pre-wire 形态）替换为 `deps.SessionMgr, petBroadcastFn` —— `petBroadcastFn` 是新的 `wsapp.BroadcastFn` closure（与 `roomBroadcastFn` 同 nil-tolerant 模式：`deps.SessionMgr == nil → 返 (0, nil) no-op` 路径；**不复用** `roomBroadcastFn` 闭包 —— 14.2 范围红线钦定"逻辑独立，本 story 直接传 nil 让 14.4 灵活决定"，pet 广播本质上确实独立但闭包实现一致）；(4) 单元测试覆盖 ≥4 case（mocked PetRepo / UserRepo / BroadcastFn / SessionManager）：happy 用户在房间 → broadcastFn 调用 1 次 + payload 字段正确 + envelope 正确 / 用户不在房间（CurrentRoomID nil）→ broadcastFn 调用 0 次 / UpdateCurrentStateByID err → broadcastFn 调用 0 次（UPDATE 失败路径不进 broadcast）/ broadcastFn 自身 return error → SyncCurrentState 仍返 HTTP 200 ack（fire-and-forget 严格语义）/ FindByID err → log warn + 不 broadcast + 不影响 HTTP 200；(5) 集成测试覆盖 ≥1 case（dockertest + 真实 WS）：建 user A + room X + A join X → A 建 WS 连接到房间 X → A 调 POST /api/v1/pets/current/state-sync `{state: 2}` → 验证 A 自己收到 `pet.state.changed` envelope（type / requestId="" / payload.userId="<A.id>" / payload.petId="<A.pet.id>" / payload.currentState=2 / ts != 0）+ DB pets.current_state = 2,
so that **下游 iOS Epic 15 全部 stories（特别是 Story 15.2 pet.state.changed WS 消息处理 / Story 15.4 自己状态变化时上报 state-sync）+ Epic 16 节点 5 demo 验收 + 跨端集成 e2e** 能基于"server 端实时广播 pet.state.changed 给同房间全员（含发起者自己）"的权威等价层全面就绪 —— 该权威等价层在 V1 §5.2 line 608-613 + §12.3 line 2252 钦定为"四处 server → client `pet.currentState` 字段（`pet.state.changed` / `room.snapshot` / `GET /rooms` / `member.joined`）自 Story 14.3 起承载相同权威级别"；本 story 是该桶最后一个 `pet.state.changed` server 端落地点（14.3 已落地三处 room.snapshot / GET /rooms / member.joined）。本 story **不**实装 iOS Epic 15 / Epic 16 / 任何 client-side 处理（pet.state.changed 接收 / merge / UI 驱动 / self-broadcast 兜底等都归属 iOS Story 15.x），**不**改 V1 接口文档（14.1 已冻结 §5.2 + §12.3 `### 宠物状态变更`），**不**改 14.2 已 done 的 handler / repo / `SyncCurrentState` 核心逻辑（err 二分 / pet-less noop / 参数校验等），**仅**激活 14.2 pre-wire 好的 broadcast 挂载点 + 新建 PetStateChangedEnvelope helper + router wire 真实实例。

## 故事定位（Epic 14 第四条 / **收官 story** = Epic 14 唯一 WS 广播实装 story；上承 14.1 契约 + 14.2 UPDATE 接口 + 14.3 权威等价层 + 11.8 broadcast 模式，下启 iOS Epic 15 + Epic 16）

- **Epic 14 进度**：14.1（POST /pets/current/state-sync + WS pet.state.changed 契约定稿，done）→ 14.2（POST /pets/current/state-sync 接口 + pets.current_state UPDATE 实装 + WS 广播 pre-wire 挂载点，done）→ 14.3（三处 server → client `pet.currentState` 字段同步切真实读取 `pets.current_state`：§10.3 GET /rooms / §12.3 room.snapshot / §12.3 member.joined，done）→ **14.4（本 story = Epic 14 收官 story；激活 14.2 pre-wire 的 broadcast 挂载点 + 新建 PetStateChangedEnvelope helper + router wire 真实实例）** → Epic 14 done。**14.4 是 Epic 14 收官 + iOS Epic 15 强前置**：iOS Story 15.2（pet.state.changed WS 消息处理）依赖本 story 落地的 server 端真实广播；iOS Story 15.4（自己状态变化时上报 state-sync）依赖本 story 落地的"发起者自己也收到 pet.state.changed"语义（让 client 端 self-broadcast UI 驱动 + HTTP 200 ack 双路兜底规则可生效，详见 V1 §5.2 line 547-551 self-broadcast 对称兜底）；Epic 16（节点 5 demo 验收 + 跨端集成 e2e）依赖 Epic 14 全部 done。

- **本 story 是 Epic 14 唯一 WS 广播实装 story**：14.1 是纯契约 / 14.2 是 HTTP REST 接口 + DB UPDATE / 14.3 是三处字段切真实（纯 SELECT / 三处既有 service 代码点改赋值来源）/ **14.4 是 service 层激活 14.2 pre-wire 挂载点 + 新增 ws envelope helper + router wire 真实实例**。改动范围相对集中（service / ws / router 三个文件 + 三个测试文件 + 1 个集成测试），与 11.8（成员加入 / 离开 WS 广播）模式直接对照（同样是"service 层 fire-and-forget broadcast + ws envelope helper + router wire BroadcastFn closure"四步实装）。

- **本 story 是 iOS Epic 15 / Epic 16 强前置**：
  - **iOS Story 15.2（pet.state.changed WS 消息处理）**：iOS 端 `RoomViewModel` 接收 WS `pet.state.changed` envelope → 走字段级 merge（仅覆盖 `members[].pet.currentState` 字段，不影响其他字段）→ 驱动 `PetSpriteView` 状态切换（.rest/.walk/.run）；本 story 是 iOS Story 15.2 的 server 端实时广播起点
  - **iOS Story 15.4（自己状态变化时上报 state-sync）**：iOS 端 `StateSyncUseCase.execute(state: Int)` 调 14.2 POST 接口 → 同时**自己也收到** `pet.state.changed`（本 story 钦定语义）→ 让 client 走 §5.2 self-broadcast 对称兜底（任一路径先到的信号都驱动本地 self entry UI 更新，后到信号走 merge no-op）；本 story 落地的"发起者自己也收到广播"是 iOS Story 15.4 self-broadcast 兜底 ruling 的 server 端实装基础
  - **iOS Story 15.5（跨房间状态恢复）**：用户进新房间 → WS 握手 → 收 `room.snapshot.members[].pet.currentState`（14.3 落地后真值）+ 加入房间后任何成员的实时 state-sync 都通过 14.4 广播到达；本 story 与 14.3 共同构成跨房间状态恢复的完整数据起点
  - **Epic 16（节点 5 demo 验收 + 跨端集成 e2e + tech debt 登记）**：14.x + iOS Epic 15 全部完成后由 Epic 16 收口；本 story 落地后跨端 e2e 可断言"A 在房间 X 内切 walk → 同房间 B 实时收到 `pet.state.changed` → B 看到 A 的猫切换为 walk 动画"

- **epics.md §Story 14.4 钦定**（行 2341-2363）：
  - **Given** Story 14.2 state-sync 接口已就绪 + Story 10.5 BroadcastToRoom 可用 + Story 14.1 WS 契约已定
  - **When** state-sync 成功更新 pets.current_state
  - **Then** service 检查 `users.current_room_id`:
    - 非 null → 调用 `BroadcastToRoom(currentRoomId, {type: "pet.state.changed", payload: {userId, petId, currentState}})`
    - null → 不广播（用户不在任何房间）
  - **And** 广播失败不影响 state-sync 接口结果（fire-and-forget）
  - **And** 同一秒多次 state-sync（即便业务上不该发生）→ 每次都广播，不去重（让 iOS 决定是否过滤）
  - **And** **单元测试覆盖**（≥4 case，mocked BroadcastToRoom）:
    - happy: 用户在房间 → state-sync 成功 → broadcast 调用 1 次，msg.type=pet.state.changed + payload 字段正确
    - happy: 用户不在房间 → broadcast 不被调用
    - edge: state-sync 失败 → broadcast 不被调用
    - edge: BroadcastToRoom 失败（网络 error）→ state-sync 接口仍返回成功
  - **And** **集成测试覆盖**（dockertest + Redis + 真实 WS）:
    - A + B 都在房间 X，A 建立 WS → A 调 /pets/current/state-sync {state: 2} → A 收到自己的 pet.state.changed 消息（含自己的 userId）
    - 注：广播给房间所有人含发起者自己，客户端逻辑统一

- **14.1 已 frozen contract（V1 §5.2 服务端逻辑步骤 5 + §12.3 `### 宠物状态变更`）—— 本 story 严格按契约实装，不重新评审 schema**：
  - **§5.2 服务端逻辑步骤 5**（V1 §5.2 line 532-541）：UPDATE 成功后 service 检查 `users.current_room_id`：
    - **非 null** → `BroadcastToRoom(currentRoomId, {type: "pet.state.changed", payload: {userId, petId, currentState}})` 广播给该房间内所有当前在线 Session（**包含**发起者自己）
    - **null** → 不广播（用户不在任何房间；HTTP 仍 200 OK + code = 0）
    - 广播失败 fire-and-forget：仅 log warning，**不**回滚 UPDATE，**不**影响 HTTP 响应（与 Story 11.8 `member.joined` 广播失败语义一致）
  - **§12.3 `### 宠物状态变更（pet.state.changed）`**（V1 line 2212-2259）：
    - **触发**：仅 Story 14.4 一处触发；任何 WS 层事件（含握手 / 心跳超时 / 断开重连 / 用户进出房间）**都不**触发 `pet.state.changed`（line 2219 钦定）
    - **envelope 字段表**：
      - `type` string 必填：固定值 `"pet.state.changed"`
      - `requestId` string 必填：固定 `""`（主动推送类消息，遵循 §12.3 通用信封）
      - `payload.userId` string 必填：状态变更的 user 主键（BIGINT 字符串化，遵循 §2.5）；来自当前 user.id
      - `payload.petId` string 必填：状态变更的 pet 主键（BIGINT 字符串化，遵循 §2.5）；来自 service 层 FindDefaultByUserID 查到的 pet.id
      - `payload.currentState` number (int) 必填：变更后宠物当前状态枚举（1=rest / 2=walk / 3=run）
      - `ts` number (int64) 必填：服务端发送时间戳（ms）；来源 `time.Now().UnixMilli()`（与 `member.joined` / `member.left` `ts` 字段语义一致）
    - **关键约束**（line 2247-2255）：
      - **广播范围：该房间内所有当前在线 Session（包含发起者自己）** —— 与 `member.joined` 排除加入者 / `member.left` 排除离开者**不同**语义；原因：(a) `member.joined` / `member.left` 是关系变化通知，发起者已通过 HTTP response 知道；(b) `pet.state.changed` server 端不区分接收者，让 client 端走单一 WS 权威路径
      - `payload.userId` / `payload.petId` / `payload.currentState` 都必填（**禁止** payload 为 `{}` 或缺任一字段）
      - 广播 fire-and-forget：任何步骤失败仅 log warning，server **不**重试
      - `ts` 字段（int64 ms）来源 `time.Now().UnixMilli()`；用途**仅限**客户端日志关联 + UI 辅助展示，**禁止**用作业务排序 / 状态新旧判定
  - **§1 节点 5 冻结声明**（V1 §1 line 47-56）：自 2026-05-12（Story 14.1 完成日）起，§5.2 + §12.3 `### 宠物状态变更` 进入冻结状态；本 story 严格遵守冻结契约，**不**修改 wire schema / json tag / 字段类型
  - **数据库 §6.4 状态枚举（`pets.current_state` TINYINT NOT NULL DEFAULT 1）**：1=rest / 2=walk / 3=run；本 story 仅消费该列值作 broadcast payload `currentState` 字段，**不**改 schema

- **14.2 pre-wire 形态（本 story 接管的具体挂载点）**：
  - **`petServiceImpl` struct 字段**（pet_service.go:108-115）：`sessionMgr ws.SessionManager` / `broadcastFn ws.BroadcastFn` 已预留，14.2 阶段 nil-tolerant 注入；14.4 在 router.go wire 真实实例时直接替换 `nil, nil` 为 `deps.SessionMgr, petBroadcastFn`，**不**改 struct 字段定义 / **不**改 NewPetService 签名（11.8 NewRoomService 同模式：14.2 阶段已对齐 4 参数，未来 14.4 / 14.6 扩展只换实参不改形参）
  - **`SyncCurrentState` TODO 占位**（pet_service.go:171-191）：UPDATE 成功 happy 路径 info log 之后、return ack 之前留了一段 `// TODO(Story 14.4): broadcast pet.state.changed if users.current_room_id != NULL` 注释 + 三行 `_ = s.sessionMgr` / `_ = s.broadcastFn` / `_ = s.userRepo` 防 compile 警告；14.4 实装时**完整替换**这一段为真实 broadcast 调用，**不**保留 TODO 注释（避免歧义 —— TODO 完成即应删除，与 11.8 r1 lesson 同模式）
  - **`router.go` wire**（router.go:415）：`service.NewPetService(petRepo, userRepo, nil, nil)` 第 3/4 参数当前是 `nil, nil`；14.4 替换为 `deps.SessionMgr, petBroadcastFn`（其中 `petBroadcastFn` 是新增的 closure，与既有 `roomBroadcastFn` 同模式 nil-tolerant）
  - **14.2 范围红线允许的范围内动作**：本 story 落地后 `petServiceImpl.sessionMgr` / `broadcastFn` / `userRepo` 字段全部进入业务路径（不再是 dead reference）—— 14.2 doc comment 标注的"14.4 才使用"承诺自此兑现；本 story 实装层应同步更新 doc comment 移除"本 story 不调用"等过时措辞

- **14-1 lessons 必须遵守（11 条 r1-r11 sequence；本 story 是 14.1 锚定的"pet.state.changed 实装"落地点）**：
  - **r1**（[幂等 + RowsAffected 误判 / WS envelope 字段归属](../../docs/lessons/2026-05-12-state-sync-idempotent-rowsaffected-and-ws-envelope-ts.md)）：广播 envelope `ts` 字段**必须在 server 端生成**（`time.Now().UnixMilli()`），**不**用 client 提交时间戳 / **不**用 DB updated_at；本 story `BuildPetStateChangedEnvelope` 内部 `Ts: time.Now().UnixMilli()`，与既有 `BuildMemberJoinedEnvelope` / `BuildMemberLeftEnvelope` 同模式（snapshot.go:465 / 488 既有实装）；service 层 `broadcastPetStateChanged` **不**重复生成 ts（envelope helper 内部生成，单一职责）
  - **r2**（[跨章节字段等价声明锁定前置 + ack vs 权威分层 + self-broadcast 丢失兜底](../../docs/lessons/2026-05-12-cross-section-equivalence-claim-must-fence-prerequisites-and-self-broadcast-fallback-2.md)）：本 story 广播 envelope `payload.currentState` 字段**直接用入参 `in.State`**（值类型 int8 → 转 int），**不**重新查 DB `pets.current_state` 读出来再下发 —— 与 14.2 `SyncCurrentState` response 回显入参同语义（值层等价）；但**本 story `payload.currentState` 入权威等价桶**（server → client，与 `room.snapshot` / `member.joined` / `GET /rooms` 同权威级别，详见 §5.2 line 608-613），而 14.2 `data.state` ack 信号**不入**权威等价桶（r10 锁定）；两者值相同但权威级别不同
  - **r3**（[member.joined `pet.currentState` 14.3 落地前 stale race + self-broadcast no-op 措辞基于到达顺序对称](../../docs/lessons/2026-05-12-member-joined-stale-state-and-self-broadcast-arrival-order-symmetric-3.md)）：本 story 广播范围**显式包含发起者自己**（与 r3 钦定的 "self-broadcast 到达走 merge no-op" 一致 —— self-broadcast 必须**真实到达** client 才能让 §5.2 self-broadcast 对称兜底规则生效；如果 server 端排除发起者，client 永远收不到 self-broadcast，无法满足"任一路径先到的信号都驱动本地 UI"对称规则）
  - **r4**（[self-broadcast UI 驱动 + 冻结边界声明区分抽象触发与阈值 + self vs 他人优先级](../../docs/lessons/2026-05-12-self-broadcast-ui-driver-and-freeze-boundary-and-self-vs-others-priority-4.md)）：本 story 实装"包含发起者自己"是 §5.2 self-broadcast 对称兜底的**前置条件**（无 self-broadcast 到达 → 对称兜底规则无意义）；service 层**不**为 self-broadcast 做特殊路径 / 不为 self vs 他人做差异化 fanout（统一调 `broadcastFn` 全 fanout，client 端自己识别 `payload.userId == self`，与 §12.3 line 2253 钦定一致）
  - **r5**（[临时窗口优先级 merge contract + `ts` 业务排序禁令 + 权威等价桶四处枚举](../../docs/lessons/2026-05-12-merge-contract-exception-and-ts-business-ordering-ban-and-ack-bucket-explicit-enum-5.md)）：本 story envelope `ts` 字段**仅作日志关联 + UI 辅助展示**（与 §12.3 line 2255 一致），**禁止** client 实装层用 `ts` 字段比较推断 `pet.state.changed` 新旧 —— 业务时序由 WS 连接内消息物理到达顺序 FIFO 保证；server 端**永远不**让 `ts` 字段参与任何业务路径判定（本 story service 层 `broadcastPetStateChanged` 不读 envelope.ts 反推任何事情）
  - **r6**（[state-sync err 二分锁定 + placeholder 例外白名单覆盖 self HTTP ack](../../docs/lessons/2026-05-12-state-sync-err-binary-and-placeholder-whitelist-self-http-ack-14-1-r6.md)）：本 story 在 `SyncCurrentState` 路径中**只**在 UPDATE 成功（`err == nil`）的分支添加 broadcast 调用 —— UPDATE 失败（`err != nil`）路径**禁止**触发广播（V1 §5.2 服务端逻辑步骤 5 钦定"成功 UPDATE 之后"，对应单测 case 3）；本 story 不引入第三条路径 / 不读 RowsAffected / 不为 broadcast 失败回滚 UPDATE
  - **r7**（[state-sync pet-less 与 /home / room / member.joined 同语义合法 edge case](../../docs/lessons/2026-05-12-state-sync-pet-less-noop-consistent-with-home-room-snapshot-14-1-r7.md)）：本 story 的 `SyncCurrentState` pet-less 路径（FindDefaultByUserID 返 ErrPetNotFound）**保持 14.2 已落地的 noop 行为** —— 跳 UPDATE + 跳广播 + 返 (output, nil)；pet-less 路径**永远不**触发广播（pet 不存在，payload.petId 无值可填，且 §5.2 服务端逻辑步骤 5 钦定"UPDATE 成功之后"，noop 路径无 UPDATE 发生）；不引入"pet-less 走 partial broadcast"等任何 contract drift
  - **r8**（[story 文件与 V1 doc 同步 self-broadcast 对称兜底 + `ts` 禁令 + 等价分层](../../docs/lessons/2026-05-12-story-file-must-stay-in-sync-with-frozen-v1-doc-14-1-r8.md)）：本 story 文件描述（Story / 故事定位 / AC / Dev Notes / References）的 schema 引用 / 错误码描述 / 广播范围语义 / fire-and-forget 语义 / ts 字段语义**严格**与 V1 doc + 14-1 / 14-2 / 14-3 story 文件 + 14-1 lessons 三方对齐；review 阶段任何"story 文件 vs V1 doc drift"都必须在本 story 内修复，不留给下游 iOS Epic 15
  - **r9**（[story 文件 RowsAffected 措辞 + 顶层 1003 引用 drift](../../docs/lessons/2026-05-12-story-file-rowsaffected-and-top-level-1003-drift-14-1-r9.md)）：本 story 文件全文 grep `RowsAffected` / `1003` 不应命中任何"业务路径触发"语义（本 story 不引入 1003 / 不读 RowsAffected）
  - **r10**（[Story AC 权威等价语义区分字段方向 client→server / server→client / ack-only](../../docs/lessons/2026-05-12-story-ac-authority-bucket-direction.md)）：本 story `pet.state.changed.payload.currentState` 字段方向**严格标注**为 server → client，权威等价桶（与 §10.3 GET / §12.3 room.snapshot / §12.3 member.joined 同级别）；**不**与 §5.2 request `state`（client → server，写入信号）/ §5.2 response `data.state`（ack-only）混淆
  - **r11**（[story 文件 14.3 落地范围三处统一 + References 1003 残留清理](../../docs/lessons/2026-05-12-story-file-14-3-scope-must-list-member-joined-14-1-r11.md)）：本 story 落地的 `pet.state.changed.payload.currentState` 是权威等价桶**第四处**（14.3 已落地三处 GET / room.snapshot / member.joined，14.4 落地最后一处 pet.state.changed），自本 story 起权威等价桶四处全部就绪；Story 文件 References 段不残留 1003 措辞 / 不残留 RowsAffected 措辞

- **14-3 r1 lesson 必须遵守（[nil-deref defense + hardcoded → 真实值切换的 integration test fixture](../../docs/lessons/2026-05-12-nil-deref-defense-and-integration-evidence-14-3-r1.md)）**：
  - **nil-deref 防御**：本 story 不涉及 `*int8` / `*uint64` 解引用（envelope payload 字段构造时直接读 `in.State int8` + `pet.ID uint64` + `*user.CurrentRoomID uint64`，前置已校验非 nil 才进入广播路径，但 `*user.CurrentRoomID` 解引用必须有显式 nil-check）—— 但 service 层 `broadcastPetStateChanged` 内部读 `user.CurrentRoomID *uint64` 时**必须** `if user.CurrentRoomID == nil { return }` 显式 guard，**禁止**直接 `*user.CurrentRoomID` 无 nil-check（即便函数级前置校验已 guard，仍保留 belt-and-suspenders 防御）
  - **integration test fixture 区别值**：本 story 集成测试中 state-sync `state: 2`（不用 default `1`）让 wire 上的 `payload.currentState=2` 与 hardcoded `1` placeholder（如果 14.4 未真实实装就会落到 `1`）能区分开来 —— 与 14-3 r1 lesson 钦定的 "hardcoded → 真实值切换 fixture 用区别值证明切换" 同模式

- **范围红线**（**严格遵守**，与 Story 11.8 / 14.2 / 14.3 同模式）：
  - 本 story **只**改：
    - `server/internal/app/ws/snapshot.go`（**扩展**；沿用 11.8 envelope helpers 同包同文件，新增 `PetStateChangedPayload` struct + `BuildPetStateChangedEnvelope(payload PetStateChangedPayload) ([]byte, error)` helper；紧接既有 `BuildMemberLeftEnvelope` 之后，与 V1 §12.3 `### 宠物状态变更` 字段表 1:1 对齐）
    - `server/internal/app/ws/snapshot_test.go`（**扩展**；新增 ≥2 case 覆盖 `BuildPetStateChangedEnvelope` happy + envelope JSON 结构断言 + ts 非 0；既有 SnapshotBuilder / member.joined / member.left envelope 测试**不动**）
    - `server/internal/service/pet_service.go`（**扩展激活预留路径**；(a) `petServiceImpl` 新增私有方法 `broadcastPetStateChanged(ctx context.Context, roomID, userID, petID uint64, state int8)` —— fire-and-forget 严格语义 + nil-guard + log warn 不返 error；(b) `SyncCurrentState` 在 UPDATE 成功 happy 路径 info log 之后**完整替换** 14.2 留的 `// TODO(Story 14.4)` 注释段 + `_ = s.sessionMgr` / `_ = s.broadcastFn` / `_ = s.userRepo` 三行防警告语句为真实 broadcast 触发逻辑 `go s.broadcastPetStateChanged(detachedCtx, *user.CurrentRoomID, in.UserID, pet.ID, in.State)`（详见 AC2 实装细节段）；(c) `PetService` interface doc comment + `petServiceImpl` struct doc comment + `NewPetService` doc comment 同步更新移除"14.4 才使用 / 本 story 不调用 / 14.4 预留"等过时措辞，改为"自 14.4 起 sessionMgr / broadcastFn / userRepo 全部进入业务路径"）
    - `server/internal/service/pet_service_test.go`（**扩展**；既有 5 case 单测中已涵盖"广播路径未被触发"占位 case（14.2 落地的 case 6），本 story 需要：(i) 删除既有 case 6 "广播路径未被触发的 wire 占位"或重写为反例；(ii) 新增 ≥4 case 覆盖 14.4 broadcast 路径：happy 用户在房间 → broadcastFn 调用 1 次 + payload 正确 / 用户不在房间（CurrentRoomID nil）→ broadcastFn 调用 0 次 / UpdateCurrentStateByID err → broadcastFn 调用 0 次 / broadcastFn 返 error → SyncCurrentState 仍返 HTTP 200 + nil error / FindByID err → log warn + broadcastFn 调用 0 次 + SyncCurrentState 仍返 HTTP 200；(iii) 同步更新既有 case 1/2/3/4 让 mock BroadcastFn / SessionManager 注入路径与新 broadcast 调用对齐 —— 现有 happy case 必须 mock UserRepo.FindByID 返一个 `*User{CurrentRoomID: &roomID}` 或 `CurrentRoomID: nil`（按 case 语义）让 broadcast 路径可控）
    - `server/internal/service/pet_service_integration_test.go`（**扩展**；既有 3 scenario 集成测试**不动**（dockertest pets.current_state 落库验证）；新增 1 case 覆盖 ws-end-to-end：建 user A + 默认 pet + room X（rooms 表 insert）+ A join room X（room_members 表 insert + users.current_room_id 设为 roomID）→ A 通过 ws.SessionManager.Register 建一个 mock WS Session 到房间 X → 调 SyncCurrentState({UserID: A, State: 2}) → 验证 broadcastFn 被调用 1 次 + msg 字节 unmarshal 后 envelope.type="pet.state.changed" + payload.userId="<A.id>" + payload.petId="<A.pet.id>" + payload.currentState=2 + ts != 0；**fixture 复用**既有 server/internal/repo/mysql/integration_test_helper.go + ws.NewSessionManager() helper；**不**真起 ws server / 不真握手 —— service 层集成测试边界到 broadcastFn 触发即可，端到端 WS 握手验证在 pets_handler_integration_test.go 或 ws_integration_test.go）
    - `server/internal/app/http/handler/pets_handler_integration_test.go`（**扩展**；既有 4 scenario 集成测试**不动**（happy / 1001 / 1002 / pet-less）；新增 1 case 覆盖 WS 端到端：dockertest + 启 ws gateway + 建 user A + room X + A join → A 通过 httptest gorilla/websocket 客户端 dial /ws/rooms/{roomId} 完成握手 → 收 room.snapshot 后 → A 调 POST /api/v1/pets/current/state-sync `{state: 2}` 带合法 Bearer token → 验证 HTTP 200 + envelope.data.state=2 + DB pets.current_state=2 + **A 自己** WS 通道收到 `pet.state.changed` envelope（type / requestId="" / payload.userId / payload.petId / payload.currentState=2 / ts != 0）；超时 5s 收不到 fail；与 ws_integration_test.go member.joined 端到端 case 同模式（参考 server/internal/app/ws/ws_integration_test.go 既有 fixture）；**fixture 复用**既有 router_test.go / ws_integration_test.go helper，**不**新建独立 ws dial helper）
    - `server/internal/app/bootstrap/router.go`（**最小改动**；(a) 在既有 `roomBroadcastFn` / `roomBroadcastExceptFn` closure 定义之后、`roomSvc := service.NewRoomService(...)` 之前**新增** `petBroadcastFn` closure（与 `roomBroadcastFn` 同模式 nil-tolerant，调用 `wsapp.BroadcastToRoom` 而非 `BroadcastToRoomExcept` —— 因为 pet.state.changed 广播范围含发起者自己）+ 同样的 doc comment 说明"含发起者自己，与 member.joined / member.left 排除发起者不同语义"；(b) `petSvc := service.NewPetService(petRepo, userRepo, nil, nil)` 第 3/4 参数替换为 `deps.SessionMgr, petBroadcastFn`；(c) 同步更新 `// Story 14.2 加：` 注释段，新增 `// Story 14.4 起 sessionMgr / broadcastFn 注入真实实例` 字样）
    - `server/internal/app/bootstrap/router_test.go`（**最小扩展**；既有 router_test.go 应覆盖 `POST /pets/current/state-sync` 路由本身（14.2 已扩展）；本 story 路由 path 不变，仅 service wire 参数改 —— 既有 fixture 应自动适配。如发现既有 router_test.go 缺少 `petBroadcastFn` nil-tolerant wire 路径覆盖，本 story 新增 1 case 验证 `deps.SessionMgr == nil` 时 petBroadcastFn no-op 不 panic；如不缺则**不动**）
    - 本 story 文件（Status 流转）+ sprint-status.yaml（14-4-pet-state-changed-ws-广播: backlog → ready-for-dev → in-progress → review → done）
  - **不**改：
    - 任何 `docs/宠物互动App_*.md`（V1接口设计.md §5.2 / §12.3 + 数据库设计.md §5.3 / §6.4 + 时序图与核心业务流程设计.md + 总体架构.md + MVP 节点规划.md + Go 项目结构.md / iOS 工程结构.md 是契约**输入**，本 story 严格对齐它们但**不修改**；V1 §5.2 line 532-541 / §12.3 line 2212-2259 已显式 frozen 14.1，本 story 不需修改任何 V1 文档）
    - 任何 ADR（ADR-0006 错误码三层映射 / ADR-0007 ctx 传播 / ADR-0011 ws stack 是契约**输入**，沿用不修改；本 story **不**新建 ADR —— "fire-and-forget broadcast + envelope helper + router wire" 是 11.8 已落地决策，本 story 沿用）
    - V1 接口契约（14.1 已冻结 §5.2 + §12.3 `### 宠物状态变更`）
    - migrations 0003 / 0006 / 0007 / 0008（pets / users / rooms / room_members 表已就绪；本 story **仅消费** schema，**不**改 SQL）
    - GORM `Pet` / `User` / `Room` / `RoomMember` struct 字段定义（既有不动；本 story 仅调既有 PetRepo.FindDefaultByUserID + UserRepo.FindByID + 新增 ws envelope helper，**不**加 struct 字段）
    - PetRepo interface（pet_repo.go:48-83）已含 `FindDefaultByUserID` + `UpdateCurrentStateByID`（14.2 落地）；本 story **不加**任何 PetRepo 方法
    - UserRepo interface（user_repo.go:68-80）已含 `FindByID` + `UpdateCurrentRoomID`（4.6 / 11.3 落地）；本 story **不加**任何 UserRepo 方法
    - 14.2 已 done 的 `SyncCurrentState` 核心逻辑（err 二分 / pet-less noop / 参数校验 / UPDATE 调用 / response 回显入参等）；本 story **仅替换** 14.2 留的 `// TODO(Story 14.4)` 注释段 + 三行 `_ = ...` 防警告语句为真实 broadcast 触发，**不动**前置逻辑
    - 14.2 已 done 的 `PetsHandler.PostStateSync` 全部代码（handler 层不需要改）
    - 14.2 已 done 的 `PetRepo.UpdateCurrentStateByID` 全部代码（repo 层不需要改）
    - 14.3 已 done 的 `RoomSnapshotBuilder` / `GetRoomDetail` / `broadcastMemberJoined` 三处真值切换（本 story 不动 14.3 三处实装）
    - 11.8 已 done 的 `roomServiceImpl.broadcastMemberJoined` / `broadcastMemberLeft` / `enqueueRoomEvent` / `runPostCommitAsyncPerRoom` 等（本 story **不复用**这些方法 —— pet 广播独立路径，但实装风格对齐）
    - WS gateway / Session / SessionManager / SnapshotBuilder（10.x / 11.x 已稳定；本 story 仅通过 router.go wire 的 deps.SessionMgr 注入到 petServiceImpl，**不**改 ws 包内部任何结构 / 不动 BroadcastToRoom / BroadcastToRoomExcept primitive）
    - `room_service.go` / `room_handler.go` / `room_member_repo.go`（11.x / 14.3 已稳定，本 story 不动）
    - `home_service.go` / `home_handler.go`（4.8 / 5.x / 11.10 已稳定，本 story 不动）
    - `auth_service.go` / `auth_handler.go`（4.6 已稳定 + Auth 中间件 4.5 已稳定，本 story 不动）
    - `step_service.go` / `dev_step_service.go` / `step_handler.go`（7.3 / 7.4 / 7.5 已稳定，本 story 不动）
    - 其他 epic 范围的 service / handler / repo（4.6 / 4.8 / 7.x / 11.x / Epic 5+ iOS 全部不动）
    - `_bmad-output/` 下其他 yaml / md（除自己的 story 文件 + sprint-status.yaml 流转 + 可能的新 lesson md）
  - **不**实装：
    - **iOS Story 15.2 pet.state.changed WS 消息处理**：iOS 端 RoomViewModel 接收 / merge / UI 驱动等都归属 iOS Epic 15
    - **iOS Story 15.4 self-broadcast 对称兜底规则**：iOS 端 HTTP 200 vs self-broadcast 任一先到驱动 UI 的对称逻辑都归属 iOS Story 15.4
    - **iOS Story 15.5 跨房间状态恢复**：iOS 端 reconnect 后 room.snapshot 全量重新对齐 + 真实 currentState 渲染都归属 iOS Story 15.5
    - **Epic 16 节点 5 跨端集成 e2e + demo 验收 + tech debt 登记**：14.x + iOS Epic 15 全部完成后由 Epic 16 收口
    - **`pet.equips.changed` WS 消息**：装备变更广播是另一独立路径（如 Epic 27 / 30 才落地），本 story payload **严格**只 `{userId, petId, currentState}` 三字段（与 V1 §12.3 行 2257 钦定 future fields 注一致）
    - **`/pets/equip` / `/pets/unequip` / 装备相关接口**：装扮 / 仓库 / 合成业务都归属 Epic 23+
    - **GORM AutoMigrate**：禁用（与 ADR-0003 §3.2 同源；本 story 不动 GORM struct 字段）
    - **rate limit 特殊化 broadcast 路径**：本 story 不引入 broadcast 端的限频（fire-and-forget 不做去重，与 V1 §12.3 line 2253 "同一秒多次 state-sync 每次都广播" 一致）
    - **WS 消息 retry / 持久化**：本 story 严格 fire-and-forget，**不**实装 retry / 持久化 / DLQ；broadcast 失败仅 log warn
    - **跨实例 WS 广播**（多实例部署 + Pub/Sub）：节点 5 阶段 ws gateway 单实例（与 10.5 同源），本 story 落地的 BroadcastToRoom 仅覆盖单实例 fanout；多实例归属后续节点 tech debt（如节点 11+）
    - **跨 epic 接口扩展**（如 GET /home / RoomSnapshotBuilder / 表情接口 / 步数接口的扩展）—— 全部归属未来 story，本 story 严格红线

## Acceptance Criteria

**AC1 — `BuildPetStateChangedEnvelope` + `PetStateChangedPayload` 在 `server/internal/app/ws/snapshot.go` 新增**

修改 `server/internal/app/ws/snapshot.go`，在既有 `BuildMemberLeftEnvelope` 之后（行 495 后）新增：

```go
// PetStateChangedPayload 是 pet.state.changed 消息的 payload（Story 14.4 引入）。
//
// 与 V1 §12.3 `### 宠物状态变更（pet.state.changed）` 字段表完全 1:1 对齐
// （V1 line 2223-2230 字段表）：
//   - UserID:       BIGINT 字符串化（V1 §2.5 全局约定）；状态变更的 user 主键，
//     来自 POST /pets/current/state-sync 当前 user.id
//   - PetID:        BIGINT 字符串化（V1 §2.5 全局约定）；状态变更的 pet 主键，
//     来自 service 层 FindDefaultByUserID 查到的 pets.id
//   - CurrentState: number (int) 必填；变更后宠物当前状态枚举（1=rest / 2=walk
//     / 3=run，与数据库 §6.4 pets.current_state 同义；与 §10.3 / §12.3 room.snapshot
//     / §12.3 member.joined 同语义；与 §5.2 request state 等价 —— 都是入参回显）
//
// **payload 字段集合严格只 3 字段**（V1 §12.3 行 2257 future fields 注 +
// 本 story 范围红线钦定）：不含 nickname / avatarUrl / equips / equips[].renderConfig
// 等任何其他字段；装备变更广播由独立路径（Epic 27 / 30 等）触发，**不**扩展本 payload。
//
// **关键约束**（V1 §12.3 line 2250 钦定）：3 字段都必填（**禁止** payload 为 `{}`
// 或缺任一字段）；缺字段视为契约违反，client 解析层走"安全忽略 + log warn"路径。
// Go struct 层不显式 omitempty（与 SnapshotMember / MemberJoinedPayload 同模式），
// 所有字段一律 JSON marshal 输出。
type PetStateChangedPayload struct {
	UserID       string `json:"userId"`
	PetID        string `json:"petId"`
	CurrentState int    `json:"currentState"`
}

// BuildPetStateChangedEnvelope wrap PetStateChangedPayload 进 serverEnvelope +
// json.Marshal 返 ([]byte, error)（Story 14.4 引入；与 BuildMemberJoinedEnvelope
// / BuildMemberLeftEnvelope 同模式）。
//
// 用途：service 层 PetService.SyncCurrentState 在 UPDATE pets.current_state
// 成功后调用本 helper 拿到 []byte 后调 BroadcastFn 推送给该房间内所有在线
// Session（**包含**发起者自己 —— 与 member.joined / member.left 排除发起者不同
// 语义，详见 V1 §12.3 line 2249 "广播范围"段）；隐藏 ws 包内部 serverEnvelope
// struct，让 service 层只 import payload 类型 + helper 函数。
//
// envelope 字段值（V1 §12.3 通用信封 + 行 2225-2230 钦定）：
//   - Type:      "pet.state.changed"
//   - RequestID: ""（主动推送类消息固定 ""）
//   - Payload:   入参 payload
//   - Ts:        time.Now().UnixMilli()（服务端发送时间戳 ms；与 member.joined /
//     member.left ts 字段语义一致 —— 仅作日志关联 + UI 辅助展示，**禁止**用作业务
//     排序 / 状态新旧判定，V1 §12.3 line 2255 + line 1961 钦定）
//
// 错误：json.Marshal 在 marshalable struct 下不可能失败；防御性 wrap（与
// SendRoomSnapshot / BuildMemberJoinedEnvelope 同模式）。caller 收到 error 时
// log warn 不重试（与 broadcast 失败同 fire-and-forget 语义，V1 §12.3 line 2254）。
func BuildPetStateChangedEnvelope(payload PetStateChangedPayload) ([]byte, error) {
	env := serverEnvelope{
		Type:      "pet.state.changed",
		RequestID: "", // V1 §12.3 主动推送类消息固定 ""
		Payload:   payload,
		Ts:        time.Now().UnixMilli(),
	}
	bytes, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("ws envelope: marshal pet.state.changed: %w", err)
	}
	return bytes, nil
}
```

**关键决策**：
- 沿用 `serverEnvelope` 内部 struct（snapshot.go 包内既有）—— **不**新建 PetStateChangedEnvelope 顶层 struct（YAGNI；envelope 字段统一 type/requestId/payload/ts，业务消息差异**仅**在 Type 字符串 + Payload 类型，与 `member.joined` / `member.left` 完全同模式）
- 沿用 `time.Now().UnixMilli()` 取 ts —— 与既有 helpers 一致；**不**新建 clock interface 给本 helper 用（YAGNI；helper 内部时间是 wire 字段，单元测试通过 unmarshal envelope 后比对 ts 字段 != 0 验证，**不**比对具体值；详见 AC1 单测 case 设计）
- **不**导出 `serverEnvelope` struct（保留 unexported）—— 与既有同模式

**AC2 — `petServiceImpl.broadcastPetStateChanged` 私有方法新增 + `SyncCurrentState` 替换 TODO 占位**

修改 `server/internal/service/pet_service.go`：

**(2a) 新增 `broadcastPetStateChanged` 私有方法**（紧接 `SyncCurrentState` 之后）：

```go
// broadcastPetStateChanged 触发 pet.state.changed WS 广播（Story 14.4 引入）。
//
// 流程（V1 §5.2 服务端逻辑步骤 5 + §12.3 `### 宠物状态变更` 字段表钦定）：
//  1. userRepo.FindByID(ctx, userID) 拿 *User.CurrentRoomID *uint64：
//     - err != nil → log warn + return（不广播，fire-and-forget；与 11.8
//       broadcastMemberJoined 同模式 —— DB 异常不阻塞 HTTP 200，不重试）
//     - CurrentRoomID == nil → return（用户不在任何房间，不广播；V1 §5.2 line 540
//       + §12.3 line 2218 钦定 null → 不广播路径，**不** log warn —— 这是合法
//       业务路径而非异常）
//     - CurrentRoomID != nil → 进步骤 2
//  2. 构造 ws.PetStateChangedPayload{UserID: 字符串化, PetID: 字符串化,
//     CurrentState: int(state)}（BIGINT 字符串化遵循 V1 §2.5）
//  3. 调 ws.BuildPetStateChangedEnvelope(payload) 拿 marshal 后 []byte：
//     - err != nil → log warn + return（fire-and-forget）
//  4. s.broadcastFn(ctx, *user.CurrentRoomID, msgBytes) 推送（**包含**发起者自己 ——
//     与 11.8 broadcastMemberJoined 用 broadcastExceptFn 排除发起者**不同**语义，
//     详见 V1 §12.3 line 2249 关键约束段；service 层调 broadcastFn 而非
//     broadcastExceptFn 是契约钦定的设计选择）
//     - err != nil → log warn（fire-and-forget，**不**返 error）
//
// **fire-and-forget 严格语义**（V1 §5.2 line 539 + §12.3 line 2217 / line 2254
// 钦定 + 与 11.8 broadcastMemberJoined 同模式）：本方法**永远不返 error** ——
// 任何步骤失败（FindByID / marshal / broadcast）一律 log warn 不返；caller
// (SyncCurrentState) 不需要走错误分流。原因：broadcast 失败不应影响 HTTP 200
// 响应（client 已通过 HTTP 拿到 server-acknowledged ack 信号，broadcast 是事件
// 通知，不参与 ack 原子性）。
//
// **不**回滚 DB UPDATE（V1 §5.2 line 539 钦定）：本方法在 UPDATE 成功之后调用，
// 任何 broadcast 失败**不**回滚 pets.current_state 写入 —— DB 真实状态以 server
// 为准（与 CLAUDE.md §"工作纪律 / 状态以 server 为准"一致），broadcast 是事件
// 通知层职责。
//
// **广播范围包含发起者自己**（V1 §12.3 line 2249 关键约束 + §1 line 55 节点 5
// 冻结声明）：service 层调 broadcastFn 全 fanout，client 端自己识别 payload.userId
// == self 走 §5.2 self-broadcast 对称兜底规则（V1 line 547-551，归属 iOS Story 15.x
// 实装）；server 层**不**为 self vs 他人做差异化 fanout。
//
// **detached ctx + timeout**（与 11.8 enqueueRoomEvent 同模式）：caller
// SyncCurrentState 用 `context.WithoutCancel(ctx)` + `context.WithTimeout(detached,
// petBroadcastTimeout)` 构造 detached ctx 避免 client cancel 误中断 broadcast 路径；
// 本方法接受该 ctx 透传到 FindByID / broadcastFn 即可。详见 SyncCurrentState 调用
// 点注释。
//
// **goroutine launch**：caller SyncCurrentState 用 `go s.broadcastPetStateChanged(...)`
// 异步触发；本方法体**不**自己启 goroutine（caller 决定启动时机，与 11.8
// enqueueRoomEvent 同模式 —— enqueue 是 caller 同步段动作，worker 异步执行）。
//
// **nil broadcastFn / nil sessionMgr guard**：本方法直接调 s.broadcastFn(...) ——
// 如 broadcastFn 为 nil（路由 wire 阶段 deps.SessionMgr nil → petBroadcastFn 直接
// 返 (0, nil) no-op）则 s.broadcastFn 本身仍非 nil（closure 函数值非 nil），可正常
// 调用走 no-op 路径；**不**需要在 broadcastPetStateChanged 内额外加 `if s.broadcastFn
// == nil` 守卫（防御失败设计：让 caller / router.go wire 阶段保证 broadcastFn 非 nil
// closure）。如 router.go 拒绝 wire（如 GormDB 全部 nil）则 NewPetService 也不被调用，
// petServiceImpl 不存在，本方法自然不会被触发。
//
// **不**调用 s.sessionMgr（保留字段为防御性预留 / future expansion 用）：本 story
// 实装严格通过 broadcastFn 走 fanout 路径，**不**需要直接调 SessionManager 接口；
// 与 11.8 roomServiceImpl 同模式 —— roomServiceImpl 的 sessionMgr 字段用于
// LeaveRoom 路径 close leaver Session，pet 广播路径不需要操作 Session lifecycle。
func (s *petServiceImpl) broadcastPetStateChanged(
	ctx context.Context,
	roomID, userID, petID uint64,
	state int8,
) {
	logger := slog.Default().With(
		slog.String("component", "pet-service-broadcast"),
		slog.String("event", "pet.state.changed"),
		slog.Uint64("roomId", roomID),
		slog.Uint64("userId", userID),
		slog.Uint64("petId", petID),
		slog.Int("state", int(state)),
	)

	// (1) 构造 payload
	payload := ws.PetStateChangedPayload{
		UserID:       strconv.FormatUint(userID, 10),
		PetID:        strconv.FormatUint(petID, 10),
		CurrentState: int(state),
	}

	// (2) marshal envelope
	msgBytes, err := ws.BuildPetStateChangedEnvelope(payload)
	if err != nil {
		logger.Warn("ws broadcast: marshal envelope failed; skip broadcast",
			slog.Any("error", err))
		return
	}

	// (3) fire-and-forget broadcast；用 broadcastFn 全 fanout（**包含**发起者自己 ——
	// 与 11.8 broadcastMemberJoined 用 broadcastExceptFn 排除发起者**不同**语义，
	// V1 §12.3 line 2249 钦定的设计选择）。
	sent, err := s.broadcastFn(ctx, roomID, msgBytes)
	if err != nil {
		logger.Warn("ws broadcast: broadcastFn failed",
			slog.Int("targetSessions", sent),
			slog.Any("error", err))
		return
	}
	logger.Info("ws broadcast: pet.state.changed sent",
		slog.Int("targetSessions", sent))
}
```

**(2b) `SyncCurrentState` UPDATE 成功路径替换 `// TODO(Story 14.4)` 注释段为真实 broadcast 触发**（pet_service.go:166-194 段替换）：

替换 14.2 落地的"happy 路径 info log + TODO 占位 + `_ = s.sessionMgr` 三行防警告语句"为：

```go
	// happy 路径 info log（业务事件可观测性，与 step_service / room_service 同模式）
	slog.InfoContext(ctx, "pet state-sync succeeded",
		slog.Uint64("userId", in.UserID),
		slog.Uint64("petId", pet.ID),
		slog.Int("state", int(in.State)))

	// (3) Story 14.4: 触发 pet.state.changed WS 广播
	//
	// 流程（V1 §5.2 服务端逻辑步骤 5 + §12.3 `### 宠物状态变更` 字段表钦定）：
	//   - userRepo.FindByID 拿 user.CurrentRoomID *uint64
	//   - CurrentRoomID == nil → 用户不在房间，**不**广播（HTTP 仍 200 OK）
	//   - CurrentRoomID != nil → BroadcastToRoom 广播给该房间所有在线 Session
	//     （**包含**发起者自己，与 member.joined / member.left 排除发起者**不同**语义）
	//
	// **fire-and-forget 严格语义**（V1 §5.2 line 539 + §12.3 line 2217 / line 2254
	// + 11.8 broadcastMemberJoined 同模式）：
	//   - broadcast 失败 / FindByID 失败 / marshal 失败一律 log warn，**不**返 error
	//   - **不**回滚 pets.current_state UPDATE（DB 真实状态以 server 为准）
	//   - **不**影响 HTTP 200 响应（client 已通过 HTTP 拿到 server-acknowledged ack 信号）
	//
	// **detached ctx + goroutine**（与 11.8 enqueueRoomEvent 同模式）：
	//   - `context.WithoutCancel(ctx)` 解除 request ctx cancel 信号 —— 让 broadcast
	//     不被 client 主动断开 / handler deadline 触发的 cancel 误中断（否则
	//     userRepo.FindByID 会 fail "context canceled" → broadcast 静默 skip）
	//   - `context.WithTimeout(detached, petBroadcastTimeout)` 加 10s 超时兜底 ——
	//     防 goroutine 泄漏（DB 卡死 / SessionManager 死锁 → goroutine 永不返回）
	//   - `go s.broadcastPetStateChanged(...)` 启 goroutine 异步执行 —— 不阻塞
	//     HTTP 200 响应（user.FindByID 是事务外查询，<10ms 级，但仍保留异步语义
	//     与 11.8 同模式）
	//
	// **先查 user 而后启 goroutine 的考虑**：本路径 user.FindByID 是事务外、**事务后**
	// 调用 —— 14.2 落地的 SyncCurrentState 不开事务（单 SELECT + 单 UPDATE
	// autocommit），UPDATE commit 后直接走 user lookup 路径；user.FindByID 是同步
	// 查询（普通连接池），返回后**才**启 goroutine 启动 broadcast（让 user.CurrentRoomID
	// nil 路径不浪费 goroutine 启动开销，让"用户不在房间"的高频路径走轻路径）。
	//
	// **如 user 查询失败**：log warn 后**不**广播（fire-and-forget；与 11.8 同模式 ——
	// load joiner user failed → skip broadcast）；**不**影响 HTTP 200 响应。
	user, err := s.userRepo.FindByID(ctx, in.UserID)
	if err != nil {
		slog.WarnContext(ctx, "pet state-sync: load user for broadcast failed; skip broadcast",
			slog.Uint64("userId", in.UserID),
			slog.Any("error", err))
		// 不返 error，继续走 ack 返回路径
	} else if user.CurrentRoomID != nil {
		// 用户在房间 → 启 goroutine 异步广播（detached ctx + timeout 防 goroutine 泄漏）
		roomID := *user.CurrentRoomID // 解引用前已 nil-check
		detached := context.WithoutCancel(ctx)
		timedCtx, cancel := context.WithTimeout(detached, petBroadcastTimeout)
		go func() {
			defer cancel()
			s.broadcastPetStateChanged(timedCtx, roomID, in.UserID, pet.ID, in.State)
		}()
	}
	// user.CurrentRoomID == nil（用户不在任何房间）→ 不启 goroutine，不广播
	// （V1 §5.2 line 540 + §12.3 line 2218 钦定 null → 不广播路径，**不** log warn ——
	// 合法业务路径而非异常）

	// (4) 返回 ack 信号（回显入参）
	return &SyncCurrentStateOutput{State: in.State}, nil
```

**(2c) 包级常量新增 `petBroadcastTimeout`**（紧接 `package service` doc + import 之后，与既有 `room_service.go:31` `postCommitTimeout` 同模式）：

```go
// petBroadcastTimeout 是 pet.state.changed broadcast goroutine 的超时上限
// （Story 14.4 引入；与 room_service.go postCommitTimeout 同模式）。
//
// **为何需要超时**：detached ctx (context.WithoutCancel) 解除 request ctx
// cancel 信号 —— 这是为了让 broadcast 不被 client 主动断开 / handler deadline
// 触发的 cancel 误中断（否则 userRepo.FindByID 会 fail "context canceled" →
// broadcast 静默 skip）。但完全 detached 会引入 goroutine 泄漏风险（DB 卡死 /
// SessionManager 死锁 → goroutine 永不返回）。所以**必须**给 detached ctx 加
// 独立 timeout 兜底。
//
// **10s 选型**（与 room_service.go postCommitTimeout 一致）：pet 广播全部 work
// （user lookup + 1 次 marshal + broadcastFn fanout）总时间上界 ~3s；取 10s
// 留冗余允许 worst-case write loop 排队。Future 节点如有 SessionManager 性能
// 压测可调小到 5s（与 room broadcast 同步调整）。
const petBroadcastTimeout = 10 * time.Second
```

**(2d) `PetService` interface doc comment + `petServiceImpl` struct doc comment + `NewPetService` doc comment 同步更新**：

- `PetService` interface doc comment：移除"WS 广播实装（14.4 才落地，service struct 已预留 sessionMgr / broadcastFn 字段）"措辞，改为"WS 广播实装：自 Story 14.4 起在 SyncCurrentState UPDATE 成功路径触发 pet.state.changed WS 广播给同房间全员（包含发起者自己）"
- `petServiceImpl` struct 字段 doc comment：移除"14.4 预留字段（本 story 不使用，**禁止**在 method body 调用）"措辞，改为"自 Story 14.4 起 sessionMgr / broadcastFn / userRepo 三字段全部进入业务路径 —— userRepo 用于 SyncCurrentState 末尾查 users.current_room_id；broadcastFn 用于触发 pet.state.changed 广播；sessionMgr 当前未直接调用（防御性预留，与 11.8 roomServiceImpl 同模式）"
- `NewPetService` doc comment：移除"sessionMgr / broadcastFn 字段是 14.4 预留：本 story 调用方（router.go wire + 测试场景）传 nil 即可"措辞，改为"sessionMgr / broadcastFn 字段：router.go wire 时传 deps.SessionMgr + petBroadcastFn closure（与 11.8 NewRoomService 同模式）；测试场景可注入 mock / stub 验证 broadcast 路径"

**AC3 — `PetService` 单测（≥4 新增 case，mocked PetRepo / UserRepo / BroadcastFn / SessionManager）**

修改 `server/internal/service/pet_service_test.go`：

**(3a) 既有 case 同步更新**：
- **case 1 happy state=2** + **case 2 pet-less noop** + **case 3 DB 异常 FindDefaultByUserID** + **case 4 DB 异常 UpdateCurrentStateByID** + **case 5 幂等同 state 重复上报**：mock UserRepo.FindByID 返 `&User{CurrentRoomID: nil}`（用户不在房间）或不 mock（case 2 / 3 / 4 walk 不到 user lookup 阶段）让既有 happy / failure 断言不破；mock BroadcastFn 注入但断言"调用次数 0"或按 case 期望
- **case 6 广播路径未被触发的 wire 占位**：14.2 落地的占位 case **删除**（自 14.4 起 broadcastFn 在 happy 路径会被触发，占位 case 语义反转）；改为本 story AC3 新增 case 之一

**(3b) 新增 ≥4 case 覆盖 broadcast 路径**：

- **case 7 — happy 用户在房间 → broadcastFn 调用 1 次 + payload 字段正确**：
  - mock petRepo.FindDefaultByUserID 返 `&Pet{ID: 100, ...}`
  - mock petRepo.UpdateCurrentStateByID 返 nil
  - mock userRepo.FindByID 返 `&User{ID: 10, CurrentRoomID: ptrUint64(500)}`
  - mock broadcastFn 是 `func(ctx, roomID, msg) (int, error) { capturedCalls = append(capturedCalls, ...); return 1, nil }`
  - 调 SyncCurrentState({UserID: 10, State: 2}) → 验证返回 `&SyncCurrentStateOutput{State: 2}` + nil error
  - **等待 broadcast goroutine 完成**：用 `sync.WaitGroup` 或 `time.Sleep(50ms)` 让 goroutine 跑完（建议用 `s.broadcastFn` 包一层 `mockBroadcastFn` 内部 `wg.Done()` + 单测主线程 `wg.Wait()` —— 与 room_service_test.go broadcastMemberJoined 测试同模式）
  - 验证 broadcastFn 调用次数 1 + capturedCalls[0].roomID == 500
  - **unmarshal msg bytes 验证 envelope 完整字段**：
    - envelope.type == "pet.state.changed"
    - envelope.requestId == ""
    - envelope.payload.userId == "10"
    - envelope.payload.petId == "100"
    - envelope.payload.currentState == 2
    - envelope.ts != 0
- **case 8 — 用户不在房间（CurrentRoomID nil）→ broadcastFn 调用 0 次**：
  - mock userRepo.FindByID 返 `&User{ID: 10, CurrentRoomID: nil}`
  - 其他与 case 7 同
  - 验证 SyncCurrentState 返回 happy + broadcastFn 调用次数 0 + **不**启 goroutine（用 wg.Wait timeout 1s 不阻塞验证 —— 实际上由于 user.CurrentRoomID == nil 分支不启 goroutine，wg 永远没 Add，Wait 立即返回）
- **case 9 — UpdateCurrentStateByID err → broadcastFn 调用 0 次（UPDATE 失败路径不进 broadcast）**：
  - mock petRepo.UpdateCurrentStateByID 返 `errors.New("deadlock")`
  - 验证 SyncCurrentState 返回 nil output + apperror.ErrServiceBusy + broadcastFn 调用 0 次（UPDATE 失败路径在 broadcast 触发之前 return error）
- **case 10 — broadcastFn 自身 return error → SyncCurrentState 仍返 HTTP 200 ack（fire-and-forget 严格语义）**：
  - mock broadcastFn 返 `0, errors.New("session manager dead")`
  - mock userRepo.FindByID 返 happy user with CurrentRoomID
  - 验证 SyncCurrentState 返回 `&SyncCurrentStateOutput{State: 2}` + nil error（**禁止** broadcast 失败影响 HTTP 200 ack）+ broadcastFn 调用 1 次
- **case 11 — FindByID err → log warn + 不 broadcast + 不影响 HTTP 200**：
  - mock userRepo.FindByID 返 `nil, errors.New("connection refused")`
  - 验证 SyncCurrentState 返回 happy + broadcastFn 调用 0 次 + **不** wg Add（FindByID 失败路径不启 goroutine —— 因为 FindByID 是事务后**同步**调用，失败后**不**进 if user.CurrentRoomID != nil 分支，自然不启 goroutine）
- **case 12（可选 ≥4 要求外）— 同一秒多次 state-sync 每次都广播**（V1 §12.3 line 2253 钦定）：
  - 调 SyncCurrentState 3 次（同 user 同 state=2）
  - 验证 broadcastFn 调用次数 3（每次都广播，不去重）

**所有 case 必须**：
- 用自定义 stub / mock 实装 BroadcastFn type alias（与 room_service_test.go broadcastFn 注入同模式；**优先 stub closure** 而非 testify/mock）
- 用 sync.WaitGroup 同步 broadcast goroutine 完成 —— 单测主线程在 broadcast 后调 wg.Wait() 等 goroutine 跑完再做 capturedCalls 断言（防 race condition 误判）
- 显式断言"fire-and-forget 不影响 HTTP 200" 语义（case 10 / case 11；通过 SyncCurrentState 返 happy + ack 字段值正确间接保证）
- 显式断言"UPDATE 失败 / pet-less 不触发 broadcast"语义（case 9 + 既有 case 2 pet-less）—— 单测 mock broadcastFn 调用次数为 0

**AC4 — `BuildPetStateChangedEnvelope` 单测（≥2 case）**

修改 `server/internal/app/ws/snapshot_test.go`，新增：

- **case 1 — happy envelope 字段全部正确**：
  - 调 BuildPetStateChangedEnvelope(PetStateChangedPayload{UserID: "10", PetID: "100", CurrentState: 2})
  - 验证 ([]byte, nil) 返回
  - json.Unmarshal 后验证：
    - envelope.type == "pet.state.changed"
    - envelope.requestId == ""
    - envelope.payload == {userId: "10", petId: "100", currentState: 2}
    - envelope.ts != 0 + envelope.ts > 当前时间戳 - 1s（合理范围）
- **case 2 — currentState=1/3 边界值都正确序列化**（覆盖 enum 全范围）：
  - 验证 currentState=1 → JSON 内 `"currentState":1`
  - 验证 currentState=3 → JSON 内 `"currentState":3`

**AC5 — `PetService` 集成测试（dockertest + WS broadcastFn captured）**

修改 `server/internal/service/pet_service_integration_test.go`，新增 1 case：

- **场景 5 — ws-end-to-end broadcastFn 被触发**：
  - 用 dockertest 启 MySQL container + 跑 migrations（既有 fixture 复用）
  - 创建 user A + 默认 pet（pet.current_state=1）+ room X + insert room_members 行 + UPDATE users.current_room_id = X
  - 构造 captured-call mockBroadcastFn（与 case 7 单测模式同）
  - 构造 PetService 注入 真实 PetRepo / UserRepo + mock BroadcastFn + nil sessionMgr（service 层不直接调用 sessionMgr）
  - 调 PetService.SyncCurrentState(ctx, {UserID: A, State: 2})
  - 等待 broadcast goroutine 完成
  - 验证：
    - SyncCurrentStateOutput.State == 2
    - DB pets.current_state = 2（既有 14.2 集成测试同模式断言）
    - broadcastFn 调用次数 1 + roomID == X
    - unmarshal msg bytes 后 envelope.type == "pet.state.changed" + payload.userId == "<A.id>" + payload.petId == "<A.pet.id>" + payload.currentState == 2 + ts != 0

**fixture 复用约束同 14.2**：
- 必须**复用** server/internal/repo/mysql/ 既有 dockertest 启动 helper；**禁止**新建独立 dockertest 启动函数
- 必须**跳过 short**：`if testing.Short() { t.Skip("skip integration test in short mode") }`

**AC6 — `PetsHandler` 集成测试 + WS end-to-end（dockertest + 真实 ws gateway + httptest gorilla/websocket dial）**

修改 `server/internal/app/http/handler/pets_handler_integration_test.go`，新增 1 case：

- **场景 5 — POST state-sync 触发 pet.state.changed 广播给发起者自己（端到端）**：
  - dockertest 启 MySQL + 跑 migrations
  - 启 ws gateway（与既有 ws_integration_test.go 同模式）
  - 创建 user A + 默认 pet + room X + A join X
  - 构造完整 router 含 ws routes + http routes + 14.4 wired petBroadcastFn closure（即 deps.SessionMgr 非 nil 走真实 BroadcastToRoom）
  - 用 jwtSigner 生成 A 的合法 Bearer token
  - **httptest dial WS**：用 gorilla/websocket Dialer 连 `ws://<server>/api/v1/ws/rooms/{roomID}` 带 token → 完成握手 → 跳过 room.snapshot 初始消息（先收掉防干扰）
  - **POST state-sync**：用 http.Client POST `/api/v1/pets/current/state-sync` `{state: 2}` 带 Authorization Bearer token
  - 验证 HTTP 200 + envelope.data.state == 2 + DB pets.current_state == 2
  - **从 WS conn 读下一条消息**（含 5s timeout 兜底）：unmarshal envelope 后验证：
    - envelope.type == "pet.state.changed"
    - envelope.requestId == ""
    - envelope.payload.userId == "<A.id>"
    - envelope.payload.petId == "<A.pet.id>"
    - envelope.payload.currentState == 2
    - envelope.ts != 0
  - **关键断言：A 自己收到自己的 pet.state.changed**（V1 §12.3 line 2249 "广播范围含发起者自己" 钦定的具体落地证明）

**fixture 复用约束**：
- 复用 server/internal/app/ws/ws_integration_test.go 既有 ws server 启动 helper / gorilla/websocket dial 代码
- 复用 router_test.go bootstrap.Deps 全 wire 路径（含 deps.SessionMgr 真实实例）
- 跳过 short：`if testing.Short() { t.Skip("skip integration test in short mode") }`

**AC7 — bootstrap router wire 替换**

修改 `server/internal/app/bootstrap/router.go`：

**(7a) 新增 `petBroadcastFn` closure**（紧接 `roomBroadcastExceptFn` closure 定义之后，行 401 后）：

```go
		// Story 14.4 加：pet.state.changed 广播 closure（与 roomBroadcastFn 同模式
		// nil-tolerant + 同样调 wsapp.BroadcastToRoom）。
		//
		// **关键差异**（与 roomBroadcastFn 同实现，但语义独立）：
		//   - roomBroadcastFn / roomBroadcastExceptFn 用于 member.joined / member.left
		//     广播路径（11.8）—— member 事件的"广播范围排除发起者自己"语义由 service
		//     层显式调 broadcastExceptFn 实现
		//   - petBroadcastFn 用于 pet.state.changed 广播路径（本 story）—— pet 事件的
		//     "广播范围包含发起者自己"语义（V1 §12.3 line 2249 钦定）由 service 层
		//     直接调 broadcastFn 无需 except 路径
		//
		// **不**复用 roomBroadcastFn closure：14.2 范围红线钦定"逻辑独立，本 story 直接
		// 传 nil 让 14.4 灵活决定"；本 story 决定**独立**新建 closure 而非复用 ——
		// 让 broadcast 语义边界清晰（"pet 广播 vs room 广播"在 router wire 层就分离）+
		// future 任一路径需要差异化时（如本路径未来需要 metric / log 前缀差异）不影响另一路径。
		//
		// 实装与 roomBroadcastFn 完全一致（nil-tolerant + wsapp.BroadcastToRoom 直调）。
		petBroadcastFn := wsapp.BroadcastFn(func(ctx context.Context, roomID uint64, msg []byte) (int, error) {
			if deps.SessionMgr == nil {
				return 0, nil
			}
			return wsapp.BroadcastToRoom(ctx, deps.SessionMgr, roomID, msg)
		})
```

**(7b) 替换 `petSvc` 构造参数**（行 415）：

```go
		// Story 14.2 加：pet service + handler（POST /pets/current/state-sync；
		// 单 UPDATE 不开事务）。复用上面构造的 petRepo / userRepo 实例。
		//
		// **sessionMgr / broadcastFn 自 Story 14.4 起注入真实实例**：14.2 阶段
		// service struct 字段 sessionMgr / broadcastFn 预留 nil 注入；14.4 落地后
		// 替换为 deps.SessionMgr + petBroadcastFn closure，让 SyncCurrentState UPDATE
		// 成功路径触发 pet.state.changed 广播给同房间全员（含发起者自己，V1 §12.3
		// line 2249 钦定）。
		petSvc := service.NewPetService(petRepo, userRepo, deps.SessionMgr, petBroadcastFn)
		petsHandler := handler.NewPetsHandler(petSvc)
```

**注**：`deps.SessionMgr` 可能为 nil（HTTP-only 部署 / 测试 fixture）；service 层 broadcastPetStateChanged 直接调 broadcastFn 不需要 sessionMgr，sessionMgr 字段是"防御性预留"（参见 AC2 doc comment）。petBroadcastFn closure 内部 `if deps.SessionMgr == nil { return 0, nil }` no-op 兜底，单测 / HTTP-only 部署不会 panic。

**AC8 — Router test 扩展**

修改 `server/internal/app/bootstrap/router_test.go`：

- 既有 router_test.go fixture 应该自动覆盖 `POST /pets/current/state-sync` 路由（14.2 已扩展），本 story 仅改 service wire 参数，路由 path 不变 —— 既有 case 应自动适配
- 如发现既有 router_test.go 缺少"完整 deps wire → 列举所有 authed 路由 path"检查，本 story **不**新增此类断言（与 11.8 / 14.2 同模式 —— 路由清单不属本 story 范围）
- 如新增 router_test.go 任何 case，必须保持既有断言不破

**AC9 — 业务事件 log + 可观测性**

`broadcastPetStateChanged` 必须按 slog 业务事件 log 规范产日志（与 room_service.broadcastMemberJoined / broadcastMemberLeft 同模式）：

- **happy 路径**（broadcast 发送成功）：service info "ws broadcast: pet.state.changed sent" + 字段 component / event / roomId / userId / petId / state / targetSessions
- **fire-and-forget 失败**（marshal / broadcast）：service warn "ws broadcast: marshal envelope failed" / "ws broadcast: broadcastFn failed" + 字段同上 + error
- **FindByID 失败**（user lookup）：service warn "pet state-sync: load user for broadcast failed; skip broadcast" + 字段 userId / error（log 在 SyncCurrentState 主路径而非 broadcastPetStateChanged，因为 user lookup 在主路径同步执行）
- **不**新增任何 prometheus metric / counter（节点 5 阶段未新增可观测性需求；与 11.8 同模式）

**AC10 — `bash scripts/build.sh --test` 全绿**

完成本 story 全部代码改动后必须：

- 跑 `bash scripts/build.sh --test`（go vet + go build + 全量单元测试 `go test -count=1 ./...`）
- 全部 PASS（0 错误 + 0 warnings）+ 所有新增单测命中预期断言 + 既有测试不破
- 跑 `bash scripts/build.sh --integration`（`-tags=integration` 仅跑集成测试）覆盖本 story 新增的 pet_service_integration_test.go + pets_handler_integration_test.go + snapshot_test.go
- 跑 `bash scripts/build.sh --race --test`（go test -race -count=1 ./...）—— race detector 必须不报警告（特别注意：broadcast goroutine 与单测主线程 capturedCalls 切片共享 → 必须用 mutex + WaitGroup 保护，详见 AC3 实装注意事项）

**AC11 — 跨 epic 一致性自检**

完成实装后跑以下抽测验证范围红线遵守 + 跨章节 schema 对齐：

- **代码范围 grep**：`git status --short` 显示**仅**命中本 AC1-AC8 钦定的文件清单 + 本 story 文件 + sprint-status.yaml；**未**命中任何 `docs/宠物互动App_*.md` / `_bmad/` 配置 / 其他 `_bmad-output/` yaml/md（除自己 + lessons 新增可能）/ `iphone/` / `ios/` / `server/migrations/` 任何 SQL / 14.2 / 14.3 已 done 的其他文件
- **错误码全局对齐**：grep `1003` / `ErrResourceNotFound` 不应命中本 story 任何路径（broadcast 路径无业务错误码，全 fire-and-forget log warn）；grep `RowsAffected` 不应命中本 story 任何路径
- **WS 实装范围**：grep `BroadcastToRoomExcept` / `broadcastExceptFn` 在本 story service 层 / router.go petBroadcastFn closure 路径**不应**出现（pet 广播范围含发起者，用 BroadcastToRoom 而非 Except）
- **包含发起者语义**：grep `excludeUserID` / `exclude joiner` / `exclude leaver` 在本 story service 层 / petBroadcastFn closure 不应命中（如出现必属反向理解 contract，应回滚至 V1 §12.3 line 2249 钦定语义）
- **三处 broadcast helpers 共存**：grep `BuildMemberJoinedEnvelope` / `BuildMemberLeftEnvelope` / `BuildPetStateChangedEnvelope` 应在 snapshot.go 同文件内连续定义（11.8 落地两个 + 14.4 落地一个）；本 story **不**把任一 helper 拆出独立文件 / 不动既有两个
- **router.go closures 共存**：grep `roomBroadcastFn` / `roomBroadcastExceptFn` / `petBroadcastFn` 在 router.go 内应有 3 个独立 closure 定义；本 story **不**复用既有 closure 也**不**抽公共 helper（YAGNI；3 个 closure 行为接近但语义独立）
- **下游 story 引用检查**：grep `Story 14\.4` / `iOS Epic 15` / `Story 15.2` / `Story 15.4` 在本 story 文件应命中下游 story 引用 + Epic 16 收口提示；不应命中"实际依赖 iOS Story 15.x / Epic 16 已 done" 性质的硬依赖（iOS Epic 15 仍 backlog）

**关键约束**：

- 本 story 的 dev 阶段**不** commit（epic-loop 流水线设计是 dev-story 阶段不 commit，由下游 fix-review / story-done sub-agent 统一收口）
- commit message 模板（dev 完成后由 story-done sub-agent 落地）：

```text
feat(server): Epic14/14.4 pet.state.changed WS 广播实装

- internal/app/ws/snapshot.go: 新增 PetStateChangedPayload + BuildPetStateChangedEnvelope helper
  - 与既有 BuildMemberJoinedEnvelope / BuildMemberLeftEnvelope 同包同模式
  - envelope: {type, requestId="", payload, ts=server now ms}
  - payload: {userId, petId, currentState} BIGINT 字符串化遵循 §2.5
- internal/service/pet_service.go: 激活 14.2 pre-wire 的 broadcast 挂载点
  - SyncCurrentState UPDATE 成功后查 users.current_room_id
  - 非 null → go s.broadcastPetStateChanged(detachedCtx, ...) 异步广播
  - null / FindByID err → 不广播（仅 log warn，不影响 HTTP 200）
  - 新增 broadcastPetStateChanged 私有方法 + petBroadcastTimeout 包级常量
  - 广播范围包含发起者自己（V1 §12.3 line 2249 钦定，与 member.joined / member.left 排除发起者不同）
  - fire-and-forget 严格语义（V1 §5.2 line 539 / §12.3 line 2217）
- internal/app/bootstrap/router.go: 新增 petBroadcastFn closure + petSvc 注入真实实例
  - petBroadcastFn 与 roomBroadcastFn 同模式 nil-tolerant
  - NewPetService(petRepo, userRepo, deps.SessionMgr, petBroadcastFn)
  - 替换 14.2 落地的 nil, nil 占位

闭合 Epic 14（pets.current_state 变更 → 同房间全员实时同步链路全部就绪）
强前置 iOS Epic 15 (pet.state.changed 接收 / merge / UI 驱动)
```

## Tasks / Subtasks

- [x] Task 1: AC1 `BuildPetStateChangedEnvelope` + `PetStateChangedPayload` 在 `server/internal/app/ws/snapshot.go` 新增（紧接 `BuildMemberLeftEnvelope` 之后）
  - [x] 1.1: PetStateChangedPayload struct + doc comment（与 V1 §12.3 行 2223-2230 字段表对齐）
  - [x] 1.2: BuildPetStateChangedEnvelope helper + doc comment + json.Marshal 防御性 wrap
  - [x] 1.3: 包级 import 不需新增（既有 encoding/json / fmt / time 已 import）
- [x] Task 2: AC2 `petServiceImpl.broadcastPetStateChanged` 私有方法新增 + `SyncCurrentState` 替换 TODO 占位
  - [x] 2.1: 包级常量 petBroadcastTimeout 新增（与 room_service.go postCommitTimeout 同模式）
  - [x] 2.2: import 新增 "strconv" + "time"
  - [x] 2.3: broadcastPetStateChanged 私有方法实装（fire-and-forget 严格语义 + doc comment）
  - [x] 2.4: SyncCurrentState 替换 14.2 落地的 `// TODO(Story 14.4)` 段为真实 broadcast 触发（含 user lookup + detached ctx + timeout + goroutine launch）
  - [x] 2.5: PetService interface / petServiceImpl struct / NewPetService 三处 doc comment 同步更新移除"14.4 预留"等过时措辞
- [x] Task 3: AC3 `PetService` 单测扩展（≥4 新增 broadcast case + 既有 case 同步更新）
  - [x] 3.1: case 1-5 既有 happy / pet-less / DB err / 幂等 case 同步更新 mock UserRepo.FindByID / mock BroadcastFn 注入（stubUserRepo 缺省 CurrentRoomID=nil → 不广播路径兼容既有断言）
  - [x] 3.2: case 6 wire 占位 case 删除（语义反转）
  - [x] 3.3: case 7 happy 用户在房间 → broadcastFn 调用 1 次 + payload 字段正确（含 envelope unmarshal 完整断言）
  - [x] 3.4: case 8 用户不在房间 → broadcastFn 调用 0 次
  - [x] 3.5: case 9 UpdateCurrentStateByID err → broadcastFn 调用 0 次
  - [x] 3.6: case 10 broadcastFn 返 error → SyncCurrentState 仍 HTTP 200 ack（fire-and-forget）
  - [x] 3.7: case 11 FindByID err → log warn + 不 broadcast + HTTP 200
  - [x] 3.8: case 12 同一秒多次 state-sync 每次都广播（≥4 要求外可选）
  - [x] 3.9: WaitGroup 同步 broadcast goroutine + race detector 验证
- [x] Task 4: AC4 `BuildPetStateChangedEnvelope` 单测（≥2 case）
  - [x] 4.1: happy envelope 字段全部正确（含 ts > 0 + ts > 当前-1s 范围验证）
  - [x] 4.2: currentState=1/3 边界值序列化
- [x] Task 5: AC5 `PetService` 集成测试新增 1 case
  - [x] 5.1: dockertest 建 user + pet + room + A join → mock broadcastFn captured-call 注入 → SyncCurrentState → 验证 broadcastFn 调用 + envelope 字段
- [x] Task 6: AC6 `PetsHandler` 集成测试新增 1 case（端到端 ws dial）
  - [x] 6.1: dockertest + 启 ws gateway + httptest gorilla/websocket dial → POST state-sync → 从 WS conn 读 pet.state.changed envelope 验证
- [x] Task 7: AC7 bootstrap router wire 替换
  - [x] 7.1: petBroadcastFn closure 新增（与 roomBroadcastFn 同模式）
  - [x] 7.2: NewPetService 第 3/4 参数从 nil, nil 替换为 deps.SessionMgr, petBroadcastFn
  - [x] 7.3: doc comment 同步更新
- [x] Task 8: AC8 router_test.go 既有 case 验证不破（`go test ./internal/app/bootstrap/...` 全绿；本 story 仅改 wire 参数，路由 path 不变，既有 fixture 自动适配；不新增 case）
- [x] Task 9: AC9 业务事件 log 规范遵守
  - broadcast 成功：`slog.Info("ws broadcast: pet.state.changed sent", ...)` 含 component/event/roomId/userId/petId/state/targetSessions
  - marshal 失败：`slog.Warn("ws broadcast: marshal envelope failed; skip broadcast", ...)`
  - broadcastFn 失败：`slog.Warn("ws broadcast: broadcastFn failed", ...)`
  - FindByID 失败：`slog.WarnContext(ctx, "pet state-sync: load user for broadcast failed; skip broadcast", ...)`
- [x] Task 10: AC10 `bash scripts/build.sh --test` + `--integration`（针对本 story 新增的两个 case）全绿
  - `--test`：全量 unit suite PASS（22 packages, ~17s 总用时）
  - `--integration -run TestPetService_..._Story144_BroadcastFnTriggered`：25.5s PASS
  - `--integration -run TestPetsHandlerIntegration_..._Story144_BroadcastsToSelfOnWS`：21.4s PASS
  - `--race`：cgo 工具链 Windows 本地不可用（gcc 缺失），跳过本地验证，依赖 CI 路径（CLAUDE.md 钦定 --race 为可选项）
- [x] Task 11: AC11 跨 epic 一致性自检（grep + git status 范围验证）
  - `git status --short`：仅命中本 AC1-AC8 钦定的 7 文件（router.go / snapshot.go / snapshot_test.go / pet_service.go / pet_service_test.go / pet_service_integration_test.go / pets_handler_integration_test.go）+ sprint-status.yaml + 本 story 文件
  - `RowsAffected` / `1003` / `ErrResourceNotFound` 在 pet_service.go 仅出现在 doc comment（标注"禁止"）
  - `BroadcastToRoomExcept` / `broadcastExceptFn` / `excludeUserID` 在 service 层 / router.go petBroadcastFn 路径**未**出现（pet 广播范围含发起者，用 BroadcastToRoom）
  - `Build(MemberJoined|MemberLeft|PetStateChanged)Envelope` 三个 helper 在 snapshot.go 同文件连续定义（line 460 / 483 / 548）
  - `roomBroadcastFn` / `roomBroadcastExceptFn` / `petBroadcastFn` 三个独立 closure 在 router.go 共存（line 387 / 396 / 422）
- [x] Task 12: 本 story 文件 Status 流转：ready-for-dev → in-progress → review（dev-story 阶段完结；后续 code-review / story-done sub-agent 接管）+ sprint-status.yaml 14-4-pet-state-changed-ws-广播 同步流转

## Dev Notes

### 关键架构 / 文档前置

**dev-story 前必读**：
- `docs/宠物互动App_总体架构设计.md`（架构总览，重读以理解 service / handler / repo 分层 + ws 模块边界）
- `docs/宠物互动App_V1接口设计.md` §5.2（行 488-617 POST /pets/current/state-sync 完整钦定 + 服务端逻辑步骤 5 broadcast 触发）+ §12.3 `### 宠物状态变更（pet.state.changed）`（行 2212-2259 envelope 字段表 + 关键约束）+ §1 节点 5 冻结声明（行 47-56）
- `docs/宠物互动App_数据库设计.md` §5.3 / §6.4（pets 表 + current_state 列 schema）+ §5.4（users 表 + current_room_id 列 schema）
- `docs/宠物互动App_时序图与核心业务流程设计.md`（节点 5 阶段时序：state-sync 接口 → UPDATE → broadcast）
- `docs/宠物互动App_Go项目结构与模块职责设计.md` §6.3 Pet 模块 + §6.10 Realtime 模块（service 包结构 + ws 包结构）

**dev-story 前必读**（实装上下文）：
- `server/internal/service/pet_service.go`（14.2 落地的全文 —— 重点关注 lines 108-115 service struct 字段预留 / lines 124-136 NewPetService / lines 140-195 SyncCurrentState 含 TODO 占位段）—— **本 story 仅替换该文件的 TODO 占位段 + 同步更新 doc comment + 新增 broadcastPetStateChanged 私有方法**
- `server/internal/service/room_service.go`（11.8 落地的全文 —— 重点关注 lines 1310-1406 broadcastMemberJoined / lines 1557-1589 broadcastMemberLeft / lines 575-613 enqueueRoomEvent / lines 17-31 postCommitTimeout）—— **本 story 沿用 11.8 fire-and-forget broadcast 模式 + detached ctx + timeout 模式，但不调用 enqueueRoomEvent（pet 广播无 per-room ordering 诉求，直接 go func() 启动 goroutine 即可）**
- `server/internal/app/ws/snapshot.go`（11.8 落地的 BuildMemberJoinedEnvelope / BuildMemberLeftEnvelope 完整实装，行 460-495）—— **本 story 沿用同模式新增 BuildPetStateChangedEnvelope**
- `server/internal/app/ws/broadcast.go`（10.5 / 11.8 落地的 BroadcastToRoom / BroadcastToRoomExcept primitive + BroadcastFn / BroadcastExceptFn type alias）—— **本 story 通过 router.go petBroadcastFn closure 间接调用 BroadcastToRoom，不直接 import broadcast 包**
- `server/internal/app/bootstrap/router.go`（行 365-416 wire 段 —— 重点关注 lines 387-401 roomBroadcastFn / roomBroadcastExceptFn closures + line 415 petSvc 构造）—— **本 story 在 line 401 后新增 petBroadcastFn closure + 替换 line 415 petSvc 构造参数**

**dev-story 前可选读**（depending 测试 fixture / lessons）：
- `server/internal/service/room_service_test.go`（11.8 落地的 broadcastMemberJoined / broadcastMemberLeft 单测 —— 重点参考 WaitGroup 同步 + capturedCalls 切片设计 + mock BroadcastFn 注入模式）
- `server/internal/app/ws/snapshot_test.go`（11.8 envelope helpers 单测 —— 直接对照新增 case 设计）
- `server/internal/app/ws/ws_integration_test.go`（11.8 ws end-to-end 集成测试 —— 直接对照 AC6 ws dial 测试设计）
- `_bmad-output/implementation-artifacts/11-8-成员加入-离开-ws-广播.md`（11.8 story 全文 —— 跨 story 范围红线 / acceptance criteria / fire-and-forget 语义参考）
- `_bmad-output/implementation-artifacts/14-2-post-pets-current-state-sync-接口-pets-current_state-更新.md`（14.2 story 全文 —— 重点理解 pre-wire 形态 + service struct 字段预留逻辑）
- `_bmad-output/implementation-artifacts/14-3-修改-roomsnapshotbuilder-snapshot-含真实-pet-currentstate.md`（14.3 story 全文 —— 理解 14.3 落地的权威等价桶三处真值切换，本 story 是第四处 pet.state.changed 落地）

### 实装关键决策与权衡

**1. broadcast 是否走 goroutine + detached ctx + timeout**：
- **采用**：与 11.8 enqueueRoomEvent 同模式 —— `context.WithoutCancel(ctx)` + `context.WithTimeout(detached, 10s)` + `go s.broadcastPetStateChanged(...)`
- **理由**：(a) request ctx cancel 信号传播会让 userRepo.FindByID fail "context canceled"，client 主动关闭连接 / handler deadline 都会触发，让 broadcast 静默 skip —— 与 V1 §5.2 line 539 "广播失败仅 log，不影响 HTTP 响应" 但 client cancel 时 broadcast 完全不触发的 semantics 偏差；(b) 11.8 已经验证 detached ctx + timeout 是正确模式（lessons 多处沉淀）；(c) 10s timeout 兜底防 goroutine 泄漏（DB 卡死场景）
- **不采用 enqueueRoomEvent（per-room queue）**：原因：(a) pet 广播无 per-room ordering 诉求 —— member.joined / member.left 需要保 commit-order = causal-order 才用 enqueueRoomEvent，但 pet.state.changed 是独立事件（用户 A 切 walk → 用户 B 切 run 即便乱序广播也不影响业务正确性，client 端字段级 merge 不依赖跨事件顺序）；(b) 同一秒多次 state-sync 即便业务上不该发生也合法（V1 §12.3 line 2253 钦定），不需要 server 端去重 / 排序；(c) 不引入 enqueueRoomEvent 让 pet broadcast 路径更轻，启动开销 ~5μs goroutine vs ~50μs queue+worker
- **不采用同步 broadcast（无 goroutine）**：原因：(a) 11.8 r2 已发现同步 broadcast 受 client ctx cancel 影响导致 broadcast 静默 skip，被 lesson 沉淀；(b) HTTP 200 响应路径不应被 broadcast 阻塞（even 同步 broadcastFn 是非阻塞入队 O(1)，但 user lookup 是 ~10ms DB IO，本路径用 goroutine 让 user lookup 不阻塞 HTTP 200）

**2. user lookup 在主路径同步执行 vs goroutine 内异步执行**：
- **采用**：user lookup 在 SyncCurrentState 主路径**同步**执行；如 user.CurrentRoomID != nil 才**启 goroutine** 走 broadcastPetStateChanged
- **理由**：(a) 用户不在房间是高频路径（房间外切 walk/run 也算 state-sync），让主路径同步 user lookup 后**不**启 goroutine，省去 goroutine 启动开销 + detached ctx 构造开销；(b) user.FindByID 在事务后调用是普通连接池查询，<10ms，主路径同步开销可忽略；(c) 如 user.FindByID 异常 → 主路径直接 log warn 后继续返 HTTP 200，**不**进 goroutine 路径，路径简洁

**3. broadcastFn closure 在 router.go 是否复用 roomBroadcastFn**：
- **不复用**：新增独立 `petBroadcastFn` closure
- **理由**：(a) 14.2 范围红线已钦定"逻辑独立，本 story 直接传 nil 让 14.4 灵活决定"；(b) future 任一路径如需差异化（如 prometheus metric label / log 字段前缀）不影响另一路径；(c) router.go 已有 roomBroadcastFn / roomBroadcastExceptFn 两 closures，新增 petBroadcastFn 不显著增加复杂度；(d) 让"pet 广播 vs room 广播"在 router wire 层就语义边界清晰
- **不抽公共 helper**：原因：3 个 closure 内部实现一致（nil-tolerant + wsapp.BroadcastToRoom 直调），但语义独立；YAGNI

**4. broadcast 范围是否含发起者自己**：
- **包含**（V1 §12.3 line 2249 关键约束 + §1 line 55 节点 5 冻结声明钦定）
- **service 层调 broadcastFn 而非 broadcastExceptFn**：差异点是 11.8 用 BroadcastToRoomExcept 是 member.joined / member.left 排除发起者；本 story 用 BroadcastToRoom 全 fanout
- **不为 self 做特殊路径**：service 层不识别 payload.userId == self；client 端自己识别 + 走 §5.2 self-broadcast 对称兜底规则（iOS Story 15.x 实装）
- **server 端实装层的 trade-off**：让所有 client 收到 envelope.payload.userId 后自己判断 self vs other —— 简单一致路径 vs envelope 体积略冗余（每个 client 收到 32 字节 userId 不大可忽略）

**5. envelope 字段集合是否扩展**：
- **严格 3 字段（userId / petId / currentState）**：V1 §12.3 line 2257 future fields 注 + 本 story 范围红线钦定
- **不**含 nickname / avatarUrl / equips / equips[].renderConfig 等任何字段（装备变更广播由独立路径触发，如 Epic 27 / 30 future）
- **不**新增 type field 子分类（如 "pet.state.changed.movement" / "pet.state.changed.equipment"）—— 每条业务 WS 消息单一职责，避免 type field overload

### Project Structure Notes

- **目录对齐**（按 `docs/宠物互动App_Go项目结构与模块职责设计.md` §6.3 Pet 模块 + §6.10 Realtime 模块）：
  - `server/internal/service/pet_service.go`（service 层，本 story 改）—— PetService.SyncCurrentState 业务逻辑
  - `server/internal/app/ws/snapshot.go`（ws 层，本 story 扩展）—— BuildPetStateChangedEnvelope helper（沿用 11.8 envelope helpers 同文件，**不**新建 pet_envelope.go 避免文件碎片）
  - `server/internal/app/bootstrap/router.go`（bootstrap 层，本 story 改）—— petBroadcastFn closure + petSvc wire
- **测试目录对齐**：
  - `server/internal/service/pet_service_test.go`（service 单测，本 story 扩展）
  - `server/internal/service/pet_service_integration_test.go`（service 集成测试，本 story 扩展）
  - `server/internal/app/ws/snapshot_test.go`（ws envelope 单测，本 story 扩展）
  - `server/internal/app/http/handler/pets_handler_integration_test.go`（handler 集成测试，本 story 扩展端到端 ws dial）
- **不变量**：本 story 全部代码改动局限于 `server/internal/service/` + `server/internal/app/ws/` + `server/internal/app/bootstrap/` 三目录；**未**触及 `server/internal/repo/` / `server/migrations/` / `iphone/` / `ios/` 任何路径

### Conflicts / Variances（与 epics.md / V1 doc 的偏移说明）

**1. epics.md §Story 14.4 (行 2341-2363) 钦定 "**集成测试覆盖**（dockertest + Redis + 真实 WS）"**：
- **本 story 偏移**：dockertest + 真实 WS 已覆盖（AC5 + AC6）；**不**需要 Redis（节点 5 阶段 ws gateway 单实例 + BroadcastToRoom 仅消费 SessionManager 内存索引；Redis presence 是 10.6 落地的独立路径，broadcast 路径不消费 Redis）
- **理由**：V1 §12.3 / §10.5 钦定的 broadcast 路径在节点 5 阶段不跨实例，不需要 Redis pub/sub；与 10.5 BroadcastToRoom primitive 实装一致（"MVP 单实例阶段走 SessionManager，多实例阶段（节点 13+）才走 Pub/Sub"）

**2. epics.md 钦定 "happy: 用户不在房间 → broadcast 不被调用"** 与 V1 §5.2 line 540 钦定 "null → 不广播"：
- **完全一致**：本 story AC3 case 8 严格按此实装

**3. epics.md 钦定 "edge: state-sync 失败 → broadcast 不被调用"** 与 V1 §5.2 line 532-537 服务端逻辑步骤 4 ："UPDATE pets ... `err == nil` 进步骤 5 / `err != nil` → 1009"：
- **完全一致**：本 story AC3 case 9 严格按此实装（UpdateCurrentStateByID err → broadcastFn 调用 0 次）

**4. epics.md 钦定 "edge: BroadcastToRoom 失败（网络 error）→ state-sync 接口仍返回成功"** 与 V1 §12.3 line 2254 钦定 "fire-and-forget"：
- **完全一致**：本 story AC3 case 10 严格按此实装

### References

- `docs/宠物互动App_V1接口设计.md` §1 (line 47-56) — Story 14.1 完成后 §5.2 + §12.3 `### 宠物状态变更` 进入冻结
- `docs/宠物互动App_V1接口设计.md` §5.2 (line 488-617) — POST /pets/current/state-sync 完整钦定（特别 line 532-541 服务端逻辑步骤 5 + line 547-551 self-broadcast 对称兜底）
- `docs/宠物互动App_V1接口设计.md` §12.3 (line 2212-2259) — `### 宠物状态变更（pet.state.changed）` envelope 字段表 + 关键约束（特别 line 2249 广播范围含发起者 + line 2254 fire-and-forget 语义 + line 2255 ts 业务排序禁令）
- `docs/宠物互动App_数据库设计.md` §5.3 / §6.4 — pets 表 + current_state 列 schema（TINYINT NOT NULL DEFAULT 1，枚举 1/2/3）
- `docs/宠物互动App_数据库设计.md` §5.4 — users 表 + current_room_id 列 schema（BIGINT UNSIGNED NULL）
- `docs/lessons/2026-05-12-state-sync-idempotent-rowsaffected-and-ws-envelope-ts.md` — 14-1 r1：幂等 + RowsAffected 误判 + WS envelope ts 字段归属
- `docs/lessons/2026-05-12-cross-section-equivalence-claim-must-fence-prerequisites-and-self-broadcast-fallback-2.md` — 14-1 r2：跨章节字段等价声明锁定前置 + ack vs 权威分层
- `docs/lessons/2026-05-12-member-joined-stale-state-and-self-broadcast-arrival-order-symmetric-3.md` — 14-1 r3：self-broadcast no-op 措辞基于到达顺序对称
- `docs/lessons/2026-05-12-self-broadcast-ui-driver-and-freeze-boundary-and-self-vs-others-priority-4.md` — 14-1 r4：self-broadcast UI 驱动 + self vs 他人优先级
- `docs/lessons/2026-05-12-merge-contract-exception-and-ts-business-ordering-ban-and-ack-bucket-explicit-enum-5.md` — 14-1 r5：ts 业务排序禁令 + 权威等价桶四处枚举
- `docs/lessons/2026-05-12-state-sync-err-binary-and-placeholder-whitelist-self-http-ack-14-1-r6.md` — 14-1 r6：state-sync err 二分锁定
- `docs/lessons/2026-05-12-state-sync-pet-less-noop-consistent-with-home-room-snapshot-14-1-r7.md` — 14-1 r7：pet-less 合法 edge case
- `docs/lessons/2026-05-12-story-file-must-stay-in-sync-with-frozen-v1-doc-14-1-r8.md` — 14-1 r8：story 文件与 V1 doc 同步
- `docs/lessons/2026-05-12-story-file-rowsaffected-and-top-level-1003-drift-14-1-r9.md` — 14-1 r9：story 文件 RowsAffected / 1003 drift
- `docs/lessons/2026-05-12-story-ac-authority-bucket-direction.md` — 14-1 r10：Story AC 权威等价语义区分字段方向
- `docs/lessons/2026-05-12-story-file-14-3-scope-must-list-member-joined-14-1-r11.md` — 14-1 r11：story 文件 14.3 落地范围三处统一
- `docs/lessons/2026-05-12-nil-deref-defense-and-integration-evidence-14-3-r1.md` — 14-3 r1：nil-deref 防御 + hardcoded → 真实值切换 integration test fixture 区别值
- `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md` — Story 14.1 全文（契约定稿）
- `_bmad-output/implementation-artifacts/14-2-post-pets-current-state-sync-接口-pets-current_state-更新.md` — Story 14.2 全文（含 service pre-wire 形态）
- `_bmad-output/implementation-artifacts/14-3-修改-roomsnapshotbuilder-snapshot-含真实-pet-currentstate.md` — Story 14.3 全文（含三处真值切换）
- `_bmad-output/implementation-artifacts/11-8-成员加入-离开-ws-广播.md` — Story 11.8 全文（fire-and-forget broadcast 同模式参照）
- `_bmad-output/implementation-artifacts/10-5-broadcasttoroom-primitive.md` — Story 10.5 全文（BroadcastToRoom primitive 底层来源）
- `_bmad-output/implementation-artifacts/decisions/0006-error-handling.md` — ADR-0006 错误码三层映射（本 story 沿用，不新增决策）
- `_bmad-output/implementation-artifacts/decisions/0007-ctx-propagation.md` — ADR-0007 ctx 传播（本 story 严格遵守 detached ctx + timeout 模式）
- `_bmad-output/implementation-artifacts/decisions/0011-ws-stack.md` — ADR-0011 ws stack（本 story 沿用，不新增决策）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

无 HALT。所有 RED → GREEN 转换均一次成功。

### Completion Notes List

- **AC1 + AC4（ws envelope helper + 单测）**：`server/internal/app/ws/snapshot.go` 在
  `BuildMemberLeftEnvelope` 之后新增 `PetStateChangedPayload` struct + `BuildPetStateChangedEnvelope`
  helper，沿用 11.8 envelope helpers 同文件同模式；`snapshot_test.go` 新增 3 case
  覆盖 happy / 边界值 / 字段集合严格 3 字段，`fmt` import 同步添加
- **AC2 + AC3（service 层激活 14.2 pre-wire + 单测）**：`pet_service.go` 替换 14.2 留的
  `// TODO(Story 14.4)` 段为真实 broadcast 触发，新增 `broadcastPetStateChanged`
  私有方法 + 包级常量 `petBroadcastTimeout = 10 * time.Second`；主路径 user lookup
  同步 + `context.WithoutCancel + WithTimeout + go func()` 启 goroutine，对齐 11.8
  detached ctx + timeout 模式但**不**走 enqueueRoomEvent per-room queue（pet 广播无
  causal ordering 诉求）。doc comments 同步移除"14.4 预留"过时措辞
- **AC3 单测**：6 个新增 case（7-12）+ 5 个既有 case（1-5）同步更新；删除原 case 6
  "广播路径未被触发占位"（语义反转）；用 sync.WaitGroup + broadcastRecorder + waitWithTimeout
  helper 同步 broadcast goroutine 防 race
- **AC5（service 集成测试）**：`pet_service_integration_test.go` 新增 1 case
  `Integration_Story144_BroadcastFnTriggered`，dockertest 真 MySQL + 注入 broadcastRecorder
  → 验证 broadcastFn 调用 + envelope/payload 完整断言；fixture state=2（与 14-3 r1 lesson
  钦定的"区别值"模式一致）
- **AC6（handler 端到端 WS dial）**：`pets_handler_integration_test.go` 新增 1 case
  `Story144_BroadcastsToSelfOnWS`，dockertest + 启 ws gateway + httptest server + gorilla
  websocket Dial → 跳过 room.snapshot → POST state-sync → 从 WS conn 读 pet.state.changed
  envelope；验证 A 自己 WS 通道收到 envelope（"广播范围含发起者自己"的具体落地证明）
- **AC7（router wire）**：`router.go` 在 `roomBroadcastExceptFn` 之后新增 `petBroadcastFn`
  closure（与 roomBroadcastFn 同 nil-tolerant 模式，但用 `BroadcastToRoom` 而非
  `BroadcastToRoomExcept` —— pet 广播含发起者）；`petSvc` 构造参数从 `nil, nil` 替换为
  `deps.SessionMgr, petBroadcastFn`
- **关键技术点验证**（grep 自检全部通过）：fire-and-forget / 广播含发起者自己 /
  detached ctx + goroutine / 14.2 pre-wire 真实激活 / 三个 envelope helpers
  在 snapshot.go 共存 / 三个 broadcast closure 在 router.go 共存 / 范围红线（仅 7 文件
  改动，未触及 docs / migrations / iphone / ios）

### File List

新增 / 修改文件（7 个 server 文件 + 1 个 story 文件 + 1 个 sprint-status）：

- `server/internal/app/ws/snapshot.go` —— 新增 `PetStateChangedPayload` struct +
  `BuildPetStateChangedEnvelope` helper（+~75 行）
- `server/internal/app/ws/snapshot_test.go` —— 新增 3 case（`fmt` import + ~110 行）
- `server/internal/service/pet_service.go` —— 重写 / 扩展（+~140 行净增；激活 14.2 pre-wire
  + 新增 `broadcastPetStateChanged` 私有方法 + 包级常量 `petBroadcastTimeout` +
  import `strconv` / `time`）
- `server/internal/service/pet_service_test.go` —— 重写 / 扩展（+~340 行；新增
  `broadcastRecorder` / `waitWithTimeout` 共用 helper + 6 个新 case）
- `server/internal/service/pet_service_integration_test.go` —— 新增 1 case
  `Integration_Story144_BroadcastFnTriggered`（+~140 行；含 `uint64ToString` 工具）
- `server/internal/app/http/handler/pets_handler_integration_test.go` —— 新增 1 case
  `Story144_BroadcastsToSelfOnWS`（+~230 行；含 `buildPetsHandlerIntegrationWithWS`
  fixture + `petsWSEnd2End*` 类型）
- `server/internal/app/bootstrap/router.go` —— 新增 `petBroadcastFn` closure +
  `petSvc` 构造点替换 nil 参数（+~25 行）
- `_bmad-output/implementation-artifacts/14-4-pet-state-changed-ws-广播.md` —— Status
  流转 + Tasks/Subtasks 勾选 + Dev Agent Record / File List 填充
- `_bmad-output/implementation-artifacts/sprint-status.yaml` —— Story 14.4 状态
  ready-for-dev → in-progress → review

### Change Log

- 2026-05-12 dev-story（Claude Opus 4.7）：Story 14.4 全量实装完成；service 层激活
  14.2 pre-wire 的 broadcast 挂载点；3 个 envelope helpers 在 ws 包共存；3 个
  broadcast closure 在 router.go 共存；fire-and-forget 严格语义 + 广播范围含发起者
  自己（V1 §12.3 line 2249 钦定）；单测 11 case + 集成测试 5 case 全绿
