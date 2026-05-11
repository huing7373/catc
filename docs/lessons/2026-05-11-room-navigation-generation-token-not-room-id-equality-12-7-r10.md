---
date: 2026-05-11
source_review: codex review /tmp/epic-loop-review-12-7-r10.md (Story 12.7 round 10)
story: 12-7-创建-加入-退出-use-case-主界面入口完善
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-11 — Room navigation 用 generation token 而非 currentRoomId equality（ABA race 根治）

## 背景

Story 12.7 落地了 CreateRoom / JoinRoom / LeaveRoom 三个 UseCase + Home/Friends/Room ViewModel
catch-path stale-guard. r5/r6/r9 历轮 fix 都用 `currentRoomId equality`（entry 时刻 capture vs await
返回时 read）做 stale-response 判断. 本轮 round 10 codex review 找到 3 条 P2，根因相同 ——
**`currentRoomId` 值无法区分新旧 navigation cycle，ABA cycle 下旧实装通通失守**.

epic-loop 钦定 r10 是最后一轮（r1-r10）；本轮要求 root cause 修复而非补丁式，避免 r11 再爆同根
问题导致 HALT.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | CreateRoomUseCase stale-guard ABA hole | P2 | architecture | fix | `iphone/PetApp/Features/Room/UseCases/CreateRoomUseCase.swift:58-66` |
| 2 | JoinRoomUseCase stale-guard ABA hole | P2 | architecture | fix | `iphone/PetApp/Features/Room/UseCases/JoinRoomUseCase.swift:57-65` |
| 3 | LeaveRoomUseCase 不能区分原 A vs re-join A | P2 | architecture | fix | `iphone/PetApp/Features/Room/UseCases/LeaveRoomUseCase.swift:62-70` |

## Lesson 1: `currentRoomId == entryRoomId` 不是 race-safe invariant；用 monotonic generation token

- **Severity**: P2 (3 条同根 finding)
- **Category**: architecture
- **分诊**: fix
- **位置**: `AppState.swift` + 3 UseCase + 3 ViewModel catch-path

### 症状（Symptom）

3 个独立 ABA race 场景，全部由 `currentRoomId == entryRoomId` 判断失守：

**Create**: idle Home (currentRoomId=nil) → 点 Create → createRoom() in-flight 期间 user join B → leave B
回 idle (currentRoomId=nil) → create response 迟到 → `liveRoomId == nil == entryRoomId` → guard 通过
→ user 被强制切到 stale 房间 A.

**Join**: idle Home (currentRoomId=nil) → join A → in-flight 期间 user 短暂进入 B → leave B 回 idle →
join A response 迟到 → 同上 → 切到 stale room A.

**Leave**: in room A → 点 Leave → leaveRoom in-flight 期间 user 立即 re-join A（currentRoomId 经历
A → nil → A）→ leave A response 迟到 → `liveRoomId == "A" == targetRoomId` → guard 通过 →
setCurrentRoomId(nil) → 用户从刚 rejoin 的 A 被踢出.

### 根因（Root cause）

把"navigation lifecycle 是否变化"的 invariant 错误地映射到"`currentRoomId` 的值是否变化". 这只在
"navigation cycle 都至少改变了 currentRoomId 最终值" 的窄假设下成立 —— ABA cycle 就违反这个假设：
最终值回到原值，但**期间发生过完整的 navigation 切换**.

正确的 invariant 应是：**"自 capture entry token 以来，是否发生过任何 navigation cycle"**.
任何被 `currentRoomId` 值表达不出来的状态机变化（如 A → nil → A）都需要更强的不变量.

工程化解决方案 = **monotonic generation token**：
- AppState 暴露 `roomNavigationGeneration: Int`（private(set), 非 @Published）
- `setCurrentRoomId(_:)` 每次调用 `+= 1`（即使 nil → nil / A → A 也 +1）
- `applyHomeData` / `reset` 也 bump（hydrate 和 logout 都算 navigation cycle）
- UseCase / ViewModel async 入口 capture `entryGen`，await 后 guard `entryGen == currentGen`
- 严格单调递增的整数对 ABA cycle 免疫（每次切换必然破坏 equality）

### 修复（Fix）

**AppState** (`iphone/PetApp/App/AppState.swift`):
- 新增 `public private(set) var roomNavigationGeneration: Int = 0`
- `setCurrentRoomId` / `applyHomeData` / `reset` 各 +1（用 `&+= 1` wrapping-overflow safe）

**3 个 UseCase**: 把 `entryRoomId: String?` 入口 capture 改成 `entryGen: Int`,
guard 从 `liveRoomId == entryRoomId` 改成 `liveGen == entryGen`. log 同时打出 entryGen / currentGen.

**3 个 ViewModel catch-path**（Home onCreateTap / Home onJoinRoomConfirm / Friends onJoinFriendTap /
Room onLeaveTap）: 同 UseCase 升级 —— catch 内的 stale-error guard 也从 currentRoomId equality
改成 roomNavigationGeneration equality.

**6 个新 regression 测试**：
- `AppStateTests`: gen 严格单调 + hydrate/reset 也 bump（2 case）
- `CreateRoomUseCaseTests`: nil → B → nil ABA cycle 后 stale create 不能覆盖 currentRoomId
- `JoinRoomUseCaseTests`: nil → B → nil ABA cycle 后 stale join 不能覆盖 currentRoomId
- `LeaveRoomUseCaseTests`: A → nil → A ABA cycle 后 stale leave 200 / 6004 不能 wipe 刚 rejoin 的 A

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写"async 操作 entry/exit 之间状态不变"的 race-guard 时，**禁止**用
> "值 equality"（如 `currentRoomId == entryRoomId`）作为 invariant；**必须**用 monotonic
> generation/version token（即使中间发生 A → X → A 的 ABA cycle 也能检测到）.
>
> **展开**：
> - 任何"用户/系统可以让 mutable state 在 in-flight 期间走完一个完整 cycle 再回到原值"的场景，
>   都触发 ABA 风险. 房间切换 / WebSocket session lifecycle / login-logout-relogin 全部属于这类.
> - 实现模式：在持有 mutable state 的 owner（如 AppState）里加一个 monotonic counter，
>   **每次 mutation 必然 +1**（无视新旧值是否相等）；async caller capture entry counter,
>   完成时 guard counter equality. 严格单调 → ABA-safe.
> - counter 用 `Int.&+= 1`（Swift wrapping overflow）保证 Int.max 后仍单调（按现实使用频率永远到不了，
>   但语义安全 + 不抛 trap）.
> - **不**走 @Published：generation 是 internal race-guard token，不直接给 SwiftUI 看，
>   避免 view diff 因 token 自增触发额外 invalidation.
> - **反例 1**：用 `currentRoomId == entryRoomId` 判断"我开始的时候是 nil，现在还是 nil 说明没变化".
>   错 —— 用户可能已经 join 又 leave 一次.
> - **反例 2**：用 nullable token (`String? == String?`) 判断"还是同一个 room". 错 —— 同一房间的
>   "原 session" 和 "rejoin 后的新 session" 是不同生命周期，token 相同但语义不同.
> - **反例 3**：catch-path 用 `currentRoomId` equality 假装做 stale-guard. 同 ABA 失守.
>   **success path 和 error path 必须用同一 invariant**（generation token），否则两者无法对称.

## Meta: 本次 review 的宏观教训

历次 r2 / r5 / r6 / r9 都已经在补 race-guard，但每次都是"补当前发现的 case"而非"理解 invariant
正确形式". codex r10 的价值是指出**所有过去的 guard 都共用同一个错误 invariant**.

教训：当 review 第二次（甚至第三次）回到同一个语义范畴（"防 stale response"），不要继续用相同
形态加新分支 / 新 case，**要重新思考 invariant 本身是否选错**. 多个补丁堆叠通常是模型有问题的
signal —— 改 model 比加 patch 高效得多.

具体应用到本仓库的其他 race-guard（如 `WebSocketClientImpl.sessionGeneration`,
`RealRoomViewModel.bind` 的 `connectingRoomId`+`webSocketClient ===` 双层校验）：
这些**已经用 generation/identity 模式**，是好榜样. UseCase / ViewModel catch-path 之前只用
roomId equality 是因为同 file 内逻辑简单，但 cross-file invariant 一致性更重要 —— 全 codebase 用同
一套 race-guard idiom 让未来读者一眼看懂.
