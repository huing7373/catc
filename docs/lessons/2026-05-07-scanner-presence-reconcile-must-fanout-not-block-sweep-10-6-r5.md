---
date: 2026-05-07
source_review: codex review (epic-loop r5) — /tmp/epic-loop-review-10-6-r5.md
story: 10-6-redis-presence-repo
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-07 — Scanner periodic reconcile 必须 fanout goroutine + per-call ctx timeout，不能在主 sweep 内同步调 Redis（10-6 r5）

## 背景

Story 10.6 r2/r3/r4 把 WS heartbeat scanner 加了"每 30s tick 对每个 active session 调 PresenceRenewer.AddOnline 重写 + 续期 Redis presence keys"路径。原版实装把 AddOnline **同步**写在 scanOnce 主 loop 内：

```go
for _, sess := range sessions {
    if idle <= timeoutMs {
        if s.renewer != nil {
            if !s.mgr.IsRegistered(ctx, sess.SessionID()) { continue }
            if err := s.renewer.AddOnline(ctx, sess.RoomID(), sess.UserID(), sess.SessionID()); err != nil {
                s.logger.Warn(...)
            }
        }
        continue
    }
    // idle 路径已经走 fanout goroutine close
    ...
}
```

codex 在 r5 指出：这条同步路径让一次 sweep 退化成 O(N session) × Redis latency，N 大或 Redis 慢时单 sweep > 30s tick → tail session 的 idle 检测被延迟、它们的 presence TTL 也错过 renew → flap offline。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | scanner periodic reconcile 必须 fanout 不阻塞主 sweep | P1 (high) | perf | fix | `server/internal/app/ws/heartbeat_scanner.go:314-321` |

修了 1 条 / defer 0 条 / wontfix 0 条。

## Lesson 1: 周期性 reconcile 不能在主 loop 同步调远程 I/O —— 必须 fanout goroutine + per-call ctx timeout

- **Severity**: high (P1)
- **Category**: perf
- **分诊**: fix
- **位置**: `server/internal/app/ws/heartbeat_scanner.go:314-321`

### 症状（Symptom）

WS heartbeat scanner 设计目标：30s tick 主 loop 在 microseconds 量级完成（O(N) 内存读 lastHeartbeatAt + dispatch fanout goroutine），让 idle 检测延迟严格 ≤ 30s + close fanout 上限。r2-r4 引入"每 active session 同步调 AddOnline" 后实际行为退化：

- 1 次 sweep 耗时 = N × Redis-RTT。Redis local < 1ms 看起来人畜无害；remote Redis 100ms RTT、N=1000 时单 sweep = 100s（>>> 30s tick），tail session 的 idle 检测被推迟到几个 tick 之后才轮到
- 同样的 tail session 也错过本 tick 的 AddOnline → presence TTL 5min 内若一直 tail，可能在下一次 reconcile 之前就过期 → ListOnline / IsOnline 误报 user offline
- 任意一个 session 的 AddOnline hang（Redis 病态卡住、网络超时未触发 default deadline）卡死整个主 loop —— 后续所有 session 既不被 reconcile 也不被 close

### 根因（Root cause）

写代码时把"reconcile"理解成"主流程的一步"（与 idle check 同级），把 idempotent + 容错的 Redis 写当成"快路径"塞进了同步循环。两个隐藏假设没被显式挑战：

1. **Redis call 的 latency SLO ≪ scanner tick**：单条命令通常 < 1ms 这是真的，但 N × latency 叠加后 SLO 不成立；同步 for-range 让总耗时是 sum 而不是 max
2. **Redis call 不会 hang**：默认 Go redis client 有连接池超时但单命令没有客户端 deadline；遇到病态（Redis half-closed connection、TCP retransmit）会卡到 OS-level TCP 超时（分钟级）

正确架构早就在隔壁路径有先例：close fanout（idle session 触发 CloseWithCode 4005）从 r1 起就是 fanout goroutine —— 同样的"per-session 远程 I/O 操作"，close 路径明白要 fanout 因为 close frame 写超时是 500ms 串行会卡 500s。reconcile 路径却没被这个先例提示，因为 AddOnline 的"通常 < 1ms"误导了风险判断。

更深的原因：性能直觉错把"快"等同于"快得可以同步"。Redis < 1ms 让人觉得"一行 sync call 没事"，但 sync call 的真实成本不是单条 latency，是 latency × N + worst-case hang。这套逻辑在网络 I/O 路径**几乎所有同步 for-range** 都适用。

### 修复（Fix）

**heartbeat_scanner.go**：把 reconcile 从主 loop 抽到 fanout goroutine（与 close fanout 同模式 + 同 wg）：

```go
if s.renewer != nil {
    s.wg.Add(1)
    go func(target *Session) {
        defer s.wg.Done()
        // 1. 入口 ctx-check：shutdown 时主 loop 已 ctx cancel，新 dispatched 路径
        //    立即 return 不做无意义 Redis I/O（与 close fanout 入口 ctx-check 同模式）
        select {
        case <-ctx.Done():
            return
        default:
        }
        // 2. IsRegistered guard 移到 goroutine 内（r4 P2 不变量保留）：让主 loop
        //    dispatch O(1) 不阻塞；snapshot 与 dispatch 之间的 race 仍由 IsRegistered
        //    捕捉避免复活 zombie presence
        if !s.mgr.IsRegistered(ctx, target.SessionID()) {
            return
        }
        // 3. per-call ctx timeout：单 hang 的 Redis 调用最多 2s 后 ctx.DeadlineExceeded
        //    自动退出，不影响其他 fanout，也不让 scanner.Run drain 时间无界增长
        callCtx, cancel := context.WithTimeout(ctx, presenceReconcileTimeout)
        defer cancel()
        if err := s.renewer.AddOnline(callCtx, target.RoomID(), target.UserID(), target.SessionID()); err != nil {
            s.logger.Warn("ws presence reconcile failed", ...)
        }
    }(sess)
}
continue
```

新增 `const presenceReconcileTimeout = 2 * time.Second`，与 main.go 现有 `presenceHookTimeout`（Register/Unregister hook 内 RemoveOnline 的 short-timeout）保持一致 —— 都是单条 Redis 命令的合理 SLO。

复用既有 `s.wg`（Story 10.4 r5 P2 引入用于 close fanout drain）：reconcile fanout 与 close fanout 共用一个 WaitGroup，scanner.Run 的 `defer s.wg.Wait()` 在 ctx cancel 时一并 drain 两条路径，shutdown 时序不变量保留。

测试侧：

- 加 3 个新测试：`ReconcileFanout_DoesNotBlockSweep`（N=20 + 100ms slowRenewer，主 loop < 200ms）/ `ReconcileFanout_PerCallCtxTimeout`（hang renewer，drain 在 ~2s 内返回）/ `Run_DrainsReconcileFanoutOnShutdown`（slowRenewer + Run 启动后 cancel ctx，drain 完所有 reconcile fanout 后才返回）
- 新 export_test.go helper `ScanOnceAndDrainForTest`（含 wg.Wait）+ `DrainFanoutForTest`（仅 wg.Wait） —— 让既有 reconcile 单测能可靠断言 fakeRenewer state；既有 `ScanOnceForTest` 保持 fire-and-forget（因为 r1 P1 TOCTOU race 测试依赖此语义在 ScanOnceForTest 返回后到 fanout 实际跑 recheck 之间塞 SetLastHeartbeatAt 模拟 race）
- 既有 5 个 reconcile 测试切换到 `ScanOnceAndDrainForTest`

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"对每个 X 周期性调远程 I/O 操作"路径** 时，**禁止**把远程 call **同步**塞进遍历主 loop —— 必须 fanout goroutine + per-call ctx timeout，且**与同 loop 内已有的 fanout 路径共用 WaitGroup**。
>
> **展开**：
> - 触发条件三项 AND：① 主 loop 是 O(N) 遍历切片 ② 每条调用是远程 I/O（Redis / DB / HTTP / RPC，任何"不在本进程内"的操作）③ 主 loop 自身有 SLO 上限（如 tick interval / shutdown 时限）。命中三条 → 必须 fanout
> - per-call ctx 用 `context.WithTimeout(parentCtx, X)` 派生；X 取**单条远程命令**的合理 SLO（local < 1ms 时 X = 2s 是合理的"卡住兜底"）—— 与 sweep tick 区分，不要把 tick interval 当 timeout
> - 用 `s.wg.Add(1)` 同步 + goroutine 内 `defer s.wg.Done()` 标准模式；主 Run 的 `defer s.wg.Wait()` 收敛 drain；同一 sweep 内多种 fanout 路径**共用同一 wg**（避免每路径单独 wg 让 shutdown 不变量散乱）
> - 入口 ctx-check（`select case <-ctx.Done(): return; default:`）是必备 —— ctx cancel 后已 dispatched 但未 IsRegistered/未跑实际 I/O 的 goroutine 立即放弃，不浪费 Redis I/O / 不延后 shutdown
> - 涉及 IsRegistered / state-membership 之类的 cheap 校验也搬到 goroutine 内 —— 让主 loop dispatch 严格 O(1) 不被锁竞争干扰；race 防护语义不丢（goroutine 内仍 check）
> - **反例 1**（本 review 修的）：
>   ```go
>   for _, sess := range sessions {
>       if err := redisRepo.AddOnline(ctx, ...); err != nil { logWarn }  // ❌ 主 loop 同步远程 I/O
>       continue
>   }
>   ```
>   病征：N=1000 + 100ms RTT → 100s sweep；任一 hang 卡死整个 sweep；同主 loop 内已有 close fanout 但 reconcile 没 fanout
> - **反例 2**（per-call ctx 缺失）：
>   ```go
>   go func() {
>       defer s.wg.Done()
>       redisRepo.AddOnline(ctx, ...)  // ❌ 直接用 parent ctx；hang 时本 goroutine 永久卡住
>   }()
>   ```
>   病征：fanout 解决了 N 序列耗时，但单 hang 让 wg.Wait drain 无界增长 → shutdown 卡死
> - **反例 3**（per-fanout 单独 wg 让 shutdown 不变量散乱）：
>   ```go
>   var closeWg, reconcileWg sync.WaitGroup
>   defer closeWg.Wait()       // ❌
>   defer reconcileWg.Wait()   // ❌ 两个 wg 让 drain ordering 隐式依赖 defer LIFO
>   ```
>   病征：未来加第三种 fanout 路径（如 metrics emit）时容易漏 wg；review 时变量越多越难看出"是否所有 fanout 都被 drain"
> - **正例**：本修中的 fanout 三件套（wg.Add(1) 同步 / 入口 ctx-check / per-call WithTimeout / defer wg.Done()）+ 与既有 close fanout 共用 s.wg + 主 Run 的 `defer s.wg.Wait()` 收敛 drain
> - 测试维度：必须有 ① **主 loop 不阻塞**（slow renewer N 个 → 主 loop ≪ N × delay）② **per-call timeout 生效**（hang renewer → drain ~ timeout）③ **shutdown drain**（cancel ctx → Run 在 reasonable 时间内返回）三类 case；缺任意一类则其他两类的修都可能在未来回归

## Meta: 本次 review 的宏观教训

性能直觉的"single call 快"≠"sync call 安全"。Redis < 1ms 的本能反应让我们倾向于把它当 cheap call 同步用，但 N × cheap = expensive，cheap × hang = catastrophic。**任何对 N 个对象做远程 I/O 的循环，默认都应该是 fanout，除非 N 有强上界 + worst-case latency 可证明 ≪ tick SLO**。这套规则在 close fanout（idle session）路径已经被 r1 验证过；本次同 sweep 内的 reconcile 路径漏了同样的提醒，是因为我们把"周期性 reconcile"和"事件驱动 close"看成两类操作，但它们在 latency 风险维度其实同构 —— 都是 per-session 远程 I/O，都需要 fanout。

下次写"周期性周期遍历"主 loop 时，先把"任何 per-X 远程 I/O 都得 fanout"作为默认假设，再问"有没有 N 上界 + 同步可证更优"反向论证 —— 而不是反过来。
