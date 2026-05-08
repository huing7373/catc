---
date: 2026-05-08
source_review: codex review round 6 on Story 11.1 接口契约最终化（/tmp/epic-loop-review-11-1-r6.md）
story: 11-1-接口契约最终化
commit: 4471308
lesson_count: 2
---

# Review Lessons — 2026-05-08 — 协议字段表 vs JSON 示例 + prose 协议变更必须同步对应 mermaid sequenceDiagram

## 背景

Story 11.1 r5 修复阶段对 `room.snapshot` 的 `payload.members[].pet.currentState` 字段做了"节点 4 placeholder + Story 11.7 真实实现均固定 `1`"的钦定（V1接口设计.md §12.3 行 1839 字段表 + §12.3 placeholder 字段值来源说明），并在时序图设计.md §13.2 / §13.3 后注解里更新为"自 Story 11.1 起 server → client active message set 加入 `member.joined` / `member.left`、HTTP join 事务提交后广播 `member.joined`、HTTP leave 事务提交后广播 `member.left` + close 4007"。但 r6 codex review 指出两处文档内部不自洽：

1. V1接口设计.md §12.3 字段表行 1839 钦定 `currentState=1`，但同节"真实示例" JSON（行 1866）仍是 r5 改动前的 `currentState: 2`，例子和字段表自相矛盾。
2. 时序图设计.md §13.2 / §13.3 文字注解已经声明"HTTP join/leave 事务后会广播 + leave 触发 close 4007"，但同文档 §11.2（加入流程）与 §12.2（退出流程）的 mermaid sequenceDiagram 仍止于 HTTP 200，没有画出 post-commit 广播 + close frame，图与文不一致。

两条都属 P2 docs：协议文档是 Story 11.x 之后所有 server/iOS 实装的唯一权威参考；示例和图与字段表 / prose 不齐时，实装方按错的那一份做就直接埋雷。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | room.snapshot 真实示例 currentState 与字段表 1839 钦定不一致 | medium | docs | fix | `docs/宠物互动App_V1接口设计.md:1866` |
| 2 | §11.2 / §12.2 mermaid 图未画出 HTTP join/leave 后 broadcast + close 4007 | medium | docs | fix | `docs/宠物互动App_时序图与核心业务流程设计.md:439-460, 478-497` |

## Lesson 1: 协议字段表与 JSON 示例必须 zip 对齐

- **Severity**: medium
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1866`

### 症状（Symptom）

§12.3 `room.snapshot` 字段表（行 1839）钦定 `payload.members[].pet.currentState` 在节点 4 placeholder 与 Story 11.7 真实实现都固定返回 `1`（rest，Epic 14 才真实驱动）；但同节"真实示例" JSON 块第一个成员 `userId=1001` 的 `currentState` 仍写 `2`。实装方拿示例 copy-paste 做 fixture，会埋下"返回 walk 状态"的错误，恰好回到 Story 11.1 全 epic 试图根除的 motion-state 混淆。

### 根因（Root cause）

字段表行的语义升级（"节点 4 阶段固定返回 1" → "Story 10.7 placeholder + 11.7 真实均固定 1"）在 r5 修复阶段加入；当时改动聚焦在字段表 prose（行 1839）和 placeholder 字段值来源说明（行 1920），没有同步扫描同节的 JSON 示例。JSON 示例的 `currentState: 2` 是字段表早期版本（"枚举允许 1/2/3，示例展示 2 表达 walk"）残留下的旧值 —— 当时字段表允许 1/2/3，示例展示一个 walk 成员合理；后来字段表升级为节点 4 阶段固定 `1`，示例没跟上。

更深层根因：协议文档里"字段表（含语义/约束/默认值/取值范围）"和"JSON 示例"是两份对同一份契约的视角，必须 zip 对齐 —— 任何一边动了，另一边必须当下同步。r5 改动时只盯着 prose 表里的语义升级，没把"示例的字段值"看作字段表语义的具体投影。

### 修复（Fix）

把 §12.3 真实示例 `userId=1001` 成员的 `pet.currentState` 从 `2` 改为 `1`。第二个成员 `userId=1002` 已经是 `1`，不动。

```diff
       {
         "userId": "1001",
         "nickname": "A",
         "pet": {
           "petId": "2001",
-          "currentState": 2
+          "currentState": 1
         }
       },
```

注：行 1926 的 placeholder 字段值来源说明里举例 "`pet.currentState: 2`" 是描述未来 Epic 14 真实驱动后场景下 client merge 行为的 hypothetical 例子，不是 §12.3 真实示例本身，保留不动。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在改协议文档（V1接口设计.md / 数据库设计.md）的**字段表 / 字段语义**时，**必须**同步扫描同节内的所有"JSON 示例 / 真实示例 / placeholder 示例"块，确保示例字段值与字段表 freeze / range / 默认值约束 zip 对齐。
>
> **展开**：
> - 改动一个字段的"取值范围"或"freeze 钦定"（如"固定返回 X" / "节点 N 阶段允许 Y"）后，**必须** `grep` 该字段名（如 `currentState`）扫整个文档，逐个确认 JSON 示例里的具体值符合新约束。
> - 协议文档里多组字段同时升级时，每组字段都要做这个 grep 扫描；批量改完只看 prose 是否自洽不够，还要看示例。
> - 字段表与示例不一致时，**优先以字段表为准** —— 字段表是契约，示例是字段表的投影；不要反过来"以示例值为基准修字段表"，那等同于让 example fixture 反向定义协议。
> - **反例（踩坑模式）**：r5 修复时把 §12.3 字段表 `currentState` 从"节点 4 阶段固定 1"升级为"Story 10.7 placeholder + 11.7 真实均固定 1"，prose 改对了，但同节示例 JSON 没扫，残留 r5 升级前的 `currentState: 2`，跨段语义自相矛盾。
> - **反例（踩坑模式）**：用"我只升级了 prose 的语义层语句，示例字段值不在我改动的语义边界里"作为不扫示例的理由 —— 错。字段值约束（freeze 到某个值）就是字段语义；示例值就在那个语义边界里。

## Lesson 2: prose 改了协议必须同步对应 mermaid sequenceDiagram

- **Severity**: medium
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_时序图与核心业务流程设计.md:439-460, 478-497`

### 症状（Symptom）

时序图设计.md §13.2 / §13.3 文字注解（r3-r5 修订期持续累加）已声明：

- 自 Story 11.1 起 server → client active message set 加入 `member.joined` / `member.left`
- HTTP `POST /rooms/{roomId}/join` 事务成功提交后 → broadcast `member.joined` 给房间内其他在线成员
- HTTP `POST /rooms/{roomId}/leave` 事务成功提交后 → broadcast `member.left` + 给 leaver 自己发 close 4007

但同文档 §11.2（加入房间流程）和 §12.2（房间退出流程）的 mermaid sequenceDiagram 仍是 r3 之前的旧版 —— 都止于 `API-->>Client: 加入/退出成功`（HTTP 200），没画 post-commit 广播 / close。实装方读时序图（这是工程参考的主要可视化路径）只看到 HTTP 200 就结束，会漏掉 WS 副作用，与 prose 钦定的协议行为脱节。

### 根因（Root cause）

r3-r5 修订期对 `member.joined` / `member.left` / close 4007 的钦定只通过 §13.2 / §13.3 后的 prose 注解块加入，没有同步去改 §11.2 / §12.2 的 mermaid 图。原因：mermaid 图位于"加入/退出 HTTP 流程"章节，prose 注解位于"WS 建连流程"章节，r3-r5 修复路径上 grep 命中的都是 §13.x 区段（搜 `member.left` / `4005` / `close 4007` 等关键字），没意识到同事件的 mermaid 图在 §11.2 / §12.2。

更深层根因：协议变更通常涉及"事件触发条件 + 事件 payload + 事件触发后副作用"三层。prose 改了"触发条件"（HTTP 事务后广播）+"副作用"（close 4007），但**承载该事件流程时序的可视化图（mermaid sequenceDiagram）位于另一个章节**。改 prose 时如果不显式问"哪些图刻画了这条流程"，图就会被静默落下。

### 修复（Fix）

§11.2 加入流程 mermaid 图末尾加：

```diff
+    participant WSGateway as WS Gateway
+    participant Others as 房间其他在线成员
     ...
     Service-->>API: 返回 joined = true
-    API-->>Client: 加入成功
+    API-->>Client: HTTP 200 加入成功
+    Note over Service,WSGateway: 事务提交后（post-commit）触发广播<br/>自 Story 11.1 起锚定
+    Service->>WSGateway: BroadcastToRoom(roomId, member.joined)
+    WSGateway->>Others: WS 推送 member.joined<br/>{userId, nickname, avatarUrl, pet:{petId, currentState}}
```

§12.2 退出流程 mermaid 图末尾加：

```diff
+    participant WSGateway as WS Gateway
+    participant Others as 房间其他在线成员
     ...
     Service-->>API: 返回 left = true
-    API-->>Client: 退出成功
+    API-->>Client: HTTP 200 退出成功
+    Note over Service,WSGateway: 事务提交后（post-commit）触发广播 + 主动关闭<br/>自 Story 11.1 起锚定
+    Service->>WSGateway: BroadcastToRoom(roomId, member.left)
+    WSGateway->>Others: WS 推送 member.left {userId}
+    Service->>WSGateway: CloseLeaverConnection(userId)
+    WSGateway->>Client: WS Close 4007 "left room via HTTP"
```

注意 leaver 与 Others 是两个 participant：leave 流程下 server 既要给"其他成员"广播 `member.left`，又要给"leaver 自己"发 close 4007；mermaid 用两个独立 participant 把这一区分画清楚，避免实装方误解为"广播 == close all"。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在改协议文档（V1接口设计.md / 时序图设计.md / 数据库设计.md）的 prose 协议规则时，**必须**显式 grep 同文档（含跨章节）所有 `mermaid` block，识别哪些图刻画了被改动的流程 / 事件，并同步更新这些图。
>
> **展开**：
> - 改协议 prose 时（如"事件 X 自 Story Y 起加入 active message set" / "操作 A 事务后广播 B" / "事件 C 触发 close D"），**必须**做两步：
>   1. `grep -nE "mermaid|sequenceDiagram" <doc>`：定位文档里所有 mermaid 图块。
>   2. 对每张图：判断它是否刻画了被改动事件 / 流程的 actor 或时序；如果是，**当下同步**改图，不要分两次 review 去做。
> - 单文档里 prose 注解和对应 mermaid 图常在不同章节（如时序图设计.md 的 prose 注解在 §13.2 / §13.3 WS 建连章节，但 join/leave 流程的 mermaid 图在 §11.2 / §12.2 房间章节）。改 prose 时**章节定位**不能只盯 grep 命中的章节范围，要按"这条规则在描述什么事件 / 流程" → "哪些章节里有该事件 / 流程的可视化图"扩展扫描。
> - 跨文档时（V1接口设计.md 的 prose 改了 → 时序图设计.md 的 mermaid 图也要改），同理。
> - 多 actor / 多副作用流程（如 leave：post-commit 广播给 N 个其他成员 + 给 leaver 自己 close）在 mermaid 里要把"N 个其他成员"和"leaver 自己"画成两个独立 participant，避免读者误读为"全部 close"或"全部广播"。
> - **反例（踩坑模式）**：r3-r5 在 §13.2 / §13.3 的 prose 注解里加了一大段"自 Story 11.1 起 active message set 加入 member.joined/.left + HTTP join/leave 后广播 + leave 触发 close 4007"的钦定，但 §11.2 / §12.2 的 mermaid sequenceDiagram 仍是 r3 之前的旧版，止于 HTTP 200，导致 prose 与图不一致。
> - **反例（踩坑模式）**：用"prose 章节和 mermaid 章节不在一处，我没意识到要扫"作为不改图的理由 —— 错。改协议时显式跑一次"全文 mermaid 扫描"是防止该类盲区的唯一可靠手段。

---

## Meta: 本次 review 的宏观教训

r1-r6 共六轮 review 都集中在 Story 11.1 协议契约文档化层，跨轮 lessons 浮现的同一个底层规律：

> **协议契约文档是多视角投影系统**：同一份契约会以"字段表 prose / JSON 示例 / mermaid sequenceDiagram / 触发点 prose 注解 / placeholder vs 真实阶段语义说明 / client merge contract"等多种视角分散表达。任何视角层的钦定升级，**必须**显式扫描所有其他视角层并同步对齐 —— 协议文档的 review 失败模式 99% 是"改了视角 A，忘了视角 B"。

具体 review skill 时的 grep checklist（写代码前先做的 hygiene step）：

1. 改字段表 → grep 字段名扫所有 JSON 示例。
2. 改 prose 协议规则 → grep mermaid block 扫所有时序图（含跨章节、跨文档）。
3. 改某事件触发条件 → grep 事件名（如 `member.joined`）扫所有触发点说明 + active message set 声明。
4. 改 placeholder 阶段约束 → grep 该 stage 的所有"过渡表 / placeholder vs 真实对照表 / client merge contract"。

这个 checklist 应该作为 Story 11.x / Epic 11 之后所有协议契约 review 的 SOP 起点，而不是每次 review 都靠 codex 兜底发现。
