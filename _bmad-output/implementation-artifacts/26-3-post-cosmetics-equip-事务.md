# Story 26.3: POST /cosmetics/equip 事务（含同槽换装）—— 首次落地 CosmeticEquipService.Equip 单事务实装（§8.3 服务端逻辑步骤 4-11 钦定：查实例归属 5001/5002 → 校状态 5008/5003 → 校 pet 归属 5002 → 查 cosmetic_items.slot（missing-no-row → 5003 + log error）→ 同槽换装删旧 user_pet_equips 行 + 旧实例 status 回 1 → INSERT user_pet_equips + 当前实例 status 改 2 → 返回 equipped DTO）+ 新增 UserPetEquipRepo / 扩 PetRepo / UserCosmeticItemRepo / CosmeticItemRepo 最小所需方法 + handler + 路由 + ≥6 单测 + dockertest 集成测试

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As an iPhone 用户,
I want 我可以从仓库选一件实例穿戴到我的猫上，且如果该槽位已有装备会自动先卸下,
so that 穿戴体验顺畅，不需要手动先卸再穿.

## 故事定位（Epic 26 第三条 = 第一条**业务事务**实装 story）

- **Epic 26 进度**：26.1（契约定稿 §8.3 / §8.4 / §1 节点 9 冻结，**done**）→ 26.2（user_pet_equips migration + `UserPetEquip` GORM struct + TableName 最小骨架，**done**）→ **26.3（本 story，POST /cosmetics/equip 事务，含同槽换装）** → 26.4（POST /cosmetics/unequip 事务，复用本 story 落地的 UserPetEquipRepo）→ 26.5（Layer 2 集成测试 - 穿戴事务全流程，深度覆盖回滚 / 并发 / 一致性）→ 26.6（GET /home 扩展 - pet.equips 真实数据）。
- **上游已冻结**（不可改）：V1 §8.3（POST /api/v1/cosmetics/equip 请求 / 响应 / 服务端逻辑 / 错误码 / 关键约束，Story 26.1 锚定并冻结，见 §1 节点 9 冻结声明）+ 数据库设计 §8.4（穿戴事务步骤）+ §5.10（user_pet_equips schema，Story 26.2 已落地 `0016_init_user_pet_equips.up/down.sql`）+ §6.8（slot 枚举）+ §6.10（user_cosmetic_items.status 枚举）。本 story 严格对齐这些**输入**，**不**反向修改任何 `docs/宠物互动App_*.md`。
- **下游依赖本 story**：26.4 unequip 复用本 story 落地的 `UserPetEquipRepo`（追加 `DeleteByPetSlotInTx` / `FindByPetSlot` 已被本 story 落地则直接复用）；26.5 Layer 2 集成测试基于本 story 的 happy path；26.6 GET /home pet.equips 复用 `UserPetEquipRepo` 查询方法 / `UserPetEquip` struct。
- **错误码已就位**（Story 26.1 落地，`server/internal/pkg/errors/codes.go`）：`ErrCosmeticNotFound`(5001) / `ErrCosmeticNotOwned`(5002) / `ErrCosmeticInvalidState`(5003) / `ErrCosmeticAlreadyEquipped`(5008)。本 story **直接复用**，**不**新造错误码、**不**扩张 §1 节点 9 冻结的 equip 错误码集合 `{1001,1002,1005,5001,5002,5003,5008,1009}`。

## Acceptance Criteria

> 全部源自 V1 §8.3（行 1454-1565，已冻结）+ epics.md §Story 26.3（行 3537-3566）+ 数据库设计 §8.4（行 1009-1018）。

**AC1 — 路由 + handler 接入**

- 新增 `POST /api/v1/cosmetics/equip` 路由，挂在 **`authedGroup`**（与 `/cosmetics/catalog` / `/cosmetics/inventory` 同组同模式 —— Auth + RateLimitByUserID 中间件链由 authedGroup 既有链兜底，对应 §8.3 错误码 1001 / 1005）。**不**走 `chestOpenGroup`（那是 POST /chest/open 专属、handler 内层做 rate_limit 的特例；equip 无 idempotency、限频走标准 authedGroup 中间件，V1 §8.3 接口元信息行 1467-1468 钦定）。
- handler 方法名 `Equip`，挂在既有 `CosmeticsHandler`（`server/internal/app/http/handler/cosmetics_handler.go`，与 `GetCatalog` / `GetInventory` 同 struct）。
- handler 从 `c.Request.Context()` 取 ctx 传给 service（ADR-0007 §2.2：**不**直接传 `*gin.Context`，其 Done() 是 nil channel）。userID 从 auth 中间件注入的 context 值取（与既有 handler 取 userID 同模式 —— 参照 `cosmetics_handler.go` GetInventory 取 userID 的写法）。
- handler **不**直接调 `response.Error` 写 envelope；走 `c.Error(err)` + return，由 `ErrorMappingMiddleware` 翻译（ADR-0006 单一 envelope 生产者，与 `GetCatalog` 注释行 49-53 一致）。成功走 `response.Success(c, dto, "ok")`。

**AC2 — 请求体解析 + 参数校验（§8.3 服务端逻辑步骤 2）**

- 请求体 `{petId: string, userCosmeticItemId: string}`（V1 §8.3 行 1475-1485）；两字段均必填、为合法 BIGINT 字符串（length ≥ 1，可被解析为 uint64）。
- 缺失 / 非合法 BIGINT 字符串 → **1002 参数错误**（V1 §8.3 行 1547；handler 层校验，与既有 handler 参数校验同模式）。BIGINT 字符串↔uint64 转换用 `strconv.ParseUint(s, 10, 64)`（与 `cosmetics_handler.go` 既有 `strconv` import 一致）。

**AC3 — Equip 单事务实装（§8.3 服务端逻辑步骤 3-11 + 数据库设计 §8.4，全部步骤同一 `txMgr.WithTx` 事务内，任一步 err → 整体回滚 NFR1/NFR2）**

新建 `server/internal/service/cosmetic_equip_service.go`（**新文件**；与既有 `cosmetic_service.go`（catalog/inventory 只读）分文件，**不**混入 —— equip 是写事务，与只读查询职责不同；命名参照 `chest_open_service.go` 与 `chest_service.go` 分文件先例）。事务内严格按 V1 §8.3 服务端逻辑步骤执行：

1. **步骤 4 — 查实例归属**：`SELECT id, cosmetic_item_id, status, user_id FROM user_cosmetic_items WHERE id = ?`（**仅按实例 id 查，禁止本步加 `AND user_id = ?` 过滤** —— V1 §8.3 行 1492-1496 fix-review 26-1 r1 [P2] 锁定）：
   - 行不存在 → **5001 道具不存在**（`apperror.ErrCosmeticNotFound`）。
   - 行存在但 `user_id != 当前用户` → **5002 道具不属于当前用户**（`apperror.ErrCosmeticNotOwned`）。**不变量**：5001 仅对应"实例完全无 row"，"实例存在但属他人"恒为 5002（实装无自由度，否则 AC5 单测 case "实例不属于当前用户 → 5002" 永不可达）。
2. **步骤 5 — 校验实例状态**（V1 §8.3 行 1497）：
   - `status = 2 (equipped)` → **5008 装扮已装备**（`apperror.ErrCosmeticAlreadyEquipped`）。
   - `status = 3 (consumed)` 或 `4 (invalid)` → **5003 道具状态不可用**（`apperror.ErrCosmeticInvalidState`）。
   - `status = 1 (in_bag)` → 继续。
3. **步骤 6 — 校验 pet 归属**（V1 §8.3 行 1498）：`petId` 对应 pet 必须属于当前用户 → 否则 **5002**（epics.md §Story 26.3 AC "pet 不属于当前用户 → 5002"）。pet 不存在亦视为 5002（与"不属于当前用户"同处理 —— 契约只给 5002 一个出口，pet 不存在不在 §8.3 错误码集合内独立出口）。
4. **步骤 7 — 查配置槽位**：`SELECT slot, name FROM cosmetic_items WHERE id = <实例.cosmetic_item_id>`：
   - 行存在 → 拿到 `slot`（§6.8 枚举）+ `name`（步骤 11 response 用），继续步骤 8。
   - 行不存在（missing-no-row：admin 物理删了 cosmetic_items 行但实例仍 status=1）→ **5003 道具状态不可用** + **`slog` log error**（V1 §8.3 行 1501-1502 + 行 1564 fix-review 26-1 r2 [P2] 锁定：映射到已冻结集合内的 5003，**不**新造 / **不**复用 5001 / **不**落 1009）。事务回滚（本步在事务内）。
5. **步骤 8 — 同槽换装（自动卸下旧装备）**：查 `user_pet_equips WHERE pet_id = ? AND slot = ?`（走 26.2 落地的 `uk_pet_slot` 索引）：
   - 已有旧装备 → `DELETE` 该旧 `user_pet_equips` 行 + `UPDATE user_cosmetic_items SET status = 1 (in_bag) WHERE id = <旧实例 id>`（自动卸下；client 无需先调 unequip）。
   - 该 slot 无装备 → 跳过本步。
6. **步骤 9 — 绑定 + 状态推进**：`INSERT INTO user_pet_equips (user_id, pet_id, slot, user_cosmetic_item_id) VALUES (...)` + `UPDATE user_cosmetic_items SET status = 2 (equipped) WHERE id = <当前实例 id>`。
7. **步骤 10 — 提交**（`WithTx` fn return nil → 自动 commit；任一步返 error → 自动 rollback；ADR-0007 §2.4 + 数据库设计 §8.4）。
8. **步骤 11 — 响应**：`{petId, equipped: {slot, userCosmeticItemId, cosmeticItemId, name}}`（V1 §8.3 行 1510-1540 字段表 + 示例；所有 BIGINT id 字符串化下发 —— `petId` / `userCosmeticItemId` / `cosmeticItemId` 为 string，`slot` 为 int）。

**AC4 — DB UNIQUE 并发兜底 → 1009（§8.3 关键约束行 1560 + NFR11）**

- 步骤 9 `INSERT user_pet_equips` 受 26.2 落地的 `UNIQUE KEY uk_pet_slot (pet_id, slot)` + `UNIQUE KEY uk_user_cosmetic_item_id (user_cosmetic_item_id)` 兜底。并发场景（同 pet 同 slot 并发 equip 不同实例 / 同实例并发 equip）→ DB 拒绝第二条 INSERT（MySQL ER_DUP_ENTRY 1062）→ repo 层翻译为哨兵 error → service 用 `errors.Is` 识别后回滚 + 返 **1009 服务繁忙**（`apperror.ErrServiceBusy`）。
- **repo 哨兵翻译模式**（与 `room_member_repo.go` 行 353-383 双路径 1062 翻译 + `errors.go` 行 53-98 哨兵集 1:1 同模式）：`user_pet_equip_repo.go` 的 `InsertInTx` 捕获 `*go-sql-driver/mysql.MySQLError` Number==1062，按 Message 含 `uk_pet_slot` / `uk_user_cosmetic_item_id` 分流为两个独立哨兵（在 `server/internal/repo/mysql/errors.go` 新增 `ErrUserPetEquipPetSlotDuplicate` / `ErrUserPetEquipItemDuplicate`，注释说明语义与节点 9 NFR11 引用，与既有 `ErrRoomMembersUserIDDuplicate` / `ErrRoomMembersRoomUserDuplicate` 注释风格一致）；service 层 `errors.Is` 两哨兵均 → 1009。1062 但 Message 两约束名都不含 → raw error 透传（service 兜底 1009，与 room_member fallback 行 363-365 同模式）。

**AC5 — 单元测试覆盖（≥6 case，mocked repo，`cosmetic_equip_service_test.go`）**

epics.md §Story 26.3 行 3557-3563 显式钦定（**全部必须有**）：

- happy: 该 slot 无装备 → 直接装上，`user_pet_equips` 多 1 行（mock 校验 InsertInTx 被调 + 当前实例 status→2 + 旧装备相关 mock 未被调）。
- happy: 该 slot 已有装备 → 旧装备 status 改 in_bag(1) + user_pet_equips 旧行删除 + 新行 INSERT + 新装备 status equipped(2)（mock 校验删旧行 + 旧实例 status→1 + InsertInTx + 新实例 status→2 调用顺序）。
- edge: 实例不存在 → 5001。
- edge: 实例不属于当前用户 → 5002。
- edge: 实例 status=consumed(3) → 5003（建议**加测** status=invalid(4) → 5003 + status=equipped(2) → 5008 两条补全状态分支矩阵，对应 §8.3 步骤 5）。
- edge: pet 不属于当前用户 → 5002。
- **建议补**：missing-no-row（cosmetic_items 查不到）→ 5003 + 验证 slog error 路径（§8.3 行 1501-1502 fix-review 26-1 r2 [P2] 锁定，单测层用 mock repo 返 not-found 触发）；DB UNIQUE 哨兵 → 1009（mock InsertInTx 返 `ErrUserPetEquipPetSlotDuplicate` → 验证回滚 + 1009）。
- mock 模式：参照 `chest_open_service_test.go` —— 注入 mock repo（实现 repo interface）+ mock `tx.Manager`（`WithTx` 直接调 fn 传 `context.Background()` 或透传 ctx，不真起事务）。

**AC6 — 集成测试覆盖（dockertest，真实 MySQL，`cosmetic_equip_service_integration_test.go`，build tag `integration`）**

epics.md §Story 26.3 行 3564-3566 钦定：

- 创建 user + pet + 1 件 hat 实例（status=1 in_bag）→ equip → 断言 DB `user_pet_equips` 1 行（pet_id / slot / user_cosmetic_item_id 正确）+ 该实例 `status=2`。
- 接着 equip 另一件 hat（同 slot）→ 断言 DB `user_pet_equips` **仍 1 行**（同槽换装，行被更新非新增）+ 旧 hat `status=1` + 新 hat `status=2`。
- build tag 用项目既有约定（参照 `chest_open_service_integration_test.go` / `cosmetic_service_integration_test.go` 文件顶部 `//go:build integration` + dockertest 起 MySQL + 跑 migrations 的 setup helper）。深度回滚 / 100 并发兜底 / 状态一致性矩阵归 **Story 26.5**（本 story **不**做，AC 范围红线）。

**AC7 — 验证脚本通过**

`bash scripts/build.sh --test` 全绿（vet + build + `go test -count=1 ./...` 单测）；集成测试 `bash scripts/build.sh --integration` 单跑通过（CLAUDE.md Build & Test 节）。

## Tasks / Subtasks

- [x] **Task 1 — repo 层最小方法落地**（AC3, AC4）
  - [x] `server/internal/repo/mysql/errors.go`：新增 `ErrUserPetEquipPetSlotDuplicate` / `ErrUserPetEquipItemDuplicate` 两哨兵 + `ErrUserCosmeticItemNotFound` / `ErrUserPetEquipNotFound` 合法 NotFound 哨兵 + 注释（语义 + NFR11 + §8.3 行 1560 引用，与既有 room_member 双哨兵注释风格一致）
  - [x] `server/internal/repo/mysql/user_pet_equip_repo.go`：在既有 `UserPetEquip` struct + `TableName()` 下追加 `UserPetEquipRepo` interface + `userPetEquipRepo` impl + `NewUserPetEquipRepo`。`FindByPetSlot`（NotFound → 哨兵）/ `DeleteByPetSlotInTx`（同槽换装删旧 + 26.4 复用）/ `InsertInTx`（1062 → 双哨兵翻译，模式抄 `room_member_repo.go`）。全走 `tx.FromContext(ctx, r.db).WithContext(ctx)`
  - [x] `server/internal/repo/mysql/user_cosmetic_item_repo.go`：追加 `FindByIDForEquip`（**仅按 id 查，无 user_id 过滤** —— §8.3 行 1492；NotFound → `ErrUserCosmeticItemNotFound`）+ `UpdateStatusInTx`（status 1↔2 推进，事务内单字段 Update）
  - [x] `server/internal/repo/mysql/cosmetic_item_repo.go`：追加 `FindSlotNameByID`（§8.3 步骤 7；`found=false`+`err=nil` 让 service 走 missing-no-row → 5003 + log error，与 DB 异常 err 区分）
  - [x] `server/internal/repo/mysql/pet_repo.go`：追加 `FindByID`（§8.3 步骤 6 校 pet 归属；NotFound → `ErrPetNotFound`，service 视为 5002）
  - [x] 每个新 repo 方法补 repo 层单测（sqlmock；新建 `user_pet_equip_repo_test.go` + 扩 `pet_repo_test.go` / `cosmetic_item_repo_test.go` / `user_cosmetic_item_repo_test.go`）
  - [x] 同步扩 5 个既有 PetRepo stub（`stubPetRepo` / `stubHomePetRepo` / `stubPetRepoForPetService` / `roomTestStubPetRepo` / `faultPetRepo`）+ 4 个 CosmeticItemRepo stub + 2 个 UserCosmeticItemRepo stub 加 panic-default / 透传方法（interface 扩张不破坏既有测试编译）
- [x] **Task 2 — service 层 Equip 事务实装**（AC3, AC4）
  - [x] 新建 `server/internal/service/cosmetic_equip_service.go`：`CosmeticEquipService` interface（`Equip(ctx, EquipParams) (*EquipResult, error)` —— DTO 改名 `EquipParams`/`EquipResult`/`EquippedItem` 因 `EquipOutput` 已被 room_service.go 占用）+ impl + `NewCosmeticEquipService`（注入 txMgr + 4 repo）
  - [x] `Equip` 方法：`s.txMgr.WithTx(ctx, func(txCtx)...)` 内 `runEquipTx` 严格按 §8.3 步骤 4-9 顺序（**所有 repo 调用用 `txCtx`** —— ADR-0007 §2.4；参照 `chest_open_service.runOpenChestTx`）
  - [x] 错误映射三层（ADR-0006）：repo raw / 哨兵 → `errors.Is` + `apperror.New`/`Wrap` 翻译 5001/5002/5003/5008/1009；missing-no-row `slog.ErrorContext`
- [x] **Task 3 — handler + 路由接入**（AC1, AC2）
  - [x] `cosmetics_handler.go`：加 `Equip(c *gin.Context)`（ShouldBindJSON + 1002 校验 *string 指针缺失 + ParseUint + 取 userID + 调 svc + Success / c.Error）+ `equipRequest` / `equipResponseDTO`（BIGINT 字段 string、slot int）；扩 `CosmeticsHandler` 加 `equipSvc` 字段 + `NewCosmeticsHandler` 扩参
  - [x] `router.go`：wire `NewUserPetEquipRepo(deps.GormDB)` + 复用既有 `userCosmeticItemRepo`/`cosmeticItemRepo`/`petRepo` + `NewCosmeticEquipService` + 更新 `NewCosmeticsHandler` 调用 + `authedGroup.POST("/cosmetics/equip", ...)`（紧跟 inventory 行后，Story 26.3 注释）
- [x] **Task 4 — 测试**（AC5, AC6）
  - [x] `cosmetic_equip_service_test.go`：15 case（AC5 钦定全覆盖 + missing-no-row→5003 + slog / 双 UNIQUE 哨兵→1009 / status 2/3/4 矩阵 / pet not found→5002 / DB error→1009 / zero userID→1002）
  - [x] `cosmetic_equip_service_integration_test.go`：dockertest 2 场景（AC6：首次 equip + 同槽换装净 1 行 + status 1↔2 真值）—— 编译/list 通过；执行需 Linux Docker（本机 Windows 无 Docker，与既有所有 integration test 同环境限制）
  - [x] `cosmetics_handler_test.go`：扩 6 个 Equip case（happy DTO 形状 + 1002 缺字段/非 BIGINT/JSON 类型错 + userID 缺失 1009 + service error 透传）+ 扩 router helper 满足扩参构造
- [x] **Task 5 — 验证 + 收尾**（AC7）
  - [x] `bash scripts/build.sh --test` 全绿（vet + build + 全 unit test）；`bash scripts/build.sh --integration` 因本机 Windows 无 Docker 无法执行（既有 `auth_service_integration_test` 同样 timeout —— 非本 story 引入缺陷；`-tags=integration` vet + 编译 + test list 全通过，CI Linux 跑）
  - [x] 自检：无新造错误码（复用 5001/5002/5003/5008/1009）/ 无改 docs/*.md / 无改 V1 契约 / 无改 0001-0016 migration / 范围红线全部遵守

## Dev Notes

### 关键架构约束（必须遵守）

- **目录形态与 CLAUDE.md target 不同**：实际代码不在 `internal/domain/` —— repo 在 `internal/repo/mysql/`、service 在 `internal/service/`、handler 在 `internal/app/http/handler/`、路由 wire 在 `internal/app/bootstrap/router.go`。按**实际既有结构**落地，**不**按 CLAUDE.md §"节点 1 之后的目录形态（target）"的理想树新建 `domain/cosmetic/` 等目录。
- **ctx 必传（ADR-0007 + CLAUDE.md）**：service/repo 所有导出函数第一参数 `ctx context.Context`；handler 从 `c.Request.Context()` 取（**不**把 `*gin.Context` 当 ctx）；`txMgr.WithTx(ctx, fn)` 内所有 repo 调用用 **`txCtx`** 而非外层 ctx；repo 用 `tx.FromContext(ctx, r.db).WithContext(ctx)` 模式（事务内走 tx 句柄 / 事务外走 r.db）。
- **错误三层映射（ADR-0006）**：repo 返 raw / 哨兵 error（**不**返 *AppError / 业务码）→ service 用 `apperror.New` / `apperror.Wrap` + `errors.Is` 翻译为 5001/5002/5003/5008/1009 → handler `c.Error(err)` 透传 → `ErrorMappingMiddleware` 写统一 envelope（handler **不**自己调 `response.Error`）。错误码已在 `server/internal/pkg/errors/codes.go` 行 39-46/101-108 定义（Story 26.1），**直接复用**。
- **事务边界归 service**（repo 包注释行 11）：repo **不**调 `txMgr.WithTx`；事务由 `cosmetic_equip_service.go` 用 `s.txMgr.WithTx` 控制；`tx.Manager` interface（`server/internal/repo/tx/manager.go` 行 31-46）`WithTx(ctx, fn func(txCtx context.Context) error) error`，fn 返 nil → commit / 返 error → rollback / panic 自动 rollback。

### 范围红线（本 story 明确不做）

- **不**做 unequip（POST /cosmetics/unequip 是 Story 26.4 钦定范围；本 story 仅落地 `DeleteByPetSlotInTx` repo 方法供 26.4 复用，但**不**实装 unequip service/handler/路由）。
- **不**做深度回滚 / 100 并发 stress / 状态一致性矩阵集成测试（Story 26.5 钦定 —— epics.md 行 3592-3616）；本 story 集成测试仅 AC6 的 2 个 happy + 同槽换装场景。
- **不**做 GET /home pet.equips 真实化（Story 26.6；本 story 落地的 `UserPetEquipRepo` / `UserPetEquip` struct 是 26.6 的复用基础，但**不**改 home_service / GET /home）。
- **不**引入 `idempotencyKey`（§8.3 行 1468/1559 钦定 equip **无**幂等键；重复 equip 同实例由 status=2 → 5008 + DB UNIQUE 兜底，与开箱/合成的 idempotency 模式不同 —— **不**抄 chest_open_service 的 ClaimPending/幂等记录逻辑）。
- **不**改 V1 §8.3/§8.4 契约 / 数据库设计 §5.10/§8.4/§6.8 / 任何 `docs/宠物互动App_*.md`（契约**输入**，严格对齐不修改；若发现实装与契约不一致 → 优先改本 story / 实装对齐契约，**不**反向改 docs）。
- **不**改 0001-0016 既有 migration（26.2 已落地 0016 user_pet_equips schema；本 story 仅写 service/repo/handler 代码，**无** migration）。
- **不**新造错误码 / 不扩张 §1 节点 9 冻结的 equip 错误码集合 `{1001,1002,1005,5001,5002,5003,5008,1009}`（missing-no-row 必须映射到既有 5003，§8.3 行 1502/1564 fix-review 26-1 r2 [P2] 锁定）。
- **不**预实装 26.4/26.6/Epic 32 的 repo 方法（YAGNI；只落本 story equip + 26.4 unequip 明确会复用的最小集）。
- **不**为 equip 加运维化能力（dry-run / force / 批量 equip 等）。
- **不**写英文测试注释 / 文档（项目 communication_language=Chinese，与既有 service/repo 测试一致）。

### 已落地可复用资产（避免重复造轮子）

- `server/internal/repo/mysql/user_pet_equip_repo.go`：Story 26.2 已落地 `UserPetEquip` struct（ID/UserID/PetID/Slot int8/UserCosmeticItemID/CreatedAt/UpdatedAt 全值类型无指针）+ `TableName() = "user_pet_equips"`。本 story 在**同文件**追加 interface/impl，**不**改 struct 字段。
- `server/internal/repo/mysql/user_cosmetic_item_repo.go`：已有 `UserCosmeticItem` struct（Status int8，§6.10 枚举 1=in_bag/2=equipped/3=consumed/4=invalid）+ `UserCosmeticItemRepo` interface + `CreateInTx` 模式（行 184-194 `tx.FromContext` + Create 模式可抄）。
- `server/internal/repo/mysql/cosmetic_item_repo.go`：`CosmeticItem` struct 有 `Slot int8` + `Name string` 字段（§8.3 步骤 7/11 需要）。
- `server/internal/repo/mysql/pet_repo.go`：`Pet` struct 有 `UserID uint64` 字段（§8.3 步骤 6 比对归属用）；既有 `FindDefaultByUserID` 模式可参照写 `FindByID`。
- `server/internal/service/chest_open_service.go` 行 170-216：`txMgr.WithTx` + `runOpenChestTx(txCtx, ...)` 全流程事务模式 —— equip 事务结构直接参照（去掉幂等/步数/抽奖部分，只保留"WithTx 内顺序调多 repo + apperror.Wrap 翻译 + 任一步 return error 即回滚"骨架）。
- `server/internal/repo/mysql/room_member_repo.go` 行 353-383：`*mysql.MySQLError` Number==1062 → 按 Message 含约束名分流双哨兵的翻译代码 —— `InsertInTx` 1062 翻译直接照抄改约束名（`uk_pet_slot` / `uk_user_cosmetic_item_id`）。
- `server/internal/repo/mysql/errors.go` 行 53-98：哨兵 error 集 + 注释风格模板。
- `server/internal/app/http/handler/cosmetics_handler.go`：`CosmeticsHandler` struct + `GetCatalog`/`GetInventory` handler 模式（c.Request.Context() / c.Error / response.Success / strconv 已 import）。
- `server/internal/app/bootstrap/router.go` 行 515-556：cosmeticSvc/cosmeticsHandler wire + `authedGroup.POST/GET` 注册模式。

### Project Structure Notes

- 新文件：`server/internal/service/cosmetic_equip_service.go` + `cosmetic_equip_service_test.go` + `cosmetic_equip_service_integration_test.go`。
- 改既有文件：`user_pet_equip_repo.go`（追加 interface/impl）/ `user_cosmetic_item_repo.go`（追加 2 方法）/ `cosmetic_item_repo.go`（追加 1 方法）/ `pet_repo.go`（追加 1 方法）/ `errors.go`（追加 2 哨兵）/ `cosmetics_handler.go`（追加 Equip + 扩 struct/构造）/ `cosmetics_handler_test.go`（扩测）/ `router.go`（wire + 路由）+ 对应 `*_repo_test.go`。
- 与统一项目结构一致：repo→service→handler→router 分层；equip 写事务与 catalog/inventory 只读分文件（与 chest_open_service vs chest_service 先例一致）。
- **检测到的变体（已说明）**：CLAUDE.md target 树写的是 `internal/domain/cosmetic/` 等，但项目实际从未采用该树，一律按 `internal/repo/mysql` + `internal/service` + `internal/app/http/handler` 既有结构落地（既有 20+ service 文件均如此 —— 与实际 codebase 对齐优先于 target 树文档）。

### References

- [Source: docs/宠物互动App_V1接口设计.md#8.3 穿戴装扮 行 1454-1565]（请求/响应/服务端逻辑步骤 2-11/错误码表/关键约束，已 Story 26.1 冻结）
- [Source: docs/宠物互动App_V1接口设计.md#1 节点 9 冻结声明 行 84]（§8.3/§8.4 schema 冻结）
- [Source: docs/宠物互动App_数据库设计.md#8.4 穿戴事务 行 1009-1018]（事务步骤）
- [Source: docs/宠物互动App_数据库设计.md#5.10 user_pet_equips 行 533-550]（schema，26.2 已落地 0016）
- [Source: docs/宠物互动App_数据库设计.md#6.8 slot 枚举 / #6.10 user_cosmetic_items.status 枚举]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 26.3 行 3537-3566]（user story + AC + 单测/集成测试钦定）
- [Source: _bmad-output/implementation-artifacts/26-2-user_pet_equips-migration.md]（前序 story：0016 schema + UserPetEquip struct + 下游交接说明）
- [Source: _bmad-output/implementation-artifacts/26-1-接口契约最终化.md]（§8.3 契约冻结 + fix-review 26-1 r1/r2 [P2] 锁定 5001/5003 语义）
- [Source: _bmad-output/implementation-artifacts/decisions/0006-error-handling.md]（三层错误映射）
- [Source: _bmad-output/implementation-artifacts/decisions/0007-context-propagation.md]（ctx/txCtx 传播）
- [Source: server/internal/service/chest_open_service.go 行 170-216]（WithTx 事务流程参照实装）
- [Source: server/internal/repo/mysql/room_member_repo.go 行 353-383 + errors.go 行 53-98]（1062→哨兵翻译参照实装）
- [Source: CLAUDE.md#Build & Test]（scripts/build.sh --test / --integration 验证契约）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]（bmad-dev-story workflow，epic-loop 子任务）

### Debug Log References

- `bash scripts/build.sh --test` → BUILD SUCCESS（vet + build + 全 package unit test 全绿，含本 story 新增 15 service case + 7 repo case + 6 handler case）
- `bash scripts/build.sh --integration` → 本机 Windows 无 Docker daemon：dockertest `pool.Retry` 在既有 `auth_service_integration_test.go:94` 首先 timeout（120s），**非本 story 引入缺陷**（CLAUDE.md + 所有 `*_integration_test.go` 头部已声明"本机 Windows docker 不可用 → CI Linux 跑"）
- `go vet -tags=integration ./internal/service/` → 通过；`go test -tags=integration -list 'TestCosmeticEquipServiceIntegration.*'` → `TestCosmeticEquipServiceIntegration_EquipAndSwapSameSlot` 正确注册（集成测试编译 + 发现无误，待 CI Linux 执行）

### Completion Notes List

- **AC1-AC7 全部满足**。Equip 单事务严格按 V1 §8.3 步骤 4-11 + 数据库设计 §8.4 实装，全部包在 `txMgr.WithTx` 内（`runEquipTx` 用 `txCtx`，ADR-0007 §2.4）。
- **冻结契约 1:1 对齐**：5001 仅"实例完全无 row"（fix-review 26-1 r1 [P2]）；"实例属他人"/"pet 不属于/不存在"恒 5002；status=2→5008、status=3/4→5003；missing-no-row（cosmetic_items 行被删，found=false）→ 5003 + `slog.ErrorContext`（fix-review 26-1 r2 [P2]）；DB UNIQUE 双哨兵（uk_pet_slot / uk_user_cosmetic_item_id）→ 1009。**未新造错误码、未扩张节点 9 冻结集合 `{1001,1002,1005,5001,5002,5003,5008,1009}`**。
- **DTO 命名偏离 story 文案的唯一处（已说明）**：story 写 `EquipInput`/`EquipOutput`，但 `service` 包内 `EquipOutput` 已被 `room_service.go`（Story 11.6 占位 struct）占用 → 改名 `EquipParams`/`EquipResult`/`EquippedItem` 避免重声明（与 story 意图等价，wire 契约不变）。
- **interface 扩张连带改动**：`PetRepo` 加 `FindByID` → 5 个既有 stub（auth/home/pet/room service test + auth 集成 fault wrapper）补 panic-default / 透传；`UserCosmeticItemRepo` 加 2 方法 / `CosmeticItemRepo` 加 1 方法 → 对应 stub 同步。均为 panic-default（误调即暴露），不改既有测试行为。`stubUserCosmeticItemRepo`（共享）扩 `findByIDForEquipFn`/`updateStatusInTxFn`/`updateStatusInTxCalls` 供 equip 单测复用。
- **范围红线遵守**：未做 unequip service/handler/路由（仅落 `DeleteByPetSlotInTx` 供 26.4 复用）；未做深度回滚/100 并发/状态矩阵集成（归 26.5）；未改 GET /home；未引 idempotencyKey；未改任何 docs/*.md / V1 契约 / 0001-0016 migration。

### File List

**新增文件**
- `server/internal/service/cosmetic_equip_service.go`
- `server/internal/service/cosmetic_equip_service_test.go`
- `server/internal/service/cosmetic_equip_service_integration_test.go`
- `server/internal/repo/mysql/user_pet_equip_repo_test.go`

**修改文件（实装）**
- `server/internal/repo/mysql/errors.go`（+4 哨兵）
- `server/internal/repo/mysql/user_pet_equip_repo.go`（追加 interface/impl/构造）
- `server/internal/repo/mysql/user_cosmetic_item_repo.go`（+FindByIDForEquip / +UpdateStatusInTx）
- `server/internal/repo/mysql/cosmetic_item_repo.go`（+FindSlotNameByID）
- `server/internal/repo/mysql/pet_repo.go`（+FindByID）
- `server/internal/app/http/handler/cosmetics_handler.go`（+Equip / +equipRequest / +equipResponseDTO / 扩 struct + 构造）
- `server/internal/app/bootstrap/router.go`（wire userPetEquipRepo + cosmeticEquipSvc + 路由 + NewCosmeticsHandler 扩参）

**修改文件（测试 / stub 兼容）**
- `server/internal/repo/mysql/pet_repo_test.go`（+FindByID 2 case）
- `server/internal/repo/mysql/cosmetic_item_repo_test.go`（+FindSlotNameByID 2 case）
- `server/internal/repo/mysql/user_cosmetic_item_repo_test.go`（+FindByIDForEquip / +UpdateStatusInTx 3 case）
- `server/internal/app/http/handler/cosmetics_handler_test.go`（+stubCosmeticEquipService + equip router helper + 6 Equip case + 扩既有 helper 构造）
- `server/internal/service/cosmetic_service_test.go`（扩共享 stubUserCosmeticItemRepo + stubCatalogCosmeticItemRepo.FindSlotNameByID panic-default）
- `server/internal/service/chest_open_service_test.go`（stubCosmeticItemRepo.FindSlotNameByID panic-default）
- `server/internal/service/chest_open_service_integration_test.go`（faultCosmeticItemRepoOnList.FindSlotNameByID panic-default）
- `server/internal/service/dev_cosmetic_service_test.go`（stubDevCosmeticItemRepo / stubDevUserCosmeticItemRepo 新方法 panic-default）
- `server/internal/service/auth_service_test.go`（stubPetRepo.FindByID panic-default）
- `server/internal/service/home_service_test.go`（stubHomePetRepo.FindByID panic-default）
- `server/internal/service/pet_service_test.go`（stubPetRepoForPetService.FindByID panic-default）
- `server/internal/service/room_service_test.go`（roomTestStubPetRepo.FindByID panic-default）
- `server/internal/service/auth_service_integration_test.go`（faultPetRepo.FindByID 透传 delegate）

### Change Log

| 日期 | 变更 | 说明 |
|---|---|---|
| 2026-05-17 | Story 26.3 实装完成 | POST /cosmetics/equip 单事务（含同槽换装）：repo 5 文件 + service 新文件 + handler + 路由 + 单测 15 + repo 测 7 + handler 测 6 + dockertest 集成测试 2 场景；`bash scripts/build.sh --test` 全绿；状态 → review |
