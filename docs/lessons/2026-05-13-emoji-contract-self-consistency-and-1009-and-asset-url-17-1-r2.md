---
date: 2026-05-13
source_review: codex review round 2 输出（/tmp/epic-loop-review-17-1-r2.md 末尾 codex 段）
story: 17-1-接口契约最终化
commit: eab7bc5
lesson_count: 3
---

# Review Lessons — 2026-05-13 — 冻结声明必须与详细规约同步 & 错误响应表必须枚举 DB-error 1009 路径 & enabled 资源契约不允许空字符串（17-1 r2）

## 背景

Story 17.1 r1 修复了 §12.2 `emoji.send` 的 6004 校验权威源（从 `users.current_room_id != NULL` 改为 `users.current_room_id == Session.roomID`），但只改了**详细规约**段，没同步**§1 末尾节点 6 freeze 声明摘要段**——两段对同一不变量描述不一致，未来 review 可能把弱的摘要当成"冻结契约"，绕过 stale-Session 校验回归。同时本轮 r2 还指出两个独立的契约空洞：(a) `emoji.send` 服务端逻辑步骤 3 / 4 的 DB 错误路径完全未规定（client 无确定行为可依赖）；(b) §11.1 GET /emojis 响应 schema 允许 `assetUrl == ""`，但 Story 17.3 seed 与 Story 18.1 渲染都钦定非空。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | §1 节点 6 freeze 摘要的 6004 描述与 §12.2 详细规约不一致 | medium | docs | fix | `docs/宠物互动App_V1接口设计.md:58` |
| 2 | §12.2 emoji.send 未规定 DB 错误 / 内部错误的 1009 失败路径 | medium | error-handling | fix | `docs/宠物互动App_V1接口设计.md:2014-2038` |
| 3 | §11.1 assetUrl 响应契约允许空字符串，与 Story 17.3 seed + 18.1 渲染契约冲突 | medium | docs | fix | `docs/宠物互动App_V1接口设计.md:1773` |

## Lesson 1: §1 末尾节点 6 freeze 摘要的 6004 描述与 §12.2 详细规约不一致

- **Severity**: medium
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:58`

### 症状（Symptom）

§12.2 服务端逻辑已经在 r1 改成 `users.current_room_id == Session.roomID` 比对（封堵 stale-Session 跨房间注入），但 §1 末尾节点 6 freeze 声明的 6004 "触发条件"摘要仍写"走 `users.current_room_id != NULL` 查询"。同一份契约文档中**摘要段 vs 详细段对同一不变量描述不同**——任何把摘要段当"冻结契约源"的下游 review 会以弱的 `!= NULL` 为准，让 stale-Session 校验静默回归。

### 根因（Root cause）

跨章节冻结摘要的同步盲区：上轮（r1）修复时只把"详细规约"段改了，没意识到**§1 末尾还有一条 freeze 声明里也复述了同一不变量的简短摘要**。冻结声明用"抽象层 + 不变量"的口径表达，措辞和详细规约不完全一样，文本搜索"`current_room_id`"会同时命中两处但 r1 修复时只改了详细处，摘要处的旧措辞被遗漏。这种**摘要 / 详细双写**结构是高维护成本设计，但项目当前依赖它做"freeze 声明的快速查阅"，所以不能简单删摘要——只能在每次详细规约变更时**强制同步摘要**。

### 修复（Fix）

§1 行 58 的"7001 / 6004 触发条件冻结在抽象层"那一段，把 6004 抽象层从：

```
"走 ... / 走 `users.current_room_id != NULL` 查询"
```

改成：

```
"走 ... / 走 `users.current_room_id == Session.roomID` 比对（**权威源 = `Session.roomID`，不是 `users.current_room_id`**；与 §12.2 服务端逻辑步骤 3 + r1 review 锁定的反 stale-Session 校验对齐）"
```

显式提到 §12.2 + r1 review 锁定的反 stale-Session 校验，让两处描述形成**正向引用关系**，未来再有人改其中之一时引用会暴露出"另一处需要同步"。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **修改 V1 接口设计 doc 任一详细规约段的不变量**（错误码触发条件 / 字段约束 / 权威源声明）时，**必须同步检查 §1 末尾的 freeze 声明摘要段**，把同一不变量的摘要措辞改成与详细段对齐。
>
> **展开**：
> - 任何 freeze 声明里只要复述了具体校验路径（"走 `xxx` 查询" / "按 `yyy` 比对"），都属于"摘要 / 详细双写"，**必须**双向同步
> - 修改完详细段后，用 `grep` 搜索详细段里的核心识别词（如 `current_room_id` / `is_enabled` / `Session.roomID`）在整篇 doc 出现的全部位置，每一处单独确认是否需要同步
> - 在摘要里**显式回引详细段章节号 + 关键变更标签**（如"r1 review 锁定的"），让未来读者能反向追到详细处
> - **反例 1**：r1 review 只改 §12.2 详细的"步骤 3"，没动 §1 末尾摘要的"走 `users.current_room_id != NULL` 查询"措辞——同一文档里同一不变量两种描述，下游 review 会以弱的为准
> - **反例 2**：把 freeze 摘要简化成"详见 §12.2 步骤 3"完全去掉具体校验路径，看似零维护，但**牺牲了 freeze 声明做"快速契约对照"的核心价值**（freeze 声明的读者通常不点开详细章节）
> - **不要**简单删除摘要里的具体校验路径换成纯引用——保留摘要 + 同步即可，反例 2 的方案在本项目场景下不可取

## Lesson 2: §12.2 emoji.send 未规定 DB 错误 / 内部错误的 1009 失败路径

- **Severity**: medium
- **Category**: error-handling
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:2014-2038`

### 症状（Symptom）

§12.2 emoji.send 错误响应表只枚举了 1002 (参数错误) / 6004 (用户不在房间) / 7001 (emoji 不存在)；服务端逻辑步骤 3 (`SELECT current_room_id FROM users`) 和步骤 4 (`SELECT 1 FROM emoji_configs`) 都需要 DB 查询，但**当查询返回 `err != nil`**（driver 错误 / 网络抖动 / 慢查询超时 / 内部 panic）契约里没规定任何行为。Story 17.1 是 "freeze message contract before implementation starts" 的钦定 story，留这种空洞的后果：Story 17.5 实装时一种实现可能回 `error{code:1009}`、另一种实现可能直接 close WS socket、甚至有第三种实现可能静默吞掉错误不回任何消息——iOS Story 18.x 无稳定行为可对齐，最终再补救要做契约变更。

### 根因（Root cause）

"happy path + 业务校验失败"两类错误的契约思维定势——契约文档大多关注"业务规则违反"（参数错 / 不在房间 / 表情不存在），但实际生产中 DB / 网络 / 内部 panic 才是更高频的"协议层失败"。本项目 §3 已经定义了 1009 "服务端内部错误" 作为统一兜底错误码，§11.1 GET /emojis 也已经在错误码表显式列出 1009——但 §12.2 emoji.send 抄表时**遗漏了 1009**，因为思维里把"DB 错误"当成"实装细节"而不是"协议契约"。**WS 路径还有额外细节**：HTTP 路径回 500 后连接自然关闭，但 WS 路径**不应**因为单次消息处理失败就 close 整个 WS 连接（其他消息还需要走同一连接），需要明示"仅回 error 消息，连接保留"。

### 修复（Fix）

1. §12.2 服务端逻辑步骤 3 末尾增补 "DB 读取失败 → 回 1009 + 不广播 + 不关 WS 连接"
2. §12.2 服务端逻辑步骤 4 同样增补一行（步骤 4 是 emoji_configs 查询，独立失败路径）
3. §12.2 错误响应表追加 1009 行：`触发条件 = service 层步骤 3 / 4 DB 读取失败 / 内部 panic（与 §3 错误码表 1009 一致 + §11.1 同语义；server 不因此错误关闭 WS 连接）`
4. 在 1009 行的"失败后 client 推荐处理"列显式声明：提示"服务繁忙"toast + **不主动重试本次发送**（避免雪崩）+ WS 连接不主动断开

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **freeze 任何接口契约**（HTTP 或 WS）时，**必须**为接口的**每个 DB / Redis / 外部 IO 调用点**显式枚举 1009（或对应的协议层错误码）失败路径，**不能**把"DB 错误是实装细节"作为遗漏理由。
>
> **展开**：
> - 接口契约的错误响应表是 "freeze 给下游 implementer + 客户端 implementer 的稳定 surface"，**任何**实装可能回的错误码都必须出现在表里
> - HTTP 路径 1009 = "回 5xx + 通用响应结构 code=1009"；WS 路径 1009 = "回 `error` 消息 + payload.code=1009 + **连接保留**（不 close WS）"——契约里必须**显式区分**，因为 HTTP / WS 的故障模式不同
> - 在每个会做 DB 查询 / Redis 读写的服务端逻辑步骤里，明示"DB 读取失败 → 回 1009"——不要把它放在"通用错误处理"段，否则下游 implementer 看不到
> - 对照检查清单：步骤 X 涉及 IO？→ 错误响应表里有没有 1009 行？→ 错误响应表里的"触发条件"列有没有引用步骤 X？→ 缺一不可
> - **反例 1**：错误响应表只列业务校验错（1002 / 6004 / 7001），不列 1009——Story 17.5 实装时三种 implementer 各自选不同失败行为，iOS 端无法对齐
> - **反例 2**：列了 1009 但不区分 HTTP / WS 的连接保留语义——一种实装在 1009 时 close WS 连接（"反正服务繁忙不如断开"），另一种实装保留连接，iOS 端要么写两套 reconnect 路径、要么不知道哪种行为是契约
> - **反例 3**：把 1009 行的"client 推荐处理"列写成"自动重试"——WS 路径下 server 抖动场景 client 立刻重试会放大雪崩，必须明示"不主动重试"

## Lesson 3: §11.1 assetUrl 响应契约允许空字符串，与 Story 17.3 seed + 18.1 渲染契约冲突

- **Severity**: medium
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1773` + 同章节"关键约束"段

### 症状（Symptom）

§11.1 GET /emojis 响应字段表对 `data.items[].assetUrl` 写"0 ≤ length ≤ 255，可空字符串 `""`"；关键约束段重复同一描述。但 Story 17.3 钦定"每个 seeded emoji 必须有可访问的 asset URL"，Story 18.1 钦定"表情面板 cell 用 AsyncImage 加载 assetUrl"——空字符串会让 AsyncImage 渲染失败 / 占位降级。契约允许 `""` = server 可以"合法"下发不可渲染的 emoji 配置，与 Story 17.3 / 18.1 的钦定矛盾，且**契约一旦 freeze**这种漏洞难以再回去收紧。

### 根因（Root cause）

把"数据库 DDL 默认值"误当成"业务契约可空兜底"——`emoji_configs.asset_url VARCHAR(255) NOT NULL DEFAULT ''` 的 `DEFAULT ''` 是 **DDL 层的 schema 兜底**（避免 INSERT 时缺字段失败），它**不**意味着"业务层允许 enabled 表情留空 asset_url"。设计者画字段表时机械参考 DDL 写成"允许空字符串"，没回头对照 17.3 seed 钦定 + 18.1 渲染钦定。**两个 schema 层（DDL / API 契约）的语义混淆**是这类 bug 的典型根因。

### 修复（Fix）

1. §11.1 字段表 `data.items[].assetUrl` 范围/约束改成"1 ≤ length ≤ 255；**禁止**空字符串 `""`"
2. 说明列新增"Story 17.3 seed 钦定每个 enabled 表情必须有非空 `asset_url`（与 Story 18.1 表情面板 cell 渲染契约一致）+ server 端 seed 层 / admin 写入层**应**校验非空 + 数据库 `DEFAULT ''` 是 DDL 兜底**不**意味业务层允许 enabled 留空"
3. 关键约束段对应行从"assetUrl 可空字符串 `""`"改成"assetUrl 必非空字符串（**禁止** `""`）"，复述同样的 DDL vs 业务契约区分

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **API 响应字段表里描述字段的"范围 / 约束"**时，**必须**对照"该字段在下游 story 的实际使用契约"，**不**可机械抄 DDL 默认值。
>
> **展开**：
> - DDL 层的 `NOT NULL DEFAULT '空值'` 仅是**schema 兜底**（防 INSERT 失败 / 防 migration 历史数据缺失），**不**是"业务层允许空值"的授权
> - API 契约的字段范围必须由**下游 story 实际需要的最严格约束**驱动：seed story 钦定非空 + 渲染 story 假设非空 → API 契约就**必须**钦定非空
> - 设计字段范围时三处对照清单：(a) DDL；(b) seed / admin 写入层钦定；(c) 下游渲染 / 业务消费契约——三者取**最严格交集**
> - 在字段说明里显式写"DDL `DEFAULT 'xxx'` 是兜底，**不**代表业务层允许该值"——预防下次设计者重复同一思维定势
> - **反例 1**：字段表写"0 ≤ length ≤ 255，可空字符串 `""`"，说明列写"与数据库 DEFAULT '' 一致" + "client 解析层应按 String 处理并降级为问号 fallback"——一句话把契约和兜底语义混在一起，下游 server implementer 看到"可空"就以为业务层允许，下游 iOS implementer 看到"降级问号"就以为这是预期 UX
> - **反例 2**：只在字段表收紧到非空，但忘了同步关键约束段——同一份 doc 两处描述不一致（lesson 1 同款问题）
> - **反例 3**：在字段表写非空但不解释"为什么 DDL `DEFAULT ''` 不算允许空"——下次审查者会以为字段表和 DDL 矛盾，回头放宽契约

---

## Meta: 本次 review 的宏观教训

三条 finding 看似无关，但宏观上指向同一类思维漏洞——**"契约文档同一不变量在多处复述时，修改某一处后必须强制同步全部复述点"**：

- Lesson 1：§1 摘要 vs §12.2 详细对同一 6004 不变量两处描述不同步
- Lesson 2：§3 全局错误码表已定义 1009，但 §12.2 接口错误响应表没复述
- Lesson 3：DDL `DEFAULT ''` vs Story 17.3 seed 钦定 vs Story 18.1 渲染钦定 vs §11.1 API 字段表对同一字段约束四处复述，互相不一致

**未来 Claude 修改契约文档时的通用规则**：

> 把契约 doc 视为"多视图描述同一份事实"的格式（freeze 摘要视图 / 详细规约视图 / 错误码表视图 / 字段约束视图 / 关键约束视图）。任何一个视图里改了**事实**（不变量 / 触发条件 / 字段范围 / 错误码），**必须**列出其他视图里复述该事实的全部位置，逐一同步——这一遍同步的工作量不该回避，回避就是**契约自洽性 bug 的温床**。
>
> 实操手段：每次改完后，用 `grep` 搜索改动的核心识别词（如 `current_room_id` / `1009` / `assetUrl` / `DEFAULT ''`）的所有出现位置，**手动**逐一确认是否需要同步——**不要**依赖"我改完详细段了就行"的直觉。
