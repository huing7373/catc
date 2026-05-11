---
date: 2026-05-11
source_review: /tmp/epic-loop-review-12-7-r13.md（codex review · 12.7 round 13）
story: 12-7-创建-加入-退出-use-case-主界面入口完善
commit: 4b0d2cf
lesson_count: 1
---

# Review Lessons — 2026-05-11 — UITEST nil-WS-client 路径 nil→A 必须保留 RoomScaffoldDefaults seed roster

## 背景

Story 12.7 round 11 引入 nil→A 分支无条件清空 `members = []` + `memberPetStates = [:]` 修复"connect 失败时 RoomScaffoldDefaults seed 假成员永久残留"问题（lesson `2026-05-11-room-entry-must-clear-scaffold-roster-before-connect-12-7-r11.md`）.

round 13 codex review 命中：r11 改动直接 break UITEST_SKIP_GUEST_LOGIN=1 + UITEST_FORCE_IN_ROOM=1 launch path（r3 P1 引入）—— 该路径下 RootView 显式把 `webSocketClient` 传 nil，永远不会有 `room.snapshot` 到达；清空 roster 让 `RoomScaffoldView` 永久失去 seeded 4 成员 → `testJoinRoomModalCrossScreenJoinFlow` 等断言 `roomMember_0/1/2` 出现的既有 UITest 全 break.

修复：把 nil→A / A→B 分支的 roster reset **条件化** —— 仅当 `webSocketClient != nil`（production path）时清；nil-client 路径下保留 scaffold seed.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | UITEST nil-WS-client 路径 nil→A 清空 roster 让 scaffold 4 成员永久消失 | high | testing | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:319-320, 417-418` |

## Lesson 1: nil→A / A→B 分支的 roster reset 必须 gate on `webSocketClient != nil`

- **Severity**: high
- **Category**: testing
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:319-320` (nil→A) / `417-418` (A→B)

### 症状（Symptom）

UITEST_SKIP_GUEST_LOGIN=1 + UITEST_FORCE_IN_ROOM=1 launch 路径下：

1. `RootView.swift:293` 把 `webSocketClient` 传 nil 给 `RealRoomViewModel.bind(...)`（r3 P1 引入 —— UITEST 路径下避开真实 WS connect 抛 `WSError.tokenMissing`）.
2. `RootView.swift:433` 写 `appState.setCurrentRoomId(...)` 触发 nil→A.
3. r11 fix 在 nil→A 分支无条件 `self.members = []` + `self.memberPetStates = [:]`.
4. nil-client 路径下既不会 `prepareForReconnect()`、也不会拨号 connect、永远没有 `room.snapshot` 到达.
5. 结果：`RoomScaffoldView` 渲染时 `vm.members` 是空数组 → `roomMember_0/1/2/3` accessibility identifier 全不出现 → 断言它们存在的 UITest（`testJoinRoomModalCrossScreenJoinFlow` 等）全 break.

### 根因（Root cause）

r11 fix 推理只考虑了 production path（webSocketClient ≠ nil + snapshot 不到 = 假成员残留）→ 选了"立即清空 roster"作为 universal 修复；**没有把 UITEST nil-client 路径（r3 P1 引入）纳入推理**.

更宏观的盲点：vm 内 `webSocketClient` 是**两种语义的状态机**而非单一 production assumption：
- **non-nil**：production / mock-with-snapshot 路径 —— 必有 snapshot 或显式失败信号，roster 会被覆盖
- **nil**：UITEST / preview / scaffold-only 路径 —— **永远不会有 snapshot**，roster 必须保留 seed

任何"等 snapshot 到达"的副作用（如 clear members）都必须 gate on `webSocketClient != nil`，否则 nil-client 路径下副作用永久生效，无任何信号能纠正.

### 修复（Fix）

把 nil→A / A→B 分支的 roster reset 改成条件化（仅当 `webSocketClient != nil` 时清）：

```swift
// 旧 r11 实装（unconditional clear）：
self.members = []
self.memberPetStates = [:]

// r13 修复（gated clear）：
if self.webSocketClient != nil {
    self.members = []
    self.memberPetStates = [:]
}
```

A→B 分支同样 gate（即使 UITEST 当前不会触发 A→B，对齐语义防御未来 UITest 扩展）.

回归测试（`iphone/PetAppTests/Features/Room/RealRoomViewModelTests.swift`）：
- `testNilToAPreservesScaffoldRosterWhenClientIsNil` —— 不注入 mock client + 触发 nil→A → 验证 4 成员保留
- `testNilToAClearsRosterWhenClientIsNotNil` —— 注入 mock client + 触发 nil→A → 验证 roster 仍立即清空（保持 r11 production 语义）

r11 既有 `testNilToAClearsScaffoldRosterBeforeConnect` / `testNilToAClearsRosterEvenWhenConnectFails` 都假设 `webSocketClient ≠ nil`，语义未变，保留即可.

UITEST 实跑验证（ios-simulator MCP）：UITEST_SKIP_GUEST_LOGIN=1 + UITEST_FORCE_IN_ROOM=1 启动后，`ui_find_element(["roomMember_0".."roomMember_3"])` 返回 4 个 group（小花 / Mocha / Latte / Espresso）—— scaffold roster 完整保留.

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 ViewModel 副作用代码里"等异步信号到达再覆盖 seed/初始状态"时，**必须** gate on "异步信号源是否会到达"的前置条件（如 `webSocketClient != nil` / `useCase != nil`）；nil 路径下保留 seed.
>
> **展开**：
> - 当一个 ViewModel 支持两类 dependency 注入语义（production 注入 vs UITEST/preview nil-注入）时，任何"等异步信号覆盖"的清空 / reset 操作都必须**条件化** —— 否则 nil 路径下副作用永久生效，无任何信号能纠正.
> - 检查清单：当你写 `self.x = []` / `self.x = nil` / `self.x = defaultValue` 是为了"等下一个 server / WS 信号覆盖"，**必须**先问：
>     1. 这个 vm 是否有 nil-injected 路径（如 UITEST / preview）？
>     2. 那条路径下"下一个信号"会到吗？不会到 → 必须 gate.
> - 配套回归测试：**两组对照** —— nil-client 验证 seed 保留 + non-nil-client 验证立即清空，让未来回归捕获本类盲点.
> - **反例**：r11 fix 在 nil→A 分支无条件 `self.members = []` —— 只验证了 mock-with-snapshot 路径，没验证 nil-client 路径；r13 review 命中.
>
> **关联 lessons**：
> - `2026-05-11-room-entry-must-clear-scaffold-roster-before-connect-12-7-r11.md`（r11 引入的 unconditional clear —— 本 lesson 是它的精化）
> - `2026-05-11-ws-connect-failure-and-uitest-real-ws-wiring-12-7-r3.md`（r3 P1 引入 UITEST 路径 webSocketClient = nil 的 gate）
> - `2026-05-11-uitest-fallback-and-leave-room-stale-response-guard.md`（r2 引入 UITEST 路径 leaveRoomUseCase = nil 的同精神 fallback）
