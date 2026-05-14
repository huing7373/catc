---
date: 2026-05-14
source_review: codex review round 5 on Story 20-1 接口契约最终化 (chest_open idempotency contract finalization)
story: 20-1-接口契约最终化
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-14 — 为什么 Redis 不能做"资产事务幂等"的可信源 & DB 同事务幂等才是 finalize 解（20-1 r5）

## 背景

Story 20-1 round 5 review。前 4 轮 review 迭代里：

- r3 把 `POST /chest/open` 的幂等记录设计为 Redis（sentinel `"__pending__"` + final-response JSON 共享 24h TTL）
- r3 review 指出 SET / DEL 失败会让 client 卡死 24h → r4 改为"sentinel TTL = 60s 短窗口 + final-response TTL = 24h"双 TTL，60s 外允许 client 换新 key + UI 提示"奖励可能已发放"
- r5 review 进一步指出：**SET-fail-after-commit case**下，client 在 60s 外**无法区分**两种 case：
  - (a) sentinel 残留 = SET 失败 + 业务事务 commit 成功（资产已落盘）→ 换新 key 重试**重复扣 1000 步 + 重复出箱**
  - (b) sentinel 残留 = DEL 失败 + 业务事务 rollback（资产未落盘）→ 换新 key 重试安全

r5 的根因诊断是契约层面而非实装：**Redis 是非事务存储，与 MySQL 不能形成原子写**，任何"在业务事务前 / 后用 Redis 做幂等记录写"的设计都存在"事务 commit 但 Redis 写失败"窗口，client 永远无法从单一可信源得到"首次到底成不成功"。r4 想用"短 TTL + 时间窗换 key"绕过，但反而引入 case (a) 的重复出箱风险。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Redis 写回失败的两种残留 case 无法被 client 区分 → 60s 外换新 key 在 SET-fail-after-commit case 下重复出箱 | high | architecture | fix | `docs/宠物互动App_V1接口设计.md` §7.2 / §13 / `docs/宠物互动App_数据库设计.md` §5.16 / §8.3 / §9.1 |

## Lesson 1: Redis 不能做"资产事务幂等"的可信源；DB 同事务幂等才是 finalize 解

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §7.2 服务端逻辑 / 关键约束 / §13 幂等与防重 + `docs/宠物互动App_数据库设计.md` 新增 §5.16 + §8.3 / §9.1 同步

### 症状（Symptom）

r4 契约钦定 Redis 双 TTL 幂等设计后，r5 review 指出在以下时序下 client 仍可能拿到重复奖励：

```
t0: client A 发 POST /chest/open key=K  → server SET NX sentinel(K, "__pending__", TTL=60s) OK
t1: server 开 MySQL 事务，扣 1000 步、写 chest_open_logs、刷新 chest、commit OK
t2: server 异步 SET (K, final_response_json, TTL=24h) → 3 次重试均失败（Redis 间歇异常）
t3: 60s 后 sentinel 自然过期，Redis 上无任何 K 相关数据
t4: client A 网络抖动重试同 K → server SET NX 成功（key 不存在）→ 进新事务 → 再扣 1000 步 + 再出一个箱
```

client 在 t3 收到的是 200 + 首次成功 response（因为首次 commit 已 OK；只是后续 SET 失败），但若 client 在 t4 因任何原因（如本地 retry 队列未及时拿到 200 OK、超时重试）再发同 K 请求，server 无可信源识别这是"首次成功后的网络重试"，会重新进事务造成 double-spend。

r4 给的兜底是"client 在 60s 外用新 key + UI 提示"，但 client 无法区分自己处于 case (a)（SET 失败 + commit 成功）还是 case (b)（DEL 失败 + rollback），UI 文案只能模糊提示"奖励可能已发放，请联系客服"——这是**契约层把不确定性推给用户和客服**，不是真正解决问题。

### 根因（Root cause）

**Redis 是非事务存储**：MySQL 业务事务 commit 与 Redis SET 是两个独立的 IO 操作，二者之间不存在"either both succeed or both rollback"的原子保证。任何"用 Redis 记录幂等状态 + 用 MySQL 记录业务数据"的设计都会在"commit 成功 + Redis 写失败"窗口内**让幂等记录与业务数据状态不一致**。

r3 / r4 的 lesson 漏洞：把 Redis 当成"低延迟的状态机存储"使用，没有意识到"幂等记录本身是事务状态的一部分，必须与业务数据同生共死"。r4 试图用"短 TTL + 时间窗"在概率上压低 case 发生频率，但**只要 case 存在，client 就需要在两种残留语义之间做猜测**——而 client 没有可观测信号能做这个猜测，最终只能引入"UI 提示模糊文案"这种产品级降级。

更深层的思维漏洞：契约层（V1 接口设计文档）应当对"server 端首次到底成不成功"提供**单一可信源**，让 client 用"同 key 重试始终安全"这条简单规则完成所有重试路径。任何把"判断首次状态"职责推给 client（"在 60s 内 X，60s 外 Y"）或推给运营（"SLA + 客服流程兜底"）的契约都是**契约的失败**。

### 修复（Fix）

把开箱幂等记录从 Redis **整体上移**到 MySQL，与业务事务**同事务原子写**：

1. **新增表 `chest_open_idempotency_records`**（数据库设计 §5.16）：
   ```sql
   CREATE TABLE chest_open_idempotency_records (
       id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
       user_id BIGINT UNSIGNED NOT NULL,
       idempotency_key VARCHAR(128) NOT NULL,
       status ENUM('pending', 'success', 'failed') NOT NULL DEFAULT 'pending',
       response_json JSON NULL,
       created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
       updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
       UNIQUE KEY uk_user_id_key (user_id, idempotency_key),
       KEY idx_status_created_at (status, created_at)
   ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
   ```

2. **改写 V1 接口设计 §7.2 服务端逻辑**：
   - **步骤 3（业务事务之前，独立 INSERT）**：`INSERT ... ON DUPLICATE KEY UPDATE id = id` 借 UNIQUE 约束做 single-statement 原子声明。
     - `affected_rows = 1` → 进业务事务
     - `affected_rows = 0` → SELECT status：`pending` → 1008 / `success` → 200 + cached response / `failed` → 1009
   - **步骤 4i（业务事务内，commit 之前）**：`UPDATE chest_open_idempotency_records SET status='success', response_json=? WHERE user_id=? AND idempotency_key=?` —— 与业务表 UPDATE / INSERT 同事务原子提交
   - **步骤 5（事务后）**：无需 Redis 写回；事务 commit 成功 → idempotency 行已落盘；事务 rollback → idempotency 行也回滚（同 key 重试自然走全流程）；可选 best-effort 写一条 `status='failed'` 让 client 明确换新 key

3. **删除 r4 钦定的"Redis sentinel + 双 TTL + 异步 SET 重试 + 60s 外换新 key + UI 提示"全套设计**（V1 §7.2 关键约束 / 数据库设计 §9.1 全面移除）。

4. **更新 §1 freeze declaration**：把"DB 幂等原子声明 + 同事务持久化"作为新的冻结契约项；明确"幂等存储介质冻结在抽象层 —— '幂等记录 + 业务数据原子写'这一不变量；把幂等记录退回 Redis 视为契约变更"。

5. **同步更新 §13 幂等与防重 + 数据库设计 §8.3 开箱事务 + §9.1 Redis 职责边界**，全面移除 Redis 在 chest_open 路径上的角色。

6. **client 重试策略大幅简化**：
   - 1008（pending）→ 同 key 退避重试，**无 60s 边界**（r4 的 60s 来源于 Redis sentinel TTL，DB 持久化幂等无此约束）
   - 1009（failed 或 5xx）→ 换新 key 重试一次，**UI 无需"奖励可能已发放"等不确定文案**

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计任何"修改 MySQL 业务数据"的接口的幂等机制** 时，**必须把幂等记录存储在 MySQL 同事务内**（如 `*_idempotency_records` 表），**禁止**用 Redis / Memcached / 外部 KV 存储做"资产事务幂等"的可信源。
>
> **展开**：
> - **判定标准**："幂等需要保证'同 key 至多产生一次业务副作用'"+ "业务副作用落在 MySQL"→ 幂等记录**必须**在 MySQL 同事务。
> - **设计模式**：
>   1. 幂等表带 `UNIQUE(user_id, idempotency_key)` 约束，用 `INSERT ... ON DUPLICATE KEY UPDATE id = id` 做 single-statement 原子声明（`affected_rows` 1 vs 0 分支）
>   2. `status` 用三态 ENUM：`pending`（事务执行中）/ `success`（commit 后含 response_json）/ `failed`（rollback 后 client 应换新 key）
>   3. `response_json JSON` 字段缓存首次成功响应，同 key 重试时直接反序列化返回
>   4. 在业务事务内 commit 前**先 UPDATE 幂等行的 status / response_json**，再 commit → 业务数据 + 幂等状态原子落盘
> - **不是用 Redis 的理由（写给未来 Claude 的硬规则）**：
>   - Redis SET / DEL 与 MySQL commit 是两个独立 IO，不存在原子性保证
>   - SET-fail-after-commit window 内 client 无法区分"首次已生效"vs"首次未生效"
>   - 任何依赖"短 TTL + 时间窗 + UI 提示"的兜底都是**把不确定性推给用户/客服**，不是契约层解
>   - Redis 在幂等路径上的合法角色**仅**限于：(a) 不修改业务数据的轻量场景（GET 防重 / 客户端去抖）；(b) rate_limit / 在线态 / WS session 等显式非事务语义场景
> - **反例（绝对禁止）**：
>   - "用 Redis SET NX 做幂等 claim + 事务后 SET 写 final response JSON + 异步重试 SET / DEL 兜底"（r3 / r4 设计；r5 锁定为反面案例）
>   - "用短 TTL sentinel + 60s 外让 client 换新 key + UI 提示模糊文案"（r4 设计；引入 SET-fail-after-commit case 下的重复出箱风险）
>   - "用 Redis 单 TTL 24h 共享 sentinel 和 final response"（r3 设计；SET / DEL 失败让 client 卡死 24h）
> - **正例**：
>   - r5 锁定方案：`chest_open_idempotency_records` 表 + `UNIQUE(user_id, idempotency_key)` + 业务事务内 step 4i UPDATE → commit 原子；client 同 key 重试始终安全，无 UI 模糊文案
>   - 适用范围：开箱 / 合成 / 穿戴 / 任何"修改 MySQL 资产数据"的 POST 接口；合成事务（节点 11 / Epic 32）应起新表 `compose_upgrade_idempotency_records` 或共用通用表 `idempotency_records`（由 Story 32.4 锚定）

## Meta: 本次 review 的宏观教训

**契约层必须给"server 端首次到底成不成功"提供单一可信源**。任何把这个判断推给 client（"60s 内 X，60s 外 Y"）或推给运营（"SLA + 客服"）的设计都是契约的失败，因为 client 没有可观测信号做这个判断，最终只能引入产品层降级（UI 提示模糊文案）。

review iteration 的反面教训：r3 → r4 的迭代是在"既有 Redis 方案"框架内做局部优化（缩短 TTL），但**没有跳出"Redis 是幂等记录存储"这个根本假设**。r5 review 跳出来后发现 finalize 解需要换存储介质（MySQL 同事务），而不是调 TTL 数值。

**给未来 Claude 的元规则**：当一个设计迭代了 2+ 轮 review 仍在引入新 case 风险时，停下来**质疑根本假设**（"为什么用 Redis？业务事务的可信源应当在哪里？"），而不是继续在既有方案上调参数 / 加兜底。
