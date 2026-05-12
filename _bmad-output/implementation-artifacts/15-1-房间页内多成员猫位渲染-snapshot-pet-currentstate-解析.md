# Story 15.1: 房间页内多成员猫位渲染 + snapshot pet.currentState 解析

Status: done

## Story

As an iPhone 用户,
I want 进入房间后能立即看到房间内每个成员的猫和当前状态,
So that 我能直观感受到房间里大家在干什么.

## 故事定位（Epic 15 第 1 条 story；节点 5 iOS 端"房间内多成员猫 sprite 渲染 + snapshot pet.currentState 真实解析"收口）

- **Epic 15 进度**：**15.1（本 story，房间页内多成员猫位渲染 + snapshot pet.currentState 解析）** → 15.2（pet.state.changed WS 消息处理）→ 15.3（状态切换动画过渡）→ 15.4（自己状态变化时上报 state-sync）→ 15.5（跨房间状态恢复）
- **本 story 是 Epic 15 第一条 story**：将 Story 12.3 / 12.1 节点 4 阶段锁定的"memberPetStates 保持空 map + server currentState 固定 1"**解禁**——自 Story 14.3 起 server snapshot 真实下发 pet.currentState 1/2/3，本 story 在 iOS 端完成解析 + 映射 + PetSpriteView 渲染
- **本 story 是 Epic 15 后续 stories 强前置**：
  - Story 15.2（pet.state.changed WS 消息处理）→ 需要本 story 落地的 memberPetStates 写入机制 + MotionState 映射
  - Story 15.3（状态切换动画过渡）→ 需要本 story 落地的 PetSpriteView 在 RoomScaffoldView 中的接缝
  - Story 15.5（跨房间状态恢复）→ 依赖 snapshot 解析写入 memberPetStates 的通路
- **节点 5 验收要求（§4.5）**：本 story 落地后用户进入房间可看到每个成员猫的真实状态（rest/walk/run），为节点 5 "房间内至少两名用户在线时，可看到对方的猫状态变化"提供视觉基础

## Acceptance Criteria

> **AC 编号体系**：AC1 是 snapshot pet.currentState 解析 + memberPetStates 写入；AC2 是 RoomScaffoldView 成员行增加 PetSpriteView 渲染；AC3 是单元测试 ≥4 case；AC4 是 UI 测试覆盖；AC5 是 build verify；AC6 是 Deliverable 清单。

---

### AC1 — snapshot pet.currentState 解析 + memberPetStates 真实写入（解禁节点 4 空 map 占位）

**给定**：

- Story 12.1 / 12.3 节点 4 阶段锁定 `memberPetStates` 为空 map（`applySnapshot` 内注释"节点 4 阶段 server 固定 currentState=1，不写入"）
- Story 14.3 已落地：server `room.snapshot.payload.members[].pet.currentState` 真实下发 1/2/3
- `RoomSnapshotPet.currentState: Int` 字段已就位（WebSocketClient.swift:172）
- `HomePetState` enum（rawValue Int：1=rest / 2=walk / 3=run）已就位（HomeData.swift:120-124）
- RoomViewModel 基类 `memberPetStates: [String: HomePetState]` 字段已就位（RoomViewModel.swift:42）

**预期行为**：

1. **`RealRoomViewModel.applySnapshot(_:)` 修改**：在 roster 集合 merge 完成后（现有 `self.members = newMembers` 之后），遍历 `payload.members`，对每个成员：
   - `snapshotMember.pet != nil` 且 `HomePetState(rawValue: snapshotMember.pet!.currentState) != nil` → 写入 `memberPetStates[snapshotMember.userId] = mappedState`
   - `snapshotMember.pet != nil` 但 `currentState` 值未知（如 99）→ 默认按 `.rest` 处理 + `os_log(.error, ...)` log warning（防御性，不应发生）
   - `snapshotMember.pet == nil`（pet-less 账号）→ 默认 `.rest`（不写入 memberPetStates 或写入 .rest 均可；推荐写入 .rest 让 PetSpriteView 有默认渲染）
   - snapshot 中不存在的 userId（已从 roster 移除的成员）→ 从 `memberPetStates` 中 removeValue
2. **`RealRoomViewModel.applyMemberJoined(_:)` 修改**：新成员加入时如 `payload.pet != nil`，同样解析 `currentState` 写入 `memberPetStates`
3. **`RealRoomViewModel.applyMemberLeft(_:)` 检查**：既有实装已有 `memberPetStates.removeValue(forKey: payload.userId)`（line 793），**不需改动**
4. **映射规则**：`HomePetState(rawValue: currentState)` 直接映射（1→.rest / 2→.walk / 3→.run）；未知值 fallback `.rest`

**红线**：

- **不**改 `RoomSnapshotPayload` / `RoomSnapshotMember` / `RoomSnapshotPet` struct 定义（WebSocketClient.swift，Story 12.1 已稳定）
- **不**改 `WSMessageCodec.decode` 路径（Story 12.2 已稳定）
- **不**引入新的 async subscribe / Combine Publisher —— memberPetStates 是 @Published dict，applySnapshot 写入后 SwiftUI 自动刷新
- **不**在 applySnapshot 内引入 MotionState 类型 —— 使用 HomePetState（Int rawValue 直接匹配 server wire 值 1/2/3）；MotionState 用于 PetSpriteView 渲染层（AC2 负责 HomePetState → MotionState 桥接）
- 移除或替换 `applySnapshot` 内原有"节点 4 阶段 server 固定 currentState=1，不写入"相关注释（标注"自 Story 14.3 / 15.1 起切真实值"）

**对应 Tasks**: Task 1.1, 1.2

---

### AC2 — RoomScaffoldView 成员行增加 PetSpriteView 渲染（每个成员位用 PetSpriteView(state:) 显示猫状态）

**给定**：

- Story 8.4 已落地 `PetSpriteView(state: MotionState)` 组件（iphone/PetApp/Features/Home/Views/PetSpriteView.swift）
- `MotionState` enum（.rest/.walk/.run）已就位（Core/Motion/MotionState.swift）
- `HomePetState` 到 `MotionState` 需要映射桥接（HomePetState.rest → MotionState.rest 等）
- RoomScaffoldView 成员行（memberRow）当前渲染：Avatar + 名字 + 队长 tag + "小猫 Lv.x · status" + paw icon

**预期行为**：

1. **HomePetState → MotionState 桥接**：在 `HomePetState` 上新增 computed property `var motionState: MotionState`（或在 RoomScaffoldView / RoomViewModel 内做映射），将 `.rest → .rest` / `.walk → .walk` / `.run → .run` 对应。
2. **RoomScaffoldView.memberRow 修改**：在成员行内加入 `PetSpriteView(state:)` 渲染：
   - 从 `state.memberPetStates[member.id]` 取 `HomePetState`（不存在时 fallback `.rest`）
   - 桥接为 `MotionState` 后传给 `PetSpriteView(state:)`
   - PetSpriteView 放在成员行合适位置（推荐替换现有 paw icon 位置 或 与 paw icon 并列，dev 决策时选其一）
   - **尺寸缩小**：房间成员行空间有限，PetSpriteView frame 应缩小到合适大小（如 40x40pt），不使用 HomeView catStage 的 180x180pt 尺寸
3. **PetSpriteView accessibility identifier**：每个成员位的 PetSpriteView 应有唯一 a11y identifier，格式 `petSprite_<state>_<memberIndex>` 或复用 `petSprite_rest/walk/run`（AC4 UI 测试需定位）
4. **自己的成员位也用相同方式渲染**（不区分自己 / 别人，epics.md §15.1 钦定）

**红线**：

- **不**改 PetSpriteView.swift 内部实现（Story 8.4 已稳定；如需调整尺寸用 `.frame()` modifier 在 caller 侧处理）
- **不**改 RoomScaffoldView 的 5 区块结构（topBar / roomCodeCard / sharedStage / membersList / leaveButton）
- **不**移除成员行现有 Avatar / 名字 / 队长 tag / 副标题渲染（仅新增 PetSpriteView 元素）
- **不**引入 `@StateObject` / `@ObservedObject` 新 ViewModel —— PetSpriteView 是 stateless representation，直接从 state.memberPetStates 读值传入

**对应 Tasks**: Task 2.1, 2.2, 2.3

---

### AC3 — 单元测试 ≥4 case（mocked WebSocketClient，覆盖 epics.md §15.1 钦定的 4 个 case）

**扩展测试文件**：`iphone/PetAppTests/Features/Room/RealRoomViewModelTests.swift`（Story 12.1 / 12.3 / 12.4 已落地多个 case；本 story **扩展**新 case 平级累加，**不**删除既有 case）

**必须覆盖的测试 case**（按 epics.md §Story 15.1 行 2382-2386 钦定）：

```
case#A happy: snapshot 含 3 成员 + currentState 分别为 1/2/3
  → vm.memberPetStates["userId1"] == .rest
  → vm.memberPetStates["userId2"] == .walk
  → vm.memberPetStates["userId3"] == .run
  测试方法：构造 RoomSnapshotPayload 含 3 成员各有 pet.currentState=1/2/3
  → mockWS.emit(.roomSnapshot(payload)) → 等 yield → 断言 memberPetStates

case#B edge: snapshot 含未知 currentState 值（如 99）
  → 默认按 .rest 处理 + log warning
  测试方法：构造 1 成员 pet.currentState=99
  → mockWS.emit(.roomSnapshot(payload)) → 等 yield
  → 断言 memberPetStates[userId] == .rest

case#C happy: snapshot 缺 pet 字段（pet-less 兜底）
  → 默认 .rest
  测试方法：构造 1 成员 pet=nil
  → mockWS.emit(.roomSnapshot(payload)) → 等 yield
  → 断言 memberPetStates 对该 userId 的值为 .rest（或不存在，按 AC1 实装策略）

case#D happy: 同一房间多次刷新 snapshot → memberPetStates 正确同步更新
  测试方法：
    1. emit snapshot 含 2 成员 currentState=1/2 → 断言 memberPetStates 正确
    2. emit 同 userId 但 currentState=2/3 → 断言 memberPetStates 更新为 .walk/.run
```

**测试基础设施约束**（与 Story 12.1 / 12.3 一致）：

- XCTest only（`@testable import PetApp`）
- `@MainActor` 标注测试 class
- `WebSocketClientMock.emit(_:)` 驱动 stream
- `await Task.yield()` / `waitForMembersCount(vm:expected:)` helper 让 AsyncStream 派发到 ViewModel @Published
- 不引 ViewInspector / SnapshotTesting / 不起 mock HTTP server

**对应 Tasks**: Task 3.1, 3.2, 3.3, 3.4

---

### AC4 — UI 测试覆盖（mock vm 注入 3 成员各自不同 state → 验证 3 个 PetSpriteView 的 accessibility identifier）

**扩展测试文件**：`iphone/PetAppUITests/RoomUITests.swift`（Story 12.1 / 12.3 已落地 case；本 story **新增** 1 case 平级累加）

**必须新增 UITest case**（按 epics.md §Story 15.1 行 2387 钦定）：

```
case: mock RoomViewModel 注入 3 成员各自不同 state
  → RoomView 验证 3 个 PetSpriteView 的 accessibility identifier 各为
    "petSprite_rest" / "petSprite_walk" / "petSprite_run"

路径：利用既有 UITEST_ROOM_THREE_MEMBERS=1 launch flag（Story 12.3 已落地）
  → MockRoomViewModel 注入 3 成员 + 设置 memberPetStates 为 {userId1: .rest, userId2: .walk, userId3: .run}
  → 断言 3 个 PetSpriteView a11y identifier 可定位
```

**策略选择**：与 Story 12.3 UITest 路径一致（策略 A：launch flag 切 MockRoomViewModel + fixed 数据）

**红线**：

- **不**在 UITest 中起真实 WS server
- **不**删除 / 重写 Story 12.1 / 12.3 既有 UITest case
- PetSpriteView a11y identifier 已由 Story 8.4 定义（`petSprite_rest` / `petSprite_walk` / `petSprite_run`）；如房间内每个成员需要区分，考虑在 RoomScaffoldView 内用 `.accessibilityIdentifier(...)` 在外层容器区分

**对应 Tasks**: Task 4.1

---

### AC5 — Build verify

**必须通过**：

```bash
bash iphone/scripts/build.sh --test
```

- xcodebuild 编译通过
- 所有单测通过（含本 story 新增 ≥4 case + 既有全部 case 不破）
- UITest 通过（本 story 新增 1 case + 既有 case 不破）

**ios-simulator MCP 验证**（CLAUDE.md "iOS UI 验证（必跑）"）：

```
1. bash iphone/scripts/build.sh
2. install_app(app_path: iphone/build/DerivedData/Build/Products/Debug-iphonesimulator/PetApp.app)
3. launch_app(bundle_id: "com.zhuming.pet.app", terminate_running: true)
4. 通过 UITEST_FORCE_IN_ROOM 或 UITEST_ROOM_THREE_MEMBERS launch flag 进入房间页
5. ui_view 验证：成员列表每行是否显示 PetSpriteView（SF Symbol 占位 sprite）
6. ui_describe_all 验证 PetSpriteView a11y identifier 存在
```

**对应 Tasks**: Task 5.1

---

### AC6 — Deliverable 清单

**修改文件**：

- `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`（AC1 核心修改：applySnapshot + applyMemberJoined 写入 memberPetStates）
- `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift`（AC2：memberRow 增加 PetSpriteView 渲染）
- `iphone/PetApp/Features/Home/Models/HomeData.swift`（AC2 推荐：HomePetState 增加 `motionState` computed property 桥接）
- `iphone/PetAppTests/Features/Room/RealRoomViewModelTests.swift`（AC3：新增 ≥4 case）
- `iphone/PetAppUITests/RoomUITests.swift`（AC4：新增 1 case）

**可能修改文件**：

- `iphone/PetApp/Features/Room/ViewModels/MockRoomViewModel.swift`（如 UITest 需要注入 memberPetStates 到 mock）
- `iphone/PetApp/App/RootView.swift`（如 UITEST_ROOM_THREE_MEMBERS launch flag 路径需要同时设置 memberPetStates）

**不需新建文件**：本 story 不引入新 Swift 类型 / 新 Domain 模型 / 新 ViewModel。PetSpriteView 已在 Story 8.4 落地，直接复用。

**对应 Tasks**: Task 6.1

## Tasks / Subtasks

- [x] **Task 1.1** — 修改 `RealRoomViewModel.applySnapshot(_:)` 解析 snapshot pet.currentState 写入 memberPetStates（AC1）
  - [x] 1.1.1 遍历 `payload.members`：`pet != nil` → `HomePetState(rawValue: pet.currentState)` 映射写入；未知值 fallback `.rest` + log error
  - [x] 1.1.2 `pet == nil`（pet-less）→ 写入 `.rest` 默认值
  - [x] 1.1.3 snapshot 中不存在的 userId → 从 memberPetStates 移除（整体替换 `memberPetStates = newPetStates` 语义；snapshot 之外的 userId 自然不出现 → 实现 removeValue 等价语义）
  - [x] 1.1.4 移除/替换旧"节点 4 阶段不写入"注释，标注"自 Story 15.1 起切真实值"
- [x] **Task 1.2** — 修改 `RealRoomViewModel.applyMemberJoined(_:)` 解析 member.joined pet.currentState 写入 memberPetStates（AC1）
  - [x] 1.2.1 `payload.pet != nil` → 同 Task 1.1 映射规则写入（抽 `applyJoinedPetState` helper 与 applySnapshot 一致语义）
  - [x] 1.2.2 `payload.pet == nil` → 写入 `.rest` 默认值
- [x] **Task 2.1** — HomePetState 增加 `motionState` computed property（AC2）
  - [x] 2.1.1 `HomePetState.motionState: MotionState` → `.rest → .rest` / `.walk → .walk` / `.run → .run`
- [x] **Task 2.2** — RoomScaffoldView.memberRow 增加 PetSpriteView 渲染（AC2）
  - [x] 2.2.1 从 `state.memberPetStates[member.id]` 取 HomePetState → `.motionState` 转 MotionState → 传入 PetSpriteView(state:)
  - [x] 2.2.2 PetSpriteView 用 `.frame(width: 40, height: 40)` 缩小（房间成员行空间有限）
  - [x] 2.2.3 放在成员行内合适位置（dev 决策：替换原 paw icon 位置 —— 与 paw icon 等价功能槽位，避免增加新位置打破 5 区块视觉契约）
- [x] **Task 2.3** — PetSpriteView 在房间成员行内的 a11y identifier 处理（AC2）
  - [x] 2.3.1 确保 PetSpriteView 自带的 `petSprite_rest/walk/run` identifier 在 RoomScaffoldView 上下文中可被 UITest 定位（dev 决策：必须在 memberRow 外层加 `.accessibilityElement(children: .contain)`，否则父层 `roomMember_N` identifier 会把 PetSpriteView 的 a11y leaf 合并掉 —— 与 HomeView.catStage 同精神）
- [x] **Task 3.1** — RealRoomViewModelTests case#A: 3 成员 currentState 1/2/3 → memberPetStates 正确映射（AC3）
- [x] **Task 3.2** — RealRoomViewModelTests case#B: 未知 currentState 值 99 → fallback .rest（AC3）
- [x] **Task 3.3** — RealRoomViewModelTests case#C: pet=nil（pet-less）→ 默认 .rest（AC3）
- [x] **Task 3.4** — RealRoomViewModelTests case#D: 多次 snapshot 刷新 → memberPetStates 正确同步更新（AC3）
- [x] **Task 4.1** — RoomUITests 新增 1 case: mock vm 注入 3 成员不同 state → 验证 PetSpriteView a11y identifier（AC4）
- [x] **Task 5.1** — `bash iphone/scripts/build.sh --test` 全绿 + ios-simulator MCP 实跑验证（AC5）
- [x] **Task 6.1** — Deliverable 清单核对 + sprint-status 收尾（AC6）

## Dev Notes

### 关键文档锚定

- `docs/宠物互动App_总体架构设计.md` — iOS Swift + SwiftUI / WebSocket
- `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §9.1 / §9.2（WS 子系统职责）
- `docs/宠物互动App_V1接口设计.md` §12.3（room.snapshot 字段表 + client merge contract + pet.currentState 字段枚举 1/2/3）
- `docs/宠物互动App_V1接口设计.md` §5.2（pet.currentState 权威等价桶四处枚举 —— 自 Story 14.3/14.4 起全部就绪）
- `_bmad-output/implementation-artifacts/12-3-房间快照解析-成员列表渲染.md`（前置 story —— snapshot 解析 + memberPetStates 空 map 守护）
- `_bmad-output/implementation-artifacts/8-4-主界面猫-sprite-三态动画切换.md`（PetSpriteView 实装 + MotionState 订阅 + 200ms 平滑过渡）
- `_bmad-output/implementation-artifacts/14-3-修改-roomsnapshotbuilder-snapshot-含真实-pet-currentstate.md`（server 端三处 pet.currentState 切真实值）
- `_bmad-output/implementation-artifacts/14-4-pet-state-changed-ws-广播.md`（server 端 pet.state.changed WS 广播实装）
- `_bmad-output/implementation-artifacts/decisions/0010-iphone-appstate.md`（AppState 注入规则 / memberPetStates 字段归属）
- `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md` §3.1（测试栈钦定 XCTest only）

### Source tree 涉及位置

```
iphone/
  PetApp/
    Core/
      Networking/
        WebSocketClient.swift          # RoomSnapshotPet.currentState 字段就位，本 story 不动
        WSMessageCodec.swift           # codec 层不动
        WebSocketClientMock.swift      # 测试中复用 .emit(_:)
      Motion/
        MotionState.swift              # .rest/.walk/.run enum，本 story 不动（AC2 在 View 层桥接）
    Features/
      Home/
        Models/
          HomeData.swift               # HomePetState enum + 新增 .motionState computed property（Task 2.1）
        Views/
          PetSpriteView.swift          # Story 8.4 落地，本 story 复用渲染组件，不动内部
      Room/
        Models/
          RoomMember.swift             # 不动（不扩展 petState 字段 — 避免与 memberPetStates map 双 source of truth）
        ViewModels/
          RoomViewModel.swift          # 基类 memberPetStates: [String: HomePetState] 字段，不动
          RealRoomViewModel.swift      # 核心修改：applySnapshot / applyMemberJoined 写入 memberPetStates
          MockRoomViewModel.swift      # 可能修改：UITest 路径设置 memberPetStates
        Views/
          RoomScaffoldView.swift       # 核心修改：memberRow 增加 PetSpriteView 渲染
    App/
      RootView.swift                   # 可能修改：UITEST_ROOM_THREE_MEMBERS 路径设置 memberPetStates
  PetAppTests/
    Features/
      Room/
        RealRoomViewModelTests.swift   # 扩展（AC3 新增 ≥4 case）
  PetAppUITests/
    RoomUITests.swift                  # 扩展（AC4 新增 1 case）
```

### Testing 标准摘要

- **单测**（PetAppTests target）：XCTest only；`@MainActor` 标注测试 class；`WebSocketClientMock.emit(_:)` 驱动 stream + `await Task.yield()` 等派发；断言 `vm.memberPetStates` dict 内容
- **UITest**（PetAppUITests target）：XCUITest only；launch argument 路径注入 MockRoomViewModel + 预设 memberPetStates；定位 `petSprite_rest/walk/run` a11y identifier
- **build verify**：`bash iphone/scripts/build.sh --test` 全绿

### Project Structure Notes

- **不扩展 RoomMember struct 字段**：避免与 `memberPetStates` map 形成双 source of truth（与 Story 12.3 开放问题 §5 决策一致）
- **HomePetState 桥接 MotionState**：在 HomePetState 上加 computed property 而非在 ViewModel 层做映射，保持 View 层传参干净（`state.memberPetStates[id]?.motionState ?? .rest`）
- **PetSpriteView 缩放**：Story 8.4 PetSpriteView 内 spriteImage frame 180x180pt 是 HomeView catStage 尺寸；在 RoomScaffoldView memberRow 内需要 caller 侧 `.frame(width: 40, height: 40)` + `.clipped()` 缩小（或用 `.scaleEffect()` + `.frame()` 组合）

### Previous story intelligence（必读 lessons）

1. **Story 12.3 关键决策**："memberPetStates 节点 4 阶段空 map"守护——本 story 正式解禁该守护，切换为真实写入
2. **Story 12.1 fix-review r3**（stale-snapshot-discard-by-room-id）——applySnapshot 内 room.id 校验仍保留，本 story 不动该守护
3. **Story 12.1 fix-review r4**（snapshot-host-must-not-infer-from-position）——isHost 严格 false 不动
4. **Story 8.4 关键实装**：PetSpriteView 是 stateless representation（`PetSpriteView(state: MotionState)`），接 MotionState 直接渲染；`.id(state)` + `.transition(.opacity)` + `.animation(.easeInOut(duration: 0.2), value: state)` 三件套确保状态切换平滑过渡
5. **Story 8.4 review lesson**（swiftui-content-swap-needs-id-and-transition）——PetSpriteView 内部已自带 `.id(state)` + `.transition(.opacity)` + `.animation()`，caller 不需要再加动画 modifier
6. **Story 14.3 / 14.4 落地后**：server 端权威等价桶四处就绪（room.snapshot / GET /rooms / member.joined / pet.state.changed），pet.currentState 自此为真实值而非 placeholder 1

### Lessons reading list（dev 实装时必读）

`docs/lessons/` 内本 story 必读：

- `2026-05-09-stale-snapshot-discard-by-room-id-12-1-r3.md`（room.id 校验路径不动但需了解）
- `2026-05-04-swiftui-content-swap-needs-id-and-transition.md`（PetSpriteView 动画三件套原理）
- `2026-05-12-nil-deref-defense-and-integration-evidence-14-3-r1.md`（*int8 nil-deref 防御 + 区别 fixture 模式）

### References

- [Source: docs/宠物互动App_总体架构设计.md] — iOS Swift+SwiftUI / WebSocket
- [Source: docs/宠物互动App_V1接口设计.md#12.3] — room.snapshot 字段表 + client merge contract + pet.currentState 枚举 1/2/3
- [Source: docs/宠物互动App_V1接口设计.md#5.2] — pet.currentState 权威等价桶四处枚举
- [Source: _bmad-output/planning-artifacts/epics.md] — Epic 15 Story 15.1 acceptance criteria（行 2369-2387）
- [Source: _bmad-output/implementation-artifacts/12-3-房间快照解析-成员列表渲染.md] — Story 12.3 落地（snapshot 解析 + memberPetStates 空 map 守护 + PetSpriteView 未在 RoomScaffoldView 使用）
- [Source: _bmad-output/implementation-artifacts/8-4-主界面猫-sprite-三态动画切换.md] — Story 8.4 PetSpriteView 实装
- [Source: _bmad-output/implementation-artifacts/14-3-修改-roomsnapshotbuilder-snapshot-含真实-pet-currentstate.md] — Story 14.3 server 三处 pet.currentState 切真实
- [Source: _bmad-output/implementation-artifacts/14-4-pet-state-changed-ws-广播.md] — Story 14.4 pet.state.changed WS 广播
- [Source: _bmad-output/implementation-artifacts/decisions/0010-iphone-appstate.md#3.1] — AppState 注入规则
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.1] — 测试栈钦定 XCTest only
- [Source: iphone/PetApp/Core/Networking/WebSocketClient.swift#170-178] — RoomSnapshotPet struct（currentState: Int）
- [Source: iphone/PetApp/Features/Home/Models/HomeData.swift#120-124] — HomePetState enum（Int rawValue 1/2/3）
- [Source: iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift#42] — memberPetStates: [String: HomePetState]
- [Source: iphone/PetApp/Features/Home/Views/PetSpriteView.swift] — PetSpriteView(state: MotionState) 完整实装
- [Source: iphone/PetApp/Core/Motion/MotionState.swift] — MotionState enum（.rest/.walk/.run + wireValue 1/2/3）
- [Source: iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift#685-717] — applySnapshot 现有实装（memberPetStates 空 map 注释行 713-714）
- [Source: iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift#738-771] — applyMemberJoined 现有实装
- [Source: iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift#279-340] — membersList + memberRow 现有渲染

### Latest tech information

- **SwiftUI `.frame()` modifier 缩放**：PetSpriteView 内 spriteImage 使用 `.resizable().scaledToFit().frame(width: 180, height: 180)`；在 RoomScaffoldView caller 侧用 `.frame(width: 40, height: 40).clipped()` 可以覆盖内部 frame（SwiftUI 的 frame modifier 在最后应用的优先）
- **HomePetState / MotionState 双枚举**：项目存在两个枚举映射相同业务概念——`HomePetState(rawValue: Int)` 从 server wire 解析、`MotionState(rawValue: String)` 从 CoreMotion 解析。两者 case 名相同但 rawValue 类型不同（Int vs String）。房间场景用 HomePetState 解析 server wire → 桥接 MotionState 传给 PetSpriteView 是正确路径（不新建第三个枚举）
- **Swift Concurrency**：`@MainActor` + `ObservableObject` + `@Published` 模式（与 Story 12.1 / 12.3 一致）；不采用 iOS 17+ `@Observable` macro

### Project context reference

`_bmad-output/implementation-artifacts/decisions/` 内本 story 必读 ADR：

- `0010-iphone-appstate.md` — AppState 单 source of truth 规则（memberPetStates 是 ViewModel transient 字段，不入 AppState 白名单 7 字段）
- `0002-ios-stack.md` — 测试栈 XCTest only
- `0009-ios-stack.md` — iPhone 工程目录决策（导航架构）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7 (1M context)

### Debug Log References

- `bash iphone/scripts/build.sh --test`：iPhone 单元测试 575 tests passed (exit 0)
- `bash iphone/scripts/build.sh`：xcodebuild 编译通过 (exit 0)
- `xcodebuild test -only-testing:PetAppUITests/RoomUITests/testMemberRowsRenderPetSpriteViewsWithDistinctAccessibilityIdentifiers`：新增 UI 测试 passed
- ios-simulator MCP 实跑验证：iPhone 17 Pro 模拟器进 UITEST_ROOM_THREE_MEMBERS=1 路径，截屏确认 3 成员行右侧 PetSpriteView 渲染 SF Symbol 三态（cat.fill / figure.walk / figure.run）各自不同

### Completion Notes List

- **AC1 落地**：`RealRoomViewModel.applySnapshot` 与 `applyMemberJoined` 都按 AC1 钦定规则解析 `pet.currentState` 写入 `memberPetStates`：pet ≠ nil + 已知 currentState → 对应状态；未知值 → fallback `.rest` + `os_log(.error)`；pet == nil → 默认 `.rest`. snapshot 路径用"整体替换 (`memberPetStates = newPetStates`)"语义，自然实现"snapshot 中不存在的 userId 移除"等价于 `removeValue`. 新增 `applyJoinedPetState(userId:pet:)` private helper 复用规则.
- **AC2 落地**：`HomePetState` 增加 `motionState: MotionState` 桥接 computed property（`.rest/.walk/.run` 1:1 映射）. `RoomScaffoldView.memberRow` 用 `PetSpriteView(state: (state.memberPetStates[member.id] ?? .rest).motionState).frame(width: 40, height: 40).clipped()` 替换原 paw icon. 关键 dev 决策：必须在 memberRow 外层加 `.accessibilityElement(children: .contain)`，否则 SwiftUI 把 PetSpriteView 的 a11y leaf 合并到父 `roomMember_N` identifier（与 HomeView.catStage line 288-289 同精神）.
- **AC3 落地**：新增 4 个单元测试 case（`testSnapshotPetCurrentStateOneTwoThreeMapsToRestWalkRun` / `testSnapshotUnknownPetCurrentStateFallsBackToRest` / `testSnapshotPetlessMemberDefaultsToRest` / `testRepeatedSnapshotUpdatesMemberPetStates`），覆盖 epics.md §15.1 行 2382-2386 钦定的 4 个 case. 同时更新两个既有测试断言（`testRoomSnapshotMessagePopulatesMembers` 与 `testRoomSnapshotIsIdempotentOnRepeatedEmit`）中关于"memberPetStates 节点 4 阶段保持空 map"的旧期望 → 改为按本 story 真实写入语义（所有 currentState=1 → `.rest`，pet=nil → fallback `.rest`）.
- **AC4 落地**：新增 `testMemberRowsRenderPetSpriteViewsWithDistinctAccessibilityIdentifiers` UITest case；在 `RootView.init()` 的 `UITEST_ROOM_THREE_MEMBERS=1` 分支后注入 `mock.memberPetStates = ["u_alice": .rest, "u_bob": .walk, "u_charlie": .run]`，让 UITest 验证 `petSprite_rest` / `petSprite_walk` / `petSprite_run` 三个 a11y identifier 各自可定位.
- **AC5 落地**：unit tests 全绿（575 tests / 0 failures）+ xcodebuild 编译通过 + ios-simulator MCP 实跑视觉验证三成员行右侧 sprite 各异.
- **AC6 落地**：5 个 deliverable 文件 + 1 个可能修改文件（RootView.swift 注入 memberPetStates）全部落地.

### File List

- `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` — applySnapshot 真实写入 memberPetStates；applyMemberJoined 写入 memberPetStates；新增 `applyJoinedPetState` helper；更新注释（标注"自 Story 15.1 起切真实值"）.
- `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift` — memberRow 用 PetSpriteView 替换 paw icon；外层加 `.accessibilityElement(children: .contain)` 让 PetSpriteView a11y identifier 可被 UITest 定位.
- `iphone/PetApp/Features/Home/Models/HomeData.swift` — HomePetState 新增 `motionState: MotionState` computed property 桥接.
- `iphone/PetApp/App/RootView.swift` — UITEST_ROOM_THREE_MEMBERS=1 路径下额外注入 mock.memberPetStates（三成员各持 .rest/.walk/.run）以支持 AC4 UITest.
- `iphone/PetAppTests/Features/Room/RealRoomViewModelTests.swift` — 新增 AC3 钦定的 4 个测试 case；更新既有 2 个测试的 memberPetStates 断言（从"保持空 map"改为"按 wire 值真实写入"）.
- `iphone/PetAppUITests/RoomUITests.swift` — 新增 AC4 钦定的 1 个 UITest case 验证 PetSpriteView 三态 a11y identifier 可定位.

### Change Log

| 日期 | 操作 | 内容 |
|------|------|------|
| 2026-05-12 | create-story | Story 15.1 上下文引擎分析完成——综合 6 条相关 story 实装记录 + iOS 现有代码结构 + server 权威等价桶四处全就绪状态，创建全面开发指南 |
| 2026-05-12 | dev-story | Story 15.1 全部 6 个 task / 13 个 subtask 落地完成；575 unit tests 全绿 + xcodebuild 编译通过 + ios-simulator MCP 实跑视觉验证；新增 UITest case 单跑 passed. status → review |
