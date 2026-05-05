---
date: 2026-05-04
source_review: codex review round 3 — /tmp/epic-loop-review-8-5-r3.md
story: 8-5-步数同步触发器
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-04 — manual trigger 必须 await in-flight，不能用 gate 短路 return；公开 async API 的等待语义要明示

## 背景

Story 8.5（步数同步触发器）round 3 codex review 命中 `StepSyncTriggerService.triggerManual()` 的等待语义 bug：当 launch / foreground / timer 触发的 fire-and-forget sync 正在 in-flight 时，新调进来的 `triggerManual()` 因复用同一 `performSync` 路径，会被 `guard !isSyncing` 短路 `return`——caller（Story 21.x ChestOpenUseCase 节点 7）于是在没等任何 sync 完成的情况下立即拿到控制权，用 stale `currentStepAccount` 跑接下来的开箱流程，破坏 epics 钦定的"同步完成后再继续开箱"契约。

修复方案 A（review 推荐）：把 `isSyncing` Bool 改为 `currentSyncTask: Task<Void, Never>?` 引用追踪 in-flight sync；`triggerManual()` 先 `await currentSyncTask?.value` 等 in-flight 完，再启动自己的新 sync Task 并 `await` 完成；保证 manual 返回时一定刚跑过一次 fresh sync。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | triggerManual 不等 in-flight sync 即返回，破坏"同步完成后再继续"契约 | P2 (medium) | architecture | fix（方案 A） | `iphone/PetApp/Features/Home/Services/StepSyncTriggerService.swift:101-129` |

## Lesson 1: 公开 `async` API 的等待语义必须由 API 自身保证，不能依赖内部 gate 的副作用

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/Services/StepSyncTriggerService.swift:101-129`

### 症状（Symptom）

`triggerManual()` 是公开的 `async` 入口，文档 + AC 钦定语义"等同步完成后返回"——但实装：

```swift
public func triggerManual() async {
    await performSync(reason: .manual)  // 复用 fire-and-forget 的同一路径
}

private func performSync(reason: SyncReason) async {
    guard !isSyncing else { return }  // ← 这里短路 return，但 isSyncing 是别的触发起的！
    isSyncing = true
    defer { isSyncing = false }
    try? await syncStepsUseCase.execute(...)
}
```

当 launch / foreground / timer 路径已把 `isSyncing` 设成 `true` 时，manual 的 `performSync` 在 `guard` 处直接 `return`，**没等任何 sync 完成**就让 caller 继续。caller（chest open）于是用 stale 步数账户跑接下来的流程。

### 根因（Root cause）

把"重叠忽略"（fire-and-forget 路径间不要叠加请求）和"manual 等待完成"（公开 API 契约）当成同一件事处理——共用 `performSync` 的 in-flight gate。两个语义其实正交：

- **重叠忽略**针对 fire-and-forget 路径——caller 不关心是否落地，gate 短路 return 是 OK 的；
- **manual 等待**针对 await 入口——caller 用返回时机当 happens-before barrier，gate 短路 return 是**致命的**（让 caller 拿到 stale state）。

只用一个 Bool flag + `guard !flag { return }` 代码模式无法表达"我要等到 in-flight 完，然后再跑一次"——因为 flag 只能告诉你"现在有人在跑"，不能给你 await handle。

### 修复（Fix）

用 `Task<Void, Never>?` 引用追踪 in-flight sync（替代 Bool flag）：

```swift
private var currentSyncTask: Task<Void, Never>?

// fire-and-forget 路径：launch / foreground / timer 共用
private func spawnSyncIfIdle(reason: SyncReason) {
    guard currentSyncTask == nil else { return }  // 重叠忽略仍然有效
    let task: Task<Void, Never> = Task { @MainActor [weak self] in
        guard let self else { return }
        await self.runSync(reason: reason)
    }
    currentSyncTask = task
}

// runSync 自清 currentSyncTask（让下一次触发 / 下一次 await 拿到 nil）
private func runSync(reason: SyncReason) async {
    defer { currentSyncTask = nil }
    let motionState = homeViewModel?.petState ?? .rest
    do { try await syncStepsUseCase.execute(motionState: motionState) }
    catch { /* swallow */ }
}

// manual 等待入口：先 await in-flight 完，再自己跑一次新 sync 并 await
public func triggerManual() async {
    await currentSyncTask?.value          // 1. 等 fire-and-forget in-flight 完
    let task: Task<Void, Never> = Task { @MainActor [weak self] in
        guard let self else { return }
        await self.runSync(reason: .manual)
    }
    currentSyncTask = task                // 2. 自己启动新 sync 占位
    await task.value                       // 3. 等自己的 sync 完——返回时 caller 拿到 fresh state
}
```

关键点：
- fire-and-forget 路径仍走 `spawnSyncIfIdle` → in-flight 时被忽略（保留 epics AC 钦定的"同步不重叠"语义）；
- manual 路径单独走"先 await in-flight、再启动自己的、再 await 完"三步——不复用 gate，自己保证等待语义；
- `Task` closure 必须显式 `guard let self else { return }`（不能写 `await self?.runSync(...)`）——否则 closure 返回 `Void?` 而非 `Void`，类型推断成 `Task<Void?, Never>` 与 `currentSyncTask: Task<Void, Never>?` 类型不匹配；
- timer Task 整体打 `@MainActor`，让循环体调 `spawnSyncIfIdle`（@MainActor 同步方法）不需要 `await self?.method()` 的奇怪 cross-actor sync hop（编译器对此模式报"no async operations within await" warning）。

测试更新：
- `testFireAndForgetTrigger_whileInFlight_isIgnored`：明确"fire-and-forget 路径"的重叠忽略仍生效（用两次 `triggerForeground` 验证）；
- 新增 `testTriggerManual_whileInFlight_awaitsThenRunsOwnSync`：先 `triggerForeground` 启动 in-flight，再 `await triggerManual`，断言总 sync = 2（manual 等了 in-flight + 自己又跑了一次）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：当一个 service 同时暴露**公开 `async` API（caller await 等结果）**和**fire-and-forget 触发路径（caller 不关心结果）**时，**禁止**让两条路径共用同一个 `guard !inFlightFlag { return }` 短路 gate——必须为 await API 单独写"等 in-flight + 跑自己 + await 完"三步路径。
>
> **展开**：
> - 用 `Task<Void, Never>?` 引用而不是 `Bool` flag 跟踪 in-flight 状态——前者能给后续 `await` 提供 handle，后者不能。
> - 公开 `async` API 的契约 = "返回时一定有过完整一次 sync 落地"——只要复用同一个 gate 短路 return 就**必然**违反这个契约（无论 caller 多小心都救不回来）。
> - **重叠忽略**和**manual 等待**是正交的两个 concern；不要为了"代码精简"硬合并到同一个 gate。
> - 设计 review checklist：每个公开 `async` 方法被调时，问一句"如果别的 task 已经在跑同样工作，我这次 await 等什么？"——答不上就说明语义模糊。
> - **反例**：
>     ```swift
>     // ❌ 错误模式：manual 复用 gate 路径
>     public func triggerManual() async { await performSync(reason: .manual) }
>     private func performSync(...) async {
>         guard !isSyncing else { return }  // manual 在 in-flight 时直接被短路！
>         ...
>     }
>     ```
>     ```swift
>     // ✅ 正确模式：manual 单独路径
>     public func triggerManual() async {
>         await currentSyncTask?.value          // 先等 in-flight
>         let task = Task { ... }
>         currentSyncTask = task
>         await task.value                       // 再等自己的
>     }
>     ```
> - **类似坑**：任何"caller 期待 happens-before"的公开 await API（refresh / load / sync / save / drain）都适用本规则；和"重叠忽略"挤一个 flag 永远是错的。

---

## Meta：跨触发路径的等待语义不能省略文档化

本次 review 的根本暴露：原 `triggerManual()` 注释只写了"等待同步完成（与 launch / foreground / timer 不等待不同——caller 需要在同步完成后再继续开箱）"——这只表达了 caller 视角的契约，**没**说"内部如何做到这个等待"。实装内部直接调 `await performSync(.manual)` 看似让 caller `await` 了一次，实际被内部 gate 短路掉。

未来对此类公开 `async` API 写文档时：

- **必须**注明"in-flight 时的语义"：
  - 是"等 in-flight 完再返回"（caller 一律拿到刚跑完的结果）？
  - 还是"in-flight 时直接 return"（caller 拿到的是 best-effort，可能不完整）？
- **必须**对应到实装代码——文档说"等 in-flight 完"就**禁止**用 gate 短路 return。
- 单测必须覆盖"in-flight 时 caller 调本 API"的场景（不能只测 idle 路径），否则这种语义错位永远漏到 review 才发现。
