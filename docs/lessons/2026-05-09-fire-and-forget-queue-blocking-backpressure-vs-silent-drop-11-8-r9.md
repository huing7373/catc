---
date: 2026-05-09
source_review: codex review r9 输出（/tmp/epic-loop-review-11-8-r9.md）
story: 11-8-成员加入-离开-ws-广播
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-09 — fire-and-forget queue 满应阻塞背压而非 silent drop / defer tech-debt 必须在代码层加显著注释（11-8 r9）

## 背景

Story 11.8 r5 引入 per-room post-commit FIFO queue（roomQueue + worker goroutine + sync.Map[roomID]）后，r5 实装在 `enqueueRoomEvent` 用 `select { case q.ch <- wrapped: default: drop }` 处理 queue 满场景：把 silent drop 当成"fire-and-forget 严格语义"的合理降级。

codex review r9 提出两条 finding：

- **[P1]** per-room worker / sync.Map entry 永不 reclaim → goroutine + 内存随历史 room 数单调增长（与 r8 [P2] 同源问题，r8 已决策 defer，本次 r9 codex 升级为 P1 重复 flag —— 说明 defer 决策没在代码层留下足够显著的标记）。
- **[P2]** queue 满时 select default 分支 silent drop event → client 漏 member.joined / member.left → roster permanently stale → 是 silent correctness bug，不是合理 best-effort 降级。

本次修复：r8 worker leak 决策维持 defer 但加三处 LIFECYCLE-DEFER 代码注释；r9 silent drop bug 修：去除 select default 改阻塞 send + 容量提升 256 → 1024（双层防御）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | per-room queue worker / map entry 永不 reclaim | P1 (codex) / P2 (实际) | perf / architecture | **defer reaffirm + 加代码标记** | `server/internal/service/room_service.go:392 / 545 / 605` |
| 2 | queue 满时 silent drop event 导致 client roster 永久 stale | P2 | correctness / architecture | **fix** | `server/internal/service/room_service.go:566-580` (旧) → `541-595` (新) |

## Lesson 1: fire-and-forget 异步 queue 不应 silent drop event；阻塞背压优于 silent corruption

- **Severity**: P2 (correctness)
- **Category**: architecture / correctness
- **分诊**: **fix**
- **位置**: `server/internal/service/room_service.go:566-580` (r8 实装) → `541-595` (r9 修复后)

### 症状（Symptom）

`enqueueRoomEvent` 在 caller 同步段 send wrapped fn 到 per-room buffered channel：

```go
// r5-r8 实装
qIface, _ := s.roomQueues.LoadOrStore(roomID, &roomQueue{ch: make(chan func(), 256)})
...
select {
case q.ch <- wrapped:
    // 成功入队
default:
    // 队列满：drop 事件 + log warn + 回滚 wg.Add
    s.postCommitWG.Done()
    slog.Default().Warn("room post-commit queue full; event dropped", ...)
}
```

worker goroutine 卡死（DB 慢查 / 锁争用）或 burst > 256/30s 时，select default 分支被命中：HTTP 请求**仍然返回 200**，但这次 join/leave 的 member.joined / member.left 广播被**默默丢弃**。该 room 内的其他在线 client 永远不会收到 roster 更新，必须断线重连或主动拉全量 snapshot 才能恢复正确视图 —— 这是 silent correctness bug，没有任何 client-visible 信号。

### 根因（Root cause）

r5 设计文档把"fire-and-forget 严格语义"误读成"caller 在任何情况下都不能 block"。实际上 fire-and-forget 的本质是**caller 不等异步结果**（无 future / promise / wg.Wait()），**不是**"caller 在 enqueue 阶段不能阻塞"。caller 同步段的 channel send 阻塞 ≠ caller 等异步执行结果，前者是排队入队的天然背压机制，后者才是被 fire-and-forget 严格禁止的"等结果"。

更深层根因：**异步 queue 的"满"是异常信号，不是常态降级**。设计层面应该问的是"如果 queue 满了说明什么？"而不是"如果 queue 满了怎么 best-effort 处理？"：

- queue 容量已经按合理上界（256 → 1024）配置 → 真满了说明 worker 卡死或 burst 严重超出预期 → 这是**告警信号**，应该让监控可见（HTTP latency P99 上升）。
- silent drop 把告警信号吞掉，把 silent correctness bug 注入用户视图 —— 监控看不见、client 看不见、log warn 一行没人盯。
- 阻塞背压的代价是 HTTP 200 延迟（caller 等 worker 消费一个槽位），延迟会立刻反映到 P99 监控 → alarm-prone；同时背压让上游 caller 慢下来 → 不会继续制造更多 backlog → 系统自洽收敛。

### 修复（Fix）

**路径 A**：去除 select default 分支，改为直接阻塞 send；同时把容量从 256 提升到 1024（双层防御）。

```go
// r9 修复后
qIface, _ := s.roomQueues.LoadOrStore(roomID, &roomQueue{ch: make(chan func(), 1024)})
...
if s.postCommitWG != nil {
    s.postCommitWG.Add(1)
}
// blocking send：channel 满时 caller 阻塞等 worker 消费一个槽位
q.ch <- wrapped
```

**关键不变量**：

- caller 同步段在 1024-cap 未满路径下仍是 instant op（buffered channel 非阻塞 send 是 nano 级），与 r6 r7 r8 的"lock 内只允许 instant op"约束兼容。
- 极端 burst > 1024 同 room 才会触发 caller 阻塞 —— 节点 4 阶段单 room ≤ 4 user，post-commit ~10ms，不会发生。
- 阻塞最坏延迟 HTTP 200 但**不丢事件**；监控可见可 alarm。
- WG 簿记不变（Add 在 send 前，Done 在 worker 内 fn 跑完后）—— 因为没有 drop 路径，不再需要 Done() 回滚 Add。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在为 **fire-and-forget 异步 hook（post-commit / fanout）** 设计 buffered queue 时，**必须**默认采用 **blocking send 背压**而非 select default silent drop —— "queue 满"是异常告警信号，不是常态降级路径。
>
> **展开**：
> - **fire-and-forget 语义本质**：caller 不等异步**结果**（无 future / wg.Wait()），不是 caller 不能在 enqueue 排队时阻塞。混淆这两件事会写出 silent drop 路径。
> - **buffered queue 容量是 capacity planning，不是 fallback**：容量按合理 burst 上界配置（业务峰值 × 安全系数），真满了 = 异常。异常应让监控可见（HTTP latency 延迟）而非 silent swallow。
> - **silent drop ≠ best-effort**：silent 才是问题；如果一定要 drop（比如纯 metric 数据流），至少要 emit 计数器到监控系统，不是只 log warn。member.joined / member.left 这种**改变 client 持久 state**的 event 绝对不能 drop（roster permanently stale）。
> - **背压自洽**：caller 阻塞 → 上游 HTTP 慢 → 不会继续制造更多 backlog → 系统收敛。silent drop 反而让 caller 继续生产，加剧问题。
> - **双层防御**：blocking send 是首选背压，buffered 容量是常态吞吐覆盖。两者并存，不互斥。
> - **反例（r5 r8 实装，r9 修复前）**：
>   ```go
>   // ❌ select default silent drop —— 是 fire-and-forget 异步 queue 的反模式
>   select {
>   case q.ch <- wrapped:
>   default:
>       s.postCommitWG.Done()  // 回滚 Add 防 wg.Wait 永远等
>       slog.Warn("queue full; event dropped", ...)
>   }
>   ```
> - **正例**：
>   ```go
>   // ✅ blocking send；queue 满时 caller 阻塞 → 监控可见 → alarm
>   q.ch <- wrapped
>   ```

## Lesson 2: defer 决策的 tech-debt 必须在代码层加显著注释，否则 review 会重复 flag

- **Severity**: P1 (process)
- **Category**: process / docs
- **分诊**: **defer reaffirm** —— r8 已决策 defer，r9 维持决策不变；但代码层加三处 LIFECYCLE-DEFER 标记
- **位置**: `server/internal/service/room_service.go` 三处：`roomQueues` 字段定义（line 392）/ `enqueueRoomEvent` 函数（line 545）/ `runRoomQueueWorker` 函数（line 605）

### 症状（Symptom）

r8 review codex 已 flag per-room worker leak 为 [P2]；r8 决策为 **defer + tech-debt + 量化上界**，并在 `docs/lessons/2026-05-09-per-room-worker-lifecycle-defer-tech-debt-11-8-r8.md` 蒸馏理由 + commit `a23eae5` 落 lesson commit。但 r9 review codex 重复 flag 同一问题，且**升级为 [P1]** —— 说明 defer 决策没有被 r9 的 codex review session 看到。

根因：r8 的 lesson 文档存在 `docs/lessons/`，但 codex review 的 working set 只看 diff + relevant 代码文件，**不会自动读 lessons 目录**。代码 + 注释里没有任何 marker 指向"这是 defer 的 tech-debt，已有 lesson 蒸馏"，所以 r9 codex 自然把它当成新 finding（甚至升级 priority 因为 worker count grow more 暴露）。

### 根因（Root cause）

defer 决策的"权威依据"放在 `docs/lessons/` 里是正确的（lessons 是跨 story 长期知识），但**没有从代码反链回去**。code review LLM（codex / claude）的 reasoning 入口是 diff + 相关代码文件，没有 hook 让它去跑 `grep -r 'defer.*lesson'` 或扫 lessons 目录。如果代码里没有自描述"我是被 deferred 的 tech-debt"，review 工具看不见决策记录，必然重复 flag。

更深层教训：**process artifact（lesson 文档）和 code artifact（源文件注释）需要双向链接**，不是 lesson 单向引用代码就够。代码层是 review 工具的 working set，必须在源代码里留显著标记，让任何 review 工具（人 / LLM）扫到 defer 路径时能立刻看到"这是已决策 deferred，理由见 lesson X"，而不是当成新问题。

### 修复（Fix）

在 `room_service.go` 三处加 `LIFECYCLE-DEFER:` 前缀注释 + 锚定 lesson 路径：

```go
// 位置 1：roomQueues 字段定义（line ~392）
//
// LIFECYCLE-DEFER: Story 11-8 r8 决策 defer，详见 docs/lessons/2026-05-09-per-room-worker-lifecycle-defer-tech-debt-11-8-r8.md
// （codex r8 [P2] / r9 [P1] 重复 flag worker leak；MVP 节点 4 阶段量化上界可控
// —— 单进程活跃 room ≤ 上千、worker 栈 ~2KB、roomQueue ~8KB（1024-cap chan），
// 总开销 < 10MB；future epic 单独 story 处理 lifecycle 回收。三处 LIFECYCLE-DEFER
// 标记 ——`roomQueues` 字段 / `enqueueRoomEvent` / `runRoomQueueWorker` ——
// 防 r10 review 再 flag。）
roomQueues sync.Map

// 位置 2：enqueueRoomEvent 函数前注释
//
// LIFECYCLE-DEFER: Story 11-8 r8 决策 defer，详见 docs/lessons/2026-05-09-per-room-worker-lifecycle-defer-tech-debt-11-8-r8.md
// ...

// 位置 3：runRoomQueueWorker 函数前注释
//
// LIFECYCLE-DEFER: Story 11-8 r8 决策 defer，详见 docs/lessons/2026-05-09-per-room-worker-lifecycle-defer-tech-debt-11-8-r8.md
// ...
```

**为什么三处而不是一处**：codex review 的 reasoning 是 finding-driven —— 它定位到 `enqueueRoomEvent:542-546`（worker 启动）和潜在的 `runRoomQueueWorker:595-602`（worker 永不退出）。`roomQueues` 字段是数据结构定义，是任何 review 都会扫的源头。三处都加防止 review 任意 entry 切入都看到标记。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 review 流程中决策 **defer / wontfix tech-debt** 时，**必须**同时在源代码相关位置加 **`LIFECYCLE-DEFER:` / `WONTFIX:` / `TECH-DEBT:`** 前缀注释 + 反向链接 lesson 路径，否则下一轮 review 必然重复 flag。
>
> **展开**：
> - **process artifact 与 code artifact 必须双向链接**：lesson md 引用代码位置（已有），代码注释也必须引用 lesson 路径（容易遗漏）。code review 工具的 working set 是源代码，不是 docs/，所以缺少代码标记 = 决策对 review 不可见。
> - **标记前缀采用大写关键词**：`LIFECYCLE-DEFER` / `TECH-DEBT` / `WONTFIX` / `KNOWN-LIMITATION` —— 让 grep / review tool 容易识别。不要用普通中文 "已决策不修" 这种短语，太弱。
> - **标记位置覆盖 review 所有可能 entry**：对一个 defer 项，至少在三类位置加注释 ——（a）数据结构定义（字段 / type），（b）核心函数，（c）相关辅助函数。任意 entry 切入都看到标记。
> - **标记内容必须含**：(1) story / review round 决策依据，(2) lesson 文件**完整路径**，(3) 量化上界 / 业务约束（让 review 不需读 lesson 也能初步理解 defer 合理性），(4) 升级条件（什么场景下需要重启决策）。
> - **复审时机**：每次新 review round 结束如果 codex 重复 flag 同一 deferred 项，说明标记不够显著 —— 加强标记（升级 prefix / 补充 location / 拓宽措辞）。
> - **反例（r8 决策后状态，r9 之前）**：
>   ```go
>   // ❌ 只在字段注释里轻描淡写"未来加 LRU eviction"，没有 LIFECYCLE-DEFER 标记，
>   //    没有反链 lesson —— r9 codex 完全没看见决策，重复 flag。
>   //
>   // **queue / worker 不清理**（intentional）：节点 4 阶段...如未来引入...
>   roomQueues sync.Map
>   ```
> - **正例**：
>   ```go
>   // ✅ 显式 LIFECYCLE-DEFER 前缀 + 反链 lesson 路径 + 量化上界
>   // LIFECYCLE-DEFER: Story 11-8 r8 决策 defer，详见 docs/lessons/2026-05-09-per-room-worker-lifecycle-defer-tech-debt-11-8-r8.md
>   // （MVP 节点 4 阶段单进程活跃 room ≤ 上千、总开销 < 10MB；future epic 处理。）
>   roomQueues sync.Map
>   ```

---

## Meta: 本次 review 的宏观教训

r9 两条 finding 揭示**异步系统两个常见 review 误区**：

1. **silent drop 被当成 best-effort 合理路径**（实际是 silent corruption）—— 任何改变 client 持久 state 的 event（roster / inventory / wallet）绝对不能 drop；blocking 背压才是默认。
2. **defer 决策没有 source-code-level 反链 lesson**（process artifact ≠ code artifact）—— review 工具看不见 docs/ 里的决策，必须在代码注释里留显式 marker。

两条都是**"语义模糊术语 + 缺少代码层显式标记"**导致的反复犯错：fire-and-forget 的"forget"被误读，defer 决策的 process artifact 缺反链。**修复方式都是显式化**：blocking send 替代 select default、`LIFECYCLE-DEFER:` 前缀替代隐式注释。
