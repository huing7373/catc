---
date: 2026-04-26
source_review: codex review for Story 4.3 round 3 (/tmp/epic-loop-review-4-3-r3.md)
story: 4-3-五张表-migrations
commit: c1b7e4b
lesson_count: 2
---

# Review Lessons — 2026-04-26 — CLI 默认相对路径必须 auto-detect 多 cwd & gomigrate GracefulStop 必须等 fn 真停

## 背景

Story 4.3 review round 3 针对 migrate 子命令实装，发现两条相关但主题不同的问题：
（1）migrate.go 的默认 migrationsPath 硬编码 `"migrations"`，但 build.sh 产物在
`<repo>/build/catserver`，从 repo-root 跑时 cwd 下没有 `migrations` 目录 → 失败；
（2）migrate.go 的 `runWithCtx` 在 ctx cancel 后立刻 return，导致 caller 的 defer
Close 把 driver 关了，让 gomigrate 后台 goroutine 失去连接、schema_migrations 锁在
dirty=true。两条都涉及"CLI 模式下进程生命周期边界"的共同教训。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | migrate 默认 migrationsPath 必须 auto-detect 多 cwd | high | config | fix | `server/internal/cli/migrate.go` |
| 2 | gomigrate GracefulStop 后必须等 fn 真停才退出 | high | error-handling | fix | `server/internal/infra/migrate/migrate.go` |

## Lesson 1: migrate 默认 migrationsPath 必须 auto-detect 多 cwd

- **Severity**: high
- **Category**: config
- **分诊**: fix
- **位置**: `server/internal/cli/migrate.go:165-168`（旧实装）

### 症状（Symptom）

`scripts/build.sh` 产出 `<repo>/build/catserver`，文档化跑法是从 repo-root：

```
./build/catserver migrate up
```

旧实装 `migrationsPath` 默认 `"migrations"`，在 cwd=repo-root 时找
`<repo>/migrations` —— 不存在（migrations 实际在 `<repo>/server/migrations/`）。
gomigrate.New 报 "no such file or directory" → migrate 命令直接挂。

而 `config.LocateDefault()` 已经显式支持 `server/configs/local.yaml` +
`configs/local.yaml` 双 cwd auto-detect —— 同一个二进制的 config / migrations 路径
不一致是设计漏洞。

### 根因（Root cause）

CLI 工具的"默认相对路径"必须把"用户从哪个目录跑二进制"作为可变维度处理。
开发时只在 `cd server && go run ./cmd/server/ migrate up` 一种 cwd 下测过，
路径假设 `cwd=server/` → `"migrations"` 正确。但 build.sh 产物在 repo-root，
运维 / CI / docker 镜像里典型 cwd 都不是 server/。

更深层：项目已有 `config.LocateDefault()` 给 config 文件做了双候选 fallback，
但 migrate path 单独走了一条硬编码 default，没复用这个模式 → 不一致。

### 修复（Fix）

在 `internal/cli/migrate.go` 新增 `LocateMigrations()`，模仿
`config.LocateDefault()` 的 auto-detect 模式：

```go
var DefaultMigrationsCandidates = []string{
    "migrations",                              // cwd=server/
    filepath.Join("server", "migrations"),     // cwd=repo-root
}

func LocateMigrations() (string, error) {
    for _, p := range DefaultMigrationsCandidates {
        info, err := os.Stat(p)
        if err == nil && info.IsDir() {
            return p, nil
        }
    }
    return "", fmt.Errorf("migrations dir not found; tried %v; set CAT_MIGRATIONS_PATH to override", DefaultMigrationsCandidates)
}
```

`RunMigrate` 改成：

```go
migrationsPath := os.Getenv("CAT_MIGRATIONS_PATH")  // env 显式覆盖最优先
if migrationsPath == "" {
    p, err := LocateMigrations()                    // 否则 auto-detect
    if err != nil {
        return fmt.Errorf("migrate: %w", err)
    }
    migrationsPath = p
}
```

新增 5 个单测：cwd=server/ 找 "migrations" / cwd=repo-root 找 "server/migrations" /
两个候选都没 → 报带提示的 error / 同名文件不能误判为目录 / 空候选列表防御性测试。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写 CLI 工具的"默认相对路径"** 时，**必须** **同时
> 列出从典型 cwd 跑该二进制的所有候选路径并 fallback 探测，并支持 env override**。
>
> **展开**：
> - 典型 cwd 至少包括：`<module>/`（dev `go run` 模式）+ `<repo-root>/`（build 产物
>   被运维 / CI / docker 跑的模式）。如果项目还有 `install/bin` 形态，再加一条
>   `os.Executable()`-relative 候选
> - 探测顺序：env override → cwd-relative 候选 → exe-relative 候选 → 报带候选列表
>   + override hint 的 error
> - 同一项目多个 CLI default path（config / migrations / data dir / log dir 等）必须
>   走**同一套** locate 函数模式 —— 不能 config 用 LocateDefault 而 migrations 硬编码
> - **反例**：
>     - `defaultPath = "migrations"`（假设单一 cwd）
>     - `defaultPath = filepath.Join(serverRoot(), "migrations")`（用编译期常量假设
>       源码布局，部署后失效）
>     - 报错只说 "not found"，不列试过的候选 + override 方法

## Lesson 2: gomigrate GracefulStop 后必须等 fn 真停才退出

- **Severity**: high
- **Category**: error-handling
- **分诊**: fix
- **位置**: `server/internal/infra/migrate/migrate.go:175-177`（旧实装）

### 症状（Symptom）

旧实装：

```go
select {
case err := <-done:
    return err
case <-ctx.Done():
    stop.sendGracefulStop()
    return ctx.Err()  // ← 立刻返回，不等 done
}
```

CLI SIGINT → ctx cancel → `runWithCtx` 立刻 return → `RunMigrate` 走
`defer mig.Close()` → migrate.Close 关 source / database driver。
但 gomigrate 的 `GracefulStop` 只是个**信号** —— 它让 Up/Down 在**下一个
statement 边界**停下，是异步的。后台 goroutine 此时可能正在 commit
schema_migrations 行；driver 突然被关 → SQL 失败 → schema_migrations 留在
**dirty=true** 状态，下次 migrate 起来需要手工 fix（gomigrate 的 dirty 锁机制）。

### 根因（Root cause）

ctx-aware 包装的目标是"让 caller 拿到 ctx.Err() 立刻 unblock"，但忽略了
**底层操作不是真的可被 cancel** 的事实。gomigrate v4 的 Up/Down 不接 ctx，
只能通过 GracefulStop chan 发"请在边界停"的信号；真停下来需要等
goroutine 跑完当前 statement 再 return done。

把"runWithCtx 立刻 return ctx.Err()" 等同于 "操作已停止" 是错误模型。
对 caller 来说，return 之后会做清理动作（defer Close、释放资源）；如果操作
还在跑，清理会破坏它。

### 修复（Fix）

ctx cancel 后**等 done channel 实际返回**，但加一个 grace timeout（30s）防止
极端情况（长 ALTER / metadata lock）下永远等：

```go
select {
case err := <-done:
    return err
case <-ctx.Done():
    stop.sendGracefulStop()
    grace := time.NewTimer(gracefulStopTimeout)
    defer grace.Stop()
    select {
    case <-done:
        return ctx.Err()  // gomigrate 干净停下；caller 仍知道是被 cancel 的
    case <-grace.C:
        slog.Warn("migrate: GracefulStop did not return within grace period, schema may be dirty",
            slog.Duration("grace_timeout", gracefulStopTimeout))
        return ctx.Err()  // 极端情况：让进程退出，下次 status 暴露 dirty
    }
}
```

抽出 `runWithCtxAndTimeout(ctx, stop, fn, graceTimeout)` 让单测能在毫秒级验证
（不等 30s）。新增 3 个单测：模拟 gomigrate 收到 stop 信号后过 50ms 才让 done
返回 / fn 永不返回时 grace timeout 触发 / DeadlineExceeded 路径同样 wait-for-done。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **包装"通过 stop channel / signal 让底层异步停下"
> 的 ctx-aware adapter** 时，**必须** **发完 stop 信号后等底层 fn 真返回，加 grace
> timeout 兜底**，**禁止** **直接 return ctx.Err() 让 caller 提前清理资源**。
>
> **展开**：
> - 区分两类底层操作：
>     1. **真 ctx-aware**（如 `database/sql.QueryContext`、`http.Client.Do(req.WithContext)`）
>        —— ctx cancel 会让底层立刻 abort + return；这种可以直接 return ctx.Err()
>     2. **信号式 stop**（如 gomigrate.GracefulStop、tcpListener.Close、custom
>        worker 的 stop chan）—— stop 信号是异步的，必须等 fn 真返回再 return
> - 加 grace timeout 处理极端不响应场景（10s ~ 60s 范围；按操作典型耗时定）；
>   超时时 log warn + return ctx.Err()，不无限等
> - return ctx.Err() 而不是 fn 的实际 error —— caller 关心的是"被 cancel 了"，
>   底层的 `gomigrate.ErrAborted` 不需要 leak 出去
> - 抽个 `runWithCtxAndTimeout(ctx, stop, fn, graceTimeout)` 内部函数让单测能
>   在毫秒级验证 wait-for-done 行为
> - **反例**：
>     - `case <-ctx.Done(): stop(); return ctx.Err()`（漏 wait-for-done）
>     - `case <-ctx.Done(): stop(); <-done; return ctx.Err()`（漏 grace timeout，
>        极端情况下进程卡死）
>     - 把 fn 的 ErrAborted / context-related error 透传给 caller（让 caller 区分
>        cancel vs business error 变难）
>     - 在 caller 层加 defer Close 但 adapter 不 wait-for-done（资源被 caller
>        提前清理破坏底层操作）

---

## Meta: 本次 review 的宏观教训

两条 finding 表面无关（一个是路径配置、一个是 ctx 取消），但底层是同一个
**"CLI 进程生命周期边界"** 维度被忽略：

- Lesson 1：CLI 启动时 cwd 是可变的（dev / build 产物 / docker 跑法不同）
- Lesson 2：CLI 退出时 ctx cancel 不等于操作停止（异步 stop signal 模型）

通用规则：**写 CLI 工具时，"启动 cwd" 和 "退出时机" 两个维度都不能假设
单一 happy path**。LSP / web server / library 都不太遇到这两个问题（因为
cwd 由调用方固定、shutdown 由调用方协调），但 CLI 直接面对运维 / CI /
人类用户三个场景，必须做防御。

未来类似 review：CLI 工具改动 → 主动检查"启动 cwd"+"退出时机"两个清单。
