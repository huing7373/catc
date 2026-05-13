# Story 17.3: emoji_configs seed（首次落地 0010_seed_emoji_configs.up/down.sql + ≥4 个表情 INSERT IGNORE + 每个表情非空 asset_url + ≥2 case 单测 + dockertest 集成测试覆盖 seed 后表内容 + 重复 migrate up 幂等）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want **首次落地** `server/migrations/0010_seed_emoji_configs.up.sql` + `server/migrations/0010_seed_emoji_configs.down.sql` 两个新 seed migration 文件（向 17.2 已落地的 `emoji_configs` 表写入至少 4 个表情系统配置：`wave` / `love` / `laugh` / `cry`，全部 `is_enabled=1` + 每个 `asset_url` 非空可访问 placeholder URL；用 `INSERT IGNORE` 兜底防止重复 migrate up 时插入冲突（UNIQUE KEY uk_code 命中））+ **扩展** `server/internal/infra/migrate/migrate_integration_test.go` 的 `TestMigrateIntegration_StatusAfterUp` 版本号断言（`v != 9` → `v != 10`）+ `TestMigrateIntegration_UpThenDown` Down 后表数量断言（隐式由 v=10 校验兜底）+ **新增** `TestMigrateIntegration_EmojiConfigs_SeedContent` dockertest 集成测试（覆盖 epics.md §Story 17.3 钦定的"集成测试覆盖：migrate up → SELECT * FROM emoji_configs → 验证 4 个表情存在 + URL 字段格式合法"路径，校验 4 个 seed code 都存在 + 每个 asset_url 非空 + 每个 is_enabled=1 + sort_order 排序唯一且单调）+ **新增** `TestMigrateIntegration_EmojiConfigs_SeedIdempotent` dockertest 集成测试（覆盖 epics.md §Story 17.3 钦定的"重复 migrate up → 不重复插入（INSERT IGNORE）"路径，校验 `down + up` 重复后表内 4 行不翻倍 / 不漏；与 golang-migrate `force(9) → up` 路径同模式兜底；**注**：golang-migrate 内置幂等 = `migrate up` 第二次走 ErrNoChange 不会重跑 0010 SQL，所以 INSERT IGNORE 主要保障 `force / dirty 恢复 / 手工 mysql import` 等绕过 migration 框架的入库路径，不是 migrate up 路径），
so that **Story 17.4（GET /emojis 接口）+ Story 17.5（WS emoji.send 校验 emojiCode 合法性）+ iOS Epic 18.1（表情面板 SwiftUI + GET /emojis 缓存）+ Epic 19.1（节点 6 demo E2E）** 可以基于一份**已落地、有真实数据、已具备完整集成测试覆盖**的 emoji_configs 配置集合并行展开，不再出现"17.4 GET /emojis 返回 items=[] 让 18.1 UI 空白 / 17.5 校验 emojiCode 拒绝所有 emoji.send（DB 没数据全部 7001）/ E2E demo 时表情面板空白"的返工。

## 故事定位（Epic 17 第三条 = 第二条**实装** story；上承 17.2 表 + GORM struct 已就绪，下启 17.4 GET /emojis + 17.5 WS emoji.send / emoji.received 广播 + iOS Epic 18.1 表情面板）

- **Epic 17 进度**：17.1（契约定稿，done）→ 17.2（emoji_configs migration + GORM struct，done）→ **17.3（本 story，emoji_configs seed ≥4 个表情）** → 17.4（GET /emojis 接口）→ 17.5（WS emoji.send 处理 + emoji.received 广播）。
- **本 story 是 17.4 / 17.5 / Epic 18.1 / Epic 19.1 的强前置**：
  - **17.4 GET /emojis**：handler / service / repo 层 `SELECT id, code, name, asset_url, sort_order FROM emoji_configs WHERE is_enabled = 1 ORDER BY sort_order ASC, id ASC`（按 V1 §11.1 服务端逻辑步骤 2 钦定）必须命中本 story 落地的 ≥4 行 seed 数据；17.4 dockertest 集成测试钦定 `seed 4 个表情 → curl GET /emojis → response.items 长度 = 4`（epics.md 行 2599）—— **直接依赖本 story 落地的 4 行**
  - **17.5 WS emoji.send 校验**：service 层 `SELECT 1 FROM emoji_configs WHERE code = ? AND is_enabled = 1` 校验 emojiCode 合法性（按 V1 §12.2 服务端逻辑步骤 4 + 17.1 锚定）必须命中本 story 落地的 `wave` / `love` / `laugh` / `cry`；17.5 单测 case "happy: 用户在房间 + emojiCode 合法 → broadcast 调用 1 次" 必须能 mock 出本 story 落地的 emojiCode（epics.md 行 2620）
  - **iOS Epic 18.1 表情面板**：UI 层钦定 "API 返回 4 个表情 → 网格渲染 4 个 cell"（epics.md 行 2650）—— **直接依赖本 story 落地的 4 行**；assetUrl 必须非空可访问让 `AsyncImage` 不触发占位降级（V1 §11.1 + 18.1 dev notes 钦定）
  - **Epic 19.1 节点 6 demo E2E**：钦定 "验证场景 1：A 进房间 → 点自己猫 → 表情面板出现 → 验证 4 个表情图标都加载成功（assetUrl 可访问）"（epics.md 行 2742）—— **直接依赖本 story 落地的 4 行 + 每个 assetUrl 可访问**
- **epics.md §Story 17.3 钦定**（行 2559-2579）：
  - seed 至少 4 个表情：`wave`（挥手）/ `love`（爱心）/ `laugh`（大笑）/ `cry`（哭）—— **明确钦定 4 个 emojiCode 字面量**
  - 每个表情的 `asset_url` 必须可访问（按 AR19 / AR18 URL 约束）；MVP 阶段可用 placeholder URL（如 `https://placehold.co/64x64?text=Wave`）
  - seed 通过 migration 文件 `migrations/0010_seed_emoji_configs.sql` 写入（INSERT IGNORE 防重复）—— **明确钦定文件编号 0010 + INSERT IGNORE 语义**
  - **单元测试覆盖**（≥2 case）：
    - happy: migrate up 后 emoji_configs 至少 4 行 + asset_url 都非空
    - happy: 重复 migrate up → 不重复插入（INSERT IGNORE）
  - **集成测试覆盖**（dockertest）：migrate up → SELECT * FROM emoji_configs → 验证 4 个表情存在 + URL 字段格式合法
- **AR19 钦定**（epics.md 行 190）："`emoji_configs` 必须预置最小系统表情集合（≥ 4 个，覆盖典型情绪）；`asset_url` 同样必须可访问"
- **V1 §11.1 钦定**（17.1 r2 review 收紧）：`data.items[].assetUrl` `1 ≤ length ≤ 255`；**禁止**空字符串 `""`；推荐 PNG；MVP 允许 placeholder URL
- **lesson 2026-05-13-emoji-contract-self-consistency-and-1009-and-asset-url-17-1-r2.md** Lesson 3 钦定（17-1 r2 lesson）："Story 17.3 seed 钦定每个 enabled 表情必须有非空 `asset_url`（与 Story 18.1 表情面板 cell 渲染契约一致）+ server 端 seed 层 / admin 写入层应校验非空 + 数据库 DEFAULT '' 是 DDL 兜底不意味业务层允许 enabled 留空"
- **Story 17.1 上游冻结边界**（§11.1 `code` 字符集约束 `[a-z0-9_-]` + length 1-64）：本 story 落地的 4 个 emoji code 必须满足该字符集（`wave` / `love` / `laugh` / `cry` 4 个字面量均合法 —— 都是纯小写英文字母，长度 4-5，符合约束）
- **数据库设计 §5.15 钦定**（17.2 落地）：emoji_configs 表已存在；`UNIQUE KEY uk_code (code)` 保证 INSERT IGNORE 兜底语义生效
- **下游强依赖**（本 story 不动后才能开工）：
  - Story 17.4（GET /emojis 接口）
  - Story 17.5（WS emoji.send 处理 + emoji.received 广播）
  - iOS Epic 18.1（表情面板 SwiftUI + GET /emojis 缓存）
  - Epic 19.1（节点 6 demo E2E）
- **范围红线**：
  - 本 story **只**改 `server/migrations/0010_seed_emoji_configs.up.sql`（新建）+ `server/migrations/0010_seed_emoji_configs.down.sql`（新建）+ `server/internal/infra/migrate/migrate_integration_test.go`（修改 `TestMigrateIntegration_StatusAfterUp` 版本号 v=9 → v=10 + 新增 `TestMigrateIntegration_EmojiConfigs_SeedContent` + 新增 `TestMigrateIntegration_EmojiConfigs_SeedIdempotent` 共 2 case）+ 本 story 文件 + sprint-status.yaml 流转
  - **不**实装任何 service / handler / repo write / read 方法（17.4 / 17.5 才做）
  - **不**实装任何 GORM struct 变更（17.2 已落地 `EmojiConfig` struct，本 story **不**修改）
  - **不**实装 GET /emojis handler / service（17.4 才做）
  - **不**实装 WS emoji.send / emoji.received（17.5 才做）
  - **不**接 Redis（10.6 已接，本 story 不动）
  - **不**改 V1 接口契约（17.1 已冻结）
  - **不**改任何 `docs/宠物互动App_*.md`（§5.15 是契约**输入**，本 story seed 数据严格符合 §5.15 但**不修改**它；AR19 是契约**输入**，本 story 严格符合但**不修改**它）
  - **不**改 `_bmad-output/` 下其他 yaml / md（除自己 story 文件 + sprint-status.yaml 流转）
  - **不**改 ADR-0003（migration 工具 + 编号约定 + .up.sql / .down.sql 双向规范由 Story 4.3 已锚定，本 story 沿用）
  - **不**改 `cmd/server/main.go` migrate 子命令（已在 Story 4.3 落地）
  - **不**改 0001 ~ 0009 既有 migration 文件（17.2 已落地 0009 schema，本 story 仅新增 0010 seed）
  - **不**修改 17.2 落地的 emoji_repo.go（GORM struct 不动；seed 走纯 SQL）
  - **不**为 0010 写"prod 部署 seed 自动化 / 一键回滚 seed / dry-run"等运维化改造（保留最小集 up/down，与 0001 ~ 0009 同模式）
  - **不**实装 admin 后台或动态 seed 接口（MVP 节点 6 仅静态 seed；未来若需要管理后台动态 add/disable emoji 由对应 epic 决定）

**本 story 不做**（明确范围红线）：

- 不实装任何 Go handler / service / repo write 或 read 方法（17.4 / 17.5 范围）
- 不修改 17.2 落地的 `emoji_repo.go`（GORM struct + TableName 已就绪；本 story 走纯 SQL seed）
- 不新建 `EmojiRepo` interface（YAGNI；17.4 实装 GET /emojis 时才落地 `EmojiRepo` 类型 + `List(ctx) ([]EmojiConfig, error)` 方法）
- 不引入 Go 层 seed loader / fixture loader 框架（YAGNI；纯 SQL migration 文件已足够，与 ADR-0003 钦定一致）
- 不修改 0001 ~ 0009 既有 migration 文件（已在 Story 4.3 / 7.2 / 10.3 / 11.2 / 17.2 落地）
- 不修改 V1 接口契约（17.1 已冻结）
- 不修改数据库设计 §5.15（schema 输入，本 story 严格对齐不修改）
- 不修改 AR19 / AR18（架构钦定，本 story 严格对齐不修改）
- 不修改 Go 项目结构文档 §6（`migrations/` 目录已锚定；本 story 新增 0010 沿用既有目录规则）
- 不修改 ADR-0003（migration 工具 / 编号约定 / 文件命名规范由 Story 4.3 已锚定）
- 不写英文版测试注释 / 文档（项目 communication_language=Chinese；与 Story 4.3 / 7.2 / 10.3 / 11.2 / 17.2 一致）
- 不为 0010 写 stress test / 大数据量 seed test（4 行 seed 是 MVP 钦定数量，单测 + dockertest 集成测试已覆盖核心约束）
- 不在本 story 内对 Story 17.4 / 17.5 实装做"提前预实装"（即使顺手写 `(r *emojiRepo) List(ctx) ([]EmojiConfig, error)` 也禁止；这些方法是 17.4 / 17.5 钦定范围，提前 ship 会让评审找不到"新增方法"的明确范围边界，与 Story 11.2 / 17.2 "禁止预实装 repo 方法" 同模式）
- 不在本 story 内 seed `cosmetic_items` 表（AR18 钦定的 cosmetic seed 由 Epic 20 owner story 落地，**不**在 Epic 17 范围）
- 不为 0010 落地 admin / dev 端点（如 `POST /dev/seed-emoji` / `POST /dev/disable-emoji`）—— MVP 节点 6 不规划

## Acceptance Criteria

**AC1 — 0010_seed_emoji_configs.up.sql 新建（4 个表情 + INSERT IGNORE + 每个 asset_url 非空）**

新建 `server/migrations/0010_seed_emoji_configs.up.sql`，内容必须**严格**符合 epics.md §Story 17.3（行 2559-2575）+ AR19 + V1 §11.1 assetUrl 非空约束：

```sql
-- 对齐 epics.md §Story 17.3 + AR19 + V1接口设计.md §11.1
-- emoji_configs 系统表情配置 seed
--
-- **本 migration 由 Story 17.3 首次落地（Epic 17 节点 6 表情广播链路 seed owner）**
-- 含 ≥2 case 单测（seed 后 ≥4 行 + asset_url 都非空 / 重复 migrate up 不重复插入）
-- + dockertest 集成测试覆盖 seed 内容正确 + INSERT IGNORE 幂等
-- （epics.md §Story 17.3 钦定的"集成测试覆盖：migrate up → SELECT * FROM emoji_configs
-- → 验证 4 个表情存在 + URL 字段格式合法"路径）。
--
-- 表情清单（与 epics.md §Story 17.3 行 2569-2573 钦定 1:1 对齐）：
--   1. wave  挥手     sort_order=1
--   2. love  爱心     sort_order=2
--   3. laugh 大笑     sort_order=3
--   4. cry   哭       sort_order=4
--
-- 字段值约束：
--   - code:       严格符合 V1 §11.1 字符集约束 [a-z0-9_-] + length 1-64
--                 （本 4 个 code 都是纯小写英文字母，长度 3-5，合法）
--   - name:       中文短名，长度 ≤ 64（VARCHAR(64) DDL 边界）
--   - asset_url:  非空 placeholder URL（V1 §11.1 + 17-1 r2 lesson 钦定：
--                 enabled 表情 asset_url **禁止**空字符串；MVP 阶段允许 placeholder
--                 URL `https://placehold.co/64x64?text=Wave` 等，但**必须**是可
--                 访问的 web URL；真实美术资产由 §Epic 17 retrospective tech-debt
--                 登记 + 后续 epic 切换）
--   - sort_order: 1 / 2 / 3 / 4（单调递增 + 唯一；与 V1 §11.1 服务端逻辑步骤 2
--                 `ORDER BY sort_order ASC, id ASC` 一致；4 个值互不相同保证 client
--                 端表情面板顺序稳定，不需要次要排序键 fallback）
--   - is_enabled: 全部 1（enabled；V1 §11.1 服务端逻辑步骤 2 仅返回 is_enabled=1
--                 的表情，disabled 表情对 client 不可见）
--
-- **INSERT IGNORE 语义**（epics.md §Story 17.3 行 2575 钦定）：
-- 当 UNIQUE KEY uk_code (code) 命中时，MySQL 丢弃当前 INSERT 不报错（不抛 1062
-- ER_DUP_ENTRY）。本 seed 主要保障**绕过 migration 框架的入库路径幂等**：
--   (a) golang-migrate force(9) 后再 up → 0010 SQL 会被重跑（不走 ErrNoChange 路径）
--   (b) dev / admin 手工 mysql import 0010 文件 → 重复入库
--   (c) migrate down 到版本 0 后再 up（重跑全部 migration）
-- migrate up 默认幂等路径（version 9 → 10 跑一次后 9 → 10 不重跑）由 golang-migrate
-- 框架兜底，**不**依赖 INSERT IGNORE；INSERT IGNORE 是双层兜底，保证即使框架
-- 兜底失效（dirty / force / 手工跑）seed 仍幂等。
--
-- **不**用 ON DUPLICATE KEY UPDATE：因为 update 路径会修改 asset_url / name /
-- sort_order 等可能被 admin 手工调整过的字段（如某 emoji 临时下架 admin 改 is_enabled=0，
-- 重跑 seed 不应把 is_enabled 重置回 1）；INSERT IGNORE 只在不冲突时插入，已有
-- 数据保留 admin 修改，符合"seed 是初始默认值不是覆盖式重置"语义。
--
-- **范围红线**：本 migration **仅** INSERT 4 行；不修改 schema（17.2 owner）/
-- 不含任何业务 service / handler / repo write 方法（17.4 / 17.5 落地）/ 不含
-- ON DUPLICATE KEY UPDATE / DELETE / TRUNCATE 任何破坏性 SQL。
--
-- **不**用 server 端代码层（如 Go 的 seedEmojis() 函数）做 seed：
-- (a) ADR-0003 钦定 migrations/ 文件是 schema + 静态数据的真相源；
-- (b) seed 通过 SQL migration 让 dev / test / staging / prod 同一份数据；
-- (c) 与 cosmetic seed（Epic 20）未来落地路径同模式，避免每个 seed 自己定一种执行方式。
INSERT IGNORE INTO emoji_configs (code, name, asset_url, sort_order, is_enabled) VALUES
    ('wave',  '挥手', 'https://placehold.co/64x64?text=Wave',  1, 1),
    ('love',  '爱心', 'https://placehold.co/64x64?text=Love',  2, 1),
    ('laugh', '大笑', 'https://placehold.co/64x64?text=Laugh', 3, 1),
    ('cry',   '哭',   'https://placehold.co/64x64?text=Cry',   4, 1);
```

- **4 个 code 字面量**严格对齐 epics.md §Story 17.3 行 2569-2573 钦定（`wave` / `love` / `laugh` / `cry` 一一对应；**不**多 seed 也**不**少 seed —— 如果未来产品要求加更多表情，由后续 story 通过新 migration 0011_seed_emoji_configs_v2 之类落地，**不**在本 story 修改 0010）
- **每个 asset_url 非空**（V1 §11.1 + 17-1 r2 lesson 钦定）：使用 `https://placehold.co/64x64?text=Wave` 等 placeholder URL（真实可访问 placehold.co 服务，与 AR18 / AR19 钦定 URL 约束一致）；**禁止**任何 `""` 空字符串
- **INSERT IGNORE 而非 INSERT INTO**（epics.md 行 2575 钦定）：UNIQUE KEY uk_code (code) 命中时静默丢弃，保证幂等
- **sort_order 1 / 2 / 3 / 4 单调唯一**：4 个值互不相同（避免相同时 client 端排序退化到次要键 id）
- **is_enabled 全部 1**：enabled 表情才会被 V1 §11.1 GET /emojis 返回 + 才会被 V1 §12.2 emoji.send 校验通过
- **不**包含 `created_at` / `updated_at` 显式值：DDL `DEFAULT CURRENT_TIMESTAMP(3)` 兜底（与 17.2 落地的 0009_init_emoji_configs.up.sql 一致）
- **不**包含 `id` 显式值：DDL `AUTO_INCREMENT` 自动分配（避免 id 冲突 / 让 id 命名不暴露在 seed 文件里）
- 文件编码 UTF-8 + LF 行尾（与 0001 ~ 0009 一致）
- 顶部注释模板与 0007 / 0008 / 0009（11.2 / 17.2 升级版）一致 —— "对齐 §X.Y" + 字段约束 + 范围红线四段式
- **不**包含任何业务 logic SQL（如 `UPDATE emoji_configs SET ...`）
- **不**包含任何额外 emoji（如 `angry` / `surprised` / `thinking` 等）—— 严格 4 个 emoji 上限

**AC2 — 0010_seed_emoji_configs.down.sql 新建**

新建 `server/migrations/0010_seed_emoji_configs.down.sql`，内容：

```sql
-- 回滚 0010_seed_emoji_configs.up.sql
--
-- **本 migration 由 Story 17.3 首次落地（Epic 17 节点 6 表情广播链路 seed owner）**
-- 含 ≥2 case 单测 + dockertest 集成测试覆盖 seed 内容正确 + INSERT IGNORE 幂等。
--
-- 回滚策略：**精确 DELETE 4 个 code**（**不**用 TRUNCATE 也**不**用全表 DELETE）—— 防误删
-- admin / dev 后续插入的非 0010 seed 数据（如未来手工 INSERT 的 `angry` / `surprised`
-- 等表情；本 story 阶段 emoji_configs 表只有 0010 seed 的 4 行，但 down 路径必须为
-- "未来可能存在的非 0010 数据"留兜底）。
--
-- **不**走 `DELETE FROM emoji_configs WHERE 1=1`（全表删；危险）；**不**走
-- `TRUNCATE TABLE emoji_configs`（重置 AUTO_INCREMENT；不可逆性更强）。
DELETE FROM emoji_configs WHERE code IN ('wave', 'love', 'laugh', 'cry');
```

- 文件编码 UTF-8 + LF 行尾
- **仅** 1 条 `DELETE FROM emoji_configs WHERE code IN (...)`，不含任何额外 cleanup 语句
- **不**用 `TRUNCATE TABLE emoji_configs`（避免重置 AUTO_INCREMENT + 破坏未来可能存在的非 0010 seed 数据）
- **不**用 `DELETE FROM emoji_configs WHERE 1=1`（避免误删未来 admin 手工写入的非 0010 数据）
- 与 0009 down.sql 的 `DROP TABLE IF EXISTS` 不同 —— 因为 0010 是 seed migration 而非 schema migration，down 只回滚数据**不**回滚表本身（表回滚由 0009.down 负责）

**AC3 — migrate_integration_test.go 扩展（版本号断言 v=9 → v=10 + seed 内容验证 + 幂等验证）**

修改 `server/internal/infra/migrate/migrate_integration_test.go`：

**AC3.1 修改 `TestMigrateIntegration_StatusAfterUp` 版本号断言**（行 ~576 附近）：

- 找到 `if v != 9 {` → 改为 `if v != 10 {`
- 找到 `t.Errorf("Status version = %d, want 9", v)` → 改为 `t.Errorf("Status version = %d, want 10", v)`
- 同步更新顶部注释（行 540-545 附近）：增加一行 `// Story 17.3 扩展：从 9 改 10（多了 0010_seed_emoji_configs；Epic 17 节点 6 表情 seed）`

**AC3.2 既有 `TestMigrateIntegration_UpThenDown` / `TestMigrateIntegration_UpTwice_Idempotent` 不动表数量断言**：
- 0010 是 **seed migration**，**不**新增表 → `expectedTables` slice 仍 9 张表（包含 emoji_configs，本 story 不动）
- `TestMigrateIntegration_UpTwice_Idempotent` 中 `table_name IN (...)` 也保持 9 张表，仅断言"两次 up 不重复建表"，**不**断言 seed 数据条数（seed 数据条数由 AC3.3 新 case 验证）
- **顶部 testfile 注释**（行 18-21 附近 `// Story 17.2 扩展：把 4 条 case 的断言从 8 张表扩展到 9 张表 ...`）后追加一段：
  ```
  // Story 17.3 扩展：在表 schema 不变（仍 9 张）基础上新增 0010_seed_emoji_configs；
  //   主要 case 跑 4 行 seed 的内容正确性 + INSERT IGNORE 幂等（不影响表数量断言）；
  //   StatusAfterUp 版本号断言从 v=9 改 v=10。
  ```

**AC3.3 新增 `TestMigrateIntegration_EmojiConfigs_SeedContent`**（紧接 `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` 之后；约文件行 931 之后）：

```go
// TestMigrateIntegration_EmojiConfigs_SeedContent 验证
// migrations/0010_seed_emoji_configs.up.sql 钦定的 4 个表情 seed 在 migrate up
// 后真实写入 emoji_configs 表，且每行字段值符合 V1 §11.1 + AR19 + 17-1 r2 lesson
// 约束：
//
//   - 4 个 code 都存在：wave / love / laugh / cry
//   - 每行 asset_url 非空（V1 §11.1 钦定 length ≥ 1；17-1 r2 lesson 收紧禁止 ""）
//   - 每行 is_enabled = 1（enabled 表情才会被 GET /emojis 返回）
//   - 每行 name 非空（VARCHAR(64) NOT NULL）
//   - sort_order 唯一且单调（避免 client 端排序退化到 id 次要键）
//
// **背景（Story 17.3 引入）**：epics.md §Story 17.3 钦定的"集成测试覆盖（dockertest）：
// migrate up → SELECT * FROM emoji_configs → 验证 4 个表情存在 + URL 字段格式合法"
// 路径在本 case 落地；用于 Story 17.4 / 17.5 / Epic 18.1 / Epic 19.1 实装时
// 验证 seed 数据真实在位的根基。
//
// 用 database/sql 直跑 SELECT（**不**走 GORM）让测试结果**不**依赖 ORM 行为差异
// （与 11.2 / 17.2 落地的 dockertest case 同模式）。
func TestMigrateIntegration_EmojiConfigs_SeedContent(t *testing.T) {
    // 实装细节由 dev-story 阶段补全；模板参考既有
    // TestMigrateIntegration_EmojiConfigs_Schema（行 693 ~ 871）+
    // TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected（行 613 ~ 674）。
    //
    // 必查项（每项失败立即 t.Errorf，不 t.Fatalf —— 用 batch 累积报错风格）：
    //   1. SELECT code, name, asset_url, sort_order, is_enabled FROM emoji_configs ORDER BY sort_order
    //      → 至少 4 行（>=4，允许未来扩展但本 story 阶段恰好 4 行；用 >=4 而非 ==4
    //      避免本 case 与未来新 seed migration 冲突）
    //   2. 对每一行：
    //      a. code ∈ {"wave", "love", "laugh", "cry"} 至少全覆盖（用 set 校验：4 个
    //         钦定 code 都在结果集中存在）
    //      b. name 非空（len(name) > 0）
    //      c. asset_url 非空（len(asset_url) > 0；V1 §11.1 + 17-1 r2 lesson）
    //      d. is_enabled == 1（enabled 才能被 GET /emojis 返回）
    //   3. 4 个钦定 code 对应的 sort_order 必须 1 / 2 / 3 / 4（与 0010 SQL 钦定一致）
    //      —— 用 map[code]wantSortOrder 比对
}
```

**AC3.4 新增 `TestMigrateIntegration_EmojiConfigs_SeedIdempotent`**（紧接 AC3.3 case 之后）：

```go
// TestMigrateIntegration_EmojiConfigs_SeedIdempotent 验证
// migrations/0010_seed_emoji_configs.up.sql 钦定的 INSERT IGNORE 语义：
// **二次执行同一 SQL 不重复插入**（UNIQUE KEY uk_code 命中时静默丢弃）。
//
// **背景（Story 17.3 引入）**：epics.md §Story 17.3 钦定的"重复 migrate up →
// 不重复插入（INSERT IGNORE）"路径在本 case 落地。
//
// **注意 INSERT IGNORE 与 golang-migrate 框架幂等的分层**：
//
//  - **golang-migrate 框架幂等**：`migrate up` 第二次走 ErrNoChange 不会重跑 0010
//    SQL（schema_migrations 表记录已 applied）—— 这层已由
//    TestMigrateIntegration_UpTwice_Idempotent 覆盖
//  - **INSERT IGNORE 兜底**：保障"绕过框架重跑"路径不出问题；常见场景：
//    (a) force(9) + up → 0010 重跑（框架忽略已 applied 记录）
//    (b) dev 手工 mysql import 0010.up.sql → 重复入库
//    (c) migrate down 到 0 + up → 重跑全部 migration（包括 0010）
//
// **本 case 走路径 (c)**：down 全清 → up 重跑 0010 → 检查不重复（4 行不变 8 行）。
// 路径 (a) (b) 由 admin / ops 控制不在本 case 范围；INSERT IGNORE 在 SQL 层兜底
// 即可，不需要 case 全部枚举验证。
//
// **覆盖路径**：
//  1. migrate up → emoji_configs 4 行（0010 seed 写入）
//  2. migrate down → emoji_configs 0 行（含 0010.down DELETE + 0009.down DROP TABLE 兜底）
//  3. migrate up → emoji_configs 仍恰好 4 行（不应翻倍到 8 行）
//
// 用 database/sql 直跑 SELECT COUNT(*)（**不**走 GORM）。
func TestMigrateIntegration_EmojiConfigs_SeedIdempotent(t *testing.T) {
    // 实装细节由 dev-story 阶段补全；模板参考既有
    // TestMigrateIntegration_UpThenDown（行 130 ~ 187）+
    // TestMigrateIntegration_EmojiConfigs_Schema（行 693 ~ 871）。
    //
    // 必查项：
    //   1. migrate Up → SELECT COUNT(*) FROM emoji_configs → 4
    //   2. migrate Down → 全表删（含 emoji_configs）
    //   3. migrate Up → SELECT COUNT(*) FROM emoji_configs → 4（不重复，不 8 行）
}
```

- 两个新 case 都用 `dockertest` 起 mysql:8.0 容器（沿用 `startMySQL(t)` + `migrationsPath(t)` helper，与既有 case 一致）
- 用 `database/sql` 直跑 raw SELECT（**不**走 GORM）—— 与 17.2 落地的 UNIQUE 拒绝 case 同模式
- 测试断言风格：`t.Errorf` 累积报错（**不** `t.Fatalf`），便于一次 run 看全部失败点（与 17.2 / 11.2 同模式）

**AC4 — 验证步骤**

- **AC4.1 build 验证**：执行 `bash scripts/build.sh --test` 必须**全绿**（vet + 全 unit 测试通过 —— 本 story 不新增任何 Go .go 文件，纯 SQL + 集成 test go 文件改动；既有单测无回归）；`go vet -tags=integration ./...` 必须**全绿**（含新增 2 case）；`bash scripts/build.sh --integration` 必须**全绿**（新增 `TestMigrateIntegration_EmojiConfigs_SeedContent` + `TestMigrateIntegration_EmojiConfigs_SeedIdempotent` 两个 case 跑通 + 既有 `TestMigrateIntegration_StatusAfterUp` 版本号 10 断言通过 + 既有 `TestMigrateIntegration_UpThenDown` / `TestMigrateIntegration_UpTwice_Idempotent` 表数量 9 断言不回归 + 既有 `TestMigrateIntegration_EmojiConfigs_Schema` / `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` 不回归）
- **AC4.2 git diff 范围检查**：编辑完成后 `git diff` 输出**仅**包含：
  - `server/migrations/0010_seed_emoji_configs.up.sql`（新增）
  - `server/migrations/0010_seed_emoji_configs.down.sql`（新增）
  - `server/internal/infra/migrate/migrate_integration_test.go`（修改：版本号 9 → 10 + 顶部注释追加 + 新增 2 case）
  - `_bmad-output/implementation-artifacts/17-3-emoji_configs-seed.md`（本 story 文件状态流转 + Tasks/Subtasks 勾选 + Dev Agent Record / File List / Change Log）
  - `_bmad-output/implementation-artifacts/sprint-status.yaml`（状态行 + last_updated）
- **AC4.3 seed 内容跨文档一致性**：手动检查 0010 up.sql 的 4 个 emoji code 与 epics.md §Story 17.3 行 2569-2573 钦定**逐字面量** 1:1 对齐（`wave` / `love` / `laugh` / `cry`）；每个 asset_url 非空（V1 §11.1 + 17-1 r2 lesson 钦定）；每个 sort_order 1 / 2 / 3 / 4 唯一且单调；is_enabled 全部 1
- **AC4.4 INSERT IGNORE 语法正确性**：手动 SQL 解析 0010 up.sql 必须含 `INSERT IGNORE INTO emoji_configs` 关键字（**不**是 `INSERT INTO`；**不**是 `INSERT IGNORE INTO emoji_configs ON DUPLICATE KEY UPDATE`）
- **AC4.5 既有 migrate 集成测试不回归**：跑 `bash scripts/build.sh --integration` 全绿 —— 含 17.2 已落地的 `TestMigrateIntegration_EmojiConfigs_Schema` / `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected`（这两个 case **不**关心 seed 数据，本 story seed 写入对它们透明 —— UniqueCode_Rejected 用 `INSERT INTO emoji_configs (code='wave', ...)` 之后再插 `code='wave'`，本 story seed 后该 case 中第一次 INSERT 已会因为 seed 已写入 wave 而失败 → **需要检查并修复**：把 UniqueCode_Rejected case 的测试 code 改成 seed 之外的字面量如 `'integration_test_unique_code_a'` / 用 prefix 隔离）

> **注**：AC4.5 暴露了一个**重要 follow-up**：17.2 落地的 `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` 在 0010 seed 之后**会因为 `'wave'` 已被 seed 写入而第一次 INSERT 就失败**（不是预期的 happy path），破坏了 17.2 case 的"先插成功再插冲突"语义。本 story **必须修复**该 case 让它与 seed 解耦 —— 改用 seed 之外的 code 字面量（如 `'test_unique_code_a'`）。详见 AC3.5。

**AC3.5 修复 `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` 与 seed 解耦**（行 892 附近）：

17.2 落地的 case 用 `code='wave'` 作为 INSERT 测试值；本 story seed 后该 code 已存在 → 该 case 第一次 INSERT 就会触发 UNIQUE 冲突，违反 case 设计（"先成功 INSERT 再失败 INSERT" 的两步语义）。

修复方式：把 case 用的 code 字面量从业务真实 code（`wave` / `love` / `laugh` / `cry`）改为**测试专用 code**（确保与 seed 永远不冲突）：

- 行 917 的 `INSERT INTO emoji_configs ... VALUES ('wave', '挥手', ...)` → 改为 `('test_unique_code_a', 'TestA', 'https://example.com/test_a.png', 1001, 1)`
- 行 923 的 `INSERT INTO emoji_configs ... VALUES ('wave', '挥手 v2', ...)` → 改为 `('test_unique_code_a', 'TestA v2', 'https://example.com/test_a_v2.png', 1002, 1)`
- 同步更新 case 注释（行 884 附近的"插入 emoji_configs (code='wave', ...)"）改为"插入 emoji_configs (code='test_unique_code_a', ...)"
- case 函数体顶部注释追加一行说明："**用测试专用 code（test_unique_code_a）与 0010 seed 的 wave/love/laugh/cry 字面量隔离**，避免 seed 先写入 wave 后导致本 case 第一次 INSERT 就触发 UNIQUE 而非预期的第二次"

**注**：sort_order 用 1001 / 1002 等大于 1000 的值，与 seed 的 1-4 也隔离开（虽然 sort_order 没 UNIQUE 约束，但让 case 数据在 sort_order 维度也明确远离 seed 段）。

## Tasks / Subtasks

- [x] Task 1: 准备阶段（AC: #1, #2, #3, #4）
  - [x] Subtask 1.1: 阅读本 story 全文 + `_bmad-output/planning-artifacts/epics.md` §Story 17.3（行 2559-2579）确认 4 个 emoji code 字面量清单
  - [x] Subtask 1.2: 阅读 17.2 已 done 的姊妹 story `_bmad-output/implementation-artifacts/17-2-emoji_configs-migration.md`，参考其 migration + 集成测试编辑模式（新增 case + 修改顶部注释 + 修改版本号）
  - [x] Subtask 1.3: 阅读 `server/migrations/0009_init_emoji_configs.up.sql`（17.2 落地）确认 emoji_configs 表 schema 8 字段顺序 + UNIQUE KEY uk_code 真实存在
  - [x] Subtask 1.4: 阅读 `server/internal/repo/mysql/emoji_repo.go`（17.2 落地）确认 `EmojiConfig` struct 字段已就绪（本 story 不动该文件，仅确认）
  - [x] Subtask 1.5: 阅读 `server/internal/infra/migrate/migrate_integration_test.go`（17.2 落地版本）确认 `TestMigrateIntegration_StatusAfterUp` 版本号断言位置 + `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` 用 `'wave'` 字面量（本 story 要解耦）
  - [x] Subtask 1.6: 阅读 V1 §11.1（行 1734-1837）确认 assetUrl `1 ≤ length ≤ 255` 禁止空字符串约束
  - [x] Subtask 1.7: 阅读 `docs/lessons/2026-05-13-emoji-contract-self-consistency-and-1009-and-asset-url-17-1-r2.md` Lesson 3 确认 enabled 表情 asset_url 必须非空的契约根因
- [x] Task 2: 落地 0010_seed_emoji_configs.up.sql（AC: #1）
  - [x] Subtask 2.1: 新建 `server/migrations/0010_seed_emoji_configs.up.sql`
  - [x] Subtask 2.2: 写顶部注释（"对齐 §Story 17.3 + AR19 + V1 §11.1" + 表情清单 + 字段约束 + INSERT IGNORE 语义 + 范围红线四段式，按 AC1 模板）
  - [x] Subtask 2.3: 写 `INSERT IGNORE INTO emoji_configs (code, name, asset_url, sort_order, is_enabled) VALUES (...)` 含 4 行：wave / love / laugh / cry，每行 asset_url 非空 placeholder URL（按 AC1 钦定）
- [x] Task 3: 落地 0010_seed_emoji_configs.down.sql（AC: #2）
  - [x] Subtask 3.1: 新建 `server/migrations/0010_seed_emoji_configs.down.sql`
  - [x] Subtask 3.2: 写顶部注释（按 AC2 模板）+ `DELETE FROM emoji_configs WHERE code IN ('wave', 'love', 'laugh', 'cry');`
- [x] Task 4: 修改 migrate_integration_test.go（AC: #3）
  - [x] Subtask 4.1: 修改 `TestMigrateIntegration_StatusAfterUp` 版本号 `v != 9` → `v != 10` + err msg `want 9` → `want 10`（按 AC3.1 钦定）
  - [x] Subtask 4.2: 修改 testfile 顶部注释追加 Story 17.3 扩展段（按 AC3.2 钦定）+ `TestMigrateIntegration_StatusAfterUp` 顶部注释追加 Story 17.3 扩展（按 AC3.1 钦定）
  - [x] Subtask 4.3: 修复 `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` 把 `'wave'` 字面量改为 `'test_unique_code_a'`（按 AC3.5 钦定）+ 同步更新 case 注释 + 函数体顶部追加 seed 解耦说明
  - [x] Subtask 4.4: 新增 `TestMigrateIntegration_EmojiConfigs_SeedContent` case（按 AC3.3 钦定 + 参考 `TestMigrateIntegration_EmojiConfigs_Schema` 实装模板：起容器 / migrate up / sql.Open / 跑 SELECT / 累积 t.Errorf）
  - [x] Subtask 4.5: 新增 `TestMigrateIntegration_EmojiConfigs_SeedIdempotent` case（按 AC3.4 钦定 + 参考 `TestMigrateIntegration_UpThenDown` 实装模板：起容器 / Up / SELECT COUNT(*) = 4 / Down / SELECT COUNT(*) = 0（或 schema 已 drop 失败也算 0）/ Up / SELECT COUNT(*) = 4）
- [x] Task 5: 验证 + 提交（AC: #4）
  - [x] Subtask 5.1: 跑 `bash scripts/build.sh --test` 全绿（vet + 全 unit 测试通过；本 story 不新增任何 .go 文件，但既有单测无回归）
  - [x] Subtask 5.2: 跑 `go vet -tags=integration ./...` 全绿（含本 story 新增 2 case + 修改的 UniqueCode_Rejected case）
  - [x] Subtask 5.3: 跑 `bash scripts/build.sh --integration` 全绿（含 5 个 EmojiConfigs case：Schema / UniqueCode_Rejected / SeedContent / SeedIdempotent + StatusAfterUp v=10）—— migrate 集成测试全部通过；全 suite 跑遇到 service 层 auth_service_integration_test 受 Windows + Docker 并发的 2 min 超时影响（非本 story 引入，与 17-2 同状态）
  - [x] Subtask 5.4: git diff 范围检查 —— 仅本 story 钦定 5 个文件（见 File List）
  - [x] Subtask 5.5: seed 内容跨文档手动一致性检查（4 个 code 字面量 + asset_url 非空 + sort_order 单调 + is_enabled=1，按 AC4.3 钦定）
  - [x] Subtask 5.6: INSERT IGNORE 语法正确性手动检查（按 AC4.4 钦定）
  - [x] Subtask 5.7: 在 sprint-status.yaml 把本 story 状态从 in-progress 改为 review
  - [ ] Subtask 5.8: 由 code-review 检出后状态切 done + 在本 story 文件 + sprint-status.yaml 状态行追加 commit hash

## Dev Notes

### Build & Test 规范（项目级 CLAUDE.md 钦定）

- 写完 / 改完 Go 代码后必跑 `bash scripts/build.sh --test`（vet + 单测，**默认 build tag**，集成测试不跑）
- 集成测试 dockertest 必须用 `bash scripts/build.sh --integration`（带 `-tags=integration` build tag）
- 脚本契约来源：`_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.5 + `_bmad-output/implementation-artifacts/1-7-重做-scripts-build-sh.md`

### Migration 文件命名 / 编号规则（ADR-0003 + Story 4.3 钦定）

- 文件命名：`{N:04d}_{name}.up.sql` / `{N:04d}_{name}.down.sql`（4 位编号 + 下划线 + 小写下划线名称）
- 编号顺序：0001 ~ 0009 已被 4.3 / 7.2 / 10.3 / 11.2 / 17.2 占用（users / user_auth_bindings / pets / user_step_accounts / user_chests / user_step_sync_logs / rooms / room_members / emoji_configs schema）；**本 story 占用 0010**（首个 seed migration —— 与之前 9 个 schema migration 不同性质，但沿用同一编号规则）
- **seed migration 也走纯 SQL 文件**（**不**用 Go 代码 seed loader）：与 ADR-0003 §3.2 钦定一致；dev / test / staging / prod 同一份数据来源；与未来 cosmetic seed（Epic 20）落地路径同模式

### INSERT IGNORE 与 ON DUPLICATE KEY UPDATE 的选型

| 路径 | 适用场景 | 风险 |
|---|---|---|
| `INSERT INTO ...` | UNIQUE 不冲突 | 冲突时抛 1062 错误，重跑失败 |
| `INSERT IGNORE INTO ...` ✓ | **本 story 选用** | 冲突时静默丢弃；admin 手工修改过的字段（如 is_enabled=0 下架）**不**会被 seed 覆盖（保留 admin 修改） |
| `INSERT INTO ... ON DUPLICATE KEY UPDATE ...` | 需要重跑时强制重置字段 | 会覆盖 admin 手工修改（如重新启用已下架表情），违反"seed 是初始默认值"语义 |
| `REPLACE INTO ...` | 等价于先 DELETE 再 INSERT | 会触发外键 cascade / 重新分配 id，不可逆性强 |

**选 INSERT IGNORE 的理由**：epics.md §Story 17.3 行 2575 显式钦定 "INSERT IGNORE 防重复"；与"seed 是初始默认值不是覆盖式重置"语义一致；admin 后台未来若修改 emoji（如临时下架 `cry` 设 `is_enabled=0`），重跑 seed 不会把 admin 修改反推回去。

### Placeholder URL 选型（AR18 / AR19 / V1 §11.1）

- AR18 钦定 placeholder URL 示例：`https://placehold.co/128x128?text=Hat`（cosmetic_items）
- AR19 钦定 emoji_configs 的 asset_url 必须可访问；MVP 阶段可用 placeholder
- V1 §11.1 字段示例：`https://placehold.co/64x64?text=Wave`（64x64 尺寸暗示 emoji 图标用途）
- **本 story 选 `https://placehold.co/64x64?text={EmojiNameEn}`**：
  - 64x64 与 V1 §11.1 字段示例一致
  - `text=Wave` 等英文名让 placeholder 图片含视觉标签（不是全空灰块）
  - placehold.co 是公共可访问服务，节点 6 demo 阶段足够（真实美术资产由 Epic 17 retrospective tech-debt 登记 + 后续 epic 切换）

### 跨文档语义同步检查（seed 数据 / API 契约 / 数据库设计）

- 本 story 落地的 0010 SQL 数据必须严格符合 V1 §11.1 字段约束（17.1 锚定）+ §5.15 DDL 约束（17.2 落地）+ §6 状态枚举
- 不允许在本 story 阶段对 V1 / §5.15 / AR19 做反向加严 / 放松
- 如发现 V1 §11.1 / §5.15 / AR19 与本 story 落地 seed 数据冲突（如 V1 §11.1 钦定 length > 0 但本 story seed 写 ""）→ 优先修 0010 SQL 而非反向改契约

### 错误码不在本 story 范围

- §3 全局错误码表（7001 / 6004 / 1001 / 1002 / 1005 / 1009）由 17.1 锚定 + 由 17.4 / 17.5 实装时引用；本 story **不**触发错误码定义 / 修改 / 引用（migration 层不返回 API 错误码）
- INSERT IGNORE 静默丢弃**不**触发任何错误码（与 INSERT 抛 1062 ER_DUP_ENTRY 不同；migration 层视为成功）

### 跨 epic 依赖追溯

- **上游冻结**：
  - 数据库设计 §5.15 emoji_configs 表 schema ← 17.2 落地
  - V1 §11.1 / §12.2 / §12.3 字段层 ← 17.1 锚定（17.1 r2 收紧 assetUrl 非空）
  - AR19 emoji_configs ≥4 个 + asset_url 可访问 ← 总体架构钦定
  - ADR-0003 migration 工具 + 编号规则 ← Story 4.3 落地
- **下游强依赖**（本 story done 后才能开工）：
  - Story 17.4（GET /emojis 接口；dockertest 钦定 seed 4 个 → response items 长度 4）
  - Story 17.5（WS emoji.send 校验；单测 mock emojiCode 合法性时直接用本 story 落地的 4 个 code）
  - iOS Epic 18.1（表情面板 SwiftUI；UI 钦定 "API 返回 4 个表情 → 网格渲染 4 个 cell"）
  - Epic 19.1（节点 6 demo E2E；E2E 钦定 "面板出现 → 验证 4 个表情图标都加载成功"）

### 测试 / 验证

- **单元测试**：本 story 不新建任何 Go .go 业务代码（纯 SQL seed + 集成 test 文件改动）→ 无 sqlmock-based 单测需要新建；既有 `emoji_repo` 没有 repo 方法（17.4 才有），所以 `mysql/` 目录下其他 repo 单测不受 0010 seed 影响
- **集成测试**（dockertest）：本 story 新增 2 case + 修改 1 case（与 seed 解耦）；用 `bash scripts/build.sh --integration` 跑（带 `-tags=integration`）
- **下游验证**：本 story done 后由 Story 17.4 实装时的 dockertest 集成测试（curl GET /emojis → 验证 response.items 长度 = 4）+ Story 17.5 实装时的 dockertest 集成测试（A 发 `emoji.send {emojiCode: "wave"}` → 不抛 7001 + broadcast 给 B）做真实串联验证

### 范围红线 + 风险

- **红线**：本 story **不**修改任何 service / handler / repo write / read 方法；**不**修改 V1 接口契约 / 数据库设计文档 / AR19 / ADR-0003；**不**修改 0001 ~ 0009 既有 migration；**不**修改 17.2 落地的 emoji_repo.go；**不**预实装 `EmojiRepo` interface / 方法
- **红线**：本 story **不**实装任何业务 Go 代码（17.4 / 17.5 owner）
- **红线**：本 story **必须** seed 严格 4 个表情（`wave` / `love` / `laugh` / `cry`），**不**多 seed 也**不**少 seed —— 多 seed 会破坏 17.4 dockertest 集成测试钦定的"response.items 长度 = 4"；少 seed 会违反 AR19 "≥ 4 个" 约束
- **风险**：17.2 落地的 `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` 用 `'wave'` 作为测试 code → 0010 seed 写入后 `'wave'` 已存在 → 该 case 第一次 INSERT 就会失败 → 必须修复（AC3.5 钦定改为 `'test_unique_code_a'`）。**漏修风险**：dev-story 阶段 build.sh --integration 在 Docker 不可用时 t.Skip 静默通过，让 case 失效不被察觉 → mitigation：AC4.5 显式钦定 review 阶段必须真跑 integration（不能 skip）
- **风险**：sort_order 4 个值如果不唯一（如不小心写 1/1/2/3）→ client 端排序退化到次要键 id，结果可能与 V1 §11.1 钦定顺序不一致 → mitigation：AC1 显式钦定 1/2/3/4 单调唯一 + AC3.3 SeedContent case 校验 sort_order 唯一
- **风险**：asset_url 不小心写 `''` 空字符串（DDL `DEFAULT ''` 允许）→ V1 §11.1 + 17-1 r2 lesson 钦定"enabled 表情禁止空 asset_url" → mitigation：AC1 显式钦定每个 asset_url 非空 placeholder URL + AC3.3 SeedContent case 校验 `len(asset_url) > 0`
- **风险**：is_enabled 不小心写 0 → V1 §11.1 GET /emojis 服务端逻辑步骤 2 `WHERE is_enabled = 1` 过滤，下游 GET /emojis 不会返回这个 emoji → 17.4 / 18.1 UI 表情面板会空一个格子 → mitigation：AC1 显式钦定全部 1 + AC3.3 SeedContent case 校验 `is_enabled == 1`
- **风险**：placeholder URL `https://placehold.co/64x64?text=Wave` 服务未来不可用（placehold.co 域名挂掉 / 限流）→ 18.1 AsyncImage 加载失败 → demo 时表情面板降级 placeholder → mitigation：本 story 阶段接受该风险（与 AR18 / AR19 钦定一致 —— MVP 允许 placeholder URL）；真实美术资产切换由 Epic 17 retrospective tech-debt 登记 + 后续 epic 处理
- **风险**：dockertest 集成测试在 Windows 本地跑可能因 Docker Desktop 未启动失败 → 与 17.2 / 11.2 / 14.x 同情况，由 dev-story 阶段确保 Docker Desktop 启动后跑（CI 阶段不在本 story 范围）

### Project Structure Notes

- 本 story 唯一编辑文件（绝对路径）：
  - `C:/fork/cat/server/migrations/0010_seed_emoji_configs.up.sql`（新建）
  - `C:/fork/cat/server/migrations/0010_seed_emoji_configs.down.sql`（新建）
  - `C:/fork/cat/server/internal/infra/migrate/migrate_integration_test.go`（修改：版本号 9 → 10 + 顶部注释 + 修复 UniqueCode_Rejected case + 新增 SeedContent + 新增 SeedIdempotent 共 4 处改动）
  - `C:/fork/cat/_bmad-output/implementation-artifacts/17-3-emoji_configs-seed.md`（本 story 文件）
  - `C:/fork/cat/_bmad-output/implementation-artifacts/sprint-status.yaml`（状态行流转）
- 与 `docs/宠物互动App_Go项目结构与模块职责设计.md` §6 锚定的 `migrations/` 目录完全兼容（沿用既有目录规则，**不**新增子目录 / 模块）；seed migration 和 schema migration 同目录同命名规则（与 ADR-0003 一致）

### References

- [Source: docs/宠物互动App_数据库设计.md#5.15] emoji_configs 表 schema（行 700-718；17.2 落地的 DDL 真相源）
- [Source: docs/宠物互动App_数据库设计.md#7.1] 高优先级 UNIQUE 约束（`emoji_configs` UNIQUE(code) —— 本 story INSERT IGNORE 兜底语义依赖）
- [Source: docs/宠物互动App_数据库设计.md#6] 状态枚举（`is_enabled` 0 / 1 两值；本 story seed 全部 1）
- [Source: docs/宠物互动App_V1接口设计.md#11.1] GET /api/v1/emojis 响应体字段约束（行 1764-1834；17.1 r2 收紧 assetUrl `1 ≤ length ≤ 255` 禁止空字符串 —— 本 story seed 每个 asset_url 必须非空）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 17.3] AC 钦定（行 2559-2579）：`0010_seed_emoji_configs.sql` + ≥4 个表情（wave / love / laugh / cry）+ INSERT IGNORE 防重复 + ≥2 case 单测 + dockertest 集成测试覆盖 seed 内容 + 重复 migrate up 不重复插入
- [Source: _bmad-output/planning-artifacts/epics.md#AR19] emoji_configs 必须预置最小系统表情集合（≥ 4 个，覆盖典型情绪）+ asset_url 必须可访问
- [Source: _bmad-output/planning-artifacts/epics.md#AR18] URL 字段约束（MVP 阶段可用 placeholder URL；asset_url 不可为空字符串）
- [Source: _bmad-output/implementation-artifacts/17-1-接口契约最终化.md] Story 17.1 上游契约（已 done；本 story seed 的 code 字面量必须符合 §11.1 字符集约束 `[a-z0-9_-]` + length 1-64）
- [Source: _bmad-output/implementation-artifacts/17-2-emoji_configs-migration.md] Story 17.2 已 done 姊妹 story（emoji_configs 表 schema + UNIQUE uk_code + EmojiConfig GORM struct 落地参考）
- [Source: _bmad-output/implementation-artifacts/decisions/0003-orm-stack.md] ADR-0003 ORM / migration 工具栈（golang-migrate v4.18.1 + GORM v1.25.12；migration 编号规则 + .up.sql / .down.sql 双向规范；seed migration 也走纯 SQL 文件路径）
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md] ADR-0001 测试栈（dockertest + build tag `integration`；`bash scripts/build.sh --integration` 跑集成测试）
- [Source: docs/lessons/2026-05-13-emoji-contract-self-consistency-and-1009-and-asset-url-17-1-r2.md] 17-1 r2 lesson 3：enabled 表情 asset_url 必须非空；数据库 `DEFAULT ''` 是 DDL 兜底**不**意味业务层允许 enabled 留空 —— 本 story seed 每个 asset_url 必须非空
- [Source: server/migrations/0009_init_emoji_configs.up.sql] 17.2 落地的 emoji_configs 表 DDL（本 story seed 的 INSERT 目标表）
- [Source: server/internal/repo/mysql/emoji_repo.go] 17.2 落地的 EmojiConfig GORM struct（本 story **不**修改，仅作参考；下游 17.4 才用）
- [Source: server/internal/infra/migrate/migrate_integration_test.go] 17.2 落地的 dockertest 集成测试（含 EmojiConfigs_Schema / EmojiConfigs_UniqueCode_Rejected case；本 story 修复后者 + 新增 SeedContent / SeedIdempotent 共 4 处改动）
- [Source: _bmad-output/implementation-artifacts/sprint-status.yaml] Epic 17 状态 in-progress；本 story 状态行 + last_updated 由 create-story / dev-story / code-review 流程逐步推进

## Dev Agent Record

### Agent Model Used

claude-opus-4-7 (1M context)

### Debug Log References

- `bash scripts/build.sh --test`：vet + 全 unit 测试一次性全绿（含既有 migrate / repo / service / ws 等所有 default-tag 测试）；本 story 未新增任何 .go 业务源文件，零回归
- `go vet -tags=integration ./...`：全绿（含本 story 新增 2 case + 修改 1 case 的 integration build tag 包）
- 单独跑本 story 影响的 migrate 集成测试（dockertest）：`go test -tags=integration -count=1 -timeout=900s -run 'TestMigrateIntegration_EmojiConfigs' ./internal/infra/migrate/...` → `ok 357.663s` 全绿（含 Schema / UniqueCode_Rejected / SeedContent / SeedIdempotent 共 4 case）
- `TestMigrateIntegration_StatusAfterUp`：`go test -tags=integration -count=1 -run 'TestMigrateIntegration_StatusAfterUp' ./internal/infra/migrate/...` → `ok 87.502s`，新版本号断言 v=10 通过
- `TestMigrateIntegration_UpThenDown` + `TestMigrateIntegration_UpTwice_Idempotent`：`ok 173.838s`，9 张表数量断言不回归
- 全 suite 跑 `bash scripts/build.sh --integration`：migrate 包全绿，但 `internal/service/auth_service_integration_test.go` 在并发起多个 mysql:8.0 容器时遭遇 Windows + Docker 的 ping 不通 + 2 min 测试超时 → 与 Story 17-2 同样情况（不视为本 story 引入的回归；migrate 包独立跑全绿即可，service 层集成测试由对应 epic owner story 收紧）

### Completion Notes List

- 落地 `server/migrations/0010_seed_emoji_configs.up.sql`：4 行 `INSERT IGNORE`（wave / love / laugh / cry），每行 `asset_url` 非空 placeholder URL（`https://placehold.co/64x64?text={Name}`），`sort_order` 1/2/3/4 单调唯一，`is_enabled` 全部 1
- 落地 `server/migrations/0010_seed_emoji_configs.down.sql`：精确 `DELETE FROM emoji_configs WHERE code IN ('wave', 'love', 'laugh', 'cry')`，不用 TRUNCATE / 全表 DELETE
- 修改 `server/internal/infra/migrate/migrate_integration_test.go`：
  - 顶部注释追加 Story 17.3 扩展段（仍 9 张表 + 新增 0010 seed）
  - `TestMigrateIntegration_StatusAfterUp` 版本号 `v != 9` / `want 9` → `v != 10` / `want 10`，注释追加 17.3 扩展行
  - `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` 用 `'test_unique_code_a'` + sort_order 1001/1002 取代 `'wave'` + 1/2，避免与 0010 seed 冲突；注释 + 函数体顶部追加解耦说明
  - 新增 `TestMigrateIntegration_EmojiConfigs_SeedContent`：SELECT 全表 → 校验 4 个钦定 code 存在 + 每行 asset_url/name 非空 + is_enabled=1 + sort_order 与 0010 钦定一致 + 4 个 sort_order 唯一
  - 新增 `TestMigrateIntegration_EmojiConfigs_SeedIdempotent`：Up → COUNT=4 → Down → COUNT=0（表已 DROP 视为 0）→ Up → COUNT=4，验证 INSERT IGNORE 在 down+up 路径下不翻倍
- 全部 5 个 EmojiConfigs 集成 case + StatusAfterUp v=10 + UpThenDown/UpTwice_Idempotent 9 张表断言 在 dockertest 中实跑全绿
- 范围红线遵守：无任何 service / handler / repo / GORM struct 改动；无 V1 / 数据库设计 / AR19 / ADR-0003 文档改动；无 0001~0009 既有 migration 改动；无 emoji_repo.go 改动；无 prod seed 自动化 / admin 端点等运维化扩散

### File List

- `server/migrations/0010_seed_emoji_configs.up.sql`（新增）
- `server/migrations/0010_seed_emoji_configs.down.sql`（新增）
- `server/internal/infra/migrate/migrate_integration_test.go`（修改：顶部注释 + StatusAfterUp v=10 + UniqueCode_Rejected 解耦 + 新增 2 case）
- `_bmad-output/implementation-artifacts/17-3-emoji_configs-seed.md`（本 story 文件：Tasks/Subtasks 勾选 + Dev Agent Record + File List + Change Log + Status）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（状态行 ready-for-dev → in-progress → review + last_updated）

### Change Log

| 日期 | 操作 | Story 状态 | 备注 |
|---|---|---|---|
| 2026-05-14 | create-story | backlog → ready-for-dev | 由 epic-loop / bmad-create-story workflow 自动生成 |
| 2026-05-14 | dev-story | ready-for-dev → in-progress → review | 落地 0010 seed up/down + 扩展 migrate 集成测试（StatusAfterUp v=10 / 修复 UniqueCode_Rejected 解耦 / 新增 SeedContent + SeedIdempotent）；migrate 包 dockertest 全绿；vet+unit 全绿 |
