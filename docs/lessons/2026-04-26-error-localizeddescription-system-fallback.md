---
date: 2026-04-26
source_review: codex round 1 review on Story 2.9 LaunchingView 设计
story: 2-9-launchingview-设计
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — Swift `Error.localizedDescription` 对非 LocalizedError 返回系统串而非空，"isEmpty 兜底" 永远不触发

## 背景

Story 2.9 实装 AppLaunchStateMachine，需要把 bootstrap 抛出的 Error 转成 RetryView 显示的中文文案。占位实装写成"`error.localizedDescription` 非空就用，空了走默认 fallback『登录失败，请重试』"。codex round 1 [P2] 指出：当 step 抛出非 LocalizedError 类型时，`localizedDescription` 仍非空，是 NSError 系统串（`"The operation couldn't be completed (PetApp.SomeError error 1.)"`），用户看到实现细节而非设计文档钦定文案。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `Error.localizedDescription` 对非 LocalizedError 返回系统串 | P2 (medium) | error-handling | fix | `iphone/PetApp/App/AppLaunchStateMachine.swift:97-103` |

## Lesson 1: Swift `Error.localizedDescription` 默认 fallback 是 NSError 系统串，不是空串

- **Severity**: medium (P2)
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/App/AppLaunchStateMachine.swift:97-103`

### 症状（Symptom）

`messageFor(error:)` 用 `error.localizedDescription` 取错误描述：

```swift
private func messageFor(error: Error) -> String {
    let raw = error.localizedDescription
    if raw.isEmpty {
        return AppLaunchStateMachine.defaultFailureMessage
    }
    return raw
}
```

设计意图：err 没自定义描述时回落到"登录失败，请重试"。实际行为：非 LocalizedError 抛出时 `raw` 是 `"The operation couldn't be completed (PetApp.SomeError error 1.)"` —— 非空，所以 isEmpty 分支永远不触发，RetryView 显示英文系统串。

### 根因（Root cause）

Swift 标准库对 `Error` 协议的默认 `localizedDescription` 实现走的是 `NSError` bridging：未实现 `LocalizedError` 的纯 Swift error 会被桥成 `NSError`，其 `localizedDescription` 是 Foundation 的 generic 模板：`"The operation couldn't be completed. (<Module>.<TypeName> error <code>.)"`。**永远非空**。

未来 Claude 写错误 → 用户文案映射时，"localizedDescription 空了走 fallback" 是错误心智模型 —— 没有"空"这条路径会走到。

### 修复（Fix）

显式 `as? LocalizedError` 检查 + 拿 `errorDescription`，**不**碰 `localizedDescription`：

```swift
private func messageFor(error: Error) -> String {
    if let localized = error as? LocalizedError, let desc = localized.errorDescription, !desc.isEmpty {
        return desc
    }
    return AppLaunchStateMachine.defaultFailureMessage
}
```

补测试：抛 plain `Error`（不实现 LocalizedError）验证 message = `defaultFailureMessage`。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **Swift 错误 → 用户文案映射** 场景下，**必须** 用 `as? LocalizedError` + `errorDescription` 检查，**禁止** 依赖 `error.localizedDescription` 的"空兜底"语义。
>
> **展开**：
> - `Error.localizedDescription` 对非 LocalizedError 返回 NSError 桥接生成的英文模板串，**永远非空**
> - 想把"无自定义描述"路径走 fallback，唯一方法是显式 `as? LocalizedError` 类型检查
> - 即使拿到 `LocalizedError`，`errorDescription` 也是 Optional —— 必须 `if let` 解包，再保险地 `!desc.isEmpty`
> - **反例**：`let raw = error.localizedDescription; if raw.isEmpty { fallback }` —— isEmpty 分支永远 dead code，所有用户都看到英文实现细节
> - 与 Epic 5 接入真实 APIError 配合：让 APIError 实现 `LocalizedError` 并提供中文 `errorDescription`，handler 路径自动走第一分支 → fallback 仅在意外类型抛出时触发
