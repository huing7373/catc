---
date: 2026-04-26
source_review: codex review on Story 4.4 token-util (round 1) — /tmp/epic-loop-review-4-4-r1.md
story: 4-4-token-util
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-26 — JWT util 校验必填 claim + 所有 sign 路径必须 enforce 配置约束

## 背景

Story 4.4（`server/internal/pkg/auth/token.go`）首版实装了 HS256 JWT 签发 / 校验工具。codex round 1 review 指出两个真问题：

1. Verify 漏校验 user_id 必填字段 → 攻击者签 minimal claims 即可拿到 UserID=0 的"已认证"上下文
2. New 校验了 maxExpireSec（30 天）但 Sign 没校验 → 调用方可绕过策略 mint 任意长 token

两条都属于"**安全约束在创建路径生效但在使用路径漏掉**"的同一类思维漏洞，合并归档。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Verify 必须拒绝缺 user_id 的 token | medium (P2) | security | fix | `server/internal/pkg/auth/token.go:189-196` |
| 2 | Sign 必须 enforce maxExpireSec 上限 | low (P3) | security | fix | `server/internal/pkg/auth/token.go:133-142` |

## Lesson 1: Verify 必须拒绝缺 user_id 的 token（pointer + nil-check 模式）

- **Severity**: medium (P2)
- **Category**: security
- **分诊**: fix
- **位置**: `server/internal/pkg/auth/token.go:85-88, 189-196`

### 症状（Symptom）

`Verify` 解析后只检查 `tc.IssuedAt == nil || tc.ExpiresAt == nil`，没检查 `user_id`。`tokenClaims.UserID` 用 `uint64` 类型 → JSON unmarshal 时若字段缺失，得到 zero value `0` —— Verify 通过 → 返回 `Claims{UserID: 0}`。调用方信任 `claims.UserID` 把 user 0 当成已认证用户处理。

攻击场景：拿到 secret 或者绕过签名（其他漏洞）的攻击者，构造 claims = `{iat, exp}`（**漏 user_id**）的 HS256 token。本来应当被 Verify 拒绝，但 zero value 静默放行。

### 根因（Root cause）

Go JSON unmarshal 对**值类型**（`uint64`、`int64`、`string` 等）不区分"字段缺失"与"字段显式为 zero value"。如果 schema 语义要求"必填"，用值类型反序列化是天然漏洞 —— 攻击者可以构造缺字段的 payload，让你的代码无法察觉。

二级根因：开发时只防御了"我们 Sign 时一定填了"的场景（`tc.IssuedAt` / `tc.ExpiresAt` 来自 jwt-v5 的 `*jwt.NumericDate`，本身就是 pointer，缺字段时为 nil），**但自定义字段 `user_id` 用了值类型**，对称性破了，思维盲点跟着破。

### 修复（Fix）

`tokenClaims.UserID` 改成 `*uint64`（pointer，加 `omitempty` tag）；Sign 时取局部变量地址写入；Verify 增加 `tc.UserID == nil` 分支返 `ErrTokenInvalid`；返回 Claims 时 `*tc.UserID` 解引用。

```go
// Before
type tokenClaims struct {
    UserID uint64 `json:"user_id"`
    jwt.RegisteredClaims
}

// After
type tokenClaims struct {
    UserID *uint64 `json:"user_id,omitempty"`
    jwt.RegisteredClaims
}

// Verify 内
if tc.UserID == nil || tc.IssuedAt == nil || tc.ExpiresAt == nil {
    return Claims{}, ErrTokenInvalid
}
```

测试：用 `jwt.MapClaims{"iat":..,"exp":..}`（漏 user_id）签 token，`Verify` 必须返 `ErrTokenInvalid`。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **JSON / JWT / protobuf 反序列化"必填"字段** 时，**必须**用 **pointer 类型 + nil 检查**（或显式的 `_present bool` 字段），**禁止**直接用值类型依赖 zero value 区分"缺失"与"零值"。
>
> **展开**：
> - 凡是 schema 上标记"必填"的字段，反序列化目标必须是 pointer / `sql.Null*` / 自定义 `optional` 包装类型；解析后第一件事检查 nil → 返业务 error
> - 关键判断："这个字段的合法 zero value（0 / "" / false）能不能在业务上被攻击者用来骗过校验？"如果**能**，必须 pointer
> - **反例 1**：`type User struct { ID uint64 }` 反序列化后用 `if u.ID == 0 { reject }` —— 这也算硬撑过去，但语义混乱（ID=0 是"缺字段"还是"业务零值"？）；遇到 `user_id` 这种业务上 0 也合法的字段（数据库设计 §3.1 BIGINT UNSIGNED 边界值合法）就完全瘫
> - **反例 2**：用 `json.RawMessage` + 后处理判断字段存在 —— 比 pointer 复杂、易错，除非确实需要保留原始字节
> - **反例 3**：依赖 jwt 库的 `RequiredClaims` validator 但不 explicit assert —— jwt-v5 的 `RegisteredClaims` 字段（exp/iat/nbf）默认 pointer 时已有这语义，**但自定义 claims 字段不在此列**，必须自己补
> - 补 unit test 覆盖"字段缺失"与"字段显式为 zero value"两条路径独立行为

## Lesson 2: Sign 必须 enforce maxExpireSec —— 创建路径与使用路径校验对称

- **Severity**: low (P3)
- **Category**: security
- **分诊**: fix
- **位置**: `server/internal/pkg/auth/token.go:108-125, 133-151`

### 症状（Symptom）

`New(secret, defaultExpireSec)` 校验 `defaultExpireSec > 30 days` 拒绝；但 `Sign(userID, expireSec)` 直接用 `expireSec` 不校验。调用方可签任意长 token（`Sign(uid, 365*86400)` 一年期），违反"严格 cap 30 天"的 invariant。

### 根因（Root cause）

策略约束（"token 寿命 ≤ 30 天"）在 `New` 里以"`defaultExpireSec` ≤ maxExpireSec"的形式表达 —— 这只覆盖**默认路径**，没覆盖**显式覆盖路径**（`Sign(uid, customExpireSec)`）。

思维漏洞：把"约束"误等同于"配置约束"。但配置约束只在配置加载时生效，运行时 API 暴露的参数同样可能突破约束 —— **所有进入约束适用域的代码路径都必须重新 enforce 一次约束**。

类比：HTTP middleware 里检查 `Authorization: Bearer ...`，但 WebSocket upgrade 路径漏检 → 同样问题。

### 修复（Fix）

`Sign` 方法体加一句：

```go
if expireSec > maxExpireSec {
    return "", fmt.Errorf("auth: expireSec %d exceeds max %d (30 days)", expireSec, maxExpireSec)
}
```

`maxExpireSec` 已经是 package const，让 `New` / `Sign` 共享，避免再 define const。注意校验顺序在 `expireSec <= 0 → fallback` 之后，确保 fallback 路径走 `defaultExpireSec`（已被 New 校验过）安全。

测试：`Sign(1, 31*86400)` 必须返 error；`Sign(1, 30*86400)` 边界值必须接受；`Sign(1, 0)` 走 default fallback 仍 OK。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **某个常量 / 上限 / 范围在"工厂函数 / constructor"里 enforce** 时，**必须**审计所有"使用方法"（mutator / setter / 显式覆盖参数）是否同样 enforce；**禁止**只在 constructor 校验然后假设"运行时不会再破坏 invariant"。
>
> **展开**：
> - 写 `New(...)` / `NewXxx(...)` 加任何 validation 时，立即列出该 struct 的所有 export 方法，逐个核对：是否有方法接受参数能突破 New 当时建立的 invariant？有则必须在该方法 enforce
> - 把约束常量提到 package level（如 `const maxExpireSec = ...`），让 New 和 Sign 共用，避免双源真相
> - **反例 1**：`func (c *Config) SetTimeout(d time.Duration)` 不校验 `d > 0`，但 `NewConfig` 里校验 —— 攻击面：caller `cfg.SetTimeout(-1)` 后 cfg 状态破坏
> - **反例 2**：把约束放在 wrapper（如 `LoginHandler` 调 Sign 时自己校验 expireSec），让 wrapper 成为安全网 —— **错**，util 包必须 self-contained 防御；调用方多了一个就少一个 enforce 点
> - **反例 3**：信任注释 / doc 里写"调用方应保证 X" —— 注释不是 enforcement，dumb util 包的"业务校验"边界（如 userID != 0）和"安全策略"边界（如 expireSec ≤ 30d）不一样，**安全策略必须代码 enforce，不能口头约定**
> - 补 unit test 同时覆盖：constructor 边界 + 该 invariant 在每个使用方法的边界

---

## Meta: 本次 review 的宏观教训

两条 finding 共享同一个思维漏洞 —— **"约束 enforcement 的覆盖面"**：

- Lesson 1：schema 必填语义 → 在反序列化路径 enforce（pointer + nil check）
- Lesson 2：策略上限 → 在所有签发路径 enforce（不只 constructor）

合起来的元规则：**任何"约束"（constraint）都不能只在一个入口生效；必须列举该约束的所有触发路径并逐个加 enforcement。** 写 util 包 / library 时尤其要 paranoid —— 你不知道未来谁怎么调用，唯一可靠的策略是"约束在 util 内部封死"。

适用面广：JWT util / auth middleware / config validator / SQL builder 的参数校验 / 序列化 schema / API rate limiter ……
