# Story 0.10: WS 上行 eventId 幂等去重 — 实现总结

为 WebSocket 上行的"权威写"消息提供基础设施：同一请求（eventId）在 5 分钟窗口内重发不会被处理第二次，客户端看到的是第一次的原始响应。这是盲盒零重复领取、触碰不重复送达等业务一票否决特性的第一道防线（NFR-SEC-9、NFR-REL-3、FR57）。

## 做了什么

### 去重基础设施（核心交付）
- `internal/ws/dedup.go` 定义消费方接口 `DedupStore`：`Acquire / StoreResult / GetResult`，以及包装 HandlerFunc 的 `dedupMiddleware`
- `pkg/redisx/dedup.go` 实现 `RedisDedupStore`：
  - Redis key `event:{scopedID}` 通过 `SETNX EX 300` 占位，值为 `"processing"` → 完成后翻到 `"done"`
  - Redis key `event_result:{scopedID}` 用 Hash 存 `{ok, payloadJSON, errorCode, errorMessage}`，同 TTL
  - 写 hash + 续 TTL + 翻 done 三步用 Pipeline 一次性下发
- `DedupResult` 实体放 `pkg/redisx`，`internal/ws` 用 `type DedupResult = redisx.DedupResult` 别名复用

### Dispatcher 双轨注册
- `Register(msgType, fn)` —— 非权威读（`users.me / friends.list / session.resume / ping / debug.echo`）继续用，不走 Redis
- `RegisterDedup(msgType, fn)` —— 权威写（未来的 `blindbox.redeem / touch.send / friend.accept / skin.equip / profile.update` 等）自动套 dedup middleware
- 同 msgType 在两套 API 之间重复注册 → panic（防配置漂移）
- `Dispatch` 错误分支支持 AppError 透传（`errors.As(err, &ae)` → 用 `ae.Code / ae.Message`；否则 fall back `INTERNAL_ERROR`）

### Middleware 流程（覆盖所有失败模式）
- 空 `envelope.id` → `VALIDATION_ERROR`，不触 Redis
- `Acquire` 成功 → 调 handler，`defer` 保证无论返回、panic 都写 `StoreResult`，panic 情况下 middleware 记录 `INTERNAL_ERROR` 后 repanic（外层 readPump recover 打 log）
- `Acquire` 失败（重复）+ `GetResult found=true` → 原样返回首次的 `{ok, payload, error}`
- `Acquire` 失败 + `found=false`（handler 还在执行 / 上次崩溃未及时写） → 返回 `EVENT_PROCESSING`（Category=`retry_after`，客户端稍后重试）

### Key Scoping（两轮 review 打磨出的关键设计）
- Round 1：原始实现只用 `env.ID` 做 key —— 跨用户 / 跨 msgType 的相同客户端 ID（"1", "2" 等）会误触发去重，改为 `userID + ":" + msgType + ":" + eventID`
- Round 2：普通 `:` 拼接仍不注射（`("a:b","c","d")` 与 `("a","b:c","d")` 都 → `"a:b:c:d"`；debugValidator 把 bearer token 原样当 userID，`env.Type` 也无格式校验，真实存在此风险），改为 `scopedDedupKey()` 用 length-prefix 编码 `"len:value:len:value:len:value"`，可证明任何不同三元组产生不同 key

### 错误码与配置
- `internal/dto/error_codes.go` 新增 `EVENT_PROCESSING`（HTTP 429, Category=retry_after）
- `WSCfg` 新增 `DedupTTLSec`，`config/default.toml` 默认 `dedup_ttl_sec = 300`（对齐 NFR-SEC-9 5 分钟窗口）
- `docs/error-codes.md` 同步新增条目（registry 与文档一致性由测试强制）

### 装配
- `cmd/cat/initialize.go` 构造 `redisx.NewDedupStore(...)` → 传入 `ws.NewDispatcher(store, clock)`
- Debug 模式同时注册 `debug.echo`（非 dedup）和 `debug.echo.dedup`（走 dedup），后者用于集成测试

## 怎么实现的

**为什么 `DedupResult` 放 pkg/redisx 而不是 internal/ws**：Go 接口采用 nominal typing —— `ws.DedupStore` 的 `StoreResult(ctx, id, DedupResult)` 参数类型必须和 `RedisDedupStore.StoreResult` 完全一致。而项目宪法要求 `pkg/` 不得引用 `internal/`。若把 `DedupResult` 放 `internal/ws`，`pkg/redisx` 就无法实现接口；放 `pkg/redisx` 是唯一让结构化 typing 成立的位置。在 `internal/ws` 用 `type DedupResult = redisx.DedupResult` 别名让消费方语义保留。

**clockx.Clock 注入**：Story 0.9 落地了 `check_time_now.sh` CI 守卫禁止 `internal/ws/` 直接调 `time.Now()`。本 story middleware 需要测 `durationMs`，解法是 Dispatcher 新增 `clock clockx.Clock` 字段 → `RegisterDedup` 时把 clock 传给 middleware → middleware 用 `clock.Now().Sub(start)` 测时间。

**length-prefix key 编码**：`fmt.Sprintf("%d:%s:%d:%s:%d:%s", len(u), u, len(t), t, len(e), e)` 是最小可证明注射的方案，比 SHA-256 hash 简单（无 crypto 依赖 / 可调试）。不同三元组要么第一段长度前缀不同，要么长度相同但字节在首个差异位置上不同 —— 任何情况都产生不同的 key。

**panic 的 defer 顺序细节**：middleware 在成功 Acquire 后进入一个匿名函数，里面 defer 处理 recover。如果 handler panic，recover 先 `StoreResult` 写入 `INTERNAL_ERROR`，再 `panic(r)` 重抛 —— 让 readPump 的 recover 继续打日志。defer 栈 LIFO，确保"持久化结果 → 重抛"的顺序。

**消费方接口定义原则**：`DedupStore` 是接口，放 `internal/ws`（消费方），实现放 `pkg/redisx`（提供方）—— 这是架构宪法 "accept interfaces, return structs" 的标准应用，测试用 fake store 替身而非真 Redis。

## 怎么验证的

- 单元测试：
  - `internal/ws/dedup_test.go` —— 8 个子用例覆盖首次成功 / 首次 AppError / 首次 panic / 重放成功 / 重放失败 / 重放未找到 → EVENT_PROCESSING / 空 envelope.id / 跨用户跨 msgType 不碰撞 / 字段含分隔符不碰撞
  - `internal/ws/dispatcher_test.go` —— Story 0.9 的 3 个回归（KnownType / UnknownType / InvalidEnvelope）保留，新增 duplicate-type panic / RegisterDedup panic / AppError 透传 / dedup 路径端到端
  - `pkg/redisx/dedup_test.go` —— miniredis table-driven 覆盖 TTL / Acquire 重复 / StoreResult + GetResult 往返（含空 payload / 有 payload / 错误结果）/ 过期 / 哈希往返
- 集成测试：
  - `internal/ws/dedup_integration_test.go`（`//go:build integration`）启动 httptest.Server + miniredis-backed DedupStore + 注册 `debug.echo.dedup` handler（atomic.Int64 计数）
  - 建连 → 连发 3 次相同 eventId 的 `{"n":1}/{"n":2}/{"n":3}` → 断言：handler 只跑 1 次；3 次响应 byte-identical 且 payload 是首次的 `{"n":1}`；Redis `event:{scoped} = "done"`、`event_result:{scoped}` hash `ok=true`
- 构建：`bash scripts/build.sh --test` 全绿；`go test -tags=integration ./internal/ws/...` 全绿；`check_time_now.sh` 通过

## 后续 story 怎么用

- **Story 0.11 (WS connect rate limit + 异常设备拒绝)**：与本 story 正交（rate limit 在 readPump 早于 dispatcher，dedup 在 dispatcher 内层）；如果设备被拉黑，连接在 upgrade 阶段就拒，根本到不了 dedup
- **Story 0.12 (session.resume 缓存 + 节流)**：`session.resume` 明确不走 dedup（AC7），继续用 `Register`
- **Story 2.2 / 5.2 / 6.4（权威写）**：调用侧改为 `dispatcher.RegisterDedup(...)` 即可；middleware 完全透明，handler 不需改签名
- **blindbox.redeem（Story 6.4）**：dedup（本 story）是第一道；Mongo conditional update `WHERE status=pending` 是第二道；`user_skins` unique index 是第三道 —— 三层防线叠加保证 NFR-REL-3 零重复领取
- **Dispatcher.NewDispatcher 签名变更**：任何后续创建 Dispatcher 的代码必须传 `(store, clock)`，store 可为 nil 但此时 `RegisterDedup` 会 panic（initialize.go 以外的场景如 integration test 可传 nil 只用普通 Register）
