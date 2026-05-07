---
date: 2026-05-06
source_review: codex review of Story 10.4 round 4 (file: /tmp/epic-loop-review-10-4-r4.md)
story: 10-4-心跳框架
commit: b23aff3
lesson_count: 2
---

# Review Lessons — 2026-05-06 — heartbeat scanner ctx 必须挂主 signal ctx & closeInternal wait 仅限 emitClose 路径（10-4 r4）

## 背景

Story 10.4（心跳框架）r4 review 抓出两个 P1 回归：

1. r3 修了 fanout goroutine 不响应 ctx 的问题，但 main.go 把 `heartbeatCtx` 用 `context.WithCancel(context.Background())` 起，与 main 的 SIGTERM signal ctx 解耦 —— SIGTERM 触发后 scanner 主循环仍在跑直到 main 返回执行 deferred cancelHeartbeat()。这条 graceful-shutdown window 内，scanner 仍可能 emit 4005，让正常 shutdown 的 client 错误地走自动重连路径。
2. r3 引入的 `wait writeLoopDone` 上限 = `writeTimeout + 200ms`（≈5.2s）原意是兜底 CloseWithCode 路径的 close frame ordering，但实装路径**无视 emitClose 标志**对所有 close 都 wait。plain `Close()` 没有任何 close frame 要写，wait 是纯 overhead。SessionManager.Register 替换路径 / Close 批量收尾都被这条 wait 拖到 5s+。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | heartbeat scanner ctx 必须从 main signal ctx 派生而非 Background() | high | architecture | fix | `server/cmd/server/main.go:225-227` |
| 2 | closeInternal 仅在 emitClose=true 时 wait writeLoopDone | high | perf | fix | `server/internal/app/ws/session.go:462-476` |

修了 2 条 / defer 0 条 / wontfix 0 条。

## Lesson 1: heartbeat scanner ctx 必须从 main signal ctx 派生

- **Severity**: high
- **Category**: architecture（context propagation / shutdown ordering）
- **分诊**: fix
- **位置**: `server/cmd/server/main.go:225`

### 症状

SIGTERM 到达后，main.go 已经 cancel(ctx) 让 bootstrap.Run 退出，但 heartbeat scanner 用独立的 `context.Background()` 派生 ctx —— scanner 主循环仍在跑直到 main 返回执行 deferred cancelHeartbeat。这条 graceful-shutdown window 内（典型几百 ms 到几秒，由 server.Shutdown 的 in-flight request drain 决定），任何 idle ws session 仍可能被 scanner 用 4005 "heartbeat timeout" 关闭，触发 client 的指数退避自动重连 —— 与 sessionMgr.Close 钦定的"标准下线（无 4005）"路径直接冲突。

### 根因

修 r3 时只关注了"fanout goroutine 入口要 check ctx"，没追溯到这条 ctx 实际来自哪里。代码注释写了"`go scanner.Run(ctx)`"看起来 ctx 已经传进去了，但没注意到 `heartbeatCtx, cancelHeartbeat := context.WithCancel(context.Background())` —— 这个 ctx **不是**从 main 的 signal ctx 派生的，而是独立的一棵子树。SIGTERM cancel main ctx 不会影响这棵 heartbeat 子树；只有 main 函数 return 后 deferred cancelHeartbeat() 才会触发，但那时已经走过整个 bootstrap.Run 收尾流程。

写 main.go 时容易陷入"每个长生命周期 goroutine 起一棵独立 ctx 树"的反直觉模式 —— 看似职责清晰（"scanner 自己管自己"），实际是在 shutdown 协调上自挖一个洞。

### 修复

```go
// before
heartbeatCtx, cancelHeartbeat := context.WithCancel(context.Background())
// after
heartbeatCtx, cancelHeartbeat := context.WithCancel(ctx)  // ctx 是 signal.NotifyContext
```

派生自 ctx 后 SIGTERM 立即 cancel scanner.Run + 已 dispatched fanout goroutines（fanout 内部本就有 ctx.Done() 入口 check / recheck，r3 P2 引入）。defer cancelHeartbeat() 仍然保留作为兜底（main return 时再 cancel 一次幂等）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 main.go 起任何 **后台 goroutine（scanner / worker / event loop / cleanup tick）** 时，**必须**让其 ctx **派生自 main 的 signal.NotifyContext ctx**，**禁止**用 `context.Background()` 起独立子树（除非该 goroutine 必须在 main 退出后继续运行 —— 那种场景在本项目中**不存在**）。
>
> **展开**：
> - 模式：`fooCtx, cancelFoo := context.WithCancel(ctx)` + `defer cancelFoo()` —— 两条防线共存，前者保证 SIGTERM 立即生效，后者保证 main 自然 return 时也清理
> - 检测信号：grep `WithCancel(context.Background())` / `WithTimeout(context.Background())` 在 cmd/server/main.go 中应**仅**用于"必须独立于 SIGTERM 的启动期 IO"（如 db.Open / redis.Open 启动期 ping —— 这些必须独立短 timeout 而不是绑死 SIGTERM 让 5s 启动 ping 被无限延后）
> - **反例**：起 ws scanner / metrics ticker / health probe 等长生命周期 goroutine 时用 `context.Background()` 派生独立 ctx 树 —— 看似"职责自管"，实际让 graceful shutdown 失去对该 goroutine 的控制权，shutdown window 内任何 side-effect（emit 4005 / 写 metrics / 触发 cleanup）都成为 race
> - 反向：启动期短 IO（db.Open / redis.Open / config.Load）**应该**用独立 timeout ctx，因为它们的语义是"5s 内必须出结果"，不是"跟主 ctx 同生命周期"

## Lesson 2: closeInternal wait writeLoopDone 仅限 emitClose=true 路径

- **Severity**: high
- **Category**: perf（regression on hot path）
- **分诊**: fix
- **位置**: `server/internal/app/ws/session.go:462-476`

### 症状

r3 加了 `wait writeLoopDone` 直到 `closeWaitTimeout = writeTimeout + 200ms`（≈5.2s）。原意是修 CloseWithCode 路径的 close frame ordering（writeLoop 卡在 conn.WriteMessage 时，wait 必须 ≥ writeTimeout 才能保证 writeLoop 真正退出后 WriteControl 写 close frame）。但实装路径无视 emitClose 标志，**所有** close 都 wait。

具体 production 路径退化：
- `SessionManager.Register` 替换路径调 `replaced.Close()` → 现在卡到 5.2s 才返回 → 同一 user 重连整体延迟 5s+，client 体感"重连后连接数秒不可用"
- `SessionManager.Close` 循环 `s.Close()` → shutdown 慢 5s+（serial 5.2s × N）
- 任何对 plain `Close()` 的调用都被这条 wait 拖累，包括 readLoop/writeLoop defer 内的幂等收尾

### 根因

修 r1/r3 时聚焦在"close frame ordering 协议正确性"，把 wait 当成 closeInternal 的固定步骤。没区分 emitClose 的两种路径在 frame ordering 上的本质差异：

- `CloseWithCode`（emitClose=true）：要写 close frame，**需要**保证它是 wire 上最后一帧 → wait writeLoop 退出，确保没有任何 data frame 跟在 close frame 后
- `Close`（emitClose=false）：**不写**任何 frame，纯关 sendChan + cancel ctx + close conn → 没有任何 ordering 约束 → wait writeLoop 是纯 overhead

把 wait 不分路径地放进 closeOnce.Do 的副作用里，违反了"最小特殊性"原则 —— wait 的唯一存在理由是为了 emitClose 路径，应该跟 emitClose 同 gate。

### 修复

```go
// before
if s.writeLoopStarted.Load() && s.writeLoopDone != nil {
    select {
    case <-s.writeLoopDone:
    case <-time.After(waitTimeout):
    }
}
// after
if emitClose && s.writeLoopStarted.Load() && s.writeLoopDone != nil {
    select {
    case <-s.writeLoopDone:
    case <-time.After(waitTimeout):
    }
}
```

加 `emitClose &&` gate 后：
- plain Close → 跳过 wait → sendChan 已关 → writeLoop 下次 select 命中关闭分支自然退出（独立于本调用，不阻塞 caller）
- CloseWithCode → 仍 wait → 保留 r1/r3 修复的协议正确性

新增三个回归测试覆盖（`session_close_internal_test.go`）：

1. `TestSession_CloseWithCode_AfterWriteLoopStarted_StillWaits` —— CloseWithCode 路径 wait 必须仍然生效
2. `TestSession_Close_AfterWriteLoopStarted_DoesNotWait` —— plain Close 在 writeLoop 启动后必须 < 50ms 返回
3. `TestSession_Close_FastEvenWhenWriteLoopBlockedOnWrite` —— plain Close 在 writeLoop 阻塞场景应 < 100ms（远小于 writeTimeout=2s 的旧上限）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在统一两个语义不同的 close 路径到一个 internal helper（`closeInternal(emitClose, code, reason)` 模式）时，**任何**与 `emitClose` 语义绑定的步骤（写 close frame / wait writeLoop 退出 / log 协议错误）**必须**在该 step 入口加 `if emitClose` gate，**禁止**把它们当成"两条路径共用的固定副作用"。
>
> **展开**：
> - close frame ordering 是 emitClose 路径独有的协议约束，与 plain Close 完全无关；任何为它服务的同步原语（wait / barrier / lock-step）都必须跟 emitClose 同 gate
> - 写 internal helper 时先列两条路径在每个 step 的具体需求，**任何 step 在两条路径上需求不同**就必须 gate；step 在两条路径都需要 = 公共代码
> - **反例 1**：把 CloseWithCode 路径的 wait writeLoop 当成 closeInternal 通用 cleanup 步骤；后果：plain Close 也付 writeTimeout+200ms tax，hot path 退化
> - **反例 2**：在 emitClose=false 路径写 log "close frame written"；后果：日志谎报，影响排障
> - 检测信号：closeInternal-style helper 内部如果有 step 引用 `emitClose / code / reason` 之外的副作用，但**未**包在 `if emitClose` 内，几乎一定是 bug
> - 性能锚定：这种 hot-path latency regression 通常体现为"重连慢 5s / 关服慢 5s"这类用户/运维感知层面的体感问题，单元测试很容易漏（happy path 仍 OK），需要专门加 latency-bound assertion

---

## Meta: 本次 review 的宏观教训

两条都是"修 A 时引入新问题 B，修 B 时引入新问题 C"的 r1→r2→r3→r4 退化链典型样本。从 r1（close frame ordering）到 r4（plain Close 不该 wait）总共 4 轮才稳定下来。元教训：

**改动 close path / shutdown coordination 这种"细节多、路径互相纠缠"的代码时，Claude 倾向于"先满足新需求，再考虑旧路径是否退化"**。每一轮 r 都是部分修复 —— 修了一个语义维度（ordering / handshake-fast-path / wait-budget / shutdown-tie / emitClose-gate）但漏了相邻维度。

预防规则：

> 未来 Claude 在改 close path / lifecycle ctx 派生 / shutdown ordering 这类"多路径多语义维度纠缠"的代码时，**必须**先列出**所有调用路径** + **每条路径在每个语义维度上的需求**（ordering 需要否 / wait 需要否 / log 级别 / channel ownership / ctx 派生源），**禁止**只看新需求那条路径就改公共代码。
>
> 具体到本次：closeInternal 有 emitClose=true / false 两条路径；wait writeLoop 是 emitClose 路径独有；ctx 派生应当跟主 signal ctx 同生命周期。任一维度被忽略都会引入下一轮 r。
