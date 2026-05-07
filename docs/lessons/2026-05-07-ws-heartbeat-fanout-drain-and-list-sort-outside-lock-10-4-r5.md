---
date: 2026-05-07
source_review: codex review of Story 10.4 round 5 (file: /tmp/epic-loop-review-10-4-r5.md)
story: 10-4-心跳框架
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-07 — heartbeat fanout 必须用 WaitGroup drain & List 操作把 sort 移到 RUnlock 之后（10-4 r5）

## 背景

Story 10.4（心跳框架）r5 review 抓出两个 P2 残余问题：

1. r3 加了 fanout goroutine 入口/recheck 的 ctx-check 让 ctx-cancelled 路径立即 return；r4 把 heartbeat ctx 挂到 main signal ctx 上让 SIGTERM 立即生效。但仅 ctx-check 不够 —— "已通过最后一次 ctx-check 即将调 CloseWithCode" 的 goroutine 在 ctx cancel 后仍会 emit 4005。SIGTERM 落在 sweep 期间 → shutdown 仍能推 4005 触发 client 重连风暴。
2. r4 引入 `ListAllSessions` 让 heartbeat scanner 每 30s 拿全量 session 切片；实装 `defer m.mu.RUnlock()` 让 O(N log N) sort 在 RLock 下跑。Register/Unregister 需要 write lock 同一 mu，sessions 多时整个 sweep 期间连接/断连被周期性阻塞。`ListSessionsByRoomID` 同模式。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | scanner.Run 退出前必须用 WaitGroup drain in-flight fanout goroutines | medium | architecture (shutdown coordination) | fix | `server/internal/app/ws/heartbeat_scanner.go:191-256` |
| 2 | ListAllSessions / ListSessionsByRoomID 必须把 O(N log N) sort 移到 RUnlock 之后 | medium | perf (lock contention) | fix | `server/internal/app/ws/session_manager.go:301-345` |

修了 2 条 / defer 0 条 / wontfix 0 条。`ListSessionsByRoomID` 同模式问题一并修了（review 建议）。

## Lesson 1: heartbeat fanout 必须用 WaitGroup drain，仅 ctx-check 不够

- **Severity**: medium（P2 - shutdown 期间 4005 race）
- **Category**: architecture（shutdown coordination / goroutine lifecycle）
- **分诊**: fix
- **位置**: `server/internal/app/ws/heartbeat_scanner.go:Run`、`scanOnce`

### 症状

SIGTERM 到达后 main 的 signal ctx 立即 cancel（r4 修复后），scanner.Run 主循环立即 return；但已经过 `scanOnce` dispatch 的 fanout goroutines 中，凡是已经通过最后一次 `select { case <-ctx.Done(): return; default: }` check + 还没调 `CloseWithCode` 的，都会继续 emit 4005 close frame。SIGTERM 命中 sweep 期间 → 正常下线的 client 仍收到 4005 "heartbeat timeout" → 触发自动重连风暴。

### 根因

修 r3 时的 mental model 是 "fanout goroutine 自己 check ctx 就能放弃 close path"。这个想法在静态视角下成立 —— 但 ctx-check 与 CloseWithCode 之间存在调度间隙：

```go
select {
case <-ctx.Done():
    return
default:
}
// ←—— ctx 在这里 cancel 了，本 goroutine 仍会调下一行
s.CloseWithCode(4005, "heartbeat timeout")
```

ctx-check 只能让 "ctx 在 check 那一刻已经 Done" 的 goroutine 退出；窗口内（check 与下一动作之间的 ns 级间隙）cancel 的 ctx 不会影响已通过 check 的 goroutine。实际生产中 sessionMgr 含 1000 sessions 时，一次 sweep dispatch 1000 fanout，SIGTERM 落在这 1000 goroutine 的 ctx-check **之后**、CloseWithCode **之前**的概率非零。

第二个思维漏洞：把 "Run 返回" 当成 "scanner 全部安静" —— 但 fanout 是 fire-and-forget 派生的独立 goroutine，Run 返回 ≠ goroutine 全跑完。修 r3 时只关注了"让 ctx-cancelled 路径 return"，没追问"已经过 ctx-check 的 goroutine 怎么办"。

### 修复

加 `sync.WaitGroup` 跟踪 in-flight fanout，Run defer wg.Wait() drain：

```go
type HeartbeatScanner struct {
    ...
    wg sync.WaitGroup
}

func (s *HeartbeatScanner) Run(ctx context.Context) {
    ticker := time.NewTicker(s.interval)
    defer ticker.Stop()
    defer s.wg.Wait()  // ← 退出前等所有 fanout 跑完
    for {
        select {
        case <-ctx.Done(): return
        case now := <-ticker.C: s.scanOnce(ctx, now)
        }
    }
}

func (s *HeartbeatScanner) scanOnce(ctx context.Context, now time.Time) {
    ...
    s.wg.Add(1)  // ← Add 在 dispatch 前同步调用
    go func(target *Session) {
        defer s.wg.Done()  // ← Done 在 goroutine defer
        // ctx-check + recheck + CloseWithCode 既有逻辑
    }(sess)
}
```

两条防线：
- fanout 入口 ctx-check 让 ctx-cancelled 路径立即 return（绝大多数 goroutine 走这条快速路径）
- Run defer wg.Wait() 让残余 goroutine（已通过 ctx-check 的）跑完 CloseWithCode 才让 Run 返回

加了一条回归测试 `TestHeartbeatScanner_Run_DrainsFanoutBeforeReturn`：起 N=10 stale session → cancel ctx → 断言 Run 在 2s 内返回（说明 wg.Wait drain 工作）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写 **fire-and-forget fanout goroutine** 时，**禁止**用单纯的 ctx-check 当作"shutdown 期间不再 emit side-effect"的保证；**必须**配合 `sync.WaitGroup` 让 dispatcher 退出前 drain 所有 in-flight goroutine。
>
> **展开**：
> - ctx-check 是必要的（让 ctx-cancelled 时绝大多数 goroutine 立即 return），但**不充分**（已通过 ctx-check 的 goroutine 会跑到底）
> - WaitGroup drain 是充分的（Run 返回 = wg 归零 = 所有 Add 都有 Done）；与 ctx-check 叠加使用：ctx-check 让快速路径瞬间退出降低 wg.Wait 时间，wg.Wait 兜底"已通过 ctx-check 的 goroutine"
> - 标准模式：`wg.Add(1)` 在 dispatcher 主线程同步调用（Add before go），`defer wg.Done()` 在 goroutine 内（Done in defer）—— 这是 sync.WaitGroup 文档钦定的正确用法
> - **反例 1**：在 goroutine 内 `wg.Add(1); defer wg.Done()` —— Add 与 dispatcher 的 wg.Wait 之间存在 race（Wait 可能在 Add 之前就观察到 wg=0 错误返回）
> - **反例 2**：fire-and-forget fanout 不带任何 drain 机制，靠 ctx-check 兜 shutdown —— 仅在 fanout goroutine **没有任何外部副作用**（如不写网络 / 不改持久化状态 / 不调用第三方）时勉强可以接受；只要有副作用就需要 WaitGroup drain
> - 检测信号：`go func() { ... }()` + 函数体内有 `WriteMessage / WriteControl / db.Exec / cache.Set / metrics.Inc` 这类有副作用的调用，**且** dispatcher 自身有 lifecycle（Run / Close / Stop），就必须 WaitGroup drain
> - 配套测试：drain 测试不能用 sleep + 观察副作用判断（race-prone），应用 `select { case <-runDone: case <-time.After(...): t.Fatalf }` 加延迟边界断言；正确性靠 `go test -race` + WaitGroup 自身的正确语义兜底

## Lesson 2: 热路径 List 操作把 O(N log N) sort 移到锁外

- **Severity**: medium（P2 - sweep 期间 lock contention）
- **Category**: perf（lock contention / hot path latency）
- **位置**: `server/internal/app/ws/session_manager.go:ListAllSessions`、`ListSessionsByRoomID`

### 症状

`ListAllSessions` 用 `defer m.mu.RUnlock()` 让整个函数体（包括 O(N log N) sort）在 RLock 下跑。HeartbeatScanner 每 30s 调一次；N=1000 时 sort 几个 ms，但 RLock 持有期间 Register/Unregister 的 write lock（同一 mu）被 starve。结果是每 30s 一次的 sweep 让 N 大时连接/断连周期性卡顿（后续 Story 10.5 BroadcastToRoom 还会更频繁调 `ListSessionsByRoomID`，问题更严重）。

`ListSessionsByRoomID` 同 anti-pattern。

### 根因

写 `ListAllSessions` 时（Story 10.4 r4 加）直接 copy `ListSessionsByRoomID` 的实装模式 —— 而 `ListSessionsByRoomID` 本身就是 r5 review 才识别出来的 anti-pattern。一个早期细节漏洞通过 copy-paste 扩散到第二处，两处都暴露在新 hot path 下。

更深层的 mental error：把 sort 当作"List 的固定步骤"放在锁内，没有问"sort 真的需要持锁吗？"答案是不需要 —— 锁的语义是"保护 manager 内部 map 不被遍历期 mutation"，而 sort 只 access 已经 copy 出来的切片，不再触碰 manager 内部 map；切片元素是 *Session 指针，sessionID 是 unexported + Register 一次性赋值后不变，sort 比较器（`out[i].sessionID < out[j].sessionID`）安全。

锁内 sort 的唯一"理由"：避免再写一个 RUnlock 的代码路径（程序员懒）。这个理由在 perf-sensitive 路径下不成立。

### 修复

把 sort 挪到 RUnlock 之后；锁内仅 copy 引用切片：

```go
// before
func (m *sessionManager) ListAllSessions(ctx context.Context) []*Session {
    m.mu.RLock()
    defer m.mu.RUnlock()
    if len(m.sessionsByID) == 0 {
        return []*Session{}
    }
    ids := make([]string, 0, len(m.sessionsByID))
    for id := range m.sessionsByID { ids = append(ids, id) }
    sort.Strings(ids)  // ← 持锁 sort
    out := make([]*Session, 0, len(ids))
    for _, id := range ids { out = append(out, m.sessionsByID[id]) }
    return out
}

// after
func (m *sessionManager) ListAllSessions(ctx context.Context) []*Session {
    m.mu.RLock()
    if len(m.sessionsByID) == 0 {
        m.mu.RUnlock()
        return []*Session{}
    }
    out := make([]*Session, 0, len(m.sessionsByID))
    for _, s := range m.sessionsByID { out = append(out, s) }
    m.mu.RUnlock()
    // 锁外 sort
    sort.Slice(out, func(i, j int) bool { return out[i].sessionID < out[j].sessionID })
    return out
}
```

`ListSessionsByRoomID` 同模式修。RLock 持锁时间从 O(N log N) 退化到 O(N) copy，N=1000 量级几十微秒到一两毫秒，不再阻塞 Register/Unregister。

加了存活性测试 `TestSessionManager_ListAllSessions_NoLockHeldDuringSort`：50 session preset → 并发 list 干扰下做 30 次 Register → 总耗时 < 30s（CI 兜底）。真正的并发正确性由 -race 兜底。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写"持锁拿 snapshot + 排序/格式化/序列化"这类 List/Dump 操作时，**必须**先问"排序/格式化的输入是 snapshot 切片还是仍在用受锁保护的 map？"如果只 access snapshot 切片，**必须**把排序挪到 RUnlock 之后；**禁止**把"sort 放锁内"作为默认模板。
>
> **展开**：
> - 锁的语义是"保护共享状态被遍历期 mutation"；snapshot 切片是 caller 私有的，与共享状态解耦 → 锁外操作安全
> - 锁内仅做"必须持锁的最小工作"：copy map → slice、判断空集等。其他工作（sort / encode / log format / 计算指纹）全部锁外
> - 检测信号：函数体里既有 `m.mu.RLock()` + `defer m.mu.RUnlock()` **又有** `sort.X()` / `json.Marshal` / `fmt.Sprintf` / `proto.Marshal` 这类纯计算调用，几乎一定能优化（移到锁外）
> - **反例 1**：`defer m.mu.RUnlock()` + `sort.Strings(ids)` —— sort 持锁，N 大时阻塞 writer
> - **反例 2**：`defer m.mu.RUnlock()` + `json.Marshal(snapshot)` —— marshal 持锁更糟（CPU + 内存分配，N×k bytes 写入临时 buffer 全发生在锁内）
> - **反例 3**：`defer m.mu.RUnlock()` + `for _, s := range snapshot { s.HeavyMethod() }` —— 调用方法本身可能再去 lock 别的资源，持本锁 + 等其他锁 = 死锁
> - 性能锚定：N=1000 量级，sort 持锁 ≈ 100 μs ~ 1 ms，每 30s 一次 sweep = 0.003% 平均 lock contention，看似不足为虑；但当 BroadcastToRoom 把 List 提到每秒级调用频率，contention 立即放大 1000 倍 = 3% 持续 starve writer，新连接卡顿肉眼可见
> - copy-paste 警惕：抽象 "List X" 出新方法时（如 ListAllSessions copy 自 ListSessionsByRoomID）必须**重新审视模式正确性**，不能假设 source 模式就一定正确 —— source 可能就是有问题的 pattern

---

## Meta: 本次 review 的宏观教训

两条 finding 都是 r4 引入的代码（heartbeat scanner ctx 派生 / ListAllSessions 新方法）的 follow-on 问题。r1→r5 共 5 轮才稳定下来，每轮修一个语义维度但漏邻近维度。

具体到 r5：
- ctx-cancellation 协议正确性（fanout 不再 emit 4005）r3 修了 ctx-check，但漏了 "already-past-check goroutine" 的 drain（r5 P2 #1）
- 新加 ListAllSessions 时 copy 旧 ListSessionsByRoomID 模式（持锁 sort），把旧 anti-pattern 扩散到新 hot path（r5 P2 #2）

预防规则：

> 未来 Claude 在 r2+ review 修复时（特别是 shutdown / lifecycle / lock 这类多语义维度纠缠的代码），**必须**做以下检查再 commit：
>
> 1. **追溯链**：本次修的代码与上一轮修复有什么关系？这一轮修了哪个语义维度？相邻维度（lifecycle / drain / ordering / lock-contention）是否还有问题？
> 2. **新代码不可 copy 旧 anti-pattern**：r4 加 ListAllSessions 时直接 copy ListSessionsByRoomID —— 后者本身就有 perf 问题，r5 才暴露。Copy 旧实装模式时必须重新审视模式正确性
> 3. **ctx-check 不是万能的**：用 ctx-cancel 控制 goroutine lifecycle 时，仅靠 ctx-check 只能让 "check 时已 cancel" 的 goroutine 退出；窗口内（check 与下一动作之间的 ns 级间隙）cancel 的 goroutine 不会被覆盖。需要 WaitGroup drain 兜底
> 4. **持锁 + 计算**几乎一定可优化：grep `defer m\.mu\.R?Unlock\(\)` 函数体内出现 `sort\.|json\.Marshal|fmt\.Sprintf` 都是潜在 hot path latency 退化点
