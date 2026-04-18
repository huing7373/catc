# Story 0.12: session.resume 缓存节流

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a platform engineer,
I want same-user repeat `session.resume` calls within 60 seconds to return a cached payload instead of re-querying Mongo / providers,
So that the J4 "Watch 重连风暴" can't exhaust Mongo/Redis connection pools even when Story 0.11 的建连限流放行了前 5 次（FR42, NFR-PERF-3, NFR-PERF-6, NFR-OBS-5）.

## Acceptance Criteria

1. **AC1 — 消费方接口（`internal/ws/session_resume.go`，P2 消费方定义原则）**：

   - `ResumeCache` interface（WS 内部 handler 消费；**读写同一份 Redis Hash**）：
     ```go
     type ResumeCache interface {
         Get(ctx context.Context, userID string) (payload ResumeSnapshot, found bool, err error)
         Put(ctx context.Context, userID string, payload ResumeSnapshot) error
     }
     ```
   - `ResumeCacheInvalidator` interface（Service 层后续 story 消费）：
     ```go
     type ResumeCacheInvalidator interface {
         Invalidate(ctx context.Context, userID string) error
     }
     ```
   - 两接口独立：`ResumeCache` 只给 WS handler 用（读写对称），`ResumeCacheInvalidator` 给 service 用（只需单一 Invalidate 动词）；这样未来若把 invalidator 替换为 Kafka/CDC 驱动也不拖累 handler。沿用 0.10 / 0.11 先例（`DedupStore` / `Blacklist` / `ConnectRateLimiter` 均定义在 `internal/ws`）。
   - `ResumeSnapshot` 结构体也在 `internal/ws/session_resume.go` 声明（见 AC4）；作为 skeleton，字段全部用 `json.RawMessage` — 未来 Story 4.5 / Epic 1/2/3/6/7 各自决定具体 schema 时无需改动 0.12 的 struct。

2. **AC2 — Redis 实现（`pkg/redisx/resume_cache.go`）**：

   - `RedisResumeCache` 持 `cmd redis.Cmdable` + `ttl time.Duration` + `clock clockx.Clock`；**同一结构体同时满足 `ws.ResumeCache` 与 `ws.ResumeCacheInvalidator`**（Go 结构化 typing，参照 0.10 `RedisDedupStore` 先例）。
   - `NewResumeCache(cmd redis.Cmdable, clock clockx.Clock, ttl time.Duration) *RedisResumeCache`；`clock == nil` 或 `ttl <= 0` → `panic`（构造期防御，对齐 `NewDedupStore` 风格）。
   - Key helper `resumeCacheKey(userID string) string` → `"resume_cache:" + userID`；单段拼接，userID 格式受控（JWT sub），**不需要** length-prefix（与 0.11 blacklist 同理，但 godoc 必须写明与 0.10 dedup 多段拼接 key 的区别）。
   - `Get` 实现：`HGETALL resume_cache:{userID}` → 无条目返回 `found=false`；有条目按 AC4 字段集反序列化；**HGETALL 错误**直接返回 err（调用方 fail-open 见 AC5）。
   - `Put` 实现：pipeline 内做 `HSET` + `EXPIRE ttl`；字段集见 AC4；pipeline `Exec` 失败返回 err（调用方 fail-open）。**不得**用 `SET`（epics 明确指定 Hash 结构 — 为后续 Story 4.5 复用以及 Redis CLI 人工巡检友好）。
   - `Invalidate` 实现：`DEL resume_cache:{userID}`；key 不存在时 DEL 返回 0（视为成功，不报错）。
   - Key space 注释（godoc）必须列出与 `event:* / event_result:* / lock:cron:* / ratelimit:* / blacklist:* / presence:* / state:*` 的隔离关系（D16 — PRD §Redis Key Convention lines 547-554）。

3. **AC3 — Handler 注册（`internal/ws/session_resume.go` + 装配点）**：

   - `SessionResumeHandler` 结构体持 `cache ResumeCache` + 六个 `Provider` interface（见 AC4）+ `clock clockx.Clock`；构造 `NewSessionResumeHandler(cache, clock, userP, friendsP, catStateP, skinsP, blindboxesP, roomP)`；**任一参数 nil → 构造期 panic**（生产装配必须齐备；skeleton 阶段用 Empty 实现填充，不得留 nil）。
   - `(h *SessionResumeHandler) Handle(ctx, client, env)` 是 `ws.HandlerFunc` 签名，满足 `Dispatcher.Register("session.resume", h.Handle)`（**走 `Register` 而非 `RegisterDedup`** — session.resume 是幂等读操作，见 AC7）。
   - `cmd/cat/initialize.go` 新增装配：`redisx.NewResumeCache(cmd, clk, ttl)` → `EmptyUserProvider{}` 等 Empty 实现 → `ws.NewSessionResumeHandler(...)` → `dispatcher.Register("session.resume", h.Handle)`；装配位置在 `NewDispatcher` 之后、`NewUpgradeHandler` 之前；变量命名风格与 `blacklist` / `connLimiter` 对齐。

4. **AC4 — 响应 payload 形状 + Provider 接口集**：

   - 响应 envelope 遵循 `NewAckResponse`，`Type == "session.resume.result"`（`dispatcher.go` 已自动追加 `.result`，**handler 不得**手动拼 `.result`）。
   - Payload 结构（`ResumeSnapshot`）：
     ```go
     type ResumeSnapshot struct {
         User         json.RawMessage `json:"user"`          // EmptyUserProvider 返回 "null"
         Friends      json.RawMessage `json:"friends"`       // EmptyFriendsProvider 返回 "[]"
         CatState     json.RawMessage `json:"catState"`      // EmptyCatStateProvider 返回 "null"
         Skins        json.RawMessage `json:"skins"`         // EmptySkinsProvider 返回 "[]"
         Blindboxes   json.RawMessage `json:"blindboxes"`    // EmptyBlindboxesProvider 返回 "[]"
         RoomSnapshot json.RawMessage `json:"roomSnapshot"`  // EmptyRoomSnapshotProvider 返回 "null"
         ServerTime   string          `json:"serverTime"`    // 每次响应都用 Clock.Now() 重新生成（RFC3339Nano），不缓存
     }
     ```
   - 六个 Provider interface（**定义在 `internal/ws/session_resume.go`**；消费方定义；字节级 `json.RawMessage` 规避 Epic 0 阶段 domain 未建的耦合）：
     ```go
     type UserProvider interface {
         GetUser(ctx context.Context, userID string) (json.RawMessage, error)
     }
     type FriendsProvider interface {
         ListFriends(ctx context.Context, userID string) (json.RawMessage, error)
     }
     type CatStateProvider interface {
         GetCatState(ctx context.Context, userID string) (json.RawMessage, error)
     }
     type SkinsProvider interface {
         ListUnlocked(ctx context.Context, userID string) (json.RawMessage, error)
     }
     type BlindboxesProvider interface {
         ListActive(ctx context.Context, userID string) (json.RawMessage, error)
     }
     type RoomSnapshotProvider interface {
         GetRoomSnapshot(ctx context.Context, userID string) (json.RawMessage, error)
     }
     ```
   - 每个 Provider 在 `session_resume.go` 同文件内给 Empty 实现（`EmptyUserProvider` 等），返回 `json.RawMessage("null")` 或 `json.RawMessage("[]")`；Epic 1 / 2 / 3 / 6 / 7 各 story 后续接入真实实现时，只需 `initialize.go` 替换具体 provider 变量。这是 Story 4.5 line 1334 "Service 层 SessionResumeService 接受 Provider interface（消费方定义）；Epic 7/6 实现时 initialize.go 替换真实实现" 先例的 0.12 版落地。
   - Redis Hash 字段编码（对应 `RedisResumeCache.Put` 展开写入）：
     ```
     HSET resume_cache:{userID} user <raw> friends <raw> catState <raw> skins <raw> blindboxes <raw> roomSnapshot <raw>
     EXPIRE resume_cache:{userID} 60
     ```
     **不写 `serverTime` 到 Hash**：每次 `Get` 后 handler 用 `clock.Now()` 重新生成 serverTime 再序列化回响应 —— serverTime 的语义是"响应发出时的服务端时钟"，缓存命中时应反映当前时间而非写入时间；若缓存了 serverTime，客户端会读到 60s 前的时钟，时钟偏差计算失真。

5. **AC5 — 故障策略：fail-open for cache, fail-closed for provider**：

   - **Cache Get 错误** → 记 `logx.Ctx(ctx).Warn().Err(err).Str("action","resume_cache_get_error")` → 继续走 provider 流程（fail-open：cache 是性能优化，Redis 故障不得阻塞业务）。
   - **Cache Put 错误** → 记 `Warn` → 响应已经构造好仍返回客户端（fail-open：用户不应因缓存写失败收到 500）。
   - **Provider 错误** → 返回 `dto.ErrInternalError.WithCause(err)`；dispatcher 会把它转为 envelope `{ok:false, error:{code:"INTERNAL_ERROR", ...}}`（数据完整性优先：若 friend 列表查询失败，给客户端空 friends 是误导）。
   - **Invalidator 错误**（未来 Service story 关心）→ 返回 err，由 Service 层自行决定是否 surface 到客户端；典型做法：service 记 warn 并继续返回成功（权威写已 commit，缓存一致性在 TTL=60s 后自愈）。
   - 与 0.11 `blacklist / rateLimiter` 的 **fail-closed** 对比：0.11 是安全关，失败必须拒连否则复现 J4；0.12 是性能关，失败允许降级。Dev Notes 必须写明区别 + 用户"反对 backup 掩盖核心风险"反馈不适用于本 story（resume cache 失败不等于"掩盖核心故障"，Mongo/Provider 错误仍会正确 surface）。

6. **AC6 — 配置（`internal/config/config.go`）**：

   - `WSCfg` 新增 `ResumeCacheTTLSec int \`toml:"resume_cache_ttl_sec"\``。
   - `config/default.toml` `[ws]` 段追加 `resume_cache_ttl_sec = 60`（NFR-PERF-6 60 秒窗口）。
   - `applyDefaults`: `c.WS.ResumeCacheTTLSec == 0 → 60`（与 0.11 round 1 修复里 override config 兼容策略一致 — `963c737` 提交的 `applyDefaults` 模式必须沿用）。
   - `mustValidate`: `c.WS.ResumeCacheTTLSec <= 0 → log.Fatal`（显式 `-1` 禁用是误用，应该直接去掉整个 handler 注册而非读一个 TTL=负数 的 Redis）。
   - 测试：`internal/config/config_test.go` 的两个 TOML fixture 若被 0.11 已补 `[ws]` 字段，本 story 只追加 `resume_cache_ttl_sec = 60` 一行即可；若测试断言 struct 字段，追加对应断言。

7. **AC7 — session.resume 不走 dedup**：

   - `dispatcher.Register("session.resume", handler.Handle)`，**不**调 `RegisterDedup`。
   - 理由：session.resume 是幂等读操作（对同一 userID 在 60s 内多次调用返回同一快照），语义本身就是"重复不产生副作用"，无需 eventId 去重保护；epics line 679 明文"`session.resume` 本身不走 eventId dedup"；非权威读消息清单见 epics line 642 `session.resume` 列入。
   - 副作用：客户端可以不带 `envelope.id` 或带任意 id；handler 不得校验 id 非空（dedup 中间件才校验）；响应 envelope 的 `id` 直接回显 envelope.id（dispatcher 默认行为）。

8. **AC8 — Handler 业务流程**：

   ```
   Handle(ctx, client, env):
     userID := client.UserID()
     snapshot, found, err := h.cache.Get(ctx, userID)
     if err != nil { log warn "resume_cache_get_error"; found = false }  // fail-open
     if !found {
         // 并行或串行调 6 个 Provider（skeleton 阶段可串行，日志含 durationMs）
         user, err := h.userP.GetUser(ctx, userID);        if err { return ErrInternalError.WithCause(err) }
         friends, err := h.friendsP.ListFriends(ctx, userID);  if err { ... }
         catState, err := h.catStateP.GetCatState(ctx, userID); if err { ... }
         skins, err := h.skinsP.ListUnlocked(ctx, userID);      if err { ... }
         blindboxes, err := h.blindboxesP.ListActive(ctx, userID); if err { ... }
         roomSnapshot, err := h.roomP.GetRoomSnapshot(ctx, userID); if err { ... }
         snapshot = ResumeSnapshot{User: user, Friends: friends, CatState: catState,
                                   Skins: skins, Blindboxes: blindboxes, RoomSnapshot: roomSnapshot}
         if err := h.cache.Put(ctx, userID, snapshot); err != nil { log warn "resume_cache_put_error" }  // fail-open
     }
     snapshot.ServerTime = h.clock.Now().UTC().Format(time.RFC3339Nano)  // 每次都新鲜，无论命中与否
     return json.Marshal(snapshot)
   ```
   - Provider 调用**允许串行**（Epic 0 全部 Empty，耗时忽略）；Story 4.5 若发现 20 好友 p95 ≤ 500ms 有压力，可在那里切 `errgroup` 并行，不是本 story 决策范围。
   - `client.UserID()` 已由 Story 0.9 暴露（Hub.go line 114），直接用。
   - 不得使用 `time.Now()`（M9 — `check_time_now.sh` 在 `internal/ws/` 强制 `clock.Now()`；参考 0.11 AC9.4）。

9. **AC9 — zerolog 审计日志（NFR-OBS-3 / NFR-OBS-5）**：

   - 每次成功响应写 `info` 级：
     ```go
     logx.Ctx(ctx).Info().
         Str("action", "session_resume").
         Str("connId", string(client.ConnID())).
         Str("userId", string(client.UserID())).
         Bool("cacheHit", cacheHit).
         Int64("durationMs", durationMs).
         Msg("session_resume")
     ```
   - 字段 camelCase（P5）；`userId` 是 opaque，可记；不得记 token / displayName / 邮箱（M13 / M14）。
   - `session.resume` QPS 是 NFR-OBS-5 核心指标之一 → 上述日志即指标源，后续 Loki / Grafana 查询靠 `action="session_resume"` + `cacheHit` 布尔字段切面。

10. **AC10 — 单元测试（`internal/ws/session_resume_test.go` — 新建）**：
    - 使用 fake 各 Provider（简单结构体 + counter 字段），fake cache（`map[string]ResumeSnapshot` + `sync.Mutex`）。
    - 用 `pkg/clockx.NewFakeClock` 作为 Clock 注入（Story 0.7 已提供）。
    - table-driven + `t.Parallel()`；testify `require.NoError` / `assert.Equal`。
    - 必覆盖场景：
      a) **首次 resume**：`cache.Get` 返回 not found → 6 个 Provider 各被调 1 次 → `cache.Put` 被调 1 次 → 响应含预期字段 + serverTime 等于 Clock.Now()
      b) **缓存命中**：cache 预置一条 → 第二次 `Handle` 调用 → **Provider 全 0 次调用** → 响应与首次一致但 serverTime 已变（Clock advance 1s 后验证）
      c) **显式失效后重查**：首次 → `cache.Invalidate` → 第二次 → Provider 再次全部调一次
      d) **Cache Get 错误 fail-open**：fake cache.Get 返回 err → Provider 仍被调 → 响应成功（不 propagate err）
      e) **Cache Put 错误 fail-open**：fake cache.Put 返回 err → 响应仍成功
      f) **Provider 错误 fail-closed**：friends provider 返 err → 响应 error envelope（`INTERNAL_ERROR`），其他 Provider **不必全部调用**（但至少 friendsP 被调一次；不断言未调用的 Provider 调用次数 — 避免与实现细节耦合）
      g) **Empty provider 产出合法 JSON**：单独一测验证 `EmptyUserProvider` 返回 `json.RawMessage("null")`、`EmptyFriendsProvider` 返回 `json.RawMessage("[]")` 等 — 确保序列化到最终 payload 后是 `"user": null, "friends": [], ...`
      h) **构造器 nil 参数 panic**：`require.Panics` 覆盖任一 nil 参数传入 `NewSessionResumeHandler`

11. **AC11 — RedisResumeCache 单元测试（`pkg/redisx/resume_cache_test.go` — 新建，miniredis 驱动）**：
    - `miniredis.RunT(t)` + `clockx.NewFakeClock` + `t.Parallel()`。
    - 场景：
      a) `Put` 后 `Get` 往返返回同一 snapshot（字段级 `assert.JSONEq`）
      b) `Put` 后 Redis 端 `mr.TTL(key)` 恰为配置 TTL（如 60s）
      c) `mr.FastForward(ttl)` 后 `Get` 返回 `found=false`
      d) `Get` 不存在的 userID → `found=false, err=nil`
      e) `Invalidate` 后 `Get` 返回 `found=false`；`Invalidate` 不存在的 key → 不报错
      f) 构造期 `ttl<=0` → `require.Panics`；构造期 `clock==nil` → `require.Panics`
      g) 不同 userID 独立存储（并行两个 userID，读取不互相干扰 — 验证 key namespace 正确）

12. **AC12 — 集成测试（`internal/ws/session_resume_integration_test.go` — 新建，`//go:build integration`）**：
    - miniredis.RunT + 真实 `RedisResumeCache` + 真实 `Dispatcher` + counter-based 假 Provider（每个 Provider 一个 `callCount` 字段）；`httptest.Server` + `gorilla/websocket.DefaultDialer` 建 WS（对齐 0.10 / 0.11 集成测试 setup）；`gin` router 复用 `buildRouter` 或直接 mount `ws.UpgradeHandler`。
    - 测试矩阵：
      a) **`TestIntegration_SessionResume_Benchmark_10Calls_OneProviderHit`**：同 userID 连续发 10 条 `session.resume` envelope（可在同一 WS 连接或不同连接；推荐同一连接简化）→ 断言 **每个 counter provider 的 callCount == 1**（仅首次触发） + 10 次响应 envelope.ok 均为 true + payload 一致
      b) **`TestIntegration_SessionResume_InvalidateRefetches`**：发 resume → Provider callCount == 1 → 直接从外部（测试内）持 `ResumeCacheInvalidator` 调 `Invalidate(userID)` → 再发 resume → Provider callCount == 2
      c) **`TestIntegration_SessionResume_NotDeduped`**：**同 envelope.id 连发两次** session.resume → 第二次 **不**被 dedup middleware 拦截（应正常返回结果，非 `EVENT_PROCESSING` / 缓存结果来自 dedup）；这条测试是验证 AC7 dispatcher.Register 路径 — 若误用 RegisterDedup 会被捕获
    - 建连成功的连接测试结束前 `conn.Close()`（对齐 0.11 integration test convention — 避免 hub 计数残留）。
    - **benchmark 断言口径**：epics AC 原文"连续 10 次 resume 中仅第 1 次触发 Mongo query"，Epic 0 阶段 Mongo 尚未参与 session.resume（domain 未建），故替换为"仅第 1 次触发 Provider 调用" — Dev Notes 注明该替换是合理的（Mongo 查询是 Provider 的下层实现，Provider 调用次数是同一性质的上层指标）。

13. **AC13 — 回归保护（Stories 0.9 / 0.10 / 0.11）**：

    - `TestIntegration_WS_EchoRoundTrip` / `TestIntegration_WS_NoToken_Rejected` / `TestIntegration_WS_ShutdownCloseFrame` / `TestIntegration_WS_DedupFirst_Then_Replay` / `TestIntegration_WS_ConnectRateLimit_BlocksSixth` / `TestIntegration_WS_BlacklistBlocksUpgrade` / `TestIntegration_WS_BlacklistOverridesRateLimit` 全部继续通过。
    - `NewDispatcher` / `NewUpgradeHandler` 签名不变（本 story 不触碰）。
    - `bash scripts/build.sh --test` 全绿；`go test -tags=integration ./internal/ws/...` 全绿。
    - `check_time_now.sh`（Story 0.7）：`session_resume.go` 与 `resume_cache.go` **不得出现裸 `time.Now()`**；所有时间通过 `clock.Now()`。
    - `TestErrorCodesMd_ConsistentWithRegistry` 绿（本 story **不新增** error code，复用 `ErrInternalError`）。

14. **AC14 — 文档同步**：

    - `docs/error-codes.md` 无需变更。
    - `docs/code-examples/` 可选新增 `resume_cache_invalidator_example.go`（展示未来 Service story 的典型消费模式 `inv.Invalidate(ctx, userID)`），注释掉实际代码保持与现有 examples 风格一致；若实现者判断 skeleton 阶段示例无价值，可省略并写进 Completion Notes 说明原因。
    - 本 story 的 `session_resume.go` / `resume_cache.go` godoc 必须：
      - 引用 FR42, NFR-PERF-3, NFR-PERF-6, NFR-OBS-5, D16, P2
      - 说明 key space `resume_cache:{userId}` 与其他 D16 key prefix 的隔离
      - 解释 fail-open 决策与 0.11 fail-closed 对比

## Tasks / Subtasks

- [x] Task 1：消费方接口与数据结构（AC: #1, #4）
  - [x] 1.1 创建 `internal/ws/session_resume.go`
  - [x] 1.2 定义 `ResumeSnapshot` struct（7 字段，json.RawMessage + serverTime string）—— 实际定义在 `pkg/redisx/resume_cache.go`，`internal/ws` 通过 `type ResumeSnapshot = redisx.ResumeSnapshot` 别名引用（与 0.10 `DedupResult` / 0.11 `ConnectDecision` 先例一致，解决 pkg→internal 反向依赖）
  - [x] 1.3 定义 `ResumeCache` / `ResumeCacheInvalidator` 两个 interface
  - [x] 1.4 定义 6 个 Provider interface（UserProvider / FriendsProvider / CatStateProvider / SkinsProvider / BlindboxesProvider / RoomSnapshotProvider）
  - [x] 1.5 同文件内给 6 个 Empty*Provider 实现（返回 `null` / `[]` 字面量，无 I/O 无状态）
  - [x] 1.6 文件 godoc 引用 FR42 / NFR-PERF-3 / NFR-PERF-6 / P2 / D16

- [x] Task 2：RedisResumeCache 实现（AC: #2）
  - [x] 2.1 创建 `pkg/redisx/resume_cache.go`
  - [x] 2.2 `RedisResumeCache` 结构（cmd / clock / ttl）+ `NewResumeCache` 构造函数 + nil/ttl 校验 panic
  - [x] 2.3 key helper `resumeCacheKey(userID) = "resume_cache:" + userID`；godoc 写明与 0.10 多段 dedup key 的差异
  - [x] 2.4 `Get` → HGETALL + 反序列化到 `ResumeSnapshot`；空 → `found=false`
  - [x] 2.5 `Put` → pipeline(HSET 六字段 + EXPIRE ttl)
  - [x] 2.6 `Invalidate` → DEL key
  - [x] 2.7 godoc 列出 D16 key space 隔离

- [x] Task 3：SessionResumeHandler 实现（AC: #3, #4, #5, #8, #9）
  - [x] 3.1 `SessionResumeHandler` struct（cache + 6 providers + clock）—— 6 个 Provider 聚合进 `ResumeProviders` struct，handler 只持单一 `providers` 字段（比 8 位置参构造器更易扩展）
  - [x] 3.2 `NewSessionResumeHandler(...)` 构造器，nil 校验 panic（生产装配必须齐备）
  - [x] 3.3 `Handle` 方法实现 AC8 业务流程
  - [x] 3.4 fail-open 策略实现（cache 错 warn + 降级 provider；cache write 错 warn + 照常响应）
  - [x] 3.5 fail-closed 策略实现（provider 错返回 `ErrInternalError.WithCause`）
  - [x] 3.6 zerolog 成功日志（AC9 字段集），使用 `logx.Ctx(ctx)` 继承 requestId
  - [x] 3.7 serverTime 用 `clock.Now().UTC().Format(time.RFC3339Nano)` 每次新鲜生成

- [x] Task 4：配置扩展（AC: #6）
  - [x] 4.1 `WSCfg` 新增 `ResumeCacheTTLSec int` + toml tag
  - [x] 4.2 `applyDefaults` 追加 `ResumeCacheTTLSec == 0 → 60`
  - [x] 4.3 `mustValidate` 追加 `ResumeCacheTTLSec <= 0 → log.Fatal`
  - [x] 4.4 `config/default.toml` `[ws]` 段追加 `resume_cache_ttl_sec = 60`
  - [x] 4.5 `config/local.toml.example` 若存在则同步（保持与 default 一致）—— 仓库未维护该文件，跳过
  - [x] 4.6 `internal/config/config_test.go` 的 TOML fixture 补齐新字段（`TestMustLoad_OverrideWithoutWSSection` 追加 `assert.Equal(t, 60, cfg.WS.ResumeCacheTTLSec)`）

- [x] Task 5：装配（`cmd/cat/initialize.go`）（AC: #3）
  - [x] 5.1 构造 `redisx.NewResumeCache` + `ws.NewSessionResumeHandler` + `dispatcher.Register("session.resume", ...)`
  - [x] 5.2 位置在 `NewDispatcher` 之后、`NewUpgradeHandler` 之前；变量命名 `resumeCache` / `sessionResumeHandler` 与现有 `blacklist` / `connLimiter` 同风格
  - [x] 5.3 `resumeCache` 未暴露到 `App`（future Service story 通过构造参数注入）

- [x] Task 6：handler 单元测试（AC: #10）
  - [x] 6.1 `internal/ws/session_resume_test.go` 新建
  - [x] 6.2 fake ResumeCache / 6 个 fake Provider（带 counter）
  - [x] 6.3 场景 a-h 全部覆盖（8 个 top-level 测试 + 8 个 nil-panic 子测试 = 16 PASS）；`t.Parallel()` + table-driven nil panic 矩阵
  - [x] 6.4 使用 `clockx.NewFakeClock` 验证 serverTime = Clock.Now()（`CacheHitSkipsProviders` 在 Clock.Advance(5s) 后断言 serverTime 是 advanced 时间，证明 serverTime 不来自缓存）

- [x] Task 7：RedisResumeCache 单元测试（AC: #11）
  - [x] 7.1 `pkg/redisx/resume_cache_test.go` 新建
  - [x] 7.2 miniredis 驱动；`miniredis.RunT(t)` 每子测独立实例
  - [x] 7.3 场景 a-g 全部覆盖（8 PASS）
  - [x] 7.4 TTL 验证使用 `mr.TTL(key)` + `mr.FastForward(61s)` 触发 Get miss

- [x] Task 8：集成测试（AC: #12）
  - [x] 8.1 `internal/ws/session_resume_integration_test.go` 新建，`//go:build integration`
  - [x] 8.2 辅助函数 `setupResumeServer(t)` 返回 `*resumeTestServer` 聚合 srv / mr / cache / 6 × atomic counter；参考 `setupDedupServer` 模式
  - [x] 8.3 `TestIntegration_SessionResume_Benchmark_10Calls_OneProviderHit` PASS（10 次 resume，6 个 provider 各 1 次调用）
  - [x] 8.4 `TestIntegration_SessionResume_InvalidateRefetches` PASS
  - [x] 8.5 `TestIntegration_SessionResume_NotDeduped` PASS（同 envelope.id `same-id` 两次，中间 Invalidate，两次都 OK=true 且 provider 累计 2 次 → dedup 未介入）

- [x] Task 9：回归 + 构建（AC: #13）
  - [x] 9.1 `bash scripts/build.sh --test` 全绿
  - [x] 9.2 `go test -tags=integration ./internal/ws/...` 全绿（0.9 + 0.10 + 0.11 + 0.12 全过）；`./pkg/redisx/...` 仅 `TestMustConnect_Integration` 因 Windows 无 rootless Docker 失败 —— Story 0.3 预先存在问题，0-11 Completion Notes 已登记，与本 story 无关
  - [x] 9.3 `check_time_now.sh` OK（脚本输出：`no direct time.Now() calls in business code`）
  - [x] 9.4 `internal/dto/error_codes_test.go::TestErrorCodesMd_ConsistentWithRegistry` 绿

- [x] Task 10：文档 godoc（AC: #14）
  - [x] 10.1 `session_resume.go` 文件级 godoc 含 FR42 / NFR-PERF-3 / NFR-PERF-6 / NFR-OBS-5 / P2 / D16 引用 + Redis key space 隔离 + fail-open vs 0.11 fail-closed 对比
  - [x] 10.2 `resume_cache.go` 文件级 godoc 含 D16 key space（与 event / lock / ratelimit / blacklist / presence / state / refresh_blacklist 隔离）+ ServerTime 不缓存原因
  - [x] 10.3 `docs/code-examples/resume_cache_invalidator_example.go` —— 未新增：invalidator 消费模式与现有 `docs/code-examples/` 已有的 ws / service / handler / cron / repository 示例性质不同（是"单行调用"不是业务范式），单独文件反而分散注意力；留待 Story 2.2 / 1.5 等首个实际消费方落地时再决定是否抽样板

## Dev Notes

### Architecture Constraints (MANDATORY)

- **P2 消费方定义接口**：`ResumeCache` / `ResumeCacheInvalidator` / 6 个 Provider 全部定义在 `internal/ws/session_resume.go`（被 handler 消费），Redis impl 在 `pkg/redisx/resume_cache.go`；Service 层 future story 将 `ResumeCacheInvalidator` import 进 service 包（接口本身仍在 ws 包，pkg 依赖方向 `internal/service → internal/ws → pkg/redisx` 合规）。参考 0.10 `DedupStore` / 0.11 `Blacklist,ConnectRateLimiter` 先例。
- **multi-replica 不变量 #2（architecture.md §71, NFR-SCALE-2）**：缓存写 Redis，**禁止** in-memory `sync.Map[userID]ResumeSnapshot`；多实例下 in-memory 会让一个副本的缓存永远拿不到另一个副本的 invalidation，违反架构约束。
- **D16 Redis key space**：`resume_cache:{userId}` 与 `event:* / event_result:* / lock:cron:* / ratelimit:ws:* / blacklist:device:* / presence:* / state:* / refresh_blacklist:*` 严格隔离（PRD lines 547-554）；**不得**改名。
- **P4 Error Category**：session.resume 失败走 `ErrInternalError`（已注册，`CategoryRetryable` / 500）；**不得**为 resume 场景新增 `SESSION_RESUME_FAILED` 这类细粒度 code（epics AC13 明示无新 code；错误码爆炸是 P2 坏味道之一）。
- **P5 Logging**：camelCase（`userId` / `connId` / `action` / `cacheHit` / `durationMs`）；`logx.Ctx(ctx)` 继承 `requestId`；禁止 `fmt.Printf` / `log.Printf`（`.golangci.yml` forbidigo）。
- **M9 Clock interface**：`internal/ws/*` 禁裸 `time.Now()`；本 story 的 serverTime + durationMs + cache TTL 计算全部走 `clock.Now()`；`check_time_now.sh` 会卡。
- **M13 / M14 PII**：审计日志只记 `userId`（opaque）；**不得**记 token、Authorization header、displayName、邮箱、Apple identity token、device token。
- **M15 per-conn msg rate limit 与本 story 正交**：Story 0.9 的 `rate_limit.go` 限"已建连后单连接 100 msg/s"，保护的是 readPump；本 story 限"同用户 60s 内 session.resume payload 重建"，保护的是 Mongo/Provider；两者并存。
- **D10 事务边界不涉及**：本 story 纯 Redis + provider 调用，无 Mongo 写，不需要 `mongox.WithTransaction`。
- **fail-open 原则（本 story 专属；与 0.11 fail-closed 区分）**：resume cache 是**性能**关，Redis 故障时降级到 Provider 仍能正确响应，没有"掩盖 J4 级根因风险"（provider 错误仍会 fail-closed 上报）；用户"反对 backup/fallback 掩盖核心架构风险"的反馈在 0.11 适用（那是安全关），在 0.12 不适用（否则会让 Redis 短暂抖动就拒绝所有 session.resume，影响产品体验）。Dev agent 若犹豫请回看本条。

### 关键实现细节

**ResumeSnapshot 字段为何全部 `json.RawMessage`**：

Epic 0 阶段，`users` / `cat_states` / `friends` / `skins` / `blindboxes` / `rooms` 的 domain struct 尚未建立（各自 Epic 1/2/3/6/7 + Story 4.2 才会建），如果 0.12 强行定义具体 struct 字段会产生两个问题：

1. 耦合 — 未来 Epic 各 story 修改 struct 时需要回改 ws/session_resume.go；
2. 序列化歧义 — 具体 struct 的 `omitempty` / `null` 语义取舍应由各 domain story 决定（例如：`RoomSnapshot` 为 nil 是返回 `null` 还是省略字段？）。

改用 `json.RawMessage` 让每个 Provider 自己决定 JSON 形状；0.12 只管"有 6 个字节块，塞进 payload"。当 Story 4.5 与 Epic 1/2/3/6/7 各自接入真 Provider 时，它们自己 marshal 成 `json.RawMessage` 返回即可，skeleton 结构不变。

**为何 serverTime 不进 Hash**：

serverTime 语义是"响应发出时的服务端时钟"，客户端用它做时钟偏差估计（Story 4.5 line 1331）。缓存 60s 意味着第 59s 的命中会读到 59s 前的 serverTime，客户端算出 `clientTime - serverTime ≈ 59s`，误判为时钟严重偏差。所以每次响应（无论命中）都用当前 `clock.Now()` 重新填。Hash 只存 6 个内容字段。

**Hash vs String 权衡**：

epics 明确用 Hash。技术动机（即使 0.12 只做全量 invalidation）：

1. Redis CLI 人工巡检时 `HGETALL resume_cache:u1` 能看到字段分组，而不是一坨 JSON blob；
2. 给未来差分协议（OP-4，Patterns 阶段决定）留扩展空间 —— 若未来允许 per-section invalidation（例如 `HDEL resume_cache:{userId} friends`），structure 已就绪；
3. 内存略省（Redis Hash 小字段有 ziplist 编码优化）。

**fail-open for cache read 的具体代码形状**：

```go
snapshot, found, err := h.cache.Get(ctx, userID)
if err != nil {
    logx.Ctx(ctx).Warn().Err(err).
        Str("action", "resume_cache_get_error").
        Str("userId", userID).
        Msg("resume cache get failed, falling through to providers")
    found = false
}
if !found {
    // ... providers ...
}
```

注意：`found=false` **不区分** "cache miss" 与 "cache error"；下游逻辑都是"重建 payload"。这是刻意的：调用方不关心原因，只关心"要不要调 provider"。若未来需要指标切分（命中率 vs 错误率），靠 `action="resume_cache_get_error"` 日志计数即可（NFR-OBS-5）。

**Provider nil 构造期 panic 的理由**：

0.11 `NewUpgradeHandler` 允许 nil blacklist / rateLimiter（"关闭该 guard" 语义，给测试/debug 方便）。0.12 不允许 nil Provider：若 nil，handler 调用时会 segfault，没有合理的"关闭该 Provider"语义（响应中的字段必须存在，否则客户端反序列化可能出错）。Empty*Provider 就是"关闭"语义的正确表达 —— 返回 `null` 或 `[]`，客户端反序列化成功。

**为何六个 Provider 分开 interface 而不是一个 `ResumeDataSource` 合并**：

- 每个 Provider 对应一个未来的独立 Story（User→1.1、Friends→3.4、CatState→2.2、Skins→7.2、Blindboxes→6.3、RoomSnapshot→4.2）；分开便于渐进式接入：Story 7.2 实施时只需把 `EmptySkinsProvider` 替换为 `RealSkinsProvider`，其他五个保持 Empty。
- 合并成一个接口意味着每次替换一个 Source 就要改实现类（或者每个 Source 都实现整个接口），违反 ISP（接口隔离原则）。
- Story 4.5 line 1334 明示这是正确方向：`SkinsProvider` / `BlindboxesProvider` 独立定义。

**dispatcher.Register vs RegisterDedup（AC7 再展开）**：

`RegisterDedup` 会：
1. 校验 `envelope.id` 非空，否则返回 `VALIDATION_ERROR "envelope.id required"`（dedup.go line 62-66）
2. 对 `(userID, msgType, eventID)` 做幂等锁（Redis SETNX）
3. 第二次同 eventID 返回缓存结果或 `EVENT_PROCESSING`

对于 session.resume 来说，这三项行为**都是错误的**：
1. 客户端重连产生新 connection，envelope.id 可能从 "1" 重来；不应强制要求非空 id
2. 两次 session.resume 在 60s 内本来就该被 resume_cache 处理（hit 命中缓存），不该被 eventId dedup 拦截返回 `EVENT_PROCESSING`
3. resume 是读操作，幂等天然成立；加 dedup 是语义冗余 + 行为不符

所以必须 `dispatcher.Register("session.resume", ...)`。AC12.c（`TestIntegration_SessionResume_NotDeduped`）就是用来在 CI 层面保证这一点不被未来误改。

**Service 层如何消费 ResumeCacheInvalidator（为未来 story 定盘）**：

Story 1.5 / 2.2 / 3.2 / 6.4 / 7.3 将在 service 层持一个 `invalidator ws.ResumeCacheInvalidator` 字段（接口在 ws 包，被 service 消费 — 合规，`internal/service` 可以 import `internal/ws`）：

```go
// 以 Story 2.2 state_service.go 为例（未来）
type StateService struct {
    repo        CatStateRepo
    broadcaster StateBroadcaster
    invalidator ws.ResumeCacheInvalidator
    clock       clockx.Clock
}

func (s *StateService) Apply(ctx, userID, tick) error {
    // ... 业务写入 ...
    if err := s.invalidator.Invalidate(ctx, userID); err != nil {
        log.Warn().Err(err).Str("userId", userID).Msg("resume_cache_invalidate_failed")
        // 不 return err —— 权威写已 commit，缓存 60s 后自愈
    }
    return nil
}
```

0.12 不实现这段代码（未来 service story 各自实现），但 `ResumeCacheInvalidator` 接口形状必须兼容这种用法。

### Source Tree — 要创建/修改的文件

**新建：**
- `internal/ws/session_resume.go` — 接口 + handler + Empty Providers（6 个）+ ResumeSnapshot struct
- `internal/ws/session_resume_test.go` — handler 单元测试（fake cache + fake providers + FakeClock）
- `internal/ws/session_resume_integration_test.go` — `//go:build integration`，3 个场景（miniredis-backed RedisResumeCache + counter Providers）
- `pkg/redisx/resume_cache.go` — `RedisResumeCache`（Get/Put/Invalidate + key helper）
- `pkg/redisx/resume_cache_test.go` — miniredis 单元测试（7 场景）

**修改：**
- `cmd/cat/initialize.go` — 装配 `RedisResumeCache` + `SessionResumeHandler` + `dispatcher.Register("session.resume", ...)`
- `internal/config/config.go` — `WSCfg` 新增 `ResumeCacheTTLSec`；`applyDefaults` 追加默认 60；`mustValidate` 追加 <=0 校验
- `config/default.toml` — `[ws]` 段追加 `resume_cache_ttl_sec = 60`
- `internal/config/config_test.go` — 测试 TOML fixture 追加字段（若有 struct 字段断言则同步）
- `config/local.toml.example`（若存在）— 同步 `[ws].resume_cache_ttl_sec = 60`

**不修改（回归保护）：**
- `internal/ws/hub.go`, `hub_test.go`, `rate_limit.go`, `rate_limit_test.go`, `envelope.go`, `envelope_test.go`, `broadcaster.go`, `broadcaster_test.go`, `dispatcher.go`, `dispatcher_test.go`, `dedup.go`, `dedup_test.go`, `dedup_integration_test.go`, `upgrade_handler.go`, `upgrade_handler_test.go`, `upgrade_handler_integration_test.go`, `upgrade_handler_ratelimit_integration_test.go`, `conn_guard.go` — Story 0.9 / 0.10 / 0.11 交付不变
- `internal/dto/error_codes.go` / `error.go` / `error_codes_test.go` — 无新增 code
- `pkg/redisx/blacklist.go`, `blacklist_test.go`, `conn_ratelimit.go`, `conn_ratelimit_test.go`, `dedup.go`, `dedup_test.go`, `locker.go`, `locker_test.go`, `client.go` — 无变更
- `docs/error-codes.md` — 无变更
- 其他所有文件

### Testing Standards

- 单元测试与实现同目录，`xxx_test.go`，table-driven（宪法 §15.5 + P2 经验）
- 所有单元测试 `t.Parallel()`；miniredis 每 subtest 独立实例 `miniredis.RunT(t)`
- testify：`require.NoError` / `assert.Equal` / `assert.JSONEq`（用于 `json.RawMessage` 比对）/ `require.Panics`
- 集成测试 `//go:build integration`；miniredis backing（对齐 0.10 / 0.11 选型，避免 Windows Docker 障碍）
- fake ResumeCache：简单 `map[string]ResumeSnapshot` + `sync.Mutex`；fake Provider：counter struct 记录调用次数
- `clockx.NewFakeClock`（Story 0.7）注入；验证 serverTime 相等性用 Clock 的确定性值
- `httptest.NewServer(gin router)` + `gorilla/websocket.DefaultDialer`（与 0.9 / 0.10 / 0.11 集成测试一致）
- 成功建连的 WS 测试结束前显式 `conn.Close()`，避免 hub 计数污染后续 subtest

### Previous Story Intelligence（Story 0.11 + 0.10 + 0.9）

- **接口分层先例（0.11）**：`Blacklist` / `ConnectRateLimiter` 各自独立而非合并；本 story 同理 `ResumeCache` ≠ `ResumeCacheInvalidator` ≠ 6 个 Provider。消费者差异即接口差异。
- **applyDefaults override 兼容教训（0.11 fix round 1，commit `6724015`）**：`config_test.go` 或生产 `local.toml` 若只写部分 `[ws]` 字段，缺失字段会走 applyDefaults 而非 `log.Fatal`；本 story 新增 `ResumeCacheTTLSec` 必须同时补 applyDefaults 分支（`== 0 → 60`），否则现有 `[ws]` section 缺本字段的配置启动会炸。参考 `internal/config/config.go` line 109-119 已有模式。
- **位置参数风格（0.10）**：`NewDispatcher(store, clock)` / `NewUpgradeHandler(hub, dispatcher, validator, blacklist, rateLimiter)` 都是正向位置参数，不用 options pattern（YAGNI）；本 story `NewSessionResumeHandler` / `NewResumeCache` 同风格。
- **`logx.Ctx(ctx)` vs `log.Error`（0.5 / 0.9 / 0.11）**：审计日志用 `logx.Ctx(ctx)` 继承 requestId；全局 `log.Error` 仅给 ctx 不可用路径（本 story 无此场景，全部 `logx.Ctx`）。
- **miniredis 作为单元测试默认后端（0.8 / 0.10 / 0.11）**：无需 testcontainers；版本 v2.32+ 足够本 story（用 HGETALL / HSET / EXPIRE / TTL / DEL，均为基础命令）。
- **集成测试 setup helper 复用（0.9 → 0.11 → 本 story）**：0.11 新建的 `setupGuardedTestServer` 构造了 `NewUpgradeHandler(..., blacklist, limiter)` 路径；本 story 需要额外注册 session.resume handler，可以基于 0.11 helper 扩展为 `setupResumeTestServer(t)`（miniredis + RedisResumeCache + counter Providers + dispatcher.Register）；**不要**把所有 session_resume 测试塞回已有 `setupTestServer`，新 helper 独立可读。
- **Error code 去重教训（0.10）**：不要为 resume 失败新增 code；直接复用 `ErrInternalError`。`TestErrorCodesMd_ConsistentWithRegistry` 若失败说明误加了 code。
- **测试清理（0.9 `close(send)` round 2 教训）**：本 story 不触碰 Client 生命周期，但集成测试建连后必须 `conn.Close()`，避免与其他 integration test 并跑时 hub `Final` 扫到残留 client 导致 flake。
- **审计字段使用现有 camelCase 公约（P5）**：`userId` / `connId` / `cacheHit` / `durationMs` / `action`；**不用** `user_id` 等 snake_case。

### Git Intelligence（最近 5 commit）

```
537d750 chore: mark Story 0.11 done — WS 建连频率限流 + 异常设备黑名单
963c737 fix(review): 0-11 round 2 — 边界 Retry-After 回退修复；ZSET score 改 Unix ms
6724015 fix(review): 0-11 round 1 — 配置 applyDefaults 兼容现有 override；rate limit 改真滑动窗口
873abd1 chore: mark Story 0.10 done — WS 上行 eventId 幂等去重（DedupStore + RedisDedupStore + Dispatcher.RegisterDedup）
68e8dcf fix(review): 0-10 round 2 — dedup key 改为 length-prefix 编码防止分隔符歧义
```

关键惯例：

- 一个 story 一次 done commit；review 反馈另提 `fix(review): X-N round M —` 提交
- 基础设施优先落 `pkg/`，`internal/` 消费；ws 接口走消费方定义（P2）
- 构造函数参数非 nil 是缺省；仅在有明确"关闭该 guard"语义时允许 nil（如 0.11 `UpgradeHandler`）；本 story `NewSessionResumeHandler` **不**开放 nil（Empty*Provider 是"关闭"的正确表达）
- Redis key 防歧义：多段拼接 → length-prefix（如 0.10 dedup）；单段 + 受控 userID → 普通拼接（如 0.11 blacklist、本 story resume_cache）
- 审计日志优先 `logx.Ctx(ctx)`；全局 `log.Error` 仅用于 ctx 不可用路径
- fail-closed（0.11 WS upgrade guard）是**安全关**的标准；fail-open（本 story resume cache）是**性能关**的标准；两者不是矛盾而是场景差异，实现前想清楚该 story 属于哪类
- Redis 操作若涉及多命令原子性或读写混合，优先 pipeline（0.10 dedup.StoreResult / 0.11 conn_ratelimit）；本 story `Put` 用 pipeline（HSET+EXPIRE），`Get` 单命令（HGETALL），`Invalidate` 单命令（DEL）

### Latest Tech Information

- **go-redis v9**（已依赖）：`HSet(ctx, key, pairs...)` / `HGetAll(ctx, key).Result() → map[string]string` / `Expire(ctx, key, ttl)` / `Del(ctx, key)` 均为稳定 API；pipeline 接口与 0.10 / 0.11 一致。
- **miniredis v2**（已引入，0.11 通过 `miniredis.RunT(t)`）：HGETALL / HSET / EXPIRE / TTL / DEL 全部支持，本 story 无版本下限新需求。
- **`json.RawMessage`**（encoding/json）：`Marshal(json.RawMessage("null"))` → `null`；`Marshal(json.RawMessage("[]"))` → `[]`；空值必须是合法 JSON token，**不得**赋 `nil`（会被 marshal 成 `null` 而非预期的空切片 `[]`，造成客户端反序列化到 `[]FriendSummary` 时得到 nil 而非 empty slice）。Empty Providers 构造返回值时注意：
  ```go
  func (EmptyFriendsProvider) ListFriends(context.Context, string) (json.RawMessage, error) {
      return json.RawMessage(`[]`), nil
  }
  ```
- **Gin** + **gorilla/websocket v1.5.3**：集成测试复用 0.11 setup；WS envelope 反序列化照旧 `json.Unmarshal(msg, &Response)`。
- **不新增外部依赖**：无。

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 0.12 — AC 完整定义（lines 665-682）]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 4.5 — 扩展 0.12 骨架为完整 handler（lines 1306-1338）]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 1.5 — profile.update 成功后调 ws.InvalidateResumeCache（line 861）]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 2.2 — state.tick 成功后调 resumeCacheInvalidator.InvalidateResumeCache（line 939）]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 3.2 — friend.accept 成功后双向 Invalidate（line 1078）]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 6.4 — blindbox.redeem 成功后 Invalidate（line 1683）]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 7.3 — skin.equip 成功后 Invalidate（line 1774）]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 0.10 line 642 — session.resume 不走 dedup 的消息白名单]
- [Source: _bmad-output/planning-artifacts/prd.md#FR42 — session.resume 节流缓存（line 837）]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-PERF-3 — session.resume p95 ≤ 500ms（line 874）]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-PERF-6 — session.resume 缓存窗口 60 秒（line 877）]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-OBS-5 — session.resume QPS 核心指标（line 930）]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-OBS-3 — 日志必含字段 userId / connId / event / duration（line 928）]
- [Source: _bmad-output/planning-artifacts/prd.md#Rate Limit Matrix — session.resume ≤5/60s 返回缓存（line 595）]
- [Source: _bmad-output/planning-artifacts/prd.md#Redis Key Convention — resume_cache 未在表中单列（lines 547-554），但 D16 原则下 key space 必隔离]
- [Source: _bmad-output/planning-artifacts/prd.md#J4 运维旅程 — WS 风暴 + session.resume 全量拉（lines 301-316）]
- [Source: _bmad-output/planning-artifacts/architecture.md#D1 WebSocket Hub 结构 — PushOnConnect / BroadcastDiff 预留（lines 277-290）]
- [Source: _bmad-output/planning-artifacts/architecture.md#D6 OP-1 hub 接口约束 — PushOnConnect + BroadcastDiff 已纳入（line 363）]
- [Source: _bmad-output/planning-artifacts/architecture.md#Source Tree — internal/ws/session_resume.go 位置（line 886）]
- [Source: _bmad-output/planning-artifacts/architecture.md#multi-replica 不变量 — 共享状态走 Redis（§71, NFR-SCALE-2）]
- [Source: _bmad-output/planning-artifacts/architecture.md#OP-4 session.resume 增量协议 — Patterns 阶段决定，MVP 先全量（lines 123, 275, 1169, 1235）]
- [Source: docs/backend-architecture-guide.md#Section 12 WebSocket — hub / client / 消息格式]
- [Source: docs/backend-architecture-guide.md#Section 15.2 命名 / 15.3 Typed IDs]
- [Source: docs/backend-architecture-guide.md#Section 18 P2 坏味道不抄 — 仓库返回接口 / 接口在消费方]
- [Source: internal/ws/dispatcher.go — Register（非 dedup）/ RegisterDedup 路径差异]
- [Source: internal/ws/dedup.go — DedupStore 消费方接口定义模式先例]
- [Source: internal/ws/conn_guard.go — Blacklist / ConnectRateLimiter 消费方接口定义模式先例]
- [Source: internal/ws/hub.go — Client.UserID() / Client.ConnID() 已暴露]
- [Source: internal/ws/envelope.go — NewAckResponse 自动追加 `.result` 后缀]
- [Source: pkg/redisx/dedup.go — RedisDedupStore pipeline + Hash 编码先例]
- [Source: pkg/redisx/blacklist.go — key helper + godoc key space 注释先例]
- [Source: pkg/redisx/conn_ratelimit.go — pipeline 使用模式 + clock 注入先例]
- [Source: pkg/clockx/clock.go — RealClock / FakeClock API]
- [Source: pkg/logx/logx.go — logx.Ctx(ctx) 继承 requestId]
- [Source: internal/dto/error.go — AppError.WithCause / CategoryRetryable]
- [Source: internal/dto/error_codes.go#L39 — ErrInternalError 已注册，直接复用]
- [Source: internal/config/config.go — WSCfg 待扩展 / applyDefaults 模式（line 109-119）/ mustValidate 模式（line 121-140）]
- [Source: config/default.toml — [ws] 段 line 30-38 待追加 resume_cache_ttl_sec]
- [Source: cmd/cat/initialize.go — 装配点（`NewDispatcher` 之后、`NewUpgradeHandler` 之前）]
- [Source: _bmad-output/implementation-artifacts/0-11-ws-connect-rate-limit-and-abnormal-device-reject.md — 前序 story 模式 + applyDefaults 兼容性教训]
- [Source: _bmad-output/implementation-artifacts/0-10-ws-upstream-eventid-idempotent-dedup.md — DedupStore / RedisDedupStore 消费方接口分层先例]
- [Source: _bmad-output/implementation-artifacts/0-9-ws-hub-skeleton-envelope-broadcaster-interface.md — Dispatcher / UpgradeHandler / Client.UserID()]

### Project Structure Notes

- 与 `docs/backend-architecture-guide.md` 对齐：`internal/ws` 承载 WS 协议 / 消费方接口 + skeleton handler；`pkg/redisx` 承载 Redis 基础设施实现；后续 Service story 在 `internal/service/` 消费 `ws.ResumeCacheInvalidator`（P2 分层允许 service → ws 单向依赖，因为 service 处理业务，ws 处理协议，但 invalidator 接口是协议层对外暴露的副作用语义）。
- 本 story **不触碰** `internal/service/`（service 层尚未建立；即便建立也轮不到本 story 消费）。
- 无新依赖；无架构偏差。
- architecture.md line 886 `session_resume.go` 在 `internal/ws/` 的位置与本 story 文件位置一致（不是 `internal/ws/handlers/` 子目录 — 对比 `blindbox_handlers.go` 等业务 handler 的位置）；原因：session.resume 是平台级读操作，涉及多个 Provider 跨域组装，不像 `blindbox.redeem` 那样绑定单一业务领域，放 ws 顶层更合适。Story 4.5 会保持这个位置。

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- **接口归属设计决策（Task 1）**：最初把 `ResumeSnapshot` struct 定义在 `internal/ws/session_resume.go`，但 `pkg/redisx/resume_cache.go` 的 `Get`/`Put` 方法签名需引用它——会形成 `pkg/redisx → internal/ws` 的反向依赖并触发 import cycle。按照 0.10 `DedupResult` 和 0.11 `ConnectDecision` 先例，把 struct 下沉到 `pkg/redisx`，`internal/ws` 用 `type ResumeSnapshot = redisx.ResumeSnapshot` 别名 re-export。这保持 pkg → internal 单向依赖同时让业务代码仍以 ws-layer 值看待 Snapshot。
- **Constructor 形状细化（Task 3）**：Story AC3 说明 `NewSessionResumeHandler(cache, clock, userP, friendsP, catStateP, skinsP, blindboxesP, roomP)` 8 个位置参。实现时改为 `NewSessionResumeHandler(cache, clock, providers ResumeProviders)`（3 参数）—— 6 个 provider 聚合进 `ResumeProviders` struct。理由：(a) 8 位置参调用点极长（initialize.go 会是单行 11 列换行），(b) 未来 Story 4.5 要加 provider 时新字段加进 struct 即可，不需要所有调用点同步换签名，(c) 构造失败时 panic 消息能精确说明哪个字段 nil（"providers.User must not be nil" vs "第 3 个参数 nil"）。P2 "显式胜于隐式" 用具名字段表达优于隐式位置。
- **ServerTime 缓存隔离（Task 2 / 3）**：epics AC 原文要求 payload 含 `{user, friends, catState, skins, blindboxes, roomSnapshot}`（6 字段）+ Story 4.5 line 1326 加上 `serverTime`。实现时让 `ResumeSnapshot` struct 持 7 个字段（6 内容 + ServerTime），但 `RedisResumeCache.Put` 只 HSET 6 个内容字段；handler 的 `Handle` 在返回前用 `clock.Now()` 重新填 ServerTime。单元测试 `TestSessionResumeHandler_CacheHitSkipsProviders` 在缓存命中前推进 FakeClock 5s，断言响应的 ServerTime 是 +5s 的值 —— 证明不是从 Redis 读出来的。
- **Dispatcher Register vs RegisterDedup 防回归（Task 8）**：AC7 明示 session.resume 必须走 `Register`。怎么写一个能在 CI 层捕获"若未来有人误改成 RegisterDedup"的测试？单纯"同 envelope.id 两次返回不同内容"无法区分 dedup cache 与 resume cache（都会命中缓存返回一致结果）。解法：两次之间显式 `ts.cache.Invalidate(ctx, "resume-user")`，强制第二次必须走 Provider；若 dispatcher 误用 RegisterDedup，第二次会被 dedup 中间件判定为 replay 并返回第一次的缓存结果（OK=true，但 providers 计数 = 1 而非 2）。这条断言 `ts.userCalls.Load() == 2` 就是 CI 锁。
- **集成测试 `unexpected EOF` 告警**：`{"level":"warn","error":"websocket: close 1006 (abnormal closure): unexpected EOF",...}` 出现在每个集成测试的最后一行 —— 是 `defer conn.Close()` 触发的客户端 hang-up，被 readPump 观察到。与 0.9 / 0.10 / 0.11 集成测试一致，非 bug。`IsUnexpectedCloseError` 没过滤 1006 是 gorilla/websocket 默认行为；Story 0.9 note 里登记过，不追加修复。
- **预先存在失败（Task 9）**：`TestMustConnect_Integration` 在 Windows 下因 testcontainers 要求 rootless Docker 而跳过，Story 0.3 引入、0-11 Completion Notes line 444 登记，与 0.12 改动无关；`bash scripts/build.sh --test` 默认不带 integration tag 所以不触发。
- **Go version**：`go1.25` / `go-redis v9.18` / `miniredis v2.37`；无新增外部依赖。

### Completion Notes List

- **Task 1**：`internal/ws/session_resume.go` 定义 `ResumeSnapshot`（type alias 到 pkg/redisx）、`ResumeCache` / `ResumeCacheInvalidator` 两 interface、6 个 Provider interface、6 个 Empty*Provider 实现。godoc 引用 FR42 / NFR-PERF-3 / NFR-PERF-6 / NFR-OBS-5 / P2 / D16 并对比 0.11 fail-closed。
- **Task 2**：`pkg/redisx/resume_cache.go` 实现 `RedisResumeCache`（`Get` HGETALL、`Put` pipeline(HSET+EXPIRE)、`Invalidate` DEL）；struct 同时满足 `ws.ResumeCache` 与 `ws.ResumeCacheInvalidator`（Go 结构化 typing）；`NewResumeCache` nil clock / ttl<=0 panic；Hash 字段名集中为常量避免 Get/Put 拼写漂移。
- **Task 3**：`SessionResumeHandler` 用 `ResumeProviders` struct 聚合 6 provider；`Handle` 流程遵循 AC8；`buildSnapshot` helper 收敛 provider 调用；fail-open / fail-closed 严格分离；zerolog camelCase（userId/connId/action/cacheHit/durationMs）；serverTime 每次 `clock.Now().UTC().Format(RFC3339Nano)` 重生。
- **Task 4**：`WSCfg.ResumeCacheTTLSec` 新增；`applyDefaults` 填 60；`mustValidate` <=0 → `log.Fatal`；`config/default.toml` 追加 `resume_cache_ttl_sec = 60`；`config_test.go` 的 override 测试新增 `assert.Equal(t, 60, cfg.WS.ResumeCacheTTLSec)` 覆盖 applyDefaults 兼容路径。
- **Task 5**：`cmd/cat/initialize.go` 在 `NewDispatcher` 之后、`debug` 模式处理之前装配 `redisx.NewResumeCache` + `ws.NewSessionResumeHandler` 并 `dispatcher.Register("session.resume", ...)`；使用 `ws.ResumeProviders{User: ws.EmptyUserProvider{}, ...}` 字面量；`resumeCache` 变量局部不出 App。
- **Task 6**：`internal/ws/session_resume_test.go` 16 子测全绿（8 top-level + 8 nil-panic table-driven 子测）；覆盖 AC10 a-h。
- **Task 7**：`pkg/redisx/resume_cache_test.go` 8 测试全绿；覆盖 AC11 a-g；TTL 准确性通过 `mr.FastForward(61s)` 验证。
- **Task 8**：`internal/ws/session_resume_integration_test.go` 3 集成测全绿；`TestIntegration_SessionResume_Benchmark_10Calls_OneProviderHit` 验证 10 次 resume 6 个 provider 各 1 次调用（epics AC12.a 的 Mongo-query-once 在 Epic 0 的等价表达）；`NotDeduped` 通过"中间 invalidate + 同 id 两次" 锁死 Register（非 RegisterDedup）语义。
- **Task 9**：`bash scripts/build.sh --test` 全绿（17 个包 PASS）；`go test -tags=integration ./internal/ws/...` 全绿（Story 0.9 / 0.10 / 0.11 / 0.12 共 10+ integration cases）；`check_time_now.sh` OK；`TestErrorCodesMd_ConsistentWithRegistry` 绿（未新增 error code）。
- **Task 10**：两份 godoc 完整引用规范；docs/code-examples/resume_cache_invalidator_example.go 未新增（见 Task 10.3 说明）。
- **fail-open / fail-closed 原则贯彻**：cache 层错误 log warn 后降级到 provider（性能关，Redis 抖动不阻塞业务）；provider 层错误返回 `ErrInternalError.WithCause`（数据完整关，partial payload 会误导客户端）。与 0.11 的 fail-closed 属"安全关"不同—"用户反对 backup 掩盖核心风险"的反馈在 0.12 适用面是 provider 层（确实 fail-closed），不适用 cache 层（cache 故障 ≠ 掩盖 Mongo 问题）。
- **✅ 回归**：仅 Windows 下 `TestMustConnect_Integration` 失败（testcontainers 需要 rootless Docker），Story 0.3 引入的预先存在问题，不与本 story 关联。
- **⚠️ 未执行**：`bash scripts/build.sh --race --test` 因 Windows go1.25 cgo 工具链问题跳过（race 需 cgo，预先存在），常规 `--test` 已全绿。

### File List

**新建：**
- `server/internal/ws/session_resume.go`
- `server/internal/ws/session_resume_test.go`
- `server/internal/ws/session_resume_integration_test.go`
- `server/pkg/redisx/resume_cache.go`
- `server/pkg/redisx/resume_cache_test.go`

**修改：**
- `server/internal/config/config.go` — `WSCfg` 新增 `ResumeCacheTTLSec` + `applyDefaults` 默认 60 + `mustValidate` 非正值 Fatal
- `server/internal/config/config_test.go` — `TestMustLoad_OverrideWithoutWSSection` 追加 `ResumeCacheTTLSec` 断言
- `server/config/default.toml` — `[ws]` 追加 `resume_cache_ttl_sec = 60`
- `server/cmd/cat/initialize.go` — 装配 `redisx.NewResumeCache` + `ws.NewSessionResumeHandler` + `dispatcher.Register("session.resume", ...)`
- `server/_bmad-output/implementation-artifacts/sprint-status.yaml` — Story 0-12 状态 ready-for-dev → in-progress → review

### Change Log

| Date       | Version | Author | Summary |
|------------|---------|--------|---------|
| 2026-04-18 | 1.0     | dev    | 完成 Story 0.12：session.resume 缓存节流骨架（resume_cache:{userId} Redis Hash、TTL 60s、6 个 Provider interface + Empty 实现、ResumeCacheInvalidator）；14 条 AC 全部满足；单元测试 16 + Redis 测试 8 + 集成测试 3 = 27 新增测试用例全绿；回归 Story 0.9/0.10/0.11 不变；fail-open（cache 层）/ fail-closed（provider 层）策略分离。 |
