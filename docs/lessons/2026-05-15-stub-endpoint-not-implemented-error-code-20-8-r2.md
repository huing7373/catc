---
date: 2026-05-15
source_review: codex review round 2 (epic-loop-review-20-8-r2.md) for Story 20-8 dev/grant-cosmetic-batch node-7 stub
story: 20-8-dev-端点-post-dev-grant-cosmetic-batch
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-15 — Stub endpoint 错误码不能复用通用 ErrServiceBusy —— HTTP status + log level 双层语义（20-8 r2）

## 背景

Story 20.8 r1 已把节点 7 stub 从 silent false-positive（`return nil` + HTTP 200 + envelope.code=0）改为 explicit failure（`return apperror.New(ErrServiceBusy, "...")`）。语义方向正确，但 codex r2 进一步指出 r1 选错了**具体**错误码：

```go
// r1 实装（service）
return apperror.New(apperror.ErrServiceBusy, "dev/grant-cosmetic-batch not yet implemented (node-7 stub; ...)")
```

`ErrorMappingMiddleware` 对 1009 的统一决策是：

```go
if ae.Code == apperror.ErrServiceBusy {
    httpStatus = http.StatusInternalServerError  // ⚠️ HTTP 500（不是 503）
    logLevel = slog.LevelError                    // ⚠️ ERROR log（污染监控）
}
```

两个 P2 后果：

1. **HTTP 500 而非 503**：dev-story 的实装文档 + r1 lesson 都说"middleware 翻 HTTP 503"，但 middleware 实际硬映射 1009 → 500。e2e 工具如果按 HTTP 503 检测"endpoint 还没激活"会失败。
2. **ERROR log 污染监控**：1009 的语义是"系统繁忙 / panic 兜底"—— middleware 钦定走 ERROR 级别以触发 LB / 监控告警。但 dev stub 端点每次调用都是**预期未激活路径**，不是系统错误；走 ERROR 会让 BUILD_DEV 环境的 e2e / demo 调用全部打到 ERROR dashboard，引发假告警。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | ErrServiceBusy 映射到 HTTP 500 而非语义正确的 501 | P2 | error-handling | fix | `server/internal/service/dev_cosmetic_service.go:118` + `error_mapping.go` |
| 2 | Deliberate stub calls 走 ERROR log → 污染监控 + 假告警 | P2 | error-handling | fix | 同上 |

两条同根 —— 都源于"stub endpoint 错误码不能复用 ErrServiceBusy"。修复方案是**引入新错误码 `ErrNotImplemented = 1010` + middleware 加路径**。

## Lesson 1: 错误码不是"凑用最近一个"——HTTP status + log level 是错误码携带的隐式契约

- **Severity**: P2
- **Category**: error-handling
- **分诊**: fix
- **位置**: `server/internal/service/dev_cosmetic_service.go:118` + `server/internal/app/http/middleware/error_mapping.go:113`

### 症状（Symptom）

r1 实装用 `apperror.New(ErrServiceBusy, ...)` 想表达"endpoint 未激活"，但 1009 在 middleware 钦定走 HTTP 500（不是 503，也不是更合适的 501）+ ERROR log。结果：

- e2e 工具按 503 检测"not yet implemented" → 收到 500 → 无法识别
- BUILD_DEV 环境每次 stub 调用打 ERROR 级别日志 → 污染监控 dashboard / 触发假告警
- dev-story 文档说"middleware 翻 503" → 与代码实际 500 不一致

### 根因（Root cause）

写 r1 的 fix-review 时把"middleware 把 503 unavailable 给你"当成了 1009 的固有属性，没去读 `error_mapping.go` 真实的 HTTP status 映射表。`ErrorMappingMiddleware` 的 status 决策 `httpStatus = http.StatusInternalServerError`（500，**非** 503）—— 1009 在本项目钦定就是"系统繁忙 / panic 兜底" → 500 + ERROR，与"endpoint 未实装"是**不同语义**。

错误码在本项目不是单纯的"业务码 + 文案"：每个 code 都通过 `ErrorMappingMiddleware` 携带两条隐式契约：

- **HTTP status**：1009 → 500；其他业务码 → 200（除非 middleware 加新路径）
- **Log level**：1009 → ERROR；其他业务码 → WARN

把 1009 用在"endpoint 未实装"等非系统错误场景，会让这两条契约都跑偏。**错误码不能"凑用最近一个"**；新的语义类别（"endpoint 未实装"）需要新的 code + 显式 middleware 路径。

### 修复（Fix）

引入 `ErrNotImplemented = 1010` 作为 dev / stub / preview 端点专用错误码：

**`server/internal/pkg/errors/codes.go`**：
```go
const (
    ...
    ErrServiceBusy    = 1009
    ErrNotImplemented = 1010 // 接口未实装（dev / stub / preview 阶段端点专用）
)
var DefaultMessages = map[int]string{
    ...
    ErrServiceBusy:    "服务繁忙",
    ErrNotImplemented: "接口未实装",
}
```

**`server/internal/app/http/middleware/error_mapping.go`**：
```go
switch ae.Code {
case apperror.ErrServiceBusy:
    httpStatus = http.StatusInternalServerError  // 500 + ERROR：系统级告警
    logLevel = slog.LevelError
case apperror.ErrNotImplemented:
    httpStatus = http.StatusNotImplemented       // 501 + WARN：dev/stub 未激活
    logLevel = slog.LevelWarn
}
```

**`server/internal/service/dev_cosmetic_service.go`**：
```go
return apperror.New(apperror.ErrNotImplemented, "dev/grant-cosmetic-batch not yet implemented (node-7 stub; ...)")
```

测试同步：service / handler / middleware 测试断言改为 1010 + HTTP 501 + WARN log。V1 接口设计 §3 错误码表追加 1010。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **service / handler 想 return AppError 表达"非常规拒绝"语义** 时，**必须先在 `error_mapping.go` 读 HTTP status + log level 决策表**，确认目标 code 携带的隐式契约匹配场景；不匹配就**新增 code + 新增 middleware 路径**，而不是凑用最近的 code。
>
> **展开**：
> - 错误码在本项目携带**三层契约**：① envelope.code（业务码本身）② HTTP status（middleware 决策）③ log level（middleware 决策）。三层都要匹配场景才能用
> - HTTP status 决策当前路径：1009 → 500 + ERROR；1010 → 501 + WARN；其他 → 200 + WARN。所有非"业务码 + 200"的场景都必须在 `error_mapping.go` 的 switch 里有**显式路径**
> - 新增错误码时同步五处：① `codes.go` 常量 + DefaultMessages ② `error_mapping.go` middleware 决策 ③ `error_mapping_test.go` 加 case 验 HTTP status + log level ④ `docs/宠物互动App_V1接口设计.md` §3 错误码表 ⑤ 业务实装 + 单测
> - **反例 1**：r1 用 `ErrServiceBusy` 表达"endpoint 未实装"—— 凑用最近的 code，结果 HTTP 500（非 501）+ ERROR log（污染监控），两个隐式契约都跑偏
> - **反例 2**：在 dev-story 实装文档写"middleware 翻 HTTP 503"，但没读 `error_mapping.go` 实际只翻 500 —— 文档凭印象写，与代码不一致；fix-review 阶段才发现

## Lesson 2: dev / stub / preview 端点是独立错误码类别，HTTP 501 (Not Implemented) 是标准语义

- **Severity**: P2
- **Category**: architecture
- **分诊**: fix
- **位置**: 同 Lesson 1

### 症状（Symptom）

r1 用通用业务错误码（1009 ErrServiceBusy）表达"dev 端点 stub 阶段未激活"。结果：

- 每次合法调用都打 ERROR 级别日志（BUILD_DEV 环境频繁触发 → 污染监控 dashboard）
- HTTP status 500 与"系统错误"语义混淆 —— LB / 监控按 5xx 计数 → 假告警
- e2e 工具无法区分"stub 阶段未激活"（预期）vs "系统真的挂了"（真问题）

### 根因（Root cause）

dev / stub / preview 端点本质上是**多阶段开发**的产物 —— 节点 N 阶段先把骨架立起来，节点 N+M 才激活真实逻辑。这是一个独立的**错误语义类别**，不应混入业务错误码（5xxx 装扮 / 6xxx 房间等）也不应混入系统错误（1009 ErrServiceBusy 服务繁忙）。

HTTP 501 (Not Implemented) 是 RFC 7231 §6.6.2 的标准语义：「server does not support the functionality required to fulfill the request」。完美匹配"端点已部署但功能未激活"的场景。e2e 工具 / 客户端能通过 501 立即识别"endpoint 在 dev / stub 阶段，等激活"，无需解析 envelope 文案。

### 修复（Fix）

`ErrNotImplemented = 1010` 专用于 dev / stub / preview 端点。中间件统一翻 HTTP 501 + WARN log（不污染 ERROR dashboard，但可通过 `phase=node-X-stub` grep 找出所有未激活端点）。生产端点**不应**用 1010；激活时由 owner 把 1010 替换为业务码或 nil。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写 dev / stub / preview 阶段端点** 时，**必须用 `ErrNotImplemented (1010) → HTTP 501 + WARN`**，**禁止**复用 `ErrServiceBusy (1009)` 或业务错误码（5xxx / 6xxx 等）。
>
> **展开**：
> - 错误码语义分类（按 middleware 决策）：
>   - **业务错误**（5xxx / 6xxx 等）→ HTTP 200 + WARN：客户端按 envelope.code 处理
>   - **参数错误 / 未登录 / 权限** (1001-1008) → HTTP 200 + WARN：同业务错误
>   - **系统繁忙 / panic 兜底** (1009 ErrServiceBusy) → HTTP 500 + ERROR：触发 LB / 监控告警
>   - **dev / stub / preview 未实装** (1010 ErrNotImplemented) → HTTP 501 + WARN：物理可达但功能未激活，不污染监控
> - dev 端点 stub 阶段**必须**返 1010 + 文案明确"node-N stub, awaits Story X.Y to activate"
> - 节点 M 激活真实逻辑时把 1010 替换为业务码或 nil；同步改 service / handler / middleware 测试
> - **反例 1**：用 1009 表达"endpoint 未实装" —— HTTP 500 触发监控告警 + ERROR log 污染 dashboard
> - **反例 2**：用 4xxx / 5xxx 业务错误码表达"endpoint 未实装" —— 客户端会按业务错误处理（如显示给用户"道具不存在"），语义错位
> - **反例 3**：dev 端点 stub `return nil` silent false-positive —— r1 lesson `2026-05-15-stub-endpoint-explicit-failure-20-8-r1.md` 已沉淀；r2 进一步细化"用哪个具体错误码"

## Meta: 本次 review 的宏观教训

r1 + r2 两轮 fix-review 共同的教训：**写 stub endpoint 时，"用什么错误码"和"用什么 HTTP status / log level"是同一个决策的两面**——不能只想"返个错就行"。错误码的隐式契约（HTTP status + log level）必须显式校对：

- r1 抓住了"必须 explicit failure"（核心方向对）
- 但 r1 选具体 code 时凭印象"1009 听起来像 503"，没读 middleware 真实决策表
- r2 修正：增加专用 code 1010，让 middleware 决策显式包含"endpoint 未实装"路径

未来 Claude 处理"想用 AppError 返个错"时，做完"选 code"这一步**立刻**去 `error_mapping.go` 查这个 code 的 HTTP status + log level，确认匹配场景才落地代码 —— 这是一个固定步骤，不应跳过。
