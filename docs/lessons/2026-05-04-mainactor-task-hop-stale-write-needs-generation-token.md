---
date: 2026-05-04
source_review: codex review round 5 of Story 8.4 (file: /tmp/epic-loop-review-8-4-r5.md)
story: 8-4-主界面猫-sprite-三态动画切换
commit: c457f93
lesson_count: 1
---

# Review Lessons — 2026-05-04 — `Task { @MainActor in ... }` hop 引入 stale write race，generation token 是稳健解

## 背景

Story 8.4 round 4 加了 motion subscription 的 downgrade 路径：用户撤权限后 rebind 时
`stopUpdates() + petState = .rest` 显式让 UI 回到 baseline. round 5 codex P2 抓出后续 race：
motion handler closure 内的 `Task { @MainActor in self?.petState = mapped }` 异步 hop 让
"stop/downgrade 之后还在飞的 stale Task"能进 main actor 后覆盖已 reset 的 `.rest`，UI 出现
"撤权限后看到 sprite 闪一下回 stale state 再被 .rest 校正"的可见 race.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | motion handler 内 Task @MainActor hop 让 stale callback 在 stop/downgrade 后覆盖 reset 状态 | P2 | concurrency | fix | `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift` |

## Lesson 1: `Task { @MainActor in mutate }` 的异步 hop 在 stop/restart 链路里需要 generation token 拦截

- **Severity**: P2 (medium)
- **Category**: concurrency / architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift:392-398`（修复前）

### 症状（Symptom）

```swift
motionProvider.startUpdates { [weak self] activity in
    let mapped = MotionStateMapper.map(activity)
    Task { @MainActor in
        self?.petState = mapped     // ← stale Task 也能跑到这里覆盖
    }
}
```

时序：

1. callback 入口（在 OperationQueue.main 跑，已经在 main thread）.
2. 创建 `Task { @MainActor ... }`（异步排队等下一个 main actor turn）.
3. 与此同时（或紧接着）bind 处理 downgrade：`stopUpdates() + petState = .rest`.
4. 排队中的 Task 此时执行——`self.petState = mapped`（stale `.walk`/`.run`），把已 reset 的
   `.rest` 覆盖回去.

UI 表现：用户撤权限后看到 sprite 闪一下回 stale state，再被 next downgrade tick 校正回 `.rest`.

### 根因（Root cause）

**Swift Concurrency 的 `Task { @MainActor in ... }` 是异步排队，不是同步派发**——即使 callback
已经在 main thread（OperationQueue.main 保证），`Task { @MainActor in ... }` 仍然会创建一个
new task 进 main actor 队列等待下一个执行 turn. 这个 ms 级的"在飞"窗口足以让 bind 的 downgrade
路径先跑完，stale Task 后跑覆盖.

`OperationQueue.main` 同步执行 → callback 闭包同步进入 → 但闭包内 `Task { @MainActor ... }`
**不**同步执行——它是 actor isolation 的入口，必须排队. 这是个 mental model trap：以为
"已经在 main thread 了，Task @MainActor 是 noop hop" → 错. Task 创建有 cost + 排队顺序无保证.

generation token 是经典 race 兜底——把"启动期捕获 token"和"消费时 check token"作为不变量，让
任何在飞 callback / Task 在状态机推进后被自然丢弃. 这个模式在 8.2 MotionProviderImpl 已经
落地（防 stop/restart 后系统 enqueue 的 stale 事件 leak 到新 generation；详见
`docs/lessons/2026-05-04-motion-stop-restart-stale-callback-race.md`）；本 lesson 是把同模式
**升级一层**到 ViewModel 层——provider 的 generation token 防得住"事件流方向的 stale callback"，
但防不住"已经 enqueue 但还没 mutate @Published 的 in-flight Task". 后者必须 ViewModel 端独立
做 generation token.

### 修复（Fix）

`HomeViewModel` 加 `motionBindGeneration: UInt64` 字段；bind 升级 / downgrade 时 `&+= 1`；
handler closure 启动期 capture 当前值；Task 内 mutate petState 前 check token：

```swift
// before（round 4）
motionProvider.startUpdates { [weak self] activity in
    let mapped = MotionStateMapper.map(activity)
    Task { @MainActor in
        self?.petState = mapped       // ← stale Task 无差别覆盖
    }
}

// after（round 5）
motionBindGeneration &+= 1            // bind upgrade 时推进 token
let token = motionBindGeneration      // closure capture
motionProvider.startUpdates { [weak self] activity in
    let mapped = MotionStateMapper.map(activity)
    Task { @MainActor in
        guard let self else { return }
        guard self.motionBindGeneration == token else { return }  // ← stale Task drop
        self.petState = mapped
    }
}

// downgrade 路径
case (true, false):
    motionProvider.stopUpdates()
    hasStartedMotionUpdates = false
    motionBindGeneration &+= 1        // ← 让在飞 stale Task 进 main 后被 token check 拦截
    petState = .rest
```

新增单测 case：

- `testStaleMotionCallback_afterDowngrade_doesNotOverwriteRestState`（核心 race regression）:
  authorize → start → walk → revoke → rebind（downgrade）→ 模拟 stale callback 仍能 invoke →
  断言 petState 仍 .rest（不被 stale `.walk` 覆盖）.
- `testStaleMotionCallback_afterRegrant_doesNotOverwriteNewState`:
  downgrade 后再 grant → 第三次 bind 走新 startUpdates → 新 callback 用新 token 正常 mutate;
  老 in-flight stale callback 仍被 token mismatch 拦截.

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写 **subscriber 模式 + 可 stop/restart 的链路 + handler 内做
> 异步 hop（Task @MainActor / DispatchQueue.async / Combine receive(on:)）写 @Published**
> 时，**必须在异步 hop 之前 capture generation token**，hop 之后 mutate state 之前 check
> token；不能依赖"反正没新事件来 UI 就是对的"——已经入队的 Task / DispatchWorkItem 不会自动
> 取消.
>
> **展开**：
> - `Task { @MainActor in ... }` 即使 callback 已在 main thread 也是**异步排队**——不是
>   "noop hop". 启动 Task 到 mutate state 之间有可观察的窗口供别的代码先跑.
> - generation token 是 race 兜底的通用模式. 用 `UInt64 &+= 1`（saturating add，58 亿年才溢出）
>   和"启动期 capture + 消费期 check"两个不变量即可——简单可证.
> - **三层 generation token 防御**（subscriber 模式典型架构）：
>   - **Adapter / Provider 层**（如 MotionProviderImpl）：防 system framework enqueue 的 stale
>     callback. 详见 `docs/lessons/2026-05-04-motion-stop-restart-stale-callback-race.md`.
>   - **ViewModel @MainActor hop 层**（本 lesson）：防"已 enqueue 的 Task @MainActor"在 state
>     machine 推进后 mutate 失效状态.
>   - **UI 层**（SwiftUI .id() / .task(id:) 模式）：防 view body 重算时引用 stale ViewModel
>     state. 详见 `docs/lessons/2026-05-04-swiftui-content-swap-needs-id-and-transition.md`.
> - `MainActor.assumeIsolated { ... }` 是另一个稳健解（Swift 5.10+），把 Task hop 改成同步
>   mutate. 但 trade-off：依赖"callback 已在 main queue"的 caller 契约，破坏契约就 trap. 在
>   本 case 下 OperationQueue.main 保证了，但用 generation token 兼容 callback 走任意调度
>   方式（更通用）. 推荐生产路径用 generation token，调试 / 单层 callback chain 简单场景可用
>   assumeIsolated.
> - 单测必须覆盖**race 时序**——不能只测 happy path（authorize → walk → assert .walk）. 需
>   显式构造"stop/downgrade 后 stale callback 仍能 invoke"的时序，并断言 mutate 被拦截.
>   `MotionProviderMock` 已提供 `injectActivity(_:expectedGeneration:)` 模拟 stale 事件.
> - **反例**（round 5 修复前的代码）：
>   ```swift
>   Task { @MainActor in
>       self?.petState = mapped     // ← 没 generation check，stale Task 覆盖 reset
>   }
>   ```
>   单看代码"我都加了 [weak self] 防循环 + Task @MainActor 跨 actor 派发"觉得没问题，但
>   只防了"循环引用"和"actor isolation"两个 concern，没防"in-flight stale Task"第三个 concern.
> - **正例**：
>   ```swift
>   let token = self.generation                  // capture
>   Task { @MainActor in
>       guard let self else { return }
>       guard self.generation == token else { return }   // check
>       self.someState = newValue
>   }
>   // 同时 stop/restart 入口要 self.generation &+= 1 推进 token
>   ```
> - **更小的反例**（同模式不同 surface，未来 Claude 可能踩到的变形）：
>   - `Combine` `receive(on: .main).sink { ... }`——sink closure 排队执行，subscription 取消
>     后已 enqueue 的 sink 仍可能跑（依赖 publisher 实现）；防御方式：
>     `.filter { [weak self] _ in self?.generation == capturedToken }` 或在 sink 内 check.
>   - `DispatchQueue.main.async { ... }` 写 @Published——同坑，必须 generation check.
>   - SwiftUI `.task(id:)` 取 priority 切换时旧 task cancel 但已经 set 的 @State / @Published
>     不会回滚——如果 mutate 在 cancel 之前已经发生，下游 UI 仍看到 stale 值；解法是 mutate
>     前 check `Task.isCancelled` 或加 generation token.

---

## Meta: 与 Story 8.2 / 8.4 round 4 的关系

本 lesson 是 8.2 MotionProviderImpl generation token race lesson 的"上一层"——8.2 防 provider
层 stale callback，本 lesson 防 ViewModel @MainActor hop 层 stale write. round 4 lesson
（auth-gated subscription downgrade）解决"reset state 是否被显式调"的问题，本 lesson 解决
"reset 后 stale Task 是否覆盖回 stale 值"的 follow-on race. 三个 lesson 链式构成 motion
subscription race-safety 的完整防御层.
