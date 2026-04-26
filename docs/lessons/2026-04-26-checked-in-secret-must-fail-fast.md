---
date: 2026-04-26
source_review: codex review on Story 4-4-token-util round 3 (file: /tmp/epic-loop-review-4-4-r3.md)
story: 4-4-token-util
commit: c9396aa
lesson_count: 1
---

# Review Lessons — 2026-04-26 — secret 字段必须空字符串 + fail-fast，不能 checked-in dev fallback（即使加了警告注释）

## 背景

Story 4-4（JWT token util）round 2 fix-review **走错方向**。

- round 1 codex 拦下"`token_secret: ""` 让 fresh clone 启动 fail-fast"，要求"checked-in dev config 必须能直接跑"
- round 2 fix 把 `token_secret` 从空串改成 `"local-dev-secret-do-not-use-in-prod-32bytes"` 让 fresh clone 一步上手
- round 3 codex [P1] 反向拦下：staging / prod 环境只要误用同一个 checked-in `local.yaml`（而 `LocateDefault()` + README 都推荐这条路径）+ 漏注入 `CAT_AUTH_TOKEN_SECRET` env → server 用**公开仓库已知** secret 启动 → 任何看过 repo 的人能伪造 HS256 token 绕过 auth
- 这是 round 2 的反向纠偏：回退 yaml 改动到空串，但**保留并强化** README 引导

本 lesson 不是"对一处 yaml 字段值的小调整"，而是钉死一条普适规则 + 一条元教训（review trade-off 决策原则），避免未来 Claude 第四次踩同一个跷跷板。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | checked-in `local.yaml` 的 `auth.token_secret` 是 fallback 值，prod 误用 = 公开 secret = auth bypass | high (P1) | security / config | fix（round 2 的反向纠偏） | `server/configs/local.yaml`, `server/README.md`, `server/internal/infra/config/loader_auth_integration_test.go` |

## Lesson 1: secret 字段在 checked-in dev config 必须空串 + fail-fast；dev 友好让 onboarding 文档承担

- **Severity**: high (P1)
- **Category**: security / config
- **分诊**: fix（**反向纠偏 round 2**）
- **位置**: `server/configs/local.yaml:50`（原 round 2 的 `:37`）

### 症状（Symptom）

`server/configs/local.yaml` 把 `auth.token_secret` 填成 `"local-dev-secret-do-not-use-in-prod-32bytes"`（round 2 fix 加的），意图是"fresh clone 一步上手"。但：

1. `LocateDefault()`（[`server/internal/infra/config/locate.go`](../../server/internal/infra/config/locate.go)）把 `server/configs/local.yaml` 列为推荐启动入口
2. `server/README.md` "快速启动" 段也用 `-config server/configs/local.yaml` 作为 SOP
3. staging / prod 部署 SOP 如果**忘记**注入 `CAT_AUTH_TOKEN_SECRET` env（部署常见疏漏） → server 用 yaml 里 checked-in 的 `local-dev-secret-do-not-use-in-prod-32bytes` 启动
4. 这个值在 GitHub / 镜像 / 公开 fork 都能搜到 → 任何攻击者都可以用它本地 sign 一个合法 HS256 token → 绕过 auth 中间件 → 完全 access 所有受保护 API

注释里写的"do-not-use-in-prod" / "32bytes 占位" / 显眼 DEV ONLY 警告 —— **拦不住 misconfiguration**。运维只看到 server 启动成功；attacker 不读注释。

### 根因（Root cause）

**round 2 fix 把"方便 vs 安全"的 trade-off 选错方向**。

- review 的"checked-in dev config 必须能直接跑"是合理诉求（dev 友好），但 round 2 把这个诉求**完全压在配置默认值上**
- 没意识到：**checked-in fallback secret 在 prod 的攻击面 >>> dev fresh clone 多敲一行 export 的不便**
- 注释 / 命名警告（`do-not-use-in-prod`）属于**纵深防御**，**不**能作为唯一安全屏障 —— 因为运维 SOP 漂移 / 部署脚本误用 / 镜像构建路径错配等场景里，注释从来不会被读到

**元层根因**：当 finding 涉及 trade-off（"方便 vs 安全"），默认应该**优先安全**（fail-fast），让 onboarding 文档负责"方便"。代码 / 配置默认值的语义是"程序不动手就这样"，把"方便"塞进默认值等于让"程序的 default 行为"承担"用户教育"职责，错位。

**与 [`2026-04-26-checked-in-config-must-boot-default.md`](2026-04-26-checked-in-config-must-boot-default.md) 的关系**：那条 lesson 说"checked-in dev fixture 应该 fresh clone 直接跑"。**它本身没错**，错在把"直接跑"的边界画得过宽 —— 包括了 secret 类字段。本 lesson 给那条加边界条件：**非 secret 字段（http port / log level / dsn / etc.）适用，secret-like 字段（auth secret / encryption key / API token）不适用**。

### 修复（Fix）

**a) `server/configs/local.yaml`**：把 `token_secret` 改回空串 + 重写注释，明确"故意留空"+ 给 trade-off 解释

```yaml
auth:
  # === 故意留空 —— 启动前必须 export CAT_AUTH_TOKEN_SECRET 覆盖 ===
  # 这里**不**放 dev fallback secret。原因（round 3 codex review [P1] 拦下的反向纠偏）：
  #   - LocateDefault() 和 README 都把这个 yaml 路径作为推荐启动入口；
  #   - 任何 staging / prod 环境只要误用此 yaml + 漏注入 CAT_AUTH_TOKEN_SECRET env，
  #     server 就会用**公开仓库已知**的 secret 启动 → auth bypass；
  #   - 留空（""）反而**更安全**：auth.New 报错 → main.go fail-fast → 永远阻止
  #     "工作但 insecure" 的 misconfiguration 上线。
  # 关键 trade-off：fresh clone "1 步上手" vs "安全默认" → 选**安全** + README 引导。
  token_secret: ""
```

**b) `server/README.md`**：把 `export CAT_AUTH_TOKEN_SECRET` 步骤**抬到主流程"第一步"**位置（不再是次要的"坑提醒"），加 example：

```bash
# 第一步（必做，否则 server 启动 fail-fast）：导出 JWT signing secret
export CAT_AUTH_TOKEN_SECRET="$(openssl rand -hex 32)"
# 没有 openssl？任意 ≥16 字节字符串都行：
# export CAT_AUTH_TOKEN_SECRET="my-local-dev-secret-32-bytes-min"
```

加 callout 解释 trade-off：为什么 yaml 里**故意**不放 fallback secret，明确指向本 lesson。

字段说明表 `auth.token_secret` 行的 default 改成 `""`（**故意留空**）；env override 表 `CAT_AUTH_TOKEN_SECRET` 行的"备注"改成"**所有环境必走**（含本地 dev）"，移除"覆盖 dev-only 默认值"措辞。

**c) `server/internal/infra/config/loader_auth_integration_test.go`**：替换 round 2 那个 "default secret 必须能让 auth.New 通过" 的测试为**两个**测试，钉死安全 + 可工作双不变量：

```go
// (1) 钉死可工作：env 注入 secret 后整条链路通
func TestLoad_RealLocalYAML_AuthNewSucceedsWithEnvOverride(t *testing.T) {
    t.Setenv("CAT_AUTH_TOKEN_SECRET", "test-secret-32-bytes-minimum-for-test")
    cfg, err := config.Load(yamlPath)
    require.NoError(t, err)
    signer, err := auth.New(cfg.Auth.TokenSecret, cfg.Auth.TokenExpireSec)
    require.NoError(t, err)
    // + Sign/Verify 烟囱
}

// (2) 钉死安全默认：没 env 时 yaml token_secret 必须空 + auth.New 必须 fail-fast
func TestLoad_RealLocalYAML_AuthNewFailsWithoutEnv(t *testing.T) {
    t.Setenv("CAT_AUTH_TOKEN_SECRET", "")
    cfg, err := config.Load(yamlPath)
    require.NoError(t, err)
    require.Empty(t, cfg.Auth.TokenSecret)  // checked-in 必须空
    _, err = auth.New(cfg.Auth.TokenSecret, cfg.Auth.TokenExpireSec)
    require.Error(t, err)  // fail-fast
}
```

未来谁再把 `token_secret` 改回有值（不论是不是加注释 / 加 "do-not-use-in-prod" 字符串）→ 第二个 test 立刻在 CI 红，附带 lesson 路径提示 + 反向纠偏指引。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **修改 checked-in 配置文件（`*/configs/*.yaml`）+ 字段是 secret-like（signing key / API token / encryption key / DB password 等可被攻击者直接利用的字段）** 时，**必须**留空字符串 + 让上层模块 New / 启动期 fail-fast + 让 onboarding 文档（README / 部署 SOP）负责"如何注入"。**禁止**填 fallback / placeholder / "obviously dev" 字符串，即使加显眼注释。
>
> **展开**：
>
> - **secret-like 字段判定 checklist**（满足任一即视为 secret-like）：
>   - 字段名含 `secret` / `key` / `password` / `token` / `apikey` / `credential`
>   - 字段被用来签名 / 加密 / 鉴权（例如 JWT signing key、AES key、HMAC secret、OAuth client secret）
>   - 字段泄露后攻击者能直接接管系统（伪造 token / 绕过 auth / 解密敏感数据）
>
> - **非 secret 字段** (`http_port` / `log_level` / `bind_host` / `read_timeout_sec` 等) 不适用本规则，按 [`2026-04-26-checked-in-config-must-boot-default.md`](2026-04-26-checked-in-config-must-boot-default.md) 的"checked-in dev fixture 必须能直接跑"原则填默认值。
>
> - **DSN / 含密码的连接串** 介于两者之间：dev 默认值（`cat:catdev@tcp(127.0.0.1:3306)/cat`）OK，因为该用户 / 库**不存在于 prod**，prod 误用必然在 db.Ping 阶段 fail-fast；只要默认值在 prod **必然 ping 失败**，就不算 secret-like 漏洞。如果 dev 默认 DSN 可能在 prod **碰巧能连**（如 `root:root` 类弱密码），就要按本规则留空。
>
> - **trade-off 决策原则**（普适）：当 review 提出"方便 vs 安全"取舍，默认**优先安全**（fail-fast）+ 让文档 / SOP 承担"方便"。代码 / 配置默认值的语义是"程序不动手就这样"，不应该承担"用户教育"职责。
>
> - **fail-fast 必须配 onboarding 引导**：留空字符串引发 fail-fast 不是"完成"，是触发以下三件事的开始：
>   1. README "快速启动"主流程**第一步**含 `export <ENV_NAME>="..."`（不能埋在"坑提醒"次要位置）
>   2. README 含 trade-off callout 解释为什么 yaml 故意留空（指向本 lesson）
>   3. 集成测试钉死"yaml 字段必须空 + 没 env 时 fail-fast" + "有 env 时整条链路通"双不变量
>
> - **回归测试钉死方向**（不是钉死值）：
>   - 写 `require.Empty(cfg.Auth.TokenSecret)` 钉"必须空"，而不是 `require.Equal("", ...)` 钉具体值（语义更清晰）
>   - 写 `require.Error(auth.New(""))` 钉"空值 fail-fast"，而不是钉具体 error message（message 可能演化）
>
> - **反例**：
>
>   - 反例 A（round 2 走错的方向）：`token_secret: "local-dev-secret-do-not-use-in-prod-32bytes"` + 注释 "DEV ONLY" → prod 误用 yaml 时启动成功，所有 token 可被任意伪造
>   - 反例 B：`token_secret: "f3a9c2b1..."`（真随机 hex）+ 没注释 → 运维 / secret scanner 看到像生产值，无法判断要不要拦
>   - 反例 C：把 secret 留空但 README 没在主流程提示 → fresh clone 启动 fail-fast，user 不知道为什么、google 半天才找到 export
>   - 反例 D：把 secret 留空 + README 提示 export 但**没**集成测试 → 下一个 reviewer 又会基于"checked-in dev fixture 必须直接跑"原则把它改回有值（即本次 round 1 → round 2 → round 3 的来回跷跷板）
>   - 反例 E：把 secret 加进 `.env.example` 并要求用户 `cp .env.example .env` → 比 yaml fallback 略好（要求用户主动动作），但仍然把"secret 注入"耦合到一个文件操作上，比直接 export 的可见性差；prefer 直接 README 教 export
>
> - **跨字段一致性 sweep**：每次新增 fail-fast 启动检查时，sweep `internal/` 看其他启动期检查是否同样严格（不能为 misconfiguration 留"工作但 insecure"路径）。Story 4-4 当前其他 fail-fast 字段：
>   - `mysql.dsn`：dev 默认 `cat:catdev@...`，prod 必 ping 失败 → 不是 secret-like 漏洞，OK
>   - `auth.token_expire_sec`：非 secret 类，按"dev fixture 默认值"处理（604800 秒 = 7 天），OK
>   - 后续 epic 加 secret-like 字段（如 Epic 10 的 Redis password / Epic 4 的 OAuth secret）必须按本规则处理

## Meta: 本次 review 的宏观教训

**fix-review 自身的反思**：fix-review skill 把"上轮 review 的修法"作为输入，但**不能盲信上轮 review 的方向是正确的**。round 2 review 给的 finding（"checked-in dev config 必须能直接跑"）是合理诉求，但 round 2 fix 把这个诉求过度压在配置默认值上，跨过了"安全 vs 方便"的边界。round 3 review 拦下后才反向纠偏。

未来 Claude 在 fix-review 时，对每条 finding：

1. **先问该诉求的本质**：是"安全"诉求还是"方便"诉求？两者冲突时优先安全
2. **再问承担诉求的层**：该诉求应该由代码 / 配置 / 文档 / 工作流哪一层承担？默认值层只能承担"程序不动手时的状态"，不能承担"用户教育"
3. **最后看防御纵深**：注释 / 命名警告永远是**纵深防御**，不能作为唯一屏障；fail-fast / 编译期检查 / CI 拦截才是**主要屏障**

这次反向纠偏教训：**fix-review 不是 echo chamber**，每轮 fix 完都要主动反问"我这个 fix 有没有引入新的、比上轮 finding 更严重的问题"。
