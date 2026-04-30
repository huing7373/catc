---
date: 2026-04-30
source_review: codex round 1 review of Story 37-3-rootview-maintabview-改造 (file: /tmp/epic-loop-review-37-3-r1.md)
story: 37-3-rootview-maintabview-改造
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-30 — Coordinator 必须镜像 server 加载的房间态 & 路由白名单缩窄时不能丢 presenter

## 背景

Story 37.3 把 iPhone App 主入口从 "HomeView 3 CTA + Sheet 路由" 改造成 "MainTabView 4 Tab + HomeContainerView 互斥状态机"（ADR-0009 §3.5 步骤 1-3）。改造引入两类 patch：① 删除主入口 Sheet 渲染（`.fullScreenCover(item:)` modifier）+ 删除 `homeView` 闭包参数；② `HomeContainerView` 内部根据 `coordinator.currentRoomId` 在 HomeView ↔ RoomViewPlaceholder 互斥切换。codex round 1 review 揭出两个功能回归：bootstrap 完成后 `coordinator.currentRoomId` 没从 `/home` 数据更新（已在房间用户回 idle home），以及 `.compose` 路由保留但 presenter 删干净（`present(.compose)` 变 silent no-op）。两条均 fix。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | bootstrap 完成后 `coordinator.currentRoomId` 没从 `/home` 数据传播 | High (P1) | architecture | fix | `iphone/PetApp/App/RootView.swift:144-146` |
| 2 | `.ready` 子树缺 `.fullScreenCover` presenter，`.compose` 路由变 silent no-op | Medium (P2) | architecture | fix | `iphone/PetApp/App/RootView.swift:225-229` |

## Lesson 1: Coordinator 字段必须从 server 加载数据回填，否则 view-routing 退化为 stale

- **Severity**: High (P1)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift` 中 `ensureLaunchStateMachineWired()` 内 `bootstrapStep1` closure 的 `await MainActor.run { ... }` 块.

### 症状（Symptom）

`/home` 接口返回 `room.currentRoomId != nil` 时，bootstrap closure 只调了 `homeViewModel.applyHomeData(homeData)`。但 `HomeContainerView` 互斥状态机以 `coordinator.currentRoomId` 为决策入参（`HomeRoomDispatcher.shouldShowRoom(currentRoomId:)`），而 coordinator 这个字段从未被任何 bootstrap/retry 路径写入 → 已在房间的用户每次 cold-start / retry 都被错误落到 idle home screen，看不到 RoomViewPlaceholder。

### 根因（Root cause）

Story 37.3 把"是否在房间"这个 routing 决策的 source of truth 从 `homeViewModel.homeData?.room.currentRoomId`（间接 source）移到 `coordinator.currentRoomId`（直接 source，临时占位字段，Story 37.4 后会改为 `appState.currentRoomId`）。但改造时只动了"读路径"（HomeContainerView 改读 coordinator），没动"写路径"（bootstrap closure 仍只更新 ViewModel，没写 coordinator）。**routing source of truth 重定向时，写入端必须同步迁移**，否则字段从 init 默认值开始就再也不会变。

误判触发条件：dev 在改 read-path 时容易把"哪些地方读"当作 review checklist，但忘了"哪些地方该写"——尤其是 source 是新加的 `@Published` 字段时，旧 write-path（`applyHomeData`）不会自动覆盖到。

### 修复（Fix）

`iphone/PetApp/App/RootView.swift:135-156` `bootstrapStep1` closure 的 success 分支 `await MainActor.run` 块内追加一行，在 `homeViewModel.applyHomeData(homeData)` 之后写 `coordinator.currentRoomId = homeData.room.currentRoomId`。同时 closure 顶部增加 `let coordinator = self.coordinator` 让 `@Sendable` closure 能 capture。

```swift
await MainActor.run {
    homeViewModel.applyHomeData(homeData)
    // codex round 1 [P1] fix: 把 /home 返回的 room.currentRoomId 传播进 coordinator.
    coordinator.currentRoomId = homeData.room.currentRoomId
}
```

测试覆盖（`iphone/PetAppTests/App/RootViewWireTests.swift`）：新增 `testBootstrapPropagatesLoadedHomeRoomIdToCoordinator` + `testBootstrapKeepsCoordinatorCurrentRoomIdNilWhenHomeRoomIsEmpty` 两个 case，复刻 step1 closure 的 `MainActor.run` 写入模式，断言 `coordinator.currentRoomId` 在 bootstrap 完成后等于 `homeData.room.currentRoomId`，并通过 `HomeRoomDispatcher.shouldShowRoom(currentRoomId:)` 验证 view-routing 决策结果一致。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **把 view-routing 决策的 source of truth 从间接 source（如 ViewModel.data.x）迁移到直接 source（如 coordinator.x / appState.x）** 时，**必须** **同步迁移所有写入路径**，让新 source 在所有 bootstrap / retry / refresh / WebSocket push 路径都被回填。
>
> **展开**：
> - "改读路径" 必须 grep 全仓搜 "改后字段在哪些地方被读"，再反向 grep "原字段在哪些地方被写" → 把每个写点都迁移到新 source。漏一个写点就会让 view-routing 在该路径下 stale。
> - 对每个 source-of-truth 字段，在 bootstrap closure 的 success path 内**显式**赋值（不能依赖 "字段默认 nil 也能跑通" 这种隐式约束）—— 因为 bootstrap 是 cold-start 的唯一路径，错过它后续 view 永远拿默认值。
> - 测试一定要走 closure-shape regression：以 RootView.bootstrapStep1 closure 为模板，在 RootViewWireTests 复刻该 closure，断言"完成后 coordinator/appState 字段值与 input 数据一致"。这种 closure-shape test 不依赖 SwiftUI runtime（ADR-0002 §3.1 禁用 ViewInspector），但能精确守护 RootView wire 的语义。
> - **反例**：临时占位字段（如 Story 37.3 的 `coordinator.currentRoomId`）从 `@Published` 默认 nil 开始，dev 只在 view 读路径接它，不在 bootstrap 写路径接它，认为"反正 Story 37.7 会落地写入" —— 这种"留给下个 story" 思维会把已合入的代码变成功能回归，dev review 抓到时已经是 codex round。占位字段的写入约定**与读取约定同时落地**才是对的。

## Lesson 2: 路由白名单缩窄不等于删 presenter；保留任何 enum case 都必须保留对应 view 渲染

- **Severity**: Medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift` `LaunchedContentView.body` `.ready` 分支.

### 症状（Symptom）

`SheetType` enum 在 Story 37.3 中从 3 case (`.room` / `.inventory` / `.compose`) 缩窄到 1 case (`.compose`)，`AppCoordinator.present(_:)` API 仍保留。但 `LaunchedContentView .ready` 分支整段移除了 `.fullScreenCover(item: $coordinator.presentedSheet)` modifier。结果：任何调 `coordinator.present(.compose)` 的流程会改 `presentedSheet` state 但 UI 不渲染 → silent no-op，dev / QA 看不到任何错误信号。

### 根因（Root cause）

dev 在 ADR-0009 §3.4 缩窄 SheetType 白名单时，把 "Sheet 路由整段移除" 误读成 "modifier 整段删"，没察觉 ADR §3.4 钦定 `.compose` 仍在白名单内（只是 `.room` / `.inventory` 被 4 Tab 接管）。**白名单缩窄 ≠ 路由删除**：保留任何一个 case 就必须保留 presenter；删 presenter 等于把保留的 case 变成"代码里有但 runtime 没用" 的 dead state。

误判触发条件：`SheetType` enum 被减到只剩 1 case 时，dev 容易把"代码体量缩小"当作"路由整段移除"，但 enum 仍是 `Identifiable` + `Equatable` + `present(_:)` API 仍 public → 这些都是"路由仍 alive" 的明确 signal。

### 修复（Fix）

`iphone/PetApp/App/RootView.swift:225-254` `.ready` 子树重新挂回 `.fullScreenCover(item: $coordinator.presentedSheet)` modifier，`switch sheet { case .compose: ComposeSheetPlaceholder() }` 渲染。新增 `ComposeSheetPlaceholder` struct（临时占位 view，渲染 `Text("compose placeholder")` + a11y identifier `compose_placeholder`）—— Story 33.1 落地真实合成 view 时替换。

```swift
.fullScreenCover(item: $coordinator.presentedSheet) { sheet in
    switch sheet {
    case .compose:
        ComposeSheetPlaceholder()
    }
}
```

测试覆盖（`iphone/PetAppTests/App/SheetTypeTests.swift`）：新增 `testComposeSheetPlaceholderIsConstructible` 类型构造守护测试。SwiftUI fullScreenCover modifier 行为本身无法走 unit test（ADR-0002 §3.1 禁用 ViewInspector / SnapshotTesting），用 a11y identifier `compose_placeholder` 给 UITest（XCUITest）做断言锚点。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **缩窄 enum-driven 路由白名单（删除部分 case 但保留至少一个 case）** 时，**必须** **保留对应 case 的 view presenter**，不能把"白名单缩到 1 case"误读成"路由整段删除"。
>
> **展开**：
> - 删除 enum case → 删 view presenter 内的 case 分支（编译器会指出 missing branch）；删 view presenter 整段 modifier → 必须把 enum 也删空 + 把 `present(_:)` API 一并删。两边任意一边只删一半都是 silent no-op 风险。
> - "白名单缩窄" decision（如 ADR-0009 §3.4 钦定 SheetType 仅留 .compose）的 review checklist：① enum 删了哪些 case？② API surface 还有哪些方法接 enum？③ View 的 presenter modifier（`.sheet` / `.fullScreenCover` / `.popover`）有几处接 enum binding？三处必须同步动；保留任意一项都意味着"路由仍 alive"。
> - 即使保留的唯一 case 还没有真实 view（如 Story 33.1 才落地真实合成 view），也要先放占位 stub view（`Text("foo placeholder")` + a11y identifier）。占位 view 的存在是"路由 alive"的可验证证据，让 UITest / dev 调试都不会 silent fail。
> - **反例**：dev 删 SheetType 的 `.room` / `.inventory` 两个 case 后，看到剩下 `.compose` 一个 case 觉得 "Sheet 路由整段都已经废了"，把 `.fullScreenCover` modifier 整段删掉，但忘了 `.compose` case 仍在 enum 内、`AppCoordinator.present(.compose)` API 也仍 public。结果 `present(.compose)` 改了 `@Published presentedSheet` 但 UI 不响应，dev 自测时也很难发现（state 改了但渲染没变化往往被当成"还没接业务"）。

---

## Meta: 本次 review 的宏观教训

两条 finding 共同的根因是 **"主入口 IA 改造（3 CTA → 4 Tab + 互斥状态机）时，对'保留'与'删除' 的边界没贯彻到底"**。Lesson 1 是"保留的 routing source（currentRoomId）只读不写"；Lesson 2 是"保留的 routing case（.compose）有 enum 没 view"。两者都是 **"半保留"反模式**：API surface 部分保留 + 实现部分删除，导致代码看起来还在跑但 runtime 行为已经退化为静默失败。

未来 Claude 处理 IA 改造 / 路由重构时，对每个"保留"决策都要走两个问题：**①"保留它的写入路径在哪？"**（防 stale source）**②"保留它的 view 渲染路径在哪？"**（防 silent no-op）。两个答案都给得出才算"保留"决策真正落地。
