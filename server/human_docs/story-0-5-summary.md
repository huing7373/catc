# Story 0.5: 结构化日志与请求关联 ID — 实现总结

为整个服务端建立了统一的结构化日志基础设施：所有日志自动带上 `requestId`、`buildVersion`、`configHash` 等运维字段，PII 数据自动脱敏，HTTP 请求自动产出 access log。

## 做了什么

### 日志核心库 `pkg/logx/`

- `Init(opts Options)` — 根据配置决定 JSON 或 Console 输出格式，设置日志级别，注入 `buildVersion` + `configHash` 全局字段到 zerolog global logger
- `Ctx(ctx) *zerolog.Logger` — 从 context 取出当前请求的 logger（含 requestId/userId 等字段）。关键设计：当 context 中没有 logger 时回退到全局 `log.Logger`，而不是返回 zerolog 的 disabled logger（会静默丢弃所有日志）
- `WithRequestID` / `WithUserID` / `WithConnID` — 向 context 中的 logger 追加字段，返回新的 context。内部通过 `ctxLogger` helper 统一处理空 context 回退逻辑
- `MaskPII(s) string` — 将 displayName/email 等敏感信息替换为 `[REDACTED]`
- `MaskAPNsToken(s) string` — 只保留 APNs device token 的前 8 字符 + `...`

### HTTP 中间件三件套 `internal/middleware/`

- `Logger()` — 每个请求结束后用 **defer** 写一条 info 级 access log（method/path/status/durationMs），通过 `logx.Ctx(ctx)` 自动继承 requestId 和 userId 等字段
- `Recover()` — 捕获 handler panic，记录 error 级日志（含 stack trace），返回 500 JSON。检查 `c.Writer.Written()` 避免在响应已部分写出时产生混合输出
- `RequestID()` — 从 `X-Request-ID` header 读取或生成 UUID，通过 `logx.WithRequestID` 种入 context，同时回写到 response header

### Lint 保护

- `.golangci.yml` 中的 forbidigo 规则阻止 `fmt.Printf` / `log.Printf` 等非结构化输出，强制使用 zerolog

## 怎么实现的

**pkg 不依赖 internal 的解耦模式：** `logx.Init` 接受自定义 `Options` struct 而非 `config.LogCfg`，避免 `pkg/` → `internal/config` 的循环依赖。这与 `mongox.ConnectOptions`、`redisx.ConnectOptions` 的模式一致（Story 0.3 review 中确立的规范）。

**中间件顺序 Logger → Recover → RequestID：** 这个顺序是 code review 中修正的关键点。Logger 在最外层用 defer 写日志，所以即使 handler panic 被 Recover 捕获，Logger 的 defer 仍然会执行，保证每个请求（包括失败的）都有 access log。RequestID 在最内层注入 requestId 到 context，Logger defer 执行时 context 已经包含该字段。

**context logger 回退机制：** zerolog 的 `zerolog.Ctx(ctx)` 在空 context 上返回 disabled logger（所有写入被丢弃）。`logx.Ctx` 和 `ctxLogger` 检测到 disabled logger 时回退到 `log.Logger`（全局 logger，含 buildVersion/configHash），确保在中间件注入之前的代码路径上日志也不会丢失。

**AppError 暂行方案：** Story 0.6 尚未实现 AppError 类型，Recover 中间件直接构造 `{"code":"INTERNAL_ERROR","message":"internal server error"}` JSON，后续替换为真正的 AppError。

## 怎么验证的

- 22 个顶层测试（含 table-driven 子测试共 31 个），覆盖：
  - `pkg/logx/logx_test.go` — Init JSON/Console 切换、Ctx 字段继承、WithRequestID/WithUserID/WithConnID 链式注入、空 context 回退
  - `pkg/logx/pii_test.go` — MaskPII 和 MaskAPNsToken 各种输入（空串、短串、完整 token）
  - `internal/middleware/*_test.go` — RequestID 生成/继承/context 注入、Logger 字段记录/userId 继承/panic 场景、Recover 捕获/透传/已写响应场景
- `bash scripts/build.sh --test` 全量通过（含 handler、config、jwtx、mongox、redisx 回归）

## 后续 story 怎么用

- **Story 0.6（AppError）** — Recover 中间件中的硬编码 JSON 将替换为 `AppError{Code: "INTERNAL_ERROR"}` 类型
- **Story 1.3（JWT Auth Middleware）** — 鉴权中间件调用 `logx.WithUserID(ctx, userId)` 注入 userId，access log 自动继承
- **Story 0.9+（WS Hub）** — WebSocket 连接通过 `logx.WithConnID(ctx, connId)` 注入 connId，关联 WS 消息日志
- **所有后续 handler/service** — 统一使用 `logx.Ctx(ctx).Info().Str("action","xxx").Msg("yyy")` 模式，参考 `docs/code-examples/` 示例
