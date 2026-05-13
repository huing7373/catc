---
date: 2026-05-13
source_review: "codex review round 4 output: /tmp/epic-loop-review-17-1-r4.md"
story: 17-1-接口契约最终化
commit: 20f8d49
lesson_count: 3
---

# Review Lessons — 2026-05-13 — 契约层不绑定 client 渲染器 / 上限要么有 SQL 要么删 / race window 不得简单 drop（17-1 round 4）

## 背景

Story 17.1 是 `GET /api/v1/emojis` + `WS emoji.send` / `emoji.received` 的契约最终化 story（contract-first，纯文档改动）。round 4 codex review 指出 3 处契约自洽性 / 客户端约束错误：① `assetUrl` 允许 GIF/WebP/SVG 但同句锁定 client 用 `AsyncImage` 渲染（renderer 不可靠渲染部分格式）；② `emoji.received` 处理规则把 "userId 不在 roster" 列为 "理论不可能"，但实际是合法 race（sender 发完立即 leave，receiver 先收 `member.left` 再收 `emoji.received`）；③ 响应字段表写 `0 ≤ length ≤ 50` 但服务端 SQL 是无 LIMIT 全量返回，契约自相矛盾。3 条均归类为 fix，已对齐到 `docs/宠物互动App_V1接口设计.md` 和 story 17.1 文件。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Narrow `assetUrl` formats to ones AsyncImage can render | P2 | docs/architecture | fix | `docs/宠物互动App_V1接口设计.md:1773` |
| 2 | Do not tell clients to drop `emoji.received` for users missing from roster | P2 | docs/architecture | fix | `docs/宠物互动App_V1接口设计.md:2473` |
| 3 | Remove the 50-item cap or add it to the query contract | P3 | docs | fix | `docs/宠物互动App_V1接口设计.md:1770` |

## Lesson 1: 契约层不得绑定特定 client 渲染器 / SwiftUI 组件能力

- **Severity**: P2 (medium)
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1773`（`data.items[].assetUrl` 字段表）+ `docs/宠物互动App_V1接口设计.md:1834`（关键约束段）

### 症状（Symptom）

契约层 `assetUrl` 字段描述写"PNG / GIF / WebP / SVG 等图片资源" + "cell 用 AsyncImage 加载 `assetUrl`"。问题是 SwiftUI 的 `AsyncImage` 并**不**可靠渲染 GIF / WebP / SVG（GIF 仅显示首帧，WebP 取决于系统版本，SVG 完全不支持） —— 一个 server 完全符合契约文本（返回 `.webp` URL）的实现，可以让 iOS client **必然**渲染失败。契约和实装之间出现"格式集合不一致"的悖论。

### 根因（Root cause）

把"契约能描述什么"和"client 实装层用什么组件"两层信息混在了同一行文字里。Contract-first 设计的目的恰恰是**让 server 实装 / client 实装可以独立演进** —— 契约只声明数据形状（resource URL 是字符串、必非空、长度限制），**不**规定客户端用什么具体技术栈渲染。一旦写进 "client 用 AsyncImage 加载"，契约就被锁死在某个 SwiftUI 组件的能力边界上，未来 client 换 SDWebImage / Kingfisher / Coil（Android）/ `<img>`（Web）都要改契约文档。

### 修复（Fix）

把字段描述从"PNG / GIF / WebP / SVG 等图片资源"改成"**标准 web 可访问的静态图片资源 URL（推荐 PNG）**"；把"cell 用 AsyncImage 加载"明确改为"具体 client 渲染器选型与可接受图片格式由各 client 实装层决定（iOS 端见 Story 18.1 dev notes）"。`AsyncImage` 仅作为**示例**保留（"如 iOS 的 `AsyncImage` / Android 的 `Coil` / Web 的 `<img>`"），明确"由各 client 实装层决定，**不**在契约层强制绑定"。

Before：
```
表情资源 URL（PNG / GIF / WebP / SVG 等图片资源）...
cell 用 AsyncImage 加载 `assetUrl`，空字符串会触发渲染失败 / 占位降级
```

After：
```
表情资源 URL —— 标准 web 可访问的静态图片资源 URL（推荐 PNG）...
具体 client 渲染器选型与可接受图片格式由各 client 实装层（如 iOS Story 18.1 dev notes）决定，
不在契约层强制绑定（避免契约耦合特定 SwiftUI / Android / Web 组件能力）
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写跨端契约文档（OpenAPI / WS schema / IDL）时，**禁止**让单字段描述同时声明"server 可下发的合法值集" + "client 用什么具体组件 / 库 / 系统 API 渲染"，二者必须分别归位到契约层和实装层 dev notes。
>
> **展开**：
> - 契约层 "field description" 只写：(a) 数据形状（type / length / charset / enum / nullable）；(b) **server 端约束**（业务校验规则、来源、唯一性）；(c) **跨端协议语义**（怎么算"该字段缺失" / "空值含义" / "ordering 保证"）
> - 客户端实装层约束（"用什么 SDK / 组件 / 哪个 iOS 版本支持" / "如何处理 nil / 空字符串"）写到对应 client story 的 dev notes，**不**写进契约字段表
> - 如果发现"契约字段约束依赖某个具体客户端能力"（如"client 用 AsyncImage 所以仅允许 PNG"），先反问：**是 server 端真的有这个约束（如 admin 写入层校验只允许 .png）？还是只是 client 实装顺手能渲染的格式？** 后者**不**进契约层
> - **反例**：字段表写 "`avatarUrl`: string, 1≤length≤255, client 用 Kingfisher 缓存，不支持 .heic" —— 这就是把客户端实装层缝进契约。正例改写：契约只写 "标准 web 可访问的静态图片 URL（推荐 jpg/png）"，可接受格式与缓存策略放 iOS dev notes
> - **反例 2**：WS 消息字段表写 "`payload.timestamp`: int64 ms, iOS client 用 `Date(timeIntervalSince1970:)` 解析" —— `Date` 构造是 client 实装细节，不该出现在契约层

## Lesson 2: 跨端 event 流的"理论不可能"分支几乎都是 race window

- **Severity**: P2 (medium)
- **Category**: architecture / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:2473`（`emoji.received` 处理规则 case (c)）

### 症状（Symptom）

`emoji.received` 处理规则原文：
> (c) `payload.userId` 不在 roster（**理论不该发生** —— server 广播范围 = `Session.roomID`，发起者必然在该房间 roster 中；除非 race condition：发起者发完 emoji.send 后立即 leave 房间，self-broadcast 到达时 leaver 自己已不在房间 roster；但 server §12.2 步骤 3 已校验 `current_room_id == Session.roomID`、§10.5 leave 事务会先 close 4007 关闭 Session，self-broadcast 到达不了 leaver；**故理论上无 race window**）→ 安全忽略 + log warn "emoji.received from unknown roster member"

这段推理只覆盖了 "**sender 自己** 是否能收到 self-echo" —— 但 case (c) 描述的是 **receiver B** 收到 sender A 的 `emoji.received` 时 A 不在 **B 的本地 roster**。这是完全不同的场景：A 发完 `emoji.send`（server 广播了 `emoji.received` 给 B）后立即 leave 房间（server 又广播了 `member.left` 给 B）。两条广播走**不同的 server 路径**：`emoji.received` 走 EmojiService.BroadcastToRoom（Story 17.5），`member.left` 走 RoomService leave 事务（Story 11.x）。它们到达同一个 receiver B 的物理顺序**不保证**严格一致 —— Goroutine 调度 / channel buffer / 网络传输都可能让 `member.left` 先到。结果：B 先把 A 从 roster 移除，再收到 A 的 emoji，按当前契约文本"安全忽略 + log warn"丢弃。这就丢失了 sender 最后一个表情。

### 根因（Root cause）

两层错误叠加：

1. **"理论不可能"是一个反审查习惯**：写下"理论不可能 / 不该发生 / 不会发生"几乎总是在掩盖未充分分析的 race。分布式系统 / 异步消息流里，两个独立路径的事件没有显式 happens-before（如 mutex / 同一连接的 FIFO 保证 / 事务串行化），物理到达顺序就**不能**假定。
2. **场景定位错位**：原文推理的是"sender 收不到 self-echo"（这条确实在 server §10.5 leave 事务会先 close 4007 关闭 Session 时成立），但 case (c) 涉及的是**第三方 receiver** 的视角 —— 它的 WS Session **没有**被关闭，它会收到所有广播。Server 单端的"逻辑互斥"不能延伸到多个独立 Session 接收顺序。

### 修复（Fix）

把 case (c) 改写为"**合法 race window**，**不**作契约违反处理"，明确解释场景（sender A 发完立即 leave，receiver B 先收 `member.left` 再收 `emoji.received`），并给出**降级渲染**方案：① 优先用 payload 自带字段（`userId` / `emojiCode`）触发飞出动效；② payload **不**自带 sender 头像 / 昵称（只有 `userId` + `emojiCode`），无 PetSpriteView 锚点（其 sprite 已随 `member.left` 移除）→ 降级到**房间中心位置**（或屏幕中央安全区）展示动效；log **info**（不是 warn / error，这是预期合法行为）；iOS Story 18.4 实装层落实"无 anchor 时的中心位置降级"渲染策略。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 设计跨端事件契约时，**禁止**把跨独立 server 路径产生的事件之间的物理顺序写成"理论不可能 / 不该发生"，必须显式列为 race window 并给出客户端 graceful degradation；只有同一 WS / TCP 连接内的消息才能依赖 FIFO 顺序。
>
> **展开**：
> - 写"理论不可能"前自问三次：(a) 这两个事件是不是同一个 server 函数 / 同一个事务内串行触发？(b) 是不是经过同一个 goroutine 串行写到同一个 client connection？(c) 是不是有显式 happens-before 锁 / mutex / channel ordering？三个都不是 → **必有** race window
> - 客户端 merge contract 对"sender 不在 roster" / "ack 早于 / 晚于 broadcast" / "B 端先收 X 再收 Y vs 先收 Y 再收 X" 这类场景**必须**写出 graceful degradation 路径，**不**得简单 drop / log warn / log error。Drop 等于"产品行为依赖底层调度顺序"，等于"客户端可能给用户展示错误状态"
> - 降级方案优先级：① 用 event 自身 payload 字段渲染；② 用 client 已缓存的静态配置（如 emoji code → assetUrl 映射）补齐渲染；③ 降级到安全位置 / 通用 placeholder；④ **最末**才考虑 drop（且必须 log warn 并明确这是 UX 退化）
> - **反例**：原 case (c) "安全忽略 + log warn" —— sender 最后一个表情消失，UX 表现为"对方根本没发表情"，是数据正确性问题，不是 graceful 降级
> - **反例 2**：WS chat 应用里"sender 发了消息然后断网，receiver 收到消息后 sender 离线状态变化早到"—— 不能因 sender 状态 = offline 就 drop 消息，必须照常显示
> - **反例 3**：HTTP 200 ack 和 WS 广播两个独立路径，永远不能假设"广播一定晚于 ack"或"ack 一定晚于广播" —— Story 14.4 / 15.4 的 self-broadcast 双轨兜底就是处理这类 race 的范式

## Lesson 3: 响应字段表的范围约束必须和服务端 SQL / 服务端逻辑严格一致

- **Severity**: P3 (low)
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1770`（`data.items` 范围约束） + `docs/宠物互动App_V1接口设计.md:1831`（关键约束段"不分页"描述）

### 症状（Symptom）

服务端逻辑明确写"`SELECT id, code, name, asset_url, sort_order FROM emoji_configs WHERE is_enabled = 1 ORDER BY sort_order ASC, id ASC`"（无 LIMIT）+ "不分页（即使数组为空也返回 `items: []`）"。但响应字段表同一份文档里写 `data.items` 范围 `0 ≤ length ≤ 50` —— 假设管理员未来 enable 60 个表情，server 真实返回 60 条，client 按契约预期最多 50 条做容量预分配 / 一次性渲染 / 字段校验 —— **两份不一致**。

### 根因（Root cause）

写契约范围字段时，**直觉地**给 array 加一个"合理上限"（防 server 误返大数组打爆 client）—— 但**没有同步**反映到：(a) 服务端 SQL（需要 `LIMIT 50` 才能让契约 truly hold）；(b) 服务端逻辑段（需要 truncate / error 路径处理 enabled 表情数 > 50 的情况）；(c) 查询契约（需要 `?limit=N&offset=M` 才能让 client 取到全量）。结果：契约范围只是"愿景"，不是"server 保证"，且和"全量返回"语义矛盾。Contract-first 的范围约束**只能写 server 真实保证的语义**，不能写期望或愿望。

### 修复（Fix）

选方向 (a)：删除 50 上限，与"不分页 / 全量返回"对齐。`data.items` 范围改为 "`length ≥ 0`（**不**分页 / 无上限）"，明确"MVP 阶段 enabled 表情数量较少（Story 17.3 seed 钦定 8-10 个），未来如需限制由对应 epic 引入分页 query 参数（如 `?limit=N&offset=M`）+ 服务端 `LIMIT/OFFSET` SQL 改造，本接口契约**不**预留"。关键约束段同步从"最大长度 50" 改为"**无条目数上限**"。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写 array / list 类响应字段的范围约束时，必须确认服务端 SQL / 服务端逻辑里有**对应的 LIMIT / 截断 / error**，否则**直接删上限**改为 `length ≥ 0` 或 `length ≥ 0（不分页 / 无上限）`，**禁止**写"善意期望"型上限。
>
> **展开**：
> - 契约层 array 长度约束的取值规则：
>   - 服务端有 `LIMIT N` SQL **且**有分页 query 参数 → 写 `0 ≤ length ≤ N`，并在字段描述里说"超过 N 条由分页 query 拉取剩余"
>   - 服务端无 LIMIT、无分页 → 写 `length ≥ 0`，明确"不分页 / 无上限"
>   - 服务端有 LIMIT 但**无**分页（截断）→ 写 `0 ≤ length ≤ N`，**必须**说明"超过 N 条会被截断（不分页）"，并标注 tech debt
> - 加范围约束前自问：（a）server 真的会保证这个上限吗？（b）是 server 哪一行代码 / SQL 保证？（c）超出上限时 server 行为是 truncate / error / 全量返回？三问无答 → **删上限**
> - **反例**：本次 review 命中的 `data.items` `0 ≤ length ≤ 50` —— server SQL 是全量 SELECT 没有 LIMIT，写 50 是凭直觉
> - **反例 2**：`data.members[]` 写 `0 ≤ length ≤ 100`，但 server `LEFT JOIN` 时没加 LIMIT 100 —— 同样是契约范围 ≠ server 真实保证
> - **正例**：分页接口 `0 ≤ length ≤ pageSize`，pageSize 来自 query 参数，server SQL `LIMIT ?`，契约范围由 query 参数动态界定，并明确"超过 pageSize 走下一页"
> - 凡是写"`0 ≤ length ≤ N`"且 server 没有对应 SQL 边界 → **必查必删**

---

## Meta: 本次 review 的宏观教训

3 条 round 4 findings 都属于"契约文档自洽性"的高密度缺陷类，且都涉及**同一个思维漏洞**：**契约层和实装层 / 服务端逻辑之间的语义边界漂移**。具体表现：

- Lesson 1：契约层悄悄混入了 client 实装层信息（AsyncImage 渲染器）
- Lesson 2：契约层把跨独立 server 路径的事件物理顺序当成"理论保证"（实际是 race）
- Lesson 3：契约层 array 范围与服务端 SQL 真实保证脱节

它们的根因都是：**写契约时没有显式回答"这一段是 server 保证 / client 实装约束 / 跨端协议语义 / 仅供示例" 的归属问题**。后续写跨端契约（无论是 REST / WS / 任何 IDL）前，**应**先在每个字段 / 每条规则旁标注归属：

- `[server-guarantee]`：服务端保证（SQL / 业务逻辑兜底）
- `[client-contract]`：client 解析层必须遵守的规则
- `[protocol-semantics]`：跨端协议的不变量（如 FIFO / happens-before / 持久化语义）
- `[example-only]`：示例，**不**是约束
- `[client-impl-hint]`：示例性的 client 实装提示，不强制

文档不要求**真的**写出这些 tag，但**写之前**要在脑中走一遍这个归属分类 —— 走完，会自然避开 round 4 命中的这 3 类问题。
