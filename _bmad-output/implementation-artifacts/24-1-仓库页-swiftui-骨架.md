# Story 24.1: 仓库页 SwiftUI 骨架 ⟶ 注入真实 RealWardrobeViewModel（替换 Mock → Real inventory 派生）

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As an iPhone 用户,
I want Wardrobe Tab 打开时显示我的真实装扮列表（来自 server inventory，按分类聚合展示，可看每件实例 id）,
so that 我看到的是真实持有数据，而不是 Story 37.9 的硬编码 mock 装扮。

## 范围红线（必读 —— Story 已被两次 supersede，原始 epics.md AC 不再 active）

> **唯一权威 AC 来源**：[sprint-change-proposal-2026-04-29-v2.md §5.4](../planning-artifacts/sprint-change-proposal-2026-04-29-v2.md)（line 396-424）。epics.md §Story 24.1（line 3326-3350）的「Sheet 弹 InventoryView / InventoryView / InventoryViewModel / InventoryGroup」语义**全部作废**——ADR-0009 推翻 Sheet 主入口模式，Wardrobe 是 MainTabView 4 Tab 之一，Story 37.9 已交付 `WardrobeScaffoldView` + `WardrobeViewModel` class 层次 + `Mock/RealWardrobeViewModel` 子类。

**本 story 是「替换 Mock → Real 数据派生」的窄范围 story，不是建新页面**：

- Story 37.9 **已落地**：`WardrobeScaffoldView`（4 区块视觉，零 edit）/ `WardrobeViewModel` 基类 / `MockWardrobeViewModel` / `RealWardrobeViewModel` 骨架（含占位 `subscribeInventory` sink）/ `CosmeticItem` / `CosmeticCategory` / `WardrobeScaffoldDefaults` / RootView wire（已用 `RealWardrobeViewModel()` 而非裸基类）/ `.onAppear` 内 `bind(appState:)`。**这些 caller / view / 注入路径本 story 全部不动**。
- 本 story 唯一实质改动：把 `RealWardrobeViewModel.subscribeInventory` 的**占位 sink**（当前：`currentInventory` 非空时保持 mock 不动）改为**真实派生**——从 `appState.currentInventory` 映射出 `[CosmeticItem]` 写 `self.inventory`；空时退回空仓库（让 Story 37.9 已实装的空态 placeholder 生效），不再退回 `WardrobeScaffoldDefaults.inventory` mock。
- Story 37.9 handoff「与 Story 24.1 衔接的红线」（37-9 文件 line 1395-1403）明确：本 story **不**预加 `LoadInventoryUseCase` 接口字段——那是 Story 24.2 的范围；本 story 仅修 sink closure 的 mapping + 确认空态切换。

## Acceptance Criteria

**权威来源**：sprint-change-proposal-2026-04-29-v2.md §5.4（line 407-422）。逐条对齐：

**Given** Story 37.9 WardrobeView Scaffold 已交付（分类 Tab / 3 列网格 / 预览区 / `WardrobeViewModel` 基类 / `MockWardrobeViewModel` 子类 + `RealWardrobeViewModel` 骨架）+ Story 37.4 `AppState.currentInventory` 字段就绪 + Story 23.4 server `GET /cosmetics/inventory` 可用
**When** 完成本 story
**Then**：

1. **AC1 — `RealWardrobeViewModel.subscribeInventory` 实装真实 inventory 派生**
   - 当前 `RealWardrobeViewModel.swift` line 104-120 的 `subscribeInventory` sink closure 是占位（`homeEquips.isEmpty` → 退回 `WardrobeScaffoldDefaults.inventory` mock；非空 → 保持不动）。本 story 改为：sink 订阅 `appState.$currentInventory`（类型 `[HomeEquip]`，见「关键约束 / 类型决策」），closure 内把 `[HomeEquip]` 映射为 `[CosmeticItem]` 写 `self.inventory`。
   - `appState.currentInventory` 为空（启动后 / reset 后 / hydrate 前）→ `self.inventory = []`（**不**再退回 `WardrobeScaffoldDefaults.inventory`；空仓库走 Story 37.9 已实装的空态 placeholder）。
   - `appState.currentInventory` 非空 → 映射写 `self.inventory`。
   - mapping 函数独立成私有方法（如 `private func mapToCosmeticItems(_:) -> [CosmeticItem]`），便于单测直接断言。
2. **AC2 — `HomeEquip → CosmeticItem` mapping 形状钦定**
   - `CosmeticItem` 字段（Story 37.9 落地，**不改 struct**）：`id: String / name: String / category: CosmeticCategory / rarity: Rarity / owned: Bool / iconEmoji: String`。
   - `HomeEquip` 是 `appState.currentInventory` 元素类型（节点 1 占位复用 Home* 类型族，见「关键约束 / 类型决策」）。mapping 规则（每个 `HomeEquip` → 一个 `CosmeticItem`）：
     - `id` ← `HomeEquip` 的实例/配置 id 字段（读 `HomeEquip` 实际字段，见 Dev Notes「HomeEquip 字段核对」——dev 必须先读 `HomeData.swift` line 146 `struct HomeEquip` 定义再写 mapping，**禁止**臆测字段名）
     - `name` ← `HomeEquip.name`
     - `category` ← 由 `HomeEquip` slot 整数映射到 `CosmeticCategory` enum（slot→category 映射表见 Dev Notes；未知 slot 走 fallback case）
     - `rarity` ← 由 `HomeEquip` rarity 整数映射到 `Rarity` enum（`{1,2,3,4}` → `N/R/SR/SSR`；映射规则读 Story 37.6 落地的 `Rarity` enum 定义，未知值 fallback `.n`）
     - `owned` ← inventory 里的都是「已拥有实例」（语义恒为 `true`；空仓库时数组为空，不出现 `owned=false`）
     - `iconEmoji` ← 占位（按 `category` 给固定 emoji，复用 `WardrobeScaffoldDefaults` 或 `CosmeticCategory` 已有 iconEmoji 派生；Story 30.x 接真实 sprite 时升级）
   - **count 聚合 / `99+` clamp 语义**：`WardrobeScaffoldView` 的分类 tab badge 与 grid 用 `state.inventory.filter { $0.category == category }.count`（Story 37.9 已实装，line 228）。本 story 保证 mapping 后**同一 cosmetic 的多个实例都映射成独立 `CosmeticItem`**（不在 ViewModel 层做去重聚合——聚合 count 由 view 层 filter().count 自然得出，与 Story 37.9 既有渲染契约一致）。`count > 99`（理论不该）时 UI 显示「99+」。**实装方式钦定**（消除 dev 选择歧义 + 满足 XCTest-only 测试栈）：把 clamp 抽为**可单测纯函数**（如 `static func badgeText(forCount count: Int) -> String { count > 99 ? "99+" : "\(count)" }`，放 `CosmeticCategory` 或新建轻量 helper），`WardrobeScaffoldView` line ~228/234 badge 文案改调该函数。**不**在 view 内联三元后用 ViewInspector 测（违反 ADR-0002 §3.1）。这是本 story 允许触碰 `WardrobeScaffoldView` 的**唯一**点（其余视图代码零 edit）。
3. **AC3 — Wardrobe Tab 首次出现自动刷 AppState 的 seam（不实装 UseCase 本体）**
   - 提案 §5.4 line 416：「Wardrobe Tab 首次出现时自动调 `LoadInventoryUseCase`（Story 24.2）刷 AppState」。
   - **范围边界**：`LoadInventoryUseCase` **本体属于 Story 24.2**（37-9 红线 line 1399-1403 + 提案 §5.4 把 UseCase 调用归 Story 24.2）。本 story **不**创建 `LoadInventoryUseCase` / `InventoryRepository` / endpoint / DTO。
   - 本 story 仅需**确认 sink 路径已就绪**：`RealWardrobeViewModel` 通过 `subscribeInventory` 订阅 `appState.$currentInventory`——Story 24.2 落地 `LoadInventoryUseCase` 后只要写 `appState.currentInventory` 即零 edit 反映到 Wardrobe（这是 Story 37.9 已 hookup 的 sink，本 story 实装 closure mapping 即完成「Story 24.2 写 → Wardrobe 显示」闭环）。
   - **不**在本 story 加 `.task` / `.onAppear` 触发 load——避免与 Story 24.2 的 UseCase 触发点冲突 / 重复实装（Story 24.2 负责「Wardrobe Tab 首次出现触发 load」）。本 story 在 Dev Notes 明确标注此 seam 留给 24.2，并在测试中验证「sink 收到非空 currentInventory → inventory 正确派生」证明闭环。
4. **AC4 — reset / hydrate 路径正确（沿用 Story 37.9 sink 模式，不回归）**
   - `appState.reset()`（用户登出/切身份）把 `currentInventory = []` → sink 派发 `[]` → `self.inventory = []`（空仓库 placeholder）；**不**残留旧 inventory（这是 Story 37.7/37.8 沉淀的 `published-derived-state-needs-publisher-subscription` lesson；本 story 把占位 sink 改实装时**必须**保持 reset → 空 的语义，不能改回一次性 hydrate）。
   - `catName` 派生（`subscribeCatName` ← `appState.$currentPet`）Story 37.9 已落地，本 story **不动**。
5. **AC5 — `RealWardrobeViewModel.init()` / `init(appState:)` seed 调整**
   - Story 37.9 现有两条 init 都 seed `self.inventory = WardrobeScaffoldDefaults.inventory`（mock 占位，让 sink 派发前 view 有数据）。本 story 决策：
     - 保留 `catName / equipped / selectedCategory` 的 `WardrobeScaffoldDefaults` seed（与 Story 37.7/37.8 `real-viewmodel-init-must-seed-scaffold-defaults` lesson 一致——hydrate 前避免空白）。
     - `self.inventory` seed 改为 `[]`（空仓库占位而非 mock 18 件）——理由：本 story 后 Real 路径语义是「真实 inventory」，hydrate 前显示 mock 18 件假装扮会误导用户（开箱前应显示空仓库 placeholder）。Story 37.9 用 mock seed 是因当时无真实数据源；本 story 接通后 seed 应反映真实初值（空）。`init(appState:)` 路径 seed 后立即 `subscribeInventory` 会用 `appState.currentInventory`（启动时通常为空）覆盖，行为一致。
   - **MockWardrobeViewModel 不动**（Preview / 单测 / UITest skip-guest-login 仍走 mock 18 件——保证 Preview 视觉与 UITest 锚不回归）。

**And** **单元测试覆盖**（≥4 case 提案 §5.4 line 417-421 钦定 + 守护 case，`MockAppState` 或真实 `AppState` 注入；测试文件 `iphone/PetAppTests/Features/Wardrobe/RealWardrobeViewModelTests.swift` 新建）：
- **happy（提案 line 418）**：`appState.currentInventory` 含 5 个 hat 类 `HomeEquip` → `RealWardrobeViewModel.inventory` 映射出 5 个 `CosmeticItem` 且 `currentCategoryItems`（selectedCategory=.hat）长度 = 5
- **happy（提案 line 419）**：切换 `selectedCategory` 到 bow（或 mapping 后真实存在的另一 category）→ `currentCategoryItems` 渲染对应分类项（验证 mapping 的 category 字段正确）
- **happy（提案 line 420）**：`appState.currentInventory` 为空 → `RealWardrobeViewModel.inventory == []`（**不**是 `WardrobeScaffoldDefaults.inventory`）→ 验证空态（Story 37.9 已实装空态 placeholder，本 case 断言 ViewModel 层 inventory 为空即可）
- **edge（提案 line 421）**：count > 99 → badge 文案 = "99+"；count = 99 → "99"；count = 0 → "0"（直接断言 AC2 钦定的 `badgeText(forCount:)` 纯函数边界值，不经 view）
- **守护 case A（reset 不回归）**：注入 `appState` + hydrate 非空 inventory → `inventory` 非空；调 `appState.reset()` → sink 派发 `[]` → `inventory == []`（防 `published-derived-state-needs-publisher-subscription` lesson 回归）
- **守护 case B（bind 幂等）**：`RealWardrobeViewModel()` 无参构造 → `bind(appState:)` 注入 → 后续 `appState.currentInventory` 变化能派生（验证 Story 37.9 `bind` idempotent guard 不破坏 inventory sink）

**And** **build verify（必跑）**：`bash iphone/scripts/build.sh --test` 全绿（含既有 ~293 case + 本 story 新增 case 不回归 Story 37.9 的 Wardrobe Scaffold 测试 / UITest）。

**And** **iOS UI 验证（CLAUDE.md「iOS UI 验证（必跑）」红线 —— build pass ≠ 行为正确）**：用 `ios-simulator` MCP 实跑：build → install → launch（terminate_running: true）→ 切到 Wardrobe Tab → `ui_view` 验证（启动游客新账号 inventory 空时显示空仓库 placeholder，**不**显示 Story 37.9 的 18 件 mock 装扮）→ `ui_describe_all` 验证 a11y 锚（`wardrobeView` 等 Story 37.9 已有锚仍在，不回归）。

## Tasks / Subtasks

- [x] **Task 1：核对类型 + mapping 形状（写代码前必做）**（AC: #2）
  - [x] 1.1 读 `iphone/PetApp/Features/Home/Models/HomeData.swift` line 146 `struct HomeEquip` 完整定义，记录其真实字段名 / 类型（slot / rarity / id / name 等的精确签名）——**禁止臆测**
  - [x] 1.2 读 Story 37.6 落地的 `Rarity` enum 定义（`iphone/PetApp/Core/DesignSystem/` 下，grep `enum Rarity`），确认 `{1,2,3,4}` → case 映射
  - [x] 1.3 读 `CosmeticCategory.swift`（已存在）确认 5 case + slot 关系 + 是否已有 iconEmoji 派生
  - [x] 1.4 在 Dev Notes「HomeEquip 字段核对」「slot→category 映射表」「rarity int→enum 映射表」三处补全实际签名（dev 自行回填后再写 mapping，防字段漂移）
- [x] **Task 2：实装 `RealWardrobeViewModel` inventory 真实派生**（AC: #1, #2, #4, #5）
  - [x] 2.1 新增私有 `func mapToCosmeticItems(_ equips: [HomeEquip]) -> [CosmeticItem]`：逐元素按 AC2 mapping 规则转换；slot/rarity 未知值走 fallback case（不 crash、不静默丢失实例）
  - [x] 2.2 改 `subscribeInventory` sink closure：`homeEquips.isEmpty` → `self.inventory = []`；非空 → `self.inventory = mapToCosmeticItems(homeEquips)`（删除占位「保持不动」分支与 `WardrobeScaffoldDefaults.inventory` 退回逻辑）
  - [x] 2.3 改两条 init 的 `self.inventory` seed：`WardrobeScaffoldDefaults.inventory` → `[]`（保留 catName/equipped/selectedCategory 的 Defaults seed）
  - [x] 2.4 更新 `RealWardrobeViewModel.swift` 文件头注释：删除「占位 sink / Story 24.1 决定 mapping shape」字样，改为本 story 落地的真实 mapping 说明 + 标注 Story 24.2 负责 `LoadInventoryUseCase` 写 `appState.currentInventory` 这一上游触发
- [x] **Task 3：`99+` count clamp（纯函数）**（AC: #2）
  - [x] 3.1 新增可单测纯函数 `badgeText(forCount:) -> String`（`count > 99 ? "99+" : "\(count)"`），放 `CosmeticCategory`
  - [x] 3.2 `WardrobeScaffoldView.swift` line 234 category badge 文案改调该函数（37.9 未做 clamp，本 story 新增；这是本 story 唯一允许触碰 `WardrobeScaffoldView` 的点）
- [x] **Task 4：单元测试**（AC: 测试覆盖）
  - [x] 4.1 新建 `iphone/PetAppTests/Features/Wardrobe/RealWardrobeViewModelTests.swift`（`@MainActor` + XCTest，**禁止** import SnapshotTesting / ViewInspector——ADR-0002 §3.1 测试栈钦定）
  - [x] 4.2 覆盖 ≥4 提案钦定 case + 守护 case A（reset）+ 守护 case B（bind 幂等）+ 守护 case 未知 slot/rarity fallback
  - [x] 4.3 `99+` clamp 测试：clamp 抽为 `CosmeticCategory.badgeText(forCount:)` 纯函数再断言（0/1/99/100/9999 边界）；不引入 ViewInspector
- [x] **Task 5：build + 模拟器实跑验证**（AC: build verify + iOS UI 验证）
  - [x] 5.1 `bash iphone/scripts/build.sh --test` 全绿（741 case 全过，含 Story 37.9 既有 Wardrobe Scaffold 测试 / UITest 不回归——3 处 inventory-seed 相关断言按 AC5 新契约更新）
  - [x] 5.2 `ios-simulator` MCP：build → install_app → launch_app(terminate_running: true) → 真实游客登录 → 切 Wardrobe Tab → `ui_describe_all` + 截图（新游客账号 inventory 空 → 空仓库 placeholder：5 分类 badge 全 `0` / 预览「未选择」/ 装备按钮 disabled / 无 `wardrobeItem_*` grid cell，**非** 18 件 mock）；`wardrobeView` + 5 `wardrobeCategory_*` + diamond + compose + equip 等 Story 37.9 a11y 锚仍在
  - [x] 5.3 截图/描述记录到 Completion Notes（CLAUDE.md 红线满足）
- [x] **Task 6：收尾**
  - [x] 6.1 更新本 story File List（实际改动文件）+ Completion Notes（每 AC 一条）+ Change Log
  - [x] 6.2 `sprint-status.yaml` `24-1-仓库页-swiftui-骨架: in-progress → review`（dev-story 完成）

## Dev Notes

### 本 story 一句话定位

Story 37.9 把 `RealWardrobeViewModel.subscribeInventory` 留成「订阅了 `appState.$currentInventory` 但 closure 内只占位（空→mock / 非空→不动）」的半成品 sink；本 story 把 closure 实装为真实 `[HomeEquip] → [CosmeticItem]` mapping，并把 init seed 从 mock 18 件改空数组——使「Story 24.2 写 `appState.currentInventory` → Wardrobe 显示真实装扮」闭环成立。**view / caller / 注入路径全部零 edit（除 `99+` clamp 一处）。**

### 关键约束 / 类型决策（最易踩坑点）

- **`appState.currentInventory` 类型是 `[HomeEquip]` 不是 `[CosmeticInstance]`**：ADR-0010 §3.2（line 105）写 `currentInventory: [CosmeticInstance]` 是**设计意向**，但 `AppState.swift` line 70-73 实际落地是 `@Published public var currentInventory: [HomeEquip] = []`（节点 1 占位「直接复用 Home* 类型族，避免预创建空类型签名影响测试；后续节点接入新 epic 时如发现需要非 Home* 派生的领域类型再做演进」——ADR-0010 §4.4 缓解策略）。**本 story 按真实代码 `[HomeEquip]` 写 mapping，不引入 `CosmeticInstance` 新类型**（引新类型会牵动 AppState/applyHomeData/reset/测试，越界且 37-9 红线 line 1403 明确「不预 over-design」）。Story 24.2 接 `LoadInventoryUseCase` 时若发现 `HomeEquip` 不足以承载 server `GET /cosmetics/inventory` 的 `groups/instances/count` 全量字段，由 24.2 决定是否演进 AppState 类型——**不是本 story 的事**。
- **HomeEquip 字段核对**（dev 在 Task 1.1 回填实际签名后再写 mapping）：`iphone/PetApp/Features/Home/Models/HomeData.swift:146 struct HomeEquip`。已知它 `Equatable + Sendable`。字段需 dev 亲自读源确认（V1 §5.1 GET /home 的 `pet.equips[]` schema 提示其形态为 `{slot:int, userCosmeticItemId:string, cosmeticItemId:string, name:string, rarity:int, assetUrl:string}`——但**以 Swift struct 实际定义为准**，不以 wire schema 臆测 Swift 字段名）。
- **slot int → CosmeticCategory 映射表**（dev Task 1.3 读 `CosmeticCategory.swift` 确认 case + 关联后回填）：V1 §6.8 slot 枚举 `{1,2,3,4,5,6,7,99}`；`CosmeticCategory` 5 case（`hat / bow / scarf / outfit / bg`，见 37-9 文件 line 24 spec）。dev 需确认 `CosmeticCategory` 是否已有 `init(slot:)` / rawValue 关联；未知/无对应 slot → fallback 到某一 case（不丢实例，宁可错分类也不静默丢——参考 V1 §8.2「已拥有不得静默丢失」精神，虽 server 侧约束本 story 是 client mapping 仍守同精神）。
- **rarity int → Rarity enum 映射**：V1 §6.9 rarity `{1,2,3,4}`；Story 37.6 落地 `Rarity` enum（`N/R/SR/SSR`，dev grep `enum Rarity` 确认 case 名 + 是否有 `init(rawValue:)`/`init(serverValue:)`）。未知值 fallback `.n`（最低品质，视觉降级而非 crash）。
- **count 聚合不在 ViewModel 做**：Story 37.9 `WardrobeScaffoldView` line 228 用 `state.inventory.filter { $0.category == category }.count` 算 badge，line 268 `ForEach(state.currentCategoryItems)` 渲染 grid。本 story mapping 后**每个实例一个 `CosmeticItem`**（不去重不聚合），count 由 view filter().count 自然得出——与 37-9 渲染契约一致。V1 §8.2 server 侧 `groups[].count = instances 数组长度`，client 侧等价于「同 cosmetic 多实例平铺后 filter count」。**不要**在 ViewModel 做 group-by 聚合（会破坏 37-9 既有 grid 渲染数据源契约 + 越界）。
- **`99+` clamp 是本 story 唯一允许改 `WardrobeScaffoldView` 的点**：提案 §5.4 line 421 edge case 钦定 count>99 → "99+"。其余 `WardrobeScaffoldView` 视觉/布局/a11y 锚全部 Story 37.9 冻结，零 edit（回归即 fail）。

### 测试栈约束（ADR-0002 §3.1 钦定 —— 不可违反）

- **XCTest only**：`iphone/PetAppTests/` 下测试**禁止** import `SnapshotTesting` / `ViewInspector`（Epic 37 §AC 红线 + ADR-0002 §3.1；Story 37.7/37.8/37.9 全部遵守）。
- `99+` clamp 若在 view 层无法纯 XCTest 断言 → **抽 clamp 为可单测纯函数**（如在 `CosmeticCategory` 或 ViewModel 上加 `func badgeText(forCount:) -> String` / `static func clampCount(_:) -> String`），view 调该函数，测试断言函数——这是 Story 37.x 系列处理「view 文案逻辑需单测」的既定模式（参考 Story 37.7 greeting 文案抽函数）。
- `@MainActor` 测试 + `MockAppState` 或真实 `AppState()` 注入（`AppState` 是 `@MainActor` ObservableObject，`init()` 无参可直接构造；hydrate 用 `appState.applyHomeData(_:)` 或直接设 `appState.currentInventory = [...]`——后者更适合本 story 单测，因本 story 只关心 inventory sink）。

### Story 37.7 / 37.8 / 37.9 沉淀 lesson（预防性应用，**不重蹈覆辙**）

- `docs/lessons/2026-04-30-published-derived-state-needs-publisher-subscription.md`：派生 state 必须订阅 publisher 而非一次性 hydrate——本 story 改 sink closure 时**保持** sink 路径（不能图省事改成「init 时一次性读 appState.currentInventory」），否则 reset 后 inventory stale（守护 case A 专测此回归）。
- `docs/lessons/2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md`：RealViewModel.init 必须 seed scaffold defaults 避免 hydrate 前空白——本 story **部分**调整此 lesson 的应用：`catName/equipped/selectedCategory` 仍 seed defaults（保留 lesson 精神：hydrate 前 UI 不空白崩溃），但 `inventory` seed 改 `[]`（语义正确性优先：本 story 后 Real 路径「真实 inventory」语义下，hydrate 前显示 mock 18 件假装扮会误导用户——空仓库 placeholder 才是正确初值）。在 `RealWardrobeViewModel.swift` 文件头注释明确记录此偏离理由（防 codex review 误判为「未遵守 seed lesson」）。
- `docs/lessons/2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`：本 story **不涉及** `onEquipTap`（那是 Story 27.1）——`RealWardrobeViewModel.onEquipTap` Story 37.9 round 1 P1 已修为本地 mutate equipped，本 story 零 edit 该 override。
- `docs/lessons/2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md`：RootView wire 用 `RealWardrobeViewModel()` 而非裸基类——Story 37.9 已落地，本 story 不动 RootView。
- `docs/lessons/2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md`：`.onAppear` 同步 bind appState——Story 37.9 已落地 RootView `.onAppear` 内 `bind(appState:)`，本 story 不动。

### Project Structure Notes

- 改动文件：`iphone/PetApp/Features/Wardrobe/ViewModels/RealWardrobeViewModel.swift`（核心）；可能 `iphone/PetApp/Features/Wardrobe/Views/WardrobeScaffoldView.swift`（仅 `99+` clamp 一处，若 37.9 未做）；可能 `iphone/PetApp/Features/Wardrobe/Models/CosmeticCategory.swift`（若需加 `init(slot:)` / `clampCount` 纯函数）。
- 新建文件：`iphone/PetAppTests/Features/Wardrobe/RealWardrobeViewModelTests.swift`（Story 37.9 已有 `iphone/PetAppTests/Features/Wardrobe/` 目录与 `WardrobeViewScaffoldTests.swift`；本测试与之同目录）。
- **不**新建 `LoadInventoryUseCase` / `InventoryRepository` / endpoint / DTO（Story 24.2 范围；37-9 红线 line 1399-1403 明确）。
- 全部走 `iphone/project.yml` 通配 inclusion；**不**改 `project.yml`（xcodegen regen 仅在新增文件后跑，`iphone/scripts/build.sh` 内部处理）。
- iOS 工程结构遵循 `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §Features 三层（ViewModels/UseCases/Repositories）；本 story 仅触 ViewModels 层 + 测试。

### 边界声明（防 scope creep）

- **不**实装 `LoadInventoryUseCase` / `GET /cosmetics/inventory` 调用（Story 24.2）。
- **不**实装筛选（按品质/槽位 segment control —— Story 24.3）。
- **不**实装穿戴按钮行为（Story 24.5 占位预留 / Story 27.1 真实）。
- **不**改 `AppState` 类型 / `applyHomeData` / `reset`（节点 1 占位类型沿用；Story 24.2 若需类型演进由 24.2 决策）。
- **不**改 `WardrobeView.swift` / `RootView.swift` / `MainTabView.swift`（Story 37.9 wire 已 final）。
- **不**改 `MockWardrobeViewModel`（Preview / UITest / Mock 单测仍走 18 件 mock，不回归 37.9 Preview 视觉与 UITest 锚）。

### References

- [Source: _bmad-output/planning-artifacts/sprint-change-proposal-2026-04-29-v2.md §5.4 (line 396-424)] —— **本 story 唯一权威 AC 来源**（缩窄范围 + Mock→Real + ≥4 单测 case 钦定）
- [Source: _bmad-output/planning-artifacts/epics.md §Story 24.1 (line 3322-3350)] —— epic 原文（含 2026-04-30 supersede 变更注；Sheet/InventoryView 语义已作废，仅留 user story 业务意图）
- [Source: _bmad-output/implementation-artifacts/37-9-wardrobeview-scaffold.md §「与 Story 24.1 衔接的红线」(line 1395-1403) + AC2 RealWardrobeViewModel (line 264-371) + File List (line 1465-1483)] —— 前置 story 交付物 + 本 story 接续点钦定（不预 over-design）
- [Source: iphone/PetApp/Features/Wardrobe/ViewModels/RealWardrobeViewModel.swift (line 99-120 占位 subscribeInventory / line 48-72 两条 init seed)] —— 本 story 核心改动点
- [Source: iphone/PetApp/Features/Wardrobe/ViewModels/WardrobeViewModel.swift] —— 基类 5 @Published + currentCategoryItems/activeItem 派生 computed（本 story 不改基类）
- [Source: iphone/PetApp/Features/Wardrobe/Models/CosmeticItem.swift / CosmeticCategory.swift] —— 目标 mapping value type（Story 37.9 落地，本 story 不改 struct）
- [Source: iphone/PetApp/Features/Wardrobe/Views/WardrobeScaffoldView.swift (line 228 badge filter().count / line 268 grid ForEach)] —— count 由 view filter 得出（本 story 仅 `99+` clamp 一处可改）
- [Source: iphone/PetApp/App/AppState.swift (line 70-123: currentInventory [HomeEquip] / applyHomeData / reset)] —— `currentInventory` 真实类型 `[HomeEquip]`（非 ADR-0010 意向的 CosmeticInstance）+ reset 清空语义
- [Source: iphone/PetApp/Features/Home/Models/HomeData.swift:146 struct HomeEquip] —— mapping 源类型（dev Task 1.1 必读实际字段签名）
- [Source: docs/宠物互动App_V1接口设计.md §8.2 (line 1311-1444)] —— `GET /cosmetics/inventory` 契约（slot `{1,2,3,4,5,6,7,99}` / rarity `{1,2,3,4}` / count=instances 长度 / 空背包 `{groups:[]}` / 两级全序排序）；**本 story 不调此接口**（Story 24.2），仅作 mapping 语义对齐参考
- [Source: _bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md] —— ADR-0009 推翻 Sheet 主入口（Wardrobe = 4 Tab 之一，本 story 不再有「主界面仓库按钮」语义）
- [Source: _bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md §3.2 §3.3 §3.5 §3.7 §4.4] —— AppState 白名单（currentInventory 节点 8 起）+ hydrate/reset 流程 + view-specific transient 边界 + §4.4 类型占位缓解策略（解释为何实际是 [HomeEquip]）
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md §3.1] —— 测试栈 XCTest only（禁 SnapshotTesting/ViewInspector）
- [Source: docs/lessons/2026-04-30-published-derived-state-needs-publisher-subscription.md] —— sink 路径不可退化为一次性 hydrate（守护 case A 防回归）
- [Source: docs/lessons/2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md] —— init seed defaults lesson（本 story 对 inventory 字段有意偏离，理由见 Dev Notes）
- [Source: CLAUDE.md §「iOS UI 验证（必跑）」] —— build pass ≠ 行为正确；必须 ios-simulator MCP 实跑验证空仓库 placeholder
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md §Features 三层] —— iOS 工程结构（本 story 仅 ViewModels 层 + 测试）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m] (Opus 4.7, 1M context)

### Debug Log References

- 类型核对（Task 1.4 回填实际签名，写 mapping 前已确认源码，**未臆测**）：
  - **HomeEquip 字段核对**（`iphone/PetApp/Features/Home/Models/HomeData.swift:146`）：
    `struct HomeEquip: Equatable, Sendable { slot: Int; userCosmeticItemId: String; cosmeticItemId: String; name: String; rarity: Int; assetUrl: String }`。
    实例级唯一 id 用 `userCosmeticItemId`（同 cosmetic 多实例各独立）。
  - **slot int → CosmeticCategory 映射表**（V1 §6.8 slot 枚举 `{1,2,3,4,5,6,7,99}` = `1=hat/2=gloves/3=glasses/4=neck/5=back/6=body/7=tail/99=other`；client `CosmeticCategory` 仅 5 桶 `hat/bow/scarf/outfit/bg`，需归并）：
    `slot 1 → .hat` / `slot 4(neck) → .scarf` / `slot 6(body) → .outfit` / `slot 5(back) → .bg` / `slot 2/3/7/99 + 任何未知值 → .bow`（饰品=配饰兜底桶，不丢实例）。实装为 `CosmeticCategory.category(forSlot:)` 静态方法。
  - **rarity int → Rarity enum 映射表**（V1 §6.9 `{1,2,3,4}`；`Rarity` enum 实际是 `.N/.R/.SR/.SSR` **大写**，rawValue 是 `String` 非 `Int`——故不能用 `Rarity(rawValue:)`，显式 switch 映射）：
    `1→.N / 2→.R / 3→.SR / 4→.SSR / 未知→.N`（最低品质视觉降级，不 crash）。
    **注**：story Dev Notes 多处写 fallback `.n`（小写），实际 enum case 是 `.N`（大写）——按真实源码用 `.N`。
- `CosmeticCategory` 已有 `iconEmoji` 派生（line 31-39），`iconEmoji` 字段复用 `category.iconEmoji`（占位，Story 30.x 接真实 sprite 升级）。
- build verify：`bash iphone/scripts/build.sh` BUILD SUCCEEDED；`bash iphone/scripts/build.sh --test` 全 741 case 0 failure（第一轮有 1 failure `testRealWardrobeViewModelOnEquipTapTogglesEquipped` ——因 AC5 init seed 改 [] 后该既有 case 依赖 seeded mock inventory force-unwrap 失败；按测试真实意图改为直接构造 CosmeticItem 传入 onEquipTap，重跑全绿）。

### Completion Notes List

- **AC1 ✅ `RealWardrobeViewModel.subscribeInventory` 真实 inventory 派生**：sink 订阅 `appState.$currentInventory`（`[HomeEquip]`），closure `homeEquips.isEmpty → self.inventory = []`（空仓库走 37.9 空态 placeholder，**不**再退回 `WardrobeScaffoldDefaults.inventory`）；非空 → `self.inventory = mapToCosmeticItems(homeEquips)`。mapping 抽独立私有方法 `mapToCosmeticItems(_:)` 便于单测直接断言。
- **AC2 ✅ `HomeEquip → CosmeticItem` mapping 形状钦定**：`id ← userCosmeticItemId`（实例级唯一，多实例各独立不去重）/ `name ← name` / `category ← CosmeticCategory.category(forSlot: slot)`（V1 §6.8 8 slot 归并 5 桶，未知 slot fallback `.bow` 不丢实例）/ `rarity ← rarity(forServerValue:)`（`{1,2,3,4}→.N/.R/.SR/.SSR`，未知 fallback `.N`）/ `owned ← true`（恒真）/ `iconEmoji ← category.iconEmoji`（占位）。count 聚合**不在 ViewModel**做，由 `WardrobeScaffoldView` filter().count 自然得出（37.9 渲染契约一致）。`99+` clamp 抽为 `CosmeticCategory.badgeText(forCount:)` 纯函数，`WardrobeScaffoldView.swift` line 234 改调（37.9 原是裸 `Text("\(count)")` 无 clamp，本 story 新增；这是唯一触碰 `WardrobeScaffoldView` 的点，其余视图代码零 edit）。
- **AC3 ✅ Wardrobe Tab 首次出现自动刷 AppState 的 seam**：`LoadInventoryUseCase` 本体属 Story 24.2——本 story **未**创建 UseCase/Repository/endpoint/DTO，**未**加 `.task`/`.onAppear` 触发 load。仅实装 sink closure mapping 完成「Story 24.2 写 `appState.currentInventory` → Wardrobe 零 edit 显示」闭环；单测 case#1/case#6 验证「sink 收到非空 currentInventory → inventory 正确派生」证明闭环成立。
- **AC4 ✅ reset / hydrate 路径正确（沿用 37.9 sink 模式，不回归）**：sink 路径保持（**未**退化为一次性 hydrate）；`appState.reset()` 把 `currentInventory=[]` → sink 派发 `[]` → `self.inventory=[]`（不残留旧 inventory）。单测守护 case A 专测此回归。`catName`（`subscribeCatName` ← `$currentPet`）零 edit；模拟器实跑验证「默认小猫 的衣柜」标题正确（catName sink 工作）。
- **AC5 ✅ `init()` / `init(appState:)` seed 调整**：保留 `catName/equipped/selectedCategory` 的 `WardrobeScaffoldDefaults` seed（hydrate 前 UI 不空白崩溃）；`self.inventory` seed `WardrobeScaffoldDefaults.inventory` → `[]`（真实 inventory 语义下空仓库 placeholder 才是正确初值）。**此偏离 `real-viewmodel-init-must-seed-scaffold-defaults` lesson 是有意决策**，理由已在 `RealWardrobeViewModel.swift` 文件头注释明确记录（防 codex review 误判）。`MockWardrobeViewModel` **零 edit**（Preview/Mock 单测仍走 18 件 mock 不回归）。
- **测试覆盖 ✅**：新建 `RealWardrobeViewModelTests.swift` 7 case（@MainActor + XCTest only，**无** SnapshotTesting/ViewInspector）：happy 5-hat 派生 / happy 切 selectedCategory 验 category 字段 / happy 空 inventory → `[]`（非 mock）/ edge `badgeText` 边界值 0/1/99/100/9999 / 守护 A reset 不回归 / 守护 B bind 幂等 / 守护 未知 slot+rarity fallback 不丢实例。既有 `WardrobeViewScaffoldTests.swift` 3 处依赖旧 mock-inventory-seed 的断言（case#7/#8/#11）按 AC5 新契约更新；`HomeUITests.swift` `wardrobeItem_h1` 断言按 AC4/AC5 改为「Real 空 inventory 不应出现 mock item」（结构 a11y 锚仍保留验证）。
- **iOS UI 验证 ✅（CLAUDE.md 红线满足，非仅 build pass）**：iPhone 17 Pro 模拟器（UDID EC54A222...），真实游客登录流程（**未**用 UITEST_SKIP_GUEST_LOGIN）→ install → launch(terminate_running:true) → 切 Wardrobe Tab。`ui_describe_all` + 截图 `/tmp/wardrobe_empty.png` 确认：5 分类 badge 全显示 `0`（`"🎩, 帽子, 0"` 等，`badgeText` 纯函数生效）/ 预览区「未选择」/ 装备按钮 disabled（无 activeItem）/ **无任何 `wardrobeItem_*` grid cell**（mock 18 件未渲染）；`wardrobeView` 容器 + 5 `wardrobeCategory_*` + diamond(248) + 合成按钮 + 装备按钮 等 Story 37.9 a11y 锚全部仍在（零回归）；标题「默认小猫 的衣柜」（catName 派生正确）。结论：真实空 inventory → 空仓库 placeholder（非 37.9 mock 18 件），符合 AC。（注：idb companion 启动后首次 mach-port 断连，已 `idb kill + idb connect` 重连，重连后 a11y 树 + tap 正常。）

### File List

- `iphone/PetApp/Features/Wardrobe/ViewModels/RealWardrobeViewModel.swift`（改：subscribeInventory 真实 mapping + mapToCosmeticItems/rarity 私有方法 + 两条 init inventory seed → [] + 文件头注释重写）
- `iphone/PetApp/Features/Wardrobe/Models/CosmeticCategory.swift`（改：新增 `category(forSlot:)` slot→category 映射静态方法 + `badgeText(forCount:)` 纯函数）
- `iphone/PetApp/Features/Wardrobe/Views/WardrobeScaffoldView.swift`（改：line 234 badge 文案 `Text("\(count)")` → `Text(CosmeticCategory.badgeText(forCount: count))`；唯一允许触碰点）
- `iphone/PetAppTests/Features/Wardrobe/RealWardrobeViewModelTests.swift`（**新建**：7 case 单元测试）
- `iphone/PetAppTests/Features/Wardrobe/WardrobeViewScaffoldTests.swift`（改：case#7/#8/#11 按 AC5 新 inventory-seed 契约更新断言，不回归测试意图）
- `iphone/PetAppUITests/HomeUITests.swift`（改：`testWardrobeScaffoldShowsAllAnchors` 的 `wardrobeItem_h1` 断言按 AC4/AC5 改为「Real 空 inventory 不应出现 mock item」，保留结构 a11y 锚验证）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（改：24-1 ready-for-dev → in-progress → review）
- `_bmad-output/implementation-artifacts/24-1-仓库页-swiftui-骨架.md`（改：Tasks 勾选 + Dev Agent Record + Status → review）

### Change Log

| 日期 | 变更 | 作者 |
|---|---|---|
| 2026-05-17 | Story 创建（create-story；范围按 sprint-change-proposal-2026-04-29-v2 §5.4 + 37-9 衔接红线锚定，缩窄为「Mock→Real inventory 派生」窄范围 story） | Bob (SM) |
| 2026-05-17 | dev-story 实装：RealWardrobeViewModel.subscribeInventory 真实 [HomeEquip]→[CosmeticItem] mapping（mapToCosmeticItems/rarity 私有方法）+ 两条 init inventory seed → [] + CosmeticCategory.category(forSlot:)/badgeText(forCount:) 纯函数 + WardrobeScaffoldView line 234 99+ clamp 接入 + 新建 RealWardrobeViewModelTests 7 case；既有 WardrobeViewScaffoldTests/HomeUITests 3+1 处旧 mock-seed 断言按 AC5/AC4 新契约更新（不回归测试意图）；741 单测全绿 + iPhone 17 Pro 模拟器真实游客登录实跑验证空仓库 placeholder（badge 全 0 / 无 wardrobeItem grid / 37.9 a11y 锚零回归）。Status → review | Amelia (Dev) |
