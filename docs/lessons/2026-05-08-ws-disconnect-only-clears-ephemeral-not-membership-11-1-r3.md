---
date: 2026-05-08
source_review: codex review (epic-loop r3) — /tmp/epic-loop-review-11-1-r3.md
story: 11-1-接口契约最终化
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-08 — WS 断开仅清 ephemeral，房间归属只能由 HTTP leave 改变 / 跨文档 disconnect 语义必须合并为单条规则（11-1 r3）

## 背景

Story 11.1 r1 / r2 修复后，r3 codex review 抓到两条相互纠缠的契约自洽 bug：

- **r1 引入**："心跳超时清理钩子触发 → 删除 `room_members` 行 + 更新 `users.current_room_id = NULL` + 触发 `member.left` 广播"作为"被动 leave"完整路径，目的是让 roster 不区分在线 / 离线但仍清理僵尸成员
- **r2 跨文档对齐**：把 V1接口设计.md §10.5 / §12.3 的"被动 leave"语义同步到时序图设计.md §13.3，并扩展为"client close / TCP 1006 也走被动 leave 路径"
- **r3 review 指出两条结构性自洽问题**：
  - **[P1]** 心跳超时删 row + close 4005 的语义与 §12.1 close code 表 4005 行 "client 应自动重连" 的 transient retryable 语义内在冲突 —— client 按钦定自动重连，握手时 §12.1 步骤 5（`room_members` 表查询）立即 fail 4003（行已被心跳超时清理删了），reconnect 形同虚设
  - **[P2]** 跨文档语义不一致：V1接口设计.md §10.3 / §12.3 现说 client 主动 close / TCP 1006 也走"被动 leave"完整路径（删 row + 广播 `member.left`），但时序图设计.md §13.3（r2 修订版）说同样的 1006 / app close 仅清 ephemeral 不动 row —— 同一场景两套行为

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 心跳超时删 row 与 4005 retryable 语义冲突，reconnect 形同虚设 | P1 / high | architecture (contract self-consistency) | fix | `docs/宠物互动App_V1接口设计.md` §10.3 / §10.5 / §12.1 4005 / §12.3 `### 成员离开` |
| 2 | 非超时 WS 断开（client close / 1006）跨文档语义矛盾（V1 vs 时序图） | P2 / medium | architecture (cross-doc consistency) | fix | `docs/宠物互动App_V1接口设计.md` §10.3 / §12.3 ↔ `docs/宠物互动App_时序图与核心业务流程设计.md` §13.3 |

## 选择的修复方向：Path B（简化为单条规则）

> **WS 断开（含心跳超时 / 1006 / client close / app 关闭）仅清 ephemeral 层；持久层（`room_members` 行 + `users.current_room_id`）只能由 HTTP `POST /rooms/{roomId}/leave` 改变；`member.left` 广播严格 1:1 对应删行事件**

为什么不选其他路径：

- **路径 A（4005 改为不可重连）**：违反"transient network failure 应 retryable"的网络协议常识；网络抖动 / 切后台是常见 transient 场景，不让重连等于让用户碰一次抖动就回主界面，UX 严重恶化；同时 1006 / 1011 / app close 等场景的 retryable 语义连带要重新讨论，影响面大
- **路径 C（短 timeout 仅清 ephemeral，超过 X 分钟才真删 row + 广播）**：引入两阶段定时器 + 跨重启的"哪个阶段"状态记录，server 实装复杂度显著上升；产品层面节点 4 阶段无"长期断线僵尸成员"产品规则需要承诺，过度设计；后续若真有需求由独立 epic 引入清理策略
- **路径 B（本选）**：单条简单规则，跨文档容易对齐，4005 retryable 自洽，server 实装最少（onUnregister 钩子只清 ephemeral，不调 leave service 函数）；"僵尸成员"由产品规则兜底（节点 4 阶段无长期断线场景，client 重连后立即恢复占用座位）

## Lesson 1: 协议层"transient retryable"语义与"重连后能否回到原状态"必须**端到端**自洽，不能只在文档片段层局部正确

- **Severity**: P1 / high
- **Category**: architecture (contract self-consistency)
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §10.3 / §10.5 / §12.1 close code 4005 行 / §12.3 `### 成员离开`

### 症状（Symptom）

§12.1 close code 表 4005 行（心跳超时）写"client **应**自动重连（指数退避；视为 transient network failure，与 1006 / 1011 同等对待）"。但 r1 / r2 修复在 §10.3 / §10.5 / §12.3 都钦定"心跳超时清理钩子触发 → 删除 `room_members` 行 + `member.left` 广播"。两份文档单独看都自洽：

- 4005 retryable：client 按指数退避重新发起 WS 握手
- 心跳超时清理：清理"僵尸"成员，避免 roster 里挂着已离线的 user

但**端到端**走一遍：client 心跳超时被 close 4005 → 按钦定重连 → 重连握手走 §12.1 校验顺序 → 步骤 5 查 `room_members WHERE user_id = ? AND room_id = ?` → 表里行已被心跳超时清理钩子删了 → close 4003 "user not in room" → 4003 是不可重连业务级拒绝 → client 回退主界面

→ 4005 retryable 设计**完全失效**：client 一旦心跳超时就回不去原房间。"60s 心跳窗口的容错"本应让用户网络抖动恢复后无感继续使用，但因为窗口结束时清理钩子已删 row，重连永远过不了归属校验。

### 根因（Root cause）

把"心跳超时"既作为"清 ephemeral 连接态"的触发，也作为"清持久层 row + 广播业务事件"的触发，让一个时间阈值（60s）同时承载两种完全不同语义的状态机转换。但这两种语义的客户端期望相反：

- 清 ephemeral 期望 **"我的连接掉了，重连一下能继续"**
- 删 row + 广播 `member.left` 期望 **"我已经被服务端认定离开了，重连得重新 join"**

二者塞进同一个钩子 → close code 4005（"应该重连"）和 4003（"不应该重连"）会在重连路径上**前后相邻**地触发，client 没办法用单一 close code 处理 —— 即使 client 实装"4005 重连，4003 回退"分支正确，行为序列对用户来说就是"心跳超时 → 看似可以重连 → 重连立即被踢回主界面"。

更深层的契约设计漏洞：**协议层的 retryable 语义钦定不能只看"close 那一刻 client 的反应"，必须看"重连握手时 server 校验流程是否还能让 client 回到原状态"**。本 case 4005 retryable 的契约价值依赖 §12.1 步骤 5 查询能通过；步骤 5 通过的前提是 row 没被删；row 没被删的前提是清理路径不删 row。这三个层级**必须**对齐为单一规则，r1 / r2 在 §10.3 / §10.5 / §12.3 单独看自洽不等于和 §12.1 端到端自洽。

### 修复（Fix）

走 Path B：**WS 断开（含心跳超时 / 1006 / client close / app 关闭）仅清 ephemeral 层；持久层 `room_members` / `users.current_room_id` 只能由 HTTP leave 改变；`member.left` 广播严格 1:1 对应删行事件**。

具体改动：

- §10.3 重写"roster 语义与被动断线交互"小节为"roster 语义与 WS 断线交互"：核心钦定段明示"持久层 vs ephemeral 层"职责分离 + 各类 WS 断开场景的语义统一为"仅清 ephemeral，行不删，roster 不变" + memberCount 一致性段落简化（只有 HTTP leave 让 memberCount 递减）
- §10.3 "不变量" 段第 4 条：把"心跳超时清理钩子真正从 `room_members` 移除该行"改为"任何 WS 断开仅清 ephemeral，唯一删行路径是 HTTP leave"
- §10.5 服务端逻辑末尾"心跳超时被动断线场景"块重写为"WS 断线场景与本接口的关系"：明确"任何 WS 断开都不走本接口的事务路径，仅清 ephemeral；唯一例外是步骤 7 的 close 4007 协议确认（leaver 自己的 WS 由 HTTP leave 触发关闭）"
- §12.1 close code 4005 行：保留"client 应自动重连"，并附加 reconnect 自洽性说明（行不删 → 步骤 5 通过 → 用户保留座位）
- §12.1 close code 4005 例外注解：把 retryable 语义的可行性 link 到 §10.3 / §10.5 / §12.3 钦定共同保证
- §12.3 `### 成员加入` "触发"段：移除"WS 心跳超时清理钩子触发的'被动 leave'路径**不**触发 member.joined"项（被动 leave 概念已废）；改为强调 `member.joined` 与 `room_members` 行新增严格 1:1 对应（仅 HTTP join）
- §12.3 `### 成员离开` "触发"段：从三条触发条件（HTTP leave / 心跳超时 / client close / 1006）简化为单条（仅 HTTP leave）；附加 reconnect 自洽性说明
- §12.3 `### 成员离开` 字段表 `payload.userId` 字段说明：去掉"心跳超时钩子捕获的 Session.userID"来源描述（被动 leave 不存在了）
- §12.3 `### 成员离开` 关键约束："主动 leave vs 被动 leave 事件不重复"改为"触发不重复 = 1:1 对应删行事件"

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计任何"transient retryable" close code / 错误码**（WS / HTTP / RPC 任何协议层）时，**必须**走完整 reconnect / retry 链条 —— 从触发瞬间 client 反应 → 重试 / 重连请求 → server 端校验路径 → 是否能恢复到 close 之前的等价状态 —— 端到端**所有节点**都不能让该 retryable 失效；**禁止**在文档某段写"应自动重连"而其他段独立加入"会让重连失败的状态变更副作用"。
>
> **展开**：
> - 协议层 retryable 语义不是"close 那一刻 client 该如何反应"的局部声明，是**整条链路**的端到端契约：close → backoff → 重连请求 → 校验 → 恢复状态。任一节点把恢复路径堵死，retryable 就是空头支票
> - 写完 retryable close code 后，必做 mental walkthrough：
>   1. close 触发的瞬间 server 做了哪些副作用（删 row / 改字段 / 广播事件 / 清缓存）？
>   2. 这些副作用对"重连握手 / 重试请求"的合法性校验有影响吗？
>   3. 如果有，重连 / 重试会不会立即被另一个 close code / 错误码拒绝？
>   4. 如果会，retryable 是假承诺，必须修
> - **同一个时间阈值 / 触发条件**承担两种语义截然不同的状态变更（如"清 ephemeral" + "改持久层"）是高风险设计 —— client 看不到内部分层，看到的就是一个不一致的 close code 序列；要么拆成两个不同 close code（每个一种语义），要么把两种状态变更绑死在同一逻辑层（且确保 retryable 链条内端到端自洽）
> - **反例 1**：本 case r2 修复："心跳超时 60s = transient retryable + 删 row + 广播 left" → 三种语义塞同一阈值 → 端到端自洽断裂
> - **反例 2**：HTTP 401 写"client 应静默刷新 token 重试"，但 token 刷新的 endpoint 又对"短期内大量 401"做了限流封禁 → 第二次 401 的"重试"方案无效
> - **反例 3**：MQ consumer ack timeout 写"server 应重发"，但重发逻辑在"已转 dead-letter"前没检查 → 短暂处理慢的 consumer 收到的是 dead-letter ack 而非业务消息

## Lesson 2: 跨文档同议题"分支条件 + 副作用"声明必须合并为单条规则，禁止用"双方都对"逃避对齐

- **Severity**: P2 / medium
- **Category**: architecture (cross-doc consistency)
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §10.3 / §12.3 ↔ `docs/宠物互动App_时序图与核心业务流程设计.md` §13.3

### 症状（Symptom）

V1接口设计.md §10.3 / §12.3（r2 修订版）说：

- client 主动 close / app 关闭 / TCP 1006 → 触发 onUnregister 钩子 → 走"被动 leave"路径 → 删 `room_members` 行 + 广播 `member.left`

时序图设计.md §13.3（r2 修订版）同时说：

- client 主动 close / TCP 异常断开 → 仅清 ephemeral 连接态（Redis 连接映射 + 在线集合）→ **不**广播 `member.left`，业务侧 roster 不变
- 心跳超时被动断线（独立子段）→ 走"被动 leave"完整路径

→ 同一场景（client close / 1006）两份文档给两套不同行为：V1 说"删 row + 广播"，时序图说"仅清 ephemeral 不广播"。server 实装看 V1 还是看时序图，行为完全相反。

### 根因（Root cause）

r2 修复跨文档同步时把"心跳超时 = 走被动 leave 完整路径"语义复制到了时序图，**但**没把 V1 文档新加的"client close / 1006 也走被动 leave"语义同步过去 —— 时序图保留了 r1 之前"client close / 1006 仅清 ephemeral"的旧表述。这是典型的"复制 80% 表述忘了 20%"+"两份文档独立演化历史导致的差异未被审查捕获"。

更深层的契约设计漏洞：**当协议把同一类事件（"WS 断开"）按子情形拆成多种处理路径时**，多份文档容易在拆分粒度上不一致 —— 一份按 "心跳超时 vs 其他" 拆，另一份按 "主动 close vs 被动 close" 拆，两套分类系统在边界 case（client close / 1006 / app close 同时算"非心跳超时"也算"被动断开"）上行为不重叠。修一边漏一边的概率很高。

最干净的修复不是"两边精确对齐分类系统"，而是"消除分类本身" —— 让所有 WS 断开走单一处理路径（清 ephemeral，不动持久层）。这样跨文档只有一条规则需要对齐，且永远只能对齐到一条结果。

### 修复（Fix）

走 Path B 的连带效果：

- V1 §10.3 / §12.3 / §10.5：所有"被动 leave"路径都被废止；任何 WS 断开（含心跳超时 / 1006 / app close）都仅清 ephemeral，唯一删 row 路径是 HTTP leave
- 时序图 §13.3：重写为"WS 断连分两个职责层（持久层 vs ephemeral 层）"+ 单一处理段（任何 WS 断开仅清 ephemeral）+ 强调"WS 断开 ≠ 离开房间"+ 明示心跳超时是 WS 层断开的子情形（与 1006 / 主动 close 同等对待）
- 时序图 §13.3 末尾添加 r3 锁定背景注解：解释为什么 r2 的"被动 leave"被废，引用 V1 §12.1 4005 retryable 自洽要求

跨文档单一规则达成："所有 WS 断开仅清 ephemeral，房间归属只能由 HTTP leave 改变" —— V1 / 时序图 / epics.md（Story 11.8 / 13.1 验收场景 8 / 13.2 / 13.3 tech debt 登记）全部更新到这条单一规则。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **跨文档协议契约把同类事件拆成多个子分支处理路径** 时，**必须**先评估"能否合并为单分支"—— 若可合并，**优先**合并，**禁止**把"两份文档各自分类自洽"作为对齐策略。
>
> **展开**：
> - 多分支处理路径在跨文档表述时容易出现"分类粒度不一致" → 边界 case 行为分歧 → review 不易抓到（每份文档单独读都自洽）。合并到单分支可彻底消除这类风险
> - 评估是否能合并：(a) 各分支的副作用差异是否真的产品需要？(b) 副作用差异能否由更下游的产品规则按需引入（独立 epic / 独立 story）？(c) 合并后单分支的语义是否自洽不破其他契约（本 case：合并后 4005 retryable 自洽）？
> - 若必须保留多分支，至少做到："分类维度"单一（按事件类型 / 按 close code / 按持久层影响等，**只**用一种维度切分），让跨文档复制时分类系统天然一致；并在主文档（V1接口设计.md）末尾加"跨文档对齐 checklist"显式引用其他文档的对齐位置
> - **反例 1**：本 case r2："心跳超时走被动 leave；其他 WS 断开走 ephemeral 清理" → V1 / 时序图分别按不同子情形展开，边界 case（client close / 1006）一份说"被动 leave"一份说"仅 ephemeral"
> - **反例 2**：错误码处理"transient 重试 / terminal 不重试"在 V1接口设计.md 按错误码段位（5xxx / 6xxx）拆，在 iOS 客户端结构.md 按 user-facing UX（toast / banner / fullscreen alert）拆 → 同一错误码两边映射逻辑不一致
> - **反例 3**：事务边界"哪些步骤在 tx 内 / 哪些 fire-and-forget"在数据库设计.md 按表分类，在 V1接口设计.md 按服务端逻辑步骤号分类 → 添加新表 / 新步骤时两边维护漂移

---

## Meta: 本次 review 的宏观教训

r1 / r2 / r3 三轮 review 同源于一个**结构性**契约设计漏洞：**用一个时间阈值（60s 心跳窗口）同时承载"transient 网络容错"和"持久层 row 清理"两种语义不同的状态机变化**。

- r1：发现"含离线成员"概念与"心跳超时删 row"矛盾 → r1 修：把语义改为"60s 内是 transient 容错 / 60s 后是真离开"
- r2：发现 client 主动 close / 1006 没走相同路径 → r2 修：把所有 WS 断开统一走"被动 leave 完整路径"
- r3：发现 r2 修后与 §12.1 4005 retryable 端到端冲突 → r3 修（本次）：把"心跳超时 = 真离开"语义**整体废弃**，所有 WS 断开仅清 ephemeral，房间归属只能由 HTTP leave 改变

更深层教训：**协议层"transient 容错时间阈值"和"持久层状态变更触发"应该用不同机制承载**。前者是 ephemeral 层（连接活性、Redis presence、SessionManager session），后者是显式用户行为（HTTP API call）或独立的产品规则（如"长期断线 24h 后离开房间"由 cron job 承载，不混入 close code 链路）。把两者混在同一个钩子里 → 任何后续修复都会撞上"想保 retryable 又想清僵尸"的两难，三轮 review 都没逃出这个陷阱。

未来 Claude 设计协议层 close code / 重试 / 心跳类契约前先画一遍：

1. 哪些副作用是 ephemeral 层（连接、缓存、内存）？
2. 哪些副作用是持久层（DB row、文件、外部服务状态）？
3. 这两类副作用的触发条件是否被同一个时间 / 事件钩子承载？
4. 如果是，能否拆开（ephemeral 层用一个钩子，持久层用另一个独立钩子或显式 API）？
5. 如果不能拆，retryable 链路端到端是否仍然自洽？

这一动作能在契约定稿前拦住 90% 的"看似自洽其实端到端断裂"陷阱。
