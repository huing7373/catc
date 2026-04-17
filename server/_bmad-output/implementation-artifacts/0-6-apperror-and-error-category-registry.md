# Story 0.6: AppError + 错误分类注册表

Status: done

## Story

As a developer,
I want a compiler-enforced AppError type with 5-tier Category classification and a complete error code registry,
so that the client can branch on Category (retry / prompt / logout / silent) and server audit logs stay complete (P4, NFR-SEC-10).

## Acceptance Criteria

1. **AC1 — AppError 结构体**：`AppError` 字段 `Code string / Message string / HTTPStatus int / Category ErrCategory / Cause error`，Category 必填（构造函数强制参数，编译期约束）
2. **AC2 — ErrCategory 枚举**：5 档 `retryable / client_error / silent_drop / retry_after / fatal`
3. **AC3 — errors.Is/As 兼容**：`AppError.Error() / Unwrap()` 实现符合 `errors.Is/As` 语义（M12）
4. **AC4 — 错误码注册表**：`dto/error_codes.go` 注册 MVP 所有错误码，每个码必须关联一个 Category；启动期 `init()` 检查遗漏时 `log.Fatal`
5. **AC5 — 注册表单元测试**：扫描 registry 断言无漏档（P4 `[test]` 强度）
6. **AC6 — RespondAppError**：`dto.RespondAppError(c *gin.Context, err error)` 响应格式 `{"error": {"code": "...", "message": "..."}}`；映射 HTTPStatus；zerolog 记录 cause + code；`Cause` 不回客户端
7. **AC7 — error-codes.md**：`docs/error-codes.md` 人类可读注册表；CI 单元测试校验与 `dto/error_codes.go` 常量一致
8. **AC8 — 单元测试**：每个 Category → HTTP status 映射正确 / `errors.Is(err, sentinel)` 判断正确 / `Cause` 不泄露给客户端 JSON / `errors.As` 解包正确
9. **AC9 — Recover 中间件升级**：`middleware.Recover` 改为返回真正的 AppError（替换现有 gin.H 临时方案）

## Tasks / Subtasks

- [x] Task 1: ErrCategory 枚举 + AppError 结构体 (AC: #1, #2, #3)
  - [x] 1.1 在 `internal/dto/error.go` 定义 `ErrCategory` 类型（string 常量枚举）：`CategoryRetryable / CategoryClientError / CategorySilentDrop / CategoryRetryAfter / CategoryFatal`
  - [x] 1.2 定义 `AppError` 结构体：`Code string / Message string / HTTPStatus int / Category ErrCategory / Cause error`
  - [x] 1.3 实现构造函数 `NewAppError(code string, message string, httpStatus int, category ErrCategory) *AppError` —— Category 为必填参数，编译期保证
  - [x] 1.4 实现 `WithCause(err error) *AppError` 方法返回新 AppError 副本（附带 Cause，不修改原 sentinel）
  - [x] 1.5 实现 `Error() string` 返回 `code + ": " + message`
  - [x] 1.6 实现 `Unwrap() error` 返回 `Cause`（M12 兼容）
- [x] Task 2: RespondAppError 辅助函数 (AC: #6)
  - [x] 2.1 在 `internal/dto/error.go` 实现 `RespondAppError(c *gin.Context, err error)`
  - [x] 2.2 用 `errors.As(err, &ae)` 解包；匹配到 AppError → `c.JSON(ae.HTTPStatus, gin.H{"error": gin.H{"code": ae.Code, "message": ae.Message}})`；zerolog 记录 `logx.Ctx(ctx).Error().Err(ae.Cause).Str("code", ae.Code).Msg("app error")`
  - [x] 2.3 未匹配 → `c.JSON(500, gin.H{"error": gin.H{"code": "INTERNAL_ERROR", "message": "internal server error"}})`；zerolog 记录 `logx.Ctx(ctx).Error().Err(err).Msg("unhandled error")`
  - [x] 2.4 确保 `Cause` 永远不出现在 JSON 响应中
- [x] Task 3: 错误码注册表 (AC: #4, #5)
  - [x] 3.1 在 `internal/dto/error_codes.go` 定义全局 registry `map[string]ErrCategory`
  - [x] 3.2 定义 MVP 错误码 sentinel 变量（每个都是 `*AppError`）：
    - `ErrAuthInvalidIdentityToken` — fatal, 401
    - `ErrAuthTokenExpired` — fatal, 401
    - `ErrAuthRefreshTokenRevoked` — fatal, 401
    - `ErrFriendAlreadyExists` — client_error, 409
    - `ErrFriendLimitReached` — client_error, 422
    - `ErrFriendInviteExpired` — client_error, 410
    - `ErrFriendInviteUsed` — client_error, 409
    - `ErrFriendBlocked` — client_error, 403
    - `ErrFriendNotFound` — client_error, 404
    - `ErrBlindboxAlreadyRedeemed` — client_error, 409
    - `ErrBlindboxInsufficientSteps` — client_error, 422
    - `ErrBlindboxNotFound` — client_error, 404
    - `ErrSkinNotOwned` — client_error, 403
    - `ErrRateLimitExceeded` — retry_after, 429
    - `ErrDeviceBlacklisted` — fatal, 403
    - `ErrInternalError` — retryable, 500
    - `ErrValidationError` — client_error, 400
    - `ErrUnknownMessageType` — client_error, 400
    - `ErrRoomFull` — client_error, 409
  - [x] 3.3 实现 `init()` 函数：遍历所有 sentinel 检查 Code 是否都在 registry 中且 Category 匹配；遗漏时 `log.Fatal().Str("code", ...).Msg("error code missing from registry")`
  - [x] 3.4 实现 `RegisteredCodes() map[string]ErrCategory` 导出函数供测试和文档校验使用
- [x] Task 4: 单元测试 (AC: #5, #8)
  - [x] 4.1 在 `internal/dto/error_codes_test.go` 编写 table-driven 测试：
    - 每个 sentinel 的 Category 正确
    - 每个 sentinel 的 HTTPStatus 正确
    - `errors.Is(sentinel.WithCause(someErr), sentinel)` 为 true
    - `errors.As(wrappedErr, &ae)` 能解包出正确 Code
  - [x] 4.2 registry 完整性测试：扫描所有 sentinel 确认都在 registry 中
  - [x] 4.3 RespondAppError 测试（用 `httptest.NewRecorder` + `gin.CreateTestContext`）：
    - AppError → 正确 HTTP status + JSON 格式 `{"error":{"code":"...","message":"..."}}`
    - 非 AppError → 500 + `INTERNAL_ERROR`
    - Cause 不出现在响应 body 中
  - [x] 4.4 error-codes.md 一致性测试：读取 `docs/error-codes.md` 解析所有 code，断言与 `RegisteredCodes()` 一致
- [x] Task 5: error-codes.md 文档 (AC: #7)
  - [x] 5.1 创建 `docs/error-codes.md`：表格列出所有错误码 + Category + HTTPStatus + Message + 说明
  - [x] 5.2 确保内容与 `error_codes.go` 完全一致（Task 4.4 的测试会验证）
- [x] Task 6: Recover 中间件升级 (AC: #9)
  - [x] 6.1 修改 `internal/middleware/recover.go`：panic recovery 改为使用 `dto.RespondAppError(c, dto.ErrInternalError.WithCause(fmt.Errorf("panic: %v", r)))`
  - [x] 6.2 更新 `internal/middleware/recover_test.go`：断言 panic 时响应格式为 `{"error":{"code":"INTERNAL_ERROR","message":"..."}}`（包裹在 error 对象中）
- [x] Task 7: 清理 + 集成验证
  - [x] 7.1 删除 `internal/dto/doc.go` 占位文件
  - [x] 7.2 `bash scripts/build.sh --test` 编译 + 所有测试通过（含回归）

## Dev Notes

### Architecture Constraints (MANDATORY)

- **宪法 §1（显式胜于隐式）**：无 DI 框架；所有依赖在 `initialize()` 手动构造
- **宪法 §4（错误码 > 错误字符串）**：对外 `AppError`（code + message + httpStatus），内部 sentinel + `errors.Is/As`
- **宪法 §6（Context 贯穿到底）**：`RespondAppError` 通过 `c.Request.Context()` 获取 logger
- **P4 Error 5 档分类 `[compiler]`**：`retryable / client_error / silent_drop / retry_after / fatal`；`AppError` 必须含 `Category` 字段；启动时校验每个 error code 都归档到一个 category
- **M12 errors.Is/As `[lint]`**：错误判断只能用 `errors.Is/As`，禁字符串比较（errorlint 拦截）
- **Error wrap 原则**：内部用 sentinel + `errors.Is/As`；仅在最外层（handler / WS dispatcher）wrap 为 AppError，避免重复 wrap 丢失 stack

### Category → HTTPStatus 映射表

| Category | HTTP Status 范围 | 客户端策略 | 典型错误码 |
|---|---|---|---|
| `retryable` | 500 / 502 / 503 / 504 | 指数退避自动重试 | `INTERNAL_ERROR` |
| `client_error` | 400 / 404 / 409 / 422 | 不重试，提示用户 | `FRIEND_ALREADY_EXISTS`, `VALIDATION_ERROR` |
| `silent_drop` | 200 / 业务静默 | 客户端无反应 | `TOUCH_RATE_LIMITED` |
| `retry_after` | 429 | 等 Retry-After header | `RATE_LIMIT_EXCEEDED` |
| `fatal` | 401 / 403 | 清 token → 强制登出 | `AUTH_TOKEN_EXPIRED`, `DEVICE_BLACKLISTED` |

### 关键实现细节

**AppError 构造函数设计（编译期强制 Category）：**
```go
func NewAppError(code string, message string, httpStatus int, category ErrCategory) *AppError
```
Category 是必填参数，Go 编译器确保不可能创建无 Category 的 AppError。

**WithCause 返回副本（保护 sentinel 不可变）：**
```go
func (e *AppError) WithCause(err error) *AppError {
    copy := *e
    copy.Cause = err
    return &copy
}
```
这样 `errors.Is(sentinel.WithCause(x), sentinel)` 通过 Unwrap 链仍然能匹配到原 sentinel 的 Code。注意：`errors.Is` 默认比较指针，需要自定义 `Is(target error) bool` 方法比较 Code 字段。

**errors.Is 自定义实现：**
```go
func (e *AppError) Is(target error) bool {
    var t *AppError
    if errors.As(target, &t) {
        return e.Code == t.Code
    }
    return false
}
```
这确保 `errors.Is(sentinel.WithCause(x), sentinel)` 为 true（Code 相同即视为同一错误）。

**RespondAppError 使用 logx.Ctx（非 log.Ctx）：**
架构指南示例用 `log.Ctx`，但项目实际使用 `logx.Ctx`（Story 0.5 建立的 context logger 模式，含 disabled logger 回退到全局 logger 逻辑）。必须用 `logx.Ctx(c.Request.Context())`。

**响应 JSON 格式（宪法 §7 + 路由约定）：**
```json
{"error": {"code": "FRIEND_ALREADY_EXISTS", "message": "好友已存在"}}
```
注意：error 嵌套在 `{"error": {...}}` 中，不是顶层 `{"code": "..."}` 平铺。Cause 永远不回客户端。

**init() 注册检查的作用域：**
`init()` 在 package 加载时执行。它遍历 `error_codes.go` 中所有定义的 sentinel（需要一个收集机制，如 `allCodes []*AppError` 切片），检查每个 Code 在 registry map 中是否存在且 Category 匹配。遗漏 → `log.Fatal`（启动期合法使用全局 logger）。

### Source Tree — 要创建/修改的文件

**新建：**
- `internal/dto/error.go` — AppError 结构体 + ErrCategory 枚举 + NewAppError 构造函数 + WithCause + Error/Unwrap/Is + RespondAppError
- `internal/dto/error_codes.go` — MVP 错误码 sentinel 定义 + registry map + init() 校验 + RegisteredCodes()
- `internal/dto/error_codes_test.go` — Category/HTTPStatus 映射测试 + errors.Is/As 测试 + registry 完整性 + RespondAppError 测试 + error-codes.md 一致性
- `docs/error-codes.md` — 人类可读错误码注册表

**修改：**
- `internal/middleware/recover.go` — 改用 `dto.RespondAppError` + `dto.ErrInternalError.WithCause(...)`
- `internal/middleware/recover_test.go` — 更新响应格式断言（`{"error":{"code":"INTERNAL_ERROR","message":"..."}}`）

**删除：**
- `internal/dto/doc.go` — 占位文件，有真实文件后删除

### Testing Standards

- 文件命名：`xxx_test.go` 同目录
- 多场景测试必须 table-driven（宪法）
- 单元测试使用 `t.Parallel()`
- testify：`require.NoError` / `assert.Equal`
- Gin 测试：`httptest.NewRecorder()` + `gin.CreateTestContext()`
- 错误判断只用 `errors.Is` / `errors.As`（M12）
- 测试 helper：`setupXxx(t)` / `assertXxx(t)`，必须调 `t.Helper()`

### Previous Story Intelligence (Story 0.5)

- **logx.Ctx 模式**：`logx.Ctx(ctx)` 在 context 无 logger 时回退到全局 `log.Logger`（含 buildVersion/configHash），RespondAppError 必须用 `logx.Ctx` 而非 `zerolog.Ctx` 或 `log.Ctx`
- **中间件测试模式**：已建立 `httptest.NewRecorder` + `gin.CreateTestContext` 测试模式，recover_test.go 可直接复用
- **Recover 暂行方案**：当前 recover.go 使用 `c.AbortWithStatusJSON(500, gin.H{"code": "INTERNAL_ERROR", ...})` 平铺格式；Story 0.6 需改为嵌套 `{"error": {...}}` 格式并使用 RespondAppError
- **Go module path**：`github.com/huing/cat/server`
- **现有依赖**：zerolog v1.35.0, gin, testify 已在 go.mod 中
- **中间件顺序**：Logger → Recover → RequestID（wire.go buildRouter）
- **启动期 log.Fatal 合法**：`init()` 中使用 `log.Fatal()` 符合项目惯例（mongox/redisx 同模式）

### Git Intelligence

最近 5 个 commit 全部属于 Story 0.5（结构化日志 + 请求关联 ID + PII 脱敏 + 中间件链）。关键模式：
- Review round 反馈驱动迭代修正（context logger 回退、中间件顺序、测试计数）
- Story 文档与实际实现保持对齐（文档即 source of truth）

### Project Structure Notes

- 所有新文件路径严格遵循架构指南目录结构
- `internal/dto/` 当前只有 `doc.go` 占位，本 story 是该包的首个真实实现
- 包命名单数小写短词（M1）
- 后续 story 会按需追加错误码到 `error_codes.go`（如 `FRIEND_NOT_FOUND` 在 Story 5.2 提到追加）

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 0.6 — AC 完整定义，Lines 545-562]
- [Source: _bmad-output/planning-artifacts/architecture.md#P4 — Error 5 档分类，Lines 536-559]
- [Source: _bmad-output/planning-artifacts/architecture.md#M12 — errors.Is/As 强制，Lines 674-675]
- [Source: _bmad-output/planning-artifacts/architecture.md#P7 — Validation error 转换，Lines 588-593]
- [Source: docs/backend-architecture-guide.md#§7 — AppError 模式 + RespondAppError 参考实现，Lines 413-452]
- [Source: docs/backend-architecture-guide.md#路由约定 — 错误响应格式 {"error": {...}}，Lines 660-662]
- [Source: _bmad-output/implementation-artifacts/0-5-structured-logging-and-request-correlation-id.md — logx.Ctx 模式 + Recover 暂行方案]

## Dev Agent Record

### Agent Model Used

Claude Opus 4.6 (1M context)

### Debug Log References

### Completion Notes List

- AppError 结构体用 NewAppError 构造函数强制 Category 参数（编译期约束），WithCause 返回副本保护 sentinel 不可变
- 自定义 Is(target error) 方法按 Code 比较，确保 `errors.Is(sentinel.WithCause(x), sentinel)` 为 true
- RespondAppError 使用 logx.Ctx（非 log.Ctx）与 Story 0.5 context logger 模式一致
- init() 在 package 加载时遍历 allCodes 切片校验 registry 完整性，遗漏 → log.Fatal（启动期合法）
- 19 个 MVP 错误码覆盖 5 档 Category：3 fatal / 12 client_error / 1 retry_after / 1 retryable / 0 silent_drop（silent_drop 待 Story 5.3 touch 限流时追加）
- middleware.Recover 升级为 dto.RespondAppError，响应格式从平铺 `{"code":"..."}` 改为嵌套 `{"error":{"code":"..."}}`
- docs/error-codes.md 与 error_codes.go 由 TestErrorCodesMd_ConsistentWithRegistry 自动校验一致性
- 全量回归：所有已有测试通过（handler, config, jwtx, mongox, redisx, logx, middleware）
- dto 包 16 个顶层测试全部通过（含 table-driven 子测试）

### Change Log

- 2026-04-17: Story 0.6 实现完成 — AppError + 5 档 ErrCategory + 19 MVP 错误码注册表 + RespondAppError + Recover 中间件升级

### File List

**新建：**
- internal/dto/error.go
- internal/dto/error_codes.go
- internal/dto/error_codes_test.go
- docs/error-codes.md

**修改���**
- internal/middleware/recover.go (改用 dto.RespondAppError + dto.ErrInternalError.WithCause)
- internal/middleware/recover_test.go (响应格式断言从平铺改为嵌套 {"error":{...}})

**删除：**
- internal/dto/doc.go (占位文件)
