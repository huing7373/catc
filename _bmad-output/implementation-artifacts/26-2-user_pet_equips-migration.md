# Story 26.2: user_pet_equips migration（首次落地 0016_init_user_pet_equips.up/down.sql + UserPetEquip GORM domain struct 最小骨架 + ≥3 case 单元/集成测试 + dockertest 双 UNIQUE 约束拒绝验证 + 对账 migrate 集成测试 expectedTables 13→14 张 / StatusAfterUp v15→v16）

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a 服务端开发,
I want **首次落地** `server/migrations/0016_init_user_pet_equips.up.sql` + `server/migrations/0016_init_user_pet_equips.down.sql` 两个新 migration 文件（严格按 `docs/宠物互动App_数据库设计.md` §5.10 行 537-550 钦定的 CREATE TABLE DDL：`id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT` + `user_id BIGINT UNSIGNED NOT NULL` + `pet_id BIGINT UNSIGNED NOT NULL` + `slot TINYINT NOT NULL` + `user_cosmetic_item_id BIGINT UNSIGNED NOT NULL` + `created_at / updated_at DATETIME(3)` + **`UNIQUE KEY uk_pet_slot (pet_id, slot)`** + **`UNIQUE KEY uk_user_cosmetic_item_id (user_cosmetic_item_id)`** + `KEY idx_user_pet (user_id, pet_id)` + `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`，1:1 对齐 §5.10；**migration 序号 = `0016` 不是 epics.md §Story 26.2 行 3526 文字写的 `0015`** —— `0015` 已被 Story 23.2 落地的 `0015_init_user_cosmetic_items.up/down.sql` 占用，本 story 取下一个空闲序号 `0016`，与 ADR-0003 顺序递增编号约定 + Story 23.2 同类序号纠偏先例一致）+ **新增** `server/internal/repo/mysql/user_pet_equip_repo.go` 含 `UserPetEquip` GORM domain struct（与 0016 真实 schema 1:1 对齐：`ID / UserID / PetID / Slot / UserCosmeticItemID / CreatedAt / UpdatedAt`，全列 NOT NULL → **全部值类型**，无 NULL 可空列故**无指针字段**）+ `TableName() string` 显式返回 `"user_pet_equips"`（**仅** struct + TableName，**不**新增 `UserPetEquipRepo` interface / 实装任何 List / Insert / Delete / FindByPetSlot 方法，YAGNI；Story 26.3 落地 equip 事务的 INSERT / DELETE / 同槽换装方法 / Story 26.4 落地 unequip 事务 DELETE 方法 / Story 26.6 落地 GET /home pet.equips JOIN 查询方法）+ **扩展** `server/internal/infra/migrate/migrate_integration_test.go` 把 `TestMigrateIntegration_UpThenDown` 的 `expectedTables`（当前 13 张，停在 0015 user_cosmetic_items）+ `TestMigrateIntegration_UpTwice_Idempotent` 的 `tableCount != 13` 断言 + `TestMigrateIntegration_StatusAfterUp` 的 `v != 15` 断言 + 文件顶部 `version=15` 注释一次性对账到本 story 落地后的正确值（`expectedTables` 追加 `user_pet_equips` → 14 张；`tableCount != 13` → `!= 14`；`v != 15` → `v != 16`；文件顶部注释 `version=15` → `version=16`，并补 Story 26.2 一条扩展记录）+ **新增** `TestMigrateIntegration_UserPetEquips_Schema`（按 §5.10 用 `INFORMATION_SCHEMA.COLUMNS` / `STATISTICS` 做字段层 + 索引层断言，含 7 列字段计数兜底防漂移、全列 IS_NULLABLE=NO 断言、`uk_pet_slot (pet_id, slot)` + `uk_user_cosmetic_item_id (user_cosmetic_item_id)` 两个 UNIQUE 索引的 non_unique=0 + 列顺序断言、`idx_user_pet (user_id, pet_id)` 普通索引列顺序断言，与 Story 23.2 落地的 `TestMigrateIntegration_UserCosmeticItems_Schema` 同模式）+ **新增** `TestMigrateIntegration_UserPetEquips_UniqueConstraints_Rejected` dockertest 集成测试（覆盖**两个** UNIQUE 约束的数据库层拒绝：同 `(pet_id, slot)` 第二行插入被 `uk_pet_slot` 拒 + 同 `user_cosmetic_item_id` 第二行插入被 `uk_user_cosmetic_item_id` 拒，与 Story 11.2 落地的 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` 行 684-745 同模式 —— 后者验证单 + 复合 UNIQUE 拒绝，本表验证两个独立 UNIQUE（一复合 `uk_pet_slot` + 一单列 `uk_user_cosmetic_item_id`）各自被 DB 拒绝插入），

so that **Story 26.3（POST /cosmetics/equip 事务，含同槽换装：查实例归属/状态 → 查 cosmetic_items.slot → 同槽已有装备则删旧 user_pet_equips 行 + 旧实例 status 回 1 → INSERT user_pet_equips → 当前实例 status 改 2）+ Story 26.4（POST /cosmetics/unequip 事务：查 user_pet_equips WHERE pet_id=? AND slot=? → DELETE 行 + 实例 status 回 1）+ Story 26.5（Layer 2 集成测试 - 穿戴事务全流程：含"并发 1：同 pet 同 slot 100 个并发 equip 不同实例 → 只 1 成功，DB UNIQUE(pet_id,slot) 兜底" + "并发 2：同实例并发 equip → 只 1 成功，DB UNIQUE(user_cosmetic_item_id) 兜底" + 回滚一致性 NFR2）+ Story 26.6（GET /home 扩展 pet.equips 真实数据：读 user_pet_equips JOIN cosmetic_items + user_cosmetic_items）+ iOS Epic 27（激活仓库穿戴按钮 + EquipUseCase / UnequipUseCase + 文字降级渲染，间接依赖本表落地后 §5.1 pet.equips / §8.3 equipped 的 DB 端真相源）**可以基于一个**已落地、已具备完整测试覆盖、已通过 dockertest 双 UNIQUE 约束真实拒绝验证、已具备完整 GORM domain struct 字段映射**的 user_pet_equips 持久化基础并行展开，不再出现"26.3 写 equip 事务 INSERT user_pet_equips 时找不到表 / 26.3 同槽换装查 `WHERE pet_id=? AND slot=?` 走不到 `uk_pet_slot` 索引导致并发 race / 26.4 unequip DELETE 查不到行的语义与 5004 错误码对不上 / 26.5 并发兜底测试断言 `UNIQUE(pet_id,slot)` / `UNIQUE(user_cosmetic_item_id)` 时约束名拼错或 schema 漂移 / 26.6 GET /home JOIN user_pet_equips 时索引 `idx_user_pet` 缺失退化 / GORM struct 把 `slot` 映射成 uint8 而非 int8 与 cosmetic_items.slot TINYINT 跨表类型不一致 / 节点 9 实装才发现 migrate 集成测试 expectedTables 早在 0015 就停在 13 张又一次积压 / 序号写成 0015 与 Story 23.2 的 user_cosmetic_items 撞号"的返工。

## 故事定位（Epic 26 第二条 = 第一条**实装** story；上承 26.1 契约定稿（§8.3 / §8.4 / §1 节点 9 冻结），下启 26.3 equip 事务 + 26.4 unequip 事务 + 26.5 Layer 2 集成测试 + 26.6 GET /home 扩展）

- **Epic 26 进度**：26.1（契约定稿 §8.3 POST /cosmetics/equip + §8.4 POST /cosmetics/unequip + §1 节点 9 冻结，**done**）→ **26.2（本 story，user_pet_equips migration + UserPetEquip GORM domain struct + 测试覆盖 + 对账 0015 后 migrate 集成测试积压断言）** → 26.3（POST /cosmetics/equip 事务，含同槽换装）→ 26.4（POST /cosmetics/unequip 事务）→ 26.5（Layer 2 集成测试 - 穿戴事务全流程）→ 26.6（GET /home 扩展 - pet.equips 真实数据）。
- **本 story 是 26.3 / 26.4 / 26.5 / 26.6 / iOS Epic 27 的强前置**（本 story done 后才能开工）：
  - **Story 26.3 POST /cosmetics/equip 事务**：service 事务流程（26.1 §8.3 服务端逻辑步骤 8-9 钦定）INSERT `user_pet_equips (user_id, pet_id, slot, user_cosmetic_item_id)` + 同槽换装时先 `DELETE` 旧 user_pet_equips 行，全部命中本 story 落地的 0016 表 schema；同槽查询 `SELECT ... FROM user_pet_equips WHERE pet_id=? AND slot=?` 必须走本 story 落地的 `uk_pet_slot (pet_id, slot)` 索引；`uk_user_cosmetic_item_id` UNIQUE 是 26.3 "一件实例同时只能装备一次"的 DB 兜底（26.1 §8.3 关键约束段已锚定引用）。本 story **不**预实装任何 equip 事务逻辑 / Repo 方法
  - **Story 26.4 POST /cosmetics/unequip 事务**：service 流程（26.1 §8.4 服务端逻辑钦定）查 `user_pet_equips WHERE pet_id=? AND slot=?` → 不存在返 5004 → 拿 `user_cosmetic_item_id` → `DELETE` 行，全部命中本 story 落地的表 schema + `uk_pet_slot` 索引
  - **Story 26.5 Layer 2 集成测试 - 穿戴事务全流程**：epics.md §Story 26.5 AC 钦定"并发 1：同一 pet 同一 slot 100 个并发 equip 不同实例 → 只 1 个成功，其他 99 个返回错误（DB UNIQUE(pet_id, slot) 兜底）" + "并发 2：同一实例 100 个并发 equip → 只 1 个成功（DB UNIQUE(user_cosmetic_item_id) 兜底）" + "状态一致性：所有 status=2 实例必然在 user_pet_equips 有对应行（NFR2）" —— 这两个并发兜底测试的契约依据 = 本 story 落地的两个 UNIQUE 约束；本 story 落地 schema + 提供本 story 的 `UniqueConstraints_Rejected` migrate 集成测试作为 26.5 service 层并发测试的 schema-correctness 前置。本 story **不**实装 26.5 service 层并发测试（属 Story 26.5 实装层）
  - **Story 26.6 GET /home 扩展 - pet.equips 真实数据**：epics.md §Story 26.6 AC 钦定"pet.equips 字段从写死 `[]` 改为读 user_pet_equips JOIN cosmetic_items + user_cosmetic_items" → 单 SQL JOIN 必须命中本 story 落地的 `idx_user_pet (user_id, pet_id)` 索引避免 N+1 退化（epics.md §Story 26.6 AC "大量装备并发查 → 单 SQL JOIN 不退化"）
  - **iOS Epic 27（激活仓库穿戴按钮 + EquipUseCase / UnequipUseCase + 文字降级渲染）**：iOS 端通过 26.3 §8.3 equip response `equipped.{slot, userCosmeticItemId, cosmeticItemId, name}` + 26.6 §5.1 GET /home `pet.equips[]` JSON response 间接依赖本 story 落地的 DB schema（`user_pet_equips.slot` → §8.3 `equipped.slot` 枚举 §6.8 / `user_pet_equips.user_cosmetic_item_id` → §8.3 `equipped.userCosmeticItemId` 字符串化）
- **epics.md §Story 26.2 钦定**（行 3516-3535）：
  - `migrations/0015_init_user_pet_equips.sql` 按 数据库设计.md §5.10 创建表（**注：序号纠偏**——epics.md 文字写 `0015`，但 `0015` 已被 **Story 23.2** 落地的 `0015_init_user_cosmetic_items.up/down.sql` 占用，本 story 实际用 `0016`；序号偏移属"epics.md 撰写时点早于 23.2 落地"的历史时序错位，**不**视为契约/范围变更，按 ADR-0003 顺序递增取下一空闲号 + 与 Story 23.2 自身的 `0014→0015` 同类纠偏先例一致；表名 / 字段 / 索引钦定全部不变）
  - 含**关键约束**：`UNIQUE KEY uk_pet_slot (pet_id, slot)`（一个宠物同一部位只能穿一件）+ `UNIQUE KEY uk_user_cosmetic_item_id (user_cosmetic_item_id)`（一件实例同时只能装备一次）+ `KEY idx_user_pet (user_id, pet_id)`
  - 含 down.sql
  - **单元测试覆盖**（≥3 case）：happy（migrate up 后表存在 + 全部约束都符合 §5.10）/ happy（migrate down 后表删除，由现有 `TestMigrateIntegration_UpThenDown` 扩展覆盖）/ edge（重复 migrate up → 幂等，由现有 `TestMigrateIntegration_UpTwice_Idempotent` 扩展覆盖）
  - **集成测试覆盖**（dockertest）：migrate up → 故意尝试违反两个 UNIQUE 约束 → 数据库拒绝插入
- **Story 26.1 上游冻结边界**（V1 §8.3 equip 字段表 / §8.4 unequip 字段表 / §1 节点 9 冻结声明 + 数据库设计 §5.10 user_pet_equips DDL + §6.8 slot 枚举 + §8.4 穿戴事务）：本 story 落地的字段（`slot TINYINT` / `user_cosmetic_item_id BIGINT UNSIGNED` / `pet_id BIGINT UNSIGNED` / `id BIGINT UNSIGNED`）是 26.1 §8.3 / §8.4 锚定的 API 字段语义（`equipped.slot` 枚举 §6.8 = `user_pet_equips.slot` / `equipped.userCosmeticItemId` BIGINT 字符串化 = `user_pet_equips.user_cosmetic_item_id` / `petId` = `user_pet_equips.pet_id`）的 **DB 端真相源**；本 story **不**反向修改 DB schema（DB → API 单向），仅严格对齐数据库设计文档 §5.10 DDL
- **范围红线**：
  - 本 story **只**改 `server/migrations/0016_init_user_pet_equips.up.sql`（新建）+ `server/migrations/0016_init_user_pet_equips.down.sql`（新建）+ `server/internal/repo/mysql/user_pet_equip_repo.go`（新建，含 `UserPetEquip` struct + `TableName()`，仅最小骨架）+ `server/internal/infra/migrate/migrate_integration_test.go`（扩展：`UpThenDown` 的 `expectedTables` 追加 `user_pet_equips` → 14 张 + `UpTwice_Idempotent` 的 `tableCount != 13` → `!= 14` + `StatusAfterUp` 的 `v != 15` → `v != 16` + 文件顶部 `version=15` 注释 → `version=16` + 补 Story 26.2 扩展记录 + 新增 `TestMigrateIntegration_UserPetEquips_Schema` + 新增 `TestMigrateIntegration_UserPetEquips_UniqueConstraints_Rejected`）+ 本 story 文件 + sprint-status.yaml 流转
  - **不**实装任何 service / handler / repo write / read 方法（26.3 / 26.4 / 26.6 才做；本 story 阶段**仅**落地 GORM struct + TableName，**不**新建 `UserPetEquipRepo` interface / 实装 `InsertInTx` / `DeleteByPetSlotInTx` / `FindByPetSlot` / `ListByUserPetForHome` 等方法）
  - **不**实装任何 INSERT / seed SQL（user_pet_equips 是运行时穿戴关系表，**无** seed 阶段；Story 26.3 equip 事务才首条 INSERT）
  - **不**实装任何 equip / unequip 事务 / 同槽换装逻辑（Story 26.3 / 26.4 钦定范围；本 story **仅** CREATE TABLE，不触碰任何已有 service / 事务 / Story 23.5 开箱事务）
  - **不**改 V1 接口契约（26.1 已冻结 §8.3 / §8.4 / §1 节点 9 声明）
  - **不**改任何 `docs/宠物互动App_*.md`（§5.10 是契约**输入**，本 story 严格对齐它但**不修改**；如发现 §5.10 与本 story 落地的 DDL 有不一致 → 优先以 §5.10 为准修改本 story 而非反向改 §5.10）
  - **不**改 `_bmad-output/` 下其他 yaml / md（除自己 story 文件 + sprint-status.yaml 流转）
  - **不**改 ADR-0003（migration 工具 + 编号约定 + .up.sql / .down.sql 双向规范由 Story 4.3 已锚定，本 story 沿用 / 不修改）
  - **不**改 `cmd/server/main.go` migrate 子命令（已在 Story 4.3 落地）
  - **不**为 0016 写"prod 部署 migration 自动化 / 一键 rollback / dry-run / force"等运维化改造（保留最小集 up/down，与 Story 4.3 / 7.2 / 10.3 / 11.2 / 17.2 / 20.2 / 20.4 / 23.2 一致）
  - **不**建 FK 约束（与本设计其他表一致 —— ADR-0003 / 数据库设计 §3 + §7 钦定"应用层校验 + 索引兜底"策略；`user_id` / `pet_id` / `user_cosmetic_item_id` 语义上 reference 其他表但**不**建 FK）
  - **不**修改 0001 ~ 0015 既有 migration 文件（已在 Story 4.3 / 7.2 / 10.3 / 11.2 / 17.2 / 17.3 / 20.2 / 20.3 / 20.4 / 20.6 / 23.2 落地）

**本 story 不做**（明确范围红线）：

- 不实装任何 Go handler / service / repo write 或 read 方法（26.3 / 26.4 / 26.6 范围）
- 不实装任何 INSERT / seed SQL（user_pet_equips 是运行时穿戴关系表，无 seed 阶段；26.3 equip 事务首条 INSERT）
- 不新建 `UserPetEquipRepo` interface（YAGNI；26.3 落地 equip 事务写方法（`InsertInTx` + 同槽换装 `DeleteByPetSlotInTx` + `FindByPetSlot`）/ 26.4 落地 unequip `DeleteByPetSlotInTx` / 26.6 落地 GET /home `ListByUserPetForHome` JOIN 查询时才落地 `UserPetEquipRepo` 类型 + 方法 —— 对标 Story 23.2 阶段 `user_cosmetic_item_repo.go` 仅 struct+TableName 最小集，interface 是 Story 23.4 后续扩展加的，**不**是 23.2 阶段产物）
- 不在 `UserPetEquip` struct 上加 GORM `uniqueIndex` / `index` 等 tag（UNIQUE / 普通索引由 SQL DDL 定义，GORM struct **不**重复声明 —— 与 ADR-0003 §3.2 "禁止 GORM AutoMigrate" 同源；GORM struct 仅为 Find / Create 提供字段映射，**不**作为 schema 真相源；与 Story 23.2 落地的 `UserCosmeticItem` / 20.2 落地的 `CosmeticItem` / 11.2 落地的 `RoomMember` struct 同模式）
- 不引入 `gorm.Model`（避免引入 `DeletedAt` / 软删除字段，与 0016 真实 schema 不符；user_pet_equips 用 DELETE 行表达卸下，**不**软删除）
- 不在 struct 中预留任何 §5.10 之外的字段（即使顺手加占位字段也会让 0016 SQL 与 §5.10 不一致 → 跨文档漂移）
- 不为 `slot` 字段加任何 NULL 可空列指针映射（user_pet_equips **全列 NOT NULL**，与 Story 23.2 user_cosmetic_items 有 `source_ref_id` / `consumed_at` 可空列不同 —— 本表无任何指针字段，全部值类型）
- 不修改 0001 ~ 0015 既有 migration 文件（已落地）
- 不修改 Story 23.2 user_cosmetic_items / 0015 migration / Story 23.5 开箱补入仓事务 / Story 20.6 开箱事务（穿戴事务对 user_cosmetic_items.status 的 1↔2 推进是 Story 26.3 / 26.4 钦定范围；本 story **仅** CREATE user_pet_equips 表，不触碰 user_cosmetic_items 任何 schema / 数据）
- 不修改 V1 接口契约（26.1 已冻结 §8.3 / §8.4 / §1 节点 9 声明）
- 不修改数据库设计 §5.10 user_pet_equips / §6.8 slot 枚举 / §8.4 穿戴事务（schema 输入，本 story 严格对齐不修改）
- 不修改 Go 项目结构文档 §6（`internal/repo/mysql/` 目录已锚定；本 story 新增 `user_pet_equip_repo.go` 沿用既有目录规则）
- 不修改 ADR-0003 / ADR-0007（migration 编号约定 / ctx 传播规范由 Story 4.3 / Story 1.9 已锚定）
- 不写英文版测试注释 / 文档（项目 communication_language=Chinese；与 Story 4.3 / 7.2 / 10.3 / 11.2 / 17.2 / 20.2 / 20.4 / 23.2 一致）
- 不为 0016 写 stress test / fuzz test（节点 9 阶段 schema 稳定 + 单测 + dockertest 集成测试已覆盖核心约束；并发兜底 stress 由 Story 26.5 service 层 100 并发测试覆盖，**不**在本 migration story）
- 不在本 story 内对 Story 26.3 / 26.4 / 26.6 实装做"提前预实装"（即使顺手写 `(r *userPetEquipRepo) InsertInTx(...)` / `DeleteByPetSlot(...)` 也禁止；这些方法是下游钦定范围，提前 ship 会让下游评审找不到"新增方法"的明确范围边界，与 Story 23.2 / 20.2 / 11.2 "禁止预实装" 同模式）
- 不写 `UserPetEquip.Slot` 字段的 enum 校验（DB 端 `TINYINT NOT NULL` 已兜底；§6.8 枚举 `{1,2,3,4,5,6,7,99}` 钦定值域；service 层校验由 26.3 实装时按需添加 —— equip 时 slot 来自 cosmetic_items 配置非客户端传入，本表只持久化）
- 不在本 story 修复 0015 user_cosmetic_items 的任何 schema / 测试问题（仅**对账**积压的 `expectedTables` / 版本号断言把 user_pet_equips 加进去；如发现 0015 schema 本身有问题 → 记 tech debt 不在本 story 修）

## Acceptance Criteria

**AC1 — 0016_init_user_pet_equips.up.sql 新建（与 §5.10 钦定 1:1 对齐）**

新建 `server/migrations/0016_init_user_pet_equips.up.sql`，内容必须**严格**对齐 `docs/宠物互动App_数据库设计.md` §5.10（行 537-550）钦定的 DDL（含中文注释头说明字段语义来源 + 范围红线，参照 Story 23.2 落地的 `0015_init_user_cosmetic_items.up.sql` / Story 11.2 落地的 `0008_init_room_members.up.sql`（有 UNIQUE 约束的同类参照）注释头风格）：

```sql
CREATE TABLE user_pet_equips (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    pet_id BIGINT UNSIGNED NOT NULL,
    slot TINYINT NOT NULL,
    user_cosmetic_item_id BIGINT UNSIGNED NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_pet_slot (pet_id, slot),
    UNIQUE KEY uk_user_cosmetic_item_id (user_cosmetic_item_id),
    KEY idx_user_pet (user_id, pet_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

- DDL **逐字段、逐索引、ENGINE / CHARSET 全部与 §5.10 行 538-550 一致**；字段顺序与 §5.10 一致。
- **全列 NOT NULL**（user_pet_equips 无任何可空列 —— 与 Story 23.2 user_cosmetic_items 有 `source_ref_id` / `consumed_at` 两个 NULL 列不同；本表所有字段 NOT NULL，DDL 不出现任何 `NULL` 可空声明）。
- `slot TINYINT NOT NULL`（§6.8 枚举 `{1=hat / 2=gloves / 3=glasses / 4=neck / 5=back / 6=body / 7=tail / 99=other}`；**无 DEFAULT**——slot 由 equip 时 cosmetic_items 配置决定必传，非客户端传入，DDL 不做 enum 约束、不设 DEFAULT，由 service 层 + §6.8 钦定值域兜底，与 §5.10 行 542 钦定一致）。
- **两个 UNIQUE 约束**（**关键约束**，epics.md §Story 26.2 行 3527-3528 + §5.10 行 547-548 钦定）：
  - `UNIQUE KEY uk_pet_slot (pet_id, slot)`（复合 UNIQUE，一个宠物同一部位只能穿一件 —— 26.3 同槽换装的 DB 兜底 + 26.5 并发 1 测试的契约依据）。
  - `UNIQUE KEY uk_user_cosmetic_item_id (user_cosmetic_item_id)`（单列 UNIQUE，一件实例同时只能装备一次 —— NFR11 + 26.5 并发 2 测试的契约依据）。
- `KEY idx_user_pet (user_id, pet_id)` 普通索引（覆盖 26.6 GET /home `WHERE user_id=? AND pet_id=?` JOIN 路径，避免 N+1 退化）。
- **不**建 FK 约束（`user_id` ref users.id / `pet_id` ref pets.id / `user_cosmetic_item_id` ref user_cosmetic_items.id —— 全部语义 reference，**不**建 FK，与本设计其他表一致 ADR-0003 / §3 + §7）。
- 注释头说明：本 migration 由 Story 26.2 首次落地（Epic 26 节点 9 穿戴 / 卸下事务一致性约束 schema 根基 owner）+ 字段 1:1 对齐 §5.10 + 两个 UNIQUE 约束语义说明（`uk_pet_slot` = 一槽位一件兜并发同槽 equip / `uk_user_cosmetic_item_id` = 一实例只装备一次兜同实例并发 equip，NFR11）+ 普通索引 `idx_user_pet` 覆盖路径说明（GET /home pet.equips JOIN，Story 26.6）+ 范围红线（仅 CREATE TABLE，无 seed，无 service，序号 0016 纠偏说明 —— 因 0015 被 Story 23.2 user_cosmetic_items 占用）。
- 文件编码 UTF-8 + LF 行尾（与其他 migration 一致）。

**AC2 — 0016_init_user_pet_equips.down.sql 新建（DROP TABLE 回滚路径）**

新建 `server/migrations/0016_init_user_pet_equips.down.sql`：

```sql
DROP TABLE IF EXISTS user_pet_equips;
```

- 与 Story 23.2 落地的 `0015_init_user_cosmetic_items.down.sql` / Story 11.2 落地的 `0008_init_room_members.down.sql` 同模式（含简短中文注释头说明回滚目标 + 由 Story 26.2 首次落地）。
- `DROP TABLE IF EXISTS`（幂等回滚，与既有 down migration 一致）。

**AC3 — UserPetEquip GORM domain struct 新建（与 0016 真实 schema 1:1 对齐，全列 NOT NULL → 全值类型无指针）**

新建 `server/internal/repo/mysql/user_pet_equip_repo.go`：

- 含 `UserPetEquip` struct，字段与 0016 真实 schema + §5.10 1:1 对齐（**全列 NOT NULL → 全部值类型，无任何指针字段**）：
  - `ID uint64` `gorm:"column:id;primaryKey;autoIncrement"`
  - `UserID uint64` `gorm:"column:user_id;not null"`
  - `PetID uint64` `gorm:"column:pet_id;not null"`
  - `Slot int8` `gorm:"column:slot;not null"`（TINYINT → int8，与 `CosmeticItem.Slot` 跨表同类型对齐，§6.8 枚举值域 `{1..7,99}`）
  - `UserCosmeticItemID uint64` `gorm:"column:user_cosmetic_item_id;not null"`
  - `CreatedAt time.Time` `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
  - `UpdatedAt time.Time` `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP(3)"`
- 含 `func (UserPetEquip) TableName() string { return "user_pet_equips" }`。
- **仅** struct + TableName；**不**新建 `UserPetEquipRepo` interface / 实装任何 List / Insert / Delete / FindByPetSlot 方法（与 Story 23.2 落地 `user_cosmetic_item_repo.go` 在 23.2 阶段仅 struct+TableName 同模式 —— 注意 `user_cosmetic_item_repo.go` 现含 `UserCosmeticItemRepo` interface + `ListByUserForInventory` / `CreateInTx` 是 Story 23.4 / 23.5 后续扩展加的，本 story 阶段对应 23.2 阶段的最小集，**不**提前加 interface / 方法）。
- **无任何指针字段**（user_pet_equips 全列 NOT NULL，与 Story 23.2 `UserCosmeticItem` 有 `SourceRefID *uint64` / `ConsumedAt *time.Time` 指针映射 NULL 列不同 —— 本 struct 不出现任何 `*uint64` / `*time.Time`，所有字段值类型）。
- 含完整中文注释头（字段说明含 §5.10 + §6.8 slot 枚举 + 两个 UNIQUE 约束语义 + 全列 NOT NULL 无指针映射说明 + UNIQUE / 索引由 SQL DDL 定义不在 struct tag 重复声明的说明 + 范围红线，参照 `user_cosmetic_item_repo.go` 行 12-61 注释头风格但去掉 NULL 列指针段落改为"全列 NOT NULL"说明）。

**AC4 — migrate 集成测试扩展 + 对账 0015 后的表/版本断言到 14 张 / v16**

修改 `server/internal/infra/migrate/migrate_integration_test.go`：

1. **`TestMigrateIntegration_UpThenDown`**：`expectedTables` slice 当前为 13 张（停在 0015 user_cosmetic_items，Story 23.2 已对账过 0014/0015）→ 追加 `"user_pet_equips"` → 14 张；表数量注释 `13 张` → `14 张`（注释补 Story 26.2 一条扩展记录：加 0016 user_pet_equips，Epic 26 节点 9 穿戴关系表 + 含双 UNIQUE）。
2. **`TestMigrateIntegration_UpTwice_Idempotent`**：该 case 现含 `tableCount != 13` 断言（行 386）+ `IN ('users', ..., 'user_cosmetic_items')` 列表（行 382）→ 列表追加 `'user_pet_equips'` + `tableCount != 13` → `!= 14` + `t.Errorf` want 文案 13 → 14 + 补 Story 26.2 扩展记录。
3. **`TestMigrateIntegration_StatusAfterUp`**：版本号断言 `if v != 15`（行 647）→ `if v != 16` + `t.Errorf` want 文案同步 15 → 16 + 函数头注释（行 599-616 区域）补一条扩展记录：`Story 26.2 扩展：从 15 改 16（多了 0016_init_user_pet_equips；Epic 26 节点 9 穿戴关系表，含 uk_pet_slot + uk_user_cosmetic_item_id 双 UNIQUE）`；同步修正文件顶部行 8 `// 4. edge: Up 后 Status 返回 (version=15, dirty=false, nil)` 注释为 `version=16`，并在文件顶部注释块（行 52-60 Story 23.2 扩展段之后）追加一段 Story 26.2 扩展说明（含序号纠偏 0015→0016 因 0015 被 Story 23.2 占用）。
4. **新增 `TestMigrateIntegration_UserPetEquips_Schema`**：按 §5.10 用 `INFORMATION_SCHEMA.COLUMNS` / `STATISTICS` 做字段层 + 索引层断言（与 Story 23.2 落地的 `TestMigrateIntegration_UserCosmeticItems_Schema` 行 2354-2388+ 同模式）：
   - `INFORMATION_SCHEMA.TABLES`：user_pet_equips 表存在。
   - 7 列字段计数兜底（防漂移：如有人误加字段会让计数变 8 失败）。
   - 逐列断言 column_type / is_nullable / column_default：`id BIGINT UNSIGNED PK AUTO_INCREMENT` / `user_id BIGINT UNSIGNED NOT NULL` / `pet_id BIGINT UNSIGNED NOT NULL` / `slot TINYINT NOT NULL`（**无 DEFAULT 断言**——§5.10 钦定 slot 无 DEFAULT）/ `user_cosmetic_item_id BIGINT UNSIGNED NOT NULL` / `created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)` / `updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE`。**全列 IS_NULLABLE = NO 断言**（与 user_cosmetic_items 有 source_ref_id/consumed_at IS_NULLABLE=YES 形成对照）。
   - **两个 UNIQUE 索引断言**（`STATISTICS`，关键覆盖点）：`uk_pet_slot` non_unique=0 + 列顺序 `(pet_id, slot)`（SEQ_IN_INDEX 1=pet_id / 2=slot）+ `uk_user_cosmetic_item_id` non_unique=0 + 单列 `(user_cosmetic_item_id)`。
   - **普通索引断言**：`idx_user_pet` non_unique=1 + 列顺序 `(user_id, pet_id)`（SEQ_IN_INDEX 1=user_id / 2=pet_id）。
   - **UNIQUE 数量正向断言**：查 `STATISTICS WHERE table_name='user_pet_equips' AND non_unique=0 AND index_name != 'PRIMARY'` 的 distinct index_name count = 2（验证恰 2 个 UNIQUE：uk_pet_slot + uk_user_cosmetic_item_id，与 user_cosmetic_items 无 UNIQUE / cosmetic_items 仅 1 个 uk_code 形成对照）。
5. **新增 `TestMigrateIntegration_UserPetEquips_UniqueConstraints_Rejected`**：dockertest 运行时双 UNIQUE 拒绝验证（epics.md §Story 26.2 行 3535 钦定"故意尝试违反两个 UNIQUE 约束 → 数据库拒绝插入"；与 Story 11.2 落地的 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` 行 684-745 同模式）：
   - migrate Up → 首条 INSERT `user_pet_equips (user_id, pet_id, slot, user_cosmetic_item_id) VALUES (1, 100, 1, 1000)` 必须成功。
   - **uk_pet_slot 拒绝**：再 INSERT `(1, 100, 1, 1001)`（同 pet_id=100 + 同 slot=1，**不同** user_cosmetic_item_id=1001）→ 必须被 `uk_pet_slot` 拒（err 非 nil + 含 `Duplicate entry` 关键字 —— 验证一个宠物同一槽位只能穿一件，26.3 同槽换装 DB 兜底 / 26.5 并发 1 契约依据）。
   - **uk_user_cosmetic_item_id 拒绝**：再 INSERT `(1, 200, 2, 1000)`（**不同** pet_id=200 + 不同 slot=2，但**相同** user_cosmetic_item_id=1000）→ 必须被 `uk_user_cosmetic_item_id` 拒（err 非 nil + 含 `Duplicate entry` 关键字 —— 验证一件实例同时只能装备一次，NFR11 / 26.5 并发 2 契约依据）。
   - 兜底 SELECT COUNT(*) = 1（证实只有首条成功，两次违反 UNIQUE 的 INSERT 都被 DB 拒绝未落库）。

**AC5 — 构建 / 测试通过**

- `bash scripts/build.sh --test` 通过（vet + build + 全量单测 `go test -count=1 ./...`；本 story 改了 Go 文件（新增 repo struct + 改集成测试文件），必须跑）。
- `bash scripts/build.sh --integration` 通过（`-tags=integration` 跑 migrate 集成测试，含本 story 新增 2 个 case + 对账后的 `UpThenDown` / `UpTwice_Idempotent` / `StatusAfterUp`；dockertest 起真实 MySQL 容器）。注：若 `./...` 全量集成跑因不相关 `internal/service` dockertest 容器冷启动 timing flake 挂掉（Story 23.2 Debug Log 已记录该已知环境侧 flake），用 `go test -tags=integration -timeout=600s ./internal/infra/migrate/` 隔离验证本 story 范围 5 个 migrate case 全 PASS + 抽查相邻既有 case 无回归即可，并在 Debug Log 如实记录。
- 全量测试无回归（既有 migrate 集成测试 case 全绿；0001~0015 既有 migration 不受影响；其他 repo / service 测试不受影响 —— 本 story 仅新增 0016 + 新增 struct + 改 migrate 集成测试断言，无既有逻辑改动）。
- `git status` 仅出现范围红线内文件改动（4 个 server 文件 + story 文件 + sprint-status.yaml）。

**AC6 — 跨文档一致性自检（migration story 必须项）**

完成 AC1~AC5 后，必须逐项核对并在本 story "Completion Notes List" 记录核对结论：

1. 0016 DDL 逐字段 / 逐索引 / ENGINE / CHARSET 与 `docs/宠物互动App_数据库设计.md` §5.10 行 538-550 **逐字符比对一致**（字段名 / 类型 / NOT NULL（全列）/ 无 DEFAULT 的 slot / 两个 UNIQUE 约束名与列顺序 / `idx_user_pet` 列顺序 / ENGINE / CHARSET）。
2. `UserPetEquip` struct 字段与 0016 真实 schema **逐字段一致**（列名映射 / **全值类型无指针**（全列 NOT NULL）/ int8 映射 `slot` TINYINT 且与 `CosmeticItem.Slot` 跨表同类型 / uint64 映射 BIGINT UNSIGNED）。
3. `slot` 无 DEFAULT 与 §5.10 行 542 + §6.8 枚举钦定语义 **无矛盾**（slot 由 equip 时 cosmetic_items 配置决定必传，非客户端，DDL 不设 DEFAULT 正确）。
4. migrate 集成测试 `expectedTables`（14 张）+ `UpTwice_Idempotent` 表数量（14）+ `StatusAfterUp` 版本号（16）与真实 migration 文件序号集合（0001~0016，其中 0010 / 0012 是 seed migration 不建表 → 实际建表数 14）**一致**；确认 Story 23.2 已对账过 0014/0015 → 本 story 仅在 13 基础上 +1（user_pet_equips）→ 14，**无**新增积压（与 Story 23.2 当时需补 0014 漏更新积压不同，本 story 起点干净）。
5. 未触碰 0001~0015 既有 migration / Story 23.2 user_cosmetic_items / Story 23.5 / 20.6 开箱事务 / V1 接口契约 / 数据库设计 §5.10 / 其他 6 份 docs。
6. 序号纠偏（epics.md §Story 26.2 文字 `0015` → 实际 `0016`，因 `0015` 被 Story 23.2 落地的 user_cosmetic_items 占用）已在 0016 up.sql 注释头 + Dev Notes + Completion Notes + migrate 测试注释多处显式记录，**不**视为 §5.10 / 契约 / 范围变更（属 epics.md 撰写时序早于 23.2 落地的历史错位，与 Story 23.2 自身 `0014→0015` 同类纠偏先例一致；表名 / 字段 / 索引钦定全部不变）。

## Tasks / Subtasks

- [x] Task 1：读取并定位（AC1 / AC2 / AC6）
  - [x] 读 `docs/宠物互动App_数据库设计.md` §5.10 行 533-567（user_pet_equips DDL + 字段说明 + 设计说明 + 关键约束，唯一 schema 真相源）
  - [x] 读 §6.8 slot 枚举（`{1=hat / 2=gloves / 3=glasses / 4=neck / 5=back / 6=body / 7=tail / 99=other}` 值域钦定）+ §8.4 穿戴事务（下游 26.3 事务边界依据）
  - [x] 读参照 migration `server/migrations/0015_init_user_cosmetic_items.up/down.sql`（同 Epic 链路 + Story 23.2 注释头 / 序号纠偏 / DDL 风格 + 范围红线最近先例）+ `server/migrations/0008_init_room_members.up.sql`（**有 UNIQUE 约束的同类参照** —— uk_room_user 复合 + uk_user_id 单列，与本表 uk_pet_slot 复合 + uk_user_cosmetic_item_id 单列同结构）
  - [x] 读参照 `server/internal/repo/mysql/user_cosmetic_item_repo.go` 行 1-77（GORM struct + TableName 注释头风格；行 78+ 的 interface / impl 是 Story 23.4 / 23.5 后补的，本 story 对应行 1-77 的 23.2 阶段最小集，**不**含 interface）
  - [x] 读 `server/internal/infra/migrate/migrate_integration_test.go`（确认 `expectedTables` 当前为 13 张（Story 23.2 已对账 0014/0015）+ `UpTwice_Idempotent` `tableCount != 13` + `StatusAfterUp` `v != 15` + 文件顶部 `version=15` 注释 + `UserCosmeticItems_Schema` / `RoomMembers_UniqueUserID_Rejected` 两个参照 case 模式 —— 实测起点确为 13 张 / v15，与 Dev Notes 推断一致，无遗漏积压）
- [x] Task 2：新建 0016 up/down migration（AC1 / AC2）
  - [x] 新建 `server/migrations/0016_init_user_pet_equips.up.sql`（注释头 + CREATE TABLE 1:1 §5.10 + 全列 NOT NULL + slot 无 DEFAULT + 两 UNIQUE（uk_pet_slot 复合 + uk_user_cosmetic_item_id 单列）+ idx_user_pet 普通索引 + 无 FK）
  - [x] 新建 `server/migrations/0016_init_user_pet_equips.down.sql`（注释头 + `DROP TABLE IF EXISTS user_pet_equips;`）
  - [x] 确认序号 0016（0015 已被 Story 23.2 user_cosmetic_items 占用）+ UTF-8 / LF
- [x] Task 3：新建 UserPetEquip GORM domain struct（AC3）
  - [x] 新建 `server/internal/repo/mysql/user_pet_equip_repo.go`（注释头 + struct 7 字段全值类型无指针 + slot int8 + `TableName()`）
  - [x] **不**加 Repo interface / 任何方法（YAGNI；最小集，对标 23.2 阶段）
- [x] Task 4：扩展 + 对账 migrate 集成测试（AC4）
  - [x] `TestMigrateIntegration_UpThenDown`：`expectedTables` 追加 `user_pet_equips` → 14 张 + 表数量注释/说明更新（补 Story 26.2 扩展记录）
  - [x] `TestMigrateIntegration_UpTwice_Idempotent`：`IN (...)` 列表追加 `'user_pet_equips'` + `tableCount != 13` → `!= 14` + want 文案 13→14
  - [x] `TestMigrateIntegration_StatusAfterUp`：`v != 15` → `v != 16` + want 文案 + 函数头注释补 Story 26.2 一条 + 文件顶部行 8 `version=15` → `version=16` + 顶部注释块追加 Story 26.2 扩展段（含序号纠偏 0015→0016）
  - [x] 新增 `TestMigrateIntegration_UserPetEquips_Schema`（7 列字段层断言 + 全列 IS_NULLABLE=NO + slot 无 DEFAULT + 2 UNIQUE non_unique=0 列顺序 + idx_user_pet 普通索引列顺序 + UNIQUE 数量=2 正向断言）
  - [x] 新增 `TestMigrateIntegration_UserPetEquips_UniqueConstraints_Rejected`（首条 INSERT 成功 → 违反 uk_pet_slot 被拒 → 违反 uk_user_cosmetic_item_id 被拒 → COUNT=1 兜底）
- [x] Task 5：构建 / 测试（AC5）
  - [x] `bash scripts/build.sh --test`（vet + build + 全量单测 → PASS，含 internal/repo/mysql + internal/infra/migrate 非集成测试全绿）
  - [x] `--integration`：本 story 范围 5 个 migrate case 全 PASS（隔离跑 `go test -tags=integration -timeout=600s ./internal/infra/migrate/` 验证；`./...` 全量 / 全包跑因不相关 Story 17.3 / 20.3 seed 测试的 dockertest 容器复用 replay flake 挂 2 个无关 case，按 AC5 钦定隔离处置 + Debug Log 如实记录，与 Story 23.2 同处置）
  - [x] 确认无回归（抽查相邻 RoomMembers_UniqueUserID_Rejected / UserCosmeticItems_Schema / UserCosmeticItems_AppendableAndUpdatable 全 PASS）+ `git status` 仅范围红线内文件（4 server 文件 + story + sprint-status）
- [x] Task 6：跨文档一致性自检（AC6）
  - [x] 逐项核对 AC6 的 6 条，结论写入本 story "Completion Notes List"
  - [x] 确认序号纠偏（0015→0016）已显式记录、不视为契约/范围变更
  - [x] 标记 sprint-status.yaml `26-2-user_pet_equips-migration` 状态流转 ready-for-dev → in-progress → review

## Dev Notes

### 这是什么类型的 story

纯持久化层 migration story。对标 Story 23.2（user_cosmetic_items migration）/ 20.4（chest_open_logs migration）/ 17.2（emoji_configs migration）/ 11.2（rooms+room_members migration）在各自 Epic 的"表落地"角色。产出物 = 0016 up/down + UserPetEquip GORM struct（最小骨架）+ 完整测试覆盖（schema 字段层 + 双 UNIQUE 拒绝）+ 对账 migrate 集成测试 expectedTables 13→14 / 版本号 v15→v16（**起点干净** —— Story 23.2 已把 0014/0015 积压一次性对账完，本 story 无新增积压，只在 13 基础上 +1）。

**与 Story 23.2 的关键差异点**（两者都是仓库/穿戴链路相邻 migration story，但表结构特性正好相反，dev 实装时务必区分）：

| 维度 | Story 23.2 user_cosmetic_items | 本 story 26.2 user_pet_equips |
|---|---|---|
| UNIQUE 约束 | **无 UNIQUE**（同种配置可持有多件 FR16） | **两个 UNIQUE**：uk_pet_slot 复合 + uk_user_cosmetic_item_id 单列 |
| 可空列 | 有 2 个（source_ref_id / consumed_at NULL） | **全列 NOT NULL，无可空列** |
| GORM 指针字段 | 有 `*uint64` / `*time.Time` 指针映射 NULL | **无任何指针字段，全值类型** |
| 集成测试运行时 case | `AppendableAndUpdatable`（验无 UNIQUE 多行 + status UPDATE） | `UniqueConstraints_Rejected`（验**两个** UNIQUE 各自拒绝插入） |
| migrate 对账 | 需补 0014 积压 + 加 0015（积压翻倍风险） | **起点干净**，仅 13→14 / v15→v16（无积压） |
| 序号纠偏 | epics.md 写 0014 → 实际 0015（0014 被 20.6 占） | epics.md 写 0015 → 实际 0016（**0015 被 23.2 占**） |

### 关键纠偏点 1：migration 序号是 0016 不是 0015

epics.md §Story 26.2 行 3526 文字写 `migrations/0015_init_user_pet_equips.sql`，但 `0015` 已被 **Story 23.2** 落地的 `0015_init_user_cosmetic_items.up/down.sql` 占用（玩家装扮实例表）。epics.md 撰写时点早于 23.2 实际落地 0015，属历史时序错位（与 Story 23.2 自身遇到的"epics.md 写 0014 但 0014 被 20.6 占 → 实际用 0015"**同类纠偏先例**）。本 story 按 ADR-0003 顺序递增编号约定取**下一个空闲序号 = `0016`**（已确认 `server/migrations/` 当前最大序号是 `0015_init_user_cosmetic_items`）。这**不**是 §5.10 schema 变更、**不**是契约变更、**不**是范围变更（表名 `user_pet_equips` / 字段 / 两个 UNIQUE / 索引钦定全部不变，仅文件序号偏移）。Completion Notes 必须显式记录此纠偏。

### 关键纠偏点 2：migrate 集成测试对账起点干净（与 23.2 不同，无积压翻倍风险）

Story 23.2 实装时遇到 0014 落地（Story 20.6）漏更新 migrate 集成测试断言的积压，需一次性对账 0014+0015 两条。**本 story 起点干净** —— Story 23.2 已在其实装中把 `expectedTables`（11→13 张，补 chest_open_idempotency_records + user_cosmetic_items）+ `StatusAfterUp`（v13→v15）一次性对账到位（已用 dockertest 实跑 PASS 验证，见 23-2 story Debug Log）。本 story 只需在**已正确的 13 张 / v15 基础上 +1**（追加 user_pet_equips → 14 张 / v16），**无**新增积压需补、**无**翻倍风险。dev 实装前请先本地核对：跑 `bash scripts/build.sh --integration` 前确认 `UpThenDown` 的 `expectedTables` 当前确为 13 张（含 chest_open_idempotency_records + user_cosmetic_items）、`StatusAfterUp` 当前断言确为 `v != 15`、文件顶部行 8 注释确为 `version=15`；若与此推断不符（例如 23.2 对账有遗漏），以代码真相为准如实对账并在 Completion Notes 记录实际起点值。

### 字段语义唯一来源 = 数据库设计文档 §5.10（DB → API 单向）

- `user_pet_equips`：`docs/宠物互动App_数据库设计.md` §5.10 行 538-550 DDL + 行 553-556 字段说明 + 行 558-565 设计说明 / 关键约束
- 枚举：§6.8 slot（`1=hat / 2=gloves / 3=glasses / 4=neck / 5=back / 6=body / 7=tail / 99=other`；与 cosmetic_items.slot 同义，§6.8 钦定）
- **两个 UNIQUE 约束**（§5.10 行 547-548 + 行 562-565 设计说明 + §6.5 行 912-917 索引说明钦定）：
  - `UNIQUE(pet_id, slot)`（一个槽位只能穿一件 —— 26.3 同槽换装 server 端自动卸旧装备后 INSERT 新行的逻辑上层保证 + DB 层 `uk_pet_slot` 兜底；26.5 "并发 1：同 pet 同 slot 100 并发 equip → 只 1 成功" 测试契约依据）
  - `UNIQUE(user_cosmetic_item_id)`（一件实例只能被装备一次 —— NFR11；26.5 "并发 2：同实例并发 equip → 只 1 成功" 测试契约依据）
- 本 story 落地的 schema 是 26.1 §8.3 / §8.4 锚定的 API 字段（`equipped.slot` 枚举 §6.8 = `user_pet_equips.slot` / `equipped.userCosmeticItemId` = `user_pet_equips.user_cosmetic_item_id` 字符串化 / `petId` = `user_pet_equips.pet_id`）的 **DB 端真相源**；DB → API 单向，**不**反向改 DB

### 易错点（review 高频命中，提前规避）

1. **序号写成 0015**：必须 0016（0015 被 Story 23.2 user_cosmetic_items 占；与 23.2 自身 0014→0015 同类纠偏先例）。
2. **漏掉 UNIQUE / 写错约束名**：必须**两个** UNIQUE 且约束名精确 —— `uk_pet_slot (pet_id, slot)`（复合，列顺序 pet_id 先 slot 后）+ `uk_user_cosmetic_item_id (user_cosmetic_item_id)`（单列）；约束名拼错会让 26.5 并发兜底测试 / 26.1 §8.3 关键约束引用对不上。
3. **给字段加 NULL / 加指针**：user_pet_equips **全列 NOT NULL**，struct **无任何指针字段**（与 Story 23.2 user_cosmetic_items 有 source_ref_id/consumed_at 可空列+指针映射相反 —— 不要照搬 23.2 的指针映射模式）。
4. **slot 加 DEFAULT**：§5.10 钦定 slot `TINYINT NOT NULL` **无 DEFAULT**（slot 由 equip 时 cosmetic_items 配置决定必传，非客户端）；不要顺手加 `DEFAULT 0` / `DEFAULT 1`。schema 测试需断言 column_default 为 NULL（无默认值）。
5. **slot 用 uint8**：必须 `int8` 映射 TINYINT，与 `CosmeticItem.Slot int8`（cosmetic_item_repo.go）跨表同类型对齐 —— 用 uint8 会让跨表 slot 比较类型不一致。
6. **建 FK**：本设计全程不建 FK（ADR-0003 / §3 + §7），`user_id` / `pet_id` / `user_cosmetic_item_id` 语义 reference 但不建 FK。
7. **migrate 对账漏掉某个 case**：必须同时改 4 处 —— `UpThenDown.expectedTables`（→14）+ `UpTwice_Idempotent` 的 `IN (...)` 列表 + `tableCount != 13`（→14）+ `StatusAfterUp` `v != 15`（→16）+ 文件顶部行 8 `version=15` 注释（→16）；遗漏任一处 `--integration` 下会断言失败。
8. **预实装 26.3/26.4/26.6 方法**：本 story 仅 struct+TableName，**不**加 Repo interface / InsertInTx / DeleteByPetSlot / FindByPetSlot / ListByUserPetForHome 方法（对标 23.2 阶段最小集；`user_cosmetic_item_repo.go` 现有 interface 是 23.4/23.5 后补的，**不**是 23.2 阶段产物）。
9. **改 0015 / 改 Story 23.x 开箱/入仓事务**：穿戴对 user_cosmetic_items.status 1↔2 推进是 26.3/26.4 范围；本 story **仅** CREATE user_pet_equips 表，不触碰任何已有事务 / 0015 / user_cosmetic_items。
10. **改 §5.10 文档**：§5.10 是 schema 输入，本 story 严格对齐**不修改**；不一致时以 §5.10 为准改本 story。

### 范围红线（再次强调）

只改 4 个 server 文件 + story 文件 + sprint-status.yaml：
- 新建 `server/migrations/0016_init_user_pet_equips.up.sql`
- 新建 `server/migrations/0016_init_user_pet_equips.down.sql`
- 新建 `server/internal/repo/mysql/user_pet_equip_repo.go`（仅 struct + TableName）
- 改 `server/internal/infra/migrate/migrate_integration_test.go`（对账 13→14 / v15→v16 断言 + 新增 2 case）

**不**改任何 `.go` service/handler、**不**改 0001~0015 既有 migration、**不**改 Story 23.x 入仓/开箱事务、**不**改 V1 接口契约 / `docs/宠物互动App_*.md` / `_bmad-output/` 下其他 yaml/md。改了 Go 代码 → **必须**跑 `bash scripts/build.sh --test` + `bash scripts/build.sh --integration`。

### Project Structure Notes

- migration 落地目录 `server/migrations/`（ADR-0003 / Story 4.3 锚定的 golang-migrate `NNNN_name.up.sql` / `.down.sql` 双向规范；本 story 沿用，不修改约定；当前最大序号 0015 → 本 story 取 0016）。
- GORM domain struct 落地 `server/internal/repo/mysql/user_pet_equip_repo.go`（与 `user_cosmetic_item_repo.go` / `cosmetic_item_repo.go` / `room_member_repo.go` 同目录同模式；`docs/宠物互动App_Go项目结构与模块职责设计.md` §6 钦定 `internal/repo/mysql/` 目录）。
- migrate 集成测试 `server/internal/infra/migrate/migrate_integration_test.go`（`-tags=integration` dockertest，Story 4.3 起既有文件，逐 story 扩展 expectedTables / 版本号 / 新增 schema case）。
- 无新增目录 / 模块 / 依赖（GORM / golang-migrate / dockertest 均既有；本 story 不引入任何新 import 之外的依赖，struct 仅需 `time` + gorm）。

### References

- [Source: docs/宠物互动App_数据库设计.md#5.10（行 533-567）] — user_pet_equips DDL + 字段说明 + 设计说明 + 两个 UNIQUE 关键约束（唯一 schema 真相源）
- [Source: docs/宠物互动App_数据库设计.md#6.8] — slot 枚举值域钦定（`{1=hat/2=gloves/3=glasses/4=neck/5=back/6=body/7=tail/99=other}`，与 cosmetic_items.slot 同义）
- [Source: docs/宠物互动App_数据库设计.md#6.5 索引设计（行 912-917）] — user_pet_equips UNIQUE(pet_id,slot) + UNIQUE(user_cosmetic_item_id) 索引语义说明
- [Source: docs/宠物互动App_数据库设计.md#8.4 穿戴事务（行 1009-1019）] — 下游 Story 26.3 equip 事务边界依据（本 story 仅落地 schema，不实装事务）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 26.2（行 3516-3535）] — AC 钦定（字段全集 + 两 UNIQUE + idx_user_pet + down.sql + ≥3 case 单测 + dockertest 双 UNIQUE 拒绝；序号 0015 文字属历史时序错位，本 story 纠偏为 0016）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 26.3（行 3537-3566） / Story 26.4（行 3568-3590） / Story 26.5（行 3592-3615） / Story 26.6（行 3617-3637）] — 下游 equip/unequip 事务 + Layer 2 并发兜底测试 + GET /home 扩展（本 story 落地 schema 的下游消费方）
- [Source: _bmad-output/planning-artifacts/epics.md#NFR11（行 113）/ NFR1（行 99）/ NFR2（行 100）] — 一件实例同时只能装备一次（UNIQUE(user_cosmetic_item_id) 依据）+ 资产操作事务原子 + equipped 状态与 user_pet_equips 一致性
- [Source: _bmad-output/implementation-artifacts/26-1-接口契约最终化.md] — 上游契约定稿（§8.3 equip / §8.4 unequip schema 锚定 + §1 节点 9 冻结声明 + §8.3 关键约束段引用本表两个 UNIQUE 作并发兜底；本 story 落地 schema 是其 DB 端真相源）
- [Source: _bmad-output/implementation-artifacts/23-2-user_cosmetic_items-migration.md] — 同类 migration story 最近先例（结构 / 范围红线 / 测试覆盖编排 / 序号纠偏模式 / migrate 对账模式；**注意本表与 23.2 表的 UNIQUE/可空列/指针特性正好相反，见 Dev Notes 差异表**）
- [Source: server/migrations/0015_init_user_cosmetic_items.up.sql / .down.sql] — 同 Epic 链路相邻 migration 注释头 + DDL 风格 + 序号纠偏先例参照（Story 23.2 落地）
- [Source: server/migrations/0008_init_room_members.up.sql] — **有 UNIQUE 约束的同类 DDL 参照**（uk_room_user 复合 + uk_user_id 单列，与本表 uk_pet_slot 复合 + uk_user_cosmetic_item_id 单列同结构；Story 11.2 落地）
- [Source: server/internal/repo/mysql/user_cosmetic_item_repo.go（行 1-77）] — GORM domain struct + TableName 注释头风格参照（**仅** 1-77 行的 23.2 阶段最小集；行 78+ interface/impl 是 23.4/23.5 后补，**不**参照）
- [Source: server/internal/repo/mysql/cosmetic_item_repo.go] — `CosmeticItem.Slot int8` 跨表 slot 同类型对齐参照
- [Source: server/internal/infra/migrate/migrate_integration_test.go#TestMigrateIntegration_UserCosmeticItems_Schema（行 2354-2388+）] — INFORMATION_SCHEMA 字段层 schema 断言模式参照（本 story 仿此写 UserPetEquips_Schema，差异：断言两 UNIQUE 而非无 UNIQUE、7 列而非 10 列、全列 NOT NULL）
- [Source: server/internal/infra/migrate/migrate_integration_test.go#TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected（行 684-745）] — **dockertest UNIQUE 拒绝验证模式参照**（本 story 仿此写 UniqueConstraints_Rejected，差异：本表两个独立 UNIQUE 各验一次拒绝 + Duplicate entry 关键字判定）
- [Source: server/internal/infra/migrate/migrate_integration_test.go#TestMigrateIntegration_UpThenDown（行 188-）/ UpTwice_Idempotent（行 351-）/ StatusAfterUp（行 599-647）+ 文件顶部注释块（行 1-60）] — 待对账的断言（expectedTables 13 张 / tableCount!=13 / v!=15 / 顶部 version=15 注释；Story 23.2 已对账 0014/0015，本 story 起点干净仅 +1）
- [Source: CLAUDE.md（Build & Test 段）] — `bash scripts/build.sh --test` / `--integration` 验证契约；资产类操作必须事务（穿戴，下游 26.3）
- [Source: _bmad-output/implementation-artifacts/decisions/0003-* / ADR-0003] — migration 工具 + 顺序递增编号约定 + .up/.down 双向规范 + 禁止 GORM AutoMigrate（本 story 沿用，序号纠偏依据）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]（bmad-dev-story workflow，epic-loop 派出的隔离 sub-agent）

### Debug Log References

- `bash scripts/build.sh --test`：vet + build（commit=401eca1）+ 全量单测 `go test -count=1 ./...` 全 PASS（含 `internal/repo/mysql` 新增 `UserPetEquip` struct 编译通过 + `internal/infra/migrate` 非集成测试全绿）。`BUILD SUCCESS`。
- 集成测试（dockertest 起真实 mysql:8.0 容器）：
  - **本 story 范围 5 个 migrate case 全 PASS**（隔离跑 `go test -tags=integration -timeout=600s -run 'UserPetEquips_Schema|UserPetEquips_UniqueConstraints_Rejected|UpThenDown|UpTwice_Idempotent|StatusAfterUp' ./internal/infra/migrate/`）：
    - `TestMigrateIntegration_UpThenDown` PASS（expectedTables 14 张含 user_pet_equips，Up→Down 全消失）
    - `TestMigrateIntegration_UpTwice_Idempotent` PASS（tableCount=14 幂等）
    - `TestMigrateIntegration_StatusAfterUp` PASS（v=16 + dirty=false）
    - `TestMigrateIntegration_UserPetEquips_Schema` PASS（新增；7 列 + 全列 IS_NULLABLE=NO + slot 无 DEFAULT + uk_pet_slot/uk_user_cosmetic_item_id non_unique=0 列顺序 + idx_user_pet non_unique=1 列顺序 + UNIQUE 数量=2 正向断言）
    - `TestMigrateIntegration_UserPetEquips_UniqueConstraints_Rejected` PASS（新增；首条 INSERT 成功 → 违反 uk_pet_slot 被拒（Duplicate entry）→ 违反 uk_user_cosmetic_item_id 被拒（Duplicate entry）→ COUNT=1 兜底）
  - **抽查相邻既有 case 无回归**：`RoomMembers_UniqueUserID_Rejected`（UNIQUE 拒绝模式参照）/ `UserCosmeticItems_Schema`（schema 断言模式参照）/ `UserCosmeticItems_AppendableAndUpdatable` 全 PASS。
  - **已知环境侧 flake（非本 story 范围、非回归）**：`./internal/infra/migrate/` 全包跑时 `TestMigrateIntegration_EmojiConfigs_SeedIdempotent`（Story 17.3）+ `TestMigrateIntegration_CosmeticItems_SeedForceOverwrite`（Story 20.3）两个 **seed-migration** 测试 FAIL（`Error 1050 (42S01): Table 'cosmetic_items'/'chest_open_logs' already exists`）。该两 case 走"手动 UPDATE schema_migrations 回滚版本号 → 再 Up"路径，在 dockertest 容器复用语境下触发 golang-migrate replay 撞已存在表的环境侧 timing flake；**在隔离单跑这两个 case 时同样 FAIL**（与本 story 改动无关，与/无本 story 改动表现一致）—— 属 Story 23.2 Debug Log 已记录的同类已知环境侧 flake，AC5 钦定遇此用隔离验证 + Debug Log 如实记录的处置路径。本 story 仅新增 0016 + struct + 改 migrate 集成测试断言，无任何既有 seed migration / 既有逻辑改动，故非回归。
- `git status`：仅范围红线内 6 个对象（2 新 migration SQL + 1 新 repo struct + 1 改 migrate 集成测试 + story 文件 + sprint-status.yaml），无越界改动。

### Completion Notes List

**实装产出**：0016 up/down migration（user_pet_equips 表，1:1 对齐 §5.10 钦定 DDL，含双 UNIQUE uk_pet_slot 复合 + uk_user_cosmetic_item_id 单列 + idx_user_pet 普通索引 + 全列 NOT NULL + slot 无 DEFAULT + 无 FK）+ `UserPetEquip` GORM domain struct 最小骨架（7 字段全值类型无指针 + slot int8 + TableName()，无 interface/方法）+ migrate 集成测试对账 13→14 张 / v15→v16 + 新增 `UserPetEquips_Schema` / `UserPetEquips_UniqueConstraints_Rejected` 两个 dockertest case。

**AC6 跨文档一致性自检（逐项结论）**：

1. **0016 DDL 与 §5.10 行 538-550 逐字符比对一致**：字段名 / 类型（id/user_id/pet_id/user_cosmetic_item_id BIGINT UNSIGNED、slot TINYINT、created_at/updated_at DATETIME(3)）/ 全列 NOT NULL / slot 无 DEFAULT / 两个 UNIQUE 约束名与列顺序（`uk_pet_slot (pet_id, slot)` 复合 + `uk_user_cosmetic_item_id (user_cosmetic_item_id)` 单列）/ `idx_user_pet (user_id, pet_id)` 列顺序 / `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4` —— 全部 1:1 对齐，dockertest schema case 实跑验证通过。✅
2. **`UserPetEquip` struct 与 0016 真实 schema 逐字段一致**：列名映射全部对齐；全值类型无任何指针（user_pet_equips 全列 NOT NULL，与 23.2 `UserCosmeticItem` 有 `*uint64`/`*time.Time` 指针映射 NULL 列正好相反）；`Slot int8` 映射 TINYINT，与 `CosmeticItem.Slot int8`（cosmetic_item_repo.go:47）跨表同类型对齐；uint64 映射 BIGINT UNSIGNED。✅
3. **slot 无 DEFAULT 与 §5.10 行 542 + §6.8 枚举钦定语义无矛盾**：slot 由 equip 时 cosmetic_items 配置决定必传、非客户端传入，DDL 不设 DEFAULT、不做 enum 约束正确；schema case 已断言 `slot` column_default 为 NULL（无默认值）。✅
4. **migrate 集成测试断言与真实 migration 序号集合一致**：0001~0016 共 16 个 up.sql，其中 0010_seed_emoji_configs + 0012_seed_cosmetic_items 是 seed migration 不建表 → 实际建表数 = 16 - 2 = 14 张。`expectedTables` 14 张 + `UpTwice_Idempotent` tableCount=14 + `StatusAfterUp` v=16 全部一致；实测起点确为 13 张 / v15（Story 23.2 已对账 0014/0015），本 story 起点干净仅 +1（user_pet_equips），无新增积压。✅
5. **未触碰红线外内容**：未改 0001~0015 既有 migration / Story 23.2 user_cosmetic_items / Story 23.5 / 20.6 开箱事务 / V1 接口契约 / 数据库设计 §5.10 / 其他 6 份 docs / ADR-0003 / ADR-0007 / cmd/server/main.go；未实装任何 service/handler/repo write/read 方法 / interface；未加 GORM uniqueIndex/index tag / gorm.Model / 指针字段 / slot DEFAULT / FK / enum 校验 / §5.10 之外字段。`git status` 实测仅 6 个范围内对象。✅
6. **序号纠偏（0015→0016）已多处显式记录**：0016 up.sql + down.sql 注释头 + Dev Notes + 本 Completion Notes + migrate 集成测试文件顶部注释块 + StatusAfterUp 函数头注释均显式记录"epics.md §Story 26.2 文字写 0015，因 0015 被 Story 23.2 落地的 user_cosmetic_items 占用，本 story 取下一空闲序号 0016"，明确属 epics.md 撰写时序早于 23.2 落地的历史时序错位、与 Story 23.2 自身 0014→0015 同类纠偏先例一致、**不**视为 §5.10/契约/范围变更（表名/字段/索引钦定全部不变，仅文件序号偏移）。✅

### File List

- `server/migrations/0016_init_user_pet_equips.up.sql`（新建）
- `server/migrations/0016_init_user_pet_equips.down.sql`（新建）
- `server/internal/repo/mysql/user_pet_equip_repo.go`（新建）
- `server/internal/infra/migrate/migrate_integration_test.go`（修改：文件顶部注释块 + 行 8 version 注释对账 + UpThenDown expectedTables 13→14 + UpTwice_Idempotent IN 列表/tableCount 13→14 + StatusAfterUp v15→v16 + 注释 + 新增 UserPetEquips_Schema + 新增 UserPetEquips_UniqueConstraints_Rejected）
- `_bmad-output/implementation-artifacts/26-2-user_pet_equips-migration.md`（修改：Tasks 勾选 + Dev Agent Record + Status review）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（修改：26-2 ready-for-dev → in-progress → review）

## Change Log

| 日期 | 变更 | 说明 |
|---|---|---|
| 2026-05-17 | Story 26.2 实装完成 | 新建 0016_init_user_pet_equips up/down migration（1:1 §5.10，双 UNIQUE + idx_user_pet + 全列 NOT NULL + slot 无 DEFAULT + 无 FK）+ UserPetEquip GORM struct 最小骨架（7 字段全值类型无指针 + slot int8 + TableName）+ migrate 集成测试对账 13→14 张 / v15→v16 + 新增 UserPetEquips_Schema / UserPetEquips_UniqueConstraints_Rejected 两个 dockertest case。`build.sh --test` PASS；本 story 范围 5 个 migrate 集成 case 隔离跑全 PASS；序号纠偏 0015→0016（0015 被 Story 23.2 占用）已多处记录。Status → review。 |
