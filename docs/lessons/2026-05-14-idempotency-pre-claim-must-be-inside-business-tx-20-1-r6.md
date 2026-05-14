---
date: 2026-05-14
source_review: /tmp/epic-loop-review-20-1-r6.md（codex round 6）
story: 20-1-接口契约最终化
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-14 — 同事务幂等的"预声明也必须事务内" & 时间派生字段不可缓存进 response_json

## 背景

Story 20.1（节点 7 宝箱接口契约 finalize）round 6 review，针对 r5 review 锁定的"MySQL 同事务幂等持久化"方案做正确性审查。r5 把开箱接口的幂等记录从 Redis 上移到 MySQL，并把"幂等记录 UPDATE 与业务事务原子提交"作为核心契约。本轮 review 发现 r5 的实现细节里仍有两条悖论：(1) 预声明 INSERT 写在业务事务**之外**作独立 INSERT，rollback 时 pending 行不跟随回滚 → 同 key 永久 1008 卡死；(2) `response_json` 缓存里包含了时间派生字段 `nextChest.remainingSeconds`（在另一处字段定义里又明确说"按 `max(0, ceil((unlock_at - now) / 1s))` 实时计算"），同 key 重试时回放的 600 秒倒计时与 `GET /chest/current` 实时计算结果漂移，让 client 倒计时错乱。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | pre-claim INSERT 写在事务外 → rollback 不撤销 pending 行 → 同 key 永久卡 1008 | high | architecture | fix | `docs/宠物互动App_V1接口设计.md` §7.2 服务端逻辑 / 关键约束 / §1 契约冻结 / §13；`docs/宠物互动App_数据库设计.md` §5.16 / §8.3 |
| 2 | `response_json` 缓存包含 stale 的 `nextChest.remainingSeconds`（时间派生字段） | medium | architecture | fix | `docs/宠物互动App_V1接口设计.md` §7.2 步骤 3j / 响应体 `nextChest.remainingSeconds` 字段表 / §1 契约冻结 / §13.3；`docs/宠物互动App_数据库设计.md` §5.16 字段说明 |

## Lesson 1: 同事务幂等持久化里，预声明 INSERT 也必须在业务事务内 —— 否则 rollback 留下 pending 行让同 key 卡死

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §7.2 服务端逻辑步骤 3（r5 版本：事务外独立 INSERT；r6 修订为事务内首条语句）

### 症状（Symptom）

r5 钦定的"DB 同事务幂等"在以下时序下表现错误：

```
t0: client 发 POST /chest/open with key=K
t1: server 步骤 3 INSERT idempotency 行 (status='pending')   ← 独立事务，立即 commit
t2: server 步骤 4 BEGIN 业务事务
t3: 步骤 4a/4c FOR UPDATE 拿不到 chest 行 → ROLLBACK → 返回 4001
   （或步骤 4d 步数 UPDATE 乐观锁失败 / 4e cosmetic_items 空 / 4j commit 失败 etc.）
t4: client 用同 key 重试 POST /chest/open
t5: server 步骤 3 INSERT 撞 UNIQUE → affected_rows=0 → SELECT 看到 status='pending'
t6: 返回 1008 幂等冲突
t7: client 同 key 继续退避重试 → 永远命中 status='pending' → 永久卡 1008
```

文档同时把"事务后写一行 failed 占位"标为 best-effort（说明作者已察觉但没根治），这条 best-effort 路径失败时同 key 永久 1008。

### 根因（Root cause）

把"同事务持久化幂等"理解为只覆盖"最终化 UPDATE 与业务表写入同事务"，**漏掉了预声明 INSERT 这一前置步骤**。r5 的论证："INSERT 在事务前用 UNIQUE 约束做原子声明 + UPDATE 在事务内做最终化"看似闭环，但忽略了一个关键时序：业务事务 rollback 时，事务外的 INSERT 已经被独立提交，**rollback 无法撤销它**。结果是"幂等状态机 (pending → success/failed)"的状态转移失去了"rollback 自动清除 pending"这条边，必须靠 best-effort 补偿事务去推进 pending → failed —— 而补偿事务失败时整个状态机卡死。

这是"看似单一可信源、实际是两段写入"的反模式：
- 第一段写入（INSERT pending）：独立事务，已提交，不可回滚
- 第二段写入（业务事务 + UPDATE success）：业务事务，可回滚

两段无法跨事务原子化。**只有把两段都放进同一个业务事务**，才能让"幂等状态 + 业务数据"真正成为单一可信源。

### 修复（Fix）

把预声明 INSERT 移进业务事务，作为事务内**首条语句**：

```sql
BEGIN;
INSERT INTO chest_open_idempotency_records (user_id, idempotency_key, status, ...)
  VALUES (?, ?, 'pending', NULL, NOW(3), NOW(3))
  ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id);

-- 路径 A: affected_rows = 1 → 行不存在（含首次到达 + 首次 rollback 后到达）
--                          → 继续业务步骤 (FOR UPDATE chest + step_account + ...)
--                          → UPDATE idempotency 行 status='success' + response_json=?
--                          → COMMIT

-- 路径 B: affected_rows = 0 → 行已存在（首次事务已 commit）
--                          → SELECT status, response_json
--                            - 'success' → COMMIT → 返回 200 + cached response（补算 remainingSeconds）
--                            - 'failed'  → COMMIT → 返回 1009
--                          → 不进业务表写入
ROLLBACK / COMMIT;
```

关键技术依据：**InnoDB 对 unique key 取排他锁的语义**。同 key 并发请求中只有一个事务能拿到 unique-key X-lock，其他事务**在 INSERT 语句上阻塞**等待，直到首个事务 commit / rollback 释放锁后再继续：

- 首个事务 commit → 锁释放 → 其他事务的 INSERT 看到行已存在 → `affected_rows = 0` → 走短路返回 200-cached / 1009-failed
- 首个事务 rollback → 锁释放 + pending 行回滚消失 → 其他事务的 INSERT 看到行不存在 → `affected_rows = 1` → 走业务全流程，等价于首次到达

并发请求的"1008 幂等冲突中"由 InnoDB 锁阻塞承担，client 看到的不是 1008 而是"请求耗时增加"，事务结束后直接拿到 final 状态。1008 退化为极窄 race 兜底语义（步骤 3b SELECT 在 read-uncommitted 异常下读到 pending 时返回）。

跨文档同步修订：
- `docs/宠物互动App_V1接口设计.md` §7.2 服务端逻辑（步骤 3 重写为事务内 a-l 子步骤）+ §7.2 关键约束「事务边界」+ §1 节点 7 契约冻结清单 + §13.2 / §13.3
- `docs/宠物互动App_数据库设计.md` §5.16 索引说明 + §8.3 开箱事务步骤

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 设计"DB 同事务幂等持久化"方案时，**必须**把"预声明 INSERT + 业务写入 + 最终化 UPDATE"**三步都放进同一个事务**，依赖 InnoDB unique-key X-lock 做并发阻塞排队，**禁止**把预声明 INSERT 作为业务事务外的独立 INSERT。
>
> **展开**：
> - 同事务幂等的核心承诺是"幂等记录 + 业务数据"是单一可信源。这一承诺要求**所有**对幂等行的写入操作都在业务事务内 —— 缺一个就会形成"已落盘但业务回滚"或"业务已落盘但幂等未更新"的悖论。
> - 同 key 并发安全靠 InnoDB unique key 的 X-lock 阻塞排队实现，**不**靠应用层 `affected_rows` 三态分支。`affected_rows` 三态分支是用来识别"首个事务以 commit 还是 rollback 结束"，**不**是用来识别"同 key 是否首次到达"。
> - rollback 后**不需要**写 `status='failed'` 占位来锁定 client；rollback 已经让 pending 行消失，下次同 key 重试等价于首次到达。写 failed 占位是 UX 优化（让 client 立即收到 1009 而不是走完整流程），**不是 safety 前提**。**禁止**把"事务后写 failed 占位"当作必须的兜底，让它在 best-effort 失败时还成立。
> - **反例**：r5 设计写的是「步骤 3 INSERT pending（事务外独立 SQL）→ 步骤 4 BEGIN 业务事务 → 步骤 4i UPDATE success → 步骤 4j COMMIT；rollback 路径 best-effort 写 failed」—— 这种"事务外预声明 + 事务内最终化"两段式是错误模式：rollback 无法撤销事务外的 INSERT，状态机被卡在 pending。**禁止**复刻该模式到 compose / equip / room 等其他需要同事务幂等的事务接口。
> - **反例 2**：依赖应用层逻辑（如"轮询、tx 内 SELECT"）做"等待首个事务结束"是错的；应该直接利用 InnoDB unique-key X-lock 的天然阻塞语义。

---

## Lesson 2: 时间派生字段（counters / TTL / remainingSeconds）不可写入 idempotency response 缓存，必须回放时实时计算

- **Severity**: medium
- **Category**: architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §7.2 步骤 3j（原 4h，r6 重命名）+ 响应体 `data.nextChest.remainingSeconds` 字段表

### 症状（Symptom）

`response_json` 缓存里包含 `nextChest.remainingSeconds`（首次成功时刻计算的 ≈ 600）。client 同 key 重试可能发生在几秒 / 几分钟后：

```
t0:  首次成功 → response_json 写入 {nextChest.remainingSeconds: 600, unlockAt: "10:35:00Z"}
t300 (5 分钟后): client 网络抖动重试同 key
              → server 步骤 3 SELECT 'success' → 反序列化 response_json → 返回 600
              → 但此时 GET /chest/current 实时计算 = max(0, ceil((10:35:00Z - 10:30:00Z)/1s)) = 300
              → client 倒计时从 600 跳回，与并发的 GET 返回值漂移
```

更糟糕：t > 600s 时 GET 返回 0（status=2 已可开箱），但缓存重试返回 600 → client 显示"还有 10 分钟"，开箱按钮 disable，UX 严重错位。

### 根因（Root cause）

把"幂等首次成功的响应 = response 字节序列"理解成"整个响应都可以缓存"，忽略了响应里存在**时间派生字段**这一类特殊字段：它们的值在响应字节序列化时由"当前时刻"计算得出，每次序列化结果不同。这类字段写入缓存等于把"序列化时刻"凝固进了响应，回放时拿到的是"过去某个时刻"的 view，而 client 期望的是"当前时刻"的 view。

更深层的原因：契约文档同时存在两条互相冲突的钦定（在 §7.2 不同段落里）：
- A：「response_json 缓存首次成功的完整响应，回放时反序列化返回」
- B：「`nextChest.remainingSeconds` server 端按 `max(0, ceil((unlock_at - now)/1s))` 计算」

A 和 B 在"是否包含 remainingSeconds"上没有显式协调 —— 作者写 A 时默认"包含全部字段"，写 B 时默认"每次都实时计算"。两条钦定在 happy path 各自成立，但在"同 key 重试 + 时间跨度大"的交集下冲突。

### 修复（Fix）

把 `response_json` 缓存范围从"完整响应"缩窄到"非时间派生字段子集"：

```
response_json 持久化范围（钦定）：
  {
    reward.*,                           // 静态业务字段
    stepAccount.*,                      // 静态业务字段（事务内 final 值）
    nextChest.{id, status, unlockAt, openCostSteps}    // 静态业务字段
  }

response_json 不包含：
  nextChest.remainingSeconds            // 时间派生字段
```

回放路径（步骤 3b `status='success'` 分支）：
```
SELECT status, response_json
deserialize response_json
nextChest.remainingSeconds = max(0, ceil((nextChest.unlockAt - now) / 1s))   ← 实时补算
return 200 + assembled response
```

首次成功路径同步对齐：步骤 3j 序列化 `response_json` 时**主动跳过** `remainingSeconds` 字段；步骤 4（commit 后）在内存 payload 上补算 `remainingSeconds` 后再返回 200。这样首次成功路径与重试 cached 路径走**同一**补算公式，与 `GET /chest/current` 同语义对齐，无 stale 漂移。

字段定义同步加注解：
```
| data.nextChest.remainingSeconds | ... | **计算字段，不持久化到 idempotency response_json 缓存；
                                          server 在响应序列化时按 max(0, ceil((unlock_at - now)/1s)) 实时计算填入** |
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 设计"幂等响应缓存（response_json 等）"时，**必须**先扫描响应所有字段，把**时间派生字段**（remainingSeconds / countdown / ttl / age / elapsed 等）**显式排除**出缓存集合，回放时实时计算填入；**禁止**把"缓存完整响应"作为默认设计。
>
> **展开**：
> - 时间派生字段的特征：值由"当前时刻"计算得出，每次序列化结果不同。常见命名包含 `remaining` / `elapsed` / `age` / `countdown` / `ttl` / `until` 后缀。
> - 识别时间派生字段的方法：看字段定义里有没有"按 `now`/`current_timestamp`/`(X - now)` 计算"的语义。**任何**包含 `now` 的计算公式 = 时间派生字段。
> - 缓存策略：缓存的是"业务状态快照"（unlockAt 这种绝对时刻 / userId 这种 ID / status 这种枚举），**不是**"响应字节序列"。响应字节序列由 (业务状态快照 + 当前时刻) 实时拼装得出。
> - 写契约文档时，幂等缓存的字段范围和字段定义里"如何计算"的语义**必须**在同一处或彼此交叉引用，避免两段钦定在 happy path 各自成立、在 edge case 互相冲突。
> - **反例**：在 §7.2 钦定「`response_json` = 完整 V1 响应（含 reward / stepAccount / nextChest）」+ 在字段表钦定「`nextChest.remainingSeconds` 按 `max(0, ceil((unlock_at - now)/1s))` 计算」—— 两段都不错，但合起来在 t > 1s 的同 key 重试场景下产生 stale。**禁止**把"缓存完整响应"当成默认；**必须**显式列出"哪些字段不缓存"。
> - **反例 2**：用 "TTL 内一定刷新缓存" 来回避问题（如 "response_json 缓存 60s 自动过期"）—— 这只是缩短了窗口，没根除问题；t < 60s 的重试仍然 stale。根本解法是**计算字段不缓存**。

---

## Meta: 本次 review 的宏观教训

r5 / r6 两轮 review 锁定了同一类思维漏洞的两个变体：**在分布式 / 事务系统里追求"原子写"时，必须把"原子边界"画在所有相关写入操作的外侧，不能放过任何一笔"看似无关的预备写入"**。

- r4 的失败：Redis SET 写后失败 → 60s 外重复出箱（原子边界画在"事务 + Redis SET" 但 SET 不在事务内）
- r5 的失败：预声明 INSERT 写在事务外 → rollback 后 pending 卡死（原子边界画在"业务事务" 但预声明不在事务内）
- r6 的修订：把所有写入都画进同一事务 + 用 InnoDB 锁阻塞同 key 并发（原子边界完整闭合）

**对应规则**：每次设计"事务幂等"方案时，**必须**列出"本接口路径上所有对幂等记录 / 业务数据的写入操作"清单，逐条审视它们是否都在同一事务里。任何一条不在事务内的写入 = 一条潜在的状态机断边 = 一条可触发的悖论。

附加教训：**`response_json` 这类缓存字段必须列出显式 schema**（"缓存什么字段、不缓存什么字段"），不要写成"完整响应"。**完整响应里有时间派生字段，缓存它就会引入 stale**。
