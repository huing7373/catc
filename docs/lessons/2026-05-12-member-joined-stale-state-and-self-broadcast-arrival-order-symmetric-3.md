---
date: 2026-05-12
source_review: file:/tmp/epic-loop-review-14-1-r3.md (codex review on Story 14.1 r3)
story: 14-1-接口契约最终化
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-12 — member.joined `pet.currentState` 在 14.3 落地前的 stale race & self-broadcast no-op 措辞必须基于到达顺序对称

## 背景

Story 14.1 r3（节点 5 接口契约最终化第 3 轮 codex review）在 r1（state-sync 幂等 / envelope ts）+ r2（跨章节等价分层 / ack vs 权威 / self-broadcast 兜底）修复之上又抓出两条契约级一致性 finding：

1. **P1 `member.joined.payload.pet.currentState` 固定 1 → join 房间后永久 stale race**：r2 临时不一致 note 只覆盖 §10.3 GET / `room.snapshot`，没覆盖 §12.3 `### 成员加入`；同时 §5.2 服务端逻辑步骤 5 允许用户在 `current_room_id == NULL` 时写 DB 但不广播 `pet.state.changed`。组合：用户房间外切 walk/run → DB `pets.current_state == 2/3` → join 房间 → `member.joined.payload.pet.currentState` 固定 1（placeholder） → 房间内其他成员永远看到 stale `1`，直到该用户**再次**切状态触发 `pet.state.changed`。

2. **P2 self-broadcast no-op 措辞与"WS 可能先于 HTTP 到达"内部矛盾**：r2 已在 §5.2 / §12.3 加了 self-broadcast 兜底规则（HTTP 200 立即驱动 → self-broadcast 到达 no-op）；但同时 §5.2 line 600 又说 WS 广播可能先于 HTTP 200 到达。两条不能同时永久成立 —— 如果 WS 先到，client 还没收到 HTTP 200 就把 self-broadcast 当 no-op 就会丢状态。

两条都是"契约文档自身矛盾 / 临时窗口未文档化"类问题。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `member.joined.payload.pet.currentState` 固定 1 与 state-sync 房间外写库语义矛盾 → join 后永久 stale race | High (P1) | docs / architecture | fix | `docs/宠物互动App_V1接口设计.md` §12.3 `### 成员加入` line 2110 + §5.2 line 606 + §10.3 line 1458 五阶段过渡表 + §1 line 46 |
| 2 | self-broadcast no-op 措辞与 §5.2 line 600"WS 可能先于 HTTP 到达"矛盾 | Medium (P2) | docs / architecture | fix | `docs/宠物互动App_V1接口设计.md` §5.2 line 547 / 600 + §12.3 line 2242 + line 2243 |

## Lesson 1: 跨业务消息的 placeholder 切真实路径 epic 落地点必须统一 + 临时不一致窗口必须覆盖**所有** server → client 同义字段

- **Severity**: high (P1)
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §12.3 `### 成员加入` `payload.pet.currentState` 字段行 + §5.2 line 606 临时不一致窗口 note + §10.3 五阶段过渡表 + §1 §46 Future Fields 声明

### 症状（Symptom）

§12.3 `### 成员加入` 的 `payload.pet.currentState` 字段在 r2 锁定的契约中只写"节点 4 阶段固定 `1`...Epic 14 才真实驱动"，没明确"由 Story 14.3 同 epic 落地点同步切真实"；同时 r2 加的临时不一致 note（§5.2 line 606）只列了 `room.snapshot` + GET `data.members[].pet.currentState`，没列 `member.joined.payload.pet.currentState`。

§5.2 服务端逻辑步骤 5 显式允许用户在 `current_room_id == NULL` 时写 DB 但不广播（无房间内成员需要被通知）；`pet.state.changed` WS 广播也仅在 user 在房间时触发。

组合 race：

1. 用户 A 在房间外切 walk（client → POST /pets/current/state-sync state=2） → DB 写 `pets.current_state = 2`，不广播
2. 用户 A join 房间 → server 触发 `member.joined` 广播给房间内其他成员 → payload `pet.currentState` 固定 `1`（节点 4 placeholder + r2 锁定未切真实）
3. 房间内其他成员看到 A 是 `rest`，但 A 实际 DB 是 `walk`
4. 直到 A 在房间内**再次**切状态触发 `pet.state.changed`，stale 才修复 —— 但如果 A 没再切，永远 stale

### 根因（Root cause）

写"`pet.currentState` 由 Epic 14 真实驱动"时，只盯着了 §10.3 五阶段过渡表（聚焦 GET / `room.snapshot`）+ `pet.state.changed`（聚焦状态变更广播），**漏看了 `member.joined` 也是一个携带 `pet.currentState` 字段的 server → client 消息**。

Story 14.3 的实装边界（接 `RoomSnapshotBuilder` 读真实 `pets.current_state`）天然覆盖 `room.snapshot`，但 `member.joined` 是 Story 11.8 落地的独立 broadcast 路径，14.3 不显式触碰它，除非契约层把"14.3 落地点同时覆盖 `member.joined`"写死。

更深层根因：**"placeholder 切真实"epic 落地点必须以"字段"为单位汇总，而不是以"消息"为单位**。同一个语义字段（`pet.currentState`，DB 来源 `pets.current_state`）出现在 N 个消息 / 接口里，切真实必须在同一 epic 同步切，否则就出现"部分消息真实、部分 placeholder"的窗口 race。

### 修复（Fix）

把 `member.joined.payload.pet.currentState` 字段行从"节点 4 阶段固定 `1` ... Epic 14 才真实驱动"改为"节点 4 阶段固定 `1`；**自 Story 14.3 起切真实值**（与 §10.3 五阶段过渡表 `pet.currentState` 节点 5 真实列 / §12.3 `room.snapshot.payload.members[].pet.currentState` 同时切真实路径，由 Story 14.3 落地 `RoomSnapshotBuilder` 真实化时同步覆盖 `member.joined`）"。

同步更新四处文档：

- §12.3 `### 成员加入` `payload.pet.currentState` 字段行（加 Story 14.3 同步切真实 + 阐明 stale race 风险）
- §10.3 五阶段过渡表 `pet.currentState` 节点 5 列（加"同一 epic 落地点 Story 14.3 同时覆盖 `room.snapshot` + `member.joined` 两处"）
- §12.3 `room.snapshot.payload.members[].pet.currentState` 字段行（加 Story 14.3 同步切真实 + 引用 `member.joined`）
- §5.2 临时不一致 note（line 606）（把覆盖范围从"`room.snapshot` / GET"扩展到"`room.snapshot` / GET / `member.joined`"，权威信号优先级表加入 `member.joined`，并显式列出 14.3 前的 join-after-out-of-room-state-change race）
- §5.2 字段语义跨章节等价（line 601 / 603）（把"五处"扩展到"六处"，"server → client 三处权威等价"扩展到"四处"）
- §12.3 `pet.state.changed` 关键约束（line 2247）（把"三处"扩展到"四处"）
- §1 line 46 Future Fields 声明（明示 `pet.currentState` 节点 5 真实驱动同时覆盖 `room.snapshot` + `member.joined`）
- §1 line 49 Epic 14 冻结影响范围（加"Story 14.3 落地点同时覆盖三处 `pet.currentState`"作为冻结契约的一部分）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **声明"字段 X 由 Epic Y 落地切真实"** 时，**必须**把所有携带 X 字段的 server → client 消息 / 接口都列出来 + 同步声明 Y 内具体哪个 story 是公共切真实落地点 + 临时不一致窗口必须覆盖该字段的**所有** placeholder location。
>
> **展开**：
> - **以字段为单位汇总切真实路径**：搜索文档中所有 `pet.currentState`（或同义字段路径，如 `payload.pet.currentState` / `data.members[].pet.currentState`）出现位置，逐一确认 placeholder 状态 + 切真实 epic / story；少列一处 → race 永久存在
> - **同一 epic 内的 story 顺序差**会制造临时不一致窗口：Epic 14 内 14.2（state-sync handler）+ 14.4（broadcast）能先于 14.3（聚合查询真实化）落地，期间已能写 / 广播真实值，但聚合查询 / join broadcast 仍 placeholder → 永久 stale race
> - **`current_room_id == NULL` 写 DB 不广播**是设计选择（用户房间外切状态不应骚扰任何房间），与"join 时下发当前状态"是契约上的两个独立责任 —— 后者**必须**覆盖前者写入的真实值，否则 stale
> - **反例 1**：只更新 `room.snapshot` 字段行的 Story 14.3 切真实声明，不更新 `member.joined` —— 14.3 落地后 `room.snapshot` 真实化但 `member.joined` 仍 placeholder `1`，race 仍存在
> - **反例 2**：临时不一致 note 只列 GET / `room.snapshot`，不列 `member.joined` —— 实装层读 note 后认为 "join 路径已覆盖"，结果 race 漏修
> - **反例 3**：把 14.3 / 14.4 当成"独立 story 各自负责自己消息字段切真实"，不显式声明 14.3 是公共落地点 —— 14.4 落地 `pet.state.changed` 真实化时，没有 contract 依据把 `member.joined` 也带上，14.3 完成时也不知道自己要管 11.8 的 broadcast 路径

## Lesson 2: HTTP/WS 双路下发的 client merge 措辞必须基于**到达顺序**对称，不假设固定顺序

- **Severity**: medium (P2)
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §5.2 line 547 / 600 + §12.3 line 2248 / 2249

### 症状（Symptom）

r2 在 §5.2 加了 self-broadcast 兜底规则："发起者在收到 HTTP 200 OK 后**立即更新自己**的本地 roster pet state，**不等** self-broadcast 到达"；后续 self-broadcast 到达走 no-op（值已相同）。

同时 §5.2 line 600 已声明："client 实装层不应**假设**两者同时到达 —— 可能 HTTP 200 先到（典型）；也可能 WS 广播先到（罕见，但合法）"。

两条不能同时永久成立：如果 WS 先到，client 还没收到 HTTP 200，self-broadcast 不能是 no-op（它就是先到的权威信号本身）；如果按 r2 的措辞 "HTTP 200 → 立即更新；self-broadcast → no-op" 实装，WS 先到的场景下 client 会把先到的 self-broadcast 直接当 no-op 丢弃，状态丢失。

### 根因（Root cause）

写 self-broadcast 兜底规则时，**默认了 HTTP 200 总是先到**（这是典型场景），把"先到的信号 → 驱动 UI；后到的信号 → no-op"这条对称规则错写成"HTTP 200 → 驱动 UI；self-broadcast → no-op"的不对称形式。

这与 §5.2 line 600 显式声明"两者到达顺序任一合法"自相矛盾。

更深层根因：**"基于到达顺序的对称无操作"是 idempotency 的核心模式，措辞必须按"先到 vs 后到"展开，而不是按"信号类型 A vs 信号类型 B"展开**。两路信号在 server 端来源同一次写库行为，值必定相同 —— merge no-op 的安全性来自"值相同"，与"哪个信号先到"无关。

### 修复（Fix）

把 §5.2 line 547 / 600 + §12.3 line 2248 / 2249 的 self-broadcast no-op 措辞精确化为"基于到达顺序的对称无操作"，对称展开 (a)/(b)/(c) 三条规则：

- **(a) HTTP 200 先到（典型）**：client 立即用 `response.data.state` 更新本地 self entry；后续 self-broadcast 到达 → merge no-op（值已相同）
- **(b) WS self-broadcast 先到（罕见但合法）**：client 立即用 `payload.currentState` 更新本地 self entry（与"别人的广播"统一走 client merge contract 字段级 merge 路径）；后续 HTTP 200 到达 → no-op（仅作 server 端入账成功的二次确认信号）
- **(c) 对称无操作不变量**：**任一路径先到的信号都立即驱动 UI 更新**，后到的信号是 no-op —— client 实装层**不**假设固定到达顺序

同步把 §12.3 line 2248（"按 §5.2 self-broadcast 兜底规则..."）+ line 2249（self-broadcast 丢失场景的 (b) 条）展开为基于到达顺序对称的措辞，避免任一处保留"HTTP 先到 → driver；self-broadcast → no-op"的不对称残留。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写 HTTP / WS 双路下发同语义信号的 client merge 措辞** 时，**必须**按"先到 vs 后到"对称展开，**禁止**按"信号类型 A vs 信号类型 B"不对称描述（除非契约层强制固定顺序，但此时必须显式声明并解释为什么 server 能保证）。
>
> **展开**：
> - **idempotency 的核心保证来自"值相同"**：两路信号来源同一次 server 端写库行为（service 层 UPDATE 后才触发 broadcast，HTTP 200 也由同一 service 函数返回） → 值必定相同 → 后到信号 merge 等价 no-op，**与具体到达顺序无关**
> - **措辞必须按到达顺序对称**：(a) X 先到 → 驱动 UI；后到的 Y → no-op；(b) Y 先到 → 驱动 UI；后到的 X → no-op；(c) 任一路径先到的信号都立即驱动，后到的是 no-op
> - **检查清单**：每写一条"X 触发 Y"措辞，问自己 —— 如果信号到达顺序反转，这条措辞还成立吗？不成立 → 必须按到达顺序对称展开
> - **反例 1**：写"HTTP 200 → 立即更新；self-broadcast → no-op"（不对称）。WS 先到场景 client 会把先到的 self-broadcast 当 no-op 直接丢弃 → 状态丢失
> - **反例 2**：写"self-broadcast 丢失由 HTTP 200 兜底"。这只覆盖 HTTP 先到场景；WS 先到但**整体丢失**的场景没有契约依据兜底（需补"HTTP 必到达，HTTP 失败时 client 已知 sync 失败 → 重试机制 by Story 15.4"）
> - **反例 3**：在同一文档内一处写"WS 可能先于 HTTP 到达"（line 600），另一处写"HTTP 200 → 立即；self-broadcast → no-op"（line 547） —— 同文档自相矛盾，client 实装无所适从

---

## Meta: 本次 review 的宏观教训

r3 的两条 finding 共享一个深层教训：**"placeholder vs 真实"和"双路下发 client merge"两类契约模式，措辞都必须按字段 / 顺序为单位**穷举**所有路径**，而不是按"消息 / 信号类型"为单位描述。

- Finding 1（`member.joined` stale）：placeholder 切真实必须以**字段**为单位汇总落地点，不能让同一字段在不同消息里"半切半不切"
- Finding 2（self-broadcast 对称 no-op）：双路下发 client merge 必须以**到达顺序**为单位对称展开，不能按"信号类型"不对称描述

两条都是"自然语言措辞偷懒 → 漏覆盖部分路径 → race / contradiction"模式。蒸馏到未来 Claude 的工作流里：**写完一条契约措辞，立即反查所有携带相同字段 / 信号的位置，确认措辞对所有路径同样成立**；不成立则展开 / 拆分至完全对称为止。
