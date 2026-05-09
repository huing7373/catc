---
date: 2026-05-09
source_review: codex review --uncommitted (Story 12.1 round 1)
story: 12-1-房间页面-swiftui-骨架
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-09 — Published 订阅 dropFirst 丢 restored state & 房间切换需 roster 重置

## 背景

Story 12.1（房间页面 SwiftUI 骨架）round 1 codex review 命中两条 P1 finding，全部位于
`iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` 的 `subscribeRoomIdConnect`
方法内 —— `appState.$currentRoomId` Published 订阅的 sink 闭包对"WS 生命周期"的处理
漏掉了两种关键场景：

1. **restored in-room session**：从 `/home` 拉到 `room.currentRoomId != nil` 时
   `AppState.applyHomeData` 会在 ViewModel 订阅前把 currentRoomId 写非 nil，订阅时同步 emit
   的"当前值"被 `.dropFirst()` 抑制，sink 永远不会进 connect 分支 → wsState 永远停 `.disconnected`
2. **直接 room A → room B 切换**：sink 的 non-nil 分支只切 wsState，没清旧 roster / 没 tear down 旧 stream，
   同 `@StateObject` ViewModel 实例下 room B 会渲染 room A 的成员，旧 stream 的 late messages
   也会污染 room B

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | dropFirst 抑制了 restored in-room 订阅时的 first emission | high | architecture | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` |
| 2 | room A → room B 直切未清 roster + 未 tear down 旧 stream | high | architecture | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` |

## Lesson 1: Published 订阅用 `.dropFirst()` 抑制订阅时同步 emit 会丢 restored state 信号

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:128-130`（修复前 `.dropFirst()` 那行）

### 症状（Symptom）

`AppState.currentRoomId` 已经是非 nil 值（如 `/home` restored in-room session 路径，
`AppState.applyHomeData(homeData)` 在 RealRoomViewModel 订阅前已写值）时构造 `RealRoomViewModel`，
sink 收到的第一条 emission 就是真实的 nil → A 转换信号 —— 这条信号被 `.dropFirst()` 当成
"订阅时的当前值同步 emit"丢弃，wsState 永远停在初始 `.disconnected`，stream consumer task
永远不起，整个 restored session 路径 WS 链路永远不连上。

### 根因（Root cause）

`@Published` 的 sink 在订阅时会**同步 emit 当前值**。开发者用 `.dropFirst()` 想避开
"订阅时如果 currentRoomId 已经是 nil，会立刻调 `webSocketClient?.disconnect()` 把刚注入的
mock client 直接 finish"的副作用。但 `.dropFirst()` 是无差别一刀切 —— 它把"订阅时当前值"
**等同于**"无意义初始信号"，忽略了"订阅时当前值已经是 transition 的目标态"这种合法语义。

正确的语义切分应该是按"前后值对"做转换识别：
- 订阅起步 + 当前 nil = `(prev=nil, new=nil)` no-op
- 订阅起步 + 当前已非 nil = `(prev=nil, new=A)` connect ← **这条 dropFirst 错误地抑制了**
- 真实转换 = `(prev=X, new=Y)` 按四种 (nil/non-nil) 组合处理

`dropFirst` 把场景一和场景二 conflate 成了"都丢弃"。

### 修复（Fix）

引入实例字段 `lastObservedRoomId: String?`（默认 nil，**不**在订阅前预设为 `appState.currentRoomId`
当前值），sink 内用 `(previous, normalizedNew)` pattern match 区分四种转换：

```swift
private func subscribeRoomIdConnect(to appState: AppState) {
    roomIdConnectSubscription = appState.$currentRoomId
        .removeDuplicates()
        .sink { [weak self] newRoomId in
            guard let self else { return }
            let previous = self.lastObservedRoomId
            let normalizedNew: String? = (newRoomId?.isEmpty == false) ? newRoomId : nil
            self.lastObservedRoomId = normalizedNew

            switch (previous, normalizedNew) {
            case (nil, nil):
                break  // 订阅起步 + 当前空房间 → no-op（替代 dropFirst 想避的副作用）
            case (nil, .some(let roomId)):
                // 订阅起步 + 当前已在房间 → connect（restored session 路径走这里）
                if self.webSocketClient != nil { self.wsState = .connected }
                self.startConsumingMessages()
            case (.some(let prev), .some(let next)):
                // A → B 真实切换 → reset + 重启 stream（见 Lesson 2）
                ...
            case (.some(let prev), nil):
                // A → nil 离开房间 → disconnect + 清空
                ...
            }
        }
}
```

回归测试：新增 `testRestoredInRoomSessionTriggersConnect` 用 `AppState.makeHydrated(currentRoomId:)`
构造，断言 wsState == .connected + stream consumer 活跃。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **订阅 `@Published` 字段且 sink 内副作用依赖"前后值对"** 时，
> **必须**用 **实例字段跟踪 lastObservedValue + sink 内做 pattern match**，**禁止**用
> `.dropFirst()` 抑制订阅时同步 emit。
>
> **展开**：
> - `.dropFirst()` 把"订阅时当前值"无差别等同于"无意义信号"，丢失"当前值已是 transition 目标态"的合法语义
> - sink 内副作用如果对"prev==nil + new==X"的处理与"prev==X + new==X 重复"不同，必须在 sink 内显式区分；不能寄希望于 `.dropFirst` / `.removeDuplicates` 之类 publisher operator 替代业务语义判断
> - `removeDuplicates()` 是合法的 publisher 层去重（"同值不重复触发"），但**不**等价于 dropFirst（"丢失订阅起步信号"）—— 二者语义不同，不能互换
> - 若担心订阅时同步 emit 触发"未拨号即 disconnect"副作用，正确做法是在 sink 内用 (prev, new) 显式 no-op 那种组合（如 `(nil, nil)` 分支 break），不是从 publisher 层抑制 emission
> - **反例**：`appState.$xxx.dropFirst().sink { ... }` 在 `xxx` 可能在订阅前就已经 hydrated 为目标值的场景下，会丢失该 hydration 的转换信号；任何代码评审看到 `.dropFirst()` 都应当问"订阅起步时当前值是否可能是关心的转换目标态？"

## Lesson 2: ViewModel 订阅 room/session id 切换时必须区分 (A→B) 与 (nil→A)，A→B 必须 tear down 旧 state + stream

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:133-145`（修复前 non-nil 分支只切 wsState）

### 症状（Symptom）

用户从 room A 直接切到 room B（中间不经"离开 → 加入"两步），同一个 `@StateObject RealRoomViewModel`
实例的 `members` / `memberPetStates` 仍是 room A 的旧 roster；旧 stream（room A 的
WebSocket 消息流）也没 tear down，room A 的 late `room.snapshot` / `member.joined` 消息
会到达后被 `applySnapshot` 写到 room B 的 ViewModel 上 → room B 渲染出 room A 的
成员 + 偶发跳变。

### 根因（Root cause）

订阅 currentRoomId 的 sink 闭包旧实装只有"non-nil → 切 wsState"和"nil → disconnect"
两个分支，把 (A → B) 和 (nil → A) 视作同一个 "non-nil" 分支 —— 但二者语义完全不同：
- nil → A：roster 本就空，新 stream 直接接收即可
- A → B：roster 是旧 room 数据，**必须显式清空**；stream 是连到 room A 的 channel，
  必须 tear down 旧、起新（节点 4 阶段没有 `connect(roomId:)` 接缝，只能 disconnect 然后
  cancel/restart consumer task；Story 12.2 落地真实 `WebSocketClientImpl.connect(...)` 后再
  调用真实重连）

把"进入房间"和"换房间"当成同一种事件是常见的 mental model 漏洞 —— SwiftUI 的同
`@StateObject` 实例跨房间存活是 ViewModel 默认 lifecycle，不会自动重置 state。

### 修复（Fix）

按 (previous, new) 4 象限分支，A→B 单独走"reset + 重启 stream"路径：

```swift
case (.some(let prev), .some(let next)):
    // A → B：disconnect 旧 stream + 清空 roster + cancel/restart consumer task
    self.webSocketClient?.disconnect()
    self.members = []
    self.memberPetStates = [:]
    if self.webSocketClient != nil { self.wsState = .connected }
    self.startConsumingMessages()  // 内部 cancel 旧 task 再启新 task
```

`startConsumingMessages` 已经做了 `messageConsumerTask?.cancel()` 内部处理。
节点 4 阶段语义：A→B 真正"拨号到新 room channel"要等 Story 12.2 落地 `connect(roomId:)`；
本 fix 落地"清空 roster + tear down 旧 stream + 重启 task"作为最小路径，避免 review
担心的"room B 渲染 room A 成员"+"late message 污染"。

回归测试：新增 `testDirectRoomToRoomSwitchResetsRosterAndStream`，断言 A→B 后
`members == []` + `mockWS.didDisconnect == true` + `wsState == .connected`。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **`@StateObject` 子树观察 ID 字段（roomId / sessionId / chatId 等）切换以驱动外部资源（WebSocket / network stream / file handle）** 时，**必须** 区分 `(prev=nil, new=X)` 进入态、`(prev=X, new=Y)` 切换态、`(prev=X, new=nil)` 离开态三种分支，**禁止** 把"进入"和"切换"合并成同一个 non-nil 分支。
>
> **展开**：
> - `@StateObject` ViewModel 实例跨"目标 ID 切换"存活是 SwiftUI 的默认 lifecycle —— ViewModel 不会自动重置 state，roster / cache / stream 全是 stale 的，必须代码层显式清空
> - 切换分支至少要做：① 清空所有依赖旧 ID 的 @Published 数据 ② tear down 旧 external resource（disconnect / cancel task / close handle）③ 起新 external resource（接 Story 12.2 后是 `connect(newId)`，节点 4 阶段是 `cancel + 重启 consumer task`）
> - 与 Lesson 1 同精神：用 `lastObservedId` 实例字段 + sink 内 4 象限 pattern match 是干净的实装方式，不要试图用 publisher operator（`.scan` / `.zip(prev)`）做这种状态机
> - **反例**：`sink { newId in if let id = newId { wsState = .connected } else { disconnect() } }` —— 漏 A→B 切换分支；用户从 room A 通过好友列表点击进 room B 时 UI 会保留 room A 的 roster，偶发收到 late message 会跳变

## Meta: 本次 review 的宏观教训

两条 finding 共同指向一个底层模式：**`@Published` 订阅的 sink 闭包**承担"驱动外部资源 lifecycle"
的职责时，必须按"prev → new"4 象限做状态机，不能用 publisher operator（dropFirst /
removeDuplicates）替代业务语义判断。Combine publisher operator 的语义是**值流层**的
（去重 / 限流 / 转换），不是**状态转换语义层**的；把状态转换语义压到 publisher
operator 上必然漏 corner case。

正确架构形状：
1. 订阅 `@Published` 用 `removeDuplicates()` 在值流层去同值噪声（合法）
2. sink 闭包内用实例字段 `lastObservedX` + pattern match 做 4 象限状态机（业务语义层）
3. 每个分支的副作用（清 state / 起 task / 切 wsState / disconnect）按状态机定义独立实现

future Claude 在写"订阅 ID 字段驱动外部资源"代码时，先画 4 象限表（nil-nil / nil-X / X-Y / X-nil），
明确每个象限的副作用集合，再 sink 实装。先画表 → 再写代码。
