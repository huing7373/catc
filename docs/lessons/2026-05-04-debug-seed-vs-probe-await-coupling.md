---
date: 2026-05-04
source_review: codex review of Story 8.1 r1 (`/tmp/epic-loop-review-8-1-r1.md`)
story: 8-1-healthkit-接入
commit: 55f5c4a
lesson_count: 2
---

# Review Lessons — 2026-05-04 — DEBUG seed 必须串到 probe `.task` 里 await（不能 detached fire-and-forget）；HK 当日缓存看似优化、实是正确性 bug

## 背景

Story 8.1 引入 `HealthProvider`（HKHealthStore + HKStatisticsQuery 读当日累计步数）+ DEBUG-only
UITest 路径（`-PetAppPreseedHealthKitSteps <N>` launch arg + `HealthProviderProbeView` 替换 RootView）。
codex r1 抓出两条独立问题：

1. **[P1] race**：`PetAppApp.init` 用 `Task.detached` fire-and-forget 调 `HealthKitDevSeedUseCase.preseedToday(...)`，
   probe view 的 `.task` 不等 seed 完成就触发 `readDailyTotalSteps` —— 慢模拟器上 UITest 间歇读到 0/error
   而非种入的 5000 步。
2. **[P2] 错误的"性能优化"**：`HealthProviderImpl` 用 NSLock + `cachedDayStart`/`cachedSteps` 缓存当日第一次成功值，
   导致**同日后续 read 永远返回早晨那一刻的值**，跨日才 invalidate。Story 8.5 即将落地的
   `SyncStepsUseCase` 期望同日多次 sync 拿到累加新值，cache 直接破坏这条契约。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | DEBUG seed 必须由 probe `.task` 串行 await，不能 detached fire-and-forget | high (P1) | testing / concurrency | fix | `iphone/PetApp/App/PetAppApp.swift` + `iphone/PetApp/Features/DevTools/Views/HealthProviderProbeView.swift` |
| 2 | HKStatisticsQuery 当日 cache 是反优化：让同日累计读永远卡在第一次值 | medium (P2) | architecture / correctness | fix | `iphone/PetApp/Core/Health/HealthProviderImpl.swift` |

修复：2 / 延期：0 / 不修：0

## Lesson 1: DEBUG-only seed 必须由消费者（probe view / test view）的 `.task` 串行 await，不能在 App.init 期 detached fire-and-forget

- **Severity**: high (P1)
- **Category**: testing / concurrency
- **分诊**: fix
- **位置**: `iphone/PetApp/App/PetAppApp.swift:37-40` + `iphone/PetApp/Features/DevTools/Views/HealthProviderProbeView.swift:.task`

### 症状（Symptom）

UITest 启动 app（带 `-PetAppPreseedHealthKitSteps 5000` + `-PetAppRunHealthProviderIntegrationProbe`）；
慢模拟器（CI / 第一次冷启动）路径下：

```
T+0ms: PetAppApp.init → Task.detached { try? await HealthKitDevSeedUseCase.preseedToday(steps: 5000) }
T+0ms: PetAppApp.body → mount HealthProviderProbeView
T+5ms: ProbeView.task → readDailyTotalSteps(date: Date()) → HKStatisticsQuery 命中 0 sample
T+50ms: probe label 显示 "0"
T+150ms: detached seed 完成 → HKHealthStore.save(5000步) 落库
       但 probe view 已经显示 "0" 了，UITest 拿到错误断言
```

UITest 间歇 fail 而代码看起来"应该工作"。

### 根因（Root cause）

App init 期 dispatch detached Task 是经典的 Swift "fire-and-forget" 反模式 —— **没有任何 await
点告诉消费者"seed 已经做完"**。SwiftUI `.task` modifier 不等 init 期任何外部 async 工作，
它只串行执行 closure 自己的 await 链。

更深的根因：**测试种子（seed）是测试的 prerequisite，必须出现在测试自己的 setup 链上**，
不能放在被测系统的初始化路径上靠"应该比 read 快"赌时序。SeedUseCase 是 async，
正确的做法是把 seed 步数注入到 probe view，由 probe view `.task` 在 read 之前 `await` 完成 seed。

类似坑（其他语言）：

- iOS UITest 里在 AppDelegate 启 GCD `dispatch_async` 做 fixture seed → probe page 读到 stale data
- Android Espresso 里在 Application.onCreate 启 coroutine seed → activity 已经 onResume 触发查询

### 修复（Fix）

```swift
// PetAppApp.swift —— 不再 detached：
let preseedStepsForProbe: Int?
init() {
    if /* parse launch arg */ steps > 0 {
        preseedStepsForProbe = steps   // 不在这里 seed；只记录步数
    } else {
        preseedStepsForProbe = nil
    }
}
var body: some Scene {
    WindowGroup {
        if useHealthProviderProbe {
            HealthProviderProbeView(
                healthProvider: probeHealthProvider,
                preseedSteps: preseedStepsForProbe   // 注入
            )
        } else { RootView() }
    }
}

// HealthProviderProbeView.swift —— .task 入口先 await seed 再 read：
.task {
    if let steps = preseedSteps {
        try? await HealthKitDevSeedUseCase.preseedToday(steps: steps)
    }
    _ = try? await healthProvider.requestPermission()
    do {
        let value = try await healthProvider.readDailyTotalSteps(date: Date())
        // ...
    }
}
```

要点：

- seed 失败 swallow（`try?`）—— seed 失败 → read 返回 0 / permissionDenied 都是 AC7 钦定的 wired-up 路径
- probe view 的 `.task` 是单一 actor 上下文 —— seed 后 read 一定看得见 seed 写入的 sample

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写 UITest fixture seed（HealthKit / CoreData / Keychain / file system）** 时，
> **必须** 把 seed 串到测试 view 的 `.task` 入口或 setup 闭包，**禁止** 在 App.init / AppDelegate.didFinishLaunching
> 用 `Task.detached` / `Task { ... }` / GCD async 启动 seed 后立刻渲染依赖该 seed 的 view。
>
> **展开**：
> - "Detached fire-and-forget seed in app init" 是 SwiftUI/UIKit/任何 lifecycle-driven 框架的反模式 ——
>   no causality between seed 完成 与 view 第一次 query
> - 正确模式：seed 数据 → 注入 view → view `.task` 内 **串行 await** seed → read → 更新 a11y label
> - 测试容忍 seed 失败：`try? await seed(...)`（让真实 read 路径自己暴露问题，比 view 拒绝渲染更易诊断）
> - **反例**：
>   - `init() { Task.detached { try? await seedKeychain() } }` + `.task { let v = try await readKeychain() }` ——
>     race，seed 100% 时间晚于 read 在慢机器上发生
>   - `applicationDidFinishLaunching` 里 `DispatchQueue.global().async { seedDB() }` + 立刻 push first VC 调 query ——
>     同样 race
>   - `Task { await seed(); MainActor.run { state.ready = true } }` 在 init 期 —— 仍 race，因为
>     `init() { Task { ... } }` 不阻塞 `body` 计算

## Lesson 2: HK / 健康数据类 provider 的"日内缓存"是反优化 —— 用户当天会持续走步，缓存让读取永远卡在第一个值

- **Severity**: medium (P2)
- **Category**: architecture / correctness
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Health/HealthProviderImpl.swift:59-64` + `:94-98`

### 症状（Symptom）

```
T=09:00 sync 1: readDailyTotalSteps(today) → HK 查到 1000，cache (dayStart=今天00:00, steps=1000)
T=12:00 用户走了 3000 步（HKHealthStore 累积到 4000）
T=12:00 sync 2: readDailyTotalSteps(today) → cache hit (dayStart 仍是今天00:00) → 返回 1000  ❌
T=15:00 用户走了 5000 步（HKHealthStore 累积到 9000）
T=15:00 sync 3: readDailyTotalSteps(today) → 仍返回 1000  ❌
T=次日00:01 sync 4: readDailyTotalSteps(today) → dayStart 变了，cache invalidate，返回新值
```

下游 Story 8.5 `SyncStepsUseCase` 一天 sync N 次，每次都拿到上午 9 点那个 1000 → server 永远不入账新增量。
用户看到步数停滞 → severe UX 退化。

### 根因（Root cause）

代码注释把这个缓存当成"避免重复 HK 查询"的性能优化（Story Dev Notes 也这么写）：

```swift
// 同一天读两次走 cache（避免重复 HK 查询）；跨自然日（系统时间过 0 点）必 invalidate cache 重新查询
```

但 **HKStatisticsQuery 本身是廉价的**（Apple-钦定 efficient API；底层是 indexed lookup），
而"健康数据"语义本来就是单调累加 + 持续变化。**整日只读一次**违背了 provider 的核心契约
"返回当下时刻为止的累计值"。

更深的根因：缓存的 invalidation 维度和数据变化维度**不匹配** ——
- HKHealthStore 数据可以**每秒**被系统步数收集器追加 sample
- cache 的 invalidation key 是 `dayStart`，每天只变一次

只有当数据**到了某个时间点就不再变化**（如"昨天的累计"、"已结算的对账单"）时，按 dayStart 缓存才正确。
"今天到此刻为止"是流式累加场景，**禁用任何按日缓存**。

### 修复（Fix）

```swift
// HealthProviderImpl.swift —— 直接删除 cache 三件套：
// ❌ 删除：private let cacheLock = NSLock()
// ❌ 删除：private var cachedDayStart: Date?
// ❌ 删除：private var cachedSteps: Int?
// ❌ 删除：read 入口的 cache hit 分支
// ❌ 删除：read 末尾的 cache 写入

public func readDailyTotalSteps(date: Date) async throws -> Int {
    // 计算 dayStart / dayEnd
    // 直接 HKStatisticsQuery → return value
    // （没有 cache hit / write）
}
```

附带收益：删 NSLock 顺手消掉 3 个 Swift 6 async-context warning（`NSLock.lock()` 在 async 函数体内
跨 await 持有是 Swift 6 strict concurrency 的 future error）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **System Adapter（HealthKit / CoreLocation / 传感器 / 持续 stream 数据源）的 read 路径**
> 上写"同日 / 同小时缓存"时，**必须** 先证明数据源在该窗口内**事实上不会变化**（由 OS 契约或文档证明）；
> **禁止** 默认按"自然日切桶"缓存累积型数据。
>
> **展开**：
> - 累积型数据（步数、距离、心率小时摘要）—— 整个时间窗口内随时可能有新 sample 追加 → **禁止 cache**
> - 静态切片数据（昨天的累计、上一小时的总和、已 finalize 的对账数据）—— 可以缓存，但 invalidation key 必须是
>   "切片关闭时间点"，不是"第一次读的时间点"
> - 性能优化"避免重复 HK 查询"听起来合理 —— 但 HKStatisticsQuery / Core Data fetch 这类 indexed query
>   benchmark 通常在 5ms 内，本就不是瓶颈
> - 如果真的需要节流（如 polling 频率太高），用 **rate-limit / debounce** 在调用方而非 provider 内缓存
>   —— 调用方知道自己的 polling 节奏；provider 不知道、容易死锁
> - **反例**：
>   - `if cached.day == today { return cached.value }` 包裹 HK / 步数 / 实时传感器 read
>   - `redis.SET("step:userId:today", value, ttl=24h)` 在 step API 上（应该 TTL=0 / 不 cache，让每次查
>     都重新计算累计）
>   - `@MainActor class StepCache { var todayValue: Int? }` 在 ViewModel 层缓存"今日步数"——
>     view 重新出现时永远显示旧值

---

## Meta: 本次 review 的宏观教训

两条 finding 表面无关，但本质都是**"给 async / streaming 数据源加错时序假设"**：

- Lesson 1：把 seed 当成"App init 期一定会先于 view query 完成"的假设错了 —— async 工作没有强制时序，必须显式 await
- Lesson 2：把"同自然日内 HK 数据不变"的假设错了 —— streaming source 在窗口内随时变化，按窗口缓存破坏 freshness

未来 Claude 在 review 任何"先做 X → 再做 Y、X 是 async"的代码时：
**必问**「Y 怎么知道 X 完成了？」如果答案是"应该比 X 快/慢"或"调度器会让 X 先跑"——这就是 race。
正确答案永远是"Y 显式 await X 的 Task / Future / Continuation handle"。
