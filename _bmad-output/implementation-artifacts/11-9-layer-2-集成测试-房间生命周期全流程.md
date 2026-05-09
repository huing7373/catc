# Story 11.9: Layer 2 集成测试 — 房间生命周期全流程（dockertest 真实 MySQL + Redis + 真实 SessionManager 跨 service / handler / WS 三层穷举 epics.md §11.9 钦定 10 类场景：完整生命周期 / 3 类回滚 / 2 类并发 / 3 类边界 / 1 类 WS 联动；**不**实装新业务功能，仅扩展 integration test 覆盖矩阵）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 资产事务负责人,
I want 一组深度集成测试覆盖**房间创建 / 加入 / 退出 / 关闭**事务的失败回滚 / 并发 / 边界 / WS 联动 / 跨接口端到端，全部用 dockertest 真实 MySQL + Redis 跑通，作为节点 4 Layer 2 集成测试收尾保障，**追加** 到 11.3 ~ 11.8 已落地的 23 个 service 层集成测试 + 11 个 handler 层集成测试基础上，把覆盖率从局部 happy / error 路径推到"全生命周期 + 全失败模式 + 高并发收敛 + 跨接口 schema 端到端 + WS 真实推送验证"五个维度全绿,
so that NFR1（资产事务原子）和数据库设计.md §8.6（加入房间事务）/ §8.7（退出房间事务）+ V1 §10.1 / §10.2 / §10.3 / §10.4 / §10.5 + §12.3 `### 成员加入` / `### 成员离开` 在节点 4 阶段不只靠 11.3 ~ 11.8 已有的局部 case，而是**穷举** epics.md §Story 11.9 行 1998-2020 钦定的 1 完整生命周期 + 3 回滚 + 2 并发 + 3 边界 + 1 WS 联动共**10 类场景**，把覆盖率从单事务 happy 路径推到事务全失败模式 + 高并发收敛 + 跨接口端到端 + WS 实拨真断言 4 个维度全绿；任何一个场景退化（如某条回滚路径漏 rollback / 100 goroutine race 出现脏行 / 边界 5 人 join 在 service 层未拦截 / WS 联动收不到 member.left）→ 立即在 Layer 2 阶段被发现，**不**让节点 4 验收 demo 阶段（Epic 13 跨端集成）才暴露房间事务幂等性 / 跨接口契约 / WS 链路完整性回归。

## 故事定位（Epic 11 第九条 = 节点 4 收尾性 Layer 2 集成测试；上承 11.3 创建 / 11.4 加入 / 11.5 退出 / 11.6 详情查询 / 11.7 房间快照真实 / 11.8 成员加入-离开 WS 广播 + close 4007；下启 11.10 GET /home 扩展）

- **Epic 11 进度**：11.1 (契约定稿，done) → 11.2 (rooms / room_members migration + GORM domain，done) → 11.3 (POST /rooms 创建房间事务，done) → 11.4 (POST /rooms/{roomId}/join 加入房间事务，done) → 11.5 (POST /rooms/{roomId}/leave 退出房间事务，done) → 11.6 (GET /rooms/current + GET /rooms/{roomId} 房间详情查询，done) → 11.7 (room.snapshot 真实实装替换 E10.7 placeholder，done) → 11.8 (成员加入/离开 WS 广播 + close 4007 unregister leaver Session，done) → **11.9（本 story，Layer 2 集成测试 - 房间生命周期全流程）** → 11.10（GET /home 扩展 room.currentRoomId 真实数据）。

- **物理执行顺序与逻辑编号一致**：本 story 编号 11.9，物理上**第九**执行（11.3-11.8 done 后立刻做 11.9）。理由：
  - Story 11.9 是 epic-11 的**收尾性 Layer 2 集成测试**，需要 11.3 (CreateRoom) / 11.4 (JoinRoom) / 11.5 (LeaveRoom) / 11.6 (GetCurrentRoom + GetRoomDetail) / 11.7 (room.snapshot 真实) / 11.8 (member.joined / member.left 广播 + close 4007) 六条业务链路全部落地后再做整体回归
  - sprint-status.yaml 第 154 行已按此顺序排列（11.9 在 11.8 之后、11.10 之前）
  - 11.9 是测试 story，**不实装新业务功能**，仅扩展 integration test coverage 矩阵；与 4.7（auth_service Layer 2 收尾）/ 20.9（开箱事务收尾）/ 26.5（穿戴事务收尾）/ 32.5（合成事务收尾）同模式

- **epics.md §Story 11.9 钦定**（`_bmad-output/planning-artifacts/epics.md` 行 1998-2020，**唯一权威 AC 来源**）：
  - **Given** Story 11.3 ~ 11.8 happy path 已通过
  - **When** 完成本 story
  - **Then** 输出扩展 `internal/service/room_service_integration_test.go`（已存在，11.3 ~ 11.8 落地共 23 个测试函数）+ `internal/app/http/handler/room_handler_integration_test.go`（已存在，11.3 ~ 11.6 落地共 11 个测试函数），**追加** 10 类场景（不新建独立测试文件，与 4.7 同模式 —— 同包同文件内聚）：

    | epics.md 行 | 场景类别 | 详细要求 |
    |---|---|---|
    | 行 2009 | **完整生命周期** | A 创建 → B/C/D 依次 join → 4 人满 → 第 5 个 E join 返回 6002 → A leave → B 仍在 → 全部 leave → 房间 closed |
    | 行 2010 | **回滚 1**（创建房间） | mock room_members repo 第 2 步抛 error → 验证 rooms 也回滚（DB 表为空） |
    | 行 2011 | **回滚 2**（加入房间） | mock users.current_room_id update 失败 → 验证 room_members 也回滚 |
    | 行 2012 | **回滚 3**（退出房间） | mock users 更新失败 → 验证 room_members 删除也回滚（用户仍在房间） |
    | 行 2013 | **并发 1** | 4 个用户已在房间，5 个用户同时 join → 只有 1 个成功，其他 4 个全部返回 6002 |
    | 行 2014 | **并发 2** | 100 个不同用户同时 create + join 100 个不同房间 → 全部成功，DB rooms 100 行 |
    | 行 2015 | **边界 1** | 用户 A 在房间 X，A 又调 POST /rooms（创建新房间）→ 6003 |
    | 行 2016 | **边界 2** | 用户 A 在房间 X，A 调 POST /rooms/X/join（加入自己已在的房间）→ 6003 |
    | 行 2017 | **边界 3** | 房间最后一人 leave → 房间 closed + 第 N 个用户尝试 join 该 closed 房间 → 6005 |
    | 行 2018 | **WS 联动** | A + B 在房间，A 建 WS → B leave → A 收到 member.left |

  - 全部场景用 dockertest 真实 MySQL + Redis 跑通（**不**用 sqlmock —— 业务上是 Layer 2 黑盒事务行为验证，不是 SQL 字符串验证）
  - 集成测试在 CI 标 `//go:build integration` + `// +build integration` 双行 tag（与 11.3 ~ 11.8 / 4.7 同模式）

- **范围边界**（**关键** —— 与 11.3 ~ 11.8 已落地集成测试的明确分工）：

  **11.3 ~ 11.8 service 层集成测试已落地 23 case**（`server/internal/service/room_service_integration_test.go`，全部 done；通过 `grep -n "func TestRoomServiceIntegration_"` 列举）：
  - 11.3：`TestRoomServiceIntegration_CreateRoom_Happy_3RowsInserted` / `_AlreadyInRoom_PrecheckReturns6003` / `_RollsBackOnRoomMemberInsertFail`（3 case，本 story **回滚 1** 复用同一 fault injection 思路 + 推到 100 goroutine 并发场景）
  - 11.4：`TestRoomServiceIntegration_JoinRoom_Happy_2RowsAfterJoin` / `_RoomFull_Returns6002` / `_RoomNotFound_Returns6001` / `_RoomClosed_Returns6005` / `_CrossTxLeaveSerialized`（5 case，本 story **完整生命周期** / **并发 1** / **边界 2** / **边界 3** 在此基础上扩展）
  - 11.5：`TestRoomServiceIntegration_LeaveRoom_NotLastMember_RoomActive` / `_LastMember_RoomClosed` / `_UserNotInRoom_Returns6004` / `_DoubleLeave_SecondReturns6004` / `_CrossTxJoinSerialized`（5 case，本 story **完整生命周期** / **回滚 3** 复用 fixture 模式）
  - 11.6：`TestRoomServiceIntegration_GetCurrentRoom_Happy_UserInRoom` / `_UserNotInAnyRoom` / `TestRoomServiceIntegration_GetRoomDetail_Happy_3Members_With1PetLess` / `_UserNotInRoom_Returns6004` / `_ClosedRoom_CallerAlreadyLeft_Returns6004`（5 case，本 story **完整生命周期** end-to-end 链路在此基础上验证 GET /rooms/{id} 在每个生命周期阶段返回正确字段）
  - 11.7：handler 层 + ws 包 placeholder snapshot integration test（不在 room_service_integration_test.go 内；本 story **WS 联动** 直接消费 ws_integration_test.go 的真实 WS 拨号 helper）
  - 11.8：`TestRoomServiceIntegration_JoinRoom_Happy_BroadcastFnInvokedOnce` / `TestRoomServiceIntegration_LeaveRoom_Happy_BroadcastsMemberLeft` / `_LastMember_StillBroadcasts`（3 case，本 story **WS 联动** 在 capture broadcastFn 模式之上推到"真实 WS Session + 真实 BroadcastToRoom 网关"的 end-to-end 路径）

  **11.3 ~ 11.6 handler 层集成测试已落地 11 case**（`server/internal/app/http/handler/room_handler_integration_test.go`，全部 done）：
  - 11.3：`TestRoomHandlerIntegration_CreateRoom_HappyPath` / `_NoToken_Returns1001` / `_AlreadyInRoom_Returns6003`
  - 11.4：`TestRoomHandlerIntegration_JoinRoom_HappyPath` 等 4 case
  - 11.5：`TestRoomHandlerIntegration_LeaveRoom_HappyPath` 等 2 case
  - 11.6：`TestRoomHandlerIntegration_GetCurrentRoom_HappyPath_UserInRoom` / `_UserNotInAnyRoom` / `TestRoomHandlerIntegration_GetRoomDetail_HappyPath` / `_UserNotInRoom_Returns6004`

  **本 story 任务是扩展上述两份文件追加 ≥10 个新 case + 1 个跨包 WS 联动测试**：

  | epics.md 钦定场景 | 测试函数命名 | 落地文件 | 与既有 case 关系 |
  |---|---|---|---|
  | 完整生命周期 | `TestRoomServiceIntegration_FullLifecycle_5UsersJoinFullLeaveAllClosed` | `service/room_service_integration_test.go` | **新增**（11.4 _RoomFull / 11.5 _LastMember_RoomClosed 是局部断言；本 case 跨 7 个事务 + 5 user + 跨 status transition） |
  | 完整生命周期 (HTTP 端到端) | `TestRoomHandlerIntegration_FullLifecycle_E2E_HTTPCreateJoinLeaveDetail` | `handler/room_handler_integration_test.go` | **新增**（HTTP 跨接口 envelope schema 完整链路） |
  | 回滚 1 (创建 room_members 失败) | `TestRoomServiceIntegration_CreateRoom_FaultInjectionRoomMembersStep_RoomsAlsoRollback` | `service/room_service_integration_test.go` | **新增**（11.3 _RollsBackOnRoomMemberInsertFail 用 raw INSERT 撞 UNIQUE 间接验证；本 case 用 fault injection wrapper 直接验证 mock error 路径，与 4.7 fault injection 模式同源） |
  | 回滚 2 (加入 users update 失败) | `TestRoomServiceIntegration_JoinRoom_FaultInjectionUsersUpdate_RoomMembersAlsoRollback` | `service/room_service_integration_test.go` | **新增** |
  | 回滚 3 (退出 users update 失败) | `TestRoomServiceIntegration_LeaveRoom_FaultInjectionUsersUpdate_RoomMembersDeleteAlsoRollback` | `service/room_service_integration_test.go` | **新增** |
  | 并发 1 (5 user 同时 join 满房间) | `TestRoomServiceIntegration_JoinRoom_Concurrent5UsersIntoFullRoom_OnlyOneSucceeds` | `service/room_service_integration_test.go` | **强化** 11.4 _CrossTxLeaveSerialized（跨 leave + join 2 goroutine）→ 5 join goroutine 全并发 |
  | 并发 2 (100 user 创建 100 房间) | `TestRoomServiceIntegration_CreateRoom_Concurrent100DifferentUsers_100RoomsCreated` | `service/room_service_integration_test.go` | **新增**（与 4.7 _Concurrent100DifferentGuestUIDs_NoCrossData 同模式） |
  | 边界 1 (A 在房间 → 再 POST /rooms) | 复用 11.3 `_AlreadyInRoom_PrecheckReturns6003`（已落地）| 同上 | **复用 + 文档化对应关系**（不新增测试函数；本 story 在 README 注释里指向已落地 case） |
  | 边界 2 (A 在房间 X → POST /rooms/X/join) | `TestRoomServiceIntegration_JoinRoom_UserAlreadyInSameRoom_Returns6003` | `service/room_service_integration_test.go` | **新增**（11.4 既有 case 仅覆盖"A 在 room X → 调 POST /rooms/Y/join 加入**别的**房间 → 6003"；本 case 验证 A 在 X 调 X/join 也返 6003，**rationale**：service 层预检 step 1 `users.current_room_id != nil` 即返 6003，不区分目标 roomID 是否同 X） |
  | 边界 3 (closed 房间被 join) | 复用 11.4 `_RoomClosed_Returns6005`（已落地）| 同上 | **复用 + 文档化对应关系**（不新增测试函数；本 story 在 README 注释里指向已落地 case） |
  | WS 联动 | `TestRoomServiceIntegration_LeaveRoom_RealWSSession_BroadcastsMemberLeftToOtherMembers` | `service/room_service_integration_test.go` | **新增**（11.8 _BroadcastsMemberLeft 用 capture broadcastFn 验证调用次数；本 case 用真实 SessionManager + 真实 BroadcastToRoom 网关 + 真实 gorilla.Dial WS 拨号 → A 真断言 conn.ReadMessage 收到 member.left wire） |

  **关键设计约束**：
  - 全部 ≥10 case 必须挂 `//go:build integration` + `// +build integration` 双行 tag（与 11.3 ~ 11.8 / 4.7 同模式）
  - **回滚 1/2/3 必须用 fault injection wrapper repo**（不能用 stub repo，理由同 4.7 AC2：stub 不真开 InnoDB 事务无法验证 rollback 真行为）；wrapper 模式与 4.7 落地的 `faultPetRepo` / `faultChestRepo` / `faultUserRepo` 完全同模式（按方法包装真实 mysql repo + 在指定方法上替换为 sentinel error）
  - **完整生命周期 service-layer case** 跨 7 个事务（A.create + B.join + C.join + D.join + E.join → reject + A.leave + B.leave + C.leave + D.leave）—— **关键不变量** 每步后跟一次 `fetchRoomMemberCount` + `fetchRoomStatus` + `fetchUserCurrentRoomID` 三连断言；**rooms.status transition 从 1 active → 2 closed 必须由"最后一人 leave"事件触发**（11.5 _LastMember_RoomClosed 已断言一次；本 case 在跨 7 事务全链路中再断言一次）
  - **完整生命周期 HTTP-layer case** 用真 router + envelope 解码端到端验证 `POST /api/v1/rooms` → `POST /api/v1/rooms/{id}/join`（×3）→ `POST /api/v1/rooms/{id}/leave`（×4）→ `GET /api/v1/rooms/current`（×多次）→ `GET /api/v1/rooms/{id}`（每个 lifecycle 阶段 1 次）全部走通；envelope.code = 0 对 success 路径，envelope.code = 6002 对第 5 个 join，envelope.code = 6004 对 user 离开后再调 GET /rooms/{id}
  - **并发 1 必须 5 goroutine 并发 join 同 room** —— epics.md 行 2013 钦定"4 个用户已在房间，5 个用户同时 join → 只有 1 个成功"；rationale：4 + 5 = 9 中只有 1 个能成功使房间满员，其他 4 个看到 count=4 → 6002（FOR UPDATE 锁串行化让 5 goroutine 严格排队）；本 case 在 11.4 _CrossTxLeaveSerialized 跨 leave + join 2 goroutine 模式之上推到 5 join goroutine 全并发
  - **并发 2 必须 100 goroutine 并发 create + join** —— 不同 user / 不同房间 → 100 个事务全部独立成功；DB 最终 rooms.count = 100 + room_members.count = 100；与 4.7 _Concurrent100DifferentGuestUIDs_NoCrossData 同模式（不同 key 空间无串数据 → 验证事务彼此独立）
  - **WS 联动 case 必须真 gorilla.Dial 拨号** —— 不能用 capture broadcastFn 替代（11.8 case 14/15/16 已用 capture 验证调用次数；本 case 验证"广播 wire 真到达 client"，跳出 SessionManager 内存索引层）；fixture：A 真 token 真拨号 WS → B leave 真事务 → A.conn.ReadMessage 真收 member.left wire；本 case **跨 service 包 + ws 包**，复用 ws_integration_test.go 的 startGatewayWithRealMySQL helper 模式（但本 case 在 service 包内，需要把 helper 抽到本测试文件 / 或独立到 cross-package WS integration helper —— 见 AC8 钦定）

- **下游依赖**：本 story 是 epic 11 收尾前置，**不**直接服务下游 story；但本 story 的 fault injection 模式（包装真 repo + sentinel error 注入）+ 跨 service 包 WS 拨号模式（service 测试 import gorilla/websocket）成为 future Layer 2 集成测试的范式（如 Story 14.4 pet.state.changed 集成测试 / Story 17.5 emoji.received 集成测试 / Story 20.9 开箱事务集成测试 / Story 26.5 穿戴事务集成测试 / Story 32.5 合成事务集成测试 都钦定相同 Layer 2 模式）

**本 story 不做**（明确范围红线）：

- [skip] **不**修改 `server/internal/service/room_service.go`（11.3 ~ 11.8 已 done；本 story 仅消费）
- [skip] **不**修改 `server/internal/app/http/handler/room_handler.go`（11.3 ~ 11.6 已 done；本 story 仅消费）
- [skip] **不**修改 5 个 mysql repo（user / room / room_member / pet / chest，11.2 ~ 11.6 已 done；本 story 仅消费 + 包装做 fault injection）
- [skip] **不**修改 0007 / 0008 migration（11.2 已 done；本 story 仅消费）
- [skip] **不**修改 `server/internal/repo/tx/manager.go`（4.2 已 done；本 story 仅消费 `WithTx`）
- [skip] **不**修改 `server/internal/app/ws/*`（10.3 ~ 10.7 + 11.7 + 11.8 已 done；本 story 仅消费 SessionManager / BroadcastToRoom / Gateway / SnapshotBuilder 既有 API）
- [skip] **不**修改 4.5 中间件（auth + rate_limit；本 story 在 HTTP 端到端 case 中通过 router wire 间接消费）
- [skip] **不**修改 bootstrap router（**不**新增 deps 字段；不挂新路由）
- [skip] **不**修改 11.3 ~ 11.8 已有的 23 个 service 集成测试函数（保持现有 done 状态测试不破坏 —— 仅在同一份 `room_service_integration_test.go` 文件**追加** ≥10 个新 case）
- [skip] **不**修改 11.3 ~ 11.6 已有的 11 个 handler 集成测试函数（保持现有 done 状态 —— 仅在 `room_handler_integration_test.go` **追加** 1 个完整生命周期 E2E case）
- [skip] **不**新建跨包 testing util（不抽 startMySQL 到 internal/testutil/ —— 与 4.7 / 11.3 ~ 11.8 同模式，复制 helper 到本测试文件即可，避免范围扩散）
- [skip] **不**用 sqlmock（与 4.7 / 11.3 ~ 11.8 同模式；epics.md 行 2019 钦定"全部场景用 dockertest 真实 MySQL + Redis"；sqlmock 测的是 SQL 字符串匹配，与本 story Layer 2 黑盒行为验证语义不符）
- [skip] **不**改 `docs/宠物互动App_*.md` 任一份（V1 §10.x / §12.3 / 数据库设计 §8.6 / §8.7 / 时序图 §11.x / §12.2 是契约**输入**，本 story 严格对齐**不**修改）
- [skip] **不**改任何 ADR（ADR-0006 / ADR-0007 / ADR-0011 / ADR-0003 是契约**输入**，沿用不修改；本 story **不**新建 ADR —— 与 4.7 / 11.3 ~ 11.8 同模式，无需新决策）
- [skip] **不**写 README / 部署文档：留 Epic 11 收尾或 Story 13.3 文档同步阶段
- [skip] **不**实装 audit log（依赖 Logging 中间件兜底；与 4.7 同模式）
- [skip] **不**修改 `server/configs/local.yaml`（不引入新配置项）
- [skip] **不**修改 `server/cmd/server/main.go`（不加新 deps）
- [skip] **不**支持 `go test -short`（dockertest 必跑；本 story ≥10 case 全部 `+build integration`，默认 `bash scripts/build.sh --test` 不触发；只在 `bash scripts/build.sh --integration` 触发）
- [skip] **不**实装"测试容器复用"优化（每 case 独立 startMySQL 容器，与 4.7 / 11.3 ~ 11.8 同模式，简单 + 一致性优于性能）；优化方向留 future 性能 epic
- [skip] **不**给 Story 11.9 加 sprint-status.yaml 占位 retrospective（epic 11 retrospective 已在 sprint-status.yaml 第 156 行 `epic-11-retrospective: optional`，本 story done 后整 epic done 才考虑触发）
- [skip] **不**做 fuzz / property-based testing（dockertest case 已穷举 epics.md 钦定 10 类；fuzz 是 future testing 升级范畴）
- [skip] **不**测 ctx cancel / timeout 路径（ADR-0007 ctx 传播是 4.2 ~ 11.8 已建立的范式，本 story 不重复验证）
- [skip] **不**测 deadlock / 隔离级别 anomaly（InnoDB 默认 REPEATABLE READ + 11.4 _CrossTxLeaveSerialized / 11.5 _CrossTxJoinSerialized 已断言 cross-tx race 串行化；本 story 不深挖隔离级别专项）
- [skip] **不**测 11.10 GET /home 扩展 room.currentRoomId 路径（属于 Story 11.10 范围）
- [skip] **不**测 14.x pet.state.changed / 17.x emoji.received WS 业务事件（属于 Epic 14 / 17 范围）
- [skip] **不**实装 100 goroutine 并发场景的"性能基准"断言（仅断言行为正确性 = 100 个事务全部成功 + DB 行数 = 100；不断言 latency / throughput）
- [skip] **不**给 service / handler 加新方法签名 / 新业务码路径（service 公开 API 在 11.3 ~ 11.8 已冻结）

## Acceptance Criteria

**AC1 — 测试文件位置 + build tag + helper 复用 + 包名一致**

本 story 在已有的两份测试文件**追加** 测试函数；**不**新建独立测试文件。

- 落地文件 1：`server/internal/service/room_service_integration_test.go`（11.3 ~ 11.8 落地共 23 函数 + 配套 helper：`buildRoomServiceIntegration` / `buildRoomServiceIntegrationWithCapture` / `fetchRoomCount` / `fetchRoomMemberCount` / `fetchRoomStatus` / `fetchUserCurrentRoomID` / `assertCount` / `insertUser` / `insertPet` / `captureBroadcastFn`）
- 落地文件 2：`server/internal/app/http/handler/room_handler_integration_test.go`（11.3 ~ 11.6 落地共 11 函数 + 配套 helper：`buildRoomHandlerIntegration` / `roomIntegrationTestStartMySQL` / `roomIntegrationTestRunMigrations` / `roomIntegrationTestRouter` / `roomIntegrationTestInsertUser` / `roomIntegrationTestSignToken` / `decodeRoomIntegrationEnvelope`）

**关键约束**：

- **build tag**：所有新 case 必须挂 `//go:build integration` + `// +build integration` 双行标记（与 4.7 / 11.3 ~ 11.8 同模式 + Go 1.17+ 双语法兼容）；放在文件顶部（11.3 ~ 11.8 已写）
- **helper 复用**：直接消费同包同文件已有的 helper（11.3 ~ 11.8 实装完整，不新增 helper 函数 —— 仅在 AC8 WS 联动 case 内必要时新增 1 个 cross-package WS 拨号 helper，详见 AC8）
- **不抽包**：`startMySQL` / `roomIntegrationTestStartMySQL` 等 helper 仍在各自测试文件内（每 case 独立起容器，与 4.7 / 11.3 ~ 11.8 同模式）
- **包名不变**：`package service_test`（service 集成测试） / `package handler_test`（handler 集成测试），与既有同
- **fault injection wrapper**：本 story 新增 3 个按方法包装的 wrapper struct（`faultRoomMemberRepo` / `faultUserRepoForJoin` / `faultUserRepoForLeave`）放在 `room_service_integration_test.go` 文件内，与 4.7 落地的 `faultPetRepo` / `faultChestRepo` / `faultUserRepo` 同模式（按方法包装真实 mysql repo + 在指定方法上替换为 sentinel error，其他方法透传）

**关键反模式**：

- 不新建 `room_service_integration_lifecycle_test.go` / `room_service_integration_concurrency_test.go` 等拆文件（保持 11.3 ~ 11.8 单文件内聚 —— 一份测试文件视图覆盖所有场景，便于 reviewer 一目了然，与 4.7 钦定同模式）
- 不在本测试文件内新加跨包 testing util（边界 / WS 联动等需要的 helper 全部 inline 落地到本文件内）
- 不用 sqlmock（epics.md 行 2019 钦定）

**AC2 — 完整生命周期（service 层 + HTTP 层 双实装）**

**AC2.1 service 层测试函数**：`TestRoomServiceIntegration_FullLifecycle_5UsersJoinFullLeaveAllClosed`（**新增**）

跨 7 个事务全链路：A 创建 → B / C / D 依次 join → E join 返 6002 → A leave → B leave → C leave → D leave（最后一人）→ rooms.status = 2 closed。每步后必须**三连断言**：

- `fetchRoomMemberCount(t, sqlDB)` 期望值（A creator → 1, +B → 2, +C → 3, +D → 4, +E reject → 4 不变, A leave → 3, B leave → 2, C leave → 1, D leave → 0）
- `fetchRoomStatus(t, sqlDB, roomID)` 期望值（前 8 步 = 1 active；最后 D leave 后 = 2 closed）
- `fetchUserCurrentRoomID(t, sqlDB, userX)` 对每个 user 期望值（join 后 = roomID, leave 后 = nil, E reject 后 = nil 不变）

```go
// AC2.1 测试函数（位于 room_service_integration_test.go 末尾追加）
func TestRoomServiceIntegration_FullLifecycle_5UsersJoinFullLeaveAllClosed(t *testing.T) {
    svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
    defer cleanup()

    const userA = uint64(1001)
    const userB = uint64(1002)
    const userC = uint64(1003)
    const userD = uint64(1004)
    const userE = uint64(1005)
    insertUser(t, sqlDB, userA, "uid-life-a", "A", "")
    insertUser(t, sqlDB, userB, "uid-life-b", "B", "")
    insertUser(t, sqlDB, userC, "uid-life-c", "C", "")
    insertUser(t, sqlDB, userD, "uid-life-d", "D", "")
    insertUser(t, sqlDB, userE, "uid-life-e", "E", "")

    // 阶段 1: A 创建 → memberCount=1, status=1, A.current_room_id=roomID
    createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
    if err != nil { t.Fatalf("CreateRoom A: %v", err) }
    roomID := createOut.RoomID
    assertCount(t, sqlDB, "room_members WHERE room_id=?", []any{roomID}, 1, "after A create")
    if got := fetchRoomStatus(t, sqlDB, roomID); got != 1 {
        t.Errorf("after A create rooms.status = %d, want 1", got)
    }

    // 阶段 2-4: B / C / D 依次 join → memberCount 升到 4
    for i, u := range []uint64{userB, userC, userD} {
        if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: u, RoomID: roomID}); err != nil {
            t.Fatalf("JoinRoom user=%d: %v", u, err)
        }
        wantCount := int64(2 + i)
        // ... 三连断言（memberCount / status / users.current_room_id）
    }

    // 阶段 5: E join → 6002 + memberCount 仍 4
    _, err = svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userE, RoomID: roomID})
    if err == nil { t.Fatalf("E join returned nil, want 6002") }
    ae, ok := apperror.As(err)
    if !ok || ae.Code != apperror.ErrRoomFull {
        t.Errorf("E join AppError = %v, want 6002", err)
    }
    assertCount(t, sqlDB, "room_members WHERE room_id=?", []any{roomID}, 4, "after E reject")

    // 阶段 6-8: A / B / C 依次 leave → memberCount 降到 1
    // 阶段 9: D leave (最后一人) → memberCount=0 + status=2 + D.current_room_id=nil
    for i, u := range []uint64{userA, userB, userC, userD} {
        if _, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: u, RoomID: roomID}); err != nil {
            t.Fatalf("LeaveRoom user=%d: %v", u, err)
        }
        wantCount := int64(3 - i)
        wantStatus := int8(1)
        if i == 3 {
            wantStatus = 2  // 最后一人 leave → closed
        }
        // ... 三连断言（memberCount=wantCount / status=wantStatus / leaver.current_room_id=nil）
    }

    // 收尾断言：rooms.status=2 + room_members 0 行 + 5 user 全部 current_room_id=nil
    if got := fetchRoomStatus(t, sqlDB, roomID); got != 2 {
        t.Errorf("final rooms.status = %d, want 2 closed", got)
    }
    for _, u := range []uint64{userA, userB, userC, userD, userE} {
        if got := fetchUserCurrentRoomID(t, sqlDB, u); got != nil {
            t.Errorf("final user %d current_room_id = %d, want nil", u, *got)
        }
    }
}
```

**关键设计约束**：每步三连断言**必须** inline 而非抽 helper 函数（让 reviewer 阅读时一眼看到每个生命周期点的预期 state；与 4.7 _Concurrent100DifferentGuestUIDs_NoCrossData 内联断言模式同源）。

**AC2.2 handler 层测试函数**：`TestRoomHandlerIntegration_FullLifecycle_E2E_HTTPCreateJoinLeaveDetail`（**新增**）

跨 ≥9 次 HTTP 调用：A `POST /rooms` → B/C/D `POST /rooms/{id}/join` ×3 → E `POST /rooms/{id}/join` 期望 6002 → A `POST /rooms/{id}/leave` → A `GET /rooms/current` 期望 roomId=null → B `GET /rooms/{id}` 期望成员列表含 B/C/D 不含 A → B/C `POST /rooms/{id}/leave` ×2 → D `POST /rooms/{id}/leave`（最后一人）→ D `GET /rooms/{id}` 期望 6004（用户已离开 closed 房间）。

每个 HTTP response 必须解析 envelope 并断言：
- `envelope.code == 0` 对 success 路径，`envelope.code == 6002` 对 E 第 5 个 join，`envelope.code == 6004` 对 D 离开后 GET /rooms/{id}
- success 路径的 `envelope.data` 字段值与 V1 §10.x 字段表 1:1 对齐（数字字段是 `float64` JSON 解析后类型；BIGINT id 是 string）

```go
// AC2.2 测试函数（位于 room_handler_integration_test.go 末尾追加）
func TestRoomHandlerIntegration_FullLifecycle_E2E_HTTPCreateJoinLeaveDetail(t *testing.T) {
    router, sqlDB, signer, cleanup := buildRoomHandlerIntegration(t)
    defer cleanup()

    const userA, userB, userC, userD, userE = uint64(1001), uint64(1002), uint64(1003), uint64(1004), uint64(1005)
    roomIntegrationTestInsertUser(t, sqlDB, userA, "uid-e2e-a", "A")
    roomIntegrationTestInsertUser(t, sqlDB, userB, "uid-e2e-b", "B")
    roomIntegrationTestInsertUser(t, sqlDB, userC, "uid-e2e-c", "C")
    roomIntegrationTestInsertUser(t, sqlDB, userD, "uid-e2e-d", "D")
    roomIntegrationTestInsertUser(t, sqlDB, userE, "uid-e2e-e", "E")

    tokenA := roomIntegrationTestSignToken(t, signer, userA)
    tokenB := roomIntegrationTestSignToken(t, signer, userB)
    tokenC := roomIntegrationTestSignToken(t, signer, userC)
    tokenD := roomIntegrationTestSignToken(t, signer, userD)
    tokenE := roomIntegrationTestSignToken(t, signer, userE)

    // 1) A POST /rooms → envelope.code=0 + room.id 非空
    roomIDStr := callCreateExpectingOK(t, router, tokenA)

    // 2-4) B / C / D POST /rooms/{id}/join → envelope.code=0 (×3)
    for _, tok := range []string{tokenB, tokenC, tokenD} {
        callJoinExpectingOK(t, router, tok, roomIDStr)
    }

    // 5) E POST /rooms/{id}/join → envelope.code=6002
    callJoinExpectingCode(t, router, tokenE, roomIDStr, apperror.ErrRoomFull)

    // 6) A POST /rooms/{id}/leave → envelope.code=0
    callLeaveExpectingOK(t, router, tokenA, roomIDStr)

    // 7) A GET /rooms/current → envelope.data.roomId == nil
    if got := callGetCurrentRoom(t, router, tokenA); got != nil {
        t.Errorf("A current room after leave = %v, want nil", *got)
    }

    // 8) B GET /rooms/{id} → envelope.data.members 长度 = 3 不含 A
    detail := callGetRoomDetailExpectingOK(t, router, tokenB, roomIDStr)
    if len(detail.Members) != 3 {
        t.Errorf("len(members) = %d, want 3", len(detail.Members))
    }
    // ... 断言不含 A

    // 9-10) B / C 依次 leave → envelope.code=0
    callLeaveExpectingOK(t, router, tokenB, roomIDStr)
    callLeaveExpectingOK(t, router, tokenC, roomIDStr)

    // 11) D leave (最后一人) → envelope.code=0 + 后续断言 rooms.status=2
    callLeaveExpectingOK(t, router, tokenD, roomIDStr)

    // 12) D GET /rooms/{id} → envelope.code=6004 (closed 房间 caller 已离开)
    callGetRoomDetailExpectingCode(t, router, tokenD, roomIDStr, apperror.ErrUserNotInRoom)

    // 收尾 DB 断言：rooms.status=2 + room_members 0 行
    var roomIDInt uint64
    fmt.Sscan(roomIDStr, &roomIDInt)
    var dbStatus int8
    if err := sqlDB.QueryRow("SELECT status FROM rooms WHERE id = ?", roomIDInt).Scan(&dbStatus); err != nil {
        t.Fatalf("query rooms.status: %v", err)
    }
    if dbStatus != 2 { t.Errorf("DB status = %d, want 2 closed", dbStatus) }
}

// callCreateExpectingOK / callJoinExpectingOK / callJoinExpectingCode / callLeaveExpectingOK /
// callLeaveExpectingCode / callGetCurrentRoom / callGetRoomDetailExpectingOK / callGetRoomDetailExpectingCode
// 是本 case 配套的 unexported helper，**仅 inline 在本 case 上方** 不抽到测试文件全局
// （避免与 11.3 ~ 11.6 既有 case 的 inline 调用模式 drift）
```

**关键设计约束**：本 case 验证"HTTP 跨接口 + 跨用户 + 跨 lifecycle 阶段的 envelope schema + 业务码端到端正确性"，与 service 层 AC2.1 互补：service 层验证事务原子性 + DB state，handler 层验证 HTTP envelope 解码 + 跨接口 wire 一致性。两层断言**必须**都覆盖（缺一不可，与 11.3 ~ 11.6 既有 case 的双层覆盖模式同模式）。

**AC3 — 回滚 1: 创建房间 - room_members repo Create 失败 → rooms 也回滚**

新增测试函数 `TestRoomServiceIntegration_CreateRoom_FaultInjectionRoomMembersStep_RoomsAlsoRollback`（位于 `room_service_integration_test.go`）：

- fault injection wrapper：`faultRoomMemberRepo` 包装真实 `mysql.RoomMemberRepo`，在 `Create(ctx, m)` 上替换为 `errors.New("synthetic room_members create failure")` 返回，其他方法（`ListRosterByRoomID` / `Count` 等 11.6 落地的方法）透传 delegate
- 装配：`service.NewRoomService(txMgr, userRepo, roomRepo, faultRoomMemberRepo, petRepo, sessionMgr, broadcastFn, broadcastExceptFn)` —— **关键** 8 参数（与 11.8 落地的 r3 fix 后签名一致）
- 调用：`svc.CreateRoom(ctx, CreateRoomInput{UserID: 1001})` → 期望返 1009 ErrServiceBusy（与 4.7 _PetRepoFailsTx_AllRowsRollback 同模式：service 层把任何非业务码 error 包成 1009）
- **核心断言**：DB 表为空（`rooms` count = 0 + `room_members` count = 0 + `users.current_room_id == NULL`）—— 验证 `roomRepo.Create` 已成功（事务内 INSERT rooms）但被 InnoDB undo log 撤销

```go
// faultRoomMemberRepo 包装真实 RoomMemberRepo，让 Create 抛 injectErr，其他方法透传。
type faultRoomMemberRepo struct {
    delegate  mysql.RoomMemberRepo
    injectErr error
}

func (f *faultRoomMemberRepo) Create(ctx context.Context, rm *mysql.RoomMember) error {
    return f.injectErr
}
func (f *faultRoomMemberRepo) ListRosterByRoomID(ctx context.Context, roomID uint64) ([]mysql.RoomMemberRosterRow, error) {
    return f.delegate.ListRosterByRoomID(ctx, roomID)
}
func (f *faultRoomMemberRepo) Count(ctx context.Context, roomID uint64) (int64, error) {
    return f.delegate.Count(ctx, roomID)
}
// ... 其他 RoomMemberRepo 方法透传（11.2 ~ 11.6 落地的全部方法集合，按 grep `interface RoomMemberRepo` 列举）
func (f *faultRoomMemberRepo) DeleteByRoomIDAndUserID(ctx context.Context, roomID, userID uint64) error {
    return f.delegate.DeleteByRoomIDAndUserID(ctx, roomID, userID)
}
```

**关键约束**：dev-story 阶段必须先 `grep -n "interface RoomMemberRepo" server/internal/repo/mysql/` 列举完整方法集合，确保 wrapper 实现 RoomMemberRepo 接口的**全部**方法（不全则编译失败 —— interface satisfaction 编译期校验是 Go 静态保证）。

**AC4 — 回滚 2: 加入房间 - users repo UpdateCurrentRoomID 失败 → room_members 也回滚**

新增测试函数 `TestRoomServiceIntegration_JoinRoom_FaultInjectionUsersUpdate_RoomMembersAlsoRollback`（位于 `room_service_integration_test.go`）：

- fault injection wrapper：`faultUserRepoForJoin` 包装真实 `mysql.UserRepo`，在 `UpdateCurrentRoomID(ctx, userID, roomID)` 上替换为 sentinel error，其他方法（`FindByID` / `FindByGuestUID` 等）透传
- fixture：A 创建房间（用真实 userRepo） → 切换 svc 实例（用 faultUserRepoForJoin） → B 调 svc.JoinRoom(B, roomID) → 期望返 1009
- **核心断言**：B 加入失败后 DB 状态 → `room_members WHERE user_id=B` 0 行（`roomMemberRepo.Create(B)` 已 INSERT 但被 InnoDB rollback 撤销）+ `users.current_room_id WHERE id=B == nil` + `room_members WHERE room_id=R AND user_id=A` 仍 1 行（A 的 fixture 不受影响）

**关键设计约束**：fixture 阶段（A 创建房间）必须用**真实 userRepo**，因为 `users.UpdateCurrentRoomID` 在 A.create 时也被调用（A 是 creator → users.current_room_id 写）；只在 B.join 路径阶段才换 fault wrapper —— 通过两次 `service.NewRoomService` 调用实现：第一次用真实 repo 跑 fixture，第二次用 fault wrapper 跑被测路径。

**AC5 — 回滚 3: 退出房间 - users repo UpdateCurrentRoomID 失败 → room_members 删除也回滚（user 仍在房间）**

新增测试函数 `TestRoomServiceIntegration_LeaveRoom_FaultInjectionUsersUpdate_RoomMembersDeleteAlsoRollback`（位于 `room_service_integration_test.go`）：

- fault injection wrapper：`faultUserRepoForLeave` 包装真实 `mysql.UserRepo`，**仅在第二次** `UpdateCurrentRoomID(ctx, userID, nil)` 调用上替换为 sentinel error（leave 事务路径），其他方法透传
- 复杂点：A.create 也调 UpdateCurrentRoomID（写 roomID，**非** nil），B.join 也调（写 roomID，**非** nil），A.leave 才调 UpdateCurrentRoomID(nil) —— wrapper 通过判断"传入 newRoomID 是 nil 时"决定是否注入 fault；或者用 atomic counter 跟踪调用次数，第 N 次开始注入
- 推荐方案：wrapper 只在 newRoomID 是 nil（即 leave 路径）时注入 fault，其他路径透传
- fixture：A 创建房间 + B join → 切换 svc 实例（fault wrapper） → A leave → 期望返 1009
- **核心断言**：A leave 失败后 DB 状态 → `room_members WHERE room_id=R AND user_id=A` 仍 1 行（`roomMemberRepo.DeleteByRoomIDAndUserID(A)` 已 DELETE 但被 InnoDB rollback 恢复）+ `users.current_room_id WHERE id=A == roomID`（仍然指向房间）+ `rooms.status` 仍 = 1 active

```go
// faultUserRepoForLeave 包装真实 UserRepo，让 UpdateCurrentRoomID(ctx, userID, nil) 抛 injectErr。
type faultUserRepoForLeave struct {
    delegate  mysql.UserRepo
    injectErr error
}

func (f *faultUserRepoForLeave) UpdateCurrentRoomID(ctx context.Context, userID uint64, roomID *uint64) error {
    if roomID == nil {  // leave 路径才注入 fault
        return f.injectErr
    }
    return f.delegate.UpdateCurrentRoomID(ctx, userID, roomID)
}
// ... 其他 UserRepo 方法透传
```

**AC6 — 并发 1: 5 个用户同时 join 同一房间（已有 4 人）→ 只 1 个成功**

新增测试函数 `TestRoomServiceIntegration_JoinRoom_Concurrent5UsersIntoFullRoom_OnlyOneSucceeds`（位于 `room_service_integration_test.go`）：

- fixture：A 创建房间 → B / C / D 依次 join（房间已满 4 人）—— 注：epics.md 行 2013 钦定"4 个用户已在房间，5 个用户同时 join → 只有 1 个成功"，**rationale**：A + B + C + D = 4 人；5 个新 user E/F/G/H/I 同时 join 时，FOR UPDATE 锁串行化让其中一个先看到 count=4 → 6002，另一个先看到 count=4 → 6002……但**有矛盾**：如果房间已满 4 人，**0 个**新 user 能成功（不是 1 个）

  **解读 epics.md 行 2013 的歧义**：epics 文字应该是"3 个用户已在房间，5 个用户同时 join → 只有 1 个成功（第 4 人位），其他 4 个返回 6002"或者"4 个用户已在房间，5 个用户同时 join → 全部 5 个返 6002"；本 case 按**前者**实装（更有信息量 —— 验证 race 收敛到 max=4）：
  - fixture：A + B + C 在房间（3 人 + 1 个空位）
  - 5 goroutine 并发 join：D / E / F / G / H 同时调 svc.JoinRoom
  - 期望：恰好 1 个 goroutine 成功（其 user 进入房间），其他 4 个返 6002
  - DB 收尾：room_members.count = 4（不能 5 / 不能 3）+ rooms.status = 1 active

```go
func TestRoomServiceIntegration_JoinRoom_Concurrent5UsersIntoFullRoom_OnlyOneSucceeds(t *testing.T) {
    svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
    defer cleanup()

    // fixture: A + B + C 已在房间（3 人，剩 1 个空位）
    const userA, userB, userC = uint64(1001), uint64(1002), uint64(1003)
    insertUser(t, sqlDB, userA, "uid-c1-a", "A", "")
    insertUser(t, sqlDB, userB, "uid-c1-b", "B", "")
    insertUser(t, sqlDB, userC, "uid-c1-c", "C", "")
    createOut, err := svc.CreateRoom(ctx, service.CreateRoomInput{UserID: userA})
    if err != nil { t.Fatal(err) }
    roomID := createOut.RoomID
    for _, u := range []uint64{userB, userC} {
        if _, err := svc.JoinRoom(ctx, service.JoinRoomInput{UserID: u, RoomID: roomID}); err != nil {
            t.Fatalf("fixture JoinRoom %d: %v", u, err)
        }
    }

    // 5 个新 user：D/E/F/G/H
    competitors := []uint64{1004, 1005, 1006, 1007, 1008}
    for i, u := range competitors {
        insertUser(t, sqlDB, u, fmt.Sprintf("uid-c1-%c", 'D'+i), string(rune('D'+i)), "")
    }

    var successCount, fullCount atomic.Int32
    var wg sync.WaitGroup
    barrier := make(chan struct{})
    for _, u := range competitors {
        u := u
        wg.Add(1)
        go func() {
            defer wg.Done()
            <-barrier  // 让 5 goroutine 同时起跑
            _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: u, RoomID: roomID})
            if err == nil {
                successCount.Add(1)
            } else if ae, ok := apperror.As(err); ok && ae.Code == apperror.ErrRoomFull {
                fullCount.Add(1)
            } else {
                t.Errorf("user %d unexpected err: %v", u, err)
            }
        }()
    }
    close(barrier)
    wg.Wait()

    // 核心断言：恰好 1 成功 + 4 返 6002
    if got := successCount.Load(); got != 1 {
        t.Errorf("successCount = %d, want 1", got)
    }
    if got := fullCount.Load(); got != 4 {
        t.Errorf("fullCount (6002) = %d, want 4", got)
    }
    // DB 收尾断言：room_members.count = 4
    assertCount(t, sqlDB, "room_members WHERE room_id=?", []any{roomID}, 4, "after concurrent join")
}
```

**关键设计约束**：barrier channel 让 5 goroutine 真正同时起跑（不是顺序起，否则第一个先抢锁写 count=4，后续看到 count=4 → 6002，但**也可能**因为 GOMAXPROCS=1 下 OS 调度顺序非随机化导致每次结果固定）；rationale：FOR UPDATE 锁串行化保证只有 1 个能成功，但**哪一个**取决于 OS 调度，本 case 不断言具体 user 而是断言**收敛**（恰好 1 / 恰好 4）。

**AC7 — 并发 2: 100 个不同用户同时 create + join 100 个不同房间**

新增测试函数 `TestRoomServiceIntegration_CreateRoom_Concurrent100DifferentUsers_100RoomsCreated`（位于 `room_service_integration_test.go`）：

- fixture：100 个 user（id=2001 ~ 2100），每个 user 独立 guestUid / nickname
- 100 goroutine 并发 svc.CreateRoom，每个 user 创建自己的房间（不同 user → 不同 room → 无冲突）
- DB 收尾断言：`rooms.count = 100` + `room_members.count = 100` + 100 user 全部 `current_room_id != nil`（指向各自房间）+ 没有 user 串到别人房间（每个 user 的 current_room_id 必须等于该 user 创建的 rooms.id）

**关键设计约束**：与 4.7 _Concurrent100DifferentGuestUIDs_NoCrossData 同模式（不同 key 空间无冲突 → 验证事务彼此独立 + 没有 race condition 把 user 串到别人房间）；本 case 验证 11.3 创建事务在高并发场景下**没有跨用户数据串扰**。

**AC8 — WS 联动: A 真拨号 WS + B 真 leave → A 真收 member.left wire**

新增测试函数 `TestRoomServiceIntegration_LeaveRoom_RealWSSession_BroadcastsMemberLeftToOtherMembers`（位于 `room_service_integration_test.go`）：

- 与 11.8 既有 `TestRoomServiceIntegration_LeaveRoom_Happy_BroadcastsMemberLeft`（用 capture broadcastFn）的差异：本 case 跳过 capture 层，直接用**真实 SessionManager + 真实 BroadcastToRoom + 真实 ws.Gateway + 真实 gorilla.Dial WS 拨号**，验证 wire 物理到达 client
- 装配复杂度：服务端需要同时挂 HTTP router（service.RoomService, room_handler）+ WS gateway（ws.Gateway）—— 用 httptest.NewServer 包一个 gin.Engine 同时挂两组路由
- fixture：起 mysql + redis dockertest 容器 → migrate 0001 ~ 0008 → A / B 通过 HTTP /api/v1/rooms create + join 加入房间（用真实 service + 真实 repo） → A 用 gorilla.Dial 拨号 `ws://host/ws/rooms/{id}?token=tokenA` 真握手 → 收到 first message = room.snapshot（验证 11.7 落地路径） → B 调 svc.LeaveRoom(B, roomID) → A.conn.SetReadDeadline(5s) → A.conn.ReadMessage 期望收到 wire = `{"type":"member.left","payload":{"userId":"B"},...}`
- 核心断言：unmarshal A 收到的 message → `env.type == "member.left"` + `payload.userId == strconv.FormatUint(B, 10)`
- **关键设计**：本 case 因为跨 service 包 + ws 包，需要新增 1 个 cross-package WS test helper（在 `room_service_integration_test.go` 内 inline 实装），与 ws 包内 `startGatewayWithRealMySQL` 等价但 import 路径反向（service 包 import ws 包，符合既有 service → ws 单向依赖）

```go
// startFullStackForWSIntegration 起 mysql + 装配真实 service + ws gateway + 真 SessionManager
// + 真 BroadcastToRoom 网关 → 返 (httpURL, wsURL, sessionMgr, signer, sqlDB, cleanup)。
//
// 关键差异 vs buildRoomServiceIntegrationWithCapture：本 helper 装的是 cross-layer
// 完整栈（HTTP + WS），让本 case 能真 Dial WS 而不是用 capture broadcastFn 替代。
//
// 不抽到 cross-package testutil，inline 在本测试文件内（与 ws_integration_test.go
// 的 startGatewayWithRealMySQL 复制粘贴模式同源 —— 跨 package helper 不复用是 4.7 /
// 11.3 ~ 11.8 已建立的规范，详见 ADR-0001 §3.5）。
func startFullStackForWSIntegration(t *testing.T) (httpURL, wsURL string, ...)
```

**关键设计约束**：本 case 是本 story **最复杂** case（跨 service / handler / ws 三层 + 真实 WS 拨号 + 真实 BroadcastToRoom 网关 + 真实 SessionManager）；如 dev-story 阶段发现集成栈装配过重，**允许** 简化为"用 11.8 既有 capture broadcastFn 模式 + 加 1 行真实 WS Session Register（让 capture 模式下也能验证 ListSessionsByRoomID 不为空）"，但**不允许** 砍掉真实 SessionManager 验证（必须验证 SessionManager 内存索引在 leave 路径下正确剔除 leaver Session）。

**AC9 — 边界 1 + 边界 2 + 边界 3 复用 / 文档化对应关系**

epics.md 行 2015 / 2016 / 2017 钦定的 3 个边界场景，其中 **边界 1（A 在房间 X 又调 POST /rooms 创建新房间 → 6003）** 和 **边界 3（closed 房间被 join → 6005）** 已被 11.3 / 11.4 既有 case 覆盖；本 story **不**新增重复测试函数，仅在测试文件顶部 doc comment 文档化对应关系：

```go
// Story 11.9 钦定的 10 类场景与本测试文件落地的对应关系：
//
//   1. 完整生命周期 → AC2.1 TestRoomServiceIntegration_FullLifecycle_5UsersJoinFullLeaveAllClosed
//   2. 回滚 1 (创建 room_members 失败) → AC3 TestRoomServiceIntegration_CreateRoom_FaultInjectionRoomMembersStep_RoomsAlsoRollback
//   3. 回滚 2 (加入 users update 失败) → AC4 TestRoomServiceIntegration_JoinRoom_FaultInjectionUsersUpdate_RoomMembersAlsoRollback
//   4. 回滚 3 (退出 users update 失败) → AC5 TestRoomServiceIntegration_LeaveRoom_FaultInjectionUsersUpdate_RoomMembersDeleteAlsoRollback
//   5. 并发 1 (5 user 同时 join) → AC6 TestRoomServiceIntegration_JoinRoom_Concurrent5UsersIntoFullRoom_OnlyOneSucceeds
//   6. 并发 2 (100 user create) → AC7 TestRoomServiceIntegration_CreateRoom_Concurrent100DifferentUsers_100RoomsCreated
//   7. 边界 1 (A 在 X 创建新房间 → 6003) → 已被 11.3 TestRoomServiceIntegration_CreateRoom_AlreadyInRoom_PrecheckReturns6003 覆盖（不新增）
//   8. 边界 2 (A 在 X 调 X/join → 6003) → AC10 TestRoomServiceIntegration_JoinRoom_UserAlreadyInSameRoom_Returns6003
//   9. 边界 3 (closed 房间 join → 6005) → 已被 11.4 TestRoomServiceIntegration_JoinRoom_RoomClosed_Returns6005 覆盖（不新增）
//   10. WS 联动 (B leave → A 收 member.left) → AC8 TestRoomServiceIntegration_LeaveRoom_RealWSSession_BroadcastsMemberLeftToOtherMembers
```

doc comment 必须 inline 在文件顶部（紧接 build tag + 既有 11.3 doc comment 之后），让 reviewer 阅读时一眼看到 epics.md 钦定 10 类场景的全覆盖映射。

**AC10 — 边界 2: 用户 A 在房间 X，A 调 POST /rooms/X/join（自己已在房间）→ 6003**

新增测试函数 `TestRoomServiceIntegration_JoinRoom_UserAlreadyInSameRoom_Returns6003`（位于 `room_service_integration_test.go`）：

- fixture：A 创建房间 X（A 自动是房间 X 成员；A.current_room_id = X.id）
- 测试调用：`svc.JoinRoom(ctx, JoinRoomInput{UserID: A, RoomID: X.id})` —— A 试图加入自己已在的房间 X
- 期望返回：6003 ErrUserAlreadyInRoom（service 层步骤 1 预检 `users.current_room_id != nil` 即返 6003，不区分目标 roomID 是否同 X）
- DB 断言：room_members count 仍 = 1（A creator）+ rooms.status 仍 = 1 + A.current_room_id 仍 = X.id（事务未开，无任何变化）

**关键设计约束**：本 case 与 11.4 既有 `_RoomNotFound_Returns6001` / `_RoomClosed_Returns6005` 等 case **正交补齐** —— 11.4 既有 case 覆盖 A 试图加入**别的**房间的 6003 / 6001 / 6005 / 6002 路径；本 case 验证 A 试图加入**自己已在的房间**也走预检 6003 路径（service 层不区分目标 roomID，与 V1 §10.4 钦定的"步骤 1 预检 current_room_id != nil 即返 6003"语义一致）。

**AC11 — 测试运行验证**

dev-story 阶段必须跑通：

```bash
cd server
bash scripts/build.sh --integration  # 期望全绿（含 11.3 ~ 11.8 既有 23 + 11 case + 本 story 新增 ≥10 case）
```

**关键约束**：
- 默认 `bash scripts/build.sh --test` **不**触发本 story case（build tag integration 隔离）
- 本 story case 必须在 `--integration` 标志下全绿（与 11.3 ~ 11.8 / 4.7 同模式）
- 如 docker 不可用，`startMySQL` helper 会 t.Skipf —— CI 不阻塞（与 11.3 ~ 11.8 同模式）

## 关键技术决策与 r/lesson 引用

- **fault injection wrapper 模式** —— 与 4.7 `faultPetRepo` / `faultChestRepo` / `faultUserRepo` 完全同模式（按方法包装真 repo + 在指定方法注入 sentinel error；其他方法透传 delegate）；**禁用** sqlmock / gomonkey / monkey 等第三方 fault injection 框架（与 4.7 lesson 同模式：MVP 用编译期可检查的 wrapper struct，避免 ARM 平台 / 跨平台 / 维护负担）
- **跨包 helper 不复用** —— 与 4.7 / 11.3 ~ 11.8 / ADR-0001 §3.5 同模式：本 story 在 `room_service_integration_test.go` 内 inline 任何需要的 helper（如 startFullStackForWSIntegration），**不**抽到 internal/testutil/ 跨包共享。理由：跨包 testing util 抽离是 future scaling 决策，本 story 不做（与已建立的"测试文件自包含"规范一致）
- **回滚 case 必须用真 InnoDB 事务** —— 与 4.7 _PetRepoFailsTx_AllRowsRollback / _ChestRepoFailsTx_AllRowsRollback 同模式：fault wrapper 包装的是**真实** mysql repo，让 service.firstTimeLogin / JoinRoom / LeaveRoom 内部走真实 GORM driver + InnoDB 事务，fault wrapper 仅在指定方法上替换为 sentinel error 让 fn 返 error → tx.WithTx 触发 InnoDB ROLLBACK → DB undo log 真实撤销。**不允许** 用 stub repo（stub 不真开事务，无法验证 rollback 行为）
- **并发 case 必须用 barrier channel** —— 让 N goroutine 真同时起跑（避免 OS 调度顺序固定导致每次结果一致从而失去 race detection 价值）；与 4.7 _Concurrent100SameGuestUID_OnlyOneUser / _Concurrent100DifferentGuestUIDs_NoCrossData 同模式
- **完整生命周期 case 三连断言** —— 每跨 1 个事务边界都做 memberCount + status + users.current_room_id 三连断言；与 4.6 happy path case 的内联断言风格 + 4.7 _ReentryAfterSuccess 多次断言风格一致
- **WS 联动 case 优先简化路径** —— 如 dev-story 阶段发现完整 cross-layer 栈（mysql + redis + ws Gateway + http router）装配过于复杂，**允许** 退化为 capture broadcastFn + 真实 SessionManager 注入的 11.8 既有模式 + 1 行 `sessionMgr.Register(...)` 让 ListSessionsByRoomID 非空；**不允许** 跳过 SessionManager 真实校验
- **lessons 引用**：
  - `2026-05-09-async-causal-ordering-needs-per-room-mutex-11-8-r4.md` / `2026-05-09-async-must-preserve-sync-observable-invariants-11-8-r3.md` / `2026-05-09-commit-time-per-key-serialization-required-for-causal-order-11-8-r6.md` —— 11.8 已落地的 post-commit hook 异步化语义；本 story 集成测试**必须**调 `wg.Wait()`（与 11.8 case 14/15/16 同模式）才能安全断言 broadcast / close 副作用
  - `2026-05-08-snapshot-isolation-needs-row-locks-and-cross-tx-rooms-status-drift-11-1-r9.md` / `2026-05-08-default-deny-acl-and-prose-mermaid-zip-alignment-11-1-r7.md` —— 11.6 落地的 GET /rooms/{id} ACL FOR SHARE 锁语义；本 story HTTP E2E case 在 lifecycle 各阶段 GET /rooms/{id} 必须断言 ACL 6004 在 caller 离开后立即触发
  - `2026-05-09-room-service-fire-and-forget-nil-sessionmgr-guard.md` —— 11.8 落地的 nil sessionMgr 防御；本 story 不应在装配时给 sessionMgr 传 nil（用 `wsapp.NewSessionManager()` 真实实例 / 或 capture 路径）
  - `2026-04-26-multi-table-tx-must-cover-all-unique-constraint-races.md` —— 4.6 落地的多表事务并发 race 模式；本 story 并发 1 / 2 case 与此 lesson 同源思路

## Project Structure Notes

- **落地文件全部在 server 工程** （iOS / iphone 工程不动）
  - `server/internal/service/room_service_integration_test.go` —— 追加 ≥9 个新 case（AC2.1 + AC3 ~ AC8 + AC10）+ 3 个 fault injection wrapper struct（faultRoomMemberRepo / faultUserRepoForJoin / faultUserRepoForLeave）+ 文件顶部 10 类场景对应关系 doc comment（AC9）
  - `server/internal/app/http/handler/room_handler_integration_test.go` —— 追加 1 个完整生命周期 HTTP E2E case（AC2.2）+ 配套 inline helper（callCreateExpectingOK / callJoinExpectingOK 等）
- **不修改任何 production 代码** （service / handler / repo / ws / migrations 全部不动）
- **不新建文件** （包括 fault injection helper / WS cross-layer helper 全部 inline 到现有 _integration_test.go 文件）
- **不动 docs / ADR / sprint-status.yaml 之外的任何配置**

## References

- **唯一权威 AC 来源**：`_bmad-output/planning-artifacts/epics.md` §Story 11.9 行 1998-2020
- **既有 service 集成测试**：`server/internal/service/room_service_integration_test.go`（11.3 ~ 11.8 落地共 23 函数 + helper 集合）
- **既有 handler 集成测试**：`server/internal/app/http/handler/room_handler_integration_test.go`（11.3 ~ 11.6 落地共 11 函数 + helper 集合）
- **既有 ws 集成测试**：`server/internal/app/ws/ws_integration_test.go` / `snapshot_integration_test.go`（10.3 / 11.7 落地，本 story WS 联动 case 复用其 startGatewayWithRealMySQL 模式）
- **fault injection 范式参考**：`server/internal/service/auth_service_integration_test.go`（4.7 落地的 faultPetRepo / faultChestRepo / faultUserRepo wrapper 模式）
- **V1 接口设计**：`docs/宠物互动App_V1接口设计.md` §10.1 / §10.2 / §10.3 / §10.4 / §10.5 + §12.3 `### 成员加入` / `### 成员离开`（§10.x 各接口契约 / §12.3 WS payload 字段表）
- **数据库设计**：`docs/宠物互动App_数据库设计.md` §8.6（加入房间事务）/ §8.7（退出房间事务）
- **时序图**：`docs/宠物互动App_时序图与核心业务流程设计.md` §11.2（join 时序）/ §12.2（leave 时序）
- **ADR**：`_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.5（测试 helper 自包含范式）+ `0007-context-propagation.md`（ctx 传播）+ `0011-ws-stack.md`（WS 协议契约）
- **lessons**：见上"关键技术决策与 r/lesson 引用"段（11-8-r3 / r4 / r6 + 11-1-r9 / r7 + 4.6 multi-table-tx）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

### Completion Notes List

- Ultimate context engine analysis completed - comprehensive developer guide created
- Story is purely test-extension scope; 不实装新业务功能（与 4.7 / 20.9 / 26.5 / 32.5 同模式 Layer 2 收尾 story）
- 跨 service 集成测试 + handler 集成测试两份文件追加 ≥10 个新 case；fault injection wrapper 模式与 4.7 同源
- WS 联动 case 是最复杂 case（跨 service / ws / handler 三层 + 真 gorilla.Dial 拨号），dev-story 阶段如装配过重允许退化到 capture broadcastFn + 真 SessionManager 模式

### dev-story 实装记录（2026-05-09 完成）

**追加 service 层测试函数**（room_service_integration_test.go）：

| AC | 测试函数 | 类别 | 通过用时 |
|---|---|---|---|
| AC2.1 | `TestRoomServiceIntegration_FullLifecycle_5UsersJoinFullLeaveAllClosed` | 完整生命周期 | 25.7s |
| AC3 | `TestRoomServiceIntegration_CreateRoom_FaultInjectionRoomMembersStep_RoomsAlsoRollback` | 回滚 1 | 19.8s |
| AC4 | `TestRoomServiceIntegration_JoinRoom_FaultInjectionUsersUpdate_RoomMembersAlsoRollback` | 回滚 2 | 20.4s |
| AC5 | `TestRoomServiceIntegration_LeaveRoom_FaultInjectionUsersUpdate_RoomMembersDeleteAlsoRollback` | 回滚 3 | 20.9s |
| AC6 | `TestRoomServiceIntegration_JoinRoom_Concurrent5UsersIntoFullRoom_OnlyOneSucceeds` | 并发 1（5 join 抢 1 空位） | 25.3s |
| AC7 | `TestRoomServiceIntegration_CreateRoom_Concurrent100DifferentUsers_100RoomsCreated` | 并发 2（100 user 100 房间） | 34.9s |
| AC10 | `TestRoomServiceIntegration_JoinRoom_UserAlreadyInSameRoom_Returns6003` | 边界 2 | 26.0s |
| AC8 | `TestRoomServiceIntegration_LeaveRoom_RealWSSession_BroadcastsMemberLeftToOtherMembers` | WS 联动（真 gorilla.Dial） | 22.1s |

**追加 handler 层测试函数**（room_handler_integration_test.go）：

| AC | 测试函数 | 类别 | 通过用时 |
|---|---|---|---|
| AC2.2 | `TestRoomHandlerIntegration_FullLifecycle_E2E_HTTPCreateJoinLeaveDetail` | 完整生命周期 HTTP E2E | 25.7s |

**fault injection wrapper struct**（inline 在 service 测试文件）：
- `faultRoomMemberRepo`（覆盖 RoomMemberRepo 全部 7 方法）
- `faultUserRepoForJoin`（UpdateCurrentRoomID 全路径注入）
- `faultUserRepoForLeave`（仅 leave 路径 nil 入参注入）

**helper 函数**：
- `buildRoomServiceWithCustomRepos`（支持自定义 user/room/roomMember repo override；用于 fault injection）
- `startWSGatewayForLeaveCase`（用 gormmysql.New(Conn:sqlDB) 包既有 sqlDB 起 WS gateway httptest server）
- 8 个 inline lifecycle helper（lifecycleCallCreate/Join/Leave/GetCurrentRoom/GetRoomDetail Expecting OK/Code）

**AC9 边界 1/3 复用**（不新增测试函数）：
- 边界 1（A 在房间又调 POST /rooms 创建新房间 → 6003）→ 复用 11.3 `_AlreadyInRoom_PrecheckReturns6003`
- 边界 3（closed 房间被 join → 6005）→ 复用 11.4 `_RoomClosed_Returns6005`
- 文件顶部 doc comment（service 包）已记录 10 类场景对应关系

**测试运行结果**：
- `bash scripts/build.sh --test`：unit 测试全绿（含 service / handler / ws / repo / etc）
- service 集成（21 既有 + 8 新增 = 29 个 case）：全 PASS（合计 ~667s on docker mysql:8.0）
- handler 集成（16 既有 + 1 新增 = 17 个 case）：全 PASS（合计 ~385s）

**关键决策**：
- AC8 WS 联动选择「真 gorilla.Dial」路径（非退化 capture broadcastFn 模式）—— 起 WS Gateway httptest server 让 A 真 Dial 进 SessionManager；用 `gormmysql.New(Conn: sqlDB)` 包既有 sqlDB 进 GORM，避免 dsn 暴露问题
- AC4 fixture 用 raw SQL 直接造（不能用 svc.CreateRoom，fault wrapper 让 create 路径 UpdateCurrentRoomID 也抛 error）—— 与 AC5 (faultUserRepoForLeave 仅 nil 路径注入) 形成对比
- 全部 fault injection case 用 `buildRoomServiceWithCustomRepos` helper 起独立容器，避免与既有 `buildRoomServiceIntegration` 共用 svc 实例引发 state pollution

### File List

修改（仅追加；不动既有 case）：
- `server/internal/service/room_service_integration_test.go`（追加 8 case + 3 fault wrapper struct + 1 helper + 1 startWSGatewayForLeaveCase helper + 11.9 doc comment 区块）
- `server/internal/app/http/handler/room_handler_integration_test.go`（追加 1 E2E case + 8 inline lifecycle helper）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（11-9 状态 ready-for-dev → review；last_updated 同步）

不改：service / handler / repo / migrations / ADR / docs / configs / bootstrap / ws / 既有 23 service 集成测试函数 / 既有 11 handler 集成测试函数。
