---
date: 2026-04-26
source_review: codex review round 2 — /tmp/epic-loop-review-4-5-r2.md
story: 4-5-auth-rate_limit-中间件
commit: b67cf45
lesson_count: 1
---

# Review Lessons — 2026-04-26 — YAML 配置默认值不能掩盖显式无效值（用 *int64 区分 nil 与 explicit 0）

## 背景

Story 4.5（auth 限频中间件）round 2 codex review 拦下 1 条 [P2]：`config/loader.go`
对 `cfg.RateLimit.PerKeyPerMin == 0` 兜底默认 60。当用户在 YAML 显式写
`per_key_per_min: 0`（无论是想禁限频、还是配置漂移、还是拼写错），loader 会**静默
替换**成默认 60 → middleware 看不到 0 → 启动正常但限频策略不符预期。

代码层面的根因是 Go 零值不可见：`int` 字段无法区分"YAML 没写这个 key"和"YAML 写了
`: 0`"。两种语义被零值合并 → loader 用 `== 0 ? default : keep` 兜底时把"显式无效值"
当成"未提供"处理，违反 fail-fast 契约。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | YAML 显式 `per_key_per_min: 0` 被 loader 静默替换为 60，绕过 fail-fast 契约 | medium [P2] | config | fix | `server/internal/infra/config/loader.go`, `config.go`, `loader_test.go`, `app/http/middleware/rate_limit.go`, `rate_limit_internal_test.go`, `rate_limit_test.go`, `error_mapping_integration_test.go`, `testdata/ratelimit_zero.yaml` |

## Lesson 1: YAML 配置字段的"未提供"与"显式无效值"必须可区分（用 *T pointer，而非靠零值兜底）

- **Severity**: medium [P2]
- **Category**: config
- **分诊**: fix
- **位置**: `server/internal/infra/config/loader.go:83-91`（旧实装）

### 症状（Symptom）

`config.RateLimitConfig.PerKeyPerMin` 是 `int`。loader 的 `if cfg.RateLimit.PerKeyPerMin == 0 { cfg.RateLimit.PerKeyPerMin = 60 }`
让两种语义无法区分：

- **YAML 未写** `per_key_per_min` key（用户期望走默认 60）
- **YAML 显式写** `per_key_per_min: 0`（用户期望禁限频 / 拼写错 / 配置漂移）

两种情况都进同一分支 → 都被替换成 60。middleware 工厂的 `cfg.PerKeyPerMin <= 0 → panic`
fail-fast 路径**永远走不到**显式 0 这条 → 启动表面成功但策略错配。

### 根因（Root cause）

Go 的零值机制让 struct 字段在"未赋值"和"赋零值"两种状态下不可区分。这是 Go 类型系统
的固有特性 —— 用 `int` 字段时，YAML decoder 默认保持零值，loader 拿到 `0` 没法回答
"这个 0 是用户显式写的还是 YAML decoder 留的零值？"

当业务语义里 `0` **是合法可解释的输入**（限频/配额/超时这类数值字段都属于此类），就**必须
让"未提供"和"提供 0"两种语义可区分**。否则任何"== 0 ? default : keep" 兜底都会把
"显式 0" 当成"未提供"，破坏 fail-fast。

类似坑：YAML/JSON decoder 对 bool / string / 数值都有零值合并问题。`omitempty` 在序列化
方向有解，反序列化方向必须靠 pointer / 自定义 Unmarshaler / 三态枚举。

### 修复（Fix）

把 `RateLimitConfig` 三个数值字段从 `int` 改成 `*int64`：

**Before**（静默替换显式 0）:
```go
// config.go
type RateLimitConfig struct {
    PerKeyPerMin int `yaml:"per_key_per_min"`
    BurstSize    int `yaml:"burst_size"`
    BucketsLimit int `yaml:"buckets_limit"`
}

// loader.go
if cfg.RateLimit.PerKeyPerMin == 0 {
    cfg.RateLimit.PerKeyPerMin = 60  // ← YAML 显式 0 也走这里 ❌
}
```

**After**（pointer 区分 nil / *0）:
```go
// config.go
type RateLimitConfig struct {
    PerKeyPerMin *int64 `yaml:"per_key_per_min"`  // nil = YAML omitted; &0 = YAML explicit 0
    BurstSize    *int64 `yaml:"burst_size"`
    BucketsLimit *int64 `yaml:"buckets_limit"`
}

// loader.go
if cfg.RateLimit.PerKeyPerMin == nil {
    v := int64(60)
    cfg.RateLimit.PerKeyPerMin = &v  // 仅 nil 才填默认；显式 0 透传 ✅
}

// middleware/rate_limit.go (deref 后走原 fail-fast)
perKeyPerMin := *cfg.PerKeyPerMin
if perKeyPerMin <= 0 {
    panic(fmt.Sprintf("PerKeyPerMin must be > 0, got %d", perKeyPerMin))
}
```

测试覆盖（关键）：
- `TestLoad_RateLimitExplicitZero_PreservedNotDefaulted` —— YAML `per_key_per_min: 0`
  → loader 必须保留 `*int=0`，不替换默认
- `TestLoad_RateLimitOmitted_DefaultedTo60` —— YAML 无该 key → loader 填默认 60
- 既有 `TestRateLimit_PanicsOnInvalidPerKeyPerMin` —— `*0` 透传到 middleware 触发 panic

顺带改动（pointer 类型迁移波及面）：
- `rate_limit_internal_test.go` / `rate_limit_test.go` / `error_mapping_integration_test.go`
  的 struct literal 全部改用 `ptrInt64()` helper 构造；
- `error_mapping_integration_test.go` 由于不在 `middleware` 包，重复一份 helper（因为
  `middleware_test` 包用 `ptrInt64`，`middleware` 内部包测试用同名 helper）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在为 **YAML/JSON 配置定义 struct 字段** 且字段 **业务语义里
> 0/""/false 是合法可解释的输入** 时，**必须**用 `*T` pointer（或自定义 Unmarshaler）
> 区分"YAML 未提供"和"YAML 显式提供零值"，**禁止**用 `if field == zero { field = default }`
> 兜底 —— 那会让"显式无效值"被静默替换成默认，破坏 fail-fast。
>
> **展开**：
> - **判定该用 pointer 的标志**：字段语义里 `0` / `""` / `false` 是用户**可能显式想表达
>   的合法值**（如限频阈值、超时秒数、重试次数、feature flag、空白模板字符串）。
>   反例：字段是 enum 字符串（"info" / "debug"），零值 `""` 永远是"未提供"，可以安全
>   用 `if v == "" { v = default }`。
> - **loader 兜底契约**：仅 `nil` 走默认；非 nil（含 *0 / *负数）原样透传 → 由下游
>   工厂 / 校验函数走 fail-fast。**不要在 loader 里替显式无效值"补救"**。
> - **middleware / service 工厂的 fail-fast 必须用 deref 后比较**：`*cfg.X <= 0 → panic`
>   而不是 `cfg.X == nil → default`。前者保护 fail-fast；后者重蹈零值兜底覆辙。
> - **所有 caller 改 pointer 后**：grep `RateLimit.PerKeyPerMin` / `cfg.PerKeyPerMin`
>   sweep 全仓库，每个 struct literal 都改 `ptrInt64(v)` 或显式 `&v`，每个 deref 处加
>   nil-check（caller bug → panic with clear message）。
> - **新加测试覆盖二义性边界**：
>   - YAML 显式写 0 / 空字符串 / false → 字段保留显式值（不被默认覆盖）
>   - YAML 完全不写该 key → 字段填默认值
>   两个测试同时存在才能锁死语义。
> - **反例**（绝不可写）：
>   ```go
>   // ❌ 这会把 YAML 显式 0 替换成默认 60
>   if cfg.RateLimit.PerKeyPerMin == 0 {
>       cfg.RateLimit.PerKeyPerMin = 60
>   }
>   ```
>   ```go
>   // ❌ 这看似"修了"但实际没区分 nil / &0
>   if cfg.X == nil || *cfg.X == 0 {
>       v := defaultX
>       cfg.X = &v
>   }
>   ```
>   ```go
>   // ✅ 正确：仅 nil 填默认，&0 透传到 fail-fast
>   if cfg.X == nil {
>       v := defaultX
>       cfg.X = &v
>   }
>   ```

---

## Meta: 本次 review 的宏观教训

本条 [P2] 与同 epic 的 `2026-04-26-checked-in-config-must-boot-default.md` /
`2026-04-26-checked-in-secret-must-fail-fast.md` 同属 **"配置默认值边界"** 主题群：

- 4.4 review：`secret` 字段空字符串必须 fail-fast，不允许 dev fallback
- 4.5 review (本条)：`per_key_per_min` 显式 0 必须 fail-fast，不允许 loader 静默兜底

共同 anti-pattern：**loader / 工厂"贴心地"替不合规配置补救默认值**。
共同正确路径：**fail-fast 契约必须能看到用户的真实输入**，loader 只补"未提供"，绝不
碰"提供了无效值"。

未来 Claude 写任何 config 兜底代码时先问一句：「这个字段的零值是不是合法的用户输入？」
回答"是" → 必须用 pointer / option type；回答"否" → 简单零值兜底 OK。
