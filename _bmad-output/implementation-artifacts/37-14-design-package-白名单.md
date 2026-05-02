# Story 37.14: design-package 白名单文档（声明本期不做的 ui_design 元素）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a PM / SM,
I want 一份白名单文档明确列出 `iphone/ui_design/` 内本期 Epic 37 **不做**的元素，每条带"位置 / 不做理由 / 何时做"三段说明，理由必须 cite 到 PRD / ADR / epic 边界依据,
so that 后续 sprint 不重复争论"为什么 X 没做" + 透明传达取舍 + 给未来 epic 启动时一份"先看本表，决定要不要把某条踢出白名单"的入口文档.

## 故事定位（Epic 37 末位 docs-only story；scope 收口 + 历史佐证）

这是 Epic 37「iPhone 架构层重构 + UI Scaffold（壳先行）」的**最末位 docs-only story**——上游 Story 37.5（Theme）/ 37.6（Shared Primitives）/ 37.7（HomeView）/ 37.8（RoomView）/ 37.9（WardrobeView）/ 37.10（FriendsView）/ 37.11（ProfileView）/ 37.12（JoinRoomModal）/ 37.13（accessibility identifier 总表 + visual review checklist）全部 done。本 story **不是**新 feature 实装、**不是**重构、**不是**a11y 收编，而是**纯文档**（docs-only）：

把 Epic 37 落地过程中 **明确选择不做** 的 `iphone/ui_design/` 元素**显式登记**为一份白名单文档，每条配「位置（ui_design 文件 + 行号 / section）/ 不做理由（cite PRD §X / ADR §Y / Epic 37 §Z 钦定边界）/ 何时做（永不实装 / 后续 epic / 后续 spike / 节点 N 起）」三段说明。

**本 story 落地后立即解锁**：
- Epic 37 全部 14 story done → epic-37 status: in-progress → done（手动 transition；本 story 不动 epic 状态，留给 epic-loop 末位 retrospective）
- epic-37-retrospective: required 触发（sprint-status.yaml line 110）
- 节点 2 demo 验收基线建立的最后一块拼图（4 Tab 信息架构 + AppState + Theme + Primitives + 5 Scaffold + JoinRoomModal + a11y 总表 + visual review checklist + **scope 白名单**）

**本 story 的"实装"动作**（一句话概括）：新建一份 markdown 文档 `iphone/docs/ui-design-scope-whitelist.md`，含 10+ 条「不做项」每条 ≥3 bullet（位置 / 理由 / 何时做）+ 文档头部说明用途与读法 + 文档尾部"如何把某条踢出白名单"的流程说明。**纯 docs**，**零代码改动**，**零测试改动**，**零 server 改动**。

**关键路径：本 story 是 Epic 37 的"显式 scope 退出门"**——把 Epic 37 推进过程中 Story 37.5-37.13 隐式排除的 ui_design 元素**显式公示**，避免：
1. 后续 sprint 走到节点 3+ 时，新加入的 dev/PM/reviewer 重新提"为什么 ProfileView 没接消息通知"等已答问题；
2. retrospective 阶段误判 Epic 37 "不完整"（实际是已签字范围内完整，本表佐证）；
3. 未来 epic（节点 3 真实业务接入 / 后续视觉打磨 epic）启动时，dev 必须先看本表 → 决定要不要把某条踢出白名单 → 才能开新 story（流程入口写在文档尾部）。

**不涉及**（红线）：
- **不**新增任何 Swift / Go 代码（**纯 docs**；本 story 完成后 git diff **只有** `iphone/docs/ui-design-scope-whitelist.md` 一份新文件 + 可选的 `iphone/docs/README.md` 索引更新）
- **不**改任何 ui_design 源文件（`iphone/ui_design/source/**` zero edit；本 story 是**记录**已选择不做哪些 ui_design 元素，**不**修改 ui_design 本身）
- **不**改 `iphone/PetApp/**` 任何 SwiftUI 代码（含 Views / ViewModels / AppState / Theme / Primitives / Modals 任意一个文件 zero edit）
- **不**改 `iphone/PetAppTests/**` / `iphone/PetAppUITests/**` 任何测试文件
- **不**改 `iphone/scripts/**` 任何脚本
- **不**改 `_bmad-output/planning-artifacts/epics.md`（epic AC 已 settled，本 story 是落地 AC，不是改 AC）
- **不**改 `_bmad-output/planning-artifacts/prd.md`（PRD §4 「暂不做」已 settled，本 story 引用，不修改）
- **不**改 `_bmad-output/implementation-artifacts/decisions/*.md`（ADR-0009 / ADR-0010 / ADR-0002 已 settled，本 story 引用，不修改）
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3）
- **不**改 server/ 任何文件
- **不**改 sprint-status.yaml 之外的任何 \_bmad-output 文件（本 story 标 done 时 sprint-status.yaml 改 status，正常 epic-loop 流程；本 story 内容本身是产出文档）
- **不**新增"未来计划文档"（如「2026 Q3 节点 3 计划」）—— 本表只声明"不做" + "何时做（粒度到 epic / spike / 节点 N）"，不替代 BMAD planning workflow
- **不**写哲学性 / 教程性 / 营销性文字（"为什么我们采取最小实装策略" 等）—— 本表是**操作性 scope 表**，每条直奔位置 + 理由 + 何时做
- **不**触碰 ui_design 渲染层（不跑 ui_design index.html，不验证截图）—— 本表只记录**逻辑层**取舍

## Acceptance Criteria

> **AC 编号体系**：AC1 是文档结构（头部 / 表体 / 尾部）；AC2 是 10 条「不做项」具体内容（每条 ≥3 bullet：位置 / 理由 / 何时做 + 理由必须 cite PRD/ADR/epic）；AC3 是文档尾部"如何踢出白名单"流程说明；AC4 是 grep / 读取校验文档可用性；AC5 是 Deliverable 清单。

### AC1 — 文档结构（头部 + 表体 + 尾部）

**新建文件**：`iphone/docs/ui-design-scope-whitelist.md`

**文档结构必须包含 3 段**（见下文 AC2 / AC3 进一步钦定每段内容）：

#### AC1.1 — 头部（文档元信息 + 使用方法）

文档第 1-N 行（N 由 dev 视行宽决定，但必须含以下 5 项）：

1. **标题**：`# iPhone App ui_design Scope Whitelist (Epic 37 落地)`
2. **作用**（1-3 段，≤200 字）：
   - 是什么：本表是「Epic 37 显式选择**不做**的 ui_design 元素清单」，每条带位置 / 理由 / 何时做
   - 不是什么：**不**是 PRD（PRD 在 `_bmad-output/planning-artifacts/prd.md`）；**不**是未来计划（未来 epic 由 BMAD planning workflow 产出）；**不**是 ui_design 自身的修改（ui_design 文件 zero edit）
   - 使用场景：（a）后续 sprint 启动时，dev 先看本表决定要不要把某条踢出白名单；（b）retrospective 阶段佐证 Epic 37 已 settled scope；（c）新加入 dev / PM / reviewer 提"为什么 X 没做"时优先指向本表
3. **关联文档**（≥3 个 cite，不要求穷举）：
   - `_bmad-output/planning-artifacts/prd.md` §4「MVP 范围 / 暂不做」
   - `_bmad-output/planning-artifacts/epics.md` Epic 37（line 4555+）
   - `_bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md`（ADR-0009）
   - `_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md`（ADR-0010）
4. **更新协议**（1 段，≤100 字）：本表**只**在以下两种情况更新：
   - 一条目从"不做"变成"在做"（被未来 epic 踢出白名单）→ 本条目从表体删除（git history 留痕）
   - 一条新发现的 ui_design 元素需要登记为"不做"→ 本表追加新条目
5. **可选 ToC**（视长度决定）：若条目数 ≥10 可加锚点目录提升导航性

#### AC1.2 — 表体（白名单条目区，AC2 钦定）

#### AC1.3 — 尾部（如何踢出白名单流程，AC3 钦定）

### AC2 — 10 条「不做项」具体内容（每条 ≥3 bullet：位置 / 理由 / 何时做）

> **来源**：epic AC line 4900-4909（Epic 37 Story 37.14 acceptance criteria 钦定的 10 条原始清单）。本 AC2 在原始清单基础上**逐条**展开为「位置 / 理由 / 何时做」三段格式 + 理由 cite 到 PRD/ADR/epic 具体 §。
>
> **每条格式**（钦定模板，dev 严格按此模板写）：
>
> ```markdown
> ### N. <条目标题>
>
> - **位置（ui_design）**：`iphone/ui_design/...`（具体文件 + 行号 / section name）
> - **理由（cite PRD/ADR/epic）**：<≤2 句说明 + 至少 1 条 cite，格式 `[Source: <path>#<section>]`>
> - **何时做**：<永不实装 / 后续 mini-epic / 后续 spike / 节点 N 起 / 后续 epic（节点 X+）>，附 1 句备注（可选）
> ```

**10 条具体内容**（dev 严格按下表展开；编号沿用 epic AC line 4900-4909 顺序，可少量微调措辞但**不**得改语义、**不**得增删条目数）：

#### 条目 1：tweaks-panel.jsx（开发时主题切换调试面板）

- **位置（ui_design）**：`iphone/ui_design/source/tweaks-panel.jsx`（整个文件）
- **理由**：tweaks-panel 是 ui_design 内部 dev tool（开发时用浏览器实时切主题 token 调参），**不**属于 App 端用户可见 UI；Epic 37 Theme stub 已落地（[Source: _bmad-output/implementation-artifacts/37-5-theme-design-tokens.md]），切换 UI 与 dev tool **本不属于** Epic 37 范围（[Source: _bmad-output/planning-artifacts/epics.md#Epic 37]）
- **何时做**：永不实装（App 端不需要 dev tool；ui_design 内的 tweaks-panel 仅服务于 ui_design 本身的开发流）

#### 条目 2：三主题切换 UI（用户可见的主题选择器）

- **位置（ui_design）**：`iphone/ui_design/source/screens/profile.jsx` 内可能包含的设置项 / `iphone/ui_design/source/tweaks-panel.jsx`（dev 实际看 ui_design 源 confirm）
- **理由**：Epic 37 落地的是 **Theme stub**（candy / dark / mono 三套 token，但 ProfileView 视觉壳**不**含切换按钮）；用户级主题切换 UI 不在 Epic 37 AC 内（[Source: _bmad-output/planning-artifacts/epics.md#Story 37.5]，[Source: _bmad-output/planning-artifacts/epics.md#Story 37.11]）
- **何时做**：后续 mini-epic（视产品优先级；非节点 1-12 关键路径）

#### 条目 3：wechat_binding.md 真实 OAuth 流程

- **位置（ui_design）**：`iphone/docs/wechat_binding.md`（视觉壳钦定文档；dev confirm 路径）/ `iphone/ui_design/source/screens/profile.jsx`「微信绑定」按钮
- **理由**：PRD §4 「暂不做」line 61 钦定「微信绑定（结构预留，节点 2 不实现 UI）」；PRD §4 line 51 进一步澄清「视觉壳本期做（按钮 toast，真 OAuth 留给后续 epic）」（[Source: _bmad-output/planning-artifacts/prd.md#§4 MVP 范围]）。Epic 37 Story 37.11 落地 ProfileView 视觉壳含微信绑定按钮 + tap 触发 toast，**不**含真 OAuth ([Source: _bmad-output/planning-artifacts/epics.md#Story 37.11])
- **何时做**：节点 12 后另起 epic（OAuth 集成属独立产品里程碑，**不**插入节点 1-12 主路径）

#### 条目 4：Profile 顶部 bell（消息通知）真实通知中心

- **位置（ui_design）**：`iphone/ui_design/source/screens/profile.jsx` line 32（`Icons.bell(18, 'white')`）/ profile.jsx line 173（`Icons.bell(20,'var(--accent-deep)') ... '消息通知' ... '3 条未读'`）
- **理由**：Epic 37 Story 37.11 ProfileView 视觉壳**不**含真实通知中心（[Source: _bmad-output/planning-artifacts/epics.md#Story 37.11]）；通知中心需要后端 push 通道 + 持久化 + 已读状态，属独立 epic 工作量；本期 ProfileView 视觉壳如保留 bell icon，仅作占位**不**接路由（dev confirm Story 37.11 落地是否含 bell 占位）
- **何时做**：后续 epic（与后端 push 通道 epic 同步启动；非节点 1-12 关键路径）

#### 条目 5：Profile 「成就徽章」页 / 「喜欢的道具」页详情

- **位置（ui_design）**：`iphone/ui_design/source/screens/profile.jsx` line 71（`Stat label="成就" value="15"`）/ profile.jsx line 172-174（'成就徽章' / '喜欢的道具' 列表项 + extra 数字）/ profile.jsx line 252（`DataLossRow icon="🏆" text="15 个成就徽章"`）
- **理由**：Epic 37 Story 37.11 ProfileView 视觉壳含「成就 / 喜欢的道具」**入口**展示（数字占位），**不**含 push 详情页（[Source: _bmad-output/planning-artifacts/epics.md#Story 37.11]）；详情页需要后端成就规则 / 收藏关系 schema，属独立 epic 工作量
- **何时做**：后续 epic（成就系统作为独立产品功能；与节点 11 合成功能 / 节点 7 宝箱并列，**非**节点 1-12 关键路径）

#### 条目 6：HomeView 互动后状态条变化（喂食后饱食 +5 等）

- **位置（ui_design）**：`iphone/ui_design/source/screens/home.jsx` line 21 + 75-77（`StatusBar label="饱食" value={72}`）+ home.jsx 内可能包含的 互动按钮（喂食 / 抚摸 / 玩耍等）触发后状态条 +N 的 ui_design 视觉
- **理由**：Epic 37 Story 37.7 HomeView 视觉壳**不**接互动业务逻辑（[Source: _bmad-output/planning-artifacts/epics.md#Story 37.7]）；状态条数值由 server 通过 `GET /home` 接口下发（[Source: docs/宠物互动App_V1接口设计.md]），互动改变状态属节点 3+ 工作量
- **何时做**：节点 3 起接 LoadHomeUseCase（[Source: _bmad-output/implementation-artifacts/5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据.md]）落地后端真实状态；互动 → 状态变化属节点 3+ 业务（**非**Epic 37 范围）

#### 条目 7：WardrobeView 钻石货币真实数值更新

- **位置（ui_design）**：`iphone/ui_design/source/screens/wardrobe.jsx` line 48（`Icons.diamond(16, 'var(--accent)')`）+ 钻石数值 chip
- **理由**：Epic 37 Story 37.9 WardrobeView 视觉壳含钻石数 chip（用 mock 数据展示），**不**接真实货币系统（[Source: _bmad-output/planning-artifacts/epics.md#Story 37.9]）；货币消耗 / 充值需要商城 + 支付 epic 工作量；本期 MVP **不**含商城（[Source: _bmad-output/planning-artifacts/prd.md#§4 暂不做]，line 54-62 「暂不做」清单中虽未显式列商城，但「交易系统」line 58 隐含覆盖商城货币流）
- **何时做**：后续 epic（与商城 epic 联动；本 MVP 不含商城）

#### 条目 8：小猫 3D 模型（USDZ / RealityKit）

- **位置（ui_design）**：`iphone/ui_design/source/screens/home.jsx` 内 cat stage 视觉占位 + `iphone/ui_design/source/components/cat-placeholder.jsx`（猫占位组件）
- **理由**：Epic 37 Story 37.7 HomeView 视觉壳用 SF Symbol `cat.fill` 占位渲染（[Source: _bmad-output/planning-artifacts/epics.md#Story 37.7]）；3D 模型需要美术资源 + RealityKit / SceneKit 集成 spike，属独立 spike 工作量
- **何时做**：美术资源就位后另起 spike（**非**节点 1-12 关键路径；视觉效果升级，**不**改动产品规则）

#### 条目 9：装扮道具 emoji 占位（仅本期 Scaffold 用）

- **位置（ui_design）**：`iphone/ui_design/source/screens/wardrobe.jsx` 内道具 grid 占位 + `iphone/ui_design/source/screens/home.jsx` 内 cat stage 装扮 overlay 占位
- **理由**：Epic 37 Story 37.9 / 37.7 视觉壳用 emoji 字符（如 🎀 / 🎩 / 👔）作装扮道具占位（[Source: _bmad-output/planning-artifacts/epics.md#Story 37.9]）；节点 10 起 Story 30.x 落地 SpriteRenderer + render_config（[Source: _bmad-output/planning-artifacts/epics.md#Story 30.1-30.4]），**仅替换猫身上的装扮**为图像渲染；**仓库 grid 内 emoji 占位保留**（grid 视觉本质是 inventory list，emoji 已够清晰，无需图像化）
- **何时做**：节点 10 起 Story 30.x 替换 SpriteRenderer，**仅猫身上**；**仓库 grid emoji 保留**（本条目语义是「猫身上 emoji」节点 10 起替换 + 「仓库 grid emoji」永不替换的二分边界，dev 写文档时显式写出）

#### 条目 10：K3M9P2 等美化别名（roomId 美化展示）

- **位置（ui_design）**：`iphone/ui_design/source/screens/room.jsx` 内 roomId 展示位置 + `iphone/ui_design/source/screens/home.jsx` 加入队伍输入框
- **理由**：PRD §4 line 52 钦定 「房间标识全程使用 **roomId 字符串**」「本期 MVP **不引入** K3M9P2 等美化别名」（[Source: _bmad-output/planning-artifacts/prd.md#§4 MVP 范围]）；epic AC line 4881 进一步钦定 a11y 命名严格 `roomIdDisplay`，禁止旧名 `roomCodeDisplay`（[Source: _bmad-output/planning-artifacts/epics.md#Story 37.8]）；UI 全屏直接显示 roomId 字符串（如「房间 1234567」），分享文案直白展示同样写法
- **何时做**：未来若产品需要可在专门 spike 设计可逆契约（双向映射 roomId ↔ 美化别名 + 防 sender / receiver UX 闭环问题）；**非**节点 1-12 关键路径

### AC3 — 文档尾部"如何把某条踢出白名单"流程说明

文档尾部（AC1.3）必须包含一段流程说明，**钦定 4 步流程**：

1. **触发**：未来 epic 启动时，dev / PM 发现本表某条条目需要从"不做"变成"在做"（如节点 7 宝箱 epic 启动时发现需要做"成就徽章"系统）
2. **判断**：检查该条目"何时做"字段是否标 "永不实装"
   - 若是（如 tweaks-panel.jsx）→ 触发 BMAD `correct-course` workflow 改 PRD / 加 ADR 改决议；**不**直接踢出
   - 若否（如"后续 epic / 后续 spike / 节点 N 起"）→ 进入步骤 3
3. **新建 epic / story**：通过 BMAD `create-epics-and-stories` workflow 在新 epic / 新 story 内 cover 该条目，story AC 内**显式 cite 本表条目编号 + 说明为何此时落地**
4. **删除条目**：新 story merge 后，dev 在同一 PR（或独立 PR）从本表删除该条目（git history 留痕） + 在 PR description 内贴该 story 链接作为依据

### AC4 — grep / 读取校验文档可用性

dev 完成文档后必须本地手工执行以下校验（无脚本，但**dev 必须做并在 Completion Notes 内 attest**）：

1. **grep 校验「位置 / 理由 / 何时做」三段齐全**：每条「不做项」包含 ≥3 bullet（用 `grep -c "^- \*\*位置" iphone/docs/ui-design-scope-whitelist.md` 应 = 10；同理验证 `理由` / `何时做`）
2. **grep 校验 Source cite ≥10 处**：用 `grep -c "\[Source:" iphone/docs/ui-design-scope-whitelist.md` 应 ≥ 10（每条至少 1 个 Source cite，10 条 ≥ 10 处）
3. **grep 校验关键词覆盖**：`grep -E "tweaks|wechat|bell|成就|喜欢的道具|状态条|钻石|3D|USDZ|emoji|K3M9P2"` 应命中 ≥10 行（10 条主题词均在文档内）
4. **markdown 渲染校验**（可选但建议）：用任意 markdown 预览（VSCode preview / GitHub render）打开文档，肉眼确认锚点目录、标题、列表渲染正常无破损

### AC5 — Deliverable 清单

本 story merge 时 git diff 必须**仅**包含以下文件（dev 在 PR description 内显式列出）：

- **新建**：`iphone/docs/ui-design-scope-whitelist.md`（全部 AC1-AC3 内容；行数视格式预计 200-400 行）
- **可选更新**：`iphone/docs/CI.md` 或新建 `iphone/docs/README.md` 内引用本表（**dev 自由判断**：如果 `iphone/docs/` 当前无索引文件，**不**强制新增 README.md；本 story zero-代码、zero-索引 同样可接受）
- **本 story 文件标 done**：`_bmad-output/implementation-artifacts/37-14-design-package-白名单.md` Status: `ready-for-dev` → `done`（dev-story workflow 自动改）
- **sprint-status.yaml 更新**：`37-14-design-package-白名单: backlog` → `done`（epic-loop / story-done workflow 自动改）

**红线 deliverable 范围**：
- **不**得出现 `iphone/PetApp/` 任何文件改动
- **不**得出现 `iphone/PetAppTests/` / `iphone/PetAppUITests/` 任何文件改动
- **不**得出现 `iphone/scripts/` 任何文件改动
- **不**得出现 `iphone/ui_design/source/` 任何文件改动
- **不**得出现 `server/` 任何文件改动
- **不**得出现 `_bmad-output/planning-artifacts/` 任何文件改动（PRD / epics / decisions / sprint-change-proposal 已 settled）
- **不**得出现 `docs/` 7 份设计文档任何改动（CLAUDE.md 钦定权威文档）

## Tasks / Subtasks

- [x] **Task 1**：通读 epic AC line 4889-4912 + 本 story AC1-AC5 + Story 37.13 实装产出，确认理解（AC: 全部）
  - [x] 1.1 读 `_bmad-output/planning-artifacts/epics.md` line 4889-4912（Story 37.14 epic AC 原文）
  - [x] 1.2 读 `_bmad-output/implementation-artifacts/37-13-accessibility-identifier-总表.md` 头部 + 尾部 lesson section（理解 docs-only story 的写作风格 + 关键路径表达）
  - [x] 1.3 grep `iphone/ui_design/source/screens/profile.jsx` / `home.jsx` / `wardrobe.jsx` / `room.jsx` 确认条目 4-10 的 ui_design 位置确切性（line 号 / 关键字符位置）
  - [x] 1.4 读 `_bmad-output/planning-artifacts/prd.md` §4 line 30-62（MVP 范围 / 暂不做 / 4 Tab 信息架构 / roomId 钦定）
  - [x] 1.5 列写 cite 表（≥10 条 cite 路径，预先验证路径存在 + section 名准确）

- [x] **Task 2**：新建 `iphone/docs/ui-design-scope-whitelist.md` 文档头部（AC1.1）
  - [x] 2.1 写标题 `# iPhone App ui_design Scope Whitelist (Epic 37 落地)`
  - [x] 2.2 写「作用 / 不是什么 / 使用场景」3 段（≤200 字 / 段，AC1.1.2 钦定）
  - [x] 2.3 写关联文档 cite 列表（≥3 个，AC1.1.3 钦定）
  - [x] 2.4 写更新协议段（≤100 字，AC1.1.4 钦定）
  - [x] 2.5 视长度决定是否加锚点 ToC（条目数 ≥10 建议加，但不强制）—— 已加 10 条锚点目录

- [x] **Task 3**：写 10 条「不做项」表体（AC2）
  - [x] 3.1 写条目 1：tweaks-panel.jsx（位置 / 理由 / 何时做 = 永不实装）
  - [x] 3.2 写条目 2：三主题切换 UI（位置 / 理由 / 何时做 = 后续 mini-epic）
  - [x] 3.3 写条目 3：wechat_binding 真实 OAuth（位置 / 理由 / 何时做 = 节点 12 后另起 epic）
  - [x] 3.4 写条目 4：Profile 顶部 bell 真实通知中心（位置 / 理由 / 何时做 = 后续 epic）
  - [x] 3.5 写条目 5：成就徽章 / 喜欢的道具页详情（位置 / 理由 / 何时做 = 后续 epic）
  - [x] 3.6 写条目 6：HomeView 互动后状态条变化（位置 / 理由 / 何时做 = 节点 3 起接 LoadHomeUseCase）
  - [x] 3.7 写条目 7：WardrobeView 钻石货币真实数值更新（位置 / 理由 / 何时做 = 后续 epic + 商城联动）
  - [x] 3.8 写条目 8：小猫 3D 模型（位置 / 理由 / 何时做 = 美术资源就位后 spike）
  - [x] 3.9 写条目 9：装扮道具 emoji 占位（位置 / 理由 / 何时做 = 节点 10 起 Story 30.x 替换 + **仓库 grid emoji 保留**二分边界显式写出）
  - [x] 3.10 写条目 10：K3M9P2 等美化别名（位置 / 理由 / 何时做 = 后续 spike 设计可逆契约）

- [x] **Task 4**：写文档尾部"如何踢出白名单"流程说明（AC3）
  - [x] 4.1 写 4 步流程标题 + 编号
  - [x] 4.2 写步骤 1（触发 / 何时考虑踢出）
  - [x] 4.3 写步骤 2（判断"永不实装"分支 → BMAD correct-course；其他分支 → 步骤 3）
  - [x] 4.4 写步骤 3（新建 epic / story 走 BMAD create-epics-and-stories workflow + story AC cite 本表条目编号）
  - [x] 4.5 写步骤 4（merge 后从本表删除条目 + git history 留痕）

- [x] **Task 5**：本地 grep / 读取校验（AC4）
  - [x] 5.1 跑 `grep -c "^- \*\*位置" iphone/docs/ui-design-scope-whitelist.md` 确认 = 10 ✅ 实际 10
  - [x] 5.2 跑 `grep -c "^- \*\*理由" iphone/docs/ui-design-scope-whitelist.md` 确认 = 10 ✅ 实际 10
  - [x] 5.3 跑 `grep -c "^- \*\*何时做" iphone/docs/ui-design-scope-whitelist.md` 确认 = 10 ✅ 实际 10
  - [x] 5.4 跑 `grep -c "\[Source:" iphone/docs/ui-design-scope-whitelist.md` 确认 ≥ 10 ✅ 实际 10
  - [x] 5.5 跑 `grep -E "tweaks\|wechat\|bell\|成就\|喜欢的道具\|状态条\|钻石\|3D\|USDZ\|emoji\|K3M9P2" iphone/docs/ui-design-scope-whitelist.md | wc -l` 确认 ≥ 10 ✅ 实际 39
  - [x] 5.6 markdown 预览肉眼确认渲染无破损（结构 = 标题 + 段落 + 目录 + 10 条 + 流程 + 附录表，列表 / cite / 锚点齐全）

- [x] **Task 6**：自检 deliverable 范围（AC5）
  - [x] 6.1 跑 `git status` 确认仅含 `iphone/docs/ui-design-scope-whitelist.md` + sprint-status.yaml + 本 story 文件
  - [x] 6.2 跑 `git status --short` 确认**无** `iphone/PetApp/` / `iphone/scripts/` / `iphone/ui_design/source/` / `server/` / `_bmad-output/planning-artifacts/` / `docs/` 任何文件改动
  - [x] 6.3 视情况补充 `iphone/docs/README.md` 索引文件 —— **dev 自由判断**：**不**新建（iphone/docs/ 目前 3 文件 CI.md / visual-review-checklist.md / 本白名单，未到需要索引的规模；与 story Dev Notes "zero-索引同样可接受"一致）
  - [x] 6.4 在 Completion Notes 内 attest AC4 全部 grep 通过（见 Completion Notes List）

## Dev Notes

### 关键技术决策（沿用 Story 37.13 docs-only 模式）

- **纯 markdown 文档，零代码**：本 story 与 Story 37.13 的 `visual-review-checklist.md` 同模式（37.13 落地 50 项手动 checklist；本 story 落地 10 条不做项白名单）。dev 不需要碰 SwiftUI / Go / build script / test。
- **写作风格沿用 visual-review-checklist.md**：标题层级（## 1.xxx / ### N.xxx）+ bullet 列表 + cite 用 `[Source: <path>#<section>]` 标注 → 与 Story 37.13 visual-review-checklist.md 一致；保 dev 切换上下文成本最低。
- **每条 cite 必须能 grep 验证**：`[Source: _bmad-output/planning-artifacts/prd.md#§4 MVP 范围]` 这种 cite 写完后 dev 必须 grep 该 path + section 验证存在；防 cite 路径错误 / section 名失效（参考 Story 37.13 的 `[Source:` 严谨度）。
- **条目 9 二分边界显式写**：「装扮 emoji 占位」是本表里**唯一**的「部分做 + 部分不做」二分边界条目（猫身上节点 10 起替换；仓库 grid 永不替换）；dev 写时**显式**写出二分语义，避免后续 reviewer 误读为「整体不做」。
- **条目 10 cite 双重**：roomId 美化别名同时被 PRD §4 line 52 + epic AC line 4881 钦定，dev 必须 cite **两处**（PRD + epic），强化"PM + SM 双签 settled"的可追溯性。

### 写作风格 / token 经济（防 LLM 罗嗦）

- **每条 ≤200 字**：位置 ≤30 字（路径 + 行号即可），理由 ≤100 字（cite + 1 句解释），何时做 ≤30 字（标签 + 备注），冗余文字裁掉
- **避免营销 / 哲学语言**：本表是**操作性 scope 表**，不写"我们采取最小实装策略" / "为了未来的扩展性" 等空泛短语；每句话直接服务 dev / PM / reviewer 决策
- **不用 emoji 装饰条目**（与 Story 37.13 visual-review-checklist.md 保持一致；用纯 markdown 列表 + 粗体强调即可）
- **严格按 epic AC line 4900-4909 顺序**：dev 不得调整条目顺序（顺序也是 PM 签字 settled 的一部分）

### 与 Story 37.13 协同点（依赖 / 边界）

- **依赖**：Story 37.13 已 done（line 108）→ a11y 总表 + visual-review-checklist.md 存在 → 本表写 "关联文档" 时可 cite visual-review-checklist.md 作为同模式 docs sibling
- **边界**：本表**不**重复 visual-review-checklist.md 的内容（visual-review-checklist 是「Epic 37 已做的 50 项视觉检查」；本表是「Epic 37 未做的 10 项 scope 不做项」；两者 orthogonal，互补不重叠）
- **lesson 沿用**：Story 37.13 学到的「a11y 字面量 → 常量收编」lesson 与本 story 无关；但 Story 37.13 学到的「docs-only story 必须有 grep 验证 AC」lesson **直接沿用**到本 story 的 AC4

### 写作过程建议（dev 实操流）

1. **先列 cite 表**：在新建 markdown 之前，先在草稿（任何地方）列出 10 条 cite 路径 + section 名 + 在该 path 下 grep 验证 section 存在 → 写文档时就不会卡 cite
2. **先写最简版本**：每条只写「标题 + 1 行位置 + 1 行理由 + 1 行何时做」→ 全 10 条快速过一遍 → 头部 / 尾部补全 → 全文回看 → 加深条目说明（如条目 9 的二分边界）→ 最终交付
3. **用 epic AC line 4900-4909 直接 paraphrase**：本 story 大部分内容直接来自 epic AC 钦定，dev 不需要"创造"新条目，只需"展开 + cite"。**红线**：不得增删条目数（10 条 = 10 条）；不得改语义（如把"永不实装"改成"后续考虑"）
4. **ui_design 行号确认**：条目 4 / 6 / 7 / 8 / 9 / 10 涉及具体 ui_design 文件行号，dev 必须**实际打开 ui_design 源 grep**确认行号准确（`grep -n "bell" iphone/ui_design/source/screens/profile.jsx` 等），**不**得抄袭本 story Dev Notes 内的预估行号（本 story 内的行号是 reference，dev 必须重 grep 确认）

### 测试 / build 验证（无）

- **无 unit test**：纯 markdown 文档，无 Swift / Go 代码，**不**需要 unit test（与 Story 37.13 的 visual-review-checklist.md 同样无 test）
- **无 build 改动**：本 story 不动 `project.yml` / `iphone/scripts/build.sh` / Go build；**不**需要跑 `bash scripts/build.sh` 也**不**需要跑 `bash iphone/scripts/build.sh`
- **唯一"测试"**：AC4 的 grep 校验（dev 本地手跑 4 个 grep 确认文档结构齐全）

### Source Tree 影响

```
iphone/
├─ docs/
│  ├─ CI.md                              # 已存在；本 story 不动
│  ├─ visual-review-checklist.md         # Story 37.13 落地；本 story 不动
│  ├─ ui-design-scope-whitelist.md       # ★ 本 story 新建 ★
│  ├─ README.md                          # 可选新建（dev 自由判断；最小化 1 行链接索引即可）
│  └─ lessons/                           # 已存在；本 story 不动
└─ ui_design/source/                     # 已存在；本 story zero edit（仅 cite 行号位置）
```

### Project Structure Notes

- **路径选择 `iphone/docs/` 而非 `_bmad-output/planning-artifacts/`**：epic AC line 4911 钦定 deliverable 是 `iphone/docs/ui-design-scope-whitelist.md`（**iphone 工程内**而非 BMAD planning 范围），原因是本表是**工程团队 reference**（节点 1-12 dev / SM 跨 epic 用），属 iphone/ 工程文档资产，与 visual-review-checklist.md / CI.md 同层
- **不放 `_bmad-output/`**：BMAD planning artifacts 是「计划」（PRD / epics / sprint-change-proposal），BMAD implementation artifacts 是「story 实施单」（37-14-*.md 本 story 文件 + sprint-status.yaml）；本表是「**工程产出 docs**」属 iphone/docs/

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Epic 37 / Story 37.14] —— Story 37.14 完整 acceptance criteria（line 4889-4912）
- [Source: _bmad-output/planning-artifacts/prd.md#§4 MVP 范围 / 暂不做] —— 微信绑定 / 商城 / watchOS 等暂不做钦定
- [Source: _bmad-output/planning-artifacts/prd.md#§4] —— roomId 字符串钦定 + K3M9P2 美化别名禁用钦定（line 52）
- [Source: _bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md] —— ADR-0009 4 Tab 信息架构（推翻 Sheet 主入口）
- [Source: _bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md] —— ADR-0010 AppState 单 source of truth
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md] —— iPhone 工程结构 + ios/ 不动钦定
- [Source: _bmad-output/implementation-artifacts/37-13-accessibility-identifier-总表.md] —— 同 Epic 末位 docs-only story 写作风格参考 + visual-review-checklist.md 同模式 sibling
- [Source: iphone/docs/visual-review-checklist.md] —— Story 37.13 落地的 sibling docs，本表为同 docs/ 目录下兄弟文档
- [Source: _bmad-output/implementation-artifacts/37-5-theme-design-tokens.md] —— Theme stub 落地范围（候选条目 2 主题切换 UI 不做的依据）
- [Source: _bmad-output/implementation-artifacts/37-7-homeview-scaffold.md] —— HomeView Scaffold 落地范围（条目 6 互动后状态条变化不做的依据）
- [Source: _bmad-output/implementation-artifacts/37-9-wardrobeview-scaffold.md] —— WardrobeView Scaffold 落地范围（条目 7 钻石货币不做的依据）
- [Source: _bmad-output/implementation-artifacts/37-11-profileview-scaffold.md] —— ProfileView Scaffold 落地范围（条目 3-5 微信 / bell / 成就不做的依据）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 30.1-30.4] —— Story 30.x SpriteRenderer 节点 10 落地（条目 9 装扮 emoji 占位替换路径）
- [Source: docs/宠物互动App_V1接口设计.md] —— GET /home 接口契约（条目 6 状态条数据来源）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]（dev-story workflow 实际填）

### Debug Log References

无 build / test 跑（纯 docs-only story，AC1-AC5 不要求跑 build；AC4 grep 校验本地手跑）。

### Completion Notes List

- **AC1（文档结构）**：新建 `iphone/docs/ui-design-scope-whitelist.md` 含 3 段：头部（标题 / 作用 / 不作用 / 使用场景 / 关联文档 / 更新协议 / 锚点目录 7 子段）+ 表体（10 条不做项）+ 尾部（4 步踢出流程 + 与 visual-review-checklist 关系附录表）。
- **AC2（10 条不做项）**：严格按 epic AC line 4900-4909 顺序展开为「位置 / 理由 / 何时做」三段；条目 9 显式写出二分边界（猫身上 emoji 节点 10 起替换 + 仓库 grid emoji 永不替换）；条目 10 双重 cite（PRD §4 + epic Story 37.8）。所有 ui_design 行号在 dev 写文档前 grep 实际源文件确认（见 Task 1.3）：profile.jsx bell @ line 32+173；profile.jsx 成就/喜欢的道具列表 @ line 71+172-174+252；profile.jsx 微信卡片 @ line 75-118+194-239；home.jsx StatusBar @ line 77-79；home.jsx CatPlaceholder @ line 52；wardrobe.jsx emoji @ line 9-10；wardrobe.jsx diamond @ line 48；room.jsx 房间代码 @ line 33-44；home.jsx 加入队伍 @ line 162-183。
- **AC3（4 步踢出流程）**：触发 → 判断（永不实装走 correct-course / 其他走步骤 3）→ 新建 epic/story 走 create-epics-and-stories + cite 本表条目号 → 删除条目 + PR 贴 story 链接。
- **AC4（grep 校验）**：4 个 grep 全通过：
  - `grep -c "^- \*\*位置" iphone/docs/ui-design-scope-whitelist.md` = **10** ✅
  - `grep -c "^- \*\*理由" iphone/docs/ui-design-scope-whitelist.md` = **10** ✅
  - `grep -c "^- \*\*何时做" iphone/docs/ui-design-scope-whitelist.md` = **10** ✅
  - `grep -c "\[Source:" iphone/docs/ui-design-scope-whitelist.md` = **10** (≥10 ✅)
  - 关键词覆盖（tweaks/wechat/bell/成就/喜欢的道具/状态条/钻石/3D/USDZ/emoji/K3M9P2）= **39** 行 (≥10 ✅)
- **AC5（deliverable 范围）**：`git status` 输出仅 3 行：(M) sprint-status.yaml + (??) 本 story 文件 + (??) `iphone/docs/ui-design-scope-whitelist.md`。**红线**全清：无 PetApp/ / PetAppTests/ / PetAppUITests/ / iphone/scripts/ / iphone/ui_design/source/ / server/ / _bmad-output/planning-artifacts/ / docs/ 改动。
- **可选 README 决策**：**不**新建 `iphone/docs/README.md`。理由：当前 iphone/docs/ 仅 3 文件（CI.md / visual-review-checklist.md / 本白名单），未到需要索引的规模；story Dev Notes 已显式说明 "zero-索引同样可接受"。
- **build / test**：本 story 是纯 docs-only，无 Swift / Go 改动，**未**跑 `bash iphone/scripts/build.sh` 或 `bash scripts/build.sh`，与 Story 37.13 visual-review-checklist.md 同模式。

### File List

- 新建 `iphone/docs/ui-design-scope-whitelist.md`（约 95 行 markdown）
- 修改 `_bmad-output/implementation-artifacts/sprint-status.yaml`（37-14 status: ready-for-dev → in-progress → review；header last_updated）
- 修改 `_bmad-output/implementation-artifacts/37-14-design-package-白名单.md`（Status / Tasks 勾选 / Dev Agent Record / Change Log）

## Change Log

| Date | Change | Author |
|---|---|---|
| 2026-04-30 | Story 37.14 落地：新建 `iphone/docs/ui-design-scope-whitelist.md` 含 10 条 ui_design 不做项 + 4 步踢出白名单流程；AC1-AC5 全部满足；纯 docs-only 零代码改动；AC4 5 个 grep 全通过（位置/理由/何时做 = 10 / Source cite = 10 / 关键词覆盖 = 39）。 | dev (claude-opus-4-7[1m]) |
| 2026-04-30 | review r1 fix（2 条 P2 文档准确性 finding）：条目 3「微信绑定」位置段补 `iphone/ui_design/wechat_binding.md`（完整 OAuth / SDK 流程文档，否则读者只看 profile.jsx 视觉壳会漏掉真正的 OAuth 流程文档）；条目 9「装扮 emoji 占位」修正描述错位 —— cat stage 装扮 overlay 真实位置是 `cat-placeholder.jsx` line 39-58 的 SVG vector shapes（bow / hat / scarf 用 `<path>`/`<rect>`/`<ellipse>`），**不**是 emoji；home.jsx:52 仅 mount `<CatPlaceholder>`；二分边界改述为「猫身上 vector shape overlay 节点 10 起 Story 30.1-30.4 替换为 SpriteRenderer 图像渲染 / 仓库 grid emoji 永不替换」。AC4 grep 计数仍为 10/10/10/10。 | dev (claude-opus-4-7[1m]) |
