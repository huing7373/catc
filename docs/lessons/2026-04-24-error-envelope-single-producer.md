---
date: 2026-04-24
source_review: 人工 review（Story 1.8 收尾后补触发的盲区，P2 一条）
story: 1-8-apperror-类型-错误三层映射框架
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-24 — error envelope 必须经 ErrorMappingMiddleware 单一产出，中间件绕过写 envelope 是反模式

## 背景

Story 1.8 引入 `ResponseErrorCodeKey` 让 ErrorMappingMiddleware 把 canonical envelope.code 广播给 Logging，修掉了 Logging 自行扫 `c.Errors` 推断的漂移（见 `2026-04-24-middleware-canonical-decision-key.md`）。

本次 review 暴露前一轮修复的**盲区**：`DevOnlyMiddleware`（`server/internal/app/http/devtools/devtools.go`）在 dev 兜底路径里**直接**调 `response.Error(c, 404, 1003, ...)` 写 envelope + `c.Abort()`，**绕过**了 `c.Error`/ErrorMappingMiddleware 管道 —— 所以 ErrorMappingMiddleware 的 `after c.Next()` 看到 `len(c.Errors)==0`，不会 Set `ResponseErrorCodeKey` → Logging 读不到 key → http_request 日志**缺** error_code，而客户端 envelope 实际是 `{"code":1003,...}`。

监控失配症状与 `2026-04-24-middleware-canonical-decision-key.md` 的 Lesson 1 同形态，但根因不同：前一条是"下游自行推断"，这一条是"上游绕过单一产出通道"。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | DevOnlyMiddleware 直接写 envelope 绕过 canonical 广播管道 | medium (P2) | error-handling / observability / architecture | fix | `server/internal/app/http/devtools/devtools.go:114`、`server/internal/app/http/middleware/error_mapping.go:14-35` 注释 |

## Lesson 1: error envelope 只能有一个生产者 —— ErrorMappingMiddleware

- **Severity**: medium (P2)
- **Category**: error-handling / observability / architecture
- **分诊**: fix
- **位置**: `server/internal/app/http/devtools/devtools.go:114`（旧实现）→ 已改走 c.Error 管道

### 症状（Symptom）

触发条件：启动时 `BUILD_DEV=true` 让 Register 挂了 `/dev/*` 路由组，运维在运行期把 `BUILD_DEV` 切成 `""`（极边缘但实现成本为零的热切换场景，也是 DevOnlyMiddleware 闸门 2 的主要价值所在）。

请求 `GET /dev/ping-dev` 时：

| 视角 | 实际看到的 |
|---|---|
| 客户端（HTTP 响应） | `HTTP 404 + {"code":1003,"message":"资源不存在",...}`（envelope 正常） |
| 运维（http_request 日志） | `{"msg":"http_request","status":404,...}` —— **缺 error_code 字段** |

基于日志的告警/看板（`count(error_code=1003)`、`rate(error_code != 0)` 等聚合规则）会**漏报**这类请求 —— 但"有人在探测 dev 端点"恰恰是**我们特别想看到**的信号。

### 根因（Root cause）

上一轮 lesson `middleware-canonical-decision-key` 定下规则：**Logging 不许自行扫 c.Errors 推断 error_code，canonical envelope.code 只从 `ResponseErrorCodeKey` 读**。这条规则假设了"所有 error envelope 都由 ErrorMappingMiddleware 写出" —— 但 DevOnlyMiddleware 跳过了这条管道，直接 `response.Error(...)`，于是 canonical key 自然不被 Set。

上一轮 lesson 堵死的是**下游消费侧**（Logging 读 key），但没堵**上游生产侧**（谁允许写 envelope）。所有"直接写 envelope 且不走 c.Error 管道"的路径都是这条盲区的候选 —— DevOnlyMiddleware 是首个暴露的，未来若不立新规矩，第二个、第三个绕过者只是时间问题。

更深一层：`ResponseErrorCodeKey` 契约在 Story 1.8 落地时只写了"ErrorMappingMiddleware 与 Logging 之间"—— 契约描述不完整，让"什么中间件有权力写 envelope" 这个问题没有明确约束，靠惯例自律。

### 修复（Fix）

**方案选择**：review 给了两条路 —— (A) DevOnlyMiddleware 转 `c.Error(apperror.New(...))` 走 ErrorMappingMiddleware；(B) 保留 `response.Error` 但追加 `c.Set(ResponseErrorCodeKey, ...)` 自发布 canonical key。

选 **A**，理由：

- A 把 "error envelope 生产者" 收敛到**唯一**一个（ErrorMappingMiddleware），规则最紧；B 允许多生产者并存但要求每个自律 Set key，未来仍会长新盲区
- A 顺带把 HTTP status 决策权也统一到 ErrorMappingMiddleware（V1接口设计 §2.4 "业务码与 HTTP status 正交，仅 1009 走 500"）；B 让每个 middleware 自己决定 HTTP status，长期会漂移

**代价**（客观记录）：

- DevOnlyMiddleware 的 HTTP status 从 404 → 200。原本注释里写的 "OpSec：让被拒的 dev 端点外观与路径不存在无差别" 这层 HTTP status 仿真失效（envelope message "资源不存在" 仍保留）。扫描器可通过 `200 JSON envelope` 与 Gin 默认 NoRoute `404 text/plain` 的外观差识别 dev 路由存在
- 若未来需要恢复严格外观隐藏，**正确做法**：在 router 层加 custom NoRoute handler 让整个系统对未命中路径统一响应形态，而**不是**退回"让业务 middleware 各自写 HTTP status"

**代码改动**（`devtools.go:111-126`）：

```go
// before
response.Error(c, http.StatusNotFound, devNotFoundCode, "资源不存在")
c.Abort()

// after
_ = c.Error(apperror.New(apperror.ErrResourceNotFound, "资源不存在"))
c.Abort()
```

**契约加强**（`error_mapping.go` `ResponseErrorCodeKey` 注释）：显式写入"**只有 ErrorMappingMiddleware 写 error envelope + Set 本 key**，其它代码一律通过 `c.Error` 推错误；**禁止**直接调 `response.Error(...)` 绕过本管道"。

**测试**：

- `TestDevOnlyMiddleware_RejectsWhenDisabled`（devtools_test.go）：挂上 ErrorMappingMiddleware（DevOnlyMiddleware 从此依赖它），HTTP status 断言从 404 改为 200，envelope.code=1003 保留
- 新增 `TestRouter_DevOnlyMiddleware_FallbackPath_LogsCanonicalErrorCode`（router_dev_test.go）：完整 middleware 链下，模拟"启动时 BUILD_DEV=true 挂路由 → 运行期切 ''" 热切换场景，断言 envelope.code=1003 **且** http_request 日志 error_code=1003 —— 这是防"DevOnlyMiddleware 未来又退化为自写 envelope"的回归测试

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 HTTP middleware / handler 里**严禁**直接调 `response.Error(...)` 产出 error envelope；所有业务错误一律 `c.Error(apperror.New(code, msg))` + `c.Abort()` 推到 `c.Errors`，由 ErrorMappingMiddleware 统一翻译成 envelope。`response.Error` 只保留给 ErrorMappingMiddleware **自己**调。
>
> **展开**：
> - **触发条件**：写任何 gin.HandlerFunc / gin middleware，要报告业务错误时 —— 不管是 auth 失败 / 参数校验挂掉 / 资源不存在 / 限流 / dev-only 拒绝。
> - **判断启发式**：代码里如果写了 `response.Error(c, ...)`（包且 import 路径是 `internal/pkg/response`），立刻 stop。问自己："能不能改成 `c.Error(apperror.New(...)); c.Abort()`？" 答案 99% 是能。
> - **唯一例外**：ErrorMappingMiddleware 自己 —— 它是**终端**消费者 + canonical 生产者，自己不能再推 c.Error（会无限递归）
> - **如果要 HTTP status ≠ ErrorMappingMiddleware 默认规则的响应**（如 403 / 429）：不要在业务 middleware 里写 `response.Error` 指定 status —— 正确做法是扩展 ErrorMappingMiddleware 的 status 决策表（如新增"401/403 类 AppError → HTTP 对应状态"），或者走 gin 的 custom NoRoute / 401 handler 统一决策。**不要**让多个 middleware 各自决定 HTTP status
> - **reviewer 视角**：PR 里看到 `response.Error(c, ...)` 调用点且不是 ErrorMappingMiddleware 内部 —— **直接打回**
> - **反例 1**：DevOnlyMiddleware 旧版 `response.Error(c, 404, 1003, "资源不存在")` 直接写 envelope，绕过 canonical key 广播 → 日志缺 error_code 与响应不一致
> - **反例 2**（类比形态）：假设未来 RateLimitMiddleware 想返回 `429 + envelope(code=1010, "请稍后重试")`，随手 `response.Error(c, 429, 1010, ...)` —— 同样的 bug 会复现（日志 error_code 缺失 + HTTP status 决策分散）。正确：`c.Error(apperror.New(ErrRateLimited, ...)); c.Abort()`，然后在 ErrorMappingMiddleware 的 status 决策表里加"1010 → HTTP 429"
> - **反例 3**（类比形态）：AuthMiddleware 想返回 `401 + envelope(code=1001)`，直接 `response.Error(c, 401, 1001, ...)` —— 同样错误
> - **思维框架**：把 gin.Context 的错误通路当成**事件总线**：
>     - 生产者（业务 middleware / handler）→ `c.Error(err)` 发事件
>     - 消费者（ErrorMappingMiddleware）→ `c.Errors` 订阅 + 翻译成统一 envelope + 广播 canonical key
>     - 下游（Logging / metrics）→ 只从 canonical key 读决策
>     - 任何"生产者直接写 envelope 绕过消费者"都会破坏下游对一致性的假设
> - **验证手段**：写完业务 middleware / handler 自问 —— "我有没有调 `response.Error`？如果有，我有理由不是 ErrorMappingMiddleware 吗？" 若理由是"我想要 HTTP status 非默认" —— 去改 ErrorMappingMiddleware 的决策表，别自己写

---

## Meta: 本次 review 的宏观教训

前一轮 lesson（`middleware-canonical-decision-key`）解决了"**下游多个消费者对同一事实自行解读会漂移**"问题。本轮 lesson 把规则推向**上游**：**同一事实也只能有一个生产者**。

两条 lesson 合起来构成 gin middleware 协作的完整范式：

```
          单一生产者                  显式契约              多消费者
          ErrorMapping                c.Keys           Logging / Metrics / ...
(error) ─────────────▶  envelope.code ──────────▶  log.error_code / metric.tag / ...
   ↑                        ↑                            ↑
   只有它能写              canonical key               只从 key 读
   (response.Error)        (ResponseErrorCodeKey)      (禁止自行推断)
```

每次新加一个 middleware / handler，都要先定位它在这张图的哪个位置：

- 生产者？—— 应该 `c.Error()` 推事件给 ErrorMappingMiddleware，**不要**自己写 envelope
- 消费者？—— 应该 `c.Get(ResponseErrorCodeKey)` 读，**不要**自己扫 c.Errors 推断
- 新的特殊响应需求？—— 去**扩展 ErrorMappingMiddleware**（如 status 决策表、特殊 code 映射），**不要**开新 envelope 写入口

这个范式把"多中间件协作"的复杂度收敛到"一个 canonical 中间件 + 多个读订阅" —— ErrorMappingMiddleware 是 server 错误路径上的 **single source of truth**。长期演进只动这一个中心点，其它中间件只推事件 + 读决策，整条链自然保持一致。

如果未来发现 ErrorMappingMiddleware 的决策规则不够用（新业务需要更复杂的 status/code 映射），优先考虑"让它更聪明"而非"允许别人绕过它"。
