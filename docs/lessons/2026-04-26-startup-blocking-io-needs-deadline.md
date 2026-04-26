---
date: 2026-04-26
source_review: codex review on Story 4.2 round 2 (/tmp/epic-loop-review-4-2-r2.md)
story: 4-2-mysql-接入
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-26 — 启动路径阻塞 IO 必须有 deadline & dev 文档命令必须与默认配置自洽

## 背景

Story 4.2（MySQL 接入）round 2 codex review 给出 2 条 P2 finding。第一条是 `cmd/server/main.go` 把 `signal.NotifyContext` 创建的 ctx（无 deadline，仅响应 SIGINT/SIGTERM）直接传给 `db.Open` → PingContext，碰到 blackhole host / 慢 DNS 会被 driver default dial timeout 卡住，fail-fast 语义实际不快。第二条是 `configs/local.yaml` 默认 DSN 用专用账号 `cat:catdev/cat`，但 `server/README.md` 文档化的 docker bootstrap 命令只有 `MYSQL_ROOT_PASSWORD=catdev` —— fresh local env 跟着 README 跑会启动失败。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | startup signal-ctx 没 deadline → db.Open ping 可挂 30s+ | medium (P2) | error-handling / reliability | fix | `server/cmd/server/main.go:59-69` |
| 2 | 默认 DSN 与 README docker 命令不一致 → fresh env 启动失败 | medium (P2) | docs / config | fix | `server/configs/local.yaml:15` & `server/README.md:51` |

## Lesson 1: 启动路径阻塞 IO 必须包局部短 timeout，不能复用 signal-ctx

- **Severity**: medium (P2)
- **Category**: error-handling / reliability
- **分诊**: fix
- **位置**: `server/cmd/server/main.go:59-96`

### 症状（Symptom）

`main` 用 `signal.NotifyContext(context.Background(), SIGINT, SIGTERM)` 拿 ctx 后直接传给 `db.Open(ctx, cfg.MySQL)`。设计意图是 fail-fast：DSN 错 / 网络不通要立刻 `os.Exit(1)`。但 `signal.NotifyContext` 返的 ctx **没有 deadline**，只在收到信号时 cancel；如果 DSN 指向 blackholed host（防火墙黑洞）或 DNS 解析慢，driver 走 `net.DialTimeout` 默认值 ≈ 30s 才报错，启动卡半分钟以上才退出。运维 / k8s readiness probe 等不到 fast 失败信号。

### 根因（Root cause）

`signal.NotifyContext` 的语义是"信号驱动 cancel"，不是"启动阶段超时控制"。把它当作"全局 main ctx"传给所有阻塞 IO，会让任何**没有自己 timeout 的 IO**继承"无 deadline"属性。MySQL driver / DNS resolver / HTTP client 等很多组件都有自己的 default timeout（30s / 90s / 永久不超时各有），但通常**比启动阶段可接受时长长一个数量级**。signal-ctx 是 **server lifecycle** 的合适抽象，**不是** **startup blocking IO** 的合适抽象 —— 后者需要的是"启动应该在 N 秒内完成或失败"的 deadline 语义，二者职责不同。

### 修复（Fix）

在 main.go 调 db.Open 前包一层 `context.WithTimeout(ctx, dbOpenTimeout)`（5s），用包过的 ctx 调 db.Open，调完立刻 cancel；后续 `bootstrap.Run` 仍用原 signal-ctx 跑 server lifecycle。

```go
const dbOpenTimeout = 5 * time.Second

// ...
dbOpenCtx, dbOpenCancel := context.WithTimeout(ctx, dbOpenTimeout)
gormDB, err := db.Open(dbOpenCtx, cfg.MySQL)
dbOpenCancel()
if err != nil { /* fail-fast */ }
```

5s 选型：太短（< 2s）误杀慢机 / VPN / 本地 docker mysql 启动延迟；太长（> 10s）违反 fail-fast 初衷。`db.Open` 内部已通过 `DisableAutomaticPing: true` 关掉 GORM 的隐式 ping（不尊重 ctx），显式 `sqlDB.PingContext(dbOpenCtx)` 此时才能真正 honor 这个 5s timeout。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写 server `main.go` 启动路径调用阻塞 IO 时（DB 连接 / Redis 连接 / 远端 service handshake / DNS lookup）**，**必须** **用 `context.WithTimeout` 包一层局部短 timeout（典型 5-10s），不能直接传 `signal.NotifyContext` 拿到的 ctx 当输入**。
>
> **展开**：
> - `signal.NotifyContext` 的 ctx 是 server **lifecycle** 抽象（"运行到 SIGTERM 为止"），**不是** **startup deadline** 抽象（"启动 N 秒内完成或失败"）。
> - 第三方 driver / client 几乎都有自己的 default timeout，但通常远长于"启动阶段可接受时长"（mysql driver dial 30s+，net.Dial 90s+，DNS 默认无 timeout）。不主动包短 timeout，fail-fast 就只能 fail-slow。
> - 局部 timeout cancel 后**不能**影响主 signal-ctx：`dbOpenCtx, cancel := context.WithTimeout(ctx, ...)`+ 调完立即 `cancel()`，主 ctx 仍可用于后续 `bootstrap.Run`。
> - GORM 的 `DisableAutomaticPing: true` 是必需配套：GORM 默认 `gorm.Open` 末尾的隐式 Ping **不尊重传入的 ctx**，会用 `*sql.DB.Ping()`（裸 ping，无 ctx）；不关掉这个开关，外层 5s timeout 失效（参见 docs/lessons/2026-04-26-config-env-override-and-gorm-auto-ping.md Lesson 2）。
> - **反例**：
>   ```go
>   ctx, stop := signal.NotifyContext(context.Background(), SIGINT, SIGTERM)
>   defer stop()
>   gormDB, err := db.Open(ctx, cfg.MySQL)  // 反例：ctx 没 deadline，blackhole host 时卡 30s+
>   ```
>   正确：
>   ```go
>   ctx, stop := signal.NotifyContext(context.Background(), SIGINT, SIGTERM)
>   defer stop()
>   dbOpenCtx, dbOpenCancel := context.WithTimeout(ctx, 5*time.Second)
>   gormDB, err := db.Open(dbOpenCtx, cfg.MySQL)
>   dbOpenCancel()
>   ```
> - **审计点**：未来在 main.go 加 redis 连接 / 远端 auth 校验 / migration runner 时同理 —— 每个启动阻塞 IO 单独包自己的短 timeout，**不**共用一个"启动 ctx"。

## Lesson 2: dev 文档的 bootstrap 命令必须与默认配置自洽，README docker run 必须能 reproduce 默认 DSN

- **Severity**: medium (P2)
- **Category**: docs / config
- **分诊**: fix
- **位置**: `server/configs/local.yaml:15` & `server/README.md:51`

### 症状（Symptom）

`local.yaml` 默认 DSN: `cat:catdev@tcp(127.0.0.1:3306)/cat?...`（user=cat, password=catdev, db=cat）。

但 `server/README.md` "MVP 演进依赖" 表给的 docker bootstrap 命令是：

```bash
docker run -d --name cat-mysql -e MYSQL_ROOT_PASSWORD=catdev -p 3306:3306 mysql:8.0
```

这条命令只创建 `root` 用户、没有 `cat` 用户、也没有 `cat` 数据库。新 dev 跟着 README 跑：先 `bash scripts/build.sh`，再 `docker run ...`，再启 server → server 启动时 PingContext 报 `Access denied for user 'cat'@...` 或 `Unknown database 'cat'`，启动失败。dev 第一次接触项目就被卡 → 排障要么"改 local.yaml 用 root"（污染 working tree），要么"手动 SQL 创建 user/db"（README 没写），都会消耗几十分钟挫败感。

### 根因（Root cause）

文档的 bootstrap 命令（"复制粘贴跑起来"）和默认配置（"复制粘贴改"）来自不同写作时刻 —— README 在 Story 1.10（节点 1 收官）写的，那时还没有 MySQL 配置；Story 4.2 加 MySQL 时只更新了 yaml 默认值和 ADR，**没有**回头同步 README 的 docker 命令。两份文档隔几个 epic 写、维护成本分散，没人注意到"文档命令 + 默认配置"必须当作一对契约维护。

### 修复（Fix）

把 README docker 命令补齐三件套环境变量，让 docker 容器启动时直接创建 `cat` 用户 + `cat` 数据库（mysql 官方镜像支持这套 env）：

```diff
- docker run -d --name cat-mysql -e MYSQL_ROOT_PASSWORD=catdev -p 3306:3306 mysql:8.0
+ docker run -d --name cat-mysql -e MYSQL_ROOT_PASSWORD=catdev -e MYSQL_USER=cat -e MYSQL_PASSWORD=catdev -e MYSQL_DATABASE=cat -p 3306:3306 mysql:8.0
```

并在表格"非 docker 路径"补一行手工 SQL（`CREATE USER 'cat'@'%' IDENTIFIED BY 'catdev'; CREATE DATABASE cat; GRANT ALL ON cat.* TO 'cat'@'%';`）兜住 brew/winget 装 native MySQL 的 dev。

不动 yaml 默认 DSN：原因是 ADR-0003 §"DSN 配置策略" 钦定本地推荐用专用账号 `cat`（不用 root，符合 dev/staging/prod 一致的 user 隔离实践）。改 yaml 用 root 反而违反 ADR。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **修改默认配置（yaml 默认 DSN / 默认端口 / 默认凭据）** 时，**必须** **同时 grep 项目所有 README / onboarding 文档里"复制粘贴可执行"的 bootstrap 命令，确保命令产物能直接被默认配置消费**。
>
> **展开**：
> - 默认配置和 dev 文档的 bootstrap 命令构成一对**双向契约**：用户只读 README 跑命令、然后用默认配置启 server，必须能成功。任一侧改动都要同步另一侧。
> - 同步触发关键词：`docker run` / `brew install` / `winget install` / `MYSQL_ROOT_PASSWORD` / `redis-server` / `createdb` / `psql -c CREATE` 等 bootstrap 形态。
> - mysql 官方镜像支持 4 个 env 变量初始化（`MYSQL_ROOT_PASSWORD` 必填 / `MYSQL_USER` / `MYSQL_PASSWORD` / `MYSQL_DATABASE` 三件套联动创建非 root user + database）—— 文档化 docker 命令时**默认全配齐**，不要只写 root 兜底。
> - **审计点**：每次给 yaml 加新字段（DSN / connection string / endpoint URL）：
>   1. grep 同名字段 / 关键词在 server/ + docs/ 下的所有 README
>   2. 检查 README "依赖" / "本地启动" / "环境变量" 段是否有对应 bootstrap 命令
>   3. 跑命令产物（不需要真启 docker，对照 env vars 看）能否直接被默认配置消费
> - **反例**：
>   - yaml 默认 `dsn: "user_a:pwd@tcp(host:port)/db_a"` + README docker 命令只 `MYSQL_ROOT_PASSWORD=pwd` —— 启动必失败。
>   - yaml 默认 `redis_addr: "localhost:6380"` + README `docker run -p 6379:6379 redis` —— 端口不匹配。
> - **跨文档副本**：本项目有 4 处 cat:catdev 引用（ADR-0003 / story 4-2 file / config.go 注释 / local.yaml）—— ADR 和 story file 是历史快照（不维护）；config.go 注释 + local.yaml 是 active source。改默认值时只改 active 处即可，但 README 必须同步。

---

## Meta: 本次 review 的宏观教训

两条 finding 表面话题不同（ctx timeout vs README docker），底层是同一个反模式：**"接入新 infrastructure 时只盯着代码层接通，没沉下来检查启动路径在异常网络场景的行为 + 没回头同步 dev onboarding 文档"**。Story 4.2 round 1 review 已经修过 env override / gorm auto-ping 这一类"infrastructure 接入边界"问题，round 2 又出 2 条同主题 —— 说明"接 infra"是反复踩坑领域，**未来类似 story 应该把"启动路径异常路径 + 文档同步"列为 mandatory checklist 项**，而不是 review-driven 补漏。
