---
date: 2026-04-26
source_review: file:/tmp/epic-loop-review-4-3-r5.md (codex review round 5 of Story 4.3)
story: 4-3-五张表-migrations
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — locate auto-detect 逻辑必须 cwd + exe-relative 双 fallback（与 config.LocateDefault 一致）

## 背景

Story 4.3（迁移工具骨架 + 五张 MySQL DDL）round 5 codex review 指出：新加的 CLI 子命令
`catserver migrate` 中的 `LocateMigrations` 只检查 cwd-relative 候选（"migrations" /
"server/migrations"），缺少二进制旁边的 executable-relative fallback。同 repo 的
`config.LocateDefault` 早在 round 3 / round 4 阶段就已经做了这个 fallback —— 两处
locate 逻辑出现了"一处有、一处没有"的不对称，导致 shipped 二进制从非 repo 路径
启动时（例如 `cd /tmp && /repo/build/catserver.exe migrate up`）`migrate` 子命令
立即失败，必须手动设 `CAT_MIGRATIONS_PATH` 才能跑。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | LocateMigrations 缺 executable-relative fallback，与 config.LocateDefault 不对称 | medium (P2) | config | fix | `server/internal/cli/migrate.go` |

## Lesson 1: locate auto-detect 逻辑必须 cwd + exe-relative 双 fallback（与 config.LocateDefault 一致）

- **Severity**: medium (P2)
- **Category**: config
- **分诊**: fix
- **位置**: `server/internal/cli/migrate.go:38-64`

### 症状（Symptom）

`LocateMigrations()` 只在 `DefaultMigrationsCandidates`（cwd-relative：`"migrations"`、
`"server/migrations"`）里找。当 build.sh 产物（`build/catserver.exe`）从这两个路径
之外的 cwd 启动时（典型：CI runner、容器、运维 `cd /tmp && /repo/build/catserver
migrate up`），auto-detect 返回 `migrations dir not found`，操作员必须显式设
`CAT_MIGRATIONS_PATH` 才能继续。

同 repo 的 `config.LocateDefault()` 已经在 round 3 / round 4 修复中加了 executable-
relative fallback（用 `os.Executable()` 找 binary 旁边的 `../server/configs/local.yaml`
或 `./configs/local.yaml`）—— 同一个二进制的两条 auto-detect 路径，配置能找到、
migrations 找不到，是不对称的设计漏洞。

### 根因（Root cause）

迁移 auto-detect 是 round 3 临时加的（"覆盖 cwd=server/ 与 cwd=repo-root 两种
开发场景"），当时只考虑了 dev 在 repo 内部的两种 cwd，没考虑生产 / CI / 运维
场景下二进制可能从任意 cwd 启动。

更深层：写 round 3 的人**没有 sweep 同 repo 已有的 locate 逻辑作为参照**。
`config.LocateDefault` 当时已经有完整的 cwd + exe-relative 双层 fallback 设计 +
配套测试 + 配套 lesson（`docs/lessons/2026-04-24-config-path-and-bind-banner.md`），
但 round 3 引入新 locate 时只复制了"cwd 多候选"这一半，把"exe-relative fallback"
那一半漏掉了 —— 复制别人代码模板时没复制完整。

### 修复（Fix）

把 `LocateMigrations` 升级成两层查找，**完全对齐 config.LocateDefault 的查找顺序**：

1. cwd-relative 候选（`DefaultMigrationsCandidates`，保持原行为）
2. executable-relative 候选（新加的 `executableRelativeMigrationsCandidates()`，
   用 `os.Executable()` 推断 binary 旁边的 `../server/migrations` / `../migrations` / `./migrations`）

before（精简版）：

```go
func LocateMigrations() (string, error) {
    return locateMigrationsIn(DefaultMigrationsCandidates)
}
func locateMigrationsIn(candidates []string) (string, error) {
    for _, p := range candidates {
        info, err := os.Stat(p); if err == nil && info.IsDir() { return p, nil }
    }
    return "", fmt.Errorf("migrations dir not found; tried %v; ...", candidates)
}
```

after（精简版，完整代码见 `server/internal/cli/migrate.go`）：

```go
func LocateMigrations() (string, error) {
    return locateMigrationsIn(DefaultMigrationsCandidates, executableRelativeMigrationsCandidates)
}
func locateMigrationsIn(cwdCandidates []string, exeCandidatesFn func() []string) (string, error) {
    for _, p := range cwdCandidates { if dirExists(p) { return p, nil } }
    if exeCandidatesFn != nil {
        for _, p := range exeCandidatesFn() { if dirExists(p) { return p, nil } }
    }
    return "", fmt.Errorf("migrations dir not found; tried CWD candidates %v + executable-relative fallback; set CAT_MIGRATIONS_PATH to override", cwdCandidates)
}
func executableRelativeMigrationsCandidates() []string {
    exe, err := os.Executable(); if err != nil { return nil }
    binDir := filepath.Dir(exe)
    return []string{
        filepath.Join(binDir, "..", "server", "migrations"),
        filepath.Join(binDir, "..", "migrations"),
        filepath.Join(binDir, "migrations"),
    }
}
```

新增测试（覆盖 fallback 路径 + 错误信息形态）：
- `TestLocateMigrationsIn_ExeRelativeFallback` —— cwd 候选缺失时回退到 exe-relative
- `TestLocateMigrationsIn_AllMissingError` —— 错误信息必须含 "executable-relative" + `CAT_MIGRATIONS_PATH`
- `TestLocateMigrationsIn_IgnoresFileMatches` —— 候选指向文件时跳过（保持 IsDir 校验）
- `TestExecutableRelativeMigrationsCandidates_NotEmpty` —— 生产候选生成函数返回 binary 旁边的相对路径

签名变更：`locateMigrationsIn(candidates)` → `locateMigrationsIn(cwdCandidates, exeCandidatesFn)`，
原 `TestLocateMigrationsIn_Empty` 同步更新为 `locateMigrationsIn(nil, nil)`。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写**任何 auto-detect 路径函数**（`Locate<Foo>` / 在 N 个
> 候选里找第一个存在的）时，**必须** sweep 同 repo 已有的同类函数作为模板参照，
> 把 cwd-relative + executable-relative 两层 fallback 一起实装；**禁止**只复制
> "cwd 多候选"那一半然后声明完成。
>
> **展开**：
> - **触发条件**：任何函数名形如 `Locate*` / `Find*` / `Discover*`，作用是在多个候选
>   路径里挑第一个存在的，就属于本规则适用范围。
> - **必做动作**：实装前先 `grep -r "os.Executable" .` + `grep -r "Locate\|Discover" .`
>   找到 repo 里已有的 locate 函数 → 把它的查找顺序作为模板（cwd 候选 + exe-relative
>   候选 + 显式 env override）→ 一比一复刻整个分层。
> - **错误信息也要复刻**：`config.LocateDefault` 的错误信息提示了 cwd 候选列表 +
>   出口（`-config <path>`），新 locate 的错误信息也要给出 cwd 候选 + fallback 提示 +
>   出口（`CAT_MIGRATIONS_PATH`）—— 让运维看错误就能知道**有几条路径被试过 + 怎么手动覆盖**。
> - **测试也要复刻**：每加一层 fallback 必须配一个对应测试（`TestLocate*_CwdHit` /
>   `TestLocate*_ExeRelativeFallback` / `TestLocate*_AllMissingError`），三层测试缺
>   一层就是隐藏 bug 等地雷。
> - **反例 1**：round 3 的 locateMigrationsIn 只接 `candidates []string`，签名结构
>   就堵死了"再加 exe-relative fallback"的扩展路径 —— round 5 必须改签名+改所有调用点
>   才能补上。**写 locate 函数的可测试核心时，签名一开始就要预留 `(cwdCandidates, exeCandidatesFn)`
>   两参数**，即使先只用 cwd 那一参，函数指针那参可以传 nil；这样后续补 fallback 是加实装
>   不是改签名。
> - **反例 2**：round 3 写 LocateMigrations 时，`config.LocateDefault` 的代码就在
>   `server/internal/infra/config/locate.go`，离 `server/internal/cli/migrate.go` 三层目录，
>   但 round 3 没去读那个文件。"我有自己的需求 / 别人的不一定适用"是常见但错误的
>   借口 —— 同 repo 的同类逻辑就是最好的模板，**不读它就重新发明轮子是技术债**。
> - **未来扩展**：如果 repo 里出现**第三处** locate 逻辑（例如 `LocateAssets` /
>   `LocateMigrations` 之外又加了 `LocateTranslations`），应当把"cwd + exe-relative
>   双层查找"抽到 `internal/pkg/locate` 通用 helper，让所有 Locate 函数共享同一个
>   分层逻辑 —— 当前两处的代码重复是**已知 tech debt**，本轮 review 阶段（round 5）
>   不做这个 refactor 是为了控制爆炸半径，不是说不该做。

---

## Meta: 本次 review 的宏观教训

复制别人代码模板时，**复制不完整比不复制更危险** —— 不复制还有人会 review 出
"你这个东西是不是该参照 X" 的提醒；复制了一半看起来"已经做了 auto-detect"，
review 容易被表面相似性骗过去。round 3 / round 4 review 就是这样放过 LocateMigrations
缺 fallback 的问题，要等到 round 5 codex 显式跑"二进制从外部 cwd 启动"这条具体
路径才暴露。

防御办法：**做"locate 类"功能时强制在 PR description 里列出"对标 X.LocateDefault，
查找层级 N 层，每层都有对应测试 Y/Z"** —— 把对标关系显式写出来，模糊"我已经做了
auto-detect"声明无处遁形。
