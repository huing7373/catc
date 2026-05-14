---
date: 2026-05-15
source_review: codex review round 3 (epic-loop-review-20-8-r3.md) for Story 20-8 dev/grant-cosmetic-batch node-7 stub
story: 20-8-dev-端点-post-dev-grant-cosmetic-batch
commit: 3b56be3
lesson_count: 1
---

# Review Lessons — 2026-05-15 — dev 端点的 metrics 边界：从一开始就该排除，5xx 不污染 alert（20-8 r3）

## 背景

Story 20.8 r2 已把 stub endpoint 错误码从 `ErrServiceBusy (1009)` 改为 `ErrNotImplemented (1010)` →
HTTP 501 + WARN log，避免触发 ERROR-level 监控告警。语义层面已经对，但 codex r3 进一步指出**还有一层污染没堵**：

```go
// logging.go middleware（每次请求末尾调）
metrics.ObserveHTTP(c.FullPath(), c.Request.Method, status, latency)
```

`ObserveHTTP` 会把 status code 拼成 label 灌进 `cat_api_requests_total{api_path, method, code}` counter。
即便我们让 dev stub 端点日志走 WARN，**HTTP 501 这个 status 仍然会被计入 5xx 维度**。任何基于 5xx 计数的 dashboard
/ Prometheus alert（如 `rate(cat_api_requests_total{code=~"5.."}[5m]) > 0.01`）依然会把每次 dev stub 调用
当作 server error 误报。

r2 修了"log dashboard 污染"，但 r3 抓到的是"metrics dashboard 污染" —— 这两个 dashboard 是独立的数据通路。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | dev 端点的 metrics 边界：从一开始就该排除，5xx 不污染 alert | P2 | architecture | fix | `server/internal/infra/metrics/http.go` + `http_test.go` |

只有一条 finding，但它推动的设计决策是**通用基建级的**：dev 端点对生产监控通道**全维度豁免**（log + metrics 都豁免），
不只是修 20.8 stub endpoint，而是把 20.7 force-unlock-chest / 7.5 grant-steps / 未来新增 dev 端点一起豁免。

## Lesson 1: 监控通道有多条独立数据通路，dev 端点必须**全维度**豁免，不能只堵 log 不堵 metrics

- **Severity**: P2
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/infra/metrics/http.go:39`（`ObserveHTTP` 入口）+ `server/internal/app/http/middleware/logging.go:97`（调用点）

### 症状（Symptom）

r2 已经让 dev stub 端点走 HTTP 501 + WARN log（不污染 ERROR-level log dashboard）。但 `ObserveHTTP` 仍把
501 拼进 `cat_api_requests_total{code="501"}` counter。任何基于 5xx 计数的 dashboard / Prometheus alert
都会把每次 dev stub 调用计为 server error，BUILD_DEV 环境 e2e / demo 流量会触发假告警。

具体证据：
- `metrics/http.go` 第 39 行 `ObserveHTTP` 入口无任何过滤逻辑，所有 status code（含 501 / 500）都进 counter
- `logging.go` 第 97 行无条件调 `metrics.ObserveHTTP(c.FullPath(), ...)` —— `/dev/grant-cosmetic-batch` 的
  pattern path 会以 `api_path="/dev/grant-cosmetic-batch"` + `code="501"` 进 counter
- 5xx 维度的 dashboard / alert 规则不会去手工排除每个 dev path（白名单维护成本高）

### 根因（Root cause）

写 r2 fix 时把"监控污染"等同于"log dashboard 污染"，没意识到 **log 和 metrics 是两条独立的数据通路**：

| 数据通路 | 写入点 | 消费方 | r2 状态 |
|---|---|---|---|
| 结构化 log（slog JSON）| `error_mapping.go` `reqLogger.LogAttrs(...)` | log dashboard / Loki / ELK | ✅ r2 已让 dev stub 走 WARN（不污染 ERROR） |
| Prometheus metrics | `logging.go` `metrics.ObserveHTTP(...)` | Prometheus → Grafana alert | ❌ r2 漏：501 仍计入 5xx counter |

错误码语义已经在 r2 修对（1010 → 501 + WARN），但**消费者侧**有两类：
- log dashboard 按 level 聚合 → WARN 不报警 ✅
- metrics dashboard 按 status code 聚合 → 501 仍算 5xx ❌

只要 dev 端点的请求**继续进 Prometheus counter**，就总会有维度让它污染告警（5xx rate / error rate / latency p99…）。
**更根因的解**：dev 端点对监控通道**全维度豁免**，而不是逐条调整 log level + alert 规则。

### 修复（Fix）

在 `metrics.ObserveHTTP` 入口加 dev 路径短路返回，**任何**以 `/dev/` 开头的 path 完全不进 counter / histogram：

```go
// server/internal/infra/metrics/http.go
const devPathPrefix = "/dev/"

func isDevPath(apiPath string) bool {
    return strings.HasPrefix(apiPath, devPathPrefix)
}

func ObserveHTTP(apiPath, method string, statusCode int, latency time.Duration) {
    if isDevPath(apiPath) {
        return // dev / stub / preview 端点全维度豁免监控
    }
    if apiPath == "" {
        apiPath = "UNKNOWN"
    }
    // ... 原 counter + histogram 逻辑
}
```

**为什么把 filter 放进 `ObserveHTTP` 内部而不是 caller 侧（logging.go）**：

| 方案 | 风险 |
|---|---|
| caller 侧过滤（logging.go 调前 if 一下）| 未来若加新 caller（如 WebSocket / RPC metrics）容易漏改 |
| `ObserveHTTP` 内部短路 ✅ | 单一防御点，所有 caller 自动受益（fail-closed 白名单） |

**测试覆盖** 3 个 case：

1. `TestObserveHTTP_DevEndpoint_NotCounted`：跑 4 个真实 dev path（含 20.8 grant-cosmetic-batch + status=501
   的 r3 核心 case），验 counter / histogram 都不增长
2. `TestObserveHTTP_DevPrefixDiscipline`：防 over-match —— `/dev`（无尾斜杠）/ `/devops/healthz` /
   `/api/dev/foo` 都必须正常计数
3. `TestIsDevPath`：内部 helper 的纯函数语义单测

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **加任何监控相关代码（log / metrics / trace / alert rule）** 时，**必须先列出该 feature
> 经过的所有监控数据通路**，逐通路检查 dev / stub / preview 端点是否被豁免；**禁止**只修一个通道的污染问题就声称"已解决"。
>
> **展开**：
> - 本项目监控数据通路（截至 2026-05-15）：
>   - **结构化 log（slog JSON）** → log dashboard：在 `error_mapping.go` / `logging.go` 写入，按 `level` 字段聚合
>   - **Prometheus metrics**（`cat_api_requests_total` + `cat_api_request_duration_seconds`）→ Grafana / alert：
>     在 `metrics.ObserveHTTP` 写入，按 `code` 标签聚合
>   - **future**：trace（OTel span）/ 业务事件流（kafka）/ audit log —— 加新通道时本规则同步扩展
> - dev / stub / preview 端点（`/dev/*` 路径）必须**在所有监控通道源头**豁免，不能依赖下游 alert 规则手工 exclude：
>   - log: dev stub 用 `ErrNotImplemented (1010) → WARN`（**不** ERROR）
>   - metrics: `/dev/*` 在 `ObserveHTTP` 入口直接 return（**不** 进 counter / histogram）
>   - 这是**白名单（fail-closed）** 策略 —— 加新 dev 端点的人不需要同步改告警规则，遗忘成本为零
> - 把 dev 路径 filter 放进**数据通路源头**（这里是 `metrics.ObserveHTTP`），不放在 caller 侧 —— 单点防御 + 所有 caller
>   自动受益。caller 侧 filter 是反模式：未来加新 caller 容易漏改
> - 加监控类 feature 时，验收 checklist 必须包含：
>   - [ ] log 通道是否区分 production / dev path？
>   - [ ] metrics 通道是否区分 production / dev path？
>   - [ ] alert 规则是否会被 dev path 误触发？（如果是，应改 metrics 源头而非 alert 规则）
> - **反例 1**：r2 只让 dev stub log 走 WARN（log 通道修对），但漏了 metrics 通道 —— HTTP 501 仍计入 5xx counter，
>   下游 5xx-based alert 仍会误报。**单维度修复是假象的"已解决"**
> - **反例 2**：在 5xx alert 规则里手工写 `code=~"5.." and api_path !~ "/dev/.*"` —— 黑名单维护成本高；每加一个 dev
>   端点都要同步改 alert 规则；忘改就误报。**源头 filter > 下游 exclude**
> - **反例 3**：caller 侧 filter（在 logging.go 调 ObserveHTTP 前 if 一下）—— 未来 WebSocket / RPC metrics 加新
>   caller 容易漏改。**filter 必须放在数据通路源头**（被调函数内部）

## Meta: 本次 review 的宏观教训

20.8 三轮 fix-review（r1 / r2 / r3）逐层揭开了一个"stub endpoint 实装"的完整决策链：

- **r1**：silent false-positive → explicit failure（**返不返错**这一层）
- **r2**：凑用 `ErrServiceBusy (1009)` → 引入专用 `ErrNotImplemented (1010)`（**返什么错码**这一层）
- **r3**：错码 + log level 都对了 → 但 metrics 通道仍污染（**监控通道豁免**这一层）

每一轮都不是"前一轮做错了"，而是"前一轮揭开的问题在下一层还有副作用没堵"。这种**"洋葱式"layered bug** 在涉及
基础设施 + 监控 + 错误处理三方交叉时尤其常见 —— 因为每一层（service / handler / middleware / metrics / log / alert）
都是独立 owner，单层 fix 难自证完整。

未来 Claude 处理"监控相关的修复"时，**必须画出 feature 经过的所有监控数据通路**，逐通路验收：
- log dashboard 通道 ✅/❌
- metrics dashboard 通道 ✅/❌
- alert 规则触发条件 ✅/❌
- trace / business event / audit ✅/❌（若适用）

**单通路修复不算 done**。把"全维度豁免"作为 dev 端点的**默认基建契约**，加进 onboarding checklist；
而不是每次 review 才回头补。
