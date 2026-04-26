---
date: 2026-04-26
source_review: codex review round 3 (file: /tmp/epic-loop-review-2-7-r3.md)
story: 2-7-ios-测试基础设施搭建
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — Combine `.prefix(N)` 替代手工 fulfill counter，避免 over-fulfillment + 让 publisher 自然 backpressure

## 背景

Story 2.7 落地的 `awaitPublishedChange(on:publisher:count:timeout:)` helper（`iphone/PetAppTests/Helpers/AsyncTestHelpers.swift`）在 round 1 / round 2 修了文档 contract 与 same-run-loop race 之后，round 3 codex 又发现一个相邻问题：sink 内手工 `expectation.fulfill()` 没有上限 — 当被观察的 publisher emit 多于 `count` 个值时（例如 `SampleViewModel.load` 同步先后写 `.loading` 再 `.ready`，调用方只要 `count: 1`），sink 会被多次调用，触发 XCTest 的 over-fulfillment failure。本 lesson 记录"用 Combine 内置 operator 限制流，而不是手工计数"的通用规则，覆盖未来 Claude 写 publisher 测试 helper 时的同类设计选择。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | sink 在 publisher emit 多于 count 时 over-fulfill XCTest expectation | high (P1) | testing | fix | `iphone/PetAppTests/Helpers/AsyncTestHelpers.swift:113-118` |

## Lesson 1: 用 `.prefix(count)` 让 publisher 自动停在第 N 个值，禁手工在 sink 内 fulfill counter

- **Severity**: high
- **Category**: testing
- **分诊**: fix
- **位置**: `iphone/PetAppTests/Helpers/AsyncTestHelpers.swift:113-118`

### 症状（Symptom）

`awaitPublishedChange` 内部用 `XCTestExpectation.expectedFulfillmentCount = count` + sink 闭包里 `expectation.fulfill()` 等待 N 次值变化。问题是 sink 闭包并不知道"够 N 次了该停"，只要 publisher 还在 emit，闭包就继续执行 `fulfill()`：

- 第 N 次 fulfill：`XCTWaiter.fulfillment(of:timeout:)` 返回 `.completed`，await 拿到值
- 第 N+1 次 fulfill：`expectation` 已 over-fulfill，XCTest 报 `API violation - over-fulfilled expectation`，整个测试失败

`cancellable.cancel()` 在 `await fulfillment` 返回**之后**才执行，期间 publisher 已经把更多值灌进 sink。同 run loop turn 内连发 N+1 次 mutation 时这个 race 最致命，因为 publisher 是**同步** emit 的，`fulfillment` 解除等待与 sink 还在跑是同一拍。

### 根因（Root cause）

把"等到 N 个值"实现成"sink 不限量调 fulfill，靠 expectation 计数器倒数到 0"是**两套独立计数器**：

- 上游（publisher → sink）：emit 多少就调多少次
- 下游（XCTestExpectation）：等到 fulfillCount ≥ expectedFulfillmentCount 即解锁

这两套计数器没有通信。上游不知道下游已经够了，继续调；下游一旦被调超量，XCTest 报错。

正确思路：把"够了就停"的责任**留在数据流上游**（publisher 端），用 Combine 内置的 `.prefix(N)` operator 让上游在 emit N 个值后自动 send completion，sink 自然接到 finished 信号不再被调用。这是 Combine 的标准 backpressure 模式 — sink 不该承担流量控制责任，operator 应该承担。

round 1 / round 2 的修复都聚焦于"等待时机正确"，但没有重新审视"等待终止条件如何强制"，所以这个手工 fulfill counter 一直留到 round 3 才被 codex 抓到。教训：写 publisher 测试 helper 时，每加一个状态机假设（"observed ≥ count 后停止"），都要问"上游怎么强制？"而不是"下游怎么截断？"

### 修复（Fix）

在 sink 之前插一个 `.prefix(count)`，让 publisher 在 emit count 个值后自动 finish。`AsyncTestHelpers.swift`：

```swift
// before（有 over-fulfill 风险）
let cancellable = object[keyPath: keyPath]
    .dropFirst()
    .sink { value in
        collector.append(value)
        expectation.fulfill()
    }

// after（publisher 自然 backpressure）
let cancellable = object[keyPath: keyPath]
    .dropFirst()
    .prefix(count)
    .sink { value in
        collector.append(value)
        expectation.fulfill()
    }
```

测试侧补 1 个 case `testAwaitPublishedChangeStopsAtCountWhenPublisherEmitsMore`：构造 publisher 同步 emit 5 次，`count: 2` 调用，断言 `captured == [1, 2]` 且不报 over-fulfillment failure。

语义变化（必读）：之前 contract 是"等到 count 个 mutation 后返回（即使 publisher 之后还会 emit 更多）"；现在变成"取 publisher dropFirst 之后的前 count 个 emit"。在 ObservableObject `@Published` 语境下两者等价（sink 都在 N 次 mutation 后返回相同结果），但新语义更精确 — 它通过 operator 显式表达"我只关心前 N 个"。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写 **"等待 publisher emit N 个值"测试 helper** 时，**必须**用 Combine `.prefix(N)` operator 让 publisher 自动 finish，**禁止**在 sink 闭包里手工调 `expectation.fulfill()` 而不限制上游。
>
> **展开**：
> - 把"够了"的责任放在**上游 operator**（`.prefix` / `.first(where:)` / `.collect(N)`），不放在下游 sink 闭包里手工计数。XCTestExpectation 的 `expectedFulfillmentCount` 只是断言机制，不是流量控制机制 — over-fulfill 会触发 API violation。
> - `.prefix(N)` 在第 N 个值后 send completion，sink 收到 finished 信号自动不再被调用；**自然终止 publisher 链**，无需依赖外部 cancel。
> - 如果需要"等待满足某条件后停止"（不是固定个数），用 `.first(where: predicate)` 而不是 sink 内手工判断 + `cancel()`；同样原因：`.first(where:)` 是上游强制停止，cancel 是异步的、可能慢一拍。
> - **反例 1**（本次 round 3 之前的实现）：sink 闭包内 `expectation.fulfill()` + 调用方调 `cancel()`，但 `cancel()` 在 `await fulfillment` 返回之后才执行，publisher 同步连发多个值时 sink 被调超量。
> - **反例 2**：sink 闭包内自己计数 `if count >= N { return }` —— 仍然每次 emit 都进入 sink 跑闭包，浪费且无法触发 publisher 自然 finish；调用方拿不到 completion 信号。
> - **反例 3**：依赖 `cancellable.cancel()` 同步切断流 —— `cancel()` 不保证 sink 当前正在跑的回调被打断；publisher 已 emit 的值可能在 cancel 之前进入 sink 队列。
> - **正例**：所有"取 publisher 前 N 个值"的测试 helper，都应当形如 `pub.dropFirst().prefix(count).sink { ... }`；把每个有上限的等待表达成 Combine operator 链，而不是 closure 内逻辑。

## Meta: 本次 review 的宏观教训（可选）

`awaitPublishedChange` helper 在 round 1 / 2 / 3 被 codex 三次 review 都发现问题，反映出"测试 helper 同时承担多个职责（订阅时机、流终止条件、错误转 XCTFail）"的设计在每个职责轴上都需要单独的边界检查：

- round 1：文档 contract（initial emit 不算 count）
- round 2：订阅时机 + same-run-loop race（用 `Published.Publisher` 替代 `objectWillChange` + dispatch async）
- round 3：流终止条件（`.prefix` 替代手工 fulfill counter）

通用规则：**写 Combine / async 测试 helper 时，把"订阅—收集—终止—断言"四个职责分开审视**，每个职责都要问"如果 publisher 表现非预期（emit 0 次 / emit 多次 / 同步 emit / 异步 emit），这条职责仍正确吗？"否则 review 会反复揪出同一个 helper 的不同侧面。
