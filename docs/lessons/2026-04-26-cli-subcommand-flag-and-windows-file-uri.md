---
date: 2026-04-26
source_review: file: /tmp/epic-loop-review-4-3-r1.md (codex P1 findings)
story: 4-3-五张表-migrations
commit: 6368594
lesson_count: 2
---

# Review Lessons — 2026-04-26 — CLI 子命令 flag 解析必须用 NewFlagSet + 跨平台 file URI 必须避免 backslash 拼接

## 背景

Story 4.3（五张表 migrations + `catserver migrate {up|down|status}` 子命令 + golang-migrate 包装）经 codex review，2 条 [P1] finding：(1) `catserver migrate up -config configs/dev.yaml` 中 `-config` 被默默忽略 → 用错 DB；(2) Windows 绝对路径直接 `"file://" + path` 拼出非法 URI → CAT_MIGRATIONS_PATH override / 集成测试 / Windows 手动 CLI 全炸。两条都触发"看似 platform-agnostic 实则在 Windows 上炸"的同一类 footgun，合并归档。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `flag.Parse` 在第一个 positional 处停止，子命令的 -config 默默丢失 | high | error-handling | fix | `server/cmd/server/main.go`, `server/internal/cli/migrate.go` |
| 2 | `"file://" + path` 在 Windows 绝对路径上拼出非法 URI | high | architecture | fix | `server/internal/infra/migrate/migrate.go` |
| 3 | （bonus）`migrate_test.go` 三处 `nil` ctx 触发 SA1012 lint | nit | testing | fix | `server/internal/infra/migrate/migrate_test.go` |

## Lesson 1: CLI 子命令的 flag 必须用独立 NewFlagSet 解析，不能依赖顶层 flag.Parse

- **Severity**: high
- **Category**: error-handling
- **分诊**: fix
- **位置**: `server/cmd/server/main.go:79-83`（旧）+ `server/internal/cli/migrate.go::ParseMigrateArgs`

### 症状（Symptom）

文档化调用 `catserver migrate up -config configs/dev.yaml` 时，main.go 的 `flag.Parse()` 在 `migrate`（第一个非 flag 位置参数）处**停止解析**，后面的 `-config configs/dev.yaml` 永远不会被读到。结果：migrate 子命令拿到 LocateDefault 找到的 `local.yaml` DSN，把 schema 推到错误的 DB。

### 根因（Root cause）

Go 标准库 `flag` 包的设计：`Parse()` 一旦遇到非 `-` 开头的 token 就停止解析（POSIX 风格），剩余 args 通过 `flag.Args()` 拿到。这是**有意的**——避免 flag 在 positional 之间穿插造成歧义。但与多子命令 CLI（git/docker/kubectl 风格）天然不兼容：子命令位置之后的 flag 必须由**子命令自己**解析，不能让顶层 `flag.Parse` 顺便处理。

新手设计 CLI 时常想"反正 flag.Parse 全包了"，结果踩坑。

### 修复（Fix）

1. main.go 不再调全局 `flag.Parse()`。新增 `parseTopLevelArgs(os.Args[1:])`：
   - 扫描 args 找 `migrate` 位置，拆成 (preMigrate, postMigrate)
   - preMigrate 用 `flag.NewFlagSet("catserver", ContinueOnError)` 解析 `-config`
   - postMigrate 原样转发给 `cli.RunMigrate`
2. cli 包新增 `ParseMigrateArgs(args, errOutput)`：
   - 手工先把 args 拆成 (flagPart, action) —— Go flag 包不支持 flag 出现在 positional 之后
   - flagPart 用 `flag.NewFlagSet("migrate", ContinueOnError)` 解析 `-config`
   - 支持三种形态：`migrate up` / `migrate -config X up` / `migrate up -config X`
3. RunMigrate 拿到 `configOverride` 后调 `config.Load` 重新加载并覆盖 cfg

before（main.go 顶层）：
```go
flag.StringVar(&configPath, "config", "", "...")
flag.Parse()
args := flag.Args()
if len(args) >= 1 && args[0] == "migrate" {
    cli.RunMigrate(ctx, cfg, args[1:])  // configPath 永远是 LocateDefault 找到的，子命令的 -config 已丢
}
```

after：
```go
configPath, migrateArgs, isMigrate := parseTopLevelArgs(os.Args[1:])
// ... config.Load(configPath) ...
if isMigrate {
    cli.RunMigrate(ctx, cfg, migrateArgs)
    // RunMigrate 内部 ParseMigrateArgs(migrateArgs, ...) 会再次解析 -config 覆盖 cfg
}
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计带子命令的 CLI**（`tool subcmd flags...`）时，**必须**为每个子命令分配 **独立的 `flag.NewFlagSet`**，**禁止**让顶层 `flag.Parse()` 处理子命令位置之后的 flag。
>
> **展开**：
> - Go 的 `flag` 包**不支持** flag 出现在 positional 参数之后；想要 `subcmd action -flag value` 就必须自己手工拆 args（识别第一个 positional = action，剩余给 FlagSet）
> - 顶层 `flag.Parse()` 在第一个非 flag 处**默默停止**——不报错，不警告，只是 `flag.Args()` 把剩余原样返回。这是非常容易漏的"行为而非错误"
> - 用 `flag.NewFlagSet(name, flag.ContinueOnError)`（而非默认 `ExitOnError`）让解析失败可被上层 wrap，统一 fail-fast 路径
> - 子命令分发的责任**收到 cli 包**（如 `cli.RunMigrate / cli.ParseMigrateArgs`），让 main.go 只做"args 拆分 + signal-ctx + 调用 + os.Exit"——main 是 `package main` 不可单测，逻辑全埋在 main 里就只能靠手测
> - **反例**：`flag.Parse(); args := flag.Args(); if args[0] == "migrate" { do(args[1:]) }`——args[1:] 里的 `-flag` 永远不会被解析。没单测覆盖、没手测不同 -flag 位置就 ship 是必然踩坑

## Lesson 2: 跨平台 file URI 拼接禁止字符串 concat，必须 ToSlash + 不加 leading slash + 知道下游 parser 行为

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/infra/migrate/migrate.go::pathToFileURI`

### 症状（Symptom）

`"file://" + migrationsPath` 在 Windows 上炸：
- `migrationsPath = "C:\\fork\\cat\\server\\migrations"` → `"file://C:\\fork\\cat\\server\\migrations"`
- `net/url.Parse` 把 `C:\fork...` 当 host:port → 报 `invalid port ":\\fork..."`

影响范围：集成测试用 `filepath.Abs` 拿绝对路径必炸；`CAT_MIGRATIONS_PATH` 用绝对路径必炸；Windows 手动 CLI 用绝对路径必炸。

### 根因（Root cause）

两层：

1. **拼接 file URI 的"看起来很 obvious"陷阱**：开发者直觉是 `file://` + 路径就是合法 URI。但 file URI 的 host 部分必须为空（或主机名），不能含 `:`。Windows 路径首字符是字母 + `:`，URL 解析器解析时 `C:` 落入 host 位置 → 解析为 host:port → 炸
2. **看似合理的"加 leading slash"修法仍然炸（更隐蔽）**：很多在线建议是 `file:///` + slashed path，对 Unix `/usr/...` 拼出 `file:///usr/...`（合法），对 Windows `C:/...` 拼出 `file:///C:/...`（**也合法**）—— 但 **golang-migrate v4 的 source/file driver 用 `u.Host + u.Path` 还原路径**，三斜杠形态在 Windows 上还原成 `/C:/fork/...`，然后传给 `os.DirFS("/C:/...")` 又炸（"open .: filename, directory name, or volume label syntax is incorrect"）—— `/C:/...` 不是合法 Windows 路径

正确做法不是"无脑 RFC 8089 三斜杠"，而是"**必须知道下游 parser 怎么用还原后的路径**"。

### 修复（Fix）

新增 `pathToFileURI(p string) (string, error)`：

```go
func pathToFileURI(p string) (string, error) {
    abs, err := filepath.Abs(p)        // 1. 转绝对路径，cwd 独立
    if err != nil { return "", err }
    slashed := filepath.ToSlash(abs)   // 2. backslash → forward slash（Unix noop）
    return "file://" + slashed, nil    // 3. 拼接，**不**加 leading /
}
```

结果：
- Unix `/usr/share/migrations` → `file:///usr/share/migrations`（slashed 已含 leading /，自然三斜杠）
- Windows `C:\fork\cat\server\migrations` → `file://C:/fork/cat/server/migrations`（双斜杠 + drive）
  - net/url 解析后 Host="C:", Path="/fork/cat/server/migrations"
  - golang-migrate `p = u.Host + u.Path = "C:/fork/cat/server/migrations"`
  - `p[0] != '/'` 触发 `filepath.Abs` 校验（noop，已是绝对）
  - `os.DirFS("C:/fork/cat/server/migrations")` 正常工作

测试覆盖：
- `TestPathToFileURI_RelativePath`：相对路径 + 不含 backslash + 以 /migrations 结尾
- `TestPathToFileURI_UnixAbsolutePath`：Unix abs path → `file:///usr/...`（仅 Unix 跑）
- `TestPathToFileURI_WindowsAbsolutePath`：Windows abs path → `file://X:/...`（双斜杠！显式断言**不**是三斜杠 + drive 后是 `:/`）
- `TestPathToFileURI_RoundTripViaURLParse`：拼出的 URI 经 `net/url.Parse` 还原后，Windows 上 path 部分**不**以 `/` 开头（防止再次踩"三斜杠 + os.DirFS 炸"的坑）
- `TestPathToFileURI_CallableFromNew`：明显不存在的路径下，错误来自 `gomigrate.New`（开 source 失败）而非 `pathToFileURI`（URI 拼接失败）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **拼接 file URI / 任何把文件系统路径塞进 URL 的场景** 时，**必须**用 `filepath.ToSlash(filepath.Abs(p))` + 拼 `"file://"`（**不**加 leading slash），并 **必须** 加单测同时覆盖 Unix abs / Windows abs / relative 三种 case。
>
> **展开**：
> - 字符串 concat `"file://" + path` 在 Windows 上炸（`C:\` 让 URL parser 报 invalid port）
> - 看似 RFC 8089 推荐的 `file:///` 三斜杠形态在 Windows 上**也炸**——下游用 `u.Host + u.Path` 还原时，path 会是 `/C:/...`，传给 `os.DirFS` / 大多数 FS API 都报 "filename, directory name, or volume label syntax is incorrect"
> - 正确形态依赖**下游 parser 行为**：
>   - 若下游用 `u.Host + u.Path` 还原（如 golang-migrate v4 source/file）→ 用双斜杠形态，让 drive 落入 Host
>   - 若下游用 `u.Path` 单独取值（标准 RFC 8089 解读）→ 用三斜杠形态
>   - **不知道下游行为时**：写双斜杠 + 测试断言 round-trip path 不带 leading slash + drive 落 Host
> - **必须** 在 Windows 真机或 `runtime.GOOS == "windows"` skip guard 下跑测试；Linux CI 不会暴露这个问题
> - **反例 1**：`url := "file://" + path` 直接拼，没区分平台 → 跨平台 100% 炸
> - **反例 2**：盲目跟从"RFC 8089 推荐三斜杠" → `"file:///" + filepath.ToSlash(filepath.Abs(p))` → 在用 `Host+Path` 还原的 parser 下 Windows 炸第二次（更隐蔽，集成测试才暴露）
> - **反例 3**：只跑 Linux CI 不跑 Windows → 永远不踩坑直到第一个 Windows 用户上手

## Lesson 3 (bonus): 测试中传 nil ctx 触发 SA1012，必须用 context.TODO() / context.Background()

- **Severity**: nit
- **Category**: testing
- **分诊**: fix
- **位置**: `server/internal/infra/migrate/migrate_test.go:64/73/82`（旧）

### 症状

LSP 报 `SA1012: should not pass nil context, use context.TODO if you are unsure about which Context to use (staticcheck)`。

### 根因

`m.Up(nil)` / `m.Down(nil)` / `m.Status(nil)` 测试 nil migrator 路径时，把 ctx 也传 nil—— 显式触发 staticcheck 警告，且未来如果实装把 ctx 真正下传，nil ctx 会触发 panic。

### 修复

把三处 `nil` 改成 `context.TODO()`（语义：未确定 ctx 但合法）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：测试构造时，**禁止**给 ctx 参数传 `nil`；用 `context.TODO()`（不确定语义）或 `context.Background()`（明确无超时无 cancel）。

---

## Meta: 本次 review 的宏观教训

两条 P1 都属于"看似 platform-agnostic 实则隐蔽地依赖单一平台行为"的同一类陷阱：
- Lesson 1：`flag.Parse` 在 args 中遇 positional 停止解析——所有平台一致，但只有"调用方按文档化形态把 flag 放在 positional 后"才暴露
- Lesson 2：`"file://" + path` —— Linux 上无症状，Windows 上必炸；甚至"修法"本身（三斜杠）在另一种下游 parser 下也炸

**通用准则**：实装新 CLI / 新 URI / 新跨进程协议时，**必须**列举所有合法 input 形态（不只是文档化的"happy path"），按每种形态写一条单测；尤其**必须**为 Windows 路径写显式 case（`runtime.GOOS == "windows"` guard），不要假设"路径处理库自动跨平台"。
