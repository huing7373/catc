---
date: 2026-05-09
source_review: codex review r4 (epic-loop output: /tmp/epic-loop-review-11-8-r4.md)
story: 11-8-成员加入-离开-ws-广播
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-09 — 异步化路径必须保留 caller commit 顺序的 causal ordering（11-8 r4：per-key serialization）

## 背景

Story 11.8（房间成员 WS 广播）r2 修复把 post-commit hook（broadcast member.joined / member.left + close leaver session）切成 fire-and-forget 异步 goroutine 解决了"5s WS write timeout 阻塞 HTTP 响应"+"request ctx cancel 误中断 broadcast"两条 [P1]。r3 又用 hybrid 切分（Unregister 同步 / CloseWithCode + broadcast 异步）+ BroadcastToRoomExcept 解决"leaver stale subscription"+"joiner self-fanout"两条 [P1]。但 r2 引入的"无约束 goroutine"语义留下了**第三层 regression** —— 同一 roomID 上的连续 mutation（join → leave / leave → join）破坏 causal ordering。r4 codex review 单一 [P1] flag 这一点。本 lesson 把 4 轮 review 累计的"异步化反复踩坑"归纳为一条规则：fire-and-forget 路径若需要保留 caller 调用顺序，必须按相关业务 key（roomID / userID）做 serialization。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | post-commit goroutine 之间缺乏 per-roomID serialization 破坏 join/leave 的 causal ordering | high | architecture / concurrency | fix | `server/internal/service/room_service.go:412-462` |

## Lesson 1: 异步化必须按业务 key 做 serialization 才能保留 caller commit 顺序的 causal ordering

- **Severity**: high
- **Category**: architecture / concurrency
- **分诊**: fix
- **位置**: `server/internal/service/room_service.go:412-462` `runPostCommitAsyncPerRoom`

### 症状（Symptom）

User 在同一 roomID 上**快速** JoinRoom → LeaveRoom（或 leave → 立刻 join）触发两次 transaction commit 后，由两次 `runPostCommitAsync(ctx, fn)` 分别启动的 goroutine 在 Go runtime 调度下顺序不可控。最坏情况下：

- goroutine A（broadcast `member.joined`）在 goroutine B（broadcast `member.left`）之**后**才跑 → 该房间其他在线 client 收到事件序列 `member.left → member.joined`，与 caller transaction 顺序相反 → client roster 错乱（join 事件让 user 重新出现在 roster 上，但 user 此刻已不在房间）
- 或 stale `member.joined` 在 user 已离开后才到达 → client 显示"已离开的 user 还在房间"

类似 r2 异步化 → r3 修 stale subscription 的连锁问题：异步化每解决一个 [P1] 都可能引入新的 ordering regression。

### 根因（Root cause）

**fire-and-forget 异步化把"caller 调用顺序"与"goroutine 执行顺序"解耦**。`go func() { ... }()` 启动的 goroutine 由 Go runtime 任意调度，不保留 caller 的 happens-before 顺序。当多个 goroutine 操作**同一个共享受众**（同一 roomID 内的 client 群）时，它们的副作用顺序对受众可见 —— 此时必须显式 serialization。

r2 写的 `runPostCommitAsync` 没考虑这一点：

```go
go func() {
    detached := context.WithoutCancel(ctx)
    timedCtx, cancel := context.WithTimeout(detached, postCommitTimeout)
    defer cancel()
    fn(timedCtx)
}()
```

每个 goroutine 独立跑，没有任何同步原语保障"先 enqueue 的 fn 先完成"。Go runtime 调度顺序取决于 GOMAXPROCS / scheduler state / IO 等多因素，单元测试在轻负载下偶然按 caller 顺序执行 → 漏到 review 阶段才暴露。

**类比**：Story 10.6 r8 / r9 也踩过同一坑 —— presence reconcile hooks 异步化后破坏 add_online / remove_online 的执行顺序，最终用"per-userID mutex"解决。本次 11.8 r4 的解法是同一 pattern 在 roomID 维度的复刻。

### 修复（Fix）

`runPostCommitAsync` → `runPostCommitAsyncPerRoom`，加 roomID 参数走 per-room mutex serialization：

```go
type roomServiceImpl struct {
    ...
    perRoomMu sync.Map // roomID uint64 → *sync.Mutex
}

func (s *roomServiceImpl) runPostCommitAsyncPerRoom(
    ctx context.Context, roomID uint64, fn func(context.Context),
) {
    if s.postCommitWG != nil {
        s.postCommitWG.Add(1)
    }
    go func() {
        muIface, _ := s.perRoomMu.LoadOrStore(roomID, &sync.Mutex{})
        mu := muIface.(*sync.Mutex)
        mu.Lock()
        defer mu.Unlock()
        detached := context.WithoutCancel(ctx)
        timedCtx, cancel := context.WithTimeout(detached, postCommitTimeout)
        defer cancel()
        if s.postCommitWG != nil {
            defer s.postCommitWG.Done()
        }
        fn(timedCtx)
    }()
}
```

JoinRoom / LeaveRoom 两个 caller 改成传 `in.RoomID`：

```go
s.runPostCommitAsyncPerRoom(ctx, in.RoomID, func(detachedCtx context.Context) {
    s.broadcastMemberJoined(detachedCtx, in.RoomID, in.UserID)
})
```

**关键设计点**：

1. **`sync.Map[uint64, *sync.Mutex]`**：每个 roomID 一把 mutex，按需 lazily 创建（LoadOrStore）。`sync.Map` 比 `map + sync.RWMutex` 更适合"读多写少 + key 集合不断增长"的 key-value pattern。
2. **同一 roomID 串行**：保留 join → leave 的 causal ordering（修复 [P1]）。
3. **不同 roomID 并行**：不损失 fanout 吞吐（与"全局单一 mutex"的备选方案 path C 相比）。
4. **跨房间场景无问题**：user 从 room1 leave → 立刻 join room2，两个不同 roomID 的 mutex 仍可能交错执行，但语义上 OK —— 两次 broadcast 给**不同房间**的不同 client 集合，不存在因果依赖。
5. **mutex 不清理**（intentional）：节点 4 阶段房间 status 严格单调（active → closed 无回退），活跃 room 数量有界（同时在线用户上限 / 4 max_members）；不会无限增长。Future 节点 8+ 引入 dynamic room reuse 时再考虑 LRU eviction。

新增测试 case（`TestRoomService_PostCommit_PerRoomSerialization_PreservesCausalOrdering`）通过在 join 路径的 enrichment FindByID 上注入 channel-blocking gate，让 join goroutine 卡住后立即触发 leave goroutine —— 断言 release gate 后 broadcast 顺序为 `[member.joined, member.left]`，证明 per-room mutex 正确阻止了 leave goroutine 抢跑。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 把"原本同步执行的 hook 链路"切成 fire-and-forget 异步 goroutine 时，**必须**先回答"是否多次 caller 调用会落到同一受众 key"，若是 → **必须**用 `sync.Map[key, *sync.Mutex]` 做 per-key serialization，**禁止**裸 `go func() { ... }()`。
>
> **展开**：
> - **触发条件三件套**：(a) 异步化 hook（fire-and-forget goroutine）；(b) 同一业务 key（roomID / userID / sessionID 等）会被多次 caller 调用；(c) 受众（broadcast 接收方 / DB row / cache key）对**顺序敏感**。三件套同时成立时立即上 per-key mutex；缺任一项才考虑无锁。
> - **per-key 而非全局 mutex**：全局单一 mutex（path C）会让所有房间的 broadcast 串行 → 损失 fanout 并发；per-key 让不同 key 的 hooks 并行（path A）。
> - **同步 fallback**（path B）也是合法选项 —— 若 hook 总耗时短（< 100ms 且无 IO 阻塞），直接同步执行最简单。但本 case hook 含 `Session.CloseWithCode` 5s drain → 同步会让 HTTP 响应延迟 5s+，所以必须异步 + 加 mutex。
> - **WG 簿记位置**（test sync）：`wg.Add(1)` 必须在 caller goroutine（runPostCommitAsync 调用点）同步完成；`wg.Done()` 在 goroutine 内 defer。**不要**把 Add 放进 goroutine —— caller 立即 wg.Wait() 会 race 错过 Add，测试假成功。
> - **mutex defer 顺序**：goroutine 内 `defer Unlock` 必须最先 defer（最后执行），让 fn 跑完 + cancel + Done 都在 Unlock 之前 —— 否则下一个等 mutex 的 hook 会和已 Done 的 hook 短暂并发跑 fn。
> - **反例（具体）**：
>   - ❌ `go func() { fn() }()` 裸异步 —— 同 key 多 caller 时破坏 ordering（11-8 r4 [P1]）
>   - ❌ 全局单一 `sync.Mutex` 串行所有 hook —— 损失 fanout 并发（path C wontfix 理由）
>   - ❌ `wg.Add(1)` 放进 goroutine 内 —— caller wg.Wait() race miss（测试假成功）
>   - ❌ `defer Unlock; defer cancel; defer Done`（顺序倒）—— Done 后下一个 hook 拿到 mu 但前一个的 fn 仍在跑 race
>   - ✅ `sync.Map[key, *sync.Mutex] + LoadOrStore + Lock` 是**节点 4 / 5 阶段标准模式**（10-6 r8/r9 + 11-8 r4 都用同 pattern）

## Meta: 本次 review 的宏观教训（异步化反复踩坑的连锁规律）

Story 11.8 review 累计 4 轮（r1 → r4），每轮一条 [P1]，4 条 [P1] **全部**指向"异步化的副作用"：

- **r1**: nil sessionMgr guard（dependency injection 边界 vs fire-and-forget）
- **r2**: post-commit 用 request ctx → 被 cancel 误中断
- **r3**: 异步化破坏"leaver immediate detach" + "joiner not self-fanout" 两条同步可观察 invariant
- **r4**: 异步化破坏 caller commit 顺序的 causal ordering（本次）

教训：**把同步路径切到异步路径时，必须先列出"同步路径下成立的所有可观察 invariant"清单**，然后逐条问"异步后还成立吗"。常见 invariant 4 类：
1. **Cancel 信号正确传播**（r2 →  detached ctx + 独立 timeout）
2. **顺序 invariant**（r4 → per-key mutex）
3. **同步副作用立即生效**（r3 R2 → hybrid 切分，关键 op 留同步段）
4. **特定接收者排除**（r3 R1 → BroadcastToRoomExcept 显式 exclude）

未来 Claude 在 propose 任何"X → 异步化"重构时，**必须**在 propose 之前给出这 4 类 invariant 的逐条 audit 表 —— 漏 audit 任一类都会导致 review 反复 flag 同源风险。
