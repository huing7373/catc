---
date: 2026-05-07
source_review: codex review (epic-loop r7) — /tmp/epic-loop-review-10-6-r7.md
story: 10-6-redis-presence-repo
commit: ed0e727
lesson_count: 2
---

# Review Lessons — 2026-05-07 — Presence same-room reconnect 必须 room-aware guard & shutdown 必须等 fire-and-forget hook 跑完才关共享 client（10-6 r7）

## 背景

Story 10.6 r6 把 lifecycle hook 内部 Redis I/O 改成 fire-and-forget goroutine（修 Register / Unregister 同步阻塞 Redis brownout），但 r7 codex review 指出两个由这个改动衍生出的新问题：

1. **r4 P1 修后 r7 P1 修前的 race**：r4 让 Lua script case 3（sessionID 不匹配）总是 SREM 旧 room 来修 cross-room reconnect 残留。但 r6 改成 fire-and-forget 后，same-room reconnect 路径变成 "new AddOnline goroutine 先跑 SADD + 旧 RemoveOnline goroutine 后跑 SREM" race —— Lua script 拿不到新 session 的 room，没法区分 same-room（不该 SREM）vs cross-room（该 SREM），所有不匹配路径都走 SREM → user 在 same-room reconnect 后短暂消失到下一次 scanner reconcile 才自愈。
2. **r6 注释里说的"shutdown 期 fire-and-forget goroutine 可能没机会跑完 → TTL 兜底" 不对**：`sessionMgr.Close()` 串行触发 N 个 Unregister 钩子各自 dispatch 一个 RemoveOnline goroutine 后立刻返；defer LIFO 让 `redisClient.Close()` 紧接着关闭共享 client → in-flight RemoveOnline 全部撞 "redis: client is closed"。每次 clean restart 后 `room:*:online_users` 留 stale member 直到 TTL 5min 过期。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Lua script 缺 room context → same-room reconnect SREM race | P1 (high) | architecture / correctness | fix | `server/internal/repo/redis/presence_repo.go` |
| 2 | Shutdown 路径上 redisClient.Close 在 fire-and-forget hook 跑完前关 | P2 (high) | shutdown / correctness | fix | `server/cmd/server/main.go` |

修了 2 条 / defer 0 条 / wontfix 0 条。

## Lesson 1: Lua script "compare-and-act" 路径缺少业务上下文（room）→ 同业务键不同分支无法区分 → 必须把上下文 encode 进 value

- **Severity**: high (P1)
- **Category**: architecture / correctness
- **分诊**: fix
- **位置**: `server/internal/repo/redis/presence_repo.go`（AddOnline value 编码 + RemoveOnline Lua script）

### 症状（Symptom）

r4 P1 修把 Lua script 改成 "case 3（current sessionID != ARGV[1]）总是 SREM 旧 room 但跳过 user key DEL"，理由是 cross-room reconnect 路径下旧 room 的 SREM 必须执行。但 r6 把 hook 改 fire-and-forget 后 same-room reconnect 真实复现路径变成：

1. 新 hook goroutine 先跑：`SET user:{id}:ws_session = newSession` + `SADD roomA userID` + `EXPIRE roomA`
2. 旧 hook goroutine 后跑：`Lua GET user:{id}:ws_session` → 返回 newSession ≠ oldSession → r4 case 3 → `SREM roomA userID`

结果：user 在 same-room reconnect 后被旧 hook 的 SREM 从 roomA 干掉，`IsOnline(roomA, userID)` / `ListOnline(roomA)` 在 30s scanner reconcile 自愈窗口内返错。前 scanner 周期任何依赖 presence 的业务调用（Epic 11 ListOnline 拉房间快照、IsOnline 守卫）都看到 "user 不在线"。

### 根因（Root cause）

Lua script 的判断条件信息不够：只能拿到 (current sessionID, oldSessionID)，但 cross-room vs same-room 的区分还需要 (current roomID, oldRoomID)。r4 修法基于一个隐式假设 "sessionID 不匹配 → 一定是 cross-room reconnect"，没意识到 fire-and-forget 路径会让 same-room reconnect 也走到 "sessionID 不匹配但其实仍同 room" 的分支。

更深的根因是 r1 设计 user value schema 时就只放了 sessionID 一个字段，没把 "新 session 在哪个 room" 也 encode 进去。接口契约里 user→sessionID 是给 sessionID guard 用的，没考虑过 "同时还要做 room context 比较"。这种"value 的语义只够支持当前 use case，但下次扩展就缺一截"是 schema 设计的常见暗坑。

### 修复（Fix）

把 `user:{id}:ws_session` 的 value 从纯 sessionID 改成 `"sessionID|roomID"` 组合字符串。schema 仍是 String 类型（兼容 V1 §9.1 钦定），只是 value 编码扩展。Lua script 解析 currentRoom 与 ARGV[3]（旧 roomID）比较，区分 same-room（跳 SREM）vs cross-room（SREM 旧 room）：

```lua
local current = redis.call("GET", KEYS[2])
if current == false then
  redis.call("SREM", KEYS[1], ARGV[2])
  return 1
end
local sep = string.find(current, "|", 1, true)
local current_session
local current_room
if sep == nil then
  current_session = current
  current_room = ""
else
  current_session = string.sub(current, 1, sep - 1)
  current_room = string.sub(current, sep + 1)
end
if current_session == ARGV[1] then
  redis.call("SREM", KEYS[1], ARGV[2])
  redis.call("DEL", KEYS[2])
  return 2
end
if current_room == ARGV[3] then
  return 4  -- same-room reconnect, skip SREM
end
redis.call("SREM", KEYS[1], ARGV[2])
return 3
```

AddOnline 的 SET 值改成 `formatUserValue(sessionID, roomID)` = `"sessionID|roomID"`。RemoveOnline 多传一个 `roomIDStr` 进 ARGV[3]。

为什么选"组合字符串"而不是改成 Hash：
- Hash schema 改动是 V1 §9.1 钦定的 user→sessionID String 兼容性破坏；组合字符串保持 String 类型，只扩展 value 编码
- Hash 需要多命令（HMSET/HGET）替代单 SET，重写 AddOnline 的整个 SET → SADD → EXPIRE 顺序 + partial-fail 矩阵
- 分隔符 `|` 与 sessionID（uuid v4 hex+`-`）和 roomID（十进制）都不冲突
- 解析成本：Lua script 用 `string.find` + `string.sub` 单次扫描 + Go 端 `strconv.FormatUint` 一次拼接，零开销

测试覆盖：
- `TestPresenceRepo_RemoveOnline_SameRoomReconnect_SkipsSREM_NoVisibilityGap`（新加）：same-room reconnect 后 IsOnline 必须**全程**返 true，没有"瞬时 false 窗口"
- `TestPresenceRepo_RemoveOnline_CrossRoomReconnect_SREMsOldRoom`（已有，r4 加）：cross-room reconnect 仍 SREM 旧 room
- `TestPresenceRepo_RemoveOnline_ReconnectRace_GuardsUserKey`（已有，r1 加）：user key 受 sessionID guard
- `TestPresenceRepo_RemoveOnline_TTLExpiredKey_StillCleansSetMember`（已有）：user key 不存在路径仍 SREM
- 既有 partial-fail 测试同步更新 value 期望（`s1` → `s1|100`）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 设计 **基于 Redis Lua script 的 compare-and-act 操作** 时，**必须先列出**所有需要进入 script 判断逻辑的业务字段，然后**把所有字段 encode 进 KEYS / ARGV / value**，让 script 自己有完整业务上下文做分支决策；**禁止依赖** "sessionID 不匹配 ⇒ 一定是某种 reconnect 类型" 这种**单字段隐式推断**。
>
> **展开**：
> - 写 Lua script 前先做"分支表"：列每个 case + case 触发条件需要的输入字段。case 不能区分 = 缺字段，必须扩展 value schema 把字段放进去。
> - "value schema 只够当前 use case" 是 mountable 暗坑：今天 user→sessionID 只为 sessionID guard，明天加 cross/same-room 区分就缺一截。设计 schema 时问 "未来还可能怎么扩展 compare-and-act 路径？"先把扩展点考虑进去。
> - **value 编码扩展 vs 类型升级**：String → Hash 是 schema-breaking change（V1 §9.1 钦定 user→sessionID 是 String），需要写 migration / 协调上下游。**优先**用分隔符（`|`、`:`、`,`）扩展 String value 多字段；只有字段超过 3-5 个或字段值含分隔符时才升级到 Hash / JSON。
> - 单测覆盖：每个 Lua script 分支必须有独立 case 锁定 return 值与边界状态。case 表现成 "SREM 后 IsOnline 应仍 true"（断言**最终用户态**而非 script 内部 return 值），让分支重构时 case 还能继续守。
> - 配合 fire-and-forget goroutine 路径：goroutine 顺序不可控 = race 路径多。compare-and-act 必须假设 "新写入比旧清理先发生" 这种最坏排序也成立，即便没有 fire-and-forget，也要假设跨进程 / 跨 region replication 让顺序不严格。
> - **反例**：`if current == ARGV[1] then ... else ... end` 只有 2 case，却用来覆盖 3 种业务路径（key 不存在 / sessionID 匹配 / sessionID 不匹配但 same-room / sessionID 不匹配且 cross-room）—— 必然有 case 要被错误归并。
> - **反例**：依赖"调用方传入的参数"反推业务上下文（"调用方传 oldRoom = newRoom 时是 same-room"）—— 调用方根本不知道 newRoom，只有 user key 当前 value 知道。必须让 script 自己读 user key 拿到 currentRoom。
> - **反例**：把"同 room 反复 reconnect 偶发 IsOnline false 窗口"当作 "TTL 兜底自愈" 可接受 —— scanner reconcile 周期 30s，30s 内任何依赖 presence 的业务调用（房间快照拉取、emoji 广播筛选目标）都会跑错；TTL 兜底是给"持续在线但 hook 漏写"准备的兜底，不能用来掩盖 "hook 写完又被旧 hook 删掉" 的逻辑 race。

## Lesson 2: Fire-and-forget hook 路径上 graceful shutdown 必须用 sync.WaitGroup 等所有 goroutine 跑完才关共享资源（如 Redis client）

- **Severity**: high (P2)
- **Category**: shutdown / correctness
- **分诊**: fix
- **位置**: `server/cmd/server/main.go`（hook goroutine 加 WaitGroup + shutdown 顺序加 wg.Wait）

### 症状（Symptom）

r6 把 hook 改 fire-and-forget 后，shutdown 序列变成：

1. SIGTERM → main signal ctx cancel → bootstrap.Run 返回
2. Defer LIFO 触发：先跑最早注册的（cancelHeartbeat → wait scannerDone → sessionMgr.Close）
3. `sessionMgr.Close()` 串行调 N 个 Session.Close → notifyClosed → Unregister → onUnregister hook → **dispatch RemoveOnline goroutine 立刻返**
4. 步骤 3 defer 整体返回 → 下一个 defer 是 `redisClient.Close()`
5. **redisClient.Close 立刻关共享 client** → 步骤 3 dispatch 的 RemoveOnline goroutines 全部撞 `redis: client is closed`
6. 每次 clean restart 后 `room:*:online_users` 留 stale member 直到 TTL 5min 过期

r6 lesson 注释里说"shutdown 期 fire-and-forget goroutine 可能没机会跑完 → TTL 兜底" —— 但 TTL 兜底是 "presence 在 5 分钟内自然清"，不是"presence 立即清"。在这 5 分钟内 IsOnline / ListOnline 会 over-report 已离线 user，违反 "graceful shutdown 应让外部状态对齐 server 真实状态" 的设计意图。

### 根因（Root cause）

写 r6 fix 时把"主路径不阻塞 Redis I/O"和"shutdown 期可以扔掉这些 I/O"两件事错误绑定了。fire-and-forget 解决的是**主路径延迟 vs Redis 健康度耦合**，不是"hook 工作允许丢失"。shutdown 期是 hook 的最后一次执行机会，丢了就只能等 TTL；而 TTL 是"暴力崩溃 / 网络分区"路径的兜底，不能挪用为"正常关停"路径的清理手段。

更深的根因是没区分两种 fire-and-forget 语义：
- **fire-and-forget for non-blocking dispatch**（业务用法）：解耦主路径与异步处理时机，但**仍要确保异步任务最终执行完**
- **fire-and-forget for opportunistic side-effect**（兜底用法）：异步任务漏跑可接受（TTL / scanner reconcile 兜底），shutdown 期允许丢弃

presence hook 是前者：业务需要 RemoveOnline 真的执行（让 ListOnline 立刻反映离线）。r6 误归到后者类别。

### 修复（Fix）

加 `sync.WaitGroup` 跟踪所有 hook goroutine：

```go
var presenceHooksWG sync.WaitGroup
sessionMgr := wsapp.NewSessionManager(
    wsapp.WithRegisterHook(func(s *wsapp.Session) {
        presenceHooksWG.Add(1)
        go func() {
            defer presenceHooksWG.Done()
            // ... AddOnline ...
        }()
    }),
    wsapp.WithUnregisterHook(func(s *wsapp.Session) {
        presenceHooksWG.Add(1)
        go func() {
            defer presenceHooksWG.Done()
            // ... RemoveOnline ...
        }()
    }),
)
```

shutdown 序列加 `presenceHooksWG.Wait()`，放在 sessionMgr.Close 之后、本 defer 返回之前（让 LIFO 让 redisClient.Close 推迟）：

```go
defer func() {
    cancelHeartbeat()
    <-scannerDone
    if cerr := sessionMgr.Close(); cerr != nil {
        slog.Error("session manager close failed", slog.Any("error", cerr))
    }
    // 等所有 fire-and-forget hook goroutine 跑完
    presenceHooksWG.Wait()
}()
```

为什么不需要把 hook 的 ctx 改用 main ctx 派生：hook ctx 是 `context.WithTimeout(context.Background(), presenceHookTimeout)`，独立于 main ctx。即便 main ctx 已 cancel，hook ctx 仍可走 2s timeout。WaitGroup 等的就是这 2s 上限内 hook 完成；如果 Redis 真的卡死 2s，timeout cancel 让 hook goroutine 退出，WaitGroup 仍 unblock。

总等待时间分析：
- N 个 hook goroutine 并发跑（不串行），单个上限 = presenceHookTimeout = 2s
- 即便 N=1000 session，并发 dispatch 后总等待时间 ≈ 2s（单个上限），不是 O(N×2s)
- K8s `terminationGracePeriodSeconds` 默认 30s，2s 完全在范围内

测试覆盖：本修法是 main 包 wire-up 改动，缺 cmd/main 单测（典型 main 包不写单测）。已有测试覆盖：
- `TestPresenceRepo_RemoveOnline_*`（presence 业务路径正确性）
- `wsapp.TestSessionManager_Close_*`（sessionMgr.Close 触发 onUnregister 钩子的正确性）

新加单测代价（mock SessionManager + WaitGroup 行为）远超本修法的复杂度，留运行时观察 + slog.Info "ws presence hooks drained" 日志做信号。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 把 lifecycle hook 内部 I/O 改 **fire-and-forget goroutine** 时，**必须区分** "异步任务必须最终执行"（业务清理 / 状态写入）vs "异步任务漏跑可接受"（metrics / opportunistic 兜底）；前者**必须**在 graceful shutdown 路径上用 `sync.WaitGroup` 等所有 goroutine 跑完才关共享资源（Redis client / DB pool / HTTP transport）；不能依赖 TTL 兜底当作"shutdown 期允许丢"的逃生口。
>
> **展开**：
> - 判定问句："这个异步任务漏跑会让外部观察者看到错误状态吗？" 答 YES → WaitGroup 必填。"业务调用 IsOnline / ListOnline 在 5min 内会 over-report" = YES。
> - 判定问句二："TTL 兜底的设计意图是什么？" TTL 是给 **暴力崩溃 / 网络分区 / hook 异常** 路径的最后兜底，不是 **正常关停** 的预期清理路径。挪用兜底当作主路径"省事工具"会让"正常 vs 异常"路径的可观测性混在一起。
> - 共享资源关闭的顺序：把 "依赖资源 R 的所有 goroutine"（hook goroutine / scanner goroutine / consumer goroutine）放在一个 WaitGroup，shutdown 序列**先**等 WaitGroup，**后**关 R。defer LIFO 让"等"放在最早注册的 defer 末尾、"关"放在更晚注册的 defer 内，自然顺序对齐。
> - WaitGroup.Wait 不会无限阻塞：每个 goroutine 有 timeout ctx 上限（这里 2s），N 个并发跑总等待 ≈ 单 goroutine 上限。比 K8s terminationGracePeriodSeconds（30s）小一个量级。
> - hook 上的 `Add(1)` **必须**在 `go func() {...}()` 之前调用（同步段），不能放 goroutine 内部 —— 否则 Wait 可能在 Add 之前跑过去（race）。
> - 跑路径 audit：每加一个 fire-and-forget goroutine 路径，必须在 shutdown 路径文档 / 注释里同步加 "等本路径 WaitGroup" 的检查项。否则下次维护时漏加新 goroutine 的 Wait 不会被发现。
> - **反例**：`go func() { redisRepo.RemoveOnline(...) }()` 没 WaitGroup 跟踪，shutdown 时立刻关 redisClient → goroutine 撞 closed client → 业务 cleanup 全部丢。
> - **反例**：把 hook 的 ctx 改用 main ctx 派生（"main ctx 还活的时候 hook 跑完"）—— main ctx 在 SIGTERM 时立刻 cancel，hook ctx 也跟着 cancel，RemoveOnline 全部因 ctx.Canceled 失败，问题更糟。
> - **反例**：依赖 "TTL 5 min 兜底" 当作 shutdown 路径的预期清理 —— TTL 是 "暴力崩溃路径" 的最后兜底，不能挪用。挪用后 5min 窗口内业务 IsOnline / ListOnline 全部 over-report，违反"graceful shutdown 应让外部状态立刻对齐"的设计意图。
> - **反例**：`presenceHooksWG.Wait()` 放在 redisClient.Close defer **里**或之后 —— defer LIFO 已经让 hook wait 在 redisClient.Close 之前跑（前者注册更早），但**也**得在 sessionMgr.Close 之后（让 sessionMgr.Close 触发的 hook dispatch 跑过）。位置：sessionMgr.Close 之后、本 defer 返回之前。

---

## Meta: r6 → r7 整体教训

r6 是"修主路径阻塞"的修法（fire-and-forget hook），r7 review 暴露的两条问题都是 r6 这个改动的副作用：

- **finding 1（lua script 缺 room）**：r6 让 hook 顺序变成 race，原 r4 修法依赖的"sessionID 不匹配 ⇒ cross-room reconnect" 隐式推断在 race 路径下失效。
- **finding 2（shutdown 不等 hook）**：r6 注释里写"TTL 兜底足够"，把 fire-and-forget 误归为 "异步任务可丢" 类别，没意识到这是业务清理任务必须最终执行。

更广义的教训：**改一个路径会让另一个路径的隐式假设失效**。r6 改 hook 时应该问 "原 r4 修法靠什么前提工作？hook 同步路径吗？现在改 fire-and-forget 后这个前提还在吗？" 答否就要同步重新审视 r4 的实装。这是 "修法之间相互依赖" 的盲点 —— 一次只看当前 review 的 P1，但 P1 修法可能让前几次的 P1 隐式假设失效。

行动规则：每次改 lifecycle hook 路径时，把所有依赖该路径执行顺序 / 同步语义的实装（这里是 RemoveOnline Lua script + redisClient.Close 顺序）列出来重新 audit 一遍，不能只看本次 review 的 P1。
