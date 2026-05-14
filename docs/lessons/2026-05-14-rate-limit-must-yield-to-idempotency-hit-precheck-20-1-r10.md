---
date: 2026-05-14
source_review: /tmp/epic-loop-review-20-1-r10.md（codex review round 10）
story: 20-1-接口契约最终化
commit: 50f3421
lesson_count: 1
---

# Review Lessons — 2026-05-14 — rate_limit middleware 必须让位于 idempotency 命中预检（20-1 r10）

## 背景

Story 20.1 round 10 codex review 针对 `POST /api/v1/chest/open` 契约的 rate_limit / idempotency 交互顺序问题：r9 完成的契约把 rate_limit middleware 放在 idempotency 命中检查之前，于是 client 用同 idempotencyKey 在网络抖动时退避重试，每次重试都被 middleware 计入 user quota，最终撞 60/min 限频拿 1005，**永远无法读取 server 端已 commit 的 cached success response** —— 破坏 §7.2 「同 key 重试始终安全」契约承诺。本次 review 锁定的唯一 P1 直击该破窗路径。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | rate_limit middleware 在 idempotency 命中检查之前消耗 quota → 同 key 重试 60 次后撞 1005 读不到 cached success | high | architecture | fix | `docs/宠物互动App_V1接口设计.md` §7.2 + 数据库设计.md §5.16 |

## Lesson 1: rate_limit 检查必须位于 idempotency 命中预检之后，cached replay 不消耗配额

- **Severity**: high
- **Category**: architecture（rate-limit 与 idempotency 的顺序契约）
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §7.2 服务端逻辑步骤 1-8 整体重排 + 关键约束「rate_limit 位置 r10 调整」段新增 + §1 节点 7 冻结声明「rate_limit 位置语义」修订 + 错误码 1005 触发条件修订 + client 重试策略段网络层重试条目修订；同步更新 `docs/宠物互动App_数据库设计.md` §5.16 表头注释

### 症状（Symptom）

用户首次 `POST /chest/open` 成功（server 端 `chest_open_idempotency_records.status = 'success'` 已 commit）+ client 因网络 timeout 未收到 200 → client 按 §7.2 「client 重试策略」用**同**一 idempotencyKey 退避重试。在 r9 设计下，rate_limit middleware 在每次重试时都按 user_id 计入 quota → 60 次重试后 client 拿到 **1005 操作过于频繁**，永远无法触发 handler 内的 idempotency lookup，读不到首次缓存的 success response。client UX 层只能向用户展示"开箱失败 / 服务繁忙"，无法确认资产是否已发放；如客户端没有 server 侧资产查询接口（节点 7 阶段确实没有），用户会被迫联系客服，跨端数据状态严重不一致。

### 根因（Root cause）

把 rate_limit 与 idempotency 视为**两个正交关注点**，于是直觉地把 rate_limit 放在通用 middleware 层（auth 之后、handler 之前，与 Story 4.5 钦定的统一 rate_limit middleware 基线一致），让 idempotency lookup 落在 handler 内部。这种结构对**普通无幂等接口**没问题，但对**支持 idempotencyKey + cached replay 的资产事务接口**有以下契约级冲突：

1. **配额语义错位**：rate_limit 的本意是节流"业务消费"（每次 chest_open 占 chest / step_account 行锁、走加权抽奖、扣步数、INSERT chest_open_logs / user_chests 等），cached replay 路径本质是"只读首次结果"，与 GET 查询等价，不该消耗配额
2. **retry-safety 契约被破坏**：「同 key 重试始终安全」的承诺要求"无论 client 怎么退避重试，只要首次已 commit success，重试必能读到 cached"，但 middleware 层 rate_limit 让该承诺在 retry 次数 > 60 时失效
3. **没有可观测信号让 client 区分"被限频拒绝"vs"server 真的失败"**：1005 是"服务端拒绝处理"，client UX 无法判断"我的开箱实际成功了吗"，破坏 §13.2 幂等规则承诺的「第一次成功，后续重复请求返回第一次结果」契约

### 修复（Fix）

把 rate_limit 检查从 middleware 层挪到 handler 内层，**置于 idempotency 命中预检之后**。具体改动：

**1. §7.2 服务端逻辑步骤重排（原 6 步 → 新 8 步）**

before（r9）:
```
1. 认证 & 限频（rate_limit middleware）
2. 参数校验
3. MySQL 事务（3a 预声明 + 3b 短路 + 3c~3l 业务/最终化/提交）
4. 首次成功响应组装
5. 事务后处理
6. 响应
```

after（r10）:
```
1. 认证（仅 auth middleware）
2. 参数校验
3. 幂等命中预检（autocommit SELECT chest_open_idempotency_records）
   - success → 跳过 4 + 5，返回 cached response
   - pending → 跳过 4 + 5，返回 1008
   - 未命中 → 继续步骤 4
4. 限频检查（handler 内层，仅对真新请求做；超限 → 1005）
5. MySQL 事务（5a 预声明 + 5b 短路 + 5c~5l 业务/最终化/提交）
6. 首次成功响应组装
7. 事务后处理
8. 响应
```

子步骤字母也同步从 3a-3l → 5a-5l（保持事务子步骤紧贴顶层"MySQL 事务"步骤）。

**2. 关键约束「rate_limit 位置 r10 调整」段新增**

详细说明：(a) 路由层显式 opt-out 全局 rate_limit middleware；(b) handler 入口先做幂等命中预检；(c) 限频检查仅对未命中的真新请求做；(d) 拒绝方案 B（middleware 接受 cache-hit hint）的理由（race 风险 + 跨 epic 改 middleware 接口成本高）；(e) 跨接口影响（节点 11 `POST /compose/upgrade` 如果复用同一持久化幂等模式，应采用相同结构）

**3. §1 节点 7 冻结声明同步**

把"`rate_limit 中间件不免 idempotency 命中请求`（重试计入用户级配额，client 须自行退避）"改为"`rate_limit 位置语义（r10 review 锁定）`：rate_limit 检查在 handler 内层、幂等命中预检之后做 —— cached replay 路径不计入 rate_limit 配额"

**4. 错误码 1005 触发条件修订**

明确"仅对未命中 idempotency 的'真新请求'做（按 user_id 限频，每用户每分钟 > 60 次）；同 key 重试命中 success / pending 时跳过步骤 4 限频，不触发 1005"

**5. client 重试策略段网络层重试条目修订**

把"避免触发 1005，因 rate_limit 中间件不对 idempotency 命中请求免限频，重试计入用户级配额"改为"同 key 重试在步骤 3 幂等命中预检命中 success / pending 时不计入 rate_limit 配额，因此同 key 重试本身不会触发 1005"，并明确各路径（pending / success / 行不存在）对应的 rate_limit 是否计入

**6. §13.3 持久化存储段同步 r10 决策 + 数据库设计.md §5.16 表头注释同步**

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计支持 idempotencyKey + cached replay 的资产事务接口（如 POST /chest/open / POST /compose/upgrade）** 时，**必须** **把 rate_limit 检查放在 idempotency 命中预检之后，让 cached replay 路径绕过 rate_limit 配额**。
>
> **展开**：
> - rate_limit middleware **不是普世默认**：它只适合"无幂等承诺的业务消费接口"（如 POST /steps/sync / POST /pets/current/state-sync）；对支持 cached replay 的接口，rate_limit 必须降级为 handler 内层检查
> - idempotency 命中预检 + rate_limit 的相对顺序是**契约级**事项，不是实装细节：必须在接口设计文档（§7.2 服务端逻辑 / 关键约束 / §1 冻结声明 / 错误码 1005 触发条件 / client 重试策略）**多处同步钦定**，避免实装时只改一处导致语义漂移
> - **路径分类决定配额计算**：(a) cached success replay → 不计配额（与 GET 查询等价）；(b) cached pending → 不计配额（不进新业务事务）；(c) 真新请求（未命中 / rollback 后到达）→ 计配额（消耗业务资源 + 防止换新 key 绕限频）；(d) rollback 后同 key 重试 → 计配额（首次失败的 retry storm 在限频上有节流）
> - **router 实装层落点**：handler 在路由挂载时显式 opt-out 全局 rate_limit middleware；handler 入口先做 idempotency lookup，命中即短路返回；未命中再做 user-id 维度的限频检查
> - **跨接口复用启示**：若节点 11 `POST /compose/upgrade` 等其他资产事务接口复用同一 idempotency 持久化模式（DB 同事务 + 二态机），rate_limit 顺序也应同样处理；**禁止**沿用 r5~r9 的"middleware 层 rate_limit + 重试消耗配额"老路径
> - **反例 1**：把 rate_limit 留在 middleware 层 + 让 client "自行做指数退避避免 1005" —— 这种"把责任甩给 client"的设计破坏「同 key 重试始终安全」契约承诺，client 退避策略没法在已成功 + 60 次失败后救自己
> - **反例 2**：让 middleware 接受 handler chain hint 做 "cache-hit 退还 quota" —— 有 race 风险（计数器在 middleware-pre 与 handler-post 之间有窗口）+ 跨 epic 改 middleware 接口成本高，**不推荐**
> - **反例 3**：把 rate_limit 完全移除掉 —— 真新请求（未命中 idempotency）必须仍按 user_id 计入配额，否则攻击者可通过"频繁换新 idempotencyKey"绕过限频
> - **反例 4**：rate_limit 检查放在 idempotency 命中预检**之前** —— 这就是 r9 的破窗路径，cached replay 路径还没机会跑就被 middleware 拦掉

## Meta: 本次 review 的宏观教训

「Middleware 层契约」与「Handler 内层契约」的边界 **必须在接口设计文档显式钦定**，不能视为"实装层细节"。原因：middleware 链是 cross-cutting 的，其位置直接决定"哪些路径绕过 / 经过该检查"，这是契约语义问题，不是路由配置问题。本 case 的 rate_limit 之所以从 r5 到 r9 一直没暴露 break path，是因为各 review round 都默认沿用 Story 4.5 钦定的"统一 rate_limit middleware"基线，没有 review 者去推演"同 key 重试 60 次后会怎样"这个极端场景。Story 4.5 的统一 rate_limit middleware 设计对**普通无幂等接口**（state-sync / steps-sync / pets/current 等）是合理的，但对**支持 cached replay 的资产事务接口**（chest/open / compose/upgrade）是契约级冲突 —— 默认基线**不**是普世真理，每个新接口在引入时都要重新评估"middleware 链中的 cross-cutting 检查 vs 本接口的语义承诺"是否有冲突。
