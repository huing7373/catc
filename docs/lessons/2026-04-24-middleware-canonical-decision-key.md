---
date: 2026-04-24
source_review: manual review by user (2 findings on Story 1.8 implementation)
story: 1-8-apperror-类型-错误三层映射框架
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-24 — 中间件之间的 canonical decision 必须走显式 c.Keys 而非各自从 c.Errors 推断

## 背景

Story 1.8 落地 `ErrorMappingMiddleware` 把 `c.Errors` 翻译成 envelope，同时 `Logging` 中间件按 ADR-0001 §4 追加 `error_code` 字段到 `http_request` 日志。两个中间件**各自**从 `c.Errors[0]` 用 `apperror.As` 推断 error_code —— 同一份原始数据、两个独立解读，看起来等价。

实际上两个独立解读会在两类边界场景失配：

1. **非 AppError 兜底路径**：handler `c.Error(io.EOF)` → ErrorMappingMiddleware **wrap** 为 1009 envelope 写出去；Logging 的 `apperror.As(io.EOF)` 返回 `(nil, false)` → 日志缺 `error_code` 字段
2. **double-write 路径**：handler 已 `response.Success` 又 `c.Error(AppError)` → ErrorMappingMiddleware 检测 `Writer.Written()==true` **保留**成功响应（不覆写）；Logging 仍按 `apperror.As(c.Errors[0])` 追加 `error_code` → 日志声称业务错误而响应实际是成功

两种场景的共同症状：**响应 envelope 与 http_request 日志的 error_code 不一致**，下游基于日志的 alert/dashboard 与客户端实际看到的状态对不上 → 监控失配（finding 1：漏报；finding 2：误报）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 非 AppError handler 路径下 Logging 丢 error_code | medium (P2) | error-handling / observability | fix | `server/internal/app/http/middleware/{logging,error_mapping}.go` |
| 2 | double-write 场景下 Logging 仍写 error_code | low (P3) | error-handling / observability | fix | `server/internal/app/http/middleware/{logging,error_mapping}.go` |

两条 finding **共用同一个修复设计**：让 ErrorMappingMiddleware 成为 canonical envelope.code 的唯一权威生产者，通过 `c.Keys[ResponseErrorCodeKey]` 显式发布；Logging 只读 key，不再扫 c.Errors。

---

## Lesson 1: 非 AppError handler 路径下 Logging 丢 error_code

- **Severity**: medium (P2)
- **Category**: error-handling / observability
- **分诊**: fix
- **位置**: `server/internal/app/http/middleware/logging.go:44-50`（旧）+ `error_mapping.go:68-71`

### 症状（Symptom）

handler 调 `c.Error(io.EOF)`（或任何非 AppError 的 stdlib error）：
- ErrorMappingMiddleware 正确兜底为 `apperror.Wrap(io.EOF, ErrServiceBusy, "服务繁忙")`，写 1009 envelope + HTTP 500
- Logging 的实现是 `if ae, ok := apperror.As(c.Errors[0].Err); ok { append error_code }`；对 `io.EOF` 而言 `apperror.As` 返回 `(nil, false)` → http_request 日志**没有** error_code 字段

客户端拿到 `{"code":1009, ...}`；运维拿到的日志里 `http_request` 缺 `error_code`，alert 规则 `count(error_code=1009) by ...` **漏掉**这一类系统级故障。

### 根因（Root cause）

设计上把 "canonical envelope.code" 这个**业务事实**让两个独立中间件**各自从 c.Errors 原始状态推断**：
- ErrorMappingMiddleware 推断："是 AppError 就用其 code，否则 wrap 为 1009"
- Logging 推断："如果 c.Errors[0] 能 As 出 AppError 就用其 code"

两条推断逻辑不等价 —— ErrorMappingMiddleware 多了一层 "wrap 为 1009" 的兜底，Logging 没有。c.Errors 是**原始**数据，不是 ErrorMappingMiddleware 的**决策**。

更深一层：**两个中间件对同一事实做独立解读，长期会漂移**。即便首版两边逻辑一致，未来 Logging 改一行 / ErrorMappingMiddleware 加个 case，两边就会失配。这是公共 fixture 的"语义等价承诺"问题（参考 lessons/2026-04-24-sample-service-nil-dto-and-slog-test-group.md Lesson 2）的另一种形态。

### 修复（Fix）

引入 `ResponseErrorCodeKey`（middleware 包级常量），由 ErrorMappingMiddleware 在写 envelope 后调 `c.Set(ResponseErrorCodeKey, ae.Code)` 显式发布它的决策。Logging 改为读 key 而非扫 c.Errors：

**before** (`logging.go`):
```go
if len(c.Errors) > 0 {
    if ae, ok := apperror.As(c.Errors[0].Err); ok {
        attrs = append(attrs, slog.Int("error_code", ae.Code))
    }
}
```

**after** (`logging.go`):
```go
if v, exists := c.Get(ResponseErrorCodeKey); exists {
    if code, ok := v.(int); ok {
        attrs = append(attrs, slog.Int("error_code", code))
    }
}
```

**新增** (`error_mapping.go`)：写 envelope 成功的路径上 `c.Set(ResponseErrorCodeKey, ae.Code)` —— 包括 AppError 和 non-AppError 兜底两条分支（兜底分支 ae.Code 已是 1009）。

**测试**：新增 `TestLogging_ErrorCode_ForNonAppErrorFallback`：handler `c.Error(io.EOF)` → 断言 http_request 日志含 `error_code=1009`。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写**多个中间件需要共享同一个 canonical decision**（如错误码 / 路由命中状态 / 限流决策）时，**必须**在产生决策的中间件里通过 `c.Keys` 显式 set 出去；下游中间件**禁止**从 c.Errors / c.Writer.Status() 等"原始状态"自行推断同一个决策值。
>
> **展开**：
> - **触发条件**：如果中间件 A 的"输出"会被中间件 B 引用（不论是日志、metric、二次响应），且这个输出是 A 经过推断/转换后的结果（非原始 c.Errors / c.Status 直读），那么就需要走 c.Keys 显式发布 + 下游显式读取。
> - **判断启发式**：如果你写中间件 B 时第一反应是"我也用 apperror.As(c.Errors[0]) 推断一遍"，停 —— 检查中间件 A 是否已经做过这个推断。如果做过，A 必须公开决策（c.Set），B 必须复用（c.Get），不许重复推断。
> - **设计思想**：原始状态（c.Errors / c.Status / c.Request）是**输入**，推断结果是**派生物**。多处复用派生物时，必须有**唯一权威生产者**（A 中间件）+ 显式发布通道（c.Keys）。否则两份独立推断长期一定漂移。
> - **反例 1**：Logging 用 `apperror.As(c.Errors[0])` 推 error_code，与 ErrorMappingMiddleware 的"是 AppError 用其 code，否则 wrap 1009"逻辑不等价；非 AppError 路径漏报 → 日志和响应不一致
> - **反例 2**（同形态另一个场景）：rate-limit 中间件写 `429` 到 c.Status，但同时有 metric 中间件按 `c.Status() == 429` 计数 —— 如果有第三个中间件改 status 到 200（如 envelope 化），metric 漏报。正确做法：rate-limit 用 `c.Set("rate_limit_blocked", true)`，metric 中间件读 c.Get
> - **反例 3**（设计臭味）：在中间件 A 写"如果 X 那么 status=Y，否则 Z"，同时在中间件 B 写"如果 status=Y 那么 ...，否则如果 X 那么 ..."—— B 的两条分支显然是想跟 A 同步逻辑，但 A 改一次 B 就忘改。正确：A 用 c.Keys 发"我做的决策是什么"，B 直接读
> - **验证手段**：写完中间件 B 后自问 —— "如果 A 的逻辑改一行（如新增 wrap 规则），B 会不会自动跟上？" 如果答案是"B 也要改"，就说明 B 没有走 c.Keys 复用 A 的决策，是潜在 bug

---

## Lesson 2: double-write 场景下 Logging 仍写 error_code

- **Severity**: low (P3)
- **Category**: error-handling / observability
- **分诊**: fix
- **位置**: `server/internal/app/http/middleware/logging.go:54-56`（旧）

### 症状（Symptom）

handler 是 dev bug：先 `response.Success(c, ..., "ok")` 写出 `{"code":0,...}` 成功响应，又 `c.Error(apperror.New(ErrInvalidParam, "too late"))` 推 AppError。

- ErrorMappingMiddleware 检测 `c.Writer.Written()==true` 正确**保留**成功响应（不覆写、不写 error envelope）
- Logging 仍扫 c.Errors[0]，apperror.As 拿到 ErrInvalidParam，追加 `error_code=1002` 到 http_request 日志

结果：客户端看到 `{"code":0,...}` 成功响应；运维看到日志里 `http_request` 含 `status=200 error_code=1002` —— alert 规则 `count(error_code != 0) by api_path` 把这个**实际成功**的请求误报成业务失败，触发**假阳性告警**。

### 根因（Root cause）

与 Lesson 1 同根：Logging 自行从 c.Errors 推断 error_code，没有看 ErrorMappingMiddleware 的决策。在 double-write 场景下，ErrorMappingMiddleware 的"决策"是**保留 success 响应、不覆写**（特意不发出 error envelope）—— 但这个决策没被显式公开，Logging 看 c.Errors 不知道这一点。

更狭义地说：c.Errors 这个数据结构在 Gin 里是 "handler 想报告的错误队列"，**不**等同于 "客户端实际看到的错误"。两者大部分时候一致，但在 double-write / handler 写完响应后追加 c.Error 等边界场景下分裂 —— Logging 必须看后者（客户端实际状态），不能只看前者（handler 意图）。

### 修复（Fix）

复用 Lesson 1 的 `ResponseErrorCodeKey` 机制：double-write 路径下 ErrorMappingMiddleware **故意不**调 `c.Set(ResponseErrorCodeKey, ...)`（语义：响应不是 error envelope，没有 canonical envelope.code 可发布）。Logging 读不到 key → 不写 error_code 字段 → 日志与实际 success 响应一致。

**关键代码片段**（`error_mapping.go`）：
```go
if c.Writer.Written() {
    // 故意不 Set ResponseErrorCodeKey：成功响应是客户端实际看到的
    reqLogger.LogAttrs(... "error_mapping skipped: response already written" ...)
    return
}
response.Error(c, httpStatus, ae.Code, ae.Message)
c.Set(ResponseErrorCodeKey, ae.Code)  // ← 仅在确实写了 envelope 才发布 key
```

**测试**：新增 `TestLogging_NoErrorCodeForDoubleWrite`：handler 先写 success 又 c.Error → 断言 http_request 日志**不含** error_code。`TestErrorMapping_DoubleWriteGuarded` 也追加断言：double-write 路径下 `c.Get(ResponseErrorCodeKey)` 返回 `(_, false)`。

### 预防规则（Rule for future Claude）⚡

> **一句话**：当中间件做的决策是 **"什么都不做 / 跳过本中间件的副作用"**（如"保留响应不覆写""跳过限流"），**必须**让 c.Keys 也显式区分"做了"和"跳过"两种状态 —— 跳过分支**不**写决策 key（key 不存在 = "本中间件未生效"），下游用"key 不存在"作为有效的负向信号，而非默认走"全跑了"路径。
>
> **展开**：
> - **触发条件**：中间件 A 有 fast-path / slow-path 两条分支，其中一条不产生副作用（例如 ErrorMappingMiddleware 的 double-write 跳过分支、rate-limit 的"已超额但配置容忍"分支、auth 的"匿名端点跳过"分支）。
> - **设计原则**：**key 的存在是肯定，key 的缺失也是信息**。不要在 fast-path 写 `c.Set(key, "skipped")`、slow-path 写 `c.Set(key, value)` —— 让 key 缺失自然表达"未生效"，下游 `c.Get(key)` 的 `exists=false` 分支自然处理。
> - **反例 1**：ErrorMappingMiddleware 在 double-write 路径里也调 `c.Set(ResponseErrorCodeKey, ae.Code)`（认为"反正这是 ae.Code，set 了不亏"）—— 然后 Logging 拿到 key 写 error_code，但响应实际是 success → 日志撒谎，假阳性告警。正确：跳过分支**不**Set key，让下游天然不写 error_code
> - **反例 2**（auth 中间件类似形态）：匿名端点跳过 auth 检查，但仍 `c.Set("user_id", "anonymous")`—— 然后业务 service 拿到 "anonymous" 当真用户 ID 跑业务逻辑（如查询订单）。正确：匿名跳过分支不 Set user_id，service 用 `c.Get("user_id")` 的 exists=false 分支显式拒绝（"本端点要 auth，没拿到 user_id 就 401"）
> - **反例 3**（限流中间件）：超额请求被放行（grace mode）后调 `c.Set("rate_limit_status", "passed_in_grace")`—— metric 看到 "passed" 关键字以为成功，漏报 grace 容忍数。正确：grace 路径不 Set；metric 用 `_, blocked := c.Get("rate_limit_blocked")` 这种二元判断，避免把 string 状态当 boolean 误读
> - **思维框架**：把 c.Keys 当成"声明性事实公告板"，不是"全量状态镜像"。每次 Set 都问自己 —— "这个值如果被下游读到了，会不会让下游做错事？" 如果 fast-path 的 set 会让下游误判，就**不要 set**

---

## Meta: 本次 review 的宏观教训

两条 finding 触及**同一个**思维漏洞：**多中间件协作时，"原始输入" 与 "经过推断的决策" 不是同一回事**。

中间件链典型形态：
```
input → middleware A → middleware B → middleware C → handler
                ↑                             ↑
          gin.Context 是公共载体     B 想知道 A 干了什么
```

新手直觉：B 看 gin.Context 的某个字段（c.Errors / c.Status / c.Keys["user_id"]）就能"复现 A 的决策"。这个直觉**只在简单场景成立**；只要 A 的决策包含**任何**条件分支（兜底 / 跳过 / 覆写），B 自行复现 A 的逻辑就一定会漂移。

正确范式：

1. **A 的决策 = 副作用 + 显式声明**。副作用（写响应 / 设 status）是给客户端的；显式声明（c.Set("xxx_decision", value)）是给下游中间件的。两者都要做。
2. **B 用 c.Get 复用 A 的声明**，绝不自行从 c.Errors / c.Status 推断同一个事实。
3. **A 的"跳过"分支同样要声明**（通过"不 Set key"的方式让 key 缺失成为有效信号），而不是 "fast-path 没事就不通知 B"。

应用到本项目：
- ErrorMappingMiddleware 是 canonical envelope.code 的**唯一**权威生产者
- Logging / 未来的 metrics / alert 中间件都从 `ResponseErrorCodeKey` 读
- 任何一处试图 `apperror.As(c.Errors[0])` 推断 error_code 都视为反模式 —— code review 时直接拒

未来如果 ErrorMappingMiddleware 的逻辑演进（如新增"5xx 的 AppError 映射到 4xx envelope code"等规则），所有下游消费者**自动**跟进，无需挨个改。这是 c.Keys 显式契约相对于"各自推断"的根本价值。
