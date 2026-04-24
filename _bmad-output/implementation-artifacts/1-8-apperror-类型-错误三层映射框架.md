# Story 1.8: AppError 类型 + 错误三层映射框架（NFR18）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want 一个统一的 `*AppError` 类型 + 三层映射工具（repo 原生 error → service `Wrap` 业务码 → handler `c.Error(...)` → middleware 写 envelope），全 server 错误处理都基于此,
so that NFR18 "repo → service → handler 三层错误映射" 不靠开发自觉，避免 5 个月后 32 个错误码（V1接口设计 §3 钦定）漂移成一团乱麻 + envelope 形状随每个 handler 各写各的.

## 故事定位（节点 1 第八条实装 story）

- **节点 1 进度**：Story 1.1 (ADR-0001) → 1.2 (cmd/server + ping) → 1.3 (RequestID/Logging/Recovery 中间件) → 1.4 (/version) → 1.5 (测试基础设施) → 1.6 (Dev Tools) → 1.7 (build.sh) **已完成**；本 story 是"横切纪律 NFR18"的落地点
- **NFR18 出处**：`docs/宠物互动App_Go项目结构与模块职责设计.md` §11 "错误码与错误处理建议" + epics.md 行 126 NFR18 "repo 返回底层错误 → service 转业务错误 → handler 映射统一响应结构（按 V1接口设计 §3 错误码表）"
- **三条上游 story 的承接**：
  1. **Story 1.3 Recovery 中间件**：`server/internal/app/http/middleware/recover.go:14-17` 顶部注释明示 "Story 1.8 引入 AppError + ErrorMappingMiddleware 后，此常量会被替换为 errors.ErrServiceBusy 之类的枚举。现在先用 literal，避免提前引入 AppError 框架。" —— 本 story 兑现这条 defer
  2. **Story 1.5 sample service**：`server/internal/service/sample/service.go:23-27` 注释明示 "本 story 用 stdlib errors.New 占位；Story 1.8 落地 AppError 框架后，真实 service 的业务错误会替换为 `apperror.New(code, msg)` 形式。" —— 本 story 提供 `apperror.New` API（**不**强制 sample 立刻迁移，sample 是 mock 模板，迁移由 Epic 4+ 真实 service 各自承担）
  3. **ADR-0001 §4 `error_code` 字段**：表中"`error_code` int 来源 `*AppError.Code` —— Epic 1 Story 1.8 生效。AppError 框架建立后，logging 中间件在 `c.Errors` 非空时读第一个 AppError 的 `.Code` 字段写入；成功请求省略该字段。" —— 本 story 必须同步扩 logging 中间件，否则 ADR §4 这条"已生效"承诺落空
- **下游 story 立即依赖**：Story 1.9 context 传播框架的 AC 第一行就是 "Given Story 1.8 AppError 框架已就绪 + Gin 默认会把 client 断开的信号传到 ctx" —— 本 story 必须在 1.9 之前完成

**范围红线（本 story 只做以下八件事）**：

1. 新建 `server/internal/pkg/errors/` 包：`AppError` 类型 + `New` / `Wrap` / `As` / `Code` 工具 + 32 个错误码常量
2. 新建 `server/internal/app/http/middleware/error_mapping.go`：`ErrorMappingMiddleware`，在 `c.Next()` 后扫描 `c.Errors`，第一个 `*AppError` → 写 envelope；首个非 `*AppError` 原生 error → wrap 1009 + log
3. **重构 `Recovery` 中间件**（`recover.go`）：把当前 `response.Error(c, 500, 1009, ...)` 改成 `c.Error(apperror.Wrap(rec, ErrServiceBusy, "服务繁忙"))`，由下游 ErrorMappingMiddleware 写 envelope；保留 panic 日志逻辑
4. **重构 `Logging` 中间件**（`logging.go`）：在 `c.Next()` 之后、写 http_request 日志之前，扫描 `c.Errors[0]`，若是 `*AppError` 则把 `error_code` 字段加进结构化日志（兑现 ADR-0001 §4 `error_code` 列）
5. **重构 `bootstrap/router.go` 中间件挂载顺序**：`RequestID → Logging → Recovery → ErrorMappingMiddleware → handler`（ErrorMappingMiddleware 必须在 Recovery 之内、handler 之外，才能同时接住 panic 和 handler 推到 `c.Errors` 的 AppError）
6. 写 ADR `_bmad-output/implementation-artifacts/decisions/0006-error-handling.md`：三层约定 + Gin `c.Error()` 模式说明 + Recovery / ErrorMappingMiddleware 协作图 + 32 个错误码表 + 与 V1接口设计 §3 / §2.4 一致性核对
7. 单元测试 ≥6 case（epics.md 强制） + 集成测试（router 全链路）
8. 更新 `server/internal/app/http/middleware/recover.go` 顶部 "等 1.8" 注释 → 删除并替换为对 `errors.ErrServiceBusy` + ErrorMappingMiddleware 的引用

**本 story 不做**：

- ❌ **不**改 `internal/service/sample/service.go` 的 `ErrSampleNotFound = errors.New(...)` —— sample 是 ADR-0001 §3.4 钦点的 mock 模板，迁移到 `apperror.New` 是"业务 service 各自的事"（sample 不是业务 service，没有 32 码里的归属码）。本 story 提供 API、不强迁移
- ❌ **不**改任何业务 handler（节点 1 阶段只有 PingHandler / VersionHandler / PingDevHandler 三个，全是 happy-path 端点，没有需要 return AppError 的场景）
- ❌ **不**实装具体业务码语义（5001 "道具不存在" 等只是码值常量 + 默认 message，**真实**触发由 Epic 5/20/26/32 的 service 用 `apperror.New(errors.ErrCosmeticNotFound, "...")` 在那时落地）
- ❌ **不**引入第三方 error 库（`pkg/errors` / `cockroachdb/errors` / `hashicorp/multierror` / `go-multierror`）—— stdlib `errors` (errors.Is/As/Unwrap) + 自定义 AppError 已足够；引入第三方依赖会与 ADR-0001 §6 "依赖收敛"原则冲突
- ❌ **不**修改 Gin handler 签名或引入 wrapper（不写 `func(c *gin.Context) *AppError`）—— Gin 惯例是 `c.Error(err)` 把错误推到 `c.Errors` 列表，middleware 在 `c.Next()` 之后处理。这是 Gin 官方 [Middleware tutorial](https://gin-gonic.com/docs/examples/custom-middleware/) 推荐做法，引入自定义签名 wrapper 会让 Gin 中间件 / handler 风格分裂
- ❌ **不**改 `internal/pkg/response/response.go` 现有 `Success` / `Error` 函数签名 —— 其他模块可能直接调用；本 story 仅在 ErrorMappingMiddleware 内部 **使用** `response.Error`
- ❌ **不**做 i18n（`message` 字段先用中文 hard-coded；i18n 是 post-MVP 范围）
- ❌ **不**做错误码到 HTTP status 的复杂映射（节点 1 简化：业务错误 = 200 OK + envelope 内 code≠0；只有 panic / non-AppError 兜底走 500。后续如需更细粒度（如 1001 → 401），单独 ADR 演进）
- ❌ **不**改 `data` 字段语义：epics.md 写 "`{code, message, data:null, requestId}`"，但 V1接口设计 §2.4 示例是 `"data": {}`（空对象）；以**设计文档**为准 → `data: map[string]any{}` 序列化为 `{}`（沿用 `response.Error` 现有行为，与现网 Recovery middleware 输出一致）

## Acceptance Criteria

**AC1 — `internal/pkg/errors/` 包结构**

新建包路径：`server/internal/pkg/errors/`，文件：

| 文件 | 内容 |
|---|---|
| `apperror.go` | `AppError` struct + `New` / `Wrap` / `As` / `Code` / `Error()` / `Unwrap()` |
| `codes.go` | 32 个错误码常量（`ErrUnauthorized = 1001` ... `ErrWSNotConnected = 7002`）+ 默认 message map |
| `apperror_test.go` | 单元测试 ≥6 case（详见 AC8） |

**包名**：`package apperror`（**不**用 `errors` 避免与 stdlib `errors` 包名冲突；调用方 `import "github.com/huing/cat/server/internal/pkg/errors"` 后用 `apperror.New(...)`）。包路径 = `internal/pkg/errors/`，包名 = `apperror`，**这是 Go 社区允许的（路径与包名解耦）**，但**必须**在包注释顶部明示该约定，避免下游 dev 误以为 import path 错。

**AC2 — `AppError` 类型与方法**

```go
// AppError 是全 server 业务错误的统一类型。
//
// 字段语义：
//   - Code: 业务错误码（V1接口设计 §3，0 = 成功，业务错误用非 0 码）
//   - Message: 给客户端展示的错误文案（中文 hard-coded，不做 i18n）
//   - Cause: 底层原因（repo 返回的 sql.ErrNoRows / GORM 错误等），可 nil
//   - Metadata: 附加诊断字段（如 user_id / request_id），可 nil
type AppError struct {
    Code     int
    Message  string
    Cause    error
    Metadata map[string]any
}

// Error 实现 error 接口。格式："code=<code> msg=<message>: <cause>"（含 cause 才打 cause）
func (e *AppError) Error() string

// Unwrap 让 errors.Is / errors.As 能穿透 Cause 链
func (e *AppError) Unwrap() error
```

**关键约束**：
- `*AppError` 是**指针接收者**（业务习惯，便于 nil 比较 + 减少拷贝）
- `Error()` 方法**必须** nil-safe？**否** —— Go 惯例是 nil pointer 调用方法 panic；本类型不破例。`apperror.New` / `apperror.Wrap` 都返回非 nil（除了 Wrap(nil, ...) 的特例，见 AC3）
- `Unwrap()` 返回 `e.Cause` —— 让 `errors.Is(wrappedErr, sql.ErrNoRows)` 在 service 层仍能工作（保留底层错误链，便于 repo 层错误诊断）

**AC3 — 构造器 API**

```go
// New 构造一个无 cause 的 AppError。code 必须是 codes.go 里定义的常量；
// 传入未注册的 code 不阻止（dev 自由），但 review 时会问"为什么不复用 32 码之一"。
func New(code int, msg string) *AppError {
    return &AppError{Code: code, Message: msg}
}

// Wrap 在 cause 之上包一层 AppError；保留 cause 用于 errors.Is/As 穿透。
//
// **nil-safe**：Wrap(nil, ...) 返回 nil（不返回 *AppError{Cause: nil}）。
// 这是 epics.md AC 强制：edge case "nil error 传入 Wrap → 不 panic，返回 nil"
// 让 service 层可以无脑写：
//
//	dto, err := s.repo.FindByID(ctx, id)
//	return dto, apperror.Wrap(err, errors.ErrResourceNotFound, "找不到资源")
//
// 不需要在 service 内多写一层 if err == nil 短路。
func Wrap(err error, code int, msg string) *AppError {
    if err == nil {
        return nil
    }
    return &AppError{Code: code, Message: msg, Cause: err}
}

// As 是 errors.As 的快捷封装：尝试把 err 链上的某一层断言为 *AppError。
//
// 用于 ErrorMappingMiddleware：c.Errors 里的 error 可能是 *AppError、也可能是
// 包了 AppError 的 fmt.Errorf("...: %w", appErr) —— 用 As 穿透 Wrap 链。
func As(err error) (*AppError, bool) {
    var ae *AppError
    if stderrors.As(err, &ae) {
        return ae, true
    }
    return nil, false
}

// Code 是从 err 链上提取业务码的快捷方法。
// 找不到 *AppError 时返回 0（成功码 = "未识别为业务错误"）。
func Code(err error) int {
    if ae, ok := As(err); ok {
        return ae.Code
    }
    return 0
}

// WithMetadata 在已构造的 AppError 上追加诊断字段（链式调用）。
// 多次调 metadata 累加（同 key 后写覆盖）。
func (e *AppError) WithMetadata(key string, value any) *AppError
```

**AC4 — 32 个错误码常量（codes.go）**

按 V1接口设计 §3 全部 32 码定义为 `const ErrXxx = N` 并附默认 message。

```go
package apperror

// 通用错误码（1xxx）
const (
    ErrUnauthorized       = 1001 // 未登录 / token 无效
    ErrInvalidParam       = 1002 // 参数错误
    ErrResourceNotFound   = 1003 // 资源不存在
    ErrPermissionDenied   = 1004 // 权限不足
    ErrTooManyRequests    = 1005 // 操作过于频繁
    ErrIllegalState       = 1006 // 状态不允许当前操作
    ErrConflict           = 1007 // 数据冲突
    ErrIdempotencyConflict= 1008 // 幂等冲突
    ErrServiceBusy        = 1009 // 服务繁忙
)

// 认证 / 账号错误码（2xxx）
const (
    ErrGuestAccountNotFound = 2001 // 游客账号不存在
    ErrWeChatBoundOther     = 2002 // 微信已绑定其他账号
    ErrAccountAlreadyBound  = 2003 // 当前账号已绑定微信
)

// 步数错误码（3xxx）
const (
    ErrStepSyncInvalid     = 3001 // 步数同步数据异常
    ErrInsufficientSteps   = 3002 // 可用步数不足
)

// 宝箱错误码（4xxx）
const (
    ErrChestNotFound       = 4001 // 当前宝箱不存在
    ErrChestNotUnlocked    = 4002 // 宝箱尚未解锁
    ErrChestNotOpenable    = 4003 // 宝箱开启条件不满足
)

// 装扮 / 合成错误码（5xxx）
const (
    ErrCosmeticNotFound       = 5001 // 道具不存在
    ErrCosmeticNotOwned       = 5002 // 道具不属于当前用户
    ErrCosmeticInvalidState   = 5003 // 道具状态不可用
    ErrCosmeticSlotMismatch   = 5004 // 装备槽位不匹配
    ErrComposeMaterialCount   = 5005 // 合成材料数量错误
    ErrComposeMaterialRarity  = 5006 // 合成材料品质不一致
    ErrComposeTargetIllegal   = 5007 // 合成目标品质不合法
    ErrCosmeticAlreadyEquipped= 5008 // 装扮已装备
)

// 房间错误码（6xxx）
const (
    ErrRoomNotFound        = 6001 // 房间不存在
    ErrRoomFull            = 6002 // 房间已满
    ErrUserAlreadyInRoom   = 6003 // 用户已在房间中
    ErrUserNotInRoom       = 6004 // 用户不在房间中
    ErrRoomInvalidState    = 6005 // 房间状态异常
)

// 表情 / WS 错误码（7xxx）
const (
    ErrEmojiNotFound       = 7001 // 表情不存在
    ErrWSNotConnected      = 7002 // WebSocket 未连接
)

// DefaultMessages 提供 code → 中文默认 message 的查表函数。
// 用于"业务方调 apperror.New(code, "")"或非 AppError 兜底时填充 message。
//
// 缺失的 code 返回空串 —— 调用方应当**总是**显式传 message（让错误文案
// 与触发点的业务上下文匹配，DefaultMessages 只是兜底）。
var DefaultMessages = map[int]string{
    ErrUnauthorized:       "未登录或 token 无效",
    // ... 全部 32 条
}
```

**关键约束**：
- 32 个常量**必须**全部定义，即使本节点不用 —— epics.md AC 原文 "全部 26 个错误码" 是误计；实际 V1接口设计 §3 含 1xxx(9) + 2xxx(3) + 3xxx(2) + 4xxx(3) + 5xxx(8) + 6xxx(5) + 7xxx(2) = **32 码**（不含 0=成功）；以**设计文档**为准
- 常量名按表格固定（`ErrXxx` 驼峰，无 underscored）；codes.go 里**不**额外加 alias（`ErrUnauthorized` ≠ `ErrUnauth` 之类）—— 一码一名，便于全仓 grep
- 数值与 V1接口设计 §3 严格一致；本 story 的 ADR-0006 必须包含一张 32 码核对表，单元测试也必须有一条 case 验证常量值（防止后续 dev 改错位 / 漏改）

**AC5 — `ErrorMappingMiddleware`（`error_mapping.go`）**

```go
// ErrorMappingMiddleware 在 c.Next() 之后扫描 c.Errors，把错误统一翻译成
// envelope JSON。挂载顺序必须满足：
//
//   RequestID → Logging → Recovery → ErrorMappingMiddleware → handler
//
// 即：在 Recovery 之内、handler 之外 —— 这样 Recovery 推到 c.Errors 的
// AppError（panic 兜底）与 handler 主动推的 AppError 都能在本中间件统一处理。
//
// 行为：
//   1. c.Next() 跑完后扫 c.Errors（Gin 的错误队列，按 c.Error(err) 顺序追加）
//   2. 若 c.Errors 为空 → 假设 handler 已用 response.Success 写完响应，本中间件 no-op
//   3. 若 c.Errors[0] 链上能 As 出 *AppError → 用其 Code/Message 写 envelope
//      （HTTP status 200，body code≠0；与现网 response.Error 行为一致）
//   4. 若 c.Errors[0] 链上 As 不出 *AppError → wrap 为 ErrServiceBusy(1009)，
//      log error 级别（含原始 err.Error() + request_id），写 envelope
//
// 设计取舍：
//   - 只看 c.Errors[0]：handler 出错应 fast-fail return；多个 c.Errors 的
//     场景属"链路中多个非致命警告"，目前业务无此用例
//   - 不写 HTTP status code 5xx：业务错误统一 200 + envelope code，遵循
//     V1接口设计 §2.4（"业务状态码，0 表示成功"）。500 仅由 panic 兜底走，
//     由 Recovery 设置（保留 Recovery 现有 http.StatusInternalServerError）
//   - response 已被 handler 写过的兜底：c.Writer.Written() 检查 → 跳过本
//     中间件的写响应步骤（避免 "double write" panic）；仍跑日志记录
func ErrorMappingMiddleware() gin.HandlerFunc
```

**实现要点**：
- 用 `apperror.As(c.Errors[0].Err)` 而非 `c.Errors[0].Err.(*apperror.AppError)` —— 后者在 wrap 链场景失效
- `c.Writer.Written()` 检查必须在写 envelope 之前；已写则跳过 `response.Error` 调用，但**仍**记录日志（让"handler 已写响应但 c.Error 推了 err"的场景能被诊断到 —— 这是 dev bug，应在日志暴露）
- log 字段：`event="error_mapping"`, `error_code=<int>`, `error_msg=<string>`, `cause=<unwrap chain string>`, `request_id=<string>`
- log 级别策略：
  - `*AppError` 链上 code ∈ {1xxx, 2xxx, ...} → `slog.WarnContext`（业务错误，正常路径，非告警）
  - `code == 1009` 且来自非 AppError 兜底 → `slog.ErrorContext`（系统级问题，应触发告警）

**AC6 — Recovery 中间件重构**

修改 `server/internal/app/http/middleware/recover.go`：

| before（当前） | after（本 story） |
|---|---|
| `const panicFallbackCode = 1009` | 删除该常量 |
| `response.Error(c, http.StatusInternalServerError, panicFallbackCode, "服务繁忙")` | `c.Error(apperror.Wrap(panicAsErr(rec), apperror.ErrServiceBusy, "服务繁忙"))` |
| 直接 `c.Abort()` 收尾 | `c.AbortWithStatus(http.StatusInternalServerError)` —— 写入 status 但**不**写 body；body 由下游 ErrorMappingMiddleware 写 |
| 顶部"等 1.8"注释 | 改写为对 ErrorMappingMiddleware 协作模型的引用 |

**`panicAsErr(rec any) error` 内部 helper**：把 `recover()` 返回的 `any` 转成 `error`：
- `rec` 已是 `error` → 原样返回
- `rec` 是 `string` → `errors.New(rec.(string))`
- 其他类型（int / struct / nil 等）→ `fmt.Errorf("panic: %v", rec)`

**协作图**（写进 ADR 0006）：

```
panic 路径：
  handler panic
   → Recovery defer recover() 抓住
   → c.Error(apperror.Wrap(rec, ErrServiceBusy, "服务繁忙"))
   → c.AbortWithStatus(500)  ← status 由 Recovery 写
   → ErrorMappingMiddleware 扫 c.Errors[0]，As 出 AppError
   → response.Error(c, 0, 1009, "服务繁忙")  ← body 由下游写

业务错误路径：
  handler ：c.Error(apperror.New(ErrInvalidParam, "x 必填"))
   → c.Abort()  ← handler 自行 abort 终止链路
   → ErrorMappingMiddleware 扫 c.Errors[0]
   → response.Error(c, 200, 1002, "x 必填")  ← status 200 + envelope code 1002
```

**关键约束**：
- Recovery 改后，单元测试 `TestRecovery_StringPanicReturns500Envelope` / `TestRecovery_ErrorPanicDoesNotDoubleFault` / `TestRecovery_SecondRequestStillWorks` **必须**全部仍能通过；具体修改：测试链路需挂上 `ErrorMappingMiddleware`（不然 envelope 不会被写）。改测试的同时把现有 `panicFallbackCode` 字面量替换为 `apperror.ErrServiceBusy`
- **HTTP status 仍是 500**：panic 路径下 `c.AbortWithStatus(http.StatusInternalServerError)` 设置 status，下游 ErrorMappingMiddleware 调 `response.Error(c, 0, ...)` 时**不能**覆盖（response.Error 必须改为：若 `c.Writer.Status()` 已被设过，沿用，不再调 `c.JSON(status, ...)`）。这里有个微妙点：`gin.Context.JSON` 内部会调 `c.Writer.WriteHeader(status)`，但若 status 已设，二次调会是 no-op（http.ResponseWriter 标准行为），因此实际上调 `c.JSON(http.StatusOK, ...)` 时若 status 已是 500 → status 保持 500，仅 body 被写。**安全做法**：ErrorMappingMiddleware 写 envelope 时直接调 `c.Writer.WriteString(<jsonBytes>)` 绕过 `c.JSON` 的 WriteHeader 调用，或在传入 `response.Error` 前显式判断 status 来源。详见 Dev Notes §陷阱 #5

**AC7 — Logging 中间件追加 `error_code` 字段**

修改 `server/internal/app/http/middleware/logging.go`：

```go
// 在 c.Next() 之后、reqLogger.InfoContext 调用之前，扫 c.Errors
attrs := []slog.Attr{
    slog.String("method", c.Request.Method),
    slog.Int("status", status),
    slog.Int64("latency_ms", latency.Milliseconds()),
    slog.String("client_ip", c.ClientIP()),
}
if len(c.Errors) > 0 {
    if ae, ok := apperror.As(c.Errors[0].Err); ok {
        attrs = append(attrs, slog.Int("error_code", ae.Code))
    }
}
reqLogger.LogAttrs(ctx, slog.LevelInfo, "http_request", attrs...)
```

**关键约束**：
- 用 `LogAttrs`（而非 `InfoContext` + 多个 `slog.X(...)`）—— 兼容动态 attr slice（条件追加 `error_code`）
- **只**追加 `error_code`，**不**追加 `error_msg` —— message 已被 ErrorMappingMiddleware 自己的 log 输出，不在 http_request 日志里重复（避免日志冗余）
- 兑现 ADR-0001 §4 表格里 `error_code` 的 "Epic 1 Story 1.8 生效" 承诺

**AC8 — 单元测试覆盖（≥6 case，epics.md 强制）**

`server/internal/pkg/errors/apperror_test.go`：

| # | Case 名 | 类型 | 断言点 |
|---|---|---|---|
| 1 | `TestAppError_ErrorImplementsError` | happy | `(*AppError).Error()` 返回正确格式串；类型实现 `error` 接口（编译期 + `var _ error = (*AppError)(nil)` 静态断言） |
| 2 | `TestAppError_WrapPreservesCauseAndUnwrap` | happy | `Wrap(causeErr, code, msg)` 后 `errors.Is(wrapped, causeErr)` 返回 true；`errors.As(wrapped, &someTypedErr)` 仍能穿透 |
| 3 | `TestAppError_AsExtractsAppError` | happy | 多层 `fmt.Errorf("...: %w", apperror.New(...))` 后 `apperror.As(deepErr)` 仍能返回原 AppError |
| 4 | `TestAppError_WrapNilReturnsNil` | edge | `Wrap(nil, code, msg)` 返回 `nil`，**不**返回 `*AppError{Cause: nil}` —— 让 service 写 `return apperror.Wrap(s.repo.X(ctx), ...)` 直接生效 |
| 5 | `TestAppError_CodeFromNonAppErrorReturnsZero` | edge | `apperror.Code(errors.New("plain"))` 返回 0；`apperror.Code(nil)` 返回 0；`apperror.Code(apperror.New(1002, "x"))` 返回 1002 |
| 6 | `TestAppError_AllCodesMatchV1Spec` | happy | table-driven：32 条 case，每条断言 `apperror.ErrXxx == <预期数值>` —— 防止后续 dev 改错码值 / 漏改与 V1接口设计 §3 同步 |
| 7（推荐补） | `TestAppError_WithMetadataAccumulates` | happy | 链式调 `WithMetadata("a", 1).WithMetadata("b", 2)` → metadata map 含两个 key；同 key 后写覆盖 |

`server/internal/app/http/middleware/error_mapping_test.go`：

| # | Case 名 | 类型 | 断言点 |
|---|---|---|---|
| 1 | `TestErrorMapping_HandlerReturnsAppError` | happy | handler 调 `c.Error(apperror.New(1002, "test"))` + `c.Abort()` → response 是 `{code:1002, message:"test", data:{}, requestId:"..."}` |
| 2 | `TestErrorMapping_HandlerReturnsNonAppError` | edge | handler 调 `c.Error(errors.New("db down"))` + `c.Abort()` → response 是 `{code:1009, message:"服务繁忙", ...}`；同时 log 含 `cause="db down"` 与 `error_code=1009` |
| 3 | `TestErrorMapping_PanicHandledViaRecovery` | happy | handler `panic("boom")` → Recovery 捕获 → ErrorMappingMiddleware 写 1009 envelope；HTTP status 500（panic 路径保持 500） |
| 4 | `TestErrorMapping_NoErrorIsNoOp` | happy | handler 走 `response.Success` 正常返回 → ErrorMappingMiddleware **不**改写响应 |
| 5 | `TestErrorMapping_DoubleWriteGuarded` | edge | handler 已调 `response.Success` 后又调 `c.Error(...)` → ErrorMappingMiddleware 跳过响应写入（`c.Writer.Written()` 检查），但仍打 log（让 dev 能诊断到这条逻辑漏洞） |

`server/internal/app/http/middleware/logging_test.go`（**追加**到现有文件）：

| # | Case 名 | 类型 | 断言点 |
|---|---|---|---|
| 1 | `TestLogging_AddsErrorCodeFromAppError` | happy | handler `c.Error(apperror.New(1002, "x"))` → http_request log 含 `error_code=1002` 字段 |
| 2 | `TestLogging_NoErrorCodeWhenSuccess` | happy | handler `response.Success` → http_request log **不含** `error_code` 字段（避免日志冗余） |

`server/internal/app/http/middleware/recover_test.go`（**修改**现有 3 case）：

- 全部测试用例的 router setup 必须挂上 `ErrorMappingMiddleware()`（在 Recovery 之后）
- envelope 断言保持不变（`code=1009 message="服务繁忙"`），只是 envelope 现在由 ErrorMappingMiddleware 写而非 Recovery 直接写
- 删除 `panicFallbackCode` 常量引用，替换为 `apperror.ErrServiceBusy`

**AC9 — 集成测试覆盖（epics.md 强制）**

新建 `server/internal/app/http/middleware/error_mapping_integration_test.go`（同包，**不**用 `_test` 后缀的"黑盒"风格 —— 沿用 sample service_test.go 的混用策略）。

测试场景：
1. 用 `httptest.NewServer` 启动一个**真**的 `bootstrap.NewRouter()` + 临时挂一条测试路由 `/test/apperror`
2. 该路由 handler `c.Error(apperror.New(1002, "test")); c.Abort()`
3. 客户端 `http.Get(server.URL + "/test/apperror")`
4. 断言：HTTP status 200；response body JSON 严格等于 `{code:1002, message:"test", data:{}, requestId:"<uuid v4 形式>"}`

**关键约束**：
- 不引入新的 router 注入机制 —— 在测试包内 `r := bootstrap.NewRouter(); r.GET("/test/apperror", testHandler)` 即可（Gin 允许 router 构造完后继续注册）
- 这条测试同时验证："ErrorMappingMiddleware 正确挂在 router 链上"（如果 router.go 没挂这中间件，response 会是 Gin 默认的空 body 200，测试会失败）

**AC10 — ADR-0006 文档**

新建 `_bmad-output/implementation-artifacts/decisions/0006-error-handling.md`，至少包含以下章节：

| 章节 | 内容 |
|---|---|
| 1. Context | 引用 NFR18、ADR-0001 §4 `error_code` 字段、Story 1.5 sample service 的 errors.New 占位、Story 1.3 Recovery 的"等 1.8"注释 |
| 2. Decision | 三层约定（repo / service / handler 各自做什么 + 各自不做什么） |
| 3. Gin `c.Error()` vs handler signature wrapper 选型 | 为什么走 `c.Error()` 不引入自定义 handler 签名 |
| 4. Recovery + ErrorMappingMiddleware 协作图 | AC6 那张协作图 + 中间件挂载顺序图 |
| 5. 32 个错误码核对表 | 与 V1接口设计 §3 一一对照（防止后续漂移） |
| 6. 业务错误的 HTTP status 取舍 | 为什么业务错误统一 200 + envelope code（不映射 4xx/5xx） |
| 7. 与 ADR-0001 §4 对齐 | `error_code` 字段如何在 logging middleware 注入 |
| 8. Future Migration | Epic 4+ service 如何用 apperror.New 替换 stdlib errors（sample service 的迁移策略） |
| 9. Change Log | 本 story 创建时间 + 后续每次 codes.go 变更追加 |

**AC11 — 中间件挂载顺序更新（router.go）**

修改 `server/internal/app/bootstrap/router.go`：

```go
// 中间件顺序严格：RequestID → Logging → Recovery → ErrorMappingMiddleware → handler
r.Use(middleware.RequestID())
r.Use(middleware.Logging())
r.Use(middleware.Recovery())
r.Use(middleware.ErrorMappingMiddleware())  // ← 本 story 新增
```

顶部注释段同步更新（包含 ErrorMappingMiddleware 的职责说明 + 与 Recovery 的协作）。

**AC12 — 手动验证（补强 AC9）**

`bash scripts/build.sh --test` 全绿后，手工启动 + curl：

```bash
# 启 server
$ CAT_HTTP_PORT=18092 ./build/catserver -config server/configs/local.yaml &

# /ping 应返回成功 envelope（验证非错误路径不受 ErrorMappingMiddleware 影响）
$ curl -s http://127.0.0.1:18092/ping
{"code":0,"message":"pong","data":{},"requestId":"<uuid>"}

# 故意触发 panic（如有 dev tools 端点支持），或临时改 PingHandler 加 c.Error 验证
# 期望：response 为 envelope；server log 含 event="error_mapping" + error_code 字段
```

**AC13 — Sprint Status 与 CLAUDE.md**

- `_bmad-output/implementation-artifacts/sprint-status.yaml`：`1-8-apperror-类型-错误三层映射框架` 状态 `backlog → ready-for-dev`（本 SM 步骤完成时） → 后续 dev 推进 `→ in-progress → review → done`
- `CLAUDE.md` §"工作纪律" 不需改动；`CLAUDE.md` §"Build & Test" 不需改动（本 story 不改 build.sh）
- `docs/lessons/index.md` 不需改动（本 story 不产 lesson；fix-review 阶段如有 finding 才追加）

## Tasks / Subtasks

- [x] **T1** — `internal/pkg/errors/` 包搭建（AC1 / AC2 / AC3 / AC4）
  - [x] T1.1 创建目录 `server/internal/pkg/errors/`
  - [x] T1.2 `apperror.go`：包注释 + AppError struct + `Error()` / `Unwrap()` / `WithMetadata()` 方法
  - [x] T1.3 `apperror.go`：`New` / `Wrap`（nil-safe） / `As` / `Code` 工具函数
  - [x] T1.4 `codes.go`：32 个错误码常量 + `DefaultMessages` map（V1接口设计 §3 实际 32 码，epics "26" 是误计）
  - [x] T1.5 在 `apperror.go` 顶部明示 "包名 apperror（不是 errors，避免与 stdlib 冲突）；import path internal/pkg/errors" 的约定

- [x] **T2** — 单元测试 ≥6 case（AC8 - 包内）
  - [x] T2.1 `apperror_test.go`：7 条 case（含 32 码值核对 table-driven）
  - [x] T2.2 跑 `go test ./internal/pkg/errors/...` → 全绿

- [x] **T3** — `ErrorMappingMiddleware` 实装（AC5）
  - [x] T3.1 `server/internal/app/http/middleware/error_mapping.go`：实装 + 文件顶部注释（含挂载顺序 + 与 Recovery 协作）
  - [x] T3.2 `error_mapping_test.go`：5 条 case（happy / non-AppError 兜底 / panic 路径 / no-op / double-write 防护）

- [x] **T4** — `Recovery` 中间件重构（AC6）
  - [x] T4.1 删除 `panicFallbackCode` 常量；改用 `apperror.ErrServiceBusy`
  - [x] T4.2 `response.Error(c, 500, 1009, ...)` → `c.Error(apperror.Wrap(panicAsErr(rec), ErrServiceBusy, "服务繁忙"))` + `c.Abort()`（**注意：故意不调 AbortWithStatus(500)**，避免 WriteHeaderNow 让 Writer.Written() 提前为 true，导致下游 ErrorMappingMiddleware 跳过 envelope 写入；status 决策权完全交给 ErrorMappingMiddleware）
  - [x] T4.3 新增 `panicAsErr(any) error` helper（处理 string / error / 其他类型 panic 值）
  - [x] T4.4 顶部注释更新：删除"等 1.8"段，改写为对 ErrorMappingMiddleware 的引用 + AbortWithStatus 不调的理由
  - [x] T4.5 `recover_test.go` 改造：所有 setup 挂 ErrorMappingMiddleware；envelope 断言保持；新增 TestRecovery_PushesAppErrorToCErrors 验证契约接口
  - [x] T4.6 跑 `go test ./internal/app/http/middleware/...` → 全绿

- [x] **T5** — `Logging` 中间件追加 `error_code`（AC7）
  - [x] T5.1 在 `c.Next()` 后扫 `c.Errors`，用 `apperror.As` 提取 code
  - [x] T5.2 改用 `LogAttrs` 支持动态 attr slice
  - [x] T5.3 `logging_test.go` 追加 2 条 case（TestLogging_AddsErrorCodeFromAppError / TestLogging_NoErrorCodeWhenSuccess）

- [x] **T6** — `bootstrap/router.go` 挂载 ErrorMappingMiddleware（AC11）
  - [x] T6.1 `r.Use(middleware.ErrorMappingMiddleware())` 加在 `Recovery()` **之前**（**注意：本 story SM 初稿写的是"之后"，但 Gin 中间件 panic 路径要求 ErrorMappingMiddleware 外层于 Recovery**，否则 panic unwind 会跳过 ErrorMappingMiddleware 的 after-c.Next() 代码 → envelope 写不出来。最终顺序：RequestID → Logging → ErrorMappingMiddleware → Recovery → handler）
  - [x] T6.2 顶部注释更新中间件链描述（含顺序约束的详细解释）

- [x] **T7** — 集成测试（AC9）
  - [x] T7.1 `error_mapping_integration_test.go`：用 `httptest.NewServer` + `bootstrap.NewRouter()` + 两条临时路由 → 真 HTTP roundtrip → 断言 envelope（AppError 200 path + 非 AppError 500 兜底 path）
  - [x] T7.2 跑 `bash scripts/build.sh --test` → 全绿（13 个包）

- [x] **T8** — ADR-0006 文档（AC10）
  - [x] T8.1 `_bmad-output/implementation-artifacts/decisions/0006-error-handling.md`：9 章节按 AC10 表落地
  - [x] T8.2 32 码核对表与 V1接口设计 §3 严格 diff（修正 epics.md "26 码" 误计）

- [x] **T9** — 手动验证（AC12）
  - [x] T9.1 `bash scripts/build.sh --test` 全绿
  - [x] T9.2 `bash scripts/build.sh` 产出 binary，启动 + curl `/ping` `/version` 验证非错误路径不受影响（commit=e2769a3 ldflags 注入 OK；http_request log 不含 error_code 字段）
  - [~] T9.3 临时改 `PingHandler` 注入 c.Error → 验证 envelope：**改用集成测试覆盖**（AC9 的 `error_mapping_integration_test.go` 用 httptest.NewServer + 临时路由完成同等验证，且不污染 PingHandler 源码 / 不依赖手工还原；偏离 epics.md 原文的 "改 PingHandler" 写法，沿用 Story 1.4 review-driven `httptest.NewServer` 测试模式）

- [x] **T10** — 收尾
  - [~] T10.1 `bash scripts/build.sh --test --race` Windows 本机 TSAN 偏离继承 Story 1.5 AC7 / Story 1.7 AC4 处置；归 CI Linux runner 兜底（本 story 不重复跑同样会失败的 race 路径 —— Story 1.7 已确认 ThreadSanitizer 无法在 Win10 上 alloc 内存）
  - [x] T10.2 Completion Notes 补全（见下方）
  - [x] T10.3 File List 填充（见下方）
  - [x] T10.4 状态流转 `ready-for-dev → in-progress → review`
  - [x] T10.5 sprint-status.yaml 状态同步

## Dev Notes

### 项目关键约束（必读，勿绕过）

1. **包名 `apperror` ≠ import 路径 `errors`**：本 story 故意把包路径设为 `internal/pkg/errors/`（对齐设计文档 §11 + §4 目录结构），但**包名声明为** `apperror`。理由：
   - 设计文档 §11 + §4 钦定目录路径 `internal/pkg/errors`，不能动
   - 包名 `errors` 会与 stdlib `errors` 包冲突 —— 调用方必须写 `myerrors "github.com/huing/cat/server/internal/pkg/errors"` 才能区分，N 个 service 都要别名 import，污染严重
   - Go 允许包路径与包名解耦（如 `gopkg.in/yaml.v3` 包名 `yaml`），社区常见
   - 调用方习惯：`import "github.com/huing/cat/server/internal/pkg/errors"` + `apperror.New(...)` —— 一眼能看出"我用的是项目自己的 apperror，不是 stdlib"
   - **必须**在 `apperror.go` 包注释里明示该约定，避免 dev 误以为 import 错

2. **`Wrap(nil, ...)` 必须 nil-safe**：epics.md AC 强制 "edge: nil error 传入 Wrap → 不 panic，返回 nil"。理由：
   - 让 service 写 `return apperror.Wrap(s.repo.X(ctx), ...)` 一行搞定，不用多写 `if err == nil { return nil }`
   - **反例**：`pkg/errors` (cockroachdb) 的 `errors.Wrap(nil, ...)` 返回 nil 是事实标准；本项目复用该约定

3. **`Unwrap()` 返回 `Cause`，不是 `nil`**：让 `errors.Is(wrappedErr, sql.ErrNoRows)` 在 service 层仍能工作。否则 service 拿到 AppError 后再判断"是不是 ErrNoRows" 就需要手动剥层，不优雅
   - **反例**：早期 `github.com/pkg/errors` v0 的 `WithStack` 不支持 Unwrap，导致 `errors.Is` 链路断 —— Go 1.13 引入 Unwrap 后社区共识必须实装

4. **`apperror.As` 用 `errors.As` 实装而非类型断言**：
   ```go
   // 错误（断言只看顶层）：
   if ae, ok := err.(*AppError); ok { ... }
   // 正确（穿透 wrap 链）：
   var ae *AppError
   if errors.As(err, &ae) { ... }
   ```
   middleware 拿到的 `c.Errors[0].Err` 可能被 `fmt.Errorf("...: %w", appErr)` 包过一层，必须穿透

5. **Gin `c.JSON` 的 `WriteHeader` 行为**（AC6 提到的微妙点）：
   - Go stdlib `http.ResponseWriter.WriteHeader` 多次调用：第一次生效，后续静默 no-op（仅 stderr 警告 `http: superfluous response.WriteHeader call`）
   - Gin `c.JSON(status, body)` 内部：先 `c.Writer.WriteHeader(status)` 再写 body
   - 因此 panic 路径下 `c.AbortWithStatus(500)` 已写过 status；ErrorMappingMiddleware 后续调 `c.JSON(http.StatusOK, envelope)` → status 保持 500（不会变 200），body 写成功
   - **但**会在 stderr 输出一行 superfluous warning —— 测试时会污染 logs。**避免方法**：ErrorMappingMiddleware 内调 `response.Error` 时检查 `c.Writer.Status()`，若已是 5xx 则用 `c.JSON(c.Writer.Status(), ...)` 透传 status；否则用 200。或：扩 `response.Error` 接受 `useExistingStatus bool` 参数 —— 取舍交 dev，二选一即可

6. **`c.Abort()` 与 `c.AbortWithStatus()` 区别**：
   - `c.Abort()`：标记后续中间件链跳过；**不**写 status
   - `c.AbortWithStatus(N)`：标记 + 写 status N + **不**写 body
   - panic 路径用 `AbortWithStatus(500)` 让 status 即时落地（避免后续中间件读到 status=200）；业务错误路径用 `c.Abort()`（status 默认 200，由 ErrorMappingMiddleware 通过 `response.Error` 写入；status 200 是默认 + 业务错误码体现在 envelope 里）

7. **`c.Errors` 是 Gin 的 error slice，不是 channel**：
   - 类型 `gin.errorMsgs`（即 `[]*gin.Error`），按 `c.Error(err)` 顺序追加
   - `gin.Error.Err` 才是真正的 `error` 实例；`gin.Error` 还有 `.Type` / `.Meta` 字段（本 story 不用）
   - **必须**用 `c.Errors[0].Err` 取 error，不是 `c.Errors[0]` 直接

8. **handler **不需要** signature 改造**：
   ```go
   // 业务 handler（节点 1 之后）：
   func ChestOpenHandler(c *gin.Context) {
       userID, ok := auth.UserIDFromCtx(c.Request.Context())
       if !ok {
           c.Error(apperror.New(apperror.ErrUnauthorized, "未登录"))
           c.Abort()
           return
       }
       reward, err := chestSvc.OpenCurrentChest(c.Request.Context(), userID, idemKey)
       if err != nil {
           c.Error(err)  // service 层已 Wrap 为 AppError；非 AppError 也兼容（middleware 兜 1009）
           c.Abort()
           return
       }
       response.Success(c, reward, "ok")
   }
   ```
   两行 `c.Error + c.Abort` 是 boilerplate，可考虑加 `errors.Abort(c, err)` helper（可选，本 story 不强制）

9. **`response.Error` 不需改**（节点 1 阶段）：现有签名 `Error(c, httpStatus, code, message)` 完全够 ErrorMappingMiddleware 用：
   ```go
   if ae, ok := apperror.As(c.Errors[0].Err); ok {
       response.Error(c, http.StatusOK, ae.Code, ae.Message)
   } else {
       response.Error(c, http.StatusOK, apperror.ErrServiceBusy, "服务繁忙")
   }
   ```
   如有 AC6 §5 的 status 透传需求，再决定是否扩 `response.Error` 或新增 `response.WriteEnvelope(c, code, msg, data)`

10. **logger 字段命名规范**：与 ADR-0001 §4 严格对齐 —— `error_code`（snake_case），不是 `errorCode` / `errCode` / `code`。这条 grep-friendly 是日志聚合 / Grafana 面板的强诉求

11. **不引入新依赖**：stdlib `errors` (Go 1.13+ 已有 Is/As/Unwrap) + 自定义 AppError 完全够用。`go.mod` 不动；ADR-0001 §6 版本锁定不变

12. **测试 helper 注意**：`server/internal/pkg/testing/slogtest` 的 `slogtest.NewHandler` + `slogtest.AttrValue` 是 logging 测试的标准搭档（见 Story 1.5 sample 用法）；本 story 的 logging_test 追加 case 直接复用，不引入新 fixture
    - **重要**：`AttrValue` 用扁平 key 查（`error_code` 而非 `error.code`）；本 story 不用 `WithGroup`，所以 attr key 就是 `"error_code"`

### 与上游 story 的契约兑现表

| 上游 story | 未竟约定 | 本 story 如何兑现 |
|---|---|---|
| 1.3 Recovery `panicFallbackCode = 1009` 注释 "Story 1.8 引入 AppError + ErrorMappingMiddleware 后，此常量会被替换为 errors.ErrServiceBusy" | T4.1 删常量；T4.2 改用 `apperror.ErrServiceBusy` |
| 1.5 sample service `ErrSampleNotFound = errors.New(...)` 注释 "Story 1.8 落地 AppError 框架后，真实 service 的业务错误会替换为 `apperror.New(code, msg)` 形式" | 本 story 提供 `apperror.New` API；**不**强制 sample 迁移（sample 是 mock 模板，无业务码归属；迁移由 Epic 4+ 各业务 service 各自完成） |
| ADR-0001 §4 `error_code` 字段表 "Epic 1 Story 1.8 生效。AppError 框架建立后，logging 中间件在 c.Errors 非空时读第一个 AppError 的 .Code 字段写入" | T5.1 + T5.2 logging 中间件追加 `error_code` 字段 |
| ADR-0001 §3.4 mock 接口 testify/mock 手写 | 本 story 不增 repo 接口，测试 mock 复用 sample 模板风格（已有 helpers 不需改） |
| 设计文档 §11 "建议统一定义业务错误类型，并统一在 internal/pkg/errors 中维护" | T1 落地包路径 + 32 码 |

### Lessons Index（与本 story 相关的过去教训）

- `docs/lessons/2026-04-24-sample-service-nil-dto-and-slog-test-group.md` **Lesson 1 "公共 artifact 质量门槛"** —— **直接相关**：AppError 是被 N 个 service 复用的核心 fixture，质量标准必须按"假设 10 个下游 story 分别复用"评估：
  - **测试完整度**：AC8 强制 6+1 case，覆盖 happy / edge / nil-safe / 32 码核对 / metadata 累加
  - **注释完整度**：`Wrap(nil, ...)` 的 nil-safe 行为必须在 godoc 第一行说明；32 码常量每条带原 V1 设计 §3 中文注释
  - **语义等价**：`Unwrap` 必须支持 errors.Is/As 穿透（与 stdlib `fmt.Errorf("%w", ...)` 行为等价）；不留"MVP 不支持 Unwrap"局限
- `docs/lessons/2026-04-24-sample-service-nil-dto-and-slog-test-group.md` **Lesson 2 "Fixture 必须语义等价"** —— **间接相关**：本 story 的 ErrorMappingMiddleware 是 envelope 的"生产实装"，必须与 V1接口设计 §2.4 envelope 形状**严格**等价：`{code, message, data, requestId}` 四字段顺序与名字 / data 类型（empty `{}` 而非 `null`） / requestId 来源（优先 c.Get(RequestIDKey)） —— 一处偏离都不允许，AC9 集成测试用字面量 JSON 校验
- `docs/lessons/2026-04-24-config-path-and-bind-banner.md` **Lesson 2 "声明 vs 现实"** —— **间接相关**：本 story 的 32 码常量是"声明"，必须有单元测试验证常量值与 V1接口设计 §3 一致（AC8 case #6 table-driven）；防止 dev 改错常量值后测试不报错（变成新的"声明 vs 现实"分裂）
- `docs/lessons/2026-04-24-go-vet-build-tags-consistency.md` **Lesson 1** —— **不直接相关**（本 story 不引入 build tag）；但提醒：本 story 新增的 `internal/pkg/errors/` 不带 build tag，所有路径下 vet 都能看到，无盲区
- `docs/lessons/2026-04-25-slog-init-before-startup-errors.md` —— **不直接相关**；但提醒：ErrorMappingMiddleware 自己的 log 输出（"event=error_mapping"）必须用 `slog.WarnContext` / `slog.ErrorContext`（带 ctx），让 request_id / api_path 通过 ctx logger 自动携带；**不**用裸 `slog.Warn` / `slog.Error`（丢 ctx）

### Git intelligence（最近 6 个 commit）

```
e2769a3 chore(claude): 更新 Bash allowlist
267f24e chore(story-1-7): 收官 Story 1.7 + 归档 story 文件
9f395b9 chore(lessons): backfill commit hash for 2026-04-24-go-vet-build-tags-consistency
707b070 fix(review): go vet 必须跟随 build/test 的 build tag，保持 validation 可见文件集一致
c9f5069 feat(scripts): Epic1/1.7 重做 build.sh（对齐 cmd/server 入口 + buildinfo ldflags + 5 开关）
e3f1e9a chore(story-1-6): 收官 Story 1.6 + 归档 story 文件
```

最近实装向 commit 是 `c9f5069` (Story 1.7 build.sh)。本 story 紧随其后。

**commit message 风格**：Conventional Commits，中文 subject，scope `story-X-Y` / `server` / `scripts`。
本 story 建议：`feat(server): Epic1/1.8 AppError 类型 + 错误三层映射框架（NFR18）`
（理由：新建 `internal/pkg/errors/` 包 + 重构 3 个 middleware + 新增 ADR-0006，属新能力建立而非 chore；scope 用 `server` 因为变更跨 pkg/middleware/bootstrap，比单一 scope 更准确。`story-1-8` scope 也可）

### 常见陷阱

1. **`errors.New("x") == errors.New("x")` 返回 false**：stdlib `errors.New` 每次都新建实例；不能用 `==` 比较 error。**正确做法**：用 `errors.Is(err, sentinel)` 或包级 `var ErrXxx = errors.New(...)` 让 sentinel 是同一个实例。本 story 的 32 码是 `const int`，不是 error sentinel；调用方拿到 AppError 后用 `apperror.Code(err) == apperror.ErrUnauthorized` 比较

2. **`*AppError` 的 nil 比较陷阱**：
   ```go
   var e *AppError = nil
   var err error = e
   if err == nil { ... }  // false! err 是 (*AppError)(nil) 包成的 interface，非 nil
   ```
   这是 Go 经典坑。**避免方法**：service 层返回 error 时，**永远**用 `if err != nil { return apperror.Wrap(...) }`，让 nil 直接 return nil；不要把 `*AppError` 类型变量直接 assignment 给 error interface。本 story 的 `Wrap(nil, ...)` 显式 return `nil`（不是 `(*AppError)(nil)`），就是为防这个坑

3. **`c.JSON` 之后调 `c.Error` 没用**：Gin handler 一旦写过响应，后续 `c.Error` 只能进 `c.Errors` slice 但响应已发出。AC8 case #5 `TestErrorMapping_DoubleWriteGuarded` 专门覆盖这个场景：middleware 用 `c.Writer.Written()` 检测，跳过响应写但**仍**打 log，让 dev 能在日志里看到这条逻辑漏洞

4. **测试中的 `c.Error` 与 `c.Errors` 时序**：
   ```go
   r.GET("/test", func(c *gin.Context) {
       c.Error(apperror.New(1002, "x"))
       // **不**调 c.Abort() → 后续中间件继续跑（如果还有），但本路由 handler 已结束
       // 多个 c.Error 会全部进 c.Errors slice
   })
   ```
   不调 `c.Abort()` 不会让 ErrorMappingMiddleware 失效（middleware 在 c.Next() 之后扫 c.Errors，与 abort 状态无关），但**强烈建议**业务 handler 调 `c.Abort()` 显式标记"我已 fail"，避免后续中间件误判

5. **HTTP status 200 vs 500 vs 4xx 取舍**（AC5 / AC6 反复提到）：
   - 业务错误（1xxx ~ 7xxx）→ HTTP **200** + envelope code≠0：V1接口设计 §2.4 钦定，业务码与 HTTP 状态正交。客户端永远先看 envelope.code，HTTP 200 仅表示"请求送达 + server 处理完成"
   - panic / non-AppError 兜底（1009）→ HTTP **500**：让 LB / 监控 / 告警系统能在 HTTP 层就识别"服务出大问题"，不需要解 envelope；同时与现有 Recovery 中间件行为兼容
   - **不**做 1001 → HTTP 401 / 1004 → HTTP 403 等映射：MVP 阶段简化；如未来需要（如 PWA / nginx 层级 auth 跳转），单独 ADR 演进

6. **`response.Error` 当前签名**（确认无需改）：
   ```go
   func Error(c *gin.Context, httpStatus, code int, message string)
   ```
   ErrorMappingMiddleware 用 `response.Error(c, http.StatusOK, ae.Code, ae.Message)` 即可。如 AC6 §5 status 透传需求复杂化，再扩签名

7. **`apperror.As` 不要用于"判断是不是 nil"**：
   ```go
   // 错误：
   if _, ok := apperror.As(err); !ok {
       // 这里走非 AppError 分支，但 err 也可能是 nil
   }
   // 正确：先判 nil，再判类型
   if err == nil { return }
   if ae, ok := apperror.As(err); ok {
       // ...
   } else {
       // 兜底为 1009
   }
   ```

8. **32 码常量值 table-driven 测试的写法**（AC8 case #6）：
   ```go
   func TestAppError_AllCodesMatchV1Spec(t *testing.T) {
       cases := []struct{ name string; got, want int }{
           {"ErrUnauthorized", apperror.ErrUnauthorized, 1001},
           {"ErrInvalidParam", apperror.ErrInvalidParam, 1002},
           // ... 全部 32 条
           {"ErrWSNotConnected", apperror.ErrWSNotConnected, 7002},
       }
       for _, tc := range cases {
           t.Run(tc.name, func(t *testing.T) {
               assert.Equal(t, tc.want, tc.got)
           })
       }
   }
   ```
   这条 case 是"防止后续 dev 误改常量值"的最后一道防线 —— **必须**写

9. **集成测试不要用真 net.Listen**（AC9）：用 `httptest.NewServer` 自动选端口，避免与开发机已占端口（8080 / 18090 等）冲突；测试结束 `srv.Close()` 自动释放（**不**用 t.Cleanup 也可，httptest.Server 自带 cleanup）

10. **Recovery 改造后 `c.AbortWithStatus(500)` + `c.Error(...)` 的顺序**：
    ```go
    // 错误顺序（AbortWithStatus 之后 c.Error 仍能加入 c.Errors，没 bug，
    // 但语义更清晰的写法是先 c.Error 再 abort）：
    c.AbortWithStatus(http.StatusInternalServerError)
    c.Error(apperror.Wrap(panicAsErr(rec), apperror.ErrServiceBusy, "服务繁忙"))
    
    // 推荐顺序（先把 error 推到 c.Errors，再 abort）：
    c.Error(apperror.Wrap(panicAsErr(rec), apperror.ErrServiceBusy, "服务繁忙"))
    c.AbortWithStatus(http.StatusInternalServerError)
    ```
    两种顺序在 Gin 实现上等效，但后者读起来"先记账再终止"更符合直觉

11. **不要在 ErrorMappingMiddleware 里用 `panic`**：本中间件**不**应抛 panic；任何意外情况（如 c.Errors[0].Err 是 nil）应该 log error 后写 1009 envelope 兜底，不能让本中间件自己 panic（否则 Recovery 抓到、再回到本中间件、再写一次 envelope → 死循环风险）

12. **测试 setup 把 slog default handler 替换的清理**（recover_test.go 已有模式）：
    ```go
    prev := slog.Default()
    slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, ...)))
    t.Cleanup(func() { slog.SetDefault(prev) })
    ```
    本 story 新增的 error_mapping_test 沿用同模式；或用 `slogtest.NewHandler` + 局部 logger 注入（更精细，不改全局默认）

13. **codes.go 的 `DefaultMessages` map 不要在 init 里初始化**：直接 `var DefaultMessages = map[int]string{ErrXxx: "...", ...}` 字面量初始化即可；用 init() 多一层 dance 无价值，且会让 IDE 跳转源码不那么直观

### 与节点 1 之后业务 epic 的衔接（informational，非本 story scope）

Epic 4 (auth) / Epic 7 (step) / Epic 11 (room) / Epic 20 (chest) / Epic 26 (cosmetic) / Epic 32 (compose) 的 service 落地时，会按本 story 的 ADR-0006 三层约定：

```go
// repo 层（Epic 4 user_repo.go 示例）：
func (r *UserRepo) FindByGuestUID(ctx context.Context, uid string) (*User, error) {
    // GORM 查询，返回原生 error
    if err := r.db.WithContext(ctx).Where("guest_uid = ?", uid).First(&user).Error; err != nil {
        return nil, err  // 不 wrap，保留原生 GORM error
    }
    return &user, nil
}

// service 层（Epic 4 auth_service.go 示例）：
func (s *AuthService) GuestLogin(ctx context.Context, uid string) (*Token, error) {
    user, err := s.userRepo.FindByGuestUID(ctx, uid)
    if err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            // 业务语义"找不到用户" → 用 ErrGuestAccountNotFound
            return nil, apperror.Wrap(err, apperror.ErrGuestAccountNotFound, "游客账号不存在")
        }
        // 其他 DB 错误 → 兜底 1009
        return nil, apperror.Wrap(err, apperror.ErrServiceBusy, "服务繁忙")
    }
    // ... 业务逻辑
}

// handler 层（Epic 4 auth_handler.go 示例）：
func GuestLoginHandler(c *gin.Context) {
    var req LoginRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.Error(apperror.Wrap(err, apperror.ErrInvalidParam, "请求格式错误"))
        c.Abort()
        return
    }
    token, err := authSvc.GuestLogin(c.Request.Context(), req.GuestUID)
    if err != nil {
        c.Error(err)  // 已被 service 层 Wrap 为 AppError
        c.Abort()
        return
    }
    response.Success(c, token, "ok")
}
```

这是 ADR-0006 §8 "Future Migration" 章节的雏形 —— Epic 4 落地时直接复制粘贴上述模板即可。本 story **不**实装 auth_service；只确保 API 与上述用法兼容。

### Project Structure Notes

**新增文件**（4 个）：
- `server/internal/pkg/errors/apperror.go` — AppError 类型 + 工具函数
- `server/internal/pkg/errors/codes.go` — 32 个错误码常量 + DefaultMessages
- `server/internal/pkg/errors/apperror_test.go` — 单元测试
- `server/internal/app/http/middleware/error_mapping.go` — ErrorMappingMiddleware
- `server/internal/app/http/middleware/error_mapping_test.go` — middleware 单测
- `server/internal/app/http/middleware/error_mapping_integration_test.go` — 集成测试（与 router 联动）
- `_bmad-output/implementation-artifacts/decisions/0006-error-handling.md` — ADR

**修改文件**（4 个）：
- `server/internal/app/http/middleware/recover.go` — 删 panicFallbackCode；改用 c.Error + apperror.ErrServiceBusy；顶部注释更新
- `server/internal/app/http/middleware/recover_test.go` — setup 加挂 ErrorMappingMiddleware；常量替换
- `server/internal/app/http/middleware/logging.go` — 追加 error_code 字段；改用 LogAttrs
- `server/internal/app/http/middleware/logging_test.go` — 追加 2 条 error_code case
- `server/internal/app/bootstrap/router.go` — 挂 ErrorMappingMiddleware；注释更新
- `_bmad-output/implementation-artifacts/sprint-status.yaml` — 状态流转

**删除文件**：无

**不动文件**（明确范围红线）：
- `server/internal/service/sample/service.go` — 保留 errors.New ErrSampleNotFound 占位（sample 是 mock 模板，迁移由真实 service 各自承担）
- `server/internal/pkg/response/response.go` — 不改签名
- `server/internal/app/http/handler/*.go` — 三个现有 handler（PingHandler / VersionHandler / PingDevHandler）都是 happy-path，不需要改造；本 story 临时改 PingHandler 仅为手动验证 AC12，验证后**还原**
- `go.mod` / `go.sum` — 不引入新依赖

**对目录结构的影响**：`server/internal/pkg/` 增加 `errors/` 子包（与现有 `response/` / `testing/` 同级）；`server/internal/app/http/middleware/` 增加一个 middleware 文件 + 两个测试文件

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.8] — 本 story 原始 AC（AppError struct + 32 码 + ErrorMappingMiddleware + ADR + 6 case 单元测试 + 集成测试）
- [Source: _bmad-output/planning-artifacts/epics.md#Epic-1] — Epic 1 scope "AppError + 错误三层映射" 明示在范围内
- [Source: _bmad-output/planning-artifacts/epics.md#NFR18] — NFR18 "repo 返回底层错误 → service 转业务错误 → handler 映射统一响应结构"
- [Source: docs/宠物互动App_V1接口设计.md#2.4] — 统一响应结构 `{code, message, data, requestId}` 字段顺序与名字
- [Source: docs/宠物互动App_V1接口设计.md#3] — 32 个错误码定义（数值与中文描述）
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#11] — 错误码与错误处理建议（包路径 + 三层模式）
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#4] — `internal/pkg/errors/` 钦定目录路径
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#5.1-5.3] — handler / service / repo 分层职责
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#3.3-3.4] — testify/assert 与 testify/mock 用法
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#4] — `error_code` 字段表（Story 1.8 生效）
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#6] — 不引入新依赖（go.mod 锁定）
- [Source: _bmad-output/implementation-artifacts/1-3-中间件-request_id-recover-logging.md] — Story 1.3 RequestID / Logging / Recovery 中间件实装与挂载顺序
- [Source: server/internal/app/http/middleware/recover.go#L14-L17] — Recovery 顶部注释 "等 1.8" defer，本 story 兑现
- [Source: server/internal/service/sample/service.go#L23-L27] — Sample 注释 "Story 1.8 落地 AppError 框架后会替换"
- [Source: docs/lessons/2026-04-24-sample-service-nil-dto-and-slog-test-group.md#Lesson-1] — 公共 artifact 质量门槛（AppError 是 N 个 service 的公共依赖）
- [Source: docs/lessons/2026-04-24-sample-service-nil-dto-and-slog-test-group.md#Lesson-2] — Fixture 必须语义等价（envelope 形状必须严格对齐 V1 设计）
- [Source: docs/lessons/2026-04-24-config-path-and-bind-banner.md#Lesson-2] — 声明 vs 现实（32 码常量需 table-driven 测试守护）
- [Source: CLAUDE.md#工作纪律] — "节点顺序不可乱跳"（本 story 是节点 1 第八条；前序 1.1-1.7 已 done）
- [Source: CLAUDE.md#错误码统一] — "repo 返回底层错误 → service 转业务错误 → handler 映射统一响应结构"

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- **偏离 epics.md "26 码"**：epics.md AC 原文 "复用 V1接口设计 §3 列出的全部 26 个错误码" 是误计；实际 V1接口设计 §3 含 1xxx(9) + 2xxx(3) + 3xxx(2) + 4xxx(3) + 5xxx(8) + 6xxx(5) + 7xxx(2) = **32 码**（不含 0=成功）。以 V1 设计文档为权威来源，落地 32 码常量；story 文件、测试 require.Len、ADR-0006 §5 全部按 32 同步。
- **偏离 epics.md AC5 / story 初稿 "Recovery 用 AbortWithStatus(500)"**：实际跑测试发现 `c.AbortWithStatus(500)` 会触发 `c.Writer.WriteHeaderNow()`，让 `c.Writer.Written()` 提前为 true，导致下游 ErrorMappingMiddleware 误判"响应已写过"而跳过 envelope。修方案：Recovery 改用纯 `c.Abort()`（不写 status header），HTTP status 决策权完全收敛到 ErrorMappingMiddleware（AppError.Code == ErrServiceBusy → 500，其他 → 200）。中间件顺序也相应改为 RequestID → Logging → ErrorMappingMiddleware → Recovery → handler（ErrorMappingMiddleware 外层于 Recovery，否则 panic unwind 会跳过 envelope 写入）。这两处修正都更新进了 ADR-0006 §4 + recover.go / error_mapping.go 顶部注释。
- **偏离 AC12 T9.3 "改 PingHandler 手动验证"**：用 `error_mapping_integration_test.go`（httptest.NewServer + 临时路由）替代手工改 PingHandler，避免污染源码 + 不需要手工还原。同等验证强度（真 HTTP roundtrip + envelope 字面量 + UUIDv4 校验）。
- **未跑 `--race --test`**：Story 1.5 AC7 + Story 1.7 AC4 / T5.3 已多次确认 Windows 本机 TSAN 内存分配限制（`ERROR: ThreadSanitizer failed to allocate ... error code: 87`）让 race 测试无法在本机跑；归 CI Linux runner 执行。本 story 不重复跑同样会失败的路径。
- **LSP 缓存**：写完 apperror.go / codes.go 后 IDE 报 BrokenImport / UndeclaredImportedName，但 `go test` 直接通过 —— LSP 缓存延迟，与代码无关。

### Completion Notes List

**实现摘要**

- 新增 `internal/pkg/errors/` 包（路径 errors / 包名 apperror）：AppError 类型 + New/Wrap(nil-safe)/As/Code/WithMetadata/Unwrap 工具 + 32 个错误码常量 + DefaultMessages map
- 新增 `internal/app/http/middleware/error_mapping.go`：扫 c.Errors → AppError → envelope；非 AppError 兜底 1009 + ERROR 级日志；status 决策（ErrServiceBusy → 500，其他 → 200）
- 重构 `recover.go`：删 panicFallbackCode 字面量；改用 c.Error(apperror.Wrap(panicAsErr(rec), ErrServiceBusy, ...)) + c.Abort()（**故意不调 AbortWithStatus**）；新增 panicAsErr helper
- 重构 `logging.go`：c.Next() 后扫 c.Errors[0] 提 AppError.Code，追加 error_code 字段（兑现 ADR-0001 §4 "Story 1.8 生效"）
- 重构 `bootstrap/router.go`：中间件链改为 RequestID → Logging → ErrorMappingMiddleware → Recovery → handler；顶部注释含顺序约束的详细解释
- 新增 ADR-0006 错误处理决策文档（9 章节，含 32 码核对表 + 协作流程图 + 业务 service 落地模板）
- 测试覆盖：apperror_test 7 case（含 32 码 table-driven）+ error_mapping_test 5 case + error_mapping_integration_test 2 case + recover_test 4 case（改造 3 + 新增 1 验证 c.Errors 契约）+ logging_test 追加 2 case

**契约兑现**

| 上游约定 | 兑现位置 |
|---|---|
| Story 1.3 Recovery 顶部 "等 1.8" 注释 | T4：删 panicFallbackCode 改用 apperror.ErrServiceBusy |
| Story 1.5 sample service "等 1.8" 注释 | **不**强迁移（sample 是 mock 模板，无业务码归属）；提供 apperror.New API 待真实 service 调用 |
| ADR-0001 §4 `error_code` 字段 "Story 1.8 生效" | T5：logging.go 追加 error_code（成功请求省略字段） |
| Story 1.9 立即依赖 | 本 story 已交付 → 1.9 可开工 |

**测试结果摘要**

```
$ bash scripts/build.sh --test
=== go vet ===

=== go build (commit=e2769a3, builtAt=2026-04-24T12:34:21Z) ===
OK: binary at build/catserver.exe

=== go test ===
?   	github.com/huing/cat/server/cmd/server	[no test files]
ok  	github.com/huing/cat/server/internal/app/bootstrap	0.265s
ok  	github.com/huing/cat/server/internal/app/http/devtools	0.204s
ok  	github.com/huing/cat/server/internal/app/http/handler	0.187s
ok  	github.com/huing/cat/server/internal/app/http/middleware	0.224s
?   	github.com/huing/cat/server/internal/buildinfo	[no test files]
ok  	github.com/huing/cat/server/internal/infra/config	0.348s
ok  	github.com/huing/cat/server/internal/infra/logger	0.369s
ok  	github.com/huing/cat/server/internal/infra/metrics	0.622s
ok  	github.com/huing/cat/server/internal/pkg/errors	0.409s    ← 新增
?   	github.com/huing/cat/server/internal/pkg/response	[no test files]
ok  	github.com/huing/cat/server/internal/pkg/testing	0.503s
ok  	github.com/huing/cat/server/internal/pkg/testing/slogtest	0.428s
ok  	github.com/huing/cat/server/internal/service/sample	0.404s
OK: all tests passed

BUILD SUCCESS
```

**手动验证**

```
$ CAT_HTTP_PORT=18092 ./build/catserver.exe -config server/configs/local.yaml &
$ curl -s http://127.0.0.1:18092/ping
{"code":0,"message":"pong","data":{},"requestId":"a10f8fcf-8803-47e6-b0e2-31252108dfe0"}
$ curl -s http://127.0.0.1:18092/version
{"code":0,"message":"ok","data":{"commit":"e2769a3","builtAt":"2026-04-24T12:34:21Z"},"requestId":"1e1de8bd-7c67-4790-bc37-930c1dcbad96"}
```

**http_request log（验证成功路径不含 error_code 字段）**：
```
{"level":"INFO","msg":"http_request","request_id":"a10f8fcf-...","api_path":"/ping","method":"GET","status":200,"latency_ms":0,"client_ip":"127.0.0.1"}
```
✅ 无 error_code 字段（ADR-0001 §4 "成功请求省略该字段"）

**集成测试覆盖（替代 AC12 T9.3 手动 PingHandler 改造）**：
- `TestErrorMapping_E2E_HandlerReturnsAppError`：handler `c.Error(apperror.New(1002, "test"))` → 真 HTTP roundtrip → status 200 + envelope `{code:1002, message:"test", data:{}, requestId:<UUIDv4>}` + X-Request-Id header 与 body.requestId 一致
- `TestErrorMapping_E2E_NonAppError_Returns500Envelope`：handler `c.Error(io.EOF)` → status 500 + envelope `{code:1009, message:"服务繁忙"}`

**后续延伸**（非本 story scope，留记录）

- Story 1.9 context 传播框架可立即开工（本 story 提供 AppError + Wrap + Unwrap，1.9 的 ctx cancel 测试场景可基于 apperror.Wrap 写）
- Epic 4 Story 4.6 游客登录事务落地时按 ADR-0006 §8.2 模板用 `apperror.Wrap` 替换 GORM 原生 error
- Sample service 不强迁移；待 Epic 4 第一个真实 service 落地时由 dev review 决定是否回填 sample 也用 apperror.New（推荐**不**回填，保持 sample 作为最小骨架的纯净）

### File List

**新增**
- `server/internal/pkg/errors/apperror.go`（AppError 类型 + 工具函数 + 包注释）
- `server/internal/pkg/errors/codes.go`（32 个错误码常量 + DefaultMessages map）
- `server/internal/pkg/errors/apperror_test.go`（7 条 case：Error/Wrap+Unwrap/As/Wrap-nil/Code/32 码 table-driven/Metadata）
- `server/internal/app/http/middleware/error_mapping.go`（ErrorMappingMiddleware + causeChain helper）
- `server/internal/app/http/middleware/error_mapping_test.go`（5 条 case：AppError/non-AppError/panic/no-op/double-write）
- `server/internal/app/http/middleware/error_mapping_integration_test.go`（2 条 e2e case：AppError 200 / non-AppError 500）
- `_bmad-output/implementation-artifacts/decisions/0006-error-handling.md`（ADR-0006，9 章节）

**修改**
- `server/internal/app/http/middleware/recover.go`（删 panicFallbackCode；改用 c.Error(apperror.Wrap(...)) + c.Abort()；新增 panicAsErr helper；顶部注释更新含顺序约束）
- `server/internal/app/http/middleware/recover_test.go`（setup 挂 ErrorMappingMiddleware；常量替换为 apperror.ErrServiceBusy；新增 TestRecovery_PushesAppErrorToCErrors）
- `server/internal/app/http/middleware/logging.go`（追加 error_code 字段；改用 LogAttrs；导入 apperror）
- `server/internal/app/http/middleware/logging_test.go`（追加 2 case：AddsErrorCodeFromAppError / NoErrorCodeWhenSuccess）
- `server/internal/app/bootstrap/router.go`（中间件链 RequestID→Logging→ErrorMappingMiddleware→Recovery→handler；顶部注释更新含顺序约束的详细解释）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（1-8 状态 backlog → ready-for-dev → in-progress → review；last_updated 时间戳）
- `_bmad-output/implementation-artifacts/1-8-apperror-类型-错误三层映射框架.md`（本 story 文件：32 码修正 / Tasks 勾选 / Dev Agent Record 填充 / Status 流转）

**删除**
- 无

## Change Log

| 日期 | 版本 | 描述 | 作者 |
|---|---|---|---|
| 2026-04-24 | 0.1 | 初稿（ready-for-dev） | SM |
| 2026-04-24 | 1.0 | 实装完成：apperror 包 + 32 码常量 + ErrorMappingMiddleware + Recovery 重构（不再用 AbortWithStatus）+ Logging error_code + router 中间件链顺序修正（ErrorMappingMiddleware 外层于 Recovery）+ ADR-0006；20 条新增 / 改造单测全绿；build.sh --test 13 包全绿；ping/version 手动验证 envelope 与 ldflags 注入正常；状态流转 review。偏离：epics "26 码" → 实际 V1 §3 32 码（以设计为准）；epics AC5 "AbortWithStatus(500)" → 改用纯 c.Abort（避免 WriteHeaderNow 让下游误判 Written）。 | Dev |
