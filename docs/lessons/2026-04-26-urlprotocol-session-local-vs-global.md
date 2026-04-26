---
date: 2026-04-26
source_review: codex review round 2 on Story 2.4 (file: /tmp/epic-loop-review-2-4-r2.md)
story: 2-4-apiclient-封装
commit: 5d97a74
lesson_count: 1
---

# Review Lessons — 2026-04-26 — URLProtocol 测试拦截：session-local 注入 vs process-global register

## 背景

Story 2.4 `APIClientIntegrationTests` 同时使用了两种 URLProtocol 注入路径：

1. `URLSessionConfiguration.protocolClasses = [StubURLProtocol.self]` — **session-local**，只拦截这个 session。
2. `URLProtocol.registerClass(StubURLProtocol.self)` — **process-global**，hook 整个测试进程的 URL loading。

两者目标重叠（让 StubURLProtocol 接管请求），但语义截然不同。codex round 2 review 指出全局 register 是冗余且有害的——它会让任何**并行运行的其它 \*Tests 子类**发出的 `URLSession` 请求被 StubURLProtocol 接管，把它们的回调 hook 到本 class 的 static stub 字段上，引发跨 test 污染 / flaky。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `URLProtocol.registerClass` 把 stub 范围放大到整个 test process | medium (P2) | testing | fix | `iphone/PetAppTests/Core/Networking/APIClientIntegrationTests.swift:22-32` |

## Lesson 1: session-local 拦截已经够用，process-global register 是反模式

- **Severity**: medium (P2)
- **Category**: testing
- **分诊**: fix
- **位置**: `iphone/PetAppTests/Core/Networking/APIClientIntegrationTests.swift:22-32`

### 症状（Symptom）

`setUp` 里写：

```swift
StubURLProtocol.reset()
URLProtocol.registerClass(StubURLProtocol.self)   // ← 进程级 hook
```

`makeClient()` 又写：

```swift
let config = URLSessionConfiguration.ephemeral
config.protocolClasses = [StubURLProtocol.self]   // ← session 级 hook
```

两条都生效。XCTest 默认开启 cross-class 并行（`parallelizable = true`），同一进程里另一个 \*Tests 子类如果也用 `URLSession.shared` 或自己 new 的 session 发请求，会因为全局 register 被 StubURLProtocol 接管，读到本 class 设置的 `stubData` / `stubStatusCode`，得到错误响应。受害方完全看不出问题来源（"我又没用 StubURLProtocol，怎么会被 hook"）。

### 根因（Root cause）

`URLProtocol` 的拦截在 Foundation 内有两条路径：

- `URLSessionConfiguration.protocolClasses` — 这是给**特定 session** 的 protocol 链。session 实例创建时拷贝快照，session 销毁就跟着没了。**完全 session-local**。
- `URLProtocol.registerClass(_:)` / `URLProtocol.unregisterClass(_:)` — 这是**进程全局**的 protocol 注册表，影响所有 `URLSession.shared` 以及未自定义 `protocolClasses` 的 session。

集成测试只想测"自己 new 出来的 session 走 stub"，那只需要第一条；第二条把范围放大到了"整个进程的所有 URLSession"，几乎肯定会误伤同一进程其它测试。

为什么会同时写两条？常见动机是"双保险"——担心 session 注入不生效就靠全局 register 兜底。这个动机本身就有问题：① session 注入是 Apple 公开 API、契约稳定，不需要"兜底"；② 真要担心 session 注入不生效，应该写一个**断言**（在 stub 的 `canInit(with:)` 里 print / count）确认 stub 被命中，而不是悄悄把范围放大。

### 修复（Fix）

删除 setUp/tearDown 里的全局 `registerClass` / `unregisterClass`，只保留 `StubURLProtocol.reset()`（清 static 字段仍然必要，因为 stub data 是 process-global 的——见 sibling lesson `2026-04-26-urlprotocol-stub-global-state.md`）。文件头注释更新，明确"仅 session-local 注入"以及"为什么不用全局 register"。

```swift
// before
override func setUp() {
    super.setUp()
    StubURLProtocol.reset()
    URLProtocol.registerClass(StubURLProtocol.self)        // ← 删
}

override func tearDown() {
    URLProtocol.unregisterClass(StubURLProtocol.self)      // ← 删
    StubURLProtocol.reset()
    super.tearDown()
}

// after
override func setUp() {
    super.setUp()
    StubURLProtocol.reset()
}

override func tearDown() {
    StubURLProtocol.reset()
    super.tearDown()
}
```

`makeClient()` 内的 `config.protocolClasses = [StubURLProtocol.self]` 不动——这是真正负责拦截的那行，它 session-local 即可工作，不依赖全局注册表。

回归验证：APIClientIntegrationTests 的 2 个 case 仍然通过（`testFullStackHappyPath` / `testFullStackBusinessError`）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 用 `URLProtocol` 子类做集成测试 stub 时，**只能**通过 `URLSessionConfiguration.protocolClasses` 注入 stub 到自己 new 出来的 session，**禁止**调 `URLProtocol.registerClass(_:)`（除非你是写整个进程默认 URL loading 行为的工具）。

> **展开**：
>
> - **session-local 与 process-global 是两个工具**：前者用于"我控制的这个 session 走 stub"，后者用于"整个进程任何 URLSession 都走 stub（含 URLSession.shared）"。集成测试 99% 场景属于前者；只有少数极端场景（如要 hook 第三方 SDK 内部 hardcoded 的 URLSession.shared）才该用后者，且必须保证测试 class 串行执行。
> - **不要把 register/unregister 当成 session 注入的"双保险"**。这两条不是叠加冗余，而是**两种语义**。同时写两条 = 把 stub 范围放大到 hold 不住的边界。
> - **判断标准**：写 stub 注入时问自己"我想拦截的请求是来自我自己 new 的 session（已知），还是来自我无法控制的某个 SDK 内部的 session（未知）？" 前者用 `protocolClasses`，后者才考虑 register（且要写注释说明为何不能用 session 注入）。
> - **配套约束 vs 互斥约束**：本次修复后仍然要 reset() static stub 字段，因为 `StubURLProtocol` 的 `stubData` / `stubStatusCode` / `stubError` 是 process-global static——这条约束**和本次修复不冲突**。session-local 只让"哪个 session 走 stub"被限定；它不能让"stub 字段值"被限定。后者由 sibling lesson 的 NSLock + 文件头注释方案兜底。
> - **反例 1**：setUp 里同时写 `URLProtocol.registerClass` 和 `config.protocolClasses = [...]`——本次踩的就是这条。
> - **反例 2**：在 tearDown 里 register 完忘记 unregister——栈式 register 会越积越多，进程级 hook 永远不消，后续测试全炸。
> - **反例 3**：用全局 register 之后又用 `XCTestSuite.parallelizable = false` 强制串行测试套绕过污染——把可并行的测试集变成 serial，整个 CI 跑慢，违反测试可扩展性原则。修复方向应该反过来——先 fix 测试代码让它能并行，而不是禁掉并行兜锅。

### 顺带改动

- `APIClientIntegrationTests.swift` 文件头注释扩展（说明拦截策略 = session-local 且为何不用 global register；引用 sibling lesson）。
- 与 sibling lesson `2026-04-26-urlprotocol-stub-global-state.md` 形成互补：一个管"哪个 session 走 stub"（本 lesson），一个管"stub 字段值的 process-global static state"（sibling）。两个问题独立，修复各自独立。

## Meta

本次 round 2 的两条 P2 都属于"接口边界吸收"主题——一条是 URL 拼接边界（baseURL/path），一条是测试拦截边界（session/process）。共同教训：**写 client / test infra 时，一旦发现两侧契约重叠或范围溢出，必须在边界处一次性吸收，不要让歧义流到下游**。
