---
date: 2026-05-07
source_review: codex review (file: /tmp/epic-loop-review-10-6-r1.md) for Story 10-6 redis-presence-repo
story: 10-6-redis-presence-repo
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-07 — Redis presence RemoveOnline 必须带 sessionID guard 走 Lua atomic compare-and-delete（10-6 r1）

## 背景

Story 10.6 在 Redis 引入 presence repo（`room:{roomId}:online_users` Set + `user:{userId}:ws_session` String），并通过 `WithRegisterHook` / `WithUnregisterHook` 把 AddOnline / RemoveOnline 挂到 SessionManager lifecycle。10.6 r1 codex review 发现一条 P1：reconnect 替换路径下，旧 Session 的延后 Unregister 钩子会**误删**新 Session 刚写入的 presence，破坏"新 WS 仍 active 但 IsOnline / ListOnline 误报 user offline"语义。

修法采用方案 (A)：**扩展 RedisClient 接口加 Eval 方法 + RemoveOnline 用 Lua script 跑 atomic compare-and-delete**。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | RemoveOnline 不带 sessionID guard，reconnect 路径误删新 session presence | high | architecture | fix | `server/internal/repo/redis/presence_repo.go:221-227` |

## Lesson 1: Redis presence remove 必须带 sessionID guard，且 compare-and-delete 必须 Lua atomic（不可拆 pipeline）

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/repo/redis/presence_repo.go:221-227`

### 症状（Symptom）

`RemoveOnline(ctx, roomID, userID)` 接口签名只用 `(roomID, userID)` 两个索引；实装走 SREM + DEL 双命令直接删除。SessionManager.Register 替换路径的设计是"new Session 完成 Register（含 AddOnline 写 user:{id}:ws_session=newSessionID）→ old Session 异步 Close → old Session 的 Unregister 钩子触发 RemoveOnline"。如果 old Unregister 在 new AddOnline **之后**跑，老路径的 SREM/DEL 会清掉 new Session 刚写的 presence —— 其结果：

- `IsOnline(roomID, newUserID)` 返 false（虽然 new WS active）
- `ListOnline(roomID)` 漏 new user
- `user:{id}:ws_session` 被删空（下游若按该 key 反查 sessionID 全部错配）

reconnect 频次在生产环境（client 网络抖动 / 移动端切前后台）非边角，bug 触发率非负。

### 根因（Root cause）

**忽略了 SessionManager 替换路径的"old Unregister 与 new AddOnline 时序非确定"约束**。Story 10.3 r2 P1 修锁死了"reconnect 路径 oldSession.onUnregister 必须触发恰好一次"不变量（lesson `2026-05-06-ws-reconnect-unregister-hook-and-prod-contract-gate.md`），但 SessionManager 实装是"new Session.Register 完 → 锁外异步 oldS.Close()"，**old.Close → notifyClosed → Unregister 钩子相对 new AddOnline 钩子的相对顺序不保证**：

- 若 oldUnregister 在 newAddOnline **之前**：先删 → 后写，最终态是 newSessionID（正确）
- 若 oldUnregister 在 newAddOnline **之后**：先写 → 后删，最终态是空（错误）

第二种顺序非边角 —— `replaced.Close()` 走 socket close fanout 路径，写 close frame / wait drainable / 触发 readLoop 退出 / 触发 notifyClosed → Unregister，整条链路在 ms 级别；新 AddOnline 是直接命令 Redis，可能更快。

`RemoveOnline` 接口仅用 `(roomID, userID)` 索引，无法区分"我要删的是哪个 sessionID 的 presence"。这是设计层漏洞，不是实现 bug —— 接口契约本身需要带 sessionID guard。

**为什么 compare-and-delete 必须 Lua atomic 而非 pipeline / 三命令分开**：

- pipeline `GET → SREM → DEL` 的三命令 not atomic（其他 client 的 SET 可能插在 GET 与 SREM 之间）
- "先 client GET 比较，再 SREM/DEL" 拆两次往返 = check-then-delete race window 仍在
- Lua script 在 Redis server 端 single-thread 跑，三命令永远原子；reconnect 路径若在 script 执行期发生，被 Redis single-thread 序列化（要么发生在 script 之前要么之后，永远不会"夹在中间"）

**为什么 GET == false（key 不存在）也走删除分支**：考虑 `user:{id}:ws_session` TTL 提前过期（极端场景，比如 RenewTTL 路径还没挂）但 `room:{roomId}:online_users` 内 member 还在 → 严格"key 不存在就跳过" 会让 SREM 漏清残留；让 nil 走删除分支兜底是对的（底层 SREM/DEL 对不存在 key/member 都是 no-op，幂等安全）。

### 修复（Fix）

1. **RedisClient 接口**（`server/internal/infra/redis/client.go`）扩 `Eval` 方法（透明 forward 到 go-redis Eval，不做 nil error 内化 —— 不同 script 语义不同）：

   ```go
   Eval(ctx context.Context, script string, keys []string, args ...interface{}) (interface{}, error)
   ```

   实装（`redis.go`）直接 `c.client.Eval(ctx, script, keys, args...).Result()`。

2. **PresenceRepo.RemoveOnline 接口签名**加 sessionID 参数：

   ```go
   RemoveOnline(ctx context.Context, roomID, userID uint64, sessionID string) error
   ```

3. **实装走 Lua script atomic compare-and-delete**（`presence_repo.go`）：

   ```lua
   local current = redis.call("GET", KEYS[2])
   if current == false or current == ARGV[1] then
     redis.call("SREM", KEYS[1], ARGV[2])
     redis.call("DEL", KEYS[2])
     return 1
   end
   return 0
   ```

   `KEYS[1] = roomKey(roomID)`, `KEYS[2] = userKey(userID)`, `ARGV[1] = sessionID`, `ARGV[2] = userID-string`。

4. **main.go 钩子 adapter**（`cmd/server/main.go`）传入 `s.SessionID()`：

   ```go
   presenceRepo.RemoveOnline(hookCtx, s.RoomID(), s.UserID(), s.SessionID())
   ```

5. **测试更新**：
   - 既有 case 加 sessionID 参数（`TestPresenceRepo_RemoveOnline_IsOnline_ReturnsFalse` / `TestPresenceRepo_RemoveOnline_NotExists_NoError`）
   - 新增 3 个 case 覆盖关键场景：
     - `TestPresenceRepo_RemoveOnline_ReconnectRace_GuardsNewSession`：oldAdd → newAdd → oldRemove(oldSessionID) 应 no-op，IsOnline / ListOnline / user:{id}:ws_session 全部保留 newSession 状态
     - `TestPresenceRepo_RemoveOnline_MatchingSessionID_ClearsPresence`：sessionID 匹配 → 走删除分支（happy path 反向）
     - `TestPresenceRepo_RemoveOnline_TTLExpiredKey_StillCleansSetMember`：user:{id}:ws_session 提前过期但 room set 还有残留 → script 走 nil 分支仍清残留
   - `ws_test.go` 的 `fakePresenceRepo.RemoveOnline` signature 同步加 sessionID 参数（fake 不做 guard 模拟，集成测试只验证钩子触发次数与时机，guard 行为由 presence_repo_test.go 的 ReconnectRace case 单测覆盖）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计 "key-versioned ephemeral state（如 presence / session token / lock token / leader election lease）的 cleanup API"** 时，**必须**让 cleanup 接口**带 token guard 参数**（如 sessionID / version / fencing token），并用 **Lua script 在 server 端原子 compare-and-delete**（不能拆 pipeline / 不能 client GET-then-CMD），否则 reconnect / takeover / lease-renew 等 race 路径会让旧 owner 的延后 cleanup 误删新 owner 的 state。
>
> **展开**：
> - 如果 ephemeral state 有"被替换（reconnect / takeover）"的可能，cleanup API 的接口契约**必然**需要 token guard —— 这是设计时就该考虑的，不是 review 阶段才补丁。**询问自己**：本 state 是否可能被新 owner 覆盖？如果"是"，cleanup 接口必须带 token。
> - **Redis pipeline 不是 atomic** —— 多命令 pipeline 只是 batched RTT 优化，命令之间仍可被其他 client 命令插入。需要原子语义（compare-and-delete / compare-and-set / 多 key 联动）必走 Lua script（Redis 6.2+ 也支持 ACL `EVAL` / `EVALSHA`；MVP 不必走 EVALSHA cache）。
> - **Lua script 处理 "key 不存在" 分支要谨慎**：严格"key 不存在 → 跳过 cleanup"会让 TTL 提前过期等 corner case 漏清残留；让 nil 走 cleanup 分支兜底（底层 SREM/DEL 对 missing key 是 no-op，幂等安全）通常是对的。但**让 nil 走 cleanup ≠ 让任意 token 都走 cleanup** —— 比较条件必须是 `current == false OR current == ARGV[1]`，**不**能简化成 `current ~= ARGV[2]`（会引入新 bug）。
> - **接口扩展优先于绕过抽象**：当业务 repo 需要新 Redis 能力（Lua / Pipeline / Pub-Sub）而 RedisClient 抽象不暴露时，**扩 RedisClient 接口加方法**（Story 10.2 钦定的渐进式扩展策略），**不**让业务 repo 直接 import go-redis 绕过抽象层。
> - **反例**：
>   - `func (r *presenceRepo) RemoveOnline(ctx, roomID, userID) error { SREM; DEL }` —— 没带 sessionID guard，reconnect 路径必踩坑
>   - `RemoveOnline(ctx, roomID, userID, sessionID) { GET; if v == sessionID { SREM; DEL } }` —— Go 端 check-then-delete 看似正确，实际是 race window 没消除
>   - `Pipeline().GET(uk).SREM(rk, uid).DEL(uk).Exec()` —— pipeline 不是 atomic，仍有 race
>   - `redis.call("GET", KEYS[2]) == ARGV[1]` 单分支判断 —— 漏处理 key 不存在场景，TTL 提前过期路径会留残留 set member
>   - 业务 repo 直接 `c.client.(redisClient).client.Eval(...)` 绕过 RedisClient 抽象 —— 破坏单一边界，未来切 Cluster / 加 Pipeline 时所有绕过点都要重写

---

## Meta: 本次 review 的宏观教训

Story 10.6 r1 是 epic 10 推进过程中**第一次**遇到"业务 repo 需要 RedisClient 没暴露的能力"的场景。处理方式（扩 RedisClient 接口加 Eval）成为后续 epic（Epic 20 idempotency / Epic 32 rate limit）扩接口的模板：**任何业务 repo 不得绕过 RedisClient 抽象拿 raw go-redis client**；需要新能力时按 Story 10.2 钦定的"扩接口而不破抽象"路径推进。这条 meta lesson 与既有 lessons `2026-05-06-go-redis-context-timeout-and-close-idempotency.md` / `2026-05-06-redis-poolsize-negative-makechan-panic.md` 共同形成了"Redis 抽象层完整边界"的三角支撑。
