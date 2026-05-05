---
date: 2026-05-04
source_review: codex review round 4 of Story 8-5-步数同步触发器（/tmp/epic-loop-review-8-5-r4.md）
story: 8-5-步数同步触发器
commit: 1042af2
lesson_count: 2
related_lessons:
  - 2026-05-04-manual-trigger-must-await-in-flight.md
  - 2026-05-04-step-sync-fresh-install-requestpermission-gap.md
---

# Review Lessons — 2026-05-04 — `await` 期间 race 让 single-flight gate 失效，必须 while-loop 链式等待 + @MainActor 同步段原子 assign（fresh install requestPermission gap 第 3 次复发 reaffirm wontfix）

## 背景

Story 8.5（步数同步触发器）codex review round 4 抓到 2 条 finding：

- **[P1]** `SyncStepsUseCase.swift:56-57` fresh install 没人调 `healthProvider.requestPermission()` →
  step sync silent fail forever. **第 3 次复发**：round 1 resumed wontfix → round 2 wontfix（同 spec gap） →
  round 4 又抓.
- **[P2]** `StepSyncTriggerService.swift:119-129` triggerManual 单次 `await currentSyncTask?.value` resume
  后无条件覆盖 currentSyncTask → automatic trigger 在 await 期间填了新 task 的话，manual 覆写会让
  automatic task 失去引用但仍在跑 → **双 sync 并发**，违反"同步不重叠"契约.

P1 reaffirm wontfix（不再写新 lesson，引用前次 precedent）；P2 是 round 3 fix 留的 race 漏洞，本轮 fix.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | fresh install HealthKit requestPermission 没 production caller（**第 3 次复发**） | P1 | architecture | wontfix（reaffirm；引用 r1/r2 precedent，不写新 lesson） | `iphone/PetApp/Features/Home/UseCases/SyncStepsUseCase.swift:56-57` |
| 2 | triggerManual `await currentSyncTask?.value` 单次 + 无条件覆盖 → 双 sync 并发 race | P2 | concurrency | fix | `iphone/PetApp/Features/Home/Services/StepSyncTriggerService.swift:119-129` |

## Lesson 1: fresh install requestPermission gap（第 3 次复发，reaffirm wontfix）

- **Severity**: P1
- **Category**: architecture / process
- **分诊**: wontfix（reaffirm；不写独立 lesson）
- **位置**: 同前次

### 症状（Symptom）

codex round 4 再次 surface 了 round 1 / round 2 已 wontfix 的同一个 spec gap. 描述完全一致：fresh
install 路径上没人调 `healthProvider.requestPermission()`，sync silent fail forever.

### 根因（Root cause）

不变 —— 与 [`2026-05-04-step-sync-fresh-install-requestpermission-gap.md`](./2026-05-04-step-sync-fresh-install-requestpermission-gap.md)
完全同根因：epic 8 切片漏专门 caller story；codex 看代码视角下每次 review 都会再次 surface.

### 修复（Fix）

不修代码（与前两轮一致）. **不**写新 lesson —— 写新 lesson 会污染蒸馏（同 gap 三份 lesson）.
引用前次 lesson + 在本轮 commit message 标注"第 3 次复发，与 r1 resumed / r2 同 wontfix 决策".

### 预防规则（Rule for future Claude）⚡

> **一句话**：同一 spec gap 在 review-loop 中**第 N 次复发**（N≥2）时，处置路径是
> **wontfix + 引用前次 lesson + commit message 显式标注复发计数**，**不**写新 lesson.
>
> **展开**：
> - 第 1 次：wontfix + 写首次 lesson（解释 gap 来源 / 根因 / 元教训）.
> - 第 2 次：wontfix + 写二次复发元 lesson（强化 sprint-planning checklist 应当 cross-grep
>   setup-side-effect API caller 的元规则；本轮 r1 resumed 已落地此 lesson）.
> - **第 3 次及之后**：wontfix + commit message 标"第 N 次复发"+ 引用前次 lesson；**不**再写新 lesson
>   —— 同 gap 第 3 个 lesson 文件只会污染蒸馏，没有新的 distillable insight.
> - 若复发次数 ≥ 4 → 升级为 epic-level blocker，强制开 caller story.
>
> **反例**：codex 第 3 次 surface 同 gap → Claude 又写一份独立 lesson 解释根因 → 蒸馏时同根因被
> 三份文档放大 → 未来 Claude 读到时以为是三个独立问题. 应当是单一 lesson 链路：r1 lesson 是 anchor，
> r2 / r3 lesson 只引用、不复述.

## Lesson 2: `await` 期间 main actor 让出 → single-flight gate 失效（必须 while-loop 链式等待 + @MainActor 同步段原子 assign）

- **Severity**: P2
- **Category**: concurrency
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/Services/StepSyncTriggerService.swift:116-145`

### 症状（Symptom）

`StepSyncTriggerService.triggerManual()` round 3 fix 写法：

```swift
// 旧实装（round 3，有 race）
public func triggerManual() async {
    await currentSyncTask?.value  // 单次 await，无 re-check
    let task: Task<Void, Never> = Task { @MainActor [weak self] in ... }
    currentSyncTask = task         // 无条件覆盖
    await task.value
}
```

race 路径：

1. launch 路径起 in-flight A；`currentSyncTask = A`.
2. caller 调 `triggerManual()` → 执行 `await currentSyncTask?.value`（即 `await A.value`）→ main actor 让出.
3. A 完成 → A 的 defer 清 `currentSyncTask = nil`.
4. main actor 让出期间，timer / foreground 路径跑 `spawnSyncIfIdle` → 看到 `currentSyncTask == nil` →
   spawn B 并 `currentSyncTask = B`.
5. manual resume → 不 re-check，直接创建 myTask 并 `currentSyncTask = myTask`（**覆盖 B**）→ B 失去
   currentSyncTask 引用但仍在跑 → manual 自己的 task 也在跑 → **双 sync 并发**.
6. 两个 sync 都会发 `/steps/sync` 请求，违反 "同步不重叠" 契约（epics.md AC 行 1577）.

### 根因（Root cause）

**Swift concurrency 关键认知**：`await` 处 main actor 让出，期间任何 @MainActor 同步代码可以跑.
`await` resume 时 main actor 重新拿到，但**期间发生的副作用（包括其他 actor-isolated 字段被改写）**
对当前函数透明 —— 编译器不会提示 `currentSyncTask` 已被改.

**single-flight gate 设计假设**：`gate == nil` ⇔ "可启动新 sync". 这个不变量靠 `if gate == nil { spawn }`
@MainActor 同步段维持. 但 `await x?.value` 是 await，不是同步段 —— resume 后 `gate` 状态不可假设.

**正确范式**：
1. **while-loop 链式等待**：每次 await 完后 re-check gate；只要还非 nil 继续等. 直到看到 nil 才进下一步.
2. **@MainActor 同步段原子 assign**：从 "看到 nil" 到 "创建 task 并赋值给 gate" 的过程**不能有 await**.
   这样保证 main actor 不让出，其他路径无法插入.

### 修复（Fix）

```swift
// 新实装（round 4，race-safe）
public func triggerManual() async {
    // 链式等待：每次 await 完后 re-check.
    while let inflight = currentSyncTask {
        await inflight.value
        // resume 时 currentSyncTask 可能被 automatic 路径覆盖为新 task；继续 loop 等.
    }
    // 此时 currentSyncTask == nil，且我们处于 @MainActor 同步段.
    // 下面创建并 assign 之间无 await → 不会被打断.
    let task: Task<Void, Never> = Task { @MainActor [weak self] in
        guard let self else { return }
        await self.runSync(reason: .manual)
    }
    currentSyncTask = task
    await task.value
}
```

**关键不变量**：
- `while let inflight` 退出条件是 `currentSyncTask == nil`，不是 "前一个 inflight 完成"
  —— 前一个完成不代表没有别的 automatic trigger 又起新 task.
- "re-check + create + assign" 三步是 @MainActor 同步段 → 不会被 race 干扰.
- automatic 路径的 `if currentSyncTask != nil { return }` 短路保证：一旦 manual assign 成功，
  automatic 不会覆盖；一旦 manual 看到 nil，automatic 也看到 nil 时是 race，但 manual 在同一个
  同步段里完成 assign 后 yield，automatic 才会跑并看到 manual task 占位 → 短路.

**单测**：新增 `testTriggerManual_chainWaitsThroughMultipleAutomaticInflights`，用 mock 的
`maxConcurrentInflight` 计数器断言任意时刻最多 1 个 sync in-flight（旧实装 race 命中时 = 2，新实装 = 1）.

### 预防规则（Rule for future Claude）⚡

> **一句话**：在 actor 隔离的 single-flight gate（如 `currentTask: Task?` / `inflight: Bool`）上
> 写 `async` 等待逻辑时，**禁止**单次 `await x?.value` + 无条件覆盖 gate；**必须**用
> `while let x = gate { await x.value }` 链式等待 + @MainActor 同步段原子 check-and-assign.
>
> **展开**：
> - **触发条件**：函数是 `async` + 内部需要 await 某个共享 actor-isolated 字段（gate）+ 之后还要
>   re-assign 该字段. 这个三联模式在 Swift concurrency 里是 race 高发区.
> - **race 机制**：`await` 处 main actor 让出，期间其他 @MainActor 代码可改写 gate；resume 后**不能假设**
>   gate 仍是 await 之前的状态. 编译器不会警告这件事.
> - **正确范式 = while-loop + 同步段**：
>   ```swift
>   while let x = gate {           // re-check loop
>       await x.value              // 每次让出后 loop body 重新检查
>   }
>   // gate == nil；下面是 @MainActor 同步段（无 await）
>   let myTask = Task { ... }
>   gate = myTask                  // 原子 assign
>   await myTask.value             // 才允许这里 await
>   ```
> - **反例 1**：单次 `await gate?.value` + 无条件 `gate = myTask` → race（本轮抓到的实装）.
> - **反例 2**：`if gate == nil { gate = myTask } else { await gate!.value; gate = myTask }` →
>   else 分支 await 后无 re-check，同样 race.
> - **反例 3**：用 lock 包 await → Swift 编译器不允许，且即使允许也会死锁（actor reentrancy）.
>   actor / @MainActor 隔离 + while-loop 是唯一稳的范式.
> - **测试断言**：用 mock 在 `execute` 入口 +1、defer -1 计 `maxConcurrentInflight`，断言 == 1.
>   仅断言 invocations.count 不够（race 命中时 count 也可能正确，但中间瞬态有并发）.
> - **范围适用**：StepSyncTriggerService / 任何 ChestOpenUseCase 这类需要 await + assign 共享 gate
>   的 service / use case；future Claude 写类似 manual-trigger / fetch-and-cache / debounce-flush
>   函数时直接套范式.

---

## Meta: 本次 review 的宏观教训

round 3 fix 时已经识别"manual 必须 await in-flight"，但用了**单次 await + 无条件覆盖**的最朴素写法，
没考虑 await 期间 gate 可能被 race 改写. 这是 Swift concurrency 在 main actor 上常见的"让出陷阱"——
编译器静态检查只保证类型安全 / Sendable 安全，不保证逻辑不变量穿过 await.

**lesson distill 信号**：本 lesson 写完后，未来 Claude 看到 `async func ...` 内部出现
`await someActorIsolatedTask?.value` + 之后改写 actor isolated 字段，应当**自动触发** "race check"，
按 while-loop 范式重写 + 加 maxConcurrentInflight 断言单测.

复发管理：**fresh install requestPermission gap 已 3 次复发**. 当前策略仍是 wontfix（spec gap，
不属代码 bug），但若 4 次复发，必须升级为 epic-level blocker 强制开 caller story.
