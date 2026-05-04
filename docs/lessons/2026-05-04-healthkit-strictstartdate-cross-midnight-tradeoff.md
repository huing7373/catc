---
date: 2026-05-04
source_review: codex review round 3 (file: /tmp/epic-loop-review-8-1-r3.md)
story: 8-1-healthkit-接入
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-04 — HealthKit `.strictStartDate` 跨午夜 sample 的 trade-off（defer 而非 codex 钦定 fix）

## 背景

Story 8.1 的 `HealthProviderImpl.readDailyTotalSteps(date:)` 用 `HKStatisticsQuery + .cumulativeSum` 读当日步数；
predicate 用的是 `HKQuery.predicateForSamples(..., options: .strictStartDate)`.

codex round 3 提出 [P2] finding：跨午夜 sample（例如 vendor batched pedometer 写入 `23:59 → 00:01`）会被
**整段归到起始日**，导致前一日 overcount + 后一日 undercount；codex 建议改成 `[.strictStartDate, .strictEndDate]`.

经技术分析：codex 钦定 fix **反而更糟**（会完全丢弃跨午夜 sample，两边都不算）；真正的 Apple 钦定 prorate 方案是
`HKStatisticsCollectionQuery` + daily anchored interval，但属于 Story 8.3 / 8.5 步数同步业务级别的重写，超出 Story 8.1 scope.
**defer**：保留 `.strictStartDate` + 加详尽注释 + 本 lesson 记录 trade-off.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `.strictStartDate` 跨午夜 sample 归属 trade-off | P2 | architecture | **defer** | `iphone/PetApp/Core/Health/HealthProviderImpl.swift:76` |

## Lesson 1: HealthKit `predicateForSamples options` 的三个候选都有 trade-off，单选 `.strictStartDate` 是 HK idiom 兜底而非 bug

- **Severity**: P2
- **Category**: architecture
- **分诊**: defer（不修，记 lesson）
- **位置**: `iphone/PetApp/Core/Health/HealthProviderImpl.swift:76`

### 症状（Symptom）

跨午夜的 step sample（罕见的 vendor batched 写入路径，如 23:59:30 → 00:00:30）在
`predicateForSamples(start: dayStart, end: dayEnd, options: .strictStartDate)` 下会被**整段归到 `dayStart` 那天**，
导致：
- 前一日（dayStart 那天）`HKStatisticsQuery + .cumulativeSum` overcount sample 中跨午夜后那部分步数
- 后一日 undercount sample 中其实属于后一日的部分

具体到 step sync 业务（Story 8.3 / 8.5）：用户 syncDate=Y 那天可能多上报几步、syncDate=Y+1 那天少上报几步.

### 根因（Root cause）

`HKQuery.predicateForSamples(withStart:end:options:)` 的 `options` 参数有三种语义，**每种都各有 trade-off**：

| options | 跨午夜 sample 行为 | 适合场景 |
|---|---|---|
| `.strictStartDate`（仅起始严格落入） | 整段归到起始日（overcount/undercount） | 大多数 HK 文档/示例钦定的 idiom；接受时间偏移 |
| `[.strictStartDate, .strictEndDate]` | **完全丢弃**（两边都不算） | 极少使用——会丢数据 |
| 不传 options（默认） | sample 与窗口**有交集**就算（重复计数） | 多日累计求和场景，不适合"日级独占归属" |

Apple 真正的 prorate（按时间比例自动拆分跨边界 sample）方案在 `HKStatisticsCollectionQuery + anchorDate + intervalComponents=daily`
路径下，框架自动按时间比例 prorate sample 到对应 interval. 但：
1. 它返回 `HKStatisticsCollection`（多 interval），不是单 `HKStatistics`，调用约定不同
2. anchored interval 的边界是 anchorDate 起算的整数倍，跨时区切日有额外细节
3. 改它要重写 `readDailyTotalSteps` 实现 + 测试，且会影响 Story 8.3 / 8.5 syncDate 时区契约的隐含假设
   （V1 §6.1 钦定 syncDate 是"客户端本地时区当日 0 点"——目前 `.strictStartDate` 路径与该契约语义对齐：
   "sample 起始时间所在的本地日"）

**Claude 的思维漏洞**：看到 review 写"P2 functional correctness issue" + 给了具体 fix（`[.strictStartDate, .strictEndDate]`）
容易顺手照搬；但 reviewer（包括 LLM reviewer）**不一定验证过自己的 fix**——这条建议事实上更糟（丢数据）.
任何 HK predicate options 修改前必须查 `predicateForSamples` 文档表 + 想清楚跨边界 sample 三态行为.

### 修复（Fix）

**defer**——不修代码逻辑；只在源码加注释 + 写本 lesson + 记入 epic 8-3/8-5 的隐含 trade-off.

源码改动（`iphone/PetApp/Core/Health/HealthProviderImpl.swift:74-86`）：

```swift
// ⚠️ 关于 `.strictStartDate` 单独使用而非 `[.strictStartDate, .strictEndDate]`（codex r3 [P2] defer）:
// - 跨午夜 sample（罕见的 vendor batched 写入，如 23:59→00:01）会**整段归到起始日**，
//   理论上前一日 overcount + 后一日 undercount.
// - 但 codex 建议的 `[.strictStartDate, .strictEndDate]` 反而**完全丢弃**跨午夜 sample，更糟.
// - Apple 钦定的 prorate 方案是 `HKStatisticsCollectionQuery` + daily anchored interval（按时间比例自动拆分），
//   但那是 Story 8.3/8.5 步数同步业务级别的重写，超出本 story（probe-level read API）scope.
// - HKQuantitySample stepCount 在实践中通常分钟级 sample 且不跨午夜；trade-off 接受.
// - 详见 docs/lessons/2026-05-04-healthkit-strictstartdate-cross-midnight-tradeoff.md.
let predicate = HKQuery.predicateForSamples(withStart: dayStart, end: dayEnd, options: .strictStartDate)
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 改 HealthKit `HKQuery.predicateForSamples` 的 `options` 时，**必须先查文档表确认三态语义**
> （`.strictStartDate` 整段归起始日 / `[.strictStartDate, .strictEndDate]` 完全丢弃跨边界 sample / 默认有交集就算），
> **禁止**未验证盲从 reviewer（包括 LLM reviewer）建议的 options 组合；真正的"按日 prorate"是
> `HKStatisticsCollectionQuery + anchorDate + intervalComponents=daily`，**不是** options 组合可以解决的.
>
> **展开**：
> - 三态对照表（务必背下）：
>   - `.strictStartDate`：sample.startDate ∈ [start, end) → 整段归起始日；跨边界 sample overcount/undercount
>   - `[.strictStartDate, .strictEndDate]`：sample.startDate ∈ [start, end) **且** sample.endDate ∈ [start, end) → 跨边界 sample 直接丢弃
>   - 不传 options：sample 与 [start, end) **有交集**即算 → 多日窗口重复计数
> - 单日窗口业务里，三种 options 都不完美；`.strictStartDate` 是 HK 文档示例钦定的 idiom，因为对绝大多数 sample（不跨午夜）
>   行为正确，对罕见跨午夜 sample 接受"归到起始日"的时间偏移（而非丢数据 / 重复算）
> - 真正想"按日比例 prorate" → 改用 `HKStatisticsCollectionQuery`：
>   ```swift
>   let query = HKStatisticsCollectionQuery(
>       quantityType: stepCountType,
>       quantitySamplePredicate: nil,
>       options: .cumulativeSum,
>       anchorDate: anchor,
>       intervalComponents: DateComponents(day: 1)
>   )
>   query.initialResultsHandler = { _, collection, error in
>       collection?.enumerateStatistics(from: dayStart, to: dayEnd) { stat, _ in
>           // stat.sumQuantity() 已按时间比例 prorate
>       }
>   }
>   ```
> - 反例 1：照搬 codex / reviewer 建议改 `[.strictStartDate, .strictEndDate]` —— 跨午夜 sample 两边都不算，比 overcount 更糟
> - 反例 2：不传 options —— 多日窗口重复算 sample，单日窗口里此问题不暴露但代码 review 时同样会被指出
> - 反例 3：手写 endDate 拆分 prorate —— 复杂、易 off-by-one、引入新 bug 面，性价比远不如 HKStatisticsCollectionQuery
> - 决策原则：**probe-level read API**（如本 story 8.1）保 `.strictStartDate` + 注释 trade-off；**业务级 sync API**（如 step sync 8.3/8.5）需要严格 prorate 时改用 HKStatisticsCollectionQuery.

### 附：何时 defer，何时改

defer 的边界：
- ✅ 跨午夜 sample 在产品步数场景**实证**罕见（HKQuantitySample stepCount 通常分钟级、由 system 实时写入）
- ✅ 当前 syncDate 时区契约（V1 §6.1）默认"sample 起始时间归属"语义，与 `.strictStartDate` 对齐
- ✅ 用户在 dev override 中明确禁用 codex 钦定的 `[.strictStartDate, .strictEndDate]` 方案

升级到改的触发条件（任一满足都该改用 `HKStatisticsCollectionQuery`）：
- ❌ 真机/线上日志出现可观测的"步数跨日漂移"问题（步数对账偏差）
- ❌ V1 syncDate 契约改成"按时间比例归属"语义
- ❌ 引入第三方 vendor 写入（如 Apple Watch + Fitbit 双源），batched 跨午夜 sample 概率显著上升
