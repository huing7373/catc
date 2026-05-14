---
date: 2026-05-14
source_review: codex review round 14 for story 20-1 (/tmp/epic-loop-review-20-1-r14.md)
story: 20-1-接口契约最终化
commit: c25b4b4
lesson_count: 2
---

# Review Lessons — 2026-05-14 — 设计文档迭代时 stale 快照应"删除即删除"而非"super-note 警告并保留"

## 背景

Story 20.1 经历 r1 ~ r13 共 13 轮 fix-review，把 chest_open 幂等设计从 r1 "Redis 幂等键 + TTL 24h + 事务后写 Redis + 1008 主动返回" 推翻并锁定为 r11 "MySQL `chest_open_idempotency_records` 表 + 业务事务内同事务原子写 + 二态机 + 1008 退役"。r12 修复时为了保留迭代轨迹，在 AC3 入口插入了"r11 super-note 警告"但**保留了 200+ 行 r1 Redis 设计快照**。r14 codex review 认为 super-note 不够强：下游 reader（Story 20.5 / 20.6 / 20.9 / iOS Epic 21.x 实装者）有概率绕过 super-note 直接照 AC3 内嵌文本踩坑，把 Redis 路径实装出来。同时 AC4 §1 冻结声明引用段中 `nextChest.status` 描述仍是"固定 status=1"，未跟上 r13 锁定的 time-derived 语义。两条 P2 都需修。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | AC3 内嵌 r1 Redis 快照即便有 super-note 仍可能被下游照搬实装 | P2 | docs | fix | `_bmad-output/implementation-artifacts/20-1-接口契约最终化.md` |
| 2 | AC4 §1 冻结声明 `nextChest.status` 描述 stale，未跟上 r13 time-derived 语义 | P2 | docs | fix | `_bmad-output/implementation-artifacts/20-1-接口契约最终化.md` |

## Lesson 1: 长篇 stale 快照应"删除即删除"，super-note 警告档不住下游照搬

- **Severity**: P2
- **Category**: docs
- **分诊**: fix
- **位置**: `_bmad-output/implementation-artifacts/20-1-接口契约最终化.md:210-402`（r14 修订前）

### 症状（Symptom）

r12 修复时为了"保留迭代轨迹 + 便于事后审计"，AC3 块入口添加了一段醒目的 super-note：

> ⚠️ AC3 r11 super-note（2026-05-14 锁定）：本 AC3 内联引用的 V1 §7.2 文本是 r1 初版的 Redis 幂等设计，已被推翻 …… 真正生效以 V1 文档当前版本为准 …… **本 AC3 内联文本仅作为 r1 历史快照保留** …… 禁止按本 AC3 内联快照的 r1 Redis 设计实装。

但 super-note 之后**完整保留**了 200+ 行 r1 设计（§7.2.1 ~ §7.2.6 六大块：接口元信息 / 请求体 / 服务端逻辑 / 响应体 / 错误码 / 关键约束），其中包括：

- "Redis 幂等键 `idem:{userId}:chest_open:{idempotencyKey}` + TTL 24h"
- "事务后写 Redis"
- "1008 主动返回"
- "三态机 `('pending', 'success', 'failed')`"
- "middleware 层 rate_limit"

codex r14 指出：下游 reader（特别是 LLM 实装 agent）有显著概率跳过 super-note 警告框直接 grep 字段定义 / 步骤描述，照内嵌长文实装出错误设计。

### 根因（Root cause）

**"保留历史快照 + 加 super-note 警告"在迭代轨迹保护层面看起来合理，但忽略了三个真实使用模式**：

1. **下游 reader 的扫读习惯**：实装者 / 实装 LLM 看 story 文件时倾向于 grep "schema" / "字段表" / "服务端逻辑" 直接定位实装契约，不会逐字读入口的警告框 —— super-note 在视觉上是"导览段"，实质内容才被认为是契约
2. **LLM 注意力分布**：警告框（"⚠️"）即便强调"禁止照此实装"，长篇技术内容（具体 SQL 语句、字段类型、错误码表）的"专业性"和"完整性"会盖过抽象警告 —— LLM 会被实在的代码 / SQL 吸引而非元层警告
3. **跨轮迭代后警告强度衰减**：r12 super-note 是当时 r1 推翻后的强警告；但随着 r13 / r14 新增内容，super-note 在 AC3 内嵌长文的"喧宾夺主"格式下变成"无关注脚"，警告强度在视觉权重上被稀释

**r12 lesson `2026-05-14-story-file-must-track-canonical-contract-drift-20-1-r12.md` 已部分提到这点**，但只钦定了"completion notes / change log 必须跟踪契约漂移"，未明确"AC 主体段的 stale 快照该删而不该警告保留"。r14 是该 lesson 的补强落地。

### 修复（Fix）

将 AC3 块（行 210-402，共 ~193 行）整体替换为：

1. **AC3 内容声明**（新版，~3 行）：明确声明"原 r1 Redis 快照已 r14 完整删除"+ 解释为何删除（super-note 档不住 grep + LLM 注意力）+ 钦定权威源为 V1 文档当前版本 + DB §5.16
2. **结构骨架**（H4 块标题与顺序，~5 行）：保留 §7.2.1 ~ §7.2.6 六大块名 + 每块一句话功能说明，但**不**复现块内字段表 / SQL / 错误码表（这些以 V1 文档为权威）
3. **r11 钦定服务端逻辑步骤简述**（步骤 3 / 4 / 5 关键路径，~10 行 bullet）：突出"幂等命中预检 → rate_limit → 业务事务原子提交"三段式骨架
4. **`response_json` 缓存内容钦定**（~4 行）：明确钦定不写入的字段（`nextChest.status` / `nextChest.remainingSeconds` / 顶层 `requestId`）+ time-derived 语义
5. **关键约束集合摘要**（~10 行）：r5~r11 七轮决策一句话摘要 + reference 锚点
6. **JSON 示例**（节点 7 阶段，~25 行）：保留示例值；新增"注：`nextChest.status` 在首次成功路径返回 1；同 key cached replay 时 server 端按 time-derived 实时计算"末注
7. **Reference 段**（~10 行）：链接到 V1 §7.2 / DB §5.16 / §8.3 + r3~r12 各轮 lesson 文件路径
8. **下游实装绝对禁令**（~3 行）：穷举列举 deprecated 设计变体（Redis 幂等键 / TTL 缓存 / 1008 主动返回 / 三态机 / middleware 层 rate_limit / `response_json` 含动态字段），明示禁止按任何旧版本实装

修改前后对比：
- 修改前 AC3 长度：~193 行（super-note 14 行 + r1 内嵌六大块 179 行）
- 修改后 AC3 长度：~80 行（摘要 + reference + 禁令，无内嵌长文）
- 删除净行数：~113 行
- 下游 reader 注意力路径：从"读警告 → 跳过警告 → 照内嵌长文实装"变为"读摘要 → 看 Reference → 跳到 V1 文档当前版本读"

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **维护多轮迭代的设计文档（PRD / API 契约 / DB schema / story AC）** 时，若某段长篇技术内容被后续轮次推翻 / 替换，**必须**采取"删除即删除"策略 —— **删除整段 stale 内容**并替换为"权威源 reference + 摘要 + 禁令"三件套，**禁止**用"super-note 警告 + 保留 stale 长文"的折中方案。
>
> **展开**：
> - **删除而非警告**：长篇技术内容（>50 行 SQL / 字段表 / 步骤描述）一旦被推翻，**完整删除**；不在原位用 super-note / collapsible / blockquote 等"软警告"保留。原因：下游 reader / LLM 看长文倾向于扫读具体内容（字段名 / SQL / code），警告框的元信息会被实质技术内容盖过；警告强度随后续轮次衰减
> - **历史轨迹另寻位置保留**：迭代历史 / 推翻决策 / round-by-round commit 链应保留在 **Change Log / Completion Notes / 单独的 r{N} lesson 文件** 中，不在主体段保留。这三个位置是 reader 明确为"历史档案"而开的视角，不会被误读为"当前契约"
> - **替换为"摘要 + reference + 禁令"三件套**：
>   - **摘要**（≤30 行）：用 bullet / 简表覆盖关键约束骨架，足够下游 reader 建立认知，但不至于详细到可以照搬实装
>   - **Reference 锚点**：链接到契约权威源文档（API 设计 / DB schema / 配置 schema）的当前版本路径 + 章节号 + 相关 lesson 文件路径
>   - **禁令段**：穷举列举 deprecated 设计变体，明示"禁止按任何旧版本实装"
> - **跨文档同步检查**：契约演进时，主体设计文档之外的**所有内嵌引用副本**（story AC / epic 文档 / 实装计划 / agent prompt）都需要同步检查，避免"主文档已演进，副本仍 stale"。可用 `grep` 反向定位副本（如 search 关键术语 "Redis 幂等键" / "TTL 24h"）
> - **反例**：r12 修复 r1 推翻时采用了"AC3 入口加 super-note + 保留 200+ 行 r1 内嵌长文"的方案。表面看保留了迭代轨迹，但 r14 codex review 证明：下游 reader 仍有显著概率绕过 super-note 照搬实装。这是典型的"为了仪式感 / 文档完整性牺牲实用性"反例 —— 正确做法是删除内嵌长文，把迭代轨迹放到 Change Log + lesson 文件
> - **反例 2**：在主体契约段用 `<details>` / 折叠块包裹 stale 内容也**不**推荐 —— 折叠块在 markdown 渲染时仍是可展开的"上下文"，LLM agent 通常会展开读取，与不折叠等价

## Lesson 2: 跨文档引用段必须随契约演进同步更新，stale 引用比 stale 主文档更危险

- **Severity**: P2
- **Category**: docs
- **分诊**: fix
- **位置**: `_bmad-output/implementation-artifacts/20-1-接口契约最终化.md:409-410`（AC4 §1 冻结声明引用段）

### 症状（Symptom）

V1 文档 §1 冻结声明在 r13 已更新为 time-derived 语义：

> `nextChest` 永远非 null 且 server 端 `unlock_at = now + 10min`、`nextChest.status` 首次返回 1 但同 key 重试时 server 端按 `(unlock_at > now) ? 1 : 2` 实时计算（**time-derived**，与 §7.1 GET /chest/current 同语义，详见 §7.2 关键约束 r9 / r11 锁定段）

但 story 文件 AC4 §1 冻结声明的引用段仍写：

> `nextChest` 永远非 null 且 server 端固定 status=1 / unlock_at=now+10min

引用段比权威源**落后两轮**（r9 锁定 time-derived，r11 锁定 cached replay 时同源同时刻计算，r13 落到 V1 文档冻结声明）。下游 reader 如果照 AC4 引用段 review 实装，会把 r9 / r11 锁定的 time-derived 行为视为"契约违反"。

### 根因（Root cause）

**story AC 中的"V1 文档引用段"是契约的副本，副本与权威源之间没有自动同步机制**。具体表现：

1. **副本初版与权威源同步**：r12 把 AC4 引用段从原"V1 §1 节点 7 冻结声明文本"的快照贴入 story 文件
2. **权威源随后演进**：r13 在 V1 §1 冻结声明中把"固定 status=1"改为"time-derived"，但**修改作用域仅限 V1 文档本身**
3. **副本未跟进**：story 文件 AC4 §1 引用段没有任何"指向权威源版本"的机制（无 git hash 引用、无 source pointer / no diff 监测），r13 修订时未触发副本同步
4. **fix-review 工具链未覆盖跨文档检查**：Codex / Claude 在 r13 fix-review 时主要关注 V1 文档内部一致性，story 文件 AC 段被视为"已 review 过"而未重检

这是经典的"**契约文档 fan-out 同步缺失**" —— 主文档演进，扇出的副本（AC / epic / 实装计划 / agent prompt）未跟进。

### 修复（Fix）

把 AC4 §1 冻结声明引用段中关于 `nextChest.status` 的描述同步到 r11/r13 钦定的 time-derived 语义。具体变更：

- **删除**："`nextChest` 永远非 null 且 server 端固定 status=1 / unlock_at=now+10min"
- **替换为**："`nextChest` 永远非 null 且 server 端 `unlock_at = now + 10min`、`nextChest.status` **首次返回 1 但同 key 重试时 server 端按 `(unlock_at > now) ? 1 : 2` 实时计算**（**time-derived**，与 §7.1 GET /chest/current 同语义；`nextChest.remainingSeconds` 始终按 `max(0, ceil((unlock_at - now) / 1s))` 实时计算；两者均不写入 `response_json` 缓存，由 server 在响应序列化时同源同时刻填入；详见 V1 §7.2 关键约束 r9 / r11 锁定段）"
- **新增**：契约变更触发列表加入"把 `nextChest.status` / `remainingSeconds` / `requestId` 等动态字段写回 `response_json` 缓存"

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **更新契约权威源文档（API 设计 / DB schema / 配置 schema）** 时，**必须**用 grep 反向定位**所有内嵌引用副本**（story AC / epic 文档 / 实装计划 / agent prompt / README / migration 注释），逐处检查并同步；**禁止**仅修改权威源就 commit。
>
> **展开**：
> - **副本反向定位 protocol**：契约修订前用 grep 把"关键术语 / 字段名 / 旧 / 新值"全仓搜索，列出所有副本位置。例如改 `nextChest.status` 语义时 grep：
>   - `grep -rn "nextChest.status" --include="*.md"`
>   - `grep -rn "固定 status=1" --include="*.md"`
>   - `grep -rn "status=1.*unlock_at" --include="*.md"`
> - **修订后回扫**：修订权威源 commit 前再 grep 一次确认所有副本都已同步；任何剩余的旧值都是 stale 副本
> - **副本最佳实践 — 摘要 + 锚点 vs 全文复制**：story AC 等内嵌引用副本应采用"摘要 + 锚点"形式（"详见 V1 §7.2 关键约束 r9 / r11 锁定段"），减少全文复制 → 减少 fan-out 同步成本。如必须复制全文（如 r12 / r14 修订的 AC4 §1 冻结声明），副本应在文末标注 source pointer（"以下文本同步自 V1 §1 冻结声明，最后同步轮次：r13；如有冲突以 V1 文档为准"）
> - **fix-review tool 提醒**：每轮 fix-review 后，**必须**做一次跨文档一致性回扫 —— 不仅检查权威源是否正确，也检查所有副本是否跟进。这是 r14 review 暴露的盲点：r9 / r10 / r11 / r12 / r13 五轮 review 都关注 V1 文档内部一致性，但 r14 才发现 story 文件副本落后
> - **反例**：r13 修订 V1 §1 冻结声明 `nextChest.status` 从 "固定 1" → "time-derived" 时，未同步 story 文件 AC4 §1 引用段。一周后 r14 review 才发现 → 下游实装如果照 story AC review 会把 time-derived 行为误判为契约违反，可能错误回滚正确实装

---

## Meta: 本次 review 的宏观教训

**13 轮 fix-review 暴露的一个共性盲点：**"契约文档的内嵌副本 / stale 快照管理"**。**

13 轮迭代中至少有 4 轮 lesson 与"文档同步"相关：
- r8 (`design-doc-cross-section-summary-must-track-canonical-20-1-r8.md`)：跨章节 summary 必须跟踪权威源
- r12 (`story-file-must-track-canonical-contract-drift-20-1-r12.md`)：story 文件必须跟踪契约漂移
- r14 lesson 1（本文件）：长篇 stale 快照应"删除即删除"而非"super-note 警告并保留"
- r14 lesson 2（本文件）：跨文档引用段必须随契约演进同步更新

**根本原因**：迭代型设计（r1 → r2 → … → r{N}）天然产出"历史 vs 当前 vs 副本"三种状态，缺少明确的纪律会让三者交叉污染。

**沉淀规则**（蒸馏给未来 Claude）：

1. **历史**应在 **Change Log + Completion Notes + lesson 文件** 中保留，明确标注"已推翻 / 已废弃 / r{N} 修订"
2. **当前**应**唯一**存在于权威源文档（V1 接口设计 / DB 设计 / 配置 schema），副本 / 摘要 / 引用段都要指向权威源
3. **副本**应采用"摘要 + 锚点"形式，避免全文复制；如必须全文复制需标注 source pointer + 最后同步轮次
4. **stale 长文不保留**：被推翻的长篇技术内容（>50 行）一旦确认 stale，**删除即删除**，禁止用 super-note / collapsible / blockquote 等"软警告"保留
5. **fix-review 跨文档回扫**：每轮 fix-review 后必须做一次"权威源 → 副本"反向同步检查；不仅检查主文档内部一致性
