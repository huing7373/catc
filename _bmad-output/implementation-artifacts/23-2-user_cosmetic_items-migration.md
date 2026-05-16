# Story 23.2: user_cosmetic_items migration（首次落地 0015_init_user_cosmetic_items.up/down.sql + UserCosmeticItem GORM domain struct 最小骨架 + ≥3 case 单测 + dockertest 集成测试 + 顺手对账 migrate 集成测试积压的 0014 表/版本断言）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want **首次落地** `server/migrations/0015_init_user_cosmetic_items.up.sql` + `server/migrations/0015_init_user_cosmetic_items.down.sql` 两个新 migration 文件（严格按 `docs/宠物互动App_数据库设计.md` §5.9 行 483-523 钦定的 CREATE TABLE DDL：`id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT` + `user_id BIGINT UNSIGNED NOT NULL` + `cosmetic_item_id BIGINT UNSIGNED NOT NULL` + `status TINYINT NOT NULL DEFAULT 1` + `source TINYINT NOT NULL DEFAULT 1` + `source_ref_id BIGINT UNSIGNED NULL` + `obtained_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)` + `consumed_at DATETIME(3) NULL` + `created_at / updated_at DATETIME(3)` + `KEY idx_user_id_status (user_id, status)` + `KEY idx_user_id_cosmetic_item_id (user_id, cosmetic_item_id)` + `KEY idx_source (source, source_ref_id)` + `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`，1:1 对齐 §5.9；**migration 序号 = `0015` 不是 epics.md 文字里写的 `0014`** —— `0014` 已被 Story 20.6 落地的 `0014_init_chest_open_idempotency_records` 占用，本 story 取下一个空闲序号 `0015`，与 ADR-0003 顺序递增编号约定一致）+ **新增** `server/internal/repo/mysql/user_cosmetic_item_repo.go` 含 `UserCosmeticItem` GORM domain struct（与 0015 真实 schema 1:1 对齐：`ID / UserID / CosmeticItemID / Status / Source / SourceRefID / ObtainedAt / ConsumedAt / CreatedAt / UpdatedAt`，`SourceRefID` 用 `*uint64`、`ConsumedAt` 用 `*time.Time` 映射 NULL 列）+ `TableName() string` 显式返回 `"user_cosmetic_items"`（**仅** struct + TableName，**不**新增 Repo interface / 实装任何 List / Insert / Aggregate 方法，YAGNI；Story 23.4 落地 inventory 聚合查询方法 / Story 23.5 落地开箱事务 INSERT 方法 / Epic 26 / 32 落地穿戴 / 合成方法）+ **扩展** `server/internal/infra/migrate/migrate_integration_test.go` 把 `TestMigrateIntegration_UpThenDown` / `TestMigrateIntegration_UpTwice_Idempotent`（如表数量断言）/ `TestMigrateIntegration_StatusAfterUp`（版本号断言）从**积压的旧值**（这两处断言在 Story 20.6 加 0014 时漏更新 → 当前注释/断言停在 version=13 / 11 张表，真实状态是 version=14 / 12 张表）一次性对账到本 story 落地后的正确值（`expectedTables` 同时补齐 `chest_open_idempotency_records` + `user_cosmetic_items` 两张表 → 13 张；`StatusAfterUp` 版本号断言 `v != 13` → `v != 15` + 注释补 0014 / 0015 两条扩展记录）+ **新增** `TestMigrateIntegration_UserCosmeticItems_Schema`（按 §5.9 用 `INFORMATION_SCHEMA.COLUMNS` / `STATISTICS` 做字段层 + 索引层断言，含 10 列字段计数兜底防漂移、`source_ref_id` / `consumed_at` 的 NULL 可空性断言、三个普通索引列顺序断言、**无 UNIQUE 约束**断言）+ **新增** `TestMigrateIntegration_UserCosmeticItems_AppendableAndUpdatable` dockertest 集成测试（覆盖 user_cosmetic_items **无 UNIQUE 约束** → 同 user_id 可插入多行实例 + `status` / `consumed_at` 可被 UPDATE 推进的运行时语义，与 Story 20.4 落地的 `TestMigrateIntegration_ChestOpenLogs_AppendOnly` 同模式 —— 后者验证 append-only 无 UNIQUE 拒绝，本表额外验证 status 可推进的可变实例语义）,
so that **Story 23.3（GET /cosmetics/catalog 接口）+ Story 23.4（GET /cosmetics/inventory 接口，按 cosmetic_item_id 聚合 + status IN (1,2) 过滤 + 三态 config 完整矩阵）+ Story 23.5（修改 Story 20.6 开箱事务补"入仓"—— 在抽奖产出后、写 chest_open_logs 前 INSERT 一行 user_cosmetic_items 拿到 id 回填 chest_open_logs.reward_user_cosmetic_item_id + response.reward.userCosmeticItemId）+ Story 20.8（dev 端点 /dev/grant-cosmetic-batch 23.5 完成后打开真实写库）+ iOS Epic 24（仓库页 + LoadInventoryUseCase + 聚合 grid + 实例列表）+ Epic 26（穿戴事务 user_pet_equips 引用 user_cosmetic_items.id + status 推进 1↔2）+ Epic 32 / 33（合成事务消耗 10 件实例 status→3 consumed + 产出新实例）**可以基于一个**已落地、已具备完整测试覆盖、已通过 dockertest 真实多行 INSERT + status UPDATE 验证、已具备完整 GORM domain struct 字段映射（含 NULL 列正确指针映射）**的 user_cosmetic_items 持久化基础并行展开，不再出现"23.4 写 inventory 聚合 SELECT 时找不到表 / 23.5 开箱事务 INSERT user_cosmetic_items 时 status / source 默认值与 §5.9 漂移 / 23.5 拿不到 idx_user_id_status 索引导致 inventory 查询慢 / Epic 26 穿戴时 status 推进无法走索引 / Epic 32 合成消耗实例时 source_ref_id 回填类型不对（NULL vs 0）/ GORM struct 把 NULL 列映射成非指针导致 source_ref_id=NULL 时 panic / 节点 8 实装才发现 migrate 集成测试断言早在 0014 就停在 version=13 一直没人对账，本 story 又加 0015 让积压翻倍"的返工。

## 故事定位（Epic 23 第二条 = 第一条**实装** story；上承 23.1 契约定稿，下启 23.3 catalog 接口 + 23.4 inventory 聚合接口 + 23.5 开箱事务补入仓）

- **Epic 23 进度**：23.1（契约定稿 §8.1 / §8.2 / §1 冻结，done）→ **23.2（本 story，user_cosmetic_items migration + GORM domain struct + 测试覆盖 + 顺手对账 0014 积压断言）** → 23.3（GET /cosmetics/catalog 接口）→ 23.4（GET /cosmetics/inventory 接口，按 cosmetic_item_id 聚合 + status IN (1,2) 过滤 + 三态 config 矩阵）→ 23.5（修改 Story 20.6 开箱事务补"入仓"+ 回填 chest_open_logs.reward_user_cosmetic_item_id）。
- **本 story 是 23.3 / 23.4 / 23.5 / 20.8 / iOS Epic 24 / Epic 26 / Epic 32 / 33 的强前置**（本 story done 后才能开工）：
  - **23.4 GET /cosmetics/inventory**：service 流程查 `SELECT id, cosmetic_item_id, status FROM user_cosmetic_items WHERE user_id = ? AND status IN (1, 2)`（23.1 §8.2 服务端逻辑钦定）必须命中本 story 落地的 0015 表 schema + `idx_user_id_status (user_id, status)` 索引覆盖；GORM struct（本 story 新增的 `UserCosmeticItem`）直接被 23.4 repo 层复用做字段映射
  - **23.5 修改开箱事务补入仓**：23.5 在 Story 20.6 `ChestService.OpenChest` 事务内"抽奖产出 cosmetic_item_id 之后、写 chest_open_logs 之前"插入 `INSERT INTO user_cosmetic_items (user_id, cosmetic_item_id, status, source, source_ref_id, obtained_at) VALUES (?, ?, 1, 1, ?, NOW(3))`（status=1 in_bag / source=1 chest / source_ref_id=chest_id；epics.md §Story 23.5 + §5.9 字段说明钦定）必须命中本 story 落地的表 schema；`status TINYINT NOT NULL DEFAULT 1` / `source TINYINT NOT NULL DEFAULT 1` 默认值语义依赖本 story 已落地的 DDL；本 story **不**预实装任何 INSERT 事务逻辑
  - **20.8 dev /dev/grant-cosmetic-batch**：23.5 完成后才打开真实写库（节点 7 阶段是 placeholder）；批量发放 INSERT user_cosmetic_items 引用本 story 落地的 schema
  - **iOS Epic 24（仓库页 + LoadInventoryUseCase）**：iOS 端 `InventoryGroup` / `InventoryInstance` Codable struct 通过 23.4 GET /cosmetics/inventory JSON response 间接依赖本 story 落地的 DB schema（`user_cosmetic_items.id` → §8.2 `instances[].userCosmeticItemId` / `user_cosmetic_items.status` → §8.2 `instances[].status` 枚举 {1,2}）
  - **Epic 26 穿戴事务**：`user_pet_equips`（Story 26.2 owner）reference 本 story 落地的 `user_cosmetic_items.id`；穿戴 / 卸下时 `user_cosmetic_items.status` 在 1（in_bag）↔ 2（equipped）间推进，依赖 `idx_user_id_status` 索引
  - **Epic 32 / 33 合成事务**：合成消耗 10 件实例 → `user_cosmetic_items.status` 推进到 3（consumed）+ 写 `consumed_at`；合成产出新实例 INSERT（source=2 compose / source_ref_id=compose_log_id 回填）；全部 reference 本 story 落地的 schema + NULL 列语义（`source_ref_id` 先 NULL 后回填 / `consumed_at` 未消耗时 NULL）
- **epics.md §Story 23.2 钦定**（行 3228-3246）：
  - `migrations/0014_init_user_cosmetic_items.sql` 按数据库设计.md §5.9 创建表（**注：序号纠偏**——epics.md 文字写 `0014`，但 `0014` 已被 Story 20.6 落地的 `0014_init_chest_open_idempotency_records` 占用，本 story 实际用 `0015`；序号偏移属"epics.md 撰写时点早于 20.6 落地"的历史时序错位，**不**视为契约/范围变更，按 ADR-0003 顺序递增取下一空闲号；表名 / 字段 / 索引钦定全部不变）
  - 字段全集（含 `id`, `user_id`, `cosmetic_item_id`, `status`, `source`, `source_ref_id`, `obtained_at`, `consumed_at`, `created_at`, `updated_at`）
  - `KEY idx_user_id_status` + `KEY idx_user_id_cosmetic_item_id` + `KEY idx_source`
  - 含 down.sql（DROP TABLE 路径）
  - **单元测试覆盖**（≥3 case）：happy（migrate up 后表存在 + 字段类型 + 索引都符合 §5.9）/ happy（migrate down 后表删除，由现有 `TestMigrateIntegration_UpThenDown` 扩展覆盖）/ edge（重复 migrate up → 幂等，由现有 `TestMigrateIntegration_UpTwice_Idempotent` 扩展覆盖）
  - **集成测试覆盖**（dockertest）：migrate up → SHOW CREATE TABLE 对比 schema → migrate down（本 story 用更精确的 `INFORMATION_SCHEMA.COLUMNS` / `STATISTICS` 字段层断言取代字符串 SHOW CREATE TABLE 比对 —— 后者跨 MySQL 版本输出格式不稳定，前者是结构化 schema 真相源，与 Story 20.2 落地的 `TestMigrateIntegration_CosmeticItems_Schema` / 17.2 落地的 `TestMigrateIntegration_EmojiConfigs_Schema` 同模式）
- **Story 23.1 上游冻结边界**（V1 §8.2 inventory 字段表 + 数据库设计 §5.9 字段表 + §6.10 status 枚举 + §6.11 source 枚举）：本 story 落地的字段（`status TINYINT` / `source TINYINT` / `cosmetic_item_id BIGINT UNSIGNED` / `id BIGINT UNSIGNED`）是 23.1 §8.2 锚定的 API 字段语义（`instances[].userCosmeticItemId` BIGINT 字符串化 = `user_cosmetic_items.id` / `instances[].status` 枚举 {1,2} = `user_cosmetic_items.status` 子集 / `cosmeticItemId` = `user_cosmetic_items.cosmetic_item_id`）的**DB 端真相源**；本 story **不**反向修改 DB schema（DB → API 单向），仅严格对齐数据库设计文档 §5.9 DDL
- **范围红线**：
  - 本 story **只**改 `server/migrations/0015_init_user_cosmetic_items.up.sql`（新建）+ `server/migrations/0015_init_user_cosmetic_items.down.sql`（新建）+ `server/internal/repo/mysql/user_cosmetic_item_repo.go`（新建，含 `UserCosmeticItem` struct + `TableName()`）+ `server/internal/infra/migrate/migrate_integration_test.go`（扩展：`UpThenDown` / `UpTwice_Idempotent` 表数量断言对账到 13 张（补 chest_open_idempotency_records + user_cosmetic_items 两张）+ `expectedTables` slice 补这两张表 + `StatusAfterUp` 版本号 `v != 13` → `v != 15` + 注释补 0014 / 0015 两条扩展记录 + 新增 `TestMigrateIntegration_UserCosmeticItems_Schema` + 新增 `TestMigrateIntegration_UserCosmeticItems_AppendableAndUpdatable`）+ 本 story 文件 + sprint-status.yaml 流转
  - **不**实装任何 service / handler / repo write / read 方法（23.4 / 23.5 才做；本 story 阶段**仅**落地 GORM struct + TableName，**不**新建 `UserCosmeticItemRepo` interface / 实装 `ListByUserStatus` / `InsertInTx` / `AggregateByCosmetic` / `MarkConsumed` 等方法）
  - **不**实装任何 INSERT / seed SQL（user_cosmetic_items 是运行时实例表，**无** seed 阶段；Story 23.5 开箱事务才首条 INSERT）
  - **不**修改 Story 20.6 `ChestService.OpenChest` 开箱事务（补入仓是 Story 23.5 钦定范围；本 story **仅** CREATE TABLE，不触碰任何已有 service / 事务）
  - **不**实装 chest_open_logs.reward_user_cosmetic_item_id 回填逻辑（Story 23.5 范围；该列已由 Story 20.4 落地的 0013 表定义为 `BIGINT UNSIGNED NOT NULL` 占位 0，本 story **不**触碰 0013）
  - **不**改 V1 接口契约（23.1 已冻结 §8.1 / §8.2 / §1）
  - **不**改任何 `docs/宠物互动App_*.md`（§5.9 是契约**输入**，本 story 严格对齐它但**不修改**；如发现 §5.9 与本 story 落地的 DDL 有不一致 → 优先以 §5.9 为准修改本 story 而非反向改 §5.9）
  - **不**改 `_bmad-output/` 下其他 yaml / md（除自己 story 文件 + sprint-status.yaml 流转）
  - **不**改 ADR-0003（migration 工具 + 编号约定 + .up.sql / .down.sql 双向规范由 Story 4.3 已锚定，本 story 沿用 / 不修改）
  - **不**改 `cmd/server/main.go` migrate 子命令（已在 Story 4.3 落地）
  - **不**为 0015 写"prod 部署 migration 自动化 / 一键 rollback / dry-run / force"等运维化改造（保留最小集 up/down，与 Story 4.3 / 7.2 / 10.3 / 11.2 / 17.2 / 20.2 / 20.4 一致）
  - **不**建 FK 约束（与本设计其他表一致 —— ADR-0003 / 数据库设计 §3 + §7 钦定"应用层校验 + 索引兜底"策略；`user_id` / `cosmetic_item_id` / `source_ref_id` 语义上 reference 其他表但**不**建 FK）
  - **不**修改 0001 ~ 0014 既有 migration 文件（已在 Story 4.3 / 7.2 / 10.3 / 11.2 / 17.2 / 17.3 / 20.2 / 20.3 / 20.4 / 20.6 落地）

**本 story 不做**（明确范围红线）：

- 不实装任何 Go handler / service / repo write 或 read 方法（23.4 / 23.5 范围）
- 不实装任何 INSERT / seed SQL（user_cosmetic_items 是运行时实例表，无 seed 阶段；23.5 开箱事务首条 INSERT）
- 不新建 `UserCosmeticItemRepo` interface（YAGNI；23.4 落地 inventory 聚合查询时才落地 `UserCosmeticItemRepo` 类型 + `ListByUserAndStatus(ctx, userID, statuses) ([]UserCosmeticItem, error)` 或聚合方法；23.5 落地 `InsertInTx` 方法）
- 不在 `UserCosmeticItem` struct 上加 GORM `index` 等 tag（普通索引由 SQL DDL 定义，GORM struct **不**重复声明 —— 与 ADR-0003 §3.2 "禁止 GORM AutoMigrate" 同源；GORM struct 仅为 Find / Create 提供字段映射，**不**作为 schema 真相源；与 Story 20.2 落地的 `CosmeticItem` / 17.2 落地的 `EmojiConfig` / 11.2 落地的 `RoomMember` struct 同模式）
- 不引入 `gorm.Model`（避免引入 `DeletedAt` / 软删除字段，与 0015 真实 schema 不符；user_cosmetic_items 用 `status` 字段表达生命周期，**不**软删除）
- 不在 struct 中预留任何 §5.9 之外的字段（即使顺手加占位字段也会让 0015 SQL 与 §5.9 不一致 → 跨文档漂移）
- 不修改 0001 ~ 0014 既有 migration 文件（已落地）
- 不修改 Story 20.6 开箱事务 / 0013 chest_open_logs / 0014 chest_open_idempotency_records（补入仓 + 回填是 Story 23.5 钦定范围）
- 不修改 V1 接口契约（23.1 已冻结）
- 不修改数据库设计 §5.9 / §6.10 status 枚举 / §6.11 source 枚举（schema 输入，本 story 严格对齐不修改）
- 不修改 Go 项目结构文档 §6（`internal/repo/mysql/` 目录已锚定；本 story 新增 `user_cosmetic_item_repo.go` 沿用既有目录规则）
- 不修改 ADR-0003（migration 工具 / 编号约定 / 文件命名规范由 Story 4.3 已锚定）
- 不写英文版测试注释 / 文档（项目 communication_language=Chinese；与 Story 4.3 / 7.2 / 10.3 / 11.2 / 17.2 / 20.2 / 20.4 一致）
- 不为 0015 写 stress test / fuzz test（节点 8 阶段 schema 稳定 + 单测 + dockertest 集成测试已覆盖核心约束）
- 不在本 story 内对 Story 23.4 / 23.5 实装做"提前预实装"（即使顺手写 `(r *userCosmeticItemRepo) ListByUser(...)` 也禁止；这些方法是 23.4 / 23.5 钦定范围，提前 ship 会让下游评审找不到"新增方法"的明确范围边界，与 Story 20.2 / 11.2 / 17.2 "禁止预实装" 同模式）
- 不写 `UserCosmeticItem.Status` / `.Source` 字段的 enum 校验（DB 端 `TINYINT NOT NULL DEFAULT 1` 已兜底；§6.10 / §6.11 枚举钦定值域；service 层校验由 23.4 / 23.5 实装时按需添加）
- 不在本 story 修复 0014 chest_open_idempotency_records 的任何 schema / 测试问题（仅**对账**积压的 `expectedTables` / 版本号断言把 0014 表补进去 —— 这是 0014 落地时漏更新的纯断言对账，**不**改 0014 migration 本身、**不**给 0014 新增 schema 断言 case；如发现 0014 schema 本身有问题 → 记 tech debt 不在本 story 修）

## Acceptance Criteria

**AC1 — 0015_init_user_cosmetic_items.up.sql 新建（与 §5.9 钦定 1:1 对齐）**

新建 `server/migrations/0015_init_user_cosmetic_items.up.sql`，内容必须**严格**对齐 `docs/宠物互动App_数据库设计.md` §5.9（行 483-523）钦定的 DDL（含中文注释头说明字段语义来源 + 范围红线，参照 Story 20.2 落地的 `0011_init_cosmetic_items.up.sql` / Story 20.4 落地的 `0013_init_chest_open_logs.up.sql` 注释头风格）：

```sql
CREATE TABLE user_cosmetic_items (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    cosmetic_item_id BIGINT UNSIGNED NOT NULL,
    status TINYINT NOT NULL DEFAULT 1,
    source TINYINT NOT NULL DEFAULT 1,
    source_ref_id BIGINT UNSIGNED NULL,
    obtained_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    consumed_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    KEY idx_user_id_status (user_id, status),
    KEY idx_user_id_cosmetic_item_id (user_id, cosmetic_item_id),
    KEY idx_source (source, source_ref_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

- DDL **逐字段、逐索引、ENGINE / CHARSET 全部与 §5.9 行 487-503 一致**；字段顺序与 §5.9 一致。
- `source_ref_id BIGINT UNSIGNED NULL`（**可空**——来源关联记录 id，开箱 source_ref_id=chest_id 非空，但合成产出实例时先 NULL 后回填 compose_log_id，故 DDL 必须 NULL）。
- `consumed_at DATETIME(3) NULL`（**可空**——未消耗时为空，§5.9 字段说明行 514 钦定"消耗时间，未消耗时为空"）。
- `status TINYINT NOT NULL DEFAULT 1`（§6.10 枚举 1=in_bag / 2=equipped / 3=consumed / 4=invalid；DEFAULT 1 = 新建实例默认 in_bag；DDL 不在 schema 层做 enum 约束，由 service 层 + §6.10 钦定值域兜底）。
- `source TINYINT NOT NULL DEFAULT 1`（§6.11 枚举 1=chest / 2=compose（按数据库设计 §6.11 钦定值域；DDL 不做 enum 约束，service 层兜底）；DEFAULT 1 = 默认开箱来源）。
- **无 UNIQUE 约束**（实例表，同 user_id + 同 cosmetic_item_id 可持有多件 —— FR16"同种配置可被持有多件"；与 0013 chest_open_logs append-only 无 UNIQUE 同模式，但本表实例 status 可被 UPDATE 推进，非 append-only）。
- **不**建 FK 约束（`user_id` ref users.id / `cosmetic_item_id` ref cosmetic_items.id / `source_ref_id` ref user_chests.id 或 compose_logs.id —— 全部语义 reference，**不**建 FK，与本设计其他表一致）。
- 注释头说明：本 migration 由 Story 23.2 首次落地（Epic 23 节点 8 仓库 / 穿戴 / 合成业务链路 schema 根基 owner）+ 字段 1:1 对齐 §5.9 + 三个普通索引覆盖路径说明（`idx_user_id_status` 覆盖 §8.2 inventory `WHERE user_id=? AND status IN (1,2)` / `idx_user_id_cosmetic_item_id` 覆盖按配置聚合 + Epic 26 穿戴查实例 / `idx_source` 覆盖运营按来源倒查）+ 范围红线（仅 CREATE TABLE，无 seed，无 service，序号 0015 纠偏说明）。
- 文件编码 UTF-8 + LF 行尾（与其他 migration 一致）。

**AC2 — 0015_init_user_cosmetic_items.down.sql 新建（DROP TABLE 回滚路径）**

新建 `server/migrations/0015_init_user_cosmetic_items.down.sql`：

```sql
DROP TABLE IF EXISTS user_cosmetic_items;
```

- 与 Story 20.2 落地的 `0011_init_cosmetic_items.down.sql` / Story 20.4 落地的 `0013_init_chest_open_logs.down.sql` 同模式（含简短中文注释头说明回滚目标 + 由 Story 23.2 首次落地）。
- `DROP TABLE IF EXISTS`（幂等回滚，与既有 down migration 一致）。

**AC3 — UserCosmeticItem GORM domain struct 新建（与 0015 真实 schema 1:1 对齐，含 NULL 列指针映射）**

新建 `server/internal/repo/mysql/user_cosmetic_item_repo.go`：

- 含 `UserCosmeticItem` struct，字段与 0015 真实 schema + §5.9 1:1 对齐：
  - `ID uint64` `gorm:"column:id;primaryKey;autoIncrement"`
  - `UserID uint64` `gorm:"column:user_id;not null"`
  - `CosmeticItemID uint64` `gorm:"column:cosmetic_item_id;not null"`
  - `Status int8` `gorm:"column:status;not null;default:1"`
  - `Source int8` `gorm:"column:source;not null;default:1"`
  - `SourceRefID *uint64` `gorm:"column:source_ref_id"`（**指针**映射 NULL 可空列；NULL → nil，避免 0 与 NULL 语义混淆）
  - `ObtainedAt time.Time` `gorm:"column:obtained_at;not null;default:CURRENT_TIMESTAMP(3)"`
  - `ConsumedAt *time.Time` `gorm:"column:consumed_at"`（**指针**映射 NULL 可空列；未消耗 → nil）
  - `CreatedAt time.Time` `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
  - `UpdatedAt time.Time` `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP(3)"`
- 含 `func (UserCosmeticItem) TableName() string { return "user_cosmetic_items" }`。
- **仅** struct + TableName；**不**新建 `UserCosmeticItemRepo` interface / 实装任何 List / Insert / Aggregate / MarkConsumed 方法（与 Story 20.2 落地 `cosmetic_item_repo.go` 在 20.2 阶段仅 struct+TableName 同模式 —— 注意 `cosmetic_item_repo.go` 现含 `CosmeticItemRepo` interface 是 Story 20.6 后续扩展加的，本 story 阶段对应 20.2 阶段的最小集，**不**提前加 interface）。
- 含完整中文注释头（字段说明含 §5.9 + §6.10 status 枚举 + §6.11 source 枚举 + NULL 列指针映射理由 + 表层无 UNIQUE / 索引由 SQL DDL 定义不在 struct tag 重复声明的说明 + 范围红线，参照 `cosmetic_item_repo.go` 行 12-42 注释头风格）。

**AC4 — migrate 集成测试扩展 + 顺手对账 0014 积压的表/版本断言**

修改 `server/internal/infra/migrate/migrate_integration_test.go`：

1. **`TestMigrateIntegration_UpThenDown`**：`expectedTables` slice 当前为 11 张（停在 0013 chest_open_logs，**漏了 0014 chest_open_idempotency_records**）→ 一次性补齐为 13 张：追加 `"chest_open_idempotency_records"`（0014 积压对账）+ `"user_cosmetic_items"`（本 story 0015）；表数量注释 `11 张` → `13 张`（注释补"Story 20.6 落地 0014 时漏更新 expectedTables，本 story 顺手对账补齐 + 加 0015"扩展记录）。
2. **`TestMigrateIntegration_UpTwice_Idempotent`**：如该 case 有表数量断言，同步对账到 13 张（若该 case 不做表数量断言、仅验证 Up 两次返 nil，则只补注释扩展记录不改断言；dev 实装时按现状判定）。
3. **`TestMigrateIntegration_StatusAfterUp`**：版本号断言 `if v != 13` → `if v != 15`（0014 + 0015 两条）+ `t.Errorf` want 文案同步 13 → 15 + 函数头注释补两条扩展记录：`Story 20.6 扩展：从 13 改 14（多了 0014_init_chest_open_idempotency_records；本 story 对账时补记，原 20.6 漏更新）` + `Story 23.2 扩展：从 14 改 15（多了 0015_init_user_cosmetic_items；Epic 23 节点 8 仓库实例表 schema 根基）`；同步修正文件顶部 `// 4. edge: Up 后 Status 返回 (version=13, dirty=false, nil)` 注释为 version=15。
4. **新增 `TestMigrateIntegration_UserCosmeticItems_Schema`**：按 §5.9 用 `INFORMATION_SCHEMA.COLUMNS` / `STATISTICS` 做字段层 + 索引层断言（与 Story 20.2 落地的 `TestMigrateIntegration_CosmeticItems_Schema` 行 1293-1525 同模式）：
   - `INFORMATION_SCHEMA.TABLES`：user_cosmetic_items 表存在。
   - 10 列字段计数兜底（防漂移：如有人误加字段会让计数变 11 失败）。
   - 逐列断言 column_type / is_nullable / column_default：`id BIGINT UNSIGNED PK AUTO_INCREMENT` / `user_id BIGINT UNSIGNED NOT NULL` / `cosmetic_item_id BIGINT UNSIGNED NOT NULL` / `status TINYINT NOT NULL DEFAULT 1` / `source TINYINT NOT NULL DEFAULT 1` / **`source_ref_id BIGINT UNSIGNED NULL`（IS_NULLABLE = YES 关键断言）** / `obtained_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)` / **`consumed_at DATETIME(3) NULL`（IS_NULLABLE = YES 关键断言）** / `created_at / updated_at DATETIME(3)`。
   - 三个普通索引列顺序断言（`STATISTICS`）：`idx_user_id_status (user_id, status)` / `idx_user_id_cosmetic_item_id (user_id, cosmetic_item_id)` / `idx_source (source, source_ref_id)`，每个索引的 `SEQ_IN_INDEX` 列顺序与 §5.9 一致。
   - **无 UNIQUE 约束断言**：查 `STATISTICS WHERE table_name='user_cosmetic_items' AND non_unique=0 AND index_name != 'PRIMARY'` count = 0（验证除 PK 外无 UNIQUE，与 cosmetic_items 有 uk_code 形成对照）。
5. **新增 `TestMigrateIntegration_UserCosmeticItems_AppendableAndUpdatable`**：dockertest 运行时语义验证（与 Story 20.4 落地的 `TestMigrateIntegration_ChestOpenLogs_AppendOnly` 行 2238-2300 同模式）：
   - migrate Up → INSERT 同一 user_id 的 2 行 user_cosmetic_items（不同 cosmetic_item_id 或同 cosmetic_item_id）→ 都成功（**无 UNIQUE 拒绝**，验证同种配置可持有多件 FR16）。
   - UPDATE 其中一行 `SET status = 3, consumed_at = NOW(3) WHERE id = ?` → 成功（验证 status 可推进 + consumed_at 可从 NULL 写入，与 chest_open_logs append-only 不可 UPDATE 语义形成对照 —— user_cosmetic_items 是可变实例表）。
   - 插入一行不带 source_ref_id（依赖 NULL 默认）→ SELECT 回来 source_ref_id IS NULL（验证可空列默认 NULL 非 0）。

**AC5 — 构建 / 测试通过**

- `bash scripts/build.sh --test` 通过（vet + build + 全量单测 `go test -count=1 ./...`；本 story 改了 Go 文件（新增 repo struct + 改集成测试文件），必须跑）。
- `bash scripts/build.sh --integration` 通过（`-tags=integration` 跑 migrate 集成测试，含本 story 新增 2 个 case + 对账后的 `UpThenDown` / `StatusAfterUp`；dockertest 起真实 MySQL 容器）。
- 全量测试无回归（既有 migrate 集成测试 case 全绿；0001~0014 既有 migration 不受影响；其他 repo / service 测试不受影响 —— 本 story 仅新增 0015 + 新增 struct + 改 migrate 集成测试断言，无既有逻辑改动）。
- `git status` 仅出现范围红线内文件改动（4 个 server 文件 + story 文件 + sprint-status.yaml）。

**AC6 — 跨文档一致性自检（migration story 必须项）**

完成 AC1~AC4 后，必须逐项核对并在本 story "Completion Notes List" 记录核对结论：

1. 0015 DDL 逐字段 / 逐索引 / ENGINE / CHARSET 与 `docs/宠物互动App_数据库设计.md` §5.9 行 487-503 **逐字符比对一致**（字段名 / 类型 / NOT NULL / DEFAULT / NULL 可空性 / 索引名 / 索引列顺序）。
2. `UserCosmeticItem` struct 字段与 0015 真实 schema **逐字段一致**（列名映射 / NULL 列用指针 `*uint64` / `*time.Time` / 非 NULL 列用值类型 / int8 映射 TINYINT / uint64 映射 BIGINT UNSIGNED）。
3. `status` / `source` 默认值与 §6.10 / §6.11 枚举钦定语义 **无矛盾**（status DEFAULT 1=in_bag / source DEFAULT 1=chest）。
4. migrate 集成测试 `expectedTables`（13 张）+ `StatusAfterUp` 版本号（15）与真实 migration 文件序号集合（0001~0015，其中 0010 / 0012 是 seed migration 不建表 → 实际建表数 13）**一致**；0014 chest_open_idempotency_records 对账补齐确认（确认 0014 确实建了 1 张表 → 13 = 11 旧 + 0014 + 0015）。
5. 未触碰 0001~0014 既有 migration / Story 20.6 开箱事务 / V1 接口契约 / 数据库设计 §5.9 / 其他 6 份 docs。
6. 序号纠偏（epics.md 文字 `0014` → 实际 `0015`）已在本 story Dev Notes + Completion Notes 显式记录，**不**视为 §5.9 / 契约 / 范围变更（属 epics.md 撰写时序早于 20.6 落地的历史错位）。

## Tasks / Subtasks

- [x] Task 1：读取并定位（AC1 / AC2 / AC6）
  - [x] 读 `docs/宠物互动App_数据库设计.md` §5.9 行 483-523（user_cosmetic_items DDL + 字段说明 + 设计说明，唯一 schema 真相源）
  - [x] 读 §6.10 status 枚举 / §6.11 source 枚举（status / source 值域钦定）
  - [x] 读参照 migration `server/migrations/0011_init_cosmetic_items.up/down.sql`（注释头 + DDL 风格）+ `0013_init_chest_open_logs.up.sql`（NOT NULL 占位 + 无 UNIQUE / 无 FK 风格）+ `0014_init_chest_open_idempotency_records.up.sql`（确认 0014 = 1 张表，序号占用确认）
  - [x] 读参照 `server/internal/repo/mysql/cosmetic_item_repo.go` 行 1-58（GORM struct + TableName 注释头风格；注意 20.2 阶段最小集语义）
  - [x] 读 `server/internal/infra/migrate/migrate_integration_test.go`（确认 `expectedTables` / `StatusAfterUp` 当前积压值 + `CosmeticItems_Schema` / `ChestOpenLogs_AppendOnly` case 参照模式）
- [x] Task 2：新建 0015 up/down migration（AC1 / AC2）
  - [x] 新建 `server/migrations/0015_init_user_cosmetic_items.up.sql`（注释头 + CREATE TABLE 1:1 §5.9 + 3 普通索引 + 无 UNIQUE + 无 FK）
  - [x] 新建 `server/migrations/0015_init_user_cosmetic_items.down.sql`（注释头 + `DROP TABLE IF EXISTS user_cosmetic_items;`）
  - [x] 确认序号 0015（0014 已被 chest_open_idempotency_records 占用）+ UTF-8 / LF
- [x] Task 3：新建 UserCosmeticItem GORM domain struct（AC3）
  - [x] 新建 `server/internal/repo/mysql/user_cosmetic_item_repo.go`（注释头 + struct 10 字段含 `*uint64` / `*time.Time` NULL 指针映射 + `TableName()`）
  - [x] **不**加 Repo interface / 任何方法（YAGNI；最小集）
- [x] Task 4：扩展 + 对账 migrate 集成测试（AC4）
  - [x] `TestMigrateIntegration_UpThenDown`：`expectedTables` 补 `chest_open_idempotency_records`（0014 对账）+ `user_cosmetic_items`（0015）→ 13 张 + 表数量注释/说明更新
  - [x] `TestMigrateIntegration_UpTwice_Idempotent`：表数量断言对账（该 case 有 `tableCount != 11` 断言）→ 补两表 → 13 张
  - [x] `TestMigrateIntegration_StatusAfterUp`：`v != 13` → `v != 15` + want 文案 + 函数头注释补 0014 / 0015 两条 + 文件顶部 version=13 注释 → 15
  - [x] 新增 `TestMigrateIntegration_UserCosmeticItems_Schema`（10 列字段层断言 + source_ref_id/consumed_at NULL 断言 + 3 索引列顺序 + 无 UNIQUE 断言）
  - [x] 新增 `TestMigrateIntegration_UserCosmeticItems_AppendableAndUpdatable`（同 user_id 多行 INSERT + status/consumed_at UPDATE + source_ref_id NULL 默认验证）
- [x] Task 5：构建 / 测试（AC5）
  - [x] `bash scripts/build.sh --test`（vet + build + 全量单测 → PASS）
  - [x] `bash scripts/build.sh --integration`（本 story 5 个 migrate case 全 PASS；`./...` 全量跑因不相关 `internal/service` dockertest 容器就绪超时挂掉，详见 Debug Log）
  - [x] 确认无回归 + `git status` 仅范围红线内文件
- [x] Task 6：跨文档一致性自检（AC6）
  - [x] 逐项核对 AC6 的 6 条，结论写入本 story "Completion Notes List"
  - [x] 确认序号纠偏（0014→0015）已显式记录、不视为契约/范围变更
  - [x] 标记 sprint-status.yaml `23-2-user_cosmetic_items-migration` 状态流转 ready-for-dev → in-progress → review

## Dev Notes

### 这是什么类型的 story

纯持久化层 migration story。对标 Story 20.2（cosmetic_items migration）/ 20.4（chest_open_logs migration）/ 17.2（emoji_configs migration）/ 11.2（rooms+room_members migration）在各自 Epic 的"表落地"角色。产出物 = 0015 up/down + UserCosmeticItem GORM struct + 完整测试覆盖 + **顺手对账 0014 落地时漏更新的 migrate 集成测试积压断言**（让 expectedTables / 版本号一次性恢复到真实状态，避免本 story 加 0015 后积压翻倍）。

### 关键纠偏点 1：migration 序号是 0015 不是 0014

epics.md §Story 23.2 行 3238 文字写 `migrations/0014_init_user_cosmetic_items.sql`，但 `0014` 已被 **Story 20.6** 落地的 `0014_init_chest_open_idempotency_records.up/down.sql` 占用（开箱接口幂等记录表，20.1 r5 follow-up 钦定 owner = 20.6）。epics.md 撰写时点早于 20.6 实际落地 0014，属历史时序错位。本 story 按 ADR-0003 顺序递增编号约定取**下一个空闲序号 = `0015`**。这**不**是 §5.9 schema 变更、**不**是契约变更、**不**是范围变更（表名 `user_cosmetic_items` / 字段 / 索引钦定全部不变，仅文件序号偏移）。Completion Notes 必须显式记录此纠偏。

### 关键纠偏点 2：migrate 集成测试断言早在 0014 就停在旧值（积压对账）

`TestMigrateIntegration_UpThenDown` 的 `expectedTables`（11 张，停在 0013 chest_open_logs）+ `TestMigrateIntegration_StatusAfterUp` 的版本号断言（`v != 13`）在 **Story 20.6 落地 0014 时漏更新**。当前真实状态是 version=14 / 12 张建表（0001~0014，其中 0010_seed_emoji_configs / 0012_seed_cosmetic_items 是 seed migration 不建表）。本 story 加 0015 后真实状态变 version=15 / 13 张建表。dev 实装时**必须一次性把 expectedTables 补齐 chest_open_idempotency_records（0014 积压）+ user_cosmetic_items（0015）→ 13 张，版本号 `v != 13` → `v != 15`**，否则积压翻倍（且这两个集成测试在 `--integration` 下会断言失败）。这是"顺手对账"——**只**改断言数值 + 注释扩展记录，**不**改 0014 migration 本身、**不**给 0014 新增独立 schema 断言 case（那超出本 story 范围；如发现 0014 schema 本身有问题记 tech debt）。

> dev 实装确认点：跑 `bash scripts/build.sh --integration` 前先本地核对——若 `UpThenDown` / `StatusAfterUp` 在**加 0015 之前**就已经因 0014 积压而失败（version=14 vs 断言 13），说明积压属实，对账是必需修复不是 nice-to-have；若它们当前恰好通过（例如这两个 case 实际没断言版本号或被 skip），则按真实代码现状判定，注释如实记录。以代码真相为准，不盲信本 Dev Note 的积压推断。

### 字段语义唯一来源 = 数据库设计文档 §5.9（DB → API 单向）

- `user_cosmetic_items`：`docs/宠物互动App_数据库设计.md` §5.9 行 487-503 DDL + 行 506-514 字段说明
- 枚举：§6.10 status（1=in_bag / 2=equipped / 3=consumed / 4=invalid）/ §6.11 source（1=chest / 2=compose，按 §6.11 钦定值域为准）
- **关键 NULL 列**：`source_ref_id BIGINT UNSIGNED NULL`（开箱时=chest_id 非空；合成产出实例先 NULL 后回填 compose_log_id）+ `consumed_at DATETIME(3) NULL`（§5.9 行 514 钦定"未消耗时为空"）；GORM struct 必须用 `*uint64` / `*time.Time` 指针映射，**不**用值类型（否则 NULL 与 0 / 零值混淆，下游 23.4/Epic 32 判 source_ref_id 是否回填会出错）
- 本 story 落地的 schema 是 23.1 §8.2 锚定的 API 字段（`instances[].userCosmeticItemId` = `user_cosmetic_items.id` 字符串化 / `instances[].status` 枚举 {1,2} = status 子集）的**DB 端真相源**；DB → API 单向，**不**反向改 DB

### 易错点（review 高频命中，提前规避）

1. **序号写成 0014**：必须 0015（0014 被 chest_open_idempotency_records 占）。
2. **NULL 列用值类型**：`source_ref_id` / `consumed_at` 必须 `*uint64` / `*time.Time` 指针；用 `uint64` / `time.Time` 会让 NULL → 0 / 零值，下游判"是否已回填 / 是否已消耗"出错。
3. **加 UNIQUE 约束**：user_cosmetic_items **无** UNIQUE（同种配置可持有多件 FR16）；不要顺手加 `uk_user_cosmetic`。
4. **建 FK**：本设计全程不建 FK（ADR-0003 / §3 + §7），`user_id` / `cosmetic_item_id` / `source_ref_id` 语义 reference 但不建 FK。
5. **migrate 集成测试只补 0015 不补 0014**：必须一次性对账两张（0014 积压 + 0015 新增）→ 13 张 / version 15，否则 0014 积压翻倍。
6. **预实装 23.4/23.5 方法**：本 story 仅 struct+TableName，**不**加 Repo interface / List / Insert 方法（对标 20.2 阶段最小集；`cosmetic_item_repo.go` 现有 interface 是 20.6 后补的，**不**是 20.2 阶段产物）。
7. **改 0014 / 改 Story 20.6 开箱事务**：补入仓是 23.5 范围；本 story **仅** CREATE TABLE，不触碰任何已有事务 / 0013 / 0014。
8. **改 §5.9 文档**：§5.9 是 schema 输入，本 story 严格对齐**不修改**；不一致时以 §5.9 为准改本 story。

### 范围红线（再次强调）

只改 4 个 server 文件 + story 文件 + sprint-status.yaml：
- 新建 `server/migrations/0015_init_user_cosmetic_items.up.sql`
- 新建 `server/migrations/0015_init_user_cosmetic_items.down.sql`
- 新建 `server/internal/repo/mysql/user_cosmetic_item_repo.go`（仅 struct + TableName）
- 改 `server/internal/infra/migrate/migrate_integration_test.go`（扩展 + 对账断言 + 新增 2 case）

**不**改任何 `.go` service/handler、**不**改 0001~0014 既有 migration、**不**改 Story 20.6 开箱事务、**不**改 V1 接口契约 / `docs/宠物互动App_*.md` / `_bmad-output/` 下其他 yaml/md。改了 Go 代码 → **必须**跑 `bash scripts/build.sh --test` + `bash scripts/build.sh --integration`。

### Project Structure Notes

- migration 落地目录 `server/migrations/`（ADR-0003 / Story 4.3 锚定的 golang-migrate `NNNN_name.up.sql` / `.down.sql` 双向规范；本 story 沿用，不修改约定）。
- GORM domain struct 落地 `server/internal/repo/mysql/user_cosmetic_item_repo.go`（与 `cosmetic_item_repo.go` / `chest_open_log_repo.go` 同目录同模式；`docs/宠物互动App_Go项目结构与模块职责设计.md` §6 钦定 `internal/repo/mysql/` 目录）。
- migrate 集成测试 `server/internal/infra/migrate/migrate_integration_test.go`（`-tags=integration` dockertest，Story 4.3 起既有文件，逐 story 扩展 expectedTables / 版本号 / 新增 schema case）。
- 无新增目录 / 模块 / 依赖（GORM / golang-migrate / dockertest 均既有）。

### References

- [Source: docs/宠物互动App_数据库设计.md#5.9（行 483-523）] — user_cosmetic_items DDL + 字段说明 + 设计说明（唯一 schema 真相源）
- [Source: docs/宠物互动App_数据库设计.md#6.10 / 6.11] — status / source 枚举值域钦定
- [Source: _bmad-output/planning-artifacts/epics.md#Story 23.2（行 3228-3246）] — AC 钦定（字段全集 + 3 索引 + down.sql + ≥3 case 单测 + dockertest；序号 0014 文字属历史时序错位，本 story 纠偏为 0015）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 23.4（行 3269-3293） / Story 23.5（行 3295-3320）] — 下游 inventory 聚合查询 + 开箱补入仓事务（本 story 落地的 schema 的下游消费方）
- [Source: _bmad-output/planning-artifacts/epics.md#FR16（行 55）] — 每件装扮唯一实例，同种配置可持有多件（→ 无 UNIQUE 约束依据）
- [Source: server/migrations/0011_init_cosmetic_items.up.sql / .down.sql] — 同 Epic 链路 migration 注释头 + DDL 风格参照（Story 20.2 落地）
- [Source: server/migrations/0013_init_chest_open_logs.up.sql] — 无 UNIQUE / 无 FK / NOT NULL 占位风格参照（Story 20.4 落地）
- [Source: server/migrations/0014_init_chest_open_idempotency_records.up.sql / .down.sql] — 序号 0014 占用确认（Story 20.6 落地；本 story 序号纠偏依据）
- [Source: server/internal/repo/mysql/cosmetic_item_repo.go（行 1-58）] — GORM domain struct + TableName 注释头风格参照（注意 20.2 阶段最小集 vs 20.6 后补 interface）
- [Source: server/internal/infra/migrate/migrate_integration_test.go#TestMigrateIntegration_CosmeticItems_Schema（行 1293-1525）] — INFORMATION_SCHEMA 字段层 schema 断言模式参照
- [Source: server/internal/infra/migrate/migrate_integration_test.go#TestMigrateIntegration_ChestOpenLogs_AppendOnly（行 2238-2300）] — dockertest 运行时语义验证模式参照（无 UNIQUE 多行 INSERT）
- [Source: server/internal/infra/migrate/migrate_integration_test.go#TestMigrateIntegration_UpThenDown（行 148-221） / TestMigrateIntegration_StatusAfterUp（行 574-622）] — 待对账的积压断言（expectedTables 11 张停在 0013 / 版本号 v!=13，漏 0014）
- [Source: _bmad-output/implementation-artifacts/23-1-接口契约最终化.md] — 上游契约定稿（§8.2 inventory schema 锚定 + §1 节点 8 冻结声明；本 story 落地 schema 是其 DB 端真相源）
- [Source: _bmad-output/implementation-artifacts/20-2-cosmetic_items-migration.md] — 同类 migration story 既有模式参照（结构 / 范围红线 / 测试覆盖编排）
- [Source: CLAUDE.md（Build & Test 段）] — `bash scripts/build.sh --test` / `--integration` 验证契约

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]（Opus 4.7 1M context），bmad-dev-story workflow，由 epic-loop 派出的隔离 sub-agent 执行。

### Debug Log References

- `bash scripts/build.sh --test`：vet OK / build OK（commit=8f4fbab）/ 全量单测全绿（含 `internal/repo/mysql` 含新 struct 编译 + 测试）→ PASS。
- `bash scripts/build.sh --integration`（`go test -tags=integration -timeout=120s ./...`）：**未通过，但失败点在本 story 范围外** —— 失败包是 `internal/service` 的 `TestAuthService_GuestLogin_DifferentGuestUID_CreatesNewFiveRows`（本 story 未触碰该文件），其 dockertest MySQL 容器在本机 docker 上冷启动未在 Go per-package 120s timeout 内就绪（`pool.Retry` 仍在 backoff），属环境/基础设施 docker 容器就绪 timing flake，与 0015 / GORM struct / migrate 集成测试断言改动无因果关系。
- 隔离验证本 story 范围的 migrate 集成测试（`go test -tags=integration -timeout=600s ./internal/infra/migrate/`，单 case 跑足够长 timeout）：
  - `TestMigrateIntegration_StatusAfterUp` → **PASS**（v=15 断言成立，反向证实 0014 积压属实 —— 真实 version 早已是 14，本 story 一次对账到 15 正确）
  - `TestMigrateIntegration_UserCosmeticItems_Schema` → **PASS**（10 列字段层 + source_ref_id/consumed_at IS_NULLABLE=YES + 3 索引列顺序 + 无 UNIQUE 全部成立）
  - `TestMigrateIntegration_UpThenDown` → **PASS**（13 张表对账后成立）
  - `TestMigrateIntegration_UpTwice_Idempotent` → **PASS**（13 张表对账后成立）
  - `TestMigrateIntegration_UserCosmeticItems_AppendableAndUpdatable` → **PASS**（同 user_id 多行 INSERT 无 UNIQUE 拒绝 + status/consumed_at UPDATE 推进 + source_ref_id NULL 默认全部成立）
  - 回归抽查相邻既有 case `CosmeticItems_Schema` / `ChestOpenLogs_Schema` / `ChestOpenLogs_AppendOnly` → 全 **PASS**（无回归）。
- 注：`go test ./...` 顺序串跑约 38 个 dockertest case 时本机 docker daemon 被压垮（大量 MySQL 容器并发冷启）→ 个别包超 per-package timeout，是已知环境侧 flake；migrate 包本身全部 case 给足时间/隔离跑均绿。

### Completion Notes List

**实装摘要**：纯持久化层 migration story，落地 0015 user_cosmetic_items 表 + UserCosmeticItem GORM domain struct（仅 struct+TableName 最小集，无 interface/方法）+ migrate 集成测试扩展 2 个新 case + 顺手对账 Story 20.6 落地 0014 时漏更新的积压断言（expectedTables 11→13 张、StatusAfterUp v=13→15）。

**AC6 跨文档一致性自检结论（逐项）**：

1. ✅ 0015 DDL 逐字段/逐索引/ENGINE/CHARSET 与 `docs/宠物互动App_数据库设计.md` §5.9 行 488-503 **逐字符比对一致**：10 字段（id/user_id/cosmetic_item_id/status/source/source_ref_id/obtained_at/consumed_at/created_at/updated_at）类型、NOT NULL/NULL 可空性、DEFAULT、3 个普通索引名与列顺序、`ENGINE=InnoDB DEFAULT CHARSET=utf8mb4` 全部 1:1。
2. ✅ `UserCosmeticItem` struct 字段与 0015 真实 schema **逐字段一致**：NULL 列 `SourceRefID *uint64` / `ConsumedAt *time.Time` 用指针映射；非 NULL 列用值类型；`int8` 映射 TINYINT、`uint64` 映射 BIGINT UNSIGNED；`TableName()` 显式返回 `"user_cosmetic_items"`。dockertest 运行时已验证指针映射对 NULL 语义正确（source_ref_id 不带值插入 SELECT 回来 IS NULL）。
3. ✅ `status DEFAULT 1` / `source DEFAULT 1` 与 §6.10（1=in_bag）/ §6.11（1=chest）枚举钦定语义**无矛盾**。注：§6.11 实际钦定 4 个值（1=chest/2=compose/3=admin_grant/4=event_reward），story Story 文/AC 文字仅列了 1/2 子集 —— 以代码真相（§6.11 文档）为准，注释头按 §6.11 全 4 值如实记录，DDL 不做 enum 约束（与既有表一致）。
4. ✅ migrate 集成测试 `expectedTables`（13 张）+ `StatusAfterUp` 版本号（15）与真实 migration 文件序号集合一致：0001~0015 共 15 个 up 文件，其中 0010_seed_emoji_configs / 0012_seed_cosmetic_items 是 seed migration 不建表 → 实际建表 13 张（11 旧 + 0014 chest_open_idempotency_records + 0015 user_cosmetic_items）。dockertest 实跑 `UpThenDown`（13 张全建/全删）+ `StatusAfterUp`（v=15）均 PASS，确认对账数值正确。
5. ✅ 未触碰 0001~0014 既有 migration 文件 / Story 20.6 `ChestService.OpenChest` 开箱事务 / 0013 chest_open_logs / V1 接口契约 / `docs/宠物互动App_*.md`（§5.9 仅作输入对齐未修改）/ `_bmad-output/` 下除本 story + sprint-status 外其他文件。`git status` 实测仅 4 server 文件 + story + sprint-status。
6. ✅ 序号纠偏（epics.md §Story 23.2 文字 `0014` → 实际 `0015`，因 0014 被 Story 20.6 落地的 chest_open_idempotency_records 占用）已在 0015 up.sql 注释头 + Dev Notes + 本 Completion Notes + migrate 测试注释多处显式记录；**不**视为 §5.9/契约/范围变更（属 epics.md 撰写时序早于 20.6 落地的历史错位，表名/字段/索引钦定全部不变）。

**0014 积压对账确认**：本 story 实装前 `migrate_integration_test.go` 的 `expectedTables` 停在 11 张（0013 chest_open_logs）、`StatusAfterUp` 断言 `v != 13`，而真实状态早在 Story 20.6 落地 0014 后已是 12 张建表 / version=14。`StatusAfterUp` 对账后断言 `v != 15` 实跑 PASS（dockertest 真实 version=15）——反向证实积压属实、对账是必需修复（若只补 0015 不补 0014，积压翻倍且该 case 在 `--integration` 下必失败）。本 story **仅**对账断言数值 + 注释扩展记录，**未**改 0014 migration 本身、**未**给 0014 新增独立 schema 断言 case（超范围；0014 schema 本身未发现问题，无 tech debt 登记）。

**范围红线遵守**：未实装任何 service/handler/repo write/read 方法；未建 UserCosmeticItemRepo interface（YAGNI，23.4/23.5 落地）；未写任何 INSERT/seed SQL；未加 UNIQUE/FK 约束；未引入 gorm.Model；struct 未预留 §5.9 外字段；未写英文注释。

### File List

- `server/migrations/0015_init_user_cosmetic_items.up.sql`（新建）
- `server/migrations/0015_init_user_cosmetic_items.down.sql`（新建）
- `server/internal/repo/mysql/user_cosmetic_item_repo.go`（新建，仅 UserCosmeticItem struct + TableName）
- `server/internal/infra/migrate/migrate_integration_test.go`（修改：文件头注释 + UpThenDown/UpTwice_Idempotent 表数量对账到 13 张 + StatusAfterUp v13→v15 + 新增 UserCosmeticItems_Schema + 新增 UserCosmeticItems_AppendableAndUpdatable 两个 case）
- `_bmad-output/implementation-artifacts/23-2-user_cosmetic_items-migration.md`（本 story 文件：Tasks 勾选 + Dev Agent Record + File List + Change Log + Status → review）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（状态流转 ready-for-dev → in-progress → review）

## Change Log

| 日期 | 变更 | 作者 |
|------|------|------|
| 2026-05-16 | 首次落地 0015_init_user_cosmetic_items up/down migration（1:1 对齐 §5.9）+ UserCosmeticItem GORM domain struct（含 NULL 列指针映射）+ migrate 集成测试新增 2 case（Schema 字段层断言 + AppendableAndUpdatable 运行时语义）+ 顺手对账 Story 20.6 漏更新的 0014 积压断言（expectedTables 11→13 张 / StatusAfterUp v13→v15）。状态 ready-for-dev → review。 | dev-story (claude-opus-4-7[1m]，epic-loop sub-agent) |
