---
date: 2026-04-26
source_review: codex review round 1, /tmp/epic-loop-review-2-7-r1.md
story: 2-7-ios-测试基础设施搭建
commit: e0c3617
lesson_count: 1
---

# Review Lessons — 2026-04-26 — `ObservableObject.objectWillChange` 不 emit initial value，helper API contract 必须显式声明这一点

## 背景

Story 2.7 实装 `iphone/PetAppTests/Helpers/AsyncTestHelpers.swift` 的 `awaitPublishedChange(on:keyPath:count:timeout:)` helper，用来等待 `@Published` 字段变化 N 次后返回收集到的值数组。codex round 1 指出 doc 注释承诺 `count` **包括 initial 值**（举例 "count: 3 yields `[.idle, .loading, .ready]`"），但实现订阅 `objectWillChange` 不会 emit initial state — 调用方按 doc 写测试会卡 timeout 或少一项。这个 helper 是后续业务 ViewModel 测试的模板，contract 不一致会被复制扩散。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `awaitPublishedChange` doc 承诺含 initial，实现不含 | medium | testing | fix | `iphone/PetAppTests/Helpers/AsyncTestHelpers.swift:75-100` |

## Lesson 1: ObservableObjectPublisher 是"变化通知"语义，订阅时不发送当前值；任何包装它的 helper 都必须把这一点写进 contract

- **Severity**: medium
- **Category**: testing（测试基础设施）
- **分诊**: fix
- **位置**: `iphone/PetAppTests/Helpers/AsyncTestHelpers.swift:75-100`

### 症状（Symptom）

helper 的 doc 注释写：

```
/// - count: 期望的值变化次数（含 initial 值；Combine sink 默认收 initial）
///
/// 用法：
/// let values = try await awaitPublishedChange(on: viewModel, keyPath: \.status, count: 3)
/// XCTAssertEqual(values, [.idle, .loading, .ready])
```

但 implementation 是：

```swift
let cancellable = object.objectWillChange.sink { _ in ... }
```

`ObservableObject.objectWillChange` 的 `Publisher.sink` **不会**在订阅时 emit 当前 state — 它本身就是"willChange"通知（变化前发信号）。所以实际拿到的是 N 次变化（不含 initial）。按 doc 写 `count: 3` 期望拿到 `[.idle, .loading, .ready]` → 实际只能拿到 2 次 willChange（loading 和 ready），第 3 次永远不来 → timeout。

更隐蔽的危害：作为 Story 2.7 的"测试基础设施模板"，业务 story 的 ViewModel 测试都会 copy-paste 它的用法。一个 contract 错误的 helper 会让所有下游测试要么 flaky（通过 lucky timing 加 `Task.sleep` 蒙过去），要么直接挂。

### 根因（Root cause）

混淆了两个 Combine 语义：

- `CurrentValueSubject` / `@Published` 直接订阅（用 `$status.sink`）→ **会 emit 订阅时的当前值** + 后续每次变化
- `ObservableObject.objectWillChange`（实际类型 `ObservableObjectPublisher`）→ 只 emit "即将变化"信号，**不 emit initial**，订阅时静默

helper 用了第二种实现（因为它更省 — 一个订阅监所有 `@Published` 字段，不用拿 keyPath 转 publisher），但 doc 的 mental model 抄的是第一种语义。两个都对，但混在一起就错。

更深层的思维漏洞：写 helper 的 doc 时容易先写"用户希望看到的接口"再写实现，结果 doc 长在 wishful 期望上，implementation 跟不上 — 没有"过一次 doc 跟实现的事实核对"。

### 修复（Fix）

走**路径 B：保留实现，改 contract**（更符合 ObservableObject 真实行为，且不引入额外 KVO 订阅 + 取初值的复杂度）：

doc 注释更新（before / after）：

```diff
- /// - count: 期望的值变化次数（含 initial 值；Combine sink 默认收 initial）
+ /// **Contract**: `count` 表示**变化次数**（即 `objectWillChange` 信号次数），**不含初始值**。
+ /// `ObservableObject.objectWillChange` 是变化通知，订阅时不会 emit 当前 state。
+ /// 调用方若需要 initial，请在调用前自己 `let initial = sut.status` 读出。
+ /// - count: 期望观察到的**变化**次数（不含初始值，默认 1 次）
```

用法举例同步更新：

```swift
let initial = viewModel.status  // .idle —— 调用方自取，helper 不返回
async let trigger: Void = viewModel.load()
let changes = try await awaitPublishedChange(on: viewModel, keyPath: \.status, count: 2)
XCTAssertEqual([initial] + changes, [.idle, .loading, .ready])
```

补一条新测试 `testAwaitPublishedChangeExcludesInitialValue`（`SampleViewModelTests.swift`）显式断言"等待 2 次变化拿到 `[loading, ready]`，第一项不是 initial"，作为 contract 的可执行验证。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写测试 helper 包装 Combine / ObservableObject API**时，**必须先**确认底层 Publisher 的"订阅时是否 emit 当前值"语义（`@Published` 直订是；`objectWillChange` 不是；`CurrentValueSubject` 是；`PassthroughSubject` 不是），然后**在 helper doc 第一段**显式声明这个语义被继承还是被屏蔽。

> **展开**：
> - **Combine Publisher 订阅时 emit initial 的速查表**：
>   - `@Published.projectedValue` (`$field`) → emit initial ✅
>   - `ObservableObject.objectWillChange` → 不 emit ❌
>   - `CurrentValueSubject` → emit initial ✅
>   - `PassthroughSubject` → 不 emit ❌
>   - `Publishers.CombineLatest` → 等所有 upstream 各发一次后才 emit
> - **helper doc 必须是可执行的契约**：写完 doc 立刻补一个 test case 显式验证 doc 里举的例子（"等待 N 次变化拿到 X" 必须有对应的 `XCTAssertEqual`），doc 和 test 互为锚点
> - **被业务复制的模板**优先级最高：本 story 的目标是"建测试基础设施"，被业务 story 复制扩散；模板的 contract 错一个，所有下游都错。模板交付前必须自己跑一遍 doc 里给的所有用法
> - **当 helper 名字是 "change"/"transition" 这类**，强烈倾向"不含 initial"语义（initial 不是 change），doc 也按这个方向写；想要含 initial 就改名为 `awaitPublishedSequence` / `collectPublished` 这类不暗示"change"的命名
> - **反例**：本次提交前 doc 写 `count: 3 yields [.idle, .loading, .ready]`，但 `Task.sleep + objectWillChange.sink` 这套实现一次都拿不到 `.idle`。任何按 doc 写 `count: 3` 的测试都会卡 1 秒 timeout。这种"doc 跟实现各走各"的 helper，比"doc 缺失"更恶劣（缺失会逼调用方读源码，错误会误导调用方继续错下去）

## Meta: 本次 review 的宏观教训

测试基础设施的"模板效应"放大缺陷：一个有 contract bug 的 helper，被业务测试 copy-paste 一次就翻倍其影响面。审查 helper 模板时要把 doc 当成 spec 当成 test 来读 — 用 doc 里的用法跟实现的真实行为对 trace，不一致就回炉。
