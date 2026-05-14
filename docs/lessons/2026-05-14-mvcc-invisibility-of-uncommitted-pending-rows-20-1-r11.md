---
date: 2026-05-14
source_review: codex review round 11 on docs/宠物互动App_V1接口设计.md §7.2 (POST /chest/open) idempotency flow refactor
story: 20-1-接口契约最终化
commit: c0c21a0
lesson_count: 2
---

# Review Lessons — 2026-05-14 — MVCC 决定 autocommit SELECT 看不到首事务未 commit 的 pending 行 + cached success 路径必须覆盖所有 time-derived 字段（20-1 r11）

## 背景

Story 20-1 在 r10 review 锁定"rate_limit 检查从 middleware 挪到 handler 内层、置于幂等命中预检之后"的设计，承诺 committed success replay + pending 命中两类同 key 重试都免 rate_limit 配额。r10 还把步骤 3（autocommit SELECT 检查 idempotency 行 status）作为 "命中 success → 返回 cached / 命中 pending → 返回 1008 + 免配额 / 未命中 → fall-through" 三分支预检。同时 r9 锁定 `response_json` 缓存不包含 `nextChest.status` / `nextChest.remainingSeconds` 两字段（同源同时刻实时计算），但 r10 改造步骤 5b cached success 短路返回路径文字时只补 `remainingSeconds` 漏 `status`。

Codex r11 review 给出两条 P1：

1. **步骤 5b cached success 短路只补 remainingSeconds 漏 status**：与 r9 锁定的"两字段同处理"决策不一致，会让同 key 重试在新 chest 已到期解锁时刻返回 `status=1` + `remainingSeconds=0` 不可能组合
2. **pending precheck 在 autocommit SELECT 下不可行**：InnoDB MVCC 默认 REPEATABLE READ 让 autocommit 的 read view 看不到首事务尚未 commit 的 pending 行 → 步骤 3 在 pending 阶段同 key 重试场景永远命中"未命中"分支 → fall-through 到 rate_limit + 业务事务 → 步骤 5a INSERT 在 unique-key X-lock 上阻塞等待首事务 commit → 拿 `affected_rows = 0` → 步骤 5b 短路读 success 返回 cached。**问题**：本路径下第二请求已被 rate_limit 计入 quota，违反 r10 "pending 命中免配额" 承诺。这是 MVCC 决定的硬约束，不可在 MySQL 默认隔离级别下规避。

本次 fix 重构 §7.2 服务端逻辑步骤 3 / 4 / 5、错误码表 1005 / 1008 / 1009、关键约束段、§1 冻结声明、§13.2 / §13.3、数据库设计 §5.16 / §8.3。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | r10 步骤 5b cached success 短路只补 remainingSeconds 漏 status | P1 | architecture/docs | fix | `docs/宠物互动App_V1接口设计.md` §7.2 |
| 2 | pending precheck 在 MVCC autocommit SELECT 下不可行 → r10 "pending 免配额" 承诺不可实现 | P1 | architecture/docs | fix | `docs/宠物互动App_V1接口设计.md` §7.2 + `docs/宠物互动App_数据库设计.md` §5.16 / §8.3 |

## Lesson 1: time-derived 字段必须穷举处理所有 cached success 短路返回路径

- **Severity**: P1
- **Category**: architecture / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §7.2 服务端逻辑步骤 5b（line ~967）

### 症状（Symptom）

r9 review 锁定 `response_json` 缓存不包含两个 time-derived 字段（`nextChest.status` + `nextChest.remainingSeconds`），二者在响应序列化时按"当前时刻"同源同时刻实时计算填入，防止 stale `status=1` + 实时 `remainingSeconds=0` 不可能组合。

但 r10 改造步骤 5b cached success 短路返回路径时，**只补 remainingSeconds 漏 status**：

```
status = 'success' → COMMIT → 反序列化 response_json + 补算
data.nextChest.remainingSeconds = max(0, ceil((nextChest.unlockAt - now) / 1s))
+ 填入当前请求的 requestId → 返回 200
```

导致同 key 重试发生在新 chest 已到期解锁时刻时，步骤 5b 返回 `status=1` (来自 stale cache) + `remainingSeconds=0` (实时计算) 不可能组合，与 §7.1 GET /chest/current 同一秒查询返回的 `status=2` + `remainingSeconds=0` 漂移。

### 根因（Root cause）

"字段集决策"（r9 锁定缓存内容不含两字段）与"短路返回路径决策"（r10 改造代码路径文字）是两个独立的更新点，r9 → r10 演进时只更新了字段集决策，**没有同步更新所有 cached success 短路返回路径上的字段补算列表**。

具体来说，§7.2 有**两条** cached success 短路返回路径：

- **步骤 3 committed success replay 路径**（r10 新增，autocommit SELECT 命中）
- **步骤 5b cached 路径**（r6 起就有，事务内 SELECT 命中）

r10 给步骤 3 补全了两字段（与 r9 一致），但**只给步骤 5b 补了 remainingSeconds 漏 status** —— 显然是 r10 作者关注步骤 3 新路径设计时，没把"r9 同源同时刻"决策同步应用到既有的步骤 5b 路径。

这是"分散决策点 → 易遗漏穷举"模式：契约文档里"哪些字段不缓存"的决策点（在 §5.16 + §13.3 + 关键约束段）和"具体短路返回路径如何处理字段"的决策点（在 §7.2 服务端逻辑步骤 3 / 5b）在文档结构上分开，文字层面没有显式 cross-reference，导致 r9 → r10 演进时遗漏。

### 修复（Fix）

步骤 5b cached success 短路返回路径**显式补齐两字段**：

```
status = 'success' → COMMIT → 反序列化 response_json + 补算
data.nextChest.status = (unlock_at > now) ? 1 : 2
+ data.nextChest.remainingSeconds = max(0, ceil((nextChest.unlockAt - now) / 1s))
（两字段**同源同时刻**计算，用同一个 now 快照；与步骤 3 cached 路径 + §7.1 GET /chest/current 完全对齐）
+ 填入当前请求的 requestId → 返回 200
```

同时在 §13.2 / §13.3 / DB §5.16 / §8.3 反复强调"所有 cached success 短路返回路径都必须同源同时刻补算两字段，**缺一不可**"，避免未来 review 演进时再次遗漏某条短路路径。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **修改"缓存层不持久化 time-derived 字段，由响应序列化时实时计算填入"类决策时**，**必须穷举更新所有可能的"短路返回点"上的字段补算列表**，而**不是**只更新最新一处。

> **展开**：
> - 当文档锁定"字段 X / Y / Z 不入缓存，由响应序列化时同源同时刻实时计算"决策时，必须列出**所有**可能的"短路返回点"（一切返回 cached payload 的位置 —— 包括 handler 入口 autocommit SELECT 命中、事务内 SELECT 命中、Redis 命中、等等）
> - **每条短路返回点都必须显式列出"补算字段集"**，且**必须包含决策中的所有 time-derived 字段**
> - 如果新增一条短路返回路径（如 r10 新增步骤 3 committed success replay），**必须同步检查**：既有路径（如步骤 5b）是否还遗漏字段
> - 在"关键约束 / 不变量"段显式声明 "所有 cached success 返回点必须 covers 字段集 S = {X, Y, Z}"，让未来 review 能自动校验
> - **反例 1**：r9 锁定缓存不含 nextChest.status 但 r10 改步骤 5b 文字时只补 remainingSeconds 漏 status —— 同一字段集决策没有 systematic 应用到所有短路点
> - **反例 2**：未来若新增"Redis 缓存 short-TTL hot-path"作为第三条短路返回路径，必须同时给 status 和 remainingSeconds 补算，不能只补一个
> - **反例 3**：未来若改成"缓存中持久化 status / remainingSeconds 但加 TTL 控制 stale"，必须显式权衡（TTL < min(unlockAt - now) 保证不漂移 vs 实时计算的简洁性），不能默认按缓存读取

## Lesson 2: MVCC 决定 autocommit SELECT 看不到首事务未 commit 的 pending 行

- **Severity**: P1
- **Category**: architecture / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §7.2 服务端逻辑步骤 3（line ~953-957）

### 症状（Symptom）

r10 设计步骤 3（handler 入口幂等命中预检，autocommit SELECT chest_open_idempotency_records）为三分支：

- 命中 `status = 'success'` → 返回 cached + 免 rate_limit
- 命中 `status = 'pending'` → 返回 1008 + 免 rate_limit
- 未命中 → fall-through 到 rate_limit + 业务事务

**问题**：第二分支 "命中 pending → 1008 + 免 rate_limit" 在 InnoDB 默认隔离级别下**不可实现**：

1. 首事务 BEGIN → INSERT pending row → 此时 pending 行**仅对首事务自己的 read view 可见**
2. 第二请求 autocommit SELECT WHERE user_id=? AND idempotency_key=? → InnoDB MVCC（默认 REPEATABLE READ）让 autocommit 的 read view 仅观察"SELECT 启动时刻已 commit 的事务"的修改 → **看不到首事务的 pending 行** → 返回空结果集
3. 步骤 3 判定为"未命中" → fall-through 到 rate_limit + 业务事务
4. 步骤 5a INSERT 在 unique-key X-lock 上阻塞排队 → 首事务 commit 后第二事务拿 `affected_rows = 0` → 走步骤 5b 短路读 success 返回 cached

**后果**：r10 关于"pending 命中免 rate_limit"的承诺**无法实现** —— pending 阶段同 key 重试会被 rate_limit 计入 quota（虽然业务正确性由 X-lock 兜底）。

### 根因（Root cause）

设计者假设 autocommit SELECT 能观察到"另一连接的事务在进行中"的中间状态，这违反 InnoDB MVCC + REPEATABLE READ 隔离级别的核心语义：

- **MVCC read view 语义**：每个事务（含 autocommit 单语句事务）在第一个 SELECT 时获取一个 snapshot，该 snapshot 仅包含 "在 snapshot 时刻已 commit 的事务" 的修改
- **REPEATABLE READ 默认**：MySQL InnoDB 默认隔离级别，autocommit SELECT 不能读 uncommitted 数据
- **更深的根因**：在 MySQL 默认隔离级别下，**没有任何方式**让 server A 的 autocommit SELECT 观察到 server A 上另一连接持有的未 commit pending 行（除非降级到 READ UNCOMMITTED，但那会引入"看到不存在的状态"更严重 bug）

设计者的思维漏洞 = 把"事务持锁"和"事务持锁但其他连接可见"混淆 —— 持锁阻塞的是**修改 / 锁等待**，不是**读可见性**。MVCC 让读取者看到"snapshot 时刻已 commit 的数据"，与锁状态无关。

### 修复（Fix）

接受 MVCC 不可见为协议层硬约束，整体重构 §7.2 步骤 3 / 4 / 5：

**步骤 3** 改为 "committed success 幂等命中预检"（仅检查 `status = 'success'`）：

- 命中 success → 返回 cached + 免 rate_limit（这是 server 端能 100% 保证的契约）
- 未命中（含真未到达 / pending 不可见两类）→ fall-through 到步骤 4

**步骤 4** 对**所有**未命中 committed success 的请求做 rate_limit（pending 阶段同 key 重试**会**计入 quota，是 MVCC 硬约束下的"次优契约"）

**步骤 5a** INSERT ... ON DUPLICATE KEY UPDATE 在 unique-key X-lock 上阻塞排队兜底业务正确性 —— 首事务结束后第二事务自然分支到 success cached（commit 路径）或全流程（rollback 路径）

**步骤 5b** 显式记录 "理论上不会观察到 `status = 'pending'`"（X-lock serialize 排队 + commit 推进 status 与释放锁原子），万一 driver bug 等读到 pending → 返回 1009（非 1008）

**1008 错误码在节点 7 阶段本接口退役**（无可达路径），全局错误码定义保留供未来扩展

**client 重试策略**：首次网络层重试间隔 ≥ 200ms 覆盖业务事务时长避开 pending 窗口；撞 1005 时退避到下个限频窗口（60s）

文档同步更新：§1 冻结声明 / §13.2 / §13.3 / DB §5.16 / DB §8.3

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计涉及"另一事务进行中状态可见性"的协议时**，**必须首先确认 MVCC + 隔离级别 + read view 语义对该可见性的约束**，**禁止**假设 autocommit SELECT 能看到未 commit 数据。

> **展开**：
> - 在 InnoDB 默认 REPEATABLE READ 下，autocommit SELECT 仅观察"snapshot 启动时刻已 commit 的事务"的修改 —— 看不到任何并发事务的 pending / in-flight 数据，无论锁状态如何
> - 设计幂等 / 锁 / pending 状态可见性时，**必须区分**"持锁阻塞写"（锁等待，影响 INSERT / UPDATE 的并发）vs "MVCC read 可见性"（snapshot，影响 SELECT 看到什么）—— 两者是正交概念
> - 若设计需要"另一事务进行中状态可见"，可选方案及代价：
>   - **降级到 READ UNCOMMITTED**：可见但会引入"看到 rollback 前的脏读"更严重一致性 bug，**禁止**
>   - **应用层 in-memory 跟踪表（如 Redis）**：与"事务原子性"冲突，需评估同步成本和一致性 risk
>   - **由 InnoDB unique-key X-lock 兜底**：让并发请求 serialize 排队（如本案 r11 决策）—— 业务正确性兜底，但接受"无法识别 pending 状态"作为协议层约束
>   - **首事务尽快 commit pending 行（auto-commit-pending）**：要求把幂等记录拆出业务事务 → 引入新风险（rollback 时 pending 残留，r6 review 的悖论），**禁止**
> - **优先采纳 "由 X-lock 兜底" 方案**：让 InnoDB unique-key X-lock 阻塞排队，业务正确性由事务原子性保证；接受"应用层无法识别 pending"作为协议层公开约束（在文档显式说明 + client 应对策略 ≥ 200ms 退避）
> - **反例 1**：r10 步骤 3 设计 "命中 pending → 1008 + 免 quota" 分支 —— 假设 autocommit SELECT 能看到 pending 行，MVCC 不允许
> - **反例 2**：未来若设计 "long-running 事务进度条 / WS push pending status" 类功能，**禁止**靠 server 端 autocommit SELECT 查 pending；要么 client 用同事务 / 同连接 SELECT（不实用）/ 要么用 Redis pub-sub / event stream 推送进度（应用层 push），不能靠 MVCC SELECT 拉
> - **反例 3**：未来若 review 提议"再加一层 pending precheck 让 client 早返回 1008"，必须先验证 MVCC + 隔离级别约束 —— 若无法实现，明确拒绝
> - **预防规则的兜底**：在文档"事务边界 / 隔离级别"章节显式声明项目使用的 MySQL 隔离级别（InnoDB 默认 REPEATABLE READ），并在所有"跨事务可见性"决策点 cross-reference 该声明

---

## Meta: 本次 review 的宏观教训（可选）

r3 → r4 → ... → r11 跨 10 轮 review 演进过程显示，**复杂协议设计**（幂等 + 限频 + 事务原子性 + MVCC + 锁）容易在每轮 review 修补一个 bug 时引入下一个 bug：

- r3：Redis sentinel TTL 24h → r4 改 60s
- r4：Redis 非事务存储 SET-fail-after-commit → r5 改 MySQL 同事务持久化
- r5：业务事务外预声明 → r6 改纳入业务事务
- r6：保留 best-effort failed upsert → r7 移除（race condition）
- r6：缓存 `remainingSeconds` → r6 改不缓存
- r6/r7：漏掉 `nextChest.status` 同样是 time-derived → r9 锁定两字段同处理
- r9：rate_limit 走 middleware 同 key 重试卡死 → r10 挪到 handler 内层
- r10：步骤 5b 改造路径漏补 status / 假设 pending precheck 可行 → r11 修复两条

**宏观教训**：设计涉及多个正交决策点（缓存内容 / 短路返回路径 / 锁可见性 / 事务原子性 / rate_limit 位置）时，**必须建立"决策点矩阵"**：每个决策点显式列出所有受影响位置，每次演进时按矩阵 grid 验证一遍 —— 避免每轮 review 只补"看得见"的那一处，漏掉对称位置上的等价问题。

未来 Claude 在做契约文档迭代时（不止本接口），都应优先考虑"决策点是否已 systematic 应用到所有可能位置" —— 与 review 提出的具体 finding 同等重要。
