# Story 0.2: 应用入口与 Runnable 生命周期

Status: done

## Story

As a maintainer,
I want the app entry point wired with explicit DI and graceful shutdown per constitution §4-§5,
so that all future Runnable components (HTTP server, WS hub, cron scheduler, APNs worker) plug into one lifecycle with deterministic startup and shutdown order.

## Acceptance Criteria

1. **Given** Story 0.1 交付的 repo 骨架, **When** 实现 `cmd/cat/{main.go, initialize.go, app.go, wire.go}` + `internal/config/config.go`, **Then** `main.go` 行数 ≤ 15（严格，宪法 §4），仅做 `flag.Parse → config.MustLoad → initialize → app.Run`

2. **Given** AC#1, **Then** `initialize.go` 是唯一的显式 DI 装配点，行数 ≤ 200，无 DI 框架、无 IoC 容器

3. **Given** AC#1, **Then** 定义 `Runnable` 接口含 `Name() string / Start(ctx context.Context) error / Final(ctx context.Context) error`；`Final` 幂等

4. **Given** AC#3, **Then** `App` 容器按注册顺序并发 Start，收到 SIGTERM/SIGINT 后逆序 Final

5. **Given** AC#4, **Then** `signal.Notify` 监听 `os.Interrupt, syscall.SIGTERM`；shutdown 总耗时预算 ≤ 30 秒（宪法 §5）

6. **Given** AC#1, **Then** 无 `func init()` 做业务 I/O

7. **Given** AC#1, **Then** 无 `sync.Map` / 全局变量 / singleton 存连接或计数器

8. **Given** AC#1, **Then** `internal/config/config.go` 使用 `BurntSushi/toml` 实现 `MustLoad(path)` + 基础字段空值校验；配置仅在 `initialize()` 读取一次

9. **Given** AC#1, **Then** 启动日志首行 info 级输出 `build_version`（go build -ldflags）+ `config_hash`（config 文件 SHA256 前 8 位）

10. **Given** all above, **Then** 集成测试：`./build/catserver -config config/local.toml` 启动后 `kill -TERM <pid>` 能在 30s 内优雅退出，退出码 0

## Tasks / Subtasks

- [x] Task 1: 实现 `internal/config/config.go` — TOML 配置加载 (AC: #8)
  - [x] 1.1 定义 `Config` struct 映射 `default.toml` 所有字段（嵌套 `ServerCfg`, `LogCfg`, `MongoCfg`, `RedisCfg`, `JWTCfg`, `APNsCfg`, `CDNCfg`）
  - [x] 1.2 实现 `MustLoad(path string) *Config`：使用 `BurntSushi/toml` 解析，失败 `log.Fatal`
  - [x] 1.3 实现 `mustValidate()` 空值校验：server.port 为 0 则 `log.Fatal`
  - [x] 1.4 计算 config 文件 SHA256 前 8 位作为 `config_hash`
  - [x] 1.5 编写 `config_test.go` 单元测试（正常加载 + hash 确定性验证）

- [x] Task 2: 实现 `cmd/cat/app.go` — Runnable 接口与 App 容器 (AC: #3, #4, #5)
  - [x] 2.1 定义 `Runnable` 接口：`Name() string`, `Start(ctx context.Context) error`, `Final(ctx context.Context) error`
  - [x] 2.2 实现 `App` struct：`runs []Runnable`, `stop chan os.Signal`
  - [x] 2.3 实现 `NewApp(...Runnable) *App`
  - [x] 2.4 实现 `App.Run()`：并发 Start 所有 Runnable → 阻塞等待信号 → 30s timeout context → 逆序 Final
  - [x] 2.5 编写 `app_test.go`（mock Runnable 验证 Start/Final、逆序 Final、空 Runnable 场景）

- [x] Task 3: 实现 `cmd/cat/initialize.go` — 显式 DI 装配 (AC: #2, #9)
  - [x] 3.1 实现 `initialize(cfg *config.Config) *App`：装配 health handler + HTTP server（Gin）
  - [x] 3.2 `buildVersion` 变量声明在 initialize.go（默认 "dev"，ldflags 可覆盖）
  - [x] 3.3 启动首行 info 日志输出 build_version + config_hash

- [x] Task 4: 实现 `cmd/cat/wire.go` — 路由聚合 (AC: #1)
  - [x] 4.1 定义 `handlers` struct 聚合 HealthHandler
  - [x] 4.2 实现 `buildRouter(cfg, handlers) *gin.Engine`：注册 `/healthz` 路由
  - [x] 4.3 `httpServer` struct 实现 Runnable 接口（Name/Start/Final）

- [x] Task 5: 重写 `cmd/cat/main.go` — 入口 (AC: #1)
  - [x] 5.1 `flag.Parse → config.MustLoad → initialize → app.Run`，行数 = 15

- [x] Task 6: 添加最小 health handler (AC: #10 辅助验证)
  - [x] 6.1 `internal/handler/health_handler.go`：`GET /healthz` 返回 `{"status":"ok"}`
  - [x] 6.2 `health_handler_test.go`：验证 200 + JSON 响应

- [x] Task 7: 添加核心依赖到 go.mod (AC: all)
  - [x] 7.1 `go get` gin, toml, zerolog, testify；go.mod 升级到 Go 1.25（gin v1.12.0 要求）

- [x] Task 8: build_version ldflags 集成 (AC: #9)
  - [x] 8.1 `var buildVersion = "dev"` 声明在 `cmd/cat/initialize.go`
  - [x] 8.2 确认 `scripts/build.sh` 当前未传 ldflags（后续 CI story 可添加）

- [x] Task 9: 验证 (AC: #10)
  - [x] 9.1 创建 `config/local.toml` 用于本地测试（.gitignore 已排除）
  - [x] 9.2 `bash scripts/build.sh` 编译通过
  - [x] 9.3 `bash scripts/build.sh --test` 测试通过（config 2 tests, app 3 tests, handler 1 test）
  - [x] 9.4 手动启动验证：启动日志含 build_version + config_hash，/healthz 返回 200 OK

## Dev Notes

### Architecture Constraints (MANDATORY)

**宪法 §4 入口规范：**
- `main.go` ≤ 15 行，仅 `flag.Parse → config.MustLoad → initialize → app.Run`
- `initialize.go` ≤ 200 行，是唯一的 DI 装配点，无框架
- 构造函数内不做 I/O，除 `MustConnect` 的一次性连接+ping（本 story 不涉及 MustConnect）
- 禁止 `func init()` 做任何业务 I/O

**宪法 §5 Runnable 接口：**
- `Runnable` 接口：`Name() string / Start(ctx) error / Final(ctx) error`
- `App.Run()`：并发 Start → 阻塞等信号 → 30s timeout → 逆序 Final
- `Final` 必须幂等
- 实现 Runnable 的对象：HTTP server、WS hub、cron scheduler、mongo client、redis client

**Graceful Shutdown 顺序（架构文档定义，本 story 仅实现框架）：**
1. 停止接受新 HTTP/WS 连接（Gin `Shutdown(ctx)`）
2-7. 后续 story 逐步添加更多 Runnable

**总耗时预算 ~20s（宪法限 30s）**

### Source Tree — 本 story 需要 touch 的文件

```
cmd/cat/main.go          — 重写（当前 placeholder → 真实入口）
cmd/cat/initialize.go    — 新建
cmd/cat/app.go           — 新建
cmd/cat/wire.go          — 新建
internal/config/config.go — 重写（当前 doc.go placeholder → 真实实现）
internal/config/config_test.go — 新建
internal/handler/health_handler.go — 重写（当前 doc.go → 最小 /healthz）
config/default.toml      — 可能需微调字段名以匹配 Go struct tags
config/local.toml.example — 同步更新
```

### Config Struct 设计指引

按 `default.toml` 当前结构映射：

```go
type Config struct {
    Server   ServerCfg   `toml:"server"`
    Log      LogCfg      `toml:"log"`
    Mongo    MongoCfg    `toml:"mongo"`
    Redis    RedisCfg    `toml:"redis"`
    JWT      JWTCfg      `toml:"jwt"`
    APNs     APNsCfg     `toml:"apns"`
    CDN      CDNCfg      `toml:"cdn"`
    Hash     string      `toml:"-"` // config file SHA256 前 8 位，运行时计算
}
```

`MustLoad` 流程：读文件 bytes → 计算 SHA256 → toml.Decode → mustValidate → 设置 Hash

### HTTP Server as Runnable

```go
type httpServer struct {
    srv *http.Server
}
func (h *httpServer) Name() string { return "http_server" }
func (h *httpServer) Start(ctx context.Context) error { return h.srv.ListenAndServe() }
func (h *httpServer) Final(ctx context.Context) error { return h.srv.Shutdown(ctx) }
```

Gin engine 通过 `http.Server{Handler: router}` 包装。

### build_version 注入

```go
// cmd/cat/main.go
var buildVersion = "dev"
```

编译时通过 ldflags 注入：`-ldflags "-X main.buildVersion=$(git describe --tags --always --dirty)"`

检查 `scripts/build.sh` 是否已支持 ldflags，如未支持则需修改。

### Logging 策略（本 story 最小实现）

本 story 需要 zerolog 做启动日志。由于 Story 0.5 才正式实现 `pkg/logx/`，本 story 直接使用 zerolog 的 `log` 全局 logger（`github.com/rs/zerolog/log`），不创建 `logx` 包的正式实现。

启动首行日志：
```go
log.Info().
    Str("build_version", buildVersion).
    Str("config_hash", cfg.Hash).
    Msg("server starting")
```

### 本 story 的边界（不做什么）

- **不连 MongoDB/Redis**（Story 0.3）
- **不实现完整 logx 包**（Story 0.5）
- **不实现 AppError**（Story 0.6）
- **不实现 middleware**（request_id/recover/logger 在 Story 0.5 后）
- **不实现 JWT**（Story 1.3）
- `/healthz` 仅返回静态 `{"status":"ok"}`，不检查外部依赖（Story 0.4 扩展）
- `initialize.go` 当前只装配 config + HTTP server；后续 story 逐步添加

### Testing Standards

- 文件命名：`xxx_test.go` 同目录
- Table-driven 多场景测试
- 使用 `testify`：`require.NoError` / `assert.Equal`
- 单元测试默认开 `t.Parallel()`
- `config_test.go`：测试正常加载、字段校验失败
- `app_test.go`：mock Runnable 验证启动/停机顺序

### Previous Story Intelligence (0.1)

Story 0.1 已完成，建立了以下基础：
- Go module: `github.com/huing/cat/server` (Go 1.24.2)
- 全部目录结构已创建，各 internal/ 和 pkg/ 子包有 `doc.go` placeholder
- `config/default.toml` 已有完整字段骨架
- `scripts/build.sh` 在 `server/` 的上级目录（`C:\fork\cat\scripts\build.sh`），不要修改
- docker-compose.yml 已配置 MongoDB + Redis（本 story 不需要启动）
- CI pipeline、golangci-lint、lefthook 均已配置
- 代码审查修正：移除了 Dockerfile/App 容器（无 Docker 构建需求）、lefthook glob 修正为 `**/*.go`

### Project Structure Notes

- 严格遵循 `docs/backend-architecture-guide.md` 目录结构
- `cmd/cat/` 下 4 个文件：main.go, initialize.go, app.go, wire.go
- `internal/config/config.go` 替换现有 `doc.go` placeholder
- 现有 `doc.go` placeholder 文件在被真实实现替换时应删除

### References

- [Source: docs/backend-architecture-guide.md §4] — 入口与初始化规范（main.go ≤15 行、initialize.go 显式 DI）
- [Source: docs/backend-architecture-guide.md §5] — Runnable 接口完整代码示例 + App.Run() 实现
- [Source: docs/backend-architecture-guide.md §9] — TOML 配置加载 MustLoad + mustValidate
- [Source: _bmad-output/planning-artifacts/epics.md, Story 0.2] — 完整验收标准
- [Source: _bmad-output/planning-artifacts/architecture.md, Baseline 代码文件清单] — S0.2 交付文件 #4/#5
- [Source: _bmad-output/planning-artifacts/architecture.md, Graceful Shutdown 顺序] — 逆序 Final 规范

## Dev Agent Record

### Agent Model Used

Claude Opus 4.6 (1M context)

### Debug Log References

### Completion Notes List

- Config 包实现：MustLoad 使用 BurntSushi/toml 解析，SHA256 前 8 位作为 config_hash，mustValidate 校验 server.port
- Runnable 接口 + App 容器：并发 Start，SIGTERM/SIGINT 触发逆序 Final，30s shutdown timeout
- initialize.go：显式 DI 装配点，当前仅装配 health handler + HTTP server
- wire.go：handlers 聚合 + buildRouter + httpServer Runnable 包装
- main.go 精确 15 行：flag.Parse → config.MustLoad → initialize → app.Run
- health handler：GET /healthz → {"status":"ok"}
- Go 1.25（gin v1.12.0 要求升级）
- buildVersion 默认 "dev"，scripts/build.sh 暂未添加 ldflags（后续 story 或 CI 可补充）
- 已删除被替换的 doc.go placeholder 文件（internal/config/doc.go, internal/handler/doc.go）

### Change Log

- 2026-04-17: Story 0.2 实现完成 — 应用入口、Runnable 生命周期、config 加载、最小 HTTP server + /healthz

### File List

- cmd/cat/main.go (modified — 重写为真实入口)
- cmd/cat/app.go (new)
- cmd/cat/app_test.go (new)
- cmd/cat/initialize.go (new)
- cmd/cat/wire.go (new)
- internal/config/config.go (new — 替换 doc.go)
- internal/config/config_test.go (new)
- internal/config/doc.go (deleted)
- internal/handler/health_handler.go (new — 替换 doc.go)
- internal/handler/health_handler_test.go (new)
- internal/handler/doc.go (deleted)
- config/local.toml (new — .gitignore 已排除)
- go.mod (modified — Go 1.25 + 新依赖)
- go.sum (new/modified)
