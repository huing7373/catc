---
date: 2026-05-09
source_review: codex review r6 输出（/tmp/epic-loop-review-11-8-r6.md）
story: 11-8-成员加入-离开-ws-广播
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-09 — commit-order = causal-order 必须 commit-time per-key serialization；caller 同步段任何工作都会破坏顺序（11-8 r6）

## 背景

Story 11.8 房间 join/leave 广播链路在 r2 → r3 → r4 → r5 五轮 review 中演进过的并发模型，r6 codex review 揭示**核心机制层**仍存在两条 P1 race：

```
LeaveRoom：
  txMgr.WithTx(...)                    // commit
  unregisterLeaverSessionSync(...)     // ← 同步段 SessionManager Unregister（O(1) 但仍是同步操作）
  enqueueRoomEvent(...)                // ← 才 enqueue broadcast member.left
```

**race window**：commit 与 enqueue 之间的 unregister 同步段。在该 gap 内，concurrent JoinRoom 完全可以 commit + enqueue member.joined：

- (a) 此时 leaver 仍在 SessionManager → JoinRoom 的 broadcastMemberJoined fanout 列表含 leaver → leaver 收到 stale event。
- (b) JoinRoom 的 enqueue 抢在 LeaveRoom 的 enqueue 之前 → channel FIFO 顺序倒置 → client 看到 member.joined 早于 member.left → 违反 commit-order = causal-order 保证。

r5 的 channel queue 方案保的是 enqueue 顺序，但**前提是 caller "commit 后立刻 enqueue 期间无任何同步操作可被 concurrent 路径夹塞"**。LeaveRoom r5 实装违反了这个前提。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | LeaveRoom unregisterLeaverSessionSync 在 commit 后 enqueue 前 → concurrent join 看到 stale leaver | high | architecture | fix | `server/internal/service/room_service.go:941` |
| 2 | LeaveRoom 同步段把 enqueue 推后 → concurrent join 的 enqueue 抢前 → broadcast 顺序倒置 | high | architecture | fix | `server/internal/service/room_service.go:943-945` |

两条同根因，合并为一条 lesson。

## Lesson 1: commit-order = causal-order 的真正条件是 commit-time per-key serialization

- **Severity**: high
- **Category**: architecture / concurrency
- **分诊**: fix
- **位置**: `server/internal/service/room_service.go` JoinRoom (~Line 781) / LeaveRoom (~Line 935)

### 症状（Symptom）

两个 caller 在同 roomID 上分别调 JoinRoom（A）和 LeaveRoom（B）：A commit 早于 B commit，但 client 收到的 broadcast 是 [member.left（B）, member.joined（A）] —— 顺序倒置。同时 leaver 仍在 SessionManager 期间，concurrent join 的 fanout 包含 leaver session，让"已离开"用户收到房间内其他人的 join 事件。

### 根因（Root cause）

**commit-order = causal-order 等式成立的前提，是 commit 与 enqueue 必须 atomic（不可被 same-key 的 concurrent path 中插）**。r5 channel FIFO queue 单独无法提供这个保证 —— 它只保 enqueue 顺序 = worker 消费顺序 = client 感知顺序。如果 caller A 的 enqueue 与 caller B 的 enqueue **顺序不等于 commit 顺序**，channel FIFO 就把"错误的 enqueue 顺序"忠实传给 worker，causal order 同样破坏。

r5 实装在 LeaveRoom 留了同步段 `unregisterLeaverSessionSync`（map op O(1)，看起来"瞬时无害"）：

```go
// r5
err = s.txMgr.WithTx(ctx, ...)        // commit (1)
target, _ := s.unregisterLeaverSessionSync(ctx, roomID, userID)  // 同步段 (2)
s.enqueueRoomEvent(ctx, roomID, broadcastMemberLeft)              // enqueue (3)
```

(1) → (2) → (3) 中 (2) 看似只占微秒，但**在多核并发下足够 concurrent JoinRoom 完成 (1) + (3)**。Go scheduler / GC pause / OS preemption 都可能在 (2) 期间挂起 LeaveRoom goroutine。即使 (2) 是单条原子 map op，**任何"非 enqueue"的同步操作都是**事件夹塞窗口。

**深层教训**：Concurrent ordering 问题的核心不是"同步操作多快"，而是"commit 与 enqueue 之间是否还有任何可被 same-key 路径见缝插针的 op"。即使 0ns 的 placeholder 同步段，理论上也能被夹塞（CPU 中断在两条相邻指令之间发生）。

### 修复（Fix）

**commit-time per-room mutex 包住 (commit + enqueue)**，让 same-room 事务串行 commit + 串行 enqueue；unregister 移进 worker 闭包，与 broadcast 同 worker 串行执行。

```go
// r6 修法（JoinRoom / LeaveRoom 同模式）
mu := s.acquireCommitLock(in.RoomID)   // sync.Map[uint64]*sync.Mutex 模式
mu.Lock()
defer mu.Unlock()                       // 兜底 panic / err 路径
err = s.txMgr.WithTx(ctx, ...)          // commit 在 lock 内
if err != nil { return ... }
s.enqueueRoomEvent(ctx, roomID, fn)     // enqueue 紧跟 commit，仍在 lock 内
return ...
// defer mu.Unlock() —— HTTP 200 在 unlock 之后；lock 内只有 instant op，client 无感
```

LeaveRoom 把 `unregisterLeaverSessionSync` 和 `runCloseLeaverAsync` 移进 enqueue 的 fn 闭包（**worker 内执行**）：

```go
s.enqueueRoomEvent(ctx, roomID, func(detachedCtx context.Context) {
    target, _ := s.unregisterLeaverSessionSync(detachedCtx, roomID, userID)  // worker 内 unregister
    if target != nil {
        s.runCloseLeaverAsync(detachedCtx, roomID, userID, target)            // close goroutine 启动是 instant op
    }
    s.broadcastMemberLeft(detachedCtx, roomID, userID)                        // unregister 后 broadcast
})
```

**关键约束**：
- lock 内**只允许 instant op**（DB commit + channel send + goroutine 启动）。**禁止** IO / wait / sleep / 远程调用。
- close goroutine 启动是 instant op，可以在 worker 内启动；CloseWithCode drain 慢路径仍跑在独立 goroutine。
- 不同 roomID 不共享 mutex（sync.Map LoadOrStore），各 room 仍并行处理。

**trade-off**：same-room 事务 commit 串行化（不再 interleave）。MVP 节点 4 阶段单 room ≤4 人并发极低，可接受；高并发场景需要重新评估。

**测试 negative control 验证**：临时移除 lock 跑 `TestRoomService_PostCommit_CommitTimeLockSerializesConcurrentJoinLeave` —— broadcast 顺序立即倒置成 [member.left, member.joined]。证明 test 真能捕获 race，不是 false-positive pass。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在设计 "commit-order = client-observable-order" 类异步广播路径时，**必须**在 caller 同步段用 per-business-key mutex 包住 (业务事务 commit + 事件 enqueue/dispatch) 段，**禁止**在两者之间插入任何同步操作（哪怕 O(1) map op）。
>
> **展开**：
>
> - **per-key serialization 是 commit-order = causal-order 的必要条件**：channel FIFO / queue worker 只保"enqueue → 消费"顺序；要保"commit → enqueue"顺序，必须 caller 同步段拿同一把 lock 才能 commit + enqueue。
>
> - **lock 粒度按业务 key（roomID / userID / orderID 等）**：sync.Map[K]*sync.Mutex 模式 LoadOrStore；不同 key 不共享 lock，吞吐不损。
>
> - **lock 内只允许 instant op**：DB commit、channel send、goroutine 启动（go func 本身瞬时）。任何潜在 IO / wait / sleep（含 SessionManager.Close drain、HTTP 远程调用）必须放 lock 之外的独立 goroutine（或推进 worker 闭包内串行执行）。
>
> - **defer Unlock 兜底**：拿 lock 后立即 `defer mu.Unlock()`，让 panic / err 早返回路径也保证 unlock。
>
> - **HTTP 响应延迟可接受**：lock 在 HTTP handler 调 service 的同步路径上，但 instant op 累积 < 1ms client 无感；不要因为"lock 在 HTTP 路径上"就妥协把它推到 goroutine 内（会重新引入 r4 / r5 的根本问题）。
>
> - **测试必须 negative control 验证**：写完 race-protection test 后，临时移除 lock 跑一遍确认 test 能 fail；不能 fail 的 race test 是 false confidence（可能因 timing 巧合每次 pass，让 future regression 漏网）。
>
> - **反例 1**（r5 留下的根因）：以为 "channel FIFO 保 enqueue 顺序就够了"，在 commit 与 enqueue 之间插入"无害"同步段（如 unregisterLeaverSessionSync）—— 该 gap 在 concurrent 路径下足以让 same-key 事务 commit + enqueue 抢先，破坏 causal order。
>
> - **反例 2**（r4 → r5 演进留下的迷思）：在 goroutine 内 Lock 同一 mutex 保序。goroutine 启动顺序由 Go scheduler 决定，**不**等于 caller 同步调用顺序 —— 后者可抢先 Lock。保序必须在 caller 同步段完成（commit-time，不是 dispatch-time）。
>
> - **反例 3**（性能优化误区）：以为 "lock 包 commit 会拖慢 throughput"，把 lock 拆细到 enqueue-only —— 错失保序的根本约束。MVP 阶段 throughput 极低（节点 4 单 room ≤4 人），优化方向应是"先正确再优化"。

## Meta: r2 → r6 五轮演进的宏观教训

并发 ordering / causal-order race 是出现频率最高的 review feedback 类型（11-8 单 story 6 轮 review，4 轮 P1 都属于此类）。每一轮修复都引入下一轮的 race window：

| Round | 修法 | 留下的 race |
|---|---|---|
| r2 | 整体异步化 post-commit | 同步语义被破坏（leaver 仍在 SessionManager 收 stale broadcast） |
| r3 | hybrid sync/async + BroadcastToRoomExcept | scheduler race（goroutine 启动顺序 ≠ commit 顺序） |
| r4 | per-room mutex（goroutine 内 Lock） | mutex Lock 在 goroutine 内，仍受 scheduler race 影响 |
| r5 | per-room channel queue（caller 同步段 enqueue） | LeaveRoom 同步段 unregister 夹在 commit 与 enqueue 之间 |
| r6 | commit-time per-room mutex 包 (commit + enqueue) | （本轮修复目标）|

**根本教训**：concurrent ordering 类设计应当**前置正确性证明**而不是迭代修复。每次修复实际上只是把 race window "推后"或"换形式"——除非从理论上证明（A）caller commit 顺序与（B）client 感知顺序之间的所有同步路径都被 lock / channel 严格串行化，否则 race 会再次出现。

**未来类似设计建议**：
1. **先画 happens-before 图**：caller commit (A) → enqueue (B) → worker consume (C) → client visible (D)。每条边必须显式约束（lock / channel / dispatch order）。
2. **逐边证明**：每条边的顺序保证是什么？是 mutex 串行化？channel FIFO？还是依赖 scheduler？scheduler 不能依赖。
3. **测试必含 negative control**：临时去掉一条约束跑 test，应当能 fail；fail 不到说明 test 不暴露 race，需要重设 timing。

如本 story 在 r2 之前先做 happens-before 分析，可能 1 轮就到位 r6 的方案，避免 4 轮 review iteration。
