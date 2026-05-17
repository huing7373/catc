# Story 26.4: POST /cosmetics/unequip 事务 —— CosmeticEquipService.Unequip 单事务实装（V1 §8.4 服务端逻辑步骤 4-8 钦定：校 pet 归属 5002 → `SELECT ... FOR UPDATE` 行锁查装备关系（无行 → 5004）→ `DELETE` 检查 RowsAffected（==0 → 回滚 + 5004 冗余兜底）→ UPDATE 实例 status 回 in_bag(1) → 返回 unequipped DTO）+ 新增 2 个 UserPetEquipRepo 方法（FOR UPDATE 行锁 + RowsAffected DELETE）+ handler + 路由 + ≥4 单测 + dockertest 集成测试

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As an iPhone 用户,
I want 我可以卸下当前装备（按槽位指定）,
so that 我可以让猫不戴这件装扮.

## 故事定位（Epic 26 第四条 = 第二条**业务事务**实装 story）

- **Epic 26 进度**：26.1（契约定稿 §8.3 / §8.4 / §1 节点 9 冻结，**done**）→ 26.2（user_pet_equips migration + `UserPetEquip` GORM struct + TableName，**done**）→ 26.3（POST /cosmetics/equip 事务，含同槽换装，**done** —— 首次落地 `CosmeticEquipService` / `UserPetEquipRepo` / 扩 `PetRepo`/`UserCosmeticItemRepo`/`CosmeticItemRepo`）→ **26.4（本 story，POST /cosmetics/unequip 事务）** → 26.5（Layer 2 集成测试 - 穿戴 / 卸下事务全流程，深度回滚 / 100 并发 / 状态一致性矩阵）→ 26.6（GET /home 扩展 - pet.equips 真实数据）。
- **上游已冻结**（不可改）：V1 §8.4（POST /api/v1/cosmetics/unequip 请求 / 响应 / 服务端逻辑 / 错误码 / 关键约束 —— Story 26.1 锚定并冻结，见 §1 节点 9 冻结声明行 84-87；**fix-review 26-1 r2 [P1] 已强化** §8.4 步骤 5 加 `FOR UPDATE` 行锁 + 步骤 6 `RowsAffected == 0 → 回滚 + 5004` 冗余兜底，行 1608-1609 / 1651 / 1657）+ 数据库设计 §8.4（穿戴事务边界）+ §5.10（user_pet_equips schema，0016 已落地）+ §6.8（slot 枚举 `{1,2,3,4,5,6,7,99}`）+ §6.10（user_cosmetic_items.status 枚举 1=in_bag）。本 story 严格对齐这些**输入**，**不**反向修改任何 `docs/宠物互动App_*.md`。
- **下游依赖本 story**：26.5 Layer 2 集成测试基于本 story 的 unequip happy path + 回滚 3（unequip 事务 mock 最后一步失败）+ 边界 3（unequip 不存在的 slot → 5004）+ 并发不变量（"已空槽必 5004"）；26.6 GET /home pet.equips 与本 story 的 DELETE user_pet_equips 行为一致（卸下后 pet.equips 不再含该 slot）。
- **错误码已就位**（Story 26.1 落地，`server/internal/pkg/errors/codes.go` 行 39-46 + 101-108）：`ErrCosmeticNotOwned`(5002) / `ErrCosmeticSlotMismatch`(5004) / `ErrInvalidParam`(1002) / `ErrServiceBusy`(1009)。本 story **直接复用**，**不**新造错误码、**不**扩张 §1 节点 9 冻结的 unequip 错误码集合 `{1001,1002,1005,5002,5004,1009}`（V1 §8.4 错误码表行 1640-1647 钦定 —— 注意 unequip 错误码集合**比 equip 窄**：无 5001 / 5003 / 5008，因 unequip 按 slot 不按实例 id，不查实例归属 / 不校实例状态）。

## ⚠️ 关键陷阱：不能裸复用 26.3 落地的 `FindByPetSlot` / `DeleteByPetSlotInTx`

> **这是本 story 最易犯的实装错误，dev agent 必读**。

26.3 在 `user_pet_equip_repo.go` 落地了 `FindByPetSlot`（普通 `SELECT ... First`，**无 `FOR UPDATE`**）+ `DeleteByPetSlotInTx`（`Delete()` 返 err 二分，**不读 RowsAffected**，注释明确写"service 层在 FindByPetSlot 命中后才调本方法，目标行必存在；RowsAffected==0 理论不发生，**不在本方法分流**"）。这两个方法是为 **equip 同槽换装步骤 8** 设计的（equip 场景下 FindByPetSlot 命中即立刻 DELETE，无并发卸下竞态需求）。

**unequip 契约（V1 §8.4 步骤 5/6，fix-review 26-1 r2 [P1] 锁定）要求**：

1. **步骤 5 必须 `SELECT user_cosmetic_item_id FROM user_pet_equips WHERE pet_id=? AND slot=? FOR UPDATE`**（对 `pet_id+slot` 行加排他锁，串行化并发 unequip —— 26.3 的 `FindByPetSlot` 是普通 SELECT 无锁，**直接复用会破坏 V1 §8.4 行 1657 钦定的"并发卸下串行化"契约**，重新引入 SELECT-then-DELETE TOCTOU 竞态）。
2. **步骤 6 `DELETE` 必须检查 `RowsAffected`**，`RowsAffected == 0 → 回滚事务 + 返回 5004`（26.3 的 `DeleteByPetSlotInTx` 注释明确"不读 RowsAffected"，**直接复用会让"步骤 5 与步骤 6 间被并发删"场景误带 0 affected rows commit → 误返 `unequipped: true`**，破坏 V1 §8.4 行 1609 / 1651 / 1657 钦定的"已空槽必 5004"不变量 + NFR2 一致性）。

**结论（AC3/Task1 红线）**：本 story **必须在 `user_pet_equip_repo.go` 新增 2 个 unequip 专用方法**（`FindUserCosmeticItemIDByPetSlotForUpdate` + `DeleteByPetSlotInTxReturningAffected`），**不**改 26.3 落地的 `FindByPetSlot` / `DeleteByPetSlotInTx`（equip 仍在用，改动会回归 26.3 + 破坏 26.5 equip 集成测试）。参照实装模式见 §References 的 `room_member_repo.go` `ExistsForShareByRoomAndUser`（FOR SHARE 行锁 Raw SQL，unequip 改 `FOR UPDATE`）+ `DeleteByRoomAndUser`（返 `(RowsAffected int64, error)`，V1 §10.5 leave 事务 6004 兜底同根因模式 —— V1 §8.4 行 1657 显式点名"与 §10.5 leave 同根因正交双保险"）。

## Acceptance Criteria

> 全部源自 V1 §8.4（行 1569-1661，已 Story 26.1 冻结 + fix-review 26-1 r2 [P1] 强化）+ epics.md §Story 26.4（行 3568-3590）+ 数据库设计 §8.4（事务边界，行 1009-1018）。

**AC1 — 路由 + handler 接入**

- 新增 `POST /api/v1/cosmetics/unequip` 路由，挂在 **`authedGroup`**（与 `/cosmetics/equip`（26.3 落地）/ `/cosmetics/catalog` / `/cosmetics/inventory` 同组同模式 —— Auth + RateLimitByUserID 中间件链由 authedGroup 既有链兜底，对应 §8.4 错误码 1001 / 1005）。**不**走 `chestOpenGroup`（unequip 无 idempotency，V1 §8.4 行 1583 钦定，与 equip 同理由 —— 限频走标准 authedGroup 中间件）。路由注册位置：紧跟 `authedGroup.POST("/cosmetics/equip", ...)` 行后（router.go 行 580 之后），加 Story 26.4 注释。
- handler 方法名 `Unequip`，挂在既有 `CosmeticsHandler`（`server/internal/app/http/handler/cosmetics_handler.go`，与 `GetCatalog` / `GetInventory` / `Equip` 同 struct）—— **复用既有 `equipSvc service.CosmeticEquipService` 字段**（unequip service 方法加在 `CosmeticEquipService` interface 上，**不**新建第三个 service / 第三个 handler 注入字段，详见 AC3）。
- handler 从 `c.Request.Context()` 取 ctx 传给 service（ADR-0007 §2.2：**不**直接传 `*gin.Context`，其 Done() 是 nil channel）。userID 从 auth 中间件注入的 context 值取（`c.Get(middleware.UserIDKey)` → `v.(uint64)`，与 `Equip` handler 行 320-330 1:1 同模式 —— 不存在 / 类型断言失败 → 1009 unreachable 兜底）。
- handler **不**直接调 `response.Error` 写 envelope；走 `c.Error(err)` + return，由 `ErrorMappingMiddleware` 翻译（ADR-0006 单一 envelope 生产者，与 `Equip` handler 行 337-341 一致）。成功走 `response.Success(c, unequipResponseDTO(out), "ok")`。

**AC2 — 请求体解析 + 参数校验（V1 §8.4 服务端逻辑步骤 2 行 1605）**

- 请求体 `{petId: string, slot: int}`（V1 §8.4 行 1590-1600）。`petId`：必填、合法 BIGINT 字符串（length ≥ 1，可被 `strconv.ParseUint(s,10,64)` 解析为 uint64 且 != 0 —— 与 `Equip` handler 行 301-313 1:1 同模式）；`slot`：必填、为 int 且**在枚举 `{1,2,3,4,5,6,7,99}` 内**（V1 §8.4 行 1593 + 行 1643 钦定 —— **注意**：equip 请求**无 slot 字段**（equip 的 slot 由 server 从 cosmetic_items 反查），unequip 请求**有 slot 且必须校验枚举**，这是 unequip 请求侧与 equip 的关键差异）。
- 缺失 / 非合法 → **1002 参数错误**（V1 §8.4 行 1643；handler 层校验）。请求体 mirror struct 用**指针类型**区分"字段缺失"vs"显式传零值"（与 26.3 `equipRequest` 行 266-269 同模式：`PetID *string` + `Slot *int` —— 值类型 `int` 缺失会解析为 `0`，与显式传 `0` 无法区分；`0` 不在枚举内故即便误判也会被枚举校验拦截，但**仍用 `*int` 显式区分**保持与 `equipRequest` 模式一致 + 错误消息更精确）。
- slot 枚举校验用一个 helper（如 `isValidSlot(s int) bool` 判 `s ∈ {1,2,3,4,5,6,7,99}`）或显式 switch，置于 handler 文件内（**不**新建 pkg；与既有 handler 参数校验同就近原则）。

**AC3 — Unequip 单事务实装（V1 §8.4 服务端逻辑步骤 3-8，全部步骤同一 `txMgr.WithTx` 事务内，任一步 err → 整体回滚 NFR1/NFR2）**

`Unequip` 方法**加在既有 `CosmeticEquipService` interface 上**（`server/internal/service/cosmetic_equip_service.go`，与 `Equip` 同 interface 同 impl struct `cosmeticEquipServiceImpl` —— equip / unequip 都是 user_pet_equips 写事务，职责同族；**不**新建 `cosmetic_unequip_service.go` / 新 interface / 新构造，复用 26.3 落地的 `NewCosmeticEquipService`(txMgr + 4 repo) 注入集，**不**改构造签名 / **不**改 router wire）。新增 `UnequipParams`（`{UserID, PetID uint64; Slot int8}`）+ `UnequipResult`（`{PetID uint64; Slot int8; Unequipped bool}`）DTO（与 `EquipParams`/`EquipResult` 同文件，命名前缀 `Unequip` 避免与 `Equip*` 冲突）。

`Unequip` 入参兜底校验（`UserID==0 || PetID==0 || slot 不在枚举` → 1002，handler 已校验过这里防御性兜底，与 `Equip` 行 130-133 同模式）后，`s.txMgr.WithTx(ctx, func(txCtx)...)` 内 `runUnequipTx(txCtx, in)` 严格按 V1 §8.4 步骤 4-7 顺序执行（**所有 repo 调用用 `txCtx`** —— ADR-0007 §2.4；参照 `runEquipTx` 行 154-273）：

1. **步骤 4 — 校验 pet 归属**（V1 §8.4 行 1607）：`s.petRepo.FindByID(txCtx, in.PetID)`（复用 26.3 落地的 `PetRepo.FindByID`）：
   - `ErrPetNotFound`（pet 不存在）→ **5002 道具不属于当前用户**（与"非本人 pet"同处理 —— V1 §8.4 错误码表只给 5002 一个出口，pet 不存在不在 §8.4 错误码集合内独立出口；与 26.3 步骤 6 行 186-199 处理 pet 不存在恒 5002 的不变量 1:1 一致）。
   - DB 异常 → **1009**。
   - `pet.UserID != in.UserID` → **5002**。
   - **顺序锚定**：V1 §8.4 步骤 4 校 pet 归属在步骤 5 查装备关系**之前**（先校 ACL 再查资源 —— 与 §8.3 equip 步骤 6 校 pet 归属在步骤 8 同槽查询前同序；不调换为"先查 user_pet_equips 再校 pet"，否则一个不属于当前用户的 pet 上的装备关系会先被 5004/查询触达，泄漏"该 pet 该 slot 有无装备"信息给非属主，违反 ACL 边界）。
2. **步骤 5 — 查装备关系（`FOR UPDATE` 行锁串行化）**（V1 §8.4 行 1608，fix-review 26-1 r2 [P1] 锁定）：调本 story 新增的 `userPetEquipRepo.FindUserCosmeticItemIDByPetSlotForUpdate(txCtx, in.PetID, in.Slot)`（`SELECT user_cosmetic_item_id FROM user_pet_equips WHERE pet_id=? AND slot=? FOR UPDATE`）：
   - 行不存在（repo 返 `ErrUserPetEquipNotFound` 哨兵）→ **5004 装备槽位不匹配**（`apperror.ErrCosmeticSlotMismatch`；该槽位当前无装备，无可卸下对象 —— V1 §8.4 行 1608 + 行 1646）。
   - DB 异常 → **1009**。
   - 命中 → 拿到 `userCosmeticItemID`（步骤 6 用），继续。
   - **`FOR UPDATE` 不可省**（V1 §8.4 行 1657 关键约束 + 行 1651 不变量）：对该 `pet_id+slot` 行加排他锁，并发 unequip 在此排队，输家事务等赢家 commit（行已 DELETE）后进入本步 → SELECT 查不到 → 直接 5004，杜绝两个并发请求都越过本步走到步骤 6。
3. **步骤 6 — 解绑 + 状态回退（`DELETE` 必须检查 affected rows）**（V1 §8.4 行 1609，fix-review 26-1 r2 [P1] 锁定）：调本 story 新增的 `userPetEquipRepo.DeleteByPetSlotInTxReturningAffected(txCtx, in.PetID, in.Slot)`（`DELETE FROM user_pet_equips WHERE pet_id=? AND slot=?`，返 `(rowsAffected int64, err error)`）：
   - `err != nil` → **1009**。
   - `rowsAffected == 0` → **回滚事务（return 5004 让 WithTx 自动 rollback）+ 返回 5004 装备槽位不匹配**（步骤 5 与本步间该行已被并发删 —— 理论上已由步骤 5 `FOR UPDATE` 排他锁阻止，本检查为不依赖锁实现细节的契约级冗余兜底；**禁止**带着 0 affected rows 继续 commit 而误返 `unequipped: true`，V1 §8.4 行 1609 / 1651 / 1657 钦定）。
   - `rowsAffected == 1` → 继续：`s.userCosmeticRepo.UpdateStatusInTx(txCtx, userCosmeticItemID, cosmeticStatusInBag)`（复用 26.3 落地的 `UserCosmeticItemRepo.UpdateStatusInTx` + `cosmeticStatusInBag = 1` 常量，行 37 / 行 236）；`err != nil` → **1009**。
   - `rowsAffected > 1`（理论不发生 —— `uk_pet_slot` UNIQUE 保证 (pet_id, slot) 至多 1 行）：按 `rowsAffected != 0` 即成功处理（与 `room_member_repo.DeleteByRoomAndUser` service 兜底 `!= 0 即成功` 行 424-425 同模式 —— 即 `rowsAffected >= 1` 走 UPDATE + commit；**仅 `== 0` 触发 5004 回滚**）。
4. **步骤 7 — 提交**（`WithTx` fn return nil → 自动 commit；任一步返 error → 自动 rollback；ADR-0007 §2.4 + 数据库设计 §8.4 事务边界 + V1 §8.4 行 1610）。
5. **步骤 8 — 响应**：`{petId, slot, unequipped: true}`（V1 §8.4 行 1611 / 字段表行 1617-1621 / 示例行 1625-1635；`petId` 字符串化下发，`slot` int 直接下发，`unequipped` 恒 `true` —— 失败走错误码不返回 `false`，V1 §8.4 行 1660 钦定）。service 层 `UnequipResult.Unequipped` 恒置 `true`（成功路径才返结果；失败路径 return error 不构造 result）。

**AC4 — 新增 2 个 UserPetEquipRepo 方法（`user_pet_equip_repo.go`，AC3 步骤 5/6）**

在 26.3 落地的 `UserPetEquipRepo` interface（行 89-144）+ `userPetEquipRepo` impl（行 146-205）**追加 2 个方法**（**不**改既有 `FindByPetSlot` / `DeleteByPetSlotInTx` / `InsertInTx` —— equip 仍在用，见本 story §"关键陷阱"红线）：

- `FindUserCosmeticItemIDByPetSlotForUpdate(ctx context.Context, petID uint64, slot int8) (uint64, error)`：
  - SQL：`SELECT user_cosmetic_item_id FROM user_pet_equips WHERE pet_id = ? AND slot = ? LIMIT 1 FOR UPDATE`。
  - **MySQL 8.0 语法红线**：`LIMIT` 必须在 `FOR UPDATE` **之前**（`... FOR UPDATE LIMIT 1` 在 MySQL 5.7+ 是 ER_PARSE_ERROR 1064；GORM 不重写顺序 —— Raw SQL 必须按 MySQL 钦定顺序写，与 `room_member_repo.ExistsForShareByRoomAndUser` 行 463-468 注释钦定一致）。用 `db.WithContext(ctx).Raw("...").Scan(&dst)` 路径（参照 `ExistsForShareByRoomAndUser` Raw+Scan 模式），**不**用 GORM `Clauses(clause.Locking{...}).First()`（Raw SQL 路径与既有 FOR SHARE 行锁实装风格一致 + 显式可控 LIMIT/FOR UPDATE 顺序）。
  - `tx.FromContext(ctx, r.db)`（事务内走 tx 句柄 —— **必须在事务内调用**，事务外 FOR UPDATE 锁立即释放，并发串行化失效；接口注释须显式声明此约束，与 `ExistsForShareByRoomAndUser` 行 444-450 注释风格一致）。
  - 0 行 → 返 `(0, ErrUserPetEquipNotFound)`（复用 26.3 落地哨兵，`server/internal/repo/mysql/errors.go` 行 107-114；service 层 `errors.Is` 区分"slot 无装备 → 5004"vs"DB 异常 → 1009"）。**注意**：Raw + Scan 0 行**不**返 `gorm.ErrRecordNotFound`（与 `ExistsForShareByRoomAndUser` 行 457-458 注释同源 —— Scan 在 0 行时保持 dst zero-value 不报错）；故须**显式判定** 0 行：用 `Scan(&dst).RowsAffected == 0`（或 sql.ErrNoRows 等价判定）→ 返哨兵，**不**靠 `errors.Is(err, gorm.ErrRecordNotFound)`（Raw 路径不产生该 error）。query 失败 → 返 `(0, raw error)` 透传（service 包成 1009）。
- `DeleteByPetSlotInTxReturningAffected(ctx context.Context, petID uint64, slot int8) (int64, error)`：
  - SQL：`DELETE FROM user_pet_equips WHERE pet_id = ? AND slot = ?`（GORM `Where(...).Delete(&UserPetEquip{})` 路径，走 0016 落地的 `uk_pet_slot` 索引）。
  - 返 `(result.RowsAffected, result.Error)`（与 `room_member_repo.DeleteByRoomAndUser` 行 432-441 1:1 同模式 —— `result.Error != nil → (0, err)`；否则 `(result.RowsAffected, nil)`，service 层做 `== 0 → 5004 回滚` 兜底分流）。
  - `tx.FromContext(ctx, r.db)`（**必须在事务内调用**；接口注释须声明，理由同 26.3 `DeleteByPetSlotInTx` 行 109-114）。
- **interface 扩张连带处理**：`UserPetEquipRepo` 加 2 方法 → 所有 `UserPetEquipRepo` 的 stub / fake 实现必须补对应方法（panic-default 或透传 —— interface 扩张不破坏既有测试编译）。**排查范围**：搜 codebase 内所有实现 `UserPetEquipRepo` 的 stub（26.3 在 `cosmetic_equip_service_test.go` 落地了 mock —— 该 mock 必须加新方法 hook；其他 service test 若有共享 stub 亦同步）。**务必全量编译验证**（`bash scripts/build.sh --test` 全包编译 —— interface 扩张漏补 stub 会编译失败，与 26.3 Dev Notes "interface 扩张连带改动" 行 187 教训一致）。

**AC5 — 单元测试覆盖（≥4 case，mocked repo，扩 `cosmetic_equip_service_test.go`）**

epics.md §Story 26.4 行 3585-3589 显式钦定（**全部必须有**）—— 测试加在既有 `cosmetic_equip_service_test.go`（与 Equip 单测同文件，复用既有 mock repo / mock txManager 基础设施；mock txManager `WithTx` 直接调 fn 传透传 ctx 不真起事务，与 26.3 行 74 + chest_open_service_test 同模式）：

- **happy**：该槽位有装备 → 卸下成功，mock 校验：步骤 4 `petRepo.FindByID` 返属主 pet → 步骤 5 `FindUserCosmeticItemIDByPetSlotForUpdate` 返 `(userCosmeticItemID, nil)` → 步骤 6 `DeleteByPetSlotInTxReturningAffected` 返 `(1, nil)` → `UpdateStatusInTx(userCosmeticItemID, 1 in_bag)` 被调 → 返 `UnequipResult{PetID, Slot, Unequipped: true}`（断言 DELETE 被调 + 实例 status→in_bag(1) + result 字段值正确）。
- **edge：该槽位无装备 → 5004**：步骤 5 `FindUserCosmeticItemIDByPetSlotForUpdate` 返 `(0, ErrUserPetEquipNotFound)` → service 翻译 `apperror.ErrCosmeticSlotMismatch`（5004）+ 断言 DELETE / UpdateStatus **未被调用**（事务在步骤 5 即 return error 回滚）。
- **edge：pet 不属于当前用户 → 5002**：步骤 4 `petRepo.FindByID` 返 `pet.UserID != in.UserID` → 5002（`apperror.ErrCosmeticNotOwned`）+ 断言步骤 5/6 **未被调用**。**建议补**：pet 不存在（`petRepo.FindByID` 返 `ErrPetNotFound`）→ 同样 5002（与 26.3 步骤 6 不变量对齐 —— 补全 pet ACL 分支矩阵）。
- **edge：卸下时事务部分失败（mock）→ 整体回滚**（epics.md 行 3589 钦定 "user_pet_equips 仍存在 + 实例仍 equipped"）：mock `DeleteByPetSlotInTxReturningAffected` 返 `(1, nil)` 但 `UpdateStatusInTx` 返 DB error → service 返 1009 + 断言 `WithTx` 收到 error（fn return non-nil → WithTx 触发 rollback；单测层 mock txManager 验证 fn 返 error 即可，**不**真起事务验回滚 —— 真回滚验证归 AC6 dockertest + Story 26.5）。
- **建议补**（强烈建议，对应 fix-review 26-1 r2 [P1] 锁定的契约级冗余兜底，Story 26.5 会深度覆盖但本 story 单测应先有最小验证）：
  - `RowsAffected == 0 → 5004 回滚`：mock 步骤 5 返 `(id, nil)`（命中）但步骤 6 `DeleteByPetSlotInTxReturningAffected` 返 `(0, nil)`（步骤 5/6 间被并发删的模拟）→ service 必须返 **5004**（**不**是 1009 / **不**是误成功）+ 断言 `UpdateStatusInTx` **未被调** + WithTx 收到 error 回滚。**这是本 story 区别于 equip 的核心防御逻辑，必须有单测覆盖**。
  - 步骤 4 `petRepo.FindByID` 返非 NotFound 的 DB error → 1009。
  - 步骤 5 `FindUserCosmeticItemIDByPetSlotForUpdate` 返非哨兵 raw DB error → 1009。
- mock 模式：扩 26.3 在 `cosmetic_equip_service_test.go` 落地的 mock `UserPetEquipRepo`（加 `findUCIDByPetSlotForUpdateFn` / `deleteByPetSlotReturningAffectedFn` + 调用计数 hook）+ 复用既有 mock `petRepo`（`FindByID` hook）/ mock `userCosmeticRepo`（`UpdateStatusInTx` hook）/ mock txManager。

**AC6 — 集成测试覆盖（dockertest，真实 MySQL，扩 `cosmetic_equip_service_integration_test.go`，build tag `integration`）**

epics.md §Story 26.4 行 3590 钦定："装一件 hat → unequip → user_pet_equips 行无 + 实例 status=1"：

- 复用 26.3 在 `cosmetic_equip_service_integration_test.go` 落地的 dockertest setup helper（`//go:build integration` + dockertest 起 MySQL + 跑 migrations，参照文件顶部既有 setup）。新增测试函数（如 `TestCosmeticEquipServiceIntegration_UnequipHappyPath`）：创建 user + pet + 1 件 hat 实例 → 先 `Equip`（复用 26.3 Equip 把 hat 装上，status→2 + user_pet_equips 1 行）→ 再 `Unequip(petId, slot=1 hat)` → 断言 DB `user_pet_equips` 该 (pet_id, slot) **行不存在**（0 行）+ 该实例 `status == 1 (in_bag)` + `UnequipResult.Unequipped == true`。
- **建议补 1 个**（契约级不变量，对应 V1 §8.4 行 1651 "已空槽必 5004"）：上一步 unequip 成功后**再次** `Unequip` 同 (petId, slot) → 断言返 **5004**（`apperror.ErrCosmeticSlotMismatch`，**不**是幂等成功）+ DB 状态不变（验证 unequip 非幂等 + 空槽显式报错契约，V1 §8.4 行 1649 / 1656）。
- **范围红线**：深度回滚（mock 最后一步失败真回滚验证）/ 100 并发 unequip 串行化压测（验证"有且仅有第一个删到行的请求成功，其余必 5004"）/ 状态一致性矩阵归 **Story 26.5**（epics.md 行 3592-3616；本 story 集成测试仅 AC6 的 happy + 重复 unequip 5004 两场景，**不**做深度回滚 / 并发 stress）。
- build tag / Docker 环境：本机 Windows 无 Docker daemon → 集成测试 `bash scripts/build.sh --integration` 本机无法执行（与 26.3 / 既有所有 `*_integration_test.go` 同环境限制，CI Linux 跑）；本 story 验收以"`-tags=integration` vet + 编译 + `go test -tags=integration -list` 测试函数正确注册"为本机可验证标准（与 26.3 Dev Notes 行 110 + Debug Log 行 180 同模式）。

**AC7 — 验证脚本通过**

`bash scripts/build.sh --test` 全绿（vet + build + `go test -count=1 ./...` 单测，含本 story 新增 ≥4 unequip service case + 新增 repo 方法的 repo 层单测）；集成测试 `bash scripts/build.sh --integration` 本机 Windows 无 Docker 无法执行（CI Linux 跑），本机以 `go vet -tags=integration ./internal/service/` 通过 + `go test -tags=integration -list 'TestCosmeticEquipServiceIntegration.*'` 新增 unequip 测试函数正确注册为可验证标准（CLAUDE.md Build & Test 节 + 26.3 Debug Log 同模式）。

## Tasks / Subtasks

- [x] **Task 1 — repo 层新增 2 个 unequip 专用方法**（AC4）
  - [x] `server/internal/repo/mysql/user_pet_equip_repo.go`：在既有 `UserPetEquipRepo` interface 追加 `FindUserCosmeticItemIDByPetSlotForUpdate(ctx, petID uint64, slot int8) (uint64, error)` + `DeleteByPetSlotInTxReturningAffected(ctx, petID uint64, slot int8) (int64, error)` 方法签名 + 完整接口注释（FOR UPDATE 行锁语义 / 必须事务内调用约束 / NotFound 哨兵语义 / RowsAffected 兜底语义 / V1 §8.4 行号引用）
  - [x] 同文件 `userPetEquipRepo` impl 追加两方法实装：`FindUserCosmeticItemIDByPetSlotForUpdate` 用 `tx.FromContext` + Raw `SELECT user_cosmetic_item_id ... LIMIT 1 FOR UPDATE`（LIMIT 在 FOR UPDATE 前）+ `Scan` + `RowsAffected==0 → ErrUserPetEquipNotFound 哨兵`（显式判 0 行）；`DeleteByPetSlotInTxReturningAffected` 用 `tx.FromContext` + `Where(...).Delete(&UserPetEquip{})` 返 `(result.RowsAffected, result.Error)`（抄 room_member `DeleteByRoomAndUser`）
  - [x] **不**改 26.3 落地的 `FindByPetSlot` / `DeleteByPetSlotInTx` / `InsertInTx`（equip 仍在用）
  - [x] 补 repo 层单测（sqlmock；扩 `user_pet_equip_repo_test.go`）：FOR UPDATE 命中返 id / 0 行返哨兵 / query 失败 raw 透传（3 case）；DELETE 删 1 行 (1,nil) / 删 0 行 (0,nil) / DB 错 (0,err)（3 case）。FOR UPDATE Raw SQL 用 `ExpectQuery("FOR UPDATE")` 正则匹配
  - [x] 全量编译排查 `UserPetEquipRepo` 唯一 stub（`stubUserPetEquipRepo` in `cosmetic_equip_service_test.go`）补 2 新方法 hook（panic-default）
- [x] **Task 2 — service 层 Unequip 事务实装**（AC3）
  - [x] `cosmetic_equip_service.go`：`CosmeticEquipService` interface 追加 `Unequip(ctx, in UnequipParams) (*UnequipResult, error)` + 接口注释（错误码集合 {5002,5004,1002,1009} + 各触发条件）
  - [x] 同文件追加 `UnequipParams`（`UserID, PetID uint64; Slot int8`）+ `UnequipResult`（`PetID uint64; Slot int8; Unequipped bool`）DTO + 字段值规则注释 + V1 §8.4 行号锚定
  - [x] `cosmeticEquipServiceImpl` 追加 `Unequip`（入参兜底 1002 + `validUnequipSlot` 枚举校验 → `s.txMgr.WithTx`）+ `runUnequipTx` 严格按 V1 §8.4 步骤 4-7：步骤 4 `petRepo.FindByID`（ErrPetNotFound/UserID 不匹配→5002；DB 错→1009）→ 步骤 5 `FindUserCosmeticItemIDByPetSlotForUpdate`（NotFound→5004；DB 错→1009）→ 步骤 6 `DeleteByPetSlotInTxReturningAffected`（err→1009；rowsAffected==0→5004 回滚；>=1→`UpdateStatusInTx(uciID, cosmeticStatusInBag)`）→ 步骤 8 返 `UnequipResult{...Unequipped: true}`。所有 repo 调用用 `txCtx`（ADR-0007 §2.4）
  - [x] 错误映射三层（ADR-0006）：复用 `apperror.ErrCosmeticNotOwned`/`ErrCosmeticSlotMismatch`/`ErrServiceBusy`/`ErrInvalidParam` + `DefaultMessages`，无新造错误码 / 无新增 message
- [x] **Task 3 — handler + 路由接入**（AC1, AC2）
  - [x] `cosmetics_handler.go`：加 `Unequip(c *gin.Context)`（ShouldBindJSON→1002；`unequipRequest{PetID *string; Slot *int}` mirror struct；petId 非 nil+非空+ParseUint 合法+!=0→1002；slot 非 nil + `isValidSlot` 枚举校验→1002；取 userID；调 `h.equipSvc.Unequip(...)`；成功 `response.Success(c, unequipResponseDTO(out), "ok")`）+ `unequipRequest` struct + `isValidSlot` helper + `unequipResponseDTO`（petId 字符串化 / slot int 直下 / unequipped bool 直下）
  - [x] **复用既有 `equipSvc service.CosmeticEquipService` 字段**（Unequip 在同 interface；不扩 struct / 不改构造签名 / 不改 router wire）
  - [x] `cosmetics_handler.go` 顶部 doc 注释更新：节点 9 阶段列表加 Unequip 已落地表述（替换原 future 占位注释）
  - [x] `router.go`：在 `authedGroup.POST("/cosmetics/equip", ...)` 后加 `authedGroup.POST("/cosmetics/unequip", cosmeticsHandler.Unequip)` + Story 26.4 注释（复用 26.3 cosmeticEquipSvc 实例不新建）
- [x] **Task 4 — 测试**（AC5, AC6）
  - [x] `cosmetic_equip_service_test.go`：扩 mock `stubUserPetEquipRepo`（加 `findUCIDByPetSlotForUpdateFn` + `deleteByPetSlotReturningAffFn` + 调用计数 `findUCIDForUpdateCalls`/`deleteReturningAffArg`，panic-default）+ 加 10 Unequip case（happy / slot 无装备 5004 / pet not owned 5002 / pet not found 5002 / 事务部分失败 1009 / **RowsAffected==0 → 5004 回滚** / 步骤4 DB error 1009 / 步骤5 DB error 1009 / 入参兜底 UserID=0 1002 / slot 不在枚举 1002）
  - [x] `cosmetic_equip_service_integration_test.go`：加 `TestCosmeticEquipServiceIntegration_UnequipHappyPath`（Equip 装 hat → Unequip → user_pet_equips 0 行 + 实例 status=1 + Unequipped=true + 重复 Unequip → 5004 + DB 状态不变）—— `go vet -tags=integration` 通过 + `-tags=integration -list` 正确注册（执行需 CI Linux Docker）
  - [x] `cosmetics_handler_test.go`：扩 `stubCosmeticEquipService`（加 `unequipFn` + `Unequip` 方法）+ `buildCosmeticsUnequipHandlerRouter` helper + 加 8 Unequip handler case（happy DTO {petId,slot,unequipped:true} + 1002 缺 petId / petId 非 BIGINT / 缺 slot / slot 不在枚举 / JSON 类型错 + userID 缺失 1009 + service error 透传）
  - [x] 全量编译排查：`UserPetEquipRepo` / `CosmeticEquipService` 各仅 1 个 stub（均已同步加新方法）；`bash scripts/build.sh --test` 全包编译通过
- [x] **Task 5 — 验证 + 收尾**（AC7）
  - [x] `bash scripts/build.sh --test` 全绿（vet + build + 全 unit test 全 pass，BUILD SUCCESS；含本 story 新增 10 service + 6 repo + 8 handler case）
  - [x] `go vet -tags=integration ./internal/service/` 通过 + `go test -tags=integration -list 'TestCosmeticEquipServiceIntegration.*'` 新增 `TestCosmeticEquipServiceIntegration_UnequipHappyPath` 正确注册（本机 Windows 无 Docker，执行归 CI Linux）
  - [x] 自检：无新造错误码（复用 5002/5004/1002/1009）/ 无改 docs/*.md / 无改 V1 §8.4 契约 / 无改 0001-0016 migration / 无改 26.3 落地的 `FindByPetSlot`/`DeleteByPetSlotInTx`/`InsertInTx` / 范围红线全部遵守

## Dev Notes

### 关键架构约束（必须遵守）

- **目录形态与 CLAUDE.md target 不同**：实际代码不在 `internal/domain/` —— repo 在 `internal/repo/mysql/`、service 在 `internal/service/`、handler 在 `internal/app/http/handler/`、路由 wire 在 `internal/app/bootstrap/router.go`。按**实际既有结构**落地，**不**按 CLAUDE.md §"节点 1 之后的目录形态（target）"的理想树新建 `domain/cosmetic/` 等目录（与 26.3 Dev Notes 行 117 同结论 —— 既有 20+ service 文件均如此）。
- **ctx 必传（ADR-0007 + CLAUDE.md）**：service/repo 所有导出函数第一参数 `ctx context.Context`；handler 从 `c.Request.Context()` 取（**不**把 `*gin.Context` 当 ctx —— 其 Done() 是 nil channel）；`txMgr.WithTx(ctx, fn)` 内所有 repo 调用用 **`txCtx`** 而非外层 ctx；repo 用 `tx.FromContext(ctx, r.db).WithContext(ctx)` 模式（事务内走 tx 句柄 —— FOR UPDATE 行锁**必须在事务内**才有效，事务外锁立即释放并发串行化失效）。
- **错误三层映射（ADR-0006）**：repo 返 raw / 哨兵 error（**不**返 *AppError / 业务码）→ service 用 `apperror.New` / `apperror.Wrap` + `errors.Is` 翻译为 5002/5004/1009 → handler `c.Error(err)` 透传 → `ErrorMappingMiddleware` 写统一 envelope（handler **不**自己调 `response.Error`）。错误码已在 `server/internal/pkg/errors/codes.go` 行 39-46/101-108 定义（Story 26.1），**直接复用** `ErrCosmeticNotOwned`(5002)/`ErrCosmeticSlotMismatch`(5004)/`ErrInvalidParam`(1002)/`ErrServiceBusy`(1009)。
- **事务边界归 service**（repo 包注释）：repo **不**调 `txMgr.WithTx`；事务由 `cosmetic_equip_service.go` 用 `s.txMgr.WithTx` 控制（fn 返 nil → commit / 返 error → rollback / panic 自动 rollback）。`tx.Manager` interface（`server/internal/repo/tx/manager.go`）签名 = service 内 `txManager` interface（行 99-103），单测注入 mock 直接调 fn 不真起事务。

### V1 §8.4 unequip vs §8.3 equip 关键差异（dev 必须吃透）

| 维度 | equip（26.3 已落地） | unequip（本 story） |
|---|---|---|
| 请求体 | `{petId, userCosmeticItemId}`（**无 slot**，slot 由 server 反查 cosmetic_items） | `{petId, slot}`（**有 slot 且必须校枚举**，**无 userCosmeticItemId** —— 按 slot 不按实例 id 定位，§5.10 UNIQUE(pet_id,slot) 保证唯一，V1 §8.4 行 1655） |
| 错误码集合 | `{1001,1002,1005,5001,5002,5003,5008,1009}`（8 个） | `{1001,1002,1005,5002,5004,1009}`（6 个 —— **无 5001/5003/5008**：不查实例归属 / 不校实例状态，只按 slot 删行） |
| 实例归属校验 | 步骤 4 查实例归属（5001/5002） | **无**（unequip 不接受 userCosmeticItemId，无实例归属概念；user_cosmetic_item_id 是步骤 5 从 user_pet_equips 反查得到的，**不**校它属谁——pet 已校归属即足够） |
| 行锁 | 步骤 8 `FindByPetSlot` 普通 SELECT（同槽换装无并发卸下竞态） | 步骤 5 **`SELECT ... FOR UPDATE` 排他锁**（并发 unequip 串行化，fix-review 26-1 r2 [P1] 锁定） |
| DELETE 兜底 | 步骤 8 `DeleteByPetSlotInTx` 不读 RowsAffected（命中后必删） | 步骤 6 **`DELETE` 检查 RowsAffected，==0 → 回滚 + 5004**（不依赖锁的契约级冗余兜底，fix-review 26-1 r2 [P1] 锁定） |
| 幂等 | 无 idempotencyKey（重复 equip 同实例 status=2→5008 兜底） | 无 idempotencyKey（重复 unequip 同空槽 → 5004，**非**幂等 noop —— V1 §8.4 行 1649/1656/1658，client 显式感知"槽位本来就空"可能是 UI 不同步信号） |
| `unequipped` 字段 | N/A | 响应恒 `true`（失败走错误码不返 `false`，V1 §8.4 行 1660 —— 防 client 解析为可选/可 false） |

### 范围红线（本 story 明确不做）

- **不**改 26.3 落地的 `FindByPetSlot` / `DeleteByPetSlotInTx` / `InsertInTx`（equip 路径在用 —— 见 §"关键陷阱"；改动会回归 Story 26.3 happy path + 破坏 26.5 equip 同槽换装 / 回滚集成测试）。本 story 在 `user_pet_equip_repo.go` **新增**专用 2 方法，与 equip 方法**正交共存**。
- **不**做深度回滚 / 100 并发 unequip 串行化压测 / 状态一致性矩阵集成测试（Story 26.5 钦定 —— epics.md 行 3592-3616 含"回滚 3：unequip 事务 mock 最后一步失败 → 验证 user_pet_equips 行未删" + 并发不变量深度验证）。本 story 集成测试仅 AC6 的 happy + 重复 unequip 5004 两场景；单测 AC5 含 `RowsAffected==0→5004` 最小验证但**不**做真并发。
- **不**做 GET /home pet.equips 真实化（Story 26.6；本 story DELETE user_pet_equips 行后 26.6 的 JOIN 查询自然不再返该 slot —— 行为一致即可，**不**改 home_service / GET /home）。
- **不**引入 `idempotencyKey`（V1 §8.4 行 1583/1658 钦定 unequip **无**幂等键；重复 unequip 同空槽由 5004 拦截，与开箱/合成的 idempotency 模式不同 —— **不**抄 chest_open_service 的 ClaimPending/幂等记录逻辑）。
- **不**新建 `cosmetic_unequip_service.go` / 新 interface / 新 handler / 新构造（Unequip 加在 26.3 落地的 `CosmeticEquipService` interface + `cosmeticEquipServiceImpl` impl + 复用 `NewCosmeticEquipService` 注入集 + 复用 `CosmeticsHandler.equipSvc` 字段 —— equip/unequip 同族写事务，与 chest_open vs chest 分文件先例**不同**：那是只读 vs 写事务分文件，equip vs unequip 同为 user_pet_equips 写事务故同 interface/同文件）。
- **不**改 V1 §8.4 契约 / 数据库设计 §5.10/§8.4/§6.8/§6.10 / 任何 `docs/宠物互动App_*.md`（契约**输入**，严格对齐不修改；若发现实装与契约不一致 → 优先改本 story / 实装对齐契约，**不**反向改 docs）。
- **不**改 0001-0016 既有 migration（26.2 已落地 0016 user_pet_equips schema；本 story 仅写 service/repo/handler 代码，**无** migration）。
- **不**新造错误码 / 不扩张 §1 节点 9 冻结的 unequip 错误码集合 `{1001,1002,1005,5002,5004,1009}`（5004 = `ErrCosmeticSlotMismatch` 已 Story 26.1 落地 codes.go 行 42/104；空槽 / RowsAffected==0 均映射既有 5004，**不**新造 / **不**复用 5001/5003）。
- **不**为 unequip 加运维化能力（dry-run / force / 批量 unequip / 卸下全部槽位等）。
- **不**写英文测试注释 / 文档（项目 communication_language=Chinese，与既有 service/repo 测试一致）。

### 已落地可复用资产（避免重复造轮子）

- `server/internal/repo/mysql/user_pet_equip_repo.go`：26.3 落地 `UserPetEquip` struct（行 64-72）+ `TableName()`（行 75）+ `UserPetEquipRepo` interface（行 89-144）+ `userPetEquipRepo` impl（行 146-205）+ `NewUserPetEquipRepo`（行 153）。本 story 在**同文件**追加 2 方法到 interface + impl，**不**改既有 struct / 既有 3 方法。
- `server/internal/repo/mysql/room_member_repo.go`：`ExistsForShareByRoomAndUser`（行 443-468）—— FOR SHARE 行锁 Raw SQL + `LIMIT 在 locking clause 前` MySQL 8.0 语法约束 + Raw+Scan 0 行不返 ErrRecordNotFound 的处理（unequip FOR UPDATE 改 `FOR SHARE`→`FOR UPDATE` + 显式判 0 行返哨兵，注释钦定语法红线行 463-466 直接照搬）；`DeleteByRoomAndUser`（行 432-441）—— 返 `(result.RowsAffected, result.Error)` 模式 + service 层 `==0` 兜底分流注释（行 420-425）**直接照抄改表名 / 哨兵语义**（V1 §8.4 行 1657 显式点名"与 §10.5 leave 同根因正交双保险"——这是**钦定的同模式实装**）。
- `server/internal/repo/mysql/errors.go`：`ErrUserPetEquipNotFound`（行 107-114，26.3 已落地）—— 本 story 步骤 5 直接复用（FOR UPDATE 0 行 → 该哨兵 → service 翻 5004）；`ErrPetNotFound`（行 46/58）—— 步骤 4 pet 不存在复用。**不**新增哨兵（unequip 无 INSERT 故无 1062 双哨兵需求）。
- `server/internal/repo/mysql/user_cosmetic_item_repo.go`：`UpdateStatusInTx`（26.3 落地）—— 步骤 6 复用（实例 status 回 `cosmeticStatusInBag=1`）。
- `server/internal/repo/mysql/pet_repo.go`：`FindByID`（26.3 落地，NotFound → `ErrPetNotFound`）—— 步骤 4 校 pet 归属复用。
- `server/internal/service/cosmetic_equip_service.go`：`cosmeticStatusInBag`(1) 常量（行 37）+ `EquipParams`/`EquipResult`/`EquippedItem` DTO 模式（行 47-72）+ `Equip` 入参兜底 + WithTx 骨架（行 129-148）+ `runEquipTx` 步骤化事务 + 三层错误翻译 + slog 模式（行 154-273）—— Unequip / runUnequipTx 直接参照此结构（去掉实例归属/状态校验/同槽换装/INSERT，换成 pet 校验 + FOR UPDATE 查 + RowsAffected DELETE + status 回 1）。`txManager` interface（行 99-103）+ `NewCosmeticEquipService` 注入集（行 109-123）复用不改。
- `server/internal/app/http/handler/cosmetics_handler.go`：`Equip`（行 292-344）—— ShouldBindJSON / 指针 mirror struct 缺失校验 / ParseUint 1002 / 取 userID / c.Error / response.Success 模式；`equipRequest`（行 266-269）指针类型区分缺失模式；`equipResponseDTO`（行 361-374）BIGINT 字符串化 / int 直下转换原则。Unequip / unequipRequest / unequipResponseDTO 直接参照（加 slot 枚举校验）。
- `server/internal/app/bootstrap/router.go` 行 528-580：cosmeticEquipSvc wire（行 537-543）+ `authedGroup.POST("/cosmetics/equip", ...)`（行 580）注册模式 —— unequip 路由紧跟其后注册，**复用** `cosmeticEquipSvc` / `cosmeticsHandler` 实例不新建。
- `server/internal/pkg/errors/codes.go` 行 39-46/101-108：`ErrCosmeticNotOwned`(5002)/`ErrCosmeticSlotMismatch`(5004)/`ErrInvalidParam`(1002)/`ErrServiceBusy`(1009) + `DefaultMessages` —— 直接复用。

### Project Structure Notes

- **无新增文件**（与 26.3 新建 `cosmetic_equip_service.go` 不同 —— 本 story 全部追加到既有文件）：unequip service 方法加在 `cosmetic_equip_service.go`（同 interface/impl）；unequip repo 2 方法加在 `user_pet_equip_repo.go`（同 interface/impl）；unequip handler 加在 `cosmetics_handler.go`（同 struct，复用 equipSvc 字段）。
- 改既有文件（实装）：`user_pet_equip_repo.go`（+2 方法）/ `cosmetic_equip_service.go`（+Unequip 方法 +UnequipParams/UnequipResult DTO）/ `cosmetics_handler.go`（+Unequip handler +unequipRequest +unequipResponseDTO + 顶部 doc 更新）/ `router.go`（+1 路由行）。
- 改既有文件（测试 / stub 兼容）：`user_pet_equip_repo_test.go`（+新 2 方法 repo 测）/ `cosmetic_equip_service_test.go`（扩 mock UserPetEquipRepo + ≥4 Unequip case）/ `cosmetic_equip_service_integration_test.go`（+unequip dockertest）/ `cosmetics_handler_test.go`（扩 stub CosmeticEquipService + Unequip handler case）+ 任何其他实现 `UserPetEquipRepo`/`CosmeticEquipService` 的共享 stub 补 panic-default。
- 与统一项目结构一致：repo→service→handler→router 分层；unequip 与 equip 同为 user_pet_equips 写事务故同 interface/同文件（与 chest_open vs chest 只读/写分文件先例**正交** —— 同族写事务不再分文件）。
- **检测到的变体（已说明）**：CLAUDE.md target 树写 `internal/domain/cosmetic/` 等，但项目实际从未采用，一律按 `internal/repo/mysql` + `internal/service` + `internal/app/http/handler` 既有结构落地（与 26.3 Dev Notes 行 152 同结论 —— 与实际 codebase 对齐优先于 target 树文档）。

### References

- [Source: docs/宠物互动App_V1接口设计.md#8.4 卸下装扮 行 1569-1661]（请求/响应/服务端逻辑步骤 2-8/错误码表/关键约束 —— Story 26.1 冻结 + fix-review 26-1 r2 [P1] 强化步骤 5 FOR UPDATE + 步骤 6 RowsAffected==0 回滚兜底）
- [Source: docs/宠物互动App_V1接口设计.md#1 节点 9 冻结声明 行 84-87]（§8.3/§8.4 schema 冻结 + 回归触发清单含 unequip handler）
- [Source: docs/宠物互动App_数据库设计.md#8.4 穿戴事务 行 1009-1018]（事务边界 —— unequip 复用同事务原则：校验 + DELETE + status 回退原子）
- [Source: docs/宠物互动App_数据库设计.md#5.10 user_pet_equips 行 533-550]（schema + uk_pet_slot UNIQUE(pet_id,slot) —— slot 唯一定位依据，0016 已落地）
- [Source: docs/宠物互动App_数据库设计.md#6.8 slot 枚举 / #6.10 user_cosmetic_items.status 枚举]（slot `{1,2,3,4,5,6,7,99}` 请求侧校验 + status=1 in_bag 回退目标值）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 26.4 行 3568-3590]（user story + AC + ≥4 单测 / dockertest 钦定）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 26.5 行 3592-3616]（下游：unequip 回滚 3 / 边界 3 / 并发不变量深度集成测试归 26.5 —— 本 story 范围红线依据）
- [Source: _bmad-output/implementation-artifacts/26-3-post-cosmetics-equip-事务.md]（前序 story：CosmeticEquipService / UserPetEquipRepo / handler / router 落地 + interface 扩张连带 stub 教训行 187 + DTO 命名偏离说明行 186）
- [Source: _bmad-output/implementation-artifacts/26-1-接口契约最终化.md]（§8.4 契约冻结 + fix-review 26-1 r2 [P1] 锁定 FOR UPDATE + RowsAffected 兜底不变量）
- [Source: _bmad-output/implementation-artifacts/decisions/0006-error-handling.md]（三层错误映射）
- [Source: _bmad-output/implementation-artifacts/decisions/0007-context-propagation.md]（ctx/txCtx 传播 §2.2/§2.4）
- [Source: server/internal/service/cosmetic_equip_service.go 行 129-273]（Equip/runEquipTx 事务流程参照实装 —— Unequip 同骨架）
- [Source: server/internal/repo/mysql/room_member_repo.go 行 432-468]（DeleteByRoomAndUser RowsAffected 返回模式 + ExistsForShareByRoomAndUser FOR SHARE 行锁 Raw SQL + MySQL 8.0 LIMIT/locking 语法红线 —— V1 §8.4 行 1657 钦定同根因实装参照）
- [Source: server/internal/repo/mysql/user_pet_equip_repo.go 行 89-205]（26.3 落地 UserPetEquipRepo interface/impl —— 本 story 追加 2 方法基座 + 不可改的既有 3 方法红线）
- [Source: CLAUDE.md#Build & Test]（scripts/build.sh --test / --integration 验证契约）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]（bmad-dev-story workflow，红绿循环）

### Debug Log References

- `bash scripts/build.sh --test`：vet + build + `go test -count=1 ./...` 全包 → **BUILD SUCCESS，OK: all tests passed**（含本 story 新增 24 个 unequip 测试：10 service + 6 repo + 8 handler）。
- 集成测试本机 Windows 无 Docker daemon 无法执行（与 26.3 / 既有所有 `*_integration_test.go` 同环境限制，CI Linux 跑）；本机降级验证：`go vet -tags=integration ./internal/service/` **通过** + `go test -tags=integration -list 'TestCosmeticEquipServiceIntegration.*'` 列出 `TestCosmeticEquipServiceIntegration_EquipAndSwapSameSlot` + 新增 `TestCosmeticEquipServiceIntegration_UnequipHappyPath` **正确注册编译通过**（AC6/AC7 本机可验证标准达成，与 story 既定降级路径一致）。
- gofmt 注：`gofmt -d` 对 CRLF 行尾的工作树文件报整文件差异（Windows checkout core.autocrlf 与 gofmt 交互产物，非本 story 代码格式问题）；项目权威验证门是 `bash scripts/build.sh --test`（vet + build + test），已 BUILD SUCCESS。`git show HEAD:<file>`（LF 输出）对 router.go / user_pet_equip_repo.go 均 gofmt-clean，证明非新引入格式问题。

### Completion Notes List

- repo 层新增 2 个 unequip 专用方法（`FindUserCosmeticItemIDByPetSlotForUpdate` Raw FOR UPDATE 行锁 + LIMIT 在 FOR UPDATE 前 MySQL 8.0 语法红线 + 显式判 0 行返哨兵；`DeleteByPetSlotInTxReturningAffected` 返 RowsAffected），**与 26.3 落地的 `FindByPetSlot`/`DeleteByPetSlotInTx`/`InsertInTx` 正交共存，三方法零改动**（§"关键陷阱"红线遵守）。
- service `Unequip` 严格按 V1 §8.4 步骤 4-7：步骤 4 校 pet 归属（先 ACL 后查资源，pet 不存在恒 5002 与 26.3 不变量对齐）→ 步骤 5 FOR UPDATE 行锁查（并发卸下串行化，NotFound→5004）→ 步骤 6 DELETE 检查 RowsAffected（**`RowsAffected==0 → 回滚 + 5004`** 契约级冗余兜底，禁止 0 affected rows 误 commit）→ UpdateStatusInTx 实例 status 回 in_bag(1)。全部 repo 调用用 `txCtx`（ADR-0007 §2.4）。
- 错误码集合严格 = 冻结契约 `{1001,1002,1005,5002,5004,1009}` 中 service 产出子集 `{1002,5002,5004,1009}`（1001/1005 authedGroup 中间件兜底）；**无 5001/5003/5008**（unequip 按 slot 不按实例 id，不查实例归属/不校实例状态）；零新造错误码。
- ✅ 核心防御逻辑单测覆盖：`TestUnequip_DeleteRowsAffectedZero_Returns5004`（步骤 5 命中但步骤 6 返 (0,nil) → 必 5004 + UpdateStatusInTx 未被调，本 story 区别于 equip 的关键不变量，fix-review 26-1 r2 [P1] 锁定）。
- 集成测试覆盖 happy（Equip→Unequip→user_pet_equips 0 行 + 实例 status=1 + Unequipped=true）+ 重复 Unequip 同空槽→5004（V1 §8.4 行 1651 "已空槽必 5004" 非幂等不变量）；深度回滚 / 100 并发串行化压测 / 状态一致性矩阵按范围红线归 Story 26.5（未做）。
- 无新增文件（全部追加到既有文件）；无改 docs/*.md / V1 §8.4 契约 / 0001-0016 migration。

### File List

实装（改既有文件，4 个）：
- `server/internal/repo/mysql/user_pet_equip_repo.go`（+2 方法 interface 签名 + impl）
- `server/internal/service/cosmetic_equip_service.go`（+`UnequipParams`/`UnequipResult` DTO + interface `Unequip` 方法 + `validUnequipSlot` helper + `Unequip`/`runUnequipTx` impl）
- `server/internal/app/http/handler/cosmetics_handler.go`（+`unequipRequest` struct + `isValidSlot` helper + `Unequip` handler + `unequipResponseDTO` + 顶部 doc 注释更新）
- `server/internal/app/bootstrap/router.go`（+1 路由行 `authedGroup.POST("/cosmetics/unequip", ...)` + Story 26.4 注释）

测试（改既有文件，3 个）：
- `server/internal/repo/mysql/user_pet_equip_repo_test.go`（+6 unequip repo case）
- `server/internal/service/cosmetic_equip_service_test.go`（扩 `stubUserPetEquipRepo` 2 hook + 10 Unequip service case）
- `server/internal/service/cosmetic_equip_service_integration_test.go`（+`apperror` import + `TestCosmeticEquipServiceIntegration_UnequipHappyPath`）
- `server/internal/app/http/handler/cosmetics_handler_test.go`（扩 `stubCosmeticEquipService` + `buildCosmeticsUnequipHandlerRouter` + 8 Unequip handler case）

流程文件：
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（26-4 状态 ready-for-dev → in-progress → review）
- `_bmad-output/implementation-artifacts/26-4-post-cosmetics-unequip-事务.md`（Tasks 勾选 + Dev Agent Record + Status）

### Change Log

| 日期 | 变更 | 说明 |
|---|---|---|
| 2026-05-17 | Story 26.4 创建 | bmad-create-story 自动从 sprint-status.yaml 发现 epic-26 首个 backlog story；状态 backlog → ready-for-dev；epic-26 已 in-progress 不变 |
| 2026-05-17 | Story 26.4 实装完成 | bmad-dev-story 红绿循环：repo +2 unequip 专用方法 / service +Unequip 单事务（FOR UPDATE + RowsAffected==0 兜底）/ handler +Unequip + slot 枚举校验 / router +1 路由；24 单测全 pass + 集成测试编译注册通过；`bash scripts/build.sh --test` BUILD SUCCESS；状态 in-progress → review |
