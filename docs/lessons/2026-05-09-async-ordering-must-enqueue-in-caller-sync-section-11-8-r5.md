---
date: 2026-05-09
source_review: /tmp/epic-loop-review-11-8-r5.md (codex review r5 of Story 11.8)
story: 11-8-成员加入-离开-ws-广播
commit: f423c33
lesson_count: 2
---

# Review Lessons — 2026-05-09 — 并发顺序保证：lock 必须在 caller 同步段获取；long-running side effect 不应持 serialization lock（11-8 r5）

## 背景

Story 11.8（房间成员加入 / 离开 WS 广播）r4 用 `sync.Map[roomID]*sync.Mutex` 做 per-room serialization 试图保留 commit 顺序的 causal ordering。codex review r5 指出两条 regression：

1. **[P1]** mutex Lock 在 goroutine **内**取，仍受 Go scheduler 影响。caller 同步段 commit 顺序为 join → leave，但两个 goroutine 启动后 leave 可能抢先 Lock 拿到 mu，broadcast 顺序反转。
2. **[P2]** LeaveRoom post-commit 的 `closeLeaverSessionAsync`（CloseWithCode 慢路径 ~5s）和 `broadcastMemberLeft` 都跑在同一 mu 临界区内。slow leaver 阻塞整 room 后续 broadcast。

修法：用 per-room **FIFO channel queue + worker goroutine** 替换 mu —— enqueue 在 caller **同步段**完成（channel send 是 atomic），FIFO 顺序由 channel 自带；slow close 拆出独立 fire-and-forget goroutine，**不进** queue。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | per-room mutex 在 goroutine 内取 → caller commit 顺序仍可能反转（scheduler race） | P1 / high | architecture / concurrency | fix | `server/internal/service/room_service.go:453-470` |
| 2 | broadcastMemberLeft 与 closeLeaverSessionAsync 共享 per-room serialization lock → slow close 阻塞整 room 后续 broadcast | P2 / high | architecture / latency | fix | `server/internal/service/room_service.go:867-870` |

## Lesson 1: lock 必须在 caller 同步段获取，goroutine 内取序为时已晚

- **Severity**: P1 / high
- **Category**: architecture / concurrency
- **分诊**: fix
- **位置**: `server/internal/service/room_service.go:453-470` (r4 perRoomMu 路径) → `room_service.go:enqueueRoomEvent` (r5 channel queue 路径)

### 症状（Symptom）

caller 在同一 roomID 上同步 commit 两次（join → leave），分别 fire-and-forget 起 goroutine A 和 goroutine B，goroutine 内**首条指令**就 Lock per-room mu。

```go
// r4 实装
go func() {
    mu := loadRoomMu(roomID)
    mu.Lock()           // ← 这里取序，但 goroutine 已经被 scheduler 重排了
    defer mu.Unlock()
    fn(timedCtx)        // broadcast
}()
```

如果 Go scheduler 让 goroutine B 比 goroutine A 先跑（在 GOMAXPROCS > 1 + 多核 / GC 抢占等场景下完全合法），B 抢先 Lock，broadcast 顺序变成 leave → join，违反 client 因果观察约束。

### 根因（Root cause）

**保序点选错了**：mu 是用来保护"同一 roomID 串行执行 fn"的，但**它不能创造序** —— 它只能让两个**已经按序到达**的 goroutine 排队。要让两个 goroutine 按 caller 顺序到达，必须在 **caller 同步段**就完成排序动作，而不是依赖 goroutine 内首条指令。

caller 同步段是单 goroutine 串行执行的（caller 是同一个请求 handler 调连续两个 service 方法，或就是单 goroutine 顺序调），任何在 caller 同步段完成的动作都自动按 caller 顺序排序。把"取序点"放到 caller 同步段才有保序意义。

### 修复（Fix）

用 per-room **FIFO channel queue + worker goroutine** 替换 mu：

```go
// r5 实装
type roomQueue struct {
    ch   chan func()
    once sync.Once
}

func (s *roomServiceImpl) enqueueRoomEvent(ctx, roomID, fn) {
    qIface, _ := s.roomQueues.LoadOrStore(roomID, &roomQueue{ch: make(chan func(), 256)})
    q := qIface.(*roomQueue)
    q.once.Do(func() { go s.runRoomQueueWorker(q) })

    wrapped := func() { /* detached ctx + timeout + fn */ }

    // ← caller 同步段：channel send 是 atomic + FIFO
    select {
    case q.ch <- wrapped:
    default:
        // 满了 drop 优于阻塞 caller
    }
}

func (s *roomServiceImpl) runRoomQueueWorker(q *roomQueue) {
    for fn := range q.ch {
        fn()
    }
}
```

**关键**：`q.ch <- wrapped` 在 caller 同步段同 goroutine 顺序执行 —— enqueue 顺序就是 caller 调用顺序。worker 通过 `for fn := range q.ch` 顺序消费，channel 自带 FIFO 语义。Go scheduler 不能重排同一 goroutine 内的 send 顺序。

**测试**：保留旧的 `TestRoomService_PostCommit_PerRoomSerialization_PreservesCausalOrdering`（仍验证基础场景），新增 `TestRoomService_PostCommit_RapidJoinLeave_PreservesEnqueueOrder`（broadcastFn 内注入 sleep 模拟慢 fanout，验证 worker 慢消费下 enqueue 顺序仍保留）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**为多个 fire-and-forget goroutine 引入因果顺序约束**时，**必须**把"取序"动作（lock acquire / channel enqueue / counter increment）放在 **caller 同步段**，**禁止**把它作为 goroutine 内首条指令。
>
> **展开**：
> - **mutex / RWMutex 不能创造序**：mu 只能让"已到达的 goroutine 排队"。如果要保 caller commit 顺序，caller 同步段必须先完成"取序"动作（如 channel send / linked list append / sequence number 分配）。
> - **channel send 是 atomic + caller-sync**：在 caller 同步段做 `ch <- v` → 同 caller goroutine 内多次 send 顺序严格 = caller 调用顺序；channel receive 端 worker 顺序消费。这是最简洁的"业务序保留"原语。
> - **`go func() { mu.Lock(); ... }()` 是反模式**：一旦 goroutine 起来，Go scheduler 可以以任意顺序调度它们到 mu.Lock。
> - **反例 1**：r4 perRoomMu —— mu 在 goroutine 内取序，scheduler 可重排。
> - **反例 2**：用 atomic counter 在 caller 同步段分配 seq + worker 内排序 —— 多了一层复杂度，不如 FIFO channel 直接。
> - **反例 3**：用 sync.Cond 在 caller 同步段 Wait → goroutine 内 Signal 触发顺序消费 —— 复杂且容易引入死锁，channel queue 更优。

## Lesson 2: long-running side effect 不应持 serialization lock —— 拆独立 goroutine 与 ordering queue 解耦

- **Severity**: P2 / high
- **Category**: architecture / latency
- **分诊**: fix
- **位置**: `server/internal/service/room_service.go:867-870` (r4 路径) → `room_service.go:LeaveRoom` post-commit + `runCloseLeaverAsync` (r5 路径)

### 症状（Symptom）

LeaveRoom post-commit 把 `closeLeaverSessionAsync`（CloseWithCode 内部 drain WS write loop ~5s）和 `broadcastMemberLeft` 都包进 per-room serialization 临界区里：

```go
// r4 实装
runPostCommitAsyncPerRoom(ctx, roomID, func(detachedCtx) {
    closeLeaverSessionAsync(detachedCtx, roomID, userID, target)  // ← 慢路径 5s
    broadcastMemberLeft(detachedCtx, roomID, userID)
})
```

后果：一个 stuck leaver session（写慢 / 网络断 / WS write timeout）让 close 跑 5s，整 room 后续所有事件（如另一 user 立刻 join 触发的 member.joined 广播）也得排队等这 5s。其他成员看到的 roster 视图持续 stale 数秒。

### 根因（Root cause）

**两类工作语义不同，不应共享 lock**：
- **broadcast** 是**因果序敏感**的（join → leave 必须按序到达），应该走 ordering queue。
- **close** 是 best-effort cleanup（leaver Session 已在同步段 unregister 了；CloseWithCode 是给 WS peer 一个 close frame 的礼貌动作，慢/失败都不影响业务正确性），**没有因果序约束**。

把 close 放进 ordering queue 是把"无序约束的 long-running 工作"塞到"严格保序的轻量队列"里 —— 直接堵塞队列吞吐。

### 修复（Fix）

把 close 拆出独立 fire-and-forget goroutine（`runCloseLeaverAsync`），与 per-room queue 完全解耦：

```go
// r5 实装
target, _ := s.unregisterLeaverSessionSync(ctx, roomID, userID)

// (a) broadcast 入 per-room queue（保序段）
s.enqueueRoomEvent(ctx, roomID, func(detachedCtx) {
    s.broadcastMemberLeft(detachedCtx, roomID, userID)
})

// (b) close 独立 goroutine（fire-and-forget；不阻塞 queue）
if target != nil {
    s.runCloseLeaverAsync(ctx, roomID, userID, target)
}
```

`runCloseLeaverAsync` 内部仍走 detached ctx + timeout 兜底：

```go
func (s *roomServiceImpl) runCloseLeaverAsync(ctx, roomID, userID, target) {
    go func() {
        detached := context.WithoutCancel(ctx)
        timedCtx, cancel := context.WithTimeout(detached, postCommitTimeout)
        defer cancel()
        s.closeLeaverSessionAsync(timedCtx, roomID, userID, target)
    }()
}
```

**测试**：新增 `TestRoomService_PostCommit_LeaveCloseDoesNotBlockBroadcast` 验证 broadcast 不被 close 阻塞（caller HTTP 路径延迟 < 500ms；wg 只追踪 broadcast，close 完不完成不影响 wg.Wait）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**设计 fire-and-forget 路径**时，**必须**按"是否需要因果序"把工作分流到不同执行上下文：保序的轻量工作进 ordering queue / serialization lock；无序的 long-running side effect 走**独立** goroutine。**禁止**把所有 fire-and-forget 工作打包进同一个 lock / queue。
>
> **展开**：
> - **判定准则**：每条 fire-and-forget 工作问"它和 queue 里其他事件有因果关系吗？"。是 → 进 queue；否 → 独立 goroutine。
> - **典型分类**：
>   - 进 queue：业务事件 broadcast / 状态机转换通知 / cross-event 顺序敏感的 cleanup
>   - 独立 goroutine：网络 close / 文件落盘 / 长 IO / metric flush / 第三方 API 调用
> - **不要**靠"工作快/慢"判断 —— 即使 close 当前实现 100ms，未来网络抖动可能让它 5s；把它放进保序 queue 一开始就是设计错误。
> - **同步段先做"必须立即可见"的部分**：本 case 中 `unregisterLeaverSessionSync` 在 caller 同步段做（让 leaver 立即从 SessionManager 索引消失，"HTTP leave immediately detaches" 语义达成），CloseWithCode 才走异步段。
> - **反例 1**：r4 把 close + broadcast 一起塞进 per-room mu 临界区 —— 慢 close 堵塞 broadcast 吞吐。
> - **反例 2**：把 metric flush 放进业务 ordering queue —— flush IO 阻塞下一个业务事件 broadcast。
> - **反例 3**：把第三方 webhook 通知放进同一 queue —— 第三方端慢 / 超时拖慢整个 queue。

---

## Meta: 本次 review 的宏观教训

r2 → r3 → r4 → r5 的连续 4 轮修复都围绕"fire-and-forget 异步化"主题。串成一条线看：

- **r2**：post-commit 同步走会被 request ctx cancel 误中断 → 改 detached ctx + 独立 goroutine + timeout
- **r3**：异步化引入"sync 可观察 invariants 失守"（joiner self-fanout / leaver stale subscription）→ 部分回归同步段（unregister sync）+ broadcastExcept 防御
- **r4**：异步化破坏 commit 顺序的 causal ordering → 加 per-room mutex
- **r5**：mutex 在 goroutine 内取序无效 + close 慢路径堵塞保序 queue → 改 channel queue（caller 同步段 enqueue）+ close 独立 goroutine

**核心启示**：异步化不是"把代码包进 goroutine"那么简单；每一处"同步语义"都要显式审视是否需要保留：
1. **可观察性**（sync 段做 unregister 让"HTTP 200 后状态立即生效"）
2. **因果序**（caller 同步段 enqueue 让"caller commit 顺序保留"）
3. **隔离性**（不同语义的工作走不同 goroutine / queue，不共享 lock）

**通用方法论**：
- 设计异步路径时画一张表：每条工作 → "需要保的 sync 语义有哪些" → 选合适的载体（caller 同步段 / ordering queue / 独立 goroutine）
- "异步化"不是黑白决策；是按工作语义粒度切分 —— 同一接口的不同 post-commit 工作可能走 3 种不同载体（如 leave 路径：unregister sync + broadcast queue + close 独立 goroutine）
