---
date: 2026-05-06
source_review: codex review (file: /tmp/epic-loop-review-10-4-r2.md, codex 段)
story: 10-4-心跳框架
commit: 50f7cb5
lesson_count: 1
---

# Review Lessons — 2026-05-06 — closeInternal 必须 gate writeLoopDone wait 在 writeLoopStarted（10-4 r2）

## 背景

Story 10.4 r1 在 `Session.closeInternal` 加了"等 writeLoop 退出再 WriteControl
close frame"的顺序保证（500ms timeout 兜底），解决 close frame 与 data frame
并发写到 wire 的协议错误。codex r2 命中其副作用：r1 在所有 close 路径都无条件
等 `writeLoopDone`，但 Gateway 的 handshake 失败路径会在启动 readLoop/writeLoop
**之前**调 session.Close → writeLoop 从未运行 → writeLoopDone 永远不 close →
每次都等到 500ms timeout 才返回。结果：每个失败 handshake（ListMembers /
buildSnapshot / snapshot WriteMessage / Register 失败）的 socket 释放从亚毫秒
拖到亚秒。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | closeInternal 在 writeLoop 未启动时仍 wait writeLoopDone，handshake 失败路径每次付 500ms timeout tax | medium (P2) | perf / concurrency | fix | `server/internal/app/ws/session.go:393-400` |

## Lesson 1: 等待 sub-goroutine 退出信号必须 gate 在"已启动"flag

- **Severity**: medium（P2 perf regression，handshake 失败路径每次 +500ms）
- **Category**: perf / concurrency
- **分诊**: fix
- **位置**: `server/internal/app/ws/session.go:393-400`（closeInternal 等
  writeLoopDone 段），`session.go:581`（writeLoop 入口处置位 writeLoopStarted）

### 症状（Symptom）

`Session.closeInternal` 在 closeOnce.Do 内：

```go
// r1 实装（有 bug）
if s.writeLoopDone != nil {
    select {
    case <-s.writeLoopDone:
    case <-time.After(closeFrameWriteDeadline): // 500ms
        s.logger.Warn("ws closeInternal writeLoop wait timeout")
    }
}
```

`writeLoopDone` channel 在 newSession 构造时分配（非 nil），但只有 writeLoop
goroutine 真正运行到 defer 才会 `close(writeLoopDone)`。Gateway.Handle 的握手
失败路径（行 268 / 280 / 294 / 310）调 `session.Close()` 时 readLoop/writeLoop
**还没启动**（启动在行 321/322，且 handshake 错误后直接 return） → wait 必然
落到 500ms timeout 分支。

每次握手失败后 socket / Session cleanup 的延迟从亚毫秒变成 ~500ms；
ListMembers / Register 等失败在压力场景下会显著拉高 connection turnover 时延。

### 根因（Root cause）

r1 加 wait 时把 `writeLoopDone != nil` 当作"writeLoop 启动过"的信号 —— 这是
**误判**：channel 在构造时就分配出来，"non-nil" 仅表示"channel 存在"，不表示
"会有人 close 它"。真正的"writeLoop 已运行" semaphore 是 writeLoop goroutine
本身，必须由它自己置位 flag 让 closeInternal 知道。

抽象一层：等一个 sub-goroutine 退出信号（channel close）的必要前提是该
goroutine **真的会跑到 close 它的那一行**。如果该 goroutine 可能根本没启动，
wait 就会变成"等一个永远不会发生的事件"，必须 fallback 到 timeout —— 这
fallback 不是兜底，是**主路径**。

类似坑（防御性兜底变主路径）在并发代码中普遍：
- `sync.WaitGroup.Wait()` 在 `wg.Add(N)` 之前调 → wait 立即返（不是 wait N
  goroutine）；如果该 goroutine 还没启动就 Wait，等的是空集合
- `context.Done()` 等一个永远不会被 cancel 的 ctx → 永远阻塞
- `chan struct{}` 等一个永远不会被 close 的 channel → 永远阻塞，timeout 兜底

修复模式都一样：用 atomic flag / counter 显式标记"sub task 已启动"，wait 路径
gate 在 flag=true 才进入。

### 修复（Fix）

加 `Session.writeLoopStarted atomic.Bool`，writeLoop 入口立即 Store(true)，
closeInternal 的 wait 分支前先 check flag：

```go
// session.go Session struct 新增字段
writeLoopStarted atomic.Bool

// session.go writeLoop 入口（defer 之前置位）
func (s *Session) writeLoop() {
    s.writeLoopStarted.Store(true)
    defer func() {
        close(s.writeLoopDone)
        _ = s.Close()
    }()
    // ... 主循环
}

// session.go closeInternal 的 wait 段
if s.writeLoopStarted.Load() && s.writeLoopDone != nil {
    select {
    case <-s.writeLoopDone:
    case <-time.After(closeFrameWriteDeadline):
        s.logger.Warn("ws closeInternal writeLoop wait timeout")
    }
}
// 否则跳过 wait —— writeLoop 没启动过，没必要等
```

为什么 Store 放 defer 之前：哪怕 writeLoop 立即退出（极端 fixture），
writeLoopStarted=true → wait 分支进入 → 立刻命中 close(writeLoopDone) →
亚毫秒返回（与 r1 happy path 行为一致）。Store 在 defer 之后则有窗口：
goroutine 已被调度但 Store 未执行，恰逢 closeInternal 检查 → 跳过 wait →
writeLoop 写完最后一帧后才 close writeLoopDone，但此时 closeInternal 已经
WriteControl close frame，data frame 跟在 close frame 之后写到 wire → r1 P2
试图防的协议错误重新出现。

回归测试（`server/internal/app/ws/session_close_internal_test.go`）：

- `TestSession_Close_FastWhenWriteLoopNotStarted` —— newSession 后**不**启动
  writeLoop，调 Close，断言 < 50ms（远小于 500ms timeout）
- `TestSession_CloseWithCode_FastWhenWriteLoopNotStarted` —— 同上但走
  CloseWithCode 路径
- `TestSession_Close_AfterWriteLoopStarted_StillWaits` —— 启动 writeLoop 后
  Close，断言返回时 writeLoopDone 已 close（r1 P2 wait 语义不 regress）

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **加"等 sub-goroutine 退出"逻辑** 时，**必须**
> **用 `atomic.Bool` / counter 显式标记 sub-goroutine 是否已启动，wait 路径
> gate 在该 flag**。
>
> **展开**：
> - "channel 已分配（non-nil）" ≠ "会被 close"。channel close 由具体
>   goroutine 的 defer / 主流程触发，goroutine 没启动 → channel 永远不 close
> - 等 channel close 的所有路径都必须考虑"该 channel **可能永远不被 close**"
>   的场景；timeout 是兜底，不是主路径
> - sub-goroutine 入口处的 `started.Store(true)` 必须放在**任何可能 panic /
>   立即 return 的逻辑之前**，确保 caller 看到 started=true 后 channel 一定
>   会被 close（哪怕 sub-goroutine 立即退出）
> - **反例**（r1 实装）：`if s.writeLoopDone != nil { select {...} }` ——
>   把"channel 存在"当"goroutine 启动"的代理，handshake 失败路径每次付 500ms
>   timeout
> - **反例**：用 `sync.WaitGroup.Wait()` 而 caller 没保证 `wg.Add(N)` 在
>   `wg.Wait()` 之前完成 —— Wait 立即返（错以为"等了 N 个"）
> - **反例**：用 `<-ctx.Done()` 等一个**没有 cancel 路径**的 ctx —— 永远阻塞，
>   timeout 兜底变主路径

## Meta: 本次 review 的宏观教训

每次给 close / cleanup 路径加"等 X 完成"的同步语义时，必须穷举调用入口里
"X **可能根本没发生过**"的场景，至少包括：
1. 资源构造完成但 worker goroutine 还没启动（本 case）
2. 资源构造时 worker 启动失败（panic / err return）
3. 资源在不同生命周期阶段被 close（Init phase / Run phase / Shutdown phase）

加 wait 时同步加一组场景测试，断言每个场景都 < timeout（远小于 timeout 兜底
值），是验证"timeout 兜底确实只是兜底，不是主路径"的最直接手段。
