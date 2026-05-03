---
date: 2026-05-04
source_review: codex review of Story 7.3 r6 (`/tmp/epic-loop-review-7-3-r6.md`)
story: 7-3-post-steps-sync-接口-累计差值入账-service
commit: be64bc3
lesson_count: 2
---

# Review Lessons — 2026-05-04 — 步数 sync r6：reset 与"截断+乱序"二选一的产品权衡 + prod 配置覆盖必须靠 env var 强制（不能只靠"运维记得"）

## 背景

Story 7.3 步数 sync 防作弊 / 容错防御已经走完五轮 review（r1 → r2 → r3 → r5 → r6）。
r6 review 抓出 r5 的 maxReported clamp 修复又**重新破坏**了 r3 修的 reset 场景：
- r5：基线改用 `MAX(client_total_steps)` → 截断+乱序场景对了
- r5 的副作用：HealthKit reset 之后用户走的步数永远 < 历史高水位 → server 永久 0 入账

经过分析发现：**reset** 与**"截断+乱序"组合**这两个 corner case 在概念上**无法用单一 server 端规则
同时满足**（除非引入 client 端额外信号如 sync_seq / device_id）。必须做产品权衡。

r6 同时抓出第二条独立的 P2：prod 环境必须用契约钦定默认 cap (5000/50000)，但代码里
完全没机制强制（仅靠开发者读文档的纪律）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | r5 max-reported clamp 又破坏 reset 修复 → 选 reset 优先回退 r3，接受截断+乱序 known limitation | high (P1) | architecture | fix | `server/internal/service/step_service.go` + 测试 + repo doc |
| 2 | prod 配置覆盖未强制 → 加 CAT_ENV gate（safe-by-default：未注入即 prod 严格） | medium (P2) | config | fix | `server/internal/service/step_service.go` + `cmd/server/main.go` + `internal/app/bootstrap/router.go` + `configs/local.yaml` |

修复：2 / 延期：0 / 不修：0

## Lesson 1: 两个 corner case 在概念上无法用单一规则同时满足时，做**产品权衡**而不是叠加更多防御层

- **Severity**: high (P1)
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/service/step_service.go` SyncSteps step (2)；测试文件同步删改

### 症状（Symptom）

r5 review 引入 maxReported clamp（`rawDelta = clientTotal - MAX(client_total_steps)`）修了
"截断 + 乱序" 组合下的小幅多算。但 r6 review 发现这破坏了 r3 修的 HealthKit reset 场景：

```
sync 1: clientTotal=250  → accepted=250；DB 写 client_total=250
sync 2: clientTotal=100  → reset；倒退 rawDelta=0；accepted=0；client_total=100
sync 3: clientTotal=105  → reset 后又 5 步
                          - r3 (latest 基线): rawDelta = 105-100=5；SUM 兜底 250+5>105 → adjusted=0
                          - r5 (maxReported): 105 ≤ MAX(250) → rawDelta=0
                          两者都 0 ✓
sync 4: clientTotal=300  → 用户走出超过历史高水位
                          - r3 (latest 基线): rawDelta = 300-105=195；SUM 兜底 250+195>300 → adjusted=50 ✓
                          - r5 (maxReported): 300 > MAX(250) → rawDelta=50；SUM 250+50=300 不>300 → 50 ✓
                          两者都 50 ✓

但 sync 3'.5: clientTotal=110 (user 慢慢走，还没超过 250)
                          - r3 (latest 基线): rawDelta = 110-105=5；SUM 兜底 → 0（先削回 0，等超过 250 才能入账）
                          - r5 (maxReported): 110 ≤ 250 → rawDelta=0；**永远 0** 直到超过 250

实际场景：reset 后用户当天剩余几小时只走了 100-200 步增量，永远小于 reset 前的 250 →
r5 路径下用户**整天剩余时间 server 都不入账**，看到步数停滞 → 严重 UX 退化
```

### 根因（Root cause）

**r5 与 r3 是真冲突的两条规则，不是"叠加"关系**：

| 场景 | r3 (latest 基线) | r5 (MAX 基线) |
|---|---|---|
| 常规递增 | ✓ | ✓ |
| 倒退/重复 | ✓ | ✓ |
| 乱序到达（无截断） | SUM 兜底削回，✓ | ✓ |
| **截断 + 乱序组合** | **✗ 小幅多算** | ✓ |
| **HealthKit reset** | ✓ 基线跟最近走 | **✗ 永久卡死历史高水位** |

注意：单一 server 端规则的能力有限 —— 它只能看到 sync_log 里 `(client_total_steps, accepted_delta_steps)`
两个数字。**它无法分辨"clientTotal=8000 出现在 maxReported=10000 之后"是**：

- **(a) 乱序到达**：旧数据延迟到达，应该当作旧数据
- **(b) 真实 reset**：device 真的回到 8000 了，应该当作新基线

要分辨 (a) vs (b) 必须有**额外信号**（client 提交 sync_seq / device_session_id /
sample_uuid 等），否则**任何 server 端规则都是在 (a)/(b) 二者间做 trade-off**。

r3 的"latest 基线 + SUM 兜底"对 (b) 友好（基线跟最近走）但对 (a) 略宽松（小幅多算）；
r5 的"MAX 基线"对 (a) 友好但对 (b) 不可用（永久卡死）。**没有第三条 server-only 规则
能同时满足两者**——加多少防御层都解决不了。

### 修复（Fix）

**1. 回退 r5**：service.SyncSteps step (2) 改回 r3 状态

```go
// 旧 (r5 maxReported clamp)：
maxReported, _ := s.stepSyncLogRepo.MaxClientTotalStepsByUserAndDate(...)
if latest == nil {
    rawDelta = int64(in.ClientTotalSteps)
} else if in.ClientTotalSteps > maxReported {
    rawDelta = int64(in.ClientTotalSteps - maxReported)
} else {
    rawDelta = 0
}

// 新 (r6 = r3)：
if latest == nil {
    rawDelta = int64(in.ClientTotalSteps)
} else if in.ClientTotalSteps > latest.ClientTotalSteps {
    rawDelta = int64(in.ClientTotalSteps - latest.ClientTotalSteps)
} else {
    rawDelta = 0
}

// SUM 兜底**保留**（r3 第二层防御）
```

**2. 保留 repo 方法 `MaxClientTotalStepsByUserAndDate`**：service 暂不调用，但留着供未来
"client 端 sync_seq / device_id 信号"接入时复用（删了再加成本更高；这是 known future work
的物质基础）。doc 注释更新：`r6 service 暂未调用；r5 曾用作 maxReported clamp，r6 选 reset
优先路径回退到 r3，本方法保留供未来"区分 reset 与乱序"额外信号接入时复用`。

**3. 测试同步**：
- 删除 r5 测试 `TestStepService_SyncSteps_TruncationPlusOutOfOrder_MaxReportedClampPreventsOverAccrual`
  + 集成测试同名镜像（不再断言"完美防御"）
- 改写为 `KnownLimitation_TruncationPlusOutOfOrder_MayOverAccrue_ByDesign`，断言 accepted=4000
  （by-design 多算 2000，文档化 known limitation；若未来重蹈 r5 覆辙改回 maxReported clamp
  → accepted 变 2000 → 此 case 立即挂，提示决策被无意改回 r5）
- **保留** `TestStepSyncLogRepo_MaxClientTotalStepsByUserAndDate_HappyPath`（repo 方法仍在，要测）
- HealthKitReset 三个 subtest 不变（r3 / r5 mock 都对，r6 自然过；本 case 是 reset 行为锁定哨兵）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **遇到"两个 corner case 用单一规则无法同时满足"** 时，**必须**先识别是
> "需要额外信号"还是"两条规则真冲突"，**禁止**继续叠加防御层 / 改 SQL ORDER BY 试图找"完美方案"。
>
> **展开**：
>
> - **触发条件**：调过 ≥ 2 轮 review 后，每次修一个场景就破坏另一个场景（"修一头漏一头"反复出现），
>   且两个场景在不同 review 之间互相对立
>
> - **强制识别步骤**：
>   1. 列出**所有相关不变式**（用纸笔列；不变式 = 业务上要求 always 成立的等式 / 不等式）
>   2. 用真值表标记**每条规则在每个场景下是否满足每个不变式**
>   3. 如果发现"任意单条规则都至少违反一个不变式" → **承认这是产品权衡问题，不是技术 bug**
>   4. **明确产品权衡的方向**（哪个场景常见 / 哪个 UX 影响更严重 / 哪个有第三方兜底）
>   5. 如果想要"两全其美"必须**引入额外信号**（input 字段 / 外部系统同步），而非更复杂的 server 端规则
>
> - **决策记录纪律**：选定权衡方向后，**必须**在代码注释 + lesson 文档双写：
>   - "为什么选这个方向"（产品理由 + 损失上限）
>   - "另一方向的损失"（方便未来重新评估）
>   - "未来若要修这个 known limitation 的唯一路径"（避免 Claude 再次试图用纯 server 端规则微调）
>   - **测试要 by-design 锁定**：用 `KnownLimitation_xxx_ByDesign` 命名 + 断言"接受的多算/少算值"，
>     若未来谁试图"修复"这个 known limitation 反而让 case 挂，反向触发讨论
>
> - **反例**：
>   - r1→r2→r3→r5→r6 这五轮就是反例 —— 每轮都试图找"完美 server-only 规则"，r6 才停下来承认
>     需要 trade-off。理想情况应该 r3 阶段就识别"reset 与乱序无法同时完美满足，必须 trade-off"，
>     然后 r4 直接做产品决策不再绕弯
>   - 不要把"已知不完美"伪装成"还差一层防御就完美" —— defense-in-depth 是为对抗未知 / 复合攻击，
>     不是为掩盖单一规则能力不足
>   - 不要为了"看起来对所有 case 都行"加上互相矛盾的两条规则（如 r5 想叠加 maxReported clamp + 保留
>     latest 基线 fallback），那只会把 bug 往更隐蔽的角落塞
>
> - **触达"需要 client 信号"的判定标志**：
>   - 同一个 server 状态在不同业务场景下**应该被不同解读**（如 "clientTotal 下降" 是 reset 还是乱序？）
>   - 仅靠 server 表里现有列无法分辨上述场景
>   - → 必须引入 client 提交的额外字段（sync_seq / device_session_id / sample_uuid 等）

---

## Lesson 2: "prod 必须用 X" 的契约只在文档里钦定 = 没钦定；必须用 env var / build tag 在代码里强制

- **Severity**: medium (P2)
- **Category**: config
- **分诊**: fix
- **位置**: `server/internal/service/step_service.go` NewStepService 加 envName 参数；`cmd/server/main.go` 读 `CAT_ENV` env；`internal/app/bootstrap/router.go` Deps 加 EnvName 字段；`configs/local.yaml` 顶部加注释

### 症状（Symptom）

V1 §6.1.4 + epics.md §Story 7.3 + StepsConfig 注释都钦定："prod 部署必须用默认值 (5000/50000)"。
但代码 `NewStepService` 无条件接受 YAML 任何正值覆盖默认 cap。运维若误把 dev YAML（含 fixture
`single_sync_cap: 100`）推到 prod，server 启动正常，但当日封顶从 50000 降到自定义值 → 单实例
契约漂移、跨端阈值不一致、用户体验异常 —— **且无声**，直到 client 端用户报障才发现。

### 根因（Root cause）

把"运维纪律"和"代码契约"混为一谈。文档说"prod 必须用 X"是**chant**，不是**enforcement**。
任何依赖"开发者/运维记得读 YAML 顶部注释 / V1 §6.1.4"的契约在多实例长期部署下都会漂移。

类似教训：4.4 review 历史上的 `auth.token_secret` 也是这模式 —— "生产必须用 env 注入"靠
`auth.New` 在启动期 `len(secret) < 16 → error` 强制（不靠"开发者记得 export 环境变量"）。
StepsConfig 的 cap 应该按同样模式做。

### 修复（Fix）

**1. service 层 prod gate**：`NewStepService` 新增 `envName` 参数

```go
func NewStepService(..., cfg config.StepsConfig, envName string) StepService {
    ...
    envLower := strings.ToLower(strings.TrimSpace(envName))
    isOverrideAllowed := envLower == "dev" || envLower == "staging" || envLower == "test"
    if !isOverrideAllowed && (cfg.SingleSyncCap > 0 || cfg.DailyCap > 0) {
        panic(fmt.Sprintf("step service: prod env (CAT_ENV=%q) must use default caps; "+
            "got single_sync_cap=%d daily_cap=%d (V1 §6.1.4 钦定 5000/50000；"+
            "dev/test 覆盖必须 export CAT_ENV=dev|staging|test)",
            envName, cfg.SingleSyncCap, cfg.DailyCap))
    }
    ...
}
```

**关键设计选择**：**safe-by-default**（未注入 / 未知值都按 prod 严格策略）：
- envName="" → strict（漏配 CAT_ENV 也防住）
- envName="prod" / "production" → strict
- envName="typo-name" / "qa" / 任何未识别值 → strict（避免 typo 让覆盖意外通过）
- 只允许显式 "dev" / "staging" / "test" / "DEV" / "Dev" 等大小写变体（容错，但精确匹配 token）

**2. main.go 注入**：读 `CAT_ENV` env var 透传

```go
envName := os.Getenv("CAT_ENV")
if envName == "" {
    envName = "prod"
}
deps := bootstrap.Deps{..., EnvName: envName}
```

**3. bootstrap.Deps 加字段** + router.go 透传给 service.NewStepService

**4. configs/local.yaml 顶部 + StepsConfig doc + service 层 doc 更新**：标明 prod gate 机制 +
dev/staging/test 必须显式 export CAT_ENV 才能覆盖

**5. 单测覆盖**：`TestStepService_NewStepService_ProdEnv_RejectsYAMLCapOverride` 11 个 subtest：
- ProdEnv + 任一正值 cap → panic（含 panic msg 内容验证）
- ProdEnv + 全 zero-value → OK
- "" / unknown env / "production" → 都按 prod 严格策略
- "dev" / "DEV" / "Dev" / "staging" / "test" → 允许覆盖

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"prod 必须用 X / 不能用 Y"的契约文档** 时，**必须**同时在
> 代码里加 fail-fast 强制（env var gate / build tag / startup validator），**禁止**只靠文档纪律。
>
> **展开**：
>
> - **触发条件**：任何 config 字段 / 业务参数文档说"prod 必须 …" / "生产环境只能 …" / "默认值
>   是契约一部分不能改" —— 这些**全部**需要代码层强制
>
> - **强制方式选项**（按推荐度排序）：
>   1. **env var gate**：startup 读 `CAT_ENV` / `APP_ENV` 类 env，prod 严格分支 panic（推荐）
>      - 优点：简单 / 容易测 / 部署侧只需一个 export
>      - 注意：必须 **safe-by-default**（未注入 / 未知值按 strict 策略，不按 dev）
>   2. **build tag gate**：用 `-tags=prod` 编译时禁用某些路径
>      - 优点：100% 静态保证，不可能运行时绕过
>      - 缺点：需要 build pipeline 改造、双 binary 维护
>   3. **startup config validator**：config 包加 Validate() 方法，main.go 启动期调
>      - 适合 cross-field 校验（如 "如果 X 启用则 Y 必填"）
>
> - **safe-by-default 原则**：env var gate 的默认行为必须是 **strict**：
>   - 未注入 env 视为 prod
>   - 未识别的 env 值（typo / 历史值 / 拼写错）视为 prod
>   - 只精确匹配 allowlist 才视为允许覆盖
>   - 反向（默认宽松，需要显式 prod 才严格）= dev YAML 流到 prod 静默漂移的灾难配方
>
> - **panic 消息纪律**：panic msg 必须包含
>   1. 字段名（哪个 cap / 哪个 secret）
>   2. 实际值（让运维知道哪里出问题）
>   3. 期待值（"必须用默认值 5000/50000"）
>   4. 修复指引（"export CAT_ENV=dev 或删除 YAML 字段"）
>
> - **测试纪律**：每条 prod gate 至少覆盖
>   - prod env + 违规 → panic（含 msg 内容验证）
>   - prod env + 合规 → OK
>   - 空 env / 未知 env → 按 prod strict
>   - allowlist 内每个值 → 允许覆盖
>
> - **反例**：
>   - 只在 YAML 顶部写 `# prod 必须用默认值` 注释 → 无任何强制（本次 r6 之前的状态）
>   - 默认宽松 + 需要 `CAT_ENV=prod` 才严格 → typo / 漏配都让覆盖通过
>   - 只 log.Warn 不 panic → 启动正常，运维看不到 warn → 与无强制等价
>   - panic msg 只说 "invalid config" → 运维不知道改哪里

---

## Meta: 本次 review 的宏观教训

r1 → r2 → r3 → r5 → r6 这**五轮 review** 反映了一个深层问题：**"持续修复局部 bug" 与
"承认设计冲突需要权衡" 的边界没识别清楚**。前四轮每次都尝试找一个新 SQL ORDER BY / 新 SUM
判断 / 新 MAX clamp 试图"完美解决"，r5 引入 maxReported clamp 时甚至以为是"两层防御叠加"
的胜利，r6 才识别出来 r5 与 r3 真在冲突。

**正确的 mental model 应该在 r3 阶段就建立**：

> "两条规则在不同 corner case 下互相对立，单一 server-only 规则不可能同时满足，
> 必须做产品权衡，要么接受其中一个 case 的有限损失，要么引入 client 端额外信号。"

如果 r3 阶段就承认这点，r4/r5/r6 直接做产品决策（reset 优先 + 文档化"截断+乱序"known
limitation），不会浪费三轮 review。

> **一句话规则**：未来 Claude 调过 ≥ 2 轮 review 还在"修一头漏一头"时，**强制停下来**
> 列真值表识别"这是产品权衡问题还是技术 bug"。如果是前者，**禁止**继续找"完美技术方案"，
> 直接做权衡决策 + 文档化 known limitation + by-design 测试锁定。
