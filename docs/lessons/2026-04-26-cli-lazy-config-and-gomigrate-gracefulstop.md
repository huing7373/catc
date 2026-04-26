---
date: 2026-04-26
source_review: file: /tmp/epic-loop-review-4-3-r2.md (codex P2 findings)
story: 4-3-五张表-migrations
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-26 — CLI 子命令必须 lazy load config + 长 IO 操作必须 ctx-aware GracefulStop

## 背景

Story 4.3 round 1 修了两个 P1（子命令 flag 解析 + Windows file URI），round 2 codex 又指出 2 条 [P2]：(1) parseTopLevelArgs 把 args 拆对了，但 main() 仍在 migrate 分支**之前**先跑 `LocateDefault + config.Load`，CI/container 只 ship `dev.yaml` 时 `local.yaml` 不存在 → exit(1) 永远进不到 RunMigrate；(2) `migrate.Up/Down/Status` 接 `ctx context.Context` 但**完全忽略**，gomigrate v4 的同步 API 阻塞在 MySQL I/O 时 SIGINT 只 cancel 一个 unused ctx，进程仍 hang。两条都是"看上去已经修了但实际半成品"的 follow-up bug。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | migrate 子命令必须绕过 main 的 default config load 路径 | medium | config | fix | `server/cmd/server/main.go`, `server/internal/cli/migrate.go` |
| 2 | `Up/Down/Status` 接 ctx 但忽略，长 IO 不可中断 | medium | architecture | fix | `server/internal/infra/migrate/migrate.go` |

## Lesson 1: 子命令的 config 加载必须延迟到 dispatcher 内部，main 不能预先 fail-fast

- **Severity**: medium
- **Category**: config
- **分诊**: fix
- **位置**: `server/cmd/server/main.go:55-68`（旧）+ `server/internal/cli/migrate.go::RunMigrate`

### 症状（Symptom）

文档化调用 `catserver migrate up -config configs/dev.yaml` 在 CI / container 环境（**只** ship `configs/dev.yaml`，**没有** `configs/local.yaml`）下：parseTopLevelArgs 已经把 args 正确拆出 `isMigrate=true` + `migrateArgs=["up", "-config", "dev.yaml"]`，但 main() 紧接着仍调 `LocateDefault()` 走默认查找路径 → 找不到 `local.yaml` → `os.Exit(1)`，根本进不到 `cli.RunMigrate` 让它消费自己的 `-config`。

### 根因（Root cause）

round 1 修法把 args 拆分逻辑做对了，但**没改 main 的 control flow**：先 LocateDefault → 再 Load → 再判断要不要进 migrate 分支。这是经典的"修 A 没动 B，B 仍在 A 之前执行"半成品 bug。

底层规则：**多子命令 CLI 的"global config"和"subcommand-local config"语义可能完全不同**。子命令完全可能拒绝 global config（就像 `kubectl --kubeconfig X` 子命令不应被 default kubeconfig 不存在 fail 阻挡一样）。main 不能做"我先把 config 加好你子命令直接用"的乐观假设——子命令必须有自己的 config resolution 路径，main 至多传一个 hint。

### 修复（Fix）

1. main.go：检测到 `isMigrate=true` 立刻进入分支；分支内部：
   - 顶层 `-config`（`catserver -config X migrate up` 形态）若给了 → main 试 Load 一次，作为 hint 传入
   - 顶层 `-config` 没给 → main 传 `nil cfg`
   - **不**调 LocateDefault
   - 进入 `cli.RunMigrate(ctx, preCfg, migrateArgs)` 后立刻 `os.Exit(0)`/`os.Exit(1)`，不 fall-through 到普通 server 启动路径
2. `cli.RunMigrate`：
   - 优先级：args 里的 `-config` > 调用方传入 cfg > LocateDefault 兜底
   - cfg=nil 且 args 也没 `-config` 才走 LocateDefault；这时 LocateDefault 失败属于"用户既没传 -config 又没默认 config"的合法 fail-fast

before（main.go）:
```go
configPath, migrateArgs, isMigrate := parseTopLevelArgs(os.Args[1:])
if configPath == "" {
    p, _ := config.LocateDefault()  // 在 CI 没 local.yaml 时这里 exit
    configPath = p
}
cfg, _ := config.Load(configPath)   // 这里 exit
// ...一长串走到下面才看到 migrate 分支
if isMigrate { cli.RunMigrate(ctx, cfg, migrateArgs); os.Exit(0) }
```

after（main.go）:
```go
configPath, migrateArgs, isMigrate := parseTopLevelArgs(os.Args[1:])
if isMigrate {
    var preCfg *config.Config
    if configPath != "" {
        c, err := config.Load(configPath)
        if err != nil { /* slog.Error + os.Exit(1) */ }
        preCfg = c
    }
    if err := cli.RunMigrate(migrateCtx, preCfg, migrateArgs); err != nil { /* exit */ }
    os.Exit(0)
}
// 下面才是非 migrate 路径的 LocateDefault + Load
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 设计/修复**多子命令 CLI** 时，**必须**让每个子命令在 dispatcher 内部自己 resolve config（`-config` flag > caller-provided > default lookup），main 不能在分发前预先 LocateDefault + Load —— 那会让"显式传子命令 -config"的合法用法在 default config 不存在的环境里直接 fail-fast。
>
> **展开**：
> - main 的责任：args 拆分 → 选 dispatcher（普通 server / migrate / dev）。**仅此而已**。
> - dispatcher 的责任：**自己** resolve config，自己 Load，自己 fail。
> - "main 传一个已经 Load 好的 cfg 给 dispatcher"是 acceptable 的 hint 但**不是契约** —— dispatcher 必须能在 cfg=nil 时自给。
> - **反例**：在 main 顶部 `cfg := config.MustLoad()` 然后所有子命令共享 cfg —— 看上去 DRY，实际把"任意子命令必须能在 default config 不存在时跑通"的部署灵活性给毁了。
> - 测试覆盖：必须有 `TestRunXxx_NilCfgWithBadConfigOverride` 这类 case，断言"-config bad.yaml" 时错误来自 Load override 路径，**不是** LocateDefault —— 否则 main 流程的 short-circuit bug 抓不到。

## Lesson 2: 长阻塞 IO 操作接收 ctx 但底层不支持时，必须用 goroutine + select + 协议化 stop chan

- **Severity**: medium
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/infra/migrate/migrate.go:140-176`（旧）

### 症状（Symptom）

`migrate.Up/Down/Status` 函数签名接 `ctx context.Context`，但函数体内**完全没用** ctx ——直接调 `mg.m.Up()` / `mg.m.Down()` / `mg.m.Version()`。golang-migrate v4 的这三个方法不支持 ctx，会同步阻塞在 MySQL I/O 上（metadata lock 抢占 / 慢 DDL / blackhole network）。main.go 用 `signal.NotifyContext` 给 SIGINT/SIGTERM 接了 ctx —— 但 ctx cancel 后，CLI 进程仍 hang，因为 Up/Down/Status 没有任何机制感知 ctx done。运维 Ctrl+C 一次没反应就 Ctrl+C 第二次（force kill），然后 schema 处于 dirty 状态需要手工修。

### 根因（Root cause）

Go 中"接 ctx 但忽略"是**最危险**的 API 反模式之一 —— 调用方看签名以为自己有 cancel 能力，实际没有；ctx-aware 测试也容易绿（fake 实现都接受 ctx）。一旦底层第三方库（如 gomigrate v4 / 老 driver / 部分 protobuf 生成的 client）不支持 ctx，开发者要么：
- (a) 直接接 ctx 但不使用 —— **错**，造成"假 cancel"
- (b) 不接 ctx —— 让上层无法 cancel，但至少诚实
- (c) **接 ctx 并用 goroutine + select 给底层包一层 cancel 协议**

(a) 是默认选择且 review 不容易抓到（要观察"ctx 参数在函数体内出现 0 次"或"测试只验单一返回值"）。

gomigrate v4 的设计：用户可以往 `*Migrate.GracefulStop` chan 发一个 `true`，下一个 statement boundary 它会停下来；这是异步、不立即的（保证 schema 不会半执行）。但默认 nobody knows this —— 文档藏在 README 中段，而且与"ctx-aware Go API"的常规直觉相反。

### 修复（Fix）

引入 `runWithCtx(ctx, stop, fn) error` 抽象：

```go
func runWithCtx(ctx context.Context, stop stopSender, fn func() error) error {
    done := make(chan error, 1)
    go func() { done <- fn() }()
    select {
    case err := <-done:
        return err
    case <-ctx.Done():
        stop.sendGracefulStop()  // 非阻塞 send 到 m.GracefulStop chan
        return ctx.Err()
    }
}
```

- `stopSender` interface：`sendGracefulStop()`，由 `realStopSender{m *Migrate}` 实现（把 `true` 非阻塞发到 `m.GracefulStop` chan）
- Up/Down/Status 全用 `runWithCtx(ctx, realStopSender{m: mg.m}, ...)` 包装
- 后台 goroutine 保留运行让 fn 自然收尾（避免 goroutine leak），但调用方立刻拿到 ctx.Err()
- 单测注入 `fakeStopSender` 验证 cancel 路径在 ctx cancel 后 < 2s 返回 + stop 被调用

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 实现"接 `ctx context.Context` 但底层第三方调用不支持 ctx"的方法时，**必须**用 `goroutine + select { <-done; <-ctx.Done() }` 模式 + 协议化的 stop hook（GracefulStop chan / Cancel func / Close 兜底），**禁止**接了 ctx 但函数体内 0 次引用 ctx —— 那是"假 cancel"，调用方看签名期望能 cancel，实际 SIGINT 时进程仍 hang。
>
> **展开**：
> - 接 ctx 的方法必须**至少**做一件事：(a) 把 ctx 传给底层（`*WithContext` 方法）；(b) `runWithCtx` 模式包装；(c) 如果连 (b) 都做不到（例如底层无任何 cancel hook），改函数签名**去掉** ctx 参数 + 在 godoc 标 `// 不可中断，慎用` —— 诚实比假承诺好。
> - select 收到 ctx.Done() 后：调用方必须立刻返回 `ctx.Err()` 解锁；后台 goroutine 仍可运行让底层自然收尾（确定性 cleanup），但不能阻塞调用方。
> - 第三方库的"非 ctx-aware 但有 stop hook"模式（gomigrate GracefulStop / sql.DB SetConnMaxLifetime / grpc client.CloseSend）—— 文档常藏在 README 中段，**必须**搜 README + 主 type 的字段一遍才能找到。
> - **反例 1**：`func (s *S) Up(ctx context.Context) error { return s.thirdParty.DoBlocking() }` —— ctx 参数从未被使用，调用方被骗。
> - **反例 2**：写 select 但忘了 stop hook：`case <-ctx.Done(): return ctx.Err()` —— 调用方解锁了，但底层 goroutine 仍 hang 在 DoBlocking()，进程退出时 leak。
> - **反例 3**："反正测试 fake migrator 接受 ctx 就过了" —— 测试必须用 `time.After(2*time.Second)` 验证 ctx cancel 后**真的** < 2s 返回 + stop hook **真的**被调用，否则覆盖等于零。
> - 测试 pattern 模板：
>   ```go
>   ctx, cancel := context.WithCancel(context.Background())
>   stop := &fakeStopSender{}
>   fnDone := make(chan struct{})
>   fn := func() error { <-fnDone; return errors.New("late") }  // 阻塞直到 fnDone
>   got := make(chan error, 1)
>   go func() { got <- runWithCtx(ctx, stop, fn) }()
>   cancel()
>   select { case err := <-got: assertEq(err, context.Canceled)
>            case <-time.After(2*time.Second): t.Fatal("did not return") }
>   assert(stop.called)
>   close(fnDone)  // 让后台 goroutine 退出，避免 leak 影响后续 test
>   ```

---

## Meta: 本次 review 的宏观教训

两条 finding 共享同一个底层模式：**"看上去已经修但实际半成品"**。

- Lesson 1：args 拆分修对了，**没动**在它前面的 control flow → main 仍先 fail。修复必须做完整的"我改了 X，X 之前 / 之后的所有依赖路径都得重看一遍"。
- Lesson 2：函数签名加了 `ctx context.Context` 看上去 ctx-aware，函数体内**没用** ctx → 假 cancel。修复必须做 `grep -L "ctx.Done()\|\bctx\.Err()\|WithContext" path/to/file.go` 之类的反查，断言"接 ctx 的方法**确实**用了 ctx"。

未来 Claude 修 review 时**必须**做"修复完整度自检"：每条 finding 修完后回答这 5 个问题：(a) 我改的代码段，前后控制流是否都重看了？(b) 我改的接口，所有 caller 是否都覆盖到？(c) 新加的参数（ctx / option / err sentinel），我**真的**在函数体内用了？(d) 测试 fake 是否过于宽容（接受任何输入都过）→ 必须有用 timing assertion / behavior assertion 的"硬"测试 case；(e) review 的 sweep 范围（grep ctx 忽略 / grep 默认 config load）是否做了。
