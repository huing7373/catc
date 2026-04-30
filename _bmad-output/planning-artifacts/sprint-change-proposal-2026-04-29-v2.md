# Sprint Change Proposal v2 — iPhone 架构层重构 + UI Scaffold（壳先行）

- **日期**：2026-04-29（v2 起草）/ 2026-04-29（v2.1 修订）/ 2026-04-29（v2.2 修订）
- **作者**：Bob (Scrum Master) via `bmad-correct-course`
- **取代**：[v1（同日）](sprint-change-proposal-2026-04-29.md)（codex 审 Reject，6 个 BLOCKER）
- **触发**：sprint 中段战略调整 —— 用户决定先把 5 屏 1 弹窗的视觉壳一次性铺完，且强约束遵守 ui_design IA → 牵出 iPhone 端导航架构 + 状态源两个底层架构问题
- **依据**：
  - `iphone/ui_design/README.md`（高保真原型）
  - codex audit on v1（2026-04-29，Reject + 6 BLOCKER）
  - codex audit on v2（2026-04-29，Reject + 3 BLOCKER + 5 WARN —— "补这 3 处就接近 Accept with revisions"）
  - codex audit on v2.1（2026-04-29，Reject + 3 BLOCKER + 2 WARN + 5 PASS —— 余下 BLOCKER：roomId 数字心智混用、chestSlot/HomeView 接缝 SwiftUI 不可实现、a11y/state owner 命名打架；本 v2.2 全部修复）
  - 用户前置决议（2026-04-29）：1=A / 2=C / 3=A / 4=B / 5=B / 6=B
  - 用户 v2.1 决议（2026-04-29）：1=A（取消 K3M9P2 别名，UI 全用 roomId 字符串）
- **范围分类**：**Major** —— 推翻 Story 2.3 + Story 5.5 的核心架构决策；含 2 个新 ADR；改动面跨节点

## v2.1 → v2.2 修订摘要（codex audit on v2.1 反馈修复）

| codex audit on v2.1 项 | 等级 | 本次 v2.2 修订 |
|---|---|---|
| BLOCKER 1 残留：12.7 / 35.3 / 37.12 仍写 "roomId 数字 / 6-10 位数字 / 前端校验 ≥6 位数字"，与 String 契约心智冲突 | 🔴 → ✅ | 全文清扫"数字"语义；统一为 "`roomId: String`（AR21）"；37.12 输入框格式提示改为"输入好友分享给你的房间号"不暗示纯数字；JoinRoomUseCase 参数类型 `String`；35.3 解析改 String |
| BLOCKER 7 chestSlot/HomeView 接缝 SwiftUI 不可实现（`any P` 不能 conform `ObservableObject`，`protocol View typealias` 错误） | 🔴 → ✅ | 改用 **class 层次结构**：基类 `class HomeViewModel: ObservableObject` + 子类 `MockHomeViewModel`/`RealHomeViewModel` 分别 override；HomeView 是 generic struct `<ChestSlot: View>` 持 `@ObservedObject var state: HomeViewModel`（基类，符合 SwiftUI 契约）；其它 4 屏（Room/Wardrobe/Friends/Profile）同样改 class 层次 |
| BLOCKER 10 v2.1 新硬伤：a11y `roomIdDisplay` vs `roomCodeDisplay` 打架；37.12 残留 `AppState.joinTeam(code:)`；JoinRoomModal state owner 双写 | 🔴 → ✅ | a11y 全文统一 `roomIdDisplay`；37.12 删除 `AppState.joinTeam(code:)` 残留方法名，改注入 `onConfirm` 闭包；showJoinModal 唯一 owner = HomeViewModel 基类的 `@Published var showJoinModal: Bool`，HomeView 用 `$state.showJoinModal` 不再加本地 `@State` |
| WARN 4 残留：ADR-0010 §4.3 未来节点表仍用旧名 `currentEmojis` | 🟡 → ✅ | §4.3 表统一改 `emojiCatalog` 与 §3.2 字段定义对齐 |
| WARN 8 残留：Icons 表与原型实际键集不一致（编了 `wardrobe/profile/cloud/fish/yarn/chevron`，漏了 `user/plus/back/ball/footprint/sparkle/shield/warn/trophy/chevronRight/dot`） | 🟡 → ✅ | Icons 表完全重做，按 `iphone/ui_design/source/components/primitives.jsx` 内 Icons 对象**真实 25 键** 1:1 对齐 + 给出 SF Symbol 映射；标注视觉精度由 Story 37.13 视觉 review 把关 |

## v2 → v2.1 修订摘要（codex audit on v2 反馈修复）

| codex audit 项 | 等级 | 本次修订 |
|---|---|---|
| BLOCKER 1 · 房间代码 UX 不闭环（K3M9P2 sender/receiver 不闭环） | 🔴 → ✅ | 取消 K3M9P2 别名；UI 全程显示 roomId 字符串；输入框接受 alphanumeric / digits（贴合 AR21）；分享文案直白 |
| BLOCKER 9 · ADR-0009 / ADR-0010 对 currentTab 所有权直接冲突 | 🔴 → ✅ | currentTab 归 AppCoordinator（与 presentedSheet 同级）；不进 AppState；两 ADR 同步；ResetIdentityViewModel / LaunchingViewModel 补入 ADR-0010 §3.5 表 |
| BLOCKER 10a · currentRoomId 类型 Int64 ↔ server 字符串契约漂移 | 🔴 → ✅ | 全文统一改为 String?（贴合 AR21 + epics.md L2016 server /home `room.currentRoomId` 字符串约定） |
| BLOCKER 10b · ViewModel 用 @EnvironmentObject 反模式 | 🔴 → ✅ | ADR-0010 §3.1 加 ADR 级硬规则：ViewModel 仅允许构造注入 AppState；@EnvironmentObject 仅 View 层用；v2 各处 Real ViewModel 文案统一改 |
| WARN 2 · "替代 snapshot" 表述夸大 | 🟡 → ✅ | Story 37.13 表述降级为「合规兜底」，不再宣称等价 snapshot |
| WARN 4 · currentEmojis / Friends domain 边界模糊 | 🟡 → ✅ | ADR-0010 §3.2 字段重命名 `currentEmojis → emojiCatalog` + 加注释明确为配置目录；Friends 数据明确归 FriendsViewModel cache（不进 AppState） |
| WARN 5 · 宝箱位"槽位机制"还是 Color.clear 占位 | 🟡 → ✅ | Story 37.7 加硬接缝：HomeView 暴露 `chestSlot: () -> ChestSlotContent` ViewBuilder closure；Story 21.1 时调用方传入 ChestCardView 即可，HomeView 内部不改 |
| WARN 7 · Risk Register 漏 3 条 | 🟡 → ✅ | §6 风险表加 4 条：roomId 类型契约（取代旧 roomCode 风险）+ currentTab 所有权冲突 + ViewModel 注入反模式 + 14 story 共享协议冻结点 + partial revert git blame 影响 |
| WARN 8 · 微信绑定 PRD 边界扩义 + Icons 映射太虚 | 🟡 → ✅ | §8.2 Handoff 加 PM 明确签字位（不签不能进 dev）；Story 37.6 加完整 Icons → SF Symbol 对照表 |

---

## 0. v1 → v2 关键变化（一图速览）

| 维度 | v1（被 Reject） | v2 |
|---|---|---|
| Scope | Moderate | **Major** |
| 新 ADR | 0 | **2**（ADR-0009 导航 + ADR-0010 AppState） |
| Story 数 | 10 | **14** |
| 下游 Story 修订数 | 5 | **7**（补 12.7 / 33.6 / 24.4 / 35.2-35.4） |
| 导航模型 | 仅"加入 4 Tab"（与 Story 2.3 Sheet 主入口共存，自相矛盾）| **TabView 推翻 Story 2.3 主入口**；Sheet 仅留次级场景 |
| roomId/roomCode 契约 | 假设 db 有 code 字段（事实错误） | **UI 全用 roomId 字符串**（AR21 ID 字符串约定）；不再生成 K3M9P2 美化别名（v2.1 修订：UX 闭环考虑——A 分享 K3M9P2 给 B，B 输入框接受不到反推 roomId） |
| 测试栈 | ViewInspector + SnapshotTesting（违背 ADR-0002 §3.1） | **严守 ADR-0002 §3.1**：纯 XCTest + 手写 mock + accessibility identifier 全屏总表 |
| Source of Truth | 隐含 3 份（AppState + HomeViewModel.homeData + RoomViewModel） | **AppState 单 source of truth**（推翻 Story 5.5 数据流） |
| 微信绑定 | 静默删 | **视觉壳做但行为 toast**（按用户决议 #4=B） |
| design-package 白名单 | 无 | **必含**（Story 37.14） |
| Risk Register | 6 条 | **10 条**（补 4 条 BLOCKER 对应风险） |

---

## 1. Issue Summary

iPhone 端 UI 设计原型（`iphone/ui_design/`）已是接近 spec 级的高保真规格（5 屏 + 1 弹窗 + Design Tokens + 状态机 + 数据模型），用户希望：

1. 先一次性把所有界面壳铺完（mock 数据 / 不调 API），后续 epic 接入业务逻辑
2. 严格遵守 ui_design IA：4 Tab + Home Tab idle⟷inRoom 互斥状态机

但**两个深层架构问题**让 v1 提案被 codex 审 Reject：

1. **ui_design 4 Tab IA 与现 Story 2.3 钦定的"3 CTA + Sheet 主入口"不相容**
2. **5 个 ViewModel 都需读相同 domain state，但现 Story 5.5 把 domain 全压在 HomeViewModel.homeData，跨 Tab 数据共享会产生 N 份冗余 / 漂移**

v2 直面这两个问题，引入 ADR-0009 / 0010 解决，再叠加 UI Scaffold 实装。

### 1.1 v1 被 Reject 的关键证据

- **BLOCKER 1**：rooms 表无 `code` 字段（实测 `docs/宠物互动App_数据库设计.md:641` rooms 仅 id/creator_user_id/status/max_members/timestamps），v1 「房间代码格式与 db 一致」是事实错误
- **BLOCKER 2**：ADR-0002 §3.1 钦定不引 SnapshotTesting/ViewInspector，v1 大量违背
- **BLOCKER 3**：Story 12.7 假设 Sheet 模式，v1 没列入修订；Story 24.1 也是 Sheet 模式
- **BLOCKER 4**：3 份状态源（AppState / HomeViewModel.homeData / RoomViewModel）；roomId vs roomCode 契约冲突
- **BLOCKER 5**：21.1 / 30.3 重叠强度判断错误
- **BLOCKER 6**：scope 应是 Major
- **BLOCKER 7**：Risk Register 漏 4 条
- **WARN**：Profile 微信绑定静默删；Icons 映射太虚

---

## 2. Impact Analysis

### 2.1 Epic 影响

| Epic | 影响 | 处理 |
|---|---|---|
| Epic 1-5（已 done） | Story 2.3 + Story 5.5 部分推翻（git 保留，sprint-status.yaml 不改） | partial revert via ADR-0009 / 0010 |
| **Epic 37（新增）** | 14 条 story，含 2 ADR + 架构重构 + 5 屏 Scaffold + Modal + 测试合规 + 治理 | 详见 §4 |
| Epic 6 节点 2 demo 验收 | 启动条件：Epic 37 done；验收基线变更（4 Tab + AppState）| description 加前置依赖 |
| Epic 12 房间页 + WS | Story 12.1 / 12.7 改写 | 详见 §5 |
| Epic 21 首页宝箱 | 21.1 acceptance 加前向声明（在 HomeView "宝箱位"槽位叠加 ChestCardView） | 详见 §5 |
| Epic 24 仓库页 | 24.1 改写（Sheet → Wardrobe Tab）；24.4 整条作废（Tab 直接路由不需要"主界面入口"）| 详见 §5 |
| Epic 27 穿戴 | 27.1 不变（仍是 WardrobeView 内的按钮激活） | 不动 |
| Epic 30 装扮渲染 | 30.3 不变（升级 EquippedCosmeticView 上身渲染，不碰仓库 grid） | 不动 |
| Epic 33 合成 | 33.1 改写（Sheet → Wardrobe Tab 内 push）；33.6「主界面入口完善」整条作废 | 详见 §5 |
| Epic 35 分享链接 | 35.2-35.4 改写：链接解析后调 JoinRoomUseCase 写 AppState.currentRoomId（不依赖 roomCode）| 详见 §5 |
| Epic 18 表情面板 / 33.x 合成（页面骨架部分） | `ui_design` 无设计稿；Epic 37 不覆盖；保持原计划 | 不动 |

### 2.2 Artifact 冲突

| Artifact | 冲突 / 修订 |
|---|---|
| **PRD §4 必做** | 不冲突（Scaffold 不引入新功能） |
| **PRD §4 暂不做（微信绑定结构预留）** | 用户决议 #4=B：UI 视觉壳做（含微信绑定卡 + Modal），按钮只 toast；不实装真 OAuth；与 PRD「结构预留 + 不实现 UI」语义略有边界争议——v2 解释为「视觉占位也属于结构预留的一部分，行为不做」 |
| **PRD §6 节点顺序** | 节点顺序不破坏（Epic 37 是跨节点 UI 基础设施，不引入 server 或新 FR）；但用户可见 IA 提前到节点 2 完成后立即就位 → 在 PRD §4 加澄清段 |
| **PRD §9 关联文档** | 加 `iphone/ui_design/README.md` |
| **ADR-0002 §3.1** | 严守，不推翻；v2 不引 SnapshotTesting/ViewInspector |
| **ADR-0002 §3.3** | 不影响（目录方案不变） |
| **Story 2.3 钦定 NavigationStack + Sheet 主入口** | **partial revert via ADR-0009**：主入口 Sheet 路由废，TabView 取代；NavigationStack 模板 + Sheet 次级场景保留 |
| **Story 5.5 钦定 LoadHomeUseCase → HomeViewModel.homeData** | **partial revert via ADR-0010**：LoadHomeUseCase 不变；hydrate 目标改 AppState；HomeViewModel.homeData 字段废 |
| **数据库设计 rooms 表** | 不改（仍无 code 字段）；roomCode 是前端展示别名（用户决议 #2=C） |
| **CLAUDE.md** | 不变（CLAUDE.md 不锁 iOS 导航 / 状态架构） |
| **iphone/README.md** | 加「导航架构」段引用 ADR-0009 + 「全局状态」段引用 ADR-0010 |
| **sprint-status.yaml** | 加 epic-37 + 14 story（详见 §4 提案 ④） |

### 2.3 技术影响

| 维度 | 影响 |
|---|---|
| 新增代码 | ~25-30 个 Swift 文件（AppState + 5 个 ViewModel 基类 + 5 屏 View + 5 个 MockViewModel 子类 + 6 primitives + Theme + MainTabView + HomeContainerView + Modal + a11y identifier 总表 + 对应测试） |
| 重构代码 | RootView .ready 分支 / AppCoordinator / HomeView / HomeViewModel partial revert |
| 删除代码 | HomeView 旧 3 CTA 按钮、AppCoordinator 主入口 sheet 路由（.room/.wardrobe case）、HomeViewModel.homeData 字段 |
| 测试 | AppStateTests / 5 个 ScaffoldViewModelTests / 改写 LoadHomeUseCase 集成测试 |
| 构建 | 沿用 `bash iphone/scripts/build.sh --test` |
| CI | 无新依赖；不引第三方测试库 |

---

## 3. Recommended Approach

### 3.1 选定路径

**Major Scope · Direct Adjustment + 2 新 ADR + Epic 37（14 story）+ 7 处下游 story 修订**

依赖顺序：

```
Story 37.1（ADR-0009）+ Story 37.2（ADR-0010）  [先决策]
   ↓
Story 37.3（RootView/TabView 改造）+ Story 37.4（AppState 实装）  [架构重构]
   ↓
Story 37.5（Theme）+ Story 37.6（primitives）  [UI 基础]
   ↓
Story 37.7-37.12（5 屏 + Modal Scaffold）  [UI Scaffold 主体]
   ↓
Story 37.13（accessibility 总表）+ Story 37.14（design-package 白名单）  [收尾]
```

### 3.2 选定理由

1. **直面架构问题，不绕路**：v1 想用 Moderate scope 包装架构级矛盾，被 codex 命中；v2 升 Major 直接立 ADR
2. **遵守用户前置 6 决议**：1=A 推翻 Story 2.3、2=C UI 用 roomId、3=A 严守 §3.1、4=B 微信绑定视觉、5=B AppState、6=B ScaffoldViewModel
3. **保持节点 MVP 顺序**：Epic 37 不引入新 FR / 不动 PRD §4 必做表 / 不动 §6 节点顺序；只是把 UI 工作集中度调整 + 架构重构
4. **测试合规**：严守 ADR-0002 §3.1，不引第三方测试库
5. **接缝设计**：每屏 ViewModel 基类（class 层次结构）让下游 Epic 12+ 接入时 View 不动
6. **风险登记完整**：补齐 codex 审 v1 命中的 4 条遗漏风险

### 3.3 Effort & Risk 估算

| 项 | v1 | v2 |
|---|---|---|
| Effort | M（10 story × 半屏 SwiftUI） | **L**（14 story 含 2 ADR + 2 重构 + 5 Scaffold + 测试合规） |
| Risk | L | **M**（推翻 2 条 done story 核心架构；roomCode 美化展示 vs roomId 真实参数边界需明确） |
| Timeline | epic-6 延后 2-3 cycle | epic-6 延后 4-6 cycle；MVP 总进度延长 ~1 周；但产品形态对齐 ui_design + 架构债务一次清 |

---

## 4. Detailed Change Proposals

### 提案 ① · 新增 ADR-0009 + ADR-0010

两个 ADR 已起草并落盘：

- `_bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md`
- `_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md`

Status 当前 = **Proposed**；用户终审通过本提案 + Story 37.1 / 37.2 落地后改 Accepted。

### 提案 ② · 新增 Epic 37（追加在 epics.md 末尾，Epic 36 之后）

**完整 Epic 37 详情见附录 A**（本文档 §7）。Story 列表：

| Story | 标题 | 性质 |
|---|---|---|
| 37.1 | ADR-0009 撰写（iPhone 导航架构 TabView） | spike + ADR |
| 37.2 | ADR-0010 撰写（全局 AppState 单 source of truth） | spike + ADR |
| 37.3 | RootView/MainTabView 改造 + AppCoordinator 缩窄 | 架构重构 |
| 37.4 | AppState 实装 + LoadHomeUseCase hydrate 迁移 + HomeViewModel.homeData 废 | 架构重构 |
| 37.5 | Theme & Design Tokens（candy 完整 + 三主题 stub） | UI 基础 |
| 37.6 | 共享 primitives（Card / PrimaryButton / Avatar / FadeIn / RarityTag / Icons 完整集） | UI 基础 |
| 37.7 | HomeView Scaffold + HomeViewModel class 层次 + Mock/Real 两实现 | Scaffold |
| 37.8 | RoomView Scaffold + RoomViewModel class 层次 + MockRoom | Scaffold |
| 37.9 | WardrobeView Scaffold + WardrobeViewModel class 层次 + MockWardrobe | Scaffold |
| 37.10 | FriendsView Scaffold + FriendsViewModel class 层次 + MockFriends | Scaffold |
| 37.11 | ProfileView Scaffold + ProfileViewModel class 层次（含微信绑定卡 + Modal 视觉，行为 toast） | Scaffold |
| 37.12 | JoinRoomModal + 跨屏 join 链路（roomId 字符串，UI 直白显示，无美化别名）| Scaffold |
| 37.13 | accessibility identifier 全屏总表 + 视觉回归 review checklist（替代 SnapshotTesting） | 测试合规 |
| 37.14 | design-package 白名单文档（声明哪些 ui_design 元素本期不做） | 治理 |

### 提案 ③ · PRD 增补两条澄清

`prd.md` §4 必做范围表后追加：

```markdown
**4 Tab 信息架构（澄清，依据 iphone/ui_design/README.md + ADR-0009）**：

App 底部 4 Tab：家（Home）/ 仓库（Wardrobe）/ 好友（Friends）/ 我的（Profile）。
- 家 Tab 内 idle ⟷ inRoom 两态互斥（已加入队伍时 RoomScreen 完全替换 HomeScreen，Tab 仍叫"家"且不变图标）
- 好友 Tab 仅做"在线状态展示 + 邀请/加入按钮"基础形态；§4「暂不做」内的"复杂好友系统"（关注/拉黑/聊天）继续不做
- 我的 Tab 仅做信息展示 + 设置入口；微信绑定 UI 视觉壳本期做（按钮 toast，真 OAuth 留给后续 epic），不突破 §4「微信绑定结构预留 + 不实现 UI」边界（视觉占位属结构预留的一部分）
- 房间标识全程使用 **roomId 字符串**（按 AR21 ID 字符串约定；db rooms 表 PK 是 BIGINT auto-increment，server 在 response 中以字符串形式返回；依据 docs/宠物互动App_数据库设计.md §5.13 rooms 表无 code 字段）。UI 全屏直接显示 **roomId 字符串**（如「房间 1234567」，文案模板就是 `"房间 \(appState.currentRoomId ?? "")"`，**不**预设 roomId 一定是数字内容），分享文案直白展示同样写法；本期 MVP **不引入** K3M9P2 等美化别名（避免 sender/receiver UX 闭环问题；未来若产品需要可在专门 spike 设计可逆 base36 + checksum 契约）
```

`prd.md` §9 关联文档加：

```markdown
- `iphone/ui_design/README.md`（iPhone 端高保真原型与 Design Tokens；Epic 37 实装依据）
- `_bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md`（iPhone 导航架构 ADR）
- `_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md`（全局 AppState ADR）
```

### 提案 ④ · sprint-status.yaml 增补

在 `epic-5-retrospective: optional` 后、`epic-6: backlog` 前**物理插入**：

```yaml
  epic-37: backlog
  37-1-adr-0009-导航架构: backlog
  37-2-adr-0010-appstate: backlog
  37-3-rootview-maintabview-改造: backlog
  37-4-appstate-实装-loadhome-迁移: backlog
  37-5-theme-design-tokens: backlog
  37-6-shared-primitives: backlog
  37-7-homeview-scaffold: backlog
  37-8-roomview-scaffold: backlog
  37-9-wardrobeview-scaffold: backlog
  37-10-friendsview-scaffold: backlog
  37-11-profileview-scaffold: backlog
  37-12-joinroommodal-跨屏跳转: backlog
  37-13-accessibility-identifier-总表: backlog
  37-14-design-package-白名单: backlog
  epic-37-retrospective: required
```

> 注：`epic-37-retrospective: required`（不是 optional）—— 含 2 ADR + 重构 partial revert，需要 retro 总结教训。

### 提案 ⑤ · Epic 6 description 加前置声明

`epics.md` Epic 6（节点 2 demo 验收）章节首行后追加：

```markdown
> **前置依赖**：Epic 37（iPhone 架构层重构 + UI Scaffold）需先 done。本 epic 启动时主入口已是 4 Tab + Home Tab 互斥（ADR-0009）+ AppState 单 source of truth（ADR-0010）。demo 验收基线变更：从「3 CTA + Sheet」改为「4 Tab + AppState 数据流 + ping/version + 自动登录」。
```

### 提案 ⑥ · 7 处下游 Story 修订

修订后的完整 acceptance 见 §5；此处仅 summary。

| Story | 修订要点 |
|---|---|
| **12.1** 房间页骨架 | Given 加「Story 37.8 RoomView Scaffold 已交付（含房间代码卡 / MiniCat 弹跳 / 成员列表 4 格 / 离开按钮）+ Story 37.4 AppState.currentRoomId 已就绪」；范围改为「在 RoomView Scaffold 上把 MockRoomViewModel 替换为 RealRoomViewModel（持 WSState + members + memberPetStates），不再持 roomId（来自 AppState）」 |
| **12.7** 创建/加入/退出 + 主界面入口完善 | Given 加「Story 37.3 主入口已是 TabView + Story 37.7/37.8 Home/Room 互斥状态机已就绪 + Story 37.12 JoinRoomModal 已就绪 + Story 37.4 AppState 已就绪」；删除「主界面进入房间按钮 → Sheet 弹层」入口设计；改为：① CreateRoomUseCase 由 HomeView idle 态 TeamIdleCard"创建队伍"按钮触发，成功后写 `appState.currentRoomId: String?` → HomeContainerView 自动切 RoomView；② JoinRoomUseCase 由 JoinRoomModal "确定加入"或 FriendsView "加入"按钮触发，**参数 `roomId: String`**（AR21 ID 字符串约定，不预设数字内容）；③ LeaveRoomUseCase 由 RoomView "离开房间"按钮触发，写 `appState.currentRoomId = nil`。删除「返回房间 #xxxx」按钮文案逻辑（HomeContainerView 互斥状态机自动接管）|
| **21.1** 首页宝箱组件 | Given 加「Story 37.7 HomeView 已交付（含 idle 态 StatusBar / CatStage / ActionRow / TeamIdleCard 槽位）+ Story 37.4 AppState.currentChest 已就绪」；范围改为「在 HomeView idle 态布局中插入 ChestCardView；HomeViewModel 仅持 chestRemainingSeconds（Timer 驱动），currentChest 来自 AppState」 |
| **24.1** 仓库页骨架 | Given 加「Story 37.9 WardrobeView Scaffold 已交付（含分类 Tab / 3 列网格 / 预览区 / mock Inventory）+ Story 37.4 AppState.currentInventory 字段就绪」；范围改为「把 WardrobeView Scaffold 内的 MockWardrobeViewModel 替换为 RealWardrobeViewModel；inventory 来自 AppState（hydrate 来自 LoadInventoryUseCase）；selectedCategory / selectedCosmeticId 仍为 view-specific」；删除「Sheet 弹 InventoryView」语义（Tab 直接路由） |
| **24.4** 主界面"仓库"按钮入口完善 | **整条作废**（Tab 直接路由不需要"主界面入口"按钮）。sprint-status.yaml 内 24-4 标 deleted；epics.md 内 24.4 章节加 `> **2026-04-29 变更**：本 story 因 ADR-0009 推翻 Sheet 主入口模式而作废；功能由 Story 37.3 MainTabView 的 Wardrobe Tab 直接承担。` |
| **27.1** 激活穿戴按钮 | 不变（仍是 WardrobeView 内装备/卸下按钮的回调改 EquipUseCase / UnequipUseCase；EquipUseCase 成功后写 `appState.currentEquips`，HomeView CatStage 自动刷新） |
| **30.3** 升级 EquippedCosmeticView | 不变（仍是猫身上的 sprite 渲染升级；不碰 WardrobeView grid 图标） |
| **33.1** 合成页骨架 | Given 加「Story 37.3 MainTabView + Wardrobe Tab 内 NavigationStack 已就绪」；范围改为「合成页通过 Wardrobe Tab 内 NavigationLink push 进入（不再是主界面 Sheet 弹）；其它 acceptance 不变」 |
| **33.6** 主界面"合成"按钮入口完善 | **整条作废**（Tab 内 push 模式不需要"主界面入口"按钮）。sprint-status.yaml 内 33-6 标 deleted；epics.md 内 33.6 章节加变更注 |
| **35.2** 房间页内"分享"按钮 + 链接生成 | 不变结构；链接 payload `catapp://room/{roomId}` 用 roomId 字符串（AR21）；分享文案直白「邀请你加入小猫房间 1234567」，不引入美化别名（v2.1 修订：避免 sender/receiver UX 不闭环） |
| **35.3** 链接解析处理 | 不变结构；解析出 `roomId: String` 传给 35.4 |
| **35.4** 已登录时自动 join 跳转房间页 | 不变结构；but 调 JoinRoomUseCase(roomId) 成功后写 `appState.currentRoomId` → HomeContainerView 自动切 RoomView（不需要"跳转到房间页"，因为 Home Tab 互斥状态机自动接管） |

### 提案 ⑦ · 文件修改清单

| 文件 | 操作 |
|---|---|
| `_bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md` | 已新建（Status: Proposed） |
| `_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md` | 已新建（Status: Proposed） |
| `_bmad-output/planning-artifacts/sprint-change-proposal-2026-04-29-v2.md` | 已新建（本文档） |
| `_bmad-output/planning-artifacts/sprint-change-proposal-2026-04-29.md` | 保留 v1（决策历史；codex Reject 痕迹保留） |
| `_bmad-output/planning-artifacts/epics.md` | +约 800 行（Epic 37 详细 + 9 处下游 acceptance 修订 / 作废注 + Epic 6 前置声明） |
| `_bmad-output/planning-artifacts/prd.md` | +12 行（§4 4 Tab IA 澄清 + §9 关联文档） |
| `_bmad-output/implementation-artifacts/sprint-status.yaml` | +16 行（epic-37 + 14 story + retrospective）；24-4 / 33-6 标 deleted |
| `iphone/README.md` | +约 8 行（导航架构段引用 ADR-0009 + 全局状态段引用 ADR-0010） |

---

## 5. 下游 Story 修订完整 acceptance（详细）

> 本节包含本提案落地时**逐字落到 epics.md** 的下游 story 改写内容。

### 5.1 Story 12.1 改写

```markdown
### Story 12.1: 房间页面 SwiftUI 骨架 ⟶ 注入真实 RoomViewModel

> **2026-04-29 变更**（v2 提案落地）：原范围「房间页 SwiftUI 骨架」由 Story 37.8 完成；本 story 缩窄为「在 RoomView Scaffold 上注入 RealRoomViewModel」。

As an iOS 开发,
I want 把 Story 37.8 RoomView Scaffold 上的 MockRoomViewModel 替换为 RealRoomViewModel（持 WSState + members + memberPetStates）,
So that 房间页连上 WS 后能显示真实成员列表 + 连接状态.

**Acceptance Criteria:**

**Given** Story 37.8 RoomView Scaffold 已交付（含 RoomViewModel 基类与 MockRoomViewModel 子类、房间代码卡、MiniCat 弹跳、成员列表 4 格、离开按钮）+ Story 37.4 AppState.currentRoomId 字段就绪 + Story 12.2 WebSocketClient 就绪
**When** 完成本 story
**Then** 实装 `RealRoomViewModel: RoomViewModel`（继承 Story 37.8 落地的 RoomViewModel 基类）:
- 持 `members: [Member]`（来自 WS room.snapshot）
- 持 `wsState: WSState`（connected / reconnecting / disconnected）
- 持 `memberPetStates: [UserId: PetState]`（节点 5 后启用）
- `roomId` getter 来自 `appState.currentRoomId`（不持本地副本）
**And** RoomView 注入 RealRoomViewModel 替换 Story 37.8 的 MockRoomViewModel
**And** RoomView「房间代码」位置直接显示 roomId 字符串（如「房间 1234567」），accessibility identifier `roomIdDisplay`（**v2.2 修订**：a11y 命名统一为 `roomIdDisplay`，全文不再使用 `roomCodeDisplay`；codex v2.1 BLOCKER 10 修复）
**And** 离开房间按钮调 LeaveRoomUseCase（Story 12.7）→ 成功后 appState.currentRoomId = nil
**And** **单元测试覆盖**（≥4 case，mocked WebSocketClient + MockAppState）:
- happy: `appState.currentRoomId = "room_1234567"` + `RealRoomViewModel.roomId == "room_1234567"`（roomId 类型 String，AR21）
- happy: WS 推 room.snapshot 含 3 成员 → ViewModel.members 长度 = 3
- happy: WS 状态切到 reconnecting → ViewModel.wsState 更新
- edge: snapshot 解码失败 → ViewModel 不破坏现有 members + log error
**And** **UI 测试覆盖**: `appState.currentRoomId = "room_1234567"` + WS mock 推 3 成员 → RoomView 渲染 → 验证 3 个成员位 accessibility identifier `roomMember_0/1/2` 可定位 + 房间号 label accessibility identifier **`roomIdDisplay`**（v2.2 a11y 命名统一）显示非空字符串
```

### 5.2 Story 12.7 改写

```markdown
### Story 12.7: 创建 / 加入 / 退出 use case ⟶ 接 TabView Home Tab 互斥状态机

> **2026-04-29 变更**（v2 提案落地）：原范围「主界面入口完善（Sheet 模式）」整段作废，由 Story 37.3 MainTabView + Story 37.7 HomeView TeamIdleCard + Story 37.12 JoinRoomModal 承担入口；本 story 缩窄为「3 个 UseCase 实装 + 写 AppState」。

As an iPhone 用户,
I want 我可以从 Home Tab 创建/加入房间，从 Room 视图退出房间，跨 Tab 数据自动同步,
So that 节点 4 业务链路对用户完整闭合.

**Acceptance Criteria:**

**Given** Story 37.3 MainTabView + Story 37.7 HomeView TeamIdleCard + Story 37.12 JoinRoomModal + Story 37.4 AppState 就绪 + Story 11.3-11.5 server 端房间 CRUD 接口可用
**When** 完成本 story
**Then** 实装三个 UseCase:
- `CreateRoomUseCase`: 调 POST /rooms → 拿到 `roomId: String`（AR21）→ `await appState.setCurrentRoomId(roomId)` → HomeContainerView 自动切到 RoomView（互斥状态机）
- `JoinRoomUseCase(roomId: String)`: 调 POST /rooms/{roomId}/join → 同上
- `LeaveRoomUseCase`: 调 POST /rooms/{currentRoomId}/leave → `await appState.setCurrentRoomId(nil)` → HomeContainerView 自动切回 HomeView
**And** HomeView TeamIdleCard "创建队伍" 按钮触发 CreateRoomUseCase
**And** HomeView TeamIdleCard "加入队伍" 按钮触发 JoinRoomModal（Story 37.12）；Modal 内输入 `roomId: String`（前端仅做 trim + length 约束，不强制纯数字校验，roomId 字符串格式由 server AR21 决定）→ 触发 JoinRoomUseCase
**And** FriendsView FriendRow "加入" 按钮（好友在房间中）→ 解析 friend.currentRoomId → 触发 JoinRoomUseCase
**And** RoomView "离开房间" 按钮触发 LeaveRoomUseCase
**And** **单元测试覆盖**（≥6 case，mocked APIClient + MockAppState）:
- happy: CreateRoomUseCase 成功 → appState.currentRoomId 写入
- happy: JoinRoomUseCase 成功 → appState.currentRoomId 写入
- edge: CreateRoomUseCase 返回 6003（用户已在房间）→ ErrorPresenter 弹 alert + appState 不变
- edge: JoinRoomUseCase 返回 6002（房间满）→ alert
- edge: JoinRoomUseCase 返回 6001（房间不存在）→ alert
- happy: LeaveRoomUseCase → appState.currentRoomId = nil
**And** **UI 测试覆盖**（XCUITest）: 启动 → Home Tab idle → 点 "创建队伍" → 等待 1s → 验证 RoomView 出现（accessibility identifier `roomView` 可定位）+ Tab Bar 仍可见 + 当前 Tab 仍是 Home（accessibility selected = "tab_home"）
```

### 5.3 Story 21.1 改写

```markdown
### Story 21.1: 首页宝箱组件 SwiftUI（倒计时 Timer + 状态切换 UI）

> **2026-04-29 变更**（v2 提案落地）：Given 段加 Story 37.7 + 37.4 前置；HomeViewModel 字段调整。

[原 acceptance 保留，仅修改 Given 段:]

**Given** Story 37.7 HomeView Scaffold 已交付（含 idle 态 StatusBar / CatStage / ActionRow / TeamIdleCard 4 槽位 + 宝箱位预留槽）+ Story 37.4 AppState.currentChest 字段就绪 + Story 20.5 server GET /chest/current 可用
**When** 完成本 story
**Then** 实装 `ChestCardView` SwiftUI 组件，**插入 HomeView idle 态布局的「宝箱位」槽位**:
[...原 ChestCardView 状态 / 视觉描述不变...]
**And** ViewModel 调整：`HomeViewModel` 持 `chestRemainingSeconds: Int`（Timer 驱动）；`currentChest` 来自**构造注入的** `appState: AppState`（HomeViewModel `init(appState:)` 模式；不允许 ViewModel 内 @EnvironmentObject，详见 ADR-0010 §3.1 注入规则）；不再持本地 chestState 字段
**And** 倒计时归零时**自动切到 unlockable 视觉状态**（基于 `appState.currentChest.unlockAt` vs now 计算，与 server 端校准 by Story 21.2）
[...其它 AC 不变...]
```

### 5.4 Story 24.1 改写

```markdown
### Story 24.1: 仓库页 SwiftUI 骨架 ⟶ 注入真实 InventoryViewModel

> **2026-04-29 变更**（v2 提案落地）：原范围「Sheet 弹 InventoryView」由 Story 37.9 WardrobeView Scaffold + Story 37.3 MainTabView 直接承担；本 story 缩窄为「替换 Mock → Real」。

As an iPhone 用户,
I want Wardrobe Tab 打开时立即显示我的最新装扮列表,
So that 我看到的是真实数据.

**Acceptance Criteria:**

**Given** Story 37.9 WardrobeView Scaffold 已交付（含分类 Tab / 3 列网格 / 预览区 / WardrobeViewModel 基类 / MockWardrobeViewModel 子类）+ Story 37.4 AppState.currentInventory 字段就绪 + Story 23.4 server GET /cosmetics/inventory 可用
**When** 完成本 story
**Then** 实装 `RealWardrobeViewModel: WardrobeViewModel`（继承 Story 37.9 落地的 WardrobeViewModel 基类）:
- `inventory` getter 来自 `appState.currentInventory`（不持本地副本）
- `selectedCategory: Category`（默认 hat）/ `selectedCosmeticId: String?`（@Published view-specific transient）
- `groups: [InventoryGroup]` computed 属性按 selectedCategory + AppState.currentInventory 实时计算
**And** WardrobeView 注入 RealWardrobeViewModel 替换 Story 37.9 的 MockWardrobeViewModel
**And** Wardrobe Tab 首次出现时自动调 LoadInventoryUseCase（Story 24.2）刷 AppState
**And** **单元测试覆盖**（≥4 case，MockAppState）:
- happy: appState.currentInventory 含 5 个 hat → groups[hat] 长度 = 5
- happy: 切换 selectedCategory 到 bow → groups[bow] 渲染
- happy: appState.currentInventory 为空 → 显示空仓库 placeholder（Story 37.9 已实装空态）
- edge: 一个 cosmetic count > 99 → 显示 "99+"

> **Story 24.4「主界面"仓库"按钮入口完善」整条作废**（ADR-0009 推翻 Sheet 主入口模式；功能由 Story 37.3 MainTabView Wardrobe Tab 直接承担）
```

### 5.5 Story 33.1 改写

```markdown
### Story 33.1: 合成页 SwiftUI 骨架 ⟶ Wardrobe Tab 内 push

> **2026-04-29 变更**（v2 提案落地）：原范围「主界面 Sheet 弹合成页」改为「Wardrobe Tab 内 NavigationLink push」。

[Acceptance 中将「主界面"合成"按钮 + Sheet」全部改为「WardrobeView 顶部"合成"小按钮 + NavigationLink push」；其它 AC 不变]

> **Story 33.6「主界面"合成"按钮入口完善」整条作废**（ADR-0009 推翻 Sheet 主入口模式；合成入口改为 Wardrobe Tab 内 push）
```

### 5.6 Story 35.2 / 35.3 / 35.4 改写

```markdown
### Story 35.2: 房间页内"分享"按钮 + 链接生成
[原结构不变]
**Then** 链接 payload 格式 `catapp://room/{roomId}` 用 **roomId 字符串**（AR21 ID 字符串约定）
**And** 分享文案直白「邀请你加入小猫房间 {roomId}」（如「邀请你加入小猫房间 1234567」）；本期不引入美化别名（避免 sender/receiver UX 不闭环；未来可在专门 spike 设计可逆契约）

### Story 35.3: 链接解析处理
[原结构不变]
**Then** 解析出 `roomId: String` 传给 Story 35.4

### Story 35.4: 已登录时自动 join 跳转房间页
[原结构不变]
**Then** 调 JoinRoomUseCase(roomId: String) → 成功后 `appState.currentRoomId = roomId` → **HomeContainerView 自动切 RoomView**（互斥状态机；不需要"跳转到房间页"，因为 Home Tab 互斥替换自动接管）
**And** AppCoordinator.switchTab(.home)（如果用户当前在其它 Tab）确保用户看到 RoomView
```

---

## 6. Risk Register（10 条，覆盖 v1 的 6 条 + codex 命中的 4 条新增）

| # | 风险 | 等级 | 缓解 |
|---|---|---|---|
| 1 | Story 2.3 partial revert：HomeView 旧 3 CTA 按钮 + AppCoordinator 主入口 sheet 路由删除时漏掉某处接线 | M | Story 37.3 AC 明列「accessibility identifier 不丢失保证」；删除前先把所有 caller / UITest 列单 |
| 2 | Story 5.5 partial revert：HomeViewModel.homeData 字段废 + LoadHomeUseCase hydrate 目标改 AppState；可能漏改某 view 的读取 | M | Story 37.4 AC 明列「grep 全 codebase HomeViewModel.homeData 引用，逐一改读 appState」；UITest 跑全套校验 |
| 3 | roomId 类型契约漂移 | **H** | v2.1 修订统一为 `String`（贴合 AR21 ID 字符串约定 + epics.md L2016 server /home `room.currentRoomId` 已是 String）；不再使用美化别名；ADR-0010 §3.2 字段类型字面量化 |
| 11 | currentTab 所有权冲突（v2 中 ADR-0009 / 0010 互斥） | M | v2.1 修订：currentTab 归 **AppCoordinator**（与 presentedSheet 同级），不进 AppState；两个 ADR 已同步 |
| 12 | ViewModel 用 @EnvironmentObject 反模式 | M | ADR-0010 §3.1 加 ADR 级硬规则：ViewModel 只允许构造注入 AppState；@EnvironmentObject 仅 View 层用；codex review 强制检查 |
| 13 | 14 story 依赖链上 37.7-37.12 6 条"可并行"声明实际共享 ViewModel 基类（HomeViewModel / RoomViewModel / etc.）冻结点未明 | M | 37.7 落地时同时定义全部 5 个 ViewModel 基类骨架（基类含 @Published 字段 + abstract method 签名，不实装 Real）；其余 37.8-37.11 在基类冻结后并行实装 Mock + View；变更基类必须 round-trip 改全部子类 |
| 14 | partial revert 对 git blame / commit history 可读性影响 | L | Story 37.3 / 37.4 commit message 显式 reference「partial revert ADR-0009 / ADR-0010 of Story 2.3 / 5.5」；保留旧 view 删除前先 grep 验证无引用 |
| 4 | TabView 浮动 TabBar 自绘与 SwiftUI 默认行为冲突（如键盘弹出时 TabBar 遮挡） | M | Story 37.3 AC 加键盘 + safe area 测试 case；fallback 到 SwiftUI 默认 TabView 样式 |
| 5 | 三主题 + dark 模式 candy stub 在 dark mode preview 失真 | L | Story 37.5 AC 明确「dark/matcha/sky 主题在本期仅 stub，不强求视觉一致；Preview 加注释 TODO」 |
| 6 | 像素级还原 SVG 条纹猫工作量不可控 | L | 不强求 SVG；用 `Image(systemName: "cat.fill")` + 椭圆背景 + 简单条纹 overlay；ui_design/README 明说后期替换 |
| 7 | HomeView 重写 → 现 Story 2.5 ping/version 调用 + Story 5.5 LoadHome 触发可能丢失 | **H** | Story 37.3 / 37.7 AC 明列「保留 ping/version 调用 + LoadHome 触发不变；UITest 验证 mock server 仍收到 1 次 /ping + 1 次 /home」 |
| 8 | accessibility identifier 总表（37.13）覆盖不全 → UI 测试无法定位某些元素 | M | Story 37.13 强制全屏 grep 检查 + 模板要求 + 单文件 `AccessibilityID.swift` 集中管理（已存在该文件，扩展即可） |
| 9 | AppState 体积膨胀 / 跨 Feature 强耦合 | M | ADR-0010 §3.2 白名单严格限制；MR review 检查；未来按 Feature 拆 sub-state 作为 fallback |
| 10 | 微信绑定 UI 视觉壳 vs PRD「不实现 UI」边界争议 | L | PRD §4 澄清段加注「视觉占位属结构预留的一部分，行为不做」；Story 37.11 AC 明列「按钮触发后仅 toast」 |

---

## 7. 附录 A · Epic 37 完整 Story 详情

> 本节是 Epic 37 14 条 story 的逐条 acceptance 全文，落地时**整段抄入** `epics.md` 末尾（Epic 36 之后）。

### Epic 37: iPhone 架构层重构 + UI Scaffold（壳先行）

依据 `iphone/ui_design/README.md` 高保真原型 + ADR-0009（导航 TabView）+ ADR-0010（AppState 单 source of truth），把 5 屏 1 弹窗的视觉壳一次性在 `iphone/PetApp/` 下实装为可视的"壳"，并完成必要的架构层重构（推翻 Story 2.3 主入口 + Story 5.5 数据流的部分钦定）。所有 UI 数据 mock，所有交互仅在本地 `@State` / Mock ViewModel 内打转，**禁止任何 APIClient / Repository / UseCase 调用**（Story 37.3 / 37.4 架构重构 story 除外，它们处理基础设施迁移）。逻辑回填留给下游 Epic 12 / 21 / 24 / 27 / 30 / 33 / 35。

> **执行顺序**：本 Epic 编号 37，但实际是 sprint 中段插入——epic-5 done 后立即开始，epic-6（节点 2 demo）等本 Epic 全部 done 才启动。`sprint-status.yaml` 内本 Epic yaml block 物理位置在 `epic-5-retrospective` 后、`epic-6` 前。
>
> **Story 依赖链**：37.1 + 37.2（决策）→ 37.3 + 37.4（重构）→ 37.5 + 37.6（UI 基础）→ 37.7-37.12（Scaffold 主体）→ 37.13 + 37.14（收尾）。前两层每对内可并行；37.7-37.12 6 条可并行（仅依赖 37.5 + 37.6）。
>
> **核心 AC 红线**（所有 Scaffold story 共性，37.3/37.4 不适用）：
> - 数据：完全 mock（构造注入 MockAppState 或 PreviewProvider）
> - API：禁止 import APIClient / Repository / UseCase（Story 37.13 含静态校验）
> - 视觉：像素级匹配 `iphone/ui_design/README.md` §Design Tokens
> - 主题：用 `@Environment(\.theme)` 取 token
> - 测试：每 View 至少一个 SwiftUI Preview；关键交互覆盖 a11y identifier；**不引 SnapshotTesting / ViewInspector**（严守 ADR-0002 §3.1）
> - 通过 `bash iphone/scripts/build.sh --test`

#### Story 37.1: ADR-0009 撰写（iPhone 导航架构 TabView）

As an iOS 开发,
I want 一份 ADR 文档明确 iPhone 导航架构改为 TabView 4 Tab，且声明 Story 2.3 主入口部分作废,
So that Story 37.3 实装有契约依据，下游 Epic 12.7 / 24.1 / 33.1 / 35.x 入口改写有原文可引.

**Acceptance Criteria:**

**Given** Sprint Change Proposal v2 已用户终审通过 + ADR-0009 草稿已落盘
**When** 完成本 story
**Then** ADR-0009 status 从 Proposed 改为 Accepted
**And** 本 story 的 dev 评估 ADR-0009 各 §3 决策点是否仍站得住（Story 37.3 实装期间发现的偏差通过 ADR-0009 修订 patch 体现）
**And** ADR-0009 §6 「验收（本 ADR 改 Accepted 的标准）」3 条全部勾选
**And** **deliverable**：ADR-0009 文档 status 字段更新 commit
**And** spike / 配置 / 文档同步类 story，不强制单元测试

#### Story 37.2: ADR-0010 撰写（全局 AppState 单 source of truth）

As an iOS 开发,
I want 一份 ADR 文档明确引入全局 AppState + Story 5.5 数据流部分作废 + AppState 范围白名单,
So that Story 37.4 实装有契约依据，下游所有 ViewModel 改用 AppState 模式有原文可引.

**Acceptance Criteria:**

**Given** Sprint Change Proposal v2 已用户终审通过 + ADR-0010 草稿已落盘
**When** 完成本 story
**Then** ADR-0010 status 从 Proposed 改为 Accepted
**And** §3.2 AppState 范围白名单各字段 type 定义清晰可直接照写
**And** §3.5 ViewModel 演变模式表覆盖所有现有 + 计划中的 ViewModel
**And** §6 验收 5 条全部勾选
**And** **deliverable**：ADR-0010 文档 status 字段更新 commit + iphone/README.md 加「全局状态」段引用
**And** spike / 配置 / 文档同步类 story，不强制单元测试

#### Story 37.3: RootView/MainTabView 改造 + AppCoordinator 缩窄

As an iPhone 用户,
I want 主入口从 3 CTA + Sheet 改为 4 Tab 浮动 TabBar，每 Tab 独立 NavigationStack,
So that App IA 与 ui_design 完全对齐.

**Acceptance Criteria:**

**Given** Story 37.1 ADR-0009 Accepted
**When** 完成本 story
**Then** 按 ADR-0009 §3.5 步骤 1-8 改造:
- 新建 `iphone/PetApp/App/MainTabView.swift`：含 `TabView(selection: $tabSelection) { ... }` + 浮动自绘 TabBar overlay（按 ui_design §iOS 设备规格：高 72pt、距底 14pt、距左右 12pt、theme.shadow.md、Card 圆角）
- 新建 `iphone/PetApp/Features/Home/Views/HomeContainerView.swift`：根据 `appState.currentRoomId` 切换 HomeView ↔ RoomView（互斥状态机，淡入淡出 0.3s）
- RootView `.ready` 分支：从 `HomeView { ... }.fullScreenCover(...)` 改为 `MainTabView()`
- HomeView 删除旧 3 CTA 按钮（"进入房间"/"仓库"/"合成"）；保留 ping/version 角落显示 + .task 调用
- AppCoordinator.SheetType 删除 `.room` / `.wardrobe` case；保留 `.compose` 占位（Story 33.1 决定是否实装）
- JoinRoomModal 改用 `.sheet` 挂在 HomeView 内（Story 37.12 落地）
- launching / needsAuth 三态机保留不动
**And** 删除前先 grep 全 codebase 找所有 `coordinator.present(.room)` / `.wardrobe` / `SheetType.room` / `.wardrobe` 引用，确认无遗漏
**And** accessibility identifier 不丢失：`tab_home` / `tab_wardrobe` / `tab_friends` / `tab_profile` 4 个新加；旧 `homeButton_room` / `homeButton_wardrobe` / `homeButton_compose` 删除（无对应 UI 元素）
**And** **单元测试覆盖**（≥5 case）:
- happy: appState.currentRoomId = nil → HomeContainerView 显示 HomeView
- happy: `appState.currentRoomId = "room_1234567"`（roomId 类型 String，AR21）→ HomeContainerView 显示 RoomView
- happy: 切换 Tab → 内容更换 + 当前 Tab 高亮
- happy: launching → MainTabView 不显示
- edge: appState.currentRoomId 从 1234 切到 nil → HomeContainerView 切回 HomeView，过渡动画触发
**And** **UI 测试覆盖**（XCUITest）:
- 启动 → 验证 4 Tab 可定位（accessibility identifier `tab_home` / `tab_wardrobe` / `tab_friends` / `tab_profile`）
- 切到 Wardrobe Tab 验证 WardrobeView 出现（accessibility identifier `wardrobeView`）
- ping/version 角落显示仍在 Home Tab 可见

#### Story 37.4: AppState 实装 + LoadHomeUseCase hydrate 迁移 + HomeViewModel.homeData 废

As an iOS 开发,
I want 引入全局 AppState 持有所有 domain state，把 Story 5.5 LoadHomeUseCase 的 hydrate 目标从 HomeViewModel.homeData 改为 AppState,
So that 4 Tab 数据共享有单一权威来源，跨 Tab 联动天然支持.

**Acceptance Criteria:**

**Given** Story 37.2 ADR-0010 Accepted
**When** 完成本 story
**Then** 按 ADR-0010 §3 落地:
- 新建 `iphone/PetApp/App/AppState.swift`（按 §3.2 白名单字段 + `@MainActor final class ObservableObject` + `@Published` 各字段）
- 新建 `iphone/PetApp/PetAppTests/Helpers/AppStateTestHelpers.swift`（MockAppState builder + reset helper）
- RootView 加 `@StateObject var appState = AppState()` + `.environmentObject(appState)` 注入子树
- LoadHomeUseCase 调用方（HomeViewModel.bind 内）改：从 `self.homeData = homeData` 改为 `self.appState.hydrate(homeData)`；HomeViewModel 持 `appState` 引用（**构造注入**——`init(appState: AppState, pingUseCase: PingUseCase, loadHomeUseCase: LoadHomeUseCase)`；不允许 @EnvironmentObject，详见 ADR-0010 §3.1 注入规则）
- 删除 HomeViewModel.homeData 字段
- ResetIdentityViewModel.resetTapped 加 `appState.reset()`
- 全 codebase grep `homeViewModel.homeData` / `HomeViewModel.homeData` 引用，逐一改读 `appState.currentUser` / `currentPet` / 等
**And** **单元测试覆盖**（AppStateTests，≥6 case）:
- happy: hydrate(homeData) → currentUser / currentPet / currentStepAccount / currentChest / currentRoomId 全部就绪
- happy: reset() → 全字段 nil/empty
- happy: `setCurrentRoomId("room_1234567")` → `currentRoomId == "room_1234567"`（String 契约）
- happy: updateCurrentEquips(.empty) → currentEquips 重置
- happy: updateMyPetState(.walk) → currentPet.currentState 更新
- edge: hydrate 之前读字段 → 全是 nil（不崩）
**And** **改写现有 LoadHomeUseCase 集成测试**：断言从 `homeViewModel.homeData != nil` 改为 `appState.currentUser != nil`
**And** **UI 测试覆盖**：启动 → 验证 mock server 收到 1 次 /home → 主界面 CatStage 显示 mock pet 名 + StatusBar 显示 mock 步数（数据从 appState 投影）

#### Story 37.5: Theme & Design Tokens（candy 完整 + 三主题 stub）

[与 v1 Story 37.1 相同 acceptance，原文略；落地时按 v1 起草版抄入]

#### Story 37.6: 共享 primitives（Card / PrimaryButton / Avatar / FadeIn / RarityTag / Icons 完整集）

[与 v1 Story 37.2 类似 primitives 实装，加上以下 Icons 完整对照表（v2.1 WARN 8 修复）：]

**Icons 完整映射表**（落地为 `iphone/PetApp/Core/DesignSystem/Primitives/Icons.swift` 内 `static let mapping: [String: String]`；本表 **1:1 对齐** `iphone/ui_design/source/components/primitives.jsx` 内 `Icons` 对象全部 25 个键，键名严格保持原型驼峰写法；本期 Scaffold 阶段不在表内的视觉元素（如 Wardrobe 锁定遮罩、Modal 装饰）用 SwiftUI 原生组件 + theme token 直接画，不放进 Icons 集合）：

| ui_design 键（驼峰） | SF Symbol | 用途定位 |
|---|---|---|
| home | house.fill | TabBar 家 |
| box | shippingbox.fill | 宝箱、TabBar 仓库（替 box） |
| friends | person.2.fill | TabBar 好友 |
| user | person.crop.circle.fill | TabBar 我的、Profile 头像 |
| paw | pawprint.fill | JoinRoomModal 输入框 prefix |
| bowl | bowl.fill | 喂食按钮（FeedButton） |
| heart | heart.fill | 抚摸按钮（PetButton；filled 变体在原型用 `heart(filled=true)`，SF Symbol 区分 heart vs heart.fill） |
| ball | circle.dotted | 玩耍按钮（PlayButton） |
| footprint | figure.walk | StatusBar 步数计 prefix |
| plus | plus.circle.fill | FriendsView 添加好友按钮 |
| enter | arrow.right.circle.fill | 创建/加入队伍 CTA、FriendRow 加入按钮 |
| close | xmark.circle.fill | Modal 关闭按钮 |
| back | chevron.left | 导航返回（Tab 内 NavigationStack push 自动给）|
| dot | circle.fill | 在线小绿点、状态指示 |
| copy | doc.on.doc.fill | 房间代码复制按钮 |
| check | checkmark.circle.fill | 已装备 / 复制成功 |
| settings | gearshape.fill | Profile 设置入口 |
| sparkle | sparkles | 装扮稀有度装饰 / Profile 称号装饰 |
| bell | bell.fill | Profile 顶部消息（视觉占位，本期不做行为）|
| chevronRight | chevron.right | Profile 菜单项 / 横向滚动指示 |
| wechat | message.fill | Profile 微信绑定（视觉占位，按钮 toast）|
| shield | shield.fill | 微信绑定 Modal 数据保护图标 |
| warn | exclamationmark.triangle.fill | 微信绑定 Modal 警告图标 |
| diamond | diamond.fill | Wardrobe 钻石货币 |
| trophy | trophy.fill | Profile 成就统计 |

**Acceptance Criteria 补充**：
- 落地为 `enum Icons { static let mapping: [String: String] = ["home": "house.fill", ...] }` 全 25 键完整列出
- 测试 case 加：`Icons.mapping["home"] == "house.fill"` + `Icons.mapping.count == 25`（≥3 case 抽样验证）+ 全 25 键 forEach `XCTAssertNotNil(UIImage(systemName: ...))` 验证 iOS 17+ 都存在
- 未在表内的键名查询 → log warning + 退回 `questionmark.circle`；不允许 silent fallback
- 视觉差异容忍：本表 `ball → circle.dotted` / `bowl → bowl.fill` / `paw → pawprint.fill` 等是 SF Symbol 视觉**近似**而非像素一致；视觉精度由 Story 37.13 visual-review-checklist 人眼把关；不接受 dev 自行替换

[其余 primitives（Card / PrimaryButton / Avatar / FadeIn / RarityTag）实装与 v1 起草版相同，原文略]

#### Story 37.7: HomeView Scaffold + HomeViewModel class 层次 + Mock/Real 两实现

As an iPhone 用户,
I want Home Tab idle 态显示 ui_design 高保真界面，且接缝设计支持 Story 21.1 / 12.7 后续注入真实 ViewModel,
So that 既有视觉壳又有可持续接缝.

**Acceptance Criteria:**

**Given** Story 37.3 MainTabView + Story 37.4 AppState + Story 37.5 Theme + Story 37.6 primitives 全部就绪
**When** 完成本 story
**Then** 在 `iphone/PetApp/Features/Home/Views/HomeView.swift` 实装（Story 2.2 占位版完整重写）:
- 顶部 StatusBar：天气问候 + 步数计（mock "今天阴天 🌥️" + appState.currentStepAccount?.balance ?? 0）
- 小猫舞台 Card：`Image(systemName: "cat.fill")` + 椭圆背景 + theme.colors.accent-soft 条纹 overlay + 等级名牌（appState.currentPet?.name + "Lv." + appState.currentPet?.level）+ 三状态条（饱食/心情/活力，0-100 progress bar；mock 值在 MockHomeViewModel）
- ActionRow：3 个互动按钮（喂食 🍥 / 抚摸 💕 / 玩耍 ⭐）；点击触发 emoji floatUp 1.4s（按 README §Interactions）
- **宝箱位接缝**（**硬接缝**，v2.1 修订 + v2.2 修复 codex review BLOCKER 7）：HomeView 是 generic struct `HomeView<ChestSlot: View>`，含 `let chestSlot: () -> ChestSlot` ViewBuilder closure 参数 + `@ObservedObject var state: HomeViewModel`（**基类直接，不再泛型**——SwiftUI 用 `@ObservedObject` + class 层次结构 + 子类多态即可，View signature 仅 `<ChestSlot>` 一个泛型参；State 走 class 层次而非泛型避免 caller 端类型膨胀）；调用方在 RootView/MainTabView 内构造 `HomeView(state: realHomeViewModel) { ChestCardView(...) }`；本期 Scaffold 阶段传入 `EmptyView()` 占位（落地写法 `HomeView(state: vm) { EmptyView() }`）；Story 21.1 时 dev 改调用方传入真实 ChestCardView，**HomeView 内部代码 zero edit**。State class 层次：基类 `class HomeViewModel: ObservableObject` 含基础字段 + abstract method；`MockHomeViewModel: HomeViewModel` 和 `RealHomeViewModel: HomeViewModel` 各自子类实装；**不**采用 `protocol P + any P` 模式（`any P` 不能 conform `ObservableObject`，SwiftUI 不支持）
- 底部 TeamIdleCard：theme.colors.accent 渐变 + Card 圆角 22 + "创建队伍" / "加入队伍" 两 PrimaryButton
- **接缝设计**（v2.2 修复 codex BLOCKER 7：用 class 层次结构而非 `any P`）：
  - 定义 `class HomeViewModel: ObservableObject { @Published var greeting: String = ""; @Published var weather: String = ""; @Published var stats: PetStats = .zero; @Published var interactionAnimation: AnimationState = .idle; func onCreateTap() { fatalError("subclass override") }; func onJoinTap() { ... }; func onFeedTap() / onPetTap() / onPlayTap() { ... } }`（基类含字段 + 默认/abstract 方法）
  - HomeView 是 generic struct `<ChestSlot: View>` 持 `@ObservedObject var state: HomeViewModel`（基类）
  - `MockHomeViewModel: HomeViewModel`：硬编码 mock 数据 + override 方法 print log；构造注入 MockAppState 或不依赖 AppState
  - `RealHomeViewModel: HomeViewModel`：通过**构造注入** AppState（`init(appState: AppState)`）+ override 方法接 UseCase；computed 派生 view-specific 状态（chestRemainingSeconds / interactionAnimation 等是 transient 字段，不在 AppState）；不在 ViewModel 内用 @EnvironmentObject
  - 默认在 RealHomeViewModel 实现下渲染（生产路径）；MockHomeViewModel 用于 #Preview
**And** "创建队伍" 按钮调 `state.onCreateTap()`（Mock 时 print log；Real 实现里调 CreateRoomUseCase by Story 12.7）
**And** "加入队伍" 按钮 trigger `state.showJoinModal = true`（HomeViewModel 基类含 `@Published var showJoinModal: Bool`，HomeView 用 `.sheet(isPresented: $state.showJoinModal)` 挂 JoinRoomModal by Story 37.12；**不在 View 层用 @State**——避免 v2.1 codex BLOCKER 10 中提到的 state owner 双写）
**And** **单元测试覆盖**（≥4 case，纯 XCTest + MockAppState + MockHomeViewModel）:
- happy: 注入 MockHomeViewModel → View body 含 greeting / weather / 三状态条 a11y identifier
- happy: 点 "创建队伍" → onCreateTap 闭包触发（用 invocations 数组验证）
- happy: 点 "喂食" → interactionAnimation = .flying("🍥")（用 invocations 验证）
- edge: stats.hunger = 0 → 状态条渲染最低值（不报错；用 a11y label 文字验证）
**And** #Preview 提供 MockHomeViewModel + candy 主题
**And** UITest: a11y identifier `homeStatusBar` / `homeCatStage` / `homeActionFeed` / `homeActionPet` / `homeActionPlay` / `homeTeamIdleCard_create` / `homeTeamIdleCard_join` 可定位

#### Story 37.8: RoomView Scaffold + RoomViewModel class 层次 + MockRoom

[结构与 37.7 类似；视觉细节按 ui_design §RoomScreen 高保真还原；**接缝采用 class 层次结构**（基类 `class RoomViewModel: ObservableObject`，子类 `MockRoomViewModel` / `RealRoomViewModel`；不用 `any P` 模式）；MockRoomViewModel 注入 mock 3 成员；RealRoomViewModel 留空骨架（Story 12.1 落实）；离开按钮调 `state.onLeaveTap()`（Mock 时 print；Real 时 LeaveRoomUseCase）；**房间代码位置直接显示 roomId 字符串**（accessibility identifier `roomIdDisplay`）；详细 acceptance 与 37.7 同等结构]

#### Story 37.9: WardrobeView Scaffold + WardrobeViewModel class 层次 + MockWardrobe

[结构同上；**接缝采用 class 层次结构**（基类 `class WardrobeViewModel: ObservableObject`，子类 Mock/Real）；mock Inventory 含 N/R/SR/SSR 各几个；装备/卸下按钮 Mock 时仅切换 ViewModel 内 selectedCosmeticId；详细 acceptance 与 37.7 同等结构]

#### Story 37.10: FriendsView Scaffold + FriendsViewModel class 层次 + MockFriends

[结构同上；**接缝采用 class 层次结构**（基类 `class FriendsViewModel: ObservableObject`，子类 Mock/Real）；mock 8 friend 含三态（在线/在房间/离线）各 2-3 个；操作按钮 Mock 时仅 toast；friends 数据归 ViewModel cache，不进 AppState（参见 ADR-0010 §3.2 Friends 数据归属注释）；详细 acceptance 与 37.7 同等结构]

#### Story 37.11: ProfileView Scaffold + ProfileViewModel class 层次（含微信绑定卡 + Modal 视觉）

[结构同上；**接缝采用 class 层次结构**（基类 `class ProfileViewModel: ObservableObject`，子类 Mock/Real）；含 ui_design/wechat_binding.md 设计的微信绑定卡 + 绑定 Modal 视觉；按钮触发后仅 toast "微信绑定（敬请期待）"；不调真 OAuth；其它 acceptance 与 37.7 同等结构]

#### Story 37.12: JoinRoomModal + 跨屏 join 链路（roomId 字符串 + 直白 UI）

As an iPhone 用户,
I want JoinRoomModal 接受房间号字符串输入，跨屏 join 链路统一走 roomId 字符串,
So that A 分享给 B 的房间号 B 能直接输入加入，无 sender/receiver 闭环问题（v2.1 BLOCKER 1 修复）.

**Acceptance Criteria:**

**Given** Story 37.4 AppState 就绪 + Story 37.7 HomeView TeamIdleCard "加入队伍"按钮 + Story 37.10 FriendsView FriendRow "加入"按钮
**When** 完成本 story
**Then** 在 `iphone/PetApp/Shared/Modals/JoinRoomModal.swift` 实装:
- 底部 sheet（`.sheet(isPresented: $homeViewModel.showJoinModal)` 挂在 HomeView 内）：背景遮罩 0.45 black + 卡片从下方 20pt 上滑 0.3s
- 卡片 Card 圆角 26 + theme.colors.surface 背景
- 标题"加入队伍" + 关闭按钮（xmark.circle.fill 右上角）
- 说明文字"输入好友分享给你的房间号"
- 大输入框：图标（pawprint.fill）prefix + 等宽字体 + 自动 trim 前后空格 + 限 64 字符（足够覆盖所有合理 roomId 字符串长度）
- 格式提示"输入好友分享给你的房间号"灰字 small（**不**暗示数字-only；roomId 类型 String，格式由 server AR21 决定）
- 取消 / 确定加入 两按钮：仅输入 trim 后非空启用确定（不做客户端格式校验——server response 决定合法性；本地仅做最少前置约束 trim + length）
- 确定加入 → 调用通过构造注入的 `onConfirm: (String) async -> Void` 闭包（**不直接调 AppState 或 UseCase**——保持 modal 与业务解耦）；onConfirm 的实际实现：HomeView 注入 `realHomeViewModel.handleJoinSubmit(roomId)` 内部调 JoinRoomUseCase；**不存在** `AppState.joinTeam(code:)` 方法（v2.2 修订删除该残留方法名）
- HomeScreen 触发：HomeView 内 `.sheet(isPresented: $state.showJoinModal)` 挂 JoinRoomModal；state 是 HomeViewModel 基类持 `@Published var showJoinModal: Bool`（**唯一 owner**，避免 state 双写）
- FriendsScreen "加入" 按钮（好友在房间中）触发：解析 `friend.currentRoomId: String` **直接**调 JoinRoomUseCase（不弹 Modal，因为已知 roomId）；按 ui_design "加入好友房间流程"
**And** **不引入** RoomCodeDisplayFormatter / K3M9P2 美化别名（v2.1 BLOCKER 1 决议：UI 全程显示 roomId 字符串，避免 sender/receiver 闭环）
**And** **单元测试覆盖**（≥5 case，纯 XCTest，不引第三方）:
- happy: 输入 "1234567" → 确定按钮启用
- happy: 输入 "1234567" 点确定 → onConfirm 闭包被调用 + 参数 == "1234567"（用 invocations 数组验证）
- edge: 空输入 → 确定按钮 disabled
- edge: 仅空格输入 → 自动 trim 后判定为空 → disabled
- happy: 输入超过 64 字符 → 截断在 64 字符
**And** UITest: HomeView 点 "加入队伍" → JoinRoomModal accessibility identifier `joinRoomModal` 出现 → 输入 "1234567" → 点 confirm → modal dismiss + RoomView 出现（`roomView` a11y identifier 可定位）
**And** **接缝设计**：JoinRoomModal struct 签名 `JoinRoomModal(onConfirm: @escaping (String) async -> Void, onCancel: @escaping () -> Void)`；调用方 HomeView 注入闭包；MockHomeViewModel 闭包仅 print；RealHomeViewModel 闭包内调 JoinRoomUseCase

#### Story 37.13: accessibility identifier 全屏总表 + 视觉回归 review checklist（合规兜底，非 snapshot 替代）

As an iOS 开发,
I want 一份覆盖全 Scaffold 的 accessibility identifier 总表 + 人工视觉 review checklist 文档,
So that UI 测试有可依赖的定位锚点 + 严守 ADR-0002 §3.1 的合规兜底（**注**：本 story 是合规兜底措施，不声称等价于 snapshot 测试覆盖度；视觉回归仍依赖人眼 PR review，未来如有需要可单独立 ADR 评估引入 snapshot 库——v2.1 WARN 2 修复）.

**Acceptance Criteria:**

**Given** Story 37.5-37.12 全部完成
**When** 完成本 story
**Then** 扩展 `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（已存在）覆盖：
- 5 屏的 Tab Bar 定位（tab_home / tab_wardrobe / tab_friends / tab_profile）
- HomeView 全部交互元素（status / cat / 3 个 action 按钮 / team idle card 2 按钮）
- RoomView 全部（returnButton / roomIdDisplay / copyButton / 4 个成员位 / 离开按钮）
- WardrobeView 全部（5 分类 Tab / 预览区 / equip/unequip 按钮 / grid 单元 cell template）
- FriendsView 全部（统计 / 添加 / 房间提示条 / 在线·全部 Tab / friend row template）
- ProfileView 全部（avatar / username / 4 列统计 / 5 张最近收藏 / 4 项菜单 / 微信绑定卡）
- JoinRoomModal 全部（input field / cancel / confirm）
**And** 静态校验脚本 `iphone/scripts/check_a11y_coverage.sh`：grep 全 Features/ + Shared/Modals 内 SwiftUI View body 含 button/list 等交互元素的 accessibilityIdentifier 注解；不达标元素列出并 fail
**And** 静态校验脚本 `iphone/scripts/check_no_apiclient_in_features.sh`：grep 全 Features/{Home,Room,Wardrobe,Friends,Profile} + Shared/Modals + Core/DesignSystem 内 import APIClient / Repository / UseCase（除 Story 37.4 落 RealHomeViewModel 等显式 wire 模式）；违规列出 fail
**And** 视觉回归 review checklist 文档 `iphone/docs/visual-review-checklist.md`：
  - 每屏的 6-10 个 manual visual check item（如"HomeScreen StatusBar 步数文字 14pt 800 weight" / "RoomScreen MiniCat 弹跳错峰 0.2s"）
  - 含截图位（人眼对比 ui_design 原型）
  - 用于 PR review 时手动逐项打勾
**And** **deliverable**：AccessibilityID.swift 扩展 + 2 个静态校验脚本 + visual-review-checklist.md 文档；后两者 commit 入 git
**And** check 脚本各跑 1 次本地通过 + 入 `iphone/scripts/build.sh --test` 调用链

#### Story 37.14: design-package 白名单文档（声明本期不做的 ui_design 元素）

As an PM / SM,
I want 一份白名单文档明确列出 iphone/ui_design 内本期 Epic 37 不做的元素（视觉壳不覆盖），且每条带 PRD 边界依据,
So that 后续 sprint 不重复争论"为什么 Profile 的 X 没做" + 透明传达取舍.

**Acceptance Criteria:**

**Given** Story 37.5-37.13 全部完成
**When** 完成本 story
**Then** 新建 `iphone/docs/ui-design-scope-whitelist.md`，列出（每条含 ui_design 文件位置 + 不做理由 + 何时做）：
- `tweaks-panel.jsx`（开发时主题切换调试面板）→ 不做（README §设计参考说明已说明此为开发时不需要）→ 永不实装
- 三主题切换 UI（让用户切 candy/matcha/sky/dark）→ 不做（仅 Theme stub；UI 切换面板可视情况未来加）→ 后续 mini-epic
- `wechat_binding.md` 真实 OAuth 调用 → 不做（视觉壳做，按钮 toast）→ 节点 12 后另起 epic（PRD §4 暂不做：微信绑定）
- Profile 顶部 bell（消息通知）按钮的真实通知中心 → 不做（按钮 toast）→ 后续 epic
- Profile "成就徽章"页 / "喜欢的道具"页详情 → 不做（菜单按钮 toast）→ 后续 epic
- HomeView 互动后状态条变化（喂食后饱食 +5 等）→ 不做（mock 状态条不变；ActionRow 仅触发 emoji floatUp）→ 节点 3 起接 LoadHomeUseCase 后端真实状态
- WardrobeView 钻石货币的真实数值更新 → 不做（mock 248 写死）→ 后续 epic（与商城功能联动；本 MVP 不含商城）
- 小猫 3D 模型（USDZ / RealityKit）→ 不做（用 SF Symbol cat.fill + 椭圆背景占位）→ 美术资源就位后另起 spike
- 装扮道具 emoji 占位 → 仅本期 Scaffold 用（节点 10 起 Story 30.x 替换为 SpriteRenderer + render_config，仅猫身上；仓库 grid emoji 占位**保留**直到美术资源就位）
**And** 文档结构清晰；每条至少 3 个 bullet（位置 / 理由 / 何时做）
**And** **deliverable**：`iphone/docs/ui-design-scope-whitelist.md` commit
**And** 治理类 story，不强制单元测试

---

## 8. Implementation Handoff

### 8.1 Scope 分类

**Major** —— 推翻 Story 2.3 + Story 5.5 核心架构决策；含 2 个新 ADR；改动跨节点（影响 Epic 6 / 12 / 21 / 24 / 27 / 30 / 33 / 35）。

### 8.2 Handoff Recipients

| 角色 | 责任 | 签字位 |
|---|---|---|
| **Architect（Winston）** | review ADR-0009 / ADR-0010 决策合理性；尤其 Story 37.3 / 37.4 的迁移步骤（partial revert 风险）；currentTab 归 AppCoordinator + ViewModel 构造注入规则确认；roomId String 类型契约确认 | [ ] 日期：____ 签字：____ |
| **Product Manager（John）** | review PRD §4 4 Tab 澄清段；**特别签字**：微信绑定 UI 视觉壳（按钮 toast）是否突破 §4「微信绑定（结构预留，节点 2 不实现 UI）」边界——v2.1 解释为「视觉占位属结构预留的一部分」是扩义而非纯澄清，需 PM 明确 yes/no | [ ] 日期：____ 签字：____ |
| **Scrum Master（Bob）** | 把提案 ① ② ③ ④ ⑤ ⑥ 落到对应 artifact；跑 `bmad-create-story 37.1` 启动；epic-loop 推进 14 story | [ ] 日期：____ |
| **Developer Agent（Amelia）** | epic-loop 循环实装；每 story codex review + lesson 归档 | n/a（持续过程）|

### 8.3 Success Criteria

- 14 条 Story 全部 done（含 codex review approved）
- 2 个 ADR Status = Accepted
- `bash iphone/scripts/build.sh --test` 通过（含 a11y identifier 全覆盖检查 + APIClient 红线检查）
- 5 屏 + Modal 在模拟器上视觉与 `iphone/ui_design/source/index.html` 一致（按 visual-review-checklist.md 人眼校验）
- AppState 三 ViewModel 联动可测可验（Home Tab CatStage 装备变化、Wardrobe Tab 装备状态、Profile Tab 猫等级 三屏数据从同一 AppState 派生）
- ResetIdentityViewModel.resetTapped 触发 appState.reset() + Keychain 清空 + 重新 bootstrap 全链路通过
- epic-37-retrospective 完成（**强制**，不是 optional）

### 8.4 Next Steps（用户终审通过后）

1. SM 把提案 ① 已落（ADR 草稿就位）；提案 ② ③ ④ ⑤ ⑥ 落到 epics.md / prd.md / sprint-status.yaml
2. SM 跑 `bmad-create-story 37.1` 启动首条 story（ADR-0009 撰写）
3. ADR-0009 / ADR-0010 完成后 Architect / PM 同步 review；签字
4. Dev 启 epic-loop 推 37.3 → 37.14（按依赖链）
5. epic-37 全部 done + retro 完成后启动 epic-6 节点 2 demo

---

## 9. 用户终审记录

- [ ] 用户终审通过日期：____
- [ ] 落盘 commit hash：____
- [ ] 落盘 commit message：____
- [ ] 第一条 story（37.1）启动日期：____
- [ ] codex 审本 v2 verdict：____
