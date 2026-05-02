---
date: 2026-05-02
source_review: codex review r9 (epic-loop sub-agent) of Story 7.1 — /tmp/epic-loop-review-7-1-r9.md
story: 7-1-接口契约最终化
commit: 030647f
lesson_count: 2
---

# Review Lessons — 2026-05-02 — Story file 内部规则副本必须通过"标准答案表"全文核对（不能只看 review 指出的两条）

## 背景

Story 7.1（节点 3 接口契约定稿）已经走完 8 轮 codex review + 8 轮 fix-review，每轮 codex 都精准命中 2 条新 P2，全是"V1 文档某条规则改了，但其他副本（story file / epics.md / 时序图 / 数据库设计 / lessons）没同步"。每轮 sub-agent 都声称扫干净了，但下一轮还是被命中。

r9 codex review 又命中两条 P2，都在 story file `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md` 内部：

1. AC5（line 269）："单日累计达 50000：当次 delta = 0 + 3001" 还是旧"已达上限"语义，没改成 V1 §6.1 已定的 `prevAccepted + curDelta > 50000`（入账后越界判断）
2. Dev Notes 配置层（line 437-439）："运行时覆盖默认值不视为契约变更" 还是旧"YAML 可调"语义，没改成 V1 §1 已定的"prod 必须默认值，dev/test 可覆盖"

根因是：r1~r8 sub-agent 都只局部修了 codex 当轮指出的两条，**没有**把 story file 当成"V1 文档片段的全副本仓库"做系统性 sweep。story file 内部 AC1-AC11 + Dev Notes + 配置层 + Refactor Guidelines 等十几个章节都是 V1 文档片段的副本，每改一次 V1 主文档就有 N 处副本要同步，**而 codex 每轮抽样命中 2 处**——不彻底扫就永远扫不干净。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | AC5 line 269 单日封顶规则未同步 V1 `prevAccepted + curDelta > 50000` 新语义 | P2 | docs | fix | `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md:269` |
| 2 | Dev Notes 配置层 line 437-439 阈值环境约束未同步 V1 §1 prod-locked 新规则 | P2 | docs | fix | `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md:437-439` |
| 3 (额外发现) | AC2.4 服务端逻辑 line 116 「阈值通过配置文件可调」也是旧语义 | P2 | docs | fix | `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md:116` |
| 4 (额外发现) | Dev Notes line 19 Story 7.3 增量 AC 摘要 也是旧语义（缺 prevAccepted+curDelta>50000 + 缺 prod 不可覆盖） | P2 | docs | fix | `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md:19` |

注：findings 1+2 是 codex 直接指出的；3+4 是按 override #3 要求做"标准答案表"全文 sweep 时主动扫到的——证明 codex 即使精准也只是抽样命中，sub-agent 必须做完整 sweep 才能避免下一轮再被打脸。

## Lesson 1: 大型文档 story 必须先建"标准答案表"再做 fix-review

- **Severity**: P2（每轮都漏 → 累计 9 轮 review 浪费）
- **Category**: docs / process
- **分诊**: fix（本轮 + 沉淀方法）
- **位置**: 整个 fix-review workflow

### 症状（Symptom）

Story 7.1 连续 9 轮 codex review，每轮都命中 2 条新 P2，全是 V1 文档某规则的副本（story file 内部 / epics.md / lessons / 时序图等）未同步。每轮 sub-agent 局部修后都声称扫干净，下一轮 codex 又精准命中其他位置。

### 根因（Root cause）

Sub-agent 没区分"V1 主文档"与"副本载体"：
- V1 主文档（`docs/宠物互动App_V1接口设计.md`）是 single source of truth
- 但 story file（AC1-AC11 + Dev Notes + 配置层 + Refactor Guidelines + 测试策略）内部有十几处规则副本，每条副本都是 V1 文档的拷贝
- epics.md / 时序图 / 数据库设计 / lessons 也都有副本
- Sub-agent 把 review findings 当作"to-do list"逐条修，而不是当作"抽样命中"反推应该做全文 sweep

每轮 codex 模型抽样命中位置随机，导致同样一组规则在多轮 review 里被多次命中（因为副本太多，单轮抽不完）。

### 修复（Fix）

本轮（r9）修复：
1. AC5 line 269 改为含 `prevAccepted + curDelta > 50000` 公式 + 例（prev=49000+cur=4000 应被拒）+ 3001 非粘性说明
2. AC5 line 266-267 阈值描述补 prod 必须默认值 / dev/test 可覆盖
3. AC2.4 line 116「阈值通过配置文件可调」改为 prod 必须默认值不可覆盖 / dev/test 可覆盖
4. Dev Notes line 437-439 配置层同改 prod-locked 语义
5. Dev Notes line 19 Story 7.3 增量 AC 摘要按新语义同步

修复方法是先建一张"标准答案表"（10 条规则的最终版描述）→ 用每条规则的关键 token 全文 grep → 命中位置逐一核对 → 修复后用同一张表反向 grep 验证 0 残留。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在做**纯文档型 story 的第 N 轮 fix-review（N ≥ 2）**时，**必须**先建一张"标准答案表"（列出主文档当前最终版的所有关键规则 + 关键 token），然后用每条规则的 token 在所有可能载有副本的文件做 **grep 全文 sweep**，**禁止**只修 review 当轮指出的位置。
>
> **展开**：
> - **强制动作**：fix-review 第 1 步不是"看 codex 指出哪两条"，而是"读 V1 主文档对应章节 → 提炼标准答案表 → grep 全文 sweep"
> - **标准答案表格式**：编号 / 规则一句话 / 旧 token（要消除）/ 新 token（应出现）/ 关联载体文件清单
> - **载体文件清单**（Story 7.1 这种契约 story 的副本载体）：
>   - story file 自身 AC1-ACN + Dev Notes + 配置层 + Refactor Guidelines + 各种"摘要"段
>   - `_bmad-output/planning-artifacts/epics.md` 对应 Story 段
>   - `docs/宠物互动App_*.md` 其他 7 份文档（数据库设计 / 时序图 / Go 项目结构 / iOS 项目结构等）
>   - `docs/lessons/` 下任何引用该规则的 lesson（注意区分"反例"是 OK 的，"正例"是错的）
> - **修复后强制**：再用同一张表反向 grep 一次（旧 token 0 命中 + 新 token 应有位置全命中），把 grep 命令 + 命中行数写进 fix-review 报告**证明**无残留
> - **反例**：以为"codex 这轮就指了两条 P2，修完这两条就行"——实际是 codex 抽样命中，story file 还有十几处副本未扫
> - **反例**：以为"V1 主文档自洽 = 整个 story 自洽"——story file 是 V1 文档片段的拷贝池，必须独立按标准答案表核对

## Lesson 2: 跨文档规则同步的"载体收敛清单"应该作为 fix-review 启动条件

- **Severity**: P2
- **Category**: process / docs
- **分诊**: fix（沉淀流程改进）

### 症状（Symptom）

Story 7.1 r1 修了 V1 文档的"3001 触发条件"，r2 修 user_id 限频 scope，r3 修 source enum，r4 修 step_account 数值不变量，r5/r6/r7/r8 各修不同副本。每次主文档修订都需要扫"哪些载体有副本"，但每个 sub-agent 重新建一遍载体清单（context 不共享），结果每次都扫漏一两个载体。

### 根因（Root cause）

Sub-agent 没有"载体清单"作为输入——每个 sub-agent 自己临时枚举一遍"可能的副本位置"，枚举的覆盖度依赖当时的 attention 状态，不稳定。

### 修复（Fix）

把"载体清单"作为 lesson 文档的固定 section，未来 fix-review 启动时先读这份 lesson 拿到清单。

载体清单（针对契约类 story，如 Story 4.1 / 7.1 / 11.1 / 14.1 / 17.1 等）：

1. **主文档**：`docs/宠物互动App_V1接口设计.md`
2. **Story file 自身**：`_bmad-output/implementation-artifacts/<story>.md`
   - AC1 ~ ACN 每个 acceptance criterion
   - Dev Notes / 关键设计原则 / 架构对齐 / 配置层 / 测试策略 / Project Structure Notes / Change Log
   - Tasks/Subtasks（task 描述里也有规则副本）
3. **Planning artifact**：`_bmad-output/planning-artifacts/epics.md` 对应 Story 段（Story X.Y AC + Story X.(Y+1) 的"参考 Story X.Y AC"段）
4. **其他 7 份 docs**：
   - `docs/宠物互动App_总体架构设计.md`
   - `docs/宠物互动App_MVP节点规划与里程碑.md`
   - `docs/宠物互动App_数据库设计.md`
   - `docs/宠物互动App_时序图与核心业务流程设计.md`
   - `docs/宠物互动App_Go项目结构与模块职责设计.md`
   - `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md`
5. **Lessons 目录**：`docs/lessons/<date>-*.md`（特别是同一 story 的历史 lesson，里面常有规则副本作为"修复内容"段）
6. **ADR 目录**：`_bmad-output/implementation-artifacts/decisions/<NNNN>-*.md`（如果该规则被 ADR 锚定）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 启动 **fix-review N（N ≥ 2）轮的契约文档型 story** 时，**必须**先把上面"载体清单"6 类文件全部 grep 一遍旧 token + 新 token，**禁止**只 grep review 直接命中的位置。
>
> **展开**：
> - **强制动作**：fix-review 第 1 步把载体清单的 6 类文件全部传入 Grep tool，用旧 token / 新 token 各 grep 一遍，把命中表打印出来
> - **判别工具**：sub-agent 在汇报修复完成时必须列出"复扫 grep 命令 + 各文件命中行数"，证明 6 类文件都已扫过且 0 旧 token 残留
> - **反例**：sub-agent 只扫了 review 直接指出的文件，没扫 lessons / ADR / epics.md / 其他 docs/*.md
> - **反例**：sub-agent 扫了所有载体但只用旧 token grep，没用新 token grep 验证应有位置都命中（漏掉"应有但缺失"的副本）

---

## Meta: 本次 review 的宏观教训

Story 7.1 用了 9 轮 review 才稳定（如果本轮是最后一轮的话），相比 Story 4.1 的 5 轮多了 4 轮，根因是 7.1 的 V1 主文档变更点更多（GAP E + GAP K + 3001 非粘性 + 限频 scope + source enum + 数值不变量 + prod-locked 阈值 7 个变更点），每个变更点都辐射多个副本。

**结构性教训**：契约文档型 story 的 review 收敛轮次 ≈ V1 主文档变更点数（因为每个变更点至少需要 1 轮发现 + 1 轮主修 + 1 轮残留扫尾）。要把这个轮次降下来，**必须**在 fix-review 流程里强制"标准答案表 + 载体全 sweep"机制，否则永远是"打地鼠"。

未来类似 story（11.1 节点 4 房间契约 / 14.1 节点 5 宠物状态契约 / 17.1 节点 6 表情契约）应在 dev-story 阶段就建好"标准答案表 + 载体清单"作为产物，避免 review 阶段反复重建。
