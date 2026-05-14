---
date: 2026-05-14
source_review: codex review round 9 — /tmp/epic-loop-review-20-1-r9.md
story: 20-1-接口契约最终化
commit: 9d5929d
lesson_count: 2
---

# Review Lessons — 2026-05-14 — time-derived 字段在幂等缓存里必须**穷举**剔除 + 文档 JSON 示例的数学一致性

## 背景

Story 20-1（接口契约最终化）r9 由 codex 复审 r8 已经把"幂等 + 设计文档跨章节一致性"压实之后**剩余**的两个细节问题：

1. **P1**：r6 把 `nextChest.remainingSeconds` 锁定为"不缓存到 `response_json`，回放时实时计算"，但**漏掉** `nextChest.status` 也是 time-derived（与 §7.1 GET /chest/current 同一规则：`status = (unlock_at > now) ? 1 : 2`）—— 同 key 重试若发生在新 chest 已到期解锁的时刻，回放 stale `status=1` 而仅重算 `remainingSeconds=0` 会形成自相矛盾的"counting 状态 + 剩 0 秒"不可能组合。
2. **P2**：§7.2 成功响应 JSON 示例的 `stepAccount` 三字段写成 `totalSteps=12560 / availableSteps=740 / consumedSteps=400`，违反 accounting model `totalSteps = availableSteps + consumedSteps`，会被实装者抄进 fixture / 测试数据 / iOS Codable mock 里污染下游。

两条都是文档级问题。P1 是"时间派生字段穷举不全"导致的契约层 stale-state inconsistency；P2 是"示例 JSON 字面数学错误"导致的 spec 污染。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | nextChest.status 同样是 time-derived，必须从 response_json 缓存剔除 + 回放时与 remainingSeconds 同源同时刻重算 | P1 / high | docs / architecture | fix | `docs/宠物互动App_V1接口设计.md` §7.2 / §13.2 / §13.3 + `docs/宠物互动App_数据库设计.md` §5.16 |
| 2 | §7.2 成功响应示例的 stepAccount 三字段数学不自洽（违反 `totalSteps = availableSteps + consumedSteps`） | P2 / medium | docs | fix | `docs/宠物互动App_V1接口设计.md` §7.2 JSON 示例 |

## Lesson 1: nextChest.status 同样是 time-derived，必须从 response_json 缓存剔除 + 回放时与 remainingSeconds 同源同时刻重算

- **Severity**: P1 / high
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §7.2 字段表 line ~1041 + 服务端逻辑步骤 3j / 4 / 5 / 6 + 关键约束 cached response_json schema + client 重试策略 + §13.2 / §13.3；`docs/宠物互动App_数据库设计.md` §5.16 `response_json` 字段说明

### 症状（Symptom）

同 key 重试在新 chest 已到期解锁的时刻发生时，cached replay 路径会返回：

- `nextChest.status = 1 (counting)`（回放首次时刻的 stale 值，因为 r6 只把 `remainingSeconds` 移出缓存，**没**移出 `status`）
- `nextChest.remainingSeconds = 0`（r6 已要求回放时实时计算，按 `max(0, ceil((unlock_at - now) / 1s))` 现实时间 `unlock_at <= now` 算出 0）

这两个字段同时返回构成**不可能组合**：`counting` 状态本意是"还在倒计时"，但 `remainingSeconds = 0` 说明已到点。同时与 §7.1 GET /chest/current 在同一秒查询返回的 `status = 2 (unlockable) + remainingSeconds = 0` 直接漂移——而 §7.2 字段表又写"`data.nextChest` 字段集与 §7.1 完全一致"，自相矛盾。

### 根因（Root cause）

r6 review 锁定"time-derived 字段不缓存"的原则时，只识别到 `remainingSeconds` 一个字段，**没**做"对照 §7.1 GET /chest/current 的全部计算字段做穷举"。§7.1 服务端逻辑里其实有两条 time-derived 规则同时生效：

```
status = (unlock_at > now) ? 1 : 2
remainingSeconds = max(0, ceil((unlock_at - now) / 1s))
```

这两条共用同一时间输入（`now` vs `unlock_at`），在同一秒的查询下**互相约束**：`status=1` 必然意味着 `remainingSeconds > 0`、`status=2` 必然意味着 `remainingSeconds = 0`。把其中一个字段持久化（stale）而另一个实时算（fresh），就拆散了这条共同约束，产生不可能组合。

更深层的认知漏洞：r6 review 时把 `remainingSeconds` 当作"特殊字段"个例处理（"它有时间衰减语义"），而**没**把它归类为"一切随 `now` 变化的字段都属于 time-derived 类"。`nextChest.status` 表面上是个枚举不是数值，看起来"静态"，但它的取值规则**依赖 `now`**——这才是 time-derived 的本质判定。

### 修复（Fix）

将 `nextChest.status` 也归入"不缓存 + 回放时实时计算"列表，与 `remainingSeconds` **同源同时刻**处理。具体改动：

- **`docs/宠物互动App_V1接口设计.md` §7.2 字段表 `nextChest.status` 行**：将"范围/约束"从 `节点 7 阶段固定 1 (counting)` 改为 `枚举 1 (counting) / 2 (unlockable)（与 §7.1 GET /chest/current data.status 同义对齐）`；"说明"列加入"计算字段，不持久化到 idempotency `response_json` 缓存；server 在响应序列化时按 §7.1 服务端逻辑同一规则实时计算 `(unlock_at > now) ? 1 : 2`"，并明示"若重试发生在新 chest 已到期解锁的时刻按现实时间应返回 2 —— 不能回放首次时刻 stale 1，否则会与 `remainingSeconds` 互相矛盾如 `status=1 + remainingSeconds=0` 这种不可能组合"。
- **§7.2 服务端逻辑步骤 3j**：可缓存 payload 的 `nextChest` 内字段集从 `{id, status, unlockAt, openCostSteps}` 收紧为 `{id, unlockAt, openCostSteps}`；明示 `status` 与 `remainingSeconds` 均为 time-derived 字段，由 server 在响应序列化时实时填入。
- **步骤 4 首次成功路径的响应组装**：补算指令从"仅 `remainingSeconds`"改为"同源同时刻补算 `status = (unlockAt > now) ? 1 : 2` + `remainingSeconds = max(0, ceil((unlockAt - now) / 1s))`"。
- **步骤 5 / 6 事务后处理 + 响应**：cached 回放路径明示 server 端补算 `nextChest.status` 与 `nextChest.remainingSeconds` 两者（同源同时刻）。
- **关键约束 §7.2 cached response_json schema 钦定段**：`{code, message, data: {reward.*, stepAccount.*, nextChest.{id, status, unlockAt, openCostSteps}}}` 调整为 `{code, message, data: {reward.*, stepAccount.*, nextChest.{id, unlockAt, openCostSteps}}}`；说明 `status` + `remainingSeconds` 均不写入缓存的同源同时刻原因。
- **client 重试策略 line ~1115**：cached response 回放时"`remainingSeconds` 按当前时刻实时补算 + `requestId` 填本次重试请求的 trace ID"改成"`status` + `remainingSeconds` 均按当前时刻实时**同源同时刻**补算 + `requestId` 填本次重试请求的 trace ID"。
- **§13.2 幂等规则 line 2873**：第一条 bullet 改成"实时**同源同时刻**补算时间派生字段 `nextChest.status` 与 `nextChest.remainingSeconds`（与 §7.1 GET /chest/current 一致）+ 实时填入当前请求的顶层 `requestId`"。
- **§13.3 持久化存储 line 2887**：`response_json` 不含字段列表明示 `nextChest.status` 与 `nextChest.remainingSeconds` 均不缓存。
- **`docs/宠物互动App_数据库设计.md` §5.16 `response_json` 字段说明**：JSON schema 从 `{code, message, data: {reward, stepAccount, nextChest.{id, status, unlockAt, openCostSteps}}}` 收紧为 `{code, message, data: {reward, stepAccount, nextChest.{id, unlockAt, openCostSteps}}}`；"不包含时间派生字段"列表追加 `nextChest.status`，并明示与 `remainingSeconds` 同源同时刻处理的原因（防 stale `status=1` + 实时 `remainingSeconds=0` 不可能组合）。
- **§5.16 设计说明**：标题从"r5 / r6 / r7 review 锁定"扩展为"r5 / r6 / r7 / r9 review 锁定"；追加 r9 修订段落 + 新增 r9 lesson 文档链接。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在设计**幂等缓存 / 响应回放**类契约时，**必须**先**枚举**所有"依赖 `now` 或外部系统时钟"的字段（即 time-derived 字段），把它们**作为整体类**标记为"不持久化 / 回放时同源同时刻重算"，**不**逐字段个例处理。
>
> **展开**：
> - **time-derived 字段的判定标准**：字段取值规则的输入中包含 `now` / `time.Now()` / `clock.Read()` / 外部时间源 → 即为 time-derived。**不**以"字段是数值还是枚举"判断（枚举字段也可以是 time-derived，如 `status = (deadline > now) ? counting : unlockable`）。
> - **多 time-derived 字段必须同源同时刻计算**：若一次响应里有 2 个以上 time-derived 字段，它们必须共用同一 `now` 快照（如 `now := clock.Now(); status = ...(now); remainingSeconds = ...(now)`）—— 防止两次 `time.Now()` 调用间隙跨过 `unlock_at` 临界点产生内部不一致（如 `status=1 (counting) + remainingSeconds=0`）。
> - **检查对照表**：写完幂等回放契约后必须**反查**对应的 GET 接口（如 chest 的 `POST /chest/open` 必须反查 `GET /chest/current`）—— 该 GET 的字段表里凡有"按服务端规则实时计算"或"`now` / 时间公式"标记的字段，POST 的回放路径都必须**同等处理**为不缓存 + 实时同源重算。
> - **反例 1**：r6 只把 `remainingSeconds` 标为不缓存，遗漏 `status` —— 这就是本 lesson 的根因，**未来禁止**只 fix 触发问题的那一个字段而不做"同类字段穷举"。
> - **反例 2**：写"`response_json` 缓存只剔除 `XXX` 字段"这种 negative list 时只列出**首先想到**的那一个 → 必须改为以"time-derived 字段类"作为 positive 判定 + 列举所有满足判定的具体字段，并在每个字段的字段表说明里加交叉引用。
> - **反例 3**：把 stale 字段和实时字段混在一个响应里返回时**不**主动反查"它们之间有没有不变量"—— 不可能组合的危险藏在多个字段的"语义耦合"里（如 `status` 与 `remainingSeconds` 的耦合是"counting iff remainingSeconds > 0"），单看任一字段都"合法"，组合起来才暴露问题。

## Lesson 2: 文档 JSON 示例必须满足同 schema 下的字段间数学约束（accounting model 自洽）

- **Severity**: P2 / medium
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §7.2 成功响应 JSON 示例 line ~1068-1071

### 症状（Symptom）

示例写成 `totalSteps=12560 / availableSteps=740 / consumedSteps=400`。但 §6.2 GET /steps/account / §6.1 POST /steps/sync / 数据库设计 §5.4 `user_step_accounts` 均钦定 accounting model：

```
totalSteps = availableSteps + consumedSteps   (累计 = 可用 + 已消耗)
```

示例里 `availableSteps + consumedSteps = 740 + 400 = 1140 ≠ 12560`，违反 model。该示例会被实装者复制到 server 单测 fixture、iOS Codable mock、Postman 文档、QA 回归 case 里，导致下游对"哪个字段是权威"产生混淆，或在断言"`total == available + consumed`"的测试里失败。

### 根因（Root cause）

写"开箱后"的响应示例时，专注点在新字段（`reward.*` / `nextChest.*`）的语义上，**没**把"已存在的字段间不变量"作为示例 review 项过一遍。`stepAccount` 三字段是从 §6.2 同义对齐过来的"已存在不变量"，而开箱事务的 UPDATE 语义是 `available -= 1000; consumed += 1000`（`total` 不变），扣完后**仍**满足 `total = available + consumed`。写示例时既没遵循"先取一组数学一致的基准值再写"的纪律，也没在写完后**算一遍**验证。

更普遍的根因：文档 JSON 示例的数值在 Claude 的"创作模式"下倾向于"随手编一个差不多的数"——这个习惯在**有跨字段约束**的 schema 下是危险的。

### 修复（Fix）

把 §7.2 成功响应 JSON 示例的 `stepAccount` 三字段改为数学一致组合：

```diff
     "stepAccount": {
       "totalSteps": 12560,
-      "availableSteps": 740,
-      "consumedSteps": 400
+      "availableSteps": 11160,
+      "consumedSteps": 1400
     },
```

新组合满足 `availableSteps + consumedSteps = 11160 + 1400 = 12560 = totalSteps`。语义解读：用户累计走过 12560 步，本次开箱前 `consumed = 400`，本次开箱扣 1000 → `consumed = 1400`、`available = 12560 - 1400 = 11160`。（语义自洽：扣 1000 这条由 §7.2 服务端逻辑钦定，示例同时演示了扣步数后的快照状态。）

### 预防规则（Rule for future Claude）⚡

> **一句话**：写**任何**文档 JSON 示例时，**必须**先扫描该 schema 是否存在**跨字段数学约束 / accounting model / unique key 关联** —— 若存在，**必须**用满足约束的具体数字组合填示例，**禁止**随手编"看起来差不多的数"。
>
> **展开**：
> - **示例数值的取法**：先在 schema 里找跨字段不变量（accounting / sum / max / min / 同步状态等），再选**一组**数学一致的具体值，最后填进示例。**禁止**逐字段单独想数值。
> - **常见跨字段约束类型**：
>   - **Accounting**：`total = available + consumed` / `balance = credit - debit` / `total = sum(items.amount)`
>   - **范围**：`min ≤ value ≤ max` / `0 ≤ remaining ≤ total`
>   - **同步状态**：`status = (deadline > now) ? A : B`（time-derived，与 Lesson 1 联动）
>   - **枚举一致性**：`status = active 时 expires_at IS NOT NULL`
>   - **同义对齐**：示例里出现的"与 §X 接口同义字段"必须用与 §X 示例**相同**或**至少满足同 schema 约束**的值
> - **写完示例后必须做一次"算账"**：用计算器（或心算）验证示例数值满足上述每条约束。
> - **反例 1**：本 lesson 的 r9 P2 finding 就是没做这一步 —— `12560` 与 `740 + 400` 差了 11420，肉眼明显但写示例时没算。
> - **反例 2**：示例里 `remainingSeconds = 600` + `unlockAt = "2026-04-23T10:35:00Z"` 看起来"差不多"，但响应时间戳 vs `unlockAt` 必须满足 `unlockAt - now = remainingSeconds (s)` —— 写示例时若顶层有时间戳字段必须互相呼应。
> - **反例 3**：开箱后的 `stepAccount` 示例与开箱**前**步数语义混淆 —— 必须明示"这是扣完后的快照"并保证数学等式成立。

---

## Meta: 本次 review 的宏观教训（可选）

r9 是 Story 20-1 的**第 9 轮** codex review。从 r1 → r9 的演进显示了一个清晰的认知模式：每一轮 review 都会发现"上一轮锁定的契约修订**触发**的**新一层**问题"——

- r3 锁定 SET 失败客户端指引 → r4 抓出 24h 卡死 → r5 抓出 Redis 与 MySQL 不能原子写 → r6 抓出预声明在事务外的 pending 卡死 → r7 抓出 best-effort failed upsert race + cached requestId → r8 抓出跨章节 summary 漂移 → **r9 抓出 r6 修复 `remainingSeconds` 时遗漏 `status` 也是 time-derived**。

每一轮 fix 都是局部正确，但都**没**触发"类似问题的同类排查"。r9 的两条 finding 性质完全不同（P1 是 time-derived 穷举漏，P2 是 JSON 示例数学错），但有**同一种**思维特征：**只想到引起当下问题的那一个字段 / 那一组数，没做"同类穷举 / 一致性反查"**。

宏观教训：**契约 finalize 类 story 的 review 阅读策略**——拿到 review 不要只 fix 它点出的那个字段，要**反向问**：

1. 这个字段所属的**类**（time-derived / accounting / 同义对齐 / 跨章节 summary）里**还有哪些字段**没被 review 提到？
2. 这些字段是否也存在**同样性质**的问题？
3. 在文档里**全部**修一遍，而不是只修 review 抓出来的那个。

这条 meta 教训在后续 story 32.x（合成事务） / 27.x（房间事务）等同样涉及幂等 + cached response + 多字段约束的 finalize work 上**必须**主动遵循，否则会无限制地走 r10 / r11 / r12 ... review 循环。
