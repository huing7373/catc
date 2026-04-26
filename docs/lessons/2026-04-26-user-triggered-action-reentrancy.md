---
date: 2026-04-26
source_review: codex round 1 review on Story 2.9 LaunchingView 设计
story: 2-9-launchingview-设计
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — 用户触发的 retry 类异步 action 必须自带并发短路 guard，不能复用 idempotency flag 替代

## 背景

Story 2.9 AppLaunchStateMachine 已经有 `hasBootstrapped` flag 防 SwiftUI `.task` 重启时重复跑 bootstrap（参考 lesson `2026-04-26-swiftui-task-modifier-reentrancy.md`）。但 `retry()` 方法的语义是"重新跑"，所以会主动 `hasBootstrapped = false` 再调 `bootstrap()`。codex round 1 [P2] 指出：用户连点两次 retry 时，第二次 retry 在第一次还在飞中时清掉 flag → 两个 bootstrap closure 并发跑，重复发请求 + race 最终 state 写入。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | retry 重入导致 hasBootstrapped 双 reset → bootstrap 并发跑 | P2 (medium) | concurrency | fix | `iphone/PetApp/App/AppLaunchStateMachine.swift:88-91` |

## Lesson 1: 用户触发的 retry/refresh 类异步 action 必须自带 isXXXing flag，bootstrap 类一次性 idempotency flag 不够用

- **Severity**: medium (P2)
- **Category**: concurrency
- **分诊**: fix
- **位置**: `iphone/PetApp/App/AppLaunchStateMachine.swift:88-91`

### 症状（Symptom）

```swift
public func retry() async {
    state = .launching
    hasBootstrapped = false  // 让 bootstrap 可再跑
    await bootstrap()
}
```

用户连点 RetryView 重试按钮（实际场景：网络抖动后用户 impatient 多按几下）→ 第二次 `retry()` 在第一次 `bootstrap()` 还在 await 中时清 `hasBootstrapped` → 第一次的 bootstrap closure 已过 guard 还在飞中，第二次也过 guard 重新跑 → 真实 GuestLoginUseCase 接入后会发两次 /v1/login 请求，且 state 最终值取决于哪个 closure 后写。

### 根因（Root cause）

`hasBootstrapped` 是为 SwiftUI `.task` 多次触发设计的"一次性 idempotency"flag，**只在 `bootstrap()` 入口检查**。`retry()` 故意 reset 这个 flag 再调 `bootstrap()` —— 短路语义被显式打破，所以 retry 自身需要独立的并发短路 guard。

混淆点：两类 reentrancy 是**正交**的：
- `.task` 重启 = 系统触发的 lifecycle 事件 → 用 `hasBootstrapped`（一次过永久不再跑）
- 用户连点 retry = 用户主动触发的 action → 用 `isRetrying`（in-flight 期间丢弃重复触发，跑完释放）

### 修复（Fix）

加 `isRetrying` flag + `defer` 释放：

```swift
private var isRetrying: Bool = false

public func retry() async {
    guard !isRetrying else { return }
    isRetrying = true
    defer { isRetrying = false }

    state = .launching
    hasBootstrapped = false
    await bootstrap()
}
```

补测试：concurrent 两次 retry → step1 计数从初始 1 增到 2（不是 3）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **用户主动触发的异步 action（retry / refresh / submit）** 场景下，**必须** 给该方法加独立的 in-flight flag + `defer` 释放，**禁止** 复用同模块"一次性 idempotency"flag 替代。
>
> **展开**：
> - "一次性 idempotency"flag（如 `hasBootstrapped` / `hasInitialized`）只在调用入口检查，没有"跑完释放"语义；用户触发的 action 如果会反向 reset 它，等于打破短路
> - 用户触发 action 的并发模型：`isXXXing` flag + `defer { isXXXing = false }` —— in-flight 期间丢弃重复触发，跑完释放允许下一轮触发
> - `@MainActor` 类内的 Bool flag 已是单线程访问安全，不需要 actor / lock
> - **反例**：`func retry() async { hasBootstrapped = false; await bootstrap() }` —— 用户连点 N 次 → bootstrap 并发 N 份，同样的 reset 又会让其他 .task 触发的 bootstrap 也意外重跑
> - 与 SwiftUI `.task` reentrancy lesson 协同：`.task` 触发场景用 `hasBootstrapped`（永久），用户 action 场景用 `isRetrying`（瞬时），两者**不互斥**且**职责不同**
