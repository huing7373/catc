---
date: 2026-05-08
source_review: codex review round 10 (file: /tmp/epic-loop-review-11-1-r10.md)
story: 11-1-接口契约最终化
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-08 — `pet` 字段必须随 §5.1 GET /home 全协议 nullable / WS close 4007 是 best-effort cleanup 而非 leave 完成的 authoritative confirmation（11-1 r10 收官）

## 背景

Story 11-1（接口契约最终化）r10（review/fix 子循环最后一轮 fix 机会）。codex review 指出两条 P1/P2 contract-level 不自洽：(1) §10.3 / §12.3 / §10.4 / §10.5 把 `data.members[].pet.{petId, currentState}` 标为必填 mandatory，但 §5.1 GET /home 已经允许 `data.pet = null`（pet-less 账号是 contract 内合法 edge case，由 Story 4.8 强制覆盖）—— 这意味着 pet-less user 加入房间时，server 端 JOIN `users` / `pets` query 要么静默丢行（违反 `memberCount === members.length` 不变量）、要么 fabricate 假 pet 数据（违反真实性）；(2) §10.5 步骤 7-8 把 close 4007 framing 为 leave 完成的"protocol-level confirmation"，但 HTTP 响应和 WS close frame 走两条独立连接，client 可能根本收不到 4007（leaver WS 已断 / 4007 比 HTTP 200 晚到 / 4007 丢包均合法），等 4007 才 tear down 房间状态会让 client 卡在 leaving 状态。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 房间 roster `members[].pet` 必填与 §5.1 `data.pet = null` pet-less 账号契约自相矛盾，需要全协议改 nullable | P1 (high) | architecture / docs | fix | `docs/宠物互动App_V1接口设计.md` (§10.3 / §12.3 room.snapshot / §12.3 member.joined) |
| 2 | WS close 4007 不能作为 leave 完成的 authoritative confirmation；HTTP 200 是 protocol 层唯一权威信号，4007 是 best-effort cleanup | P2 (medium) | docs / protocol | fix | `docs/宠物互动App_V1接口设计.md` (§10.5 步骤 7-9 / §12.1 close code 表 4007 行)、`docs/宠物互动App_时序图与核心业务流程设计.md` (§12.2 mermaid + §13.3 注解) |

## Lesson 1: 房间 roster `members[].pet` 必须随 §5.1 GET /home pet 全协议保持 nullable，LEFT JOIN `pets` 防止 pet-less 用户被静默丢行

- **Severity**: P1 (high)
- **Category**: architecture / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §10.3 GET /rooms/{roomId} 字段表 + JSON 示例 + 五阶段过渡表 + 不变量；§12.3 `room.snapshot` 字段表 + JSON 真实示例 + placeholder 字段值来源说明 + client merge contract 的 null 处理路径；§12.3 `member.joined` 字段表 + JSON 示例（新增 pet:null 边界示例）+ 关键约束

### 症状（Symptom）

§5.1 GET /home 已经把 `data.pet` 声明为 `object | null`（pet-less 账号 = 用户无活跃 pet 行的 contract 内合法 edge case，由 Story 4.8 强制覆盖）；但本 story 写的房间相关接口（§10.3 GET /rooms/{roomId}、§12.3 `room.snapshot`、§12.3 `member.joined`）字段表统一把 `data.members[].pet.{petId, currentState, equips}` / `payload.members[].pet.*` / `payload.pet.*` 标为"必填"，未声明 pet 整体可空。pet-less 账号加入房间时，server 端任意一种实装都违反某条契约：

- INNER JOIN `users` / `pets`：pet-less user 的 `pets` 行查不到 → 整行被丢 → `memberCount === members.length` 不变量被破坏（snapshot/GET 响应 memberCount 是真实房间总数，但 members[].length 少 1）
- LEFT JOIN `pets` 但仍下发"必填" pet object：`pets.*` 列为 NULL → server 必须 fabricate 假 petId / 假 currentState → 违反真实性原则
- 简单地省略 `pet` key：违反字段表声明的"必填"

### 根因（Root cause）

契约 freeze 类 story 在做协议字段定义时，**没有跨章节统一扫描"同一对象 / 同一概念在不同接口里的可空性是否一致"**。`pet` 这个对象在 §5.1 已经是 nullable（pet-less edge case），但本 story 在写房间侧 roster 接口时只关注了"房间内的 pet 应该展示什么字段"，忽略了"用户层 pet 可以是 null → roster 里的 user 也应继承 nullable 语义"这条传递性约束。本 story 写到 r10 才被 codex 抓到，说明前 9 轮 review/fix 都没在"字段可空性 invariant 跨接口一致性"维度做横扫。

更深层：`docs/宠物互动App_数据库设计.md` 没有"用户必有 1 行活跃 pet"的硬约束（pet-less 是真实 edge case），但 §5.1 是文档体系内"唯一明确写出来 pet 可空"的接口；其他接口默认沿用了"pet 必非空"的隐含假设，contract freeze 类 story 缺一次穿透扫描。

### 修复（Fix）

- **§10.3 GET /rooms/{roomId}**：
  - 把 `data.members[].pet` 字段表整体改成 `object | null`（必填 nullable），`pet.{petId, currentState, equips}` 子字段全部加"仅当 `pet ≠ null`"修饰
  - 服务端逻辑步骤 3 显式锁定为 INNER JOIN `users` + **LEFT JOIN `pets`**，明示 INNER JOIN `pets` 会丢行违反不变量
  - JSON 示例新增 `userId: "1003"` 第三个成员 `pet: null` 边界 case + 示例说明文字
  - 五阶段过渡表新增 `pet 整体` 行（说明 LEFT JOIN 路径），子字段标注"仅 `pet ≠ null`"
  - 关键约束新增 `data.members[].pet` nullable 解析规则（`pet == null` 时整个 pet 子树不下发，**禁止** `pet: {}`）
  - ACL 注 + 不变量小节同步补 nullable 说明
- **§12.3 `room.snapshot`**：
  - `payload.members[].pet` 字段表改 `object | null`，子字段标注"仅 `pet ≠ null`"
  - JSON 真实示例新增 pet-less 边界（userId: "1003" + nickname: "C" + pet: null），memberCount 改成 3
  - placeholder 字段值来源说明明确：placeholder 阶段允许下发"`pet ≠ null` + `petId: ""`"占位结构（保留 client merge 兼容），Story 11.7 真实实装走 LEFT JOIN `pets` 判定路径
  - client merge contract null 处理路径新增明确语义：`pet = null` 是 authoritative pet-less 信号（与 server 不知道的空字符串不同），client 应直接覆盖
  - JOIN 描述统一改为 INNER JOIN users + LEFT JOIN pets
- **§12.3 `member.joined`**：
  - 触发段把 payload schema 改成 `pet: {petId, currentState} | null`
  - 字段表 `payload.pet` 改 `object | null`，子字段标注"仅 `pet ≠ null`"
  - JSON 示例分两份（常规 `pet ≠ null` + pet-less `pet == null`）
  - 关键约束更新：`payload.pet` 是 nullable，禁止下发 `pet: {}` 空 object 或省略 `pet` key

before/after 关键 diff（§10.3 字段表）：

```diff
-| `data.members[].pet.petId` | string | 必填 | 成员当前宠物主键... |
-| `data.members[].pet.currentState` | number (int) | 必填 | 宠物当前状态枚举... |
-| `data.members[].pet.equips` | array | 必填 | 成员当前装备数组... |
+| `data.members[].pet` | object \| null | 必填（nullable） | 成员当前宠物容器；与 §5.1 GET /home `data.pet` 同语义，**pet-less 账号**...时下发 `null`... |
+| `data.members[].pet.petId` | string | 必填（仅当 `pet ≠ null`） | 成员当前宠物主键... |
+| `data.members[].pet.currentState` | number (int) | 必填（仅当 `pet ≠ null`） | 宠物当前状态枚举... |
+| `data.members[].pet.equips` | array | 必填（仅当 `pet ≠ null`） | 成员当前装备数组... |
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在做"接口契约 freeze 类 story"时，**必须**在每一个 nullable 概念上做"全协议跨接口扫描"—— 任何 schema 字段（DTO 对象 / sub-object / value field）只要在某一个接口里被声明为 nullable，**所有引用同一概念的其他接口字段表必须显式标注同样 nullable**（除非有明确硬约束证明本接口下该值不可空）。
>
> **展开**：
> - **触发条件**：写 / 改 V1 接口 schema 文档；review V1 接口契约；做契约 freeze 故事（如 11-1 这种）
> - **检查清单**：扫描已存在 schema 的 nullable 字段（典型：`pet`、`avatarUrl?` / `nickname?` 这种 optional / `null` 字段、`currentRoomId | null` 这种 enum-with-null），然后 grep 所有引用这个 sub-object 的接口字段表，确认每一处都显式标注 `object | null` 而不是默认必填
> - **跨章节扫描动作**：对每个 nullable 概念，**至少**搜：(a) 字段路径名（如 `pet.petId`、`pet.currentState`）；(b) 所有"必填"修饰符；(c) 所有 JOIN 描述（`INNER JOIN xxx` 在 nullable concept 上必然踩坑，必须 LEFT JOIN）；(d) 所有 JSON 示例是否覆盖了 null 边界 case
> - **JOIN query 实装规则**：服务端逻辑写 SQL JOIN 时，凡是涉及 nullable 概念的子表（如 `pets` 表对应 nullable `pet` 字段）**必须**用 LEFT JOIN 而非 INNER JOIN，否则会静默丢行违反不变量（如 `memberCount === members.length`）
> - **client merge contract 必须区分 `null` vs 空字符串**：`null` 是 authoritative "明确无值"信号（应直接覆盖 client 已有值），空字符串是 "server 不知道"的 placeholder 信号（应保留 client 已有值）—— 两者语义不同，文档必须分别给出 client 处理路径
> - **反例**：r10 之前的 §10.3 / §12.3 字段表只关心了"房间内 pet 应该展示什么"，没注意到 pet 整体在 §5.1 已经 nullable；INNER JOIN `pets` 静默丢 pet-less user 行 → 违反 `memberCount === members.length` invariant

## Lesson 2: WS close 4007 是 server 端 best-effort cleanup，不是 leave 完成的 authoritative confirmation；HTTP 200 才是 protocol 层唯一权威信号

- **Severity**: P2 (medium)
- **Category**: docs / protocol
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §10.5 步骤 7-9 + 事务边界规则 + WS 断线场景关系；§12.1 close code 表 4007 行 + 关键约束；§12.3 `### 成员离开` 触发段 + 关键约束；`docs/宠物互动App_时序图与核心业务流程设计.md` §12.2 mermaid sequenceDiagram + §13.3 注解

### 症状（Symptom）

r10 之前 §10.5 步骤 8 把 close 4007 描述为 "client 解析层按 4xxx 业务级终态语义处理"，§12.1 close code 4007 行 client 行为指引说 "client 收到本 close code **应**视为'自己的 HTTP leave 已被 server 端确认完成'的协议层信号"—— 这把 4007 framing 为 protocol-level confirmation。但实际上 HTTP 响应和 WS close frame 走两条独立 TCP 连接，无任何协议层 ordering / delivery 保证：

- leaver 的 WS 可能早已断开（如 client 离开房间页前主动 close 了 WS）→ server 走"Session 撤销失败 no-op"路径，4007 frame 根本不会被发出
- 4007 frame 可能比 HTTP 200 晚到（WS 与 HTTP 是不同连接，TCP 层无 ordering）→ client 等 4007 引入不必要 UX 延迟
- 中间网络 / 代理层可能丢 4007 frame（best-effort 投递，无 ack）

如果 client 实装层把 4007 当 leave 完成的 authoritative confirmation 等待，会在合法场景下卡在 leaving 状态。

### 根因（Root cause）

WebSocket 协议层只保证"同一连接内消息的 ordered delivery"，**不**保证跨连接（HTTP vs WS）/ 跨 frame type（control frame 如 close vs application data frame）的 ordering / delivery。close frame 在 WS RFC 6455 里只是"我要关连接了"的提示信号，TCP RST 也可以让连接断在 close frame 到达 client 之前。

文档里把 4007 framing 为 confirmation 是因为顺手"复用 server 端业务事件的语义到 protocol 层"—— server 端确实在 leave 事务成功后立即发 4007，但这只代表 server 端的 cleanup 时机；client 侧 frame 是否到达是 transport 层的事，文档不能把 server-side 时机直接翻译成 client-side observable confirmation。

### 修复（Fix）

- **§10.5 步骤 8**：补一段说明 "**该步骤是 server 端 best-effort cleanup，不是 client 侧 leave 完成的 authoritative confirmation**"，解释 close frame 与 HTTP 不同连接 + 各种 4007 不到达的合法场景
- **§10.5 步骤 9**：明确"**这是 leave 成功的 authoritative 信号**：client 收到 HTTP 200 + `data.left: true` 即应立即清本地房间状态并退出 RoomView，**不**等待 close 4007"
- **§10.5 末尾新增 "HTTP 200 vs WS close 4007 — authority 与 best-effort 分工" 段**（r10 锁定）：systematically 列出 4007 不到达的 3 种合法场景 + HTTP 200 是唯一 authoritative signal 的 rationale
- **§10.5 "WS 断线场景与本接口的关系" 段**：把 "唯一例外是 §10.5 步骤 7 的 close 4007 协议确认" 改成 "步骤 8 的 close 4007 是 server 端 best-effort cleanup，不构成独立'离开房间'路径"
- **§12.1 close code 表 4007 行**：client 行为指引重写——把 "应视为'自己的 HTTP leave 已被 server 端确认完成'的协议层信号" 替换成 "**4007 是 best-effort cleanup signal，不是 leave 完成的 authoritative confirmation** —— **HTTP 200 响应是 leave 完成的唯一权威信号**，client **必须**以 HTTP 200 为推进 RoomView 退出 / 清房间状态的唯一触发点；4007 仅作冗余 UX 辅助 fallback"
- **§12.1 关键约束**：把 "4007 = 自身 HTTP leave 完成的协议层确认" 改成 "4007 = server 端 best-effort cleanup 信号，仅作 fallback UX 辅助"
- **§12.3 `### 成员离开` 触发段 + 关键约束**：把 "leaver 自己的 WS Session 由 §10.5 步骤 8 通过 close 4007 协议层确认完成" 改成 "leaver 自己以 HTTP 200 响应作为 leave 完成的 authoritative signal；步骤 8 的 close 4007 做 server 端 best-effort cleanup，不是 leave 完成的协议层确认"
- **时序图设计.md §12.2 mermaid sequenceDiagram**：在 post-commit Note 里加 r10 注脚，明示"close 4007 是 best-effort cleanup（HTTP 与 WS 走两条独立连接，无 ordering / delivery 保证）；leave 完成的 authoritative signal 是 HTTP 200 响应"；mermaid 箭头注释把 "WS Close 4007" 改成 "WS Close 4007 ... (best-effort, 可能不到达)"，HTTP 200 注释加 "(authoritative signal)"
- **时序图设计.md §13.3**：把 "唯一例外是 §10.5 步骤 7 的 close 4007 协议确认" 改成 "§10.5 步骤 8 的 close 4007 是 server 端 best-effort cleanup —— 不是独立'离开'路径"

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在协议契约里描述"事件 ack / 确认"时，**必须**先确认 ack 走的是同一连接还是跨连接 / 跨 frame type；**禁止**把跨连接 / 跨 frame type 的延后 signal 描述为 protocol-level authoritative confirmation —— **跨连接 ack 永远 best-effort，不可能 authoritative**。
>
> **展开**：
> - **触发条件**：协议设计文档里出现 "close X / frame Y / event Z 是某事件完成的协议层确认 / authoritative confirmation"；HTTP + WebSocket / HTTP + push notification / WS + REST 等多通道协议设计
> - **判别标准**：
>   - **同一连接**（如 WS 内部消息）：可以 authoritative，因为 RFC 保证 ordered delivery
>   - **跨连接**（HTTP 200 + 另一条 WS close frame）：永远 best-effort，无任何 protocol-level ordering / delivery 保证
>   - **跨 frame type**（同一 WS 连接内 control frame vs application frame）：control frame 优先级、TCP RST 等可让 control frame 不到达，仍属于 best-effort
> - **client 行为指引必须明示 authoritative vs best-effort**：authoritative signal = client 必须等待 / 收到才推进；best-effort signal = client 不应等待，可作 fallback UX 辅助；两者**不能混淆描述**
> - **server 端 cleanup 时机 ≠ client 端 observable confirmation**：server 端在 X 事件后立即发 cleanup signal 不代表 client 一定能收到；文档描述时**禁止**把 server-side 时机直接翻译成 client-side observable
> - **反例**：把 WS close 4007 framing 为 "client 收到本 close code 应视为'自己的 HTTP leave 已被 server 端确认完成'的协议层信号" —— 这条 framing 让 client 实装层有理由等 4007 才 tear down 房间状态，进而在 4007 不到达的合法场景（leaver WS 已断 / 4007 丢包 / 4007 比 HTTP 200 晚到）卡 UX
> - **类比**：HTTP 长连接 + Server-Sent Events 同样有这个陷阱；推送通知 + REST API 同步也有；任何 cross-channel ack 模式都要先用本规则审一遍

---

## Meta: 本次 review 的宏观教训 & r1-r10 整轮契约 freeze 故事的元蒸馏

### r1-r10 整轮契约迭代史复盘

11-1（接口契约最终化）这个故事走完了完整 10 轮 review/fix 循环（review/fix 子循环上限刚从 5 轮提到 10 轮，11-1 用尽了配额），每一轮都被 codex 抓到不同维度的契约自洽问题：

| Round | 主题 | 涉及维度 |
|---|---|---|
| r1 | 房间 roster 契约自洽 / member.joined 必须自包含丰富字段 | schema completeness（join 后 enrich 路径） |
| r2 | HTTP leave 必须关闭 WS Session / 跨文档触发点对齐 | cross-resource cleanup / cross-doc consistency |
| r3 | WS 断开仅清 ephemeral / 持久层 vs ephemeral 层职责分离 | event semantics layering |
| r4 | 创建/leave 并发 race 必须落到正确业务码 / 锚点稳定性 | concurrency error code mapping |
| r5 | `pet.currentState` 枚举跨章节对齐（§6.4 vs §6.5 同 1/2/3 不同义） | cross-section enum alignment |
| r6 | 协议字段表 vs JSON 示例 zip 对齐 / prose vs mermaid 对齐 | example fidelity / cross-format consistency |
| r7 | 接口默认 deny + 显式 allow（白名单 ACL）/ prose vs mermaid zip | ACL discipline / cross-format zip |
| r8 | ACL 校验 + 受 ACL 保护数据返回必须共享同一事务 snapshot | tx snapshot consistency for ACL |
| r9 | snapshot ≠ 锁；ACL guard 需 FOR SHARE；跨事务字段 drift 需 FOR UPDATE | row-level lock granularity / cross-tx race |
| r10 | nullable concept 跨接口传递 / 跨连接 ack 不可作 authoritative | nullability transitivity / cross-channel ack discipline |

### 元教训（给未来 Claude 做契约 freeze 类 story 的 checklist）

> **协议契约的 ACL / race / cross-channel ack / 字段 nullability / 跨章节枚举 / prose-mermaid zip 等横向 invariant 必须在 freeze 之前一次性扫齐，否则会撞 N 轮文档迭代。**

未来 Claude 做"接口契约 freeze 类 story"时，**在第一次产出 spec 之前**就应该做以下横向扫描（这是 r1-r10 累积出来的 checklist；每一项都对应至少一个上面已记录的 lesson）：

1. **schema completeness** ——任何"事件后客户端会用到的展示字段"必须在事件 payload 里自包含，不能依赖事件后客户端自己再发 HTTP 拉取（除非显式钦定）
2. **跨资源 cleanup** —— HTTP 操作如果会改变 WS / Redis presence / 任何辅助资源，必须显式钦定 cleanup 时机 + cleanup 顺序 + cleanup 失败语义（fire-and-forget vs 必须成功）
3. **持久层 vs ephemeral 层职责分离** —— 不要把 WS 连接生命周期事件和持久层成员关系事件混淆
4. **并发 race 错误码 mapping** —— 任何并发场景必须穷举 winner/loser timeline，输家必须落到精确的业务错误码（不能 fallback 到通用 1009 服务繁忙）
5. **跨章节枚举对齐** —— 同样的"1/2/3 三态"如果出现在不同表（如 `pets.current_state` 1=rest 2=walk 3=run vs `motion_states.motion_state` 1=stationary 2=walking 3=running），**禁止**复用枚举命名假设它们是同义；必须显式声明"虽然都是 1/2/3 但语义不同，绑死哪一个枚举的 source of truth"
6. **prose vs JSON 示例 zip** —— 字段表里写"必填" / "可选" / "节点 X 阶段固定 Y" 等约束时，对应的 JSON 示例必须**逐字段**反映；字段表改了就同步刷 JSON 示例
7. **prose vs mermaid sequenceDiagram zip** —— prose 里写"步骤 1 → 2 → 3" 改了顺序，对应的 mermaid 必须 zip 同步；mermaid 里钦定"严格顺序：A → B → C"必须与 prose 写的顺序一致
8. **接口默认 deny + 显式 allow（白名单 ACL）** —— 任何接口都先假设"任意认证用户可以访问 = 攻击面"，再显式列白名单（如"caller 必须是该房间成员才能查 roster"）；不是默认开放
9. **ACL 校验 + 受 ACL 保护的数据返回必须共享同一事务 snapshot** —— 跨步骤 ACL 校验 + 数据返回必须包在一个 snapshot 隔离的事务里，避免 ACL 通过后数据变更让响应泄漏权限外数据
10. **snapshot 隔离 ≠ 锁** —— REPEATABLE READ snapshot 仅保证"事务内多次读看到同一时刻"（内部一致性），**不**阻止其他事务在期间提交对相同行的写入（外部一致性）；ACL guard 必须用 FOR SHARE 锁明确阻止并发 leaving 事务，跨事务状态字段 drift 必须用 FOR UPDATE 串行化
11. **字段可空性传递性** —— 任何 schema 字段在某接口里 nullable，**所有**其他接口引用同一概念的字段也必须 nullable；JOIN SQL 涉及 nullable 概念必须 LEFT JOIN 而非 INNER JOIN
12. **跨连接 ack 永远 best-effort** —— 协议设计里"事件 X 完成的 ack / confirmation"必须先判别走的是同一连接还是跨连接；跨连接 ack（如 HTTP 200 + 另一条 WS close）永远 best-effort，**禁止** framing 为 authoritative confirmation
13. **client merge contract 区分 null vs 空字符串 vs 字段缺席** —— 三种"不下发"形态语义不同（null = authoritative 明确无值；空串 = server 不知道；缺席 = 字段未启用），文档必须给三种独立处理路径
14. **跨文档同步** —— 改一个契约维度（如 close code 表）必须 grep 所有引用这个维度的 markdown 文件（V1 接口设计 / 时序图设计 / 数据库设计 / story file / planning artifact）逐文件 zip 同步

**meta-meta 建议**：在第一次写接口契约 schema 之前做一次"横向 invariant 扫描"，按上述 14 条 checklist 逐条 dry-run 一遍当前 schema，提前 flag 自己可能违反的项 —— 比每轮被 codex 反复抓更省 review 配额（5 轮 → 10 轮的 cap 提升说明这种 case 在文档类 story 已经成为常态）。
