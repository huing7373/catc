# ADR-0010: iPhone 引入全局 AppState 单 source of truth（**完全 supersede** Story 5.5 数据持有部分）

- **Status**: Accepted（2026-04-30 Story 37.2 落地）
- **Date**: 2026-04-29 / 2026-04-30 v2 (X1+X2 修订：partial revert → completely supersedes)
- **Decider**: Developer
- **Supersedes**: Story 5.5 已 done 决策的「`HomeViewModel.homeData` 持有 user/pet/stepAccount/chest/room」**数据持有部分**（**完全 supersede 而非修订**——该字段 acceptance 不再 active；LoadHomeUseCase / HomeRepository 不变，仅 hydrate 目标改 AppState）
- **Related**: ADR-0009（导航架构 TabView，本 ADR 联动）；Story 37.2（本 ADR）；Story 37.4（实装 AppState + 迁移）；Story 12.7 / 24.1 / 27.1 / 35.x（下游 ViewModel 改用 AppState）

---

## 1. Context

### 1.1 现状

Story 5.5「LoadHomeUseCase + 主界面用 GET /home 一次拉取全部数据」已 done，钦定的数据流是：

- `LoadHomeUseCase` 调 `GET /home` 拿到 `HomeData`（含 user / pet / stepAccount / chest / room.currentRoomId）
- `HomeData` hydrate 到 `HomeViewModel.homeData`
- 主界面（HomeView）从 `HomeViewModel.homeData` 读取所有数据
- 后续 epic 增量字段（chest 状态、装备、房间）也走 `HomeViewModel.homeData`

物理产物（已落地）：
- `iphone/PetApp/Features/Home/UseCases/LoadHomeUseCase.swift`
- `iphone/PetApp/Features/Home/Repositories/HomeRepository.swift`
- `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift` 持 `homeData: HomeData?`

### 1.2 矛盾点

ADR-0009 引入 4 Tab IA 后，5 个 ViewModel（HomeViewModel / WardrobeViewModel / FriendsViewModel / ProfileViewModel / RoomViewModel）都需要访问相同的 domain state：

| Domain State | 哪些 ViewModel 需要 |
|---|---|
| currentUser（用户名 / ID / 称号 / 等级） | HomeViewModel（猫主信息）、ProfileViewModel（资料展示）、FriendsViewModel（"我的房间"提示条） |
| currentPet（猫名 / 等级 / 状态条 / equips） | HomeViewModel（CatStage）、WardrobeViewModel（预览区猫）、ProfileViewModel（统计-猫等级） |
| currentRoomId | HomeContainerView（互斥状态机）、RoomViewModel、FriendsViewModel（"我的房间"卡）、JoinRoomModal、ShareLinkUseCase |
| currentInventory（节点 8+） | WardrobeViewModel、HomeViewModel（CatStage 装备显示） |
| currentChest（节点 7+） | HomeViewModel（ChestCardView） |
| currentStepAccount | HomeViewModel（StatusBar 步数）、OpenChestUseCase（开箱前校验）|

**问题**：

1. **若每个 ViewModel 各持一份 domain state** → N 份冗余数据；server 推一次 update 要写 N 份；漂移雷
2. **若 HomeViewModel 持有全部** → HomeViewModel 变成事实上的 god object；其它 ViewModel 还要向它要数据，跨 Feature 反向依赖
3. **WS 推送（pet.state.changed / member.joined）需要全局广播** → 没有单一的 mutation 入口，每条 WS 消息要找谁更新

Sprint Change Proposal v1 提议的「全局 AppState」方案被 codex 审 v1 BLOCKER 4 命中：v1 没说清楚 AppState 与现 `HomeViewModel.homeData` + `SessionStore` 的关系，造成至少 3 份状态共存（AppState / HomeViewModel.homeData / RoomViewModel）。

### 1.3 用户决议

2026-04-29 用户在 Sprint Change Proposal v2 前置决议中选 #5=B：**新建 AppState 顶替 HomeViewModel.homeData**。

本 ADR 落实该决议，定义 AppState 与现有 SessionStore / HomeViewModel / LoadHomeUseCase 的边界。

---

## 2. Decision Summary

| 领域 | 选定 |
|---|---|
| **AppState 类型** | `@MainActor final class AppState: ObservableObject` 全局单实例 |
| **AppState 范围** | Domain state（user / pet / stepAccount / chest / currentRoomId / inventory / equips） |
| **AppState 不含** | UI transient state（current Tab、sheet open state、loading、error toast）→ 留给 SwiftUI `@State` 或 ViewModel-specific transient |
| **hydrate 入口** | `LoadHomeUseCase` 完成后 → `appState.hydrate(homeData)`；WS 消息 → `appState.apply(wsMessage)`；REST mutation 接口（equip/unequip/chest open）成功后 → `appState.update*(...)` |
| **ViewModel 角色** | 退化为「view-specific 投影 + 行为」；**仅允许构造注入** AppState（不允许 `@EnvironmentObject` 反模式，详见 §3.1 注入规则） |
| **当前 Tab 所有权** | **由 `AppCoordinator.currentTab` 持有**（不进 AppState）；与 `AppCoordinator.presentedSheet` 同级（参见 ADR-0009 §3.4） |
| **与 SessionStore 关系** | 并行：SessionStore 是 auth/session 边界；AppState 是 domain 边界 |

---

## 3. Decisions

### 3.1 AppState 类型与生命周期

- **选定**：`@MainActor final class AppState: ObservableObject` 全局单实例（`@StateObject` 在 RootView 持有）
- **生命周期**：与 App 同周期；用户登出 / 重置身份时 `appState.reset()` 清空
- **注入规则**（**ADR 级硬规则**，违反触发 codex review reject）：
  - **View 层**：通过 `.environmentObject(appState)` 在 RootView 注入；子视图用 `@EnvironmentObject var appState: AppState` 读
  - **ViewModel 层**：**只允许构造注入** `AppState`；**禁止** ViewModel 内部用 `@EnvironmentObject` 读（@EnvironmentObject 是 SwiftUI View 专属属性包装；ViewModel 是普通 class，用 @EnvironmentObject 落地变成半 View 半 VM 的怪物，且无法被单元测试 mock）
  - **ViewModel 构造模式**：`init(appState: AppState, ...)` 或 `bind(appState: AppState, ...)`；MockViewModel 时注入 MockAppState
  - View 通过 `@StateObject`（持有生命周期）或 `@ObservedObject`（外部传入）持 ViewModel；ViewModel 持 AppState 引用
  - 例外：纯展示性 SwiftUI View（无 ViewModel）可以直接 `@EnvironmentObject AppState` 读 domain 数据

- **理由**：
  1. **SwiftUI 原生**：`ObservableObject` + `@Published` + `@EnvironmentObject` 是 SwiftUI 数据流官方推荐；`@Observable` (iOS 17+) 更新潮但 deployment target 17.0 已支持，未来可平滑迁移
  2. **MainActor 约束**：domain state 在 UI 上消费，所有 mutation 走 MainActor 避免数据竞争；与 ADR-0002 §3.2 异步测试方案 strict concurrency 对齐
  3. **单实例 vs 多实例**：domain state 全局唯一（一个用户、一只猫、一个 token），不存在多实例需求；`@StateObject` 在 RootView 持有保证生命周期与 App 一致

- **否决候选**：
  - **`@Observable` (iOS 17+ Macros)**：暂不选 — 当前项目代码风格已用 `@Published` (Story 5.5 内 HomeViewModel 也用)；保持一致更新成本低；未来 Swift 6.x 全面切 `@Observable` 时单独 spike
  - **Singleton (`AppState.shared`)**：否决 — 测试不友好（无法注入 mock）；SwiftUI 推荐 environment 模式
  - **每 Feature 一个 ObservableObject + 跨 Feature 通过 NotificationCenter / Combine 同步**：否决 — N 份状态 + 同步逻辑膨胀；正是要避免的反模式

### 3.2 AppState 范围（白名单）

**含**（domain state）：

```swift
@MainActor
final class AppState: ObservableObject {
    @Published var currentUser: User?         // 节点 2 起
    @Published var currentPet: Pet?            // 节点 2 起
    @Published var currentStepAccount: StepAccount?  // 节点 2 起（节点 3 后含真实步数）
    @Published var currentChest: Chest?        // 节点 2 起占位，节点 7 起真实
    @Published var currentRoomId: String?      // 节点 2 起 nil，节点 4 起真实；类型与 server /home `room.currentRoomId` 字符串契约对齐（AR21 ID 字符串约定，参见 epics.md L2016）
    @Published var currentInventory: [CosmeticInstance] = []  // 节点 8 起
    @Published var currentEquips: Equipment = .empty  // 节点 9 起
    @Published var emojiCatalog: [EmojiConfig] = []  // 节点 6 起；表情**配置目录**（系统固定集合，App 启动时拉一次缓存），非房间内 active emoji 队列；后者属于 RoomViewModel transient
}

// Friends 数据归属：Friends Tab 数据（在线列表 / 状态文字）是 tab-specific cache，不进 AppState；
// 由 FriendsViewModel 自己拉 + 缓存（节点 4 后接 GET /friends 接口；本期 Scaffold 用 mock）。
// 理由：好友状态强 transient（5s WS 心跳变化），且非"我的"domain。
```

**不含**（UI / transient）：

| 状态 | 持有方 | 理由 |
|---|---|---|
| 当前 Tab（Home/Wardrobe/Friends/Profile） | **`AppCoordinator.currentTab: Tab` @Published** | UI 状态，不属 domain；放 AppCoordinator 与 `presentedSheet` 同级，方便深 link / 跨 ViewModel 程式化切 Tab；不进 AppState（保持 AppState 仅 domain）。详见 ADR-0009 §3.4 |
| Sheet 是否打开 | `AppCoordinator.presentedSheet`（次级 sheet）/ ViewModel `@Published var showXModal` | UI 状态 |
| Loading / error toast | ViewModel `@Published var loadingState: LoadingState` | view-specific transient |
| WS 连接状态 (connected/reconnecting/disconnected) | `RoomViewModel.wsState` | view-specific（WS 仅 Room 用） |
| 表单输入（JoinRoomModal 输入框） | `JoinRoomModal` `@State var input: String` | 临时输入 |
| 倒计时秒数（chest unlock countdown） | `HomeViewModel.chestRemainingSeconds`（Timer 驱动） | view-specific transient（domain `currentChest.unlockAt` 是绝对时间，秒级减是 view 行为） |

**理由**：domain state 是「由 server 决定的事实」（user 是谁、有哪些 cosmetic）；UI state 是「视觉/交互临时态」。混在一起会让 reset 边界模糊。

### 3.3 hydrate 流程

#### 启动 / 自动登录后

```
GuestLoginUseCase 完成 (Story 5.2)
  → LoadHomeUseCase 调 GET /home (Story 5.5)
  → 拿到 HomeData
  → appState.hydrate(homeData)
      • appState.currentUser = homeData.user
      • appState.currentPet = homeData.pet
      • appState.currentStepAccount = homeData.stepAccount
      • appState.currentChest = homeData.chest
      • appState.currentRoomId = homeData.room.currentRoomId
  → AppLaunchStateMachine 切到 .ready
  → MainTabView 出现，HomeContainerView 根据 currentRoomId 决定显示 HomeView 还是 RoomView
```

#### WS 消息（节点 4+）

```
WS recv room.snapshot
  → RoomViewModel.handleSnapshot(snapshot)
  → 不写 AppState（snapshot 是 RoomViewModel 私有 view state）

WS recv pet.state.changed { userId: self.userId, currentState }
  → appState.updateMyPetState(currentState)  // 自己的猫
  → 跨 Tab 联动（Home Tab CatStage、Wardrobe Tab 预览区都看到）

WS recv pet.state.changed { userId: other, currentState }
  → RoomViewModel.updateMemberPetState(userId, currentState)  // 别人的猫
  → 不写 AppState（别人的状态不属于"我的 domain"）

WS recv member.joined / member.left
  → RoomViewModel.handleMemberEvent(...)
  → 不写 AppState
```

#### REST mutation

```
EquipUseCase POST /cosmetics/equip 成功
  → response 含 newEquips
  → appState.currentEquips = newEquips
  → Home Tab CatStage / Wardrobe Tab 预览区自动刷新（@Published 重渲染）

OpenChestUseCase POST /chest/open 成功
  → response 含 nextChest + reward + newStepBalance
  → appState.currentChest = response.nextChest
  → appState.currentStepAccount.balance = response.newStepBalance
  → appState.currentInventory.append(response.reward)  // 节点 8+ 入仓
  → 全 Tab 联动

CreateRoomUseCase POST /rooms 成功
  → response 含 `roomId: String`（AR21 ID 字符串约定）
  → appState.currentRoomId = roomId
  → HomeContainerView 自动切到 RoomView（互斥状态机）

LeaveRoomUseCase POST /rooms/{id}/leave 成功
  → appState.currentRoomId = nil
  → HomeContainerView 自动切回 HomeView
```

### 3.4 与 SessionStore 关系（明确边界）

| 边界 | SessionStore | AppState |
|---|---|---|
| **职责** | auth / session 凭据 | domain data |
| **字段** | guestUid / token / tokenExpiresAt | user / pet / inventory / chest / roomId / equips / emojis |
| **持久化** | Keychain（Story 5.1） | 不持久化（每次启动 re-hydrate via LoadHomeUseCase） |
| **生命周期** | 跨进程持久；用户主动退出 / 401 时清 | 与 App 进程同生死；登出时 reset() |
| **mutation 来源** | 登录接口、token 刷新、重置身份 | LoadHomeUseCase / WS / REST mutation 接口 |

**两者并行**：APIClient 用 SessionStore 取 token；Repository 拿响应后写 AppState；ViewModel 从 AppState 读 domain。

### 3.5 ViewModel 演变模式

| ViewModel | 持有字段（Story 37 后） | 数据来源 |
|---|---|---|
| **HomeViewModel** | `chestRemainingSeconds: Int`（Timer 驱动）、`interactionAnimation: AnimationState`（喂食/抚摸/玩耍 floatUp） | domain 数据（user/pet/chest）从**构造注入的 AppState** 读；不再持 homeData |
| **RoomViewModel** | `members: [Member]`（WS snapshot）、`wsState: WSState`、`memberPetStates: [UserId: PetState]` | roomId 从构造注入的 AppState 读；其余字段是 WS 视图态 |
| **WardrobeViewModel** | `selectedCategory: Category`、`selectedCosmeticId: String?` | inventory / equips 从构造注入的 AppState 读 |
| **FriendsViewModel** | `selectedTab: FriendTab`、`friends: [Friend]`（节点 4 后真实） | `myRoomId` 从 AppState.currentRoomId 派生 |
| **ProfileViewModel** | `showWeChatModal: Bool` | user 从构造注入的 AppState 读 |
| **LaunchingViewModel** | n/a（Story 2.9 已 done，目前内嵌在 AppLaunchStateMachine） | 不直接读 AppState；启动 bootstrap 完成后由 LoadHomeUseCase 写 AppState |
| **ResetIdentityViewModel** | dev 按钮触发态（Story 2.8） | 调 `appState.reset()` + Keychain 清空；构造注入 AppState |

**HomeViewModel 关键变化**（Story 37.4 落地步骤）：

- **删除**：`@Published var homeData: HomeData?` 字段
- **保留**：`bind(pingUseCase:loadHomeUseCase:)` 方法签名（不变 Story 5.5 wire 模式）
- **改动**：`bind` 内部调 `loadHomeUseCase.execute()` 拿到 `HomeData` 后，**不再** `self.homeData = homeData`，而是 `self.appState.hydrate(homeData)`
- **新增**：HomeViewModel 持 `appState` 引用（构造注入）

### 3.6 测试影响

- **AppState 测试**：新增 `AppStateTests.swift`，覆盖 hydrate / reset / mutation 各路径
- **ViewModel 测试简化**：注入 `MockAppState`（继承 AppState 直接 set 字段），不再注入完整 `HomeData`
- **集成测试**：现 `LoadHomeUseCase` 集成测试断言改为「hydrate 后 appState.currentUser != nil」而非「homeViewModel.homeData != nil」

### 3.7 Reset 流程（用户主动登出 / 重置身份）

```
ResetIdentityViewModel.resetTapped() (Story 2.8 dev 按钮)
  → KeychainStore.removeAll()       (现有逻辑)
  → SessionStore.reset()              (现有逻辑)
  → appState.reset()                  (新增)
      • currentUser = nil
      • currentPet = nil
      • currentRoomId = nil
      • currentInventory = []
      • ...全部置 nil/empty
  → AppLaunchStateMachine.bootstrap() 重新启动
  → 重新走 GuestLoginUseCase + LoadHomeUseCase + appState.hydrate
```

---

## 4. Consequences

### 4.1 对 Story 5.5 的影响：completely supersedes 数据持有部分（不是 partial revert）

> **2026-04-30 X1+X2 措辞强化**：本节原使用 "partial revert" 表述（暗含"现有代码保留 + 局部修改"语义），但实装路径是「**重新实装**：HomeViewModel.homeData 字段直接删除；caller 漏改靠**编译器报错**驱动；不做"全 codebase grep `homeViewModel.homeData` 引用"这种妥协式校验」。Story 5.5 的数据持有部分钦定**已 dead**——sprint-status.yaml 内 `5-5-...` 状态改 superseded（见提案 v2 §4 提案 ④）。

**保留（独立决策延续，非 Story 5.5 acceptance 的一部分）**：
- LoadHomeUseCase 接口签名（输入无参 / 输出 HomeData）
- HomeRepository
- GET /home 调用契约
- LoadHomeUseCase 集成测试基本结构

**Supersede（Story 5.5 该部分钦定不再 active）**：
- 旧路径：`LoadHomeUseCase → HomeViewModel.homeData (字段持 user/pet/stepAccount/chest/room)`
- 新路径：`LoadHomeUseCase → appState.hydrate(homeData)`；HomeViewModel.homeData **字段不存在**；主界面数据读取一律走 `appState.*`

**Git 历史**：完整保留（commit 不可逆）。Story 5.5 sprint-status 改 **`superseded`** 状态——done 的 commit 保留作为历史记录，但其数据持有部分的 acceptance 不再作为 active spec；新实装见 Story 37.4。**不走 "partial revert + 渐进迁移 + grep 兜底"路径**——彻底重写。

### 4.2 对 ADR-0009 的联动

- TabView 4 Tab 间数据共享天然走 AppState（无需在 RootView 写数据中转层）
- HomeContainerView 互斥状态机直接读 `appState.currentRoomId`
- Wardrobe / Friends / Profile Tab 各自从 AppState 读所需 slice

### 4.3 对未来 epic 的影响

| 节点 | AppState 字段填充 |
|---|---|
| 节点 2（已 done） | currentUser / currentPet / currentStepAccount / currentChest / currentRoomId（initial 值） |
| 节点 3 | currentStepAccount 增量更新（POST /steps/sync 后） |
| 节点 4 | currentRoomId hydrate / WS member 事件处理 |
| 节点 5 | currentPet.currentState（WS pet.state.changed 后） |
| 节点 6 | emojiCatalog（GET /emojis 启动时拉一次缓存配置目录） |
| 节点 7 | currentChest 状态变化 + currentStepAccount 扣减 |
| 节点 8 | currentInventory（GET /cosmetics/inventory） |
| 节点 9 | currentEquips（POST /cosmetics/equip 后） |
| 节点 10 | currentEquips 含 renderConfig 字段 |
| 节点 11 | currentInventory 增删（合成消耗 + 产出）|
| 节点 12 | 不影响 AppState |

### 4.4 已知坑

- **AppState 体积膨胀**：缓解 — §3.2 白名单严格限制；MR review 检查；未来按 Feature 拆 sub-state（如 `appState.cosmetics: CosmeticsState`）作为 fallback
- **MainActor 强制**：所有 mutation 必须在 MainActor；后台线程拿到 server 响应需 `await MainActor.run { appState.update... }`
- **测试 mock**：MockAppState 继承 AppState 实现需要 testing helper（在 Story 37.4 同步落 `Tests/Helpers/AppStateTestHelpers.swift`）
- **跨 ViewModel 联动**：当 EquipUseCase 写 AppState 后，HomeViewModel 的 CatStage 需自动刷新——View 层用 `@EnvironmentObject AppState` + SwiftUI `@Published` 天然重渲染；ViewModel 层用 Combine `sink` 订阅 `appState.objectWillChange`（构造注入的 AppState 引用）；不在 ViewModel 内用 @EnvironmentObject

---

## 5. Post-Decision TODO

- [ ] **Story 37.2**：本 ADR 撰写 + spike
- [ ] **Story 37.4**：实装 AppState 类 + 迁移 HomeViewModel.homeData → AppState；改 LoadHomeUseCase chain；ResetIdentityViewModel 加 appState.reset()
- [ ] **Story 37.7-37.11**：每个 Scaffold ViewModel 用**构造注入** AppState 模式（`init(appState: AppState)`）；Mock 时注入 MockAppState；View 层（非 ViewModel）才允许 `@EnvironmentObject`
- [ ] **Story 12.7 改写**：CreateRoom/Join/Leave UseCase 写 AppState.currentRoomId
- [ ] **Story 14.x / 15.x（节点 5）**：pet.state.changed WS 消息处理走 appState.updateMyPetState
- [ ] **Story 21.3（开箱）**：OpenChestUseCase 成功后写 appState.currentChest + currentStepAccount
- [ ] **Story 24.2（仓库 inventory）**：LoadInventoryUseCase 成功后写 appState.currentInventory
- [ ] **Story 26.x / 27.x（穿戴）**：EquipUseCase / UnequipUseCase 成功后写 appState.currentEquips
- [ ] **iphone/README.md 同步**：在「测试依赖」段后追加「全局状态」段引用本 ADR
- [ ] **CLAUDE.md 同步**：本 ADR 不直接影响 CLAUDE.md（CLAUDE.md 不锁 iOS 状态架构）

---

## 6. 验收（本 ADR 改 Accepted 的标准）

> **路径 B 验证模型说明**：本节 checkbox 在 ADR Accepted 时勾选，记录 Sprint Change Proposal v2.5 终审时的 architect/PM **契约级签字** = checkbox 验证语义的等价物。下表第 2-4 项的"Story 37.4 落地后…"等措辞描述的是**契约级前置依赖已确认**（架构决策已 freeze、解锁下游开工），**不是**已物理验证。物理验证（build + test 通过、AppStateTests / LoadHomeUseCase 集成测试存在）由 Story 37.4 实装期 codex review 兜底；若届时发现 ADR-0010 §3 决策有偏差，走"ADR 修订 patch + 改 v2 Accepted"路径（参考 ADR-0008 v2 commit `ec5beb3` 先例）。本说明系 fix-review round 2 codex 担忧"future reviewer 把 ADR §6 当成 false-success signal、Story 37.4 跳过验证"的就地 forward reference；详见 `docs/lessons/2026-04-30-doc-tense-vs-path-b-adr-acceptance.md` 与本 lesson 文件 `docs/lessons/2026-04-30-adr-section-6-path-b-inline-semantics.md`。

- [x] 用户终审通过 Sprint Change Proposal v2
- [x] Story 37.4 落地后跑 `bash iphone/scripts/build.sh --test` 通过
- [x] AppStateTests.swift 含 ≥6 case（hydrate / reset / 各 update mutation）
- [x] LoadHomeUseCase 集成测试改为断言 appState.* 而非 homeViewModel.homeData
- [x] codex 对 Sprint Change Proposal v2 verdict ≥ Accept with revisions
