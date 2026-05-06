---
date: 2026-05-06
source_review: codex review r8 (epic-loop-review-10-1-r8.md, /tmp/epic-loop-review-10-1-r8.md)
story: 10-1-接口契约最终化
commit: c15f247
lesson_count: 2
---

# Review Lessons — 2026-05-06 — room.snapshot placeholder 必须反映 room_members 全表 & 节点 4 断连分支同样不广播 member.left

## 背景

Story 10.1（节点 4 协议骨架契约最终化）的 r8 codex review。r6 把 `memberCount` / `members[]` 不变量统一为 "full roster view"，r7 又把 placeholder 例子从 "全零" 改成 "单成员快照"（仅当前握手用户）—— r8 指出**单成员快照同样错误**：当房间已有 ≥2 成员时，placeholder 仅返回当前用户会让 client 把 first authoritative `room.snapshot` 当成 "房间被清空"，错误覆盖已加载的合法 roster。

并行问题：r7 在 `时序图与核心业务流程设计.md §13.2` 末尾加了 "节点 4 阶段服务端只发 `room.snapshot` / `pong` / `error`" 注解，但忘记 sweep 紧邻的 §13.3 断连处理 —— §13.3 仍写 "广播 `member.left` 给其他在线成员"，同文档自相矛盾。

两条都是 r6/r7 同主题（snapshot 视图模型 + 跨文档时序图同步）的连续修复，必须一次贯穿到所有相关字段表 + 示例 + 跨文档引用。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | room.snapshot placeholder 限死单成员 → 房间已有 ≥2 成员时 client 漏返其他成员 | high (P1) | architecture / docs | fix | `docs/宠物互动App_V1接口设计.md:1485-1486` `:1487-1490` `:1534` `:1536-1563` `:1307` |
| 2 | §13.3 断连处理仍写 "广播 member.left"，与 §13.2 末尾 "节点 4 只发三类" 冲突 | medium (P2) | docs | fix | `docs/宠物互动App_时序图与核心业务流程设计.md:541-554` |

## Lesson 1: room.snapshot placeholder 必须反映 room_members 全部成员行 — 仅丰富字段降级，禁止在成员数量上偷工

- **Severity**: high (P1)
- **Category**: architecture / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1485-1486`（字段表 memberCount / members 行），`:1534`（不变量），`:1536-1563`（placeholder 示例 + 来源说明），`:1307`（§12.1.3 握手成功流程对 placeholder 的描述）

### 症状（Symptom）

r7 把 placeholder 描述改成 "当前握手用户自身的单成员快照"（`memberCount: 1`，`members: [{当前用户}]`），并把示例 JSON 改成 1 entry。但 §12.1 第 5 步握手成功**充分条件**只校验 "当前用户已在 `room_members` 表中"，**不**保证房间只有 1 个成员。当房间已经有 2+ 成员（比如另一个用户先 `POST /api/v1/rooms/{roomId}/join` 加入但当前没开 WS）时，单成员 placeholder snapshot 会让 client 把首条 authoritative 消息当成 "房间被清空"，错误覆盖已加载的合法 roster。本质与 r7 finding 同根因 —— 只是从 "全零" 改成 "单成员"，仍是 "少返"。

### 根因（Root cause）

placeholder 阶段的 "降级" 必须区分两个维度：**(a) 成员数量结构** vs **(b) 单成员丰富字段**。r7 错误地把 placeholder 简化作用在维度 (a)（直接砍成 1 个成员），但维度 (a) 是 client 渲染的核心结构 —— roster 是房间页的主要状态。维度 (b)（`nickname` / `pet.*` 等丰富字段）才是 placeholder 阶段可以降级的部分（避免 JOIN `users` / `pets` 多表查询）。

更本质的思维漏洞：r7 误以为 placeholder = "实装最简化" = "只查 §12.1 第 5 步已查到的当前用户行，避免增量 query"。但 placeholder 阶段就算只做单表查询 `SELECT * FROM room_members WHERE roomId=?`，复杂度也只是 O(1) query 数 —— **依然 placeholder 复杂度**。真正避免耦合 Story 11.7 的是 "不 JOIN 多表"，而不是 "只返当前用户"。

### 修复（Fix）

1. **§12.3 字段表 `memberCount` 行**（`:1485`）：改写为 "placeholder 实现 = `SELECT COUNT(*) FROM room_members WHERE roomId=?` 的真实行数（或同一次 query 直接取 `len(members)`，二者必须一致），即真实反映 room_members 全表的成员数，**禁止**写死为 1"
2. **§12.3 字段表 `members` 行**（`:1486`）：改写为 "placeholder 实现 = `SELECT * FROM room_members WHERE roomId=?` 的全部行（**禁止**只返回当前握手用户自己），单表查询不依赖 JOIN，本身已足够简单；丰富字段在 placeholder 阶段降级"
3. **§12.3 字段表 `nickname` / `pet.petId` 行**（`:1488-1489`）：加 placeholder 行为说明 —— "node-4 placeholder 阶段允许返回**空字符串** `""`（避免 JOIN `users` / `pets`），Story 11.x（nickname）/ Story 14.x（pet）真实回填"
4. **§12.3 不变量 paragraph**（`:1534`）：删 "至少包含当前握手用户自身条目（`memberCount: 1`，`members: [{当前用户}]`）" 那一段，改 "必须反映 `room_members` 全部成员行（最少 1 个，即握手用户自己；房间已有 ≥2 成员时必须返回全部）"；同时**新增**禁止 "单成员快照" 的明确反例（与禁止 "全零 placeholder" 并列）；明确 placeholder vs 真实实装差异在于 "丰富字段" 而非 "成员数量"
5. **§12.3 placeholder 示例**（`:1536-1563`）：改成 2 成员示例（`userId: "1001"` 当前用户 + `userId: "1002"` 离线成员），`nickname` / `pet.petId` 用空字符串 `""` —— 让示例直接说明 placeholder 含离线成员，且降级位置仅在丰富字段
6. **§12.1.3 握手成功流程第 1 条**（`:1307`）：把 "members 数组**至少**含当前握手用户自身条目" 改成 "members 数组反映 `room_members` 全部成员行 —— 单表查询不 JOIN，丰富字段 `nickname` / `pet.petId` 在 placeholder 阶段允许空字符串"

### 预防规则（Rule for future Claude）⚡

> **一句话**：写 placeholder 实装契约时，**必须**区分 "成员数量结构" vs "单成员丰富字段" 两个降级维度，**禁止**在成员数量上偷工 —— placeholder 只能降级丰富字段（允许空串 / 默认值 / 省略），**绝不能**只返回 "当前握手用户自己" 而漏返其他已在 `room_members` 表的成员。
>
> **展开**：
> - placeholder 的 "简化" 应该作用在 "丰富字段层"（不 JOIN 多表，丰富字段允许空串），**不**作用在 "结构层"（roster 数量、ID 列表必须真实）
> - 单表 `SELECT * FROM room_members WHERE roomId=?` 的复杂度本身就是 placeholder 级别，**不**算 "过早实装" —— 真正避免耦合下一阶段的是 "不 JOIN 多表"，不是 "只查当前用户行"
> - Snapshot / state-broadcast 类消息的 placeholder 必须 "结构上真实，丰富字段降级"。client 把 first authoritative 消息作为权威态采用时，丰富字段缺失只是渲染降级（空昵称 / 占位头像），结构缺失（漏成员）会直接污染 client state
> - 改 placeholder 描述时**强制**问自己：「房间已有 N 成员（N ≥ 2）的场景下，client 收到 placeholder snapshot 会怎么渲染？」如果会清空已加载的合法 roster，placeholder 描述就有问题
> - **反例**：r7 把 placeholder 改成 "仅当前握手用户的单成员快照（`memberCount: 1`）"，理由是 "避免 placeholder 阶段过度耦合 Story 11.7 的多表 JOIN" —— 错误地把 "避免 JOIN" 翻译成了 "只查当前用户行"。正确的 "避免 JOIN" 应该是 "只查 `room_members` 单表（保留全部成员），丰富字段用空串"
> - **反例**：placeholder 描述里写 "至少包含当前握手用户自身条目" —— "至少" 是必要条件，不是充分条件；server 实装者读到 "至少 1 个" 会偷懒只返 1 个；正确措辞是 "**反映** `room_members` **全部成员行**，最少 1 个（即握手用户自己），房间已有 ≥2 成员时必须返回全部"

## Lesson 2: 跨文档冻结声明的 sweep 必须覆盖同文档所有相邻分支（§13.2 + §13.3 必须同步）

- **Severity**: medium (P2)
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_时序图与核心业务流程设计.md:541-554`（§13.3 断连处理）

### 症状（Symptom）

r7 在 `时序图与核心业务流程设计.md §13.2` 末尾加了 "业务消息延后锚定" 注解，明示 "节点 4 阶段（Epic 10 ~ 13）服务端 → 客户端**只**会主动发送 `room.snapshot` / `pong` / `error` 三种消息，**不**在握手完成时广播 `member.joined`"，并删除了时序图里 "推送 member.joined" 步骤。但同文档紧接着的 §13.3 断连处理仍写 "广播 `member.left` 给其他在线成员"，与 §13.2 末尾的冻结声明直接冲突。dev 实装节点 4 时无法判断 `member.left` 在断连时到底应不应该广播。

### 根因（Root cause）

r7 修 §13.2 时聚焦在 "握手时的 `member.joined`"，但**忘记 sweep 同文档相邻 §13.3 的对偶分支**（断连时的 `member.left`）—— `member.joined` 和 `member.left` 是同一类业务广播消息（成员状态变更），冻结声明对一个生效就必然对另一个生效。修一处忘 sweep 对偶分支是结构化文档的常见踩坑形态。

### 修复（Fix）

§13.3 断连处理：

- 删除 "广播 `member.left` 给其他在线成员" 这一步，改为 "（业务消息延后锚定，节点 4 阶段**不**广播 `member.left`；详见本节末"业务消息延后锚定"注解；字段层契约由 Story 11.1 锚定）"
- 在 §13.3 注意事项之后**新增** "业务消息延后锚定" 注解块，与 §13.2 末尾同模板：明示节点 4 阶段服务端只发三类消息、断连时不广播 `member.left`、`member.left` 字段层契约由 Story 11.1 锚定、本时序图已删除原 "广播 member.left" 步骤、真实时序在 Story 11.1 实装期重绘

### 预防规则（Rule for future Claude）⚡

> **一句话**：在文档里加 "节点 X 阶段只发 / 只做某某操作" 的冻结声明时，**必须** sweep 同文档**所有对偶分支**（如握手 vs 断连、加入 vs 离开、写 vs 读）—— 业务消息的字段层契约对一个分支生效就对所有同类分支生效，修一处忘对偶是结构化文档常见踩坑。
>
> **展开**：
> - 冻结声明的 "颗粒度" 是消息类型（`member.joined` / `member.left` / `emoji.received` 都是业务广播），不是触发场景（握手 vs 断连 vs 业务事件）。改一处时必须问 "这条消息还在哪些分支被提到？"
> - sweep 的具体动作：grep 同文档所有 "广播 X" / "推送 X" / "发送 X" 形态的句子；不是只看光标周围的段落
> - 注解块的措辞要可复用：在 §13.2 末尾用过的 "业务消息延后锚定 …… 字段层契约由 Story Y 锚定 …… 本时序图已对齐节点 N 协议骨架冻结" 模板，§13.3 应直接复用同模板（一致性 + 减少 dev 阅读认知负担）
> - **反例**：r7 修 §13.2 时序图末尾加 "节点 4 只发三类消息" 注解、删除 "推送 member.joined" 步骤，但忘 sweep 紧邻 §13.3 断连处理仍写 "广播 member.left" —— 同文档自相矛盾，dev 实装时按 §13.3 写代码就会违反 §13.2 冻结声明
> - **反例**：把冻结声明只放在 "握手成功流程" 那一节，认为断连流程是 "另一个 use case 不影响" —— 错误。冻结声明的语义是 "整个阶段的消息白名单"，不是 "某个流程的消息白名单"

---

## Meta: 本次 review 的宏观教训

r6 / r7 / r8 三轮连续都在 snapshot 视图模型 + 跨文档时序图同步上栽跟头，根因是同一个：**复杂契约修复时缺乏 "全篇贯穿性" sweep**。每轮都在修改局部表述，但忘了把同主题的所有引用点（字段表 + 示例 + 不变量段 + 跨节引用 + 对偶分支 + 跨文档引用）一次性贯穿对齐。

预防规则：每次 review 修复**至少**包含一个 "sweep 自检表" 步骤 —— 在写 commit 前列一个 check list：

```
[ ] 字段表里所有相关字段的 description 都已同步
[ ] 不变量段 / 约束段已同步
[ ] 示例 JSON 已同步（真实示例 + placeholder 示例都要查）
[ ] 同文档其它节对此契约的引用已同步（同文档 grep）
[ ] 跨文档引用已同步（V1 接口 / 时序图 / 数据库 / 架构 / 工程结构 / iOS 结构 / 节点规划 七份文档全 grep）
[ ] 对偶分支已 sweep（握手 vs 断连、加入 vs 离开、写 vs 读、open vs close）
[ ] epics.md / story 文件里相关描述已同步（如有）
```

每一项都用 grep 验证（不是凭脑子记），确认无遗漏才 commit。
