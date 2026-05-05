---
date: 2026-05-06
source_review: codex review (epic-loop r7, /tmp/epic-loop-review-10-1-r7.md)
story: 10-1-接口契约最终化
commit: <pending>
lesson_count: 3
---

# Review Lessons — 2026-05-06 — 握手 first-snapshot 契约的"启动时序原子性"+ placeholder 必须给真实可用最小值 + 跨文档时序图必须随冻结声明同步

## 背景

Story 10-1（V1 协议契约节点 4 冻结）codex review 第 7 轮。前 6 轮已经稳定了 close code 段位、信封字段、authz 用持久化数据、snapshot 的 full roster 单一视图等结构。r7 codex 在已有冻结结构内部做"协议自洽性"扫荡，抓出三处：

1. `room.snapshot` 节点 4 placeholder 例子写"全零"（`memberCount: 0` + `members: []`），与 §12.1 握手成功**充分条件**（当前用户已在 `room_members` 表）+ §12.3 不变量"snapshot 是房间 full roster 视图"+ client 侧推荐房间进入流程"先 GET 房间状态再开 WS"三处契约同时撞车 —— client 把 snapshot 作为权威态采用时会清空已加载的合法 roster，房间页错误渲染为空
2. 握手成功后启动顺序 (1) Session → (2) SessionManager → (3) **启动读/写 goroutine** → (4) 推 snapshot → (5) presence —— 第 (3) 步先于 (4) 启动读 goroutine 后，client 可以在 server 推 snapshot 之前发 ping，server 写 goroutine 已经活了 → 回 pong 在 snapshot 之前到达 client，违反 §12.1.3 钦定的"first must be snapshot"契约
3. 本 story 在 §12.1.3 / §12.3 末尾"业务消息延后锚定"块明确"节点 4 阶段服务端**不**广播 `member.joined`"，但本段引用的 `docs/宠物互动App_时序图与核心业务流程设计.md §13.2` 时序图末尾仍写"WSGateway-->>房间其他在线成员: 推送 member.joined" —— 跨文档 drift，按图实装的 dev 会写出与本 story 冻结声明冲突的代码

r7 是 10 轮上限的第 7 轮，还剩 3 轮。本轮目标：把"协议契约文档冻结期的内部一致性"再扫一层（启动时序原子性 + placeholder 真实可用 + 跨文档同步）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | room.snapshot placeholder 写"全零"与 §12.1 握手充分条件 + client 推荐流程冲突 — 必须给"自己单成员"真实可用最小快照 | high (P1) | architecture / docs | fix | `docs/宠物互动App_V1接口设计.md:1485-1486` `:1532-1550` |
| 2 | 握手后启动顺序：read/write goroutine 先于 snapshot 启动 — 必须把 snapshot 写入放在 goroutine 启动**之前**的同步段 | high (P1) | architecture / docs | fix | `docs/宠物互动App_V1接口设计.md:1353-1359` |
| 3 | §13.2 时序图末尾仍写"推送 member.joined"与本 story 节点 4 冻结声明冲突 — 选修法 (a) 同步时序图 + 加业务消息延后锚定注解 | medium (P2) | docs | fix | `docs/宠物互动App_时序图与核心业务流程设计.md:518-538` |

修了 3 条 / defer 0 条 / wontfix 0 条。

## Lesson 1: protocol-frozen placeholder 必须给"真实可用最小值"，**禁止**写"全零占位符"

- **Severity**: high (P1)
- **Category**: architecture / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1485-1486` `:1532-1550`

### 症状（Symptom）

`room.snapshot` 节点 4 阶段 placeholder 例子写：

```json
{
  "room": { "id": "3001", "maxMembers": 4, "memberCount": 0 },
  "members": []
}
```

字段表也写"节点 4 阶段 placeholder 实现固定返回 0 / []"。

但 §12.1 握手成功流程钦定：握手成功 ⇔ §12.1 第 5 步 `room_members WHERE user_id=? AND room_id=?` 命中。也就是握手成功的**充分条件**就是当前用户已在房间合法成员表里 → 房间里**至少**有一个合法成员（当前 caller 自己）。同时 client 侧推荐房间进入流程是"先 `GET /api/v1/rooms/{roomId}` 加载房间状态，再开 WS"，client 在收 snapshot 之前已经持有一个合法 roster 视图。再加上 §12.1.3 钦定 "snapshot 是握手后**必发**的 authoritative 消息"，client 把 snapshot 作为权威态采用 → 清空已加载的合法 roster，房间页错误渲染为空。

3 处契约（"握手成功 ⇒ 至少一个合法成员"+"client 已加载 roster"+"snapshot 权威覆盖"）任意两两组合就把"全零 placeholder"卡死。

### 根因（Root cause）

把"placeholder"误解为"全空 stub" —— 因为 placeholder 在节点 4 阶段不需要做"真实业务聚合"（多表 JOIN 由 Story 11.7 接管），就直觉上写"返回最简单的全零结构"。错的层在于：placeholder 仍然是**协议层的 authoritative 消息**，client 拿它做权威态，必须给出真实可用的最小快照（用握手当下已经查到的数据），**不**是"占位符 = 给一个 schema 合法但语义空的值"。

具体到 `room.snapshot`：placeholder 阶段的"最小可行"= "至少枚举当前 caller 自己"（因为这个数据已经在 §12.1 第 5 步查 `room_members` 时拿到了，placeholder 实装路径里**复用**这个查询结果即可，不需要额外 SQL）；其他离线成员的字段聚合（多表 JOIN）才是真正可以延后到 Story 11.7 的部分。

### 修复（Fix）

`docs/宠物互动App_V1接口设计.md`:

1. §12.3 字段表 `memberCount` / `members` 行，把"placeholder = 0 / []"改成"placeholder = 当前握手用户自身的单成员快照"
2. §12.3 末尾 placeholder 示例 JSON 改成 `memberCount: 1` + `members: [{当前用户}]`，nickname 来自 §12.1 第 5 步 JOIN `users`，pet 来自当前用户的 `pets.id`，currentState 节点 4 阶段固定 1
3. 不变量小节里"两者均为 0"那段改写为"至少包含当前握手用户自身条目（memberCount: 1，members: [当前用户]）"+ 解释为何禁止"全零 placeholder"
4. §12.1 握手成功流程第 1 条对 placeholder 的描述同步改为"members 数组**至少**含当前握手用户自身条目"
5. 加一段 placeholder 字段值来源说明：复用 §12.1 第 5 步已查的当前用户行，**禁止**额外查询其它成员（避免 placeholder 阶段过度耦合 Story 11.7 的多表 JOIN）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在"协议契约文档冻结期为某 authoritative 消息写 placeholder 示例"时，**禁止**写"schema 合法但语义空"的全零 / 空数组占位符 —— 必须用"握手 / 请求当下已经查到的数据"构造**真实可用的最小快照**。
>
> **展开**：
> - placeholder ≠ stub。Placeholder 仍是协议层 authoritative 消息，client 拿它做权威态会用它覆盖已加载状态
> - 写 placeholder 前先扫一遍：当前消息的"必须存在"语义（如握手成功 ⇒ 至少一个合法成员）+ client 推荐调用顺序里"在收本消息之前已经做了什么"+ "本消息是不是 authoritative" —— 任意一个被破坏，就是错误的 placeholder
> - 实装路径上要识别"placeholder 阶段已经天然能拿到的数据"vs"需要额外多表聚合才能拿到的数据"。前者必须放进 placeholder（成本零），后者才是真正可以延后的部分
> - 多表 JOIN / 复杂聚合可以延后；"复用握手 / 请求自身已经查到的字段"**不能**延后
> - **反例 1**：`room.snapshot` placeholder 写 `members: []` —— 握手成功时当前用户必在 `room_members` 表里，第 5 步授权校验已经查过，复用即可
> - **反例 2**：`pet.snapshot` placeholder 写 `petId: ""` —— 当前用户的 `pets.id` 在登录时就已经查到了，复用即可
> - **反例 3**："这个字段后面 epic 才真实驱动" → 直接置 null/空 —— 应该置该字段在节点阶段的**默认合法值**（如 `currentState: 1` = stationary_or_unknown），让 client 解析路径稳定

## Lesson 2: 握手后"必发第一条消息"契约要求"启动消息处理 goroutine **之前**完成同步推送"

- **Severity**: high (P1)
- **Category**: architecture / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1353-1359`

### 症状（Symptom）

§12.1 校验通过后启动顺序原写：

```
1. 创建 Session
2. 注册到 SessionManager
3. 启动读 / 写 goroutine
4. 推送 room.snapshot
5. 在 Redis presence 记录在线
```

但 §12.1.3 钦定"`room.snapshot` 是握手成功后**第一条**消息"。在原顺序下：第 (3) 步启动读 goroutine 后，client 在 upgrade 完成的下一个 tick 即可发 `ping`（client 不知道 server 还没推过 snapshot）；server 写 goroutine 已经活了 → 直接回 `pong`；如果 SnapshotBuilder 走 DB 慢路径 / 锁等待，窗口会被放大；最终 client 收到 `pong` 在 `room.snapshot` 之前 → 违反"first must be snapshot"契约，client 解析层（节点 4 阶段已经按"first must be snapshot"实装）行为未定义。

### 根因（Root cause）

直觉上以为"启动顺序 = 把所有相关组件先初始化好，然后再做业务推送"，所以把 goroutine 启动和 snapshot 推送当成两件可以独立列序的事。但底层 race 模型决定了：一旦读 goroutine 启动，client 的合法消息就可以在任何时刻进入；写 goroutine 启动后，对 client 消息的响应 frame 就可以在任何时刻流出。这两个动作**之间**不存在"server 一定先推 snapshot 才会回应 client 消息"的隐式 happens-before。

正确的物理保证是：把 snapshot 写入放在 goroutine 启动**之前**的同步段执行 —— 此时写 goroutine 不存在，物理上不可能产生竞争性写入；snapshot 写完才启动 goroutine。这才能保证 wire 上 snapshot frame 在任何 client 触发的响应 frame 之前。

### 修复（Fix）

`docs/宠物互动App_V1接口设计.md` §12.1 校验通过后顺序改为：

```
1. 创建 Session
2. 注册到 SessionManager
3. 同步构建并发送 room.snapshot —— 必须在握手响应路径里同步写入 underlying *websocket.Conn（或 enqueue 到 send buffer 后 flush 完成）；不通过"投递到尚未启动的写 goroutine 队列"完成
4. 启动读 / 写 goroutine —— 必须在第 3 步 snapshot 写入完成（write call 返回 nil error）之后才启动；snapshot 写失败时不启动 goroutine，按 1011 close
5. 在 Redis presence 记录在线
```

并加一段 rationale：解释为何必须这个顺序、若先启动读 goroutine 会出什么 race。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在"为协议契约写'握手成功后必发第一条 X 消息'语义"时，**必须**把 X 消息的发送放在"启动消息处理 goroutine **之前**"的同步段，**禁止**让"goroutine 启动"和"X 消息发送"成为两个可以乱序的步骤。
>
> **展开**：
> - "first must be X"契约不能只在文档里写一句，必须在实装顺序里物理保证 —— 即写 goroutine 启动之前，X frame 已经在 wire 上
> - 同步段写入 = 直接调 conn.WriteMessage（或 enqueue 到 buffer 后立即 flush），**不**用"投递到 channel 等 goroutine 异步消费"，因为此时 goroutine 还没存在
> - 若 X 构建失败，**不**启动 goroutine，**不**推 error 消息（client 会永等 X），统一走 close 路径
> - 这是一个"协议层的 happens-before"问题，不是"业务先后"问题。不要用业务直觉来排序，要用 race 模型来排序
> - **反例**：把启动顺序写成 "(1) Session → (2) Manager → (3) 启动 goroutine → (4) 推 snapshot" —— 第 (3) 步打开了 client 消息接收和响应的窗口，client 在 (4) 之前发的合法消息（如 ping）会被 server 在 snapshot 之前回应

## Lesson 3: 跨文档引用必须在冻结声明落地的同时同步，**禁止** drift

- **Severity**: medium (P2)
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_时序图与核心业务流程设计.md:518-538`

### 症状（Symptom）

V1 接口设计 §12.1.3 + §12.3 末尾业务消息延后锚定块明确"节点 4 阶段服务端**不**广播 `member.joined`"。但 §12.1.3 段落引用 `docs/宠物互动App_时序图与核心业务流程设计.md §13.2` 作为时序图权威来源，而 §13.2 时序图末尾仍写：

```
WSGateway-->>房间其他在线成员: 推送 member.joined
```

按图实装的 dev 会同时看到两份文档，得到两个相反结论。

### 根因（Root cause）

冻结声明（"节点 4 阶段服务端不广播 member.joined"）只在 V1 接口设计这一份文档里落地，没有同步检查所有引用本节的其它文档。跨文档引用一旦存在，本节内容变更必须 sweep 所有引用方。

### 修复（Fix）

选修法 (a)：

1. 修 `docs/宠物互动App_时序图与核心业务流程设计.md §13.2` 时序图，删除"WSGateway-->>房间其他在线成员: 推送 member.joined"这一行
2. 在 §13.2 时序图后追加一段"业务消息延后锚定"注解，与 V1 接口设计 §12.3 末尾业务消息延后锚定块同一文案 —— 节点 4 阶段服务端只会主动发 `room.snapshot` / `pong` / `error`；`member.joined` / `member.left` 由 Story 11.1 锚定；`emoji.received` 由 Story 17.1 锚定

修法 (b)（删除引用 + 改为"待 Story 11.1 / 11.7 真实实装期再绘"）也可行，但保留时序图的引用价值更大，故选 (a)。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在"为某协议章节落地冻结声明（'节点 X 阶段不发某消息'/'某字段废弃'/'某流程改顺序'）"时，**必须**同时 sweep 所有引用该章节的其它文档，把冻结声明同步到引用方；**禁止**只在主文档落地、留引用方 drift。
>
> **展开**：
> - 跨文档引用是一种隐式契约副本。本节变了，所有 cite 本节的地方都得验
> - 检查范围：(a) 主文档里出现 "见 / 参考 / 详见 OTHER_DOC §X" 的所有 OTHER_DOC §X；(b) 本节 / 本子章节关键术语在其它文档里的所有出现位置（如本案例的 `member.joined` / `room.snapshot`）
> - 如果引用方文档里的内容不再需要随本节同步（如时序图过于详细、本节只想保留 high-level 引用），改为"删除引用 / 改成 stub 注释"，**不**留 stale cross-ref
> - 在引用方加上"业务消息延后锚定"等同步注解，文案应与主文档**逐字一致**，让未来读者一眼看出"这两段是同一个冻结声明的两份副本"
> - **反例 1**：V1 接口设计 §12.1 改了"不发 member.joined"，但时序图设计 §13.2 仍画"推送 member.joined" —— 两份文档同时被 dev 读到时给出相反指令
> - **反例 2**：删了主文档里的某 enum 值，但 schema 文档 / 数据库设计 / iOS 客户端工程结构里仍写该值
> - **反例 3**：改了某 API 的字段名，cross-doc 副本（如 V1 接口设计 / 时序图 / Go 项目结构 / iOS 工程结构）只改了其中一处

---

## Meta: 本次 review 的宏观教训

r7 三条 finding 都属于**冻结声明落地后的"边界打磨"**：协议大结构早就定下来了，但每个"冻结条款"都会往周边泛起涟漪 —— 既要往**内**保证"自洽"（lesson 1：placeholder 不能自相矛盾；lesson 2：契约的"必发第一条"在物理实装顺序里要被保证），又要往**外**保证"同步"（lesson 3：所有引用本节的其它文档都得跟上）。

这三条 lesson 共同指向一个 sweep 模式：**冻结声明 = 主文档落地 + 内部自洽扫描 + 引用方同步扫描 + 实装顺序物理保证扫描**，四者缺一不可。前 6 轮都修了内部自洽相关的，r7 把"实装顺序物理保证"和"跨文档同步"两条新维度补上了。下一轮 review 之前，应当把**所有**冻结声明（V1 接口设计 §1 节点 4 协议骨架冻结声明、§12.1.3 first-message 契约、§12.3 全成员 roster 视图、§3 错误码段位）跑一遍这四维 sweep。
