# Story 0.13: APNs 推送平台（Pusher 接口 + 队列 + 路由 + 410 清理） — 实现总结

给整个服务端搭一个统一的 APNs 推送底座：业务代码只面对 `push.Pusher` 接口 `Enqueue`，底层 Redis Streams 队列 + 消费者 + 指数退避 + DLQ + token 410 清理 + quiet-hours 降级全部异步跑。这是 Epic 0 最后一块实质性基座，为 Story 5.2 (touch 离线回退)、6.2 (盲盒掉落)、8.2 (冷启动召回)、1.5 (quietHours 降级) 铺好同一条"fire-and-forget 的推送公路"。

**本 story 交付骨架**：Pusher 接口 + RedisStreamsPusher 实现 + APNsWorker Runnable + 4 个 Provider interface（全 Empty 实现）+ 日志/退避/DLQ/410 全路径 + 30 天 token 清理 cron。真实的 `TokenProvider / TokenDeleter / TokenCleaner / QuietHoursResolver` 由 Story 1.4 (apns_tokens 仓库) / 1.5 (用户 quietHours / timezone) 逐个补。Epic 0 无业务方消费，`initialize.go` 只在 `apns.enabled=true` 时构造完整链路。

## 做了什么

### Pusher 接口 + Payload（internal/push/pusher.go）

- `Pusher` interface —— `Enqueue(ctx, userID, PushPayload) error`，是所有未来业务服务调用 APNs 的唯一入口
- `PushPayload` struct，字段全部 exported（service 层直接构造）：
  - `Kind PushKind` —— `PushKindAlert` / `PushKindSilent`，必填
  - `Title` / `Body` —— alert 必须有 Title，silent 忽略两字段
  - `DeepLink` —— 客户端 tap 跳转用，放在 APS custom `deepLink` 字段
  - `RespectsQuietHours` —— true + 收件人在 quiet 窗口 → worker 在 consume 时强制降级为 silent（见下）
  - `IdempotencyKey` —— 非空时 `SET apns:idem:{key} "1" NX EX 300`，5 分钟内重复 Enqueue 直接 return nil（NFR-SEC-9）
- `NoopPusher{}` —— 满足 Pusher 接口但什么都不做，Epic 0 `apns.enabled=false` 时 `initialize.go` 注入它保证下游 service 永远不拿到 nil 接口

### Redis Streams 队列层（pkg/redisx/stream.go + internal/push/pusher.go）

- `pkg/redisx/stream.go` 新建 `StreamPusher` / `StreamConsumer` 两个薄包装：
  - `StreamPusher.XAdd` —— 单条目写入 `apns:queue`
  - `StreamConsumer.EnsureGroup` —— `XGROUP CREATE MKSTREAM ... $`，对 `BUSYGROUP` 错误宽容（多副本启动幂等）
  - `StreamConsumer.Read` —— `XREADGROUP BLOCK count COUNT n`，`redis.Nil` 翻译为 `nil, nil`
  - `StreamConsumer.Ack` —— `XACK`
  - 文件顶部 godoc 列出 `apns:queue / apns:dlq / apns:retry / apns:idem:{key}` 在 D16 key space 中的邻居（与 `event:* / ratelimit:* / blacklist:* / lock:cron:* / resume_cache:*` 严格隔离）
- `RedisStreamsPusher.Enqueue` 流程：
  1. 校验 Kind 合法 + alert 必须有 Title（违反 → `VALIDATION_ERROR`）
  2. 若有 IdempotencyKey：`SETNX apns:idem:{key}`；命中返回 `nil`；Redis 出错 **fail-open** 继续 XADD（见下故障策略）
  3. JSON marshal `queueMessage{UserID, Payload, Attempt=0, EnqueuedAtMs}` → XADD 到 `apns:queue`，字段 `{userId, msg, attempt}`（userId / attempt 冗余给运维 CLI 巡检看）
  4. XADD 失败 **fail-closed** 返回 `INTERNAL_ERROR`，调用方知道推送没入队
- 整个 Enqueue 是非阻塞的 —— XADD 回来立即 return，不等 APNs send（D3 / NFR-REL-5）

### Provider 消费侧接口（internal/push/providers.go）

4 个接口定义在 `internal/push/` 包（不是消费方各自的包），Epic 0 同文件提供 4 个 Empty 实现：

- `TokenProvider.ListTokens(ctx, userID) ([]TokenInfo, error)` —— 返回该用户所有已注册设备 (platform, deviceToken) —— Story 1.4 真实实现（读 `apns_tokens` 集合）
- `TokenDeleter.Delete(ctx, userID, deviceToken) error` —— APNs 410 后删除失效 token —— Story 1.4
- `TokenCleaner.DeleteExpired(ctx, cutoff) (int64, error)` —— 30 天未更新的 token 批量删除 —— Story 1.4
- `QuietHoursResolver.Resolve(ctx, userID) (quiet bool, err error)` —— Story 1.5 读 `users.preferences.quietHours + users.timezone`

接口之所以放 `internal/push/`（和 Pusher 同包）：push 包有来自 service / cron / 未来 WS 多个消费者的 fan-in，放任意一个消费包都会让其他消费者反向 import 违反 P2。这和 Story 0.12 `ResumeCache` 放 `internal/ws/`（WS 是唯一消费者）是截然相反的判断——**包本身就是抽象**，和 `io.Reader` 在 `io` 包同构。

### APNs Router（internal/push/apns_router.go）

- `APNsRouter.RouteTokens(ctx, userID)` 把 TokenProvider 返回的 `[]TokenInfo` 展开为 `[]RoutedToken{DeviceToken, Topic, Platform}`
- 路由规则（FR58）：`platform == "watch"` → `cfg.APNs.WatchTopic`；`"iphone"` → `cfg.APNs.IphoneTopic`；其他值 log warn 并**跳过**（不路由到任一 bundle，防止数据损坏或未来 schema 扩展时意外推到错误设备类）
- 同一用户 Watch + iPhone 两 token 会展开为两个 RoutedToken —— worker 对每个单独 Send（APNs 协议每条 Notification 绑定一个 DeviceToken，不能多播）

### APNsClient（internal/push/apns_client.go + apns_sender.go）

- `ApnsSender` 接口 `Send(ctx, *apns2.Notification) (*apns2.Response, error)` —— 测试可 fake，worker 通过接口而非具体类型依赖
- `apnsClient` 封装 `github.com/sideshow/apns2` v0.25.0（NFR-INT-2 指定库）：
  - 构造期校验 keyPath/keyID/teamID 非空 → 加载 `.p8` 私钥（PKCS#8 格式，`token.AuthKeyFromFile`）→ 构建 `token.Token{AuthKey, KeyID, TeamID}` → `apns2.NewTokenClient(tok).Production()` 或 `.Development()`，`production=true` 仅当 `cfg.Server.Mode=="release"`
  - 构造期 log `{action:"apns_client_init", mode, keyId, teamId}` —— keyId/teamId 都不是秘密，**不记录**私钥内容
  - `Send(ctx, n)` 先检查 ctx 已取消则直接返回，否则调 `Client.PushWithContext(ctx, n)` —— 关键设计：worker 关机时的 `ctx.Done()` 能立刻 abort HTTP/2 请求
- 测试密钥 `internal/push/testdata/test_key.p8` —— 用 `openssl ecparam + pkcs8` 本地生成的 ECDSA P-256，仅用于单元测试验 `.p8` 解析路径；README.md 明确标注非生产

### APNsWorker Runnable（internal/push/apns_worker.go）

这是本 story 最大的一块，实现了消息消费的完整状态机：

**结构**：
- `APNsWorker` 实现 Runnable `Name() / Start(ctx) / Final(ctx)`
- `APNsWorkerConfig` 聚合 `StreamKey / DLQKey / RetryZSetKey / ConsumerGroup / WorkerCount / ReadBlock / ReadCount / RetryBackoffsMs / MaxAttempts / InstanceID`
- 构造期对每个依赖 + 每个 config 字段做 nil/零值 panic 校验（Epic 0 精神：启动即验证，别把崩溃推到首条消息才炸）
- `Start` 同步跑 `EnsureGroup`（失败则 return err 让 `App.Run` 退出），然后 `wg.Go(WorkerCount 次)` 启动消费 goroutine + `wg.Go` 启动 1 个 retry promoter goroutine；每个消费者名字 `cfg.InstanceID + "-" + i`（消费者组需要唯一名字才能正确负载均衡）
- `Final` 发 cancel 等所有 goroutine 退出，bound 5s 超时（architecture §Graceful Shutdown line 218 的 APNs worker 预算子项）

**核心状态机 `handle(ctx, c, msg)`**：
1. JSON decode `msg.Values["msg"]` 到 `queueMessage`；失败 → DLQ + Ack
2. `qm.Payload.RespectsQuietHours` → 调 `QuietHoursResolver.Resolve`；返回 true + alert → 强制降级为 silent；**resolver 出错 fail-open 到 alert**（宁可吵醒用户也不要漏消息）
3. `APNsRouter.RouteTokens` → 错误视为 retryable；无 token → Ack + log（用户还没注册设备）
4. 对每个 RoutedToken：
   - `buildNotification(rt, payload, now)` → 设 `Topic / DeviceToken / Priority / PushType / Expiration=now+1h / APS payload`
   - `sender.Send(ctx, n)` → 分类响应：
     - `200` → log ok
     - `410` → 调 `TokenDeleter.Delete`（per-token，不标 retryable）
     - `400 / 403 / 404 / 413` → 永久失败 log error，不重试
     - `429 / 500 / 503` → 标 `anyRetryable = true`
     - transport error → 标 `anyRetryable = true`
     - `ctx.Canceled / DeadlineExceeded` → 标 `anyRetryable = true` + break 循环（让 retry 调度路径跑完）
5. 若任一 token 标了 retryable → `retryOrDLQ`；否则 → Ack

**退避 + DLQ**：
- `retryOrDLQ(qm)` 检查 `qm.Attempt+1 < MaxAttempts=4`：是则 `scheduleRetry`，否则 `xaddDLQ`（reason=`retries_exhausted`）+ Ack
- `scheduleRetry`：`qm.Attempt++` → `dueAtMs = clock.Now().UnixMilli() + RetryBackoffsMs[Attempt-1]`（`[1000, 3000, 9000]`）→ **pipeline** ZADD `apns:retry` + XACK `apns:queue`（原子交接：要么两个都做了，要么一个都没做 —— 后者下次消费者重连仍能看到 PEL 里的消息继续处理）
- `xaddDLQ`：写 `apns:dlq` stream 追加审计，字段 `{userId, msg, reason, attempts, failedAtMs}`

**Retry Promoter goroutine**：
- `promoteRetries(ctx)` —— 100ms `time.NewTicker`（调度原语不是时间源，M9 豁免）
- 每 tick 跑 `PromoteOnce`：`ZRangeByScore apns:retry [0, nowMs] LIMIT 100` → 每个成员 pipeline `ZREM + XADD apns:queue`（ZREM 保证 at-most-once promote）
- `PromoteOnce` 导出 → 集成测试 + 未来 ops 工具都能同步触发一次

### 关机时消息不滞留 PEL —— writeCtxFor helper（review round 1+2 的累计修复）

Round 1 最初版本：`sender.Send` 收到 `ctx.Canceled` 就直接 `return`，不 XACK 不 scheduleRetry —— 消息永久留在消费者组 PEL，重启后 worker 只 `XREADGROUP ">"` 不会重新读这条消息，等价数据丢失。Round 1 修复：Send 后的收尾分支用 `context.Background()+2s` 兜底。

Round 2 发现 Round 1 漏了：decode error / RouteTokens error / 无 token 三条**更早**的分支仍用原始 ctx 做 Redis 写。提取 `writeCtxFor(parent)` helper：

```go
func (w *APNsWorker) writeCtxFor(parent context.Context) (context.Context, context.CancelFunc) {
    if parent.Err() == nil {
        return parent, func() {}  // no-op cancel
    }
    return context.WithTimeout(context.Background(), 2*time.Second)
}
```

handle 的**每一条**收尾路径（decode DLQ / route-err retry / no-tokens ack / 主收尾 retry-or-ack）统一走 `writeCtxFor` + `defer cancel()`。单元测试 `TestHandle_*_CancelledCtx_*` 分别在 `ctx` 已取消前提下断言 `XPENDING.Count == 0` + retry/DLQ 条目写入成功，锁死 PEL 无残留契约。

### 30 天 token 清理 cron（internal/cron/apns_token_cleanup_job.go）

- `apnsTokenCleanupJob(ctx, cleaner, clock, retention)` —— `cutoff = clock.Now().Add(-retention)` → `cleaner.DeleteExpired(ctx, cutoff)` → log
- Scheduler 构造签名新增 `tokenCleaner push.TokenCleaner` + `tokenExpiry time.Duration` 两个参数
- `registerJobs` 追加 `s.addLockedJob("@daily", "apns_token_cleanup", ...)` —— 走 `locker.WithLock` 多副本去重
- Round 1 曾把保留期硬编码成 `const = 30`（忽略 `cfg.APNs.TokenExpiryDays`），修复后通过 Scheduler 把 `cfg.APNs.TokenExpiryDays * 24h` 注进来；新增 `TestApnsTokenCleanupJob_CutoffNonDefaultRetention` 用 `7*24h` 验证端到端一致性

### 配置（internal/config/config.go + config/default.toml）

`APNsCfg` 从 4 个字段扩到 18 个：

```toml
[apns]
key_id = ""
team_id = ""
bundle_id = ""
key_path = ""
watch_topic = ""
iphone_topic = ""
stream_key = "apns:queue"
dlq_key = "apns:dlq"
retry_zset_key = "apns:retry"
consumer_group = "apns_workers"
worker_count = 2
idem_ttl_sec = 300
read_block_ms = 1000
read_count = 10
retry_backoffs_ms = [1000, 3000, 9000]
max_attempts = 4
token_expiry_days = 30
enabled = false
```

- `applyDefaults` 对 11 个字段做零值回填（`config/local.toml` 薄配置仍能启动）
- 新增 `validateAPNs()` helper（`mustValidate` 调用）：
  - **总是校验**：正整数 `worker_count / idem_ttl_sec / read_block_ms / read_count / max_attempts / token_expiry_days > 0`，`retry_backoffs_ms` 非空且每项 > 0
  - **`enabled=true` 才校验**：`key_path / key_id / team_id / watch_topic / iphone_topic` 全部非空
- 禁用 push 的**唯一正确姿势**：`enabled = false`。不要靠把 `worker_count = 0` 来"关掉"——`mustValidate` 会 log.Fatal（沿用 0.11/0.12 同样的配置纪律）

### 装配（cmd/cat/initialize.go）

在 cron scheduler 构造之后、wsHub 之前插入：

```go
cronSch := cron.NewScheduler(
    locker, redisCli.Cmdable(), clk, push.EmptyTokenCleaner{},
    time.Duration(cfg.APNs.TokenExpiryDays)*24*time.Hour,
)

var pusher push.Pusher = push.NoopPusher{}
var apnsWorker *push.APNsWorker
if cfg.APNs.Enabled {
    sender, err := push.NewApnsClient(cfg.APNs.KeyPath, cfg.APNs.KeyID, cfg.APNs.TeamID,
                                       cfg.Server.Mode == "release")
    if err != nil { log.Fatal()... }
    streamPusher := redisx.NewStreamPusher(redisCli.Cmdable(), cfg.APNs.StreamKey)
    pusher = push.NewRedisStreamsPusher(streamPusher, redisCli.Cmdable(), clk,
                                         time.Duration(cfg.APNs.IdemTTLSec)*time.Second)
    router := push.NewAPNsRouter(push.EmptyTokenProvider{}, cfg.APNs.WatchTopic, cfg.APNs.IphoneTopic)
    apnsWorker = push.NewAPNsWorker(...APNsWorkerConfig{...}, ...EmptyQuietHoursResolver{}, EmptyTokenDeleter{}, clk)
}
_ = pusher  // Epic 0 无消费；Story 5.2/6.2/8.2/1.5 逐个接入
```

`App.runs` 在 `apnsWorker != nil` 时插入 `apnsWorker` 位置 = `mongo, redis, cron, wsHub, worker, http`；Final 反向顺序 `http → worker → wsHub → cron → redis → mongo` 对齐 architecture §Graceful Shutdown line 218。

## 怎么实现的

**为什么 `MaxAttempts = 4` 而不是 `3`**：epics 原文 "失败重试指数退避 3 次（基准 1s / 3s / 9s）"。"3 次"指**重试**次数——initial 失败（attempt=0）+ 3 次重试（attempts 1/2/3）= 4 次 send 总数。`RetryBackoffsMs` 数组长度 3 对应 attempts 1/2/3；`retryOrDLQ` 判断 `qm.Attempt+1 < MaxAttempts` —— attempt=3 是最后一次通过 retry path 跑的 send，下次进来 attempt+1=4 = MaxAttempts 就走 DLQ 分支。

**为什么 quiet-hours 在 consume 时而不是 enqueue 时做降级**：Enqueue 到 worker 的时间差最坏可达 1+3+9=13s 重试窗口，期间用户可能调整 timezone / quietHours。在 consume 时读最新状态就避免了"发送时用户已进入 quiet 但推送仍按 alert 发"。Story 1.5 profile.update 会显式失效 `resume_cache:{userId}`，但推送队列**没有**类似失效机制——只能在最后一刻读。

**为什么 retry 用 Redis ZSET 而不是 `time.Sleep`**：
- 多副本就绪（NFR-SCALE-3）：ZSET 在 Redis 里，重启后 retry 可继续；`time.Sleep` 在进程内，重启丢
- 不阻塞 worker：一条消息 Sleep 9s 期间该 goroutine 不能处理其他消息；ZSET + 非阻塞 promoter 让 worker 始终可用
- 可测试：FakeClock advance + `PromoteOnce` 精确控制时序
- 坏处：ZSET 多 1 次 ZRANGEBYSCORE/ZREM 每 100ms，对 MVP 量级（<100 push/s）可忽略

**为什么 promoter ticker 用 `time.NewTicker` 而不走 `clockx.Clock`**：`clockx.Clock` 接口只有 `Now()`，没有 `NewTicker / Sleep`——有意为之（FakeClock 的推进是测试中的显式 step，不是后台 ticker）。在 promoter 里**"什么时候 tick"用 real clock**、**"该不该 promote"用 injected clock**（`clock.Now().UnixMilli()` 做 scoring）。测试把 FakeClock advance 1.1s 然后 `PromoteOnce` 同步触发 —— 不靠 ticker 的运气。

**为什么 APNs 410 不触发 retry 但其他 5xx 触发**：410 "Unregistered/BadDeviceToken" 是**永久**失败——token 已经失效，重试只会得到同样的 410。FR43 要求立即删除。其他 5xx / transport 错误是**瞬时**——APNs 服务抖动、网络闪断，retry 有成功可能。4xx non-410（400/403/413）也是**永久**（payload 格式错 / auth 错 / 超大），记 error 不 retry。

**为什么每个 token 独立判断 retry 合格性**：一个用户可能同时注册 Watch + iPhone token。Watch 返回 410（设备卸载）应立即删除 Watch token；iPhone 返回 200 应正常 ACK——同一条队列消息里两件事。`handle` 的 token 遍历：聚合"有 retryable 错的 token"（`anyRetryable=true`）→ 整条消息重试；所有 token 要么成功要么永久失败 → ACK。**但 retry 会重发给那些已经成功的 token**——这是 MVP 的合理权衡：用户收到重复通知（特别高并发时）是 uncomfortable 但 recoverable；引入 per-token retry state 就需要 ZSET 存 (msg, token) 元组而非 msg JSON，工作量翻倍。未来 Story 5.2 评估能否用 `IdempotencyKey` 作为 APNs collapse-id 在 APNs 侧去重。

**为什么 Pusher 接口放 `internal/push/` 而不是某个消费方包**：Story 0.12 `ResumeCache` 的唯一消费者是 `internal/ws/handlers/session_resume.go`，放 WS 包合理。对 Pusher，消费者横跨 `internal/service/touch / blindbox / cold_start_recall / profile`，放任何一个消费包都会让另外几个反向 import。解法：放 `internal/push/`（抽象包本身），所有消费者 `import "internal/push"` 只拿到接口 + payload struct，impl 也在同包但消费者 import 的是"push 抽象"——和 Go 标准库 `io.Reader` / `io.Writer` 在 `io` 包的决策同构。

**故障策略：fail-open（性能关）vs fail-closed（安全关）矩阵**（AC11）：

| 关注点 | 模式 | 理由 |
|---|---|---|
| `Enqueue` XADD 错 | fail-closed | 调用方的业务写已经 commit，把错误抛出去让 service 决定（重试 / 记账 / 回滚） |
| `Enqueue` IdempotencyKey SETNX 错 | fail-open 继续 XADD | 重复推送 < 丢推送 |
| Worker TokenProvider 错 | retry via ZSET | Mongo 瞬时——retry 再给一次机会，用尽 → DLQ |
| Worker QuietHoursResolver 错 | fail-open 到 alert | 吵醒 < 漏消息 |
| Worker APNs 410 | 删除 token 不 retry | FR43；per-token |
| Worker APNs 4xx non-410 | 不 retry log error | 永久；per-token |
| Worker APNs 5xx / transport | retry | 瞬时 |
| Cron TokenCleaner 错 | log + skip | 下次 @daily 再试 |

这和 Story 0.11 的 blacklist/ratelimit 是**安全关**（fail-closed，Redis 挂了放行 = J4 重现）不同；也和 Story 0.12 resume cache 是**性能关**（fail-open，Redis 挂了拒绝全部 resume = 可用性崩盘）不同——0.13 是**交付管道**，不同环节不同分类。用户"反对 backup/fallback 掩盖核心风险"的反馈适用于安全关，不适用于性能关。

**ZSET score 用 UnixMilli 而不是 UnixNano**（延续 0.11 round 2 的结论）：Redis ZSET score 是 double，精确表示整数到 2^53 ≈ 9e15；UnixNano 1.7e18 级 round-trip 损失百纳秒级精度，导致边界 `dueAt == now` 出现伪正负数；UnixMilli 1.7e12 在 2100 年前都稳稳落在精度内。

**ZREM-first vs ZPOPMIN**：ZPOPMIN 返回最小 score 成员但没法过滤 "score ≤ now"——会 pop 未来的 retry 然后要 put 回去，产生 race。ZRANGEBYSCORE + ZREM 把过滤和删除原子化（per-member），代价是两次命令，但 pipeline 一次 RTT。

## 怎么验证的

**单元测试（`internal/push/*_test.go`，全部 `t.Parallel()`，miniredis + FakeClock）**：

- `pusher_test.go` 7 用例：XADD 正常 + idempotency 去重 + SETNX 错 fail-open + alert 空 Title 校验 + 非法 Kind 校验 + 构造期 nil 矩阵 panic + NoopPusher
- `apns_router_test.go` 6 用例：Watch→WatchTopic + iPhone→IphoneTopic + 混合平台 + 未知平台跳过 + Empty Provider 空返回 + Provider 错上抛
- `apns_sender_test.go` 3 用例：空 keyPath/keyID/teamID 错 + 非法 PEM 错 + 有效 .p8 成功
- `apns_worker_test.go` 14 用例（含 review round 2 的 3 个 `*_CancelledCtx_*`）：
  - 全成功 → Ack + 无 retry
  - 无 token → Ack + 无 send
  - 410 → deleter 被调 + 无 retry
  - 500 → retry 调度且 dueAt 准确
  - 连续 3 次 500 → 退避 1s/3s/9s 精确递进（FakeClock.Advance）
  - attempt=MaxAttempts-1 失败 → DLQ + 无 retry
  - quiet=true alert → 降级 silent（断言 PushType=background）
  - quiet resolver 错 → fail-open alert
  - 未知平台 token 跳过（sender 不被调）
  - decode error → DLQ
  - 403 permanent → 无 retry
  - 构造期 nil/空矩阵 panic（8 子测）
  - **TestHandle_DecodeError_CancelledCtx_StillDlqAndAcks** —— `ctx` 已 cancel 前提下 decode error 仍 DLQ + PEL 空
  - **TestHandle_RouteError_CancelledCtx_SchedulesRetry** —— `ctx` 已 cancel 前提下 route error 仍 retry 调度 + PEL 空
  - **TestHandle_NoTokens_CancelledCtx_StillAcks** —— `ctx` 已 cancel 前提下无 token 仍 Ack + PEL 空
- `stream_test.go` 4 用例：StreamPusher XADD + EnsureGroup idempotent (BUSYGROUP 宽容) + Read/Ack round-trip + BLOCK 超时返回空
- `internal/cron/apns_token_cleanup_job_test.go` 3 用例：30-day 默认 cutoff + **7-day 非默认 cutoff 锚定 config 一致性** + cleaner 错误上抛
- `internal/config/config_test.go` 扩展 + 新增 `TestMustLoad_APNsDefaultsAppliedWhenSectionOmitted` 验证 `[apns]` 段缺失时 11 个默认值自动回填

**集成测试（`//go:build integration`，miniredis + fake sender，5 用例）**：
- `TestIntegration_APNs_EndToEnd_Success` —— Pusher.Enqueue → 真实 worker goroutine 读 → Send 返回 200 → Ack；断言 sender.Calls == 1
- `TestIntegration_APNs_RetryPromotion_Succeeds` —— 首次 500 → 调度 retry → FakeClock.Advance(1.1s) + PromoteOnce → 再 send 200 → 最终 retry ZSET 清空
- `TestIntegration_APNs_MaxRetries_DLQ` —— 连续 4 次 500 → 3 次 advance + promote → DLQ 获得 1 条目；断言 sender.Calls == 4
- `TestIntegration_APNs_410_DeletesToken` —— 单个 410 → deleter.calls 含 (userID, token) + 无 retry
- `TestIntegration_APNs_IdempotencyDedupes` —— 同 IdempotencyKey 两次 Enqueue + 一路失败到 DLQ → DLQ 只有 1 条（证明第二次 Enqueue 没入流）

**构建验证**：
- `bash scripts/build.sh --test` —— 全绿，含 `go vet` + `check_time_now.sh`（`internal/push/*` + `internal/cron/apns_token_cleanup_job.go` 无裸 `time.Now()`，promoter 的 `time.NewTicker` 是调度原语 M9 豁免）
- `go test -tags=integration ./internal/push/ ./internal/cron/` —— 全绿
- `go mod tidy` diff —— 只新增 `github.com/sideshow/apns2 v0.25.0` + 其 indirect `golang-jwt/jwt/v4 v4.4.1`

## 后续 story 怎么用

- **Story 1.4 (apns_tokens 注册 endpoint)** —— 落地三个真实 provider：
  - `RealTokenProvider`（`ListTokens` 查 `apns_tokens` 集合，index on userId）
  - `RealTokenDeleter`（410 后按 (userID, deviceToken) 删，FR43）
  - `RealTokenCleaner`（DeleteMany updatedAt < cutoff，NFR-SEC-7）
  - `cmd/cat/initialize.go` 把 `push.EmptyTokenProvider{} / EmptyTokenDeleter{} / EmptyTokenCleaner{}` 替换为真实实现
- **Story 1.5 (profile.update + quietHours)** —— 落地 `RealQuietHoursResolver`（读 `users.preferences.quietHours + users.timezone`，时区感知地判断"现在是不是 quiet"）；同时是本 story 之后第一个 Pusher 消费者雏形
- **Story 5.2 (touch offline → APNs fallback)** —— 第一个真实业务消费者。`TouchService.SendTouch` 注入 `push.Pusher`：WS 在线分支正常发 RPC；离线分支调
  ```go
  pusher.Enqueue(ctx, receiverID, push.PushPayload{
      Kind: PushKindAlert, Title: senderName, Body: "sent you a touch",
      DeepLink: "cat://touch?from="+senderID,
      RespectsQuietHours: true,
      IdempotencyKey: touchEnvelopeID,
  })
  ```
  Enqueue 必须**在 Mongo 事务 commit 之后**调用（D10 边界：Mongo/Redis 是两个系统，事务 abort 后推送已入队会产生孤儿推送）
- **Story 5.5 (跨时区免打扰)** —— 实际是 Story 1.5 `RealQuietHoursResolver` 在 touch 场景的端到端验证；Story 5.2 的 `RespectsQuietHours: true` 在本 story 已经落地的 consume-time 降级路径上直接生效
- **Story 6.2 (盲盒掉落 push)** —— cron job 触发 `pusher.Enqueue(PushKindAlert, "新盲盒掉落", ..., RespectsQuietHours=true)`；IdempotencyKey 用 `"blindbox_"+dropCycleId+"_"+userId` 防 cron 重入重复推送
- **Story 8.2 (冷启动召回 push)** —— 类似 6.2，cron 扫"N 天未登录"用户 → `pusher.Enqueue`，IdempotencyKey 用 `"recall_"+userId+"_"+date` 防当日重复召回
- **Story 0.15 (Spike-OP1 watchOS WS 稳定性测试矩阵)** —— 若测试场景需要同时验证 WS 离线路径，打开 `apns.enabled = true` + 真实 .p8（开发账号）即可跑通端到端
- **Story 1.6 (账户注销)** —— 注销流程调 `RealTokenDeleter` 批量删该用户所有 token（不等 30 天 cron）；本 story 的 `TokenDeleter.Delete` 签名按单 (userID, token) 设计，1.6 若需要按 userID 批删，可在 Story 1.4 的 Repository 上加 `DeleteByUser(userID)` 方法而不改 Push 包接口
- **DLQ 运维工具**（未来独立小 story）—— 写 `server/tools/apns_dlq_dump` 消费 `apns:dlq` stream，按 reason/userId 聚合统计，辅助定位推送失败根因
