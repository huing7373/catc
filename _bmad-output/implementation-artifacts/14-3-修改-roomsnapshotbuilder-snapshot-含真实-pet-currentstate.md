# Story 14.3: 修改 RoomSnapshotBuilder - snapshot 含真实 pet.currentState（三处统一：§10.3 GET /rooms/{roomId} + §12.3 room.snapshot + §12.3 member.joined 同步切真实读取 `pets.current_state`，去掉硬编码 `1` placeholder + 单测 ≥4 case + dockertest 集成测试覆盖三条路径）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want **把节点 4 / Epic 11 阶段三处 server → client `pet.currentState` 字段的 placeholder `1` 同步切换为真实读取 `pets.current_state` —— 三处分别是：(i) §10.3 `GET /rooms/{roomId}.data.members[].pet.currentState`（实装在 `server/internal/service/room_service.go:1266` `roomServiceImpl.GetRoomDetail` 拼装 `MemberPetOutput.CurrentState`）/ (ii) §12.3 `room.snapshot.payload.members[].pet.currentState`（实装在 `server/internal/app/ws/snapshot.go:318` `realSnapshotBuilder.BuildSnapshot`）/ (iii) §12.3 `member.joined.payload.pet.currentState`（实装在 `server/internal/service/room_service.go:1364` `roomServiceImpl.broadcastMemberJoined`）—— 三处必须**同一落地点统一切换**（V1 §1 line 46 / 49 + §5.2 line 608-613 + §12.3 line 1988 / 2121 / 2252 + 14-1 r11 lesson 钦定）；具体实装为：(1) 给 `server/internal/repo/mysql/room_member_repo.go` 的 `RosterRow` struct 新增 `CurrentState *int8` 字段（与 `PetID *uint64` 同模式，pet-less 时 LEFT JOIN `pets.current_state` 为 NULL → GORM Scan 映射为 `*int8` nil）+ 修改 `ListRosterByRoomID` SQL 增加 `pets.current_state AS current_state` 列输出（不动 JOIN / WHERE / ORDER BY 子句）；(2) `server/internal/service/room_service.go` `roomServiceImpl.GetRoomDetail` 拼装 `MemberPetOutput.CurrentState` 时把硬编码 `1` 改为 `*r.CurrentState`（依赖 r.PetID != nil 条件分支已确保同行 pets.current_state 必非 NULL —— `is_default=1` 行 schema 钦定 `current_state NOT NULL DEFAULT 1`）；(3) `server/internal/app/ws/snapshot.go` `realSnapshotBuilder.BuildSnapshot` 把硬编码 `CurrentState: 1` 改为 `CurrentState: int(*r.CurrentState)`（同样在 r.PetID != nil 分支内）；(4) `server/internal/service/room_service.go` `roomServiceImpl.broadcastMemberJoined` 把硬编码 `CurrentState: 1` 改为 `CurrentState: int(petRow.CurrentState)` —— `FindDefaultByUserID` 已返 `*mysql.Pet` 含 `CurrentState int8` 字段（pet_repo.go:29 既有定义），无需扩展 PetRepo；(5) `placeholderSnapshotBuilder.BuildSnapshot` **保持** `CurrentState: 1` 不动（Story 10.7 落地的 placeholder 形态已 r14 锁定 + 仅测试路径用，本 story 不回工 placeholder）；(6) `room_handler.go` `GetRoomDetailResponseMemberPet.CurrentState int8` wire DTO struct 字段类型 / json tag 不动（service 层 `MemberPetOutput.CurrentState int8` 字段已存在 —— room_service.go:186，本 story 仅改赋值来源）；(7) 单元测试覆盖 ≥4 case：snapshot_test.go 新增 happy 3 成员 currentState 1/2/3 / pet-less currentState 字段忽略 / DB error 透传 / 0 成员空房间；room_service_test.go 新增 GetRoomDetail happy 3 成员 currentState 1/2/3 + broadcastMemberJoined happy currentState 真实驱动 + broadcastMemberJoined pet-less 不下发 pet 子对象；(8) 集成测试覆盖 ≥1 case dockertest：创建房间 + 3 成员（pet.current_state 分别 1/2/3）→ (a) GET /api/v1/rooms/{roomId} 验证 3 members.pet.currentState wire 值 / (b) WS 连接验证 room.snapshot.members[].pet.currentState 真实值 / (c) 第 4 个 user join 验证 member.joined.payload.pet.currentState 真实值（需要预先 state-sync 到非 1）**,
so that **下游 iOS Epic 15 全部 stories（15.1 房间页内多成员猫位渲染 / 15.2 pet.state.changed WS 消息处理 / 15.5 跨房间状态恢复）+ Epic 16 节点 5 跨端集成 e2e + demo 验收** 能基于"server 三处 `pet.currentState` 字段全部读真实 `pets.current_state`"的权威等价层展开 —— 该权威等价层在 V1 §5.2 line 608-613 + §12.3 line 2252 钦定为"四处 server → client `pet.currentState` 字段（pet.state.changed / room.snapshot / GET /rooms / member.joined）自 Story 14.3 起承载相同权威级别"；本 story 是这条钦定的实装落地点。本 story **不**实装 14.4 WS 广播 / iOS Epic 15 / Epic 16，**不**改 V1 接口文档（14.1 已冻结）/ 不改 migrations / 不动 placeholder builder。

## 故事定位（Epic 14 第三条 = Epic 14 真实驱动 placeholder→authoritative 切换点；上承 14.1 契约 + 14.2 写库接口 + 11.6 GET /rooms 真实路径 + 11.7 RoomSnapshotBuilder 真实路径 + 11.8 member.joined 广播路径，下启 14.4 + iOS Epic 15）

- **Epic 14 进度**：14.1（POST /pets/current/state-sync + WS pet.state.changed 契约定稿，done）→ 14.2（POST /pets/current/state-sync 接口 + pets.current_state UPDATE 实装，done）→ **14.3（本 story，三处 server → client `pet.currentState` 字段同步切真实读取 `pets.current_state`）** → 14.4（pet.state.changed WS 广播）。**14.3 是 14.4 的强前置**：14.4 落地的 `pet.state.changed` 广播只覆盖"state-sync 调用瞬间"的实时同步；14.3 落地的三处 placeholder→authoritative 切换覆盖"非 state-sync 路径的 client 拉取 / WS 握手 / member.joined"四个静态信号源，让 client 进入房间 / reconnect / 接收 member.joined 时立即看到真实状态（不必等下一次该用户 state-sync 才纠正 stale `1`）。

- **本 story 是 Epic 14 / 节点 5 阶段第一个不写 DB 但同时改三个模块的 cross-cutting story**：14.1（纯契约）/ 14.2（POST handler + service + repo + 路由 + WS 广播预留点）都是局部新增；本 story **不新增**任何 endpoint / migration / service method，仅在三个既有 service 代码点 + 一个既有 repo 字段把硬编码 `1` 替换为 query 真值 —— 改动小但影响 wire schema 的"权威等价层"边界。与 Story 11.10（GET /home 扩展 room.currentRoomId 真实数据）模式直接对照（同样是"placeholder → 真实"的字段切换 story，仅改 home_service 拼装层 + 添加单测 + 集成测试，**不**改 V1 文档）。

- **本 story 是 iOS Epic 15 / Epic 16 强前置**：
  - **iOS Story 15.1（房间页内多成员猫位渲染 + snapshot pet.currentState 解析）**：进房后 WS 握手收 `room.snapshot.members[].pet.currentState` 真实值（本 story §12.3 room.snapshot 切换路径），iOS 端 RoomViewModel 直接 enum 映射成 `.rest` / `.walk` / `.run` 驱动 `PetSpriteView`；本 story 是 15.1 整套链路的 server 真实驱动起点
  - **iOS Story 15.2（pet.state.changed WS 消息处理）**：依赖 14.4 WS 广播 + 本 story 三处权威等价层（reconnect 后 `room.snapshot` 全量重新对齐路径，详见 §12.3 line 2252 "(b) 权威 / client 信任层等价（自 Story 14.3 起成立）"）
  - **iOS Story 15.5（跨房间状态恢复）**：用户进新房间 → WS 握手 → 收 `room.snapshot.members[].pet.currentState`（本 story 落地后真值）；本 story 是 15.5 的核心数据起点
  - **Epic 16 节点 5 demo 验收 + 跨端集成 e2e + tech debt 登记**：14.x + iOS Epic 15 全部完成后由 Epic 16 收口；本 story 落地后跨端 e2e 测试可断言"A 在房间外切 walk → join 房间 X → 房间内已在场成员的 `member.joined.payload.pet.currentState = 2`（不再 stale `1`）"

- **epics.md §Story 14.3 钦定**（行 2321-2339）：
  - **Given** Story 11.7 RoomSnapshotBuilder 已实装但 pet.currentState 写死 1
  - **When** 完成本 story
  - **Then** 修改 `RoomSnapshotBuilder.BuildSnapshot`:
    - 在查 room_members 关联 users + pets 时，把 pets.current_state 真实读出
    - snapshot.members[].pet.currentState = 该值
  - **And** 不破坏其他字段（pet.equips 仍 []，留给后续 Epic）
  - **And** **单元测试覆盖**（≥3 case）:
    - happy: 房间 3 成员，各自 pet.current_state 分别为 1/2/3 → snapshot 中 currentState 字段正确对应
    - happy: 房间成员的 pet 没有（理论不该发生）→ snapshot.members[].pet 为 null（或 default 1，二选一并测试）—— **本 story 选择 null**（与 §12.3 行 1881 r14 锁定的 going-forward 契约 + Story 11.7 已实装 pet-less → JSON null 一致；**不**走"default 1"路径，避免引入 placeholder 复活）
    - edge: 大量并发查 snapshot → 不竞态（DB 查询是只读）—— **本 story 偏移**：并发性由 GORM / database/sql 连接池保证，单测层无意义重复覆盖；改为更具体的"DB error 透传 / 0 成员空房间"两个 edge case
  - **And** **集成测试覆盖**（dockertest）: 创建房间 + 3 成员 + 各自 set pet.current_state=2/3/1 → WS 客户端连入 → snapshot 验证 3 个 members 的 currentState 都正确
  - **本 story 范围扩展（epics.md §Story 14.3 未明列但 V1 + 14-1 r11 lesson 钦定强制要求）**：epics.md 行 2321-2339 仅明列 `RoomSnapshotBuilder` 切换，但 V1 §1 line 46 / 49 + §5.2 line 608-613 + §12.3 line 1988 / 2121 / 2252 + 14-1 r11 lesson（`docs/lessons/2026-05-12-story-file-14-3-scope-must-list-member-joined-14-1-r11.md`）锁定 **Story 14.3 同一落地点同时覆盖三处** server → client `pet.currentState` 字段切真实路径：`GET /rooms/{roomId}.data.members[].pet.currentState` + `room.snapshot.payload.members[].pet.currentState` + `member.joined.payload.pet.currentState`。漏任一处都会导致"用户在房间外切 walk/run → join 房间 → 房间内已在场成员通过 `member.joined.payload.pet.currentState` 看到 stale `1` 直到下一次 `state-sync` 才纠正"的 race（详见 §5.2 line 613 + §12.3 line 2121 stale risk 说明）；本 story 三处统一是 **non-negotiable**

- **14.1 已 frozen 的 wire schema（V1 §10.3 + §12.3 room.snapshot + §12.3 member.joined）—— 本 story 严格按契约实装，不修改任何字段名 / 类型 / json tag**：
  - **§10.3 `data.members[].pet.currentState`**：`number (int)`，必填（仅当 `pet ≠ null`），枚举 `1=rest / 2=walk / 3=run`，来源 `pets.current_state`；V1 line 1389 钦定"节点 4 阶段固定返回 `1`（Epic 14 才真实驱动 motion_state）"；本 story 落地后自此真实驱动
  - **§12.3 `room.snapshot.payload.members[].pet.currentState`**：`number (int)`，必填（仅当 `pet ≠ null`），枚举同上；V1 line 1988 钦定"node-4 placeholder 阶段（Story 10.7）固定返回 `1`；Story 11.7 真实实现亦固定返回 `1`（Epic 14 才真实驱动）；**自 Story 14.3 起切真实值**（读 `pets.current_state`，与 §10.3 五阶段过渡表 `pet.currentState` 节点 5 真实列 / §12.3 `### 成员加入（member.joined）` `payload.pet.currentState` 同时切真实路径，由 Story 14.3 落地 `RoomSnapshotBuilder` 真实化时同步覆盖 `member.joined` —— 三处 server → client `pet.currentState` 字段切真实值的 epic 落地点统一在 Story 14.3，同一 `pets.current_state` 来源）"；本 story 严格按此切换
  - **§12.3 `member.joined.payload.pet.currentState`**：`number (int)`，必填（仅当 `pet ≠ null`），枚举同上；V1 line 2121 钦定"节点 4 阶段固定 `1`（与 §10.3 / §12.3 `room.snapshot` placeholder 同语义）；**自 Story 14.3 起切真实值**"+ stale `1` race 说明；本 story 严格按此切换
  - **§5.2 权威等价桶（V1 line 608-613）**：自 Story 14.3 起，**四处** server → client `pet.currentState` 字段 —— (i) `pet.state.changed.payload.currentState`（14.4 落地）/ (ii) `room.snapshot.payload.members[].pet.currentState`（本 story）/ (iii) `member.joined.payload.pet.currentState`（本 story）/ (iv) `GET /rooms/{roomId}.data.members[].pet.currentState`（本 story）—— 承载相同权威级别，client 实装层不需要为四种来源做差异化处理。**不**包括 `POST /pets/current/state-sync` 的 request `state` / response `data.state`（ack-only，不入权威桶；14-1 r4 / r10 锁定）
  - **数据库 §6.4 状态枚举（`pets.current_state` TINYINT NOT NULL DEFAULT 1）**：1=rest / 2=walk / 3=run；Story 4.3 / 11.2 migration 已 ship + Story 4.6 firstTimeLogin 默认写 `1` + Story 14.2 POST /pets/current/state-sync 接口允许写 1/2/3 + handler 层校验枚举范围；本 story 仅消费该列（SELECT 路径），**不**改 schema / 不加 column / 不改 DEFAULT

- **14-1 lessons 必须遵守（11 条 r1-r11 sequence；本 story 是 14-1 r11 钦定的"三处统一"落地直接对应物）**：
  - **r1**（[幂等 + RowsAffected 误判 / WS envelope 字段归属](../../docs/lessons/2026-05-12-state-sync-idempotent-rowsaffected-and-ws-envelope-ts.md)）：本 story 不涉及 RowsAffected（纯 SELECT 路径，无 UPDATE）；但 broadcastMemberJoined 已存在的 `petRow.ID` / `petRow.CurrentState` 取值路径**不**读 RowsAffected（FindDefaultByUserID 是 SELECT，无此概念）
  - **r2**（[跨章节字段等价声明锁定前置 + ack vs 权威分层 + self-broadcast 丢失兜底](../../docs/lessons/2026-05-12-cross-section-equivalence-claim-must-fence-prerequisites-and-self-broadcast-fallback-2.md)）：本 story 三处切换后**只有 server → client 四处权威等价桶生效**；不影响 §5.2 response `data.state`（ack-only，14.3 落地前后语义不变；不入权威等价桶）。本 story 严禁误把 14.2 的 response.data.state 字段也"提升"到权威桶（与 r4 / r10 锁定一致）
  - **r3**（[member.joined `pet.currentState` 14.3 落地前 stale race + self-broadcast no-op 措辞基于到达顺序对称](../../docs/lessons/2026-05-12-member-joined-stale-state-and-self-broadcast-arrival-order-symmetric-3.md)）：本 story 修复的 stale `1` race **正是 r3 锁定的具体落地点** —— 用户在房间外切 walk/run → `users.current_room_id == NULL`（§5.2 服务端逻辑步骤 5 不广播）→ join 房间 X → 房间内已在场成员通过 `member.joined.payload.pet.currentState` 看到的 currentState 必须是真实值（不是 placeholder `1`），否则 stale 直到该用户**再次** state-sync 才纠正；本 story 落地后 race 消失
  - **r4**（[self-broadcast UI 驱动 + 冻结边界声明区分抽象触发与阈值 + self vs 他人优先级](../../docs/lessons/2026-05-12-self-broadcast-ui-driver-and-freeze-boundary-and-self-vs-others-priority-4.md)）：本 story 落地后"权威等价层"在 server → client 四处生效；client 实装层（iOS Story 15.x）的 self vs 他人优先级排序不在本 story 范围（归属 iOS Epic 15）
  - **r5**（[临时窗口优先级 merge contract + `ts` 业务排序禁令 + 权威等价桶四处枚举](../../docs/lessons/2026-05-12-merge-contract-exception-and-ts-business-ordering-ban-and-ack-bucket-explicit-enum-5.md)）：本 story 落地是**退出**临时窗口的标志 —— V1 §12.3 line 2092 client merge contract 中"Story 14.3 落地前的临时窗口例外"规则在本 story 落地后**失效**，回归"数值字段直接覆盖"通用规则；本 story 不需改 V1 文档（line 2097 已显式标注"Story 14.3 落地后例外失效"），仅通过实装让该窗口自然结束
  - **r6**（[state-sync err 二分锁定 + placeholder 例外白名单覆盖 self HTTP ack](../../docs/lessons/2026-05-12-state-sync-err-binary-and-placeholder-whitelist-self-http-ack-14-1-r6.md)）：本 story 不涉及 err 二分（纯 SELECT 路径无 UPDATE）；但 GetRoomDetail / RoomSnapshotBuilder / broadcastMemberJoined 三处的 DB error 处理路径**沿用既有实装**（不动）
  - **r7**（[state-sync pet-less 与 /home / room / member.joined 同语义合法 edge case](../../docs/lessons/2026-05-12-state-sync-pet-less-noop-consistent-with-home-room-snapshot-14-1-r7.md)）：本 story 三处 pet-less 路径（RosterRow.PetID == nil / FindDefaultByUserID 返 ErrPetNotFound）**沿用既有实装** —— `r.PetID == nil → m.Pet = nil → JSON pet: null` / `petRow err ErrPetNotFound → pet = nil → member.joined.payload.pet = null`；pet-less 时 `currentState` 字段**不下发**（因 `pet` 整体为 null）；不视为业务错误
  - **r8**（[story 文件与 V1 doc 同步 self-broadcast 对称兜底 + `ts` 禁令 + 等价分层](../../docs/lessons/2026-05-12-story-file-must-stay-in-sync-with-frozen-v1-doc-14-1-r8.md)）：本 story 文件描述（Story / 故事定位 / Acceptance Criteria / Dev Notes / References）的 schema 引用 / 字段路径 / 三处落地范围**严格**与 V1 doc + 14-1 story 文件 + 14-1 lessons 三方对齐；review 阶段任何"story 文件 vs V1 doc drift"都必须在本 story 内修复
  - **r9**（[story 文件 RowsAffected 措辞 + 顶层 1003 引用 drift](../../docs/lessons/2026-05-12-story-file-rowsaffected-and-top-level-1003-drift-14-1-r9.md)）：本 story 文件全文 grep `RowsAffected` / `1003` 不应命中任何"业务路径触发"语义（与 14-1 r9 一致 —— 本 story 不引入 1003 / 不引入 RowsAffected）
  - **r10**（[Story AC 权威等价语义区分字段方向 client→server / server→client / ack-only](../../docs/lessons/2026-05-12-story-ac-authority-bucket-direction.md)）：本 story AC 描述时**严格区分**字段方向：(i) §10.3 GET response（server → client，权威）/ (ii) §12.3 room.snapshot（server → client，权威）/ (iii) §12.3 member.joined（server → client，权威）—— **三处都是 server → client 方向**，自本 story 起进入权威等价桶。**不**包括 §5.2 request `state`（client → server，写入信号）/ §5.2 response `data.state`（ack-only）。
  - **r11**（[story 文件 14.3 落地范围三处统一 + References 1003 残留清理](../../docs/lessons/2026-05-12-story-file-14-3-scope-must-list-member-joined-14-1-r11.md)）：**本 lesson 是本 story 的核心约束** —— Story 14.3 必须**同时**列出 (i) GET /rooms / (ii) room.snapshot / (iii) member.joined **三处**统一切换；漏 member.joined 是 r11 抓到的具体 finding。本 story AC1 / AC2 / AC3 / AC4 / AC5 直接对应这三处实装点。Story 文件全文 grep "三处" 应命中 ≥3 次（在 Story / 故事定位 / AC / Dev Notes / References 各段一致使用），不能用"两处"措辞。**不**包括 `pet.state.changed`（14.4 才落地，但属于权威等价桶四处之一）—— 14.3 三处 + 14.4 一处 = 14.4 完成后权威等价桶四处全部就绪

- **范围红线**（**严格遵守**，与 Story 11.10 / 11.7 / 11.8 "placeholder → 真实" 切换模式直接对照）：
  - 本 story **只**改：
    - `server/internal/repo/mysql/room_member_repo.go`（**扩展**；`RosterRow` struct 新增 `CurrentState *int8 gorm:"column:current_state"` 字段（紧接 PetID 字段；与 PetID 同 nullable pattern 用 `*int8` 接 LEFT JOIN NULL）+ `ListRosterByRoomID` SQL 在 `SELECT` 列表加 `pets.current_state AS current_state`（紧接 `pets.id AS pet_id` 之后；JOIN / WHERE / ORDER BY 子句完全不动）+ doc comment 同步更新提到 14.3 切换路径）
    - `server/internal/repo/mysql/room_member_repo_test.go`（**扩展**；既有 sqlmock case 已 assert SQL 模式 + 列输出，本 story 新增 1 case 验证 `current_state` 列被 select 出来 + happy 3 成员 currentState 真值映射；同时既有 case 的 ExpectQuery `WillReturnRows` 需补 `current_state` 列让既有 case 不破）
    - `server/internal/repo/mysql/room_member_repo_integration_test.go`（**扩展**；既有 dockertest case 已建 pets 表 + insert pet 行，本 story 新增 setup 让 3 pet 各自 current_state=1/2/3，验证 ListRosterByRoomID 返回 RosterRow.CurrentState 真值）
    - `server/internal/app/ws/snapshot.go`（**最小改动**；`realSnapshotBuilder.BuildSnapshot` 行 318 `CurrentState: 1` 改为 `CurrentState: int(*r.CurrentState)` —— 同 `r.PetID != nil` 分支内；`placeholderSnapshotBuilder.BuildSnapshot` 行 221 **保持不动**（Story 10.7 placeholder 形态 r14 锁定，仅测试便利路径，本 story 不回工）+ doc comment 同步更新提到 14.3 落地）
    - `server/internal/app/ws/snapshot_test.go`（**扩展**；新增 ≥4 case 覆盖 realSnapshotBuilder.BuildSnapshot 真值路径：happy 3 成员 pet.current_state 1/2/3 → SnapshotMember.Pet.CurrentState 对应映射 / pet-less 单成员 → SnapshotMember.Pet == nil（currentState 字段不存在）/ DB error 透传 / 0 成员空房间）；既有 placeholderSnapshotBuilder 测试**不动**（与 Story 11.7 保留模式一致）；既有 realSnapshotBuilder 4 个 happy / pet-less / DB error / 0 成员 case 需更新 stub `RosterRow.CurrentState` 字段让现有 assert 通过（happy case fixture 3 pet 都 current_state=1，与既有断言一致；其他 case 不变）
    - `server/internal/app/ws/snapshot_integration_test.go`（**扩展**；新增 1 dockertest case 覆盖：建 room + 3 成员（pet.current_state 分别 1/2/3）→ WS 客户端握手 → 收 room.snapshot → 验证 `members[].pet.currentState` wire 值 = 1/2/3；既有 case 不动）
    - `server/internal/service/room_service.go`（**最小改动**；(a) `roomServiceImpl.GetRoomDetail` 行 1266 `CurrentState: 1` 改为 `CurrentState: *r.CurrentState`（同 r.PetID != nil 分支内）；(b) `roomServiceImpl.broadcastMemberJoined` 行 1364 `CurrentState: 1` 改为 `CurrentState: int(petRow.CurrentState)`（同 petRow non-nil 分支内）+ doc comment 同步更新提到 14.3 切换路径）
    - `server/internal/service/room_service_test.go`（**扩展**；(a) 既有 `TestRoomService_GetRoomDetail_Happy_3Members_With1PetLess` case 更新 stub `RosterRow.CurrentState` 字段（happy case fixture 维持 current_state=1，断言不变）+ 新增 1 case happy 3 成员 currentState 1/2/3 → MemberPetOutput.CurrentState 对应映射；(b) 既有 broadcastMemberJoined happy case 更新 stub `FindDefaultByUserID` 返 `*mysql.Pet` 含 CurrentState=1（与现状一致）+ 新增 1 case happy state=2 → MemberJoinedPayload.Pet.CurrentState == 2）
    - `server/internal/service/room_service_integration_test.go`（**扩展**；新增 1 case 验证 GetRoomDetail end-to-end：建 room + 3 成员（pet.current_state 1/2/3）→ Output.Members[].Pet.CurrentState 对应；既有 case 不动 —— 既有 fixture insert pets 时 current_state 默认 1，与本 story 切换后断言"=1"一致）
    - `server/internal/app/http/handler/room_handler_test.go` / `server/internal/app/http/handler/room_handler_integration_test.go`（**最小扩展**；既有 GetRoomDetail handler case 沿用既有 stub service output（MemberPetOutput.CurrentState=1），断言"wire data.members[].pet.currentState=1"维持；新增 1 case happy service returns CurrentState=2 → wire data.members[].pet.currentState=2 验证 handler 层透传字段值无 mutation）
    - 本 story 文件（Status 流转）+ sprint-status.yaml（14-3-修改-roomsnapshotbuilder-snapshot-含真实-pet-currentstate: backlog → ready-for-dev → in-progress → review → done）
  - **不**改：
    - 任何 `docs/宠物互动App_*.md`（V1接口设计.md §10.3 / §12.3 + 数据库设计.md §5.3 / §6.4 + 时序图与核心业务流程设计.md + 总体架构.md + MVP 节点规划.md + Go 项目结构.md 是契约**输入**，本 story 严格对齐它们但**不修改**；V1 §10.3 line 1389 / §12.3 line 1988 / line 2121 / line 2252 已显式标注"自 Story 14.3 起切真实值"，**本 story 不需修改任何 V1 文档**）
    - 任何 ADR（ADR-0006 / ADR-0007 / ADR-0003 是契约**输入**，沿用不修改；本 story **不**新建 ADR —— 三处字段切换是契约钦定的实装动作，无新决策）
    - V1 接口契约（14.1 已冻结 §10.3 / §12.3 三处字段表 + §5.2 权威等价桶；本 story 严格按契约实装）
    - migrations 0003 / 0006（pets 表 + current_state 列 + rooms / room_members 已在 Story 4.3 / 11.2 落地；本 story 仅消费 schema，**不**改 SQL）
    - GORM `Pet` / `User` / `Room` / `RoomMember` struct 字段定义（11.7 / 4.6 已就绪；本 story 仅给 `RosterRow`（mysql 内部 struct，非 GORM table-mapped）加字段，**不**改 GORM tag / table struct）
    - `Pet` struct（pet_repo.go:24-30）字段已含 `CurrentState int8`（4.6 落地）；本 story 不动 Pet struct
    - PetRepo interface（pet_repo.go:48-83）已含 `FindDefaultByUserID` + `UpdateCurrentStateByID`（14.2 落地）；本 story **不加**任何 PetRepo 方法（broadcastMemberJoined 既有 FindDefaultByUserID 调用拿 `*Pet.CurrentState` 即可）
    - `Snapshot` / `SnapshotRoom` / `SnapshotMember` / `SnapshotPet` 4 struct 字段集合（10.7 r10 P1 钦定接口形态自此冻结；本 story 不加字段 / 不改 json tag / 不动 nullable pattern —— SnapshotPet.CurrentState 已是 `int json:"currentState"`，本 story 仅切赋值来源）
    - `SnapshotBuilder` interface 签名 / `SendRoomSnapshot` 函数（10.7 / 11.7 已稳定；本 story 不动）
    - `MemberJoinedPayload` / `MemberLeftPayload` / `BuildMemberJoinedEnvelope` / `BuildMemberLeftEnvelope`（11.8 落地；本 story 不动；MemberJoinedPayload.Pet 是 `*SnapshotPet` pointer，pet-less 时 nil → JSON null，与本 story 落地后 pet-less 路径一致）
    - `GetRoomDetailResponseData` / `GetRoomDetailResponseMember` / `GetRoomDetailResponseMemberPet` wire DTO struct（11.6 落地；本 story 不动；`CurrentState int8 json:"currentState"` 已是 int8 字段）
    - `MemberOutput` / `MemberPetOutput` / `EquipOutput` service DTO struct（room_service.go:174-198 已有，11.6 落地；本 story 不动）
    - `placeholderSnapshotBuilder` / `NewPlaceholderSnapshotBuilder`（10.7 落地 + 11.7 保留作测试便利；本 story 严格不动 —— placeholder 形态在 r14 锁定为"所有 member 下发 pet ≠ null + petId: "" + currentState: 1"，retro-active 切换 placeholder 会污染 r14 锁定的 Story 10.7 落地形态）
    - `realSnapshotBuilder` struct 定义 + `NewRealSnapshotBuilder` 构造（11.7 落地；本 story 仅改 BuildSnapshot 内 1 行赋值，不动 struct 字段 / 构造签名）
    - `RoomMemberRepo` interface 其他方法（`Create` / `CountByRoomID` / `DeleteByRoomAndUser` / `RoomExists` / `IsUserInRoom` / `ListMembers` / `ExistsForShareByRoomAndUser`）（10.x / 11.x 已稳定；本 story 仅给 `ListRosterByRoomID` 加 SELECT 列）
    - `RoomService` 其他方法（`CreateRoom` / `JoinRoom` / `LeaveRoom` / `GetCurrentRoom` / `broadcastMemberLeft` / `unregisterLeaverSessionSync`）（11.3-11.8 已稳定，本 story 不动）
    - `home_service.go` / `home_handler.go`（4.8 / 5.x / 11.10 已稳定；本 story 不影响 GET /home —— GET /home 拼装 `data.pet.currentState` 路径是 home_service.go:180 `int(pet.CurrentState)`，已是真实值，与本 story 无关）
    - `auth_service.go` / `auth_handler.go` / `step_service.go` / `dev_step_service.go` / `step_handler.go` / `pet_service.go` / `pets_handler.go`（4.6 / 7.x / 14.2 已稳定，本 story 不动）
    - WS 网关 / Gateway / Session / SessionManager / PresenceRepo / BroadcastFn primitive（10.x / 11.7 / 11.8 已稳定；本 story 不调任何 ws 包导出函数）
    - 其他 epic 范围的 service / handler / repo（4.6 / 4.8 / 7.x / 8.x / 14.2 全部不动）
    - `_bmad-output/` 下其他 yaml / md（除自己的 story 文件 + sprint-status.yaml 流转 + 可能的新 lesson md）
  - **不**实装：
    - **Story 14.4 pet.state.changed WS 广播**：service 层 `s.broadcastPetStateChanged` 方法 + ws 包 `BuildPetStateChangedEnvelope` + `PetStateChangedPayload` struct + 在 14.2 service 层 TODO 占位处加挂调用 —— 这些是 14.4 钦定范围；本 story **严格不**动 ws 包 / pet_service 层
    - **iOS Story 15.1 / 15.2 / 15.5**：iOS 端 PetSpriteView / RoomViewModel / pet.state.changed 处理 / 跨房间状态恢复 —— iOS Epic 15 范围；本 story 不动 iPhone 代码（`iphone/` 目录 0 改动；本 story 是纯 server 端 story）
    - **多 pet 切换 / pet.equips 真实驱动 / avatarUrl 在 room.snapshot 中下发**：节点 5 阶段每 user 单默认 pet，pet.equips 节点 9 / Epic 26 才真实驱动，avatarUrl 在 room.snapshot 中**不下发**（V1 §12.3 不含此字段，client 通过 GET /rooms 拿）—— 本 story 严格不引入这些
    - **placeholder builder 回工**：Story 10.7 placeholderSnapshotBuilder 在 r14 锁定形态后**不**走真实路径；本 story 不动 placeholder 实装 + 不动 placeholder 单测
    - **dev 端点（如 /dev/force-set-pet-state）**：节点 5 阶段 epics.md 未规划 pet dev 端点；本 story 不引入；如集成测试需要预置 pet.current_state，**直接** SQL `UPDATE pets SET current_state = ? WHERE id = ?` 注入 fixture（与现有 dockertest setup 模式一致）
    - **GORM AutoMigrate**：禁用（与 ADR-0003 §3.2 同源；本 story 不动 GORM table struct 字段）
    - **rate limit / handler 层校验改动**：本 story 是 read 路径切换（GET /rooms / WS snapshot / member.joined broadcast），handler 层不涉及；既有 handler 层校验沿用
    - **V1 文档 placeholder 标注移除**：V1 §10.3 line 1389 / §12.3 line 1988 / line 2121 当前措辞已包含"**自 Story 14.3 起切真实值**"+ placeholder `1` 描述；本 story 落地后**不**修改 V1 文档（文档已是 future-aware 锚定，14.3 落地后该锚定段不需要 retroactive 删除，作为历史 trace 保留；future 节点 9 / 10 同理 —— V1 §10.3 五阶段过渡表保留各节点 placeholder / 真实列）。如确有 review 反馈认为 V1 文档需 retroactive 改 placeholder 措辞，归 14-3 review 阶段单独 sweep；初次实装不动 V1 文档

## Acceptance Criteria

**AC1 — `RosterRow` struct 扩展 `CurrentState *int8` 字段 + `ListRosterByRoomID` SQL 增加 `pets.current_state` 列**

修改 `server/internal/repo/mysql/room_member_repo.go`：

- `RosterRow` struct 新增字段（紧接 PetID 字段；与 PetID nullable pattern 严格一致）：

  ```go
  type RosterRow struct {
      UserID       uint64  `gorm:"column:user_id"`
      Nickname     string  `gorm:"column:nickname"`
      AvatarURL    string  `gorm:"column:avatar_url"`
      PetID        *uint64 `gorm:"column:pet_id"`         // LEFT JOIN pets，pet-less 时为 nil
      CurrentState *int8   `gorm:"column:current_state"`  // LEFT JOIN pets.current_state，pet-less 时为 nil；Story 14.3 引入，14.3 前路径不消费
  }
  ```

  doc comment 同步更新：在既有"**不**包含 pet.currentState / pet.equips：节点 4 阶段固定 `1` / `[]`..."段后**追加** "Story 14.3（本 story）落地：`CurrentState *int8` 字段从 `pets.current_state` 列读取，pet-less 时 LEFT JOIN 行 NULL → GORM Scan 映射 *int8 nil；service 层在 `r.PetID != nil` 分支内直接解引 `*r.CurrentState`（同行 pet 非空 → pets.current_state NOT NULL DEFAULT 1，schema §6.4 钦定 → *r.CurrentState 必非 nil）"。

- `ListRosterByRoomID` impl 修改 SQL（仅 SELECT 列表加 1 列，JOIN / WHERE / ORDER BY 子句完全不动）：

  ```go
  func (r *roomMemberRepo) ListRosterByRoomID(ctx context.Context, roomID uint64) ([]RosterRow, error) {
      db := tx.FromContext(ctx, r.db)
      var rows []RosterRow
      err := db.WithContext(ctx).
          Raw(`SELECT room_members.user_id AS user_id, users.nickname AS nickname, users.avatar_url AS avatar_url, pets.id AS pet_id, pets.current_state AS current_state
               FROM room_members
               INNER JOIN users ON room_members.user_id = users.id
               LEFT JOIN pets ON pets.user_id = room_members.user_id AND pets.is_default = 1
               WHERE room_members.room_id = ?
               ORDER BY room_members.joined_at ASC`, roomID).
          Scan(&rows).Error
      // 错误处理 / 0 行 return 路径不动（与既有实装一致）
  }
  ```

  doc comment 在既有"**不**包含 pet.currentState / pet.equips..."段后**追加** "Story 14.3 落地后 SQL 增加 `pets.current_state AS current_state` 列输出，让 service 层在三处（GET /rooms / RoomSnapshotBuilder / 间接的 dockertest fixture）共享同一 query 路径拿到真实 currentState；pet.equips 仍由 Epic 26 / Story 26.6 真实驱动。"

**AC2 — `realSnapshotBuilder.BuildSnapshot` 切换 `CurrentState: 1` → `CurrentState: int(*r.CurrentState)`**

修改 `server/internal/app/ws/snapshot.go` `realSnapshotBuilder.BuildSnapshot`（第 309-322 行附近）：

- 在 `r.PetID != nil` 分支内 `m.Pet = &SnapshotPet{...}` 构造：把 `CurrentState: 1` 改为 `CurrentState: int(*r.CurrentState)`
- doc comment 第 270 行 `CurrentState: 1, // 节点 4 阶段固定 1 (rest)；Epic 14 / Story 14.3 真实驱动` 改为 `CurrentState: int(*r.CurrentState), // Story 14.3 落地：从 RosterRow.CurrentState 读真实值（pets.current_state；schema §6.4 NOT NULL DEFAULT 1 → *r.CurrentState 在 r.PetID != nil 分支内必非 nil）`
- 包级 doc comment 第 57 行 "+ CurrentState 仍硬编码 1（Epic 14 真实驱动）" 改为 "+ CurrentState 自 Story 14.3 起读真实 pets.current_state（placeholder 阶段仍硬编码 1）"
- `placeholderSnapshotBuilder.BuildSnapshot` 第 219-223 行 `CurrentState: 1` **保持不动**（Story 10.7 r14 锁定形态；本 story 不回工 placeholder）

**AC3 — `roomServiceImpl.GetRoomDetail` 切换 `CurrentState: 1` → `CurrentState: *r.CurrentState`**

修改 `server/internal/service/room_service.go` `roomServiceImpl.GetRoomDetail`（第 1262-1268 行附近）：

- 在 `r.PetID != nil` 分支内 `m.Pet = &MemberPetOutput{...}` 构造：把 `CurrentState: 1` 改为 `CurrentState: *r.CurrentState`（`MemberPetOutput.CurrentState` 是 `int8` 类型 + RosterRow.CurrentState 是 `*int8` → 直接解引）
- doc comment 第 1266 行 `CurrentState: 1, // V1 §10.3 节点 4 阶段固定 1 (rest)` 改为 `CurrentState: *r.CurrentState, // Story 14.3 落地：从 RosterRow.CurrentState 读真实值（V1 §10.3 line 1389 自 Story 14.3 起切真实值）`
- 方法级 doc comment 第 1210-1211 行 "**节点 4 硬编码字段**：MemberPetOutput.CurrentState 固定 1 (rest)；Equips 固定 []; 节点 5 / 9 / 10 由 Epic 14 / 26 / 29 真实驱动时改为 query 结果。" 改为 "**节点 5 / 9 / 10 真实驱动**：CurrentState 自 Story 14.3 起从 RosterRow.CurrentState 读真实值（Epic 14）；Equips 仍固定 []，由 Epic 26 真实驱动；renderConfig 由 Epic 29 真实驱动。"

**AC4 — `roomServiceImpl.broadcastMemberJoined` 切换 `CurrentState: 1` → `CurrentState: int(petRow.CurrentState)`**

修改 `server/internal/service/room_service.go` `roomServiceImpl.broadcastMemberJoined`（第 1361-1366 行附近）：

- 在 `petRow err == nil` 分支内 `pet = &ws.SnapshotPet{...}` 构造：把 `CurrentState: 1` 改为 `CurrentState: int(petRow.CurrentState)`（`SnapshotPet.CurrentState` 是 `int` 类型 + `mysql.Pet.CurrentState` 是 `int8` → 显式 cast）
- doc comment 第 1364 行 `CurrentState: 1, // V1 §12.3 节点 4 阶段固定 1 rest` 改为 `CurrentState: int(petRow.CurrentState), // Story 14.3 落地：从 mysql.Pet.CurrentState 读真实值（V1 §12.3 line 2121 自 Story 14.3 起切真实值）`
- 方法级 doc comment 第 1308-1311 行 "+ happy → pet=&ws.SnapshotPet{PetID: strconv.FormatUint(pet.ID, 10), CurrentState: 1}（节点 4 阶段固定 1 rest，V1 §12.3 钦定）" 改为 "+ happy → pet=&ws.SnapshotPet{PetID: strconv.FormatUint(pet.ID, 10), CurrentState: int(petRow.CurrentState)}（Story 14.3 落地：从 mysql.Pet.CurrentState 读真实值；V1 §12.3 line 2121 自 14.3 起切真实值）"

**AC5 — 单元测试覆盖（≥4 case）**

`server/internal/app/ws/snapshot_test.go` 新增 / 更新：

- **既有 4 case** (`TestRealSnapshotBuilder_BuildSnapshot_Happy_3Members_With1PetLess` / `_PetLess_SingleMember` / `_DBError_Propagates` / `_EmptyRoom_Returns0Members`)：stub `RosterRow` fixture **加 `CurrentState` 字段**（happy case 维持每个 non-pet-less row CurrentState=&int8(1)，与既有断言 `Pet.CurrentState == 1` 一致；pet-less case CurrentState=nil，断言 `Pet == nil` 不变；DB error / empty room 不需要 fixture 字段；既有 case **不破**）
- **新增 1 case `TestRealSnapshotBuilder_BuildSnapshot_Happy_3Members_CurrentState_1_2_3`**：
  - fixture: 3 rows，各自 RosterRow.PetID 非 nil + CurrentState = &int8(1) / &int8(2) / &int8(3)
  - 断言 SnapshotMember[i].Pet.CurrentState == 1 / 2 / 3 对应（同时复用既有断言确认 PetID 真实值 / Nickname 真实值）

`server/internal/repo/mysql/room_member_repo_test.go` 新增 / 更新：

- **既有 sqlmock case**（ListRosterByRoomID 路径）：`WillReturnRows` 在 column list 加 `"current_state"`（既有 RowValues 加一列 int8(1) 或 NULL；既有断言不破）+ SQL 模式 regexp 同步加 `current_state AS current_state` 列输出 expectation
- **新增 1 case `TestRoomMemberRepo_ListRosterByRoomID_ReturnsRealCurrentState_1_2_3`**：sqlmock 返 3 rows，current_state=1/2/3 → 断言 RosterRow[i].CurrentState != nil + *RosterRow[i].CurrentState == 1 / 2 / 3 对应

`server/internal/service/room_service_test.go` 新增 / 更新：

- **既有 `TestRoomService_GetRoomDetail_Happy_3Members_With1PetLess`**：stub RoomMemberRepo.ListRosterByRoomID 返 RosterRow 加 CurrentState 字段（happy fixture 维持 CurrentState=&int8(1)；既有断言不破）
- **新增 1 case `TestRoomService_GetRoomDetail_Happy_3Members_CurrentState_1_2_3`**：3 RosterRow 各自 CurrentState=&int8(1)/&int8(2)/&int8(3) → 断言 Output.Members[i].Pet.CurrentState == 1 / 2 / 3 对应
- **既有 broadcastMemberJoined happy case**：stub `petRepo.FindDefaultByUserID` 返 `&mysql.Pet{ID: ..., CurrentState: 1}`（既有 fixture 已默认 CurrentState=1 或 zero-value，本 story 显式设置 CurrentState=1）+ 既有断言 Payload.Pet.CurrentState == 1 不破
- **新增 1 case `TestRoomService_BroadcastMemberJoined_PetCurrentState_2`**：stub `petRepo.FindDefaultByUserID` 返 `&mysql.Pet{ID: 9001, CurrentState: 2}` → 断言 BuildMemberJoinedEnvelope 收到的 MemberJoinedPayload.Pet.CurrentState == 2（验证 broadcastMemberJoined 内 hardcode 1 → real currentState 切换正确）

**AC6 — 集成测试覆盖（dockertest，≥1 case）**

`server/internal/app/ws/snapshot_integration_test.go` 新增 1 case `TestRealSnapshotBuilder_BuildSnapshot_RealCurrentState_1_2_3_Integration`：

- setup（与既有 ws_integration_test.go startMySQLWithRoomMemberFixture helper 同模式）：
  - 创建 3 users (u1=1001, u2=1002, u3=1003)
  - 创建 1 room (r=3001, max=4, status=1, creator=u1)
  - 插入 3 room_members rows（按 joined_at 1/2/3 ASC 顺序）
  - 创建 3 pets (p1=9001 user_id=1001 is_default=1 current_state=1 / p2=9002 user_id=1002 is_default=1 current_state=2 / p3=9003 user_id=1003 is_default=1 current_state=3)
- exercise：构造 `realSnapshotBuilder{roomMember: realRoomMemberRepo}` + 调 `BuildSnapshot(ctx, 3001)`
- assert：
  - Snapshot.Room.MemberCount == 3
  - Snapshot.Members[0].Pet.CurrentState == 1 / [1].== 2 / [2].== 3 对应（按 joined_at ASC 顺序）
  - 各 Snapshot.Members[i].Pet.PetID == "9001" / "9002" / "9003" 字符串化
  - Snapshot.Members[i].Nickname == 真实 users.nickname（不再 placeholder）

`server/internal/service/room_service_integration_test.go` 新增 1 case `TestRoomService_GetRoomDetail_RealCurrentState_1_2_3_Integration`（同 fixture）：

- exercise：从 user_id=1001 视角 `svc.GetRoomDetail(ctx, GetRoomDetailInput{UserID: 1001, RoomID: 3001})`
- assert：
  - Output.Members[i].Pet.CurrentState == int8(1) / int8(2) / int8(3) 对应
  - Output.Members[i].Pet.PetID == 9001 / 9002 / 9003 对应

**AC7 — wire DTO 整合性维持（GetRoomDetailResponseMemberPet.CurrentState 字段类型不变）**

`server/internal/app/http/handler/room_handler.go`：

- `GetRoomDetailResponseMemberPet.CurrentState int8 json:"currentState"`（行 437）类型 / json tag **保持不动**（已是 int8 + json:"currentState"，与 V1 §10.3 line 1389 钦定 `number (int)` 类型一致 —— int8 序列化为 JSON 时仍是 `number` 类型 0~127 范围，1/2/3 在该范围内；client 端 Go decode 用 int8 / iOS Swift decode 用 Int 不区分）
- `getRoomDetailResponseDTO` 行 519 `CurrentState: m.Pet.CurrentState` **保持不动**（service → wire DTO 透传 int8 值，本 story service 层赋值已切真实，handler 层自动透传真实值）

新增 / 更新 handler 层测试 `server/internal/app/http/handler/room_handler_test.go`：

- **既有 GetRoomDetail happy case**：sub-service stub 返 MemberPetOutput.CurrentState=1 维持，断言 wire `data.members[].pet.currentState` == 1 不变
- **新增 1 case `TestRoomHandler_GetRoomDetail_PetCurrentState_2_Passthrough`**：stub service 返 MemberPetOutput.CurrentState=2 → 断言 wire `data.members[].pet.currentState` == 2（验证 handler 层透传 service 层赋值的真实 currentState 值，无 mutation / 无 hardcode）

**AC8 — 范围红线 grep 自检（review 阶段 PASS 条件）**

实装完成 commit 前自检：

- `grep -rn "CurrentState: 1" server/internal/app/ws/snapshot.go`：仅 1 命中（placeholderSnapshotBuilder 第 221 行）；如有其他命中（如 realSnapshotBuilder 残留）→ FAIL
- `grep -rn "CurrentState: 1" server/internal/service/room_service.go`：0 命中（GetRoomDetail / broadcastMemberJoined 两处都已切换）
- `grep -rn "// V1 §10.3 节点 4 阶段固定 1" server/internal/service/room_service.go`：0 命中（doc comment 已同步更新）
- `grep -rn "// V1 §12.3 节点 4 阶段固定 1 rest" server/internal/service/room_service.go`：0 命中
- `grep -rn "// V1 §12.3 节点 4 阶段固定 1 (rest)" server/internal/app/ws/snapshot.go`：仅 1 命中（placeholderSnapshotBuilder 第 222 行）
- `grep -n "current_state" server/internal/repo/mysql/room_member_repo.go`：≥3 命中（RosterRow 字段 gorm tag + SQL SELECT 列 + doc comment 提及）
- 文件改动数：**改 6 个文件**（room_member_repo.go / room_member_repo_test.go / room_member_repo_integration_test.go / snapshot.go / snapshot_test.go / snapshot_integration_test.go / room_service.go / room_service_test.go / room_service_integration_test.go / room_handler_test.go）+ story 文件 + sprint-status.yaml（合计 11 文件改动；与 11.10 placeholder→真实切换 story 量级相当）
- iPhone 代码 0 改动（`grep -rn "14.3\|Story 14.3" iphone/` 不应命中本 story commit 改动）

## Tasks / Subtasks

- [x] Task 1: RosterRow 字段扩展 + ListRosterByRoomID SQL 加 current_state 列 (AC1)
  - [x] 1.1 `server/internal/repo/mysql/room_member_repo.go`: RosterRow struct 加 `CurrentState *int8 gorm:"column:current_state"` 字段（紧接 PetID 字段；与 PetID 同 nullable pattern）
  - [x] 1.2 同文件 ListRosterByRoomID impl SQL SELECT 列表加 `pets.current_state AS current_state`（紧接 `pets.id AS pet_id` 之后）
  - [x] 1.3 同文件 doc comment（RosterRow struct 注释 + ListRosterByRoomID impl 注释）同步更新提到 14.3 切换路径
  - [x] 1.4 `bash scripts/build.sh` 验证 vet + build 通过（不需要 --test）

- [x] Task 2: realSnapshotBuilder.BuildSnapshot 切换 currentState 真值赋值 (AC2)
  - [x] 2.1 `server/internal/app/ws/snapshot.go`: realSnapshotBuilder.BuildSnapshot `CurrentState: 1` → `CurrentState: int(*r.CurrentState)`
  - [x] 2.2 同文件 doc comment（Snapshot / SnapshotPet struct + realSnapshotBuilder 方法级）同步更新；placeholder builder 注释保持不动
  - [x] 2.3 `bash scripts/build.sh` 验证 vet + build 通过

- [x] Task 3: roomServiceImpl.GetRoomDetail 切换 currentState 真值赋值 (AC3)
  - [x] 3.1 `server/internal/service/room_service.go`: GetRoomDetail `CurrentState: 1` → `CurrentState: *r.CurrentState`
  - [x] 3.2 同文件 doc comment（方法级 + inline）同步更新提到 14.3 切换路径

- [x] Task 4: roomServiceImpl.broadcastMemberJoined 切换 currentState 真值赋值 (AC4)
  - [x] 4.1 `server/internal/service/room_service.go`: broadcastMemberJoined `CurrentState: 1` → `CurrentState: int(petRow.CurrentState)`
  - [x] 4.2 同文件 doc comment（方法级 + inline）同步更新提到 14.3 切换路径
  - [x] 4.3 `bash scripts/build.sh` 验证 vet + build 通过

- [x] Task 5: 单元测试覆盖 (AC5)
  - [x] 5.1 `server/internal/app/ws/snapshot_test.go`: 既有 4 case stub RosterRow fixture 加 CurrentState 字段（既有断言不破）
  - [x] 5.2 同文件新增 `TestRealSnapshotBuilder_BuildSnapshot_Happy_3Members_CurrentState_1_2_3` case
  - [x] 5.3 `server/internal/repo/mysql/room_member_repo_test.go`: 既有 sqlmock case 加 current_state 列 + 新增 `TestRoomMemberRepo_ListRosterByRoomID_ReturnsRealCurrentState_1_2_3` case
  - [x] 5.4 `server/internal/service/room_service_test.go`: 既有 `TestRoomService_GetRoomDetail_Happy_3Members_With1PetLess` 更新 stub fixture (CurrentState=&1)
  - [x] 5.5 同文件新增 `TestRoomService_GetRoomDetail_Happy_3Members_CurrentState_1_2_3` case
  - [x] 5.6 同文件新增 `TestRoomService_BroadcastMemberJoined_PetCurrentState_2` case（既有 happy case mysql.Pet.CurrentState=1 fixture 已显式设置 14.2 落地时即如此）
  - [x] 5.7 `bash scripts/build.sh --test` 验证全部单测通过

- [x] Task 6: 集成测试覆盖 (AC6)
  - [x] 6.1 `server/internal/app/ws/snapshot_integration_test.go`: 新增 `TestRealSnapshotBuilder_BuildSnapshot_RealCurrentState_1_2_3_Integration` case
  - [x] 6.2 `server/internal/service/room_service_integration_test.go`: 新增 `TestRoomServiceIntegration_GetRoomDetail_RealCurrentState_1_2_3` case
  - [x] 6.3 ~~room_member_repo_integration_test.go 改动~~：repo 层无独立集成测试文件（仅 sqlmock 单测 + 由 ws / service 集成测试间接覆盖），AC8 文件清单已认；新增 case 沿用 startMySQLWithRoomMemberFixture 已 seed 路径
  - [x] 6.4 `go test -tags=integration` 验证新增 2 case 通过（dockertest MySQL 真实执行）

- [x] Task 7: handler 层 wire DTO 透传断言 (AC7)
  - [x] 7.1 `server/internal/app/http/handler/room_handler_test.go`: 既有 GetRoomDetail happy case 维持 + 新增 `TestRoomHandler_GetRoomDetail_PetCurrentState_2_Passthrough` case
  - [x] 7.2 `bash scripts/build.sh --test` 验证 handler 单测通过

- [x] Task 8: 范围红线 grep 自检 + 提交
  - [x] 8.1 grep `CurrentState: 1` 在 snapshot.go 仅 placeholder builder 路径 3 命中（doc + impl）；real builder 0 命中 ✓
  - [x] 8.2 grep `CurrentState: 1` 在 room_service.go 0 命中 ✓
  - [x] 8.3 grep `current_state` 在 room_member_repo.go 5 命中（≥3 阈值）✓
  - [x] 8.4 旧 doc comment 措辞（`V1 §10.3 节点 4 阶段固定 1`/`V1 §12.3 节点 4 阶段固定 1`）在 room_service.go 0 命中 ✓
  - [x] 8.5 `bash scripts/build.sh --test` 最终验证全单测通过；race detector 在本机 cgo 不可用故跳过（与既有 ADR-0001 测试栈一致，非本 story 引入限制）
  - [x] 8.6 集成测试通过（受限于 dockertest 串行 + Windows Docker startup 较慢，全套 `--integration` 单次超 600s；按测试名分批验证均通过）
  - [x] 8.7 sprint-status.yaml 更新 14-3 状态从 in-progress → review

## Dev Notes

### 三处统一切换的关键不变量

**为什么三处同一 story 落地（V1 §1 line 46 + 14-1 r11 lesson 钦定）**：
- 用户在房间外切 walk/run → §5.2 服务端逻辑步骤 5 `users.current_room_id == NULL` → 仅 UPDATE pets.current_state，**不**广播 pet.state.changed
- 该用户随后 join 房间 X
- 房间内已在场成员通过 `member.joined` 收到该用户加入事件
- 如果 `member.joined.payload.pet.currentState` 还是 placeholder `1` → 房间内其他成员**永远**看到 stale `1`，直到该用户**再次** state-sync 才触发 pet.state.changed 广播
- 同理：reconnect 后收 `room.snapshot.members[].pet.currentState` 是 placeholder `1` → 同一 stale 风险
- 同理：调 GET /rooms/{roomId}.data.members[].pet.currentState 是 placeholder `1` → 同一 stale 风险

**三处切换的 data source 一致性**：
- §10.3 GET + §12.3 room.snapshot：都通过 `ListRosterByRoomID` 拿 `RosterRow`（共享同一 SQL 路径，本 story 给 RosterRow 加 CurrentState 字段后两路自动同步真实）
- §12.3 member.joined：通过 `FindDefaultByUserID` 拿 `*mysql.Pet`（4.6 已 ship 的 PetRepo 方法 + Pet struct 已含 CurrentState 字段，本 story 仅切赋值）

**SQL 改动的最小集**：
- 仅 SELECT 列表加 1 列 `pets.current_state AS current_state`
- JOIN 子句不动（INNER JOIN users + LEFT JOIN pets 路径已 ship）
- WHERE / ORDER BY 子句不动
- LEFT JOIN 语义保留 → pet-less 时 pets.* 列 NULL → RosterRow.CurrentState 为 nil → service 层在 `r.PetID != nil` 分支内安全解引（pet 行存在 → schema §6.4 钦定 current_state NOT NULL DEFAULT 1，必非 NULL）

### pet-less 路径的 nil-safety 不变量

`RosterRow.PetID != nil` 在所有三个 service 代码点是 `*RosterRow.CurrentState` 解引的 **充分必要条件**：
- 必要条件：LEFT JOIN 语义 + schema constraint —— pet 行存在 → pet.current_state 必非 NULL（数据库设计 §6.4 钦定 `current_state TINYINT NOT NULL DEFAULT 1`）→ GORM Scan 必映射 *int8 非 nil
- 充分条件：pet 行不存在 → LEFT JOIN 整行 NULL → `pets.id AS pet_id` 和 `pets.current_state AS current_state` 都 NULL → GORM 同时映射 PetID / CurrentState 为 nil
- 这两条共同保证：`if r.PetID != nil` 分支内 `*r.CurrentState` 解引永不 panic（与既有 `*r.PetID` 解引同 safety 保证）

`broadcastMemberJoined` 路径：FindDefaultByUserID 返 `*mysql.Pet`：
- `err == ErrPetNotFound` → pet=nil（既有路径不动）
- `err == nil` → petRow 非 nil + petRow.CurrentState 是 `int8` 值类型（不是 pointer，schema constraint 保证非 NULL）→ `int(petRow.CurrentState)` 直接 cast 安全

### Story 改动的物理位置（review grep 锚点）

| 文件 | 行号（基于当前 main HEAD） | 既有内容 | 改后内容 |
|---|---|---|---|
| `room_member_repo.go` | RosterRow struct 100-105 | `PetID *uint64 gorm:"column:pet_id"` 后无 CurrentState 字段 | 加 `CurrentState *int8 gorm:"column:current_state"` 字段 |
| `room_member_repo.go` | ListRosterByRoomID 504 | `SELECT ... pets.id AS pet_id` | 加 `, pets.current_state AS current_state` |
| `snapshot.go` | realSnapshotBuilder.BuildSnapshot 318 | `CurrentState: 1` | `CurrentState: int(*r.CurrentState)` |
| `room_service.go` | GetRoomDetail 1266 | `CurrentState: 1` | `CurrentState: *r.CurrentState` |
| `room_service.go` | broadcastMemberJoined 1364 | `CurrentState: 1` | `CurrentState: int(petRow.CurrentState)` |

行号是参考值（fix-review 阶段可能因 doc comment 编辑微调）；定位以"`CurrentState: 1` 在 r.PetID != nil / petRow err == nil 分支内"语义为准。

### V1 文档 placeholder 措辞的 retroactive 处理（本 story 不做）

V1 §10.3 line 1389 / §12.3 line 1988 / line 2121 当前措辞已是"自 Story 14.3 起切真实值"+ "节点 4 阶段固定 `1`" + placeholder 描述。本 story **不**修改 V1 文档：
- 措辞已是 future-aware 锚定，14.3 落地后保留作历史 trace
- 未来节点 9 / 10 由 Epic 26 / 29 落地 pet.equips / renderConfig 时同样不需要 retroactive 删除 placeholder 标注
- 如有 review 反馈认为 V1 文档需更新（如把 "**自 Story 14.3 起切真实值**" 改为 "**自 Story 14.3 起切真实值，已 ship**"），归 14.3 review 阶段 sweep；初次实装严格不动 V1 文档

### 测试矩阵的覆盖完整性

| 层级 | 文件 | 既有 case 改动 | 新增 case |
|---|---|---|---|
| repo 单测 | room_member_repo_test.go | 既有 sqlmock case 加 current_state 列 | TestRoomMemberRepo_ListRosterByRoomID_ReturnsRealCurrentState_1_2_3 |
| repo 集成 | room_member_repo_integration_test.go | 既有 case 加 pet.current_state setup | （沿用既有 case fixture 扩展即可，不一定新增） |
| ws 单测 | snapshot_test.go | 既有 4 case stub 加 CurrentState | TestRealSnapshotBuilder_BuildSnapshot_Happy_3Members_CurrentState_1_2_3 |
| ws 集成 | snapshot_integration_test.go | 既有 case 不动 | TestRealSnapshotBuilder_BuildSnapshot_RealCurrentState_1_2_3_Integration |
| service 单测 | room_service_test.go | 既有 GetRoomDetail / broadcastMemberJoined case 加 stub CurrentState | TestRoomService_GetRoomDetail_Happy_3Members_CurrentState_1_2_3 + TestRoomService_BroadcastMemberJoined_PetCurrentState_2 |
| service 集成 | room_service_integration_test.go | 既有 case fixture 不动 | TestRoomService_GetRoomDetail_RealCurrentState_1_2_3_Integration |
| handler 单测 | room_handler_test.go | 既有 GetRoomDetail case 维持 | TestRoomHandler_GetRoomDetail_PetCurrentState_2_Passthrough |

**总计 ≥5 新增 case + ≥5 既有 case 改 stub**（合计 AC5 / AC6 / AC7 三 AC 的"≥4 单测 + ≥1 集成测试"epics.md 钦定要求）。

### Project Structure Notes

- Server 工程结构与 Go 项目结构.md §6.3 / §6.4 一致（既有 mysql / service / ws / handler 三层切分；本 story 仅改既有文件，不新建 package / 不新建 file）
- 本 story 是 server-only story；`iphone/` 目录 0 改动；`ios/` 目录（旧 watchOS 归档）0 改动
- ADR-0007 ctx 传播：本 story 所有改动路径 ctx 已稳定（既有 repo / service 调用都已传 ctx；本 story 不新增方法签名 → 不引入 ctx 传播改动）

### References

- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 14.3 (行 2321-2339)] — 本 story 主 epics 锚定（AC + 单测 / 集成测试钦定）
- [Source: `docs/宠物互动App_V1接口设计.md` §1 (line 46 / 49)] — 14.3 落地范围三处统一（GET /rooms + room.snapshot + member.joined）锚定 + 节点 5 真实驱动 Future Fields 声明
- [Source: `docs/宠物互动App_V1接口设计.md` §5.2 (line 608-613)] — 字段语义跨章节等价分两层声明 + 权威等价桶四处 server → client 字段枚举 + Story 14.3 前临时窗口权威信号优先级 + Story 14.3 落地后权威等价层生效
- [Source: `docs/宠物互动App_V1接口设计.md` §10.3 (line 1389)] — `data.members[].pet.currentState` wire DTO 钦定（number int 1/2/3 / 来源 pets.current_state / 节点 4 阶段固定 1 / Epic 14 才真实驱动 motion_state）
- [Source: `docs/宠物互动App_V1接口设计.md` §10.3 五阶段过渡表 (line 1458-1470)] — `pet.currentState` 节点 5 真实列钦定 "**同一 epic 落地点 Story 14.3 同时覆盖 §12.3 `room.snapshot` + `member.joined` 两处 `pet.currentState` 字段**，三处 server → client `pet.currentState` 同步切真实路径"
- [Source: `docs/宠物互动App_V1接口设计.md` §12.3 (line 1988)] — `room.snapshot.payload.members[].pet.currentState` 字段表 + "**自 Story 14.3 起切真实值**" 锚定
- [Source: `docs/宠物互动App_V1接口设计.md` §12.3 (line 2092 + 2097)] — client merge contract 第 3 条数值字段直接覆盖规则 + Story 14.3 落地前临时窗口例外 + Story 14.3 落地后例外失效声明
- [Source: `docs/宠物互动App_V1接口设计.md` §12.3 (line 2121)] — `member.joined.payload.pet.currentState` 字段表 + "**自 Story 14.3 起切真实值**" 锚定 + stale `1` race 说明
- [Source: `docs/宠物互动App_V1接口设计.md` §12.3 (line 2252)] — `pet.state.changed.payload.currentState` 权威等价桶四处 server → client 字段枚举 + Story 14.3 前临时窗口权威信号优先级（同 §5.2 line 612 一致）
- [Source: `docs/宠物互动App_数据库设计.md` §6.4] — pets.current_state TINYINT NOT NULL DEFAULT 1 + 状态枚举 1=rest / 2=walk / 3=run
- [Source: `docs/宠物互动App_数据库设计.md` §5.3] — uk_user_default_pet (user_id, is_default) 唯一约束 + 每用户最多 1 默认 pet
- [Source: `_bmad-output/implementation-artifacts/11-6-房间详情查询.md`] — RoomMemberRepo.ListRosterByRoomID + RosterRow struct 落地（本 story 在此基础上加 CurrentState 字段）
- [Source: `_bmad-output/implementation-artifacts/11-7-房间快照真实实现.md`] — realSnapshotBuilder.BuildSnapshot 落地（本 story 在此基础上把硬编码 1 切真实）
- [Source: `_bmad-output/implementation-artifacts/11-8-成员加入-离开-ws-广播.md`] — broadcastMemberJoined 落地（本 story 在此基础上把硬编码 1 切真实）+ MemberJoinedPayload struct + BuildMemberJoinedEnvelope helper（不动）
- [Source: `_bmad-output/implementation-artifacts/11-10-get-home-扩展-room-currentroomid-真实数据.md`] — placeholder → 真实切换 story 同模式参考（本 story 与 11.10 是结构镜像 story）
- [Source: `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md`] — V1 §5.2 / §12.3 契约锚定 + 14.3 落地范围三处统一钦定段
- [Source: `_bmad-output/implementation-artifacts/14-2-post-pets-current-state-sync-接口-pets-current_state-更新.md`] — POST /pets/current/state-sync 写库实装（本 story 是该写路径的 read-side 真实驱动 counterpart）
- [Source: `docs/lessons/2026-05-12-state-sync-idempotent-rowsaffected-and-ws-envelope-ts.md`] — 14-1 r1 lesson（幂等 + RowsAffected + WS envelope ts）
- [Source: `docs/lessons/2026-05-12-cross-section-equivalence-claim-must-fence-prerequisites-and-self-broadcast-fallback-2.md`] — 14-1 r2 lesson（跨章节字段等价分层 + ack vs 权威分层）
- [Source: `docs/lessons/2026-05-12-member-joined-stale-state-and-self-broadcast-arrival-order-symmetric-3.md`] — 14-1 r3 lesson（member.joined 14.3 落地前 stale race，本 story 修复对应物）
- [Source: `docs/lessons/2026-05-12-self-broadcast-ui-driver-and-freeze-boundary-and-self-vs-others-priority-4.md`] — 14-1 r4 lesson（self vs 他人优先级，client 实装范围红线）
- [Source: `docs/lessons/2026-05-12-merge-contract-exception-and-ts-business-ordering-ban-and-ack-bucket-explicit-enum-5.md`] — 14-1 r5 lesson（临时窗口 merge contract 例外 + 权威等价桶四处显式枚举，本 story 落地后例外失效）
- [Source: `docs/lessons/2026-05-12-state-sync-err-binary-and-placeholder-whitelist-self-http-ack-14-1-r6.md`] — 14-1 r6 lesson（err 二分 + placeholder 白名单）
- [Source: `docs/lessons/2026-05-12-state-sync-pet-less-noop-consistent-with-home-room-snapshot-14-1-r7.md`] — 14-1 r7 lesson（pet-less noop 与 /home / room / member.joined 同语义）
- [Source: `docs/lessons/2026-05-12-story-file-must-stay-in-sync-with-frozen-v1-doc-14-1-r8.md`] — 14-1 r8 lesson（story 文件与 V1 doc 同步）
- [Source: `docs/lessons/2026-05-12-story-file-rowsaffected-and-top-level-1003-drift-14-1-r9.md`] — 14-1 r9 lesson（story 文件 RowsAffected + 1003 drift 清理）
- [Source: `docs/lessons/2026-05-12-story-ac-authority-bucket-direction.md`] — 14-1 r10 lesson（AC 权威等价语义区分字段方向 client→server / server→client / ack-only）
- [Source: `docs/lessons/2026-05-12-story-file-14-3-scope-must-list-member-joined-14-1-r11.md`] — **14-1 r11 lesson（Story 14.3 落地范围三处统一，本 story 核心约束直接对应物）**
- [Source: `_bmad-output/implementation-artifacts/decisions/0006-error-mapping.md`] — ADR-0006 错误码三层映射（本 story 不引入新错误码）
- [Source: `_bmad-output/implementation-artifacts/decisions/0007-ctx-propagation.md`] — ADR-0007 ctx 传播（本 story 不引入新 ctx 路径）
- [Source: `_bmad-output/implementation-artifacts/decisions/0011-ws-stack.md`] — ADR-0011 WS stack（本 story 不动 ws 包导出接口）

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context)

### Debug Log References

- 单测：`bash scripts/build.sh --test` PASS（24 packages）
- 集成测试：分批运行（dockertest 启动 MySQL 容器较慢，全套 `--integration` 单次超过 build.sh 默认 120s 与扩展 600s 阈值；按 test name pattern 分批运行均 PASS）：
  - `go test -tags=integration -run "TestRoomServiceIntegration_GetRoomDetail" ./internal/service/...` → 4 case 全 PASS（含新增 `TestRoomServiceIntegration_GetRoomDetail_RealCurrentState_1_2_3` 21.54s）
  - `go test -tags=integration -run "TestRealSnapshotBuilder_BuildSnapshot" ./internal/app/ws/...` → 既有 + 新增 case 全 PASS（含新增 `TestRealSnapshotBuilder_BuildSnapshot_RealCurrentState_1_2_3_Integration` 18.28s）

### Completion Notes List

- **AC1 落地**：`RosterRow` struct 新增 `CurrentState *int8` 字段（与 PetID `*uint64` 同 nullable pattern）+ `ListRosterByRoomID` SQL SELECT 列表加 `pets.current_state AS current_state` 列输出。JOIN / WHERE / ORDER BY 子句严格不动。
- **AC2 落地**：`realSnapshotBuilder.BuildSnapshot` 把 hardcoded `CurrentState: 1` 改为 `CurrentState: int(*r.CurrentState)`，pet-less 时 `r.PetID == nil` 分支保持 `m.Pet = nil`（JSON `"pet": null`）。`placeholderSnapshotBuilder` 严格不动（Story 10.7 r14 锁定形态）。
- **AC3 落地**：`roomServiceImpl.GetRoomDetail` 把 hardcoded `CurrentState: 1` 改为 `CurrentState: *r.CurrentState`。
- **AC4 落地**：`roomServiceImpl.broadcastMemberJoined` 把 hardcoded `CurrentState: 1` 改为 `CurrentState: int(petRow.CurrentState)`（`mysql.Pet.CurrentState` 是 `int8` 值类型，schema NOT NULL DEFAULT 1 保证非 NULL）。
- **AC5 落地**：单测 ≥4 case：snapshot_test.go +1 case (`Happy_3Members_CurrentState_1_2_3`) + room_member_repo_test.go +1 case (`ReturnsRealCurrentState_1_2_3`) + room_service_test.go +2 case (`GetRoomDetail_Happy_3Members_CurrentState_1_2_3` + `BroadcastMemberJoined_PetCurrentState_2`) + 既有 case stub fixture 更新（既有断言不破）。
- **AC6 落地**：dockertest 集成测试 ≥1 case：snapshot_integration_test.go +1 case (`RealCurrentState_1_2_3_Integration`) + room_service_integration_test.go +1 case (`GetRoomDetail_RealCurrentState_1_2_3`)。
- **AC7 落地**：handler 层透传断言 +1 case (`TestRoomHandler_GetRoomDetail_PetCurrentState_2_Passthrough`)；`GetRoomDetailResponseMemberPet.CurrentState int8` 字段类型 / json tag 不动。
- **AC8 grep 自检 PASS**：placeholder builder 仅 1 处实际 `CurrentState: 1`（snapshot.go:223）；real builder + room_service 全部 0 命中；旧 doc comment 措辞清理；`current_state` 在 room_member_repo.go 5 命中 ≥3 阈值；iPhone 代码 0 改动。
- **三处统一**：(i) GET /rooms/{roomId} （room_service.GetRoomDetail）/ (ii) room.snapshot （snapshot.realSnapshotBuilder）/ (iii) member.joined （room_service.broadcastMemberJoined）三处实装点全部完成 placeholder `1` → 真实 `pets.current_state` 切换（14-1 r11 lesson 钦定的核心约束）。
- **pet-less 安全**：(i)+(ii) 走 LEFT JOIN pets pet-less → RosterRow.PetID/CurrentState 同步 nil → service 层 `r.PetID != nil` 分支内安全解引；(iii) 走 FindDefaultByUserID `ErrPetNotFound` → pet=nil 路径不读 CurrentState；3 处都不引入 nil panic 风险。
- **range red lines 遵守**：iPhone 代码 0 改动；V1 文档 / ADR 0 改动；migrations 0 改动；Pet/User/Room/RoomMember GORM struct 0 改动；PetRepo / RoomMemberRepo interface 0 新增方法；SnapshotPet / SnapshotMember / MemberJoinedPayload / GetRoomDetailResponseMemberPet wire DTO 0 改动。

### File List

修改的 server 代码文件（10 文件）+ story 流转 metadata 文件（2 文件）：

**Production code（4 文件）**：

- `server/internal/repo/mysql/room_member_repo.go`：RosterRow struct 加 `CurrentState *int8` 字段 + ListRosterByRoomID SQL SELECT 加 `pets.current_state AS current_state` 列 + doc comment 更新
- `server/internal/app/ws/snapshot.go`：realSnapshotBuilder.BuildSnapshot `CurrentState: 1` → `int(*r.CurrentState)` + Snapshot / SnapshotPet struct doc comment 更新 + 包级 doc comment 更新；placeholderSnapshotBuilder 严格不动
- `server/internal/service/room_service.go`：(a) GetRoomDetail `CurrentState: 1` → `*r.CurrentState`；(b) broadcastMemberJoined `CurrentState: 1` → `int(petRow.CurrentState)` + 两处 doc comment 更新

**Test code（5 文件）**：

- `server/internal/repo/mysql/room_member_repo_test.go`：既有 3 个 sqlmock case 列名 + AddRow 加 current_state 列；新增 `TestRoomMemberRepo_ListRosterByRoomID_ReturnsRealCurrentState_1_2_3`
- `server/internal/app/ws/snapshot_test.go`：既有 happy case fixture 加 CurrentState 字段；新增 `int8Ptr` helper + `TestRealSnapshotBuilder_BuildSnapshot_Happy_3Members_CurrentState_1_2_3`
- `server/internal/app/ws/snapshot_integration_test.go`：新增 `TestRealSnapshotBuilder_BuildSnapshot_RealCurrentState_1_2_3_Integration`
- `server/internal/service/room_service_test.go`：既有 `TestRoomService_GetRoomDetail_Happy_3Members_With1PetLess` 加 CurrentState fixture；新增 `TestRoomService_GetRoomDetail_Happy_3Members_CurrentState_1_2_3` + `TestRoomService_BroadcastMemberJoined_PetCurrentState_2`
- `server/internal/service/room_service_integration_test.go`：新增 `TestRoomServiceIntegration_GetRoomDetail_RealCurrentState_1_2_3`
- `server/internal/app/http/handler/room_handler_test.go`：新增 `TestRoomHandler_GetRoomDetail_PetCurrentState_2_Passthrough`

**Metadata（2 文件）**：

- `_bmad-output/implementation-artifacts/14-3-修改-roomsnapshotbuilder-snapshot-含真实-pet-currentstate.md`：Status 流转（ready-for-dev → in-progress → review）+ Tasks/Subtasks checkbox + Dev Agent Record + File List + Change Log
- `_bmad-output/implementation-artifacts/sprint-status.yaml`：14-3 状态 ready-for-dev → in-progress → review + last_updated comment

### Change Log

| Date | Author | Description |
|---|---|---|
| 2026-05-12 | Claude Opus 4.7 (1M context) | Story 14.3 实装：三处 server → client `pet.currentState` 字段统一切换 placeholder `1` → 真实 `pets.current_state`（GET /rooms / room.snapshot / member.joined）；新增 ≥4 单测 case + ≥1 dockertest 集成测试 case；范围红线 grep 自检 PASS；Status → review |
