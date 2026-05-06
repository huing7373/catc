---
date: 2026-05-06
source_review: codex review of Story 10.1 r10 (epic-loop final round, no further verification)
story: 10-1-接口契约最终化
commit: 65b98f2
lesson_count: 2
---

# Review Lessons — 2026-05-06 — `room.snapshot` authoritative-but-non-destructive merge 契约 + WS roomId 来源按场景分两路（r10 收官）

## 背景

Story 10.1 第 10 轮（也是 epic-loop 10 轮上限的**最后一轮** — 修不彻底后续无验证机会）codex review 发现 V1 协议文档中两条**契约级**遗留问题：(1) §12.3 `room.snapshot` 钦定为握手后**必发**的第一条 authoritative 消息，但 placeholder 阶段允许 `nickname` / `pet.petId` 字段下发空字符串 — 与 §15.6 推荐的"先 `GET /api/v1/rooms/{roomId}` 再开 WS"流程互斥（client 已加载真实丰富字段，snapshot 一来就被 placeholder 空串擦掉，每次重连退化一次）。(2) §12.1 WS URL 段说 client 从 `GET /home.room.currentRoomId` 拿 roomId，但 §4.3 钦定该字段是**永久 null** schema 占位（节点 4 阶段甚至 Story 11.10 都不真正可用），Epic 12 reconnect 场景（app 冷启 / token 刷新）拿不到 roomId。两条都属"文档前后矛盾"型陷阱，落到 implement 层会变 client 真 bug。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `room.snapshot` "authoritative" 与 "placeholder 字段降级" 互斥 → 必须给 client merge contract | high | architecture / docs | fix (Option C) | `docs/宠物互动App_V1接口设计.md:1308 / 1487-1499 / 1577+` |
| 2 | WS URL roomId 来源指向永久 null 字段 → Epic 12 reconnect 拿不到 | medium | docs | fix | `docs/宠物互动App_V1接口设计.md:1286` |

修了 2 条 / defer 0 条 / wontfix 0 条。

## Lesson 1: snapshot 是"authoritative but non-destructive"，必须给 client field-level merge contract

- **Severity**: high
- **Category**: architecture / docs
- **分诊**: fix（Option C — merge semantics）
- **位置**: `docs/宠物互动App_V1接口设计.md` §12.1 握手成功流程 + §12.3 `room.snapshot` 字段表 / placeholder 示例 / 不变量段

### 症状（Symptom）

§12.3 钦定 `room.snapshot` 是握手后服务端**必发**的第一条 authoritative 消息（client 把它作为权威态采用），但 placeholder 阶段（Story 10.7）允许 `payload.members[].nickname` / `payload.members[].pet.petId` 下发**空字符串**，且 `avatarUrl` 等 future fields 节点 4 阶段不下发。结合 §15.6 推荐的"先 `GET /api/v1/rooms/{roomId}` 再开 WS"流程：client 在建连前已加载真实昵称 / petId / avatarUrl，然后第一帧 `room.snapshot` 一到，按"authoritative = 直接覆盖" 的朴素理解，client 会把 placeholder 空串赋给视图模型 → 每次重连都退化一次为"空昵称 + 空 petId + 无头像"。

### 根因（Root cause）

把"authoritative"理解为"破坏性覆盖"（destructive replace）。真正的契约语义应该是 **enrich/correct**：

- snapshot 提供**最新已知**的房间状态视图；
- 但"server 不知道某个字段的真实值"是合法的 placeholder 阶段中间态；
- 这种"不知道"的语义必须用**显式信号**表达（空字符串 / 字段缺席），client 必须能区分"server 说清空"和"server 不知道"两种语义；
- 在协议层把这条规则写死，让 placeholder 阶段（Story 10.7）和真实阶段（Story 11.7）行为一致 — server 实装层不需要为两个阶段写不同 client 代码，只需 client 始终按 merge contract 执行。

之前的 r1-r9 在 placeholder 数量层面（"member 条目要不要写死 1 / 0"）反复修，但**没有**把"authoritative"和"placeholder 字段降级"这两个语义在 client 视角统一调和；直到 r10 才显式落到契约层。

### 修复（Fix）

**选了 Option C — Merge Semantics**。理由：让 placeholder 阶段（Story 10.7）实装范围保持原貌（仅单表查询，不 JOIN `users` / `pets`），仅给 client 一条 field-level merge 规则；Option B（要求 placeholder 阶段强制 JOIN `users.nickname`）会把 Story 10.7 的"placeholder 单表 + 仅 pet 真实驱动延后"边界打破，且 pet.* 仍只能 placeholder 空值（Epic 14 才真实驱动），半边修对半边不修。Option C 是最小契约改动 + 最长效一致性保证。

落到 `docs/宠物互动App_V1接口设计.md` 的具体改动：

1. §12.1 握手成功流程末尾加一段**权威性 + Merge 语义**导言（line 1308 后），明示 `room.snapshot` 的权威性是 **enrich/correct** 而**非** wipe-out；
2. §12.3 字段表（`nickname` / `pet.petId` 两行）每行 placeholder 段末尾加 "空字符串 = 我不知道，client 保留已有值，禁止用空串覆盖" 提示；
3. §12.3 Future Fields 引用块（`avatarUrl` / `equips` / `renderConfig`）每条加"按 merge contract，未出现字段保留 client 已有值"提示；
4. §12.3 placeholder 示例 caption（"节点 4 阶段 placeholder 示例"段）加注 "下例 `nickname: ""` / `pet.petId: ""` 是 placeholder 信号，client 按 merge contract 保留真实值"；
5. §12.3 placeholder 字段值来源说明段后**新增** `Client merge contract` 块（roster 集合层 / 字段级 / 数值字段 / rationale / client 实装位置 五小节）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：在协议契约里钦定某条消息为 "authoritative" 时，**必须**同步钦定 client 的 merge 语义 — 是 destructive replace 还是 enrich/correct，不能让"权威"二字隐含一个未明说的 client 行为。
>
> **展开**：
> - "authoritative" 在分布式契约语境下有两类语义：(a) **destructive replace** — 收到该消息即整体替换 client 当前视图；(b) **enrich/correct** — 字段级 merge，非空覆盖 / 空保留 / 缺席保留。两者**不能默认其一**，必须写明。
> - 当存在"渐进 enrichment"语义（如 placeholder 阶段部分字段允许降级）时，自动落到 enrich/correct 一侧 — 否则 placeholder 必然破坏已加载真实数据。
> - **空字符串 ≠ null ≠ 字段缺席**，三者在 merge 语义下必须区分：
>   - 空字符串 `""`：约定为 "server 不知道这个值"的 placeholder 信号 → client **保留**已有真实值；
>   - `null`：约定为 "明确无值"（如 `currentRoomId: null` = 用户不在任何房间）→ client 应**清空**对应字段；
>   - 字段缺席：等价于空字符串语义（"未下发 = 不知道"）→ client **保留**已有真实值。
> - "成员存在性"（roster 集合层）和"成员字段"（字段级）要分开看：snapshot 在集合层是 authoritative（缺失的 userId 视为已离开），但在字段层不是（空字段不视为清空）。
> - **反例**：把 placeholder 阶段的"字段降级"语义留给客户端各自解读，没在协议层落"snapshot 是 enrich/correct" 的明文契约 → server 实装方以为"我下发空串 client 自己决定怎么处理"，client 实装方以为"authoritative 就是直接覆盖" → 上线后 roster UI 在每次重连退化为空状态。
> - **反例**：用 Option B（强制 placeholder 阶段 JOIN `users.nickname`）替代 merge contract，会让 placeholder 范围扩大 + pet.* 仍只能空值（Epic 14 才能真实），半截修复反而留下"为啥 nickname 真实而 pet.* 假"的诡异组合，长期看反而难维护。
> - **反例**：在 V1 文档新增"client merge contract"小节但忘记在字段表 / 示例 caption / Future Fields 三处都做交叉引用 → 实装方读字段表时仍然不知道空串特殊语义，照旧覆盖。Cross-link 在跨小节契约中是必须的。

## Lesson 2: WS roomId 来源必须按"首次连接 / 冷启 reconnect"分两路；不能引用"永久 null"字段

- **Severity**: medium
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1286` § WS URL 字段说明 `roomId`

### 症状（Symptom）

§12.1 WS URL 文档之前写 "client 从 `GET /home.room.currentRoomId` 拿 roomId"，但 §4.3 + Story 11.10 锚定明示 `home.room.currentRoomId` **节点 4 阶段返回 null**，要等 Story 11.10 真实实装。在 Epic 12（reconnect / 冷启 / token 刷新）场景下，client 拿不到 roomId — 协议契约对消费方（Epic 12 / iOS Story 12.5）实际不可用。

### 根因（Root cause）

把"未来某个 story 会真实实装"和"现在就可以引用"混淆了。文档化协议时如果把"节点 X 才生效"的字段当成"现在就可读"，等于在契约里埋下永久失效引用。**对每个引用，必须问：当下读它会得到什么？** —— 如果是 null / 空 / fall-through，必须给替代来源，不能让消费方踩坑。

另外，单一来源思维在多场景需求下不够用：WS roomId 的需求来自三类场景（首次创建房间 / 加入房间 / app 冷启重连），它们的"已知信息"截然不同，硬塞一个 server endpoint 必然漏其中一类。

### 修复（Fix）

§12.1 `roomId` 字段说明改为**按场景分两路 + 一条未来增强**：

1. **首次连接 / 刚完成房间动作（热路径）**：client 从本次会话内房间动作响应直接拿（`POST /rooms` 创建响应的 `data.room.id`、`POST /rooms/{roomId}/join` 加入响应的 `data.roomId`、用户输入分享链接 / 主动输入 roomId 等）— 建连前 client 内存中已持有 roomId；
2. **冷启 / token 刷新 reconnect（Epic 12 场景）**：client **本地持久化**最近一次成功 WS 连接的 roomId（iOS Story 12.5 实装：UserDefaults / Keychain；由"成功握手并接受 `room.snapshot`"事件触发写入；用户主动 leave / server close 4003 / 4004 时清除）；冷启读取本地，发起 WS 连接验证；
3. **未来增强**：节点 4 之后 Story 11.10 真实实装 `GET /home.room.currentRoomId` 后，client 可用 server 端权威来源**替代**本地持久化作为冷启场景来源 — 在此之前**禁止**依赖该字段（永久 null）。

明确禁止使用 `GET /me.user.currentRoomId`（永久 null schema 占位）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：协议契约引用任何字段 / endpoint 之前，**必须**先核对"当下读它会得到什么值" — 如果是 null / 空 / 未实装，必须给替代来源 + 标注该字段的真实激活节点。
>
> **展开**：
> - "永久 null schema 占位"是合法的设计（保留 schema 形状一致，但不计划回填） — 但任何外部小节引用它时必须避开；
> - "未来某 story 真实实装"是 fragile 引用 — 在 V1 文档里写"client 用它"会让消费方实装期间踩 null 坑，必须写"未来增强：节点 X 后由 Story Y 启用"，并给当下可用的替代；
> - 多场景需求（如 roomId 的三种获取场景）下，**单一 server endpoint 假设**通常不够 — 应识别 client 在每种场景下"已经持有什么信息"，按场景分路；
> - 本地持久化是合法的契约工具（如 last-known-good roomId），不是"协议外的脏 hack" — 但要明示**写入条件**（成功握手）和**清除条件**（leave / 4003 / 4004），否则会留 stale roomId；
> - **反例**：在 V1 文档某节钦定 client 从某 endpoint 拿值，但同一文档另一节钦定该 endpoint / 字段在当前节点返回 null —— 跨节自相矛盾，直接 leak 到 implement 阶段；
> - **反例**：发现 Story X 未来才真实实装某字段，就立刻在协议里写"client 用这个字段"，没有给当下的替代方案 → 任何在 Story X 之前的 epic 都无法实装；
> - **反例**：只给"首次连接"的 happy path roomId 来源（如刚 join 完），忽略 reconnect / 冷启 / token 刷新等需要 last-known roomId 的场景 → Epic 12 reconnect 场景拿不到，契约对该 epic 不可用。

---

## Meta: 本次 review 的宏观教训（r10 收官 sweep）

r1-r10 一直围绕 §12.x（WS 协议）的内部一致性反复打磨。从 close code 段位 / 业务消息冻结边界 / placeholder 字段语义 / 启动时序原子性 / 跨文档同步，到本轮的 **client merge contract** 与 **roomId 来源分场景**，每一轮都在逼近一个共同主题：

> **协议契约不是字段表的拼贴，而是消费方端到端可执行的语义闭环**。

每条消息 / 字段都要回答：**消费方在哪个场景拿到这个值，他下一步要怎么用？** 字段表只回答"server 下发什么"，但不回答"client 用它做什么"。"authoritative" / "placeholder" / "future fields" / "永久 null" 等修饰词在字段层下发了，但**消费侧 merge 语义 / 替代来源 / 激活节点**是必须配套的协议元信息 — 没有这些，字段表只是 server 视角的内部备忘录，对 client 实装不可用。

未来 Claude 检查协议文档完整性时的 checklist：
- 每条 authoritative 消息：明示 destructive replace 或 enrich/correct；
- 每个允许 placeholder 降级的字段：明示 placeholder 信号（空串 / null / 缺席）的 client 应对动作；
- 每个引用其他 endpoint / 字段的小节：核对引用点的当前真实值 / 激活节点；
- 多场景获取需求（同一信息的不同 client 场景）：识别每场景的已知信息，按场景分路而非单点；
- "未来增强"标注必须包含：(a) 真实激活的 story 锚点，(b) 当前替代方案，(c) 消费方何时可以切换。

本轮选择 Option C 而非 Option B 的关键判断 — **不要让"修当下"扩大到"重设计 placeholder 范围"**：当一条 finding 的根因是"两个语义未在协议层调和"，最干净的解法通常是**新增协议层规则**（merge contract）而非**收紧 placeholder 范围**（强制 JOIN）。Story 10.7 placeholder 范围已经是 r7 / r8 多轮打磨过的合理边界，不应为 client merge 问题反向扩大。
