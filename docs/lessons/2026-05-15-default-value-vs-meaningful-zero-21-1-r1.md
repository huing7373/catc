---
date: 2026-05-15
source_review: codex review file /tmp/epic-loop-review-21-1-r1.md (epic-loop r1)
story: 21-1-首页宝箱组件-swiftui
commit: c2526d6
lesson_count: 1
---

# Review Lessons — 2026-05-15 — 默认值 0 vs 业务意义上的 0：状态派生不能让 view-state 默认值伪装成业务状态（21-1 r1）

## 背景

Story 21.1 落地 SwiftUI 首页宝箱组件（`ChestCardView` + `ChestTimerDriver` + `HomeViewModel.chestRemainingSeconds`）。codex r1 抓到 `ChestCardView.isUnlockableForTesting` 的派生逻辑用 `(status == .unlockable) || (remainingSeconds <= 0)`：当 `.counting` 宝箱刚 hydrate（`AppState.currentChest` 写入完成）但 `ChestTimerDriver` 的 Combine sink 还没把真实 `remainingSeconds` 写到 `HomeViewModel` 之前，`chestRemainingSeconds` 仍是 `@Published` 默认值 `0`，view 错把它当作"倒计时已到 0 → unlockable"，闪一帧金色 + "开宝箱" 按钮——直接违反 server 权威态（`.counting` 不可开）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 默认值 0 与"倒计时超时 0"语义无法区分，导致 hydrate 阶段错渲 unlockable | medium (P2) | architecture / state-derivation | fix | `iphone/PetApp/Features/Home/Views/ChestCardView.swift:127` |

## Lesson 1: 默认值 0 vs 业务意义上的 0 —— view-state 默认值不能参与业务态派生

- **Severity**: medium (P2)
- **Category**: architecture / state-derivation
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/Views/ChestCardView.swift:127`（`isUnlockableForTesting`）

### 症状（Symptom）

`.counting` 宝箱第一次 hydrate 到 `AppState.currentChest` 后,`HomeView` body 先重新求值;此时 `HomeViewModel.chestRemainingSeconds` 还是 `@Published Int = 0` 的默认值（`ChestTimerDriver` 的 Combine sink 异步派发到 main runloop 还没跑）。`ChestCardView.isUnlockableForTesting(status: .counting, remainingSeconds: 0)` 命中 `remainingSeconds <= 0` 短路 → 返回 `true` → 渲染金色 unlockable 卡片 + 开箱按钮。等 driver 写入真实秒数（如 300）后下一帧 body 才纠正回 counting 视觉。用户看到一帧的 "可开启" 闪烁，按钮可点（虽然 onOpenTap 是占位 `{}` 不会发生副作用，但视觉契约已违反）。

### 根因（Root cause）

设计 `isUnlockableForTesting` 时把"两种独立的 0 语义"挤进一个分支：

1. **业务意义上的 0**：`.counting` 宝箱倒计时自然 tick 到 0（unlockAt ≤ now）—— 这是"已超时,客户端可乐观切 unlockable"
2. **view-state 默认 0**：`@Published Int = 0` 的初始值,在 driver 还没 sink 之前的占位值 —— 这**不是**业务状态,只是 SwiftUI `@Published` 初始化要求一个值

把两种 0 用同一条 `remainingSeconds <= 0` 短路掉,违反"状态派生只看权威源"原则——`status` 是 server 权威态（WS / 60s 轮询推送）,`remainingSeconds` 是 view-state 派生值(driver 算的)。让派生层混合两层语义会让"hydrate 时序"破坏视觉契约。

更深层的反模式：**乐观 UI 切换跨越了状态权威边界**。"倒计时到 0 客户端先切 unlockable,等 server 确认"听起来合理（响应快）,但代价是 view 层需要区分"我自己算出来的 0" vs "默认 0",而 SwiftUI 的 `@Published` 默认值机制天然不允许这种区分。

### 修复（Fix）

`ChestCardView` 派生改为**纯 status 判定**:

```swift
// before
internal static func isUnlockableForTesting(
    status: HomeChestStatus,
    remainingSeconds: Int
) -> Bool {
    return (status == .unlockable) || (remainingSeconds <= 0)
}

// after
internal static func isUnlockableForTesting(status: HomeChestStatus) -> Bool {
    return status == .unlockable
}
```

派生函数签名同步收窄（删 `remainingSeconds` 参数）；call site `content(for chest:)` 改 `isUnlockableForTesting(status: chest.status)`；测试 `testChestCardViewUnlockableDerivation` 把 4 个 case 收敛为 2 个（status 一个维度的真值表）。

`HomeViewModel.chestRemainingSeconds` 字段文档同步补 review r1 修订说明 —— 该字段仅供 counting 态 mm:ss 文案展示,**不**参与 unlockable 视觉态决策。

倒计时到 0 后的"乐观切 unlockable"由 server 端 WS push / 60s 轮询主动推 `status: .unlockable` 触发,driver 收 `currentChest` 更新后自然 react —— 让权威态留在 server,不让客户端越界派生。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **派生 view 视觉态时使用 `@Published` / `@State` 数值字段** 时,**必须**先问自己"这个字段的默认值（0 / "" / nil）和业务意义上的 0 是否能被调用方区分"——如果不能,**禁止**让该字段参与判定,只让权威 enum/status 字段参与派生。
>
> **展开**：
> - SwiftUI `@Published Int = 0` / `@Published String = ""` / `@Published Bool = false` 等默认值是**纯占位**,不是业务状态。"是否处于业务 0 态"必须由独立 enum / flag 表达（如 `.idle` / `.notLoaded` / `Optional.none`）,不能让数值字段双重承载。
> - 状态机派生层（如 `isUnlockable` / `shouldShow` / `canTap`）**只看 source-of-truth 字段**（通常是 enum status,由 server / 用户行为权威驱动）。view-state 派生字段（如 driver 算的相对秒数 / 本地 timestamp 差）**只能用于显示**,不能进入视觉态决策。
> - 跨权威边界的乐观 UI 切换（"客户端先变,server 后追"）是**反模式陷阱**——它要求 view 层区分"假状态" vs "真状态",但 SwiftUI 的 `@Published` 默认值机制天然不允许这种区分。要么完全信任 server 推送（本 lesson 选择）,要么用独立 flag 显式标"已 hydrate"（`hasLoaded: Bool = false`）让派生层 guard 它。
> - **反例 1**：`isUnlockable = (status == .unlockable) || (remainingSeconds <= 0)` —— 把"默认 0" 和"超时 0"混为一谈,hydrate 阶段闪烁不可避免。
> - **反例 2**：`isLoading = (data == nil)` —— 让 `data` 同时承载"未加载"和"加载完成但确实是空"两种语义。正确做法用独立 `LoadingState` enum 表达四态。
> - **反例 3**：`canTap = !buttonState.isEmpty` —— 让 `String` 默认 `""` 兼任"未初始化"和"业务空"。
> - **正例**：用 `HomeChestStatus` enum (`.counting` / `.unlockable`) 单字段权威,view 派生只看 enum,数值字段只做显示。

## Meta: 本次 review 的宏观教训

本次 review 单 finding,但暴露了一个**跨 epic 通用反模式**：**SwiftUI `@Published` 默认值不是中性占位,它会以业务状态身份参与派生**。Story 21.x 后续（21.2 LoadChestUseCase / 21.3 OpenChestUseCase）扩展 `HomeViewModel` 时如果加更多 `@Published` 数值字段（如 `chestStepCost` / `chestRetryCount`）,必须先问"默认 0 会不会被某个派生层误判"——若会,改 `Optional<Int>` 或加独立 `hasLoaded` flag。

同精神 lesson 在 server 端的对应物：`_bmad-output/implementation-artifacts/decisions/0010-domain-state-vs-view-state.md`（ADR-0010 §3.2 表格"哪些归 ViewModel @Published / 哪些归 AppState"已经划过线,但**没**强调"@Published 默认值的语义陷阱"——本 lesson 补这一刀）。
