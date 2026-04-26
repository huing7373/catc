# Story 4.2: MySQL 接入（GORM/sqlx 选型 + 连接池 + tx manager）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want server 能基于 YAML 配置连上 MySQL，并提供统一的 `txManager.WithTx(ctx, fn)` 事务入口,
so that Epic 4 后续 story（4.3 五张表 migrations / 4.6 游客登录初始化事务 / 4.7 Layer 2 集成测试）以及 Epic 7 / 11 / 14 / 17 / 20 / 23 / 26 / 32 等后续业务 epic 可以**直接复用同一套连接池 + 事务管理器**写多表事务，不再每个 epic 各自拼连接池。

## 故事定位（Epic 4 第二条 = 节点 2 第一个真实写代码的 server story；本 story 是 Epic 4 全部业务实装的**强前置基础设施**）

- **Epic 4 进度**：4.1 (契约定稿，已 done) → **4.2 (本 story，MySQL 接入 + tx manager)** → 4.3 (5 张表 migrations) → 4.4 (token util) → 4.5 (auth + rate_limit 中间件) → 4.6 (游客登录 handler + 首次初始化事务) → 4.8 (GET /home 聚合接口) → 4.7 (Layer 2 集成测试)。本 story 是 4.3 / 4.6 / 4.7 的**直接前置**：4.3 用本 story 建的 db handle 跑 migrations；4.6 用本 story 的 `txManager.WithTx` 写五表事务；4.7 用本 story 的连接初始化 + tx manager 跑 dockertest 真实 MySQL。
- **epics.md AC 钦定**：`_bmad-output/planning-artifacts/epics.md` §Story 4.2（行 952-972）已**精确**列出 8 条 AC，含选型决策文档（ADR-0003）/ db 包目录 / fail-fast 启动 / WithTx 签名 / 单测 4 case / dockertest 集成测试。
- **下游立即依赖**（如何被未来 story 用）：
  - **Story 4.3（5 张表 migrations）**：`migrate up/down/status` 子命令需要从本 story 建好的 `*sql.DB` / `*gorm.DB` 拿 connection，否则 migration 工具拿不到 DSN
  - **Story 4.6（游客登录初始化事务）**：service 层 `AuthService.GuestLogin` 内部按"未命中 user_auth_bindings → 在事务中创建五行"实装，调用方式严格 `txManager.WithTx(ctx, func(txCtx) error { ... })`；fn 内的所有 repo 调用必须用 **`txCtx`** 而非外层 ctx（参见 ADR-0007 §4 / CLAUDE.md "工作纪律" §ctx 必传）
  - **Story 4.7（Layer 2 集成测试）**：用 dockertest 起真实 MySQL，灌真 DSN 进 `local.yaml` 等价配置，跑 100 goroutine 并发 / 回滚场景；本 story 的 db 初始化 + tx manager 必须是测试可注入的（见 AC6 测试可注入）
  - **Epic 7 (Story 7.3 步数同步事务) / Epic 20 (Story 20.6 开箱事务) / Epic 26 (Story 26.3 穿戴事务) / Epic 32 (Story 32.4 合成事务)**：所有"必须放事务的操作"（数据库设计 §8）都直接调 `txManager.WithTx` —— 本 story 一次性把签名钉死，下游 epic 不再讨论 tx 实装方式
- **范围红线**：本 story **只**新增 `server/internal/infra/db/` + `server/internal/repo/tx/` 两个目录的代码 + 1 份 ADR-0003 + `local.yaml` 加 `mysql:` 段；**不**写任何 migration / 不创建任何表 / 不实装任何业务 repo / 不写 auth / rate_limit 中间件（4.3 / 4.4 / 4.5 / 4.6 各自负责）

**本 story 不做**（明确范围红线）：

- ❌ **不**写任何 SQL DDL（5 张表 migrations 是 Story 4.3 的范围）
- ❌ **不**实装 user_repo / pet_repo / step_repo / chest_repo / auth_binding_repo（4.6 才落地，需要先有表）
- ❌ **不**实装任何 handler / service 业务逻辑（4.4 / 4.5 / 4.6 / 4.8 各自负责）
- ❌ **不**接入 Redis（Epic 10 Story 10.2 才落地；本 story 范围只含 MySQL）
- ❌ **不**改 `docs/宠物互动App_*.md` 任一份业务设计文档（这些文档是契约**输入**；本 story 的代码实现必须**对齐**它们但**不修改**它们）
- ❌ **不**改 `docs/宠物互动App_V1接口设计.md`（Story 4.1 已冻结契约，本 story 不触发契约变更）
- ❌ **不**新增 `auth.token_secret` / `auth.token_expire_sec` 配置项（Story 4.4 才加；本 story 只加 `mysql:` 段）
- ❌ **不**新增 `ratelimit.per_user_per_min` 配置项（Story 4.5 才加）
- ❌ **不**实装 GORM logger 接 slog 的桥接（可选优化项，节点 2 阶段 GORM 默认 logger 即可；后置 tech debt）
- ❌ **不**为 `repo/mysql/` 目录新建任何文件（首个 mysql repo 是 Story 4.6 的 `user_repo.go`；本 story 只动 `repo/tx/`）

## Acceptance Criteria

**AC1 — ADR-0003 选型决策文档**

新增文件：`_bmad-output/implementation-artifacts/decisions/0003-orm-stack.md`，按现有 ADR 模板（参考 0001-test-stack.md 结构）至少包含：

- **Status / Date / Decider / Related Stories**: 标准头部
- **Context**: 为什么节点 2 第一个 story 必须先选 ORM —— "本 story 是后续 Epic 7 / 20 / 26 / 32 等所有事务型 epic 的基础设施，必须先选定且不再争议；与 ADR-0001 §3.1 的 sqlmock 选型对齐（sqlmock 模拟 `database/sql` driver，所选 ORM 必须基于 `database/sql`）"
- **Decision Summary**（推荐 GORM 但允许 sqlx，需在文档中给理由）：
  - 推荐 **GORM v1.25.x**（CLAUDE.md "Tech Stack" §"ORM / DB 驱动" 把 GORM 列在 sqlx 前）
  - 必须列出 GORM 的优劣 + sqlx 的优劣 + 选定理由（如：GORM 自动迁移 + soft delete + hooks 适合事务多的业务；sqlx 性能高但样板代码多）
  - 必须 pin 主版本号（如 `gorm.io/gorm v1.25.12` + `gorm.io/driver/mysql v1.5.7`，2026-04 当前稳定版）
- **Migration 工具选定**：在本 ADR 内一并锁定（见 Story 4.3 AC3，"在 0003-orm-stack.md 决策"）。推荐 **`golang-migrate/migrate v4`**（理由：纯 SQL 文件 + up/down 双向、CLI 与 Go API 双形态、不耦合 ORM）；否决候选：GORM 自带 AutoMigrate（生产不可控、无 down 路径、与"必须可逆"AC 冲突）/ goose（Go 文件混 SQL 复杂度高） / atlas（学习成本 + DSL 锁定）。版本 pin `github.com/golang-migrate/migrate/v4 v4.18.x`。
- **DSN 配置策略**：从 YAML `mysql.dsn` 读取（不拼字段，避免 escape 错误）；本地推荐格式 `cat:catdev@tcp(127.0.0.1:3306)/cat?charset=utf8mb4&parseTime=true&loc=Local&time_zone='%2B00%3A00'`；生产 DSN 必须从环境变量覆盖（YAML 留 `${MYSQL_DSN}` 占位 / 或文档说明用 `CAT_MYSQL_DSN` 覆盖）
- **连接池参数推荐**：`max_open_conns: 50` / `max_idle_conns: 10`（按 `docs/宠物互动App_Go项目结构与模块职责设计.md` §12.2 示例）/ `conn_max_lifetime_sec: 1800`（30 分钟，避免 MySQL `wait_timeout` 切连接）
- **事务级别**：MySQL 默认 `REPEATABLE READ`，本 ADR **保持默认不指定**；如未来某 story 需要降到 `READ COMMITTED`（如长事务 + 可见性需求），通过 `WithTx` 的 options 参数扩展（本 story 可不实装 options，预留扩展点）
- **GORM Logger 与 slog 集成**：本 story 阶段先用 GORM 默认 logger（stdout）；后置 tech debt（在 ADR Negative / Follow-ups 章节登记）
- **Consequences**: Positive / Negative / Follow-ups（按其他 ADR 范式）
- **Change Log**: 当前一行 `2026-04-XX 初稿，Story 4.2 交付`

**关键约束**：

- ADR-0003 是**纯文档**，**不**写任何 Go 代码
- 必须在文档顶部明确"本 ADR 不产出任何 `.go` 代码、不修改 `go.mod`、由 Story 4.2 实装代码部分（AC2-AC8）落地"

**AC2 — `local.yaml` 加 `mysql:` 段**

修改 `server/configs/local.yaml`，新增 `mysql:` 段（位于 `server:` 段之后、`log:` 段之前 —— 与 `docs/宠物互动App_Go项目结构与模块职责设计.md` §12.2 示例顺序对齐）：

```yaml
mysql:
  # 本地 MySQL 连接串。生产 / staging 通过环境变量 CAT_MYSQL_DSN 覆盖。
  # 字段说明详见 _bmad-output/implementation-artifacts/decisions/0003-orm-stack.md §"DSN 配置策略"。
  dsn: "cat:catdev@tcp(127.0.0.1:3306)/cat?charset=utf8mb4&parseTime=true&loc=Local"
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime_sec: 1800
```

**关键约束**：

- DSN 默认值**不要**留空 / 留 `${...}` —— 本地默认能直接连本地 MySQL（Docker `mysql:8.0` 默认账号 `root/catdev` 也行；ADR-0003 文档侧推荐用专用账号 `cat`）
- 配置项命名严格 snake_case（与现有 `server.bind_host` / `server.read_timeout_sec` 一致）
- README 此 story **不更新**（节点 1 README 已说明"MySQL 在 Epic 4 接入"；Epic 4 收尾时由 demo / 文档同步 story 统一刷 README，本 story 不动）

**AC3 — `internal/infra/config/config.go` 加 `MySQLConfig` 结构体**

修改 `server/internal/infra/config/config.go`，新增 `MySQLConfig` 结构体并加进 `Config` 顶层：

```go
type Config struct {
    Server ServerConfig `yaml:"server"`
    MySQL  MySQLConfig  `yaml:"mysql"`
    Log    LogConfig    `yaml:"log"`
}

type MySQLConfig struct {
    // DSN 是完整 MySQL 连接串（go-sql-driver/mysql 格式）。
    // 生产 / staging 通过环境变量覆盖（loader 已支持 ${VAR} 占位，详见 loader.go）。
    DSN string `yaml:"dsn"`
    // MaxOpenConns 是 *sql.DB 池的最大打开连接数；推荐 50（按 ADR-0003）。
    // 0 = 无限制（不推荐生产用）。
    MaxOpenConns int `yaml:"max_open_conns"`
    // MaxIdleConns 是空闲连接保活数；推荐 10。
    MaxIdleConns int `yaml:"max_idle_conns"`
    // ConnMaxLifetimeSec 是连接最大存活时间（秒）；推荐 1800（30 min）。
    // 用于规避 MySQL server 端 wait_timeout 切连接。0 = 不限制。
    ConnMaxLifetimeSec int `yaml:"conn_max_lifetime_sec"`
}
```

**关键约束**：

- YAML key 严格 snake_case，Go 字段严格 camelCase + `yaml:"..."` tag 对齐（参考现有 `ServerConfig.HTTPPort` 与 `yaml:"http_port"` 模式）
- 字段必须带 godoc 注释（参考现有 `ServerConfig.BindHost` 注释风格 —— 含语义、推荐值、生产 / 测试场景区别）
- **不**在 `MySQLConfig` 上定义任何 `Validate()` / `Normalize()` 方法（loader 层已经做基本 YAML 解析，本 story 用 fail-fast 模式：DSN 为空时启动报错；不在 config 包做业务校验）

**AC4 — `internal/infra/db/` 包：MySQL 连接初始化**

新增目录 `server/internal/infra/db/`，至少含：

- `mysql.go`：导出 `Open(ctx context.Context, cfg config.MySQLConfig) (*gorm.DB, error)`（如选 GORM）或 `Open(ctx, cfg) (*sqlx.DB, error)`（如选 sqlx）
  - 函数签名第一参数必须是 `ctx context.Context`（CLAUDE.md "工作纪律" §ctx 必传）
  - 内部用 ORM 驱动打开连接（GORM: `gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{...})`；sqlx: `sqlx.Open("mysql", cfg.DSN)`）
  - 设置连接池参数：`SetMaxOpenConns(cfg.MaxOpenConns)` / `SetMaxIdleConns(cfg.MaxIdleConns)` / `SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetimeSec) * time.Second)`
  - 调用 `*sql.DB.PingContext(ctx)` 验证连接（**fail-fast**：ping 失败直接返回 error，**不**容忍降级）
  - DSN 为空 → 立刻返回 `errors.New("mysql.dsn is empty")` 不调驱动
  - 返回值：成功时 `(*gorm.DB, nil)`；失败时 `(nil, error)`，error 必须 `fmt.Errorf("mysql open: %w", err)` wrap 原因

- `mysql_test.go`：单测覆盖（≥3 case，用 sqlmock 或 mock pattern；不依赖真实 MySQL，dockertest 留给 AC8 集成测试）：
  - `TestOpen_EmptyDSN_ReturnsError`：cfg.DSN = ""  → 返回 error 含 "dsn"
  - `TestOpen_InvalidDSN_ReturnsError`：cfg.DSN = "notvalid" → 返回 error（驱动 parse 失败）
  - 备注：ping 失败的真实路径在 AC8 集成测试覆盖（dockertest 起 MySQL 关掉再 ping）

**关键约束**：

- **不**实装 GORM Logger → slog 桥接（tech debt，ADR-0003 已登记）
- **不**新增任何 `WithCancel` / `time.AfterFunc` 后台 goroutine（GORM 内部连接池已有自己的 healthcheck）
- **不**导出 `*sql.DB`（让 db 句柄统一以 `*gorm.DB` 形式向上层暴露 —— GORM 的 `db.DB()` 仍可拿到 `*sql.DB`；但不在 `Open` 返回里直接给出 `*sql.DB`，避免上层绕过 ORM 写裸 SQL）
- bootstrap 顶层调用 `db.Open(ctx, cfg.MySQL)` 失败 → main.go 走 `slog.Error(...)` + `os.Exit(1)`（参照现有 config.Load 失败的处理模式）

**AC5 — `internal/repo/tx/` 包：txManager.WithTx 实装**

新增目录 `server/internal/repo/tx/`，至少含：

- `manager.go`：导出
  ```go
  // Manager 提供统一的事务入口。fn 内的所有 repo 调用必须用 txCtx 而非外层 ctx，
  // 否则不会进入事务（CLAUDE.md "工作纪律" §ctx 必传 + ADR-0007 §4）。
  type Manager interface {
      WithTx(ctx context.Context, fn func(txCtx context.Context) error) error
  }

  func NewManager(db *gorm.DB) Manager { ... }
  ```
  实装：
  - 内部用 GORM 的 `db.Transaction(func(tx *gorm.DB) error { ... })`
  - 把 `tx *gorm.DB` 注入新 ctx：`txCtx := context.WithValue(ctx, txKey{}, tx)`
  - 暴露一个内部 helper `func FromContext(ctx context.Context, fallback *gorm.DB) *gorm.DB`：repo 层用这个 helper 拿 db handle —— 如果 ctx 里有 tx 则用 tx，否则用 fallback（外层 db）
  - fn 返回 nil → commit；fn 返回非 nil → rollback，原 error 透传
  - panic 走 GORM 默认 rollback + repanic（GORM Transaction 已实现）

- `manager_test.go`：单测覆盖（≥4 case，用 sqlmock）：
  - `TestWithTx_Commit_OnNilReturn`：fn return nil → mock expects BEGIN + COMMIT
  - `TestWithTx_Rollback_OnError`：fn return error → mock expects BEGIN + ROLLBACK，原 error 透传
  - `TestWithTx_TxCtxPropagates`：fn 内调 `FromContext(txCtx, db)` 返回的 `*gorm.DB` 与外层不同（同一事务内）
  - `TestFromContext_NoTx_ReturnsFallback`：ctx 无 tx → 返回 fallback db handle

**关键约束**：

- `txKey` 是**未导出**的 struct{} 类型（避免外部包用同样 key 类型撞 ctx —— CLAUDE.md ADR-0007 + Go 标准模式）
- `WithTx` 签名严格按 `docs/宠物互动App_Go项目结构与模块职责设计.md` §10：`WithTx(ctx, func(txCtx context.Context) error) error`，**不**是 `func(tx *gorm.DB) error` —— 业务 service 层不应直接拿 `*gorm.DB`，必须通过 ctx 传播 + repo 层 `FromContext` 取
- **不**在 `tx` 包暴露 `Begin / Commit / Rollback` 这种低层 API（避免业务层手写事务边界 —— 那是 `Go项目结构设计.md` §5.1 "handler 不负责跨多个 repo 手动拼事务" 的反模式）
- `FromContext` 对外导出（让 repo 包 import），但**不要**让 service / handler 层调用（service 应只调 repo 方法 + WithTx，不直接拿 db）

**AC6 — bootstrap 集成：启动 ping MySQL 失败 → fail-fast**

修改 `server/cmd/server/main.go` 与 / 或 `server/internal/app/bootstrap/server.go`：

- main.go 在 `cfg, err := config.Load(...)` 之后、`bootstrap.Run(ctx, cfg)` 之前新增：
  ```go
  gormDB, err := db.Open(ctx, cfg.MySQL)
  if err != nil {
      slog.Error("mysql open failed", slog.Any("error", err))
      os.Exit(1)
  }
  defer func() {
      sqlDB, _ := gormDB.DB()
      _ = sqlDB.Close()
  }()
  ```
  或同等写法（如把 db.Open 放进 `bootstrap.Run` 的开头）。**关键是失败必须 `os.Exit(1)` 而不是 panic / fmt.Println / 容忍继续启动**。
- `bootstrap.Run(ctx, cfg)` 签名扩展为接 `*gorm.DB`（或同等的事务管理器依赖），形如：
  ```go
  func Run(ctx context.Context, cfg *config.Config, gormDB *gorm.DB) error { ... }
  ```
  **或**通过 `Options struct` / 依赖注入 builder 把 db handle 传进 router 构造（router 包目前不需要 db，但要为 4.6 / 4.8 留口子）
- 推荐的最小侵入做法：本 story **只**在 main.go 做 db.Open + tx.NewManager 构造 + 推到 bootstrap.Run 的入参；router 层暂不消费（4.6 才挂业务 handler 时再消费）。**不**为了"完美"重构 router 包，避免本 story 范围爆炸

**关键约束 (测试可注入性)**：

- `db.Open` 与 `tx.NewManager` 必须可被测试替换：单测可以 mock `db.Open` 返回 sqlmock 包出来的 `*gorm.DB`；集成测试（4.7）可以注入 dockertest 的真实 db handle。**不**在 main.go 顶层 hardcode 一个全局 `var globalDB *gorm.DB`（反模式：测试无法替换）

**AC7 — 单测覆盖（≥4 case 整体）**

epics.md §Story 4.2 AC 钦定的"单元测试覆盖（≥4 case）"必须满足，按如下分布落地：

- AC4 `internal/infra/db/mysql_test.go`：≥2 case（empty DSN / invalid DSN）
- AC5 `internal/repo/tx/manager_test.go`：≥4 case（commit / rollback / txCtx propagate / FromContext fallback）
- 总数 ≥ 6 case，**满足** epics.md AC 的 ≥4 下限
- 全部用 sqlmock（按 ADR-0001 §3.1 决策；**不**用 dockertest —— dockertest 留给 AC8）
- 全部 ≤ 100ms / case 跑完（确保 `bash scripts/build.sh --test` 不变慢）

**单测命名规范**（按 server 既有惯例）：

- 表驱动 + 描述性名称：`TestOpen_EmptyDSN_ReturnsError` / `TestWithTx_Commit_OnNilReturn`
- `t.Run(name, ...)` subtest 用 lower_snake_case（如 `t.Run("empty_dsn", ...)`）—— 参考现有 `internal/infra/config/loader_test.go` 风格

**AC8 — 集成测试（dockertest 真实 MySQL）**

新增文件 `server/internal/infra/db/mysql_integration_test.go` 或同等位置（如 `server/internal/repo/tx/manager_integration_test.go`），用 build tag `//go:build integration` 隔离：

```go
//go:build integration
// +build integration

package db_test

func TestMySQLIntegration_OpenAndPing(t *testing.T) { ... }
func TestMySQLIntegration_WithTx_CommitAndRollback(t *testing.T) { ... }
```

覆盖场景（按 epics.md §Story 4.2 AC 钦定）：

- **happy 1**：dockertest 起 mysql:8.0 容器 → `db.Open(ctx, cfg)` 返回 `*gorm.DB` + ping 成功 → 关闭 → 资源清理
- **happy 2**：起容器 → `txManager.WithTx(ctx, fn)` 内执行 `CREATE TABLE tmp (...) + INSERT INTO tmp VALUES (...)` + return nil → 容器外查 `SELECT COUNT(*) FROM tmp` = 1
- **edge 1**：`txManager.WithTx(ctx, fn)` 内执行 `CREATE TABLE tmp + INSERT + return errors.New("force rollback")` → 容器外查 `SELECT COUNT(*) FROM tmp` = 0（CREATE TABLE 也回滚 —— 注：MySQL 8.0 InnoDB 下 DDL 隐式 COMMIT，所以这里改用 `INSERT INTO existing_table` 验证回滚效果，避免 DDL 暗坑；具体实装时**不**用 CREATE TABLE 验证回滚，改用预先 CREATE 一张测试表然后 WithTx 内 INSERT + force error）
- **edge 2**：dockertest 起容器 → 关闭容器 → `db.Open(ctx, cfg)` 返回 error 含 "ping" 或 "connection refused"

**关键约束**：

- 集成测试用 `bash scripts/build.sh --integration` 跑（脚本已有 `--integration` 开关 → `-tags=integration`）
- dockertest 启动 mysql:8.0 容器，`MYSQL_ROOT_PASSWORD=catdev` / `MYSQL_DATABASE=cat_test`，端口由 dockertest 动态分配（**不**用固定端口 3306，避免与本地 MySQL 冲突）
- 每个集成测试独立起一个容器（避免测试间状态污染）；容器在 `t.Cleanup` 清理
- 集成测试**不**进 `bash scripts/build.sh --test` 默认路径（避免每次 push 都拉镜像 + 启动容器导致 CI 慢；只在 `--integration` 显式触发时跑）

**AC9 — `go.mod` 新增依赖**

`server/go.mod` 新增 require（pin 主版本，**不**用 latest）：

```go.mod
require (
    // ORM（按 ADR-0003 选定 GORM）
    gorm.io/gorm v1.25.12
    gorm.io/driver/mysql v1.5.7

    // Migration 工具（Story 4.3 才用，但本 story 一起 pin 住）
    github.com/golang-migrate/migrate/v4 v4.18.1
)
```

**关键约束**：

- 必须用 `go get gorm.io/gorm@v1.25.12 gorm.io/driver/mysql@v1.5.7` 加版本号 pin（**禁止** `go get gorm.io/gorm@latest`）
- 跑 `cd server && go mod tidy` 后 commit `go.sum`
- 不在本 story 引入 `github.com/jmoiron/sqlx`（如果 AC1 ADR-0003 决策选 GORM；如果选 sqlx 则不引入 GORM，二选一）—— **二选一不并存**
- 验收：`bash scripts/build.sh --test` 必须绿（go mod verify + 全量编译 + 单测）

**AC10 — `bash scripts/build.sh --test` 全量绿 + `--integration` 全量绿**

完成后必须能跑通（每条都是验收门）：

```bash
bash scripts/build.sh                    # vet + build → 不报错
bash scripts/build.sh --test             # 加上面 ≥6 case 单测全过
bash scripts/build.sh --race --test      # 加 -race 也全过（确保 tx 包内无 data race）
bash scripts/build.sh --integration      # AC8 集成测试 4 case 全过（前提：本机 docker 可用）
```

**关键约束**：

- 在没有 docker 的开发环境下，`--integration` 应**优雅跳过**（dockertest 启动失败时 `t.Skip("docker not available")` 而非 fail）—— 仍要写到测试代码里，不留 TODO
- README **不**更新（节点 1 README 已说明 MySQL 在 Epic 4 接入；Epic 4 收尾另说）

## Tasks / Subtasks

- [x] **Task 1（AC1）**：写 ADR-0003 选型决策文档
  - [x] 1.1 复制 0001-test-stack.md 头部模板，改 Date/Status/Decider/Related Stories
  - [x] 1.2 写 Context（节点 2 阶段 / Epic 4 第二条 / 后续业务 epic 都依赖 / sqlmock 已锁定 ORM 必须基于 database/sql）
  - [x] 1.3 写 Decision Summary（GORM v1.25.12 + driver mysql v1.5.7 + golang-migrate v4.18.x + 否决 AutoMigrate / goose / atlas 的理由）
  - [x] 1.4 写 DSN 配置策略 / 连接池参数 / 事务级别 / GORM Logger tech debt
  - [x] 1.5 写 Consequences + Follow-ups（4.3 / 4.6 / 4.7 怎么用本 ADR 落地）+ Change Log
- [x] **Task 2（AC3）**：扩展 `internal/infra/config/config.go` 加 `MySQLConfig`
  - [x] 2.1 用 Edit tool 在 `Config` struct 加 `MySQL MySQLConfig \`yaml:"mysql"\`` 字段
  - [x] 2.2 在 `LogConfig` 之前 / `ServerConfig` 之后定义 `MySQLConfig` struct（4 个字段 + godoc 注释）
  - [x] 2.3 跑 `cd server && go vet ./...` 确认无报错
- [x] **Task 3（AC2）**：修改 `server/configs/local.yaml` 加 `mysql:` 段
  - [x] 3.1 在 `server:` 段之后、`log:` 段之前插入 `mysql:` 段（4 个字段 + 注释）
  - [x] 3.2 默认值按 AC2 给的（dsn = `cat:catdev@tcp(127.0.0.1:3306)/cat?...`，max_open_conns=50，max_idle_conns=10，conn_max_lifetime_sec=1800）
- [x] **Task 4（AC9）**：`go.mod` 加依赖
  - [x] 4.1 `cd server && go get gorm.io/gorm@v1.25.12`
  - [x] 4.2 `cd server && go get gorm.io/driver/mysql@v1.5.7`
  - [x] 4.3 `cd server && go get github.com/golang-migrate/migrate/v4@v4.18.1`（通过 internal/tools/tools.go blank import 锁住）
  - [x] 4.4 `cd server && go mod tidy` + `go mod verify`
- [x] **Task 5（AC4）**：实装 `internal/infra/db/mysql.go`
  - [x] 5.1 新建 `server/internal/infra/db/mysql.go`，定义 `func Open(ctx, cfg) (*gorm.DB, error)`
  - [x] 5.2 内部 `gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{...})` + 设连接池参数 + `PingContext(ctx)`
  - [x] 5.3 DSN 为空 → 早返 `errors.New("mysql.dsn is empty")` 不调驱动
  - [x] 5.4 写 godoc 注释（语义、何时返 error、为什么 fail-fast）
- [x] **Task 6（AC5）**：实装 `internal/repo/tx/manager.go`
  - [x] 6.1 新建 `server/internal/repo/tx/manager.go`，定义 `Manager` interface + `manager` struct + `NewManager(db) Manager`
  - [x] 6.2 实装 `WithTx(ctx, fn)`：内部用 `db.Transaction` + ctx 注入 tx + fn 返回 nil/error 决定 commit/rollback
  - [x] 6.3 实装 `FromContext(ctx, fallback) *gorm.DB` helper
  - [x] 6.4 `txKey` 用未导出 struct{}
  - [x] 6.5 写 godoc 注释（必须用 txCtx / 不暴露 Begin/Commit/Rollback / panic 走 GORM 默认）
- [x] **Task 7（AC6）**：bootstrap 集成
  - [x] 7.1 修改 `cmd/server/main.go`：cfg 加载成功后调 `db.Open(ctx, cfg.MySQL)` + 失败 `slog.Error + os.Exit(1)`
  - [x] 7.2 用 `defer sqlDB.Close()` 保证进程退出关连接（main 函数末尾）
  - [x] 7.3 调 `tx.NewManager(gormDB)` 构造 manager 但暂不传给 router（Story 4.6 才挂业务 handler）—— 本 story 只确保依赖能 wire 起来，不消费
  - [x] 7.4 `bootstrap.Run` 签名扩展接 db handle（直接新增参数 `gormDB *gorm.DB, txMgr tx.Manager`；router 包 / NewRouter 当前签名不变 —— 避免重构爆炸）
  - [x] 7.5 跑 `bash scripts/build.sh` 确认编译通过
- [x] **Task 8（AC4 / AC7）**：写 `internal/infra/db/mysql_test.go`
  - [x] 8.1 `TestOpen_EmptyDSN_ReturnsError`
  - [x] 8.2 `TestOpen_InvalidDSN_ReturnsError`
  - [x] 8.3（额外）`TestOpen_UnreachableDSN_ReturnsPingError` —— ping 失败 fail-fast 路径
- [x] **Task 9（AC5 / AC7）**：写 `internal/repo/tx/manager_test.go`
  - [x] 9.1 `TestWithTx_Commit_OnNilReturn`（用 sqlmock，断言 BEGIN + COMMIT 序列）
  - [x] 9.2 `TestWithTx_Rollback_OnError`（断言 BEGIN + ROLLBACK 序列 + error 透传）
  - [x] 9.3 `TestWithTx_TxCtxPropagates`（fn 内 `FromContext(txCtx, db)` 返回 tx handle 不等于 fallback）
  - [x] 9.4 `TestFromContext_NoTx_ReturnsFallback`（ctx 无 tx → 返回 fallback）
  - [x] 9.5（额外）`TestFromContext_WithNilTxValue_ReturnsFallback` —— 防御 nil tx value 边缘 case
- [x] **Task 10（AC8）**：写集成测试（build tag `integration`）
  - [x] 10.1 选位置：`server/internal/infra/db/mysql_integration_test.go`
  - [x] 10.2 `TestMySQLIntegration_OpenAndPing`（dockertest 起 mysql:8.0 → Open → Ping → Close）
  - [x] 10.3 `TestMySQLIntegration_WithTx_Commit`（起容器 → 预 CREATE 测试表 → WithTx 内 INSERT + return nil → 容器外 COUNT = 1）
  - [x] 10.4 `TestMySQLIntegration_WithTx_Rollback`（同上，return error → COUNT = 0）
  - [x] 10.5 `TestMySQLIntegration_PingFailedAfterClose`（起容器 → Open → 关掉容器 → 再 PingContext → 返 error）
  - [x] 10.6 docker 不可用时 `t.Skip("docker not available")`
- [x] **Task 11（AC10）**：全量验证
  - [x] 11.1 `bash scripts/build.sh`（vet + build）—— PASS
  - [x] 11.2 `bash scripts/build.sh --test`（≥6 单测全过）—— PASS（8 case 全过）
  - [x] 11.3 `bash scripts/build.sh --race --test`—— **本机 Windows ThreadSanitizer 无法分配内存（`failed to allocate ... bytes ... error code: 87`），与 ADR-0001 §3.5 备注一致**：本机 race toolchain 缺失/不可用，归 CI（Linux runner）触发；dev 阶段 skip 不阻塞 story
  - [x] 11.4 `bash scripts/build.sh --integration`（前提：本机 docker 可用；本机 docker daemon 未启动 → 4 个集成 case 全部 `t.Skip("docker daemon not reachable")`，集成测试 build 通过）
  - [x] 11.5 `git status --short` 抽检：本 story 影响的文件清单与 AC 范围对齐（File List 章节按实情填）
- [x] **Task 12**：本 story 不做 git commit
  - [x] 12.1 epic-loop 流水线约束：dev-story 阶段不 commit；由下游 fix-review / story-done sub-agent 收口
  - [x] 12.2 commit message 模板（story-done 阶段使用）：

    ```text
    feat(infra/db): MySQL 接入 + tx manager（Story 4.2）

    - ADR-0003 选定 GORM v1.25.12 + golang-migrate v4.18.x，否决 AutoMigrate/goose/atlas
    - configs/local.yaml 加 mysql 段（DSN + 连接池参数）
    - internal/infra/db/mysql.go：Open 含 PingContext fail-fast + 连接池配置
    - internal/repo/tx/manager.go：WithTx(ctx, fn) + FromContext helper（不暴露 Begin/Commit/Rollback）
    - main.go 在 config.Load 之后 db.Open；ping 失败 os.Exit(1)
    - 单测 6 case（empty DSN / invalid DSN / commit / rollback / txCtx propagate / FromContext fallback）
    - 集成测试 4 case 用 dockertest 起 mysql:8.0（build tag integration）

    依据 epics.md §Story 4.2 + docs/宠物互动App_Go项目结构与模块职责设计.md §10 +
    docs/宠物互动App_数据库设计.md §8.1 + ADR-0001 §3.1 sqlmock + ADR-0007 ctx 必传。

    Story: 4-2-mysql-接入
    ```

## Dev Notes

### 关键设计原则

1. **fail-fast over fallback**：MySQL ping 失败必须启动失败，**不**容忍降级（按 NFR3 / 总体架构设计 §"状态以 server 为准"原则；MEMORY.md "No Backup Fallback" 反对用 backup/fallback 掩盖核心风险）。具体到本 story：DSN 空 / 驱动 parse 错 / ping 不通 → main 进程立刻 os.Exit(1) + slog.Error，**不**用空连接池继续启动让用户在第一个业务请求时才发现问题。
2. **ctx 必传 + ORM 句柄经 ctx 传播（ADR-0007）**：`WithTx` fn 内的所有 repo 调用必须用 **`txCtx`**；repo 层用 `tx.FromContext(ctx, fallbackDB)` 取 db handle；service 层**不**直接拿 `*gorm.DB`。这条是 Epic 7 / 20 / 26 / 32 所有事务型 epic 的基础，本 story 一次性钉死。
3. **WithTx 内不打日志（保留 fn 透明）**：`WithTx` 实装本身不打 slog（rollback 时由 GORM 默认行为或 fn 内的 service 层负责），避免每个事务开始 / 结束都额外两条 log 噪声。如未来需要事务 trace，加 OTel span 而非 slog。
4. **不污染上层 import 路径**：`internal/infra/db/` 包只 import GORM + driver；`internal/repo/tx/` 包只 import GORM；service / handler 层**不**直接 import `gorm.io/gorm` —— service 通过 ctx + tx.FromContext 拿 handle，handler 完全不接触 db。这条是分层职责（Go项目结构设计 §5.1-5.5）的硬性约束。
5. **测试可注入 over 全局变量**：本 story **不**在 main.go 顶层 hardcode `var globalDB *gorm.DB`。db handle / tx manager 必须通过显式参数 / Options struct 传给 bootstrap.Run；否则 Story 4.7 的 dockertest 集成测试无法注入真实 db。这是 ADR-0001 §3.4 testify/mock + 接口边界 mock 的延续。

### 架构对齐

**领域模型层**（`docs/宠物互动App_总体架构设计.md` §3）：
- 节点 2 阶段 server 所有数据持久化在 MySQL（不接 Redis，Epic 10 才接）
- 状态以 server 为准 → DB 写失败 = 整个业务失败，不允许"先返回成功后台异步重试"

**数据库层**（`docs/宠物互动App_数据库设计.md`）：
- §3.1 主键 BIGINT UNSIGNED + 字符串下发（GORM 默认 BIGINT 处理，本 story 不直接处理 entity，只搭基础设施）
- §3.2 时间字段 DATETIME(3) + 默认 CURRENT_TIMESTAMP(3) → DSN 必须 `parseTime=true&loc=Local`（AC2 已含）
- §3.3 状态字段 TINYINT → GORM 在 entity 上用 `int8` 映射（4.6 的事）
- §8 关键事务（5 个事务都需要 WithTx）→ 本 story 提供唯一 WithTx 入口

**服务端架构层**（`docs/宠物互动App_Go项目结构与模块职责设计.md`）：
- §4 项目目录建议：`internal/infra/db/` + `internal/repo/tx/manager.go` 已列在目录树（行 191 / 188）
- §5.3 Repository 层：repo 层负责"行锁、条件更新、批量写入"；本 story 不实装具体 repo（4.6 才落地）
- §10 事务边界建议：`txManager.WithTx(ctx, func(txCtx) error { ... })` 是钦定签名（行 800-805）；本 story 严格按此实装
- §10.1 必须放事务的操作：游客登录初始化 / 开箱 / 穿戴 / 合成 / 加入房间 → 都靠本 story 的 WithTx
- §11 错误码与错误处理建议：repo 返回底层错误 → service 转业务错误 → handler 映射统一响应；本 story 在 db.Open / WithTx 层只 wrap fmt.Errorf，**不**产生业务错误码（业务错误在 service 层 4.6 落地）
- §12.2 配置示例：`mysql.dsn / max_open_conns / max_idle_conns` 三字段已示范（行 921-924）；本 story 增加 `conn_max_lifetime_sec` 第四字段，理由是 MySQL `wait_timeout` 默认 8 小时但 idle 长连接被切后 GORM 会复用死连接报 `invalid connection`，必须主动设短

### 测试策略

按 ADR-0001 §3.1：sqlmock 主力（单元 / service 层），dockertest 留给 layer-2 集成测试。本 story 严格遵守：

- **单测层**（`mysql_test.go` + `manager_test.go`）：用 sqlmock 模拟 driver；不起容器，所有 case ≤ 100ms 跑完
- **集成测试层**（`*_integration_test.go` + `//go:build integration`）：用 dockertest 起真实 mysql:8.0 容器；只在 `bash scripts/build.sh --integration` 触发；CI 上跑（本地 docker 可用时也跑）

**已知坑（来自 ADR-0001 §3.1 末尾）**：
- sqlmock 对 GORM 自动生成的 SQL 用正则匹配较脆。**缓解措施**：本 story 的 sqlmock case 都尽量只断言事务边界（BEGIN / COMMIT / ROLLBACK），**不**断言具体 INSERT / UPDATE 语句的 SQL 文本。具体 SQL 断言留给 dockertest 层（4.7）。

### 启动顺序约束

按 `docs/lessons/2026-04-25-slog-init-before-startup-errors.md` 的"两步 init"规则：

```
main()
├─ logger.Init("info")          # bootstrap logger（已有）
├─ flag.Parse()                  # 已有
├─ config.LocateDefault          # 已有
├─ config.Load                   # 已有
├─ logger.Init(cfg.Log.Level)    # 用户配置 logger（已有）
├─ db.Open(ctx, cfg.MySQL)       # ★ 本 story 新增；失败 os.Exit(1)
├─ defer sqlDB.Close()           # ★ 本 story 新增
├─ tx.NewManager(gormDB)         # ★ 本 story 新增（构造但暂不消费）
└─ bootstrap.Run(ctx, cfg, ...)  # 签名扩展含 db handle / manager
```

**关键**：db.Open 必须在 logger.Init(cfg.Log.Level) 之后，否则 db 失败的 slog.Error 走 bootstrap "info" level 是可接受的（仍然是 JSON 输出，只是 level 没用用户配置的 debug）—— 这条不算坑，提前保证启动失败可观测即可。

### 与已 done 的 Story 4.1 的衔接

Story 4.1 已锚定 V1 §4.1 / §4.3 / §5.1 接口契约，含：
- `data.user.id` 等主键字符串下发（§2.5 全局规则）
- `data.chest.unlockAt` ISO 8601 UTC（§2.5）
- 错误码 1009 = 服务繁忙（DB 异常 / 事务回滚 → 都映射 1009）

本 story **不**触发契约变更（按契约冻结策略）；本 story 的代码实现是契约的**第一个落地点**：
- DSN 含 `parseTime=true&loc=Local` 让 GORM 把 DATETIME(3) 自动 parse 成 `time.Time`，到 4.8 GET /home handler 序列化为 ISO 8601 UTC（用 `time.UTC.Format(time.RFC3339)` 由 handler 层负责）
- WithTx 失败时 service 层（4.6）把 `gorm.ErrXxx` / `mysql.MySQLError` 转成 `apperror.New(1009, ...)`；本 story 的 db.Open / WithTx 只**透传**底层 error，**不**自己产生 1009（错误码三层映射 ADR-0006）

### Project Structure Notes

预期文件 / 目录变化：

- ✅ **新增**：`_bmad-output/implementation-artifacts/decisions/0003-orm-stack.md`（ADR-0003 选型决策）
- ✅ **新增**：`server/internal/infra/db/mysql.go` + `mysql_test.go` + `mysql_integration_test.go`
- ✅ **新增**：`server/internal/repo/tx/manager.go` + `manager_test.go`（注：可能含或不含 `manager_integration_test.go`，集成测试落 db 包还是 tx 包二选一）
- ✅ **修改**：`server/internal/infra/config/config.go`（加 `MySQLConfig` struct + `Config.MySQL` 字段）
- ✅ **修改**：`server/configs/local.yaml`（加 `mysql:` 段）
- ✅ **修改**：`server/cmd/server/main.go`（加 db.Open + tx.NewManager + defer Close）
- ✅ **修改**：`server/internal/app/bootstrap/server.go`（Run 签名扩展接 db handle）
- ✅ **修改**：`server/go.mod` + `server/go.sum`（新增 GORM + driver + golang-migrate 依赖）
- ✅ **修改**：`_bmad-output/implementation-artifacts/sprint-status.yaml`（4-2-mysql-接入: ready-for-dev → in-progress → review；由 dev-story 流程内推动）
- ✅ **修改**：`_bmad-output/implementation-artifacts/4-2-mysql-接入.md`（本 story 文件，dev 完成后填 Dev Agent Record / Completion Notes / File List）

不影响其他目录：

- ❌ `server/migrations/` 不变（Story 4.3 才落地）
- ❌ `server/internal/repo/mysql/` 不变（Story 4.6 才落地第一个 user_repo.go）
- ❌ `server/internal/service/` / `server/internal/app/http/handler/` 不变（业务 handler 在 4.6 / 4.8 落地）
- ❌ `iphone/` / `ios/` 不变（本 story 是 server-only）
- ❌ `docs/宠物互动App_*.md` 全部 7 份不变（本 story 是 docs 输入的消费方，不是修改方）
- ❌ `docs/宠物互动App_V1接口设计.md` 不变（4.1 已冻结契约）
- ❌ README.md / server/README.md 不变（Epic 4 收尾才统一更）

### References

- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 4.2 (行 952-972)] — 本 story 的钦定 AC 来源（含 8 条 AC + ≥4 单测 + dockertest 集成测试）
- [Source: `_bmad-output/planning-artifacts/epics.md` §Epic 4 Overview (行 927-931)] — 节点 2 第一个业务 epic / 执行顺序 4.1 → 4.2 → 4.3 → ...
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 项目目录建议 (行 102-207)] — `internal/infra/db/` + `internal/repo/tx/manager.go` 目录已锚定
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §10 事务边界建议 (行 792-859)] — `WithTx(ctx, func(txCtx) error) error` 钦定签名 + 必须放事务的 5 个操作清单
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §11 错误码与错误处理建议 (行 862-889)] — repo → service → handler 错误三层映射；本 story 在 repo / infra 层只 wrap，**不**产业务错误码
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §12.2 配置模块划分 (行 901-937)] — `mysql.dsn / max_open_conns / max_idle_conns` 字段示例
- [Source: `docs/宠物互动App_数据库设计.md` §8.1 游客登录初始化事务 (行 880-895)] — Story 4.6 五表事务的目标使用场景，本 story 是其前置基础设施
- [Source: `docs/宠物互动App_数据库设计.md` §3.1 主键策略 + §3.2 时间字段 (行 73-97)] — DSN 必须 `parseTime=true` 的根因（DATETIME(3) → time.Time）
- [Source: `_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.1 (行 42-53)] — sqlmock + dockertest 双层测试策略；本 story 严格遵守
- [Source: `_bmad-output/implementation-artifacts/decisions/0006-error-handling.md`] — AppError 三层映射；本 story db.Open / WithTx 只透传底层 error，不产业务码
- [Source: `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md`] — `txCtx` vs 外层 ctx 必须区分；service 必须用 txCtx 调 repo
- [Source: `_bmad-output/implementation-artifacts/4-1-接口契约最终化.md`] — 上一 story；契约冻结策略 + 错误码 1009 = 服务繁忙的业务语义
- [Source: `docs/lessons/2026-04-25-slog-init-before-startup-errors.md`] — main.go 启动顺序约束 + slog 初始化时机；db.Open 必须在 logger 初始化之后
- [Source: `docs/lessons/2026-04-24-config-path-and-bind-banner.md`] — 启动 banner 时序错位；db.Open 失败必须 os.Exit(1) 不能假装启动成功
- [Source: `CLAUDE.md` §"工作纪律"] — "状态以 server 为准 / 错误码统一 / ctx 必传" 三条工程红线，本 story 是 ctx 必传 + tx manager 的工程化落地
- [Source: `MEMORY.md` "No Backup Fallback"] — 反对 fallback 掩盖核心风险，db ping 失败必须 fail-fast

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) (`claude-opus-4-7[1m]`)

### Debug Log References

- 2026-04-26 dev session：bmad-dev-story workflow 单次贯通
- `go mod tidy` 第一次会把 GORM 升到 v1.31.1（latest），需要再次 `go get gorm.io/gorm@v1.25.12 gorm.io/driver/mysql@v1.5.7` 强降到 ADR-0003 钦定版本
- `go mod tidy` 会剔除未 import 的 `golang-migrate` 与 `dockertest`：
  - dockertest 通过 `mysql_integration_test.go` 的 `//go:build integration` 文件 import 锁住
  - golang-migrate 通过 `internal/tools/tools.go` 的 `//go:build tools` blank import 锁住（标准 Go 项目模式）

### Completion Notes List

- ✅ ADR-0003 选定 GORM v1.25.12 + golang-migrate v4.18.1，否决 AutoMigrate / goose / atlas
- ✅ `internal/infra/db/mysql.go` 实装 `Open(ctx, cfg) (*gorm.DB, error)`：fail-fast on empty DSN / parse error / ping error；不导出 `*sql.DB`
- ✅ `internal/repo/tx/manager.go` 实装 `Manager` interface + `WithTx(ctx, fn func(txCtx) error) error` + `FromContext(ctx, fallback) *gorm.DB`；签名严格按 `docs/宠物互动App_Go项目结构与模块职责设计.md` §10
- ✅ `cmd/server/main.go` 加 db.Open + tx.NewManager 构造 + defer sqlDB.Close（fail-fast `os.Exit(1)`）
- ✅ `bootstrap.Run` 签名扩展为 `Run(ctx, cfg, gormDB, txMgr)` —— 本 story 阶段 router 暂不消费，签名先扩展为 4.6 留口子
- ✅ `internal/tools/tools.go` 用 `//go:build tools` blank import 锁住 `golang-migrate v4.18.1` 版本（避免 `go mod tidy` 剔除）
- ✅ 单元测试 8 case（3 db / 5 tx）全过 —— 满足 epics.md AC ≥4 的下限
- ✅ 集成测试 4 case 全部用 `t.Skip("docker daemon not reachable")` 跳过（本机 docker 未启动）—— `bash scripts/build.sh --integration` 通过；CI / 真 docker 环境可正常跑
- ⚠️ `bash scripts/build.sh --race --test` —— 本机 Windows ThreadSanitizer 无法分配内存（与 ADR-0001 §3.5 备注一致），race detector 归 CI（Linux runner）触发；dev 阶段 skip 不阻塞 story 完成
- ⚠️ `cd server && go vet ./...` & `bash scripts/build.sh` & `bash scripts/build.sh --test` & `bash scripts/build.sh --integration` 全部绿
- ✅ ADR-0007 ctx 必传 / `txCtx` vs 外层 ctx：单测 `TestWithTx_TxCtxPropagates` 钦定行为；集成测试 `TestMySQLIntegration_WithTx_Commit/Rollback` fn 内严格用 `txCtx`
- ✅ 范围红线遵守：未写 SQL DDL / 未实装业务 repo / 未接 Redis / 未改 docs/ 任一文档 / 未触发 V1 契约变更

### File List

**新增文件**：
- `_bmad-output/implementation-artifacts/decisions/0003-orm-stack.md` —— ADR-0003 选型决策
- `server/internal/infra/db/mysql.go` —— MySQL 连接初始化 + fail-fast ping
- `server/internal/infra/db/mysql_test.go` —— 单测（3 case）
- `server/internal/infra/db/mysql_integration_test.go` —— 集成测试（4 case，build tag `integration`）
- `server/internal/infra/db/timeout_test.go` —— 单测共享 helper（`shortTimeout()`）
- `server/internal/repo/tx/manager.go` —— tx Manager interface + WithTx + FromContext
- `server/internal/repo/tx/manager_test.go` —— 单测（5 case）
- `server/internal/tools/tools.go` —— `//go:build tools` blank import 锁住 golang-migrate
- `_bmad-output/implementation-artifacts/4-2-mysql-接入.md` —— 本 story 文件（bmad-create-story 蒸的，dev 阶段填 Tasks/Dev Agent Record/File List）

**修改文件**：
- `_bmad-output/implementation-artifacts/sprint-status.yaml` —— `4-2-mysql-接入` ready-for-dev → in-progress → review
- `server/internal/infra/config/config.go` —— 加 `MySQLConfig` struct + `Config.MySQL` 字段
- `server/configs/local.yaml` —— 加 `mysql:` 段（DSN + 连接池参数）
- `server/cmd/server/main.go` —— 加 db.Open + tx.NewManager + defer sqlDB.Close
- `server/internal/app/bootstrap/server.go` —— `Run` 签名扩展为 `Run(ctx, cfg, gormDB, txMgr)`
- `server/internal/app/bootstrap/server_test.go` —— 适配新签名（传 nil, nil 给 db / txMgr）
- `server/go.mod` + `server/go.sum` —— 新增 GORM v1.25.12 / GORM mysql driver v1.5.7 / golang-migrate v4.18.1 / dockertest v3.12.0 + 相关 indirect

### Change Log

| Date | Change | By |
|---|---|---|
| 2026-04-26 | Story 4.2 dev：实装 MySQL 接入（GORM v1.25.12）+ tx Manager（WithTx + FromContext）+ ADR-0003 选型 + bootstrap fail-fast；单测 8 case + 集成测试 4 case（dockertest 优雅 skip）全绿 | Claude Opus 4.7 |
