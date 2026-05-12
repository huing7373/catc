---
date: 2026-05-12
source_review: file:/tmp/epic-loop-review-14-1-r2.md (codex review on Story 14.1 r2)
story: 14-1-接口契约最终化
commit: 97545b5
lesson_count: 3
---

# Review Lessons — 2026-05-12 — 跨章节字段等价声明必须锁定前置条件 & ack vs 权威等价的分层 & self-broadcast 丢失的 sender-side 兜底

## 背景

Story 14.1 r2（节点 5 接口契约最终化第 2 轮 codex review）在 r1 修复（state-sync 幂等、envelope ts）之上又抓出三条契约级一致性 finding：

1. **P1 跨章节"完全等价"声明 vs Story 14.3 前的临时不一致窗口**：line 593（§5.2 关键约束）+ line 2228（§12.3 `pet.state.changed` 关键约束）声明五处字段"语义完全等价"，但同文档 line 1369 / 1968 / 2073 仍保留"§10.3 / `room.snapshot` 在 Story 14.3 落地前固定返回 `1`"。在 Story 14.2 / 14.4 先于 14.3 实装的窗口里，`state-sync` / `pet.state.changed` 能广播真实 2/3，而 `room.snapshot` / GET 仍 placeholder `1`，契约自相矛盾。

2. **P2 §5.2 `data.state` ack-only 与"完全等价"语义混淆**：line 557 说 `data.state` 是 server-acknowledged ack，client 不应反推为 final state；line 543 / 592 把 WS 广播定义为 final-consistency 信号；但 line 593 又把 `data.state` 塞进"完全等价"桶。这是"值等价 ≠ 权威等价"的混淆：值域 / DB 来源相同（ack 回显本身就是入参原值）不等于 client 信任级别相同。

3. **P2 self-broadcast WS 丢失无 contract 级 recovery path**：line 543 / 2225 让 WS 广播对发起者自己也是"single source of truth"，line 557 说 HTTP `data.state` 不应驱动 final state，line 2230 让 self-broadcast fire-and-forget 不重试。组合结果：HTTP 200 OK + 发起者自己的 `pet.state.changed` 丢失 → 发起者本地 UI 永远 stale（直到下次状态切换 / WS 重连）。

三条都是"契约文档自身矛盾"类问题（节点 5 server 实装由 Story 14.2 / 14.3 / 14.4 才落地），但在 14.1 闭环前不修，会让后续实装在"按声明等价做、还是按 placeholder 固定 1 做"之间反复横跳；client 端 iOS Story 15.2 / 15.4 在 self-broadcast 丢失场景也无 contract 依据兜底。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 跨章节"完全等价"声明与 §10.3 / `room.snapshot` 在 Story 14.3 前固定 `1` 矛盾 | High (P1) | docs / architecture | fix | `docs/宠物互动App_V1接口设计.md` line 593 + 2228 |
| 2 | §5.2 `data.state` ack-only 与"完全等价"桶语义混淆 | Medium (P2) | docs | fix | `docs/宠物互动App_V1接口设计.md` line 557 + 593 |
| 3 | self-broadcast WS 丢失无 contract 级 recovery path | Medium (P2) | docs / architecture | fix | `docs/宠物互动App_V1接口设计.md` line 543 / 557 / 592 / 2225 / 2230 |

## Lesson 1: 跨章节字段"完全等价"声明必须显式锁定前置条件 + 临时不一致窗口必须文档化

- **Severity**: high (P1)
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §5.2 line 593 + §12.3 `pet.state.changed` line 2228

### 症状（Symptom）

§5.2 关键约束最末条与 §12.3 `pet.state.changed` 关键约束第 3 条都写"字段语义完全等价"，但同文档 §10.3 line 1369 / §12.3 `room.snapshot` line 1968 + line 2073 仍保留"节点 4 阶段（含 Story 11.7 真实实装）固定返回 `1`"。

在 epic 时序上，Story 14.2 / 14.4 实装顺序早于 Story 14.3（接 motion_state → pets.current_state 真实驱动）。该窗口内：

- `state-sync` 接口能写入 `2` / `3` 入库
- `pet.state.changed` WS 广播能发送 `2` / `3`
- 但 `room.snapshot` / `GET /rooms/{roomId}` 仍在该窗口固定回 `1`

新加入房间的 client 收到 `room.snapshot` `currentState: 1`，会把已通过 `pet.state.changed` 更新到 `2` 的本地 roster 反向覆盖回 `1`，破坏一致性。

### 根因（Root cause）

写"五处字段语义完全等价"时，把**值域 / DB 来源等价**（type / enum / 列映射相同）和**权威 / client 信任层等价**（client 是否可以无差别处理）混为一谈，并默认 Story 14.3 已经落地。

实际上节点 5 内部有"先实装能写真实值的 endpoint（14.2 / 14.4）→ 再切换聚合查询读真实值（14.3）"的时序差，等价声明必须明确这条前置条件。

### 修复（Fix）

把 line 593 的"五处字段语义完全等价"声明拆成**值域 / DB 来源层** + **权威 / client 信任层**两层 + 显式标注 Story 14.3 前置条件：

- 值域 / DB 来源等价：恒成立（五处 type 相同、enum 相同、都映射 `pets.current_state`）
- 权威 / client 信任层等价：**自 Story 14.3 起成立** + 仅对 server → client 三处（`pet.state.changed` / `room.snapshot` / §10.3 GET）
- §5.2 `data.state` 是 ack 信号，**不**进入权威等价桶（解 Lesson 2 同步处理）
- §5.2 请求体 `state` 是 client → server 单向写入，不参与 server → client 权威等价讨论
- **Story 14.3 落地前的临时不一致窗口**：显式记录 §10.3 / `room.snapshot` 仍固定 `1`，并定义 client 在该窗口内的权威信号优先级 `pet.state.changed` > `state-sync data.state`（仅 self ack）> `room.snapshot` / GET（在 14.3 前是 placeholder）

line 2228（§12.3 `pet.state.changed`）做同样的分层处理 + 同样的临时窗口声明，避免"三处 server → client 字段在 14.3 前一致性"的隐含假设。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"跨章节字段语义等价"类契约声明** 时，**必须** **(a) 区分"值域 / DB 来源等价"与"权威 / client 信任级别等价"两层，并在哪一层成立 (b) 明确锁定该等价声明的前置条件（特别是依赖未落地 story 的 epic 内时序差）(c) 把前置条件未满足的临时窗口显式写成"窗口内权威信号优先级"列表，让 client 实装层在该窗口内有明确依据**。
>
> **展开**：
> - 跨章节等价声明永远会被未来 Claude 当作"无条件 invariant"读 —— 必须**主动用 placeholder / 固定值 / future epic 落地** 等关键词去 grep 同文档其它章节，确认所有"未落地"的字段都被这条等价声明覆盖到，否则就是契约内部矛盾
> - 多 story 时序差（Story 14.2 / 14.4 早于 14.3）+ epic 内字段聚合（state-sync 写 / `room.snapshot` 读 / GET 读）一定会形成临时不一致窗口；契约声明必须显式承认这个窗口而**不是** 假装它不存在
> - "值等价"和"权威等价"是两个独立维度：ack 回显本身值 = 入参，DB 来源也确实是 `pets.current_state`（写入即是这个值），但 client 信任级别完全不同 —— 决不能把这两层等价混为一谈
> - **反例（本次踩坑）**：line 593 写"五处字段语义完全等价" + 同文档 line 1369 / 1968 / 2073 仍说"固定返回 1"；line 2228 三处等价又落实"自 Story 14.3 起" —— 让 Story 14.2 实装时无 contract 依据判断该不该把 state-sync `2` 入库（按等价声明 → §10.3 也该返 `2`；按 line 1369 → 仍返 `1`）
> - **正例**：等价声明拆两层 + Story 14.3 前置条件显式标记 + 临时窗口内权威信号优先级列出 → 实装层可机械判断"该走哪条路径"

## Lesson 2: ack-only 信号决不能与权威态信号塞进同一个"等价桶"

- **Severity**: medium (P2)
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §5.2 line 557 + line 593

### 症状（Symptom）

§5.2 line 557 明确把 `data.state` 定义为"server-acknowledged 入账确认；client 不应用此值作为 server 端最终态反推"；line 543 / 592 把 WS 广播定义为 final-consistency 信号。但 line 593 的"五处字段语义完全等价"声明又把 `data.state` 塞进与 `room.snapshot` / `pet.state.changed` 同等级别的等价桶。

这是契约内部对 `data.state` 信任级别的自相矛盾：line 557 让 client 不信，line 593 让 client 把它当与 WS 广播同级别使用。

### 根因（Root cause）

"等价" 二字在工程语境下至少有两层意思：(a) 值域 / 类型 / DB 来源相同（在 happy path 下值必定相同）(b) client 信任级别相同（client 实装层可以无差别处理）。

ack 回显设计的本意是给 client 提供一个简单的"我提的值入账成功"校验信号，并**不**承诺"这是 server 端真实查出来的状态" —— 即 (a) 成立、(b) 不成立。写等价声明时只关注 (a)（"反正值相同嘛"），把 (b) 也默认带上了。

### 修复（Fix）

- line 557 字段说明里补充 self-broadcast 例外（self-only 权威 ack 边界），同时强化"client 不应用此值反推**他人** server 端最终态"
- line 593 等价声明拆两层（见 Lesson 1）后，把 `data.state` 显式从"权威 / client 信任层等价桶"里**排除**，只承认它在"值域 / DB 来源层"等价
- 不删除 `data.state` 回显设计本身（line 572 设计权衡仍成立），只是把它在"等价桶"里的归属位置说清楚

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **同一份契约文档里写"字段语义等价"声明** 时，**必须** **先 grep 该文档对每个被列入"等价桶"的字段的独立信任级别定义（如某字段是否被声明为 ack-only / placeholder / authoritative），把信任级别不同的字段排除出"权威等价桶"，只保留值域 / DB 来源层等价**。
>
> **展开**：
> - 一旦在文档某处声明某字段是 ack-only / placeholder / fixed-value，再在另一处把它和真实权威态字段塞进"完全等价桶"就构成契约自相矛盾 —— client 实装层无法同时遵守两条
> - "值相同" 是 happy path 的运行时观察，**不**是契约层信任级别的承诺；契约层必须用"client 信任级别"维度判断字段是否能"等价"使用
> - 写"完全等价"前先 grep 这些字段名（特别是同名跨章节字段），看是否有 "ack" / "placeholder" / "fixed" / "回显" 等关键词出现 —— 任何一个字段有这类弱信任级别标签，就**不**能进入"权威等价桶"
> - **反例**：line 593 五字段等价桶塞了 `data.state`（line 557 已声明 ack-only）+ `room.snapshot.currentState`（line 2073 已声明节点 5 / Story 14.3 前固定 `1` placeholder）+ `pet.state.changed.payload.currentState`（真实权威态）—— 三种不同信任级别字段塞进同一桶
> - **正例**：等价桶按信任级别**分两层** —— 值域 / DB 来源层包所有五字段；权威 / client 信任层只包真实权威态三字段（`pet.state.changed` / `room.snapshot` / §10.3 GET），ack / placeholder 字段显式排除并解释为什么

## Lesson 3: self-broadcast 的"single source of truth"语义对发起者自己必须有 sender-side 兜底

- **Severity**: medium (P2)
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §5.2 line 543 / 557 / 592 + §12.3 line 2225 / 2230

### 症状（Symptom）

原契约设计：

- line 543 + line 2225：`pet.state.changed` WS 广播范围包含发起者自己，让发起者也走"统一 WS 路径"更新 roster
- line 557：HTTP `data.state` 是 ack-only，client 不应用其作为 final state 反推
- line 2230：广播 fire-and-forget，server 不重试

组合结果：发起者 A 调用 `state-sync(state=2)` 成功 200 OK → server BroadcastToRoom → A 的 WS Session 因抖动 / close / SessionManager 状态不一致 broadcast 失败 → A 永远收不到自己的 `pet.state.changed` → 按原契约 A 不能用 HTTP 200 + `data.state` 驱动自己的本地 roster pet state → A 自己的房间页 cat sprite 状态永久停留在切换前（rest）。

兜底路径只有：(a) A 下次再调一次 state-sync 触发新一轮广播 (b) A 的 WS 重连后 `room.snapshot` 全量重新下发 —— 但 (a) 要用户再次触发动作切换、(b) 要 WS 断重连，正常稳态运行下都不会发生，导致 stale 永久持续。

### 根因（Root cause）

写 "WS 是 single source of truth"原则时，套用 CLAUDE.md §"状态以 server 为准"做隐喻表述，**没区分**"对自己 vs 对别人"两条不同的 client 更新路径：

- 对别人的状态变化：HTTP（state-sync）是别人发的，自己根本收不到 → 必须靠 WS 广播驱动 → WS 真是 single source of truth ✓
- 对自己的状态变化：HTTP 200 已经是入账成功的强 ack，self-broadcast 只是"server 端入账后真实 broadcast 链路的活性探测信号"，并非独立的"再次入账确认" → 让自己的本地 UI 也只信 WS 广播是过度强约束 ✗

把"对别人"的合理路径机械套用到"对自己"上，没考虑到 self-broadcast 这条路径与 HTTP 200 的语义重叠（同一次写库行为的两路下发）+ self-broadcast 丢失场景。

### 修复（Fix）

按 review 推荐的 option (b)：给"发起者自己"开 self-broadcast 例外，让 HTTP 200 立即驱动本地 self entry，不等 self-broadcast；对"别人"仍以 WS 广播为唯一权威。

具体 diff（≤ 10 行级要点）：

- §5.2 line 543 区块整段重写为"WS 广播 vs HTTP 响应的关系（含发起者自己的 self-broadcast 兜底规则）"，明确拆"对别人"和"对自己"两条路径，self entry 的契约级 fallback 是 HTTP 200 立即驱动，self-broadcast 走 merge no-op
- §5.2 line 557 `data.state` 字段说明补充 self-broadcast 例外（self-only 权威 ack），同时强化"client 不应用此值反推**他人** server 端最终态"
- §5.2 line 592 "HTTP 200 vs WS 广播的端到端语义"关键约束条更新：对"别人"走 WS 唯一路径；对"自己"HTTP 200 → 立即更新 self roster；self-broadcast 到达走字段级 merge no-op
- §12.3 line 2225 `pet.state.changed` 广播范围说明里补充"发起者自己的 self-broadcast 不作 UI 唯一来源（HTTP 200 已驱动），仅作跨设备一致性 + WS 链路活性探测"
- §12.3 line 2230 fire-and-forget 段补充"对自己"的兜底是 §5.2 self-broadcast 兜底规则 —— 发起者本地 UI 不依赖 self-broadcast 到达

self-broadcast 与 HTTP 200 的语义自洽性论证：(a) HTTP 200 已是 server 端入账完成的强 ack；(b) self-broadcast 值与 HTTP 200 ack 回显值必定相同（同一次写库的两路下发，service 层在 UPDATE 后才触发 BroadcastToRoom）；(c) self-broadcast 到达时按 client merge contract 字段级 merge → 值已相同 → no-op，不触发状态闪烁。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计"WS 广播 + HTTP ack"双信号场景的 client 状态更新契约** 时，**必须** **区分"对自己（发起者自己的状态）"和"对别人（房间内其他成员的状态）"两条独立路径 —— 对别人 → WS 唯一权威；对自己 → HTTP 200 立即驱动 + self-broadcast 仅 ack 校验 / 链路探测，避免 self-broadcast 单路径 fire-and-forget 丢失导致发起者自己 UI 永久 stale**。
>
> **展开**：
> - "single source of truth" 是抽象原则，落到 client 状态更新具体路径上必须按"自己 vs 别人"拆两条
> - 对"自己"的状态变化：HTTP 200 已是入账成功强 ack，self-broadcast 是同一次写库行为的两路下发 —— 让自己的本地 UI 等 self-broadcast 是不必要的强约束，并在 self-broadcast 丢失场景产生永久 stale
> - 对"别人"的状态变化：HTTP（state-sync）是别人发的，自己根本收不到 → 必须靠 WS 广播驱动 → 对别人 WS 真是 single source of truth
> - self-broadcast 的剩余职责定义（不作 UI 唯一来源）：(a) 跨设备一致性校验（多端登录另一端的视图）(b) WS 链路活性探测 → 这两条都不要求 self-broadcast 必到达 → 丢失不影响发起者本地 UI
> - **反例（本次踩坑）**：把 "WS 是 single source of truth" 机械套用到"对自己"上 → HTTP 200 + self-broadcast 丢失 → 发起者 UI 永久 stale；兜底仅靠"下次状态切换 / WS 重连"无法在稳态运行中触发
> - **正例**：在契约层明确 self-broadcast 例外 —— 发起者本地 UI 由 HTTP 200 立即驱动 + self-broadcast 到达走 merge no-op；self-broadcast 仅作跨设备一致性 + 链路活性探测；让 self-broadcast 单路径丢失场景不影响发起者本地 UI 体验

---

## Meta: 本次 review 的宏观教训

本次 r2 三条 finding 都指向一个共同思维漏洞：**契约文档写"统一 / 等价 / single source of truth"这类强声明时，没有先把作用范围 / 前置条件 / 例外情况列清楚**。

- Lesson 1：跨章节"完全等价"声明没锁定 Story 14.3 前置条件 + 临时窗口
- Lesson 2：等价桶没区分"信任级别"维度，ack-only 字段被错塞进权威等价
- Lesson 3：single source of truth 没区分"对自己 / 对别人"两条路径，self-broadcast 丢失场景没兜底

三条都是"声明太抽象 → 应用到具体场景时产生矛盾"的同型问题。下次写契约级强声明前的标准 checklist：

1. 这条声明的**作用范围**是什么？所有字段 / 仅 server → client / 仅 client → server / 自己 / 别人 / 多端 / 单端？
2. 这条声明依赖**哪些前置条件**？哪些 story 必须先落地？前置条件未满足的临时窗口里 client 怎么办？
3. 这条声明的**反例 / 例外**是什么？哪些字段的信任级别不同？哪些 client 端身份（发起者 / 接收者）路径不同？

把这三条 checklist 答完再下"统一 / 等价 / single source of truth"的笔，能避免大部分"声明太强 → 实装层无所适从"的契约级矛盾。
