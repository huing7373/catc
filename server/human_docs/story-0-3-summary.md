# Story 0.3: 基础设施连通性与客户端 — 实现总结

为后续所有业务代码提供 MongoDB、Redis、JWT 三个基础设施客户端，统一连接管理、健康检查和生命周期。

## 做了什么

### MongoDB 客户端 (`pkg/mongox/`)

- `MustConnect(ConnectOptions)` — 启动期一次性连接 + Ping，失败直接 `log.Fatal`；超时由配置的 `TimeoutSec` 控制（默认 10s）
- `Client` 封装了 `*mongo.Client`，提供 `DB()` 拿数据库 handle、`Raw()` 拿原始 client（用于事务）、`HealthCheck(ctx)` 供后续 healthz endpoint 调用
- `WithTx(ctx, cli, fn)` — 事务辅助函数，封装 `StartSession` + `WithTransaction`；session cleanup 使用 `context.Background()` 避免在已取消的 context 上做清理
- 实现了 `Runnable` 接口（`Name/Start/Final`），`Final` 执行 `Disconnect`

### Redis 客户端 (`pkg/redisx/`)

- `MustConnect(ConnectOptions)` — 启动期 `NewClient` + Ping（带 10s 超时），失败 `log.Fatal`
- `Client` 封装了 `*redis.Client`，提供 `Cmdable()` 拿命令接口、`HealthCheck(ctx)`
- 同样实现 `Runnable` 接口，`Final` 执行 `Close`

### JWT RS256 管理器 (`pkg/jwtx/`)

- `New(Options)` — 从 PEM 文件加载 RSA 私钥（兼容 PKCS8 和 PKCS1 格式），支持双密钥轮换：`activeKey` 用于签发，`oldPub` 用于验证轮换期旧 token
- `Issue(CustomClaims)` — 用 active 私钥签发 RS256 JWT，header 写入 `kid`，自动设置 issuer/iat/exp（access 15min / refresh 30 天）
- `Verify(tokenStr)` — 按 header `kid` 选择公钥，强制 RS256（拒绝 RS384/RS512）、强制 issuer 匹配、强制 exp 必填
- 启动期校验：`active_kid`、`issuer` 不能为空，`access_expiry_sec`/`refresh_expiry_sec` 必须正数

### Testcontainers 测试辅助 (`internal/testutil/`)

- `SetupMongo(t)` / `SetupRedis(t)` — 通过 Testcontainers 启动一次性容器，返回已连接的 client + cleanup 函数
- 供后续 repository 层集成测试复用；`pkg/` 内的集成测试因无法 import `internal/` 而内联启动容器

### 配置重构

- `JWTCfg` 从对称密钥（`Secret/Issuer/Expiry`）重构为 RS256 双密钥（`PrivateKeyPath/PrivateKeyPathOld/ActiveKID/OldKID/AccessExpirySec/RefreshExpirySec`）
- `default.toml` 同步更新，`[mongo]` 补充 `timeout_sec`

### App 生命周期改进

- `App.Run()` 中 Runnable Start 失败不再直接 `log.Fatal`（会跳过 Final 清理），改为 channel 通知 → 逆序 Final 全部清理 → `os.Exit(1)`
- `initialize.go` 注册顺序 `mongoCli → redisCli → httpSrv`，逆序 Final 保证先停 HTTP 最后停 Mongo

## 怎么实现的

**分层解耦**：`pkg/` 包不依赖 `internal/config`，每个 pkg 定义自己的 Options struct（`mongox.ConnectOptions`、`redisx.ConnectOptions`、`jwtx.Options`），`initialize.go` 负责 config → options 的转换。这遵守了架构宪法的 `pkg/` 不引用 `internal/` 规则。

**mongo-driver v2**：与 v1 的关键区别是 `mongo.Connect()` 不接受 context 参数、`WithTransaction` 回调签名是 `func(context.Context) (any, error)` 而非 v1 的 `mongo.SessionContext`。

**JWT 双密钥轮换**：生产环境轮换密钥时，将旧密钥路径填入 `private_key_path_old`，旧 kid 填入 `old_kid`。Verify 按 token header 中的 kid 选择对应公钥，同时接受新旧两把密钥签发的 token。轮换完成后清空 old 配置即可。

## 怎么验证的

- `bash scripts/build.sh --test` — 20 个单元测试全部通过（config 2, app 3, handler 1, mongox 3, redisx 3, jwtx 10 含 round-trip/过期/错误 kid/旧密钥轮换/错误 issuer/错误签名算法/无 exp/registered claims 保留）
- `go vet -tags=integration` — 集成测试编译通过（实际运行需 Docker）
- 集成测试覆盖：MustConnect + HealthCheck + DB/Raw + WithTx 成功路径 + WithTx 回滚验证 + Redis Set/Get
- 经历 3 轮 code review，累计修复 11 个问题（issuer 校验、RS256 钉死、RegisteredClaims 保留、kid 非空校验、Redis ping 超时、pkg→internal 解耦、JWT expiry 校验、WithTx session cleanup、exp 必填、App.Run graceful failure、WithTx 回滚测试）

## 后续 story 怎么用

- **Story 0.4**（多维健康检查）：调用 `mongoCli.HealthCheck(ctx)` 和 `redisCli.HealthCheck(ctx)` 实现 `/healthz` 多维探针
- **Story 1.1**（Sign in with Apple）：在 `initialize.go` 中创建 `jwtx.New(opts)` 并注入 auth service
- **后续 repository 层**：通过 `mongoCli.DB()` 获取数据库 handle，用 `redisCli.Cmdable()` 执行 Redis 命令；跨 collection 写操作用 `mongox.WithTx(ctx, mongoCli.Raw(), fn)`
- **集成测试**：repository 层测试用 `testutil.SetupMongo(t)` / `testutil.SetupRedis(t)` 启动 Testcontainers
