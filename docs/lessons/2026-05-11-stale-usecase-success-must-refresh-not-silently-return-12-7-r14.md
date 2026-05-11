---
date: 2026-05-11
source_review: codex review (epic-loop r14) — /tmp/epic-loop-review-12-7-r14.md
story: 12-7-创建-加入-退出-use-case-主界面入口完善
commit: facec5d
lesson_count: 1
---

# Review Lessons — 2026-05-11 — UseCase stale-success path 必须显式失败（throw error 触发 refresh），不能 silent return 让 client/server desync（12-7 r14）

## 背景

Story 12.7 r14 codex review。r10 / r12 累计架构里，Create/Join/Leave Room UseCase 在 navigation
race（用户在 HTTP in-flight 时切到别处 → `appState.roomNavigationGeneration` 已 bump）会用
generation token 检测 stale → **静默 skip `setCurrentRoomId`** 但 **仍返回 success（不抛错）**.

r14 codex 找到该设计的后果：server 端已 commit 用户进 room / 离开 room，但 client UI 因 silent skip
没写 `appState.currentRoomId`，造成 client/server 短暂 desync —— 后续 create/join 会被 server 拒
（6003 already-in-room / 6001 not-in-room / membership mismatch）直到下次 `/home` 重新 hydrate。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | UseCase stale-success path silent return → client/server desync（Create/Join/Leave 三路同根） | P1 (high) | architecture | fix | `iphone/PetApp/Features/Room/UseCases/CreateRoomUseCase.swift:63-71`、`JoinRoomUseCase.swift:57-66`、`LeaveRoomUseCase.swift`（顺带统一） |

## Lesson 1: UseCase 的 stale-success path 不能 silent return — 必须 throw error 让 caller 决定 refresh

- **Severity**: P1 (high)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/UseCases/{CreateRoomUseCase,JoinRoomUseCase,LeaveRoomUseCase}.swift`

### 症状（Symptom）

`CreateRoomUseCase.execute()` 在 HTTP 200 后检测到 `appState.roomNavigationGeneration != entryGen`,
判定 navigation race 已发生，于是 skip `setCurrentRoomId` —— **但函数仍 `return roomId`**.

外层 `RealHomeViewModel.onCreateTap` 收到 success → 没有任何 alert / refresh path 触发。然而：
- server 端已经把 user 放进 room
- client `appState.currentRoomId` 仍是用户切走前的值
- 用户下一次点 create / join → server 直接给 6003 "已在房间"
- 用户视角下完全无法理解（"我没在房间啊？"）

直到下一次 `/home` 重新 hydrate（用户重启 / 用户手动 refresh）client 才看到自己当前所在的 room.

JoinRoomUseCase / LeaveRoomUseCase 同根；leave path 还有"用户已 re-join 同房间"的衍生 desync.

### 根因（Root cause）

r10 / r12 引入 generation token 时只考虑了一件事：**不要 wipe newer room selection**。
但没回答另一个问题：**stale 路径里 server 已经 commit 了的事实怎么处理？**

silent skip 处理了"不要 overwrite UI 当前态"，但漏了"client 必须知道 server state 已经变了"。

抽象出来的反思维：**"server 端已 commit 但 client 没接收"这个事实必须被显式表达给 caller**。
silent success 是"假装没事"，而正确 contract 是"这是 race，请去问 /home 拿权威态"。

### 修复（Fix）

新增 marker error type：

```swift
public struct RoomNavigationStaleError: Error, Equatable, Sendable {
    public enum Source: String, Sendable { case createRoom, joinRoom, leaveRoom }
    public let source: Source
}
```

CreateRoomUseCase / JoinRoomUseCase / LeaveRoomUseCase 的 stale path 改成 throw 它而非 silent return：

```swift
let staleSignal: Bool = await MainActor.run {
    let liveGen = appState.roomNavigationGeneration
    guard liveGen == entryGen else {
        os_log(.info, "stale ... will throw RoomNavigationStaleError so caller refreshes home")
        return true
    }
    appState.setCurrentRoomId(roomId)
    return false
}
if staleSignal {
    throw RoomNavigationStaleError(source: .createRoom)
}
return roomId
```

ViewModel 端 catch 优先级在其他 catch 块之前（`catch is RoomNavigationStaleError` 在
`catch let APIError.business...` 之前）：

```swift
} catch is RoomNavigationStaleError {
    // silent skip errorPresenter（用户视角下没出错）+ 触发 home refresh 拿 authoritative state
    refreshHome?()
} catch let APIError.business(code, _, _) where code == 6003 {
    ...
}
```

RootView 在 `.onAppear` 注入 `refreshHomeOnStaleNavigation` closure：

```swift
let refreshHomeClosure: @MainActor @Sendable () -> Void = { [weak homeViewModel] in
    homeViewModel?.resetLoadHomeForRetry()
    Task { @MainActor [weak homeViewModel] in
        await homeViewModel?.loadHome()
    }
}
realHomeVM.bind(..., refreshHomeOnStaleNavigation: refreshHomeClosure)
realRoomVM.bind(..., refreshHomeOnStaleNavigation: refreshHomeClosure)
realFriendsVM.bind(..., refreshHomeOnStaleNavigation: refreshHomeClosure)
```

UITEST 路径下 closure 注入 `nil`（与 UseCase / errorPresenter 注入同 gate）—— 让 fallback path
保持 hard no-op + 同步直写 `setCurrentRoomId`。

LeaveRoomUseCase 顺带统一（review 未点名但同根）：避免下一轮 review 找到衍生 issue。

回归测试覆盖：
- `CreateRoomUseCaseTests.testExecuteThrowsStaleErrorAndDoesNotWipeNewerRoomSelectionWhenStaleResponseArrives`
- `CreateRoomUseCaseTests.testExecuteThrowsStaleErrorAcrossABAcycleViaGeneration`
- `CreateRoomUseCaseTests.testExecuteHappyPathDoesNotThrowStaleError`
- `JoinRoomUseCaseTests.testExecuteThrowsStaleErrorAndDoesNotWipeNewerRoomSelectionWhenStaleResponseArrives`
- `JoinRoomUseCaseTests.testExecuteThrowsStaleErrorAcrossABAcycleViaGeneration`
- `JoinRoomUseCaseTests.testExecuteHappyPathDoesNotThrowStaleError`
- `LeaveRoomUseCaseTests.testExecuteHttp200ThrowsStaleErrorAndDoesNotWipeNewerRoomSelection`
- `LeaveRoomUseCaseTests.testExecuteBusiness6004ThrowsStaleErrorAndDoesNotWipeNewerRoomSelection`
- `LeaveRoomUseCaseTests.testExecuteHttp200ThrowsStaleErrorAfterRejoinSameRoomViaGeneration`
- `LeaveRoomUseCaseTests.testExecuteBusiness6004ThrowsStaleErrorAfterRejoinSameRoomViaGeneration`
- `LeaveRoomUseCaseTests.testExecuteHappyPathDoesNotThrowStaleError`
- `RealHomeViewModelTests.testOnCreateTapCaughtStaleErrorTriggersHomeRefresh`
- `RealHomeViewModelTests.testOnJoinRoomConfirmCaughtStaleErrorTriggersHomeRefresh`
- `RealHomeViewModelTests.testOnCreateTapHappyPathDoesNotTriggerHomeRefresh`
- `RealRoomViewModelTests.testOnLeaveTapCaughtStaleErrorTriggersHomeRefresh`
- `RealRoomViewModelTests.testOnLeaveTapHappyPathDoesNotTriggerHomeRefresh`

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写 UseCase 检测到"async response 抵达但本地 entry token 已过期"
> 这类 stale-success 路径** 时，**必须** 让 stale path **抛 marker error 让 caller 显式决定 refresh
> 路径**，**禁止** silent return 假装成功。

> **展开**：
>
> - "server 已 commit 但 client 没接收"必须被显式表达 —— silent skip 只解决"不要 overwrite UI"，
>   解决不了"client/server 的 state 飘了"
> - marker error 应是 lightweight struct（不带 user-facing 信息），让 ViewModel layer 自己决定
>   route：通常是 silent skip errorPresenter + 触发 authoritative refresh（重拉 /home / `/me` 等）
> - ViewModel 的 catch 顺序：先 `catch is RoomNavigationStaleError`（race 信号），再 business code
>   case-by-case，再 fallback `catch {}`。优先级反了会让 stale error 被 fallback 当成 generic error
>   弹给 user
> - 注入 refresh closure 而非协议方法 —— closure 让 RootView 持有 `homeViewModel.resetLoadHomeForRetry`
>   + `loadHome()` 两步组合，多个 ViewModel 共用同一 closure；协议方法会让 ViewModel 间循环依赖
> - **反例**：UseCase 在 stale path 写 `return existingValue`（silently skip side-effect 同时返回
>   成功）→ caller 完全感知不到 race → 后续操作必然撞 server-side validation 错误（6003 / membership
>   mismatch / 等等），用户视角下完全无法理解
> - **反例**：在 stale path 调 `appState.setCurrentRoomId(serverReturnedRoomId)`（"硬塞回去"）—
>   这与 stale guard 的初衷冲突（用户已 navigate away，强行写回会再次破坏 UI），不是正确路径
> - **反例**：让 UseCase 自己持 LoadHomeUseCase 直接调 refresh —— 增加 UseCase 依赖链、让 unit
>   test setup 变重；正确架构是 UseCase 只发"我检测到 race"信号，ViewModel 决定怎么 refresh

## Meta: 本次 review 的宏观教训

stale-guard 类的"防御性编程"必须回答两个独立问题：
1. **如何防止 stale response overwrite 当前 UI 态**（r10 / r12 已答）
2. **如何让 client 知道 server state 已经变了**（r14 才答）

两个问题不可合并。silent skip 只答第 1 个；marker error 让 caller decide 才答第 2 个。设计
async UseCase 时若引入 generation / token guard，必须同时设计 stale-success 的 refresh contract，
否则衍生 desync 一定会在后续被发现。
