---
date: 2026-05-08
source_review: codex review round 7 on Story 11.1 接口契约最终化（/tmp/epic-loop-review-11-1-r7.md）
story: 11-1-接口契约最终化
commit: 6a7866e
lesson_count: 2
---

# Review Lessons — 2026-05-08 — 接口默认 deny + 显式 allow（白名单 ACL）+ prose 步骤序与 mermaid sequenceDiagram 必须 zip 对齐

## 背景

Story 11.1 r6 锁定了 `member.joined` / `member.left` 广播相关的字段表 / 示例 / mermaid 三处对齐，但 r7 codex review 指出 r6 锁定的协议里仍有两处契约缺陷：

1. **`GET /rooms/{roomId}` 缺访问控制**：V1接口设计.md §10.3 行 1243 当时的注释是"节点 4 阶段**不**强制要求 caller 是该房间成员 —— 用户**可以**查询任意 roomId 的房间详情（用于'加入前预览房间'等未来场景），后续节点若需私有房间禁止非成员查看再加 ACL"。但 §10.3 响应体下发 `members[].nickname` / `avatarUrl` / `pet.petId` / `pet.currentState` 都是其他用户的隐私字段；同时数据库设计.md §6.6 钦定 `rooms.id` 是 BIGINT auto_increment（顺序号、可枚举）。两点叠加 → 任何认证用户都能用 `for roomId in 1..N` 暴力枚举 GET，抓全站房间成员关系（昵称 + 头像 + 宠物），形成显著的隐私 / security 攻击面。这不是"未来 ACL 增强"，是**当下就存在的契约级 regression**。

2. **§12.2 leave 时序图与 §10.5 钦定步骤序相反**：r5 修复阶段在 V1接口设计.md §10.5 钦定了步骤 5（提交事务）→ 步骤 6（broadcast member.left）→ 步骤 7（close leaver Session 4007）→ 步骤 8（HTTP 200 响应）；rationale 是"避免 leaver 在 HTTP 200 后仍持有 WS 还能收到本次 leave 触发的 member.left 广播"。但时序图设计.md §12.2 mermaid 仍画 `提交事务 → 返回 left=true → HTTP 200 → broadcast → close 4007` —— 顺序与 prose 反向。实装方若按 mermaid 写代码 / 测试，就会重现 r5 锁定试图根除的 post-leave race。

两条都是 P1/P2 docs 级缺陷，必须在 11.1 收官前修；同时 11.1 是 11.3 ~ 11.8 / iOS Epic 12 的强前置，下游实装直接依赖本 story 锚定的协议。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | GET /rooms/{roomId} 缺 ACL —— 任何认证用户可枚举全站房间成员隐私字段 | high (P1) | security | fix | `docs/宠物互动App_V1接口设计.md:1243-1325`（注释 + 服务端逻辑 + 错误码表） |
| 2 | §12.2 / §11.2 mermaid 步骤序与 §10.5 / §10.4 prose 反向（HTTP 200 在 broadcast / close 之前） | medium (P2) | docs | fix | `docs/宠物互动App_时序图与核心业务流程设计.md:459-465, 501-509` |

## Lesson 1: 接口默认 deny + 显式 allow（白名单 ACL）

- **Severity**: high (P1)
- **Category**: security
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1243`（GET /rooms/{roomId} ACL 注释 + 新增服务端逻辑步骤 1 + 错误码表 6004 行）

### 症状（Symptom）

`GET /api/v1/rooms/{roomId}` 在 r6 锁定状态下"任何认证 user 都能查任意 roomId"，且响应体包含其他用户的隐私字段（nickname / avatarUrl / petId / currentState）；数据库设计.md §6.6 钦定 `rooms.id` 是 BIGINT auto_increment（顺序号），攻击者认证后可暴力枚举 roomId 抓全站房间成员关系。`auth 中间件已通过 → 业务自动放行`这条隐含规则把"认证"等同于"授权"，跳过了"caller 与该资源的从属关系"校验。

### 根因（Root cause）

r5 / r6 review 阶段聚焦"成员名册 placeholder vs 真实路径"语义对齐，把 ACL 当作"未来 epic 的产品增强"延后了 —— 当时的注释明示"由对应 epic §X.1 story 加 ACL 校验，不在本 story 范围"。但本 story 是 **协议契约最终化** 的 freeze 点：契约一旦冻结，下游 11.6 实装会**严格按字面意思**写 handler（"不强制要求 caller 是该房间成员" → handler 不查 `users.current_room_id` 直接返回 → 上线即漏）。"未来加 ACL"不是技术延后选项，是把 security regression 写入冻结协议。

更深层根因：协议设计阶段"默认 allow + 未来按需加 deny"的思路是对**安全敏感接口**的反向 default —— 应当采用"默认 deny + 显式 allow（白名单 ACL）"基线：每个会下发其他用户字段的接口必须先回答"caller 与目标资源的关系是什么 → 哪种关系下 allow"，没显式 allow 就 deny。本接口 §10.3 已存在一个对称参照（§10.5 leave 步骤 1：caller 必须是该房间成员），ACL 模型直接沿用即可，零设计代价。

### 修复（Fix）

V1接口设计.md §10.3 三处协同改动（路径 A：强 ACL，复用 §10.5 步骤 1 的 caller-is-member 模型）：

1. 行 1243 注释：从"**不**强制要求 caller 是当前用户所在房间"改为"**强制要求** caller 是该房间的当前成员（即 `users.current_room_id == path roomId`），否则返回 6004"，并补 rationale（隐私字段 + BIGINT 顺序 ID 枚举攻击面）和"未来路径"声明（如需"加入前预览"由后续 epic 单开"轻量预览接口"或"高熵 roomId 改造"，**不**回退本 ACL）
2. 新增"#### 服务端逻辑"H4 块：步骤 1 查 `users.current_room_id` 不一致 → 6004；步骤 2 查 `rooms WHERE id = ?` 兜底 → 6001；步骤 3 查 `room_members` 聚合 + JOIN；步骤 4 返回。本接口为只读查询，不开 MySQL 事务
3. 行 1317-1325 错误码表：增加 `6004 用户不在房间中 | 服务端逻辑步骤 1：caller 不是该房间成员` 行；调整下方"注"，明示 6004 与 §10.5 leave 步骤 1 同源、统一语义；保留 `6001 房间不存在`改注释为"兜底场景"

故事 spec 文件 `_bmad-output/implementation-artifacts/11-1-接口契约最终化.md` 的 AC4 §10.3.2 / §10.3.4 块同步 —— 协议变更必须穿透到 story spec，否则下次 dev-story 仍会读到旧契约。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计 / 锚定任何下发其他用户字段（nickname / avatar / pet / status / location ...）的协议接口** 时，**必须** **先回答"caller 与目标资源的从属关系是什么 → 哪种关系下 allow"，没有显式 allow 路径就 deny + 返回业务错误码（如 6004 / 403）；不能把"认证"等同于"授权"**。
>
> **展开**：
> - 任何接口下发"非 caller 自己"的用户字段（含 nickname / avatarUrl / pet 状态 / motion / location / 任何隐私字段），都必须有显式的 **白名单 ACL** —— "caller 是房间成员"、"caller 是好友"、"caller 是房主"等，把允许关系写进 prose + 错误码表
> - **不要**把 ACL 描述成"节点 X 阶段不强制 / 未来再加"—— 协议冻结意味着下游 handler 严格按字面写代码，"未来再加"就是上线即漏
> - 资源 ID 的可枚举性是 ACL 缺失的放大器：BIGINT auto_increment / 短 numeric ID 路径（`/rooms/{id}` / `/users/{id}` / `/orders/{id}`）下，"任何认证用户能查任意 ID"等价于"全站脱库"。如果 ACL 模型一时没法落，**先**改 ID 形态（auto_increment → nanoid / UUID），把枚举攻击门槛抬上去
> - **设计阶段**对每个新接口建立"ACL 自检 checklist"：① 响应体含哪些非 caller 自己的字段？② caller 与目标资源的允许关系是什么？③ 不满足时返回什么错误码（不要静默成功 / 返回空对象，要业务错误码 + 触发条件 prose）？④ 错误码与同模块其他接口（如 leave）是否复用同一码统一语义？
> - **反例**：
>   - "节点 4 阶段不强制要求 caller 是该房间成员，未来 epic 再加 ACL" —— 协议冻结后下游 handler 跳 ACL 校验，上线即可被枚举
>   - "auth 中间件已通过就可以查任意 roomId" —— 认证 ≠ 授权，认证只回答"你是谁"，授权回答"你能不能看这个资源"
>   - "GET 接口是只读查询，不需要 ACL" —— 只读查询泄露的恰恰是隐私字段，比写入更危险（写入可被审计回滚，读取没法回滚）
>   - "假设攻击者拿不到合法 token 就枚举不了" —— 游客登录 / 任意已注册用户都可拿合法 token，认证门槛不挡 ACL 攻击

## Lesson 2: 协议步骤 prose 与 mermaid sequenceDiagram 必须 zip 对齐

- **Severity**: medium (P2)
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_时序图与核心业务流程设计.md:459-465, 501-509`（join + leave 两图）

### 症状（Symptom）

V1接口设计.md §10.5 leave 接口在 r5 修复阶段钦定步骤 5（提交事务）→ 6（broadcast member.left）→ 7（close 4007）→ 8（HTTP 200），rationale 是"避免 leaver 在 HTTP 200 后仍持有 WS 收到本次 leave 触发的 member.left 广播"。但时序图设计.md §12.2 mermaid 顺序是"提交事务 → 返回 left=true → HTTP 200 → broadcast → close 4007"，与 prose 反向。同样 §11.2 join 图也是"HTTP 200 → broadcast"在前，与 §10.4 步骤 8/9 反向。实装方若按 mermaid 写代码 / 测试，会让 leaver 在 HTTP 200 后还能收到本次 leave 的广播，重现 r5 锁定试图根除的 post-leave race。

### 根因（Root cause）

r6 修复时聚焦"§13.2 / §13.3 文字注解里 server → client active message set 加入 member.joined / member.left + 后置广播 / close 锚定"，**新增**了文字注解但没回头校验"§11.2 / §12.2 已有的 mermaid 是不是被这条注解推翻"。当前文档的 prose 与 mermaid 是两份独立维护的契约视图：注解里说"事务后广播 + close"是新加的，但 mermaid 里画的步骤序是 r5 之前的旧状态（HTTP 200 在前，post-commit 注释只是说"广播在事务后"，没明确广播与 HTTP 200 的相对顺序）。

更深层根因：mermaid sequenceDiagram 的箭头序就是契约的一部分，**箭头序 = 实装方按"行动者间消息发生顺序"读到的指令**。即使 prose 钦定步骤 1/2/3/4，mermaid 把这些消息按错误时序画出来，实装就会按错的那一份做。"prose 钦定 + mermaid 用 Note 注释 zip"不够 —— 必须让箭头本身的物理顺序（mermaid 行序）与 prose 步骤号 zip。

### 修复（Fix）

时序图设计.md §11.2 + §12.2 mermaid 同步重排（保留所有原箭头，仅调整顺序 + 增强 Note 文字）：

§12.2 leave 图（行 501-509）：

```diff
     Service->>MySQL: 提交事务
-    Service-->>API: 返回 left = true
-    API-->>Client: HTTP 200 退出成功
-    Note over Service,WSGateway: 事务提交后（post-commit）触发广播 + 主动关闭<br/>自 Story 11.1 起锚定
+    Note over Service,WSGateway: 事务提交后（post-commit）触发广播 + 主动关闭<br/>**严格顺序**：broadcast → close 4007 → HTTP 200<br/>（避免 leaver 在 HTTP 200 后仍持有 WS 收到本次 member.left；自 Story 11.1 r7 锚定，与 V1接口设计.md §10.5 步骤 6/7/8 zip 对齐）
     Service->>WSGateway: BroadcastToRoom(roomId, member.left)
     WSGateway->>Others: WS 推送 member.left {userId}
     Service->>WSGateway: CloseLeaverConnection(userId)
     WSGateway->>Client: WS Close 4007 "left room via HTTP"
+    Service-->>API: 返回 left = true
+    API-->>Client: HTTP 200 退出成功
```

§11.2 join 图（行 459-465）同理重排：broadcast 箭头放在 HTTP 200 之前，Note 增强为"顺序：broadcast → HTTP 200；fire-and-forget 仅 log，不影响 HTTP 200"，与 §10.4 步骤 8/9 zip 对齐。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **修改任何接口的服务端逻辑步骤序（prose 步骤 1/2/3/...）时**，**必须** **同步扫描所有相关的 mermaid sequenceDiagram，校验箭头物理顺序与 prose 步骤号 zip 对齐**，不能依赖 Note 文字注释当作"软对齐"。
>
> **展开**：
> - mermaid sequenceDiagram 的**箭头物理行序就是契约**：实装方读图按"上下行序 = 时间序"理解消息发生顺序，Note 注释帮助理解但不能推翻箭头序
> - prose 步骤序变更时（如"事务提交 → broadcast → close → HTTP 200"），**必须**同步把 mermaid 里所有相关箭头按 prose 顺序物理重排 —— 仅改 Note 不够
> - 协议文档双向 zip 对齐 checklist（任何 prose 步骤序变更必跑）：① grep 所有引用该接口的 mermaid 章节；② 校验 mermaid 行序 == prose 步骤号顺序；③ Note 文字与 prose 顺序一致；④ 跨章节（如 join + leave 的对称图）一并扫，避免单边修
> - 一个接口的 prose 顺序变更，往往牵连**多个**相关图（join 改了 → leave 也要扫；REST 改了 → WS 也要扫；rooms 改了 → 时序图设计 + 总体架构设计 都要扫）
> - **反例**：
>   - "我在 §10.5 步骤 prose 钦定 broadcast → close → HTTP 200，时序图里只在 Note 加了一句'事务提交后广播'就算齐了" —— Note 不锁箭头序，下游照旧画图次实装错
>   - "join 流程是 fire-and-forget 广播，HTTP 200 顺序无所谓" —— 即使 fire-and-forget，prose 步骤号已写"步骤 8 broadcast → 步骤 9 HTTP 200"，mermaid 必须 zip；否则下次有人按图加新步骤会基于错的顺序推断
>   - "改了 leave 但没改 join 图" —— 对称接口必同时扫，避免一致性 drift

---

## Meta: 本次 review 的宏观教训

两条 lesson 共同的"思维漏洞模板"：**协议冻结 = 下游实装严格按字面写代码 + 多份契约视图（prose / 字段表 / mermaid / JSON 示例）必须全部 zip 对齐**。

- Story 11.1 是节点 4 房间业务的契约 freeze 点，下游 11.3 ~ 11.8 / iOS Epic 12 / 后续审计回顾会**严格按本 story 锚定的协议字面写代码**。"未来再加 X / 节点 X 阶段不强制 X" 这类 prose 是契约级负债，会直接转成 handler 漏洞或测试缺口
- 协议文档的多视图 zip 对齐是 r1 ~ r7 review 反复出现的主题：r4 "create / leave 文档锚点"、r5 "pet currentState 不能 alias motion state"、r6 "字段表 vs JSON 示例 / prose vs mermaid"、r7 "ACL prose 缺失 + prose 步骤序与 mermaid 反向"。每一轮都是不同的具体表现，但根因同一类：**协议文档的多份视图（prose 钦定 / 字段表 / 错误码表 / JSON 示例 / mermaid 时序图 / story spec / 数据库 DDL）任何一份单独动 = 自动产生 zip 偏差**
- 长期对策（蒸馏维度）：建议在协议变更 PR 模板里加"zip checklist"——每条钦定 prose 改动必勾对应的 ① 字段表 ② 错误码表 ③ JSON 示例 ④ mermaid ⑤ story spec ⑥ DDL 是否同步；勾不齐就 block merge。
