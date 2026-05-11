---
date: 2026-05-11
source_review: /tmp/epic-loop-review-12-7-r6.md (codex review, Story 12-7 round 6)
story: 12-7-创建-加入-退出-use-case-主界面入口完善
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-11 — CreateRoom / JoinRoom UseCase 必须 guard entry==current 防 stale-response 抹掉用户已切的新房间

## 背景

Story 12.7（创建/加入/退出 UseCase + 主界面入口完善）落地后,
`CreateRoomUseCase.execute()` 与 `JoinRoomUseCase.execute(roomId:)` 在 await HTTP
返回后**无条件** `setCurrentRoomId(...)`. 这与 `LeaveRoomUseCase` r2 [P2] 修过的
stale-response 问题同精神：迟到的 HTTP 200 会 wipe 用户在 await 期间已切的新房间.

codex review round 6（基于 r5 后续）命中两条 P1：

1. **[P1]** `CreateRoomUseCase.swift:46-47` — Create await 期间用户切 tab + join friend room B,
   create response 迟到 → 强制把 user 带回 stale 新建 room A.
2. **[P1]** `JoinRoomUseCase.swift:44-45` — 从 Home join room A in-flight 时,
   user 切 tab + join room B，A 的 HTTP 200 迟到 → 静默把 user 切回 stale room A.

两条同属一个 family（UseCase 写 `appState.currentRoomId` 必须 guard target / entry vs current）,
合并成一个 lesson + 单 commit 处理.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | CreateRoomUseCase await 后无条件 setCurrentRoomId 不防 stale-response | P1 (high) | architecture | fix | `iphone/PetApp/Features/Room/UseCases/CreateRoomUseCase.swift:40-51` |
| 2 | JoinRoomUseCase await 后无条件 setCurrentRoomId 不防 stale-response | P1 (high) | architecture | fix | `iphone/PetApp/Features/Room/UseCases/JoinRoomUseCase.swift:33-47` |

## Lesson 1: UseCase 写 `appState.currentRoomId` 必须 entry-capture + entry==current guard —— 不止 leave

- **Severity**: high
- **Category**: architecture (race / state mutation)
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/UseCases/CreateRoomUseCase.swift`、`JoinRoomUseCase.swift`

### 症状（Symptom）

复现路径 A（create）：

1. user 在 idle Home（`currentRoomId == nil`）→ 点 "创建队伍" → `createRoom()` HTTP in-flight.
2. user 切到 friend tab → join room "B" → `setCurrentRoomId("B")` 立即写 AppState（friend join 路径独立）.
3. createRoom() HTTP 200 迟到带回 newRoomId "A".
4. 旧实装无条件 `setCurrentRoomId("A")` → AppState 从 "B" 跳回 stale "A" → UI 强制切到 RoomView(A),
   user 困惑 "我刚不是 join 了 B 吗？".

复现路径 B（join from Home）：

1. user 在 idle Home → 点 "加入队伍" → modal 输入 roomId "A" → `joinRoom("A")` HTTP in-flight.
2. user 切 tab → 从 friend list join room "B" → `setCurrentRoomId("B")`.
3. join "A" HTTP 200 迟到.
4. 旧实装无条件 `setCurrentRoomId("A")` → 静默把 user 切回 stale room A.

两条都属于 **"async UseCase + 用户可继续交互" 的经典 race**: UI 不锁 / 不 cancellation,
迟到 success response 必须用 entry-snapshot guard 才能正确丢弃.

### 根因（Root cause）

`LeaveRoomUseCase` 在 r2 [P2] 已经修过完全相同模式（leave A in-flight → user join B →
leave 200 迟到 wipe "B"），引入了 `targetRoomId` capture + `liveRoomId == targetRoomId`
guard. 但当时 fix 仅覆盖 leave 路径，没有把同一 race-guard 模式扩到 create / join.

**思维漏洞**：把 race-guard 当成 "leave 特有问题"（因为 leave 写 nil 最显眼），
没意识到任何**异步**写 `appState.currentRoomId` 的 UseCase 都共享这个 race surface ——
只要 UI 不锁交互，stale success response 都可能 wipe 用户最新选择.

延伸：UseCase 层 nil-fallback 设计（"无 backend 时也能切 UI"）让 RealRoomViewModel /
RealHomeViewModel 走真实 HTTP 路径成为常态，进一步放大 race window（HTTP 比同步本地写
慢得多）.

### 修复（Fix）

两个 UseCase 都加入 entry-capture + post-await guard，与 leave r2 fix 同模式.

**CreateRoomUseCase.swift**（before / after 核心片段）：

```swift
// before
public func execute() async throws -> String {
    let response = try await roomRepository.createRoom()
    let roomId = response.room.id
    await MainActor.run {
        appState.setCurrentRoomId(roomId)
    }
    return roomId
}

// after
public func execute() async throws -> String {
    let entryRoomId: String? = await MainActor.run { appState.currentRoomId }

    let response = try await roomRepository.createRoom()
    let roomId = response.room.id
    await MainActor.run {
        let liveRoomId = appState.currentRoomId
        guard liveRoomId == entryRoomId else {
            os_log(.info,
                   "CreateRoomUseCase: stale create response (entry=%{public}@, current=%{public}@, newRoom=%{public}@); skip setCurrentRoomId to keep newer room selection",
                   entryRoomId ?? "nil", liveRoomId ?? "nil", roomId)
            return
        }
        appState.setCurrentRoomId(roomId)
    }
    return roomId
}
```

**JoinRoomUseCase.swift**：同模式 —— entry capture + 在 mismatch 防御层之后再加 guard.
（mismatch 路径不变：仍抛 `.decoding(JoinRoomMismatchError)`，不写 AppState.）

**注意**：return value 仍是真实 roomId（server 已建好 room A），让 caller 自行决定是否
使用. 这与 leave 的 nil 写法不同：create 即使 stale 也已经在 server 端占用资源，
caller 可能想知道. join 的 return 是 Void，无影响.

**回归测试**（新增 2 个 case）：

- `CreateRoomUseCaseTests.testExecuteDoesNotWipeNewerRoomSelectionWhenStaleResponseArrives`：
  entry nil → mid-await set "B" → 验证 final == "B" 且 return == "A".
- `JoinRoomUseCaseTests.testExecuteDoesNotWipeNewerRoomSelectionWhenStaleResponseArrives`：
  entry nil → mid-await set "B" → 验证 final == "B".

`MockRoomRepository` 增加 `createRoomBeforeReturn` / `joinRoomBeforeReturn` 注入 hook
（与既有 `leaveRoomBeforeReturn` 一致），让测试在 await stub return 之前 mutate AppState
模拟用户操作.

### 预防规则（Rule for future Claude） ⚡

> **一句话**：未来 Claude 实现 "async UseCase 写 `appState.currentRoomId` / 类似 single-source-of-truth
> 共享 ID 字段" 时，**必须**在 execute 入口 capture `entryFooId`，await 返回后 guard
> `appState.currentFooId == entryFooId` 才 mutate；mismatch 时静默 skip + dev-facing log.
>
> **展开**：
> - 任何 **async + 写 mutable shared state** 的 UseCase 都属于 race surface，与 sync UseCase
>   完全不同 —— sync UseCase 写值瞬间生效，没有 race window；async UseCase 写值要跨 await
>   边界，window 内 UI 可继续 mutate state.
> - 修一个 UseCase 的 race-guard 时，**主动扫描同 module 其他 async UseCase 写同一字段**.
>   r2 修了 leave 没扩 create / join 是典型漏面 —— 应该当时就同步检查全 family.
> - guard 用 `== entry`（snapshot 一致性）而非 `== nil`（绝对值）—— 后者会错误覆盖
>   "entry 非 nil 但 user 没变化" 的合法路径.
> - mismatch 路径**不抛错**：因为这不是逻辑错误，是 user-initiated race；抛错会让 caller
>   弹 alert 让 user 困惑. 用 `os_log(.info, ...)` 输出 dev-facing signal 即可.
> - **反例 1**：直接 `await foo(); appState.setX(value)` 不 capture entry —— 经典 stale
>   response wipe newer selection bug.
> - **反例 2**：只在最高频路径加 guard（如 leave），忽略 create / join / save / cancel 等
>   sibling UseCase —— review 会逐个 round 反复命中相同精神的不同 finding，浪费 sprint
>   预算. 用 grep 找 `appState\.set` + `await` 在同一 fn 内的模式，一次性扫干净.
> - **反例 3**：在测试里只覆盖 happy / error 路径，不覆盖 stale-response 路径 ——
>   `MockBeforeReturn` hook 是必需的测试基础设施，没有就 race-guard 没有 regression 保护.

## Meta: 本次 review 的宏观教训

修 race-condition 类 finding 时，应当**主动把同模块同精神的 sibling code 一并扫一遍**.
r2 修 leave 时，应该顺手 grep 所有 `appState.setCurrentRoomId` 的 async caller，
当时就发现 create / join 同精神漏洞. 让 review 通过 round 5 / round 6 才发现，
是延迟成本 —— 每一 round codex review + fix 都是几分钟到几十分钟，
而 r2 时多扫 30 秒就能省 r6 整轮.

**操作化**：fix-review 时，对于 "X 路径 race 漏洞" 类 finding，在 step 3（应用修复）
之前先 grep + 列出同精神 sibling，问 "这些是不是也要一起修". 如果是，**纳入本 commit 范围**
而不是等下一 round.
