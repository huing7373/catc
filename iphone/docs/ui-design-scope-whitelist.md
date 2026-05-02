# iPhone App ui_design Scope Whitelist (Epic 37 落地)

> **本文档作用**：「Epic 37 显式选择**不做**的 ui_design 元素清单」，每条带「位置（ui_design 文件 + 行号）/ 不做理由（cite PRD / ADR / epic 边界）/ 何时做（永不实装 / 后续 epic / 后续 spike / 节点 N 起）」三段说明。

> **本文档不作用**：
> 1. **不是** PRD（PRD 在 `_bmad-output/planning-artifacts/prd.md`，本表只 cite，不修改）；
> 2. **不是** 未来计划（未来 epic 由 BMAD planning workflow 产出；本表只声明粒度到 epic / spike / 节点 N 的"何时做"）；
> 3. **不是** ui_design 自身的修改（`iphone/ui_design/source/**` zero edit；本表只**记录**已选择不做哪些 ui_design 元素，**不**修改 ui_design 本身）。

## 使用场景

1. 后续 sprint 启动时，dev / PM / SM 先看本表 → 决定要不要把某条踢出白名单 → 才能开新 story（流程见尾部「如何把某条踢出白名单」）。
2. retrospective 阶段佐证 Epic 37 已 settled scope（防止误判"不完整"）。
3. 新加入 dev / PM / reviewer 提"为什么 X 没做"时优先指向本表（而非每次 sprint 重复争论）。

## 关联文档

- `_bmad-output/planning-artifacts/prd.md` §4「MVP 范围 / 暂不做」
- `_bmad-output/planning-artifacts/epics.md` Epic 37（line 4555+）/ Story 37.14（line 4889+）
- `_bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md`（ADR-0009 4 Tab 信息架构）
- `_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md`（ADR-0010 AppState single source of truth）
- `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md`（ADR-0002 iPhone 工程结构 + ios/ 不动钦定）
- `iphone/docs/visual-review-checklist.md`（Story 37.13 落地的 sibling docs；本表与之 orthogonal 互补不重叠）

## 更新协议

本表**只**在以下两种情况更新：
1. 一条目从"不做"变成"在做"（被未来 epic 踢出白名单）→ 本条目从表体删除（git history 留痕）+ PR description 贴新 story 链接作依据。
2. 新发现的 ui_design 元素需要登记为"不做"→ 本表追加新条目，理由必须 cite PRD / ADR / epic。

## 目录

1. [tweaks-panel.jsx（开发时主题切换调试面板）](#1-tweaks-paneljsx开发时主题切换调试面板)
2. [三主题切换 UI（用户可见的主题选择器）](#2-三主题切换-ui用户可见的主题选择器)
3. [wechat_binding 真实 OAuth 流程](#3-wechat_binding-真实-oauth-流程)
4. [Profile 顶部 bell（消息通知）真实通知中心](#4-profile-顶部-bell消息通知真实通知中心)
5. [Profile「成就徽章」页 / 「喜欢的道具」页详情](#5-profile成就徽章页--喜欢的道具页详情)
6. [HomeView 互动后状态条变化（喂食后饱食 +5 等）](#6-homeview-互动后状态条变化喂食后饱食-5-等)
7. [WardrobeView 钻石货币真实数值更新](#7-wardrobeview-钻石货币真实数值更新)
8. [小猫 3D 模型（USDZ / RealityKit）](#8-小猫-3d-模型usdz--realitykit)
9. [装扮道具 emoji 占位（仅本期 Scaffold 用）](#9-装扮道具-emoji-占位仅本期-scaffold-用)
10. [K3M9P2 等美化别名（roomId 美化展示）](#10-k3m9p2-等美化别名roomid-美化展示)

## 不做项清单（10 条）

### 1. tweaks-panel.jsx（开发时主题切换调试面板）

- **位置（ui_design）**：`iphone/ui_design/source/tweaks-panel.jsx`（整个文件，419 行）
- **理由**：tweaks-panel 是 ui_design 内部 dev tool（开发时用浏览器实时切主题 token 调参），**不**属于 App 端用户可见 UI。Epic 37 Theme stub 已落地 candy / dark / mono 三套 token，但 dev tool 与 App 用户面 orthogonal。[Source: _bmad-output/implementation-artifacts/37-5-theme-design-tokens.md] [Source: _bmad-output/planning-artifacts/epics.md#Story 37.5]
- **何时做**：永不实装。App 端不需要 dev tool；ui_design 内的 tweaks-panel 仅服务于 ui_design 本身的开发流。

### 2. 三主题切换 UI（用户可见的主题选择器）

- **位置（ui_design）**：`iphone/ui_design/source/tweaks-panel.jsx`（dev tool 内的主题切换 UI，本期不映射到 App）；`iphone/ui_design/source/screens/profile.jsx`（视觉壳**不**含主题切换按钮）
- **理由**：Epic 37 Story 37.5 落地的是 **Theme stub**（candy / dark / mono 三套 token，编译期可切，但 ProfileView 视觉壳**不**含用户可见的主题切换控件）。用户级主题切换 UI 不在 Epic 37 AC 内。[Source: _bmad-output/planning-artifacts/epics.md#Story 37.5] [Source: _bmad-output/planning-artifacts/epics.md#Story 37.11]
- **何时做**：后续 mini-epic（视产品优先级；非节点 1-12 关键路径）。

### 3. wechat_binding 真实 OAuth 流程

- **位置（ui_design）**：`iphone/ui_design/source/screens/profile.jsx` line 75-118（绑定微信卡片）+ line 194-239（绑定微信浮窗）+ `iphone/ui_design/wechat_binding.md`（完整 OAuth / SDK 集成流程文档：唤起微信授权、回调换 code、server 换 unionId / openId 的端到端时序与 SDK 选型）
- **理由**：PRD §4「暂不做」line 61 钦定「微信绑定（结构预留，节点 2 不实现 UI）」；PRD §4 line 51 进一步澄清「视觉壳本期做（按钮 toast，真 OAuth 留给后续 epic）」。Epic 37 Story 37.11 落地 ProfileView 视觉壳含微信绑定按钮 + tap 触发 toast，**不**含真 OAuth。[Source: _bmad-output/planning-artifacts/prd.md#§4 MVP 范围] [Source: _bmad-output/planning-artifacts/epics.md#Story 37.11]
- **何时做**：节点 12 后另起 epic（OAuth 集成属独立产品里程碑，**不**插入节点 1-12 主路径）。

### 4. Profile 顶部 bell（消息通知）真实通知中心

- **位置（ui_design）**：`iphone/ui_design/source/screens/profile.jsx` line 32（`Icons.bell(18, 'white')` 顶部 bell 按钮）+ line 173（`Icons.bell(20,'var(--accent-deep)') ... '消息通知' ... '3 条未读'` 列表项）
- **理由**：Epic 37 Story 37.11 ProfileView 视觉壳**不**含真实通知中心（**仅**为视觉占位，tap 不接路由）。通知中心需要后端 push 通道 + 持久化 + 已读状态 schema，属独立 epic 工作量；本期 ProfileView 视觉壳如保留 bell icon，仅作占位。[Source: _bmad-output/planning-artifacts/epics.md#Story 37.11] [Source: _bmad-output/implementation-artifacts/37-11-profileview-scaffold.md]
- **何时做**：后续 epic（与后端 push 通道 epic 同步启动；非节点 1-12 关键路径）。

### 5. Profile「成就徽章」页 / 「喜欢的道具」页详情

- **位置（ui_design）**：`iphone/ui_design/source/screens/profile.jsx` line 71（`Stat label="成就" value="15"` 顶部数字占位）+ line 172-174（`'成就徽章' / '喜欢的道具'` 列表项 + extra 数字）+ line 252（`DataLossRow icon="🏆" text="15 个成就徽章"` 数据行）
- **理由**：Epic 37 Story 37.11 ProfileView 视觉壳含「成就 / 喜欢的道具」**入口**展示（数字占位），**不**含 push 详情页。详情页需要后端成就规则 schema / 收藏关系 schema，属独立 epic 工作量；与节点 7 宝箱 / 节点 11 合成功能并列产品功能。[Source: _bmad-output/planning-artifacts/epics.md#Story 37.11] [Source: _bmad-output/implementation-artifacts/37-11-profileview-scaffold.md]
- **何时做**：后续 epic（成就系统作为独立产品功能；**非**节点 1-12 关键路径）。

### 6. HomeView 互动后状态条变化（喂食后饱食 +5 等）

- **位置（ui_design）**：`iphone/ui_design/source/screens/home.jsx` line 77（`StatusBar label="饱食" value={72}` 饱食条）+ line 78（心情条）+ line 79（活力条）；ui_design 内 mock 数据写死，无互动 → 状态变化逻辑
- **理由**：Epic 37 Story 37.7 HomeView 视觉壳**不**接互动业务逻辑，状态条数值由 server 通过 `GET /home` 接口下发。互动改变状态属节点 3+ 工作量（LoadHomeUseCase 起接真实数据）。[Source: _bmad-output/planning-artifacts/epics.md#Story 37.7] [Source: _bmad-output/implementation-artifacts/5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据.md] [Source: docs/宠物互动App_V1接口设计.md]
- **何时做**：节点 3 起接 LoadHomeUseCase 落地后端真实状态；互动 → 状态变化属节点 3+ 业务范围。

### 7. WardrobeView 钻石货币真实数值更新

- **位置（ui_design）**：`iphone/ui_design/source/screens/wardrobe.jsx` line 48（`Icons.diamond(16, 'var(--accent)')` 钻石 icon + 数值 chip）
- **理由**：Epic 37 Story 37.9 WardrobeView 视觉壳含钻石数 chip（用 mock 数据展示），**不**接真实货币系统。货币消耗 / 充值需要商城 + 支付 epic 工作量；本期 MVP **不**含商城（PRD §4「暂不做」line 54-62 隐含覆盖商城货币流：「交易系统」line 58）。[Source: _bmad-output/planning-artifacts/epics.md#Story 37.9] [Source: _bmad-output/planning-artifacts/prd.md#§4 暂不做] [Source: _bmad-output/implementation-artifacts/37-9-wardrobeview-scaffold.md]
- **何时做**：后续 epic（与商城 epic 联动；本 MVP 不含商城）。

### 8. 小猫 3D 模型（USDZ / RealityKit）

- **位置（ui_design）**：`iphone/ui_design/source/screens/home.jsx` line 52（`<CatPlaceholder size={220} mood={mood} label="猫 3D 模型"/>` cat stage 视觉占位）+ `iphone/ui_design/source/components/cat-placeholder.jsx`（猫占位组件，97 行，placeholder 为 SVG-based vector shape：line 18 `<svg viewBox=...>` + line 28-36 `<circle>` / `<path>` 头脸耳朵眼鼻嘴）
- **理由**：本条目作用域是 **ui_design prototype** 的 placeholder 替换路径。ui_design 侧 placeholder 是 SVG-based `CatPlaceholder` 组件（vector 勾勒猫头）；当前 SwiftUI 实装侧（Story 37.7 HomeView 视觉壳）独立用 `Image(systemName: "cat.fill")` 占位渲染（两者不是同一份 placeholder）。3D 模型需要美术资源 + RealityKit / SceneKit 集成 spike，属独立 spike 工作量；视觉效果升级，**不**改动产品规则。3D 模型属另起 spike，**非** Story 30.x（Story 30.1-30.4 是 2D cosmetic 渲染：RenderConfig + SpriteRenderer + EquippedCosmeticView + 槽位锚点，scope 完全不同；ui_design prototype 不上线、不需替换）。[Source: _bmad-output/planning-artifacts/epics.md#Story 37.7] [Source: _bmad-output/planning-artifacts/epics.md#Story 30.1-30.4] [Source: _bmad-output/implementation-artifacts/37-7-homeview-scaffold.md]
- **何时做**：美术资源就位后另起 spike（**非**节点 1-12 关键路径）。

### 9. 装扮道具 emoji 占位（仅本期 Scaffold 用）

- **位置（ui_design）**：`iphone/ui_design/source/screens/wardrobe.jsx` line 9-10（道具 grid 占位 `🎩` / `🎀` 等 emoji）+ `iphone/ui_design/source/components/cat-placeholder.jsx` line 39-58（cat stage 上的 bow / hat / scarf 装扮 overlay 由 `<path>` / `<rect>` / `<ellipse>` 等 SVG vector shapes 勾勒，非 emoji；home.jsx line 52 仅 mount `<CatPlaceholder>`，真实 accessory 渲染在 cat-placeholder.jsx 内）
- **理由**：Epic 37 Story 37.9 wardrobe grid 用 emoji 字符（如 🎀 / 🎩 / 👔）作道具占位；Epic 37 Story 37.7 cat stage 装扮则在 cat-placeholder.jsx 用 SVG vector shapes 占位（不是 emoji）。节点 10 起 Story 30.1-30.4 落地 RenderConfig + SpriteRenderer + 升级 EquippedCosmeticView + 槽位锚点，**替换的是 SwiftUI 实装侧**的 EquippedCosmeticView（当前可能用 SF Symbol / Text emoji 占位）走 SpriteRenderer 图像渲染；**ui_design prototype 的 cat-placeholder.jsx 是设计参考资源、不上线、不在替换范围内**；**仓库 grid 内 emoji 占位保留**（grid 视觉本质是 inventory list，emoji 已够清晰，无需图像化）。**三分边界**：「猫身上装扮 overlay（SwiftUI 实装侧）」节点 10 起 Story 30.x 替换为 SpriteRenderer 图像渲染；「仓库 grid emoji」永不替换；「ui_design prototype（cat-placeholder.jsx vector overlay）」不上线，不在替换范围内。[Source: _bmad-output/planning-artifacts/epics.md#Story 37.9] [Source: _bmad-output/planning-artifacts/epics.md#Story 37.7] [Source: _bmad-output/planning-artifacts/epics.md#Story 30.1-30.4]
- **何时做**：节点 10 起 Story 30.1-30.4 替换 **SwiftUI 实装侧**的 EquippedCosmeticView 走 SpriteRenderer 图像渲染；**仓库 grid emoji 保留**；**ui_design prototype（cat-placeholder.jsx）不上线、不在替换范围内**（dev / reviewer 必须读懂这条三分边界）。

### 10. K3M9P2 等美化别名（roomId 美化展示）

- **位置（ui_design）**：`iphone/ui_design/source/app.jsx` line 30 + 65（`roomCode` state，初值 `'7K3-P2'` / fallback `'9X2-L8'`）+ line 18 + 92（friends mock 数据 statusText `在房间 9X2-L8 中`）+ line 215-271（`JoinRoomModal` 定义，line 249 placeholder `例如 9X2-L8`，line 259 `房间代码格式：3 个字母 - 2 位数字` 格式说明）+ `iphone/ui_design/source/screens/home.jsx` line 173 + 183（"创建队伍" / "加入队伍" 按钮，触发 modal；该 range 不含 input field 本身）+ `iphone/ui_design/source/screens/room.jsx` line 33-44（房间代码卡片，line 6 `clipboard.writeText(roomCode)` 复制；ui_design 内变量名仍是 `roomCode`，本期 SwiftUI 实装统一改为 `roomId`）
- **理由**：PRD §4 line 52 钦定「房间标识全程使用 **roomId 字符串**」「本期 MVP **不引入** K3M9P2 等美化别名」（依据 AR21 ID 字符串约定 + db rooms 表无 code 字段）。Epic 37 Story 37.8 进一步钦定 a11y 命名严格 `roomIdDisplay`，禁止旧名 `roomCodeDisplay`。UI 全屏直接显示 roomId 字符串（如「房间 1234567」），分享文案直白展示同样写法。[Source: _bmad-output/planning-artifacts/prd.md#§4 MVP 范围] [Source: _bmad-output/planning-artifacts/epics.md#Story 37.8]
- **何时做**：未来若产品需要可在专门 spike 设计可逆契约（双向映射 roomId ↔ 美化别名 + 防 sender / receiver UX 闭环问题）；**非**节点 1-12 关键路径。

## 如何把某条踢出白名单（4 步流程）

未来 epic / sprint 启动时，若发现本表某条条目需要从"不做"变成"在做"，按以下 4 步执行：

### 步骤 1：触发

未来 epic 启动时，dev / PM 发现本表某条条目需要从"不做"变成"在做"（如节点 7 宝箱 epic 启动时发现需要做"成就徽章"系统）。

### 步骤 2：判断

检查该条目"何时做"字段：

- **若标"永不实装"**（如条目 1 tweaks-panel.jsx）→ 触发 BMAD `correct-course` workflow 改 PRD / 加 ADR 改决议；**不**直接踢出本表。
- **若标"后续 epic / 后续 spike / 节点 N 起"**（如条目 3-10）→ 进入步骤 3。

### 步骤 3：新建 epic / story

通过 BMAD `create-epics-and-stories` workflow 在新 epic / 新 story 内 cover 该条目，story AC 内**显式 cite 本表条目编号**（如「踢出 ui-design-scope-whitelist.md 条目 5：成就徽章详情页」）+ 说明为何此时落地。

### 步骤 4：删除条目

新 story merge 后，dev 在同一 PR（或独立 PR）从本表删除该条目（git history 留痕） + 在 PR description 内贴该 story 链接作为依据；同时更新本表"目录"段落的锚点列表。

## 附录：与 visual-review-checklist.md 的关系

本表与 `iphone/docs/visual-review-checklist.md`（Story 37.13 落地）orthogonal 互补：

| 维度 | visual-review-checklist.md | ui-design-scope-whitelist.md（本表） |
|---|---|---|
| 内容 | Epic 37 **已做的** 50+ 项视觉检查 | Epic 37 **未做的** 10 项 scope 不做项 |
| 用途 | PR review 时 dev 自查 + reviewer 抽检 | 后续 sprint 启动时 dev / PM 决策入口 |
| 维护节奏 | 每次 view 改动 PR 触发 | 仅在「踢出条目 / 追加条目」时更新 |
| 触发时机 | PR review 流程内 | epic / sprint 启动时 |

两文档不重叠，互不替代。
