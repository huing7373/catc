# ADR-0009: iPhone 导航架构改用 TabView（推翻 Story 2.3 主入口模式）

- **Status**: Proposed（待用户终审 + Story 37.1 落地后改 Accepted）
- **Date**: 2026-04-29
- **Decider**: Developer
- **Supersedes**: Story 2.3 已 done 决策的「主入口 NavigationStack + 全屏 Sheet」**主入口部分**（次级 sheet 场景仍保留）；ADR-0002 §3.3 不变
- **Related**: ADR-0010（AppState 单 source of truth，本 ADR 联动）；Story 37.1（本 ADR）；Story 37.3（实装 RootView TabView 改造）；Story 12.7 / 24.1 / 33.1 / 35.x（下游入口改写）

---

## 1. Context

### 1.1 现状

Story 2.3「导航架构搭建」已 done，钦定的主入口模式是：

- 主界面 = HomeView（含 3 个 CTA 按钮：进入房间 / 仓库 / 合成）
- 点击 CTA → `AppCoordinator.present(.room/.wardrobe/.compose)` → `RootView .fullScreenCover(item: $coordinator.presentedSheet)` 弹全屏 Sheet
- Sheet 内 placeholder（`Text("Room Placeholder")` 等）
- 后续 Epic 12.7 / 24.1 / 33.1 / 35.x 都假设这个 Sheet 模式作为入口

物理产物（已落地）：
- `iphone/PetApp/App/AppCoordinator.swift`（`@Published var presentedSheet: SheetType?`）
- `iphone/PetApp/App/RootView.swift` 的 `.ready` 分支用 `.fullScreenCover(item:)` 路由 sheet
- `iphone/PetApp/Features/Home/Views/HomeView.swift` 的 3 按钮 CTA 模式

### 1.2 矛盾点

`iphone/ui_design/README.md`（设计参考）锁定的产品 IA 是：

- 底部 4 Tab：家（Home）/ 仓库（Wardrobe）/ 好友（Friends）/ 我的（Profile）
- 家 Tab 内有 idle ⟷ inRoom 互斥状态：
  - 未加入队伍 → HomeScreen（养成界面 + 创建/加入队伍按钮）
  - 已加入队伍 → RoomScreen（队伍房间界面）**完全替换** Home，Tab 仍叫"家"

**冲突**：Story 2.3 主入口 Sheet 模式与 ui_design 4 Tab IA 不相容。Sprint Change Proposal v1（2026-04-29）想引入 4 Tab 但保留 Sheet 模式，导致 Story 12.7（房间入口经 Sheet 弹出 RoomView）与 Epic 37（房间在 Home Tab 内互斥替换）当场打架。codex 审 v1 给 Reject，BLOCKER 4 命中此矛盾。

### 1.3 用户决议

2026-04-29 用户在 Sprint Change Proposal v2 前置决议中选 #1=A：**全 TabView 推翻 Story 2.3**。

本 ADR 落实该决议，定义新的导航架构契约。

---

## 2. Decision Summary

| 领域 | 选定 | 备注 |
|---|---|---|
| **主入口模型** | **TabView**（4 Tab：家 / 仓库 / 好友 / 我的） | 推翻 Story 2.3 的 3 CTA + Sheet 主入口模式 |
| **Home Tab 互斥** | HomeContainerView 根据 `AppState.currentRoomId` 在 HomeView ↔ RoomView 切换 | "Tab 仍是家但视图切换" |
| **Tab 内导航** | NavigationStack（沿用 Story 2.3 push 模板） | 仅 Story 2.3 的 push 模式保留 |
| **Sheet / fullScreenCover 保留范围** | 仅次级场景：launching / needsAuth / JoinRoomModal / 设置详情 / 合成 sub-flow / 奖励弹窗 / Profile 微信绑定 Modal 等 | Sheet 不再用作主入口 |
| **AppCoordinator 角色变化** | `presentedSheet` 从「主入口路由」缩小为「次级 sheet 路由」；**新增 `currentTab: Tab` 字段**作为 TabView selection binding 的 single source of truth；主入口路由（哪个 Tab 当前激活）由 `AppCoordinator.currentTab` 控制 | 不删除 AppCoordinator，扩展为统一的 UI 导航协调中心 |

---

## 3. Decisions

### 3.1 主入口 = TabView 4 Tab

- **选定**：根视图改为 `TabView` 含 4 Tab，每个 Tab 含一个独立的 NavigationStack：
  - Tab 0 · 家 · `HomeContainerView`（Home / Room 互斥，§3.2）
  - Tab 1 · 仓库 · `WardrobeView`（节点 8 起填充真实数据）
  - Tab 2 · 好友 · `FriendsView`（节点 4-5 起填充真实数据）
  - Tab 3 · 我的 · `ProfileView`（节点 2 起填充真实数据，本期含微信绑定 UI 视觉壳）

- **Tab 视觉规格**（按 ui_design/README §iOS 设备规格）：
  - 浮动 TabBar，高 72pt，距底 14pt，距左右各 12pt
  - theme.shadow.md 阴影 + Card 圆角背景
  - 当前选中 Tab：accent 色高亮 + 缩放 1.1x；其余：ink-mute 灰

- **理由**：
  1. 与 ui_design IA 像素级对齐（README §App 结构）
  2. 4 Tab 比 3 CTA + Sheet 更符合 iOS 主流应用习惯
  3. Tab 切换天然支持状态保留（用户从 Wardrobe 切到 Home 再切回 Wardrobe，scroll position / selected category 保留），Sheet 模式做不到这点
  4. Tab 间数据共享通过 AppState（ADR-0010）天然解决

- **否决候选**：
  - **保留 Sheet 模式（v1 老方案）**：否决 — 用户决议 #1=A 明确推翻；与 ui_design 不容
  - **抽屉式（drawer/hamburger）**：否决 — ui_design 无此 IA；iOS HIG 不推荐
  - **单视图全屏 + 滑动切换**：否决 — 4 屏内容差异大，单视图组织混乱；TabView 是 iOS HIG 推荐

- **已知坑**：
  - **TabView 默认 iOS Tab Bar 与 ui_design 浮动样式不同**：缓解 — 用 `TabView` 隐藏默认 TabBar（`.toolbar(.hidden, for: .tabBar)` 或 iOS 16+ `tabViewStyle`），自绘浮动 TabBar overlay
  - **Tab 切换 + Sheet 弹出共存**：缓解 — Sheet 通过 `.fullScreenCover` / `.sheet` 挂在每个 Tab 内部 NavigationStack 上（按 SwiftUI 推荐做法），不挂 TabView 之上

### 3.2 Home Tab 互斥模式 = `HomeContainerView` 内部状态机

- **选定**：Home Tab 的根视图是 `HomeContainerView`，内部根据 `AppState.currentRoomId` 切换：
  ```
  if appState.currentRoomId == nil → HomeView (idle 态)
  if appState.currentRoomId != nil → RoomView (inRoom 态)
  ```
- **不创建独立 Room Tab**：保持 4 Tab 不变，符合 ui_design "Tab 仍叫家但视图切换"
- **过渡动画**：`.transition(.opacity.combined(with: .move(edge: .bottom)))`，0.3s ease（沿用 ui_design Modal 出现动画 spec）
- **理由**：
  1. 与 ui_design "RoomScreen 完全替换 HomeScreen" 描述一致
  2. 用户心智简化（始终是"家"，不需要学"房间是另一个 Tab"）
  3. Tab Bar 在 inRoom 态仍可见，用户可切换到 Wardrobe / Friends 但 Home Tab 仍保持 inRoom 视图

- **否决候选**：
  - **新建 Room Tab（5 Tab）**：否决 — 偏离 ui_design 的 4 Tab + 互斥设计
  - **fullScreenCover 弹 RoomView**：否决 — 这就是 v1 / Story 12.7 老方案，不符合"Tab 内互斥替换"语义
  - **NavigationLink push RoomView**：否决 — Push 模式有"返回箭头"，与 ui_design"独立离开房间按钮"语义不同

### 3.3 Sheet / fullScreenCover 保留白名单

仅以下场景仍用 Sheet / fullScreenCover：

| 场景 | Modifier | 挂载点 | 备注 |
|---|---|---|---|
| Launching / NeedsAuth | `.fullScreenCover` 或直接 RootView 三态 switch | RootView 顶层（不在 TabView 内） | 沿用 Story 2.9 三态机；Tab 在 launching/needsAuth 阶段不显示 |
| JoinRoomModal | `.sheet` (bottom sheet) | HomeView 内（Home Tab 内部） | 按 ui_design Modal 规格：底部弹出 + 遮罩 0.45 + 上滑 0.3s |
| 合成页 | `.sheet` 或 NavigationLink push | Wardrobe Tab 内部 | Story 33.1 决定具体形式（建议 push） |
| 奖励弹窗（开箱） | `.fullScreenCover` 或自定义 overlay | Home Tab 内部（HomeView 之上） | Story 21.4 决定 |
| Profile 设置详情 | NavigationLink push | Profile Tab 内部 NavigationStack | 按 ui_design "更多菜单"四项 |
| Profile 微信绑定 Modal | `.sheet` (bottom sheet) | Profile Tab 内部 | 按 ui_design/wechat_binding.md（本期视觉壳，按钮 toast） |

**禁用场景**：
- ❌ 主入口 fullScreenCover（4 Tab 直接路由）
- ❌ 跨 Tab 全屏覆盖（如 Wardrobe Tab 内的 Sheet 不能盖住 TabBar）

### 3.4 AppCoordinator 角色变化

- **保留**：AppCoordinator 类不删
- **缩窄职责**：
  - 旧职责（Story 2.3）：主入口 sheet 路由（`presentedSheet: SheetType?` 含 .room / .wardrobe / .compose）
  - 新职责（本 ADR 后）：
    - `presentedSheet` 仅含次级 sheet（.compose / .achievementDetail 等）
    - **新增 `currentTab: Tab` @Published**（Tab enum 含 .home / .wardrobe / .friends / .profile）作为 TabView selection 的 single source of truth
    - 主入口路由由 `TabView(selection: $coordinator.currentTab)` 控制（**注意**：不在 AppState — currentTab 是 UI 状态，不属 domain；ADR-0010 §3.2 表格脚注已对齐）
    - 程式化切 Tab（如深 link、跨 ViewModel 跳转）通过 `coordinator.switchTab(.wardrobe)` 方法
    - Home/Room 互斥由 `HomeContainerView` 内部状态机控制（不经 Coordinator）
- **删除**：`SheetType.room`、`SheetType.wardrobe` 枚举 case（这两个不再是 sheet）
- **保留**：`SheetType.compose`（合成仍可能是 sheet，由 Story 33.1 决定）

### 3.5 RootView 改造步骤（Story 37.3 落地依据）

按以下步骤改造 `iphone/PetApp/App/RootView.swift`：

| 步骤 | 改动 |
|---|---|
| 1 | RootView `.ready` 分支视图从 `HomeView { ... }.fullScreenCover(item: $coordinator.presentedSheet) { ... }` 改为 `MainTabView()` |
| 2 | 新建 `iphone/PetApp/App/MainTabView.swift`：含 `TabView(selection: $coordinator.currentTab) { HomeContainerView().tag(Tab.home); WardrobeView().tag(Tab.wardrobe); FriendsView().tag(Tab.friends); ProfileView().tag(Tab.profile) }` + 浮动自绘 TabBar overlay；MainTabView 持 `@EnvironmentObject var coordinator: AppCoordinator` |
| 3 | 新建 `iphone/PetApp/Features/Home/Views/HomeContainerView.swift`：根据 `AppState.currentRoomId` 切换 HomeView / RoomView |
| 4 | HomeView 内 3 按钮 CTA 删除：「进入房间」改为 TeamIdleCard 的"创建/加入队伍"两按钮（按 ui_design）；「仓库」「合成」按钮删除（用户改用 Tab Bar 切换到 Wardrobe Tab）|
| 5 | AppCoordinator.SheetType 删除 .room / .wardrobe case；保留 .compose（视 Story 33.1 决定） |
| 6 | 每个 Tab 根视图内嵌 NavigationStack（保留 Story 2.3 push 模板）|
| 7 | JoinRoomModal 改用 `.sheet` 挂在 HomeView 内（不在 RootView） |
| 8 | launching / needsAuth 三态机保留不动（仍是 RootView 顶层） |

---

## 4. Consequences

### 4.1 对 Story 2.3 的影响：partial revert

**保留**：
- NavigationStack push 模板（Tab 内部沿用）
- Sheet 模式（次级场景沿用，含 JoinRoomModal / 合成 / 奖励弹窗 / 设置详情 / 微信绑定 Modal）
- AppCoordinator 类
- accessibility identifier 命名规范

**推翻**：
- 主界面 3 CTA 模式（进入房间 / 仓库 / 合成 三按钮）
- AppCoordinator.presentedSheet 用作主入口路由
- `SheetType.room` / `SheetType.wardrobe` 枚举 case
- RootView .ready 分支的 fullScreenCover 主入口路由

**Git 历史**：完整保留。Story 2.3 status 不改（仍标 done），但本 ADR 内声明 partial revert，sprint-status.yaml 不动。

### 4.2 对下游 Story 的影响

| Story | 修订要点 |
|---|---|
| **12.7**（创建/加入/退出 use case + 主界面入口完善） | 入口从 Sheet 模式改为 Home Tab 互斥模式：从「主界面进入房间按钮 → 选项弹层」改为「Home idle 态 TeamIdleCard 创建/加入按钮 + Friends Tab 加入按钮」；CreateRoomUseCase 拿 roomId 后写 `AppState.currentRoomId`（联动 ADR-0010）；HomeContainerView 自动切到 RoomView |
| **21.1**（首页宝箱组件） | 不受导航影响；ChestCardView 仍嵌入 HomeView idle 态的"宝箱位"槽位 |
| **24.1**（仓库页骨架） | 从 Sheet 模式改 Wardrobe Tab；从 InventoryView 重命名 WardrobeView；Story 24.4「主界面入口完善」整条作废（Tab 直接路由不需要"主界面入口"） |
| **27.1**（穿戴入口） | 不受导航影响；激活 WardrobeView 内的装备/卸下按钮 |
| **30.3**（装扮渲染） | 不变 |
| **33.1**（合成页） | 从主界面 Sheet 改 Wardrobe Tab 内 push 或 sub-sheet（建议 push）；Story 33.6「主界面入口完善」整条作废 |
| **35.x**（分享链接） | 链接解析后调 JoinRoomUseCase 写 `AppState.currentRoomId` → HomeContainerView 自动切到 RoomView；Story 35.2「房间页内分享按钮」位置不变（仍在 RoomView 内） |

### 4.3 对 ui_design 契合度

| ui_design 元素 | 本 ADR 是否对齐 |
|---|---|
| 4 Tab IA | ✅ 完全对齐 |
| Home Tab 互斥（HomeScreen ↔ RoomScreen） | ✅ 完全对齐 |
| Tab 仍叫"家"且不变图标 | ✅ HomeContainerView 实装 |
| RoomScreen「离开房间」按钮回到 HomeScreen | ✅ leaveRoom() 清空 currentRoomId |
| JoinRoomModal 底部 sheet | ✅ §3.3 白名单 |
| 邀请好友自动创建房间流程 | ✅ FriendsView 调 createTeam() 写 AppState |

### 4.4 对 ADR-0002 §3.3（iPhone 工程目录方案）的影响

无影响。ADR-0002 §3.3 仅锁定目录结构（`iphone/PetApp/{App,Core,Shared,Features,Resources,Tests}/`），不锁定导航模型。本 ADR 在 ADR-0002 §3.3 选定的目录树内落地。

---

## 5. Post-Decision TODO

- [ ] **Story 37.3**：按 §3.5 改造 RootView；新建 MainTabView + HomeContainerView；删除 HomeView 旧 3 CTA 按钮 + AppCoordinator 旧 sheet 路由
- [ ] **Story 12.7 改写**：入口逻辑改 Home Tab 互斥（在 Sprint Change Proposal v2 提案内详述）
- [ ] **Story 24.1 改写 + 24.4 删除**：见 Sprint Change Proposal v2
- [ ] **Story 33.1 / 33.6 改写**：见 Sprint Change Proposal v2
- [ ] **Story 35.x 改写**：见 Sprint Change Proposal v2
- [ ] **Story 21.x / 27.x / 30.x**：不变
- [ ] **CLAUDE.md 同步**：本 ADR commit 时更新 CLAUDE.md 内 iPhone 端导航相关描述（如有）
- [ ] **iphone/README.md 同步**：在「测试依赖」段后追加「导航架构」段引用本 ADR

---

## 6. 验收（本 ADR 改 Accepted 的标准）

- [ ] 用户终审通过 Sprint Change Proposal v2
- [ ] Story 37.3 落地后跑 `bash iphone/scripts/build.sh --test` 通过
- [ ] codex 对 Sprint Change Proposal v2 verdict ≥ Accept with revisions
