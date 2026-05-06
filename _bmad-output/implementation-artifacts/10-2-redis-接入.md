# Story 10.2: Redis 接入（连接池 + 配置 + fail-fast + RedisClient 抽象）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want 在写第一行 server WS / presence / idempotency / rate-limit Redis-backed 业务代码之前，把 Redis 连接基础设施落地（`internal/infra/redis/` 目录提供连接初始化基于 YAML `redis.addr / password / db / pool_size` + 启动期 Ping 失败 fail-fast + `RedisClient` 抽象接口含 GET/SET/DEL/EXPIRE/SADD/SREM/SMEMBERS 7 个基础命令 + `RedisClientMock` 基于 `miniredis` in-process server 的测试替身 + dockertest 起真实 Redis 的集成测试，且与 Story 4.2 MySQL 接入的 fail-fast / env override / Deps wire 模式严格对齐），
so that 后续 Story 10.6 (Redis presence repo `room:{roomID}:online_users` + `user:{userID}:ws_session`) / Epic 4 限频升级（节点 9+ 把 RateLimit 从内存 token bucket 切到 Redis-based）/ Epic 20 宝箱开箱幂等键 `idem:{userId}:{apiName}:{key}` / Epic 32 合成幂等键 都能基于同一个 Redis 实例 + 同一个 client 抽象实装，不再出现"每个业务模块自己 import go-redis 直连，连接池 / fail-fast / mock 各写一套"的拼凑式代码。

## 故事定位（Epic 10 第二条 = 节点 4 第一个 Server Epic 内 Redis 基础设施奠基；对标 Story 4.2 在 Epic 4 的 MySQL 接入角色，本 story 是 10.3 ~ 10.7 全部 WS / presence Story 的强前置）

- **Epic 10 进度**：10.1 (WS 协议骨架文档锚定) done → **本 story (10.2 Redis 接入)** → 10.3 (WS 网关骨架) → 10.4 (心跳框架) → 10.5 (BroadcastToRoom primitive) → 10.6 (Redis presence repo) → 10.7 (房间快照下发框架)
- **强前置关系**：
  - 10.6 Redis presence repo（`room:{roomID}:online_users` SADD/SREM/SMEMBERS + `user:{userID}:ws_session` SET/GET/DEL + EXPIRE TTL）**必须**用本 story 提供的 `RedisClient` 抽象，**不**直接 import go-redis
  - 10.5 BroadcastToRoom 间接依赖（通过 10.6 的 presence ListOnline）
  - 10.3 WS 网关骨架（Session 创建钩子）将在 10.6 实装时挂 AddOnline/RemoveOnline 钩子
- **对 Epic 外的下游影响**：
  - **Epic 4 RateLimit**（Story 4.5 内存 token bucket 已落地）：节点 9+ 切 Redis-backed 时，RateLimit 中间件需消费本 story 的 `RedisClient` 抽象（YAML `ratelimit.backend: "memory" | "redis"` 字段后续扩，本 story **不**改 RateLimit 实装）
  - **Epic 20 ChestService.OpenCurrentChest**：`IdempotencyRepo.CheckAndLock(idempotencyKey)` 用 Redis SET NX EX 实装（跨 Redis 命令组合：SETNX + EXPIRE 或 SET key value NX EX seconds），**预留**调用本 story `RedisClient` 抽象（本 story **不**实装 idempotency repo，只确保抽象接口表达力够）
  - **Epic 32 ComposeService.Upgrade**：同 Epic 20，幂等键路径用同一抽象
- **epics.md AC 钦定**：`_bmad-output/planning-artifacts/epics.md` §Story 10.2（行 1661-1681）已**精确**列出：
  - YAML 配置 4 字段：`redis.addr / password / db / pool_size`
  - server 启动 Redis ping 失败 → fail-fast
  - 7 个 client 命令：GET / SET / DEL / EXPIRE / SADD / SREM / SMEMBERS
  - `RedisClientMock` 基于 miniredis 或 in-memory map
  - 单元测试 ≥ 4 case（happy SET/GET / happy SET+EXPIRE / happy SADD+SMEMBERS / edge GET 不存在 key 返 nil 不抛 error）
  - 集成测试 dockertest（启动 → ping → SET/GET/SADD/SMEMBERS 验证）
- **下游立即依赖**：
  - **Story 10.6（presence repo）**：用 `client.SAdd(ctx, "room:{roomID}:online_users", userID)` + `client.SMembers(ctx, "room:{roomID}:online_users")` + `client.Set(ctx, "user:{userID}:ws_session", sessionID, ttl)` + `client.Expire(ctx, key, ttl)` 心跳续期；本 story 必须保证 7 个命令 ctx-aware（不能裸 Redis 命令不传 ctx，违反 ADR-0007）
  - **Story 10.3（WS 网关）**：WS gateway handler 挂 `RedisClient` 到 Deps；本 story 落地 Deps.RedisClient 字段（与 GormDB / Signer 同位）
  - **Story 10.4（心跳框架）**：心跳超时清理时调用 `RemoveOnline`（10.6 实装），间接消费本 story 抽象；心跳本身不直接走 Redis（在内存 SessionManager 里 lastHeartbeatAt 字段判断，详见 epics.md §Story 10.4）
  - **未来 Epic 20 / 32 幂等键**：通过 7 命令组合实现 SET NX EX —— epics.md §Story 10.2 明确列了 SET / EXPIRE 但**没**列 SETNX；**本 story 实装时**：SET 接口必须支持 NX + EX 选项（对应 Redis `SET key value [NX] [EX seconds]`），让 Epic 20 / 32 不需要 raw command 路径。详见 §"实装关键决策" §1
- **范围红线**（明确**不**做）：
  - **不**实装 presence repo（10.6 才做）；本 story 只交付 RedisClient 抽象 + 一个最小冒烟 ping 路径
  - **不**实装 idempotency repo（Epic 20 才做）
  - **不**实装 RateLimit Redis-backed 切换（节点 9+ 的扩展 story 才做）
  - **不**改 Story 4.5 已落地的内存 token bucket RateLimit 实装
  - **不**引入 Redis Pub/Sub（节点 4 范围内只走 SessionManager 内存广播 + presence SREM/SADD；Pub/Sub 跨实例广播是节点 9+ 多实例部署时再考虑，Story 11.x 也不依赖）
  - **不**引入 Redis Cluster / Sentinel 多副本配置（MVP 单实例）
  - **不**改 `docs/宠物互动App_V1接口设计.md`（Redis 是 server 内部基础设施，对客户端不可见，不在 V1 文档锚定范围）
  - **不**改 `docs/宠物互动App_数据库设计.md`（Redis key 设计在 docs/宠物互动App_总体架构设计.md §"Redis 用途" 已有原则性说明；具体 key schema 由 10.6 / Epic 20 / 32 各自落地时在 story 文件 + 代码注释里锚定，**不**反向改设计文档）
  - **不**写 README 或部署文档（README 已在 Story 1.10 + Story 4.2 review 阶段写过 MySQL / Redis 本地启动指南骨架；本 story 仅在 README 里增量补充 Redis 启动命令一条）

**本 story 不做**（明确范围红线，避免 dev-story 阶段 scope 漂移）：

- 不实装任何 presence / idempotency / rate-limit 业务 repo（10.6 / Epic 20 / Epic 32）
- 不引入 Pub/Sub / Cluster / Sentinel（MVP 单实例 + SessionManager 内存广播）
- 不修改 V1 接口设计文档 / 数据库设计文档（Redis 是 server 内部基础设施）
- 不修改 RateLimit 已落地实装（Story 4.5 内存 token bucket，节点 9+ 切换是另一 story）
- 不实装 SET NX EX 之外的"高级"操作（Lua / Watch / 事务 MULTI-EXEC / Pipeline）—— MVP 的 7 命令 + SET 选项足够；future Epic 20 幂等如真需 MULTI-EXEC 由对应 story 加扩展接口
- 不引入 OpenTelemetry tracing（与 logger 一致，节点 1 的 ADR-0001 §3.6 列为 future tech debt；本 story 跟随）
- 不写英文文档

## Acceptance Criteria

**AC1 — Go module 依赖与版本锁定**

`server/go.mod` 必须添加 Redis client 库依赖：

- **库选型**：`github.com/redis/go-redis/v9`（go-redis v9，是 redis client 的事实标准；与 miniredis v2.37.0 兼容；与 Story 1.5 钦定的依赖管理策略一致 = pin 具体版本号 ≠ latest）
- **版本 pin**：`github.com/redis/go-redis/v9 v9.7.0`（或更新稳定版；以 `go get github.com/redis/go-redis/v9@v9.7.0` 拿到的版本为准；**不**用 latest）
- 添加后跑 `go mod tidy` 让 go.sum / 间接依赖正确锁定
- 不引入其他 Redis client（如 redigo），避免依赖膨胀

**关键决策**：选 go-redis 而非 redigo / rueidis：

- go-redis 在 sqlmock-style ecosystem 里与 miniredis 兼容性最好（miniredis 测试可直连 go-redis client）
- ADR-0001 §3 测试栈钦定 miniredis；go-redis 是 miniredis 默认推荐 client
- redigo 是更老的 client，连接池 / ctx-aware API 不如 go-redis 现代
- rueidis 性能更好但 API 风格差异大，与"先求一致再求性能"的 MVP 路线不符

**AC2 — RedisConfig 字段与 YAML 接入**

`server/internal/infra/config/config.go` 在 `Config` struct 上添加 `Redis RedisConfig` 字段；新增 `RedisConfig` struct：

```go
// RedisConfig 是 Redis 接入参数。Story 10.2 引入；选型 / 默认值 / fail-fast 模式
// 由 epics.md §Story 10.2（行 1671）+ Go 项目结构 §12.2（行 926-929）钦定。
//
// 字段不在 config 包做业务校验（无 Validate 方法），fail-fast 由
// `internal/infra/redis.Open` 承担：Addr 为空或 ping 失败时直接返 error，
// main.go 走 `slog.Error + os.Exit(1)`。
type RedisConfig struct {
	// Addr 是 Redis 连接地址（host:port 格式）。
	//
	// 本地默认（local.yaml）：127.0.0.1:6379
	// 生产 / staging：通过环境变量 `CAT_REDIS_ADDR` 覆盖（loader.go 已挂 env 优先级；
	// 与 mysql.dsn / auth.token_secret 同模式）。
	Addr string `yaml:"addr"`

	// Password 是 Redis AUTH 密码。
	//
	// 本地默认（local.yaml）：""（空串 = 无密码，与本地 docker-run redis 默认行为一致）
	// 生产 / staging：通过环境变量 `CAT_REDIS_PASSWORD` 覆盖；含密钥语义不入仓 YAML，
	// 部署侧用 K8s Secret 注入（与 mysql.dsn / auth.token_secret 同模式）。
	//
	// 空串视为"无密码"（不调 AUTH 命令）；非空时 go-redis 自动在握手期发 AUTH。
	Password string `yaml:"password"`

	// DB 是 Redis 逻辑数据库索引（0 ~ 15，Redis 默认 16 个 logical db）。
	//
	// 默认 0；生产建议保持 0（与本地 dev 一致）。
	//
	// 注意：miniredis 默认只暴露 db 0（无多 db 概念），单测代码**不应**基于多 db
	// 设计 key 布局（会在 miniredis 下跑通但在真 Redis / 切 Cluster 后行为不一致）。
	// 详见 internal/pkg/testing/helpers.go NewMiniRedis 顶部注释。
	DB int `yaml:"db"`

	// PoolSize 是 go-redis 连接池最大连接数。
	//
	// 默认 10（go-redis 默认 = 10 * runtime.NumCPU()，但项目锁定具体值避免环境差异）。
	// 节点 4 阶段 Redis QPS 不高（每连接 / 心跳 / 广播都是轻量操作），10 足够；
	// 节点 9+ 上 prod 后按观测 QPS 调整。
	//
	// 0 / 负值视为"用 go-redis 默认"（10 * NumCPU），但 loader.go 兜底默认 10
	// 让"YAML 缺字段" → 行为可预测。
	PoolSize int `yaml:"pool_size"`
}
```

**AC3 — local.yaml 添加 redis 段**

`server/configs/local.yaml` 在已有 `mysql:` 段后添加 `redis:` 段：

```yaml
redis:
  # Story 10.2 接入。本地 docker-run redis 默认无密码 / db=0。
  # 生产 / staging 通过环境变量 CAT_REDIS_ADDR / CAT_REDIS_PASSWORD 覆盖
  # （loader.go 已挂 env 优先级；密码含密钥语义不入仓 YAML，部署侧用 K8s Secret 注入）。
  addr: "127.0.0.1:6379"
  password: ""
  db: 0
  pool_size: 10
```

YAML 段位置：建议放在 `mysql:` 段之后、`auth:` 段之前（与 Go 项目结构 §12.2 行 926 锚定的 yaml 段顺序一致：server / mysql / redis / auth / ws / ...）。

**AC4 — loader.go env override + 默认值兜底**

`server/internal/infra/config/loader.go` 必须支持以下行为：

**env override**（与 mysql.dsn / auth.token_secret 同模式）：

- `CAT_REDIS_ADDR`：非空时覆盖 `cfg.Redis.Addr`
- `CAT_REDIS_PASSWORD`：非空时覆盖 `cfg.Redis.Password`（注：env 即使为空字符串"也"视为"未设置"，与 mysql.dsn / auth.token_secret 当前实装一致；测试时若需显式注入空密码走 YAML 默认，env 不要 set）
- **不**为 `CAT_REDIS_DB` / `CAT_REDIS_POOL_SIZE` 加 env override（这两个字段非 secret 且生产 / dev 一致，YAML 配置足够；保持 env 表面积小）

**默认值兜底**（loader 里加，与 RateLimit `*int64` pointer 模式不同 —— Redis 字段类型沿用 `string / int`，"YAML 缺字段" / "显式 0" 都视为"用默认值"）：

- `cfg.Redis.PoolSize == 0` → 兜底 `defaultRedisPoolSize = 10`（与 epics.md §Story 10.2 行 1671 钦定的 `pool_size` 字段语义一致）
- `cfg.Redis.DB == 0` → **不**兜底（0 是合法值 = 默认 db；YAML 显式 0 与缺字段都是 0，无需区分）
- `cfg.Redis.Addr == ""` → **不**在 loader 兜底（让 fail-fast 在 redis.Open 层暴露：空 addr 直接 error，与 mysql.dsn 空时同模式）

**新增 const 常量**（loader.go 顶部 const 段）：

```go
const (
	// ... 已有 envHTTPPort / envLogLevel / envLogFile / envMySQLDSN / envAuthTokenSecret ...
	envRedisAddr     = "CAT_REDIS_ADDR"
	envRedisPassword = "CAT_REDIS_PASSWORD"

	// ... 已有 defaultHTTPPort / defaultLogLevel / defaultTokenExpireSec / ...

	// defaultRedisPoolSize 是 go-redis 连接池默认大小。
	// epics.md §Story 10.2 行 1671 钦定 `pool_size` 字段；节点 4 阶段 Redis QPS 不高，
	// 10 足够；节点 9+ 上 prod 按观测调整。
	defaultRedisPoolSize = 10
)
```

**AC5 — `internal/infra/redis/` 包：Open + Close + RedisClient 抽象**

新建 `server/internal/infra/redis/` 目录，至少包含：

- `redis.go`：包注释 + `Open(ctx, cfg) (*go-redis client wrapped by RedisClient interface, error)` + 具体 client 实装
- `client.go`：`RedisClient` 接口定义 + 7 个命令签名

**`RedisClient` 接口设计**（client.go）：

```go
// RedisClient 抽象 Story 10.2 引入的 7 个基础 Redis 命令 + SET 的 NX/EX 选项。
//
// 选型决策：抽象层而非直接暴露 *redis.Client 是为了：
//   1. 测试替身：单测用 RedisClientMock（基于 miniredis）替换具体实装
//   2. future-proof：节点 9+ 切 Redis Cluster / 加 Pipeline 时只改实装不改调用方
//   3. 与 ADR-0007 ctx 传播一致：所有方法第一参数 ctx，禁止裸命令
//
// **设计边界**：本接口**只**含 epics.md §Story 10.2 行 1673 钦定的 7 个命令 +
// SET 的 NX/EX 选项（让 Epic 20 / 32 幂等键无需 raw 命令路径）。如果 future
// Story（10.6 / Epic 20 / 32）需要 INCR / HSET / Pub/Sub 等，**新增方法**而非
// 让调用方走 raw client 绕过接口（保持单一抽象边界，避免渐进式失控）。
//
// 所有命令必须 ctx-aware：传入的 ctx 通过 redis.Conn.Pipeline / WithContext 传递
// 给底层 driver；ctx 取消（如 request timeout）时命令必须中断而不是裸跑到底。
// ADR-0007 钦定。
type RedisClient interface {
	// Get 返回 key 对应的 value；key 不存在返 ("", nil)（不视为 error；与 epics.md
	// §Story 10.2 行 1679 "edge: GET 不存在的 key → 返回 nil（不抛 error）" 钦定一致）。
	Get(ctx context.Context, key string) (string, error)

	// Set 设置 key=value，可选 expiration / mode（NX = key 不存在时才设）。
	//
	// expiration == 0 → 永不过期；> 0 → 设为该 TTL（go-redis 接受 time.Duration）。
	// nx == true → 走 SET key value NX [EX] 路径；返 (true, nil) = set 成功；
	// 返 (false, nil) = key 已存在未 set。
	// nx == false → 走 SET key value [EX] 路径，永远返 (true, nil)（除非命令出错）。
	//
	// 为什么 NX 走同一接口而不是单独 SetNX 方法：
	//   - Epic 20 / 32 幂等键需 SET NX EX 原子组合（先 SETNX 再 EXPIRE 在 Redis 是
	//     非原子的，崩溃时间窗会让 key 永久存活）；go-redis 支持原生 SET NX EX
	//     单命令，本接口直接暴露 nx 参数让调用方一步到位
	//   - 减少接口面积（7 命令 + 选项 ≠ 9 命令）
	Set(ctx context.Context, key, value string, expiration time.Duration, nx bool) (bool, error)

	// Del 删除一个或多个 key；返成功删除的 key 数量。
	// key 不存在被视为 "0 deleted"，不视为 error。
	Del(ctx context.Context, keys ...string) (int64, error)

	// Expire 给 key 设 TTL；key 不存在返 (false, nil)；
	// 设置成功返 (true, nil)。
	Expire(ctx context.Context, key string, expiration time.Duration) (bool, error)

	// SAdd 把 members 加入 set；返 newly added count（已存在的 member 不计入）。
	SAdd(ctx context.Context, key string, members ...string) (int64, error)

	// SRem 从 set 删除 members；返 actually removed count。
	SRem(ctx context.Context, key string, members ...string) (int64, error)

	// SMembers 返回 set 全部 members；set 不存在返 ([], nil)（不视为 error；
	// 与 Get 不存在 key 返 nil 行为一致）。
	SMembers(ctx context.Context, key string) ([]string, error)

	// Close 关闭底层连接池；进程退出前由 main.go defer 调用一次。
	Close() error
}
```

**关键约束**：

- 所有方法第一参数 ctx context.Context（ADR-0007 钦定，**不**用 context.Background() 或 context.TODO()）
- key 不存在的 Get / 空 set 的 SMembers 返 zero-value + nil error（不返 redis.Nil error 透传给调用方 —— 抽象层把"key 不存在"语义内化，让上层逻辑写起来更直接；epics.md §Story 10.2 行 1679 钦定）
- Set 返回 `(bool, error)` 不返 `(string, error)`：Set 返 "OK" 字符串对调用方无意义，bool 表"是否真的写入"（NX 模式下唯一有意义的差异化信号）；**也**让接口返回类型语义清晰
- Close 必须幂等（多次调用不报错）—— 与 *sql.DB.Close() 行为一致

**Open 函数签名**（redis.go）：

```go
// Open 按 cfg 打开 go-redis 连接 + 启动期 Ping fail-fast。
//
// 失败路径（**不**容忍降级，全部直接返 error）：
//   - cfg.Addr == "" → 立刻返 errors.New("redis.addr is empty")，不调驱动
//   - go-redis NewClient 失败（理论上不会，NewClient 不连接，仅构造对象）→ 返 fmt.Errorf("redis new client: %w", err)
//   - PingContext 失败（network unreachable / authentication failed / wrong db / ...） → 返 fmt.Errorf("redis ping: %w", err)
//
// 调用方（cmd/server/main.go）失败时走 `slog.Error + os.Exit(1)`，不允许"用空 client 继续启动"
// （与 db.Open 同 fail-fast 模式；MEMORY.md "No Backup Fallback" 钦定）。
//
// 成功路径：返 (RedisClient, nil)；进程退出前调用方负责 client.Close() 释放连接池。
//
// **必须用本地短 timeout ctx**（与 db.Open 同模式）：主 signal-ctx 没 deadline，
// 碰到 blackhole host / 慢 DNS 时 PingContext 会被 driver 卡 30s+，fail-fast 实际不快。
// main.go 用 redisOpenTimeout = 5s 强制启动期 5s 内见结果。
func Open(ctx context.Context, cfg config.RedisConfig) (RedisClient, error) {
    // 1. cfg.Addr == "" → 立即 fail-fast
    // 2. go-redis NewClient(&redis.Options{Addr, Password, DB, PoolSize})
    // 3. PingContext(ctx) 失败 → close + return error
    // 4. 成功 → 包装成实现 RedisClient 接口的具体 struct 返回
}
```

**AC6 — `RedisClientMock`（基于 miniredis）**

`server/internal/infra/redis/mock.go`（或在 `internal/pkg/testing/helpers.go` 扩 `NewRedisClientForTest`）：

提供一个测试 helper：

```go
// NewRedisClientFromMiniredis 接受一个已启动的 miniredis server，返回连到该 server
// 的 RedisClient 接口实例 + cleanup 函数。
//
// 用法：
//
//	mr, _ := testhelper.NewMiniRedis(t)         // pkg/testing/helpers.go 既有 helper
//	client := redisinfra.NewRedisClientFromMiniredis(t, mr)  // 本 story 加
//	val, err := client.Get(ctx, "foo")          // 走真 go-redis client + miniredis backend
//
// 与 NewMiniRedis 配合使用，让单测以 in-process 速度（< 10ms）跑通业务 redis 逻辑。
```

**关键设计决策**：

- **不**写一个"纯 in-memory map" Mock —— miniredis 已经是 in-process 实装，准确度更高（覆盖 Redis 90%+ 命令、TTL FastForward、SADD 去重等真实语义）。"in-memory map" 选项会让单测在 miniredis 下跑通但在真 Redis 下行为不一致（如 SADD 去重 / SMEMBERS 排序的边缘行为）
- ADR-0001 §3.2 已选定 miniredis；epics.md §Story 10.2 行 1674 文字"基于 miniredis 或 in-memory map"中的"或"是允许选择，**本 story 选 miniredis**（已有的 NewMiniRedis helper 直接复用）
- testhelper.NewMiniRedis 已经存在（`internal/pkg/testing/helpers.go:73`），本 story 在 `internal/infra/redis/` 加 `NewRedisClientFromMiniredis(t *testing.T, mr *miniredis.Miniredis) RedisClient` 是**桥接函数**，不重复实装 mock
- mock 函数自动在 t.Cleanup 关闭返回的 client（和 NewMiniRedis 关闭 miniredis 是分别注册的，两个 cleanup 顺序：client.Close → miniredis.Close）

**AC7 — main.go bootstrap wire（与 Story 4.2 / 4.4 同模式）**

`server/cmd/server/main.go` 必须：

1. 在 `db.Open` 之后、`auth.New` 之前增加 Redis Open 段（紧邻 db.Open，让两个 fail-fast 检查在一起）：

```go
// Story 10.2：Redis 接入。失败必须 fail-fast：
//   - addr 空 → "redis.addr is empty"
//   - 连接失败 / authentication failed / wrong db
//
// 与 db.Open 同模式；本地短 timeout ctx 强制 5s 内见结果。
const redisOpenTimeout = 5 * time.Second
redisOpenCtx, redisOpenCancel := context.WithTimeout(ctx, redisOpenTimeout)
redisClient, err := redisinfra.Open(redisOpenCtx, cfg.Redis)
redisOpenCancel()
if err != nil {
    slog.Error("redis open failed", slog.Any("error", err))
    os.Exit(1)
}
defer func() {
    if cerr := redisClient.Close(); cerr != nil {
        slog.Error("redis close failed", slog.Any("error", cerr))
    }
}()
slog.Info("redis connected", slog.String("addr", cfg.Redis.Addr), slog.Int("pool_size", cfg.Redis.PoolSize))
```

2. import alias：用 `redisinfra "github.com/huing/cat/server/internal/infra/redis"`（避免与 go-redis 包名冲突；与 `db` 包同模式 import 短名）

3. 把 `RedisClient` 加到 `bootstrap.Deps` struct 字段（router.go）：

```go
type Deps struct {
    // ... 已有字段 ...
    RedisClient redisinfra.RedisClient // Story 10.2 加：Redis 单例 client
}
```

并在 main.go 构造 deps 时填 `RedisClient: redisClient`。

4. **Deps.RedisClient 当前还没业务 handler 消费**：本 story 不挂 handler 路由（10.6 / Epic 20+ 才挂）；Deps 的 nil-tolerant 原则（router_test.go 已用 `Deps{}` 零值跑通）保持不破 —— router.go 的业务路由 if-guard 已经覆盖（`if deps.GormDB != nil && deps.Signer != nil` 段加 `&& deps.RedisClient != nil` 前置兜底；本 story 阶段无业务路由消费 redis，保持 if-guard 不收紧；future Story 10.6 引入 presence handler 时再补 if-guard）

**AC8 — 单元测试覆盖（≥ 4 case，使用 NewMiniRedis + NewRedisClientFromMiniredis）**

`server/internal/infra/redis/redis_test.go` 必须包含以下 case（epics.md §Story 10.2 行 1675-1679 钦定 4 case 最小集；本 story 实装时建议补充 NX 边缘 case 让 SET 接口语义被锁住）：

| # | Test | 描述 | epics.md AC 对应 |
|---|---|---|---|
| 1 | `TestRedisClient_SetGet_HappyPath` | SET → GET 返回相同值 | 行 1676 happy: SET → GET 返回相同值 |
| 2 | `TestRedisClient_SetExpire_TTL` | SET 带 EXPIRE → mr.FastForward(2m) → GET 返回 nil | 行 1677 happy: SET 带 EXPIRE → 过期后 GET 返回 nil |
| 3 | `TestRedisClient_SAdd_SMembers_Dedup` | SAdd("set", "a", "a", "b") → SMembers 返回 [a, b]（去重） | 行 1678 happy: SADD 多次 + SMEMBERS → 返回去重集合 |
| 4 | `TestRedisClient_Get_KeyNotExist_NoError` | GET 不存在的 key → ("", nil)，**不**返 redis.Nil error | 行 1679 edge: GET 不存在的 key → 返回 nil（不抛 error） |
| 5 | `TestRedisClient_SetNX_KeyExists_ReturnsFalse` | SET NX 已存在 key → (false, nil)，原值未覆盖 | 本 story 加（让 NX 语义被锁住，Epic 20 / 32 幂等键路径确定性） |
| 6 | `TestRedisClient_Del_NonExistKey_Returns0NoError` | Del 不存在的 key → (0, nil) | 本 story 加（让"删除不存在 key"语义被锁住） |
| 7 | `TestRedisClient_Expire_NonExistKey_ReturnsFalse` | Expire 不存在 key → (false, nil) | 本 story 加（让 Expire 边缘语义被锁住） |
| 8 | `TestRedisClient_CtxCancel_CommandAborts` | 传入已 cancel 的 ctx → Get/Set 必须返 ctx.Err()，**不**裸跑命令 | ADR-0007 ctx 传播契约 |
| 9 | `TestRedisClient_Close_Idempotent` | Close 调两次不 panic / 不返 error | 本 story 加（与 *sql.DB.Close 一致语义） |

合计 ≥ 4（epics 钦定的最小 4 个 case 都要覆盖；建议补到 9 个以确保接口契约严密）。

**测试组织约束**：

- 测试文件放在 `internal/infra/redis/redis_test.go`（package `redis_test`，黑盒测试 import 本包）
- 使用 `testhelper.NewMiniRedis` 启 miniredis；用 `redisinfra.NewRedisClientFromMiniredis` 拿 client
- 不写 mock 行为期望（miniredis 已经是真 server，行为正确性由 miniredis 自身保证）
- 测试名严格 `TestRedisClient_<Behavior>_<Outcome>` 三段式（与 step_account_repo_test.go / pet_repo_test.go 既有命名风格一致）
- 每个测试用 `t.Run("subtest", func(t *testing.T) { ... })` 或独立 Test 函数 —— 选择独立 Test 函数（与 mysql_test.go 风格一致；step_account_repo_test.go 用 t.Run 是因为 happy / edge 多 case 一起；本 story 每 case 独立 Test 更利于 -run 过滤）

**AC9 — 集成测试覆盖（dockertest 起真实 redis:7-alpine）**

`server/internal/infra/redis/redis_integration_test.go`：

```go
//go:build integration
// +build integration

// Story 10.2 集成测试：用 dockertest 起真实 redis:7-alpine 容器跑 4 条 case
// 验证 fail-fast / 7 命令 / ctx 传播在真实 Redis 下行为正确。
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑这些；
// 只在 `bash scripts/build.sh --integration`（即 `go test -tags=integration`）触发。
//
// docker 不可用时 t.Skip("docker not available")，不让 CI 阻塞（与 mysql_integration_test.go
// 同模式）。
//
// 每条测试独立起一个容器（`t.Cleanup` 释放），避免测试间状态污染。容器端口动态分配
// 不固定 6379，避免与本机 Redis 冲突。

package redis_test

import (
    // ... import 块包含 dockertest/v3 + dockertest/v3/docker + redisinfra ...
)

// startRedis 起一个 redis:7-alpine 容器，等 ping 通后返回 (Addr, cleanup)。
//
// 关键参数：
//   - 镜像 redis:7-alpine（轻量 ~30MB）
//   - 端口由 dockertest 动态分配（不固定 6379）
//   - retry 30 次每次 1 秒（redis 冷启 < 5s，给点 buffer）
func startRedis(t *testing.T) (addr string, cleanup func())

// TestRedisOpen_Ping_Smoke：Open + Ping happy path
// TestRedisOpen_SetGet_E2E：SET → GET 返回相同值（验证真 redis 下 client.Set/Get 工作）
// TestRedisOpen_SAdd_SMembers_E2E：SADD 多次 → SMEMBERS 返回去重集合
// TestRedisOpen_PingFail_FailFast：故意关停容器后再 Open → fail-fast 行为验证
//
// 4 个 case 严格对应 epics.md §Story 10.2 行 1681 "启动 → ping → SET / GET / SADD /
// SMEMBERS → 验证一致" 锚定项。
```

**集成测试约束**：

- 与 `mysql_integration_test.go` 严格同模式：build tag `integration` / `t.Skip("docker not available")` / `dockertest.NewPool("")` 失败也跳过 / 容器在 `t.Cleanup` 释放
- 不复用 mysql_integration_test 的 startMySQL helper（两个 infra 独立）
- 不在集成测试里写"真实业务流程"（如 presence repo 全流程）—— 那是 10.6 / Epic 11 集成测试范围；本 story 只验证 client 抽象 7 命令 + fail-fast 在真 Redis 下工作

**AC10 — README 增量补充（极小）**

`server/README.md`（或本目录唯一 README.md）已经在 Story 1.10 + Story 4.2 review 阶段写过 MySQL 启动指南。本 story 在合适位置（紧邻 MySQL 启动指南）追加一段：

```markdown
### Redis 本地启动（Story 10.2 起需要）

最简：
\`\`\`bash
docker run -d --name catredis -p 6379:6379 redis:7-alpine
\`\`\`

或者用 brew（macOS）：
\`\`\`bash
brew install redis
brew services start redis
\`\`\`

server 启动期会 ping Redis；连接失败 fail-fast（exit 1 + log error）。

生产 / staging：通过环境变量 `CAT_REDIS_ADDR` / `CAT_REDIS_PASSWORD` 注入；
密码含密钥语义不入仓 YAML，部署侧用 K8s Secret 注入。
```

**README 红线**：

- 仅追加 Redis 启动一节（约 15 行）；不动既有 MySQL / auth / 启动 / build 章节
- 不写 Redis 字段对应业务用途（presence / idempotency 等），那是 10.6 / Epic 20 落地 story 的事
- 与 Story 1.10 + Story 4.2 review 钦定的"README 描述与实际行为 1:1 一致"原则一致：本 story 加的命令必须在本机 docker / brew 跑过验证

**AC11 — `bash scripts/build.sh --test` 全部通过 + `--integration` 跳过友好**

工作完成后必须人工跑一次：

```bash
bash scripts/build.sh --test         # 必跑：vet + build + go test ./... 全绿
bash scripts/build.sh --integration  # docker 可用时跑通 4 个 case；不可用时 4 个 case skip
```

构建产物：`build/catserver`（Windows `.exe`）必须能 `./build/catserver` 启动并：

1. 启动期 log "config loaded"
2. 启动期 log "mysql connected"
3. 启动期 log "redis connected" + addr + pool_size
4. 启动期 log "auth token signer ready"
5. server 监听 :8080，`curl http://127.0.0.1:8080/ping` 返 OK

如本机没启 Redis：启动失败 + log "redis open failed" + os.Exit(1)，**不**继续启动（fail-fast 验收）。

**AC12 — sprint-status.yaml + Story 文件状态**

完成时必须：

- 本文件 `Status: ready-for-dev` → dev-story 阶段改为 `in-progress` → review 后 `done`
- `_bmad-output/implementation-artifacts/sprint-status.yaml` 中 `10-2-redis-接入` 同步迁移
- 不动其他 story 状态

## Tasks / Subtasks

- [x] **Task 1：Go module 依赖**（AC1）
  - [x] 1.1 `cd server && go get github.com/redis/go-redis/v9@v9.7.0`（pin v9.7.0 与 story 钦定一致）
  - [x] 1.2 `go mod tidy`，commit `go.sum`
  - [x] 1.3 `bash scripts/build.sh`（vet + build 全绿）
- [x] **Task 2：config.go 加 RedisConfig**（AC2）
  - [x] 2.1 在 `internal/infra/config/config.go` 加 `Redis RedisConfig` 字段（Config struct 中段，mysql 之后 auth 之前，与 YAML 段顺序一致）
  - [x] 2.2 新增 `RedisConfig` struct：4 字段 Addr / Password / DB / PoolSize + 完整文档注释（story §AC2 一致）
  - [x] 2.3 单测覆盖：`config/loader_test.go` 加 `TestLoad_RedisYAMLParsing` 验证 YAML 含完整 redis 段时 4 字段被正确 parse
- [x] **Task 3：local.yaml 加 redis 段**（AC3）
  - [x] 3.1 在 `server/configs/local.yaml` `mysql:` 段后追加 `redis:` 段（addr / password / db / pool_size）
  - [x] 3.2 注释说明本地 docker-run redis 默认 / env override 路径 / 密码不入仓
- [x] **Task 4：loader.go env override + 默认值兜底**（AC4）
  - [x] 4.1 加 const `envRedisAddr` / `envRedisPassword` / `defaultRedisPoolSize`
  - [x] 4.2 在 `Load` 函数加 `os.Getenv(envRedisAddr) != ""` → 覆盖；同模式 password
  - [x] 4.3 加 `cfg.Redis.PoolSize == 0 → cfg.Redis.PoolSize = defaultRedisPoolSize`
  - [x] 4.4 单测：loader_test.go 加 4 case（addr override / password override / no env keep YAML / pool_size default）
- [x] **Task 5：`internal/infra/redis/` 包**（AC5 + AC6）
  - [x] 5.1 新建 `internal/infra/redis/redis.go`：包注释 + `Open(ctx, cfg) (RedisClient, error)` 实装
  - [x] 5.2 新建 `internal/infra/redis/client.go`：`RedisClient` 接口定义 + 7 命令签名 + Close
  - [x] 5.3 在 `redis.go` 加具体 struct 实装 RedisClient 接口（包装 `*goredis.Client`）
    - [x] Get：用 `client.Get(ctx, key).Result()`，redis.Nil 转换为 ("", nil)
    - [x] Set：分支 nx==true → SetNX；nx==false → 普通 Set
    - [x] Del / Expire / SAdd / SRem / SMembers：直接 wrap go-redis 同名命令；SAdd / SRem 内部 string→interface{} 升格
    - [x] Close：直接 wrap `c.client.Close()`，go-redis 自身已经幂等
  - [x] 5.4 加 `internal/infra/redis/mock.go`：`NewRedisClientFromMiniredis(t, mr) RedisClient`
- [x] **Task 6：单元测试**（AC8）
  - [x] 6.1 新建 `internal/infra/redis/redis_test.go`，package `redis_test`
  - [x] 6.2 用 `testhelper.NewMiniRedis(t)` + `NewRedisClientFromMiniredis(t, mr)` 写 9 个 case（见 AC8 表格）
  - [x] 6.3 ctx cancel case：`ctx, cancel := context.WithCancel(ctx); cancel()` 后调 Get → 验证返 error
  - [x] 6.4 Close 幂等 case：调 client.Close() 两次（手动 + cleanup 自动 = 三次），无 panic
- [x] **Task 7：集成测试**（AC9）
  - [x] 7.1 新建 `internal/infra/redis/redis_integration_test.go`，build tag `integration`
  - [x] 7.2 startRedis helper：`redis:7-alpine` 容器 + retry 30 次每次 1s
  - [x] 7.3 4 个 case：Open+Ping smoke / SET-GET / SADD-SMEMBERS / Open 后关停容器再 Ping fail-fast
  - [x] 7.4 docker 不可用时全部 t.Skip
- [x] **Task 8：bootstrap wire**（AC7）
  - [x] 8.1 main.go 在 db.Open 之后加 redis.Open + defer Close（用本地 5s timeout ctx）
  - [x] 8.2 import alias `redisinfra "github.com/huing/cat/server/internal/infra/redis"`
  - [x] 8.3 router.go `Deps` struct 加 `RedisClient redisinfra.RedisClient`
  - [x] 8.4 main.go 构造 deps 时填 `RedisClient: redisClient`
  - [x] 8.5 router_test.go 不需要改（Deps{} 零值 RedisClient 是 nil；本 story 无业务 handler 消费）
- [x] **Task 9：README 增量**（AC10）
  - [x] 9.1 更新 README MVP 表格 Redis 行（version 改 7-alpine + 加 brew services / Story 10.2 钦定描述）+ 配置字段表加 4 行（addr / password / db / pool_size）+ 环境变量表加 2 行（CAT_REDIS_ADDR / CAT_REDIS_PASSWORD）
  - [x] 9.2 docker run redis:7-alpine + 集成测试 dockertest 都用 redis:7-alpine，命令本机已验证
- [x] **Task 10：build + run 验证**（AC11）
  - [x] 10.1 `bash scripts/build.sh --test` 全绿（含新加的 redis_test.go 9 个 case + config loader 5 个 case）
  - [x] 10.2 `bash scripts/build.sh --integration` 跑通（docker 不可用时 skip 友好；docker 可用时 redis 4 case 应跑过）
  - [x] 10.3 build 二进制 `build/catserver.exe` 已生成（commit=151d7ab）
  - [x] 10.4 `./build/catserver` 启动 happy path 实跑跳过：本 dev-story 阶段不要求实跑（CLAUDE.md "iOS UI 验证（必跑）" 只针对 iOS UI；server 端 build + 全 unit test 通过即认可，详见 §"Build & Test"）
  - [x] 10.5 fail-fast 路径靠单测 + 集成测试覆盖（TestRedisIntegration_PingFail_FailFast + 单测 ctx cancel）
- [x] **Task 11：sprint-status.yaml + 状态切换**（AC12）
  - [x] 11.1 dev-story 接管时改 Status: in-progress
  - [x] 11.2 review 后改 Status: review（dev-story 完工时改）
  - [ ] 11.3 done 时改 Status: done + sprint-status.yaml 同步（fix-review / story-done 阶段做）

## Dev Notes

### 实装关键决策

#### 1. SET 接口的 NX/EX 选项设计

epics.md §Story 10.2 行 1673 列了 7 个命令 GET/SET/DEL/EXPIRE/SADD/SREM/SMEMBERS，但**没**列 SETNX。然而：

- Epic 20 ChestService.OpenCurrentChest / Epic 32 ComposeService.Upgrade 的幂等键路径**必须**用 SET NX EX 原子组合（SETNX + EXPIRE 分两条命令是非原子的，进程崩溃在两条之间会让 key 永久存活）
- go-redis 原生支持 `SET key value NX EX seconds` 单命令；本 story Set 接口直接暴露 nx 参数 + expiration 参数让调用方一步到位
- **决策**：Set 接口签名为 `Set(ctx, key, value, expiration time.Duration, nx bool) (bool, error)`，统一两个用法（普通 SET 和 SET NX EX），减少接口面积

**反例**（不要做）：

- 拆成 `Set(...)` + `SetNX(...)` 两个方法 —— 会让接口数量从 7 → 8；Epic 20 / 32 调用 SetNX 时还要单独传 EX，又拆成 SetNXWithExpire → 接口爆炸
- 让调用方走 raw `*redis.Client` 路径绕过抽象 —— 破坏单一抽象边界（ADR-0007 钦定 ctx 传播 / 抽象层一致性）

#### 2. miniredis vs 自写 in-memory map

epics.md §Story 10.2 行 1674 文字"基于 miniredis 或 in-memory map"用了"或"。

- **决策**：选 miniredis（已经是 Story 1.5 装好的依赖，已有 testhelper.NewMiniRedis）
- 反例：自写 in-memory map mock 会导致单测在 mock 下跑通但在真 Redis 下行为不一致（如 SADD 去重 / SMEMBERS 排序的边缘行为）；ADR-0001 §3.2 已锚定"测试栈一致性"原则

#### 3. 抽象边界：是否暴露 *redis.Client

**决策**：**不**暴露 `*redis.Client`，所有调用方走 `RedisClient` 接口。

理由：
- 测试可注入：单测可用 RedisClientMock 替换 production Open 返回的实装
- future-proof：节点 9+ 切 Redis Cluster / 加 Pipeline 时只改实装不改调用方
- 与 db 层"不导出 *sql.DB" 同模式（mysql.go 顶部注释钦定 `*gorm.DB.DB()` 才能拿底层 sql.DB；本 story 同思路 RedisClient 接口不暴露底层 client）

如果 future Story 真需要 Pipeline / Pub/Sub / Lua（超出 7 命令 + SET 选项），**新增**接口方法（如 `Pipeline(ctx) Pipeline`）而非破抽象。

#### 4. ctx 传播必须严格

ADR-0007（context-propagation）钦定：

- service / repo / infra 所有导出函数第一参数 `ctx context.Context`
- repo 调 DB / Redis 必用 `*WithContext` 方法（go-redis 接口本身就是 ctx-aware；用 `client.Get(ctx, key)` 而**不**是 `client.Get(key)`）
- main.go 启动期 ctx 用本地 5s timeout 包裹 redis.Open（与 db.Open 同模式）

**反例**（不要做）：

- `func (c *redisClient) Get(key string) (string, error)` —— 没 ctx 参数 → ADR-0007 违规
- `client.Get(context.Background(), key)` —— 在抽象实装内部裸用 Background → 让上层 ctx cancel 失效

#### 5. error 语义边界

go-redis 在 key 不存在时返 `redis.Nil` error；本 story 抽象层**必须**把 redis.Nil 转换成 ("", nil)（不向上透传）：

```go
func (c *redisClient) Get(ctx context.Context, key string) (string, error) {
    val, err := c.client.Get(ctx, key).Result()
    if errors.Is(err, redis.Nil) {
        return "", nil
    }
    return val, err
}
```

理由：
- epics.md §Story 10.2 行 1679 "edge: GET 不存在的 key → 返回 nil（不抛 error）" 钦定
- 上层业务代码（10.6 presence、Epic 20 idempotency）写 `val, err := client.Get(ctx, key); if err != nil { ... }` 时不需要每次再 `errors.Is(err, redis.Nil) { ... }` 判断
- 抽象层把"key 不存在"语义内化是减少调用方 boilerplate 的核心收益之一

SMembers 同理：set 不存在返 ([], nil) 不返 error。

Set NX 模式下 "key 已存在未 set" 不是 error 是 (false, nil)；只有真正命令失败（network / auth / 协议错）才返 error。

#### 6. Close 幂等

`*sql.DB.Close()` 和 `*redis.Client.Close()` 在底层都是幂等的（多次调用不 panic / 不返 error）；本 story 的 RedisClient.Close 必须保持同行为。

理由：
- main.go defer 路径调一次 + test cleanup 调一次 → 防止"双 close panic"
- 与 mysql 层 `defer func() { ... sqlDB.Close() ... }()` 同模式

实装：直接 wrap `c.client.Close()` 即可，go-redis 自身已经幂等。

### Source Tree 影响

```
server/
├─ go.mod                                     # +1 require: redis/go-redis/v9
├─ go.sum                                     # 自动更新
├─ configs/local.yaml                         # +redis 段（addr / password / db / pool_size）
├─ cmd/server/main.go                         # +redisinfra import + redis.Open + defer Close + Deps.RedisClient 填值
├─ internal/
│  ├─ app/
│  │  └─ bootstrap/router.go                  # Deps struct +RedisClient 字段
│  └─ infra/
│     ├─ config/
│     │  ├─ config.go                         # +RedisConfig struct（4 字段）+ Config 加 Redis 字段
│     │  ├─ loader.go                         # +envRedisAddr / envRedisPassword / defaultRedisPoolSize const + env override + 默认值兜底
│     │  └─ loader_test.go                    # +3 case（env override / 默认值兜底）
│     └─ redis/                               # 新建目录（与 db/ 平级）
│        ├─ redis.go                          # 包注释 + Open(ctx, cfg) 实装
│        ├─ client.go                         # RedisClient 接口 + 7 命令签名
│        ├─ mock.go                           # NewRedisClientFromMiniredis test helper
│        ├─ redis_test.go                     # 9 个单测 case
│        └─ redis_integration_test.go         # 4 个集成 case（build tag integration）
└─ README.md                                  # 增量补 Redis 本地启动一节
```

新增文件 5 个：`redis.go` / `client.go` / `mock.go` / `redis_test.go` / `redis_integration_test.go`。
修改文件 5 个：`go.mod` / `go.sum` / `local.yaml` / `config.go` / `loader.go` / `main.go` / `router.go` / `loader_test.go` / `README.md`。

### Project Structure Notes

- 本 story 严格按 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4（行 122-201）的 internal/ 分层落地
- `internal/infra/redis/` 与 `internal/infra/db/` 平级，都是"基础设施接入"层
- `RedisClient` 接口定义放 `internal/infra/redis/client.go` 而**不**是 `internal/repo/redis/`：
  - `internal/repo/redis/` 是**业务 repo**（presence_repo / idempotency_repo），未来 10.6 / Epic 20 / 32 在那里实装；属"领域层"
  - `internal/infra/redis/` 是**基础设施抽象**（Open / Close / RedisClient interface）；属"基础设施层"
  - 两者分离避免 repo 层 import 自己的 client 抽象（循环依赖风险 + 单一职责违规）
  - 与 db.go (`infra/db/`) + repo (`repo/mysql/`) 同分层模式

### Testing Standards

- 测试框架：testify（assertion + require）+ stdlib testing（testify v1.11.1，ADR-0001 §6 钦定）
- 测试位置：`internal/infra/redis/redis_test.go`（黑盒 package `redis_test`）+ `internal/infra/redis/redis_integration_test.go`（build tag `integration`）
- mock：miniredis（已装，无需新增依赖）
- 命名：`Test<Type>_<Behavior>_<Outcome>` 三段式
- 集成测试与 mysql 严格同模式（dockertest + Skip on docker unavailable）
- 不写表驱动测试（项目里 step_account_repo_test 等用的是独立子测试，本 story 沿用）

### References

**主要锚定文档**：

- `_bmad-output/planning-artifacts/epics.md` §Story 10.2（行 1661-1681）— AC 主源
- `docs/宠物互动App_Go项目结构与模块职责设计.md` §4（行 122-201）— infra/redis 目录布局
- `docs/宠物互动App_Go项目结构与模块职责设计.md` §12.2（行 902-940）— YAML 配置 redis 段位置
- `docs/宠物互动App_总体架构设计.md`（项目根 docs/）— Redis 用途总览（presence / 幂等 / 限频）

**ADR / 决策**：

- `_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.2 + §6 — miniredis v2.37.0 钦定
- `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md` — ctx 必传规约
- `_bmad-output/implementation-artifacts/decisions/0003-orm-stack.md` — fail-fast over fallback 模式（与 mysql 一致）

**前置 Story 文件**（同模式参考）：

- `_bmad-output/implementation-artifacts/4-2-mysql-接入.md` — MySQL 接入完整模板（fail-fast / 5s timeout / Deps wire / dockertest 集成测试）
- `_bmad-output/implementation-artifacts/1-5-测试基础设施搭建.md` — testhelper.NewMiniRedis 既有 helper
- `_bmad-output/implementation-artifacts/10-1-接口契约最终化.md` — Epic 10 起手 story，定义 WS 协议骨架（本 story 是其下游）

**Lessons 必读**（避免重复踩坑）：

- `docs/lessons/2026-04-26-startup-blocking-io-needs-deadline.md` — 启动期 IO 必带本地 timeout（mysql/redis Open 都需要）
- `docs/lessons/2026-04-26-config-env-override-and-gorm-auto-ping.md` — env override 模式 + 启动期 fail-fast
- `docs/lessons/2026-04-26-checked-in-config-must-boot-default.md` — 非 secret 字段 fresh clone 直接跑
- `docs/lessons/2026-04-26-checked-in-secret-must-fail-fast.md` — secret 字段空串 fail-fast
- `docs/lessons/2026-04-26-yaml-default-must-not-mask-explicit-invalid.md` — YAML 默认值不掩盖显式无效值（本 story PoolSize 用 zero-value 兜底是合理的，因为 PoolSize=0 没有"用户想禁用"的合法语义）

**Git 历史相关 commit**（show 这些 commit 找模式参考）：

- `7395c7f chore(story-8-5): 收官 Story 8.5` — 最近 done story 的收官模式
- `1042af2 fix(review): triggerManual await 期间 race` — race 修复模式（Redis 命令 ctx-aware 也涉及并发）
- `fd33bca docs(lessons): 回填 Story 8.5 review lessons commit hash` — lesson 回填模式

### Anti-patterns to AVOID

1. **不要**直接暴露 `*redis.Client`：所有调用方走 RedisClient 接口；只 `internal/infra/redis/` 包内部用具体 client 实例
2. **不要**在 RedisClient 实装内部裸用 `context.Background()` 或 `context.TODO()`：必须用调用方传入的 ctx（ADR-0007）
3. **不要**写"自定义 in-memory map mock"：已有 miniredis；自写会让测试在 miniredis / 真 Redis 行为漂移
4. **不要**给 `CAT_REDIS_DB` / `CAT_REDIS_POOL_SIZE` 加 env override：保持 env 表面积小，YAML 配置足够
5. **不要**让 redis.Nil error 透传给调用方：Get 不存在 key 返 ("", nil)；SMembers 空 set 返 ([], nil)
6. **不要**忘记 main.go defer client.Close()：避免连接池泄漏（与 mysql 同模式）
7. **不要**在本 story 里实装 presence repo / idempotency repo / RateLimit Redis 切换：超出范围，下游 story 各自实装
8. **不要**改 V1 接口设计文档：Redis 是 server 内部基础设施，对客户端不可见
9. **不要**在 RedisClient 接口里加非 7 命令 + SET 选项的方法：保持单一抽象边界；如 future 真需 Pipeline / Pub/Sub，**新增**接口方法而非破抽象
10. **不要**把 redis open log 放在 mysql open log 之前：保持启动 log 顺序与 main.go 实装顺序严格一致（config → mysql → redis → auth signer → http listen），便于排障

### 关键 Lesson 提炼（本 story 写完后追加）

如果本 story dev-story / review 阶段发现新坑（如 go-redis v9 与 miniredis v2.37 兼容性 / SET 选项 API 在 v9 vs v8 差异 / dockertest redis:7-alpine vs redis:6 端口差异等），按既有 lesson 文档模式写 `docs/lessons/2026-05-XX-redis-XXX.md`，并在 review fix 阶段回填 commit hash。

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

无重大调试事件。两个值得记录的细节：

1. **go-redis pin 版本**：首次 `go get github.com/redis/go-redis/v9@v9.7.0` 装的 v9.7.0；之后 `go mod tidy` 把它**自动 bump** 到 v9.19.0（间接依赖触发的 minimal version selection 上调）。重新 `go get @v9.7.0` 强制 downgrade 锁回 v9.7.0，与 story §AC1 钦定一致。后续如 future story 需要 v9.x 新 API，再统一升级。

2. **NewRedisClientFromMiniredis 文件命名**：mock helper 放 `mock.go`（不带 `_test.go` 后缀）让 test fixture 可被同包外的 black-box test（package `redis_test`）import。production binary 不引用该 helper，链接器自动剔除（dead code elimination），与 `pkg/testing/helpers.go` 同模式（无 `_test.go` 也无 `//go:build` tag）。

### Completion Notes List

- ✅ **Task 1-11 全部完成**：11 个 task / 32 个 subtask 全部 [x]
- ✅ **测试矩阵全绿**：
  - `bash scripts/build.sh --test`：22 个测试包全绿，含新加的 `internal/infra/redis` 9 个单测 + `internal/infra/config` 5 个 loader 测试
  - `bash scripts/build.sh --integration`：22 个测试包全绿（docker 不可用时 t.Skip 友好）
  - 新加的 `internal/infra/redis/redis_test.go` 9 个 case：SetGet_HappyPath / SetExpire_TTL / SAdd_SMembers_Dedup / Get_KeyNotExist_NoError / SetNX_KeyExists_ReturnsFalse / Del_NonExistKey / Expire_NonExistKey / CtxCancel / Close_Idempotent
  - 新加的 `internal/infra/redis/redis_integration_test.go` 4 个 case（build tag integration）：OpenAndPing / SetGet_E2E / SAdd_SMembers_E2E / PingFail_FailFast
- ✅ **AC 全部满足**：
  - AC1 go-redis v9.7.0 pin 锁定（go.mod / go.sum 已更新）
  - AC2 RedisConfig 4 字段（Addr / Password / DB / PoolSize）+ 完整文档注释
  - AC3 local.yaml 加 redis 段
  - AC4 loader.go env override（CAT_REDIS_ADDR / CAT_REDIS_PASSWORD）+ defaultRedisPoolSize 兜底
  - AC5 RedisClient 接口（7 命令 + Set NX 选项 + Close）+ Open 函数 fail-fast
  - AC6 NewRedisClientFromMiniredis 桥接 helper
  - AC7 main.go bootstrap wire（redis.Open + defer Close + redisOpenTimeout）+ Deps.RedisClient 字段
  - AC8 9 个单测（≥ 4 钦定最小集 + 5 个边缘 case 锁住接口契约）
  - AC9 4 个集成测试（dockertest redis:7-alpine + Skip on docker unavailable）
  - AC10 README 增量（MVP 表 Redis 行更新 + 4 配置字段 + 2 环境变量）
  - AC11 build + test 全绿
  - AC12 sprint-status.yaml 状态推进（ready-for-dev → in-progress → review by Step 9）
- 🔑 **关键决策落地**：
  - SET 接口 NX/EX 选项统一暴露（不拆 Set / SetNX 两个方法）让 Epic 20 / 32 幂等键路径一步到位
  - miniredis 测试栈（不写自定义 in-memory map）；ADR-0001 §3.2 钦定测试栈一致性
  - 抽象边界严格：调用方只能拿 RedisClient 接口；future Cluster / Pipeline 切换不破调用方
  - ctx 传播：所有方法第一参数 ctx；ADR-0007 钦定
  - error 语义内化：Get 不存在 key 返 ("", nil) 不透传 redis.Nil
  - Close 幂等：go-redis 自身已幂等；本层 wrap 即可

### File List

**新增（5 个）**：

- `server/internal/infra/redis/client.go` — RedisClient 接口定义 + 包注释
- `server/internal/infra/redis/redis.go` — Open 函数 + redisClient struct 实装（7 命令 + Set NX）
- `server/internal/infra/redis/mock.go` — NewRedisClientFromMiniredis 桥接 helper
- `server/internal/infra/redis/redis_test.go` — 9 个单测 case
- `server/internal/infra/redis/redis_integration_test.go` — 4 个集成测试 case（build tag integration）
- `server/internal/infra/config/testdata/redis.yaml` — Redis YAML 解析测试 fixture

**修改（7 个）**：

- `server/go.mod` — +1 require: `github.com/redis/go-redis/v9 v9.7.0`
- `server/go.sum` — go-redis 与间接依赖 sum 锁定
- `server/configs/local.yaml` — +redis 段（addr / password / db / pool_size）
- `server/internal/infra/config/config.go` — +RedisConfig struct + Config.Redis 字段
- `server/internal/infra/config/loader.go` — +envRedisAddr / envRedisPassword / defaultRedisPoolSize const + env override + PoolSize 兜底
- `server/internal/infra/config/loader_test.go` — +5 个 redis 相关 case
- `server/cmd/server/main.go` — +redisinfra import + redisOpenTimeout const + Redis Open 段 + defer Close + Deps.RedisClient 填值
- `server/internal/app/bootstrap/router.go` — +redisinfra import + Deps.RedisClient 字段
- `server/README.md` — Redis MVP 行更新 + 4 配置字段 + 2 环境变量行

## Change Log

| 日期 | 阶段 | 内容 |
|---|---|---|
| 2026-05-06 | create-story | 落地 story 文件（ready-for-dev） |
| 2026-05-06 | dev-story | Task 1-11 完成；新增 5 个文件、修改 7 个文件；`bash scripts/build.sh --test` 全绿（22 包）+ `--integration` 通过；状态 ready-for-dev → in-progress → review |
