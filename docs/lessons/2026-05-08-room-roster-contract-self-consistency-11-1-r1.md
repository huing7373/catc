---
date: 2026-05-08
source_review: codex review (epic-loop r1) — /tmp/epic-loop-review-11-1-r1.md
story: 11-1-接口契约最终化
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-08 — 房间 roster 契约自洽 / member.joined 必须自包含丰富字段（11-1 r1）

## 背景

Story 11.1 锚定节点 4 房间 5 个 REST 接口 + `member.joined` / `member.left` 两个 WS 业务消息字段层契约。dev-story 完成后 codex review 指出两条 P1 契约不自洽问题：(a) `member.joined` payload 仅含 `userId` / `nickname`，但同时让 client "等下次 `room.snapshot` 全量推送时 enrich"，而 §12.1.3 钦定 `room.snapshot` 仅握手时下发一次 —— 已连接成员永远拿不到新成员的 `avatarUrl` / `pet.*` 真实值；(b) §10.3 / §12.3 同时声明 roster "含离线成员"，但 Story 10.4 r6 lesson 钦定心跳超时清理钩子 = 真删 `room_members` 行 + 广播 `member.left` —— 这条路径产生不出"离线 member"，二者契约自相矛盾。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `member.joined` 缺失 avatarUrl/pet.* 导致已连接成员无法 enrich 真实数据 | P1 / high | architecture (contract) | fix | `docs/宠物互动App_V1接口设计.md` §12.3 `### 成员加入` |
| 2 | 心跳超时被动断线删除 room_members 行 vs "含离线成员" roster 契约自相矛盾 | P1 / high | architecture (contract) | fix | `docs/宠物互动App_V1接口设计.md` §10.3 / §12.3 |

## Lesson 1: `member.joined` 必须自包含成员展示所需的全部丰富字段

- **Severity**: P1 / high
- **Category**: architecture (contract)
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §12.3 `### 成员加入`（行 1925 附近，新增 `payload.avatarUrl` / `payload.pet.petId` / `payload.pet.currentState`）

### 症状（Symptom）

`member.joined` 字段表只锚定 `userId` + `nickname`，关键约束段说"client 收到 `member.joined` 后 ... 等下次 `room.snapshot` 全量推送时由 server 真实驱动 enrich"。但同 spec 中 §12.1.3 / §12.3 钦定 `room.snapshot` **仅在握手成功后必发一次**，server **不**对已连接成员追发 snapshot —— 已连接成员永远等不到一个"下次的 snapshot"，新加入成员的 `avatarUrl` / `pet.*` 真实值永远拿不到，roster 中该 user 永久停留在 placeholder 默认值（空串 / `pet.currentState=1`）。

实装层若严格按字段表实现，client 渲染层只能显示昵称 + 默认头像；要拿真实头像 / pet 必须**额外**走一次 `GET /api/v1/rooms/{roomId}` —— 这是契约层未声明的隐式依赖（每收到一次 `member.joined` 都额外打一次 HTTP 拉取，既增加 server 压力也增加端到端延迟）。

### 根因（Root cause）

- 把 `member.joined` 类比 `member.left`（仅 `userId`），认为"事件型 broadcast 只需身份 ID，丰富字段由 client 从 roster 查"，但 **`member.left` 的"client 已有 entry 仅需移除"语义** vs **`member.joined` 的"client 没有该 entry，需要新增"语义** 不对称：left 不需要新字段，joined 必须有完整渲染数据。
- 让"client merge contract 字段级 merge"机制成为兜底，期待"未来某次 snapshot 会补齐"—— 但 spec 同时钦定 snapshot 不追发；两个契约钦定撞车。
- 写文档时**仅看了 §12.3 内部一致性**（merge contract 自身能 work），没回头校验 §12.1.3 钦定的"snapshot 是握手专属消息"约束是否给 merge 留了 enrich 通道。

### 修复（Fix）

扩展 `member.joined.payload` 字段表，新增三个**必填**字段（与 §10.3 `data.members[]` schema 对齐，仅省略 Future Fields）：

- `payload.avatarUrl`（string, 必填，可空字符串）—— `users.avatar_url`
- `payload.pet.petId`（string, 必填，必非空字符串）—— 加入事务内已查询的活跃 `pets.id`
- `payload.pet.currentState`（number int, 必填）—— 节点 4 固定 `1`

同步更新关键约束段：移除"等下次 snapshot enrich"承诺，改写为 client 收到 `member.joined` 即可直接 append 一条**完整**的 roster entry，**不**需要二次 HTTP 拉取。Future Fields（`pet.equips` / `equips[].renderConfig`）与 §10.3 同节奏，节点 9 / 10 由对应 epic 同步扩展。

```diff
- | `payload.userId` | string | 必填 | ... |
- | `payload.nickname` | string | 必填 | ... |
- | `ts` | number (int64) | 必填 | ... |
+ | `payload.userId` | string | 必填 | ... |
+ | `payload.nickname` | string | 必填 | ... |
+ | `payload.avatarUrl` | string | 必填 | (可空字符串) ... |
+ | `payload.pet.petId` | string | 必填 | (必非空字符串) ... |
+ | `payload.pet.currentState` | number (int) | 必填 | 节点 4 阶段固定 1 ... |
+ | `ts` | number (int64) | 必填 | ... |
```

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在为"事件型广播消息"（如 `xxx.joined` / `xxx.created` / `xxx.added`）写字段表时，**必须**校验 client 收到该消息后**渲染所需的所有字段是否都在 payload 里**；**禁止**把"丰富字段由 client 从 roster 查 / 等下一次全量 snapshot"当兜底，**除非** spec 同时钦定了"会有下一次全量 snapshot 主动追发"。
>
> **展开**：
> - 写 `xxx.joined` / `xxx.created` 类**新增型**事件广播时，payload 字段集**应**与对应 REST 列表查询接口（如 `GET /rooms/{roomId}` 的 `data.members[]`）字段集对齐（仅可省略 Future Fields）—— 因为 client 收到 joined 时 roster 中**没有**该 entry，必须从 payload 自包含构造完整新 entry。
> - 写 `xxx.left` / `xxx.removed` / `xxx.deleted` 类**移除型**事件广播时，payload 仅需 `id` 类身份字段即可（client 已有 entry，仅按 ID 删除）—— 这种不对称是**有意设计**，不是"统一精简"的对象。
> - 写 broadcast / snapshot 类消息时，**主动**找 spec 中"该消息何时下发 / 是否会重复下发"的约束（如本案 §12.1.3 "snapshot 是握手专属"），用它校验你"等下次 X 推送会补齐"的兜底承诺是否成立 —— 不成立时立即调字段表，不依赖 merge contract 兜底。
> - **反例**：在 `member.joined` 字段表里只放 `userId` + `nickname`，关键约束段写"其他字段等下次 snapshot 推送 enrich" —— 同 spec 中 snapshot 是握手专属，永远不会有下次推送，client 永久停留在 placeholder。

## Lesson 2: roster 契约必须与心跳超时清理路径自洽 — "含离线成员" 在节点 4 阶段没有产生路径

- **Severity**: P1 / high
- **Category**: architecture (contract)
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §10.3（`data.members` 字段说明 + 不变量 + 新增 "roster 语义与被动断线交互"小节）+ §12.3（`payload.members` 字段说明 + 不变量段）

### 症状（Symptom）

§10.3 `data.members` 字段说明 + §10.3 不变量小节 + §12.3 `payload.members` 字段说明 + §12.3 snapshot 不变量段，**四处**都用"**含离线成员**"作为 roster 完整性的关键修饰，service 层"禁止做在线态过滤"被钉成不变量。但 Story 10.4 r6 lesson 钦定的心跳超时清理钩子（onUnregister）行为是：**真删** `room_members WHERE room_id = ? AND user_id = ?` 行 + 更新 `users.current_room_id = NULL` + 广播 `member.left`。两条契约对接后效果：transient WS disconnect（>60s 心跳超时窗口）→ user 被踢出 `room_members` → 后续 snapshot / GET 不再返回该 user → "含离线成员" 这一不变量在节点 4 阶段**根本没有产生路径**（产生不出留在 `room_members` 里但被认定为"离线"的 user）。

contract reader 读到此处会困惑：节点 4 阶段是否要新增 `members[].online: bool` 字段？是否要改 Story 10.4 不删行？epics.md / 本 spec 没给答案。

### 根因（Root cause）

- 写 §10.3 / §12.3 roster 契约时，把"server 不做 WS 在线过滤"（这条是真的：service 不应在 query 后再按 SessionManager 过滤一遍）误写成"含离线成员"（这条是错的：节点 4 阶段没有"离线 member"概念，row 在 = 在房间，row 没了 = 不在房间）。
- 没有把 §10.3 / §12.3 的 roster 语义与 Story 10.4 r6 lesson "心跳超时 = 真删 row + 广播 left" 的产物**端到端**走一遍 —— 单看本节内部一致性（memberCount = members.length）通过，但跨节点、跨 epic 的语义对接没校验。
- 把"server 端 row 状态" 与 "WS connection 状态" 两个概念**混用** —— 用"在线 / 离线"描述前者本身就是错的，那是 connection 层概念；row 层只有"在 / 不在房间"。

### 修复（Fix）

四处文字修订 + 新增一节"roster 语义与被动断线交互"：

- §10.3 `data.members` 字段说明：删除"**含离线成员**；不区分在线 / 离线"语义，改为 "= `room_members WHERE room_id = ?` 全行 + JOIN ...；不做'WS 此刻是否连接'层的过滤；roster 反映 server 端'仍在房间'的状态；节点 4 阶段不下发 online/offline 区分字段"。
- §10.3 不变量段：把 "service 实装层禁止做'在线态过滤'" 改为 "禁止做 'WS 此刻是否连接'层的过滤"；并 cross-ref 到新增的小节。
- §10.3 新增"roster 语义与被动断线交互"小节：钦定三条路径（① WS 当前连接但暂时停留 → row 在 → 仍在 roster；② 心跳超时 60s 清理钩子触发 → row 删 → 离开 roster + 广播 left；③ client 主动 close → 同路径②）+ 说明 memberCount / member.left 三方同步语义；末尾用 design rationale 块明示"节点 4 阶段没有'含离线成员'概念，transient 容错由 60s 心跳阈值实现而非'离线 member 占座'"。
- §12.3 `payload.members` 字段说明 + snapshot 不变量段：同步把"含离线成员"措辞替换为 "`room_members` 全行 = server 端仍在房间"，并 cross-ref 到 §10.3 新增小节。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在写跨节点 / 跨 epic 的状态契约（如 roster / session set / cache view）时，**必须**把"该状态如何被填充 / 如何被清理"的全部 producer 路径列出来，校验契约里写的"该状态包含什么"在每条 producer 路径下都成立；**禁止**让契约描述出现"产生不出来的成员"（如本案 "含离线成员" 在节点 4 没有清理路径之外的产物）。
>
> **展开**：
> - **状态层 vs 连接层概念严格区分**：DB 行 / Redis key / 缓存条目是"状态层"（"在 / 不在"语义）；WS connection / TCP socket / HTTP keep-alive 是"连接层"（"online / offline" 语义）。**禁止**用连接层词汇描述状态层（"含离线成员"应改为"`room_members` 全行"），状态层和连接层之间的同步路径应单独写一节，明示触发条件 + 副作用。
> - **跨节点契约对接校验**：写 §10.x / §12.x 这种 "REST + WS" 双侧契约时，**主动**找现有的 server-side 后台任务 / 钩子（如 Story 10.4 onUnregister / scanner / 定时清理）去查它们的 mutation 路径，把它们对状态的影响在新契约里明示；**不**只看本节内部 `memberCount = members.length` 之类自洽。
> - **写 "禁止做 X 过滤" 的不变量时，先问自己 X 是什么层的概念**：本案最初写 "禁止做'在线态过滤'"（连接层）—— service 实装层的"过滤"动作只能发生在状态层（query 后再按某条件 reject 行），不存在"按连接层过滤"这个动作。正确表述应是"禁止做'WS 此刻是否连接'层的过滤"或"禁止 query 后按 SessionManager 在线列表过滤"。
> - **反例**：在 roster 契约里写"含离线成员"，但 spec 同时钦定心跳超时 = 真删 row —— 节点 4 阶段没有"离线 member"产生路径，契约描述了一个产生不出来的状态；client / server 实装层会困惑，QA / Stage 测试会发现没有数据可以覆盖该不变量分支。

---

## Meta: 本次 review 的宏观教训

两条 finding 都是**跨节点 / 跨 sub-section 契约对接漏洞**（不是单节内部错误）。共同根因：写新 spec 章节时**只读本节上下文**，没有主动 cross-ref 已锚定的相邻契约（§12.1.3 snapshot 触发时机 / Story 10.4 r6 心跳超时清理路径）。Lesson 1 的"等下次 snapshot enrich"撞 §12.1.3 钦定；Lesson 2 的"含离线成员"撞 Story 10.4 r6 钦定。两者在 dev-story 阶段单测层（YAML schema / DTO unit test）都不会暴露，**只在跨节点契约 walkthrough 时才能发现**（codex review 是该机制的实现）。

未来写"契约最终化 / X.1 锚定" 类 story 时**应**在 dev-story workflow 加一道"跨节点契约 walkthrough"步骤：列出本 story 触及的所有 §X.Y 节点，对每节点找出其依赖的相邻已锚定节点（如 §12.3 依赖 §12.1.3 / Epic 10 r6 lesson），人工复述一遍依赖契约的钦定语义，校验本 story 新写的语义不与之冲突。
