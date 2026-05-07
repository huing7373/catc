---
date: 2026-05-07
source_review: codex review (file: /tmp/epic-loop-review-10-6-r2.md) for Story 10-6 redis-presence-repo
story: 10-6-redis-presence-repo
commit: 32b5d5b
lesson_count: 2
---

# Review Lessons — 2026-05-07 — Redis presence TTL 必须周期续期 & AddOnline 命令顺序必须让 partial-fail 不留永久 zombie（10-6 r2）

## 背景

Story 10.6 r2 codex review 命中两条与 Redis presence repo 实装 / wire 相关的 finding：一条 P1（long-lived WS session 5 分钟后被 TTL 误报 offline，因为 RenewTTL 写好但没人调）+ 一条 P2（AddOnline 三命令编排在 partial fail 路径下让 room set 永久无 TTL）。两条都是"语义层面会真坏"的问题，必须修。

修法：
- P1 走方案 (b) — HeartbeatScanner.scanOnce 对 active session 同步调 PresenceRenewer.RenewTTL，复用 30s tick 频率（远小于 TTL 5min）做续期。
- P2 走简化版调换命令顺序（SET → SADD → EXPIRE）让最严重的"永久 zombie" 退化为"短窗口轻度不一致 + TTL 自愈"，不上 Lua script 升级（影响范围 / 改动量都更大）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | long-lived WS session 没有任何路径调 RenewTTL，5min 后 presence 自动过期误报 offline | high | architecture | fix | `server/cmd/server/main.go:311-317` + `server/internal/app/ws/heartbeat_scanner.go:221-258` |
| 2 | AddOnline 命令顺序 SADD→SET→EXPIRE 在 SET 失败时让 room set 永久无 TTL | medium | error-handling | fix | `server/internal/repo/redis/presence_repo.go:209-230` |

## Lesson 1: 周期性 TTL 续期必须挂在 server 端的 lifecycle scanner 而非依赖 client 心跳路径

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/cmd/server/main.go:311-317` + `server/internal/app/ws/heartbeat_scanner.go:63-105, 221-258`

### 症状（Symptom）

`PresenceRepo.RenewTTL` 接口与实装在 Story 10.6 主交付里就有了 + 单测 `TestPresenceRepo_RenewTTL_KeepsKeyAlive` 通过，但**没有任何调用站**。WS 长会话超过 `redis.presence_ttl_sec`（默认 300s）后：

- `room:{id}:online_users` 与 `user:{id}:ws_session` 因 TTL 过期被 Redis 清掉
- `IsOnline(roomID, userID)` 返 false（即使 SessionManager 内 Session 还活）
- `ListOnline(roomID)` 漏掉该 user
- 任何 future story 把 presence 当作"是否在线"信号源都会误报

5 分钟在生产环境是常态会话长度（pet app 用户挂着不操作很正常），bug 触发率 ≈ 100%。

### 根因（Root cause）

Story 10.6 设计阶段把 RenewTTL 实装好，但**钩子挂载点延后到"future 优化 story"**（见接口 godoc：`本 story 仅交付 RenewTTL 方法实装 + 单测；钩子挂载由 future 优化 story 推进`）。这是典型的"接口先行 + 实装在 + 调用站没接 = 死 API"反模式：

- 单测层面 RenewTTL 单独测能过（`TestPresenceRepo_RenewTTL_KeepsKeyAlive` 直接调接口验证）
- review 层面会被发现因为 reviewer 走"完整 lifecycle"心智模型而不是"接口逐一 cover" 心智
- production 跑起来才会真坏

**钩子挂载点选项分析**（review 提供 a/b/c 三选项）：

- (a) `readLoop.handlePing` 内调 RenewTTL：refresh 与 client 实际活动直接挂钩，"client 还在 → 还在线"语义最自然；但需要在 Session/SessionManager 接口加 ping hook 钩子机制，改动面更大
- (b) `HeartbeatScanner.scanOnce` 内对 active session 调 RenewTTL：复用 Story 10.4 已有的 30s tick scanner，注入 PresenceRepo 依赖即可；scanner 扫的是"manager 认为活跃"的 session，与 manager 一致性更强，不依赖 client 实际 ping 频率
- (c) 新增独立 PresenceRenewer goroutine：每 30s 扫所有 active session 调 RenewTTL —— 增加复杂度，无收益

**选 (b)** 的关键理由：
1. 实装难度：复用既有 ListAllSessions + for loop，加一行 RenewTTL，不需要新 hook 接口
2. 与 manager 一致性：scanner 视角与 IsOnline / ListOnline 的 "active session" 心智模型一致
3. 不依赖 client ping 频率：即使 client 短暂 ping 慢一点（移动端切前后台），manager 还认为它活就还会续期，避免 client-side 时序波动让 server presence 抖动

### 修复（Fix）

1. **新增窄化接口** `PresenceRenewer`（`heartbeat_scanner.go`）— 仅 1 个 method 让 scanner 不直接 import repo/redis 包：

   ```go
   type PresenceRenewer interface {
       RenewTTL(ctx context.Context, roomID, userID uint64) error
   }
   ```

   `PresenceRepo` 接口超集自动满足。

2. **HeartbeatScanner 加可空 renewer 字段**：

   ```go
   type HeartbeatScanner struct {
       mgr      SessionManager
       cfg      heartbeatScannerConfig
       logger   *slog.Logger
       interval time.Duration
       renewer  PresenceRenewer // review 10-6 r2 P1 加；可空
       wg       sync.WaitGroup
   }
   ```

   `NewHeartbeatScanner` signature 加第 4 参数 `renewer PresenceRenewer`；nil 接受（单测 / 未接 Redis 的最小路径）。

3. **scanOnce 在 active 分支调 RenewTTL**：

   ```go
   for _, sess := range sessions {
       idle := nowMs - sess.LastHeartbeatAt()
       if idle <= timeoutMs {
           // active session 续 presence TTL
           if s.renewer != nil {
               if err := s.renewer.RenewTTL(ctx, sess.RoomID(), sess.UserID()); err != nil {
                   s.logger.Warn("ws presence renew ttl failed", ...)
               }
           }
           continue
       }
       // idle 路径走 close fanout（既有逻辑）
   }
   ```

   关键设计：仅对 **active**（idle <= timeoutMs）session 续期；idle session 即将走 4005 close 路径，没必要续 TTL（onUnregister 钩子会在 close 完成后清干净 presence）。

4. **main.go wire**：

   ```go
   heartbeatScanner := wsapp.NewHeartbeatScanner(sessionMgr, cfg.WS.HeartbeatTimeoutSec, slog.Default(), presenceRepo)
   ```

5. **测试**（`server/internal/app/ws/ws_test.go`）：
   - `TestHeartbeatScanner_ScanOnce_ActiveSession_RenewsTTL`：active session 触发 RenewTTL，参数匹配
   - `TestHeartbeatScanner_ScanOnce_IdleSession_DoesNotRenewTTL`：idle session 走 close 路径不续期
   - `TestHeartbeatScanner_ScanOnce_NilRenewer_DoesNotPanic`：nil renewer 兜底
   - `TestHeartbeatScanner_ScanOnce_RenewerError_LoggedNotAborted`：单 session RenewTTL 失败不阻塞遍历下一 session（log warn 继续）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计带 TTL 的"在线态" / "幂等键" / "心跳" 类 Redis state 时**，**必须**在同一个 story 落地 TTL **续期路径的调用站**（不能只交付接口让"future story 接"）；钩子点优先选 server 端**已有的 lifecycle scanner**，**禁止**只挂 client-driven 路径（如 client ping）。

> **展开**：
> - "接口先行，调用站延后" 是死 API 高发反模式：单测能过，review 容易漏，生产必坏。落地 story 必须包含至少一个真正调用站
> - 续期点选择：**优先选 server 端已有的周期 scanner**（如 HeartbeatScanner / heartbeat lifecycle），让续期与"manager 视角的 active session" 一致，不依赖 client 实际行为；client-driven 路径（如 ping hook）作为可选第二层
> - tick 周期 vs TTL 关系：scanner 周期必须 << TTL；本案 30s tick vs 300s TTL 留 10 倍 buffer，足以容忍偶发 Redis 抖动 / scanner 跳过几次
> - 失败语义：单 session RenewTTL 失败必须 log warn 继续遍历，**不**让单 session 阻塞整批续期；TTL 自愈窗口（300s）远 > 几次 30s 重试，足以容忍偶发失败
> - **反例**：`PresenceRepo.RenewTTL` 实装好 + 单测覆盖 + interface godoc 写"future story 挂载"—— 看似工作完成，实际 production 行为完全没接，是 story 无完成的隐藏漏洞
> - **反例**：仅挂在 client ping 路径（选项 a）—— 在 client 短暂 ping 慢 / 切后台 / 网络抖动场景下让 server presence 抖动；server 端 scanner 是更稳的"manager truth source"

## Lesson 2: Redis 多命令编排在 partial-fail 下，命令顺序必须让"严重残留" 收敛到"短窗口 + TTL 自愈"

- **Severity**: medium
- **Category**: error-handling
- **分诊**: fix
- **位置**: `server/internal/repo/redis/presence_repo.go:209-230`

### 症状（Symptom）

原 AddOnline 编排顺序：`SADD room:{id}:online_users → SET user:{id}:ws_session ttl → EXPIRE room set ttl`。三命令 not atomic（Redis 不支持跨命令事务）。在 SADD 成功 + SET 失败路径下：

- 函数 `return fmt.Errorf("presence add online set: %w", err)` 立即返回
- **EXPIRE 没机会执行**
- 如果该 user 是 room 第一个 user → room set 是新建的 + 没有任何 TTL
- 该 zombie member 永久存活（永远不会被 Redis 自动清）

与 repo godoc 钦定的 "TTL 兜底让残留在 5 分钟内自然清除" 直接矛盾。

### 根因（Root cause）

**忽略了"命令顺序对 partial-fail 后果的影响"**。三个命令各自语义都对，但执行顺序决定了"哪类 partial fail 后果最坏"。原顺序选 SADD 在前是直觉的"先把 user 加到 room 里"，但忽略了：

- SADD 是**没有 TTL 选项**的命令（与 SET KEY VAL EX 不同）；SADD 后必须靠后续 EXPIRE 续命
- 所以 SADD 成功 + 后续任一命令失败 + return → room set 永远没有 TTL → 永久 zombie

正确思路：**让最容易 atomic 的命令在前**。`SET KEY VAL EX` 是 Redis 单命令原子（自带 TTL），把 SET 放第一位让"SET 失败" 路径完全没动其他 key，"SET 成功" 后即使后续命令失败也至少有 TTL 兜底。

**修后顺序的 partial-fail 矩阵**：

| 失败点 | user:{id}:ws_session | room set member | room set TTL | 后果 |
|---|---|---|---|---|
| SET 失败（首位）| 没写入 | 没写入 | n/a | 完全干净，下次重试 |
| SET ✓ + SADD 失败 | 已写入（**有 TTL**）| 没写入 | n/a | user key 5min 后自愈；下次 AddOnline 同 user 覆盖恢复 |
| SET ✓ + SADD ✓ + EXPIRE 失败 | 已写入（**有 TTL**）| 已写入 | 没 TTL | room set 上 member 残留直到下一次 AddOnline 同 room 触发 EXPIRE 续期；RemoveOnline 走 Lua 仍能 SREM 干净 |

最坏情况（SET ✓ SADD ✓ EXPIRE ✗）也比原版"SADD ✓ SET ✗"严重程度低：原版 zombie 永久存活，修后 zombie 受 RemoveOnline 路径 + 下次 AddOnline 续期约束。

**为什么不上 Lua script**：

- Lua script 包 SADD + EXPIRE 两命令原子可以彻底解决问题，但：
  - SET 仍要单独走（已经原子 SET KEY VAL EX）→ 三段式 + Lua 混合让代码复杂度上升
  - partial-fail 后果在简化版下已经"短窗口轻度不一致 + 自愈"，没必要为剩余的最坏 case 上更重的工具
- review 也明示"建议先用调换顺序的简化实装"

### 修复（Fix）

`server/internal/repo/redis/presence_repo.go:209-230`：

```go
// before（原顺序）:
if _, err := r.client.SAdd(ctx, rk, uidStr); err != nil { return ... }
if _, err := r.client.Set(ctx, uk, sessionID, r.ttl, false); err != nil { return ... }
if _, err := r.client.Expire(ctx, rk, r.ttl); err != nil { return ... }

// after（review r2 P2 修后）:
if _, err := r.client.Set(ctx, uk, sessionID, r.ttl, false); err != nil { return ... }
if _, err := r.client.SAdd(ctx, rk, uidStr); err != nil { return ... }
if _, err := r.client.Expire(ctx, rk, r.ttl); err != nil { return ... }
```

interface godoc 与实装 godoc 同步更新流程顺序 + 详述 partial-fail 矩阵。

**测试**（`server/internal/repo/redis/presence_repo_test.go`）加 `faultInjectingClient` wrapper（封装 RedisClient + 在第 N 次 SAdd/Set/Expire 调用时注 error）+ 三 case：

- `TestPresenceRepo_AddOnline_SetFails_NoLeftover`：SET 失败 → SADD/EXPIRE 没执行 → user 与 room set 都干净
- `TestPresenceRepo_AddOnline_SAddFails_UserKeyHasTTL_RoomSetEmpty`：SET ✓ + SADD 失败 → user key 有 TTL（FastForward 6min 后过期），room set 空；解除 fault 后下次 AddOnline 能恢复
- `TestPresenceRepo_AddOnline_ExpireFails_UserKeyHasTTL`：EXPIRE 失败 → user key 仍**有** TTL（验证 SET KEY VAL EX 已带 TTL）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **编排多个 Redis 命令** 时（无法 atomic 的场景），**必须**走"最容易 atomic 的命令放第一位 + 失败立即 return 让其他命令完全没动" 的顺序，**禁止**让"无 TTL 选项的命令"先成功后留下需要后续命令兜底的 key。

> **展开**：
> - 命令顺序设计原则：列出每条命令对 "残留状态" 的最坏后果矩阵；选让"最坏后果最轻" 的顺序
> - 优先把 `SET KEY VAL EX` 类原子命令放最前面：失败时其他状态完全没动，最干净；成功后即使后续命令失败也有 TTL 兜底自愈
> - 把"无 TTL 选项的命令"（SADD / HSET / LPUSH 等）放在已有 TTL key 之后；它们的副作用必须靠后续 EXPIRE 续命，先做就有"中途失败 → 永久 zombie" 风险
> - 简化版 vs Lua atomic：partial-fail 后果在简化版下能收敛到 "短窗口 + 自愈"，**不**需要 Lua；只有当后果是"永久不一致 / 影响多 user / 被 reconnect 路径 race"才升级到 Lua（与 RemoveOnline 的 Lua compare-and-delete 决策模式一致）
> - 测试 partial-fail：需要 fault-injection wrapper（counting-based 让特定 N 次调用注入 error），不只测 happy path
> - **反例**：`SADD room:set member → SET user:key val ttl → EXPIRE room:set ttl` —— SADD 在前，SET 失败时 EXPIRE 不跑，room set 永久无 TTL；与 "TTL 兜底清理 zombie" 语义相反
> - **反例**：仅测 happy path 不验证 partial-fail（"反正每个命令都用 fmt.Errorf 包了 → 失败时调用方会 log warn → 没事"）—— 残留状态的语义后果与 error 是否被 log 无关；必须显式验证残留矩阵

## Meta: 本次 review 的宏观教训

两条 finding 都属于 **"接口 / 命令编排在单元层面正确，在系统层面缺关键挂载或顺序设计"** 同一族：

- P1：RenewTTL 接口正确 + 单测正确，**调用站没接**
- P2：每个命令 + 错误包装正确，**命令顺序在 partial-fail 下后果最坏**

预防：每个 Redis state-mutation 类的 story 在 review 阶段必须问两个钩子问题：
1. "这个接口的所有方法都至少有一个真实生产调用站吗？" —— 防 P1 类 dead API
2. "如果第 N 个命令失败 + 函数 return，残留状态最坏是什么？" —— 防 P2 类残留矩阵盲区
