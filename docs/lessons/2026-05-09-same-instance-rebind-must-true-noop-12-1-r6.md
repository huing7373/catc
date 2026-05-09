---
date: 2026-05-09
source_review: codex CLI review (epic-loop round 6) — file: /tmp/epic-loop-review-12-1-r6.md
story: 12-1-房间页面-swiftui-骨架
commit: a4dd8dd
lesson_count: 1
---

# Review Lessons — 2026-05-09 — Same-instance rebind 必须 true no-op：consumer 重启需 gated on 实际 client swap / first injection

## 背景

Story 12.1（房间页面 SwiftUI 骨架）codex review round 6。前几轮已修了 client swap 路径（round 5 P2：换 instance 时 disconnect 旧 + cancel task），但 round 6 codex 抓到 same-instance rebind 路径仍**无条件** restart consumer task → in-flight `room.snapshot` 在 rebind 缝隙间被丢。本 review 是房间 ViewModel 异步并发抖动场景的最后一公里。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Same-instance rebind 重启 consumer 丢 in-flight snapshot | P2 | architecture / concurrency | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:108-167` |

## Lesson 1: Same-instance rebind 必须 true no-op，consumer 重启 gated on 实际 client swap / first injection

- **Severity**: P2 (medium)
- **Category**: architecture / concurrency
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:108-167`

### 症状（Symptom）

`RealRoomViewModel.bind(appState:webSocketClient:)` 在以下条件下：vm 已 bound + 在房间内 + 调用方传入**同一个** `WebSocketClient` instance（如 SwiftUI `onAppear` 二次触发、依赖刷新、environment 多次 publish）：

- Round 5 修过的 `oldClient === newClient` 分支正确跳过 `disconnect()` + cancel task ✓
- 但紧接着的 `else if webSocketClient != nil && lastObservedRoomId != nil` 路径**无条件**调 `startConsumingMessages()`
- `startConsumingMessages` 内部：① cancel 当前 consumer ② 在**同一个 `AsyncStream`** 上 start new iterator
- 没调 `prepareForReconnect()` swap 出 fresh stream
- 结果：rebind 期间 in-flight 的 `room.snapshot` 落入 stream buffer，在 cancel→restart 缝隙中被丢

### 根因（Root cause）

ViewModel 的 `bind()` 把"client 是否变更"和"是否需要 (re)start consumer"两个语义混在一起。round 5 修了第一个语义（`===` identity 判定），但忘了把这个判定**也** gate 第二个语义。`else if` 看的是"current state 是否需要起 task"，没看"client 是否真的变了"—— 后者才是 restart 的合法条件。

更深层：`AsyncStream` 不是 multi-iterator 安全的。在同一 stream 上 cancel iterator + start new iterator 是未定义行为，buffered 值的去向不确定（`for await` 协议规定 cancellation 会丢未被 yield 的 element）。所以 same-instance rebind **必须**避免动 consumer task，否则就要先 `prepareForReconnect()` 给一个 fresh stream。

### 修复（Fix）

引入 `clientChanged: Bool` 局部变量在三个分支里显式标记：

```swift
var clientChanged = false
if let newClient = webSocketClient {
    if let oldClient = self.webSocketClient, oldClient === newClient {
        // (a) 同 instance: true no-op
        clientChanged = false
    } else if let oldClient = self.webSocketClient {
        // (b) swap: disconnect 旧 + cancel + swap + prepareForReconnect 新
        oldClient.disconnect()
        self.messageConsumerTask?.cancel()
        self.messageConsumerTask = nil
        self.webSocketClient = newClient
        newClient.prepareForReconnect()
        clientChanged = true
    } else {
        // (c) first injection: 仅 swap
        self.webSocketClient = newClient
        clientChanged = true
    }
}

// ...subscribe 路径...

} else if clientChanged && webSocketClient != nil && lastObservedRoomId != nil {
    // 仅当 client 实际变更才 restart consumer
    self.wsState = .connected
    startConsumingMessages()
}
```

回归测试两条：
- `testSameInstanceRebindDoesNotDropInFlightSnapshot` — rebind 后立即 emit snapshot → 断言 vm 收到（覆盖单次 rebind）
- `testRepeatedSameInstanceRebindPreservesInFlightSnapshot` — 两次连续 rebind + 两次之间 emit → 断言 snapshot 不丢（覆盖抖动）

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 SwiftUI ViewModel 写 `bind()` / `inject()` 类方法时，遇到「同一个底层资源 instance 被重新注入」的情况，**必须**把"资源是否变更"作为**单一布尔**控制下游所有副作用（restart task / 重置 stream / wsState 切换），不能让"current state 看似需要"或"已订阅与否"独立触发副作用。
>
> **展开**：
> - 一个 `bind()` 方法里通常有三种情况：(a) same instance、(b) swap to different instance、(c) first injection from nil。**每种情况**都要明确决定下游所有副作用（disconnect 旧、cancel task、restart consumer、prepareForReconnect、setState）—— 不能"按需"在最后一个 if 链里组合判断。
> - 把 swap/first/same 三态显式记到一个局部布尔（如 `clientChanged`），下游所有 `else if`/`if` 都 gate on 它，绝不让"看似需要起 task"的状态机条件独立触发 restart。
> - **`AsyncStream` 不安全的多 iterator**：同一 stream 上 cancel iterator + start new iterator 会丢 buffered value（cancellation 期间 yielded value 去向未定义）。如果**确实**需要 restart consumer 复用同 client，必须先调 `prepareForReconnect()` swap 出 fresh stream，再起新 iterator。
> - **反例**：
>   ```swift
>   // 错：上面分类完 client 是否变了，下面又"看 state 需不需要"独立判断
>   if oldClient === newClient { /* no-op */ } else { /* swap */ }
>   if !subscribed { subscribe() }
>   else if webSocketClient != nil && inRoom { startConsumingMessages() }  // ← BUG: same-instance 也走这里
>   ```
>   ```swift
>   // 错：复用同 stream 起新 iterator
>   self.consumerTask?.cancel()
>   self.consumerTask = Task { for await m in client.messages { ... } }  // ← BUG: 没 prepareForReconnect
>   ```
> - **正例**：把 swap/first/same 标到 `clientChanged`，下游所有副作用都 gate 它；swap 路径主动 `prepareForReconnect()` 给新 client 一个 fresh stream，避免 buffered 消息在 cancel/restart 缝隙间丢失。
> - **回归测试模式**：写 same-instance rebind 测试时，**关键时序**是「rebind 后立即 emit + 两次连续 rebind 之间 emit」—— 这两个时序专门暴露 cancel-restart-on-same-stream 的丢消息 bug。光测「rebind 后还能 emit」不够，必须测「rebind **缝隙间** emit」。
