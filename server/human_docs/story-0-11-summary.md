# Story 0.11: WS 建连频率限流 + 异常设备拒连 — 实现总结

在 WebSocket upgrade 握手完成前插入两段守卫：每用户 60s ≤ 5 次建连的滑动窗口限流、被拉黑设备直接拒。这是 J4 运维旅程的正面堵漏（一次真实事故的教训：凌晨 3 点某客户端 bug 以 100 次/秒 狂连，打满 hub 进程），同时满足 FR41、FR45、NFR-SCALE-5、NFR-SEC-8、NFR-SEC-10 的明确要求。

## 做了什么

### 消费方接口（internal/ws）
- `internal/ws/conn_guard.go` 定义两个独立接口：
  - `Blacklist.IsBlacklisted(ctx, userID)` —— 只读
  - `ConnectRateLimiter.AcquireConnectSlot(ctx, userID) (ConnectDecision, error)` —— 每次建连触发，返回 `{Allowed, Count, RetryAfter}`
- 两个接口故意不合并：`tools/blacklist_user` 只需要写路径，合并会让 CLI 被迫依赖限流器；未来"只限流不黑名单"场景也能独立复用
- `ConnectDecision` 实体放 `pkg/redisx`，`internal/ws` 用 `type ConnectDecision = redisx.ConnectDecision` 别名（同 Story 0.10 `DedupResult` 模式，因为 pkg 不能引用 internal，但接口要用同一个具体类型）

### Redis 实现（pkg/redisx）

**RedisBlacklist（`pkg/redisx/blacklist.go`）**：
- Key `blacklist:device:{userID}` 单段字符串，TTL 可变
- 方法 `IsBlacklisted / Add / Remove / TTL`；`Add(ttl<=0)` 直接 error（禁止永久封禁，保证每条记录都会自动过期）
- `TTL` 返回 `(duration, exists, err)` 三元组，优雅处理 Redis 的 -2/-1 sentinel 值（注意：go-redis v9 把 sentinel 映射为 `time.Duration(-2)` / `time.Duration(-1)`，即 -2ns / -1ns，不是 -2s / -1s）

**RedisConnectRateLimiter（`pkg/redisx/conn_ratelimit.go`）** —— 真滑动窗口（review round 1 纠错产物）：
- Key `ratelimit:ws:{userID}` 是 ZSET，member=`"{ms}:{uuid}"`, score=Unix 毫秒
- 每次建连走一条 Pipeline：
  1. `ZREMRANGEBYSCORE key -inf (cutoff` —— 剔除超窗条目
  2. `ZADD key score=nowMs member=ms:uuid` —— 记录本次
  3. `ZCARD key` —— 窗口内总数
  4. `ZRANGE key 0 0 WITHSCORES` —— 最老条目（用来算 RetryAfter）
  5. `PEXPIRE key window` —— 静默用户自动 GC
- `Count` 不截顶（即便超限也继续累加），审计日志能看到真实的风暴规模
- 构造期校验 `clock != nil / threshold > 0 / window > 0`，否则 panic（生产配置错误必须在启动时炸出来）

### UpgradeHandler 两段守卫（internal/ws/upgrade_handler.go）
- `NewUpgradeHandler` 签名扩展：`(hub, dispatcher, validator, blacklist, rateLimiter)`。nil 允许 = 关闭该 guard（仅供测试 / debug）；生产 `initialize.go` 装非 nil
- 流程在 `ValidateToken` 成功之后、`upgrader.Upgrade` 之前：
  1. blacklist 命中 → `DEVICE_BLACKLISTED`（403 fatal）
  2. 未命中，但 rate limit 超过 → `RATE_LIMIT_EXCEEDED`（429 retry_after，`Retry-After` header 由 `dto.RespondAppError` 自动写入）
  3. 全通过 → 进入原有 upgrade 路径
- **顺序硬性要求**：blacklist 优先级高于 rate limit（fatal 客户端必须清 token 重登，retry_after 只是退避；两个都命中时走 fatal 分支）
- **fail-closed 原则**：任何一侧 Redis 返回错误都直接 `INTERNAL_ERROR` 拒连（不是放行）。Redis 故障期间继续放行会让 J4 事件重现，这是设计上明确反对的

### 审计日志（AC7）
每次拒连都写一条 `info` 级结构化日志，字段 camelCase：
```
{action: "ws_connect_reject", userId, reason: "blacklist" | "ratelimit",
 count?: 6, retryAfterSec?: 42}
```
用 `logx.Ctx(ctx)` 继承 Story 0.5 middleware 注入的 `requestId`，方便用同一 correlation id 穿透排查。敏感字段（displayName / token / 邮箱）禁写。

### 配置（AC5 / AC6）
- `WSCfg` 新增三字段：`ConnectRatePerWindow` / `ConnectRateWindowSec` / `BlacklistDefaultTTLSec`
- `config/default.toml` `[ws]`：`5 / 60 / 86400`（对齐 epics §0.11"默认 24h TTL"、NFR-SCALE-5"60s ≤ 5"）
- `config.applyDefaults()` 先于 `mustValidate()` 填零值，保证 `config/local.toml` 这种只覆盖个别字段的薄配置文件不会因为本 story 新加字段就启动失败（review round 1 纠错产物）

### 运维 CLI（tools/blacklist_user）
- 项目首个 tools 脚本，为后续数据迁移 / apns 清理等建立模式：`main.go` 极薄，`run(args, out, errOut, cfg, cli)` 纯函数便于用 miniredis 测试
- 命令：`add <userId> [ttl]` / `remove <userId>` / `status <userId>`
- `ttl` 接受 Go duration（`24h`），省略取 `cfg.WS.BlacklistDefaultTTLSec`
- 输出：成功 stdout 一行 JSON；错误 stderr + exit 1

## 怎么实现的

**为什么滑动窗口要用 sorted set 而不是 `INCR + EXPIRE NX`**：初版 `INCR + EXPIRE NX` 看起来能工作但只是固定窗口（TTL 锚在第一次 INCR 上），review 正确地指出存在边界绕过：客户端可以在窗口关闭前打满 5 次 + 立刻在窗口 reset 后再打 5 次，瞬时 10 次穿过 60s ≤ 5 的保证。sorted set 按每条记录的时间戳单独老化，任意时刻 ZCARD 就是严格"过去 window 内的次数"，无边界。

**为什么 score 用 Unix 毫秒而不是纳秒**：Redis ZSET score 是 IEEE 754 双精度浮点，只能精确表示到 `2^53 ≈ 9e15`。Unix 纳秒约 `1.7e18` 已经超过，round-trip 会丢几百纳秒精度，导致边界的 `d = ageoutAt - now` 本应为 0 变成伪正 / 伪负。用毫秒（`~1.7e12 < 2^53`）可精确 round-trip，边界条件变成可确定复现的测试场景（review round 2 纠错产物）。

**RetryAfter 的最小等待下限**：ZRemRangeByScore 用 `(cutoff` 排他上界，保留 score == cutoff 的最老条目，此时 `d == 0` 表示该条目正好到期。把这种情况 clamp 到 1ms（经 `ceilSeconds` 向上取 1s），而不是回退到整窗（客户端按 header 退避会被平白多锁 60s）——review round 2 的直接修复。

**两个接口而非一个合并接口**：Go 结构化 typing 自然支持同一个 `RedisBlacklist` 值同时满足 `ws.Blacklist`（只读） 和 tools 直接用具体类型调 `Add/Remove/TTL`（写路径）。在 ws 侧只暴露只读接口满足"最小消费方契约"（P2 单一关注点），写路径留给 CLI / 未来自动拉黑服务。

**fail-closed vs fail-open**：Redis 故障时拒连（fail-closed）直接断开所有用户的 WS 是很扎心的，但这是正确的默认。fail-open（放行）等于在故障期间彻底关闭黑名单和限流，重现 J4 事件。用户在 CLAUDE.md 明确反对"用 backup/fallback 掩盖核心架构风险"，这里是同一个原则的体现。观察面由 Story 0.4 的 `/healthz` 探 Redis 提供——Redis 挂 `/healthz` 降级，运维路径收敛。

**M9 Clock 注入只在需要测时间的位置**：`RedisConnectRateLimiter` 需要 clockx.Clock（score = now），Blacklist 不需要（过期完全交给 Redis TTL）。UpgradeHandler 也不需要新 clock —— 拒连路径不测耗时，审计日志用 zerolog 自带时间戳。`check_time_now.sh` 对 `internal/ws/` 扫描裸 `time.Now()`，新代码干净。

**接口参数 nil 允许 = 关闭该 guard**：把 nil 作为"不检查"语义让单元测试构造 fake 非常灵活（只关心其中一端时可传 nil 给另一端）；生产 `initialize.go` 装配必须传非 nil，否则只是等于移除了本 story 的防护。与 Story 0.10 `NewDispatcher(store, clock)` store=nil 时 RegisterDedup panic 是同一个取舍风格：宽松构造 + 严格运行时。

## 怎么验证的

**单元测试（all `t.Parallel()`，miniredis backing）**：
- `pkg/redisx/blacklist_test.go` —— 8 用例覆盖 add/isBlacklisted 往返 / TTL 正确性 / FastForward 过期 / Remove / Remove 不存在 key / ttl <= 0 拒绝 / TTL(not-exist) 返回 `(0, false, nil)` / key 格式 `blacklist:device:{userID}`
- `pkg/redisx/conn_ratelimit_test.go` —— 9 用例：
  - 5 次 Allowed + 第 6 次 Allowed=false
  - FastForward 61s 后计数重置
  - **滑动窗口无边界绕过**（`TestRedisConnectRateLimiter_SlidingWindow_NoBoundaryBypass`：burst 5 + 推进到 ~60s → 仍 block，证明不是固定窗口）
  - **RetryAfter 精准边界**（`TestRedisConnectRateLimiter_RetryAfterAtExactBoundary_IsSmall`：精心构造 d==0 场景，断言 `RetryAfter < 1s` 不是 60s）
  - RetryAfter 跟最老条目老化追踪
  - 多用户独立计数（并发 alice/bob 互不干扰）
  - 构造期 panic（nil clock / threshold<=0 / window<=0）
- `internal/ws/upgrade_handler_test.go` —— 9 用例用 fake Blacklist/Limiter（`sync/atomic.Int32` 记录调用次数）：
  - 空 token → 401 `AUTH_INVALID_IDENTITY_TOKEN`
  - blacklist 命中 → 403 `DEVICE_BLACKLISTED`，且 limiter.calls == 0（验证优先级）
  - rate limit 超 → 429 `RATE_LIMIT_EXCEEDED` + `Retry-After: 42`
  - 亚秒 RetryAfter（10ms）→ `Retry-After: 1`（`ceilSeconds` 向上取整）
  - blacklist / limiter 两侧 Redis 错误都 → 500 `INTERNAL_ERROR`（fail-closed）
  - nil guards → 不短路（走到 upgrade，因无 WS header 返 400，但关键是不返 403/429/500）
- `tools/blacklist_user/main_test.go` —— 10 用例覆盖 add 显式 ttl / add 默认 ttl / remove / status blacklisted/not / 未知 action / 缺参 / 无效 ttl / zero ttl / 完全无参

**集成测试（`//go:build integration`，每个 subtest 独立 miniredis 实例避免污染）**：
- `TestIntegration_WS_ConnectRateLimit_BlocksSixth` —— 真 httptest.Server 上连续 6 次同 userId，前 5 次 `101 Switching Protocols`，第 6 次 429 + `Retry-After` header 非空
- `TestIntegration_WS_BlacklistBlocksUpgrade` —— `blacklist.Add` → 建连 403 → `blacklist.Remove` → 再建连成功
- `TestIntegration_WS_BlacklistOverridesRateLimit` —— 先打满限流，再拉黑 → 第 N 次返回 403 而非 429（优先级的端到端证据）

**回归**：Story 0.9 的 3 个 integration test + Story 0.10 的 dedup integration test 全绿（`setupTestServer` 同步 `NewUpgradeHandler(..., nil, nil)`）。

**构建**：`bash scripts/build.sh --test` 全绿；`go test -tags=integration ./internal/ws/... ./tools/...` 全绿；`check_time_now.sh` OK（`internal/ws/conn_guard.go` / `upgrade_handler.go` 无裸 `time.Now()`）；`internal/dto` 的 `TestErrorCodesMd_ConsistentWithRegistry` 绿（本 story 未新增 error code，`DEVICE_BLACKLISTED` / `RATE_LIMIT_EXCEEDED` 是 Story 0.6 就注册过的）。

## 后续 story 怎么用

- **Story 0.12（session.resume 缓存 + 节流）**：声明依赖 0.11 的连接限流（见 epics.md line 673）—— resume 在建连后的第一条消息，已经过了本 story 的 upgrade 守卫，不需要额外 per-user 连接限流；节流是另外的"RPC 频率限流"，与本 story 的"建连频率限流"正交
- **Story 1.1（Sign in with Apple + JWT）**：接入真正的 JWT validator 后 `userID` 会变成 `ids.UserID`（Mongo ObjectID hex），格式受控（正则等价 `[a-zA-Z0-9_-]+`），不含 `:`，blacklist/ratelimit key 设计天然兼容。deviceId 作为第二道防御纵深也挂 0.11（见 epics.md line 776），可以复用 `RedisBlacklist` 写路径（由 1.x 的登录风控服务调）
- **Story 1.5（profile.update 权威写）/ Story 5.x（touch.send）/ Story 6.4（blindbox.redeem）**：权威写 RPC 是 Story 0.10 dedup 的主要消费方，本 story 的限流处于 upgrade 阶段，两者正交——客户端即便通过限流建上连，单次 RPC 的重发仍会被 dedup 拦下
- **运维自动化（未来）**：如需实现"一小时内触发 X 次 $ERROR 自动拉黑 24h"的规则，服务端新增一个 `internal/service/risk` 之类的包直接用 `redisx.NewBlacklist(...)` 的写路径即可，不必走 `tools/blacklist_user` CLI
- **tools/ 目录模式**：`tools/blacklist_user` 的结构（`main.go` 调 `run(...)` 然后 `os.Exit`；`run` 是纯函数接 `io.Writer / config / redis.Cmdable`）是后续所有运维脚本的模板，测试友好、无全局状态、强制 `config.MustLoad` 读配置
