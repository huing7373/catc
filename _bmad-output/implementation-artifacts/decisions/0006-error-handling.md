# ADR-0006: 错误处理框架（AppError + 三层映射 + ErrorMappingMiddleware）

- **Status**: Accepted
- **Date**: 2026-04-24
- **Decider**: Developer
- **Supersedes**: N/A
- **Related Stories**: 1.3 (Recovery 中间件落地与 defer 注释), 1.5 (sample service errors.New 占位), 1.7 (build.sh 不影响本 ADR), **1.8 (本决策落地)**, 1.9 (context 传播框架，依赖本 ADR), Epic 4+ (业务 service 用 apperror.New 替换 stdlib errors)

---

## 1. Context

### 1.1 NFR18 与 设计文档 §11

`docs/宠物互动App_Go项目结构与模块职责设计.md` §11 钦定：

> 建议统一定义业务错误类型，并统一在 `internal/pkg/errors` 中维护。
> 建议模式：repo 返回底层错误 → service 将底层错误转成业务错误 → handler 再映射为统一响应结构。
> 这样可以避免 SQL 错误直接暴露到接口层 / 各 handler 自己散乱地写错误码。

`_bmad-output/planning-artifacts/epics.md` 行 126 NFR18 同义。

### 1.2 上游 story 留下的三处 "等 1.8" 借条

- **Story 1.3 Recovery 中间件**（`server/internal/app/http/middleware/recover.go`）：原本用 `panicFallbackCode = 1009` literal + `response.Error(c, 500, panicFallbackCode, "服务繁忙")` 直接写 envelope；顶部注释明示 "Story 1.8 引入 AppError + ErrorMappingMiddleware 后，此常量会被替换为 errors.ErrServiceBusy 之类的枚举"
- **Story 1.5 sample service**（`server/internal/service/sample/service.go`）：`var ErrSampleNotFound = errors.New("sample not found")` 旁注 "本 story 用 stdlib errors.New 占位；Story 1.8 落地 AppError 框架后，真实 service 的业务错误会替换为 apperror.New(code, msg) 形式"
- **ADR-0001 §4 logger 字段表**：`error_code` 字段 "Epic 1 Story 1.8 生效。AppError 框架建立后，logging 中间件在 c.Errors 非空时读第一个 AppError 的 .Code 字段写入"

### 1.3 V1接口设计 §3 实际错误码数

epics.md 提到 "全部 26 个错误码"，但 V1接口设计 §3 实际定义 **32 码**（见本文 §5）。以 V1接口设计为权威来源。

### 1.4 Story 1.9 立即下游

Story 1.9 context 传播框架的 AC 第一行 "Given Story 1.8 AppError 框架已就绪 + Gin 默认会把 client 断开的信号传到 ctx" —— 本 ADR 落地是 1.9 的前置。

---

## 2. Decision

### 2.1 包结构

| 路径 | 包名 | 职责 |
|---|---|---|
| `server/internal/pkg/errors/apperror.go` | `apperror` | `AppError` struct + `Error()` / `Unwrap()` / `WithMetadata()` 方法 + `New` / `Wrap` / `As` / `Code` 工具函数 |
| `server/internal/pkg/errors/codes.go` | `apperror` | 32 个错误码常量 + `DefaultMessages` map |
| `server/internal/pkg/errors/apperror_test.go` | `apperror_test` | 7 条单元测试（含 32 码值 table-driven 核对） |
| `server/internal/app/http/middleware/error_mapping.go` | `middleware` | `ErrorMappingMiddleware` —— 把 c.Errors 翻译成 envelope |

**包名 vs 路径**：路径 `internal/pkg/errors`（对齐设计文档 §4 + §11 钦定），包名 `apperror`（避免与 stdlib `errors` 冲突）。Go 语言允许路径与包名解耦（如 `gopkg.in/yaml.v3` 包名 `yaml`），属社区常见做法。调用方惯用法：

```go
import (
    stderrors "errors"
    apperror "github.com/huing/cat/server/internal/pkg/errors"
)
```

### 2.2 三层映射约定

| 层 | 做什么 | 不做什么 |
|---|---|---|
| **Repository** | 返回原生 error（GORM 的 `gorm.ErrRecordNotFound`、Redis 的 `redis.Nil`、MongoDB 的 "no document"、SQL 驱动的网络错误等） | 不 wrap 为 AppError；不决定业务码（业务上下文不在 repo 层） |
| **Service** | 调 `apperror.Wrap(err, code, msg)` 把 repo error 转业务错误；业务码取自 codes.go 的 32 码 | 不直接写 HTTP 响应；不依赖 Gin / Echo 框架类型 |
| **Handler** | `c.Error(err)` + `c.Abort()`；happy path 调 `response.Success(c, data, msg)` | 不直接调 `c.JSON(...)` 写错误响应；不重复定义错误码常量 |
| **ErrorMappingMiddleware** | 扫 `c.Errors[0]`，AppError → envelope（status 由 code 决定）；非 AppError → wrap 1009 | 不修改 c.Errors（只读）；不再次包裹已是 AppError 的 error |

### 2.3 nil-safety 与穿透

- `apperror.Wrap(nil, code, msg)` **必返回 nil**（不返回 `*AppError{Cause: nil}`）—— 让 service 写 `return apperror.Wrap(s.repo.X(ctx), ...)` 一行就够
- `apperror.As(nil)` 返回 `(nil, false)`；`apperror.Code(nil)` 返回 `0`
- `*AppError.Unwrap()` 返回 `Cause`，让 `stderrors.Is(wrappedErr, sql.ErrNoRows)` 在 service 层穿透 wrap 链

---

## 3. Gin `c.Error()` 选型 vs handler signature wrapper

### 3.1 候选方案

| 方案 | 形式 | 优势 | 劣势 |
|---|---|---|---|
| **A：c.Error()（采纳）** | `func(c *gin.Context) { c.Error(...); c.Abort() }` | Gin 官方推荐；不改 handler 签名；与 Gin 中间件链原生兼容 | handler 多两行 boilerplate（c.Error + c.Abort） |
| **B：自定义 signature wrapper** | `func(c *gin.Context) *AppError`，外层 wrapper 接 return 值 | handler 写法简洁（一个 return） | 引入 wrapper 层；Gin 习惯断裂；新 dev 需要学非标准 pattern；middleware 链交互复杂 |
| **C：panic AppError** | `panic(apperror.New(...))`，Recovery 抓住 | 极简单 handler 写法 | panic 用作业务流程是反模式；性能差（栈展开）；与 panic-as-bug 语义混淆 |

### 3.2 选 A 的理由

- Gin 官方 [Custom Middleware tutorial](https://gin-gonic.com/docs/examples/custom-middleware/) 钦定 `c.Error()` 是错误聚合接口
- `c.Errors` 是 `[]*gin.Error`，按 `c.Error(err)` 顺序追加 —— middleware 在 `c.Next()` 之后扫描即可
- 不引入额外抽象层；新 dev 看 Gin 文档就能上手
- 多余的两行（c.Error + c.Abort）可考虑加 `errors.Abort(c, err)` helper，但本 ADR 不强制（保留给 Epic 4 落地时按需要决定）

---

## 4. Recovery + ErrorMappingMiddleware 协作图

### 4.1 中间件挂载顺序（router.go）

```
RequestID → Logging → ErrorMappingMiddleware → Recovery → handler
```

约束：**ErrorMappingMiddleware 必须外层于 Recovery**，否则 panic 路径下 envelope 永远写不出来。

### 4.2 panic 路径

```
1. handler panic("boom")
2. unwind 经过 Recovery 的 defer
3. Recovery defer 内：
     - reqLogger.ErrorContext("handler panic", panic, stack)
     - c.Error(apperror.Wrap(panicAsErr(rec), ErrServiceBusy, "服务繁忙"))
     - c.Abort()  ← 注意：不调 AbortWithStatus(500)，避免 WriteHeaderNow
4. Recovery 中间件正常返回（panic 已被 defer 吞掉）
5. 控制权回到 ErrorMappingMiddleware 的 after-c.Next() 代码
6. ErrorMappingMiddleware：
     - 扫 c.Errors[0] → As 出 *AppError(ErrServiceBusy)
     - 决策 status：ErrServiceBusy → HTTP 500
     - response.Error(c, 500, 1009, "服务繁忙") → 写 envelope
     - reqLogger.ErrorContext("error_mapping", code=1009, cause="boom")
7. 控制权回到 Logging 的 after-c.Next()
8. Logging 扫 c.Errors[0] → 追加 error_code=1009 到 http_request 日志
```

### 4.3 业务错误路径

```
1. handler 业务校验失败：
     c.Error(apperror.New(ErrInvalidParam, "userId 必填"))
     c.Abort()
2. handler 函数返回（无 panic）
3. Recovery defer：rec == nil，no-op
4. Recovery 返回到 ErrorMappingMiddleware
5. ErrorMappingMiddleware：
     - 扫 c.Errors[0] → As 出 *AppError(ErrInvalidParam)
     - 决策 status：业务码 → HTTP 200
     - response.Error(c, 200, 1002, "userId 必填")
     - reqLogger.WarnContext("error_mapping", code=1002, ...)
6. Logging 追加 error_code=1002 到 http_request 日志（INFO 级）
```

### 4.4 成功路径

```
1. handler 正常处理：response.Success(c, data, "ok")
2. handler 返回
3. Recovery defer：rec == nil，no-op
4. ErrorMappingMiddleware：c.Errors 为空 → no-op
5. Logging：c.Errors 为空 → 不追加 error_code 字段
```

### 4.5 HTTP status 决策逻辑

| 触发场景 | AppError.Code | HTTP Status | log level |
|---|---|---|---|
| panic 兜底 | ErrServiceBusy (1009) | 500 | ERROR |
| 非 AppError 兜底 | ErrServiceBusy (1009) | 500 | ERROR |
| 业务错误 | 1001-1008, 2xxx-7xxx | 200 | WARN |
| 成功 | — | 200 | INFO（http_request 默认）|

理由：HTTP 500 让 LB / Prometheus / Grafana / Alertmanager 在 HTTP 层就能识别"服务出大问题"，不用解 envelope；业务错误是预期路径（参数错 / 余额不足 / 房间满等），用 HTTP 200 + envelope.code，遵循 V1接口设计 §2.4。

---

## 5. 32 个错误码核对表（V1接口设计 §3 严格对齐）

### 5.1 通用错误码（1xxx）

| 常量名 | 数值 | V1 §3 中文描述 |
|---|---|---|
| `ErrUnauthorized` | 1001 | 未登录 / token 无效 |
| `ErrInvalidParam` | 1002 | 参数错误 |
| `ErrResourceNotFound` | 1003 | 资源不存在 |
| `ErrPermissionDenied` | 1004 | 权限不足 |
| `ErrTooManyRequests` | 1005 | 操作过于频繁 |
| `ErrIllegalState` | 1006 | 状态不允许当前操作 |
| `ErrConflict` | 1007 | 数据冲突 |
| `ErrIdempotencyConflict` | 1008 | 幂等冲突 |
| `ErrServiceBusy` | 1009 | 服务繁忙 |

### 5.2 认证 / 账号（2xxx）

| 常量名 | 数值 | V1 §3 中文描述 |
|---|---|---|
| `ErrGuestAccountNotFound` | 2001 | 游客账号不存在 |
| `ErrWeChatBoundOther` | 2002 | 微信已绑定其他账号 |
| `ErrAccountAlreadyBound` | 2003 | 当前账号已绑定微信 |

### 5.3 步数（3xxx）

| 常量名 | 数值 | V1 §3 中文描述 |
|---|---|---|
| `ErrStepSyncInvalid` | 3001 | 步数同步数据异常 |
| `ErrInsufficientSteps` | 3002 | 可用步数不足 |

### 5.4 宝箱（4xxx）

| 常量名 | 数值 | V1 §3 中文描述 |
|---|---|---|
| `ErrChestNotFound` | 4001 | 当前宝箱不存在 |
| `ErrChestNotUnlocked` | 4002 | 宝箱尚未解锁 |
| `ErrChestNotOpenable` | 4003 | 宝箱开启条件不满足 |

### 5.5 装扮 / 合成（5xxx）

| 常量名 | 数值 | V1 §3 中文描述 |
|---|---|---|
| `ErrCosmeticNotFound` | 5001 | 道具不存在 |
| `ErrCosmeticNotOwned` | 5002 | 道具不属于当前用户 |
| `ErrCosmeticInvalidState` | 5003 | 道具状态不可用 |
| `ErrCosmeticSlotMismatch` | 5004 | 装备槽位不匹配 |
| `ErrComposeMaterialCount` | 5005 | 合成材料数量错误 |
| `ErrComposeMaterialRarity` | 5006 | 合成材料品质不一致 |
| `ErrComposeTargetIllegal` | 5007 | 合成目标品质不合法 |
| `ErrCosmeticAlreadyEquipped` | 5008 | 装扮已装备 |

### 5.6 房间（6xxx）

| 常量名 | 数值 | V1 §3 中文描述 |
|---|---|---|
| `ErrRoomNotFound` | 6001 | 房间不存在 |
| `ErrRoomFull` | 6002 | 房间已满 |
| `ErrUserAlreadyInRoom` | 6003 | 用户已在房间中 |
| `ErrUserNotInRoom` | 6004 | 用户不在房间中 |
| `ErrRoomInvalidState` | 6005 | 房间状态异常 |

### 5.7 表情 / WS（7xxx）

| 常量名 | 数值 | V1 §3 中文描述 |
|---|---|---|
| `ErrEmojiNotFound` | 7001 | 表情不存在 |
| `ErrWSNotConnected` | 7002 | WebSocket 未连接 |

**核对**：1xxx(9) + 2xxx(3) + 3xxx(2) + 4xxx(3) + 5xxx(8) + 6xxx(5) + 7xxx(2) = **32 码**（不含 0=成功）。`apperror_test.go#TestAppError_AllCodesMatchV1Spec` 用 32 条 table-driven case 守护，并断言 `DefaultMessages` 含 32 条 entry。**未来如新增 / 修改 / 删除任何码值，必须同步更新 codes.go + 本表 + 单元测试三处**。

---

## 6. 业务错误的 HTTP status 取舍

### 6.1 选定

业务错误（任意非 1009 的 AppError）→ **HTTP 200 + envelope code≠0**。
panic / 非 AppError 兜底（统一 1009）→ **HTTP 500 + envelope code=1009**。

### 6.2 理由

- **客户端契约**：V1接口设计 §2.4 钦定 "code: 业务状态码，0 表示成功"；客户端永远先解 envelope.code 决策业务流程，HTTP status 只用于"请求送达 + server 正常处理完成"判断
- **运维契约**：HTTP 500 是 LB / Prometheus / Grafana / Alertmanager 在 HTTP 层就能识别的告警信号，不用解 envelope；业务错误是预期路径（参数错 / 余额不足等），不应触发 5xx 告警
- **简单**：status 决策完全在 ErrorMappingMiddleware 一处，不依赖 handler / service 显式传 status

### 6.3 否决候选

- **1001 → HTTP 401 / 1004 → HTTP 403 / 1003 → HTTP 404** 等映射：MVP 阶段简化；若未来需要（如 PWA / nginx 层级 auth 跳转），单独 ADR 演进，不影响本 ADR 的 envelope 契约
- **业务错误也走 5xx**：与 V1接口设计 §2.4 矛盾，且会触发不必要的告警

---

## 7. 与 ADR-0001 §4 对齐（`error_code` 字段）

ADR-0001 §4 logger 字段表：

| 字段 | 类型 | 来源 | 生效时机 |
|---|---|---|---|
| `error_code` | int | `*AppError.Code` | **Epic 1 Story 1.8 生效**。AppError 框架建立后，logging 中间件在 c.Errors 非空时读第一个 AppError 的 .Code 字段写入；成功请求省略该字段。 |

本 ADR 落地兑现：

- `server/internal/app/http/middleware/logging.go`：在 `c.Next()` 之后用 `apperror.As(c.Errors[0].Err)` 提取 code，追加到 `http_request` 结构化日志
- 成功请求 / 非 AppError 错误下 `error_code` 字段不出现（避免日志冗余）
- 测试守护：`logging_test.go#TestLogging_AddsErrorCodeFromAppError` + `TestLogging_NoErrorCodeWhenSuccess`

---

## 8. Future Migration（Epic 4+ 业务 service 落地范例）

### 8.1 Sample service 的迁移策略

`server/internal/service/sample/service.go` 当前 `var ErrSampleNotFound = errors.New(...)` **保留**（不在本 ADR 范围内迁移）。理由：

- sample 是 ADR-0001 §3.4 钦点的 mock 模板，**没有**业务码归属（不属 1xxx-7xxx 任一）
- 真实业务 service（Epic 4 auth / Epic 7 step / Epic 11 room / Epic 20 chest / Epic 26 cosmetic / Epic 32 compose）落地时，每个 service 自己用 `apperror.New(code, msg)` 替换占位
- sample 的 `ErrSampleNotFound` 演示 "service 层有业务错误概念" 的最小骨架，迁移到 apperror 反而会让 mock 模板带上 32 码包袱

### 8.2 业务 service 落地标准模板

```go
// repo 层（保留原生 error，便于 errors.Is/As 穿透识别）
func (r *UserRepo) FindByGuestUID(ctx context.Context, uid string) (*User, error) {
    var user User
    if err := r.db.WithContext(ctx).Where("guest_uid = ?", uid).First(&user).Error; err != nil {
        return nil, err
    }
    return &user, nil
}

// service 层（用 apperror.Wrap 转业务码）
func (s *AuthService) GuestLogin(ctx context.Context, uid string) (*Token, error) {
    user, err := s.userRepo.FindByGuestUID(ctx, uid)
    if err != nil {
        if stderrors.Is(err, gorm.ErrRecordNotFound) {
            return nil, apperror.Wrap(err, apperror.ErrGuestAccountNotFound, "游客账号不存在")
        }
        return nil, apperror.Wrap(err, apperror.ErrServiceBusy, "服务繁忙")
    }
    // ... 业务逻辑
    return token, nil
}

// handler 层（c.Error + c.Abort）
func GuestLoginHandler(c *gin.Context) {
    var req LoginRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        _ = c.Error(apperror.Wrap(err, apperror.ErrInvalidParam, "请求格式错误"))
        c.Abort()
        return
    }
    token, err := authSvc.GuestLogin(c.Request.Context(), req.GuestUID)
    if err != nil {
        _ = c.Error(err) // service 已 Wrap 为 AppError；非 AppError 也兼容（middleware 兜 1009）
        c.Abort()
        return
    }
    response.Success(c, token, "ok")
}
```

### 8.3 迁移检查清单（Epic 4+ 每个新 service 走一遍）

- [ ] repo 层：返回原生 error（不 wrap）
- [ ] service 层：catch repo error 后用 `apperror.Wrap` 转业务码
- [ ] handler 层：用 `c.Error(err)` + `c.Abort()`；happy 走 `response.Success`
- [ ] 业务码取自 `apperror.ErrXxx` 32 码常量，不另定义
- [ ] 单测覆盖：service 返回的 error 用 `apperror.As` 断言 code；handler 测试覆盖 "业务错误 → envelope code 正确"

---

## 9. Change Log

| Date | Change | By |
|---|---|---|
| 2026-04-24 | 初稿（Story 1.8 落地）：32 码常量 + AppError 类型 + ErrorMappingMiddleware + Recovery 重构 + Logging error_code | Developer |
