---
date: 2026-05-11
source_review: /tmp/epic-loop-review-12-7-r2.md (codex review, Story 12-7 round 2)
story: 12-7-创建-加入-退出-use-case-主界面入口完善
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-11 — UITEST 启动路径必须保留 UseCase nil-fallback + LeaveRoom 必须 guard target==current 防 stale-response 抹掉新房间

## 背景

Story 12.7（创建/加入/退出 UseCase + 主界面入口完善）落地后,
RootView 在 `.onAppear` 内**无条件**注入 real CreateRoom / JoinRoom / LeaveRoom UseCase
到 RealHomeViewModel / RealRoomViewModel / RealFriendsViewModel.

codex review round 2 命中两条问题:

1. **[P1]** `UITEST_SKIP_GUEST_LOGIN=1` 路径（无 token + 无 backend）下 forcing real
   JoinRoomUseCase → HTTP 失败 → 既有依赖 `RealHomeViewModel.onJoinRoomConfirm` nil-fallback
   到 `setCurrentRoomId(roomId)` 本地的 `HomeUITests.testJoinRoomModalCrossScreenJoinFlow`
   持续 broken.
2. **[P2]** `LeaveRoomUseCase.execute` await 返回后**无条件**清 `currentRoomId`,
   即便用户已切到新房间. `6004` 兼用 "已离开" 和 "current_room_id != path roomId"（V1 §10.5
   三种 race 场景），任意一种迟到的 6004 都可能 wipe 后续 join 的新房间 + disconnect fresh session.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | RootView 在 UITEST 路径强制注入 real JoinRoomUseCase 让 nil-fallback 失效 | high | testing | fix | `iphone/PetApp/App/RootView.swift:243-278` |
| 2 | LeaveRoomUseCase await 后无条件清 currentRoomId 不防 stale-response race | medium | architecture | fix | `iphone/PetApp/Features/Room/UseCases/LeaveRoomUseCase.swift:41-90` |

## Lesson 1: UITEST 启动路径必须保留 UseCase nil-fallback —— 不能强制注入 real HTTP UseCase

- **Severity**: high
- **Category**: testing
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift:243-278`

### 症状（Symptom）

`UITEST_SKIP_GUEST_LOGIN=1` 启动路径下，用户在 HomeView 点 "加入队伍" → modal 输入房间号 → 点
"确定加入" 之后 `RoomScaffoldView` 永远不出现 —— `RealHomeViewModel.onJoinRoomConfirm` 拿到
real `JoinRoomUseCase`，调真实 HTTP `/rooms/{id}/join`，无 token / 无 backend → 401 / network
error → fallback 路径走不到，`appState.setCurrentRoomId(roomId)` 永远不写 → HomeContainerView
互斥状态机不切到 inRoom 分支. `testJoinRoomModalCrossScreenJoinFlow` UITest 卡在
"RoomScaffoldView 未在 join confirm 后渲染" 断言.

### 根因（Root cause）

新 wire 设计假设 "ViewModel 拿到 real UseCase 就走 server 路径，没拿到走 fallback 兜底" 是
**生产场景** 的合理默认；但忘了 UITest 启动路径同样会经过 RootView `.onAppear` 注入流程 ——
**UITest 路径 RootView 既没有真 token 也没 backend**，强行注入 real UseCase 等于强制让 UITest
跑真实 HTTP，必定失败. ViewModel 内既有的 nil-fallback 设计本来就是为此场景（"无 backend
模式下也能切 UI"）准备的，但 caller 不知道 / 没经过 caller 这层 gate.

**思维漏洞**：把"注入真实依赖"和"启动环境"耦合在一起，没意识到 UITest 路径需要选择性退化到
nil-fallback. UseCase nil-fallback 不仅是 "Story 落地前 / Preview 时" 的临时兜底，更是
**测试 / 离线模式的 first-class 设计**.

### 修复（Fix）

`RootView.onAppear` 内在注入 real UseCase 前 gate `UITEST_SKIP_GUEST_LOGIN` env flag:

```swift
let isUITestSkipGuestLogin: Bool = {
    #if DEBUG
    return ProcessInfo.processInfo.environment["UITEST_SKIP_GUEST_LOGIN"] == "1"
    #else
    return false
    #endif
}()
if let realHomeVM = homeViewModel as? RealHomeViewModel, !isUITestSkipGuestLogin {
    realHomeVM.bind(
        createRoomUseCase: container.makeCreateRoomUseCase(appState: appState),
        joinRoomUseCase: container.makeJoinRoomUseCase(appState: appState),
        errorPresenter: container.errorPresenter
    )
}
```

同精神应用到 `RealRoomViewModel.bind(leaveRoomUseCase: ...)` 和
`RealFriendsViewModel.bind(joinRoomUseCase: ...)`：UITEST 路径下传 nil 让 onLeaveTap /
onJoinFriendTap 走老 fallback 直写 appState. webSocketClient 注入保持原路径不动（无 token 时
WS connect fail-fast，不影响 UI scaffold 验证；err 会经 ErrorPresenter 覆盖式 overlay,
但 RoomScaffoldView 在底层已渲染，XCUITest 通过 a11y tree 仍可定位 returnButton anchor）.

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **RootView 类入口 `.onAppear` / `.task` 内注入 production HTTP UseCase
> 之前**，**必须** **gate `ProcessInfo.processInfo.environment["UITEST_*"]` env flag, UITEST 路径下传
> nil 保留 ViewModel 既有 nil-fallback 路径**.
>
> **展开**：
> - 任何 UseCase 注入入口 `bind(xxxUseCase:)` 设计时必须配套支持 `nil` 走 fallback（写 appState /
>   占位 mock 行为），不能假设 caller 一定注入. ViewModel 内 `guard let useCase else { fallback }`
>   分支是 first-class 设计.
> - UITest 启动路径（`UITEST_SKIP_GUEST_LOGIN=1` / `UITEST_FORCE_IN_ROOM=1` 等）属于"无 backend
>   离线模式"，与 production / DEBUG no-test 模式属于不同 wire profile —— **不能复用同一注入入口**.
> - 注入入口加 gate 时优先 `#if DEBUG` 包裹 env 读取，让 Release build **不**带 UITest 代码路径
>   （编译期剪枝），避免 Release 用户万一带上 UITEST_xxx env 触发非预期分支.
> - 同 RootView 内多处 UseCase 注入（Home / Room / Friends 等）必须**同步** apply gate —— 否则一处漏
>   单条 UITest case 仍会 broken; codex 本次只命中 JoinRoom 是因为只有 testJoinRoomModalCrossScreenJoinFlow
>   覆盖到这条 path，其他 UseCase 漏 gate 等下一个 UITest case 写进来才会暴露.
> - **反例**：
>   ```swift
>   // BAD: 无条件注入 real UseCase
>   .onAppear {
>       realHomeVM.bind(joinRoomUseCase: container.makeJoinRoomUseCase(appState: appState), ...)
>   }
>   ```
>   UITest 路径（无 token + 无 backend）下 join HTTP 调用必失败 → fallback 走不到 →
>   依赖 fallback 的 UITest case persistently broken.

## Lesson 2: 异步 Use Case "完成后清状态" 必须 guard `target == current` 防 stale-response 抹掉新选择

- **Severity**: medium
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/UseCases/LeaveRoomUseCase.swift:41-90`

### 症状（Symptom）

用户场景：在房间 A → 点 "离开房间" 触发 `leaveRoom(A)` → 不等响应直接点 "加入"
切到房间 B（写 `appState.currentRoomId = "B"`）→ leave 的 HTTP 200 / 6004 响应迟到一些 →
旧实装无条件 `setCurrentRoomId(nil)` → 用户在房间 B 内 UI 被 wipe 回 idle，fresh WS session
被 disconnect.

V1 §10.5 钦定 `6004` 兼用 "已离开" + "current_room_id != path roomId" + "DELETE RowsAffected==0"
三种 race 场景，**任意一种** 迟到的 6004 都可能 trigger 这条 wipe.

### 根因（Root cause）

异步 use case execute 入口读取 `appState.currentRoomId` 作为 leave 目标，但 await 返回后**没**
重新对比当前 `appState.currentRoomId` 与入口记录的 target；以为 "leave 成功 = 清当前房间"
等价于 "leave 成功 = 清入口时刻的房间"，忽略了用户在 await 期间可能已切到新房间.

**思维漏洞**：把 "操作完成后写回 state" 看成原子动作，忘了 `await` 给状态变更腾出了时间窗,
async use case **入口的 target** 与 **返回时的 live state** 必须显式对齐.

### 修复（Fix）

execute 入口 capture `targetRoomId = roomId`；await 返回后 guard `appState.currentRoomId == targetRoomId`
才清；不匹配时 silent skip + log debug：

```swift
let targetRoomId = roomId

do {
    _ = try await roomRepository.leaveRoom(roomId: roomId)
    await MainActor.run {
        let liveRoomId = appState.currentRoomId
        guard liveRoomId == targetRoomId else {
            os_log(.info, "LeaveRoomUseCase: stale leave HTTP-200 response (target=%{public}@, current=%{public}@); skip setCurrentRoomId(nil) to keep newer room selection", targetRoomId, liveRoomId ?? "nil")
            return
        }
        appState.setCurrentRoomId(nil)
    }
} catch let APIError.business(code, _, _) where code == 6004 {
    await MainActor.run {
        let liveRoomId = appState.currentRoomId
        guard liveRoomId == targetRoomId else {
            os_log(.info, "LeaveRoomUseCase: stale leave 6004 response (target=%{public}@, current=%{public}@); skip setCurrentRoomId(nil) to keep newer room selection", targetRoomId, liveRoomId ?? "nil")
            return
        }
        appState.setCurrentRoomId(nil)
    }
}
```

200 路径 + 6004 路径都加 guard；其他 throw 路径（1009 / network / unauthorized）保留原 throw 行为
（不动 appState，让用户在 in-room UI 重试）—— 它们本就不会 wipe `currentRoomId`，不受 race 影响.

回归测试覆盖（`LeaveRoomUseCaseTests.swift` 新增 case#6 / case#7）：用
`MockRoomRepository.leaveRoomBeforeReturn` hook 在 `leaveRoom` await 期间把
`appState.currentRoomId` 改成 "5005"，断言 leave HTTP 200 / 6004 返回后 `currentRoomId` 仍是 "5005"
（不被 wipe）.

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写 **async use case** 操作 **共享可变状态**（如 AppState
> `currentRoomId` / `currentUserId` 等）时，**必须** **在入口 capture 操作 target，await 返回后
> guard `live state == target` 才写回；不匹配 silent skip + log，不抛错**.
>
> **展开**：
> - 任何形如 "读 state → 调 API（await）→ 根据结果写回 state" 的 use case 都存在 stale-response
>   wipe 风险. 入口 capture target + 返回后 guard 是标配防御.
> - 200 / 业务错误（含 idempotent "已完成" 错误码如 6004）/ network 错误三类返回的语义不同,
>   但凡返回路径会写回 state 的，**全部**都要 guard；只 200 加 guard 漏 6004 race 仍会出问题.
> - 不匹配场景 = 用户在 await 期间已切到新 target 或主动 cancel；silent skip 不抛错是因为外层视角
>   操作早已完成 / 外层视角已切到下一个 target，强行抛错或弹 alert 反而破坏用户体验.
> - **反例**：
>   ```swift
>   // BAD: await 后无条件清状态
>   let roomId = await MainActor.run { appState.currentRoomId }!
>   _ = try await repo.leaveRoom(roomId: roomId)
>   await MainActor.run { appState.setCurrentRoomId(nil) }   // ← stale response wipes newer room
>   ```
>   用户 A→B 中切途中 leave(A) 的迟到响应会 wipe B；fresh WS session 被 disconnect.

## Meta: 本次 review 的宏观教训

两条 finding 表面无关，深层共享同一思维漏洞：**忽略 async 边界 / env 边界的语义切换**.

- Lesson 1 是 "env 边界" 切换被忽略：production / UITest 两种启动 profile 共用同一注入入口,
  没意识到 UITest 缺少 backend 这一前提让 production wire 注入是 broken.
- Lesson 2 是 "async 边界" 切换被忽略：use case 入口读 state 与 await 返回后写 state 之间存在
  并发窗口，没意识到 user 可以在窗口里改 state.

未来设计 / review 时，**任何"读 → 操作 → 写"的代码路径都要先问两个问题**：
1. **这段代码可能跑在哪些 env / launch profile 下？**（production / DEBUG / UITest）每种 profile
   的依赖前提是什么？
2. **读和写之间有没有 await / suspend point？**有的话读到的"target"在写时是否还有效？
