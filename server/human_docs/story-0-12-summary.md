# Story 0.12: session.resume 缓存节流 — 实现总结

给 WebSocket 的 `session.resume` RPC 搭一个 Redis Hash 缓存骨架：同一用户 60 秒内重复请求只触发一次上游读，后续直接命中缓存返回。这是 J4 运维旅程的第二道防线（Story 0.11 在 TCP upgrade 前挡住狂连；0.12 挡住"连上之后每次立刻 session.resume 全量拉"的另一半风暴），同时满足 FR42、NFR-PERF-3（p95 ≤ 500ms）、NFR-PERF-6（60s 缓存窗口）、NFR-OBS-5（session.resume QPS 核心指标）的明确要求。

**本 story 交付骨架**：handler + 缓存层 + 6 个 Provider interface（全 Empty 实现）+ `ResumeCacheInvalidator` 供后续 Service story 消费。真实的 `user / friends / catState / skins / blindboxes / roomSnapshot` payload 由 Story 1.1 / 2.2 / 3.4 / 6.3 / 7.2 / 4.2 逐个补，Story 4.5 统一收口变完整 handler。

## 做了什么

### 消费方接口（internal/ws）

- `internal/ws/session_resume.go` 定义：
  - `ResumeCache` —— handler 读写缓存（`Get` / `Put`）
  - `ResumeCacheInvalidator` —— Service 层权威写后调 `Invalidate(ctx, userID)` 失效缓存（Story 1.5 / 2.2 / 3.2 / 6.4 / 7.3 未来消费方）
  - 6 个 Provider interface：`UserProvider` / `FriendsProvider` / `CatStateProvider` / `SkinsProvider` / `BlindboxesProvider` / `RoomSnapshotProvider` —— 每个一个方法返回 `json.RawMessage`
  - 对应 6 个 `Empty*Provider` 结构体（无字段、无 I/O），返回 `json.RawMessage("null")` 或 `json.RawMessage("[]")` 占位
- `ResumeSnapshot` 实体放 `pkg/redisx`，`internal/ws` 用 `type ResumeSnapshot = redisx.ResumeSnapshot` 别名（同 Story 0.10 `DedupResult` / 0.11 `ConnectDecision` 模式，解决 pkg 不能 import internal 的依赖方向问题）
- 每个 content 字段是 `json.RawMessage` 而非具体 struct，Epic 1 / 2 / 3 / 6 / 7 各自决定 payload shape 时不需要回改本 story

### Redis 实现（pkg/redisx/resume_cache.go）

**RedisResumeCache**：
- Key `resume_cache:{userID}` 是 Redis Hash，字段 `{user, friends, catState, skins, blindboxes, roomSnapshot}`（**不**存 `serverTime`）
- 同一结构体同时实现 `ws.ResumeCache` 读写接口和 `ws.ResumeCacheInvalidator` 失效接口（Go 结构化 typing 自然支持）
- `Get` → HGETALL + 6 字段 `rawFromString` 反序列化；空 Hash → `found=false`
- `Put` → Pipeline(HSET 6 字段 + EXPIRE ttl)；单 RTT
- `Invalidate` → DEL key；不存在的 key DEL 返回 0 不报错
- 构造期校验 `clock != nil / ttl > 0`，否则 panic（生产配置错必须启动时炸出来）

**为何 Hash 而非 String blob**：epics 明确指定 Hash；Redis CLI 人工巡检可直接看到字段分组；给未来 OP-4 增量协议（Story 4.5 之后决定）留 per-section 失效的扩展空间

**为何 serverTime 不入缓存**：serverTime 是客户端用来估算时钟偏差的。若缓存了 serverTime，第 59 秒命中会返回 59 秒前的时间戳，客户端误算出 59s 时钟偏差。每次响应由 handler 用 `clock.Now()` 重新戳。

### SessionResumeHandler（internal/ws/session_resume.go）

- `ResumeProviders` struct 聚合 6 个 Provider（而不是 8 个位置参数）——未来 Story 4.5 加 Provider 只需扩 struct，不用改调用点
- `NewSessionResumeHandler(cache, clock, providers)` —— 所有字段 nil 检查全 panic（Empty*Provider 才是"关闭"的正确表达，nil 不是）
- 业务流程（`Handle` 方法，dispatcher 走 `Register` **非** `RegisterDedup`）：
  1. `cache.Get(ctx, userID)` → 命中直接用
  2. Miss（或 Get 报错 fail-open）→ 按 userID 走 `singleflight.Group.Do` 合并并发请求 → winner 调 6 个 Provider → 写回 cache（Put 报错 fail-open）
  3. 无论命中与否，用 `clock.Now().UTC().Format(RFC3339Nano)` 覆盖 `snapshot.ServerTime` 再 marshal
- 整个 handler **不走 eventId dedup**：session.resume 是读操作、天然幂等；RegisterDedup 会强制 envelope.id 非空并把 60s 内重复请求误判为 `EVENT_PROCESSING`，与 resume_cache 命中语义冲突

### singleflight 合并并发 miss（review round 1 的直接修复）

最初的 handler 在 cache miss 分支里直接跑 6 个 Provider —— 在 cache 尚未 `Put` 的窗口期，并发 N 个请求每个都看到 miss 独立 fan-out，把"一次 cold build"放大为 N × 6 次上游读。这恰好破坏本 story 要防御的连接池耗尽场景。

引入 `golang.org/x/sync/singleflight`：
- 以 `userID` 为 key，winner 内部再 `cache.Get` 一次（防止前一个 singleflight winner 刚写完的竞态），然后调 Provider + Put
- 非 winner 阻塞在 `group.Do`，winner 返回后同步收到相同的 `ResumeSnapshot`
- 每个调用者仍然独立盖 `serverTime`（在 singleflight 之外）—— 合并的是 build，不是响应

### 故障策略：fail-open（cache）/ fail-closed（provider）

这是本 story 最值得看清的设计决策：

- **Cache Get / Put 错误**：log warn 降级到 provider，**响应照常返回**。cache 是性能层，Redis 短暂不可用不得阻塞所有 session.resume。
- **Provider 错误**：`dto.ErrInternalError.WithCause(err)` 上抛，客户端收到 error envelope。若 friend 查询失败却返回空 friends list，客户端会当成"我没好友"是严重误导。

这和 Story 0.11 对 blacklist / rateLimiter 的 **fail-closed** 是不同场景：0.11 是**安全关**（Redis 挂了放行 = J4 事件重现），0.12 是**性能关**（Redis 挂了拒绝所有 session.resume = 可用性崩盘）。用户"反对 backup/fallback 掩盖核心风险"的反馈只适用于安全关——这里 provider 真的 fail-closed、只有 cache 这层 fail-open，不掩盖任何业务故障。

### release 模式不注册 session.resume（review round 1 的第二个修复）

反直觉但正确：release 模式里所有 Provider 都是 Empty，返回 `user=null / friends=[] / ...`，客户端无法区分这是"系统未就绪"还是"合法新账户"。在 Story 1.1 真实 UserProvider 落地前，release 部署注册 session.resume 反而会污染客户端状态。

现在：
- **debug 模式**：注册 session.resume（Story 0.15 Spike-OP1 集成测试矩阵仍可跑、本 story 集成测试仍跑）
- **release 模式**：**不注册**，handler 仍然构造（保留 `ResumeCacheTTLSec <= 0 → log.Fatal` 的启动期配置校验），`log.Info` 明确说明为什么没注册

Story 1.1 把 `UserProvider` 做成真实的那天这个 gate 可以逐步放开。

### 配置（AC6）

- `WSCfg` 新增 `ResumeCacheTTLSec int` toml tag `resume_cache_ttl_sec`
- `config/default.toml` `[ws]` 追加 `resume_cache_ttl_sec = 60`（对齐 NFR-PERF-6）
- `applyDefaults` 填 60（给 `config/local.toml` 薄配置留兼容）
- `mustValidate` `<= 0 → log.Fatal`（同 0.11 round 1 教训）

### 装配（cmd/cat/initialize.go）

在 `NewDispatcher` 之后构造 `redisx.NewResumeCache` + `ws.NewSessionResumeHandler`（注入 6 个 Empty Provider），然后分流：
- debug 模式：`dispatcher.Register("session.resume", handler.Handle)` + log.Info
- release 模式：blank-assign handler（跑构造期校验）+ log.Info 明确为什么没 Register

## 怎么实现的

**为什么 6 个 Provider 各自独立接口而不是一个 `ResumeDataSource` 大接口**：每个 Provider 对应一个独立 Epic 的 story（User→1.1 / Friends→3.4 / CatState→2.2 / Skins→7.2 / Blindboxes→6.3 / RoomSnapshot→4.2），分开便于**渐进式接入**——Story 7.2 实施时只把 `EmptySkinsProvider` 换成 `RealSkinsProvider`，其他五个保持 Empty。合并成大接口意味着每加一个 Source 就要改所有实现类，违反 ISP。Story 4.5 的 epics 原文也明示这是正确方向。

**`json.RawMessage` 占位 Empty 的细节**：`json.RawMessage("null")` 序列化成 `null`，`json.RawMessage("[]")` 成 `[]`。**不能赋 `nil`**——会被 marshal 成 `null` 而非预期的 `[]`，客户端反序列化到 `[]Friend` 时拿到 nil 而非 empty slice，语义失真。Empty Providers 全部显式返回合法 JSON token。

**ResumeSnapshot 字段 = 6 内容 + 1 ServerTime**：struct 有 7 字段，但 Redis Hash 只存 6 个内容字段，ServerTime 每次 handler 响应前用 Clock 重新填。单元测试 `CacheHitSkipsProviders` 在命中前把 FakeClock advance 5 秒，断言响应的 ServerTime 是 +5s 的值——证明命中路径下 ServerTime 不是从 Redis 读出来的。

**singleflight 内部为什么要再读一次 cache**：纯 singleflight 只保证"同 key 的并发 build 合并成一次"，不保证"上一个 winner 刚写完的结果不被下一个 winner 重复 build"。考虑这个时序：goroutine A 首 miss → 进 singleflight → 跑 Provider → Put → 出 singleflight。此时 goroutine B 刚到 handler，cache.Get 还没跑；然后 goroutine B 进 singleflight（A 已经退出，B 是新 winner），如果 winner 函数不再读一次 cache，B 会重跑 Provider。所以 winner 第一步是 `cache.Get`，命中就直接返回。

**两个接口（ResumeCache + ResumeCacheInvalidator）而不是一个**：handler 需要读写（`Get` + `Put`），Service 层只需要失效（`Invalidate`）。合并接口会让 Service 代码被迫承担对整个缓存 IO 的依赖，实际上没用到。Go 结构化 typing 让同一个 `RedisResumeCache` 值自然满足两个接口，最小消费者契约 + 零重复代码。

**dispatcher.Register vs RegisterDedup 的 CI 层回归守卫**：如果有人未来误把 session.resume 改成 `RegisterDedup`，怎么在 CI 里抓住？单纯"同 envelope.id 两次返回一致"无法区分 dedup cache 和 resume cache。方案：集成测试 `TestIntegration_SessionResume_NotDeduped` 在两次同 id 调用之间**显式调 `ResumeCacheInvalidator.Invalidate`**，强制第二次必须走 Provider；若 dispatcher 误用 dedup，第二次会被 dedup 中间件判定为 replay 返回首次缓存（Provider 计数 = 1）。断言 `providerCalls == 2` 就是锁。

**fail-open 的日志形状**：所有 cache 层错误都写 `Warn` 级、`action="resume_cache_{get,put}_error"`，**不**写 Error（error 级在监控上会触发告警，而 Redis 抖动不应惊动 oncall——真正的 Mongo/Provider 故障才会触发 session_resume 层面的 Error，通过 dispatcher 转成 `INTERNAL_ERROR` envelope 给客户端）。

**M9 Clock 注入**：三处用到 Clock——handler 的 `serverTime` + `durationMs`、`RedisResumeCache` 虽然目前只持 `ttl` 不测时间但构造期强制注入（为未来加 built-at metadata 留口子、保持与 0.11 `NewConnectRateLimiter` 同风格）。`check_time_now.sh` 扫 `internal/ws/*` 和 `pkg/redisx/*` 干净。

## 怎么验证的

**单元测试（`session_resume_test.go`，16 用例，`t.Parallel()` + FakeClock）**：
- 首次 resume → Provider 各调 1 次 + cache Put + serverTime = Clock.Now()
- 命中 → Provider 零调用 + serverTime 是推进后的 Clock.Now()（证明不从缓存读）
- Invalidate 后重查 → Provider 再各调 1 次
- cache.Get 错 → fail-open 成功响应 + Provider 仍被调
- cache.Put 错 → fail-open 成功响应
- Provider 错 → fail-closed 返回 `INTERNAL_ERROR` + `errors.Is` 保留 cause
- Empty Provider → 合法 JSON（`null` / `[]`）
- 构造器 nil 矩阵 → 8 子测 panic
- **singleflight 并发合并**：5 个 goroutine 同 userID 并发 Handle，用阻塞的 `blockingUserProvider` + `gate` channel 控制"第一个 goroutine 被卡住时后面的都在 singleflight 队列里等"，关 gate 后断言 UserProvider 调用次数 = 1（不是 5）

**RedisResumeCache 单元测试（`resume_cache_test.go`，8 用例，miniredis + FastForward）**：
- Put/Get 往返字段逐个 `assert.JSONEq`
- `mr.TTL("resume_cache:u1")` 验证 TTL 设上
- `mr.FastForward(61s)` 后 Get miss
- 不存在用户 Get 返回 `(_, false, nil)`
- Invalidate 后 Get miss
- Invalidate 不存在的 key 不报错
- 构造期 `clock=nil` / `ttl<=0` 全 panic
- 并发两个不同 userID 独立写，Invalidate 一个不影响另一个

**集成测试（`session_resume_integration_test.go` `//go:build integration`，miniredis + 真实 RedisResumeCache + 真实 Dispatcher + atomic counter Provider + `httptest.Server` + `gorilla/websocket`）**：
- `TestIntegration_SessionResume_Benchmark_10Calls_OneProviderHit`：同一 WS 连接发 10 次 session.resume，断言每个 Provider 的 atomic callCount = 1（epics AC "10 次 resume 只触发 1 次 Mongo" 在 Epic 0 阶段的等价表达）
- `TestIntegration_SessionResume_InvalidateRefetches`：第一次 resume → Provider 计数=1 → 测试代码直接调 `cache.Invalidate` → 第二次 resume → Provider 计数=2
- `TestIntegration_SessionResume_NotDeduped`：同 envelope.id 两次，中间 Invalidate，两次都 `OK=true` 且 Provider 计数=2（见前面的"CI 层回归守卫"设计说明）

**构建验证**：
- `bash scripts/build.sh --test`：全绿
- `go test -tags=integration ./internal/ws/...`：Story 0.9 / 0.10 / 0.11 / 0.12 累计 10+ integration cases 全过
- `check_time_now.sh`：`internal/ws/session_resume.go` / `pkg/redisx/resume_cache.go` 无裸 `time.Now()`

## 后续 story 怎么用

- **Story 1.1 (Sign in with Apple)** — 落地第一个真实 Provider（`UserProvider`），更新 `cmd/cat/initialize.go`：`User: NewRealUserProvider(userRepo)` 替换 `EmptyUserProvider{}`。放开 release 模式的 session.resume 注册 gate 可以在这一刻或更晚决策（还有 5 个 Empty 时继续 gate 也合理）
- **Story 1.5 (profile.update)** — Service 层持 `invalidator ws.ResumeCacheInvalidator`，写入 `users` 成功后调 `invalidator.Invalidate(ctx, userID)`；接口本身是 ws 包的，pkg 依赖方向是 `internal/service → internal/ws → pkg/redisx`，合规
- **Story 2.2 (state.tick)** — state_service.Apply 成功后同样 `invalidator.Invalidate`；同时落地 `CatStateProvider`
- **Story 3.2 (friend.accept)** — 事务成功后**双向**调 invalidator.Invalidate（双方的 resume cache 都要失效）；Story 3.4 落地 `FriendsProvider`
- **Story 6.3 / 6.4 (blindbox inventory / redeem)** — 落地 `BlindboxesProvider`；redeem 成功后 invalidator.Invalidate（blindboxes + skins + points 均变动）
- **Story 7.2 / 7.3 (unlocked skins / equip)** — 落地 `RealSkinsProvider`；equip 成功后 invalidator.Invalidate
- **Story 4.2 / 4.5 (RoomSnapshot / 完整 session.resume handler)** — Story 4.5 把本 story 的 skeleton 扩为完整的 FR25 handler（处理 `lastEventId` 字段、补 OP-4 增量协议 hook）；Story 4.2 落地 `RoomSnapshotProvider`
- **Story 0.15 (Spike-OP1 watchOS WS 稳定性)** — 需要 session.resume 已注册（debug 模式），本 story 交付满足该前置
- **Story 1.6 (账户注销)** — 调 `invalidator.Invalidate` 清理被注销用户的缓存（防止登录复活时看到残留状态）
