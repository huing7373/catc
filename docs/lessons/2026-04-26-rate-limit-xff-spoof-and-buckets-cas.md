---
date: 2026-04-26
source_review: /tmp/epic-loop-review-4-5-r1.md (codex review of Story 4.5)
story: 4-5-auth-rate_limit-中间件
commit: 933c71b
lesson_count: 2
---

# Review Lessons — 2026-04-26 — IP 限频 key 必须用 RemoteIP + SetTrustedProxies 锁定 Gin / atomic + sync.Map 必须用 CAS 才真正 bounded

## 背景

Story 4.5 引入 `RateLimit` / `RateLimitByIP` / `RateLimitByUserID` 中间件
（`server/internal/app/http/middleware/rate_limit.go`）。本轮 codex review
（round 1）指出两条**实际 bypass 当前实装的安全 / 正确性问题**：(1) IP 维度
key 取自 `c.ClientIP()`，可被 `X-Forwarded-For` 伪造绕过 60/min 限制；(2)
`BucketsLimit` 在并发洪泛下不真实 bounded —— atomic check + 后置 increment
之间存在 race，map 大小可膨胀超过 limit。两条都属"看起来线程安全 / 看起来
配套了反代假设，实际不安全"的反模式。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | IP 限频 key 用 c.ClientIP() 可被 X-Forwarded-For 伪造绕过 | high | security | fix | `server/internal/app/http/middleware/rate_limit.go:30,49` + `server/internal/app/bootstrap/router.go:80` |
| 2 | `count.Load() < limit` 后 `count.Add(1)` 不原子，并发洪泛下 BucketsLimit 不真 bounded | medium | perf / security | fix | `server/internal/app/http/middleware/rate_limit.go:142-147` |

## Lesson 1: IP 限频 key 用 c.ClientIP() 可被 X-Forwarded-For 伪造绕过

- **Severity**: high
- **Category**: security
- **分诊**: fix
- **位置**: `server/internal/app/http/middleware/rate_limit.go:30-31` + `:49`；配套修
  `server/internal/app/bootstrap/router.go:80`

### 症状（Symptom）

- `RateLimitByIP` 用 `c.ClientIP()` 作为 key prefix。
- Gin 的 `c.ClientIP()` 默认会**信任** `X-Forwarded-For` / `X-Real-IP`
  header（除非 engine 显式调过 `SetTrustedProxies` 限制信任范围）。
- `bootstrap.NewRouter` 没调 `SetTrustedProxies`，因此 Gin 默认 trustedProxies
  是 `0.0.0.0/0`（信任**任意**反代 IP）。
- 攻击者循环伪造 `X-Forwarded-For: 10.0.0.1`、`10.0.0.2`、…→ 每次都被 Gin
  采信为新客户端 IP → 每次都是新 key → 60/min 限制完全失效。
- 同样的 bypass 也影响 `/auth/guest-login` 这类登录前路径（V1 §4.1 钦定的同 IP
  60/min 实际不存在）。

### 根因（Root cause）

- `c.ClientIP()` 的语义不是"server 端可信源 IP"，而是"按 trustedProxies 配置
  解析出来的应用层 IP"。它**默认行为**就把所有 `RemoteAddr` 当反代 IP（信任）。
- 写代码的人如果脑里默认 "ClientIP = 真客户端 IP"，会忽略一个**安全前提**：
  **必须先在 engine 层显式锁定 trustedProxies**，否则 ClientIP 是攻击面，不是
  真客户端 IP。
- rate_limit 顶部注释写过"生产 reverse proxy 部署需配 SetTrustedProxies；节点
  2 阶段单实例无 reverse proxy"——**但没在 NewRouter 调** `SetTrustedProxies(nil)`
  把"无反代"这件事**告诉 Gin**。注释 != 代码。Gin 不读注释，它读默认值。

### 修复（Fix）

两步配套（任一不做都是漏的）：

1. `bootstrap/router.go` 在 `gin.New()` 后立刻 `r.SetTrustedProxies(nil)`：
   - `nil` 表示"不信任任何反代"，`c.ClientIP()` 退化为 `RemoteAddr` 的 host
     部分，跳过 XFF 解析。
   - 同时修复 `logging.go` / `devtools.go` 里 `c.ClientIP()` 的 audit 字段
     （它们用 ClientIP 不是限频维度，但 audit 日志被 XFF 污染同样有害）。
2. `rate_limit.go` `RateLimitByIP` / `RateLimitByUserID.fallback` 都换成
   `c.RemoteIP()`：
   - `RemoteIP()` 直接取 `Request.RemoteAddr` 的 host 部分，**不**经过
     trustedProxies 解析路径，**永远**返回 TCP 连接源 IP。
   - 这是 defense-in-depth：即使 future 改 `SetTrustedProxies` 出错或漂移，
     限频 key 仍然 server-only 可信。

```go
// before
func RateLimitByIP(c *gin.Context) string { return "ip:" + c.ClientIP() }

// after
func RateLimitByIP(c *gin.Context) string { return "ip:" + c.RemoteIP() }
```

```go
// bootstrap/router.go NewRouter
r := gin.New()
_ = r.SetTrustedProxies(nil)  // 安全默认：不解析 XFF
```

新增白盒测试 `TestRateLimit_XFFSpoofing_DoesNotBypass`：固定 RemoteAddr +
轮换 XFF header，前 burst 次通过、第 burst+1 次必须被拒（envelope 1005）。
旧实现这条 fail。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **任何"按 IP 维度做安全决策"的代码（rate limit / IP
> 黑白名单 / 审计日志 / 风控）** 时，**禁止直接用 `c.ClientIP()`**，必须 (a) 在
> Gin engine 层显式调 `SetTrustedProxies(nil 或白名单)`、且 (b) 安全决策点用
> `c.RemoteIP()` 而不是 `c.ClientIP()`，做双层防御。
>
> **展开**：
> - Gin 的 `c.ClientIP()` 默认 trustedProxies 是 `0.0.0.0/0`，等价于"信任所有
>   人发 X-Forwarded-For"——这是**安全反模式默认值**，必须显式覆盖。
> - `SetTrustedProxies(nil)` = 不信任任何反代（适用 server 直对外）；反代部署
>   时改成 CIDR 白名单（CDN / nginx 出口 IP 段）。
> - 即使 `SetTrustedProxies` 配对了，安全决策点（rate limit key / audit）仍应
>   优先 `c.RemoteIP()` —— 它绕过 trustedProxies 解析，永远是 TCP 连接源 IP。
>   双层防御，配置漂移仍兜底。
> - 注释写了"生产部署需配 SetTrustedProxies"**不算修复**，必须代码里**真**调
>   `SetTrustedProxies`。Gin 的默认值是反直觉的，注释提醒人但不提醒框架。
> - **反例**：rate_limit 中间件 key 用 `c.ClientIP()`，bootstrap 不调
>   `SetTrustedProxies`，假设"节点 2 没反代所以 ClientIP 就是 RemoteAddr"。
>   实际：Gin 默认信任全网反代 → ClientIP 直接读 client 发的 XFF → 限频被
>   一行 header 绕过。

## Lesson 2: atomic check + 后置 increment 之间存在 race，counter + sync.Map 必须用 CAS 预占才真 bounded

- **Severity**: medium
- **Category**: perf / concurrency
- **分诊**: fix
- **位置**: `server/internal/app/http/middleware/rate_limit.go:142-147`

### 症状（Symptom）

- `BucketsLimit` 设计意图是"buckets map 大小不超过 N，超过的 key 共享 overflow
  限流器"，防 IP 洪泛攻击撑爆内存。
- 旧实现：

  ```go
  if count.Load() < limit {
      newLim := rate.NewLimiter(...)
      actual, loaded := buckets.LoadOrStore(key, newLim)
      if !loaded { count.Add(1) }
  }
  ```

- race：N 个 goroutine 用 N 个不同 key 同时进入，全部观察到 `count.Load() <
  limit`（因为 `count.Add` 还没发生），各自 `LoadOrStore` 不同 key，最后
  `count.Add(1) × N`。结果 buckets map 大小可达任意值，`BucketsLimit` 形同
  虚设。
- 在"洪泛攻击"（恰恰是 BucketsLimit 想防御的场景）下，这条 race 100% 触发。

### 根因（Root cause）

- atomic 操作的"check + act"分裂成两步，中间没有锁/CAS 保护，是典型的
  TOCTOU（Time-of-check / Time-of-use）漏洞。
- `sync.Map.LoadOrStore` 自身原子，但它**只**保证"key 存在性"原子，不保证
  "插入数 ≤ 全局上限"原子。上限是分离的 atomic counter，必须用 CAS 把
  "占槽位 + 写入 map" 一起做成原子序列。
- 写代码时容易被 `sync.Map` + `atomic.Int64` 这两个"线程安全"零件骗到，以为
  组合就是线程安全 —— **零件线程安全 ≠ 用法线程安全**。

### 修复（Fix）

CAS 预占模式：

```go
reserved := false
for {
    cur := count.Load()
    if cur >= limit { break }                 // 满了：放弃，走 overflow
    if count.CompareAndSwap(cur, cur+1) {
        reserved = true
        break
    }
    // CAS 输给别人，重读 cur 重试
}
if !reserved {
    lim = overflow
} else {
    newLim := rate.NewLimiter(...)
    actual, loaded := buckets.LoadOrStore(key, newLim)
    lim = actual.(*rate.Limiter)
    if loaded {
        // 同 key 已被别人写入：撤销刚抢到的槽位
        count.Add(-1)
    }
}
```

关键点：
- **先 CAS 占槽位**：保证"buckets 实际写入 ≤ count" 这个不变量。CAS 失败重
  读 + 重试是惯用法，不是 livelock 风险（CAS 在 hot loop 下也是 fast path）。
- **LoadOrStore 撞 key 已存在则撤销槽位**：不撤销会让 count 单调膨胀（同 key
  被多次访问后 count 漂移到 limit，新 key 全部走 overflow，效果上 limit
  自动衰减）。
- 用 `atomic.CompareAndSwap` 而不是 mutex：避免限流 hot path 多余锁竞争；
  CAS 自带 happens-before 边界。

新增白盒测试 `TestRateLimit_ConcurrentFlood_BucketsBounded`：500 个 goroutine
用 500 个不同 key，BucketsLimit=50 → 断言 `count <= 50`。旧实现这条 fail。

为支撑测试断言 internal counter，把 `RateLimit` 拆出 `newRateLimit` 内部工厂
（额外返回 `*atomic.Int64`），`RateLimit` 是 thin wrapper 丢弃 counter。
生产 API 不变。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **看到 `if atomic.Load() < limit { ... atomic.Add(1)
> ... }` 这种"读后写不原子"模式** 时，**必须改成 CAS 预占模式**
> （`for { cur := Load(); if cur >= limit { break }; if CompareAndSwap(cur, cur+1) { ... } }`），并且配套写**并发洪泛单测**断言上限。
>
> **展开**：
> - 即使 `sync.Map` + `atomic.Int64` 各自线程安全，"check counter → 操作 map
>   → bump counter" 这种序列**不是**原子的，多 goroutine 并发会越过 limit。
> - CAS 模式必须配套"操作失败时撤销 reserve" 的 undo 分支（如 LoadOrStore
>   loaded=true 时 `count.Add(-1)`），否则 counter 单调膨胀，limit 自动衰减。
> - 并发性测试必须用 **N >> limit** 的 distinct key 触发洪泛 + `WaitGroup`
>   等齐 + 后置断言 count ≤ limit。**不**用串行循环 —— 串行 race 永远不出现。
> - 跑 `go test -count=10 ./...`（Windows 无 race detector） 或
>   `go test -race -count=10 ./...`（Linux/macOS）验证稳定性。一次绿不算稳定。
> - **反例**：`sync.Map.LoadOrStore` + `atomic.Int64` 拼一起，写"我用了线程
>   安全零件，所以是线程安全的"注释而不写并发测试 → 100% race 实装。
> - **反例 2**：mutex 包整段（`mu.Lock; check; LoadOrStore; bump; mu.Unlock`）
>   —— 正确但 hot path 上每次都锁竞争，限流中间件的 QPS 上限被人为压低。
>   CAS 是更优解。

---

## Meta: 本次 review 的宏观教训

两条 finding 都是同一类思维漏洞的不同切面：**"零件级安全 / 零件级线程安全"
不蕴含"系统级安全 / 系统级线程安全"**。

- Lesson 1：Gin 的 `c.ClientIP()`、`SetTrustedProxies` 各自有定义良好的语义，
  但默认拼装方式（`gin.New()` + 用 `ClientIP` 做安全决策）在威胁模型下不
  安全。必须在 engine 层 + 决策点都做防御。
- Lesson 2：`sync.Map` / `atomic.Int64` 各自线程安全，但拼成"先 check 再
  insert 再 bump"序列后整体不是原子。必须 CAS / mutex 把序列原子化。

未来 Claude 写涉及 (a) HTTP 客户端 IP 任何用途、(b) 多个并发原语组合实现
"上限 / 全局不变量" 的代码时，应**默认怀疑"零件 + 零件 = 安全"**，主动设
计威胁模型 + 写攻击者视角的测试。
