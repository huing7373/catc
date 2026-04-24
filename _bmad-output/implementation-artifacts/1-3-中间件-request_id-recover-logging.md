# Story 1.3: 中间件 request_id / recover / logging

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want 所有请求自动带 request_id 并打印结构化日志、panic 时不挂掉进程,
so that 后续每个业务接口都自动满足 NFR14（请求带 request_id）和 NFR15（结构化日志）.

## 故事定位（节点 1 第三条实装 story）

- Story 1.2 已建立 `gin.New()` **零中间件**的裸引擎 + `/ping`。本 story 把三件套中间件 + `log/slog` JSON logger + prometheus metrics 一次性挂上来。
- ADR-0001（`_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.6 / §4 / §5 / §7 Follow-ups）已锁：
  - Logger = `log/slog`（Go 1.22+ stdlib）+ 自定义 JSON Handler
  - Metrics = `github.com/prometheus/client_golang v1.23.2`
  - Structured log 6 字段：`request_id` / `user_id` / `api_path` / `latency_ms` / `business_result` / `error_code`
  - Story 1.3 同步注册 2 个 prometheus metric：`cat_api_requests_total` Counter + `cat_api_request_duration_seconds` Histogram
- **不涉及**（严格按需引入原则 `MVP节点规划与里程碑.md` §2 原则 7）：
  - **auth / rate_limit 中间件** → Epic 4 Story 4.5
  - **AppError 框架 + `error_code` 字段落地** → Story 1.8（本 story 的 recover 直接返回 `code=1009, message="服务繁忙"`，先用纯 literal，不提前引 AppError）
  - **`user_id` 字段真实注入** → Epic 4 Story 4.5（本 story 的 log 字段**不**输出 `user_id` / `business_result` / `error_code`，缺失即"未注入"）
  - **ctx propagation 决策文档** → Story 1.9（0007-context-propagation.md）；本 story **遵守**"handler 用 `c.Request.Context()`"的 Gin 默认，但不落地文档
  - **domain metrics**（chest / compose / rooms_active / ws_connections_active）→ 各自 Epic
  - **测试基础设施依赖**（testify / sqlmock / miniredis）→ Story 1.5；本 story 测试继续用 stdlib `testing`

**范围红线**：
- 只建 `internal/app/http/middleware/` + `internal/infra/logger/` + `internal/infra/metrics/` 三个新包
- **不**建 `internal/app/http/request/` / `internal/app/http/response/`（业务接口才开始用；Epic 2 iOS / Epic 4 Server 开始时建）
- **不**在 `internal/app/bootstrap/` 下新增 `logger.go`（`Go项目结构与模块职责设计.md` §4 建议位置为 `bootstrap/logger.go`，但本 story 把 slog 初始化放 `internal/infra/logger/` 更符合"infra 层负责外部设施"的分层理念 —— 变更详情见 Dev Notes §Project Structure Notes）
- **不**动 Story 1.2 已落地的 `config.Load` / `config.LocateDefault` / `bootstrap.Run` 主骨架，只在 `bootstrap.NewRouter` / `bootstrap.Run` 里**新增**挂载点

## Acceptance Criteria

**AC1 — 依赖版本锁**

`server/go.mod` 的 `require` 段追加：

- `github.com/prometheus/client_golang v1.23.2`（ADR-0001 §6 锁定版本）
- `github.com/google/uuid v1.6.0`（UUID v4 生成；Go 生态事实标准，单一职责库，版本稳定 3 年+；ADR-0001 §4 描述 "UUIDv7 / ULID" 是"**缺省**生成策略"，v4 是最低可接受版本，dev 可自选 v7 如觉得合适）

`go mod tidy` 后确认 `go.sum` 更新，提交。

**不**在本 story 安装：testify / sqlmock / miniredis / dockertest（Story 1.5）。

**AC2 — 目录与文件**

必须建立：

```
server/
├─ internal/
│  ├─ app/
│  │  └─ http/
│  │     └─ middleware/                  # 新增目录
│  │        ├─ request_id.go
│  │        ├─ request_id_test.go
│  │        ├─ recover.go
│  │        ├─ recover_test.go
│  │        ├─ logging.go
│  │        └─ logging_test.go
│  └─ infra/
│     ├─ logger/                         # 新增目录
│     │  ├─ slog.go                      # slog JSON Handler 初始化 + ctx 传播
│     │  └─ slog_test.go
│     └─ metrics/                        # 新增目录
│        ├─ registry.go                  # prom Registry + HTTP Handler helper
│        ├─ http.go                      # cat_api_requests_total + cat_api_request_duration_seconds
│        └─ http_test.go
```

必须修改：

- `server/internal/app/bootstrap/router.go`：挂载 3 个中间件 + 注册 `GET /metrics`（非业务接口，不在 `/api/v1` 前缀下）
- `server/internal/app/bootstrap/server.go`：在 `Run` 入口调用 `logger.Init(cfg)`，把返回的 `*slog.Logger` 作为 default
- `server/cmd/server/main.go`：把 `log.Fatalf` / `log.Printf` 全部换成 `slog.Error(...)` + `os.Exit(1)`（stdlib `log` 在 Story 1.2 作为过渡用，本 story 完成后应切 slog；否则 JSON log 里混 stdlib text log 不一致）
  - **保留**：Story 1.2 `bootstrap/server.go` 里的 `log.Printf("server started on %s", addr)` / `log.Printf("server stopped")` —— 但改成 `slog.Info` 调用
- `server/internal/pkg/response/response.go`：`requestIDFromCtx` 改成**先**读 `c.Get("request_id")`（middleware 注入），**后**回落 `X-Request-Id` header，**最终**回落 `"req_xxx"`（该回落在本 story 挂完中间件后理论上不会再触发，但作为安全兜底保留）

**不**建立（留给后续 story）：`internal/app/http/request/` / `internal/app/http/response/` / `internal/app/bootstrap/logger.go` / `internal/app/http/middleware/auth.go` / `internal/app/http/middleware/rate_limit.go`。

**AC3 — `log/slog` 初始化（`internal/infra/logger/slog.go`）**

提供：

- `Init(cfg *config.Config) *slog.Logger`：
  - 从 `cfg.Log.Level` 解析 level（`"debug" / "info" / "warn" / "error"` 不区分大小写；无法识别 → 默认 `info` 并打印一行 warn 级别日志"unknown log level '...', fallback to info"）
  - 构造 `slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl})` 作为根 handler
  - `slog.SetDefault(logger)` 全局设置
  - 返回 `*slog.Logger`，方便测试断言
- `NewContext(ctx context.Context, logger *slog.Logger) context.Context`：把 logger 塞进 ctx（使用内部 key 类型避免冲突）
- `FromContext(ctx context.Context) *slog.Logger`：从 ctx 取 logger；取不到返回 `slog.Default()`
- **JSON Handler 输出格式**：每行一条 JSON，字段顺序由 slog 决定（不强制）；`time` 字段用 RFC3339Nano；`level` 字段用 `INFO` / `WARN` / `ERROR` 等大写枚举；`msg` 字段为 log message

**禁止**：向 slog 写入敏感字段（暂无，Epic 4 之后需留意 `Authorization` / `password` / 手机号等，Story 1.3 先留注释提醒）。

**AC4 — request_id 中间件（`middleware/request_id.go`）**

```go
func RequestID() gin.HandlerFunc {
    return func(c *gin.Context) {
        rid := c.GetHeader("X-Request-Id")
        if rid == "" {
            rid = uuid.NewString()   // UUID v4
        }
        c.Set("request_id", rid)
        c.Header("X-Request-Id", rid)
        c.Next()
    }
}
```

**契约**：
- 传入 `X-Request-Id` header → **原样透传**（不校验格式，不加前缀）
- 未传 → 生成 UUID v4（`google/uuid.NewString()`）
- **响应** `X-Request-Id` header **总是**被设置（含未传入场景下生成的）
- **context key** = 字符串 `"request_id"`（和 AC5 `response.requestIDFromCtx` 读的 key 一致）

**AC5 — logging 中间件（`middleware/logging.go`）**

> ⚠️ **中间件顺序关键**：挂载必须是 `request_id → logging → recover → handler`（见 AC8）。原因：Gin middleware 链 + Go panic unwind 语义意味着**logging 必须在 recover 的"外层"才能在 panic 情况下仍跑完 log**（panic 从 handler 冒上来时，先被内层 recover 的 `defer` 吃掉 → recover 正常返回 → logging 的"after `c.Next()`" 代码才有机会执行，读到 `c.Writer.Status() = 500` 并打日志）。反过来把 recover 放外层 → logging 的后续代码会因为 panic 跳过，这条请求在日志里消失。

```go
import (
    "log/slog"
    "time"

    "github.com/gin-gonic/gin"

    "github.com/huing/cat/server/internal/infra/logger"
    "github.com/huing/cat/server/internal/infra/metrics"
)

func Logging() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()

        // 在 ctx 上绑定当前请求的 child logger（request_id + api_path）
        rid, _ := c.Get("request_id")
        ridStr, _ := rid.(string)
        reqLogger := slog.Default().With(
            slog.String("request_id", ridStr),
            slog.String("api_path", c.FullPath()),
        )
        ctx := logger.NewContext(c.Request.Context(), reqLogger)
        c.Request = c.Request.WithContext(ctx)

        c.Next()

        latency := time.Since(start)
        status := c.Writer.Status()
        reqLogger.InfoContext(ctx, "http_request",
            slog.String("method", c.Request.Method),
            slog.Int("status", status),
            slog.Int64("latency_ms", latency.Milliseconds()),
            slog.String("client_ip", c.ClientIP()),
        )

        // Metrics：2 指标同时更新
        metrics.ObserveHTTP(c.FullPath(), c.Request.Method, status, latency)
    }
}
```

**契约**：
- log message 固定 `"http_request"`；关键字段以 `slog.Attr` 形式附加
- **6 字段状态**（本 story 阶段）：
  - `request_id` ✅ 必须有
  - `user_id` ❌ **不输出**（Epic 4 Story 4.5 才接 auth 上下文；字段缺失而非空串）
  - `api_path` ✅ 必须有（`c.FullPath()`，未命中路由时为空串 —— 不要用 `c.Request.URL.Path` 防止 metric cardinality 爆炸）
  - `latency_ms` ✅ 必须有（int64 毫秒）
  - `business_result` ❌ **不输出**（service 层主动写；Story 1.3 不涉及 service）
  - `error_code` ❌ **不输出**（Story 1.8 AppError 接入后，从 `c.Errors` 读）
- 额外字段：`method`、`status`、`client_ip` —— 虽然 ADR-0001 §4 不列入"6 字段"，但 epics.md Story 1.3 AC 明确要求，故保留
- **挂载位置**：`request_id` 之后、**`recover` 之前**（关键，见顶部说明）
- **Metrics 调用**：必须在 log 之后、函数返回前；让 `metrics.ObserveHTTP` 读到 final status
- **变量名避坑**：不要写 `logger := slog.Default().With(...)` —— 会 shadow 导入的 `logger` 包；用 `reqLogger` 作局部名

**AC6 — recover 中间件（`middleware/recover.go`）**

```go
import (
    "log/slog"
    "net/http"
    "runtime/debug"

    "github.com/gin-gonic/gin"

    "github.com/huing/cat/server/internal/infra/logger"
    "github.com/huing/cat/server/internal/pkg/response"
)

func Recovery() gin.HandlerFunc {
    return func(c *gin.Context) {
        defer func() {
            if rec := recover(); rec != nil {
                stack := debug.Stack()
                reqLogger := logger.FromContext(c.Request.Context())
                reqLogger.ErrorContext(c.Request.Context(), "handler panic",
                    slog.Any("panic", rec),
                    slog.String("stack", string(stack)),
                )
                response.Error(c, http.StatusInternalServerError, 1009, "服务繁忙")
                c.Abort()
            }
        }()
        c.Next()
    }
}
```

**契约**：
- **必须**在 request_id 中间件**之后**挂（让 panic response envelope 的 `requestId` 正确）
- **必须**在 logging 中间件**之后**挂（关键！见 AC5 顶部说明）
- log level = `Error`；包含 `panic` 字段（原始 recover 值，用 `slog.Any` 防止 non-string panic 报错）和 `stack` 字段（`debug.Stack()` 字符串）
- HTTP 500 + `response.Error` 统一封装；**固定** `code=1009, message="服务繁忙"`（Story 1.8 AppError 就位后会被 ErrorMappingMiddleware 替代，本 story 先用 literal）
- `c.Abort()` **必须**调；否则 Gin 会继续执行后续 handler 链
- `logger.FromContext` 在"logging 中间件已建好 ctx"的前提下拿到含 `request_id + api_path` 的 child logger —— 因为 logging 在 recover 外层、handler 内层，recover 运行时 logging 已经把 child 塞进 ctx
- **变量名避坑**：同 AC5，用 `reqLogger` 不用 `logger` 作局部名

**AC7 — Prometheus metrics（`internal/infra/metrics/`）**

`registry.go`：
```go
var Registry = prometheus.NewRegistry()

// Handler returns the prometheus HTTP handler bound to Registry.
func Handler() http.Handler {
    return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{})
}
```

`http.go`：
```go
var (
    httpRequestsTotal = promauto.With(Registry).NewCounterVec(
        prometheus.CounterOpts{
            Name: "cat_api_requests_total",
            Help: "Total HTTP requests by api_path, method, code.",
        },
        []string{"api_path", "method", "code"},
    )
    httpRequestDuration = promauto.With(Registry).NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "cat_api_request_duration_seconds",
            Help:    "HTTP request duration by api_path, method.",
            Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
        },
        []string{"api_path", "method"},
    )
)

func ObserveHTTP(apiPath, method string, statusCode int, latency time.Duration) {
    if apiPath == "" {
        apiPath = "UNKNOWN"   // 避免空 label value 让 prom 刷全路由
    }
    code := strconv.Itoa(statusCode)
    httpRequestsTotal.WithLabelValues(apiPath, method, code).Inc()
    httpRequestDuration.WithLabelValues(apiPath, method).Observe(latency.Seconds())
}
```

**契约**：
- 全局 `Registry`（**不**用 `prometheus.DefaultRegisterer`，避免被 Go stdlib 的默认 metrics 污染）
- metric 名称以 `cat_` 前缀（ADR-0001 §5）
- buckets 固定 10 档（ADR-0001 §5 推荐）
- `api_path` 必须用 Gin `c.FullPath()`（已在 logging 中间件里拿好传入）；空串时用字面量 `"UNKNOWN"` 占位
- 暴露端点：`GET /metrics`（AC8 挂载；`promhttp.HandlerFor(Registry, ...)` 返回的 `http.Handler`，通过 `gin.WrapH` 适配）

**AC8 — router 挂载顺序（`bootstrap/router.go`）**

```go
func NewRouter() *gin.Engine {
    r := gin.New()
    r.Use(middleware.RequestID())    // 1st: 最先，让后续都能拿到 request_id
    r.Use(middleware.Logging())      // 2nd: 包在 recover 外层，panic 也能被计入 log
    r.Use(middleware.Recovery())     // 3rd: 贴近 handler，defer 吃掉 panic
    r.GET("/ping", handler.PingHandler)
    r.GET("/metrics", gin.WrapH(metrics.Handler()))
    return r
}
```

**契约**：
- 顺序**严格**：request_id → logging → recover → handler 链（反过来会导致 panic 请求在日志里消失；详见 AC5 顶部说明）
- `/metrics` 挂在**所有业务路由之外**；不加 `/api/v1` 前缀（同 `/ping` / `/version` 运维端点）
- `/metrics` **暴露在主 HTTP server**（复用 `cfg.Server.HTTPPort`）；ADR-0001 §5 提到"Story 10-3 之后考虑是否独立端口"，节点 1 阶段共用即可
- `/metrics` **不**在 `RequestID` / `Logging` middleware 中产生额外 latency noise？→ 会产生 —— 所有请求都走这 3 个中间件，含 `/metrics` 自身。这导致 `/metrics` 自己会在 `cat_api_requests_total` 里被记。**接受这个 trade-off**（符合直觉，易于监控 scrape 是否工作）；如要屏蔽 `/metrics` 自身，需在 logging 中间件开头加 `if c.FullPath() == "/metrics" { c.Next(); return }` 的 skip 分支 —— 本 story **不做**，Story 10-3 考虑是否引入 skip list

**AC9 — `main.go` / `bootstrap/server.go` 适配 slog**

- `main.go`：
  - 删掉 `"log"` import，换 `"log/slog"` + `"os"`
  - 所有 `log.Fatalf(...)` → `slog.Error(...)` + `os.Exit(1)`
  - `log.Printf("config loaded: ...")` → `slog.Info("config loaded", slog.Int("http_port", cfg.Server.HTTPPort), slog.String("log_level", cfg.Log.Level))`
  - 在 `config.Load` 之后、`bootstrap.Run` 之前，调用 `logger.Init(cfg)` 把 slog 默认 logger 初始化
- `bootstrap/server.go`：
  - 删掉 `"log"` import，换 `"log/slog"`
  - `log.Printf("server started on %s", addr)` → `slog.Info("server started", slog.String("addr", addr))`
  - `log.Printf("server stopped")` → `slog.Info("server stopped")`
  - `log.Printf("server shutdown error: %v", err)` → `slog.Error("server shutdown error", slog.Any("error", err))`
- **不**动 `bootstrap.Run` 的 `net.Listen` / `srv.Serve` 拆分逻辑（Story 1.2 review fix 的核心，见 `docs/lessons/2026-04-24-config-path-and-bind-banner.md` Lesson 2）

**AC10 — `response.requestIDFromCtx` 更新**

修改 `internal/pkg/response/response.go`：

```go
func requestIDFromCtx(c *gin.Context) string {
    if v, ok := c.Get("request_id"); ok {
        if rid, ok := v.(string); ok && rid != "" {
            return rid
        }
    }
    if v := c.Request.Header.Get("X-Request-Id"); v != "" {
        return v
    }
    return "req_xxx"
}
```

**契约**：
- 优先读 `c.Get("request_id")`（request_id 中间件设置）
- 读不到回落 header（测试或直接打 handler 场景）
- 再读不到回落 `"req_xxx"`（极端情况；正常流程下本 story 完成后不会触发）

**AC11 — 单元测试覆盖（≥ 8 case）**

> 依旧用 stdlib `testing`，**不**引入 testify（Story 1.5）。

**`request_id_test.go`（2 case）**：

| # | case | 构造 | 断言 |
|---|---|---|---|
| 1 | happy: 未传 header → 生成 UUIDv4 | `httptest.NewRequest` 不带 `X-Request-Id` | 响应 header `X-Request-Id` 匹配 UUIDv4 正则 `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`；`c.Get("request_id")` 等于同值 |
| 2 | edge: 传入自定义 `X-Request-Id` | 请求带 header `"my-custom-rid-123"` | 响应 header 等于 `"my-custom-rid-123"`（原样透传）；`c.Get("request_id")` 等值 |

**`recover_test.go`（2 case）**：

| # | case | 构造 | 断言 |
|---|---|---|---|
| 1 | happy: handler panic string | 在测试 router 注册 `/panic` 的 handler 主动 `panic("oops")` | 响应 status = 500；body JSON `code==1009` / `message=="服务繁忙"` / `data=={}` / `requestId` 非空；log buffer 含 `"handler panic"` + `panic=oops` + `stack` 字段非空 |
| 2 | edge: handler panic 非 string | `panic(errors.New("boom"))` | 同上 #1；`panic` 字段为 error 序列化字符串（不因类型 panic 二次 panic）|

**`logging_test.go`（2 case）**：

| # | case | 构造 | 断言 |
|---|---|---|---|
| 1 | happy: 正常请求 | `GET /ping` → 200 | log buffer 含 **1 行 JSON**；字段齐备：`request_id`（非空）、`api_path=="/ping"`、`latency_ms`（>=0）、`method=="GET"`、`status==200`、`client_ip` 非空、`msg=="http_request"` |
| 2 | edge: 未命中路由 | `GET /nonexistent` → 404 | `api_path` 为 `""`（Gin `c.FullPath()` 在未命中时返回空串，但 metrics 这边 `ObserveHTTP` 会换成 `"UNKNOWN"`；log 里原样空串或缺失字段 —— 明确选一种：建议 **log 里保留空串**，便于 grep 出"有请求但未命中路由"的情况）|

**`metrics_http_test.go`（2 case）**：

| # | case | 构造 | 断言 |
|---|---|---|---|
| 1 | happy: 连续 3 次 /ping | 重置 Registry → 调 `ObserveHTTP("/ping", "GET", 200, 10ms)` × 3 | `httpRequestsTotal.WithLabelValues("/ping", "GET", "200")` Counter value == 3；`httpRequestDuration` Histogram count == 3 且 sum 约等于 30ms |
| 2 | edge: 空 path → UNKNOWN | `ObserveHTTP("", "GET", 404, 1ms)` | Counter 有 label `api_path="UNKNOWN"` 的样本值 == 1 |

**`slog_test.go`（2 case）**：

| # | case | 构造 | 断言 |
|---|---|---|---|
| 1 | happy: level 覆盖 | `Init(&Config{Log:{Level:"warn"}})` | 返回 logger 的 handler 对 `Info` 级别 `Enabled` 返回 false，对 `Warn` 返回 true |
| 2 | edge: level 非法回落 info | `Init(&Config{Log:{Level:"TRACE"}})` | 返回 logger 的 level 为 `Info`；log buffer 含"unknown log level"提示（或通过 stdlib slog.WarnContext 捕获 fallback 日志）|

**测试工具约定**：
- slog 断言用**内存 buffer** + 自定义 `slog.NewJSONHandler(&buf, ...)` 注入；解析 buffer 每行用 `json.Unmarshal` 拿字段
- Gin 测试 handler 用 `gin.TestMode`（Story 1.2 已在 `router_test.go` 示范）
- metrics 断言用 `prometheus/client_golang` 自带的 `testutil.ToFloat64(counter)` / `testutil.CollectAndCount(collector)` —— stdlib `testing` 完全可配合，不需要 testify

**AC12 — 集成测试（router 层）**

修改 `internal/app/bootstrap/router_test.go`（Story 1.2 已存在）**或**新增 `router_integration_test.go`，新增 **3 个** case：

| # | case | 断言 |
|---|---|---|
| 1 | 挂完中间件后 `/ping` 的 `requestId` 字段非 `"req_xxx"` 占位（而是 UUIDv4） | `env.RequestID` 匹配 UUIDv4 正则；响应 header `X-Request-Id` 与 body `requestId` 一致 |
| 2 | 故意注册一个 `/panic-test` 路由 panic，发起请求 → 500 + envelope；再发一个 `/ping` → 200（**服务进程未挂**） | 第一次响应 `status=500` / `code=1009`；第二次响应 `status=200` / `code=0` |
| 3 | `/metrics` 返回 200 + Content-Type 含 `text/plain; version=0.0.4` | body 含字符串 `cat_api_requests_total` 和 `cat_api_request_duration_seconds` |

Story 1.2 遗留的原 `TestRouter_Ping` case：由于挂中间件后 `requestId` 从占位变 UUIDv4，原测试的"非空即过"断言**继续成立**（未断言占位值）。**禁止**修改该 case 断言以迁就 Story 1.3 —— 应新增 case 2 / 3 / AC12 里的第 1 条作为 1.3 专属断言。

**AC13 — 验收 build 可跑**

- `cd server && go vet ./...` 无 error
- `cd server && go test ./...` 通过（含 Story 1.2 的 7 测试 + 本 story 新增 ≥ 8 测试 + 集成新增 3 case = 18+ case）
- `cd server && go build -o ../build/catserver.exe ./cmd/server/` 成功
- 从 repo root 启动 `./build/catserver.exe`，curl 三次：
  1. `curl http://127.0.0.1:8080/ping` → 响应 header 有 `X-Request-Id`（UUIDv4）；body 的 `requestId` 与 header 一致
  2. `curl -H "X-Request-Id: manual-test-abc" http://127.0.0.1:8080/ping` → 响应 header / body 的 `requestId` 都等于 `"manual-test-abc"`
  3. `curl http://127.0.0.1:8080/metrics | head -20` → 返回 prometheus 格式文本，含 `cat_api_requests_total` 和 `cat_api_request_duration_seconds`
- 启动时 server 终端输出是 **JSON 单行**格式（不再是 stdlib log 的 `2026/04/24 12:00:00 ...` 文本格式）
- dev 需把这三次 curl 的响应 body 或关键字段贴到 Completion Notes

## Tasks / Subtasks

- [x] **T1**：安装依赖（AC1）
  - [x] T1.1 `cd server && go get github.com/prometheus/client_golang@v1.23.2`
  - [x] T1.2 `go get github.com/google/uuid@v1.6.0`
  - [x] T1.3 `go mod tidy`，确认 `go.sum` 有新增条目

- [x] **T2**：slog logger（AC2 / AC3）
  - [x] T2.1 建 `internal/infra/logger/slog.go`
  - [x] T2.2 建 `internal/infra/logger/slog_test.go`（5 个 case；超过 spec 的 2 个）

- [x] **T3**：metrics（AC2 / AC7）
  - [x] T3.1 建 `internal/infra/metrics/registry.go`
  - [x] T3.2 建 `internal/infra/metrics/http.go`
  - [x] T3.3 建 `internal/infra/metrics/http_test.go`（2 case）

- [x] **T4**：request_id 中间件（AC2 / AC4 / AC11）
  - [x] T4.1 建 `internal/app/http/middleware/request_id.go`
  - [x] T4.2 建 `internal/app/http/middleware/request_id_test.go`（2 case）

- [x] **T5**：recover 中间件（AC2 / AC5 / AC11）
  - [x] T5.1 建 `internal/app/http/middleware/recover.go`
  - [x] T5.2 建 `internal/app/http/middleware/recover_test.go`（3 case；含一个额外的"第二次请求仍正常"）

- [x] **T6**：logging 中间件（AC2 / AC6 / AC11）
  - [x] T6.1 建 `internal/app/http/middleware/logging.go`
  - [x] T6.2 建 `internal/app/http/middleware/logging_test.go`（2 case）

- [x] **T7**：router 挂载 + /metrics 注册（AC8 / AC12）
  - [x] T7.1 修改 `bootstrap/router.go`：req_id → logging → recover 顺序；`GET /metrics` 挂载
  - [x] T7.2 更新 `bootstrap/router_test.go`：保留原 TestRouter_Ping + 新增 3 case

- [x] **T8**：切 slog 替换 stdlib `log`（AC9 + 迁移 Story 1.2 测试）
  - [x] T8.1 修改 `cmd/server/main.go`：切 slog + `logger.Init(cfg)`
  - [x] T8.2 修改 `bootstrap/server.go`：切 slog；保留 `net.Listen` / `srv.Serve` 拆分
  - [x] T8.3 迁移 `bootstrap/server_test.go`：新增 `captureSlog` helper 替换 `log.SetOutput`

- [x] **T9**：`response.requestIDFromCtx` 升级（AC10）
  - [x] T9.1 优先读 `c.Get("request_id")`

- [x] **T10**：本地验收（AC13）
  - [x] T10.1 `go vet ./...` pass
  - [x] T10.2 `go test ./...` pass（6 个包全绿）
  - [x] T10.3 `go build` pass（24 MB binary）
  - [x] T10.4 3 次 curl 验证完成
  - [x] T10.5 JSON log 贴在 Completion Notes

- [x] **T11**：收尾
  - [x] T11.1 curl 响应 + log 贴 Completion Notes
  - [x] T11.2 File List 填充
  - [x] T11.3 状态流转 `ready-for-dev → in-progress → review`

## Dev Notes

### 项目关键约束（必读，勿绕过）

1. **中间件挂载顺序不可变**：request_id → recover → logging → handler。任何交换都会破坏字段追踪（见 AC5/AC6 contract 块）。
2. **Story 1.2 review lesson 直接指导本 story**（`docs/lessons/2026-04-24-config-path-and-bind-banner.md` Lesson 2 的 Meta）：
   > 任何 "declare → execute" 的代码对，declare 不是 execute 的同步结果就会说谎。
   - 应用到本 story：log 里的 `status` 字段必须在 `c.Next()` **之后**读（才是 final status），不能在 `c.Next()` 前。同理 `latency_ms` 用 `time.Since(start)` 在 `c.Next()` 后算，不在前。
   - recover 中间件里 `c.Writer.Status()` 必须在 `response.Error` 之后读，否则 logging 里看到的还是 200 而不是 500。Gin 的 writer 是"写完 header 锁死 status"，所以 `c.Next()` 之后读就安全。
3. **logger 注入链路**：
   - main.go `logger.Init(cfg)` → `slog.SetDefault(logger)` 设全局 default
   - logging 中间件每请求拿 `slog.Default().With(request_id=..., api_path=...)` 生成 child，**通过 ctx 传下去**
   - handler 层要写 log 时 `slog.InfoContext(c.Request.Context(), "msg", ...)` 直接拿 child（Story 1.3 的 handler 只有 `/ping`，不写业务 log）
   - **切勿** `slog.Info("msg", ...)` 裸调用 —— 会丢 request_id / api_path 字段
4. **recover 的 panic 必须仍走 logging 中间件**：挂载顺序保证；panic 被 recover 捕获后 `c.Next()` 正常返回，logging 在 deferred / 函数末尾读到 final status = 500；如果交换顺序（logging 在 recover 之前），logging 的 deferred panic recovery 会吃掉 panic，recover 中间件就看不到。
5. **Story 1.9 追加字段 `ctx_done`**（epics.md Story 1.9 AC）：logging 中间件末尾读 `c.Request.Context().Err()` 判断 cancel。**本 story 不做**，Story 1.9 会 revisit 本文件。
6. **`c.FullPath()` 未命中路由时返回空串**：这是 Gin 行为特性，logging 里字段保留空串；metrics 里换成 `"UNKNOWN"`（基数隔离）。
7. **Gin 的 Abort 与 panic**：`c.Abort()` 不等于 panic，只是标记"跳过后续 handler"。recover 里 `response.Error` 之后 `c.Abort()` 是双保险。

### Structured Log 字段 6 + 3 一览（本 story 阶段）

| 字段 | 类型 | 本 story 是否输出 | 注入方 |
|---|---|---|---|
| `request_id` | string | ✅ | request_id 中间件 → ctx → logging child logger |
| `user_id` | string | ❌ 留空 | Epic 4 Story 4.5（auth 中间件） |
| `api_path` | string | ✅ | logging 中间件（`c.FullPath()`） |
| `latency_ms` | int64 | ✅ | logging 中间件（`time.Since(start).Milliseconds()`） |
| `business_result` | string | ❌ 留空 | 各 service 层关键路径（Epic 4+） |
| `error_code` | int | ❌ 留空 | Story 1.8（AppError + ErrorMappingMiddleware）|
| `method` | string | ✅ | logging 中间件（epics.md 1.3 AC 额外要求）|
| `status` | int | ✅ | logging 中间件（`c.Writer.Status()`） |
| `client_ip` | string | ✅ | logging 中间件（`c.ClientIP()`） |

实际 log JSON 示例（本 story 完成后）：
```json
{"time":"2026-04-24T12:34:56.789Z","level":"INFO","msg":"http_request","request_id":"550e8400-e29b-41d4-a716-446655440000","api_path":"/ping","method":"GET","status":200,"latency_ms":2,"client_ip":"127.0.0.1"}
```

### Prometheus 两个 metric 样式预览

`/metrics` 响应摘录（本 story 完成后 + 1 次 `/ping` 请求后）：
```
# HELP cat_api_requests_total Total HTTP requests by api_path, method, code.
# TYPE cat_api_requests_total counter
cat_api_requests_total{api_path="/ping",code="200",method="GET"} 1
# HELP cat_api_request_duration_seconds HTTP request duration by api_path, method.
# TYPE cat_api_request_duration_seconds histogram
cat_api_request_duration_seconds_bucket{api_path="/ping",method="GET",le="0.005"} 1
cat_api_request_duration_seconds_bucket{api_path="/ping",method="GET",le="0.01"} 1
...
cat_api_request_duration_seconds_sum{api_path="/ping",method="GET"} 0.0012
cat_api_request_duration_seconds_count{api_path="/ping",method="GET"} 1
```

### testify 取舍再议

Story 1.2 Dev Notes 说"testify 留给 Story 1.5"，本 story 继续这个策略。但 slog / prometheus 的断言代码用 stdlib 写出来稍 verbose（尤其 slog 字段解析）。如果 dev 觉得 verbose 到影响可读性，允许提前激活 testify：
- `go get github.com/stretchr/testify@v1.11.1`
- 在 Completion Notes 记录"本 story 提前激活了 testify，Story 1.5 只需安装 sqlmock / miniredis / dockertest"
- 团队内对账即可，不是错误选择

默认推荐保持 stdlib 风格，与 Story 1.2 对齐。

### UUID 库选型

- 选 `github.com/google/uuid v1.6.0`
- 使用 `uuid.NewString()` 生成 UUID v4（符合 epics.md Story 1.3 AC 明示的 "UUID v4"）
- ADR-0001 §4 提到 "UUIDv7 / ULID" 作为"缺省生成策略"，但 epics.md 的 "UUID v4" 是更早的决策，两文档对齐时 **epics.md 优先**（story AC 是 dev 实装的合约）
- **不**自己写 UUID 实现（大坑：UUIDv4 的随机源 / version bit 编码容易出错）

### Gin `c.Set` vs `context.Value`

- Gin 提供 `c.Set(key, value)` + `c.Get(key)` 作为 handler 间共享状态的 API
- request_id 中间件**必须**用 `c.Set("request_id", rid)`，这样后续 handler 能通过 `c.Get("request_id")` 拿
- 但 **service / repo 层**不能依赖 `*gin.Context`，只能拿 `ctx context.Context`。怎么桥接？
  - logging 中间件调 `logger.NewContext(c.Request.Context(), childLogger)` 把 child logger 塞进 ctx
  - 下游 service 用 `logger.FromContext(ctx)` 拿 child logger（已含 request_id 字段）
  - 这样 service 层写 log 时不需要知道 "request_id" 这个 key，只需要调 `logger.FromContext(ctx).Info(...)` 就天然带上
- `request_id` 字符串本身如果 service 需要读（如写入 DB audit 字段），可以在 logging 中间件里**同时**存一份到 ctx：
  ```go
  ctx = context.WithValue(ctx, ridKey{}, rid)
  ```
  但 Story 1.3 不需要这条 —— Epic 4 审计时再加

### 切 slog 时的潜在兼容问题

Story 1.2 的 `bootstrap/server.go` 里 `log.Printf("server shutdown error: %v", err)` 直接把 error 格式化进 message。slog 对等写法是 `slog.Error("server shutdown error", slog.Any("error", err))` —— error 变成独立字段，message 保持静态字符串。这是**正确的风格迁移**，不是无意义改动：

- 静态 message 让 log 聚合工具能按 message 分组
- error 独立字段便于按 error 类型过滤 / 告警

### `/metrics` 端点的安全考虑（留给后续）

- 本 story 不加 auth 保护（Epic 4 才有 auth）
- 生产部署前必须决定：① 独立监控端口（不暴露公网）② 在主端口但前置 basic-auth 中间件 ③ 依赖 K8s NetworkPolicy
- 本 story 留 TODO 注释在 router.go `/metrics` 挂载点附近，提醒 Epic 36 上线前处理

### 测试变更清单（Story 1.2 文件受影响处）⚠️ 必改

切 slog 会让 Story 1.2 两个测试**断言漂移**（对 stdlib `log` 的捕获失效），必须在 T8 切换时**同步迁移**：

- `server/internal/app/bootstrap/router_test.go` 的 `TestRouter_Ping`：
  - 原断言 `env.RequestID == ""` 是 false（非空即可）→ 保留，不改
  - **不**新增针对 UUID 格式的断言在此（那是 Story 1.3 的 AC12 case 1 的事；原 case 继续作为"最小 happy path"验证）

- `server/internal/app/bootstrap/server_test.go`（Story 1.2 review fix 时新增）：
  - 两个测试 `TestRun_BindFailureReturnsErrorAndNoStartedBanner` / `TestRun_ShutdownStopsServer` 都用 `log.SetOutput(&buf)` 捕获 `log.Printf` 文本输出，断言 `strings.Contains(buf.String(), "server started")` / `"server stopped"`
  - Story 1.3 AC9 把 `bootstrap/server.go` 里的 `log.Printf` 切 `slog.Info`，**这些 buf 会变空**，两个测试必挂
  - **迁移方案**（T8 必做）：
    1. `log.SetOutput(&buf)` → 用 `slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})` 构造 test logger，`slog.SetDefault(testLogger)` + deferred 恢复
    2. `strings.Contains(buf.String(), "server started")` 不变（slog TextHandler 输出含 `msg=server started`）
  - **不**删除 Story 1.2 的 server_test.go，它测的是 "bind 失败不出 started banner"，仍然是关键回归；只迁移 log 捕获实现
  - T8.2 subtask 明示覆盖了这个迁移

### Previous Story Intelligence（Story 1.2 交付物关键点）

- Story 1.2 把 `server/` 从零起到可运行 + 第一次 review fix（配置路径 CWD 耦合 + banner 时序）全部 commit 在 `8913fa7`
- `bootstrap/router.go` 里留了一条注释占位：
  ```go
  // Future: Story 1.3 wraps middleware; Story 1.4 adds GET /version;
  //         Story 1.6 registers /dev/* group behind BUILD_DEV flag.
  ```
  本 story 落地后这条注释的"Story 1.3 wraps middleware"部分完成；更新为 "Story 1.4 / 1.6 待做"
- `bootstrap/server.go` 里 `Run` 的 `net.Listen` + `srv.Serve` 拆分**不动**
- `cmd/server/main.go` 的 `config.LocateDefault` 探测逻辑**不动**
- `pkg/response/response.go` 的 `requestIDFromCtx` 在本 story 升级（AC10）—— 唯一会改的 Story 1.2 内部函数
- `docs/lessons/2026-04-24-config-path-and-bind-banner.md` 的 Lesson 2 Meta 是本 story 的核心执行原则

### Git intelligence

- 最近 5 个 commit（按时间逆序）：
  - `4516b31 chore(story-1-2): 收官 Story 1.2 + 归档 story 文件 + 新增 /fix-review 命令`
  - `90e40e1 chore(lessons): backfill commit hash for 2026-04-24 lesson`
  - `8913fa7 feat(server): Epic1/1.2 cmd/server 入口 + config 加载 + Gin + /ping`
  - `e7f5e9c docs(decision): 0001 test stack - slog / prometheus / sqlmock+miniredis / testify`
  - `4c5ad2e chore(commands): 新增 /story-done 命令 - 一键收官 story`
- commit message 风格：Conventional Commits，中文 subject，scope 用 `story-X-Y` 或模块名（`server` / `sprint` / `decision` / `lessons` / `commands`）
- 本 story 建议 commit message：`feat(server): Epic1/1.3 中间件三件套 + slog + prometheus metrics`

### 常见陷阱

1. **slog child logger 没传下去**：用 `slog.Default().With(...)` 生成 child 但没塞 ctx，下游 `slog.Info` 拿的还是 default。**修复**：`logger.NewContext(ctx, child)` 后把 ctx **写回** `c.Request = c.Request.WithContext(ctx)`。
2. **recover 后 `c.Abort()` 漏调**：后续 middleware / handler 可能还在跑（虽然 response 已写）。补上 `c.Abort()`。
3. **`slog.Any("panic", rec)` 对 nil 安全**：rec 不会是 nil（能进 recover 分支说明真的 panic 了），但为了防御性，slog `Any` 对 nil 会输出 `null` JSON —— 接受即可。
4. **prometheus Registry 测试隔离**：如果不同测试共享 `var Registry`，一个测试注册的 metric 会污染另一个。**修复**：本 story 的 metrics 测试用局部 `NewRegistry()` 做隔离（不碰全局 `Registry`），或在 `TestMain` 里 `Registry = prometheus.NewRegistry()` 重置。
5. **UUIDv4 正则校验**：`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$` —— 第 15 位必须是 `4`，第 20 位必须在 `[89ab]`。`google/uuid.NewString()` 生成 v4，合规。
6. **/metrics 自己出现在 `cat_api_requests_total`**：正常，接受（见 AC8 最后一段）；不要试图 skip。
7. **`c.ClientIP()` 在单测里返回 `""`**：httptest 造的 req 没有 RemoteAddr。测试里可手动 `req.RemoteAddr = "127.0.0.1:12345"` 或接受空串断言为非 nil。

### Project Structure Notes

- `Go项目结构与模块职责设计.md` §4 建议 `bootstrap/logger.go` 做 slog 初始化。本 story 选 `internal/infra/logger/` 而非 `bootstrap/`，理由：
  - `bootstrap/` 的语义偏"组装 / 启动顺序"，里面放 `router.go` + `server.go` 合适
  - `infra/` 是"外部设施接入层"（infra/config、infra/db、infra/redis 都在 §4 里列在 infra 下），logger 作为对外输出通道同理
  - 不同路径会导致 §4 描述与代码不完全一致，**应在 Completion Notes 记录这个 variance**，便于后续读 §4 的人能找到实际位置
- 或者 dev 选 `bootstrap/logger.go` 也不错；两者语义都说得通。只要在 Completion Notes 说清楚一次。**推荐** `infra/logger/`，与 `infra/config/` / `infra/metrics/` 并列。

### References

- [Source: docs/宠物互动App_总体架构设计.md#6.3-推荐目录] — `internal/infra` / `internal/app/http/middleware` 结构
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#4-项目目录建议] — `middleware/{request_id,recover,logging,auth,rate_limit}.go` 命名
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#8.2-中间件建议] — 中间件清单（本 story 只做前 3 个）
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#13.1] — 日志最少字段要求
- [Source: docs/宠物互动App_V1接口设计.md#2.4-通用响应结构] — `{code, message, data, requestId}` 四字段（recover middleware 写 500 用此封装）
- [Source: docs/宠物互动App_V1接口设计.md#3] — 业务错误码（1009 "服务繁忙" 是本 story recover 用的过渡 code）
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#3.6-Logger-选型] — slog + 自定义 JSON Handler
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#3.7-Metrics-选型] — prometheus client_golang
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#4-Structured-Log-Fields] — 6 字段表
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#5-Metrics-Reserved-Slots] — Story 1.3 注册 2 metric
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#6-Version-Lock] — prometheus/client_golang v1.23.2
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#7-Follow-ups] — Story 1.3 分工明示
- [Source: _bmad-output/implementation-artifacts/1-2-cmd-server-入口-配置加载-gin-ping.md#Completion-Notes] — Story 1.2 `requestIDFromCtx` 升级伏笔
- [Source: _bmad-output/implementation-artifacts/1-2-cmd-server-入口-配置加载-gin-ping.md#Dev-Notes#response-helper-实现要点] — 原函数实现
- [Source: docs/lessons/2026-04-24-config-path-and-bind-banner.md#Meta] — "声明 vs 现实"原则，直接指导 log 字段时序
- [Source: docs/lessons/2026-04-24-config-path-and-bind-banner.md#Lesson-2] — `net.Listen` 拆分示例（本 story 对应要点：log 必须在关键副作用后）
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.3] — 本 story 原始 AC
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.4] — 下游 `/version`（本 story 留 TODO 注释）
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.6] — 下游 `/dev/*`（本 story 留 TODO 注释）
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.8] — 下游 AppError（本 story 用 literal 1009）
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.9] — 下游 ctx propagation（logging 中间件追加 `ctx_done`）
- [Source: _bmad-output/planning-artifacts/epics.md#Story-4.5] — 下游 auth 中间件 + `user_id` 字段接入
- [Source: CLAUDE.md#Build-Test] — build 脚本现状（Story 1.7 重做；本 story 仍用 `go build` 验收）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7 (1M context) — BMM dev-story workflow v6.2

### Debug Log References

- `go test ./...` — 6 个包全绿：
  - `internal/app/bootstrap`：TestRouter_Ping + 3 个新增 integration case + 2 个 Story 1.2 迁移后的 Run case
  - `internal/app/http/middleware`：request_id 2 + recover 3 + logging 2 = 7 case
  - `internal/infra/logger`：parseLevel 表驱动 7 case + Init 2 + NewContext/FromContext 2 = 5 case
  - `internal/infra/metrics`：ObserveHTTP counter + histogram + UNKNOWN path = 2 case
  - `internal/infra/config`：Story 1.2 的 8 case 全部继续绿
- `go vet ./...` — 零告警
- `go build -o ../build/catserver.exe ./cmd/server/` — 产出 24 MB Windows binary
- 3 次 curl 验证（重建 PID=9660 后）：见下方 "实测输出" 小节

### Completion Notes List

**实装路径（对齐 story spec）**：
- 中间件顺序 **req_id → logging → recover → handler**（Story spec 原本写错顺序，self-review 时已纠正；见 Story 1.3 AC5 顶部的顺序说明）
- 局部变量命名避开 `logger` 包 shadowing —— 全部使用 `reqLogger`
- slog 默认 handler 为 JSON handler 写 `os.Stdout`；level 从 `cfg.Log.Level` 解析；非法值回落 info 并 warn 一次
- Prometheus Registry 用独立实例（不用 DefaultRegisterer），避免 Go runtime metrics 污染输出
- Recover middleware 用 literal `code=1009, message="服务繁忙"`（Story 1.8 引入 AppError 后会被 ErrorMappingMiddleware 替代）

**偏离 Story spec 的地方**：
- `logger/slog_test.go` 写了 5 个 case（含 parseLevel 表驱动 7 小 case），超过 spec 的 2 个 —— parseLevel 的大小写容忍 / 空串 / 非法值路径都值得覆盖，不算 scope creep
- `recover_test.go` 写了 3 个 case（多一个"第二次请求仍正常"），明确验证 AC12 case 2 的同一语义在 middleware 单测层
- Story 1.2 的 `router_test.go` 的 `TestRouter_Ping` 的 `requestId != ""` 断言依然成立（现在返回的是 UUIDv4），但原测试**完全未修改** —— 符合 spec 要求的"新增 case 不动老 case"

**Story 1.2 迁移清单（T8.3 细节）**：
- `bootstrap/server_test.go` 的两个测试（TestRun_BindFailureReturnsErrorAndNoStartedBanner / TestRun_ShutdownStopsServer）原本用 `log.SetOutput(&buf)` 捕获 stdlib log 输出
- 切 slog 后新增 `captureSlog(t)` helper：用 `slog.NewTextHandler(&buf, ...)` + `slog.SetDefault` 替换，deferred 恢复
- TextHandler 输出仍含 `msg=server started` 和 `msg=server stopped` 字面量，`strings.Contains` 断言不用改

**AC12 测试覆盖**：
- Case 1 `/ping` requestId 为 UUIDv4 + 与 header 一致 ✅
- Case 2 panic 路由 500 + 第二次 /ping 200 ✅
- Case 3 `/metrics` 含两个 metric 名 ✅

**实测 curl 输出**（端口 18082，从 repo root `./build/catserver.exe` 启动）：

启动 JSON log（摘录）：
```
{"time":"2026-04-24T14:09:29.8301771+08:00","level":"INFO","msg":"config loaded","path":"server\\configs\\local.yaml","http_port":18082,"log_level":"info"}
{"time":"2026-04-24T14:09:29.8373611+08:00","level":"INFO","msg":"server started","addr":":18082"}
```

① `curl http://127.0.0.1:18082/ping`：
```
HTTP/1.1 200 OK
X-Request-Id: cb61d637-e042-4155-87b8-eb1486af5944
Content-Type: application/json; charset=utf-8

{"code":0,"message":"pong","data":{},"requestId":"cb61d637-e042-4155-87b8-eb1486af5944"}
```
对应 server 端 log：
```
{"time":"2026-04-24T14:09:31.8035443+08:00","level":"INFO","msg":"http_request","request_id":"cb61d637-e042-4155-87b8-eb1486af5944","api_path":"/ping","method":"GET","status":200,"latency_ms":0,"client_ip":"127.0.0.1"}
```

② `curl -H "X-Request-Id: manual-test-abc" http://127.0.0.1:18082/ping`：
```
HTTP/1.1 200 OK
X-Request-Id: manual-test-abc

{"code":0,"message":"pong","data":{},"requestId":"manual-test-abc"}
```

③ `curl http://127.0.0.1:18082/metrics`（摘录）：
```
# HELP cat_api_requests_total Total HTTP requests served, partitioned by api_path, method, and response code.
# TYPE cat_api_requests_total counter
cat_api_requests_total{api_path="/ping",code="200",method="GET"} 2

# HELP cat_api_request_duration_seconds HTTP request duration in seconds, partitioned by api_path and method.
# TYPE cat_api_request_duration_seconds histogram
cat_api_request_duration_seconds_bucket{api_path="/ping",method="GET",le="0.005"} 2
cat_api_request_duration_seconds_bucket{api_path="/ping",method="GET",le="+Inf"} 2
cat_api_request_duration_seconds_sum{api_path="/ping",method="GET"} 0
cat_api_request_duration_seconds_count{api_path="/ping",method="GET"} 2
```

**已知遗留 / TODO 提示**：
- `/metrics` 暂无 auth 保护（Epic 36 上线前处理）；router.go 顶部已留 TODO 注释
- `user_id` / `business_result` / `error_code` 三字段本 story 不输出 —— Epic 4（auth）/ 各 service / Story 1.8（AppError）接入时补齐
- Story 1.7 重做 `scripts/build.sh` 时一并加 `--race` 开关（当前 Windows 本地 cgo.exe 与 toolchain 的 race 检测不兼容，遗留问题与 Story 1.2 相同）
- Story 1.9 `ctx_done` 字段：logging 中间件末尾需读 `c.Request.Context().Err()`，Story 1.9 revisit 本文件

### File List

新增：
- `server/internal/infra/logger/slog.go`
- `server/internal/infra/logger/slog_test.go`
- `server/internal/infra/metrics/registry.go`
- `server/internal/infra/metrics/http.go`
- `server/internal/infra/metrics/http_test.go`
- `server/internal/app/http/middleware/request_id.go`
- `server/internal/app/http/middleware/request_id_test.go`
- `server/internal/app/http/middleware/recover.go`
- `server/internal/app/http/middleware/recover_test.go`
- `server/internal/app/http/middleware/logging.go`
- `server/internal/app/http/middleware/logging_test.go`

修改：
- `server/go.mod`（新增 `prometheus/client_golang v1.23.2` + `google/uuid v1.6.0` + 它们的 transitive）
- `server/go.sum`
- `server/cmd/server/main.go`（stdlib `log` → slog；新增 `logger.Init(cfg)` 调用）
- `server/internal/app/bootstrap/server.go`（stdlib `log` → slog）
- `server/internal/app/bootstrap/server_test.go`（`log.SetOutput` → `captureSlog` helper）
- `server/internal/app/bootstrap/router.go`（挂三件套中间件 + 注册 `/metrics`）
- `server/internal/app/bootstrap/router_test.go`（新增 3 integration case）
- `server/internal/pkg/response/response.go`（`requestIDFromCtx` 优先读 `c.Get("request_id")`）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（1-3 ready-for-dev → in-progress → review；last_updated）
- `_bmad-output/implementation-artifacts/1-3-中间件-request_id-recover-logging.md`（Tasks 勾选；Dev Agent Record；Status → review）

产出（不入 git）：
- `build/catserver.exe`（24 MB）

### Change Log

| 日期 | 变更 | Story |
|---|---|---|
| 2026-04-24 | 初次实装：三件套中间件 + slog JSON Handler + 2 个 prom HTTP metric；Story 1.2 的 server_test.go 迁移；`requestIDFromCtx` 升级 | 1.3 |
