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
