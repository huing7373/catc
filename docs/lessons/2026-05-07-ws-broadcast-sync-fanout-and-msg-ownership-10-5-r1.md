---
date: 2026-05-07
source_review: codex review (file: /tmp/epic-loop-review-10-5-r1.md) for Story 10-5 broadcasttoroom-primitive
story: 10-5-broadcasttoroom-primitive
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-07 — WS BroadcastToRoom 同步 fanout + msg buffer ownership 隔离（10-5 r1）

## 背景

Story 10.5 实装 `ws.BroadcastToRoom` package-level primitive：把已序列化的 msg 字节流推送给目标 room 内所有 active Session。10-5 r1 codex review 指出实装的 fire-and-forget goroutine fanout 引入两个细粒度 runtime correctness bug：

1. **per-session 顺序破坏**：连续两次 `BroadcastToRoom(msg1)` → `BroadcastToRoom(msg2)`，goroutine 调度无序导致同一 session 可能先收 msg2 再收 msg1（破坏 "room 广播是 ordered stream" 的语义）
2. **msg buffer ownership race**：fanout goroutine 在 `BroadcastToRoom` return 后才调 `s.Send(msg)` → caller 复用 / mutate 原 msg buffer 时与 goroutine 抢字节 → 部分 client 收到 corrupted bytes

修法采用方案 (c)：**同步 fanout + 入口 bytes.Clone**（一举两得，最干净的修法）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | fanout goroutine 不保证 per-session 顺序 across 连续 broadcasts | high | architecture | fix | `server/internal/app/ws/broadcast.go:134-146` |
| 2 | fanout goroutine 持有 caller msg buffer 引用，return 后存 race | medium | architecture | fix | `server/internal/app/ws/broadcast.go:136-154` |

## Lesson 1: fanout goroutine 不保证 per-session 顺序 across 连续 broadcasts

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/app/ws/broadcast.go:134-146`

### 症状（Symptom）

`BroadcastToRoom` 实装走 fire-and-forget goroutine fanout（每个 Session 启一个 goroutine 调 `s.Send(msg)`），主函数立即 return。caller 在同一 goroutine 里依次调用 `BroadcastToRoom(msg1)` 和 `BroadcastToRoom(msg2)`（典型场景：room 内某用户加入 → server 广播 `member.joined`，再立即广播 `member.left`），由于 fanout goroutine 与 fanout goroutine 之间的调度顺序由 Go runtime 决定，**同一个 Session 看到的入队顺序无法保证**。client 端可能先收到 `member.left` 再收到 `member.joined`，破坏 "room 广播是 ordered stream" 的语义。

### 根因（Root cause）

把"fanout 的语义"误等同于"fire-and-forget 的语义"。fire-and-forget 只是说 caller 不等结果；不代表必须用 goroutine 实现 fanout。把 N 个 Session 的入队动作并行化（goroutine fanout）只在每个动作本身阻塞时间显著时才有意义；但 `Session.Send` 是非阻塞 select-default 入队（O(1)），没有阻塞可言 → goroutine fanout 唯一引入的是**调度无序**，没有任何性能收益。

### 修复（Fix）

`broadcastToRoomFanout` 实装从 goroutine fanout 改为**同步 for-range 调 Session.Send**：

```go
// before: goroutine fanout，调度无序
for _, s := range sessions {
    s := s
    go func() {
        defer wg.Done()
        if sendErr := s.Send(msg); sendErr != nil { ... }
    }()
}
if wait { wg.Wait() }

// after: 同步遍历，跨调用顺序保证
for _, s := range sessions {
    if sendErr := s.Send(payload); sendErr != nil {
        logger.Warn("ws broadcast Send failed", ...)
    }
}
```

同步后：caller 在同 goroutine 调 `BroadcastToRoom(msg1)` → `BroadcastToRoom(msg2)`，msg1 入队所有 session sendChan **后** BroadcastToRoom 才 return → caller 调 msg2 入队 → 所有 session 的 sendChan 内 msg1 物理位置先于 msg2 → writeLoop FIFO 消费写到 conn → client 端观察到 msg1 在 msg2 之前。

副作用：`broadcastToRoomFanout` 的 `wait bool` 参数失去意义（同步后必然 wait），与 `BroadcastToRoomForTest` 一并简化（ForTest 直接 call `BroadcastToRoom` 即可）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在实装"对多个 sink 广播一条 msg"的 primitive 时，**禁止**为了"看起来 concurrent"而启 goroutine fanout，**必须**先确认每个 sink 的 send 操作本身是否阻塞 —— 非阻塞 send（如 select-default 入队 chan）必须用同步 for-range，因为 goroutine fanout 唯一引入的是调度无序，没有任何性能收益但破坏跨调用顺序保证。
>
> **展开**：
> - 判断 sink 的 send 阻塞性：阻塞 → goroutine fanout 才有意义；非阻塞（O(1) select-default 入队 / atomic / lock-free queue）→ 必须同步遍历
> - 同步遍历的 caller-visible 不变量：`primitive return` 时点等价于"所有 sink 入队完成"时点 → 跨调用顺序自动保证（caller 同 goroutine 依次调用 → 所有 sink 看到的顺序与 caller 调用顺序一致）
> - 对 broadcast / fan-out / pub-sub 类 primitive 尤其要警惕：业务上很少需要"广播本身并行"（每个 sink 收到时间独立无 deadline），但很常需要"跨多次广播保证顺序"
> - **反例**：实装 `ChannelManager.Publish(channel, msg)`，对 channel 内 1000 个 subscriber 启 1000 个 goroutine 各自 `subscriber.input <- msg`。如果 subscriber.input 是 buffered chan（select-default），goroutine fanout 就是**纯负优化**：不仅没有性能收益（每个 send 本来就 O(1)），还引入了 publisher 连续两次 `Publish(msgA), Publish(msgB)` → 同一 subscriber 收到 msgB 在 msgA 之前的可能。正解是同步 for-range。
> - **反例**：把 "fire-and-forget" 当作 "必须 goroutine"。fire-and-forget 只是说 caller 不等 callee 完成；callee 实现是不是同步与 fire-and-forget 语义无关。同步实现的 fire-and-forget = "调用方拿到 return 时所有同步动作已完成（入队），后续动作（writeLoop / consumer）才是异步"，这个语义对很多 primitive（broadcast / log fanout / event multicast）都是更稳健的选择。

## Lesson 2: fanout goroutine 持有 caller msg buffer 引用，return 后存 race

- **Severity**: medium
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/app/ws/broadcast.go:136-154`

### 症状（Symptom）

`BroadcastToRoom` 主函数 wait=false 路径下，主函数 return 时 fanout goroutine 还**没**调 `s.Send(msg)`。caller 如果用 reusable buffer 序列化 msg（如 `bytes.Buffer.Bytes()` 或 sync.Pool 拿出来的 buffer）→ caller return 后立即 reset / mutate buffer → 与 fanout goroutine 抢字节 → 部分 client 收到 corrupted bytes。

API 形态上让这个误用极易触发：caller 看到 `func(ctx, mgr, roomID, msg []byte)` 签名只能猜 msg 的 ownership 语义；既有注释 "不在 primitive 内修改 msg / 不复制 msg" 让 caller 误以为可以放心 reuse / mutate。

### 根因（Root cause）

`[]byte` 参数的 ownership 语义在 Go 里是**模糊**的：caller 既可以承诺"return 后不再 mutate"，也可以期望 callee 自己 clone。当 callee 是异步路径（goroutine 在 caller return 后才 read msg）时，模糊语义就等价于 race。primitive 设计者的责任是：**异步路径 + 字节切片 = 入口必须 defensive copy**，把 ownership 隔离明示在实装里，不靠注释要求 caller 配合。

### 修复（Fix）

`broadcastToRoomFanout` 入口加 `bytes.Clone(msg)`：

```go
// 入口 clone，与 caller buffer 完全隔离
payload := bytes.Clone(msg)

// 后续 Send 用 payload 而非 msg
for _, s := range sessions {
    if sendErr := s.Send(payload); sendErr != nil { ... }
}
```

`bytes.Clone(nil)` 返 `nil`（Go 1.20+ stdlib 行为）→ 与既有 nil-msg 测试兼容。

成本：每次 broadcast 多一次 O(len(msg)) 的 alloc + copy。对 sub-KB 级 msg（V1 协议典型 envelope 100~500 字节）可忽略；对 1000 session × 500 字节 = 500KB 的 fanout 也只多 500 字节 alloc。换来的是 caller-side ownership 完全隔离 —— 任意 buffer reuse 模式都安全。

合并 Lesson 1 的同步化后，"caller return 时所有 session sendChan 入队完成" + "payload 与 caller buffer 隔离" → caller 在 BroadcastToRoom return 后**立即** reset / reuse / mutate 原 buffer 完全安全。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在实装"接受 caller-owned `[]byte` 然后异步消费"的 primitive 时，**必须**在入口 `bytes.Clone(msg)` 把 ownership 明示隔离 —— 不要靠注释要求 caller "保证不 mutate"，因为字节切片 ownership 的模糊语义在 Go 里没有 type system 兜底，靠注释等同于靠运气。
>
> **展开**：
> - 判定标准：primitive return 后是否还有 goroutine / chan / cache 持有 msg slice 的引用？是 → 必须 clone；否（同步消费完 return）→ 可不 clone
> - 同步消费的特例：本 fix 的 same-call sync fanout 已让 callee return 时所有 Session.Send 入队完成，但 sendChan 内仍持有 slice header → writeLoop 异步消费才写到 conn → 所以**仍要 clone**（caller 在 return 后 mutate 原 buffer 仍会 race writeLoop）
> - clone 成本权衡：sub-KB msg 可忽略；MB 级大 msg 才需要权衡是否要 caller 端契约约束（"不要 mutate" 注释 + linter 检查）替代 clone
> - **反例**：实装 `EventBus.Publish(event []byte)`，文档承诺"event 在 Publish 后由 caller 拥有，bus 内部已处理"，但实装是把 event slice 入队到内部 chan 让 background goroutine 消费 → caller 看到 Publish 返回，立即 reuse buffer → background goroutine 读到 mutated 字节。正解是入口 `event = bytes.Clone(event)` 或者改 API 形态（接受 `func() []byte` callback 让 caller 在 callee 决定的时机生产 msg）。
> - **反例**：实装 `Logger.LogBytes(b []byte)` 把 b 写到 file via async writer，注释要求 caller "do not modify b after return"。3 个月后另一个 Claude 来加新 caller，没读注释，从 sync.Pool 拿 buffer 序列化完调 LogBytes 然后 Put 回 pool → race。注释式 ownership 契约**等于**没有契约，必须由实装强制（clone）。

---

## Meta: 本次 review 的宏观教训

两条 finding 都指向同一个**异步 primitive 设计反模式**：把 fire-and-forget 语义错等价于 goroutine fanout，再让 goroutine fanout 持有 caller-owned 字节切片。两个 bug 单独都不致命，但叠加在一个 primitive 里就让 "正确使用" 的窗口非常小（caller 必须既不依赖跨调用顺序，又不能 reuse msg buffer）。

修复方案 (c) 同步 + clone 的优雅之处：两个修法相互正交但相互加强 —— 同步化让 "return 时点 = 入队完成时点" 不变量成立；clone 让 "入队的 payload 与 caller msg 完全独立" 不变量成立。两者一起达成 "caller 在 BroadcastToRoom return 后无任何隐藏约束（顺序保证 + buffer 任意复用）" 的最强 caller-side 契约。

未来设计 broadcast / fanout / pub-sub 类 primitive 时，先检查这两个不变量是否同时成立 —— 不成立就不要发布给 caller，再讨好的注释也不行。
