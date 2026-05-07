---
date: 2026-05-07
source_review: codex review (epic-loop fix-review r10 — /tmp/epic-loop-review-10-6-r10.md)
story: 10-6-redis-presence-repo
commit: 29d21fe
lesson_count: 2
---

# Review Lessons — 2026-05-07 — AddOnline 自动清旧 room stale + Register 顺序倒装消除 reconnect 离线窗口（10-6 r10）

## 背景

Story 10-6（Redis presence repo）review r1-r9 一路演进完结构性翻案（r9：fire-and-forget
hook 改同步 + scanner reconcile 共享 user mutex），r10 codex 仍然指出两条独立的
正确性漏：

1. AddOnline 不会主动清旧 room 的 stale member —— RemoveOnline 一旦失败 / 漏跑，
   下一轮 AddOnline 重写 user_key 但永远不 SREM 旧 room；`ListOnline`/`IsOnline`
   会让 user 在 oldRoom + newRoom 同时 online，直到 oldRoom 整 set TTL 过期（仅当
   全部 user 都离开 + 无人续 TTL；任何活跃 user 续 TTL 就让 stale 永久存活）。

2. SessionManager.Register 替换路径下 hook 触发顺序是 "replaced.Close →
   onUnregister(OLD) → RemoveOnline(OLD) → onRegister(NEW) → AddOnline(NEW)"。
   中间窗口里 user_key 已被 RemoveOnline DEL，IsOnline / ListOnline 看到 user
   暂时离线（生产路径 Redis brownout 期可达几百毫秒）—— 违反 V1 §12 "presence
   是查询时态" 的连续性语义。

两条都是 r9 之前没注意到的"自我修复 / 调用顺序"漏。这是 r10 / 10 轮 review
上限的最后一轮，需修彻底无下一轮验证。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | AddOnline 不清旧 room stale member — RemoveOnline 漏跑后 user 永久同时在两 room | high (P1) | architecture | fix | `server/internal/repo/redis/presence_repo.go:302-325` |
| 2 | Register 替换路径 onUnregister(OLD) 先于 onRegister(NEW) — reconnect false offline window | medium (P2) | architecture | fix | `server/internal/app/ws/session_manager.go:266-272` |

## Lesson 1: AddOnline 必须自动 SREM 旧 room 的 stale member（cross-room reconnect 自愈）

- **Severity**: high (P1)
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/repo/redis/presence_repo.go:302-360`

### 症状（Symptom）

当 RemoveOnline(oldRoom, oldSession) 因 hook ctx timeout / Redis transient 失败
而漏跑 / 失败时：

- user_key 残留 oldSession|oldRoom（直到 TTL 过期 5min）
- oldRoom set 残留 user member（直到整 set TTL 过期）

下一轮 hook 链路或 scanner reconcile 调 AddOnline(newRoom, userID, newSession)：

- SET user_key=newSession|newRoom（覆盖旧 value）
- SADD newRoom（user 在 newRoom 上线）
- **但永远不 SREM oldRoom**

→ user 在 oldRoom + newRoom 同时 online。oldRoom 上任何活跃 user 周期续 TTL →
stale 永久存活。`ListOnline(oldRoom)` 永久 over-report 该 user。

修前架构里 RemoveOnline 是清旧 room 的唯一路径；scanner 30s reconcile 只走
AddOnline 路径，没有"扫描所有可能的旧 room SREM 过期 member"机制。

### 根因（Root cause）

把"清理旧 room 的责任"完全交给"必须正确 fire 一次的 hook 链路"是脆弱的。生产
路径上 hook 失败的方式很多：

- Redis brownout / network blip 导致 RemoveOnline ctx timeout
- Hook 内部 panic（即便加了 recover，仍可能丢调用）
- shutdown 路径上 hook 还没 fire 完进程就退出（即便有 WaitGroup，timeout 期可能跳过）

**自愈架构原则**：写入新状态的路径（AddOnline）应自带"清理旧状态"语义，让漏跑
的清理操作在下次 AddOnline 时自动补上。这与 r3 P2 同方向（scanner reconcile 调
AddOnline 而非 RenewTTL，因为 AddOnline 自带"恢复完整 presence" 语义；RenewTTL
只能续期已存在的 key，不会从 partial-fail 恢复）。

### 修复（Fix）

改 `presenceRepo.AddOnline` 流程：

```go
// before（r9）：
SET user_key = newSession|newRoom (with TTL)
EVAL Lua(SADD newRoom + EXPIRE newRoom)

// after（r10 P1）：
GET user_key
  → if oldRaw != "" → parseUserValue(oldRaw) → oldRoomID
    → if oldRoomID != newRoomID → SREM oldRoom userID  ← 新增自愈
SET user_key = newSession|newRoom (with TTL)
EVAL Lua(SADD newRoom + EXPIRE newRoom)
```

加 `parseUserValue(raw) (sessionID, roomID, error)` helper（与 Lua script 内
string.find/sub 解析逻辑等价）。

加 3 个单测 case：
- `TestPresenceRepo_AddOnline_CrossRoomReconnect_SREMsStaleOldRoom`：模拟漏跑 →
  AddOnline 自动 SREM
- `TestPresenceRepo_AddOnline_SameRoomReconnect_DoesNotSREM`：same-room 路径
  必须跳过 SREM 防瞬时离线
- `TestPresenceRepo_AddOnline_FirstTime_NoStaleSREM`：first-time AddOnline 不
  错误 SREM

调整 `TestPresenceRepo_RemoveOnline_CrossRoomReconnect_SREMsOldRoom` 既有 case：
step 2 后 roomA 已被 r10 P1 SREM 干净，step 3 RemoveOnline 走 case 3 SREM 是
idempotent no-op 兜底（不耦合调用顺序）。

**为什么 Go-side 多步而非单 Lua 原子**：
- Lua KEYS 必须在 EVAL 前列举（Redis Cluster 限制 cross-slot）
- oldRoom 是从 user_key value 动态解析出来，无法预知
- 配合 r9 P1 per-user mutex 串行化（hook 与 scanner 共享 LockFor）足以保证
  TOCTOU 不发生：同 userID 的所有 AddOnline / RemoveOnline 路径在锁内排队，
  GET → SREM → SET → Lua 这一段不可被并发抢占

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计带 lifecycle 钩子的去重型缓存 / 索引 / 注册表**
> 时，**禁止**把"清理旧状态"的责任完全交给"必须正确 fire 一次的退出钩子"，
> **必须**让"写入新状态"的路径自带"清理与新状态冲突的旧状态"自愈语义。
>
> **展开**：
> - lifecycle hook fire 失败有 N 种方式（network、ctx timeout、panic、shutdown
>   race），每种都让"清理动作漏一次"。如果靠"必须 fire 一次"作为正确性的唯一
>   保证，生产路径的尾分布很容易被这些失败 dominate。
> - 自愈架构：写入路径自带读取旧状态 → 比对 → 清理冲突的语义。这与 r3 P2
>   "scanner reconcile 调 AddOnline 而非 RenewTTL" 同方向（AddOnline 自带恢复
>   完整 presence；RenewTTL 只能 maintain 已有 key）。每次 AddOnline 都是"权威
>   重写当前状态"的机会，要利用。
> - 自愈 SREM 的成本：一次额外 GET（local Redis < 1ms，远端 < 10ms RTT），
>   reconnect 路径已经是 hot path 不在乎多一次 round trip，但带来的好处是消除
>   "依赖 hook 必须正确 fire" 的脆弱性。
> - **反例**：RemoveOnline 是清旧 room 的唯一路径，若失败 → 永久残留 → 业务方
>   看到 over-report 数据。即便 RemoveOnline 自身已经原子化（Lua script），
>   也无法保护它"被调用一次"这一外部不变量。
> - **反例**：靠 scanner 30s tick 兜底"扫描所有 room SREM 过期 member"。这种
>   全表扫描方案性能不可控（room 数 × user 数），且要求 scanner 知道每个 user
>   "曾经在哪些 room"（需要二级反向索引）。把自愈下沉到 AddOnline 路径让架构
>   保持简单。

## Lesson 2: SessionManager.Register 替换路径必须先 onRegister(NEW) 再 replaced.Close()

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/app/ws/session_manager.go:266-272` + 配套注释 `Register` godoc

### 症状（Symptom）

reconnect 替换路径下 hook 触发顺序是：

```
锁释放
→ replaced.Close() → notifyClosed → Unregister(oldID) → onUnregister(OLD)
   → RemoveOnline(OLD) [user_key DEL + oldRoom SREM]
→ onRegister(NEW)
   → AddOnline(NEW) [user_key SET + newRoom SADD]
```

中间窗口（RemoveOnline 跑完到 AddOnline 跑完之间，~10ms 正常 / ~几百ms brownout 期）
里 IsOnline / ListOnline 查询都看到 user **暂时离线**。Redis-backed 业务路径
（节点 11+ 房间快照、节点 21+ presence broadcast 等）在该窗口内拿到错的状态。

修前理由："锁外先 Close 旧 session" 是因为 `replaced.Close()` 可能慢（writeLoop
flush + WS frame 发送），不阻塞 onRegister 让新 session 注册更快感觉直觉合理 ——
但这是 r9 同步 hook 之前的错误直觉，r9 之后 hook 已是 inline Redis I/O，顺序
关系直接决定 presence 状态可见性。

### 根因（Root cause）

`replaced.Close() → onUnregister(OLD)` 与 `onRegister(NEW)` 是两个独立 hook
触发，调用顺序由 `Register` 函数本身决定。修前实装是先 Close 后 onRegister
（见 `session_manager.go` r9 之前的 26x 行）。这个顺序在 hook adapter 是
fire-and-forget 的早期版本无所谓（goroutine 抢锁不可控），但 r9 P1 改成同步
hook 后，调用顺序就**直接等于 user 状态可见时间线**：先 Close = 先看到 OLD
状态消失，然后才看到 NEW 状态出现 → 中间一段空窗。

正确顺序：先把 NEW 状态写入（AddOnline），再清 OLD 状态（RemoveOnline）。
这与 lock-free 算法的 "publish before retire" 模式同方向 —— 让查询者永远看到
"至少一个有效状态"。

### 修复（Fix）

调换 `session_manager.go:Register` 锁外两步顺序：

```go
// before：
if replaced != nil {
    _ = replaced.Close()
}
if onRegister != nil {
    onRegister(s)
}

// after：
if onRegister != nil {
    onRegister(s)              // 先写入 NEW（AddOnline）
}
if replaced != nil {
    _ = replaced.Close()       // 后清 OLD（RemoveOnline）
}
```

加配套测试：`TestPresenceHook_Reconnect_AddBeforeRemove_NoOfflineWindow`
通过 hook adapter 收集 callOrder，断言 `add:NEW` 在 `remove:OLD` **之前**。

**与既有不变量的兼容性**（sweep 自检）：

- **r2 P1**（reconnect 必须触发旧 Session onUnregister 钩子恰好一次）：✓
  保留。`replaced.Close()` 仍在锁外触发，仅顺序调换；OLD 在 sessionsByID
  保留路径不动 → Close → notifyClosed → Unregister(oldID) → onUnregister
  仍正常 fire。`TestSessionManager_Reconnect_TriggersUnregisterHookForOldSession`
  仅检查"恰好 1 次"不检查顺序，不破坏。

- **r5 P2**（reconnect 中场 ListSessionsByRoomID 不能同时返回 OLD/NEW）：✓
  保留。Register 锁内已经把 OLD 从 sessionsByRoom 移除 + 把 NEW 加入；锁外
  调 onRegister(NEW) 时 byRoom 已只剩 NEW。`TestSessionManager_Reconnect_NoDoubleBroadcastWindow`
  在 NEW 的 register hook 里 sample，仍只见 NEW。无破坏。

- **r7 P1**（same-room reconnect Lua case 4 跳 SREM）：✓ 保留。AddOnline(NEW)
  先跑（user_key=newSession|newRoom，SADD newRoom — same-room 时是 idempotent
  no-op）；replaced.Close → RemoveOnline(OLD) 后跑：Lua GET → newSession|newRoom，
  currentSession=newSession≠oldSession + currentRoom=newRoom==oldRoomID(=newRoom)
  → case 4 跳 SREM，user 在 room set 内连续在线。**与 r10 P1 配合更干净**：
  AddOnline 在 same-room 路径自身也跳 SREM（GET 拿到 oldSession|sameRoom →
  oldRoomID == newRoomID → 跳 SREM），双层防护。

- **r10 P1**（cross-room reconnect AddOnline 自愈 SREM oldRoom）：✓ 配合。
  AddOnline(NEW) 先跑：GET 拿到 oldSession|oldRoom → oldRoomID != newRoomID
  → SREM oldRoom 干净；之后 replaced.Close → RemoveOnline(OLD)：Lua case 3
  仍 SREM oldRoom（idempotent no-op，不耦合顺序）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **同步 lifecycle hook 模型下设计 "替换 / 切换"
> 类操作（reconnect、connection migration、leader handoff 等）** 时，**必须**
> 让"写入新状态"的 hook 先于"清理旧状态"的 hook 触发，让查询者全程至少看到
> 一个有效状态。
>
> **展开**：
> - "publish before retire" 是 lock-free 编程的经典模式：先发布新状态让 reader
>   能看到，再回收旧状态，避免 reader 在两者之间看到空。把这个原则推广到
>   sync hook 序列化：先 add 后 remove，而不是先 remove 后 add。
> - 同步 hook 模型下，hook fire 顺序 = 外部状态可见性时间线。任何"先清后立"
>   的顺序都会暴露中间空窗（即使 ms 量级在 brownout 期会放大到几百 ms）。
> - "为什么不先 Close 旧 session" 的反向直觉来自 fire-and-forget 时代 ——
>   hook 不阻塞，顺序由 goroutine 抢锁决定。同步化之后这条理由不成立。
> - 如果 add hook 自身依赖"旧状态已清"才能正确（如 unique constraint），
>   解决方案不是回到"先清后立"，而是让 add hook 自带"清旧状态"自愈语义
>   （见 lesson 1 的 r10 P1 修法 —— AddOnline 内嵌 SREM oldRoom）。
> - **反例**：r9 之前实装 "锁外先 replaced.Close 再 onRegister"。fire-and-forget
>   时代无所谓，r9 同步化后变成正确性 bug。修代码时**必须**重新审视一切顺序
>   假设，尤其是从异步改同步的 refactor。
> - **反例**：靠"replaced.Close 慢，onRegister 应先" 这种性能直觉决定调用顺序。
>   性能优化不能违背正确性 —— 真要快可以并发起两个 hook（但需要清楚 race，
>   不在本案 scope）。

## Meta: 本次 review 的宏观教训

10-6 r1-r10 共 10 轮 review fix 的累积观察：每一轮 review 都在揭示
**presence 这一类"状态在多 actor 间共享 + 异步触发"系统的根本难点 —— 不变量
的边界与 actor 之间的契约**。

观察到的 root cause taxonomy：
1. r1：**单命令不原子** → Lua compare-and-delete
2. r2：**多命令顺序错** → 调换命令顺序让 partial-fail 不留残留
3. r3：**RenewTTL 不能从 partial-fail 恢复** → 改用 AddOnline 全态自愈
4. r4：**特殊 case 漏分支**（cross-room reconnect SREM）→ Lua case 3 SREM
5. r5：**fanout 没 ctx timeout** → per-call ctx timeout
6. r6：**hook 同步阻塞 main path** → fire-and-forget（**反向引入新问题**）
7. r7：**fire-and-forget race**（race condition）→ value 编码 roomID +
   Lua case 4 区分 same/cross
8. r8：**TOCTOU race 仍然存在** → per-user mutex（但仍是补丁）
9. r9：**fire-and-forget 模式根本性 race** → 同步化 + 共享锁（结构性翻案）
10. r10：**"靠正确 fire 一次的清理"是脆弱契约**（Lesson 1）+ **同步 hook
    顺序 = 状态可见时间线**（Lesson 2）

每一轮的 lesson 都在不同抽象层：1-5 是 Redis 命令层；6-9 是 Go 并发层；
10 是 architecture 层（自愈 vs 必须正确 fire）+ 时间线层（hook 顺序 = 状态
可见性）。

**给未来 Claude 的元规则**：写"状态在多 actor 间共享 + 异步触发"的系统时，
不要按"功能完整 → 性能优化 → 正确性补丁"的顺序写。倒过来：先想清楚

1. 不变量是什么（presence 数据准确度 + 连续可见性）？
2. 谁能破坏不变量（fire 失败 / 调用顺序）？
3. 怎么让破坏自愈 / 怎么让顺序关系本身保护不变量？

如果发现自己在加第 N 个 mutex / guard / retry 来"保护不变量被某 actor 破坏"，
stop，回到 1 重新设计 —— 通常需要把不变量本身改造（state-driven self-healing
或 publish-before-retire ordering），而不是给 actor 加更多"必须正确做 X" 的契约。
