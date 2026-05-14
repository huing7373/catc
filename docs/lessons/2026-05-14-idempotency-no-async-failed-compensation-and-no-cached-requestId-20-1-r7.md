---
date: 2026-05-14
source_review: "codex review round 7 on Story 20-1 接口契约最终化 (HEAD=817eba6)"
story: 20-1-接口契约最终化
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-14 — DB 同事务幂等：禁止 post-rollback failed 异步补偿 + response_json 缓存禁止包含 requestId

## 背景

Story 20-1 round 7 codex review 针对 round 6 已经把幂等"预声明 INSERT"纳入业务事务、消除 pending 卡死悖论之后**仍保留**的两个细节做最后审查：

1. r6 在「事务后处理」步骤 5 留了"server 端**可**在事务 rollback 后新开独立短事务写 `INSERT ... ON DUPLICATE KEY UPDATE status='failed'` 把同 key 锁定为 failed"作为**可选** UX 优化
2. r6 把"完整 V1 响应"写入 `chest_open_idempotency_records.response_json` 缓存时**包含**了顶层 `requestId` 字段

codex 锁定两条都是 P 级问题：(1) 是 P1 race condition 可破坏数据一致性；(2) 是 P3 但破坏 trace/log 关联语义。本 lesson 把这两条根因 + 修复 + 预防规则沉淀下来。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | rollback 后 best-effort failed upsert race 把后到达的 success 覆盖为 failed | high | architecture | fix | `docs/宠物互动App_V1接口设计.md` §7.2 + 数据库设计 §5.16 |
| 2 | response_json 缓存若含 requestId，重试请求会回放首次 trace ID 破坏 log 关联 | low | docs | fix | `docs/宠物互动App_V1接口设计.md` §7.2 字段表 + §13 + 数据库设计 §5.16 |

## Lesson 1: rollback 后 best-effort failed upsert 与同 key 重试 success 的 UNIQUE race

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §7.2 服务端逻辑步骤 5 / §7.2 关键约束「事务边界 + DB 持久化幂等的写后保证」/ `docs/宠物互动App_数据库设计.md` §5.16

### 症状（Symptom）

- 时序：(t1) 首次 `POST /chest/open` 业务事务失败 → ROLLBACK（pending 行随之消失）→ server **异步**触发 best-effort compensation 写 `INSERT ... ON DUPLICATE KEY UPDATE status='failed'`
- (t2) **比 compensation 早一步**，client 立即用**同**一 idempotencyKey 重试 → 新事务在步骤 3a INSERT pending（rollback 已让旧 pending 消失，`affected_rows = 1`）→ 业务全部成功 → 步骤 3k UPDATE `status='success'` → COMMIT
- (t3) server A 的 best-effort compensation 写入到达 → 因 UNIQUE 冲突触发 `ON DUPLICATE KEY UPDATE status='failed'` → 把第二次请求**已 commit 的 `success` 行**覆盖为 `failed`
- 后果：
  - 后续同 key 重试看到 `status='failed'` → 返回 1009，但**业务事务实际已 commit success**（步数已扣 / chest 已发 / nextChest 已 INSERT）
  - `chest_open_logs` 已落盘 + `user_step_accounts` 已扣减但 `chest_open_idempotency_records` 标记 failed → 数据审计错位

### 根因（Root cause）

把"事务 rollback 后的 UX 优化补偿"和"事务原子性已保证的安全性"概念混淆了。r6 设计时认为 best-effort failed upsert 是**纯粹的 UX 提示**（让 client 立即知道换 key），逻辑上"无安全性影响"。但忽略了：

1. **rollback 路径的 pending 行已经自动消失** —— 同 key 立即重试就是合法且安全的（事务原子性保证）
2. **client 立即同 key 重试与 server best-effort compensation 是两个独立时序事件**，server 没有任何机制保证 compensation 必然先于 client 的重试 commit；事实上 client 重试通常更快（client 收到错误响应立即重试 vs server 起新短事务写 compensation）
3. **`ON DUPLICATE KEY UPDATE status='failed'`** 在 UNIQUE 冲突时**强制覆盖**当前行 status，没有"仅当 status='pending' 时才覆盖"的条件保护
4. **所谓 UX 优化**本身就**没必要** —— failed 状态唯一意义是"让 client 换 key"，但事务原子性已让同 key 重试安全，UX 上 client 看到 1009 后**可以**选择同 key 或新 key 重试，二者都安全

更深层的设计教训：**只要异步补偿写与正常路径写共享 UNIQUE 约束，且补偿是"无条件 overwrite status"语义，就必然存在 race 把后到达的 final 状态覆盖**。这种 race 不靠"补偿失败时 log.Warn"能兜住 —— 因为 compensation 本身**没有失败**（UNIQUE 冲突触发 `ON DUPLICATE KEY UPDATE` 是 INSERT 的合法语义，DB 返回 success），错的不是 SQL，而是设计上不该写。

### 修复（Fix）

**方案 A（采纳）—— 彻底移除 best-effort failed upsert + schema 简化为二态机**：

- `docs/宠物互动App_数据库设计.md` §5.16：`status` ENUM 从 `('pending', 'success', 'failed')` → `('pending', 'success')`；字段说明改写明确"无 failed 状态"+ 列出 r7 决策理由
- `docs/宠物互动App_V1接口设计.md` §7.2 服务端逻辑步骤 5：「事务后处理」整体重写为"无 post-tx Redis 写回 / 无 best-effort post-rollback failed upsert"；rollback 路径说明"无需任何 best-effort 异步补偿"，事务原子性已保证安全
- §7.2 步骤 3b 短路分支：移除 `status = 'failed'` 分支，仅保留 `success` / `pending` 两个分支
- §7.2 错误码表 1009 行：移除 "idempotency 行 `status = 'failed'`" 触发条件
- §7.2 client 重试策略 1009 行：从"必须换新 key"改为"同 key / 新 key 均安全"
- §7.2 关键约束新增「r7 移除 best-effort failed upsert 决策」段记录 race condition + 决策理由
- `docs/宠物互动App_V1接口设计.md` §13.2 / §13.3：同步更新二态机 + 实装关键字段 + 历史决策段
- `docs/宠物互动App_数据库设计.md` §8.3 注释段：同步标注 r7 简化
- `docs/宠物互动App_数据库设计.md` §5.16 阶段适用段：提示 Story 32.4 复用时也应采用二态机，禁止异步 failed 补偿

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**设计 DB 持久化幂等表** 时，**禁止**引入"事务回滚后异步 best-effort 写 `failed` 占位行"补偿；幂等状态机**应**用二态 `(pending, success)`，rollback 路径靠事务原子性自动消除 pending 行 + 同 key 重试在 UNIQUE 上拿 `affected_rows = 1` 走全流程。
>
> **展开**：
> - **判定触发条件**：当你设计的幂等表用 `(user_id, idempotency_key)` UNIQUE 约束做并发阻塞 + 业务事务内 INSERT pending → UPDATE success 模式时，**禁止**在事务外再叠加"rollback 后异步写 failed"补偿
> - **判定原则**：异步补偿 + UNIQUE 冲突 + `ON DUPLICATE KEY UPDATE status` 强覆盖语义 = 必然 race；client 重试通常比 server 异步 compensation 快，compensation 几乎**总是**覆盖到后到达的合法 success 行
> - **替代方案**：rollback 路径就让 pending 行随事务原子性自动消失；同 key 重试在步骤 3a INSERT 拿 `affected_rows = 1`，与首次到达等价走全流程；client 1009 重试用同 key / 新 key 都安全
> - **schema 设计纪律**：幂等状态机优先用**最少状态**（二态 pending/success 优于三态 pending/success/failed）—— `failed` 状态唯一用途是"强制 client 换 key"，但事务原子性已让同 key 重试安全，**没必要**引入此约束
> - **如果非要保留 failed 状态**（如未来业务真的需要"硬失败 + 阻止 client 同 key 重试"），**必须**：(a) 用条件 UPDATE（`UPDATE ... SET status='failed' WHERE status='pending'`）而非 `ON DUPLICATE KEY UPDATE` 无条件覆盖；(b) 在业务事务**内**写 failed 行而非事务外异步补偿
> - **反例**：r6 设计的 `INSERT ... ON DUPLICATE KEY UPDATE status='failed'` 异步 compensation —— UNIQUE 冲突时无条件覆盖任何 status，包括已 commit 的 success
> - **反例**：把"UX 优化"和"安全性"混为一谈 —— "让 client 立即知道换 key"听起来无害，但只要异步 compensation 路径存在，就有可能干扰正常 commit success 路径，破坏单一可信源
> - **反例**：用"compensation 失败 → log.Warn 即可"的兜底来证明设计安全 —— 错的不是 compensation 本身失败，而是 compensation 本身成功但语义错（成功覆盖了**不该覆盖**的行）；log.Warn 看不到这种错误

## Lesson 2: response_json 缓存禁止包含 requestId（每次请求独立的 trace ID）

- **Severity**: low
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §7.2 响应体字段表 + 服务端逻辑步骤 3b / 3j / 5 / §13 / `docs/宠物互动App_数据库设计.md` §5.16 字段说明

### 症状（Symptom）

r6 把"完整 V1 响应 JSON"写入 `chest_open_idempotency_records.response_json` 缓存，包括顶层信封字段 `requestId`。同 key 重试时 server 反序列化 `response_json` 直接返回 → response 里的 `requestId` 是**首次**请求生成的 trace ID。但 client 这次 retry 是一个新的 HTTP 请求，它的 log / observability pipeline 期待响应里的 `requestId` 等于本次 request 入口生成的 trace ID。结果：

- client 端日志 / APM 工具看不到"这次 retry"的端到端 trace；只看到"另一个旧 trace 又出现一次"
- server 端 access log 记录本次 retry request 的 requestId 是 X，但响应给 client 的是 Y（首次请求的）→ log/trace correlation 整个错位

### 根因（Root cause）

把"业务响应数据"和"上层信封字段"混为一谈。`response_json` 缓存的**唯一目的**是回放幂等业务结果（reward / stepAccount / nextChest），但 r6 直接缓存了完整 V1 响应（含 code / message / data / requestId 全包）。

`requestId` 在 V1 响应信封中的语义是**本次 HTTP 请求**的 trace ID（server 端从 request 入口生成 / 透传），它是**每次请求独立的动态字段**，与 `nextChest.remainingSeconds` 同属"上层动态字段"类别 —— 不应被持久化到任何缓存层。r6 已经识别了 `remainingSeconds` 不能缓存（避免 stale 倒计时），但漏了 `requestId`。

### 修复（Fix）

- `docs/宠物互动App_V1接口设计.md` §7.2 响应体字段表新增"顶层信封字段补充说明"小节，列 `requestId` 行，明确"计算字段，不持久化到 idempotency `response_json` 缓存；server 在响应序列化时**重新填**本次重试请求的 `requestId`（与 `nextChest.remainingSeconds` 同样作为'上层动态字段'处理）"
- §7.2 服务端逻辑步骤 3b（短路分支 success 分支）/ 步骤 3j（序列化可缓存 payload）/ 步骤 5（事务后处理 + 响应组装）/ 步骤 6（响应）全部更新为"反序列化 + 补算 remainingSeconds + 填入**当前**请求的 requestId"
- §7.2 关键约束「DB 持久化幂等的写后保证」+ 「server 端不变量」段：列出 `response_json` 缓存内容钦定 —— **不**包含 `nextChest.remainingSeconds` / **不**包含顶层 `requestId`
- §13.2 / §13.3：同步标注 `response_json` 缓存不含 `requestId`
- `docs/宠物互动App_数据库设计.md` §5.16 `response_json` 字段说明：明确缓存内容 + 标注"不包含顶层 `requestId`"+ r7 review 锁定理由

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**设计"业务响应缓存"** 时（无论存 DB / Redis / 内存），**禁止**把"每次请求独立的 trace ID 字段"（如 `requestId` / `correlationId` / `traceId`）写入缓存；这类字段是**上层信封动态字段**，与时间派生字段同类，应在响应序列化时**实时填入本次请求的值**。
>
> **展开**：
> - **判定触发条件**：你正在设计一个"幂等首次响应回放"或"业务结果缓存"机制，准备序列化整个 V1 响应（含 `{code, message, data, requestId}`）写入 DB JSON / Redis HSET 等
> - **划分两类字段**：
>   - **业务结果字段**（应缓存）：data.* 下的业务返回值（如 `reward`, `stepAccount`, `nextChest.{id, status, unlockAt, openCostSteps}`）—— 这些在"首次成功"事件上是确定值，重试时回放才有意义
>   - **上层动态字段**（**不**应缓存）：每次 HTTP 请求独立生成 / 取决于"当前时刻"的字段：`requestId` / `traceId` / `correlationId` / `nextChest.remainingSeconds` / 任何 `*At` 减 `now` 派生秒数
> - **实装规则**：`response_json` 缓存仅持久化业务结果字段；server 在响应序列化时**重新填**上层动态字段（用本次 request 的 trace ID + 用 now 计算时间派生字段）
> - **跨字段类比记忆**：`requestId` 与 `nextChest.remainingSeconds` 是同一类问题的两个实例 —— 第一次发现时（r6 处理了 `remainingSeconds`），就应该把同类字段全部审一遍；r7 review 揭示这种"举一不反三"是 LLM 设计文档时的典型疏漏
> - **反例**：r6 设计中把完整 V1 响应（含 `requestId`）直接 `json.Marshal` 写入 `response_json` —— 因为已经处理了 `remainingSeconds` 排除，但没把"每次请求独立的字段"作为一类去审，漏了 `requestId`
> - **反例**：在测试中 mock 出一个固定 `requestId = "req_xxx"`，导致重试场景测试看不出问题（因为示例 JSON 和 cached JSON 长得一样）—— 测试需要**模拟"两次不同请求"** 才能暴露 stale trace 问题
> - **跨语义对齐**：缓存范围 / 上层信封语义应在**字段层**而非"整个响应 JSON 层"决定；写文档时字段表的每一行都要标注"是否进入持久化缓存"

---

## Meta: 本次 review 的宏观教训

r5 → r6 → r7 三轮 review 揭示了**契约层级幂等设计的渐进收敛过程**，其中每一轮都消除了**上一轮以"可选优化"或"完整保留"形式留下的隐患**：

- r5 → r6：r5 把"幂等记录最终化 UPDATE"放入业务事务（这是核心修复），但保留了"预声明 INSERT 在业务事务**之外**独立执行"作为"优化"（避免事务太长持锁）—— r6 锁定该"优化"是 pending 卡死悖论根源，必须把 INSERT 也纳入业务事务
- r6 → r7：r6 把"预声明 INSERT"纳入业务事务（核心修复），但保留了"post-rollback best-effort 写 failed"作为"UX 优化"—— r7 锁定该"优化"是 race condition 根源，必须移除；同时也补上了 `requestId` 不该缓存的细节

**LLM 设计文档的疏漏模式**：在 r4 → r5 → r6 → r7 序列里，每次 review 都揭示出"自以为是 UX / 性能优化的子功能"实际上是**新的隐患引入路径**。同步的设计模式是 **YAGNI 应用到契约文档**：每个"可选 best-effort 补偿写""可选异步重试""可选 cache 层"在写入文档时都需要被**明确质疑**："如果省掉这个补偿，正常路径是否安全？是否有 client 卡死风险？" 如果答案是 No（即基线机制已足够安全），就**不该**把它写入契约 —— 因为契约文档里的每一条"可选"路径都会被实装者忠实落地，并带来新的 race 维度。

**核心 takeaway**：DB 持久化幂等的安全性**完全来自事务原子性 + UNIQUE 约束**，不应被任何异步 best-effort 路径"加强"—— 异步路径与 UNIQUE 约束的相互作用几乎必然引入 race。
