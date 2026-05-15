---
date: 2026-05-15
source_review: codex review on Story 21-1 r3 → r4
story: 21-1-首页宝箱组件-swiftui
commit: 1d7c39c
lesson_count: 1
---

# Review Lessons — 2026-05-15 — Server-anchored time：device clock skew 时 unlockAt 派生破坏 source of truth

## 背景

Story 21-1 第 4 轮 codex review，针对 `ChestTimerDriver.swift:102` `recomputeAndWrite(unlockAt:)` 的时间源选择。

这是**新缺陷类**（与 r1/r2/r3 的 "hydrate flicker" 不同轴）：r1-r3 都在修"frame-level timing 一致性"，本轮指出"**source of truth for time 错了**"——CLAUDE.md "状态以 server 为准"原则被违反。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 用 `unlockAt - Date()` 派生 remainingSeconds → device clock skew 时误显示 unlockable | P2 | architecture | fix | `iphone/PetApp/Features/Home/ViewModels/ChestTimerDriver.swift:102` |

## Lesson 1: device clock skew 下，`absoluteTime - now()` 不是合法的 countdown 时间源

- **Severity**: P2 (medium)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/ViewModels/ChestTimerDriver.swift:102`

### 症状（Symptom）

device 时钟与 server 时钟存在 wall-clock skew（例如：device 时钟比 server 快 5 分钟、用户手动改时区、NTP 漂移）时：

- server 返回 `chest.remainingSeconds=180`（即 server 视角倒计时还剩 3 分钟）+ `chest.unlockAt=server_now+180s`
- iPhone 端 driver 用 `unlockAt.timeIntervalSince(Date())` 派生：因为 device `Date()` 比 server 快 5 分钟，算出 `180 - 300 = -120` → 钳到 0
- ChestCardView `isUnlockable(.counting, 0) == true` → 立即显示金色 unlockable 卡片，**视觉上倒计时直接归零**
- 用户点开箱 → 但 server 仍判定 .counting（server 时钟视角还没到 unlockAt）→ 返回业务错误 → 用户困惑

更糟糕的相反场景：device 时钟比 server **慢**5 分钟 → driver 算出的 remainingSeconds 永远大于真实值 → 视觉倒计时永远到不了 0，用户错过开箱窗口。

### 根因（Root cause）

**`unlockAt`（server 计算的绝对时间）与 `Date()`（device 本地绝对时间）属于两个不同时钟域**。两个不同时钟域的绝对时间相减是无意义的 —— 差值不再是"剩余时间"，而是"剩余时间 + 时钟偏差"。

r0 dev-story 选 `unlockAt - Date()` 路径的思维漏洞：
- "server 已经给了 absolute unlockAt，那 driver 直接拿当前时间相减就行" —— 但 server 算的 `unlockAt = server_now + remainingSeconds`，server 的 `server_now` 与 device 的 `Date()` **不是同一个 clock**
- 误把 `unlockAt` 当成"客观绝对时间"，实际它是"server clock 视角的绝对时间"
- 触发场景隐蔽（开发环境 device 和 server 时钟通常都被 NTP 同步），生产场景才暴露（用户自己改时间 / 设备 NTP 失联 / 跨时区飞行）

CLAUDE.md 「工作纪律」明文写："**状态以 server 为准**：步数余额、宝箱状态、背包归属、合成结果、房间成员关系都以 server 响应为最终态" —— `remainingSeconds` 是 server 算好的真实剩余秒数，直接拿来用就是 "以 server 为准"；用 `unlockAt - Date()` 派生是 "以 device 为准 + 拿 server 数据校准"，方向反了。

### 修复（Fix）

**方案 A（采用）**：**server-anchored time**——hydrate 时刻捕获 `(hydratedAt, anchorRemaining)` 锚点，之后所有 displayed 都从这两个锚点派生。

```swift
// before (r0)
private func recomputeAndWrite(unlockAt: Date) {
    let remaining = max(0, Int(unlockAt.timeIntervalSince(Date())))
    viewModel?.chestRemainingSeconds = remaining
}

// after (r4)
private var hydratedAt: Date?
private var anchorRemaining: Int?

private func handleChestChange(_ chest: HomeChest?) {
    // ... cancel old task ...
    hydratedAt = clock()
    anchorRemaining = chest.remainingSeconds  // server 真值
    recomputeAndWrite()
    // ... start tick task ...
}

private func recomputeAndWrite() {
    guard let hydratedAt, let anchorRemaining else {
        viewModel?.chestRemainingSeconds = 0
        return
    }
    let elapsed = Int(clock().timeIntervalSince(hydratedAt))
    let remaining = max(0, anchorRemaining - elapsed)
    viewModel?.chestRemainingSeconds = remaining
}
```

关键性质：
- **抗 wall-clock skew**：`clock()` 和 `hydratedAt` 都是同 device 同 clock 的快照 → 差值 = 真实"自 hydrate 起经过的秒数"，与 server/device 时钟绝对偏差无关
- **抗 background/foreground**：app 进后台 X 秒后回前台调 tick → `clock() - hydratedAt` 自动跳进 X 秒，displayed 追上正确 remaining，无需额外 lifecycle hook
- **chest id 变 → 重新 anchor**：新 chest hydrate（或 Story 21.2 60s 定时拉取覆盖）时，`anchorRemaining` 更新为新 server 真值，driver 跟随

**配套设计：clock 注入（DI）**：

```swift
public init(
    appState: AppState,
    viewModel: HomeViewModel,
    clock: @escaping () -> Date = { Date() }
)
```

prod 用默认 `{ Date() }`，测试注入 mock clock 函数。理由：
- 不引第三方 clock-abstraction 库
- 不侵入 `AppState` / 其他无关接口
- 闭包形态比 protocol 更轻，prod 零开销
- 测试可推进时钟验证 anchor 派生公式

**配套测试**（双 case 守门）：

1. `testChestTimerDriverUsesServerRemainingSecondsAtHydration`：构造 `unlockAt` 在 device-clock 过去（模拟 device 时钟快 vs server）+ `remainingSeconds=180` → 断言 `chestRemainingSeconds = 180`（**不**派生自 `unlockAt - Date()` 否则为 0）
2. `testChestTimerDriverTickDecrementsFromServerAnchor`：推进 fake clock 60s → 触发同 id chest 重新 hydrate（带新 server `remainingSeconds`）→ 断言 displayed 立即写新 anchor。最后用极端 case（`unlockAt` 拨到 device-clock 过去 + `remainingSeconds=90`）验证：若退化为 `unlockAt - Date()` 会算出 0，正确实现写 90。

反弹守门：任何未来 PR 改回 `unlockAt.timeIntervalSince(Date())` 派生 → 两测试立刻挂。

**未选用的方案**：

- **方案 B（同步 server clock）**：客户端定期拉 server time，本地维护 clock-offset。**否决**：复杂度爆炸（offset 漂移、多次拉取的并发、首次启动还没拉就要 render）；对单一倒计时场景过度
- **方案 C（每次都从 server 拉 remainingSeconds）**：依赖每秒一次 API call，违反 Story 21.2 "60s 拉取" 设计 + 离线场景不可用
- **方案 D（用 `CFAbsoluteTimeGetCurrent` 替 `Date()`）**：还是 device clock，没解决问题
- **方案 E（用 `ProcessInfo.systemUptime` 单调时钟）**：抗 wall-clock skew，但 app 杀掉重启后清零、不能跨 app lifecycle 持久化。MVP 阶段 chest 倒计时只在 app live 期间显示，方案 A 的 anchor 已经够；未来如要 hydrate 时连"app 启动前 server 已下推的 unlockAt"都准确，再考虑混合 monotonic clock

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **客户端要把"绝对时间"渲染成"相对剩余时间"** 时，**禁止**用 `serverAbsoluteTime - Date()` 派生；**必须**让 server 直接下推 `remainingSeconds`（或类似 relative 字段），客户端用 `(hydratedAt, anchorRemaining)` 锚点 + 本地 elapsed 派生。

> **展开**：
>
> - **判定条件**：server 给出的 `xxxAt: Date` 字段是 server 时钟视角的绝对时间；client 的 `Date()` 是 device 时钟视角的绝对时间。**两者不是同一时钟域**。相减无业务意义。
> - **正确公式**：`displayed = max(0, anchorRemaining - Int(now - hydratedAt))`，其中 `now` 和 `hydratedAt` 都用同 device 同 clock provider；`anchorRemaining` 由 server 直接下推。
> - **DI 注入 clock**：driver 类 init 接收 `clock: () -> Date = { Date() }`，prod 用默认值，测试注入 mock —— 比 protocol 轻、零运行时开销、测试推进时钟即可验证 anchor 公式。
> - **chest id 变 → 重新 anchor**：domain object 切换时立即重新捕获 `(hydratedAt, anchorRemaining)`；不要复用旧锚点（旧锚点对应旧 domain object 的语义）。
> - **测试守门**：构造一个 device-clock 与 server 不一致的极端场景（`unlockAt` 在过去 + `remainingSeconds` 是正值），断言 displayed = `remainingSeconds`。任何"`unlockAt - Date()`"退化的实现都过不了。
> - **CLAUDE.md 锚定**：「状态以 server 为准」—— `remainingSeconds` 是 server 算好的状态字段，**直接采信**。`unlockAt` 是辅助字段（可用于 future-proof 但不作为 derivation source）。
> - **反例 1**：dev-story 阶段写 `Int(unlockAt.timeIntervalSince(Date()))` "因为 V1 接口设计里 unlockAt 看起来更精确" —— V1 接口设计同时提供 `unlockAt` + `remainingSeconds` 不是冗余，是 source-of-truth 分层（`remainingSeconds` 给客户端 render，`unlockAt` 是 future-proof 辅助 / 给可能的 watchOS complication 使用）.
> - **反例 2**：测试用真实 `Date()` 跑 + sleep 等真时钟流逝验证倒计时 —— 测出的是 eventually consistent，捕捉不到 clock skew 类缺陷。必须**注入 fake clock + 推进**.
> - **反例 3**：用 `unlockAt` + 一个 "clockOffset" 偏移量纠正 —— 还是间接派生，复杂度爆炸（offset 怎么测、怎么更新、首次启动取什么值）；server-anchored 一步到位.
> - **泛化领域**：所有 "client 渲染相对时间"场景都适用：宝箱倒计时（本 lesson）/ 房间倒计时 / cooldown / 验证码有效期 / 限时活动结束倒计时 等. 凡是 server 已经算好 remaining 字段的，直接采信；server 只给绝对 At 字段的，让 server 加一个 remaining 字段（或客户端用 monotonic clock 自维护 elapsed）。

---

## Meta: 本次 review 与 r1-r3 的轴关系

Story 21-1 的 4 轮 review 修了**两类不同的缺陷**：

- **r1/r2/r3：frame-level timing 一致性**（view-state 与 domain state 同步派生）
  - r1: 业务真值表错（默认值 0 派生）
  - r2: 加同步初始化
  - r3: 删 `.receive(on:)` 异步 hop
- **r4（本轮）：source of truth for time**
  - r4: server-anchored 替 `unlockAt - Date()`

判定方式：r1-r3 都是改"何时写 chestRemainingSeconds"；r4 改"写什么值"。前者是 timing，后者是 semantic correctness。两轴正交，r4 不是 over-correction chain 的延续，是新缺陷类。

防 over-correction 链路的关键判断：**新轮 review 的 finding 是否能用现有"双轴守门测试"自然 cover？** 不能 → 新缺陷类 → 加新测试（本轮 case#9 + case#10）；能 → over-correction → 暂停修复，先看现有测试为何没挡住。
