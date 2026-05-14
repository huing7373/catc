---
date: 2026-05-14
source_review: codex review (epic-loop story 20-1 r1) — /tmp/epic-loop-review-20-1-r1.md
story: 20-1-接口契约最终化
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-14 — POST /chest/open 幂等原子声明 & rate_limit 中间件不可绕开

## 背景

Story 20.1 是节点 7 宝箱业务的"接口契约最终化"纯文档 story —— 在 `docs/宠物互动App_V1接口设计.md` §7.1 / §7.2 锚定 `GET /chest/current` / `POST /chest/open` 字段表、错误码、关键约束，并在 §1 末尾追加节点 7 宝箱契约冻结声明。codex r1 review 命中 §7.2 服务端逻辑里两条 P1 契约层错误：

1. 步骤 3 描述 "命中 Redis idempotency 缓存不计入新限频"，但步骤 1 写明 rate_limit 是中间件 —— 中间件在路由匹配前就已执行，无法被路由逻辑回溯豁免 → 承诺不可实施
2. 步骤 3 描述 "查 Redis；命中则返回；未命中则进事务" —— 是两步非原子操作，两个并发同 key 请求会同时 miss → 各自进事务 → 第二个事务被首请求 FOR UPDATE 串行后看到的是 status 已变更的 chest（被 DELETE / unlock 状态变化）→ 返回错误码而非首次成功结果 → 违反 "同 idempotencyKey 重试安全" 契约

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 删除 "缓存命中不计入新限频" 不可实施承诺 | high (P1) | architecture / docs | fix | `docs/宠物互动App_V1接口设计.md:953` |
| 2 | 用 SET NX sentinel 原子 claim 替代 "GET + SET" 两步分离 | high (P1) | architecture / docs | fix | `docs/宠物互动App_V1接口设计.md:953-965` |

## Lesson 1: rate_limit 中间件位置决定 "免限频" 承诺的可实施性

- **Severity**: high (P1)
- **Category**: architecture / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:953`

### 症状（Symptom）

接口契约文档同时写了：
- "rate_limit 中间件按默认配置限频"（步骤 1，中间件层）
- "Redis 幂等键命中 → 不计入新限频"（步骤 3，路由内逻辑层）

下游 Story 20.6（server）/ Story 21.x（iOS）按此实装时会发现：中间件已先于步骤 3 执行，路由层无法回溯豁免；client 网络层重试若不退避，仍会被 rate_limit 拦截并返回 1005，而契约承诺的 "重试免限频" 不存在。

### 根因（Root cause）

写契约时把 "中间件" 当作 "可以被路由逻辑覆写的装饰器" 来理解 —— 但 Gin / Echo / Chi 等主流 Go HTTP 框架的中间件链是**洋葱模型**：中间件在 handler 调用前**已经执行完准入检查**（rate_limit 判定后才轮到路由 handler），路由内任何 "豁免" 都是事后补救，无法回滚 rate_limit 已扣的配额计数。

更深层根因：把 "幂等性" 和 "限频" 两个正交关切混在一起讨论，误以为 "幂等命中" 可以从 "限频统计" 里被减去 —— 但限频是**用户级**保护（防滥用），幂等是**请求级**结果保护（防重复副作用），二者作用对象不同、生命周期不同，强行耦合会让中间件链感知到路由级状态（幂等结果），违反中间件设计的关注点分离。

### 修复（Fix）

把 "不计入新限频" 删除，替换为：

> rate_limit 在步骤 1 已执行，因此**重试请求即使命中 idempotency 缓存，本次调用仍计入 rate_limit 配额**（与节点 2 / 3 / 4 / 5 / 6 已认证业务路由一致）。client 在网络层重试时**应**自行做退避（指数退避 / 抖动）避免触发 1005；server 端**不**为本接口提供"重试免限频"特殊路径

同步在 §1 冻结条款里新增 "rate_limit 中间件不免 idempotency 命中请求" 条款，把这条契约决策显式冻结。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **撰写包含 "中间件 + 路由内逻辑" 双层结构的接口契约** 时，**禁止** **让路由内分支"豁免"中间件已经执行过的准入检查**。

> **展开**：
> - 中间件（auth / rate_limit / cors / request_id 等）的语义边界 = "请求进入 handler 前就已生效，handler 内无法回溯撤销"
> - 写契约时若想让某类请求 "免限频 / 免 auth"，必须把豁免规则**前置到中间件配置层**（如 rate_limit 中间件接收 "豁免路由白名单" 参数），而**不**靠路由内 if 分支
> - 限频和幂等是正交关切：限频保护 server 资源（用户级配额），幂等保护副作用语义（请求级结果回放）；不要让一个机制"借用"另一个机制的状态
> - **反例**：
>   - 契约写 "命中 Redis idempotency 缓存 → 不计入限频"（路由内逻辑无法影响中间件已扣的配额）
>   - 契约写 "重试请求 → 跳过 auth 中间件"（中间件已先校验，路由内"跳过"无意义）
>   - 契约写 "WS upgrade 后 → 不走 HTTP rate_limit"（实际上 WS 路由本来就不挂 HTTP rate_limit 中间件，是路由注册时就分离的，**不**是路由内"豁免"）—— 这种是描述错误而非语义错误，但容易让下游 story 误以为有 "动态豁免" 机制

## Lesson 2: Redis 幂等键必须 SET NX 原子 claim，不能 "GET 检查 + 后续 SET" 两步分离

- **Severity**: high (P1)
- **Category**: architecture / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:953-965`

### 症状（Symptom）

原契约描述：
- 步骤 3：查 Redis `idem:{userId}:chest_open:{idempotencyKey}` —— 命中则返回缓存，未命中继续步骤 4
- 步骤 4：MySQL 事务（FOR UPDATE chest + step_account + UPDATE + INSERT + DELETE + INSERT）
- 步骤 5：写 Redis 缓存

两个**并发同 idempotencyKey 请求**到达时：
- T1 步骤 3：miss → 进步骤 4
- T2 步骤 3：miss（T1 尚未到步骤 5）→ 也进步骤 4
- T1 / T2 在步骤 4a FOR UPDATE 上串行：T1 先持锁 → DELETE 旧 chest + INSERT 新 chest + COMMIT
- T2 持锁后：原 chest 已 DELETE → 步骤 4a 拿到 4001（"宝箱不存在"）→ rollback
- T2 client 收到 4001 而非首次成功 200 + reward → **违反 "同 key 重试安全"**

### 根因（Root cause）

把 Redis 幂等键当作"读多于写的缓存"来设计 —— 用 "GET 检查存在性 + 后续 SET 回填" 的 cache-aside 模式。但幂等键的语义不是缓存，是**互斥锁 + 结果存储二合一**：
- 互斥锁语义：同 key 在 "事务执行中" 期间禁止并发请求进入
- 结果存储语义：事务结束后存储 response 供后续重试回放

Cache-aside 模式只满足结果存储，不满足互斥锁。要同时满足两者，必须用 **SET NX**（或 Lua CAS）做单步原子 claim：首次声明时写入 sentinel 占位 → 事务结束时把 sentinel 替换为真实 response JSON（或事务失败时 DEL sentinel）。

更深层根因：MVP 阶段倾向于 "简化设计 → MySQL 行锁兜底就够了" —— 但 FOR UPDATE 行锁仅在事务**开启之后**才能阻塞并发，**不能**阻止两个事务**同时被开启**。事务一旦开启就要消耗 connection + 事务隔离级别开销，且在事务内才发现 "撞车" 时只能 rollback —— 既浪费资源又破坏 client 视角下的语义（client 期望同 key 重试无副作用，但 server 已经开过一个空跑事务）。

### 修复（Fix）

把步骤 3 从 "GET 检查" 改为 "SET NX 原子声明"，明确三态分支：

> - **首次声明成功**（key 之前不存在，本次写入 sentinel `"__pending__"` 占位）→ 进事务
> - **sentinel 命中**（同 key 另一请求执行中）→ 返回 1008
> - **完整 JSON 命中**（首次已成功）→ 返回 200 + 首次结果

事务结束后：
- 成功 → 步骤 5 `SET` 覆盖 sentinel 为真实 response JSON
- 失败 → 步骤 5 改为 `DEL` 清理 sentinel，让 client 可用新 key 重试

同步更新错误码表（1008 在本接口表征 "sentinel 命中、并发执行中"，**不是** "已完成"）+ 关键约束的 client 重试策略（区分网络层重试 / 业务错误重试 / 1008 重试 三类不同语义）+ §1 冻结条款（把 "Redis 幂等键命中行为" 升格为 "原子声明 + 三态分支"）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计 "client 用 idempotencyKey 保证同请求重试安全" 的接口契约** 时，**必须** **用 SET NX（或 Lua CAS）单步原子 claim 而非 GET + SET 两步分离**。

> **展开**：
> - 幂等键不是缓存，是 "互斥锁 + 结果存储" 二合一原子结构
> - 步骤序：claim sentinel（原子 SET NX）→ 进事务 → 事务成功 → SET 覆盖 sentinel 为真实结果；事务失败 → DEL sentinel
> - 三态分支必须在契约层明示：首次 claim 成功 / sentinel 命中（1008）/ 完整结果命中（200 + 首次结果）
> - **不能**靠 DB 行锁兜底并发 —— 行锁在事务开启后才生效，并发请求已各自进事务消耗连接 / 隔离级别 / rollback 资源
> - sentinel 清理责任在事务边界外：成功路径在事务后 SET 覆盖，失败路径在 rollback 后 DEL；DEL 失败仅 log warn（best-effort 退化，client 改用新 key 重试，符合 MVP Redis 可用性目标）
> - **反例**：
>   - 契约写 "查 Redis；miss 则进事务；事务后写 Redis"（两步分离，不抗并发）
>   - 契约写 "MySQL 事务 FOR UPDATE 行锁就够了，不用 Redis claim"（行锁不能阻止并发事务同时开启，只能让它们 rollback —— rollback 后第二个 client 收到错误码而非首次成功，违反契约）
>   - 契约写 "事务失败时不删 Redis sentinel"（同 key 重试会持续 1008 直到 TTL 过期，违反 "业务错误后用新 key 重试" 语义）
>   - 契约不分 "sentinel 命中" vs "完整结果命中" 两个错误码（client 不知道是该等 / 该退避 / 该用新 key）

## Meta: 本次 review 的宏观教训

**契约文档不只是字段表 + 错误码表，必须显式描述并发 / 中间件 / 副作用 / 重试的端到端语义闭环。**

两条 P1 的共性是：契约写得"看起来合理"，但下游 server / iOS story 按此实装时会遇到**并发实现层的细节冲突**（中间件先后顺序 / 原子操作 vs 两步分离）。这种冲突在字段层 / 错误码层都看不出来，需要：
- 把 "client 视角下的承诺"（同 key 重试安全 / 重试免限频）和 "server 实装层的执行序"（中间件 → 路由 handler → 事务 → 缓存）做端到端 trace
- 任何 "承诺" 都要追问 "用什么机制实现？是中间件还是路由内？是 Redis 还是 MySQL？" —— 没有具体机制兜底的承诺都是 wishful thinking

冻结契约**前**做完这层端到端 trace 是写契约的必经步骤；冻结后再修复属于事故，需触发 §1 冻结条款里的 4 步流程（评审 + 回归 + 标注）—— 本次 r1 fix 因 story 尚在 review 状态、未广播给下游 story，**不**触发 4 步流程（story 状态仍是 review，待 done 后才真正冻结），但 lesson 必须留底防未来再犯。
