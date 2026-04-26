---
date: 2026-04-26
source_review: codex review round 2, /tmp/epic-loop-review-2-7-r2.md
story: 2-7-ios-测试基础设施搭建
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — `Published.Publisher` 是 mutation 之前同步 emit NEW value，比 `objectWillChange + DispatchQueue.main.async` 更可靠

## 背景

Story 2.7 实装的 `awaitPublishedChange(on:keyPath:count:timeout:)` helper（`iphone/PetAppTests/Helpers/AsyncTestHelpers.swift`）用 `objectWillChange.sink + DispatchQueue.main.async` 让出一拍读 `object[keyPath: keyPath]` 取"变化后"的值。codex round 2 指出这个机制在**同一 run loop turn 内连续两次 mutation** 的场景下会漏中间值：两次 sink 回调被 dispatch 到 main async 后，等回调真正跑时两次 mutation 都已发生，两次都读到 final state，调用方拿到 `[.ready, .ready]` 而非 `[.loading, .ready]`。这个 helper 是后续业务 ViewModel 测试的模板，contract 错一个 → 所有依赖它做状态机断言的下游测试都不可靠。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `awaitPublishedChange` 同 run loop turn 内多次 mutation 漏中间值 | high | testing | fix | `iphone/PetAppTests/Helpers/AsyncTestHelpers.swift:97-104` |

## Lesson 1: 用 Published.Publisher（`\.$field`）订阅而非 objectWillChange，从根本上消灭 dispatch async race

- **Severity**: high
- **Category**: testing（测试基础设施）
- **分诊**: fix
- **位置**: `iphone/PetAppTests/Helpers/AsyncTestHelpers.swift:97-104`

### 症状（Symptom）

旧实现：

```swift
let cancellable = object.objectWillChange.sink { _ in
    DispatchQueue.main.async {
        let value = object[keyPath: keyPath]   // <- 读字段值
        collector.append(value)
        expectation.fulfill()
    }
}
```

`objectWillChange` 在 mutation **之前**触发，所以 sink 同步读会拿到旧值；workaround 是 `DispatchQueue.main.async` 让出一拍再读 —— 假设让出回来时字段已被写好。

但当 `SampleViewModel.load` 这类代码在**同一 run loop turn** 内连发两次 mutation：

```swift
status = .loading                       // willChange 触发 → sink #1 dispatch async
let value = try await useCase.execute() // mock 立即返回 → 不让出 run loop
status = .ready(value: value)           // willChange 触发 → sink #2 dispatch async
// run loop turn 结束 → 两个 dispatch async 同时被调度
```

两个 callback 被 enqueue 到同一个 main queue 的下一拍。等它们跑时，**两次** mutation 都已经完成，两次都读 `object[keyPath: keyPath]` 拿到 final state（`.ready`）。collector 收到 `[.ready, .ready]`。调用方期待 `[.loading, .ready]` → 测试失败或更糟（pass 但断言松弛）。

### 根因（Root cause）

混淆了 SwiftUI 的两套 Combine 接口的"emit 时机":

- `ObservableObject.objectWillChange`（类型 `ObservableObjectPublisher`）→ emit 的是 **void 信号**，**mutation 之前**触发；要拿到 NEW value 必须等 mutation 落定后再读字段，强制引入 dispatch async（或类似让出机制）→ 引入 race
- `@Published<V>.Publisher`（即 `\.$status` 这种 KeyPath，类型 `Published<V>.Publisher`）→ emit 的是 **NEW value 本身**，**mutation 之前**同步发出（见 Apple 文档：emits before the property is set），sink 同步收到，**不需要** dispatch async
  - 同 run loop turn 内多次 mutation → 同步顺序触发多次 sink → collector 顺序 append 拿到完整序列

旧实现选了第一条路（"用 objectWillChange 监所有 @Published 字段，省一个 keyPath → publisher 转换"），但忽略了"读取字段值"这个动作本身需要 dispatch async 制造的 race 漏洞。新实现走第二条路：直接订阅 `Published<V>.Publisher`，emit 的就是 NEW value，没有"读取字段"这一步，没有 dispatch async，没有 race。

更深层思维漏洞：**当一个机制需要"让出一拍"才能拿到正确状态时，警惕它同 turn 内多次触发会让"让出"挤在一起读到 final state。** 这是 dispatch-async-then-read 模式的通病；解法是**消灭"再读"步骤**，让数据直接随事件流动。

### 修复（Fix）

API 改为接 `KeyPath<O, Published<V>.Publisher>`（调用方写 `\.$status`，注意 `$`）。删除旧的 keyPath-to-value API，**不**保留 deprecated overload —— 避免误用。实现：

```swift
public func awaitPublishedChange<O: AnyObject, V>(
    on object: O,
    publisher keyPath: KeyPath<O, Published<V>.Publisher>,
    count: Int = 1,
    timeout: TimeInterval = 1.0
) async throws -> [V] {
    let collector = _AsyncTestCollector<V>()
    let expectation = XCTestExpectation(...)
    expectation.expectedFulfillmentCount = count

    // Published.Publisher 订阅时同步 emit initial → dropFirst() 屏蔽
    // 后续每次 mutation 之前同步 emit NEW value，sink 同步 append
    let cancellable = object[keyPath: keyPath]
        .dropFirst()
        .sink { value in
            collector.append(value)
            expectation.fulfill()
        }

    _ = await XCTWaiter.fulfillment(of: [expectation], timeout: timeout)
    cancellable.cancel()
    return collector.snapshot()
}
```

调用方迁移：`awaitPublishedChange(on: vm, keyPath: \.status)` → `awaitPublishedChange(on: vm, publisher: \.$status)`。

新增测试 `testAwaitPublishedChangeCapturesAllIntermediateValues`（`SampleViewModelTests.swift`）：构造 `Burst.bump()` 在同 run loop turn 内连发 `value = 1; value = 2`，断言 collector 拿到 `[1, 2]` 而非 `[2, 2]`。这条测试是 contract 的可执行守卫，旧实现下会失败、新实现下通过 → 防止未来回退。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写测试 helper 观察 `@Published` 字段变化**时，**必须**用 `Published.Publisher`（`\.$field` keyPath）订阅，**禁止**用 `objectWillChange + DispatchQueue.main.async + 再读字段` 的模式。

> **展开**：
> - **Combine 订阅 emit 时机速查表（重要更新）**：
>   - `@Published<V>.Publisher` (`$field`) → 订阅时 emit current；后续 mutation **之前**同步 emit NEW value（**不**需要 dispatch async 拿 NEW value）
>   - `ObservableObject.objectWillChange` → **不** emit initial；mutation **之前**emit void 信号；想拿 NEW value 必须等 mutation 落定（dispatch async / DispatchQueue 让出）→ 同 turn 多 mutation 的 race
>   - `CurrentValueSubject` → 订阅时 emit current；后续 send() 时同步 emit
>   - `PassthroughSubject` → **不** emit initial；send() 时同步 emit
> - **任何"先发信号再读字段"的模式都暗藏 race**：当读取动作被 dispatch async 让出时，多次连续触发会让所有读取挤在 mutation 完成之后，全部读到 final state；解法是**消灭"再读"步骤**，让 publisher 本身就 emit 所需数据
> - **API 设计偏好可执行 contract**：让 KeyPath 类型本身（`Published<V>.Publisher`）在编译期就强制调用方传 publisher 而非值；helper 没法被错误调用 = 不需要文档说明就能避坑
> - **同 run loop turn 多 mutation 是常态而非边界**：ViewModel 在 async function 里写 `state = .loading; let v = try await ...; state = .ready` 时，如果 mock 同步返回（无真正 await suspension），两次 state 写都在同一 turn；测试这种代码时这是主路径，不是边角
> - **新写 helper 必须有"漏中间值"的反例测试**：构造一个故意在同 turn 内连发多次 mutation 的最小例子，断言 helper 全捕获；旧实现跑这条会失败，新实现通过，回退就会被立刻发现
> - **反例**：旧实现 `objectWillChange.sink { DispatchQueue.main.async { object[keyPath:] } }` 在 mock 立即返回的 happy path 上 100% 漏中间状态。`SampleViewModel.load` + `mock.stubResult = .success(7)` 这套组合是测试基础设施模板的"hello world"路径 → 模板的核心 helper 在最 trivial 的用法下都会错。这种"看似工作但默默错"的 bug 比 fail-fast 错误更恶劣，因为调用方会用更松的断言（`XCTAssertGreaterThanOrEqual(captured.count, 1)`）回避 → 测试通过但什么都没验证

## Meta: 本次 review 的宏观教训

测试基础设施 helper 的 bug 通过率高于业务代码，因为：(1) 没有 production 环境跑它；(2) "测试都 pass" 这一信号本身被作为正确性证据 → 测试自身的 bug 反而最难暴露。对策：helper 的 contract 必须有**反例测试**（故意构造易踩坑的 input 验证 helper 不踩），不是只有 happy path 验证；反例测试本身就是 contract 的可执行规约。
