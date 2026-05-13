---
date: 2026-05-13
source_review: codex review output (/tmp/epic-loop-review-17-1-r1.md)
story: 17-1-接口契约最终化
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-13 — room-scoped WS 消息必须以 `Session.roomID` 而非 `users.current_room_id` 做权威校验

## 背景

Story 17.1 在 `docs/宠物互动App_V1接口设计.md` §12.2 `### 发送表情` 锚定了 `emoji.send`（client → server WS 消息）的字段层契约 + 6 步服务端逻辑。codex r1 review 指出步骤 3"房间归属校验"仅写 `SELECT current_room_id FROM users WHERE id = ?` + 判 `!= NULL` 即放行，对一条**已经从 `GET /ws/rooms/{roomId}` 路径建立的 WS Session** 来说不足以决定本条消息合法的广播目标，因此存在 stale-Session 跨房间消息注入风险（用户 A 在房间 X 持 stale Session，HTTP join 房间 Y 后 `users.current_room_id` = Y，A 仍能在房间 X 的 socket 上发 `emoji.send` 然后被广播到房间 Y）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `emoji.send` 房间归属校验仅判 `current_room_id != NULL`，未与 WS Session.roomID 比对 → stale-Session 跨房间注入风险 | high | security / architecture | fix | `docs/宠物互动App_V1接口设计.md` §12.2 步骤 3（line 2014-2016）+ Story 17.1 文件 |

## Lesson 1: room-scoped WS 消息必须以 `Session.roomID`（握手 path roomId）为权威源校验广播目标

- **Severity**: high
- **Category**: security / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §12.2 `### 发送表情` 服务端逻辑步骤 3 / 步骤 5 / 错误响应表 6004 行 / 关键约束"房间归属校验顺序"；同步覆盖 `_bmad-output/implementation-artifacts/17-1-接口契约最终化.md` 对应描述行（AC1 描述 / AC3 钦定的 6 步逻辑 / 步骤 6004 触发条件 / 跨章节冻结边界 line 389 不变量声明）

### 症状（Symptom）

§12.2 步骤 3 当前契约：

> `SELECT current_room_id FROM users WHERE id = ?`
> - `current_room_id == NULL` → 回 6004
> - `current_room_id != NULL` → 记录 currentRoomId 进入步骤 4
>
> 步骤 5 用 `BroadcastToRoom(currentRoomId, ...)` 广播。

这只校验"用户当前在某个房间"，不校验"用户当前所在房间 == 本条 `emoji.send` frame 来自的那条 WS Session 所属房间"。**广播目标也直接取自 `users.current_room_id`** 而非 WS Session 的 path roomId，于是发送路径与连接路径的房间归属是解耦的，stale Session 可以跨房间注入消息。

### 根因（Root cause）

把"房间归属"在不同协议路径（HTTP vs WS）的语义当成同一个：

- HTTP 房间路径（§10.4 join / §10.5 leave）以 **path `{roomId}`** 为权威源校验 ACL（HTTP 是无状态的 request-response 模型，每次请求都自带 path roomId，所以这是天然权威）
- WS 路径有"长连接 Session"概念：握手时 `GET /ws/rooms/{roomId}` 的 path roomId 被写入 `Session.roomID`（§9.1 Session 字段表），并已在 §12.1 校验顺序步骤 5 通过 `room_members` 校验放行；这条 Session 的整个生命周期都"属于"该 roomId
- 但 `users.current_room_id` 是**可在 WS 连接生命周期内被独立 HTTP 路径修改的**（HTTP leave 清 NULL → HTTP join 别的房间），它与某条特定 WS Session 的归属是**两套独立时间线**

只在 WS 业务 handler 里查 `users.current_room_id` 而忽略 `Session.roomID`，等于让"connection-scoped 房间归属"被"user-scoped 房间归属"覆盖掉，丧失了 WS path roomId 在握手时已经建立的 invariant。

更深层的思维漏洞：**写 WS room-scoped 消息 handler 时，没有把"本条 frame 来自哪条 Session、Session 携带哪个 roomID"作为 first-class 输入**。如果把 `Session` 当成无状态匿名 frame source（只用 user.id），就会反复重新查 user 全局态去推断房间归属，而忽略 frame 本身已经携带的 connection 上下文。

### 修复（Fix）

§12.2 `### 发送表情` 服务端逻辑步骤 3 + 步骤 5 + 错误响应表 + 关键约束四处同步修改（V1 spec + Story 17.1 文件）：

- **步骤 3** 改为：查 `users.current_room_id` 后**必须**与 `Session.roomID` 比对；新增 race 来源说明（4007 best-effort cleanup 失败 / 多设备跨标签 / client 实装 bug），三类都报 6004 + log warn
- **步骤 5** 改为：`BroadcastToRoom(Session.roomID, ...)`（不再写 `BroadcastToRoom(currentRoomId, ...)`），显式以 `Session.roomID` 为广播目标参数
- **错误响应表 6004 行** 扩成 (a) NULL + (b) 不匹配两类合并报同一错误码（避免暴露具体房间归属）
- **关键约束"房间归属校验顺序"** 升级为"房间归属校验顺序 + 权威源"，明确钦定 `Session.roomID` 为权威源、与 §10.4 / §10.5 HTTP ACL 用 path roomId 校验语义对齐
- **Story 17.1 描述同步**：AC1 Story 17.5 钦定段、AC4 emoji.received 触发段、AC4 关键约束 (c) race 描述、line 389 冻结边界 6004 不变量声明四处同步更新

before / after 简化片段（V1 spec §12.2 步骤 3）：

```diff
-3. **房间归属校验**：SELECT current_room_id FROM users WHERE id = ?
-   - current_room_id == NULL → 6004
-   - current_room_id != NULL → 记录 currentRoomId 进入步骤 4
+3. **房间归属校验**（权威源 = Session.roomID，握手 path 写入）：SELECT current_room_id FROM users WHERE id = ?
+   - current_room_id == NULL → 6004
+   - current_room_id != NULL 但 current_room_id != Session.roomID → 6004 + log warn（stale Session 跨房间企图）
+   - current_room_id == Session.roomID → 进入步骤 4
```

```diff
-5. **广播**：BroadcastToRoom(currentRoomId, ...)
+5. **广播**：BroadcastToRoom(Session.roomID, ...)
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **锚定 / 实装任何"通过 `GET /ws/rooms/{roomId}` 路径建立的 WS 连接上、由 client → server 发起的 room-scoped 业务消息"（含但不限于 `emoji.send` 类未来扩展）** 时，**必须** 把 `Session.roomID`（握手 path 写入的连接所属房间）作为**房间归属校验权威源 + 广播目标参数**，并**禁止** 仅以 `users.current_room_id != NULL` 作放行条件。
>
> **展开**：
> - **校验范式**：先查 `users.current_room_id`，**必须**与 `Session.roomID` 严格 `==` 比对；NULL / 不匹配两类合并报同一错误码（业务层 6004 类语义），不暴露具体房间归属（避免 information leak）
> - **广播目标**：`BroadcastToRoom(...)` 的第一个参数固定取 `Session.roomID`，不取 `users.current_room_id`（即使二者刚被校验相等，也要从 Session 取 —— 让广播路径与 connection scope 同源，避免下次需要扩展校验时漏点）
> - **Session 在 handler 是 first-class 输入**：写 room-scoped WS handler 时，把 "本条 frame 来自的 Session 对象（含 `userID` / `roomID`）" 作为必传输入，**不**以"只拿 user.id 然后查全局 user 态"的范式实装；如果框架层不暴露 Session 上下文，先在 §9.1 / dispatcher 接口锚定补出来再写业务 handler
> - **与 HTTP ACL 对齐**：HTTP 房间接口 `/api/v1/rooms/{roomId}/*` 已经以 path roomId 为 ACL 权威源（§10.4 / §10.5），WS 路径必须保持同语义对齐，**禁止** 出现"HTTP 严格按 path 校验、WS 走 user 全局态"的双标
> - **stale-Session 来源场景登记**：契约层应在校验步骤注脚里枚举至少三类 stale-Session 来源 —— (a) `POST /rooms/{roomId}/leave` 步骤 7 close 4007 是 best-effort cleanup（4007 frame 写失败 / client 没读到时 Session 残留）；(b) 多设备 / 跨标签：设备 1 持房间 X Session + 设备 2 HTTP join 房间 Y 改 `users.current_room_id`；(c) client 实装 bug 持 stale socket 继续发送。三类都不可被"4007 / 心跳超时清 ephemeral"完美兜住，**必须** 由协议层校验封堵
> - **反例 1**：在 `emoji.send` / 任何未来 room-scoped WS 业务消息 handler 里写 `SELECT current_room_id FROM users WHERE id = ?` 然后判 `!= NULL` 就放行，并直接 `BroadcastToRoom(currentRoomId, ...)` —— **错**，stale-Session 跨房间注入风险无法封堵
> - **反例 2**：在 contract 文档里把 6004 触发条件冻结成"走 `users.current_room_id != NULL` 查询"这一抽象层不变量 —— **错**，不变量本身就丢失了 connection scope，应该冻结为"走 `users.current_room_id == Session.roomID` 校验"
> - **反例 3**：在错误响应表把"NULL"和"不匹配"分成两个不同 error code（如 6004 + 新增 6005）暴露给 client —— **错**，server 应避免暴露具体房间归属 / `users.current_room_id` 的存在性给跨房间的 stale-Session 发起者，两类合并报同一 6004 + server 端 log warn 区分

## Meta: 本次 review 的宏观教训（可选）

WS room-scoped 业务消息 handler 写校验时，**契约层就要把 connection scope 表达清楚**。仅在实装层"实际取 Session.roomID 来用"是不够的（实装可能后期重构换人写时丢失），必须在 V1 接口设计文档 §12.x 服务端逻辑步骤里**显式钦定** "权威源 = Session.roomID"，且把"为什么不能仅判 `users.current_room_id != NULL`"的 race 说明留在 contract 注脚里，让未来读 spec 的 Claude / 人都看到完整不变量。
