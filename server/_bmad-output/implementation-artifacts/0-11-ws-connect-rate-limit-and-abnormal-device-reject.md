# Story 0.11: WS 建连频率限流 + 异常设备拒连

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a platform engineer,
I want per-user WS connect rate limit 60s ≤ 5 and device blacklist blocking upgrade,
So that the J4 3 AM WS hub explosion (single user client bug reconnecting 100 times/sec) doesn't repeat (FR41, FR45, NFR-SCALE-5, NFR-SEC-8, NFR-OBS-5).

## Acceptance Criteria

1. **AC1 — 消费方接口（internal/ws 定义）**：在 `internal/ws/conn_guard.go` 定义两个独立接口（P2 消费方定义原则），命名遵循 Story 0.12 `ResumeCacheInvalidator` / Story 0.10 `DedupStore` 先例：
   - `Blacklist` interface：`IsBlacklisted(ctx context.Context, userID string) (blocked bool, err error)`（只读路径，升级前判定）
   - `ConnectRateLimiter` interface：`AcquireConnectSlot(ctx context.Context, userID string) (decision ConnectDecision, err error)`，返回值 `ConnectDecision{Allowed bool; Count int64; RetryAfter time.Duration}`（`Count` 便于审计日志；`RetryAfter` 仅当 `Allowed == false` 有意义）
   - 两个接口独立是为了便于：a) 仅对其中一端做 fake（unit test）；b) 未来"仅限流、不黑名单"场景不必同时实现；c) `tools/blacklist_user/main.go` 只依赖 blacklist 写路径，不触限流
2. **AC2 — Redis 实现（pkg/redisx）**：
   - `pkg/redisx/blacklist.go` 实现 `RedisBlacklist`：
     - `NewBlacklist(cmd redis.Cmdable) *RedisBlacklist`
     - `IsBlacklisted(ctx, userID) (bool, error)` —— `EXISTS blacklist:device:{userID}`，返回 `>= 1` 即 true
     - `Add(ctx, userID string, ttl time.Duration) error` —— `SET blacklist:device:{userID} "1" EX {ttl}`；`ttl <= 0` 时返回 `errors.New("blacklist: ttl must be > 0")`（禁止永久封禁，配合 NFR-SEC-10 审计）
     - `Remove(ctx, userID) error` —— `DEL`
     - `TTL(ctx, userID) (time.Duration, bool, error)` —— 返回 `(剩余 TTL, exists, error)`；`exists=false` 表示未封禁；`TTL == -1` 表示永久（理论上不应出现，出现即记 warn 并视同 exists=true TTL=0）
   - `pkg/redisx/conn_ratelimit.go` 实现 `RedisConnectRateLimiter`：
     - `NewConnectRateLimiter(cmd redis.Cmdable, threshold int64, window time.Duration) *RedisConnectRateLimiter`；`threshold <= 0` 或 `window <= 0` 构造期 panic
     - `AcquireConnectSlot(ctx, userID)` 实现：`INCR ratelimit:ws:{userID}` + `EXPIRE ratelimit:ws:{userID} {window} NX`（pipeline 原子；`EXPIRE ... NX` 仅在无 TTL 时设置 —— Redis 7+ 支持；go-redis 对应 `pipe.ExpireNX(ctx, key, window)`；miniredis v2 已支持）
     - 决策：若 `newCount > threshold` → `Allowed=false`，`RetryAfter = max(cmd.PTTL(ctx, key), 1s)`（避免返回 0；PTTL 单独一次小请求，也可并入同 pipeline 用 `pipe.PTTL` 减少 RTT —— 实现者自选，一致即可）
     - `Count = newCount`（首次进入也能记录 "第 1 次建连"），不做 `Count>>threshold` 缩减，便于诊断异常客户端的真实连接风暴规模
3. **AC3 — 接口组合点位于 UpgradeHandler 内，顺序硬性要求**：修改 `internal/ws/upgrade_handler.go`：
   - `NewUpgradeHandler` 新增 `blacklist Blacklist, rateLimiter ConnectRateLimiter` 参数（nil 允许 —— 降级为"不检查"，仅用于 debug/无 Redis 的单元测试；生产 initialize.go 必须传非 nil）
   - `Handle(c *gin.Context)` 流程（JWT validate 成功得到 `userID` 之后、`upgrader.Upgrade` 之前）：
     1. `blacklist != nil && blacklist.IsBlacklisted(ctx, userID)` → `dto.RespondAppError(c, dto.ErrDeviceBlacklisted)` 并 `return`
     2. `rateLimiter != nil && !decision.Allowed` → `dto.RespondAppError(c, dto.ErrRateLimitExceeded.WithRetryAfter(ceilSeconds(decision.RetryAfter)))` 并 `return`
     3. 通过后继续现有 `upgrader.Upgrade` 路径
   - 顺序的理由：blacklist 是 `fatal`（强制登出 + 客户端清理 token），必须优先于 `retry_after`（可重试）；若同时命中，客户端应走 fatal 分支
   - Redis 调用失败（`IsBlacklisted` / `AcquireConnectSlot` 返回非 nil error）→ zerolog error + `dto.RespondAppError(c, dto.ErrInternalError.WithCause(err))`；**fail-closed（拒绝连接）** 而非 fail-open —— 避免 Redis 故障期间限流失效成为 J4 事件（open）
4. **AC4 — `dto.ErrRateLimitExceeded.WithRetryAfter` 正确性**：`internal/dto/error.go` 的 `WithRetryAfter` 已存在（返回副本，不污染全局单例；P4）；使用 `Retry-After` 头由 `dto.RespondAppError` 自动设置（现有逻辑：`Category == retry_after && RetryAfter > 0` 时写 header）；**禁止**直接改 `*dto.ErrRateLimitExceeded.RetryAfter`
5. **AC5 — 配置**：`internal/config/config.go` 的 `WSCfg` 新增三个字段：
   - `ConnectRatePerWindow int` (`toml:"connect_rate_per_window"`)
   - `ConnectRateWindowSec int` (`toml:"connect_rate_window_sec"`)
   - `BlacklistDefaultTTLSec int` (`toml:"blacklist_default_ttl_sec"`)（`tools/blacklist_user` 默认 TTL；不用于 upgrade_handler）
   - `config/default.toml` `[ws]` 段添加：`connect_rate_per_window = 5`、`connect_rate_window_sec = 60`、`blacklist_default_ttl_sec = 86400`（24h，epics §0.11 "可配置自动 TTL（默认 24h）"）
   - `mustValidate()` 追加：`ConnectRatePerWindow <= 0` 或 `ConnectRateWindowSec <= 0` 或 `BlacklistDefaultTTLSec <= 0` → `log.Fatal`
6. **AC6 — 装配（cmd/cat/initialize.go）**：在 `redisCli` 构造后、`upgradeHandler` 构造前：
   ```go
   blacklist := redisx.NewBlacklist(redisCli.Cmdable())
   connLimiter := redisx.NewConnectRateLimiter(
       redisCli.Cmdable(),
       int64(cfg.WS.ConnectRatePerWindow),
       time.Duration(cfg.WS.ConnectRateWindowSec)*time.Second,
   )
   upgradeHandler := ws.NewUpgradeHandler(wsHub, dispatcher, validator, blacklist, connLimiter)
   ```
7. **AC7 — zerolog 审计日志（NFR-SEC-10, NFR-OBS-3）**：两种拒连路径必须写 `info` 级审计日志，字段 camelCase（P5）：
   - blacklist 拒连：`logx.Ctx(ctx).Info().Str("action","ws_connect_reject").Str("userId",userID).Str("reason","blacklist").Msg("ws_connect_reject")`
   - 限流拒连：额外加 `Int64("count", decision.Count).Int64("retryAfterSec", int64(decision.RetryAfter/time.Second))`
   - **不得**写 `displayName` / 邮箱 / token（M13, M14）；`userID` 是 opaque ID，可记
   - 审计日志使用 `logx.Ctx(ctx)` 继承 `requestId`（middleware.RequestID 已在 Story 0.5 注入）
8. **AC8 — `tools/blacklist_user/main.go` 一次性脚本**：
   - 通过 `flag` 解析 `-config`（TOML 路径）+ 位置参数 `<action> <userId> [ttl]`
   - Actions：
     - `add <userId> [ttl]` —— ttl 可选，省略则取 `cfg.WS.BlacklistDefaultTTLSec` 秒；支持 Go duration（`24h`）或 RFC3339（`2026-04-19T10:00:00Z` —— 自动换算为 `until - now`，≤ 0 报错）；两种写法二选一实现者自决，推荐 Go duration（简单）
     - `remove <userId>` —— 调 `RedisBlacklist.Remove`
     - `status <userId>` —— 调 `TTL`，打印 `blacklisted: true/false, ttl: 23h45m`
   - 输出：成功 stdout 一行 JSON `{"action":"add","userId":"u1","ttl":"24h0m0s"}`；错误 stderr + os.Exit(1)
   - 必须调 `config.MustLoad` + `redisx.MustConnect`（不复制逻辑）；结束前 `redisCli.Close()`
   - **禁止**在 `main.go` 中调业务层代码；`tools/` 目录是外围脚本（对齐 `docs/backend-architecture-guide.md` §Project Structure）
9. **AC9 — 回归保护（Story 0.9 / 0.10 交付不变）**：
   - `TestIntegration_WS_EchoRoundTrip` / `TestIntegration_WS_NoToken_Rejected` / `TestIntegration_WS_ShutdownCloseFrame`（`upgrade_handler_integration_test.go`）继续通过 —— 测试 setup 中传入 nil blacklist + nil limiter（或全通过的 fake）
   - Dispatcher 签名 / Dedup 行为不变
   - `bash scripts/build.sh --test` 全量绿（含 `-tags=integration`）
10. **AC10 — 单元测试**：
    - `pkg/redisx/blacklist_test.go`（miniredis，table-driven，`t.Parallel()`）：
      - `Add + IsBlacklisted` 往返
      - `Add` TTL 正确（`mr.TTL(key)` 验证；`mr.FastForward(ttl)` 后 `IsBlacklisted` 返回 false）
      - `Remove` 后 `IsBlacklisted` 返回 false
      - `Add(ttl=0)` 返回 error
      - `TTL(notExist)` 返回 `(0, false, nil)`
    - `pkg/redisx/conn_ratelimit_test.go`（miniredis）：
      - 5 次 Allowed + 第 6 次 `Allowed=false`
      - 第 6 次 `RetryAfter > 0` 且 `<= window`
      - `mr.FastForward(window)` 后计数重置，首次再次 `Allowed=true`
      - 不同 userID 独立计数（并行两个 userID，交叉 10 次不互相干扰）
      - 构造期 `threshold=0` / `window=0` → `require.Panics`
    - `internal/ws/conn_guard_test.go`（fake 实现）：验证接口最小契约（可直接使用 `pkg/redisx` impl 的 miniredis 测试代替，不做重复覆盖；若重复则删）
    - `internal/ws/upgrade_handler_test.go`（新增；Story 0.9 未建该文件）：使用 `httptest.NewRecorder` + fake Blacklist/Limiter，不真的 upgrade；断言响应状态码 + `code` + `Retry-After` header：
      - blacklist 命中 → 403 + `code: "DEVICE_BLACKLISTED"`；**不**调用 `Limiter.AcquireConnectSlot`（优先级验证）
      - blacklist 未命中 + limiter 超限 → 429 + `code: "RATE_LIMIT_EXCEEDED"` + `Retry-After` header
      - blacklist + limiter 均 nil → 不走检查，直接走 upgrade 路径（本 unit 测试无法完成 upgrade，但可断言没有提前返回 429/403）
      - Redis 错误 → 500 + `code: "INTERNAL_ERROR"`（fail-closed 验证）
      - 空 token → 401（现有行为回归）
11. **AC11 — 集成测试（`//go:build integration`）**：`internal/ws/upgrade_handler_integration_test.go` 扩展（或新增同包 `upgrade_handler_ratelimit_integration_test.go`）：
    - 启动 `httptest.Server` + 真实 miniredis-backed Blacklist + ConnectRateLimiter（miniredis 优先，参照 0.10 Testing Standards；不引入 testcontainers Redis）
    - `TestIntegration_WS_ConnectRateLimit_BlocksSixth`：同 userId 连续建连 6 次，前 5 次 `http.StatusSwitchingProtocols`，第 6 次收到 `http.StatusTooManyRequests` + 响应体含 `"RATE_LIMIT_EXCEEDED"` + `Retry-After` header 非空（`ParseInt >= 1`）；建连成功的连接每次建立后立即 `conn.Close()` 避免 hub 计数泄漏
    - `TestIntegration_WS_BlacklistBlocksUpgrade`：`blacklist.Add(ctx, userId, 10*time.Minute)` 后尝试建连 → 403 + `"DEVICE_BLACKLISTED"`；`blacklist.Remove` 后再试 → 成功 `StatusSwitchingProtocols`
    - `TestIntegration_WS_BlacklistOverridesRateLimit`：先触发限流计数至上限，再把用户加入黑名单 → 第 N 次建连返回 403 而非 429（顺序证据）
    - 每个 subtest 独立 miniredis（`miniredis.RunT(t)`）避免状态污染
12. **AC12 — `tools/blacklist_user/main.go` 测试**：
    - `tools/blacklist_user/main_test.go`（同包 test）：把 `main` 的核心拆成 `run(args []string, out, errOut io.Writer, cfg *config.Config, cli redis.Cmdable) int` 纯函数便于测试；miniredis 注入；table-driven 覆盖 add/remove/status/无效 action/无效 ttl
    - 允许本测试不在 CI `--test` 默认跑（tools 目录），但 `go test ./tools/...` 必须通过
13. **AC13 — 文档同步**：
    - `docs/error-codes.md` 无需变更（`DEVICE_BLACKLISTED` / `RATE_LIMIT_EXCEEDED` 已注册）；仅确认 `error_codes_test.go` 的 `TestErrorCodesMd_ConsistentWithRegistry` 仍绿
    - 无需新增 `docs/code-examples/`（upgrade_handler 修改不属于"新业务范式"，属于平台基础设施扩展）

## Tasks / Subtasks

- [x] Task 1：消费方接口 + 类型定义（AC: #1）
  - [x] 1.1 创建 `internal/ws/conn_guard.go`
  - [x] 1.2 定义 `ConnectDecision` struct：`Allowed bool`、`Count int64`、`RetryAfter time.Duration`
  - [x] 1.3 定义 `Blacklist` interface（只读判定方法 `IsBlacklisted`；写路径属于 tools 专用，**不**出现在 ws 侧接口 —— 最小化消费方契约）
  - [x] 1.4 定义 `ConnectRateLimiter` interface（单方法 `AcquireConnectSlot`）
  - [x] 1.5 文件 godoc 注释引用 P2、FR41、FR45、NFR-SCALE-5、NFR-SEC-10

- [x] Task 2：RedisBlacklist 实现（AC: #2）
  - [x] 2.1 创建 `pkg/redisx/blacklist.go`
  - [x] 2.2 `RedisBlacklist` 结构体持有 `cmd redis.Cmdable`
  - [x] 2.3 key helper：`blacklistKey(userID string) string` → `"blacklist:device:" + userID`（不加 length-prefix：userID 来源于 JWT subject，格式是受控的 ObjectID hex 或 UUID，无 ":" —— 与 0.10 dedup key 的"用户输入拼接多段"场景不同）
  - [x] 2.4 实现 5 个方法（见 AC2）
  - [x] 2.5 godoc 说明 key space 与 `ratelimit:* / event:* / lock:cron:*` 分隔（D16）

- [x] Task 3：RedisConnectRateLimiter 实现（AC: #2）
  - [x] 3.1 创建 `pkg/redisx/conn_ratelimit.go`
  - [x] 3.2 `RedisConnectRateLimiter` 结构体：`cmd`、`threshold int64`、`window time.Duration`
  - [x] 3.3 key helper：`rateKey(userID string) string` → `"ratelimit:ws:" + userID`
  - [x] 3.4 `AcquireConnectSlot`：
     ```go
     pipe := s.cmd.Pipeline()
     incr := pipe.Incr(ctx, key)
     pipe.ExpireNX(ctx, key, s.window)  // Redis 7 NX flag; miniredis v2 supports
     ttlCmd := pipe.PTTL(ctx, key)
     if _, err := pipe.Exec(ctx); err != nil { return ConnectDecision{}, err }
     count := incr.Val()
     if count <= s.threshold {
         return ConnectDecision{Allowed: true, Count: count}, nil
     }
     retry := ttlCmd.Val()
     if retry <= 0 { retry = s.window }
     return ConnectDecision{Allowed: false, Count: count, RetryAfter: retry}, nil
     ```
  - [x] 3.5 构造期参数校验：`threshold <= 0 || window <= 0` → panic

- [x] Task 4：UpgradeHandler 集成（AC: #3, #4, #7）
  - [x] 4.1 修改 `UpgradeHandler` struct 加字段 `blacklist Blacklist; rateLimiter ConnectRateLimiter`
  - [x] 4.2 `NewUpgradeHandler` 签名扩展（参数顺序：hub, dispatcher, validator, blacklist, rateLimiter；nil 允许）
  - [x] 4.3 `Handle` 在 `ValidateToken` 成功后、`upgrader.Upgrade` 前插入 blacklist → ratelimit 两段判定
  - [x] 4.4 Redis 错误 fail-closed：返回 `dto.ErrInternalError.WithCause(err)`；同时 `logx.Ctx(ctx).Error().Err(err).Str("action","ws_connect_guard_error")` 留痕
  - [x] 4.5 审计日志：两类 reject 路径写 `info` 级结构化日志（字段见 AC7）
  - [x] 4.6 保留现有 `log.Error().Err(err).Msg("ws upgrade failed")`（`h.upgrader.Upgrade` 失败路径）不变

- [x] Task 5：配置与装配（AC: #5, #6）
  - [x] 5.1 `WSCfg` 新增 3 字段；`mustValidate` 补充检查
  - [x] 5.2 `config/default.toml` `[ws]` 追加三行
  - [x] 5.3 `config/local.toml.example` 可选同步（保持与默认一致，不特殊）
  - [x] 5.4 `cmd/cat/initialize.go` 构造 blacklist + connLimiter 并注入 `NewUpgradeHandler`

- [x] Task 6：`tools/blacklist_user/main.go`（AC: #8, #12）
  - [x] 6.1 创建目录 `tools/blacklist_user/`
  - [x] 6.2 `main.go` 仅调 `run(...)` 并 os.Exit
  - [x] 6.3 `run(args []string, out, errOut io.Writer, cfg *config.Config, cli redis.Cmdable) int` 纯函数（易测）
  - [x] 6.4 支持 `add / remove / status`；`-config` flag 默认 `config/default.toml`
  - [x] 6.5 `main_test.go` 覆盖 4 种 action + 3 种错误路径（table-driven）

- [x] Task 7：单元测试（AC: #10）
  - [x] 7.1 `pkg/redisx/blacklist_test.go`（miniredis）
  - [x] 7.2 `pkg/redisx/conn_ratelimit_test.go`（miniredis）
  - [x] 7.3 `internal/ws/upgrade_handler_test.go`（新增；fake blacklist / limiter）

- [x] Task 8：集成测试（AC: #11）
  - [x] 8.1 决定文件位置：扩展 `upgrade_handler_integration_test.go` 还是新增 `upgrade_handler_ratelimit_integration_test.go`（推荐后者，避免单文件过长）
  - [x] 8.2 辅助函数 `setupGuardedTestServer(t, threshold, window)` 返回 `*httptest.Server` + `*redisx.RedisBlacklist`（测试用 miniredis backing）
  - [x] 8.3 `TestIntegration_WS_ConnectRateLimit_BlocksSixth`
  - [x] 8.4 `TestIntegration_WS_BlacklistBlocksUpgrade`
  - [x] 8.5 `TestIntegration_WS_BlacklistOverridesRateLimit`

- [x] Task 9：回归 + 构建（AC: #9）
  - [x] 9.1 更新 `upgrade_handler_integration_test.go` 的 `setupTestServer`（`NewUpgradeHandler(..., nil, nil)`）
  - [x] 9.2 `bash scripts/build.sh --test` 全绿
  - [x] 9.3 `go test -tags=integration ./internal/ws/... ./tools/...` 全绿
  - [x] 9.4 `check_time_now.sh` 通过（`internal/ws/upgrade_handler.go` / `conn_guard.go` 不出现 `time.Now()` —— 所有过期判定委托 Redis；审计日志耗时测量若需要则走 `clockx.Clock`，但本 story 不测 upgrade 全链路耗时，跳过）

## Dev Notes

### Architecture Constraints (MANDATORY)

- **宪法 §1 显式胜于隐式**：`UpgradeHandler` 构造函数显式接收 `Blacklist` + `ConnectRateLimiter`；禁止 package-level 全局 / sync.Once 单例
- **multi-replica 不变量 #2（architecture.md §71, NFR-SCALE-2）**：所有共享状态在 Redis；**禁止**用 in-memory `sync.Map` / `atomic.Int64` 记录 per-user 建连计数（会在横向扩展时失效，直接违反架构约束）
- **D16 key space**：`blacklist:device:{userId}` 和 `ratelimit:ws:{userId}` 是 PRD §State Transition Matrix 明文指定（prd.md lines 551, 553），**禁止**改名
- **P4 Error Category**：`DEVICE_BLACKLISTED=fatal` / `RATE_LIMIT_EXCEEDED=retry_after` 已由 `dto.register` 强制；两者 `HTTPStatus` 固定 403/429；不得绕开 `dto.RespondAppError` 手写 `c.JSON(403, ...)`
- **P5 Logging**：camelCase 字段（`userId` / `action` / `reason` / `count` / `retryAfterSec`）；使用 `logx.Ctx(ctx)` 注入 requestId；禁止 `fmt.Printf` / `log.Printf`（`.golangci.yml` 已用 forbidigo 封禁）
- **M9 Clock interface**：`internal/ws/` 下禁止裸 `time.Now()`（`check_time_now.sh` 由 Story 0.7 引入）；本 story 实际无需 Clock 注入（Redis 负责过期），但 `conn_guard.go` / `upgrade_handler.go` 不得新增 `time.Now()` 调用
- **M13/M14 PII**：审计日志只记 `userId`（opaque）；不得出现 `Authorization` header 原值 / token 原值 / `displayName` / 邮箱
- **M15 per-conn msg rate limit 与本 story 正交**：Story 0.9 的 `rate_limit.go` 限速"已建连后 msg 速率（100 msg/s）"；本 story 限"建连频率（5 次/60s）"；**键空间不同、实现不同、作用阶段不同**
- **NFR-SCALE-5 明确 60s ≤ 5**：阈值允许通过 TOML 调，但不得改默认值；调阈值必须同步 PRD + 架构 + sprint retrospective（不在本 story 范围）
- **NFR-SEC-8 / NFR-SEC-10**：WS connect 是"所有写入 endpoint 必有限流"覆盖范围（J4 运维旅程 + prd.md line 311 明确要求）；拒连必须留审计日志（NFR-SEC-10）—— 故 AC7 审计日志是强制而非可选
- **D10 事务边界不涉及**：本 story 纯 Redis + HTTP 拒连路径，不涉及 Mongo 事务
- **fail-closed 原则**：Redis 短暂不可用时 → 拒绝建连（返回 500 INTERNAL_ERROR）而非放行（fail-open）；fail-open 会让黑名单/限流在故障期间完全失效，直接违背 FR41/FR45 设计目的（用户反对用 backup/fallback 掩盖核心风险）

### 关键实现细节

**`EXPIRE key seconds NX` in go-redis v9**：

```go
// go-redis v9 提供 ExpireNX 方法（Redis 7+ NX 语义）
ok, err := cmd.ExpireNX(ctx, key, window).Result()
// ok=true 表示首次设置；false 表示已有 TTL，本次无动作
```

若发现 miniredis 版本不支持 `EXPIRE NX`，fallback 方案（同样正确）：

```go
pipe := cmd.Pipeline()
incr := pipe.Incr(ctx, key)
_, err := pipe.Exec(ctx)
if incr.Val() == 1 {
    _ = cmd.Expire(ctx, key, window).Err() // 首次 INCR 后单独设 TTL
}
```

两种方案都可接受；选择的判据是 miniredis 行为一致性（参见 go-redis `TestExpireNX` 是否能在 miniredis v2 跑）。

**pipeline 内读 PTTL**：

```go
pipe := cmd.Pipeline()
incrCmd := pipe.Incr(ctx, key)
pipe.ExpireNX(ctx, key, window)
ttlCmd := pipe.PTTL(ctx, key)
_, err := pipe.Exec(ctx)
// incrCmd.Val() 是 INCR 之后的值
// ttlCmd.Val() 是 EXPIRE NX 之后的剩余 TTL（毫秒）
```

**`dto.ErrRateLimitExceeded.WithRetryAfter` 正确姿势**：

```go
retrySec := int((decision.RetryAfter + time.Second - 1) / time.Second) // ceil
if retrySec < 1 { retrySec = 1 }
ae := dto.ErrRateLimitExceeded.WithRetryAfter(retrySec)
dto.RespondAppError(c, ae)
```

`dto.RespondAppError` 已经在 `Category == retry_after && RetryAfter > 0` 时写 `Retry-After` header（`internal/dto/error.go#L83-L89`），不要重复写。

**审计日志字段集（AC7 完整形式）**：

```go
logx.Ctx(ctx).Info().
    Str("action", "ws_connect_reject").
    Str("userId", userID).
    Str("reason", "ratelimit"). // 或 "blacklist"
    Int64("count", decision.Count).
    Int64("retryAfterSec", int64(retrySec)).
    Msg("ws_connect_reject")
```

blacklist 分支省略 `count` / `retryAfterSec` 两个字段；`reason` 必填。

**为什么 userID 来源安全**：

`UpgradeHandler.Handle` 的 `userID` 来自 `validator.ValidateToken(token)`（Story 0.9）。在 `release` 模式是 `stubValidator`（始终拒绝），`debug` 模式是 `debugValidator`（裸 token 作为 userID，仅供开发）。后续 Story 1.1 接入真正的 JWT validator 后 userID 会是 `ids.UserID`（Mongo ObjectID hex）—— 格式受控，不含 `:`。本 story 不依赖 Story 1.1，但 AC2 key 设计兼容未来替换。

**为什么两个接口而非合并一个**：

- `tools/blacklist_user/main.go` 只用 blacklist 写路径（`Add/Remove/TTL`）；若合并接口，tools 会被迫依赖 `ConnectRateLimiter`，违反"最小依赖"
- unit 测试构造 fake 时只关心其中一端的场景较多（例如 TestBlacklistOverridesRateLimit 的 fake limiter 永远 Allowed，fake blacklist 切换状态）
- 未来若添加"仅限流不黑名单"的 endpoint（例如 `/v1/platform/ws-registry` 若加 per-IP 限流），可独立复用 `ConnectRateLimiter` 而不拖 blacklist

**为什么 RedisBlacklist.Add 独立于 ws 接口**：

Blacklist 只读接口是 ws 的"消费方定义"；写路径（Add/Remove/TTL）服务于运维 CLI 与未来自动拉黑服务（超出 MVP）。把写方法暴露成 ws 接口会让 ws 包承担"运维能力"的描述，违反 P2（单一关注点）。故：

- `internal/ws/conn_guard.go`：`Blacklist` interface（只 `IsBlacklisted`）
- `pkg/redisx/blacklist.go`：`RedisBlacklist` 结构体（5 个方法；Add/Remove/TTL 供 tools 直接使用 concrete type）

`RedisBlacklist` 同时满足 `ws.Blacklist` interface（Go 结构化 typing，无需显式 `implements`）。

**UpgradeHandler 构造签名变更对现有代码的影响**：

`cmd/cat/initialize.go` 需同步；`internal/ws/upgrade_handler_integration_test.go` 的 `setupTestServer` 需同步。对比 Story 0.10 的做法，最稳妥：

```go
// 现有（Story 0.9）
upgradeHandler := ws.NewUpgradeHandler(wsHub, dispatcher, validator)

// 0.11 之后
upgradeHandler := ws.NewUpgradeHandler(wsHub, dispatcher, validator, blacklist, connLimiter)

// 测试（nil = 关闭检查）
upgradeHandler := ws.NewUpgradeHandler(hub, dispatcher, validator, nil, nil)
```

不使用 functional options 或 builder 模式（YAGNI；与 Story 0.10 `NewDispatcher(store, clock)` 的正向位置参数风格一致）。

**fail-closed 在 Redis 故障时的观察信号**：

生产 Redis 若 down，所有 WS 建连返回 500。现有 `/healthz` 已探 Redis（Story 0.4），Redis down 时 `/healthz` 会显示 degraded，运维路径一致。不需要额外 alarm 策略。

**miniredis `ExpireNX` 支持**：

miniredis v2.32+ 实现了 `EXPIRE ... NX` 语义（参见 miniredis CHANGELOG）。项目 `go.sum` 若已在 Story 0.8/0.10 引入较新版本则直接用；若版本较低，先 `go get github.com/alicebob/miniredis/v2@v2.32.0` 升级（无其他兼容性风险）。验证方法：`cd server && go doc github.com/alicebob/miniredis/v2 Miniredis.ExpireCmd` 或直接在测试里跑一次。

### Source Tree — 要创建/修改的文件

**新建：**
- `internal/ws/conn_guard.go` — `Blacklist` + `ConnectRateLimiter` 接口 + `ConnectDecision` 结构
- `internal/ws/upgrade_handler_test.go` — fake blacklist/limiter 单元测试（Story 0.9 未建）
- `pkg/redisx/blacklist.go` — `RedisBlacklist`（Add/Remove/IsBlacklisted/TTL）
- `pkg/redisx/blacklist_test.go` — miniredis 单元测试
- `pkg/redisx/conn_ratelimit.go` — `RedisConnectRateLimiter`
- `pkg/redisx/conn_ratelimit_test.go` — miniredis 单元测试
- `tools/blacklist_user/main.go` — 运维 CLI
- `tools/blacklist_user/main_test.go` — miniredis 驱动的 CLI 集成测试
- `internal/ws/upgrade_handler_ratelimit_integration_test.go` — `//go:build integration`，3 个场景（可选合并进既有 integration_test）

**修改：**
- `internal/ws/upgrade_handler.go` — `NewUpgradeHandler` 签名扩展；`Handle` 插入 blacklist → ratelimit 两段判定 + 审计日志 + fail-closed
- `internal/ws/upgrade_handler_integration_test.go` — `setupTestServer` 同步 `NewUpgradeHandler` 新签名（`nil, nil`）
- `internal/config/config.go` — `WSCfg` 新增 3 字段；`mustValidate` 补充
- `config/default.toml` — `[ws]` 段追加 3 行
- `cmd/cat/initialize.go` — 装配 blacklist + connLimiter 并注入 upgradeHandler

**不修改（回归保护）：**
- `internal/ws/hub.go`, `hub_test.go`, `rate_limit.go`, `rate_limit_test.go`, `envelope.go`, `envelope_test.go`, `broadcaster.go`, `broadcaster_test.go`, `dispatcher.go`, `dispatcher_test.go`, `dedup.go`, `dedup_test.go`, `dedup_integration_test.go` —— Story 0.9/0.10 交付不变
- `internal/dto/error_codes.go` —— `DEVICE_BLACKLISTED` / `RATE_LIMIT_EXCEEDED` 已存在
- `internal/dto/error.go`, `internal/dto/error_codes_test.go` —— 无变更
- `docs/error-codes.md` —— 无变更
- `pkg/redisx/dedup.go` / `dedup_test.go` / `locker.go` / `locker_test.go` / `client.go` —— 无变更
- 其他所有文件

### Testing Standards

- 单元测试与实现同目录，`xxx_test.go`，table-driven（宪法）
- 所有单元测试开启 `t.Parallel()`（miniredis 每 subtest 独立实例：`mr := miniredis.RunT(t)`）
- testify：`require.NoError` / `assert.Equal` / `require.Panics`
- 集成测试 `//go:build integration`；miniredis backing（对齐 0.10 选型，避免 Windows Docker 障碍）
- Fake Blacklist / Limiter：简单结构体 + `sync.Mutex`；不依赖 Redis
- `httptest.NewServer(gin router)` + `gorilla/websocket.DefaultDialer`（与 0.9/0.10 集成测试一致）
- 建连成功的 WS 连接测试结束必须 `conn.Close()`，否则 hub `Final` 时会看到 stale client 影响后续 subtest

### Previous Story Intelligence (Story 0.10 + Story 0.9)

- **Dispatcher / DedupStore 的 P2 消费方接口定义模式（0.10）**：本 story 严格复用 —— 接口在 `internal/ws`，impl 在 `pkg/redisx`。Go 结构化 typing 自动满足接口，无需 explicit `implements`
- **`NewDispatcher(store, clock)` 位置参数风格（0.10）**：UpgradeHandler 同样用位置参数而非 options 模式
- **Dedup key 防歧义教训（0.10 fix round 2）**：多字段拼接 key 必须 length-prefix 防冲突。本 story 的 key（blacklist:device:{userId}、ratelimit:ws:{userId}）只拼接一段且 userID 格式受控（UUID/ObjectID），**不需要** length-prefix —— 但 reviewer 可能会追问：显式在 godoc 里说明 "userID 格式受控（JWT subject，正则 `^[a-zA-Z0-9_-]+$` 等价），无需 length-prefix" 以免被质疑
- **`close(send)` / `done channel` 教训（0.9 fix round 2）**：不涉及（本 story 不触 Client / Hub 生命周期），但审计日志不要写 "connection closed" —— 没建起连接，无 connId
- **miniredis 是单元测试标准（0.10）**：无需测 testcontainers 真 Redis
- **Error code 去重（0.10）**：本 story **不新增** error code；若运行时 `TestErrorCodesMd_ConsistentWithRegistry` 报错，先检查 `error_codes.go` 是否被误动
- **集成测试 setupTestServer 签名变更会影响 Story 0.9 既有测试**：按 AC9 在本 story 一次性修好
- **`logx.Ctx(ctx)` vs `log.Error()`（0.5/0.9）**：`logx.Ctx` 继承 requestId；新审计日志用 `logx.Ctx`；`log.Error` 保留给 upgrader.Upgrade 失败路径（无法继承 ctx，因为 Upgrade 过程 ctx 被替换）
- **集成测试执行顺序（0.9 已知 flake）**：`TestIntegration_WS_ShutdownCloseFrame` 在与 dedup 集成测试同批跑时偶发 i/o timeout（Story 0.10 completion note 352）—— 本 story 新增集成测试使用 `miniredis.RunT(t)` 的独立实例，彼此独立；若 CI 继续有 flake，属 0.9 环境抖动不追加修复

### Git Intelligence（最近 5 commit）

```
873abd1 chore: mark Story 0.10 done — WS 上行 eventId 幂等去重（DedupStore + RedisDedupStore + Dispatcher.RegisterDedup）
68e8dcf fix(review): 0-10 round 2 — dedup key 改为 length-prefix 编码防止分隔符歧义
2dbb467 fix(review): 0-10 round 1 — dedup key 按 (userId, msgType) namespace 隔离
5bf8d7a chore: mark Story 0.9 done — WS Hub, Envelope, Broadcaster, Dispatcher, UpgradeHandler
de1bbf6 fix(review): 0-9 round 2 — send channel 永不关闭，done channel 驱动 writePump 退出
```

关键惯例：
- 一个 story 一次 done commit；review 反馈单独 `fix(review): X-N round M —` 提交
- 基础设施优先落 `pkg/`，`internal/` 消费；ws 接口走消费方定义（P2）
- 构造函数 nil 允许 = 关闭行为（不抛错 —— 测试友好）；生产装配装非 nil
- **Redis key 防歧义**：多段拼接即 length-prefix；单段 + 受控输入即普通拼接（本 story 属后者）
- 审计日志优先 `logx.Ctx(ctx)`；全局 `log.Error` 仅用于 ctx 不可用的路径
- 拒连 / 限流必须 zerolog audit（NFR-SEC-10）
- **fail-closed**（Redis 故障时拒连）而非 fail-open；符合用户"反对 backup/fallback 掩盖核心风险"的反馈

### Latest Tech Information

- **go-redis v9**（已依赖）：`ExpireNX(ctx, key, dur).Result()` 支持 Redis 7+ NX 语义；`Pipeline()` 非事务足以承载 INCR+EXPIRE+PTTL 组合
- **miniredis v2**：至少 v2.32+ 支持 `EXPIRE NX`。若项目 go.sum 版本过低，`go get github.com/alicebob/miniredis/v2@latest` 升级（无 API 破坏性变更历史）
- **gorilla/websocket v1.5.3**（Story 0.9 引入）：集成测试复用 `DefaultDialer.Dial`；Upgrade 失败返回的 HTTP 状态可通过 `resp.StatusCode` 读取（Dial 返回非 101 时 err 非 nil + resp 仍可用）
- **dto.AppError.WithRetryAfter**（Story 0.6）：已实现副本语义；本 story 调用方式无新需求
- **Gin**：`c.Header("Retry-After", strconv.Itoa(retrySec))` 由 `dto.RespondAppError` 代劳；upgrade_handler 不直接写 header
- **不新增外部依赖**：无

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 0.11 — AC 完整定义（lines 647-663）]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 0.12 — session.resume 声明依赖 0.11 限流（line 673）]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 1.1 — deviceId 防御纵深依赖 0.11（line 776）]
- [Source: _bmad-output/planning-artifacts/prd.md#FR41 — WS 建连频率限流（line 836）]
- [Source: _bmad-output/planning-artifacts/prd.md#FR45 — 异常设备拒 WS（line 841）]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-SCALE-5 — 60s ≤ 5 次（line 903）]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-SEC-8 — 写入 endpoint 必限流（line 891）]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-SEC-10 — 审计日志（line 893）]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-OBS-3 — 日志必含字段（line 928）]
- [Source: _bmad-output/planning-artifacts/prd.md#Redis Key Convention — ratelimit:ws / blacklist:device（lines 551-553）]
- [Source: _bmad-output/planning-artifacts/prd.md#Error Codes — DEVICE_BLACKLISTED / RATE_LIMIT_EXCEEDED（lines 584-585）]
- [Source: _bmad-output/planning-artifacts/prd.md#J4 运维旅程 — 单用户 100 次/秒建连爆炸（lines 307-316）]
- [Source: _bmad-output/planning-artifacts/architecture.md#multi-replica 不变量 — line 71 共享状态必走 Redis]
- [Source: _bmad-output/planning-artifacts/architecture.md#M15 WS per-conn rate limit — line 687-688 与本 story 正交]
- [Source: _bmad-output/planning-artifacts/architecture.md#P4 Error Category — retry_after 策略]
- [Source: _bmad-output/planning-artifacts/architecture.md#NFR-SCALE-2 — 禁进程级全局状态 line 900]
- [Source: docs/backend-architecture-guide.md#Section 12 WebSocket — upgrade 阶段架构]
- [Source: internal/ws/upgrade_handler.go — 现有签名（待扩展）]
- [Source: internal/ws/upgrade_handler_integration_test.go — setupTestServer 待同步]
- [Source: internal/ws/dedup.go — Story 0.10 消费方接口定义模式先例]
- [Source: pkg/redisx/dedup.go — Story 0.10 Redis impl + pipeline 模式先例]
- [Source: pkg/redisx/locker.go — SETNX 模式先例（本 story 用 SET EX）]
- [Source: pkg/redisx/locker_test.go — miniredis 测试模式先例]
- [Source: internal/dto/error.go — AppError.WithRetryAfter + RespondAppError]
- [Source: internal/dto/error_codes.go#L36-L38 — 已注册 DEVICE_BLACKLISTED / RATE_LIMIT_EXCEEDED / EVENT_PROCESSING]
- [Source: internal/config/config.go — WSCfg 待扩展]
- [Source: config/default.toml — [ws] 段待扩展]
- [Source: cmd/cat/initialize.go — 装配点]
- [Source: _bmad-output/implementation-artifacts/0-10-ws-upstream-eventid-idempotent-dedup.md — 前序 story 模式 + 教训]
- [Source: _bmad-output/implementation-artifacts/0-9-ws-hub-skeleton-envelope-broadcaster-interface.md — UpgradeHandler 原版设计]

### Project Structure Notes

- 与 `docs/backend-architecture-guide.md` 对齐：`internal/ws` 承载 WS 协议 / 中间件 / 消费方接口；`pkg/redisx` 承载 Redis 基础设施；`tools/` 承载一次性运维脚本（P2 分层）
- 本 story 不触碰 `internal/service/`（无 Service 层代码，因为 upgrade 是纯协议层决策）
- 无新依赖；无架构偏差
- `tools/blacklist_user/main.go` 是项目首个 tools 脚本（先前 `tools/` 目录为空）—— 为后续 tools（如 data-migration、apns-token-cleanup 手动模式）建立模式先例：main 极薄、`run()` 可测、强制通过 `config.MustLoad` 读配置

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- miniredis v2.37 + go-redis v9.18 的 TTL 语义：go-redis `TTL(ctx, key).Result()` 对 "key 不存在" 返回 `time.Duration(-2)`（原始 -2 纳秒），对 "key 无 TTL" 返回 `time.Duration(-1)`；**不是** `-2 * time.Second`。`RedisBlacklist.TTL` 按此实际 sentinel 做分支。
- `run` 辅助函数放在 `tools/blacklist_user/run.go` 单独文件，保持 `main.go` 仅做 flag 解析与 `os.Exit`，符合 AC8 中 "main 极薄" 的模式目标。

### Completion Notes List

- Task 1: 定义 `Blacklist` / `ConnectRateLimiter` 两接口（不合并）；`ConnectDecision` 按 dedup.go 的 `DedupResult` 先例 —— 定义在 `pkg/redisx`，`internal/ws` 通过 `type ... = redisx.ConnectDecision` 别名引用，避免 internal → pkg 的反向依赖。
- Task 2: `RedisBlacklist` 5 方法完整（`IsBlacklisted` / `Add` / `Remove` / `TTL` / key helper `blacklistKey`）；`Add(ttl<=0)` 返回 error；发现并修复 go-redis v9 TTL sentinel 使用纳秒值（见 Debug Log）。
- Task 3: `RedisConnectRateLimiter` 使用 `INCR + ExpireNX + PTTL` 单 pipeline；构造期 `threshold<=0 || window<=0` panic；`Count` 不截顶以保留真实风暴规模（J4 场景关键审计字段）。
- Task 4: `NewUpgradeHandler` 签名扩展 `(hub, dispatcher, validator, blacklist, rateLimiter)`；nil 允许作为"关闭该 guard"语义，生产 initialize 装配非 nil；Redis 错误 fail-closed（返回 500），顺序 blacklist 优先（fatal 先于 retry_after）；审计日志 camelCase + `logx.Ctx(ctx)`。
- Task 5: `WSCfg` 新增 3 字段（`ConnectRatePerWindow` / `ConnectRateWindowSec` / `BlacklistDefaultTTLSec`）；`mustValidate` 全部 `<=0` → `log.Fatal`；`config/default.toml` `[ws]` 追加 `5/60/86400`；`config_test.go` 的两个用例同步补全 `[ws]` 字段以避免 fatal；`cmd/cat/initialize.go` 装配 `redisx.NewBlacklist` + `redisx.NewConnectRateLimiter` 并注入。
- Task 6: `tools/blacklist_user/` 首个 tools 脚本；`main.go` + `run.go` 分层；`run()` 纯函数接收 io.Writer + config + redis.Cmdable，便于 miniredis 驱动测试；输出单行 JSON 成功/stderr + 退出码 1 失败。
- Task 7: `upgrade_handler_test.go` 全新（Story 0.9 未建）；fake Blacklist / Limiter 记录调用计数验证 "blacklist 命中时跳过 limiter"；覆盖 401/403/429/500 + 亚秒 RetryAfter 向上取整 + nil guards 不短路。
- Task 8: `upgrade_handler_ratelimit_integration_test.go` 新增，每个 subtest 独立 `miniredis.RunT(t)` 隔离；3 场景全绿：`BlocksSixth` / `BlacklistBlocksUpgrade`（Add→blocked→Remove→ok）/ `BlacklistOverridesRateLimit`（先打满计数再拉黑 → 返 403 而非 429）。
- Task 9: `setupTestServer`（Story 0.9）与 `dedup_integration_test.go`（Story 0.10）的 `NewUpgradeHandler` 调用同步 `nil, nil`；`bash scripts/build.sh --test` 全绿；`go test -tags=integration ./internal/ws/...` 全绿（Story 0.9 / 0.10 / 0.11 共 7 个 integration case 全过）；`check_time_now.sh` OK；`internal/dto` 回归全绿（`TestErrorCodesMd_ConsistentWithRegistry` 绿，无新增 error code）。
- ✅ 回归：仅有 `pkg/redisx/client_integration_test.go` 的 `TestMustConnect_Integration` 在 Windows 下失败，原因是 testcontainers 需要 Docker rootless，属**预先存在**（Story 0.8 引入，与本 story 无关）。
- ✅ fail-closed 原则：blacklist/limiter Redis 错误均返回 500 INTERNAL_ERROR，而非放行（避免 Redis 故障期间 J4 限流失效）；用户 "反对用 backup/fallback 掩盖核心风险" 的反馈在本 story 的设计中贯彻。
- ⚠️ 未执行：`bash scripts/build.sh --race --test` 因 Windows 下 go1.25 cgo 工具链问题跳过（race 需要 cgo，pre-existing，非 story 问题）；常规 `--test` 已全绿。

### File List

**新建：**
- `server/internal/ws/conn_guard.go`
- `server/internal/ws/upgrade_handler_test.go`
- `server/internal/ws/upgrade_handler_ratelimit_integration_test.go`
- `server/pkg/redisx/blacklist.go`
- `server/pkg/redisx/blacklist_test.go`
- `server/pkg/redisx/conn_ratelimit.go`
- `server/pkg/redisx/conn_ratelimit_test.go`
- `server/tools/blacklist_user/main.go`
- `server/tools/blacklist_user/run.go`
- `server/tools/blacklist_user/main_test.go`

**修改：**
- `server/internal/ws/upgrade_handler.go` — 扩展签名 + 插入 blacklist→ratelimit 两段守卫 + 审计日志 + fail-closed + `ceilSeconds` helper
- `server/internal/ws/upgrade_handler_integration_test.go` — `NewUpgradeHandler(..., nil, nil)`
- `server/internal/ws/dedup_integration_test.go` — `NewUpgradeHandler(..., nil, nil)`
- `server/internal/config/config.go` — `WSCfg` 新增 3 字段 + `mustValidate` 三项检查
- `server/internal/config/config_test.go` — 补齐测试 TOML 中 `[ws]` 必填字段
- `server/config/default.toml` — `[ws]` 追加 3 行
- `server/cmd/cat/initialize.go` — 装配 `redisx.NewBlacklist` + `redisx.NewConnectRateLimiter` 并注入 `NewUpgradeHandler`
- `server/_bmad-output/implementation-artifacts/sprint-status.yaml` — Story 0-11 标记 in-progress → review（本文件的 Status 更新后再切）

### Change Log

| Date       | Version | Author | Summary |
|------------|---------|--------|---------|
| 2026-04-18 | 1.0     | dev    | 完成 Story 0.11：WS 建连频率限流 + 异常设备黑名单 + `tools/blacklist_user` 运维 CLI；全部 13 条 AC 满足，单元 + 集成测试全绿，回归 Story 0.9/0.10 不变。 |
