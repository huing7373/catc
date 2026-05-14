---
date: 2026-05-14
source_review: codex review (epic-loop round 1, /tmp/epic-loop-review-18-1-r1.md tail "codex" section)
story: 18-1-表情面板-swiftui
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-14 — Swift actor reentrancy 下 cache-miss path 必须用 inflightTask 才能 single-flight

## 背景

Story 18.1 实装 `actor DefaultLoadEmojisUseCase` 时，文件头注释 + `execute()` 内注释都写"actor 自带串行，多 caller 并发调本方法时仅第一次 hit miss path"。codex round 1 review 指出：actor 仅保证**单 hop** 串行，遇到 `await` 会释放 isolation 让其他 caller 抢进 (actor reentrancy)。两个并发 caller 在 `if cache == nil { try await repo.listEmojis() }` 模式下都能通过 `cache == nil` 检查 → 各发一次 GET /emojis，破坏 AC2 钦定的 "App 生命周期 single-load / cache-once" 契约。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Actor reentrancy 让 cache-miss path 无法 single-flight | P2 (medium) | architecture / concurrency | fix | `iphone/PetApp/Features/Emoji/UseCases/LoadEmojisUseCase.swift:44-55` |

## Lesson 1: Actor reentrancy 让 cache-miss path 无法 single-flight

- **Severity**: P2 (medium)
- **Category**: architecture / concurrency
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Emoji/UseCases/LoadEmojisUseCase.swift:44-55`

### 症状（Symptom）

`actor DefaultLoadEmojisUseCase` 持 `var cache: [EmojiConfig]?` 字段，`execute()` 内：

```swift
if let cached = cache { return cached }
let emojis = try await repository.listEmojis()  // ← reentrancy 释放 isolation
self.cache = emojis
```

两个 EmojiPanelViewModel 并发进 panel 时各自调一次 `execute()`：
- caller A 进 actor → cache == nil → `await repository.listEmojis()` 释放 isolation 等响应
- caller B 此时进 actor → 看到 cache 仍是 nil（A 还没写）→ 也走 miss path → 也发一次 GET /emojis

结果：repo.listEmojis() 被调 N 次（N = 并发 caller 数）；契约写的"App 生命周期 single-load"被破坏。生产风险面：MainTabView 多 tab 同时初始化 ViewModel、push notification 唤起 panel + tab 切换、SwiftUI 视图重建导致 ViewModel 重新创建并触发 init-time load。

### 根因（Root cause）

误把 "actor 串行" 理解成 "actor 方法整体原子"。实际上 Swift actor 的串行保证是 **per-suspension-point**：

- actor 方法在不 `await` 的同步代码段里独占 actor isolation
- 一旦 `await` 一个其他 actor / async 资源（包括 `try await repository.listEmojis()`），当前任务**主动让出** isolation，actor 可以接受其他任务 hop 进来
- 这叫 **actor reentrancy**（[SE-0306 §Reentrancy](https://github.com/apple/swift-evolution/blob/main/proposals/0306-actors.md#actor-reentrancy)）

误信"actor 自动 lock 整个方法"是经典 Swift concurrency 反模式。正确的 single-flight 模式：把"正在进行中的 async 工作"显式存成 `Task<T, Error>?` 字段，后续 caller 看到 inflight 直接 `await task.value` 共享同一次 work。

### 修复（Fix）

actor 内新增 `var inflightTask: Task<[EmojiConfig], Error>?` 字段，`execute()` 改为三态分支：

```swift
public func execute() async throws -> [EmojiConfig] {
    if let cached = cache { return cached }            // ① cache hit
    if let inflight = inflightTask {                    // ② 已有 in-flight load
        return try await inflight.value                 //    → 复用同一 Task
    }
    let task = Task<[EmojiConfig], Error> { [repository] in   // ③ miss + 无 inflight
        try await repository.listEmojis()
    }
    self.inflightTask = task                            //    先存 (让后续 caller 看见)
    do {
        let emojis = try await task.value
        self.cache = emojis
        self.inflightTask = nil
        return emojis
    } catch {
        self.inflightTask = nil                         //    失败清 inflight 让下次 caller 重试
        throw error
    }
}
```

关键点：
- `self.inflightTask = task` 必须在 `await task.value` **之前**赋值 —— 否则同 actor 的后续 caller 在 actor 让出 isolation 后看不到 inflight，又会自己起 Task
- `Task { [repository] in ... }` 用 capture list 把 repository 引用挪进闭包，避免捕获 `self` 形成强引用 cycle（actor 持 Task，Task 持 self 持 Task）
- 失败路径**只清 inflight，不写 cache**（与原契约一致：失败不污染 cache，下次 caller 重新 miss path）

测试侧：新增 `test_execute_concurrentMiss_singleFlight_repoCalledOnce` + 一个 `GatedMockEmojiRepository` actor（`listEmojis()` 挂在 `CheckedContinuation` 上，外部调 `resume()` 才返）。test 起两个 `async let` 并发 caller，等 `gatedRepo.callCount == 1` 后 resume，断言 `callCount == 1`（未修复时此值 == 2）。

顺带修了文件头注释：把"actor 自带串行 → 仅第一次 miss path"误述删掉，换成"actor 仅保证单 hop 串行，遇 await 会 reentrancy，必须 inflightTask single-flight"的精确描述。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **Swift actor 方法里写 "if cache == nil { cache = try await fetch() }" 这种 lazy-init 模式** 时，**必须** **额外存一个 `inflightTask: Task<T, Error>?` 字段做 single-flight**，因为 actor 仅保证单 hop 串行，遇 `await` 会释放 isolation 让其他 caller 进来重复触发 fetch。
>
> **展开**：
> - actor 串行 ≠ 方法原子。理解 actor 时永远默认 "每个 await 都是潜在 reentrancy point"
> - 凡是 actor 字段是 `var x: T?` 且赋值依赖 `await` 拿外部数据的场景，都要审查：两个并发 caller 同时穿过 `if x == nil` 检查会发生什么？如果答案是"会发两次 fetch / 写两次 x"，就需要 inflightTask 模式
> - 标准 pattern（记下来直接套用）：
>   ```swift
>   actor Loader {
>       private var cache: T?
>       private var inflight: Task<T, Error>?
>
>       func load() async throws -> T {
>           if let c = cache { return c }
>           if let t = inflight { return try await t.value }
>           let task = Task<T, Error> { [dep] in try await dep.fetch() }
>           inflight = task
>           defer { inflight = nil }   // 注意：defer 在 actor 中是同步的，但跨 await 时 actor 不一定保留 inflight 引用，所以推荐显式 do/catch 而非 defer，见下面反例
>           let value = try await task.value
>           cache = value
>           return value
>       }
>   }
>   ```
>   实战中推荐 `do { ... cache = v; inflight = nil; return v } catch { inflight = nil; throw }` 形式而非 `defer`，因为 cache 写入只在成功路径，error path 仅清 inflight
> - 测试时**必须**覆盖并发 miss path：单测里用一个"慢 repo"（继续 continuation 形态 / actor + `withCheckedContinuation`）配 `async let firstTask; async let secondTask` 起两个并发 caller，断言 repo.callCount == 1。仅写"先调一次 → 再调一次 cache hit"的串行 case 无法暴露此 bug（既有 4 个串行 case 都通过了，缺这第 5 个并发 case）
> - **反例 1**（actor reentrancy 错认）：
>   ```swift
>   actor Loader {
>       private var cache: T?
>       func load() async throws -> T {
>           if let c = cache { return c }
>           // ↓ actor "自带串行" 的错觉。await 释放 isolation, 并发 caller 都通过 cache==nil 检查
>           let v = try await fetcher.fetch()
>           cache = v
>           return v
>       }
>   }
>   ```
> - **反例 2**（defer 写 inflight = nil 的微妙坑）：
>   ```swift
>   inflight = task
>   defer { inflight = nil }
>   let v = try await task.value   // 这里 task 已经被 inflight 持有；await 期间其他 caller 拿 inflight.value OK；
>                                  // 但 defer 触发时机是函数返回前 —— 在 success path 里 cache = v 也在 defer 之前，
>                                  // 仍然 OK. 不过同步 actor 字段写入和 defer 顺序很容易绕晕，
>                                  // 显式 do/catch 可读性 > defer.
>   ```
> - **反例 3**（捕获 self 的 Task）：
>   ```swift
>   let task = Task { try await self.repository.listEmojis() }
>   // self 是 actor，task 闭包是 nonisolated，引用 self 会要求 hop 回 actor 拿 repository,
>   // 引入不必要的 hop. 用 capture list `[repository]` 显式捕获不可变依赖更轻量.
>   ```

---

## Meta: 本次 review 的宏观教训（可选）

Swift concurrency 误解谱（actor reentrancy 是其中最经典的一条）值得未来 Claude 在写 actor 代码时主动 self-audit：
- 任何 actor 内的 `if x == nil { x = try await ... }` 形态都要立刻问"两个 caller 并发跑会怎样？"
- 任何 cache + lazy-init 在 async 上下文里都需要 inflight pattern（同样适用于 Swift Concurrency Task tree 之外的旧 callback 风格 → 用 NSLock + dispatch_once；这里 actor 让事情看起来简单但实际上需要更主动的 single-flight 设计）
- 串行测试 + 并发测试 是两个独立的覆盖维度；没并发测试的话此类 bug 永远漏到 review 才发现
