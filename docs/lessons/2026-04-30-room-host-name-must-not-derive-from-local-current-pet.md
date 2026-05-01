---
date: 2026-04-30
source_review: codex review round 3 for Story 37.8 (file: /tmp/epic-loop-review-37-8-r3.md)
story: 37-8-roomview-scaffold
commit: 4ee34b3
lesson_count: 1
---

# Review Lessons — 2026-04-30 — Room host 名不得派生自 appState.currentPet（local 猫 ≠ room host 猫）

## 背景

Story 37.8 round 3 codex review（baseline a0de9c4，HEAD 7556329）针对 `RealRoomViewModel.swift:99-103` 提出 1 个 P2 finding：`subscribeHostCatName(to:)` sink 把 `hostCatName` 派生自 `appState.$currentPet`，但 `currentPet` 是**本地用户的猫**，不是 room host 的猫。当 `/home` 接口为"加入了别人房间"的用户 restore session（`currentRoomId` 非 nil + `currentPet` = 本地用户的猫）时，scaffold 标题渲染为 `"<我的猫>的小屋"` —— 这是 **user-visible 错误数据**，违反 pre-WS 阶段"不知道就用 placeholder"的安全策略。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | hostCatName 不得派生自 appState.currentPet | medium (P2) | architecture | fix (option A) | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` |

## Lesson 1: room host 名不得复用 appState.currentPet 作派生源

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix (option A — 删除 sink，永远占位直到 WS 接通)
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:99-103`（修复前）

### 症状（Symptom）

`RealRoomViewModel` 在 init / bind 时通过 `subscribeHostCatName(to:)` sink 订阅 `appState.$currentPet`，把 `pet?.name` 写入 `hostCatName`。但 `appState.currentPet` 永远是**本地登录用户**的猫（由 `/home` API hydrate / WS pet.state.changed 自身分支更新）。当用户加入别人创建的房间后 kill app + 重新启动 → `/home` 返回 `currentRoomId` 非 nil + 本地猫 → restore in-room state → `RoomScaffoldView` 顶部 title 显示 `"<我的猫>的小屋"`，但实际 host 是另一个用户。直到 Story 12.1 接 WS room.snapshot 才有真实 host 名 —— 这段窗口（哪怕几秒）就是 user-visible bug。

### 根因（Root cause）

把"全局 domain state"（`AppState.currentPet`）当成"per-feature 派生源"误用：

1. **AppState 的字段都是"本地用户视角"**。`currentPet` 命名直觉上像是"当前的某只猫"，但 ADR-0010 §3.2 白名单语义钦定为"**本地登录用户**的猫"。Room 域里 host 可能是另一用户，host 的猫不在 AppState 任何字段里。
2. **派生 state 的"sink override 模式"被滥用**。Story 37.8 早前轮 fix 引入"派生 state 必须订阅 publisher 而非一次性 hydrate"的好习惯（避免 reset 后残留旧值），但模式没问"派生源是不是正确的真理源"。pet.name 不是 host 的真理源 —— 把 sink 接上去只是把"残留旧值的 bug"换成"残留错值的 bug"。
3. **pre-WS 占位阶段的安全策略缺位**。Epic 37 是 UI Scaffold，明确数据走 mock。但"mock"的两种做法语义差很大：
   - "mock = 静态占位"（永远 `RoomScaffoldDefaults.hostCatName = "小花"`）
   - "mock = 用本地数据假装"（"我的猫名"当 host 名）
   后者只在"我就是 host"场景下不出错，"我加入了别人的房间"场景必出错。

### 修复（Fix）

采用 review 推荐的 **option A**：彻底删除 hostCatName 的 sink，永远保持 init 时 seed 的 `RoomScaffoldDefaults.hostCatName` 占位，直到 Story 12.1 接 WS room.snapshot 后由新 subscribe 入口写真实 host 名。

具体改动：

1. `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`
   - 删除 `private var hostCatNameSubscription: AnyCancellable?` 字段
   - 删除 `private func subscribeHostCatName(to appState: AppState)` 方法（替换为详细注释说明为什么删 + Story 12.1 forward action）
   - `init(appState:)` / `bind(appState:)` 不再调 `subscribeHostCatName(to:)`
   - 文件头加 round 3 P2 fix 注释
2. `iphone/PetAppTests/Features/Room/RoomViewScaffoldTests.swift`
   - 调整 case#7 (`testRealRoomViewModelBindAppStateThenResetUpdatesFields`) 断言：bind 后 `hostCatName` 应是 `RoomScaffoldDefaults.hostCatName`（不再是 `"测试猫"`），reset 后同样保持占位
   - 新增 case#10 (`testRealRoomViewModelHostCatNameDoesNotDeriveFromCurrentPet`) 契约守护：用 `RealRoomViewModel(appState:)` + `RealRoomViewModel().bind(appState:)` 两路，注入"本地猫已 hydrate"的 AppState → 断言 `hostCatName` 仍 = `RoomScaffoldDefaults.hostCatName`；并校验 currentPet 事后变更也不让 `hostCatName` 跟动

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **给"非自身视角域"（room/social/inbox/...）的 ViewModel 加派生 state** 时，**禁止**直接 sink 订阅 `AppState` 中"本地用户视角"字段（`currentUser` / `currentPet` / `currentStepAccount` / `currentEquips` / `currentInventory`）作为该域的派生源。
>
> **展开**：
> - **AppState 字段语义钦定**（ADR-0010 §3.2）：白名单 7 字段全部是"**本地登录用户视角**"——不是"全局任意主体的最新值"。命名上 `currentPet` 易误读为"当前正在显示的某只猫"，但语义是"我自己的猫"。任何域代码在订阅前必须先问："本域里这个字段代表的'人/物'一定是本地用户吗？" 答否就不能用 AppState。
> - **Room 域的 host 真理源**：只能是 WS `room.snapshot` event（Story 12.1 后到来），或者 REST `/room/info` 响应（Story 12.2）。pre-WS 阶段没有真理源 → 用 `RoomScaffoldDefaults` 占位，**不**用本地猫"假装"。
> - **派生 state sink 的两个独立问题**：① "派生只在 init 一次性 hydrate"会残留旧值（用 sink 修） ② "派生源本身是错的"用 sink 反而把 bug 持久化（必须先验证真理源）。两个问题必须独立检查 —— sink 是工具，不是答案。
> - **pre-feature 阶段（数据未接通前）的安全策略**："不知道就用 placeholder"。**禁止**用本地数据"假装"该域数据 —— 假装在"我就是 host"等单一视角下不出错，但对"我加入别人房间 / 收到别人消息 / 别人在玩游戏" 等场景立刻是 user-visible bug。
> - **何时可以反向例外**：仅当字段语义本身就钦定为"本地用户的某条信息"（如个人 home view 顶部显示自己的猫名）—— 这种 case AppState 字段就是真理源，可以 sink。Room/social 等"对端 / 多方"语境一律不行。
> - **反例**：
>   ```swift
>   // ❌ wrong：room host 名派生自本地猫
>   appState.$currentPet.sink { [weak self] pet in
>       self?.hostCatName = pet?.name ?? RoomScaffoldDefaults.hostCatName
>   }
>   // ❌ wrong（option B 模式）：用 userIsHost 兜底也不行 —— pre-WS 阶段 userIsHost 也是 placeholder true，仍走错路径
>   appState.$currentPet.sink { [weak self] pet in
>       guard let self else { return }
>       self.hostCatName = self.userIsHost ? (pet?.name ?? default) : default
>   }
>   ```
>   ```swift
>   // ✅ right：pre-WS 阶段 hostCatName 永远是 placeholder；Story 12.1 后通过新 subscribe 入口写真实值
>   public override init() {
>       super.init()
>       self.hostCatName = RoomScaffoldDefaults.hostCatName  // 永远占位
>       // 不订阅 appState.$currentPet
>   }
>   // 未来 Story 12.1 落地：
>   private func subscribeRoomSnapshot(to wsClient: WebSocketClient) {
>       wsClient.roomSnapshotPublisher.sink { [weak self] snap in
>           self?.hostCatName = snap.hostMember.catName  // 真理源 = WS snapshot
>       }
>   }
>   ```

---

## Meta: 本次 review 的宏观教训

`/fix-review` 跨多轮处理同一个 ViewModel 文件时，前轮的"局部正确"修复可能掩盖更深的"语义错误"：

- round 1 P2 fix 教会了"两条 init 路径都 seed defaults"——对的
- round 1 还引入了"sink 派生 hostCatName from currentPet"——本身是"派生 state 用 publisher 而非 hydrate"模式的应用，但**派生源选错了**
- round 2 P2 fix 处理 `bind(appState:)` 同步 vs 异步契约——对的
- round 3 P2 才发现 round 1 引入的 sink 派生源错误

教训：每次给 ViewModel 加 sink 时，三个问题必问一遍：
1. **派生源真的是这个域的真理源吗？**（语义校验，不是结构校验）
2. **如果不是，pre-feature 阶段的占位策略是什么？**（永远占位 vs 用 mock 数据派生）
3. **被订阅的 publisher 字段语义在文档/ADR 里是怎么钦定的？**（如 ADR-0010 §3.2 表格）

这三问能挡住多数"sink 接对了结构、接错了语义"类 bug。
