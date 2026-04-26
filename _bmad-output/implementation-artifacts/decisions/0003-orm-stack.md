# ADR-0003: Server ORM / Migration 工具栈

- **Status**: Accepted
- **Date**: 2026-04-26
- **Decider**: Developer
- **Supersedes**: N/A
- **Related Stories**: **4.2 (本决策落地)**, 4.3 (五张表 migrations 用本 ADR pin 的 golang-migrate v4), 4.6 (游客登录初始化事务用本 ADR 的 GORM tx + WithTx 签名), 4.7 (Layer 2 集成测试用 dockertest 起真实 MySQL), Epic 7 / 11 / 14 / 17 / 20 / 23 / 26 / 32 (所有事务型 epic 复用本 ADR 选定的 GORM tx + golang-migrate)

---

## 1. Context

### 1.1 节点 2 阶段的位置

当前项目处于"节点 2：自动登录与默认猫主界面"阶段：

- 节点 1（Epic 1 / 2 / 3）已完成 server / iOS App 可运行的工程骨架。
- 节点 2 第一个真实写代码的 server story 是 **Epic 4 Story 4.2（本 story）**，目标是接入 MySQL + 提供统一事务管理器。
- 后续 Epic 7（步数事务） / 20（开箱事务） / 26（穿戴事务） / 32（合成事务）都依赖一个**已选型确定**的 ORM + 事务管理框架；如果在 Epic 4 阶段不锁定，后续每个 epic 各自拼连接池 / 各自争论 ORM = 风格漂移。

本 ADR 的目的：**一次性锁定 ORM + Migration 工具 + DSN 策略 + 连接池参数 + 事务级别 + GORM Logger tech debt**，覆盖 Epic 4 到 Epic 36 全部 MySQL 相关选型。

**本 ADR 不产出任何 `.go` 代码、不修改 `go.mod`、不写 SQL DDL**。实际代码落地由 Story 4.2 实装部分（AC2-AC10）承担；migration 文件落地由 Story 4.3 承担。

### 1.2 上游 ADR 的约束

ADR-0001 §3.1 已锁定测试工具栈：

- 主力 mock：`sqlmock` v1.5.2（模拟 `database/sql` driver）+ `miniredis` v2.37.0
- 集成测试：`dockertest/v3` v3.12.0（首次 layer-2 集成测试场景引入）
- 已知坑：sqlmock 对 GORM 自动生成的 SQL 用正则匹配较脆 → 集成测试用 dockertest 兜底

**关键约束**：sqlmock 模拟的是 `database/sql` driver，因此本 ADR 选定的 ORM **必须基于 `database/sql`**（GORM ✅ / sqlx ✅；entgo ❌ / go-pg ❌）。这条已经隐式排除了部分候选。

### 1.3 设计文档的钦定签名

`docs/宠物互动App_Go项目结构与模块职责设计.md` §10 钦定了事务签名：

```go
err := txManager.WithTx(ctx, func(txCtx context.Context) error { ... })
```

本 ADR 必须选一个能落地这个签名的 ORM；同时与 ADR-0007 §2.4（tx fn 内 repo 调用必须用 `txCtx` 而非外层 `ctx`）兼容。

### 1.4 CLAUDE.md Tech Stack 的优先序

CLAUDE.md "Tech Stack（新方向）" §"ORM / DB 驱动" 原文：

> ORM / DB 驱动：GORM 或 sqlx

**GORM 列在 sqlx 前**，但允许 sqlx 候选。本 ADR 必须明确选择并记录否决理由。

---

## 2. Decision Summary

| 领域 | 选定 | 版本 |
|---|---|---|
| ORM | **GORM v2** (`gorm.io/gorm`) + MySQL driver (`gorm.io/driver/mysql`) | gorm.io/gorm v1.25.12 / gorm.io/driver/mysql v1.5.7 |
| Migration 工具 | **`golang-migrate/migrate v4`**（CLI + Go API 双形态） | github.com/golang-migrate/migrate/v4 v4.18.1 |
| DSN 配置策略 | YAML `mysql.dsn` 整串读取（不拼字段），生产用环境变量 `CAT_MYSQL_DSN` 覆盖 | — |
| 连接池参数 | `max_open_conns: 50` / `max_idle_conns: 10` / `conn_max_lifetime_sec: 1800`（30 min） | — |
| 事务级别 | MySQL 默认 `REPEATABLE READ`，**保持默认不指定**（未来按需扩展 options） | — |
| GORM Logger | 本 story 用 GORM 默认 logger（stdout）；slog 桥接列为 tech debt | — |

---

## 3. Decisions

### 3.1 ORM 选型

- **选定**：GORM v2 (`gorm.io/gorm` v1.25.12 + `gorm.io/driver/mysql` v1.5.7)。
- **理由**：
  1. **CLAUDE.md Tech Stack 列序优先**：原文 "GORM 或 sqlx"，GORM 在前。
  2. **事务多的业务契合**：Story 4.6 / 7.3 / 20.6 / 26.3 / 32.4 都是多表事务（游客登录初始化 5 张表 / 开箱链路 / 穿戴 / 合成）。GORM 的 `db.Transaction(func(tx *gorm.DB) error)` 内置 BEGIN / COMMIT / ROLLBACK / panic 自动 rollback + repanic，本 story 的 `WithTx(ctx, fn)` 直接 wrap 一层即可。
  3. **soft delete 支持**：`docs/宠物互动App_数据库设计.md` §3.5 钦定 `deleted_at TIMESTAMP NULL` 软删除模式 → GORM 的 `gorm.DeletedAt` 类型 + 自动过滤已删除记录的 query 行为，业务代码不需要每次 `WHERE deleted_at IS NULL`。
  4. **Hooks 机制**：GORM 的 `BeforeCreate` / `AfterCreate` 等钩子可承载未来 audit log / 事件发布（Epic 17 表情广播 / Epic 20 开箱日志）。sqlx 没有等价机制，需要业务层手写。
  5. **基于 `database/sql`**：GORM 使用 `database/sql` driver 接口，与 ADR-0001 §3.1 sqlmock 兼容（sqlmock 提供 `*sql.DB` → `gorm.io/driver/mysql` 的 `New(mysql.Config{Conn: sqlDB})` 可注入）。
  6. **Pin 版本**：`gorm.io/gorm v1.25.12` 是 2026-03 发布的稳定版（GORM v2 主版本下的 1.25.x 系列已稳定多年），`gorm.io/driver/mysql v1.5.7` 配套。

- **否决候选**：
  - **sqlx (`github.com/jmoiron/sqlx`)**：否决 — 性能更好（接近裸 `database/sql`）但样板代码多：每条 INSERT / UPDATE / SELECT 都要手写 SQL + struct 映射；多表事务需要业务层显式 `tx.NamedExec` 序列；soft delete 需要业务层显式 `WHERE deleted_at IS NULL`。本项目 QPS 不属于高频场景（节点 10 上线 WS 后单实例预计 < 1000 msg/s），GORM 的 ORM 开销在总延迟中 < 1%；用 sqlx 换性能不值。**保留迁移路径**：未来如果某 epic 出现 GORM 性能瓶颈，可在该 epic 内单独引入 sqlx 写裸 SQL（`gorm.DB.DB()` 仍能拿 `*sql.DB`），不需要废本 ADR。
  - **entgo (`entgo.io/ent`)**：否决 — 强类型代码生成 + schema-first 是优秀模式，但生态更小，文档主要英文；当前项目 dev 一人 + Claude 主写代码（MEMORY.md "Claude Does All Coding"），引入 ent 增加学习成本不值。
  - **go-pg / go-pgx**：否决 — 仅 PostgreSQL，本项目固定 MySQL 8.0（`docs/宠物互动App_数据库设计.md`）。
  - **xorm**：否决 — 社区活跃度不如 GORM，CLAUDE.md 未列。

### 3.2 Migration 工具选型

- **选定**：`golang-migrate/migrate v4` (`github.com/golang-migrate/migrate/v4` v4.18.1)。
- **理由**：
  1. **纯 SQL 文件 + up/down 双向**：`{N}_{name}.up.sql` / `{N}_{name}.down.sql` 文件对，可逆。Story 4.3 五张表 migrations 必须可 down，否则灰度回滚或本地重置场景无路径。
  2. **CLI + Go API 双形态**：CI / 生产部署可用 `migrate` CLI 一行 up；server 启动时也可用 Go API 检测 schema version（未来扩展）。本 story 不实装 server 内嵌 migration，但 pin 住版本，Story 4.3 直接用。
  3. **不耦合 ORM**：与 GORM 的 AutoMigrate 解耦。GORM `AutoMigrate(&User{})` 生成的 DDL 不可控（如忘记加 index、生成多余字段）；纯 SQL 文件可在 review 阶段精确审查。
  4. **多数据库支持**：未来如果需要切换或新增 PostgreSQL 实例，golang-migrate 支持多 driver 不需要换工具。
  5. **Pin 版本**：`v4.18.1` 是 2026-03 发布的稳定版。

- **否决候选**：
  - **GORM AutoMigrate**：**强烈否决**。
    - 生产不可控：AutoMigrate 不会 drop 列 / 不会改字段类型（设计上保守），但会 add 列 / add index → 一次拼错字段名 = 生产多一列垃圾，且不能 down。
    - 无 down 路径：与"必须可逆"AC 直接冲突。
    - 与 ORM 强绑定：AutoMigrate 依赖 GORM 模型 struct → 业务代码改 struct 就会触发 schema 变更，code-review 难以拦截。
  - **goose (`github.com/pressly/goose`)**：否决 — 同样支持 SQL + Go 文件，但 Go 文件 migration 复杂度高（需要写 Go 代码而非纯 SQL），review 成本高；CLI 易用度不如 golang-migrate。社区活跃度两者相当，但 golang-migrate 在 K8s / Docker 生态使用更广（init container 模式标准支持）。
  - **Atlas (`ariga.io/atlas`)**：否决 — schema-first DSL 强大但学习成本陡（自有 HCL DSL）；本项目体量不需要 atlas 的"declarative schema diff"能力，纯 SQL 文件够用。

### 3.3 DSN 配置策略

- **本地默认 DSN**（`server/configs/local.yaml` 写死）：
  ```
  cat:catdev@tcp(127.0.0.1:3306)/cat?charset=utf8mb4&parseTime=true&loc=Local
  ```
- **DSN 字段说明**（go-sql-driver/mysql 格式）：
  - `cat:catdev` — 本地推荐用专用账号 `cat` / 密码 `catdev`（ADR 级建议；本地 docker `mysql:8.0` 默认 `root:catdev` 也可用）
  - `tcp(127.0.0.1:3306)` — 本地 loopback / 默认端口
  - `/cat` — 数据库名（Story 4.3 创建 schema）
  - `charset=utf8mb4` — 必须，支持 emoji（Epic 17 表情系统、用户名昵称）
  - `parseTime=true` — 必须，让 driver 把 DATETIME(3) 自动 parse 成 `time.Time`（`docs/宠物互动App_数据库设计.md` §3.2）
  - `loc=Local` — 时区。**本地用 Local 即可**；生产 / staging 通过 DSN 覆盖为 `loc=UTC` + 数据库 server 时区也设 UTC（统一规则见数据库设计 §3.2）
- **生产 / staging DSN**：通过环境变量 `CAT_MYSQL_DSN` 覆盖。loader 已支持 env 覆盖范式（参考 `internal/infra/config/loader.go` 现有 `CAT_HTTP_PORT` / `CAT_LOG_LEVEL`），Story 4.2 在 loader 添加 `CAT_MYSQL_DSN` 覆盖。
  - **Note**：本 story 的 loader 改动是可选的范围扩展；如果不在 4.2 加 env 覆盖，可留给 Epic 4 收尾或 Epic 36 部署 story。最低要求：YAML 默认值能跑本地，生产部署侧通过其他机制（如 K8s ConfigMap）覆盖也可接受。**本 story 选择不在 loader 添加 env 覆盖**，理由：env 覆盖是部署侧需求，节点 2 阶段尚未触及部署；保持 loader 最小化。
- **不拼字段**：YAML 不暴露 `host` / `port` / `user` / `password` 等独立字段，避免拼接时漏 escape（特殊字符密码 → URL encoding 错误等暗坑）。

### 3.4 连接池参数

按 `docs/宠物互动App_Go项目结构与模块职责设计.md` §12.2 示例 + 本 ADR 增加 `conn_max_lifetime_sec`：

| 字段 | 推荐值 | 理由 |
|---|---|---|
| `max_open_conns` | 50 | 节点 2 单实例预期 QPS < 100，50 连接足够 burst；MySQL 8.0 默认 `max_connections=151`，预留 ~100 给其他服务（运维 / migration / 其他实例） |
| `max_idle_conns` | 10 | 长连接保活数；空闲 ≤ 10 时不切，避免每次请求 reconnect 增加延迟 |
| `conn_max_lifetime_sec` | 1800（30 分钟） | MySQL server 端 `wait_timeout` 默认 28800s（8 小时）但中间件 / LB（如阿里云 SLB）可能 1-5 分钟切 idle 连接；如果 `*sql.DB` 池里的 idle 连接被 LB 切了 client 不知道，下次复用会报 `invalid connection`。30 分钟主动 refresh 规避 |

**未来可扩展**：`conn_max_idle_time_sec`（idle 连接最大存活）—— 当前不加，Epic 36 部署阶段如有 LB 短切场景再补。

### 3.5 事务级别

- **决策**：MySQL 默认事务级别 `REPEATABLE READ`，**保持默认不指定**。
- **理由**：
  1. MySQL 8.0 InnoDB 默认 RR + gap lock，对游客登录初始化 / 开箱 / 合成等"读旧数据 + 写新数据"场景**够用**。
  2. Phantom read 在本项目场景影响有限（开箱 / 合成的"幂等键 + 加权抽取"逻辑都在 Redis 锁定，不依赖 SQL 隔离级别）。
- **预留扩展**：未来如需要 `READ COMMITTED`（如长事务 + 高并发可见性需求），通过 `WithTx` 的 options 参数扩展：
  ```go
  // Future: 不在本 story 实装，预留思路
  txManager.WithTx(ctx, fn, tx.WithIsolation(sql.LevelReadCommitted))
  ```
  当前 Story 4.2 不实装 options，签名严格 `WithTx(ctx, fn) error`。如未来某 story 需要变隔离级别，通过新 ADR 扩展签名（不破坏向后兼容，新增 variadic options 即可）。

### 3.6 GORM Logger 与 slog 集成

- **本 story 决策**：使用 GORM **默认 logger**（输出到 stdout），**不**实装 slog 桥接。
- **理由**：
  1. **节点 2 阶段够用**：本地 dev 看 SQL 用 GORM 默认 logger（带颜色、可读）即可；生产环境 GORM logger 可关闭（`gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}`）。
  2. **避免本 story 范围爆炸**：slog 桥接需要实装 `gorm.Logger` interface（Trace / Info / Warn / Error 四方法）+ 按 SQL 慢查询阈值告警，是独立 1-2 天的工作量。本 story 焦点是事务管理器 + 连接池，不应捆绑 logger。
- **Tech Debt 登记**（Follow-ups）：
  - `internal/infra/db/gorm_slog_logger.go` — 未来 epic 实装 GORM logger → slog 桥接
  - 触发时机：Epic 36 部署前（生产可观测性需求）或 Epic 4 收尾（如 dev 觉得 stdout 噪声大）
  - 优先级：低（不阻塞业务功能）

---

## 4. Consequences

### 4.1 Positive

- **选型争议清零**：Epic 4-32 的 ORM / migration / 连接池实装直接照搬本 ADR，不再每个 story 重新讨论。
- **依赖可控**：新增 3 个外部库（`gorm.io/gorm` + `gorm.io/driver/mysql` + `golang-migrate/migrate/v4`），加上现有 ADR-0001 的 5 库，总依赖树仍浅。
- **可逆 migration**：纯 SQL 文件 + golang-migrate down 路径，灰度 / 本地重置 / 生产回滚都有方案。
- **可测试性**：sqlmock + GORM 配合（`gorm.io/driver/mysql.New(mysql.Config{Conn: sqlMockDB})`）单元测试无 docker 依赖；layer-2 用 dockertest 跑真 mysql:8.0。
- **签名稳定**：`WithTx(ctx, fn func(txCtx) error) error` 是 Epic 4-32 全部事务的统一入口，下游 epic 只学一次。

### 4.2 Negative / Accepted Trade-offs

- **GORM 性能不如 sqlx**：接受 — 本项目 QPS 场景不需要零分配；future 可在某个高频接口里单独引入 sqlx 而不废 ADR。
- **GORM 默认 logger 噪声**：接受（短期）— 节点 2 阶段 stdout SQL 日志便于 dev 调试；生产前切 Silent 或实装 slog 桥接。
- **sqlmock + GORM 正则匹配脆**：接受 — ADR-0001 §3.1 已登记，本 ADR 重申：单元测试 sqlmock case 优先断言事务边界（BEGIN / COMMIT / ROLLBACK），具体 SQL 文本断言留给 dockertest 集成测试。
- **AutoMigrate 永远不用**：接受 — 即使 dev 阶段也不能用，避免风格漂移到生产。

### 4.3 Follow-ups（按 story 分工）

- **Story 4.2（本 story）**：
  - 实装 `internal/infra/db/mysql.go`：`Open(ctx, cfg) (*gorm.DB, error)` + ping fail-fast + 连接池配置
  - 实装 `internal/repo/tx/manager.go`：`Manager` interface + `WithTx(ctx, fn)` + `FromContext(ctx, fallback)` helper
  - 单测覆盖（empty DSN / invalid DSN / commit / rollback / txCtx propagate / FromContext fallback ≥ 6 case，sqlmock）
  - 集成测试覆盖（dockertest 起 mysql:8.0 → Open + Ping + WithTx commit / rollback / 关容器后 ping 失败 ≥ 4 case）
- **Story 4.3**：用 golang-migrate v4 落 5 张表的 up/down SQL；DDL 严格按 `docs/宠物互动App_数据库设计.md` §5.1-5.5；migrations 目录 `server/migrations/` 第一个文件 `00001_init_schema.up.sql` + `00001_init_schema.down.sql`
- **Story 4.6**：游客登录初始化事务用本 ADR 的 `txManager.WithTx`；fn 内 5 个 repo 调用全部用 `txCtx`（按 ADR-0007 §2.4）；service 层抛 AppError(1009) 兜底 db 错误
- **Story 4.7**：Layer 2 集成测试用 dockertest 起真实 mysql:8.0 + 跑 100 goroutine 并发开箱场景；本 ADR 的 db.Open / WithTx 必须可注入测试用 DSN（不依赖 local.yaml 默认值）
- **Future tech debt**：
  - GORM logger → slog 桥接（触发时机：Epic 36 部署前 / dev 觉得 stdout 噪声大）
  - `WithTx` options 扩展（触发时机：某个 story 出现 RR 不够用的具体场景）
  - `CAT_MYSQL_DSN` 环境变量覆盖（触发时机：Epic 36 部署 story / staging 配置）

---

## 5. Change Log

| Date | Change | By |
|---|---|---|
| 2026-04-26 | 初稿，Story 4.2 交付：选定 GORM v1.25.12 + golang-migrate v4.18.1；DSN / 连接池 / 事务级别 / GORM Logger tech debt 全部锁定 | Developer |
