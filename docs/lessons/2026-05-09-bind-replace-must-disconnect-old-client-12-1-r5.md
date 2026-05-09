---
date: 2026-05-09
source_review: codex review (epic-loop round 5) — /tmp/epic-loop-review-12-1-r5.md
story: 12-1-房间页面-swiftui-骨架
commit: 791d942
lesson_count: 1
---

# Review Lessons — 2026-05-09 — `bind()` 替换 client instance 必须 tear down 旧 client（12-1 r5）

## 背景

Story 12.1 房间页面 SwiftUI 骨架第 5 轮 codex review。前 4 轮都集中在 ViewModel 的「订阅起步 / room A→B 切换 / stale snapshot / isHost 推断」等 publisher / stream / merge 契约层；本轮 codex 把视角拉到**注入入口本身**：`RealRoomViewModel.bind(appState:webSocketClient:)` 在 vm 已 bound + 在房间内时被传入**不同的** WebSocketClient instance 时，旧实装只 swap `self.webSocketClient` 字段就直接 `startConsumingMessages()`，**不**对旧 client 调 disconnect、**不** cancel 旧 messageConsumerTask。结果：

- 旧 socket 仍 subscribed 在后台 → 资源泄漏（连接 / continuation / 后台 task 都活着）
- 旧 client 仍可能 deliver 消息（mock 直接 emit；真实 client 上是排队 frame）→ ViewModel 已经 swap 到新 client 但旧 stream 上的消息仍被处理 = **duplicate room traffic**
- 旧 messageConsumerTask 没 cancel → for-await 仍在旧 stream 上等下一条消息（mock 不 finish stream 就永远不退）

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | bind() 替换 webSocketClient instance 时未 disconnect 旧 client | P2 | architecture | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:106-125` |

## Lesson 1: 注入入口替换持有的资源时必须先 tear down 旧的，identity 比较走 `===`

- **Severity**: P2
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:bind(appState:webSocketClient:)`

### 症状（Symptom）

`bind(appState:webSocketClient:)` 在 vm 已 bound + 已经在房间内时，被以**不同的** `WebSocketClient` instance 重新调用：

- 旧实装：`self.webSocketClient = client` 直接 swap，再 `startConsumingMessages()` 起新 task
- 旧 client 没收到 `disconnect()` → 后台 socket / stream / continuation 仍活
- 旧 messageConsumerTask 没 `cancel()` → 旧 for-await loop 还在等旧 stream 的消息
- 真实路径下：旧 client 的排队 frame 仍可能进入 vm → duplicate room traffic + UI 状态紊乱

### 根因（Root cause）

bind() 的语义只考虑了**首次注入**（旧 = nil → 新 = 某 client）和**保留同一 instance 重 bind**两种正常路径。「替换为不同 instance」是个**完全合法的**第三条路径（Story 12.7 LeaveRoomUseCase 落地后、或测试 helper rebind、或 DI 容器在生命周期切换时 reinject 都会走这条），但旧实装把它当成「与首次注入同等」处理 —— 漏了「旧 client 的资源/订阅生命周期」。

更深层的原因：注入入口的代码模型容易盯着「写新值」忘记「旧值的 tear down」，尤其当字段类型是 protocol / 引用类型（class）时，swap 不会触发 deinit（旧 instance 仍被 caller / 测试 / 其他持有者强引用）。

### 修复（Fix）

`bind()` 内对 `webSocketClient` 字段做三分支处理：

1. **同 instance（`===`）重 bind**：no-op（不动 webSocketClient / 不 disconnect / 不 cancel task）
2. **不同 instance + 旧不为 nil**（替换路径）：`oldClient.disconnect()` + `messageConsumerTask?.cancel() + = nil`，再 swap
3. **旧 = nil 首次注入**：仅 swap

identity 比较用 `===`：`WebSocketClient` protocol 已经是 `: AnyObject, Sendable`（class-only），`===` 直接可用；不能用 `==`（protocol 类型不能 Equatable，且 instance 同身份不一定字段同等）。

```swift
if let newClient = webSocketClient {
    if let oldClient = self.webSocketClient, oldClient === newClient {
        // 同一 instance：no-op
    } else {
        if let oldClient = self.webSocketClient {
            oldClient.disconnect()
            self.messageConsumerTask?.cancel()
            self.messageConsumerTask = nil
        }
        self.webSocketClient = newClient
    }
}
```

回归测试两条：
- `testBindWithDifferentClientDisconnectsOldClient`：rebind 传不同 client → 断言旧 client `didDisconnect == true` + 旧 stream emit 不再被路由到 vm + 新 stream emit 能被路由
- `testBindWithSameClientInstanceIsNoop`：rebind 传同一 instance → 旧 client 不应误调 disconnect

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写**注入入口**（`bind` / `inject` / `setX`）替换持有的引用类型资源时，**必须**先 identity 比较旧 vs 新，不同则 tear down 旧资源（disconnect / cancel task / unsubscribe / dispose）再 swap，同 instance 则 no-op。
>
> **展开**：
> - 注入入口的"完整心智模型"必须三分支：① 同 instance no-op / ② 不同 instance 替换（先 tear down 旧 + swap）/ ③ 旧为 nil 首次注入（仅 swap）。**漏第②条 = 资源泄漏 + duplicate event delivery**
> - identity 比较用 `===`（要求 protocol 加 `: AnyObject` 约束）。protocol 不强制 AnyObject 时无法 `===`，会逼出"用 ObjectIdentifier 包一层"或"加 AnyObject 约束"的二选一 —— 选后者（约束更紧反而更安全）
> - tear down 列表 = 该资源**所有衍生的生命周期挂钩**：socket disconnect / cancel 持有它的 Task / 取消 Combine subscription / 释放 file descriptor / 等等。漏一项就是部分泄漏
> - **反例**：`self.client = newClient; startConsumingTask()` 直接 swap 不管旧的 —— 旧 client 资源 / 旧 task 都活着
> - **反例**：用 `==` 而非 `===` 比较 —— protocol 多数没有 Equatable，编译错；class instance 同身份不一定字段同等（自定义 `==` 还可能假阳性 / 假阴性）
> - **反例**：「同 instance 重 bind 也调一遍 disconnect」—— 把好 client 关掉，连带它正在用的 stream consumer 一起死

---

## Meta: 注入入口的"完整心智模型"补全

本次是 Story 12.1 第 5 轮 review。前 4 轮已经把 ViewModel 内部状态机各种边界打磨一遍（A→B 切换、stale snapshot、isHost 推断、空 roomId 对齐 dispatcher），但**注入入口本身**的语义在 round 5 才被审到 —— 暴露了一个普适规律：

> **bind/inject/set 类型的"接缝点"代码**容易只考虑「正常路径写新值」，**漏掉「旧值的生命周期管理」**。
>
> 这类接缝点往往是 protocol-driven DI 接缝（让测试可注入 mock、让 production 路径与 test 路径分开），其语义边界**最容易被低估**：caller 写「你接受 X 作为依赖」很自然，但**「我之前持有过另一个 X，现在你来取代它」是个独立的、容易漏的状态转换**。

未来 Claude 评审 / 写注入入口时，**显式把"被取代的旧资源去哪了"写进函数文档**（不写就等于没考虑），评审时优先 grep 「self.X = ...」前面有没有对应的 tear down。这条比"补 disconnect"本身更重要。
