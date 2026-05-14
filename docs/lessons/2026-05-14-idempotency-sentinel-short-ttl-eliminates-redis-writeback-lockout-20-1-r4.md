---
date: 2026-05-14
source_review: "file: /tmp/epic-loop-review-20-1-r4.md (codex review round 4 for story 20-1-接口契约最终化)"
story: 20-1-接口契约最终化
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-14 — 幂等 sentinel 短 TTL 设计消除 Redis 写回失败的 24h 卡死 / 不要把"防重复"的最大代价加在"幂等承诺"身上（20-1 r4）

## 背景

Story 20.1 把 §7.2 POST /chest/open 契约 finalize；前序 round 3 已修了 client 换 key 重试导致重复出箱的 bug，把"SET 失败 / DEL 失败后禁止换新 key"写成绝对禁令。codex round 4 抓出该绝对禁令的代价：

- **[P1] SET 失败 + client 未收到 200**：client 唯一可走的路径是同 key 重试，但 sentinel TTL = 24h → 卡死 24h 才能用任何方式恢复；破坏"幂等重放"承诺最需要兜底的场景（网络层失败）反而成了最坏场景
- **[P2] 事务回滚 + DEL 失败**：事务已回滚（无副作用），但 sentinel 残留 24h → 开箱功能对该 user / key 不可用 24h；这个分支换新 key 本应安全（事务确实未生效），但 round 3 文档机械禁用

两条 finding 指向**同一根因**：sentinel TTL 与 final-response TTL 被合并为单值 24h。本轮通过**双 TTL 设计**（sentinel 60s / final JSON 24h）一次性解决两条。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | SET-after-commit 失败时 client 卡死 24h 破坏幂等重放 | high | architecture / docs | fix | `docs/宠物互动App_V1接口设计.md:953-967, 994-1009, 1115-1134, 1140` |
| 2 | DEL-after-rollback 失败造成 24h lockout（其实事务已回滚，换新 key 安全） | medium | architecture / docs | fix | `docs/宠物互动App_V1接口设计.md:992, 1000-1005, 1140` |

修了 2 条 / defer 0 条 / wontfix 0 条。

## Lesson 1: 幂等 sentinel TTL 必须与 final-response TTL 分离 —— "并发声明短窗口" vs "重放重读长窗口"是两件事

- **Severity**: high
- **Category**: architecture / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §7.2.3 (双 TTL 设计) + §7.2.5 (Redis 回填失败处理) + §7.2 关键约束 (client 重试策略 §1008 + 事务边界 case (c))

### 症状（Symptom）

Round 3 契约把 sentinel `"__pending__"` 和 final response JSON 都用 `EX 86400`（24h TTL）写入同一 Redis key。两种边界 case 因此恶化：

1. **SET-after-commit 失败**：事务已 commit，但步骤 5 SET 失败 → sentinel 仍是 `"__pending__"` + TTL 24h → 同 key 任何重试持续返回 1008 长达 24h；client 端契约又"禁止换新 key"（防重复出箱），用户被卡 24h 才能再发任何开箱请求
2. **DEL-after-rollback 失败**：事务回滚后 sentinel 应被 DEL，但 DEL 失败 → sentinel 残留 24h → 同 key 持续 1008 24h；该分支事务回滚后**完全无副作用**，换新 key 本应立即安全，但契约机械禁用

### 根因（Root cause）

把 sentinel 和 final cache 当作"同一种数据"放进同一 key 同一 TTL 是直觉性陷阱：

1. **两者的语义寿命完全不同**：
   - sentinel 是"事务正在进行中"短时标记 —— 寿命应 ≈ 一次事务的最大正常时延（数毫秒到秒级，60s 是巨大 margin）
   - final JSON 是"幂等重读窗口"长时缓存 —— 寿命应覆盖正常 client 重试 / 用户切前后台 / 弱网重连等场景（小时级）
2. **把它们合并为单值 24h，等价于把所有 Redis 写失败的兜底成本全部加在 client 卡死时间上**：sentinel 24h = client 任何异常路径最长卡死 24h
3. round 3 review fix 的思维链：「换新 key 会重复出箱 → 禁止换新 key → client 只能同 key 重试」—— 没意识到这条链的**前提**是"sentinel 长期残留"；把前提换掉（缩短 sentinel TTL），结论自然变化

### 修复（Fix）

**双 TTL 设计**：

- **sentinel `"__pending__"`**：`SET NX EX 60`（60s 短窗口）
  - 60s 覆盖事务正常时延的 100x margin（事务 P99 < 1s）
  - 事务正常完成时步骤 5 SET 会在 60s 内覆盖 → 实际寿命 = 事务时延
  - 事务异常（DB 卡 / Redis SET 失败 / DEL 失败）→ 60s 内 sentinel 自然过期
- **final response JSON**：步骤 5 SET 时 `EX 86400`（24h 长窗口；覆盖 sentinel + 延长 TTL）
  - 覆盖正常 client 网络重试、用户重连、跨 app session 重读等场景

**配套调整**：

1. **server 端**：步骤 5 SET / DEL 失败 → 异步重试 3 次（指数退避 100ms / 500ms / 2s）；仍失败则 log.Error + counter，但**不**回滚 commit、**不**做补偿
2. **client 端 1008 重试策略**（契约层硬要求 + 伪代码）：
   - **60s 内**：同 key 短退避（2s / 4s / 8s ≤ 30s 累计）
   - **60s 外**：放弃同 key + 显式换**新** idempotencyKey + **UI 强制提示**「奖励可能已发放，请刷新页面；如未到账请联系客服」
3. **case (b) DEL 失败**：最坏 = 60s 卡死（不再是 24h），事务已回滚故 60s 后换新 key / 同 key 都安全
4. **case (c) SET 失败**：60s 内同 key 退避；60s 外允许换新 key + UI 提示（牺牲极罕见"60s 外仍 SET 失败"重复出箱风险，换取所有 client 卡死场景的 60s 自愈上限）
5. **tech debt 登记**：未来加 server 端后台 reconcile job 扫 `chest_open_logs` vs Redis final JSON 一致性，补写丢失的 final JSON

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计基于 Redis 的幂等键 / sentinel + 持久结果缓存** 时，**必须**把 **sentinel 短窗口 TTL** 与 **final-response 长窗口 TTL** **物理分离设计**（同 key 不同 TTL 的覆盖语义，或不同 key 完全分离），**禁止**把两者合并为单一 TTL。
>
> **展开**：
> - sentinel TTL = "事务正常时延的 N × margin"（典型 30s ~ 120s），用于"声明并发执行中"，**唯一目的**是让同 key 并发请求知道有同伴在跑
> - final-response TTL = "client 端合理重读窗口"（典型小时 ~ 天级），用于"重放幂等读"
> - 步骤 5 的 SET 操作天然就是"覆盖 sentinel + 延长 TTL"双语义合一（用同一 key 不同 TTL 的两次 SET 完成）
> - **设计 sentinel 时问的第一个问题不是"TTL 是多长"，而是"sentinel 残留的最坏后果是什么"**：如果残留会卡死 client → TTL 必须短；如果残留只是"幂等读不到"（client 已收到首发响应）→ 不存在"卡死"，但仍然没理由让 sentinel 寿命 > 事务正常时延
> - **反例**：把 sentinel 和 final cache 都用同一 TTL（如 24h）写 —— 等价于把"server 端写 Redis 任何失败的代价"全部转嫁到"client 端最长卡死时间"，违反"幂等承诺"对"网络层失败"场景的兜底初衷
> - **反例**：在 review 中机械接受"禁止换 key"作为防重复出箱的银弹 —— 没意识到"禁止换 key"的代价 = sentinel 卡死时间窗（如果窗口长，代价巨大）；把这两件事当独立约束看待会让任一变量变化时另一变量错配
> - **反例**：把"server 端 Redis 写失败"假设为不可恢复的 best-effort —— 应有最低限度的异步有限重试（N = 3，指数退避）+ counter 告警，让"瞬时 Redis 抖动"不会一次性卡死 client；保留长期 reconcile 兜底（即使 MVP 不实装也要 tech debt 登记）

## Lesson 2: 防止 "重复副作用" 的契约约束代价必须用 "可恢复时间窗口" 度量，不是绝对禁令

- **Severity**: medium
- **Category**: architecture / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §7.2 关键约束 (client 重试策略 §1008)

### 症状（Symptom）

Round 3 把 case (c) (SET-after-commit 失败) 的 client 重试约束写成「**禁止**改用新 idempotencyKey 重试」绝对禁令。该禁令孤立看正确（换新 key 会重新进事务 → 重复扣步数 + 重复出箱），但与"sentinel TTL = 24h"耦合后，client 实际可走路径只剩"同 key 退避 N 次后放弃"—— 把"防重复"的代价加在"幂等承诺"上：用户为了避免概率极小的重复扣费，被迫承担 24h 开箱不可用。round 4 review 把这件事打回来。

### 根因（Root cause）

写"禁止 X"型契约约束时，没量化"X 被禁止后 client 的兜底路径有多痛"：

1. 把"防止 server 端坏行为"和"client 端用户体验"当成独立约束权衡 —— 实际上两者是耦合的（"防止重复出箱"的代价就是"用户卡死时间"）
2. 没有把"换新 key 安全 / 不安全"细分到具体边界 case：
   - case (b) DEL 失败：事务回滚后**完全安全**（首发未生效）—— round 3 仍机械禁用是过严
   - case (c) SET 失败 + 60s 内：不安全（事务可能 commit 在前 60s 内）—— 禁用合理
   - case (c) SET 失败 + 60s 外：极罕见（要求 server 端 3 次异步重试都失败）—— 此时换 key 风险低（重试次数 + 时间窗约束已收敛）+ UI 提示给用户知情权 → 允许换 key 是更平衡的兜底
3. 绝对禁令简单好记，但简单不等于正确 —— "禁止" + "可观察的成本"才能让 reviewer 看到代价

### 修复（Fix）

把 §7.2「client 重试策略」§1008 条款从「禁止换新 key」改为**时间分段策略**：

- **60s 内**：必须同 key 退避（强语义，对应 sentinel 短窗口内的不确定性）
- **60s 外**：允许 + 推荐换新 key + **UI 强制提示**（给用户知情权，让"可能重复发放"成为已沟通风险）
- 同时把决策细节（"用 X 个代价换 Y 个收益"）写进契约 ——「牺牲极小概率的'60s 外仍 SET 失败'重复出箱风险，换取所有 client 卡死场景的 60s 自愈上限」

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **契约层写"禁止 X"约束** 时，**必须**同时写出 **"X 被禁止后 client / 用户兜底路径的最坏代价"**（时间窗口、可恢复性、用户感知），并在该代价大于 X 本身造成的坏行为概率时**优先选时间分段策略**而非绝对禁令。
>
> **展开**：
> - "防重复副作用"约束的代价 ≈ "用户卡死时间 × 用户感知"，量化后再决定禁令强度
> - 时间分段策略的标准模板：「在 [短窗口 T_short] 内做 [严约束]；T_short 外允许 [松约束] + [UI 提示用户知情]」—— 用 UI 提示把"残余风险"转化为"已沟通的运营成本"
> - 写"禁止"约束时**必须**配一句"否则会发生 X" —— 没有"否则"的禁令是教条
> - **反例**：「client 禁止改用新 idempotencyKey 重试」+ 不说"否则会重复扣步数 + 重复出箱"+ 不说"sentinel TTL 是 24h 所以禁令的代价是 24h 卡死" —— 看不到代价的约束让 review 无法权衡
> - **反例**：契约只列约束不列约束代价 —— Story 20.6 实装者读到「禁止换 key」就照做，不会回头质疑"代价是什么"；当代价不可接受时 review round 才能抓出来已晚（round 3 → round 4 来回）
> - **反例**：把"server 端坏行为"和"client 端用户体验"分两节写而不显式 link —— 两者本质耦合，应该在同一段里写出 trade-off

---

## Meta: 本次 review 的宏观教训

Round 3 → round 4 的来回展示了**契约层的两个相互拉扯的约束**（"防重复副作用" vs "幂等承诺"）：

- round 1 / round 2：契约层抓"server 端原子声明 / 跨文档一致性"等基础问题
- round 3：抓"client 换新 key 会重复出箱" —— 加上「禁止换新 key」约束
- round 4：抓"禁止换新 key + 24h TTL → client 卡死 24h" —— 缩短 sentinel TTL + 时间分段策略

**核心抽象**：**契约约束之间会形成 "约束链"，链中任一节点的代价变化（如 TTL 从 24h 缩到 60s）会让整条链的最优解改变。** 写契约时应该把"链上的相互依赖"显式标出来（如本轮 §7.2.3 / §7.2.5 / §7.2「client 重试策略」三处之间的 cross-reference），让未来读者看到「这个约束是为了配合另一个约束而存在的」，而不是孤立信条。

写"禁止 X"型约束 + "代价 Y" + "代价来源 Z 的可调性" 是后续 contract review 的标准 checklist：

1. 禁止什么（X）
2. 不禁止的话会发生什么（X 的坏行为）
3. 禁止后 client / 用户的兜底路径是什么（兜底路径的代价 Y）
4. 代价 Y 是哪些变量决定的（如 TTL 长度 / 重试次数）（变量集 Z）
5. Z 是否可调 —— 如果可调，先调 Z 再决定 X 的禁令强度
