---
date: 2026-04-30
source_review: file:/tmp/epic-loop-review-37-2-r1.md (codex review --uncommitted, round 1, Story 37.2)
story: 37-2-adr-0010-appstate
commit: 55ae68c
lesson_count: 2
---

# Review Lessons — 2026-04-30 — 文档时态精确性 vs 路径 B ADR Accepted 语义（与 codex review 的天然张力）

## 背景

Story 37.2「ADR-0010 AppState（决策落地、非实装）」是一次纯文档 / 决策 artifact 改动 story：
1. 把 ADR-0010 Status 从 Proposed 改 Accepted，§6 的 5 条 verification checkbox 全部勾选
2. 在 iphone/README.md「Swift Package 依赖」段后插入「导航架构」+「全局状态」两段引用 anchor（履行 Story 37.1 AC6 延后承诺，引用 ADR-0009 + ADR-0010）

codex round 1 给了 2 条 P2 finding：（A）README 用现在时描述了未实装的架构；（B）ADR-0010 §6 第 2-4 条 checkbox 物理依赖 Story 37.4，但 37.4 当前还是 backlog。

本 story（与 Story 37.1 同样）显式选择**「路径 B」**：§6 字面 vs Story epic AC 字面冲突时，按 Sprint Change Proposal v2.5 终审 + architect/PM 契约级签字 = 物理验证的契约级等价物来解读，物理验证由下游实装 story 的 codex review 兜底。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | iphone/README.md「导航架构」/「全局状态」段用现在时描述未实装架构 | P2 / medium | docs | fix | `iphone/README.md:76-82` |
| 2 | ADR-0010 §6 第 2-4 条 verification checkbox 物理依赖 Story 37.4，37.4 仍 backlog | P2 / medium | architecture / docs | wontfix | `_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md:312-315` |

## Lesson 1: README 引用 anchor 段必须明示「目标态 / pending implementation」边界

- **Severity**: P2 / medium
- **Category**: docs
- **分诊**: fix
- **位置**: `iphone/README.md:76-82`

### 症状（Symptom）

README 在「Swift Package 依赖」段后插入了「导航架构」+「全局状态」两段，文字用了「主入口为 4 Tab...」「由全局 `AppState` 单实例持有...」这种**现在时**陈述句，把 ADR-0009 / ADR-0010 钦定的目标态写成了仓库当前状态。但 sprint-status.yaml 里 `37-3-rootview-maintabview-改造` / `37-4-appstate-实装-loadhome-迁移` 都还是 `backlog`，这两段在仓库当前 HEAD 实际上**不可执行**（无 `HomeContainerView` 类型 / 无 `AppState` 类型）。

### 根因（Root cause）

「ADR Accepted = 决策已 settle」与「Story 实装已 done = 代码已落地」在 BMAD 工作流里是**两层独立 lifecycle**。本 story（37.2）只负责前者，后者由 37.3 / 37.4 完成。但 README 是仓库**当前可读文档**，新读者打开仓库不会先看 sprint-status.yaml 区分这两层；如果 README 用现在时描述目标态，会把仍是 backlog 的实装当成已落地，引导读者去 grep 不存在的类型。

更深层：「ADR Accepted」语义本身允许 ADR 比代码先 land（这是 ADR-0008 v2 / 路径 B 的核心模式），但 README / docs 是**面向当前代码读者**的引用 anchor，必须显式标注「这是目标态、pending implementation、依赖 Story X 落地」边界，不能默认读者能通过上下文推出来。

### 修复（Fix）

`iphone/README.md` 「导航架构」/「全局状态」两段：
1. 段首加一行 `> ⚠️ **目标态文档（pending implementation）**` blockquote，明示「实装由 Story 37.3 / 37.4 完成」+「sprint-status.yaml 中 37-X 仍为 backlog」+「在 X 落地前，仓库内并不存在 `HomeContainerView` / `AppState` 类型」
2. 正文动词改未来时（「**将**为 4 Tab...」「**将**由全局 `AppState` 单实例持有...」「**将**在 Story 37.X 落地后 supersede」）
3. 保留 ADR Status: Accepted 字样不变（ADR Accepted 语义独立于实装状态）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **README / 长期面向读者的 docs 里加 ADR 引用 anchor，且对应实装 story 仍 backlog** 时，**必须**在引用段顶部加 `pending implementation` blockquote + 正文动词用**未来时**，而非用现在时把目标态写成已落地。
>
> **展开**：
> - blockquote 必须明示三件事：（i）这是目标态 / pending implementation；（ii）实装由哪个 story 完成；（iii）当前 sprint-status.yaml 里那个 story 是什么状态
> - 「supersede」陈述也要改未来时（「**将**在 Story X 落地后 supersede」），不能写成「已 supersede」——supersede 这种状态变更是**实装 + 测试通过**才算成立，不是 ADR Accepted 那一刻就成立
> - **反例**：Story 37.2 r1 提交把 README 写成「iPhone 主入口**为** 4 Tab...」「由全局 `AppState` 单实例持有...」「Story 2.3 主入口部分**已** supersede」——这三处都是错的，应该是「**将**为」/「**将**由」/「**将**在 Story 37.3 落地后 supersede」
> - **判定边界**：如果是 ADR 文档自身（`_bmad-output/.../decisions/00XX-*.md`），动词时态可以保留现在时（ADR 是决策语境，描述目标态是分内事）；如果是 README / 工程目录 docs / 测试入口文档，必须区分

## Lesson 2: ADR §6 verification checkbox 在路径 B 模式下不等于物理验证已完成

- **Severity**: P2 / medium
- **Category**: architecture / docs
- **分诊**: wontfix（**关键**：写明为何 review 结论站不住）
- **位置**: `_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md:312-315`

### 症状（Symptom）

ADR-0010 §6 5 条 verification checkbox 全部勾选。其中：
- 第 1 条「用户终审通过 Sprint Change Proposal v2」—— 物理已发生 ✅
- 第 5 条「codex 对 Sprint Change Proposal v2 verdict ≥ Accept with revisions」—— 物理已发生 ✅
- 第 2 / 3 / 4 条「Story 37.4 落地后跑 build.sh --test 通过 / AppStateTests.swift 含 ≥6 case / LoadHomeUseCase 集成测试改写」—— 物理**未**发生（37.4 仍 backlog）

codex 给出 P2 finding：「ADR 记录验证完成早于依赖的实装 story 存在 → false-success state → 实装 story 容易跳过 ADR 声称已发生的验证」。

### 根因（review 结论为何不成立）

codex 隐含的假设是「ADR §6 = 物理验证 checklist，必须实装完才能勾」。但本 story（与 Story 37.1 同样）显式选择**路径 B**，在仓库内已经形成**先例 + 钦定状态**：

1. **Story 37.2 Dev Notes 里 AC2 路径选择**：
   > 走**路径 B**（与 Story 37.1 / ADR-0009 仓库内一致）。§6 第 2-4 条作为 ADR-0010 contract validity 的外部前置依赖，已经在 Sprint Change Proposal v2.5 终审 + ADR-0010 §1.3 + §2 决策表 + §3.1-§3.7 详细落地步骤的 architect/PM 评估中**契约级**确认；物理验证由 Story 37.4 实装期 codex review 兜底；若届时发现偏差走「ADR 修订 patch + 改 v2 Accepted」路径。

2. **Story 37.1 codex round 1 已通过同样模式**：ADR-0009 §6 物理依赖 Story 37.3 build 通过，Story 37.1 同样 §6 全勾，codex round 1 verdict 通过。本 story 与 37.1 在 review 维度完全对偶。

3. **ADR-0008 v2 commit `ec5beb3` 提供「修订 patch」先例**：若届时（37.4 实装期）发现 ADR §3 设计不可行，走 ADR 修订 patch + 改 v2 Accepted 路径，§6 不需要回退。

4. **物理验证机制**：Story 37.4 自身的 codex review（dev → review → fix-review → done 流程）在 37.4 实装期**重新**对 §6 第 2-4 条做物理 verdict —— 这是兜底，不是回避。

5. **不修的代价 vs 修的代价**：
   - 不修 = 保留路径 B 钦定状态，下游实装 story 自然走 codex review 兜底；与仓库内 37.1 / ADR-0008 v2 先例完全一致
   - 修 = 取消 §6 第 2-4 条勾选 → 部分回退路径 B；但 ADR Status: Accepted 保留就出现「Accepted 但 §6 不全勾」的内部矛盾；同时与 37.1 codex round 1 已通过的先例打架，要么 37.1 也得回退，要么承认两 story 标准不一致

结论：codex 的 finding 在「ADR §6 = 物理 checklist」语义下成立，但在仓库内已经钦定的「路径 B = 契约级签字 = 验证等价物 + 下游 story 兜底」语义下不成立。

### 修复（Fix）

**不修**。理由见上「根因」节。Story 37.2 dev story 文件 Dev Notes / Completion Notes 已经显式记录路径 B 选择，sprint-status.yaml 中 `37-2-adr-0010-appstate` 状态为 `review`（fix-review 不动状态），路径 B 钦定状态保留。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **codex review 对 ADR §6 verification checkbox 给 finding，但 story Dev Notes / sprint-status / 仓库内有路径 B 先例（ADR Accepted 早于实装 story）** 时，**优先**判 wontfix 并在 lesson 记录路径 B 钦定状态 + 同类先例 commit 引用，而非盲目取消勾选。
>
> **展开**：
> - 路径 B 三要素：（i）story Dev Notes 显式选「路径 B」并记录契约级签字依据；（ii）仓库内有同类先例（如 Story 37.1 + ADR-0009）已通过 codex review；（iii）下游实装 story 的 codex review 是物理验证兜底
> - codex review 与路径 B 是**天然张力**关系，不是 codex 错也不是路径 B 错——codex 没有 story Dev Notes 上下文；路径 B 信任契约级签字。判 wontfix 时必须在 lesson 里**完整列出**路径 B 三要素 + 兜底机制 + 修的代价，让未来 Claude / 评审者能复盘
> - **反例**：盲目取消 §6 第 2-4 条勾选，破坏 Story 37.2 dev story 钦定的路径 B + 与 Story 37.1 先例打架，引发「同类 story 不同处理」的工作流不一致
> - **判定边界**：如果 story Dev Notes 里**没有**显式选路径 B / 仓库**没有**同类先例，就该判 fix 取消勾选；路径 B 不是默认状态，必须由 story 显式声明

---

## Meta: 本次 review 的宏观教训

两条 finding 共同指向一个深层问题：**ADR Accepted lifecycle vs Story Done lifecycle vs Repo HEAD readable state 是三个相互独立的层**。

- ADR Accepted = 决策契约 settle（路径 B 允许早于实装）
- Story Done = 实装代码 + 测试 land
- Repo HEAD readable state = 当前读者打开仓库看到的状态

三者错位时（如 ADR Accepted 早于 Story Done），文档需要显式标注边界：
- ADR 文档自身（`_bmad-output/.../decisions/`）：可以用现在时描述目标态，因为 ADR 是决策语境
- README / 工程入口 docs / 长期 docs：必须用未来时 + `pending implementation` blockquote 标注边界
- §6 verification checkbox：如果 story 选路径 B，可以勾（契约级签字）；但下游实装 story codex review 是兜底物理验证，不能跳

这次 codex review 帮助暴露了「README 引用 anchor 段在路径 B 模式下需要时态校准」这个工作流盲点——以前路径 B 只在 ADR 文档自身用过，没在 README 上下文用过；本 story 是首次把路径 B 的影响传播到 README 引用 anchor，需要建立配套的「README pending implementation 标注规范」。

---

## Round 2 outcome（2026-04-30 后续追加）

codex review round 2 重申了本文 Lesson 2 的同一 finding（ADR-0010 §6 第 2-4 条 checkbox），论点更加强："README 已正确改未来时 + sprint-status 仍标 backlog → §6 全勾形成 internally inconsistent state → false-success signal 让 Story 37.4 跳过验证"。

round 2 fix-review 没有反转路径 B 决策，而是采用 **option C**：保留 §6 checkbox 全勾不动（路径 B 钦定 + 与 Story 37.1 先例对齐），在 §6 段顶加 inline blockquote annotation 把本 lesson Lesson 2 的核心论证（契约级签字 = 验证等价物 + 下游 codex review 兜底 + ADR-0008 v2 修订路径）就地 forward 到 ADR 现场。详见后续 lesson 文件 `docs/lessons/2026-04-30-adr-section-6-path-b-inline-semantics.md`。

**对本 lesson Lesson 2「预防规则」的修订**：判 wontfix 时**不应仅靠 lesson sink** —— codex review 上下文只看 baseline..HEAD diff，看不到 lesson 文件。路径 B 类 wontfix 应当**同时**做两件事：（i）lesson 归档（未来 Claude 学习材料）；（ii）ADR / 文档现场 inline annotation（当前 + 未来 reviewer 就地语义说明）。round 1 漏了 (ii) 导致 round 2 复发；未来路径 B 类 wontfix 一开始就该同时落地这两份产物，不等 round 2 复发再补。
