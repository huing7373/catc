---
date: 2026-05-11
source_review: "codex review round 3 output: /tmp/epic-loop-review-12-7-r3.md"
story: 12-7-创建-加入-退出-use-case-主界面入口完善
commit: 4abf3a9
lesson_count: 2
---

# Review Lessons — 2026-05-11 — WS connect 失败不走 errorPresenter & UITEST 路径必须跳过 real WS / errorPresenter wiring（12-7 r3）

## 背景

Story 12.7 review round 3（codex review）针对 round 2 修复后产物再批：

- round 1 修了 `subscribeRoomIdConnect` 内 `try? await client.connect(roomId:)` 静默吞 WSError 导致 wsState 卡假 `.connected` 的问题。修复路径在 catch 内同时调 `errorPresenter.present(error)` 让用户看到失败 —— 但 `WSError` 在 `AppErrorMapper` 没特殊映射，fallback 到全屏 `.retry` overlay，`errorPresentationHost` 禁用整个 app hit-testing；transient WS 故障（server down / network flap / handshake error）变成 block 整个 UI 的 modal，而不是仅反映 room 的 disconnected/reconnecting 状态 —— **regression**。
- round 2 修了 `UITEST_SKIP_GUEST_LOGIN=1` 路径下 RealHomeViewModel / RealRoomViewModel / RealFriendsViewModel 不应注入 real UseCase；但**漏掉**了 `realRoomVM.bind(...)` 仍无条件注入 real `webSocketClient` + `errorPresenter` → UITEST_FORCE_IN_ROOM 写 currentRoomId 后 nil→A connect 抛 `WSError.tokenMissing` → modal retry overlay 弹 → UITest `RoomScaffoldView 直接渲染` 用例被遮挡。

两条互为补充：1 修生产用户体验，2 修 UITest 红线。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | WS connect failure 不应走 errorPresenter（弹全屏 retry overlay block 整个 app） | P1 | error-handling | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` |
| 2 | UITEST_SKIP_GUEST_LOGIN 路径下不应注入 real webSocketClient + errorPresenter | P1 | testing | fix | `iphone/PetApp/App/RootView.swift` |

## Lesson 1: WS connect failure 是后台 transient 状态，**禁止**走 `errorPresenter.present(error)`

- **Severity**: P1
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:313-314`（修复前），bind / nil→A / A→B 三处 catch 路径

### 症状（Symptom）

`subscribeRoomIdConnect` 内 nil→A / A→B 分支以及 `bind` first-injection / swap 分支 catch path 都调了 `presenter?.present(error)`。`WSError` 在 `AppErrorMapper` 没特殊映射 → fallback 到全屏 `.retry` overlay；`errorPresentationHost` 禁用整个 app 的 hit-testing；transient WS 故障（server down / network flap / handshake 错误 / 后台 reconnect 失败）会突然 block 整个 UI，而不是只展示 room 的 disconnected/reconnecting 状态 banner。

### 根因（Root cause）

`errorPresenter` 路径的语义被 round 1 修复滥用：

- `errorPresenter` 的设计语义是「用户主动操作触发的同步错误」（如点 onLeaveTap → LeaveRoomUseCase 抛 6004 → 用户应该看到弹窗解释）。
- WS 后台 connect / reconnect 是 transient subscription 状态，**不**是用户主动操作；它应该通过 `wsState=.disconnected/.reconnecting` 让 `RoomScaffoldView` 的 `wsStateLabel` 反映给用户 + 重连状态机后续自动尝试 reconnect（Story 12.6 已实装 ping/pong + transient close 重连）。
- round 1 的修复初衷是好的（避免 r1 之前 `try?` 静默吞错），但选错了通道 —— 应该只让 wsState 反映，**不**该再叠加 errorPresenter。

要点：每个 error path 选 channel 时要问"这是 user-initiated 错还是后台 transient 状态"。后者**永远不**走 modal overlay。

### 修复（Fix）

`RealRoomViewModel.swift` 三处 catch 内删 `presenter?.present(error)` + 删局部 `let presenter = self.errorPresenter`；仅保留 `os_log(.error, ...)` + `self?.wsState = .disconnected`：

```swift
// Before（r1 修复后，r3 review 命中）：
catch {
    os_log(.error, "RealRoomViewModel: nil→A connect failed: %@", error)
    self?.wsState = .disconnected
    presenter?.present(error)   // ← 删
}

// After：
catch {
    os_log(.error, "RealRoomViewModel: nil→A connect failed: %@", error)
    self?.wsState = .disconnected
}
```

`errorPresenter` 字段保留（onLeaveTap 仍用于 LeaveRoomUseCase 的 1009/其他业务错误 —— 那是用户主动操作的正确路径）。

新增回归测试 `testNilToAConnectFailureDoesNotInvokeErrorPresenter`：mock WS `connectError = .tokenMissing` + 注入 ErrorPresenter → 触发 nil→A → 断言 `presenter.current == nil`。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **某 ViewModel 的"后台订阅 / 重连 / 状态 stream"路径抛错** 时，**禁止**调 **`errorPresenter.present(error)`**（或任何会走 AppErrorMapper fallback 到全屏 retry overlay 的 channel）；改用 **状态字段（如 `wsState=.disconnected`）+ os_log**。
>
> **展开**：
> - `errorPresenter` 的语义是「user-initiated 同步错误」，**不是**「任何 error 的兜底通道」。在 catch 内手贱叠加 `present(error)` 是常见误用。
> - 区分 channel：
>   - User-initiated 同步错误（onLeaveTap / onCreateTap / onJoinTap 业务错） → `errorPresenter.present(error)` ✓
>   - 后台 transient 状态（WS connect / reconnect / heartbeat / 拉取轮询失败） → 状态字段 + 日志，**不走** errorPresenter ✗
> - 决策启发式：问"用户能不能通过点'重试'按钮自己 recover？"如果不能（如 socket 重连必须等状态机自动重试），就**不**该弹 retry overlay。
> - 加测试时同时断言两个事实：① wsState=.disconnected ② errorPresenter.current == nil（仅断言 wsState 不足，未来重新引入 `present(error)` 不会被捕获）。
> - **反例**：在 `try await connect()` 的 catch 内同时 `wsState = .disconnected` + `presenter?.present(error)` —— 二者意图重复（都是"告诉用户失败"），但 modal overlay 会 block 整个 app；只留状态字段。

## Lesson 2: UITEST_SKIP_GUEST_LOGIN 路径下 RealRoomViewModel.bind 必须**同时** gate webSocketClient + errorPresenter，不仅 leaveRoomUseCase

- **Severity**: P1
- **Category**: testing
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift:278-283`

### 症状（Symptom）

`UITEST_SKIP_GUEST_LOGIN=1` 启动路径下 `ensureLaunchStateMachineWired()` 仍可能 force `currentRoomId` for `UITEST_FORCE_IN_ROOM=1`。round 2 修复让 `realRoomVM.bind(..., leaveRoomUseCase: nil)` 跳过真实 LeaveRoom，但 `webSocketClient: container.webSocketClient` 与 `errorPresenter: container.errorPresenter` 仍无条件注入 → RealRoomViewModel `subscribeRoomIdConnect` nil→A 分支尝试真实 `connect(roomId:)` without token → 抛 `WSError.tokenMissing` → catch 路径调 `presenter?.present(error)`（在 Lesson 1 修复之前）→ modal retry overlay 弹出遮挡 `RoomScaffoldView` → UITest force-in-room 用例 broken。

### 根因（Root cause）

round 2 的 gate 只 cover 了 UseCase 注入（"无 token 不能调 HTTP"），漏了**同样依赖 token 的 WS 接通**：

- WebSocketClientImpl 同样需要 sessionStore.token 才能拨 `wss://.../v1/ws?token=...`；UITEST 路径下 token 永不存在 → 注入 real client 等于"放一颗定时炸弹等 setCurrentRoomId 引爆"。
- `errorPresenter` 单独注入是无用的（业务 onCreateTap / onJoinTap 也走 fallback 不需要 presenter），保留它仅给 WS connect failure 路径（Lesson 1 修复路径已删，但作为防御性 nil-out 让 vm 完全脱离 errorPresentationHost 路径，避免任何 regression）。

修复路径的同精神 = 「UITEST 路径下所有"依赖 sessionStore.token / 与 real backend 交互"的依赖**统一**传 nil，让 vm 走完整的 nil-fallback path」。

### 修复（Fix）

`RootView.swift` 内 `realRoomVM.bind(...)` 三参数同步 gate：

```swift
realRoomVM.bind(
    appState: appState,
    webSocketClient: isUITestSkipGuestLogin ? nil : container.webSocketClient,         // ← 新加 gate
    leaveRoomUseCase: isUITestSkipGuestLogin ? nil : container.makeLeaveRoomUseCase(appState: appState),
    errorPresenter: isUITestSkipGuestLogin ? nil : container.errorPresenter             // ← 新加 gate
)
```

UITest 实跑验证（`UITEST_SKIP_GUEST_LOGIN=1 + UITEST_FORCE_IN_ROOM=1`）：
- `wsStateLabel` = "已断开"（webSocketClient = nil → `subscribeRoomIdConnect` nil→A 分支 `self.webSocketClient != nil` 守护 false → wsState 保持 .disconnected）
- `roomIdDisplay` = "1234567"（UITEST_FORCE_IN_ROOM）
- `leaveButton` 可点击、`roomMember_0..3` 可见、4 个 Tab 可见 —— **无 modal overlay 遮挡**
- 无 retry / alert 元素出现在 a11y 树

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **iOS RootView 给 ViewModel.bind 注入 dependencies 时**，**所有**"依赖 sessionStore.token / 调 real backend / 持有 ErrorPresenter 引用" 的 dep **必须**统一通过 **UITEST_SKIP_GUEST_LOGIN gate**（同一个 `isUITestSkipGuestLogin` 三元）传 nil，而不是只 gate 部分。
>
> **展开**：
> - UITest 路径下 vm 必须能完全走 nil-fallback path（即 ViewModel 已实装的「mock 路径」），任何残留的 real dep 都可能在 UITest 启动后某条 path 上炸（典型路径：UITEST_FORCE_IN_ROOM 写 currentRoomId → subscribe 触发 connect → token 不存在抛错）。
> - 决策启发式：注入参数前问"这个 dep 在 UITEST 路径下的合法用途是什么？"如果答案是"无"或"会失败"，统一传 nil。
> - 加 lesson 反链：UITEST gate 一旦确立，后续 review 看到任何新参数同样要走 gate（regression 漏点 = 新加参数时忘了 gate）。
> - **反例**：`realRoomVM.bind(appState: ..., webSocketClient: container.webSocketClient, leaveRoomUseCase: isUITest ? nil : container.makeLeaveRoomUseCase(...), errorPresenter: container.errorPresenter)` —— 只 gate 中间一个参数，左右两个继续注入真实实例，等于"半 gate"，留下定时炸弹。

---

## Meta: 本次 review 的宏观教训

两条 P1 互为补充地暴露了同一个思维漏洞：**「上一轮修复时引入的新依赖关系/通道，必须重新评估它在所有 launch path（生产 + UITest + dev）下的行为」**。

- round 1 给 catch path 引入 `errorPresenter.present(error)` 时，没问"这条错走全屏 retry overlay 合不合适？" —— 假设 errorPresenter 是通用兜底，实际它有特定语义（user-initiated 同步错误）。
- round 2 给 RealHomeViewModel / RealFriendsViewModel.bind 加 UITEST gate 时，**忘了 RealRoomViewModel.bind 也是同模式**，漏 gate webSocketClient / errorPresenter。

通用规则：**修复 review 时新加一个参数 / channel / gate，必须巡视同 epic 内所有「同模式 callsite」是否同步更新**，不能只改 review 指出的具体那一行。round 2 review 显式提示「与 r2 P1 修复中的 UseCase 注入 gate 同精神」就是在抓这个 meta 漏洞 —— 未来 Claude 看到 codex review 用「同精神」「同模式」这类措辞，必须主动去 cross-check 所有 callsite。
