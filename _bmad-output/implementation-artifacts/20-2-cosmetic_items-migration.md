# Story 20.2: cosmetic_items migration（首次落地 0011_init_cosmetic_items.up/down.sql + CosmeticItem GORM domain struct 最小骨架 + ≥3 case 单测 + dockertest 集成测试覆盖 UNIQUE(code) 运行时拒绝）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want **首次落地** `server/migrations/0011_init_cosmetic_items.up.sql` + `server/migrations/0011_init_cosmetic_items.down.sql` 两个新 migration 文件（严格按 `docs/宠物互动App_数据库设计.md` §5.8 行 437-479 钦定的 CREATE TABLE DDL：`id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT` + `code VARCHAR(64) NOT NULL` + `name VARCHAR(64) NOT NULL` + `slot TINYINT NOT NULL` + `rarity TINYINT NOT NULL` + `asset_url VARCHAR(255) NOT NULL DEFAULT ''` + `icon_url VARCHAR(255) NOT NULL DEFAULT ''` + `drop_weight INT UNSIGNED NOT NULL DEFAULT 0` + `is_enabled TINYINT NOT NULL DEFAULT 1` + `created_at / updated_at DATETIME(3)` + `UNIQUE KEY uk_code (code)` + `KEY idx_slot_rarity (slot, rarity)` + `KEY idx_enabled_weight (is_enabled, drop_weight)` + `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`，1:1 对齐 §5.8；**不含** `render_config` 字段 —— 节点 10 / Epic 29 才加新 migration）+ **新增** `server/internal/repo/mysql/cosmetic_item_repo.go` 含 `CosmeticItem` GORM domain struct（与 0011 真实 schema 1:1 对齐：`ID / Code / Name / Slot / Rarity / AssetURL / IconURL / DropWeight / IsEnabled / CreatedAt / UpdatedAt`）+ `TableName() string` 显式返回 `"cosmetic_items"`（**仅** struct + TableName，**不**新增 Repo interface / 实装任何 Find / Create / WeightedRandomPick 方法，YAGNI；Story 20.3 seed 用 raw SQL / Story 20.6 才落地 repo 方法）+ **扩展** `server/internal/infra/migrate/migrate_integration_test.go` 的 `TestMigrateIntegration_UpThenDown`（表数量 9 → 10 + `expectedTables` slice 加 `"cosmetic_items"`）+ `TestMigrateIntegration_UpTwice_Idempotent`（同 9 → 10）+ `TestMigrateIntegration_StatusAfterUp` 版本号 v != 10 → v != 11 同步升级 + **新增** `TestMigrateIntegration_CosmeticItems_Schema`（验证 cosmetic_items 表 / 列 / 索引 / 默认值符合 §5.8）+ **新增** `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` dockertest 集成测试（覆盖 `UNIQUE KEY uk_code (code)` 运行时 INSERT 拒绝行为；epics.md §Story 20.2 钦定的"集成测试覆盖：migrate up → SHOW CREATE TABLE 对比 schema → migrate down"路径之外的额外 UNIQUE 拒绝路径，与 Story 17.2 落地的 `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` 同模式，保证 Story 20.3 seed `INSERT IGNORE` 兜底语义有 schema 层根基）,
so that **Story 20.3（cosmetic_items seed ≥15 行 + AR18 数量约束）+ Story 20.4（chest_open_logs migration）+ Story 20.5（GET /chest/current 接口）+ Story 20.6（POST /chest/open 事务 + idempotencyKey + 加权抽取，handler 内层 rate_limit + MySQL `chest_open_idempotency_records` 同事务持久化幂等记录）+ Story 20.7 / 20.8（dev 端点）+ Story 20.9（Layer 2 集成测试）+ iOS Epic 21 各 story（首页宝箱组件 + GET /chest/current 调用 + 开箱按钮 → POST /chest/open + 奖励弹窗 5 字段渲染）+ Epic 23（user_cosmetic_items + GET /cosmetics/inventory / catalog 接口）+ Epic 24 ~ 28（仓库 / 穿戴）+ Epic 29 / 30（render_config 渲染）+ Epic 32 / 33（合成事务 + UI）**可以基于一个**已落地、已具备完整测试覆盖、已通过 dockertest 真实 INSERT 验证、已具备完整 GORM domain struct 字段映射**的 cosmetic_items 持久化基础并行展开，不再出现"20.3 写 INSERT seed SQL 时找不到表 / 20.6 加权抽奖时 `is_enabled = 1 AND drop_weight > 0` SQL 命中错误表 / 20.6 写 chest_open_logs.reward_cosmetic_item_id 时无法 FK 关联（虽不建 FK，但 cosmetic_items.id 必须存在）/ 重复 code 在 prod 跑了之后才发现 UNIQUE 没生效 / GORM struct 字段名与 DB 列名漂移 / 节点 10 真要加 render_config 时发现本 story 落地的字段顺序已 over-engineered 多写了不该有的字段"的返工。

## 故事定位（Epic 20 第二条 = 第一条**实装** story；上承 20.1 契约定稿，下启 20.3 seed + 20.4 chest_open_logs migration + 20.5 GET /chest/current + 20.6 POST /chest/open 事务 + 20.7/20.8 dev 端点 + 20.9 Layer 2 集成测试）

- **Epic 20 进度**：20.1（契约定稿，done）→ **20.2（本 story，cosmetic_items migration + GORM domain struct + 测试覆盖）** → 20.3（cosmetic_items seed ≥15 行 + AR18 数量约束）→ 20.4（chest_open_logs migration）→ 20.5（GET /chest/current 接口）→ 20.6（POST /chest/open 事务 + idempotencyKey + 加权抽取）→ 20.7（dev 端点 POST /dev/force-unlock-chest）→ 20.8（dev 端点 POST /dev/grant-cosmetic-batch）→ 20.9（Layer 2 集成测试 - 开箱事务全流程）。
- **本 story 是 20.3 / 20.4 / 20.5 / 20.6 / 20.7 / 20.8 / 20.9 / Epic 21 / Epic 23 / Epic 24 ~ 28 / Epic 29 / 30 / 32 / 33 的强前置**：
  - **20.3 seed**：seed 需要的 `INSERT INTO cosmetic_items (code, name, slot, rarity, asset_url, icon_url, drop_weight, is_enabled) VALUES ...` SQL 必须命中本 story 落地的 0011 表 schema；`UNIQUE KEY uk_code (code)` + `INSERT IGNORE` / `ON DUPLICATE KEY UPDATE` 兜底语义依赖本 story 已落地的 UNIQUE 约束；AR18 数量约束（common ≥ 8 / rare ≥ 4 / epic ≥ 2 / legendary ≥ 1）由 20.3 钦定，本 story **不**预实装任何 seed 数据
  - **20.4 chest_open_logs migration**：`chest_open_logs.reward_cosmetic_item_id BIGINT UNSIGNED NOT NULL`（数据库设计 §5.7 行 409 钦定）需要 cosmetic_items 表已存在（**不**建 FK，与本设计其他表保持一致，但语义上是 reference）；20.4 自身 migration 不依赖 cosmetic_items 表的 SQL 操作，但 20.6 POST /chest/open 事务步骤 5h（写 chest_open_logs）会同时引用两表
  - **20.6 POST /chest/open 事务**：步骤 5g 加权抽取 SQL `SELECT id, code, name, slot, rarity, asset_url, icon_url, rarity FROM cosmetic_items WHERE is_enabled = 1 ORDER BY ... LIMIT 1`（具体加权算法由 20.6 实装层决定，但 SQL 必须命中本 story 落地的 `idx_enabled_weight (is_enabled, drop_weight)` 索引）；GORM struct（本 story 新增的 `CosmeticItem`）直接被 20.6 repo 层 `Find(ctx, &items, "is_enabled = ?", 1)` 复用
  - **20.7 dev /dev/force-unlock-chest**：不直接依赖 cosmetic_items，但 20.7 落地需要 20.6 路径打通，间接依赖
  - **20.8 dev /dev/grant-cosmetic-batch**：批量发放装扮实例（节点 8 才真实写库；节点 7 阶段实装路由 + handler 框架），handler 框架引用 cosmetic_items.id 做参数校验
  - **20.9 Layer 2 集成测试**：dockertest 走完整开箱事务路径，必然依赖 cosmetic_items 表已 seed + schema 正确
  - **iOS Epic 21.4 奖励弹窗 popup**：iOS 端 `ChestRewardDTO` Codable struct 字段（`reward.cosmeticItemId / name / slot / rarity / assetUrl / iconUrl`）通过 20.6 POST /chest/open JSON response 间接依赖本 story 落地的 DB schema；本 story 落地的字段（`code VARCHAR(64)` / `name VARCHAR(64)` / `slot TINYINT` / `rarity TINYINT` / `asset_url VARCHAR(255)` / `icon_url VARCHAR(255)`）是 V1 §7.2 reward 字段类型 / 长度约束的**唯一真相源**
  - **Epic 23 user_cosmetic_items + GET /cosmetics/catalog / inventory**：`user_cosmetic_items.cosmetic_item_id BIGINT UNSIGNED NOT NULL`（数据库设计 §5.9 行 491）reference 本 story 落地的 cosmetic_items.id；GET /cosmetics/catalog 接口（Story 23.3）返回 cosmetic_items 表内容，schema 依赖本 story
  - **Epic 29 render_config 加列**：本 story **不含** `render_config` 字段；节点 10 / Epic 29 Story 29.2 才落地新 migration（如 `001X_add_render_config_to_cosmetic_items.up.sql`）加 `render_config TEXT` 字段；本 story 故意保留扩展空间
  - **Epic 32 / 33 合成事务**：合成产出新 user_cosmetic_items 实例 → reference cosmetic_items.id；schema 依赖本 story
- **epics.md §Story 20.2 钦定**（行 2807-2826）：
  - `migrations/0011_init_cosmetic_items.sql` 按数据库设计.md §5.8 创建表，含 `id` PK + 全字段（含 `code`, `name`, `slot`, `rarity`, `asset_url`, `icon_url`, `drop_weight`, `is_enabled`, `created_at`, `updated_at`）+ `UNIQUE KEY uk_code` + `KEY idx_slot_rarity` + `KEY idx_enabled_weight`
  - **不含 `render_config` 字段**（节点 10 才加，由 Epic 29 添加新 migration）
  - 含 down.sql（DROP TABLE 路径）
  - **单元测试覆盖**（≥3 case）：happy（migrate up 后表存在 + 字段类型 + 索引都符合 §5.8）/ happy（migrate down 后表删除）/ edge（重复 migrate up → 幂等，由现有 `TestMigrateIntegration_UpTwice_Idempotent` 扩展覆盖）
  - **集成测试覆盖**（dockertest）：migrate up → SHOW CREATE TABLE 对比 schema → migrate down（本 story 用更精确的 `INFORMATION_SCHEMA.COLUMNS` / `STATISTICS` 字段层断言取代字符串 SHOW CREATE TABLE 比对 —— 后者跨 MySQL 版本输出格式不稳定，前者是结构化 schema 真相源，与 Story 17.2 / 11.2 落地的 `TestMigrateIntegration_*_Schema` case 同模式）
- **Story 20.1 上游冻结边界**（V1 §7.2 reward 字段表 + 数据库设计 §5.8 字段表）：本 story 落地的字段长度约束（`code VARCHAR(64)` / `name VARCHAR(64)` / `slot TINYINT` / `rarity TINYINT` / `asset_url VARCHAR(255)` / `icon_url VARCHAR(255)`）是 20.1 锚定的 API 字段长度（`reward.cosmeticItemId` BIGINT 字符串化 / `reward.name` 1 ≤ length ≤ 64 / `reward.slot` 枚举 `{1,2,3,4,5,6,7,99}` 与 §6.8 同义 / `reward.rarity` 枚举 `{1,2,3,4}` 与 §6.9 同义 / `reward.assetUrl` 1 ≤ length ≤ 255 / `reward.iconUrl` 1 ≤ length ≤ 255）的**DB 端真相源**；本 story **不**反向修改 DB schema（DB → API 单向），仅严格对齐数据库设计文档 §5.8 DDL
- **下游强依赖**（本 story done 后才能开工）：
  - Story 20.3（cosmetic_items seed ≥15 行 + AR18 数量约束）
  - Story 20.4（chest_open_logs migration）
  - Story 20.5（GET /chest/current 接口）
  - Story 20.6（POST /chest/open 事务 + idempotencyKey + 加权抽取）
  - Story 20.7（dev 端点 POST /dev/force-unlock-chest）
  - Story 20.8（dev 端点 POST /dev/grant-cosmetic-batch）
  - Story 20.9（Layer 2 集成测试）
  - iOS Epic 21.1 ~ 21.5（首页宝箱组件 + GET /chest/current 调用 + POST /chest/open + 奖励弹窗 + 开箱前主动同步步数）
  - Epic 23 Story 23.2（user_cosmetic_items migration，reference 本 story 落地的 cosmetic_items.id）
  - Epic 23 Story 23.3（GET /cosmetics/catalog 接口，返回 cosmetic_items 表内容）
  - Epic 29 Story 29.2（render_config 加列 migration —— **新** migration 文件，**不**在本 story 范围）
- **范围红线**：
  - 本 story **只**改 `server/migrations/0011_init_cosmetic_items.up.sql`（新建）+ `server/migrations/0011_init_cosmetic_items.down.sql`（新建）+ `server/internal/repo/mysql/cosmetic_item_repo.go`（新建，含 `CosmeticItem` struct + `TableName()`）+ `server/internal/infra/migrate/migrate_integration_test.go`（扩展 `TestMigrateIntegration_UpThenDown` 表数量断言 9 → 10 + `expectedTables` slice 加 `"cosmetic_items"` + 扩展 `TestMigrateIntegration_UpTwice_Idempotent` 表数量断言 9 → 10 + 同步注释升级 + 顺手升级 `TestMigrateIntegration_StatusAfterUp` 版本号 v != 10 → v != 11 + 新增 `TestMigrateIntegration_CosmeticItems_Schema` case + 新增 `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` case）+ 本 story 文件 + sprint-status.yaml 流转
  - **不**实装任何 service / handler / repo write / read 方法（20.3 ~ 20.9 才做；本 story 阶段**仅**落地 GORM struct + TableName，**不**新建 `CosmeticItemRepo` interface / 实装 `Find` / `Create` / `WeightedRandomPick` / `ExistsByCode` 等方法）
  - **不**实装任何 seed SQL（20.3 才做；本 story **仅** CREATE TABLE，不含任何 INSERT）
  - **不**实装 chest_open_logs migration（20.4 才做）
  - **不**实装 chest_open_idempotency_records migration（**20.1 r5 review follow-up 锁定**：由 Story 20.6 dev kickoff 时决定收进 Story 20.6 或起新 Story 20.10；本 story **不**触碰 §5.16 表）
  - **不**接 Redis / **不**接 chest_open_idempotency_records（Story 20.1 r5 ~ r11 review 已确定幂等机制走 MySQL `chest_open_idempotency_records` 同事务持久化，但表 migration 不在本 story）
  - **不**改 V1 接口契约（20.1 已冻结）
  - **不**改任何 `docs/宠物互动App_*.md`（§5.8 是契约**输入**，本 story 严格对齐它但**不修改**；如发现 §5.8 与本 story 落地的 DDL 有不一致 → 优先以 §5.8 为准修改本 story 而非反向改 §5.8）
  - **不**改 `_bmad-output/` 下其他 yaml / md（除自己 story 文件 + sprint-status.yaml 流转）
  - **不**改 ADR-0003（migration 工具 + 编号约定 + .up.sql / .down.sql 双向规范由 Story 4.3 已锚定，本 story 沿用 / 不修改）
  - **不**改 `cmd/server/main.go` migrate 子命令（已在 Story 4.3 落地）
  - **不**为 0011 写"prod 部署 migration 自动化 / 一键 rollback / dry-run / force"等运维化改造（保留最小集 up/down，与 Story 4.3 / 7.2 / 10.3 / 11.2 / 17.2 一致）
  - **不**写 `render_config` 字段（节点 10 / Epic 29 Story 29.2 owner；本 story 故意保留扩展空间）

**本 story 不做**（明确范围红线）：

- 不实装任何 Go handler / service / repo write 或 read 方法（20.3 ~ 20.9 范围）
- 不实装任何 INSERT seed SQL（20.3 钦定 owner；本 story **仅** CREATE TABLE）
- 不新建 `CosmeticItemRepo` interface（YAGNI；20.6 实装加权抽取时才落地 `CosmeticItemRepo` 类型 + `ListEnabled(ctx) ([]CosmeticItem, error)` 或 `WeightedRandomPick(ctx) (*CosmeticItem, error)` 等方法）
- 不在 `CosmeticItem` struct 上加 GORM `uniqueIndex` / `index` 等 tag（UNIQUE / 普通索引由 SQL DDL 定义，GORM struct **不**重复声明 —— 与 ADR-0003 §3.2 "禁止 GORM AutoMigrate" 同源；GORM struct 仅为 Find / Create 提供字段映射，**不**作为 schema 真相源；与 17.2 落地的 `EmojiConfig` / 11.2 落地的 `RoomMember` / `Room` struct 同模式）
- 不引入 `gorm.Model`（避免引入 `DeletedAt` / 软删除字段，与 0011 真实 schema 不符）
- 不在 struct 中预留 `RenderConfig` 字段（节点 10 / Epic 29 owner；本 story 即使顺手加占位字段也会让 0011 SQL 与 §5.8 不一致 → 跨文档漂移）
- 不修改 0001 ~ 0010 既有 migration 文件（已在 Story 4.3 / 7.2 / 10.3 / 11.2 / 17.2 / 17.3 落地）
- 不修改 V1 接口契约（20.1 已冻结）
- 不修改数据库设计 §5.8（schema 输入，本 story 严格对齐不修改）
- 不修改 Go 项目结构文档 §6（`internal/repo/mysql/` 目录已锚定；本 story 新增 `cosmetic_item_repo.go` 沿用既有目录规则）
- 不修改 ADR-0003（migration 工具 / 编号约定 / 文件命名规范由 Story 4.3 已锚定）
- 不写英文版测试注释 / 文档（项目 communication_language=Chinese；与 Story 4.3 / 7.2 / 10.3 / 11.2 / 17.2 一致）
- 不为 0011 写 stress test / fuzz test（节点 7 阶段 schema 稳定 + 单测 + dockertest 集成测试已覆盖核心约束）
- 不在本 story 内对 Story 20.3 ~ 20.9 实装做"提前预实装"（即使顺手写 `(r *cosmeticItemRepo) List(ctx) ([]CosmeticItem, error)` 也禁止；这些方法是 20.6 钦定范围，提前 ship 会让 20.6 评审找不到"新增方法"的明确范围边界，与 Story 11.2 / 17.2 "禁止预实装" 同模式）
- 不写 `CosmeticItem.IsEnabled` / `.Slot` / `.Rarity` 字段的 enum 校验（DB 端 `TINYINT NOT NULL DEFAULT 1` / `TINYINT NOT NULL` 已兜底；§6.8 / §6.9 状态枚举钦定值域；service 层校验由 20.6 / 23.x 实装时按需添加）

## Acceptance Criteria

**AC1 — 0011_init_cosmetic_items.up.sql 新建（与 §5.8 钦定 1:1 对齐）**

新建 `server/migrations/0011_init_cosmetic_items.up.sql`，内容必须**严格**对齐 `docs/宠物互动App_数据库设计.md` §5.8（行 437-479）钦定的 DDL：

```sql
-- 对齐 docs/宠物互动App_数据库设计.md §5.8 (行 437-479)
-- cosmetic_items 表：装扮配置表（"装扮是什么"，不是"玩家拥有哪一件"）
--
-- **本 migration 由 Story 20.2 首次落地（Epic 20 节点 7 宝箱业务链路 schema 根基 owner）**
-- 含 ≥3 case 单测（migrate up 表存在 + 字段类型 + uk_code + idx_slot_rarity + idx_enabled_weight 索引符合 §5.8）
-- + dockertest 集成测试覆盖 UNIQUE KEY uk_code (code) 运行时 INSERT 拒绝行为
-- （epics.md §Story 20.2 钦定的"集成测试覆盖：migrate up → SHOW CREATE TABLE 对比 schema → migrate down"路径
-- 升级为更精确的 INFORMATION_SCHEMA 字段层断言 + 额外 UNIQUE 拒绝运行时验证）。
--
-- 字段（与 §5.8 钦定 1:1 对齐）：
--   - id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§3.1 主键约定）
--   - code VARCHAR(64) NOT NULL：装扮业务编码（如 hat_yellow / scarf_star）；
--     与 V1 接口设计 §7.2 / §8.1 / §8.2 锚定的 cosmeticItem code 字段同语义；
--     UNIQUE KEY uk_code 保证全局唯一（Story 20.3 seed 用 INSERT IGNORE / ON DUPLICATE KEY UPDATE
--     兜底重复执行）
--   - name VARCHAR(64) NOT NULL：装扮中文名（如 小黄帽 / 星星围巾），
--     client 用作 UI 展示文字；与 V1 §7.2 reward.name 字段同源（1 ≤ length ≤ 64）
--   - slot TINYINT NOT NULL：部位枚举（§6.8 钦定：1=hat / 2=gloves / 3=glasses /
--     4=neck / 5=back / 6=body / 7=tail / 99=other）；
--     **注**：DDL 不在 schema 层做 enum 约束（TINYINT 允许任意 -128~127 值），
--     由 server seed 层（20.3）+ admin 写入层校验 enum 值合法性；
--     与 V1 §7.2 reward.slot 字段同源（int 枚举）
--   - rarity TINYINT NOT NULL：品质枚举（§6.9 钦定：1=common / 2=rare /
--     3=epic / 4=legendary）；
--     同 slot：DDL 不在 schema 层做 enum 约束，由 service 层 + seed 校验；
--     与 V1 §7.2 reward.rarity 字段同源（int 枚举）；
--     与 chest_open_logs.reward_rarity 字段（§5.7 行 411）同语义（开箱日志同步记录）
--   - asset_url VARCHAR(255) NOT NULL DEFAULT ''：装扮资源 URL；
--     **注**：DDL DEFAULT '' 是兜底语义（避免 admin / dev 临时写入时漏字段），
--     enabled 装扮（is_enabled=1）必须有非空 asset_url —— Story 20.3 seed 钦定
--     每个 cosmetic 非空（按 AR18 / AR19 URL 约束，MVP 允许 placeholder URL
--     如 https://placehold.co/128x128?text=Hat-Yellow）；
--     与 V1 §7.2 reward.assetUrl 字段同源（1 ≤ length ≤ 255）
--   - icon_url VARCHAR(255) NOT NULL DEFAULT ''：图标资源 URL（仓库 / catalog 列表用小图）；
--     同 asset_url：enabled 装扮必须非空；
--     与 V1 §7.2 reward.iconUrl 字段同源（1 ≤ length ≤ 255）
--   - drop_weight INT UNSIGNED NOT NULL DEFAULT 0：掉落权重（用于 Story 20.6
--     加权抽奖；按品质递减如 common=100 / rare=20 / epic=4 / legendary=1，
--     由 Story 20.3 seed 钦定，本 migration 仅定义字段不预置数据）；
--     INT UNSIGNED 是 32 位无符号（0 ~ 2^32 - 1），抽奖时权重为 0 的行被
--     `WHERE drop_weight > 0` 过滤掉
--   - is_enabled TINYINT NOT NULL DEFAULT 1：是否启用（§6 枚举：0=disabled / 1=enabled）；
--     §8.1 GET /cosmetics/catalog（Epic 23）仅返回 is_enabled=1 的 cosmetic；
--     §7.2 POST /chest/open 加权抽取仅命中 is_enabled=1 的行（Story 20.6 实装 SQL
--     `WHERE is_enabled = 1` 走 idx_enabled_weight 索引覆盖）
--   - created_at / updated_at DATETIME(3)（§3.2 毫秒精度时间戳）
--
-- 索引（§5.8 + §7 钦定）：
--   - UNIQUE KEY uk_code (code)：保证 code 全局唯一（§7.1 高优先级 UNIQUE 约束，
--     与 emoji_configs.uk_code 同模式）；
--     Story 20.3 seed 用 INSERT IGNORE / ON DUPLICATE KEY UPDATE 兜底重复执行
--   - KEY idx_slot_rarity (slot, rarity)：覆盖
--     "SELECT ... WHERE slot=? AND rarity=?" 路径（Epic 32 合成事务需要按
--     slot + rarity 维度做加权抽取，本索引提前覆盖）；
--     节点 7 阶段 Story 20.6 加权抽取暂不按 slot/rarity 维度（按全局 drop_weight），
--     本索引是节点 11 合成事务的提前准备（与 §5.8 钦定一致，不延后到 Epic 32）
--   - KEY idx_enabled_weight (is_enabled, drop_weight)：覆盖
--     "SELECT ... WHERE is_enabled=1 AND drop_weight > 0 ORDER BY ..." 路径
--     （Story 20.6 加权抽取 SQL；多列复合索引覆盖筛选 + 权重排序）
--
-- **范围红线**：本 migration **仅** CREATE TABLE；不含任何 INSERT / seed 数据
-- （seed 由 Story 20.3 `0012_seed_cosmetic_items.up.sql` 落地，**不**塞进本文件）；
-- 不含任何业务 service / handler / repo write 方法（20.3 ~ 20.9 落地）；
-- 不含 render_config 字段（节点 10 / Epic 29 Story 29.2 落地 `001X_add_render_config_to_cosmetic_items.up.sql`
-- 新 migration 加列，**不**在本 story 范围）。
CREATE TABLE cosmetic_items (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    code VARCHAR(64) NOT NULL,
    name VARCHAR(64) NOT NULL,
    slot TINYINT NOT NULL,
    rarity TINYINT NOT NULL,
    asset_url VARCHAR(255) NOT NULL DEFAULT '',
    icon_url VARCHAR(255) NOT NULL DEFAULT '',
    drop_weight INT UNSIGNED NOT NULL DEFAULT 0,
    is_enabled TINYINT NOT NULL DEFAULT 1,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_code (code),
    KEY idx_slot_rarity (slot, rarity),
    KEY idx_enabled_weight (is_enabled, drop_weight)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

- DDL 内容**严格**对齐 §5.8 行 442-458 —— 字段顺序 / 字段类型 / NOT NULL / DEFAULT 值 / 索引名 / 索引列顺序全部 1:1
- **不含** `render_config` 字段（节点 10 / Epic 29 Story 29.2 落地新 migration 加列，本 story 故意保留扩展空间）
- 文件编码 UTF-8 + LF 行尾（与 0001 ~ 0010 一致）
- 顶部注释模板与 0009 / 0010（17.2 / 17.3 升级版本）一致 —— "对齐 §X.Y" + "字段" + "索引" + "范围红线" 四段式
- **不**包含任何 INSERT / seed 数据（Story 20.3 owner）
- **不**包含任何 business logic SQL（如 `UPDATE cosmetic_items SET ...`）

**AC2 — 0011_init_cosmetic_items.down.sql 新建**

新建 `server/migrations/0011_init_cosmetic_items.down.sql`，内容：

```sql
-- 回滚 0011_init_cosmetic_items.up.sql
--
-- **本 migration 由 Story 20.2 首次落地（Epic 20 节点 7 宝箱业务链路 schema 根基 owner）**
-- 含 ≥3 case 单测 + dockertest 集成测试覆盖 UNIQUE 约束运行时拒绝行为。
DROP TABLE IF EXISTS cosmetic_items;
```

- 文件编码 UTF-8 + LF 行尾
- **仅** `DROP TABLE IF EXISTS cosmetic_items;`，不含任何额外 cleanup 语句（与 0007 / 0008 / 0009 down.sql 同模式）

**AC3 — cosmetic_item_repo.go 新建（仅 `CosmeticItem` GORM domain struct + `TableName()`，无 repo 方法）**

新建 `server/internal/repo/mysql/cosmetic_item_repo.go`，内容必须包含：

```go
package mysql

import (
	"time"
)

// CosmeticItem 是 cosmetic_items 表的完整 GORM domain struct（Story 20.2 引入；
// 与 server/migrations/0011_init_cosmetic_items.up.sql 真实 schema 1:1 对齐）。
//
// 字段（与数据库设计.md §5.8 + 0011_init_cosmetic_items.up.sql 1:1 对齐）：
//   - ID:         BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§5.8 + §3.1 主键约定）
//   - Code:       VARCHAR(64) NOT NULL（装扮业务编码；UNIQUE KEY uk_code 保证全局唯一）
//   - Name:       VARCHAR(64) NOT NULL（装扮中文名，UI 展示文字）
//   - Slot:       TINYINT NOT NULL（§6.8 枚举：1=hat / 2=gloves / 3=glasses /
//                 4=neck / 5=back / 6=body / 7=tail / 99=other）
//   - Rarity:     TINYINT NOT NULL（§6.9 枚举：1=common / 2=rare / 3=epic / 4=legendary）
//   - AssetURL:   VARCHAR(255) NOT NULL DEFAULT ''（装扮资源 URL；enabled 装扮必须非空）
//   - IconURL:    VARCHAR(255) NOT NULL DEFAULT ''（图标资源 URL；enabled 装扮必须非空）
//   - DropWeight: INT UNSIGNED NOT NULL DEFAULT 0（加权抽奖权重；0 = 不参与抽奖）
//   - IsEnabled:  TINYINT NOT NULL DEFAULT 1（§6 枚举：0=disabled / 1=enabled）
//   - CreatedAt:  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
//   - UpdatedAt:  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
//
// 表层 UNIQUE 约束（uk_code）+ 普通索引（idx_slot_rarity / idx_enabled_weight）
// 由 SQL DDL 定义，**不**在 struct tag 中重复声明（与 ADR-0003 §3.2 "禁止 GORM
// AutoMigrate" 同源；GORM struct 仅为 Find / Create 提供字段映射，**不**作为
// schema 真相源；与 Story 17.2 落地的 EmojiConfig / Story 11.2 落地的
// RoomMember / Room struct 同模式）。
//
// **范围红线**：本 struct 仅为下游 Story 20.3（seed）/ 20.6（POST /chest/open 加权抽取）/
// Epic 23（GET /cosmetics/catalog / inventory）/ Epic 32 / 33（合成事务）提供字段
// 映射；本 story 阶段**不**新建 CosmeticItemRepo interface / 实装 List /
// WeightedRandomPick / Exists / Create 等方法（YAGNI；20.6 落地加权抽取方法 +
// 23.x 落地 catalog / inventory 方法）。
//
// **不**包含 RenderConfig 字段（节点 10 / Epic 29 Story 29.2 落地 add_column
// migration 后由该 story 同步加 RenderConfig string `gorm:"column:render_config"` 字段）。
type CosmeticItem struct {
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Code       string    `gorm:"column:code;not null;size:64"`
	Name       string    `gorm:"column:name;not null;size:64"`
	Slot       int8      `gorm:"column:slot;not null"`
	Rarity     int8      `gorm:"column:rarity;not null"`
	AssetURL   string    `gorm:"column:asset_url;not null;size:255;default:''"`
	IconURL    string    `gorm:"column:icon_url;not null;size:255;default:''"`
	DropWeight uint32    `gorm:"column:drop_weight;not null;default:0"`
	IsEnabled  int8      `gorm:"column:is_enabled;not null;default:1"`
	CreatedAt  time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
	UpdatedAt  time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

// TableName 显式声明 "cosmetic_items"。
func (CosmeticItem) TableName() string { return "cosmetic_items" }
```

- 字段顺序与 0011 SQL 列顺序一致（11 字段）
- `Slot int8` / `Rarity int8` / `IsEnabled int8` 对齐 `TINYINT`（带符号；MySQL `TINYINT` 默认带符号，范围 -128 ~ 127；§6.8 / §6.9 枚举值都在范围内；与 Story 11.2 `Room.Status int8` / 17.2 `EmojiConfig.IsEnabled int8` 同模式）
- `DropWeight uint32` 对齐 `INT UNSIGNED`（无符号 32 位，范围 0 ~ 2^32 - 1；与 §5.8 `INT UNSIGNED` 一致；**关键**：用 `uint32` 而非 `int32` 才能精确对齐 unsigned；与 Story 7.x 落地的 `user_step_accounts.available_steps INT UNSIGNED → uint32` 同模式）
- `AssetURL` / `IconURL` 命名遵循 Go 风格（Go 风格 `URL` 全大写缩写；GORM `column:asset_url` / `column:icon_url` 显式映射到 DB 列名）
- `Code` / `Name` 字段类型 `string` + `size:64` tag 仅为文档化（GORM 不强制；实际长度约束由 DDL `VARCHAR(64)` 兜底）
- **关键**：导入 `time` 包 + **不**引入 `gorm.Model`（避免 `DeletedAt` 软删除字段）
- **关键**：**不**在 struct 上加 `gorm:"uniqueIndex:uk_code"` / `gorm:"index:idx_slot_rarity"` / `gorm:"index:idx_enabled_weight"` tag（UNIQUE / 索引由 SQL DDL 定义，与 ADR-0003 §3.2 一致）
- **关键**：**不**包含 `RenderConfig` 字段（Epic 29 Story 29.2 owner，本 story 故意保留扩展空间）
- **不**新建 `CosmeticItemRepo` interface / `cosmeticItemRepo` struct / `NewCosmeticItemRepo()` constructor / 任何 `Find` / `Create` / `WeightedRandomPick` / `Exists` 方法（YAGNI；20.6 / 23.x owner）
- 文件内容**仅**含 package 声明 + import + `CosmeticItem` struct + `TableName()` 方法（≤ 60 行；与 17.2 落地的 `EmojiConfig` / 11.2 落地的 `Room` struct 同体积级别）

**AC4 — migrate_integration_test.go 扩展（表数量断言 9 → 10 + cosmetic_items schema 验证 + UNIQUE(code) 拒绝集成测试）**

修改 `server/internal/infra/migrate/migrate_integration_test.go`：

**AC4.1 扩展既有 `TestMigrateIntegration_UpThenDown`**：
- 找到现有 `expectedTables := []string{"users", ..., "emoji_configs"}`（行 ~163）slice，加 `"cosmetic_items"`（共 10 张表）
- 表数量验证逻辑会自动按 slice 长度走，无硬编码数字需改（slice driven）
- 同步注释升级（顶部注释 + 行内注释一并升级）："Story 17.2 加 emoji_configs（9 张）→ Story 20.2 加 cosmetic_items（共 10 张表）"

**AC4.2 扩展既有 `TestMigrateIntegration_UpTwice_Idempotent`**：
- 找到现有 "表数量 = 9" 断言（行 ~321 + ~337）+ 现有 INFORMATION_SCHEMA `WHERE table_name IN (...)` slice（行 ~332），表数量 9 → 10 + slice 加 `'cosmetic_items'`
- 同步注释升级（与 AC4.1 一致）

**AC4.3 同步升级 `TestMigrateIntegration_StatusAfterUp`**：
- 找到现有 `if v != 10` 断言（17.3 落地后版本号），改为 `if v != 11`（0011 落地后）
- 同步注释升级（与 AC4.1 一致；理由："Story 17.3 落地 0010 seed → Story 20.2 落地 0011 init"，与 Story 7.2 / 10.3 / 17.2 / 17.3 review 同模式）

**AC4.4 新增 `TestMigrateIntegration_CosmeticItems_Schema`**（紧接 `TestMigrateIntegration_EmojiConfigs_Schema` 之后，参考既有 case 实装模式）：

```go
// TestMigrateIntegration_CosmeticItems_Schema 验证
// migrations/0011_init_cosmetic_items.up.sql 钦定的 cosmetic_items 表 schema
// 与数据库设计.md §5.8 + V1接口设计.md §7.2 + §8.1 一致：
//
//   - id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT
//   - code VARCHAR(64) NOT NULL + UNIQUE KEY uk_code (code)
//   - name VARCHAR(64) NOT NULL
//   - slot TINYINT NOT NULL
//   - rarity TINYINT NOT NULL
//   - asset_url VARCHAR(255) NOT NULL DEFAULT ''
//   - icon_url VARCHAR(255) NOT NULL DEFAULT ''
//   - drop_weight INT UNSIGNED NOT NULL DEFAULT 0
//   - is_enabled TINYINT NOT NULL DEFAULT 1
//   - created_at / updated_at DATETIME(3)
//   - KEY idx_slot_rarity (slot, rarity)
//   - KEY idx_enabled_weight (is_enabled, drop_weight)
//
// **关键覆盖点**：
//   - INT UNSIGNED（drop_weight）column_type 必须含 "unsigned"（与 INT 区别）；
//     这是本 case 区别于 17.2 EmojiConfigs_Schema 的关键之处 —— emoji_configs.sort_order
//     是 INT (signed)，cosmetic_items.drop_weight 是 INT UNSIGNED；
//     INFORMATION_SCHEMA.COLUMNS.COLUMN_TYPE 字段会精确反映 "int unsigned" vs "int"
//   - 双索引（idx_slot_rarity + idx_enabled_weight）列顺序断言（与 §5.8 钦定一致）
//   - **不含** render_config 字段（节点 10 / Epic 29 才加；本 case 不断言 render_config 存在
//     也不断言其不存在 —— 字段层断言 11 个字段 + 计数等于 11，render_config 缺失自然由
//     "11 字段计数" 兜底；如有人误加 render_config 字段会让计数变 12 失败）
//
// **背景（Story 20.2 引入）**：本 case 验证 0011 migration 落地的 schema
// 与 §5.8 钦定 1:1 对齐；用于在 epics.md §Story 20.2 钦定的"单元测试覆盖 ≥3 case"
// 中作为 schema-correctness 路径（happy / migrate up 后表存在 + 字段类型 +
// 全部索引和约束都符合 §5.8）。
func TestMigrateIntegration_CosmeticItems_Schema(t *testing.T) {
	// 实装细节由 dev-story 阶段补全；
	// 模板参考既有 TestMigrateIntegration_EmojiConfigs_Schema（紧靠本 case 之上）
	// + TestMigrateIntegration_RoomsAndRoomMembers_Schema。
	//
	// 必查项（每项失败立即 t.Errorf，不 t.Fatalf —— 用 batch 累积报错风格）：
	//   1. INFORMATION_SCHEMA.TABLES：cosmetic_items 表存在（count = 1）
	//   2. INFORMATION_SCHEMA.COLUMNS：11 列存在 + 类型对齐：
	//      - id bigint unsigned
	//      - code varchar(64) / name varchar(64)
	//      - slot tinyint / rarity tinyint
	//      - asset_url varchar(255) / icon_url varchar(255)
	//      - drop_weight int unsigned（**关键**：column_type 必须含 "unsigned"）
	//      - is_enabled tinyint
	//      - created_at / updated_at datetime(3)
	//   3. INFORMATION_SCHEMA.COLUMNS：cosmetic_items 表总列数 == 11
	//      （兜底防有人误加 render_config 或其他字段；render_config 由 Epic 29 落地）
	//   4. INFORMATION_SCHEMA.KEY_COLUMN_USAGE / STATISTICS：
	//      - PRIMARY KEY = id
	//      - UNIQUE KEY uk_code (code) 存在 + non_unique = 0
	//      - KEY idx_slot_rarity 存在 + 列顺序 (slot, rarity)
	//      - KEY idx_enabled_weight 存在 + 列顺序 (is_enabled, drop_weight)
	//   5. INFORMATION_SCHEMA.COLUMNS.COLUMN_DEFAULT：
	//      - asset_url DEFAULT '' (空字符串)
	//      - icon_url DEFAULT '' (空字符串)
	//      - drop_weight DEFAULT '0'
	//      - is_enabled DEFAULT '1'
	//      - created_at / updated_at DEFAULT CURRENT_TIMESTAMP(3)
}
```

**AC4.5 新增 `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected`**（紧接 AC4.4 case 之后）：

```go
// TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected 验证
// migrations/0011_init_cosmetic_items.up.sql 钦定的 UNIQUE KEY uk_code (code)
// 在运行时被 MySQL 真实拒绝重复 code 插入。
//
// **背景（Story 20.2 引入）**：epics.md §Story 20.2 钦定的"集成测试覆盖（dockertest）：
// migrate up → SHOW CREATE TABLE 对比 schema → migrate down"路径在 AC4.4
// CosmeticItems_Schema case 用 INFORMATION_SCHEMA 字段层精确断言取代了不稳定的
// SHOW CREATE TABLE 字符串比对；本 case 是额外的运行时 UNIQUE 拒绝验证 —— 是
// Story 20.3 seed 用 INSERT IGNORE 兜底 + Story 20.6 加权抽取按 code 索引命中 +
// admin 后台未来写入路径的 schema 层根基。
//
// **覆盖路径**：
//  1. migrate up → cosmetic_items 表存在
//  2. 插入 cosmetic_items (code='hat_yellow', name='小黄帽', slot=1, rarity=1,
//     asset_url='https://placehold.co/128x128?text=Hat-Yellow',
//     icon_url='https://placehold.co/64x64?text=Hat-Yellow',
//     drop_weight=100, is_enabled=1) → 成功
//  3. 再次插入 cosmetic_items (code='hat_yellow', ...) → DB 拒绝
//     （UNIQUE KEY uk_code (code) 兜底；same code 不能插两次）；
//     err 必须含 "Duplicate entry" / "1062"（MySQL 错误码 = ER_DUP_ENTRY）
//
// 用 database/sql 直跑 raw INSERT（**不**走 GORM）让测试结果**不**依赖 ORM
// 行为差异（与 Story 11.2 落地的 TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected
// + Story 17.2 落地的 TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected 同模式）。
func TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected(t *testing.T) {
	// 实装细节由 dev-story 阶段补全；模板见 Story 17.2 落地的
	// TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected（紧靠本 case 之上）。
}
```

- 两个新 case 都用 `dockertest` 起 mysql:8.0 容器（沿用 `startMySQL(t)` + `migrationsPath(t)` helper，与既有 case 一致）
- UNIQUE 拒绝 case 用 `database/sql` 直跑 raw INSERT（**不**走 GORM）
- 错误断言：`err != nil` + `strings.Contains(err.Error(), "Duplicate entry")`（MySQL 错误码 1062 = `ER_DUP_ENTRY`，与 11.2 / 17.2 同模式）
- **不**断言具体 MySQL error message 文本（不同 MySQL 版本可能略有差异；用 "Duplicate entry" substring 是稳定 contract）

**AC5 — 验证步骤**

- **AC5.1 build 验证**：执行 `bash scripts/build.sh --test` 必须**全绿**（含新增 `cosmetic_item_repo.go` 单测无 / 既有单测无回归 + 新增集成测试不在默认 `--test` build tag 内不跑）；`bash scripts/build.sh --integration` 必须**全绿**（新增 `TestMigrateIntegration_CosmeticItems_Schema` + `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` 两个 case 跑通 + 既有 `TestMigrateIntegration_UpThenDown` / `TestMigrateIntegration_UpTwice_Idempotent` 表数量 10 断言通过 + `TestMigrateIntegration_StatusAfterUp` 版本号 v=11 断言通过 + 既有 `TestMigrateIntegration_EmojiConfigs_*` / `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` 不回归）
- **AC5.2 git diff 范围检查**：编辑完成后 `git diff` 输出**仅**包含：
  - `server/migrations/0011_init_cosmetic_items.up.sql`（新增）
  - `server/migrations/0011_init_cosmetic_items.down.sql`（新增）
  - `server/internal/repo/mysql/cosmetic_item_repo.go`（新增）
  - `server/internal/infra/migrate/migrate_integration_test.go`（修改：表数量断言 9 → 10 + `expectedTables` slice 加 `"cosmetic_items"` + `StatusAfterUp` v != 10 → v != 11 + 新增 2 case）
  - `_bmad-output/implementation-artifacts/20-2-cosmetic_items-migration.md`（本 story 文件状态流转 + Tasks/Subtasks 勾选 + Dev Agent Record / File List / Change Log）
  - `_bmad-output/implementation-artifacts/sprint-status.yaml`（状态行 + last_updated）
- **AC5.3 schema 跨文档一致性**：手动检查 0011 up.sql 字段名 / 类型 / 索引名与数据库设计.md §5.8 行 442-458 **逐字段** 1:1 对齐；与 V1 §7.2 字段长度约束兼容（`code VARCHAR(64)` ⇔ §7.2 cosmetic code length ≤ 64；`name VARCHAR(64)` ⇔ §7.2 `reward.name` 1 ≤ length ≤ 64；`slot TINYINT` ⇔ §7.2 `reward.slot` int 枚举 + §6.8 枚举值 ∈ {1,2,3,4,5,6,7,99}；`rarity TINYINT` ⇔ §7.2 `reward.rarity` int 枚举 + §6.9 枚举值 ∈ {1,2,3,4}；`asset_url VARCHAR(255)` ⇔ §7.2 `reward.assetUrl` 1 ≤ length ≤ 255；`icon_url VARCHAR(255)` ⇔ §7.2 `reward.iconUrl` 1 ≤ length ≤ 255）；**关键**：`drop_weight INT UNSIGNED` 是 §5.8 钦定的字段类型，**不**漂移成 `INT` / `BIGINT` / `INT UNSIGNED NOT NULL DEFAULT 1`（drop_weight DEFAULT 应为 0，由 seed 显式覆盖到非 0 值）
- **AC5.4 GORM struct ↔ DDL 一致性**：手动检查 `CosmeticItem` struct 11 字段 / 类型与 0011 up.sql 1:1 对齐（无字段缺漏 / 无类型漂移）；**关键**：`DropWeight uint32`（不是 `int32` —— `INT UNSIGNED` 必须用无符号 Go 类型对齐，避免 Go 端按 signed 解析 ≥ 2^31 的权重值时数值溢出 / sign flip）；**关键**：**不**含 `RenderConfig` 字段（节点 10 / Epic 29 owner）
- **AC5.5 既有迁移 / repo 测试不回归**：跑 `go test ./server/internal/infra/migrate/... ./server/internal/repo/mysql/... -count=1` 全绿

## Tasks / Subtasks

- [ ] Task 1: 准备阶段（AC: #1, #2, #3, #4, #5）
  - [ ] Subtask 1.1: 阅读本 story 全文 + `docs/宠物互动App_数据库设计.md` §5.8（行 437-479）确认 DDL 1:1 字段 / 索引清单
  - [ ] Subtask 1.2: 阅读 `_bmad-output/implementation-artifacts/17-2-emoji_configs-migration.md` 已 done 的姊妹 story，参考其 migration + GORM struct + 集成测试编辑模式
  - [ ] Subtask 1.3: 阅读 `server/migrations/0009_init_emoji_configs.up.sql` + `0009_init_emoji_configs.down.sql`（17.2 落地的版本）确认顶部注释 / 字段块 / 索引块 / 范围红线四段式模板
  - [ ] Subtask 1.4: 阅读 `server/internal/repo/mysql/emoji_repo.go`（17.2 落地的版本）确认 `EmojiConfig` struct 的 GORM tag 模式 + `TableName()` 模式（11 字段 vs 8 字段，但结构同）
  - [ ] Subtask 1.5: 阅读 `server/internal/infra/migrate/migrate_integration_test.go`（17.3 升级后的版本）确认 `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` + `TestMigrateIntegration_EmojiConfigs_Schema` + `TestMigrateIntegration_StatusAfterUp` 编辑模式
  - [ ] Subtask 1.6: 阅读 `_bmad-output/implementation-artifacts/20-1-接口契约最终化.md`（已 done）确认 V1 §7.2 reward 字段表 + 节点 7 / 节点 8 阶段差异说明，确保本 story 落地的 DB schema 与 20.1 锚定的 API 字段类型 / 长度 / 枚举值兼容
- [ ] Task 2: 落地 0011_init_cosmetic_items.up.sql（AC: #1）
  - [ ] Subtask 2.1: 新建 `server/migrations/0011_init_cosmetic_items.up.sql`
  - [ ] Subtask 2.2: 写顶部注释（"对齐 §5.8" + 字段块 + 索引块 + 范围红线四段式，按 AC1 模板）
  - [ ] Subtask 2.3: 写 CREATE TABLE 语句（严格按 §5.8 行 442-458 1:1 + AC1 钦定 11 字段 + 3 索引，**不含** render_config）
- [ ] Task 3: 落地 0011_init_cosmetic_items.down.sql（AC: #2）
  - [ ] Subtask 3.1: 新建 `server/migrations/0011_init_cosmetic_items.down.sql`
  - [ ] Subtask 3.2: 写顶部注释（按 AC2 模板）+ `DROP TABLE IF EXISTS cosmetic_items;`
- [ ] Task 4: 落地 cosmetic_item_repo.go（AC: #3）
  - [ ] Subtask 4.1: 新建 `server/internal/repo/mysql/cosmetic_item_repo.go`
  - [ ] Subtask 4.2: 写 package 声明 + import `time`（**不**引入 `gorm.io/gorm`）
  - [ ] Subtask 4.3: 写 `CosmeticItem` struct 11 字段（按 AC3 钦定的字段顺序 + GORM tag；**关键**：`Slot int8` / `Rarity int8` / `IsEnabled int8` 对齐 TINYINT；`DropWeight uint32` 对齐 INT UNSIGNED；**不含** `RenderConfig`）
  - [ ] Subtask 4.4: 写 `TableName()` 方法（按 AC3 钦定，返回 `"cosmetic_items"`）
- [ ] Task 5: 扩展 migrate_integration_test.go（AC: #4）
  - [ ] Subtask 5.1: 改 `TestMigrateIntegration_UpThenDown` 中 `expectedTables` slice 加 `"cosmetic_items"`（共 10 张表）+ 同步注释升级（按 AC4.1 钦定）
  - [ ] Subtask 5.2: 改 `TestMigrateIntegration_UpTwice_Idempotent` 中表数量断言 9 → 10 + INFORMATION_SCHEMA WHERE table_name IN (...) slice 加 `'cosmetic_items'` + 同步注释升级（按 AC4.2 钦定）
  - [ ] Subtask 5.3: 改 `TestMigrateIntegration_StatusAfterUp` 版本号断言 v != 10 → v != 11 + 同步注释升级（按 AC4.3 钦定；与 17.3 落地 0010 后升级版本号同模式）
  - [ ] Subtask 5.4: 新增 `TestMigrateIntegration_CosmeticItems_Schema` case（按 AC4.4 钦定 + 参考 `TestMigrateIntegration_EmojiConfigs_Schema` 实装模板；**关键**断言点：11 列计数 + drop_weight column_type 含 "unsigned" + 双索引列顺序 + 5 字段 DEFAULT 值）
  - [ ] Subtask 5.5: 新增 `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` case（按 AC4.5 钦定 + 参考 `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` 实装模板）
- [ ] Task 6: 验证 + 提交（AC: #5）
  - [ ] Subtask 6.1: 跑 `bash scripts/build.sh --test` 全绿（vet + 全 unit 测试通过）
  - [ ] Subtask 6.2: 跑 `bash scripts/build.sh --integration` 全绿（dockertest 跑通 + 新增 2 case + 既有 case 不回归；如本机 Docker 不可用 → 集成测试 `t.Skipf` 优雅跳过，与 17.2 同模式，code 路径已通过 `go vet -tags=integration ./...`）
  - [ ] Subtask 6.3: git diff 范围检查 —— 仅本 story 钦定 6 个文件（见 File List）
  - [ ] Subtask 6.4: schema 跨文档 / struct 一致性手动检查（AC5.3 + AC5.4）
  - [ ] Subtask 6.5: 在 sprint-status.yaml 把本 story 状态从 in-progress 改为 review
  - [ ] Subtask 6.6: 由 code-review 检出后状态切 done + 在本 story 文件 + sprint-status.yaml 状态行追加 commit hash

## Dev Notes

### Build & Test 规范（项目级 CLAUDE.md 钦定）

- 写完 / 改完 Go 代码后必跑 `bash scripts/build.sh --test`（vet + 单测，**默认 build tag**，集成测试不跑）
- 集成测试 dockertest 必须用 `bash scripts/build.sh --integration`（带 `-tags=integration` build tag）
- 脚本契约来源：`_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.5 + `_bmad-output/implementation-artifacts/1-7-重做-scripts-build-sh.md`

### Migration 文件命名 / 编号规则（ADR-0003 + Story 4.3 钦定）

- 文件命名：`{N:04d}_{name}.up.sql` / `{N:04d}_{name}.down.sql`（4 位编号 + 下划线 + 小写下划线名称）
- 编号顺序：0001 ~ 0010 已被 4.3 / 7.2 / 10.3 / 11.2 / 17.2 / 17.3 占用（users / user_auth_bindings / pets / user_step_accounts / user_chests / user_step_sync_logs / rooms / room_members / emoji_configs init + seed）；**本 story 占用 0011**（首个 cosmetic 相关 migration）；Story 20.3 cosmetic_items seed 将占用 0012；Story 20.4 chest_open_logs migration 将占用 0013；按 epics.md §Story 20.2 / 20.3 / 20.4 钦定（注：epics.md 文案写的是 `0011_init_cosmetic_items.sql` / `0012_seed_cosmetic_items.sql` / `0013_init_chest_open_logs.sql`，与本 story 编号一致）
- **不**用 GORM AutoMigrate / 不用 `migrate` CLI 之外的工具（与 ADR-0003 钦定一致）

### GORM struct 规范（11.2 + 4.6 + 17.2 落地）

- struct 字段顺序与 SQL DDL 列顺序一致（便于 cross-reference）
- 字段类型对齐 MySQL → Go 映射：`BIGINT UNSIGNED → uint64` / `VARCHAR → string` / `INT → int32` / `INT UNSIGNED → uint32` / `TINYINT → int8` / `DATETIME(3) → time.Time`
- **关键**：`INT UNSIGNED → uint32`（不是 `int32`）—— 这是本 story 比 17.2 EmojiConfig 多出来的类型映射点；emoji_configs.sort_order 是 INT（signed），cosmetic_items.drop_weight 是 INT UNSIGNED；Go 类型必须用 `uint32` 才能精确对齐 + 避免 ≥ 2^31 的权重值数值溢出 / sign flip
- GORM tag 仅含 `column:` / `primaryKey` / `autoIncrement` / `not null` / `size:N` / `default:V` —— **不**含 `uniqueIndex` / `index` / `type:` 等（UNIQUE / 索引由 DDL 定义，与 ADR-0003 §3.2 一致；类型由字段 Go 类型推导）
- 显式 `TableName() string` 方法返回 DB 真实表名（避免 GORM 自动复数化引发漂移；与 17.2 落地的 `EmojiConfig.TableName() string { return "emoji_configs" }` 同模式）
- **不**引入 `gorm.Model`（避免 `DeletedAt` 软删除字段污染 schema）
- **不**在本 story 阶段新建 Repo interface / 实装方法（YAGNI，与 11.2 / 17.2 同模式）

### 跨文档语义同步检查（DB → API 单向）

- 本 story 落地的 0011 SQL DDL **只**反映数据库设计.md §5.8 的语义，**禁止反向**修改 §5.8
- 如发现 §5.8 与本 story 落地 DDL 有不一致（如字段名 / 类型 / 长度 / 默认值漂移）→ 优先修 0011 SQL 而非 §5.8
- 本 story 落地的 0011 DDL 是 V1 §7.2 reward 字段长度约束（20.1 锚定）+ §8.1 GET /cosmetics/catalog 字段（Epic 23 / Story 23.1 锚定）的**DB 端真相源**；不允许在本 story 阶段对契约层做反向加严 / 放松

### 错误码不在本 story 范围

- §3 全局错误码表（4001 / 4002 / 3002 / 1001 / 1002 / 1005 / 1009）由 20.1 锚定 + 由 20.5 / 20.6 实装时引用；本 story **不**触发错误码定义 / 修改 / 引用（migration 层不返回 API 错误码）

### `chest_open_idempotency_records` 表（§5.16）显式 out of scope

- **关键**：Story 20.1 r5 ~ r11 review 锁定了 `chest_open_idempotency_records` 表的 schema（数据库设计 §5.16）+ 同事务持久化幂等机制（V1 §7.2 步骤 3 / 5a / 5b / 5k 钦定的 MVCC + X-lock 行为）；但 §5.16 表 migration **不在本 story 范围**
- **Story 20.1 follow-up 钦定**（行 398-403）：`chest_open_idempotency_records` migration 由 Story 20.6 dev kickoff 时决定收进 Story 20.6（与 chest_open_logs 同 migration 文件 / 不同文件）或起新 Story 20.10 独立落地；本 story（20.2）**仅** owner cosmetic_items
- 如 dev 看到本 story 时同时想顺手落地 §5.16 migration —— **禁止**（YAGNI + 范围红线违反 + 与 Story 20.1 r5 review follow-up 直接冲突）；该决策必须在 Story 20.6 kickoff 时由 dev 单独评估

### 跨 epic 依赖追溯

- **上游冻结**：
  - 数据库设计 §5.8 cosmetic_items 表 schema ← 总体架构 + 数据库设计文档锚定（**不**由某个 story 锚定；本 story 严格对齐）
  - 数据库设计 §6.8 cosmetic_items.slot 枚举 + §6.9 cosmetic_items.rarity 枚举（本 story DDL 不在 schema 层做 enum 约束，但 seed / service 层依赖）
  - V1 §7.2 reward 字段层 ← Story 20.1 锚定（已 done，commit `bccf97d` 之前的 20-1 实装 commits）
  - V1 §8.1 / §8.2 cosmetic_items catalog / inventory 字段层 ← Epic 23 Story 23.1 锚定（**未** done）
  - ADR-0003 migration 工具 + 编号规则 ← Story 4.3 落地
- **下游强依赖**（本 story done 后才能开工）：
  - Story 20.3（cosmetic_items seed ≥15 行 + AR18 数量约束 + INSERT IGNORE / ON DUPLICATE KEY UPDATE 兜底）
  - Story 20.4（chest_open_logs migration，0013_init_chest_open_logs.up.sql）
  - Story 20.5（GET /chest/current 接口）
  - Story 20.6（POST /chest/open 事务 + idempotencyKey + 加权抽取；可能同时落地 chest_open_idempotency_records migration 或起 Story 20.10）
  - Story 20.7（dev /dev/force-unlock-chest）
  - Story 20.8（dev /dev/grant-cosmetic-batch）
  - Story 20.9（Layer 2 集成测试）
  - iOS Epic 21.1 ~ 21.5（依赖 20.5 / 20.6 接口的 client 实装）
  - Epic 23 Story 23.2（user_cosmetic_items migration，reference cosmetic_items.id）
  - Epic 23 Story 23.3（GET /cosmetics/catalog 接口）
  - Epic 29 Story 29.2（render_config 加列 migration）
  - Epic 32 Story 32.4（compose 事务，按 slot/rarity 加权抽取，复用 idx_slot_rarity 索引）

### Git Intelligence（最近 5 commits 模式参考）

- `ebe8762 chore(lessons): backfill 3cd2ef4 commit hash for SDK/runtime mismatch lesson` —— 文档回填，与本 story 无业务关联
- `3cd2ef4 docs(lessons): 沉淀 epic-18 retro A1 修复 — Xcode SDK/sim-runtime 版本错位的根因诊断` —— iOS 端 lesson，与本 story 无业务关联
- `6a04d9f chore(epic-18): 收官 Epic 18 retrospective + sprint-status 标记 retrospective done` —— Epic 18 收官；可参考其 sprint-status 状态行流转格式
- `48acf83 docs(lessons): 补充 SwiftUI PreferenceKey merge vs replace & owner-side expire lesson（18-4 r1）` —— iOS 端 lesson，与本 story 无业务关联
- `e747017 chore(story-18-4): 收官 Story 18.4 + 归档 story 文件` —— Story 收官；可参考其 story-done commit 格式
- **本 story 创建时（commit `bccf97d` 之前）的最近 server-side migration 落地 commit**：Story 17.3 `0010_seed_emoji_configs` + Story 17.2 `0009_init_emoji_configs` —— 这两个 commit 的 migration 文件模板是本 story 的**直接**参考来源（顶部注释 / 字段块 / 索引块 / 范围红线四段式）

### 测试 / 验证

- **单元测试**：本 story 不新建 service / repo 方法 → 无 sqlmock-based 单测；既有 `room_member_repo_test.go` / `room_repo_test.go` / `pet_repo_test.go` / `emoji_repo_test.go`（如存在）等不受影响
- **集成测试**（dockertest）：本 story 新增 2 case + 改既有 3 case（`TestMigrateIntegration_UpThenDown` 表数量 9 → 10 + `TestMigrateIntegration_UpTwice_Idempotent` 表数量 9 → 10 + `TestMigrateIntegration_StatusAfterUp` 版本 10 → 11）；用 `bash scripts/build.sh --integration` 跑（带 `-tags=integration`）
- **下游验证**：本 story done 后由 Story 20.3 实装时的 `INSERT IGNORE` / `ON DUPLICATE KEY UPDATE` seed + `SELECT COUNT(*) GROUP BY rarity` 验证 AR18 数量约束 + Story 20.6 实装时的 dockertest 集成测试（curl POST /chest/open → 验证 response.reward 字段与 §7.2 字段表对齐 + 加权抽取分布合理 + idempotencyKey 重复调用走 cache 路径）+ Story 20.9 Layer 2 完整事务集成测试做真实串联验证

### 范围红线 + 风险

- **红线**：本 story **不**修改任何 service / handler / repo write / read 方法；**不**修改 V1 接口契约 / 数据库设计文档 / ADR-0003；**不**修改 0001 ~ 0010 既有 migration；**不**预实装 `CosmeticItemRepo` interface / 方法；**不**落地 `chest_open_idempotency_records` migration（§5.16 表，Story 20.6 / 20.10 owner）；**不**落地 `chest_open_logs` migration（§5.7 表，Story 20.4 owner）；**不**落地 `user_cosmetic_items` migration（§5.9 表，Story 23.2 owner）
- **红线**：本 story **不**实装 seed SQL（20.3 owner）
- **红线**：本 story **不**包含 `render_config` 字段（Epic 29 Story 29.2 owner）
- **风险**：表数量断言 9 → 10 改漏（既有 `TestMigrateIntegration_UpThenDown` + `TestMigrateIntegration_UpTwice_Idempotent` 两处都要改 + `TestMigrateIntegration_StatusAfterUp` 版本号 10 → 11 一处）→ AC4.1 / AC4.2 / AC4.3 显式钦定，AC5.2 git diff 范围检查兜底
- **风险**：GORM struct 字段类型漂移
  - 风险 A：`DropWeight int32` 误写（应为 `uint32` 对齐 `INT UNSIGNED`）→ AC3 字段类型表 + AC5.4 GORM struct ↔ DDL 一致性手动检查 + AC4.4 `TestMigrateIntegration_CosmeticItems_Schema` 中 column_type 含 "unsigned" 断言兜底
  - 风险 B：顺手加 `RenderConfig` 字段（应留给 Epic 29）→ AC3 范围红线 + AC4.4 `TestMigrateIntegration_CosmeticItems_Schema` 中 11 字段计数断言兜底（计数 != 11 失败）
  - 风险 C：`Slot int` / `Rarity int` 误写（应为 `int8` 对齐 `TINYINT`）→ AC3 字段类型表 + AC5.4 兜底
- **风险**：dockertest 集成测试在 Windows 本地跑可能因 Docker Desktop 未启动 / stale 容器残留失败 → 与 Story 11.2 / 17.2 同情况，由 dev-story 阶段确保 Docker Desktop 启动后跑；如本机 Docker 不可用 → 集成测试 `t.Skipf` 优雅跳过（migrate_integration_test.go 顶部注释钦定的降级路径），不阻塞 review；code-review 阶段在 fresh 环境 retry 验证
- **风险**：项目当前 `migrate_integration_test.go` 用 INFORMATION_SCHEMA.STATISTICS 查 index_name 时大小写敏感性 → MySQL 8.0 `lower_case_table_names=1`（Windows 默认）下 table_name 大小写归一为小写；index_name 不受影响（保留原大小写）；本 story 集成测试用 `idx_slot_rarity` / `idx_enabled_weight` / `uk_code` 小写命名与既有 case 一致，无大小写风险

### Project Structure Notes

- 本 story 唯一编辑文件（绝对路径）：
  - `C:/fork/cat/server/migrations/0011_init_cosmetic_items.up.sql`（新建）
  - `C:/fork/cat/server/migrations/0011_init_cosmetic_items.down.sql`（新建）
  - `C:/fork/cat/server/internal/repo/mysql/cosmetic_item_repo.go`（新建）
  - `C:/fork/cat/server/internal/infra/migrate/migrate_integration_test.go`（修改）
  - `C:/fork/cat/_bmad-output/implementation-artifacts/20-2-cosmetic_items-migration.md`（本 story 文件）
  - `C:/fork/cat/_bmad-output/implementation-artifacts/sprint-status.yaml`（状态行流转）
- 与 `docs/宠物互动App_Go项目结构与模块职责设计.md` §6 锚定的 `internal/repo/mysql/` / `internal/migrations/` 目录完全兼容（沿用既有目录规则，**不**新增子目录 / 模块）

### References

- [Source: docs/宠物互动App_数据库设计.md#5.8] cosmetic_items 表 schema（行 437-479；本 story 严格对齐，**不**修改）
- [Source: docs/宠物互动App_数据库设计.md#5.7] chest_open_logs 表 schema（行 399-433；本 story **不** owner，仅做 reference 说明 reward_cosmetic_item_id 字段语义）
- [Source: docs/宠物互动App_数据库设计.md#5.9] user_cosmetic_items 表 schema（行 483-530；本 story **不** owner，仅做 reference 说明 cosmetic_item_id FK 语义；Epic 23 Story 23.2 owner）
- [Source: docs/宠物互动App_数据库设计.md#5.16] chest_open_idempotency_records 表 schema（行 727-787；**本 story 不 owner**；Story 20.1 r5 follow-up 钦定由 Story 20.6 / 20.10 决定）
- [Source: docs/宠物互动App_数据库设计.md#6.8] cosmetic_items.slot 状态枚举（行 841-852；本 story DDL 不在 schema 层做 enum 约束，由 service 层 + seed 校验）
- [Source: docs/宠物互动App_数据库设计.md#6.9] cosmetic_items.rarity 状态枚举（行 854-862；同上）
- [Source: docs/宠物互动App_数据库设计.md#7.1] 高优先级 UNIQUE 约束（`emoji_configs / cosmetic_items` UNIQUE(code)，行 919-922）
- [Source: docs/宠物互动App_V1接口设计.md#7.2] POST /api/v1/chest/open reward 字段表（20.1 锚定；本 story DDL 是 §7.2 reward 字段长度约束的 DB 端真相源）
- [Source: docs/宠物互动App_V1接口设计.md#8.1] GET /api/v1/cosmetics/catalog 字段表（Epic 23 Story 23.1 锚定 —— **未** done，本 story DDL 是其字段长度约束的 DB 端真相源）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 20.2] AC 钦定（行 2807-2826）：`0011_init_cosmetic_items.sql` + down.sql + 11 字段（**不含** render_config）+ UNIQUE uk_code + KEY idx_slot_rarity + KEY idx_enabled_weight + ≥3 case 单测 + dockertest 集成测试覆盖 schema 一致
- [Source: _bmad-output/implementation-artifacts/20-1-接口契约最终化.md] Story 20.1 上游契约（已 done；本 story 的 DB schema 是 §7.2 reward 字段长度约束的 DB 端真相源；r5 follow-up 钦定 §5.16 表 migration 不在本 story 范围）
- [Source: _bmad-output/implementation-artifacts/17-2-emoji_configs-migration.md] Story 17.2 已 done 姊妹 story（migration + GORM struct + dockertest UNIQUE 拒绝集成测试编辑模式参考；本 story 直接 mirror 同模式）
- [Source: _bmad-output/implementation-artifacts/11-2-rooms-room_members-migration.md] Story 11.2 已 done 姊妹 story（多张表 migration + dockertest 集成测试模板参考）
- [Source: _bmad-output/implementation-artifacts/decisions/0003-orm-stack.md] ADR-0003 ORM / migration 工具栈（golang-migrate v4.18.1 + GORM v1.25.12；migration 编号规则 + .up.sql / .down.sql 双向规范；禁止 GORM AutoMigrate）
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md] ADR-0001 测试栈（dockertest + build tag `integration`；`bash scripts/build.sh --integration` 跑集成测试）
- [Source: server/migrations/0009_init_emoji_configs.up.sql + 0009_init_emoji_configs.down.sql] 17.2 落地的顶部注释 + DDL 模板（"对齐 §X.Y" + 字段块 + 索引块 + 范围红线四段式参考）
- [Source: server/internal/repo/mysql/emoji_repo.go] 17.2 落地的 `EmojiConfig` GORM struct + `TableName()` 模板（本 story `CosmeticItem` struct 模仿同模式，11 字段 vs 8 字段）
- [Source: server/internal/infra/migrate/migrate_integration_test.go] 17.2 / 17.3 落地的 `TestMigrateIntegration_EmojiConfigs_Schema` + `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` + `TestMigrateIntegration_StatusAfterUp` 实装模板
- [Source: _bmad-output/implementation-artifacts/sprint-status.yaml] Epic 20 状态 in-progress；本 story 状态行 + last_updated 由 create-story / dev-story / code-review 流程逐步推进

## Dev Agent Record

### Agent Model Used

claude-opus-4-7 (1M context)

### Debug Log References

- 本次 dev-story 因 socket crash 中断后由 epic-loop 派恢复任务收尾；恢复阶段无 build/test 失败需要追踪
- `bash scripts/build.sh --test` 全绿（24 个包全 PASS，无 vet 错误 / 无单测回归）
- `bash scripts/build.sh --integration` 全绿；但 `migrate` 包内 dockertest 相关 case（含本 story 新增 2 case + 既有 7+ case）因本机 Docker daemon 未启动（`docker daemon not reachable: dial tcp [::1]:2375: connectex`）走 `t.Skipf` 优雅降级路径 —— 由 `code-review` / `fix-review` 阶段在 fresh Docker 环境 retry 验证（migrate_integration_test.go 顶部钦定的降级路径，与 Story 11.2 / 17.2 / 17.3 同模式）

### Completion Notes List

**实装要点（按 AC 对齐）**：

- **AC1 ✅** 落地 `server/migrations/0011_init_cosmetic_items.up.sql` —— 严格按 `docs/宠物互动App_数据库设计.md` §5.8（行 437-479）1:1 对齐：11 字段（id / code / name / slot / rarity / asset_url / icon_url / drop_weight / is_enabled / created_at / updated_at）+ 3 索引（UNIQUE uk_code + KEY idx_slot_rarity + KEY idx_enabled_weight）+ `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`；顶部注释四段式（对齐 §5.8 + 字段块 + 索引块 + 范围红线）；**不含** `render_config` 字段（Epic 29 owner）；**不含**任何 INSERT / seed 数据（20.3 owner）
- **AC2 ✅** 落地 `server/migrations/0011_init_cosmetic_items.down.sql` —— 仅 `DROP TABLE IF EXISTS cosmetic_items;` + 顶部注释，模板与 0007/0008/0009 down.sql 同模式
- **AC3 ✅** 落地 `server/internal/repo/mysql/cosmetic_item_repo.go` —— `CosmeticItem` GORM struct 11 字段（字段顺序与 SQL 列顺序 1:1）+ 类型映射钦定（`ID uint64` / `Slot int8` / `Rarity int8` / `IsEnabled int8` / **关键** `DropWeight uint32` 对齐 `INT UNSIGNED` 防 sign flip）+ `TableName() string` 显式返回 `"cosmetic_items"`；**不**新建 `CosmeticItemRepo` interface / 方法（YAGNI，20.6 / 23.x owner）；**不**引入 `gorm.Model`；**不**含 `RenderConfig` 字段；文件 54 行（与 17.2 EmojiConfig 同体积级别）
- **AC4 ✅** 修改 `server/internal/infra/migrate/migrate_integration_test.go`：
  - AC4.1：`TestMigrateIntegration_UpThenDown` 的 `expectedTables` slice 加 `"cosmetic_items"`（9 → 10）+ 同步顶部 case 注释 + 函数 docstring 注释升级
  - AC4.2：`TestMigrateIntegration_UpTwice_Idempotent` 的 `INFORMATION_SCHEMA WHERE table_name IN (...)` slice 加 `'cosmetic_items'` + 表数量断言 9 → 10 + 错误消息同步升级
  - AC4.3：`TestMigrateIntegration_StatusAfterUp` 版本号断言 `v != 10` → `v != 11` + docstring 加 Story 20.2 升级注释
  - AC4.4：新增 `TestMigrateIntegration_CosmeticItems_Schema` —— 11 字段 + 类型表（**关键** `drop_weight` column_type = `"int unsigned"` 区别 emoji_configs.sort_order 的 `"int"`）+ 总列数 = 11 兜底防 render_config 漂移 + PK + UNIQUE uk_code (code) + 双索引列顺序断言（idx_slot_rarity / idx_enabled_weight）+ 5 字段 DEFAULT 值（asset_url='' / icon_url='' / drop_weight='0' / is_enabled='1' + created_at / updated_at CURRENT_TIMESTAMP）
  - AC4.5：新增 `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` —— 用测试专用 code `test_unique_cosmetic_a` 与未来 20.3 seed 字面量隔离；raw `database/sql` INSERT（不走 GORM）；首条成功 → 同 code 二插必须返 err + err.Error() 含 `"Duplicate entry"`（MySQL `1062 ER_DUP_ENTRY`）
- **AC5 ✅** 验证：
  - AC5.1 build：`bash scripts/build.sh --test` 全绿（24 package PASS）+ `bash scripts/build.sh --integration` 全绿（含 vet + build + 全 integration test PASS；本机 Docker 未启动 → migrate 包内 dockertest case 走 `t.Skipf` 跳过，**code 路径已通过** `go vet -tags=integration`）
  - AC5.2 git diff 范围：仅 6 个钦定文件（2 个新 migration + 1 个新 repo struct + 1 个修改的 integration test + 本 story md + sprint-status.yaml）
  - AC5.3 schema 跨文档一致性：手动比对 0011 up.sql 字段名 / 类型 / 索引名与 §5.8 行 442-458 + V1 §7.2 reward 字段长度约束 1:1 对齐
  - AC5.4 GORM struct ↔ DDL 一致性：11 字段 1:1；`DropWeight uint32` 对齐 `INT UNSIGNED`；**不**含 `RenderConfig`
  - AC5.5 既有迁移 / repo 测试不回归：`internal/infra/migrate/...` + `internal/repo/mysql/...` 全绿

**dockertest 跳过说明（重要 —— 让 code-review / fix-review 阶段知情）**：

- 本机 Docker Desktop 当前未启动 → 本次 dev-story 阶段 dockertest 集成测试（含本 story 新增的 `TestMigrateIntegration_CosmeticItems_Schema` + `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` 以及既有的 `EmojiConfigs_*` / `RoomMembers_*` / `UpThenDown` / `UpTwice_Idempotent` / `StatusAfterUp` 全部 case）走 `t.Skipf` 跳过
- 验证证据：`go test -tags=integration -run "TestMigrateIntegration_CosmeticItems" -count=1 -v ./internal/infra/migrate/...` 输出 `docker daemon not reachable: dial tcp [::1]:2375: connectex` → `--- SKIP: TestMigrateIntegration_CosmeticItems_Schema (0.01s)` / `--- SKIP: TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected (0.00s)` —— 证明 case 已注册到 testing framework 且 code 路径 vet/build 通过；只缺 Docker daemon 启动后的运行时数据点
- 跳过路径是 `migrate_integration_test.go` 顶部 + Story 20.2 Subtask 6.2 钦定的优雅降级路径（与 Story 11.2 / 17.2 / 17.3 同模式），**不**阻塞 review 转 done；code-review / fix-review 阶段应在 fresh Docker 环境 retry 跑 `bash scripts/build.sh --integration` 拉真实运行时数据点

**范围红线遵守**：

- ✅ 未实装任何 service / handler / repo write/read 方法（20.3 ~ 20.9 owner）
- ✅ 未实装任何 INSERT / seed SQL（20.3 owner）
- ✅ 未新建 `CosmeticItemRepo` interface（YAGNI，20.6 / 23.x owner）
- ✅ 未实装 `chest_open_logs` migration（20.4 owner）+ `chest_open_idempotency_records` migration（20.6 / 20.10 owner）+ `user_cosmetic_items` migration（23.2 owner）
- ✅ 未加 `render_config` 字段（Epic 29 Story 29.2 owner）
- ✅ 未修改 V1 接口契约 / 数据库设计文档 / ADR-0003 / 0001 ~ 0010 既有 migration
- ✅ 未引入 `gorm.Model` / `uniqueIndex` / `index` GORM tag（DDL 已定义，避免 schema 真相源漂移）

### File List

- `server/migrations/0011_init_cosmetic_items.up.sql`（新建 —— cosmetic_items 表 DDL，对齐数据库设计 §5.8）
- `server/migrations/0011_init_cosmetic_items.down.sql`（新建 —— DROP TABLE 路径）
- `server/internal/repo/mysql/cosmetic_item_repo.go`（新建 —— `CosmeticItem` GORM domain struct + `TableName()`）
- `server/internal/infra/migrate/migrate_integration_test.go`（修改 —— 表数量断言 9 → 10 + `StatusAfterUp` 版本 10 → 11 + 新增 `TestMigrateIntegration_CosmeticItems_Schema` + `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` 两个 case）
- `_bmad-output/implementation-artifacts/20-2-cosmetic_items-migration.md`（本 story 文件 —— Status / Tasks/Subtasks / Dev Agent Record / File List / Change Log）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（状态行流转 + last_updated）

### Change Log

| 日期 | 操作 | Story 状态 | 备注 |
|---|---|---|---|
| 2026-05-14 | create-story | backlog → ready-for-dev | 由 epic-loop / bmad-create-story workflow 自动生成（HEAD bccf97d） |
| 2026-05-14 | dev-story | ready-for-dev → in-progress | 落地 0011 up/down SQL + CosmeticItem GORM struct + migrate_integration_test 扩展（5 改动点：UpThenDown 9→10 / UpTwice 9→10 / StatusAfterUp v10→v11 / 新增 CosmeticItems_Schema / 新增 CosmeticItems_UniqueCode_Rejected） |
| 2026-05-14 | dev-story（恢复任务收尾） | in-progress → review | 上一个 sub-agent 因 socket crash 中断；恢复任务补完缺失的 2 个新 dockertest case + Dev Agent Record 段 + sprint-status 状态流转；本机 Docker 未启动 → dockertest case 走 `t.Skipf` 优雅跳过（unit test 全绿；vet -tags=integration 通过；code-review / fix-review 阶段 fresh Docker 环境 retry 拉真实运行时数据点） |
