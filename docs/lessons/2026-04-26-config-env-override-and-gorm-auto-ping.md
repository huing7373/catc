---
date: 2026-04-26
source_review: codex review (file: /tmp/epic-loop-review-4-2-r1.md, round 1)
story: 4-2-mysql-接入
commit: 40f5d01
lesson_count: 2
---

# Review Lessons — 2026-04-26 — infrastructure 接入必须配齐 env override + 第三方库默认行为陷阱

## 背景

Story 4.2 接入 MySQL（GORM v2 + go-sql-driver/mysql），实装在 `internal/infra/db.Open` 做 fail-fast：DSN 空 / parse 错 / ping 不通直接返 error 让 `main.go` `os.Exit(1)`。

Codex review 指出两条 finding，主题是同一个层面的 "infrastructure 接入" 疏漏：

1. **配置入口侧**：dev-story 给 MySQL 加了 `cfg.MySQL.DSN` 的 fail-fast 启动校验，**但** `config.Load` 当前只读 `CAT_HTTP_PORT` / `CAT_LOG_LEVEL` 两个 env，**不读 `CAT_MYSQL_DSN`** —— staging / prod 用环境变量注入 DB secret 的标准做法行不通，server 会用 hardcoded local DSN 连接 prod 然后失败。
2. **第三方库语义**：`gorm.Open` 内部默认 `DisableAutomaticPing = false`，会**先**做一次隐式 Ping（不尊重传入的 ctx，被 driver default dial timeout 阻塞 30s+），**然后**才走到我们写的 `sqlDB.PingContext(ctx)` 那行。fail-fast 的"显式 ping 是唯一连通性校验点"语义被破坏。

两条主题不同（config-loader env override / GORM 隐式 ping）但抽象同源：**"接一个新基础设施时，光把它跑通不够 —— 必须把 (a) 配置注入路径打通到部署侧的 secret 管理，(b) 第三方库默认行为里所有 footgun 显式关闭。"**

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `config.Load` 不读 `CAT_MYSQL_DSN`，DB secret 无法用环境变量注入 | high (P1) | config | fix | `server/internal/infra/config/loader.go`、`server/configs/local.yaml`、`server/internal/infra/config/config.go` |
| 2 | GORM `DisableAutomaticPing=false` 默认让隐式 ping 早于 ctx-aware ping，破坏 fail-fast 语义 | medium (P2) | architecture / dependency | fix | `server/internal/infra/db/mysql.go` |

---

## Lesson 1: 配置 loader 必须为每个含 secret / per-env-overridable 的字段挂 env 优先级覆盖

- **Severity**: high (P1)
- **Category**: config
- **分诊**: fix
- **位置**: `server/internal/infra/config/loader.go:12-18`（修前 const 块只列了 PORT / LEVEL 两条 env）

### 症状（Symptom）

部署 staging 时 SRE 用标准做法注入 DB 密码：

```bash
export CAT_MYSQL_DSN="cat:$(vault kv get -field=password secret/staging/db)@tcp(staging-mysql.svc:3306)/cat?charset=utf8mb4&parseTime=true&loc=UTC"
./catserver -config configs/staging.yaml
```

`config.Load` 把 staging.yaml 解析成功后**不读** `CAT_MYSQL_DSN`，`cfg.MySQL.DSN` 仍是 YAML 里的本地 sentinel `cat:catdev@tcp(127.0.0.1:3306)/cat?...`。`db.Open` 拿这个 DSN 去连 staging（本地 IP 解析失败 / auth 不过 / 没这个库），ping 失败 → `os.Exit(1)`。日志里写"mysql ping: ..."，但 SRE 看不出"DSN 没被覆盖"，以为是网络 / 权限问题，排障路径浪费几十分钟。

### 根因（Root cause）

`config.Load` 的 env override 实现是**逐字段 hardcode**，不是泛化的：

```go
const (
    envHTTPPort = "CAT_HTTP_PORT"
    envLogLevel = "CAT_LOG_LEVEL"
)

if v := os.Getenv(envHTTPPort); v != "" { cfg.Server.HTTPPort = port }
if v := os.Getenv(envLogLevel); v != "" { cfg.Log.Level = v }
```

Story 1.2 落地这套时只覆盖了已存在的两个字段。Story 4.2 给 `Config` 加 `MySQLConfig` 子结构 + DSN 字段，但**只动了 struct**，没去 loader.go 加对应 env override —— 因为本地开发用 YAML 默认 DSN 就能跑通，单测里也只测了 DSN 空 / 不合法两种 negative path，**正向链路（YAML default vs env override）漏测**。

CLAUDE.md 配置一节明写"配置格式：YAML，**支持环境变量覆盖**"——这是项目级承诺，但实装只覆盖了 2 个字段是**承诺与实装不一致**（"声明 ≠ 现实"复现：见 `docs/lessons/2026-04-25-slog-init-before-startup-errors.md` Meta 小节列举的同 pattern）。

### 修复（Fix）

在 `loader.go` 加第三条 env：

```go
const (
    envHTTPPort = "CAT_HTTP_PORT"
    envLogLevel = "CAT_LOG_LEVEL"
    envMySQLDSN = "CAT_MYSQL_DSN"  // ← 新增
)

if v := os.Getenv(envMySQLDSN); v != "" {
    cfg.MySQL.DSN = v
}
```

测试加 2 条：

- `TestLoad_MySQLDSNEnvOverride`：设 `CAT_MYSQL_DSN=u:p@tcp(prod-mysql:3306)/cat?...` → `cfg.MySQL.DSN` 等于覆盖值
- `TestLoad_MySQLDSNNoEnv_KeepsYAMLDefault`：`CAT_MYSQL_DSN=""` → 保留 YAML（本测试 fixture 无 mysql 段，验证 DSN 仍空）

顺带 doc-fix：`config.go` 的 `MySQLConfig.DSN` 字段注释里"loader 当前未挂 CAT_MYSQL_DSN 环境变量，Epic 36 部署 story 落地"已过时，改成"loader.go 已挂"；`local.yaml` 的注释同步更新。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **给 config struct 添加任何含 secret / 跨环境差异 / 部署可调** 的字段时，**必须**同步在 loader 加 `CAT_<UPPER_SNAKE>` env 覆盖 + 单测覆盖（override 命中 / env 未设保留 YAML 默认 两条），**不能**只改 struct 就交付。
>
> **展开**：
> - 项目级承诺（CLAUDE.md / ADR / docs/）说"支持环境变量覆盖"时，意思是**所有可覆盖字段**支持，不是"我心情好的字段"支持。新加字段视为**默认应该可覆盖**，除非有明确反向理由（如 server 名 / app 版本号这种 immutable identity）。
> - DSN / API key / token / 第三方 service URL / feature flag 这类典型"跨 env 差异 + 含 secret"字段，**禁止**只放在 YAML 等部署侧改文件 —— 部署 pipeline 通常只会注 env 不改 YAML in place（K8s ConfigMap immutable + Secret 单独走，写文件就要重打 image）。
> - **测试要求**：每个 env override 字段必须有 2 条单测 —— ① override 命中（设环境变量后值被读到），② 不设环境变量保留 YAML 默认。**不要**只测一条 override 命中就算完。
> - **反例 1**：`Config` 加 `Redis.Addr` 字段，只在 YAML 配，loader 不挂 `CAT_REDIS_ADDR` —— 同样会在部署时踩坑。
> - **反例 2**：env override 只在 happy path（YAML + 环境变量都设）测试，没测"未设环境变量"路径 → 实装里不小心让 `os.Getenv("X")` 返回空时也覆盖（即把 cfg.X 设成 ""），把 YAML 默认值清成空。修法：用 `if v := os.Getenv(...); v != "" { ... }` 模式（loader.go 现有写法），并显式加测试。
> - **正例**：每加一个 config 字段就过一遍 checklist —— "(a) 这个字段需要跨 env 差异吗？需要 → 加 env override；(b) 含 secret 吗？是 → YAML 留 sentinel / 留空；(c) 测试 (1) override 命中 (2) 未设保留默认 各一条。"

---

## Lesson 2: 接入第三方基础设施库时，必须显式列举所有 default-on 的"自动行为"开关，按 fail-fast 语义需求逐个 audit

- **Severity**: medium (P2)
- **Category**: architecture / dependency
- **分诊**: fix
- **位置**: `server/internal/infra/db/mysql.go:43-50`（修前 `gorm.Config{}` 字面量只设了 `SkipDefaultTransaction: true`）

### 症状（Symptom）

`db.Open(ctx, cfg)` 调用方期望：传一个短 timeout 的 ctx 进去 → 连不通时在 ctx timeout 内立即返 error。

实际行为：传 `context.WithTimeout(ctx, 2*time.Second)` 进去，但 `gorm.Open` 内部会先做一次**不带 ctx**的隐式 Ping（GORM 默认 `DisableAutomaticPing=false`），这次 ping 走 driver 的 default dial timeout（go-sql-driver/mysql 的 `timeout` query 参数没设时是 OS-level TCP timeout，典型 30s+）—— 调用方设的 2s ctx **完全没生效**，进程在 ping 阶段被卡 30s+ 才退出。

线上影响：fail-fast 语义被破坏 —— health check / k8s liveness probe 期望"启动失败 5s 内退出 → 重新调度"，实际启动卡 30s+，调度器要么超时 kill SIGKILL（连日志都没刷出来），要么误判"启动慢但还活着"。

### 根因（Root cause）

GORM v2 `gorm.Config` 的零值字段 `DisableAutomaticPing` 是 `false`（默认开启自动 ping）。这是 GORM 设计的"开发者友好"行为：让你 `gorm.Open` 一行就能拿到一个验证过连通性的 *gorm.DB。

但**反直觉**之处在于：

1. 自动 ping **不**接受 ctx 参数 —— GORM 内部直接调 `sqlDB.Ping()` 而非 `sqlDB.PingContext(ctx)`，所以你在外层包的 ctx 完全无效
2. dev-story 写的"显式 `sqlDB.PingContext(ctx)`"在自动 ping **之后**才执行 —— 看起来"显式 ping 是唯一连通性校验"是错觉，实际有两次 ping，第一次（自动）才是真正决定 fail-fast 时长的那次

dev-story 实装时**没读** `gorm.Config` 的全部字段文档，只挑了 `SkipDefaultTransaction` 一个就放心了。代码 review 也没人逐字段审视 default 行为。

### 修复（Fix）

`gorm.Config{}` 字面量加 `DisableAutomaticPing: true`：

```go
gormDB, err := gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{
    SkipDefaultTransaction: true,
    // DisableAutomaticPing：禁用 gorm.Open 内部的隐式 Ping（默认会做一次
    // 不接 ctx 的 Ping，被 driver default dial timeout 阻塞 30s+）。
    // fail-fast 语义靠下面显式的 sqlDB.PingContext(ctx) 实现。
    DisableAutomaticPing: true,
})
```

带 docstring 解释**为什么** disable，让后人不会误以为忘了 / 出于性能考虑省略了。

测试方面：现有 `TestOpen_UnreachableDSN_ReturnsPingError` 用 `127.0.0.1:1` + `?timeout=1s` query 参数 + 2s ctx 已能在测试套件中体现 fail-fast 时长 —— 修复前后都过的原因是 driver-side `?timeout=1s` 参数恰好让自动 ping 也快速失败，但 production 部署不会在 DSN 里加 `timeout=1s`（生产 DSN 一般不限 dial timeout），故这条测试**没有覆盖** GORM 隐式 ping 的真实风险。本次不加新测（构造一个能体现"自动 ping 卡 30s vs 显式 ctx ping 1s"的单测需要拦截 driver 层，超出本 story 范围），改为靠注释 + 预防规则维持纪律。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **接入任何第三方基础设施库（ORM / cache client / queue client / RPC framework）** 时，**必须**显式 audit 该库的"配置结构体所有 bool 默认行为字段" + "Open / Connect / Init 函数的隐式副作用"，按 fail-fast / observability 语义需求逐个决定 disable 还是保留，**禁止**只设 1-2 个字段就放行。
>
> **展开**：
> - 第三方库的 default 通常面向"开发者友好"（少配置、自动验证、自动重试）—— 这些 default 在生产语义下经常**与 fail-fast / 显式 ctx / 严格资源管理**冲突
> - 关键开关 audit checklist（接入新库时逐项过）：
>   - **是否有自动 health check / ping**？是 → 通常想 disable 改用自己 ctx-aware 的版本
>   - **是否有自动 retry / backoff**？是 → 通常想 disable 改用上层 middleware / circuit breaker
>   - **是否有自动 connection pool warm-up**？是 → 一般保留，但要确认参数（max conns / idle / lifetime）显式设
>   - **是否有自动 logger 注入**（写 stdout / stderr）？是 → 通常想替换成项目 logger（slog）
>   - **是否有自动 metric / trace 采集**？是 → 与项目 observability 对齐
> - **GORM 特定**：`gorm.Config` 至少检查 `SkipDefaultTransaction` / `DisableAutomaticPing` / `Logger` / `NowFunc` / `PrepareStmt` 这五个字段
> - **Redis / go-redis 特定**：`redis.Options` 至少检查 `DialTimeout` / `ReadTimeout` / `WriteTimeout` / `MaxRetries` / `OnConnect` 五个字段
> - **反例 1**：`gorm.Open(dsn, &gorm.Config{})` 直接零值结构体 → 全部 default 行为生效，等于没读文档
> - **反例 2**：复制粘贴 GitHub 上某个开源项目的 `gorm.Config{}` 而不验证它和自己的 fail-fast 语义对齐 —— 别人项目可能允许 30s 启动延迟，你的项目不允许
> - **正例**：`gorm.Config{}` 字面量里**每个**显式设的字段都带行内注释解释**为什么这个值**，让 review 时能逐字段验证

---

## Meta: 接基础设施时的双重审视

本次两条 finding 主题不同但**反映同一个思维漏洞**："接通本地 happy path 即认为 done"。

具体到 Story 4.2：
- 配置侧：本地 YAML 默认 DSN 能跑通 → 忽略部署侧的 env 注入路径
- 库使用侧：单测 ping 能在 2s 内失败 → 忽略 GORM 默认还有一次不带 ctx 的隐式 ping

**升级版规则**：未来 Claude 接入任何"会被部署 / 跨 env 的基础设施"时，self-review checklist 增加两条：

1. **跨 env 路径**：本字段从 dev → staging → prod 是怎么变的？谁注入？走 YAML / env / Secret / ConfigMap / Vault？我的 loader 都支持吗？
2. **库默认行为**：库的 Config 结构体 / Init 函数有哪些 default-on 行为？逐个对照本项目的 fail-fast / ctx-aware / 资源管理 / observability 要求 audit。

这条 Meta 应该和 `docs/lessons/2026-04-25-slog-init-before-startup-errors.md` Meta 小节合并 —— 它们都是"声明 ≠ 现实 / happy path ≠ 全路径"的具体化。下次 `/bmad-distillator` 蒸馏时建议把这三条（Story 1.2 P1/P3、Story 1.3 P2、Story 4.2 P1/P2）合并成一条 cheatsheet 项："**接入 / 改动基础设施时，必须 audit 部署路径 + 第三方库默认行为，本地能跑 ≠ 全 env 能跑**"。
