# Story 7.2: user_step_sync_logs migration

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want 在节点 3 server 实装第一行业务代码（Story 7.3 POST /steps/sync 的 service / repo / handler）之前，把 `user_step_sync_logs` 表的 migration 文件（编号 0006，up + down 双向）按 `docs/宠物互动App_数据库设计.md` §5.5 + §6.5 + §6.6 + §7.2 钦定的 schema **逐字段**落地到 `server/migrations/`，并接入 Story 4.3 已建立的 `internal/infra/migrate` Go API + `catserver migrate {up|down|status}` CLI + dockertest 集成测试管线，
so that Story 7.3 的 service 在 `txManager.WithTx(ctx, fn)` 内可以直接 INSERT `user_step_sync_logs`、Story 7.4 / 7.5 / iOS 8.5 的下游链路无需关心 schema 漂移、Epic 7 集成测试场景跑 dockertest 起容器后第一步 `migrate up` 拿到这张空表就能跑差值计算事务。

## 故事定位（Epic 7 第二条 = 节点 3 第一张表落地；上承 7.1 契约冻结，下启 7.3 service 实装）

- **Epic 7 进度**：7.1（契约最终化，**done** —— V1 文档 §1 / §6.1 / §6.2 字段表 + 错误码 + 防作弊阈值 + syncDate 时区契约 + 节点 3 冻结声明完成）→ **7.2（本 story，user_step_sync_logs 表 migration 0006_up/down + 单测 ≥3 + dockertest 集成测试）** → 7.3（POST /steps/sync handler + service + 累计差值入账事务 + 防作弊阈值 5000/50000 实施）→ 7.4（GET /steps/account handler）→ 7.5（dev 端点 POST /dev/grant-steps，写 sync_log 用 source=2）。**本 story 是 7.3 / 7.4 / 7.5 的强前置**：
  - **7.3 强依赖**：service 层差值计算逻辑 `查 user_step_sync_logs WHERE user_id=? AND sync_date=? 取最近一条`（数据库设计 §8.2 + epics.md Story 7.3 行 1379）必须命中本 story 建好的索引 `idx_user_date (user_id, sync_date)`；事务内 `INSERT user_step_sync_logs` 必须命中本 story 钉死的 8 列字段（`id` / `user_id` / `sync_date` / `client_total_steps` / `accepted_delta_steps` / `motion_state` / `source` / `client_ts` / `created_at`）；表 / 字段 / 索引任一与 §5.5 不符 → 7.3 单测 / 集成测试全红 → 节点 3 demo 阻塞
  - **7.4 间接依赖**：7.4 GET /steps/account 不读 `user_step_sync_logs`（只读 `user_step_accounts`），但**集成测试**会先 sync 几次再 GET account，sync 链路依赖本表
  - **7.5 强依赖**：dev grant-steps 接口写一条 `source=2 (admin_grant)` 的 sync_log（epics.md §Story 7.5 行 1434），命中本 story 的 `source` 列 + §6.6 枚举
  - **iOS 8.5 间接依赖**：iOS 端 `POST /steps/sync` 调用走 7.3 service，最终落到本表
- **epics.md AC 钦定**（`_bmad-output/planning-artifacts/epics.md` §Story 7.2 行 1348-1366）：
  - migration 文件路径 `migrations/0006_init_user_step_sync_logs.sql`（**注意**：epics.md 用的是"无 .up./.down. 后缀"的简写，与 Story 4.3 同情况；本 story 按 ADR-0003 §3.2 + golang-migrate v4 工具规范落地为 `0006_init_user_step_sync_logs.up.sql` + `0006_init_user_step_sync_logs.down.sql` 双向文件对，编号 / 表名一致，仅形式上多了 `.up`/`.down` 后缀，与 4.3 落地的 0001-0005 五张表风格一致）
  - 字段全集：`id`, `user_id`, `sync_date`, `client_total_steps`, `accepted_delta_steps`, `motion_state`, `source`, `client_ts`, `created_at`
  - 索引：`idx_user_date (user_id, sync_date)` + `idx_user_created_at (user_id, created_at)`
  - 含 down.sql
  - **单元测试覆盖**（≥3 case）：happy migrate up 后表存在 + 字段类型 + 索引符合 §5.5 / happy migrate down 后表删除 / edge 重复 migrate up 幂等不报错
  - **集成测试覆盖**（dockertest）：migrate up → SHOW CREATE TABLE 对比 §5.5 schema → migrate down
- **数据库设计 §5.5 钦定 DDL**（`docs/宠物互动App_数据库设计.md` 行 317-359）：

  ```sql
  CREATE TABLE user_step_sync_logs (
      id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
      user_id BIGINT UNSIGNED NOT NULL,
      sync_date DATE NOT NULL,
      client_total_steps BIGINT UNSIGNED NOT NULL,
      accepted_delta_steps INT NOT NULL DEFAULT 0,
      motion_state TINYINT NOT NULL DEFAULT 1,
      source TINYINT NOT NULL DEFAULT 1,
      client_ts BIGINT UNSIGNED NOT NULL DEFAULT 0,
      created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

      KEY idx_user_date (user_id, sync_date),
      KEY idx_user_created_at (user_id, created_at)
  ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
  ```

  本 story 的 0006_up.sql 必须**逐字符对齐**这段 DDL（除注释外），不擅自加列、不改类型、不动默认值、不动索引名 / 索引顺序。
- **§6.5 motion_state 枚举**（行 757-763）：`1 = stationary_or_unknown / 2 = walking / 3 = running`（V1 §6.1.3 钦定一致）
- **§6.6 source 枚举**（行 765-770）：`1 = healthkit（V1 §6.1 客户端正常上报） / 2 = admin_grant（Story 7.5 dev/运营手动发放）`
- **§7.2 索引建议**（行 866-868）：`user_step_sync_logs(user_id, sync_date)` + `user_step_sync_logs(user_id, created_at)` —— 与 §5.5 钦定的两条索引完全一致，本 story 不**新增**任何索引（哪怕实装时觉得"再加一个 covering index 也好"也不加，等 7.3 service 实装出来发现真有 query 模式漏覆盖再后置 tech debt 处理）
- **下游立即依赖**：
  - **Story 7.3 (POST /steps/sync handler + service + 差值入账事务)**：service 层 `query := repo.GetLatestByUserAndDate(ctx, userID, syncDate)` 命中 `idx_user_date`；`INSERT user_step_sync_logs(user_id, sync_date, client_total_steps, accepted_delta_steps, motion_state, source, client_ts) VALUES (...)` 必须命中本 story 钉死的 7 列业务字段（`id` / `created_at` 由 DB 自动生成）；事务用 4.2 已实装的 `txManager.WithTx(ctx, fn)` 在 fn 内调本表 INSERT。**字段名 / 类型任一漂移 → 7.3 单测 + 集成测试全红**
  - **Story 7.5 (dev 端点 POST /dev/grant-steps)**：写一条 `source=2 (admin_grant)` 的 sync_log（epics.md §Story 7.5 行 1434），命中本 story 的 `source` 列；其他字段处理（`accepted_delta_steps` 直接取 grant 的 steps 值 / `motion_state=1` / `client_ts=0`）属 7.5 service 范围
  - **Epic 7 集成测试**（节点 3 跨端 e2e by Epic 9 Story 9.1 / 节点 3 demo by 9.2）：dockertest 起容器 → migrate up（含本表）→ 运行多次 sync → INFORMATION_SCHEMA 验本表行数 / 字段值
- **范围红线**：本 story **只**做"建一张空表 + 验它能正确建出来"。**不**做：
  - **不**实装任何 repo（如 `step_sync_log_repo.go` —— 那是 Story 7.3 的范围）
  - **不**实装任何 service / handler
  - **不**写 GORM model struct / Codable struct
  - **不**改 V1 接口契约（7.1 已冻结）
  - **不**改任何 docs/*.md（数据库设计 §5.5 是契约**输入**，本 story 严格对齐；不**反向**修改）
  - **不**修改 0001-0005 任一既有 migration 文件（Story 4.3 已落地，节点 2 / 3 阶段冻结）
  - **不**修改 `internal/infra/migrate/` 包的 Go API 实装（4.3 已实装，本 story 只**新增**两个 SQL 文件，migrate Go API 自动 pickup）
  - **不**改 `cmd/server/main.go` / `internal/cli/migrate.go`（4.3 已实装的 `catserver migrate up/down/status` 子命令自动支持本表 —— `migrate up` 跑到 0006 时执行本 story 的 up.sql；不需要修改任何 Go 代码）
  - **不**接 Redis（Epic 10.2 才接）
  - **不**写 seed 数据（本表是日志表，永远没有 seed —— 数据由 7.3 service 在用户 sync 时插入，由 7.5 dev grant 时插入）
  - **不**给 SQL 加 FK / CHECK / TRIGGER（与 4.3 落地的 0001-0005 一致：业务层保证引用完整性）
  - **不**写"启动自动 migrate"逻辑（4.3 钉死：migrate 是显式运维操作，由 CI / 部署阶段单独 step 跑）

**本 story 不做**（明确范围红线）：

- 不实装 `internal/repo/mysql/step_sync_log_repo.go` 或任何 repo（Story 7.3 才做）
- 不实装 `internal/service/step_service.go` / `internal/app/http/handler/steps.go` / `internal/app/http/dto/steps.go`（Story 7.3 才做）
- 不写 GORM model `StepSyncLog struct`（Story 7.3 落地 repo 时定义）
- 不改 `docs/宠物互动App_*.md` 任一份（数据库设计 §5.5 / §6.5 / §6.6 / §7.2 是契约输入；本 story 严格对齐它，**不**修改）
- 不改 `docs/lessons/` 任一份（如 review 阶段产生新 lesson 由 fix-review workflow 处理）
- 不修改 0001-0005 既有 migration 文件
- 不修改 `internal/infra/migrate/migrate.go` / `internal/cli/migrate.go` / `cmd/server/main.go` 三个 Go 文件（4.3 已实装；本 story 仅新增 SQL 文件 + 跨包测试，Go 代码改动 0 行 —— 例外：本 story 的集成测试 case 在已有 `migrate_integration_test.go` **新增**几个 case 来覆盖第六张表，文件本身已经存在，是修改不是新增）
- 不接 Redis / 不引入新依赖（节点 3 server 阶段保持现有依赖最小集）
- 不写 README / 不更新部署文档（节点 3 / Epic 36 部署 story 才统一写）

## Acceptance Criteria

**AC1 — `server/migrations/` 目录新增 `user_step_sync_logs` up/down 双向 SQL**

新增两个 SQL 文件（**仅**新增，**不**修改 0001-0005）：

```
server/migrations/
├── 0001_init_users.up.sql                       # 4.3 落地，不动
├── 0001_init_users.down.sql                     # 4.3 落地，不动
├── 0002_init_user_auth_bindings.up.sql          # 4.3 落地，不动
├── 0002_init_user_auth_bindings.down.sql        # 4.3 落地，不动
├── 0003_init_pets.up.sql                        # 4.3 落地，不动
├── 0003_init_pets.down.sql                      # 4.3 落地，不动
├── 0004_init_user_step_accounts.up.sql          # 4.3 落地，不动
├── 0004_init_user_step_accounts.down.sql        # 4.3 落地，不动
├── 0005_init_user_chests.up.sql                 # 4.3 落地，不动
├── 0005_init_user_chests.down.sql               # 4.3 落地，不动
├── 0006_init_user_step_sync_logs.up.sql         # ★ 本 story 新增
└── 0006_init_user_step_sync_logs.down.sql       # ★ 本 story 新增
```

**`0006_init_user_step_sync_logs.up.sql` 内容必须严格逐字段对齐 §5.5（行 317-359）**：

```sql
-- 对齐 docs/宠物互动App_数据库设计.md §5.5 (行 317-359)
-- user_step_sync_logs 表：步数同步日志（节点 3 步数业务核心审计 / 增量计算依据）
-- - id BIGINT UNSIGNED AUTO_INCREMENT 主键（§3.1）
-- - user_id BIGINT UNSIGNED NOT NULL：归属用户
-- - sync_date DATE NOT NULL：客户端按本机时区算出的"今天"，server 直接采用不二次转换
--   （V1 §6.1.2 syncDate 字段说明 + GAP E 时区契约）
-- - client_total_steps BIGINT UNSIGNED NOT NULL：客户端读取到的"当天系统累计步数"
--   （非增量；增量由 server 按上次同步差值计算 —— V1 §6.1.4 服务端逻辑 + §8.2 步数同步事务）
-- - accepted_delta_steps INT NOT NULL DEFAULT 0：服务端实际确认入账的增量
--   （可能因截断 5000 / 当日封顶 50000 / 倒退 < clientTotalSteps 而 ≠ "client - last"，
--    防作弊语义对齐 V1 §6.1.4 + epics.md Story 7.3 GAP K）
-- - motion_state TINYINT NOT NULL DEFAULT 1：同步时客户端活动状态
--   （§6.5 钦定 1=stationary_or_unknown / 2=walking / 3=running）
-- - source TINYINT NOT NULL DEFAULT 1：步数来源
--   （§6.6 钦定 1=healthkit（客户端正常上报） / 2=admin_grant（Story 7.5 dev/运营手动发放））
-- - client_ts BIGINT UNSIGNED NOT NULL DEFAULT 0：客户端调用接口时的本机毫秒时间戳
--   （仅写日志审计用，不参与差值计算 —— V1 §6.1.2 clientTimestamp 字段说明）
-- - created_at DATETIME(3)：服务端写入时间（毫秒精度，§3.2）
-- 索引（§7.2 钦定）：
-- - idx_user_date (user_id, sync_date)：服务端差值计算查"最近一条"用（V1 §6.1.4 / §8.2）
-- - idx_user_created_at (user_id, created_at)：审计 / 时间序追溯（按用户按时间倒序查）
CREATE TABLE user_step_sync_logs (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    sync_date DATE NOT NULL,
    client_total_steps BIGINT UNSIGNED NOT NULL,
    accepted_delta_steps INT NOT NULL DEFAULT 0,
    motion_state TINYINT NOT NULL DEFAULT 1,
    source TINYINT NOT NULL DEFAULT 1,
    client_ts BIGINT UNSIGNED NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    KEY idx_user_date (user_id, sync_date),
    KEY idx_user_created_at (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

**`0006_init_user_step_sync_logs.down.sql` 内容**：

```sql
-- 回滚 0006_init_user_step_sync_logs.up.sql
DROP TABLE IF EXISTS user_step_sync_logs;
```

**关键 SQL 语法约束（与 4.3 落地的 0001-0005 完全一致 —— 不擅自分化）**：

- `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`（数据库设计 §3.4，**不**改 ENGINE / 不改 charset）
- `created_at` 用 `DATETIME(3)`（毫秒精度，§3.2）+ `DEFAULT CURRENT_TIMESTAMP(3)`
- 主键 `id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT`（§3.1，下发字符串由 service / handler 层负责，本 story 只关心 DB 层）
- 状态 / 枚举字段 `TINYINT NOT NULL DEFAULT N`（§3.3；本表 motion_state / source 都是 TINYINT）
- **明确不加 FK 约束**：MySQL 8.0 InnoDB 支持 FK，但项目惯例不依赖 FK 做引用完整性（业务层保证），与数据库设计 §5.5 钦定 DDL 一致（§5.5 全 DDL **不**含 FOREIGN KEY 子句；user_id 引 users.id 由业务层保证）
- **明确不加 deleted_at 字段**：节点 3 阶段表均**不需要** soft delete（§3.5 钦定 soft delete 是"未来扩展机制"，本 story 严格对齐 §5.5 **不**加列）
- **明确不加 updated_at 字段**：本表是**日志表**（append-only），日志一旦写入就不再修改 —— §5.5 钦定 DDL **没有** `updated_at`（区别于 user_step_accounts §5.4 / user_chests §5.6 这种"账户态"表）；本 story 严格对齐**不**加 updated_at
- **明确不加 sync_date 单列索引**：epics.md / §7.2 / §5.5 都没列 `sync_date` 单列索引；查"最近一条"走 `idx_user_date (user_id, sync_date)` 复合索引最左前缀已足够，**不**加 `KEY idx_sync_date (sync_date)` 这种冗余索引

**关键反模式**：

- **不**用 `IF NOT EXISTS` 在 .up.sql 的 `CREATE TABLE` —— golang-migrate 自带 schema_migrations 表防重复，up 一次只跑一次；用 `IF NOT EXISTS` 会掩盖"上一次 migrate 没干净"的状态污染（与 4.3 同纪律）
- **不**在 SQL 里 INSERT 任何业务数据（本表是日志表，数据由 7.3 / 7.5 service 实时写入；**不**写 seed —— 没有 seed 概念）
- **不**改文件命名格式（必须 `0006_init_user_step_sync_logs.{up|down}.sql` —— 编号 0006 单调递增，表名 `init_<table_name>` 与 0001-0005 一致；偏离会被 golang-migrate 拒绝或与 4.3 风格不一致）
- **不**新增 / 修改 0001-0005 任一文件（4.3 已锁定，节点 2 / 3 不动）

**AC2 — golang-migrate 自动 pickup（不改 Go 代码）**

本 story **不**需要修改任何 Go 代码：

- `server/internal/infra/migrate/migrate.go`：4.3 已实装的 `Migrator` interface（`Up` / `Down` / `Status` / `Close`）通过 golang-migrate v4 file source 自动扫描 `server/migrations/` 目录所有 `*.up.sql` / `*.down.sql` 文件 → 添加 0006 文件后 `Up` 会自动从 5 跑到 6
- `server/internal/cli/migrate.go`：4.3 已实装的 `RunMigrate` / `runMigrateAction` 不需要任何变更
- `server/cmd/server/main.go`：4.3 已实装的 `args[0] == "migrate"` 子命令分支不需要任何变更

**关键约束**：

- 本 story 改动**仅限**：(1) `server/migrations/` 下两个新 SQL 文件；(2) 集成测试文件 `server/internal/infra/migrate/migrate_integration_test.go` 在已有的"5 张表"断言里**扩展**到 "6 张表"（详见 AC4）；(3) 单测文件 `server/internal/infra/migrate/migrate_test.go` **不需要**任何改动（4.3 已覆盖 New / Close / 错误路径，与表数量无关）；(4) sprint-status.yaml + 本 story 文件
- 任何对 `migrate.go` / `migrate_test.go`（本身）/ `cli/migrate.go` / `cmd/server/main.go` 的改动都属"超范围"—— 触发即应 HALT 并问设计

**AC3 — `bash scripts/build.sh` 全量绿（vet + build + test + integration）**

完成后必须能跑通：

```bash
bash scripts/build.sh                    # vet + build → 不报错（migrate 包认 0006 文件，go vet 不会管 SQL）
bash scripts/build.sh --test             # 4.3 + 4.6 既有单测全过；本 story 不引入 Go 单测代码（SQL 测试走 dockertest）
bash scripts/build.sh --race --test      # Linux / CI 必过；本机 Windows 如 ThreadSanitizer 失败按 ADR-0001 §3.5 备注 skip 不阻塞
bash scripts/build.sh --integration      # 4.3 既有 4 个 migrate 集成测试 case + 本 story 扩展的断言全过；docker 不可用时 t.Skip
```

**关键约束**：

- 本 story **不**新增 Go 单测代码；新增的覆盖路径全在 dockertest 集成测试（AC4）—— 理由对齐 4.3 决策："纯 SQL 文件 + golang-migrate 跑 file source 直接走 sql.Open 路径，sqlmock 介入路径很深得不偿失"，单测覆盖 Go API 错误路径（4.3 已盖），新表的真实跑通靠 dockertest
- 本机 Windows 无 docker daemon → 集成测试 `t.Skip("docker not available")` 不阻塞（4.3 已建立 graceful skip 模式）
- 全程**不**改 `bash scripts/build.sh` 自身（脚本契约由 Story 1.7 钉死）

**AC4 — 集成测试覆盖（dockertest 起 mysql:8.0 + 真跑 6 张表 DDL + 验本表 schema）**

修改 `server/internal/infra/migrate/migrate_integration_test.go`（**不**新建文件，文件本身已经存在），**扩展**已有 4 个 case 中的两个：

**4.1 `TestMigrateIntegration_UpThenDown`**（已有）—— 把 `expectedTables` 从 5 元素扩到 6：

- 现有断言：`expectedTables := []string{"users", "user_auth_bindings", "pets", "user_step_accounts", "user_chests"}`
- **本 story 扩展**：append `"user_step_sync_logs"` 第六个元素 → 验 Up 后 6 张表都存在 / Down 后 6 张表都消失（仅留 `schema_migrations`）

**4.2 `TestMigrateIntegration_UpTwice_Idempotent`**（已有）—— 表数量从 5 改 6：

- 现有断言：`SELECT COUNT(*) ... WHERE table_name IN ('users', 'user_auth_bindings', 'pets', 'user_step_accounts', 'user_chests')` → 期望 5
- **本 story 扩展**：把 IN 列表加上 `'user_step_sync_logs'` 共 6 个表名；期望 `tableCount == 6`

**4.3 `TestMigrateIntegration_TablesPresent_AfterUp`**（已有）—— 扩展 `indexCases` + 新增本表关键字段类型断言：

- **新增 indexCases**（在原 9 个之后追加 2 个）：
  ```go
  {"user_step_sync_logs", "idx_user_date"},
  {"user_step_sync_logs", "idx_user_created_at"},
  ```
- **新增本表关键字段类型断言**（参照原 `users.id` / `version` 字段验证模式）：
  - `user_step_sync_logs.id`：`column_type == "bigint unsigned"`（PK，§3.1）
  - `user_step_sync_logs.user_id`：`column_type == "bigint unsigned"`（PK 引）
  - `user_step_sync_logs.sync_date`：`data_type == "date"` **不是** `datetime` —— 本表特殊，§5.5 钦定 sync_date 是 DATE 类型（仅日期不含时间，对齐 V1 §6.1.2 ISO 8601 date `YYYY-MM-DD` 字符串语义）
  - `user_step_sync_logs.client_total_steps`：`column_type == "bigint unsigned"`（§5.5 钦定 BIGINT UNSIGNED）
  - `user_step_sync_logs.accepted_delta_steps`：`column_type == "int"`（**注意**：§5.5 钦定 INT 不是 BIGINT，**不是 UNSIGNED** —— 因为防作弊场景 delta=0 时不能存负数没事，但保留 signed 让未来可能的"负向修正"扩展不卡死；本 story 严格按 §5.5 落地 INT signed）
  - `user_step_sync_logs.motion_state`：`column_type == "tinyint"`（§5.5 + §6.5）
  - `user_step_sync_logs.source`：`column_type == "tinyint"`（§5.5 + §6.6）
  - `user_step_sync_logs.client_ts`：`column_type == "bigint unsigned"`（§5.5 钦定 BIGINT UNSIGNED 毫秒时间戳）
  - `user_step_sync_logs.created_at`：`column_type == "datetime(3)"`（§3.2）
- **本表特殊验证**：**没有** `updated_at`（日志表 append-only） —— 在断言数组里**不**加 `{"user_step_sync_logs", "updated_at"}`；如有冗余检查可加一行"列不存在"断言：
  ```go
  // 本表是日志表（append-only），不含 updated_at 列
  var updatedAtCount int
  err = sqlDB.QueryRowContext(ctx, `
      SELECT COUNT(*) FROM information_schema.columns
      WHERE table_schema = 'cat_test' AND table_name = 'user_step_sync_logs' AND column_name = 'updated_at'`).Scan(&updatedAtCount)
  if err != nil { t.Errorf("query user_step_sync_logs.updated_at column existence: %v", err) }
  if updatedAtCount != 0 { t.Errorf("user_step_sync_logs.updated_at unexpectedly present (column count=%d, want 0; this table is append-only per §5.5)", updatedAtCount) }
  ```
- **本表 PK 验证**：与 user_step_accounts 不同（user_step_accounts PK = user_id），本表 PK = id（自增）。验法对应 `key_column_usage`：
  ```go
  var pkCol string
  err = sqlDB.QueryRowContext(ctx, `
      SELECT column_name FROM information_schema.key_column_usage
      WHERE table_schema = 'cat_test' AND table_name = 'user_step_sync_logs' AND constraint_name = 'PRIMARY'`).Scan(&pkCol)
  if err != nil { t.Errorf("query user_step_sync_logs PK: %v", err) }
  if pkCol != "id" { t.Errorf("user_step_sync_logs PK column = %q, want 'id'", pkCol) }
  ```

**4.4 `TestMigrateIntegration_StatusAfterUp`**（已有）—— 期望 version 从 5 改 6：

- 现有断言：`if v != 5 { ... want 5 }`
- **本 story 扩展**：改为 `if v != 6 { ... want 6 }`（多了 0006 一个 migration）

**关键约束**：

- **不**新增独立的 `*_test.go` 文件（统一在 `migrate_integration_test.go` 扩展，避免新建文件 / 重复 startMySQL helper / 测试发现冲突）
- **不**修改 4.3 落地的 startMySQL helper 函数 / migrationsPath helper 函数（这两个 helper 在同一文件，本 story 复用不动）
- 集成测试用的 DSN 必须含 `multiStatements=true` —— 4.3 startMySQL helper 已加，本 story 复用
- docker 不可用 → 4.3 helper 已实装 `t.Skip("docker not available")`，本 story 复用
- `bash scripts/build.sh --integration` 跑 4 个 case 全过；4 个 case 共用 dockertest，预计每个 case ~30-60s 冷启 mysql:8.0；总耗时 2-5 分钟可接受（与 4.3 同量级）

**AC5 — 不写独立 Go 单测（与 4.3 决策一致）**

本 story **不**写独立 `*_test.go` Go 单元测试代码：

- 4.3 已覆盖 `Migrator` Go API 的错误路径单测（New/Close/dirty status 等）—— 这些与"具体加几张表"无关，本 story **不**重复
- epics.md §Story 7.2 钦定的"≥3 单测 case"：
  - happy migrate up 后表存在 + 字段类型 + 索引符合 §5.5 → **AC4 集成测试 4.1 + 4.3 覆盖**
  - happy migrate down 后表删除 → **AC4 集成测试 4.1 覆盖**
  - edge 重复 migrate up → 幂等不报错 → **AC4 集成测试 4.2 覆盖**
- "≥3 case" 的语义在 4.3 落地 + 本 story 扩展上下文中应解读为"≥3 个验证场景全部覆盖到"，**不**强制要求"≥3 个 Go 单元测试函数"；纯 SQL 测试用 dockertest 集成测试比 sqlmock 单测更接近真实运行环境（4.3 已建立此模式 + 4.3 dev notes "测试策略" §4.3 已显式钦定）
- 如果 review 阶段坚持要补"Go 单元测试覆盖 ≥3 case"，则补 `migrate_test.go` 加 stub 单测（不连 DB，仅验"file source 能被 golang-migrate 找到 0006_init_user_step_sync_logs.up.sql"等不依赖容器的事实）；本 story 默认不补，由 review 决定

**关键反模式**：

- ❌ **不**用 sqlmock 模拟"INSERT user_step_sync_logs" —— 这是 7.3 service 的范围，不是本 story
- ❌ **不**新增 `migrate_test.go` 里"验 0006 文件存在"的 stub case —— 文件存在性靠 git 提交保证，不是 Go 测试的责任
- ❌ **不**写 go:embed 把 SQL 编进 binary 然后单测它能被解析 —— 4.3 已用 `file://` source 走 OS 文件系统，本 story 沿用，不引入新 source 类型

**AC6 — 验证清单（人工 + 自动化）**

完成后**人工**核对以下 6 项（结果记到 Completion Notes List）：

| # | 验证项 | 验证方式 |
|---|---|---|
| 1 | `0006_init_user_step_sync_logs.up.sql` 内容**逐字段**对齐数据库设计 §5.5 行 322-335（CREATE TABLE 块）| `Read` 工具读 SQL 文件 + `Read` 工具读 §5.5 源段，逐行核对 |
| 2 | `0006_init_user_step_sync_logs.up.sql` 顶部含 `-- 对齐 docs/宠物互动App_数据库设计.md §5.5 (行 NNN-MMM)` 注释（与 0001-0005 风格一致）| `Read` 工具读 SQL 文件首行 |
| 3 | `0006_init_user_step_sync_logs.down.sql` 是单行 `DROP TABLE IF EXISTS user_step_sync_logs;` + 注释（与 0001-0005 down 文件风格一致）| `Read` 工具读 SQL 文件 |
| 4 | `bash scripts/build.sh --test` 全绿 | bash 命令实际跑 |
| 5 | `bash scripts/build.sh --integration` —— 本机 Windows docker 不可用时 4 个集成测试 case 全部 SKIP（gracefuld）；如 docker 可用则 4 case 全过 | bash 命令实际跑（log 抽看 SKIP / PASS） |
| 6 | `git diff --name-only` 改动文件清单仅含：`server/migrations/0006_init_user_step_sync_logs.up.sql`（新增）+ `server/migrations/0006_init_user_step_sync_logs.down.sql`（新增）+ `server/internal/infra/migrate/migrate_integration_test.go`（修改 expectedTables / indexCases / Status version 等）+ `_bmad-output/implementation-artifacts/sprint-status.yaml`（状态翻 in-progress → review）+ `_bmad-output/implementation-artifacts/7-2-user_step_sync_logs-migration.md`（本 story 文件）= **5 个文件命中 / 0 个超范围**（如 .go 业务代码改 / 其他 docs 改 → 应触发 HALT）| `git status --short` + `git diff --stat` 抽看 |

**关键约束**：

- 验证 1 是本 story **最关键**的检查项：SQL 一旦与 §5.5 不一致，下游 7.3 service 实装会全红；review 阶段也会被 codex 抓住
- 验证 6 是范围红线最终防线：每多一个文件改动都需要在 Completion Notes 解释为何超范围

**AC7 — 不 commit（流水线由 epic-loop 下游收口）**

epics.md §Story 7.2 AC 钦定"含 down.sql + ≥3 单测 + dockertest 集成测试"，**不**像 7.1 那样钦定"Git commit 单独提交契约定稿"——本 story 是 server 业务代码 / migration story，commit 由 epic-loop 流水线在下游 fix-review / story-done sub-agent 阶段统一收口。

- 本 story 的 dev workflow **不** commit / **不** push
- commit message 模板（story-done 阶段使用）：
  ```text
  feat(migrations): 0006_init_user_step_sync_logs 表 + dockertest 6 表断言（Story 7.2）

  - server/migrations/ 加 0006_init_user_step_sync_logs.{up,down}.sql 严格对齐
    数据库设计 §5.5 字段集（id / user_id / sync_date / client_total_steps /
    accepted_delta_steps / motion_state / source / client_ts / created_at）+
    §7.2 索引（idx_user_date / idx_user_created_at）
  - migrate_integration_test.go 4 个 case 扩展到 6 表断言
    （UpThenDown / UpTwice_Idempotent / TablesPresent_AfterUp / StatusAfterUp）
  - 仅新增 SQL 文件，不修改任何 Go 业务代码（4.3 migrate 框架自动 pickup）

  依据 epics.md §Story 7.2 + docs/宠物互动App_数据库设计.md §5.5 / §6.5 / §6.6 / §7.2。
  下游 Story 7.3 service 差值入账事务 INSERT 命中本表；Story 7.5 dev grant 写
  source=2 (admin_grant) 命中 §6.6 枚举。

  Story: 7-2-user_step_sync_logs-migration
  ```
- commit hash 待 story-done 阶段产生后回填到本文件（如有 fix-review 返工，commit hash 在 fix-review commit 之后再统一更新）

## Tasks / Subtasks

- [x] **Task 1（AC1）**：写 `0006_init_user_step_sync_logs.up.sql` + `.down.sql`（与 4.3 落地的 0001-0005 风格完全一致）
  - [x] 1.1 `Read` 工具读 `docs/宠物互动App_数据库设计.md` §5.5（行 317-359）+ §6.5 / §6.6（行 757-770）+ §7.2（行 866-868），把 schema 当"标准答案"
  - [x] 1.2 `Read` 工具读已有 `server/migrations/0005_init_user_chests.up.sql` 当模板（注释风格 / SQL 缩进 / ENGINE 写法）
  - [x] 1.3 `Write` 工具创建 `server/migrations/0006_init_user_step_sync_logs.up.sql`：顶部加 `-- 对齐 docs/宠物互动App_数据库设计.md §5.5 (行 317-359)` 注释（行号要确认）+ 字段说明注释（参考 AC1 模板，每个字段一行注释说明语义 + 引用 V1 / §X.Y）+ CREATE TABLE 块逐字段抄 §5.5 + 索引 idx_user_date / idx_user_created_at
  - [x] 1.4 `Write` 工具创建 `server/migrations/0006_init_user_step_sync_logs.down.sql`：单行 `DROP TABLE IF EXISTS user_step_sync_logs;` + 顶部 `-- 回滚 0006_init_user_step_sync_logs.up.sql` 注释（与 0005 down 风格一致）
  - [x] 1.5 `Read` 工具回读两个新文件，逐字符核对 SQL 内容与 §5.5 一致（特别检查：sync_date 是 DATE 不是 DATETIME / accepted_delta_steps 是 INT 不是 BIGINT / 没有 updated_at 列 / 没有 FK 约束 / ENGINE=InnoDB DEFAULT CHARSET=utf8mb4）
- [x] **Task 2（AC2）**：验 4.3 实装的 migrate Go API 不需要任何改动
  - [x] 2.1 `Read` 工具读 `server/internal/infra/migrate/migrate.go` 的 New 函数 + `internal/cli/migrate.go` 的 RunMigrate 函数，确认它们用 `file://` source 自动扫描 `server/migrations/` 目录（**不**写死文件名 / 编号上限 / 表数量）
  - [x] 2.2 确认 0006_*.{up,down}.sql 文件加进 `server/migrations/` 目录后，下次 `catserver migrate up` 会从当前 schema_migrations 版本（5）自动跑到 6（这是 golang-migrate v4 的语义保证 + 4.3 集成测试 `TestMigrateIntegration_StatusAfterUp` 已验证 Status 返回 version 等于实际跑过的最大编号）
  - [x] 2.3 **不**修改 `migrate.go` / `cli/migrate.go` / `cmd/server/main.go` 任一行；任何修改这些文件的尝试触发停下问设计
- [x] **Task 3（AC4）**：扩展 `migrate_integration_test.go` 4 个 case 的断言到 6 张表
  - [x] 3.1 `Read` 工具读 `server/internal/infra/migrate/migrate_integration_test.go` 全文（至少行 1-389），定位四个 case 的断言点
  - [x] 3.2 `Edit` 工具改 `TestMigrateIntegration_UpThenDown` 的 `expectedTables` 从 5 元素扩到 6（append `"user_step_sync_logs"`）
  - [x] 3.3 `Edit` 工具改 `TestMigrateIntegration_UpTwice_Idempotent` 的 IN 列表 + `tableCount` 期望从 5 改 6
  - [x] 3.4 `Edit` 工具在 `TestMigrateIntegration_TablesPresent_AfterUp` 的 `indexCases` 末尾追加 2 行（`{"user_step_sync_logs", "idx_user_date"}` + `{"user_step_sync_logs", "idx_user_created_at"}`）
  - [x] 3.5 `Edit` 工具在 `TestMigrateIntegration_TablesPresent_AfterUp` 末尾追加本表关键字段类型断言（参照 AC4.3 给的 8 个字段 column_type / data_type 校验 + PK 是 id 校验 + updated_at 列不存在校验）
  - [x] 3.6 `Edit` 工具改 `TestMigrateIntegration_StatusAfterUp` 的 version 期望从 5 改 6
- [x] **Task 4（AC3 / AC6）**：全量验证
  - [x] 4.1 `bash scripts/build.sh`（vet + build） — 必过（migrate 包 file source 自动认 0006 文件，go vet 不会管 SQL 内容）
  - [x] 4.2 `bash scripts/build.sh --test` — 既有单测全过；本 story 不引入新单测（与 AC5 决策一致）
  - [x] 4.3 `bash scripts/build.sh --race --test` — 跳过本机 race（Windows + msys ThreadSanitizer 限制；ADR-0001 §3.5 允许 skip 不阻塞，由 CI Linux 真验）
  - [x] 4.4 `bash scripts/build.sh --integration` — 4 个 migrate 集成测试 case 全部 SKIP（docker daemon not reachable, 本机 Windows）；不阻塞
  - [x] 4.5 `git status --short` 抽检：5 个文件命中预期范围（新增 SQL 2 个 / 修改 migrate_integration_test.go / 修改 sprint-status.yaml / 修改本 story 文件）
  - [x] 4.6 `git diff` 抽检：SQL 文件内容与 §5.5 一致；测试文件改动量小（仅扩展现有断言 + 新增本表字段验证 / PK 验证 / 无 updated_at 验证）
  - [x] 4.7 在下方 Completion Notes List 勾选 AC6 验证清单的 6 项（pass / fail / skip 原因）
- [x] **Task 5（AC7）**：本 story 不做 git commit
  - [x] 5.1 epic-loop 流水线约束：dev-story 阶段不 commit；由下游 fix-review / story-done sub-agent 收口
  - [x] 5.2 commit message 模板（AC7）保留在 story 文件中，story-done sub-agent 可直接复用
  - [x] 5.3 commit hash 待 story-done 阶段产生后回填（如有 fix-review 返工，commit hash 在 fix-review commit 之后再统一更新）

## Dev Notes

### 关键设计原则

1. **schema 字面量必须严格对齐 §5.5**：本 story 是 `user_step_sync_logs` 的**首次**也是**唯一**落地。`docs/宠物互动App_数据库设计.md` §5.5 是契约**输入**，本 story 是其**第一次落地**——schema 漂移会在 7.3 / 7.5 / Epic 9 e2e 测试时持续返工，比 4.3 5 张表的纠错代价**更高**（节点 3 demo 阻塞）。具体到本 story：DDL **不**用 GORM AutoMigrate 自动生成（生成结果会偷加列 / 索引 / 改类型），**手写**纯 SQL 让 review 阶段能逐字段核对 §5.5。
2. **本表是 append-only 日志**：与 `user_step_accounts` §5.4（账户态表，三柱式记账 + version 乐观锁 + updated_at 反映最后修改时间）不同，本表是**日志表** —— 一行写入后再不修改（数据库设计 §5.5 钦定**没有** updated_at 列；没有 version 乐观锁列；没有"软删除"语义）。这条性质决定下游 7.3 / 7.5 service 实装时**只 INSERT 不 UPDATE 不 DELETE**：每次 sync / dev grant 都新增一行；倒退 / 重复 sync 也新增（V1 §6.1.4 服务端逻辑第 3 段"倒退场景仍写日志"明确）。本 story SQL 严格不加 updated_at / version / deleted_at —— 添加这些列会引入"日志可能被改写"的语义歧义。
3. **`accepted_delta_steps` INT 不是 BIGINT 也不是 UNSIGNED**：§5.5 钦定 `accepted_delta_steps INT NOT NULL DEFAULT 0`。理由：(1) INT 范围 -2^31 ~ 2^31-1（约 21 亿）足够覆盖单次入账（防作弊封顶 5000 远小于 INT 上限）；(2) **保留 signed**：未来如果出现"运营手动负向修正"场景（如发现某用户作弊后 admin 用 -10000 抵消之前的累计），仍能用同一字段记录，不需要新加 `is_revert` 列；(3) `client_total_steps` 是 BIGINT UNSIGNED 因为它代表"累计步数"理论上 long-running 用户可能上亿，需要更大范围；`accepted_delta_steps` 是单次入账，不需要 BIGINT。本 story **严格按 §5.5 落地 INT signed**，不擅自改类型。
4. **`sync_date` DATE 类型不是 VARCHAR 不是 DATETIME**：§5.5 钦定 `sync_date DATE NOT NULL`。理由：(1) DATE 类型存"日期"语义（YYYY-MM-DD），InnoDB 占 3 字节，比 VARCHAR(10) 省 7 字节 / 行；(2) DATE 类型可以用 `WHERE sync_date = ?` 直接索引匹配（V1 §6.1.4 / §8.2 服务端差值计算"查 user_step_sync_logs WHERE user_id=? AND sync_date=?"），如果用 VARCHAR 则需要字符串匹配，索引效率打折；(3) DATE 不带时区，对齐 V1 §6.1.2 syncDate 字段语义"客户端按本机时区算今天，server 直接采用不二次转换"—— 这种 "字符串语义日期"用 DATE 最合适，DATETIME 反而携带"时间分量 00:00:00"会让对比逻辑混淆。
5. **索引 `idx_user_date` 在前 `idx_user_created_at` 在后**：与 §5.5 钦定一致。**最左前缀**优先使用 `(user_id, sync_date)` —— V1 §6.1.4 服务端差值计算"查 user_step_sync_logs WHERE user_id=? AND sync_date=? 取最近一条"是高频查询，命中 `idx_user_date` 完整最左前缀；`(user_id, created_at)` 是审计查询（按用户按时间倒序），命中 `idx_user_created_at` 完整最左前缀。**不**合并成一个索引（两个 query 模式不重叠），**不**加 `sync_date` 单列索引（无独立按日期跨用户聚合的查询）。
6. **fail-fast over auto-migrate**（4.3 已锁定）：本 story 沿用 4.3 决策 —— server 启动**不**自动跑 migrate up（"启动自动 migrate"是 footgun）；migrate 是显式运维操作，靠 `catserver migrate up` 子命令在 CI / 部署阶段单独 step 执行。本 story 加了 0006 文件后，运维 / 测试需要在升级到节点 3 server 时**手动**或 CI step 跑一次 `catserver migrate up`，把 schema 从 5 推到 6。
7. **不重复 4.3 的 Go API 单测**：4.3 在 `internal/infra/migrate/migrate_test.go` + `internal/cli/migrate_test.go` 已覆盖 New / Close / RunMigrate / runMigrateAction 错误路径单测。本 story **不**新增 Go 单测（AC5 钦定）—— epics.md "≥3 单测 case" 在本 story 上下文应解读为"≥3 个验证场景全部覆盖"，集成测试覆盖更接近真实运行环境（4.3 dev notes 已建立此模式）。

### 架构对齐

**领域模型层**（`docs/宠物互动App_总体架构设计.md`）：

- 步数是节点 3 的核心可消费资产；`user_step_sync_logs` 是步数账户的**审计 + 增量计算依据**（一切对账户的修改都由本表的"上次记录"差值算出）
- 差值算法（不接受客户端直接报 delta）抗重放攻击：本表必须能让 server 准确查到"上次同步记录"才能计算正确 delta

**数据库层**（`docs/宠物互动App_数据库设计.md`）：

- §3.1 主键策略：本表 PK = `id BIGINT UNSIGNED AUTO_INCREMENT`（与 user_step_accounts §5.4 PK = user_id 不同 —— user_step_accounts 是 1:1 关联用户的"账户表"，本表是 1:N 的"日志表"）
- §3.2 时间字段：created_at = DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3)；本表**没有** updated_at（日志 append-only）
- §3.3 状态字段：motion_state / source 都是 TINYINT NOT NULL DEFAULT N
- §3.4 索引命名：UNIQUE 用 `uk_xxx`（本表无 UNIQUE 索引，因为日志表允许同一用户同一日多次 sync），普通 INDEX 用 `idx_xxx`（本表 idx_user_date / idx_user_created_at）
- §5.5：本 story 的 source of truth；逐字段对齐
- §6.5 motion_state 枚举（1=stationary_or_unknown / 2=walking / 3=running）+ §6.6 source 枚举（1=healthkit / 2=admin_grant）：本 story 在 SQL 注释里标注枚举值（让 SQL 自描述，与 4.3 5 张表风格一致）
- §7.2 索引建议（`user_step_sync_logs(user_id, sync_date)` + `user_step_sync_logs(user_id, created_at)`）：本 story 严格按这两条建索引
- §8.2 步数同步事务（行 897-906）：本 story 是其**前置**（建空表，让 7.3 service 在 `txManager.WithTx(ctx, fn)` 内 INSERT）

**服务端架构层**（`docs/宠物互动App_Go项目结构与模块职责设计.md`）：

- §4 项目目录建议：`migrations/` 目录已列在目录树（含 `0006_init_user_step_sync_logs.sql` 示例文件名提示）；本 story 在 `server/migrations/` 落地 `.up.sql` + `.down.sql` 双向文件（与 4.3 同形式）
- §5.5 Infrastructure 层：本 story 不动 `internal/infra/migrate/` 包的 Go 代码（4.3 已实装），仅扩集成测试断言

**接口契约层**（`docs/宠物互动App_V1接口设计.md`）：

- §1（节点 3 冻结声明）：本 story 实装严格按 7.1 已冻结的契约 —— `client_total_steps` (BIGINT UNSIGNED) 对齐 §6.1.2 `clientTotalSteps` (number int) 语义；`motion_state` 对齐 §6.1.3 motionState 枚举；`source` 对齐 §6.6 + Story 7.5 dev grant
- §6.1 + §6.5 + §6.6 + §8.2 是本 story 的**消费方**（本 story 是它们的**实装**）

### 测试策略

按 4.3 已建立的测试范式 + ADR-0001 §3.1：

- **单测层**：本 story **不**新增 Go 单测（4.3 Go API 单测已覆盖；纯 SQL DDL 用单测 / sqlmock 不现实）
- **集成测试层**（`migrate_integration_test.go` + `//go:build integration`）：用 4.3 已有的 dockertest 起 mysql:8.0 容器；扩展 4 个 case 的断言到 6 张表 + 本表关键字段类型 / 索引 / PK
- 触发：`bash scripts/build.sh --integration`（不在 default `go test ./...` 跑，避免 CI 慢）；docker 不可用 → t.Skip 不阻塞

**关键决策**：

- 本 story **不**用 sqlmock（4.3 已论证：golang-migrate file source 直接读文件 + database/mysql driver 走 sql.Open，sqlmock 介入路径深得不偿失）
- 本 story **不**抽 startMySQL 跨包 helper（4.3 已选择"复制一份在 migrate 包内"）
- 本 story 集成测试断言严格对齐 §5.5 字段集（每个字段都验 column_type，避免"测了几个字段就放过"导致 schema 漂移）

### 与已 done 的 4.3 / 7.1 的衔接

**4.3 已实装 / 本 story 复用**：

- `server/migrations/` 目录 + golang-migrate v4 file source 扫描机制 → 本 story 加文件即生效
- `internal/infra/migrate.Migrator` interface (Up/Down/Status/Close) → 本 story 不改一行
- `internal/cli/migrate.go RunMigrate / runMigrateAction` → 本 story 不改一行
- `cmd/server/main.go args[0]=="migrate"` 子命令分支 → 本 story 不改一行
- `migrate_integration_test.go` 的 `startMySQL` / `migrationsPath` helper → 本 story 复用
- `_ "github.com/golang-migrate/migrate/v4/source/file"` + `_ "github.com/golang-migrate/migrate/v4/database/mysql"` blank import → 本 story 不动

**7.1 已冻结契约 / 本 story 实装的契约对齐**：

- `client_total_steps` BIGINT UNSIGNED 对齐 V1 §6.1.2 `clientTotalSteps` number int（client 上行）
- `sync_date` DATE 对齐 V1 §6.1.2 `syncDate` string ISO 8601 date `YYYY-MM-DD`（client 上行）
- `motion_state` TINYINT 对齐 V1 §6.1.3 motionState enum 1/2/3 + §6.5 motion_state 枚举
- `source` TINYINT 对齐 §6.6 + Story 7.5 admin_grant=2
- `client_ts` BIGINT UNSIGNED 对齐 V1 §6.1.2 `clientTimestamp` int64 ms（client 上行）
- `accepted_delta_steps` INT 对齐 V1 §6.1.5 `data.acceptedDeltaSteps` number int（server 下行）

### 与下游 7.3 / 7.5 / iOS 8.5 的接口

**7.3 落地时会做（依赖本 story）**：

1. 假设 DB 已有 `user_step_sync_logs` 空表（运维 / 测试调过 `migrate up`）
2. 实装 `repo/mysql/step_sync_log_repo.go`：`Create(ctx, log) error` / `GetLatestByUserAndDate(ctx, userID, syncDate) (*StepSyncLog, error)`
3. 实装 `service/step_service.go::SyncSteps(ctx, ...)`：在 `txManager.WithTx(ctx, fn)` 内串 `step_account_repo.UpdateBalance` + `step_sync_log_repo.Create`
4. 实装 `app/http/handler/steps.go::PostSync`：参数校验 → 调 service → 转 response（V1 §6.1.5 schema）
5. 集成测试在 dockertest 容器内：先 `migrator.Up(ctx)` 建表（包括本表）→ 再调 service.SyncSteps → 验 sync_log + step_account 状态

**7.5 落地时会做（依赖本 story）**：

1. 假设 DB 已有 `user_step_sync_logs` 空表
2. 实装 `app/http/handler/dev/grant_steps.go::GrantSteps`：调 service 增加 step_account.total_steps += steps + 写一条 `source=2 (admin_grant)` 的 sync_log
3. 集成测试：`/dev/grant-steps` 5000 → `/steps/account` 返回 available_steps=5000 + sync_log 表有一条 source=2 行

**iOS 8.5 间接依赖**：通过 7.3 service → 本表

**本 story 必须保证下游能直接用**：

- 本表字段都按 §5.5 钦定，7.3 / 7.5 的 repo 不会找不到字段
- 索引 `idx_user_date` 让 7.3 service "查最近一条" query 命中索引最左前缀
- 索引 `idx_user_created_at` 让审计 query / Epic 9 e2e 测试"按用户时间序回查"命中索引

### Project Structure Notes

预期文件 / 目录变化（**仅 5 个文件**，超出即 HALT）：

- ✅ **新增**：`server/migrations/0006_init_user_step_sync_logs.up.sql`
- ✅ **新增**：`server/migrations/0006_init_user_step_sync_logs.down.sql`
- ✅ **修改**：`server/internal/infra/migrate/migrate_integration_test.go`（4 个 case 的断言扩展到 6 张表 / 6 表 schema 验证）
- ✅ **修改**：`_bmad-output/implementation-artifacts/sprint-status.yaml`（7-2-user_step_sync_logs-migration: backlog → ready-for-dev → in-progress → review）
- ✅ **修改**：`_bmad-output/implementation-artifacts/7-2-user_step_sync_logs-migration.md`（本 story 文件，dev 完成后填 Tasks/Dev Agent Record/File List/Completion Notes）

不影响其他目录（**全部不动**）：

- ❌ `server/internal/infra/migrate/migrate.go` 不变（4.3 已实装）
- ❌ `server/internal/infra/migrate/migrate_test.go` 不变（4.3 单测与本 story 无关）
- ❌ `server/internal/cli/migrate.go` / `migrate_test.go` 不变（4.3 已实装）
- ❌ `server/cmd/server/main.go` 不变（4.3 已实装的子命令分支自动 pickup 0006）
- ❌ `server/internal/infra/db/` 不变（4.2 已实装）
- ❌ `server/internal/repo/` 任一子包不变（节点 3 业务 repo 在 7.3 才实装）
- ❌ `server/internal/service/` 任一子包不变（业务在 7.3 才实装）
- ❌ `server/internal/app/http/` 任一子包不变（handler 在 7.3 / 7.4 / 7.5 才实装）
- ❌ `server/configs/local.yaml` / `dev.yaml` / `staging.yaml` / `prod.yaml` 不变（无新配置项）
- ❌ `server/internal/infra/config/` 不变（无新配置项）
- ❌ `server/migrations/0001-0005` 任一文件不变（4.3 已锁定）
- ❌ `iphone/` / `ios/` 全部不动（server-only story）
- ❌ `docs/宠物互动App_*.md` 全部 7 份不变（消费方）
- ❌ `docs/宠物互动App_V1接口设计.md` 不变（7.1 已冻结契约）
- ❌ `docs/lessons/` 不变（如 review 阶段产生新 lesson 由 fix-review workflow 处理）
- ❌ `_bmad-output/planning-artifacts/*` 不变（除非 review 阶段发现 epics.md / V1 文档存在跨文档不一致需要同步刷新；本 story dev 阶段不主动改）
- ❌ `README.md` / `server/README.md` 不变（Epic 4 / Epic 36 收尾才统一更新）
- ❌ 其他 `_bmad-output/implementation-artifacts/*.md` 不变（本 story 是独立 story）

### 与 4.3（5 张表 migrations）的对比（参照 4.3 模式而非复刻）

4.3 落地了 0001-0005 五张表 + migrate Go API 包装层 + cli 子命令分发 + 8 个单测 + 4 个集成测试，开了"如何加 migration"的范式（文件命名 / up.sql + down.sql 双文件 / 编号顺序 / migrate CLI 子命令调用 / 集成测试断言模式）。本 story（0006 第六张表）相比 4.3：

- **表数量减少**：4.3 一次性落 5 张表（10 个 SQL 文件 + 大量字段验证）；本 story 落 1 张表（2 个 SQL 文件 + 集成测试增量断言）；工作量约为 4.3 的 1/5
- **Go 代码改动量**：4.3 实装了 `internal/infra/migrate/migrate.go` + `internal/cli/migrate.go` + `cmd/server/main.go` 三个 Go 文件（~250 行）；本 story Go 代码改动量 = 0（仅集成测试 4 个 case 扩展断言，~30 行 diff）
- **单测**：4.3 写了 8 个 Go 单测 case；本 story 写 0 个 Go 单测（AC5 钦定，理由在 dev notes 测试策略）
- **集成测试**：4.3 落地了 4 个独立 case（UpThenDown / UpTwice_Idempotent / TablesPresent_AfterUp / StatusAfterUp）+ startMySQL helper；本 story **复用** 4 个 case 不新增 case，仅在每个 case 里**扩展断言**到 6 张表（不重写测试结构）
- **字段层差异**：本表是日志表（append-only，无 updated_at / version），与 user_step_accounts §5.4 / user_chests §5.6 这种"账户态表"不同；DDL 在"无 updated_at" / "accepted_delta_steps INT signed" / "sync_date DATE 类型" 三处需要特别注意

参照 4.3 的成熟模式：4.3 走过的 5 轮 codex review 主要是 (1) 集成测试 helper 复制 vs 抽包决策；(2) `internal/cli` interface 复制 vs 共享决策；(3) tools.go 删除 vs 保留决策；(4) ErrNoChange / ErrNilVersion 吞掉的语义；(5) signal-ctx 独立性。**本 story 都不会再触发这些点**（架构决策已固化），review 焦点应放在：

- SQL 内容是否逐字段对齐 §5.5（每个字段类型 / 默认值 / 索引名 / 约束）
- 集成测试断言是否覆盖到 6 张表的关键 schema 元素（特别是本表特殊点：sync_date DATE / accepted_delta_steps INT signed / 无 updated_at / PK = id）
- 范围红线是否守住（仅 5 个文件改动）

### golang-migrate v4 关键 API 提示（4.3 已总结，本 story 沿用）

实装时注意（避免常见坑，4.3 dev notes 已总结）：

- `migrate.New("file://"+path, "mysql://"+dsn)` 的 file source 自动扫描目录所有 `*.{up|down}.sql` 文件（按编号排序）→ 加 0006 文件后下次 Up 自动跑到 6
- DSN 必须加 `multiStatements=true`（4.3 startMySQL helper 已加）—— 本表 SQL 是单 CREATE TABLE 语句，不严格依赖 multiStatements；但 helper 已含，本 story 不改
- `migrate.ErrNoChange`：4.3 已在 Up/Down 内吞掉返 nil；幂等 Up 的语义保证下游集成测试 `TestMigrateIntegration_UpTwice_Idempotent` 仍 pass
- `migrate.ErrLocked`：4.3 让 error 透传给 CLI；本 story 不特殊处理

### 节点 3 阶段 server 一致性纪律

- 数据库设计 §5.5 是契约**输入**：本 story 严格对齐它，**不**反向修改 §5.5（如发现 §5.5 与 V1 §6.1 字段语义有歧义 → 不在本 story 阶段调整 docs/，由 review 阶段或 fix-review 处理）
- V1 §6.1 / §6.5 / §6.6 已冻结（7.1）：本 story 是 V1 契约的**底层实装**之一，字段名 / 类型对齐
- 节点 3 阶段所有 server 业务都是"上层 service 用 4.2 / 4.3 / 7.2 已建好的基础设施" —— 本 story 是节点 3 server 业务的**第二个**底层 epic story（7.1 是契约 / 本 story 是表 / 7.3 是业务事务）；先把基础打好，节点 3 业务（7.3 / 7.4 / 7.5）才能并行开工

### References

- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 7.2 (行 1348-1366)] — 本 story 的钦定 AC 来源（migration 文件名 + 字段全集 + 索引 + ≥3 单测 + dockertest 集成测试）
- [Source: `_bmad-output/planning-artifacts/epics.md` §Epic 7 Overview (行 1326-1328)] — Epic 7 节点 3 server 端步数同步与账户记账定位
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 7.3 (行 1368-1402)] — 下游 service 实装的字段使用 + 防作弊阈值 GAP K + syncDate 时区契约 GAP E + 集成测试场景
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 7.5 (行 1423-1443)] — 下游 dev grant-steps 写 source=2 sync_log 的来源
- [Source: `docs/宠物互动App_数据库设计.md` §5.5 user_step_sync_logs (行 317-359)] — 本 story DDL 的 source of truth；逐字段对齐
- [Source: `docs/宠物互动App_数据库设计.md` §3.1 主键策略 (行 73-80)] — 本表 PK = id BIGINT UNSIGNED AUTO_INCREMENT
- [Source: `docs/宠物互动App_数据库设计.md` §3.2 时间字段 (行 84-95)] — created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3) 规则
- [Source: `docs/宠物互动App_数据库设计.md` §3.3 状态字段 (行 99-104)] — motion_state / source 都是 TINYINT
- [Source: `docs/宠物互动App_数据库设计.md` §3.4 索引命名 (行 108-118)] — UNIQUE 用 uk_xxx / 普通 INDEX 用 idx_xxx 命名约定
- [Source: `docs/宠物互动App_数据库设计.md` §6.5 user_step_sync_logs.motion_state (行 757-763)] — 1=stationary_or_unknown / 2=walking / 3=running 枚举
- [Source: `docs/宠物互动App_数据库设计.md` §6.6 user_step_sync_logs.source (行 765-770)] — 1=healthkit / 2=admin_grant 枚举
- [Source: `docs/宠物互动App_数据库设计.md` §7.2 高优先级普通索引 (行 866-868)] — `user_step_sync_logs(user_id, sync_date)` + `user_step_sync_logs(user_id, created_at)` 索引建议（与 §5.5 完全一致）
- [Source: `docs/宠物互动App_数据库设计.md` §8.2 步数同步事务 (行 897-906)] — 下游 7.3 service 事务边界（本 story 是其前置）
- [Source: `docs/宠物互动App_V1接口设计.md` §6.1 POST /steps/sync] — 7.1 已冻结契约；本 story 字段实装与之对齐（client_total_steps / motion_state / source / client_ts / sync_date / accepted_delta_steps 字段语义）
- [Source: `docs/宠物互动App_V1接口设计.md` §1 节点 3 契约冻结声明] — 7.1 已锚定的节点 3 契约冻结约束；本 story 实装严格对齐
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 项目目录建议 (行 116-121)] — `migrations/` 目录已列在目录树；本 story 在 `server/migrations/` 落地与 4.3 同形式
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §5.5 Infrastructure 层 (行 319-340)] — `internal/infra/migrate/` 已实装（4.3）；本 story 不动 Go 代码
- [Source: `_bmad-output/implementation-artifacts/decisions/0003-orm-stack.md` §3.2] — golang-migrate v4 选定 + 否决 AutoMigrate / goose / atlas + 版本 pin v4.18.1
- [Source: `_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.1 (行 42-53)] — sqlmock + dockertest 双层测试策略；本 story 沿用 4.3 决策（不写 sqlmock，集成测试用 dockertest）
- [Source: `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md`] — ctx 必传；本 story 不引入新 Go 代码，无 ctx 适配点
- [Source: `_bmad-output/implementation-artifacts/4-3-五张表-migrations.md`] — 上一 migration story；本 story 复用 4.3 的 migrate Go API + cli 子命令分发 + dockertest helper + 测试 case 结构；按 4.3 风格扩展断言
- [Source: `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md`] — 上一 story；7.1 冻结的 V1 §6.1 / §6.5 / §6.6 字段语义是本 story DDL 的契约对齐目标
- [Source: `_bmad-output/implementation-artifacts/4-2-mysql-接入.md`] — 4.2 实装的 cfg.MySQL.DSN + tx manager + db.Open；本 story 不直接消费但下游 7.3 / 7.5 会用
- [Source: `_bmad-output/implementation-artifacts/4-6-游客登录接口-首次初始化事务.md`] — 4.6 是 4.3 + 4.2 + 4.4 + 4.5 的下游业务集成示范；本 story 是 4.3 之后的第一张表扩展，与 4.6 不直接交叉但工程纪律相同（事务包裹 + ctx 传播 + repo 边界）
- [Source: `docs/lessons/2026-04-26-config-env-override-and-gorm-auto-ping.md`] — 4.2 review lesson；本 story 不引入新 env override，不触发该 lesson 关注点
- [Source: `docs/lessons/2026-04-26-startup-blocking-io-needs-deadline.md`] — 启动阻塞 IO 必须有 deadline；本 story 不引入新启动路径，不触发该 lesson 关注点
- [Source: `docs/lessons/2026-04-30-r1-r9-接口契约 story 跨文档同步纪律.md`（如已 distill 到 lessons）] — 7.1 review 教训：契约 story 必须连同时序图 + 数据库枚举一起锚定；本 story 是契约的**实装**而非契约本身，但严格对齐 §5.5 / §6.5 / §6.6 / §7.2 + V1 §6.1 已冻结条款，避免再次踩"跨文档不同步"的坑
- [Source: `CLAUDE.md` §"工作纪律"] — "节点顺序不可乱跳 / 资产类操作必须事务 / 状态以 server 为准 / ctx 必传"；本 story 是节点 3 第二条 server story（7.1 done → **本 story** → 7.3 → 7.4 → 7.5），按节点顺序推
- [Source: `CLAUDE.md` §"节点 1 之后的目录形态 (target)"] — `migrations/` 目录已锚定在 `server/migrations/` 顶层
- [Source: `MEMORY.md` "No Backup Fallback"] — 反对 fallback 掩盖核心风险；本 story 严格按 §5.5 落地，不擅自加 IF NOT EXISTS / DROP IF EXISTS（仅 down.sql 用 DROP IF EXISTS 是回滚正确语义）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]（Anthropic Claude Opus 4.7, 1M context）

### Debug Log References

- `bash scripts/build.sh --test`：全绿（vet + build + 全部 Go 单测，22 个包，no failures）
- `bash scripts/build.sh --integration`：全绿（同 --test 路径 + integration tag；migrate 包 4 个集成测试在本机 Windows 因 `docker daemon not reachable` 全部 SKIP，符合 4.3 graceful skip 模式）
- `go test -tags=integration -v -run TestMigrateIntegration ./internal/infra/migrate/...` 抽样确认 4 case 输出：`--- SKIP` × 4，`PASS` ok（dockertest 容器未启动 → t.Skipf 触发 → 不阻塞，下游 CI Linux 跑 docker 真验断言）

### Completion Notes List

**实装路径**：

- AC1：在 `server/migrations/` 新增 `0006_init_user_step_sync_logs.up.sql`（CREATE TABLE 块逐字段对齐 §5.5 行 322-335，含 9 列 + 2 索引 + 详细字段语义注释）+ `0006_init_user_step_sync_logs.down.sql`（单行 `DROP TABLE IF EXISTS user_step_sync_logs;` + 注释，与 0001-0005 down 风格一致）
- AC2：`migrate.go` / `cli/migrate.go` / `cmd/server/main.go` 任一行均**不**修改；golang-migrate file source 自动 pickup 0006 文件，CLI `catserver migrate up` 自动从 5 推到 6（语义由 4.3 集成测试保证）
- AC3：`bash scripts/build.sh --test` / `--integration` 全绿；本 story 不引入新 Go 单测（与 AC5 / 4.3 决策一致）
- AC4：扩展 `migrate_integration_test.go` 四个 case：
  - `TestMigrateIntegration_UpThenDown`：`expectedTables` 从 5 → 6（append `"user_step_sync_logs"`）
  - `TestMigrateIntegration_UpTwice_Idempotent`：IN 列表 + `tableCount` 期望从 5 → 6
  - `TestMigrateIntegration_TablesPresent_AfterUp`：`indexCases` 追加 `{user_step_sync_logs, idx_user_date}` + `{user_step_sync_logs, idx_user_created_at}`；`timeCols` 追加 `{user_step_sync_logs, created_at}`；末尾追加 8 字段 data_type+column_type 断言（id/user_id/sync_date/client_total_steps/accepted_delta_steps/motion_state/source/client_ts）+ `updated_at` 列不存在断言 + PK = id 断言
  - `TestMigrateIntegration_StatusAfterUp`：version 期望从 5 → 6
- AC5：本 story **不**写独立 Go 单测；epics.md "≥3 单测 case" 由 AC4 集成测试覆盖（happy up / happy down 在 case 1，幂等 up 在 case 2，schema 元素验证在 case 3，version 验证在 case 4）—— 与 4.3 dev notes 测试策略一致
- AC6：验证清单 6 项见下表
- AC7：本 story dev-story 阶段不 commit，commit 由 epic-loop story-done sub-agent 收口

**AC6 验证清单**：

| # | 验证项 | 结果 |
|---|---|---|
| 1 | `0006_init_user_step_sync_logs.up.sql` CREATE TABLE 块逐字段对齐 §5.5 行 322-335 | ✅ PASS（9 列 + 2 索引 + ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 完全一致） |
| 2 | up.sql 顶部含 `-- 对齐 docs/宠物互动App_数据库设计.md §5.5 (行 317-359)` 注释 | ✅ PASS（与 0001-0005 风格一致） |
| 3 | down.sql 是单行 `DROP TABLE IF EXISTS user_step_sync_logs;` + 注释 | ✅ PASS（与 0005 down 风格一致） |
| 4 | `bash scripts/build.sh --test` 全绿 | ✅ PASS（22 包全过） |
| 5 | `bash scripts/build.sh --integration` 4 case 全 SKIP（本机 Windows docker 不可用） | ✅ PASS（与 4.3 graceful skip 模式一致；CI Linux 跑 docker 真验） |
| 6 | `git status --short` 仅 5 个文件命中预期范围 | ✅ PASS（2 SQL 新增 + 1 集成测试改 + 1 sprint-status 改 + 1 本 story 改 = 5/5 命中） |

**关键决策回顾**（不变 + 新落地）：

- DDL 严格按 §5.5（不擅自加列 / 不改类型 / 不动索引名 / 索引顺序）—— 例如 `accepted_delta_steps INT NOT NULL DEFAULT 0`（INT signed，**不** BIGINT，**不** UNSIGNED）；`sync_date DATE`（**不** DATETIME，**不** VARCHAR）；**没有** `updated_at` 列（日志表 append-only）；**没有** FK 约束
- Go 代码改动 = 0（仅新增 SQL + 扩集成测试断言）
- 集成测试 SKIP on Windows 是预期行为；4.3 已建立 graceful skip pattern 由本 story 复用

### File List

预期 5 个文件，全部命中：

- ✅ **新增**：`server/migrations/0006_init_user_step_sync_logs.up.sql`
- ✅ **新增**：`server/migrations/0006_init_user_step_sync_logs.down.sql`
- ✅ **修改**：`server/internal/infra/migrate/migrate_integration_test.go`（4 case 断言扩展到 6 张表 + 本表字段类型断言 + PK 断言 + 无 updated_at 断言）
- ✅ **修改**：`_bmad-output/implementation-artifacts/sprint-status.yaml`（7-2 状态 ready-for-dev → in-progress → review）
- ✅ **修改**：`_bmad-output/implementation-artifacts/7-2-user_step_sync_logs-migration.md`（本 story 文件，Tasks 全部 [x] / Status review / Dev Agent Record / File List / Change Log 落地）

### Change Log

| 日期 | 变更 | 备注 |
|---|---|---|
| 2026-05-02 | Story 7.2 ready-for-dev：context engine 分析完成；migration 0006_init_user_step_sync_logs.up/down 实装范围 + 集成测试 4 case 扩展断言 + 范围红线 + 与 4.3 / 7.1 衔接全部锚定 | bmad-create-story workflow，未 commit（dev 阶段未启动） |
| 2026-05-02 | Story 7.2 dev 完成：0006_init_user_step_sync_logs.up/down.sql 落地（严格对齐 §5.5）+ migrate_integration_test.go 扩展到 6 张表（4 case 全部更新）+ build/test/integration 全绿（integration 4 case 在本机 Windows 因 docker 不可用 SKIP，与 4.3 graceful skip 一致） | bmad-dev-story workflow，未 commit（epic-loop 流水线在下游 story-done 阶段统一收口） |
