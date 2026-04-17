# Story 0.5: 结构化日志与请求关联 ID

Status: done

## Story

As an operator,
I want all logs in zerolog JSON with `requestId / userId / connId / event / duration / buildVersion / configHash` fields and PII auto-masked,
so that I can grep-and-jq through 3 AM WS hub incident logs to locate users and requests (NFR-OBS-1~3, FR46).

## Acceptance Criteria

1. **AC1 — 日志初始化**：`logx.Init(cfg.Log)` 配置 zerolog 为 JSON 输出（production）或 ConsoleWriter（`cfg.Log.Format == "console"`，仅 dev）
2. **AC2 — 上下文字段提取**：`logx.Ctx(ctx) *zerolog.Logger` 自动携带 `requestId / userId / connId` 字段
3. **AC3 — PII 脱敏**：`logx.MaskPII(s string) string` 将 displayName / email 替换为 `[REDACTED]`（M13）
4. **AC4 — APNs Token 脱敏**：`logx.MaskAPNsToken(s string) string` 仅保留前 8 字符 + `...`；INFO+ 级别日志不得包含完整 token（M14）
5. **AC5 — Request ID 中间件**：`middleware.RequestID` 从 `X-Request-ID` header 读取或生成 UUID，注入 context + response header
6. **AC6 — Logger 中间件**：`middleware.Logger` 每个 HTTP 请求记录一条 info 日志：`method / path / status / durationMs / userId（如有）`
7. **AC7 — Recover 中间件**：`middleware.Recover` 捕获 panic → zerolog `error` 级别 + stack + 返回 `AppError{Code: "INTERNAL_ERROR"}` 500
8. **AC8 — 全局字段**：所有日志包含 `buildVersion` + `configHash`（启动时通过 zerolog global context 注入）
9. **AC9 — Lint 规则**：forbidigo 规则阻止 `fmt.Printf / fmt.Println / log.Printf / log.Println`（CI 失败阻止合并）
10. **AC10 — 代码示例**：`docs/code-examples/` 提供 `handler_example.go` 和 `service_example.go` 演示 `logx.Ctx(ctx).Info().Str().Msg()` 字段 API 用法
11. **AC11 — 单元测试**：PII 脱敏正确性 / APNs token 脱敏 / requestId 从 header 继承 / requestId 缺失时生成

## Tasks / Subtasks

- [x] Task 1: LogCfg 扩展 + logx 包实现 (AC: #1, #2, #8)
  - [x] 1.1 config.LogCfg 添加 `Format` 字段（`toml:"format"`，值 `"json"` 或 `"console"`）
  - [x] 1.2 config/default.toml 添加 `format = "json"`
  - [x] 1.3 实现 `pkg/logx/logx.go`：`Init(opts)` 根据 Format 设置 JSON 或 ConsoleWriter；设置全局 level；注入 `buildVersion` + `configHash` 到 zerolog global context
  - [x] 1.4 实现 `logx.Ctx(ctx) *zerolog.Logger`：从 context 提取 logger（含 requestId/userId/connId 字段）
  - [x] 1.5 实现 `logx.WithRequestID(ctx, id) context.Context`：向 context 注入 requestId
  - [x] 1.6 实现 `logx.WithUserID(ctx, id) context.Context`：向 context 注入 userId
  - [x] 1.7 实现 `logx.WithConnID(ctx, id) context.Context`：向 context 注入 connId（WS 场景预留）
- [x] Task 2: PII 脱敏 (AC: #3, #4)
  - [x] 2.1 实现 `pkg/logx/pii.go`：`MaskPII(s) string` + `MaskAPNsToken(s) string`
  - [x] 2.2 单元测试 `pkg/logx/pii_test.go`（table-driven）
- [x] Task 3: Middleware 三件套 (AC: #5, #6, #7)
  - [x] 3.1 实现 `internal/middleware/request_id.go`：读 `X-Request-ID` 或 `uuid.New()`，调 `logx.WithRequestID`，设置 response header
  - [x] 3.2 实现 `internal/middleware/logger.go`：记录 method/path/status/durationMs/userId，使用 `logx.Ctx(ctx)`
  - [x] 3.3 实现 `internal/middleware/recover.go`：panic recovery → zerolog error + stack → `{"code":"INTERNAL_ERROR","message":"internal server error"}` 500
  - [x] 3.4 单元测试 `internal/middleware/*_test.go`（使用 `httptest.NewRecorder` + `gin.CreateTestContext`）
- [x] Task 4: Router 集成 (AC: #5, #6, #7)
  - [x] 4.1 `cmd/cat/wire.go` buildRouter 中注册中间件链：`Logger → Recover → RequestID`
  - [x] 4.2 `cmd/cat/initialize.go` 启动时调用 `logx.Init(logx.Options{...})`
- [x] Task 5: 全局 zerolog 迁移 (AC: #8)
  - [x] 5.1 `initialize.go` / `app.go` / `wire.go` 中的全局 log 调用保持不变（启动期合法）
  - [x] 5.2 `mongox` / `redisx` / `config` 中的 `log.Fatal()` 保持不变（启动期 fatal 合法）
- [x] Task 6: Forbidigo lint 配置 (AC: #9)
  - [x] 6.1 `.golangci.yml` 已启用 forbidigo 并配置禁止 `fmt.Printf` / `fmt.Println` / `log.Printf` / `log.Println`
  - [x] 6.2 文件已存在，无需创建
- [x] Task 7: 代码示例 (AC: #10)
  - [x] 7.1 创建 `docs/code-examples/handler_example.go`
  - [x] 7.2 创建 `docs/code-examples/service_example.go`
- [x] Task 8: 集成验证 (AC: #11)
  - [x] 8.1 `pkg/logx/logx_test.go`：测试 Init JSON/Console 切换、Ctx 字段继承、全局字段存在
  - [x] 8.2 `bash scripts/build.sh --test` 编译 + 所有测试通过

## Dev Notes

### Architecture Constraints (MANDATORY)

- **Constitution §1 (Explicit over implicit)**：无 DI 框架，所有依赖在 `initialize()` 手动构造
- **Constitution §7 (Context throughout)**：所有 I/O 必须传 `context.Context`；禁 `context.TODO()`
- **P5 强制规则**：字段命名全 camelCase（`requestId` 非 `request_id`，`userId` 非 `user_id`）
- **M13 PII 规则**：`userId` 可记录；`displayName`、邮箱必须 `[REDACTED]`
- **M14 APNs Token 规则**：日志中只显示前 8 字符 + `...`；INFO+ 不出现完整 token
- **AppError 尚未实现**（Story 0.6）：`middleware.Recover` 暂时直接返回 JSON `{"code":"INTERNAL_ERROR","message":"internal server error"}`，不依赖 AppError 类型。Story 0.6 实现后替换为真正的 AppError

### Source Tree — 要创建/修改的文件

**新建：**
- `pkg/logx/logx.go` — `Init(cfg, buildVersion, configHash)` + `Ctx(ctx)` + `WithRequestID` + `WithUserID` + `WithConnID`
- `pkg/logx/pii.go` — `MaskPII` + `MaskAPNsToken`
- `pkg/logx/logx_test.go` — Init/Ctx/全局字段测试
- `pkg/logx/pii_test.go` — PII 脱敏测试
- `internal/middleware/request_id.go` — RequestID 中间件
- `internal/middleware/logger.go` — Logger 中间件
- `internal/middleware/recover.go` — Recover 中间件
- `internal/middleware/request_id_test.go`
- `internal/middleware/logger_test.go`
- `internal/middleware/recover_test.go`
- `docs/code-examples/handler_example.go`
- `docs/code-examples/service_example.go`

**修改：**
- `internal/config/config.go` — LogCfg 添加 `Format` 字段
- `config/default.toml` — `[log]` 段添加 `format = "json"`
- `cmd/cat/initialize.go` — 启动时调用 `logx.Init()`
- `cmd/cat/wire.go` — `buildRouter` 注册中间件链
- `internal/middleware/doc.go` — 删除或保留（有真实文件后无用）

**不动：**
- `pkg/mongox/client.go`、`pkg/redisx/client.go`、`internal/config/config.go` 中的 `log.Fatal()` — 启动期合法使用全局 logger
- `cmd/cat/app.go` 中的 `log.Error()` / `log.Info()` — 启动期合法

### 关键实现细节

**logx.Init 签名与行为：**
```go
func Init(opts Options)  // Options{Level, Format, BuildVersion, ConfigHash}
```
- `opts.Format == "console"` → `zerolog.ConsoleWriter{Out: os.Stdout}`
- 否则 → `zerolog.New(os.Stdout)`（JSON，默认）
- 设置 `zerolog.SetGlobalLevel` 根据 `opts.Level`（默认 "info"）
- 注入全局字段：`log.Logger = logger.With().Str("buildVersion", ...).Str("configHash", ...).Logger()`

**logx.Ctx 实现：**
```go
func Ctx(ctx context.Context) *zerolog.Logger {
    l := zerolog.Ctx(ctx)
    if l.GetLevel() == zerolog.Disabled {
        return &log.Logger  // 回退到全局 logger（含 buildVersion/configHash）
    }
    return l
}
```
当 context 中没有 logger 时（如中间件尚未注入），回退到全局 `log.Logger` 而非返回 disabled logger。`WithRequestID`/`WithUserID`/`WithConnID` 内部使用相同的回退逻辑（`ctxLogger` helper），确保在空 context 上也能正确链式注入字段。

**Context 注入链路：**
1. `middleware.RequestID` → `logx.WithRequestID(ctx, id)` → 基于全局 logger（或已有 context logger）创建带 requestId 字段的新 logger，种入 context
2. 后续 handler/service 调用 `logx.Ctx(ctx)` 自动获取含 requestId + 全局字段的 logger
3. userId 由 JWT auth middleware（Story 1.3）通过 `logx.WithUserID(ctx, id)` 注入，access log 通过 `logx.Ctx(ctx)` 自动继承，无需手动读取

**middleware.Recover 暂行方案：**
Story 0.6 AppError 尚未实现，Recover 检查 `c.Writer.Written()` 后决定响应策略：
```go
if !c.Writer.Written() {
    c.AbortWithStatusJSON(500, gin.H{
        "code":    "INTERNAL_ERROR",
        "message": "internal server error",
    })
} else {
    c.Abort()  // 已写部分响应，只中止后续处理
}
```

**中间件注册顺序（wire.go buildRouter）：**
```go
r := gin.New()
r.Use(middleware.Logger())    // 最外层，defer 确保 panic 也产出 access log
r.Use(middleware.Recover())   // 捕获 panic，写 500 JSON
r.Use(middleware.RequestID()) // 注入 requestId 到 context logger
```
Logger 使用 defer 写日志，即使 Recover 处理了 panic，Logger 的 defer 仍会执行。RequestID 最内层注入 requestId，Logger defer 读取时 context 已包含该字段。

### Testing Standards

- 文件命名：`xxx_test.go` 同目录
- 多场景测试必须 table-driven（宪法）
- 单元测试使用 `t.Parallel()`
- testify：`require.NoError` / `assert.Equal`
- Gin 测试：`httptest.NewRecorder()` + `gin.CreateTestContext()`
- 测试 helper：`setupXxx(t)` / `assertXxx(t)`，必须调 `t.Helper()`
- 错误判断只用 `errors.Is` / `errors.As`（M12）

### Previous Story Intelligence (Story 0.4)

- **Gin 测试模式**：已在 health_handler_test.go 建立 `httptest.NewRecorder` + `gin.CreateTestContext` 模式
- **initialize.go 模式**：依赖在 `initialize()` 中构造并注入，`buildRouter()` 构建路由
- **Hub Stub**：`internal/ws/hub_stub.go` 存在，WS 相关字段 connId 本 story 仅预留接口
- **Go module path**：`github.com/huing/cat/server`
- **现有依赖**：zerolog v1.35.0 已在 go.mod 中

### LogCfg 变更说明

现有 `LogCfg` 有 `Level` 和 `Output` 字段。需要添加 `Format` 字段：
```go
type LogCfg struct {
    Level  string `toml:"level"`
    Format string `toml:"format"` // "json" (default) or "console"
    Output string `toml:"output"`
}
```
`Output` 字段当前未使用，保留不动。

### Project Structure Notes

- 所有新文件路径严格遵循架构指南目录结构
- `pkg/logx/` 已有 `doc.go` 占位，添加实际文件后可删除 `doc.go`
- `internal/middleware/` 已有 `doc.go` 占位，添加实际文件后可删除 `doc.go`
- 包命名单数小写短词（M1）

### References

- [Source: _bmad-output/planning-artifacts/epics.md — Story 0.5 详细 AC，Lines 523-543]
- [Source: _bmad-output/planning-artifacts/architecture.md — P5 Logging 强制规则]
- [Source: _bmad-output/planning-artifacts/architecture.md — Baseline Code Files 表]
- [Source: _bmad-output/planning-artifacts/architecture.md — Middleware 目录结构]
- [Source: docs/backend-architecture-guide.md — 日志约定 §455-481]
- [Source: _bmad-output/implementation-artifacts/0-4-multi-dimensional-healthcheck-endpoint.md — Dev Notes & File List]

## Dev Agent Record

### Agent Model Used

Claude Opus 4.6 (1M context)

### Debug Log References

### Completion Notes List

- logx.Init 使用 Options struct（非 config.LogCfg）避免 pkg→internal 循环依赖，与 mongox/redisx ConnectOptions 模式一致
- logx.Ctx 在 context 无 logger 时回退到全局 log.Logger（含 buildVersion/configHash），避免 disabled logger 静默吞日志
- WithRequestID/WithUserID/WithConnID 通过 ctxLogger helper 回退到全局 logger 后链式注入字段
- middleware.Recover 检查 c.Writer.Written() 后决定是否写 500 JSON（Story 0.6 AppError 未实现）
- middleware.Logger 使用 defer 写日志，从 logx.Ctx(ctx) 自动继承 userId（由 JWT auth middleware Story 1.3 通过 logx.WithUserID 注入）
- 中间件顺序 Logger→Recover→RequestID：Logger defer 确保 panic 也产出 access log
- .golangci.yml 已有完整 forbidigo 配置，无需修改
- 删除 pkg/logx/doc.go、internal/middleware/doc.go、docs/code-examples/.gitkeep 占位文件
- 22 个顶层测试全部通过（含 table-driven 子测试共 31 个），覆盖 logx + PII + middleware
- 全量回归：所有已有测试通过（handler, config, jwtx, mongox, redisx）

### Change Log

- 2026-04-17: Story 0.5 实现完成 — 结构化日志 + 请求关联 ID + PII 脱敏 + 中间件三件套

### File List

**新建：**
- pkg/logx/logx.go
- pkg/logx/pii.go
- pkg/logx/logx_test.go
- pkg/logx/pii_test.go
- internal/middleware/request_id.go
- internal/middleware/logger.go
- internal/middleware/recover.go
- internal/middleware/request_id_test.go
- internal/middleware/logger_test.go
- internal/middleware/recover_test.go
- docs/code-examples/handler_example.go
- docs/code-examples/service_example.go

**修改：**
- internal/config/config.go (LogCfg 添加 Format 字段)
- config/default.toml (添加 format = "json")
- cmd/cat/initialize.go (添加 logx.Init 调用)
- cmd/cat/wire.go (注册中间件链 Logger→Recover→RequestID)

**删除：**
- pkg/logx/doc.go (占位文件)
- internal/middleware/doc.go (占位文件)
- docs/code-examples/.gitkeep (占位文件)
