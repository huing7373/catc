---
date: 2026-05-15
source_review: codex review round 4（epic-loop /tmp/epic-loop-review-20-8-r4.md）
story: 20-8-dev-端点-post-dev-grant-cosmetic-batch
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-15 — dev 路径检查必须在 caller 侧（基于 raw URL）而非 callee 侧（基于已解析 route pattern）（20-8 r4）

## 背景

Story 20.8 r3 给 metrics 包加了 `isDevPath` + `ObserveHTTP` 早返回，目标是把 `/dev/*` 流量从 prometheus counter / histogram 里彻底排除。codex r4 找出该 fix 的盲区：当 `/dev/*` 路由**未注册**（prod build / `-tags devtools` 缺失场景）时，Gin 的 `c.FullPath()` 返**空串** → `isDevPath("")` = false → metrics 把空串兜底成 `"UNKNOWN"` → 5xx-based 告警仍然吃 dev 端点污染。本 lesson 沉淀"为什么 callee 侧检查在该场景失效，以及 caller 侧检查为什么是根因 fix"。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | dev 路径检查必须在 caller 侧基于 raw URL.Path，而非 callee 侧基于 Gin 已解析 route pattern | medium | architecture | fix | `server/internal/app/http/middleware/logging.go`, `server/internal/infra/metrics/http.go` |

## Lesson 1: caller-side path check vs callee-side path check —— 边界识别原则

- **Severity**: medium
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/app/http/middleware/logging.go:97`（修复前）

### 症状（Symptom）

```
prod build（无 -tags devtools） + 任何 /dev/* probe 请求
  → handler 物理不存在 → Gin NoRoute → status=404
  → c.FullPath() = ""           （未匹配任何已注册 route）
  → metrics.isDevPath("") = false
  → ObserveHTTP("", method, 404, latency)
  → "" 兜底为 "UNKNOWN"
  → cat_api_requests_total{api_path="UNKNOWN", code="404"}++
  → 5xx-aware dashboard 把这桶当 server error 误报
```

r3 设计意图（"5xx alert 不该被 dev 流量污染"）**在该场景下完全失效** —— stub 阶段排除了 501，但 prod build 上的 404 没排除。

### 根因（Root cause）

**`isDevPath` 检查的输入是错的语义层 ——** 它检查的是 **Gin 已解析的 route pattern**（`c.FullPath()`），不是 **客户端实际请求的 URL**。两者在 happy path 一致，但在以下场景分叉：

| 场景 | URL（客户端） | FullPath（Gin 已解析 pattern） | 当前 isDevPath 结果 | 期望结果 |
|---|---|---|---|---|
| devtools build + dev 路由已注册 | `/dev/grant-cosmetic-batch` | `/dev/grant-cosmetic-batch` | true（skip）| true |
| **prod build + dev 路由未注册** | `/dev/grant-cosmetic-batch` | `""`（NoRoute）| **false（误计入）**| **true** |
| 任何场景 + 未知 URL | `/foobar` | `""` | false（→ "UNKNOWN" 计入）| false（NoRoute 必须可观测）|

callee 侧（metrics 包）只能见到 `FullPath()` 这层语义，没有 raw URL 信息 → 在"未注册路由"场景**本质上**不可能区分 dev 404 和真实 NoRoute 404。

更深一层根因：**"是否 dev 端点"这个判定的权威信源是 URL 本身（约定 `/dev/` 前缀），不是 Gin 的 route registration 状态**。callee 侧只能看到 routing 已发生**之后**的产物（pattern 或空），所以 callee 永远是次优观察位置。

### 修复（Fix）

在 logging middleware（caller 侧，能拿到 raw `c.Request.URL.Path`）做 prefix 检查，命中即跳过 `ObserveHTTP`：

```go
// server/internal/app/http/middleware/logging.go
const devURLPrefix = "/dev/"

// ... in Logging() handler，c.Next() + log 之后：
if !strings.HasPrefix(c.Request.URL.Path, devURLPrefix) {
    metrics.ObserveHTTP(c.FullPath(), c.Request.Method, status, latency)
}
```

`metrics.isDevPath` 保留作**双层防御**（已注册的 dev 路由从 callee 侧也能命中），但 caller 侧才是根因 fix —— 不依赖 Gin route registration 状态。

测试覆盖（`server/internal/app/http/middleware/logging_test.go`）：
- `TestLogging_DevURL_Unregistered_NotCounted`：故意不挂 `/dev/*` 路由 → 请求 `/dev/foo` → 404 + 不应出现在 metrics 文本里（关键防回归）
- `TestLogging_DevURL_Registered_NotCounted`：已挂 `/dev/registered-probe` 路由 → 200 + 仍不计 metrics（验证 caller 侧检查命中 happy path）
- `TestLogging_NonDevURL_StillCounted`：`/devops/healthz` / `/ping` → 必须正常计入（防误伤）

`server/internal/infra/metrics/http_test.go` 原有 case 保留（双层防御的 callee 层独立可测）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **"根据 URL 路径做路由分组级别的策略决策"（如 metric 豁免 / 限流分组 / auth scope 选择）** 时，**必须在 middleware caller 侧用 `c.Request.URL.Path` 做检查**，**禁止**在 callee（业务/基础设施层）依赖 Gin `c.FullPath()` 作为唯一权威源。
>
> **展开**：
> - `c.FullPath()` 是 Gin **已解析后**的 route pattern；它会因 route registration 状态变化而变化（未注册 → 空串）。把它当成"客户端意图"的权威源，会在 NoRoute 场景下漏判。
> - `c.Request.URL.Path` 是客户端原始请求 URL，不依赖路由表状态，是"客户端意图"的真正权威源。
> - 路由分组级策略（按前缀分类 `/dev/*` / `/internal/*` / `/admin/*`）属于**caller 侧职责**，应在 router-adjacent 中间件做；callee 侧（如 metrics observer / handler 本身）应该接受**显式 skip flag**（如本场景中"don't call ObserveHTTP at all"）而非自己解析路径。
> - **双层防御原则**：caller 侧检查是根因 fix；callee 侧检查（如 `metrics.isDevPath`）可保留为"加固层"，但**不能作为唯一防线** —— callee 看不到 routing 未发生时的状态。
> - **反例 1**（callee 单层 = 本 bug）：只在 `ObserveHTTP` 内 check `strings.HasPrefix(apiPath, "/dev/")`，apiPath 来源是 caller 传入的 `c.FullPath()`。NoRoute 场景下 apiPath="" → 检查失败 → 误计入 UNKNOWN 桶。
> - **反例 2**（caller 用错语义）：在 middleware 里写 `if strings.HasPrefix(c.FullPath(), "/dev/") { skip }` —— 看似在 caller 侧，但仍依赖 Gin 解析结果，bug 完全没修，只是把代码挪了位置。**必须用 `c.Request.URL.Path`**。
> - **反例 3**（依赖 route registration 状态决策）：写 `if c.FullPath() == "" { /* 当 NoRoute 处理 */ }` —— 这把"路由是否注册"当成"客户端意图"的代理信号。意图应直接从 URL 读，不该绕一层 Gin 解析。
>
> **场景识别启发**：当出现"以路径前缀做策略决策"的需求时，问自己 3 个问题：
> 1. 这条 URL 在路由表不存在时（NoRoute），策略还成立吗？→ 成立则必须 caller 侧 raw URL 检查
> 2. 这个策略要不要随路由注册状态变化？→ 不变化则不能用 `FullPath()`
> 3. callee 侧能不能拿到 raw URL？→ 拿不到则策略决策必须在 caller 侧做

## Meta: 本次 review 的宏观教训

**"权威信源识别"是 review 类问题的常见根因**：

r3 时 Claude 把 `c.FullPath()` 当成"路径检查的权威输入"，但它只是"Gin 解析后的产物"。r4 揭示的是：**任何依赖中间件/框架解析产物的策略决策，必须先问"如果解析失败 / 未注册，原始输入还能保留意图吗"**。
- HTTP path：`URL.Path`（raw）vs `FullPath()`（已解析）vs route group name（已分组）
- HTTP header：`r.Header.Get(...)`（raw）vs middleware 设置的 `c.Keys[...]`（已规范化）
- 这类二元区分在每个 web framework 都有，但每个 framework 的"raw vs parsed"对照表不同（Echo / Chi / Fiber 各有差异）

未来引入新框架 / 新中间件时，先列一遍"raw input vs parsed output"映射表，再决定 callee/caller 谁该看哪个。
