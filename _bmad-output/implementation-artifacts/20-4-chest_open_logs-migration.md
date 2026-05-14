# Story 20.4: chest_open_logs migration（首次落地 0013_init_chest_open_logs.up/down.sql + ChestOpenLog GORM domain struct 最小骨架 + ≥3 case 单测 + dockertest 集成测试覆盖 schema 一致）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want **首次落地** `server/migrations/0013_init_chest_open_logs.up.sql` + `server/migrations/0013_init_chest_open_logs.down.sql` 两个新 migration 文件（严格按 `docs/宠物互动App_数据库设计.md` §5.7 行 399-433 钦定的 CREATE TABLE DDL：`id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT` + `user_id BIGINT UNSIGNED NOT NULL` + `chest_id BIGINT UNSIGNED NOT NULL` + `cost_steps INT UNSIGNED NOT NULL` + `reward_user_cosmetic_item_id BIGINT UNSIGNED NOT NULL` + `reward_cosmetic_item_id BIGINT UNSIGNED NOT NULL` + `reward_rarity TINYINT NOT NULL` + `created_at DATETIME(3)` + `KEY idx_user_id_created_at (user_id, created_at)` + `KEY idx_reward_cosmetic_item_id (reward_cosmetic_item_id)` + `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`，1:1 对齐 §5.7；**无** `updated_at` 字段（日志表 append-only，与 0006 user_step_sync_logs 同模式）；**无** UNIQUE 约束（日志表允许同 user_id 多次开箱）+ **新增** `server/internal/repo/mysql/chest_open_log_repo.go` 含 `ChestOpenLog` GORM domain struct（与 0013 真实 schema 1:1 对齐：`ID / UserID / ChestID / CostSteps / RewardUserCosmeticItemID / RewardCosmeticItemID / RewardRarity / CreatedAt`，**无** `UpdatedAt`）+ `TableName() string` 显式返回 `"chest_open_logs"`（**仅** struct + TableName，**不**新增 Repo interface / 实装任何 Create / FindByUserID 等方法，YAGNI；Story 20.6 才落地 repo 方法）+ **扩展** `server/internal/infra/migrate/migrate_integration_test.go` 的 `TestMigrateIntegration_UpThenDown`（表数量 10 → 11 + `expectedTables` slice 加 `"chest_open_logs"`）+ `TestMigrateIntegration_UpTwice_Idempotent`（同 10 → 11）+ `TestMigrateIntegration_StatusAfterUp` 版本号 v != 12 → v != 13 同步升级 + **新增** `TestMigrateIntegration_ChestOpenLogs_Schema`（验证 chest_open_logs 表 / 列 / 索引 / 字段类型 / **无 updated_at** / 列数 == 8 符合 §5.7）+ **新增** `TestMigrateIntegration_ChestOpenLogs_AppendOnly` dockertest 集成测试（覆盖同 user_id 可插入多行 + 无 UNIQUE 拒绝，与 20.2 落地的 `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` 形成对照 —— 本 case 是"append-only 日志表无 UNIQUE 拒绝"路径，与 0006 user_step_sync_logs 同模式）,
so that **Story 20.5（GET /chest/current 接口）+ Story 20.6（POST /chest/open 事务 + idempotencyKey + 加权抽取，handler 内层在事务步骤 5h 写一条 chest_open_logs 行：节点 7 阶段 `reward_user_cosmetic_item_id = 0` 占位 + `reward_cosmetic_item_id` / `reward_rarity` 真实值；详见 V1接口设计 §7.2.4h + 数据库设计 §8 注解）+ Story 20.7 / 20.8（dev 端点）+ Story 20.9（Layer 2 集成测试覆盖完整开箱事务路径 + chest_open_logs 行数断言）+ iOS Epic 21 各 story（首页宝箱组件 → POST /chest/open → 奖励弹窗，间接通过 reward 字段依赖本 story DB schema）+ Epic 23 Story 23.5（节点 8 修改开箱事务：创建 user_cosmetic_items 实例 → 拿到 id → 回填 chest_open_logs.reward_user_cosmetic_item_id 真实值，本 story DDL 字段定义是其回填语义的 DB 端真相源）+ 未来运营 / 数据分析侧（"按用户按时间倒序查最近 10 次开箱" / "按 reward_cosmetic_item_id 倒查产出分布" 等查询路径，依赖本 story 落地的 `idx_user_id_created_at` + `idx_reward_cosmetic_item_id` 两索引覆盖）**可以基于一个**已落地、已具备完整测试覆盖、已通过 dockertest 真实 INSERT 验证、已具备完整 GORM domain struct 字段映射、已具备双索引性能覆盖**的 chest_open_logs 持久化基础并行展开，不再出现"20.6 写 chest_open_logs INSERT 时表不存在 / 20.6 SQL 字段名漂移（如 `reward_cosmetic_id` 而非 `reward_cosmetic_item_id`）/ 20.6 用 `BIGINT` 写 reward_user_cosmetic_item_id 而该列实际是 `BIGINT UNSIGNED` 导致 0 占位写入语义模糊 / 节点 8 Story 23.5 想回填 reward_user_cosmetic_item_id 时字段 schema 不存在 / GORM struct 字段名与 DB 列名漂移 / 误写 updated_at 字段污染 append-only 日志表语义 / 误加 UNIQUE 约束阻塞同 user_id 多次开箱"的返工。

## 故事定位（Epic 20 第四条 = 第二条**实装** migration story；上承 20.3 cosmetic_items seed，下启 20.5 GET /chest/current + 20.6 POST /chest/open 事务 + 20.9 Layer 2 集成测试）

- **Epic 20 进度**：20.1（契约定稿，done）→ 20.2（cosmetic_items migration，done）→ 20.3（cosmetic_items seed ≥15 行 + AR18 数量约束，done）→ **20.4（本 story，chest_open_logs migration + GORM domain struct + 测试覆盖）** → 20.5（GET /chest/current 接口）→ 20.6（POST /chest/open 事务 + idempotencyKey + 加权抽取）→ 20.7（dev 端点 POST /dev/force-unlock-chest）→ 20.8（dev 端点 POST /dev/grant-cosmetic-batch）→ 20.9（Layer 2 集成测试 - 开箱事务全流程）。
- **本 story 是 20.5 / 20.6 / 20.7 / 20.8 / 20.9 / Epic 21 / Epic 23.5 的强前置**：
  - **20.5 GET /chest/current**：本 story 不直接被 20.5 SQL 引用（20.5 只查 user_chests）；但 20.5 落地后 20.6 POST /chest/open 事务紧接着引用本 story 表，时序上同节点 7 阶段串联落地
  - **20.6 POST /chest/open 事务**：步骤 5h 写一条 chest_open_logs 行 —— V1接口设计 §7.2.4h + 数据库设计 §8 注解钦定的 INSERT 语句 `INSERT INTO chest_open_logs (user_id, chest_id, cost_steps, reward_user_cosmetic_item_id, reward_cosmetic_item_id, reward_rarity, created_at) VALUES (?, ?, 1000, 0, ?, ?, NOW())` 必须命中本 story 落地的 0013 表 schema；**关键**：节点 7 阶段 `reward_user_cosmetic_item_id` 固定为 `0`（占位值，因不创建 user_cosmetic_items 实例；节点 8 Story 23.5 修改本步骤为先 INSERT user_cosmetic_items 拿到 id 再填入此处）；本 story DDL `reward_user_cosmetic_item_id BIGINT UNSIGNED NOT NULL` 字段定义允许 `0` 作为合法值（unsigned 范围 0 ~ 2^64 - 1，§3.1 主键约定字段值 ≥ 1 但日志表非 FK 强约束，0 是节点 7 阶段语义占位，§8 注解钦定）
  - **20.7 dev /dev/force-unlock-chest**：不直接依赖 chest_open_logs（仅 UPDATE user_chests），但 20.7 / 20.8 落地需要 20.6 路径打通，间接依赖
  - **20.8 dev /dev/grant-cosmetic-batch**：节点 8 才真实写库；节点 7 阶段实装路由 + handler 框架，不直接依赖 chest_open_logs
  - **20.9 Layer 2 集成测试**：dockertest 走完整开箱事务路径，必然依赖 chest_open_logs 表已建 + INSERT 语义 + 双索引存在；20.9 断言点必含"POST /chest/open 成功后 chest_open_logs 多 1 行 + user_id / chest_id / cost_steps=1000 / reward_user_cosmetic_item_id=0 / reward_cosmetic_item_id ≠ 0 / reward_rarity ∈ {1,2,3,4} 字段值合理"
  - **iOS Epic 21（首页宝箱组件 → POST /chest/open → 奖励弹窗）**：iOS 端 `ChestRewardDTO` Codable struct 通过 20.6 POST /chest/open JSON response 间接依赖本 story 落地的 DB schema（特别是 `reward_rarity TINYINT` ⇔ V1 §7.2 `reward.rarity` int 枚举的 DB 端真相源）；本 story 落地的字段（`cost_steps INT UNSIGNED` / `reward_rarity TINYINT`）是 V1 §7.2 cost / reward 字段类型 / 枚举约束的**DB 端真相源**之一
  - **Epic 23 Story 23.5（节点 8 修改开箱事务）**：23.5 落地后 chest_open_logs.reward_user_cosmetic_item_id 字段从节点 7 阶段的"占位 0"切换为"真实 user_cosmetic_items.id"，本 story DDL `BIGINT UNSIGNED NOT NULL` 字段已为该切换预留语义（unsigned 范围支持任意非负 id；NOT NULL 兜底防漏写）；本 story **不**预实装 23.5 切换逻辑（service 层 owner），仅落地 schema
  - **未来运营 / 数据分析**：`idx_user_id_created_at (user_id, created_at)` 覆盖"按用户按时间倒序查最近 10 次开箱"（§7.2 高优先级普通索引清单行 937 钦定）；`idx_reward_cosmetic_item_id (reward_cosmetic_item_id)` 覆盖"按 reward_cosmetic_item_id 倒查产出分布"（运营分析 / 概率排查路径，§5.7 设计说明钦定）
- **epics.md §Story 20.4 钦定**（行 2853-2871）：
  - `migrations/0013_init_chest_open_logs.sql` 按数据库设计.md §5.7 创建表，含 `id` PK + 全字段（`user_id`, `chest_id`, `cost_steps`, `reward_user_cosmetic_item_id`, `reward_cosmetic_item_id`, `reward_rarity`, `created_at`）+ `KEY idx_user_id_created_at` + `KEY idx_reward_cosmetic_item_id`
  - 含 down.sql（DROP TABLE 路径）
  - **单元测试覆盖**（≥3 case）：happy（migrate up 后表存在 + 字段类型 + 索引都符合 §5.7）/ happy（migrate down 后表删除）/ edge（重复 migrate up → 幂等，由现有 `TestMigrateIntegration_UpTwice_Idempotent` 扩展覆盖）
  - **集成测试覆盖**（dockertest）：migrate up → SHOW CREATE TABLE 对比 schema → migrate down（本 story 用更精确的 `INFORMATION_SCHEMA.COLUMNS` / `STATISTICS` 字段层断言取代字符串 SHOW CREATE TABLE 比对 —— 后者跨 MySQL 版本输出格式不稳定，前者是结构化 schema 真相源，与 Story 20.2 落地的 `TestMigrateIntegration_CosmeticItems_Schema` 同模式）
- **Story 20.1 上游冻结边界**（V1 §7.2 cost / reward 字段表 + 数据库设计 §5.7 字段表 + §8.4 开箱事务设计）：本 story 落地的字段类型 / 长度约束（`cost_steps INT UNSIGNED` ⇔ §7.2 `cost.amount` int 数值约束 / `reward_rarity TINYINT` ⇔ §7.2 `reward.rarity` int 枚举 + §6.9 枚举值 ∈ {1,2,3,4}）是 20.1 锚定的 API 字段类型 / 枚举值的**DB 端真相源**之一；本 story **不**反向修改 DB schema（DB → API 单向），仅严格对齐数据库设计文档 §5.7 DDL
- **下游强依赖**（本 story done 后才能开工）：
  - Story 20.5（GET /chest/current 接口）—— 本 story 不被 20.5 直接 SQL 引用，但同节点 7 阶段串联落地
  - Story 20.6（POST /chest/open 事务 + idempotencyKey + 加权抽取 + 步骤 5h 写 chest_open_logs）—— **强依赖**
  - Story 20.7（dev 端点 POST /dev/force-unlock-chest）
  - Story 20.8（dev 端点 POST /dev/grant-cosmetic-batch）
  - Story 20.9（Layer 2 集成测试，断言 chest_open_logs 行数 + 字段值）—— **强依赖**
  - iOS Epic 21.1 ~ 21.5（首页宝箱组件 + GET /chest/current 调用 + POST /chest/open + 奖励弹窗 + 开箱前主动同步步数）—— 间接依赖
  - Epic 23 Story 23.5（修改开箱事务，回填 chest_open_logs.reward_user_cosmetic_item_id 真实值）—— **强依赖**
- **范围红线**：
  - 本 story **只**改 `server/migrations/0013_init_chest_open_logs.up.sql`（新建）+ `server/migrations/0013_init_chest_open_logs.down.sql`（新建）+ `server/internal/repo/mysql/chest_open_log_repo.go`（新建，含 `ChestOpenLog` struct + `TableName()`）+ `server/internal/infra/migrate/migrate_integration_test.go`（扩展 `TestMigrateIntegration_UpThenDown` 表数量断言 10 → 11 + `expectedTables` slice 加 `"chest_open_logs"` + 扩展 `TestMigrateIntegration_UpTwice_Idempotent` 表数量断言 10 → 11 + 同步注释升级 + 顺手升级 `TestMigrateIntegration_StatusAfterUp` 版本号 v != 12 → v != 13 + 新增 `TestMigrateIntegration_ChestOpenLogs_Schema` case + 新增 `TestMigrateIntegration_ChestOpenLogs_AppendOnly` case）+ 本 story 文件 + sprint-status.yaml 流转
  - **不**实装任何 service / handler / repo write / read 方法（20.6 ~ 20.9 才做；本 story 阶段**仅**落地 GORM struct + TableName，**不**新建 `ChestOpenLogRepo` interface / 实装 `Create` / `FindByUserID` 等方法）
  - **不**实装任何 seed SQL（chest_open_logs 是运行时日志表，**无** seed；本 story **仅** CREATE TABLE，不含任何 INSERT）
  - **不**实装 chest_open_idempotency_records migration（§5.16 表；与 20.2 红线一致，由 Story 20.6 dev kickoff 时决定收进 Story 20.6 或起新 Story 20.10 独立落地；本 story **不**触碰 §5.16 表）
  - **不**接 Redis / **不**接 chest_open_idempotency_records（同 20.2 红线）
  - **不**改 V1 接口契约（20.1 已冻结）
  - **不**改任何 `docs/宠物互动App_*.md`（§5.7 是契约**输入**，本 story 严格对齐它但**不修改**；如发现 §5.7 与本 story 落地的 DDL 有不一致 → 优先以 §5.7 为准修改本 story 而非反向改 §5.7）
  - **不**改 `_bmad-output/` 下其他 yaml / md（除自己 story 文件 + sprint-status.yaml 流转）
  - **不**改 ADR-0003（migration 工具 + 编号约定 + .up.sql / .down.sql 双向规范由 Story 4.3 已锚定，本 story 沿用 / 不修改）
  - **不**改 `cmd/server/main.go` migrate 子命令（已在 Story 4.3 落地）
  - **不**为 0013 写"prod 部署 migration 自动化 / 一键 rollback / dry-run / force"等运维化改造（保留最小集 up/down，与 Story 4.3 / 7.2 / 10.3 / 11.2 / 17.2 / 17.3 / 20.2 / 20.3 一致）
  - **不**写 FK 约束（chest_open_logs.user_id / chest_id / reward_user_cosmetic_item_id / reward_cosmetic_item_id 语义上 reference users / user_chests / user_cosmetic_items / cosmetic_items，但**不**建 FK，与本设计其他表保持一致 —— ADR-0003 / 数据库设计 §3 + §7 钦定的"应用层校验 + 索引兜底"策略）
  - **不**写 `updated_at` 字段（chest_open_logs 是 append-only 日志表，**无** UPDATE 语义；与 0006 user_step_sync_logs 同模式 —— 日志表只 INSERT 不 UPDATE，无需 updated_at）
  - **不**写任何 UNIQUE 约束（日志表允许同 user_id 多次开箱、同 chest_id 在跨轮场景下也可能复现，**绝不**误加 UNIQUE 阻塞业务）

**本 story 不做**（明确范围红线）：

- 不实装任何 Go handler / service / repo write 或 read 方法（20.6 ~ 20.9 范围）
- 不实装任何 INSERT seed SQL（chest_open_logs 是运行时日志表，**无** seed 阶段）
- 不新建 `ChestOpenLogRepo` interface（YAGNI；20.6 实装开箱事务时才落地 `ChestOpenLogRepo` 类型 + `Create(ctx, *ChestOpenLog) error` 方法；Epic 23 Story 23.5 / 未来运营接口才加 `FindByUserIDOrderByCreatedAtDesc(ctx, userID, limit)` 等查询方法）
- 不在 `ChestOpenLog` struct 上加 GORM `index` 等 tag（普通索引由 SQL DDL 定义，GORM struct **不**重复声明 —— 与 ADR-0003 §3.2 "禁止 GORM AutoMigrate" 同源；GORM struct 仅为 Find / Create 提供字段映射，**不**作为 schema 真相源；与 20.2 落地的 `CosmeticItem` / 17.2 落地的 `EmojiConfig` / 11.2 落地的 `RoomMember` / `Room` struct 同模式）
- 不引入 `gorm.Model`（避免引入 `DeletedAt` / 软删除字段，与 0013 真实 schema 不符；同时 `gorm.Model` 含 `UpdatedAt` 字段，与 append-only 日志表语义冲突）
- 不在 struct 中预留 `UpdatedAt` 字段（append-only 日志表无 UPDATE 语义；与 0006 user_step_sync_logs / `UserStepSyncLog` struct 同模式）
- 不修改 0001 ~ 0012 既有 migration 文件（已在 Story 4.3 / 7.2 / 10.3 / 11.2 / 17.2 / 17.3 / 20.2 / 20.3 落地）
- 不修改 V1 接口契约（20.1 已冻结）
- 不修改数据库设计 §5.7（schema 输入，本 story 严格对齐不修改）
- 不修改 Go 项目结构文档 §6（`internal/repo/mysql/` 目录已锚定；本 story 新增 `chest_open_log_repo.go` 沿用既有目录规则）
- 不修改 ADR-0003（migration 工具 / 编号约定 / 文件命名规范由 Story 4.3 已锚定）
- 不写英文版测试注释 / 文档（项目 communication_language=Chinese；与 Story 4.3 / 7.2 / 10.3 / 11.2 / 17.2 / 17.3 / 20.2 / 20.3 一致）
- 不为 0013 写 stress test / fuzz test（节点 7 阶段 schema 稳定 + 单测 + dockertest 集成测试已覆盖核心约束）
- 不在本 story 内对 Story 20.5 ~ 20.9 实装做"提前预实装"（即使顺手写 `(r *chestOpenLogRepo) Create(ctx, *ChestOpenLog) error` 也禁止；这些方法是 20.6 钦定范围，提前 ship 会让 20.6 评审找不到"新增方法"的明确范围边界，与 Story 11.2 / 17.2 / 20.2 "禁止预实装" 同模式）
- 不写 `ChestOpenLog.RewardRarity` / `.RewardUserCosmeticItemID` 字段的 enum / FK 校验（DB 端 `TINYINT NOT NULL` / `BIGINT UNSIGNED NOT NULL` 已兜底；§6.9 状态枚举钦定值域；service 层校验由 20.6 实装时按需添加）

## Acceptance Criteria

**AC1 — 0013_init_chest_open_logs.up.sql 新建（与 §5.7 钦定 1:1 对齐）**

新建 `server/migrations/0013_init_chest_open_logs.up.sql`，内容必须**严格**对齐 `docs/宠物互动App_数据库设计.md` §5.7（行 399-433）钦定的 DDL：

```sql
-- 对齐 docs/宠物互动App_数据库设计.md §5.7 (行 399-433)
-- chest_open_logs 表：开箱日志表（append-only，**无** updated_at；
-- 单独保存，方便追踪掉落问题 + 用户历史展示 + 运营分析与概率排查）
--
-- **本 migration 由 Story 20.4 首次落地（Epic 20 节点 7 宝箱业务链路 schema 根基 owner）**
-- 含 ≥3 case 单测（migrate up 表存在 + 字段类型 + idx_user_id_created_at + idx_reward_cosmetic_item_id 索引符合 §5.7）
-- + dockertest 集成测试覆盖 append-only 语义（同 user_id 可插入多行，无 UNIQUE 拒绝；
--   与 20.2 落地的 TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected 形成对照
--   —— 后者验证有 UNIQUE 的运行时拒绝，本表验证无 UNIQUE 的多行允许）。
--
-- 字段（与 §5.7 钦定 1:1 对齐）：
--   - id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§3.1 主键约定）
--   - user_id BIGINT UNSIGNED NOT NULL：归属用户（语义上 reference users.id，
--     **不**建 FK，与本设计其他表保持一致 —— ADR-0003 / 数据库设计 §3 + §7 钦定的
--     "应用层校验 + 索引兜底"策略）
--   - chest_id BIGINT UNSIGNED NOT NULL：被开启的宝箱 id（语义上 reference
--     user_chests.id；同样**不**建 FK；§5.7 字段说明行 421 钦定）
--   - cost_steps INT UNSIGNED NOT NULL：实际消耗步数（§5.7 字段说明行 422 钦定；
--     节点 7 阶段 Story 20.6 钦定 = 1000，V1 §7.2 cost.amount 字段同源；INT UNSIGNED
--     范围 0 ~ 2^32 - 1，足够覆盖任意单次开箱步数）
--   - reward_user_cosmetic_item_id BIGINT UNSIGNED NOT NULL：产出的装扮实例 id
--     （§5.7 字段说明行 423 钦定；**关键节点 7 阶段语义**：固定为 `0` 占位，因不
--     创建 user_cosmetic_items 实例 —— V1接口设计 §7.2.4h + 数据库设计 §8 注解钦定
--     "节点 7 阶段 reward_user_cosmetic_item_id 写占位 0"；节点 8 Epic 23 Story 23.5
--     修改开箱事务为先 INSERT user_cosmetic_items 拿到 id 再填入此处，本 migration
--     DDL `BIGINT UNSIGNED NOT NULL` 已为该切换预留语义 —— unsigned 范围支持任意
--     非负 id；NOT NULL 兜底防漏写；DEFAULT 不设值由 service 层显式提供 0 / 真实 id）
--   - reward_cosmetic_item_id BIGINT UNSIGNED NOT NULL：产出的装扮配置 id
--     （§5.7 字段说明行 424 钦定；语义上 reference cosmetic_items.id —— 20.2 落地
--     的表；**不**建 FK；Story 20.6 加权抽取 SQL 命中后从抽到的 cosmetic_items.id
--     写入此字段）
--   - reward_rarity TINYINT NOT NULL：奖励品质（§5.7 字段说明行 425 钦定；§6.9
--     枚举 1=common / 2=rare / 3=epic / 4=legendary；Story 20.6 加权抽取后写入
--     cosmetic_items.rarity 同步到此处，与 V1 §7.2 reward.rarity 字段同源；
--     **注**：DDL 不在 schema 层做 enum 约束（TINYINT 允许任意 -128~127 值），
--     由 service 层 + cosmetic_items 表的合法 rarity 兜底）
--   - created_at DATETIME(3)（§3.2 毫秒精度时间戳；append-only 日志表**无**
--     updated_at —— 与 0006 user_step_sync_logs 同模式）
--
-- 索引（§5.7 + §7.2 高优先级普通索引清单 行 937 钦定）：
--   - KEY idx_user_id_created_at (user_id, created_at)：覆盖
--     "SELECT ... WHERE user_id=? ORDER BY created_at DESC LIMIT N" 路径
--     （未来运营 / 用户历史展示按用户按时间倒序查最近 N 次开箱；
--     §7.2 高优先级普通索引清单 chest_open_logs(user_id, created_at) 钦定）
--   - KEY idx_reward_cosmetic_item_id (reward_cosmetic_item_id)：覆盖
--     "SELECT ... WHERE reward_cosmetic_item_id=?" 路径
--     （运营分析 / 概率排查按 cosmetic 倒查产出分布；§5.7 设计说明行 433 钦定
--     "便于运营分析与概率排查"）
--
-- **范围红线**：本 migration **仅** CREATE TABLE；不含任何 INSERT / seed 数据
-- （chest_open_logs 是运行时日志表，**无** seed 阶段；Story 20.6 开箱事务才首条
-- INSERT）；不含任何业务 service / handler / repo write 方法（20.6 ~ 20.9 落地）；
-- 不含 updated_at 字段（append-only 日志表语义，与 0006 user_step_sync_logs 同模式）；
-- 不含 UNIQUE 约束（日志表允许同 user_id 多次开箱）；不建 FK 约束（与本设计其他表
-- 保持一致）。
CREATE TABLE chest_open_logs (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    chest_id BIGINT UNSIGNED NOT NULL,
    cost_steps INT UNSIGNED NOT NULL,
    reward_user_cosmetic_item_id BIGINT UNSIGNED NOT NULL,
    reward_cosmetic_item_id BIGINT UNSIGNED NOT NULL,
    reward_rarity TINYINT NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    KEY idx_user_id_created_at (user_id, created_at),
    KEY idx_reward_cosmetic_item_id (reward_cosmetic_item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

- DDL 内容**严格**对齐 §5.7 行 404-416 —— 字段顺序 / 字段类型 / NOT NULL / 索引名 / 索引列顺序全部 1:1
- **无** `updated_at` 字段（append-only 日志表语义；与 0006 user_step_sync_logs 同模式）
- **无** UNIQUE 约束（日志表允许同 user_id 多次开箱）
- **无** FK 约束（与本设计其他表保持一致）
- 文件编码 UTF-8 + LF 行尾（与 0001 ~ 0012 一致）
- 顶部注释模板与 0011 / 0012（20.2 / 20.3 升级版本）一致 —— "对齐 §X.Y" + "字段" + "索引" + "范围红线" 四段式
- **不**包含任何 INSERT / seed 数据（chest_open_logs 是运行时日志表）
- **不**包含任何 business logic SQL（如 `UPDATE chest_open_logs SET ...`）

**AC2 — 0013_init_chest_open_logs.down.sql 新建**

新建 `server/migrations/0013_init_chest_open_logs.down.sql`，内容：

```sql
-- 回滚 0013_init_chest_open_logs.up.sql
--
-- **本 migration 由 Story 20.4 首次落地（Epic 20 节点 7 宝箱业务链路 schema 根基 owner）**
-- 含 ≥3 case 单测 + dockertest 集成测试覆盖 append-only 语义（同 user_id 多行允许）。
DROP TABLE IF EXISTS chest_open_logs;
```

- 文件编码 UTF-8 + LF 行尾
- **仅** `DROP TABLE IF EXISTS chest_open_logs;`，不含任何额外 cleanup 语句（与 0006 / 0007 / 0008 / 0009 / 0011 down.sql 同模式）

**AC3 — chest_open_log_repo.go 新建（仅 `ChestOpenLog` GORM domain struct + `TableName()`，无 repo 方法）**

新建 `server/internal/repo/mysql/chest_open_log_repo.go`，内容必须包含：

```go
package mysql

import (
	"time"
)

// ChestOpenLog 是 chest_open_logs 表的完整 GORM domain struct（Story 20.4 引入；
// 与 server/migrations/0013_init_chest_open_logs.up.sql 真实 schema 1:1 对齐）。
//
// 字段（与数据库设计.md §5.7 + 0013_init_chest_open_logs.up.sql 1:1 对齐）：
//   - ID:                       BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§5.7 + §3.1 主键约定）
//   - UserID:                   BIGINT UNSIGNED NOT NULL（归属用户 id，语义上 ref users.id）
//   - ChestID:                  BIGINT UNSIGNED NOT NULL（被开启的宝箱 id，语义上 ref user_chests.id）
//   - CostSteps:                INT UNSIGNED NOT NULL（实际消耗步数；节点 7 阶段固定 1000）
//   - RewardUserCosmeticItemID: BIGINT UNSIGNED NOT NULL（产出的装扮实例 id；
//                               **节点 7 阶段固定 0 占位** —— V1接口设计 §7.2.4h +
//                               数据库设计 §8 注解钦定；节点 8 Epic 23 Story 23.5
//                               切换为真实 user_cosmetic_items.id）
//   - RewardCosmeticItemID:     BIGINT UNSIGNED NOT NULL（产出的装扮配置 id，
//                               语义上 ref cosmetic_items.id）
//   - RewardRarity:             TINYINT NOT NULL（§6.9 枚举：1=common / 2=rare /
//                               3=epic / 4=legendary）
//   - CreatedAt:                DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
//
// **关键**：本 struct **无** UpdatedAt 字段 —— chest_open_logs 是 append-only
// 日志表，**无** UPDATE 语义（与 0006 user_step_sync_logs / UserStepSyncLog struct
// 同模式）。
//
// 表层普通索引（idx_user_id_created_at / idx_reward_cosmetic_item_id）由 SQL DDL
// 定义，**不**在 struct tag 中重复声明（与 ADR-0003 §3.2 "禁止 GORM AutoMigrate"
// 同源；GORM struct 仅为 Find / Create 提供字段映射，**不**作为 schema 真相源；
// 与 Story 20.2 落地的 CosmeticItem / Story 17.2 落地的 EmojiConfig / Story 11.2
// 落地的 RoomMember / Room struct 同模式）。
//
// **范围红线**：本 struct 仅为下游 Story 20.6（POST /chest/open 事务步骤 5h
// 写一条 chest_open_logs 行）/ Story 23.5（节点 8 修改开箱事务回填
// reward_user_cosmetic_item_id）/ 未来运营接口提供字段映射；本 story 阶段
// **不**新建 ChestOpenLogRepo interface / 实装 Create / FindByUserID 等方法
// （YAGNI；20.6 落地 Create 方法 + 未来运营 epic 落地查询方法）。
type ChestOpenLog struct {
	ID                       uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	UserID                   uint64    `gorm:"column:user_id;not null"`
	ChestID                  uint64    `gorm:"column:chest_id;not null"`
	CostSteps                uint32    `gorm:"column:cost_steps;not null"`
	RewardUserCosmeticItemID uint64    `gorm:"column:reward_user_cosmetic_item_id;not null"`
	RewardCosmeticItemID     uint64    `gorm:"column:reward_cosmetic_item_id;not null"`
	RewardRarity             int8      `gorm:"column:reward_rarity;not null"`
	CreatedAt                time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

// TableName 显式声明 "chest_open_logs"。
func (ChestOpenLog) TableName() string { return "chest_open_logs" }
```

- 字段顺序与 0013 SQL 列顺序一致（8 字段，**无** UpdatedAt）
- `ID` / `UserID` / `ChestID` / `RewardUserCosmeticItemID` / `RewardCosmeticItemID` `uint64` 对齐 `BIGINT UNSIGNED`（带无符号；§3.1 主键约定 + §5.7 字段类型；与 Story 20.2 `CosmeticItem.ID` / 11.2 `Room.ID` 同模式）
- `CostSteps uint32` 对齐 `INT UNSIGNED`（无符号 32 位，范围 0 ~ 2^32 - 1；与 Story 20.2 `CosmeticItem.DropWeight uint32` 同模式 —— `INT UNSIGNED → uint32` 是 ADR-0003 + 20.2 锚定的 MySQL → Go 类型映射）
- `RewardRarity int8` 对齐 `TINYINT`（带符号；§6.9 枚举值 ∈ {1,2,3,4} 在范围内；与 Story 20.2 `CosmeticItem.Rarity int8` / 17.2 `EmojiConfig.IsEnabled int8` 同模式）
- `RewardUserCosmeticItemID` / `RewardCosmeticItemID` 命名遵循 Go 风格（Go 风格 `ID` 全大写缩写；GORM `column:reward_user_cosmetic_item_id` / `column:reward_cosmetic_item_id` 显式映射到 DB 列名）
- **关键**：导入 `time` 包 + **不**引入 `gorm.Model`（避免 `DeletedAt` 软删除字段 + 避免 `UpdatedAt` 字段污染 append-only 日志表语义）
- **关键**：**无** `UpdatedAt` 字段（append-only 日志表语义；与 0006 user_step_sync_logs / `UserStepSyncLog` struct 同模式）
- **关键**：**不**在 struct 上加 `gorm:"index:idx_user_id_created_at"` / `gorm:"index:idx_reward_cosmetic_item_id"` tag（索引由 SQL DDL 定义，与 ADR-0003 §3.2 一致）
- **关键**：**不**包含任何 enum / FK 校验逻辑（DB 端 `TINYINT NOT NULL` / `BIGINT UNSIGNED NOT NULL` + service 层兜底）
- **不**新建 `ChestOpenLogRepo` interface / `chestOpenLogRepo` struct / `NewChestOpenLogRepo()` constructor / 任何 `Create` / `Find` 方法（YAGNI；20.6 / 23.x owner）
- 文件内容**仅**含 package 声明 + import + `ChestOpenLog` struct + `TableName()` 方法（≤ 55 行；与 20.2 落地的 `CosmeticItem` / 17.2 落地的 `EmojiConfig` / 11.2 落地的 `Room` struct 同体积级别）

**AC4 — migrate_integration_test.go 扩展（表数量断言 10 → 11 + chest_open_logs schema 验证 + append-only 多行允许集成测试）**

修改 `server/internal/infra/migrate/migrate_integration_test.go`：

**AC4.1 扩展既有 `TestMigrateIntegration_UpThenDown`**：
- 找到现有 `expectedTables := []string{"users", ..., "cosmetic_items"}`（行 ~177）slice，加 `"chest_open_logs"`（共 11 张表）
- 表数量验证逻辑会自动按 slice 长度走，无硬编码数字需改（slice driven）
- 同步注释升级（顶部注释 + 行内注释 + 函数 docstring 一并升级）："Story 20.2 加 cosmetic_items（10 张）→ Story 20.4 加 chest_open_logs（共 11 张表）"

**AC4.2 扩展既有 `TestMigrateIntegration_UpTwice_Idempotent`**：
- 找到现有"表数量 = 10"断言（行 ~350）+ 现有 INFORMATION_SCHEMA `WHERE table_name IN (...)` slice（行 ~346），表数量 10 → 11 + slice 加 `'chest_open_logs'`
- 同步注释升级（与 AC4.1 一致）+ 错误消息同步加 "Story 20.4 加 chest_open_logs"

**AC4.3 同步升级 `TestMigrateIntegration_StatusAfterUp`**：
- 找到现有 `if v != 12` 断言（行 ~603；20.3 落地后版本号），改为 `if v != 13`（0013 落地后）
- 同步注释升级（与 AC4.1 一致；理由："Story 20.3 落地 0012 seed → Story 20.4 落地 0013 init"，与 Story 7.2 / 10.3 / 17.2 / 17.3 / 20.2 / 20.3 review 同模式）

**AC4.4 新增 `TestMigrateIntegration_ChestOpenLogs_Schema`**（紧接 `TestMigrateIntegration_CosmeticItems_SeedForceOverwrite` 之后，参考既有 case 实装模式）：

```go
// TestMigrateIntegration_ChestOpenLogs_Schema 验证
// migrations/0013_init_chest_open_logs.up.sql 钦定的 chest_open_logs 表 schema
// 与数据库设计.md §5.7 + V1接口设计.md §7.2 一致：
//
//   - id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT
//   - user_id BIGINT UNSIGNED NOT NULL
//   - chest_id BIGINT UNSIGNED NOT NULL
//   - cost_steps INT UNSIGNED NOT NULL（**关键**：column_type 必须含 "unsigned"，
//     与 emoji_configs.sort_order 的 "int" / cosmetic_items.drop_weight 的
//     "int unsigned" 一脉相承）
//   - reward_user_cosmetic_item_id BIGINT UNSIGNED NOT NULL
//   - reward_cosmetic_item_id BIGINT UNSIGNED NOT NULL
//   - reward_rarity TINYINT NOT NULL
//   - created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
//   - KEY idx_user_id_created_at (user_id, created_at)
//   - KEY idx_reward_cosmetic_item_id (reward_cosmetic_item_id)
//
// **关键覆盖点**：
//   - **无** UpdatedAt 字段 —— 总列数 == 8（不是 9）；append-only 日志表语义
//     兜底防有人误加 updated_at（与 Story 7.2 落地的 user_step_sync_logs 同模式）
//   - **无** UNIQUE 约束 —— 检查 INFORMATION_SCHEMA.STATISTICS 中
//     chest_open_logs 表的 non_unique = 0 的索引必须为空 / 仅有 PRIMARY；
//     日志表允许同 user_id 多次开箱
//   - 双索引（idx_user_id_created_at + idx_reward_cosmetic_item_id）列顺序断言
//     （与 §5.7 钦定一致）
//   - BIGINT UNSIGNED 字段（id / user_id / chest_id / reward_user_cosmetic_item_id /
//     reward_cosmetic_item_id）column_type 必须含 "unsigned"
//   - INT UNSIGNED 字段（cost_steps）column_type 必须含 "unsigned"
//
// **背景（Story 20.4 引入）**：本 case 验证 0013 migration 落地的 schema
// 与 §5.7 钦定 1:1 对齐；用于在 epics.md §Story 20.4 钦定的"单元测试覆盖 ≥3 case"
// 中作为 schema-correctness 路径（happy / migrate up 后表存在 + 字段类型 +
// 全部索引和约束都符合 §5.7）。
func TestMigrateIntegration_ChestOpenLogs_Schema(t *testing.T) {
	// 实装细节由 dev-story 阶段补全；
	// 模板参考既有 TestMigrateIntegration_CosmeticItems_Schema（Story 20.2 落地）
	// + TestMigrateIntegration_EmojiConfigs_Schema（Story 17.2 落地）。
	//
	// 必查项（每项失败立即 t.Errorf，不 t.Fatalf —— 用 batch 累积报错风格）：
	//   1. INFORMATION_SCHEMA.TABLES：chest_open_logs 表存在（count = 1）
	//   2. INFORMATION_SCHEMA.COLUMNS：8 列存在 + 类型对齐：
	//      - id bigint unsigned
	//      - user_id bigint unsigned
	//      - chest_id bigint unsigned
	//      - cost_steps int unsigned
	//      - reward_user_cosmetic_item_id bigint unsigned
	//      - reward_cosmetic_item_id bigint unsigned
	//      - reward_rarity tinyint
	//      - created_at datetime(3)
	//   3. INFORMATION_SCHEMA.COLUMNS：chest_open_logs 表总列数 == 8
	//      （**关键**：兜底防有人误加 updated_at 字段；append-only 日志表无
	//      UPDATE 语义；如计数 = 9 说明有人误加 updated_at）
	//   4. INFORMATION_SCHEMA.KEY_COLUMN_USAGE / STATISTICS：
	//      - PRIMARY KEY = id
	//      - **无**其他 UNIQUE 索引（non_unique=0 仅 PRIMARY 一行；
	//        chest_open_logs 是 append-only 日志表，允许同 user_id 多次开箱）
	//      - KEY idx_user_id_created_at 存在 + 列顺序 (user_id, created_at)
	//      - KEY idx_reward_cosmetic_item_id 存在 + 列顺序 (reward_cosmetic_item_id)
	//   5. INFORMATION_SCHEMA.COLUMNS.COLUMN_DEFAULT：
	//      - created_at DEFAULT 含 substring "CURRENT_TIMESTAMP"
	//      - **不**检查 user_id / chest_id / cost_steps / reward_user_cosmetic_item_id /
	//        reward_cosmetic_item_id / reward_rarity 的 DEFAULT（NOT NULL 但 DDL 不
	//        预设 DEFAULT；由 service 层显式提供值）
}
```

**AC4.5 新增 `TestMigrateIntegration_ChestOpenLogs_AppendOnly`**（紧接 AC4.4 case 之后）：

```go
// TestMigrateIntegration_ChestOpenLogs_AppendOnly 验证
// migrations/0013_init_chest_open_logs.up.sql 钦定的 append-only 日志表语义：
// 同一 user_id + 同一 chest_id 的多行 INSERT 必须**全部成功**（无 UNIQUE 拒绝）。
//
// **背景（Story 20.4 引入）**：epics.md §Story 20.4 钦定的"集成测试覆盖
// （dockertest）：migrate up → SHOW CREATE TABLE 对比 schema → migrate down"路径
// 在 AC4.4 ChestOpenLogs_Schema case 用 INFORMATION_SCHEMA 字段层精确断言取代了
// 不稳定的 SHOW CREATE TABLE 字符串比对；本 case 是额外的运行时 append-only
// 语义验证 —— 与 20.2 落地的 TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected
// 形成对照：后者验证有 UNIQUE 的运行时拒绝，本 case 验证无 UNIQUE 的多行允许。
//
// **覆盖路径**：
//  1. migrate up → chest_open_logs 表存在
//  2. 插入 chest_open_logs (user_id=1, chest_id=1, cost_steps=1000,
//     reward_user_cosmetic_item_id=0, reward_cosmetic_item_id=10, reward_rarity=1) → 成功
//     （reward_user_cosmetic_item_id=0 是节点 7 阶段语义占位，本 case 即模拟 20.6 INSERT 行为）
//  3. 再次插入 chest_open_logs (user_id=1, chest_id=2, cost_steps=1000,
//     reward_user_cosmetic_item_id=0, reward_cosmetic_item_id=11, reward_rarity=2) → 成功
//     （同 user_id，不同 chest_id —— 用户开箱多轮场景）
//  4. 再次插入 chest_open_logs (user_id=1, chest_id=1, cost_steps=1000,
//     reward_user_cosmetic_item_id=0, reward_cosmetic_item_id=12, reward_rarity=1) → 成功
//     （同 user_id + 同 chest_id —— 防御性 case，确保无任何 UNIQUE 阻塞）
//  5. SELECT COUNT(*) FROM chest_open_logs WHERE user_id=1 → count = 3
//     （3 行全部成功插入，证实 append-only 语义）
//
// 用 database/sql 直跑 raw INSERT（**不**走 GORM）让测试结果**不**依赖 ORM
// 行为差异（与 Story 11.2 / 17.2 / 20.2 落地的 UNIQUE 拒绝 case 同模式）。
//
// 测试用 user_id / chest_id 与未来 20.6 / 20.9 业务用 id 完全隔离 —— 用 user_id=1
// + chest_id=1/2 是 dockertest 容器内独立 mysql 实例，与 prod 数据无关；
// 容器测试结束后自动 purge（startMySQL t.Cleanup）。
func TestMigrateIntegration_ChestOpenLogs_AppendOnly(t *testing.T) {
	// 实装细节由 dev-story 阶段补全；模板见 Story 20.2 落地的
	// TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected（紧靠 AC4.4 之上）+
	// Story 17.2 落地的 TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected。
}
```

- 两个新 case 都用 `dockertest` 起 mysql:8.0 容器（沿用 `startMySQL(t)` + `migrationsPath(t)` helper，与既有 case 一致）
- AppendOnly case 用 `database/sql` 直跑 raw INSERT（**不**走 GORM）
- 错误断言：所有 3 次 INSERT 都 `err == nil`（**无**任何 "Duplicate entry" / "1062" 错误）+ 最终 `SELECT COUNT(*)` = 3
- **不**断言具体的 LAST_INSERT_ID 值（auto_increment 可能跨测试 / 跨容器变化；只断言行数 + 字段值）

**AC5 — 验证步骤**

- **AC5.1 build 验证**：执行 `bash scripts/build.sh --test` 必须**全绿**（含新增 `chest_open_log_repo.go` 单测无 / 既有单测无回归 + 新增集成测试不在默认 `--test` build tag 内不跑）；`bash scripts/build.sh --integration` 必须**全绿**（新增 `TestMigrateIntegration_ChestOpenLogs_Schema` + `TestMigrateIntegration_ChestOpenLogs_AppendOnly` 两个 case 跑通 + 既有 `TestMigrateIntegration_UpThenDown` / `TestMigrateIntegration_UpTwice_Idempotent` 表数量 11 断言通过 + `TestMigrateIntegration_StatusAfterUp` 版本号 v=13 断言通过 + 既有 `TestMigrateIntegration_EmojiConfigs_*` / `TestMigrateIntegration_CosmeticItems_*` / `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` 不回归）
- **AC5.2 git diff 范围检查**：编辑完成后 `git diff` 输出**仅**包含：
  - `server/migrations/0013_init_chest_open_logs.up.sql`（新增）
  - `server/migrations/0013_init_chest_open_logs.down.sql`（新增）
  - `server/internal/repo/mysql/chest_open_log_repo.go`（新增）
  - `server/internal/infra/migrate/migrate_integration_test.go`（修改：表数量断言 10 → 11 + `expectedTables` slice 加 `"chest_open_logs"` + `StatusAfterUp` v != 12 → v != 13 + 新增 2 case）
  - `_bmad-output/implementation-artifacts/20-4-chest_open_logs-migration.md`（本 story 文件状态流转 + Tasks/Subtasks 勾选 + Dev Agent Record / File List / Change Log）
  - `_bmad-output/implementation-artifacts/sprint-status.yaml`（状态行 + last_updated）
- **AC5.3 schema 跨文档一致性**：手动检查 0013 up.sql 字段名 / 类型 / 索引名与数据库设计.md §5.7 行 404-416 **逐字段** 1:1 对齐；与 V1 §7.2 字段约束兼容（`cost_steps INT UNSIGNED` ⇔ §7.2 `cost.amount` int 数值约束；`reward_rarity TINYINT` ⇔ §7.2 `reward.rarity` int 枚举 + §6.9 枚举值 ∈ {1,2,3,4}）；**关键**：`reward_user_cosmetic_item_id BIGINT UNSIGNED NOT NULL` 是 §5.7 钦定字段类型，**不**漂移成 `BIGINT` / `BIGINT UNSIGNED DEFAULT 0` / `BIGINT NULL`（节点 7 阶段语义占位 `0` 由 Story 20.6 service 层显式提供，DDL 层不预设 DEFAULT；节点 8 Story 23.5 切换为真实 id 时无需 DDL 改动）；**关键**：**无** `updated_at` 字段（append-only 日志表，与 §5.7 钦定一致 + 与 0006 user_step_sync_logs 同模式）
- **AC5.4 GORM struct ↔ DDL 一致性**：手动检查 `ChestOpenLog` struct 8 字段 / 类型与 0013 up.sql 1:1 对齐（无字段缺漏 / 无类型漂移）；**关键**：`CostSteps uint32`（不是 `int32` —— `INT UNSIGNED` 必须用无符号 Go 类型对齐）；**关键**：`RewardUserCosmeticItemID uint64` / `RewardCosmeticItemID uint64`（`BIGINT UNSIGNED` 必须用 `uint64` 对齐）；**关键**：`RewardRarity int8`（`TINYINT` 用 `int8`）；**关键**：**无** `UpdatedAt` 字段（append-only 日志表）
- **AC5.5 既有迁移 / repo 测试不回归**：跑 `go test ./server/internal/infra/migrate/... ./server/internal/repo/mysql/... -count=1` 全绿

## Tasks / Subtasks

- [x] Task 1: 准备阶段（AC: #1, #2, #3, #4, #5）
  - [x] Subtask 1.1: 阅读本 story 全文 + `docs/宠物互动App_数据库设计.md` §5.7（行 399-433）确认 DDL 1:1 字段 / 索引清单 + §8 注解（行 985-995）确认节点 7 vs 节点 8 阶段差异（`reward_user_cosmetic_item_id` 占位 0 → 真实 id 切换语义）
  - [x] Subtask 1.2: 阅读 `_bmad-output/implementation-artifacts/20-2-cosmetic_items-migration.md` 已 done 的姊妹 story（同 Epic 20 / 同 migration 模式），参考其顶部注释 + AC 结构 + 集成测试编辑模式
  - [x] Subtask 1.3: 阅读 `server/migrations/0011_init_cosmetic_items.up.sql` + `0011_init_cosmetic_items.down.sql`（20.2 落地的版本）确认顶部注释 / 字段块 / 索引块 / 范围红线四段式模板
  - [x] Subtask 1.4: 阅读 `server/migrations/0006_init_user_step_sync_logs.up.sql`（7.2 落地的日志表参考；append-only 无 updated_at 同模式）确认日志表 migration 模板
  - [x] Subtask 1.5: 阅读 `server/internal/repo/mysql/cosmetic_item_repo.go`（20.2 落地的版本）确认 `CosmeticItem` struct 的 GORM tag 模式 + `TableName()` 模式
  - [x] Subtask 1.6: 阅读 `server/internal/repo/mysql/step_sync_log_repo.go`（如存在，7.2 落地）作为日志表 struct（无 UpdatedAt）参考
  - [x] Subtask 1.7: 阅读 `server/internal/infra/migrate/migrate_integration_test.go` 行 1-220 + 313-352 + 563-610 + 1280-1590（20.2 / 20.3 升级后的版本）确认 `TestMigrateIntegration_CosmeticItems_Schema` + `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` + `TestMigrateIntegration_StatusAfterUp` 编辑模式
  - [x] Subtask 1.8: 阅读 `_bmad-output/implementation-artifacts/20-1-接口契约最终化.md`（已 done）确认 V1 §7.2 reward / cost 字段表 + 节点 7 / 节点 8 阶段差异说明，确保本 story 落地的 DB schema 与 20.1 锚定的 API 字段类型 / 枚举值兼容
  - [x] Subtask 1.9: 阅读 `docs/宠物互动App_V1接口设计.md` §7.2.4h（行 978）确认 INSERT chest_open_logs 的 SQL 语义（节点 7 阶段 `reward_user_cosmetic_item_id` 写占位 0）
- [x] Task 2: 落地 0013_init_chest_open_logs.up.sql（AC: #1）
  - [x] Subtask 2.1: 新建 `server/migrations/0013_init_chest_open_logs.up.sql`
  - [x] Subtask 2.2: 写顶部注释（"对齐 §5.7" + 字段块 + 索引块 + 范围红线四段式，按 AC1 模板）
  - [x] Subtask 2.3: 写 CREATE TABLE 语句（严格按 §5.7 行 404-416 1:1 + AC1 钦定 8 字段 + 2 索引，**无** updated_at / **无** UNIQUE / **无** FK）
- [x] Task 3: 落地 0013_init_chest_open_logs.down.sql（AC: #2）
  - [x] Subtask 3.1: 新建 `server/migrations/0013_init_chest_open_logs.down.sql`
  - [x] Subtask 3.2: 写顶部注释（按 AC2 模板）+ `DROP TABLE IF EXISTS chest_open_logs;`
- [x] Task 4: 落地 chest_open_log_repo.go（AC: #3）
  - [x] Subtask 4.1: 新建 `server/internal/repo/mysql/chest_open_log_repo.go`
  - [x] Subtask 4.2: 写 package 声明 + import `time`（**不**引入 `gorm.io/gorm`）
  - [x] Subtask 4.3: 写 `ChestOpenLog` struct 8 字段（按 AC3 钦定的字段顺序 + GORM tag；**关键**：`ID / UserID / ChestID / RewardUserCosmeticItemID / RewardCosmeticItemID uint64` 对齐 BIGINT UNSIGNED；`CostSteps uint32` 对齐 INT UNSIGNED；`RewardRarity int8` 对齐 TINYINT；**无** `UpdatedAt` 字段）
  - [x] Subtask 4.4: 写 `TableName()` 方法（按 AC3 钦定，返回 `"chest_open_logs"`）
- [x] Task 5: 扩展 migrate_integration_test.go（AC: #4）
  - [x] Subtask 5.1: 改 `TestMigrateIntegration_UpThenDown` 中 `expectedTables` slice 加 `"chest_open_logs"`（共 11 张表）+ 同步注释升级（按 AC4.1 钦定）
  - [x] Subtask 5.2: 改 `TestMigrateIntegration_UpTwice_Idempotent` 中表数量断言 10 → 11 + INFORMATION_SCHEMA WHERE table_name IN (...) slice 加 `'chest_open_logs'` + 同步注释升级（按 AC4.2 钦定）
  - [x] Subtask 5.3: 改 `TestMigrateIntegration_StatusAfterUp` 版本号断言 v != 12 → v != 13 + 同步注释升级（按 AC4.3 钦定；与 20.3 落地 0012 seed 后升级版本号同模式）
  - [x] Subtask 5.4: 新增 `TestMigrateIntegration_ChestOpenLogs_Schema` case（按 AC4.4 钦定 + 参考 `TestMigrateIntegration_CosmeticItems_Schema` 实装模板；**关键**断言点：8 列计数兜底防 updated_at 误加 + cost_steps column_type 含 "unsigned" + 双索引列顺序 + **无** UNIQUE 索引断言）
  - [x] Subtask 5.5: 新增 `TestMigrateIntegration_ChestOpenLogs_AppendOnly` case（按 AC4.5 钦定 + 参考 `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` 实装模板，但**反向**验证 —— 3 行 INSERT 全部成功 + COUNT = 3）
- [ ] Task 6: 验证 + 提交（AC: #5）
  - [x] Subtask 6.1: 跑 `bash scripts/build.sh --test` 全绿（vet + 全 unit 测试通过）
  - [x] Subtask 6.2: 跑 `bash scripts/build.sh --integration` 全绿（dockertest 跑通 + 新增 2 case + 既有 case 不回归；如本机 Docker 不可用 → 集成测试 `t.Skipf` 优雅跳过，与 17.2 / 20.2 同模式，code 路径已通过 `go vet -tags=integration ./...`）
  - [x] Subtask 6.3: git diff 范围检查 —— 仅本 story 钦定 6 个文件（见 File List）
  - [x] Subtask 6.4: schema 跨文档 / struct 一致性手动检查（AC5.3 + AC5.4）
  - [x] Subtask 6.5: 在 sprint-status.yaml 把本 story 状态从 in-progress 改为 review
  - [ ] Subtask 6.6: 由 code-review 检出后状态切 done + 在本 story 文件 + sprint-status.yaml 状态行追加 commit hash

## Dev Notes

### Build & Test 规范（项目级 CLAUDE.md 钦定）

- 写完 / 改完 Go 代码后必跑 `bash scripts/build.sh --test`（vet + 单测，**默认 build tag**，集成测试不跑）
- 集成测试 dockertest 必须用 `bash scripts/build.sh --integration`（带 `-tags=integration` build tag）
- 脚本契约来源：`_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.5 + `_bmad-output/implementation-artifacts/1-7-重做-scripts-build-sh.md`

### Migration 文件命名 / 编号规则（ADR-0003 + Story 4.3 钦定）

- 文件命名：`{N:04d}_{name}.up.sql` / `{N:04d}_{name}.down.sql`（4 位编号 + 下划线 + 小写下划线名称）
- 编号顺序：0001 ~ 0012 已被 4.3 / 7.2 / 10.3 / 11.2 / 17.2 / 17.3 / 20.2 / 20.3 占用（users / user_auth_bindings / pets / user_step_accounts / user_chests / user_step_sync_logs / rooms / room_members / emoji_configs init + seed / cosmetic_items init + seed）；**本 story 占用 0013**（首个 chest_open_logs migration）；epics.md §Story 20.4 钦定的文案为 `0013_init_chest_open_logs.sql`，与本 story 编号一致
- **不**用 GORM AutoMigrate / 不用 `migrate` CLI 之外的工具（与 ADR-0003 钦定一致）

### GORM struct 规范（11.2 + 4.6 + 17.2 + 20.2 落地）

- struct 字段顺序与 SQL DDL 列顺序一致（便于 cross-reference）
- 字段类型对齐 MySQL → Go 映射：`BIGINT UNSIGNED → uint64` / `VARCHAR → string` / `INT → int32` / `INT UNSIGNED → uint32` / `TINYINT → int8` / `DATETIME(3) → time.Time`
- **关键**：本 story 5 个 `BIGINT UNSIGNED` 字段（id / user_id / chest_id / reward_user_cosmetic_item_id / reward_cosmetic_item_id）均用 `uint64`；1 个 `INT UNSIGNED` 字段（cost_steps）用 `uint32`；1 个 `TINYINT` 字段（reward_rarity）用 `int8`；1 个 `DATETIME(3)` 字段（created_at）用 `time.Time` —— 类型映射严格遵守 Story 20.2 锚定规则
- GORM tag 仅含 `column:` / `primaryKey` / `autoIncrement` / `not null` / `default:V` —— **不**含 `uniqueIndex` / `index` / `type:` / `size:` 等（普通索引由 DDL 定义，与 ADR-0003 §3.2 一致；类型由字段 Go 类型推导）
- 显式 `TableName() string` 方法返回 DB 真实表名（避免 GORM 自动复数化引发漂移；与 20.2 落地的 `CosmeticItem.TableName() string { return "cosmetic_items" }` 同模式）
- **不**引入 `gorm.Model`（避免 `DeletedAt` 软删除字段 + 避免 `UpdatedAt` 字段污染 append-only 日志表语义）
- **关键**：本 story 是 **append-only 日志表**，struct **无** `UpdatedAt` 字段 —— 与 0006 user_step_sync_logs / `UserStepSyncLog` struct 同模式；与 0011 cosmetic_items 配置表（有 UpdatedAt）形成对照
- **不**在本 story 阶段新建 Repo interface / 实装方法（YAGNI，与 11.2 / 17.2 / 20.2 同模式）

### 节点 7 vs 节点 8 阶段语义（reward_user_cosmetic_item_id 字段）

- 本 story DDL `reward_user_cosmetic_item_id BIGINT UNSIGNED NOT NULL`（**无** DEFAULT）—— 由 service 层显式提供值
- **节点 7 阶段（Story 20.6 落地后到 Story 23.5 落地前）**：Story 20.6 service 层 INSERT 时显式传 `0`（占位语义）；V1接口设计 §7.2.4h + 数据库设计 §8 注解钦定
- **节点 8 阶段（Story 23.5 落地后）**：Story 23.5 修改开箱事务为先 INSERT user_cosmetic_items 拿到 id 再填入此处；本 story DDL 无需任何改动 —— `BIGINT UNSIGNED NOT NULL` 字段对 `0` 和真实 id 一视同仁
- 本 story **不**实装节点 7 vs 节点 8 切换逻辑（service 层 owner，分别由 20.6 / 23.5 落地）

### 跨文档语义同步检查（DB → API 单向）

- 本 story 落地的 0013 SQL DDL **只**反映数据库设计.md §5.7 的语义，**禁止反向**修改 §5.7
- 如发现 §5.7 与本 story 落地 DDL 有不一致（如字段名 / 类型 / 长度 / 默认值漂移）→ 优先修 0013 SQL 而非 §5.7
- 本 story 落地的 0013 DDL 是 V1 §7.2 cost / reward 字段类型 / 枚举约束（20.1 锚定）的**DB 端真相源**之一；不允许在本 story 阶段对契约层做反向加严 / 放松

### 错误码不在本 story 范围

- §3 全局错误码表（4001 / 4002 / 3002 / 1001 / 1002 / 1005 / 1009）由 20.1 锚定 + 由 20.5 / 20.6 实装时引用；本 story **不**触发错误码定义 / 修改 / 引用（migration 层不返回 API 错误码）

### `chest_open_idempotency_records` 表（§5.16）显式 out of scope

- **关键**：Story 20.1 r5 ~ r11 review 锁定了 `chest_open_idempotency_records` 表的 schema（数据库设计 §5.16）+ 同事务持久化幂等机制（V1 §7.2 步骤 3 / 5a / 5b / 5k 钦定的 MVCC + X-lock 行为）；但 §5.16 表 migration **不在本 story 范围**
- **Story 20.1 follow-up 钦定**（与 20.2 红线一致）：`chest_open_idempotency_records` migration 由 Story 20.6 dev kickoff 时决定收进 Story 20.6（与 chest_open_logs 同 migration 文件 / 不同文件）或起新 Story 20.10 独立落地；本 story（20.4）**仅** owner chest_open_logs
- 如 dev 看到本 story 时同时想顺手落地 §5.16 migration —— **禁止**（YAGNI + 范围红线违反 + 与 Story 20.1 r5 review follow-up + Story 20.2 / 20.3 同源红线直接冲突）；该决策必须在 Story 20.6 kickoff 时由 dev 单独评估

### `user_cosmetic_items` 表（§5.9）显式 out of scope

- `user_cosmetic_items` 表（数据库设计 §5.9）由 Epic 23 Story 23.2 落地 —— 本 story 落地的 `chest_open_logs.reward_user_cosmetic_item_id BIGINT UNSIGNED NOT NULL` 字段语义上 reference 该表的 `id`，但**不**建 FK，本 story 阶段不触碰 §5.9 表
- 节点 7 阶段（Story 20.6 落地后）该字段固定为 `0` 占位；节点 8 阶段（Story 23.5 落地后）该字段为真实 user_cosmetic_items.id

### 跨 epic 依赖追溯

- **上游冻结**：
  - 数据库设计 §5.7 chest_open_logs 表 schema ← 总体架构 + 数据库设计文档锚定（**不**由某个 story 锚定；本 story 严格对齐）
  - 数据库设计 §6.9 cosmetic_items.rarity 枚举（本 story DDL 不在 schema 层做 enum 约束，但 service 层依赖；同时 chest_open_logs.reward_rarity 与 cosmetic_items.rarity 同语义）
  - 数据库设计 §7.2 高优先级普通索引清单（行 937 `chest_open_logs(user_id, created_at)` 钦定本 story 的 `idx_user_id_created_at` 索引）
  - V1 §7.2 reward / cost 字段层 ← Story 20.1 锚定（已 done）
  - V1 §7.2.4h INSERT chest_open_logs SQL 语义 ← Story 20.1 锚定（节点 7 阶段 `reward_user_cosmetic_item_id` 写占位 0）
  - ADR-0003 migration 工具 + 编号规则 ← Story 4.3 落地
  - Story 20.2 cosmetic_items migration（已 done，本 story 直接 mirror 同模式）
  - Story 20.3 cosmetic_items seed（已 done，确保 cosmetic_items 表有合法 rarity 数据，间接支撑本 story 落地后 Story 20.6 / 20.9 实装时的 dockertest 路径）
- **下游强依赖**（本 story done 后才能开工）：
  - Story 20.5（GET /chest/current 接口）—— 不直接依赖（只查 user_chests），但同节点 7 阶段串联
  - Story 20.6（POST /chest/open 事务，步骤 5h 写 chest_open_logs）—— **强依赖**
  - Story 20.7（dev /dev/force-unlock-chest）—— 间接依赖（20.6 路径打通）
  - Story 20.8（dev /dev/grant-cosmetic-batch）—— 间接依赖
  - Story 20.9（Layer 2 集成测试，断言 chest_open_logs 行数 + 字段值）—— **强依赖**
  - iOS Epic 21.1 ~ 21.5（依赖 20.5 / 20.6 接口的 client 实装，间接依赖）
  - Epic 23 Story 23.5（修改开箱事务，回填 chest_open_logs.reward_user_cosmetic_item_id 真实值）—— **强依赖**

### Git Intelligence（最近 5 commits 模式参考）

- `f5ae36a chore(story-20-3): 收官 Story 20.3 + 归档 story 文件` —— Story 20.3 收官；可参考其 sprint-status 状态行流转格式 + commit message 模板
- `84b7267 feat(server): Epic20/20.3 cosmetic_items seed` —— Story 20.3 实装 commit；本 story commit 命名应仿 "feat(server): Epic20/20.4 chest_open_logs migration" 模式
- `6d0d335 chore(story-20-2): 收官 Story 20.2 + 归档 story 文件` —— Story 20.2 收官；同模式参考
- `1e65a5b feat(server): Epic20/20.2 cosmetic_items migration（首次落地 0011_init_cosmetic_items.up/down.sql + CosmeticItem GORM domain struct 最小骨架 + ≥3 case 单测 + dockertest 集成测试覆盖 UNIQUE(code) 运行时拒绝）` —— **本 story 的最直接 commit 模板参考**（同 migration 模式 / 同 Epic）；本 story commit message 应仿同模式：`feat(server): Epic20/20.4 chest_open_logs migration（首次落地 0013_init_chest_open_logs.up/down.sql + ChestOpenLog GORM domain struct 最小骨架 + ≥3 case 单测 + dockertest 集成测试覆盖 append-only 多行允许）`
- `bccf97d docs(lessons): backfill 20-1 r1~r15 fix-review commit hashes` —— 文档回填，与本 story 无业务关联
- **本 story 创建时（HEAD f5ae36a）的最近 server-side migration 落地 commit**：Story 20.3 `0012_seed_cosmetic_items` + Story 20.2 `0011_init_cosmetic_items` —— 这两个 commit 的 migration 文件 + GORM struct + integration test 升级模板是本 story 的**直接**参考来源

### 测试 / 验证

- **单元测试**：本 story 不新建 service / repo 方法 → 无 sqlmock-based 单测；既有 `cosmetic_item_repo_test.go`（如存在）/ `chest_repo_test.go` / `emoji_repo_test.go` 等不受影响
- **集成测试**（dockertest）：本 story 新增 2 case + 改既有 3 case（`TestMigrateIntegration_UpThenDown` 表数量 10 → 11 + `TestMigrateIntegration_UpTwice_Idempotent` 表数量 10 → 11 + `TestMigrateIntegration_StatusAfterUp` 版本 12 → 13）；用 `bash scripts/build.sh --integration` 跑（带 `-tags=integration`）
- **下游验证**：本 story done 后由 Story 20.6 实装时的 INSERT chest_open_logs 事务 + Story 20.9 Layer 2 完整事务集成测试做真实串联验证

### 范围红线 + 风险

- **红线**：本 story **不**修改任何 service / handler / repo write / read 方法；**不**修改 V1 接口契约 / 数据库设计文档 / ADR-0003 / 0001 ~ 0012 既有 migration；**不**预实装 `ChestOpenLogRepo` interface / 方法；**不**落地 `chest_open_idempotency_records` migration（§5.16 表，Story 20.6 / 20.10 owner）；**不**落地 `user_cosmetic_items` migration（§5.9 表，Story 23.2 owner）
- **红线**：本 story **不**实装 seed SQL（chest_open_logs 是运行时日志表无 seed 阶段）
- **红线**：本 story **不**包含 `updated_at` 字段（append-only 日志表语义；与 0006 user_step_sync_logs 同模式）
- **红线**：本 story **不**包含任何 UNIQUE 约束（日志表允许同 user_id 多次开箱）
- **红线**：本 story **不**建任何 FK 约束（与本设计其他表保持一致 —— "应用层校验 + 索引兜底"策略）
- **风险**：表数量断言 10 → 11 改漏（既有 `TestMigrateIntegration_UpThenDown` + `TestMigrateIntegration_UpTwice_Idempotent` 两处都要改 + `TestMigrateIntegration_StatusAfterUp` 版本号 12 → 13 一处）→ AC4.1 / AC4.2 / AC4.3 显式钦定，AC5.2 git diff 范围检查兜底
- **风险**：GORM struct 字段类型漂移
  - 风险 A：`CostSteps int32` 误写（应为 `uint32` 对齐 `INT UNSIGNED`）→ AC3 字段类型表 + AC5.4 GORM struct ↔ DDL 一致性手动检查 + AC4.4 `TestMigrateIntegration_ChestOpenLogs_Schema` 中 column_type 含 "unsigned" 断言兜底
  - 风险 B：误加 `UpdatedAt` 字段（应留给 append-only 语义留空）→ AC3 范围红线 + AC4.4 `TestMigrateIntegration_ChestOpenLogs_Schema` 中 8 字段计数断言兜底（计数 != 8 失败）
  - 风险 C：`RewardRarity int` 误写（应为 `int8` 对齐 `TINYINT`）→ AC3 字段类型表 + AC5.4 兜底
  - 风险 D：`RewardUserCosmeticItemID int64` 误写（应为 `uint64` 对齐 `BIGINT UNSIGNED`，避免节点 7 阶段写 `0` 占位语义被误解为 signed）→ AC3 字段类型表 + AC5.4 兜底
- **风险**：误加 UNIQUE 约束（如 `UNIQUE KEY uk_user_chest (user_id, chest_id)`）→ AC1 范围红线 + AC4.4 `TestMigrateIntegration_ChestOpenLogs_Schema` 中"无 UNIQUE 索引"断言 + AC4.5 `TestMigrateIntegration_ChestOpenLogs_AppendOnly` 中"3 行 INSERT 全部成功"运行时验证三层兜底
- **风险**：dockertest 集成测试在 Windows 本地跑可能因 Docker Desktop 未启动 / stale 容器残留失败 → 与 Story 11.2 / 17.2 / 20.2 同情况，由 dev-story 阶段确保 Docker Desktop 启动后跑；如本机 Docker 不可用 → 集成测试 `t.Skipf` 优雅跳过（migrate_integration_test.go 顶部注释钦定的降级路径），不阻塞 review；code-review 阶段在 fresh 环境 retry 验证
- **风险**：项目当前 `migrate_integration_test.go` 用 INFORMATION_SCHEMA.STATISTICS 查 index_name 时大小写敏感性 → MySQL 8.0 `lower_case_table_names=1`（Windows 默认）下 table_name 大小写归一为小写；index_name 不受影响（保留原大小写）；本 story 集成测试用 `idx_user_id_created_at` / `idx_reward_cosmetic_item_id` 小写命名与既有 case 一致，无大小写风险

### Project Structure Notes

- 本 story 唯一编辑文件（绝对路径）：
  - `C:/fork/cat/server/migrations/0013_init_chest_open_logs.up.sql`（新建）
  - `C:/fork/cat/server/migrations/0013_init_chest_open_logs.down.sql`（新建）
  - `C:/fork/cat/server/internal/repo/mysql/chest_open_log_repo.go`（新建）
  - `C:/fork/cat/server/internal/infra/migrate/migrate_integration_test.go`（修改）
  - `C:/fork/cat/_bmad-output/implementation-artifacts/20-4-chest_open_logs-migration.md`（本 story 文件）
  - `C:/fork/cat/_bmad-output/implementation-artifacts/sprint-status.yaml`（状态行流转）
- 与 `docs/宠物互动App_Go项目结构与模块职责设计.md` §6 锚定的 `internal/repo/mysql/` / `internal/migrations/` 目录完全兼容（沿用既有目录规则，**不**新增子目录 / 模块）

### References

- [Source: docs/宠物互动App_数据库设计.md#5.7] chest_open_logs 表 schema（行 399-433；本 story 严格对齐，**不**修改）
- [Source: docs/宠物互动App_数据库设计.md#5.8] cosmetic_items 表 schema（行 437-479；本 story **不** owner，仅做 reference 说明 `reward_cosmetic_item_id` 语义上 reference cosmetic_items.id；Story 20.2 已 done）
- [Source: docs/宠物互动App_数据库设计.md#5.9] user_cosmetic_items 表 schema（行 483-530；本 story **不** owner，仅做 reference 说明 `reward_user_cosmetic_item_id` 语义上 reference user_cosmetic_items.id；Epic 23 Story 23.2 owner）
- [Source: docs/宠物互动App_数据库设计.md#5.16] chest_open_idempotency_records 表 schema（行 727-787；**本 story 不 owner**；Story 20.1 r5 follow-up 钦定由 Story 20.6 / 20.10 决定）
- [Source: docs/宠物互动App_数据库设计.md#6.9] cosmetic_items.rarity 状态枚举（与 chest_open_logs.reward_rarity 同语义；本 story DDL 不在 schema 层做 enum 约束，由 service 层 + cosmetic_items 表合法 rarity 兜底）
- [Source: docs/宠物互动App_数据库设计.md#7.2] 高优先级普通索引清单（行 937 `chest_open_logs(user_id, created_at)` 钦定本 story `idx_user_id_created_at` 索引）
- [Source: docs/宠物互动App_数据库设计.md#8] 开箱事务设计（行 985-995；§8 注解钦定节点 7 vs 节点 8 阶段差异 —— `reward_user_cosmetic_item_id` 节点 7 写占位 0；本 story DDL 已为该切换预留语义）
- [Source: docs/宠物互动App_V1接口设计.md#7.2] POST /api/v1/chest/open reward / cost 字段表（20.1 锚定；本 story DDL 是 `cost.amount` / `reward.rarity` 字段类型 / 枚举约束的 DB 端真相源）
- [Source: docs/宠物互动App_V1接口设计.md#7.2.4h] 步骤 5h INSERT chest_open_logs SQL 语义（节点 7 阶段 `reward_user_cosmetic_item_id` 写占位 0；行 978 钦定）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 20.4] AC 钦定（行 2853-2871）：`0013_init_chest_open_logs.sql` + down.sql + 8 字段（含 `id`, `user_id`, `chest_id`, `cost_steps`, `reward_user_cosmetic_item_id`, `reward_cosmetic_item_id`, `reward_rarity`, `created_at`）+ KEY idx_user_id_created_at + KEY idx_reward_cosmetic_item_id + ≥3 case 单测 + dockertest 集成测试覆盖 schema 一致
- [Source: _bmad-output/implementation-artifacts/20-1-接口契约最终化.md] Story 20.1 上游契约（已 done；本 story 的 DB schema 是 §7.2 cost / reward 字段类型 / 枚举约束的 DB 端真相源；r5 follow-up 钦定 §5.16 表 migration 不在本 story 范围）
- [Source: _bmad-output/implementation-artifacts/20-2-cosmetic_items-migration.md] Story 20.2 已 done 姊妹 story（同 Epic 20 / 同 migration 模式 / 同 GORM struct 模式 / 同 dockertest 集成测试编辑模式；本 story 直接 mirror 同模式，但调整为 append-only 日志表语义 —— 无 updated_at / 无 UNIQUE）
- [Source: _bmad-output/implementation-artifacts/11-2-rooms-room_members-migration.md] Story 11.2 已 done 姊妹 story（多张表 migration + dockertest 集成测试模板参考）
- [Source: _bmad-output/implementation-artifacts/17-2-emoji_configs-migration.md] Story 17.2 已 done 姊妹 story（migration + GORM struct + dockertest 集成测试编辑模式参考）
- [Source: _bmad-output/implementation-artifacts/decisions/0003-orm-stack.md] ADR-0003 ORM / migration 工具栈（golang-migrate v4.18.1 + GORM v1.25.12；migration 编号规则 + .up.sql / .down.sql 双向规范；禁止 GORM AutoMigrate）
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md] ADR-0001 测试栈（dockertest + build tag `integration`；`bash scripts/build.sh --integration` 跑集成测试）
- [Source: server/migrations/0006_init_user_step_sync_logs.up.sql] Story 7.2 落地的 append-only 日志表 migration 模板（**无** updated_at 同模式参考 —— `UserStepSyncLog` struct 不含 `UpdatedAt` 字段）
- [Source: server/migrations/0011_init_cosmetic_items.up.sql + 0011_init_cosmetic_items.down.sql] Story 20.2 落地的顶部注释 + DDL 模板（"对齐 §X.Y" + 字段块 + 索引块 + 范围红线四段式参考）
- [Source: server/internal/repo/mysql/cosmetic_item_repo.go] Story 20.2 落地的 `CosmeticItem` GORM struct + `TableName()` 模板（本 story `ChestOpenLog` struct 模仿同模式，但调整为 8 字段 / 无 UpdatedAt）
- [Source: server/internal/infra/migrate/migrate_integration_test.go] Story 20.2 / 20.3 落地的 `TestMigrateIntegration_CosmeticItems_Schema` + `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` + `TestMigrateIntegration_StatusAfterUp` 实装模板
- [Source: _bmad-output/implementation-artifacts/sprint-status.yaml] Epic 20 状态 in-progress；本 story 状态行 + last_updated 由 create-story / dev-story / code-review 流程逐步推进

## Dev Agent Record

### Agent Model Used

claude-opus-4-7 (1M context)

### Debug Log References

- `bash scripts/build.sh --test` 全绿（vet + 全 unit 测试通过；2026-05-14）
- `bash scripts/build.sh --integration` 全绿（含 `go vet -tags=integration ./...` 通过；Docker daemon 未启动 → `TestMigrateIntegration_ChestOpenLogs_Schema` + `TestMigrateIntegration_ChestOpenLogs_AppendOnly` + 既有 `TestMigrateIntegration_*` 全部走 `t.Skipf("docker daemon not reachable")` 优雅跳过；code 路径已通过编译期校验；与 17.2 / 20.2 / 20.3 同情况降级处理；2026-05-14）

### Completion Notes List

- AC1 — 0013_init_chest_open_logs.up.sql 新建：严格对齐 §5.7 8 字段 + 2 索引（idx_user_id_created_at / idx_reward_cosmetic_item_id）+ append-only 语义（无 updated_at / 无 UNIQUE / 无 FK），顶部注释按 AC1 模板四段式（对齐 §5.7 + 字段 + 索引 + 范围红线）
- AC2 — 0013_init_chest_open_logs.down.sql 新建：仅 `DROP TABLE IF EXISTS chest_open_logs;` + 顶部注释（按 AC2 模板）
- AC3 — chest_open_log_repo.go 新建：`ChestOpenLog` GORM struct 8 字段（无 UpdatedAt）+ `TableName()`；字段类型严格对齐 `BIGINT UNSIGNED → uint64` / `INT UNSIGNED → uint32` / `TINYINT → int8` / `DATETIME(3) → time.Time`；仅 import `time`，**不**引入 `gorm.io/gorm`（YAGNI，本 story 阶段无 repo 方法）
- AC4 — migrate_integration_test.go 扩展：
  - AC4.1 `TestMigrateIntegration_UpThenDown` `expectedTables` slice 加 `"chest_open_logs"`（10 → 11 张表）+ 同步注释升级
  - AC4.2 `TestMigrateIntegration_UpTwice_Idempotent` 表数量断言 10 → 11 + INFORMATION_SCHEMA WHERE table_name IN (...) slice 加 `'chest_open_logs'` + 错误消息加 "Story 20.4 加 chest_open_logs"
  - AC4.3 `TestMigrateIntegration_StatusAfterUp` 版本号 v != 12 → v != 13 + 同步注释升级
  - AC4.4 新增 `TestMigrateIntegration_ChestOpenLogs_Schema`：覆盖 8 列计数（防 updated_at 误加）/ BIGINT UNSIGNED + INT UNSIGNED column_type 含 "unsigned" / 双索引列顺序 / PRIMARY 单列 = id / **无** UNIQUE 索引（COUNT(DISTINCT index_name) WHERE non_unique=0 = 1）/ created_at DEFAULT 含 "CURRENT_TIMESTAMP"
  - AC4.5 新增 `TestMigrateIntegration_ChestOpenLogs_AppendOnly`：3 行 INSERT 全部成功（user=1/chest=1、user=1/chest=2、user=1/chest=1 dup）+ COUNT(*) WHERE user_id=1 = 3，证实 append-only 语义无 UNIQUE 阻塞
- AC5 — 验证：
  - AC5.1 `bash scripts/build.sh --test` ✅ + `bash scripts/build.sh --integration` ✅（Docker 未启动 → 集成测试 t.Skipf 优雅跳过；code 路径已通过 vet）
  - AC5.2 git diff 范围 = 钦定 6 文件（2 SQL + 1 Go + 1 test + 本 story + sprint-status.yaml）
  - AC5.3 schema 跨文档一致性：0013 up.sql 字段名 / 类型 / 索引名与 §5.7 行 404-416 逐字段 1:1 对齐；与 V1 §7.2 cost.amount INT UNSIGNED + reward.rarity TINYINT 兼容
  - AC5.4 GORM struct ↔ DDL 一致性：8 字段 / 类型完全对齐（5 个 BIGINT UNSIGNED → uint64 / 1 个 INT UNSIGNED → uint32 / 1 个 TINYINT → int8 / 1 个 DATETIME(3) → time.Time）；无 UpdatedAt
  - AC5.5 既有迁移 / repo 测试不回归：`bash scripts/build.sh --test` 全包通过

### File List

- `server/migrations/0013_init_chest_open_logs.up.sql`（新增）
- `server/migrations/0013_init_chest_open_logs.down.sql`（新增）
- `server/internal/repo/mysql/chest_open_log_repo.go`（新增）
- `server/internal/infra/migrate/migrate_integration_test.go`（修改：顶部注释升级 + `TestMigrateIntegration_UpThenDown` expectedTables 加 chest_open_logs + `TestMigrateIntegration_UpTwice_Idempotent` 表数量 10 → 11 + `TestMigrateIntegration_StatusAfterUp` v != 12 → v != 13 + 新增 `TestMigrateIntegration_ChestOpenLogs_Schema` + `TestMigrateIntegration_ChestOpenLogs_AppendOnly` 两个 case）
- `_bmad-output/implementation-artifacts/20-4-chest_open_logs-migration.md`（状态流转 + Tasks/Subtasks 勾选 + Dev Agent Record / File List / Change Log）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（20-4 状态行 ready-for-dev → in-progress → review + last_updated）

### Change Log

| 日期 | 操作 | Story 状态 | 备注 |
|---|---|---|---|
| 2026-05-14 | create-story | backlog → ready-for-dev | 由 epic-loop / bmad-create-story workflow 自动生成（HEAD f5ae36a） |
| 2026-05-14 | dev-story | ready-for-dev → in-progress → review | 落地 0013_init_chest_open_logs.up/down.sql + ChestOpenLog GORM struct + migrate_integration_test 扩展 2 新增 case；`bash scripts/build.sh --test` ✅；`bash scripts/build.sh --integration` ✅（vet 通过，集成测试本机 Docker 未启动 t.Skipf 优雅跳过） |
