---
date: 2026-04-26
source_review: codex review round 2, /tmp/epic-loop-review-2-7-r2.md
story: 2-7-ios-测试基础设施搭建
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — `MockBase` 内部存储字段一律 `private`，只通过 snapshot helper 读 — 不要 expose mutable storage（即使 `private(set)`）

## 背景

Story 2.7 实装的 `MockBase`（`iphone/PetAppTests/Helpers/MockBase.swift`）作为业务 mock 的通用基类，提供 `record(...)` 在 NSLock 内写 `invocations` / `lastArguments` / `callCounts` 三个字段，对外承诺线程安全。codex round 2 指出 `public private(set) var invocations` 这种声明的 read 路径**不**经过锁 —— 任何 async 测试在 reader task 读 `mock.invocations` 同时另一个 task 调 `mock.record(...)` 写时，TSAN 会报 race，或读到部分更新的状态。snapshot helper（`invocationsSnapshot()` 等）已经存在并加锁，但只是"可选"路径而非"唯一"路径。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `MockBase` mutable 字段 public 暴露 → bypass NSLock 的 race | medium | testing | fix | `iphone/PetAppTests/Helpers/MockBase.swift:52-58` |

## Lesson 1: 任何承诺线程安全的 class，mutable 字段必须 `private`，只通过加锁的 snapshot 方法读 — `public private(set)` 是错觉

- **Severity**: medium
- **Category**: testing（测试基础设施）/ concurrency
- **分诊**: fix
- **位置**: `iphone/PetAppTests/Helpers/MockBase.swift:52-58`

### 症状（Symptom）

旧声明：

```swift
public class MockBase {
    public private(set) var invocations: [String] = []
    public private(set) var lastArguments: [Any] = []
    public private(set) var callCounts: [String: Int] = [:]
    private let lock = NSLock()

    public func record(method: String, arguments: [Any] = []) {
        lock.lock(); defer { lock.unlock() }
        invocations.append(method); ...
    }

    public func invocationsSnapshot() -> [String] {
        lock.lock(); defer { lock.unlock() }
        return invocations
    }
}
```

`record(...)` 写 invocations 时拿锁，但调用方 `mock.invocations` 直接读字段时**不**经过锁。Swift `[String].append` 内部会重分配 buffer，部分写中途读 → undefined behavior。即使 `Array.append` 在某些情况下能侥幸跑通，TSAN（thread sanitizer）在并发测试场景下必报 data race。

类层面的承诺是"thread-safe，可以多 task 并发调"，但接口允许调用方 bypass 锁直接读 → 承诺不成立。`private(set)` 只阻止外部**写**，不阻止外部**读** —— 而读 mutable 字段没有锁就是 race。

### 根因（Root cause）

`public private(set)` 是 Swift 鼓励的 immutable-from-outside 模式，但这个模式默认**只针对线程内顺序**安全（"外部不能直接写，所以单线程 reader 拿到的是有效状态"）。一旦类承诺**多线程**安全，这个模式就失效 —— 外部读和内部写并发发生 → race。

更深层思维漏洞：把"只读访问"等同于"无害访问"是单线程时代的直觉，多线程下读 mutable 字段同样需要内存屏障 / 锁保护。设计 thread-safe class 时应当**默认禁止外部直接读**，即使是 `private(set)`，把所有读路径强制走锁内 snapshot 方法。

### 修复（Fix）

把三个存储字段降级为 `private`，让现有的 `*Snapshot()` 系列成为唯一 read API。新增 `lastArgumentsSnapshot()` 补齐对称性（之前少一个）：

```diff
 public class MockBase {
-    public private(set) var invocations: [String] = []
-    public private(set) var lastArguments: [Any] = []
-    public private(set) var callCounts: [String: Int] = [:]
+    private var invocations: [String] = []
+    private var lastArguments: [Any] = []
+    private var callCounts: [String: Int] = [:]

     public func invocationsSnapshot() -> [String] { lock.lock(); ...; return invocations }
+    public func lastArgumentsSnapshot() -> [Any]   { lock.lock(); ...; return lastArguments }
     public func callCountsSnapshot() -> [String: Int] { lock.lock(); ...; return callCounts }
 }
```

调用方迁移：grep 全部测试代码，把 `mock.invocations` / `mock.lastArguments` / `mock.callCounts` 替换为对应的 `*Snapshot()` 调用。本次只有 `SampleViewModelTests.swift:55` 一处直接读 `mockUseCase.lastArguments.first`，迁移为 `mockUseCase.lastArgumentsSnapshot().first`。`MockURLSession` / `MockAPIClient`（不继承 MockBase 的 networking 专用 mock）保持原样。

snapshot 已有的 NSLock 测试覆盖 `record + read` 并发安全；不必加新测试。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写承诺 thread-safe 的 class**时，mutable 字段**必须** `private`，**禁止**用 `public private(set)` 把它暴露为可外部读 —— 即使有 `private(set)` 阻止外部写，外部 reader 仍会 bypass 内部锁形成 race。

> **展开**：
> - **thread-safe class 的字段可见性规则**：
>   - 写：方法内 lock-protected
>   - 读：**只**通过 method 返回锁内 snapshot 拷贝（`*Snapshot()` 命名约定）
>   - 字段本身：`private`，**不**用 `private(set)`
> - **`private(set)` 仅适用于单线程不可变性场景**：当 class 承诺单线程语义（如 SwiftUI `@Published` 在 main actor 上）时 OK；承诺多线程时不够
> - **TSAN 是检验 thread-safe 承诺的客观标准**：任何 thread-safe 类的承诺必须能跑过 `-sanitize=thread` 而不报错；做不到就降级承诺为"single-thread access only"，把字段改成 `internal`/`private` 把 race 风险拒在外面
> - **lessons 同主题汇编**：
>   - `2026-04-26-urlprotocol-stub-global-state.md`：URLProtocol stub 的全局可变状态 → snapshot + NSLock + per-test reset
>   - `2026-04-26-jsondecoder-encoder-thread-safety.md`：JSONDecoder/Encoder 不是 thread-safe → 短临界区 lock 包装
>   - 本 lesson：MockBase 的 thread-safe contract → 强制 snapshot-only 读
>   - 三者共同主题：**承诺 thread-safe 的 API 必须把可变状态藏在锁内，对外只暴露 snapshot 或纯函数 API**
> - **API 命名约定固化**：snapshot-only-reads 模式下 read API 全部用 `*Snapshot()` 后缀，让调用方一眼看出"我读到的是拷贝"，且方便 grep 审核（没有 Snapshot 后缀的字段访问 = 可疑）
> - **反例**：旧 MockBase 把 `invocations` 标 `public private(set)` → 调用方写 `mock.invocations.count` / `mock.invocations.first?.url` 这种链式访问会自然冒出来；reviewer 看到 `private(set)` 容易误认为"外部不能改、所以安全"。但该 class 同时承诺多线程并发 record，外部读 + 内部写就是经典 race。这种"接口允许 bypass 锁但口头承诺线程安全"的 class 是 production code 中数据损坏 bug 的常见来源

## Meta: 本次 review 的宏观教训

承诺 thread-safe 的 API 设计不止"加锁"一项 —— 还要保证调用方**没有任何路径**可以 bypass 锁。Swift 的 `public private(set)` / Kotlin 的 `val`-with-getter / Go 的 exported fields 等"读访问看似无害"的模式都不抗多线程；写 thread-safe 类时把可见性收到最紧（`private`），通过 snapshot/纯函数对外暴露。
