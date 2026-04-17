# Story 0-2: 应用入口与 Runnable 生命周期 — 实现总结

Story 0-2 把 Story 0-1 的空壳 `main.go` 变成了一个真正能跑起来的 HTTP 服务，建立了整个项目的启动/停机骨架。

## 做了什么

### 应用入口（cmd/cat/）

- `main.go` 精确 15 行，遵循宪法 §4 的"四步走"：`flag.Parse → config.MustLoad → initialize → app.Run`
- `initialize.go` 是唯一的显式 DI 装配点，目前只装配了 health handler 和 HTTP server，后续 story 在这里添加新依赖
- `wire.go` 聚合所有 handler，构建 Gin 路由，并把 Gin engine 包装成 `httpServer` 实现 Runnable 接口
- `buildVersion` 变量通过 `go build -ldflags` 注入（`scripts/build.sh` 已支持）

### Runnable 生命周期（cmd/cat/app.go）

- 定义了 `Runnable` 接口：`Name() string / Start(ctx) error / Final(ctx) error`
- `App` 容器并发启动所有 Runnable，收到 SIGTERM/SIGINT 后立即 cancel context（让 Start 循环退出），然后按注册的逆序调用 Final，30 秒超时
- 这是后续所有组件（WS hub、cron scheduler、APNs worker 等）接入的统一生命周期框架

### TOML 配置加载（internal/config/config.go）

- `Config` struct 映射 `default.toml` 的七个配置段（server/log/mongo/redis/jwt/apns/cdn）
- `MustLoad(path)` 先读文件原始字节算 SHA256 前 8 位作为 `config_hash`，再用 BurntSushi/toml 解析
- `mustValidate()` 校验关键字段（当前仅检查 port >= 0）
- 配置只在 `initialize()` 读取一次，之后通过参数传递，不存全局变量

### 最小 /healthz endpoint

- `internal/handler/health_handler.go`：`GET /healthz` 返回 `{"status":"ok"}`
- 后续 Story 0.4 会扩展为多维健康检查（Mongo/Redis 状态）

### 构建链路改进

- `scripts/build.sh` 新增 `--integration` flag，支持 `go test -tags=integration`
- build 时通过 ldflags 注入 `buildVersion`（`git describe --tags --always --dirty`）
- CI pipeline（`.github/workflows/ci.yml`）新增 integration test step，Go 版本升级到 1.25

## 怎么实现的

核心设计遵循 `docs/backend-architecture-guide.md` 的宪法 §4（入口）和 §5（Runnable 接口）。

HTTP server 通过 `http.Server{Handler: ginEngine}` 包装成 Runnable：Start 调用 `ListenAndServe`，Final 调用 `Shutdown(ctx)`。收到信号后先 `cancel()` 通知所有 Runnable 的 Start 上下文，再逆序 Final，这样长生命周期的 goroutine（如 WS hub 的读写循环）能在 shutdown 阶段及时退出。

config hash 的作用是在启动日志中标识当前配置版本，方便运维排查"是不是配置没更新"。

## 怎么验证的

- `bash scripts/build.sh --test`：3 个测试包通过（config 2 tests, app 3 tests, handler 1 test）
- 手动启动验证：启动日志输出 `build_version` + `config_hash`，`curl /healthz` 返回 200
- 集成测试（`//go:build integration`）：启动真实二进制 → SIGTERM → 验证 30s 内退出码 0（CI 在 Linux 上跑）

## 后续 story 怎么用

- **Story 0.3** 在 `initialize.go` 里加 `mongox.MustConnect` + `redisx.MustConnect`，两者都实现 Runnable（Final 时关闭连接）
- **Story 0.4** 扩展 `HealthHandler`，注入 Mongo/Redis client 做多维检查
- **Story 0.5** 实现 `pkg/logx/`，替换当前直接使用的 `zerolog/log` 全局 logger
- 所有新组件只需实现 `Runnable` 接口，在 `initialize.go` 里 `NewApp(...)` 注册即可自动获得启动/停机管理
