---
date: 2026-05-02
source_review: codex review r5 of Story 7.1 接口契约最终化（/tmp/epic-loop-review-7-1-r5.md）
story: 7-1-接口契约最终化
commit: 4cda156
lesson_count: 3
---

# Review Lessons — 2026-05-02 — Story artifact 里的 AC 副本必须与主文档每轮 review fix 同步刷新

## 背景

Story 7.1（POST /steps/sync + GET /steps/account 契约锚定）从 r1 起，连续 4 轮 review 都修的是 `docs/宠物互动App_V1接口设计.md` 主文档（V1 doc）：r1 删了 `clientTotalSteps ≤ 200000` 业务硬上限、把 3001 触发改成"将超过 50000"语义；r2/r3/r4 修了 user_id 限频 scope / source enum / step_account 数值不变量等等。

但 r5 codex 又挑出 2 条 P2 finding：story 自身 file（`_bmad-output/implementation-artifacts/7-1-接口契约最终化.md`）的 AC 章节里**复制粘贴了 V1 doc 的 schema 片段**作为"AC2 §6.1.2 请求体"等小节的内容；这些副本在 r1 时**没**跟着主文档一起改。所以主文档已经写了"value ≥ 0；不设业务硬上限"，story 文件 line 82 还停留在"0 ≤ value ≤ 200000"；主文档已经写了"prevAccepted + curDelta > 50000"，story 文件 line 115 还停留在"累计 ≥ 50000"。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | story artifact line 82 残留 `clientTotalSteps ≤ 200000` 硬上限 | P2 | docs | fix | `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md:82` |
| 2 | story artifact line 115 当日封顶判断条件停留在 `累计 ≥ 50000`（不是"将超过"语义） | P2 | docs | fix | `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md:115` |
| 3 | story artifact line 256 AC4 例子文字"累计已达 50000"违反 r1 修订的"将超过"语义 | P2 | docs | fix | `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md:256` |

第 3 条是本轮 fix-review 主动 grep 整篇 story 时扫到的额外不一致（review 原文未指出，但属于同一根因）。

## Lesson 1: clientTotalSteps 业务硬上限（200000）残留在 story artifact

- **Severity**: P2
- **Category**: docs
- **分诊**: fix
- **位置**: `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md:82`

### 症状

story file AC2 章节的请求体字段表写着：

```
| `clientTotalSteps` | number (int) | 必填 | 0 ≤ value ≤ 200000（实际硬上限，超出按防作弊处理） | ... |
```

而 V1 主文档（`docs/宠物互动App_V1接口设计.md:515`）在 r1 已改成：

```
| `clientTotalSteps` | ... | `value ≥ 0`（int32 自然上限即可，**不**设业务硬上限；server 不对总值本身做防作弊判断，所有反作弊只针对 delta，见下方"防作弊阈值"） | ... |
```

下游 Story 7.3 / 8.5 实装者如果以 story file 为 source of truth，会错误地在 handler 加 `clientTotalSteps > 200000` 校验返 1002，或在 iOS 端加客户端硬上限校验。

### 根因

story file 的 AC 章节用 markdown fenced block 完整复制了 V1 主文档的字段表 / 服务端逻辑 / 错误码表，作为"实施指引"。fix-review 修主文档时只 Edit 了 V1 doc，没意识到 story file 里有一份已经"印好"的副本 —— 副本变成了**第二份契约权威**，但没人记得它存在。

### 修复

```diff
- | `clientTotalSteps` | number (int) | 必填 | 0 ≤ value ≤ 200000（实际硬上限，超出按防作弊处理） | ...
+ | `clientTotalSteps` | number (int) | 必填 | `value ≥ 0`（int32 自然上限即可，**不**设业务硬上限；server 不对总值本身做防作弊判断，所有反作弊只针对 delta，见下方"防作弊阈值"） | ...
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **fix-review 修改 V1 主文档（或任何被 story artifact 用 fenced block 抄写过的设计文档）** 时，**必须**同步 grep 当前 story file 是否有 AC 副本；任何与主文档不同步的 AC 副本都属于本轮 fix 的 scope，不能留到下一轮 review。
>
> **展开**：
> - story artifact 里的 `\`\`\`markdown` fenced block 通常是"AC 锚定的目标内容副本"——它和主文档**必须**逐字一致；副本和主文档分裂等于契约分裂
> - **强制动作**：每次 fix V1 doc 之后，立刻对当前 story file 做一次 grep 扫描（用 review 中提到的关键 token：`200000` / `≥ 50000` / `prevAccepted` / `已达上限` 等），确认所有副本都已同步
> - **反例**：r1 review 修了 V1 doc 的 `clientTotalSteps` 范围约束 + 3001 触发条件，但**没**扫 story file —— 副本一直留到 r5 才被 codex 挑出来；连续 4 轮 review 都 miss 了这个根因
> - **反例**：以为"主文档改了 = 契约改了"，忘记下游实装者直接读的是 story file 而不是 V1 doc（story file 才是 dev 阶段的实施引子）

## Lesson 2: 当日封顶判断条件 `≥ 50000` vs `prevAccepted + curDelta > 50000` 的语义差异

- **Severity**: P2
- **Category**: docs
- **分诊**: fix
- **位置**: `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md:115`

### 症状

story file AC2 §6.1.4 服务端逻辑第 4 段：

```
- **当日封顶**：若同 sync_date 当日 `accepted_delta_steps` 累计 ≥ 50000（配置 `steps.daily_cap`） → 当次 delta 强制 = 0 + log warning + **返回 3001 步数同步数据异常**
```

V1 主文档 r1 已改成"prevAccepted + curDelta > 50000"判断（即"本次入账后是否越界"，不是"已达上限"）。

两者对"当日已经累计到 50000 后的同一天后续 sync"行为不同：
- 旧文（`累计 ≥ 50000`）：后续每次 sync（即使 delta = 0 的重复 / 倒退场景）都会被判 3001
- 新文（`prevAccepted + curDelta > 50000`）：delta = 0 的同步入账后还是 50000，**不**越界，**不**返 3001（仍 code = 0 + 写日志）

后者才是正确的契约（重复 / 倒退场景属于正常客户端行为，不是作弊）。

### 根因

同 Lesson 1：副本未同步主文档修订。这条特别危险因为是**业务逻辑分支判断**——下游 service 实装者会直接照抄判断条件 `if accepted_total >= 50000 { return ErrAbnormal }`，而不是更精确的 `if accepted_total + curDelta > 50000 { return ErrAbnormal }`。

### 修复

```diff
- - **当日封顶**：若同 sync_date 当日 `accepted_delta_steps` 累计 ≥ 50000（配置 `steps.daily_cap`） → 当次 delta 强制 = 0 + log warning + **返回 3001 步数同步数据异常**
+ - **当日封顶**：若同 sync_date 当日 `accepted_delta_steps` 历史累计 + 本次截断后 delta **将超过** 50000（配置 `steps.daily_cap`，即 `prevAccepted + curDelta > 50000`） → 当次 delta 强制 = 0 + log warning + **返回 3001 步数同步数据异常**（注：判断条件是"本次入账后是否越界"，**不**是"已达上限"，否则会放过最后一次跨界 sync，例如 prev=49000 + cur=4000 应被拒）
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude **不能用"模糊一致"标准验收 review fix**——业务条件判断（`>=` vs `>`、"已达" vs "将超过"）的字面差异即语义差异，必须每个出现该条件的位置都精确同步。
>
> **展开**：
> - 当 review 修了某个**条件判断**（不仅是文字润色），grep 范围要**扩展到所有可能复刻该判断的副本**——包括 story file / lesson file / 其他设计 doc / commit message
> - **强制动作**：fix 完后，用 review 提到的旧 token（如 `≥ 50000` / `已达上限`）做一次全工作区 grep，确认 0 命中；用新 token（如 `将超过` / `prevAccepted`）做正向 grep，确认所有应有的位置都命中
> - **反例**：以为"V1 主文档错误码表里的 3001 行已经写了 `prevAccepted + curDelta > 50000` = 整篇文档自洽"，但忽略服务端逻辑 bullet 里还停留在 `累计 ≥ 50000` 老语义；同一份文档内不一致比跨文档不一致更隐蔽

## Lesson 3: AC 文字描述（不是 fenced block 副本）也属于副本同步范围

- **Severity**: P2
- **Category**: docs
- **分诊**: fix
- **位置**: `_bmad-output/implementation-artifacts/7-1-接口契约最终化.md:256`

### 症状

story file AC4 章节的"关键约束"段：

```
- 每个错误码的"触发条件"列必须**精确**（如"`syncDate` 格式不符 YYYY-MM-DD"而非"参数有问题"；"当日 accepted_delta_steps 累计已达 50000"而非"步数异常"）
```

这里"累计已达 50000"是 AC 编写者在写 AC 元规则（"触发条件必须精确"）时**举例**的"精确写法"，但这个例子本身就违反了主文档已经在 r1 改的"将超过"语义。

### 根因

AC 写元规则时举例引用了主文档老版本的"标准写法"，主文档改了之后，**例子也需要跟着改**。这种"AC 元规则里的例子"特别容易被忽略——它不是 fenced block 副本，是普通 markdown 段落里的引号里的字符串，grep 时如果只搜 fenced block 范围会漏掉。

### 修复

```diff
- - 每个错误码的"触发条件"列必须**精确**（如"`syncDate` 格式不符 YYYY-MM-DD"而非"参数有问题"；"当日 accepted_delta_steps 累计已达 50000"而非"步数异常"）
+ - 每个错误码的"触发条件"列必须**精确**（如"`syncDate` 格式不符 YYYY-MM-DD"而非"参数有问题"；"当日 `prevAccepted + curDelta > 50000`（本次入账后越界）"而非"步数异常"）
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 grep 寻找契约副本时，**不要**只搜 fenced block 内容；普通段落里的引号字符串、AC 元规则里的"反例 / 正例"举例、commit message 草稿、Change Log 条目都可能是契约副本。
>
> **展开**：
> - **强制动作**：grep 时用关键 token（如旧条件值 `≥ 50000` / `200000`）而**非**结构形式（fenced block / 表格行）；token 出现在哪一行就要修哪一行
> - **反例**：以为"AC4 是元规则不是契约副本，可以不动"——但 AC4 里的例子文字如果与主文档不一致，下游 dev 抄走元规则时连例子一起抄，照样错

---

## Meta: 本次 review 的宏观教训

Story 7.1 是纯文档 story，连续 5 轮 review 都在修同一个表面问题：**主文档 vs story artifact AC 副本**之间的契约漂移。这暴露的是 BMAD 工作流的一个结构性缺陷：

- story artifact 在 dev-story 阶段被创建后，AC 章节会包含主文档的 markdown fenced block 副本作为"实施目标快照"
- fix-review 阶段修主文档时，副本不会自动跟随
- review 工具（codex）只看主文档 + diff，能发现主文档与目标契约的偏离，但**不主动**告诉你"还有副本在 story file 里没改"
- 所以"主文档干净 + story file 残留" 这种状态可以连续 4 轮通过 review，直到下游 review 终于扩展 scope 把 story file 也读进去

**根治方案**（不在本次 fix 范围，提议给未来 epic）：
1. dev-story 模板减少 markdown fenced block 副本，改成"引用 V1 文档行号"的形式（避免副本本身存在）
2. fix-review skill 的步骤 5 默认 grep "review 提到的关键 token" 全工作区，自动扩展 scope 到 story file
3. 或者在 sprint-status.yaml 标 review→done 之前强制跑一次"AC 副本与主文档 diff"检查
