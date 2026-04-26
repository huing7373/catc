---
date: 2026-04-26
source_review: codex review on Story 4-4-token-util round 2 (file: /tmp/epic-loop-review-4-4-r2.md)
story: 4-4-token-util
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — checked-in dev config 必须能直接跑 + 部署文档必须与新增配置项同步

## 背景

Story 4-4（JWT token util）round 1 fix-review 把 `auth.token_secret` 在 `server/configs/local.yaml` 留空，理由是"密钥不入仓库 YAML，让 auth.New fail-fast"。但这把"安全防御"做过头了：仓库里 checked-in 的 local.yaml **本来就是给本地开发 / iOS simulator 联调用的**，留空导致 `./build/catserver -config server/configs/local.yaml` 启动立刻 exit 1，需要先 `export CAT_AUTH_TOKEN_SECRET` 才能跑 —— 而 `server/README.md` 没说这件事。fresh clone 体验断了，CI / dev 工作流断了。round 2 codex review [P2] 拦下。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | local.yaml `auth.token_secret` 留空让默认启动路径失败 | medium (P2) | config | fix | `server/configs/local.yaml`, `server/README.md`, `server/internal/infra/config/loader_auth_integration_test.go` |

## Lesson 1: checked-in dev config 必须能直接跑（不依赖额外 env）+ 新增 fail-fast 字段必须同步部署文档

- **Severity**: medium (P2)
- **Category**: config
- **分诊**: fix
- **位置**: `server/configs/local.yaml:27`（原），`server/README.md`

### 症状（Symptom）

仓库 checked-in 的 `server/configs/local.yaml` 把 `auth.token_secret` 留为 `""`，理由是"避免密钥入仓"。结果：

1. fresh clone repo + 跑 `./build/catserver -config server/configs/local.yaml` → 立刻 `os.Exit(1)`，错误信息 `auth: secret is empty (set CAT_AUTH_TOKEN_SECRET or auth.token_secret)`
2. `server/README.md` 的 "快速启动 3 行命令" 没说要先 `export CAT_AUTH_TOKEN_SECRET=...`
3. `iphone/README.md` 的 simulator 联调 runbook 用同一条 server 启动命令，也没提 secret env 要求
4. CI / 既有 dev 工作流（已配 MySQL 但没配 secret env）全部断了

### 根因（Root cause）

**把"密钥不入仓"原则套到了 dev 配置上，没区分 dev 和 prod 的语义**：

- prod 配置确实不能入仓 secret（K8s Secret / Vault 注入是正路）
- 但 **checked-in `local.yaml` 是 dev fixture**，不是 prod artifact —— 它的核心约定是"fresh clone 直接能跑"
- 把 prod 的"secret 不入仓"原则机械套到 dev fixture 上，等于把 fixture 变成"半成品"，破坏了它作为"开发友好默认"的契约

**部署文档同步遗漏**：

- 新增了 `auth` 配置块（YAML）+ `CAT_AUTH_TOKEN_SECRET` env override，但 README 的"环境变量覆盖"表 / "字段说明"表没补
- "fail-fast" 是好特性，但**必须**配套"默认值能让 fail-fast 不触发"（dev fixture）+ "文档教用户怎么避开 fail-fast"（prod 用 env）

### 修复（Fix）

**a) `server/configs/local.yaml`**：填一个 dev-only 语义化默认 secret + 加显眼注释

```yaml
# === DEV ONLY — production MUST override via CAT_AUTH_TOKEN_SECRET env ===
# 这里填的是 **本地开发专用** 的语义化默认 secret（≥32 字节，明显 dev 用途）。
# 目的：fresh clone 后 `./build/catserver -config server/configs/local.yaml`
# 可以**直接跑起来**，不需要先 export env。
# ...
token_secret: "local-dev-secret-do-not-use-in-prod-32bytes"
```

**关键选择**：用 `"local-dev-secret-do-not-use-in-prod-32bytes"` 这类语义化字符串而**不**用真随机 hex —— 真随机值看起来像生产密钥，运维 / secret scanner 可能误判；语义化字符串一眼看出是 dev 占位，被生产环境误用的成本极低。

**b) `server/README.md`**：补三处文档

1. "快速启动 → 坑提醒" 加一条："local.yaml 已带 dev-only 默认 JWT secret，fresh clone 直接跑即可；生产必须用 env 覆盖。"
2. "配置 → 字段说明" 表加 `auth.token_secret` / `auth.token_expire_sec` 两行
3. "配置 → 环境变量覆盖" 表加 `CAT_MYSQL_DSN` / `CAT_AUTH_TOKEN_SECRET` 两行（顺带把 4-2 的 mysql DSN env 也补上 —— 上一个 review 也没补这行）

**c) 新增回归测试** `server/internal/infra/config/loader_auth_integration_test.go`：

```go
// TestLoad_RealLocalYAML_AuthNewSucceeds 锁定 "checked-in dev config 必须能直接跑" 契约。
// 走完整链路 loader.Load → config.AuthConfig → auth.New → Sign/Verify smoke test。
// 未来谁把 local.yaml 的 token_secret 改回空串 / < 16 字节，CI 立即拦。
```

不复制 yaml fixture（一旦 fixture 与 source-of-truth 漂移就废了），直接读相对路径 `../../../configs/local.yaml`；显式 unset `CAT_AUTH_TOKEN_SECRET` env 防 host 环境覆盖；走完整 bootstrap 链路 + Sign/Verify smoke test。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **修改 `*/configs/local.yaml`（或任何 checked-in dev fixture）+ 引入新的 fail-fast 必填字段** 时，**必须**为该字段填入"显式标记 dev-only 的可工作默认值"，并**同步更新** README 的"字段说明 + env override + 快速启动坑提醒"三处。
>
> **展开**：
>
> - **"密钥不入仓" 原则**适用于 **prod / staging 配置**（这些通常不 checked-in，或入仓时要走 SOPS / sealed-secrets 等加密）。**不**适用于 checked-in 的 `local.yaml` —— 后者的契约是 "fresh clone 直接跑"，不能依赖额外 env / SOP。
> - dev-only 默认值的命名要**自我标识**：用 `"local-dev-secret-do-not-use-in-prod-32bytes"` 这类含 "do-not-use-in-prod" 的语义化字符串，**不**用真随机 hex（hex 看着像生产密钥，secret scanner / 运维可能误判 → 反而带来安全摩擦）。
> - 引入新的 fail-fast 字段（如 `auth.token_secret` / `mysql.dsn`）时，**强制 sweep 三处文档**：
>   1. `local.yaml` 字段是否填了能让 fail-fast 不触发的默认值
>   2. README 的"字段说明"表 / "环境变量覆盖"表是否同步加行
>   3. README 的"快速启动"段是否还能 fresh clone 一键跑（必要时加坑提醒）
> - **回归测试**：每个 checked-in dev config 都应有 "loader.Load + 业务模块 New 必须不报错" 的集成测试，**直接读 source-of-truth 文件**而不是复制 fixture，让漂移立刻被 CI 拦下。
> - **反例**：
>   - 反例 A：`token_secret: ""` + 注释 "留空让 fail-fast" + README 不提 env 要求 —— fresh clone 启动就死，dev 体验断
>   - 反例 B：`token_secret: "f3a9c2b1..."`（真随机 hex）+ 没注释 —— 看起来像生产密钥，secret scanner 命中后无法判断要不要拦
>   - 反例 C：填了 dev secret 但 README 不更新 env override 表 —— 生产部署时找不到怎么覆盖，被迫读源码
>   - 反例 D：填了 dev secret 但没加回归测试 —— 下一个 reviewer 又会基于"密钥不入仓"原则把它改回空串

## Meta: 本次 review 的宏观教训

**dev fixture 不是 prod 配置**：在做 "secret 防漏" 类防御性设计时，要先问 "这是 checked-in 的 dev fixture 吗"。如果是，"安全防御" 不能以"破坏开发者一键启动体验"为代价。dev 友好和 prod 安全应该用**配置层级**（dev fixture 默认值 + prod env 覆盖）解决，**不是**用"统一留空让所有人 fail-fast"解决。

这是和 [`2026-04-26-config-env-override-and-gorm-auto-ping.md`](2026-04-26-config-env-override-and-gorm-auto-ping.md) "infrastructure 接入必须配齐 env override" 互补的另一面：env override 是"prod 路径"，dev 默认值是"dev 路径"，两条都要齐才算配置层完整。
