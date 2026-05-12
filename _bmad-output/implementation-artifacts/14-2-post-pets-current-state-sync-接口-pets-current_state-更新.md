# Story 14.2: POST /pets/current/state-sync 接口 + pets.current_state 更新（service / repo / handler / router 全栈实装 + WS 广播触发挂载点 + 单测 ≥4 case + dockertest 集成测试）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a iPhone 用户,
I want **正式实装 POST /api/v1/pets/current/state-sync 接口**：handler 层 `PetsHandler.PostStateSync` 解析 `{state: int}` 单字段 + 参数校验（state ∈ {1,2,3}）+ 从 auth 中间件取 userID + 调 service + handler 不写 envelope；service 层 `PetService.SyncCurrentState` 走 **server-acknowledged 路径**：查默认 pet（`pets WHERE user_id=? AND is_default=1`）→ pet-less 走 noop（跳 UPDATE 跳广播 + 返 200 OK + 回显入参 state，与 §5.1 / §10.3 / §12.3 pet-less 合法 edge case 同语义）→ 否则 `UPDATE pets SET current_state=?, updated_at=NOW() WHERE id=?`（`err == nil` ⇒ 200 OK + code = 0；`err != nil` ⇒ 1009；**不**读 `RowsAffected`、**不**根据该值分支业务逻辑）→ 成功后**预留** WS 广播挂载点（本 story **不**真正发广播，14.4 才落地；本 story 在 service 层留 `// TODO(Story 14.4): broadcast pet.state.changed if users.current_room_id != NULL` 占位 + service 注入字段先 nil + 单测覆盖广播路径 wire 接口）→ response 回显入参 state（`data.state` 等于 request state，server-acknowledged ack 信号）；repo 层 `PetRepo` 扩展 `UpdateCurrentStateByID(ctx, petID uint64, state int8) error` 方法（**不**新增 `UpdateCurrentStateByUserID` —— 钦定按 PetID 走主键定位，与 service 层"查默认 pet 拿 pet.ID → 用主键 UPDATE"路径一致；与 user_repo `UpdateCurrentRoomID` 走 user.id 一样按主键定位）；handler 层 `PetsHandler` 新建 + 挂 `authedGroup.POST("/pets/current/state-sync", petsHandler.PostStateSync)`；service / handler / repo / router 单测覆盖（≥4 case 单测，dockertest 集成测试覆盖"DB 建 user+pet → POST state-sync → 验证 pets.current_state 落库"），
so that **下游 Story 14.3（修改 RoomSnapshotBuilder - snapshot 含真实 pet.currentState）/ Story 14.4（pet.state.changed WS 广播）/ iOS Epic 15 全部 stories（15.1 房间页多成员猫位渲染 / 15.2 pet.state.changed WS 消息处理 / 15.4 自己状态变化时上报 state-sync / 15.5 跨房间状态恢复）/ Story 16.x（节点 5 跨端集成 e2e + demo 验收 + tech debt 登记）** 全部能基于一个**已实装、已测试、已挂载到 router 的 POST /pets/current/state-sync 接口**展开 —— Story 14.3 实装的 `RoomSnapshotBuilder` 在 14.2 落地后即可从 DB 读 `pets.current_state` 真实值（替换 `1` placeholder）；Story 14.4 在 14.2 service 函数末尾加挂广播触发逻辑（本 story 已预留 TODO 占位 + service 注入字段先 nil 让 14.4 直接补 wiring）；iOS Story 15.4 客户端 `StateSyncUseCase` 调本接口拿 HTTP 200 + `data.state` ack 信号；iOS Story 15.2 接收 WS 广播驱动他人 roster pet state；iOS Story 15.5 跨房间状态恢复借由 14.2 落地后的 DB 真值持久化。本 story **不**实装 14.3 / 14.4 / iOS 15.x 中的任何东西，仅做"接口 + DB UPDATE + pet-less noop + handler / router / 单测 / 集成测试"这一个单接口动作。

## 故事定位（Epic 14 第二条 = Epic 14 第一条业务实装 story；上承 14.1 契约 + 节点 4 server 端基础设施，下启 14.3 / 14.4 + iOS Epic 15）

- **Epic 14 进度**：14.1（POST /pets/current/state-sync + WS pet.state.changed 契约定稿，done）→ **14.2（本 story，POST /pets/current/state-sync 接口 + pets.current_state UPDATE 实装）** → 14.3（修改 RoomSnapshotBuilder - snapshot 含真实 pet.currentState）→ 14.4（pet.state.changed WS 广播）。**14.2 是 14.3 / 14.4 的强前置**：14.3 修改 `RoomSnapshotBuilder` 时需要 `pets.current_state` 列能被业务路径写入（否则 snapshot 即便从 DB 读也只能读到 4.6 firstTimeLogin 初始化的固定 `1`）；14.4 加挂的广播触发点严格在 14.2 service 层 `UPDATE pets ... err == nil` 之后；本 story 在 service 层留好"广播触发位"的 TODO 占位 + service struct 字段（broadcastFn / sessionMgr）先以 nil 注入（与 11.8 `roomServiceImpl` 加 `broadcastFn` 字段 + 在 NewRoomService 注入时 nil-tolerant 同模式），14.4 落地仅需补 `if user.CurrentRoomID != nil { go s.broadcastPetStateChanged(...) }` 调用。
- **本 story 是 Epic 14 第一个真实业务实装 story**：上一 story（14.1）是纯文档契约定稿；本 story 才是 Epic 14 第一个**写 service / handler / repo / 挂路由 / 单测 / 集成测试**的实装 story；与 Story 4.6 (游客登录初始化事务) / Story 7.3 (POST /steps/sync) / Story 11.3 (POST /rooms 创建房间事务) 的"epic 第一条业务实装"模式直接对照。
- **本 story 是 14.3 / 14.4 / iOS Epic 15 全部 stories 的强前置**：
  - **14.3 修改 RoomSnapshotBuilder**：`RoomSnapshotBuilder.BuildSnapshot` 在查 room_members → JOIN users → JOIN pets 时把 `pets.current_state` 读出后回填到 `SnapshotMember.Pet.CurrentState`（替换 `1` placeholder）；本 story 让 `pets.current_state` 列能被业务路径动态写入是 14.3 真实驱动的数据起点（如果只有 4.6 firstTimeLogin 写一次 `1`，14.3 切换为真实读也只能读到 `1`）
  - **14.4 pet.state.changed WS 广播**：本 story service 层 `UPDATE pets ... err == nil` 之后留好"广播触发位"TODO 占位，14.4 落地仅需在该位置加 `s.broadcastPetStateChanged(ctx, in.UserID, pet.ID, in.State)` 调用 + `broadcastPetStateChanged` 方法实装（查 `users.current_room_id` 非 null → 调 `s.broadcastFn(ctx, *user.CurrentRoomID, msgBytes)` 推 `pet.state.changed` envelope）；本 story 的 service struct 已预留 `sessionMgr` / `broadcastFn` 字段（nil-tolerant，14.4 在 router.go wire 真实实例）
  - **iOS Story 15.1 房间页多成员猫位渲染**：依赖 14.3 RoomSnapshotBuilder 真实驱动；间接依赖本 story（DB 真值入口）
  - **iOS Story 15.2 pet.state.changed WS 消息处理**：依赖 14.4 WS 广播；间接依赖本 story（service 层 UPDATE 成功路径）
  - **iOS Story 15.4 自己状态变化时上报 state-sync**：iOS 端 `StateSyncUseCase.execute(state: Int)` 直接调本接口；client 收 200 OK + `data.state` 走 §5.2 self-broadcast 兜底规则（HTTP 200 先到 → 立即驱动本地 self entry UI 更新；后续 14.4 落地后 self-broadcast 到达走 merge no-op）
  - **iOS Story 15.5 跨房间状态恢复**：用户进新房间 → WS 握手 → 收 `room.snapshot` 含 `members[].pet.currentState`（14.3 落地后真值）；本 story 是该真值的数据起点
  - **Story 16.x 节点 5 demo 验收 + 跨端集成 e2e + tech debt 登记**：14.x 全部完成 + iOS Epic 15 完成后由 Epic 16 收口；本 story 是 demo 验收链路的第一步（state-sync 接口可用）

- **epics.md §Story 14.2 钦定**（行 2297-2319）：
  - **Given** Story 4.6 默认 pet 已在登录初始化时创建（pets.current_state 默认 1）+ Story 14.1 契约已定
  - **When** 调用 `POST /pets/current/state-sync` 带 state ∈ {1,2,3}
  - **Then** service 找到当前用户的默认 pet（pets WHERE user_id=? AND is_default=1）→ 更新 current_state = state
  - **And** 接口要求 auth
  - **And** state 不在 [1,2,3] → 1002 参数错误
  - **And** 用户没有 pet（理论不该发生，但兜底）→ ~~1003~~（**已被 14.1 r7 reversed**：pet-less 改走 server-acknowledged noop 路径返回 200 OK + code = 0，与 §5.1 / §10.3 / §12.3 pet-less 合法 edge case 同语义；详见 docs/lessons/2026-05-12-state-sync-pet-less-noop-consistent-with-home-room-snapshot-14-1-r7.md + V1 §5.2 服务端逻辑步骤 3 + §5.2 错误码表注 + §1 节点 5 冻结声明）
  - **And** **状态写库不要求高频**：业务上 iOS 端只在状态切换瞬间上报，不每秒上报；server 不限频但应在日志中注意频率异常
  - **And** **单元测试覆盖**（≥4 case，mocked pet repo）:
    - happy: state=2 → pet.current_state 更新为 2，返回 {state: 2}
    - edge: state=4（非法）→ 1002，DB 不变（service 层不到这步；handler 层就拦截 + 不调 service —— 本 case 在 handler 单测覆盖；service 单测可省略此 case 但仍至少 ≥4 case 总数，例如加 `repo.UpdateCurrentStateByID` 返 raw error → 1009）
    - edge: 用户无默认 pet → ~~1003~~（**已 reversed**）→ **走 server-acknowledged noop 路径**：service 跳 UPDATE + 返 SyncCurrentStateOutput{State: in.State}（回显入参）；**单测必须覆盖该路径** + 验证 repo.UpdateCurrentStateByID **未被调用**
    - happy: 同一 state 重复上报 → 接受但不报错（幂等）—— service 层 UPDATE 路径无 RowsAffected 判定，`err == nil` ⇒ 200 OK
  - **And** **集成测试覆盖**（dockertest）:
    - 创建 user + 默认 pet（current_state=1）→ POST /pets/current/state-sync {state: 3} → DB pets.current_state = 3
    - 再 POST {state: 1} → DB pets.current_state = 1

- **14.1 已 frozen contract（V1 §5.2 + §12.3）—— 本 story 严格按契约实装，不重新评审 schema**：
  - **§5.2 接口元信息**：POST /api/v1/pets/current/state-sync，auth 必需，限频默认（Story 4.5 RateLimitByUserID 60 次/分），幂等（同 state 重复上报合法），**不**接受 `idempotencyKey` header
  - **§5.2 请求体**：`{state: int (1=rest, 2=walk, 3=run)}` 单字段必填；state 不在 {1,2,3} → 1002（handler 层拦截，service 层不重复校验 —— 与 7.3 handler 层 motionState 校验 / 11.3 handler 层 ParseUint roomID 校验 同模式）
  - **§5.2 服务端逻辑**：步骤 1（auth + 限频）/ 2（参数校验 → 1002）/ 3（查 pets WHERE user_id=? AND is_default=1 → 0 行走 noop / 1 行进步骤 4）/ 4（UPDATE pets SET current_state=?, updated_at=NOW() WHERE id=? → `err == nil` 进步骤 5 / `err != nil` → 1009；**不**读 RowsAffected）/ 5（**广播 fire-and-forget**：检查 users.current_room_id 非 null → BroadcastToRoom；null → 不广播；**本 story 不真正落地广播，14.4 才落地，本 story service 留 TODO 占位**）/ 6（响应回显入参 state）
  - **§5.2 响应体**：`{code: 0, message: "ok", data: {state: <回显入参>}, requestId: "req_xxx"}`；`data.state` 字段是 server-acknowledged ack 信号，回显入参（与 service step 4 UPDATE 入库的入参值完全等价）
  - **§5.2 错误码**：1001（auth 失败）/ 1002（state 字段缺失 / 类型非 int / 不在 {1,2,3}）/ 1005（限频）/ 1009（DB 异常）；**不**含 1003（pet-less 走 noop 而非业务错误，r7 reversed）；**不**含 3xxx / 4xxx / 5xxx / 6xxx / 7xxx（state-sync 不涉及步数 / 宝箱 / 装扮 / 房间业务 / 表情业务；用户不在房间不视为业务错误）
  - **§5.2 关键约束（本 story 必须遵守）**：(a) **`err == nil` ⇒ 200 OK + code = 0；`err != nil` ⇒ 1009；不读 RowsAffected**（r6 lessons + r9 sweep checklist）；(b) **pet-less 走 server-acknowledged noop 路径**（r7 lessons）；(c) **广播范围包含发起者自己**（与 `member.joined` / `member.left` 排除发起者不同语义；本 story 不实装广播，14.4 才落地，但 service 层 TODO 占位 / 注释 / 测试 fixture 必须遵守该语义不写反）；(d) **fire-and-forget 广播失败不影响 HTTP 响应**（本 story 不实装广播，14.4 才落地，但 service 层 TODO 占位必须显式注解 fire-and-forget 语义）；(e) **HTTP 200 / WS 广播基于到达顺序对称的 self-only 兜底规则**（仅 iOS Epic 15 实装侧需消化；server 层无对称信号处理诉求）
  - **§12.3 `### 宠物状态变更（pet.state.changed）` envelope**：`type=pet.state.changed`、`requestId=""`、`payload={userId: <BIGINT 字符串化>, petId: <BIGINT 字符串化>, currentState: <int 1/2/3>}`、`ts=server time.Now().UnixMilli()`（本 story 不实装真实广播，14.4 才落地，但 service 层 TODO 占位注释中可引用 envelope schema 字段表给 14.4 使用）

- **14-1 lessons 必须遵守（11 条 r1-r11 sequence）**：
  - **r1**（[幂等 + RowsAffected 误判 / WS envelope 字段归属](../../docs/lessons/2026-05-12-state-sync-idempotent-rowsaffected-and-ws-envelope-ts.md)）：service 层**永远不读 RowsAffected** —— `err == nil` ⇒ 200 OK + code = 0 / `err != nil` ⇒ 1009（两个互斥二分）；本 story service 层 `UpdateCurrentStateByID` 调用后**禁止**写 `result.RowsAffected == 0 → return 1009` / `result.RowsAffected == 0 → return 1003`；广播 envelope（14.4 才实装）`ts` 字段必须在 server 端生成（`time.Now().UnixMilli()`），**不**用 client 提交时间戳
  - **r2**（[跨章节字段等价声明锁定前置 + ack vs 权威分层 + self-broadcast 丢失兜底](../../docs/lessons/2026-05-12-cross-section-equivalence-claim-must-fence-prerequisites-and-self-broadcast-fallback-2.md)）：本 story service 层 response 回显入参 `data.state = in.State`（**不**重新从 DB 查 pet.current_state 读出后返回 —— 哪怕 happy path 值相等，**不读 DB 减少一次查询 + 让 ack-only 语义清晰**）；service 层 `SyncCurrentStateOutput` struct 字段就一个 `State int8`，与 request DTO `state` 字段值层完全等价（不字符串化、不枚举类型转换）
  - **r3**（[member.joined `pet.currentState` 14.3 落地前 stale race + self-broadcast no-op 措辞基于到达顺序对称](../../docs/lessons/2026-05-12-member-joined-stale-state-and-self-broadcast-arrival-order-symmetric-3.md)）：本 story service 层 service 注释必须**显式标注** "本 service 层执行 UPDATE pets.current_state 后，房间内其他成员通过 14.4 落地的 `pet.state.changed` WS 广播感知；本 service 层**不**广播 `member.joined` / `room.snapshot` / 其他 WS 消息"
  - **r4**（[self-broadcast UI 驱动 + 冻结边界声明区分抽象触发与阈值 + self vs 他人优先级](../../docs/lessons/2026-05-12-self-broadcast-ui-driver-and-freeze-boundary-and-self-vs-others-priority-4.md)）：本 story 范围红线**不**包含"iOS 端 self-broadcast 到达顺序处理" —— 那归属 iOS Story 15.2 / 15.4；server 层**不**为 self-broadcast 做特殊路径（与 11.8 `member.joined` 排除 joiner 不同：14.4 落地 `pet.state.changed` 将广播给房间内**全部**在线成员**含**发起者自己）
  - **r5**（[临时窗口优先级 merge contract + `ts` 业务排序禁令 + 权威等价桶四处枚举](../../docs/lessons/2026-05-12-merge-contract-exception-and-ts-business-ordering-ban-and-ack-bucket-explicit-enum-5.md)）：本 story service 层**禁止**让 `ts` 字段参与任何业务排序 / 状态新旧判定（14.4 落地广播 envelope 时 `ts` 仅作日志关联 / UI 辅助展示）；server 端 UPDATE 是物理时序，DB 自己保证按 SQL 提交顺序应用
  - **r6**（[state-sync err 二分锁定 + placeholder 例外白名单覆盖 self HTTP ack](../../docs/lessons/2026-05-12-state-sync-err-binary-and-placeholder-whitelist-self-http-ack-14-1-r6.md)）：本 story service 层 UPDATE 后**严格**走两个互斥二分（`err == nil` / `err != nil`），不存在第三条路径；service 注释 / dev notes / 单测都要**显式**写明该锁定（防 review 阶段把 RowsAffected 当成"幂等性证据"重新引入）
  - **r7**（[state-sync pet-less 与 /home / room / member.joined 同语义合法 edge case](../../docs/lessons/2026-05-12-state-sync-pet-less-noop-consistent-with-home-room-snapshot-14-1-r7.md)）：本 story service 层 pet-less 路径（`SELECT id FROM pets WHERE user_id=? AND is_default=1` 0 行）**严格**走 server-acknowledged noop —— **不**返 apperror.ErrResourceNotFound (1003) / **不**返 mysql.ErrPetNotFound 透传给 handler；handler 层**禁止**为 pet-less 加 special-case 状态码降级；client 层（iOS Story 15.4）**不**需要为 pet-less 加 special-case suppress
  - **r8**（[story 文件与 V1 doc 同步 self-broadcast 对称兜底 + `ts` 禁令 + 等价分层](../../docs/lessons/2026-05-12-story-file-must-stay-in-sync-with-frozen-v1-doc-14-1-r8.md)）：本 story 文件描述（Story / 故事定位 / Acceptance Criteria / Dev Notes / References）的 schema 引用 / 错误码描述 / pet-less 语义 / RowsAffected 锁定**严格**与 V1 doc + 14-1 story 文件 + 14-1 lessons 三方对齐；review 阶段任何"story 文件 vs V1 doc drift"都必须在本 story 内修复，不留给下游 14.3 / 14.4
  - **r9**（[story 文件 RowsAffected 措辞 + 顶层 1003 引用 drift](../../docs/lessons/2026-05-12-story-file-rowsaffected-and-top-level-1003-drift-14-1-r9.md)）：本 story 文件**禁止**残留任何"RowsAffected == 0 → 1009" / "用户无 pet → 1003" 等 r6 / r7 之前的措辞 —— story 文件全文（Story / 故事定位 / AC / Dev Notes / References / Tasks）grep 'RowsAffected' / '1003' 不应命中任何"业务路径触发"语义（仅可在错误码全局对齐段引用 1003 在 §3 全局表保留 + 本接口不触发）
  - **r10**（[Story AC 权威等价语义区分字段方向 client→server / server→client / ack-only](../../docs/lessons/2026-05-12-story-ac-authority-bucket-direction.md)）：本 story AC 描述 `data.state` 字段时**必须**标注其方向（server → client）+ 权威性级别（ack-only，**不**入权威等价桶）；不要把 request `state`（client → server 写入信号）+ response `data.state`（ack-only 信号）+ 14.4 广播 `payload.currentState`（server → client 权威信号）+ §10.3 GET `pet.currentState`（server → client 权威信号自 14.3 起）混入同一个"字段等价桶"
  - **r11**（[story 文件 14.3 落地范围三处统一 + References 1003 残留清理](../../docs/lessons/2026-05-12-story-file-14-3-scope-must-list-member-joined-14-1-r11.md)）：本 story 描述下游 14.3 落地范围时**必须**同时列出 `RoomSnapshotBuilder.payload.members[].pet.currentState` + `GET /rooms/{roomId}.data.members[].pet.currentState` + `member.joined.payload.pet.currentState` 三处统一切换为真实驱动（与 V1 doc §5.2 关键约束等价分层桶四处枚举对齐 —— 本 story 描述层删 §10.3 GET 表述不影响 server 实装；但要确保下游 reader 不漏 14.3 落地范围）；References 段不残留 1003 措辞

- **范围红线**（**严格遵守**，与 Story 4.6 / 7.3 / 11.3 同模式）：
  - 本 story **只**改：
    - `server/internal/repo/mysql/pet_repo.go`（**扩展**；给既有 `PetRepo` interface 加 `UpdateCurrentStateByID(ctx context.Context, petID uint64, state int8) error` 方法签名 + impl）
    - `server/internal/repo/mysql/pet_repo_test.go`（**扩展**；新增 UpdateCurrentStateByID happy + DB 异常 2 个 sqlmock case；既有 Create / FindDefaultByUserID 单测保留不动）
    - `server/internal/service/pet_service.go`（**新建**；PetService interface + petServiceImpl + `SyncCurrentState(ctx, in SyncCurrentStateInput) (*SyncCurrentStateOutput, error)` 方法 + DTO 类型 + service struct 字段预留 sessionMgr / broadcastFn（14.4 才 wire 真实实例，本 story nil-tolerant + TODO 占位））
    - `server/internal/service/pet_service_test.go`（**新建**；mocked pet repo + tx manager（实际本接口**不**需要事务，仅作 wiring 占位）；≥4 个 case 覆盖 epics.md AC + r1 / r6 / r7 lessons）
    - `server/internal/service/pet_service_integration_test.go`（**新建**；dockertest 集成测试覆盖"创建 user + 默认 pet → POST /pets/current/state-sync {state: 3} → 验证 DB pets.current_state = 3 + updated_at 已变" + "再 POST {state: 1} → DB pets.current_state = 1" + "pet-less 账号路径（手动 DELETE pets 行）→ POST → HTTP 200 + DB 不变（pet 行不存在）"）
    - `server/internal/app/http/handler/pets_handler.go`（**新建**；PetsHandler struct + `PostStateSync(c *gin.Context)` 处理 POST /pets/current/state-sync 单字段请求体 + 业务码三层映射 + 业务事件 log）
    - `server/internal/app/http/handler/pets_handler_test.go`（**新建**；handler 单测，注入 stub PetService，验证 happy + 1001 / 1002 / 1005 / 1009 全部错误码响应 envelope；至少覆盖 4 个 case：happy state=2 / state 字段缺失 → 1002 / state=4 非法 → 1002 / service 返 1009 → 1009 envelope）
    - `server/internal/app/http/handler/pets_handler_integration_test.go`（**新建**；端到端集成测试覆盖：构造完整 router → POST /api/v1/pets/current/state-sync 带合法 Bearer token → 验证 envelope code=0 + data.state 字段 + DB pets.current_state 落库）
    - `server/internal/app/bootstrap/router.go`（**扩展**；wire `petService` / `petsHandler` + 在已认证子组挂 `authedGroup.POST("/pets/current/state-sync", petsHandler.PostStateSync)` —— 紧接现有 `/rooms/:roomId` 路由声明之后，与 §6.3 Pet 模块对齐）
    - 本 story 文件（Status 流转）+ sprint-status.yaml（14-2-post-pets-current-state-sync-接口-pets-current_state-更新: backlog → ready-for-dev → in-progress → review → done）
  - **不**改：
    - 任何 `docs/宠物互动App_*.md`（V1接口设计.md §5.2 / §12.3 / §3 / §2.5 / §1 是契约**输入**，本 story 严格对齐它们但**不修改**；同样不动数据库设计.md §5.3 / §6.4 / Go 项目结构.md §6.3 / iOS 工程结构.md / 时序图.md / 总体架构.md / MVP 节点规划.md）
    - 任何 ADR（ADR-0006 错误码三层映射 / ADR-0007 ctx 传播 / ADR-0011 ws stack 是契约**输入**，沿用不修改；本 story **不**新建 ADR —— 单接口实装 + 不引入新决策）
    - V1 接口契约（14.1 已冻结 §5.2 + §12.3 `### 宠物状态变更`）
    - migrations 0003（pets 表 + current_state 列已在 Story 4.3 落地；本 story 仅消费 schema，**不**改 SQL）
    - GORM `Pet` struct 字段定义（4.6 已就绪 + 11.x 系列不动 Pet struct；本 story 仅 import 后调用 + 给 PetRepo interface 加 method，**不**加字段 / 改 tag）
    - WS 网关 / Gateway / Session / SessionManager / SnapshotBuilder / BroadcastFn primitive（11.7 / 11.8 / 10.x 已稳定；本 story 仅在 service struct 预留 sessionMgr / broadcastFn 字段（nil-tolerant），不调任何 ws 包导出函数；14.4 才真正使用）
    - `room_service.go` / `room_handler.go`（11.3-11.8 已稳定，本 story 不动 —— 即便本 story service 层未来需要查 `users.current_room_id`，也是通过既有 `userRepo.FindByID(ctx, userID)` 拿 User struct 读 `CurrentRoomID` 字段，**不**调 room_service 任何方法）
    - `home_service.go` / `home_handler.go`（4.8 / 5.x / 11.10 已稳定；本 story 不影响 GET /home / GET /home.data.pet.currentState 字段读取路径 —— 那是另一个独立的 home_service.LoadHome 调用 `petRepo.FindDefaultByUserID` 拿 pet.CurrentState；本 story 写入 pets.current_state 后，home_service 自然能读到新值，但**不**需要本 story 修改 home_service）
    - `auth_service.go` / `auth_handler.go`（4.6 已稳定 + Auth 中间件 4.5 已稳定；本 story 不动）
    - `step_service.go` / `dev_step_service.go` / `step_handler.go`（7.3 / 7.4 / 7.5 已稳定）
    - 其他 epic 范围的 service / handler / repo（4.6 / 4.8 / 7.x / 11.x / Epic 5+ iOS 全部不动）
    - `_bmad-output/` 下其他 yaml / md（除自己的 story 文件 + sprint-status.yaml 流转 + 可能的新 lesson md）
  - **不**实装：
    - **Story 14.3 修改 RoomSnapshotBuilder**：`RoomSnapshotBuilder.BuildSnapshot` 内 `members[].pet.currentState` 从 placeholder `1` 切换为真实读 `pets.current_state` —— 这是 14.3 钦定范围
    - **Story 14.4 pet.state.changed WS 广播**：service 层 `s.broadcastPetStateChanged(ctx, userID, petID, state)` 调用 + 实装；本 story 仅预留 service 层 TODO 占位 + struct 字段 `sessionMgr` / `broadcastFn` 注入接口（nil-tolerant，单元测试 deps 不传 → 不广播；router.go wire 时传 nil 即可，14.4 才换成真实实例）
    - **WS envelope builder（pet.state.changed）**：14.4 才在 `server/internal/app/ws/snapshot.go`（与 `BuildMemberJoinedEnvelope` 同包同文件）新增 `BuildPetStateChangedEnvelope(payload PetStateChangedPayload) ([]byte, error)` + `PetStateChangedPayload` struct；本 story **不**新增任何 ws 包导出符号
    - **`users.current_room_id` 查询路径优化**：本 story service 层只查 `pets WHERE user_id=? AND is_default=1`，**不**查 users 表（不需要知道 user 是否在房间 —— 那是 14.4 广播触发判定才需要，本 story service 层 TODO 占位段引用 `userRepo.FindByID(ctx, userID).CurrentRoomID` 给 14.4 直接复用，**不**真的调用）
    - **多 pet 切换**（即 request 带 `petId` 字段让 client 指定操作哪个 pet）：节点 5 阶段每 user 单默认 pet（§5.3 `uk_user_default_pet` 唯一约束）；本接口请求体严格只 `{state: int}` 单字段；如后续 epic 需要由对应契约 story 决定
    - **dev 端点**（如 `/dev/force-set-pet-state`）：节点 5 阶段 epics.md 未规划 pet dev 端点；本 story 不引入
    - **GORM AutoMigrate**：禁用（与 ADR-0003 §3.2 同源；本 story 不动 GORM struct 字段）
    - **rate limit 特殊化**：本接口走默认 `60次/分`（按 Story 4.5 RateLimitByUserID 默认值），**不**新增独立限频策略；与 V1 §5.2 line 499 钦定一致
    - **WS 路径**：本 story 完全是 HTTP REST 接口，与 WS 完全无关；review 阶段如果出现 "ws / websocket / Gateway / SessionManager / broadcast 真实实现" 等关键词在本 story 实装层，必属范围越界（14.4 才该出现）
    - **跨 epic 接口**（如 GET /home 扩展 / RoomSnapshotBuilder 改造 / 表情接口 / 步数接口的扩展）—— 全部归属未来 story，本 story 严格红线

## Acceptance Criteria

**AC1 — `PetRepo` interface 扩展 `UpdateCurrentStateByID` 方法**

修改 `server/internal/repo/mysql/pet_repo.go`：

- `PetRepo` interface 新增方法签名（紧接既有 `FindDefaultByUserID` 之后）：

  ```go
  // UpdateCurrentStateByID 按主键更新 pets.current_state 列（Story 14.2 引入；
  // service 层先 FindDefaultByUserID 拿 pet.ID 后用主键定位更新）。
  //
  // **state 取值**：1 = rest, 2 = walk, 3 = run（与数据库设计 §6.4 + V1
  // §5.2 / §12.3 `### 宠物状态变更` 同义）；调用方（service 层）已确保 state ∈ {1,2,3}
  // （handler 层 1002 拦截在前）；本方法不重复校验入参枚举范围，仅做 SQL UPDATE。
  //
  // **更新字段**：current_state（显式 SET）+ updated_at（GORM autoUpdateTime 自动写
  // 当前时间，与数据库 §3.2 ON UPDATE CURRENT_TIMESTAMP(3) 语义一致）；**不**显式 SET
  // updated_at —— 让 GORM tag 处理避免与 ORM autoUpdateTime 双写冲突（参与 user_repo
  // .UpdateNickname / UpdateCurrentRoomID 同模式）。
  //
  // **err 二分**（V1 §5.2 line 532-537 + r6 lessons 锁定）：
  //   - err == nil → 成功（**不**读 RowsAffected）：service 层 一律视为成功，返 200 OK + code = 0
  //   - err != nil → 失败（driver / 网络 / 约束冲突 / 任何 DB 异常）：service 层包成 1009
  //
  // **不**读 RowsAffected：MySQL/GORM 语义下"同 user 同 state 重复上报"幂等场景的
  // `RowsAffected == 0` 是合法路径（V1 §5.2 关键约束 + r1 lessons + r6 实装锁定 + r9 sweep）；
  // 本接口的 UPDATE 把 updated_at 也写新值，理论上即便 current_state 未变 updated_at 仍变
  // → MySQL 通常仍报 RowsAffected == 1；但 GORM/driver 在某些 time-zone / 配置组合下
  // 可能仍返 0，service 层**不**依赖该值判断成功失败。
  //
  // ctx 用法（ADR-0007 §2.3）：本方法第一参数 ctx；GORM 调用 .WithContext(ctx)；
  // 本接口**不**入事务（数据库设计 §8.x 不含 state-sync 事务行；service 层不开 txMgr.WithTx）
  // —— 即便如此 repo 仍走 tx.FromContext(ctx, r.db) 模式（与 UpdateNickname /
  // UpdateBalance 一致），让本方法**未来若**被纳入事务（如多接口聚合）也能 ctx-aware
  // 无需改 repo signature。
  UpdateCurrentStateByID(ctx context.Context, petID uint64, state int8) error
  ```

- `petRepo.UpdateCurrentStateByID` impl（紧接既有 `FindDefaultByUserID` 之后）：

  ```go
  // UpdateCurrentStateByID 实装：用 Update("current_state", v) 单字段更新（参考
  // user_repo.UpdateNickname 模式 —— state 是 int8 不存在 nil-skip 陷阱，**不**需要
  // Updates(map[string]interface{}) 路径）。
  //
  // **关键**：用 db.Model(&Pet{}).Where("id = ?", petID).Update("current_state", state)
  // 而非 Save(&pet) —— Save 会写**全部**字段（含 created_at / pet_type / name /
  // is_default 等），可能引入并发数据丢失 / autoUpdateTime 行为差异。Update 单字段
  // 仅触发 SET current_state=?, updated_at=NOW() WHERE id=? + tag autoUpdateTime
  // 自动加 updated_at SET（与数据库 §3.2 一致）。
  func (r *petRepo) UpdateCurrentStateByID(ctx context.Context, petID uint64, state int8) error {
      db := tx.FromContext(ctx, r.db)
      return db.WithContext(ctx).
          Model(&Pet{}).
          Where("id = ?", petID).
          Update("current_state", state).Error
  }
  ```

- `server/internal/repo/mysql/pet_repo_test.go` 新增 ≥2 个 sqlmock case：
  - **happy**: `UPDATE pets SET current_state=?, updated_at=? WHERE id=?` SQL 模式命中（不解析具体 SQL 文本，用 `regexp.QuoteMeta` 转义 `WHERE id` 等关键字 + sqlmock `ExpectExec` matcher）+ args 严格匹配 `state, sqlmock.AnyArg() /* updated_at */, petID` → 返 `sqlmock.NewResult(0, 1)` → repo 返 nil error
  - **DB error**: `ExpectExec` 返 `errors.New("connection refused")` → repo 透传 raw error（与 PetRepo doc 一致：err != nil 透传给 service，service 包成 1009）；**禁止**新增"RowsAffected == 0 → 特殊错误"case（与 V1 + r6 + r9 锁定一致）

**AC2 — `PetService` interface + impl 新建**

新建 `server/internal/service/pet_service.go`：

- 包级 doc comment 必须显式标注：(a) 节点 5 / Epic 14 引入；(b) 范围红线（仅 `SyncCurrentState` 方法 + 不开事务 + 不调 WS 广播 + 14.4 才落地 WS 广播）；(c) sessionMgr / broadcastFn 字段为 14.4 预留，本 story 接受 nil 注入（router.go wire 时传 nil）
- `PetService` interface 声明：

  ```go
  // PetService 是 /api/v1/pets/* 路由的 service 层接口（Story 14.2 引入）。
  //
  // 节点 5 / Epic 14 范围：
  //   - Story 14.2（本 story）：SyncCurrentState —— UPDATE pets.current_state + pet-less noop
  //   - Story 14.4（future）：在 SyncCurrentState 成功路径加挂 pet.state.changed WS 广播
  //
  // **不**在本 story 落地：
  //   - GetCurrent（GET /pets/current；节点 5 阶段未规划，可能由 Story 14.6 / Epic 26 引入）
  //   - WS 广播实装（14.4 才落地，service struct 已预留 sessionMgr / broadcastFn 字段）
  type PetService interface {
      // SyncCurrentState 处理 POST /api/v1/pets/current/state-sync 业务（Story 14.2）。
      //
      // 流程（V1 §5.2 服务端逻辑 + 数据库设计 §5.3 / §6.4 + r6 / r7 lessons 锁定）：
      //  1. petRepo.FindDefaultByUserID(ctx, userID) 查默认 pet：
      //     - ErrPetNotFound（pet-less，V1 §5.2 line 530-531 + r7 钦定**合法 edge case**） →
      //       **server-acknowledged noop 路径**：跳 UPDATE + 跳广播 + 直接返
      //       SyncCurrentStateOutput{State: in.State}（回显入参），nil error
      //     - 其他 DB 异常 → apperror.Wrap(err, ErrServiceBusy, ...)（1009）
      //     - happy → 进步骤 2
      //  2. petRepo.UpdateCurrentStateByID(ctx, pet.ID, in.State)：
      //     - **err == nil** → 进步骤 3（**禁止**读 RowsAffected；r6 / r9 实装锁定）
      //     - **err != nil** → apperror.Wrap(err, ErrServiceBusy, ...)（1009）
      //  3. **broadcast trigger TODO（Story 14.4）**：检查 users.current_room_id 非 null →
      //     调 s.broadcastPetStateChanged(ctx, userID, pet.ID, in.State) （fire-and-forget）；
      //     本 story **不**实装 —— service struct 字段 sessionMgr / broadcastFn 先 nil 注入，
      //     该位置仅留 // TODO(Story 14.4) 注释 + 单测覆盖"广播路径未被调用"。**禁止**真的
      //     调用任何 ws 包导出函数 / BroadcastFn。
      //  4. 返 SyncCurrentStateOutput{State: in.State}（回显入参，**不**读 DB pet.current_state
      //     反推 —— ack-only 信号，r2 + r10 lessons 锁定 ack 不入权威等价桶）
      //
      // **不**走事务：仅 1 个 SELECT + 1 个 UPDATE 单语句（DB 引擎默认 autocommit）；
      // 与 11.3 CreateRoom 的 4 步事务不同（参见 V1 §5.2 服务端逻辑"事务边界规则"段）。
      //
      // 错误约定（ADR-0006 三层映射）：
      //   - pet-less（ErrPetNotFound）→ **不**包成 error，走 noop 返 (output, nil)（r7 锁定）
      //   - 其他 DB 异常 → apperror.Wrap(err, ErrServiceBusy, ...)（1009）
      //   - **不**触发 ErrResourceNotFound (1003)（r7 锁定；1003 仍在 §3 全局表保留但本接口不触发）
      SyncCurrentState(ctx context.Context, in SyncCurrentStateInput) (*SyncCurrentStateOutput, error)
  }

  // SyncCurrentStateInput 是 service 层 DTO（**不是** wire DTO；handler 转换）。
  //
  // State 字段范围 [1,3] 由 handler 层校验 + 1002 拦截；service 层入参假设已校验。
  type SyncCurrentStateInput struct {
      UserID uint64
      State  int8 // 1 = rest, 2 = walk, 3 = run
  }

  // SyncCurrentStateOutput 是 service 层 DTO；handler 翻译成 V1 §5.2 wire DTO。
  //
  // State 字段是回显入参（ack-only 信号，V1 §5.2 响应体 + r2 / r10 lessons 锁定 ack
  // 不入权威等价桶）；service 层**不**重新查 DB pet.current_state 反推。
  type SyncCurrentStateOutput struct {
      State int8
  }
  ```

- `petServiceImpl` struct + `NewPetService(petRepo mysql.PetRepo, userRepo mysql.UserRepo, sessionMgr ws.SessionManager, broadcastFn ws.BroadcastFn) PetService` 构造器：
  - **userRepo 字段**：为 14.4 预留（14.4 落地时 `broadcastPetStateChanged` 内部需要查 `users.current_room_id`）；本 story service 层**不**调用 userRepo —— 但**禁止**省略字段定义，因为下游 14.4 实装需要它，本 story 提前 wire 进 service struct 让 14.4 落地仅需补方法实装，不动 service 构造函数 / router.go wire；**注**：构造函数参数顺序遵循 11.8 NewRoomService 同模式（repo 在前，sessionMgr / broadcastFn 在后）；测试场景 deps 不全时单测构造 `NewPetService(petRepo, userRepo, nil, nil)` 即可（与 router.go zero-deps 路径一致）
  - **sessionMgr / broadcastFn 字段**：14.4 预留；本 story 不调用；router.go wire 时**先**传 nil 即可（与 11.3 落地时未挂广播但 11.8 才挂的模式一致 —— 当时 service struct 字段也是先 nil 注入）；本 story **不**让 NewPetService 函数体内做 nil-check fail-fast（与 11.8 NewRoomService 不做 nil-check 同模式 —— 字段为 nil 时调用方就不会调到广播路径，不存在 nil pointer panic 风险）
- `SyncCurrentState` impl 严格按 PetService doc comment 流程：

  ```go
  func (s *petServiceImpl) SyncCurrentState(ctx context.Context, in SyncCurrentStateInput) (*SyncCurrentStateOutput, error) {
      // (1) 查默认 pet
      pet, err := s.petRepo.FindDefaultByUserID(ctx, in.UserID)
      if err != nil {
          if stderrors.Is(err, mysql.ErrPetNotFound) {
              // pet-less 路径（r7 锁定）：跳 UPDATE + 跳广播 + 返 server-acknowledged noop
              // **不** log error（pet-less 是 contract-valid 状态，不是 invariant 损坏）；
              // 可 log info 级 "pet-less state-sync noop" 作可观测性（与 V1 §5.2 line 531 一致）
              slog.InfoContext(ctx, "pet-less state-sync noop",
                  slog.Uint64("userId", in.UserID),
                  slog.Int("state", int(in.State)))
              return &SyncCurrentStateOutput{State: in.State}, nil
          }
          // 其他 DB 异常 → 1009
          return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
      }

      // (2) UPDATE pets.current_state
      // **err 二分锁定**（V1 §5.2 line 532-537 + r6 lessons）：
      //   - err == nil → 成功（**不**读 RowsAffected；r1 / r6 / r9 锁定）
      //   - err != nil → 1009
      if err := s.petRepo.UpdateCurrentStateByID(ctx, pet.ID, in.State); err != nil {
          return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
      }

      // (3) TODO(Story 14.4): broadcast pet.state.changed if users.current_room_id != NULL
      //
      // 实装方向（14.4 接管，本 story **不**写代码）：
      //   - user, err := s.userRepo.FindByID(ctx, in.UserID)  // 拿 user.CurrentRoomID *uint64
      //   - if err == nil && user.CurrentRoomID != nil {
      //       go s.broadcastPetStateChanged(detachedCtx, *user.CurrentRoomID, in.UserID, pet.ID, in.State)
      //     }
      //   - 注意 fire-and-forget 严格语义（与 11.8 broadcastMemberJoined 同模式）：
      //     广播失败仅 log warn，**不**回滚 UPDATE，**不**影响 HTTP 200 响应
      //   - envelope schema（14.1 锚定 V1 §12.3 `### 宠物状态变更`）：
      //     type = "pet.state.changed", requestId = "", payload = {userId, petId, currentState}, ts = server now ms
      //   - 广播范围：包含发起者自己（V1 §12.3 关键约束 + §1 节点 5 冻结声明；与 member.joined / member.left 排除发起者不同）
      //   - 14.4 wire 后 NewPetService 第 3/4 参数（sessionMgr / broadcastFn）传真实实例，本 story 传 nil
      //
      // 本 story service 不调用 ws 包任何导出函数；不读 users.current_room_id；
      // 单测覆盖"广播路径未被触发"（sessionMgr / broadcastFn 字段 nil 时也不 panic）

      // (4) 返回 ack 信号（回显入参）
      return &SyncCurrentStateOutput{State: in.State}, nil
  }
  ```

**AC3 — `PetService` 单测（≥4 case，mocked PetRepo / UserRepo）**

新建 `server/internal/service/pet_service_test.go`，覆盖以下 case（与 epics.md §Story 14.2 AC + r1 / r6 / r7 lessons 对齐）：

- **case 1 — happy state=2**（state ∈ {1,2,3} happy path）：mock petRepo.FindDefaultByUserID 返 `&Pet{ID: 100, ...}` / mock petRepo.UpdateCurrentStateByID 返 nil → 调 SyncCurrentState({UserID: 10, State: 2}) → 验证返回 `&SyncCurrentStateOutput{State: 2}` + nil error + mock 调用次数（FindDefaultByUserID 1 次 / UpdateCurrentStateByID 1 次带 petID=100, state=2）；**额外断言**：mock sessionMgr / broadcastFn **未被调用**（本 story 不广播；14.4 才触发）
- **case 2 — pet-less noop**（V1 §5.2 line 530-531 + r7 lessons）：mock petRepo.FindDefaultByUserID 返 mysql.ErrPetNotFound → 调 SyncCurrentState({UserID: 10, State: 3}) → 验证返回 `&SyncCurrentStateOutput{State: 3}` (回显入参) + nil error + mock 调用次数（FindDefaultByUserID 1 次 / UpdateCurrentStateByID **0 次** —— 这是 noop 路径的关键校验）；**断言禁止**：不验证任何 errors.Is + apperror.Code，因为 pet-less 走 noop 路径 nil error
- **case 3 — DB 异常（FindDefaultByUserID 返其他 raw error）**：mock petRepo.FindDefaultByUserID 返 `errors.New("connection refused")` → 调 SyncCurrentState → 验证返回 nil output + apperror.Code(err) == ErrServiceBusy (1009) + UpdateCurrentStateByID **0 次调用**
- **case 4 — DB 异常（UpdateCurrentStateByID 返 raw error）**：mock petRepo.FindDefaultByUserID 返 `&Pet{ID: 100}` / mock petRepo.UpdateCurrentStateByID 返 `errors.New("deadlock")` → 调 SyncCurrentState → 验证返回 nil output + apperror.Code(err) == ErrServiceBusy (1009) + FindDefaultByUserID 1 次 + UpdateCurrentStateByID 1 次（**确认调用发生但失败**，不被跳过）
- **case 5（可选 ≥4 要求外）— 幂等同 state 重复上报**（V1 §5.2 line 500 元信息表 + 服务端逻辑步骤 4 + r1 lessons）：mock petRepo.UpdateCurrentStateByID 返 nil（即便业务上"同 user 同 state 重复上报"也 mock nil，**禁止**为该 case 让 mock 返 `RowsAffected == 0` —— service 层不读 RowsAffected，case 与 case 1 行为完全等价）→ 调 SyncCurrentState({UserID: 10, State: 2}) 连续 2 次 → 两次都返 `&SyncCurrentStateOutput{State: 2}` + nil error
- **case 6（可选 ≥4 要求外）— 广播路径未被触发的 wire 占位**：用 mock broadcastFn / mock sessionMgr 注入 NewPetService → 调 SyncCurrentState happy → 验证 broadcastFn / sessionMgr 任何方法 **0 次调用**（本 story 不广播；14.4 单测才覆盖广播路径）

**所有 case 必须**：
- 用 testify/mock 或自定义 stub 实装 PetRepo / UserRepo / ws.SessionManager / ws.BroadcastFn（与 step_service_test.go / room_service_test.go 同模式；**优先 stub** 而非 testify/mock，与 server 端既有测试风格一致）
- 显式断言"未读 RowsAffected"语义（通过"mock UpdateCurrentStateByID 不接受 RowsAffected 参数"间接保证 —— interface 签名就没有该参数，service 层无法读）+ "pet-less 不触发 1003"语义（通过 apperror.Code(err) == 0 或 err == nil 显式断言）

**AC4 — `PetService` 集成测试（dockertest）**

新建 `server/internal/service/pet_service_integration_test.go`，覆盖：

- **场景 1 — 创建 user + 默认 pet（current_state=1）→ POST /pets/current/state-sync {state: 3} → DB pets.current_state = 3**：
  - 用 dockertest 启 MySQL container + 跑 migrations（pets 表 + users 表 + user_step_accounts 等全部依赖；与 7-3 / 11-3 集成测试 fixture 同模式 —— 复用 server/internal/repo/mysql/integration_test_helper.go 或类似已有 fixture）
  - 用 authSvc.firstTimeLogin 或直接 INSERT 创建一个 user + 默认 pet（current_state=1, is_default=1）
  - 调 PetService.SyncCurrentState(ctx, {UserID: <new user.id>, State: 3})
  - 验证 SyncCurrentStateOutput.State == 3
  - 验证 DB `SELECT current_state, updated_at FROM pets WHERE user_id=?` → current_state = 3，updated_at 已变（>原值）
- **场景 2 — 同 user 再 POST {state: 1} → DB pets.current_state = 1**（幂等性 + 反向切换）：
  - 接场景 1 后立即调 SyncCurrentState({UserID, State: 1}) → DB pets.current_state = 1
- **场景 3 — pet-less 账号路径（DELETE pet 行 → POST → HTTP 200 + DB 不变）**：
  - 接场景 2 后手动 `DELETE FROM pets WHERE user_id=?` 清掉 pet 行（模拟 pet-less 账号）
  - 调 SyncCurrentState({UserID, State: 2})
  - 验证 SyncCurrentStateOutput.State == 2（回显入参，server-acknowledged noop）
  - 验证 DB `SELECT COUNT(*) FROM pets WHERE user_id=?` == 0（pet 行确实不存在，本接口未重新创建）
  - 验证 err == nil（**断言不为 apperror.ErrResourceNotFound** —— r7 锁定）
- **场景 4 — happy 路径回 envelope 三层映射端到端**（接入 handler + router 全链路）：放在 `pets_handler_integration_test.go` 而非 service 集成测试

**fixture 复用约束**：
- 必须**复用** server/internal/repo/mysql/ 既有 dockertest 启动 helper（如 startMySQL / setupRepoTestDB / setupIntegrationDB 等 —— 见现有 `*_integration_test.go` 文件）；**禁止**新建独立 dockertest 启动函数
- 必须**跳过 short**：`if testing.Short() { t.Skip("skip integration test in short mode") }`（与既有 integration 测试一致）

**AC5 — `PetsHandler` 新建 + 错误码三层映射**

新建 `server/internal/app/http/handler/pets_handler.go`：

- 包级 doc comment：节点 5 / Epic 14 引入 + 范围红线（仅 PostStateSync）；未来 Story 14.6 / Epic 26 可能加 GetCurrent
- `PostStateSyncRequest` struct（与 V1 §5.2 请求体 1:1 对齐）：

  ```go
  // PostStateSyncRequest 是 V1 §5.2 钦定请求体的 Go mirror。
  //
  // **State 用 *int8 指针类型**（与 7.3 PostSyncRequest.MotionState 同模式 + r2 lessons）：
  //   - V1 §5.2 规定 state 字段 required
  //   - 若用值类型 int8，client 缺字段 JSON 解析为 zero value（0），与显式传 0 无法区分
  //     → 漏掉的 state-sync 会被静默接受为 "state=0" → handler 范围校验 [1,3] 拦截
  //     时给出错误信息 "state 必须是 1 / 2 / 3"，但**真实场景是字段缺失** —— 用指针类型 +
  //     handler 显式 `if x == nil` 校验，能拦截"字段缺失"并给出更精确的错误信息 "state 必填"
  //   - 不用 binding:"required"（validator/v10 在数值字段上把 0 视为缺失 —— 与 motionState
  //     同 trap，详见 7.3 PostSyncRequest 注释）
  //
  // JSON tag 严格对齐 V1 §5.2（camelCase；state 字段就是 "state"）。
  type PostStateSyncRequest struct {
      State *int8 `json:"state"` // 指针：区分缺失与显式 0
  }
  ```

- `PostStateSync` handler 实装：

  ```go
  func (h *PetsHandler) PostStateSync(c *gin.Context) {
      var req PostStateSyncRequest
      if err := c.ShouldBindJSON(&req); err != nil {
          _ = c.Error(apperror.Wrap(err, apperror.ErrInvalidParam, apperror.DefaultMessages[apperror.ErrInvalidParam]))
          return
      }

      // === required 字段缺失校验（V1 §5.2 钦定 required；指针 nil → 字段未传 → 1002）===
      if req.State == nil {
          _ = c.Error(apperror.New(apperror.ErrInvalidParam, "state 必填"))
          return
      }

      // === state ∈ {1, 2, 3} 校验（V1 §5.2 + 数据库设计 §6.4 + r9 sweep）===
      if *req.State < 1 || *req.State > 3 {
          _ = c.Error(apperror.New(apperror.ErrInvalidParam, "state 必须是 1 / 2 / 3"))
          return
      }

      // 从 auth 中间件取 userID（与 home_handler.LoadHome / steps_handler.PostSync / room_handler.* 同模式）
      v, ok := c.Get(middleware.UserIDKey)
      if !ok {
          // unreachable: Auth 中间件挂在前；保险兜底走 1009
          _ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
          return
      }
      userID, ok := v.(uint64)
      if !ok {
          _ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
          return
      }

      out, err := h.svc.SyncCurrentState(c.Request.Context(), service.SyncCurrentStateInput{
          UserID: userID,
          State:  *req.State,
      })
      if err != nil {
          _ = c.Error(err) // service 已 wrap *AppError；ErrorMappingMiddleware 写 envelope
          return
      }

      response.Success(c, postStateSyncResponseDTO(out), "ok")
  }

  // postStateSyncResponseDTO 把 service 输出转成 V1 §5.2 wire 格式 `data: {state: int}`。
  func postStateSyncResponseDTO(out *service.SyncCurrentStateOutput) gin.H {
      return gin.H{
          "state": out.State,
      }
  }
  ```

- **不**直接调 response.Error 写 envelope（ADR-0006 单一 envelope 生产者；与 4.6 / 4.8 / 7.3 / 11.3 同模式）
- 错误码三层映射：handler 层 1001 / 1002 / 1005 在 middleware 层处理（auth / rate_limit / ShouldBindJSON）；1002 由 handler 手动校验补；1009 由 middleware 兜底 + service 层显式 wrap；**禁止** handler 层直接产 1003 / 3xxx / 4xxx / 5xxx / 6xxx / 7xxx（V1 §5.2 错误码表注 + r7 锁定）

**AC6 — `PetsHandler` 单测（≥4 case）**

新建 `server/internal/app/http/handler/pets_handler_test.go`，注入 stub PetService，验证：

- **case 1 — happy state=2**：stub PetService.SyncCurrentState 返 `&SyncCurrentStateOutput{State: 2}, nil` → POST /api/v1/pets/current/state-sync `{"state": 2}` → 200 OK + envelope `{code:0, message:"ok", data:{state:2}, requestId:"..."}` + stub 调用次数 1 + UserID 透传正确（用预设 user_id key 验证）
- **case 2 — state 字段缺失 → 1002**：POST 空 body `{}` 或 `{}` → 400 / 1002 envelope `{code:1002, message:"state 必填"}` + stub 调用次数 0
- **case 3 — state=4 非法 → 1002**：POST `{"state": 4}` → 400 / 1002 envelope `{code:1002, message:"state 必须是 1 / 2 / 3"}` + stub 调用次数 0
- **case 4 — service 返 1009 → 1009 envelope**：stub PetService.SyncCurrentState 返 `nil, apperror.New(ErrServiceBusy, "服务繁忙")` → POST `{"state": 2}` → 500 / 1009 envelope `{code:1009, message:"服务繁忙"}` + stub 调用次数 1
- **case 5（可选 ≥4 要求外）— state=0 非法 → 1002**（边界值校验 + 指针类型 zero-value-vs-missing 区分）：POST `{"state": 0}` → 1002 envelope `{code:1002, message:"state 必须是 1 / 2 / 3"}`（与"字段缺失 → 1002"区分：缺失的 message 是"state 必填"，0 的 message 是"必须是 1 / 2 / 3"，handler 区分能力来自指针类型 + 顺序校验）

**所有 case 必须**：
- 用 httptest.NewRequest + httptest.NewRecorder + gin engine（与既有 handler_test.go 同模式）
- 用 stub PetService 实装 PetService interface（与 step_service stub / room_service stub 同模式）
- userID 注入：在 stub 路由前 `c.Set(middleware.UserIDKey, uint64(10))` 模拟 auth 中间件已通过
- 端到端验证 envelope JSON（code / message / data / requestId 字段都有；data.state 字段在 happy case 验证正确）

**AC7 — `PetsHandler` 集成测试（dockertest + 完整 router）**

新建 `server/internal/app/http/handler/pets_handler_integration_test.go`，覆盖：

- **场景 1 — 完整 router happy 路径**：
  - 用 dockertest 启 MySQL + 跑 migrations + 创建 user + 默认 pet（current_state=1）
  - 构造完整 router（与 router_test.go 现有 fixture 复用；通过 bootstrap.Deps 传入真实 GormDB / TxMgr / Signer / SessionMgr 等）
  - 用 jwtSigner 生成合法 Bearer token
  - POST /api/v1/pets/current/state-sync `{"state":3}` 带 `Authorization: Bearer <token>` header
  - 验证 HTTP 200 + envelope `{code:0, message:"ok", data:{state:3}, requestId:"..."}` + DB pets.current_state = 3
- **场景 2 — 未带 token → 1001**：POST 无 Authorization header → 401 / 1001
- **场景 3 — state=4 → 1002**：POST `{"state":4}` 带合法 token → 400 / 1002
- **场景 4 — pet-less 路径端到端**：DB 先 DELETE FROM pets WHERE user_id=? → POST `{"state":2}` 带 token → HTTP 200 + envelope data.state=2 + DB 仍 0 行 pets

**fixture 复用约束同 AC4**：复用既有 integration helper；testing.Short skip；**禁止**复制现有 router_test.go 全文，仅扩展现有 fixture 加 pets 路由测试

**AC8 — bootstrap router wire**

修改 `server/internal/app/bootstrap/router.go`：

- 在现有 `if deps.GormDB != nil && deps.TxMgr != nil && deps.Signer != nil { ... }` 块内：
  - 新增 PetService 构造：`petSvc := service.NewPetService(petRepo, userRepo, deps.SessionMgr, nil)`（**broadcastFn 先 nil**，14.4 才换成 `roomBroadcastFn` 或新建独立 closure；sessionMgr 复用已 wire 的 `deps.SessionMgr` —— 与 11.8 roomService wire 时复用 SessionMgr 同模式；本 story sessionMgr 为 nil-tolerant（router_test.go zero-deps 路径需要兼容））
  - 新增 PetsHandler 构造：`petsHandler := handler.NewPetsHandler(petSvc)`
  - 在 `authedGroup` 已有路由声明之后追加：`authedGroup.POST("/pets/current/state-sync", petsHandler.PostStateSync) // Story 14.2 加`（**位置**：在现有 `/rooms/...` 路由声明之后；与 §6.3 Pet 模块对齐）
- **不**改 `Deps` struct 字段（本 story 不引入新依赖；petRepo / userRepo / sessionMgr 都已 wire；broadcastFn 14.4 再决定如何 wire）
- **不**改 WS 路由块（`deps.SessionMgr != nil` 内的 `r.GET("/ws/rooms/:roomId", gateway.Handle)` 不动）
- **不**改 dev_step / dev_chest / dev_compose 等 dev 路由

**AC9 — Router test 扩展**

修改 `server/internal/app/bootstrap/router_test.go`：

- 既有 router_test.go fixture 应该自动覆盖 `POST /pets/current/state-sync` 路由的"未挂时"行为；本 story 新增路由后**预期**既有测试不破（路由不在 deps 完整 fixture 内测试的话）
- **不**新增独立的 router_pets_test.go（与 router_dev_test.go / router_ws_test.go 分离原则一致，但 pets 路由是普通 authed 路由，没有特殊 wire 条件 —— 直接 fall into 既有 GormDB / TxMgr / Signer 完整 fixture 即可）
- 如发现既有 router_test.go 缺少"完整 deps wire → 列举所有 authed 路由 path"检查，本 story **不**新增此类断言（与 11.3 同模式 —— 路由清单不属本 story 范围）
- 如新增 router_test.go 任何 case，必须保持既有断言不破

**AC10 — 业务事件 log + 可观测性**

PetService.SyncCurrentState + PetsHandler.PostStateSync 必须按 slog 业务事件 log 规范产日志（与 step_service / room_service 同模式）：

- **happy 路径**：service info "pet state-sync succeeded" + 字段 userId / petId / state（**petId** 字段在 pet-less noop 路径下省略）；handler 不重复 log（middleware Logging 已记录 request / response）
- **pet-less noop**：service info "pet-less state-sync noop" + 字段 userId / state；**不**log error / warn（pet-less 是合法路径）
- **DB 异常**：service 不主动 log error（由 apperror.Wrap 链路 + middleware ErrorMapping 兜底 log，与现有 step_service / room_service 一致）
- **不**新增任何 prometheus metric / counter（节点 5 阶段未新增可观测性需求；与 7.3 / 11.3 同模式）

**AC11 — `bash scripts/build.sh --test` 全绿**

完成本 story 全部代码改动后必须：

- 跑 `bash scripts/build.sh --test`（go vet + go build + 全量单元测试 `go test -count=1 ./...`）
- 全部 PASS（0 错误 + 0 warnings）+ 所有新增单测命中预期断言 + 既有测试不破
- 跑 `bash scripts/build.sh --integration`（`-tags=integration` 仅跑集成测试）覆盖本 story 新增的 pet_service_integration_test.go + pets_handler_integration_test.go
- 跑 `bash scripts/build.sh --race --test`（go test -race -count=1 ./...）—— race detector 必须不报警告

**AC12 — 跨 epic 一致性自检**

完成实装后跑以下抽测验证范围红线遵守 + 跨章节 schema 对齐：

- **代码范围 grep**：`git status --short` 显示**仅**命中本 AC1 / AC2 / AC5 / AC7 / AC8 钦定的文件清单 +本 story 文件 + sprint-status.yaml；**未**命中任何 `docs/宠物互动App_*.md` / `_bmad/` 配置 / 其他 `_bmad-output/` yaml/md（除自己 + lessons 新增可能）/ `iphone/` / `ios/` / `server/internal/app/ws/` 既有文件 / `server/migrations/` 任何 SQL
- **错误码全局对齐**：handler / service grep `1003` / `ErrResourceNotFound` 不应命中本 story 任何路径（pet-less 走 noop 而非 1003，r7 锁定）；grep `RowsAffected` 不应命中本 story 任何 service / repo 路径（r1 / r6 / r9 锁定）
- **WS 实装范围**：service grep `ws\.BroadcastFn` / `ws\.SessionManager` / `BroadcastToRoom` 在本 story service 层**只**应出现在 struct 字段定义（预留 14.4 用）+ TODO 占位注释 + 单测 stub —— **不**应在任何 service method body 内被实际调用（如发现调用，必属范围越界，应回滚至 14.4 实装）
- **DB 列对齐**：grep `pets.current_state` / `current_state` 在本 story repo / service 路径**只**应命中本 AC1 引入的 `UpdateCurrentStateByID` 方法 + 单测 / 集成测试 SELECT 验证 SQL；**不**应触发任何 schema 变更（migrations 0003 已就绪，pets 表 + current_state 列存在）
- **下游 story 引用检查**：grep `Story 14\.3` / `Story 14\.4` / `Epic 15` 在本 story 文件应命中下游 story 引用 + TODO 占位注释（在 service code 中以 `TODO(Story 14.4)` 形式）；不应命中"实际依赖 14.3 / 14.4 已 done" 性质的硬依赖（14.3 / 14.4 仍 backlog）

**关键约束**：

- 本 story 的 dev 阶段**不** commit（epic-loop 流水线设计是 dev-story 阶段不 commit，由下游 fix-review / story-done sub-agent 统一收口）
- commit message 模板（Task 8 落地）：

```text
feat(pet): 实装 POST /pets/current/state-sync 接口 + pets.current_state UPDATE

- internal/repo/mysql/pet_repo.go: PetRepo interface 扩展 UpdateCurrentStateByID 方法 + sqlmock 单测
- internal/service/pet_service.go: 新建 PetService + SyncCurrentState 方法
  - pet-less 走 server-acknowledged noop 路径（r7 lessons）
  - UPDATE err 二分锁定：err == nil ⇒ 200 OK / err != nil ⇒ 1009；不读 RowsAffected（r1/r6/r9 lessons）
  - WS 广播 TODO 占位（Story 14.4 接管，本 story 不实装；service struct sessionMgr / broadcastFn 字段预留 nil 注入）
- internal/service/pet_service_test.go: ≥4 case mock 单测（happy / pet-less noop / DB 异常 x 2）
- internal/service/pet_service_integration_test.go: dockertest 集成测试覆盖 epics.md §Story 14.2 + pet-less 端到端
- internal/app/http/handler/pets_handler.go: PostStateSync handler + PostStateSyncRequest struct（State *int8 指针避免 zero-value-vs-missing trap）
- internal/app/http/handler/pets_handler_test.go: ≥4 case handler 单测（happy / state 缺失 / state=4 / 1009）
- internal/app/http/handler/pets_handler_integration_test.go: 端到端集成测试
- internal/app/bootstrap/router.go: wire petService + petsHandler + authedGroup.POST("/pets/current/state-sync")

实装严格按 V1 §5.2 + §12.3 `### 宠物状态变更` (14.1 已 frozen) +
docs/lessons/2026-05-12-*-14-1-* (r1/r6/r7 锁定) +
epics.md §Story 14.2 钦定 AC（pet-less 改走 noop，14.1 r7 reversed）+
docs/宠物互动App_Go项目结构与模块职责设计.md §6.3 Pet 模块。

下游 Story 14.3 / 14.4 / iOS Epic 15 全部 stories 基于本 story 接口展开。

Story: 14-2-post-pets-current-state-sync-接口-pets-current_state-更新
```

## Tasks / Subtasks

- [x] **Task 0 — pre-flight read**：本 story 引用的 docs / V1 doc / 数据库设计 / Go 项目结构 / 14-1 story / 14-1 lessons / 上游同模式 story（4.6 / 7.3 / 11.3）必须**全部**先读完再写代码
  - [x] 0.1 读 `docs/宠物互动App_总体架构设计.md`（首次 session 必读）+ `docs/宠物互动App_MVP节点规划与里程碑.md` §3 / §5（节点顺序）
  - [x] 0.2 读 `docs/宠物互动App_V1接口设计.md` §1 / §3 / §5.2 / §12.3 `### 宠物状态变更（pet.state.changed）`（14.1 已 frozen，**本 story 必读**契约）
  - [x] 0.3 读 `docs/宠物互动App_数据库设计.md` §5.3 pets / §6.4 pets.current_state / §8.x（确认本接口**不**入事务列表）
  - [x] 0.4 读 `docs/宠物互动App_时序图与核心业务流程设计.md` §13 / 行 555（pet.state.changed 业务消息触发点）
  - [x] 0.5 读 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 项目目录 / §5 分层职责 / §6.3 Pet 模块
  - [x] 0.6 读 `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md` 整文件（理解 14.1 frozen contract + r1-r11 解决的所有问题 + 全部已 archived 的设计陷阱）
  - [x] 0.7 读 `docs/lessons/2026-05-12-*-14-1-*.md` 全部 11 个 lessons（r1-r11；按顺序读，理解每条 lesson 的"原始踩坑" + "修正方向"，本 story 落地时**不**重新踩同样的坑）
  - [x] 0.8 读 `_bmad-output/implementation-artifacts/4-6-游客登录接口-首次初始化事务.md`（同模式：epic 第一条业务实装 story，含 service / handler / repo / router 全栈 + 单测 + 集成测试）
  - [x] 0.9 读 `_bmad-output/implementation-artifacts/7-3-post-steps-sync-接口-累计差值入账-service.md`（同模式：单接口 + handler 字段 *int8 指针避免 zero-value trap + service 层 err 处理 + dockertest 集成测试）
  - [x] 0.10 读 `_bmad-output/implementation-artifacts/11-3-创建房间事务.md`（同模式：单接口 + service / handler / repo / router 全栈；本 story 范围更小因为单 UPDATE 不开事务）
  - [x] 0.11 读 `_bmad-output/implementation-artifacts/decisions/0006-error-handling.md`（ADR-0006 错误码三层映射） + `0007-context-propagation.md`（ADR-0007 ctx 传播）
  - [x] 0.12 抽读 `server/internal/service/room_service.go` 的 `broadcastMemberJoined` 方法（11.8 实装，理解 fire-and-forget 模式 + envelope build 调用 + sessionMgr / broadcastFn 注入），让本 story service 层 TODO 占位的注释可以精确引用未来 14.4 实装方向

- [x] **Task 1（AC1）— PetRepo.UpdateCurrentStateByID 方法新增 + 单测**
  - [x] 1.1 修改 `server/internal/repo/mysql/pet_repo.go`：在 `PetRepo` interface 既有 `FindDefaultByUserID` 方法之后新增 `UpdateCurrentStateByID(ctx context.Context, petID uint64, state int8) error` 方法签名 + 完整 doc comment（含 err 二分锁定 + 不读 RowsAffected + state 取值与 §6.4 同义 + ctx 用法 ADR-0007 §2.3 + 未来事务化 ctx-aware 兼容性说明）
  - [x] 1.2 新增 `petRepo.UpdateCurrentStateByID` impl：`db.WithContext(ctx).Model(&Pet{}).Where("id = ?", petID).Update("current_state", state).Error`（注意是 `Update` 单字段而非 `Updates` map 路径 —— state 是 int8 不存在 nil-skip 陷阱；与 user_repo.UpdateNickname 同模式；GORM autoUpdateTime tag 自动写 updated_at）
  - [x] 1.3 修改 `server/internal/repo/mysql/pet_repo_test.go`：新增 `TestPetRepo_UpdateCurrentStateByID_Happy` + `TestPetRepo_UpdateCurrentStateByID_DBError` 2 个 sqlmock case（参考既有 `TestPetRepo_Create_Happy` / `TestPetRepo_FindDefaultByUserID_NotFound` 模式 + 上游 user_repo_test.go `TestUserRepo_UpdateCurrentRoomID_*` 模式）；**禁止**新增 RowsAffected 相关 case

- [x] **Task 2（AC2, AC3）— PetService interface + impl + 单测**
  - [x] 2.1 新建 `server/internal/service/pet_service.go`：包级 doc comment + `PetService` interface + `SyncCurrentStateInput` / `SyncCurrentStateOutput` DTO + `petServiceImpl` struct（字段 petRepo / userRepo / sessionMgr / broadcastFn）+ `NewPetService` 构造器（4 参数；userRepo / sessionMgr / broadcastFn 为 nil-tolerant）+ `SyncCurrentState` impl（步骤 1 查 pet → pet-less noop / 步骤 2 UPDATE / 步骤 3 TODO 占位 / 步骤 4 返回 ack）
  - [x] 2.2 新建 `server/internal/service/pet_service_test.go`：6 case 覆盖 AC3 钦定（happy / pet-less noop / DB error x 2 + 可选 idempotent / broadcast-not-invoked 增量）
  - [x] 2.3 用 stub 实装 PetRepo（手写 stub struct，与 server 端既有测试风格一致）
  - [x] 2.4 用 stub 实装 UserRepo / ws.BroadcastFn closure（sessionMgr 直接传 nil 验证 nil-tolerant；测试场景不调任何 ws 包导出函数）
  - [x] 2.5 显式断言 "未读 RowsAffected"（通过 PetRepo interface 签名间接保证）+ "pet-less 不触发 1003"（通过 err == nil + apperror.Code(err) == 0 显式断言）

- [x] **Task 3（AC4）— PetService 集成测试（dockertest）**
  - [x] 3.1 新建 `server/internal/service/pet_service_integration_test.go`：复用既有 startMySQL / runMigrations helper（auth_service_integration_test.go 落地）；build tag `integration` 隔离
  - [x] 3.2 场景 1 + 场景 2：创建 user + 默认 pet → state=3 → DB 验证 → state=1 → DB 验证 + updated_at 单调递增
  - [x] 3.3 场景 3：DELETE pets 行 → SyncCurrentState state=2 → 验证 err == nil + apperror.Code(err)==0 + DB pets 仍 0 行（pet-less noop 路径）

- [x] **Task 4（AC5, AC6）— PetsHandler 新建 + 单测**
  - [x] 4.1 新建 `server/internal/app/http/handler/pets_handler.go`：`PetsHandler` struct + `NewPetsHandler` 构造器 + `PostStateSyncRequest` struct（State *int8 指针）+ `PostStateSync` handler 实装（按 AC5 完整步骤：ShouldBindJSON / required 字段校验 / state ∈ {1,2,3} 校验 / 取 userID / 调 service / response.Success / c.Error 走 middleware）
  - [x] 4.2 新建 `server/internal/app/http/handler/pets_handler_test.go`：6 case 覆盖 AC6 钦定（happy / state 缺失 / state=4 / 1009 service error + 可选 case 5 state=0 边界 + case 6 missing userID 兜底）
  - [x] 4.3 stub PetService 实装（与既有 stepsHandler_test.go / roomHandler_test.go 中 service stub 同模式）

- [x] **Task 5（AC7）— PetsHandler 集成测试（dockertest + 完整 router）**
  - [x] 5.1 新建 `server/internal/app/http/handler/pets_handler_integration_test.go`：复用 room_handler_integration_test.go 模式独立命名（petsIntegrationTest...）；build tag `integration` 隔离
  - [x] 5.2 场景 1（happy 端到端 + DB pets.current_state 校验）/ 场景 2（无 token → 1001）/ 场景 3（state=4 → 1002 + DB 未变）/ 场景 4（pet-less 端到端 + DB 仍 0 行）

- [x] **Task 6（AC8, AC9）— bootstrap router wire + router test 验证**
  - [x] 6.1 修改 `server/internal/app/bootstrap/router.go`：在 GormDB / TxMgr / Signer 完整 if-guard 块内，紧接 `roomHandler` 构造之后新增 `petSvc := service.NewPetService(petRepo, userRepo, nil, nil)` + `petsHandler := handler.NewPetsHandler(petSvc)` + 在 authedGroup `/rooms/:roomId` 路由之后追加 `authedGroup.POST("/pets/current/state-sync", petsHandler.PostStateSync) // Story 14.2 加`
  - [x] 6.2 跑 `bash scripts/build.sh --test` 确认既有 router_test.go 不破 —— PASS

- [x] **Task 7（AC10, AC11, AC12）— 业务事件 log + build / test 全绿 + 跨章节自检**
  - [x] 7.1 PetService.SyncCurrentState 的 slog 业务事件 log：happy → `info "pet state-sync succeeded"` + userId/petId/state 字段；pet-less noop → `info "pet-less state-sync noop"` + userId/state 字段；DB error 由 apperror.Wrap 链路 + middleware ErrorMapping 兜底 log；handler 不重复 log
  - [x] 7.2 跑 `bash scripts/build.sh --test` —— 全绿（0 错误 + 0 warning）
  - [x] 7.3 跑 `go test -tags=integration -run 'TestPetService_SyncCurrentState_Integration|TestPetsHandlerIntegration_PostStateSync'` —— pet_service_integration_test + pets_handler_integration_test 全部 PASS（7 case：3 service + 4 handler）。**注**：`bash scripts/build.sh --integration` 全套 120s 超时（pre-existing condition：Windows Docker 串行启容器慢），但单独跑本 story 新增 case 300s 内全绿
  - [x] 7.4 `bash scripts/build.sh --race --test` —— SKIPPED（本地 Windows 缺 cgo gcc 工具链，race build 失败；非 story 14.2 引入的环境问题；CI/Linux 上 race 仍生效）
  - [x] 7.5 `git status --short` 验证 scope 严格：仅命中本 AC 钦定的文件 + 本 story 文件 + sprint-status.yaml；**未**命中 docs/ / migrations/ / iphone/ / ios/ / 其他 _bmad/ 配置
  - [x] 7.6 grep `RowsAffected` / `1003` / `ErrResourceNotFound` 验证：本 story service / handler / repo 代码路径**仅**在 doc-comment 内出现（锁定语义注释），method body **0 命中**
  - [x] 7.7 grep service / handler / repo 路径下 `ws.BroadcastFn` / `ws.SessionManager`：**仅**命中 petServiceImpl struct 字段定义 + NewPetService 构造器签名；method body **0 命中**

- [x] **Task 8（AC8 commit message 模板）— 不本地 commit（dev 阶段跳过）**
  - [x] 8.1 **跳过** dev 阶段 commit：epic-loop 流水线设计是 dev-story 阶段不 commit，由下游 fix-review / story-done sub-agent 统一收口
  - [x] 8.2 commit message 模板（AC 关键约束段）保留在本 story 文件中，story-done sub-agent 直接复用

## Dev Notes

### 关键设计原则

1. **契约严格对齐 V1 §5.2 (14.1 frozen)**：本 story service / handler / response 字段语义、错误码触发条件、pet-less noop 路径、`err == nil ⇒ 200 OK / err != nil ⇒ 1009 不读 RowsAffected` 二分、广播包含发起者自己（14.4 实装）、fire-and-forget 语义（14.4 实装）—— 全部已在 V1 doc 锚定 + 14-1 r1-r11 各轮 review 收敛。本 story 落地时**严格**按 frozen contract，**不**重新评审 / **不**变更 schema / **不**新增错误码。

2. **pet-less 走 server-acknowledged noop**（r7 lessons + V1 §5.2 line 530-531）：service 层 `FindDefaultByUserID` 返 `ErrPetNotFound` 时**禁止**包成 1003，**禁止**透传 raw error；走 noop 路径返 `&Output{State: in.State}` + nil error，让 handler 直接走 happy response.Success 路径。理由：pet-less 与 §5.1 GET /home `data.pet = null` / §10.3 `data.members[].pet = null` / §12.3 `member.joined.payload.pet = null` 是**同一类合法 edge case**（contract 内显式覆盖，非 invariant 损坏）；client 实装层（iOS Story 15.4）**不**需要为 pet-less state-sync 做 special-case suppress。

3. **err 二分锁定**（r1 / r6 / r9 lessons + V1 §5.2 line 532-537）：service 层 UPDATE 后**严格**走两个互斥二分：`err == nil` ⇒ 200 OK + code = 0；`err != nil` ⇒ 1009；**禁止**读 `RowsAffected`、**禁止**根据该值分支业务逻辑。理由：MySQL/GORM 语义下"同 user 同 state 重复上报"幂等场景的 RowsAffected == 0 是合法路径（V1 §5.2 关键约束 + 元信息表幂等性钦定）；本接口的 UPDATE 把 updated_at 也写新值，理论上即便 current_state 未变 updated_at 仍变 → MySQL 通常仍报 RowsAffected == 1；但 GORM/driver 在某些 time-zone / 配置组合下可能仍返 0 —— service 层**不**依赖该值判断成功失败。

4. **本接口不开事务**（V1 §5.2 服务端逻辑"事务边界规则" + 数据库设计 §8.x）：单 SELECT + 单 UPDATE 走 DB 引擎默认 autocommit，**禁止**调 txMgr.WithTx。但 PetRepo.UpdateCurrentStateByID 仍走 `tx.FromContext(ctx, r.db)` 模式（与 user_repo.UpdateNickname / step_repo.UpdateBalance 一致），让本方法**未来若**被纳入事务（如多接口聚合）也能 ctx-aware 无需改 repo signature。

5. **WS 广播是 14.4 范围，本 story 不实装**：本 story service struct 字段预留 `sessionMgr ws.SessionManager` / `broadcastFn ws.BroadcastFn`（14.4 落地时 wire 真实实例 + 加 `broadcastPetStateChanged` 方法 + 在 SyncCurrentState 步骤 4 调用 fire-and-forget）；本 story 这些字段全部 **nil 注入**，service method body **不**调用任何 ws 包导出函数。这样设计的好处：14.4 落地仅需在既有 service 上加新方法 + router.go wire 时换 broadcastFn 实参（而非改 NewPetService 签名 / 改 service struct 字段），改动面更可控 + 单元测试不破。

6. **request `state` / response `data.state` 字段方向 + 权威等价桶**（r10 lessons + V1 §5.2 line 608-613）：本 story 的 service 层输入 `SyncCurrentStateInput.State` 是 client → server 写入信号（请求体入参）；输出 `SyncCurrentStateOutput.State` 是 server → client 单向 ack-only 信号（**永不**入跨 client / 多设备权威等价桶；仅作 self entry 本地 UI 立即更新依据，iOS Story 15.4 实装侧由 V1 doc §5.2 line 549-555 self-broadcast 兜底规则覆盖）。本 story service 层 response 用入参回显（`data.state = in.State`），**不**重新从 DB 查 pet.current_state 反推返回 —— 哪怕 happy path 值相等，**不读 DB 减少一次查询 + 让 ack-only 语义清晰**。

7. **handler 层 *int8 指针避免 zero-value vs missing trap**（r2 + 7.3 lessons）：`PostStateSyncRequest.State` 用 `*int8` 而非 `int8`，让 handler 能区分"字段缺失"（指针 nil → "state 必填"）vs "显式传 0"（指针非 nil 但 deref 后是 0 → "state 必须是 1 / 2 / 3"）。不能用 `binding:"required"`（validator/v10 在数值字段上把 0 视为缺失 —— 与 motionState 同 trap）。

8. **服务端不限频，client 自律节流**（V1 §5.2 客户端节流约束段）：本接口走 Story 4.5 默认 60 次/分通用 RateLimitByUserID，**不**新增独立限频策略；iOS 端（Story 15.4）在动作识别状态机切换瞬间上报（self-imposed 节流），server 不强制。server 应在 log 层关注异常高频上报作客户端实装 bug 的间接信号，但**不**触发 3xxx 业务错误码（与 §6.1 步数防作弊 3001 不同 —— 步数同步是资产入账，需 server 强制阈值；状态同步只是 UI 展示，无资产）。

### 架构对齐

**领域模型层**（`docs/宠物互动App_总体架构设计.md` §3.X 宠物状态）：
- 宠物状态是 UI 展示态（idle/walk/run 三态决定主界面猫 sprite 动画 + 房间内成员猫位渲染）
- 客户端动作识别状态机（节点 3 Epic 8 已实装）的最新切换瞬间上报到 server
- server 端权威记录 `pets.current_state` + 14.4 落地后房间内广播给同房间成员
- 本 story 实装第一步（写库 + ack）；14.4 接管广播；iOS Epic 15 接管客户端集成

**数据库层**（`docs/宠物互动App_数据库设计.md`）：
- §5.3 pets 表 + `uk_user_default_pet (user_id, is_default)` 唯一约束保证 SELECT 默认 pet 命中至多 1 行
- §6.4 `pets.current_state` 枚举 TINYINT 1=rest / 2=walk / 3=run；本 story 使用 int8 与 TINYINT 1:1 对应（与 7.3 motion_state 同模式）
- §3.2 `updated_at DATETIME(3) ON UPDATE CURRENT_TIMESTAMP(3)` 自动更新；GORM autoUpdateTime tag 1:1 对齐
- 数据库设计 §8.x 关键事务设计列表**不包含** state-sync 接口 —— 单 UPDATE 走 autocommit，**不**入事务边界

**WS 协议层**（`docs/宠物互动App_V1接口设计.md` §12.3 + §12.1 / §12.2 通用信封）：
- `pet.state.changed` 信封字段（`type` / `requestId` / `payload.{userId, petId, currentState}` / `ts`）遵循 §12.3 通用信封 + 14.1 锚定的字段表
- 广播 fire-and-forget 语义与 Story 10.5 BroadcastToRoom primitive 一致
- 广播范围：包含发起者自己（与 `member.joined` / `member.left` 排除发起者**不同**语义；14.1 锚定 + §1 节点 5 冻结声明）
- **本 story 不实装广播**；14.4 才接管 `BuildPetStateChangedEnvelope` helper（ws/snapshot.go 同 BuildMemberJoinedEnvelope 同包）+ `broadcastPetStateChanged` service 方法 + service struct broadcastFn / sessionMgr 字段 wire 真实实例

**服务端架构层**（`docs/宠物互动App_Go项目结构与模块职责设计.md` §4 / §5 / §6.3）：
- §6.3 Pet 模块职责：默认宠物初始化（4.6 已实装）+ 当前宠物状态同步（本 story）+ 当前宠物穿戴展示聚合（Epic 26+ 未来）
- §6.3 设计说明："可以写库，但不要高频强依赖；更适合关键时机同步而不是传感器变化就刷库" —— 与 V1 §5.2 客户端节流约束 + Story 15.4 self-imposed 节流策略一致
- §6.3 关联表：pets / user_pet_equips / user_cosmetic_items / cosmetic_items —— 本 story 只动 pets 表 + UPDATE current_state 单列；其他三表是 Epic 26 装扮 / 仓库的范围
- §5.1 Handler 层：DTO 严格按 V1 §5.2 wire schema；handler 不做业务计算，只做参数校验 + 调 service + 转 response
- §5.2 Service 层：单查 pet + 单 UPDATE + 错误转换 + （14.4 接管）广播触发；本接口**不**需要 §5.2 事务管理器（不像 4.6 / 7.3 / 11.3 / 11.4 / 11.5 都需要 txMgr.WithTx）
- §5.3 Repo 层：repo 不直接对外暴露业务错误码（与 4.6 同模式 —— pet_repo.UpdateCurrentStateByID 透传 raw error，service 层用 errors.Is + apperror.Wrap 包成 1009）
- §5.4 Domain 层：当前 server 工程 domain 包仅含 GORM struct（pet_repo.Pet 已在 4.6 落地），本 story 不引入 domain 业务规则常量

**iOS 客户端层**（`docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` + ADR-0002）：
- iOS Story 15.4 落地 `StateSyncUseCase` 调本接口 + Codable `StateSyncRequest{state: Int}` / `StateSyncResponse{state: Int}` —— **本 story server 端 wire 严格按 V1 §5.2 schema 落地，让 iOS Story 15.4 不需要在 client 端做任何 schema 转换**
- DTO 用手写 Codable struct（不上 Swift OpenAPI Generator）；字段命名严格 camelCase；`state` 用 Int 解析（**不**用 enum 类型，让 ViewModel 层基于 Int 值做枚举决策）—— 与 V1 §5.2 wire schema `{state: int}` 字段命名 + 类型一致

### 与 Story 11.3 / 7.3 / 4.6 的对比（参照同模式但简化）

本 story（节点 5 单接口 + 单 UPDATE + 不开事务 + 14.4 才挂广播）相比同模式 stories：

| 维度 | Story 4.6 | Story 7.3 | Story 11.3 | **Story 14.2（本）** |
|---|---|---|---|---|
| 接口数 | 1 (POST /auth/guest-login) | 1 (POST /steps/sync) | 1 (POST /rooms) | 1 (POST /pets/current/state-sync) |
| 事务步数 | 5 步（u / b / p / a / c 五表） | 6 步（账户 + sync_log） | 4 步（预检 + r + rm + u） | **0 步（不开事务）** |
| 业务规则复杂度 | 中（首次 vs 复用 vs 并发回退） | 高（防作弊 3 层 + 时区 + 乱序兜底） | 中（6003 双路径预检 + UNIQUE 兜底） | **低（pet-less noop + err 二分）** |
| 新增 repo 方法 | 5 个（Create x 5） | 4 个（Create / Find / Update / Sum） | 5 个（Create x 2 + UpdateCurrentRoomID + 两个哨兵） | **1 个（UpdateCurrentStateByID）** |
| 新增 service | 1 个 AuthService.firstTimeLogin/reuseLogin | 1 个 StepService.SyncSteps | 1 个 RoomService.CreateRoom | **1 个 PetService.SyncCurrentState** |
| 错误码处理 | 1001 / 1009 + 并发 retry | 1002 / 3001 / 1009 + 阈值 | 1009 / 6003 + UNIQUE 双路径 | **1001 / 1002 / 1005 / 1009（不含 1003）** |
| WS 广播 | 不广播 | 不广播 | 不广播（11.8 才广播） | **不广播（14.4 才广播）；但留 TODO 占位 + service struct 字段预留** |
| 单测 case 数 | 5 个（首次 / 复用 / 并发 / DB 异常 x 2） | 9 个（happy / 倒退 / 截断 / 封顶 / 乱序 / 时区 / 等） | 4 个（happy / 6003 预检 / 6003 兜底 / DB） | **≥4 个（happy / pet-less / DB x 2）+ 可选 ≥5/6** |
| 集成测试 | 单场景（5 表创建） | 单场景（dockertest 端到端） | 双场景（创建 / 6003 重复） | **3 场景（happy / 反向切换 / pet-less）** |

本 story 落地难度评估：**低**（业务规则简单 + 不开事务 + 错误码集合小 + 14-1 已 frozen 全部 contract 边界）；**预期 1 轮 review 收敛**（相比 11.3 走 4 轮 / 11.1 走 14 轮 / 11.7 走 11+ 轮）。

### 测试策略（service 单测 + handler 单测 + 双 dockertest）

本 story 测试覆盖参照 epic-12 retro 的"主旋律 1"（客户端集成 12.7 走到 r14 才收敛）经验 + 4.6 / 7.3 / 11.3 同模式：

- **service 单测**（≥4 case）：mocked PetRepo + UserRepo + ws.SessionManager + ws.BroadcastFn；覆盖 happy / pet-less noop / DB error x 2；可选 ≥5 / 6（幂等同 state 重复上报 / 广播路径未触发）
- **handler 单测**（≥4 case）：stub PetService；覆盖 happy / state 缺失 / state=4 / 1009 envelope；可选 ≥5（state=0 边界）
- **repo 单测**（≥2 case）：sqlmock；覆盖 UpdateCurrentStateByID happy / DB error
- **service 集成测试**（dockertest，3 场景）：真实 MySQL + migrations + user/pet fixture；覆盖 happy state=3 + 反向切换 state=1 + pet-less noop（手动 DELETE pets 行）
- **handler 集成测试**（dockertest，4 场景）：完整 router + 真实 MySQL + JWT signer；覆盖 happy / 无 token / state=4 / pet-less 端到端

测试**禁止**：
- 不复制现有 fixture 全文（必须复用 `_helper.go` 类 helper / shared startup function）
- 不写 Go-level 单元测试覆盖 V1 §5.2 schema 字段说明（schema 已在 14.1 frozen，单测覆盖 ack 信号字段就够；schema 漂移由 review + grep 兜底）
- 不写"未来 14.4 才实装"的广播相关单测（本 story 仅覆盖"广播路径未触发"的 wire 占位 sanity check）

### Project Structure Notes

唯一受影响的文件（按 Go 项目结构 §4 目录形态分类）：

**repo 层** (`server/internal/repo/mysql/`)：
- `pet_repo.go`（扩展 interface + impl）
- `pet_repo_test.go`（新增 ≥2 sqlmock case）

**service 层** (`server/internal/service/`)：
- `pet_service.go`（**新建**；interface + impl + DTO + 构造器）
- `pet_service_test.go`（**新建**；≥4 case mock 单测）
- `pet_service_integration_test.go`（**新建**；≥3 场景 dockertest）

**handler 层** (`server/internal/app/http/handler/`)：
- `pets_handler.go`（**新建**；handler + request DTO）
- `pets_handler_test.go`（**新建**；≥4 case unit 单测）
- `pets_handler_integration_test.go`（**新建**；≥4 场景 dockertest + 完整 router）

**bootstrap** (`server/internal/app/bootstrap/`)：
- `router.go`（扩展 wire petService + petsHandler + authedGroup.POST 挂路由）

**story 文件 + 状态**：
- `_bmad-output/implementation-artifacts/14-2-post-pets-current-state-sync-接口-pets-current_state-更新.md`（**新建**；本 story 自身；dev 阶段填充 Dev Agent Record / File List）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（状态翻 done 时编辑；dev 阶段先翻 in-progress）

**不影响其他目录**：
- `server/migrations/` 不动（pets 表 + current_state 列已在 0003 落地）
- `server/configs/` 不动（本接口不需要新配置；走 RateLimit / Steps 配置无关）
- `server/cmd/server/` 不动（main.go 已透传 Deps，本 story 不新增字段）
- `server/internal/app/ws/` 不动（14.4 才动；本 story service struct 仅 import ws 包类型作字段定义，**不**调任何 ws 包导出函数）
- `server/internal/infra/` 不动
- `server/internal/pkg/` 不动
- `docs/**` 不动（V1 doc / 数据库设计 / Go 项目结构 / iOS 工程结构 / 时序图 / 总体架构 / MVP 节点规划 全部是契约**输入**）
- `iphone/` / `ios/` 不动（iOS Epic 15 才落地客户端）
- `_bmad/` 不动（本 story 不改 BMM 配置）
- `_bmad-output/` 下其他 yaml / md（除自己 + sprint-status.yaml + 可能的 lesson）不动

**lessons 文件**（如 dev / review 过程产生新经验）：
- `docs/lessons/2026-05-12-*-14-2-r*.md`（**可能新增**；视 review 轮次 + 踩坑情况；本 story 预期 1 轮收敛 → 大概率不新增 lesson）

### References

- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 14.2 (行 2297-2319)] — 本 story 钦定 AC 来源（含 1003 → noop 修正注 by r7 lessons + 4 case 单测覆盖 + 集成测试场景）
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 14.1 (行 2279-2295)] — 上游契约 story（已 frozen V1 §5.2 + §12.3）
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 14.3 (行 2321-2339)] — 下游 RoomSnapshotBuilder 真实驱动（本 story 是其数据起点）
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 14.4 (行 2341-2363)] — 下游 WS 广播（本 story 在 service 留 TODO 占位 + struct 字段预留 sessionMgr / broadcastFn）
- [Source: `_bmad-output/planning-artifacts/epics.md` §Epic 15] — iOS 节点 5 端 client 集成路径（Story 15.1 / 15.2 / 15.4 / 15.5 间接 / 直接依赖本 story）
- [Source: `docs/宠物互动App_V1接口设计.md` §1 + §3 + §5.2 + §12.3 `### 宠物状态变更`] — 14.1 frozen contract（本 story 严格对齐）
- [Source: `docs/宠物互动App_数据库设计.md` §5.3 pets + §6.4 pets.current_state + §3.2 时间戳列] — DB schema 输入
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 + §5 + §6.3 Pet 模块] — Go 工程目录 + 分层职责 + 模块边界
- [Source: `docs/宠物互动App_时序图与核心业务流程设计.md` 行 555] — pet.state.changed 业务消息触发点说明
- [Source: `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md`] — 14.1 整文件（含 r1-r11 各轮 review 收敛过程 + 全部已 archived 的设计陷阱 + AC8 commit message 模板参考）
- [Source: `_bmad-output/implementation-artifacts/4-6-游客登录接口-首次初始化事务.md`] — 同模式参照（epic 第一条业务实装 story；service / handler / repo / router / 单测 / 集成测试全栈）
- [Source: `_bmad-output/implementation-artifacts/7-3-post-steps-sync-接口-累计差值入账-service.md`] — 同模式参照（单接口 + handler 字段 *int 指针 + service 层 err 处理 + dockertest）
- [Source: `_bmad-output/implementation-artifacts/11-3-创建房间事务.md`] — 同模式参照（单接口 + service / handler / repo / router；本 story 范围更小因不开事务）
- [Source: `_bmad-output/implementation-artifacts/decisions/0006-error-handling.md`] — ADR-0006 错误码三层映射（repo raw error / sentinel → service apperror.Wrap → handler middleware envelope）
- [Source: `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md`] — ADR-0007 ctx 传播（service / repo 第一参数 ctx；GORM .WithContext(ctx)；txMgr.WithTx 内用 txCtx）
- [Source: `_bmad-output/implementation-artifacts/decisions/0011-ws-stack.md`] — ADR-0011 WS stack 选型（本 story 不直接使用，但 14.4 实装层需要；本 story service struct 字段类型 `ws.SessionManager` / `ws.BroadcastFn` 来源于 ADR-0011）
- [Source: `docs/lessons/2026-05-12-state-sync-idempotent-rowsaffected-and-ws-envelope-ts.md`（r1）] — 幂等 + RowsAffected 误判 + WS envelope ts 字段归属
- [Source: `docs/lessons/2026-05-12-cross-section-equivalence-claim-must-fence-prerequisites-and-self-broadcast-fallback-2.md`（r2）] — 跨章节字段等价 + ack vs 权威分层 + self-broadcast 丢失 sender-side 兜底
- [Source: `docs/lessons/2026-05-12-member-joined-stale-state-and-self-broadcast-arrival-order-symmetric-3.md`（r3）] — member.joined stale state + self-broadcast 到达顺序对称
- [Source: `docs/lessons/2026-05-12-self-broadcast-ui-driver-and-freeze-boundary-and-self-vs-others-priority-4.md`（r4）] — self-broadcast UI 驱动 + 冻结边界 + self vs 他人优先级
- [Source: `docs/lessons/2026-05-12-merge-contract-exception-and-ts-business-ordering-ban-and-ack-bucket-explicit-enum-5.md`（r5）] — merge contract 例外 + `ts` 业务排序禁令 + 权威等价桶四处枚举
- [Source: `docs/lessons/2026-05-12-state-sync-err-binary-and-placeholder-whitelist-self-http-ack-14-1-r6.md`（r6）] — state-sync err 二分锁定 + placeholder 例外白名单（本 story 服务端逻辑实装最关键 lesson）
- [Source: `docs/lessons/2026-05-12-state-sync-pet-less-noop-consistent-with-home-room-snapshot-14-1-r7.md`（r7）] — pet-less 走 noop（本 story service 层 pet-less 路径实装最关键 lesson）
- [Source: `docs/lessons/2026-05-12-story-file-must-stay-in-sync-with-frozen-v1-doc-14-1-r8.md`（r8）] — story 文件 vs V1 doc 同步纪律
- [Source: `docs/lessons/2026-05-12-story-file-rowsaffected-and-top-level-1003-drift-14-1-r9.md`（r9）] — story 文件 RowsAffected / 1003 残留 drift 清理
- [Source: `docs/lessons/2026-05-12-story-ac-authority-bucket-direction.md`（r10）] — Story AC 字段方向（client→server / server→client / ack-only）权威等价桶区分
- [Source: `docs/lessons/2026-05-12-story-file-14-3-scope-must-list-member-joined-14-1-r11.md`（r11）] — story 文件 14.3 落地范围三处统一 + References 1003 残留清理
- [Source: `CLAUDE.md` §"工作纪律"] — 状态以 server 为准 / 错误码统一 / ctx 必传（本 story 严格遵守）
- [Source: `server/internal/service/room_service.go` `broadcastMemberJoined` 方法] — fire-and-forget broadcast 同模式参照（14.4 实装时直接复用）
- [Source: `server/internal/app/ws/snapshot.go` `BuildMemberJoinedEnvelope` / `BuildMemberLeftEnvelope`] — 14.4 落地时新增 `BuildPetStateChangedEnvelope` 的同包同模式参照

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) — `claude-opus-4-7[1m]`

### Debug Log References

无（dev 阶段无 debug 循环；红绿循环一次通过）。

### Completion Notes List

**AC1 — `PetRepo.UpdateCurrentStateByID` 方法 + 单测**
- `server/internal/repo/mysql/pet_repo.go`：PetRepo interface 加 `UpdateCurrentStateByID(ctx, petID, state) error` 方法签名 + 完整 doc comment（err 二分锁定 / 不读 RowsAffected / state 取值与 §6.4 同义 / ctx 用法 ADR-0007 §2.3 / 未来事务化 ctx-aware 兼容性说明）；impl 用 `db.WithContext(ctx).Model(&Pet{}).Where("id = ?", petID).Update("current_state", state).Error`（Update 单字段路径，autoUpdateTime tag 自动写 updated_at）。
- `pet_repo_test.go` 加 2 个 sqlmock case：`TestPetRepo_UpdateCurrentStateByID_Happy`（args 严格匹配 state, AnyArg, petID → NewResult(0,1) → nil err）+ `TestPetRepo_UpdateCurrentStateByID_DBError`（driver error 透传）。**禁止**新增 RowsAffected 相关 case（与 r1/r6/r9 锁定一致）。

**AC2 — `PetService` interface + impl 新建**
- `server/internal/service/pet_service.go`（**新建** 173 行）：包级 doc comment 标注节点 5 / Epic 14 范围 + 14.4 未实装 + sessionMgr/broadcastFn 字段语义；`PetService` interface + `SyncCurrentStateInput` / `SyncCurrentStateOutput` DTO + `petServiceImpl` struct（4 字段：petRepo / userRepo / sessionMgr / broadcastFn）+ `NewPetService(petRepo, userRepo, sessionMgr, broadcastFn) PetService` 构造器（nil-tolerant）+ `SyncCurrentState` impl 严格按 V1 §5.2 + r6/r7 lessons 流程：(1) FindDefaultByUserID → pet-less noop / 其他 1009；(2) UpdateCurrentStateByID → err 二分；(3) 14.4 TODO 占位（**不**调任何 ws 包导出函数）；(4) 回显入参 ack。

**AC3 — `PetService` 单测（≥4 case）**
- `pet_service_test.go`（**新建** 6 case）：
  - case 1 happy state=2（验证 repo 调用 args + output 回显）
  - case 2 pet-less noop（验证 UpdateCurrentStateByID 0 次调用 + apperror.Code(err)==0）
  - case 3 FindDefault DB 错误 → 1009（验证 errors.Is 链 + UpdateCurrentStateByID 0 次）
  - case 4 Update DB 错误 → 1009（验证 UpdateCurrentStateByID 调用发生但失败）
  - case 5 幂等同 state 重复上报（两次都 nil error）
  - case 6 广播路径未触发（broadcastCalls==0 验证本 story 不广播）
- 手写 stub PetRepo / stub UserRepo（UserRepo 所有方法 panic 兜底确保"误调"可见）。

**AC4 — `PetService` 集成测试（dockertest，3 场景）**
- `pet_service_integration_test.go`（**新建**）：复用 `startMySQL` / `runMigrations` helper（auth_service_integration_test.go 落地）；build tag `integration`。
  - 场景 1+2 合并 `TestPetService_SyncCurrentState_Integration_HappyAndReverseSwitch`：state=3 → DB current_state=3 + updated_at 单调递增 → state=1 → DB current_state=1。
  - 场景 3 `TestPetService_SyncCurrentState_Integration_PetLess_Noop`：DELETE pets → SyncCurrentState state=2 → err==nil + apperror.Code==0 + DB pets 行数仍 0。

**AC5 — `PetsHandler` 新建 + 错误码三层映射**
- `server/internal/app/http/handler/pets_handler.go`（**新建** 110 行）：`PetsHandler` struct + `NewPetsHandler` 构造器 + `PostStateSyncRequest{State *int8}`（指针避免 zero-value-vs-missing trap，与 7.3 PostSyncRequest 同模式）+ `PostStateSync` handler：(1) ShouldBindJSON / (2) State==nil → 1002 "state 必填" / (3) State 范围 [1,3] → 1002 "state 必须是 1 / 2 / 3" / (4) 取 c.Get(UserIDKey) / (5) 调 svc.SyncCurrentState / (6) response.Success or c.Error；**不**直接调 response.Error（ADR-0006 单一 envelope 生产者）。

**AC6 — `PetsHandler` 单测（≥4 case）**
- `pets_handler_test.go`（**新建** 6 case）：happy / state 缺失（"必填" message）/ state=4（"1 / 2 / 3" message）/ service 1009 forwards / state=0 边界（区分缺失 vs 显式 0）/ missing UserID → 1009 兜底。

**AC7 — `PetsHandler` 集成测试（dockertest + 完整 router，4 场景）**
- `pets_handler_integration_test.go`（**新建** 4 case）：复用 room_handler_integration_test.go 模式独立命名（petsIntegrationTest...）；build tag `integration`。
  - HappyPath：合法 Bearer token → POST {"state":3} → envelope code=0 + data.state=3 + DB pets.current_state=3
  - NoToken_Returns1001：无 Authorization → 1001 envelope（HTTP 200 业务码正交）
  - StateOutOfRange_Returns1002：state=4 → 1002 + DB 未变（handler 拦截在前）
  - PetLessAccount_Returns200WithEcho：DELETE pets → state=2 → HTTP 200 + envelope code=0 + data.state=2 + DB pets 仍 0 行

**AC8 — bootstrap router wire**
- `server/internal/app/bootstrap/router.go`：在 GormDB / TxMgr / Signer 完整 if-guard 块内紧接 `roomHandler` 构造之后加 `petSvc := service.NewPetService(petRepo, userRepo, nil, nil)` + `petsHandler := handler.NewPetsHandler(petSvc)`（broadcastFn / sessionMgr 都先传 nil，14.4 才换真实实例）；authedGroup 在 `/rooms/:roomId` 之后追加 `authedGroup.POST("/pets/current/state-sync", petsHandler.PostStateSync) // Story 14.2 加`。

**AC9 — Router test 扩展**
- 不新增 router_test.go case（与 11.3 同模式）；既有 router_test.go zero-deps 路径不受影响（PetService 在 GormDB if-guard 内构造，未满足条件时不挂路由，与 ws / room 路由同模式）；`bash scripts/build.sh --test` 全绿验证既有 router_test 不破。

**AC10 — 业务事件 log**
- PetService.SyncCurrentState happy 路径 `info "pet state-sync succeeded"` + slog.Uint64("userId") + slog.Uint64("petId") + slog.Int("state")；pet-less noop `info "pet-less state-sync noop"` + userId / state；DB error 不主动 log error，由 apperror.Wrap + middleware ErrorMapping 兜底；handler 层不重复 log（middleware Logging 已记录）；**不**新增 prometheus metric（节点 5 阶段无新增可观测性需求）。

**AC11 — build / test 全绿**
- `bash scripts/build.sh --test`：**PASS**（go vet + go build + 全量单元测试 25 packages 全绿，0 错误 0 warning）。
- 单独跑本 story 7 个 integration case（`go test -tags=integration -run 'TestPetService_SyncCurrentState_Integration|TestPetsHandlerIntegration_PostStateSync'`）：**PASS**（service 3 个 case + handler 4 个 case；300s timeout 内完成）。
- `bash scripts/build.sh --integration` 全套：超出 120s 默认 timeout 因 pre-existing 条件（Windows Docker 串行启容器慢），非 14.2 引入；本 story 单独 case 全绿。
- `bash scripts/build.sh --race --test`：SKIPPED（本地 Windows 缺 cgo gcc；race detector 需 CI/Linux）。

**AC12 — 跨 epic 一致性自检**
- `git status --short` scope 范围检查：
  - 新增：pet_service.go / pet_service_test.go / pet_service_integration_test.go / pets_handler.go / pets_handler_test.go / pets_handler_integration_test.go + 本 story 文件
  - 修改：pet_repo.go / pet_repo_test.go（AC1 扩展）+ router.go（AC8 wire）+ sprint-status.yaml（状态流转）+ auth_service_test.go / home_service_test.go / room_service_test.go / auth_service_integration_test.go（既有 stub PetRepo 加 UpdateCurrentStateByID 兜底方法以保持 mysql.PetRepo interface 满足）
  - **未命中**：`docs/*.md` / `_bmad/` 配置 / 其他 `_bmad-output/` / `iphone/` / `ios/` / `server/migrations/` / `server/internal/app/ws/` 既有文件 / `server/configs/`
- grep `RowsAffected` / `1003` / `ErrResourceNotFound`：本 story 代码路径仅在 doc-comment 内出现（锁定语义注释），method body 0 命中。
- grep `ws.BroadcastFn` / `ws.SessionManager`：本 story service 代码仅在 struct 字段定义 + NewPetService 构造器签名出现，**method body 0 命中**；**不**调用任何 ws 包导出函数（与 14.4 范围红线一致）。

### File List

**新增** (7 files)：
- `server/internal/service/pet_service.go` (~173 lines)
- `server/internal/service/pet_service_test.go` (~290 lines, 6 case)
- `server/internal/service/pet_service_integration_test.go` (~225 lines, 3 case)
- `server/internal/app/http/handler/pets_handler.go` (~110 lines)
- `server/internal/app/http/handler/pets_handler_test.go` (~250 lines, 6 case)
- `server/internal/app/http/handler/pets_handler_integration_test.go` (~330 lines, 4 case)

**修改** (8 files)：
- `server/internal/repo/mysql/pet_repo.go` (+~70 lines: interface 方法 + impl + doc comment)
- `server/internal/repo/mysql/pet_repo_test.go` (+~50 lines: 2 sqlmock case)
- `server/internal/app/bootstrap/router.go` (+~15 lines: wire petSvc + petsHandler + 路由)
- `server/internal/service/auth_service_test.go` (+5 lines: stubPetRepo.UpdateCurrentStateByID panic 兜底)
- `server/internal/service/home_service_test.go` (+4 lines: stubHomePetRepo.UpdateCurrentStateByID no-op 兜底)
- `server/internal/service/room_service_test.go` (+5 lines: roomTestStubPetRepo.UpdateCurrentStateByID panic 兜底)
- `server/internal/service/auth_service_integration_test.go` (+5 lines: faultPetRepo.UpdateCurrentStateByID delegate transit)
- `_bmad-output/implementation-artifacts/sprint-status.yaml` (1 line: ready-for-dev → review)

**Story 文件**：`_bmad-output/implementation-artifacts/14-2-post-pets-current-state-sync-接口-pets-current_state-更新.md`（Status / Tasks checkbox / Dev Agent Record / File List / Change Log 更新）。

### Change Log

| 日期 | 变更 | 备注 |
|---|---|---|
| 2026-05-12 | Story 14.2 created (ready-for-dev) | bmad-create-story workflow（epic-loop 派单）生成；按 epics.md §Story 14.2（含 1003 → noop r7 修正注）+ Story 14.1 frozen contract + 14-1 r1-r11 全部 lessons + Go 项目结构 §6.3 Pet 模块 + 同模式 4.6 / 7.3 / 11.3 锚定 |
| 2026-05-12 | Story 14.2 dev-story 实装完成 (review) | bmad-dev-story workflow（epic-loop 派单）执行；PetRepo / PetService / PetsHandler / router wire 全栈落地 + 6 service unit case + 6 handler unit case + 3 service integration case + 4 handler integration case；scripts/build.sh --test 全绿；7 个 integration case 单独跑全绿（dockertest）；scope 严格 AC1/AC2/AC4/AC5/AC7/AC8 钦定文件；RowsAffected / 1003 / ws.BroadcastFn 在 method body 0 命中（r1/r6/r7/r9 锁定遵守） |
