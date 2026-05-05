---
date: 2026-05-06
source_review: codex review (epic-loop r4) — file: /tmp/epic-loop-review-10-1-r4.md
story: 10-1-接口契约最终化
commit: <pending>
lesson_count: 3
---

# Review Lessons — 2026-05-06 — WS 冻结段内部一致性（example 字段值 / 强制信封 / handshake 必发消息的失败路径）

## 背景

Story 10-1 r3 修复后再过 r4 codex review。前几轮已修过"error 双重语义"、"4005 心跳超时 close code"、"信封字段冻结"等大问题；r4 暴露的全是"冻结段内部前后矛盾"——字段表写一套、example 给另一套，或者 §12.1 / §12.3 不同子段对同一失败场景给互斥的处理路径。这些不是字段层 bug，而是**冻结时多人/多轮编辑下没做"全段交叉一致性 sweep"**的产物。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | room.snapshot 真实示例 `memberCount=2` 但 `members[]` 只 1 项 | medium (P2) | docs | fix | `docs/宠物互动App_V1接口设计.md:1502-1517` |
| 2 | SnapshotBuilder 失败 §12.3 标 "推 error(6005) 不 close"，与 §12.1 "snapshot 是握手后必发第一条" 互斥 | high (P1) | docs / architecture | fix | `docs/宠物互动App_V1接口设计.md:1330 / 1641-1648 / 1620 / 1639` |
| 3 | `member.joined` / `member.left` / `emoji.received` 草稿示例缺 mandatory `requestId` | medium (P2) | docs | fix | `docs/宠物互动App_V1接口设计.md:1543-1577` |

## Lesson 1: example 内部数值字段必须满足字段表给的相互不变量

- **Severity**: medium (P2)
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1502-1517`

### 症状（Symptom）

`room.snapshot` 真实示例 payload 写 `memberCount: 2`，但 `members[]` 数组只有一个 member entry。字段表把 `memberCount` 定义为 "当前房间在线成员数"——客户端 / fixture 用这个示例时会拿到自相矛盾的两个值，不知道该信谁。

### 根因（Root cause）

冻结段在 r3 把 placeholder 示例 (`members: []` + `memberCount: 0`) 和真实示例 (Story 11.7 落地后形态) 拆成两个 example block 时，真实示例只是从 placeholder 改了几个字段值，**没有重新校验"两个字段间存在的相互不变量"**：`memberCount === members.length`（节点 4 阶段两者均 placeholder 0；真实阶段两者由同一次 JOIN 聚合产出）。example 字段值是孤立维护的，没人把它们当 "必须满足同一约束的一组值" 看。

### 修复（Fix）

- 真实示例 `members[]` 补齐第二个成员 entry（userId=1002, nickname=B, pet=2002, currentState=1），让 `members.length === memberCount === 2`
- 在示例后追加一句注：`memberCount` **必须**等于 `members[]` 数组长度，server 实装层面禁止两者 drift（明确把"相互不变量"写成契约文字，不留口头约束）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **冻结协议章节里写 / 改 JSON example** 时，**必须**对 example 内**任一组存在数学/语义关系的字段**（如 count vs array length、sum vs items[].amount、startTs < endTs）做一次显式核对，并把"相互不变量"写成 example 旁的契约文字，不留口头约束。
>
> **展开**：
> - example 不是装饰；下游 client 会拿来当 fixture / parser test / UI count 来源。example 内字段值若违反字段表给的不变量，下游会得到"该信哪个？"的歧义。
> - 修 example 时，凡是改一个 count / sum / length / 时间戳，必须**同步检查**与之挂钩的另一字段是否也要改。
> - 如果该不变量在字段表的"说明"列没明写，**借这次修复把它写明**（"必须等于 X"），不要让后续 reader 重新踩同一个坑。
> - **反例**：把 placeholder example 复制成真实 example、只改了 `memberCount: 0 → 2` 但忘了把 `members: []` 也改成两个 entry，留下"数字说 2 / 数组只 1"的内部矛盾。

## Lesson 2: handshake 必发消息的失败路径必须 close socket，不能仅推 error

- **Severity**: high (P1)
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1330` (close code 1011 行) / `1641-1648` (节点 4 阶段适用错误码表 + 末尾注) / `1620` (error 字段表 code 列说明) / `1639` (error 示例后注)

### 症状（Symptom）

§12.1 钦定 `room.snapshot` 是握手成功后**必发的第一条消息**（"成功后返回房间快照"），但 §12.3 节点 4 适用错误码表写 "Story 10.7 SnapshotBuilder 抛 error 时 → server 主动推 error(code=6005)"——这是 `error` 消息，按 §12.3 error 章节说明 server **不**关闭连接。结果：client 进入"已建连但永远收不到必发的 snapshot"的卡死态——房间页因为缺 snapshot 无法初始化，且因为连接没 close，client 的 auto-reconnect 也不会启动。

### 根因（Root cause）

§12.1 和 §12.3 是**不同章节、不同视角**：§12.1 写连接生命周期（close code 是"连接挂掉的语义"），§12.3 写消息层 error（error 是"连接还活着但业务侧出错"）。SnapshotBuilder 是握手后第一个 server-side 动作——它失败到底属于"连接生命周期挂了"还是"业务侧出错了"？冻结时按"业务错误码"思路把它扔进了 §12.3 error 表（因为有现成的 6005 房间状态异常码），但**没回头检查这个语义在 §12.1 的握手契约下是否能 work**——只要 snapshot 是 must-have first message，它的失败就只能是 close-path，不能是 non-fatal error-path。

### 修复（Fix）

- §12.1 close code 表 1011 行：触发条件追加 "**含**握手完成后 SnapshotBuilder 构建初始 `room.snapshot` 失败的场景（reason = `"snapshot build failed"`）"，并把"为什么"明写出来——snapshot 是必发第一条消息，不 close 会卡死 client + 不触发 auto-reconnect。
- §12.3 节点 4 适用错误码表：删除 6005 行（不再把 SnapshotBuilder 失败列为 error 推送场景）；在表下追加注，明示 snapshot 构建失败统一走 close 1011；6005 保留给后续业务 epic 在房间已可用之后的运行时状态错误推送。
- §12.3 error 字段表 `payload.code` 列：在示例码列表里去掉 `6005`，并加 inline 提醒"6005 不用于初始 snapshot 失败"。
- §12.3 error 示例后的"注"：把"Story 10.7 SnapshotBuilder 抛错"从"server 主动推 error 的例子"列表里删掉，新增一段明示"握手后初始 snapshot 失败不在 error 范围、统一走 close 1011"。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **WS 协议文档里给某个握手后 must-have 消息（"必发第一条" / "must-receive before X")的失败场景写处理路径** 时，**必须**走 close-path（指定 close code + reason），**禁止**走 "保连接 + 推 error" 的 non-fatal path——因为后者会让 client 卡在"已建连但永远拿不到必发消息"的死锁态，且 auto-reconnect 不会触发。
>
> **展开**：
> - 判断标准：如果 message X 在协议契约上是 "握手成功 → 必须收到 X 才能进入正常状态"，那 X 的失败就是连接生命周期问题，不是业务错误。
> - 写文档时务必做一次"反向 trace"：从 client side 角度想——"如果我在 X 之前收到 error 但 socket 还活着，我能干嘛？"。如果答案是"什么也干不了 / 只能等永远等不到的 X"，说明该路径必须 close，不能 error。
> - 同一失败场景的处理路径不能跨 §（§12.1 close 表 vs §12.3 error 表）出现互斥结论。冻结一个章节前必须 cross-section sweep：把每个错误场景列出来，确认 close path 段和 error path 段对它的归属一致。
> - **反例**：把 SnapshotBuilder 失败写成 "推 error(6005) 不 close"，理由是 "已经有 6005 房间状态异常码可复用"——这是按"已有错误码视角"做归类，没考虑"snapshot 是 must-have first message" 这个上层契约约束。

## Lesson 3: 即使是延后锚定的草稿示例，也必须满足本 story 已冻结的信封 schema

- **Severity**: medium (P2)
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1543-1577`

### 症状（Symptom）

§12.3 章节末尾保留了 `member.joined` / `member.left` / `emoji.received` 三个 server→client 草稿示例（业务消息延后锚定到 Story 11.1 / 17.1）。但这些示例只有 `type` / `payload` / `ts` 三个字段，**没有 `requestId`**。本 story 已冻结的 §12.3 通用信封表把 `requestId` 标为 mandatory，且明确广播 / 主动推送类消息固定填 `""`。两者并存——下游 Story 11.1 / 17.1 直接复制这些示例当 fixture，会得到一份不符合冻结信封的实现。

### 根因（Root cause）

冻结时正确地理解到 "业务消息字段层契约延后锚定 ≠ 草稿示例可以脱离当前冻结的信封 schema"——前者是字段层（payload 内部字段），后者是消息层（envelope 字段）。但执行时只把"业务消息延后锚定"作为口头免责声明加在示例段末，没回头检查草稿示例本身**是否已经符合本 story 已冻结的信封最小集**。延后锚定不是"全段不管"的护身符；payload 字段可以延后，但 envelope（type / requestId / payload / ts）是 §12.3 本 story 范围内冻结的字段，**草稿示例必须符合**。

### 修复（Fix）

三个示例分别在 `type` 后、`payload` 前补 `"requestId": ""`（广播 / 主动推送类消息固定 `""`，与冻结的 envelope 表一致）。**不**修 payload 字段（那才是延后锚定的部分）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **冻结某段协议文档时遇到"业务消息延后锚定"草稿示例** 时，**必须**让草稿示例**完全符合本 story 已冻结的 envelope schema**（type / requestId / payload / ts 等信封字段），延后锚定的语义只覆盖 payload 内部字段，**不**覆盖 envelope。
>
> **展开**：
> - "延后锚定"声明的精确语义：payload schema 由后续 epic 锁定；envelope schema 是本 story 冻结，**当前**就是契约。
> - 检查动作：冻结某段时，对该段所有 server→client 示例（含草稿）跑一遍 envelope schema sweep——type / requestId / payload / ts 是否都有？requestId 的取值（broadcast `""` vs response 回带）是否对？空 payload 是否显式写 `{}`？
> - 草稿示例的存在意义是给下游 epic story 复制粘贴当 fixture。如果它本身违反当前冻结的 envelope，下游一复制就引入 bug。
> - **反例**：用"业务消息延后锚定"作为护身符，把整个示例（含 envelope）从冻结检查里豁免，导致草稿示例与冻结信封表两套不兼容 schema 并存于同一段。

---

## Meta: 本次 review 的宏观教训

r4 三条 finding 共享同一个根因模式：**冻结某协议段时，没做"段内交叉一致性 sweep"**。表现形式不同——

- Lesson 1 是 example 内部数值字段间的不变量没核对；
- Lesson 2 是 §12.1 vs §12.3 跨子段对同一失败场景给互斥的处理路径；
- Lesson 3 是冻结字段表 vs 草稿示例之间存在两套不兼容 schema。

**统一规则**：**每次冻结一个协议章节前**，必须执行至少三个 sweep：

1. **example × 字段表**：每个 example 的字段值是否满足字段表所有约束（不变量、必填、取值域）？
2. **跨子段（§X.1 / §X.2 / §X.3）**：同一概念（如某个失败场景、某个状态、某个字段语义）在不同子段是否给出一致的处理路径？
3. **冻结字段 × 延后锚定字段**：草稿示例 / placeholder 示例是否完全符合本 story 已冻结的最小集（envelope schema）？延后锚定的口头声明只覆盖延后字段，不覆盖冻结字段。

冻结协议段是高 stakes 操作（一旦冻结，跨多 epic 多 story 的实装都基于此契约），sweep cost 远小于事后让多个下游 story 复刻同一个 bug 的 cost。
