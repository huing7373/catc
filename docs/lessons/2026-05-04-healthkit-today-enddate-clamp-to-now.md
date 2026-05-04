---
date: 2026-05-04
source_review: codex review round 4 (file: /tmp/epic-loop-review-8-1-r4.md)
story: 8-1-healthkit-接入
commit: 7d7b1b7
lesson_count: 1
---

# Review Lessons — 2026-05-04 — HealthKit 当日窗口的 endDate 必须 clamp 到 now，不能用次日 0 点

## 背景

Story 8.1 (HealthKit 接入) round 4 codex review。round 1-3 已分别处理了 cache 反优化、auth probe 推断、`.strictStartDate` 跨午夜归属 trade-off。round 4 揪出窗口 **endDate 端**的对称坑：当 `readDailyTotalSteps(date:)` 调当日时，predicate 终点 `dayEnd` 是次日 00:00（未来时刻），任何 future-timestamp sample（manual entries / 本仓 debug seed 11:00 写入）会在早晨读时被提前计入 → inflated read。Story 8.5 `/steps/sync` 也基于此 API → 不修则会上传通胀步数。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 当日 readDailyTotalSteps endDate 未 clamp 到 now，future-dated sample 提前计入 | medium (P2) | other (correctness) | fix | `iphone/PetApp/Core/Health/HealthProviderImpl.swift:85` |

## Lesson 1: HealthKit 当日窗口 endDate 必须 clamp 到 `min(now, dayEnd)`

- **Severity**: medium (P2)
- **Category**: other (correctness) / health-data-window
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Health/HealthProviderImpl.swift:85`

### 症状（Symptom）

`readDailyTotalSteps(date:)` 用 `Calendar.current.startOfDay` + 加 1 天得 `dayEnd`（次日 00:00）作为 predicate 终点。当 date == 今天 时：
- debug seed 在 09:00 启动时把 5000 步打在今天 11:00（HKQuantitySample 时间戳允许写未来时刻）
- 09:05 probe-read 用 dayEnd 作终点 → 读到 5000，而真实"截至 09:05 累计"应为 0
- 任何 manual entries / 第三方 app 写入未来时刻的 sample 都触发同样 inflated read
- Story 8.5 `/steps/sync` 调用此 API → 上传通胀步数

### 根因（Root cause）

时间窗口数据源在"读今天"场景天然有不对称性：起始端（dayStart）是过去时刻天然合规，终点端（dayEnd 次日 00:00）是**未来时刻**会包含尚未发生的 sample。对历史日（date < 今天）这没问题（dayEnd ≤ now），对今天必须把终点 clamp 到 now 才符合"截至现在的累计"语义。

写代码时容易只考虑"日窗口 = [dayStart, dayStart+1d)"这种 calendar 语义，忽视"sample 时间戳可以是未来"这一 HK 特性。本仓的 debug seed（11:00 固定时刻写入 5000 步）正好命中此坑——既是 reviewer 的发现路径，也是 self-test 的反例素材。

### 修复（Fix）

`iphone/PetApp/Core/Health/HealthProviderImpl.swift:85` 在构造 predicate 前，把 endDate 从 `dayEnd` 改为 `min(Date(), dayEnd)`：

```swift
let now = Date()
let endDate = min(now, dayEnd)
let predicate = HKQuery.predicateForSamples(withStart: dayStart, end: endDate, options: .strictStartDate)
```

历史日（date < 今天）`dayEnd ≤ now`，`min` 不影响结果；当日（date == 今天）`dayEnd > now`，clamp 到 now。

注：未新增单测——HealthProviderImpl 依赖 HKHealthStore 真实 framework，单测用 HealthProviderMock 不走 predicate 路径；现有 352 tests 全 pass 验证未回归。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写 **基于时间窗口查询数据源（HK / HM / Calendar / Log）的"读今天"代码** 时，**必须** 把 endDate clamp 到 `min(now, windowEnd)`，绝不能直接用 `windowEnd`（如 startOfDay + 1d）。
>
> **展开**：
> - 时间窗口边界两端是不对称的：起始端是过去时刻天然合规，终点端在"读今天"时是**未来时刻**会包含尚未发生的数据
> - 数据源的 sample 时间戳允许是未来（HK 写入不校验 endDate ≤ now）；manual entries / debug seed / 第三方 app 都可能造成 future-timestamp sample
> - 历史日 query 不受影响（windowEnd ≤ now，min 是 no-op），所以一行 `min(now, windowEnd)` 是 universal-safe 的修复
> - 上游业务（如 `/steps/sync` 上传步数）依赖此读 API → 不 clamp 会通胀传播到 server
> - 与窗口起始端的"跨午夜归属"坑（见 `2026-05-04-healthkit-strictstartdate-cross-midnight-tradeoff.md`）形成对称：起始端的 trade-off 可 defer，终点端的 future-clamp 必须 fix
> - **反例 1**：`HKQuery.predicateForSamples(withStart: dayStart, end: dayEnd, options: .strictStartDate)`（dayEnd 是次日 0 点，今天 query 会读到 future sample）
> - **反例 2**：debug seed 把 5000 步打在 11:00，09:00 probe-read 拿到 5000——这是本仓自己代码 + 自己 review 跑出来的真实反例
> - **正例**：`let endDate = min(Date(), dayEnd); HKQuery.predicateForSamples(withStart: dayStart, end: endDate, options: .strictStartDate)`

---

## Meta: 8.1 review 四轮的"窗口边界两端坑"知识图

- **r1**：HK 当日 cache 是反优化（同日 sync 拿陈旧值）→ 删 cache
- **r2**：HK auth 不能信 success bool，必须 probe-read 推断
- **r3** (defer)：窗口**起始端** `.strictStartDate` 单独使用 vs `[strictStart, strictEnd]` 的跨午夜 sample 归属 trade-off——前者整段归起始日（前日 over / 后日 under），后者完全丢弃；defer 到 8.3/8.5 用 HKStatisticsCollectionQuery prorate
- **r4** (fix)：窗口**终点端** dayEnd 是未来时刻 → 必须 clamp 到 `min(now, dayEnd)`

合起来形成完整的"HK 时间窗口陷阱图"：起始端坑可 defer（影响罕见跨午夜 sample），终点端坑必须 fix（影响所有 future-timestamp sample，含本仓 debug seed）。
