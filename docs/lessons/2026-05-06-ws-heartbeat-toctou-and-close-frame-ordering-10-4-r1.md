---
date: 2026-05-06
source_review: codex review (file: /tmp/epic-loop-review-10-4-r1.md, codex 段)
story: 10-4-心跳框架
commit: 0b68956
lesson_count: 2
---

# Review Lessons — 2026-05-06 — 心跳超时扫描的 TOCTOU race 与 4005 close frame 顺序保证

## 背景

Story 10.4 在 server WS 网关上加心跳超时扫描器（`HeartbeatScanner`）+
`Session.CloseWithCode(4005, "heartbeat timeout")`。codex review r1 命中两个
**新引入**的并发 race，都跨 goroutine、都在阈值边界场景下让线上行为偏离协议
契约：

1. P1：scanner 在主循环判定 idle，把 close 调用 dispatch 到 fanout
   goroutine 之间，readLoop 仍可能刷新 lastHeartbeatAt（client 的 ping 刚到）
   → 健康连接被误踢。
2. P2：`CloseWithCode` 先 WriteControl close frame 再调 Close()；中间窗口
   writeLoop 仍在跑，可能继续把 sendChan / sendPriorityChan 里的业务 msg /
   pong 写到 wire → 客户端收到 "close frame → data frame" 的协议错误（V1 §12.1
   钦定 close frame 必须是 connection 上最后一个 frame）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | scanOnce TOCTOU：goroutine 内未重新校验 idleMs，client 边界 ping 刷新后健康连接被误踢 | high | concurrency | fix | `server/internal/app/ws/heartbeat_scanner.go:183-201` |
| 2 | CloseWithCode 顺序倒：WriteControl 在 close channel 之前，writeLoop 在 close frame 之后仍可能写 data frame | high | concurrency | fix | `server/internal/app/ws/session.go:363-380` |

## Lesson 1: scanOnce 主循环读 + fanout goroutine close 之间的 TOCTOU

- **Severity**: high
- **Category**: concurrency
- **分诊**: fix
- **位置**: `server/internal/app/ws/heartbeat_scanner.go:183-201`

### 症状（Symptom）

`scanOnce` 主循环：读一次 `LastHeartbeatAt()` → 算 idleMs → 若 `idleMs > timeoutMs`
就 `go func() { CloseWithCode(4005, ...) }()`。从主循环读完 timestamp 到 fanout
goroutine 实际 dispatch close 之间存在一段非零时间窗口（go routine scheduler
延迟、单 scanOnce 迭代多个 session 的串行耗时）；这段窗口里 readLoop 仍可能
收到 client 的 ping 并刷新 lastHeartbeatAt。结果：fanout goroutine 拿着**过期**
判定结果触发 close，把刚刚活跃的连接误判 4005 → 触发 client 不必要的 reconnect。

阈值边界（≈ 60s）才是问题区：客户端正好在 59-60s 这段间隔发 ping，server 正好
在 60s tick 扫描 → 主循环把它判 idle → fanout goroutine 把它 close。

### 根因（Root cause）

把 "判定" 与 "执行 close" 跨 goroutine 切分，但**没**在执行端重新校验前置条件 ——
经典 TOCTOU（time-of-check vs time-of-use）模式。fanout 的目的是消化 close frame
500ms write deadline 的串行延迟（1000 session 串行写要 500s），但 fanout 一旦
跨 goroutine boundary，主循环读到的 idleMs 就只是"判定瞬间的快照"，对 close
执行端不是当前事实。

### 修复（Fix）

`scanOnce` 主循环判定后仍 spawn goroutine（保留 fanout 的 SLO 收益），但
goroutine 内**先重新读 `LastHeartbeatAt` 再算 idle**，只有"重新校验"仍超阈才
真正调 `CloseWithCode`。

```go
go func(target *Session) {
    // review r1 P1 TOCTOU 防护：在执行 CloseWithCode 之前**重新**读
    // LastHeartbeatAt。主循环读时刻与本 goroutine 实际执行 close 之间，
    // readLoop 仍可能收到 client 心跳并刷新 lastHeartbeatAt。
    recheckNowMs := time.Now().UnixMilli()
    idleMs := recheckNowMs - target.LastHeartbeatAt()
    if idleMs <= timeoutMs {
        return // race 窗口期内 client 已刷新心跳；本次 fanout 跳过
    }
    // ... log + CloseWithCode
}(sess)
```

加测试：用 `SetLastHeartbeatAtForTest`（新增 export_test.go helper）模拟 race —
先把 lastHeartbeatAt 拖到过期，跑 scanOnce 让主循环 spawn fanout，**立即**把
lastHeartbeatAt 刷新回 now（模拟 readLoop 收到 ping），等 300ms 后断言 session
仍 active。

### 预防规则（Rule for future Claude）⚡

> **一句话**：跨 goroutine 把"判定"和"执行"切分时，**必须**在执行端重新校验
> 前置条件 —— 主循环的判定结果只是判定瞬间的快照，不能直接传给执行端使用。
>
> **展开**：
> - **典型场景**：scanner 模式（list → 过滤判定 → fanout 执行）、connection
>   pool eviction、缓存 LRU 淘汰、超时 reaper。所有"在 main goroutine 判，在
>   worker goroutine 杀"的设计都有 TOCTOU 风险。
> - **规则**：worker goroutine 收到 target 后，**第一步**永远是用最新观测值
>   重新校验"是否还该执行"，不要相信调度方传来的派生量（idleMs、stale flag、
>   priority score）。
> - **测试 anti-fragility**：单测里专门构造"判定-执行间隙"的 race（用 export
>   helper 直接操作内部状态），断言"间隙内变化时不执行"。
> - **反例**：本 review 的 r0 实装 `go func(target, idleMs int64) { ... }` 把
>   判定时刻的 idleMs 当 ground truth 传给 worker；worker 不再读 lastHeartbeatAt
>   直接 close → 边界场景误踢健康连接。
> - **不要走的歪路**：把 fanout 改成串行（"消除 race"）—— 牺牲 SLO 换不必要的
>   简化。fanout + 重新校验是正确范式。

## Lesson 2: WS CloseWithCode 必须先关 send channel + 等 writeLoop 退出，再 WriteControl close frame

- **Severity**: high
- **Category**: concurrency
- **分诊**: fix
- **位置**: `server/internal/app/ws/session.go:363-380`

### 症状（Symptom）

原版 `CloseWithCode`：

```go
func (s *Session) CloseWithCode(code int, reason string) error {
    if s.closed.Load() { return ErrSessionClosed }
    deadline := time.Now().Add(closeFrameWriteDeadline)
    s.conn.WriteControl(websocket.CloseMessage,
        websocket.FormatCloseMessage(code, reason), deadline)  // 写 close frame
    return s.Close()  // 之后才关 channels
}
```

WriteControl 完成到 Close() 内 close(sendChan) 之间是非零时间窗口；这段窗口
writeLoop 仍在跑、并发 Send / SendPriority 仍可入队、handlePing 仍可推 pong。
结果：client 看到的 wire 帧序列可能是
`(close frame) → (residual data frame)`，违反 V1 §12.1 + RFC 6455 钦定的
"close frame is the last frame on the connection"。轻则触发客户端协议错误
日志噪声，重则触发 gorilla 客户端 ReadMessage 进入 unrecoverable error 状态。

### 根因（Root cause）

把"emit close frame"做成 best-effort 单步操作 + 之后再做 cleanup，没考虑
writeLoop 是独立 goroutine 在并发 drain channels。"先发 close frame 再关
连接"看起来对，但忽略了：close frame 的协议语义不仅是"信号"，还约束了**之后
不能再有任何 frame 写到 wire**。该约束只能通过"在写 close frame 之前停止
writeLoop"实现，不能通过"写完 close frame 就立即 conn.Close()"实现 —— writeLoop
和 WriteControl 共享 conn，conn.Close() 在 writeLoop 写一帧的中间也不能让
writeLoop 立即停。

### 修复（Fix）

抽 `closeInternal(emitClose bool, code int, reason string)` 共享路径，正确顺序：

```
1) atomic CAS 翻 closed flag + close(sendChan) + close(sendPriorityChan)
2) 等 writeLoop 退出（writeLoopDone signal）
3) WriteControl close frame（仅 CloseWithCode 路径）
4) cancelCtx + conn.Close + notifier.notifyClosed
```

Step 1 让并发 Send / SendPriority 立即看到 closed=true 返 ErrSessionClosed；
Step 2 让 writeLoop 看到关闭的 channel 退出循环（select 命中 `case msg, ok := <-ch
{ if !ok { return } }`）；Step 3 此时是 conn 上的唯一写者，没有 race。

`writeLoopDone` 是新加的 `chan struct{}`：writeLoop 的 defer 里**先**
`close(s.writeLoopDone)` **再** `_ = s.Close()`。顺序关键：如果反了，writeLoop
defer 内的 Close 会 block 在 closeOnce.Do（等 closeInternal 跑完），而
closeInternal 又在等 writeLoopDone → 死锁。

`Close()` 也走 `closeInternal(emitClose=false, ...)`，但需要兼容入口语义
"`Close()` 第二次调用返 nil"（`TestSession_Close_Idempotent` 钦定）—— 包装
吞掉 `errors.Is(err, ErrSessionClosed)` 的 sentinel 返 nil。

加测试：
- `TestSession_CloseWithCode_SendReturnsErrAfterCloseWithCode`：CloseWithCode
  返回后 Send / SendPriority 立即返 ErrSessionClosed
- `TestSession_CloseWithCode_ConcurrentSend_StopsImmediately`：4 个 goroutine
  并发 Send 时 CloseWithCode；返回后 Send 必须立即 ErrSessionClosed（不能侥幸
  入队）

### 预防规则（Rule for future Claude）⚡

> **一句话**：写"协议钦定的最后一个 frame"（WS close frame、HTTP/2 GOAWAY、
> SMTP QUIT 等）**之前**，必须先停掉所有可能并发写 conn 的 goroutine ——
> 不能依赖"写完 last frame 再关 conn"，conn 共享方在 frame 写到一半也不能
> 立即被打断。
>
> **展开**：
> - **正确顺序**：（1）原子翻 closed flag → 让入口路径立即拒绝新 work（
>   `Send` / `Write` / etc）；（2）关闭所有让 worker 退出的 channel；（3）
>   等 worker goroutine 用 done signal 报告退出；（4）才能调用 protocol-level
>   "last frame" 写入（WriteControl / GOAWAY / QUIT）；（5）最后释放 conn。
> - **死锁陷阱**：worker goroutine 的 defer 里如果调"对自己的 Close"+用
>   `done signal` 通知关闭路径，**必须**先 `close(done)` 再 `Close()` ——
>   反了的话 defer 的 Close 会 block 等 sync.Once 完成，sync.Once 又在等
>   done signal → 死锁。本 lesson 的实装通过"writeLoop defer 顺序：先
>   close(writeLoopDone) 再 _ = s.Close()" 锚定。
> - **测试要求**：测 close 路径的"入口面"（Send/SendPriority 在 CloseWithCode
>   返回后立即返 ErrSessionClosed），不要纠结"wire 上是不是真没 data frame
>   跟在 close 后面"——后者由实装层保证，gorilla ReadMessage 收到 close
>   后即 surface error，client side 难以做精确顺序断言。但入口面的"立即拒绝
>   新 work"是 close frame 顺序保证的**充分条件**：如果入口面在 CloseWithCode
>   返回后还能入队，writeLoop 一定还能写 → close frame 后必有 data frame。
> - **反例**：原版实装 `WriteControl(...); return s.Close()` —— 中间窗口
>   writeLoop 仍在 drain sendChan，pong / 业务 msg 都可能写到 close frame 之后。
> - **类比**：HTTP/1.1 chunked encoding 的 `0\r\n\r\n` 终结块、gRPC 的 trailer
>   metadata、Redis CLUSTER FAILOVER 的 OK 响应 —— 所有"协议钦定的终结信号"
>   写入前必须先 quiesce data path。

---

## Meta: 本次 review 的宏观教训

两条 finding 都是"跨 goroutine 协作的同步语义被忽略"。共通教训：

> **跨 goroutine 边界的状态读 / 副作用执行，必须在执行端重新校验前置条件 +
> 显式同步退出信号**；不能依赖"主循环读完了 → 一定还成立"的假设。Go 的并发
> 原语（channel close / sync.Once / done signal）每一个的语义都是 **消息传递**，
> 不是 **共享内存**——主循环看到的 lastHeartbeatAt / closed flag 永远只是它读
> 那一瞬间的快照。

历史上类似 lesson 已有：
- `2026-04-27-actor-coalesce-cleanup-must-bind-resource-not-caller.md`（actor
  cleanup 必须绑资源不是 caller）
- `2026-05-06-ws-session-send-close-race-and-shutdown-hooks.md`（Send/Close
  RWMutex 互斥防 send-on-closed-channel panic）

本次两条进一步 distill：
1. **判定-执行模式**必须在执行端重读（lesson 1）；
2. **协议钦定的"最后帧"**写入前必须 quiesce 所有 conn 写入路径（lesson 2）。
