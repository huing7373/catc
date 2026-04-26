---
date: 2026-04-26
source_review: codex review round 1 — Story 2.6 基础错误 UI 框架
story: 2-6-基础错误-ui-框架
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-26 — 队列型 UI 状态机存储 presentation 必须连带 callback 一起入队

## 背景

Story 2.6 引入 `ErrorPresenter`：用 `current: ErrorPresentation?` 当前展示项 + `queue: [ErrorPresentation]` FIFO 等待队列。`.retry` 这个 case 由 caller 在 `present(_:onRetry:)` 时注入 `() -> Void` 闭包，闭包**不**写在 `ErrorPresentation` 上（避免污染 Equatable 合成），而是单独存在 `pendingOnRetry` 字段。

第一版 `enqueue(_:onRetry:)` 在 current 非空时只把 presentation push 进 queue，**丢弃 onRetry**：

```swift
private func enqueue(_ presentation: ErrorPresentation, onRetry: (() -> Void)?) {
    if current == nil {
        present(presentation, onRetry: onRetry)
    } else {
        queue.append(presentation)
        // onRetry 入队时先丢失：当前实现不支持队列项携带 onRetry。
    }
}
```

注释里把这个标成"设计权衡 / MVP 不支持队列内 retry"，但 codex round 1 判断这是**真 bug**：`.retry` 入队是可发生场景（e.g. 一条 toast 正展示，紧接着发起的请求 timeout 触发 `.retry`），用户最终点 retry 按钮会以为重试了，实际只是 dismiss。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | enqueue() 丢 onRetry callback | high (P1) | error-handling | fix | `iphone/PetApp/Shared/ErrorHandling/ErrorPresenter.swift:91-99` |

## Lesson 1: 队列存的是 (presentation, onRetry) tuple，不是 presentation

- **Severity**: high
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/Shared/ErrorHandling/ErrorPresenter.swift:29, 84, 91-99`

### 症状（Symptom）

调用方按 `presenter.present(error, onRetry: { /* re-fire request */ })` 注入重试动作；如果此时 presenter 已有正在展示的 `current`（例如更早的一条 toast），新 presentation 入队，**onRetry 闭包被丢弃**。等 toast 自动消失、retry 弹出，用户点"重试"按钮：UI 从屏幕消失（dismiss 走通），但**重试动作不执行** —— 调用方期待的"二次请求"永远不发出。

UI 看起来正常，行为静默失败。

### 根因（Root cause）

错误地把 `(presentation, onRetry)` 拆成两个生命周期分别管理：
- `presentation` 进 queue
- `onRetry` 只在 `present(_:onRetry:)` 立即展示路径上送进 `pendingOnRetry`

队列里没有 onRetry 的位置；caller 注入的闭包在入队那一刻就丢了。

设计时把"queue 内 retry 概率低"当成可接受边界，但 ErrorPresenter 的契约是 `.present(error, onRetry:)` —— caller 不知道也不应关心当前是否已有展示项；只要 `.retry` 弹出，onRetry 就该可被触发。**契约不该因为内部状态机的实现细节而打折扣**。

### 修复（Fix）

queue 元素类型从 `[ErrorPresentation]` 改成 `[(presentation: ErrorPresentation, onRetry: (() -> Void)?)]`：

```swift
private var queue: [(presentation: ErrorPresentation, onRetry: (() -> Void)?)] = []

private func enqueue(_ presentation: ErrorPresentation, onRetry: (() -> Void)?) {
    if current == nil {
        present(presentation, onRetry: onRetry)
    } else {
        queue.append((presentation: presentation, onRetry: onRetry))
    }
}

// dismiss 推进队列时把 onRetry 一起出队
if !queue.isEmpty {
    let next = queue.removeFirst()
    present(next.presentation, onRetry: next.onRetry)
}
```

测试：`testQueuedRetryPreservesOnRetryCallback` ——
- presentToast("first")（占用 current）
- present(.network, onRetry: spy)（入队）
- dismiss → retry 弹出
- dismiss(triggerOnRetry: true) → spy 被调用 1 次

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **设计支持队列的 UI 状态机** 时，**必须** 把 caller 注入的所有 callback / 上下文 **与 presentation 一起作为复合元素入队**，不可以只入队 presentation 本体。
>
> **展开**：
> - 队列元素的字段集 = "立即展示路径需要哪些字段" 的全集。如果 `present(p, onRetry:)` 同时消费 p 和 onRetry，那 queue 元素就必须能承载 (p, onRetry)，而不是只 p。
> - "入队时丢 callback / 留 TODO 以后改" 是反模式：caller 看到的是"public API 接受 onRetry 闭包"，不知道你内部按队列状态丢弃。一旦丢弃，调用方的重试逻辑静默不触发，UI 行为正确但语义错。
> - 闭包不能放进 Equatable 的 enum case，但可以**和 case 配对组成 tuple / struct**。在 queue / pendingX 这类内部容器里用 tuple / 简单 struct 把"展示态 + 闭包"绑成一个 unit。
> - **反例**：在内部容器（queue / map / cache）只存 Equatable 字段、把 closure / context 拆到旁路字段去管理，靠"立即展示路径"暗中维持 pairing —— 一旦走非立即路径（入队 / 跳到队尾 / 替换 current），pairing 立刻断裂。
> - **反例 2**：在源码注释里写"MVP 不支持 X / 设计权衡：X 概率低"作为 callback 丢失的理由。Public API 的契约不允许"概率低就不实现"；要么从 API 删掉这个能力（让调用方编译期看见），要么实现到位。注释不能替代行为。

## Meta: 本次 review 的宏观教训

`ErrorPresenter` 这类"含队列的 UI 状态机"是 SwiftUI MVVM 里典型的容器型 ObservableObject。queue + current 双状态时，要把所有"原本立即处理路径上消费的输入" —— 不只是要展示的数据，还包括 callback / 触发上下文 / 测试钩子等 —— 视为同一 unit 入队。后续若再扩展状态机（如加 `.confirm(message:onConfirm:onCancel:)` 这种带双 callback 的 case），更要警惕同一个 enum + 多个旁路字段的组合容易丢同步。
