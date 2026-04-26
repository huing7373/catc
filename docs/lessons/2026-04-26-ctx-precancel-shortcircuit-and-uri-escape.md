---
date: 2026-04-26
source_review: codex review round 4 on Story 4.3 (五张表 migrations)
story: 4-3-五张表-migrations
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-26 — ctx-aware 包装必须 short-circuit pre-canceled ctx & 文件路径转 URI 必须 escape 元字符

## 背景

Story 4.3 引入 `internal/infra/migrate` 包封装 golang-migrate v4，让 CLI / 测试都能调 `Up/Down/Status`。前几轮已修过 file URI Windows 兼容性（round 2）、CLI lazy config（round 3）、GracefulStop wait-for-done（round 3）。本轮（round 4）codex 又挑出 2 条：

1. `runWithCtxAndTimeout` 没在启动 goroutine 之前 short-circuit pre-canceled ctx → caller 显式 cancel 后再调 Up，DDL 仍会发出去
2. `pathToFileURI` 用 raw string concat 拼 URI，路径含 `#` / `?` / 空格时被 net/url 当 fragment / query → migrate 失败

两条都是 ctx-aware 包装 / URI 拼接的通用陷阱，本 lesson 沉淀两条规则供未来 Claude 写同类工具时直接套用。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | runWithCtxAndTimeout 不 short-circuit pre-canceled ctx | high (P1) | error-handling | fix | `server/internal/infra/migrate/migrate.go:201-204` |
| 2 | pathToFileURI 用 string concat 不 escape URI 元字符 | medium (P2) | error-handling | fix | `server/internal/infra/migrate/migrate.go:157-161` |

## Lesson 1: ctx-aware 包装必须先 short-circuit pre-canceled ctx 再启动 goroutine

- **Severity**: high (P1)
- **Category**: error-handling
- **分诊**: fix
- **位置**: `server/internal/infra/migrate/migrate.go:201-204`

### 症状（Symptom）

`runWithCtxAndTimeout` 之前实装：

```go
func runWithCtxAndTimeout(ctx context.Context, stop stopSender, fn func() error, graceTimeout time.Duration) error {
    done := make(chan error, 1)
    go func() { done <- fn() }()  // ← 先启动 goroutine fn()
    select {
    case err := <-done: ...
    case <-ctx.Done():
        stop.sendGracefulStop()
        ...
    }
}
```

如果 caller 传入的 ctx 已经 cancel（典型场景：CLI 已经接到 SIGINT，handler 又被调到 Up），go func 立刻启动 → fn() 把 SQL 发给 MySQL 开始改 schema → 才走 select → ctx.Done 命中 → 才 sendGracefulStop。已发出去的 DDL 已不可逆（server-side 一旦开始执行，schema 已 mutate），即使 GracefulStop 在 next-statement boundary 停下，前面的 ALTER 已经生效。

caller 显式 cancel 但 schema 仍被改 → 违反 ctx-aware API 语义。

### 根因（Root cause）

ctx-aware wrapper 的常见误解："反正 select 里有 case <-ctx.Done()，cancel 总能被检测到"。但 select 在 goroutine 启动**之后**才执行 —— Go runtime 不保证 select 哪个 case 先 ready，但这不重要，因为 fn() 里的 SQL 发送是**同步且不可逆**的。一旦 goroutine 进入 fn()，select 已经太晚。

类似陷阱：HTTP client、数据库 driver、文件 IO wrapper —— 任何"先启动 IO 再 select ctx"的模式都有这个问题，区别只是 IO 是否可逆。可逆的 IO（如 TCP connect 还没建立）勉强能接受；不可逆的 IO（如 DDL、外部 API 副作用）必须 fail-fast。

### 修复（Fix）

在启动 goroutine 之前先检查 `ctx.Err()`：

```go
func runWithCtxAndTimeout(ctx context.Context, stop stopSender, fn func() error, graceTimeout time.Duration) error {
    // 早 cancel short-circuit：caller 已 cancel ctx 时不调 fn、不碰 DB
    if err := ctx.Err(); err != nil {
        return err
    }
    done := make(chan error, 1)
    go func() { done <- fn() }()
    select { ... }
}
```

加单测覆盖：传 pre-canceled ctx → 立刻返回 ctx.Err()，fn 计数器为 0，stop sender 也不应被调（goroutine 都没起）。同样验证 pre-expired DeadlineExceeded 路径。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写"用 goroutine + select ctx.Done() 包装同步 IO 让其 ctx-aware"的 wrapper 时，**必须**在启动 goroutine 之前**先检查 `ctx.Err()`**，pre-canceled / pre-expired ctx 直接返回 `ctx.Err()`，绝不让 IO 被启动。
>
> **展开**：
> - "先启动 IO 再 select" 的模式只对**可逆**或**幂等**的 IO 安全；任何会改 server-side state（DDL、外部 API write、写文件）的 IO 都必须前置 ctx 检查
> - 检查的位置在 `done := make(chan error, 1)` **之前**（连 channel 都不要分配）
> - 单测必须显式覆盖：`ctx, cancel := context.WithCancel(...); cancel(); 然后调 wrapper` —— 验证 fn 调用计数为 0
> - 同时覆盖 DeadlineExceeded 路径：`context.WithDeadline(..., time.Now().Add(-1*time.Second))`
> - **反例**：以为 select 会"自动检测 ctx cancel"就把 fn() 放在 goroutine 里启动；以为 GracefulStop 信号能救回已发出的 SQL（救不回，server-side execution 已开始）；只测 fn-runs-then-cancel 不测 pre-cancel
> - **类似场景**：HTTP server bootstrap.Run（`net.Listen` 之前应检查 ctx）、文件写 wrapper（os.Create 之前应检查 ctx）、外部 API 调用 wrapper（client.Do 之前应检查 ctx）—— 同一规则适用

## Lesson 2: 文件路径转 URI 必须按 path segment 逐段 url.PathEscape，不能 raw string concat

- **Severity**: medium (P2)
- **Category**: error-handling
- **分诊**: fix
- **位置**: `server/internal/infra/migrate/migrate.go:157-161`

### 症状（Symptom）

之前实装：

```go
func pathToFileURI(p string) (string, error) {
    abs, err := filepath.Abs(p)
    if err != nil { return "", err }
    slashed := filepath.ToSlash(abs)
    return "file://" + slashed, nil  // ← raw concat
}
```

路径含 URI 元字符时 net/url.Parse 会误判：

- `C:\work\repo#1\migrations` → ToSlash → `C:/work/repo#1/migrations` → 拼成 `file://C:/work/repo#1/migrations`
- net/url.Parse 把 `#1/migrations` 当 fragment → Host="C:" Path="/work/repo" Fragment="1/migrations"
- golang-migrate file source 取 `Host + Path` = `C:/work/repo` → os.DirFS 找不到目录
- 同理 `?` 当 query；空格违反 URI 语法

实际触发：用户 checkout 在 `C:\work\repo#1\...` 路径，或 `CAT_MIGRATIONS_PATH` 含特殊字符 → migrate 失败但错误信息晦涩（"file does not exist" 但路径明明存在）。

### 根因（Root cause）

URI 不是"路径 + scheme prefix"那么简单 —— RFC 3986 给 `#` `?` `/` `[` `]` `@` 等 reserved characters 赋了语法地位。文件系统对这些字符没有限制（POSIX 只禁 `/` 和 NUL；Windows 也允许 `#` `?`），所以**任何**未经 escape 的文件路径转 URI 都可能撞到。

raw concat 的另一个常见形态："只要 ASCII 就没事" —— 错。`#` / `?` 都是 ASCII 但都是 URI reserved。

### 修复（Fix）

按 `/` 拆段后逐段 `url.PathEscape`，再用 `/` 拼回。这样 `/` 保留为路径分段符，段内的 `#` / `?` / 空格被 percent-encoded：

```go
import "net/url"

func pathToFileURI(p string) (string, error) {
    abs, err := filepath.Abs(p)
    if err != nil { return "", fmt.Errorf("filepath.Abs(%q): %w", p, err) }
    slashed := filepath.ToSlash(abs)
    // 按 / 拆段后逐段 escape（保留分段符 / 不转义，仅转义段内的 URI 元字符）
    parts := strings.Split(slashed, "/")
    for i, part := range parts {
        parts[i] = url.PathEscape(part)
    }
    escaped := strings.Join(parts, "/")
    return "file://" + escaped, nil
}
```

注意：**不**直接用 `url.URL{Scheme:"file", Path: slashed}.String()` —— 因为 Windows 路径 drive 字母含 `:`，`(*url.URL).String()` 在 `Path` 含 `:` 时会触发 opaque-form 检测，输出形态不稳定（v4 source/file driver 也不接受）。手工逐段 escape + 保留 `file://X:/...` 双斜杠形态是最稳的。

加单测覆盖：路径含 `#` / `?` / 空格 / 多元字符组合 → escape 后 net/url.Parse 还原回原 path（u.Host + u.Path），且 u.Fragment / u.RawQuery 必须为空。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 把任意"文件系统路径"转成"`scheme://...` URI"时，**必须**按 `/` 拆段、每段 `url.PathEscape`、再 `/` 拼回，**禁止** raw string concat。
>
> **展开**：
> - 凡输出会被某层 `net/url.Parse` 解析的 URI（包括传给 golang-migrate / sqlx / 任何 driver），都必须 escape
> - `url.PathEscape` 不 escape `:` 和 `/`，正合用：`/` 保留为分段符，`:` 保留供 Windows drive letter 当 Host
> - **不要**用 `(&url.URL{Scheme:..., Path:...}).String()` 处理含 `:` 的 Windows 路径 —— Go runtime 会触发 opaque form 输出，driver 解析行为不稳定
> - 单测的 escape case 必须覆盖：`#`、`?`、空格、组合（`a#b/c?d`），并 round-trip 验证 net/url.Parse 还原回原 path
> - **反例**：`return scheme + "://" + filepath.ToSlash(abs)` 这种一行 concat；以为"只要不是 URL 直接出现的字符就安全"（`#` / `?` 是 ASCII 但是 URI reserved）；用 `url.URL{}.String()` 处理 Windows drive 路径
> - **类似陷阱**：把 dsn 拼到 redis URL（密码含 `@` / `#`）、把 git URL 拼参数（path 含空格）、构造 OAuth callback URL（state 含 `+`）—— 都是同一类问题

## Meta: 本次 review 的宏观教训（可选）

两条 finding 都是"看似工作的快路径"陷阱：

- Finding 1 是"goroutine + select 看起来 ctx-aware 实际不是" —— ctx-aware wrapper 的正确语义包含 pre-cancel short-circuit
- Finding 2 是"`filepath.ToSlash` + concat 看起来跨平台实际不跨 URI" —— path 转 URI 必经 escape 步骤

共同主题：**为不可控输入做防御**。ctx 来自 caller、文件路径来自用户 / 配置 / checkout 位置 —— 凡是来自外部、且后续会触发副作用 / 解析的输入，都要在第一道关卡做完整防御，不要假设 caller 不会传 pre-canceled ctx，也不要假设用户不会在含 `#` 的目录下 checkout。

另一个 meta 教训：sweep 隔离工作。本轮检查了 server/ 全局是否有别的"go func + select ctx" 和 "string concat URI" 模式：

- bootstrap.Run 也有 "先 net.Listen 再 select ctx" 的模式，但 bind 失败本身有 fail-fast，且 listener 是可关闭的可逆资源（不像 DDL 不可逆），暂不修
- 其它 URI 拼接：只有 `"mysql://" + dsn`（migrate.New 内部），dsn 是 opaque 字符串、按 golang-migrate 约定就是这么传，不动

留作以后扩大 ctx-aware 防御时一起 sweep（比如 Story 4.6 真接 handler 用 db 事务时，每个 service 入口可以加一道 ctx.Err() 早返回）。
