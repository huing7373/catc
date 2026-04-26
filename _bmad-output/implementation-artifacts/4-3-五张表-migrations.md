# Story 4.3: 五张表 migrations

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want migration 工具 + 节点 2 需要的 5 张表 DDL,
so that `go run cmd/server/main.go migrate up` 能一键建库，并且每张表的字段 / 索引 / 唯一约束严格对齐 `docs/宠物互动App_数据库设计.md` §5（让 Story 4.6 游客登录初始化事务直接跑五张表的 INSERT，而不是边写 service 边补 DDL）。

## 故事定位（Epic 4 第三条 = 节点 2 server 写第一行 SQL DDL；上承 4.2 MySQL 接入，下启 4.6 五表事务）

- **Epic 4 进度**：4.1 (契约定稿，done) → 4.2 (MySQL 接入 + tx manager，done) → **4.3 (本 story，5 张表 migrations + migrate CLI)** → 4.4 (token util) → 4.5 (auth + rate_limit 中间件) → 4.6 (游客登录 handler + 首次初始化事务) → 4.8 (GET /home) → 4.7 (Layer 2 集成测试)。本 story 是 4.6 / 4.7 / 4.8 的**直接前置**：4.6 五表事务的 INSERT 要落到本 story 建好的表；4.7 Layer 2 集成测试用 dockertest 起容器后**第一步**就是跑 `migrate up` 才能 INSERT；4.8 GET /home 聚合查询读的是本 story 建的 users / pets / user_step_accounts / user_chests 字段。
- **epics.md AC 钦定**：`_bmad-output/planning-artifacts/epics.md` §Story 4.3（行 974-997）已**精确**列出 5 个 migration 文件名 + 必须可逆 + `migrate up/down/status` 子命令 + ≥3 单测 case + dockertest 集成测试 1 case。**注意：epics.md 钦定的文件名是 `0001_init_users.sql` 这种"无 .up./down. 后缀"的写法，但 ADR-0003 §3.2 + golang-migrate 工具规范要求 `.up.sql` / `.down.sql` 双向文件对**——本 story 按 ADR-0003 + golang-migrate 实际规范落地，文件命名为 `0001_init_users.up.sql` + `0001_init_users.down.sql`，与 epics.md 钦定的"含编号文件"语义一致（编号 / 表名一致），仅形式上多了 `.up`/`.down` 后缀。
- **ADR-0003 钦定**：
  - migration 工具用 `github.com/golang-migrate/migrate/v4 v4.18.1`（已在 4.2 通过 `internal/tools/tools.go` blank import pin 住，本 story 把它从"工具依赖"升级为"生产代码依赖"）
  - 纯 SQL 文件 + up/down 双向；**禁止**用 GORM AutoMigrate
  - migrations 目录在 `server/migrations/`（CLAUDE.md "节点 1 之后的目录形态" §"target" 已锚定）
- **下游立即依赖**：
  - **Story 4.6 (游客登录初始化事务)**：`AuthService.GuestLogin` 在 `txManager.WithTx(ctx, fn)` 内执行 5 个 repo 调用（user_repo.Create / auth_binding_repo.Create / pet_repo.Create / step_account_repo.Create / chest_repo.Create）→ 每个 repo 的 INSERT 必须命中本 story 建的表。表字段 / 默认值 / 唯一约束都不对，4.6 的事务测试会全部红。
  - **Story 4.7 (Layer 2 集成测试)**：dockertest 起 mysql:8.0 容器后**第一步**就是跑 `migrate up` 拿到 5 张空表，才能跑 100 goroutine 并发 / 回滚 / 边界场景。本 story 必须留可被测试代码调用的 Go API（不只是 CLI），让 `_test.go` 里能 `migrate.Up()` 而不是 shell out。
  - **Story 4.8 (GET /home 聚合接口)**：service 层一次性查 users + pets + user_step_accounts + user_chests + users.current_room_id；本 story 的字段名 / 类型 / 索引必须严格对齐数据库设计 §5.1 / §5.3 / §5.4 / §5.6，否则 GET /home 的 SELECT 在节点 2 末尾才发现 schema 不一致。
  - **Epic 7 / 11 / 14 / 17 / 20 / 23 / 26 / 29 / 32**：每个后续业务 epic 都会新增 1-3 个 migration 文件（如 7.2 user_step_sync_logs、11.2 rooms / room_members、17.2 emoji_configs、20.2 cosmetic_items 等），**全部复用本 story 建立的 migrations/ 目录 + 编号约定 + migrate CLI 子命令**。本 story 一次性把"如何加 migration"的范式（文件命名 / up.sql + down.sql 双文件 / 编号顺序 / 子命令调用）钉死，下游 epic 不再讨论 migration 实装方式。
- **范围红线**：本 story **只**新增 `server/migrations/` 目录（10 个 SQL 文件：5 张表各 up + down）+ migrate Go API 包装层 + `cmd/server/main.go` 的 `migrate {up|down|status}` 子命令 + 单测 + 集成测试。**不**实装任何业务 repo / 不改 service / 不改 handler / 不接 Redis / 不改 docs/。

**本 story 不做**（明确范围红线）：

- ❌ **不**写节点 2 之外的 5 张表（`user_step_sync_logs` / `chest_open_logs` / `cosmetic_items` / `user_cosmetic_items` / `user_pet_equips` / `compose_logs` / `compose_log_materials` / `rooms` / `room_members` / `emoji_configs` 这 10 张表分别由 Epic 7.2 / 20.4 / 20.2 / 23.2 / 26.2 / 32.2 / 11.2 / 17.2 落地；本 story **只**做 §5.1 / §5.2 / §5.3 / §5.4 / §5.6 五张）
- ❌ **不**实装任何 repo（首个 repo 是 Story 4.6 的 `user_repo.go` / `auth_binding_repo.go` / `pet_repo.go` / `step_account_repo.go` / `chest_repo.go`；本 story **不**碰 `internal/repo/mysql/`）
- ❌ **不**实装任何业务 handler / service（4.4-4.6 / 4.8 各自负责）
- ❌ **不**用 GORM AutoMigrate（ADR-0003 §3.2 强烈否决：生产不可控、无 down 路径、与"必须可逆"AC 冲突）
- ❌ **不**实装 GORM model struct（如 `User`、`Pet` 这种业务实体）—— GORM model 是 4.6 落地 repo 时才需要的；本 story 是纯 SQL DDL 文件 + migrate CLI
- ❌ **不**改 `docs/宠物互动App_*.md` 任一份文档（数据库设计 §5 是契约**输入**，本 story 严格对齐它但**不修改**它）
- ❌ **不**接 Redis（Epic 10.2 才接）
- ❌ **不**改 V1 接口契约（Story 4.1 已冻结）
- ❌ **不**新增 `auth.token_secret` 等配置（Story 4.4 / 4.5 才加）
- ❌ **不**给 migrate 子命令加自动恢复 / dry-run / force 等高级开关（保留最小集 up/down/status；高级开关后置 tech debt）
- ❌ **不**实装 server 启动时自动 migrate（"启动自动 migrate"是危险默认，迁移失败会让 server 假装启动；migrate 是**显式**的运维操作；CI 部署阶段单独 step 跑 `migrate up`，server 启动时只验 schema_migrations 表存在 + 当前版本 ≥ 期望版本即可，本 story **不**实装这条 verification —— 后置 Epic 36 部署 story）

## Acceptance Criteria

**AC1 — `server/migrations/` 目录建立 + 5 张表 up/down 双向 SQL**

新增目录 `server/migrations/`，含 10 个 SQL 文件（5 张表 × 2 方向），文件命名严格按 golang-migrate v4 规范 `{N}_{name}.{up|down}.sql`：

```
server/migrations/
├── 0001_init_users.up.sql
├── 0001_init_users.down.sql
├── 0002_init_user_auth_bindings.up.sql
├── 0002_init_user_auth_bindings.down.sql
├── 0003_init_pets.up.sql
├── 0003_init_pets.down.sql
├── 0004_init_user_step_accounts.up.sql
├── 0004_init_user_step_accounts.down.sql
├── 0005_init_user_chests.up.sql
└── 0005_init_user_chests.down.sql
```

**SQL 内容必须严格对齐 `docs/宠物互动App_数据库设计.md` §5 钦定的 DDL**（行号见下面"References"），逐字段 / 逐索引 / 逐唯一约束对齐：

- `0001_init_users.up.sql` ⟸ §5.1 (行 173-192)：`users` 表（`id` BIGINT UNSIGNED PK + `guest_uid` UNIQUE + `nickname` / `avatar_url` / `status` / `current_room_id` + `created_at` / `updated_at` DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3)）+ 索引 `uk_guest_uid` / `idx_current_room_id` / `idx_created_at`
- `0002_init_user_auth_bindings.up.sql` ⟸ §5.2 (行 210-227)：`user_auth_bindings` 表 + UNIQUE (auth_type, auth_identifier) + INDEX (user_id)
- `0003_init_pets.up.sql` ⟸ §5.3 (行 246-264)：`pets` 表 + UNIQUE (user_id, is_default) + INDEX (user_id)
- `0004_init_user_step_accounts.up.sql` ⟸ §5.4 (行 280-294)：`user_step_accounts` 表（注意 PK 是 `user_id` 而非 `id` —— 这张表是 1:1 关联 users 的"账户表"）+ `version` 乐观锁字段
- `0005_init_user_chests.up.sql` ⟸ §5.6 (行 362-380)：`user_chests` 表 + UNIQUE (user_id) + INDEX (status, unlock_at) + `version` 乐观锁

**对应 `.down.sql` 必须可逆**：每条 up 创建的表 / 索引 / 约束在 down 里 `DROP TABLE IF EXISTS` 干净（不留残留）。

**关键 SQL 语法约束**：

- 所有表 `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`（数据库设计 §3.4）
- 所有 `created_at` / `updated_at` 用 `DATETIME(3)`（毫秒精度，§3.2）+ `DEFAULT CURRENT_TIMESTAMP(3)` + `updated_at` 加 `ON UPDATE CURRENT_TIMESTAMP(3)`
- 所有主键 `BIGINT UNSIGNED`（§3.1，下发字符串由 service / handler 层负责，本 story 只关心 DB 层）
- 所有状态字段 `TINYINT NOT NULL` 含枚举注释（§3.3 + §6 状态枚举汇总）
- **明确不加 FK 约束**：MySQL 8.0 InnoDB 支持 FK，但项目惯例不依赖 FK 做引用完整性（业务层保证），与数据库设计 §5 的所有 DDL 一致（§5 全部 DDL **不**含 FOREIGN KEY 子句）
- **明确不加 deleted_at 字段**：节点 2 阶段 5 张表均**不需要** soft delete（数据库设计 §3.5 钦定 soft delete 是"未来扩展机制"，**节点 2 不强制** —— 现实是数据库设计 §5.1 / §5.2 / §5.3 / §5.4 / §5.6 钦定 DDL **没有 deleted_at 列**；本 story 严格对齐，不擅自加列）
- **每个 .up.sql 顶部加注释**：标注 "对齐 docs/宠物互动App_数据库设计.md §5.X 行 NNN-MMM"（让未来 schema 变更时容易找回原始钦定）

**关键约束 (反模式)**：

- ❌ **不**用 `IF NOT EXISTS` 在 .up.sql 的 `CREATE TABLE` —— golang-migrate 自带 schema_migrations 表防重复，up 一次只跑一次；用 `IF NOT EXISTS` 会掩盖"上一次 migrate 没干净"的状态污染
- ❌ **不**在 SQL 里 INSERT 任何业务数据（seed 数据是 Story 17.3 / 20.3 / 23.x / 29.3 才需要；本 story 5 张表全是空表）
- ❌ **不**改 .down.sql 的执行顺序 —— FK 约束本来就没加，但表之间逻辑依赖（user_auth_bindings.user_id 引 users.id）要求 down 时反向 DROP；本 story 每个 .down.sql 只 drop 自己表，由 migrate 工具按 0005 → 0001 倒序执行（migrate down 默认行为）

**AC2 — `server/internal/infra/migrate/` 包：Go API 包装层**

新增目录 `server/internal/infra/migrate/`，提供 Go API 包装 golang-migrate v4 / mysql driver，至少含：

```go
// Package migrate 提供 server 内部用的 migration Go API。封装 golang-migrate v4
// 让 cmd/server/main.go 子命令 + 集成测试都能直接调；不暴露 golang-migrate 的
// *Migrate 对象给上层（避免上层耦合具体工具）。
//
// 设计文档：ADR-0003 §3.2 钦定 golang-migrate v4 + 纯 SQL 文件双向 + CLI/Go API 双形态。
package migrate

type Migrator interface {
    // Up 把 schema 推到最新。已经是最新版本时返 ErrNoChange（migrate 标准语义）。
    Up(ctx context.Context) error
    // Down 把 schema 全回滚（删除所有 migration）。慎用，仅 dev / test 场景。
    Down(ctx context.Context) error
    // Status 返回当前 schema 版本（uint）+ 是否 dirty。
    Status(ctx context.Context) (version uint, dirty bool, err error)
    // Close 释放底层资源（migrate.Close 会关 source / driver 的连接）。
    Close() error
}

// New 构造 Migrator。dsn 是 MySQL 连接串（与 cfg.MySQL.DSN 同格式）；
// migrationsPath 是相对工作目录的 migrations 目录（本项目用 "server/migrations"）。
//
// 实装：内部用 migrate.New("file://"+migrationsPath, "mysql://"+dsn) 构造。
// 返回的 Migrator 必须在 Close 时释放底层资源（避免连接泄漏）。
func New(dsn, migrationsPath string) (Migrator, error) { ... }
```

**关键设计约束**：

- **不**让 migrate 包持有外层 `*gorm.DB`：migrate 工具自己管连接（用独立的 `mysql://` driver），与 4.2 的 `db.Open` 返回的 `*gorm.DB` 解耦。理由：(1) golang-migrate v4 期望自己的 driver instance 上锁（advisory lock 防并发 migrate）；(2) 测试场景灵活——集成测试可以用 dockertest 给的 DSN，CLI 场景用 cfg.MySQL.DSN，不需要先 db.Open 再传 gormDB
- **`Up()` 返回 `migrate.ErrNoChange` 时不当 error 处理**：`migrate.ErrNoChange` 是 "schema 已经是最新" 的正常状态。本 story 的 Up 实装内部 catch 这个 error 并 `return nil`（让 CLI / 测试调用方拿到 nil 表示成功）
- **`Status()` 返回 `(version=0, dirty=false, ErrNilVersion)` 时映射为 `(0, false, nil)`**：`ErrNilVersion` 是"还没跑过任何 migration"的状态——业务语义上是"version 0"。本 story 的 Status 实装内部 catch 这个 error 并返回 `(0, false, nil)`
- **Migrator 必须 ctx-aware**：所有公开方法第一参数 `ctx context.Context`（CLAUDE.md "ctx 必传"）。golang-migrate v4 的 Up/Down 内部不直接接 ctx，但本 story 的实装可以通过 goroutine + ctx.Done() select 提供"外层 cancel 时停止"的语义（细节由实装决定；最低要求：签名带 ctx，内部最少透传给可 ctx-aware 的部分）
- **Close 必须可重入**：CLI 短跑场景退出前会 Close，集成测试场景每个测试 case 都 Close，重复 Close 不应 panic / 报 error；用 `sync.Once` 包一层

**单测覆盖**（≥2 case，用 sqlmock 不现实——migrate 工具内部直接跑 source 文件不走 sqlmock 路径；所以本 AC 的"单测"实际上是验 New / Close 错误路径，真正的 up/down 单测放 AC4）：

- `TestNew_EmptyDSN_ReturnsError`：dsn = "" → 返 error 含 "dsn"
- `TestNew_InvalidPath_ReturnsError`：migrationsPath = "/nonexistent/path" → 返 error 含 "open" / "no such file"

**AC3 — `cmd/server/main.go`：`migrate {up|down|status}` 子命令**

修改 `server/cmd/server/main.go`，新增子命令解析：

```bash
# 当前（4.2 已实装）：
catserver -config configs/local.yaml          # 启动 HTTP server

# 本 story 新增：
catserver migrate up                           # 推 schema 到最新（exit 0 = 成功 / 非 0 = 失败）
catserver migrate down                         # 全回滚（dev / test only）
catserver migrate status                       # 打印当前 version + dirty 状态（exit 0 = 健康 / 1 = dirty）
catserver migrate up -config configs/dev.yaml  # 子命令也接 -config 参数
```

**关键实装约束**：

- **子命令解析放在 `flag.Parse` 之后** —— 现有 main.go 用 `flag.Parse` 拿 `-config`，本 story 在 `flag.Args()` 检查首个非 flag 参数是否为 `migrate`：
  ```go
  // 简化伪码
  flag.Parse()
  args := flag.Args()
  if len(args) >= 1 && args[0] == "migrate" {
      runMigrate(ctx, cfg, args[1:])  // args[1:] = ["up"] / ["down"] / ["status"]
      return  // 不进入 server 启动路径
  }
  // 没 migrate 子命令 → 走原 server 启动逻辑（4.2 已实装的 db.Open + tx.NewManager + bootstrap.Run）
  ```
- **migrate 子命令必须复用 cfg.MySQL.DSN** —— migrate up / down / status 都要连 MySQL，DSN 来源严格走 4.2 已实装的 config.Load 路径（含 CAT_MYSQL_DSN env 覆盖）。**禁止**给 migrate 子命令再开一个独立的 `-dsn` 参数（会与 4.2 的 cfg 路径形成 2 个 source of truth）
- **migrate 子命令不需要 db.Open 也不需要 tx.NewManager** —— 跳过 4.2 的 db.Open 直接进 `migrate.New(cfg.MySQL.DSN, "migrations")`；migrate 子命令成功 / 失败都 `os.Exit(0/1)` 立刻退出，不进 bootstrap.Run
- **`migrate up` 失败 / `migrate status` 显示 dirty** → `os.Exit(1)` 让 CI 能感知失败；正常完成 → `os.Exit(0)` 退出
- **`migrate status` 输出格式**：单行 `migrate: version=NN dirty=false` 或 `migrate: version=NN dirty=true`（dirty 时同时 stderr 打 WARN，方便运维 grep）。版本号是 golang-migrate 的 schema_migrations 表里的 `version` 列；新建库（没跑过 migrate）打 `migrate: version=0 dirty=false`
- **migrationsPath 解析**：默认 `migrations`（相对当前工作目录）—— 用户在 `server/` 下跑 `catserver migrate up` 时找 `server/migrations`；测试 / 容器场景可通过 env `CAT_MIGRATIONS_PATH` 覆盖（loader 不挂这个 env，仅 main.go 子命令分支内 `os.Getenv` 直接读 —— 范围控在 migrate 子命令而不是全局 cfg）
- **slog.Info 打两条**：`migrate started action=up` / `migrate finished action=up version=NN dirty=false`，便于运维排障
- **不**在 server 主启动路径调 migrate（不接管"启动自动 migrate"——危险默认）；migrate 是**显式**的运维操作

**关键反模式**：

- ❌ **不**用第三方子命令库（cobra / urfave-cli）—— 节点 2 阶段保持 stdlib `flag` + 手写 args 解析，依赖最小；引入子命令库是 epic-level 工程决定，本 story 不做
- ❌ **不**给 migrate 子命令加 confirm prompt（"are you sure?"）—— 自动化场景（CI / 容器 init）不能交互；危险性靠"手敲 `migrate down` 才会跑 down" + "default 不在 prod 跑 down" 政策保证（本 story 不实装环境守护，留给 Epic 36）

**AC4 — 单元测试覆盖（≥3 case，本 AC 主要落 Go API 层 + main.go 子命令解析层；DDL 内容的真实跑通在 AC5 集成测试）**

按 epics.md §Story 4.3 钦定 "≥3 case" 落地：

- **AC4.1 `internal/infra/migrate/migrate_test.go`**（Migrator Go API 单测，≥2 case）：
  - `TestNew_EmptyDSN_ReturnsError`：dsn = "" → 返 error 含 "dsn"
  - `TestNew_InvalidMigrationsPath_ReturnsError`：migrationsPath = "/nonexistent/path/xxx" → 返 error 含 "source" / "open" / "no such file"
  - `TestClose_DoubleClose_NoError`：构造 Migrator → Close 两次（第一次 nil error；第二次也 nil error，验证 sync.Once 包了重入）。**注意**：构造一个能 Close 的 Migrator 需要走真实路径 —— 用 dockertest 不现实（这是单测）；用 sqlite/in-memory driver 构造 mock Migrator 需要嵌套抽象层。**最简实装**：把 Migrator interface 的 Close 行为直接在自己实装里用 sync.Once 包，`TestClose_DoubleClose_NoError` 写一个不依赖底层 migrate.Migrate 实例的 unit test（直接构造 `migrator{closed: false, closeOnce: sync.Once{}}` 然后调 2 次 Close）—— 设计上让 `migrator` struct 的 Close 即使 underlying migrate.Migrate 是 nil 也不 panic
- **AC4.2 `cmd/server/main_test.go` 或独立子包**（main.go 子命令解析单测，≥1 case）：
  - `TestParseSubcommand_MigrateUp`：mock os.Args = `["catserver", "migrate", "up"]` → 解析成 `subcommand="migrate", action="up"`（不真跑 migrate）
  - **注意**：main.go 当前是 `package main`，外部包测试需要把子命令解析逻辑抽到一个可测的内部函数（如 `internal/cli/migrate.go` 暴露 `RunMigrate(ctx, cfg, args) error`），让 main.go 只做 args 透传 + os.Exit。**这是本 story 的设计选择**：抽出 `internal/cli/migrate.go` 包，main.go 调 `cli.RunMigrate(...)`，单测对 `cli.RunMigrate` 的子命令分发逻辑（不真连 MySQL，可注入 mock Migrator 验证调用了 Up/Down/Status 哪个方法）。该抽象层把"参数解析 + Migrator 调用 + exit code 决策"集中到一个可测的 internal 包。
  - **进一步说明**：实装时 `RunMigrate(ctx, cfg, args, mig Migrator) (exitCode int, err error)`——把 Migrator 作为参数注入而非内部 New，让单测可以传 mock；main.go 调用方做 `mig, _ := migrate.New(...)` 后传给 RunMigrate
  - 单测 case：(1) `args=["up"]` + mock Migrator.Up 返 nil → exit=0；(2) `args=["up"]` + mock Migrator.Up 返 error → exit=1；(3) `args=["status"]` + mock Migrator.Status 返 (1, false, nil) → exit=0 + stdout 含 "version=1 dirty=false"；(4) `args=["status"]` + 返 (1, true, nil) → exit=1（dirty 视为 unhealthy）；(5) `args=["unknown"]` → exit=2 + stderr 含 "unknown migrate action"

**总数**：AC4.1 (3) + AC4.2 (5) = ≥8 case，满足 epics.md "≥3 case" 下限的 2 倍以上

**关键约束**：

- 全部用 `testing.T` + 标准 testify/assert（已有依赖）；**不**额外引入 mock 框架
- 全部 ≤ 100ms / case 跑完
- mock Migrator 用纯接口实现（5-10 行 Go），不依赖 mockgen / mockery

**AC5 — 集成测试（dockertest 起 mysql:8.0 + 真跑 5 张表 DDL）**

新增 `server/internal/infra/migrate/migrate_integration_test.go`（build tag `//go:build integration`），覆盖 epics.md 钦定的"happy: migrate up 后五张表存在 + 字段类型 + 索引 + 唯一约束都符合 §5 / happy: migrate down 后表全部删除 / edge: 重复 migrate up → 幂等不报错 / 集成测试 SHOW CREATE TABLE 对比 §5 schema → migrate down → 表为空"：

```go
//go:build integration
// +build integration

package migrate_test

func TestMigrateIntegration_UpThenDown(t *testing.T) { ... }              // happy 1
func TestMigrateIntegration_UpTwice_Idempotent(t *testing.T) { ... }     // edge 1（幂等）
func TestMigrateIntegration_TablesPresent_AfterUp(t *testing.T) { ... }  // happy 2（5 张表存在 + 字段验证）
func TestMigrateIntegration_StatusAfterUp(t *testing.T) { ... }          // edge 2（Status 返回 version=5）
```

**`TestMigrateIntegration_TablesPresent_AfterUp` 必须验证**（这是 epics.md 钦定 "字段类型 + 索引 + 唯一约束都符合 §5"）：

- `INFORMATION_SCHEMA.TABLES` 查 5 张表全部存在（`users` / `user_auth_bindings` / `pets` / `user_step_accounts` / `user_chests`）
- `SHOW CREATE TABLE users` 输出含 `BIGINT unsigned`（id 列）+ `varchar(128)` (guest_uid) + `UNIQUE KEY \`uk_guest_uid\``
- `SHOW CREATE TABLE user_auth_bindings` 输出含 `UNIQUE KEY \`uk_auth_type_identifier\` (\`auth_type\`,\`auth_identifier\`)` + `KEY \`idx_user_id\``
- `SHOW CREATE TABLE pets` 输出含 `UNIQUE KEY \`uk_user_default_pet\` (\`user_id\`,\`is_default\`)` + `KEY \`idx_user_id\``
- `SHOW CREATE TABLE user_step_accounts` 输出主键是 `PRIMARY KEY (\`user_id\`)` 而非 auto-increment id（这张表 PK 就是 user_id）
- `SHOW CREATE TABLE user_chests` 输出含 `UNIQUE KEY \`uk_user_id\` (\`user_id\`)` + `KEY \`idx_status_unlock_at\` (\`status\`,\`unlock_at\`)`
- 每张表都用 `INFORMATION_SCHEMA.COLUMNS` 抽样验关键字段类型（`created_at` `datetime(3)` / `version` `bigint unsigned` 等），不需要 100% 字段比对（脆性测试），但必须验证：
  - 所有 `created_at` / `updated_at` 类型 = `datetime(3)`
  - 所有主键和外键引用列类型 = `bigint unsigned`
  - 所有状态字段类型 = `tinyint`
  - 字符串字段长度对齐 §5（如 `users.guest_uid` = 128，`users.nickname` = 64）

**`TestMigrateIntegration_UpThenDown` 实装**：起容器 → migrate Up → INFORMATION_SCHEMA 查到 5 张表 → migrate Down → INFORMATION_SCHEMA 查到 5 张表全消失（仅剩 `schema_migrations` 表，那是 migrate 工具自己的）

**`TestMigrateIntegration_UpTwice_Idempotent` 实装**：migrate Up → migrate Up（第 2 次应不报错，返 nil；底层 `migrate.ErrNoChange` 已被本 story 的 Up 实装吞掉）→ 表数量与第一次一致

**`TestMigrateIntegration_StatusAfterUp` 实装**：起容器 → migrate Up → Status 返回 `(version=5, dirty=false, nil)`（5 个 migration 都跑完）

**关键约束**：

- dockertest 起 mysql:8.0 容器复用 4.2 集成测试的 `startMySQL(t)` helper 模式（**不**写新 helper —— 把 4.2 的 helper 抽到 `internal/infra/db/dockertest_helpers_test.go` 或同等共享位置 supports 跨包用）。**最简方案**：本 story 在 `internal/infra/migrate/` 包内复制一份 `startMySQL` helper（接受测试间的少量重复），避免新建跨包 testing util 的范围扩散；4.2 已有的 `mysql_integration_test.go` 不动
- docker 不可用 → `t.Skip("docker not available")`，不让 CI 阻塞
- `bash scripts/build.sh --integration` 跑通 4 个新 case + 4.2 已有 4 个 case = 8 case 全过
- 集成测试用的 DSN 必须含 `multiStatements=true` —— golang-migrate 默认期望 driver 支持多语句（4.2 helper 已加，本 story 复用）

**AC6 — `bash scripts/build.sh` 全量绿（vet + build + test + integration）**

完成后必须能跑通：

```bash
bash scripts/build.sh                    # vet + build → 不报错
bash scripts/build.sh --test             # ≥8 case 单测全过
bash scripts/build.sh --race --test      # 加 -race 也全过（本机 Windows ThreadSanitizer 不可用 → skip 走 ADR-0001 §3.5；CI / Linux 必须过）
bash scripts/build.sh --integration      # 8 个集成测试 case（4.2 旧 4 + 4.3 新 4）全过；docker 不可用时 t.Skip
```

**关键约束**：

- 单测层 ≤ 100ms / case 总时长，整体 `go test ./...` < 5s
- 集成测试 docker 启动慢，预期 2-5 分钟跑完 4 个新 case（每个 case ~30-60s mysql:8.0 冷启）
- 全程**不**改 `bash scripts/build.sh` 自身（脚本契约由 Story 1.7 钉死，不在本 story 动）

**AC7 — `cmd/server/main.go` 复用 4.2 启动顺序，子命令分支不破坏现有逻辑**

修改 `cmd/server/main.go` 时严格按以下顺序：

```go
func main() {
    logger.Init("info")                          // 已有
    flag.StringVar(&configPath, ...)             // 已有
    flag.Parse()                                 // 已有

    // 解析 config 路径（已有逻辑）
    if configPath == "" { ... LocateDefault ... }
    cfg, err := config.Load(configPath)          // 已有
    logger.Init(cfg.Log.Level)                    // 已有

    // ★ 本 story 新增：检查子命令
    args := flag.Args()
    if len(args) >= 1 && args[0] == "migrate" {
        ctx, stop := signal.NotifyContext(...)
        defer stop()
        if err := cli.RunMigrate(ctx, cfg, args[1:]); err != nil {
            slog.Error("migrate failed", slog.Any("error", err))
            os.Exit(1)
        }
        os.Exit(0)
    }

    // 没 migrate 子命令 → 走 4.2 已实装的原路径（db.Open + tx.NewManager + bootstrap.Run）
    if devtools.IsEnabled() { ... }              // 已有
    ctx, stop := signal.NotifyContext(...)       // 已有
    defer stop()
    dbOpenCtx, dbOpenCancel := context.WithTimeout(ctx, dbOpenTimeout)
    gormDB, err := db.Open(dbOpenCtx, cfg.MySQL)  // 已有
    ...
}
```

**关键约束**：

- 子命令分支必须**在 4.2 的 db.Open 之前**——migrate 子命令自己管 MySQL 连接（用 golang-migrate 的 driver），不该被 db.Open 失败阻塞（如 schema 不存在时 Open 也会 ping fail，连 migrate up 都没法跑）
- 子命令分支也必须**在 logger.Init(cfg.Log.Level) 之后**——migrate 期间的 slog 输出走用户配置的 level
- 子命令分支用**自己的** signal-ctx（migrate 操作支持 SIGINT cancel）；不复用 4.2 的 dbOpenCtx 短 timeout（migrate up 跑 5 个 SQL 文件可能耗时 1-30s，5s timeout 不够）
- main.go 改动行数控制在 ~20 行内；复杂逻辑全部抽到 `internal/cli/migrate.go` 包

**AC8 — `internal/cli/` 包：子命令分发 + Migrator 注入**

新增 `server/internal/cli/migrate.go`：

```go
// Package cli 实装 catserver 的子命令分发逻辑。当前仅 migrate；未来可扩展（dev /
// dump-schema / etc）。从 main.go 抽出来是为了让单测可对子命令分发逻辑做表驱动覆盖
// （main.go 是 package main 难以单测）。
package cli

type Migrator interface { ... }  // 与 internal/infra/migrate.Migrator 同签名（避免 main.go 间接 import 路径）

// RunMigrate 是 catserver migrate {up|down|status} 子命令的入口。
//
// args = ["up"] / ["down"] / ["status"]；其他值返 error("unknown migrate action: %s")。
//
// 错误返回时调用方（main.go）打 slog.Error + os.Exit(1)；status 返回 dirty=true 时也返 error
// （让 CI 能感知 schema 处于 dirty 状态）。
func RunMigrate(ctx context.Context, cfg *config.Config, args []string) error {
    if len(args) < 1 {
        return errors.New("migrate requires action: up / down / status")
    }
    migrationsPath := os.Getenv("CAT_MIGRATIONS_PATH")
    if migrationsPath == "" {
        migrationsPath = "migrations"
    }
    mig, err := migrate.New(cfg.MySQL.DSN, migrationsPath)
    if err != nil {
        return fmt.Errorf("migrate.New: %w", err)
    }
    defer mig.Close()
    return runMigrateAction(ctx, mig, args[0])
}

// runMigrateAction 是可被 mock 注入测试的核心分发函数（接 Migrator 接口而非
// 具体类型，让单测注入 fake）。
func runMigrateAction(ctx context.Context, mig Migrator, action string) error {
    switch action {
    case "up": return mig.Up(ctx)
    case "down": return mig.Down(ctx)
    case "status":
        version, dirty, err := mig.Status(ctx)
        if err != nil { return err }
        slog.Info("migrate status", slog.Uint64("version", uint64(version)), slog.Bool("dirty", dirty))
        if dirty {
            return fmt.Errorf("schema is dirty at version %d (manual fix required)", version)
        }
        return nil
    default:
        return fmt.Errorf("unknown migrate action: %s (expected up / down / status)", action)
    }
}
```

**关键约束**：

- `cli.Migrator` 接口与 `infra/migrate.Migrator` 同签名（**不**直接 import migrate 包到 cli 包里——通过 interface 解耦让单测可注入）。`RunMigrate` 内部 `migrate.New(...)` 创建实装并 type-assert 为 `cli.Migrator`（GO 自动满足，因签名一致）
- `runMigrateAction` 接口注入是**纯**单测点：单测构造 fake Migrator（5-10 行）调 runMigrateAction，验证分发逻辑 + exit code 映射，**不**起 docker
- `cli.RunMigrate` 不能直接被 unit test（它内部 New 真实 Migrator 连 MySQL）；这条路径走 AC5 集成测试覆盖

**AC9 — `go.mod` 状态确认 + `internal/tools/tools.go` 升级**

- 确认 `server/go.mod` `require` 段已含 `github.com/golang-migrate/migrate/v4 v4.18.1`（4.2 已加，本 story 应不变 require 行；但需要新增 source / driver indirect 依赖 —— migrate 工具默认 import path `migrate/v4` **不**带 file source / mysql driver；本 story 必须显式 import：
  - `github.com/golang-migrate/migrate/v4/source/file`（file source，读 `file://` URL）
  - `github.com/golang-migrate/migrate/v4/database/mysql`（mysql driver，处理 `mysql://` URL）
- `internal/infra/migrate/migrate.go` 在 `import` 块加：
  ```go
  _ "github.com/golang-migrate/migrate/v4/source/file"
  _ "github.com/golang-migrate/migrate/v4/database/mysql"
  ```
- **删除 `internal/tools/tools.go`** —— 4.2 用 tools.go 把 golang-migrate "占位" pin 住；本 story 把它升级为生产代码 import（`internal/infra/migrate/migrate.go` 直接 import），tools.go 占位文件就不再需要。**注意**：删除前确认 4.2 引入的其他工具依赖不在 tools.go pin 列表里（应该没有，4.2 tools.go 只 pin migrate）；删除后 `go mod tidy` 不会剔 migrate（因为 migrate 包正在被生产代码 import）
- `cd server && go mod tidy` 完成后 commit `go.sum`；`go.mod` 应不增不减 require 行（仅可能新增 source/file 和 database/mysql 这两个 sub-package 但它们都是 migrate v4 主 module 内的子包，无独立 require）

**关键约束**：

- **不**升级 `golang-migrate v4` 主版本（保持 v4.18.1 与 ADR-0003 钦定一致）
- **不**新增任何外部依赖（不引入 cobra / urfave-cli / viper 等子命令 / 配置框架）
- 删除 tools.go 时如担心 future tooling 失去占位机制，可保留一个空文件 `internal/tools/tools.go` 含 `//go:build tools` + 空 package（不 import 任何东西） —— **本 story 的实装选择直接删除**（最干净），如果未来某 story 需要 pin 工具依赖再恢复模板

**AC10 — README / docs 不更新**

本 story **不**更新：

- `README.md` / `server/README.md`：节点 1 README 已说明"MySQL 在 Epic 4 接入"；migrate 子命令的运维指南由 Epic 4 收尾 / Epic 36 部署 story 统一写；本 story 不抢工作
- `docs/宠物互动App_*.md` 任一份：本 story 严格对齐数据库设计 §5，**消费方**不是修改方
- `docs/lessons/` 任一份：本 story 不主动写 lesson；如 review 阶段发现新坑，由 fix-review 阶段 lesson 写入（epic-loop 流水线职责分工）

**关键约束**：

- 如果 dev 阶段实装时发现某条 AC 与文档存在冲突 / 漏洞 / 暗坑（如 §5 钦定字段类型与 GORM mapping 不兼容），**不**自行修文档，**而是**在 Completion Notes 里登记 issue + 让 fix-review 阶段处理

## Tasks / Subtasks

- [x] **Task 1（AC1）**：写 5 张表 up + down SQL 文件
  - [x] 1.1 新建目录 `server/migrations/`
  - [x] 1.2 写 `0001_init_users.up.sql` 严格对齐数据库设计 §5.1（包括 `uk_guest_uid` / `idx_current_room_id` / `idx_created_at` 三个索引）；顶部注释引用源行号
  - [x] 1.3 写 `0001_init_users.down.sql`：`DROP TABLE IF EXISTS users`
  - [x] 1.4 写 `0002_init_user_auth_bindings.up.sql` + `.down.sql`（按 §5.2，含 UNIQUE (auth_type, auth_identifier) + INDEX (user_id)）
  - [x] 1.5 写 `0003_init_pets.up.sql` + `.down.sql`（按 §5.3，含 UNIQUE (user_id, is_default) + INDEX (user_id)）
  - [x] 1.6 写 `0004_init_user_step_accounts.up.sql` + `.down.sql`（按 §5.4，**主键是 user_id 不是 id**，含 `version` 乐观锁字段）
  - [x] 1.7 写 `0005_init_user_chests.up.sql` + `.down.sql`（按 §5.6，含 UNIQUE (user_id) + INDEX (status, unlock_at) + `version` 乐观锁）
  - [x] 1.8 每个 .up.sql 顶部 `-- 对齐 docs/宠物互动App_数据库设计.md §5.X (行 NNN-MMM)` 注释
- [x] **Task 2（AC2）**：实装 `internal/infra/migrate/migrate.go`
  - [x] 2.1 新建 `server/internal/infra/migrate/migrate.go` 定义 `Migrator` interface + `migrator` struct + `New(dsn, path)` 工厂
  - [x] 2.2 import 加 `_ "github.com/golang-migrate/migrate/v4/source/file"` + `_ "github.com/golang-migrate/migrate/v4/database/mysql"`
  - [x] 2.3 实装 `Up(ctx)`：内部调 `m.migrate.Up()`；`migrate.ErrNoChange` 吞掉返 nil；其他 error wrap 透传
  - [x] 2.4 实装 `Down(ctx)`：内部调 `m.migrate.Down()`；`migrate.ErrNoChange` 吞掉返 nil
  - [x] 2.5 实装 `Status(ctx)`：内部调 `m.migrate.Version()`；`migrate.ErrNilVersion` 吞掉返 `(0, false, nil)`；其他 error 透传
  - [x] 2.6 实装 `Close()`：用 `sync.Once` 包 `m.migrate.Close()` 防重入
  - [x] 2.7 godoc 注释（语义、错误路径、ctx 透传策略、与 4.2 db.Open 解耦的理由）
- [x] **Task 3（AC2 / AC4.1）**：写 `internal/infra/migrate/migrate_test.go`
  - [x] 3.1 `TestNew_EmptyDSN_ReturnsError`
  - [x] 3.2 `TestNew_InvalidMigrationsPath_ReturnsError`
  - [x] 3.3 `TestClose_DoubleClose_NoError`（构造 `migrator{closeOnce: sync.Once{}}` 用一个不依赖底层 migrate.Migrate 的 path 验证 sync.Once 包了重入语义；可能需要让 `migrator.migrate` 字段是接口而非具体类型，或者用 `if m.migrate != nil` 防御）
- [x] **Task 4（AC8 / AC4.2）**：实装 `internal/cli/migrate.go`
  - [x] 4.1 新建 `server/internal/cli/migrate.go` 定义 `Migrator` interface + `RunMigrate(ctx, cfg, args)` 入口 + `runMigrateAction(ctx, mig, action)` 分发函数
  - [x] 4.2 实装 `RunMigrate`：检查 args 非空 → 读 `CAT_MIGRATIONS_PATH` env / 默认 "migrations" → `migrate.New(...)` → defer Close → 调 runMigrateAction
  - [x] 4.3 实装 `runMigrateAction`：switch action → 调 mig.Up/Down/Status；status dirty 时返 error
  - [x] 4.4 godoc 注释（分发逻辑 / 错误路径 / 为什么抽出 runMigrateAction 让单测可注入）
- [x] **Task 5（AC4.2）**：写 `internal/cli/migrate_test.go`
  - [x] 5.1 定义 `fakeMigrator` struct 实装 cli.Migrator interface（5-10 行）
  - [x] 5.2 `TestRunMigrateAction_UpSuccess`：fake.Up 返 nil → runMigrateAction 返 nil
  - [x] 5.3 `TestRunMigrateAction_UpFailure`：fake.Up 返 error → runMigrateAction 返 同 error
  - [x] 5.4 `TestRunMigrateAction_DownSuccess`：fake.Down 返 nil → runMigrateAction 返 nil
  - [x] 5.5 `TestRunMigrateAction_StatusClean`：fake.Status 返 (5, false, nil) → runMigrateAction 返 nil
  - [x] 5.6 `TestRunMigrateAction_StatusDirty`：fake.Status 返 (5, true, nil) → runMigrateAction 返 error 含 "dirty"
  - [x] 5.7 `TestRunMigrateAction_UnknownAction`：action="foo" → 返 error 含 "unknown"
- [x] **Task 6（AC3 / AC7）**：修改 `cmd/server/main.go` 加 migrate 子命令分支
  - [x] 6.1 在 `flag.Parse()` + `config.Load` + `logger.Init(cfg.Log.Level)` 之后插 args 检查
  - [x] 6.2 args[0] == "migrate" → 走子命令分支：`signal.NotifyContext` + `cli.RunMigrate(ctx, cfg, args[1:])` + `os.Exit(0/1)`
  - [x] 6.3 args[0] != "migrate" 或 args 为空 → 走原 server 启动路径（4.2 已实装的 db.Open + bootstrap.Run）
  - [x] 6.4 跑 `bash scripts/build.sh` 确认编译通过
  - [x] 6.5 手工烟测：`build/catserver migrate status` 在没起 MySQL 时**应该 fail-fast 报错** "migrate.New: ... connection refused"（验证子命令真正连了 MySQL），exit code 1
- [x] **Task 7（AC5）**：写集成测试（dockertest 起真实 mysql:8.0）
  - [x] 7.1 新建 `server/internal/infra/migrate/migrate_integration_test.go` 加 `//go:build integration` build tag
  - [x] 7.2 复制 4.2 的 `startMySQL(t)` helper 到本文件（或抽到 dockertest helper 文件按 4.2 风格）
  - [x] 7.3 `TestMigrateIntegration_UpThenDown`：起容器 → migrate Up → INFORMATION_SCHEMA 查 5 张表存在 → migrate Down → 5 张表全消失（仅 schema_migrations）
  - [x] 7.4 `TestMigrateIntegration_UpTwice_Idempotent`：连续两次 Up 都不报错；表数不变
  - [x] 7.5 `TestMigrateIntegration_TablesPresent_AfterUp`：Up → 抽样验关键索引 + 关键字段类型 + 主键约束（如 user_step_accounts 主键 = user_id）
  - [x] 7.6 `TestMigrateIntegration_StatusAfterUp`：Up → Status 返 (version=5, dirty=false, nil)
  - [x] 7.7 docker 不可用 → `t.Skip`；DSN 必须含 `multiStatements=true` 防 migrate 多语句失败
- [x] **Task 8（AC9）**：调整依赖 + 删除 tools.go 占位
  - [x] 8.1 `internal/infra/migrate/migrate.go` 已 import migrate v4 主包 + source/file + database/mysql 子包
  - [x] 8.2 删除 `server/internal/tools/tools.go`（migrate 已在生产代码 import，不再需要 blank import 占位）
  - [x] 8.3 `cd server && go mod tidy` + `go mod verify`；确认 require 段还有 `github.com/golang-migrate/migrate/v4 v4.18.1`（不被 tidy 剔除）
- [x] **Task 9（AC6）**：全量验证
  - [x] 9.1 `bash scripts/build.sh`（vet + build） — 必过
  - [x] 9.2 `bash scripts/build.sh --test` — ≥8 case 单测全过
  - [x] 9.3 `bash scripts/build.sh --race --test` — Linux / CI 必过；本机 Windows 如果 ThreadSanitizer 失败按 ADR-0001 §3.5 备注 skip 不阻塞
  - [x] 9.4 `bash scripts/build.sh --integration` — 8 个集成测试 case（4.2 旧 4 + 4.3 新 4）全过；docker 不可用时 t.Skip
  - [x] 9.5 `git status --short` 抽检：本 story 影响的文件清单与 AC 范围对齐
- [x] **Task 10**：本 story 不做 git commit
  - [x] 10.1 epic-loop 流水线约束：dev-story 阶段不 commit；由下游 fix-review / story-done sub-agent 收口
  - [x] 10.2 commit message 模板（story-done 阶段使用）：

    ```text
    feat(migrations): 5 张表 migrations + migrate CLI 子命令（Story 4.3）

    - server/migrations/ 加 5 个 up/down SQL 文件对（users / user_auth_bindings /
      pets / user_step_accounts / user_chests），严格对齐数据库设计 §5.1/5.2/5.3/5.4/5.6
    - internal/infra/migrate/ 包装 golang-migrate v4 → Migrator interface（Up/Down/Status/Close）
    - internal/cli/migrate.go：catserver migrate {up|down|status} 子命令分发
    - cmd/server/main.go 加 args[0]=="migrate" 子命令分支（在 db.Open 之前）
    - 单测 8 case + 集成测试 4 case（dockertest 起 mysql:8.0 验 SHOW CREATE TABLE）
    - 删除 internal/tools/tools.go 占位（migrate 已在生产代码 import）

    依据 epics.md §Story 4.3 + ADR-0003 §3.2 + docs/宠物互动App_数据库设计.md §5。

    Story: 4-3-五张表-migrations
    ```

## Dev Notes

### 关键设计原则

1. **DDL 字面量必须严格对齐数据库设计 §5**：每张表的字段名 / 类型 / 默认值 / 索引名 / 唯一约束都不能擅自改。`docs/宠物互动App_数据库设计.md` §5 是**契约输入**，本 story 是其**第一次落地**——schema 漂移会在 Epic 7 / 11 / 14 ... 业务实装时持续返工。具体到本 story：DDL **不**用 GORM AutoMigrate 自动生成（生成结果会偷加列 / 索引），**手写**纯 SQL 让 review 阶段能逐字段核对 §5。
2. **fail-fast over auto-migrate**：server 启动时**不**自动跑 migrate up（"启动自动 migrate" 的 footgun：迁移失败让 server 假装健康；多实例并发启动时 race 抢锁；rollback 路径不可控）。migrate 是**显式**运维操作，靠 `catserver migrate up` 子命令在 CI / 部署阶段单独 step 执行。Epic 36 部署 story 落地完整流水线时 server 启动可加"version check"（schema_migrations 当前版本 ≥ 期望版本则启动；否则 fail）但本 story **不**实装。
3. **golang-migrate 工具与 4.2 的 GORM 解耦**：migrate 工具自己开 connection 用 advisory lock，**不**复用 4.2 的 `*gorm.DB`。理由：(1) golang-migrate v4 期望 driver instance 上锁；(2) migrate 子命令场景不需要走 4.2 的 fail-fast db.Open（若 schema 还不存在，db.Open 的 ping 会失败让 migrate 都跑不起来——子命令分支必须在 db.Open 之前）；(3) 测试场景灵活——集成测试用 dockertest 给 DSN 直接 New Migrator，CLI 场景用 cfg.MySQL.DSN，不需要 db.Open 中介。
4. **抽出 `internal/cli/` 包让 main.go 可测**：main.go 是 `package main` 难以单测。抽出子命令分发到 `internal/cli/` 包后，main.go 只做"args 解析 + signal-ctx + RunMigrate + os.Exit"，逻辑骨架 ~10 行；复杂分发 + Migrator 注入全在 `internal/cli/migrate.go` 可被单测覆盖。这是 ADR-0001 §3.4 testify/mock + 接口边界 mock 模式的延续。
5. **WithTx 不参与 migrate**：migrate 工具有自己的 BEGIN / COMMIT 语义（每个 up.sql 是一个事务），与 4.2 的 `txManager.WithTx` 是两回事。本 story 的 cli / migrate 包**不** import `internal/repo/tx`（避免误用）。

### 架构对齐

**领域模型层**（`docs/宠物互动App_总体架构设计.md` §3）：
- 节点 2 阶段所有数据持久化在 MySQL，本 story 的 5 张表是节点 2 端到端业务需要的**最小集**：游客身份（users + user_auth_bindings）+ 默认猫（pets）+ 步数账户（user_step_accounts）+ 当前宝箱（user_chests）

**数据库层**（`docs/宠物互动App_数据库设计.md`）：
- §3.1 主键策略：所有表 PK = `BIGINT UNSIGNED AUTO_INCREMENT`（user_step_accounts 例外，PK = user_id）
- §3.2 时间字段：DATETIME(3) + DEFAULT CURRENT_TIMESTAMP(3) + ON UPDATE
- §3.3 状态字段：TINYINT
- §3.4 索引：UNIQUE 用 `uk_xxx` 前缀，普通 INDEX 用 `idx_xxx` 前缀（§5 钦定的索引名都符合此规范，本 story 严格沿用）
- §5.1-5.6：5 张表的 DDL 字面量是 source of truth；本 story 是其**首次实装**

**服务端架构层**（`docs/宠物互动App_Go项目结构与模块职责设计.md`）：
- §4 项目目录建议：`migrations/` 目录已列在目录树（行 116-121，含示例文件名 `0001_init_users.sql`）；本 story 在 `server/migrations/` 落地，文件命名按 golang-migrate 规范加 `.up`/`.down` 后缀
- §5.5 Infrastructure 层：`internal/infra/db/` 是 MySQL 连接（4.2 已实装），本 story 在同层加 `internal/infra/migrate/`（migrate 工具包装层）

### 测试策略

按 ADR-0001 §3.1 + 4.2 已建立的测试范式：

- **单测层**（`migrate_test.go` + `cli/migrate_test.go`）：用纯 fake Migrator + sync.Once 验证；不起容器，不触实际 MySQL
- **集成测试层**（`migrate_integration_test.go` + `//go:build integration`）：用 dockertest 起真实 mysql:8.0，跑真 SQL DDL；只在 `bash scripts/build.sh --integration` 触发；CI 上跑

**关键决策**：本 story 单测**不**用 sqlmock。原因：sqlmock 模拟的是 `database/sql` driver 层；golang-migrate 的 source/file driver 直接读文件 + database/mysql driver 走 sql.Open，sqlmock 介入路径很深，得不偿失。本 story 单测策略：上层逻辑（Migrator 接口分发 / cli 包 action 分发）用纯 fake；底层 SQL 跑通靠 dockertest。

### 启动顺序约束

按 4.2 已建立的"两步 init"+ 本 story 的子命令分支：

```
main()
├─ logger.Init("info")                # bootstrap logger（已有）
├─ flag.Parse()                        # 已有
├─ config.LocateDefault                # 已有
├─ config.Load                         # 已有
├─ logger.Init(cfg.Log.Level)          # 用户配置 logger（已有）
│
├─ if args[0] == "migrate":           # ★ 本 story 新增：子命令分支
│   ├─ signal.NotifyContext           # 子命令独立 ctx
│   ├─ cli.RunMigrate(ctx, cfg, args) # 内部 migrate.New + Up/Down/Status
│   └─ os.Exit(0/1)                    # 立刻退出
│
├─ if devtools.IsEnabled() ...          # 已有
├─ signal.NotifyContext                 # 主 server ctx（已有）
├─ db.Open(dbOpenCtx, cfg.MySQL)        # ★ 4.2 实装；migrate 子命令不走这里
├─ defer sqlDB.Close()                   # 已有
├─ tx.NewManager(gormDB)                 # 已有
└─ bootstrap.Run(ctx, cfg, gormDB, txMgr) # 已有
```

**关键**：migrate 子命令分支必须**早于** db.Open—— schema 还不存在时 db.Open 会 ping 失败，migrate 子命令永远跑不起来。这是"先建库再启动 server"的物理依赖。

### 与已 done 的 4.2 的衔接

4.2 实装：
- `internal/infra/db/mysql.go`：`Open(ctx, cfg) (*gorm.DB, error)`
- `internal/repo/tx/manager.go`：`Manager` interface + `WithTx(ctx, fn)` + `FromContext(ctx, fallback)`
- `internal/infra/config/config.go` + `loader.go`：`MySQLConfig` + env override `CAT_MYSQL_DSN`
- `internal/tools/tools.go`：blank import pin `golang-migrate v4.18.1`（占位，本 story 删除）

本 story **不动** 4.2 实装，**复用**：
- `cfg.MySQL.DSN`：migrate 子命令直接用，复用 env override 路径
- `golang-migrate v4.18.1`：从 tools.go 占位升级为生产代码 import

本 story **新增**与 4.2 解耦的 path：
- 子命令分支跳过 db.Open / tx.NewManager —— migrate 自己开连接
- `internal/infra/migrate/` 包不 import `internal/infra/db` 或 `internal/repo/tx` —— 解耦让单测 / 集成测试更灵活

### 与下游 4.6 的接口

4.6 落地时会做：
1. 假设 DB 已有 5 张空表（运维 / 测试调过 `migrate up`）
2. `repo/mysql/user_repo.go` 实装 `Create(ctx, user) error`，INSERT 命中 `users` 表
3. 同样 4 个 repo 命中 `user_auth_bindings` / `pets` / `user_step_accounts` / `user_chests`
4. `service/auth_service.go` 在 `txManager.WithTx(ctx, fn)` 内串 5 个 repo 调用
5. 集成测试在 dockertest 容器内：先 `migrate.Up(ctx)` 建表 → 再 `service.GuestLogin(ctx, ...)`

**本 story 必须保证 4.6 能直接用**：
- 5 张表的字段都按 §5 钦定，4.6 的 repo 不会找不到字段
- migrate Go API 在测试场景可注入：4.6 / 4.7 的集成测试 setUp 阶段调 `migrator.Up(ctx)` 而不是 shell out 跑 CLI

### Project Structure Notes

预期文件 / 目录变化：

- ✅ **新增**：`server/migrations/` 目录（10 个 SQL 文件）
- ✅ **新增**：`server/internal/infra/migrate/migrate.go` + `migrate_test.go` + `migrate_integration_test.go`
- ✅ **新增**：`server/internal/cli/migrate.go` + `migrate_test.go`
- ✅ **修改**：`server/cmd/server/main.go`（加 args[0] == "migrate" 子命令分支）
- ✅ **修改**：`server/go.mod` + `server/go.sum`（migrate v4 主版本不变；新增 source/file + database/mysql 子包 import 不算 require 行变化）
- ✅ **删除**：`server/internal/tools/tools.go`（migrate 已在生产代码 import，不再需要占位）
- ✅ **修改**：`_bmad-output/implementation-artifacts/sprint-status.yaml`（4-3-五张表-migrations: backlog → ready-for-dev → in-progress → review；由 dev-story 流程内推动）
- ✅ **修改**：`_bmad-output/implementation-artifacts/4-3-五张表-migrations.md`（本 story 文件，dev 完成后填 Tasks/Dev Agent Record/File List/Completion Notes）

不影响其他目录：

- ❌ `server/internal/infra/db/` 不变（4.2 已实装，本 story 不动）
- ❌ `server/internal/repo/tx/` 不变（4.2 已实装）
- ❌ `server/internal/repo/mysql/` 不变（4.6 才落地第一个 user_repo.go）
- ❌ `server/internal/service/` / `server/internal/app/http/handler/` 不变（业务在 4.4-4.6 / 4.8 落地）
- ❌ `server/configs/local.yaml` 不变（mysql 段 4.2 已加，无新字段）
- ❌ `server/internal/infra/config/` 不变（无新配置项）
- ❌ `iphone/` / `ios/` 不变（server-only story）
- ❌ `docs/宠物互动App_*.md` 全部 7 份不变（消费方）
- ❌ `docs/宠物互动App_V1接口设计.md` 不变（4.1 已冻结契约；本 story 不触发契约变更）
- ❌ README.md / server/README.md 不变（Epic 4 / Epic 36 收尾才统一更新）

### golang-migrate v4 关键 API 提示

实装时注意（避免常见坑）：

- `migrate.New("file://"+path, "mysql://"+dsn)` 接受**两个 URL 字符串**，第一个是 source（migration 文件位置），第二个是 database driver。`file://` 是相对当前工作目录的相对路径——必须是文件 URL 不是 OS 路径
- `migrate/v4` 主包**不**自动 import file source 和 mysql driver——必须显式 blank import：
  ```go
  _ "github.com/golang-migrate/migrate/v4/source/file"
  _ "github.com/golang-migrate/migrate/v4/database/mysql"
  ```
  忘了导致 `migrate.New` 返 error "unknown driver" / "source not found"
- DSN 必须加 `multiStatements=true` 让 driver 接受多语句（一个 .up.sql 文件可能含多条 SQL，如 CREATE TABLE + ALTER TABLE）。本项目 DDL 都是单 CREATE TABLE，但**仍建议加** `multiStatements=true` 避免后续扩展踩坑
- `migrate.ErrNoChange`：schema 已经是最新版本时 Up / Down 会返这个 error；业务语义上是"成功（无操作）"，本 story 的 Up / Down 实装应**吞掉**返 nil
- `migrate.ErrNilVersion`：还没跑过任何 migration 时 Version() 返这个 error；业务语义上是"version = 0"，本 story 的 Status 实装应**吞掉**返 `(0, false, nil)`
- `migrate.ErrLocked`：另一个进程正在跑 migrate（advisory lock）；本 story **不**特殊处理，让 error 透传给 CLI（用户看到 "database is locked" 等几秒重试即可）
- `migrate.Close()`：会同时关 source 和 database 两个 driver，单次调用，重复调可能 panic / nil pointer——本 story 用 `sync.Once` 包

### References

- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 4.3 (行 974-997)] — 本 story 的钦定 AC 来源（5 个 migration 文件 + 必须可逆 + migrate up/down/status 子命令 + ≥3 单测 + dockertest 集成测试）
- [Source: `_bmad-output/planning-artifacts/epics.md` §Epic 4 Overview (行 927-931)] — 节点 2 第一个业务 epic / 执行顺序 4.1 → 4.2 → **4.3** → 4.4 → ...
- [Source: `_bmad-output/implementation-artifacts/decisions/0003-orm-stack.md` §3.2 (行 87-103)] — golang-migrate v4 选定 + 否决 AutoMigrate / goose / atlas 的理由 + 版本 pin v4.18.1
- [Source: `_bmad-output/implementation-artifacts/decisions/0003-orm-stack.md` §4.3 (行 184)] — Story 4.3 的预期 follow-up：5 张表的 up/down SQL + 编号约定
- [Source: `docs/宠物互动App_数据库设计.md` §5.1 (行 173-207)] — `users` 表 DDL 钦定：guest_uid VARCHAR(128) UNIQUE / current_room_id BIGINT UNSIGNED NULL / 三个索引
- [Source: `docs/宠物互动App_数据库设计.md` §5.2 (行 210-243)] — `user_auth_bindings` 表 DDL：UNIQUE (auth_type, auth_identifier) + INDEX (user_id) + auth_extra JSON 字段
- [Source: `docs/宠物互动App_数据库设计.md` §5.3 (行 246-277)] — `pets` 表 DDL：UNIQUE (user_id, is_default) + INDEX (user_id) + current_state TINYINT
- [Source: `docs/宠物互动App_数据库设计.md` §5.4 (行 280-313)] — `user_step_accounts` 表 DDL：**主键 = user_id 不是 id**（1:1 关联 users）+ version 乐观锁
- [Source: `docs/宠物互动App_数据库设计.md` §5.6 (行 362-395)] — `user_chests` 表 DDL：UNIQUE (user_id) + INDEX (status, unlock_at) + version 乐观锁 + open_cost_steps INT UNSIGNED
- [Source: `docs/宠物互动App_数据库设计.md` §3 全表设计原则 (行 73-167)] — 主键 / 时间 / 状态 / 索引命名 / 字符集统一规则；本 story 所有 DDL 必须遵守
- [Source: `docs/宠物互动App_数据库设计.md` §6 状态枚举汇总 (行 726-826)] — TINYINT 状态字段的取值含义；本 story 在 SQL 注释里标注每个状态字段的枚举值（让 SQL 自描述）
- [Source: `docs/宠物互动App_数据库设计.md` §8.1 游客登录初始化事务 (行 880-895)] — Story 4.6 五表事务的目标使用场景；本 story 是其前置（建空表）
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 项目目录建议 (行 116-121)] — `migrations/` 目录已列在目录树，含 `0001_init_users.sql` 等示例文件名（本 story 按 golang-migrate 实际规范加 `.up`/`.down` 后缀）
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §5.5 Infrastructure 层 (行 319-340)] — `internal/infra/` 是不含业务的 thin 层；本 story 的 `internal/infra/migrate/` 严格遵守该层职责（只包装 golang-migrate，不掺业务逻辑）
- [Source: `_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.1 (行 42-53)] — sqlmock + dockertest 双层测试策略；本 story 沿用（单测纯 fake / 集成测试 dockertest）
- [Source: `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md`] — ctx 必传；本 story 所有公开方法第一参数必须是 ctx
- [Source: `_bmad-output/implementation-artifacts/4-2-mysql-接入.md`] — 上一 story；本 story 复用 4.2 的 cfg.MySQL.DSN + golang-migrate v4.18.1 pin；删除 4.2 的 tools.go 占位
- [Source: `docs/lessons/2026-04-26-config-env-override-and-gorm-auto-ping.md`] — 4.2 review lesson：infrastructure 接入必须配齐 env override（本 story 复用 4.2 已加的 CAT_MYSQL_DSN，**不**新增 env override；migrate 子命令的 CAT_MIGRATIONS_PATH env 是局部开关不入 cfg.Config）
- [Source: `docs/lessons/2026-04-26-startup-blocking-io-needs-deadline.md`] — 启动阶段阻塞 IO 必须有 deadline；本 story 的 migrate 子命令分支用自己的 signal-ctx（不复用 4.2 的 dbOpenCtx 5s timeout，因为 migrate up 可能耗时几十秒）
- [Source: `CLAUDE.md` §"工作纪律"] — "节点顺序不可乱跳 / 资产类操作必须事务 / 状态以 server 为准 / ctx 必传"；本 story 是节点 2 第三条 server story 严格按顺序推
- [Source: `CLAUDE.md` §"节点 1 之后的目录形态 (target)"] — `migrations/` 目录已锚定在 `server/migrations/` 顶层（不在 `internal/infra/` 下，因为 SQL 文件不是 Go 代码）
- [Source: `MEMORY.md` "No Backup Fallback"] — 反对 fallback 掩盖核心风险；本 story 的 migrate 子命令失败必须 os.Exit(1)，不容忍部分成功

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m] (Anthropic Opus 4.7, 1M context)

### Debug Log References

- `bash scripts/build.sh --test` — 全绿（17 个包，包括新加的 `internal/cli` + `internal/infra/migrate`）
- `bash scripts/build.sh --integration` — 17 个包绿；migrate 集成测试 4 case 全部 SKIP（本机 Windows 无 docker daemon，按 4.2 同模式 graceful skip）
- `bash scripts/build.sh --race --test` — Windows 平台 ThreadSanitizer 内存分配失败（`failed to allocate ... bytes`），按 ADR-0001 §3.5 + Story 9.3 钦定的"本机 Windows skip 不阻塞"处理；Linux / CI 上 race 必过
- 烟测 `build/catserver.exe migrate status`（无 MySQL 启动）→ exit code 1 + 结构化 ERROR 日志含 "dial tcp 127.0.0.1:3306: connectex: No connection could be made"，验证 fail-fast 路径正确

### Completion Notes List

- ✅ **Task 1 (AC1)**：5 张表 up/down SQL 文件落地 `server/migrations/`，每张严格对齐 §5.1/5.2/5.3/5.4/5.6 字段类型 / 索引 / 唯一约束；每个 .up.sql 顶部带 `-- 对齐 docs/...` 锚定注释
- ✅ **Task 2-3 (AC2 / AC4.1)**：`internal/infra/migrate/migrate.go` 实装 Migrator interface（Up/Down/Status/Close）+ migrator struct；ErrNoChange 在 Up/Down 吞掉，ErrNilVersion 在 Status 吞掉返 (0, false, nil)；Close 用 sync.Once 重入安全；7 个单测 case（DSN 空 / path 空 / path 不存在 / Close 重入 / nil migrator 防 panic 三个分支）
- ✅ **Task 4-5 (AC8 / AC4.2)**：`internal/cli/migrate.go` 实装 RunMigrate 入口 + runMigrateAction 分发函数；CAT_MIGRATIONS_PATH env 覆盖默认 "migrations"；status dirty=true 时返 error 让 CI 感知；9 个单测 case 覆盖 up/down/status (clean/dirty/error) / 未知 action / nil cfg / 空 args
- ✅ **Task 6 (AC3 / AC7)**：`cmd/server/main.go` 加 args[0]=="migrate" 子命令分支，**严格放在 db.Open 之前**（schema 不存在场景 fail-fast 需要）；用独立 signal-ctx（不复用 4.2 的 5s dbOpenCtx，因 migrate 可能耗时几十秒）
- ✅ **Task 7 (AC5)**：`migrate_integration_test.go` 加 `//go:build integration` tag + 4 个 case（UpThenDown / UpTwice_Idempotent / TablesPresent_AfterUp / StatusAfterUp）；复制 4.2 的 startMySQL helper（避免跨包 testing util 范围扩散）；docker 不可用 → t.Skip
- ✅ **Task 8 (AC9)**：删除 `internal/tools/tools.go` 占位（migrate 已在生产代码 import）+ `internal/tools/` 空目录；`go mod tidy` 后 `golang-migrate/migrate/v4 v4.18.1` 仍在 go.mod require 段（被 internal/infra/migrate 生产 import）
- ✅ **Task 9 (AC6)**：全量验证 — vet+build 绿 / 单测全绿 / 集成测试本机 skip / race 在 Windows skip 按 ADR-0001
- ✅ **Task 10**：本 story 不 commit（流水线由 fix-review / story-done 阶段收口）

**关键决策点**：

1. **错误 wrap 双层 prefix**：初版 `cli.RunMigrate` 写成 `fmt.Errorf("migrate.New: %w", err)`，但 migrate 包 New 内部已经 wrap 一层 `migrate.New: ...` → 实际输出 "migrate.New: migrate.New: ..." 重复。修正：cli 层直接透传 migrate.New 的 error（注释说明）
2. **migrate 子命令路径解析**：CAT_MIGRATIONS_PATH env 仅 cli 包内部消费（`os.Getenv` 直接读），**不**入 cfg.Config 全局 —— 范围控在 migrate 子命令而非全局 cfg，对齐 4.2 review lesson "infrastructure 接入必须配齐 env override 但局部开关不污染 cfg"
3. **集成测试用绝对路径锚定 migrations 目录**：`filepath.Abs("../../../migrations")` 从测试文件位置向上推到 `server/migrations`；不依赖 `os.Chdir` 或 cwd 假设，避免不同 IDE / 命令行调用方式 fail
4. **不抽 startMySQL 跨包 helper**：4.2 已有 db/mysql_integration_test.go::startMySQL，本 story 选择复制一份到 migrate 包内（接受测试间少量重复），避免新建 `internal/pkg/testing/dockertest_helpers.go` 这种跨包 testing util，范围扩散风险更可控
5. **Migrator interface 复制定义**：`cli.Migrator` 与 `internal/infra/migrate.Migrator` 同签名；故意不让 cli import migrate.Migrator —— 让 runMigrateAction 单测注入 fake 时不需要 import 整个 migrate 包及其 source/file / database/mysql 子包
6. **范围红线全部遵守**：
   - 未实装任何 model / repo（4.6 才做）
   - 未触碰 `docs/宠物互动App_*.md`
   - 未实装启动自动 migrate（"启动自动 migrate" 是 footgun，由 Epic 36 部署阶段实装 schema_migrations version check）
   - 未引入 cobra / urfave-cli 子命令框架（保持 stdlib flag + 手写 args 解析）

### File List

**新增（migrations/）**：
- `server/migrations/0001_init_users.up.sql`
- `server/migrations/0001_init_users.down.sql`
- `server/migrations/0002_init_user_auth_bindings.up.sql`
- `server/migrations/0002_init_user_auth_bindings.down.sql`
- `server/migrations/0003_init_pets.up.sql`
- `server/migrations/0003_init_pets.down.sql`
- `server/migrations/0004_init_user_step_accounts.up.sql`
- `server/migrations/0004_init_user_step_accounts.down.sql`
- `server/migrations/0005_init_user_chests.up.sql`
- `server/migrations/0005_init_user_chests.down.sql`

**新增（internal/infra/migrate/）**：
- `server/internal/infra/migrate/migrate.go`
- `server/internal/infra/migrate/migrate_test.go`
- `server/internal/infra/migrate/migrate_integration_test.go`

**新增（internal/cli/）**：
- `server/internal/cli/migrate.go`
- `server/internal/cli/migrate_test.go`

**修改**：
- `server/cmd/server/main.go`（加 cli import + args[0]=="migrate" 子命令分支，~15 行新增）
- `server/go.mod` / `server/go.sum`（go mod tidy 后 dhui/dktest + otel 等 indirect 依赖更新；require 段 `golang-migrate/migrate/v4 v4.18.1` 不变）

**删除**：
- `server/internal/tools/tools.go`（4.2 的 blank import 占位升级为生产代码 import 后已不需要）
- `server/internal/tools/`（空目录被 git 自动忽略）

**Story 状态文件**：
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（4-3-五张表-migrations: ready-for-dev → in-progress → review）
- `_bmad-output/implementation-artifacts/4-3-五张表-migrations.md`（本 story 文件，Tasks 全打 [x] / Status: review）

### Change Log

| Date | Change | By |
|---|---|---|
| 2026-04-26 | 落地 5 张表 up/down SQL（§5.1/5.2/5.3/5.4/5.6）+ migrate Go API 包装层 + cli 子命令分发 + 8 个单测 + 4 个集成测试；删除 tools.go 占位；状态推到 review | Claude (Opus 4.7) |
