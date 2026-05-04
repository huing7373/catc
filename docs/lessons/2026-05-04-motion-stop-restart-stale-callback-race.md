---
date: 2026-05-04
source_review: codex review (epic-loop round 1) — /tmp/epic-loop-review-8-2-r1.md
story: 8-2-coremotion-接入
commit: 4907398
lesson_count: 2
---

# Review Lessons — 2026-05-04 — MotionProvider stop/restart 的 stale callback 必须用 generation token 拦截 + UI test 不能把 (waiting) 占位当 PASS

## 背景

Story 8.2（CoreMotion MotionProvider 接入）round 1 codex review 给了 2 条意见，全部站得住：

1. **[P2]** `MotionProviderImpl.stopUpdates()` 之后立刻 `startUpdates()`，**`CMMotionActivityManager`** 已经 enqueue 到 `OperationQueue.main` 但还没 invoke 的 callback 会读到 fresh `currentHandler`，把"上一代"的 motion event 送给新订阅者。Story 8.4（HomeViewModel onAppear/onDisappear lifecycle rebind）会把这个 race 暴露出来。
2. **[P3]** `MotionProviderIntegrationTests` 在 30s 轮询里把"`(waiting)` 占位 + 没 error"也判 PASS，意味着 `startUpdates` 路径整段死掉时 UI 测试仍然绿。

两条都是 wiring layer 的设计漏洞而非"有意 trade-off"，全部 fix。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | stop/restart race — stale callback 串到新订阅 | P2 / medium | architecture / concurrency | fix | `iphone/PetApp/Core/Motion/MotionProviderImpl.swift` + `MotionProviderMock.swift` |
| 2 | UI 集成测试把 `(waiting)` 占位当 PASS | P3 / low | testing | fix | `iphone/PetAppUITests/MotionProviderIntegrationTests.swift` |

## Lesson 1: stop/restart race — stale callback 必须用 generation token 拦截

- **Severity**: medium (P2)
- **Category**: architecture / concurrency
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Motion/MotionProviderImpl.swift:67-90`

### 症状（Symptom）

`MotionProviderImpl.startUpdates(handler:)` 注册 closure 时只 capture `[weak self]`，closure 内部用 lock 读 `self.currentHandler` 并 forward。语义看起来 fresh，但实际上：

- 一次 `startUpdates(h1)` → `manager.startActivityUpdates(to: queue, withHandler: closure)`，closure 跑在 `OperationQueue.main`。
- 调 `stopUpdates()`：`currentHandler = nil`、`isUpdating = false`，然后 `manager.stopActivityUpdates()`。**但 `stopActivityUpdates` 不撤销已经 enqueue 但还没跑的 closure** —— 这是 `OperationQueue` 的契约，CoreMotion 的回调一旦投递到队列就只能等队列调度。
- 立即调 `startUpdates(h2)`：`currentHandler = h2`、`isUpdating = true`、再次注册一个**新的** closure。
- 此时上一代 closure（原本属于 h1 的订阅）排到队头开始 invoke → 它读 `self.currentHandler`，读到的是 **h2** → 把"上一代"motion event forward 给 h2。

Story 8.4 lifecycle rebind 时（HomeViewModel `.onAppear` start / `.onDisappear` stop / 再 `.onAppear` start）必然踩。

### 根因（Root cause）

System adapter 的 `currentHandler` 字段是"全局唯一槽位"，所有上一代 closure 都从同一个槽位读 fresh ref —— 这个模式假设"stop 之后系统不会再调 callback"，但 `OperationQueue` 的实际语义是"已 enqueue 的 work item 会被执行"。

更深的思维漏洞：**CoreMotion / HealthKit 这类 system adapter 的 stop/start 周期不是"原子翻转"** —— stop 只能告诉 OS "未来别再 enqueue"，已 enqueue 的事件 sink 必须由 adapter 自己识别并丢弃。Story 8.1 HealthProvider 没踩是因为它没有 long-running subscription，每次都是 one-shot query。

### 修复（Fix）

在 `MotionProviderImpl` 引入 `generation: UInt64` token：

```swift
// startUpdates:
generation &+= 1
let myGen = generation               // 闭包捕获本次 generation
currentHandler = handler
// ... 注册 manager callback：
manager.startActivityUpdates(to: queue) { [weak self] activity in
    guard let self, let activity else { return }
    self.lock.lock()
    guard self.generation == myGen, let captured = self.currentHandler else {
        self.lock.unlock()
        return                       // 上一代 callback：丢弃
    }
    self.lock.unlock()
    captured(activity)
}

// stopUpdates:
currentHandler = nil
generation &+= 1                      // 让任何已 enqueue 的"上一代"closure 在 check 时被拒
manager.stopActivityUpdates()
```

`MotionProviderMock` 也加同精神的 generation 字段 + 暴露 `captureGeneration()` / `injectActivity(_:expectedGeneration:)` helper，让 unit test 可以重现 race 时序而不依赖真 `CMMotionActivityManager`。新增 2 条单测：
- `testStopRestartRace_staleCallbackWithOldGenerationIsDiscarded`：模拟"start → capture gen → stop → start → 以旧 gen inject"，断言两代 handler 都收不到。
- `testInjectActivity_withoutGenerationParam_stillForwardsAfterStopRestart`：保证不带 generation 参数的 `injectActivity` 不被新机制误伤（case 5 stop-then-start 仍然 work）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：实装 system adapter（CoreMotion / HealthKit observer / Location updates）的 **stop+restart 周期** 时，**必须**给注册到系统 queue 的 callback closure 加 **generation token / sequence number**，stop 时自增 generation 让"已 enqueue 但未 invoke"的 stale callback 在 forward 前被拒。
>
> **展开**：
> - System adapter 的 `currentHandler: ((Event) -> Void)?` 字段是"全局唯一槽位"反模式：上一代和下一代订阅共享同一个槽，stop 只清 ref 不清队列已排好的 work item，restart 后 stale work item 读到 fresh ref 就漏过去了。
> - `OperationQueue` 的契约：`stopUpdates`（系统层）只阻止后续 enqueue，已 enqueue 的 work 仍会跑。CoreMotion / `CLLocationManager` / HealthKit observer query 全部如此。
> - 修法标准模板（lock 内**禁止** await，CoreMotion callback 是同步函数所以 lock 安全）：
>   ```swift
>   private var generation: UInt64 = 0
>   private var currentHandler: ((Event) -> Void)?
>
>   func startUpdates(handler: ...) {
>       lock.lock()
>       generation &+= 1
>       let myGen = generation
>       currentHandler = handler
>       lock.unlock()
>       manager.start { [weak self] event in
>           guard let self else { return }
>           self.lock.lock()
>           guard self.generation == myGen, let h = self.currentHandler else {
>               self.lock.unlock(); return
>           }
>           self.lock.unlock()
>           h(event)
>       }
>   }
>   func stopUpdates() {
>       lock.lock()
>       currentHandler = nil
>       generation &+= 1
>       lock.unlock()
>       manager.stop()
>   }
>   ```
> - **单测策略**：在 mock 上暴露 `captureGeneration()` + `injectActivity(_:expectedGeneration:)`，模拟"以旧 generation inject"时序，断言两代 handler 都收不到，**避免依赖真 system 的 queue 时序**（不可靠 + 不可重现）。
> - **反例**：`startUpdates` 注册 closure 时只 `[weak self]` + lock 内读 `currentHandler` —— 这是 8.2 dev 阶段的初版写法，看起来"fresh"但被 `OperationQueue` 已 enqueue 语义打穿。**任何 system adapter** 出现"stop 后立即 start"的合理使用场景（lifecycle rebind / 用户手动 toggle / view 重建）就是该模式必踩坑的契约边界。

---

## Lesson 2: UI 集成测试不能把 "(waiting) + no error" 视作 PASS（dead-wiring blind spot）

- **Severity**: low (P3)
- **Category**: testing
- **分诊**: fix
- **位置**: `iphone/PetAppUITests/MotionProviderIntegrationTests.swift:73-94`

### 症状（Symptom）

旧版 UITest 的 30s 轮询逻辑把 "三态 PASS" 全部视作绿：
1. result label 脱离 `(waiting)`（happy path）
2. error label 出现（permissionDenied / systemFailure 路径）
3. **result 仍是 `(waiting)` + error 也空**（被注释为"sandbox 没发 activity 的合法 fallback"）

第 3 态的问题：**它和"`startUpdates` 没被调"完全无法区分**。如果未来 dev 把 `MotionProviderProbeView .task` 里的 `motionProvider.startUpdates(handler:)` 误删（或 closure 注册失败），UITest 仍然绿；wiring 已死却没人发现。

### 根因（Root cause）

借用 Story 8.1 的"PASS-by-default"模板时混淆了两类 fallback：
- **8.1 HealthKit one-shot query**：UI 显示 0 是"实际拿到 0 步"的合法值（HealthKit 已读、user 今日确实 0 步），不是 dead wiring。
- **8.2 CoreMotion long-running subscription**：UI 停留 `(waiting)` 只可能是 "handler 没收到事件"，无法区分"系统真没发"和"`startUpdates` 没被调"。

把 8.1 模板照搬到 8.2 → 把 dead-wiring 当合法 fallback。

### 修复（Fix）

收紧到 "二态 PASS"：30s 内 result label 必须脱离 `(waiting)` **或** error label 必须出现，否则 `XCTAssertTrue(resolved, ...)` 失败并打印诊断方向（probe view `.task` 是否被调 / `startUpdates` handler 是否注册 / Xcode 26 simulator CoreMotion 是否需要 `simctl push activity` 触发）。

before/after：
```swift
// before:
if resolved { ... } else {
    XCTAssertEqual(lastResultLabel, "(waiting)", "...")    // 这是 PASS 路径
    print("INFO: ... 视作 PASS（路径已 wired up）.")
}

// after:
XCTAssertTrue(
    resolved,
    "30s 内 result label 既未脱离 (waiting) 占位，errorLabel 也未出现 → wiring 死链"
)
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：写 system adapter 的 **集成测试** 时，UI 显示的"初始占位值"**绝不能**视作 PASS —— 必须断言"UI 状态从初始占位发生过迁移"或"error label 出现"，二者择一为绿，否则 dead wiring 会被掩盖。
>
> **展开**：
> - 区分 "data fallback" vs "wiring fallback"：one-shot query 的 0 步 / 空集合是 data fallback（real data，PASS 合法）；subscription 的"占位字符串没变"是 wiring fallback（无法和 dead wiring 区分，**禁止** PASS）。
> - 8.1 HealthProviderIntegrationTests 模板适用于"读端一次性查询"，**不**能直接复制到"long-running subscription"。
> - assert message 必须给"诊断方向"列表（哪些环节可能死），让 review / debug 时有线索：
>   ```
>   - probe view .task 是否被调（启动期 launch arg 解析）
>   - subscription handler 是否注册到 manager
>   - simulator 是否需要 simctl push 触发事件
>   ```
> - **反例**：把"占位 + 无 error"加进 PASS 条件并注释成"sandbox 限制下的合法 fallback" —— 是 8.2 dev 阶段写下的反模式，借用 8.1 模板时没意识到 query/subscription 的本质差异。同类反模式：把 spinner 还在转、loading text 还在显示也视作 PASS。

---

## Meta: 本次 review 的宏观教训（可选）

两条 finding 同根：**8.2 dev 阶段把 8.1 的实装模板（HealthProvider）整体照搬到 8.2（MotionProvider）时，没有意识到 one-shot query 和 long-running subscription 的契约差异**。

- 共享 trait：lock + handler 槽位 + UI 占位 fallback 都是 8.1 留下的好模式。
- 关键差异：subscription 多了 "stop 之后系统残留 enqueue" 和 "handler 没被调 vs 系统没发事件无法区分" 两类 race，都是 query 模式不存在的。

未来在 epic 8 / 节点 3 后续 stories 接入 **CoreLocation / CMPedometer / NotificationCenter Observer** 等 long-running 订阅时，8.1 模板**必须**先做 subscription-vs-query gap 分析，再决定能否复用。
