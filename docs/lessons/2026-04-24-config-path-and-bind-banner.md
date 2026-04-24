---
date: 2026-04-24
source_review: manual review from user (inline review comments via /fix-review)
story: 1-2-cmd-server-入口-配置加载-gin-ping
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-24 — 配置路径 CWD 耦合 与 启动 banner 时序错位

## 背景

Story 1.2 实装（`cmd/server` 入口 + 配置加载 + Gin + `/ping`）review 中暴露的两条 finding：

- P1：默认 `-config` 路径是 CWD-relative，从 repo root 跑文档化的 `./build/catserver` 会因为找不到配置文件立即退出
- P3：`server started on :<port>` 这条 banner 在 `srv.ListenAndServe()` 之前打印，bind 失败时会产生假阳性启动日志

两条 finding 背后是同一个思维漏洞："声明事件"（flag 默认值 / log 行）和"事件真正发生"（文件能被读到 / 端口真的 bind 成功）没对齐。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 默认 `-config` 路径 CWD 耦合，文档化启动方式直接失败 | high (P1) | config | fix | `server/cmd/server/main.go`、`server/internal/infra/config/locate.go`（新增） |
| 2 | "server started" 日志在 bind 成功前就打印，bind 失败留假阳性 banner | low (P3) | error-handling | fix | `server/internal/app/bootstrap/server.go` |

---

## Lesson 1: flag 默认值用 CWD-relative 路径等于和 CWD 耦合

- **Severity**: high (P1)
- **Category**: config
- **分诊**: fix
- **位置**: `server/cmd/server/main.go:16`（修前）

### 症状（Symptom）

`flag.StringVar(&configPath, "config", "configs/local.yaml", ...)` 的默认值是相对路径。

- CLAUDE.md 文档化的执行方式是从 repo root 跑 `./build/catserver`（二进制产物 `build/catserver`）
- YAML 真实位置是 `server/configs/local.yaml`
- Go `os.ReadFile(relativePath)` 基于**当前工作目录**解析相对路径

结果：从 repo root 跑二进制立刻退出：

```
$ ./build/catserver
config load failed: config file not found: configs/local.yaml
```

用户只有 `cd server && ../build/catserver` 或 `./build/catserver -config server/configs/local.yaml` 才能启动。默认路径在文档化 CWD 下是坏的。

### 根因（Root cause）

两层原因：

1. **API 层面**：Go 标准库 `os.ReadFile` / `os.Stat` 对相对路径的解析是 **CWD-based**，不是**二进制所在目录-based**。这对只从项目根跑的脚本没问题，但对可分发二进制是反直觉的 —— C/C++ 世界里 argv[0]-relative 的 lookup 更常见。
2. **人肉层面**：Story 作者写 `Dev Notes §入口 main.go 骨架建议` 时默认值是 `configs/local.yaml`，心智模型是"开发者在 `server/` 目录跑"；CLAUDE.md `Build & Test` 一节说的是"从 repo root 跑 `./build/catserver`"。两份文档的 CWD 假设不一致。实装时照抄 Dev Notes 的骨架，没跟 CLAUDE.md 的执行 recipe 交叉验证。

### 修复（Fix）

在 `server/internal/infra/config/locate.go` 新增 `LocateDefault()`，按候选列表查找第一个存在的文件：

1. CWD-relative：`server/configs/local.yaml`（repo root）、`configs/local.yaml`（`server/` 目录）
2. 二进制-relative（通过 `os.Executable()`）：`<binDir>/../server/configs/local.yaml`、`<binDir>/configs/local.yaml`

`main.go` flag 默认值改为空；空值时调用 `LocateDefault()`；找不到时 fatal 并提示 `-config <path>`：

```go
flag.StringVar(&configPath, "config", "", "path to config YAML (default: auto-detect ...)")
flag.Parse()

if configPath == "" {
    p, err := config.LocateDefault()
    if err != nil { log.Fatalf("%v", err) }
    configPath = p
}
```

新增 `locate_test.go` 覆盖：第一候选命中 / 落回 exe 相对候选 / 全部缺失返回错误 / 忽略目录（避免同名目录误命中）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **给 CLI 工具设任何文件路径默认值** 时，**禁止使用 CWD-relative 路径作为唯一来源**；**必须**至少提供 ① 一组候选路径 或 ② 二进制相对路径（`os.Executable()`）解析，并给出清晰的 `-flag <path>` override 错误提示。
>
> **展开**：
> - 凡是二进制可能"从多个 CWD 被启动"（repo root / 子目录 / systemd / docker），相对路径默认值就是错的。
> - 候选路径列表要明确写进代码（不是魔法探测整棵文件系统），每条候选对应一种文档化的启动方式。
> - 写代码前**强制交叉对照**：`docs/CLAUDE.md` 的 Build/Run recipe × story 里的骨架建议。两份一旦 CWD 假设不一致，必须在 story 阶段就澄清，而不是默默实装偏向其中一方。
> - **反例**：`flag.StringVar(&path, "config", "configs/local.yaml", ...)`。看起来合理，但这串默认值绑死了"用户必须在 `server/` 目录跑"。只要有文档说从别的目录跑，这个默认值就是 bug。
> - **正例**：flag 默认值设为空字符串；空时走显式候选查找函数；错误信息里列出尝试过的路径，方便用户诊断。

---

## Lesson 2: 一个 API 把"瞬时操作 + 阻塞循环"打包时，想准确标注"瞬时操作完成"必须找它的分离版本

- **Severity**: low (P3)
- **Category**: error-handling
- **分诊**: fix
- **位置**: `server/internal/app/bootstrap/server.go:27-29`（修前）

### 症状（Symptom）

修前代码：

```go
go func() {
    log.Printf("server started on %s", addr)          // 先
    if err := srv.ListenAndServe(); err != nil && ... // 后
```

端口被占用时的日志：

```
2026/04/24 11:59:35 server started on :8080          ← 假阳性
2026/04/24 11:59:35 server run failed: listen tcp :8080: bind: ...
```

对依赖 "server started" 作为 readiness 信号的进程监控 / systemd unit / 本地验收流程 —— 这条日志出现 = 服务已起 —— 但实际上服务从未进入 accept 状态。

（注：这个 bug 在 Story 1.2 验收时作者亲眼见过一次 —— 当时 8080 被占，作者换端口继续跑但没意识到 banner 本身已经是 bug。Review 后才被正式定位。）

### 根因（Root cause）

`http.Server.ListenAndServe()` 这个 API **在一个调用里合并了两件事**：

1. **瞬时操作**：在 `Addr` 上做 TCP bind + listen（可能失败，毫秒级）
2. **阻塞循环**：进入 accept loop 直到 `Shutdown` 被调用（长时运行，正常情况永不返回）

API **不暴露**"第 1 步完成"的钩子。要想让 log 只在第 1 步成功后出现，只能用 Go 标准库提供的**分离版本**：

- `net.Listen("tcp", addr)` → 立刻返回 `Listener` 或错误（只做 bind + listen）
- `srv.Serve(listener)` → 阻塞（只做 accept 循环）

修前的代码照抄了 Story `Dev Notes §bootstrap.Run 骨架建议` 里的 `ListenAndServe` 写法。骨架本身在"happy path 下逻辑正确"，但没覆盖 "bind 失败时 log 语义是不是诚实"这个角度。

### 修复（Fix）

拆分 Listen 与 Serve：

```go
listener, err := net.Listen("tcp", addr)
if err != nil {
    return fmt.Errorf("listen %s: %w", addr, err)   // bind 失败作为 Run 的同步返回值
}
log.Printf("server started on %s", addr)             // 只有在 bind 确实成功后才打 banner

errCh := make(chan error, 1)
go func() {
    if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
        errCh <- err
        return
    }
    errCh <- nil
}()
```

`http.Server.Addr` 字段删除（之前用来告诉 `ListenAndServe` 要 bind 哪里 —— 拆分后 Serve 直接用传入的 listener，Addr 字段变成死代码）。

新增 `server_test.go`：

- `TestRun_BindFailureReturnsErrorAndNoStartedBanner`：预占端口 → Run 同步 return error + log buffer 不含 "server started"
- `TestRun_ShutdownStopsServer`：HTTPPort=0 让 OS 分配 → 50ms 后 cancel → 断言 Run 返回 nil 且 log 含 "server started" + "server stopped"

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **想 log / 通知 / 埋点一个"操作开始"事件** 时，**必须**把该操作拆成"瞬时成功的判定点 → log → 阻塞运行"三段，**禁止**把 log 放在一个不返回 / 不给钩子的合并 API 之前。
>
> **展开**：
> - Go 标准库里"xxxAndServe" / "ListenAndXxx" / "RunAndBlock" 这类合并 API **几乎总是有分离版本**，要养成反射：先查有没有 `net.Listen` + `srv.Serve` 这样的分离对，再决定用合并还是拆分。
> - 判定点通常是一个**同步返回 error or value** 的调用。log 只能放在它**成功返回之后**。
> - 写完 happy path 后必须主动想一次"如果这个操作瞬时失败，log 会不会说谎"—— 失败路径是 log 正确性的最关键测试场景。
> - 写测试时，bind 冲突场景的占位 listener **必须和被测代码用同一地址族**（Linux 下通常无影响，但 Windows 上 `127.0.0.1:N` 和 `0.0.0.0:N` 可以共存，用错会让 bind 不失败从而测试悬挂）。测试里加 `context.WithTimeout` 作为悬挂保险。
> - **反例**：`log.Printf("started"); go srv.ListenAndServe()` —— 看起来无害，实际是"声明发生在事件真正发生之前"的典型。
> - **正例**：`listener, err := net.Listen(...); if err != nil { return err }; log.Printf("started"); go srv.Serve(listener)`。

---

## Meta: 本次 review 的宏观教训

**两条 finding 背后是同一个思维漏洞：声明与现实对齐**

- Lesson 1 的 flag 默认值 = "声明配置文件在这" vs. "文件能不能真读到要看 CWD"
- Lesson 2 的 log = "声明 server 起来了" vs. "起没起要看 bind 成没成"

两个 case 里都**没有**在关键动作前做"声明与现实是否一致"的 minimal sanity check。写代码时把"写了什么" (`flag.StringVar`、`log.Printf`) 当成了"会发生什么"。

**通用规则**：任何 "declare → execute" 的代码对，如果 declare 不是 execute 的**同步结果**，就存在声明说谎的风险。关键动作前养成反问：

> 我写的这句话，在它之后代码的任何一种执行分支下，**仍然会是真的**吗？

- flag 默认值 `"configs/local.yaml"` → 在 `CWD != server/` 时**不是**真的
- `log.Printf("server started")` → 在 `ListenAndServe` bind 失败时**不是**真的

反问过不去 → 代码有 bug。把它列入 code review 的 checklist。
