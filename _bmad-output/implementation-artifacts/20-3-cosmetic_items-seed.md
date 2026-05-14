# Story 20.3: cosmetic_items seed（首次落地 0012_seed_cosmetic_items.up/down.sql + ≥15 行 INSERT ... ON DUPLICATE KEY UPDATE 强制覆盖 + AR18 数量约束 common≥8/rare≥4/epic≥2/legendary≥1 + common 至少覆盖 4 个不同 slot + 每行 icon_url / asset_url 非空 placeholder URL + drop_weight 按品质递减 + ≥4 case 单测 + dockertest 集成测试覆盖 seed 数量 / 槽位 / URL / 强制覆盖语义 / down narrow DELETE）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want **首次落地** `server/migrations/0012_seed_cosmetic_items.up.sql` + `server/migrations/0012_seed_cosmetic_items.down.sql` 两个新 seed migration 文件（向 20.2 已落地的 `cosmetic_items` 表写入**至少 15 件**装扮配置，严格满足 AR18 数量约束：`common ≥ 8 件，至少覆盖 4 个不同 slot` + `rare ≥ 4 件` + `epic ≥ 2 件` + `legendary ≥ 1 件`；每件 `asset_url` / `icon_url` 必须非空可访问 placeholder URL（如 `https://placehold.co/128x128?text=Hat-Yellow` / `https://placehold.co/64x64?text=Hat-Yellow`）；`drop_weight` 按品质递减分布（common=100 / rare=20 / epic=4 / legendary=1）；`is_enabled` 全部 1；`slot` 严格符合 §6.8 枚举值 ∈ {1,2,3,4,5,6,7,99}；`rarity` 严格符合 §6.9 枚举值 ∈ {1,2,3,4}；用 `INSERT ... ON DUPLICATE KEY UPDATE` **强制覆盖**这 15+ 个 code 的 `name / slot / rarity / asset_url / icon_url / drop_weight / is_enabled` 7 字段，确保即使 admin / dev 历史残留让 cosmetic_items 表里预先存在这 15+ 个 code 的"坏行"也会被覆盖回 0012 钦定值 —— 与 17.3 r3 最终决断 lesson `docs/lessons/2026-05-14-0010-final-decision-up-force-overwrite-down-narrow-delete.md` 同模式，**显式拒绝** INSERT IGNORE 的"幸存 admin 坏行"路径）+ **扩展** `server/internal/infra/migrate/migrate_integration_test.go` 的 `TestMigrateIntegration_StatusAfterUp` 版本号断言（`v != 11` → `v != 12`）+ **新增** `TestMigrateIntegration_CosmeticItems_SeedContent` dockertest 集成测试（覆盖 epics.md §Story 20.3 钦定的"集成测试覆盖：migrate up → SELECT count(*) GROUP BY rarity → 验证各品质数量 ≥ AR18 约束"路径，校验 ≥15 行 + 各 rarity 数量符合 AR18 + common 至少覆盖 4 个不同 slot + 每行 asset_url / icon_url 非空 + 每行 is_enabled=1 + 每行 slot ∈ {1,2,3,4,5,6,7,99} + 每行 rarity ∈ {1,2,3,4} + drop_weight 按品质递减分布合理）+ **新增** `TestMigrateIntegration_CosmeticItems_SeedForceOverwrite` dockertest 集成测试（覆盖"预填 admin-flavored 行 → 回滚 schema_migrations 版本号 → 重跑 0012.up → 验证 ON DUPLICATE KEY UPDATE 强制覆盖 7 字段回钦定值"路径，与 17.3 落地的 `TestMigrateIntegration_EmojiConfigs_SeedIdempotent` 同模式；同时验证不翻倍到 30 行），
so that **Story 20.4（chest_open_logs migration）+ Story 20.5（GET /chest/current 接口）+ Story 20.6（POST /chest/open 事务 + 加权抽取）+ Story 20.7（dev /dev/force-unlock-chest）+ Story 20.8（dev /dev/grant-cosmetic-batch）+ Story 20.9（Layer 2 集成测试 - 开箱事务全流程）+ iOS Epic 21 各 story（首页宝箱组件 + GET /chest/current 调用 + POST /chest/open + 奖励弹窗 5 字段渲染 + 开箱前主动同步步数）+ Epic 23（user_cosmetic_items + GET /cosmetics/catalog / inventory）+ Epic 24 ~ 28（仓库 / 穿戴）+ Epic 29 / 30（render_config 渲染）+ Epic 32 / 33（合成事务 + UI，按 rarity 加权抽材料）** 可以基于一份**已落地、有真实数据、各品质数量满足 AR18 / 单测 / 集成测试已覆盖**的 cosmetic_items seed 数据集并行展开，不再出现"20.5 GET /chest/current 返回宝箱状态但 20.6 抽奖时表里没有 cosmetic 让 SELECT … LIMIT 1 命中 0 行 / 20.6 加权抽取连续 20 次只命中 1 件让 Story 19.1 节点 7 demo E2E "验证场景 8（抽奖多样性）连开 20 次 → 弹窗里能看到至少 3-4 种不同 cosmetic"挂掉 / 21.4 奖励弹窗 popup AsyncImage 因 asset_url 空而占位降级 / 32.x 合成事务按 rarity 抽 10 件 common 时表里 common 数量不够"的返工。

## 故事定位（Epic 20 第三条 = 第二条**实装** story；上承 20.2 表 + GORM struct 已就绪，下启 20.4 chest_open_logs migration + 20.5 GET /chest/current + 20.6 POST /chest/open 事务 + 20.7 / 20.8 dev 端点 + 20.9 Layer 2 集成测试 + iOS Epic 21 + Epic 23 + Epic 32 / 33）

- **Epic 20 进度**：20.1（契约定稿，done）→ 20.2（cosmetic_items migration + GORM struct，done）→ **20.3（本 story，cosmetic_items seed ≥15 行 + AR18 数量约束 + URL 非空 placeholder + drop_weight 按品质递减 + ON DUPLICATE KEY UPDATE 强制覆盖）** → 20.4（chest_open_logs migration）→ 20.5（GET /chest/current 接口）→ 20.6（POST /chest/open 事务 + idempotencyKey + 加权抽取）→ 20.7（dev 端点 POST /dev/force-unlock-chest）→ 20.8（dev 端点 POST /dev/grant-cosmetic-batch）→ 20.9（Layer 2 集成测试 - 开箱事务全流程）。
- **本 story 是 20.4 ~ 20.9 / Epic 21 / Epic 23 / Epic 32 / 33 的强前置**：
  - **20.6 POST /chest/open 加权抽取**：service / repo 层 `SELECT id, code, name, slot, rarity, asset_url, icon_url FROM cosmetic_items WHERE is_enabled = 1 AND drop_weight > 0 ORDER BY <加权算法>` 必须命中本 story 落地的 ≥15 行 seed 数据；20.6 dockertest 集成测试（Story 19.1 节点 7 demo E2E "验证场景 8 抽奖多样性"）钦定 "连开 20 次 → 弹窗里能看到至少 3-4 种不同 cosmetic" —— **直接依赖本 story 落地的 15+ 行 + drop_weight 分布合理 + AR18 数量约束**
  - **20.8 dev /dev/grant-cosmetic-batch**：handler `POST /dev/grant-cosmetic-batch {userId, rarity, count}` 按 rarity 从 cosmetic_items 中随机抽 count 个 cosmetic_item_id 创建 user_cosmetic_items 实例 —— 必须命中本 story 落地的"common ≥ 8 件 / rare ≥ 4 件 / epic ≥ 2 件 / legendary ≥ 1 件"，否则 dev 跑 `rarity=1, count=10` 时（demo 凑齐 10 件 common 用于合成）表里 common 不够会导致 dev 端点失败
  - **21.4 奖励弹窗 popup**：UI 层钦定 "AsyncImage(url: assetUrl) 加载 + 显示装扮名 / 品质边框 / 部位图标" —— 必须命中本 story 落地的每行非空 asset_url / icon_url，否则 demo 时弹窗会触发 placeholder 降级 / 破图
  - **Epic 19.1 节点 7 demo E2E**：钦定"验证场景 8（抽奖多样性）：连开 20 次 → 弹窗里能看到至少 3-4 种不同 cosmetic（验证 AR18 数量约束生效）" —— **直接依赖本 story 落地的 15+ 行 + AR18 数量约束**
  - **Epic 23 Story 23.3 GET /cosmetics/catalog**：handler 层 `SELECT * FROM cosmetic_items WHERE is_enabled = 1` 必须命中本 story 落地的 ≥15 行；epics.md 行 3254-3256 钦定 "Given Story 20.3 cosmetic_items 已 seed When 调用 GET /cosmetics/catalog Then 返回 {items: [...]}，仅含 is_enabled=1 的配置"
  - **Epic 29 Story 29.3 render_config seed**：epics.md 行 3844-3855 钦定 "Story 20.3 seed 的 15+ 个 cosmetic 全部补上合理的 render_config 数据" —— **直接依赖本 story 落地的 15+ 行 code 字面量清单**
  - **Epic 32 / 33 合成事务**：合成按 rarity 抽 10 件 common 升 1 件 rare（数量比例 10:1）→ 必须命中本 story 落地的 common ≥ 8（实际落地建议 ≥ 10 让合成 10 件凑得齐）
- **epics.md §Story 20.3 钦定**（行 2828-2851）：
  - seed 数据量满足 AR18：common ≥ **8 件，至少覆盖 4 个不同槽位（hat / gloves / glasses / neck 等）** / rare ≥ **4 件** / epic ≥ **2 件** / legendary ≥ **1 件** —— 总 ≥ 15 行
  - 每件 cosmetic 的 `icon_url` 与 `asset_url` 必须为可访问 URL（按 AR18 / AR19 URL 约束），MVP 阶段可用 placeholder（如 `https://placehold.co/128x128?text=Hat-Yellow`）
  - `drop_weight` 按品质递减分布（如 common=100, rare=20, epic=4, legendary=1），保证抽奖比例合理
  - seed 通过 migration 文件 `migrations/0012_seed_cosmetic_items.sql` 写入（**INSERT IGNORE 防重复** —— epics.md 文案，但本 story r0 决策升级为 **`INSERT ... ON DUPLICATE KEY UPDATE`** 强制覆盖语义，理由见下方"r0 锁定的 ON DUPLICATE KEY UPDATE vs INSERT IGNORE 选型"）
  - **单元测试覆盖**（≥4 case）：
    - happy: migrate up 后 cosmetic_items 至少 15 行 + URL 都非空
    - happy: 各品质数量符合 AR18 最小约束（common ≥ 8 / rare ≥ 4 / epic ≥ 2 / legendary ≥ 1）
    - happy: common 至少覆盖 4 个不同 slot 值
    - happy: 重复 migrate up → 不重复插入（本 story 实装为 ON DUPLICATE KEY UPDATE 强制覆盖，与 17.3 r3 最终决断模式一致；不翻倍到 30 行）
  - **集成测试覆盖**（dockertest）：migrate up → SELECT count(*) GROUP BY rarity → 验证各品质数量 ≥ AR18 约束
- **AR18 钦定**（epics.md 行 183-189）：
  - `cosmetic_items` 必须预置足够广度的配置集合，保证开箱 / 合成 demo 不会反复产出同一件
  - **MVP 最小数量约束**：common ≥ 8 件（**至少覆盖 4 个不同槽位**） / rare ≥ 4 件 / epic ≥ 2 件 / legendary ≥ 1 件
  - 在 Epic 20（宝箱 server）的 cosmetic seed story acceptance 中硬性写入
  - **URL 字段约束**：`icon_url` 与 `asset_url` 必须为可访问 URL（MVP 阶段可用 placeholder，例如 `https://placehold.co/128x128?text=Hat`），**不可为空字符串**。否则 demo 时开箱 popup / 仓库页会显示破图
- **V1 §7.2 reward 字段表钦定**（20.1 锚定）：
  - `reward.cosmeticItemId` BIGINT 字符串化（与 §2.5 全局约定 + DB `cosmetic_items.id BIGINT UNSIGNED` 一致）；length ≥ 1
  - `reward.name` 1 ≤ length ≤ 64；与 DB `cosmetic_items.name VARCHAR(64)` 一致
  - `reward.slot` int 枚举 ∈ {1,2,3,4,5,6,7,99}（§6.8）
  - `reward.rarity` int 枚举 ∈ {1,2,3,4}（§6.9）
  - `reward.assetUrl` 1 ≤ length ≤ 255；**不允许空字符串 ""**（开箱奖励必须有 asset）
  - `reward.iconUrl` 1 ≤ length ≤ 255；**不允许空字符串 ""**（开箱奖励必须有 icon）
- **17-3 r3 最终决断 lesson** `docs/lessons/2026-05-14-0010-final-decision-up-force-overwrite-down-narrow-delete.md` 钦定（本 story r0 直接复用同决策）：
  - **0010 owns 4 codes (wave/love/laugh/cry)** → 本 story 0012 owns 15+ codes (hat_yellow / gloves_pink / ... 钦定全列表见 AC1)
  - **up 强制覆盖** = `INSERT ... ON DUPLICATE KEY UPDATE` 覆盖所有非主键字段为 0012 钦定值
  - **down narrow DELETE** = `DELETE FROM cosmetic_items WHERE code IN (...)` 只删 0012 owned 的 15+ codes，不动 0013+ 新加的 cosmetic / admin 手工 INSERT 的非 owned codes
  - admin / 运维 **禁止**在 0012 owned 这 15+ 个 code 上做 customization；如要加 cosmetic **必须**新建 migration 0013+
- **17-3 r2 lesson** `docs/lessons/2026-05-14-down-must-undo-up-invariant-over-admin-data.md` 钦定：
  - down **必须**真正 undo up（golang-migrate framework invariant）；不能写 no-op `SELECT 1;` 路径
  - migration framework version ↔ data 一致是 hard constraint；admin 数据保留是 soft convention
- **Story 20.1 上游冻结边界**（V1 §7.2 reward 字段长度 + 数据库设计 §5.8 字段约束 + §6.8 / §6.9 枚举）：本 story 落地的 15+ 行 seed 数据必须严格符合 V1 §7.2 字段长度 + §5.8 DDL 字段类型 + §6.8 / §6.9 枚举值；本 story **不**反向修改 20.1 锚定的契约
- **数据库设计 §5.8 钦定**（20.2 落地的 DDL）：cosmetic_items 表已存在；`UNIQUE KEY uk_code (code)` 保证 ON DUPLICATE KEY UPDATE 兜底语义生效（命中 uk_code 走 UPDATE 路径）；`KEY idx_enabled_weight (is_enabled, drop_weight)` 是 20.6 加权抽取的索引根基
- **下游强依赖**（本 story done 后才能开工）：
  - Story 20.4（chest_open_logs migration，不直接依赖但属节点 7 阶段顺位）
  - Story 20.5（GET /chest/current 接口，不直接依赖 cosmetic_items 数据）
  - Story 20.6（POST /chest/open 事务 + 加权抽取，**直接依赖** seed 数据 + drop_weight 分布）
  - Story 20.7（dev /dev/force-unlock-chest，不直接依赖）
  - Story 20.8（dev /dev/grant-cosmetic-batch，**直接依赖** AR18 数量约束）
  - Story 20.9（Layer 2 集成测试 - 开箱事务全流程，**直接依赖** seed）
  - iOS Epic 21.1 ~ 21.5（首页宝箱组件 + GET /chest/current + POST /chest/open + 奖励弹窗 + 开箱前同步步数；21.4 弹窗 popup AsyncImage **直接依赖** seed 的 asset_url 非空）
  - Epic 19.1 节点 7 demo E2E（**直接依赖** AR18 + drop_weight 分布）
  - Epic 23 Story 23.3 GET /cosmetics/catalog（**直接依赖** seed）
  - Epic 29 Story 29.3 render_config seed（**直接依赖** seed 的 15+ codes 字面量清单做 UPDATE）
  - Epic 32 / 33 合成事务（**直接依赖** common ≥ 10 才能凑得齐 10 件合成材料）
- **范围红线**：
  - 本 story **只**改 `server/migrations/0012_seed_cosmetic_items.up.sql`（新建）+ `server/migrations/0012_seed_cosmetic_items.down.sql`（新建）+ `server/internal/infra/migrate/migrate_integration_test.go`（修改 `TestMigrateIntegration_StatusAfterUp` 版本号 v=11 → v=12 + 顶部注释追加 Story 20.3 扩展段 + 新增 `TestMigrateIntegration_CosmeticItems_SeedContent` + 新增 `TestMigrateIntegration_CosmeticItems_SeedForceOverwrite` 共 2 个新 case + 修改既有 `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` 与 seed 解耦，与 17.3 修复 `TestMigrateIntegration_EmojiConfigs_UniqueCode_Rejected` 同模式）+ 本 story 文件 + sprint-status.yaml 流转
  - **不**实装任何 service / handler / repo write / read 方法（20.5 / 20.6 / 20.8 / 23.3 才做）
  - **不**实装任何 GORM struct 变更（20.2 已落地 `CosmeticItem` struct，本 story **不**修改）
  - **不**实装 GET /chest/current / POST /chest/open / GET /cosmetics/catalog 等任何 handler / service（20.5 / 20.6 / 23.3 owner）
  - **不**接 Redis / chest_open_logs / user_cosmetic_items（10.6 / 20.4 / 23.2 owner）
  - **不**改 V1 接口契约（20.1 已冻结）
  - **不**改任何 `docs/宠物互动App_*.md`（§5.8 / §6.8 / §6.9 是契约**输入**，本 story seed 数据严格符合但**不修改**它；AR18 是契约**输入**，本 story 严格符合但**不修改**它）
  - **不**改 `_bmad-output/` 下其他 yaml / md（除自己 story 文件 + sprint-status.yaml 流转）
  - **不**改 ADR-0003（migration 工具 + 编号约定 + .up.sql / .down.sql 双向规范由 Story 4.3 已锚定，本 story 沿用）
  - **不**改 `cmd/server/main.go` migrate 子命令（已在 Story 4.3 落地）
  - **不**改 0001 ~ 0011 既有 migration 文件（17.3 落地 0010 seed / 20.2 落地 0011 schema，本 story 仅新增 0012 seed）
  - **不**修改 20.2 落地的 `cosmetic_item_repo.go`（GORM struct 不动；seed 走纯 SQL）
  - **不**新建 `CosmeticItemRepo` interface / 实装任何方法（YAGNI；20.6 owner）
  - **不**包含 `render_config` 字段值（20.2 落地的 0011 schema 不含 render_config 字段；节点 10 / Epic 29 Story 29.2 才加列 + 29.3 才 seed 数据）
  - **不**为 0012 写"prod 部署 seed 自动化 / 一键回滚 seed / dry-run"等运维化改造（保留最小集 up/down，与 0010 同模式）
  - **不**实装 admin 后台或动态 seed 接口（MVP 节点 7 仅静态 seed；未来若需要管理后台动态 add/disable cosmetic 由对应 epic 决定）

**本 story 不做**（明确范围红线）：

- 不实装任何 Go handler / service / repo write 或 read 方法（20.5 / 20.6 / 20.7 / 20.8 / 23.3 范围）
- 不修改 20.2 落地的 `cosmetic_item_repo.go`（GORM struct + TableName 已就绪；本 story 走纯 SQL seed）
- 不新建 `CosmeticItemRepo` interface（YAGNI；20.6 实装加权抽取时才落地 `CosmeticItemRepo` 类型 + `WeightedRandomPick(ctx) (*CosmeticItem, error)` / `ListEnabled(ctx)` 等方法）
- 不引入 Go 层 seed loader / fixture loader 框架（YAGNI；纯 SQL migration 文件已足够，与 ADR-0003 钦定一致 + 与 17.3 同模式）
- 不修改 0001 ~ 0011 既有 migration 文件（已在 Story 4.3 / 7.2 / 10.3 / 11.2 / 17.2 / 17.3 / 20.2 落地）
- 不修改 V1 接口契约（20.1 已冻结）
- 不修改数据库设计 §5.8 / §6.8 / §6.9（schema 输入，本 story 严格对齐不修改）
- 不修改 AR18 / AR19（架构钦定，本 story 严格对齐不修改）
- 不修改 Go 项目结构文档 §6（`migrations/` 目录已锚定；本 story 新增 0012 沿用既有目录规则）
- 不修改 ADR-0003（migration 工具 / 编号约定 / 文件命名规范由 Story 4.3 已锚定）
- 不写英文版测试注释 / 文档（项目 communication_language=Chinese；与 Story 4.3 / 7.2 / 10.3 / 11.2 / 17.2 / 17.3 / 20.2 一致）
- 不为 0012 写 stress test / 大数据量 seed test（15+ 行 seed 是 AR18 钦定最小量，单测 + dockertest 集成测试已覆盖核心约束）
- 不在本 story 内对 Story 20.4 ~ 20.9 / 23.x 实装做"提前预实装"（即使顺手写 `(r *cosmeticItemRepo) WeightedRandomPick(ctx) (*CosmeticItem, error)` 也禁止；这些方法是 20.6 / 23.x 钦定范围，提前 ship 会让评审找不到"新增方法"的明确范围边界，与 Story 17.3 / 20.2 "禁止预实装 repo 方法" 同模式）
- 不在本 story 内 seed `emoji_configs` / `chest_open_logs` / `user_cosmetic_items` 表（17.3 / 20.4 / 23.2 owner）
- 不为 0012 落地 admin / dev 端点（如 `POST /dev/seed-cosmetic` / `POST /dev/disable-cosmetic`）—— MVP 节点 7 不规划
- 不**预实装** `render_config` 字段值（即使顺手在 0012 SQL 加 `render_config TEXT` 列值也禁止；20.2 落地的 0011 schema 不含该字段；节点 10 / Epic 29 Story 29.2 加列 + 29.3 seed 数据 owner）

## Acceptance Criteria

**AC1 — 0012_seed_cosmetic_items.up.sql 新建（≥15 行 + AR18 数量约束 + ON DUPLICATE KEY UPDATE 强制覆盖 + 每行 asset_url / icon_url 非空 placeholder + drop_weight 按品质递减）**

新建 `server/migrations/0012_seed_cosmetic_items.up.sql`，内容必须**严格**符合 epics.md §Story 20.3（行 2828-2851）+ AR18（行 183-189）+ V1 §7.2 reward 字段长度约束 + 数据库设计 §5.8 / §6.8 / §6.9：

```sql
-- 对齐 epics.md §Story 20.3 + AR18 + V1接口设计.md §7.2 + 数据库设计.md §5.8 / §6.8 / §6.9
-- cosmetic_items 装扮配置 seed
--
-- **本 migration 由 Story 20.3 首次落地（Epic 20 节点 7 宝箱业务链路 seed owner）**
-- 含 ≥4 case 单测（seed 后 ≥15 行 + AR18 各品质数量 + common 至少覆盖 4 个不同 slot +
-- 每行 URL 非空 + 重复 migrate up 不重复插入 + ON DUPLICATE KEY UPDATE 强制覆盖）
-- + dockertest 集成测试覆盖 seed 内容正确 + 强制覆盖语义。
--
-- 装扮清单（满足 AR18 数量约束，与 epics.md §Story 20.3 行 2838-2845 钦定一致；
-- 总 15 件：common 8 / rare 4 / epic 2 / legendary 1）：
--
--   common（8 件，至少覆盖 4 个不同 slot；drop_weight=100）：
--     1.  hat_yellow      小黄帽       slot=1(hat)     rarity=1(common)
--     2.  hat_red         小红帽       slot=1(hat)     rarity=1(common)
--     3.  gloves_white    白手套       slot=2(gloves)  rarity=1(common)
--     4.  gloves_brown    棕手套       slot=2(gloves)  rarity=1(common)
--     5.  glasses_round   圆框眼镜     slot=3(glasses) rarity=1(common)
--     6.  neck_blue       蓝围脖       slot=4(neck)    rarity=1(common)
--     7.  back_bag        小书包       slot=5(back)    rarity=1(common)
--     8.  tail_ribbon     蝴蝶结尾巾   slot=7(tail)    rarity=1(common)
--   → common 覆盖 slot ∈ {1, 2, 3, 4, 5, 7}，共 6 个不同槽位（≥ 4 满足 AR18）
--
--   rare（4 件；drop_weight=20）：
--     9.  hat_chef        厨师帽       slot=1(hat)     rarity=2(rare)
--     10. glasses_star    星星眼镜     slot=3(glasses) rarity=2(rare)
--     11. neck_scarf_star 星星围巾     slot=4(neck)    rarity=2(rare)
--     12. body_tshirt     白T恤        slot=6(body)    rarity=2(rare)
--
--   epic（2 件；drop_weight=4）：
--     13. hat_crown       金王冠       slot=1(hat)     rarity=3(epic)
--     14. back_wings      天使翅膀     slot=5(back)    rarity=3(epic)
--
--   legendary（1 件；drop_weight=1）：
--     15. body_armor      黄金圣衣     slot=6(body)    rarity=4(legendary)
--
-- 字段值约束：
--   - code:       严格符合 §5.8 VARCHAR(64) + 业务命名约定 `{slot}_{name_en}`
--                 （如 hat_yellow / gloves_white / glasses_star）；
--                 与未来 Epic 29 Story 29.3 render_config seed 的 UPDATE 字面量 1:1 对齐
--   - name:       中文短名，长度 ≤ 64（§5.8 VARCHAR(64) NOT NULL）；
--                 与 V1 §7.2 reward.name `1 ≤ length ≤ 64` 一致
--   - slot:       §6.8 枚举值 ∈ {1,2,3,4,5,6,7,99}（hat / gloves / glasses /
--                 neck / back / body / tail / other）；本 seed **不**用 99=other
--                 槽位（保留给特殊冠名 cosmetic）
--   - rarity:     §6.9 枚举值 ∈ {1,2,3,4}（common / rare / epic / legendary）
--   - asset_url:  非空 placeholder URL `https://placehold.co/128x128?text={EnglishName}`
--                 （V1 §7.2 reward.assetUrl 1 ≤ length ≤ 255 + 禁止空字符串；AR18
--                 钦定 MVP 阶段允许 placeholder URL，真实美术资产由 Epic 20 retro
--                 / 后续 epic 切换；与 17.3 placehold.co 同源服务）
--   - icon_url:   非空 placeholder URL `https://placehold.co/64x64?text={EnglishName}`
--                 （V1 §7.2 reward.iconUrl 1 ≤ length ≤ 255 + 禁止空字符串；
--                 64x64 是小尺寸预览图，仓库 grid / 奖励弹窗 icon 用）
--   - drop_weight: 按品质递减分布（common=100 / rare=20 / epic=4 / legendary=1，
--                 与 epics.md §Story 20.3 行 2844 钦定一致）；权重比 100:20:4:1 ≈
--                 25:5:1:0.25 让连开 20 次能命中 3-4 种不同 cosmetic（验证 AR18
--                 + Story 19.1 节点 7 demo E2E "验证场景 8 抽奖多样性"）
--   - is_enabled: 全部 1（enabled；V1 §7.2 加权抽取 + §8.1 GET /cosmetics/catalog
--                 仅返回 / 命中 is_enabled=1 的 cosmetic）
--
-- ============================================================================
-- **最终决策（r0 锁定，沿用 17-3 r3 lesson；不再走 r1 / r2 / r3 三轮 review）**
-- ============================================================================
-- **0012 owns 15 codes**（hat_yellow / hat_red / gloves_white / gloves_brown /
-- glasses_round / neck_blue / back_bag / tail_ribbon / hat_chef / glasses_star /
-- neck_scarf_star / body_tshirt / hat_crown / back_wings / body_armor）由 0012
-- 完全占用 / 强制覆盖。admin / 运维 **禁止**在这 15 个 code 上做 customization；
-- 如要加 cosmetic **必须**新建 migration 0013+（如 `0013_seed_cosmetic_seasonal.up.sql`）。
--
-- 配套：
--   - **up 强制覆盖** = `INSERT ... ON DUPLICATE KEY UPDATE`（覆盖 name / slot / rarity /
--     asset_url / icon_url / drop_weight / is_enabled 7 字段为 0012 钦定值）
--   - **down narrow DELETE** = `DELETE FROM cosmetic_items WHERE code IN (15 个钦定 code)`
--
-- 这一对决策共同保证：Story 20.5 / 20.6 / 20.8 / Epic 21 / Epic 23 / Epic 29 Story 29.3 /
-- Epic 32 / 33 依赖的 **15 个 enabled cosmetic 配置 invariant 100% 强保证**，
-- 不依赖任何 admin 自律。即使 admin 误操作 / dev 历史残留 / migrate force / 手工
-- mysql import 等异常路径让 cosmetic_items 表里预先存在这 15 个 code 的"坏行"
-- （is_enabled=0 / asset_url='' / drop_weight 漂移 / rarity 漂移 / slot 漂移 /
-- icon_url=''），跑 0012.up 后**也会**被覆盖回 0012 钦定值。
--
-- **r0 直接复用 17-3 r3 最终决断**（不再走 r1 / r2 / r3 三轮反复）：
--   - 17.3 落地 emoji_configs seed 时经历 INSERT IGNORE → no-op down → narrow DELETE
--     down → ON DUPLICATE KEY UPDATE up 的反复决策路径，最终落定为本 story 直接
--     沿用的"up 强制覆盖 + down narrow DELETE"模式
--   - 详见 lesson：
--     - docs/lessons/2026-05-14-down-must-undo-up-invariant-over-admin-data.md (17-3 r2)
--     - docs/lessons/2026-05-14-0010-final-decision-up-force-overwrite-down-narrow-delete.md (17-3 r3, **最终决断**)
--
-- **不**用 INSERT IGNORE：因为 IGNORE 路径下，admin 预存的"坏行"（如 `is_enabled=0`
-- 把某 cosmetic 临时下架 / `asset_url=''` 让奖励弹窗破图 / `drop_weight=0` 让某
-- cosmetic 永远抽不到）会**幸存** → Story 20.6 加权抽取 + 21.4 奖励弹窗 + 19.1
-- E2E 钦定的 "AR18 数量约束生效"无法 100% 保证。ON DUPLICATE KEY UPDATE 强制覆盖
-- 是唯一能让"15 个 enabled cosmetic invariant"100% 强保证的路径。
--
-- **不**用 REPLACE INTO：REPLACE 是 DELETE+INSERT 实现，会重新分配 id；
-- 虽然 0011 schema 当前 cosmetic_items.id 没被外键引用（user_cosmetic_items /
-- chest_open_logs.reward_cosmetic_item_id 都不建 FK），但 REPLACE 触发的 id 变化
-- 会让 chest_open_logs.reward_cosmeticItemId 的历史日志和当前 cosmetic_items.id
-- 断开 reference 语义；ON DUPLICATE KEY UPDATE 走 UPDATE 路径保留 id 不变，安全。
--
-- **范围红线**：本 migration **仅** UPSERT 15 行；不修改 schema（20.2 owner）/
-- 不含任何业务 service / handler / repo write 方法（20.5 / 20.6 / 20.7 / 20.8 /
-- 23.3 落地）/ 不含 TRUNCATE / 全表 DELETE 任何破坏性 SQL / 不含 render_config
-- 字段（节点 10 / Epic 29 Story 29.2 加列 + 29.3 seed owner，本 story 故意保留扩展空间）。
--
-- **不**用 server 端代码层（如 Go 的 seedCosmetics() 函数）做 seed：
-- (a) ADR-0003 钦定 migrations/ 文件是 schema + 静态数据的真相源；
-- (b) seed 通过 SQL migration 让 dev / test / staging / prod 同一份数据；
-- (c) 与 emoji seed（0010_seed_emoji_configs）落地路径同模式，避免每个 seed 自己
--     定一种执行方式。
INSERT INTO cosmetic_items (code, name, slot, rarity, asset_url, icon_url, drop_weight, is_enabled) VALUES
    -- common（8 件，覆盖 6 个不同 slot；drop_weight=100）
    ('hat_yellow',      '小黄帽',       1, 1, 'https://placehold.co/128x128?text=Hat-Yellow',     'https://placehold.co/64x64?text=Hat-Yellow',     100, 1),
    ('hat_red',         '小红帽',       1, 1, 'https://placehold.co/128x128?text=Hat-Red',        'https://placehold.co/64x64?text=Hat-Red',        100, 1),
    ('gloves_white',    '白手套',       2, 1, 'https://placehold.co/128x128?text=Gloves-White',   'https://placehold.co/64x64?text=Gloves-White',   100, 1),
    ('gloves_brown',    '棕手套',       2, 1, 'https://placehold.co/128x128?text=Gloves-Brown',   'https://placehold.co/64x64?text=Gloves-Brown',   100, 1),
    ('glasses_round',   '圆框眼镜',     3, 1, 'https://placehold.co/128x128?text=Glasses-Round',  'https://placehold.co/64x64?text=Glasses-Round',  100, 1),
    ('neck_blue',       '蓝围脖',       4, 1, 'https://placehold.co/128x128?text=Neck-Blue',      'https://placehold.co/64x64?text=Neck-Blue',      100, 1),
    ('back_bag',        '小书包',       5, 1, 'https://placehold.co/128x128?text=Back-Bag',       'https://placehold.co/64x64?text=Back-Bag',       100, 1),
    ('tail_ribbon',     '蝴蝶结尾巾',   7, 1, 'https://placehold.co/128x128?text=Tail-Ribbon',    'https://placehold.co/64x64?text=Tail-Ribbon',    100, 1),
    -- rare（4 件；drop_weight=20）
    ('hat_chef',        '厨师帽',       1, 2, 'https://placehold.co/128x128?text=Hat-Chef',       'https://placehold.co/64x64?text=Hat-Chef',        20, 1),
    ('glasses_star',    '星星眼镜',     3, 2, 'https://placehold.co/128x128?text=Glasses-Star',   'https://placehold.co/64x64?text=Glasses-Star',    20, 1),
    ('neck_scarf_star', '星星围巾',     4, 2, 'https://placehold.co/128x128?text=Neck-Scarf-Star','https://placehold.co/64x64?text=Neck-Scarf-Star', 20, 1),
    ('body_tshirt',     '白T恤',        6, 2, 'https://placehold.co/128x128?text=Body-Tshirt',    'https://placehold.co/64x64?text=Body-Tshirt',     20, 1),
    -- epic（2 件；drop_weight=4）
    ('hat_crown',       '金王冠',       1, 3, 'https://placehold.co/128x128?text=Hat-Crown',      'https://placehold.co/64x64?text=Hat-Crown',        4, 1),
    ('back_wings',      '天使翅膀',     5, 3, 'https://placehold.co/128x128?text=Back-Wings',     'https://placehold.co/64x64?text=Back-Wings',       4, 1),
    -- legendary（1 件；drop_weight=1）
    ('body_armor',      '黄金圣衣',     6, 4, 'https://placehold.co/128x128?text=Body-Armor',     'https://placehold.co/64x64?text=Body-Armor',       1, 1)
ON DUPLICATE KEY UPDATE
    name        = VALUES(name),
    slot        = VALUES(slot),
    rarity      = VALUES(rarity),
    asset_url   = VALUES(asset_url),
    icon_url    = VALUES(icon_url),
    drop_weight = VALUES(drop_weight),
    is_enabled  = VALUES(is_enabled);
```

- **15 个 code 字面量**严格符合 AR18 数量约束（common 8 / rare 4 / epic 2 / legendary 1 = 15 件），不多不少（少 → 违反 AR18；多 → 让本 story 测试 case "rows ≥ 15" 仍 pass 但 27.x / 32.x 数量 baseline 漂移）
- **common 至少覆盖 4 个不同 slot**：本 seed common 行覆盖 slot ∈ {1, 2, 3, 4, 5, 7}，共 6 个不同槽位（≥ 4 满足 AR18 + epics.md §Story 20.3 行 2839 钦定）
- **每个 asset_url 非空** + **每个 icon_url 非空**（V1 §7.2 reward.assetUrl / iconUrl 钦定禁止 ""）：使用 `https://placehold.co/128x128?text={EnglishName}` / `https://placehold.co/64x64?text={EnglishName}` 等 placeholder URL；**禁止**任何 `""` 空字符串
- **`INSERT ... ON DUPLICATE KEY UPDATE` 7 字段强制覆盖**（r0 锁定 / 17-3 r3 模式）：覆盖 `name / slot / rarity / asset_url / icon_url / drop_weight / is_enabled` 7 个字段为 0012 钦定值（**不**覆盖 `id` / `created_at` —— `id` 是 AUTO_INCREMENT 主键不能改；`created_at` 是历史首次写入的真实时间不应被 seed 覆盖；`updated_at` 由 DDL `ON UPDATE CURRENT_TIMESTAMP(3)` 自动刷新）
- **drop_weight 按品质递减分布**（与 epics.md §Story 20.3 行 2844 钦定一致）：common=100 / rare=20 / epic=4 / legendary=1
- **is_enabled 全部 1**：enabled 装扮才会被 V1 §8.1 GET /cosmetics/catalog 返回 + 才会被 V1 §7.2 加权抽取命中
- **slot 严格 ∈ {1,2,3,4,5,6,7}** —— 本 seed **不**用 99=other（保留给未来特殊冠名 / 限定皮肤）
- **rarity 严格 ∈ {1,2,3,4}**
- **不**包含 `created_at` / `updated_at` / `id` 显式值：DDL `DEFAULT CURRENT_TIMESTAMP(3)` / `AUTO_INCREMENT` 兜底（与 0010 / 17.3 一致）
- 文件编码 UTF-8 + LF 行尾（与 0001 ~ 0011 一致）
- 顶部注释模板与 0010_seed_emoji_configs.up.sql（17.3 落地）一致 —— "对齐 §X.Y" + 装扮清单 + 字段约束 + 最终决策段（17-3 r3 复用） + 范围红线四段式
- **不**包含任何业务 logic SQL（如 `UPDATE cosmetic_items SET …`）
- **不**包含 `render_config` 字段值（节点 10 / Epic 29 Story 29.2 / 29.3 owner）
- **不**包含 16 件 + 装扮（保留 0013+ 季节性 / 主题包 cosmetic 的扩展空间）

**AC2 — 0012_seed_cosmetic_items.down.sql 新建（narrow DELETE 15 行）**

新建 `server/migrations/0012_seed_cosmetic_items.down.sql`，内容：

```sql
-- 回滚 0012_seed_cosmetic_items.up.sql
--
-- **本 migration 由 Story 20.3 首次落地（Epic 20 节点 7 宝箱业务链路 seed owner）**
-- 含 ≥4 case 单测 + dockertest 集成测试覆盖 seed 内容正确 + ON DUPLICATE KEY UPDATE
-- 强制覆盖语义。
--
-- 回滚策略：**narrow DELETE 15 行**（精确删 0012 owned 的 15 个 code；
-- **不**用 TRUNCATE 也**不**用全表 DELETE）—— 防误删 admin / dev / 0013+ migration
-- 后续插入的非 0012 seed 数据。
--
-- ============================================================================
-- **最终决策（r0 锁定，沿用 17-3 r3 lesson）**
-- ============================================================================
-- 0012 owns 15 codes（见 0012_seed_cosmetic_items.up.sql 顶部清单）由 0012
-- 完全占用 / 强制覆盖。配套：
--   - **up 强制覆盖** = `INSERT ... ON DUPLICATE KEY UPDATE`（覆盖 7 字段为 0012 钦定值）
--   - **down narrow DELETE** = 当前文件下面这行 SQL（只删 0012 owned 的 15 个 code）
--
-- **强约定（admin / 运维必读）**：
--   1. 0012 owned 的 15 个 code（hat_yellow / hat_red / gloves_white /
--      gloves_brown / glasses_round / neck_blue / back_bag / tail_ribbon /
--      hat_chef / glasses_star / neck_scarf_star / body_tshirt / hat_crown /
--      back_wings / body_armor）**由 0012 完全占用 / 强制覆盖**；admin / 运维
--      **禁止**手工 INSERT / UPDATE 这 15 个 code 的行（up 重跑会被覆盖；
--      down 会被删除；任何 customization 都无法存活）。
--   2. 需要新增装扮（如 season_winter / event_lunar_2026）→ 通过**新 migration**
--      （0013+）添加，不要在 cosmetic_items 表上做 admin 直插。
--   3. 本 down 是 **narrow DELETE 15 行**（不是 TRUNCATE、不是全表 DELETE）：
--      只删 15 个钦定 code，**不会动** 0013+ 加的新装扮 / admin 手工 INSERT 的
--      非 owned codes（如 admin 测试时插的 `code='admin_test_001'` 行）。
--
-- 详见 lesson：
--   - docs/lessons/2026-05-14-down-must-undo-up-invariant-over-admin-data.md (17-3 r2)
--   - docs/lessons/2026-05-14-0010-final-decision-up-force-overwrite-down-narrow-delete.md (17-3 r3, **最终决断**)
--
-- **down 实际执行场景**（与 golang-migrate 语义对齐）：
--   (a) 单跑 0012.down（保留 0011 schema）→ DELETE 15 行 → schema_migrations 回退到 v=11
--       → cosmetic_items 表存在但 0012 owned 15 个 code 不在；再 up 一次会重新跑
--       0012.up（ON DUPLICATE KEY UPDATE）把 15 行 INSERT/覆盖回来。
--   (b) 跑 0011.down 链式带 0012.down 先跑 → 先 DELETE 15 行 → 再 DROP TABLE
--       cosmetic_items → 整张表清理；语义自洽。
DELETE FROM cosmetic_items WHERE code IN (
    'hat_yellow', 'hat_red', 'gloves_white', 'gloves_brown',
    'glasses_round', 'neck_blue', 'back_bag', 'tail_ribbon',
    'hat_chef', 'glasses_star', 'neck_scarf_star', 'body_tshirt',
    'hat_crown', 'back_wings', 'body_armor'
);
```

- 文件编码 UTF-8 + LF 行尾
- **仅** 1 条 `DELETE FROM cosmetic_items WHERE code IN (...)`，不含任何额外 cleanup 语句
- **15 个 code 字面量**严格与 0012.up.sql 钦定 15 个 code 1:1 对齐（任何漂移会让 down 漏删 / 误删）
- **不**用 `TRUNCATE TABLE cosmetic_items`（避免重置 AUTO_INCREMENT + 破坏 0013+ / admin 手工插入的非 0012 seed 数据）
- **不**用 `DELETE FROM cosmetic_items WHERE 1=1`（避免误删 admin / dev 手工写入的非 0012 数据）
- 与 0011.down.sql 的 `DROP TABLE IF EXISTS` 不同 —— 因为 0012 是 seed migration 而非 schema migration，down 只回滚数据**不**回滚表本身（表回滚由 0011.down 负责）

**AC3 — migrate_integration_test.go 扩展（版本号断言 v=11 → v=12 + seed 内容验证 + 强制覆盖验证 + UniqueCode_Rejected 与 seed 解耦）**

修改 `server/internal/infra/migrate/migrate_integration_test.go`：

**AC3.1 修改 `TestMigrateIntegration_StatusAfterUp` 版本号断言**：

- 找到 `if v != 11 {` → 改为 `if v != 12 {`
- 找到 `t.Errorf("Status version = %d, want 11", v)` → 改为 `t.Errorf("Status version = %d, want 12", v)`
- 同步更新顶部 case 注释 / docstring：追加一行 `// Story 20.3 扩展：从 11 改 12（多了 0012_seed_cosmetic_items；Epic 20 节点 7 cosmetic seed）`

**AC3.2 顶部 testfile 注释追加 Story 20.3 扩展段**（文件 1-44 行附近）：

```go
// Story 20.3 扩展：在表 schema 不变（仍 10 张）基础上新增 0012_seed_cosmetic_items；
//   主要 case 跑 15 行 seed 的内容正确性 + AR18 各品质数量约束 + common 至少覆盖
//   4 个不同 slot + ON DUPLICATE KEY UPDATE 强制覆盖语义（不影响表数量断言）；
//   StatusAfterUp 版本号断言从 v=11 改 v=12。
```

**AC3.3 既有 `TestMigrateIntegration_UpThenDown` / `TestMigrateIntegration_UpTwice_Idempotent` 不动表数量断言**：
- 0012 是 **seed migration**，**不**新增表 → `expectedTables` slice 仍 10 张表（含 cosmetic_items，本 story 不动）
- `TestMigrateIntegration_UpTwice_Idempotent` 中 `table_name IN (...)` 也保持 10 张表，仅断言"两次 up 不重复建表"，**不**断言 seed 数据条数（seed 数据条数由 AC3.4 新 case 验证）

**AC3.4 新增 `TestMigrateIntegration_CosmeticItems_SeedContent`**（紧接 `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` 之后；约文件行 1620+ 之后）：

```go
// TestMigrateIntegration_CosmeticItems_SeedContent 验证
// migrations/0012_seed_cosmetic_items.up.sql 钦定的 15 个装扮 seed 在 migrate up
// 后真实写入 cosmetic_items 表，且每行字段值符合 epics.md §Story 20.3 + AR18 +
// V1 §7.2 reward 字段约束：
//
//   - 至少 15 行存在（实际 0012 钦定 15 行；用 >= 15 而非 == 15 兼容未来 0013+
//     新 migration 加 cosmetic）
//   - 各 rarity 数量符合 AR18：
//       rarity=1(common)    ≥ 8
//       rarity=2(rare)      ≥ 4
//       rarity=3(epic)      ≥ 2
//       rarity=4(legendary) ≥ 1
//   - common 至少覆盖 4 个不同 slot（AR18 钦定 + epics.md §Story 20.3 行 2839）
//   - 每行 asset_url 非空（V1 §7.2 reward.assetUrl 钦定 length ≥ 1 + 禁止 ""）
//   - 每行 icon_url 非空（V1 §7.2 reward.iconUrl 钦定 length ≥ 1 + 禁止 ""）
//   - 每行 is_enabled = 1（enabled 才会被 GET /cosmetics/catalog 返回 + 加权抽取命中）
//   - 每行 name 非空（VARCHAR(64) NOT NULL）
//   - 每行 slot ∈ {1,2,3,4,5,6,7,99}（§6.8 枚举值；本 case 用 set 校验）
//   - 每行 rarity ∈ {1,2,3,4}（§6.9 枚举值；本 case 用 set 校验）
//   - 各 rarity 的 drop_weight 按品质递减分布（common > rare > epic > legendary；
//     0012 钦定 common=100 / rare=20 / epic=4 / legendary=1）
//
// **背景（Story 20.3 引入）**：epics.md §Story 20.3 钦定的"集成测试覆盖（dockertest）：
// migrate up → SELECT count(*) GROUP BY rarity → 验证各品质数量 ≥ AR18 约束"
// 路径在本 case 落地；用于 Story 20.4 ~ 20.9 / iOS Epic 21 / Epic 19.1 节点 7
// demo E2E / Epic 23 / Epic 29 Story 29.3 / Epic 32 / 33 实装时验证 seed 数据
// 真实在位 + AR18 数量约束 100% 强保证的根基。
//
// 用 database/sql 直跑 SELECT（**不**走 GORM）让测试结果**不**依赖 ORM 行为差异
// （与 17.3 / 11.2 / 17.2 落地的 dockertest case 同模式）。
func TestMigrateIntegration_CosmeticItems_SeedContent(t *testing.T) {
    // 实装细节由 dev-story 阶段补全；模板参考既有
    // TestMigrateIntegration_EmojiConfigs_SeedContent（行 980 ~ 1084）+
    // TestMigrateIntegration_CosmeticItems_Schema（行 1304 ~ 1535）。
    //
    // 必查项（每项失败立即 t.Errorf，不 t.Fatalf —— 用 batch 累积报错风格）：
    //   1. SELECT code, name, slot, rarity, asset_url, icon_url, drop_weight, is_enabled
    //      FROM cosmetic_items ORDER BY rarity ASC, slot ASC, code ASC
    //      → 至少 15 行（>= 15）
    //   2. 各 rarity 数量 GROUP BY：
    //      a. rarity=1(common)    count >= 8 (AR18)
    //      b. rarity=2(rare)      count >= 4 (AR18)
    //      c. rarity=3(epic)      count >= 2 (AR18)
    //      d. rarity=4(legendary) count >= 1 (AR18)
    //   3. common（rarity=1）的 slot 至少覆盖 4 个不同值（去重后 len(set) >= 4；
    //      AR18 行 184 钦定 + epics.md §Story 20.3 行 2839 钦定）
    //   4. 对每一行：
    //      a. name 非空（len > 0）
    //      b. asset_url 非空（len > 0；V1 §7.2 reward.assetUrl + AR18 钦定）
    //      c. icon_url 非空（len > 0；V1 §7.2 reward.iconUrl + AR18 钦定）
    //      d. is_enabled == 1
    //      e. slot ∈ {1,2,3,4,5,6,7,99}（§6.8）
    //      f. rarity ∈ {1,2,3,4}（§6.9）
    //   5. 各 rarity 的 drop_weight 中位数 / max 按品质递减（epics.md §Story 20.3
    //      行 2844 钦定 common=100 > rare=20 > epic=4 > legendary=1）：
    //      用 GROUP BY rarity + MIN(drop_weight) / MAX(drop_weight) 断言：
    //      a. common 的 MIN(drop_weight) > rare 的 MAX(drop_weight)（100 > 20）
    //      b. rare 的 MIN(drop_weight) > epic 的 MAX(drop_weight)（20 > 4）
    //      c. epic 的 MIN(drop_weight) > legendary 的 MAX(drop_weight)（4 > 1）
}
```

**AC3.5 新增 `TestMigrateIntegration_CosmeticItems_SeedForceOverwrite`**（紧接 AC3.4 case 之后）：

```go
// TestMigrateIntegration_CosmeticItems_SeedForceOverwrite 验证
// migrations/0012_seed_cosmetic_items.up.sql 钦定的 ON DUPLICATE KEY UPDATE 语义在
// duplicate-code 路径下的 server 端表现：
// **当 UNIQUE KEY uk_code 命中时，ON DUPLICATE KEY UPDATE 不报错 + 不翻倍 +
// 强制把 name / slot / rarity / asset_url / icon_url / drop_weight / is_enabled
// 7 字段覆盖回 0012 钦定值**。
//
// **背景（Story 20.3 引入；r0 直接复用 17-3 r3 决断）**：
// epics.md §Story 20.3 钦定的"重复 migrate up → 不重复插入"+ 17-3 r3 lesson
// "0010 owns 4 codes / up 强制覆盖" 路径在本 case 落地，与
// TestMigrateIntegration_EmojiConfigs_SeedIdempotent 同模式（行 1130 ~ 1303）。
//
// **覆盖路径**：
//  1. migrate Up 全程 (v=12) → cosmetic_items 15 行（0012 seed 写入）
//  2. DELETE seed 15 行 + 手动 INSERT 15 行 admin-flavored 数据（name / slot /
//     rarity / asset_url / icon_url / drop_weight / is_enabled 7 字段都和 seed
//     不同，模拟 admin 在 0012 owned codes 上做了"违规 customization"，包括
//     is_enabled=0 把某 cosmetic 临时下架 / asset_url='' 让奖励弹窗破图 /
//     drop_weight=0 让某 cosmetic 永远抽不到）
//  3. UPDATE schema_migrations SET version = 11 → 让 golang-migrate 认为 0012 还没跑过
//  4. migrate Up 重跑 → 触发 0012.up ON DUPLICATE KEY UPDATE 命中 uk_code 15 次
//  5. 断言：
//     a. 行数仍 15（不翻倍到 30）
//     b. 7 字段被**强制覆盖回** 0012 seed 钦定值（hat_yellow 的 name 恢复为
//        "小黄帽" / slot 恢复为 1 / rarity 恢复为 1 / asset_url 恢复为
//        "https://placehold.co/128x128?text=Hat-Yellow" / icon_url 恢复为
//        "https://placehold.co/64x64?text=Hat-Yellow" / drop_weight 恢复为 100 /
//        is_enabled 恢复为 1；其他 14 行同理逐字段抽样验证）
//     这是 r0 决策"0012 owns these 15 codes"的 100% 强保证。
//
// **三种"伪幂等"实现仍被本测试抓到**：
//   - INSERT INTO（无 IGNORE / ON DUPLICATE KEY UPDATE）→ 步骤 4 撞 uk_code 1062 直接报错
//   - INSERT IGNORE → 行数对但 admin 7 字段**保留不被覆盖** → 步骤 5b 断言炸
//     （断言期望"覆盖回 seed 值"，IGNORE 路径下还是 admin 值）
//   - REPLACE INTO → 行数对、字段覆盖正确，但 REPLACE 触发 id 重排会让
//     chest_open_logs.reward_cosmetic_item_id 历史日志断开 reference 语义；
//     虽然字段断言可能能过，但语义不对（这里靠 SQL review 兜底，本 case 不强测）
//
// **为什么不走 force(11)**：本 migrate 包没暴露 Force API（migrate.go 仅 Up / Down
// / Status / Close）。直接 UPDATE schema_migrations 是 dockertest 集成测试可控
// 范围内的最小操作；也忠实模拟了 ops 在生产里手工修复 dirty 后的回退场景；
// 与 17.3 落地的 TestMigrateIntegration_EmojiConfigs_SeedIdempotent 同模式。
//
// 用 database/sql 直跑 SQL（**不**走 GORM）。
func TestMigrateIntegration_CosmeticItems_SeedForceOverwrite(t *testing.T) {
    // 实装细节由 dev-story 阶段补全；模板参考既有
    // TestMigrateIntegration_EmojiConfigs_SeedIdempotent（行 1130 ~ 1303）。
    //
    // 必查项：
    //   1. migrate Up → SELECT COUNT(*) FROM cosmetic_items WHERE code IN (15 codes) → 15
    //   2. DELETE seed 15 行 → 手动 INSERT 15 行 admin-flavored 数据（7 字段都和 seed 不同）
    //   3. UPDATE schema_migrations SET version = 11
    //   4. migrate Up → 不报错（ON DUPLICATE KEY UPDATE 命中 uk_code 走 UPDATE 路径）
    //   5. 断言：
    //      a. SELECT COUNT(*) FROM cosmetic_items WHERE code IN (15 codes) → 15（不翻倍 30）
    //      b. 抽样 3 ~ 5 行（如 hat_yellow / body_armor / hat_crown）逐字段断言 7 字段
    //         被覆盖回 seed 钦定值
}
```

**AC3.6 修复 `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` 与 0012 seed 解耦**：

20.2 落地的 `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` case（约文件行 1536+）使用了测试专用 code `test_unique_cosmetic_a`（按 20.2 AC4.5 钦定，已经与 0011 schema layer 解耦），但本 story 0012 seed 落地后该 case 跑时**会先经过 0012 migrate up 写入 15 行**（包括 `hat_yellow` 等）；既有 case 用 `test_unique_cosmetic_a` 与 0012 owned 15 个 code 字面量不冲突 → **本 case 无需改动**（与 17.3 落地 0010 seed 后必须改 17.2 `EmojiConfigs_UniqueCode_Rejected` 用 `'wave'` 字面量的情况不同 —— 17.2 case 偷懒用了业务 code `'wave'` 导致 17.3 必须解耦；20.2 已经吸取教训用了 `'test_unique_cosmetic_a'`，本 story 不需要再改）。

**只**需在该 case 函数体顶部追加一行注释说明："**用测试专用 code (test_unique_cosmetic_a) 与 0012 seed 的 15 个 owned codes 字面量隔离**（hat_yellow / hat_red / gloves_white / gloves_brown / glasses_round / neck_blue / back_bag / tail_ribbon / hat_chef / glasses_star / neck_scarf_star / body_tshirt / hat_crown / back_wings / body_armor），避免 seed 先写入后该 case 第一次 INSERT 就触发 UNIQUE"。

- 两个新 case 都用 `dockertest` 起 mysql:8.0 容器（沿用 `startMySQL(t)` + `migrationsPath(t)` helper，与既有 case 一致）
- 用 `database/sql` 直跑 raw SELECT / INSERT / UPDATE（**不**走 GORM）—— 与 17.3 落地的 SeedContent / SeedIdempotent case 同模式
- 测试断言风格：`t.Errorf` 累积报错（**不** `t.Fatalf`），便于一次 run 看全部失败点（与 17.3 / 17.2 / 11.2 同模式）
- 关键 fixture 操作 `UPDATE schema_migrations SET version = 11` 与 17.3 SeedIdempotent 同模式（手工回滚框架版本号让 0012.up 第二次重跑）

**AC4 — 验证步骤**

- **AC4.1 build 验证**：执行 `bash scripts/build.sh --test` 必须**全绿**（vet + 全 unit 测试通过 —— 本 story 不新增任何 Go .go 业务源文件，纯 SQL + 集成 test go 文件改动；既有单测无回归）；`go vet -tags=integration ./...` 必须**全绿**（含新增 2 case + 修改的 UniqueCode_Rejected 注释）；`bash scripts/build.sh --integration` 必须**全绿**（新增 `TestMigrateIntegration_CosmeticItems_SeedContent` + `TestMigrateIntegration_CosmeticItems_SeedForceOverwrite` 两个 case 跑通 + 既有 `TestMigrateIntegration_StatusAfterUp` 版本号 12 断言通过 + 既有 `TestMigrateIntegration_UpThenDown` / `TestMigrateIntegration_UpTwice_Idempotent` 表数量 10 断言不回归 + 既有 `TestMigrateIntegration_CosmeticItems_Schema` / `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` / `TestMigrateIntegration_EmojiConfigs_*` 不回归）
- **AC4.2 git diff 范围检查**：编辑完成后 `git diff` 输出**仅**包含：
  - `server/migrations/0012_seed_cosmetic_items.up.sql`（新增）
  - `server/migrations/0012_seed_cosmetic_items.down.sql`（新增）
  - `server/internal/infra/migrate/migrate_integration_test.go`（修改：版本号 11 → 12 + 顶部注释追加 Story 20.3 段 + UniqueCode_Rejected case 顶部追加 seed 字面量隔离注释 + 新增 2 case）
  - `_bmad-output/implementation-artifacts/20-3-cosmetic_items-seed.md`（本 story 文件状态流转 + Tasks/Subtasks 勾选 + Dev Agent Record / File List / Change Log）
  - `_bmad-output/implementation-artifacts/sprint-status.yaml`（状态行 + last_updated）
- **AC4.3 seed 内容跨文档一致性**：手动检查 0012 up.sql 的 15 个 cosmetic 字段值符合 §5.8 DDL 约束 + §6.8 / §6.9 枚举 + V1 §7.2 reward 字段长度约束 + AR18 数量约束（common ≥ 8 / rare ≥ 4 / epic ≥ 2 / legendary ≥ 1）+ common 至少覆盖 4 个不同 slot + 每个 asset_url / icon_url 非空 + drop_weight 按品质递减
- **AC4.4 ON DUPLICATE KEY UPDATE 语法正确性**：手动 SQL 解析 0012 up.sql 必须含 `INSERT INTO cosmetic_items ... VALUES ... ON DUPLICATE KEY UPDATE` 关键字（**不**是 `INSERT IGNORE INTO`；**不**是 `REPLACE INTO`）；`ON DUPLICATE KEY UPDATE` 必须覆盖 7 个字段（name / slot / rarity / asset_url / icon_url / drop_weight / is_enabled），**不**覆盖 id / created_at / updated_at
- **AC4.5 down narrow DELETE 语法正确性**：手动 SQL 解析 0012 down.sql 必须含 `DELETE FROM cosmetic_items WHERE code IN (15 codes)` 关键字（**不**是 `TRUNCATE TABLE cosmetic_items`；**不**是 `DELETE FROM cosmetic_items` 全表删；**不**是 `DELETE FROM cosmetic_items WHERE 1=1`）；15 个 code 字面量与 up.sql 1:1 对齐
- **AC4.6 既有 migrate 集成测试不回归**：跑 `bash scripts/build.sh --integration` 全绿 —— 含 20.2 / 17.3 / 17.2 / 11.2 落地的所有 EmojiConfigs / CosmeticItems / RoomMembers / UpThenDown / UpTwice_Idempotent / StatusAfterUp / TablesPresent_AfterUp 各 case

## Tasks / Subtasks

- [x] Task 1: 准备阶段（AC: #1, #2, #3, #4）
  - [x] Subtask 1.1: 阅读本 story 全文 + `_bmad-output/planning-artifacts/epics.md` §Story 20.3（行 2828-2851）确认 AR18 数量约束 + 单测 / 集成测试覆盖钦定
  - [x] Subtask 1.2: 阅读 `_bmad-output/planning-artifacts/epics.md` AR18（行 183-189）确认 cosmetic 数量约束 + URL 字段约束
  - [x] Subtask 1.3: 阅读 17.3 已 done 的姊妹 story `_bmad-output/implementation-artifacts/17-3-emoji_configs-seed.md`，参考其 seed migration + 集成测试编辑模式（强制覆盖 up + narrow DELETE down + SeedContent + SeedIdempotent / SeedForceOverwrite case 设计）
  - [x] Subtask 1.4: 阅读 `docs/lessons/2026-05-14-0010-final-decision-up-force-overwrite-down-narrow-delete.md`（17-3 r3 最终决断）+ `docs/lessons/2026-05-14-down-must-undo-up-invariant-over-admin-data.md`（17-3 r2）确认 ON DUPLICATE KEY UPDATE + narrow DELETE 决策根因
  - [x] Subtask 1.5: 阅读 `server/migrations/0010_seed_emoji_configs.up.sql` + `0010_seed_emoji_configs.down.sql`（17.3 落地最终版本）确认顶部注释 / 装扮清单 / 字段约束 / 最终决策段 / 范围红线四段式模板
  - [x] Subtask 1.6: 阅读 `server/migrations/0011_init_cosmetic_items.up.sql`（20.2 落地）确认 cosmetic_items 表 schema 11 字段顺序 + UNIQUE KEY uk_code 真实存在 + DEFAULT 值
  - [x] Subtask 1.7: 阅读 `server/internal/repo/mysql/cosmetic_item_repo.go`（20.2 落地）确认 `CosmeticItem` GORM struct 字段已就绪（本 story **不**动该文件，仅确认）
  - [x] Subtask 1.8: 阅读 `server/internal/infra/migrate/migrate_integration_test.go`（20.2 落地版本）确认 `TestMigrateIntegration_StatusAfterUp` 版本号断言位置（v != 11）+ `TestMigrateIntegration_CosmeticItems_Schema` / `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` 行号 + `TestMigrateIntegration_EmojiConfigs_SeedContent` / `SeedIdempotent` 实装模板
  - [x] Subtask 1.9: 阅读 V1 §7.2 reward 字段表（行 1060-1065）确认 `reward.name / slot / rarity / assetUrl / iconUrl` 字段长度 + 枚举约束
  - [x] Subtask 1.10: 阅读数据库设计 §6.8 / §6.9（行 841-862）确认 slot / rarity 枚举值
- [x] Task 2: 落地 0012_seed_cosmetic_items.up.sql（AC: #1）
  - [x] Subtask 2.1: 新建 `server/migrations/0012_seed_cosmetic_items.up.sql`
  - [x] Subtask 2.2: 写顶部注释（"对齐 §Story 20.3 + AR18 + V1 §7.2 + 数据库设计 §5.8 / §6.8 / §6.9" + 装扮清单 + 字段约束 + 最终决策段（沿用 17-3 r3 lesson）+ 范围红线四段式，按 AC1 模板）
  - [x] Subtask 2.3: 写 `INSERT INTO cosmetic_items (code, name, slot, rarity, asset_url, icon_url, drop_weight, is_enabled) VALUES (...) ON DUPLICATE KEY UPDATE name=VALUES(name), slot=VALUES(slot), rarity=VALUES(rarity), asset_url=VALUES(asset_url), icon_url=VALUES(icon_url), drop_weight=VALUES(drop_weight), is_enabled=VALUES(is_enabled);` 含 15 行：8 common + 4 rare + 2 epic + 1 legendary，common 覆盖 6 个不同 slot（按 AC1 钦定）
- [x] Task 3: 落地 0012_seed_cosmetic_items.down.sql（AC: #2）
  - [x] Subtask 3.1: 新建 `server/migrations/0012_seed_cosmetic_items.down.sql`
  - [x] Subtask 3.2: 写顶部注释（按 AC2 模板，"最终决策"段沿用 17-3 r3 lesson + "强约定" 段 + "down 实际执行场景" 段）+ `DELETE FROM cosmetic_items WHERE code IN ('hat_yellow', 'hat_red', ..., 'body_armor');`（15 个 code 与 up.sql 1:1 对齐）
- [x] Task 4: 修改 migrate_integration_test.go（AC: #3）
  - [x] Subtask 4.1: 修改 `TestMigrateIntegration_StatusAfterUp` 版本号 `v != 11` → `v != 12` + err msg `want 11` → `want 12`（按 AC3.1 钦定）+ docstring 注释追加 Story 20.3 扩展行
  - [x] Subtask 4.2: 修改 testfile 顶部注释追加 Story 20.3 扩展段（按 AC3.2 钦定）
  - [x] Subtask 4.3: 在 `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` 函数体顶部追加注释说明（按 AC3.6 钦定 —— 仅注释级别改动，**不**改实际 SQL 字面量）
  - [x] Subtask 4.4: 新增 `TestMigrateIntegration_CosmeticItems_SeedContent` case（按 AC3.4 钦定 + 参考 `TestMigrateIntegration_EmojiConfigs_SeedContent` 实装模板：起容器 / migrate up / sql.Open / 跑 SELECT 全表 + SELECT GROUP BY rarity + SELECT DISTINCT slot WHERE rarity=1 + SELECT GROUP BY rarity MIN/MAX(drop_weight) / 累积 t.Errorf）
  - [x] Subtask 4.5: 新增 `TestMigrateIntegration_CosmeticItems_SeedForceOverwrite` case（按 AC3.5 钦定 + 参考 `TestMigrateIntegration_EmojiConfigs_SeedIdempotent` 实装模板：起容器 / Up → COUNT=15 / DELETE seed → INSERT admin-flavored 15 行 / UPDATE schema_migrations SET version = 11 / Up 重跑 / 断言 COUNT=15 + 抽样 3 ~ 5 行字段值被覆盖回 seed 钦定值）
- [x] Task 5: 验证 + 提交（AC: #4）
  - [x] Subtask 5.1: 跑 `bash scripts/build.sh --test` 全绿（vet + 全 unit 测试通过；本 story 不新增任何 .go 业务文件，但既有单测无回归）
  - [x] Subtask 5.2: 跑 `go vet -tags=integration ./...` 全绿（含本 story 新增 2 case + 修改的 UniqueCode_Rejected 注释 + StatusAfterUp 版本号断言）
  - [x] Subtask 5.3: 跑 `bash scripts/build.sh --integration` 全绿（含 4 个 CosmeticItems case：Schema / UniqueCode_Rejected / SeedContent / SeedForceOverwrite + StatusAfterUp v=12 + 既有 EmojiConfigs / RoomMembers 等 case 不回归）；如本机 Docker 不可用 → 集成测试 `t.Skipf` 优雅跳过，与 17.3 / 20.2 同模式
  - [x] Subtask 5.4: git diff 范围检查 —— 仅本 story 钦定 5 个文件（见 File List）
  - [x] Subtask 5.5: seed 内容跨文档手动一致性检查（15 个 code 字面量 + AR18 数量 + common 覆盖 4 slot + asset_url / icon_url 非空 + drop_weight 递减 + slot / rarity 枚举值，按 AC4.3 钦定）
  - [x] Subtask 5.6: ON DUPLICATE KEY UPDATE 语法正确性手动检查（按 AC4.4 钦定 —— 覆盖 7 字段不覆盖 id / created_at）
  - [x] Subtask 5.7: down narrow DELETE 语法正确性手动检查（按 AC4.5 钦定 —— `DELETE FROM cosmetic_items WHERE code IN (15 codes)` 不是 TRUNCATE / 全表删）
  - [x] Subtask 5.8: 在 sprint-status.yaml 把本 story 状态从 in-progress 改为 review
  - [ ] Subtask 5.9: 由 code-review 检出后状态切 done + 在本 story 文件 + sprint-status.yaml 状态行追加 commit hash

## Dev Notes

### Build & Test 规范（项目级 CLAUDE.md 钦定）

- 写完 / 改完 Go 代码后必跑 `bash scripts/build.sh --test`（vet + 单测，**默认 build tag**，集成测试不跑）
- 集成测试 dockertest 必须用 `bash scripts/build.sh --integration`（带 `-tags=integration` build tag）
- 脚本契约来源：`_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.5 + `_bmad-output/implementation-artifacts/1-7-重做-scripts-build-sh.md`

### Migration 文件命名 / 编号规则（ADR-0003 + Story 4.3 钦定）

- 文件命名：`{N:04d}_{name}.up.sql` / `{N:04d}_{name}.down.sql`（4 位编号 + 下划线 + 小写下划线名称）
- 编号顺序：0001 ~ 0011 已被 4.3 / 7.2 / 10.3 / 11.2 / 17.2 / 17.3 / 20.2 占用（users / user_auth_bindings / pets / user_step_accounts / user_chests / user_step_sync_logs / rooms / room_members / emoji_configs schema / emoji_configs seed / cosmetic_items schema）；**本 story 占用 0012**（第二个 seed migration —— 与 0010 同性质，沿用相同命名规则）；按 epics.md §Story 20.3 文案 `migrations/0012_seed_cosmetic_items.sql` 钦定（注：本 story 拆为 .up.sql / .down.sql 双向）
- **seed migration 也走纯 SQL 文件**（**不**用 Go 代码 seed loader）：与 ADR-0003 §3.2 钦定一致；dev / test / staging / prod 同一份数据来源；与 17.3 落地的 emoji_configs seed 同模式

### r0 锁定的 ON DUPLICATE KEY UPDATE vs INSERT IGNORE 选型（**直接复用 17-3 r3 最终决断**）

| 路径 | 适用场景 | 风险 |
|---|---|---|
| `INSERT INTO ...` | UNIQUE 不冲突 | 冲突时抛 1062 错误，重跑失败 |
| `INSERT IGNORE INTO ...` | "seed 是初始默认值不覆盖 admin 修改" | **风险**：admin 预存的"坏行"（is_enabled=0 / asset_url='' / drop_weight 漂移）会幸存 → AR18 数量约束 100% 强保证无法保证 |
| `INSERT INTO ... ON DUPLICATE KEY UPDATE ...` ✓ | **本 story 选用** | 冲突时强制覆盖钦定字段；**Story 20.6 加权抽取 / 21.4 奖励弹窗 / 19.1 E2E "抽奖多样性" 钦定的"AR18 数量 / URL 非空 invariant" 100% 强保证**；admin 在 0012 owned 15 个 code 上的 customization 会被覆盖（与 17-3 r3 同模式）；admin / 运维必须遵守"不在 0012 owned codes 上做 customization"约定 |
| `REPLACE INTO ...` | 等价于先 DELETE 再 INSERT | 会重新分配 id；虽 cosmetic_items.id 当前没被外键引用，但 REPLACE 会让 chest_open_logs.reward_cosmetic_item_id 历史日志和当前 cosmetic_items.id 断开 reference 语义；不可逆性强 |

**选 ON DUPLICATE KEY UPDATE 的理由**：
1. 17-3 r3 lesson `docs/lessons/2026-05-14-0010-final-decision-up-force-overwrite-down-narrow-delete.md` 钦定 "0010 owns 4 codes / up 强制覆盖 / down narrow DELETE" 模式；本 story 直接复用同决策（**不**再走 r1 / r2 / r3 三轮反复 review）
2. Story 20.6 加权抽取 + 21.4 奖励弹窗 + 19.1 节点 7 demo E2E "验证场景 8 抽奖多样性" 都依赖 "15 个 enabled cosmetic + AR18 数量约束 + URL 非空 invariant"；这个 invariant 必须 100% 强保证，**不能**依赖 admin 自律
3. epics.md §Story 20.3 行 2845 文案写的是 "INSERT IGNORE 防重复"，但这是 epic 起草阶段的初稿（在 17.3 r3 决断之前）；本 story r0 决策升级为 ON DUPLICATE KEY UPDATE 与 17-3 r3 lesson 对齐，**不**反向修改 epics.md 文案（epics.md 是契约**输入**，本 story 落地的 0012 SQL 是 invariant 的真相源；契约层面"防重复 INSERT 引发冲突"语义两者都覆盖；语义升级由本 story Dev Notes + lesson 链接说明）

### Placeholder URL 选型（AR18 / V1 §7.2）

- AR18 钦定 placeholder URL 示例：`https://placehold.co/128x128?text=Hat`
- V1 §7.2 reward.assetUrl 钦定 1 ≤ length ≤ 255；MVP 阶段允许 placeholder
- V1 §7.2 reward.iconUrl 钦定 1 ≤ length ≤ 255；同样允许 placeholder
- **本 story 选 `https://placehold.co/128x128?text={EnglishName}` (asset_url) 和 `https://placehold.co/64x64?text={EnglishName}` (icon_url)**：
  - 128x128 是装扮主图尺寸（用于角色穿戴渲染 + 奖励弹窗大图）
  - 64x64 是图标尺寸（用于仓库 grid + 奖励弹窗小预览）
  - `text=Hat-Yellow` 等英文名让 placeholder 图片含视觉标签（不是全空灰块）
  - placehold.co 是公共可访问服务，节点 7 demo 阶段足够（真实美术资产由 Epic 20 retrospective tech-debt 登记 + 后续 epic 切换；与 17.3 emoji seed 同 placeholder 服务）

### drop_weight 分布数学

epics.md §Story 20.3 行 2844 钦定 `common=100 / rare=20 / epic=4 / legendary=1`，比例 `100:20:4:1 = 25:5:1:0.25`。

按本 story 15 件 seed 计算总权重：
- common 8 件 × 100 = 800
- rare 4 件 × 20 = 80
- epic 2 件 × 4 = 8
- legendary 1 件 × 1 = 1
- **总权重 = 889**

各品质命中概率：
- common ~ 800/889 ≈ 90.0%
- rare ~ 80/889 ≈ 9.0%
- epic ~ 8/889 ≈ 0.9%
- legendary ~ 1/889 ≈ 0.1%

连开 20 次的期望分布：
- common ~ 18 次（不同 cosmetic 数量期望 ~ 5-6 种，覆盖 8 件 common 的概率 > 90%）
- rare ~ 1.8 次（~ 1-2 种）
- epic ~ 0.18 次（~ 偶尔出现）
- legendary ~ 0.02 次（~ 几乎不出现）

→ "至少 3-4 种不同 cosmetic"（Epic 19.1 节点 7 demo E2E "验证场景 8 抽奖多样性"钦定）的概率接近 1.0（即使只从 8 件 common 里抽 18 次，覆盖 5+ 种的概率也 ≈ 99.5%）。

**风险**：如果 dev 阶段把 common drop_weight 改成 10000（远超 rare 20），rare/epic/legendary 几乎抽不到，连开 20 次只看到 common → 违反 E2E "至少 3-4 种" 钦定。**mitigation**：AC1 明确钦定 common=100 / rare=20 / epic=4 / legendary=1；AC3.4 SeedContent case 校验 "common 的 MIN(drop_weight) > rare 的 MAX(drop_weight)" 等递减关系；不允许漂移。

### 跨文档语义同步检查（seed 数据 / API 契约 / 数据库设计）

- 本 story 落地的 0012 SQL 数据必须严格符合 V1 §7.2 reward 字段约束（20.1 锚定）+ §5.8 DDL 约束（20.2 落地）+ §6.8 / §6.9 状态枚举
- 不允许在本 story 阶段对 V1 / §5.8 / §6.8 / §6.9 / AR18 做反向加严 / 放松
- 如发现 V1 §7.2 / §5.8 / §6.8 / §6.9 / AR18 与本 story 落地 seed 数据冲突（如 V1 §7.2 钦定 assetUrl length > 0 但本 story seed 写 ""）→ 优先修 0012 SQL 而非反向改契约

### 错误码不在本 story 范围

- §3 全局错误码表（4001 / 4002 / 3002 等）由 20.1 锚定 + 由 20.5 / 20.6 实装时引用；本 story **不**触发错误码定义 / 修改 / 引用（migration 层不返回 API 错误码）
- ON DUPLICATE KEY UPDATE 走 UPDATE 路径**不**触发任何错误码（与 INSERT 抛 1062 ER_DUP_ENTRY 不同；migration 层视为成功）

### admin 在 0012 owned codes 上的 customization 无法存活（与 17-3 r3 同决策延续）

**强约定（admin / 运维必读）**：
1. 0012 owned 的 15 个 code（hat_yellow / hat_red / gloves_white / gloves_brown / glasses_round / neck_blue / back_bag / tail_ribbon / hat_chef / glasses_star / neck_scarf_star / body_tshirt / hat_crown / back_wings / body_armor）**由 0012 完全占用 / 强制覆盖**
2. admin / 运维 **禁止**手工 INSERT / UPDATE 这 15 个 code 的行（up 重跑会被覆盖；down 会被删除；任何 customization 都无法存活）
3. 需要新增装扮 → 通过**新 migration**（0013+）添加，不要在 cosmetic_items 表上做 admin 直插
4. **未来 Epic 29 Story 29.3 render_config seed 是 UPDATE 语义**（给已有 cosmetic 配置补 render_config 字段值），不是 INSERT；29.3 改的是 0011 schema 后 Epic 29 Story 29.2 加列后的字段；29.3 不冲突 0012 的 owned codes（同 code 不同字段）

### 跨 epic 依赖追溯

- **上游冻结**：
  - 数据库设计 §5.8 cosmetic_items 表 schema ← 20.2 落地
  - 数据库设计 §6.8 cosmetic_items.slot 枚举 + §6.9 cosmetic_items.rarity 枚举 ← 总体架构钦定
  - V1 §7.2 reward 字段表 ← 20.1 锚定（assetUrl / iconUrl 禁止 ""）
  - AR18 cosmetic_items ≥ 15 件 + AR18 数量约束 + URL 非空 ← 总体架构钦定
  - ADR-0003 migration 工具 + 编号规则 ← Story 4.3 落地
  - 17-3 r3 lesson "up 强制覆盖 + down narrow DELETE" ← Story 17.3 落地（**本 story r0 直接复用同决策**）
- **下游强依赖**（本 story done 后才能开工）：
  - Story 20.4（chest_open_logs migration，节点 7 顺位但不直接依赖 seed）
  - Story 20.5（GET /chest/current 接口，不直接依赖 seed）
  - Story 20.6（POST /chest/open 事务 + 加权抽取，**直接依赖** seed + drop_weight 分布）
  - Story 20.7（dev /dev/force-unlock-chest，不直接依赖 seed）
  - Story 20.8（dev /dev/grant-cosmetic-batch，**直接依赖** AR18 数量约束）
  - Story 20.9（Layer 2 集成测试 - 开箱事务全流程，**直接依赖** seed）
  - iOS Epic 21.1 ~ 21.5（首页宝箱 + GET /chest/current + POST /chest/open + 奖励弹窗 + 开箱前同步步数；21.4 popup AsyncImage **直接依赖** seed 的 asset_url 非空）
  - Epic 19.1 节点 7 demo E2E（**直接依赖** AR18 + drop_weight 分布）
  - Epic 23 Story 23.3 GET /cosmetics/catalog（**直接依赖** seed 数据）
  - Epic 29 Story 29.3 render_config seed（**直接依赖** seed 的 15 codes 字面量清单做 UPDATE）
  - Epic 32 / 33 合成事务（**直接依赖** common ≥ 8 让合成 10 件凑得齐）

### Git Intelligence（最近 5 commits 模式参考）

- `ebe8762 chore(lessons): backfill 3cd2ef4 commit hash for SDK/runtime mismatch lesson` —— 文档回填，与本 story 无业务关联
- `3cd2ef4 docs(lessons): 沉淀 epic-18 retro A1 修复 — Xcode SDK/sim-runtime 版本错位的根因诊断` —— iOS 端 lesson，与本 story 无业务关联
- `6a04d9f chore(epic-18): 收官 Epic 18 retrospective + sprint-status 标记 retrospective done` —— Epic 18 收官；可参考其 sprint-status 状态行流转格式
- `48acf83 docs(lessons): 补充 SwiftUI PreferenceKey merge vs replace & owner-side expire lesson（18-4 r1）` —— iOS 端 lesson，与本 story 无业务关联
- `e747017 chore(story-18-4): 收官 Story 18.4 + 归档 story 文件` —— Story 收官；可参考其 story-done commit 格式
- **本 story 最相关的 server-side migration 落地 commit**：
  - Story 17.3 `0010_seed_emoji_configs.up.sql` + `0010_seed_emoji_configs.down.sql` + r3 review 最终决断 lesson（**本 story r0 直接复用同决策**，是本 story 顶部注释模板 / 字段约束模板 / 最终决策段 / 范围红线四段式的直接参考来源）
  - Story 20.2 `0011_init_cosmetic_items.up.sql` + `0011_init_cosmetic_items.down.sql` + `cosmetic_item_repo.go`（本 story seed 写入的目标表 + GORM struct，本 story 不动该文件，仅复用）

### 测试 / 验证

- **单元测试**：本 story 不新建任何 Go .go 业务代码（纯 SQL seed + 集成 test 文件改动）→ 无 sqlmock-based 单测需要新建；20.2 落地的 `cosmetic_item_repo.go` 没有 repo 方法（20.6 才有），所以 `mysql/` 目录下其他 repo 单测不受 0012 seed 影响
- **集成测试**（dockertest）：本 story 新增 2 case + 修改 1 case 顶部注释（与 17.3 修复 17.2 UniqueCode_Rejected 模式相同，但 20.2 已经吸取教训用了测试专用 code `test_unique_cosmetic_a`，所以本 story **只需追加注释解释**，不需要改实际 SQL 字面量）+ 修改 1 case 版本号（StatusAfterUp v=12）；用 `bash scripts/build.sh --integration` 跑（带 `-tags=integration`）
- **下游验证**：本 story done 后由 Story 20.6 实装时的 dockertest 集成测试（curl POST /chest/open → 验证 response.reward 字段与 §7.2 字段表对齐 + 加权抽取分布合理 + idempotencyKey 重复调用走 cache 路径）+ Story 20.9 Layer 2 完整事务集成测试 + Story 23.3 实装时的 dockertest 集成测试（curl GET /cosmetics/catalog → 验证 response.items 长度 ≥ 15）+ Epic 19.1 节点 7 demo E2E（连开 20 次 → 弹窗至少 3-4 种不同 cosmetic）做真实串联验证

### 范围红线 + 风险

- **红线**：本 story **不**修改任何 service / handler / repo write / read 方法；**不**修改 V1 接口契约 / 数据库设计文档 / AR18 / ADR-0003；**不**修改 0001 ~ 0011 既有 migration；**不**修改 20.2 落地的 cosmetic_item_repo.go；**不**预实装 `CosmeticItemRepo` interface / 方法
- **红线**：本 story **不**实装任何业务 Go 代码（20.5 / 20.6 / 20.7 / 20.8 / 23.3 owner）
- **红线**：本 story **必须** seed 严格 15 件装扮（common 8 / rare 4 / epic 2 / legendary 1），**不**多 seed 也**不**少 seed —— 多 seed 会让 SeedContent case "rows >= 15" 仍 pass 但后续 epic baseline 漂移；少 seed 会违反 AR18 "common ≥ 8 / rare ≥ 4 / epic ≥ 2 / legendary ≥ 1" 约束
- **红线**：本 story **不**包含 `render_config` 字段值（节点 10 / Epic 29 Story 29.2 / 29.3 owner）
- **风险**：drop_weight 分布漂移（如把 common 写 1 / rare 写 100，让 common 反而最稀有）→ Story 20.6 加权抽取分布错误 → Epic 19.1 节点 7 demo E2E "至少 3-4 种不同 cosmetic" 挂掉 → mitigation：AC1 明确钦定 100/20/4/1；AC3.4 SeedContent case 校验 "common 的 MIN(drop_weight) > rare 的 MAX(drop_weight)" 等递减关系
- **风险**：common 只覆盖 3 个 slot（如全部 hat / gloves / glasses，少 neck）→ 违反 AR18 "common 至少覆盖 4 个不同槽位" → mitigation：AC1 明确钦定 common 8 件覆盖 6 个 slot（{1,2,3,4,5,7}）；AC3.4 SeedContent case 校验 "SELECT DISTINCT slot WHERE rarity=1 → COUNT >= 4"
- **风险**：asset_url 或 icon_url 不小心写 `''` 空字符串（DDL `DEFAULT ''` 允许）→ V1 §7.2 + AR18 钦定"开箱奖励必须有 asset / icon" → mitigation：AC1 显式钦定每个 asset_url / icon_url 非空 placeholder URL + AC3.4 SeedContent case 校验 `len(asset_url) > 0` + `len(icon_url) > 0`
- **风险**：is_enabled 不小心写 0 → V1 §8.1 GET /cosmetics/catalog 服务端逻辑 `WHERE is_enabled = 1` 过滤，下游 GET /cosmetics/catalog 不会返回这个 cosmetic → 20.6 加权抽取也跳过 → 仓库页空一格 / 奖励弹窗少一种可能 → mitigation：AC1 显式钦定全部 1 + AC3.4 SeedContent case 校验 `is_enabled == 1`
- **风险**：slot 写 8（错值不在 §6.8 枚举 {1,2,3,4,5,6,7,99} 内）→ Client 端 UI 展示无法分类 → mitigation：AC1 明确钦定 slot ∈ {1,2,3,4,5,7}；AC3.4 SeedContent case 校验 `slot ∈ {1,2,3,4,5,6,7,99}`
- **风险**：rarity 写 5（错值不在 §6.9 枚举 {1,2,3,4} 内）→ Client 端 UI 边框 / 颜色无法渲染 → mitigation：AC1 明确钦定 rarity ∈ {1,2,3,4}；AC3.4 SeedContent case 校验 `rarity ∈ {1,2,3,4}`
- **风险**：误写 INSERT IGNORE 取代 ON DUPLICATE KEY UPDATE → 让 admin 预存的"坏行"幸存 → AR18 invariant 无法 100% 强保证（与 17-3 r3 lesson 直接对立） → mitigation：AC1 明确钦定 ON DUPLICATE KEY UPDATE 覆盖 7 字段；AC3.5 SeedForceOverwrite case 真实验证强制覆盖语义（如果误写 INSERT IGNORE 该 case 会失败）
- **风险**：down.sql 用 TRUNCATE / 全表 DELETE / `DELETE WHERE 1=1` → 误删未来 0013+ 加的新装扮 / admin 手工 INSERT 的非 0012 数据 → mitigation：AC2 显式钦定 narrow DELETE WHERE code IN (15 codes)
- **风险**：down.sql 15 个 code 字面量与 up.sql 漂移（如 up 写 hat_yellow，down 写 hat_yellowo）→ down 漏删 → schema_migrations 回退到 v=11 但 cosmetic_items 表里仍有 hat_yellow → mitigation：AC2 钦定 15 个 code 与 up.sql 1:1 对齐；AC4.5 手动检查
- **风险**：placeholder URL `https://placehold.co/128x128?text=Hat-Yellow` 服务未来不可用（placehold.co 域名挂掉 / 限流）→ 21.4 AsyncImage 加载失败 → demo 时奖励弹窗降级 placeholder → mitigation：本 story 阶段接受该风险（与 AR18 钦定一致 —— MVP 允许 placeholder URL）；真实美术资产切换由 Epic 20 retrospective tech-debt 登记 + 后续 epic 处理
- **风险**：dockertest 集成测试在 Windows 本地跑可能因 Docker Desktop 未启动失败 → 与 17.3 / 20.2 / 11.2 同情况，由 dev-story 阶段确保 Docker Desktop 启动后跑；如本机 Docker 不可用 → 集成测试 `t.Skipf` 优雅跳过（migrate_integration_test.go 顶部注释钦定的降级路径），不阻塞 review；code-review 阶段在 fresh 环境 retry 验证

### Project Structure Notes

- 本 story 唯一编辑文件（绝对路径）：
  - `C:/fork/cat/server/migrations/0012_seed_cosmetic_items.up.sql`（新建）
  - `C:/fork/cat/server/migrations/0012_seed_cosmetic_items.down.sql`（新建）
  - `C:/fork/cat/server/internal/infra/migrate/migrate_integration_test.go`（修改：版本号 11 → 12 + 顶部注释 + UniqueCode_Rejected 顶部追加 seed 字面量隔离注释 + 新增 SeedContent + 新增 SeedForceOverwrite 共 4 处改动）
  - `C:/fork/cat/_bmad-output/implementation-artifacts/20-3-cosmetic_items-seed.md`（本 story 文件）
  - `C:/fork/cat/_bmad-output/implementation-artifacts/sprint-status.yaml`（状态行流转）
- 与 `docs/宠物互动App_Go项目结构与模块职责设计.md` §6 锚定的 `migrations/` 目录完全兼容（沿用既有目录规则，**不**新增子目录 / 模块）；seed migration 和 schema migration 同目录同命名规则（与 ADR-0003 一致 + 与 17.3 同模式）

### References

- [Source: docs/宠物互动App_数据库设计.md#5.8] cosmetic_items 表 schema（行 437-479；20.2 落地的 DDL 真相源）
- [Source: docs/宠物互动App_数据库设计.md#6.8] cosmetic_items.slot 枚举（行 841-852；1=hat / 2=gloves / 3=glasses / 4=neck / 5=back / 6=body / 7=tail / 99=other）
- [Source: docs/宠物互动App_数据库设计.md#6.9] cosmetic_items.rarity 枚举（行 854-862；1=common / 2=rare / 3=epic / 4=legendary）
- [Source: docs/宠物互动App_数据库设计.md#7.1] 高优先级 UNIQUE 约束（`cosmetic_items` UNIQUE(code) —— 本 story ON DUPLICATE KEY UPDATE 兜底语义依赖）
- [Source: docs/宠物互动App_V1接口设计.md#7.2] POST /api/v1/chest/open reward 字段表（行 1060-1065；20.1 锚定；本 story seed 的 name / slot / rarity / asset_url / icon_url 字段长度 / 枚举值必须符合此契约）
- [Source: _bmad-output/planning-artifacts/epics.md#AR18] cosmetic_items 数量约束 + URL 字段约束（行 183-189）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 20.3] AC 钦定（行 2828-2851）：15+ 行 + AR18 数量 + URL 非空 + drop_weight 递减 + INSERT IGNORE → 本 story r0 升级为 ON DUPLICATE KEY UPDATE
- [Source: _bmad-output/planning-artifacts/epics.md#Story 19.1] 节点 7 demo E2E "验证场景 8 抽奖多样性"钦定（行 3165；连开 20 次 → 至少 3-4 种不同 cosmetic，验证 AR18 数量约束生效）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 29.3] render_config seed UPDATE 语义（行 3844-3855；依赖本 story 落地的 15 个 code 字面量）
- [Source: _bmad-output/implementation-artifacts/20-1-接口契约最终化.md] Story 20.1 上游契约（已 done；V1 §7.2 reward 字段长度 / 枚举锚定）
- [Source: _bmad-output/implementation-artifacts/20-2-cosmetic_items-migration.md] Story 20.2 已 done 姊妹 story（cosmetic_items 表 schema + UNIQUE uk_code + CosmeticItem GORM struct + dockertest CosmeticItems_Schema / CosmeticItems_UniqueCode_Rejected 落地参考）
- [Source: _bmad-output/implementation-artifacts/17-3-emoji_configs-seed.md] Story 17.3 已 done 姊妹 story（emoji_configs seed + ON DUPLICATE KEY UPDATE 强制覆盖 + narrow DELETE down + SeedContent / SeedIdempotent dockertest case 落地参考；**本 story r0 直接复用同模式**）
- [Source: _bmad-output/implementation-artifacts/decisions/0003-orm-stack.md] ADR-0003 ORM / migration 工具栈（golang-migrate v4.18.1 + GORM v1.25.12；migration 编号规则 + .up.sql / .down.sql 双向规范；seed migration 也走纯 SQL 文件路径）
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md] ADR-0001 测试栈（dockertest + build tag `integration`；`bash scripts/build.sh --integration` 跑集成测试）
- [Source: docs/lessons/2026-05-14-0010-final-decision-up-force-overwrite-down-narrow-delete.md] 17-3 r3 最终决断 lesson（**本 story r0 直接复用同决策**：up 强制覆盖 + down narrow DELETE + admin 在 owned codes 上 customization 无法存活）
- [Source: docs/lessons/2026-05-14-down-must-undo-up-invariant-over-admin-data.md] 17-3 r2 lesson（down 必须真正 undo up；migration framework invariant 优先 admin 数据保留）
- [Source: server/migrations/0011_init_cosmetic_items.up.sql] 20.2 落地的 cosmetic_items 表 DDL（本 story seed 的 INSERT 目标表）
- [Source: server/migrations/0010_seed_emoji_configs.up.sql + 0010_seed_emoji_configs.down.sql] 17.3 落地的 seed migration 模板（顶部注释 + 字段约束 + 最终决策段 + 范围红线四段式参考）
- [Source: server/internal/repo/mysql/cosmetic_item_repo.go] 20.2 落地的 CosmeticItem GORM struct（本 story **不**修改，仅作参考；下游 20.6 / 23.x 才用）
- [Source: server/internal/infra/migrate/migrate_integration_test.go] 17.3 / 20.2 落地的 dockertest 集成测试（含 EmojiConfigs_SeedContent / SeedIdempotent + CosmeticItems_Schema / UniqueCode_Rejected case；本 story 沿用同模式新增 2 case + 修改 1 case 注释 + 1 case 版本号）
- [Source: _bmad-output/implementation-artifacts/sprint-status.yaml] Epic 20 状态 in-progress；本 story 状态行 + last_updated 由 create-story / dev-story / code-review 流程逐步推进

## Dev Agent Record

### Agent Model Used

claude-opus-4-7 (1M context)

### Debug Log References

- `bash scripts/build.sh --test`：vet + 单测全绿（仅 SQL 文件 + integration tag 测试改动，对默认 build 单测无回归）
- `cd /c/fork/cat/server && go vet -tags=integration ./...`：全绿（新增 2 case + 修改的 UniqueCode_Rejected 注释 + StatusAfterUp 版本号断言均编译通过）
- `bash scripts/build.sh --integration`：编译通过 + 集成测试运行；本机 Docker daemon 未启动，所有 dockertest case `t.Skipf` 优雅跳过（与 17.3 / 20.2 同模式；code-review 阶段在 Docker 可用环境 retry 验证 SeedContent / SeedForceOverwrite 实际 SQL 行为）

### Completion Notes List

- AC1 落地：`server/migrations/0012_seed_cosmetic_items.up.sql` 新建，15 行 `INSERT ... ON DUPLICATE KEY UPDATE`（common 8 / rare 4 / epic 2 / legendary 1；common 覆盖 slot ∈ {1,2,3,4,5,7} = 6 个不同槽位；drop_weight 100 / 20 / 4 / 1 按品质递减；每行 asset_url / icon_url 均为 placehold.co 非空 placeholder URL；ON DUPLICATE KEY UPDATE 覆盖 7 字段 name / slot / rarity / asset_url / icon_url / drop_weight / is_enabled，**不**覆盖 id / created_at）。顶部注释含装扮清单 + 字段约束 + r0 最终决策段（沿用 17-3 r3 lesson）+ 范围红线四段式。
- AC2 落地：`server/migrations/0012_seed_cosmetic_items.down.sql` 新建，仅 1 条 `DELETE FROM cosmetic_items WHERE code IN (15 codes)`（narrow DELETE）。15 个 code 字面量与 up.sql 1:1 对齐。顶部注释含 r0 最终决策段 + 强约定段 + down 实际执行场景两路径段。
- AC3 落地：`server/internal/infra/migrate/migrate_integration_test.go` 修改 4 处 ——
  - 顶部 testfile docstring 追加 Story 20.3 扩展段（15 行 seed + AR18 数量约束 + StatusAfterUp v=11 改 v=12）
  - `TestMigrateIntegration_StatusAfterUp` docstring 追加 Story 20.3 扩展行 + 内部 `if v != 11` 改 `if v != 12` + err msg `want 11` 改 `want 12`
  - `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` 函数体顶部追加 Story 20.3 注释段，明示 `test_unique_cosmetic_a` 与 0012 owned 15 codes 隔离的解耦机制（SQL 字面量未改动）
  - 末尾新增 `TestMigrateIntegration_CosmeticItems_SeedContent`（5 类断言：≥15 行 + AR18 各 rarity 数量 + common ≥ 4 不同 slot + 每行 7 字段约束 + drop_weight 按品质 MIN / MAX 递减分布）
  - 末尾新增 `TestMigrateIntegration_CosmeticItems_SeedForceOverwrite`（覆盖 5 步路径：Up → DELETE + INSERT 15 行 admin-flavored 数据 → UPDATE schema_migrations 回退 v=11 → Up 重跑 → 断言行数仍 15 + 15 行各 7 字段被覆盖回 seed 钦定值）
- AC4 落地：build.sh --test 全绿、go vet -tags=integration 全绿、build.sh --integration 编译通过（Docker 不可用时跳过；本机 Windows 环境 daemon 未启动）；File List 仅 5 个钦定文件；ON DUPLICATE KEY UPDATE 覆盖 7 字段不覆盖 id / created_at；down narrow DELETE 仅 1 条 SQL，15 个 code 字面量与 up.sql 严格对齐。
- 红线遵循：未修改任何 service / handler / repo write / read 方法；未修改 V1 接口契约 / 数据库设计文档 / AR18 / ADR-0003；未修改 0001 ~ 0011 既有 migration；未修改 `cosmetic_item_repo.go`；未预实装任何 CosmeticItemRepo 方法；未引入 render_config 字段值；未引入 admin / dev 端点。

### File List

新建：
- `server/migrations/0012_seed_cosmetic_items.up.sql`
- `server/migrations/0012_seed_cosmetic_items.down.sql`

修改：
- `server/internal/infra/migrate/migrate_integration_test.go`（顶部 testfile docstring 追加 Story 20.3 扩展段 + `TestMigrateIntegration_StatusAfterUp` 版本号 11→12 + `TestMigrateIntegration_CosmeticItems_UniqueCode_Rejected` 顶部追加 seed 字面量隔离注释 + 末尾新增 `TestMigrateIntegration_CosmeticItems_SeedContent` + 新增 `TestMigrateIntegration_CosmeticItems_SeedForceOverwrite`）
- `_bmad-output/implementation-artifacts/20-3-cosmetic_items-seed.md`（Status 流转 + Tasks/Subtasks 勾选 + Dev Agent Record 填充 + File List + Change Log）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（`20-3-cosmetic_items-seed` 状态行 ready-for-dev → in-progress → review + `last_updated`）

### Change Log

| 日期 | 操作 | Story 状态 | 备注 |
|---|---|---|---|
| 2026-05-14 | create-story | backlog → ready-for-dev | 由 epic-loop / bmad-create-story workflow 自动生成 |
| 2026-05-14 | dev-story | ready-for-dev → in-progress → review | 落地 0012_seed_cosmetic_items.up/down.sql + migrate_integration_test.go 扩展（SeedContent + SeedForceOverwrite + StatusAfterUp v=12 + UniqueCode_Rejected 解耦注释）；build.sh --test 全绿；build.sh --integration 编译通过、Docker 不可用时跳过 |
