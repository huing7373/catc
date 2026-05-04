---
date: 2026-05-04
source_review: codex review round 5 (file: /tmp/epic-loop-review-8-1-r5.md)
story: 8-1-healthkit-接入
commit: 9b55cc2
lesson_count: 1
review_round: 5 of 5 (final round, no round 6)
related_lessons:
  - 2026-05-04-healthkit-today-enddate-clamp-to-now.md  (round 4 fix on read side)
  - 2026-05-04-healthkit-read-auth-and-preseed-flag-non-probe.md
  - 2026-05-04-debug-seed-vs-probe-await-coupling.md
---

# Review Lessons — 2026-05-04 — HealthKit dev-seed sample 必须落在过去（与读端 endDate clamp 配套）

## 背景

Story 8.1 HealthKit 接入的第 5 轮 codex review。round 4 在读端
（`HealthProviderImpl.readDailyTotalSteps`）把当日查询的 endDate clamp 到 `min(now, dayEnd)`
以排除 future-timestamp sample；round 5 指出**写端**还没配套——
`HealthKitDevSeedUseCase.preseedToday` 仍把 sample 固定打在 `today 11:00`，
在早晨（11:00 之前）启动测试时 sample 落在"未来"，会被 round 4 的 clamp 直接排除，
`-PetAppPreseedHealthKitSteps` flag 失效（probe/non-probe 都读 0）。

**这是 5 轮上限的最后一轮**——本轮 fix 后 epic-loop 主循环 iter 6 触发 HALT，
不会再跑 round 6 codex 验证。所以修法选最保守路径：动 1 个文件 1 个常数，
不重写架构 / 不顺手 refactor。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | preseed sample 时戳必须落在过去（与读端 clamp 配套） | medium (P2) | testing / config | fix | `iphone/PetApp/Features/DevTools/UseCases/HealthKitDevSeedUseCase.swift:52-56` |

## Lesson 1: preseed sample 时戳必须落在过去（写端 / 读端 clamp 配套）

- **Severity**: medium (P2)
- **Category**: testing
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/DevTools/UseCases/HealthKitDevSeedUseCase.swift:52-56`

### 症状（Symptom）

`HealthKitDevSeedUseCase.preseedToday` 把 sample 固定打在 `today 11:00`：

```swift
let dayStart = calendar.startOfDay(for: Date())
let sampleStart = calendar.date(byAdding: .hour, value: 11, to: dayStart)
```

但 round 4 已把 `HealthProviderImpl.readDailyTotalSteps` 当日 endDate clamp 到
`min(now, dayEnd)`。当 UITest / 手动测试在 11:00 之前启动时，sample 在"未来"，
被 clamp 排除，preseed flag 整个失效——probe view 读到 0、non-probe 路径也读 0。
`-PetAppPreseedHealthKitSteps` 这个 debug/UITest hook 在上午时段直接不可用。

### 根因（Root cause）

写端（dev seed）和读端（read clamp）是**双向耦合**，但 round 4 修读端时没同时检查写端。
读端 clamp 决定了"哪些 sample 会被纳入查询"，写端 sample 时戳决定了"sample 是否落在
clamp 之后"——任一端单独正确不够，必须配对正确。

具体踩坑思路：seed 时选 `today 11:00` 这种 magic clock-hour 常数时，作者只想到
"避开模拟器在午夜附近的边界 race"（dayStart=00:00 时分钟级 sample 难以归属），
没想到读端可能 clamp 在 11:00 之前的 endDate。当读端 r4 fix 改成 `min(now, dayEnd)`，
写端的 future-dated sample 就被新规则排除了。

### 修复（Fix）

**改动**：把 sample 窗口从固定 `today 11:00 ~ 11:01` 改成 `now-120s ~ now-60s`：

```swift
// before:
let dayStart = calendar.startOfDay(for: Date())
let sampleStart = calendar.date(byAdding: .hour, value: 11, to: dayStart) ?? dayStart
let sampleEnd = calendar.date(byAdding: .minute, value: 1, to: sampleStart) ?? sampleStart

// after:
let now = Date()
let sampleEnd = now.addingTimeInterval(-60)
let sampleStart = sampleEnd.addingTimeInterval(-60)
```

**选 (b) Date()-60 的理由**（vs (a) min(11:00, now-60) / (c) today 00:30）：
- (a) 仍带 magic clock-hour 常数，没去掉根本耦合
- (c) `today 00:30` 看似简单，但模拟器跨天时段同样会卡边界（午夜 00:00 启动时正好踩 race）
- (b) **永远落在 now 之前 60 秒**，与读端 `min(now, dayEnd)` clamp 天然兼容，
  没 magic constant，没小时假设，逻辑最简洁

**已承认的边界**：刚跨过午夜的极短窗口（00:00:00~00:01:00）下，`now-60` 落在前一天，
sample 归属前一天；UITest 在 0 点启动属极罕见场景，且 probe 此时读"今天"理应为 0，
trade-off 接受。已在代码注释里明示。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在改 **HealthKit 这类 sample 写入路径** 时，**必须先检查相同模块
> 的读路径有无 endDate / startDate 边界 clamp**，**禁止**写"固定 clock-hour"或"未来时戳"
> 的 sample——所有 dev seed / fixture sample 都用 **`Date() - N seconds`** 这类相对偏移落在过去。
>
> **展开**：
> - HealthKit 的 sample 时戳是"写时定值"的，但 query 的 startDate/endDate 是"读时动态"的；
>   读端如果 clamp 到 `now`（避免 future-dated 通胀），写端任何 future timestamp 都白写。
> - **配套机制**：`min(now, dayEnd)` 读端 clamp + `Date() - 60s` 写端偏移 = 双向兼容。
>   两端必须同时 audit；只 fix 一端 = 引入隐藏 regression。
> - **禁用 magic clock-hour 常数**（11:00 / 12:00 / 09:00 等）作为 fixture 时戳——
>   它对"启动时间 < 该常数"的所有路径都失效。改用 `Date() - N` 相对偏移。
> - **承认且文档化跨午夜边界**：相对偏移在 00:00:00 ~ 00:01:00 极短窗口会跨日，
>   接受 trade-off 时在代码注释里写清"为何不修这个 corner case"。
> - **反例 1**：seed 时调 `calendar.date(byAdding: .hour, value: 11, to: dayStart)`——
>   早晨启动时 sample 在未来，被读端 clamp 排除。
> - **反例 2**：seed 时调 `Date()`（恰好 now）——读端 query end = now 时
>   `[strictStartDate]` 边界判定可能 race；用 `now - 60s` 留 60 秒安全缓冲。
> - **反例 3**：只修读端 clamp、不 audit 写端 fixture——CI 在不同时段（CI runner 时区
>   常 UTC，本地 CST）跑出不同结果，后续 review 才暴露写端没配套。

## Meta: 5 轮 review 后的总结（Story 8.1 收官）

Story 8.1 走完 5 轮 codex review（r1~r5）才稳定下来。每轮主题：

1. **r1**：HealthKit auth / read 框架基本正确性
2. **r2**：probe view 与 dev seed 的 await 时序耦合
3. **r3**：strictStartDate 的跨午夜 sample 归属 trade-off（defer，承认而不修）
4. **r4**：读端 endDate clamp 到 `min(now, dayEnd)`（避免 future-dated 通胀）
5. **r5（本轮）**：写端 sample 必须落在过去（与 r4 读端 clamp 配套）

**5 轮的共同主题** = HealthKit 的"时间窗口"在读 / 写 / 时区 / 模拟器/真机 多个维度上都
有边界陷阱；每轮 review 沿一个新维度暴露下一个陷阱。这暗示一类系统性教训：

> **`HealthKit 时间窗口"问题不能靠单点修，需要在 r1 阶段就把"读端 query 边界"和
> "写端 fixture 时戳"作为一对契约同时设计`**——任何一端独立改动都要同时 audit 另一端，
> 不然必然在后续 review 才暴露错配。

**为什么本轮是最后机会**：epic-loop 主循环钦定 5 轮上限。本轮修完后 iter 6 直接 HALT，
不再跑 r6 codex 验证。所以本轮修法刻意选最保守、最局部的路径——只动 1 个常数，
不顺手 refactor 任何相邻代码——降低"修一个引入两个"的风险。

如果本轮还有遗留问题（比如本 fix 的"00:00:00~00:01:00 凌晨边界"），按 epic-loop 流程
会留到下一个 story / 下一次 review 处理，不再压在 8.1 收官上。
