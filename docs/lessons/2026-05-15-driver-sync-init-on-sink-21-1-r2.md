---
date: 2026-05-15
source_review: codex review file /tmp/epic-loop-review-21-1-r2.md (epic-loop r2)
story: 21-1-首页宝箱组件-swiftui
commit: 9a603d6
lesson_count: 1
supersedes_partial: docs/lessons/2026-05-15-default-value-vs-meaningful-zero-21-1-r1.md
---

# Review Lessons — 2026-05-15 — Driver 必须在 sink 时同步初始化 view-state，否则 over-correction chain 永远收不住（21-1 r2）

## 背景

Story 21.1 r1 fix-review 把 `ChestCardView.isUnlockableForTesting` 从 `(status == .unlockable) || (remainingSeconds <= 0)` 简化为纯 status 判定（修了 hydrate 阶段闪烁 unlockable）。codex r2 review 抓到该修复**反弹**：driver tick 把 `chestRemainingSeconds` 减到 0 后，view 不再切 unlockable —— 一直渲染 counting 卡片直到 server 下一次 status push（Story 21.2 60s 轮询）。直接违反 `docs/宠物互动App_MVP节点规划与里程碑.md` epic 21 AC「倒计时归零时自动切到 unlockable 视觉状态」。

这是 over-correction chain 的第二跳，必须用**正交修复**终结循环，不能继续在派生层 ping-pong。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | r1 收敛为 status-only 导致本地 tick 归零无法切视觉态（epic AC 反弹） | medium (P2) | architecture / state-derivation | fix | `iphone/PetApp/Features/Home/Views/ChestCardView.swift:62`, `iphone/PetApp/Features/Home/ViewModels/ChestTimerDriver.swift:36` |

## Lesson 1: Driver 必须在 sink 时同步初始化 view-state —— 否则派生层无路可走

- **Severity**: medium (P2)
- **Category**: architecture / state-derivation / driver-init-timing
- **分诊**: fix
- **位置**:
  - `iphone/PetApp/Features/Home/Views/ChestCardView.swift:62`（isUnlockableForTesting 派生函数 + call site）
  - `iphone/PetApp/Features/Home/ViewModels/ChestTimerDriver.swift:36`（start() 同步初始化）

### 症状（Symptom）

r1 修复后的真值表：

| 场景 | status | remainingSeconds | isUnlockable | 用户感知 |
|------|--------|------------------|--------------|----------|
| 倒计时进行中 | `.counting` | 300 | false | 显示 mm:ss（正确） |
| **本地 tick 归零** | `.counting` | 0 | **false** | **仍显示 00:00 counting 卡片，等不到 unlockable**（坏） |
| server 推 unlockable | `.unlockable` | 0 | true | 显示金色卡片（正确） |

中间一行的"坏"是本轮 finding —— driver tick 一直把 `chestRemainingSeconds` 减到 0 后停在那（`recomputeAndWrite` 用 `max(0, ...)` 钳零），但 status 是 `.counting`，r1 派生只看 status → view 永远不切 unlockable。要等 Story 21.2 落地的 60s 轮询拉 server 才会把 status 推成 `.unlockable`。

epic AC 钦定的"倒计时归零自动切视觉态"在 21.1 阶段（无 21.2 轮询）完全失效。

### 根因（Root cause）

这是 **over-correction chain 的第二跳**。完整链路：

- **r0**（dev-story 提交）: `isUnlockable = (remainingSeconds <= 0) || (status == .unlockable)` —— r1 codex 报：hydrate 阶段 `chestRemainingSeconds=0` 默认值被误判 unlockable，闪一帧。
- **r1 fix**: 简化为 `isUnlockable = (status == .unlockable)` —— r2 codex 报：本地 tick 到 0 不切视觉态（反弹）。
- **r2 fix**（本 lesson）: 必须**同时**满足两个约束（不能再做第 3 跳）。

正交根因：r1 解法把"默认值 0 vs 业务 0"的混淆**单方面甩给派生层**解决（让派生层不看数值字段）。但 epic AC 要求派生层**必须**看 `remainingSeconds`（否则倒计时归零行为丢失）。两个约束不可调和 —— **除非从根上让 view 永远读不到默认值 0**。

正确的责任分配：

- **派生层** (`ChestCardView.isUnlockableForTesting`) **必须**做 status-aware 双轴判定：
  - `status == .unlockable` → true (server 权威态)
  - `status == .counting && remainingSeconds <= 0` → true (本地 tick 归零乐观切)
  - 否则 → false
- **初始化层** (`ChestTimerDriver.start()`) **必须**在 sink 之前同步用当前 `appState.currentChest` 跑一次 `handleChestChange`，让 `viewModel.chestRemainingSeconds` 在 start() 返回前就拿到 server 推下来的真实值（如 300），**不**等 Combine `.receive(on: .main)` 的下一 runloop。

两者**联合**保证：
- 视图永远读不到 `@Published Int = 0` 默认值（driver 同步写过了）→ hydrate 帧不闪 unlockable
- `remainingSeconds <= 0` 仅在 driver tick 之后才出现（语义上确实是"业务超时"）→ 派生切 unlockable 是 epic 钦定行为

### 修复（Fix）

**Step 1**: `ChestTimerDriver.start()` 同步初始化（核心架构点）：

```swift
public func start() {
    guard subscription == nil else { return }
    // 同步初始化：sink 前先用当前 currentChest 跑一次，让 chestRemainingSeconds
    // 在 start() 返回前就拿到正确初值.
    handleChestChange(appState?.currentChest)
    subscription = appState?.$currentChest
        .dropFirst()  // 跳过 @Published 首次发送（避免重复 cancel/restart tick task）
        .receive(on: DispatchQueue.main)
        .sink { [weak self] newChest in
            self?.handleChestChange(newChest)
        }
}
```

`.dropFirst()` 不可省 —— `@Published` 订阅会立即发当前值，我们已经手动调过 `handleChestChange` 一次，再让 sink 收到首发就会重复 cancel + 重启 tick task（虽然功能正确但浪费 + 增加 race window）。

**Step 2**: `ChestCardView.isUnlockableForTesting` 恢复 `remainingSeconds` 参数，内部 status-aware 判定：

```swift
internal static func isUnlockableForTesting(
    status: HomeChestStatus,
    remainingSeconds: Int
) -> Bool {
    if status == .unlockable {
        return true  // server 权威
    }
    return status == .counting && remainingSeconds <= 0  // 本地 tick 归零乐观切
}
```

**Step 3**: 测试扩到 ~6 case 真值表 + 新增 `testChestTimerDriverInitializesSynchronouslyOnStart` 验证 driver 同步初始化（无 `await` / `sleep` 直接断言 `start()` 返回后 `chestRemainingSeconds` 已正确）。

### 与 r1 lesson 的关系（不推翻，而是补充）

`docs/lessons/2026-05-15-default-value-vs-meaningful-zero-21-1-r1.md` 的核心论点「`@Published` 默认值不是中性占位」**仍然成立**。r1 lesson 漏掉的维度是 **"如何避免视图读到默认值"**，给出的解法只有「派生层不看数值字段」这一种。本 lesson 补充第二种解法：「让 driver 同步初始化让默认 0 永远不被 view 读到」。

两种解法的取舍：

| 解法 | 优点 | 代价 |
|------|------|------|
| (r1) 派生层不看数值字段 | 派生函数纯，单字段权威 | 丢失"本地倒计时归零自动切"行为（违反 epic AC） |
| (r2) Driver 同步初始化 | 保住 epic AC，派生函数能看双轴 | 初始化路径多一行同步调用 + `.dropFirst()` |

对于本 story，epic AC 是硬约束（"倒计时归零自动切视觉态"），所以只能选 r2。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **driver / subscriber 订阅 `@Published` 字段** 时，**必须**在 subscribe 之前**先用当前快照同步处理一次** —— 否则视图在第一帧会读到 `@Published` 默认值（`@Published Int = 0` / `@Published String = ""` 等），让任何依赖该字段的派生层在 hydrate 帧出现"假状态"。配合 `.dropFirst()` 跳过订阅时的首次自动发送。
>
> **展开**：
> - Combine `.receive(on: DispatchQueue.main)` 把 sink closure 派发到下一 main runloop —— 即使 `appState.currentChest` 已经在主线程写过了，sink fire **不**同步发生。中间一帧（SwiftUI body 第一次求值）会读到 `@Published` 默认值。
> - 修复模式：
>   ```swift
>   func start() {
>       handleChange(appState.currentValue)   // 同步处理一次
>       subscription = appState.$publisher
>           .dropFirst()                      // 跳过订阅时首发
>           .sink { [weak self] in self?.handleChange($0) }
>   }
>   ```
> - **反例**: 只调 `.sink` 不做同步初始化 —— 第一帧 view 读到默认 0 / "" / false / nil。
> - **正例**: 任何"driver 订阅 published 字段、把结果写到另一 published 字段供 view 读"的链路，都必须有同步初始化。
>
> **跨 layer 推论**：当 over-correction chain 进入第 2 跳（修了 A 又坏了 B）时，**必须停下来重新审视责任分配**，不要在原 layer 继续 ping-pong。正交修复往往要求 (1) 多个 layer 各自负责一个不变量；(2) 找到能让两个原本冲突的约束同时满足的初始化时序。本 lesson 是 (1)+(2) 的典型 —— 派生层负责"双轴判定"，初始化层负责"同步消除默认值"，两者联合产生 epic AC + 无闪烁的复合不变量。
>
> **反 over-correction 心智**: 当 review feedback 让你想"那干脆把 X 删了"时，先问 "X 是否承载了某个 spec 要求的行为"。如果是，**不要删 X**，而是找另一个 layer 修复让 X 能继续工作。

## Meta: 本次 review 的宏观教训

- **Over-correction chain 是真实的、可重复的**：epic-20 已经多次出现（Story 20.7 r3 / r4 / r5 / 20.9 r6 / r7 等），现在 21.1 又来一次。识别 trigger：上一轮 fix-review 用"删除 / 简化 / 收敛"语言描述的修改，下一轮 review 很容易找到 over-correction 反弹。
- **正交修复 = 多 layer 联合不变量**：单点修复不够时，必须问"是否有另一个 layer 能承担一部分不变量责任"。本 case 就是把"hydrate 帧不闪"和"tick 归零切"拆给 driver 初始化层和 view 派生层各做一个。
- **测试真值表是 over-correction 检测器**：把所有可能 input 组合列成表（本 case 是 status × remainingSeconds 的 6 case 真值表），逐行确认"业务期望" vs "实际行为"。r1 之所以反弹，是因为 r1 改派生函数后没扩测试覆盖 `(.counting, 0)` 这个 critical case。
