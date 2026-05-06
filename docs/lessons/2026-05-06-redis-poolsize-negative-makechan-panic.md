---
date: 2026-05-06
source_review: codex review (epic-loop /tmp/epic-loop-review-10-2-r1.md)
story: 10-2-redis-接入
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-06 — Redis PoolSize 负值绕过 fail-fast，go-redis NewClient panic

## 背景

Story 10.2（Redis 接入）r1 review。`server/internal/infra/config/loader.go` 给
`Redis.PoolSize` 做 zero-value 兜底，注释声明"<=0 → 默认"，但实现只判 `== 0`。
codex 指出：YAML 写 `redis.pool_size: -1` 会让 go-redis `NewClient` 内部
`makechan` 直接 panic（"makechan: size out of range"），绕过了 `redis.Open` 的
fail-fast 错误返回路径，server 启动期 SIGABRT。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Redis PoolSize 负值未兜底，go-redis NewClient panic | medium (P2) | error-handling / config | fix | `server/internal/infra/config/loader.go:138-139` |

## Lesson 1: loader 兜底必须让"会让下游 panic 的非法值"全部落进合法范围

- **Severity**: medium (P2)
- **Category**: error-handling / config
- **分诊**: fix
- **位置**: `server/internal/infra/config/loader.go:138-139`

### 症状（Symptom）

`Redis.PoolSize` 的兜底分支只判 `== 0`：

```go
if cfg.Redis.PoolSize == 0 {
    cfg.Redis.PoolSize = defaultRedisPoolSize
}
```

YAML 写 `pool_size: -1` 时，loader 不动它 → `redis.Open` 把 -1 透传给
`go-redis.NewClient(&Options{PoolSize: -1})` → 内部 `make(chan, -1)` 触发
`runtime.makechan` panic："makechan: size out of range"。整个 server 进程
SIGABRT，绕过 `redis.Open` 应有的"返回 startup error → main 退出码非零"链路。

注释还偏偏写着 "≤0 → 兜底"，注释和实现不一致 → 维护者误判语义。

### 根因（Root cause）

写 zero-value 兜底时把"YAML 缺字段"和"显式 0"两种情况合并考虑（都是 Go
struct 的 zero value），但漏了**显式负数**这第三种 happy-path 之外的输入。

更深一层的根因是：loader 兜底的真实职责不是"把缺字段补上"，而是"把所有
**会让下游 fail-fast 路径绕过的非法值**挤进合法范围"。判断"哪些值非法"
**必须看下游消费者如何处理**：

- go-redis：`PoolSize <= 0` 时**应当**用默认 10，但 `< 0` 时 internal
  `makechan(size)` 会 panic（Go runtime 限制：channel size 不能为负）→
  这是不可恢复的进程级崩溃，**必须**在 loader 层拦下
- gorm.MaxOpenConns / sql.DB.SetMaxOpenConns：负值被解释为"unlimited" →
  语义不同，可能允许透传

所以"`==` 还是 `<=`" 不是风格问题，是**对下游 panic 边界的认知问题**。

### 修复（Fix）

`server/internal/infra/config/loader.go`：把 `== 0` 改成 `<= 0`，注释补全
"为什么必须涵盖负数"的依据（go-redis NewClient internal makechan panic
约束）。

```go
// before
if cfg.Redis.PoolSize == 0 {
    cfg.Redis.PoolSize = defaultRedisPoolSize
}

// after
// 为什么必须涵盖负数：go-redis NewClient(Options{PoolSize: -1}) 会在内部
// makechan 处直接 panic("makechan: size out of range")，绕过 redis.Open 的
// fail-fast → 进程 SIGABRT 而不是返回 startup error。
if cfg.Redis.PoolSize <= 0 {
    cfg.Redis.PoolSize = defaultRedisPoolSize
}
```

`server/internal/infra/config/loader_test.go` 加 case
`TestLoad_RedisPoolSizeNegative_FallbackToDefault` + 新 fixture
`testdata/redis_negative_pool.yaml`（pool_size: -1），断言负值被兜底成 10。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **loader 层为某个数值字段做 zero-value 兜底**
> 时，**必须**先查清楚 **下游消费者（driver/library）对 < 0 / 越界值的处理
> 是 panic 还是 graceful fallback**；只要下游会 panic，loader 兜底条件就
> **必须**用 `<= 0`（或更宽的 range guard）而不是 `== 0`。
>
> **展开**：
> - 对 channel size / 缓冲池 size / goroutine 数 / 连接池 size 这类
>   "size 语义" 字段，下游 99% 概率是 `make(chan, n)` 或类似的 runtime call —
>   **Go runtime 拒绝 channel size < 0** → loader 必须先挤进合法范围
> - 对 timeout / interval / TTL 这类"时长语义"字段，下游通常会把负值当
>   `0` 或 `forever` 处理，可能不 panic（但语义错误） → 仍建议 `<=0` 兜底
>   防意外
> - 对 max_conns / rate_limit_per_min 这类"上限语义"字段，**有些**库把负值
>   当 unlimited（如 gorm.SetMaxOpenConns(-1) = unlimited） → 这类不能盲
>   目兜底，要按 story 业务语义决定是透传还是兜底
> - **写注释要和实现一致**：注释写 `≤0 → 默认` 但代码 `== 0` 是双倍违规
>   （误导维护 + 真实 bug）；改实现时必须同步注释
> - 测试必须**显式覆盖负值**，不能只测 `== 0` 兜底就当 `≤0` 也通过 ——
>   `== 0` pass 不能推出 `< 0` pass
> - **反例**：`if cfg.Redis.PoolSize == 0 { /* default */ }` 注释写"≤0 兜底"
>   但实现只兜 0。YAML 拼错 `pool_size: -1` → server 启动 panic
>   而非返回 startup error。
>
> **同主题 lesson 链**：
> - `docs/lessons/2026-04-26-yaml-default-must-not-mask-explicit-invalid.md`
>   讲的是反向：YAML **显式 0** 不该被 loader 静默替换成默认（针对
>   有"显式 0 = 禁用功能"业务语义的字段，如 RateLimit）。本 lesson 是正向：
>   PoolSize **没有**"显式负数 = 业务语义"，必须兜底防 panic。两条 lesson
>   共同的判断 key 是"该字段的 0 / 负值在业务和下游消费者那有没有真实语义"。
