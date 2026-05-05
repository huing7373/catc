---
date: 2026-05-04
source_review: codex review round 4 of Story 8.4 (file: /tmp/epic-loop-review-8-4-r4.md)
story: 8-4-主界面猫-sprite-三态动画切换
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-04 — auth-gated subscription 必须支持 downgrade，不能用单向 flag 短路 rebind

## 背景

Story 8.4 round 1 引入了"bind 前查 authorizationStatus + `hasStartedMotionUpdates` flag 防重复订阅"
（详见 docs/lessons/2026-05-04-motion-bind-must-gate-on-authorization-status.md）.
round 4 codex 抓出 follow-on bug：`hasStartedMotionUpdates` 是单向 flag——一旦置 true 就只允许"已订阅"
半边的状态机，permission downgrade（用户中途去 Settings 撤销 Motion 权限）后 rebind 会被
`guard !hasStartedMotionUpdates else { return }` 短路，老订阅不拆 + petState 卡 stale `.walk`/`.run`，
UI 永远错位直到 app 重启.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | bind 用 hasStartedMotionUpdates 单向 flag 短路 rebind，permission revoke 后老订阅不拆 + petState 卡 stale | P2 | architecture | fix | `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift` |

## Lesson 1: auth-gated subscription 的 rebind 必须能处理 downgrade（不只是 upgrade）

- **Severity**: P2 (medium)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift:373`（旧 guard 语句行）

### 症状（Symptom）

`HomeViewModel.bind(motionProvider:)` 用 `hasStartedMotionUpdates` 标志做"已订阅则短路 return"
的防重复保护. 但保护过强——只覆盖了"权限恒为 authorized 的 rebind"路径，没覆盖 downgrade：

1. first-launch 用户授权 Motion → bind 走 (subscribed=false, auth=true) → 调 startUpdates，flag=true.
2. 系统正常 deliver activity，petState 进入 `.walk` 或 `.run`.
3. 用户去 Settings 撤销 Motion 权限.
4. RootView 在 ScenePhase active 触发 rebind（Story 8.4 round 3 引入的 retry path）.
5. **bug**：bind 第一行 `guard !hasStartedMotionUpdates else { return }` 直接 return——没机会
   重查 `authorizationStatus()`，老订阅没 stop，`petState` 没回 `.rest`，sprite 永远卡在
   stale 状态直到 app 完全重启.

### 根因（Root cause）

把 `hasStartedMotionUpdates` 当**单向 flag** 用了——只考虑了 upgrade 半边状态机
（false→true）的 idempotent 保护，把 downgrade 半边（true→false）当作"不会发生"忽略了.

权限可以**双向变化**：

```
.notDetermined ─grant─→ .authorized ─revoke─→ .denied
                ↑                              │
                └──────────reset(rare)─────────┘
```

bind 是 SwiftUI 的"声明式同步入口"——RootView 在 ScenePhase active 调它的目的就是"用最新的
权限状态再重算一遍订阅"，而**不是**"我已经订过了就别再操心了". 用 flag 短路就把"声明式
re-evaluation"扭曲成了"一次性 setup"，丢了对 source-of-truth (authorizationStatus) 的重新读取.

第二层根因：generation token（MotionProviderImpl 用来防 stale callback 的 race 兜底，详见
docs/lessons/2026-05-04-motion-stop-restart-stale-callback-race.md）只防得住 **provider 层**
的 stale callback，防不住**ViewModel @Published** 已经被 set 过的 stale 值 —— UI 端的 stale
必须 ViewModel 自己主动 reset，不能寄希望于"反正没新事件来 UI 就是对的".

### 修复（Fix）

把 bind 改成"四象限 switch (currentlySubscribed, isAuthorized)"，每次都查
`authorizationStatus()`，按象限决策 action：

| currentlySubscribed | newAuthStatus     | action                                              |
|---------------------|-------------------|-----------------------------------------------------|
| false               | .authorized       | startUpdates + hasStartedMotionUpdates = true       |
| false               | not authorized    | noop（仅持引用）                                    |
| true                | .authorized       | noop（idempotent；防重复订阅 / 双倍事件）           |
| **true**            | **not authorized**| **stopUpdates + flag=false + petState = .rest** ← round 4 新增 |

before（round 3）：

```swift
guard !hasStartedMotionUpdates else { return }   // 单向 flag 短路
guard motionProvider.authorizationStatus() == .authorized else { return }
hasStartedMotionUpdates = true
motionProvider.startUpdates { ... }
```

after（round 4）：

```swift
let status = motionProvider.authorizationStatus()
let isAuthorized = (status == .authorized)
switch (hasStartedMotionUpdates, isAuthorized) {
case (false, true):  hasStartedMotionUpdates = true; motionProvider.startUpdates { ... }
case (false, false): return  // 持引用 noop
case (true, true):   return  // idempotent
case (true, false):  motionProvider.stopUpdates()
                     hasStartedMotionUpdates = false
                     petState = .rest
                     return
}
```

`hasStartedMotionUpdates` 字段保留——它的语义现在是"当前是否持有活跃订阅"
（与 motionProvider 引用的"是否注入"分离，仍是两态字段）.

新增单测 case：
- `testBind_authorizedSubscribed_thenRevoked_stopsUpdatesAndResetsState`: 完整覆盖 downgrade
  路径（authorize → start → walk → revoke → rebind → assert stopUpdates 被调 + petState=.rest +
  flag=false 让 re-grant rebind 还能升级）.
- `testBind_authorizedTwice_idempotent`: 防御 RootView 在 ScenePhase active 频繁 rebind 但权限
  没变化的常态路径——验证不重复 startUpdates / 不动 petState.
- `testBind_unauthorizedTwice_remainsNoOp`: first-launch 始终未授权时多次 rebind 仍纯 noop.

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写 **auth-gated 长连接订阅的 bind / setup** 时，**必须**让 rebind
> **每次都重新查 authorizationStatus**（不能用单向"已 setup"flag 短路），并且**显式定义** all
> 四象限（currentlySubscribed × isAuthorized）的 action——尤其 (subscribed=true, auth=false)
> 的 downgrade 路径必须 stopUpdates + reset @Published 到 baseline.
>
> **展开**：
> - 权限是**双向可变**的资源——permission 只会"被 grant"是错觉. user 可以随时去 Settings
>   撤销 / 系统级 restriction（Screen Time / MDM）可以中途生效. SwiftUI ScenePhase active
>   就是给你机会重读权限的 hook，不重读就漏 downgrade.
> - generation token 类的 race 兜底（在 provider 层防 stale callback）**只防得住事件流方向的 stale**，
>   防不住 @Published 已经被 set 过的 stale 值——后者必须 ViewModel 主动 reset.
> - 单向 flag（`hasStartedX: Bool`）和**两态 enum**（`subscriptionState: .idle / .active`）从静态
>   语义看等价，但单向 flag 让人**心理上**只想着"防重复"半边——优先用更显式的 switch (state, auth)
>   分支表达 4 象限，不要在 guard 链里堆短路.
> - 单测必须覆盖**双向**（grant 后 revoke、revoke 后 grant），不只测 grant 上行.
> - **反例（round 4 之前的代码）**：
>   ```swift
>   guard !hasStartedMotionUpdates else { return }   // 单向 flag 短路 = downgrade 路径不可达
>   guard authorizationStatus() == .authorized else { return }
>   hasStartedMotionUpdates = true
>   startUpdates { ... }
>   ```
>   此处 guard 链的"先 flag 后 auth"顺序就把 (true, false) 象限直接屏蔽了——auth 检查根本走不到.
> - **正例**：先把状态机画清（4 象限表），然后 switch on tuple，让 compiler 强制覆盖所有
>   case（Swift 上 `switch (Bool, Bool)` 不会强制 exhaustive，但 4 个 case 写出来后未来读
>   代码的人能立刻看到 downgrade 路径在那里）.
