---
date: 2026-05-14
source_review: codex review round 12 of story 20-1（/tmp/epic-loop-review-20-1-r12.md）
story: 20-1-接口契约最终化
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-14 — Story 文件自身需 in-flight 同步追踪契约迭代（20-1 r12）

## 背景

Story 20-1（接口契约最终化）经历 r1 dev-story + r3 ~ r11 共 11 轮 fix-review，每轮迭代都在修订 V1 接口设计文档（§7.2 服务端逻辑 + 关键约束）+ 数据库设计文档（§5.16 chest_open_idempotency_records 表 schema + §8.3 开箱事务边界）。**但 story 文件本身**（`_bmad-output/implementation-artifacts/20-1-接口契约最终化.md`）的 Tasks 段 / AC 段 / Dev Agent Record (Completion Notes / File List / Change Log) 自 r1 dev-story 落地后**没有同步迭代**，停留在 r1 初版的 Redis 幂等设计描述（"Redis 幂等键 `idem:{userId}:chest_open:{idempotencyKey}` + TTL 24h + 事务后写 Redis + 1008 主动返回"）。r12 codex review 锁定这一漂移：

> The new Story 20.1 artifact still tells downstream work to implement the pre-r5 Redis design ... but the V1/DB docs in this same patch switch the frozen contract to `chest_open_idempotency_records` in the business transaction and explicitly retire 1008 for this endpoint. Because Story 20.6/20.9 are supposed to implement from this file, leaving these sections stale will send follow-up implementation toward a different idempotency model than the one you just finalized.

第二条 P3 finding：AC7.5 + Completion Notes 中的"git diff 范围检查"自校验断言"diff 只触及 V1 接口设计文档 + sprint-status + 本 story 文件"，与 11 轮 fix-review 实际累计的 diff 范围（含数据库设计文档 + lessons 系列 + index.md）严重失真，让 story 自身的 scope 验证段成为假陈述。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Story 20.1 文件本身 stale — Tasks / AC / Dev Agent Record 仍描述 r5 前的 Redis 幂等设计 | high | docs / architecture | fix | `_bmad-output/implementation-artifacts/20-1-接口契约最终化.md`（多处）|
| 2 | AC7.5 / Completion Notes 中"diff 范围"自校验失真（实际改动含 DB 设计文档 + lessons/*）| low | docs | fix | `_bmad-output/implementation-artifacts/20-1-接口契约最终化.md:478` |

## Lesson 1: Story 文件自身是契约权威之一，必须随每轮 fix-review 同步迭代

- **Severity**: high
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `_bmad-output/implementation-artifacts/20-1-接口契约最终化.md`（Story 段 line 10-11 / 故事定位 line 29 / 本 story 不做 line 49 / line 64 / Tasks Task 3 line 462 / AC3 super-note 入口 / AC4 §1 冻结声明引用 / Dev Notes 错误码引用 line 512 / Dev Notes 跨文档同步检查 line 505 / Completion Notes / File List / Change Log）

### 症状（Symptom）

Story 文件中的 Tasks / AC / Dev Agent Record 描述的是 r1 初版 contract（Redis 幂等键 + TTL 24h + 事务后写 Redis + 1008 主动返回），但 V1 文档 §7.2 + 数据库设计文档 §5.16 经过 11 轮 fix-review 已切换到完全不同的设计（MySQL `chest_open_idempotency_records` 表 + 业务事务内同事务原子写 + 二态机 `('pending', 'success')` + 1008 在本接口退役）。下游 Story 20.6（POST /chest/open 事务实装）/ Story 20.9（Layer 2 集成测试）/ iOS Epic 21.x 是按 story 文件读契约的实装 SOP —— 留 stale story 内容会让下游按 Redis 设计走错方向。

### 根因（Root cause）

**"canonical 契约文档（V1 / DB）随 fix-review 滚动迭代，但 story 文件停留在 dev-story 落地版本"的偏差结构**。具体触发链：

1. `/bmad-dev-story` 流程产出 story 文件时把 Tasks / AC / Dev Agent Record 三段一次性填到 dev-story 完成时刻的契约理解上
2. 后续 `/fix-review` 流程的 SKILL 里**没有显式条款**要求 "review 触及的契约 + 必须把 story 文件的契约描述一起同步" —— SKILL 默认认为 fix-review 修的是 canonical 文档（V1 / DB），不修 story 文件
3. 跨多轮迭代（11 轮）累积漂移到完全失真状态
4. r12 review 显式 surface 这一漂移之前，没有任何自动校验机制兜底（lesson 文档 + Change Log + commit message 都没有强制要求"story 文件必须反映当前契约状态"）

更深层根因：**story 文件在 BMAD 流程中承担双重角色** —— 既是 dev-story 落地后的归档，又是下游 story 实装的"契约入口"。当 fix-review 修改契约后，第二个角色失效但第一个角色仍存在，导致下游 agent 读到的是"过时的契约入口 + 仍在 review 状态的 sprint 标记"组合，错误概率极高。

### 修复（Fix）

在 `_bmad-output/implementation-artifacts/20-1-接口契约最终化.md` 中做以下精准修订：

1. **Story 段 User Story 文本**（line 10-11）：把"错误码 1001 / 1002 / 1005 / 1008 / 1009 ..."更新为"1001 / 1002 / 1005 / 1009"且加上 r11 锁定的"1008 已退役"标注；把"idempotencyKey 字符集 / 长度约束 / Redis TTL 语义"改为"MySQL `chest_open_idempotency_records` 同事务持久化幂等语义（r5 ~ r11 11 轮 fix-review 锁定）"；"1008 幂等冲突语义与 Redis 缓存命中行为关系不清楚"改为"幂等存储介质（Redis cache vs MySQL 同事务持久化）/ pending 阶段同 key 重试行为 / cached success replay 是否计入 rate_limit 配额 / 1008 是否在本接口实际触发 不清楚"
2. **故事定位 Story 20.6 下游依赖描述**（line 29）：把 "先查 Redis 幂等键 → 命中直接返回 cache；未命中开事务" 替换为 r11 锁定的完整步骤 3 / 步骤 4 / 步骤 5a / 步骤 5k 描述（autocommit committed success 预检 + MySQL `chest_open_idempotency_records` 同事务原子写 + X-lock 阻塞同 key 并发 + 二态机 + 1008 退役）
3. **本 story 不做范围红线**（line 49 / line 64）：把 "Redis 幂等键代码" / "Redis 幂等键缓存命中行为" 改为 "MySQL 同事务持久化幂等记录代码" / "MySQL 同事务幂等记录命中行为 + cached success replay 免 rate_limit 配额 + pending 阶段同 key 重试走 X-lock 排队 + 1008 在本接口退役"
4. **AC3 入口添加 r11 super-note**：在 AC3 §7.2 schema 锚定段开头插入显著警告，说明本 AC3 内联文本是 r1 历史快照（Redis 设计），r11 已推翻为 MySQL 同事务持久化设计；下游实装必须以 V1 文档当前版本为准，禁止按 AC3 内联快照实装
5. **AC4 §1 冻结声明引用**（line 408 → 新版）：把"Redis 幂等键命中行为"改为"MySQL `chest_open_idempotency_records` 同事务持久化幂等语义（r5~r11 锁定，移除 Redis）"；把契约变更条件中的"删除 idempotencyKey + Redis 幂等机制"扩充为"删除 idempotencyKey + DB 幂等机制 / 把原子声明退化为非原子两步 / 把预声明退回到业务事务外 / 重新引入异步 failed 补偿写 / 把幂等记录退回 Redis"
6. **Tasks Task 3 §7.2.3 服务端逻辑勾选行**（line 462）：把"Redis 幂等键流程"改为"幂等键流程（按 AC3.§7.2.3 + r5~r11 锁定的 MySQL 同事务持久化方案；r1 内联文本仅为历史快照）"
7. **AC7.5 git diff 范围检查**（line 478）：r12 修订，反映实际 11 轮 fix-review 累计的 diff 范围（V1 + DB + sprint-status + story 文件 + lessons 系列 + index.md），废除 r1 "3 文件"承诺
8. **AC7 入口添加 scope 修订段**：明确"本 story 实装期间允许新增 docs/lessons/*.md 和修订 docs/宠物互动App_数据库设计.md（含新增 §5.16 + 修订 §8.3），因为 11 轮 fix-review 推动了相关设计文档同步更新"
9. **Dev Notes 错误码引用**（line 512）：把"本接口不主动返回 1008 —— idempotencyKey 命中走 cache 命中路径（200 + 缓存结果）"更新为 r11 锁定的"1008 在节点 7 阶段本接口已退役（X-lock serialize 排队下步骤 5b 永远观察到 status='success'，无可达触发路径）"
10. **Dev Notes 跨文档同步检查 idempotencyKey 描述**（line 505）：把"DB 无对应字段（idempotencyKey 仅缓存在 Redis，不入 DB）"改为"r5 review 锁定后：DB 层面有对应字段（`chest_open_idempotency_records.idempotency_key VARCHAR(128) NOT NULL` + `UNIQUE KEY uk_user_id_idempotency_key`，参见数据库设计 §5.16），idempotencyKey 不再仅是 Redis 缓存键，而是 MySQL 持久化幂等记录的 unique key"
11. **Completion Notes 重写**：r12 修订段标注 + 重写最终状态描述（反映 r11 锁定的 MySQL 同事务持久化设计 + 1008 退役 + X-lock 排队 + 二态机 + cached success 同源同时刻补算）
12. **File List 重写**：反映实际 diff 范围（V1 + DB + sprint-status + story 文件 + 11 个 lesson 文件 + index.md）
13. **Change Log 补充 r3~r12 各轮**：每轮一行，链接到对应 lesson 文档（让 Change Log 成为 11 轮迭代轨迹的索引）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **执行 fix-review 修改任何被多个下游 story 引用的契约文档（V1 接口设计 / 数据库设计 / 总体架构 等）时**，**必须** **同时检查并同步更新所有引用该契约的 story 文件（Tasks / AC / Dev Agent Record）**，**禁止**仅修改 canonical 文档而留下 story 文件停留在过时契约描述。
>
> **展开**：
> - 每轮 fix-review 完成 canonical 文档修订后，对每个引用该契约的 story 文件做以下三类自检：
>   1. Tasks 段：是否仍在描述被推翻的旧设计？（如旧 SQL / 旧 Redis 流程 / 旧错误码）
>   2. AC 段：内联 schema / 流程 / 错误码是否与 canonical 文档当前版本一致？
>   3. Dev Agent Record（Completion Notes / File List / Change Log）：是否准确反映本轮 + 历史轮次的真实状态？
> - 若 story 已 done：在 Story Change Log 追加一行说明"canonical 文档在 r{N} 已更新到 ... 设计，本 story 内联描述仅为 r1 历史快照，下游实装以 canonical 当前版本为准"
> - 若 story 仍在 review / in-progress：直接在原段落改写 + 添加 r{N} super-note 标注变更原因
> - **不要**仅依赖 `_bmad-output/implementation-artifacts/decisions/*.md`（ADR）作为契约同步介质 —— 下游 dev agent 是按 story 文件读 SOP 的，不是按 ADR
> - **反例**：fix-review r3 ~ r11 期间每轮都精修了 V1 §7.2 + 数据库设计 §5.16，但 11 轮内**没有任何一轮**回头同步 story 20-1 文件中的契约描述 → 累积漂移到 r1 完全失真状态，直到 r12 codex review 显式 surface（如果 r12 没 surface，Story 20.6 dev agent 大概率会按 Redis 设计实装出完全错位的代码）
> - **反例**：fix-review 流程只关心 canonical 文档"内容正确"，不关心 story 文件"描述与 canonical 一致" —— 这是 BMAD 流程的隐性裂缝（story 文件的"双重角色"导致），lesson 必须显式提示后续 Claude
> - **加强约束**：当一个 story 在 review 状态下经历 ≥ 3 轮 fix-review 时，**必须**在第 3 轮起开始同步 story 文件（不要积累到 r10+ 才补救） —— 这条数量阈值在 ADR-0007（如适用）或本 lesson 蒸馏后的 cheatsheet 中可作为机械检查项

## Lesson 2: Story 文件中的"diff 范围"自校验段必须随 scope 扩张同步修订，禁止留下假陈述

- **Severity**: low
- **Category**: docs
- **分诊**: fix
- **位置**: `_bmad-output/implementation-artifacts/20-1-接口契约最终化.md:478`（AC7.5）+ `:607-610`（Completion Notes "git diff 范围" 行）

### 症状（Symptom）

AC7.5 / Completion Notes 中的 "git diff 范围检查" 自校验断言：

> diff 只触及 V1 接口设计文档 + sprint-status + 本 story 文件三处

但 11 轮 fix-review 实际累计 diff 范围远超此声明，至少包括：

- `docs/宠物互动App_V1接口设计.md`
- `docs/宠物互动App_数据库设计.md`（新增 §5.16 + 修订 §8.3 等）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`
- `_bmad-output/implementation-artifacts/20-1-接口契约最终化.md`
- `docs/lessons/2026-05-14-*-20-1-r*.md`（约 10 个新 lesson）
- `docs/lessons/index.md`（多次追加）

让 story 自身的 scope 验证段成为假陈述 —— 任何使用本 story 文件做后续 audit / scope 审查的人（含未来 Claude）都会得到错误结论。

### 根因（Root cause）

Story 文件中存在"过去时陈述"（如"diff 仅触及 X / Y / Z 三文件"），但**没有显式标注"陈述对应的时间点"**（是 r1 dev-story 落地时刻？还是 done 时刻？）。当 scope 在后续迭代中合法扩张（fix-review 推动了关联 canonical 文档同步修订 + lesson 沉淀），原陈述失效但**没有任何机制提示"这条陈述需要更新"**。

更深层根因：BMAD story file 模板里的 "AC + scope 验证段" 假设 dev-story 是**单次落地**（dev → review → done），不假设 dev-story 落地后还会经历多轮 fix-review 触发 scope 扩张。当现实出现 11 轮 fix-review 时，模板的"过去时 scope 断言"语义就破产了。

### 修复（Fix）

把 AC7.5 + Completion Notes 中的相关声明改写为"r12 修订"段，**显式标注**：

- 原 r1 完成笔记中的"3 文件"承诺不再成立
- 11 轮 fix-review 后实际 diff 范围包括 V1 + DB + sprint-status + 本 story + lessons 系列 + index.md
- 在 AC7 顶部追加 scope 修订段，明确"本 story 实装期间允许新增 docs/lessons/*.md 和修订 docs/宠物互动App_数据库设计.md，因为 11 轮 fix-review 推动了相关设计文档同步更新"
- File List 重写为反映实际 diff 范围

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **story 文件中写"diff 范围 / scope 验证"类过去时陈述时**，**必须** **同时标注陈述对应的时间点（dev-story 落地时 / r{N} 修订时 / 当前最新）**，并在每次后续修订时**显式更新陈述或添加 r{N} 修订段**，**禁止**留下"不带时间标签的全局过去时断言"。
>
> **展开**：
> - "scope 验证段" / "diff 范围" / "本 story 不做" 等过去时陈述应采取以下任一形式：
>   1. **带时间戳的快照**（"截至 r1 dev-story 落地时刻，diff 仅触及 X / Y / Z"）+ 后续每轮 fix-review 添加新快照
>   2. **永远当前态**（"截至本文件最新版本，diff 实际触及 ..."）+ 每次修订时强制同步
> - **禁止**写"diff 仅触及 X / Y / Z 三文件"这类**不带时间标签的全局断言** —— 这等价于"声明本 story 永远不会经历 scope 扩张"，与 BMAD 实际流程不符
> - **反例**：本 story 20-1 r1 写的 AC7.5 "git diff --stat 仅显示 3 file changed" 是典型的"无时间标签全局断言"，11 轮 fix-review 后必然破产 → 修订为"r12 review 修订：scope 已扩张以容纳 11 轮 fix-review"
> - **加强约束**：BMAD story 模板的 AC / Tasks 段允许出现"scope 红线"（"本 story 不做 X / Y / Z"，用作 dev-story 锚定时的指导），但**禁止**出现"scope 既成事实"（"diff 仅触及 X / Y / Z"，用作 done 时刻的 audit）—— 后者只能出现在带时间戳的 Completion Notes 段
> - **元规则**：任何写在 story 文件里的"既成事实"断言，必须能回答"如果未来本 story 再迭代一轮，这条断言会不会失效？" —— 如果会失效，就必须加时间戳 + 修订机制

---

## Meta: 本次 review 的宏观教训

本轮（r12）+ 上一轮（r11）暴露的是 **BMAD 流程在"story 文件作为契约入口"这一隐性角色上的系统性裂缝**：

1. **canonical 文档 + story 文件 + lesson 文档** 这三类介质在 BMAD 流程里是**异步同步**关系 —— canonical 修订后没有自动机制确保 story 文件 + lesson 文档同步迭代
2. **多轮 fix-review** 是 BMAD 流程允许的形态（review status 可以承受任意多轮 fix-review），但 story 文件的"既成事实"断言段（AC scope / Completion Notes / File List）**没有针对"多轮迭代"的设计**
3. **下游 dev agent 的读 SOP** 是按 story 文件入口的（不是按 canonical 文档入口的），这放大了"story 文件 stale"的破坏力 —— 下游会按 stale story 实装出与 canonical 不一致的代码
4. **修复模式**：以后所有"story 在 review 状态下经历多轮 fix-review"的场景，最后一轮 fix-review 应该显式做"story 文件同步收尾"步骤（不只是修 canonical 文档），这一步骤应该被沉淀到 /fix-review SKILL 的检查清单里（本 lesson 是触发点）
