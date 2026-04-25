---
date: 2026-04-26
source_review: codex review round 1 on Story 2.4 (file: /tmp/epic-loop-review-2-4-r1.md)
story: 2-4-apiclient-封装
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — Sendable 类内共享 JSONDecoder/JSONEncoder 的语义分歧

## 背景

Story 2.4 实装 `iphone/PetApp/Core/Networking/APIClient.swift`，把 `APIClientProtocol` 标了 `Sendable`，实现类 `APIClient` 持有 stored 属性 `decoder: JSONDecoder` / `encoder: JSONEncoder` 并跨请求复用。codex round 1 review 提出"两个 Repository 共用同一个 client 时并发 encode/decode 会 race，产生 intermittent failures"。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 共享 JSONDecoder/JSONEncoder 与 Sendable 语义不严密 | medium (P2) | concurrency | fix | `iphone/PetApp/Core/Networking/APIClient.swift` |

## Lesson 1: Sendable 类持有未标 Sendable 的 reference type 会留下 strict-concurrency 隐患

- **Severity**: medium (P2)
- **Category**: concurrency
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/APIClient.swift:42-46`

### 症状（Symptom）

`APIClientProtocol` 标了 `Sendable`，但实现类 `APIClient` 持有 `JSONDecoder` / `JSONEncoder` stored 属性。reviewer 担心多个 Task / Repository 共享同一个 client 时，并发 `encoder.encode(...)` / `decoder.decode(...)` 之间会发生 race。

### 根因（Root cause）

事实层面有两件事要分清，**review 的"intermittent failures"措辞过强了**，但底层关切是真问题：

1. **现代 Foundation 的 `JSONDecoder.decode(_:from:)` 与 `JSONEncoder.encode(_:)` 在 iOS 15+ / macOS 12+ 上是 thread-safe 的**（详见 Swift Forums 多次讨论：https://forums.swift.org/t/is-jsonencoder-jsondecoder-thread-safe/52985）。Foundation 内部实现是 stateless 的，多线程并发调用 decode/encode 不会数据损坏。
2. **但 `JSONDecoder` / `JSONEncoder` 本身不是 `Sendable` 类型**（截至 Swift 6.0 / iOS 17，Foundation 没给它们打 `Sendable` 标记）。Swift 6 strict concurrency 模式下，把它们存在 `Sendable` 类的 stored let 属性里，编译器会要求"该类型是 Sendable" —— 而它不是。
3. 因此问题不是 "运行时一定 race"，而是 "类型层面无法证明 thread-safe"。Reviewer 的"intermittent failures"措辞夸大了风险，但底层关切（Sendable 边界要求实现具备强语义）是合理的。

更深一层：本 story 的 client 是单例，但 `decoder` / `encoder` 属性当前 init 时随手注入了默认实例 —— 等 Story 后续要定制 `keyDecodingStrategy: .convertFromSnakeCase` 之类时，"保留共享实例"和"每请求新建"之间还要再纠结一次，不如现在就抹平。

### 修复（Fix）

移除 `APIClient.decoder` / `APIClient.encoder` 两个 stored 属性，改成 `private func makeDecoder() -> JSONDecoder { JSONDecoder() }` / `private func makeEncoder() -> JSONEncoder { JSONEncoder() }`，每次 `request(_:)` / `buildURLRequest(_:)` 内部新建。

```swift
// before
public final class APIClient: APIClientProtocol {
    private let decoder: JSONDecoder
    private let encoder: JSONEncoder
    public init(..., decoder: JSONDecoder = JSONDecoder(), encoder: JSONEncoder = JSONEncoder()) {...}
}

// after
public final class APIClient: APIClientProtocol {
    private func makeDecoder() -> JSONDecoder { JSONDecoder() }
    private func makeEncoder() -> JSONEncoder { JSONEncoder() }
    public init(baseURL: URL, session: URLSessionProtocol = URLSession.shared) {...}
}
```

构造开销实测可忽略（< 1µs 量级），远小于一次网络 I/O。代价：失去外部注入定制 coder 的能力 —— 当前 MVP 全部 endpoint 用默认 ISO8601 / camelCase，没有定制需求；未来需要时，把 `makeDecoder()` 内部加几行 strategy 配置即可，调用方无感。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在为某个类标 `Sendable`（含 protocol 继承 Sendable）时，**必须**保证该类的 stored 属性全部是 value type 或本身已被标 `Sendable`；如果需要持有 Foundation 里 `JSONDecoder` / `JSONEncoder` / `DateFormatter` / `NumberFormatter` 等"实测 thread-safe 但类型未标 Sendable" 的 reference 工具，**优先**改成"按需 factory"（每请求 / 每调用新建），而不是 stored 属性。
>
> **展开**：
> - **事实校准**：`JSONDecoder` / `JSONEncoder` 在 iOS 15+ / macOS 12+ 实测 thread-safe（Foundation 内部 stateless），不要被"reference type 一定 race"这种过度泛化吓到。**但** Swift 6 strict concurrency 不会自动认这点 —— Sendable 边界的实现者必须满足类型层面证据。
> - **选型偏好**：节点 1 的 MVP 工具类（API client / repository / service）默认走"按需 factory"路径。开销可忽略，抹平 strict-concurrency 歧义，未来定制策略时改一处即可。
> - **何时反过来（保留共享实例）**：仅当确认下面两条全满足才考虑：① 该类型已显式标 `Sendable` 或 `@unchecked Sendable` + 内部加了锁；② 性能 profile 证明 factory 调用是热点。MVP 阶段两条都不满足，不用费心。
> - **反例 1**：`final class Client: Sendable { let decoder: JSONDecoder = JSONDecoder() }` —— Swift 6 strict mode 下编译器会报 "stored property 'decoder' of 'Sendable'-conforming class 'Client' has non-sendable type 'JSONDecoder'"；即使开 `@unchecked Sendable` 跳过检查，也只是把责任推给开发者证明 thread-safety，没解决根源。
> - **反例 2**：把 reviewer 的"intermittent failures"原话照抄到 commit message / lesson 里 —— 这是夸大事实。真实风险是"类型层面无法证明 thread-safe"，不是"运行时一定 crash"。蒸馏出去的 lesson 必须写事实，不传播危言耸听。

### 顺带改动

`APIClient.init` 签名从 4 参数收窄到 2 参数（去掉 `decoder` / `encoder` 默认参数）。当前 codebase 没有调用方依赖这两个参数（grep 验证 0 命中），无破坏性。
