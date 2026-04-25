---
date: 2026-04-26
source_review: codex review round 1 on Story 2.4 (file: /tmp/epic-loop-review-2-4-r1.md)
story: 2-4-apiclient-封装
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — URLProtocol 测试 stub 的 process-global static 状态隔离

## 背景

Story 2.4 AC8 用 `URLProtocol` 子类作 fake server，配合真 `URLSession` 拦截集成测试的网络层。`StubURLProtocol` 把 stub data / status / error 存在 `static var` 里，每个测试用例 setUp/tearDown 调 `reset()` 清状态。codex round 1 review 提出"XCTest 并行化 / 多 testcase 同时用 StubURLProtocol 时，会跨 testcase 覆写 stub 字段，引发 flaky"。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | StubURLProtocol 全局 static 字段在并行执行下会污染 | medium (P2) | testing | fix | `iphone/PetAppTests/Core/Networking/StubURLProtocol.swift` |

## Lesson 1: URLProtocol stub 的 global state 必须配套并发约束 + 锁 + 文档化

- **Severity**: medium (P2)
- **Category**: testing
- **分诊**: fix
- **位置**: `iphone/PetAppTests/Core/Networking/StubURLProtocol.swift:13-16`

### 症状（Symptom）

`StubURLProtocol` 用 `static var stubData / stubStatusCode / stubError` 持有 stub。同一进程内任意两个 testcase 同时用它时（XCTest 跨 class 并行 / swift-testing / `swift test --parallel`），一个 case 的 `stubData = X` 写入可能在另一 case 的 `startLoading()` 读取过程中被覆写，导致 flaky。

### 根因（Root cause）

事实层面 reviewer 是对的，但要分清两层：

1. **XCTest 默认 same-class scope 是串行**的（一个 XCTestCase 子类内的 testcase 顺序执行）。Story 2.4 的 `APIClientIntegrationTests` 在单个 class scope 内跑 2 个 case，**实际不会触发竞态**。
2. **但**：① 跨 class 并行（默认 enabled）；② 未来引入 swift-testing；③ 任何一个测试作者把 stub 注入挪出 setUp/tearDown 配对 —— 三种情况下 race 都会暴露。这是个"现在不爆，但语义脆弱"的设计债。

URLProtocol 子类的 stub 模式在 iOS 社区被广泛使用（OHHTTPStubs / SwiftNetMock 等），**正确做法是把 stub instance 绑到 URLRequest 上**（用 `URLProtocol.setProperty(_:forKey:in:)`），彻底消除 static state。但这是个完整重构，超出 fix-review 单 commit 边界。

### 修复（Fix）

不重构成 per-request stub instance（避免 scope creep），用三层防御抹平当前问题：

1. **加 NSLock 包静态字段 getter / setter / reset()**：保证最坏情况下不会读出"一半新一半旧"的 stub（写一半时被并发读）。同时新增 `snapshot()` private helper，让 `startLoading()` 在执行开始一次性原子读出三个字段，避免在 send response 中途被并发 reset 打断。
2. **强化文件头注释**：把"任一时刻进程内只允许一个 testcase 在使用 StubURLProtocol"明确写成文件级硬约定，列出"约束 A / B / C"、并标注 swift-testing 的不兼容性。
3. **保留 TODO**：未来 Story 2.7+ 测试基础设施层考虑用 `URLProtocol.setProperty` 把 stub 绑到 URLRequest 上彻底消除 static state；MVP 阶段不做。

```swift
// before（无锁，仅静态字段）
static var stubData: Data?
static var stubStatusCode: Int = 200
static var stubError: Error?
static func reset() { stubData = nil; stubStatusCode = 200; stubError = nil }

// after（NSLock 保护 + snapshot 原子读）
private static let lock = NSLock()
private static var _stubData: Data?
// ... 所有 getter/setter/reset 都包 lock.lock()/unlock()
private static func snapshot() -> (data: Data?, statusCode: Int, error: Error?) {
    lock.lock(); defer { lock.unlock() }
    return (_stubData, _stubStatusCode, _stubError)
}
override func startLoading() {
    let snap = Self.snapshot()  // 原子读，再消费
    ...
}
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写"必须用 static / global state 的测试基础设施"（如 URLProtocol stub / 全局 mock registry）时，**必须**至少同时做三件事：① 用锁保护读写；② 提供原子 snapshot helper 让消费方一次读完；③ 在文件头明确写出"一时刻只允许一个 testcase 用本工具" 的硬约定 + 列出未来重构方向。
>
> **展开**：
> - **不要**听见 reviewer 说"global state 不好"就去做大重构（per-request stub instance）—— 那超出单 commit 边界。先用最小改动（锁 + 文档）抹平急性风险，重构留作后续 story（写 TODO 钩住）。
> - **XCTest 当前默认行为**：same-class scope 串行 / 跨 class 并行（开启）。这意味着单个 XCTestCase 子类内安全，但同一进程多个 *Tests 子类同时用 stub 时不安全。Story 2.4 当前只有 1 个 *IntegrationTests class 用 StubURLProtocol，**目前**安全；但写在文件头注释，未来新增第二个 class 时需要回头看。
> - **swift-testing 不兼容**：swift-testing 默认并行执行 `@Test`，全局 stub 完全没法用。如果未来引入 swift-testing，必须先把 stub 改 per-instance。
> - **反例 1**：`static var stubData: Data?` 没加锁 + 没注释 —— 测试在 CI 上偶发挂，调试 1 天才找到根因。
> - **反例 2**：直接重构成 per-request stub instance + 改所有调用方的 setUp/tearDown 为复杂的 instance lifecycle 管理 —— scope creep 出 fix-review 边界，单 commit 直径变大，review 可信度反而下降。
> - **反例 3**：把锁加在 `startLoading()` 整个函数体外面 —— 锁住期间网络回调还在跑，可能死锁；正确做法是**锁只保护字段读写**，不保护回调。

### 顺带改动

无（修改限定在 `StubURLProtocol.swift` 本体）。
