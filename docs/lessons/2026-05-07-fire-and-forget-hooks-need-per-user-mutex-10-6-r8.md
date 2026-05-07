---
date: 2026-05-07
source_review: codex review (epic-loop r8) — /tmp/epic-loop-review-10-6-r8.md
story: 10-6-redis-presence-repo
commit: <pending>
lesson_count: 3
---

# Review Lessons — 2026-05-07 — fire-and-forget hooks 同 user 串行化 + scanner reconcile guard 升级 + AddOnline SADD/EXPIRE 原子化（Story 10.6 r8）

## 背景

Story 10.6 "Redis presence repo + WS lifecycle 钩子接入"经过 r1–r7 七轮 review fix
后仍存在三类 race / partial-fail 漏洞，r8 codex review 指出：

1. **r6 引入的 fire-and-forget hook**（不阻塞 SessionManager.Register/Unregister 主路径）
   让同 userID 的 AddOnline / RemoveOnline 两个 goroutine 调度顺序**不定** —— quick
   connect-then-close 或 reconnect 替换路径下 RemoveOnline 可能先跑完，AddOnline 后跑
   把 presence "复活" 已离线 session，IsOnline / ListOnline over-report 直到 TTL 过期。
2. **r4 P2 引入的 IsRegistered guard**（scanner reconcile 前）太弱 —— Register 替换路径
   有意保留 OLD session 在 sessionsByID 直到 oldS.Close() 跑完，scanner 在这个窗口看到
   IsRegistered=true 仍 AddOnline OLD，把 user_key 改回 OLD session/room → 后续
   RemoveOnline(oldSessionID) 在 Lua script 看到 currentSession==OLD 走 case 2 完整清理
   → 真正活的 NEW 的 presence 被清掉。
3. **r2 引入的 SET → SADD → EXPIRE 顺序**修了"SADD 后 SET 失败永久 zombie"路径，
   但 SADD 成功 + EXPIRE 失败仍让 room set 写入 member 但**无 TTL** → process crash
   后 user_key 因 SET KEY VAL EX 自带 TTL 5min 过期，room set 上 member 永久残留，
   IsOnline / ListOnline 永久 over-report 该 (room, user)。

三条都是真问题，全 fix。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | fire-and-forget hooks 同 userID Add/Remove 顺序不定 → presence 复活 | P1 | architecture | fix | `server/cmd/server/main.go:282-355` |
| 2 | scanner reconcile 用 IsRegistered 太弱，reconnect 替换中场污染 NEW presence | P1 | architecture | fix | `server/internal/app/ws/heartbeat_scanner.go:368-378` + `session_manager.go` |
| 3 | AddOnline SADD+EXPIRE 非原子，EXPIRE 失败让 room set 永久无 TTL | P2 | error-handling | fix | `server/internal/repo/redis/presence_repo.go:273-310` |

## Lesson 1: fire-and-forget hooks 同 userID Add/Remove 必须串行化

- **Severity**: P1
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/cmd/server/main.go:282-355`

### 症状（Symptom）

r6 把 SessionManager 的 Register / Unregister 钩子内部 Redis I/O 改成
fire-and-forget goroutine，不阻塞主路径（避免 Redis 慢时 connect / shutdown
visible 延迟）。但同 userID 的 AddOnline 与 RemoveOnline 两个独立 goroutine
调度顺序不定：

- **Quick connect-then-close**：客户端 WS 握手成功后 < 1ms 主动断开 →
  Register goroutine A 启动 (AddOnline) + Close 触发 Unregister goroutine B 启动
  (RemoveOnline)。如果 B 先跑完 → A 跑 AddOnline → presence 复活已离线 session
  的 keys，IsOnline / ListOnline 在 TTL 5min 内 over-report。
- **Reconnect 替换**：OLD session 与 NEW session 同 userID。
  AddOnline(NEW) 与 RemoveOnline(OLD) 两个独立 goroutine 顺序不定。

### 根因（Root cause）

r6 的 fire-and-forget 设计**忘了**：goroutine 调度顺序与触发顺序无关。
Hook 触发顺序在 SessionManager 端是确定的（Register 先于 Unregister，或
reconnect 替换路径下 onRegister(NEW) 先于 oldS.Close → onUnregister(OLD)），
但**一旦 dispatch 到独立 goroutine，谁先跑取决于 Go runtime 调度** —— 没有
强制序的话，Redis 端命令到达顺序与触发顺序解耦，破坏"先 Add 后 Remove"
不变量。

### 修复（Fix）

引入 `userKeyedMutex` 工具类型（main.go 顶部，包级；带完整 godoc）：
`sync.Map[uint64]*sync.Mutex`，同 userID 的 hook goroutine 抢同一把锁。

- 走 r3 P2 lesson 学到的 "Load fast path"：先 `m.Load` 单次原子命中，miss 才走
  `LoadOrStore`（避免每次 hook 都分配一个 *sync.Mutex 然后又被 GC）。
- 不同 userID 的 mutex 互不阻塞 → N user 的 hook 仍能并行；只有同 user 的
  Add/Remove 串行（这正是不变量需要的最小串行域）。
- 锁拿取在 ctx.WithTimeout 之前，否则 timeout 期已经从锁队列里出去了再做
  Redis I/O 没意义。

```go
type userKeyedMutex struct { m sync.Map /* map[uint64]*sync.Mutex */ }

func (u *userKeyedMutex) lockFor(userID uint64) *sync.Mutex {
    if muVal, ok := u.m.Load(userID); ok {
        return muVal.(*sync.Mutex)  // fast path
    }
    muVal, _ := u.m.LoadOrStore(userID, &sync.Mutex{})
    return muVal.(*sync.Mutex)
}

// Register / Unregister hook 同模式：
go func() {
    defer presenceHooksWG.Done()
    mu := userPresenceMu.lockFor(s.UserID())
    mu.Lock(); defer mu.Unlock()
    hookCtx, cancel := context.WithTimeout(...); defer cancel()
    presenceRepo.AddOnline(...)  // 同 userID 串行
}()
```

加测试 `server/cmd/server/main_internal_test.go`：
- `TestUserKeyedMutex_SameUserSerializes` —— 100 goroutine 并发 lockFor 同
  userID，临界区内观察 inFlight 计数器，断言任何时刻 ≤ 1（串行不变量）。
- `TestUserKeyedMutex_DifferentUsersDoNotBlock` —— 锁 user A 时锁 user B 不
  阻塞（不同 userID 互不串行 → 性能不退化到全局串行）。
- `TestUserKeyedMutex_LockFor_ReturnsSamePointerForSameUser` —— 多次 lockFor
  同 userID 返回同一 *Mutex（sync.Map LoadOrStore 关键不变量）。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 写 fire-and-forget goroutine 处理某 key 的 Add/Remove
> 这种**必须保持触发顺序**的 lifecycle 操作时，**必须**用 per-key 串行化（如
> sync.Map[key]*sync.Mutex 或 per-key channel queue）。

> **展开**：
> - "我把 hook 改 fire-and-forget 了 —— 主路径变快了"是只看了一半。另一半是
>   "goroutine 调度顺序与触发顺序解耦"—— 必须问"这两个 goroutine 之间是否有
>   happens-before 不变量需要保留？"
> - 全局 mutex 太粗（让 N user 的 hook 退化到 O(N × Redis latency) 串行 →
>   shutdown 慢回原样）；per-key mutex 是黄金中道：同 key 串行，不同 key 并行。
> - sync.Map 的 entry 不会自动回收 —— 长生命周期 server 累积 O(active keys) 内存。
>   单实例 MVP < 万级 user 可接受；如果 key 集合可能爆炸（如 sessionID 而非
>   userID）需要加 LRU evict。
> - `lockFor` 返回 *Mutex 的地址必须稳定 —— sync.Map LoadOrStore 保证首次 store
>   后续 Load 命中同地址；不要换成"每次新建一把 Mutex 然后塞进去再返回"，那样
>   不同 goroutine 抢的是不同实例，根本没串行。
> - **反例**：`go func() { presenceRepo.AddOnline(...) }()` 同时在 Register hook 和
>   Unregister hook 里出现，没有 per-key serialization → 同 user 顺序不定 →
>   presence 复活 zombie。

## Lesson 2: scanner reconcile 必须用 IsCurrentForUser 而非 IsRegistered 做 gate

- **Severity**: P1
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/app/ws/heartbeat_scanner.go:368-378` + `session_manager.go`（新增 `IsCurrentForUser` 方法）

### 症状（Symptom）

r4 P2 加了 `IsRegistered` guard 防 scanner 在 snapshot 与 dispatch 之间 session
已 Unregister 时仍 AddOnline 复活 zombie。但 reconnect 替换路径**有意**保留 OLD
session 在 sessionsByID 直到 oldS.Close() → notifyClosed → Unregister 跑完才被清
（r2 P1 不变量：让 OLD 的 onUnregister 钩子触发标准路径，不漏 presence cleanup）。

scanner sweep 在这个**替换中场窗口**看到 OLD session 仍在 → IsRegistered=true →
AddOnline → 把 `user:{id}:ws_session` 改回 OLD session/room → 后续
RemoveOnline(oldSessionID) 在 Lua script 看到 currentSession==OLD 走 case 2 完整清理
→ 真正活的 NEW session 的 presence 被清掉。

### 根因（Root cause）

`IsRegistered` 的语义是"这个 sessionID 还在 manager 索引里 = 会话还活着"，是个
**弱**的 liveness check。但 scanner reconcile 路径需要的是更**强**的语义：
"这个 sessionID 是该 user 当前 active 的 session"—— reconnect 替换中场 OLD 还
活着但不是 current；scanner 对 OLD reconcile 会污染 NEW 的 presence。

> 弱 guard 在"普通 disconnect"路径已经够用，但 reconnect 替换路径的"中场窗口"
> 把 OLD 与 NEW 同时塞在 sessionsByID（双索引故意分离生命周期），这时弱 guard
> 让 OLD 通过，污染 NEW。

### 修复（Fix）

1. SessionManager 接口新增 `IsCurrentForUser(ctx, sessionID) bool`：
   严格匹配双索引一致 —— 只有 `sessionsByID[id] != nil` **且**
   `userToSessionID[user] == id` 时才返 true。reconnect 替换中场 OLD 返 false
   （userToSessionID 已指 NEW），普通 disconnect 后 OLD 也返 false（sessionsByID
   已删）—— 在两类 race 路径下都正确。
2. heartbeat scanner 的 reconcile 路径把 `IsRegistered` 换成 `IsCurrentForUser`。

```go
// session_manager.go 新增
func (m *sessionManager) IsCurrentForUser(_ context.Context, sessionID string) bool {
    m.mu.RLock(); defer m.mu.RUnlock()
    sess, ok := m.sessionsByID[sessionID]
    if !ok { return false }
    currentID, ok := m.userToSessionID[sess.userID]
    return ok && currentID == sessionID
}

// heartbeat_scanner.go reconcile fanout 入口
if !s.mgr.IsCurrentForUser(ctx, target.SessionID()) { return }
// （之前是 IsRegistered → 改成 IsCurrentForUser）
```

`IsRegistered` 接口**保留** —— 作为更弱的 "会话还活着" check 给其他场景用
（broadcast 路径、未来 graceful shutdown 等）。godoc 加 explicit 警告"不要用本
方法做 reconcile 路径的 gate，必须用 IsCurrentForUser"。

加测试：
- `TestSessionManager_IsCurrentForUser_OldSessionAfterReplacement_ReturnsFalse`
  （session_manager_internal_test.go）—— 手动构造 reconnect 替换中场态（OLD 仍在
  sessionsByID + userToSessionID 已指 NEW），断言 IsRegistered(OLD)=true 但
  IsCurrentForUser(OLD)=false（这是 r8 P1 修法的核心区分点）；NEW 仍返 true。
- `TestSessionManager_IsCurrentForUser_RegisteredSession_ReturnsTrue` /
  `_AfterUnregister_ReturnsFalse` / `_UnknownSession_ReturnsFalse` —— 普通路径
  basis tests。
- `TestHeartbeatScanner_ScanOnce_NotCurrentForUser_SkipsReconcile`（ws_test.go）
  —— 用 `notCurrentMgr` 包装 mock IsRegistered=true + IsCurrentForUser=false，
  断言 scanner 跳过 reconcile（renewer.count == 0）。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在写 reconcile / 续期 / "对 active 资源做某操作"路径
> 时，guard 必须区分**弱 liveness**（资源还在 = "活着"）vs **强 currentness**
> （资源是当前实例 = "正主"）—— 替换 / 抢占场景下两者会分离，用错会污染正主。

> **展开**：
> - 当系统支持 reconnect 替换 / failover / 抢占语义（这些场景下 OLD 与 NEW 同时
>   存在一段窗口），任何对 active 资源的 mutating 操作（如续期、reconcile、
>   续 TTL）都必须用强 gate。仅查询的操作可以用弱 gate（看到 OLD 也只是返回多
>   一条数据，不破坏状态）。
> - 写新 manager 类时，给 "is this still active?" check 想清楚两种语义：
>   - `IsRegistered` / `IsAlive` / `Exists` → 弱（索引命中）
>   - `IsCurrent` / `IsAuthoritative` / `IsLeader` → 强（双索引一致 / leader epoch 命中）
>   并在 godoc 里**显式标注** "不要把弱 gate 用在 mutating 路径"，避免后人写出
>   r4 P2 这种"看似够用，replacement window 漏掉"的代码。
> - 改 guard 时一定加测试覆盖**两种 race 路径**：(a) 普通 disconnect 后 reconcile
>   （应跳过）；(b) reconnect 替换中场 reconcile OLD（应跳过 NEW 不被污染）。
>   只覆盖 (a) 不能发现 reconnect 路径的 bug。
> - **反例**：reconcile / 续期 / refresh 路径写 `if !mgr.IsRegistered(id) { return }`
>   而不区分"this session 是否仍是 user 的当前 session"—— reconnect 替换中场让
>   OLD 通过 guard，污染 NEW。

## Lesson 3: AddOnline 的 SADD + EXPIRE 必须 Lua 原子化，不能用两条独立命令

- **Severity**: P2
- **Category**: error-handling
- **分诊**: fix
- **位置**: `server/internal/repo/redis/presence_repo.go:273-310` + `addRoomMemberLuaScript`

### 症状（Symptom）

r2 P2 把 AddOnline 命令顺序从 `SADD → SET → EXPIRE` 改成 `SET → SADD → EXPIRE`，
修了"SADD 成功 + SET 失败 → return → room set 永远没 EXPIRE"那条永久 zombie
路径。但 SADD 成功 + EXPIRE 失败仍是漏洞：

- room set 写入 member 但**无 TTL** → process crash 后 user_key 因 SET KEY VAL EX
  自带 TTL 在 5min 后过期；room set 上 member 永久残留（无 TTL 兜底；下次
  RemoveOnline 走 Lua script GET user_key 已不存在 → 走 case 1 仅 SREM，但如果 user
  一直没 reconnect 也没 unregister 就**没人触发 SREM**）。
- 后果：IsOnline / ListOnline 永久 over-report 该 (room, user)。违反"presence 必须
  最终一致"的设计意图（V1 §12 钦定）。

### 根因（Root cause）

Redis 多命令间没事务保证（pipeline 也不保证 partial 失败的回滚）。`SADD member;
EXPIRE key ttl` 是逻辑上一个 "membership + TTL" 原子操作，但物理上是两条命令 —
任一在 server / network 中断都让两个 effect 分裂。修法只能让 Redis **server 端**
原子化，client 端无法保证。

### 修复（Fix）

新增 `addRoomMemberLuaScript`：
```lua
redis.call("SADD", KEYS[1], ARGV[1])
redis.call("EXPIRE", KEYS[1], ARGV[2])
return 1
```
AddOnline 用 EVAL 跑这段：
```go
ttlSeconds := int64(r.ttl / time.Second)
if _, err := r.client.Eval(ctx, addRoomMemberLuaScript, []string{rk}, uidStr, ttlSeconds); err != nil {
    return fmt.Errorf("presence add online sadd+expire: %w", err)
}
```
SET 仍是单条 `SET KEY VAL EX`（已经原子，不需要包进 Lua）。

修后 partial-fail 矩阵：
- SET 失败 → return → Lua 段没执行 → 不留任何残留
- SET 成功 + Lua 段失败 → user_key 已写入有 TTL（Set KEY VAL EX 原子）；room set
  不变（要么 SADD+EXPIRE 都成功要么都没执行）。下次 AddOnline 同 user 重试自愈。
  process crash 时 user_key TTL 5min 过期，room set 也不变（无残留）。
- SET 成功 + Lua 段成功 → 完整一致状态。

加测试：
- `TestPresenceRepo_AddOnline_SAddExpireLuaFails_NoLeftover` —— Eval fault
  injection 失败，断言 user_key 已 SET 但 room set **不含 member**（r8 P2 相对
  r2 命令分离方案的核心改进）；下次 AddOnline 重试自愈。
- `TestPresenceRepo_AddOnline_HappyPath_RoomSetHasTTL` —— 正向覆盖：走完整
  AddOnline 后 FastForward 超过 TTL，room set 确实过期（验证 Lua 段 EXPIRE 已写入）。
- 删除 r2 P2 时代的两个测试（`_SAddFails_*` / `_ExpireFails_*`），它们假设
  SADD/Expire 走 client.SAdd/Expire 调用 —— Lua 化后这两个命令在 Redis 内部跑，
  client wrapper 拦不到。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 Redis 写多条命令组成一个**逻辑原子操作**时（如
> "SADD + EXPIRE"、"GET + SET-if-equal"、"INCR + RENAME"），必须用 EVAL Lua
> script 跑在 server 端原子；不能依赖 pipeline 或 "命令顺序 + partial fail 兜底"。

> **展开**：
> - Pipeline 是网络优化（少 RTT），**不**是事务 —— pipeline 里某条命令失败不会
>   回滚前面的；Redis MULTI/EXEC 是 client-side 队列，server 端实际仍是顺序执行
>   各命令，单个命令失败也不回滚（与 SQL 事务语义不同）。真正的"server 端原子
>   多命令"只有 EVAL Lua（Redis single-thread 串行执行整段）。
> - "命令顺序 + partial fail 容忍"是个**会反复掉坑**的设计 —— 即便排列出"看起来
>   最干净"的顺序（如 r2 把 SET 提前），仍有某条 partial fail 路径让某个 key 没
>   配 TTL / 多 key 状态分裂。这种设计要求枚举所有 fail 排列且证明每条都"可接受"，
>   一旦遗漏一条（如 r8 P2 的 SADD 成功 + EXPIRE 失败 + process crash 路径）就
>   留下永久 zombie。
> - 改成 Lua 后，partial-fail 矩阵从 N×N 缩到 2×（成功 / 失败）—— 测试覆盖更简单。
> - 写 Lua script 后**显式记录 ARGV / KEYS 编码**到 godoc，让上层调用方传参时不
>   会错位（如 r7 把 user_key value 改成 "sessionID|roomID" 编码后，下游 Lua
>   script 解析必须配套改）。
> - **反例**：`r.client.SAdd(ctx, key, member); r.client.Expire(ctx, key, ttl)` —
>   两条命令顺序写在 Go 代码里，看似简洁但是 partial fail 不受控。改成
>   `r.client.Eval(ctx, "SADD KEYS[1] ARGV[1]; EXPIRE KEYS[1] ARGV[2]", ...)`
>   是结构性的修法，不留残留。

## Meta: r8 三条 finding 共同的宏观教训

r6 引入的两个"性能优化"（fire-and-forget hook + 命令顺序而非 Lua）和 r4 引入
的 "IsRegistered guard" 都是**只看了 80% 路径**的设计：

- fire-and-forget hook 看了"主路径快"，没看 "goroutine 调度顺序与触发顺序解耦"。
- SET → SADD → EXPIRE 顺序看了 "SADD 后 SET 失败"那条 zombie 路径，没看 "SADD
  成功 + EXPIRE 失败 + process crash" 那条永久残留路径。
- IsRegistered guard 看了 "snapshot 后 disconnect" 路径，没看 "reconnect 替换
  中场" 路径。

**共同的根因**：每次都没把"系统支持的所有 lifecycle 路径"列全 ——
（普通 disconnect / reconnect 替换 / quick connect-then-close / process crash /
network partial fail）。每条 lifecycle 路径都要单独走一遍 partial-fail 矩阵 +
race window 矩阵，才能拍板说"这个 guard / 顺序 / 锁够"。

> **未来规则**：在 review 一个"分布式 lifecycle"模块（presence / session
> management / leader election / rate limiting 等）的修法时，**先列出所有
> lifecycle 路径**（成对：触发器 × 系统状态），逐路径推演 happens-before 与
> partial fail —— 不要只跑 happy path + 一两个 race 就拍板。r1 → r8 共 8 轮
> review 暴露的就是"每轮只盖一条路径，下一轮发现还有路径未盖"。
