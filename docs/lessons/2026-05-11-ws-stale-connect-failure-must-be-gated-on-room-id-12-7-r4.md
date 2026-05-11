---
date: 2026-05-11
source_review: codex review (epic-loop round 4) — /tmp/epic-loop-review-12-7-r4.md
story: 12-7-创建-加入-退出-use-case-主界面入口完善
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-11 — stale connect(roomId:) failure 必须 gated on lastObservedRoomId

## 背景

Story 12.7 落地"创建/加入/退出 UseCase + 主界面入口"全链路。前轮（r1 / r3）已修两条 connect-catch 路径问题（连接失败时 wsState 卡在假 `.connected` + errorPresenter 不应被 transient WS 故障触发）。本轮 codex r4 找出 r1 catch 引入的**残留 race**：用户在 `connect(roomId:)` 还 await 时切换房间 / 离开，旧 connect 因 `disconnect()` / `prepareForReconnect()` 之后 throw 出来 —— 这个 stale failure 仍会**无条件** set `wsState = .disconnected`，覆盖当前 room B 的 wsState，让 `RoomScaffoldView` 显示错误的连接状态。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | stale `connect(roomId:)` failure 覆盖当前 room wsState | P2 (medium) | architecture | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:377-381` + 三处 connect catch |

## Lesson 1: stale connect(roomId:) failure must be gated on lastObservedRoomId before mutating wsState

- **Severity**: medium (P2)
- **Category**: architecture（concurrency / state-machine race）
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`
  - bind 路径 connect catch（line 207-216 旧版本）
  - subscribeRoomIdConnect nil→A 分支 connect catch（line 313-330 旧版本）
  - subscribeRoomIdConnect A→B 分支 connect catch（line 370-384 旧版本）

### 症状（Symptom）

时序：
1. 用户进入 room_A → vm 占位 `wsState = .connected` + spawn Task `await client.connect(roomId: "room_A")`
2. connect 还在 await（network round-trip / handshake / DNS）
3. 用户切到 room_B → sink 走 A→B 分支：`disconnect()` 旧 client + `prepareForReconnect()` + spawn 新 Task `await client.connect(roomId: "room_B")` + 占位 `wsState = .connected`
4. 旧 `client.connect(roomId: "room_A")` 因 disconnect 触发 throw → 进入 catch
5. **旧实装**：catch 内**无条件** `self?.wsState = .disconnected`
6. **后果**：当前 room_B 的 wsState 被 stale room_A 的 failure 覆盖 → `RoomScaffoldView.wsStateLabel` 显示 "连接已断开" 但实际 room_B 占位 / 真实连着

同精神时序 A→nil（离开）：connect await 中用户离开 → A→nil 分支已把 wsState 设 `.disconnected` → 旧 connect throw later 把 wsState 重写为 `.disconnected`（虽然结果相同，但语义错位，且若后续重连状态机切到 `.reconnecting` 时同样会被 stale catch 覆盖到 `.disconnected`）。

### 根因（Root cause）

Swift Concurrency 下 `await` 期间的 race 容易被忽略 —— `catch` 的执行上下文已经"穿越"到未来某时刻，当时的状态机可能已切换。在 ViewModel 的 sink 内 spawn `Task { await ... }`，如果 catch 直接读写 `self.wsState` 而不验证"我是不是还在那个调用的房间"，就会用过期信号覆盖当前状态。

设计盲点：**前轮 r1 的修复焦点是"让 catch 必须执行（替代 `try?`）"**，但没有意识到"catch 必须执行 ≠ catch 必须无条件 mutate state"。前轮 r2 / r3 已经在三类消息（`room.snapshot` / `member.joined` / `member.left` / `connectionStateChanged`）上引入 `streamRoomId` / `lastObservedRoomId` 守护抵御 race，**但 connect 本身的 catch 没用同套守护** —— 思维漏洞是把"消息流的 race"和"connect 调用的 race"当成不同问题，实际上是同一类（在 await 期间发生 state transition）。

### 修复（Fix）

三处 catch 都加同样的 guard：捕获 connect 调用时的 `connectingRoomId` 作为 closure 局部变量，await 返回后比对 `self?.lastObservedRoomId == connectingRoomId`，不匹配则丢弃信号 + log debug，匹配才 set `wsState = .disconnected`。

before（A→B 分支为例）：
```swift
if !next.isEmpty {
    let client = self.webSocketClient
    Task { @MainActor [weak self, weak client] in
        guard let client else { return }
        do { try await client.connect(roomId: next) }
        catch {
            os_log(.error, "...", ...)
            self?.wsState = .disconnected  // 无条件！stale failure 也会写
        }
    }
}
```

after：
```swift
if !next.isEmpty {
    let client = self.webSocketClient
    let connectingRoomId = next   // 捕获调用时刻 roomId
    Task { @MainActor [weak self, weak client] in
        guard let client else { return }
        do { try await client.connect(roomId: connectingRoomId) }
        catch {
            guard self?.lastObservedRoomId == connectingRoomId else {
                os_log(.debug, "discard stale connect failure ...")
                return
            }
            os_log(.error, "...", ...)
            self?.wsState = .disconnected
        }
    }
}
```

回归测试两条（`RealRoomViewModelTests.swift` 末尾）：
- `testStaleConnectFailureDoesNotOverwriteCurrentRoomWsState`：A→B mid-flight，stale A failure 不覆盖 room_B 的 .connected
- `testStaleConnectFailureAfterLeaveDoesNotMutateWsState`：A→nil mid-flight，stale A failure 不再 mutate 已离开的 wsState

为了构造 mid-flight 时序，给 `WebSocketClientMock` 加 `connectShouldGate` + `releaseConnect(at:throwing:)` 接缝，让单测能让 connect 在 continuation 上 await，然后选择性 throw 来模拟 stale failure。

### 预防规则（Rule for future Claude）⚡

> **一句话**：在 ViewModel `sink` 内 spawn `Task { @MainActor [weak self] in try await client.xxx(roomId:) catch { ... } }` 时，**catch 内任何 mutate 当前 view state 的代码必须 gated on "我是不是还在那个调用关心的 room/session"**，比对方式是把调用时的 roomId（或 session-id / generation-token）**作为 closure 局部变量捕获**，await 返回后与最新 `lastObservedRoomId`（或对应 session-id 字段）对比，不匹配丢弃 + log debug。
>
> **展开**：
> - 这是 12.4 r1 `streamRoomId` 守护的同一种 race 的不同表现：消息流是 stale event，connect catch 是 stale failure，**两者必须都守护**。
> - guard 的写法：`guard self?.lastObservedRoomId == connectingRoomId else { os_log(.debug, "discard stale ..."); return }` —— 先 return 再写 state，**不**用 `if matches { mutate }` 包整段（防漏改字段）。
> - `connectingRoomId` **不能**用 `self?.lastObservedRoomId` 现读取代 —— 必须在 `Task { ... }` 之前 `let connectingRoomId = next`（或 `roomId`），让 closure 捕获那一刻的值。
> - 同精神扩到任何 `await network/IO + catch` 路径：`UseCase.execute()` / `client.send(_:)` / `client.disconnect()` 的 async 版本，凡是 catch 内 mutate ViewModel 的字段，都先问"现在的 state 还是我那时候 await 的 state 吗"。
> - **反例 1**：`catch { self?.wsState = .disconnected }` —— 无条件覆盖；review r1 的 fix 就是这个反例（修了 sync failure 路径但留了 stale race）。
> - **反例 2**：`guard self?.roomId == roomId else { return }` 用 `self?.roomId`（computed getter 读 `appState.currentRoomId`）—— 比 `lastObservedRoomId` 更脆弱（appState 可能在 sink 处理顺序之外被外部 mutate），守护源**优先用 ViewModel 字段层 sink 内 atomic 写入的字段**（如 `lastObservedRoomId`），不优先用 computed getter 透传上游字段。
> - 测试构造：mock 的 async 接缝（connect / send / disconnect）需要提供 "gate" 机制（CheckedContinuation 注册 + 外部 release）才能在单测里构造 await mid-flight 切换的真实时序，否则只能依赖 stub error 的同步 throw 不能覆盖 race。本 lesson 给 `WebSocketClientMock` 加的 `connectShouldGate` + `releaseConnect(at:throwing:)` 就是这个 pattern 的参考实装。

---

## Meta: 本轮 review 的宏观教训

本次 r4 是 r1 catch 引入的回归 —— "把 `try?` 改成 do/catch 让错误信号能被处理"是对的方向，但要把这个动作和"catch 内的 state mutation 是否还有效"分开思考。Story 12.x 全程暴露了 Swift Concurrency 下"sink 内 spawn Task" pattern 的一类长尾 race：每次 await 都是一次"穿越"，醒来时的现实可能已经变了。下一次给 ViewModel 写"sink 内 await 网络调用"时，默认假设"我醒来时的 self.state 可能已经不是我离开时的 self.state"，并在 catch / 也包括 success 路径上 都加守护。
