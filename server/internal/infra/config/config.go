package config

type Config struct {
	Server    ServerConfig    `yaml:"server"`
	MySQL     MySQLConfig     `yaml:"mysql"`
	Auth      AuthConfig      `yaml:"auth"`
	RateLimit RateLimitConfig `yaml:"ratelimit"`
	Log       LogConfig       `yaml:"log"`
}

type ServerConfig struct {
	// BindHost 是 HTTP listener 绑定的 host 部分。空串（默认）= 绑所有接口
	// （0.0.0.0），与生产部署一致、Windows 环境会触发防火墙弹窗。测试场景
	// 注入 "127.0.0.1" → loopback-only，Windows Defender Firewall 对 loopback
	// 免检，避免每次 go test 新 hash 的 bootstrap.test.exe 反复弹授权窗。
	//
	// YAML key 可省略；仅在需要绑特定网卡 / localhost-only 场景时填。
	BindHost        string `yaml:"bind_host"`
	HTTPPort        int    `yaml:"http_port"`
	ReadTimeoutSec  int    `yaml:"read_timeout_sec"`
	WriteTimeoutSec int    `yaml:"write_timeout_sec"`
}

// MySQLConfig 是 MySQL 接入参数。Story 4.2 引入；选型 / DSN 策略 / 连接池参数
// 由 ADR-0003 (`_bmad-output/implementation-artifacts/decisions/0003-orm-stack.md`) 钦定。
//
// 字段不在 config 包做业务校验（无 Validate / Normalize 方法），fail-fast
// 由 `internal/infra/db.Open` 承担：DSN 为空或 ping 失败时直接返 error，
// main.go 走 `slog.Error + os.Exit(1)`。
type MySQLConfig struct {
	// DSN 是完整 MySQL 连接串（go-sql-driver/mysql 格式）。
	//
	// 本地默认（local.yaml）：
	//   cat:catdev@tcp(127.0.0.1:3306)/cat?charset=utf8mb4&parseTime=true&loc=Local
	//
	// 生产 / staging：通过环境变量 `CAT_MYSQL_DSN` 覆盖（loader.go 已挂；
	// CLAUDE.md "配置格式：YAML，支持环境变量覆盖" 钦定）。DSN 含密码 → 不入
	// 仓库的 YAML，部署侧用 K8s Secret 注入。
	//
	// 关键 query 参数：
	//   - charset=utf8mb4   必须；emoji / 用户名昵称 / 表情商品 4 字节字符
	//   - parseTime=true    必须；让 driver 把 DATETIME(3) 自动 parse 成 time.Time
	//                       （docs/宠物互动App_数据库设计.md §3.2）
	//   - loc=Local         本地时区。生产建议改 UTC（数据库侧也设 UTC）
	DSN string `yaml:"dsn"`

	// MaxOpenConns 是 *sql.DB 池的最大打开连接数。推荐 50（ADR-0003 §3.4）。
	// 0 = 无限制（不推荐生产用，会耗尽 MySQL `max_connections`）。
	MaxOpenConns int `yaml:"max_open_conns"`

	// MaxIdleConns 是空闲连接保活数。推荐 10。
	// 长连接保活避免每次请求 reconnect 增加延迟。
	MaxIdleConns int `yaml:"max_idle_conns"`

	// ConnMaxLifetimeSec 是连接最大存活时间（秒）。推荐 1800（30 min）。
	//
	// MySQL server 端 wait_timeout 默认 28800s（8h），但中间件 / LB 可能
	// 更短切 idle 连接 → *sql.DB 池里 idle 连接被切后下次复用会报
	// "invalid connection"。30 分钟主动 refresh 规避此类问题。
	//
	// 0 = 不限制（连接永远不主动 refresh，不推荐生产用）。
	ConnMaxLifetimeSec int `yaml:"conn_max_lifetime_sec"`
}

// AuthConfig 是 JWT 签发 / 校验配置。Story 4.4 引入；选型 / 默认值 / fail-fast
// 由 epics.md §Story 4.4（行 999-1020）+ V1接口设计 §4.1 行 180 钦定。
//
// 字段不在 config 包做业务校验（无 Validate 方法），fail-fast 由
// `internal/pkg/auth.New` 承担：TokenSecret 为空 / < 16 字节 / TokenExpireSec ≤ 0
// 直接返 error，main.go 走 `slog.Error + os.Exit(1)`。
type AuthConfig struct {
	// TokenSecret 是 HS256 签名 secret。**生产必须用 env 注入**（CAT_AUTH_TOKEN_SECRET）；
	// 仓库 YAML 默认留空让启动 fail-fast，避免空 secret 让 token 可任意伪造。
	//
	// 长度要求：≥ 16 字节（128 bit；HMAC-SHA256 推荐至少与 hash output size 等长，
	// 16 字节 secret + HMAC 的 amplification 已可抗暴力破解）。
	//
	// 生产注入：K8s Secret / Vault → CAT_AUTH_TOKEN_SECRET env，与 4.2 mysql.dsn
	// 同模式（密钥不入仓库 YAML）。
	TokenSecret string `yaml:"token_secret"`

	// TokenExpireSec 是默认 token 过期时间（秒）。epics.md 行 1014 钦定默认 7 天 = 604800。
	//
	// 配置可覆盖（如 dev 环境短到 1 小时方便测试）；范围限制 (0, 30*86400] 在
	// `internal/pkg/auth.New` 内实装，超过 30 天的 token 接近 "永不过期"，
	// 违反 V1 §4.1 "默认过期 7 天" 契约语义。
	//
	// loader.go 兜底：YAML 没填 → 默认 604800（7 天）。
	TokenExpireSec int64 `yaml:"token_expire_sec"`
}

// RateLimitConfig 是限频中间件配置。Story 4.5 引入；选型 / 默认值由 epics.md
// §Story 4.5（行 1039）+ V1 §4.1 行 218 钦定。
//
// 字段不在 config 包做业务校验（无 Validate 方法），fail-fast 由
// `internal/app/http/middleware.RateLimit` 工厂在启动期承担：PerKeyPerMin ≤ 0
// → panic（启动期就暴露，与 4.4 auth.New 同模式）。
//
// **节点 2 阶段是内存 token bucket**：单实例部署 OK；多实例 / 节点 10+
// Story 10.6 接 Redis 后，本 struct 加 `Backend string yaml:"backend"`
// 字段切换实装。
//
// 没有 secret 类字段：本 struct 全是非敏感参数，可放 checked-in YAML 默认值
// （与 docs/lessons/2026-04-26-checked-in-config-must-boot-default.md 钦定的
// "非 secret 字段必须 fresh clone 直接跑" 一致）；**不**加 env override
// （节点 2 阶段所有 env 都用 60，无差异化需求）。
//
// # 为什么字段是 `*int64` 而非 `int`
//
// 区分"YAML 缺字段"（nil）和"YAML 显式写 0"（&0）。round 2 codex review [P2]
// 拦下的反向纠偏：旧实现用 `int + == 0 兜底默认值` 把 `per_key_per_min: 0`
// 静默替换为 60 → 用户期望禁限频或拼写错被掩盖 → 启动正常但策略不符预期。
//
// 语义切分：
//   - **loader.go**：nil（key omitted）→ 填默认值 `&60`；非 nil（含 *0 / *负数）
//     → 透传不动
//   - **middleware.RateLimit 工厂**：deref 后走原 fail-fast 路径：PerKeyPerMin
//     nil 或 *≤0 → panic；BurstSize / BucketsLimit nil 或 *≤0 → 用默认兜底
//
// 这样 YAML 显式 `per_key_per_min: 0` 直达 middleware 工厂被 panic 拦截，
// 与文档钦定的 fail-fast 契约一致。
//
// 详见 docs/lessons/2026-04-26-yaml-default-must-not-mask-explicit-invalid.md。
type RateLimitConfig struct {
	// PerKeyPerMin 是每 key（IP 或 userID）每分钟允许的请求数。
	//
	// 默认 60（epics.md 行 1039 + V1 §4.1 行 218 钦定）。可调小（如 30）保守
	// 限频或调大（如 120）放宽（但 > 600 接近"无限频"，违反限频初衷）。
	//
	// 范围限制：(0, +∞)，由 middleware.RateLimit 工厂在启动期校验；nil 或 *≤0 → panic。
	//
	// 类型为 `*int64` 而非 `int`：见 RateLimitConfig 顶部"为什么字段是 *int64"。
	PerKeyPerMin *int64 `yaml:"per_key_per_min"`

	// BurstSize 是 token bucket 容量（瞬时突发上限）。
	//
	// 默认 = PerKeyPerMin（让 epics.md 钦定的"1 分钟内 60 次内 → 通过"
	// 测试 happy path 一次 burst 60 也能通过；epics.md 行 1047 钦定的语义就是
	// burst-friendly）。
	//
	// 调小（如 10）让 burst 更保守 —— 防 client bug 突发雪崩 server。
	//
	// 类型为 `*int64` 而非 `int`：见 RateLimitConfig 顶部"为什么字段是 *int64"。
	// 与 PerKeyPerMin 不同，BurstSize 的 nil / *≤0 走 middleware 工厂兜底（用
	// PerKeyPerMin 默认），**不** panic —— BurstSize 显式 0 没有"用户想禁掉"的
	// 真实场景（burst=0 等价于"完全限频"，即 PerKeyPerMin=0），落到 PerKeyPerMin
	// 那一步 panic 已足够。
	BurstSize *int64 `yaml:"burst_size"`

	// BucketsLimit 是内存中保存的 bucket 数上限（防 IP 洪泛 OOM）。
	//
	// 默认 10000（约 1MB 内存）。每个 limiter ~100 字节；超过该上限的新 key
	// 走共享降级 bucket。生产单实例 QPS 不高（节点 2 阶段未对外发布）→
	// 10000 远超实际负载。
	//
	// 节点 10+ 切 Redis 后本字段废弃（Redis 自身 eviction 处理）。
	//
	// 类型为 `*int64` 而非 `int`：见 RateLimitConfig 顶部"为什么字段是 *int64"。
	// 同 BurstSize，nil / *≤0 走 middleware 工厂兜底（10000），**不** panic ——
	// BucketsLimit=0 没有真实业务语义（"不让任何 key 进 bucket map"等价于
	// "全部走 overflow shared limiter"，与"完全限频"不同，但这种极端配置应该
	// 被默认值兜底而非 fail-fast，与既有 ≤0 → 兜底 10000 行为一致）。
	BucketsLimit *int64 `yaml:"buckets_limit"`
}

type LogConfig struct {
	Level string `yaml:"level"`
}
