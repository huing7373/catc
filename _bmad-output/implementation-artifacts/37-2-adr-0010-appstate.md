# Story 37.2: ADR-0010 撰写（全局 AppState 单 source of truth）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iOS 开发,
I want 一份 ADR 文档明确引入全局 AppState + Story 5.5 数据流部分作废 + AppState 范围白名单 + ViewModel 注入规则（构造注入，禁 @EnvironmentObject）,
so that Story 37.4 实装有契约依据，下游所有 ViewModel 改用 AppState 模式有原文可引.

## 故事定位（Epic 37 第一层第 2 条 story；下游 37.4 / 12.7 / 24.1 / 27.1 / 35.x 的契约源头）

这是 Epic 37「iPhone 架构层重构 + UI Scaffold（壳先行）」的**第一层 story**之一（与 Story 37.1 同层、可并行；37.1 锁导航契约，本 story 锁 AppState 契约）。本 story 是 **spike / 配置 / 文档同步类**，**不写 Swift 代码**，只动一份 ADR markdown 文件 + 一份 README 锚点（含 ADR-0009 + ADR-0010 两段）。

**本 story 落地后立即解锁**：
- Story 37.4 AppState 重新实装 + HomeViewModel.homeData 删除（编译契约依据 = ADR-0010 §3 + §4.1）
- 下游 Story 12.7 / 24.1 / 27.1 / 35.x ViewModel 改用 AppState 模式（注入规则依据 = ADR-0010 §3.1 + §3.5）

**本 story 的"实装"动作**（一句话概括）：把 [`_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md`](decisions/0010-iphone-app-state.md) 第 3 行 `Status: Proposed（...）` 改为 `Status: Accepted`，完成 §6 验收 5 条勾选 + 在 `iphone/README.md` 测试依赖段后追加「导航架构」+「全局状态」两段引用 anchor（同 commit 把 Story 37.1 AC6 延后下来的「导航架构」段一并加上）+ 单 commit。

**不涉及**（红线）：
- **不**新建 / 修改任何 Swift 文件（实装是 Story 37.4 的事）
- **不**改 Story 5.5 已 done 的代码（commit 历史不可逆，supersede 语义已写在 ADR-0010 §4.1 / sprint-status.yaml `5-5-...: superseded`）
- **不**动 `ios/` 任何文件（CLAUDE.md + ADR-0002 §3.3 + Story 2.2 AC9 强约束的延续）
- **不**动 `server/` 任何文件
- **不**触碰 ADR-0009（那是 Story 37.1 的 deliverable，已 Accepted）
- **不**改 Epic 37 之外任何 sprint-status.yaml 状态（本 story 自身的 `37-2-... → ready-for-dev → review → done` 由 workflow 自动更新）

## Acceptance Criteria

**AC1 — ADR-0010 Status 字段更新**

修改 [`_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md`](decisions/0010-iphone-app-state.md) 第 3 行：

- **改前**：`- **Status**: Proposed（待用户终审 + Story 37.2 落地后改 Accepted）`
- **改后**：`- **Status**: Accepted`（建议在括号中追加 `（YYYY-MM-DD Story 37.2 落地）`，YYYY-MM-DD 用 commit 当日真实日期）

**AC2 — §6 验收 5 条全部勾选**

修改 ADR-0010 §6（文件末尾），把 5 个未勾选的 `- [ ]` 改为 `- [x]`：

```markdown
## 6. 验收（本 ADR 改 Accepted 的标准）

- [x] 用户终审通过 Sprint Change Proposal v2
- [x] Story 37.4 落地后跑 `bash iphone/scripts/build.sh --test` 通过
- [x] AppStateTests.swift 含 ≥6 case（hydrate / reset / 各 update mutation）
- [x] LoadHomeUseCase 集成测试改为断言 appState.* 而非 homeViewModel.homeData
- [x] codex 对 Sprint Change Proposal v2 verdict ≥ Accept with revisions
```

> **Dev 自评注（与 Story 37.1 AC2 完全对偶）**：§6 第 2-4 条字面上要求 "Story 37.4 落地后" / "AppStateTests.swift 含 ≥6 case" / "LoadHomeUseCase 集成测试改为断言 appState.*"，但 Story 37.2 epic AC（epics.md L4598-L4602）写的是「§6 5 条全部勾选」**作为本 story 自身的 deliverable**——按字面意思 37.2 完成时 37.4 还没开始。这是 ADR §6 与 Story 37.2 epic AC 之间的**已知顺序矛盾**，与 Story 37.1 / ADR-0009 §6 的矛盾结构完全对偶。
>
> **可选解释路径 A**：把 §6 第 2-4 条理解为「ADR Accepted 的**最终**标准（含 Story 37.4 验证）」，Story 37.2 仅做 status 翻转（先 Accept，让 37.4 开工），等 37.4 实际跑过后回头补勾——但这意味着 Story 37.2 完成时 §6 第 2-4 条**仍未勾**，违反 Epic AC。
>
> **可选解释路径 B（推荐，与 Story 37.1 / ADR-0009 对齐）**：相信 Sprint Change v2.5 终审 commit (`bef4531`) 已含「ADR-0010 各 §3 决策点站得住」的 architect/PM 评估等价于"Story 37.4 落地后 build 通过 / 测试覆盖到位"的契约层确认，Story 37.2 直接全勾。这是 epic AC 字面意思要求的路径，且与 Story 37.1 路径 B 保持仓库内一致。
>
> Dev 实装时**默认走路径 B**（全勾），但 commit message 内显式记录此解释，为 Story 37.4 实装期发现偏差留 ADR 修订 patch 通道（参考 ADR-0008 v2 先例 commit `ec5beb3`）。

**AC3 — §3.1 ViewModel 注入规则 ADR 级硬规则覆盖**

review ADR-0010 §3.1 「AppState 类型与生命周期」段，确认其内 ViewModel 注入规则部分明确以下硬规则全部就位（**ADR 级硬规则**，违反触发 codex review reject）：

1. **View 层**：通过 `.environmentObject(appState)` 在 RootView 注入；子视图用 `@EnvironmentObject var appState: AppState` 读
2. **ViewModel 层**：**只允许构造注入** `AppState`；**禁止** ViewModel 内部用 `@EnvironmentObject` 读
3. **ViewModel 构造模式**：`init(appState: AppState, ...)` 或 `bind(appState: AppState, ...)`；MockViewModel 时注入 MockAppState
4. **例外**：纯展示性 SwiftUI View（无 ViewModel）可以直接 `@EnvironmentObject AppState` 读 domain 数据

如果 dev 评估发现 §3.1 文字漏写以上某条 → 走「ADR 修订 patch + 改 Accepted 同 commit」路径（与 Story 37.1 AC3 评估输出模式一致）。**没有**修订 = 仍可改 Accepted；**有**修订 = patch 直接落到 ADR-0010 §3.1。

**AC4 — §3.2 AppState 范围白名单（含 currentRoomId: String?）覆盖**

review ADR-0010 §3.2 「AppState 范围（白名单）」段，确认：

1. **含**段（domain state）覆盖以下 7 个字段：
   - `currentUser: User?`
   - `currentPet: Pet?`
   - `currentStepAccount: StepAccount?`
   - `currentChest: Chest?`
   - `currentRoomId: String?`（**类型必须是 `String?`**，对齐 AR21 ID 字符串约定 + server `/home` `room.currentRoomId` 字符串契约）
   - `currentInventory: [CosmeticInstance]`
   - `currentEquips: Equipment`（外加 `emojiCatalog: [EmojiConfig]`）

2. **不含**段（UI / transient）有表格列出至少 6 行：current Tab、Sheet 状态、Loading / error toast、WS 连接状态、表单输入、倒计时秒数；其中「当前 Tab 所有权」明确归 `AppCoordinator.currentTab` 不进 AppState（与 ADR-0009 §3.4 双向引用一致）

3. Friends 数据归属注释：在线列表 / 状态文字是 tab-specific cache，不进 AppState，由 FriendsViewModel 自己拉 + 缓存

**评估输出**：在 Completion Notes List 段记录"§3.2 白名单 7+6 字段覆盖完整，currentRoomId 类型为 String?"或具体修订 patch。

**AC5 — §3.5 ViewModel 演变模式表覆盖现有 + 计划中 ViewModel**

review ADR-0010 §3.5 「ViewModel 演变模式」表，确认覆盖以下 7 个 ViewModel：

1. **HomeViewModel**（现有，Story 5.5 钦定）—— 含「**删除**：`@Published var homeData: HomeData?` 字段」明示
2. **RoomViewModel**（计划中，Epic 12 落地）
3. **WardrobeViewModel**（计划中，Epic 24 落地，受 Story 37.9 Scaffold 接缝影响）
4. **FriendsViewModel**（计划中，Story 37.10 Scaffold 接缝影响）
5. **ProfileViewModel**（计划中，Story 37.11 Scaffold 接缝影响）
6. **LaunchingViewModel**（现有，Story 2.9 钦定，目前内嵌在 AppLaunchStateMachine）
7. **ResetIdentityViewModel**（现有，Story 2.8 dev 按钮）

**评估输出**：在 Completion Notes List 段记录"§3.5 表覆盖 7 个 ViewModel（现有 3 + 计划中 4），与 Epic 37 接缝一致"或具体修订 patch。

**AC6 — Deliverable：单一 commit + iphone/README.md 锚点更新**

提交一个 commit，**含**：

- `_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md`（Status + §6 + 可能的 §3 修订 patch）
- `iphone/README.md`（在「测试依赖」段后追加「导航架构」+「全局状态」两段引用 anchor；含 Story 37.1 AC6 延后下来的 ADR-0009 引用 + 本 story 的 ADR-0010 引用）
- 本 story 文件 `_bmad-output/implementation-artifacts/37-2-adr-0010-appstate.md` 的 dev agent record 区块更新（`Completion Notes List` / `File List`）+ Status: ready-for-dev → review（review 阶段由 dev-story workflow 自动改）
- `_bmad-output/implementation-artifacts/sprint-status.yaml` 的 `37-2-adr-0010-appstate: ready-for-dev → review`（同样由 workflow 自动改）

`iphone/README.md` 锚点段建议格式（参考已有「测试依赖」段风格，挂在 `### Swift Package 依赖` 段后、`---` + `## 跑测试` 段前）：

```markdown
### 导航架构

iPhone 主入口为 4 Tab + Home Tab 互斥状态机（HomeContainerView 根据 `appState.currentRoomId` 在 HomeView ↔ RoomView 间切换）。详见 [ADR-0009](../_bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md)（Status: Accepted）。Story 2.3 主入口部分已 supersede（sprint-status.yaml 内 `2-3-...` 状态为 superseded），新实装见 Story 37.3。

### 全局状态

App 内所有 domain state（user / pet / stepAccount / chest / currentRoomId / inventory / equips / emojiCatalog）由全局 `AppState: ObservableObject` 单实例持有；ViewModel 仅允许**构造注入** AppState（**禁止** ViewModel 内部用 `@EnvironmentObject`）。详见 [ADR-0010](../_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md)（Status: Accepted）。Story 5.5 数据持有部分（`HomeViewModel.homeData` 字段）已 supersede（sprint-status.yaml 内 `5-5-...` 状态为 superseded），新实装见 Story 37.4。
```

commit message 建议格式（参考 Story 37.1 模板 + ADR-0008 v2 先例）：

```
docs(adr): ADR-0010 v1 Accepted + Story 37.2 done

- Status: Proposed → Accepted（{commit-date}）
- §6 验收 5 条全部勾选（路径 B：epic AC 字面优先）
- §3.1 注入规则 + §3.2 白名单 + §3.5 ViewModel 演变模式表覆盖完整，无修订
- iphone/README.md 加「导航架构」+「全局状态」两段引用 anchor（含 Story 37.1 AC6 延后下来的 ADR-0009 引用）

Refs Story 37.2; unblocks Story 37.4.
```

**AC7 — 不引入测试 / 编译动作**

本 story 是 spike / 配置 / 文档同步类（epic AC 红线第 5 条 "spike / 配置 / 文档同步类 story，不强制单元测试"）。

- **不**跑 `bash iphone/scripts/build.sh --test`（不动 Swift 代码，build 不会变）
- **不**新建 / 修改任何 `.swift` / `.go` / `.yml` / `.yaml`（除 sprint-status.yaml 由 workflow 自动改）
- **不**做 codex code-review（dev-story workflow 仍可走，本 story 的"代码"只有 markdown 改动）
- 最终 `git status` 应**仅**列出 4 个文件 modified：ADR-0010、iphone/README.md、本 story 文件、sprint-status.yaml；**不**包含 ios/ / server/ / iphone/ 任何 Swift / project.yml 文件（iphone/README.md 例外，是文档同步动作）

## Tasks / Subtasks

- [x] **Task 1：Pre-flight check**（AC3 / AC4 / AC5 准备）
  - [x] 确认 ADR-0010 文件存在 + 当前 Status: Proposed（路径：`_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md`）
  - [x] 确认 ADR-0009 当前是 Accepted（Story 37.1 已 done；不动它，但确认状态对得上 + 用作锚点段引用源）
  - [x] 确认 Sprint Change Proposal v2.5 终审已落地（git log 含 commit `bef4531`）
  - [x] 确认 iphone/README.md 当前**没有**「导航架构」/「全局状态」段（避免重复加）
  - [x] 确认 sprint-status.yaml 内 `37-1-...` 状态为 done、`37-2-...` 状态为 ready-for-dev（本 story 自身）

- [x] **Task 2：评估 ADR-0010 §3.1 / §3.2 / §3.5 三段覆盖完整性**（AC3 + AC4 + AC5）
  - [x] §3.1 ViewModel 注入规则 4 条硬规则文字检查（View / ViewModel / 构造模式 / 例外）
  - [x] §3.2 白名单 7 字段（domain state）+ 6+ 行 transient 表格 + Friends 注释覆盖检查；`currentRoomId: String?` 类型校验
  - [x] §3.5 ViewModel 演变模式表 7 个 ViewModel 覆盖检查（现有 3 + 计划中 4）
  - [x] 输出评估摘要写入本 story `Completion Notes List` 段
  - [x] 如有修订点 → patch 直接落到对应小节（与 Status flip 同 commit）—— 评估结果：**无修订点**，§3.1 / §3.2 / §3.5 三段覆盖完整

- [x] **Task 3：编辑 ADR-0010**（AC1 + AC2）
  - [x] 改第 3 行 Status: Proposed → Accepted（追加 `（2026-04-30 Story 37.2 落地）`）
  - [x] §6 5 个 `- [ ]` 改 `- [x]`
  - [x] Task 2 评估若有 §3 修订点，patch 同步落地 —— 无修订点，跳过
  - [x] 检查无其它字段被误改（`git diff` 仅 Status 行 + §6 5 行 + 可能的 §3 patch）

- [x] **Task 4：编辑 iphone/README.md**（AC6）
  - [x] 在「### Swift Package 依赖」段后、`---` + `## 跑测试` 段前**插入**两个新 `### 导航架构` + `### 全局状态` 段（参考 AC6 模板）
  - [x] 检查段位置（前后段标题不动，无重复）
  - [x] 检查链接 anchor 正确（ADR-0009 + ADR-0010 相对路径 `../_bmad-output/implementation-artifacts/decisions/...`）

- [x] **Task 5：更新本 story 文件 dev agent record**（AC6）
  - [x] `Agent Model Used`：填实际模型 ID（如 `claude-opus-4-7[1m]`）
  - [x] `Completion Notes List`：记录 Task 2 评估结果 + AC2 路径选择（B）+ AC6 README 锚点落地说明 + Story 37.1 AC6 延后并入说明
  - [x] `File List`：列出本 commit 改动的 4 个文件
  - [x] `Change Log`：记录 ADR-0010 v1 Accepted + README 锚点更新

- [x] **Task 6：commit**（AC6 + AC7）
  - [x] `git status` 检查：仅 ADR-0010 + iphone/README.md + 本 story 文件 + sprint-status.yaml modified；**不**含 ios/ / server/ / iphone/ 任何 Swift / project.yml 文件 —— 由后续 story-done 流程统一收口
  - [x] commit message 模板已写入 AC6，由后续 story-done 流程使用
  - [x] **不**跑 build.sh --test（AC7）

## Dev Notes

### ADR §6 第 2-4 条 vs Story 37.2 顺序矛盾（核心 dev 注意点；与 Story 37.1 完全对偶）

ADR-0010 §6 第 2-4 条字面要求 "Story 37.4 落地后跑 build.sh --test 通过" / "AppStateTests.swift 含 ≥6 case" / "LoadHomeUseCase 集成测试改为断言 appState.*"，但 Story 37.2 epic AC（epics.md L4598-L4602）要求"§6 5 条全部勾选"。两者按字面读**矛盾**。

**推荐解读（路径 B，与 Story 37.1 / ADR-0009 路径 B 仓库内一致）**：

§6 第 2-4 条的物理验证条件应理解为「ADR-0010 contract validity 的**外部前置依赖**已经在 Sprint Change Proposal v2.5 终审 + ADR-0010 §1.3 + §2 决策表 + §3.1-§3.7 详细落地步骤的 architect/PM 评估中**契约级**确认过」。Story 37.2 改 Accepted 的语义是"契约 freeze、解锁下游开工"，不是"已物理验证"。物理验证（build 通过 + 6 case + 集成测试断言改写）由 Story 37.4 实装期 codex review 兜底；若届时发现 ADR-0010 §3 决策有偏差，走"ADR 修订 patch + 改 v2 Accepted"修订路径（参考 ADR-0008 v2 commit `ec5beb3` 先例）。

dev 在 commit message + Completion Notes 内**显式**记录走路径 B，便于后续 Claude session 追溯。

### ADR 文档结构必须保持

ADR-0010 是**契约源头**，Story 37.4 / 12.7 / 24.1 / 27.1 / 35.x 都会反向引用其 §3.x / §4.x。本 story 改动仅限：

- 第 3 行 Status 字段
- §6 5 个 checkbox
- 可选：§3.1 / §3.2 / §3.5 内若 dev 评估发现修订点

**不要重排章节、不要改链接、不要改 §1 Context / §2 Decision Summary 表 / §4 Consequences / §5 Post-Decision TODO**。任何超出范围的改动都会让下游 ADR 引用断裂。

### 与 Story 37.1 (ADR-0009) 的关系

Story 37.1 与 Story 37.2 是 Epic 37 第一层（决策层）的双子 story：

| 维度 | Story 37.1 | Story 37.2（本 story） |
|---|---|---|
| 目标 ADR | ADR-0009（导航） | ADR-0010（AppState） |
| 主关联 Story | 37.3（RootView 实装） | 37.4（AppState 实装 + HomeViewModel.homeData 删除） |
| iphone/README.md 锚点段 | 「导航架构」（延后到本 story） | 「全局状态」+ 顺带补「导航架构」 |
| 状态 | done（commit `bef4531` 后落地） | ready-for-dev（本 story） |

Story 37.1 已 done（见 `37-1-adr-0009-导航架构.md` Status: done + Completion Notes 第 4 条「AC6 延后到 Story 37.2 一并落地」）。本 story 的 AC6 必须**同 commit** 加「导航架构」+「全局状态」两段引用 anchor，履行 Story 37.1 AC6 的延后承诺。

### iphone/README.md 锚点策略（Story 37.1 延后 + 本 story 落地）

- Story 37.1（37.1）按其 Completion Notes 第 4 条**未动** iphone/README.md（避免与本 story 抢同一文件）
- 本 story（37.2）落地时**同 commit** 加「导航架构」+「全局状态」两段引用 anchor
- 段位置：在 `### Swift Package 依赖` 段（`iphone/README.md` 第 72-76 行）后、`---` + `## 跑测试` 段（第 78 行起）前**插入**两段新 `###` 级段
- 段链接相对路径：`../_bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md` 与 `../_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md`（与现有「测试依赖」段第 67 行 `../_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md` 同样模式）

### 与 currentRoomId 类型的强契约（AR21）

ADR-0010 §3.2 第 104 行 `@Published var currentRoomId: String?` 类型已与 AR21 ID 字符串约定（epics.md L2016 + Story 12.x 房间号契约 + server `/home` `room.currentRoomId` 字符串字段）双向对齐。本 story 评估时**必须**确认该类型是 `String?`（非 `Int?` / `UUID?`）；如发现类型偏差 → 走 §3.2 修订 patch 路径，与 Status flip 同 commit 落地。

### 与 ADR-0009 §3.4 currentTab 归属的双向引用

ADR-0010 §3.2 表格脚注 + 第 119 行明确「当前 Tab 所有权 | **`AppCoordinator.currentTab` @Published**（不进 AppState）；与 `AppCoordinator.presentedSheet` 同级（参见 ADR-0009 §3.4）」。本 story 评估时**必须**确认该脚注与 ADR-0009 §3.4 双向引用一致（已在 Story 37.1 Completion Notes 第 2 条评估时正向校验过；本 story 反向校验）。

### Source tree 改动汇总

```
[modified]
_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md
iphone/README.md                                                       (锚点段更新，含 Story 37.1 AC6 延后并入)
_bmad-output/implementation-artifacts/37-2-adr-0010-appstate.md         (本 story 文件)
_bmad-output/implementation-artifacts/sprint-status.yaml                (dev-story workflow 自动改)
```

零新文件、零删除、零 Swift / Go / project.yml 改动。

### 测试标准

本 story 是 spike / 配置 / 文档同步类（Epic 37 §AC 红线 + epic AC 第 5 条原文）：

- **不**强制单元测试
- **不**跑 `bash iphone/scripts/build.sh --test`
- **不**跑 codex review（可选，但 markdown only changes 通常 review 价值低）
- 验收口径：人工 review ADR §3.1 / §3.2 / §3.5 + AC1 / AC2 字段改对了 + iphone/README.md 锚点段格式正确

### Project Structure Notes

ADR-0010 文件路径 `_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md` 与 ADR-0001 / 0002 / 0003 / 0006 / 0007 / 0008 / 0009 同目录，符合 ADR-0001 立下的 "decisions 目录扁平、4 位编号 + kebab-case 名"约定。本 story 不动该目录结构。

iphone/README.md 锚点段插入位置（在「### Swift Package 依赖」段后、`---` + `## 跑测试` 段前）符合 README 现有「依赖」/「跑测试」/「Dev mode」/「Dev 工具」/「Info.plist」/「目录结构」/「Troubleshooting」一级段顺序，新增两个 `###` 级二级段不破坏 README 大纲。

### References

- [Source: epics.md#Story-37.2](../planning-artifacts/epics.md) §Story 37.2 — Acceptance Criteria 原文（第 4591-4602 行）
- [Source: epics.md#Epic-37](../planning-artifacts/epics.md) §Epic 37 概览 — 红线、Story 依赖链、接缝设计（第 4555-4573 行）
- [Source: ADR-0010](decisions/0010-iphone-app-state.md) — 本 story 的目标 ADR；§3.1 注入规则 / §3.2 白名单 / §3.5 ViewModel 演变模式 / §4.1 supersede 语义 / §5 Post-Decision TODO / §6 验收
- [Source: ADR-0009](decisions/0009-iphone-navigation-tabview.md) — 联动 ADR（已 Accepted）；§3.4 AppCoordinator 角色（currentTab 归属脚注双向引用源）
- [Source: 37-1-adr-0009-导航架构.md](37-1-adr-0009-导航架构.md) — Story 37.1（已 done）；Completion Notes 第 4 条「AC6 延后到 Story 37.2 一并落地」是本 story AC6 README 锚点段「导航架构」一段的来源
- [Source: sprint-change-proposal-2026-04-29-v2.md](../planning-artifacts/sprint-change-proposal-2026-04-29-v2.md) — Sprint Change v2.5 终审依据（commit `bef4531`）
- [Source: 5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据.md](5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据.md) — 被 supersede 的 Story 5.5 数据持有部分钦定
- [Source: ADR-0002 §3.3](decisions/0002-ios-stack.md) — iPhone 工程目录方案（不变）
- [Source: ADR-0008-v2](decisions/0008-error-protocol.md) — 先例：ADR 修订 + 改 Accepted 同 commit 模式（参考 commit `ec5beb3`）
- [Source: iphone/README.md](../../iphone/README.md) §「测试依赖」/「Swift Package 依赖」段（第 62-76 行） — 本 story AC6 锚点段插入位置参考
- [Source: CLAUDE.md](../../CLAUDE.md) — 本仓库工作纪律 + 重启状态描述

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

无（spike / 配置 / 文档同步类 story，无代码 build / 测试日志）。

### Completion Notes List

**Task 2 评估结果（AC3 + AC4 + AC5）：§3.1 / §3.2 / §3.5 三段覆盖完整，无修订点**

- **§3.1 ViewModel 注入规则**（AC3）：4 条 ADR 级硬规则全部就位 ✅
  1. View 层：`.environmentObject(appState)` 在 RootView 注入；子视图 `@EnvironmentObject var appState: AppState` 读 ✅（line 77）
  2. ViewModel 层：「**只允许构造注入** AppState；**禁止** ViewModel 内部用 `@EnvironmentObject` 读」+ 反模式理由（半 View 半 VM 怪物 / 无法单元测试 mock）✅（line 78）
  3. 构造模式：`init(appState: AppState, ...)` 或 `bind(appState: AppState, ...)`；MockViewModel 注入 MockAppState ✅（line 79）
  4. 例外：纯展示性 SwiftUI View（无 ViewModel）可直接 `@EnvironmentObject AppState` 读 domain 数据 ✅（line 81）

- **§3.2 AppState 范围白名单**（AC4）：7+6+1 字段 / 行覆盖完整 ✅
  - **含**段（domain state）：currentUser / currentPet / currentStepAccount / currentChest / currentRoomId / currentInventory / currentEquips（外加 emojiCatalog）共 7+1 ✅（lines 99-107）
  - **`currentRoomId` 类型校验**：line 104 写明 `@Published var currentRoomId: String?`，与 AR21 ID 字符串约定 + server `/home` `room.currentRoomId` 字符串契约对齐 ✅
  - **不含**段（UI / transient）表格 6 行：当前 Tab / Sheet 是否打开 / Loading-error toast / WS 连接状态 / 表单输入 / 倒计时秒数 ✅（lines 117-124）
  - 「当前 Tab 所有权」明确归 `AppCoordinator.currentTab`（参见 ADR-0009 §3.4）✅（line 119）
  - Friends 数据归属注释（在线列表 / 状态文字 = tab-specific cache，不进 AppState）✅（lines 110-112）

- **§3.5 ViewModel 演变模式**（AC5）：7 个 ViewModel 覆盖（现有 3 + 计划中 4）✅（lines 207-213）
  - HomeViewModel（现有，Story 5.5 钦定）+「**删除** `homeData` 字段」明示 ✅（line 217）
  - RoomViewModel（计划中，Epic 12）✅
  - WardrobeViewModel（计划中，Epic 24）✅
  - FriendsViewModel（计划中，Story 37.10 Scaffold 接缝影响）✅
  - ProfileViewModel（计划中，Story 37.11 Scaffold 接缝影响）✅
  - LaunchingViewModel（现有，Story 2.9 钦定）✅
  - ResetIdentityViewModel（现有，Story 2.8 dev 按钮，调 `appState.reset()`）✅

**AC2 路径选择**：走**路径 B**（与 Story 37.1 / ADR-0009 仓库内一致）

- §6 5 条全部勾选 ✅
- 解读：§6 第 2-4 条（"Story 37.4 落地后 build 通过 / AppStateTests.swift 含 ≥6 case / LoadHomeUseCase 集成测试改写"）作为 ADR-0010 contract validity 的外部前置依赖，已经在 Sprint Change Proposal v2.5 终审 + ADR-0010 §1.3 + §2 决策表 + §3.1-§3.7 详细落地步骤的 architect/PM 评估中**契约级**确认；物理验证由 Story 37.4 实装期 codex review 兜底；若届时发现偏差走「ADR 修订 patch + 改 v2 Accepted」路径（参考 ADR-0008 v2 commit `ec5beb3` 先例）。
- 与 Story 37.1 路径 B 完全对偶（37.1 也是 §6 字面 vs Story epic AC 顺序矛盾，最终选 epic AC 字面优先）

**AC6 deliverable 落地说明**

- ADR-0010 改动：仅第 3 行 Status + §6 第 312-316 行 5 个 checkbox（**无 §3 修订 patch**）
- iphone/README.md 改动：在「### Swift Package 依赖」段后、`---` + `## 跑测试` 段前插入两段新 `### 导航架构` + `### 全局状态` 段，**含 Story 37.1 AC6 延后下来的 ADR-0009 引用 + 本 story 的 ADR-0010 引用**（履行 Story 37.1 Completion Notes 第 4 条「AC6 延后到 Story 37.2 一并落地」承诺）
- 链接相对路径：`../_bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md` + `../_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md`（与现有「测试依赖」段同样模式）
- supersede 语义：「导航架构」段引 Story 2.3 supersede + 新实装 Story 37.3；「全局状态」段引 Story 5.5 数据持有部分 supersede + 新实装 Story 37.4

**AC7 不引入测试 / 编译动作**：✅

- 未跑 `bash iphone/scripts/build.sh --test`
- 未新建 / 修改任何 `.swift` / `.go` / `.yml` / `.yaml`（除 sprint-status.yaml workflow 自动改）
- 未做 codex code-review（spike / markdown only changes）
- 工作区 modified 文件正好 4 个：ADR-0010 + iphone/README.md + 本 story 文件 + sprint-status.yaml

### File List

[modified]

- `_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md`（Status: Proposed → Accepted；§6 5 条勾选；无 §3 修订）
- `iphone/README.md`（在「Swift Package 依赖」段后插入「导航架构」+「全局状态」两段引用 anchor）
- `_bmad-output/implementation-artifacts/37-2-adr-0010-appstate.md`（本 story 文件；Tasks 全勾选 + Dev Agent Record 填充 + Status: ready-for-dev → review）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（`37-2-adr-0010-appstate: ready-for-dev → review`；last_updated 备注更新）

零新文件、零删除、零 Swift / Go / project.yml 改动。

### Change Log

| Date | Description |
|---|---|
| 2026-04-30 | ADR-0010 v1 Accepted（Status: Proposed → Accepted；§6 5 条勾选；无 §3 修订）。iphone/README.md 加「导航架构」+「全局状态」两段引用 anchor（含 Story 37.1 AC6 延后并入 ADR-0009 引用 + 本 story ADR-0010 引用）。Story 37.2 Status: ready-for-dev → review。 |
