---
date: 2026-05-07
source_review: codex review output (/tmp/epic-loop-review-10-5-r2.md)
story: 10-5-broadcasttoroom-primitive
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-07 — WS BroadcastToRoom 跨 goroutine 序一致性 & 大 N 性能测试 fixture 必须脱离 httptest（10-5 r2）

## 背景

Story 10.5（BroadcastToRoom primitive）r2 codex review。r1 修复同步 fanout 让单 goroutine 连续 broadcast 顺序保证；r2 review 指出：
1. 单 goroutine 顺序虽保住了，但**跨 goroutine** 同 room 并发 broadcast 仍可乱序，让"room 内所有 client 看到一致事件流"的协议契约破裂
2. r1 加的大 N 性能测试用 `useGatewayDial` 真起 100 个 `httptest.Server` → Windows / 慢 CI fd 耗尽 / 端口分配 flaky；100ms 断言实际测的是 server 启动开销而非 BroadcastToRoom 自身性能

两条都是 P2，全部 fix。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | per-room mutex 序列化同 room 并发 broadcast | P2 | architecture | fix | `server/internal/app/ws/broadcast.go:98-167` |
| 2 | N=100 性能测试改用 stub Session 取代 httptest.Server | P2 | testing | fix | `server/internal/app/ws/broadcast_perf_internal_test.go`（新增）<br>`server/internal/app/ws/ws_test.go:3576-3624`（删除） |

## Lesson 1: BroadcastToRoom 必须 per-room 序列化才能保证跨 goroutine 序一致性

- **Severity**: P2
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/app/ws/broadcast.go:98-167`

### 症状（Symptom）

r1 同步 fanout 改完后，**同 goroutine** 连续调 `BroadcastToRoom(roomX, msgA)` 再 `BroadcastToRoom(roomX, msgB)` 顺序确定（msg1 入队完所有 session sendChan 才到 msg2）。但**两个 goroutine** 同时调 `BroadcastToRoom(roomX, msgA) || BroadcastToRoom(roomX, msgB)`：

- A 的 for-range 遍历 sessions: [s1, s2, s3, s4]，逐个 Send(msgA)
- B 的 for-range 遍历 sessions: [s1, s2, s3, s4]，逐个 Send(msgB)
- Go scheduler 可以让两条 for-range 在任意 session 边界交错：
  - s1.sendChan: [msgA, msgB]（A 先入队）
  - s2.sendChan: [msgB, msgA]（B 抢到了）
  - s3.sendChan: [msgA, msgB]
  - s4.sendChan: [msgB, msgA]

→ 同 room 内不同 client 看到不同的事件序，破坏"room 是 ordered event stream"协议契约。room 状态变更（成员加入 / 离开 / 表情发送）由并发 handler / 后台 job 触发时直接踩坑。

### 根因（Root cause）

r1 的"同步 fanout"修复只考虑了**同一 caller goroutine** 的两次 broadcast 顺序（在 caller 持续运行的代码里 BroadcastToRoom return 形成 happens-before 链），**忽略了多 goroutine 并发 broadcast** 这条路径。

更深一层：BroadcastToRoom 的语义是"原子地把一条消息推给 room 内所有 session"。"原子"意味着任何观察者（每个 session）都看到相同的"消息序列"，**而不是**"每个 session 看到的子序列与全局序列前缀兼容"那种弱保证。同步 fanout 只让"消息 X 投递到所有 session 是连续的"成立，未让"两次 broadcast 整体不交错"成立 —— 后者必须靠互斥。

设计 broadcast primitive 时的思维漏洞：默认认为"caller 只在一处调用"或"caller 自己负责加锁"，把并发安全外推给 caller。这违反了 primitive 的"对外简单 / 内部正确"原则 —— ws 包本身就是 IO concurrency 的家，并发是常态而非 caller 责任。

### 修复（Fix）

`broadcast.go` 加包级 `var roomBroadcastMu sync.Map // map[uint64]*sync.Mutex`：

```go
func broadcastToRoomFanout(ctx context.Context, mgr SessionManager, roomID uint64, msg []byte) (int, error) {
    muVal, _ := roomBroadcastMu.LoadOrStore(roomID, &sync.Mutex{})
    mu := muVal.(*sync.Mutex)
    mu.Lock()
    defer mu.Unlock()

    sessions := mgr.ListSessionsByRoomID(ctx, roomID)
    // ... 原有 bytes.Clone(msg) + for-range Send 不变 ...
}
```

**为什么选包级 sync.Map（方案 a）而非 SessionManager 字段（方案 b）**：
- (a) 简单清晰，跨 SessionManager 实例 mutex 自然按 roomID shard，开销 < 100B/room，房间数远低于 1M 可忽略
- (b) 把 mutex map 挂在 SessionManager 上能跟 `sessionsByRoom` 一起 lifecycle 管理（archive 时回收 mutex），但本 story 阶段没有 archive 路径，复杂度收益不匹配
- (c) **直接用 SessionManager.mu** 全局锁：所有 broadcast 串行 → 跨 room 吞吐大降，**否决**

**关键不变量**：mutex 持有期间只做 fanout（`ListSessionsByRoomID` + N 次 `Send` 入队），全程 µs 级（Send 是非阻塞 select-default 入队 O(1)）。**禁止**在 mutex 内做任何阻塞 IO（DB / Redis / network）—— 那会让同 room 所有 broadcast 串行等 IO，吞吐崩溃。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计 fanout / broadcast / 多消费者投递的 primitive** 时，**必须**在设计阶段就回答"同 target 多 caller goroutine 并发投递时，所有消费者看到的序列是否需要一致" —— 答案是"是"则用 per-target mutex 序列化整个 fanout 段。
>
> **展开**：
> - "ordered event stream"协议契约 + 多 goroutine 调用环境 = 必须互斥。同步 fanout 只解决 caller 内部时序，不解决跨 caller 时序
> - per-target mutex 实装首选包级 `sync.Map[targetID]*sync.Mutex`：简单、零分配（懒加载）、自然 shard、开销可忽略；mutex 不会被 GC 回收但每条 mutex < 100B，target 数量级远低于 1M 时可忽略
> - per-target mutex 的持有时长**必须**是 µs 级 —— 内部禁止阻塞 IO（DB / Redis / network），否则同 target 所有 caller 串行等 IO，吞吐崩溃。fanout 内的"投递"动作必须是非阻塞入队（chan select-default），不能是同步 write
> - 加并发回归测试：N 个 goroutine 各推 K 条 distinct-tag 消息到同 target，drain 所有消费者 chan，断言**所有消费者看到完全相同的消息序列**（不断言全局序具体内容，scheduler 自由）。该测试在 mutex 缺失时会 fail，加上后稳定 pass
> - **反例**：r1 提交"同步 fanout 完成 → BroadcastToRoom return 时所有 sendChan 都已入队完 → 跨调用顺序保证"的描述，把 caller 内 happens-before 链外推到所有 caller，忘了 multiple caller goroutines 之间没有 happens-before。这种"看起来同步就万事大吉"是经典的并发误判，每次设计 fanout 都要主动反问"两个 caller 同时调用是否还成立"

## Lesson 2: 大 N 性能测试**禁止**用 httptest.Server 做 fixture，必须用裸 stub Session

- **Severity**: P2
- **Category**: testing
- **分诊**: fix
- **位置**: `server/internal/app/ws/broadcast_perf_internal_test.go`（新增）；`server/internal/app/ws/ws_test.go:3576-3624`（旧测试删除并留迁移注释）

### 症状（Symptom）

r1 加的 `TestBroadcastToRoom_R1_LargeN_SyncFanoutFastEnough` 注释说"避免 100 个 httptest server 的端口耗尽"，**但实际**通过 `useGatewayDial` 调 `httptest.NewServer` 100 次。Windows / 慢 CI 上 100 个 listening socket + TCP backlog 真的可能耗尽 fd / 端口；更糟的是 100ms 断言里 server 启动开销占 80ms+，"BroadcastToRoom 是否快"的信号被淹没。

### 根因（Root cause）

测试 fixture 选型错误。`useGatewayDial` 是 ws_test 包的"全栈握手"helper —— 它的设计目标是"测真实 wire IO 路径"（gateway upgrade、token 校验、snapshot 下发、Register），适合 single-session 集成测试。把它当作"批量构造 Session"的快捷方式 → fixture 副作用（httptest.Server / TCP listen / wire IO）远超测试目的。

更深的思维漏洞：`ws_test`（外部测试包）不能访问 unexported 字段，所以"批量裸构造 Session"看起来需要走公开 API，第一反应是复用现成 helper。但 `internal_test.go`（内部测试包）可以直接 `&Session{userID: ..., sendChan: make(...)}` 构造 fixture —— 已有 `session_manager_internal_test.go` 用这个模式跑 1000 session 注册测试。"测试代码必须走公开 API"是误解，对**纯白盒性能测试**来说内部测试包是更合适的位置。

### 修复（Fix）

1. 删除 `ws_test.go` 里的 `TestBroadcastToRoom_R1_LargeN_SyncFanoutFastEnough`（保留迁移轨迹注释）
2. 新增 `broadcast_perf_internal_test.go`（package `ws`，内部测试包）：
   - `makeStubSessionForBroadcast(userID, roomID)` helper：返回裸 `&Session{userID, roomID, sendChan: make(chan []byte, sendChanCapacity), logger: io.Discard}`，**不**绑 conn / 不启 writeLoop / 不走握手
   - `TestBroadcastToRoom_R2_LargeN_SyncFanoutFastEnough_StubSessions`：N=100 stub session 注册到 manager，调一次 BroadcastToRoom，断言 sent==N + elapsed<100ms + 每个 sendChan 内有 1 条消息
   - **顺带新增** `TestBroadcastToRoom_R2_ConcurrentBroadcasts_SameRoomGlobalOrder`（Lesson 1 的并发回归测试）：M=4 session × G=4 goroutine × K=5 broadcast，drain 所有 sendChan，断言所有 session 看到相同的消息序列
3. 实测：N=100 stub session 调一次 BroadcastToRoom，elapsed = ~5ms（未带 -race）vs 旧实装大概率 60ms+ 的 httptest 启动开销

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写大 N（≥50）性能 / 并发 fixture 的测试** 时，**禁止**用 httptest.Server / 真实 wire / 真实 conn 批量构造 fixture，**必须**改用 internal test package 的裸结构体 + 必要 chan / atomic 字段。
>
> **展开**：
> - 性能测试断言（latency / throughput）的"信号"必须来自被测路径本身；fixture 启动开销混进 elapsed 就是污染信号
> - "测试代码必须走公开 API"对集成测试 / E2E 成立，对**白盒性能测试**不成立。internal test package（`package ws` 而非 `package ws_test`）可以直接构造裸结构体绕过公开构造函数 —— 已有 `session_manager_internal_test.go` 用这个模式
> - 裸 fixture 必须**只**初始化被测路径会触达的字段（如 BroadcastToRoom 路径只触 sendChan / closed / userID / roomID / logger）；其他字段（conn / writeLoopDone / cancelCtx）保持 zero-value。这种"最小 fixture"让测试意图更清晰，也不会因为别的路径副作用污染断言
> - 集成层面的真实 wire 路径仍由现有 single-session 测试覆盖（`TestSession_Send_HappyPath` 等），白盒性能测试**不**重复覆盖
> - **反例**：r1 提交的 `TestBroadcastToRoom_R1_LargeN_SyncFanoutFastEnough` 注释明明说"避免 httptest server"，实装却调 useGatewayDial 100 次启 100 个 httptest.Server。注释和实装彻底脱钩，是没看清 helper 内部到底做什么就直接复用的典型表现。**任何 helper 在循环里被调 ≥10 次**，都必须先回头读 helper 实装，确认它的副作用是 O(1) 还是 O(每次新起 server)

---

## Meta: 本次 review 的宏观教训

r2 的两条 finding 指向同一个思维模式：**只考虑"happy path 一次"，不考虑"happy path × N 并发 / 重复"**。

- Lesson 1：单 goroutine 路径正确 → 没主动追问"多 goroutine 同时调还正确吗"
- Lesson 2：单 session 测试用 useGatewayDial 没问题 → 没主动追问"循环里跑 100 次副作用还可控吗"

这是同一个 anti-pattern。未来设计任何 primitive / 写任何测试 fixture，**默认提问**：

1. 这条路径在**多 caller 并发**下还正确吗？（Primitive 层面的"原子性"必须显式互斥保证）
2. 这条路径在**循环里被调 N 次**时副作用是否仍 O(1)？（Test fixture 层面的"启动开销 vs 测试目标"）

把这两个问题加进 design review 默认问号清单，能拦住至少 60% 的"看起来对实际有 race / 看起来快实际是测启动开销"类 review finding。
