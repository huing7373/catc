---
date: 2026-05-08
source_review: codex review round 13 of Story 11.1（接口契约最终化）
story: 11-1-接口契约最终化
commit: f35c9e5
lesson_count: 1
---

# Review Lessons — 2026-05-08 — 协议契约的步骤顺序必须可被现有 primitive 实装；step 顺序设计要校验 primitive capabilities

## 背景

Story 11.1（接口契约最终化）codex review 第 13 轮针对 r10 + r12 后的 §10.5 leave 接口步骤序列做"实装可行性"复核。codex 指出 §10.5 当前钦定的"step 7 broadcast `member.left` → step 8 close 4007 leaver Session → step 9 HTTP 200"顺序与 Story 10.5 落地的 `BroadcastToRoom` primitive 不兼容：该 primitive 走 `ListSessionsByRoomID` fanout 全部 session，没有 `excludeUserID` 参数；先 broadcast 后 close 时 leaver Session 仍在 SessionManager 列表，broadcast 会把 `member.left` 投递给 leaver 自己 —— 与 §12.3 "leaver 不收自己 member.left（fanout 列表中物理排除离开者）" 语义直接矛盾。一条 P1（提示）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | §10.5 step 7 broadcast → step 8 close 4007 顺序与 Story 10.5 落地的 BroadcastToRoom primitive 不兼容（无 excludeUserID 参数） | P1 | architecture（protocol-vs-primitive consistency） | fix | `docs/宠物互动App_V1接口设计.md` §10.5 / §12.1 / §12.3、`docs/宠物互动App_时序图与核心业务流程设计.md` §12.2、`_bmad-output/implementation-artifacts/11-1-接口契约最终化.md` |

## Lesson 1: 协议契约必须可被现有 primitive 实装；step 顺序设计要先校验 primitive capabilities

- **Severity**: P1
- **Category**: architecture（protocol-vs-primitive consistency）
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §10.5 服务端逻辑步骤 7-8-9（行 1544-1546）+ §10.5 close 顺序 rationale 段 + §12.1 close code 4007 行（行 1720）+ §12.3 `### 成员离开` 触发段（行 2062）+ 关键约束段（行 2091, 2093）；`docs/宠物互动App_时序图与核心业务流程设计.md` §12.2 mermaid + §13.3 引用；`_bmad-output/implementation-artifacts/11-1-接口契约最终化.md` §10.5 spec block + 关键约束段 + AC checklist 表

### 症状（Symptom）

§10.5 leave 接口钦定的步骤顺序（step 7 broadcast → step 8 close 4007 → step 9 HTTP 200）与 Story 10.5 已落地的 `BroadcastToRoom` primitive 不兼容：

- `BroadcastToRoom(roomID, msg)` primitive 实装是"调 `ListSessionsByRoomID(roomID)` 拿到全部该房间在线 session → 给每个 session 投递 msg"，**没有 excludeUserID 参数**
- 钦定顺序下，step 7 broadcast 时 leaver Session 还没被 close（step 8 才执行），仍在 SessionManager 注册表里
- 因此 `ListSessionsByRoomID` 会返回 leaver 自己的 Session，broadcast 把 `member.left` 投递给 leaver
- 与 §12.3 `### 成员离开` 关键约束 "广播范围：仅该房间内当前在线的其他 Session（不含离开者自己）" + "server 实装上应从 fanout 列表中排除离开者自己的 Session" 直接矛盾

文档的契约语义（leaver 物理上收不到自己的 member.left）与现有 primitive 能力（无法 exclude 调用者）不一致，Story 11.8 实装时无法不扩 primitive 即满足契约。

### 根因（Root cause）

**契约设计时只考虑"逻辑语义自洽"，忽略"用现有 primitive 能否 mechanically 实现该语义"**。具体思维漏洞链：

1. **r2 锁定 close 顺序时只想到"避免 leaver 拖到心跳超时仍收广播"**：r2 把 step 8 (close 4007) 放在 step 7 (broadcast) 之后，rationale 写为"避免 leaver 在 HTTP 200 后仍持有 WS 收到本次 member.left"。这句话本身就**语义矛盾** —— 既然 broadcast 在 close 之前执行，leaver Session 还在列表里，正是 broadcast 把 member.left 投递给 leaver；close 4007 在 broadcast 之后只能管"未来其他广播不再到达"，**管不到本次 member.left**。
2. **依赖未实装的"fanout 自动跳过"假设**：原契约段写"fanout 物理上跳过 leaver Session"——这句话假设 `BroadcastToRoom` primitive 提供"自动跳过调用者"或"excludeUserID 参数"能力，但 Story 10.5 落地实装并没有这个能力；契约文档相当于在描述一个不存在的 primitive 行为。
3. **跨 story 边界的能力假设没核对**：协议契约 story（11.1）写规则、primitive 实装 story（10.5）已落地、protocol consumer story（11.8）尚未实装。11.1 写规则时没去翻 10.5 落地的 primitive 签名，假设 primitive "应该"提供 excludeUserID（没有任何文档承诺过）。
4. **step 序号本可以提示问题**：r12 review 后 step 7 (broadcast) 紧挨 step 8 (close)，序号上"先 broadcast 后 close"原本就是反直觉的（close 是 cleanup，cleanup 通常先于 follow-up event）；但因为 r2 当时把 close 列为"独立 best-effort cleanup"且把 broadcast 看作"业务事件"，序号顺序被锁定后没有人质疑。

**蒸馏出的元教训**：契约层 step 顺序设计**必须**先列举该顺序对每个 step 调用的 primitive 提出了什么 invariant，再去 grep 现有 primitive 是否提供该 invariant。如果某 step 顺序要求 "primitive X 自动跳过 caller"，**先去看 primitive X 的签名 / 已落地实装是否有该参数**，没有则要么扩 primitive，要么改契约 step 顺序绕过该需求。

### 修复（Fix）

**路径选择**：路径 A（改契约步骤顺序，无需扩 primitive，改动最小）。

- **路径 A（采用）**：把 step 7 (broadcast) 和 step 8 (close 4007) 顺序对调 → 先 close 4007 + unregister leaver Session（新 step 7），再 broadcast member.left（新 step 8）。这样 broadcast 时 leaver Session 已不在 SessionManager 列表，`ListSessionsByRoomID` 返回列表物理上不含 leaver，fanout 自然跳过 leaver —— **无需任何 BroadcastToRoom primitive 修改**。
- **路径 B（不采用）**：要求 BroadcastToRoom primitive 加 `excludeUserID` 参数 / 扩展为新 primitive `BroadcastToRoomExcept(roomID, excludeUserID, msg)`。代价：扩 primitive，跨 story 改动（10.5 已 done 还要回工 + 11.8 实装时用新 primitive）；优点：保留原 step 顺序。**ROI 不匹配**，路径 A 显著更小。
- **路径 C（不采用）**：契约钦定 server 实装层在 fanout 包装里手动 skip leaver session（"调 BroadcastToRoom 之前先在 fanout 列表里过滤 leaver"）。实装上 hacky，每个 broadcaster 都要自己 skip caller，比路径 A 干净度差。

**路径 A 是否破坏 r10 锁定的 "HTTP 200 是 authoritative" 不变量？答：不破坏**：

- HTTP 200 仍是 authoritative success signal（contract layer 不变）
- close 4007 仍是 best-effort cleanup（leaver client 收到 4007 时是协议确认；收不到时 client 仍按收到 HTTP 200 时 tear down，不依赖 4007）
- close 4007 提前到 broadcast 之前，**只**改变 server-side 内部时序（broadcast fanout 时 leaver session 已 unregister），**不**改变 client-perceived 协议契约

**改动**：

1. `docs/宠物互动App_V1接口设计.md` §10.5 服务端逻辑：
   - 旧 step 7 broadcast / 旧 step 8 close 4007 顺序对调 → 新 step 7 close 4007 + unregister leaver Session / 新 step 8 broadcast `member.left`
   - 在新 step 7 / 8 描述中明确写"step 7 必须先于 step 8"+ rationale（BroadcastToRoom primitive 不带 excludeUserID 参数 → 必须靠 unregister 物理排除 leaver）
   - 事务边界规则段同步钦定 "步骤 7（close 4007 + unregister）必须先于步骤 8（broadcast）"（r13 锁定）
   - close 顺序 rationale 段更新："close 4007（步骤 7）"（不再是"步骤 8"），server 端职责扩展为双重：(a) 让 broadcast fanout 列表不含 leaver；(b) 立刻让 leaver 不再收后续广播
   - §12.1 close code 4007 行更新触发条件描述（"先 close 后 broadcast"）+ 更新 step 引用
   - §12.3 `### 成员离开` 触发段 + 关键约束段更新 step 引用（step 7 close + step 8 broadcast，step 7 在 step 8 之前）+ 明示 "无需 BroadcastToRoom primitive 提供 excludeUserID 参数"
2. `docs/宠物互动App_时序图与核心业务流程设计.md` §12.2 leave mermaid：
   - mermaid 节点顺序对调（CloseLeaverConnection / unregister 节点提到 BroadcastToRoom 之前）
   - Note 段顺序 + rationale 重写："close 4007 + unregister leaver Session → broadcast member.left → HTTP 200"
   - §13.3 引用块更新 step 引用（"§10.5 步骤 7 close 4007"，原"步骤 8"）
3. `_bmad-output/implementation-artifacts/11-1-接口契约最终化.md` §10.5 spec block + 关键约束段 + AC checklist 表：
   - 同样 step 顺序对调
   - 关键约束段引用 step 7（close）/ step 8（broadcast）+ 强调"无需扩 primitive"

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写跨 story 协议契约（含 step 顺序）** 时，**必须** **逐 step 列出该步骤对其调用的 primitive 提出的 invariant，然后 grep 该 primitive 已落地实装的签名 / 行为，确认 invariant 真的 mechanically 成立**；不允许"假设 primitive 应该有 X 能力"作为契约推理基础。
>
> **展开**：
>
> - 协议契约 story（如 epic-N.1）通常写规则时，被规则约束的 primitive 实装 story 可能 (a) 已落地（必须翻签名），(b) 同 epic 后续 story（必须把 invariant 加进 acceptance criteria），或 (c) 跨 epic 后续 story（必须在契约文档显式标注 "依赖 primitive X 提供 Y 能力，由 Story Z 实装"）
> - "step 顺序" 是 architecture-level 决策，不是 cosmetic：每个 step 顺序选择都对应一组对 primitive 的 capability 约束。如果钦定 "step A → step B" 顺序，要问"如果反过来是否会破坏正确性？" 如果答案是"会破坏，因为 primitive X 没有 capability Y"，必须在 step 顺序段显式钦定该顺序 + 引用 primitive X 不带 capability Y 的事实
> - **架构层和实装层共享词汇但语义不同**：契约层 "fanout 物理上跳过 X" 不是自动成立的语义 —— 需要 primitive 实装层提供 "exclude" 参数 / 调用方先 unregister X 再 fanout 等具体机制；契约层不能写"物理上跳过"假装机制自然存在
> - **跨 story 协议设计模板**（推荐）：
>   1. 列出该接口调用的所有 primitive（如 `BroadcastToRoom` / `Unregister` / `Commit Tx` / `WriteHTTPResponse`）
>   2. 每个 primitive 旁边记 "签名" + "已落地实装的 invariant"（grep 实装 / story spec）
>   3. 对每个 step 顺序选择，问"该顺序对每个 primitive 提出什么前置条件？"
>   4. 如果前置条件不在 primitive 已有 invariant 内 → 选项 (a) 改 step 顺序 / (b) 扩 primitive（重 epic 决策） / (c) 用 helper 包装层 hack（不推荐）
> - **反例 1**：r2 锁定 leave step 顺序时，只想到"避免 leaver 拖到心跳超时仍收广播" 这一面，没问"先 broadcast 后 close 时 leaver 能否真的被 broadcast 跳过？"。如果当时翻 Story 10.5 BroadcastToRoom primitive 签名（无 excludeUserID 参数），立刻能识别该顺序 broken。
> - **反例 2**：契约段写"fanout 物理上跳过 leaver Session" 这种 "物理上" 的表述时，必须紧接着回答"哪个 primitive 提供该跳过？参数名 / 实装路径是什么？"。如果回答不出，该表述就是空头支票。
> - **反例 3（特别危险）**：依赖"自然成立"的不变量。如"自然 fanout 不到 leaver" / "primitive 应该会 skip caller" / "实装时显然要 exclude 调用方"。所有"自然 / 应该 / 显然"都是危险信号 —— 必须落实到具体 primitive 签名 / 行为。
> - **跨 epic 协议契约必须考虑 backward compatibility**：如果协议契约 step 顺序约束某 primitive 提供能力 X，但该 primitive 已在 prior epic 落地（如 Story 10.5），扩 primitive 等同回工 prior epic story（可能违反"已 done story 不改"纪律）。优先选择"改契约 step 顺序绕过 primitive 限制"路径（路径 A），保留 primitive 不变（最小化 retroactive 改动）。
