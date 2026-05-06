---
date: 2026-05-06
source_review: codex review (file: /tmp/epic-loop-review-10-4-r3.md, codex 段)
story: 10-4-心跳框架
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-06 — closeInternal wait 上限不足以 cover writeTimeout & scanner fanout 不响应 ctx 让 SIGTERM emit 4005

## 背景

Story 10.4 r1 / r2 在 `closeInternal` 引入了 "先关 chan + 等 writeLoop 退出
+ 再 WriteControl 写 close frame" 的顺序约束（避免 4005 close frame 后跟 data
frame）。codex review r3 进一步指出**两个引入但没修对**的边界：

1. P2-1：wait writeLoopDone 用的是 `closeFrameWriteDeadline = 500ms`，但 writeLoop
   内 `conn.WriteMessage` 的 deadline 是 `writeTimeout`（生产 5s / 测试 2s），意味着
   writeLoop 在最坏情况下合法卡 5s 才退出。500ms < 5s 让 wait 提前超时，**回归**
   r1 P2 想修的"close frame 与 data frame 顺序错乱" race。
2. P2-2：scanner 的 per-session fanout goroutine 不响应 `ctx`。`scanner.Run` 在
   `ctx.Done` 后主循环退出，但已 dispatch 的 goroutines 仍在跑，仍 emit
   `CloseWithCode(4005)`，与 main.go defer LIFO 钦定的"scanner 先停 → sessionMgr.Close
   走标准 close 路径"流程 race —— 用户 SIGTERM 期间正常下线却收到 4005 触发自动
   重连。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | closeInternal wait 上限 500ms < writeTimeout（5s）导致 close frame 仍可能在 data frame 之前 | medium | concurrency | fix | `server/internal/app/ws/session.go:410-425` |
| 2 | scanner per-session fanout goroutine 不响应 ctx，shutdown 期间仍 emit 4005 | medium | concurrency | fix | `server/internal/app/ws/heartbeat_scanner.go:197-200` |

修了 2 条 / defer 0 条 / wontfix 0 条。

## Lesson 1: closeInternal wait 上限必须 ≥ writeLoop 阻塞最坏时间（writeTimeout + buffer）

- **Severity**: medium (P2)
- **Category**: concurrency
- **分诊**: fix
- **位置**: `server/internal/app/ws/session.go:410-425`

### 症状（Symptom）

`closeInternal` 在 `WriteControl` close frame **之前** wait `writeLoopDone` 上限
取自 `closeFrameWriteDeadline = 500ms`。但 writeLoop 写一帧的 deadline 是
`writeTimeout`（生产默认 5s）—— writeLoop 卡在 `conn.WriteMessage` 时**合法**
阻塞最长 `writeTimeout` 才返回 error 退出。500ms < writeTimeout → wait 提前
超时 → `closeInternal` 误以为 writeLoop 已经退出 → 走 `WriteControl` 写出 close
frame → writeLoop 之后才结束写出 data frame → wire 上 close frame 后跟 data
frame，违反 V1 §12.1 钦定的 "close frame 是 connection 最后一个 frame"。这正
是 r1 P2 想消除的 race，500ms 这个数字让修复**无效**。

### 根因（Root cause）

把 "WriteControl close frame 自己的 deadline" 与 "wait writeLoop 退出的上限"
**合用**了同一个 const。两者语义完全不同：

- WriteControl 是单 packet 操作，500ms 写一帧足够，超时通常意味着对端已掉线
- wait writeLoop 退出**必须** ≥ writeLoop 内 IO 阻塞最坏时间（= writeTimeout），
  否则只能"提前放弃 wait → 顺序错乱"

复用同一 const 看着是简洁，实际是把两个**独立**约束耦合到同一个 magic number。
原 r1 P2 实装时只关注"加 wait 步骤"这件事，没意识到 wait 上限选 500ms 直接让
wait 提前于 writeLoop 真退出 —— 修法没真正消除原 race。

更深层根因：r1 P2 没问 "writeLoop 在最坏情况下需要多久才能退出"。如果当时
回答了这个问题，会立即看到答案是 `writeTimeout`，且必须 ≥ 这个值。

### 修复（Fix）

按 review 推荐方案 (c)：单独引入 `closeWaitTimeout` 字段（per-Session，因为
writeTimeout 是 per-Session 配置），公式 `closeWaitTimeout = writeTimeout +
closeWaitBufferDuration (200ms)`。`closeFrameWriteDeadline` 仍仅用作
`WriteControl` 自己的 deadline（500ms 不变）。

- `session.go`：加 `closeWaitTimeout time.Duration` 字段；`newSession` 算
  `closeWait = writeTimeout + 200ms`（writeTimeout ≤ 0 时 fall back 到
  `closeFrameWriteDeadline + 200ms = 700ms`，与原 500ms 行为相近）；
  `closeInternal` 的 wait 上限改用 `s.closeWaitTimeout`。
- 新 const `closeWaitBufferDuration = 200 * time.Millisecond`：cover writeLoop
  在 WriteMessage 触发 deadline error 后到 break 出 loop + close
  `writeLoopDone` 的纯 in-process 调度延迟，200ms 给 Windows / CI 抖动留充足
  余量。
- 新单测 `TestSession_CloseWaitTimeout_EqualsWriteTimeoutPlusBuffer`（含 4
  个 sub-test）+ `TestSession_CloseWaitTimeout_GreaterThanCloseFrameWriteDeadline_ForProductionWriteTimeout`
  分别覆盖：公式正确 + 生产配置下 closeWaitTimeout 严格大于
  `closeFrameWriteDeadline`（防 r3 修复无效的回归）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：在为 "等待某个 IO-bound goroutine 退出" 实装超时上限时，**必须**
> 让上限 ≥ 该 goroutine 内 IO 操作的 deadline，**严禁**复用其他用途（如发一帧
> control message）的小 deadline 当 wait 上限。
>
> **展开**：
> - 写 wait branch 时先回答："被 wait 的 goroutine 在最坏情况下要多久才能退出？"
>   答案不能 < wait 上限。
> - 如果 goroutine 内有 `SetWriteDeadline / SetReadDeadline / context.WithTimeout`
>   等本地 deadline，wait 上限**至少** = 该 deadline + goroutine 调度 buffer
>   （200ms 是经验值，能 cover Windows + Go runtime 调度抖动）。
> - 反过来：用于"自己 IO 操作"的 deadline（如 WriteControl）可以小（毫秒级），
>   因为它**不**等待别人。两者语义不同**禁止**合用一个 const。
> - **反例**：用 `closeFrameWriteDeadline = 500ms` 同时做 WriteControl deadline
>   + wait writeLoopDone 上限。当 writeLoop 内 WriteMessage 的 writeTimeout=5s
>   时，wait 在 0.5s 提前超时，让本来想消除的"close frame 后 data frame"顺序
>   错乱**回归**。教训：变量复用要看语义，看着精简实际是耦合两个独立约束。
> - **反例**：写 `select { case <-doneCh: case <-time.After(magic) }` 时不算
>   `magic` 是否 ≥ 被 wait 的 goroutine 实际退出时间，硬塞一个看着合理的小
>   数值（如 500ms）。这种代码 review 时容易过 —— 因为"500ms"看着像合理的
>   timeout —— 但语义上是错的。

## Lesson 2: 短期 fanout goroutine 必须显式响应 ctx，否则会跨越 ctx.Done 边界 emit 副作用

- **Severity**: medium (P2)
- **Category**: concurrency
- **分诊**: fix
- **位置**: `server/internal/app/ws/heartbeat_scanner.go:197-200`

### 症状（Symptom）

scanner 的 fanout goroutine（`go func(target *Session) { ... CloseWithCode(4005,
"heartbeat timeout") ... }(sess)`）**完全不**接收 ctx。SIGTERM 触发
`cancelHeartbeat()` 让 `scanner.Run` 主循环退出，但**已 dispatched** 的
per-session goroutines 仍然在跑：它们继续 recheck idleMs → 调用
`CloseWithCode(4005)` 写 close frame。预期的 shutdown 流程是 `sessionMgr.Close()`
走标准 close 路径（无 4005 frame，让 client 走 normal close 而非 transient-error
重连），实际是 race：用户 SIGTERM 期间正常下线，部分连接收到 4005 反而被 iOS
Story 12.5 自动重连风暴误触发。

### 根因（Root cause）

写 fanout goroutine 时只考虑了 happy path（"主循环 dispatch → goroutine 跑完
close → fire-and-forget 完成"），没有思考 "scanner cancel 之后**已经 dispatch
但还没跑完**的 goroutine 怎么办"。

更具体的认知漏洞：把"main 主循环 退出 = scanner 整体停止"这件事想成原子操作，
忽略了 fanout 已经把 closure capture 出去后那些 goroutine 在 OS 调度器手里有
独立生命周期 —— 它们不会因为 dispatcher 退出而自动停止，必须**主动**持有
ctx 才能响应取消。

类比 Go `http.Server.Shutdown` 在等 in-flight handlers 完成时也是依赖 handler
内部 select ctx.Done —— 没有内部 ctx 检查，外层 cancel 信号传不下去。fanout
goroutine 同理。

### 修复（Fix）

按 review 推荐做法，让 fanout goroutine 捕获 ctx 并在两个关键点 check `ctx.Done`：

```go
go func(target *Session) {
    // 入口 check：已 cancel → 直接 return，不进入 close path
    select {
    case <-ctx.Done():
        return
    default:
    }
    // ... TOCTOU recheck idleMs ...
    if idleMs <= timeoutMs {
        return
    }
    // 二次 check：recheck 与 CloseWithCode 之间还有调度间隙
    select {
    case <-ctx.Done():
        return
    default:
    }
    // ... CloseWithCode(4005, ...)
}(sess)
```

- 入口 check 是主防线（cover "scanOnce 整个跑完了，goroutine 还在排队"的常见
  case，scanner cancel 后立即生效）。
- 二次 check 让 race window 尽可能小（recheck idle 与 close 之间只有几纳秒，
  无法完美闭合，但实战中能 cover 99% 调度延迟场景）。

新单测 `TestHeartbeatScanner_ScanOnce_FanoutGoroutineRespectsCtxCancel` 用已
cancel 的 ctx 调 `scanOnce`，断言 session 不被 close（Send 仍能成功）—— 入口
ctx check 是主防线，能稳定通过。

### 预防规则（Rule for future Claude）⚡

> **一句话**：**任何**生命周期可能跨越 cancel 边界的短期 goroutine 必须
> 接收并 check 至少一次 ctx.Done —— "fire-and-forget" **不**等于"不响应取消"。
>
> **展开**：
> - 写 `go func() { ... }()` 时立即问："如果外层 ctx 被 cancel 了，这个
>   goroutine 应该停止吗？" 答案 99% 是 yes（除非是非常短的 in-process 计算）。
> - 如果有 IO / 副作用（写 socket、调外部服务、改持久状态），**必须**入口 +
>   关键操作前都 check ctx.Done。
> - 启动多个 fanout goroutine 时，**所有** goroutine 都要捕获 ctx —— 不能图省事
>   只让 dispatcher 检查 ctx。dispatcher 退出**不**自动停止已 dispatch 的子
>   goroutine。
> - **反例**：scanner.Run 主循环 select ctx.Done → return；dispatch 的 fanout
>   goroutine 从不 check ctx。SIGTERM 后主循环立刻退，子 goroutine 仍跑完
>   `CloseWithCode(4005)`，与 sessionMgr.Close 标准路径 race，让正常 shutdown
>   误触发 client 重连风暴。
> - **反例**：把 "fire-and-forget" 等同 "不需要管 cancel"。fire-and-forget 只是
>   "调用方不等结果"，**不**意味着 goroutine 自己可以无视取消信号。
> - **检查清单**：写完 fanout goroutine 后用这条 grep 自查：
>   `grep -A2 'go func' | grep -v 'select.*ctx.Done\|<-ctx.Done\|ctx.Err'`
>   匹配到的就是潜在风险点（要么是合理的纯 CPU 短任务，要么是漏写 ctx check）。

## Meta: 本次 review 的宏观教训

两条 finding 都是 r1/r2 修复时**没真正闭合**留下的余烬，共同病灶是 r1/r2 在写
"加 wait/select 步骤"时没补足边界推理：

- r1/r2 加 wait writeLoopDone 时没问 "wait 上限够不够 cover 被 wait 的最坏退出
  时间？"
- r1/r2 加 fanout goroutine 时没问 "scanner cancel 后这些 goroutine 怎么停？"

教训：每次给并发代码加 "等待 / 取消" 路径，**必须**用清单式检查至少回答这两
类问题：

1. **wait 类路径**：被 wait 的对象（goroutine / IO / channel close）在最坏情
   况下需要多久退出？wait 上限是否 ≥ 该值 + buffer？
2. **取消类路径**：cancel 信号能否传达到所有相关 goroutine？已 dispatched 的
   后台任务有没有捕获 ctx？

加 select / wait / cancel **不**等于"修好了"——修好的 def 是"被修的不变量在所有
相关边界条件下都成立"。每次 fix-review 完成后，逼自己写一句"这条修复在 X 边界
条件下是否成立？"作为最后 self-check。
