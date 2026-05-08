---
date: 2026-05-08
source_review: codex review round 12 of Story 11.1（接口契约最终化）
story: 11-1-接口契约最终化
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-08 — ACL FOR SHARE 锁的精确边界 & mermaid 字段 payload 与 prose nullable 同步

## 背景

Story 11.1（接口契约最终化）codex review 第 12 轮针对 r9 引入的 "FOR SHARE 行锁保证 caller 在 HTTP 响应发出时仍是房间成员" 措辞进行 lock 边界精确化复核（P1），同时复核 r10 把 `pet` 字段全协议 nullable 后是否还有遗漏未同步的 mermaid 节点（P2）。两条都需要修。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | §10.3 / §8.8 "FOR SHARE 锁保证响应发出时仍是成员" 措辞混淆事务持续期与响应发出时刻 | P1 | docs | fix | `docs/宠物互动App_V1接口设计.md` §10.3、`docs/宠物互动App_数据库设计.md` §8.8 |
| 2 | §11.2 join mermaid `member.joined` payload 写死 `pet:{petId, currentState}`，未同步 r10 nullable | P2 | docs | fix | `docs/宠物互动App_时序图与核心业务流程设计.md` §11.2 |

## Lesson 1: FOR SHARE / 行锁的"保护期"边界 = 事务持续期，而非"响应字节发出时"

- **Severity**: P1
- **Category**: docs（concurrency-correctness 文档化语义）
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §10.3（行 1250-1252）、`docs/宠物互动App_数据库设计.md` §8.8（rationale 段）

### 症状（Symptom）

r9 引入的 ACL FOR SHARE 行锁说明把保护边界写为"caller **在 HTTP 响应发出时仍是房间成员**"。codex 指出措辞 imprecise：FOR SHARE 锁随事务 commit 释放，而 handler 通常的执行顺序是 `commit tx → 序列化 JSON → ctx.WriteJSON / flush response body`，因此存在一段 **commit → flush** 的 μs 量级窗口；并发 leave 完全可以在该窗口内 commit 其 DELETE，使得 roster 字节流到达对端时 caller 已不再是成员 —— 严格说"HTTP 响应发出时仍是成员"不成立。文档的强保证措辞与协议层实际能提供的保护边界错位。

### 根因（Root cause）

把"事务持续期内 ACL 成立"和"响应字节真正打到对端时 ACL 仍成立"两件事用同一句话描述，**忽略了 commit-after-response 这个窗口**。

具体思维漏洞：

1. **lock-lifetime 直觉**：写"FOR SHARE 阻止 leave 的 DELETE 在本读事务内 commit" 是正确的 —— 但 reader 容易把"事务内"自动延伸成"响应内"，因为多数人没明确想过 `commit → flush` 的边界。
2. **协议层 / 实装层的责任混淆**：协议层只管"事务持续期 ACL"（这是数据库 lock 能 mechanically 保证的边界）；"响应字节发出时 ACL"是端到端语义，需要应用层（handler 重构 commit-after-write 模式）+ client 防御才能实质保证。文档应清晰把这两者分开，承认 commit-after-response μs 窗口不可在协议层完全消除。
3. **想当然把"DB 层强保证"等同于"端到端强保证"**：DB lock 的覆盖范围以**事务**为单位；HTTP response delivery 以**字节流到达对端**为单位。两个时间轴不重合 —— commit 到 flush 之间还有 μs 量级的序列化 + socket 写出耗时。

### 修复（Fix）

**路径选择**：路径 A（措辞精确化 + 把 commit-after-response 窗口定性为 best-effort μs 量级，承认 race 不可消除但量级可忽略；责任划给 client 防御性 discard）。

- **路径 B**（强保证 + 实装代价：在 commit 前先序列化 + WriteJSON 后 commit）= 重构 handler 模式，需要改变所有 ACL-protected 读接口的 handler 流程，与 ROI 不匹配，**不推荐**。
- **路径 C**（双段 ACL：commit 后再 SELECT 一次再 flush）= 仍有窗口（虽然更窄）+ 代价大，**不推荐**。

**改动**：

1. `docs/宠物互动App_V1接口设计.md` §10.3：
   - 把"caller **在 HTTP 响应发出时仍是房间成员**"改写为"caller **在本读事务持续期间（含步骤 3 SELECT roster）仍是房间成员**"，明确锁释放时机 = 事务 commit；
   - rationale 段把"caller 在 HTTP 响应发出时仍是成员"改为"caller 在事务持续期间仍是成员"；
   - 新增独立段落"**关于 commit-after-response 残留窗口（r12 锁定）**"，承认 μs 量级窗口、说明为何不走路径 B（实装代价大）、把责任划给 client 防御性 discard（已 leave 后收到的 stale roster 直接丢弃 / 不写入 RoomView state）。
2. `docs/宠物互动App_数据库设计.md` §8.8 rationale 段：
   - 同步措辞修正（"HTTP 响应发出时" → "事务持续期间，含 SELECT roster；commit 时 lock 释放"）；
   - 同步追加"**关于 commit-after-response 残留窗口（r12 锁定）**"段落，引向 V1 接口设计 §10.3 同源说明，避免文档间口径漂移。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在文档化"DB lock / 事务保证 ACL / 一致性"时，**必须**精确写出 lock 的释放时刻（事务 commit / rollback），**禁止**把"事务持续期保护"等同于"HTTP 响应发出时保护" —— 这两个时间窗之间有 commit→flush 的 μs 量级残留窗口，是端到端语义而非协议层语义。
>
> **展开**：
> - 写到 lock-based 保证时，明确两个边界：① **lock 持有期** =（最常见）事务 begin 到 commit/rollback；② **应用层观察期** = commit 后 handler 序列化 + 写 socket 到字节流到达对端。这两个边界**不重合**，commit-after-response 窗口客观存在。
> - 如果文档需要描述端到端语义（"client 收到 X 时，server 状态仍是 Y"），不要假装 DB lock 能提供该保证，要么走"应用层重构 + commit-after-write"路径（明确写代价 + 列出受影响 handler），要么承认窗口存在 + 把责任划给 client 防御层（明确写 client 该做什么 fallback —— 如 discard / 重新 fetch / 接受 stale）。
> - 任何"FOR SHARE / FOR UPDATE 保证 X" 的句子，要 review 一下 X 是不是写得太外延 —— 锁的物理保证范围 = 事务持续期，外延到响应层 / client 层都需要额外机制。
> - **反例**：在 V1 接口文档写 "FOR SHARE 锁保证 caller 在 HTTP 响应发出时仍是成员" —— 看起来很自然，但严格 audit 时立刻被发现 commit→flush 窗口未被覆盖。正例："FOR SHARE 锁保证 caller 在本读事务持续期间（含 SELECT roster；commit 时 lock 释放）仍是成员；commit→flush 之间存在 μs 量级残留窗口，端到端实质安全由 client 防御性 discard 兜底。"
> - **同源跨文件**：本类语义如果在 V1 接口设计 + 数据库设计两份文档都写了，措辞修正必须**同步两份**，避免 audit 时两份文档之间口径漂移（已是本 Story 11.1 review 多轮的固定 pattern；本轮 r12 同样涉及 V1 §10.3 + 数据库 §8.8 的双 file 同步）。

## Lesson 2: 字段契约改动后必须扫描所有 mermaid / 时序图节点的 inline payload，不能只更新字段表

- **Severity**: P2
- **Category**: docs（cross-doc consistency）
- **分诊**: fix
- **位置**: `docs/宠物互动App_时序图与核心业务流程设计.md` §11.2 join mermaid（行 462）

### 症状（Symptom）

r10 已在 V1 接口设计文档（§10.4 join 响应字段表 + §10.3 GET roster 响应字段表 + WS 协议字段说明）把 `pet` 字段统一改为 `object | null`（pet-less 账号下发 null），但时序图设计.md §11.2 join 流程 mermaid 里 `member.joined` WS push 的 inline payload 仍写死 `{userId, nickname, avatarUrl, pet:{petId, currentState}}`，未同步反映 nullable。这种 inline payload 在 prose 字段表更新时容易被漏 —— 字段表 grep 命中，inline mermaid 字符串往往不命中。

### 根因（Root cause）

字段契约（如 nullable / 默认值 / 类型）改动时，**只搜索字段表 / TypeScript-style 字段定义** 而忽略**散落在 mermaid / 流程图 / 示例 JSON / 注释 inline 例子里的 payload 字符串**。这是 prose-mermaid zip 对齐 lesson（已在 Story 11.1 r6 / r7 锁定）在"字段类型 nullable 改动"上的延续。

特别是 mermaid 的 sequenceDiagram 节点里写 `pet:{petId, currentState}` 这种 inline payload，是 doc 渲染时给读者快速看出参数构成用的，但 grep `pet:{petId` 才能命中，搜 `pet` 字段表的 grep 不会命中 —— **审计该字段类型是否一致时，必须用 mermaid-payload-aware 的 grep pattern**。

### 修复（Fix）

`docs/宠物互动App_时序图与核心业务流程设计.md` §11.2：把 mermaid 的 `member.joined` 推送行从

```
WSGateway->>Others: WS 推送 member.joined<br/>{userId, nickname, avatarUrl, pet:{petId, currentState}}
```

改为

```
WSGateway->>Others: WS 推送 member.joined<br/>{userId, nickname, avatarUrl, pet: object | null}<br/>**注（r10/r12 锁定）**：`pet` 为 nullable —— pet-less 账号下发 `null`，<br/>非 null 时含 `{petId, currentState}`；与 V1接口设计.md §10.4 / §10.3 字段表对齐
```

并校验过 §12.2 leave mermaid（其 `member.left` payload 只含 `{userId}`，无 pet 字段，无需改）。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在改动字段契约（nullable / 类型 / 必填）时，**必须**用至少两种 grep pattern 扫描所有相关文档：① 字段名独立出现（找字段表 / 字段说明）；② 字段名后紧跟 `:` 或 `:{`（找 inline payload / mermaid / 示例 JSON）—— **缺一不可**，否则 mermaid / 示例 JSON 里的 inline payload 会被静默漏改。
>
> **展开**：
> - 字段契约改动后的 cross-file audit checklist：① 同名字段表（V1 接口设计 + iOS 客户端工程结构 + 时序图 + 任何 SDK / proto）；② mermaid `<br/>{...}` 里的 inline payload 字符串；③ 示例 JSON 块（` ```json ` 围栏）里该字段的硬编码值；④ 文档 prose 段里"假设 X 不为空"之类的隐式契约句式。
> - 执行 grep 时同时用 `<field>:` `<field>: object` `<field>:{` `pet:{` 这种带紧邻字符的 pattern，仅搜 `pet` 会被字段表 / fixtures / 注释里大量误命中淹没。
> - **反例**：r10 把 `pet` 字段表全协议改 nullable，但漏了 `时序图.md §11.2 mermaid` 里 `pet:{petId, currentState}` 这串 inline payload —— grep `pet` 命中太多噪音被忽略，没用 grep `pet:\{` 精确扫；r12 才捕到。**正例**：改字段契约时，准备一份具体的 grep regex list（包含 `<field>:`、`<field>:\{`、` `<field>` ` 在围栏 JSON 块中的命中），逐个跑过再宣告改动完成。
> - 这条与 Story 11.1 r6 / r7 锁定的"prose-mermaid zip 对齐" lesson 同源 —— 本质都是"散布在 mermaid / 示例里的契约 inline copy 容易漏更新"。

---

## Meta: 本次 review 的宏观教训

11.1 走到 r12 这一轮，留下来的两条都是 **r9 / r10 主修复完成后的"语义边界精度 / 跨文档同步"残余**，没有新的语义 bug —— 说明 r9 r10 已把核心 race 的解法收敛了，剩下都是**文档表述精度**问题。

这种"修主问题后的剩余 P1/P2"通常来自两个方向：

- **过度概括**：把一个本来精确的物理保证（FOR SHARE 锁覆盖事务期）写得太外延（覆盖到响应发出时），造成 audit 时立刻被发现夸大。
- **inline copy 漂移**：契约改动 propagate 到字段表 / V1 设计 / DB 设计都做了，但**散布在 mermaid / 流程图 inline string** 的同义 copy 漏更新。

应对：每次大修后，自己先做一遍"反向 audit"—— 假装自己是下一轮 reviewer，问自己：① 物理 lock 的覆盖范围有没有被措辞夸大？② 改动的字段在所有 inline payload 字符串里都同步了吗？跑两次 grep（字段表 grep + inline payload regex grep）。这能在自审阶段就把 r12 这种 review 找到的 finding 提前消化掉。
