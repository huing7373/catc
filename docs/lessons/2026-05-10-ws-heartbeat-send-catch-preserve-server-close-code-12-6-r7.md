---
date: 2026-05-10
source_review: codex review (epic-loop r7) — /tmp/epic-loop-review-12-6-r7.md
story: 12-6-心跳维护
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-10 — WS heartbeat send catch 路径不能用 cancel(.goingAway) 注入 1001 覆盖 server 真实 close code（terminal vs transient contract 守护）

## 背景

Story 12.6 心跳维护 round 7：codex 发现 `WebSocketClientImpl.swift:1392-1436` heartbeat send 失败 catch 路径
**无条件**调 `cancelUnderlyingTaskWithGoingAwayIfCurrent`（注入 1001 = transient `.goingAway`），但 socket
可能**已被 server 用 terminal close code（4001/4002/4003/4004 等）关闭**，receive loop 还没消费到那个
close frame —— 1001 注入会**覆盖** server 真实 close code（`URLSessionWebSocketTask.closeCode` 字段被
后到的 cancel 覆盖），导致 receive loop classify 走 transient → schedule reconnect 而非 terminal →
emit `.disconnected` → caller re-auth，破坏 12.5 钦定的 terminal-vs-transient contract。

review 来源：r7 文件 6142 行，真实结论在 `^codex$` 段（line 6130 起）；前面 6129 行是历史 lesson md /
git diff 引用，全部忽略（CLAUDE.md 工作流约束 + epic-loop override 钦定的解析规则）.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | heartbeat send catch 不能用 1001 覆盖 server terminal close code | high | architecture | fix | `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:1392-1436` |

## Lesson 1: heartbeat send 失败 catch 路径必须先观测 underlyingTask.closeCode；server 已发 close frame 则 silent skip cancel

- **Severity**: high (P1 — 破坏 12.5 terminal-vs-transient contract)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:1392-1436`

### 症状（Symptom）

post-handshake server-initiated terminal close（如 4001 token 过期）与 heartbeat send 之间的 race：

```
T0  heartbeat send（在 socketA 上）suspended（async I/O）
T1  server 发送 close frame（4001 token 过期）→ URLSessionWebSocketTask runtime
    把 task.closeCode 设为 4001（read-only property，server close frame 到达时 runtime 设置）
T2  receive-loop 还没消费到 close（异步调度延迟）
T3  T0 那个 send 终于抛错（socket 已被关）
T4  catch 跑 → 旧实装无条件调 cancelUnderlyingTaskWithGoingAwayIfCurrent
    → URLSessionWebSocketTask.cancel(with: .goingAway, ...) → closeCode 字段被覆盖为 1001
T5  receive-loop 终于跑到 catch → 读 task.closeCode → 拿到 1001（已被 T4 覆盖）→
    classify 为 transient → schedule reconnect 而非 emit .disconnected →
    破坏 12.5 钦定的 terminal-vs-transient contract（4001 应触发 re-auth，不应 silent retry）
```

### 根因（Root cause）

round 3 P1 修 "send 抛错时 silent return → client 卡死无 heartbeat 无 reconnect" 时，把
heartbeat send catch 改成与 pong timeout **完全相同**的 fallback —— 调
`cancelUnderlyingTaskWithGoingAwayIfCurrent` 注入 1001 走 transient reconnect 路径。

但这个对齐**只在一种 race 时序里正确**：
- **正确场景** = "locally broken socket"，server **没**发任何 close frame，
  underlyingTask.closeCode == .invalid（rawValue=0）→ 1001 注入是合理的，
  让 receive-loop 拿到 1001 → classify transient → reconnect。
- **错误场景** = "post-handshake server-initiated close" 与 heartbeat send 的 race，
  server **已**发 close frame，underlyingTask.closeCode 已被 runtime 设为真实 code（4001/4002/...）→
  1001 注入会**覆盖**真实 close code，破坏分类。

旧实装漏掉的 invariant：**`URLSessionWebSocketTask.closeCode` 是 single-write field —— 一旦被 server
close frame 或本地 cancel 设置后，再次 cancel(with:) 会**覆盖**它**。所以注入 close code 之前必须
先观测当前值；只有 .invalid（即"还没人写过"）才能注入。

`WebSocketTaskHandle` protocol（Story 12.5 实装）已暴露 `var closeCode: URLSessionWebSocketTask.CloseCode { get }`
getter，本来就是为 receive-loop 在 catch 时按真实 close code 分类决策准备的；heartbeat catch 路径
忽略了同一个观测点，留下了这个 race window。

### 修复（Fix）

在 heartbeat send catch 内、调 `cancelUnderlyingTaskWithGoingAwayIfCurrent` 之前，**先**观测
`activeTask.closeCode`：

```swift
let observedCloseCode = activeTask.closeCode  // 持锁前观测，避免 hold-while-getting

// ... existing latch cleanup + task identity 校验（round 5 P1）...

// round 7 P1：server 已发送 terminal/transient close → silent skip cancel
if observedCloseCode != .invalid {
    os_log(.info, "heartbeat send catch: server-initiated close already in flight (closeCode rawValue=%d) — silent skip cancel; receive-loop will classify real close code", observedCloseCode.rawValue)
    return
}

// 没有 server close → 走原有 transient reconnect 路径（locally broken socket 场景）
strongSelf.cancelUnderlyingTaskWithGoingAwayIfCurrent(mySession: mySession)
```

变更点：
- `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift` heartbeat catch 块新增 closeCode 观测分支
- `iphone/PetAppTests/Core/Networking/WebSocketClientImplTests.swift` 新增 `test_heartbeat_sendCatchPreservesServerCloseCode_round7_P1` 回归测试（hook 内切 stubbedCloseCode 到 4001 模拟 server close frame 到达 → send 抛错 → 断言 firstTask.lastCancelCloseCode != .goingAway）
- 同文件 `test_heartbeat_pingSendThrows_cancelsUnderlyingWithGoingAwayTriggersReconnect`（round 3 P1）pre-set `stubbedCloseCode = .goingAway` 改为保持默认 `.invalid` —— 原 pre-set 是为了让 receive-loop cancel 后能 classify 1001，新修复后这个 pre-set 反而 trigger silent skip；保留 .invalid 后 cancel(.goingAway) → receive-loop 仍读 .invalid 走 1006 transient（V1 §12.1 钦定 1006 等价 transient），断言路径不变。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **WebSocket（或任何"close code 影响后续状态机决策"的 OS-managed
> socket）的 send/recv catch 路径上、需要主动 cancel(with closeCode:) 触发后续 cleanup 时**，
> **必须先观测 underlyingTask 当前 closeCode**；非初始/未写值（如 URLSessionWebSocketTask 的 .invalid）
> 才能注入新 close code，否则 silent skip 让 receive-loop 拿到 server 的真实 close code 走分类。
>
> **展开**：
> - URLSessionWebSocketTask.closeCode 是 single-write semantics —— server close frame 到达 OR 本地
>   cancel(with:) 都会写入；后者会覆盖前者。任何 "我用 1001 强制走 transient 重连" 的注入逻辑必须
>   先确认这个字段还没被 server 写过。
> - heartbeat send 抛错的语义是模糊的：可能是 "locally broken socket"（server 没说话）也可能是
>   "server 已发 close 但 client 还没消费到"。catch 路径必须按 **closeCode 字段是否已写过** 区分两种
>   场景，不能简单等同 "send 抛错 = transient reconnect"。
> - 多层守护的检查顺序：先做最便宜的观测（读 closeCode getter，无锁/lock 内读都行），再做需要持锁的
>   状态校验（task identity、sessionGeneration），再决定 cancel 与否。closeCode 观测**不**依赖
>   sessionGeneration / underlyingTask 引用是否仍合法，所以放最外层。
> - **反例**：catch 路径无条件 `task.cancel(with: .goingAway, reason: ...)` 而不先校验 closeCode；
>   或仅靠 sessionGeneration / task identity 做守护（这两个守护处理"task 被 swap 到新 socket"的 race，
>   但**不**处理"task 仍是同一个但 server 已发 close"的 race）。两个守护是**正交**的，必须都加。
> - **反例**：用错误描述字符串或 send error 类型做分类（`URLError(.notConnectedToInternet)` 等）—
>   这些是脆弱的且与 close code 语义解耦；OS 可能在 send 抛错的同时 / 之前 / 之后任何时序写入 close code，
>   所以必须直接读 closeCode 字段。

## Meta: 本次 review 的宏观教训

12.6 已经修了 6 轮 race（r1–r6 各自针对一个 race window），r7 是 race 拓扑学上的最后一片：
**heartbeat send 抛错的 catch 路径 vs server-initiated terminal close** 的时序竞争。

这一系列 race 反映了一个共同模式：每当 client-side 主动状态变更（cancel / close / reconnect / swap）
与 server-side 异步事件（close frame / response delay）发生时序竞争时，**任何 "覆盖式" 操作（cancel
注入 close code、underlyingTask 替换、sessionGeneration 翻番、stream finish）都必须先观测当前态**，
不能 "盲改"。

观测点的层级（从最便宜到最贵）：
1. **read-only 字段直读**（如 `task.closeCode`）—— 无锁，O(1)，不依赖 generation/session 状态
2. **lock 内读 generation / task identity** —— 廉价，但需要锁
3. **跨 task 的 atomic CAS / continuation finish** —— 最重，但语义最强

**新规则（write into ADR-0008 candidate）**：每次 `cancel(with closeCode:)` 调用 site 必须 audit "如果
此处 close code != .invalid，注入 close code 会覆盖什么 server 信息？"。如果会破坏后续分类决策，
就必须加观测条件。
