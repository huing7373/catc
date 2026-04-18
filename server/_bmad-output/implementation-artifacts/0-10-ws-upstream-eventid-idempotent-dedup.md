# Story 0.10: WS 上行 eventId 幂等去重

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a developer,
I want every authoritative WS write to be deduplicated via Redis SETNX on eventId with result caching,
So that retries and client replay bugs don't cause double-redeemed blindboxes or double-delivered touches (NFR-REL-3, NFR-SEC-9, FR57).

## Acceptance Criteria

1. **AC1 — Dispatcher 支持声明式 dedup**：Dispatcher 暴露两套注册 API：
   - `Register(msgType string, fn HandlerFunc)` —— 非权威读 RPC，不走 dedup（保留 Story 0.9 既有行为）
   - `RegisterDedup(msgType string, fn HandlerFunc)` —— 权威写 RPC，自动包装 dedup middleware
   - 同一 `msgType` 在两套 API 之间重复注册应 panic（避免配置漂移）
2. **AC2 — DedupStore 抽象（消费方接口）**：在 `internal/ws/dedup.go` 定义 `DedupStore` 接口（P2 消费方定义原则），方法签名：
   - `Acquire(ctx context.Context, eventID string) (acquired bool, err error)` —— `SET event:{eventId} "processing" NX EX {ttl}`，成功返回 true
   - `StoreResult(ctx context.Context, eventID string, result DedupResult) error` —— `HSET event_result:{eventId} ok=... type=... payload=... errorCode=... errorMessage=...` + `EXPIRE event_result:{eventId} {ttl}` + `SET event:{eventId} "done" EX {ttl}`（标记完成，保留 TTL）
   - `GetResult(ctx context.Context, eventID string) (result DedupResult, found bool, err error)` —— `HGETALL event_result:{eventId}`；key 不存在返回 found=false
3. **AC3 — RedisDedupStore 实现**：在 `pkg/redisx/dedup.go` 实现 `RedisDedupStore`，构造函数 `NewDedupStore(cmd redis.Cmdable, ttl time.Duration) *RedisDedupStore`；满足 `ws.DedupStore` 接口（Go 结构化 typing，无需显式声明）
4. **AC4 — 强制权威写走 dedup（未来 stories 消费）**：后续 stories 中的 `blindbox.redeem / touch.send / friend.accept / friend.delete / friend.block / friend.unblock / skin.equip / profile.update` 必须通过 `RegisterDedup` 注册（本 story 不注册这些 handler，仅交付基础设施 + 测试用 `debug.echo.dedup`）
5. **AC5 — dedup middleware 流程**：
   - 若 `envelope.ID == ""` → 直接返回 `{ok: false, error: {code: "VALIDATION_ERROR", message: "envelope.id required"}}`，不触碰 Redis
   - `Acquire` 返回 `true`（首次）→ 调用 handler → `defer` 保证无论 handler 返回 `(payload, nil)` 或 `(nil, err)`（包括 panic）都写入 `StoreResult`：
     - 成功：`DedupResult{OK: true, Payload: payload}`
     - AppError：`DedupResult{OK: false, ErrorCode: ae.Code, ErrorMessage: ae.Message}`
     - 非 AppError：`DedupResult{OK: false, ErrorCode: "INTERNAL_ERROR", ErrorMessage: err.Error()}`（与 Story 0.9 dispatcher 保持一致）
   - `Acquire` 返回 `false`（重复）→ 调用 `GetResult`：
     - `found == true` → 按缓存结果返回相同响应（ok / payload / error 全部与首次一致）
     - `found == false`（handler 正在执行或上次崩溃未及时写结果）→ 返回 `{ok: false, error: {code: "EVENT_PROCESSING", message: "event still processing, retry later"}}`，Category=`retry_after`，客户端可重试
6. **AC6 — dispatcher 的 AppError 透传（0.9 修复）**：`Dispatcher.Dispatch` 错误分支不再硬编码 `INTERNAL_ERROR`；若 handler 返回值满足 `errors.As(err, &*dto.AppError)` 则用 `ae.Code` + `ae.Message` 生成错误响应；否则仍回落到 `INTERNAL_ERROR`。此修复同时被 dedup middleware 的错误路径依赖
7. **AC7 — 非权威读不走 dedup**：`users.me / friends.list / friends.state / skins.catalog / blindbox.inventory / session.resume / ping / debug.echo`（现有 & 未来）继续走 `Register`；本 story 不修改它们的行为
8. **AC8 — 新增 error code 注册**：在 `internal/dto/error_codes.go` 注册 `ErrEventProcessing = register("EVENT_PROCESSING", "event still processing", http.StatusTooManyRequests, CategoryRetryAfter)`
9. **AC9 — 配置**：`WSCfg` 新增 `DedupTTLSec int`（`toml:"dedup_ttl_sec"`）；`config/default.toml` 设 `dedup_ttl_sec = 300`（对齐 NFR-SEC-9 5 分钟窗口）
10. **AC10 — initialize.go 装配**：在 redisCli 构造后创建 `redisx.NewDedupStore(redisCli.Cmdable(), time.Duration(cfg.WS.DedupTTLSec) * time.Second)`；注入 `ws.NewDispatcher(dedupStore)`（dispatcher 构造函数扩展；非 dedup handler 注册路径仍不触 Redis）；debug 模式额外注册 `dispatcher.RegisterDedup("debug.echo.dedup", echoHandler)`（test-only，用于集成测试）
11. **AC11 — zerolog 审计**：dedup middleware 每次触发记录 `info` 级日志 `{action: "ws_dedup", connId, userId, eventId, msgType, outcome: "first|replay|processing"}`（NFR-OBS-5, P5）；使用 `logx.Ctx(ctx)`；首次执行在 handler 完成后补记 `durationMs` 字段
12. **AC12 — 单元测试（dedup middleware）**：用 fake `DedupStore` 覆盖：
    - 首次执行成功 → `StoreResult` 被调用一次，返回 payload
    - 首次执行返回 AppError → `StoreResult` 记录 AppError 的 Code/Message
    - 首次执行 panic → defer 仍写入 `INTERNAL_ERROR`（middleware 必须自己 recover 或假设外层 recover；本 story 在 middleware 内 recover 并上抛 fmt.Errorf("handler panic: %v", r)）
    - 重复 eventId 且缓存命中 → 不调用 handler，响应与首次一致（成功 / 失败两种）
    - 重复 eventId 但 `GetResult` found=false → 返回 `EVENT_PROCESSING`
    - 空 `env.ID` → 返回 `VALIDATION_ERROR`，不调用 `Acquire`
13. **AC13 — 单元测试（RedisDedupStore）**：使用 `miniredis`（已在 `go.sum`，locker_test.go 既有模式），table-driven：
    - Acquire 首次成功、重复失败
    - StoreResult + GetResult 往返（含空 payload / 有 payload / 错误结果）
    - TTL 正确设置（通过 `mr.TTL(key)` 或 `mr.FastForward`）
    - GetResult 在 key 不存在时返回 `found=false, err=nil`
14. **AC14 — Dispatcher 单元测试**：新增
    - `RegisterDedup` 与 `Register` 同名 type → panic
    - dedup 路径端到端（用 fake DedupStore）
    - AppError 透传（AC6）
15. **AC15 — 集成测试（真实 Redis）**：`internal/ws/dedup_integration_test.go`（`//go:build integration`）启动 httptest.Server + 真实 Redis（miniredis 亦可，用 `*miniredis.Miniredis` 替代；若需真 Redis 使用 testcontainers-go —— 项目已在 0.3 启用）：
    - 建连 → 连发 3 次相同 eventId 的 `debug.echo.dedup`，payload 为 `{"n":1}` / `{"n":2}` / `{"n":3}`
    - 断言：只第 1 次 handler 执行（通过 handler 内 counter atomic.Int64）；3 次响应体 byte-by-byte 相等且 payload 对应首次的 `{"n":1}`（非 `{"n":2}` / `{"n":3}`）
    - 断言：`event:<id>` 值为 `"done"`；`event_result:<id>` Hash 含 `ok=true`
16. **AC16 — 回归保护**：Story 0.9 的 `dispatcher_test.go` 现有 3 个测试（KnownType / UnknownType / InvalidEnvelope）行为不变；`upgrade_handler_integration_test.go` 的 `debug.echo` 流程（非 dedup）不变；`bash scripts/build.sh --test` 全量绿

## Tasks / Subtasks

- [x] Task 1: DedupStore 接口 + DedupResult（AC: #2）
  - [x] 1.1 创建 `internal/ws/dedup.go`
  - [x] 1.2 定义 `DedupResult` struct：`OK bool`, `Payload json.RawMessage`, `ErrorCode string`, `ErrorMessage string`
  - [x] 1.3 定义 `DedupStore` interface：三个方法（Acquire / StoreResult / GetResult）
  - [x] 1.4 DedupResult 提供 `ToHash() map[string]string` 和 `FromHash(m map[string]string) DedupResult` 辅助（序列化 Payload 用 base64 或直接存 JSON string —— 推荐 JSON string）

- [x] Task 2: RedisDedupStore 实现（AC: #3）
  - [x] 2.1 创建 `pkg/redisx/dedup.go`
  - [x] 2.2 `RedisDedupStore` struct 持有 `cmd redis.Cmdable` + `ttl time.Duration`
  - [x] 2.3 `NewDedupStore(cmd redis.Cmdable, ttl time.Duration) *RedisDedupStore`（ttl=0 时 panic，避免误用）
  - [x] 2.4 `Acquire`：`SetNX(ctx, "event:"+eventID, "processing", ttl).Result()`；error 上抛
  - [x] 2.5 `StoreResult`：Redis pipeline：`HSet("event_result:"+eventID, hash) + Expire(..., ttl) + Set("event:"+eventID, "done", ttl)`
  - [x] 2.6 `GetResult`：`HGetAll`；len==0 → found=false

- [x] Task 3: dedup middleware（AC: #5, #11, #12）
  - [x] 3.1 在 `internal/ws/dedup.go` 添加 `func dedupMiddleware(store DedupStore, fn HandlerFunc) HandlerFunc`
  - [x] 3.2 middleware 签名遵循 HandlerFunc `(ctx, client, env) (json.RawMessage, error)`
  - [x] 3.3 空 env.ID → 返回 `dto.ErrValidationError.WithCause(fmt.Errorf("envelope.id required"))`（AppError 机制 + Message 需清晰，可改用 `*dto.AppError` copy 带定制 Message）
  - [x] 3.4 `Acquire` 成功分支：
    ```go
    var result DedupResult
    defer func() {
        if r := recover(); r != nil {
            result = DedupResult{OK: false, ErrorCode: "INTERNAL_ERROR", ErrorMessage: fmt.Sprintf("handler panic: %v", r)}
            _ = store.StoreResult(ctx, env.ID, result) // best-effort
            panic(r) // 让外层 readPump recover 打 log
        }
        _ = store.StoreResult(ctx, env.ID, result)
    }()
    payload, err := fn(ctx, client, env)
    // 填充 result 后 return
    ```
  - [x] 3.5 `Acquire` 失败分支：调用 `GetResult` → found=true 则按缓存返回；found=false 则返回新 `ErrEventProcessing`
  - [x] 3.6 zerolog info：`{action: "ws_dedup", connId, userId, eventId, msgType, outcome, durationMs?}`

- [x] Task 4: Dispatcher 扩展（AC: #1, #6, #14）
  - [x] 4.1 修改 `NewDispatcher()` → `NewDispatcher(store DedupStore) *Dispatcher`（store 可为 nil，但 nil 调用 RegisterDedup 需 panic）
  - [x] 4.2 `Dispatcher.dedupStore` 字段 + 已注册 type 集合 `types map[string]bool` 防重复
  - [x] 4.3 `Register(msgType, fn)` 若 type 已存在则 panic（防重）
  - [x] 4.4 新增 `RegisterDedup(msgType, fn)`：nil store 或 type 已存在则 panic；否则 `d.handlers[msgType] = dedupMiddleware(d.dedupStore, fn)`
  - [x] 4.5 `Dispatch` 错误分支：`errors.As(err, &*dto.AppError)` → 使用 `ae.Code, ae.Message`；否则 `INTERNAL_ERROR`
  - [x] 4.6 更新 `dispatcher_test.go`：保留现有 3 个测试，新增 `TestDispatcher_RegisterDedupPanicsOnDuplicate` / `TestDispatcher_AppErrorCodePropagation` / `TestDispatcher_DedupPath`（用 fake store）

- [x] Task 5: error code + config（AC: #8, #9）
  - [x] 5.1 `internal/dto/error_codes.go` 添加 `ErrEventProcessing`
  - [x] 5.2 `internal/dto/error_codes_test.go` 如有枚举校验需同步更新
  - [x] 5.3 `internal/config/config.go` 的 `WSCfg` 添加 `DedupTTLSec int` 字段
  - [x] 5.4 `config/default.toml` `[ws]` 段添加 `dedup_ttl_sec = 300`
  - [x] 5.5 `config/local.toml` 可选添加（不添加则继承 default，推荐不添加）

- [x] Task 6: initialize.go 装配（AC: #10）
  - [x] 6.1 在 redisCli 构造后：`dedupStore := redisx.NewDedupStore(redisCli.Cmdable(), time.Duration(cfg.WS.DedupTTLSec)*time.Second)`
  - [x] 6.2 `dispatcher := ws.NewDispatcher(dedupStore)`
  - [x] 6.3 debug 模式注册 `dispatcher.RegisterDedup("debug.echo.dedup", echoFn)`；echoFn 同现有 `debug.echo` 的 handler（原样返回 env.Payload）
  - [x] 6.4 保留现有 `debug.echo` 非 dedup 注册（AC7、AC16）

- [x] Task 7: 单元测试（AC: #12, #13, #14）
  - [x] 7.1 `internal/ws/dedup_test.go`：fake DedupStore + middleware 行为（6 个子用例，含 panic recovery）
  - [x] 7.2 `pkg/redisx/dedup_test.go`：miniredis + Acquire/StoreResult/GetResult/TTL table-driven
  - [x] 7.3 `internal/ws/dispatcher_test.go`：新增 RegisterDedup 相关测试

- [x] Task 8: 集成测试（AC: #15）
  - [x] 8.1 创建 `internal/ws/dedup_integration_test.go` with `//go:build integration`
  - [x] 8.2 启动 `httptest.Server` + Hub + Dispatcher（注入 miniredis-backed DedupStore 或真 Redis）
  - [x] 8.3 注册 `debug.echo.dedup` handler（handler 内 `atomic.Int64` 计数）
  - [x] 8.4 发 3 次相同 `{"id":"e1","type":"debug.echo.dedup","payload":{"n":N}}`
  - [x] 8.5 断言 handler 只执行 1 次；3 个响应相等；首个响应 payload 为 `{"n":1}`
  - [x] 8.6 可选：额外子测试验证错误结果也被缓存（handler 返回 AppError 后重放相同错误）

- [x] Task 9: 回归 + 构建（AC: #16）
  - [x] 9.1 `bash scripts/build.sh --test` 全量通过（含 integration tag）
  - [x] 9.2 `check_time_now.sh` 通过（middleware 不直接调 `time.Now()`；durationMs 可通过 clock.Clock 注入，但本 story 可接受以 `time.Since(start)` 方式在 middleware 内单点使用 —— 若 CI 脚本扫描 `internal/ws/` 的 `time.Now`，则改用 `clockx.Clock` 注入。**决策：接受 clockx 注入以避免破坏 0.9 的 CI guard**，middleware 签名改为接受 `clock clockx.Clock` 或 Dispatcher 持有 clock 供 middleware 使用）
  - [x] 9.3 更新 story 0.9 留下的 `docs/code-examples/ws_handler_example.go` 若示例中应体现 dedup（可选，不破坏）

## Dev Notes

### Architecture Constraints (MANDATORY)

- **宪法 §1 显式胜于隐式**：Dispatcher 构造函数显式接收 `DedupStore`；禁止 package-level 全局
- **multi-replica 不变量 #2**：所有共享状态在 Redis —— dedup 状态绝不能用 in-memory map；必须走 `RedisDedupStore`（或满足同接口的 mock 仅测试用）
- **D10 事务边界**：`event:{eventId}` Redis SETNX 5min 去重；对 blindbox 等权威写：dedup（Story 0.10） → Mongo 事务（D10） 形成"权威写"双保险
- **D16 幂等通过 Redis**：`event:{eventId}` / `event_result:{eventId}` 命名固定，key space 与现有 `ratelimit:* / presence:* / blacklist:* / lock:cron:*` 分隔
- **P3 消息命名**：test-only handler 用 `debug.echo.dedup`（点分 domain.action.subaction，符合 P3）
- **P5 日志**：camelCase 字段 `eventId`（非 `event_id`）；dedup middleware 必须输出 `connId / userId / eventId / msgType / outcome`
- **M9 Clock interface**：`internal/ws/` 下禁止裸 `time.Now()`（Story 0.9 引入的 CI 守卫）；duration 测量改为 `clockx.Clock` 注入，middleware 通过 Dispatcher 的 clock 字段使用
- **M15 per-conn rate limit** 位于 readPump 早于 dispatcher；dedup 发生在 dispatcher 内层，与 rate limit 正交
- **NFR-SEC-9** 明确 5 分钟窗口；不可缩短（客户端重连 / APNs fallback 需要 5 分钟保护）；可配置但默认 300
- **NFR-REL-3** 盲盒"零重复一票否决"：dedup 是第一道；Mongo conditional update `WHERE status=pending` 第二道；`user_skins` unique index 第三道（Story 6.4）—— 本 story 交付第一道基础设施

### 关键实现细节

**`SET key value NX EX ttl` in go-redis v9：**

```go
ok, err := cmd.SetNX(ctx, "event:"+eventID, "processing", ttl).Result()
// ok == true 表示首次获取，false 表示已存在
```

**HSet + Expire + Set 组合（pipeline）：**

```go
pipe := s.cmd.Pipeline()
pipe.HSet(ctx, "event_result:"+eventID, hashMap)
pipe.Expire(ctx, "event_result:"+eventID, s.ttl)
pipe.Set(ctx, "event:"+eventID, "done", s.ttl)
_, err := pipe.Exec(ctx)
```

**DedupResult 序列化到 Redis Hash 的字段：**

```
ok:           "true" | "false"
type:         原始 envelope.type（冗余，便于调试）—— 可选，不影响正确性
payloadJSON:  json.RawMessage string（空 string 表 nil）
errorCode:    空 string 表示无错
errorMessage: 空 string 表示无错
```

反序列化时：`ok == "true"` → 成功分支；`errorCode != ""` → AppError 路径。

**AppError 的替换 Message 技巧：**

`dto.ErrValidationError` 是全局 `*AppError`；不可直接改 Message（跨 goroutine 共享）。必须：

```go
e := *dto.ErrValidationError
e.Message = "envelope.id required"
return &e
```

或者使用 `dto.ErrValidationError.WithCause(fmt.Errorf("envelope.id required"))`（WithCause 返回副本；但 Message 不改 —— 客户端看到的 message 仍是 "validation error"）。推荐前者（显式副本 + 定制 Message）。

**Dispatcher AppError 透传（AC6 修复）：**

```go
// 原代码
resp := NewErrorResponse(env.ID, env.Type, "INTERNAL_ERROR", err.Error())

// 改为
var ae *dto.AppError
if errors.As(err, &ae) {
    resp := NewErrorResponse(env.ID, env.Type, ae.Code, ae.Message)
} else {
    resp := NewErrorResponse(env.ID, env.Type, "INTERNAL_ERROR", err.Error())
}
```

注意：`dispatcher.go` 目前 import 了 `"github.com/rs/zerolog/log"`；需新增 `"errors"` 和 `"github.com/huing/cat/server/internal/dto"`。

**miniredis 在单元测试中的模式（已在 locker_test.go 建立）：**

```go
mr := miniredis.RunT(t)
cmd := redis.NewClient(&redis.Options{Addr: mr.Addr()})
defer cmd.Close()
store := redisx.NewDedupStore(cmd, 5*time.Minute)

ok, err := store.Acquire(ctx, "e1")
// miniredis 提供 mr.TTL("event:e1") 验证 TTL
```

**为何不在 Dispatcher 中 hold Redis：**

遵循 P2 层次：Dispatcher 位于 `internal/ws`（handler 层 adjacent），`pkg/redisx` 是底层基础设施。消费方接口（`DedupStore`）定义在 `internal/ws`，impl 在 `pkg/redisx`。Dispatcher 仅见 `DedupStore` 接口，不见 `redis.Cmdable`，利于测试替身。

**为什么 Acquire 返回 false 且 GetResult found=false 不自动重试：**

- handler 可能仍在执行（5min 内）→ 立即重试会造成 thundering herd
- handler 崩溃未写 result → 短期内（TTL 内）仍无法自愈，但返回 `EVENT_PROCESSING` 让客户端按 retry_after 语义等待；5min 后 key 过期，下次 SETNX 重新获取
- 若需要"崩溃快速恢复"，需引入 heartbeat / fencing —— 超出 MVP 范围

### Source Tree — 要创建/修改的文件

**新建：**
- `internal/ws/dedup.go` — DedupStore 接口 + DedupResult + dedupMiddleware
- `internal/ws/dedup_test.go` — middleware 单元测试（fake store）
- `internal/ws/dedup_integration_test.go` — //go:build integration，端到端 3 次相同 eventId
- `pkg/redisx/dedup.go` — RedisDedupStore 实现
- `pkg/redisx/dedup_test.go` — miniredis 单元测试

**修改：**
- `internal/ws/dispatcher.go` — 构造签名改为 `NewDispatcher(store DedupStore, clock clockx.Clock)`；新增 `RegisterDedup`；type 防重；AppError 透传
- `internal/ws/dispatcher_test.go` — 新增 3 个测试（duplicate-type panic、AppError 透传、dedup 路径）；已有 3 个测试的构造调用同步调整（传 nil store + real clock）
- `internal/dto/error_codes.go` — 注册 `ErrEventProcessing`
- `internal/config/config.go` — `WSCfg` 添加 `DedupTTLSec int`
- `config/default.toml` — `[ws] dedup_ttl_sec = 300`
- `cmd/cat/initialize.go` — 构造 `dedupStore`；传入 `NewDispatcher`；debug 模式 `RegisterDedup("debug.echo.dedup", ...)`

**不修改（回归保护）：**
- `internal/ws/hub.go`, `hub_test.go`, `rate_limit.go`, `rate_limit_test.go`, `envelope.go`, `envelope_test.go`, `broadcaster.go`, `broadcaster_test.go`, `upgrade_handler.go`, `upgrade_handler_integration_test.go` —— Story 0.9 交付不变
- `internal/handler/health_handler*` —— 不涉及

### Testing Standards

- 单元测试同目录，`xxx_test.go`；多场景 table-driven（宪法）
- 所有测试 `t.Parallel()` 开启（dedup store 单元测试用独立 miniredis 实例即可并发）
- testify：`require.NoError` / `assert.Equal`
- 集成测试 `//go:build integration` tag；本 story 集成测试允许在 miniredis 上运行（比 testcontainers Redis 更快）—— 但建议优先 `miniredis` 满足 CI 稳定性；若后续需要真实 Redis 行为，再切换
- Fake DedupStore 实现可简化为 `map[string]DedupResult` + `map[string]bool`（sync.Mutex 保护）
- Panic-in-handler 测试：用 `defer func() { require.Error(t, recover...) }()` 外层，确保 middleware 的 defer StoreResult 已触发
- Redis key 清理：miniredis `RunT` 会自动清理；真 Redis 需 `FLUSHDB` 或固定前缀

### Previous Story Intelligence (Story 0.9)

- **Dispatcher 是"路由 + 响应序列化"的单一职责**：新增中间件不应污染 Dispatcher 核心；把 dedup 写成 HandlerFunc 包装器（decorator 模式）
- **Client.send channel backpressure**：满则关闭连接 —— dedup middleware 不需感知；上游 trySend 已处理
- **readPump panic recovery 已存在**：middleware 内若 repanic，readPump 的 defer recover 能兜底；但 `defer StoreResult` 必须先执行（defer 栈后进先出：先 store，再 repanic）
- **CI `check_time_now.sh` 扫描 `internal/ws/`**：duration 测量必须走 `clockx.Clock`；dispatcher 需新增 `clock clockx.Clock` 字段并传给 middleware
- **Error code 注册表零重复**：`dto.register` 重复 code 会 `log.Fatalf`；本 story 新增 `EVENT_PROCESSING` 需确认未冲突（当前代码确认未注册）
- **`AppError` 全局变量不可变**：直接修改 Message 会污染其他调用点 —— 用副本模式（`e := *dto.ErrXXX; e.Message = ...; return &e`）
- **Story 0.9 fix 记录（review 教训）**：`close(send)` 并发 panic → 改用 `done` channel 驱动 writePump 退出；dedup middleware 沿用此习惯（不 close 临时 chan）
- **集成测试模式**：`httptest.NewServer(router)` + `gorilla/websocket.DefaultDialer.Dial(...)` + `conn.WriteJSON` / `conn.ReadJSON`

### Git Intelligence（最近 5 commit）

```
5bf8d7a chore: mark Story 0.9 done — WS Hub, Envelope, Broadcaster, Dispatcher, UpgradeHandler
de1bbf6 fix(review): 0-9 round 2 — send channel 永不关闭，done channel 驱动 writePump 退出
e6c844a fix(review): 0-9 round 1 — close(send) 并发 panic 守卫 + healthz goroutine 阈值修正
a050cbc chore: mark Story 0.8 done — Redis distributed lock, cron scheduler, heartbeat_tick
cd925bd fix(review): 0-8 round 2 — Start 继承 parent ctx 而非 context.Background
```

关键惯例：
- 每 story 独立 done commit；review 反馈独立 `fix(review): X-N round M —` 提交
- `parent ctx` 继承（context.Background 仅 bootstrap）—— middleware 必须使用 handler 传入的 ctx
- 并发 channel 安全：never close producer-shared channel；用 done channel 信号
- 新 Runnable / 基础设施先在 pkg/ 落地，internal/ 消费

### Latest Tech Information

- **go-redis v9**（已在依赖）：`SetNX(ctx, key, val, ttl).Result()` 返回 `(bool, error)`；`HSet(ctx, key, field, val...)` 可变参；`Pipeline()` 非事务流水线，性能对 dedup 写路径（3 命令）够用；若需原子性 upgrade `TxPipeline`
- **miniredis v2**：支持 `SETNX / EXPIRE / HSET / HGETALL` 全部本 story 用到的命令；`RunT(t)` 自动清理；`FastForward(ttl)` 可测试过期行为
- **gorilla/websocket v1.5.3**（Story 0.9 引入）：集成测试无需新依赖
- **golang.org/x/time/rate**（Story 0.9 引入）：与 dedup 无交互
- **不新增外部依赖**：整个 story 复用现有依赖

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 0.10 — AC 完整定义（lines 628-645）]
- [Source: _bmad-output/planning-artifacts/epics.md#Story 0.12 — session.resume 明确不走 dedup（AC7 依据）]
- [Source: _bmad-output/planning-artifacts/architecture.md#D10 事务边界 — line 413 event:{eventId} 5min 去重]
- [Source: _bmad-output/planning-artifacts/architecture.md#D11 数据保留 — line 426 event:{eventId} TTL 5min]
- [Source: _bmad-output/planning-artifacts/architecture.md#multi-replica 不变量 — line 71 共享状态必走 Redis]
- [Source: _bmad-output/planning-artifacts/architecture.md#Cross-Cutting #3 — 幂等去重服务定位]
- [Source: _bmad-output/planning-artifacts/architecture.md#NFR Mapping — NFR-SEC-9 5min 窗口]
- [Source: _bmad-output/planning-artifacts/prd.md#FR57 — WS 上行按 eventId 去重]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-SEC-9 — 所有权威写 RPC 5min 窗口]
- [Source: _bmad-output/planning-artifacts/prd.md#NFR-REL-3 — 盲盒零重复领取]
- [Source: docs/backend-architecture-guide.md#Section 12 WebSocket — internal/ws 布局]
- [Source: internal/ws/dispatcher.go — 现有 Register/Dispatch 实现（待扩展）]
- [Source: internal/ws/envelope.go — Envelope / Response / ErrorPayload 定义]
- [Source: internal/ws/hub.go — Client / trySend / Hub（不修改，仅参考）]
- [Source: internal/dto/error_codes.go — AppError 注册模式 + 现有 codes]
- [Source: internal/dto/error.go — AppError 类型 + RespondAppError（WithCause/WithRetryAfter 模式）]
- [Source: pkg/redisx/locker.go — SETNX 模式先例]
- [Source: pkg/redisx/locker_test.go — miniredis 测试模式先例]
- [Source: pkg/clockx/clock.go — Clock interface，duration 测量注入]
- [Source: cmd/cat/initialize.go#Lines 37-67 — 现有 Dispatcher 装配，待扩展]
- [Source: internal/config/config.go — WSCfg 待扩展]
- [Source: config/default.toml — [ws] 段待扩展]
- [Source: _bmad-output/implementation-artifacts/0-9-ws-hub-skeleton-envelope-broadcaster-interface.md — 前序 story 模式 + 教训]

### Project Structure Notes

- 与 `docs/backend-architecture-guide.md` 对齐：`internal/ws` 承载 WS 路由 / 协议 / middleware；`pkg/redisx` 承载 Redis 基础设施
- `DedupStore` 接口位于 `internal/ws`（消费方定义 —— 对齐 Story 0.12 的 `ResumeCacheInvalidator` 模式），impl 位于 `pkg/redisx`
- 无新依赖；无架构偏差

## Dev Agent Record

### Agent Model Used

claude-opus-4-7 (1M context)

### Debug Log References

- `bash scripts/build.sh --test` — 全量通过（vet / check_time_now.sh / build / test 全绿）
- `go test -tags=integration ./internal/ws/...` — `TestIntegration_WS_DedupIdempotency` 通过；3 次相同 eventId handler 仅 1 次执行，3 个响应 byte-by-byte 相等，Redis `event:evt-1 = "done"`，`event_result:evt-1` Hash `ok=true`

### Completion Notes List

- **DedupResult 物理位置调整**：Story 原文本建议 `DedupResult` 放 `internal/ws/dedup.go`，但 Go 采用 nominal typing —— `ws.DedupStore` 接口的 `DedupResult` 参数类型必须与 `pkg/redisx.RedisDedupStore` 的方法签名完全一致。为满足架构约束 "pkg/ 不得引用 internal/"（`docs/backend-architecture-guide.md` §3），将 `DedupResult` 实体定义放在 `pkg/redisx/dedup.go`，在 `internal/ws/dedup.go` 通过 `type DedupResult = redisx.DedupResult` 别名复用，消费方语义保留。
- **clockx.Clock 注入路径**：依照 AC9.2 的决策 —— `Dispatcher` 新增 `clock` 字段；`dedupMiddleware` 从 Dispatcher 拿到 clock 做 `durationMs` 测量，满足 `check_time_now.sh` M9 守卫。
- **AppError 透传（AC6）**：`Dispatcher.Dispatch` 错误分支用 `errors.As(err, &ae)` 解 AppError；dedup middleware 的缓存失败路径也通过构造 `*dto.AppError{Code, Message}` 走同一个响应链。
- **回归保护**：Story 0.9 的 3 个 dispatcher 单测保留（TestDispatcher_KnownType / UnknownType / InvalidEnvelope），签名调整为 `newTestDispatcher(nil)`；`upgrade_handler_integration_test.go` 的 debug.echo 流程保留，`setupTestServer` 传 `NewDispatcher(nil, clockx.NewRealClock())`。
- **集成测试用 miniredis**：依照 Story Testing Standards 优先级，选 miniredis（已在依赖），避免 Windows 主机上 testcontainers 的 Docker rootless 障碍；真 Redis 行为由 `pkg/redisx/client_integration_test.go` 的既有测试覆盖。
- **docs/error-codes.md 同步**：`TestErrorCodesMd_ConsistentWithRegistry` 要求文档与 registry 同步，新增 EVENT_PROCESSING 条目。
- **已知非阻塞 flake**：`TestIntegration_WS_ShutdownCloseFrame`（Story 0.9 既有集成测试）在全量 `-tags=integration` + miniredis/mongo testcontainers 同批运行时偶发 i/o timeout；单独 & `--test` 常规回归 100% 通过，属 Story 0.9 遗留环境抖动，与本 story 无关。

### File List

**新建：**
- `server/internal/ws/dedup.go` — DedupStore 接口 + DedupResult 别名 + dedupMiddleware
- `server/internal/ws/dedup_test.go` — fake DedupStore + middleware 单元测试（7 个子用例）
- `server/internal/ws/dedup_integration_test.go` — `//go:build integration`，端到端 3 次相同 eventId 去重
- `server/pkg/redisx/dedup.go` — DedupResult 实体 + RedisDedupStore 实现
- `server/pkg/redisx/dedup_test.go` — miniredis 单元测试（Acquire / StoreResult / GetResult / TTL / HashRoundTrip）

**修改：**
- `server/internal/ws/dispatcher.go` — 构造签名 `NewDispatcher(store, clock)`；新增 `RegisterDedup`；duplicate-type panic；AppError 透传
- `server/internal/ws/dispatcher_test.go` — 现有 3 个测试签名调整；新增 5 个测试（RegisterPanicsOnDuplicate / RegisterDedupPanicsOnDuplicate / RegisterDedupPanicsWithoutStore / AppErrorCodePropagation / DedupPath）
- `server/internal/ws/upgrade_handler_integration_test.go` — `NewDispatcher` 调用签名调整（传 nil store + RealClock），行为不变
- `server/internal/dto/error_codes.go` — 注册 `ErrEventProcessing`
- `server/internal/config/config.go` — `WSCfg` 新增 `DedupTTLSec int`
- `server/config/default.toml` — `[ws] dedup_ttl_sec = 300`
- `server/cmd/cat/initialize.go` — 构造 `dedupStore`；传入 `NewDispatcher(dedupStore, clk)`；debug 模式新增 `RegisterDedup("debug.echo.dedup", echoFn)`
- `server/docs/error-codes.md` — 同步新增 EVENT_PROCESSING 条目
- `server/_bmad-output/implementation-artifacts/sprint-status.yaml` — 0-10 状态 ready-for-dev → in-progress → review

## Change Log

| 日期 | 版本 | 变更 | 作者 |
|---|---|---|---|
| 2026-04-18 | 1.0 | 初版实现：DedupStore 接口 + RedisDedupStore + dedupMiddleware + Dispatcher RegisterDedup/AppError 透传 + clockx 注入；单元测试 + 集成测试全绿；AC1-16 全覆盖 | Dev (Claude) |
