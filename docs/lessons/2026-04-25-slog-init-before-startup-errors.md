---
date: 2026-04-25
source_review: manual review from user (inline comment via /fix-review)
story: 1-3-中间件-request_id-recover-logging
commit: 0a0d108
lesson_count: 1
---

# Review Lessons — 2026-04-25 — slog 初始化时机 vs 启动失败路径

## 背景

Story 1.3 实装了 "所有请求 JSON 日志" 的 slog 初始化逻辑（`logger.Init(cfg)`），部署层以为从进程第一条 log 开始就是 JSON。
Review 发现 `cmd/server/main.go` 里 `config.LocateDefault` / `config.Load` 失败的 `slog.Error(...)` 调用发生在 `logger.Init(cfg)` **之前** —— 此时 slog.Default() 还是 Go 标准库自带的 text handler，启动失败日志实际以文本格式吐出。
日志聚合 / 告警 / 结构化 grep 在 boot 失败时正好需要这些诊断字段，但拿到的是无法解析的文本。

讽刺点：Story 1.2 刚在 `docs/lessons/2026-04-24-config-path-and-bind-banner.md` Lesson 2 Meta 归档过这个 pattern —— "声明 ≠ 现实"，Story 1.3 原样复现了一次。本文档核心价值就在于把这条经验**从通用原则升级为针对性强的启动路径规则**。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 启动失败路径的 slog 输出落回 stdlib 文本 handler，破坏结构化日志承诺 | medium (P2) | error-handling / observability | fix | `server/cmd/server/main.go`、`server/internal/infra/logger/slog.go` |

---

## Lesson 1: 任何"早期启动错误"日志必须在 logger 显式初始化**之后**才能声称是结构化的

- **Severity**: medium (P2)
- **Category**: error-handling / observability
- **分诊**: fix
- **位置**: `server/cmd/server/main.go:24-32`（修前）

### 症状（Symptom）

二进制启动时 config 文件路径错误 / YAML 解析失败，用户看到：

```
2026/04/25 15:09:30 ERROR config load failed error=config file not found: /path/to/thing.yaml
```

而不是期望的：

```
{"time":"2026-04-25T15:09:30.8265434+08:00","level":"ERROR","msg":"config load failed","error":"config file not found: /path/to/thing.yaml"}
```

部署侧的 log aggregator（如 Loki / ELK / CloudWatch）按 JSON 解析时这行会进 `parse_failed` 桶，不触发 `level=ERROR` 告警规则。**boot 失败是最需要告警的时刻**，日志 pipeline 却在此刻失效。

### 根因（Root cause）

原 `main.go` 的调用顺序：

```go
func main() {
    flag.Parse()
    cfg, err := config.Load(configPath)
    if err != nil {
        slog.Error("config load failed", ...)   // ← 这里 slog 的 default handler 还是 Go 自带的 text handler
        os.Exit(1)
    }

    logger.Init(cfg)                              // ← 才在这里替换成 JSON handler
    // ... 后续所有 slog 都是 JSON
}
```

slog 标准库的 `slog.Default()` 在程序启动时是一个 **TextHandler**（Go 1.21+ 的默认实现）。`logger.Init(cfg)` 做的正是 `slog.SetDefault(slog.New(slog.NewJSONHandler(...)))`。

**因果链**：
- Dev 的心智模型：写 `slog.Error` → 期望走 Story 1.3 定义的 JSON handler
- 实际：`slog.Error` 调用 `slog.Default()`，default 只有在 `logger.Init` 调完之后才是 JSON handler
- 两件事中间夹着 config 加载，config 加载本身可能失败 —— 于是"失败路径"永远用 text handler 吐出
- **第二层根因**：`logger.Init(cfg *config.Config)` 的签名耦合了 config 包，导致只能在 config 加载成功之后才能调 Init。API 设计把"先加 config 再加 logger"写死了顺序。

### 修复（Fix）

**两步初始化**（bootstrap → 用户配置）：

```go
// cmd/server/main.go 顶部
func main() {
    logger.Init("info")    // (1) 启动即 JSON handler，info level 兜底

    var configPath string
    flag.StringVar(&configPath, "config", "", ...)
    flag.Parse()

    if configPath == "" {
        p, err := config.LocateDefault()
        if err != nil {
            slog.Error("config locate failed", slog.Any("error", err))  // (1) 生效：JSON 输出
            os.Exit(1)
        }
        configPath = p
    }

    cfg, err := config.Load(configPath)
    if err != nil {
        slog.Error("config load failed", slog.Any("error", err))  // (1) 生效：JSON 输出
        os.Exit(1)
    }

    logger.Init(cfg.Log.Level)    // (2) 用户配置的 level 覆盖
    slog.Info("config loaded", ...)
}
```

**API 重构**：把 `logger.Init` 从"接 `*config.Config`"改成"接 level 字符串"：

```go
// before
func Init(cfg *config.Config) *slog.Logger { ... }

// after
func Init(level string) *slog.Logger { ... }
```

好处：
- 解耦 logger 与 config 包（logger 不再 `import config`，避免循环依赖风险）
- `main.go` 可以用任意字面量 level 预热（`"info"` / `"debug"`）
- `slog_test.go` 的测试不再需要构造 `*config.Config`

测试 / 验证：
- `TestInit_LevelWarn` / `TestInit_InvalidLevelFallsBackToInfo` 改签名后继续绿
- 手动验证：`./build/catserver.exe -config /tmp/nonexistent.yaml` 返回的是 JSON 单行 error，确认 (1) 的 bootstrap init 生效

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **任何需要结构化日志的二进制 `main()`** 里，**必须**在 `flag.Parse()` / 配置加载 / 任何可能产生 `log/slog` 调用的代码**之前**做一次 **"bootstrap logger 初始化"**，用内置默认值（通常是 `info` + JSON handler），然后在配置加载成功后再做一次用户配置的初始化；**禁止**把 logger 初始化放在配置加载**之后**就期望所有日志都是结构化的。
>
> **展开**：
> - 结构化日志的"结构"是靠进程级 default handler 生效的 —— handler 不换，日志就不是你想要的格式
> - Go `slog.Default()` 在 `slog.SetDefault` 调用前总是 TextHandler，这是**反直觉**的默认（你不显式做 SetDefault，就永远不是 JSON）
> - 这条规则对所有"先加载配置才能决定 logger 配置"的 bootstrap 都适用（不限于 slog —— zap / zerolog / log/log 同理）
> - API 设计层面：**logger 初始化不要依赖 config 包**。让 logger 接受最原始的参数（level 字符串、输出目标、格式开关），让调用方从任意来源（字面量 / 环境变量 / 配置文件）喂进去 —— 这样才能在配置加载前后都调用
> - **自举原则**：两步 init 是必要的但足够简单 —— 第一次用最保守的默认（info + JSON + stdout），第二次用用户指定的 level；两次 SetDefault 是幂等的，无副作用
> - **反例 1**：`logger.Init(cfg)` 签名绑死 config 包 → 强制"先 config 再 logger" → 启动失败路径没 logger → text 输出
> - **反例 2**：靠"程序启动第一步就加载配置然后 init logger"的"配置永不失败"假设 → 配置文件缺失 / 权限错误 / 语法错误都会打破这个假设，导致在最需要诊断的时刻日志失效
> - **正例**：`logger.Init("info")` 立刻在 `main()` 顶部调，之后任意 `slog.Error(...)` 都走 JSON；配置加载成功后 `logger.Init(cfg.Log.Level)` 覆盖用户偏好

---

## Meta: 从 Story 1.2 Lesson 2 Meta 继承

这条 lesson 是 "声明 ≠ 现实" pattern 的**第三次复现**：
- Story 1.2 P1：flag 默认值声明配置文件位置，CWD 不同时不成立
- Story 1.2 P3：log 声明 server 起来了，bind 失败时不成立
- Story 1.3 P2（本次）：slog.Error 声明结构化日志，Init 未调时不成立

**升级版的规则**：未来 Claude 完成 story 后应**主动 scan `docs/lessons/` 目录**，把已归档的 meta 规则当成自己 review 前的 checklist，而不是等 reviewer 指出后才对号入座。

具体到本 story：实装完 `logger.Init(cfg)` 时应立刻问自己——"cfg 加载失败前，slog 的 default handler 是什么？" —— 这个问题直接命中根因，无需外部 review。

**结论**：`/story-done` 前建议的 self-review 步骤应该包含一条 "grep docs/lessons/ | read Meta 小节 | 逐条对照本 story 的代码"。这条建议应反馈到 `/story-done` 命令或 BMAD 的 dev-story workflow 的 step 9 completion gates。
