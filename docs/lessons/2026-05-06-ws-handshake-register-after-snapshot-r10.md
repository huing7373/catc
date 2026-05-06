---
date: 2026-05-06
source_review: codex review r10 — /tmp/epic-loop-review-10-3-r10.md
story: 10-3-ws-网关骨架
commit: 8718b3f
lesson_count: 1
---

# Review Lessons — 2026-05-06 — WS handshake Register 顺序违反"事务性 reconnect"语义

## 背景

Story 10.3（WS 网关骨架）r10 codex review 唯一 P1：reconnect 路径下 `Gateway.Handle`
按 V1 §12.1 字面顺序先 `mgr.Register(session)` 再 `ListMembers + buildSnapshot +
WriteMessage(snapshot)`。`Register` 内部 reconnect 命中时会 evict + Close 旧 session。
若紧接的 snapshot 步骤 transient 失败（DB 抖动 / client 中途断），handler 走 close
1011 → 新 session 也死 → user **同时失去新旧两条连接**，违反 "transient 失败应可
容错" 的语义。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | reconnect 路径在新 handshake snapshot 完成前 evict 旧 session | high (P1) | architecture | fix | `server/internal/app/ws/gateway.go:236-298` |

## Lesson 1: reconnect 路径必须把"破坏性 evict"延后到所有可失败步骤之后

- **Severity**: high (P1)
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/app/ws/gateway.go:236-298`

### 症状（Symptom）

`Gateway.Handle` 校验通过后按"先 Register 再 snapshot" 顺序执行：

```
1. newSession(...)
2. mgr.Register(session)     // reconnect 命中 → evict + Close OLD
3. ListMembers + buildSnapshot + conn.WriteMessage(snapshot)  // 任一 transient 失败
4. close 1011                 // NEW conn 也死
```

reconnect + transient snapshot 失败 → user **既失去 NEW（被 close 1011）也失去 OLD
（被 Register 段 evict）**。在 V1 §12.1 钦定 "client 应对 1011 自动重连" 的背景下，
单次 transient 抖动让 user 走完整重连风暴。

### 根因（Root cause）

V1 §12.1 字面顺序"1. 创建 Session 2. 注册到 SessionManager 3. 同步推 snapshot
4. 启动 read/write goroutine"是从单 user 单连接 happy path 视角写的，**没考虑 reconnect
路径下 Register 自带破坏性副作用**（evict + Close OLD）。字面照搬 spec 顺序会把
"用户期望可容错的 transient 失败" 转化为 "destructive failure"。

更通用的根因：**"先做有副作用的 destructive 操作，再做可能 transient 失败的同步 IO"
是反模式**。正确顺序是把"可失败的同步 IO"放前面（失败时不破坏既有状态），把
"destructive 替换"放最后（一旦执行就 commit point，前置步骤已确保后续不会因 transient
原因失败）。这是事务性思维：把 reconnect 看成"用 NEW 替换 OLD" 的 transaction，
commit point 应在所有 prerequisite 都成功之后。

### 修复（Fix）

把 `mgr.Register` 推迟到 snapshot 写成功之后：

```
1. newSession(...)            // 仅构造对象，未进 manager 索引；OLD 仍活跃
2. ListMembers                // transient 失败 → close 1011；OLD 不动
3. buildSnapshot              // marshal 失败（防御）→ close 1011；OLD 不动
4. conn.WriteMessage(snapshot) // 写失败 → close 1011；OLD 不动
5. mgr.Register(session)       // **commit point** —— 此时 evict OLD + Close OLD
6. go readLoop / go writeLoop  // notifier 已 set，自闭路径安全
```

关键不变量：步骤 5 之前**任何**失败路径都**不**破坏 OLD session；步骤 5 一旦执行
就 commit reconnect 替换。spec 字面顺序的精神（"snapshot 必为新 conn 上第一条
authoritative msg"）不变 —— 在新 conn wire 上 snapshot 仍是第一条 frame。

不修 spec 文档：spec §12.1 5 步顺序是"happy path 概念顺序"；实装层为正确性做
局部偏离是合理的，注释里写明 rationale + 引用 V1 §12.1。

加 3 条防回归测试：
- `TestGateway_Reconnect_SnapshotTransientFail_OldSessionStillActive`：reconnect 同 room +
  ListMembers transient fail → close 1011 + OLD 仍在 manager 索引 + OLD.Send 不返
  ErrSessionClosed
- `TestGateway_Reconnect_SnapshotTransientFail_CrossRoom_OldSessionStillActive`：
  cross-room reconnect 变体
- `TestGateway_Reconnect_HappyPath_OldSessionEvicted`：happy path 不破坏 —— NEW 全成功
  仍按 reconnect 替换 evict OLD

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在实装 **任何"reconnect / replace / takeover" 类语义** 时，
> **必须** 把 destructive replace 步骤（evict + close 旧资源）放在 **所有可能 transient
> 失败的同步 IO 之后**，而不是按 spec 字面顺序排列。
>
> **展开**：
> - 把 reconnect 想象成 transaction：commit point = destructive replace；commit point
>   之前的步骤都必须能"软失败"（失败时旧资源仍可用）。
> - 检查清单 —— 写 reconnect handshake 时问自己 4 个问题：
>   1. 哪一步是 destructive 的？（evict / close / state 翻转）
>   2. destructive 步骤之后是否还有可能 transient 失败的同步 IO？（DB query / 网络写
>      / cache 操作）
>   3. 如果有，能不能把那些 IO 提前到 destructive 步骤之前？
>   4. 顺序调整后是否破坏 spec 的语义本意？（通常 spec 写的是 happy path 概念顺序，
>      实装层为正确性局部偏离合理）
> - spec 字面顺序与正确性发生冲突时，**正确性优先**，但**必须**在代码注释里写明
>   rationale + 引用 spec 章节，并加防回归测试覆盖"transient 失败时旧资源不被破坏"。
> - **反例 1**：handler 内顺序 `mgr.Register → ListMembers → buildSnapshot →
>   WriteMessage`，Register 自带 evict OLD 副作用 —— transient ListMembers/Write 失败
>   让 user 完全断线。
> - **反例 2**：cache replace 路径 `cache.Set(newVal) → 写 DB`，DB 失败时 cache 已
>   是新值但 DB 是旧值 → 后续读 cache 命中假新值。修法是先写 DB 成功再 cache.Set。
> - **反例 3**：file rename `os.Rename(new, target)` 后做 `validate(target)`；validate
>   失败时 target 已被 rename 覆盖、原始 target 已丢失。修法是 validate(new) → 再
>   rename。
> - **正例**：本次修复的 ws gateway handshake —— "snapshot 写完才 Register"。
> - **共同原则**：destructive 操作是 commit point，把它推到能推的最后；保持系统
>   在 commit point 之前**任何步骤失败都能回到失败前状态**。

## Meta: 本次 review 的宏观教训

spec 写的是"协议顺序"，实装写的是"代码顺序"，两者不是 1:1 对应。spec 关心的是
**消息在 wire 上的顺序契约**（如 "snapshot 必为第一条 authoritative msg"）；
实装关心的是**state 变更与 transient 失败的交互**（如 "Register 是 destructive
commit point"）。当二者一致时直接按 spec 写；当二者冲突时，spec 关心的语义本意
通常仍可保持（snapshot 仍是 NEW conn 上第一条 frame），但实装顺序需为正确性微调。

下次写"按 spec 顺序实装"类 story 时，Claude 应主动 challenge："spec 顺序里有
没有 destructive step 紧跟 transient IO？" 这是协议骨架类 story 的常见 pitfall —
spec 作者关心 happy path 字段顺序，可能没穷举 reconnect / replace 路径下的失败
组合。
