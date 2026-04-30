# Sprint Change Proposal — iPhone UI Scaffold（壳先行）

- **日期**：2026-04-29
- **作者**：Bob (Scrum Master) via `bmad-correct-course`
- **触发**：sprint 中段战略调整 —— epic-5 done 后用户决定先把 5 屏 1 弹窗的视觉壳一次性铺完，再回填业务逻辑
- **依据**：`iphone/ui_design/README.md`（高保真原型 + Design Tokens + 状态机 + 数据模型）
- **范围分类**：**Moderate**（涉及 epics.md 增补 + PRD 局部澄清 + 7 条下游 story acceptance 前向声明 + sprint-status.yaml 更新；不涉及架构 / 技术栈推翻）

---

## 1. Issue Summary

### 1.1 问题陈述

iPhone 端 UI 设计原型（`iphone/ui_design/`，5 屏 + 1 弹窗 + 完整 Design Tokens）已完成且为高保真级别，但当前 sprint 计划中，UI 实装是**散落**在节点 4-11 的对应 iPhone Epic（Epic 12 房间页 / Epic 21 宝箱 / Epic 24 仓库 / Epic 27 穿戴 / Epic 33 合成 等）首条 story 内的"页面骨架"项 —— 每条骨架 story 都和该 epic 的业务逻辑（WS / API 调用 / ViewModel 状态机）耦合实装。

用户希望调整开发节奏：**先把 5 屏 1 弹窗的视觉壳一次性铺完（mock 数据驱动、不调任何 API、不引入业务逻辑），后续 epic 再在已交付的壳上接入业务逻辑**。

### 1.2 问题分类

**Strategic pivot（开发节奏调整）** —— 非需求新增、非技术失败、非误解原始需求；MVP 范围 / 节点顺序 / 技术栈均不变，仅是 UI 工作的**集中度**和**实装时机**调整。

### 1.3 证据

- `iphone/ui_design/source/screens/{home,room,wardrobe,friends,profile}.jsx` + `components/{primitives,cat-placeholder,tab-bar}.jsx` 高保真原型可读
- `iphone/ui_design/README.md` 含完整 Design Tokens（colors / spacing / radius / shadow / typography）+ 状态机 + 数据模型 + 动画规格 + 三主题 + 资产替换清单
- 当前 sprint 状态：epic-1 / epic-2 / epic-4 / epic-5 全部 done；epic-3 / epic-6 / epic-7+ backlog —— epic-5 ↔ epic-6 之间是天然插入窗口
- iphone/PetApp/ 工程骨架已就绪（App / Core / Features / Resources / Shared 五段；已含 Auth / DevTools / Home / Launching 4 个 feature 模块）

---

## 2. Impact Analysis

### 2.1 Epic 影响

| Epic | 影响 | 处理 |
|---|---|---|
| **Epic 1-5（已 done）** | 无影响 | 保持原状 |
| **Epic 6 节点 2 demo 验收** | 时间表延后（等 Epic 37 done 后启动）；demo 验收时 HomeScreen 已是高保真版，验收更直观 | 不改 acceptance；description 备注"前置 Epic 37" |
| **新增 Epic 37（本提案）** | sprint 中段插入，10 条 story（Theme + primitives + TabBar / 状态机 + 5 屏 + Modal + Snapshot 兜底） | 新建（详见 §4） |
| **Epic 12 iOS 房间页 + WS** | Story 12.1「房间页骨架」与 Epic 37 Story 37.5「RoomScreen 壳」重叠 | Story 12.1 acceptance 重写为「在 37.5 已交付壳上接入 RoomViewModel + WSState + Sheet 模式切换」 |
| **Epic 21 iOS 首页宝箱** | Story 21.1「首页宝箱组件」假设 HomeScreen 已存在 | Story 21.1 acceptance 加 Given「Story 37.4 HomeScreen 壳已交付」前置 |
| **Epic 24 iOS 仓库页** | Story 24.1「仓库页骨架」与 Epic 37 Story 37.6「WardrobeScreen 壳」重叠 | Story 24.1 acceptance 重写为「把 37.6 mock Inventory 替换为 GET /cosmetics/inventory 真实数据」 |
| **Epic 27 iOS 穿戴交互** | Story 27.1「激活穿戴按钮」假设按钮已存在 | Story 27.1 acceptance 加 Given「Story 37.6 已含装备/卸下按钮（本地切换）」前置 |
| **Epic 30 iOS 装扮渲染** | WardrobeScreen 道具图标占位（emoji + SF Symbol）由 SpriteRenderer + render_config 替换 | 不改 Epic 30 范围；description 备注"替换 37.6 占位资源" |
| **Epic 18 iOS 表情面板** | `ui_design/` 无表情面板设计稿，不在 Epic 37 范围 | 不受影响（Epic 18 自建） |
| **Epic 33 iOS 合成页** | `ui_design/` 无合成屏设计稿，不在 Epic 37 范围 | 不受影响（Epic 33 自建） |
| **Epic 36 节点 12 demo / MVP 收官** | 不影响 | 不变 |

### 2.2 Artifact 冲突

| Artifact | 冲突 | 修订 |
|---|---|---|
| **PRD** | §4 必做范围**未冲突**（Scaffold 不引入新功能）；但 4 Tab 信息架构（Home / Wardrobe / Friends / Profile）从未明文写出 → 隐含但不显式 | §4 加澄清段「4 Tab 信息架构」；§9 关联文档加 `iphone/ui_design/README.md` |
| **Architecture（ADR-0002）** | 无冲突 —— `iphone/PetApp/Features/{Home,Room,Wardrobe,Friends,Profile}/` 目录方案与 Scaffold 完全兼容 | 无需改 |
| **iOS 客户端工程结构设计文档** | 无冲突 | 无需改 |
| **V1 接口设计 / 数据库设计 / 时序图** | 不涉及 UI Scaffold | 无需改 |
| **CLAUDE.md** | 不涉及 | 无需改 |
| **MEMORY.md** | 不涉及 | 无需改 |
| **sprint-status.yaml** | 需新增 epic-37 + 10 条 story 占位 | 详见 §4 提案 ⑤ |

### 2.3 技术影响

- **代码体量**：新增约 12-18 个 Swift 文件（`iphone/PetApp/Core/DesignSystem/Theme.swift` + 6 个 primitives + 5 个 Feature/*View.swift + 1 个 Modal）+ 同等数量测试文件
- **构建影响**：无（沿用 `bash iphone/scripts/build.sh --test`）
- **CI 影响**：可能新增 snapshot baseline 文件（首次建立后 commit）
- **运行时影响**：无（Scaffold 不调 server，节点 1 demo 验收的 ping 调用仍由现有 Home / Launching 模块承担）

---

## 3. Recommended Approach

### 3.1 选定路径

**Direct Adjustment（Option 1）+ epic-37 追加末尾**：
1. 新增 Epic 37「iPhone UI Scaffold（壳先行）」追加在 `epics.md` Epic 36 之后
2. `sprint-status.yaml` 的 epic-37 yaml block **物理顺序**插在 epic-5-retrospective 后、epic-6 前（让 dashboard 读 yaml 顺序时仍反映 sprint 中段执行）
3. Epic 37 description 强硬声明"执行顺序：epic-5 done 后立即开始，epic-6 节点 2 demo 验收等本 epic 全部 done 才启动"
4. 5 条下游 iPhone Epic（12 / 21 / 24 / 27 / 30）的相关 story acceptance 加前向声明（Given Story 37.X 已交付）
5. PRD §4 / §9 局部澄清

### 3.2 选定理由

- **MVP 范围 0 改动**：Scaffold 不引入新功能，仅是 UI 工作集中度调整；§4 必做表 / §6 节点顺序均不变
- **节点顺序无破坏**：Scaffold story 不依赖任何 server 接口、不调 APIClient、不破坏"节点顺序不可跳"原则；它是**跨节点的 UI 基础设施 epic**，类似 Epic 1（server 脚手架）+ Epic 2（iOS 脚手架）那种"一次性奠基"性质
- **改动面 vs 长期收益对比**：
  - 改动面：epics.md +约 250 行 / prd.md +约 5 行 / sprint-status.yaml +12 行 / 5 处下游 story acceptance 微调
  - 长期收益：5 屏完整视觉一次到位，下游 5 个 iPhone Epic 的"骨架 story"工作量减少 60-70%，每条 story 集中在业务逻辑（ViewModel / API / WS / 状态同步）
- **风险低**：Scaffold 不依赖 server，可独立开发 / 测试 / demo；与已 done 的 Epic 1-5 零耦合
- **已拒方案对照**：
  - 「epic-5.5 小数编号」被用户拒绝（编号不规范）
  - 「重编号 6-36 → 7-37」工作量大且会破坏 git log 内可能的 epic 引用（虽然实测当前 git log 内并无下游 epic 引用，但仍是潜在 long-term tech debt）
  - 「Quick Spec / Quick Dev 一把梭」破坏现有 BMad story 纪律（codex review / lesson 归档 / sprint-status 追踪），且 ui_design 已是接近 spec 级，重新写 quick spec 是重复劳动

### 3.3 Effort & Risk 估算

| 项 | 估算 |
|---|---|
| **Effort** | Medium（10 条 story × 半屏到一屏 SwiftUI） |
| **Risk** | Low（壳不依赖 server / 不调 API / 数据全 mock；与已 done 模块零耦合） |
| **Timeline 影响** | epic-6（节点 2 demo）延后约 2-3 个 story cycle；MVP 总时间表延长但前期视觉就绪，对 demo 反而正向 |

---

## 4. Detailed Change Proposals

### 提案 ① · 新增 Epic 37（追加在 `epics.md` 末尾，Epic 36 之后）

```markdown
## Epic 37: iPhone UI Scaffold（壳先行）

依据 `iphone/ui_design/README.md` 高保真原型，把 5 屏 1 弹窗的视觉壳一次性在 `iphone/PetApp/Features/` 下实装为可视的"壳"。所有数据 mock，所有交互仅在本地 `@State` / `@Observable AppState` 内打转，**禁止任何 APIClient / WebSocket / Repository / UseCase 调用**。逻辑回填留给下游 Epic 12（房间）/ 21（宝箱）/ 24（仓库）/ 27（穿戴）/ 30（装扮渲染）。

> **执行顺序（关键）**：本 Epic 编号 37 但**实际是 sprint 中段插入**——epic-5 done 后立即开始，epic-6（节点 2 demo 验收）等本 Epic 全部 done 才启动。`sprint-status.yaml` 内本 Epic 的 yaml block 物理位置在 `epic-5-retrospective` 后、`epic-6` 前，让 dashboard 工具读 yaml 顺序时反映正确执行序。
>
> **核心约束**（每条 story 共性 AC）：
> - 数据：完全 mock（`@State` 写死或 `PreviewProvider` 注入）
> - API：禁止 import APIClient / Repository / UseCase（本 Epic 验收脚本会 grep 检查）
> - 视觉：像素级匹配 `iphone/ui_design/README.md` §Design Tokens
> - 主题：用 `@Environment(\.theme)` 取 token
> - 测试：每个 View 至少一个 SwiftUI Preview；关键交互 snapshot 测试
> - 通过 `bash iphone/scripts/build.sh --test`
>
> **接缝设计**（避免下游 Epic 重写 View）：每屏暴露一个 `<ScreenName>State` 结构体作为 view 的唯一 input；mock 时 hard-code，未来 ViewModel 注入即对接，View 内部代码零改动。

### Story 37.1: Theme & Design Tokens（candy 优先 + 三主题抽象）

As an iOS 开发,
I want 一套 Theme 系统能注入 colors / spacing / radius / shadow / typography 五类 token,
So that 所有 Feature View 通过 `@Environment(\.theme)` 取色取间距，三主题切换零代码改动.

**Acceptance Criteria:**

**Given** Epic 2 SwiftUI 入口 + iphone/PetApp/Core/DesignSystem/ 目录已就绪
**When** 完成本 story
**Then** 在 `iphone/PetApp/Core/DesignSystem/` 落地：
- `Theme.swift`: enum `ThemeName { candy, matcha, sky, dark }` + struct `Theme { colors, spacing, radius, shadow, typography }`
- `ThemeColors.swift` / `ThemeSpacing.swift` / `ThemeRadius.swift` / `ThemeShadow.swift` / `ThemeTypography.swift`：分别覆盖 README §Design Tokens 五类
- `Environment+Theme.swift`：`@Environment(\.theme) var theme` 注入入口
- candy 主题完整实装（page-bg / accent / accent-soft / accent-deep / surface / surface-2 / ink / ink-soft / ink-mute / success / warn / coin / border 全套）
- matcha / sky / dark 主题：enum case 抽象齐全 + token 表 stub（每个字段 TODO 注释，给值用 candy 同字段 placeholder）
- RootView 用 `@State var currentTheme: ThemeName = .candy` 注入到环境中
**And** **单元测试覆盖**（≥3 case）:
- happy: 切换 currentTheme 到 candy → @Environment(\.theme).colors.accent == #ff8fa3
- happy: 切换 currentTheme 到 dark → @Environment(\.theme).colors.pageBg == #2a1c22
- edge: 未注入主题（默认值）→ 返回 candy

### Story 37.2: 共享 primitives（Card / PrimaryButton / Avatar / FadeIn / RarityTag / Icons）

As an iOS 开发,
I want 一组从 ui_design/components/primitives.jsx 翻译来的 SwiftUI 共享 primitives,
So that 5 个 Feature View 复用统一的卡片 / 按钮 / 头像 / 渐入动效 / 稀有度色条 / 图标.

**Acceptance Criteria:**

**Given** Story 37.1 Theme 已就绪
**When** 完成本 story
**Then** 在 `iphone/PetApp/Core/DesignSystem/Primitives/` 落地 6 个文件：
- `Card.swift`: 圆角 16-24 + theme.colors.surface 背景 + theme.shadow.sm 阴影；接受 `@ViewBuilder content`
- `PrimaryButton.swift`: 圆药丸（高度一半圆角）+ theme.colors.accent 底色 + 立体硬阴影 `0 4px 0 accent-deep` + 按下 `translateY(2)` 动效（按 README §Shadows 描述）；支持 disabled 态
- `Avatar.swift`: 圆形 + 光环描边（borderColor: theme.accent，width 2）+ image / SF Symbol 兼容；可附加在线小绿点
- `FadeIn.swift`: ViewModifier，0.28s ease 渐入 + 上移 8pt（按 README §Interactions 中 Tab 切换动画描述）
- `RarityTag.swift`: 接受 `Rarity { N, R, SR, SSR }` enum，按 README §Wardrobe 配色 (N=灰 #b0b0b0 / R=蓝 #7db3e8 / SR=紫 #c58ae8 / SSR=金红渐变) 渲染色条
- `Icons.swift`: 静态映射 ui_design/components/primitives.jsx 内 Icons 对象到 SF Symbols（如 home → "house.fill"，wardrobe → "tshirt.fill"，friends → "person.2.fill"，profile → "person.crop.circle.fill"，等）；不能找到对应 SF Symbol 时退回 `Image(systemName: "questionmark.circle")` + log warning
**And** 每个 primitive 有 `#Preview` 块，candy 主题 + dark 主题各一个预览
**And** **单元测试覆盖**（≥3 case，主测 RarityTag 渲染逻辑）:
- happy: RarityTag(.SSR) → 渐变 LinearGradient 含 #ffd166 + #ef476f
- happy: PrimaryButton disabled 态 → background opacity < 1
- happy: Icons["home"] → "house.fill" SF Symbol

### Story 37.3: TabBar 4 Tab 路由 + Home 互斥状态机

As an iPhone 用户,
I want 底部 4 Tab（家 / 仓库 / 好友 / 我的）+ 家 Tab 内 idle ⟷ inRoom 互斥切换,
So that App 信息架构与 ui_design 一致，未加入队伍时显示养成界面，已加入队伍时家 Tab 自动变成房间界面.

**Acceptance Criteria:**

**Given** Story 37.1 Theme + Story 37.2 primitives 已就绪
**When** 完成本 story
**Then** 改造 `iphone/PetApp/App/RootView.swift`（或新建 `iphone/PetApp/App/MainTabView.swift`）:
- 用 `TabView` 含 4 Tab：家 / 仓库 / 好友 / 我的（按 ui_design/components/tab-bar.jsx 顺序）
- 底部 TabBar 高 72pt，距底 14pt，距左右 12pt（按 README §iOS 设备规格），浮动样式 + theme.shadow.md 阴影 + Card 背景
- 家 Tab 内是 `HomeContainerView`，根据全局 `@Observable AppState.hasTeam` 在 `HomeView`（idle）↔ `RoomView`（inRoom）之间切换，**Tab 仍叫"家"且不变图标**
- 全局 `AppState`（`@Observable` class）含：`hasTeam: Bool`, `roomCode: String?`, `currentTab: Tab` enum
- 创建 / 加入 / 离开队伍的方法（同步 mock 实现）：`createTeam()` 生成 `{3字母}-{2数字}` 格式代码 + 切 hasTeam = true；`joinTeam(code)` 校验格式 + 切 hasTeam = true + 设置 roomCode；`leaveRoom()` 切 hasTeam = false + 清空 roomCode
**And** 切 Tab 时内容用 `FadeIn` 动效（0.28s）
**And** **单元测试覆盖**（≥4 case）:
- happy: 初始 hasTeam = false → HomeContainerView 显示 HomeView
- happy: createTeam() → hasTeam = true + roomCode 符合格式 + HomeContainerView 显示 RoomView
- happy: leaveRoom() → hasTeam = false + roomCode = nil
- edge: joinTeam("invalid") → hasTeam 不变 + 抛 InvalidRoomCode 错误
**And** **UI 测试覆盖**（XCUITest）: 启动 → 验证 4 Tab 可定位（accessibility identifier "tab.home" / "tab.wardrobe" / "tab.friends" / "tab.profile"）+ 切到 Wardrobe Tab 验证 WardrobeView 出现

### Story 37.4: HomeScreen 壳

As an iPhone 用户,
I want 家 Tab idle 态显示与小猫互动 + 状态条 + 创建/加入队伍 CTA 的高保真界面,
So that 我能与小猫互动并发起组队（视觉先实装，逻辑后续 Epic 接入）.

**Acceptance Criteria:**

**Given** Story 37.3 TabBar + 互斥状态机已就绪
**When** 完成本 story
**Then** 在 `iphone/PetApp/Features/Home/Views/HomeView.swift` 实装（Story 2.2 的占位版完整重写）:
- 顶部 StatusBar：天气问候 + 步数计（mock "今天阴天 🌥️" + "1234 步"）
- 小猫舞台 Card：3D 模型占位（`Image(systemName: "cat.fill")` + 椭圆背景 + theme.colors.accent-soft 条纹 overlay 模拟原型 SVG）+ 等级名牌（"Lv.5 小橘"）+ 三条状态条（饱食 / 心情 / 活力，0-100 横向 progress bar）
- ActionRow：3 个互动按钮（喂食 🍥 / 抚摸 💕 / 玩耍 ⭐）；点击触发 emoji 从猫身上 floatUp（1.4s：0%→25% 渐显放大上移 -20pt，→100% 缩小消失到 -110pt）—— 按 README §Interactions 实装
- 底部 TeamIdleCard：theme.colors.accent 渐变背景 + Card 圆角 22 + 「创建队伍」「加入队伍」两个 PrimaryButton；创建 → AppState.createTeam()；加入 → 触发 JoinRoomModal（占位 sheet，Story 37.9 实装真正 modal）
- View 暴露 `HomeViewState { greeting: String, steps: Int, cat: MockCat, stats: (hunger, mood, energy) }` 接受外部注入
**And** **单元测试覆盖**（≥4 case）:
- happy: 注入 HomeViewState → View body 含 greeting / steps / 三状态条
- happy: 点击「创建队伍」→ AppState.hasTeam = true + roomCode 非空
- happy: 点击「喂食」→ 触发 emoji floatUp 动画（用 ViewInspector 或 snapshot 验证）
- edge: stats.hunger = 0 → 状态条渲染最低值（不报错）
**And** snapshot 测试: HomeView 在 candy 主题下完整渲染快照入库

### Story 37.5: RoomScreen 壳

As an iPhone 用户,
I want 已加入队伍时家 Tab 显示房间界面（房间代码卡 + 共享舞台 + 成员列表 + 离开按钮）,
So that 我能与队友共处虚拟空间（视觉先实装，WS 同步留给 Epic 12）.

**Acceptance Criteria:**

**Given** Story 37.3 互斥状态机 + Story 37.2 primitives 已就绪
**When** 完成本 story
**Then** 在 `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift` 实装（命名加 Scaffold 后缀避免与 Epic 12 的 RoomView 冲突；Epic 12 时改名/合并）:
- 顶部：返回按钮（leaveRoom）+ 标题"队伍房间 / {猫名}的小屋"
- 房间代码 Card：大字号（48pt 800 weight 等宽字体）+ 代码格式 `XXX-NN`（如 7K3-P2）+ 复制按钮；点击复制后按钮变绿 + 显示对勾，1.2s 后还原（按 README §Interactions）
- 共享舞台 Card：粉橙渐变背景（LinearGradient theme.accent → #ffb26b）+ 云朵装饰 SF Symbol（cloud.fill 半透明）+ 鱼/毛线球元素（fish / circle.fill 装饰）+ 4 只 MiniCat 在底部上下弹跳（bounce 2.2s ease-in-out infinite，错峰 0.2s 间隔）
- 成员列表：4 格水平 HStack；mock 3 个成员 + 1 个虚线占位"+ 等待好友加入"；队长格右上角"队长"小标签
- 底部"离开房间"PrimaryButton（次要按钮样式：theme.colors.surface-2 背景 + theme.colors.ink-soft 文字）
- View 暴露 `RoomScaffoldViewState { roomCode: String, hostCatName: String, members: [MockMember], userIsHost: Bool }`
**And** **单元测试覆盖**（≥4 case）:
- happy: 注入 4 成员 → View 渲染 4 格无占位
- happy: 注入 2 成员 → View 渲染 2 实 + 2 虚线占位
- happy: 点击复制按钮 → 1.2s 内按钮显示绿色对勾
- happy: 点击离开 → AppState.hasTeam = false
**And** snapshot 测试: RoomScaffoldView 在 candy 主题下完整渲染快照入库

### Story 37.6: WardrobeScreen 壳

As an iPhone 用户,
I want 仓库 Tab 显示 3D 试衣间式装扮浏览界面（预览区 + 分类 Tab + 3 列网格）,
So that 我能查看拥有的装扮和未解锁道具（视觉先实装，inventory 数据留给 Epic 24）.

**Acceptance Criteria:**

**Given** Story 37.3 TabBar + Story 37.2 primitives + RarityTag 已就绪
**When** 完成本 story
**Then** 在 `iphone/PetApp/Features/Wardrobe/Views/WardrobeView.swift` 实装:
- 顶部 Card：标题"{猫名} 的衣柜" + 收藏数（mock "36/53"）+ 钻石货币 mini 标签（mock "248"）
- 预览区 Card（关键 UX）：左侧 1/2 是带当前装扮的小猫预览（复用 HomeScreen 占位猫 + 当前装扮 emoji 叠加）；右侧 1/2 显示当前选中道具名 + RarityTag + 已拥有/未解锁标签 + 装备/卸下 PrimaryButton
- 分类 Tab：横向滚动 5 个：帽子 🎩 / 饰品 🎀 / 围巾 🧣 / 服装 👘 / 背景 🏞️
- 网格列表：3 列 LazyVGrid；每个道具一个方形卡片，含图标（emoji）+ 名字 + 稀有度色条（RarityTag）；未拥有的卡片半透明（0.4 opacity）+ 锁图标（lock.fill）；已装备的右上角有绿色对勾（checkmark.circle.fill）
- mock Inventory：每分类 6-8 件，含 N / R / SR / SSR 各几个，部分 unowned；维护一个本地 `equipped: Equipment` @State 用于展示装备态
- 装备 / 卸下按钮：仅本地 @State 切换 equipped 字段；不调任何接口
- View 暴露 `WardrobeViewState { catName: String, collectedCount: Int, totalCount: Int, gemCount: Int, items: [MockItem], equipped: Equipment }`
**And** **单元测试覆盖**（≥4 case）:
- happy: 切到「饰品」Tab → grid 只显 category=bow 道具
- happy: 点选 owned 道具 → 装备按钮可点击 + 状态变化
- happy: 点选 unowned 道具 → 装备按钮 disabled
- edge: items 为空数组 → grid 显空态 placeholder
**And** snapshot 测试: WardrobeView 在 candy 主题下完整渲染快照入库

### Story 37.7: FriendsScreen 壳

As an iPhone 用户,
I want 好友 Tab 显示在线人数 + 我的房间提示条 + Tab 在线/全部 + 好友列表（含三态操作按钮）,
So that 我能查看好友状态、邀请好友、加入好友房间（视觉先实装，邀请/加入逻辑留给后续 Epic）.

**Acceptance Criteria:**

**Given** Story 37.3 TabBar + Story 37.2 primitives 已就绪
**When** 完成本 story
**Then** 在 `iphone/PetApp/Features/Friends/Views/FriendsView.swift` 实装:
- 顶部 Card：在线人数统计（mock "5 位好友在线"）+ 添加好友按钮（圆形 + plus.circle.fill SF Symbol）
- 我的房间提示条 Card：仅当 AppState.hasTeam = true 时显示；含房间代码（可点击复制）+ "分享给好友"次要按钮（占位 toast，本 epic 不实装真分享）
- Tab：在线 / 全部 segmented control（在线时只显 online + inRoom 的；全部时显所有）
- 好友列表 ScrollView + LazyVStack：每行 FriendRow 含 Avatar（带在线小绿点 overlay）+ 名字 + "房间中"角标（仅 inRoom 状态有）+ 状态文字（"在房间 7K3-P2 中玩耍" / "在线" / "30 分钟前在线"）+ 右侧操作按钮（三态）
- 操作按钮逻辑（仅本地反馈，不调任何接口）：
  - 好友在房间中 → "加入"实心 PrimaryButton；点击 → 解析 statusText 中的 6 位代码 → AppState.joinTeam(code)
  - 好友在线但不在房间 → "邀请"描边按钮；点击 → 若当前用户没房间则 AppState.createTeam() 生成代码 → toast "已邀请 {名字} 加入房间 {code}"
  - 好友离线 → "离线"灰字（不可点）
- mock 8 个 Friend（含三态各 2-3 个）
- View 暴露 `FriendsViewState { onlineCount: Int, friends: [MockFriend], myRoomCode: String? }`
**And** **单元测试覆盖**（≥5 case）:
- happy: 切到「在线」Tab → 列表只显 online + inRoom
- happy: 点击 inRoom 好友的「加入」→ AppState.hasTeam = true
- happy: 点击 online 好友的「邀请」（用户没房间）→ AppState.createTeam() 触发 + toast 显示
- happy: 点击 online 好友的「邀请」（用户已有房间）→ 仅 toast 不重复创建
- happy: offline 好友按钮 disabled
**And** snapshot 测试: FriendsView 在 candy 主题下完整渲染快照入库

### Story 37.8: ProfileScreen 壳

As an iPhone 用户,
I want 我的 Tab 显示个人信息 / 收藏成就总览 / 设置入口,
So that 我能查看自己资料、最近收藏、进入设置（视觉先实装，真实数据留给后续 Epic）.

**Acceptance Criteria:**

**Given** Story 37.3 TabBar + Story 37.2 primitives 已就绪
**When** 完成本 story
**Then** 在 `iphone/PetApp/Features/Profile/Views/ProfileView.swift` 实装:
- 顶部渐变头图：theme.colors.accent → accent-soft 渐变背景 + 中间大 Avatar（带光环描边 + theme.colors.accent border 3pt）+ 用户名（22pt 800）+ 用户 ID 小字（mock "ID: U10086"）+ 称号 mini Tag（mock "见习铲屎官"）+ "加入于 2024年3月15日" 小药丸
- 统计卡：覆盖在头图底部 1/3（offset -40pt + Card 圆角 24）；4 列横向：收藏品（数值 + tshirt.fill icon）/ 好友（数值 + person.2.fill）/ 小猫等级（数值 + cat.fill）/ 成就（数值 + trophy.fill）
- 最近收藏：横向 ScrollView + 5 个 Card（mock 5 件最近开箱获得的道具，emoji + 名字 + RarityTag）
- 更多菜单：纵向列表 4 项，每项 HStack（icon + 标题 + chevron.right）
  - 成就徽章
  - 消息通知
  - 喜欢的道具
  - 设置
- View 暴露 `ProfileViewState { user: MockUser, stats: (collected, friends, catLevel, achievements), recentItems: [MockItem] }`
**And** **单元测试覆盖**（≥3 case）:
- happy: 注入 user → View 渲染 username / userId / title
- happy: 点击「设置」→ 触发 navigation push（Sheet 或 NavigationLink，本 epic 用空 SettingsPlaceholderView）
- edge: recentItems 为空 → 横向滚动区显 placeholder "还没有收藏"
**And** snapshot 测试: ProfileView 在 candy 主题下完整渲染快照入库

### Story 37.9: JoinRoomModal + 跨屏跳转

As an iPhone 用户,
I want 加入队伍时弹出输入房间代码的 modal（猫爪图标 + 自动大写 + 3 字符启用确定按钮）,
So that 我能在 HomeScreen / FriendsScreen 两处入口都用统一交互加入房间.

**Acceptance Criteria:**

**Given** Story 37.4 HomeScreen + Story 37.7 FriendsScreen 都触发 join 入口 + Story 37.3 状态机已就绪
**When** 完成本 story
**Then** 在 `iphone/PetApp/Shared/Modals/JoinRoomModal.swift` 实装:
- 底部居中弹出：背景遮罩 0.45 black opacity + 卡片从下方 20pt 上滑 0.3s（按 README §Interactions）
- 卡片 Card 圆角 26 + theme.colors.surface 背景
- 标题"加入队伍" + 关闭按钮（xmark.circle.fill 右上角）
- 说明文字"输入好友分享给你的 6 位房间代码"
- 大输入框：猫爪图标 SF Symbol（pawprint.fill）prefix + 等宽字体（system monospaced）+ 字距 3pt + 自动转大写（onChange 内 uppercased + filter `[A-Z0-9-]`）+ 限 8 字符（含连字符）
- 格式提示"3 个字母 - 2 位数字"灰字 small
- 取消 / 确定加入 两按钮：仅 ≥3 字符且匹配正则 `^[A-Z]{3}-[0-9]{2}$` 才启用确定
- 确定加入 → AppState.joinTeam(code) → modal dismiss → 切换为 RoomScaffoldView
- HomeScreen 加入按钮触发：HomeView 内 `@State var showJoinModal = false`，绑定 sheet
- FriendsScreen 加入按钮触发：解析 statusText 房间代码后**直接**调 AppState.joinTeam（不走 modal，对应 README §邀请好友流程"加入好友房间"）
**And** **单元测试覆盖**（≥5 case）:
- happy: 输入 "abc-12" → 自动转 "ABC-12" + 确定按钮启用
- happy: 输入 "ABC-12" 点确定 → AppState.hasTeam = true + roomCode == "ABC-12"
- edge: 输入 "AB-12"（少 1 字母）→ 确定按钮 disabled
- edge: 输入 "ABCD-12" → 限 8 字符，超出截断
- happy: 输入 "abc-1@" → 过滤非法字符 → 显示 "ABC-1"

### Story 37.10: Preview & Snapshot 测试兜底

As an iOS 开发,
I want 5 屏 + Modal 全部含 SwiftUI Preview（candy + dark 主题各一）+ 关键交互快照入库,
So that 后续 Epic 修改 View 时能快速发现视觉回归.

**Acceptance Criteria:**

**Given** Story 37.4 - 37.9 全部完成
**When** 完成本 story
**Then** 5 屏 + JoinRoomModal 各含至少 2 个 `#Preview`：candy 主题 + dark 主题
**And** 关键交互 snapshot 测试入库（用 Story 2.7 已搭好的 XCTest infra；如需 SnapshotTesting 库可在本 story 引入）:
- TabBar 切换 4 Tab 各一张
- HomeContainerView idle / inRoom 两态各一张
- JoinRoomModal 弹出态一张
- WardrobeView 选中 owned / unowned 各一张
- FriendsView 在线 / 全部 Tab 各一张
**And** snapshot baseline 文件 commit 入 git，路径 `iphone/PetAppTests/__Snapshots__/`
**And** **CI 集成**：`bash iphone/scripts/build.sh --test` 失败时 .xcresult 含 snapshot diff 报告
**And** **接缝校验脚本**：本 epic 验收前跑 `grep -rE "import.*(APIClient|UseCase|Repository)" iphone/PetApp/Features/{Home,Room,Wardrobe,Friends,Profile}` 必须输出空 —— 任何引用即视为本 epic 红线（非 Story 37.4-37.8 范围）
```

### 提案 ② · 5 条下游 iPhone Story 的 acceptance 加 "前向声明"

| Story | 当前位置 | 改写要点 |
|---|---|---|
| **Story 12.1 房间页 SwiftUI 骨架** | epics.md L2030-2052 | 头部 Given 加「Story 37.5 RoomScaffoldView 已交付（含房间代码卡 / MiniCat 弹跳 / 成员列表 4 格 / 离开按钮）」；范围改为「在 RoomScaffoldView 上叠加 RoomViewModel + WSState 状态显示 + Sheet 模式整合」；删除「成员列表 4 格占位」「房间号显示」「退出房间按钮」等已由 37.5 完成的 AC 项 |
| **Story 21.1 首页宝箱组件 SwiftUI** | epics.md（搜索定位） | 头部 Given 加「Story 37.4 HomeView 已交付（含 StatusBar / CatStage / ActionRow / TeamIdleCard）」；范围明确「在 HomeView 上叠加宝箱倒计时 Card」；不删除原 AC，仅声明前置 |
| **Story 24.1 仓库页 SwiftUI 骨架** | epics.md（搜索定位） | 头部 Given 加「Story 37.6 WardrobeView 已交付（含分类 Tab / 3 列网格 / 预览区 / mock Inventory）」；范围改为「把 WardrobeView 内的 mock Inventory `@State` 替换为 GET /cosmetics/inventory 真实数据 via LoadInventoryUseCase」 |
| **Story 27.1 激活穿戴按钮** | epics.md（搜索定位） | 头部 Given 加「Story 37.6 WardrobeView 已含装备/卸下按钮（本地 @State 切换）」；范围改为「把按钮回调从本地切换改为 POST /cosmetics/equip / unequip 真实事务调用 via EquipUseCase」 |
| **Story 30.3 升级 EquippedCosmeticView** | epics.md（搜索定位） | 描述加「替换 Story 37.6 WardrobeView 内 emoji + SF Symbol 占位资源 → SpriteRenderer + render_config 渲染」 |

> **Story 18.1 表情面板 / 33.1 合成页** 不受影响（ui_design 无对应设计稿）；本提案不修改它们。

### 提案 ③ · PRD 增补两条澄清

**位置**：`prd.md` §4 必做范围表后追加（约 L46 前），§9 关联文档（约 L120）追加一行。

```markdown
**4 Tab 信息架构（澄清，依据 iphone/ui_design/README.md）**：

App 底部 Tab Bar 共 4 个入口：
1. **家（Home）** —— idle ⟷ inRoom 两态互斥（已加入队伍时 RoomScreen 完全替换 HomeScreen，Tab 仍叫"家"且不变图标）
2. **仓库（Wardrobe）** —— 装扮道具收藏 + 试衣间式预览
3. **好友（Friends）** —— 仅做"在线状态展示 + 邀请/加入按钮"基础形态；§4「暂不做」内的"复杂好友系统"（关注 / 拉黑 / 聊天 / 二级菜单）继续不做
4. **我的（Profile）** —— 仅做信息展示 + 设置入口；不做账户体系扩展（节点 2 的微信绑定 UI 仍按"暂不做"）
```

§9 加：

```markdown
- `iphone/ui_design/README.md`（iPhone 端高保真原型与 Design Tokens；Epic 37 实装依据）
```

### 提案 ④ · sprint-status.yaml 增补

在 `epic-5-retrospective: optional` 行后、`epic-6: backlog` 行前**物理插入**：

```yaml
  epic-37: backlog
  37-1-theme-design-tokens: backlog
  37-2-shared-primitives: backlog
  37-3-tabbar-home-互斥状态机: backlog
  37-4-homescreen-壳: backlog
  37-5-roomscreen-壳: backlog
  37-6-wardrobescreen-壳: backlog
  37-7-friendsscreen-壳: backlog
  37-8-profilescreen-壳: backlog
  37-9-joinroommodal-跨屏跳转: backlog
  37-10-preview-snapshot-测试兜底: backlog
  epic-37-retrospective: optional
```

> 注：yaml dict 解析不依赖文件文本顺序，但本仓 BMad dashboard / SM 工具按文本顺序读，物理插在 `epic-5-retrospective` 后即反映"sprint 中段执行"语义。

### 提案 ⑤ · Epic 6 description 加前置声明

`epics.md` Epic 6（节点 2 demo 验收）章节首行后追加一句：

```markdown
> **前置依赖**：Epic 37（iPhone UI Scaffold）需先 done。本 epic 启动时 HomeScreen 已是高保真版（Story 37.4），ping + LoadHome 数据展示在该高保真壳上验收。
```

---

## 5. Implementation Handoff

### 5.1 Scope 分类

**Moderate** —— 涉及 sprint backlog 重排（新增 1 epic + 10 story + 5 处下游 acceptance 微调）+ PRD 局部澄清，但不涉及架构 / 技术栈 / MVP 范围推翻。

### 5.2 Handoff Recipients

| 角色 | 责任 |
|---|---|
| **Scrum Master（Bob）** | 提案落实到 epics.md / prd.md / sprint-status.yaml；运行 `bmad-create-story` 生成 Story 37.1 文件；后续 epic-loop 推进 |
| **Developer Agent（Amelia）** | 按 epic-loop 循环实装 10 条 story；每条 story 跑 codex review + lesson 归档 |
| **Architect（Winston）** | 不主动介入；若 Story 37.10 接缝校验脚本发现"State 结构体不足以支撑 ViewModel 注入"等接缝问题，由架构师 review |

### 5.3 Success Criteria

- 10 条 Story 全部 done（含 codex review approved）
- `bash iphone/scripts/build.sh --test` 通过（含 snapshot baseline）
- `grep -rE "import.*(APIClient|UseCase|Repository)" iphone/PetApp/Features/{Home,Room,Wardrobe,Friends,Profile}` 输出空（接缝红线）
- 5 屏 + Modal 在模拟器上视觉与 `iphone/ui_design/source/index.html` 一致（人眼对比 + snapshot baseline 双校验）
- AppState 三态切换可测可验（idle ⟷ inRoom；TabBar 4 Tab 切换）
- epic-37-retrospective optional 状态保持（Scaffold epic 不强制 retro）

### 5.4 Next Steps（用户终审通过后）

1. SM 把提案 ① ② ③ ④ ⑤ 落到对应 artifact（epics.md / prd.md / sprint-status.yaml）
2. SM 运行 `bmad-sprint-planning` 验证 sprint-status.yaml 一致性（可选，结构没变量化大）
3. SM 运行 `bmad-create-story 37.1` 生成首条 story 文件
4. Dev 启动 epic-loop 循环（用户已有自定义 `epic-loop` skill）
5. epic-37 全部 done 后启动 epic-6 节点 2 demo

---

## 6. Risk Register（高风险项快速登记）

| 风险 | 等级 | 缓解 |
|---|---|---|
| Scaffold 与未来 ViewModel 接缝处理不当导致下游 Epic 重写 View | M | Story 37.10 强制接缝红线检查 + Story 37.4-37.8 强制每屏暴露 `<ScreenName>State` 结构体作为唯一 input |
| 三主题 + dark 模式同期上线工作量爆炸 | M | Story 37.1 范围限定 candy 完整 + 其它三主题 token stub；matcha / sky / dark 完整实装留给"主题完善 mini-epic"未来追加 |
| 像素级还原 SVG 条纹猫工作量不可控 | L | 不强求 SVG；用 SF Symbol cat.fill + 椭圆 + 简单条纹 overlay；ui_design/README 明确条纹猫是后期替换 placeholder |
| 房间代码格式（3 字母-2 数字）与数据库设计 rooms.code 字段格式不一致 | M | Story 37.5 / 37.9 仅本地 mock 生成 `XXX-NN`；真实格式以 `docs/宠物互动App_数据库设计.md` 为准；Epic 11（房间事务）实装时若不一致，由那时单独 reconcile |
| 已有 `iphone/PetApp/Features/Home/` 是 Story 2.2 占位版（猫展示位 / 步数位等占位）；Story 37.4 重写会覆盖 | L | git 历史完整保留旧版本；Story 37.4 起 commit message 明示"覆盖 Story 2.2 占位 → ui_design 高保真"；旧 view 删除前确认 ping / LoadHome 调用迁移到新 view 的 onAppear |
| Snapshot baseline 跨平台跨 Xcode 版本不稳定 | M | Story 37.10 baseline 在 Xcode 26.4.1 + iPhone 17 simulator 上生成；未来 contributor 升级 Xcode 后 baseline 重生（与 ADR-0002 §3.4 destination fallback 协同） |

---

## 7. 用户终审记录（Step 5 时填）

- [ ] 用户终审通过日期：____
- [ ] 落盘 commit hash：____
- [ ] 落盘 commit message：____
- [ ] 第一条 story（37.1）启动日期：____
