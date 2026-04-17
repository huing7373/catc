# Story 0.3: 基础设施连通性与客户端

Status: done

## Story

As a maintainer,
I want Mongo + Redis + JWT clients with `MustConnect` + `HealthCheck` + transaction helper,
so that all repos and services route I/O through controlled clients and integration tests boot via Testcontainers.

## Acceptance Criteria

1. **Given** Story 0.2 交付的 Runnable + config 加载能力, **When** 实现 `pkg/mongox/{client.go, tx.go}` + `pkg/redisx/client.go` + `pkg/jwtx/manager.go` + `internal/testutil/{mongo_setup.go, redis_setup.go}`, **Then** `mongox.MustConnect(cfg) *mongo.Client` 内部完成 Connect + Ping + timeout 控制，失败 `log.Fatal`

2. **Given** AC#1, **Then** `redisx.MustConnect(cfg) *redis.Client` 内部完成 Ping，失败 `log.Fatal`

3. **Given** AC#1 AC#2, **Then** 两客户端暴露 `HealthCheck(ctx context.Context) error` 方法供 Story 0.4 的 healthz 复用

4. **Given** AC#1, **Then** `mongox.WithTx(ctx, cli, fn func(ctx context.Context) error) error` 封装 `StartSession` + `WithTransaction`（D10 事务辅助；mongo-driver v2 无 SessionContext，统一用 context.Context）

5. **Given** 配置能力, **Then** `jwtx.New(cfg)` 初始化 RS256 双密钥（含 kid 轮换字段），实现 `Issue(claims) (string, error)` 和 `Verify(token) (*Claims, error)`

6. **Given** AC#1 AC#2, **Then** `internal/testutil/` 提供 `SetupMongo(t *testing.T) (*mongo.Client, func())` + `SetupRedis(t *testing.T) (*redis.Client, func())` 通过 Testcontainers 启动一次性容器（M10 强制 `t.Helper()`；M11 禁 `t.Parallel()`）

7. **Given** AC#6, **Then** 示范集成测试 `pkg/mongox/client_integration_test.go` + `pkg/redisx/client_integration_test.go` 文件顶部含 `//go:build integration` tag，通过 Testcontainers 完成 Connect + Ping 验证

8. **Given** all above, **Then** `bash scripts/build.sh --test` 编译+单元测试通过；`bash scripts/build.sh --test --integration` 能触发集成测试且通过（需 Docker daemon 运行）

9. **Given** all above, **Then** 任何 handler / service 代码禁止直接持有 `*mongo.Client` / `*redis.Client`（宪法 §6，code review 强制）

10. **Given** AC#1 AC#2, **Then** Mongo client 和 Redis client 均实现 `Runnable` 接口（`Name/Start/Final`），在 `initialize.go` 中注册到 `App` 容器，Final 执行 `Disconnect`

## Tasks / Subtasks

- [x] Task 1: 更新 `internal/config/config.go` — JWTCfg 改为 RS256 双密钥配置 (AC: #5)
  - [x] 1.1 JWTCfg 字段重构：移除 `Secret`，添加 `PrivateKeyPath string`、`PrivateKeyPathOld string`（轮换期旧密钥，可空）、`ActiveKID string`、`OldKID string`、`Issuer string`、`AccessExpirySec int`、`RefreshExpirySec int`
  - [x] 1.2 MongoCfg 补充：确认 `TimeoutSec` 字段已存在（当前值在 struct 中，default.toml 需同步）
  - [x] 1.3 更新 `config/default.toml` 对齐新的 JWTCfg 字段 + 补充 `timeout_sec` 到 `[mongo]`
  - [x] 1.4 更新 `config_test.go`：验证新字段正确加载

- [x] Task 2: 实现 `pkg/mongox/client.go` — MongoDB 客户端封装 (AC: #1, #3, #10)
  - [x] 2.1 实现 `MustConnect(cfg config.MongoCfg) *Client`：`mongo.Connect(options.Client().ApplyURI(cfg.URI))` → `Ping(ctx, nil)` → timeout 由 `cfg.TimeoutSec` 控制 → 失败 `log.Fatal`
  - [x] 2.2 定义 `Client` struct 包装 `*mongo.Client`，暴露 `DB() *mongo.Database`（返回 `cfg.DB` 对应的 database）
  - [x] 2.3 实现 `HealthCheck(ctx context.Context) error`：执行 `Ping(ctx, nil)` 返回错误
  - [x] 2.4 实现 `Runnable` 接口：`Name() = "mongo"`, `Start(ctx) = nil`（MustConnect 已在初始化期连接）, `Final(ctx) = cli.Disconnect(ctx)`
  - [x] 2.5 单元测试 `client_test.go`：验证 `Client` 结构体的方法存在（接口满足检查）

- [x] Task 3: 实现 `pkg/mongox/tx.go` — 事务辅助 (AC: #4)
  - [x] 3.1 实现 `WithTx(ctx context.Context, cli *mongo.Client, fn func(ctx context.Context) error) error`：`StartSession` → `sess.WithTransaction` → `EndSession(defer)`
  - [x] 3.2 mongo-driver v2 API：`sess.WithTransaction(ctx, callback)` callback 签名 `func(ctx context.Context) (any, error)`

- [x] Task 4: 实现 `pkg/redisx/client.go` — Redis 客户端封装 (AC: #2, #3, #10)
  - [x] 4.1 实现 `MustConnect(cfg config.RedisCfg) *Client`：`redis.NewClient(&redis.Options{Addr: cfg.Addr, DB: cfg.DB})` → `Ping(ctx)` → 失败 `log.Fatal`
  - [x] 4.2 定义 `Client` struct 包装 `*redis.Client`，暴露 `Cmdable() redis.Cmdable`
  - [x] 4.3 实现 `HealthCheck(ctx context.Context) error`：执行 `Ping(ctx).Err()`
  - [x] 4.4 实现 `Runnable` 接口：`Name() = "redis"`, `Start(ctx) = nil`, `Final(ctx) = cli.Close()`
  - [x] 4.5 单元测试 `client_test.go`：接口满足检查

- [x] Task 5: 实现 `pkg/jwtx/manager.go` — JWT RS256 管理器 (AC: #5)
  - [x] 5.1 `New(cfg config.JWTCfg) *Manager`：读取 `cfg.PrivateKeyPath` 解析 RSA 私钥 → 导出公钥 → 若 `cfg.PrivateKeyPathOld` 非空则解析旧密钥对用于验签
  - [x] 5.2 `Issue(claims CustomClaims) (string, error)`：用 ActiveKID 对应的私钥签发 RS256 JWT；header 含 `kid`
  - [x] 5.3 `Verify(tokenStr string) (*CustomClaims, error)`：解析 token header 中 `kid` → 选对应公钥 → 校验签名+过期
  - [x] 5.4 `CustomClaims` struct 含 `UserID string / DeviceID string / Platform string / TokenType string`，嵌入 `jwt.RegisteredClaims`
  - [x] 5.5 单元测试 `manager_test.go`：7 个测试覆盖 round-trip、过期、unknown kid、旧密钥轮换、无旧密钥、JTI 保留、refresh expiry

- [x] Task 6: 实现 `internal/testutil/mongo_setup.go` + `redis_setup.go` — Testcontainers 辅助 (AC: #6)
  - [x] 6.1 `SetupMongo(t *testing.T) (*mongo.Client, func())`：启动 MongoDB Testcontainer → 返回已 Ping 的 client + cleanup 函数；`t.Helper()` 必调
  - [x] 6.2 `SetupRedis(t *testing.T) (*redis.Client, func())`：启动 Redis Testcontainer → 返回已 Ping 的 client + cleanup 函数；`t.Helper()` 必调
  - [x] 6.3 两个 setup 函数均禁止 `t.Parallel()`（M11）
  - [x] 6.4 Testcontainers Go 模块：使用 `testcontainers-go/modules/mongodb` 和 `testcontainers-go/modules/redis`

- [x] Task 7: 集成测试 (AC: #7)
  - [x] 7.1 `pkg/mongox/client_integration_test.go`：`//go:build integration` + 内联 Testcontainer 启动 → 验证 MustConnect + HealthCheck + DB + Raw + WithTx
  - [x] 7.2 `pkg/redisx/client_integration_test.go`：`//go:build integration` + 内联 Testcontainer 启动 → 验证 MustConnect + HealthCheck + Set/Get
  - [x] 7.3 集成测试内联 Testcontainer（避免 pkg→internal 可见性问题），testutil 留给 repository 层复用

- [x] Task 8: 更新 `cmd/cat/initialize.go` — 注册基础设施客户端 (AC: #10)
  - [x] 8.1 添加 `mongox.MustConnect(cfg.Mongo)` + `redisx.MustConnect(cfg.Redis)` 调用
  - [x] 8.2 将 Mongo/Redis 的 Runnable 注册到 `NewApp(mongoCli, redisCli, httpSrv)` — 逆序 Final 保证先停 HTTP 再停 Redis 再停 Mongo
  - [x] 8.3 暂不添加 jwtx（Story 1.1 实际使用时再注入）
  - [x] 8.4 wire.go 无需修改（handler 签名未变）

- [x] Task 9: 删除被替换的 doc.go placeholder (AC: all)
  - [x] 9.1 删除 `pkg/mongox/doc.go`、`pkg/redisx/doc.go`、`pkg/jwtx/doc.go`

- [x] Task 10: 验证 (AC: #8)
  - [x] 10.1 `bash scripts/build.sh --test` 编译+单元测试通过（17 tests: config 2, app 3, handler 1, mongox 3, redisx 3, jwtx 7）(注：handler 1 test 是 Story 0.2 遗留)
  - [x] 10.2 `go vet -tags=integration` 集成测试编译通过（实际运行需 Docker daemon）
  - [x] 10.3 go.mod 新增直接依赖：mongo-driver/v2 v2.5.1, go-redis/v9 v9.18.0, golang-jwt/v5 v5.3.1, testcontainers-go modules

## Dev Notes

### Architecture Constraints (MANDATORY)

**宪法 §1 显式胜于隐式：**
- 无 DI 框架，所有依赖在 `initialize()` 手工构造
- `MustConnect` 是唯一允许在构造期做 I/O 的例外（一次性连接+ping）

**宪法 §6 数据访问只通过 repository：**
- handler / service 禁止直接持有 `*mongo.Client` / `*redis.Client`
- `mongox.Client` 和 `redisx.Client` 只注入给 repository 层

**宪法 §5 Runnable 接口：**
- Mongo / Redis 客户端必须实现 `Runnable`，注册到 `App` 容器
- `Final` 执行断开连接，必须幂等

**宪法 §7 Context 贯穿：**
- 所有 I/O 函数首参 `ctx context.Context`
- `MustConnect` 内部用 `context.WithTimeout(context.Background(), ...)` 是启动期允许的

### Source Tree — 本 story 需要 touch 的文件

```
pkg/mongox/client.go           — 新建（替换 doc.go）
pkg/mongox/tx.go               — 新建
pkg/mongox/client_test.go      — 新建（单元测试）
pkg/mongox/client_integration_test.go — 新建（集成测试，//go:build integration）
pkg/mongox/doc.go              — 删除

pkg/redisx/client.go           — 新建（替换 doc.go）
pkg/redisx/client_test.go      — 新建（单元测试）
pkg/redisx/client_integration_test.go — 新建（集成测试，//go:build integration）
pkg/redisx/doc.go              — 删除

pkg/jwtx/manager.go            — 新建（替换 doc.go）
pkg/jwtx/manager_test.go       — 新建
pkg/jwtx/doc.go                — 删除

internal/testutil/mongo_setup.go — 新建
internal/testutil/redis_setup.go — 新建

internal/config/config.go       — 修改（JWTCfg 字段重构）
internal/config/config_test.go  — 修改

cmd/cat/initialize.go           — 修改（添加 Mongo/Redis 客户端）

config/default.toml             — 修改（JWTCfg + timeout_sec）
```

### Mongo Driver v2 API 要点

本项目使用 `go.mongodb.org/mongo-driver/v2`（**不是 v1**）。v2 的关键 API 差异：

- **Connect**: `mongo.Connect(opts...)` 无需 `context` 参数（v2 改动）；opts 用 `options.Client().ApplyURI(uri)`
- **Ping**: `cli.Ping(ctx, nil)` 仍需 context
- **Database**: `cli.Database(name)`
- **Session/Transaction**: `cli.StartSession()` 返回 `(mongo.Session, error)`；`sess.WithTransaction(ctx, fn)` 其中 fn 签名 `func(ctx context.Context) (interface{}, error)`
- **Disconnect**: `cli.Disconnect(ctx)`
- Import paths: `go.mongodb.org/mongo-driver/v2/mongo`, `go.mongodb.org/mongo-driver/v2/mongo/options`

### Redis go-redis/v9 API 要点

- `redis.NewClient(&redis.Options{Addr: addr, DB: db})`
- `cli.Ping(ctx)` 返回 `*StatusCmd`，用 `.Err()` 检查
- `cli.Close()` 关闭连接（无 ctx 参数）

### JWT RS256 双密钥设计

**配置结构（TOML）：**
```toml
[jwt]
private_key_path = "config/jwt_private.pem"
private_key_path_old = ""  # 轮换期填旧密钥路径
active_kid = "key-2026-04"
old_kid = ""               # 轮换期填旧 kid
issuer = "catserver"
access_expiry_sec = 900    # 15min
refresh_expiry_sec = 2592000  # 30 days
```

**Manager 结构：**
```go
type Manager struct {
    activeKey    *rsa.PrivateKey
    activePub    *rsa.PublicKey
    activeKID    string
    oldPub       *rsa.PublicKey  // nil if no rotation
    oldKID       string
    issuer       string
    accessExpiry time.Duration
    refreshExpiry time.Duration
}
```

**Issue 流程：** 用 activeKey 签名，header 写入 `kid = activeKID`
**Verify 流程：** 解析 header `kid` → 匹配 activeKID 则用 activePub，匹配 oldKID 则用 oldPub → 校验签名+过期

**测试密钥生成（测试中 in-memory）：**
```go
key, _ := rsa.GenerateKey(rand.Reader, 2048)
// 写入 PEM 临时文件供 New() 读取
```

### mongox.Client 包装设计

```go
type Client struct {
    cli *mongo.Client
    db  string
}

func MustConnect(cfg config.MongoCfg) *Client { ... }
func (c *Client) DB() *mongo.Database    { return c.cli.Database(c.db) }
func (c *Client) Raw() *mongo.Client     { return c.cli }  // WithTx 需要
func (c *Client) HealthCheck(ctx context.Context) error { return c.cli.Ping(ctx, nil) }
func (c *Client) Name() string           { return "mongo" }
func (c *Client) Start(ctx context.Context) error { return nil }
func (c *Client) Final(ctx context.Context) error { return c.cli.Disconnect(ctx) }
```

### redisx.Client 包装设计

```go
type Client struct {
    cli *redis.Client
}

func MustConnect(cfg config.RedisCfg) *Client { ... }
func (c *Client) Cmdable() redis.Cmdable { return c.cli }
func (c *Client) HealthCheck(ctx context.Context) error { return c.cli.Ping(ctx).Err() }
func (c *Client) Name() string           { return "redis" }
func (c *Client) Start(ctx context.Context) error { return nil }
func (c *Client) Final(ctx context.Context) error { return c.cli.Close() }
```

### Testcontainers 使用模式

```go
// internal/testutil/mongo_setup.go
func SetupMongo(t *testing.T) (*mongo.Client, func()) {
    t.Helper()
    ctx := context.Background()
    container, err := mongodb.Run(ctx, "mongo:7")
    require.NoError(t, err)
    uri, err := container.ConnectionString(ctx)
    require.NoError(t, err)
    cli, err := mongo.Connect(options.Client().ApplyURI(uri))
    require.NoError(t, err)
    require.NoError(t, cli.Ping(ctx, nil))
    cleanup := func() {
        cli.Disconnect(ctx)
        container.Terminate(ctx)
    }
    return cli, cleanup
}
```

注意：`testutil` 返回原始 `*mongo.Client` 而非 `mongox.Client`，因为 `testutil` 在 `internal/` 可以访问 `pkg/`，但集成测试在 `pkg/mongox/` 包内无法 import `internal/testutil`。

**解决方案**：集成测试 `pkg/mongox/client_integration_test.go` 和 `pkg/redisx/client_integration_test.go` 内联 Testcontainer 启动（不复用 testutil）。`internal/testutil/` 的 setup 函数供后续 repository 层集成测试复用。

### initialize.go 更新模式

```go
func initialize(cfg *config.Config) *App {
    log.Info().
        Str("build_version", buildVersion).
        Str("config_hash", cfg.Hash).
        Msg("server starting")

    mongoCli := mongox.MustConnect(cfg.Mongo)
    redisCli := redisx.MustConnect(cfg.Redis)

    h := &handlers{
        health: handler.NewHealthHandler(),
    }

    router := buildRouter(cfg, h)
    httpSrv := newHTTPServer(cfg, router)

    return NewApp(httpSrv, mongoCli, redisCli)
}
```

Shutdown 顺序按宪法 §5：逆序 Final → 先停 HTTP，再停 Redis，再停 Mongo。因此 `NewApp` 注册顺序为 `mongoCli, redisCli, httpSrv`（Mongo 最先启动、最后关闭）。

### 本 story 的边界（不做什么）

- **不实现 repository 层**（后续 epic 按需）
- **不注入 jwtx 到 initialize.go**（Story 1.1 使用时注入）
- **不实现 logx 包**（Story 0.5）
- **不修改 health handler**（Story 0.4 才集成 HealthCheck）
- **不实现 middleware**（Story 0.5 之后）
- **不实现 ids 包**（后续 story 按需）
- **不连接真实 Mongo/Redis**（单元测试不需要连接；集成测试通过 Testcontainers）
- **不生成 RSA 密钥文件到 repo**（测试中临时生成；生产环境由运维提供并挂载）

### Testing Standards

- 文件命名：`xxx_test.go` 同目录
- Table-driven 多场景测试
- 使用 `testify`：`require.NoError` / `assert.Equal`
- 单元测试默认开 `t.Parallel()`
- 集成测试禁 `t.Parallel()`（M11 Testcontainer 共享端口）
- 集成测试文件顶部 `//go:build integration`
- 命名：`TestClient_HealthCheck_Success`、`TestManager_Issue_Verify_RoundTrip`

### Previous Story Intelligence (0.2)

Story 0.2 已完成，建立了以下基础：
- `cmd/cat/initialize.go`：当前只装配 health handler + HTTP server + `NewApp(httpSrv)`
- `cmd/cat/app.go`：`Runnable` 接口 + `App` 容器 + 信号处理已完整实现
- `cmd/cat/wire.go`：`handlers` struct 聚合 + `buildRouter` + `httpServer` Runnable
- `internal/config/config.go`：`MustLoad` + `mustValidate` + SHA256 hash
- `Config` struct 已有 `MongoCfg`（URI/DB/TimeoutSec）、`RedisCfg`（Addr/DB）、`JWTCfg`（Secret/Issuer/Expiry）
- Go 1.25 + gin v1.12.0 + zerolog + testify 已在 go.mod
- `scripts/build.sh` 支持 `--test`、`--race`、`--integration` 标志
- 已删除被替换的 doc.go placeholder 文件
- Go module path: `github.com/huing/cat/server`

### Git Intelligence

最近提交：
- `1197b86 feat: implement Story 0.1 + 0.2` — 建立了骨架 + Runnable 生命周期
- 代码风格：Go 标准风格，zerolog 全局 logger（Story 0.5 才实现 logx）
- 当前 zerolog 使用 `github.com/rs/zerolog/log` 全局 logger，本 story 同样使用全局 logger

### Project Structure Notes

- 严格遵循 `docs/backend-architecture-guide.md` 目录结构
- `pkg/` 不得引用 `internal/`；`internal/` 可引用 `pkg/`
- 现有 `doc.go` placeholder 文件在被真实实现替换时应删除
- import 分组：标准库 → 第三方 → 本项目

### References

- [Source: docs/backend-architecture-guide.md §4] — initialize.go 显式 DI 装配：`mongoCli := mongox.MustConnect(cfg.Mongo)`
- [Source: docs/backend-architecture-guide.md §5] — Runnable 接口，Mongo/Redis 需实现
- [Source: docs/backend-architecture-guide.md §10.1] — `mongox.MustConnect` 完整代码示例
- [Source: docs/backend-architecture-guide.md §10.3] — `mongox.WithTx` 完整代码示例
- [Source: docs/backend-architecture-guide.md §16] — 测试标准：repository 集成测试打真 Mongo
- [Source: _bmad-output/planning-artifacts/epics.md, Story 0.3] — 完整验收标准
- [Source: _bmad-output/planning-artifacts/epics.md, NFR-SEC-2] — JWT RS256 双密钥轮换
- [Source: _bmad-output/planning-artifacts/prd.md §JWT] — RS256 双密钥，golang-jwt/jwt/v5

## Dev Agent Record

### Agent Model Used

Claude Opus 4.6 (1M context)

### Debug Log References

### Completion Notes List

- JWTCfg 从对称密钥（Secret/Issuer/Expiry）重构为 RS256 双密钥（PrivateKeyPath/PrivateKeyPathOld/ActiveKID/OldKID/AccessExpirySec/RefreshExpirySec）
- mongox.Client 包装 *mongo.Client，提供 MustConnect（Connect+Ping+timeout）、DB()、Raw()、HealthCheck()、Runnable 接口
- mongox.WithTx 封装 StartSession + WithTransaction，callback 使用 mongo-driver v2 签名 func(context.Context) (any, error)
- redisx.Client 包装 *redis.Client，提供 MustConnect（NewClient+Ping）、Cmdable()、HealthCheck()、Runnable 接口
- jwtx.Manager 实现 RS256 双密钥轮换：New 读取 PEM 文件（PKCS8/PKCS1 兼容），Issue 用 activeKey 签名含 kid header，Verify 按 kid 选公钥校验
- internal/testutil 提供 SetupMongo/SetupRedis 通过 Testcontainers 启动容器，供后续 repository 集成测试复用
- 集成测试在 pkg/ 内联 Testcontainer 启动（避免 pkg→internal 导入限制），使用 //go:build integration tag
- initialize.go 注册顺序 mongoCli→redisCli→httpSrv，逆序 Final 保证先停 HTTP 最后停 Mongo

### Change Log

- 2026-04-17: Story 0.3 实现完成 — Mongo/Redis/JWT 客户端封装 + Testcontainers 集成测试 + initialize.go 集成

### File List

- internal/config/config.go (modified — JWTCfg RS256 字段重构)
- internal/config/config_test.go (modified — 适配新 JWTCfg 字段)
- config/default.toml (modified — JWT 新字段 + mongo timeout_sec)
- pkg/mongox/client.go (new — MustConnect + Client wrapper + Runnable)
- pkg/mongox/tx.go (new — WithTx 事务辅助)
- pkg/mongox/client_test.go (new — 单元测试 3 tests)
- pkg/mongox/client_integration_test.go (new — 集成测试 //go:build integration)
- pkg/mongox/doc.go (deleted)
- pkg/redisx/client.go (new — MustConnect + Client wrapper + Runnable)
- pkg/redisx/client_test.go (new — 单元测试 3 tests)
- pkg/redisx/client_integration_test.go (new — 集成测试 //go:build integration)
- pkg/redisx/doc.go (deleted)
- pkg/jwtx/manager.go (new — RS256 Manager: New/Issue/Verify + 双密钥轮换)
- pkg/jwtx/manager_test.go (new — 单元测试 7 tests)
- pkg/jwtx/doc.go (deleted)
- internal/testutil/mongo_setup.go (new — SetupMongo Testcontainer helper)
- internal/testutil/redis_setup.go (new — SetupRedis Testcontainer helper)
- cmd/cat/initialize.go (modified — 添加 mongox/redisx MustConnect + App 注册)
- go.mod (modified — 新增 mongo-driver/v2, go-redis/v9, golang-jwt/v5, testcontainers-go)
- go.sum (modified)
