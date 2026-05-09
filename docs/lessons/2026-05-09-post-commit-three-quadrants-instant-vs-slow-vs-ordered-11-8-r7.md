---
date: 2026-05-09
source_review: codex review r7 输出（/tmp/epic-loop-review-11-8-r7.md）
story: 11-8-成员加入-离开-ws-广播
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-09 — post-commit hook 三象限：sync 段必须 instant ops；slow ops 进 fire-and-forget；ordering-sensitive ops 进 worker queue（11-8 r7）

## 背景

Story 11.8 房间 leave 路径在 r2 → r6 六轮演进里把 unregisterLeaverSessionSync 在三处不同位置之间挪动，每挪一次都带来新 trade-off：

```
r3-r5: caller 同步段（commit 与 enqueue 之间）
       → r6 race：leaver 仍在 SessionManager，concurrent join fanout 含 leaver

r6:    worker 闭包内（与 broadcast 同 fn）
       → r7 race：worker FIFO 串行消费；前序 backlog 未跑完时 leaver 仍在
         SessionManager；HTTP 200 已返回 → 违反"HTTP leave immediately
         detaches WS"钦定（V1 §10.5 步骤 7）

r7:    commit-time lock 段内（commit 后、enqueue 前）同步执行
       → 两条 invariant 同时成立：HTTP 200 前 leaver 已 detached + commit-order
         由 lock 保留
```

**根因**：post-commit hook 的"三象限"分类没有清楚 —— 每个 work item 必须先归类（instant op / slow op / ordering-sensitive op），再放到正确位置。挪一次 unregister 等于在三个象限之间反复横跳。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | LeaveRoom unregister 在 worker 闭包 → 前序 backlog 未跑完时 leaver 仍 active | high | architecture | fix | `server/internal/service/room_service.go:1045-1048` |

## Lesson 1: post-commit hook 三象限分类规则（instant / slow / ordering-sensitive）

- **Severity**: high
- **Category**: architecture / concurrency
- **分诊**: fix
- **位置**: `server/internal/service/room_service.go` LeaveRoom (~Line 1040-1060)

### 症状（Symptom）

LeaveRoom HTTP 200 返回时，leaver 仍在 SessionManager —— 期间 worker queue 跑前序排队（如同 room 之前 JoinRoom 的 broadcastMemberJoined 慢路径）的 broadcast 时仍把 leaver 当 active member fanout，leaver 收到 stale member.joined event。违反 V1 §10.5 步骤 7 "HTTP leave immediately detaches WS" 钦定。

### 根因（Root cause）

post-commit hook 不是单一 "异步路径"——它是**三种性质 work 的混合**，每种性质需要不同的执行位置：

| 象限 | 性质 | 例子 | 正确位置 |
|---|---|---|---|
| **A: instant ops** | nano~微秒级；无 IO；执行确定时间 | DB commit、map.Delete（SessionManager.Unregister）、channel send、go spawn | **caller 同步段（在 commit-time lock 内）** |
| **B: slow ops** | 毫秒~秒级；可能阻塞 / 含 IO / 含 wait | CloseWithCode（drain WS write loop ~5s）、远程 RPC | **独立 fire-and-forget goroutine**（不阻塞 lock / 不阻塞 worker） |
| **C: ordering-sensitive ops** | 必须按 caller commit 顺序 fanout 给 client | broadcast member.joined / member.left | **per-key worker queue**（lock 内 enqueue 保 commit-order = enqueue-order = causal-order） |

r6 把 unregister（属象限 A）误放到象限 C 的 worker queue 里 —— worker 是 FIFO 串行消费，前序 backlog 未跑完时 unregister 不执行，违反"HTTP leave immediately detaches WS"。

**为什么 r6 会犯这个错？** 因为 r6 关注的 trade-off 是 "commit + enqueue 之间不能有同步段（怕被 concurrent 路径夹塞抢序）"。修复时把 unregister 整体移走 —— 但忘了 unregister 本身是 instant op，可以**同样在 lock 内**执行，既不破坏 commit-order（lock 包住 commit + unregister + enqueue 整体），又不延迟 HTTP（map op nano 级）。

### 修复（Fix）

LeaveRoom 把 unregisterLeaverSessionSync 从 worker 闭包移回 commit-time lock 段，commit 之后、enqueue 之前同步执行：

```go
LeaveRoom:
  mu := s.acquireCommitLock(roomID)
  mu.Lock()
  defer mu.Unlock()

  // (1) commit (instant: DB write, ms 级)
  s.txMgr.WithTx(ctx, ...)

  // (2) unregister (instant: map.Delete, nano 级) ← r7 新位置
  target, _ := s.unregisterLeaverSessionSync(ctx, roomID, userID)

  // (3) enqueue broadcast (instant: channel send + select default 兜底)
  s.enqueueRoomEvent(ctx, roomID, broadcastMemberLeft)

  // (4) close goroutine (instant: go spawn; drain ~5s 在 goroutine 内)
  if target != nil {
    s.runCloseLeaverAsync(ctx, roomID, userID, target)
  }
  return // HTTP 200 (defer mu.Unlock())
```

JoinRoom 同模式（commit + enqueue 都在 lock 段内，无 unregister）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计 post-commit / async hook 链路** 时，**必须先把每个 work item 按"三象限"分类**（instant op / slow op / ordering-sensitive op），**禁止把 instant op 误放进 worker queue 或 ordering-sensitive op 误放进 fire-and-forget goroutine**。
>
> **展开**：
> - **象限 A（instant ops）**：DB commit、map 操作（Register/Unregister）、channel send、go spawn —— 全部放进 caller 同步段（在 commit-time per-key lock 内）。这些 op nano~毫秒级完成，不延迟 HTTP，反而**必须**在同步段执行才能让"HTTP 200 前 invariant 已成立"成立（如 "leave HTTP 200 → leaver 立即从 SessionManager detach"）。
> - **象限 B（slow ops）**：CloseWithCode（drain ~5s）、远程 RPC、含网络 IO 的清理 —— 拆独立 goroutine（fire-and-forget），caller 同步段只做 `go func(){...}()` spawn（spawn 本身是 instant op，可放进 lock）。绝不能在 lock / worker queue 内同步 wait（会阻塞同 room 后续所有事件）。
> - **象限 C（ordering-sensitive ops）**：fanout 给 client 的 broadcast —— 进 per-key worker queue。caller 同步段的 enqueue（channel send）属 instant op 可放进 lock；fanout 本身属 worker queue 串行段。**关键**：lock 包住的是 (commit + 象限 A 同步段 + 象限 C 的 enqueue + 象限 B 的 spawn)，不是 (commit + 象限 C 的 fanout)；fanout 还是异步跑。
> - **lock 持有时间不变量**：commit-time lock 内必须 100% 是象限 A op（含象限 B 的 spawn / 象限 C 的 enqueue 视为 spawn/send 这一步是 instant）；任何 wait / IO / drain 都必须出 lock。这条 invariant 让 lock 持有 < 1ms，HTTP 路径无感。
> - **反例 1**：r6 把 unregister（象限 A）误放进 worker queue 闭包 —— worker FIFO 串行消费让 instant op 等前序 backlog，违反"HTTP 200 时 invariant 已成立"。
> - **反例 2**：r4 把 close-frame drain（象限 B）放进 per-room mutex 临界区 —— 一个 stuck leaver 拖累整 room 后续所有 broadcast 几秒钟。
> - **反例 3**：早期把 broadcast fanout（象限 C）整体移到独立 goroutine 但**不**用 worker queue —— goroutine 启动顺序由 scheduler 决定，concurrent join/leave 顺序倒置（违反 commit-order）。
> - **反例 4**：把 unregister（象限 A）放在 commit 后但 lock 之外 —— concurrent join 可在 gap 内 commit + enqueue 抢前序，破坏 commit-order = causal-order（r6 finding）。
> - **正例（r7）**：lock 内顺序执行 commit (A) → unregister (A) → enqueue (C 的 send 步骤是 A) → close-spawn (B 的 spawn 步骤是 A)，全部 < 1ms 完成 unlock。worker 异步消费 enqueue 的 broadcast；close goroutine 异步 drain。两条 invariant（detach + commit-order）同时成立。
>
> **诊断启发**：如果 review 反复挪同一个 op 在三象限之间横跳（r3 → r4 → r5 → r6 → r7 都涉及 unregister），说明根本上没分类清楚 —— 不是该 op 的问题，是分类规则没建立。先画三象限表把所有 work item 归类，再决定位置。

## Meta: 本次 review 与 r2-r6 的连续教训

Story 11.8 的 r2 → r7 六轮 review 揭示了一个 meta 模式：**异步并发设计的 trade-off 不是"速度 vs 正确性"，而是"哪些 invariant 在哪个时间点必须成立"**。每个象限对应一组 invariant：

- 象限 A 同步段：**HTTP 200 时 invariant 已成立**（如 detach、idempotency key 已写入）。
- 象限 C worker queue：**caller commit 顺序 = client 感知顺序**（causal ordering）。
- 象限 B fire-and-forget：**best-effort cleanup，不影响 HTTP 路径 / 不阻塞 ordering 路径**。

任何 work item 错位都会破坏对应 invariant。三象限分类是 post-commit hook 设计的**前置工序**，不是事后补救。
