# Story 24.2: LoadInventoryUseCase + GET /cosmetics/inventory 调用

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As an iPhone 用户,
I want Wardrobe Tab 首次出现时立即从 server（`GET /api/v1/cosmetics/inventory`）加载我的最新装扮列表，每次进 Tab 都重拉不缓存,
so that 我看到的是真实持有数据（尤其开箱后立刻能看到新道具），加载中有 loading、失败可重试。

## 范围红线（必读 —— 与 Story 24.1 / 37.9 接续点严格对齐）

> **本 story 是「补上 Story 24.1 故意留空的上游触发 + 数据通道」的窄范围 story，不是建新页面、不改 sink、不改 mapping。**

Story 24.1 已完成「下游」：`RealWardrobeViewModel.subscribeInventory` 订阅 `appState.$currentInventory`（`[HomeEquip]`），closure 内做 `[HomeEquip] → [CosmeticItem]` 真实 mapping，空 → 空仓库 placeholder。**Story 24.1 显式声明**（24-1 文件 line 48-50 AC3）：「`LoadInventoryUseCase` 本体属 Story 24.2；本 story 不创建 UseCase/Repository/endpoint/DTO；不加 `.task`/`.onAppear` 触发 load」。本 story 正是接续这条红线，完成「上游写入 → 下游已就绪 sink 零 edit 反映」的闭环。

本 story 必须做的（且**仅**做的）：

1. 新建 inventory wire DTO（`InventoryResponse` envelope.data + `InventoryGroup` + `InventoryInstance`），严格对齐 V1 §8.2 已冻结 schema（group 聚合结构）。
2. 新建 `InventoryRepository`（封装 `GET /api/v1/cosmetics/inventory`，wire 直通，APIError 原样透传）+ `InventoryEndpoints` 工厂。
3. 新建 `LoadInventoryUseCase`（repo → wire DTO → 把 `groups[].instances[]` **展平**为 `[HomeEquip]` → 写 `appState.currentInventory`）。
4. `AppState` 新增单字段 mutation 入口 `applyInventory(_:)`（沿用 `apply*` 前缀 + 与 `applyCurrentChest` 同模式；ADR-0010 §3.3）。
5. `AppContainer` 新增 `makeInventoryRepository()` / `makeLoadInventoryUseCase(appState:)` 工厂（与 `makeChestRepository` / `makeLoadChestUseCase` 同模式）。
6. 在 **Wardrobe Tab 首次出现**触发点接 `LoadInventoryUseCase.execute()`：loading 显示 `ProgressView`、失败显示可重试 `RetryView`（复用 `ErrorPresenter` 既有机制）；**每次进 Tab 重拉不缓存**。

本 story **不**做（防 scope creep）：

- **不**改 `RealWardrobeViewModel.subscribeInventory` / `mapToCosmeticItems` / `rarity(forServerValue:)`（Story 24.1 已 final；本 story 写 `appState.currentInventory` 后零 edit 通过既有 sink 反映）。
- **不**改 `WardrobeScaffoldView` 视觉 / 布局 / a11y 锚 / `badgeText` 纯函数（Story 37.9 + 24.1 冻结）。
- **不**实装筛选（segment control）—— Story 24.3。
- **不**实装穿戴按钮行为 —— Story 24.5（占位预留）/ Story 27.1（真实）。
- **不**改 `MockWardrobeViewModel`（Preview / UITest / Mock 单测仍走 mock，不回归 37.9 视觉与 UITest 锚）。
- **不**反向修改 V1 §7.x / §8.x 接口文档（schema 已冻结，见 §1 冻结流程）。

## Acceptance Criteria

**权威来源**：epics.md §Story 24.2（line 3352-3372）+ V1 §8.2（line 1311-1445，schema 已冻结）+ Story 24.1 文件 AC3 接续红线（24-1 文件 line 46-50）+ Story 37.9 handoff「与 Story 24.1 衔接的红线」。

**Given** Story 24.1 `RealWardrobeViewModel.subscribeInventory` 真实派生已就绪（订阅 `appState.$currentInventory: [HomeEquip]`，空→空仓库 placeholder / 非空→`mapToCosmeticItems`）+ Story 37.4 `AppState.currentInventory` 字段就绪 + Story 23.4 server `GET /api/v1/cosmetics/inventory` 可用（契约 Epic 23 冻结）
**When** Wardrobe Tab 首次出现（用户切到「仓库」Tab）
**Then**：

1. **AC1 — Inventory wire DTO 严格对齐 V1 §8.2 冻结 schema**
   - 新建 `InventoryResponse`（envelope.data 形状 `{groups: [InventoryGroup]}`）：`groups` 是**非可选** `[InventoryGroup]`（V1 §8.2「空背包 `{groups: []}` 契约」+ 关键约束「防 Swift Codable 解析 groups 为 nil」—— 严格非可选解析，空时为 `[]`，缺字段 / `null` 时 APIClient 抛 `APIError.decoding` 走 fail-fast，与 `EmojiListResponse` 同精神）。
   - `InventoryGroup` 字段（严格对齐 V1 §8.2 响应体字段表 line 1366-1377，字段名 / 类型一字不差）：`cosmeticItemId: String` / `name: String` / `slot: Int` / `rarity: Int` / `iconUrl: String` / `assetUrl: String` / `count: Int` / `instances: [InventoryInstance]`。
   - `InventoryInstance` 字段：`userCosmeticItemId: String` / `status: Int`（V1 §8.2 仅可能为 `1=in_bag` / `2=equipped`，server 已过滤 `3=consumed`/`4=invalid`）。
   - DTO 仅 `Decodable, Equatable, Sendable`（与 `EmojiListResponse` / `ChestCurrentResponse` 同 conformance 模式）；**不**在 DTO 层做排序 / 去重 / 校验（V1 §8.2 步骤 5 已契约保证两级确定性全序，client 直通；与 `EmojiRepository` 注释「server 已保证顺序，repo 只 wire→domain 直通」同精神）。
2. **AC2 — `InventoryRepository` 封装 GET /cosmetics/inventory（wire 直通，APIError 原样透传）**
   - 新建 `InventoryRepositoryProtocol`（`: Sendable`）+ `DefaultInventoryRepository`（`struct`，value type 无内部状态，与 `DefaultChestRepository` / `DefaultEmojiRepository` 同模式）。
   - 方法 `func fetchInventory() async throws -> InventoryResponse`：调 `apiClient.request(InventoryEndpoints.inventory())`；APIError **原样透传**（不在 repo 层吞错 / 转码 / 二次排序）。
   - 新建 `InventoryEndpoints.inventory()`：`Endpoint(path: "/api/v1/cosmetics/inventory", method: .get, body: nil, requiresAuth: true)`（path 必含 `/api/v1` 前缀，lesson `2026-04-26-baseurl-host-only-contract.md`；`requiresAuth=true` → 经 `AuthBoundaryAPIClient` 自动注 token + 拦 401，与 `EmojisEndpoints` 同模式；GET 无 body / 无 query 参数 —— V1 §8.2「Query 参数：无」）。
3. **AC3 — `LoadInventoryUseCase`：repo → wire DTO → 展平为 `[HomeEquip]` → 写 `appState.currentInventory`**
   - 新建 `LoadInventoryUseCaseProtocol`（`: Sendable`，`func execute() async throws`）+ `DefaultLoadInventoryUseCase`（`struct`，无内部状态，**不**缓存 —— epics.md line 3367「每次打开都重新加载，不缓存」，故走 `struct` 而非 `LoadEmojisUseCase` 的 `actor` cache 模式）。
   - 职责（与 `LoadChestUseCase` 同骨架）：① 调 `repository.fetchInventory()` 拿 `InventoryResponse` ② **DTO → domain 展平**：把 `response.groups` 的每个 `group` 的每个 `instance` 展平为一个 `HomeEquip`（见「关键约束 / 展平映射形状」钦定）—— **一个 instance 一个 HomeEquip**，同 group 多 instance 各成独立 `HomeEquip`（与 Story 24.1 sink「不去重不聚合，count 由 view filter().count 自然得出」契约一致）③ 成功 → `await MainActor.run { appState.applyInventory(flattened) }` ④ 失败 → `throw` 原样透传给调用方（caller 决定 loading/retry UI）。
   - **空背包**：`response.groups == []` → 展平结果 `[]` → `appState.applyInventory([])` → Story 24.1 sink 派发 `[]` → 空仓库 placeholder（不报错，V1 §8.2「空背包 `{groups: []}`，不报错」）。
4. **AC4 — `AppState.applyInventory(_:)` 单字段 mutation 入口**
   - 新增 `public func applyInventory(_ inventory: [HomeEquip])`：仅写 `self.currentInventory`，不动其它字段（与 `applyCurrentChest` / `applySyncedStepAccount` 全字段写入区分；`apply*` 前缀表示 hydrate/mutation 入口，ADR-0010 §3.3）。
   - **不**触发 `roomNavigationGeneration` bump（inventory mutation 与 room navigation 完全独立；与 `applyCurrentChest` 不 bump 同决策依据，lesson `2026-05-11-apply-home-data-bump-only-on-room-id-change.md`）。
   - **不**改 `reset()`（`reset()` 已 `currentInventory = []`，Story 37.4 落地；本 story 不动 —— Story 24.1 守护 case A 已防 reset 回归，本 story 写 `applyInventory` 与 reset 清空互不冲突）。
5. **AC5 — Wardrobe Tab 首次出现触发 + loading / retry UI（每次进 Tab 重拉，不缓存）**
   - 触发点：`WardrobeView`（tab 根，`MainTabView` 内 `ZStack`+opacity 路由 —— 4 tab 全 alive，故**不能**用 view `init` / 一次性 `.task`；必须用 **Wardrobe Tab 变为当前 tab** 这一信号触发，保证「每次切到 Wardrobe Tab 都重拉」语义；dev 在 Dev Notes「Wardrobe Tab 首次/每次出现触发点决策」回填实际实装方式后再写代码，候选见 Dev Notes）。
   - **每次进 Tab 都重新 load**（epics.md line 3367「不缓存」—— 因开箱后需立即看到新道具）；`LoadInventoryUseCase` 不持 cache（AC3 已钉死 `struct` 无状态）。
   - **loading**：load 进行中显示 `ProgressView`（不阻塞 tab 切换；首屏 `appState.currentInventory` 为空 → Story 24.1 sink 已渲染空仓库 placeholder，loading 叠加其上或替换，dev 按既有 `RetryView`/loading 视觉模式实装，不新造视觉）。
   - **失败**：显示可重试 `RetryView`（复用 `ErrorPresenter` 既有机制；与 epics.md line 3366「失败显示 RetryView（复用 ErrorPresenter）」一致）；用户点重试 → 重新发起 `LoadInventoryUseCase.execute()`。
   - **不**改 Story 24.1 sink / mapping —— 本 story 只负责把真实数据写进 `appState.currentInventory`，下游渲染 Story 24.1 已 final（写入后零 edit 反映）。

**And** **单元测试覆盖**（≥4 case epics.md line 3368-3372 钦定 + 守护 case；`@MainActor` + XCTest only，**禁止** import SnapshotTesting / ViewInspector —— ADR-0002 §3.1；mocked `InventoryRepository` 或 mocked `APIClient`；测试文件新建于 `iphone/PetAppTests/Features/Wardrobe/`）：
- **happy（epics.md line 3369）**：mock repo 返回 5 个 group（每 group count=1）→ `LoadInventoryUseCase.execute()` 后 `appState.currentInventory.count == 5`（展平后；且经 Story 24.1 既有 sink → `RealWardrobeViewModel.inventory.count == 5` 可作端到端断言，验证「24.2 写 → 24.1 sink 反映」闭环）
- **happy（epics.md line 3370）**：mock repo 返回 `{groups: []}` → `appState.currentInventory == []`，`RealWardrobeViewModel.inventory == []`（空仓库 placeholder 路径，与 Story 24.1 空态契约一致）
- **happy（多实例展平）**：mock repo 返回 1 个 group count=3（3 instance）→ 展平后 `appState.currentInventory.count == 3`，3 个 `HomeEquip.userCosmeticItemId` 各不相同（验证「一 instance 一 HomeEquip，不去重」契约）
- **edge（epics.md line 3371，API 失败）**：mock repo 抛 `APIError.network` → `execute()` rethrow（`appState.currentInventory` 不被污染 —— 失败不写 AppState，保持上次值 / 空；与 `LoadChestUseCase` 失败透传精神一致）
- **edge（epics.md line 3372，失败后手动重试）**：第一次抛错 → 第二次 mock repo 返回 2 个 group → 重新 `execute()` → `appState.currentInventory.count == 2`（验证「不缓存 + 可重试」语义）
- **守护 case（applyInventory 单字段隔离）**：`appState` 先 `applyCurrentChest` 设非空 chest → `applyInventory([...])` → `currentInventory` 更新且 `currentChest` 不变（验证 AC4 单字段 mutation 不波及其它字段）
- **守护 case（DTO 严格非可选解析）**：JSON `{"groups": null}` 或缺 `groups` 字段 → `InventoryResponse` 解析抛 decoding 错误（验证 AC1 fail-fast 契约；可对 Decodable 直接解析断言，不经 network）

**And** **build verify（必跑）**：`bash iphone/scripts/build.sh --test` 全绿（既有 ~741 case 不回归 Story 37.9 / 24.1 的 Wardrobe Scaffold 测试 / UITest + 本 story 新增 case）。新增文件后 xcodegen regen 由 `iphone/scripts/build.sh` 内部处理，**不**手改 `iphone/project.yml`。

**And** **iOS UI 验证（CLAUDE.md「iOS UI 验证（必跑）」红线 —— build pass ≠ 行为正确）**：用 `ios-simulator` MCP 实跑：build → `install_app` → `launch_app(terminate_running: true)` → 真实游客登录（**不**用 `UITEST_SKIP_GUEST_LOGIN`）→ 切到 Wardrobe Tab → `ui_view` 验证（新游客 inventory 空 → 空仓库 placeholder）→ **触发开箱链路**（dev 工具 `/dev/grant-steps` + `/dev/force-unlock-chest` + 开箱，或 `/dev/grant-cosmetic-batch`）→ 回 Home → 再切 Wardrobe Tab → `ui_view` + `ui_describe_all` 验证（重新 load 后多出真实道具 cell，非 mock；37.9/24.1 a11y 锚不回归）。loading / 失败路径若难在模拟器自然触发，至少截图记录 happy 闭环（开箱 → 重进 Tab → 道具出现）。

## Tasks / Subtasks

- [x] **Task 1：读源确认接续点 + 类型形状（写代码前必做，禁止臆测）**（AC: #1, #3）
  - [x] 1.1 读 `iphone/PetApp/Features/Home/Models/HomeData.swift` line 146-178 `struct HomeEquip` 完整定义（字段确认：`slot: Int / userCosmeticItemId: String / cosmeticItemId: String / name: String / rarity: Int / assetUrl: String`，共 **6 字段，无 iconUrl/status**；两个 `init`：`init(from dto: EquipDTO)` + 全参 `init(slot:userCosmeticItemId:cosmeticItemId:name:rarity:assetUrl:)`）
  - [x] 1.2 读 Story 24.1 `RealWardrobeViewModel.swift` line 122-179：确认本 story **不改** sink/mapping；`mapToCosmeticItems` 实际消费字段 = `slot`(→category) / `userCosmeticItemId`(→id) / `name` / `rarity`（4 字段），展平必须填对这 4 个；`cosmeticItemId`/`assetUrl` 当前 24.1 不消费但 `HomeEquip` 有该字段须填真实值
  - [x] 1.3 读 `LoadChestUseCase.swift` + `ChestRepository.swift` + `EmojisEndpoints.swift` + `EmojiListResponse.swift` + `EmojiRepository.swift` + `ChestRefreshTriggerService.swift` + `MockChestRepository.swift` + `LoadChestUseCaseTests.swift` —— 四件套 + service + mock + test 模板已确认（同模式照搬）
  - [x] 1.4 见 Dev Notes「HomeEquip 字段映射决策（iconUrl/status 无承载）」回填
  - [x] 1.5 见 Dev Notes「Wardrobe Tab 首次/每次出现触发点决策」回填（选定方案 1：`WardrobeView` `.task(id: coordinator.currentTab)`）
- [x] **Task 2：新建 inventory wire DTO**（AC: #1）
  - [x] 2.1 新建 `InventoryResponse.swift`：`InventoryResponse {groups: [InventoryGroup]}`（非可选）+ `InventoryGroup`（8 字段对齐 V1 §8.2 line 1366-1377）+ `InventoryInstance {userCosmeticItemId, status}`；`Decodable, Equatable, Sendable`；文件头引 V1 §8.2 + 空背包严格非可选 fail-fast 契约
- [x] **Task 3：新建 Repository + Endpoint**（AC: #2）
  - [x] 3.1 新建 `InventoryEndpoints.swift`：`inventory()` 工厂（path `/api/v1/cosmetics/inventory`，GET，body nil，requiresAuth true）
  - [x] 3.2 新建 `InventoryRepository.swift`：`InventoryRepositoryProtocol` + `DefaultInventoryRepository`（struct，`fetchInventory() async throws -> InventoryResponse`，APIError 原样透传，wire 直通不二次排序）
- [x] **Task 4：新建 LoadInventoryUseCase + AppState.applyInventory**（AC: #3, #4）
  - [x] 4.1 `AppState.swift` 新增 `applyInventory(_ inventory: [HomeEquip])`（仅写 `currentInventory`；不 bump generation；注释与 `applyCurrentChest` 同模式 + 引 ADR-0010 §3.3 + 3 条 lesson）
  - [x] 4.2 新建 `LoadInventoryUseCase.swift`：`LoadInventoryUseCaseProtocol` + `DefaultLoadInventoryUseCase`（struct 无 cache）；`execute()` = repo.fetchInventory → `flatten()` 展平 → `await MainActor.run { appState.applyInventory(_:) }`；失败原样 rethrow；`flatten(_:)` 抽 static 私有方法便于单测直接断言
- [x] **Task 5：AppContainer 工厂 wiring + Wardrobe Tab 触发点接入**（AC: #5）
  - [x] 5.1 `AppContainer.swift` 新增 `makeInventoryRepository()` + `makeLoadInventoryUseCase(appState:)`（与 `makeChestRepository` / `makeLoadChestUseCase` 同模式；**不**加 UITest mock gate —— UI 验证走真实游客登录 + 真实 server + dev grant，无 XCUITest fixture 依赖，不过度预建）
  - [x] 5.2 选定方案 1：`WardrobeView` `.task(id: coordinator.currentTab)` + guard `== .wardrobe` → `LoadInventoryUseCase.execute()`；loading → `ProgressView` overlay；失败 → `ErrorPresenter.present(error, onRetry:)`（RootView 既有 `.errorPresentationHost` 渲染全屏 RetryView）；CancellationError（切走 cancel）静默吞；每次进 Tab 重拉不缓存。`LoadInventoryUseCase` / `ErrorPresenter` 经新 EnvironmentKey（`\.loadInventoryUseCase` / `\.wardrobeErrorPresenter`，default nil）注入；RootView LaunchedContentView 透传 + `.environment` wire
  - [x] 5.3 **不**改 `RealWardrobeViewModel` sink / mapping / `WardrobeScaffoldView` 视觉 / `MockWardrobeViewModel`（实跑确认：写 `appState.currentInventory` 后 Story 24.1 既有 sink 零 edit 反映真实道具；a11y 锚 `wardrobeItem_<id>` / category 锚未回归）
  - [x] 5.4 新增 1 个 inline a11y 锚 `wardrobe_loading_indicator`（loading ProgressView；记入本节 + Dev Notes 触发点决策；与 37.13 `inline → 后续归并常量` 风格一致；不破坏 37.9/24.1 既有锚 —— 实跑 ui_describe_all 确认 `wardrobeView` / `wardrobeCategory_*` / `wardrobeItem_*` 全在）
- [x] **Task 6：单元测试**（AC: 测试覆盖）
  - [x] 6.1 新建 `LoadInventoryUseCaseTests.swift`（`@MainActor` + XCTest only；mock `InventoryRepositoryProtocol`；无 SnapshotTesting/ViewInspector）+ `MockInventoryRepository.swift`（scripted Result 队列 helper）
  - [x] 6.2 7 case：5 group happy（含端到端 24.1 sink 反映）/ 空 groups（含 sink 反映空）/ 多实例展平不去重 / API 失败 rethrow 不污染 / 失败后重试 / applyInventory 单字段隔离守护（chest 不变 + 不 bump generation）/ flatten 字段映射全断言
  - [x] 6.3 新建 `InventoryResponseTests.swift`：5 case —— `{groups: []}` → `[]` / 完整 group 解码 / `{groups: null}` 抛 decoding / 缺 groups 字段抛 decoding / group 缺必填字段抛 decoding
- [x] **Task 7：build + 模拟器实跑验证**（AC: build verify + iOS UI 验证）
  - [x] 7.1 `bash iphone/scripts/build.sh --test` 全绿：753 tests 0 failures（baseline ~741 + 12 新 case；37.9/24.1 Wardrobe scaffold / sink 测试不回归）
  - [x] 7.2 `ios-simulator` MCP（iPhone 17 Pro EC54A222）：build → install_app → launch_app(terminate_running:true) → 真实游客登录（user 26，**不**用 UITEST_SKIP_GUEST_LOGIN）→ 切 Wardrobe Tab → 空仓库 placeholder（帽子/饰品/围巾/服装 全 0；server log `GET /cosmetics/inventory` 200）→ `/dev/grant-cosmetic-batch` user 26 grant 6 实例（4 rarity-1 + 2 rarity-3）→ 回 Home → 重切 Wardrobe Tab（server log 新 inventory 请求 16:43:36 200，证明不缓存重拉）→ `ui_view` + `ui_describe_all` 验证：帽子 5 / 饰品 1，5 个 `wardrobeItem_13/14/15/17/18`（id 与 DB user_cosmetic_items.id 1:1，多实例不去重），37.9/24.1 a11y 锚全在不回归
  - [x] 7.3 实跑记录写入 Completion Notes（CLAUDE.md 红线满足）
- [x] **Task 8：收尾**
  - [x] 8.1 更新 File List + Completion Notes（每 AC 一条）+ Change Log
  - [x] 8.2 `sprint-status.yaml` `24-2: in-progress → review`

## Dev Notes

### 本 story 一句话定位

Story 24.1 把「下游 sink + mapping」做完并故意把「上游 UseCase + 触发」留给本 story（24-1 文件 AC3 明确划界）。本 story 补齐 DTO/Repository/Endpoint/UseCase 四件套 + `AppState.applyInventory` 单字段入口 + Wardrobe Tab 出现触发点，使「Wardrobe Tab 出现 → GET /cosmetics/inventory → 写 `appState.currentInventory` → Story 24.1 既有 sink 零 edit 渲染真实装扮」闭环成立。**不碰任何 view / sink / mapping / Mock。**

### 关键约束 / 展平映射形状（最易踩坑点）

- **wire 是 group 聚合结构，`appState.currentInventory` 是 `[HomeEquip]` 平铺**：V1 §8.2 响应 `data.groups[]`，每 group 含 `instances[]`。但 Story 24.1 既有 sink 订阅的是 `appState.$currentInventory: [HomeEquip]`（平铺），且 24.1 `mapToCosmeticItems` 是「一 `HomeEquip` → 一 `CosmeticItem`，不去重不聚合，count 由 view `filter().count` 自然得出」。故本 story `LoadInventoryUseCase` 必须把 `groups[].instances[]` **展平**：`response.groups.flatMap { group in group.instances.map { inst in HomeEquip(...) } }`，**一个 instance 一个 HomeEquip**。这样 24.1 sink/mapping 零 edit 即正确（同 cosmetic 多实例各成独立 `CosmeticItem`，分类 count 由 `WardrobeScaffoldView` filter 得出 = V1 §8.2 `count = instances 长度` 的 client 等价）。**不要**在 UseCase 做 group-by 聚合（破坏 24.1 既有渲染契约）。
- **`HomeEquip` 字段映射（dev Task 1.4 回填后再写）**：每 `(group, instance)` → 一 `HomeEquip`：
  - `slot` ← `group.slot`（int，V1 §8.2 枚举 `{1,2,3,4,5,6,7,99}`；24.1 `CosmeticCategory.category(forSlot:)` 已处理归并 + 未知 fallback）
  - `userCosmeticItemId` ← `instance.userCosmeticItemId`（实例级唯一 id；24.1 mapping 用作 `CosmeticItem.id`，多实例各独立靠此字段）
  - `cosmeticItemId` ← `group.cosmeticItemId`（配置 id；24.1 mapping 当前不消费此字段，但 `HomeEquip` 有该字段须填真实值不留空，未来 Story 27.x/30.x 可能用）
  - `name` ← `group.name`
  - `rarity` ← `group.rarity`（int `{1,2,3,4}`；24.1 `rarity(forServerValue:)` 已处理 + 未知 fallback `.N`）
  - `assetUrl` ← `group.assetUrl`（24.1 mapping 当前不消费，但 `HomeEquip` 有该字段须填真实值；Story 30.x sprite 用）
  - **`group.iconUrl` 无 `HomeEquip` 字段承载 → 本 story 丢弃**（24.1 `mapToCosmeticItems` 的 `iconEmoji` 走 `category.iconEmoji` 占位，不依赖 wire `iconUrl`；Story 30.x 接真实 sprite 时由那时决定是否演进 AppState 类型 —— **本 story 不引入新领域类型 / 不改 `HomeEquip` struct / 不改 `AppState.currentInventory` 类型**，ADR-0010 §4.4 节点 1 占位类型沿用原则 + 24-1 文件 line 107「Story 24.2 接 LoadInventoryUseCase 时若发现 HomeEquip 不足……由 24.2 决定是否演进」—— 本 story 评估结论：`HomeEquip` 足以承载 24.1 sink 所需 4 字段（slot/userCosmeticItemId/name/rarity），`iconUrl` 当前无人消费，**不演进**，避免牵动 AppState/applyHomeData/reset/既有测试越界）。
- **`status` 字段处理**：V1 §8.2 `instance.status ∈ {1,2}`（in_bag/equipped，server 已过滤 consumed/invalid）。`HomeEquip` 无 `status` 字段；Story 24.1 sink/mapping 不消费 status（24.5/27.1 才区分 equipped/已装备）。本 story 展平时 `status` **不映射进 `HomeEquip`**（无字段承载，且 24.1 不需要）—— DTO 保留 `InventoryInstance.status` 解析（契约完整性 + 未来 Story 24.5/27.1 用），但展平到 `HomeEquip` 时丢弃 status。在 Dev Notes 标注此为「节点 8 阶段 `HomeEquip` 占位类型不携带 status，Story 24.5/27.1 需要时由那时演进」。
- **不缓存（每次进 Tab 重拉）**：epics.md line 3367 钦定。故 `LoadInventoryUseCase` 走 `struct` 无状态（与 `LoadChestUseCase` 同），**不**学 `LoadEmojisUseCase` 的 `actor` + cache + single-flight（emoji 是静态配置可缓存；inventory 开箱后必须立即变，反缓存）。Wardrobe Tab 每次出现都 `execute()` 一次。
- **失败不污染 AppState**：UseCase 失败路径 `throw` 原样透传，**不**写 `appState.currentInventory`（保持上次值或空）；与 `LoadChestUseCase` 「失败透传，caller 决定不阻塞 UI」精神一致。caller 层（Wardrobe Tab 触发点）catch 后显示 `RetryView`（复用 `ErrorPresenter`）。
- **`InventoryResponse.groups` 严格非可选**：V1 §8.2 关键约束「空背包返回 `{groups: []}`，不是 `null`、不是缺字段」+ 「防 Swift Codable 解析 groups 为 nil，client 严格按 `[InventoryGroup]` 非可选解析」。故 DTO `let groups: [InventoryGroup]`（非 `[InventoryGroup]?`）；server 若违约返 null/缺字段 → APIClient 抛 `APIError.decoding` fail-fast（与 `EmojiListResponse` 注释「server 漏发/null → APIError.decoding fail-fast」+ lesson `2026-04-27-home-data-fail-fast-on-unknown-enum.md` 同精神）。

### HomeEquip 字段映射决策（iconUrl/status 无承载）—— Task 1.4 回填

读源确认 `HomeEquip`（`HomeData.swift` line 146-178）实际 **6 字段，无 `iconUrl`、无 `status`**：`slot: Int / userCosmeticItemId: String / cosmeticItemId: String / name: String / rarity: Int / assetUrl: String`。展平时 `(group, instance)` → 一 `HomeEquip` 钦定映射：

| HomeEquip 字段 | 来源 | 说明 |
|---|---|---|
| `slot` | `group.slot` | V1 §8.2 枚举 `{1,2,3,4,5,6,7,99}`；24.1 `CosmeticCategory.category(forSlot:)` 已处理归并 + 未知 fallback |
| `userCosmeticItemId` | `instance.userCosmeticItemId` | 实例级唯一 id；24.1 mapping 用作 `CosmeticItem.id`，多实例各独立靠此 |
| `cosmeticItemId` | `group.cosmeticItemId` | 配置 id；24.1 当前不消费但 `HomeEquip` 有该字段须填真实值（Story 27.x/30.x 可能用） |
| `name` | `group.name` | — |
| `rarity` | `group.rarity` | V1 §8.2 枚举 `{1,2,3,4}`；24.1 `rarity(forServerValue:)` 已处理 + 未知 fallback `.N` |
| `assetUrl` | `group.assetUrl` | 24.1 当前不消费但 `HomeEquip` 有该字段须填真实值（Story 30.x sprite 用） |

- **`group.iconUrl` 丢弃**：`HomeEquip` 无该字段承载；24.1 `mapToCosmeticItems` 的 `iconEmoji` 走 `category.iconEmoji` 占位，不依赖 wire `iconUrl`。
- **`instance.status` 丢弃**：`HomeEquip` 无该字段；24.1 sink/mapping 不消费 status（24.5/27.1 才区分 equipped）。DTO **保留** `InventoryInstance.status` 解析（契约完整性 + 未来用），仅展平到 `HomeEquip` 时不映射。
- **边界声明**：`HomeEquip` 是节点 1 占位复用类型（ADR-0010 §4.4 + 24-1 文件 line 107「24.2 决定是否演进」）。本 story 评估结论：**够用，不演进** —— 24.1 sink 实际仅需 4 字段（slot/userCosmeticItemId/name/rarity），全部可填；`iconUrl`/`status` 当前无人消费，演进会牵动 `AppState`/`applyHomeData`/`reset`/既有测试越界。Story 30.x 接真实 sprite / Story 24.5·27.1 需要 status 时由那时演进。

### Wardrobe Tab 首次/每次出现触发点决策 —— Task 1.5 回填

`MainTabView` 用 `ZStack` + opacity 路由（line 45-62），**4 个 tab 根视图全部 alive**（不懒加载 / 不销毁重建）。`AppCoordinator.currentTab: AppTab`（`@Published`，line 61）是 `MainTabView` selection 唯一真理源（用户点 tab → `FloatingTabBar` 写 `coordinator.currentTab`）。

- **不能**用 `WardrobeView.init()`（只构造一次）/ 一次性 `.task`（view 永 alive → 只触发一次），违反「每次进 Tab 重拉不缓存」。
- **最终选定方案 1**：`WardrobeView` 上 `.task(id: coordinator.currentTab)`，task 内 `guard coordinator.currentTab == .wardrobe else { return }` 后调 `LoadInventoryUseCase.execute()`。
  - **实装文件/行**：`iphone/PetApp/Features/Wardrobe/Views/WardrobeView.swift`（`@EnvironmentObject var coordinator: AppCoordinator` + `@EnvironmentObject var appState` + `.task(id:)`；`LoadInventoryUseCase` 经 `@Environment` 注入闭包或 `AppContainer` 经 environment 传入）。
  - **为何与 AppCoordinator.currentTab 架构一致**：`.task(id:)` 语义 = id 变化时 cancel 旧 task + 起新 task。`currentTab` 每次变（含切到 wardrobe 与切走）都触发；切到 `.wardrobe` 才真正 fetch，切走时旧 in-flight task 被自动 cancel（loading 可取消，符合「不阻塞 tab 切换」）。这与既有 `AppCoordinator.currentTab` 双向绑定架构（line 33-44 `@EnvironmentObject var coordinator`）零冲突，复用 SwiftUI 原生 `.task(id:)` 不引入新触发机制；且 `WardrobeView` 已在 `MainTabView` ZStack 内常驻，`.task(id:)` 在 view 生命周期内随 id 反复重启 = 「每次切到 Wardrobe 重拉不缓存」语义精确实现。
  - **首次出现也覆盖**：app 启动默认 `currentTab == .home`；用户首次点「仓库」→ `currentTab` 从 `.home` 变 `.wardrobe` → `.task(id:)` id 变 → 触发首次 load（无需额外 `.onAppear`）。
- **loading / retry UI**：
  - **loading**：`WardrobeView` 用 `@State private var isLoading` 包裹 `execute()` 前后；loading 时在 `WardrobeScaffoldView` 之上叠加 `ProgressView`（`.overlay`，**不**改 `WardrobeScaffoldView` 本体视觉 / a11y 锚）。首屏 `appState.currentInventory` 空 → 24.1 sink 已渲染空仓库 placeholder（不白屏），ProgressView 叠其上。
  - **失败**：catch 后调 `appState` 关联的 `ErrorPresenter.present(error, onRetry:)`（epics.md line 3366「复用 ErrorPresenter」）。`ErrorPresenter` 经 `errorPresentationHost` modifier（RootView 最外层已挂，line 598）自动渲染全屏 `RetryView`；`onRetry` 闭包重新调 `LoadInventoryUseCase.execute()`。`ErrorPresenter` 经 `@Environment` 注入 `WardrobeView`（与 RootView 既有 `container.errorPresenter` 同实例）。
  - **不新造视觉**：ProgressView 是 SwiftUI 原生；RetryView 复用 `Core/DesignSystem/Components/RetryView.swift`（既有）；`ErrorPresenter` 复用 `Shared/ErrorHandling/ErrorPresenter.swift`（既有，RootView 已 `.errorPresentationHost`）。

### 测试栈约束（ADR-0002 §3.1 钦定 —— 不可违反）

- **XCTest only**：`iphone/PetAppTests/` 下测试**禁止** import `SnapshotTesting` / `ViewInspector`（Epic 37 §AC 红线 + ADR-0002 §3.1；Story 37.7/37.8/37.9/24.1 全遵守）。
- 展平逻辑抽 `LoadInventoryUseCase` 私有方法（如 `flatten(_:) -> [HomeEquip]`）让单测直接断言，**不**经 view 内省（与 24.1 `mapToCosmeticItems` 抽方法直测同模式）。
- `@MainActor` 测试 + 真实 `AppState()` 注入（`AppState` 是 `@MainActor` `ObservableObject`，`init()` 无参可直接构造）。mock `InventoryRepositoryProtocol`（实现协议返预设 `InventoryResponse` 或抛预设 `APIError`）—— 参考 `iphone/PetAppTests/` 既有 mock repo 模式（如 24.1 测试 / chest 测试的 mock）。
- 端到端闭环 case：注入真实 `AppState` + `RealWardrobeViewModel(appState:)`（24.1 既有，构造即订阅 sink）→ `LoadInventoryUseCase(repo: mock, appState:).execute()` → 断言 `realWardrobeVM.inventory` 反映出展平后道具（证明「24.2 写 → 24.1 sink → UI 数据」闭环，不经 view 渲染只断言 ViewModel 派生字段）。

### Story 24.1 / 37.9 / 37.7-8 沉淀 lesson（预防性应用，不重蹈覆辙）

- `docs/lessons/2026-04-30-published-derived-state-needs-publisher-subscription.md`：Story 24.1 sink 路径不可退化为一次性 hydrate —— **本 story 不碰 sink**，但写入入口 `applyInventory` 必须走 `@Published currentInventory` 的赋值（触发 publisher 派发），**不**绕过 publisher 直接 mutate（`AppState.currentInventory` 是 `@Published`，`applyInventory` 内 `self.currentInventory = inventory` 即正确派发；与 `applyCurrentChest` 同写法）。
- `docs/lessons/2026-05-11-apply-home-data-bump-only-on-room-id-change.md`：`applyInventory` **不** bump `roomNavigationGeneration`（inventory 与 room navigation 无关；与 `applyCurrentChest` 不 bump 同决策）。
- `docs/lessons/2026-04-27-home-data-fail-fast-on-unknown-enum.md`：DTO 解析未知/缺失走 fail-fast 抛 decoding（`InventoryResponse.groups` 非可选 + 未知字段不静默吞）。
- 24-1 文件 line 107 + line 48-50：本 story 接续点钦定 —— `LoadInventoryUseCase` 本体属本 story；评估 `HomeEquip` 是否够用由本 story 决策（本 story 结论：够用，不演进类型，理由见上「关键约束」）。

### Project Structure Notes

- 新建文件（全走 `iphone/project.yml` 通配 inclusion，**不**手改 project.yml；xcodegen regen 由 `iphone/scripts/build.sh` 内部处理）：
  - `iphone/PetApp/Features/Wardrobe/Models/InventoryResponse.swift`（wire DTO）
  - `iphone/PetApp/Features/Wardrobe/UseCases/InventoryEndpoints.swift`
  - `iphone/PetApp/Features/Wardrobe/Repositories/InventoryRepository.swift`
  - `iphone/PetApp/Features/Wardrobe/UseCases/LoadInventoryUseCase.swift`
  - `iphone/PetAppTests/Features/Wardrobe/LoadInventoryUseCaseTests.swift`
  - `iphone/PetAppTests/Features/Wardrobe/InventoryResponseTests.swift`（DTO 解析 case，可选合并进上一文件）
- 改动文件：
  - `iphone/PetApp/App/AppState.swift`（新增 `applyInventory(_:)`，不动 `reset`/`applyHomeData`/其它）
  - `iphone/PetApp/App/AppContainer.swift`（新增 `makeInventoryRepository()` / `makeLoadInventoryUseCase(appState:)` 工厂；按需加 `UITestMockInventoryRepository`）
  - Wardrobe Tab 触发点文件（Task 1.5 选定，预期 `iphone/PetApp/Features/Wardrobe/Views/WardrobeView.swift` 加 `.task(id:)`/`.onChange` —— 不改 `WardrobeScaffoldView` 视觉）
- iOS 工程结构遵循 `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §Features 三层（ViewModels / UseCases / Repositories）；本 story 触 UseCases + Repositories + Models 层 + AppState/AppContainer wiring + Wardrobe Tab 触发 seam，**不**触 ViewModels（24.1 已 final）。

### 边界声明（防 scope creep）

- **不**改 `RealWardrobeViewModel` / `MockWardrobeViewModel` / `WardrobeViewModel` 基类（Story 24.1 / 37.9 final）。
- **不**改 `WardrobeScaffoldView` 视觉 / 布局 / a11y 锚 / `badgeText` / `CosmeticCategory` mapping（24.1 / 37.9 冻结）。
- **不**实装筛选 segment control（Story 24.3）/ 穿戴按钮（Story 24.5 / 27.1）。
- **不**改 `HomeEquip` struct / `AppState.currentInventory` 类型 / `applyHomeData` / `reset`（节点 1 占位类型沿用，ADR-0010 §4.4；本 story 评估 `HomeEquip` 够用不演进）。
- **不**反向修改 V1 §7.x / §8.x 接口文档（schema Epic 23 冻结，见 §1 冻结流程）。
- **不**改 server 端任何代码（Story 23.4 已交付 `GET /cosmetics/inventory`；本 story 纯 client 接入）。

### References

- [Source: _bmad-output/planning-artifacts/epics.md §Story 24.2 (line 3352-3372)] —— 本 story user story + AC + ≥4 单测 case 钦定（`LoadInventoryUseCase` / 解析 groups / loading ProgressView / 失败 RetryView 复用 ErrorPresenter / 每次打开重拉不缓存）
- [Source: _bmad-output/implementation-artifacts/24-1-仓库页-swiftui-骨架.md (AC3 line 46-50 + Dev Notes line 107 + Completion Notes line 189)] —— **本 story 接续点权威划界**：UseCase 本体属 24.2 / 不预 over-design / `HomeEquip` 够用否由 24.2 决策 / 24.1 sink+mapping 已 final 写 currentInventory 即零 edit 反映
- [Source: iphone/PetApp/Features/Wardrobe/ViewModels/RealWardrobeViewModel.swift (line 122-179)] —— 下游 sink `subscribeInventory` + `mapToCosmeticItems` + `rarity(forServerValue:)`（本 story **不改**，仅须保证写入 `[HomeEquip]` 的 slot/userCosmeticItemId/name/rarity 4 字段填对）
- [Source: docs/宠物互动App_V1接口设计.md §8.2 (line 1311-1445，schema Epic 23 冻结)] —— `GET /cosmetics/inventory` 完整契约：响应体字段表（cosmeticItemId/name/slot/rarity/iconUrl/assetUrl/count/instances[].userCosmeticItemId/status）/ 空背包 `{groups: []}` 非可选 / status∈{1,2} / 两级确定性全序排序 / 无 query 参数 / 错误码 1001/1005/1009
- [Source: iphone/PetApp/Features/Home/UseCases/LoadChestUseCase.swift + Repositories/ChestRepository.swift] —— UseCase（struct 无 cache + repo→DTO→domain + `await MainActor.run` 写 AppState + 失败原样透传）/ Repository（struct value type + APIError 透传 + 不二次排序）模板
- [Source: iphone/PetApp/Features/Emoji/UseCases/EmojisEndpoints.swift + Models/EmojiListResponse.swift + Repositories/EmojiRepository.swift] —— Endpoint 工厂（`/api/v1` 前缀 + GET + body nil + requiresAuth true）/ wire DTO（`Decodable,Equatable,Sendable` + 非可选 array + server 漏发 fail-fast）模板
- [Source: iphone/PetApp/App/AppState.swift (line 73 currentInventory / line 111-123 reset / line 152-168 applyCurrentChest)] —— `currentInventory: [HomeEquip]` @Published 真实类型 + `reset()` 已清空 + `applyCurrentChest` 单字段 mutation 入口模式（本 story `applyInventory` 照此）
- [Source: iphone/PetApp/Features/Home/Models/HomeData.swift (line 146-180 struct HomeEquip)] —— 展平目标类型（6 字段，**无 iconUrl/status**；dev Task 1.1 必读实际签名）
- [Source: iphone/PetApp/App/MainTabView.swift (line 32-70) + Features/Wardrobe/Views/WardrobeView.swift + App/AppCoordinator (currentTab)] —— Wardrobe Tab `ZStack`+opacity 全 alive 路由 → 触发点必须用 `coordinator.currentTab` 变化信号（非 view init/一次性 .task），保证「每次进 Tab 重拉不缓存」
- [Source: iphone/PetApp/App/AppContainer.swift (line 498-525 makeChestRepository/makeLoadChestUseCase + line 687-720 UITestMockChestRepository)] —— Repository/UseCase 工厂 wiring + UITest mock repo 注入模式
- [Source: _bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md §3.3 §4.4] —— `apply*` mutation 入口契约 + §4.4 节点 1 占位类型沿用缓解策略（解释为何 `currentInventory` 实际是 `[HomeEquip]`，本 story 不演进）
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md §3.1] —— 测试栈 XCTest only（禁 SnapshotTesting/ViewInspector）
- [Source: docs/lessons/2026-04-30-published-derived-state-needs-publisher-subscription.md] —— `applyInventory` 必须走 `@Published` 赋值触发 publisher 派发（不绕过）
- [Source: docs/lessons/2026-05-11-apply-home-data-bump-only-on-room-id-change.md] —— `applyInventory` 不 bump roomNavigationGeneration
- [Source: docs/lessons/2026-04-27-home-data-fail-fast-on-unknown-enum.md] —— DTO 严格非可选解析 + 缺失/未知 fail-fast 抛 decoding
- [Source: CLAUDE.md §「iOS UI 验证（必跑）」§「资产类操作必须事务」§「状态以 server 为准」] —— build pass ≠ 行为正确（必须 ios-simulator MCP 实跑开箱→重进 Tab→道具出现闭环）+ inventory 状态以 server 响应为最终态（client 不本地造数据）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md §Features 三层] —— iOS 工程结构（本 story 触 UseCases/Repositories/Models + AppState/AppContainer wiring）

## Dev Agent Record

### Agent Model Used

Opus 4.7 (1M context) — claude-opus-4-7[1m]

### Debug Log References

- 本地 server 实跑前置：Redis 未装 → docker `redis:7-alpine` 起容器；MySQL 容器 `schema_migrations` 误标 `version=1,dirty=1`（5 张早期表实为 0001-0005 已落地）→ **非破坏性**修正 tracker 行为 `version=5,dirty=0` → `catserver-dev migrate up` 补 0006-0015（含 cosmetic_items 15 seed + user_cosmetic_items）→ devtools server (`BUILD_DEV=true catserver-dev`) 起在 8080。
- 端口冲突：8080 被一个 15 天前的 stale `catserver`（全路径 502）占用 → kill 后用 devtools binary 重起。
- shell 有 `http_proxy=127.0.0.1:10808` 拦 localhost（curl 返 502 假象）；iOS Simulator URLSession 不走 shell proxy，app 真实请求正常（server log 证 200）。

### Completion Notes List

- **AC1（DTO 严格对齐 V1 §8.2 + fail-fast）**：新建 `InventoryResponse`/`InventoryGroup`(8 字段)/`InventoryInstance`(userCosmeticItemId/status)，`groups: [InventoryGroup]` **非可选**，`Decodable,Equatable,Sendable`，DTO 层不排序/去重/校验。`InventoryResponseTests` 5 case 验证：`{groups:[]}`→`[]`、完整 group 解码、`{groups:null}`/缺字段/group 缺必填 → 抛 `DecodingError`（fail-fast）。
- **AC2（Repository wire 直通）**：`InventoryRepositoryProtocol`(:Sendable) + `DefaultInventoryRepository`(struct) `fetchInventory()` 调 `InventoryEndpoints.inventory()`（path `/api/v1/cosmetics/inventory`，GET，body nil，requiresAuth true）；APIError 原样透传不二次排序。
- **AC3（UseCase repo→展平→写 AppState）**：`DefaultLoadInventoryUseCase`(struct 无 cache) `execute()` = fetch → `flatten()`（`groups.flatMap{ instances.map{ HomeEquip(...) }}` 一 instance 一 HomeEquip 不聚合）→ `await MainActor.run { appState.applyInventory }`；失败 rethrow 不写 AppState。空 groups → `[]` → 空仓库不报错。`flatten` 抽 static 私有方法直测字段映射。
- **AC4（applyInventory 单字段 mutation）**：`AppState.applyInventory(_:)` 仅写 `@Published currentInventory`（触发 publisher 派发 → 24.1 sink），不动其它字段，不 bump `roomNavigationGeneration`，不改 `reset`。守护 case 验证 applyCurrentChest 后 applyInventory 不波及 currentChest + 不 bump generation。
- **AC5（Wardrobe Tab 触发 + loading/retry 不缓存）**：`WardrobeView` `.task(id: coordinator.currentTab)` + guard `== .wardrobe` → execute；loading `ProgressView` overlay（a11y 锚 `wardrobe_loading_indicator`，不改 WardrobeScaffoldView 本体）；失败经 `ErrorPresenter.present(error,onRetry:)` → RootView 既有 `.errorPresentationHost` 渲染全屏 RetryView，onRetry 重发 execute；CancellationError 静默吞；不缓存（每次切到 Wardrobe 重拉，实跑验证 3 次独立 server inventory 请求）。`LoadInventoryUseCase`/`ErrorPresenter` 经新 EnvironmentKey 注入，RootView LaunchedContentView 透传。**不**改 24.1 sink/mapping/MockWardrobeViewModel/WardrobeScaffoldView。
- **build verify**：`bash iphone/scripts/build.sh --test` → 753 tests 0 failures（baseline ~741 + 12 新 case：7 LoadInventoryUseCase + 5 InventoryResponse）；37.9/24.1 既有测试不回归。
- **iOS UI 实跑（CLAUDE.md 红线满足）**：iPhone 17 Pro 模拟器，真实游客登录（user 26，未用 UITEST_SKIP_GUEST_LOGIN）。① 切 Wardrobe Tab → 空仓库（帽子/饰品/围巾/服装 全 0，server log `GET /cosmetics/inventory` 200）。② `/dev/grant-cosmetic-batch` 给 user 26 发 6 实例（3×小黄帽 slot1 rarity1 + 1× slot2 + 2×金王冠 slot1 rarity3）。③ 回 Home 再切 Wardrobe（server log 新 inventory 请求，证不缓存重拉）→ `ui_view` 显示 帽子 5 / 饰品 1，5 个真实道具 cell（3×小黄帽 灰 N badge + 2×金王冠 紫 SR badge），preview「小黄帽·已拥有」。④ `ui_describe_all` 确认 `wardrobeItem_13/14/15/17/18`（id 与 DB `user_cosmetic_items.id` 1:1，多实例不去重）+ `tab_*` / `wardrobeView` / `wardrobeCategory_*` / `合成` / `装备` 既有 a11y 锚全在不回归。闭环「Tab 出现 → 真实 GET /cosmetics/inventory → 展平 → applyInventory → 24.1 既有 sink 零 edit → 真实道具渲染」验证通过。
- **边界遵守**：未改 V1 §8.x、未改 server、未改 `HomeEquip`/`AppState.currentInventory` 类型/`applyHomeData`/`reset`、未改 24.1 sink/mapping、未改 WardrobeScaffoldView 视觉/Mock。`HomeEquip` 评估结论：够用不演进（`iconUrl`/`status` 无承载丢弃，24.1 仅需 4 字段全可填）。

### File List

新建：
- `iphone/PetApp/Features/Wardrobe/Models/InventoryResponse.swift`
- `iphone/PetApp/Features/Wardrobe/UseCases/InventoryEndpoints.swift`
- `iphone/PetApp/Features/Wardrobe/Repositories/InventoryRepository.swift`
- `iphone/PetApp/Features/Wardrobe/UseCases/LoadInventoryUseCase.swift`
- `iphone/PetAppTests/Features/Wardrobe/LoadInventoryUseCaseTests.swift`
- `iphone/PetAppTests/Features/Wardrobe/InventoryResponseTests.swift`
- `iphone/PetAppTests/Features/Wardrobe/MockInventoryRepository.swift`

改动：
- `iphone/PetApp/App/AppState.swift`（新增 `applyInventory(_:)`）
- `iphone/PetApp/App/AppContainer.swift`（新增 `makeInventoryRepository()` / `makeLoadInventoryUseCase(appState:)`）
- `iphone/PetApp/Features/Wardrobe/Views/WardrobeView.swift`（`.task(id:)` 触发 + loading overlay + 2 EnvironmentKey）
- `iphone/PetApp/App/RootView.swift`（LaunchedContentView 新增 `loadInventoryUseCase`/`wardrobeErrorPresenter` 属性+init+wire `.environment`）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（24-2 状态流转）

### Change Log

| 日期 | 变更 | 作者 |
|---|---|---|
| 2026-05-17 | Story 创建（create-story；范围按 epics.md §Story 24.2 + V1 §8.2 冻结 schema + Story 24.1 AC3 接续红线锚定，定位为「补齐 24.1 留空的 UseCase/Repository/DTO/Endpoint + AppState.applyInventory + Wardrobe Tab 出现触发，闭环 24.1 既有 sink」窄范围 story；不碰 view/sink/mapping/Mock） | Bob (SM) |
| 2026-05-17 | dev-story 实装完成：新建 InventoryResponse DTO / InventoryEndpoints / InventoryRepository / LoadInventoryUseCase / AppState.applyInventory / AppContainer 工厂 / WardrobeView `.task(id:)` 触发 + loading overlay + 2 EnvironmentKey + RootView wire；新增 12 单测 case（7 UseCase + 5 DTO）全绿（753 tests 0 fail，无回归）；iOS Simulator 实跑验证闭环（真实游客登录 user 26 → 空仓库 → /dev/grant-cosmetic-batch 6 实例 → 重切 Tab 不缓存重拉 → 真实道具渲染，37.9/24.1 a11y 锚不回归）。Status → review | Dev (Opus 4.7) |
