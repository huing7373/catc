---
date: 2026-05-09
source_review: codex review r11 of Story 11.8 (epic-loop fix-review round 11)
story: 11-8-成员加入-离开-ws-广播
commit: 3f50b78
lesson_count: 1
---

# Review Lessons — 2026-05-09 — snapshot+act 模式必须 atomic 持锁或 act 时 re-check 状态（11-8 r11）

## 背景

Story 11.8 codex review r11 单条 [P1]：`broadcastToRoomFanout` 里 `ListSessionsByRoomID` 拿一份 sessions snapshot 后，进入 `for { Send(...) }` 循环；这两步**不**原子 —— 期间另一线程跑 `LeaveRoom` 同步段 (`unregisterLeaverSessionSync` → `mgr.Unregister(s)`) 把 leaver 从 SessionManager 双索引移除，但**已 snapshot 的列表里仍有 leaver 引用**，于是 fanout 循环到 leaver 时仍 `Send(payload)` 入队，leaver 仍能 drain 出一条 stale `member.joined` / `member.left`。这违反 V1 §10.5 步骤 7 钦定的"HTTP leave 立即 detach WS"语义。

同 Epic 之前 r3 / r4 / r5 / r6 / r7 都在围绕"async ordering / sync invariants"打转；r11 是更精微的一类：**snapshot 时机 vs 副作用执行时机**之间的时间差。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | snapshot+send race window：fanout 循环必须在 Send 前 re-check IsRegistered | high | concurrency | fix | `server/internal/app/ws/broadcast.go:324` |

## Lesson 1: snapshot+act 模式必须 atomic 持锁或 act 时 re-check 状态

- **Severity**: high
- **Category**: concurrency
- **分诊**: fix
- **位置**: `server/internal/app/ws/broadcast.go:324` (broadcastToRoomFanout 内 fanout 循环)

### 症状（Symptom）

`broadcastToRoomFanout` 在 `ListSessionsByRoomID` 拿 snapshot 后、`for s := range sessions { s.Send(payload) }` 循环里，另一线程 `LeaveRoom` 同步段调 `mgr.Unregister(leaverSessionID)` 把 leaver 从索引移除。此时本 fanout 循环仍含有 leaver 引用 → `s.Send(payload)` 入队 leaver 的 `sendChan` → leaver writeLoop drain 出 stale msg → leaver 收到 `member.joined`/`member.left`，与刚返 200 的 HTTP `/leave` 响应在时间线上**矛盾**（"HTTP leave immediately detaches WS"语义破坏）。

测试覆盖洞：r3-r10 系列加的测试只覆盖 backlog waiting / per-room worker / async ordering，**没有**覆盖 snapshot 后 unregister 的 race。

### 根因（Root cause）

**snapshot+act 模式的固有 race**：先取一份"该时刻系统状态"的快照（list / map / slice），随后基于快照做副作用（Send / Write / Notify）。两步之间任何"使快照部分元素失效"的并发写都会让副作用作用在 stale 元素上。

具体到本 case：
- `ListSessionsByRoomID` 走 `m.RLock()` 拿 sessionsByRoom 快照 → 释放 RLock → 返回切片
- 切片返回后任何对 manager 的并发 mutate（Register / Unregister / replace）都不在快照里反映
- `roomBroadcastMu` 串行化的是**同 room 的多个 broadcast goroutine 之间**，**不**串行化 broadcast vs Unregister
- 修复 r6 / r7 引入的 commit-time per-room serialization 串行化的是**broadcast 自身的 enqueue 顺序**，与 Unregister race 无关

设计层错误：把 Session.Send / SessionManager.Unregister 当成两个**独立**的并发原语用，但语义上"unregister 后立刻不应再收 broadcast"是个**跨原语**的不变量，必须在 broadcast 路径里显式 enforce。

### 修复（Fix）

在 fanout 循环 Send 之前调 `mgr.IsRegistered(ctx, s.SessionID())` re-check session 仍在索引中；返 false 则 skip 该 session 不 Send。`IsRegistered` 走 `m.RLock` 直接 lookup `sessionsByID[id]`，O(1)，对 N=4 单 room 性能可忽略。

```go
// broadcast.go：
for _, s := range sessions {
    // r11 P1 fix：re-check session 仍 registered（snapshot+send race window）
    if !mgr.IsRegistered(ctx, s.SessionID()) {
        skippedUnregistered++
        continue
    }
    if sendErr := s.Send(payload); sendErr != nil {
        logger.Warn("ws broadcast Send failed", ...)
    }
    sentCount++
}
```

**残留 race**（接受 best-effort）：IsRegistered=true → 另一线程 Unregister → Send 入队，仍可能下发。这是不可避免的（要彻底原子化必须把 SessionManager 写锁与 fanout 串行化，会大幅降低吞吐）。修复让 race window 从"snapshot 后整个 fanout 全程"缩到"check 与 Send 之间纳秒级"，远小于 client 端 observable 窗口。

**`sent` 返回值语义微调**：r11 修后 `sent` 只统计**实际发起 Send** 的数量（被 IsRegistered guard skip 的不计入）；既有测试 `TestBroadcastToRoom_OneSessionSendFails_OthersStillReceive` 已预先放宽断言为 `sent != 2 && sent != 3 (race with Unregister)`，与新行为兼容。

**新增测试**：`broadcast_except_internal_test.go` 加两个 case
- `TestBroadcastToRoomFanout_R11_SnapshotVsConcurrentUnregister_LeaverNotSent`
- `TestBroadcastToRoomExceptFanout_R11_SnapshotVsConcurrentUnregister_LeaverNotSent`

通过 `raceListMgr` 包装器让 `ListSessionsByRoomID` 返回固定 stale snapshot（绕过底层 mgr 的 Unregister 状态），`IsRegistered` 委托真实 mgr → 模拟 race window 精确复现。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写"snapshot 后做副作用"代码（先 `List…` / `Snapshot…` / `Range…` 拿一份快照，再 `for { do_side_effect(item) }`）时，**必须**在副作用前 re-check item 在原 source-of-truth 里仍合法（`IsRegistered` / `Exists` / `IsActive` / 状态机当前态），或者把 snapshot+side-effect 整段包在持有 source-of-truth 写锁内做成 atomic 段。
>
> **展开**：
> - **判定一个原语是不是 snapshot+act**：返回 `[]T` / `[]K` / `map[X]Y` 切片或副本的查询接口 + 后续基于返回值的 mutate / Send / Notify。这类调用所有"读后写"模式都有此 race window。
> - **三种修法路径，按工作量从小到大**：
>   - 路径 A（首选）：每个 item 在 act 前 re-check 状态（`IsRegistered` / `Exists`）。改动小，best-effort 残留极窄 race 通常可接受。
>   - 路径 B：act 路径自身在 source-of-truth 内做 closed/cancelled 检测（如 `Session.Send` 内查 `closed atomic.Bool` 返 ErrSessionClosed）。但要求 source-of-truth 能传递信号给 act 原语（本 case `Unregister` **不**调 `Session.Close`，所以 Session.Send 的 closed flag 仍 false → 路径 B 不可用）。
>   - 路径 C：source-of-truth 层提供 atomic broadcast / batch-act API（持锁段内一起做 list+act），改 primitive 工作量大但 race 彻底消除。
>   - 路径 D：定义"snapshot 失效后 stale send 是合法行为"（重新审视语义合约）。仅当业务 SLA 允许时用。
> - **跨原语不变量必须显式 enforce**：本 case 的"Unregister 后立刻不应再收 broadcast"是 SessionManager + broadcast 两个模块共同维护的不变量，**不能**默认两边各自原子操作就够。
> - **测试方法**：用 manager 包装器把 List 接口返回的切片"冻结"成 stale snapshot（绕过实际状态），把对应原语的 IsRegistered / Exists 委托给真实 manager（已反映 race 后状态），断言 act 路径正确 skip stale 元素。这种"决定论模拟 race"比 sleep 注入更可靠（不依赖 timing）。
> - **反例**：
>   - ❌ "我已经在 fanout 循环外加了 per-room mutex"——per-room mutex 串行化的是同 room 多个 broadcast goroutine 之间，**不**串行化 broadcast vs Unregister。
>   - ❌ "Session.Send 内部有 closed flag check 兜底"——只有当 Unregister 路径调 Session.Close 时才有效；本 repo 的 Unregister 仅做索引移除，**不**翻 Session.closed flag。
>   - ❌ "snapshot 切片是引用拷贝，反映最新状态"——切片是引用拷贝，但每个元素是独立堆对象，元素上的"是否仍 registered"信号需要从 source-of-truth 单独 query。
>   - ❌ "race window 极小 < µs，可忽略"——HTTP 200 → client 收 close.4007 → client 主动 close 的 observable window 是 ms-s 级；snapshot 后的 fanout 循环本身也是 µs 级跨度，足以让 race 在 prod 真实出现。

## Meta: 11-8 系列 11 轮 review 的宏观教训

11-8 单 story 跑了 11 轮 codex review；前 10 轮主线问题均围绕"async / commit-time / per-room serialization"打转，r11 揭示**最后一类**漏网 race：snapshot 与 act 之间的时间窗口。

宏观教训：每当系统里**多个原语共同维护一个跨原语不变量**（如"unregister 即时生效" + "broadcast 不下发给 unregistered"），必须**在每个原语的边界处显式 enforce**该不变量；不能依赖"两个原语各自原子操作"自动达成。BMAD review 流程的价值在于这类深层 race 单凭 dev 自己的"顺手补 mutex"是发现不了的——要靠外部 review 持续追问"snapshot 后 X 变化怎么办"。

11-8 review 走 11 轮的根因：原始设计把 Session.Close / Unregister 视为幂等可乱序的 lifecycle 操作，但叠加 V1 §10.5 步骤 7 "HTTP leave 立即 detach WS" 这个**强同步**语义后，系统里突然多出"必须保证某时刻起不再收 X 事件"的硬约束 —— 这种"时刻语义"无法由幂等 lifecycle 操作自动达成，必须在每条 fanout 路径显式 re-check。
