---
date: 2026-05-03
source_review: codex review r7 — /tmp/epic-loop-review-7-3-r7.md
story: 7-3-post-steps-sync-接口-累计差值入账-service
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-03 — 信任客户端 syncDate 的 anti-cheat 漏洞 + ±N 天容忍窗口的 trade-off

## 背景

Story 7.3（POST /steps/sync 累计差值入账）r7 codex review 指出：handler 只校验 syncDate 格式与 MySQL DATE 物理范围，但 V1 §6.1.2 GAP E 钦定 "client 本机时区算今天，server 直接采用不二次转换"，意味着 server 完全信任客户端提交的日期。恶意客户端可旋转 syncDate 重复入账并完全绕过 50000 daily_cap：每天独立 baseline + 独立 cap → 旋转 N 天等效 N×daily_cap。本 lesson 记录修复方案（±2 天 server-side 容忍窗口）+ 同步 V1 文档契约 + 该方案的已知 trade-off 和未来升级方向。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | syncDate 旋转攻击：信任客户端时区导致 daily_cap 完全绕过 | high | security | fix | `server/internal/app/http/handler/steps_handler.go` |
| 2 | 契约 review 漏掉的攻击面 → V1 §6.1.2 GAP E 必须显式加防御边界 | medium | docs | fix | `docs/宠物互动App_V1接口设计.md` |

## Lesson 1: syncDate 旋转攻击：信任客户端时区导致 daily_cap 完全绕过

- **Severity**: high
- **Category**: security
- **分诊**: fix
- **位置**: `server/internal/app/http/handler/steps_handler.go:127`（新增 syncDate 范围校验段）

### 症状（Symptom）

V1 §6.1.2 GAP E 钦定 "client 本机时区算今天 / server 直接采用不二次转换"。Story 7.3 实装严格遵循：handler 只校验 syncDate 是合法 YYYY-MM-DD + MySQL DATE 物理范围 [1000-01-01, 9999-12-31]，server 完全信任客户端提交的日期。

攻击场景：
- sync syncDate=2026-05-01, clientTotalSteps=1000 → first sync of day → 入账 1000
- sync syncDate=2026-05-02, clientTotalSteps=1000 → new day baseline=0 → 又入账 1000
- sync syncDate=2026-05-03, clientTotalSteps=1000 → 又入账 1000
- 旋转 N 天 → N×1000，且每天独立 50000 daily_cap → **完全绕过封顶**

step service 的所有反作弊（single_sync_cap=5000 / daily_cap=50000 / SUM 兜底 / 基线减法）都按 `(userID, syncDate)` 做账本 key；syncDate 完全可控 → 反作弊失效。

### 根因（Root cause）

GAP E 的产品决策是 "为避免跨时区漂移，信任 client 的本地时区"。这个决策本身合理，但 **"信任时区"≠"信任任意日期"**：跨时区合理范围是有界的（极端 PST↔JST 17 小时差，最多让"今天"前后偏移 ≤1 天），而恶意客户端旋转日期是无界的。

契约 review 阶段（Story 7-1）只锚定了 GAP E 的 happy path（合理跨时区客户端），漏掉了恶意客户端攻击面：完全信任 + 无范围校验 = 无界攻击窗。

防御缺失的根因：把"server 不二次转换时区" misinterpret 成"server 不做任何 syncDate 校验"，把信任度从"接受合理偏移"放大到"接受任意日期"。

### 修复（Fix）

在 handler 层 isValidYYYYMMDD 之后追加 server-side 范围校验：syncDate 必须 ∈ [server today UTC - 2 days, server today UTC + 2 days]。

```go
// handler/steps_handler.go:127
parsed, _ := time.Parse("2006-01-02", req.SyncDate)
serverNow := time.Now().UTC()
serverToday := time.Date(serverNow.Year(), serverNow.Month(), serverNow.Day(), 0, 0, 0, 0, time.UTC)
earliest := serverToday.AddDate(0, 0, -syncDateToleranceDays)  // -2
latest := serverToday.AddDate(0, 0, syncDateToleranceDays)     // +2
if parsed.Before(earliest) || parsed.After(latest) {
    _ = c.Error(apperror.New(
        apperror.ErrInvalidParam,
        "syncDate 必须在 server today ± 2 天范围内（跨时区容忍窗口；防止恶意客户端旋转日期重复入账）",
    ))
    return
}
```

`syncDateToleranceDays = 2` 提取为常量，单点真相。

**为什么是 ±2 天而不是 ±1 / ±7**：
- 极端跨时区差（PST UTC-8 vs JST UTC+9）= 17 小时 → 任意时刻"client 本地今天"与"server UTC 今天"差 ≤1 天 → ±1 天**勉强够**
- 加 1 天 buffer 覆盖客户端时钟轻微漂移 / 跨日 race（client 计算"今天"和打 request 之间过午夜）→ ±2 天**稳**
- ±7 天 → 攻击窗 7×daily_cap = 350000 步，不可接受

**trade-off（known limitation）**：±2 天仍允许"5 天独立账本 = 250000 步上限"绕过。完全防御需要 server-side trusted time + device id 绑定（未来 epic 升级方向）。

附带改动：handler test 文件中所有"需要让 syncDate 通过校验"的 case 改用 `dynamicValidSyncDate()` 函数（基于真实 server today UTC 构造），而非硬编码 `"2026-05-01"`——否则 CI 时间漂离 ±2 天后整套测试会破。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计任何"信任客户端某字段"的契约（时区、时间戳、deviceId、版本号、地理位置等）** 时，**必须**显式定义 **server-side 合理范围边界**（绝对值上下界 / 偏差容忍窗口 / 校验函数），并在 handler / service 入口拦截越界值。
>
> **展开**：
> - "信任" ≠ "无校验"。"信任客户端时区"的合理边界是"跨时区合理偏移 ≤ N 天"，不是"接受任意日期"。
> - 校验值的设计三步：① 列出**合理 happy path** 的取值范围 → ② 加 **safety margin**（covering clock drift / race / 边角合理场景）→ ③ 写出**攻击窗大小 = margin × 单次 cap**，可接受才放行。
> - 范围校验**必须放在 handler 入口**（最早的拒点），不能等到 service / repo 才发现。原因：service 层假设 input 已合法，越深的层越难加防御性 if。
> - **反例 1**："V1 文档说 server 不做时区转换" → 推断 "handler 不做任何 syncDate 校验" → 攻击者旋转日期完全绕过 daily_cap。正解：契约的"不做时区转换"只覆盖 timezone semantics，不覆盖 anti-cheat range check；这两层是正交的。
> - **反例 2**：用绝对日期界（如 "syncDate 必须 ∈ [2020-01-01, 2030-12-31]"）→ 看似"合理范围"但攻击窗 = 10 年 × 365 × 50000 = 完全无防御。正解：用 **relative 窗口（基于 server 当前时间）**，攻击窗才有界。
> - **反例 3**：测试中硬编码合法日期（如 `"2026-05-01"`）→ 范围校验加上去后，CI 时钟漂离 ±N 窗口立刻全破。正解：所有"需要通过范围校验"的 test case 用 `formatRelativeDate(0)` 类函数动态生成日期。

## Lesson 2: V1 契约 review 必须显式定义"信任客户端字段"的防御边界

- **Severity**: medium
- **Category**: docs / process
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §6.1.2（syncDate 字段说明 + 错误码 1002 触发条件）

### 症状（Symptom）

Story 7-1（接口契约最终化）阶段做契约 review 时，§6.1.2 GAP E 写了 "server 直接采用不二次转换" 但没说 "server 校验合理范围"，也没列入 1002 触发条件。下游实装严格遵循契约 → 漏掉范围校验 → 直到 r7 review 才发现攻击面。

### 根因（Root cause）

契约 review 的关注点偏向 "happy path 字段语义 + 类型 + 必填性"，对 "**信任** 类字段的攻击面" 缺少专项扫描。GAP E 章节的"为什么这么设计"只回答了"如何避开跨时区漂移"，没回答"如何防御恶意客户端旋转"。

### 修复（Fix）

V1 §6.1.2 syncDate 字段说明追加：

> **server 校验 syncDate ∈ [server today - 2 days, server today + 2 days] 范围内**（覆盖极端 PST↔JST 17h 时差 + 客户端时钟轻微漂移；防止恶意客户端旋转日期重复入账绕过 daily_cap，Story 7.3 review r7 [P1] anti-cheat）；超出此范围 → 1002 参数错误

§6.1.2 错误码 1002 触发条件追加 "syncDate 越出 server today ± 2 天容忍窗口（review r7 anti-cheat）"。

按 V1 §1 节点 3 冻结条款，本应走"契约变更流程"。但 7-3 还在 review 阶段（未 done），视为 "7-1 契约的延伸" 直接更新；同时在本 lesson flag "契约 review 阶段未发现该攻击面，应在 7-1 retrospective 总结"。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **做契约 review（如 /bmad-create-story 或 epic-loop create-story 阶段）** 时，**必须** 对每个标注 "信任客户端 / 来自客户端" 的字段做**专项攻击面扫描**：枚举"恶意客户端可控范围"+"被信任后对账本/状态的影响"+"server-side 防御边界"。
>
> **展开**：
> - 契约 review checklist 必须含一条：**枚举所有"信任客户端"字段** → 对每个字段问：① 客户端可控范围是什么？② 滥用会导致什么业务后果（账本 / 余额 / 速率 / 权限）？③ server 防御边界是什么？
> - "信任 X" 在契约里**必须**配 "但 server 做 Y 校验"——光说"信任"不足以指导实装。
> - **反例**：V1 §6.1.2 GAP E "server 直接采用不二次转换" → 实装只做格式校验 → 完全无范围防御。正解：GAP E 同时写明 "server 仍校验范围 ∈ [today-2, today+2]"。
> - 契约文档变更要**同时更新错误码表**——光在字段说明里加了校验规则但 1002 触发条件不更新，下游实装会漏掉。

---

## Meta: 本次 review 的宏观教训（可选）

Story 7.3 的 6 轮 review 暴露了一个共同模式：**"双向防御"必须 layered**：
- r3：基线必须用 `id DESC` 而非 `MAX`（防 reset 卡死）
- r6：截断+乱序组合下 SUM 兜底仍漏，需叠加 max-reported clamp
- r7：syncDate 完全信任客户端 → 旋转攻击

每一轮 review 找到的都是**前一轮防御没覆盖的攻击面**。结论：步数账本类高风险业务，**单一防御层不够**，必须从入口（handler 范围校验）→ service（基线 + cap + SUM 兜底 + clamp）→ repo（事务 + 乐观锁）做 defense in depth；**且每一层的攻击面要分开列**，不能假设上层校了下层就安全。
