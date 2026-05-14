---
date: 2026-05-14
source_review: codex review round 15 of Story 20-1 (file: /tmp/epic-loop-review-20-1-r15.md)
story: 20-1-接口契约最终化
commit: 566071f
lesson_count: 2
---

# Review Lessons — 2026-05-14 — 跨接口字段语义对齐 + 错误码表收口（无可达路径就删除而非保留）

## 背景

Story 20-1 接口契约最终化 r15 review。codex 抓出 V1 接口设计文档两条跨段一致性 finding：(1) §7.1 GET /chest/current 声称与 §5.1 GET /home 的 `data.chest.*` 同义，但 §5.1 字段表仍写 `remainingSeconds <= 0`，与 §7.1 钦定的"clamp 到 0"矛盾；(2) §7.2 POST /chest/open 错误码表保留 1008 行，但行内自述"节点 7 阶段无可达路径"——契约表自相矛盾会诱导 client / 测试方实装 dead path。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | §5.1 `data.chest.remainingSeconds` 字段表与 §7.1 不一致 | P2 (medium) | docs | fix | `docs/宠物互动App_V1接口设计.md` §5.1 ~line 403 |
| 2 | §7.2 错误码表保留 1008 dead-path 行 | P2 (medium) | docs | fix | `docs/宠物互动App_V1接口设计.md` §7.2 ~line 1133 |

## Lesson 1: 跨接口的"同义字段"声明必须双向对齐 —— 单边写"同 X"不算契约对齐

- **Severity**: P2 (medium)
- **Category**: docs / architecture (contract consistency)
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §5.1 line 403 + §7.1 line 854

### 症状（Symptom）

§7.1 字段表声称 `data.remainingSeconds` 与 `§5.1 data.chest.remainingSeconds` "同义"，但 §5.1 字段表 description 仍写 "> 0 表示 counting，≤ 0 表示已可开启"（容忍负数），与 §7.1 钦定的 `max(0, ceil(...))` clamp 公式（永远 ≥ 0）不一致。downstream client / server 实装会同时把 §5.1 和 §7.1 当 spec 读 → 两份实装会用不同的边界条件（一份做 `value > 0` 判定，另一份做 `value > 0 || value == 0 但 status == 1`），form 出难以 reproduce 的"看着像状态机抖动"bug。

### 根因（Root cause）

迭代时只更新了"主接口"字段表（§7.1 新加 chest endpoint），但**忘记同步**已存在的"次接口"字段表（§5.1 GET /home 早就有 chest 子字段）。"同义"是一种**断言**，断言要么双向验证、要么改一边。仅在新加的那侧写"与 §X 同义"是单边断言，不构成跨段对齐 —— 旧 §X 的措辞可能写于更早的 round，承载着与新断言冲突的语义边界（这里：`≤ 0`）。

### 修复（Fix）

把 §5.1 `data.chest.remainingSeconds` 行的描述同步成 §7.1 同公式同 clamp：

- before: `> 0 表示 counting，≤ 0 表示已可开启`
- after: `> 0 表示 counting，= 0 表示已可开启（与 status = 2 等价 —— client 可二选一判定）；server 端按 max(0, ceil((unlock_at - now) / 1s)) 计算，到 0 时不会出现负数（与 §7.1 GET /chest/current data.remainingSeconds 同语义同公式 —— 跨接口字段对齐，client 解析层应按 Int 处理，避免 Swift UInt 在收到负数时 crash 的防御性需求消失）`

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **新增字段表里写"与 §X 同义"声明** 时，**必须**在同次 patch 里**双向验证** §X 那边的字段表 description / 边界值与本侧一致；若 §X 旧描述与本次断言冲突，**两边一起改**，不能只改新侧留下旧侧矛盾。
>
> **展开**：
> - "同义" / "对齐" / "一致" 这类声明本质是 spec 间的双向约束，写一边就要验另一边
> - 对齐字段必须覆盖：**类型 + 取值范围 + 计算公式 + 边界 case**；只对齐字段名不算对齐
> - 跨段 review 时优先 grep 字段名（如 `remainingSeconds`），把所有出现位置一次性对齐 —— 这比"一段一段读"快且不漏
> - **反例**：在 §7.1 写 "remainingSeconds 永远 ≥ 0" 同时声称 "与 §5.1 同义"，但留着 §5.1 的 "≤ 0 表示已可开启" 描述不动 —— 这不是契约最终化，这是契约分叉

## Lesson 2: 错误码表保留 "dead path" 行会污染契约 —— 无可达路径就从接口级错误码表删除，不是注释保留

- **Severity**: P2 (medium)
- **Category**: docs / error-handling (contract surface)
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §7.2 line 1133

### 症状（Symptom）

§7.2 POST /chest/open 的"可能的错误码"表列 1008 行，触发条件文字却自述"节点 7 阶段本接口实际无可达路径"。client SDK 作者读契约时会按表实装 1008 分支 → dead branch；测试方读表设计 1008 用例 → 永远不会触发的 case 浪费回归资源 + 误以为"测不到 = 实装漏了"反过来去 review server。

### 根因（Root cause）

迭代过程中（r11 review）把 1008 在本接口设计为"无可达路径"后，**保守心理**让作者选了"保留行 + 注释退役"而不是"删除行"。但接口级错误码表的语义是"server 在本接口可能返回的 code 列表"——保留 dead path 等于撒谎。"全局错误码定义保留"（§3 全局错误码表）与"接口级错误码表保留"是两件事：前者是 catalog，后者是 contract surface。catalog 可以列未启用的码，contract surface 只列实际可能出现的码。

### 修复（Fix）

- 从 §7.2 错误码表删除整个 1008 行
- 在表下方既有"注：本接口**不**触发 4003 / 5xxx / 6xxx / 7xxx"段之后**新增一段** `> **注（r15 review 锁定）**`，说明本接口在节点 7 阶段不触发 1008（含 r11 锁定的两条不可达理由：MVCC 限制 + InnoDB X-lock serialize 限制），并明确 §3 全局错误码表中 1008 定义保留供未来其他幂等接口复用
- 顺手把 §7.2 1009 行内的 "详见 1008 行" 引用、关键约束段「1008 错误码 r11 退役决策」末尾的"错误码表更新"小节都同步成"1008 行已移除 + 详见表下方注段"，避免文档内死链
- **顺带改动**：关键约束段对"错误码表更新"的措辞从描述性（"触发条件文字标注..."）改为决策性（"1008 行已从 §7.2 错误码表移除..."）—— 必要，因为这是契约最终化文档，决策段必须与表的实际形态一致

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **接口级错误码表 / response code 表里看到某行被注释为"理论上不会触发 / 无可达路径 / dead path"** 时，**必须删除该行**而不是留着加注释 —— 同时在表下方加一段"本接口不触发 X，X 全局定义保留在 §catalog 供未来扩展复用"。
>
> **展开**：
> - **接口级错误码表 = contract surface**（server 实际会返回的 code），**全局错误码表 = catalog**（系统所有定义的 code），两者语义不同
> - dead path 留在 catalog 里没问题（未来其他接口可能复用），但留在 contract surface 上一定要删 —— 否则 client / SDK / 测试方会实装 dead branch
> - 删除后**必须**在表下方加注，否则跨段引用（如"详见 X 行"）会变成死链 —— 删除 + 加注是一个原子改动，不能拆
> - **反例**：保留 1008 行 + 在行内写"r11 review 锁定：实际无可达路径"——这是把 review 决策当注释挂在 dead path 上，contract surface 仍然在说"server 可能返回 1008"，client 看完依然会去写 1008 分支
> - 类似 pattern：API spec 里"deprecated 字段表"也走同一规则 —— deprecated 字段从字段表删除 + 在变更记录段说明，**不**保留行加 deprecated 注释

---

## Meta: 本次 review 的宏观教训

两条 finding 共享同一个根因：**契约最终化阶段，"声明"和"实际"必须双向一致**。

- Lesson 1 是"字段语义声明（同义）"与"实际描述"分叉
- Lesson 2 是"决策声明（dead path 退役）"与"实际错误码表"分叉

未来 Claude 在做 contract finalization / spec freeze 类工作时，**应**对每条决策段 / 跨段断言做一次"反向 grep + 双向核对"——决策段说"X 已退役" → grep X 在所有接口级表 / 字段表 / 示例 JSON 里的出现位置，逐个核对实际形态与决策段一致。这种核对**不依赖 LLM 自身的逻辑推理**，是个机械动作（grep + diff），但漏掉就会形成 r12-r15 这种"小尾巴 review 收敛多轮"的成本。
