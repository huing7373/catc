---
date: 2026-05-04
source_review: codex review round 1 of Story 8-5-步数同步触发器（/tmp/epic-loop-review-8-5-r1.md）
story: 8-5-步数同步触发器
commit: 8f24404
lesson_count: 2
---

# Review Lessons — 2026-05-04 — 跨午夜时间字段必须从单一 captured Date 派生 & scenePhase 幂等 start 不能并排两条同效 API

## 背景

Story 8.5 落地 `SyncStepsUseCase`（拼 `/steps/sync` 请求体）+ `StepSyncTriggerService`（4 触发器：
launch / foreground / 5min timer / manual）+ RootView wire（`.onReadyTask` 调 `start()`,
`.scenePhase .active` 同时调 `triggerForeground()` + `start()`）。codex round 1 review 命中两条
正确性 finding：

1. 拼请求体时 `dateProvider.now()` / `todayString()` / `nowMillis()` 各自独立 fetch —— 跨午夜 race
2. RootView `.scenePhase .active` 路径同时调两个等效 trigger API —— in-flight gate 仅压重叠，
   第一个完成后第二个会真发出 duplicate `/steps/sync`

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 跨午夜时间字段 race（payload 构造）| medium (P2) | architecture | fix | `iphone/PetApp/Features/Home/UseCases/SyncStepsUseCase.swift:49-60` |
| 2 | scenePhase .active 双 API 触发 duplicate sync | low (P3) | architecture | fix | `iphone/PetApp/App/RootView.swift:273-274` + `Features/Home/Services/StepSyncTriggerService.swift` |

## Lesson 1: 跨午夜时间字段必须从单一 captured Date 派生

- **Severity**: medium (P2)
- **Category**: architecture / correctness
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/UseCases/SyncStepsUseCase.swift:49-69`

### 症状（Symptom）

`SyncStepsUseCase.execute` 内连续做 4 次时间相关动作：

1. `let now = dateProvider.now()` — 取本机 Date
2. `healthProvider.readDailyTotalSteps(date: now)` — 读 day1 总步数
3. `dateProvider.todayString()` — 内部又 `Date()` 一次拼 syncDate
4. `dateProvider.nowMillis()` — 内部又 `Date()` 一次拼 clientTimestamp

如果 step 1-2 完成后 main thread 跨过本机午夜（`HKStatisticsQuery` 慢 + 系统时钟 tick），step 3
取到 day2 的 "YYYY-MM-DD"，但 step 2 的总步数是 day1 累计 → server 收到 day1 总数被标 day2 →
触发 server 7.3 防作弊 reject 或写到错误的 `daily_steps` 行。

### 根因（Root cause）

DateProvider 协议把 `now() / todayString() / nowMillis()` 拆成 3 个独立方法是合理抽象（让单测分别
锁固定值），但 use case **盲目调 3 个方法等价于 3 次独立 `Date()` 取值** —— 中间任何 await 点都
可能跨过自然日 / 时区切换 / 闰秒边界。设计上忽视了"取到的 3 个值彼此应是同一瞬间快照"这条隐式
不变量。

### 修复（Fix）

让 use case capture **一次** `Date`，所有派生字段从这个 `now` 推：

```swift
// before（race-prone）
let now = dateProvider.now()
let totalSteps = try await healthProvider.readDailyTotalSteps(date: now)
let request = StepsSyncRequest(
    syncDate: dateProvider.todayString(),       // 又 Date() 一次
    clientTimestamp: dateProvider.nowMillis()   // 又 Date() 一次
)

// after（single capture）
let now = dateProvider.now()
let totalSteps = try await healthProvider.readDailyTotalSteps(date: now)
let request = StepsSyncRequest(
    syncDate: Self.formatLocalDateString(from: now),
    clientTimestamp: Int64(now.timeIntervalSince1970 * 1000)
)
```

DateProvider 协议**不动**（其它地方可能用 `todayString()` / `nowMillis()`，且单测 mock 仍方便）；
use case 自己用 Calendar / `timeIntervalSince1970` 派生。`formatLocalDateString` 私有静态 helper 与
`DefaultDateProvider.todayString()` 实现一致（gregorian / en_US_POSIX / TimeZone.current）。

测试同步：

- 旧 `MockDateProvider(now:, todayString:, nowMillis:)` stub 三独立值 → 改成"故意把 todayString /
  nowMillis 设成陷阱值"，断言 use case 完全不取它们（任意值都不影响 syncDate / clientTimestamp）
- expected syncDate 改用 mirror helper 计算（TZ-independent，CI UTC / 本地 CST 都过）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **同一个请求/响应/事务里需要多个时间相关字段**（如 `syncDate` +
> `timestamp`，或 day key + day string + ms epoch）时，**必须 capture 一次 Date 派生全部字段**，
> 禁止"调时间抽象的多个独立方法"看似干净实则埋 race。
>
> **展开**：
> - DateProvider / Clock 抽象的多个 query 方法（`now()` / `today()` / `nowMillis()`）**不是**
>   "同一瞬间"的保证——它们各自调 `Date()` / `Date.timeIntervalSince1970`，跨午夜 / 跨闰秒会
>   错位。
> - 在 use case / handler / 任何业务逻辑入口，第一行先 `let now = clock.now()`（或同义入口），
>   后续所有派生（dateString / msEpoch / startOfDay / weekStart）从这个 `now` 用 Calendar /
>   timeIntervalSince1970 推。如果抽象层没暴露入参版（`func todayString(from: Date)`）就在
>   caller 层加 private helper，不动 protocol。
> - 单测里"分别 stub 三个方法返回错位值"是反向用例 —— **断言 use case 不取它们**（陷阱值
>   不影响输出），而不是断言 use case 用 stub 值。前者锁的是不变量，后者锁的是实现细节。
> - **反例 1**：`request.syncDate = clock.todayString(); request.timestamp = clock.nowMillis()`
>   —— 两次独立 `Date()`，跨午夜 race。
> - **反例 2**：`let day = Calendar.current.startOfDay(for: Date()); ... let ts = Date()` ——
>   两次 `Date()` 之间 main thread 可能 await，跨午夜 race。
> - **正例**：`let now = clock.now(); req.syncDate = format(now); req.ts = ms(now); read(date: now)`
>   —— 同一瞬间快照传透。

## Lesson 2: scenePhase .active 不能并排调多条同效 API；让 start() 自身幂等

- **Severity**: low (P3)
- **Category**: architecture / concurrency
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift:273-274` + `iphone/PetApp/Features/Home/Services/StepSyncTriggerService.swift:74`

### 症状（Symptom）

RootView `.onChange(of: scenePhase)` 进入 `.active` 时同时调：

```swift
stepSyncTriggerService?.triggerForeground()  // spawn Task A
stepSyncTriggerService?.start()              // 内部又 spawn Task B（launch sync）
```

两个 Task 各自走 `performSync` 入口。in-flight gate (`isSyncing` flag) 只能压重叠：

- **真机慢网络**：Task A 还没完成，Task B 进入 → gate 压住 Task B → 只 1 次 sync ✅
- **mock / 快本地后端**：Task A 几毫秒就完成 → `isSyncing` 已 release → Task B 进入 → 第二次
  `/steps/sync` 真发出 ❌ duplicate

### 根因（Root cause）

`StepSyncTriggerService` 提供了两个语义重叠的入口：

- `start()` = "启 timer + 立即 sync once"
- `triggerForeground()` = "立即 sync once"

caller 端误以为两者必须**叠加**（"先 trigger 再 start"），但其实 `start()` 内已含"立即 sync"。
caller 不知道这个隐藏副作用 → 加倍调。in-flight gate 是兜底而非保证唯一性。

### 修复（Fix）

让 `start()` 自身充当幂等 reactivate 入口；caller 端只调 `start()`：

```swift
// service: start() 现在自带幂等语义
public func start() {
    let wasFirstStart = !hasStartedTimer
    startTimerIfNeeded()  // 内部 hasStartedTimer guard，重启不会拉新 timer
    Task { [weak self] in
        await self?.performSync(reason: wasFirstStart ? .launch : .foreground)
    }
}

// RootView: 删 triggerForeground() 调用
.onChange(of: scenePhase) { old, new in
    if new == .background { service?.stop() }
    guard new == .active, old != .active else { return }
    // 旧：service?.triggerForeground(); service?.start()
    // 新：只调 start()，自身幂等
    service?.start()
}
```

`triggerForeground()` 公开 API 保留（其它 caller 可能仍要纯触发不启 timer 的语义），但 RootView
不再用。新增 `testStart_idempotent_noDuplicateOnSecondCall` 覆盖：first start → 1 sync；second
start → 2 sync 累计（不漏不重）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **同一个 caller 路径（如 scenePhase listener / lifecycle hook）**
> 触发 service 时，**禁止并排调用两条副作用重叠的 API**（如 "starter + trigger"），应让 starter
> 自身幂等地包含 trigger 语义；caller 只调一次。
>
> **展开**：
> - "in-flight gate"（isSyncing / debounce 等）是**兜底防御**，不是单一性保证。caller 端 enqueue
>   两个独立 Task 时，gate 只压住时间重叠的那部分；只要第一个先完成，第二个就会"合法"地真触发。
> - service API 设计原则：暴露**正交**入口（`start()` 启 timer + 立即 sync 一次；`triggerOnce()`
>   单纯触发；`stop()` cancel timer），并文档明示重叠 caller 不要叠加调用。如果 caller 路径就是
>   "回前台都按一遍"，让那个路径的入口（通常是 `start()`）自身幂等吸收所有需求。
> - 幂等 starter 的实现模板：`hasStarted` guard 防 timer 重启 + `Task { performOnce() }` 每次调用
>   都 enqueue（performOnce 内 in-flight gate 决定是否真跑）。读 `hasStarted` 决定 reason 标签
>   （launch / reactivate）但**不**短路 sync —— "再启动" 必须意味着"再触发一次"。
> - 单测必须有 `idempotent_noDuplicateOnSecondCall`：连续 N 次 start() → 应触发 N 次 sync（每次
>   等待前一次完成）；**不**允许"第二次 start() silently 返回"，那会让 caller 漏触发回前台 sync。
> - **反例 1**：`onActive { service.triggerForeground(); service.start() }` —— 两个 Task spawn,
>   in-flight 仅压重叠，快路径 duplicate。
> - **反例 2**：`func start() { guard !hasStarted else { return } ... }` —— 第二次 silent return,
>   reactivate 路径漏触发，回前台不更新。
> - **正例**：`func start() { startTimerIfNeeded(); Task { performSync() } }` ——
>   timer 单次启动，sync 每次都 enqueue（in-flight 内决定是否真跑）。

---

## Meta: 本次 review 的宏观教训

两条 finding 共享同一个思维漏洞：**"看似冗余的多次调用其实是 race / duplicate 来源"**。

- Finding 1：3 次时间方法 fetch 看起来都返回"现在"，但 3 次"现在"不是同一瞬间。
- Finding 2：trigger + start 看起来都是"现在 sync 一下"，但是两个独立 Task。

防御都是同一个原则 —— **collapse 到单一 source**：

- 时间字段 → 单一 captured Date
- 触发入口 → 单一幂等 API

未来 Claude 写"同一个请求要好几个时间字段"或"同一个 lifecycle hook 要触发一系列 service action"
时，先问一句：**"这些值/触发是不是必须出自同一瞬间 / 同一个 caller intent？"**——如果是，
collapse 到单一入口；如果不是，就明确文档化"它们是独立的两件事"。
