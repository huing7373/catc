---
date: 2026-05-13
source_review: /tmp/epic-loop-review-17-1-r3.md (codex r3)
story: 17-1-接口契约最终化
commit: 8881b57
lesson_count: 2
---

# Review Lessons — 2026-05-13 — self-broadcast 不承担 ACK 职责 & 冻结摘要错误码列表必须与详细规约 / 中间件挂载边界一致（17-1 r3）

## 背景

Story 17.1 r3 review（codex）针对 r2 修完后的 `docs/宠物互动App_V1接口设计.md` 内 §1 冻结声明 + §12.2 `emoji.send` 详细规约，发现两条契约内部不一致：

1. §12.2 步骤 5 同段同时主张"self-broadcast = server ACK 信号"和"fanout fire-and-forget 允许 self-Session 漏收 self-broadcast"，两处逻辑互相否定，下游 client / 测试如果把 self-broadcast 当作 success ACK 判据会在 self-Session 单腿丢包时假阴性。
2. §1 冻结声明的错误码列表里列了 `1005`，但 §12.2 "不限频"段明确说 `emoji.send` 走 WS 路由 → 不经 HTTP rate_limit 中间件 → **不**触发 1005；同一份契约两处复述同一不变量但事实不一致。

两条 finding 都属于"已经在 r1 / r2 做过几轮跨段同步、但仍残留细颗粒不一致"。修复后 §1 摘要、§12.2 详细规约、§12.2 错误响应表三者对错误码列表的描述完全对齐；§12.2 步骤 5 self-broadcast 语义不再被误读为 ACK。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | self-broadcast 不应承担 server ACK 职责 —— 与 fire-and-forget 容忍 self-Session 漏收的语义自相矛盾 | medium (P2) | docs | fix | `docs/宠物互动App_V1接口设计.md:2024` |
| 2 | §1 冻结声明错误码列表列 1005，但 §12.2 明确 `emoji.send` 不走 rate_limit 中间件 → 实际无 1005 路径 | low / nit (P3) | docs | fix | `docs/宠物互动App_V1接口设计.md:58` |

修了 2 条 / defer 0 条 / wontfix 0 条。

## Lesson 1: self-broadcast 不承担 ACK 职责 —— 与 fire-and-forget 容忍 self-Session 漏收的语义自相矛盾

- **Severity**: medium (P2)
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:2024`

### 症状（Symptom）

§12.2 步骤 5 同段同时出现两段互相否定的描述：

- 前半句："如发起者需要'server 确认收到'的信号，依赖广播范围**包含发起者自己**的语义 —— 发起者会收到自己的 `emoji.received`，那就是 server 确认信号"
- 后半句（同段 "WS 广播 vs 客户端本地动效的关系" + "广播失败容忍"段）：fanout 是 fire-and-forget，"广播因网络抖动 / Session 已 close 失败（含发起者自己 Session 失败 → 自己收不到 self-broadcast），仅 log warning；client 端不应假设每条 `emoji.send` 都能 server-acknowledged via self-broadcast"

如果 Story 17.5 server implementer / Story 18.x iOS implementer / 测试编写者读契约时把"self-broadcast = server ACK"当作 success 判据，则 self-Session 单腿丢包场景下：server 实际**已接受** `emoji.send`、对房间内其他成员**已成功** broadcast、但发起者自己 Session 漏收 self-echo → client / 测试**假阴性** "server 没接受 / 超时"，可能触发不必要的重试 / 错误上报 / 测试 hang。

### 根因（Root cause）

设计者在补充"emoji.send 是 WS 单向消息无 ack"这一不变量时，想"借"广播范围包含发起者自己的语义提供一个"勉强可用的 ACK 信号"，但**没意识到 fire-and-forget 容忍 self-Session 单腿丢包**这一同段已存在的不变量与"self-broadcast = ACK"是**互相否定**的：

- ACK 信号的本质要求是"server 决定接受 → client 可观测的信号一定到达"
- fire-and-forget self-broadcast 的本质是"server 决定接受 → 是否广播成功是次要的、单腿丢包不影响成功判定"
- 二者不能在同一契约里**同时**为真

ACK 语义和 fire-and-forget 语义是**正交且互斥**的两套契约。把广播副产物当作 ACK 信号的思维定势源自 HTTP / RPC 范式（response 必到达），但 WS broadcast 的 fanout 不保证逐个接收者成功 → 用 fanout 路径承担 ACK 职责是范式错配。

### 修复（Fix）

`docs/宠物互动App_V1接口设计.md:2024` 步骤 5 末段重写：

- **删除**："如发起者需要'server 确认收到'的信号，依赖广播范围**包含发起者自己**的语义 —— 发起者会收到自己的 `emoji.received`，那就是 server 确认信号"
- **加入**：
  - 明示"`emoji.send` 是 WS 路径，**无 HTTP 响应、无 server → client ack 消息**，server 端'成功'= 仅完成上述步骤 1-5 + 广播尝试本身（广播是否真正送达任何接收者，包括发起者自己，**不**影响'成功'判定）"
  - 明示"**本契约不提供'server 已接受 emoji.send'的 client 可观测信号**：self-broadcast `emoji.received` **不**承担 ACK 职责 —— 由于 fanout fire-and-forget 允许发起者自己的 Session 漏收 self-broadcast（见下文'广播失败容忍'），任何'以 self-broadcast 到达视为 server ACK'的 client / 测试假设都会在 self-Session 单腿丢包时假阴性"
  - 明示"与本接口'emoji 是 transient UI 事件、本地动效已是发起者主要 UX 反馈、18.3 钦定本地动效立即播放不等 server'的设计一致"
  - 明示"如未来确需 server → client ack 信号，视为契约变更"

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计 WS 广播 / fanout 路径契约**时，**禁止**把"fire-and-forget 副产物（含 self-broadcast / fanout echo）"描述为"server ACK / 成功确认信号"。如客户端确需 ACK，必须显式设计独立的 ack 消息路径（不复用广播）。
>
> **展开**：
> - ACK 信号 = "server 决定接受 → client 可观测的信号一定到达"；fire-and-forget = "server 决定接受 → 广播副产物不保证逐个接收者送达"。两者**正交且互斥**，同一信号路径**不能**同时承担两职责
> - WS broadcast 的 fanout 不保证逐个接收者成功（含发起者自己的 self-echo —— Session close / 网络抖动 / 服务端瞬时故障都可能让 self-Session 单腿丢包）；用 fanout 路径承担 ACK 职责是 HTTP / RPC 范式错配
> - 设计 WS 单向消息（client → server，无 response）契约时，必须显式声明"本契约**不**提供 server-acknowledged 信号"+ 给出 UX 兜底方案（如本契约用"本地动效立即播放 + transient UI 事件 + 18.3 钦定的'网络极差时温和 toast'"作为发起者主要 UX 反馈）
> - 如果接口确实需要 server ACK（例如计费 / 状态变更类），**禁止**用 broadcast；改用 HTTP 响应（同步入账） / 独立 WS ack 消息（如 server → client 回带 `requestId` 的专用 `emoji.ack` 类型）—— 但这会让接口语义从 fire-and-forget 升级为"至少一次送达 / 强一致"，契约复杂度激增
> - 同段两处复述同一信号语义时（如步骤 5 的"server 确认"+ 末段的"client 不应假设 ACK"），用**同一句话**收口（"self-broadcast **不**承担 ACK 职责，因 ..."），不要分两段写两个相反的解读
> - **反例 1**：步骤 5 写"发起者会收到自己的 `emoji.received`，那就是 server 确认信号" + 末段写"广播失败仅 log warning，client 端不应假设 server-acknowledged via self-broadcast" —— 同段两处对同一信号给两个相反解读，下游 implementer 必踩坑（本 lesson 修复前的状态）
> - **反例 2**：契约里写"client 可以选择性把 self-broadcast 当作 ACK，否则用本地动效" —— "可选地把 fire-and-forget 副产物当 ACK" 比"不把它当 ACK"更糟糕，因为下游测试 / iOS / 三方 client 会各自选不同策略，互相对不上
> - **反例 3**：契约里说"self-broadcast = ACK，server 应**保证** self-Session 不丢包" —— 这相当于把单条消息路径升级为"至少一次送达"，与 fire-and-forget 设计前提矛盾；如要做此升级，必须重新设计整个 fanout 路径（含失败重试 / 离线队列 / Session 恢复后补发），单接口契约层无法承担
> - **正面范式**：(a) 显式声明"本接口无 ACK 信号"+ 显式给 UX 兜底；(b) 真需要 ACK → 升级为独立 ack 路径（专用消息类型 / HTTP 响应）；(c) 永远不要"借"广播路径做 ACK

## Lesson 2: 冻结摘要错误码列表必须与详细规约 / 中间件挂载边界一致 —— 不能机械抄通用错误码集

- **Severity**: low / nit (P3)
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:58`

### 症状（Symptom）

§1 冻结声明列了 `emoji.send` 的错误码 `(1001 / 1002 / 1005 / 1009 / 6004 / 7001)` 触发条件冻结，且在尾部"冻结边界说明"段为 1005 写了"触发条件冻结在抽象层 —— '走通用 rate_limit 中间件按 user_id 维度限频拦截'"的描述。

但同一份契约 §12.2 "不限频"段明确：

> 节点 6 阶段 server **不**对 `emoji.send` 做特殊限频（仅走 §12.1 / Story 10.4 心跳层 + Story 4.5 通用 rate_limit 中间件按 user_id 每分钟 60 次默认 —— **注**：rate_limit 中间件挂在 HTTP 路由，**不**挂 WS 路由，故 `emoji.send` 实际**不**走 1005 限频拦截）

§12.2 错误响应表也只列了 `1002 / 6004 / 7001 / 1009`，**没有** 1005 行。

§1 摘要 vs §12.2 详细规约 vs §12.2 错误响应表三者对同一不变量复述不一致：摘要说 1005 是冻结路径，详细说 1005 不存在。下游 implementer（Story 17.5 / Epic 18 / 测试编写者）会产生两种解读：

- 读 §1 摘要 → 认为契约要求实现 1005 拦截路径 → 写测试 / 拦截器 / iOS 端 1005 错误处理分支
- 读 §12.2 详细规约 → 认为契约**不**要求 1005 → 不写

### 根因（Root cause）

复用其它接口（HTTP 类，如 §10.4 / §10.5）冻结声明的错误码模板时，机械抄了"标准错误码集"（1001 / 1002 / 1005 / 1009 + 业务错误码），没意识到 WS 接口和 HTTP 接口的中间件挂载边界不同 —— rate_limit 中间件在 Story 4.5 决策里只挂 HTTP 路由（性能 + 实现简单 + 不污染 WS 消息层），WS 路由不经此中间件，故 WS 接口**不**暴露 1005 路径。

通用错误码集是按"全局错误码注册表"层面定义的（§3 §1252 等都列），但每个具体接口实际触发哪些错误码是**接口实装路径决定的**，而不是"通用错误码集都该出现在每个接口的冻结清单里"。

类似的还有 1003（pet 不存在）—— 只有触及 `pets` 表的接口才暴露；1004（房间满）—— 只有 join / create 路径触及。把通用错误码集当作"每个接口默认错误码列表"是错误的复用模板。

### 修复（Fix）

`docs/宠物互动App_V1接口设计.md:58` 修两处：

- 错误码列表 `(1001 / 1002 / 1005 / 1009 / 6004 / 7001)` 改成 `(1001 / 1002 / 1009 / 6004 / 7001)`（删 1005），并加 inline 注："`emoji.send` 走 WS 路由 → 不经 HTTP rate_limit 中间件 → **不**暴露 1005 路径，故 1005 **不**在本接口冻结错误码列表内；与 §12.2 '不限频'段 + §12.2 错误响应表对齐"
- "冻结边界说明"段删除关于 1005 抽象层条款的描述，改为只保留 7001 / 6004 的抽象层条款；末尾追加："**注**：本接口**不**冻结 1005 路径，故无 1005 抽象层条款 —— 如未来 WS 路由新增按 user_id 限频中间件 + 暴露 1005 错误码，视为契约变更"

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写接口冻结声明的错误码列表**时，**必须**从该接口**实际实装路径**反推（错误响应表 + 中间件挂载边界 + 服务端逻辑步骤）枚举错误码，**禁止**机械抄通用错误码集或其它接口的模板。
>
> **展开**：
> - 通用错误码集（如 1001 鉴权 / 1002 参数 / 1005 限频 / 1009 内部错误）是**全局注册表**层面的定义，**不**意味着每个接口都暴露所有码
> - 每个具体接口实际触发哪些错误码由**实装路径决定**：
>   - 中间件层（auth / rate_limit）—— HTTP / WS 挂载边界不同（如本项目 rate_limit 只挂 HTTP，WS 路由无 1005）
>   - 参数校验层 —— 几乎都有 1002
>   - 服务端逻辑层 —— 业务错误码（如 6004 / 7001）+ DB / IO 失败 1009
> - 冻结声明错误码列表的对照清单：(a) 该接口的中间件链上有哪些可能拦截的码？(b) 该接口的错误响应表里列了哪些码？(c) 该接口的服务端逻辑步骤里哪些步骤会回错？—— 三者取并集即为本接口的真实错误码集合
> - 不在该接口可触发集合里的错误码（如 WS 接口的 1005、不触 pets 表接口的 1003、非 join 路径的 1004）**禁止**出现在冻结声明里 —— 出现即是"冻结了不存在的不变量"，下游 implementer 会被误导
> - 跨段复述同一错误码集合时（§1 摘要 / §X 详细规约 / §X 错误响应表 / 关键约束段），必须**全部对齐**到同一集合 —— 任何一处改动后，grep 该接口名 + 错误码字面量，逐处确认
> - **反例 1**：抄其它 HTTP 接口冻结模板时直接复用 `(1001 / 1002 / 1005 / 1009 / 业务码)` 到 WS 接口冻结声明，没意识到 WS 路由不挂 rate_limit（本 lesson 修复前的状态）
> - **反例 2**：发现 1005 不适用于本接口后只删摘要里的列表，不删"冻结边界说明"里的 1005 抽象层条款 —— 两处仍互相矛盾
> - **反例 3**：摘要里删 1005 但不加 inline 注（"本接口为何不暴露 1005"）—— 下次 review 者会以为是"漏列"而不是"显式排除"，可能误加回来
> - **反例 4**：把通用错误码集照搬到所有接口冻结声明（如全列 1001 / 1002 / 1003 / 1004 / 1005 / 1009 + 业务码），假装"完整"—— 实际每个接口只能触发其中一个子集，列了不可达的码 = 噪声 = 误导

---

## Meta: 本次 review 的宏观教训

两条 finding 表面无关（一条是 ACK 语义错配，一条是错误码列表机械复制），但宏观上指向同一类"复用既有契约模板时未做'本接口特异性'校核"的思维漏洞：

- Lesson 1：复用"广播范围包含发起者自己"的语义（来自 `pet.state.changed` 的 self-broadcast 双轨兜底）时，没意识到本接口 fire-and-forget 已经否定了那条语义可承担的"ACK 信号"角色
- Lesson 2：复用其它 HTTP 接口的错误码冻结模板时，没意识到 WS 路由的中间件挂载边界不同 → 1005 不适用

**未来 Claude 写接口契约时的通用规则**：

> 复用既有契约段落（模板 / 错误码列表 / 语义描述）时，**必须**做"本接口特异性校核"：
> - 本接口的协议（HTTP / WS / 长连接广播）有什么实装路径差异？
> - 本接口的中间件链与模板接口是否一致？
> - 本接口的不变量（fire-and-forget / 强一致 / transient vs 持久化）是否与模板接口一致？
> - 复用的语义是否在本接口的不变量集合下仍然为真？
>
> 实操手段：复用段落写完后，逐句把段落里的每个事实判断（"X 是 Y" / "如果 A 则 B" / "X 错误码触发条件是 Z"）拎出来，对照本接口的协议层 / 中间件层 / 服务端逻辑层逐一验证；判断有任何一条在本接口下不成立，**必须**显式排除或重写，**禁止**保留有"歧义可能"的复用副本。
