---
date: 2026-05-12
source_review: codex review (/tmp/epic-loop-review-14-1-r9.md) — Story 14.1 第 9 轮
story: 14-1-接口契约最终化
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-12 — story 文件 RowsAffected 措辞 + 顶层规则 1003 引用必须与 V1 doc 同步：r8 漏改的 r6 / r7 残留 drift

## 背景

Story 14.1 接口契约最终化第 9 轮 codex review。r6 把"`RowsAffected == 0 → 1009`"切换为"`err == nil` ⇒ 200 OK；`err != nil` ⇒ 1009；不读 RowsAffected"（V1 doc §5.2 line 532-537 + 597）；r7 把"pet-less → 1003"切换为"pet-less → server-acknowledged noop 200 OK + code = 0"（V1 doc §5.2 line 530 + 599）；r8 已做过一轮 story 文件 vs V1 doc resync sweep，但仅对齐了 self-broadcast / ts / 等价分层等 r2-r5 议题；**r6 / r7 议题在 story 文件中的残留措辞被 r8 漏改**：

- line 117-120 服务端逻辑步骤 4 仍写"DB 异常 / `RowsAffected == 0`（极罕见竞态）→ 1009"+"`RowsAffected == 1` → 进入步骤 5"，与 V1 doc 实装锁定的两个互斥二分直接矛盾
- line 170 错误码表 1009 触发条件仍写"DB 异常 / UPDATE RowsAffected == 0（极罕见竞态）/ 内部 panic"，把 RowsAffected==0 当成 1009 触发条件之一
- line 15 顶层"Epic 14 进度"bullet 仍写"service 层错误码映射必须严格按 14.1 锚定的 1002 / 1003 / 1009"，而 line 19 / 172 已对齐 r7（1003 移除）→ 顶层规则 stale，会被下游 Story 14.2 实装工程师按"1003 仍然有效"误读
- line 11 narrative `so that ...` 子句残留"1002 vs 1003 触发条件混淆"措辞，虽然语义上是"列出过去的混淆"，但读者扫到时仍可能误以为 1003 是 state-sync 接口的现役错误码

如果 Story 14.2 实装工程师只看 story 文件 line 15 / 117-120 / 170 一路实装（不点开 V1 doc 对照 line 532 / 597），会得到与 frozen 契约完全相反的 service 层映射：(a) `RowsAffected==0` 当成 1009 服务繁忙抛错 → "同 user 同 state 重复上报"幂等场景触发 1009，违反 §5.2 line 500 元信息表"幂等"声明；(b) pet-less 账号触发 1003 → 与 §5.1 / §10.3 / §12.3 pet-less 合法 edge case 不一致，与 r7 改动直接冲突。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | story 文件 RowsAffected 措辞未与 V1 doc r6 同步：仍写"RowsAffected==0 → 1009"违反两个互斥二分锁定 | high (P2) | docs | fix | `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md` |
| 2 | story 文件 line 15 顶层规则未与 V1 doc r7 同步：仍写"1002 / 1003 / 1009"令下游误读 1003 仍有效 | medium (P3) | docs | fix | `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md` |

## Lesson 1: story 文件服务端逻辑章节 + 错误码表 RowsAffected 措辞必须与 V1 doc"err==nil ⇒ 200；err!=nil ⇒ 1009；不读 RowsAffected"两个互斥二分逐字对齐

- **Severity**: high (P2)
- **Category**: docs
- **分诊**: fix
- **位置**: `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md:117-120` + `:170`

### 症状（Symptom）

V1 doc r6（line 532-537 + 597）已把 state-sync 服务端逻辑步骤 4 + 错误码表 1009 触发条件锁定为：

- `err == nil` → 一律 200 OK + code = 0（**不**读 `RowsAffected`、**不**区分 0/1 行）
- `err != nil` → 1009（含 driver / 网络 / 约束冲突 / 任何 DB 异常）
- 实装锁定：两个互斥二分，无第三条路径

但 story 文件 line 117-120 仍写"DB 异常 / `RowsAffected == 0`（极罕见竞态：pet 在步骤 3 后被删 —— 节点 5 阶段无业务路径会删 pets 行，故该分支视为数据损坏）→ 返回 1009 服务繁忙"+"`RowsAffected == 1` → 进入步骤 5"。line 170 错误码表 1009 行写"DB 异常 / UPDATE RowsAffected == 0（极罕见竞态）/ 内部 panic"。两处都把 `RowsAffected == 0` 当成 1009 的触发条件，违反 V1 doc 锁定。

### 根因（Root cause）

story 文件 line 117-120 + 170 是 dev-story 阶段（r0）按 epics.md 钦定 AC 写的初稿，那时 AC 文字仍是"DB 异常 / RowsAffected == 0 → 1009"（epics.md §Story 14.2 现仍保留该措辞作为下游 Story 14.2 实装 AC 的部分，与 V1 doc r6 调整后的措辞有 known gap，但 14.2 实装时按 V1 doc 为准）。r1-r8 修复的焦点是 V1 doc 本身的措辞，没有同步回 story 文件的 V1 doc 编辑指令 fenced block + AC 描述章节。r8 做过一轮 sweep，但 sweep checklist 只覆盖 self-broadcast / ts / 等价分层等 r2-r5 议题，**没有把 r6 RowsAffected 措辞列为 sweep 项**，所以漏改了。

### 修复（Fix）

按 V1 doc line 532-537 + 597 措辞重写 story 文件 line 117-120 + 170。

**Before（line 117-120）**：

```
4. **UPDATE**：`UPDATE pets SET current_state = ?, updated_at = NOW() WHERE id = ?`（按上一步查到的 pet.id）
   - DB 异常 / `RowsAffected == 0`（极罕见竞态：pet 在步骤 3 后被删 —— 节点 5 阶段无业务路径会删 pets 行，故该分支视为数据损坏）→ 返回 1009 服务繁忙
   - `RowsAffected == 1` → 进入步骤 5
```

**After（line 117-120）**：

```
4. **UPDATE**：`UPDATE pets SET current_state = ?, updated_at = NOW() WHERE id = ?`（按上一步查到的 pet.id）
   - **`err == nil`**（DB 调用无错误）→ **一律**返回 200 OK + code = 0，进入步骤 5；service 层**不**读 `RowsAffected`、**不**根据该值分支业务逻辑（与 V1 doc §5.2 line 532-537 实装锁定一致：1009 ⇔ `err != nil`；200 OK + code = 0 ⇔ `err == nil`；两个互斥二分，无第三条路径）
   - **`err != nil`**（driver / 网络 / 约束冲突 / 任何 DB 异常）→ 返回 1009 服务繁忙
   - 理由：(a) `RowsAffected == 0` 在 MySQL/GORM 语义下主要触发条件是"同 user 同 state 重复上报"幂等场景（WHERE 命中 1 行但所有列值与入参完全一致），业务上必须返回 200 OK + code = 0；(b) "步骤 3 到步骤 4 之间 pet 行消失"是节点 5 阶段无业务路径触发 DELETE pets 的理论不可达 race，即便发生仍按 `err == nil` ⇒ 200 OK 路径走，**不**降级为 1009 / 1003、**不**读 `RowsAffected`
```

**Before（line 170）**：

```
| 1009 | 服务繁忙 | DB 异常 / UPDATE RowsAffected == 0（极罕见竞态）/ 内部 panic（见 Story 14.2 service 实装） |
```

**After（line 170）**：

```
| 1009 | 服务繁忙 | DB 异常（UPDATE 执行返回 `err != nil`，含 driver / 网络 / 约束冲突等）/ 内部 panic（见 Story 14.2 service 实装）。**不**包含 `RowsAffected == 0` —— 该分支在本接口下视为幂等成功（详见 §5.2 服务端逻辑步骤 4） |
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **改 V1 doc 接口契约的服务端逻辑步骤 / 错误码表触发条件**（如把 X → 错误码 切成 200 OK 幂等 / 反之）时，**必须**同步把 story 文件里 V1 doc 编辑指令 fenced block + AC 描述章节 + 错误码表说明 + 关键约束段 + 顶层"Epic 进度 / 强前置说明"bullet 里**所有提到相同字段措辞**的位置一起改。
>
> **展开**：
> - story 文件不只是"派单文档"——它在 epic-loop 多轮 codex review 期间是 dev / fix sub-agent 的**唯一上下文窗口**（不主动读 V1 doc 时）。任何 V1 doc 措辞调整必须把 story 文件视为镜像目标
> - **机械化做法**：每一轮 fix-review 改完 V1 doc 后，**强制**对 story 文件做一次完整 grep sweep：把本轮涉及的关键字段（如 `RowsAffected` / `1003` / `pet-less` / `self-broadcast` / `ts` / `等价` 等）逐项 grep，比对每处命中是否与 V1 doc 措辞**字面一致**，不一致即修
> - **sweep checklist 必须每轮增长**：r8 sweep checklist 没把 RowsAffected 列入 → r9 暴露 drift；后续 fix-review 在 lesson 文档"预防规则"段累积该 checklist（如"r6 调整 → 加 RowsAffected 项；r7 调整 → 加 1003 项；r9 调整 → 加 X 项"），不让 sweep 范围随轮次衰减
> - **反例**：r8 fix-review 只对齐了 self-broadcast / ts / 等价分层等 r2-r5 议题就收工 → r9 codex 立刻抓到 r6 / r7 议题 drift。如果 r8 当时把 sweep checklist 扩展到"凡 r1-r7 修过的关键字段全部 grep 一遍"，本轮 r9 就不会出现

## Lesson 2: story 文件顶层规则 / narrative 描述章节的错误码列表必须与 V1 doc"§3 全局错误码表"vs"§5.2 本接口可能错误码"区分对齐

- **Severity**: medium (P3)
- **Category**: docs
- **分诊**: fix
- **位置**: `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md:15` + `:11`

### 症状（Symptom）

V1 doc r7（line 530 + 599 + 603）把 state-sync 接口的 pet-less 路径从"触发 1003"切换为"server-acknowledged noop 200 OK + code = 0"；§5.2 §5.2.6 错误码表移除 1003 行；§3 全局错误码表保留 1003（供 §6.x step-sync 等其他接口使用）。

但 story 文件 line 15 顶层"Epic 14 进度"bullet 仍写"service 层错误码映射必须严格按 14.1 锚定的 1002 / 1003 / 1009"。Story 14.2 实装工程师按 line 15 走会得到与 V1 doc 现行契约相反的指引。同时 line 11 narrative `so that ...` 子句残留"1002 vs 1003 触发条件混淆"措辞，虽然语义上是"列出过去的混淆"，但读者扫到时仍可能误以为 1003 是 state-sync 接口的现役错误码。

### 根因（Root cause）

story 文件 line 15 / line 11 是 r0 dev-story 阶段按 r0 V1 doc 措辞写的（那时 §5.2 错误码表含 1002 / 1003 / 1009 三行）。r7 fix-review 改了 V1 doc + line 19 / 172 / 177 / 261 / 284 等局部 bullet（这些 bullet 在 r7 fix-review 的 scope 内），但 **r7 fix-review 没扫到 line 15 顶层 bullet 和 line 11 narrative 子句**，因为 r7 sub-agent 只搜了"1003"在 §5.2 段落范围里的命中点，没扫顶层 Story 概述 / so that 子句。

### 修复（Fix）

把 line 15 / line 11 改为与现行契约一致：line 15 改为"1002 / 1009 触发条件 + 显式说明 r7 移除 1003 触发"；line 11 把"1002 vs 1003 触发条件混淆"改为"1002 错误码触发条件不精确（pet-less 是否触发 1003 / 是否触发其他 3xxx-7xxx 业务错误码模糊）"——以"触发条件不精确"作概括语，避免读者误以为 1003 仍是 state-sync 现役错误码。

**Before（line 15）**：

```
- **Epic 14 进度**：... service 层错误码映射必须严格按 14.1 锚定的 1002 / 1003 / 1009 触发条件 ...
```

**After（line 15）**：

```
- **Epic 14 进度**：... service 层错误码映射必须严格按 14.1 锚定的 1002 / 1009 触发条件（**不**含 1003 —— pet-less 走 server-acknowledged noop 路径返回 200 OK + code = 0，与 §5.1 / §10.3 / §12.3 pet-less 合法 edge case 同语义；r7 调整后契约层已移除 §5.2 的 1003 触发路径，仅在 §3 全局错误码表保留供其他接口使用）...
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **改 V1 doc 某接口的错误码触发条件**（特别是"移除某个错误码"或"新增某个错误码"）时，**必须**把 story 文件**全部** mentions 该错误码数字的位置（含顶层 Story / Epic 进度 bullet / narrative `so that ...` / "强前置说明" / AC / Task / Dev Notes / References / Change Log）逐处 grep + 比对，**禁止**只扫主要 §X.Y 编辑段。
>
> **展开**：
> - 错误码数字（如 1003 / 1009 / 6001）是**全局符号**，会在 story 文件多个不同语境出现：顶层概述 / narrative 子句 / AC 表格 / 关键约束 / Task subtask / Dev Notes 设计原则 / References 引用 epics.md 时回带的描述。任何一处 drift 都会让下游实装 / review sub-agent 困惑
> - **机械化做法**：fix-review sub-agent 在 r-th 轮改某错误码触发条件前，先 `Grep "<错误码数字>"` 在 story 文件全文，**列出全部命中位置**作为待对齐 checklist；改完后再 grep 一次，确认每处都已对齐或已加"显式不触发"注释
> - **§3 全局表 vs §X.Y 本接口表的区分**：错误码在 §3 全局表存在 ≠ §X.Y 本接口可触发。story 文件描述 §X.Y 错误码时**必须**用"本接口可能触发"表述，避免与 §3 全局表混淆。如某错误码在 §X.Y 已移除但 §3 仍保留（如 1003），需在 story 文件 §X.Y 描述段**显式**写"本接口**不**触发 X；X 仍在 §3 全局错误码表保留供其他接口使用"
> - **narrative `so that ...` 子句的处理**：列出"过去 / 潜在的混淆"时**禁止**直接引用现役 schema 的错误码数字，改用概括语（如"1002 错误码触发条件不精确"），避免读者误以为该错误码是当前接口的现役错误码
> - **反例**：r7 fix-review 把 §5.2 段落内的 1003 全部对齐了，但顶层"Epic 14 进度"bullet 和 `so that ...` 子句被遗漏 → r9 codex 抓到 line 15 drift。如果 r7 当时机械 grep 全文"1003"再逐处对齐，本轮 r9 就不会暴露该问题

---

## Meta: 本次 review 的宏观教训

**story 文件 drift 是 epic-loop 多轮 review 的复合 failure mode**：

- **不是单轮失误**：r1-r7 集中改 V1 doc → 每轮都没扫 story 文件 → r8 做了 partial sweep（self-broadcast / ts / 等价）→ r9 还是抓到 r6 / r7 议题 drift。这是**多轮累积**的问题
- **跨轮 sweep checklist 必须累积，不能衰减**：如果每轮 sweep checklist 只覆盖本轮新议题 → 老议题 drift 会累积下来；fix-review sub-agent 应在 lesson 文档"预防规则"段建立**累积式** sweep checklist（"r6 调整 → +RowsAffected；r7 调整 → +1003；r9 调整 → +X"），后续 fix-review 在 sweep 阶段引用最新版 checklist
- **story 文件 = sub-agent 的唯一上下文**：epic-loop 每个 sub-agent 启动时只读 story 文件 + 派单消息，不主动读 V1 doc。如果 story 文件与 V1 doc drift，sub-agent 就按 stale story 实装 → review 又抓 → 循环膨胀 → epic-loop 内部 review_round 上限被吃光。所以 story 文件的 sync 优先级**等同于** V1 doc 本身
- **fix-review 的 scope 应包含 story 文件 sweep**：本 lesson 触发对 fix-review skill 默认行为的反思 —— 是否每轮 fix-review 都应默认跑一次 story 文件 vs V1 doc resync sweep？如果跑，sweep 应基于本 epic 历史 lesson 累积出的 checklist。这件事可以由后续 epic retro 决定是否升级 fix-review skill 默认行为
