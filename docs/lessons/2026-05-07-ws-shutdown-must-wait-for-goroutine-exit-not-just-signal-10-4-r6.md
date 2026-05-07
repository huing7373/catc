---
date: 2026-05-07
source_review: file:/tmp/epic-loop-review-10-4-r6.md
story: 10-4-心跳框架
commit: d4cfc90
lesson_count: 1
---

# Review Lessons — 2026-05-07 — shutdown 必须 wait goroutine 退出而不是只 signal cancel（10-4 r6）

## 背景

Story 10.4 心跳框架 review 第 6 轮（codex）。r5 给 `HeartbeatScanner.Run` 的 defer 加了
`wg.Wait()`，让 Run 在所有 in-flight fanout goroutine 跑完才返回 —— 这样 `Run` 返回 = scanner
完全静默。但 main.go 的 shutdown 路径只 `cancelHeartbeat()` 然后让另一个 deferred 函数跑
`sessionMgr.Close()`，**没等 `Run` 真正 return**。cancel 与 Run 实际 return 之间存在窗口（覆盖
fanout drain），窗口内 fanout goroutine 仍可调 `CloseWithCode(4005,...)`，与 `sessionMgr.Close`
标准 close 路径并发 race，导致 idle client 收到 4005 而非正常 shutdown close —— 恰好是 r4
想消灭的"误重连"。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | main shutdown 不等 scanner.Run 退出就调 sessionMgr.Close | high | architecture / shutdown-ordering | fix | `server/cmd/server/main.go:235-241` |

## Lesson 1: shutdown 必须 wait goroutine 真正退出而不是只 signal cancel

- **Severity**: high
- **Category**: architecture / shutdown-ordering / concurrency
- **分诊**: fix
- **位置**: `server/cmd/server/main.go:235-241`

### 症状（Symptom）

main.go shutdown 顺序原本依赖 `defer LIFO`：

```go
defer sessionMgr.Close()        // 先注册 → 后执行
// ...
defer cancelHeartbeat()         // 后注册 → 先执行
go scanner.Run(heartbeatCtx)
```

LIFO 让 cancelHeartbeat **先**于 sessionMgr.Close 执行。但这只保证两个 deferred 函数的
**注册顺序**对应的 **调用顺序**，**不**保证 cancelHeartbeat 调用之后 scanner 实际已退出。
r5 给 `Run` 的 defer 加了 `wg.Wait()`：Run 必须等所有 in-flight fanout goroutine 跑完才返回。
所以 cancelHeartbeat() 返回的瞬间 scanner.Run **仍**在跑（drain fanout 中），fanout 内部
"已通过 ctx-check 即将调 CloseWithCode" 的 goroutine 会照常 emit 4005，与 sessionMgr.Close
并发 race。

### 根因（Root cause）

把 `defer LIFO` 当成 happens-before 同步原语用了。LIFO 保证的是 deferred 函数**调用时序**，
**不**保证某个 deferred 函数返回时它 signal 的 goroutine 已退出。**signal cancel ≠ wait done**。
goroutine 取消的标准 idiom 是**两步**：

1. cancel(ctx)：通知
2. <-done：阻塞等 goroutine 真正退出（含 cleanup）

只做第一步等于"按了关机键就走"，目标进程的 cleanup 还在跑。原 r4 / r5 修复都集中在 scanner
内部（fanout 入口 ctx-check + Run defer wg.Wait），让 scanner 自己 drain 干净。但 main 这一侧
**没接住**这个不变量 —— 没等 Run 真正 return 就让 sessionMgr.Close 接管，drain 期间的 fanout
仍能跑，race 仍在。

更深层的思维漏洞：**多个 deferred 函数注册分散** → 顺序难看清 → 容易把 LIFO 当时序保证。
正确做法：**关键 ordering 收成一个 deferred 函数**，串行写出，依赖关系显式。

### 修复（Fix）

把 cancelHeartbeat → wait scannerDone → sessionMgr.Close 收成**一个 deferred 函数**，串行执行：

```go
heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)
heartbeatScanner := wsapp.NewHeartbeatScanner(sessionMgr, cfg.WS.HeartbeatTimeoutSec, slog.Default())
scannerDone := make(chan struct{})
go func() {
    defer close(scannerDone)
    heartbeatScanner.Run(heartbeatCtx)
}()
defer func() {
    // 1. signal scanner 开始退出
    cancelHeartbeat()
    // 2. 等 Run 真正返回（含 wg.Wait drain 所有 in-flight fanout）
    <-scannerDone
    // 3. 现在批量清 Session，没有 4005 race
    if cerr := sessionMgr.Close(); cerr != nil {
        slog.Error("session manager close failed", slog.Any("error", cerr))
    }
}()
```

加了一个 regression 测试 `TestHeartbeatScanner_ShutdownOrdering_NoFourThousandFiveAfterScannerExit`：
模拟 main shutdown helper 的 ordering 不变量 —— scanner.Run 必须先 return 才能调 mgr.Close，
用 chan timing 验证 closeStarted 时 scannerDone 已 close。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在为长生命周期 goroutine 编写 shutdown 路径时，**必须**用 `done chan`
> 显式 wait goroutine 真正退出再调下一阶段 cleanup，**不**依赖 `defer LIFO` 间接对齐 ordering。
>
> **展开**：
> - **signal ≠ wait**：`cancel(ctx)` 只是通知 goroutine 应该退出，goroutine 内部可能还在跑
>   defer cleanup（如 `wg.Wait()` drain children、close client、flush log）。后续 cleanup 步骤
>   要等 goroutine 真正退出，**必须** `<-done`，不能假设 cancel 返回 = goroutine 退出。
> - **Cleanup ordering 收成一个 deferred 函数**：当 shutdown 涉及多步串行依赖（A 必须先于 B
>   完成 → C 才能开始），把这些步骤写在**一个**deferred 函数里，串行排列。多个 deferred 函数
>   靠 LIFO 拼出来的"顺序"只保证**调用时机**，不保证**完成时机**。
> - **graceful-shutdown window 是脆弱的**：从"signal 发出"到"goroutine 真正退出"的窗口期
>   可能毫秒到几秒不等（取决于 in-flight 工作量）。这段时间任何并发资源访问都是潜在 race。
>   设计时假设这个窗口非零。
> - **反例**：
>   ```go
>   // BAD：依赖 LIFO 拼 ordering，且不等 goroutine 真正退出
>   defer mgr.Close()              // 想让它最后跑
>   defer cancel()                  // 想让它先跑
>   go worker.Run(ctx)              // worker 内部 defer wg.Wait drain children
>   // cancel() 返回时 worker 仍在 drain → mgr.Close 与 worker 并发跑
>   ```
>   ```go
>   // GOOD：done chan + 单 deferred 串行
>   done := make(chan struct{})
>   go func() { defer close(done); worker.Run(ctx) }()
>   defer func() {
>       cancel()
>       <-done           // 等 worker 真正退出
>       _ = mgr.Close()  // 现在独占
>   }()
>   ```
> - **测试角度**：写 regression 测试时不要只断言 cancel 调用了 / done chan 用上了，而是断言
>   **ordering**：goroutine 完成 < 下一阶段开始。chan timing（select default 探测、time.After
>   兜底）是验证这种"先后关系"的标准手段。
