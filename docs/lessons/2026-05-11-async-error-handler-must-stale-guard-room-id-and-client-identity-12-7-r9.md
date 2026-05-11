---
date: 2026-05-11
source_review: /tmp/epic-loop-review-12-7-r9.md (codex review, Story 12-7 round 9)
story: 12-7-创建-加入-退出-use-case-主界面入口完善
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-11 — 异步 error catch 必须做 stale-guard：roomId 以及 client identity 两层都要看

## 背景

Story 12.7 落地 CreateRoom/JoinRoom/LeaveRoom UseCase 后,
ViewModel 层的 success 路径已经在 r6 P1 fix（lesson `2026-05-11-create-join-room-guard-target-vs-current-against-stale-response-12-7-r6.md`）和
r4 P2 fix（lesson `2026-05-11-ws-stale-connect-failure-must-be-gated-on-room-id-12-7-r4.md`）覆盖了 stale-guard。
但 codex r9 review 命中 3 条 **error catch 路径** 同精神漏面：

1. **[P2]** `RealHomeViewModel.onCreateTap` / `onJoinRoomConfirm` 的 catch 块 **无条件** present alert/error
   —— 没有 entry==current guard。用户在 create / join HTTP in-flight 中切换房间，迟到的失败会把 stale
   error overlay 弹在新房间之上。

2. **[P2]** `RealFriendsViewModel.onJoinFriendTap` 的 catch 块同样 **无条件** present。
   同精神：friend join in-flight 中用户切到 room B → A 失败迟到 → alert/retry 弹到 room B。

3. **[P2]** `RealRoomViewModel` 三处 `connect(roomId:)` 调用的 catch 块只比对 `lastObservedRoomId`,
   不校验 **client instance**. `bind` 路径明确支持 swap webSocketClient instance（同 roomId 下注入新 client）—
   旧 client 的 in-flight connect 会因 swap-path 触发 `disconnect()` 而 throw later。此 throw
   仍通过 roomId 校验，老实装会把 wsState 错误 flip 回 `.disconnected`，即使新 client 已成功 connect。

这是 3 条 P2 **同一个精神 family**：异步 error handler 也要 stale-guard，而不仅是 success 路径。
合并成 1 个 lesson + 单 commit 处理。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | RealHomeViewModel onCreateTap/onJoinRoomConfirm catch 内 ErrorPresenter 不 guard stale | P2 (medium) | error-handling | fix | `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift:156-172` + `:229-268` |
| 2 | RealFriendsViewModel onJoinFriendTap catch 内 ErrorPresenter 不 guard stale | P2 (medium) | error-handling | fix | `iphone/PetApp/Features/Friends/ViewModels/RealFriendsViewModel.swift:147-183` |
| 3 | RealRoomViewModel connect failure stale check 缺 client identity check | P2 (medium) | error-handling | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:203-235` + nil→A + A→B 3 callsites |

## Lesson 1: 异步 error catch 必须和 success 路径同等做 stale-guard；race 维度不止 roomId 还包括 collaborator identity

- **Severity**: medium
- **Category**: error-handling (async race)
- **分诊**: fix
- **位置**: Home / Friends / Room ViewModel 三处异步 catch

### 症状（Symptom）

**场景 1（Home 创建）**：

1. user 在 idle Home → 点 "创建队伍" → `CreateRoomUseCase.execute()` HTTP in-flight。
2. user 切到 friend tab → 加入 room "B"（`setCurrentRoomId("B")` 立即生效）。
3. createRoom() HTTP 抛 6003 / 1009 / network error 迟到。
4. 老实装 catch 内无条件 `presenter?.presentAlert(...)` / `presenter?.present(error)` →
   stale retry/alert overlay 弹在 room B 之上，干扰用户在新房间的交互。

**场景 2（Friends 加入）**：

1. user 在 friends tab 点 join friend A 的 room "A" → `joinRoom("A")` HTTP in-flight。
2. user 切到 friend list 其他入口 / Home → join room "B" → `setCurrentRoomId("B")`。
3. join("A") HTTP 抛 6002 / 1009 等错误迟到。
4. 老实装无条件 present → alert/retry 弹到 room B。

**场景 3（WS connect 同 roomId 不同 client）**：

1. RealRoomViewModel 已 bind 了 clientA + 进入 room_A → `clientA.connect("room_A")` await 中（卡 gate / 慢网）。
2. 外部代码（test / future token refresh path）调 `vm.bind(webSocketClient: clientB)` 传入新 client（同 roomId）。
3. bind 路径走 swap 分支：disconnect(clientA) + cancel 旧 task + swap + prepareForReconnect(clientB)
   + 起新 connect(clientB)。
4. clientB.connect("room_A") 成功 → wsState = .connected。
5. clientA 的旧 connect 因 disconnect() 而 throw later。
6. 老实装 catch 只 guard `lastObservedRoomId == connectingRoomId`（"room_A" == "room_A" ✓ match）→
   `wsState = .disconnected` → 错误覆盖 clientB 已 set 的 `.connected`。

### 根因（Root cause）

**思维漏洞 1**：把 race-guard 当成"success 路径专属问题"。修 r6（success setCurrentRoomId）+
r4（connect success）时只覆盖正向路径，没意识到 **catch 路径也写 shared state**
（`presenter.present` 写全屏 retry overlay，`wsState = .disconnected` 写公开字段）—— 这些都是
"async + write shared state" 的 race surface。

**思维漏洞 2**：race-guard 维度局限在 ID-level（roomId）。当 bind 路径明确支持 swap collaborator
instance（webSocketClient 是 class-only protocol, 可 `===` 比较）时，**collaborator identity**
也是 race 维度的一部分。仅 roomId guard 在"同 roomId 换 client"路径下漏抓 stale。

延伸：fix-review 已经是 r9，r2 / r4 / r5 / r6 / r7 / r8 反复命中 stale-guard 不同精神的同 family。
每一 round 都是新发现一个被遗漏的 callsite —— 说明分诊时缺少"family scan"步骤。

### 修复（Fix）

**Fix #1 + #2（Home / Friends async catch）**：

```swift
// before
Task { @MainActor [weak self] in
    do {
        try await useCase.execute(...)
    } catch {
        presenter?.present(error)   // 无条件 present
    }
}

// after
let entryRoomId = self.appState?.currentRoomId  // entry-capture
Task { @MainActor [weak self] in
    guard let self else { return }
    do {
        try await useCase.execute(...)
    } catch {
        let liveRoomId = self.appState?.currentRoomId
        guard liveRoomId == entryRoomId else {
            os_log(.debug, "stale error (entry=%@, current=%@); skip presenter", ...)
            return
        }
        presenter?.present(error)
    }
}
```

应用到 4 个 catch 位置：
- `RealHomeViewModel.onCreateTap` 内 6003 branch + default branch
- `RealHomeViewModel.onJoinRoomConfirm` 内整个 catch
- `RealFriendsViewModel.onJoinFriendTap` 内整个 catch

**Fix #3（Room WS connect identity check）**：3 个 callsite（bind first-injection/swap、nil→A、A→B）
catch 内**额外**加 `webSocketClient === connectingClient` 校验：

```swift
let connectingRoomId = roomId
let connectingClient = client   // capture client instance
Task { @MainActor [weak self, weak client] in
    do {
        try await client.connect(roomId: connectingRoomId)
    } catch {
        guard self?.lastObservedRoomId == connectingRoomId else { return }  // r4 已有
        // r9 新增：
        guard self?.webSocketClient === connectingClient else {
            os_log(.debug, "discard stale connect failure from replaced client")
            return
        }
        self?.wsState = .disconnected
    }
}
```

**回归测试**（4 新增 case）：

1. `RealHomeViewModelTests.testOnCreateTapStaleErrorAfterRoomSwitchSkipsPresenter`：
   mock execute mid-await 切 currentRoomId → "B" + 抛 6003 → 断言 `presenter.current == nil`。
2. `RealHomeViewModelTests.testOnJoinRoomConfirmStaleErrorAfterRoomSwitchSkipsPresenter`：同精神。
3. `RealFriendsViewModelTests.testOnJoinFriendTapStaleErrorAfterRoomSwitchSkipsPresenter`：同精神。
4. `RealRoomViewModelTests.testStaleConnectFailureFromReplacedClientDoesNotOverwriteWsState`：
   clientA gate 卡 connect → bind(clientB) swap → 释放 clientA throwing → 断言 `wsState == .connected`。

测试基础设施：
- `MockCreateRoomUseCase` 新增 `onExecute: (@Sendable () async -> Void)?` hook。
- `MockJoinRoomUseCase` / `MockJoinRoomUseCaseFriends` 新增 `onExecuteAsync` hook（async 版本让 stale 副作用确定性 await 完成再让 stub throw）。

### 预防规则（Rule for future Claude） ⚡

> **一句话**：未来 Claude 在 async ViewModel 方法的 **catch 块** 写 shared state（present alert/error,
> 改 @Published 字段等）时，**必须**和 success 路径用同一套 stale-guard 维度（entry-capture +
> guard `entry == current`）；race 维度包括 **ID-level（roomId, userId, ...）+ collaborator
> identity（class instance `===`）** 两层。
>
> **展开**：
> - **catch 路径不是 race-free**：它同样在 `await ... } catch { ... }` 之内，跨 await 边界，UI 间
>   可以 mutate state. 任何在 catch 内 `present(error)` / `wsState = .X` / `appState.setY(...)`
>   都是 race surface，必须同 success 路径一样 guard.
> - **race 维度不止 ID**：当 ViewModel 持有 collaborator object（websocketClient / repository /
>   subscriber 等 class-only protocol）且支持运行时 swap 时，**collaborator identity** 也是 race
>   维度. 用 `===` 校验 `self.foo === capturedFoo`. 仅 ID-level guard 在 "ID 不变 swap collaborator"
>   路径漏抓.
> - **修一处 stale-guard 时主动扫 family**：grep 同 module 所有 `Task { ... do { try await ... }
>   catch { presenter?.present | wsState = | setCurrentRoomId | ...` 模式，列出 callsite，一次性
>   补齐. r2 修 leave / r4 修 connect / r6 修 create+join success / r9 修 catch + identity——
>   这些都是同精神反复命中. 当时多扫 1 分钟可省一 round.
> - **mismatch 路径不抛错、不弹 UI**：用 `os_log(.debug, ...)` 输出 dev-facing 信号；
>   抛错或弹 alert 会让 user 困惑（这不是 user-facing 错误，是 user-initiated race 的副产品）.
> - **测试基础设施需求**：mock UseCase / mock client 必须暴露 **async hook**（`onExecuteAsync` /
>   `connectShouldGate` 等），让单测在 stub.get 抛错 / await 返回**之前**确定性地 mutate
>   shared state（模拟 mid-await 用户操作）. fire-and-forget Task hook 不够 ——
>   stub throw 与 Task 副作用之间有 race window 让回归测试 flaky.
> - **反例 1**：`do { try await useCase.execute() } catch { presenter?.present(error) }` —— 经典
>   stale-error-overlay-on-new-room bug.
> - **反例 2**：guard 只看 ID 不看 collaborator identity —— "同 ID swap client" 路径漏抓.
> - **反例 3**：只在主路径加 guard，"次要"路径（6003 special-case branch / fallback alert
>   branch / unknown business code branch）没 guard —— 同 catch 内不同 sub-branch 都需 guard,
>   不能漏一个.
> - **反例 4**：测试 mock 用 fire-and-forget `Task { @MainActor in setCurrentRoomId("B") }` 模拟
>   mid-await 副作用 —— 副作用与 stub throw 之间 race，测试 flaky；用 `async` hook + `await
>   MainActor.run { ... }` 让顺序确定.

## Meta: 本次 review 的宏观教训

这是 Story 12.7 review 第 9 轮，前 8 轮已经分别覆盖：r1 URL escape / WS 同步抛错；r2 UITEST fallback +
leave stale-response；r3 WS connect 不走 errorPresenter；r4 WS connect stale-failure roomId-guard；
r5 leave-thrown-error stale-guard + create nil fallback；r6 create/join UseCase success
stale-guard；r7 percent-encode pre-encoded；r8 business error fallback forward。

r9 命中的 3 条 P2 都是 **"前几轮 stale-guard 修过的同精神，但当时没扫齐 family"**：
- r4 修 connect success → 该 round 应该一并扫"catch path 也写 shared state 吗" → 当时就发现
  identity check 缺口。
- r6 修 success setCurrentRoomId stale → 该 round 应该一并扫"catch path present alert/error 也
  是 race surface 吗" → 当时就发现 Home/Friends 三处缺口。

**操作化**：fix-review skill 在 step 3（apply fix）前应增加 **family-scan 步骤**：
"对每条 finding，grep 同 module 同精神 sibling pattern；列出 ≥3 个 callsite 时进入 batch fix 模式，
一次性覆盖 family，避免后续 round 浪费 sprint budget"。

这次合并成单 commit 是正确做法（3 条 P2 同 family，单 lesson），但理想是 r4/r6 round 就应该一起修完。
