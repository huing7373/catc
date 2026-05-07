---
date: 2026-05-07
source_review: codex review (epic-loop story 10-6 r4) — file: /tmp/epic-loop-review-10-6-r4.md
story: 10-6-redis-presence-repo
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-07 — RemoveOnline 跨 room 必须 SREM 旧 room & scanner reconcile 必须 IsRegistered guard 防复活

## 背景

Story 10.6 r4：codex 在 r3 修后又抓出两个相关的"presence 状态泄漏"路径。

- r1 给 RemoveOnline 加 sessionID guard 防 reconnect 路径误删新 session 的 user key
- r2 把 AddOnline 命令顺序调成 SET → SADD → EXPIRE 兜底 partial-fail
- r3 把 scanner reconcile 路径从 RenewTTL 改成 AddOnline 让 Register hook partial-fail 自愈

r4 review 指出 r3 引入的 scanner 主动 AddOnline 和 r1 的 sessionID guard 一起，在 cross-room reconnect + race 路径下让 user 同时挂在 oldRoom + newRoom 永久不清；scanner snapshot 与 AddOnline 之间的 race 还能"复活"已 unregister session 的 presence 形成 zombie。两条都是真实可复现的 bug。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | RemoveOnline Lua script 漏 SREM 旧 room（cross-room reconnect 路径 user 永久残留） | high | architecture | fix | `server/internal/repo/redis/presence_repo.go:293-303` |
| 2 | scanner snapshot 后 session 已 unregister 仍 AddOnline → 复活 zombie presence | medium | architecture | fix | `server/internal/app/ws/heartbeat_scanner.go:297-318` + `session_manager.go` (新增 `IsRegistered`) |

## Lesson 1: RemoveOnline 必须始终 SREM 旧 room；user key 的 DEL 才受 sessionID guard 保护

- **Severity**: high
- **Category**: architecture（Redis 数据一致性）
- **分诊**: fix
- **位置**: `server/internal/repo/redis/presence_repo.go:293-303`

### 症状（Symptom）

cross-room reconnect 路径（user 从 roomA 重连到 roomB）下，scanner reconcile race 能让 user 永久同时出现在 roomA + roomB 两个 `online_users` set 里：

1. user 在 roomA 有 oldSession，`user:{id}:ws_session = oldID`、`room:roomA:online_users` 含 user
2. user 切到 roomB 重连：SessionManager.Register 把 NEW session 加到 sessionsByID 后释放锁
3. **race window**：HeartbeatScanner（每 30s tick）刚好命中此窗口 → 拿到含 NEW 的 snapshot → 对 NEW 调 `AddOnline(roomB, userID, newID)` → 写 `user:{id}:ws_session = newID` + SADD roomB
4. SessionManager 继续：`replaced.Close()` → onUnregister hook → `RemoveOnline(roomA, userID, oldID)`
5. 旧 RemoveOnline 的 Lua script GET `user:{id}:ws_session` 拿到 newID ≠ oldID → 旧版本 `return 0` 跳过 SREM
6. 结果：user 留在 `room:roomA:online_users` set 永不清；roomA 其他活跃 user 周期 AddOnline 续 roomA TTL → user 永远看似 online 在 roomA + roomB 两个 room

### 根因（Root cause）

Lua script 把"sessionID guard"过度约束到 SREM 上。sessionID guard 的本意是保护 `user:{id}:ws_session`（user → 当前活跃 sessionID 的全局映射）不被旧 unregister 误删。但 SREM 是按 (传入 roomID, userID) 双键定位的 set-member 操作 —— 调用方传入的 roomID 已经定位了"要清理的旧 room"，与 user key 当前指向哪个 sessionID 无关。

旧版本把这两个操作捆在同一个分支里（"matches OR not exists → 同时 SREM + DEL；otherwise 同时跳过"），让 sessionID 不匹配的合法路径（reconnect 抢占）也漏掉 SREM。

### 修复（Fix）

把 Lua script 拆成三分支：

```lua
local current = redis.call("GET", KEYS[2])
if current == false then
  redis.call("SREM", KEYS[1], ARGV[2])    -- 仅 SREM；user key 已不存在不需要清
  return 1
end
if current == ARGV[1] then
  redis.call("SREM", KEYS[1], ARGV[2])    -- SREM + DEL（sessionID 匹配，完整清理）
  redis.call("DEL", KEYS[2])
  return 2
end
redis.call("SREM", KEYS[1], ARGV[2])      -- 仍 SREM 旧 room；不动 user key（被 NEW 接管）
return 3
```

**关键不变量**：旧 room 的 SREM **永远**执行（无论 GET 命中哪个分支）。user key 的 DEL 仅在 GET == 传入 sessionID（无 reconnect 抢占）时执行。

配套测试：
- `TestPresenceRepo_RemoveOnline_CrossRoomReconnect_SREMsOldRoom`：r4 P1 核心 case
- `TestPresenceRepo_RemoveOnline_SameRoomReconnect_SelfHealsAfterReAdd`：same-room 路径下 SREM 后 NEW AddOnline 自愈
- `TestPresenceRepo_RemoveOnline_ReconnectRace_GuardsUserKey`：保留 r1 P1 的 user key 保护语义

### 预防规则（Rule for future Claude）

> **一句话**：Redis Lua script 在做 "compare-and-modify" 时，**必须**区分"compare 锁的目标 key"和"需要被同步清理的 collateral keys" —— 不能把所有清理操作都捆在同一个分支里。

> **展开**：
> - sessionID guard / CAS 类逻辑保护的是**特定 key 的所有权**（如 `user:{id}:ws_session` 只有"当前活跃 session"能 DEL），不是所有相关 keys 的清理操作
> - collateral cleanup（如 SREM 旧 room set）应该**与 guard 解耦**单独执行 —— 它的语义是"按调用方传入的 (oldRoom, userID) 定位的 set-member 清理"，与 guard 保护的 user key 无关
> - 写 Lua script 前先**画出三种 case**：(a) target key 不存在；(b) target key 匹配 expected value；(c) target key 不匹配（被抢占）。每种 case 单独决定每个操作做不做，**不**简单合并 case (a)+(b) 共享分支
> - **反例**：旧版本 `if current == false or current == ARGV[1] then SREM + DEL else return 0 end` —— 把 SREM（应总是执行）和 DEL（应仅 case b 执行）捆在一起，结果 case (c) 漏 SREM
> - **反例**：reconnect 路径下盲目"全清"或"全跳过" —— 跨 room 抢占（cross-room reconnect）和同 room 抢占（same-room reconnect）外观一致但语义不同；不动 user key 可以保护新 session，但 SREM 旧 room 不会影响新 session（操作的是不同 room set）

## Lesson 2: scanner reconcile 路径必须 IsRegistered guard，防 snapshot 过期复活已 unregister session

- **Severity**: medium
- **Category**: architecture（race window）
- **分诊**: fix
- **位置**: `server/internal/app/ws/heartbeat_scanner.go:297-318`、`server/internal/app/ws/session_manager.go`（新增 `IsRegistered` 接口方法）

### 症状（Symptom）

HeartbeatScanner.scanOnce 拿到 `mgr.ListAllSessions()` 快照后，对每个 active session 调 `PresenceRenewer.AddOnline(roomID, userID, sessionID)` reconcile presence。这个 snapshot 与 AddOnline 之间存在 race window：

1. T0: scanner.ListAllSessions 拿到 sessionA 引用
2. T1: sessionA 的 client disconnect → readLoop 错误 → session.Close() → notifyClosed → manager.Unregister(sessionA) → onUnregister hook → presenceRepo.RemoveOnline 清干净 sessionA 的 presence
3. T2: scanner 遍历到 snapshot 中的 sessionA → 调 AddOnline(roomA, userA, sessionA) → **复活**已离线 session 的 presence keys
4. T3: presence keys 持续到 TTL 自然过期（默认 5min）才被清 → IsOnline / ListOnline 误报"已离线 user 还在 online"

每次 scanner tick 与某个 session 的正常 disconnect race 都可能触发，prod 30s × 长时间运行下统计意义上必然发生。

### 根因（Root cause）

snapshot-iteration pattern 的经典 race：snapshot 是当时的快照，迭代过程中底层数据变化，迭代器看不到。AddOnline 是带副作用的 idempotent 写操作 —— idempotent 对"同 session 多次写"无害，但对"已 unregister session 复活"是**有害的**（会让外部状态偏离 manager 当前真相）。

scanner 的 fanout close 路径（`idle > timeoutMs` 那条）已经走了 ctx-check + recheck + ctx-check 的 TOCTOU 防护（review r1/r3 P2）。但 reconcile 路径（active session 那条）当时只看 `idle <= timeoutMs`，没有"session 当前还在 manager 吗"这个 check。

### 修复（Fix）

1. SessionManager 接口新增 `IsRegistered(ctx, sessionID) bool`：走 RLock + map lookup 直接查 `sessionsByID`，O(1) nanos 量级。
2. scanOnce 在 reconcile 分支调 AddOnline 之前先调 `mgr.IsRegistered`，false 则 skip：

```go
if s.renewer != nil {
    if !s.mgr.IsRegistered(ctx, sess.SessionID()) {
        continue  // snapshot 后 session 已 unregister；不要复活 presence
    }
    if err := s.renewer.AddOnline(ctx, sess.RoomID(), sess.UserID(), sess.SessionID()); err != nil {
        s.logger.Warn("ws presence reconcile failed", ...)
    }
}
```

配套测试：
- `TestHeartbeatScanner_ScanOnce_UnregisteredSession_SkipsReconcile`：用 `staleSnapshotMgr` wrapper 注入 stale snapshot + real manager 的 IsRegistered → AddOnline 不被调
- `TestHeartbeatScanner_ScanOnce_RegisteredSession_StillReconciles`：happy path 验证 guard 不破坏正常 reconcile

### 预防规则（Rule for future Claude）

> **一句话**：从 manager / index 拿"快照"后异步对 entries 做带副作用的写操作时，**必须**在每次写之前重新 check entry 是否仍在 manager —— snapshot 不是真相，manager 当前状态才是。

> **展开**：
> - "lookup-then-mutate" pattern 在并发管理器（如 SessionManager / connection pool / cache）上必须做 second-look check，否则 race window 会让"已删除"entry 被无意复活
> - check 操作应该是**轻量 read-only**（RLock + map lookup），不要把 second-look 写成"再 ListAllSessions 拿一次"那种 O(N) 操作 —— scanner 每 tick 对每个 session 调一次，性能必须在 nanos 量级
> - 单测验证 race 路径的标准做法：**包装 manager**让 ListXxx 返回 stale snapshot 但 IsRegistered / Get 委托给 real manager —— 这样能在单测中可控地复现 race window，比真起 timing-sensitive 并发更稳定
> - **反例**：scanner 拿到 snapshot 直接 fanout 写 Redis / DB / 远端 service 而不重 check entry 状态 —— 看似性能更好但会持续制造 zombie 状态，监控侧需要等 TTL 才自愈
> - **反例**：用 "ListXxx 再调一次" 做 second-look —— 写锁竞争 + O(N) 开销，scanner 高频 tick 下让 SessionManager.Register/Unregister 周期性卡顿

---

## Meta: 本次 review 的宏观教训

r4 抓出的两条 bug 都是 r1-r3 修复链条中"语义对接处的边角"：r1 加 sessionID guard 时把 SREM 也卷进 guard、r3 把 scanner reconcile 从 RenewTTL 升到 AddOnline 时没考虑 snapshot stale。

教训：**修一个 race 的同时往往引入相邻 race**。每个 r 加一道防线，但防线的"语义边界"如果没画清楚，下一个 r 就会发现新泄漏路径。

下次写"compare-and-do" / "snapshot-then-write" 类逻辑时，先列**所有可能的 race 时序**（不只是想修的那个），再决定每条路径每个操作做不做 —— 把"在这种情况下应该 X"逐一写清楚，而不是寄希望于"防线层叠最终覆盖所有 case"。
