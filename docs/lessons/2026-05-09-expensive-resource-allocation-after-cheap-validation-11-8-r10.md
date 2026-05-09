---
date: 2026-05-09
source_review: codex review r10 输出（/tmp/epic-loop-review-11-8-r10.md）
story: 11-8-成员加入-离开-ws-广播
commit: d3080fc
lesson_count: 1
---

# Review Lessons — 2026-05-09 — 昂贵资源分配必须在 cheap validation 之后；attack vector 与 successful path 同 family 时必须分层处理（11-8 r10）

## 背景

Story 11.8 r6 引入 commit-time per-room mutex（`acquireCommitLock` + `roomCommitLocks sync.Map`）解决 r5 enqueue 顺序 race。r6/r7/r8/r9 累积之后，real roomID 的 lock entry "successful join 路径"留在 sync.Map 不回收 —— r8 已经把这条 leak 按"节点 4 阶段活跃 room 数有界 + attacker 受限于必须先获得 valid roomID"的判断决策为 defer / tech-debt。

codex review r10 提出 **1 条新 [P1]**：

- **[P1]** `JoinRoom` 在 `acquireCommitLock` **之前没有**做 room 存在性校验 —— 任何 JoinRoom 请求都会 LoadOrStore 一个 lock entry。即使 room 不存在（事务内 SELECT FOR UPDATE 找不到 → 6001），entry 仍永久留在 sync.Map。**attacker 可用任意 fake roomID 暴力 join → roomCommitLocks 无界增长 → memory leak（attacker-controlled）**。

与 r8 的 successful-path leak 同 family（都是"sync.Map entry 不回收"），但 r10 这一条把攻击面**扩大到 unauthenticated/non-existent roomID** —— attacker 不再需要先获得 valid roomID。r8 决策的"活跃 room 数有界"前提失效。

本次修复：在 `acquireCommitLock` 之前插入 cheap room-exists check（普通 SELECT，无 FOR UPDATE）。room 不存在 → 直接返 6001，**不进 lock 段**，**不污染 sync.Map**。性能代价：每 join 多 1 次 PRIMARY KEY 索引 SELECT（ms 级），节点 4 阶段可接受。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | JoinRoom 在 commit lock 之前缺 cheap room-exists check → attacker 可用 fake roomID 制造 sync.Map entry 泄漏 | P1 | security / perf | **fix** | `server/internal/service/room_service.go:803-823` (lock 段前新增 cheap check) |

## Lesson 1: 昂贵资源分配（lock entry / goroutine / map entry）必须在 cheap validation 之后；attack vector 与 successful-path leak 同 family 时分层处理

- **Severity**: P1 (security / DoS-flavored memory leak)
- **Category**: security / perf / architecture
- **分诊**: **fix**
- **位置**: `server/internal/service/room_service.go:803-823`（lock 段前新增 cheap check）+ `server/internal/service/room_service.go:486-525`（acquireCommitLock 注释加 LIFECYCLE-DEFER 分层说明 r8/r9/r10）

### 症状（Symptom）

JoinRoom 修复前的代码顺序：

```go
func (s *roomServiceImpl) JoinRoom(ctx context.Context, in JoinRoomInput) (*JoinRoomOutput, error) {
    // 1. 预检 user.current_room_id
    user, err := s.userRepo.FindByID(ctx, in.UserID)
    ...
    if user.CurrentRoomID != nil { return ErrUserAlreadyInRoom } // 6003

    // 2. **直接** acquire commit lock（**没有** room-exists check）
    mu := s.acquireCommitLock(in.RoomID) // ← LoadOrStore 必然制造 entry
    mu.Lock()
    defer mu.Unlock()

    // 3. 事务内 FindByIDForUpdate → ErrRoomNotFound → 6001 返回
    ...
}
```

attacker 任意构造 fake roomID（如 `9999999999`）反复调 JoinRoom：

- 每次 request HTTP 200 之外的 6001 响应（业务码层面），attacker 看到正常错误
- 但 server 端 `roomCommitLocks` sync.Map 每个 request 都 LoadOrStore 一个新 entry（key = fake roomID）
- entry 永远不回收 → memory leak 单调增长，**速率受限于 attacker 的 RPS（最多被 rate-limit 中间件限流，但 lock entry 已经创建）**
- 与 r8 决策 defer 的 "successful-join real roomID 累积" 不同：r8 假设"活跃 room 数有界"，r10 attack vector 让 attacker 控制 roomID 空间 → `2^64 - active_rooms` 个 fake roomID 都可被武器化

### 根因（Root cause）

**核心思维漏洞**：把"事务内 SELECT FOR UPDATE 校验 room 存在性"当成唯一防线 —— 忽略了**事务内校验之前**已经完成的"昂贵资源分配"步骤（lock entry 创建）。

事务内 SELECT FOR UPDATE 是**正确性保证**（race 兜底，rollback 时不留 DB inconsistency），但它**无法**回滚 sync.Map entry 创建 —— 那是 service 层在事务**之外**做的。

更深层根因：

1. **昂贵资源分配 vs cheap validation 的顺序原则缺失**。在加 lock / spawn goroutine / LoadOrStore map entry 之前，必须先做廉价的输入校验把"必然失败"的 request 过滤掉。否则 attacker 用 100% 失败的 input 也能制造资源消耗。

2. **r8 defer 决策的边界条件被误推广**。r8 决策的 leak 限定于"successful-join real roomID"路径，前提是"活跃 room 数有界 + attacker 必须有 valid roomID 才能武器化"。r10 这条 finding 打破了第二个前提：failed-join 路径同样制造 entry，且 attacker 不需要 valid roomID。**两条 finding 同 family 但风险等级不同，分层处理是必须的**，不能以"r8 已 defer"为由对 r10 也 defer。

3. **commit-time per-room mutex 设计文档的隐含假设没有被显式校验**。`acquireCommitLock` 的注释只说了"sync.Map LoadOrStore 模式 + 不主动清理"，没有把 caller 的责任（"调用本函数前必须确保 roomID 存在"）写出来。隐式契约被违反时没有 compile-time / test-time 检查。

### 修复（Fix）

#### 1. JoinRoom lock 段之前插入 cheap room-exists check

```go
// (1') **r10 [P1] 修复**：lock 之前先做 cheap room-exists check（普通 SELECT，
// 无 FOR UPDATE）—— 防止 attacker 用随机 / 不存在的 roomID 暴力 join 在
// roomCommitLocks sync.Map 内 LoadOrStore 出无穷多 entry 制造 memory leak
// （attacker-controlled）。
//
// **不变量**：本 check **best-effort**（lock 之前；race 内 room 可能被 close），
// 真正的正确性保证仍由事务内 SELECT FOR UPDATE 维护 —— 本 check 仅过滤掉
// 100% 不存在的 roomID，绝大多数 attacker 暴力 join 路径在此被丢弃，不再
// 污染 commit lock map。
//
// **与 r8/r9 worker leak 关系**：r8/r9 已 defer 的是"successful join 路径
// 留下 commit lock entry"（real roomID 累积，tech-debt 已记 lesson）；
// 本 r10 修复的是"failed join 路径（room 不存在）也留 entry"的 attack
// vector —— 二者同 family（map entry 不 reclaim），分层处理。
//
// **性能影响**：每 join 多 1 次普通 SELECT（ms 级；走 PRIMARY KEY 索引，无锁）
// —— 节点 4 阶段 join 频率低（人类操作级），可接受。
if _, err = s.roomRepo.FindByID(ctx, in.RoomID); err != nil {
    if stderrors.Is(err, mysql.ErrRoomNotFound) {
        return nil, apperror.New(apperror.ErrRoomNotFound, ...)
    }
    return nil, apperror.Wrap(err, apperror.ErrServiceBusy, ...)
}

mu := s.acquireCommitLock(in.RoomID) // 现在 caller 已保证 room 至少在最近 SELECT 时存在
mu.Lock()
defer mu.Unlock()
```

#### 2. acquireCommitLock 注释加 LIFECYCLE-DEFER 分层（r8 / r9 / r10 累积说明）

```go
// **LIFECYCLE-DEFER 分层**（review r8 / r9 / r10 累积结论）：
//
//	r8 / r9: successful-join 路径下 real roomID 的 lock entry 不回收 —— tech-debt
//	         已 defer，与 roomQueues / worker goroutine 同源，节点 4 阶段不构成
//	         attack vector（attacker 必须先获得 valid roomID 才能触发，速率受限）。
//	r10:     **failed-join 路径**（room 不存在）必须**不进**本函数 —— attacker
//	         可用任意 fake roomID 暴力调用 JoinRoom，若进 lock 段则每次都
//	         LoadOrStore 一个新 entry → memory leak 攻击面扩大到
//	         attacker-controlled。修法：JoinRoom 在调用本函数**之前**先做
//	         cheap room-exists check（普通 SELECT），不存在 → 直接 6001 返回，
//	         **不进**本函数，attack vector 关闭。详见 JoinRoom 注释。
//
// **关系**：r10 修复**不**解决 r8/r9 的 successful-join leak（仍是 tech-debt）；
// r10 仅关闭 unauthenticated/non-existent attack vector。两者同 family（map entry
// 不回收），分层应对。
```

#### 3. 测试更新（共 11 处 Join 路径 stub 增补 + 2 个 case 重写）

- `TestRoomService_JoinRoom_RoomNotFound_Returns6001`：模拟 cheap check 直接返 ErrRoomNotFound；断言 `findByIDCalls == 1` + `withTxCalled == false`（不进事务 / lock 段，attack vector 关闭）
- `TestRoomService_JoinRoom_CheapCheckRawDBError_Returns1009`（**新增**）：cheap check 返 raw DB error → 1009 + 不进事务
- 其他 9 个会过 precheck 进入 lock 段的 join 测试：在 `roomTestStubRoomRepo` 加 `findByIDFn` pass-through，让 cheap check 放行
- `TestRoomService_JoinRoom_Happy_5StepsExecute` 的 expected calls 数组首部多一个 `roomRepo.FindByID`

#### 4. LeaveRoom 不需要本修复

LeaveRoom 在 `acquireCommitLock` 之前已先做 user.CurrentRoomID 校验（不一致即 6004），attacker 没法在自己 user.CurrentRoomID == nil 的情况下触发 LeaveRoom 的 lock 创建。**只有 JoinRoom 是 attack vector**。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **公开 API 端点（HTTP / gRPC）涉及 service 层 acquire lock / spawn goroutine / LoadOrStore sync.Map 等"昂贵资源分配"操作** 时，**必须**在分配之前先做 cheap input validation（廉价 SELECT / cache 查询 / size 校验），把 100% 失败的 input 过滤掉，再决定是否进入资源分配段。
>
> **展开**：
>
> - **昂贵资源分配的判定标准**：分配的资源 (a) 不会随 request 失败自动回收（如 sync.Map entry 不回收、goroutine 启动后独立运行、lock entry LoadOrStore 后永驻）；(b) 单 request 占用虽小但累积可观（attacker 高 RPS 暴力调用即可放大）；(c) attacker 不需要持有合法资源（如 valid roomID）即可触发。三个条件任一成立都要做 cheap validation 前置。
>
> - **cheap validation 的位置**：必须在**资源分配点之前**（lock acquire / goroutine spawn / map LoadOrStore 之前），而不是分配之后再检查 —— 分配后检查的失败路径仍然产生不可回收的副作用。
>
> - **cheap validation 是 best-effort**：本身不是正确性保证（lock 之外的 race 仍可能让 room 在 lock 内被 close），正确性最终由事务 / lock 内的 strict check 维护。但它过滤了**绝大多数**确定性失败的 request → 攻击面降到"最坏情况一次 SELECT 的成本"。
>
> - **attack vector 与 successful-path leak 同 family 时必须分层处理**：如果某 family 的 leak（如 sync.Map entry 不回收）已在更早的 review 决策为 defer / tech-debt，新 review 提出"该 family 在 attacker-controlled 路径也成立"时，**不能**简单沿用 defer 决策。判断依据是触发条件是否扩大：(a) attacker 需要持有 valid resource → defer 可接受；(b) attacker 用任意 fake input 即可触发 → 必须 fix。
>
> - **注释代码层显式标记分层 defer**：当一个机制（如 sync.Map LoadOrStore 模式）的 lifecycle 在不同 review 阶段累积"半 defer 半 fix"决策时，必须在代码注释里把每个 review round 的判断（如 r8 defer 哪条 / r9 defer 哪条 / r10 fix 哪条）显式列出。否则下一个 review round 又会重提同一个 family 的问题（如 r9 重提 r8 defer 的 worker leak）—— 注释里有分层说明，下一个 review 才能识别 "这是 family X 的第 N 个变体，前 N-1 个已分别 defer/fix"。
>
> - **反例**：
>   - "事务内 SELECT FOR UPDATE 已经校验 room 存在性了，service 层不需要重复 check" —— 错。事务内校验**无法回滚**事务**之外**的资源分配（lock entry / goroutine / map entry）。
>   - "rate-limit 中间件已经限制 attacker RPS 了，sync.Map entry 累积速度有限，不算 attack vector" —— 错。limit 后的累积速度即使慢，永不回收的 leak 在长时间运行下仍是 unbounded growth。
>   - "r8 已经决策这条 leak family defer 了，r10 同 family 的新 finding 也直接 defer" —— 错。两个 finding 触发条件不同（successful-path real roomID vs failed-path fake roomID），attacker controllability 不同，必须分层判断。

## Meta: 本次 review 的宏观教训

11.8 review 已经走到 r10（review/fix 上限 10 轮的最后一轮）。从 r6 commit-time mutex 引入到 r10，每一轮 codex review 都对同一个 sync.Map 模式（`roomCommitLocks` / `roomQueues`）提出新的 leak 角度：

| Round | Finding | 决策 |
|---|---|---|
| r6 | 引入 commit-time per-room mutex 修 enqueue race | fix |
| r7 | unregister 移回 lock 段 | fix |
| r8 | sync.Map worker / lock entry 不回收（successful path） | **defer** |
| r9 | r8 升级为 P1 重复 flag + queue silent drop bug | fix silent drop / defer reaffirm + 加注释 |
| r10 | sync.Map lock entry 在 failed path 也不回收（attack vector） | **fix** |

宏观教训：**lifecycle 不主动清理的 sync.Map 模式有"死循环 review"风险** —— 每轮 review 总能从新角度提出新的 leak 变体。本质上 sync.Map LoadOrStore 没回收路径 = 永远存在 leak，差别只是触发条件严苛程度。最终方案应该是**在节点 4 之后引入 lifecycle 管理**（room close 时 reclaim entry，或 LRU eviction），不是继续在 cheap-check 层加防御。

但 r10 的 cheap-check 修复仍然有价值：**关闭 attacker-controlled 触发路径**这一最严重的子问题，剩下的 successful-path leak 真正受"活跃 room 数有界"约束。后续节点 4 的 lifecycle 重构是正确的长期方向，本 lesson 同时归档"为什么这个修复是局部最优而非全局最优"。
