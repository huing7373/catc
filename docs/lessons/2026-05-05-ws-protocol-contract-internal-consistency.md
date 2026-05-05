---
date: 2026-05-05
source_review: codex review (Story 10.1 r1, file: /tmp/epic-loop-review-10-1-r1.md)
story: 10-1-接口契约最终化
commit: <pending>
lesson_count: 3
---

# Review Lessons — 2026-05-05 — WS 协议契约的"内部自洽"三连：reserved close code / 永久 null 字段引用 / 业务消息冻结边界

## 背景

Story 10.1（接口契约最终化）落地了 V1 接口设计文档的 §12.1 / §12.2 / §12.3 三节 WS 协议骨架，并在 §1 顶部声明了"节点 4 协议骨架冻结"。Codex review r1 在同一 patch 内识别出 3 处契约级矛盾 —— 全部不是"外部不一致"（与其他文档冲突），而是**同一文档的同一 patch 内部自洽性破坏**：

1. close code 表把 RFC 6455 reserved code `1006` 列为"服务端必须主动 close 时使用"
2. WS URL 字段说明把"获取 roomId"的来源同时指向 `GET /me.user.currentRoomId`（永久 null）和 `GET /home.room.currentRoomId`（节点 4 起回填真实数据）
3. 握手成功流程列出"服务端**应**广播 `member.joined`"，但同文档 §12.3 末尾 + §1 冻结声明都明示节点 4 服务端只发 `room.snapshot`/`pong`/`error` 三种消息

3 条全部 fix（无 defer / wontfix）—— 因为 review 命中的全部是文档内部已经存在 ground truth、但作者写这一句时漏看了的语义破坏。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | RFC 6455 reserved close code 1006 不能被服务端 emit | high (P1) | architecture | fix | `docs/宠物互动App_V1接口设计.md:1326-1334` |
| 2 | WS roomId 来源不能指向 `GET /me.user.currentRoomId`（永久 null 占位） | medium (P2) | docs | fix | `docs/宠物互动App_V1接口设计.md:1285` |
| 3 | 握手成功流程不能要求服务端广播 `member.joined`（违反节点 4 消息冻结） | medium (P2) | architecture | fix | `docs/宠物互动App_V1接口设计.md:1307-1308` |

## Lesson 1: RFC 6455 reserved close code 1006 不能出现在 close frame 内

- **Severity**: high (P1)
- **Category**: architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1326`（修复后 1327 行）

### 症状（Symptom）

close code 表"由谁产生"列原本不存在，所有 8 个 code（4001/4002/4003/4004/1000/1001/1006/1011）被一同列在"服务端必须主动 close 连接，并使用以下 close code"标题下；其中 `1006` 触发条件是"异常断开（无 close frame，TCP 中断 / 网络抖动）"。如果 Story 10.3（节点 4 server 实装）/ Story 12.5（iOS 重连）按这张表照抄实装，就会出现"服务端在 TCP 中断时主动写 1006 到 close frame"的错误代码 —— 而 RFC 6455 §7.1.5 明确规定 1006 是 **reserved**，**MUST NOT** be set as a status code in a Close control frame。

### 根因（Root cause）

写"服务端 close code 表"时，作者把"客户端可能观测到的所有断开 code"和"服务端可以主动 emit 的 close code"混为一谈。1006 的语义是"客户端 WebSocket runtime 在底层 TCP 断开且未收到 close frame 时**本地合成**给上层的 code"，本质上不存在"服务端发 1006"这条路径 —— 服务端要么发 1011（内部错误带 reason）/ 1001（going away）/ 1000（正常关闭），要么直接 TCP 断开（这种情况客户端运行时合成 1006）。文档把 1006 和 server-emitted code 放在同一张表的同一栏，违反了 RFC 6455 reserved 段的语义边界。

### 修复（Fix）

- close code 表新增"由谁产生"列，把 4001/4002/4003/4004/1011 标为 `server 主动 close`，1000/1001 标为 `server 或 client 任一方主动 close`，1006 标为 `仅 client 侧观测（不可被任一端 emit）`
- 1006 行触发条件描述补充 RFC 6455 §7.1.5 reserved 限制 + "客户端 WebSocket runtime 在底层 TCP 断开时本地合成"的精确语义
- 1006 行"服务端是否带 reason"列改为"不适用（无 close frame，因此服务端**不**emit 该 code）"
- "关键约束"列表把"1000/1001/1006/1011 是协议 / 网络级断开"改为"1000/1001/1011 是协议 / 网络级断开（1xxx 段是 RFC 标准段，可由服务端主动 emit）"，并新增"1006 例外"段：明确禁止服务端实装层写 1006 到 close frame
- 保留 1006 行而非删除 —— 客户端侧重连策略仍需要它（1006 → 自动重连），但通过"由谁产生"列明示它不属于服务端可 emit 集合

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"服务端必须 close 时使用以下 status code"类的协议契约表** 时，**必须**先查 **RFC 6455 §7.1.5 reserved status code list**（1004 / 1005 / 1006 / 1015），把 reserved code 单独标注为"client-observed only / server MUST NOT emit"，**不能**混入 server-emit 列。
>
> **展开**：
> - WebSocket close status code 分三段：1000-2999 为协议 / IANA 定义，3000-3999 为 IANA 注册库，4000-4999 为应用自定义（RFC 6455 §7.4.2）。在 1000-2999 段里，1004（reserved）/ 1005（no status received）/ 1006（abnormal closure）/ 1015（TLS handshake failure）四个是 **MUST NOT be set in close frame** 的 reserved code
> - 写契约表时如果同时面向 server 实装方和 client 实装方，要么拆两张表（server-emit 表 + client-observed 表），要么加"由谁产生"列；不能让 server 实装方照表照抄就出错
> - **反例**：close code 表标题写"服务端必须主动 close 连接，并使用以下 close code"，然后在表里列 1006 —— 这就是踩坑形态。读表的 Story 10.3 dev 会写出 `conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(1006, ""))`，违反 RFC

## Lesson 2: 引用本文档其他章节的字段时，必须验证该字段是"可用值"还是"永久占位"

- **Severity**: medium (P2)
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1285`

### 症状（Symptom）

WS URL 模板字段说明里写："`roomId`：路径参数，必须是用户当前所在房间的 ID（client 从 `GET /me.user.currentRoomId` 或 `GET /home.room.currentRoomId` 拿…）"。但同一文档 §1 设计原则 + §4.3 Future Fields 引用块都明示：`data.user.currentRoomId`（仅出现在 `GET /me`）是**永久 schema 占位**（始终 `null`，无后续节点回填计划），获取真实房间归属请改用 `GET /home`。

如果 client（iOS Story 12.5 / 等）按字面执行，从 `GET /me` 拿 currentRoomId 拼 WS URL → 拿到 `null` → URL 变成 `wss://host/ws/rooms/null?token=...` → 服务端校验 4002（roomId 路径参数格式错）→ 客户端"不自动重连"业务级拒绝路径，整条 WS 接入流程在 fresh install 后立即失败。

### 根因（Root cause）

写 WS URL 字段说明时，作者把"两个字段都叫 currentRoomId"当成了"两个字段都能拿到 roomId"，没意识到一个是永久 null schema 占位 / 一个是真实数据。这是**符号撞名陷阱** —— `user.currentRoomId` 和 `room.currentRoomId` 字段名拼写一样、JSON 路径深度一样，但语义完全不同。文档前面已经花了 4 处篇幅（§1 引言 / §4.3 字段表 / §4.3 Future Fields 引用块 / §5 home 接口）警告这件事，但 §12.1 字段说明还是踩了。

### 修复（Fix）

- §12.1 `roomId` 字段说明删除"`GET /me.user.currentRoomId` 或"，明示"client 从 `GET /home.room.currentRoomId` 拿"
- 同一行追加显式警告：`**不要**使用 `GET /me.user.currentRoomId`，该字段是永久 `null` schema 占位，详见 §4.3 Future Fields 引用块`

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写跨章节字段引用** 时，对**同名字段**（`a.x.foo` vs `a.y.foo`）**必须**先 grep 全文核对每个引用点的字段语义（schema 占位 / Future Fields / 真实数据），**不能**因字段名拼写一致就当作可互换。
>
> **展开**：
> - "schema 占位字段"（永久 null）和"Future Fields"（按节点 increment）和"真实字段"（当下可用）是三种语义不同的状态。同名字段在两个章节可能分属不同状态
> - 对协议 / 字段说明类的写作，引用其他章节字段前 grep `<字段名>` 看其在文档其他出现点的描述，确认引用方拿到的值是 client 能用的；不能光看"这个字段叫 currentRoomId 可以拿房间 ID 吧"
> - **反例**：把"client 从 `GET /me.user.currentRoomId` 拿 roomId"和"client 从 `GET /home.room.currentRoomId` 拿 roomId"用"或"连接 —— 这就是踩坑形态。第一个永远拿到 null，第二个才是真实数据；用"或"等于把 null 当作合法选项

## Lesson 3: 同文档同 patch 内的"流程要求"必须和"消息冻结边界"对账

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1307-1308`

### 症状（Symptom）

§12.1 握手成功流程列出两条服务端主动推送：
1. `room.snapshot`（节点 4 字段层契约由本 story 锚定）
2. **可选广播**：服务端**应**对房间内其他在线用户广播 `member.joined`（业务消息，由 Story 11.1 锚定，本 story 不规定字段）

但同文档 §12.3 末尾的"业务消息延后锚定"块 + §1 顶部"节点 4 协议骨架冻结"声明都明示：节点 4 阶段服务端 → 客户端**只**会主动发送 `room.snapshot` / `pong` / `error` 三种消息，`member.joined` 字段层契约由 Story 11.1（Epic 11）独立锚定 + 独立冻结。

让 Story 10.3 / 12.x 实装方左右为难：到底节点 4 阶段要不要 emit / parse `member.joined`？字段都没冻结的消息怎么实装？

### 根因（Root cause）

写握手流程时，作者想写一段"完整的房间进入语义"（自我推满 + 推给其他在线用户），但忘了节点 4 阶段是按 epic 切片的 —— 业务消息是 Epic 11 的事，Epic 10 只锚定协议骨架。"应该"和"节点 4 范围"两个维度被混在一起。文档结构上其实已经设了消息冻结边界（§12.3 末尾的延后锚定块 + §1 顶部冻结声明），但握手流程小节自己写时绕过了这层边界。

### 修复（Fix）

- 删除"可选广播：服务端**应**对房间内其他在线用户广播 `member.joined`"这一行
- 在 `room.snapshot` 推送条目之后追加一段引用块，明示与 §12.3 业务消息延后锚定块 + §1 节点 4 协议骨架冻结声明的对账：节点 4 阶段服务端只发三种消息，`member.joined` 由 Story 11.1 锚定，**不**在握手时广播
- 不删除 `member.joined` 整体（其字段层契约 / 时序由 Story 11.1 单独定），只删除节点 4 阶段对它的"握手时广播"要求

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写按 epic 切片冻结的协议文档** 时，对**当前 epic 范围之外**的业务消息 / 字段，**必须**写成"由 Story X.Y 锚定，本 story 不规定 / 不要求"，**禁止**写"应该"/"必须"/"要"等动作要求。
>
> **展开**：
> - 协议骨架文档常见错误模式：作者脑中有一张"完整业务流程图"，写流程小节时无意识把后续 epic 的业务消息当作"已存在 / 可使用的语义"塞进当前 story 的流程要求里
> - 写完每段"必须 / 应该 / 要"动作要求后，回头核对该动作牵涉的消息 / 字段是否已经在当前 story 范围内冻结字段层契约。如果没冻结，要么把动作改成"由 Story X.Y 锚定，本 story 不规定"，要么把该消息从动作要求里删掉
> - **反例**：握手成功流程列"服务端**应**广播 `member.joined`（由 Story 11.1 锚定，本 story 不规定字段）" —— 这就是踩坑形态。"应"是动作要求，但"由 Story 11.1 锚定字段"等于说服务端拿不到字段定义还要 emit，逻辑闭环不成立

---

## Meta: 本次 review 的宏观教训

3 条 finding 全部命中"**同一 patch 内部自洽性破坏**"：

- close code 表 vs RFC 6455 §7.1.5 reserved 段（同 patch 内未对账）
- WS URL `roomId` 字段说明 vs §1 引言 + §4.3 永久 null 声明（同文档未对账）
- 握手流程 vs §12.3 + §1 节点 4 消息冻结（同 patch 不同小节互相打架）

对协议契约 / 大文档类的写作（>1000 行 single source of truth），写完任何一段"主动动作要求"或"字段引用"后，**必须**做一次三向对账：

1. **对外**：是否符合上游 RFC / 标准（如 WS RFC 6455 reserved code 段）
2. **对内（垂直）**：是否符合同文档 §1 顶部的设计原则 / 冻结声明 / 永久占位字段标注
3. **对内（水平）**：是否符合同 patch / 同文档其他小节对同一概念的描述

这次 r1 review 命中的 3 条全部是缺第 1 步或第 2 步对账。蒸馏角度，**契约文档的内部自洽性自检 checklist** 比单条字段错误更值得 ADR 化（如果未来再积累 3-5 次类似 review，可考虑把本 lesson 升级为 "ADR-XXXX: V1 接口设计文档写作 self-check checklist"）。
