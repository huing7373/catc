# Story 0.4: 多维健康检查 endpoint

Status: review

## Story

As an operator,
I want `GET /healthz` + `/readyz` exposing multi-dimensional health state,
so that Uptime Robot and I can instantly diagnose the root cause when paged at 3 AM (J4 ops journey).

## Acceptance Criteria

1. **Given** Story 0.3 交付的 Mongo/Redis 客户端 + Story 0.9 将交付的 WS Hub（占位，本 story 只校验 interface 可达），**When** 实现 `internal/handler/health_handler.go`，**Then** `GET /healthz` 返回 200 + JSON `{"status":"ok","mongo":"ok","redis":"ok","wsHub":"ok","lastCronTick":"<RFC3339>"}` 当所有维度健康

2. **Given** AC#1，**Then** Mongo ping 失败时 `/healthz` 返回 503 + JSON 对应字段为 `"error: <reason>"`，其余字段保持真实值

3. **Given** AC#1，**Then** Redis ping 失败 → 返回 503；WS hub goroutine count 超过 `cfg.WS.MaxConnections`（目标 10k，NFR-SCALE-4）→ 返回 503

4. **Given** AC#1，**Then** `GET /readyz` 仅检查"进程是否 ready 接受流量"（内存中的启动完成标志），不触发外部依赖调用

5. **Given** AC#1，**Then** 两 endpoint 不需要鉴权（NFR-OBS-4）

6. **Given** AC#1，**Then** 两 endpoint 响应时间 p95 ≤ 50ms（内部探针，不含网络延迟）（集成测试断言）

7. **Given** AC#1，**Then** 单元测试覆盖四种状态：全健康 / Mongo 故障 / Redis 故障 / wsHub goroutine 超限

8. **Given** AC#1，**Then** 集成测试（Testcontainers）停掉 Mongo 容器后 `/healthz` 返回 503 且服务不 panic（宪法 §19）

## Tasks / Subtasks

- [x] Task 1: 添加 WS 配置字段到 Config (AC: #3)
  - [x] 1.1 在 `internal/config/config.go` 的 `Config` struct 添加 `WS WSCfg`
  - [x] 1.2 定义 `WSCfg struct { MaxConnections int }`
  - [x] 1.3 在 `config/default.toml` 添加 `[ws]` section：`max_connections = 10000`
  - [x] 1.4 更新 `config_test.go` 验证新字段加载
- [x] Task 2: 定义 HealthChecker 接口 + WS Hub 占位实现 (AC: #1, #3)
  - [x] 2.1 在 `internal/handler/health_handler.go` 中定义两个接口：`type InfraChecker interface { HealthCheck(ctx context.Context) error }` 和 `type WSHubChecker interface { GoroutineCount() int }`
  - [x] 2.2 创建 `internal/ws/hub_stub.go`：`type HubStub struct{}` 实现 `WSHubChecker` 接口，`GoroutineCount() int` 返回 0
  - [x] 2.3 HubStub 也实现 `Name() string` 返回 "ws_hub"，供 Story 0.9 替换为真实实现
- [x] Task 3: 重写 HealthHandler — 多维 healthz + readyz (AC: #1-#6)
  - [x] 3.1 `NewHealthHandler(mongo InfraChecker, redis InfraChecker, wsHub WSHubChecker, redisCmdable redis.Cmdable, maxConn int)` — 注入依赖
  - [x] 3.2 `Healthz(c *gin.Context)` 实现：并行执行 Mongo ping + Redis ping → 读 WS hub goroutine count 与 maxConn 对比 → 读 Redis key `cron:last_tick` → 构造 JSON 响应 → 任一维度异常返回 503
  - [x] 3.3 `Readyz(c *gin.Context)` 实现：检查内存中 `ready` 标志（`atomic.Bool`），200 或 503
  - [x] 3.4 `SetReady()` 方法供 `App.Run()` 在所有 Runnable.Start 成功后调用
- [x] Task 4: 更新 initialize.go + wire.go 接入新 HealthHandler (AC: #1-#5)
  - [x] 4.1 在 `initialize.go` 中创建 `ws.NewHubStub()` 占位
  - [x] 4.2 `NewHealthHandler(mongoCli, redisCli, hubStub, redisCli.Cmdable(), cfg.WS.MaxConnections)` 替换旧的无参构造
  - [x] 4.3 在 `wire.go` 的 `buildRouter` 添加 `r.GET("/readyz", h.health.Readyz)` 路由
  - [x] 4.4 在 `app.go` 的 `Run()` 方法中，所有 Runnable.Start 成功后调用 `healthHandler.SetReady()`
- [x] Task 5: 单元测试 (AC: #7)
  - [x] 5.1 创建 `internal/handler/health_handler_test.go`
  - [x] 5.2 Mock 实现 `InfraChecker`（可控返回 nil 或 error）和 `WSHubChecker`（可控返回 count）
  - [x] 5.3 Table-driven 测试覆盖：全健康、Mongo 故障、Redis 故障、WS Hub 超限、readyz ready=true、readyz ready=false
  - [x] 5.4 验证 JSON 响应结构和 HTTP 状态码
- [x] Task 6: 集成测试 (AC: #8)
  - [x] 6.1 创建 `internal/handler/health_handler_integration_test.go`（`//go:build integration`）
  - [x] 6.2 通过 Testcontainers 启动 Mongo + Redis
  - [x] 6.3 构建真实 HealthHandler 并启动 Gin test server
  - [x] 6.4 测试正常场景：`GET /healthz` 返回 200
  - [x] 6.5 测试 Mongo 容器停止后：`GET /healthz` 返回 503 且不 panic
  - [x] 6.6 性能断言：healthz 响应 p95 ≤ 50ms（循环调用 100 次取 p95）
- [x] Task 7: 验证 (AC: all)
  - [x] 7.1 `bash scripts/build.sh --test` 编译+单元测试通过
  - [x] 7.2 确认无 lint 错误

## Dev Notes

### Architecture Constraints (MANDATORY)

**宪法 §1 显式胜于隐式：**
- 无 DI 框架，所有依赖在 `initialize()` 手工构造
- HealthHandler 通过构造函数注入 Mongo/Redis/WSHub checker

**宪法 §6 数据访问只通过 repository：**
- 本 story 是例外情况：health handler 直接调用 `HealthCheck()` 是基础设施探测而非业务数据访问
- 但读取 `cron:last_tick` Redis key 需通过 `redis.Cmdable` 接口（不创建 repository，因为这是纯运维探测）

**宪法 §7 Context 贯穿：**
- `Healthz` handler 应传递 `c.Request.Context()` 到所有 HealthCheck 调用

**架构指南 §4 initialize.go 示范：**
- 架构指南中 `handler.NewHealthHandler(mongoCli, rdb)` 表明 health handler 接受基础设施客户端
- 本实现扩展为接受更多维度的 checker

### 接口设计（Accept interfaces, return structs）

HealthHandler 接受两个接口而非具体类型：

```go
type InfraChecker interface {
    HealthCheck(ctx context.Context) error
}

type WSHubChecker interface {
    GoroutineCount() int
}
```

- `mongox.Client` 和 `redisx.Client` 已有 `HealthCheck(ctx) error` 方法 → 自动满足 `InfraChecker`
- WS Hub 本 story 用占位 stub，Story 0.9 替换为真实实现
- `redis.Cmdable` 用于读取 `cron:last_tick` key（Story 0.8 交付的 cron heartbeat 写入）

### Response Schema

**`GET /healthz` — 200 全健康：**
```json
{
  "status": "ok",
  "mongo": "ok",
  "redis": "ok",
  "wsHub": "ok",
  "lastCronTick": "2026-04-17T03:00:00Z"
}
```

**`GET /healthz` — 503 Mongo 故障：**
```json
{
  "status": "error",
  "mongo": "error: connection refused",
  "redis": "ok",
  "wsHub": "ok",
  "lastCronTick": "2026-04-17T03:00:00Z"
}
```

**`GET /readyz` — 200 就绪：**
```json
{"ready": true}
```

**`GET /readyz` — 503 未就绪：**
```json
{"ready": false}
```

### lastCronTick 字段处理

- Story 0.8 尚未实现，`cron:last_tick` Redis key 不存在时：
  - 字段值为 `""` （空字符串），不返回 error
  - 不影响整体 status（cron tick 缺失不算故障，只是尚未运行）
- Story 0.8 实现后，cron heartbeat 每分钟更新此 key

### WS Hub 占位策略

- Story 0.9 尚未实现，本 story 创建 `internal/ws/hub_stub.go`
- `HubStub` 实现 `WSHubChecker` 接口，`GoroutineCount()` 始终返回 0
- 当 Story 0.9 实现真实 Hub 时，替换 stub 并在 `initialize.go` 中注入真实实例
- 占位期间 wsHub 维度始终为 "ok"（0 < maxConn）

### Readyz 设计

- `readyz` 用于 K8s/负载均衡器探针，仅检查进程启动完成
- 使用 `sync/atomic.Bool` 标记就绪状态，零锁争用
- `App.Run()` 在所有 Runnable.Start 成功后调用 `healthHandler.SetReady()`
- 不触发任何外部 I/O

### Healthz 并行执行

- Mongo ping 和 Redis ping 应并行执行（goroutine + errgroup 或手动 WaitGroup）
- 避免串行执行导致 p95 超过 50ms 限制
- WS hub count 和 Redis GET 是内存/本地操作，可串行

### Source Tree — 本 story 需要 touch 的文件

```
internal/handler/health_handler.go          — 重写（多维健康检查）
internal/handler/health_handler_test.go     — 新建（单元测试）
internal/handler/health_handler_integration_test.go — 新建（集成测试）
internal/ws/hub_stub.go                     — 新建（WS Hub 占位）
internal/config/config.go                   — 修改（添加 WSCfg）
internal/config/config_test.go              — 修改（验证 WSCfg）
config/default.toml                         — 修改（添加 [ws] section）
cmd/cat/initialize.go                       — 修改（注入依赖到 HealthHandler）
cmd/cat/wire.go                             — 修改（添加 /readyz 路由）
cmd/cat/app.go                              — 修改（SetReady 回调）
```

### 现有代码要替换/修改

**`internal/handler/health_handler.go` 当前实现：**
```go
type HealthHandler struct{}
func NewHealthHandler() *HealthHandler { return &HealthHandler{} }
func (h *HealthHandler) Healthz(c *gin.Context) {
    c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
```
→ 需要完全重写为多维检查版本

**`cmd/cat/initialize.go` 当前：**
```go
h := &handlers{
    health: handler.NewHealthHandler(),
}
```
→ 改为 `handler.NewHealthHandler(mongoCli, redisCli, hubStub, redisCli.Cmdable(), cfg.WS.MaxConnections)`

**`cmd/cat/wire.go` 当前路由：**
```go
r.GET("/healthz", h.health.Healthz)
```
→ 添加 `r.GET("/readyz", h.health.Readyz)`

**`cmd/cat/app.go`：**
→ 在 Run() 中所有 Start 成功后回调 SetReady

### Testing Standards

- 文件命名：`xxx_test.go` 同目录
- Table-driven 多场景测试
- 使用 `testify`：`require.NoError` / `assert.Equal`
- 单元测试用 mock 实现接口（不依赖真实 Mongo/Redis）
- 集成测试文件顶部 `//go:build integration`
- 集成测试使用 Testcontainers（复用 `internal/testutil/` 的 SetupMongo/SetupRedis）
- 集成测试禁 `t.Parallel()`（M11）
- Gin 测试用 `httptest.NewRecorder()` + `gin.CreateTestContext()`
- 命名：`TestHealthHandler_Healthz_AllHealthy`、`TestHealthHandler_Healthz_MongoFailure`

### Mongo/Redis Client API（Story 0.3 已交付）

```go
// pkg/mongox/client.go
func (c *Client) HealthCheck(ctx context.Context) error  // Ping, returns error
func (c *Client) DB() *mongo.Database
func (c *Client) Raw() *mongo.Client

// pkg/redisx/client.go
func (c *Client) HealthCheck(ctx context.Context) error  // Ping, returns error
func (c *Client) Cmdable() redis.Cmdable                 // 用于 GET cron:last_tick
```

### Previous Story Intelligence (0.3)

Story 0.3 已完成的关键决定：
- `mongox.Client` 和 `redisx.Client` 使用 `ConnectOptions` struct 而非直接传 config（解耦 `pkg/` 对 `internal/config` 的依赖）
- 两个 client 都实现了 Runnable 接口 + HealthCheck 方法
- `initialize.go` 注册顺序 `mongoCli → redisCli → httpSrv`，逆序 Final
- Testcontainers 模式：`internal/testutil/` 提供 `SetupMongo(t)` 和 `SetupRedis(t)`
- Go module path: `github.com/huing/cat/server`
- go.mod 已有：mongo-driver/v2 v2.5.1, go-redis/v9 v9.18.0, testify, testcontainers-go

### Git Intelligence

最近提交（Story 0.3）：
- `cd2d9b3 feat: implement Story 0.3` — Mongo/Redis/JWT 客户端 + Testcontainers
- Review 修复 3 轮：issuer 校验、ping 超时、JWT exp 必填、App.Run 启动失败处理
- 代码风格：Go 标准风格，zerolog 全局 logger（`github.com/rs/zerolog/log`）
- 当前 build.sh 支持 `--test`、`--race`、`--integration`

### Project Structure Notes

- 严格遵循 `docs/backend-architecture-guide.md` 目录结构
- `pkg/` 不得引用 `internal/`；`internal/` 可引用 `pkg/`
- `internal/ws/` 目录可能需要新建（当前只有 doc.go 或不存在）
- import 分组：标准库 → 第三方 → 本项目

### 本 story 的边界（不做什么）

- **不实现真实 WS Hub**（Story 0.9）— 只创建 HubStub 占位
- **不实现 cron heartbeat**（Story 0.8）— lastCronTick 字段在 key 不存在时返回空字符串
- **不实现 logx 包**（Story 0.5）— 使用全局 zerolog logger
- **不实现 middleware**（后续 story）
- **不实现 AppError 错误体系**（Story 0.6）— healthz 直接返回 JSON，不走统一错误处理
- **不实现 request ID middleware**（Story 0.5）

### References

- [Source: docs/backend-architecture-guide.md §4] — `handler.NewHealthHandler(mongoCli, rdb)` 示范
- [Source: _bmad-output/planning-artifacts/epics.md, Story 0.4] — 完整验收标准
- [Source: _bmad-output/planning-artifacts/epics.md, FR40] — `/healthz` 多维健康探测
- [Source: _bmad-output/planning-artifacts/prd.md, NFR-OBS-4] — healthz 多维检查要求
- [Source: _bmad-output/planning-artifacts/prd.md, NFR-SCALE-4] — WS 单实例 ≤ 10k 连接
- [Source: _bmad-output/planning-artifacts/architecture.md §16] — 测试标准
- [Source: _bmad-output/implementation-artifacts/0-3-infra-connectivity-and-clients.md] — Story 0.3 完成记录

## Dev Agent Record

### Agent Model Used

Claude Opus 4.6 (1M context)

### Debug Log References

### Completion Notes List

- HealthHandler 从无参简单 `{"status":"ok"}` 重写为多维健康检查：Mongo ping + Redis ping + WS hub goroutine count + cron:last_tick Redis key
- Mongo/Redis ping 并行执行（sync.WaitGroup），确保 p95 ≤ 50ms
- 新增 `GET /readyz` endpoint，使用 `sync/atomic.Bool` 标记启动完成状态
- App.OnReady 回调机制：在所有 Runnable goroutine 启动后设置 ready 标志
- WS Hub 占位：`internal/ws/hub_stub.go` 实现 WSHubChecker 接口，GoroutineCount 返回 0
- InfraChecker 接口：mongox.Client 和 redisx.Client 自动满足
- WrapClient 辅助函数：mongox/redisx 新增用于集成测试包装原始客户端
- 单元测试 8 个用例：全健康、Mongo 故障、Redis 故障、WS Hub 超限、cron tick 缺失、cron tick 存��、readyz ready、readyz not ready
- 集成测试 5 个用例：全健康、Mongo 容器停止后 503 不 panic、p95 ≤ 50ms、readyz、cron tick

### Change Log

- 2026-04-17: Story 0.4 实现完成 — 多维健康检查 endpoint（healthz + readyz）

### File List

- internal/handler/health_handler.go (rewritten — 多维健康检查 + readyz)
- internal/handler/health_handler_test.go (rewritten — 8 unit tests)
- internal/handler/health_handler_integration_test.go (new — 5 integration tests, //go:build integration)
- internal/ws/hub_stub.go (new — WS Hub 占位)
- internal/config/config.go (modified — 添加 WSCfg)
- internal/config/config_test.go (modified — 验证 WSCfg)
- config/default.toml (modified — 添加 [ws] section)
- cmd/cat/initialize.go (modified — 注入 HealthHandler 依赖 + OnReady 回调)
- cmd/cat/wire.go (modified — 添加 /readyz 路由)
- cmd/cat/app.go (modified — 添加 OnReady 回调机制)
- pkg/mongox/client.go (modified — 添加 WrapClient 辅助)
- pkg/redisx/client.go (modified — 添加 WrapClient 辅助)
