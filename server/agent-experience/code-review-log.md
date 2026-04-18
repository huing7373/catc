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
