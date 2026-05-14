---
date: 2026-05-14
source_review: "file: /tmp/epic-loop-review-20-1-r3.md (codex review round 3 for story 20-1-接口契约最终化)"
story: 20-1-接口契约最终化
commit: 9b929ed
lesson_count: 3
---

# Review Lessons — 2026-05-14 — 幂等缓存写回失败的 client 重试指引 / 数据完整性 vs 业务错误的错误码归类 / 跨节同名字段语义对齐（20-1 r3）

## 背景

Story 20.1 把 §7.2 POST /chest/open 契约 finalize 进文档；codex round 3 review 抓到三处契约层语义陷阱：(1) Redis 幂等缓存 SET 失败后的 client 指引（"换新 key"）会破坏"同 key 重试安全"承诺并造成重复扣步数 + 重复出箱；(2) `user_step_accounts` 行缺失被映射到 3002 "步数不足"，把 server 数据完整性问题伪装成 client 可恢复的业务错误；(3) `data.nextChest.remainingSeconds` 字段在 §7.2 写"固定 600"，与 §7.1 GET /chest/current 同名字段"按 ceil((unlock_at - now)/1s) 计算"语义漂移。三条均为契约层叙述问题，**不**涉及代码。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Redis SET fail-after-commit 的 client 重试指引必须**禁止换新 key** | high | architecture / docs | fix | `docs/宠物互动App_V1接口设计.md:989, 1092` |
| 2 | `user_step_accounts` 行缺失应映射 1009 而非 3002 | medium | error-handling / docs | fix | `docs/宠物互动App_V1接口设计.md:964, 1069-1070` |
| 3 | `nextChest.remainingSeconds` 应按 §7.1 同义计算语义而非固定字面量 600 | medium | docs | fix | `docs/宠物互动App_V1接口设计.md:1024` |

## Lesson 1: Redis 幂等缓存 SET-after-commit 失败时，client **禁止**自动换新 key 重试

- **Severity**: high
- **Category**: architecture / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:988-990`（Redis 回填失败处理）+ `:1092`（关键约束 § 事务边界 case (c)）

### 症状（Symptom）

§7.2 服务端逻辑步骤 5 描述：MySQL 事务已 commit 成功 + Redis SET（把 sentinel `"__pending__"` 覆盖为完整 response JSON）失败 → 原文档建议 client「改用**新** idempotencyKey 重试」。问题：用户首次请求实际已成功（步数已扣 1000 + chest_open_logs 已写 + 新 chest 已生成 + reward 已下发），若 client 在收到 200 后又因网络层乐观重试改用新 key 触发第二次开箱事务，会**重复**扣 1000 步 + **重复**发放装扮 —— 破坏「同一逻辑动作仅扣一次步数」的核心契约。

### 根因（Root cause）

文档作者把"sentinel SET 失败 → 同 key 后续重试会持续命中 sentinel 收到 1008"当成「需要 client 主动绕过」的可恢复 case，机械给出"换新 key"建议。**误判点**：

1. 把"幂等重读失败"和"幂等首发失败"混为一谈——前者是性能 / UX 退化（读不出首次结果，但首次响应已发出，用户实际无损），后者才是正确性 bug
2. 忽视了 client 端**无法区分**「事务 commit 成功 + SET 失败」（首发已生效）vs「事务 commit 失败 + DEL 也失败」（首发未生效）两种 sentinel 残留 case——前者换 key 会重复扣费、后者换 key 无害；从 client 视角两种 case 看起来一样（都是同 key 返回 1008）
3. 幂等契约的本质是「同 key 重试**至多产生一次副作用**」，把"换新 key 重试"作为兜底相当于把幂等承诺降级为「best-effort + 用户运气」

### 修复（Fix）

把"SET 失败 → 建议换新 key"改为"SET 失败 → 禁止换新 key + 同 key 退避 N 次后放弃 + 联系客服 + server 端 log.Error + 触发 metrics 告警"：

- §988-990 改写：成功路径 SET 失败 / 失败路径 DEL 失败均**禁止** client 自动换新 key；指向 §7.2「client 重试策略」§1008 重试条款（同 key + 指数退避 N 次后放弃 + 提示用户「奖励已发放，请刷新查看；如未到账请联系客服」）
- §988 / §990 新增"运营层面 Redis 可用性 SLA ≥ 99.9%"约束 + 实装锁定（Story 20.6 必须 log.Error + metrics counter `chest_open_idem_cache_writeback_failed_total`）
- §1092 case (c) 同步改写措辞，并在原文"故 MVP 阶段禁止业务流程允许重复开箱"基础上明示「client 禁止自动换新 key 重试 → 由用户联系客服路径介入」

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **撰写 / review 幂等接口契约** 时，**禁止**建议 client 在「同 key 命中 sentinel」case 下**自动换新 key 重试**——必须区分「事务真的失败 + 缓存清理失败」vs「事务成功 + 缓存回填失败」两种 case，由于 client 端无法区分，唯一安全策略是**同 key + 退避 + N 次后由人工介入**。
>
> **展开**：
> - 幂等契约的硬约束 = 「同 key 重试至多产生一次副作用」，任何把"换新 key"塞进重试链的建议都在破坏此约束
> - 缓存写回失败后产生的「sentinel 残留」必须由 server 端运营层介入（log.Error + alert + SLA），**不**由 client 端业务路径承担
> - 网络层重试 / 业务错误重试 / 1008 sentinel 重试三种 case 的指引必须**显式分离**写入文档；其中 1008 sentinel 重试只能"同 key + 退避 + 放弃"，**不**与"业务错误重试可换新 key"混说
> - **反例**：写文档时这么说「Redis SET 失败 → client 用新 idempotencyKey 重试，首次资产已落盘 + 已下发，无重复风险」—— 这种说法暗含了「资产已落盘 = 换新 key 无害」的错误前提，忽略了「换新 key 会再开第二次事务」的语义层后果

## Lesson 2: 数据完整性异常应映射 1009 服务繁忙，**禁止**与"业务条件不满足"的 3xxx / 4xxx 同码

- **Severity**: medium
- **Category**: error-handling / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:964`（步骤 4c 错误映射）+ `:1069-1070`（错误码表 1009 / 3002 触发条件）

### 症状（Symptom）

§7.2 服务端逻辑步骤 4c：FOR UPDATE 锁 `user_step_accounts` 时若行不存在 → 原文档映射到 **3002 "可用步数不足"**。问题：3002 的语义是「用户余额 < 1000，请走路赚步数」，client 收到会引导用户走步 + 主动同步步数；但「行不存在」是 server 端 Story 4.6 登录初始化未执行 / 行被异常删除导致的数据完整性问题，让用户走步无法修复，反而陷入「走再多步也不会出现行」的死循环 UX。

### 根因（Root cause）

把「逻辑等价于零余额」（行不存在 = available_steps 视为 0）当成可以**与零余额复用同一错误码**。**误判点**：错误码的归类应**按 client 端可采取的纠错行动**而非按「条件表达式语义相似度」分组。`available_steps < 1000`（行存在 + 余额不足）→ 用户可以走路；`row missing`（行不存在 + server 数据缺失）→ 用户无能为力。两条路径的 client UX 完全不同，不能合并到同一 code。

### 修复（Fix）

- §964 改写：行不存在 → rollback → **1009 服务繁忙**（数据完整性异常）；与"available_steps < 1000 → 3002"明确分离
- §1069-1070 错误码表同步更新：1009 触发条件新增「`user_step_accounts` 行缺失」；3002 触发条件加注「`user_step_accounts` 行**不存在**的 case **不**映射 3002，走 1009」

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计 / review 错误码映射** 时，必须按 **client 端可采取的纠错行动**分组，**禁止**把「数据完整性异常」（server 端 bug / 数据缺失）与「业务条件不满足」（用户可补救）映射到同一 code。
>
> **展开**：
> - 错误码归类启发式：先问"client 收到这个 code 该做什么"——「走路赚步数」/「联系客服」/「重试」/「换 key」是完全不同的纠错动作，对应完全不同的 code
> - 数据完整性异常（row missing / unique constraint 应该不撞 / FK 应该存在）→ 一律走 5xx 等价的"服务繁忙" / "系统异常"码（如本契约的 1009），**不**走 3xxx / 4xxx 业务码
> - 业务条件不满足（余额不足 / 状态不对 / 权限不够）→ 走 3xxx / 4xxx，client UX 给用户纠错入口
> - **反例**：「FOR UPDATE 时行不存在 → 视为余额 = 0 → 映射 3002」—— 这种映射会让 client 误把 server 端 data integrity bug 包装成"用户走路就能解决"的 UX，掩盖了真实 server 故障
> - 在错误码表里，每条触发条件应有"反向声明"——「3002 **不**包含 row missing case，那条走 1009」——避免 finalize 后实装者机械拷贝忽略边界 case

## Lesson 3: 同名字段在不同接口间必须按**同一计算语义**描述，**禁止**因"通常值固定"就写成字面量

- **Severity**: medium
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1024`（§7.2 nextChest 字段表 remainingSeconds 行）

### 症状（Symptom）

§7.2 `data.nextChest.remainingSeconds` 字段表写「节点 7 阶段固定 `600`」，但 §7.1 GET /chest/current 同名字段定义为「`max(0, ceil((unlock_at - now) / 1s))`」。问题：开箱事务里 INSERT 新 chest 设 `unlock_at = NOW() + INTERVAL 10 MINUTE`，**序列化响应**时计算 `remainingSeconds` 会因 NOW() 与序列化时刻的微小漂移返回 600 / 599 / 598 等；client 紧随其后调 GET /chest/current 也走 §7.1 的 ceil 计算 → 两个接口对同一字段语义应**完全一致**，写"固定 600"会让实装者倾向于在 §7.2 hardcode `600` 字面量，导致开箱响应和后续 GET 响应轻微漂移（相邻调用 599 → 600 → 599），违反"同名字段同语义"对齐承诺。

### 根因（Root cause）

文档作者把「新 chest 刚创建时 remainingSeconds **通常**接近 600」误写成「server 端**固定下发** 600」。**误判点**：契约描述的是**计算规则**而非**当前观察值**；即使某个字段在某个阶段几乎总是某个值，只要它与其他接口同名 + 同义，就必须用**同一计算公式**描述。

### 修复（Fix）

§1024 改写：从「节点 7 阶段固定 `600`（10 min = 600 sec）/ server 端固定下发 600」改成范围 `0 ≤ value ≤ 600` + 计算公式 `max(0, ceil((unlock_at - now) / 1s))` + 显式禁令"禁止实装时写死 600 字面量"，并指明与 §7.1 GET /chest/current 同义对齐。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **finalize 跨接口共享字段的契约**时，对每个同名字段必须用**同一计算公式 + 同一范围 + 同一约束**描述，**禁止**因"某个接口阶段下该值通常固定"就改写成字面量，**禁止**给实装者埋"写死字面量"的暗示。
>
> **展开**：
> - 跨接口同名字段（如 `remainingSeconds` 在 §7.1 GET / §7.2 POST nextChest 两处出现）必须做"语义对齐声明"——文档显式说"按 §X.X 同义计算"或直接抄完整公式
> - 「值通常 = 常量」≠「值固定 = 常量」；前者是阶段性观察，后者是契约硬约束；只有当 **server 端代码确实硬编码**（而非按状态计算）时才能写字面量
> - 契约里如果写明字面量，实装者大概率会 `nextChest.RemainingSeconds = 600` 硬编码而不是 `compute(unlockAt, now)` —— 这是契约措辞**直接**诱导的实装陷阱
> - **反例**：「`unlockAt`：server 端固定为 `now() + 10 min`；`remainingSeconds`：固定 `600`」—— 第一句没问题（确实是固定计算的输入），第二句错（这是基于第一句计算出来的派生量，不该用字面量描述）；正确做法是后者写「按 ceil((unlockAt - now)/1s) 计算，新 chest 刚创建时通常 = 600」

---

## Meta: 本次 review 的宏观教训

三条 finding 的共同思维漏洞：**契约措辞会"诱导"实装的安全性**。三处问题不是逻辑错误而是叙述错误，但因为契约是 server / iOS 实装的唯一权威，叙述上的"建议换 key"/"映射 3002"/"固定 600"会被实装者机械抄写到代码里，把契约层的措辞 bug 变成实装层的正确性 bug。

> **元规则**：finalize 契约文档时，对每条规则问三个问题：
> 1. 实装者机械抄这条规则做出来的代码会**违反**任何其他规则吗？（Lesson 1: 抄"换新 key"会违反幂等承诺）
> 2. client 端按这条规则做出来的 UX 会让用户**陷入死循环**吗？（Lesson 2: 抄 3002 让用户走路也救不回 row missing）
> 3. 同一字段 / 同一概念在文档其他地方有**对应表述**吗？两边的措辞是否**字面级一致**？（Lesson 3: §7.1 ceil 计算 vs §7.2 固定 600）

这是 contract finalization 的标准 review checklist，下次 review 契约文档时必须**先**问完这三条再开始。
