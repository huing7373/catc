---
date: 2026-05-03
source_review: codex review of Story 7.3 r3 (`/tmp/epic-loop-review-7-3-r3.md`)
story: 7-3-post-steps-sync-接口-累计差值入账-service
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-03 — 步数 sync 基线综合方案：id DESC + SUM 兜底，而非"乱序 vs reset 二选一"

## 背景

Story 7.3 的 `FindLatestByUserAndDate` 基线查询经历了三轮 review：

- **r1**：`ORDER BY id DESC`（最近 INSERT 行）
- **r2**：codex 抓出乱序到达 bug —— 改为 `ORDER BY client_total_steps DESC, id DESC`（最大累计行）
- **r3**：codex 又抓出 r2 的回归 —— HealthKit reset/correction 真实场景下，最大值会**永远**作为基线，用户当日剩余时间所有步数都被算成 rawDelta=0（永久少算）

两个单一 ORDER BY 方案各踩一头，**都不对**。本轮综合方案：基线退回 `id DESC` + service 层加 SUM 兜底校验。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | r2 的 max(client_total_steps) 基线在 reset/correction 场景永久卡死 → 综合方案 | high (P1) | architecture | fix | `server/internal/repo/mysql/step_sync_log_repo.go` + `server/internal/service/step_service.go` |

## Lesson 1: 防作弊与容错冲突时不要"改 ORDER BY"，加业务层兜底

- **Severity**: high (P1)
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/repo/mysql/step_sync_log_repo.go:127` + `server/internal/service/step_service.go:175-208`

### 症状（Symptom）

r2 把 `FindLatestByUserAndDate` 的 ORDER BY 从 `id DESC` 改为 `client_total_steps DESC, id DESC`，本意是：取"当日历史最大累计步数"作为基线，避免乱序到达让旧 sync 成新基线导致重复入账（A=250, B 延迟=200, C=260 → 按 id DESC 基线取 B → C delta=60 错）。

但这把"基线"语义钉死成"历史高水位线"。在 HealthKit 真实场景下：

```
sync 1: client_total=250 → accepted=250；total=250；sum=250；max=250
sync 2: client_total=100（device reset / data correction）→ rawDelta=0（倒退）
sync 3: client_total=105（reset 后又 5 步）
  - r2 基线 = max=250 → rawDelta = max(0, 105-250) = 0  ← 永远 0
sync 4: client_total=110 → r2 基线 = 250 → rawDelta = 0  ← 永远 0
... 直到用户 client_total 重新超过 250 才有 delta
```

reset 之后用户走的所有真实步数（5 + 5 + ...）**永久丢失**，直到累计超过历史高水位。

### 根因（Root cause）

**误把"防作弊"和"容错"当作单一字段排序问题，导致在两个场景之间二选一**。

正确分析：
- **防作弊（乱序到达）**：核心约束是"当日入账总和不能超过 client 报告的当日累计"。这是个 **SUM 比较**，不是 ORDER BY 能表达的。
- **容错（reset/correction）**：核心约束是"基线必须跟得上 client 的最新报告"。这是 **id DESC 自然语义**。

两个约束**正交**，应该分两层：
- repo 层：基线 = 最近 INSERT（id DESC），保证 reset 后基线能跟上。
- service 层：rawDelta 算完后，加一道 `if SumAccepted(today) + rawDelta > clientTotalSteps then rawDelta = max(0, clientTotalSteps - SumAccepted)` 兜底。

兜底逻辑为什么对：`client_total_steps` 是健康源累计值（SUM 真值），任何时刻"已入账总和"都不应超过它。乱序到达把基线带低 → rawDelta 算多 → SUM 兜底削回到上限。reset 场景因为 SUM 不会增长，但基线跟得上，所以走小步数也能正常入账（被兜底削但不卡死）。

r2 的错误是把"防乱序"硬塞进 ORDER BY，损失了"基线跟最新走"的语义。**遇到约束冲突时不应该在单点用更复杂的 ORDER BY 解决，应该拆成正交两层（取数 + 校验）**。

### 修复（Fix）

**repo 层**：撤销 r2，回到 `ORDER BY id DESC`：

```go
// before (r2)
.Order("client_total_steps DESC, id DESC")

// after (r3)
.Order("id DESC")
```

**service 层**：在 rawDelta 算完后、单次截断之前，加 SUM 兜底：

```go
prevAccepted, err := s.stepSyncLogRepo.SumAcceptedDeltaByUserAndDate(txCtx, in.UserID, in.SyncDate)
if err != nil { return apperror.Wrap(err, ...) }

if latest != nil && prevAccepted+rawDelta > int64(in.ClientTotalSteps) {
    adjusted := int64(in.ClientTotalSteps) - prevAccepted
    if adjusted < 0 { adjusted = 0 }
    slog.WarnContext(txCtx, "step sync sum cap adjusted", ...)
    rawDelta = adjusted
}
```

**顺序关键**：SUM 兜底必须在单次截断（singleSyncCap）和当日封顶（dailyCap）**之前**算 —— 单次截断和封顶都是基于"最终入账值"的进一步约束，SUM 兜底是基于"健康源累计真值"的更基础约束。

**测试**（4 条新增/改写）：

1. `TestStepService_SyncSteps_HealthKitReset_BaselineFollowsLatestNotMax_DeltasResume`（service 单测，3 个 subtests）：reset 后第一次 sync 105 → delta=0；第二次 sync 110 → delta=0（基线跟到 105 而非锁死 250）；第三次 sync 270（超过历史最大）→ delta=20（SUM 兜底 adjusted）
2. `TestStepService_SyncSteps_OutOfOrderSync_SumCapPreventsRepeatedAccrual`（service 单测）：A=250 → B=200 延迟 → C=260；C 应入账 10 而非 60（SUM 兜底削回）
3. `TestStepServiceIntegration_OutOfOrderSync_SumCapPreventsRepeatedAccrual`（integration，原 r2 OOO 测试改写）：同场景 + 真 MySQL
4. `TestStepServiceIntegration_HealthKitReset_AccrualResumesNotPermanentlyBlocked`（integration 新增）：250 → 100 reset → 105 → 300，验证 sync 4 能 accept=50（不被永久卡死）

repo 层 SQL 断言改写：`TestStepSyncLogRepo_FindLatestByUserAndDate_OrderByIDDesc_BaselineSQLAssertion`（原 OrderByClientTotalSteps 改名 + 改断言为 `ORDER BY (id|`id`) DESC`）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **append-only 日志表上同时承担"防作弊"和"容错"约束** 时，**禁止**用单一 ORDER BY 同时解决两者；**必须**拆成正交两层 —— repo 层只负责"取最近一行"（自然时序 id DESC），业务约束（SUM 不超过真值上界 / 计数不超过封顶）放 service 层。
>
> **展开**：
> - 当 review 抓出"基线计算 bug"时，**第一反应不是改 ORDER BY**，而是问：这是"基线取错了"还是"基线对的但缺少其他约束"？后者用 ORDER BY 改总会引入对偶 bug。
> - 业务字段（如 `client_total_steps`）作为 ORDER BY 第一键时，警惕"该字段非严格单调"的真实场景（reset / correction / 数据重导）。健康源、订单流水、IM 消息这种"理论单调但实际可能下降"的字段都有这个陷阱。
> - SUM 兜底是"客户端报告的累计真值"约束的标准实装。在 sync/upsert 类接口里，每当存在"累计真值字段"，都应该考虑"已入账之和不能超过累计真值"作为兜底。
> - **fix-review 前必看本 lesson**：r1 → r2 → r3 三轮决策史已经证明"在两个 ORDER BY 之间反复横跳"是错的；任何想再改 ORDER BY 的修复都需要先考虑能否用 service 层校验解决。
> - **反例 1**（r1）：`ORDER BY id DESC`，无任何兜底 → 乱序到达 = 重复入账（防作弊漏洞）
> - **反例 2**（r2）：`ORDER BY client_total_steps DESC, id DESC`，无任何兜底 → reset/correction = 永久卡死（容错回归）
> - **正例**（r3）：`ORDER BY id DESC` + service 层 `SumAccepted + rawDelta ≤ clientTotalSteps` 兜底 → 两个场景都正确

## Meta: 本次 review 的宏观教训

**"防御性编程不是改 ORDER BY"**。同一份 review feedback 已经迫使 Story 7.3 的基线查询改了三轮（r1→r2→r3）。教训是：当 review 指出某个查询/算法的边界 case bug 时，需要先穷举**所有**对偶场景（反向 / 重复 / 倒退 / reset / 乱序 / 极端值 / 多用户并发），确认"任何修复方案不能在解决 A 的同时把 B 弄坏"。如果一个方案不能同时覆盖所有场景，**说明该方案在错的层次解决问题**。

具体到本 case：基线查询是个"取数"问题，业务约束是"约束"问题，两者放同一层（repo SQL 的 ORDER BY）必然冲突。拆开就轻松。**遇到反复 review 的同一个查询，第一反应应该是"这个问题本不该在这一层解决"**。
