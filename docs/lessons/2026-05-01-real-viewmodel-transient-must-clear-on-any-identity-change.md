---
date: 2026-05-01
source_review: file:/tmp/epic-loop-review-37-11-r2.md (codex round 2, Story 37.11)
story: 37-11-profileview-scaffold
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-01 — Real ViewModel transient state 清理判据必须用「user 身份变化」而非仅「user == nil」（cold-start 路径不经 appState.reset()）

## 背景

Story 37.11 round 1 codex review 已经把 `RealProfileViewModel` 的 transient state（`wechatBound` / `showBindModal` / `lastToastMessage`）清理 sink 加了 ——「订阅 `appState.$currentUser`，user == nil 时清回 defaults」（lesson `2026-04-30-real-viewmodel-must-clear-transient-state-on-reset.md`）。round 2 codex 指出该 fix 仍不完整：除了 `ResetIdentityViewModel.tap() → appState.reset()` 的「显式登出」路径之外，还有一条 **cold-start 路径** 同样是会话切换，但**不经过** `appState.reset()`：

- 401 触发 `AppLaunchStateMachine.triggerColdStart()`
- `RootView` 注入的 unauthorized handler 调 `SessionStore.clear()` + 重跑 bootstrap
- 重新自动登录拿到新 user → `appState.applyHomeData(用户 B)` 直接覆盖 `currentUser`
- **没人调** `appState.reset()` —— `currentUser` 直接 A → B（无 nil 中间态）

旧的 `if user == nil` 判据在这条路径下永远不触发，transient state（旧用户的 `wechatBound = true` / `lastToastMessage = "微信绑定（敬请期待）"`）跨会话泄漏到 B。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | RealProfileViewModel cold-start 路径（A→B 无 nil 中间态）transient state 泄漏 | medium (P2) | architecture | fix | `iphone/PetApp/Features/Profile/ViewModels/RealProfileViewModel.swift` |

## Lesson 1: Real ViewModel transient state 清理判据必须用「身份变化」而非仅「nil」

- **Severity**: medium (P2)
- **Category**: architecture / state-management
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Profile/ViewModels/RealProfileViewModel.swift:108-148`

### 症状（Symptom）

- round 1 fix 装的 sink：`appState.$currentUser.sink { if user == nil { 清 transient } }`。
- 失效路径：401 cold-start 不经 `appState.reset()`，`currentUser` 直接 A→B → sink 触发但 `user != nil` → 不清 transient。
- 结果：B 会话起步即看到旧 A 的 `wechatBound = true` / `showBindModal` / `lastToastMessage`。

### 根因（Root cause）

会话切换在 iPhone App 里有两条路径，**只有一条会把 currentUser 置 nil**：

| 路径 | 触发 | 是否经 `appState.reset()` | currentUser 轨迹 |
|---|---|---|---|
| 用户主动登出 | ResetIdentityViewModel.tap() | 是 | A → nil（→ 之后 nil → B 如果再登录） |
| 401 cold-start | AppLaunchStateMachine.triggerColdStart() + 重跑 bootstrap | **否** | A → B（直接覆盖，无 nil 中间态） |

round 1 修 transient 清理时只看到第一条路径就以为 nil 是充要条件 —— 其实 nil 只是「显式 reset」语义的副作用，真正的清理边界是**「会话身份变化」**（who 变了 = 任何 currentUser.id 切换，含 A→nil / nil→A / A→B / B→A）。把判据从 `user == nil` 升级到 `user?.id != lastObservedUserId`，三条边界全覆盖。

写 ViewModel 的 transient 清理 sink 时，思维容易绑死「reset 路径 = nil」一种心智模型，忘了「直接换 user」也是会话切换。SessionStore.clear() + bootstrap 重跑路径里只 mutate session token + 用户身份，从不调 AppState.reset()，正是因为 AppState.reset() 是「主动登出」语义，不是 cold-start 语义 —— 这两条路径的边界条件应该用相同的清理触发器。

### 修复（Fix）

在 sink 闭包中维护 `lastObservedUserId: String?`（`@MainActor` ViewModel 字段，sink 全部跑在 main thread 上无并发风险），判 `newUserId != lastObservedUserId` 触发清理 + 更新 last id：

```swift
private var lastObservedUserId: String?

appState.$currentUser
    .sink { [weak self] user in
        guard let self else { return }
        let newUserId = user?.id
        if newUserId != self.lastObservedUserId {
            self.wechatBound = false
            self.showBindModal = false
            self.lastToastMessage = nil
            self.lastObservedUserId = newUserId
        }
    }
    .store(in: &profileSubscriptions)
```

边界守卫验证（手算所有场景）：

| 场景 | last → new | 是否触发清理 | 是否符合预期 |
|---|---|---|---|
| parameterless init 后首次 sink emit（init 默认 currentUser = nil） | nil → nil | 否 | 是（已是 defaults，no-op） |
| init(appState:) 首次 sink emit（hydrate 已发生） | nil → "A" | 是 | 是（hydrate 时 transient 也应是 defaults，触发清理无害） |
| 主动登出 | "A" → nil | 是 | 是（reset 路径必清） |
| 重新登录 | nil → "A" | 是 | 是 |
| cold-start 切户 | "A" → "B" | 是 | 是（**round 2 关键修复点**） |
| 同会话刷新 user 字段（昵称改） | "A" → "A" | 否 | 是（id 没变，transient 不应被清） |

并补 1 条 guard test（`testRealProfileViewModelColdStartClearsTransientStateOnUserSwitch`）：`applyHomeData(用户 A)` → 用户交互让 transient 都 mutate → `applyHomeData(用户 B 不同 id)` 不调 reset → 断言 transient 全回 defaults。fixture helper `makeHomeDataFixture` 加可选 `userId` / `petId` 默认参数让两次 hydrate 区分身份。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **ViewModel 加 transient state 清理 sink** 时，**必须** 用「身份变化」（`newId != lastId`）而非「nil 哨兵」作为清理触发器，且 hold 一个 `lastObservedXXXId` 字段做边界判据。

> **展开**：
> - **触发条件识别**：ViewModel 字段如果分两类 ——（a）派生类（sink 订阅 publisher 自动 fallback）+（b）transient 类（用户交互 mutate，无 publisher 联动）—— transient 类必须显式找到「会话边界事件」并清理。
> - **会话边界 ≠ nil 边界**：iPhone App 里至少有两条会话切换路径：① 主动登出（经 `appState.reset()` → user nil）② 401 cold-start（经 `SessionStore.clear()` + bootstrap 重跑 → user 直接换 id 不经 nil）。判据要用 user 身份（`user?.id`）变化，不要绑在 nil 检查。
> - **lastObservedXXX 字段**：在 ViewModel（`@MainActor` final class）里加 `private var lastObservedUserId: String?`。sink 跑在 main thread → 无并发风险。每次 sink 触发都比较 + 更新。
> - **首次 sink emit 守卫**：`Published` publisher 一订阅就 emit 当前值。`lastObservedUserId` 初始 nil，首次 emit 如果 `user.id == nil` 也是 nil → 不触发清理（已是 defaults，no-op）；如果 user 已 hydrate（`init(appState:)` 路径） → nil → user.id 是身份变化 → 清理触发但 transient 已是 defaults → no-op。两种情况都正确。
> - **同身份内字段更新不应触发清理**：例如用户改了昵称（`user.nickname` 变但 `user.id` 不变），不应清 transient（用户的会话没换，UI 临时态应保留）。用 id 而非 user 全等做判据正好满足。
> - **反例**：写 `if user == nil { 清 transient }` 只覆盖 `appState.reset()` 路径，**不**覆盖 401 cold-start 路径（`applyHomeData(B)` 直接覆盖 `currentUser`，`user != nil` 不触发）→ 旧用户 transient 泄漏到新会话。

## Meta: 跨 review 轮次的 lesson 演进模式

round 1 和 round 2 同一个 issue 演进了两次：先识别出「transient state 不在 publisher 派生路径里」（round 1，`user == nil` 修法），再识别出「会话切换路径不止 nil 一条」（round 2，`id != lastId` 修法）。两轮 lesson 应该并行存在，不删旧 lesson —— round 1 lesson（`2026-04-30-real-viewmodel-must-clear-transient-state-on-reset.md`）是 transient/派生 二分类的根本认知；本 round 2 lesson 是其升级版细则。未来 Claude 读 round 1 lesson 时若漏读本 lesson，仍可能写出 `if user == nil` 的不完整判据 —— 提醒：**写 transient 清理 sink 前两条 lesson 都要读**。
