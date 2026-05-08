---
date: 2026-05-09
source_review: codex review r2 输出文件 `/tmp/epic-loop-review-11-8-r2.md`（Story 11.8 review round 2）
story: 11-8-成员加入-离开-ws-广播
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-09 — post-commit fire-and-forget 必须 detached ctx + 独立 goroutine + timeout 兜底（11-8 r2）

## 背景

Story 11.8 落地 HTTP join / leave 完成后的 post-commit WS 广播（`member.joined` / `member.left`）+ leaver session close 4007。codex r1 之后 working tree clean 进 review r2；r2 codex 在末尾 `^codex$` 之后给出 2 条 P1 finding，全部围绕**事务后 fire-and-forget hook 的实装语义**：

1. `closeLeaverSession` 同步调 `Session.CloseWithCode` 会等 write loop drain（默认 ~5s WS write timeout），违反 fire-and-forget 语义、阻塞 HTTP 200。
2. `broadcastMemberJoined` / `broadcastMemberLeft` 用 request ctx，client 断开 / handler deadline cancel 后 user/pet lookup 会 fail "context canceled"，broadcast 静默 skip。

两条 finding 同源同修：post-commit hook 的 **ctx 处理 + 执行模型**两个轴向都被原实装做错了。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | closeLeaverSession 同步 CloseWithCode 阻塞 HTTP 200（违反 fire-and-forget） | high | architecture | fix | `server/internal/service/room_service.go:923-971` |
| 2 | post-commit broadcast 用 request ctx，cancel 后 lookup fail "context canceled" | high | architecture | fix | `server/internal/service/room_service.go:835-994` |

## Lesson 1: post-commit fire-and-forget hook 必须**整体异步化**（独立 goroutine）

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/service/room_service.go:559, 663-664`（事务后 hook 调用点）

### 症状（Symptom）

LeaveRoom 事务 commit 后，`s.closeLeaverSession(ctx, ...)` 同步调用：

```go
// 修复前
s.closeLeaverSession(ctx, in.RoomID, in.UserID)
s.broadcastMemberLeft(ctx, in.RoomID, in.UserID)
return &LeaveRoomOutput{...}, nil
```

`closeLeaverSession` 内部 `target.CloseWithCode(4007, ...)` 会等 write loop 把 close frame drain 完（WS write deadline 默认 ~5s）；最坏情况下 HTTP 200 响应会被延迟 ~5s + 后续 broadcast 也被串在 close 后（虽然语序正确但被 close 阻塞）。client 体感 leave 操作"卡了 5s 才响应"。

### 根因（Root cause）

`fire-and-forget` 这个词在 V1 §10.5 步骤 7 / 8 钦定 **"broadcast / close 失败不影响 HTTP 200"**，但实装把它**只**理解成"忽略 error"，没意识到**fire-and-forget 还包含"不阻塞 caller 等待结果"**这一层语义。同步调用即使忽略 error 也仍然在调用栈里阻塞主路径，违反 fire-and-forget 的 latency 维度。

具体到 close 4007 路径：`Session.CloseWithCode` 是设计为 best-effort 通知 client，本身就允许 client 已断开 / write 超时；这种"我尽力告诉你但不等你确认"的语义必须**异步**执行，否则 server 主路径会被 client 端的网络状况倒挂。

### 修复（Fix）

把 post-commit hook 整体放进一个独立 goroutine，let caller 立刻继续返回：

```go
// 修复后（room_service.go LeaveRoom 末尾）
s.runPostCommitAsync(ctx, func(detachedCtx context.Context) {
    s.closeLeaverSession(detachedCtx, in.RoomID, in.UserID)
    s.broadcastMemberLeft(detachedCtx, in.RoomID, in.UserID)
})
return &LeaveRoomOutput{...}, nil
```

`runPostCommitAsync` 是新引入的 helper：

```go
func (s *roomServiceImpl) runPostCommitAsync(ctx context.Context, fn func(detachedCtx context.Context)) {
    if s.postCommitWG != nil {
        s.postCommitWG.Add(1)
    }
    go func() {
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

JoinRoom 同模式改为 `s.runPostCommitAsync(ctx, func(c) { s.broadcastMemberJoined(c, ...) })`。

**关键约束保留**：close 4007 必须先于 broadcast member.left（V1 §10.5 r13 钦定的顺序约束），所以 close + broadcast 放进**同一 goroutine** 顺序执行 —— 而不是各起一个 goroutine（那样 r13 顺序无法保证）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **service 层事务 commit 之后调用 fire-and-forget hook 时**，**必须**把 hook 整体放进独立 goroutine（不阻塞 caller 主路径），**禁止**让 caller 同步等待 hook 完成 —— 即使 hook 内部已经"忽略 error"。

> **展开**：
>
> - "fire-and-forget" 包含**两层语义**：(1) error 被忽略（不传播给 caller）；(2) 调用 latency 不计入 caller 主路径。两层缺一不可，光做 (1) 不够。
> - 如果 hook 内部有**多个顺序约束**（如 r13: close 必须先于 broadcast），把整组顺序操作**放进同一个 goroutine 内顺序调用**，不要拆成多个独立 goroutine 并行（那样会破坏顺序约束）。
> - hook 涉及 WS write / network I/O / SessionManager close 这类**有内部超时**的调用时，"主路径同步等" 在最坏情况下会拖累 P99 latency 5s+，client 体感"接口卡了"。这种 case 必须异步化 —— 即使 happy 路径下耗时只有 ms 级，最坏 case 也不能进 main path。
> - **反例**：
>   ```go
>   // ❌ 错：同步调用 fire-and-forget hook，让 caller 等 close frame drain
>   s.closeLeaverSession(ctx, ...)
>   s.broadcastMemberLeft(ctx, ...)
>   return out, nil
>   ```
>   ```go
>   // ❌ 错：拆成两个独立 goroutine，破坏 r13 顺序约束（broadcast 可能先于 close 跑）
>   go s.closeLeaverSession(ctx, ...)
>   go s.broadcastMemberLeft(ctx, ...)
>   ```
>   ```go
>   // ✅ 对：单个 goroutine 内保持顺序 + detached ctx + timeout
>   s.runPostCommitAsync(ctx, func(c) {
>       s.closeLeaverSession(c, ...)
>       s.broadcastMemberLeft(c, ...)
>   })
>   ```

## Lesson 2: post-commit goroutine 必须用 detached ctx（`context.WithoutCancel`）+ 独立 timeout 兜底

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/service/room_service.go:835-994`（broadcastMemberJoined / closeLeaverSession / broadcastMemberLeft 的 ctx 用法）

### 症状（Symptom）

`broadcastMemberJoined(ctx, ...)` 内部调 `s.userRepo.FindByID(ctx, joinerUserID)` + `s.petRepo.FindDefaultByUserID(ctx, joinerUserID)` 用的是 caller 传入的 request ctx。如果 client 在 HTTP 响应到达前主动断开，或 handler 上层有 deadline 已经到点，request ctx 会被 cancel，post-commit lookup 立刻 fail "context canceled" → broadcast 静默 skip / payload 字段空。

closeLeaverSession 同样问题：`s.sessionMgr.ListSessionsByRoomID(ctx, roomID)` / `s.sessionMgr.Unregister(ctx, sessionID)` 用 request ctx，client 断开后无法清理 leaver session，leaver session 残留 SessionManager 索引。

### 根因（Root cause）

事务 commit 之后的工作（broadcast / cleanup）**与原 request 不再有"必须共完成或共失败"的耦合关系** —— commit 已经把 authoritative state 持久化，HTTP 200 也准备返回，broadcast 只是**事件通知**而非事务的一部分。但实装把 request ctx 透传给 post-commit hook，把"client 端的网络状况"耦合进了"server 端是否能完成事件通知"，这是不正确的依赖反向。

Go 1.21 引入 `context.WithoutCancel` 正是为了解决这类场景：**保留 ctx 的 values（trace ID / request ID 等 propagation）但移除 cancel 信号**。比 `context.Background()` 更优 —— 后者完全 detached 但丢失 trace/observability propagation。

但单纯 WithoutCancel 又会引入新风险：**goroutine 永不被取消** = goroutine 泄漏可能。例如 DB 死锁 / SessionManager 内部死循环 / 上游服务卡死，goroutine 永远不返回。所以 detached ctx 必须**配合独立 timeout** 兜底。

### 修复（Fix）

`runPostCommitAsync` helper 同时做两件事：

```go
go func() {
    // 1. detached ctx：保留 values，移除 cancel 信号
    detached := context.WithoutCancel(ctx)
    // 2. 独立 timeout：避免 goroutine 泄漏
    timedCtx, cancel := context.WithTimeout(detached, postCommitTimeout)
    defer cancel()
    if s.postCommitWG != nil {
        defer s.postCommitWG.Done()
    }
    fn(timedCtx)
}()
```

`postCommitTimeout = 10 * time.Second` —— 估算 user/pet lookup（~ms 级）+ 1 次 marshal + broadcastFn fanout + Session.CloseWithCode 含 ~5s WS write timeout，worst-case ~6s；取 10s 留冗余。

测试同步：因为 hook 现在异步，测试需要等 goroutine 完成才断言副作用。引入 `SetPostCommitWaitGroupForTest(svc, wg)` exported helper —— 注入 `*sync.WaitGroup`，`runPostCommitAsync` 在 production 路径 wg=nil 零开销，测试路径 wg 非 nil 用 `Add/Done` 让 `wg.Wait()` 阻塞到 goroutine 完成。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **post-commit / fire-and-forget goroutine 内**，**必须**用 `context.WithoutCancel(ctx)` + `context.WithTimeout` 组合派生 ctx，**禁止**直接透传 request ctx，**禁止**只用 `context.Background()` 丢失 propagation values。

> **展开**：
>
> - 三层选择：
>   - `request ctx`（原样透传）→ ❌ goroutine 会被 client 断开 / handler deadline 误中断
>   - `context.Background()` → ⚠️ 丢失 trace ID / request ID，observability 断链
>   - `context.WithoutCancel(ctx)` → ✅ Go 1.21+ 推荐，保留 values 但移除 cancel
> - WithoutCancel 是必要但不充分条件：必须再叠 `context.WithTimeout`，否则 DB / 上游服务卡死会让 goroutine 泄漏。timeout 估算 = (worst-case 业务延迟 × 1.5) 或常用 10s 起步。
> - 测试侧需要**显式同步机制**等 goroutine 完成 —— 不能用 `time.Sleep(100ms)` 这种 flaky polling；用 `*sync.WaitGroup` / channel signal / 测试专用 hook 注入。production 路径不能引入这种同步开销（fire-and-forget 必须真 fire-and-forget）。
> - 业务字段 lookup（user / pet / room enrichment）放进 post-commit hook **不能依赖 request ctx 仍 alive** —— 即使 happy 路径几乎总是 alive，也必须在 ctx model 上把这层依赖切断，才符合 fire-and-forget 语义。
> - **反例**：
>   ```go
>   // ❌ 错：透传 request ctx 进 post-commit goroutine
>   go s.broadcastMemberJoined(ctx, in.RoomID, in.UserID)
>   // 结果：client 断开后 userRepo.FindByID 返 "context canceled"，broadcast 静默 skip
>   ```
>   ```go
>   // ❌ 错：只用 Background，丢失 trace ID
>   go s.broadcastMemberJoined(context.Background(), in.RoomID, in.UserID)
>   ```
>   ```go
>   // ❌ 错：只 WithoutCancel 不加 timeout
>   go s.broadcastMemberJoined(context.WithoutCancel(ctx), in.RoomID, in.UserID)
>   // 结果：DB 死锁时 goroutine 永不返回 → goroutine 泄漏
>   ```
>   ```go
>   // ✅ 对：detached + timeout + 测试同步 hook
>   go func() {
>       detached := context.WithoutCancel(ctx)
>       timedCtx, cancel := context.WithTimeout(detached, postCommitTimeout)
>       defer cancel()
>       fn(timedCtx)
>   }()
>   ```

---

## Meta: 本次 review 的宏观教训

r2 两条 finding 都集中在 **post-commit hook 的执行模型**这一个抽象上 —— 不是各自独立的 nit，而是**同一个抽象漏洞的两个症状**。这表明：

> 当 review 多条 finding 都指向同一段代码（这里 = JoinRoom / LeaveRoom 的事务后阶段）时，**通常存在一个统一的修复抽象**（这里 = `runPostCommitAsync` helper），用这个抽象一次解决所有相关 finding，比逐条打 patch 更彻底。

future Claude 在 fix-review 时遇到"多条 P1 finding 都在同一函数 / 同一事务 phase"的情况，**先不要着急逐条改**，停下来想：是不是缺一个公共抽象？提取 helper 通常能让全部 finding 一次清掉，且不会留 patchwork 给下次 review 再 flag。
