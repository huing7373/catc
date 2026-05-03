---
date: 2026-05-03
source_review: codex review of Story 7.3 r5 (`/tmp/epic-loop-review-7-3-r5.md`)
story: 7-3-post-steps-sync-接口-累计差值入账-service
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-03 — 步数 sync 第三层防御：截断 + 乱序组合下 SUM 兜底仍漏，需叠加 max-reported clamp

## 背景

Story 7.3 的步数 sync 防作弊 / 容错防御已经经历了四轮 review：

- **r1**：`ORDER BY id DESC` 当基线（最近 INSERT）—— 乱序到达让旧 sync 成新基线 → 重复入账
- **r2**：改 `ORDER BY client_total_steps DESC` 当基线（历史最大累计）—— 乱序解决但 HealthKit reset 永久卡死
- **r3**：基线退回 `id DESC` + service 层加 SUM(accepted_delta) 兜底 —— 解决了 r1/r2 二选一，但**单层 SUM 兜底仍能在"截断 + 乱序"组合下漏**
- **r5**（本轮）：在 SUM 兜底之外**叠加** maxReported (MAX(client_total_steps)) clamp —— 两层防御纵深

r3 的"两个约束分两层（repo 基线 + service SUM 兜底）"思路是对的，但**第三个约束**（截断丢的步数永久丢，不能被乱序到达的旧 sync 间接恢复）一开始没识别出来。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | r3 SUM 兜底在"截断 + 乱序"组合下失效 → 叠加 maxReported clamp | high (P1) | architecture | fix | `server/internal/repo/mysql/step_sync_log_repo.go` + `server/internal/service/step_service.go` |

## Lesson 1: 防御层不是"二选一"是"叠加"——截断改变了 prevAccepted 与"已报告最大累计"的等价关系

- **Severity**: high (P1)
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/repo/mysql/step_sync_log_repo.go` (新增 `MaxClientTotalStepsByUserAndDate` 方法) + `server/internal/service/step_service.go` (SyncSteps 步骤 (2) 改用 maxReported clamp)

### 症状（Symptom）

r3 的 SUM 兜底在常规乱序场景下正确（A=250 → B=200 延迟 → C=260：基线=200, rawDelta=60, SUM(250)+60=310>260, 削回 10 ✓）。

但加上**截断**后失效：

```
sync A: client_total=10000 → 截断 5000 → accepted=5000；DB 写 client_total=10000 / accepted=5000
                                          prevAccepted=5000 ≠ maxReported=10000  ← 关键不等
sync B: client_total=8000 (delayed) → 倒退 rawDelta=0；accepted=0
                                       prevAccepted=5000；maxReported=10000

sync C: client_total=12000
  - r3 算法：基线 = latest = 8000 → rawDelta = 12000 - 8000 = 4000
            SUM 兜底：prevAccepted(5000) + 4000 = 9000 < 12000 → **不触发**
            → accepted = 4000

  - 实际从 sync A 到现在客户端只新增 2000 步（10000 → 12000），但服务多入账 2000 步
```

### 根因（Root cause）

r3 SUM 兜底的隐藏假设：**"prevAccepted (已入账总和) 等于 maxReported (已报告的最大累计)"**。这个假设在"无截断"场景下成立 —— 每次入账等于报告增量，累加就是最大累计。

但**单次截断**打破了这个假设：sync A 报 10000，截断后只入账 5000 —— prevAccepted 永久 < maxReported（差额 = 截断丢的步数）。后续乱序到达把 latest 拉低，rawDelta = clientTotal - latest 算出比 "clientTotal - maxReported" 更大的值，SUM 兜底用 prevAccepted 做减法削得不够。

更深层：r3 把"基线"和"防作弊削回"当作两个独立的 1D 字段问题（基线选 id DESC 还是 max；削回用 SUM(accepted)）。但实际是 3D：

1. **基线选谁**（latest vs max(client_total)）—— 决定 rawDelta 算出多少
2. **削回参考"已入账"还是"已报告"**（SUM(accepted) vs MAX(client_total)）—— 决定能不能正确削回
3. **截断 + 乱序组合**让 (1) 和 (2) 之间的关系被打破

正确分解：
- **rawDelta 应该用 `clientTotal - maxReported`**（不用 latest 当基线，避免乱序拉低）
- **SUM 兜底保留作为第二层防御**（应对未来更复杂的乱序模式）
- **两层独立、叠加 → defense in depth**

### 修复（Fix）

**新增 repo 方法 `MaxClientTotalStepsByUserAndDate`**：

```go
// SELECT COALESCE(MAX(client_total_steps), 0) FROM user_step_sync_logs
// WHERE user_id = ? AND sync_date = ?
func (r *stepSyncLogRepo) MaxClientTotalStepsByUserAndDate(ctx, userID, syncDate) (uint64, error)
```

**service.SyncSteps 改造**：rawDelta 不再用 `latest.ClientTotalSteps` 当基线，改用 `maxReported`：

```go
// 旧 (r3)：
if latest != nil {
    if in.ClientTotalSteps > lastClientTotalSteps {
        rawDelta = int64(in.ClientTotalSteps - lastClientTotalSteps)  // ← 乱序场景出 bug
    } else {
        rawDelta = 0
    }
}

// 新 (r5)：
maxReported, _ := s.stepSyncLogRepo.MaxClientTotalStepsByUserAndDate(...)
if latest == nil {
    rawDelta = int64(in.ClientTotalSteps)  // 首次同步
} else if in.ClientTotalSteps > maxReported {
    rawDelta = int64(in.ClientTotalSteps - maxReported)  // 新高
} else {
    rawDelta = 0  // 旧 / 重复 / 乱序到达，全部 0
}

// SUM 兜底**保留**（第二层防御；未来更复杂乱序模式的兜底）
prevAccepted, _ := s.stepSyncLogRepo.SumAcceptedDeltaByUserAndDate(...)
if latest != nil && prevAccepted+rawDelta > int64(in.ClientTotalSteps) {
    // 削回（绝大多数场景冗余，但作为 defense-in-depth）
}
```

**测试覆盖**：
- 新增 repo 单测：`TestStepSyncLogRepo_MaxClientTotalStepsByUserAndDate_HappyPath`（HasMax12000 + ZeroFallback 两 subtest）
- 新增 service 单测：`TestStepService_SyncSteps_TruncationPlusOutOfOrder_MaxReportedClampPreventsOverAccrual`（精确反例 mock：latest=8000, maxReported=10000, prevAccepted=5000, clientTotal=12000 → accepted=2000）
- 新增集成测试：`TestStepServiceIntegration_TruncationPlusOutOfOrder_MaxReportedClampPreventsOverAccrual`（真 docker MySQL 跑完 sync A→B→C 三步链路）
- 现有 14 / 15 个单测全部更新 `maxReportedFn` mock 字段（让两层防御都被覆盖）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计"防作弊截断 + 容错乱序到达"复合防御逻辑** 时，**必须**把"已入账总和"(`SUM(accepted)`) 和 "已报告的最大累计" (`MAX(reported)`) 作为**两个独立信号**显式 query，不要默认它们相等。
>
> **展开**：
> - **触发条件**：业务上有"输入累计值 → 截断后入账增量"语义（步数 / 流量 / 积分等任何"健康源累计 → 服务侧增量入账"模式），且需要支持乱序到达 / 重试
> - **核心识别**：截断 / 封顶 / 入账规则只要会让"实际入账"小于"报告增量"，那么 `SUM(accepted) ≠ MAX(reported)`，二者**不能互相替代**
> - **正确防御**：分两层
>   - **第一层（基线）**：用 `MAX(reported)` 算 `rawDelta = max(0, current - MAX(reported))` —— 防"乱序拉低基线"
>   - **第二层（总额兜底）**：用 `SUM(accepted) + rawDelta ≤ current` 削回 —— 防未来更复杂的乱序 / 修复回归
> - **架构纪律**：每加一层防御**都要写反例测试**（service 单测 + 集成测试），且反例必须在 mock 里设 `MAX ≠ SUM` 来锁定区分度
> - **反例**：
>   - `rawDelta = current - latest.value` （用 latest 当基线 —— 乱序场景出 bug）→ 错
>   - `rawDelta = current - SUM(accepted)` （用 SUM 当基线 —— 截断场景下 SUM < MAX，rawDelta 被算大）→ 错
>   - `rawDelta = current - MAX(reported)` + SUM 兜底叠加 → 对
>   - 只在第一层基线下功夫（改 ORDER BY 之类），不上第二层兜底 → 错（r1/r2 失败模式）
>   - 用单层 SUM 兜底，不上第一层 maxReported clamp → 错（r3 失败模式）
> - **测试纪律**：每条新增防御层至少有一条单测同时把 latest / maxReported / sumAccepted 三个 mock 设成**不相等**值，验证 service 用对了字段。否则 mock 会让"误用 latest 当基线"也"看起来"通过

---

## Meta: 本次 review 的宏观教训

四轮 review（r1 → r2 → r3 → r5）让"步数防作弊基线"问题持续出现"修一头漏一头"的回归，根因不是单条 logic bug，而是 **mental model 不够分层**。每次 review 都倾向找"该怎么改这一个 SQL 字段 / 加哪个判断"，而没有问 **"这个业务规则能拆成几个独立的不变式（invariant），每个不变式独立的 query 是什么"**。

正确的做法应该在 r1 阶段就识别出三个不变式：

1. **乱序保护**：`final_accepted_for_sync ≤ current_total - last_max_reported`
2. **总额保护**：`SUM(accepted_today) ≤ MAX(reported_today)`
3. **截断 / 封顶**：`single_delta ≤ singleCap` 且 `SUM(accepted_today) + delta ≤ dailyCap`

每个不变式独立 query、独立 enforce，不要试图用一条 ORDER BY 同时满足三个。

> **一句话规则**：未来设计任何"输入累计 → 服务增量入账 + 防作弊"业务时，**先列出全部不变式**（用纸笔列），然后**每条不变式单独写 query 单独 enforce**，禁止在一条 SQL / 一段 service 逻辑里"一鱼多吃"。
