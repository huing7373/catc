---
date: 2026-05-06
source_review: codex review (Story 10-1 r9, /tmp/epic-loop-review-10-1-r9.md)
story: 10-1-接口契约最终化
commit: 9c82129
lesson_count: 3
---

# Review Lessons — 2026-05-06 — V1 协议骨架冻结声明的范围收窄 & epics.md planning artifact 同步

## 背景

Story 10.1 r6-r8 多轮 review 把 V1 §12 的 WS 协议契约改了三处关键语义（snapshot 必含 full roster / 节点 4 阶段 server-active 消息集 / SnapshotBuilder 失败走 close 1011），但 downstream Epic 10 / 11 / 12 的 planning artifact（`_bmad-output/planning-artifacts/epics.md`）没同步，造成跨文档漂移。codex r9 准确定位 3 处，本次 fix-review 在原 story scope（V1 §12 协议契约最终化）的"最后一公里"上同步 V1 范围声明 + epics.md Story 10.7 描述。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | "节点 4 三类消息" 范围声明过宽，与 Epic 11/12 既有 plan 冲突 | P1 (high) | docs/architecture | fix | `docs/宠物互动App_V1接口设计.md` §1 / §12.1.3 / §12.2 / §12.3 |
| 2 | epics.md Story 10.7 placeholder 描述 = `members: []`，与 V1 §12.3 钦定 full roster 漂移 | P1 (high) | docs/architecture | fix | `_bmad-output/planning-artifacts/epics.md` lines 1791-1800 |
| 3 | epics.md Story 10.7 SnapshotBuilder 失败 = `type=error, code=6005`，与 V1 §12.3 钦定 close 1011 漂移 | P1 (high) | docs/architecture | fix | `_bmad-output/planning-artifacts/epics.md` line 1798 |

## Lesson 1: WS 协议骨架冻结声明的"范围"必须按"具体 epic 阶段"显式收窄，不能用"节点 X 阶段"宽泛表述

- **Severity**: high
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §1 line 36 / §12.1.3 line 1309 / §12.2 line 1411 / §12.3 line 1694

### 症状（Symptom）

V1 §12.1.3 / §12.2 / §12.3 写"节点 4 阶段（Epic 10 ~ 13）服务端 → 客户端只会主动发送 room.snapshot / pong / error 三种消息"。但 epics.md Epic 11 Story 11.4 / 11.5 钦定加入 / 退出房间事务后 server 会广播 `member.joined` / `member.left`（节点 4 中段，正属"节点 4 阶段"），Epic 12.4 client 端接收 `member.joined` / `member.left` 也属节点 4。downstream 实装如果按 V1 字面落地，Epic 11 的 `member.joined` 推送会变成"未识别消息" 走 log warn 路径——Epic 12.4 收不到合法 member 推送，房间页 member 同步链路断裂。

### 根因（Root cause）

"节点 4 阶段"是个宽泛标签（节点 4 跨 Epic 10 / 11 / 12 / 13 四个 epic）；本 story（Story 10.1）只冻结 Epic 10 阶段（Story 10.1 ~ 10.7）的协议骨架，**不**冻结 Epic 11 / 12 / 13 的业务消息扩展。把"Epic 10 阶段冻结的 active message set"误写成"节点 4 阶段冻结"，相当于本 story 越权冻结了三个 downstream epic 的消息集合，与 §1 已有的"协议骨架在 Epic 10 冻结，业务消息按 epic 顺序逐步冻结"声明自相矛盾。

### 修复（Fix）

1. V1 §12.1.3 line 1309（snapshot 必发为第一条消息附注）：把"节点 4 阶段"改为"**Epic 10 阶段**（即本 story 范围 / Story 10.1 ~ 10.7）"，并显式列出 Epic 11 / 14 / 17 各自锚定 story 与可扩展的消息（`member.joined` / `member.left` / `pet.state.changed` / `emoji.received`）
2. V1 §12.3 line 1694（server → client 消息集合冻结声明）：同样收窄到 Epic 10 阶段；显式声明 Epic 11 起按 §X.1 锚定 story 合法扩展
3. V1 §12.2 line 1411（client → server 消息集合冻结声明）：同样收窄到 Epic 10 阶段；显式声明 Epic 17 起 `emoji.send` 合法加入 client-active 集合
4. V1 §1 line 36 之后追加一条：明示 server / client active message set 的"Epic 10 阶段冻结"语义 + 后续 epic 按 §X.1 story 合法扩展

```diff
- > 节点 4 阶段（Epic 10 ~ 13）服务端 → 客户端**只**会主动发送 ...三种消息
+ > **Epic 10 阶段**（即本 story 范围 / Story 10.1 ~ 10.7）服务端 → 客户端**只**会主动发送 ...三种消息（Epic 11 起按 Story 11.1 / 14.1 / 17.1 锚定的语义合法扩展）
```

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **写"协议骨架冻结声明"时**，**必须** **把冻结范围限定到当前 story 所属的具体 epic 阶段**，不要用"节点 X 阶段"这种跨多个 epic 的宽泛标签。
>
> **展开**：
> - 节点（node）是产品里程碑层级的概念（节点 4 = Epic 10 / 11 / 12 / 13 全部），epic 才是 story 的实装载体；冻结声明的范围**必须**与 story 的 scope 边界（即所属 epic）对齐
> - 如果跨 epic 的消息集合存在演进（Epic 11 加 member.joined / Epic 14 加 pet.state.changed / Epic 17 加 emoji.received），冻结声明里**必须**显式列出这些后续锚定 story（如 "Epic 11 起按 Story 11.1 锚定 member.joined"），让 reviewer 一眼就能验证"本 story 没越权冻结后续 epic"
> - **反例**：「节点 4 阶段（Epic 10 ~ 13）只发三种消息」—— 把 4 个 epic 的消息集合都钉死在本 story 锚定，Epic 11 的 member.joined 广播立刻变非法，downstream 实装无路可走
> - **正例**：「Epic 10 阶段（本 story 范围 / Story 10.1 ~ 10.7）只发三种；Epic 11 起按 Story 11.1 锚定 member.joined / member.left 加入消息集合」—— 范围收窄 + 显式扩展点

## Lesson 2: 协议契约文档改了，planning artifact 必须同步——不可只改一边

- **Severity**: high
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `_bmad-output/planning-artifacts/epics.md` Story 10.7 (lines 1791-1800)

### 症状（Symptom）

V1 §12.3 字段表 r8 已经把 placeholder 实装从 `members: []` 改成"`SELECT * FROM room_members` 全表"，并附 placeholder 示例（房间已有 2 成员场景）。但 epics.md Story 10.7 的 AC 仍写 `placeholder = {room: {memberCount: 0}, members: []}` + 单测期望"返回 placeholder snapshot" + 集成测试断言"members = []"。两份 source of truth 一旦给到 dev，dev 必然要么跟 V1 实装出 full roster（让 epics.md 集成测试失败），要么跟 epics.md 实装空 roster（让 V1 钦定的 placeholder 行为破裂、Epic 11/12 client 解析会被空 roster 误清空已加载的 member 缓存）。

### 根因（Root cause）

R6-R8 review 几轮迭代里，contract 在 V1 §12.3 不停修正语义（placeholder 必须返回 full roster 才能避免 client roster 误清空），但 fix 链路只动了 V1 协议契约文档，没把 epics.md 这种 planning artifact 同步——**忘了 planning artifact 也是 dev 看的 source of truth**。Story 10.1 的本质是"WS 协议契约最终化"，契约一致性的最后一公里就是把 V1 钦定的语义同步到 epics.md 的对应 story AC，否则"协议最终化"的目的（让 dev 单一权威照做）就没达成。

### 修复（Fix）

修 epics.md Story 10.7 的"实现是 placeholder"描述：

```diff
- 实现是 placeholder：返回 `{room: {id: roomID, maxMembers: 4, memberCount: 0}, members: []}`
+ 实现是 placeholder：执行 `SELECT * FROM room_members WHERE roomId=?` 单表查询，把全部成员行映射成 members[]；丰富字段（nickname / pet.petId 等）允许空字符串降级；memberCount 必须等于 len(members)，禁止写死为 0
```

集成测试断言同步：

```diff
- WS 客户端连接 → 立即收到 placeholder room.snapshot 消息（room.id = 请求的 roomID, members = []）
+ WS 客户端连接（roomId 已在 room_members 中预置 ≥2 条 fixture 成员行）→ payload.members 长度 = fixture 行数（禁止断言 members: []）；丰富字段允许空字符串降级
```

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **改了 V1 协议契约文档（如 §12.3 字段表 / placeholder 行为）后**，**必须** **同步 sweep `_bmad-output/planning-artifacts/epics.md` 中所有引用该契约的 story AC**（描述 + 单测期望 + 集成测试断言），保证两份文档对 dev 给出**单一权威**的指令。
>
> **展开**：
> - "协议契约最终化"类 story（如 10.1 / 11.1 / 14.1 / 17.1）的 scope 天然包含"保证 V1 与 planning artifact 对齐"——这是契约一致性的第一性目标，不是 scope creep
> - sweep 命令推荐：`grep -n "<契约关键词>" _bmad-output/planning-artifacts/epics.md`（如本次 sweep 了 `memberCount: 0` / `members: \[\]` / `code=6005` / `Epic 10 ~ 13`），确认所有命中点都对齐 V1 钦定语义
> - **反例**：r6-r8 已经在 V1 §12.3 反复改 placeholder 语义到 full roster，但每轮 fix 只看 V1 文档自洽，不 grep epics.md ——结果 r9 codex 自动 grep 把漂移挖出来；如果不修，Story 10-2 ~ 10-7 dev 阶段会看到两份相互矛盾的指令，必然实装一边掉一边
> - **正例**：每次改协议契约文档（V1 / 数据库设计 / 时序图）后，固定 sweep 一次 epics.md 的对应 story AC + 关联 lesson，把语义对齐作为最后一步

## Lesson 3: SnapshotBuilder 失败走 close 1011 vs error 6005——bootstrap 失败语义必须用 close 而非 error 消息

- **Severity**: high
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `_bmad-output/planning-artifacts/epics.md` Story 10.7 line 1798

### 症状（Symptom）

V1 §12.3 line 1676 钦定"握手后初始 room.snapshot 构建失败 → close 1011（reason=`snapshot build failed`）"，rationale 在 V1 文档已写明：room.snapshot 是握手成功后的第一条 authoritative 消息，构建失败若仅推 error 而保持连接，client 会永远等待一个永不到达的 snapshot，房间页无法初始化、auto-reconnect 也不触发。但 epics.md Story 10.7 单测仍写"SnapshotBuilder 抛 error → Session 收到 error 消息（type=error, code=6005 房间状态异常）"。如果 dev 按 epics.md 写测，server 实装会保持错误的 bootstrap fail 行为，破坏 V1 钦定的房间页初始化语义。

### 根因（Root cause）

"WS error" 这条路径在 V1 §12.3 r4 之后已经语义分化了：
- **运行时业务错误**（连接已可用、业务侧出问题但不致死，如 emoji.send 失败 / Story 11.x / 14.x 的状态错误推送）→ 推 `type=error` 消息，连接保留
- **bootstrap 失败 / 致死错误**（握手前后无法继续提供服务，如初始 snapshot 失败 / 内部 panic）→ close 连接（1011）

epics.md Story 10.7 的旧描述是 r4 之前写的，把所有 error 都走 type=error 路径，没跟上 V1 r4 之后的语义分化。

### 修复（Fix）

修 epics.md Story 10.7 单测描述 line 1798：

```diff
- edge: SnapshotBuilder 抛 error → Session 收到 error 消息（type=error, code=6005 房间状态异常）
+ edge: SnapshotBuilder 抛 error → Session 不走"推 type=error 消息"路径，而是被 close 1011（reason="snapshot build failed"）—— 测试 close frame 的 code 与 reason，不测 error 消息 message 字段值（错误码 6005 保留给 Story 11.x / 14.x 业务流程的运行时状态错误推送，不用于初始 snapshot 失败场景；锁定 V1 §12.3 §12.1 close code 表 1011 行 + §12.3 "snapshot 构建失败的处理路径" 注）
```

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **设计 / review WS 错误处理路径时**，**必须** **区分"bootstrap 失败"（走 close）与"运行时业务错误"（走 type=error 消息）两条语义路径**，不能把所有错误都塞进 type=error 消息。
>
> **展开**：
> - **bootstrap 失败**：握手成功后到 first authoritative message（如 room.snapshot）之间的失败 → close（1011 内部错误 / 4xxx 业务级拒绝）。理由：first authoritative message 是 client UX 初始化的依赖，缺它 client 没法进入正常态；保留连接 + 推 error 会让 client 永远卡 loading
> - **运行时业务错误**：连接已可用、first authoritative message 已发送之后的非致死业务错误（如 emoji.send 失败 / 房间状态异常推送）→ type=error 消息 + 保留连接。理由：client 已能正常运作，只是某次操作失败，连接复用避免重连开销
> - **反例**：「SnapshotBuilder 失败 → 推 error code=6005，保留连接」—— 让 client 卡在等 snapshot 的 loading 态、无 reconnect 触发、UX 永久死锁
> - **正例**：「SnapshotBuilder 失败 → close 1011；6005 仅用于运行时房间状态错误推送（如 Story 11.x leave 时房间已 closed 的提醒）」—— bootstrap 失败靠 close 触发 client 的 transient network failure 重连逻辑，6005 走业务路径用于已活连接

---

## Meta: 本次 review 的宏观教训

三条 finding 都指向同一个思维漏洞：**协议契约文档（V1）改完后，没把 planning artifact（epics.md）当成同等权威的 source of truth 来同步**。R6-R8 三轮 review 都集中在 V1 §12.x 内部自洽，没人 grep epics.md 验证跨文档漂移，直到 r9 codex 自动 grep 把 3 处漂移挖出来。

**契约最终化类 story（10.1 / 11.1 / 14.1 / 17.1）的 fix-review 流程必须包含的最后一步**：

```
sweep _bmad-output/planning-artifacts/epics.md 全文
  vs 本轮改动的契约关键词（用 grep 命中所有引用点）
  → 确认每个命中点都对齐 V1 最新语义
  → 漂移点必须在同一 commit 同步修复（不要拖到下一个 story）
```

否则下一个跑 epic-loop 的 sub-agent 会拿着 stale planning artifact 实装出错误行为。
