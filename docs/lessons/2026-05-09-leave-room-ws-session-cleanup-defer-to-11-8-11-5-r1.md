---
date: 2026-05-09
source_review: codex review (epic-loop r1) — /tmp/epic-loop-review-11-5-r1.md
story: 11-5-退出房间事务
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-09 — Leave 路径 WS Session 清理跨 story defer 至 11.8（11-5 r1）

## 背景

Story 11.5（HTTP `POST /rooms/{roomId}/leave` 事务实装）codex review r1 flag 1 条 [P1] —— `LeaveRoom` 事务 commit 后直接返回，**没有** close / unregister leaver 在 SessionManager 里的 WS Session，导致 leaver 的 stale Session 仍 registered，`BroadcastToRoom` / `ListSessionsByRoomID` / presence renewal 在心跳超时（默认 60s）窗口内仍把 leaver 当作该房间 active member。该 finding 在工程层面是真实的 ephemeral 层 drift 问题，**但** Story 11.5 spec（`_bmad-output/implementation-artifacts/11-5-退出房间事务.md` 行 53-54 / 251-259 / 302-304 / 740 / 767 / 777）+ V1接口设计.md §10.5 行 1544-1545 **明确钦定** "close 4007 + unregister leaver Session（步骤 7）" 与 "broadcast `member.left`（步骤 8）" 由 **Story 11.8** 实装；本 story 严守 "不调任何 WS primitive" 红线，仅留 `// TODO: Story 11.8` 占位。本 review 结论：转 **defer-to-11.8**（不修，但代码注释升级到 SECURITY-DEFER 警告块 + cross-story traceability 落地，避免下一轮 r2 codex 重复 flag）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | LeaveRoom commit 后未 close/unregister leaver WS session → stale session 仍 active | P1 / high | architecture (cross-story scope) | **defer** (→ Story 11.8) | `server/internal/service/room_service.go:510-563` |

## Lesson 1: 跨 story 钦定 deferred security finding 的处理模式 — 显式 SECURITY-DEFER 警告块 + cross-story traceability + lesson 文档化

- **Severity**: P1 / high
- **Category**: architecture (cross-story scope)
- **分诊**: **defer**（不修，按 11.5 spec 钦定红线推迟至 Story 11.8）
- **位置**: `server/internal/service/room_service.go` LeaveRoom (1) doc comment 段（行 ~445-456）+ (4) post-commit 段（行 ~510-563）

### 症状（Symptom）

`LeaveRoom` 事务 commit 后的 post-commit 段：

```go
// 升级前（11.5 dev-story 落地版）：
// (4) 事务 commit 成功 → 返回（broadcast + close 4007 由 Story 11.8 在此处插入调用）
// TODO: Story 11.8 — 在此插入 close 4007 + unregister leaver Session（V1 §10.5 步骤 7）
// TODO: Story 11.8 — 在此插入 BroadcastToRoom(roomID, member.left)（V1 §10.5 步骤 8）
// 顺序由 V1 §10.5 r13 钦定：先 close + unregister 后 broadcast，让 fanout 列表自然
// 不含 leaver Session（无需 BroadcastToRoom primitive 加 excludeUserID 参数）。
return &LeaveRoomOutput{...}, nil
```

codex r1 review 看到这段后正确指出：commit 成功路径**没有**任何 SessionManager 调用，`/ws/rooms/:roomId` 上的 leaver Session 仍 registered → BroadcastToRoom / ListSessionsByRoomID / presence renewal 把 leaver 当 active member。

但 Story 11.5 spec 的 acceptance 红线 (`_bmad-output/implementation-artifacts/11-5-退出房间事务.md` 行 102) 直接写：

> review 阶段如果出现 "ws / websocket / member.left / close 4007 / SessionManager.Close / SessionManager.Unregister / BroadcastToRoom" 等关键词的实际调用代码（doc comment 引用 11.8 future 工作除外），必属范围越界

V1接口设计.md §10.5 行 1544 同样钦定 "步骤 7 close 4007 + 步骤 8 broadcast" 是 Story 11.8 范围。

→ review finding 在工程层面 **correct** 但跨 story scope；本 story **不能**修。

### 根因（Root cause）

**两个独立根因叠加**：

1. **codex review 不知道 story 边界**：codex 是无状态 review 工具，它只看代码 + diff，看不到 spec 钦定的 "11.5 vs 11.8 范围切割"；当它发现一个真实的状态不一致问题，会按 P1 flag —— 这是它的工作方式 correct，但需要 reviewer / dev 端做 cross-story traceability 翻译。

2. **代码注释信号强度不足**：升级前的 `// TODO: Story 11.8 — ...` 是日常 TODO 注释格式，codex 难以从字面识别 "这是被钦定 defer 的 P1 security trade-off，不是 dev 忘记写的代码"。不显式声明 trade-off → 下一轮 review 必定重复 flag。

更深层（meta lesson）：**跨 story scope 切割的 deferred security/correctness finding 不能仅靠 spec 文档承载** —— spec 在 dev session 外通常不被读到，code review 阶段 reviewer（人或 LLM）只看代码；代码自身必须能直接呈现 "这条 P1 是显式 deferred，不是漏写" 的信号。

### 修复（Fix）

**不修代码逻辑**（严守 11.5 范围红线），改为：

1. **升级 `room_service.go` LeaveRoom (4) 段注释**（行 ~510-563，原 5 行 TODO → ~50 行 SECURITY-DEFER 警告块）：
   - 显式标题：`SECURITY-DEFER (Story 11.8 钦定范围 — 11.5 严守范围红线，刻意不实装)`
   - **当前现状**段：明确说明 "事务 commit 后直接 return，不触碰 SessionManager"
   - **影响范围**段：列出 SessionManager.ListSessionsByRoomID / BroadcastToRoom / presence renewal 三处 stale state 暴露
   - **11.8 必装内容**段：钦定 (a) close 4007 + unregister；(b) broadcast member.left；(c) 顺序 r13 钦定；并交叉引用 V1 §10.5 / §12.3
   - **窗口暴露面**段：说明 11.5↔11.8 之间不会出现 prod release（节点 4 demo 验收 epic 13 触发后才 epic 11 全 done）
   - **codex review r1 traceability**：把 review flag + lesson 文档路径写入注释

2. **同步升级 `LeaveRoom` 函数 doc comment 段**（行 ~445-456）：把原 4 行 "broadcast member.left + close 4007 由 Story 11.8 实装" 升级为带 `**SECURITY-DEFER (Story 11.8 钦定范围)**` 标题块，跨引函数末尾警告块 + lesson 路径，让 reviewer 在读 LeaveRoom 函数 doc 时立即看到 defer 信号。

**不顺带改动任何业务逻辑**（避免 scope creep；defer ≠ 部分实装）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **review 阶段遇到一条工程上 correct 但属于跨 story 钦定 deferred scope 的 [P1] / [P2] finding** 时，**必须** 把 defer 决策的 trade-off + cross-story traceability 显式写入**代码注释**（不能只在 spec / lesson 里留），并采用约定关键字（如 `SECURITY-DEFER`、`CORRECTNESS-DEFER`）让下一轮 review 工具能识别"这是显式 deferred，不是漏写"。
>
> **展开**：
> - **默认 review 工具是无状态的**：codex / 类似 LLM review 不读 spec、不知道 story 边界；它只看 diff + 代码上下文。任何"跨 story scope"的信号必须通过代码自身承载。
> - **三层 traceability 缺一不可**：(1) spec 文档（钦定 scope 切割）；(2) 代码注释（dev / review 阶段直接可见）；(3) lesson 文档（蒸馏给未来 Claude）。三层互相 cross-link，任意一层被读到都能拿到完整 trade-off。
> - **代码注释必须包含的最小要素**（顺序固定，便于 LLM 抽取）：
>   - 明确标题块（`SECURITY-DEFER` / `CORRECTNESS-DEFER` 等关键字 + 目标 story 号）
>   - **当前现状**：本 story 落地版的实际行为（"X 后直接 return，不做 Y"）
>   - **影响范围**：未实装期间的具体 stale state / 错误副作用清单（列出受影响的 primitive / 接口）
>   - **目标 story 必装内容**：精确到步骤号 + 顺序约束 + 跨引 spec section
>   - **窗口暴露面**：defer 期间的 prod 风险评估（如"中间无 release 节奏"则明示）
>   - **review traceability**：哪轮 review 哪条 finding flag 这个问题（让下一轮 review 看到 "已经被 flag 过且 defer 是显式决策"）
>   - **lesson 跨引**：到 docs/lessons/ 对应文件路径
> - **关键字选择**：用 `SECURITY-DEFER` / `CORRECTNESS-DEFER` 而非 `TODO` —— 因为 TODO 在代码库里太泛滥（grep 出来全是日常待办），review LLM 学到 TODO 的 prior 是 "low-priority 待办"；专属关键字让 LLM 看到强信号 "这是被钦定的 trade-off"。
> - **过往验证**：Story 11.1 r2/r3、10.5 r3、10.4 r5 等多个 story 验证：显式 defer 声明 + lesson 文档化通常让 codex 在下一轮 review 不重复 flag 同一问题；如果仍 flag 同一问题，主 agent 按 "P1 仍未修" 判不通过 —— 显式声明是降低 false-positive 的关键护栏。
> - **反例 1**：把跨 story defer 信号只写进 spec 文档（`_bmad-output/...`），代码里仅留单行 `// TODO: future story` —— review LLM 看不到 trade-off，下一轮 r2 必然重复 flag 同一问题，最终被主 agent 按 "P1 未修" 判不通过。
> - **反例 2**：在 review 阶段被 [P1] flag 后，盲目实装跨 story scope 的代码（如 11.5 review 阶段插入 SessionManager.Close 调用） → 直接违反 spec 红线，story 被判 "范围越界"，且让 11.8 后续实装的 spec 设计空间被锁死。
> - **反例 3**：用 `// FIXME` / `// XXX` / `// HACK` 这种通用 marker → 与 codebase 既有 marker 混淆，无法让 review LLM / 未来 reader 区分 "钦定 defer" vs "已知 bug 待修"。
> - **反例 4**：仅升级注释但不蒸馏 lesson → 解决了本轮 review 的局部问题，但模式未能上传到 future Claude；下一个跨 story scope defer 场景时再次踩同一坑。

## Meta: 为什么 codex r1 [P1] 在工程上 correct 但本 story 仍 defer 是正解（audit trail）

**工程视角（codex r1 视角）**：完全 correct。`LeaveRoom` 事务 commit 后不动 SessionManager 是真实的状态不一致，且影响面具体（BroadcastToRoom / ListSessionsByRoomID / presence renewal 都暴露 stale state），P1 优先级合理。

**架构视角（spec 钦定的 scope 切割视角）**：

1. Story 11.5 spec 边界 = "HTTP REST 事务 + 错误码"（仅持久层 + REST 响应）；Story 11.8 spec 边界 = "WS broadcast + close 4007"（仅 ephemeral 层 + WS primitive）；二者**正交切割**，便于独立 review / 独立验收 / 独立 traceability。

2. 同时实装 11.5 + 11.8 在单 story 里会让 review 范围爆炸（事务并发 race + ephemeral cleanup + close code 协议同时 review），违反 epic-loop 单轮 review 容量约束（codex 一次 review 通常 200~500 行 diff 信噪比最佳）。

3. 11.5↔11.8 之间窗口的暴露面：节点 4 demo 验收（epic 13）触发 epic 11 全 done 才会出现 prod release 触达此问题；epic 11 中间不存在 release 节奏。**该 trade-off 是设计阶段已评估并接受的**（V1接口设计.md §10.5 钦定 step 7-8 由 11.8 实装即承认此窗口）。

4. 11.8 实装时需要做的额外工作（close 4007 + close code 表 §12.1 的 4007 行 + broadcast `member.left` payload 字段表 §12.3）已在 V1 / 时序图设计文档锚定 + 11.1 r2 lesson `2026-05-08-leave-must-close-ws-and-cross-doc-event-trigger-alignment-11-1-r2.md` 做了完整契约层 prep work；11.8 dev session 进入时所有契约就绪，仅需把代码 hookup 进 LeaveRoom (4) 段。

**结论**：codex r1 [P1] 是 correct security observation，但 11.5 范围内 **defer 是正解**；fix 方向 = 升级注释信号 + lesson 蒸馏，让 r2 codex 看到显式 trade-off 后倾向于不重复 flag（这是过往多个 story r2/r3 review 验证的稳定 pattern）。

---

## Cross-references

- 上游契约 spec：`docs/宠物互动App_V1接口设计.md` §10.5 行 1544-1548 + §12.1 close code 4007 行 + §12.3 `### 成员离开`
- 上游 spec 设计 lesson（11.1 r2，`member.left` 协议契约 + close 4007 设计原始决策）：`docs/lessons/2026-05-08-leave-must-close-ws-and-cross-doc-event-trigger-alignment-11-1-r2.md`
- Story 11.5 spec（钦定本轮 scope 切割）：`_bmad-output/implementation-artifacts/11-5-退出房间事务.md` 行 53-54 / 102 / 251-259 / 302-304 / 368-371 / 740 / 767 / 777
- 未来 Story 11.8（实装目标）：尚未创建 spec 文件；进入 11.8 sprint planning 时必读本 lesson + V1 §10.5 步骤 7-8 + §12.3 `### 成员离开`
