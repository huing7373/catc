---
date: 2026-05-06
source_review: codex review (epic-loop r2 for Story 10-2-redis-接入)
story: 10-2-redis-接入
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-06 — go-redis ContextTimeoutEnabled 默认 false 让 ctx 形同虚设 + 抽象层 Close 真·幂等需要 sync.Once 兜底

## 背景

Story 10-2 的 review r2 由 codex 扫出两条问题，都集中在 `server/internal/infra/redis/redis.go`。
共同主题：**抽象层接口的契约承诺与底层第三方驱动的默认行为存在不可见的偏差**。仅靠"接口注释 + 直接 wrap 驱动 API"
满足不了承诺，必须在抽象层主动校准 Options / 加 sync.Once。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | go-redis/v9 默认忽略 command ctx，redisOpenTimeout 与 ADR-0007 ctx 传播全部失效 | high (P1) | config | fix | `server/internal/infra/redis/redis.go:34-39` |
| 2 | `redisClient.Close()` 直接 wrap 底层 Close，二次调用返 spurious "client is closed" | low (P3) | architecture | fix | `server/internal/infra/redis/redis.go:143-148` |

## Lesson 1: go-redis/v9 ContextTimeoutEnabled 默认 false → 命令 ctx 全部失效

- **Severity**: high (P1)
- **Category**: config
- **分诊**: fix
- **位置**: `server/internal/infra/redis/redis.go:34-39`

### 症状（Symptom）

`redisinfra.Open` 把 `redisOpenTimeout = 5s` 的 ctx-with-timeout 传给 `rc.Ping(ctx)`，
意图碰到 blackhole host 时启动期 5s 内 fail-fast。RedisClient 接口所有方法的第一参数也是 ctx，
意图让 handler 取消 / request deadline 能传到底层命令。

实际 codex 看 v9.7.0 源码发现：`baseClient.context()` 在 `Options.ContextTimeoutEnabled == false`
（默认值）时直接返回 `context.Background()`，所有命令路径**完全绕过**传入 ctx 的 deadline / cancel
检查。后果：
- `redisOpenTimeout` 对 Ping 不生效，fail-fast 退化到驱动 socket-level timeout
- 业务 handler 收到 client cancel / request timeout 后，下游 Redis 命令仍裸跑到底，违反 ADR-0007

### 根因（Root cause）

go-redis 的"关闭式"opt-in 设计与 Go 生态主流 ctx-aware library 反直觉：

1. **库默认 = 历史遗留兼容**：v8 以前 ctx 不传播是事实，v9 引入 ctx 支持但**怕 break existing users**，把 `ContextTimeoutEnabled` 设成 false 默认。这种"默认值与文档承诺的语义反方向"在 Go 生态算少见。
2. **接口签名误导**：`func (c *Client) Get(ctx context.Context, key string) *StringCmd` 看起来像 ctx-aware，但实际是 schema-only 的 ctx，不进入命令路径。如果不读源码或精读 Options 文档，单看签名会 **assume ctx 生效**。
3. **抽象层注释承诺脱离驱动配置**：本接口 `client.go` 行 37-39 注释明确说"所有命令必须 ctx-aware：传入的 ctx 通过 redis.Conn / WithContext 传递给底层 driver；ctx 取消时命令必须中断"，但实装层只 `goredis.NewClient(&Options{...})` 没设 `ContextTimeoutEnabled`，把承诺甩给了驱动默认值（默认值不满足承诺）。
4. **测试盲点**：原 `TestRedisClient_CtxCancel_CommandAborts` 用 `t.Logf` 记录而非 `t.Errorf` 断言 `errors.Is(err, context.Canceled)`，让"ctx 形同虚设"这种状态在 ContextTimeoutEnabled 漏配时仍能跑过。注释里写"主要验证'是 error 且能识别为取消'，不严卡具体类型" —— 这种宽松断言放过了实质性 regression。

### 修复（Fix）

`redis.go` Open 里 `goredis.NewClient(&Options{...})` 显式加 `ContextTimeoutEnabled: true`：

```go
rc := goredis.NewClient(&goredis.Options{
    Addr:                  cfg.Addr,
    Password:              cfg.Password,
    DB:                    cfg.DB,
    PoolSize:              cfg.PoolSize,
    ContextTimeoutEnabled: true, // 不开则 ctx cancel/deadline 全部失效
})
```

测试加强（`redis_test.go`）：
- `TestRedisClient_CtxCancel_CommandAborts`: `t.Logf` 改 `t.Errorf`，严格断言 `errors.Is(err, context.Canceled)`
- 新增 `TestRedisClient_CtxDeadline_CommandAborts`: 用过期 deadline 的 ctx，断言 `errors.Is(err, context.DeadlineExceeded)`，专门锚 deadline 路径生效

两个 test 在 `ContextTimeoutEnabled` 被回退时会立刻挂掉（不会等 socket-level timeout）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**包装第三方 ctx-aware library 给业务用**时，**必须**逐项核对 driver 默认 Options 是否满足抽象层承诺的"ctx 必生效"语义；不能因签名带 ctx 参数就 assume 它生效。
>
> **展开**：
> - 引入新驱动时，**先读 driver 的 Options struct** 全部字段，特别是名字带 `Enabled` / `Disabled` / `Mode` 的开关 —— 这些是默认行为的反方向 opt-in/opt-out。
> - go-redis/v9 specifically：`Options.ContextTimeoutEnabled = true` 必须显式开启，否则 ctx 完全无效。
> - 抽象层接口注释承诺的语义（如"ctx 取消必须中断命令"）**必须**在实装层有对应的 Options 配置 + 测试断言；纯靠注释不构成契约。
> - 测试 ctx 传播的契约时，**严格用 `t.Errorf + errors.Is`，不用 `t.Logf` 宽松记录**。"宽松接受任何 error"这种断言放过 regression。
> - **反例**：`goredis.NewClient(&Options{Addr: "..."})` 未设 `ContextTimeoutEnabled` + 测试只断"err != nil 且 t.Logf 期望 errors.Is" → 命令 ctx 完全失效，但 happy path 测试全绿。这种"假绿"是抽象层对底层默认值放任的典型形态。

## Lesson 2: 抽象层 Close 幂等承诺需要 sync.Once 兜底，不能直接 wrap 底层 Close

- **Severity**: low (P3)
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/infra/redis/redis.go:143-148`

### 症状（Symptom）

`RedisClient.Close()` 接口注释承诺"必须幂等（多次调用不报错）—— 与 *sql.DB.Close() 行为一致"，
但实装直接 `return c.client.Close()`。go-redis baseClient.Close 第二次调用会返 `redis: client is closed`
（v9.7.0 `redis.go` baseClient.Close 不挡重复关闭）。

实际影响场景：
- main.go `defer redisClient.Close()` + 业务路径自己也调过一次 Close → spurious shutdown error log
- 测试用 `t.Cleanup(rc.Close)` 注册 + 业务测试自己手动调 `rc.Close()` → 第二次 cleanup 拿到 error

### 根因（Root cause）

1. **类比错误**：`*sql.DB.Close()` 内部已经用 `sync.Once` 包了，所以多次调用返 nil。本实装把"和 *sql.DB.Close() 行为一致"承诺照搬，但只 wrap 了 go-redis 底层（go-redis 没做幂等）—— 同一句承诺在两个驱动需要不同的实装手段。
2. **测试断言过宽**：原 `TestRedisClient_Close_Idempotent` 注释明确写"go-redis Client.Close 第二次调用确实可能返 'redis: client is closed' 类错误；接受任何 error 但**不**接受 panic" —— 把"幂等"降级成"不 panic"，让接口承诺与测试断言不一致。这种"宽松测试 + 严格接口承诺"的不对称让 bug 漏到 review。

### 修复（Fix）

`redisClient` struct 加 `closeOnce sync.Once` + `closeErr error` 字段，Close 用 sync.Once 短路：

```go
type redisClient struct {
    client    *goredis.Client
    closeOnce sync.Once
    closeErr  error
}

func (c *redisClient) Close() error {
    c.closeOnce.Do(func() {
        c.closeErr = c.client.Close()
    })
    return c.closeErr
}
```

测试加强：`TestRedisClient_Close_Idempotent` 把"接受任何 error" 收紧成 `t.Errorf("second Close returned %v, want nil")`，与接口承诺对齐。

`client.go` 接口注释也加注：注明 go-redis 自身不幂等、本层用 sync.Once 补齐契约。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**抽象层接口承诺"Close 幂等"** 时，**必须**用 sync.Once 在抽象层主动包装，不能直接 forward 底层驱动的 Close —— 不同驱动的 Close 默认行为不同，类比 `*sql.DB.Close()` 不构成实装根据。
>
> **展开**：
> - "Close 幂等"是接口契约，不是底层驱动属性。哪怕底层驱动**碰巧**幂等（如 `*sql.DB`），抽象层的 sync.Once 包裹也是 cheap 防御 —— driver 升级后默认行为反转的可能性永远存在。
> - sync.Once + closeErr 字段的标准 pattern：第一次真关 + 记 err；后续调用返第一次的 err（通常 nil）。一行代码成本，零风险。
> - 测试断言必须与接口承诺**严格对齐**。承诺"幂等" → 测试断言"第二次返 nil"；承诺"不 panic" → 测试断言"不 panic"。**不允许测试比接口注释宽松**（宽松测试是 contract drift 的最常见入口）。
> - **反例**：抽象层注释写"Close 必须幂等"，实装写 `return c.client.Close()`，测试断言"不 panic 即可" → 接口 / 实装 / 测试三者契约层级不一致；happy path 全绿但承诺不成立。

## Meta: 本次 review 的宏观教训

两条 finding 共同指向同一个反模式：**抽象层接口注释写下了承诺，但实装层既没在 driver Options 里配置兑现，
也没在测试断言里验证兑现**。注释独自承担契约重量是不行的 —— 注释不会被编译器 / 测试 runner 检查。

行动启示：
- 写完接口的 godoc，**立刻**问自己三个问题：
  1. 这个承诺需要 driver Options 配置吗？（如 ContextTimeoutEnabled）
  2. 这个承诺需要抽象层主动包装吗？（如 sync.Once for idempotent Close）
  3. 这个承诺有没有对应的严格测试断言？（不是 t.Logf 宽松记录）
- 任意一个回答 "no / 没明确做" → 该承诺还没真正落地，必须补齐再交付。

这是把"承诺 → 配置 → 实装 → 测试"四层一致性贯穿到位的最小可执行规则。
