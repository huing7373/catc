# 代码审查日志

记录每次代码审查的发现，供后续蒸馏提取编码规范。
实际修复内容见同一 git commit 的 diff（`git log --grep="fix(review)"` + `git show <hash>`）。

---

## [0-3-infra-connectivity-and-clients] Round 1 — 2026-04-17

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | Verify 未校验 issuer claim | pkg/jwtx/manager.go:85 | 接受其他服务/环境签发的 token，打穿多环境 token 边界 |
| 2 | patch | Verify 只检查 *SigningMethodRSA 而非钉死 RS256 | pkg/jwtx/manager.go:86 | 放行 RS384/RS512，不符合 NFR-SEC-2 RS256 唯一要求 |
| 3 | patch | Issue 整体赋值 RegisteredClaims，静默丢失调用方传入的 Subject/Audience/NotBefore | pkg/jwtx/manager.go:70 | 后续基于标准 claim 的授权或时序约束失效，调用侧无法感知 |
| 4 | patch | active_kid/old_kid 允许为空且 Verify 接受空 kid header | pkg/jwtx/manager.go:40,90 | 轮换配置错误时不 fail-fast，可能签发/接受无 kid token |
| 5 | patch | Redis MustConnect Ping 无超时保护 | pkg/redisx/client.go:19 | Redis 地址不可达时 initialize() 无限挂起 |
| 6 | bad_spec | WithTx 回调签名在 AC/task/dev notes 三处不一致（mongo.SessionContext vs context.Context） | pkg/mongox/tx.go:11 | mongo-driver v2 无 SessionContext；已统一 AC 为 func(context.Context) error |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过

## [0-3-infra-connectivity-and-clients] Round 2 — 2026-04-17

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | pkg/ 直接 import internal/config，违反项目分层约束 | pkg/mongox/client.go:11, pkg/redisx/client.go:10, pkg/jwtx/manager.go:14 | 架构违规：pkg/ 不得引用 internal/；改为各 pkg 定义 Options struct，initialize.go 做转换 |
| 2 | patch | JWT 过期时间未校验非正数，配置写错时静默签发已过期 token | pkg/jwtx/manager.go:50 | 故障延后到鉴权阶段才暴露；添加 AccessExpirySec/RefreshExpirySec <= 0 的 log.Fatal 校验 |
| 3 | patch | WithTx EndSession 复用事务 ctx，取消/超时时 cleanup 在失效上下文上执行 | pkg/mongox/tx.go:16 | session cleanup 可能失败或阻塞；改为 context.Background() |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过 + `go vet -tags=integration` 通过

## [0-3-infra-connectivity-and-clients] Round 3 — 2026-04-17

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | Verify 未要求 exp claim 必填，允许永不过期 token 通过验签 | pkg/jwtx/manager.go:120 | jwt/v5 默认 exp 非必填，无 exp 的 token 绕过过期校验；添加 jwt.WithExpirationRequired() |
| 2 | patch | App.Run 中 Start 失败后直接 log.Fatal，跳过所有 Final 清理 | cmd/cat/app.go:38 | Mongo/Redis 连接泄漏；改为 channel 通知→逆序 Final→os.Exit(1) |
| 3 | patch | WithTx 集成测试只覆盖成功路径，未验证回滚 | pkg/mongox/client_integration_test.go:68 | 添加 callback 返回错误→断言集合为空的回滚测试 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过 + `go vet -tags=integration` 通过

## [0-4-multi-dimensional-healthcheck-endpoint] Round 1 — 2026-04-17

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | onReady 在启动 Start goroutine 后立即调用，未等待 HTTP 端口绑定 | cmd/cat/app.go:50 | readyz 在端口未 bind 时就返回 200，违反"启动完成标志"语义；httpServer 改用 net.Listen 先绑端口再 Serve，App 通过 ReadySignaler 接口等待 |
| 2 | patch | healthz 依赖检查无超时边界，复用无 deadline 的 request context | internal/handler/health_handler.go:48 | 依赖卡住时探针一起卡死，p95 无法保证；添加 3s WithTimeout 上界 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过 + `go vet -tags=integration` 通过

## [0-5-structured-logging-and-request-correlation-id] Round 1 — 2026-04-17

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | WithRequestID/Ctx 基于 zerolog.Ctx 取 context logger，空 context 返回 disabled logger，字段注入静默失效 | pkg/logx/logx.go:48,52 | requestId/全局字段/请求日志链路在真实请求中全部失效；AC2/AC5/AC6/AC8 不成立 |
| 2 | patch | Logger 中间件手动查 c.Get("userId") 而非从 context logger 继承 | internal/middleware/logger.go:21 | logx.WithUserID() 注入的 userId 不会出现在 access log 中，API 链路与文档脱节 |
| 3 | patch | Logger 中间件在 c.Next() 后顺序执行写日志，panic 时 Logger 后半段不执行 | internal/middleware/logger.go:12, cmd/cat/wire.go:23 | 失败请求只有 recover error log 没有 access log，违反 AC6"每请求一条" |
| 4 | patch | Recover() 未检查 c.Writer.Written() 就强行写 500 JSON | internal/middleware/recover.go:20 | handler 已写部分响应后 panic，客户端收到混合响应 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过

## [0-5-structured-logging-and-request-correlation-id] Round 2 — 2026-04-17

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | bad_spec | Story 文档声明中间件顺序 Recover→RequestID→Logger，实际已修正为 Logger→Recover→RequestID | 0-5 story:44,130 | 后续开发者按文档回改会重新引入 panic 请求无 access log 的问题 |
| 2 | bad_spec | Story 文档描述 logx.Ctx 为 zerolog.Ctx 薄封装 + Logger 从 gin.Get 读 userId，实际已改为回退全局 logger + context logger 自动继承 | 0-5 story:108,116,199 | 后续开发者按旧文档接入会走回 disabled logger 的错误路径 |

**构建验证：** ✅ 文档修正，无代码变更

## [0-5-structured-logging-and-request-correlation-id] Round 3 — 2026-04-17

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | bad_spec | File List 中 wire.go 描述仍写旧中间件顺序 Recover→RequestID→Logger | 0-5 story:238 | 与已修正的 Task 4.1 和 Dev Notes 矛盾，文档内部不一致 |
| 2 | bad_spec | Completion Notes 测试计数自相矛盾："19 个"与"24 个子测试" | 0-5 story:211 | 验证记录不可靠；实际为 22 个顶层测试 / 31 个含子测试 |

**构建验证：** ✅ 文档修正，无代码变更

## [0-6-apperror-and-error-category-registry] Round 1 — 2026-04-17

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | intent_gap→patch | retry_after 分类缺 Retry-After header 语义：AppError 无延迟字段，RespondAppError 不设该 header | internal/dto/error.go:59 | RATE_LIMIT_EXCEEDED 返回裸 429，客户端无法知道何时重试 |
| 2 | patch | allCodes 切片自证正确：init() 基于 allCodes 生成 registry，漏加 sentinel 不会被发现 | internal/dto/error_codes.go:30 | AC4/AC5 "遗漏检测"形同虚设；添加源码扫描测试独立验证 sentinel 计数 |
| 3 | patch | 文档一致性测试只校验 code 名称存在，不验证 Category/HTTPStatus/Message 逐行匹配 | internal/dto/error_codes_test.go:219 | error-codes.md 某行填错仍能通过测试，不满足 AC7 |
| 4 | patch | 并行测试各自调 gin.SetMode 修改全局状态，存在竞态风险 | internal/dto/error_codes_test.go:148,168,186,203 | flaky 测试；改为 TestMain 统一设置 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过

## [0-6-apperror-and-error-category-registry] Round 2 — 2026-04-17

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | intent_gap→patch | retry_after 裸 sentinel 无默认 Retry-After header，客户端收到 429 但不知何时重试 | internal/dto/error.go:72 | RespondAppError 对 retry_after category 默认 Retry-After: 60 |
| 2 | patch | allCodes 切片与 registry 仍是两套维护：init() 遍历 allCodes 建 registry，漏加 sentinel 启动不报错 | internal/dto/error_codes.go:30,54 | 改为 register() 构造函数同时创建 sentinel 并注册到 registry，结构上消除遗漏可能 |
| 3 | patch | RespondAppError 对 typed-nil *AppError 无防护，errors.As 命中后 ae 为 nil 触发 panic | internal/dto/error.go:70 | 添加 ae != nil 守卫 |
| 4 | patch | TestCategoryHTTPStatus_Mapping 抽样 9/19 码，未测到的码可能带错误 HTTPStatus 过 CI | internal/dto/error_codes_test.go:150 | 改为遍历 RegisteredCodes() 全量校验 Category→HTTPStatus 范围约束 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过

## [0-6-apperror-and-error-category-registry] Round 3 — 2026-04-17

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | RegisteredCodes() 返回 map[string]*AppError，浅拷贝暴露全局 sentinel 可变指针 | internal/dto/error_codes.go:44 | 调用方修改返回值污染全局 sentinel；改为返回 map[string]AppError 值拷贝 |
| 2 | patch | NewAppError 公开构造函数不校验 category，包外可构造空/无效 category 的 AppError | internal/dto/error.go:30 | 分类不变量被绕过；添加 validCategories 集合 + panic 守卫 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过

## [0-8-cron-scheduler-and-distributed-lock] Round 1 — 2026-04-18

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | cron.New() 无 Recover 中间件，job panic 向上穿透到 cron worker 导致进程崩溃 | internal/cron/scheduler.go:26 | 任何 job panic 终止整个服务器而非静默跳过该轮执行 |
| 2 | patch | addLockedJob 用 context.Background() 调 WithLock 和 job body，shutdown 取消信号传不进来 | internal/cron/scheduler.go:57 | Final() 等 cron.Stop() 完成时，阻塞在 Redis I/O 的 job 无法被取消，可能拖过 30s 关机上限 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过

## [0-8-cron-scheduler-and-distributed-lock] Round 2 — 2026-04-18

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | Start 用 context.Background() 而非继承 App.Run 传入的 ctx，shutdown cancel 在 Final() 调用前不可达 | internal/cron/scheduler.go:37 | shutdown 信号到达到 Final() 被调用之间的窗口期，运行中的 job 无法感知取消 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过

## [0-9-ws-hub-skeleton-envelope-broadcaster-interface] Round 1 — 2026-04-18

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | Unregister/unregisterClient 直接 close(c.send)，并发 publisher（BroadcastToUser/Dispatcher.sendResponse）仍在 select-send，触发 send on closed channel panic | internal/ws/hub.go:70-80 | 客户端正常断连时若有广播或 handler 响应在飞，进程级 panic 崩溃 |
| 2 | patch | GoroutineCount() 返回 连接数×2，但 health_handler 用 MaxConnections（连接上限）做比较阈值，~50% 连接数时就报不健康 | cmd/cat/initialize.go:69 | 服务在远未达连接上限时被探针标为不健康，触发误报告警或负载均衡摘除 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过

## [0-9-ws-hub-skeleton-envelope-broadcaster-interface] Round 2 — 2026-04-18

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | atomic bool 检查 + channel send 不是原子操作，closed.Load()=false 后另一 goroutine 执行 close(send)，前者进入 send 仍 panic | internal/ws/hub.go:116-133 | Round 1 修复未彻底消除竞态窗口；改为 done channel 驱动退出，send 永不关闭，trySend 用 select done/send/default 三路复用 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过

## [0-10-ws-upstream-eventid-idempotent-dedup] Round 1 — 2026-04-18

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | dedup middleware 直接用 env.ID 作 Redis key，未按 (userId, msgType) 命名空间隔离 | internal/ws/dedup.go:58-71 | 不同用户或不同 RPC 在 5min TTL 内复用相同客户端生成 ID（如 "1", "2"）会互相 EVENT_PROCESSING / 重放对方响应，破坏幂等正确性；scope 为 "{userId}:{msgType}:{eventID}" |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过 + `go test -tags=integration` 通过

## [0-10-ws-upstream-eventid-idempotent-dedup] Round 2 — 2026-04-18

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | Round 1 用 "userId:msgType:eventId" 普通冒号拼接，字段本身含冒号（debugValidator 把 bearer token 原样当 userID；Envelope.Type 无格式校验）时仍会映射到相同 key | internal/ws/dedup.go:62 | ("a:b","c","d") 与 ("a","b:c","d") 都 → "a:b:c:d"，跨三元组仍可能 EVENT_PROCESSING 或重放他人缓存；改为 length-prefix 编码 "len:v:len:v:len:v" 使函数注射 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过 + `go test -tags=integration` 通过

## [0-11-ws-connect-rate-limit-and-abnormal-device-reject] Round 1 — 2026-04-18

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | 新增必填 `ws.*` 字段却只有 `mustValidate` 严格校验，未提供 applyDefaults；MustLoad 仅解析单文件不 merge default.toml | internal/config/config.go:96-113 | 现有的 config/local.toml / local.toml.example 省略 [ws] 段的启动路径直接 `log.Fatal`，破坏"override 配置不必重复默认值"的既有契约；改为在 validate 前 applyDefaults 填零值 |
| 2 | patch | 用 INCR + EXPIRE NX 实现声称的"滑动窗口 60s ≤ 5"；TTL 仅首次设置，语义其实是固定窗口 | pkg/redisx/conn_ratelimit.go:75-78 | 客户端可以在窗口关闭前 5 次 + 窗口 reset 后立即 5 次 = 短时 10 次，绕过 NFR-SCALE-5 保证；改为 sorted set（ZADD 时间戳 + ZREMRANGEBYSCORE + ZCARD）做真正的滑动窗口，构造期注入 clockx.Clock 以便 FakeClock 驱动测试 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过 + `go test -tags=integration ./internal/ws/... ./tools/...` 通过

## [0-11-ws-connect-rate-limit-and-abnormal-device-reject] Round 2 — 2026-04-18

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | ZRemRangeByScore 用 `(cutoff` 保留 score == cutoff 的最老项，blocked 分支的 `d > 0 && d <= window` 守卫在 d == 0 时 retry 回退到 r.window | pkg/redisx/conn_ratelimit.go:122 | 客户端在"下一瞬间就该放行"的边界点反收到 60s Retry-After，按 header 退避被额外锁一个窗口；改 `d <= 0 → retry = 1ms`（ceilSeconds 向上取 1s）作为防御 |
| 2 | patch | ZSET 用 nanosecond Unix 时间戳作 score；float64 仅精确表示到 2^53 ≈ 9e15，1.7e18 级时间戳 round-trip 损失百纳秒级精度 | pkg/redisx/conn_ratelimit.go:98-106 | ageoutAt − now 的整数算术依赖 float→int round-trip，边界 d 本应为 0 的场景会出现伪正数或伪负数，让 Round 1 的回归测试伪通过；改为 Unix millisecond（1.7e12 < 2^53，精确表示），一并让上一条 d==0 边界成为可确定复现的测试条件 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过 + `go test -tags=integration ./internal/ws/... ./tools/...` 通过

## [0-12-session-resume-cache-throttle] Round 1 — 2026-04-18

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | cache miss 分支直接 fan-out 六个 provider 调用，同一 userID 并发请求不会合并；J4 Watch 重连风暴下 N 个请求各自独立触发 6 个 provider 调用，把 "一次 cold build" 放大成 N × 6 上游读 | internal/ws/session_resume.go:250-255 | 本 story 的核心目标就是保护 Mongo/provider 连接池免于 resume 风暴，但在缓存尚未 Put 完成的窗口期，第 N 个请求仍看到 miss 独立调 provider；引入 `golang.org/x/sync/singleflight`，按 userID 合并 in-flight build，winner 内部再读一次 cache 防止重复写入 |
| 2 | patch | `dispatcher.Register("session.resume", handler)` 无条件在所有模式下执行，release 模式里 handler 返回的 6 个字段全部来自 Empty*Provider（user=null / friends=[] / ...），与合法"新注册无好友"账号状态不可区分 | cmd/cat/initialize.go:61-69 | Story 1.1 真实 UserProvider 上线前的任何 release 部署都会把占位数据当真账户状态回给客户端，客户端 UI 会长期显示空帐号视图；gate 到 debug 模式（Story 0.15 Spike-OP1 的集成测试路径），release 模式明确不注册 + log.Info 说明；Story 1.1 起逐个 provider 真实化时再放开该 gate |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过 + `go vet -tags=integration ./...` 通过

## [0-13-apns-push-platform-pusher-queue-routing-410-cleanup] Round 1 — 2026-04-18

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | `sender.Send` 返回 `context.Canceled` / `DeadlineExceeded` 时 handle 直接 return，既未 XACK 也未 scheduleRetry；而 worker 只 `XREADGROUP ... ">"`（不回收 PEL 中的旧条目） | internal/push/apns_worker.go:267-270 | 进程关闭瞬间正在飞的 APNs 请求永远滞留在 consumer group 的 Pending Entries List，重启后不被重投 → 真正的消息丢失路径；改为 break 标记 anyRetryable，并在 ctx 已取消时用 `context.Background()` + 2s 兜底 ctx 做最终的 retryOrDLQ / Ack 写入，保证优雅关机也能把 PEL 排空 |
| 2 | patch | `apns_token_cleanup_job` 把保留期硬编码为 `tokenCleanupRetentionDays = 30`，完全忽略 config 新增的 `cfg.APNs.TokenExpiryDays`（且已经过 `validateAPNs` 强校验） | internal/cron/apns_token_cleanup_job.go:13-24 | 运营方把 token_expiry_days 改成非默认值（例如 7 天合规窗口）后，夜间作业仍按 30 天裁断，NFR-SEC-7 合规窗口与配置脱节，等效于"config 是装饰"；job 签名增加 `retention time.Duration` 参数，Scheduler 构造签名传入 `cfg.APNs.TokenExpiryDays*24h`，新增 TestApnsTokenCleanupJob_CutoffNonDefaultRetention 锚定端到端一致性 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过 + `go vet -tags=integration ./...` 通过 + `go test -tags=integration ./internal/push/ ./internal/cron/` 通过

## [0-13-apns-push-platform-pusher-queue-routing-410-cleanup] Round 2 — 2026-04-18

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | Round 1 的 detached writeCtx 只套在 Send 之后的收尾分支（原 handle 尾部），decode error / RouteTokens error / 无 token 三条早期分支仍然用原始（可能已取消的）ctx 做 xaddDLQ / retryOrDLQ / Ack；若 shutdown 恰在 quiet.Resolve / RouteTokens 期间到达，这几处 Redis 写全部失败，消息仍滞留 PEL；而 worker 只 `XREADGROUP ... ">"` 从不回收 PEL，等价于数据丢失 | internal/push/apns_worker.go:233,250,257 | 提取 `writeCtxFor(parent)` helper（parent 未 Done 时直接返回 + no-op cancel，否则 `context.Background()+2s`），覆盖 handle 的每一条收尾路径；新增 3 个单元测试（decode / route-err / no-token）在 `ctx` 已 cancel 的前提下断言 `XPENDING.Count == 0`，锁死 PEL 无残留契约 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过 + `go test -tags=integration ./internal/push/ ./internal/cron/` 通过

## [1-2-refresh-token-revoke-per-device-session] Round 1 — 2026-04-19

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | UpsertSession 在 `/auth/refresh` 旋转路径里只按 `_id` 过滤更新，未对 `sessions.<device>.current_jti` 做 compare-and-swap；两个并发请求都能通过 Step 4 reuse-detection 后 OVERWRITE 会话，违反 rolling-rotation 单用语义 | internal/repository/user_repo.go:164-167 | 竞态下两个 refresh 都返 200、但只有最后写入的 current_jti 存活；另一方客户端手里的新 token 在下次 refresh 会触发 reuse detection，直接烧掉活着的会话；新增 `UpsertSessionIfJTIMatches` + `ErrSessionStale` 哨兵；service 侧改用 CAS，落败者返 AUTH_REFRESH_TOKEN_REVOKED |
| 2 | patch | RefreshToken Step 6/7 顺序是 UpsertSession → Revoke(oldJTI)，一旦 blacklist 写失败，session.current_jti 已指向客户端未接收的 newJTI，下次同一 oldJTI 的 retry 必命中 reuse detection 烧掉 newJTI | internal/service/auth_service.go:617-627 | 单次瞬时的 Redis 写失败会把合法用户直接踢回 SIWA；改为 Revoke FIRST → UpsertSession LAST：Revoke 失败 ⇒ 500 + session 不动，客户端可用同一 oldJTI 重试自愈；反向的 Revoke OK + UpsertSession Mongo 失败是接受的复合故障长尾 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过 + `go vet -tags=integration ./cmd/cat/... ./internal/repository/...` 通过

## [1-2-refresh-token-revoke-per-device-session] Round 2 — 2026-04-19

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | Manager 新加 `issueClock` 字段与"Issue / Verify 共用注入时钟"的 godoc，但 `Verify` 的 `jwt.ParseWithClaims` 只传了 `WithIssuer + WithExpirationRequired`，没传 `WithTimeFunc(m.issueClock.Now)` —— Issue 走 FakeClock，Verify 退回 `time.Now()` | pkg/jwtx/manager.go:159,173 | refresh/expiry 集成测试的 FakeClock 夹具无效：token 发行时间走 2026-04-19 12:00 UTC 假时钟，15 分钟后真实墙钟把 access token 看成"已过期"；短 TTL 相关路径会在 CI 非此时此刻运行时回归；补 `jwt.WithTimeFunc(m.issueClock.Now)` + 两个单测 (`TestManager_VerifyAndIssue_ShareInjectedClock` / `TestManager_Verify_ExpiredAgainstInjectedClock`) 锁定双向契约 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过 + `go vet -tags=integration ./cmd/cat/... ./internal/repository/... ./pkg/...` 通过

## [1-3-jwt-auth-middleware-userid-context-injection] Round 1 — 2026-04-19

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | `Hub.DisconnectUser` 在 `FindByUser` 取 sync.Map 快照后无条件 `count++`，但 `unregisterClient` 内的 `LoadAndDelete` 可能因 readPump defer 自身已先一步 unregister 而返回 false（snapshot-vs-self-disconnect race），count 仍被加 1 —— 让 `unregisterClient` 返回 bool，仅在真正驱逐时计数 | server/internal/ws/hub.go:174-175 | DisconnectUser 的 `connectionsClosed` 返回值 + 审计日志会大于实际关闭的连接数；Story 1.6 账户注销 / 任何依赖该 count 做下游决策的 revocation flow 都会被误导（既违反 godoc "actually torn down" 契约，也让 ops 看到的关闭数偏多）；新增 `TestHub_DisconnectUser_RaceWithSelfDisconnect_DoesNotOvercount` 锁定 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过

## [1-3-jwt-auth-middleware-userid-context-injection] Round 2 — 2026-04-19

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | Round 1 新增的"race regression"测试在调 `DisconnectUser` 之前 `hub.Unregister("conn-stale")`，导致 `FindByUser` 快照根本不含 stale entry，循环只跑 1 次 —— 无论是否带 `if h.unregisterClient(c) { count++ }` 守护，count 都是 1。测试名声称覆盖 race window 但实际只覆盖"调 DisconnectUser 之前已经少一个连接"的退化场景；如果未来有人把 count++ 改回无条件，这条测试不会失败。 | server/internal/ws/hub_disconnect_user_test.go:184 (round-1 提交) | 守护代码失去"未来回归被测试拦下来"的保护层，Story 1.6 调用方依赖的 connectionsClosed 契约重新变成靠人 review；修法：把 DisconnectUser 的循环抽到 `disconnectAllForUser(userID, clients []*Client)` 测试 seam，新测试在 Unregister 之前**先**捕获 snapshot，再 Unregister，再调 helper 注入 stale snapshot —— 这样 loop 真正跑两次，guard 有作用时 count=1，无 guard 时 count=2，回归立刻被抓 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过

## [1-4-apns-device-token-registration-endpoint] Round 1 — 2026-04-19

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | `ApnsTokenService.RegisterApnsToken` 在 `limiter.Acquire` 返回 allowed=false 时拿到了 retry `time.Duration` 却只把它塞进 `WithCause` 的错误消息里，没调 `WithRetryAfter`；`dto.RespondAppError` 遇到 `CategoryRetryAfter` 且 `RetryAfter==0` 会 fallback 到硬编码 60s —— 客户端不论实际限流窗口剩 1s 还是 45s 都被告知等 60s | server/internal/service/apns_token_service.go:109-112 | 任何遵守 Retry-After 的客户端被系统性误导：窗口还剩 1s 时客户端傻等 60s（UX 差），窗口还剩 45s 时客户端以为只要 60s 可 Retry-After 其实已经过期重试会再 429（触发客户端重试风暴）—— 修法：加 `ceilRateLimitSeconds(d)` 私有 helper（镜像 ws/upgrade_handler 的 ceilSeconds 逻辑，独立一份保持 internal 包互不依赖），返错前 `.WithRetryAfter(ceilRateLimitSeconds(retry))`；单测加 `TestRegisterApnsToken_LimiterBlocks` 断言 `appErr.RetryAfter == 2`（2s 输入）+ 新增 `_SubSecondRetryRoundsUpTo1` 锁 §9.3 boundary 1ms→1s ceiling |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过

## [1-5-profile-preferences-displayname-timezone-quiethours] Round 1 — 2026-04-19

| # | 类别 | 错误模式 | 文件 | 影响 |
|---|------|---------|------|------|
| 1 | patch | Story 1.5 允许 quietHours-only 的 partial update，但 `RealQuietHoursResolver.Resolve` 在 `u.Timezone == nil \|\| *u.Timezone == ""` 时早退返回 `(false, nil)`（fail-open）—— fresh-SIWA 用户 timezone 默认 nil，客户端可成功保存 quietHours 但夜间 APNs 仍照旧响铃，直到某次 profile.update 把 timezone 补上。写入成功、运行期短路的语义不一致让用户的 quietHours 配置功能性失效。 | server/internal/push/real_quiet_hours_resolver.go:74-76 | 新 quietHours-only 代码路径对首次设置 quietHours 的 fresh-SIWA 用户功能失效，NFR-COMP 跨时区免打扰合规红线漏洞；修法：在 `service.ProfileService.Update` 加 preflight —— 若 `p.QuietHours != nil && p.Timezone == nil` 则 `repo.FindByID` 检查 persisted tz，stored tz 仍为空则返 `VALIDATION_ERROR`（"quietHours requires timezone to be set …"）；把 `FindByID` 加入 `profileUpdater` 消费接口；5 条新单测（QuietHoursOnly_NoExistingTimezone 拒 / ExistingTimezone 过 / AndTimezone_SkipsPreflight / PreflightRepoError + PreflightUserNotFound 传播）+ 1 条集成测试（`TestProfileUpdate_Integration_QuietHoursOnly_RejectedWhenTimezoneUnset` 端到端 SIWA→reject→set tz→retry ok）；docs bump `1.5.0-epic1 → 1.5.1-epic1`，`ws-message-registry.md` 与 `integration-mvp-client-guide.md §15.5` 文档化 quietHours↔timezone 耦合 |

**构建验证：** ✅ `bash scripts/build.sh --test` 通过 + `go vet -tags=integration ./...` 通过
