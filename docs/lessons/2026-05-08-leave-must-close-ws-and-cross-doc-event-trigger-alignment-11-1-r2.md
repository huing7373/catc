---
date: 2026-05-08
source_review: codex review (epic-loop r2) — /tmp/epic-loop-review-11-1-r2.md
story: 11-1-接口契约最终化
commit: 0b77824
lesson_count: 3
---

# Review Lessons — 2026-05-08 — HTTP leave 必须关闭 WS Session / `member.joined` trigger 示例必须含全 payload / 跨文档触发点声明对齐（11-1 r2）

## 背景

Story 11.1 r1 修复了 `member.joined` 字段表（lesson `2026-05-08-room-roster-contract-self-consistency-11-1-r1.md`）后，r2 codex review 发现三处契约自洽问题：(a) §10.5 leave 接口未要求关闭 leaver 自己的 WS session，导致 leaver 在 HTTP leave 后仍能收到该房间的 `member.joined` / `member.left` 等广播直到心跳超时；(b) §12.3 `### 成员加入` 触发示例 payload `{userId, nickname}` 与同节字段表（要求必含 `avatarUrl` + `pet.{petId, currentState}`）不一致，实装层若按示例做会让已连接成员永远看不到新成员的头像 / 宠物；(c) `时序图与核心业务流程设计.md` §13.2 / §13.3 仍声明节点 4 阶段不广播 `member.joined` / `member.left`，与 V1接口设计.md §1 第 37 行"自 Story 11.1 起 server → client active message set 升级"声明跨文档矛盾。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | HTTP leave 后未关闭 leaver 自己的 WS session，stale 订阅在心跳超时窗口（默认 60s）内仍收广播 | P1 / high | architecture (contract) | fix | `docs/宠物互动App_V1接口设计.md` §10.5 / §12.1 close code 表 |
| 2 | `member.joined` 触发示例 payload 缺 `avatarUrl` / `pet.*`，与字段表不一致 | P2 / medium | docs (contract example) | fix | `docs/宠物互动App_V1接口设计.md` §12.3 `### 成员加入` |
| 3 | 时序图设计.md 仍说节点 4 不广播 `member.*` 事件，与 V1接口设计.md §1 active message set 升级跨文档矛盾 | P2 / medium | architecture (cross-doc consistency) | fix | `docs/宠物互动App_时序图与核心业务流程设计.md` §13.2 / §13.3 |

## Lesson 1: HTTP 路径改变 ephemeral 连接态归属时，必须同步关闭对应 WS session

- **Severity**: P1 / high
- **Category**: architecture (contract)
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §10.5 服务端逻辑 + §12.1 close code 表

### 症状（Symptom）

`POST /api/v1/rooms/{roomId}/leave` 的服务端逻辑只描述了「删 `room_members` 行 + 更新 `users.current_room_id = NULL` + 广播 `member.left`」三件事，**没有**说明要关闭 leaver 自己持有的 WS 连接。在当前 WS 架构下：

- WS 授权只发生在握手时（§12.1 校验顺序步骤 5 `room_members` 表查询）；连接建立后 server 不再重新校验房间归属
- `BroadcastToRoom` 从 SessionManager 内存里的 room → sessions 映射 fanout，而 leave 事务**不**触碰 SessionManager

→ leaver 调完 HTTP leave 拿到 200 响应后，只要不主动 close WS（很多 client 实装不会立即 close，因为没收到协议层信号），就会在心跳超时窗口（默认 60s）内继续收到该 roomId 的 `member.joined` / `member.left` / 后续 `pet.state.changed` / `emoji.received` 等广播 —— 即"业务上已离开 + WS 上仍在"的脏状态。

### 根因（Root cause）

把房间归属管理拆成"持久层 (`room_members` + `users.current_room_id`)"和"ephemeral 层 (SessionManager)"两层后，HTTP leave 路径只改了持久层就 commit，没意识到 ephemeral 层是另一个 source of truth；广播 fanout 只看 ephemeral 层 → 持久层的修改对广播路径不可见，直到心跳超时把 ephemeral 层"碰巧"清理掉才一致。

更深层：契约文档写"广播 `member.left`" 时只想到了"通知房间里其他人"，没想到"通知 leaver 自己也已物理脱离 WS 订阅"是另一项独立必须的副作用 —— 二者在协议层都需要显式声明，不能依赖"反正心跳超时会兜底"。

### 修复（Fix）

§10.5 服务端逻辑新增步骤 7："关闭 leaver 自己的 WS Session" —— 从 SessionManager 撤销 + close underlying WebSocket（close code = `4007`，reason = `"left room via HTTP"`）；步骤必须发生在步骤 6 广播之后；fire-and-forget 不影响 HTTP 200 响应。

§12.1 close code 表新增 `4007` 行（业务级拒绝，client 不自动重连，视为"自身 HTTP leave 完成的协议层信号"用于触发 RoomView 退出 UX）；4001 / 4002 / 4003 / 4004 / 4006 / 4007 业务级集合 + 1000 / 1001 / 1011 + 4005 共 10 个 close code 集合更新。

心跳超时被动断线场景下步骤 7 退化为 no-op（leaver Session 已被心跳框架自然撤销），但 service 层调用语义保持一致 —— 主动 leave / 被动 leave 共用同一 service 层函数。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计任何"通过 REST 接口改变 user 在某个房间 / channel / room-like 容器中的归属关系"的契约**时，**必须**显式声明"是否同步关闭该 user 持有的 ephemeral 连接（WS / SSE / long-poll session）"，**禁止**依赖心跳超时兜底。
>
> **展开**：
> - 房间 / channel / 群组类容器有两层 source of truth：**持久层**（DB 表 + ID 字段）和 **ephemeral 层**（SessionManager / 在线集合 / pub-sub 订阅）；任何 REST 路径改了持久层就必须明确指定 ephemeral 层的同步语义
> - 广播 fanout 路径 99% 走 ephemeral 层；持久层的变更对广播**不可见**，直到 ephemeral 层被某个独立路径（心跳超时、TCP RST、LRU 淘汰）"碰巧"清理才一致 —— 这个窗口里"已离开但仍在订阅"的脏状态会持续接收广播
> - 关闭 ephemeral 连接的 close code **必须**与"无效 token / 房间不存在 / 心跳超时 / 协议违规"等已存在的 close code 段位区分 —— 业务级"主动离开协议确认"应分配独立 4xxx 段值，避免 client 把"自己主动 leave"误判为"被服务器拒绝重新加入"
> - **反例 1**：契约写"leave 接口删 DB 行 + 广播 `member.left`"，没写"关 WS session" → 心跳超时（60s）窗口内 leaver 仍收广播
> - **反例 2**：复用既有 4004 (`room not found`) 关 leaver session → 客户端 close 处理逻辑在"主动 leave 完成"和"加入了不存在的房间"两个完全不同 UX 路径上分叉，触发错误的 toast / 重试策略
> - **反例 3**：仅在心跳超时清理钩子里写"关 session" → 主动 leave 路径的 ephemeral 清理被推迟到心跳超时（60s），违反"REST 200 立即生效"语义

## Lesson 2: 协议触发示例 payload 必须与字段表逐字段一致，不能简化

- **Severity**: P2 / medium
- **Category**: docs (contract example)
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §12.3 `### 成员加入` 触发条件

### 症状（Symptom）

§12.3 `### 成员加入` 第一行「触发」条件示例：

```
BroadcastToRoom(roomID, {type: "member.joined", payload: {userId, nickname}})
```

但同节"字段"表声明 `avatarUrl` + `pet.petId` + `pet.currentState` **必填**。已连接成员仅在握手时收一次 `room.snapshot`（§12.1.3 钦定），后续无新 snapshot 触发；若 Story 11.8 实装层照"触发"行的简化示例下发 `{userId, nickname}`，已连接成员将永远看不到新成员的头像 / 宠物 —— 字段表里把 `avatarUrl` / `pet.*` 标"必填"无效。

### 根因（Root cause）

r1 lesson 已经修了字段表（要求 `member.joined` 必含 `avatarUrl` + `pet.*`），但同节顶部的"触发"行示例 payload 是 r1 之前的旧文本残留，r1 没扫到；r2 codex review 把字段表 + 触发示例**对照**读出来了。

更深层：契约文档里同一消息出现在多处（"触发条件"行 + "字段"表 + "JSON 示例"块），改一处忘改另一处的风险很高 —— 字段定义只能有一处 source of truth，其他地方应该是"引用"而非"复制"。但当前文档结构没有 enforced 引用关系，靠人工对齐。

### 修复（Fix）

§12.3 `### 成员加入` 触发行的示例 payload 改为完整字段：`{userId, nickname, avatarUrl, pet: {petId, currentState}}`；并在触发行末尾追加一段长 rationale：解释为什么不能简化（已连接成员无 snapshot 触发路径 → `member.joined` 是 enrich 唯一路径），把"必含 4 字段"的强约束直接挂在示例旁边而非散落到字段表。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **修改任何含字段表 + JSON 示例 + 触发条件示例多处展示的契约消息** 时，**必须**逐处对照修订，**禁止**只改字段表不改其他展示。
>
> **展开**：
> - 协议文档里同一消息常有 3 处展示：① 触发条件行内的简化 payload 示例；② 字段表（每行一字段，标必填 / 类型 / 含义）；③ 完整 JSON 示例块。这三处必须严格 zip 对齐
> - 修改前先 grep 同一消息名（如 `member.joined`），列出所有出现位置 → 逐处对照修订 → 提交前再 grep 一遍验证
> - "必填"字段在字段表声明，但在简化示例里只字段名（如 `{userId, nickname}`），需要审查者把 `{...}` 字段集合与字段表必填子集对齐 —— 任何不一致都是契约 bug
> - **反例 1**：r1 修了 `member.joined` 字段表加了 `avatarUrl` / `pet.*`，但忘改触发条件行的示例 → r2 review 抓出来
> - **反例 2**：JSON 示例里 `nickname: ""` 但字段表说"必非空字符串" → 二者哪边是真实约束？审查者无法判断
> - **反例 3**：简化示例 `payload: {userId}` 但字段表 5 个字段 → 实装层照简化示例做，必填字段没下发，下游永远拿不到

## Lesson 3: 跨文档同议题协议声明必须用统一 source of truth + 升级链路

- **Severity**: P2 / medium
- **Category**: architecture (cross-doc consistency)
- **分诊**: fix
- **位置**: `docs/宠物互动App_时序图与核心业务流程设计.md` §13.2 / §13.3 vs `docs/宠物互动App_V1接口设计.md` §1 第 37 行

### 症状（Symptom）

V1接口设计.md §1 第 37 行（Story 10.1 锚定）声明 active message set 边界**仅适用于 Epic 10**：「Epic 11 / 14 / 17 起，对应 §X.1 锚定 story 按各自语义把新业务消息合法加入 active message set，不视为本 story 冻结的违反」—— 即 Story 11.1 锚定后 `member.joined` / `member.left` 已合法进入 server → client active message set。

但时序图与核心业务流程设计.md §13.2 / §13.3 仍说"节点 4 阶段（Epic 10 ~ 13）服务端 → 客户端**只**会主动发送 `room.snapshot` / `pong` / `error` 三种消息，**不**广播 `member.joined` / `member.left`" —— 这是 Story 10.1 协议骨架冻结期的等价表述，没有跟随 Story 11.1 升级。

→ 工程师按时序图实装会主动**抑制**本契约要求的广播；按 V1接口设计.md 实装则会广播 —— 跨文档矛盾。

### 根因（Root cause）

V1接口设计.md §1 第 37 行的 active message set 升级声明用了"覆盖 Story 10.1 既有声明"的语义钉子，但**只**钉了 V1接口设计.md 内部 (§12.1.3 / §12.2 / §12.3)，没钉到时序图设计.md —— 后者复制粘贴了 Story 10.1 阶段的等价表述，独立于 V1接口设计.md 演化。

更深层：协议消息边界（active message set / 触发条件 / 广播时序）在两个文档同时存在 source of truth：V1接口设计.md（接口契约视角）+ 时序图设计.md（业务流程视角）。两份在 Story 10.1 阶段一致，但 Story 11.1 升级时只改了一份。

### 修复（Fix）

时序图设计.md §13.2 / §13.3 重写两处"业务消息延后锚定"注解块：

- §13.2 注解改为"业务消息触发点说明"：明确 WS 握手 vs HTTP join 是两个独立事件 —— 握手时仅给新连接的 client 推 snapshot 让自己初始化 roster；HTTP join 时广播 `member.joined` 给已在房间的其他成员让他们 enrich roster。两者协议角色互补，握手期不广播 `member.joined` 是事件分工本就如此，**不再**等价表述为"节点 4 阶段不广播 `member.*`"
- §13.3 注解改为"业务消息触发点说明"：明确 `member.left` 触发点 = HTTP leave 提交后 + 心跳超时被动清理；WS 主动 / TCP 异常断开**不**触发；自 Story 11.1 起 active message set 加入 `member.left`，引用 V1接口设计.md §1 第 37 行升级声明
- §13.3 流程描述补充"心跳超时被动断线"独立子段，与"主动断开 / TCP 异常断开"并列；前者走"被动 leave"完整路径（删 row + 广播 + 清 ephemeral），后者只清 ephemeral

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **修改协议契约文档（V1接口设计.md / 数据库设计.md 等"X 设计.md"）任一处的边界声明**（active message set / 状态机 / 错误码集 / 字段冻结清单）时，**必须**grep 全 `docs/` 目录找出所有相关同议题段落 → 逐处升级，**禁止**只在原文件内自洽就 commit。
>
> **展开**：
> - 协议契约通常分散在 6 份"X 设计.md"文档（总体架构 / 接口设计 / 数据库设计 / 时序图设计 / Go项目结构 / iOS项目结构）；同一议题（如"server → client 业务消息集合"）经常在 ≥ 2 份里有声明
> - 修改任一份的边界声明前：grep 关键标识（如 `active message set` / `member.joined` / `节点 4 阶段不广播`）找出所有出现位置 → 逐处对照"是否需要同步升级"
> - 边界声明的演化要用"覆盖 + 时间标记"语义：新声明里写"自 Story X 起覆盖原 Story Y 的 Z 声明"，让审查者能反向追溯哪份文档落后了
> - **反例 1**：V1接口设计.md §1 第 37 行升级 active message set，但时序图设计.md 仍在用 Story 10.1 阶段的等价表述 → r2 review 抓出来
> - **反例 2**：数据库设计.md §8 改了事务边界（如新增"步骤 7 关 session"），但 V1接口设计.md 服务端逻辑没同步 → 实装层不知该不该写
> - **反例 3**：用"以 X 文档为准"的脚注偷懒，让 Y 文档保持错误 → 工程师打开 Y 文档时不会想"这里可能是 stale 的，去 X 验证"，直接照抄

---

## Meta: 本次 review 的宏观教训

三条 finding 同源于一个宏观漏洞：**契约文档存在多个 source of truth（同一文档内多处 / 跨文档），契约演化时只改主入口、忘改其他承载点**。

- Lesson 1：把"持久层 + ephemeral 层"两个 source of truth 都视作契约一部分，缺一个就有 stale 状态窗口
- Lesson 2：把"字段表 + JSON 示例 + 触发条件示例"视作三个 source of truth，必须 zip 对齐
- Lesson 3：把"V1接口设计.md + 时序图设计.md + 数据库设计.md"视作多文档 source of truth，演化时必须扫齐

未来 Claude 改任何契约 / 协议文档前，先在心里画一遍 source of truth 拓扑图：本议题在哪几处声明？哪些是 authoritative / 哪些是 derived？修改时是否需要扫齐所有节点？这一动作能预防 90% 同类型契约 bug。
