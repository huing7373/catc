---
date: 2026-05-07
source_review: codex review (epic-loop fix-review r9 — /tmp/epic-loop-review-10-6-r9.md)
story: 10-6-redis-presence-repo
commit: adcb1d3
lesson_count: 1
---

# Review Lessons — 2026-05-07 — presence hook 必须改成同步调用 + 与 scanner reconcile 共享 per-user mutex（10-6 r9）

## 背景

Story 10-6（Redis presence repo）review r1-r8 一路演进，每一轮都为新发现的 race
打补丁：r1 加 sessionID guard；r2 加 scanner TTL renewal；r3 加 self-heal AddOnline；
r4 加 IsRegistered guard；r5 加 fanout + per-call ctx timeout；r7 加 presenceHooksWG；
r8 加 per-user mutex 在 hook goroutine 之间。codex r9 指出**两条独立 goroutine 树
（hook 路径 + scanner reconcile 路径）即使加了 per-user mutex 仍然 race** —— 因为
mutex 仅保证互斥不保证 FIFO，且 scanner 没有共享同一把锁 → IsCurrentForUser 仍然
是 TOCTOU snapshot。结论：fire-and-forget 模式本质上不能消除 ordering race，必须
改成同步调用 + 共享锁的结构性修法。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | userKeyedMutex 不保证 FIFO order — quick connect+close 同 user 路径下 RemoveOnline 可能先 Lock 让 AddOnline 复活 presence | high | architecture | fix | `server/cmd/server/main.go:335-358` |
| 2 | scanner reconcile 不共享 hook 的 user mutex — 即使 IsCurrentForUser 也是 TOCTOU；snapshot → guard 通过 → hook 跑完 RemoveOnline → scanner AddOnline 复活已离线 presence | high | architecture | fix | `server/internal/app/ws/heartbeat_scanner.go:377-383` |

## Lesson 1: presence hook 改同步调用 + scanner reconcile 共享 per-user mutex

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/cmd/server/main.go:335-380` + `server/internal/app/ws/heartbeat_scanner.go:103-260` + `server/internal/app/ws/heartbeat_scanner.go:356-398`

### 症状（Symptom）

两条独立 goroutine 树（hook adapter / scanner reconcile）对同一 Redis presence
状态做读改写但缺乏统一的 critical section：

1. **userKeyedMutex 不保证 FIFO**：同 user 连续 connect+disconnect → Register
   启 goroutine A 调 AddOnline + Unregister 启 goroutine B 调 RemoveOnline；两
   goroutine racing for `mu.Lock()`，谁先 Lock 谁先跑。如果 B 先 Lock →
   RemoveOnline 先跑（user 还没 Add 过，no-op）→ A 后跑 AddOnline 复活已离线
   session 的 presence 直到 TTL 5min 过期。

2. **scanner reconcile 不共享 hook 的 user mutex**：scanner 走另一组 goroutine 树，
   IsCurrentForUser 检查后到 AddOnline 之间，hook 的 RemoveOnline 仍可能跑完 →
   scanner AddOnline "复活" hook 已经清掉的 presence。即使 IsCurrentForUser
   guard 也是 TOCTOU snapshot：check 时 true，AddOnline 之前 false。

测试集 fakeRenewer / miniredis 都跑得过 —— 因为这些都是窗口期 nanos 量级的
ordering race，单测稳定 reproducer 困难，但 production 高并发下必发。

### 根因（Root cause）

**fire-and-forget 模式本质上无法消除 ordering race**：

- mutex 只能保证两个 goroutine **不同时**进 critical section，**不**保证它们按
  调用方的语义顺序进入（Go runtime scheduler 决定）
- 调用方语义顺序（Register hook fire 完才 fire Unregister hook，由 SessionManager
  锁内串行触发）→ goroutine dispatch 顺序 → goroutine 抢锁顺序，每一跳都不传递
  顺序保证
- 两条独立 goroutine 树之间没有 single source of truth 的 critical section，
  无论加多少 IsCurrent guard / 顺序检查，每一 round 都会暴露下一个 race window
- r6-r8 一路打补丁的 lesson：Add hook → race → guard1 → race → guard2 → race ...
  每加一层都暴露下一层，根本没有终点

**结构性修法**：消灭 fire-and-forget。把 hook adapter 改成**同步调用**（调用方
锁内串行 = AddOnline 顺序天然 FIFO，Register 锁释放前 AddOnline 必跑完），同时
让 scanner reconcile **共享同一把 per-user mutex** 与 hook 串行化（scanner 持锁
后再 IsCurrentForUser 重新校验，hook 跑完 RemoveOnline 释放锁 → scanner 看到
session 已 unregister → 跳过 AddOnline 不污染）。

### 修复（Fix）

**1. `server/cmd/server/main.go`** — 把 hook 改同步，删 `presenceHooksWG`，把
   userPresenceMu 暴露给 scanner：

```go
// before（r8 fire-and-forget + 自己的 mutex）：
var presenceHooksWG sync.WaitGroup
var userPresenceMu userKeyedMutex
sessionMgr := wsapp.NewSessionManager(
    wsapp.WithRegisterHook(func(s *wsapp.Session) {
        presenceHooksWG.Add(1)
        go func() {
            defer presenceHooksWG.Done()
            mu := userPresenceMu.lockFor(s.UserID()); mu.Lock(); defer mu.Unlock()
            ...AddOnline...
        }()
    }),
    ...
)

// after（r9 同步 + 共享 mutex）：
var userPresenceMu userKeyedMutex // 实装 ws.UserPresenceMutex 接口
sessionMgr := wsapp.NewSessionManager(
    wsapp.WithRegisterHook(func(s *wsapp.Session) {
        mu := userPresenceMu.LockFor(s.UserID()); mu.Lock(); defer mu.Unlock()
        ...AddOnline...
    }),
    ...
)
heartbeatScanner := wsapp.NewHeartbeatScanner(sessionMgr, ..., presenceRepo, &userPresenceMu)
```

**2. `server/internal/app/ws/heartbeat_scanner.go`** — 加 `UserPresenceMutex`
   接口 + 字段；reconcile fanout 在 LockFor 后再 IsCurrentForUser：

```go
type UserPresenceMutex interface { LockFor(userID uint64) *sync.Mutex }
type HeartbeatScanner struct { ...; userPresenceMu UserPresenceMutex }

// scanOnce reconcile fanout：
go func(target *Session) {
    ...
    if s.userPresenceMu != nil {
        mu := s.userPresenceMu.LockFor(target.UserID())
        mu.Lock(); defer mu.Unlock()
    }
    if !s.mgr.IsCurrentForUser(ctx, target.SessionID()) { return }  // 持锁后重新校验
    ...renewer.AddOnline(callCtx, ...)
}(sess)
```

**3. shutdown 序列简化**：删 `presenceHooksWG.Wait()` —— hook 已同步，
   sessionMgr.Close 串行调 Unregister 钩子时 RemoveOnline 自然顺序完成。
   normal Redis 下每个 ~10ms × N session = 1秒级，K8s termination grace 内可接受。

**4. 测试**（`internal/app/ws/ws_test.go` 末尾）：
   - `TestHeartbeatScanner_Reconcile_AcquiresSharedUserMutex_BlocksUntilReleased`：
     外部持锁 → scanner reconcile 必须卡住；释放锁 → scanner 才跑 AddOnline
   - `TestHeartbeatScanner_Reconcile_NilUserMutex_DoesNotPanic`：单测兼容
   - `TestHeartbeatScanner_Reconcile_DifferentUsers_DoNotBlockEachOther`：per-user
     锁隔离 —— user A 持锁不阻塞 user B 的 reconcile

**5. 接口签名调整**：`NewHeartbeatScanner` 加第 5 个参数 `UserPresenceMutex`；
   `NewHeartbeatScannerForTestWithRenewer` 保持原 signature 走 nil mutex（既有
   单测兼容），新加 `NewHeartbeatScannerForTestWithMutex` 让单测注入共享 mutex。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计两条独立 goroutine 树共享外部状态（DB / Redis /
> file system）** 时，**禁止**靠"加 mutex + guard + retry"补 fire-and-forget 的
> race，**必须**优先评估"消灭 fire-and-forget，改同步调用 + 共享单一 critical
> section"的结构性修法。
>
> **展开**：
> - mutex 仅保证互斥不保证 FIFO；fire-and-forget goroutine 之间的 dispatch 顺序
>   不等于 lock 抢占顺序。如果业务语义依赖"调用方顺序"（Register 完才 Unregister
>   等），不能靠 goroutine + mutex 兑现，必须**调用方锁内串行**触发。
> - 跨 goroutine 树的"先 check 再 act"模式（IsRegistered → AddOnline / IsExpired →
>   Refresh 等）所有变体都是 TOCTOU race —— check 与 act 之间另一条路径可能改动
>   状态。修法：**先持锁后 check**，让 check 与 act 在同一 critical section。
> - "fire-and-forget 是为了不阻塞 N×Redis 延迟"的 trade-off 通常被高估：normal
>   Redis 下单 op ~10ms，N=100 同步串行也才 1s（K8s grace 内）。**brownout 期**
>   靠 per-call ctx timeout 兜底（presenceHookTimeout=2s × N session 在 fanout
>   架构下也是 2s 上限，不是 N×2s）。
> - 跨包共享锁要走**接口注入**而非具体类型暴露 —— 本案例 ws 包定义
>   `UserPresenceMutex` 接口，main 包的 userKeyedMutex 实装它，避免循环依赖 +
>   单测可注入 stub。
> - **反例**：r6-r8 三轮打补丁 —— 每次都用"再加一层 guard / mutex"修上一轮的
>   race，每次单测都过但生产仍 race；r9 才意识到 fire-and-forget 模式根本不能
>   消除 ordering race，必须结构性翻案。如果发现自己第三次给同一段代码加 guard，
>   stop，重新设计架构。
> - **反例**：用 mutex.Lock() 顺序绑定调用方语义 —— Lock 不是 channel，没有
>   "先到先得"FIFO 保证（Go runtime 自由调度抢占）。靠 mutex 实现"按调用顺序
>   执行"是错的，必须靠**调用方在锁内串行触发**或**channel-based 序列化**。

## Meta: 本次 review 的宏观教训

r1-r9 共 9 轮 review fix 的累积观察：**fire-and-forget 是"不阻塞主路径"与
"消除 ordering race"的零和 trade-off**，二选一。当业务语义依赖跨路径的状态
顺序（lifecycle hook、cache invalidation、event ordering 等）时，主路径必须
**同步等 fire 完成**才能放走下一步；否则要么放弃顺序保证，要么放弃 fire-and-forget。

观察到的反模式（每一 round 都在重复）：
1. 主路径调 fire-and-forget hook X
2. 发现 X 与另一条路径 Y race → 加 mutex
3. 发现 mutex 不保证 FIFO → 加 guard
4. 发现 guard 是 TOCTOU → 加 second guard
5. 发现 shutdown drain 不全 → 加 WaitGroup
6. 发现 shutdown 顺序错 → 加 chan signal
7. ... 循环

每一步都是"加一层补丁"。终点是 r9 的结构性翻案：**先问"为什么我要 fire-and-forget"
而不是"怎么修这一层 race"**。如果同步调用的延迟在 SLO 内（多数情况都在），就
不要 fire-and-forget。
