# Story 0.6: AppError + 错误分类注册表 — 实现总结

为整个后端建立统一的错误处理体系：一个带 5 档分类的 `AppError` 类型 + 19 个 MVP 错误码 sentinel + 统一的 HTTP 错误响应函数。

## 做了什么

### AppError 类型体系 (`internal/dto/error.go`)

- 定义了 `ErrCategory` 枚举（5 档）：
  - `retryable` — 服务端临时故障，客户端指数退避重试（5xx）
  - `client_error` — 请求有误，不重试，提示用户（4xx）
  - `silent_drop` — 静默丢弃，客户端无感知（预留给 touch 限流）
  - `retry_after` — 限流，等 `Retry-After` header 后重试（429）
  - `fatal` — 鉴权失败，客户端清 token 强制登出（401/403）
- `AppError` 结构体：`Code / Message / HTTPStatus / Category / Cause / RetryAfter`
- `NewAppError()` 构造函数强制要求 Category 参数（编译期约束），且校验必须是合法的 5 档之一，无效值直接 panic
- `WithCause(err)` / `WithRetryAfter(seconds)` 返回副本，保护全局 sentinel 不可变
- 自定义 `Is(target)` 方法按 Code 比较，使得 `errors.Is(sentinel.WithCause(x), sentinel)` 为 true

### 错误码注册表 (`internal/dto/error_codes.go`)

- 19 个 MVP 错误码通过 `register()` 函数同时创建 sentinel 并注册到全局 registry，结构上不可能"定义了 sentinel 但忘注册"
- `register()` 在包加载期执行，空 category 或重复 code 直接 `log.Fatal`
- `RegisteredCodes()` 导出函数返回 `map[string]AppError`（值拷贝，不暴露可变指针）

### 统一错误响应 (`RespondAppError`)

- `errors.As` 解包 AppError → 返回 `{"error": {"code": "...", "message": "..."}}` + 对应 HTTPStatus
- `Cause` 只写入 zerolog 日志，永不回客户端（防内部信息泄露）
- `retry_after` 分类默认设置 `Retry-After: 60` header（即使调用方未显式 `WithRetryAfter`）
- typed-nil `*AppError` 安全守卫，防止二次 panic
- 非 AppError 的 error 统一返回 500 + `INTERNAL_ERROR`

### Recover 中间件升级 (`internal/middleware/recover.go`)

- 从 Story 0.5 的临时 `gin.H{"code": "INTERNAL_ERROR", ...}` 平铺格式，升级为 `dto.RespondAppError(c, dto.ErrInternalError.WithCause(...))`
- 响应格式统一为嵌套的 `{"error": {...}}`

### 人类可读文档 (`docs/error-codes.md`)

- 完整的错误码表格（Code / Category / HTTP Status / Message）
- CI 测试自动校验文档与代码逐行一致（Category、Status、Message 全部匹配）

## 怎么实现的

**核心设计决策：`register()` 替代 `allCodes` 切片**

最初用 `NewAppError` 创建 sentinel + `allCodes` 切片 + `init()` 遍历建 registry。代码审查指出这是"自证正确"——漏加 `allCodes` 不会被 `init()` 发现。改为 `register()` 构造函数，定义即注册，从结构上消除遗漏可能性。

**`Is()` 方法的必要性**

Go 的 `errors.Is` 默认比较指针。但 `WithCause` 返回的是副本（新指针），所以需要自定义 `Is()` 按 `Code` 字符串比较。这确保 service 层 `return ErrFriendNotFound.WithCause(mongoErr)` 后，handler 层 `errors.Is(err, ErrFriendNotFound)` 仍然为 true。

**为什么用 `logx.Ctx` 而不是 `log.Ctx`**

架构指南示例写的 `log.Ctx`，但 Story 0.5 建立了 `logx.Ctx` 模式（context 无 logger 时回退全局 logger，避免 disabled logger 静默吞日志）。`RespondAppError` 遵循同一模式。

## 怎么验证的

```bash
bash scripts/build.sh --test   # 编译 + 全部测试通过
go test -v ./internal/dto/     # dto 包详细输出
```

dto 包测试覆盖：
- `TestNewAppError_PanicsOnInvalidCategory` — 空/无效 category panic
- `TestAppError_ErrorsIs` — 4 种场景 table-driven（same sentinel / with cause / different code / fmt.Errorf wrapped）
- `TestAppError_ErrorsAs` — 多层 wrap 后解包正确
- `TestCategoryHTTPStatus_AllCodes` — 遍历全部 19 个注册码校验 Category → HTTPStatus 范围约束
- `TestRegistry_AllSentinelsRegistered` — 解析 error_codes.go 源码验证 sentinel 数量与 registry 一致
- `TestRespondAppError_*` — AppError / 非 AppError / Cause 不泄露 / Retry-After header / typed-nil 安全 / retry_after 默认 60s
- `TestErrorCodesMd_ConsistentWithRegistry` — 解析 docs/error-codes.md 逐行校验 Code/Category/HTTPStatus/Message

全量回归：handler, config, jwtx, mongox, redisx, logx, middleware 全部通过。

## 后续 story 怎么用

- **Story 0.7（Clock）及之后所有 story**：service 层返回 sentinel（如 `ErrFriendNotFound.WithCause(err)`），handler 层调用 `dto.RespondAppError(c, err)` 统一响应
- **Story 0.11（WS rate limit）**：`ErrRateLimitExceeded.WithRetryAfter(seconds)` 设��具体重试延迟
- **Story 1.3（JWT auth middleware）**：返回 `ErrAuthTokenExpired` / `ErrAuthInvalidIdentityToken`（fatal 分类，客户端强制登出）
- **Story 5.3（Touch rate limit）**：追加 `TOUCH_RATE_LIMITED` 到 error_codes.go（silent_drop 分类的首个码）
- **新增错误码**：在 `error_codes.go` 添加 `register(...)` 一行即可，同步更新 `docs/error-codes.md`
- **P7 Validation error**：Gin binding 校验失败后转 `dto.ErrValidationError.WithCause(bindErr)`
