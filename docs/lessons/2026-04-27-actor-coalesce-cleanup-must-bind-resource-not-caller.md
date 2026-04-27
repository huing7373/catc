---
date: 2026-04-27
source_review: codex review (epic-loop round 1) — /tmp/epic-loop-review-5-4-r1.md
story: 5-4-无效-token-静默重新登录
commit: 31c4fe7
lesson_count: 1
---

# Review Lessons — 2026-04-27 — actor coalesce 协调器的 inFlight 清理必须绑定资源 task 生命周期，而不是 caller defer

## 背景

Story 5.4 落地了 `SilentReloginCoordinator`（actor），用 `inFlight: Task<String, Error>?` 字段做"同一时刻只跑一次重登 + 多并发等待复用结果"的 single-flight coalesce。codex review round 1 指出：把 `defer { inFlight = nil }` 写在 caller 的 `relogin()` 里，等于把"释放共享资源占用"绑定在 caller 函数何时返回 —— 一旦 caller 在 spawned task 还没完成时就退出 relogin（例如 cancellation 传播到 caller-await），inFlight 会被提前清空，但 spawned task 仍在跑 → 后续并发 relogin 看到 inFlight = nil 就会启第二个 task，duplicate guest-login。

经过实际验证 + 写小型 Swift sandbox 复现，**当前 Swift 版本下** `Task.value` await 不是 cancellation 自动 throw 的 suspension point —— 即使 caller Task 被 cancel，`await task.value` 仍会等 spawned task 完成才返回。codex 描述的"`await task.value` 抛 CancellationError"在标准 unstructured Task 路径下并不直接成立。但：

1. structured concurrency（async-let / TaskGroup）的 cancellation 传播 + 未来 Swift 演进可能让这个 await 变成 cancellation-aware
2. 即使 codex 的具体触发条件叙述不精确，**清理时机绑定 caller 而非资源本身**这件事本身就是错误的资源管理模式 —— 它依赖"caller 行为符合预期"这个隐式约定

所以 fix 落地：把清理 hop 进 spawned Task 的 body —— 严格绑定 task 自己的生命周期。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | inFlight 清理时机绑定 caller defer，与 spawned task 生命周期解耦 | P2 (medium) | architecture / concurrency | fix | `iphone/PetApp/Features/Auth/UseCases/SilentReloginCoordinator.swift:42-49` |

## Lesson 1: actor coalesce 协调器的 inFlight 清理必须绑定 spawned task 生命周期

- **Severity**: medium (P2)
- **Category**: architecture / concurrency
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Auth/UseCases/SilentReloginCoordinator.swift:35-74`

### 症状（Symptom）

旧实现：
```swift
public func relogin() async throws -> String {
    if let existing = inFlight {
        return try await existing.value
    }
    let task = Task { try await useCase.execute() }
    inFlight = task
    defer { inFlight = nil }            // ← 清理绑定 caller relogin 的 defer
    return try await task.value
}
```

按字面语义：spawned task 完成 → `task.value` 返回 → relogin 函数即将返回 → defer 执行 → `inFlight = nil`。在"caller 一路同步等到底"的 happy path 下两者时机重合，没问题。

但在 cancellation-heavy flows 下，理论上 `task.value` 可能因 caller cancellation 而提前 throw（具体在当前 Swift 版本下不会，但 future-proof 视角必须考虑），或 caller 在 await 之前/之后被同步退出 → defer 跑了，spawned task 还在跑 → 窗口期内的并发 relogin 看到 `inFlight = nil`，启第二个 spawned task → useCase.execute 被调多次 → duplicate `/auth/guest-login` + race token writes。

### 根因（Root cause）

**资源管理模式错误：清理动作绑定的是"消费者（caller）的退出时机"，而不是"资源（spawned task）自身的生命周期"。**

把 `inFlight` 看成是"指向 spawned task 的引用 / 占位"，它的存在意义就是声明"这个 task 还活着，别启第二个"。那么清空它的语义应该是"task 真的死了"，而不是"caller 不再关心了"。caller 不关心 ≠ task 不存在。

`defer` 是 caller 函数语法块的工具，绑定的是**函数何时离开**。当资源生命周期 ≠ 函数调用栈生命周期时（spawn / async / 引用语义），用 `defer` 清理共享资源就会出现错位。

具体的思维漏洞：**"actor 字段是共享状态，它的清理时机必须由共享状态的真正终结事件触发"**。actor 字段的写入和读出是 atomic，但**写入时机本身**必须建模正确 —— actor 不会替你判断"什么时候该清"，它只保证"清的动作不和读撞 race"。

### 修复（Fix）

把清理动作放进 spawned Task body 内部，task 完成（成功 / 失败）后 hop 回 actor 调 `clearInFlight()`：

```swift
public func relogin() async throws -> String {
    if let existing = inFlight {
        return try await existing.value
    }
    let useCase = self.useCase
    let task = Task { () async throws -> String in
        do {
            let token = try await useCase.execute()
            await self.clearInFlight()   // ← 清理绑定 spawned task 生命周期
            return token
        } catch {
            await self.clearInFlight()
            throw error
        }
    }
    inFlight = task
    return try await task.value
}

private func clearInFlight() {
    inFlight = nil
}
```

关键点：
- `clearInFlight()` 是 actor isolated method，hop 一次保证写入与并发读不撞 race
- spawned task body 用显式 `do/catch` 而非 `defer` —— Task 体内 defer 在 throw 路径上和 await 交织复杂，显式 catch 更直观
- caller 的 `try await task.value` 怎么走（成功 / throw / cancel-aware-future-throw）都不再影响 inFlight 清理时机

测试：新增 `testInFlightSurvivesUntilSpawnedTaskCompletes` —— 启 taskA 调 relogin（不立即 await），actor 装上 inFlight 后立即启 taskB 调 relogin，验证 useCase.execute 仅被调 1 次（B coalesce 到 A 的 spawned task），并验证两个都完成后第三次 relogin 能再启新 task（spawned task body 里的 clearInFlight 已 hop 回 actor 清空字段）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **actor / class 里实现 single-flight coalesce 模式**（用一个 `inFlight: Task?` 字段保证"同一时刻只跑一次 + 多 caller 等同一结果"）时，**必须**把"清空 inFlight 字段"的动作放进 **spawned Task body 内部**（task 完成后 hop 回 actor 调 isolated method 清），**禁止**写成 caller 函数体的 `defer { inFlight = nil }`。
>
> **展开**：
> - 把 inFlight 字段当作"指向资源的引用"理解 —— 它的清空时机由资源（spawned task）的真正终结事件触发，不由 caller 是否还在等待触发
> - 实现模板：
>   ```swift
>   actor Coordinator {
>       private var inFlight: Task<T, Error>?
>       func run() async throws -> T {
>           if let existing = inFlight { return try await existing.value }
>           let task = Task { [self] () async throws -> T in
>               do {
>                   let result = try await actualWork()
>                   await self.clearInFlight()
>                   return result
>               } catch {
>                   await self.clearInFlight()
>                   throw error
>               }
>           }
>           inFlight = task
>           return try await task.value
>       }
>       private func clearInFlight() { inFlight = nil }
>   }
>   ```
> - 不要假设"`defer { inFlight = nil }` 在 caller 函数返回时跑 = spawned task 完成时跑" —— 当前 Swift 版本的 `Task.value` 不是 cancellation-aware suspension point，所以这两个时机现在恰好重合，但这是巧合不是契约：未来 Swift 让 Task.value 可被 cancellation 直接 throw 时，整个模式就会破裂；structured concurrency 里 async-let / TaskGroup cancel 传播组合时也可能踩到边界
> - **反例**：
>   ```swift
>   // ❌ 错：清理绑定 caller defer
>   func run() async throws -> T {
>       if let existing = inFlight { return try await existing.value }
>       let task = Task { try await actualWork() }
>       inFlight = task
>       defer { inFlight = nil }            // ← 绑定 caller 函数返回时机
>       return try await task.value
>   }
>   ```
>   ```swift
>   // ❌ 错：用 weak self 让 spawned task "悬空"
>   //（协调器整个生命周期由 DI 容器持有，弱引用反而引入"task 跑完时 self 已没了，inFlight 没清空"的风险）
>   let task = Task { [weak self] in
>       defer { Task { await self?.clearInFlight() } }   // 多层嵌套 + 弱引用，逻辑混乱
>       ...
>   }
>   ```
> - 同模式适用范围：除了 silent relogin，任何"防抖 / 限流 / single-flight 缓存填充 / 共享 future"的协调器都该用这个模式。Server 端如 Go 的 `singleflight.Group` 库内部也是同样的资源生命周期绑定逻辑（fn 完成才 delete key），不是 caller defer

---

## Meta: 本次 review 的宏观教训

codex 给出的具体技术触发链（"`await task.value` 抛 CancellationError"）在当前 Swift 版本下不直接成立 —— 但其指出的**资源管理反模式**是真实且重要的。处理 review 时遇到这种"finding 描述的具体触发条件可能不精确，但底层指出的反模式是真问题"的情况，**应该**按反模式去 fix（即使触发条件不精确），同时在 lesson 里**诚实记录**触发条件的精确边界 —— 这样未来 Claude 既能复用 fix 的预防规则，又不会被不精确的描述误导。

切勿因为"codex 说的具体触发不成立"就 wontfix —— 真实问题被精确化叙述这件事很难，capturing the underlying smell 比 chasing 精确触发链 更有保护价值。
