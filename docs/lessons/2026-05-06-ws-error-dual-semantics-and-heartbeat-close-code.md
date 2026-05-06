---
date: 2026-05-06
source_review: codex review r3 of Story 10-1 接口契约最终化 (file: /tmp/epic-loop-review-10-1-r3.md)
story: 10-1-接口契约最终化
commit: 397eea6
lesson_count: 2
---

# Review Lessons — 2026-05-06 — `error` 消息的双重语义 & 心跳 close code 必须在冻结表里给具体值

## 背景

Story 10-1 是节点 4（Epic 10 ~ 13）WS 协议契约的最终冻结 story，r1/r2 review 已经处理过 reserved close code（1006/1009）、业务消息冻结边界、示例字面量自洽等内部矛盾。r3 review 找到的两条 finding 都是"信封表 / 章节表之间，或 §12.1 close code 表 / §12.2 心跳小节之间"的**章节级矛盾** —— 两份描述同一字段 / 同一断开场景，但措辞不一致，足以让下游 client / server 各自实装时按不同段落落地、产生 incompatible behavior。

本次 lesson 不是 Story 10-1 第一次出现"章节级矛盾"教训（参考 2026-05-05 同 story r1 lesson `ws-protocol-contract-internal-consistency.md`），但加了**两条新维度**：① 双重语义字段不能在信封表里被强行归为单一类（响应 / 广播）；② 冻结表（close code）声称冻结时**禁止**把任一行的具体值推给下游 story —— 推则等于自我证伪"已冻结"。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `error` 的 `requestId` 语义在 §12.3 信封表 与 `### error` 段表 之间矛盾 | high | docs/architecture | fix | `docs/宠物互动App_V1接口设计.md:1443` + `:1617` |
| 2 | 心跳超时 close code 推给 Story 10.4，但 §12.1 已声称是冻结的 close code 表 | medium | docs/architecture | fix | `docs/宠物互动App_V1接口设计.md:1410` + `:1319` 表 |

## Lesson 1: `error` 消息是双重语义字段，信封表里不能把它归为单一类

- **Severity**: high
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1443` (§12.3 server→client 通用信封表) ↔ `:1617` (`### error` 段字段表)

### 症状（Symptom）

§12.3 server→client 通用信封表把所有 server-pushed 消息分两类：响应类（`pong`，回带 `requestId`）vs 广播 / 主动推送类（`room.snapshot` / `member.joined` / `emoji.received` / `error`，固定 `""`）。`error` 被列在第二类。

但同一文件 1617 行 `### error` 小节字段表却写：

> 如该 error 是某 client 请求的响应（如 `emoji.send` 失败），回带原 `requestId`；如是 server 主动错误（如内部状态异常），固定 `""`

两段直接冲突。下游实装（Story 10.3 server / Story 12.3 iOS 解析层 / 未来 Story 17.1 emoji 业务）若按信封表落地 → `error.requestId` 永远 `""` → emoji.send 失败时客户端无法把 error 配回原 request；若按段表落地 → server 实现两条路径，但其他广播消息（`member.joined`）实装者只读信封表 → 不知道 `error` 也要 echo `requestId`，又写错。

### 根因（Root cause）

写信封表（"响应类 vs 广播 / 主动推送类"）时把 `error` 当成**单一形态消息**思考 —— 但 `error` 实际上是**两种语义在同一 type 上复用**：request-response 失败（响应类） + server 主动事件（推送类）。其他业务消息（`pong` 是纯响应、`member.joined` 是纯广播）都是单一形态，所以二分法成立；`error` 不成立。

> 触发条件：信封表 / 大类约束表 在归类一组消息时，把"行为依上下文动态切换"的消息（如 `error`）按高频形态硬塞进单一类。

### 修复（Fix）

把信封表 1443 行那一格的措辞拆成"形态-by-形态"而不是"消息-by-消息"，并显式说明 `error` 是**双重语义** —— 然后在通用信封表"关键约束"里加一条"特例"bullet，把 `error` 的双重语义和客户端解析建议（`requestId != ""` → 走 request-response 配对；`""` → 走全局错误事件总线）写明，让下游 Story 12.3 实装者读信封约束就够。

before（信封表）：
```
| `requestId` | string | 必填 | **响应类**消息（如 `pong`）回带 client 请求的 `requestId`（client 未提供 → 回 `""`）；**广播 / 主动推送类**消息（如 `room.snapshot` / `member.joined` / `emoji.received` / `error`）固定 `""`；**不**省略 key（即使值为空） |
```

after：
```
| `requestId` | string | 必填 | **响应类**消息（如 `pong`，及作为某 client 请求响应的 `error`）回带 client 请求的 `requestId`（client 未提供 → 回 `""`）；**广播 / 主动推送类**消息（如 `room.snapshot` / `member.joined` / `emoji.received`，及 server 主动产生的 `error`）固定 `""`；**不**省略 key（即使值为空）；`error` 的双重语义详见 §12.3 `error` 小节字段表 |
```

并在"关键约束"加一条特例 bullet。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"按消息分类的约束表"（如 server→client 信封 requestId 行为）** 时，**禁止**把"行为随上下文切换的双语义消息（如 `error`）"按高频形态硬塞进单一类，**必须**在表格行里显式提及双重语义并指向详细段表。
>
> **展开**：
> - 双语义消息的判别准则：同一 `type` 在不同触发路径下走不同字段语义。`error`（request-response 失败 vs server 主动事件）是典型；其他可能也是双语义的：未来如果 `room.snapshot` 既支持握手时主动推（无 requestId）又支持 client 显式 `room.refresh` 拉取（带 requestId），就会变成双语义
> - 信封表 / 大类表的归类原则：**单一形态消息**直接归类（如 `pong` → 响应类，`member.joined` → 广播类）；**双语义消息**单独列特例，不强行二选一
> - 客户端解析层（如 iOS Story 12.3 Codable）建议：双语义消息按 `requestId` 是否为空字符串分流（`!= ""` → 投递到 request 等待表完成 promise；`== ""` → 投递到全局事件总线）。该路由策略要在信封约束里说清楚，不要等到对应业务 story 再补
> - **反例**：在二分法的归类表里把 `error` 写到"广播 / 主动推送类"那一格，理由是"reasonable default 多数 error 是 server-pushed"，然后在 `### error` 小节再"详细说明"。结果：90% 的实装者只读信封表，不读段表 → 实装漏掉 request-scoped error 的 `requestId` echo

## Lesson 2: 冻结的 close code 表里**禁止**把具体行的具体值推给下游 story 决定 —— 推 = 自证未冻结

- **Severity**: medium
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1410` (§12.2 ping 心跳间隔) ↔ `:1319` 表 (§12.1 close code 表)

### 症状（Symptom）

§12.1 close code 表声称是节点 4 阶段的**冻结契约**（"§12.1 是被声明为 frozen 的 close code 表" —— Story 10-1 的核心 AC 就是"冻结接口契约"）。但 §12.2 ping 小节 1410 行写：

> 服务端 60 秒未收到任何消息（含 ping）→ Session 被 Story 10.4 心跳框架自动清理 + close（**close code 由 Story 10.4 决定，本 story 不规定具体 code**）

这等于在冻结表里留了一个"待定"格 → 下游 Story 12.5 iOS 重连分类（哪些 close code 走自动重连指数退避 vs 哪些走业务级回退）拿不到具体数值，会在心跳超时这条最高频的断开路径上分类失败 → server 实装（Story 10.4）和 client 实装（Story 12.5）可能各自挑不同 code（如 server 选 1011，client 期望 4xxx），双方都"按 spec 走"但 incompatible。

### 根因（Root cause）

把"协议层契约"和"实装侧选型"混淆：心跳超时**用什么 close code**是协议契约（client 必须知道才能写重连分类），不是 server 内部实装细节。误以为"close code 是 server 主动 emit 的，所以让 server 那个 story 决定"，但 close code 是 server / client **共同语言**，决定权属于协议 story 而非实装 story。

更深的根因：冻结表的"冻结"二字意味着**全部行 + 全部值都已确定**，不是"格式冻结、留空格的值待定"。"格式冻结值待定"这种状态在工程契约上等于"未冻结"。

> 触发条件：写"冻结契约表"时，对某一行的具体值不确定 / 觉得需要更多信息 → 临时写"由下游 story 决定"。

### 修复（Fix）

直接在 1410 行锚定具体 close code（`4005`，应用自定义段，reason = `"heartbeat timeout"`）+ 客户端处理（应自动重连指数退避，与 1006 / 1011 同等对待 —— transient network failure 类，不是业务级拒绝）。在 §12.1 close code 表里加 `4005` 行，并在表的"关键约束"里加一条"4005 是 4xxx 段中的例外，应自动重连"（因为 4xxx 段默认是业务级拒绝不重连）。同时更新 log level 指南：`4005 → log info`（这是常态，心跳超时多半是网络抖动 / 切后台，写 warn 会让正常网络抖动场景下日志噪声暴涨）。

为什么选 4005 而不是 1001 / 1011：
- `1011` 是"内部错误（panic / 不可恢复异常）"，写 log error 必排查 —— 心跳超时是常态网络抖动，不该当 internal error
- `1001` 是"going away（重启 / 切后台）"，含义太泛，client 区分不出"server 重启"vs"心跳超时"两种语义不同的断开
- `4005` 是 4xxx 应用自定义段下一个可用值（4001-4004 已用），语义专属于"心跳超时"，client 重连分类时按 code 直接路由

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"声称已冻结的协议表"（如 close code / 错误码 / 消息 type 列表）** 时，**禁止**对表中任一行的**具体值**写"由下游 story 决定 / 待 implementation 决定 / 留空"，**必须**当场锚定具体值；写不出具体值就不要声称"已冻结"。
>
> **展开**：
> - "冻结"的工程含义 = 表中**每一行 × 每一列**都有具体可比对的值，下游实装者读表就能 1:1 落地，不需要再去其他地方查"那个值到底是什么"
> - 区分"协议层契约"vs"实装侧选型"的判断方法：**这个值是双方共同语言吗？** 如果 client 要根据这个值做分类 / 路由 / UX 决策（如 close code → 是否自动重连），那它是协议层契约，必须在协议 story 锚定；如果只是 server 内部日志格式 / 内部告警阈值，那是实装层，可以推给实装 story
> - "待下游 story 决定"在协议表里**等于自证未冻结**：如果你真不确定，正确做法是在 story AC 里把这一行的"决定"也列为 deliverable（"在本 story 定 close code 4005"），而不是把决定推到下个 story。推 = 你这个声称冻结的 story 实际上没冻结
> - **反例**：在 §12.1 close code 表里写 `4001-4004` 全部具体 + reason，然后心跳超时这一最高频断开场景写成"由 Story 10.4 决定" —— 等于在冻结表里留一个 `<TODO>` 占位。reviewer 一眼看穿"这表没冻结完整"
> - **特例 4xxx 应用自定义段的设计原则**：4xxx 段默认是"业务级拒绝（不重连）"语义（如 4001 token 失效、4003 不在房间）；如果某个 4xxx code 需要"transient → 应重连"语义（如 4005 心跳超时），必须在表的"关键约束"里**显式列为例外**，否则 iOS 实装者会按段位默认规则分类成"不重连"导致心跳超时后用户掉线无法自动恢复

---

## Meta: 本次 review 的宏观教训

Story 10-1 的连续三轮 review（r1/r2/r3）都在揪"内部章节级矛盾" —— r1 揪 reserved close code 1006/1009 与全局错误码冲突，r2 揪示例字面量与字段表错位，r3 揪信封表与段表的双语义错误归类 + 冻结表里的"待定"占位。这三轮的共性 root cause：**协议契约文档的章节是分头写的，但章节之间共享同一组字段 / 同一套 code，章节级修改完成后必须做一次"跨章节一致性扫描"**。

未来 Claude 在写 / 改大型协议契约文档（特别是声称"冻结"的版本）时，**应**在结束 story 前跑一次自检：

1. **同一字段在多处出现的，措辞是否一致？**（如 `requestId` 在通用信封表 vs 各消息段表 vs 关键约束 bullet 三处出现 → 三处必须互相印证不冲突）
2. **同一 code / type 在多处出现的，每处是否给的同一值？**（如 close code 在表 vs 各触发段落 vs reason 字符串 vs log level 指南 vs 客户端处理推荐五处出现）
3. **声称"冻结"的表里有没有"由 X 决定 / 待定 / TBD / placeholder"？** 有就不算冻结
4. **双语义字段（行为依上下文切换）有没有被信封表二分法硬塞进单一类？** 有就要拆出来作为特例

这条 meta 规则 + 2026-05-05 的 ws-protocol-contract-internal-consistency.md 应当一起作为未来"协议契约 / API 冻结类 story"的 SM checklist 强项。
