---
date: 2026-04-30
source_review: file:/tmp/epic-loop-review-37-11-r1.md (codex round 1, Story 37.11)
story: 37-11-profileview-scaffold
commit: 2929d78
lesson_count: 1
---

# Review Lessons — 2026-04-30 — Real ViewModel transient state 必须在 appState reset 路径同步清回 defaults（不只重算派生字段）

## 背景

Story 37.11 ProfileViewScaffold 落地 `RealProfileViewModel`，sink 订阅 `appState.$currentUser` + `appState.$currentPet` 派生 `profile`（name / petName / id 等真实字段）。但 transient UI state —— `wechatBound` / `showBindModal` / `lastToastMessage` —— 不在 sink 派生路径里，是用户交互（`onWeChatBindConfirmTap` / `onWeChatCardTap`）本地 mutate 出来的。codex round 1 指出：`ResetIdentityViewModel.tap() → appState.reset()` 后，`profile` 因 sink 重算回到 defaults，但 transient state 残留旧用户的值（如 `wechatBound = true`、`lastToastMessage = "微信绑定（敬请期待）"`），跨会话污染下一用户的 UI。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | RealProfileViewModel reset 路径未清 transient state | medium (P2) | architecture | fix | `iphone/PetApp/Features/Profile/ViewModels/RealProfileViewModel.swift` |

## Lesson 1: Real ViewModel transient state 必须在 reset 路径同步清回 defaults

- **Severity**: medium (P2)
- **Category**: architecture / state-management
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Profile/ViewModels/RealProfileViewModel.swift:90-129`

### 症状（Symptom）

- `RealProfileViewModel` 用 `Publishers.CombineLatest(appState.$currentUser, $currentPet)` 派生 `profile` 字段，reset 路径下 `profile` 自动 fallback 到 `ProfileScaffoldDefaults.profile`（因 sink 用 `?? defaults` 兜底）。
- 但 `wechatBound` / `showBindModal` / `lastToastMessage` 是用户交互方法本地 mutate 的状态，不在派生 sink 里。
- 测试链路：`appState.applyHomeData(...)` → 用户点未绑卡 → `onWeChatBindConfirmTap()` → `wechatBound = true` + `lastToastMessage = "..."` → `appState.reset()` → `wechatBound` **仍是 true**（污染下一会话的初始 UI）。

### 根因（Root cause）

派生字段（`profile`）和 transient 字段（`wechatBound` / `showBindModal` / `lastToastMessage`）有**两类状态**，但只有**一类**接到 publisher：

1. **派生类**：sink 订阅 publisher，每次 publisher fire 都从源真理重新计算 → reset 路径自动覆盖。
2. **Transient 类**：纯 ViewModel 内部状态，由用户交互方法 mutate；与 publisher 无连接 → reset 时无人通知。

写 ViewModel 时容易"派生字段订阅完就以为 ViewModel 状态全管了"。但 transient 状态本质是"会话级 UI 临时态"，会话边界（user → nil）必须显式清理。这条边界在派生字段 sink 里看不见，因为派生字段的 fallback 链刚好掩盖了"reset 没人清"这件事 —— 派生字段的 reset 行为是"凑巧对"，transient 字段就裸露在外。

### 修复（Fix）

**单独订阅 `appState.$currentUser`**（与 `CombineLatest` 派生 sink 并存），监听 `user == nil` → 清 transient state 回 defaults：

```swift
// round 1 codex review [P2] 修复：监听 currentUser → nil（reset 路径）清 transient state.
appState.$currentUser
    .sink { [weak self] user in
        guard let self else { return }
        if user == nil {
            self.wechatBound = false
            self.showBindModal = false
            self.lastToastMessage = nil
        }
    }
    .store(in: &profileSubscriptions)
```

**为何不合并进 `CombineLatest` sink**：profile 派生 sink 的语义是"派生 profile 字段从 user/pet 真理"；transient 清理是"会话边界事件触发的副作用"。两者职责正交，混在一起未来重构 profile 派生时容易顺带搞砸 transient 边界。单独 sink 让"reset 清 transient"语义更显式可读。

**guard test**：`testRealProfileViewModelResetClearsTransientState` 走完整路径：`applyHomeData → onWeChatCardTap → onWeChatBindConfirmTap → 断言 wechatBound=true → appState.reset() → 断言 wechatBound=false / showBindModal=false / lastToastMessage=nil`。

测试 318/318 pass（baseline 317 + 1 新 guard）。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在写 **Real ViewModel sink 订阅 appState publisher** 时，**必须分清"派生字段（subscribe-derive）"和"transient 字段（用户交互 mutate）"两类 state**，并**为 transient 字段加单独的 `appState.$currentUser` reset sink**，监听 `user == nil` 时清回 defaults。

> **展开**：
> - **触发条件**：ViewModel 同时持有派生字段（如 `profile.name = user?.nickname ?? defaults`）+ transient 字段（如 `showBindModal: Bool` / `lastToastMessage: String?` —— 由 onTap 方法 mutate，不在 publisher 派生链里）。
> - **必做**：在 `subscribeXxx(to:)` 内除了派生 sink 外，**追加** 一条 `appState.$currentUser.sink` 监听 `user == nil` → 清 transient state 回各自 defaults。
> - **不做**：把 transient 清理混进派生 sink（违反正交性，未来重构派生逻辑时容易丢清理动作）。
> - **保险测试**：guard test 走"hydrate → 用户交互改 transient → reset → 断言 transient 回 defaults"完整路径；不能只测 `init` 后的 transient defaults（那是初始态，与 reset 路径不同代码路径）。
> - **`@Published` 同步语义**：`appState.$currentUser` 是 `@Published` projected publisher，**订阅时立即 emit 当前值**。若初始 `currentUser` 为 nil，sink 立刻触发清 transient —— 这正好与 init seed 的 defaults 一致（`ProfileScaffoldDefaults.wechatBound = false` 等），是无害 no-op。**反过来若 transient defaults 不全是"falsy"**（比如某天某 default 改成 `lastToastMessage = "欢迎"`），订阅时立即覆盖会触发 false negative —— 此时改成"由非 nil → nil 的反向边触发"语义（用 `dropFirst()` 或保存 prev value 比较）。
> - **反例 1**（本 P2 修前）：只在 `CombineLatest($currentUser, $currentPet)` 派生 sink 里更新 `profile` —— transient 字段没人清，reset 后污染下一会话。
> - **反例 2**：把 reset 清理塞进 `appState.reset()` 自己（让 AppState 反向调用 ViewModel 清 transient）—— 违反 ADR-0010 单向数据流（AppState 是真理源，ViewModel 订阅它，反向耦合 = 反模式）。
> - **反例 3**：依赖 `RootView` 重建 ViewModel 替代清理 —— `@StateObject` 跨 root view 重建保持实例（SwiftUI 默认行为），不会因 currentUser nil 重建；不能假定"ViewModel 实例会被重新构造一次"。

## Meta: 本次 review 的宏观教训

Story 37.11 同模式的 lesson 已经积累 4 条（`real-viewmodel-init-must-seed-scaffold-defaults` / `published-derived-state-needs-publisher-subscription` / `real-viewmodel-override-placeholder-must-mutate-state` / 本条），全部围绕"Real ViewModel × AppState publisher × ViewModel 内部 state"三角。共同模式：**ViewModel 持有的每个字段都要回答"是派生 / transient / 缓存"中哪一类，并对每一类都明确 hydrate / reset / 用户交互 三条路径如何 mutate**。下次写新 Real ViewModel（如 Story 37.12 / 后续 Epic）请用此三角自检，避免再出同类 P2。
